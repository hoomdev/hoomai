//go:build !windows

// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre el Studio
// (CA-814, aclaracion 20): POST /api/tasks/{slug}/done rechaza con 409 y
// {"error": <el mensaje de sin-cierre>} un ultimo veredicto completo con
// no_aplica, sin cerrar (ni el item ni el worktree cambian); tras un verde de
// `hoom verify --cierre` commiteado con la misma huella, cierra. Y (CA-813,
// aclaracion 3) POST /api/verify es siempre un verify de trabajo.
//
// Usa los fixtures del paquete: newServer, doPOST, tbErrorJSON, tbItem,
// tbFoto y veeVerify. Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc,
// sin leer la implementacion.
package servecmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

func gprGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gprCommit(t *testing.T, dir, msg string) {
	t.Helper()
	gprGit(t, dir, "add", "-A")
	gprGit(t, dir, "commit", "-q", "-m", msg)
}

// gprProyecto es un proyecto git con base main, test sin aplica_en y static
// y mutation con aplica_en: [app/**] (comandos que pasan).
func gprProyecto(t *testing.T) string {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER", "HOOM_TASK"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gprGit(t, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	gprGit(t, root, "config", "user.email", "test@hoom.dev")
	gprGit(t, root, "config", "user.name", "hoom test")
	yaml := "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"
	for _, n := range []string{"static", "mutation"} {
		yaml += "  " + n + ":\n    required: true\n    cmd: \"true\"\n    diff_cmd: \"true\"\n    aplica_en: [\"app/**\"]\n"
	}
	tbEscribir(t, root, manifest.FileName, yaml)
	tbEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	tbEscribir(t, root, "app/Models/User.php", "<?php\n")
	tbEscribir(t, root, "resources/js/app.js", "console.log(1)\n")
	gprCommit(t, root, "inicial")
	return root
}

func gprVerify(t *testing.T, ca, dir string, opt verifycmd.Options) *verdict.Verdict {
	t.Helper()
	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatalf("%s: fixture: manifest.Load(%s): %v", ca, dir, err)
	}
	v, _, err := verifycmd.Run(m, opt)
	if err != nil {
		t.Fatalf("%s: verify corre: %v", ca, err)
	}
	return v
}

// CA-813 y aclaracion 3: POST /api/verify no recibe cierre: el Studio hace
// siempre un verify de trabajo. Con solo resources/js/app.js cambiado en la
// rama, static y mutation quedan en no_aplica (veeVerify, de
// verify_en_el_entorno_test.go).
func TestCA813_ElVerifyDelStudioEsDeTrabajo(t *testing.T) {
	const ca = "CA-813"
	root := gprProyecto(t)
	gprGit(t, root, "checkout", "-q", "-b", "feature")
	tbEscribir(t, root, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, root, "solo el js")
	code, body, v := veeVerify(t, root)
	if code != http.StatusOK || v == nil {
		t.Fatalf("%s: POST /api/verify responde 200 con el veredicto: %d %s", ca, code, body)
	}
	if v.Summary.NoAplica != 2 || v.Verdict != "green" {
		t.Fatalf("%s: el verify del Studio es de trabajo: static y mutation en no_aplica: %s %+v", ca, v.Verdict, v.Summary)
	}
	for _, g := range v.Gates {
		if (g.Name == "static" || g.Name == "mutation") && g.Status != verdict.StatusNoAplica {
			t.Fatalf("%s: el Studio no hace un verify de cierre: %s quedo %s", ca, g.Name, g.Status)
		}
	}
}

// CA-814 y aclaraciones 18 y 20: el Studio responde 409 con el mensaje de
// sin-cierre de taskcmd.Ready, verbatim, y no cierra; tras el verde de
// --cierre commiteado, responde 200 {"ok": true} y la tarea se cierra.
func TestCA814_StudioRechazaSinCierreYCierraTrasElCierre(t *testing.T) {
	const ca = "CA-814"
	const slug = "precios"
	root := gprProyecto(t)
	tbItem(t, root, slug, "")
	if err := taskcmd.Start(root, slug, "main"); err != nil {
		t.Fatalf("%s: fixture: hoom task start: %v", ca, err)
	}
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	tbEscribir(t, wt, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, wt, "solo el js")
	v := gprVerify(t, ca, wt, verifycmd.Options{})
	if v.Summary.NoAplica != 2 || v.Verdict != "green" {
		t.Fatalf("%s: fixture: el verify de trabajo deja dos no_aplica y es verde: %s %+v", ca, v.Verdict, v.Summary)
	}
	gprCommit(t, wt, "veredicto de trabajo")

	want := fmt.Sprintf(taskcmd.FormatoSinCierre, slug, "static, mutation")
	if err := taskcmd.Ready(root, slug, "main"); err == nil || err.Error() != want {
		t.Fatalf("%s: fixture: Ready da el texto de sin-cierre:\n  %q\nfue\n  %v", ca, want, err)
	}
	itemPath := filepath.Join(root, ".hoom", "items", slug+".yaml")
	itemAntes, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatal(err)
	}
	fotoAntes := tbFoto(t, wt)

	s := newServer(t, root)
	msg := tbErrorJSON(t, ca, doPOST(t, s, "/api/tasks/"+slug+"/done", s.Token(), nil, ""), http.StatusConflict)
	if msg != want {
		t.Fatalf("%s: el 409 trae el mensaje de sin-cierre, verbatim:\n  %q\nfue\n  %q", ca, want, msg)
	}
	if raw, err := os.ReadFile(itemPath); err != nil || !bytes.Equal(raw, itemAntes) {
		t.Fatalf("%s: el item queda igual: %v\n%s", ca, err, raw)
	}
	fotoDespues := tbFoto(t, wt)
	for k, v := range fotoAntes {
		if w, ok := fotoDespues[k]; !ok || w != v {
			t.Fatalf("%s: el worktree queda igual: %s cambio o desaparecio", ca, k)
		}
	}
	for k := range fotoDespues {
		if _, ok := fotoAntes[k]; !ok {
			t.Fatalf("%s: el worktree queda igual: aparecio %s", ca, k)
		}
	}

	c := gprVerify(t, ca, wt, verifycmd.Options{Cierre: true})
	if c.Summary.NoAplica != 0 || c.Verdict != "green" {
		t.Fatalf("%s: --cierre deja un verde sin no_aplica: %s %+v", ca, c.Verdict, c.Summary)
	}
	gprCommit(t, wt, "veredicto de cierre")
	rec := doPOST(t, s, "/api/tasks/"+slug+"/done", s.Token(), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: tras el verde de --cierre, done responde 200: %d %s", ca, rec.Code, rec.Body.String())
	}
	var ok map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &ok); err != nil || !ok["ok"] {
		t.Fatalf("%s: la respuesta de done es {\"ok\": true}: %s", ca, rec.Body.String())
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("%s: done quito el worktree (err=%v)", ca, err)
	}
	if it, err := item.Load(root, slug); err != nil || it.HechoEn == nil {
		t.Fatalf("%s: el item registra el cierre: %+v %v", ca, it, err)
	}
}
