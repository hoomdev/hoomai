// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-344): `hoom item save <slug>` commitea EXACTAMENTE lo que la tarjeta
// tiene sin guardar (`unsynced`), un commit por arbol con el mensaje fijo
// `hoom: guardar la tarjeta <slug>`: en el espacio de trabajo de la tarea sus
// rutas, en el arbol raiz el item. Lo que otro dejo en el indice del arbol
// raiz no entra ni se pierde. Repos git reales en directorios temporales.
package itemcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const cbSlug = "precios"

var cbSHA = regexp.MustCompile(`^[0-9a-f]{40}$`)

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

func cbLineas(s string) []string {
	out := []string{}
	for _, l := range strings.Split(s, "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func cbMismos(a, b []string) bool {
	x := append([]string{}, a...)
	y := append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, "\n") == strings.Join(y, "\n")
}

func cbHead(t *testing.T, dir string) string { return cbGit(t, dir, "rev-parse", "HEAD") }

// cbEnIndice lista lo que esta en el indice de dir y todavia no se commiteo.
func cbEnIndice(t *testing.T, dir string) []string {
	return cbLineas(cbGit(t, dir, "diff", "--cached", "--name-only"))
}

// cbDelCommit lista las rutas (relativas a dir) que toco el commit HEAD de dir.
func cbDelCommit(t *testing.T, dir string) []string {
	return cbLineas(cbGit(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"))
}

// cbExec corre `hoom item <args>` como la CLI: Parse y despues Execute.
func cbExec(t *testing.T, root string, args ...string) (string, string, error) {
	t.Helper()
	req, err := Parse(args)
	if err != nil {
		t.Fatalf("CA-344: Parse(%q) entiende el verbo save: %v", args, err)
	}
	var out, errb bytes.Buffer
	err = Execute(root, "main", "high", req, &out, &errb)
	return out.String(), errb.String(), err
}

func cbItemYAML() string {
	return icItemYAML("Precios por region", "2026-09-20T10:00:00Z", "")
}

// cbRepoSave arma un proyecto con la telemetria escondida y dos archivos de
// codigo commiteados.
func cbRepoSave(t *testing.T) string {
	t.Helper()
	root := icRepo(t)
	icEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	icEscribir(t, root, "app.go", "package app\n")
	icEscribir(t, root, "viejo.go", "package app\n\nfunc Viejo() {}\n")
	icGit(t, root, "add", "-A")
	icGit(t, root, "commit", "-q", "-m", "codigo")
	return root
}

// cbTareaSave arma una tarjeta con su tarea (como `hoom task start`) y con
// cosas sin guardar en los dos arboles: en el worktree, un archivo
// modificado, uno nuevo y uno borrado; en el raiz, el item (sin commitear) y
// un archivo AJENO de la persona en el indice.
func cbTareaSave(t *testing.T) (root, wt string) {
	t.Helper()
	root = cbRepoSave(t)
	if err := taskcmd.Start(root, cbSlug, "main"); err != nil {
		t.Fatalf("CA-344: fixture: task start: %v", err)
	}
	wt = filepath.Join(root, ".hoom", "worktrees", cbSlug)
	icEscribir(t, root, ".hoom/items/"+cbSlug+".yaml", cbItemYAML())
	icEscribir(t, root, "ajeno.txt", "esto es de la persona\n")
	icGit(t, root, "add", "ajeno.txt")
	icEscribir(t, wt, "app.go", "package app\n\nfunc Precio() int { return 1 }\n")
	icEscribir(t, wt, "nuevo.go", "package app\n\nfunc Nuevo() {}\n")
	if err := os.Remove(filepath.Join(wt, "viejo.go")); err != nil {
		t.Fatal(err)
	}
	return root, wt
}

var cbEsperadas = []string{
	".hoom/items/" + cbSlug + ".yaml",
	".hoom/worktrees/" + cbSlug + "/app.go",
	".hoom/worktrees/" + cbSlug + "/nuevo.go",
	".hoom/worktrees/" + cbSlug + "/viejo.go",
}

func cbUnsynced(t *testing.T, root string) []string {
	t.Helper()
	c, err := boardcmd.CardFor(root, "main", "high", cbSlug, time.Now().UTC())
	if err != nil {
		t.Fatalf("CA-344: la tarjeta se deriva: %v", err)
	}
	return c.Unsynced
}

// cbSinCambios exige que nada se haya commiteado ni movido del indice.
func cbSinCambios(t *testing.T, caso, root, wt, headRoot, headWt string) {
	t.Helper()
	if h := cbHead(t, root); h != headRoot {
		t.Fatalf("CA-344: %s: no se commitea nada en el arbol raiz: %s -> %s", caso, headRoot, h)
	}
	if h := cbHead(t, wt); h != headWt {
		t.Fatalf("CA-344: %s: no se commitea nada en el worktree: %s -> %s", caso, headWt, h)
	}
	if got := cbUnsynced(t, root); !cbMismos(got, cbEsperadas) {
		t.Fatalf("CA-344: %s: lo sin guardar queda igual: %v", caso, got)
	}
	if got := cbEnIndice(t, root); !cbMismos(got, []string{"ajeno.txt"}) {
		t.Fatalf("CA-344: %s: el indice del arbol raiz queda igual: %v", caso, got)
	}
}

// CA-344: un commit por arbol con el mensaje fijo y la identidad configurada:
// en el worktree sus rutas (modificada, nueva y borrada), en el raiz SOLO el
// item. El archivo ajeno del indice del raiz sigue ahi sin commitear, y
// despues la tarjeta no tiene nada sin guardar.
func TestCA344_SaveCommiteaUnCommitPorArbol(t *testing.T) {
	root, wt := cbTareaSave(t)
	if got := cbUnsynced(t, root); !cbMismos(got, cbEsperadas) {
		t.Fatalf("CA-344: fixture: la tarjeta tiene sin guardar %v, no %v", got, cbEsperadas)
	}
	if SaveMessage(cbSlug) != "hoom: guardar la tarjeta "+cbSlug {
		t.Fatalf("CA-344: el mensaje fijo es \"hoom: guardar la tarjeta <slug>\": %q", SaveMessage(cbSlug))
	}
	headRoot, headWt := cbHead(t, root), cbHead(t, wt)

	res, err := Save(root, "main", "high", cbSlug, nil)
	if err != nil {
		t.Fatalf("CA-344: Save: %v", err)
	}
	if res.Slug != cbSlug || res.Message != "hoom: guardar la tarjeta "+cbSlug {
		t.Fatalf("CA-344: el resultado nombra la tarjeta y el mensaje: %+v", res)
	}
	if len(res.Commits) != 2 {
		t.Fatalf("CA-344: un commit por arbol (worktree y raiz): %+v", res.Commits)
	}
	porDir := map[string]SaveCommit{}
	for _, c := range res.Commits {
		porDir[c.Dir] = c
	}
	cw, okW := porDir[".hoom/worktrees/"+cbSlug]
	cr, okR := porDir["."]
	if !okW || !okR {
		t.Fatalf("CA-344: los commits nombran su arbol como \".hoom/worktrees/%s\" y \".\": %+v", cbSlug, res.Commits)
	}

	// el worktree: sus tres rutas, en UN commit nuevo con el mensaje fijo
	if !cbSHA.MatchString(cw.SHA) || cw.SHA != cbHead(t, wt) || cw.SHA == headWt {
		t.Fatalf("CA-344: el commit del worktree es su nuevo HEAD (%s): %+v", cbHead(t, wt), cw)
	}
	if !cbMismos(cw.Paths, cbEsperadas[1:]) {
		t.Fatalf("CA-344: el commit del worktree lleva sus rutas, relativas a la raiz: %v", cw.Paths)
	}
	if got := cbDelCommit(t, wt); !cbMismos(got, []string{"app.go", "nuevo.go", "viejo.go"}) {
		t.Fatalf("CA-344: el commit del worktree toca exactamente sus rutas (incluida la borrada): %v", got)
	}
	if p := cbGit(t, wt, "rev-parse", "HEAD~1"); p != headWt {
		t.Fatalf("CA-344: un solo commit en el worktree: el padre es %s, no %s", p, headWt)
	}
	if msg := cbGit(t, wt, "log", "-1", "--format=%B"); msg != "hoom: guardar la tarjeta "+cbSlug {
		t.Fatalf("CA-344: el mensaje del commit del worktree es el fijo: %q", msg)
	}
	if autor := cbGit(t, wt, "log", "-1", "--format=%an <%ae>"); autor != "hoom test <test@hoom.dev>" {
		t.Fatalf("CA-344: el commit usa la identidad git configurada: %q", autor)
	}

	// el raiz: SOLO el item
	if !cbSHA.MatchString(cr.SHA) || cr.SHA != cbHead(t, root) || cr.SHA == headRoot {
		t.Fatalf("CA-344: el commit del raiz es su nuevo HEAD (%s): %+v", cbHead(t, root), cr)
	}
	if !cbMismos(cr.Paths, []string{".hoom/items/" + cbSlug + ".yaml"}) {
		t.Fatalf("CA-344: en el arbol raiz se guarda solo el item: %v", cr.Paths)
	}
	if got := cbDelCommit(t, root); !cbMismos(got, []string{".hoom/items/" + cbSlug + ".yaml"}) {
		t.Fatalf("CA-344: el commit del raiz lleva solo el item, no el archivo ajeno: %v", got)
	}
	if p := cbGit(t, root, "rev-parse", "HEAD~1"); p != headRoot {
		t.Fatalf("CA-344: un solo commit en el raiz: el padre es %s, no %s", p, headRoot)
	}
	if msg := cbGit(t, root, "log", "-1", "--format=%B"); msg != "hoom: guardar la tarjeta "+cbSlug {
		t.Fatalf("CA-344: el mensaje del commit del raiz es el fijo: %q", msg)
	}

	// lo ajeno sigue en el indice, sin commitear
	if got := cbEnIndice(t, root); !cbMismos(got, []string{"ajeno.txt"}) {
		t.Fatalf("CA-344: el archivo ajeno sigue en el indice del raiz sin commitear: %v", got)
	}
	// y la tarjeta ya no tiene nada sin guardar
	if got := cbUnsynced(t, root); len(got) != 0 {
		t.Fatalf("CA-344: despues de guardar, unsynced esta vacio: %v", got)
	}
	if st := cbGit(t, wt, "status", "--porcelain"); st != "" {
		t.Fatalf("CA-344: el worktree queda limpio:\n%s", st)
	}
}

// CA-344: en una tarjeta sin tarea (su evidencia vive en el arbol raiz) el
// commit lleva exactamente sus rutas: ni un archivo ajeno del indice ni uno
// sin rastrear que no es de la tarjeta.
func TestCA344_TarjetaDelArbolRaizCommiteaSoloLoSuyo(t *testing.T) {
	root := cbRepoSave(t)
	icEscribir(t, root, ".hoom/items/"+cbSlug+".yaml", cbItemYAML())
	icEscribir(t, root, ".hoom/specs/"+cbSlug+".md", "# Spec: precios\n")
	icEscribir(t, root, "ajeno.txt", "esto es de la persona\n")
	icGit(t, root, "add", "ajeno.txt")
	icEscribir(t, root, "suelto.go", "package app\n")
	propias := []string{".hoom/items/" + cbSlug + ".yaml", ".hoom/specs/" + cbSlug + ".md"}
	if got := cbUnsynced(t, root); !cbMismos(got, propias) {
		t.Fatalf("CA-344: fixture: la tarjeta del raiz tiene sin guardar %v", got)
	}
	head := cbHead(t, root)

	res, err := Save(root, "main", "high", cbSlug, nil)
	if err != nil {
		t.Fatalf("CA-344: Save: %v", err)
	}
	if len(res.Commits) != 1 || res.Commits[0].Dir != "." || !cbMismos(res.Commits[0].Paths, propias) {
		t.Fatalf("CA-344: un solo arbol, un solo commit con las rutas de la tarjeta: %+v", res.Commits)
	}
	if res.Commits[0].SHA != cbHead(t, root) || cbGit(t, root, "rev-parse", "HEAD~1") != head {
		t.Fatalf("CA-344: un commit nuevo en el raiz: %+v", res.Commits[0])
	}
	if got := cbDelCommit(t, root); !cbMismos(got, propias) {
		t.Fatalf("CA-344: el commit lleva solo lo de la tarjeta: %v", got)
	}
	if got := cbEnIndice(t, root); !cbMismos(got, []string{"ajeno.txt"}) {
		t.Fatalf("CA-344: el archivo ajeno sigue en el indice: %v", got)
	}
	if st := cbGit(t, root, "status", "--porcelain", "--", "suelto.go"); !strings.HasPrefix(st, "??") {
		t.Fatalf("CA-344: un archivo que no es de la tarjeta sigue sin rastrear: %q", st)
	}
	if got := cbUnsynced(t, root); len(got) != 0 {
		t.Fatalf("CA-344: despues de guardar, unsynced esta vacio: %v", got)
	}
}

// CA-344: con la tarjeta en curso (un sobre abierto cuyo dueno vive) Save se
// niega sin commitear nada, con la frase del contrato.
func TestCA344_ConLaTarjetaEnCursoSeNiega(t *testing.T) {
	root, wt := cbTareaSave(t)
	envelope.Write(root, envelope.Record{
		ID: "20260923T120000_cbsv01", Role: "writer", Provider: "claude", Task: cbSlug, Dir: wt,
		Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: time.Now().UTC(), PID: os.Getpid(),
	})
	c, err := boardcmd.CardFor(root, "main", "high", cbSlug, time.Now().UTC())
	if err != nil || c.Running == nil {
		t.Fatalf("CA-344: fixture: la tarjeta esta en curso: %+v %v", c.Running, err)
	}
	headRoot, headWt := cbHead(t, root), cbHead(t, wt)

	_, err = Save(root, "main", "high", cbSlug, nil)
	if err == nil || !strings.Contains(err.Error(), "espera a que termine el writer que esta trabajando") {
		t.Fatalf("CA-344: con la tarjeta en curso Save se niega con \"espera a que termine el writer que esta trabajando\": %v", err)
	}
	cbSinCambios(t, "en curso", root, wt, headRoot, headWt)

	// la CLI tambien
	_, _, err = cbExec(t, root, "save", cbSlug)
	if err == nil || !strings.Contains(err.Error(), "espera a que termine el writer que esta trabajando") {
		t.Fatalf("CA-344: hoom item save se niega igual: %v", err)
	}
	cbSinCambios(t, "en curso (CLI)", root, wt, headRoot, headWt)
}

// CA-344: expect no nil tiene que ser igual, como conjunto, a unsynced: si no,
// se niega sin commitear. El orden no importa.
func TestCA344_ExpectDistintoSeNiega(t *testing.T) {
	root, wt := cbTareaSave(t)
	headRoot, headWt := cbHead(t, root), cbHead(t, wt)
	const frase = "los cambios de la tarjeta cambiaron desde que los viste: revisalos de nuevo"

	for _, expect := range [][]string{
		{".hoom/items/" + cbSlug + ".yaml"},                   // le faltan rutas
		append(append([]string{}, cbEsperadas...), "otra.go"), // le sobra una
		{}, // no nil y vacio
	} {
		_, err := Save(root, "main", "high", cbSlug, expect)
		if err == nil || !strings.Contains(err.Error(), frase) {
			t.Fatalf("CA-344: expect %v distinto de unsynced se niega con %q: %v", expect, frase, err)
		}
		cbSinCambios(t, "expect distinto", root, wt, headRoot, headWt)
	}

	// el mismo conjunto en otro orden guarda
	al := append([]string{}, cbEsperadas...)
	sort.Sort(sort.Reverse(sort.StringSlice(al)))
	res, err := Save(root, "main", "high", cbSlug, al)
	if err != nil {
		t.Fatalf("CA-344: expect igual como conjunto guarda: %v", err)
	}
	if len(res.Commits) != 2 || len(cbUnsynced(t, root)) != 0 {
		t.Fatalf("CA-344: con expect igual se guarda todo: %+v", res)
	}
}

// CA-344: sin nada que guardar no commitea, no falla, y la CLI lo dice.
func TestCA344_SinNadaQueGuardar(t *testing.T) {
	root := cbRepoSave(t)
	icEscribir(t, root, ".hoom/items/"+cbSlug+".yaml", cbItemYAML())
	icGit(t, root, "add", "-A")
	icGit(t, root, "commit", "-q", "-m", "item")
	if err := taskcmd.Start(root, cbSlug, "main"); err != nil {
		t.Fatalf("CA-344: fixture: task start: %v", err)
	}
	wt := filepath.Join(root, ".hoom", "worktrees", cbSlug)
	if got := cbUnsynced(t, root); len(got) != 0 {
		t.Fatalf("CA-344: fixture: la tarjeta no tiene nada sin guardar: %v", got)
	}
	headRoot, headWt := cbHead(t, root), cbHead(t, wt)

	res, err := Save(root, "main", "high", cbSlug, nil)
	if err != nil {
		t.Fatalf("CA-344: sin nada que guardar Save no falla: %v", err)
	}
	if len(res.Commits) != 0 {
		t.Fatalf("CA-344: sin nada que guardar no hay commits: %+v", res.Commits)
	}
	if cbHead(t, root) != headRoot || cbHead(t, wt) != headWt {
		t.Fatal("CA-344: sin nada que guardar no se commitea nada")
	}

	out, _, err := cbExec(t, root, "save", cbSlug)
	if err != nil {
		t.Fatalf("CA-344: hoom item save sin nada que guardar sale con 0: %v", err)
	}
	if !strings.Contains(out, "hoom item save: la tarjeta "+cbSlug+" no tiene nada sin guardar") {
		t.Fatalf("CA-344: la CLI dice que no hay nada sin guardar:\n%s", out)
	}
	if cbHead(t, root) != headRoot || cbHead(t, wt) != headWt {
		t.Fatal("CA-344: la CLI tampoco commitea")
	}
}

// CA-344: Parse entiende `hoom item save <slug> [--json]`, y el texto de la
// CLI dice que guardo y que commit hizo en cada arbol.
func TestCA344_CLITextoDeSave(t *testing.T) {
	req, err := Parse([]string{"save", cbSlug})
	if err != nil {
		t.Fatalf("CA-344: Parse(save %s): %v", cbSlug, err)
	}
	if req.Sub != SubSave || req.Slug != cbSlug || req.JSON {
		t.Fatalf("CA-344: Parse(save) arma el pedido de guardar: %+v", req)
	}
	req, err = Parse([]string{"save", cbSlug, "--json"})
	if err != nil || req.Sub != SubSave || req.Slug != cbSlug || !req.JSON {
		t.Fatalf("CA-344: Parse(save --json): %+v %v", req, err)
	}

	root, wt := cbTareaSave(t)
	out, _, err := cbExec(t, root, "save", cbSlug)
	if err != nil {
		t.Fatalf("CA-344: hoom item save: %v", err)
	}
	if !strings.Contains(out, "hoom item save: tarjeta "+cbSlug+" guardada") {
		t.Fatalf("CA-344: el texto empieza con \"hoom item save: tarjeta %s guardada\":\n%s", cbSlug, out)
	}
	cbLineaDe := func(sha string) string {
		for _, l := range strings.Split(out, "\n") {
			if strings.Contains(l, sha[:12]) {
				return l
			}
		}
		return ""
	}
	lw := cbLineaDe(cbHead(t, wt))
	if !strings.Contains(lw, ".hoom/worktrees/"+cbSlug+": ") || !strings.Contains(lw, "(3 archivos)") {
		t.Fatalf("CA-344: una linea \".hoom/worktrees/%s: <sha12> (3 archivos)\":\n%s", cbSlug, out)
	}
	lr := cbLineaDe(cbHead(t, root))
	if !strings.Contains(lr, ".: ") || !strings.Contains(lr, "(1 archivo)") {
		t.Fatalf("CA-344: una linea \".: <sha12> (1 archivo)\":\n%s", out)
	}
}

// CA-344: --json emite el SaveResult del contrato: slug, message y commits
// con dir ("." para el raiz, ".hoom/worktrees/<slug>" para el worktree), sha
// de 40 y paths relativas a la raiz.
func TestCA344_CLIJSONDeSave(t *testing.T) {
	root, wt := cbTareaSave(t)
	out, _, err := cbExec(t, root, "save", cbSlug, "--json")
	if err != nil {
		t.Fatalf("CA-344: hoom item save --json: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace([]byte(out)), &got); err != nil {
		t.Fatalf("CA-344: --json emite un objeto JSON: %v\n%s", err, out)
	}
	if got["slug"] != cbSlug || got["message"] != "hoom: guardar la tarjeta "+cbSlug {
		t.Fatalf("CA-344: el JSON trae slug y message: %v", got)
	}
	commits, ok := got["commits"].([]any)
	if !ok || len(commits) != 2 {
		t.Fatalf("CA-344: el JSON trae un commit por arbol: %v", got["commits"])
	}
	vistos := map[string]bool{}
	for _, raw := range commits {
		c, _ := raw.(map[string]any)
		dir, _ := c["dir"].(string)
		sha, _ := c["sha"].(string)
		var paths []string
		for _, p := range c["paths"].([]any) {
			paths = append(paths, p.(string))
		}
		vistos[dir] = true
		switch dir {
		case ".":
			if sha != cbHead(t, root) || !cbMismos(paths, cbEsperadas[:1]) {
				t.Fatalf("CA-344: el commit \".\" es el HEAD del raiz con el item: %v", c)
			}
		case ".hoom/worktrees/" + cbSlug:
			if sha != cbHead(t, wt) || !cbMismos(paths, cbEsperadas[1:]) {
				t.Fatalf("CA-344: el commit del worktree es su HEAD con sus rutas relativas a la raiz: %v", c)
			}
		default:
			t.Fatalf("CA-344: dir desconocido en el JSON: %v", c)
		}
		if !cbSHA.MatchString(sha) {
			t.Fatalf("CA-344: sha es el hash completo de 40: %q", sha)
		}
	}
	if !vistos["."] || !vistos[".hoom/worktrees/"+cbSlug] {
		t.Fatalf("CA-344: los dos arboles aparecen en commits: %v", vistos)
	}
}

// CA-344: los hooks del repo corren; uno que falla devuelve su salida, no
// commitea, y las rutas quedan en el indice.
func TestCA344_UnHookQueFallaDevuelveSuSalida(t *testing.T) {
	root := cbRepoSave(t)
	hooks := filepath.Join(root, ".git", "hooks")
	icGit(t, root, "config", "core.hooksPath", hooks)
	icEscribir(t, root, ".git/hooks/pre-commit", "#!/bin/sh\necho 'el hook de la casa dice que no' >&2\nexit 1\n")
	if err := os.Chmod(filepath.Join(hooks, "pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	icEscribir(t, root, ".hoom/items/"+cbSlug+".yaml", cbItemYAML())
	head := cbHead(t, root)

	_, err := Save(root, "main", "high", cbSlug, nil)
	if err == nil || !strings.Contains(err.Error(), "el hook de la casa dice que no") {
		t.Fatalf("CA-344: un hook que falla devuelve su salida: %v", err)
	}
	if cbHead(t, root) != head {
		t.Fatal("CA-344: con el hook en rojo no hay commit")
	}
	if got := cbEnIndice(t, root); !cbMismos(got, []string{".hoom/items/" + cbSlug + ".yaml"}) {
		t.Fatalf("CA-344: las rutas quedan en el indice: %v", got)
	}
}
