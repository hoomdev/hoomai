//go:build !windows

// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre el cierre
// (CA-814, CAL-9.2), como consumidor externo de taskcmd:
//
//   - taskcmd.Ready rechaza un ultimo veredicto completo y verde con algun
//     no_aplica con el kind sin-cierre y el texto exacto del spec, con los
//     gates en el orden del veredicto (aclaracion 17), y `hoom task done`
//     (Done sin --force) da el mismo error sin cerrar;
//   - el orden de Ready (aclaracion 16): rojo antes que sin-cierre, y
//     sin-cierre antes que la huella;
//   - de punta a punta con verifycmd: el verify de trabajo deja no_aplica, el
//     cierre se niega, `hoom check` lo acepta, y tras un verde de --cierre
//     commiteado con la misma huella (aclaracion 18) task done cierra.
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package taskcmd_test

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

const gprSlug = "precios"

// gprTexto es el mensaje del spec para la tarea precios con static y
// mutation en no_aplica, letra por letra.
const gprTexto = "el ultimo veredicto de \"precios\" no es de cierre: no aplicaron static, mutation.\n" +
	"  Accion: ejecuta 'hoom verify --cierre' dentro del worktree y commitea"

func gprAislar(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER", "HOOM_TASK",
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

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

func gprEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gprCommit(t *testing.T, dir, msg string) {
	t.Helper()
	gprGit(t, dir, "add", "-A")
	gprGit(t, dir, "commit", "-q", "-m", msg)
}

// gprTarea es un proyecto con test sin aplica_en y static y mutation con
// aplica_en: [app/**] (comandos que pasan), y la tarea slug creada con
// taskcmd.Start. Devuelve la raiz y el worktree de la tarea.
func gprTarea(t *testing.T, slug string) (root, wt string) {
	t.Helper()
	gprAislar(t)
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
	gprEscribir(t, root, manifest.FileName, yaml)
	gprEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	gprEscribir(t, root, "app/Models/User.php", "<?php\n")
	gprEscribir(t, root, "resources/js/app.js", "console.log(1)\n")
	gprCommit(t, root, "inicial")
	if err := taskcmd.Start(root, slug, "main"); err != nil {
		t.Fatalf("fixture: hoom task start %s: %v", slug, err)
	}
	return root, filepath.Join(root, ".hoom", "worktrees", slug)
}

// gprVeredicto escribe en wt un veredicto completo con esos gates y la huella
// actual (o huella, si no es "").
func gprVeredicto(t *testing.T, wt string, at time.Time, huella string, gs ...verdict.GateResult) *verdict.Verdict {
	t.Helper()
	g := gitx.Snapshot(wt, "main")
	if huella != "" {
		g.ChangeFingerprint = huella
	}
	v := &verdict.Verdict{Schema: verdict.SchemaID, Project: "demo", CreatedAt: at, Git: g, Gates: gs}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	return v
}

func gprGateDe(nombre, status string) verdict.GateResult {
	g := verdict.GateResult{Name: nombre, Required: true, Status: status, Scope: "full", Command: "true"}
	switch status {
	case verdict.StatusNoAplica:
		g.Scope, g.Command, g.Notes = "none", "", fmt.Sprintf(gates.MotivoNoAplica, "app/**")
	case verdict.StatusFail:
		g.ExitCode = 1
	}
	return g
}

// gprReady exige que Ready (y Done sin --force) rechacen con el kind y el
// mensaje exactos, y que la tarea no se cierre.
func gprReady(t *testing.T, ca, root, wt, kind, msg string) {
	t.Helper()
	err := taskcmd.Ready(root, gprSlug, "main")
	var re *taskcmd.ReadyError
	if !errors.As(err, &re) {
		t.Fatalf("%s: Ready rechaza con un *taskcmd.ReadyError (%s), fue %T: %v", ca, kind, err, err)
	}
	if re.Kind != kind || re.Msg != msg || err.Error() != msg {
		t.Fatalf("%s: Ready da el kind %q y el texto\n  %q\nfue %q\n  %q", ca, kind, msg, re.Kind, err.Error())
	}
	derr := taskcmd.Done(root, gprSlug, "main", false)
	if derr == nil || derr.Error() != msg {
		t.Fatalf("%s: hoom task done rechaza con el mismo texto:\n  %q\nfue\n  %v", ca, msg, derr)
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("%s: rechazar no cierra: el worktree sigue: %v", ca, err)
	}
	if gprGit(t, root, "branch", "--list", "hoom/"+gprSlug) == "" {
		t.Fatalf("%s: rechazar no cierra: la rama sigue", ca)
	}
}

// gprNoAplicaDe son los gates en no_aplica de v, en su orden, unidos por ", ".
func gprNoAplicaDe(v *verdict.Verdict) string {
	var out []string
	for _, g := range v.Gates {
		if g.Status == verdict.StatusNoAplica {
			out = append(out, g.Name)
		}
	}
	return strings.Join(out, ", ")
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

// ---------------------------------------------------------------- CA-814

// CA-814 y aclaracion 17: el kind es sin-cierre, el formato es el del spec, y
// con un ultimo veredicto completo y verde con static y mutation en no_aplica
// Ready y task done rechazan con ese texto exacto, sin cerrar.
func TestCA814_ReadyRechazaSinCierreConElTextoExacto(t *testing.T) {
	const ca = "CA-814"
	if taskcmd.ReadySinCierre != "sin-cierre" {
		t.Fatalf("%s: el kind es sin-cierre: %q", ca, taskcmd.ReadySinCierre)
	}
	if got := fmt.Sprintf(taskcmd.FormatoSinCierre, gprSlug, "static, mutation"); got != gprTexto {
		t.Fatalf("%s: el texto del spec:\n  %q\nfue\n  %q", ca, gprTexto, got)
	}
	root, wt := gprTarea(t, gprSlug)
	gprEscribir(t, wt, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, wt, "solo el js")
	gprVeredicto(t, wt, time.Now().UTC(), "",
		gprGateDe("test", verdict.StatusPass), gprGateDe("static", verdict.StatusNoAplica), gprGateDe("mutation", verdict.StatusNoAplica))
	gprCommit(t, wt, "veredicto de trabajo")
	gprReady(t, ca, root, wt, taskcmd.ReadySinCierre, gprTexto)
}

// CA-814 y aclaracion 16: sin-cierre solo sale con un ultimo completo VERDE:
// uno rojo con no_aplica es rojo; y va antes que la huella: un verde con
// no_aplica y otra huella es sin-cierre.
func TestCA814_SinCierreDespuesDeRojoYAntesDeLaHuella(t *testing.T) {
	const ca = "CA-814"
	root, wt := gprTarea(t, gprSlug)
	gprEscribir(t, wt, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, wt, "solo el js")
	t0 := time.Now().UTC()

	rojo := gprVeredicto(t, wt, t0, "",
		gprGateDe("test", verdict.StatusFail), gprGateDe("static", verdict.StatusNoAplica), gprGateDe("mutation", verdict.StatusNoAplica))
	gprCommit(t, wt, "rojo con no_aplica")
	gprReady(t, ca, root, wt, taskcmd.ReadyRojo,
		fmt.Sprintf("el ultimo veredicto de %q es ROJO (%s). Accion: corrige y re-ejecuta 'hoom verify' en el worktree", gprSlug, rojo.ID))

	gprVeredicto(t, wt, t0.Add(time.Second), "otra-huella",
		gprGateDe("test", verdict.StatusPass), gprGateDe("static", verdict.StatusNoAplica), gprGateDe("mutation", verdict.StatusNoAplica))
	gprCommit(t, wt, "verde con no_aplica y otra huella")
	gprReady(t, ca, root, wt, taskcmd.ReadySinCierre, gprTexto)
}

// CA-814 (CAL-9.2) de punta a punta: el verify de trabajo de la tarea, con
// solo resources/js/app.js cambiado, deja static y mutation en no_aplica;
// commiteado, task done lo rechaza con el texto exacto (los gates en el orden
// del veredicto) y `hoom check` lo acepta; tras un verde de --cierre,
// commiteado y con la misma huella, Ready da nil y task done cierra.
func TestCA814_TaskDoneCierraTrasElVerifyDeCierre(t *testing.T) {
	const ca = "CA-814"
	root, wt := gprTarea(t, gprSlug)
	gprEscribir(t, wt, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, wt, "solo el js")

	v := gprVerify(t, ca, wt, verifycmd.Options{})
	if v.Summary.NoAplica != 2 || v.Verdict != "green" || v.IsPartial() {
		t.Fatalf("%s: fixture: el verify de trabajo deja dos no_aplica y es verde: %s %+v", ca, v.Verdict, v.Summary)
	}
	if got := gprNoAplicaDe(v); got != "static, mutation" {
		t.Fatalf("%s: fixture: los no_aplica del veredicto, en su orden: %q", ca, got)
	}
	gprCommit(t, wt, "veredicto de trabajo")
	gprReady(t, ca, root, wt, taskcmd.ReadySinCierre, fmt.Sprintf(taskcmd.FormatoSinCierre, gprSlug, gprNoAplicaDe(v)))

	res, err := checkcmd.Run(wt, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK || res.ExitCode() != 0 || res.VerdictID != v.ID {
		t.Fatalf("%s: hoom check acepta el veredicto con no_aplica: %+v", ca, res)
	}

	c := gprVerify(t, ca, wt, verifycmd.Options{Cierre: true})
	if c.Summary.NoAplica != 0 || c.Verdict != "green" || c.IsPartial() {
		t.Fatalf("%s: --cierre deja un verde completo sin no_aplica: %s %+v", ca, c.Verdict, c.Summary)
	}
	gprCommit(t, wt, "veredicto de cierre")
	if c.Git.ChangeFingerprint != gitx.Snapshot(wt, "main").ChangeFingerprint {
		t.Fatalf("%s: fixture: el verde de --cierre tiene la huella del arbol", ca)
	}
	if err := taskcmd.Ready(root, gprSlug, "main"); err != nil {
		t.Fatalf("%s: tras el verde de --cierre, Ready es nil: %v", ca, err)
	}
	if err := taskcmd.Done(root, gprSlug, "main", false); err != nil {
		t.Fatalf("%s: tras el verde de --cierre, task done cierra: %v", ca, err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("%s: task done quito el worktree (err=%v)", ca, err)
	}
}
