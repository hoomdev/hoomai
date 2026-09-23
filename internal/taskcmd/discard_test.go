// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-346): `hoom task discard <slug> [--yes] [--json]` vuelve el espacio de
// trabajo de la tarea a HEAD FUERA de .hoom/: lo que HEAD tiene se restaura
// (modificado, borrado, en el indice), lo que HEAD no tiene se borra. hoom
// nunca descarta evidencia. Sin --yes no toca nada. Repos git reales.
package taskcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

const cbSlug = "precios"

func cbGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func cbEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cbLeer(t *testing.T, dir, rel string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, rel))
	if err != nil {
		return "", false
	}
	return string(raw), true
}

func cbMismos(a, b []string) bool {
	x := append([]string{}, a...)
	y := append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, "\n") == strings.Join(y, "\n")
}

// cbFoto es el estado completo del worktree: status de git y contenido de
// cada archivo (fuera de .git). Dos fotos iguales = no se toco nada.
func cbFoto(t *testing.T, wt string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString(cbGit(t, wt, "status", "--porcelain", "--untracked-files=all"))
	b.WriteString("\n--\n")
	filepath.WalkDir(wt, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		raw, _ := os.ReadFile(p)
		rel, _ := filepath.Rel(wt, p)
		fmt.Fprintf(&b, "%s=%q\n", rel, raw)
		return nil
	})
	return b.String()
}

// cbTareaSucia arma la tarea precios (como `hoom task start`) con de todo sin
// commitear: un archivo modificado, uno borrado, uno modificado y agregado al
// indice, uno nuevo, uno nuevo agregado al indice, uno nuevo en un
// subdirectorio, y evidencia bajo .hoom/ (un veredicto nuevo y un spec
// modificado) que nunca se descarta.
func cbTareaSucia(t *testing.T) (root, wt string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cbGit(t, root, "init", "-b", "main")
	cbGit(t, root, "config", "user.email", "test@hoom.dev")
	cbGit(t, root, "config", "user.name", "hoom test")
	cbEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	cbEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	cbEscribir(t, root, ".hoom/specs/"+cbSlug+".md", "# Spec: precios\n")
	cbEscribir(t, root, "app.go", "package app\n")
	cbEscribir(t, root, "viejo.go", "package app\n\nfunc Viejo() {}\n")
	cbEscribir(t, root, "lib.go", "package app\n\nfunc Lib() {}\n")
	cbGit(t, root, "add", "-A")
	cbGit(t, root, "commit", "-q", "-m", "inicial")
	if err := Start(root, cbSlug, "main"); err != nil {
		t.Fatalf("CA-346: fixture: task start: %v", err)
	}
	wt = filepath.Join(root, ".hoom", "worktrees", cbSlug)

	cbEscribir(t, wt, "app.go", "package app\n\nfunc Roto() {}\n")   // modificado
	if err := os.Remove(filepath.Join(wt, "viejo.go")); err != nil { // borrado
		t.Fatal(err)
	}
	cbEscribir(t, wt, "lib.go", "package app\n\nfunc LibRota() {}\n") // modificado y en el indice
	cbGit(t, wt, "add", "lib.go")
	cbEscribir(t, wt, "nuevo.go", "package app\n")    // nuevo
	cbEscribir(t, wt, "indexado.go", "package app\n") // nuevo y en el indice
	cbGit(t, wt, "add", "indexado.go")
	cbEscribir(t, wt, "pkg/nuevo/x.go", "package nuevo\n") // nuevo en un subdirectorio

	// evidencia: nunca se descarta
	cbEscribir(t, wt, ".hoom/verdicts/20260923T100000_cbv001.json", "{\"verdict\":\"green\"}\n")
	cbEscribir(t, wt, ".hoom/specs/"+cbSlug+".md", "# Spec: precios\n\nCA nuevo sin commitear\n")
	return root, wt
}

var (
	cbRestaurar = []string{
		".hoom/worktrees/" + cbSlug + "/app.go",
		".hoom/worktrees/" + cbSlug + "/viejo.go",
		".hoom/worktrees/" + cbSlug + "/lib.go",
	}
	cbBorrar = []string{
		".hoom/worktrees/" + cbSlug + "/nuevo.go",
		".hoom/worktrees/" + cbSlug + "/indexado.go",
		".hoom/worktrees/" + cbSlug + "/pkg/nuevo/x.go",
	}
)

func cbDescartables() []string { return append(append([]string{}, cbRestaurar...), cbBorrar...) }

// cbRunActivo deja el sidecar de un run en curso de OTRO proceso vivo (este)
// en el arbol de la tarea, como lo deja runcmd en .hoom/runs/ del proyecto.
func cbRunActivo(t *testing.T, root, wt, id string) {
	t.Helper()
	raw, _ := json.MarshalIndent(map[string]any{
		"id": id, "provider": "claude", "role": "writer", "task": cbSlug, "dir": wt,
		"created_at": time.Now().UTC(), "status": "running", "exit_code": -1, "pid": os.Getpid(),
	}, "", "  ")
	cbEscribir(t, root, filepath.Join(".hoom", "runs", id+".meta.json"), string(raw))
}

// CA-346: Discardable lista las rutas sin guardar del worktree FUERA de
// .hoom/, relativas a la raiz del proyecto.
func TestCA346_DiscardableListaFueraDeHoom(t *testing.T) {
	root, _ := cbTareaSucia(t)
	got, err := Discardable(root, cbSlug)
	if err != nil {
		t.Fatalf("CA-346: Discardable: %v", err)
	}
	if !cbMismos(got, cbDescartables()) {
		t.Fatalf("CA-346: lo descartable son las rutas sin guardar fuera de .hoom/, relativas a la raiz:\n got  %v\n want %v", got, cbDescartables())
	}
	for _, p := range got {
		if strings.HasPrefix(p, ".hoom/worktrees/"+cbSlug+"/.hoom/") {
			t.Fatalf("CA-346: la evidencia bajo .hoom/ nunca es descartable: %s", p)
		}
	}
}

// CA-346: sin --yes, la CLI lista las rutas, falla (exit 1) y no toca nada.
func TestCA346_SinYesListaYNoTocaNada(t *testing.T) {
	root, wt := cbTareaSucia(t)
	antes := cbFoto(t, wt)
	var out bytes.Buffer
	err := RunDiscard(root, cbSlug, false, false, &out)
	if err == nil {
		t.Fatalf("CA-346: sin --yes, hoom task discard termina con error (exit 1):\n%s", out.String())
	}
	for _, p := range cbDescartables() {
		if !strings.Contains(out.String(), p) {
			t.Fatalf("CA-346: sin --yes se lista %s:\n%s", p, out.String())
		}
	}
	if texto := out.String() + err.Error(); !strings.Contains(texto, "Accion: repeti con --yes para descartarlos (no se puede deshacer)") {
		t.Fatalf("CA-346: sin --yes se dice \"Accion: repeti con --yes para descartarlos (no se puede deshacer)\":\n%s", texto)
	}
	if despues := cbFoto(t, wt); despues != antes {
		t.Fatalf("CA-346: sin --yes no se toca nada:\nantes:\n%s\ndespues:\n%s", antes, despues)
	}
}

// CA-346: Discard vuelve a HEAD lo que HEAD tiene (modificado, borrado y lo
// que estaba en el indice) y borra lo que HEAD no tiene, y NUNCA toca .hoom/:
// un veredicto sin commitear y un spec modificado siguen ahi.
func TestCA346_DiscardRestauraYBorraSinTocarLaEvidencia(t *testing.T) {
	root, wt := cbTareaSucia(t)
	res, err := Discard(root, cbSlug, nil)
	if err != nil {
		t.Fatalf("CA-346: Discard: %v", err)
	}
	if res.Slug != cbSlug {
		t.Fatalf("CA-346: el resultado nombra la tarea: %+v", res)
	}
	if !cbMismos(res.Restored, cbRestaurar) {
		t.Fatalf("CA-346: restored son las rutas que HEAD tiene, relativas a la raiz: %v", res.Restored)
	}
	if !cbMismos(res.Removed, cbBorrar) {
		t.Fatalf("CA-346: removed son las rutas que HEAD no tiene, relativas a la raiz: %v", res.Removed)
	}

	for rel, want := range map[string]string{
		"app.go":   "package app\n",
		"viejo.go": "package app\n\nfunc Viejo() {}\n",
		"lib.go":   "package app\n\nfunc Lib() {}\n",
	} {
		if got, ok := cbLeer(t, wt, rel); !ok || got != want {
			t.Fatalf("CA-346: %s vuelve a como esta en HEAD: %q (existe: %v)", rel, got, ok)
		}
	}
	for _, rel := range []string{"nuevo.go", "indexado.go", "pkg/nuevo/x.go"} {
		if _, ok := cbLeer(t, wt, rel); ok {
			t.Fatalf("CA-346: %s no esta en HEAD y se borra", rel)
		}
	}
	if idx := cbGit(t, wt, "diff", "--cached", "--name-only"); idx != "" {
		t.Fatalf("CA-346: lo que estaba en el indice vuelve a HEAD (indice vacio): %q", idx)
	}

	// la evidencia no se toca
	if got, ok := cbLeer(t, wt, ".hoom/verdicts/20260923T100000_cbv001.json"); !ok || got != "{\"verdict\":\"green\"}\n" {
		t.Fatalf("CA-346: un veredicto sin commitear sigue ahi, intacto: %q (existe: %v)", got, ok)
	}
	if got, _ := cbLeer(t, wt, ".hoom/specs/"+cbSlug+".md"); got != "# Spec: precios\n\nCA nuevo sin commitear\n" {
		t.Fatalf("CA-346: un spec modificado bajo .hoom/ no se descarta: %q", got)
	}

	// lo unico sin guardar que queda es la evidencia
	for _, l := range strings.Split(cbGit(t, wt, "status", "--porcelain", "--untracked-files=all"), "\n") {
		if l = strings.TrimSpace(l); l == "" {
			continue
		}
		if p := strings.TrimSpace(l[2:]); !strings.HasPrefix(p, ".hoom/") {
			t.Fatalf("CA-346: despues de descartar, lo unico sin guardar es la evidencia: %q", l)
		}
	}
	if quedan, err := Discardable(root, cbSlug); err != nil || len(quedan) != 0 {
		t.Fatalf("CA-346: despues de descartar no queda nada descartable: %v %v", quedan, err)
	}
}

// CA-346: con --yes y --json, la CLI emite slug, restored y removed.
func TestCA346_RunDiscardYesJSON(t *testing.T) {
	root, _ := cbTareaSucia(t)
	var out bytes.Buffer
	if err := RunDiscard(root, cbSlug, true, true, &out); err != nil {
		t.Fatalf("CA-346: hoom task discard --yes --json: %v\n%s", err, out.String())
	}
	var got struct {
		Slug     *string   `json:"slug"`
		Restored *[]string `json:"restored"`
		Removed  *[]string `json:"removed"`
	}
	if err := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &got); err != nil {
		t.Fatalf("CA-346: --json emite un objeto JSON: %v\n%s", err, out.String())
	}
	if got.Slug == nil || *got.Slug != cbSlug || got.Restored == nil || got.Removed == nil {
		t.Fatalf("CA-346: el JSON trae slug, restored y removed:\n%s", out.String())
	}
	if !cbMismos(*got.Restored, cbRestaurar) || !cbMismos(*got.Removed, cbBorrar) {
		t.Fatalf("CA-346: restored y removed son las rutas relativas a la raiz:\n%s", out.String())
	}
}

// CA-346: una tarea que no existe es un error, sin efectos.
func TestCA346_TareaInexistente(t *testing.T) {
	root, wt := cbTareaSucia(t)
	antes := cbFoto(t, wt)
	if _, err := Discardable(root, "no-existe"); err == nil {
		t.Fatal("CA-346: Discardable de una tarea que no existe es un error")
	}
	if _, err := Discard(root, "no-existe", nil); err == nil {
		t.Fatal("CA-346: Discard de una tarea que no existe es un error")
	}
	var out bytes.Buffer
	if err := RunDiscard(root, "no-existe", true, false, &out); err == nil {
		t.Fatal("CA-346: hoom task discard de una tarea que no existe es un error")
	}
	if despues := cbFoto(t, wt); despues != antes {
		t.Fatal("CA-346: descartar una tarea que no existe no toca otra")
	}
}

// CA-346: con un run activo en el arbol de la tarea, Discard se niega
// nombrando el run y no toca nada.
func TestCA346_ConRunActivoSeNiega(t *testing.T) {
	root, wt := cbTareaSucia(t)
	const id = "20260923T120000_cbrun1"
	cbRunActivo(t, root, wt, id)
	antes := cbFoto(t, wt)
	_, err := Discard(root, cbSlug, nil)
	if err == nil || !strings.Contains(err.Error(), id) {
		t.Fatalf("CA-346: con un run activo en el arbol, Discard se niega nombrando el run %s: %v", id, err)
	}
	if despues := cbFoto(t, wt); despues != antes {
		t.Fatalf("CA-346: con un run activo no se toca nada:\nantes:\n%s\ndespues:\n%s", antes, despues)
	}
	var out bytes.Buffer
	if err := RunDiscard(root, cbSlug, true, false, &out); err == nil {
		t.Fatal("CA-346: la CLI con --yes tambien se niega con un run activo")
	}
	if despues := cbFoto(t, wt); despues != antes {
		t.Fatal("CA-346: la CLI con un run activo no toca nada")
	}
}

// CA-346: expect no nil tiene que ser igual, como conjunto, a lo descartable:
// si no, la misma negativa que Guardar. El orden no importa.
func TestCA346_ExpectDistintoSeNiega(t *testing.T) {
	root, wt := cbTareaSucia(t)
	antes := cbFoto(t, wt)
	for _, expect := range [][]string{
		cbRestaurar, // le faltan rutas
		append(cbDescartables(), ".hoom/worktrees/"+cbSlug+"/otra.go"), // le sobra una
		{}, // no nil y vacio
	} {
		_, err := Discard(root, cbSlug, expect)
		if err == nil || !strings.Contains(err.Error(), "cambiaron desde que los viste") {
			t.Fatalf("CA-346: expect %v distinto de lo descartable se niega (\"cambiaron desde que los viste\"): %v", expect, err)
		}
		if despues := cbFoto(t, wt); despues != antes {
			t.Fatalf("CA-346: con expect distinto no se toca nada (expect %v)", expect)
		}
	}
	al := cbDescartables()
	sort.Sort(sort.Reverse(sort.StringSlice(al)))
	res, err := Discard(root, cbSlug, al)
	if err != nil {
		t.Fatalf("CA-346: expect igual como conjunto descarta: %v", err)
	}
	if !cbMismos(res.Restored, cbRestaurar) || !cbMismos(res.Removed, cbBorrar) {
		t.Fatalf("CA-346: con expect igual se descarta todo: %+v", res)
	}
}
