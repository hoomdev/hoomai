// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-345, CA-346, CA-348 y CA-350, la parte del Studio): Guardar,
// Descartar, Abrir sesion y Ver terminal como endpoints de la tarjeta.
// Ningun CLI real: el PATH del test tiene solo git y sh de verdad, y un tmux
// y un claude falsos cuando hacen falta, asi que ningun tmux ni CLI de IA de
// la maquina se ve. Los fixtures son los de C2 (tbProyecto, tbItem): un repo
// con identidad git configurada y el .gitignore canonico de .hoom.
package servecmd

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cockpitcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gitx"
)

// La negativa de Guardar y Descartar cuando las rutas no son las de ahora.
const atCambiaron = "los cambios de la tarjeta cambiaron desde que los viste: revisalos de nuevo"

// El texto que tmux "pinto" en el pane de la IA, con sus secuencias de color.
const atCapturaColor = `printf 'hola \033[31mrojo\033[0m fin\n'`

// Una pantalla de 300000 bytes: mas que el tope del espejo (256 KiB).
const atCapturaGrande = `i=0
while [ $i -lt 3000 ]; do
  printf '%s\n' 'xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx'
  i=$((i+1))
done`

// atPATH deja en el PATH solo git y sh de verdad y los binarios falsos que se
// pidan (nombre -> cuerpo del script). Devuelve el directorio, para poder
// sacar o cambiar un falso a mitad del test.
func atPATH(t *testing.T, falsos map[string]string) string {
	t.Helper()
	bin := t.TempDir()
	for _, herramienta := range []string{"git", "sh"} {
		real, err := exec.LookPath(herramienta)
		if err != nil {
			t.Fatalf("el test necesita %s: %v", herramienta, err)
		}
		if err := os.Symlink(real, filepath.Join(bin, herramienta)); err != nil {
			t.Fatal(err)
		}
	}
	for nombre, script := range falsos {
		atFalso(t, bin, nombre, script)
	}
	t.Setenv("PATH", bin)
	return bin
}

func atFalso(t *testing.T, bin, nombre, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(bin, nombre), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// atTmux es un tmux falso que anota cada invocacion (sus argumentos, una
// linea) en log, y simula UNA sesion con el archivo marca: has-session
// responde 1 mientras la marca no existe, new-session la crea, list-panes
// lista %3 y %4, y capture-pane corre captura. Solo usa builtins de sh.
func atTmux(log, marca, captura string) string {
	return strings.NewReplacer("@LOG@", log, "@MARCA@", marca, "@CAPTURA@", captura).Replace(`printf '%s\n' "$*" >> '@LOG@'
case "$1" in
has-session)
  [ -f '@MARCA@' ] && exit 0
  exit 1 ;;
new-session)
  : > '@MARCA@'
  exit 0 ;;
list-panes)
  [ -f '@MARCA@' ] || { printf "can't find session\n" >&2; exit 1; }
  printf '%%3\n%%4\n'
  exit 0 ;;
capture-pane)
  [ -f '@MARCA@' ] || { printf "can't find pane\n" >&2; exit 1; }
@CAPTURA@
  exit 0 ;;
esac
exit 0
`)
}

// atLineas devuelve las invocaciones del tmux falso ([] si no hubo ninguna).
func atLineas(t *testing.T, log string) []string {
	t.Helper()
	raw, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return []string{}
	}
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, l := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}

func atConVerbo(lineas []string, verbo string) []string {
	out := []string{}
	for _, l := range lineas {
		if f := strings.Fields(l); len(f) > 0 && f[0] == verbo {
			out = append(out, l)
		}
	}
	return out
}

// atConEspacio arma una tarjeta con espacio de trabajo propio: el item en el
// arbol raiz (sin commitear) y el worktree .hoom/worktrees/<slug> en la rama
// hoom/<slug>, como lo deja hoom task start.
func atConEspacio(t *testing.T, dir, slug string) string {
	t.Helper()
	tbItem(t, dir, slug, "")
	wt := filepath.Join(dir, ".hoom", "worktrees", slug)
	gitRun(t, dir, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	return wt
}

// atPIDMuerto devuelve el PID de un proceso que ya termino (y fue esperado).
func atPIDMuerto(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("/bin/sh", "-c", "exit 0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.ProcessState.Pid()
}

// atSobre escribe TAL CUAL un registro de sobre abierto en .hoom/envelopes/
// de dir (envelope.Write pisaria updated_at con la hora de ahora).
func atSobre(t *testing.T, dir, id, slug, rol string, pid int, empezo, actualizo time.Time) {
	t.Helper()
	rec := envelope.Record{ID: id, Role: rol, Provider: "claude", Task: slug, Dir: dir,
		Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: empezo, UpdatedAt: actualizo, PID: pid}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tbEscribir(t, dir, ".hoom/envelopes/"+id+".json", string(raw)+"\n")
}

// atInterrumpida deja la tarjeta con un sobre interrumpido: abierto, con el
// PID de un proceso muerto y sin latido desde hace dos horas (interrumpido
// con la regla de C3 y con la de C1).
func atInterrumpida(t *testing.T, dir, slug string) {
	t.Helper()
	now := time.Now().UTC()
	atSobre(t, dir, "20260923T100000_"+atHex(slug), slug, "writer", atPIDMuerto(t), now.Add(-3*time.Hour), now.Add(-2*time.Hour))
}

// atEnCurso deja la tarjeta con un writer en curso: sobre abierto con el PID
// de este proceso y latido fresco. Empezo antes que cualquier registro de
// atInterrumpida, asi que no lo releva.
func atEnCurso(t *testing.T, dir, slug string) string {
	t.Helper()
	now := time.Now().UTC()
	id := "20260923T090000_" + atHex(slug+"-vivo")
	atSobre(t, dir, id, slug, "writer", os.Getpid(), now.Add(-4*time.Hour), now)
	return filepath.Join(dir, ".hoom", "envelopes", id+".json")
}

func atHex(s string) string {
	h := 0
	for _, r := range s {
		h = (h*31 + int(r)) & 0xffffff
	}
	return fmt.Sprintf("%06x", h)
}

// atPedir hace un pedido al Handler; un panico del handler es una falla del
// criterio, no la caida de todo el paquete de tests.
func atPedir(t *testing.T, ca string, s *Server, metodo, path, token, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(metodo, path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set(TokenHeader, token)
	}
	rec := httptest.NewRecorder()
	func() {
		defer func() {
			if p := recover(); p != nil {
				t.Fatalf("%s: %s %s entro en panico: %v", ca, metodo, path, p)
			}
		}()
		s.Handler().ServeHTTP(rec, req)
	}()
	return rec
}

// atOK exige 200 JSON y lo decodifica en into; devuelve las claves del objeto.
func atOK(t *testing.T, ca string, rec *httptest.ResponseRecorder, into any) map[string]json.RawMessage {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: se esperaba 200, fue %d: %s", ca, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%s: la respuesta es JSON (Content-Type %q)", ca, rec.Header().Get("Content-Type"))
	}
	var claves map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &claves); err != nil {
		t.Fatalf("%s: la respuesta es un objeto JSON: %v\n%s", ca, err, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), into); err != nil {
		t.Fatalf("%s: %v\n%s", ca, err, rec.Body.String())
	}
	return claves
}

func atTieneClaves(t *testing.T, ca string, claves map[string]json.RawMessage, quiero ...string) {
	t.Helper()
	for _, k := range quiero {
		if _, ok := claves[k]; !ok {
			t.Fatalf("%s: a la respuesta le falta la clave %q: %v", ca, k, claves)
		}
	}
}

func atCuerpoPaths(paths []string) string {
	raw, _ := json.Marshal(map[string][]string{"paths": paths})
	return string(raw)
}

// atGit corre git en dir y devuelve su salida sin el salto final.
func atGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimRight(string(out), "\n")
}

// atEstadoGit resume lo que un commit cambiaria en cada arbol: HEAD, el
// indice y lo que no esta commiteado.
func atEstadoGit(t *testing.T, dirs ...string) string {
	t.Helper()
	var b strings.Builder
	for _, d := range dirs {
		fmt.Fprintf(&b, "%s\nHEAD %s\nindice:\n%s\nestado:\n%s\n", d,
			atGit(t, d, "rev-parse", "HEAD"),
			atGit(t, d, "diff", "--cached", "--name-only"),
			atGit(t, d, "status", "--porcelain", "--untracked-files=all"))
	}
	return b.String()
}

func atMismoGit(t *testing.T, ca, antes string, dirs ...string) {
	t.Helper()
	if despues := atEstadoGit(t, dirs...); despues != antes {
		t.Fatalf("%s: no se commitea nada ni cambia el indice:\nantes:\n%s\ndespues:\n%s", ca, antes, despues)
	}
}

func atOrdenadas(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

func atMismas(a, b []string) bool {
	return strings.Join(atOrdenadas(a), "\n") == strings.Join(atOrdenadas(b), "\n")
}

// atTarjeta deriva la tarjeta con el codigo de C1 (lo que hoom item show --json dice).
func atTarjeta(t *testing.T, dir, slug string) boardcmd.Card {
	t.Helper()
	c, err := boardcmd.CardFor(dir, "main", tbBlockOn, slug, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// atYaNoEsta es la negativa del contrato cuando la accion ya no esta en la
// tarjeta: su columna (el nombre que da el binario) y su plain.
func atYaNoEsta(t *testing.T, dir, slug string) string {
	t.Helper()
	c := atTarjeta(t, dir, slug)
	return fmt.Sprintf("la tarjeta ya no esta donde la viste: ahora esta en %s (%s)", c.ColumnName, c.Plain)
}

// atFotoHoom fotografia .hoom/ de dir (contenido, modo y mtime), sin entrar
// a .hoom/worktrees/ (los espacios de trabajo se miran aparte).
func atFotoHoom(t *testing.T, dir string) map[string]string {
	t.Helper()
	root := filepath.Join(dir, ".hoom")
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if rel == "worktrees" {
				return filepath.SkipDir
			}
			out[rel+"/"] = "dir"
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = fmt.Sprintf("%v %x %d", info.Mode(), raw, info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func atMismaFotoHoom(t *testing.T, ca, donde string, antes, despues map[string]string) {
	t.Helper()
	for k, v := range antes {
		if w, ok := despues[k]; !ok {
			t.Fatalf("%s: se borro %s bajo .hoom/ de %s: hoom nunca descarta evidencia", ca, k, donde)
		} else if w != v {
			t.Fatalf("%s: se modifico %s bajo .hoom/ de %s: hoom nunca descarta evidencia", ca, k, donde)
		}
	}
	for k := range despues {
		if _, ok := antes[k]; !ok {
			t.Fatalf("%s: aparecio %s bajo .hoom/ de %s", ca, k, donde)
		}
	}
}

// ---------------------------------------------------------------------------
// CA-345: Guardar

// CA-345: POST /api/board/{slug}/save con las rutas de unsynced responde 200
// con el SaveResult {slug, message, commits[{dir, sha, paths}]}: un commit
// por arbol con el mensaje fijo "hoom: guardar la tarjeta <slug>" (en el
// espacio de trabajo sus rutas, en el arbol raiz solo el item), y despues
// unsynced queda vacio. Con otras rutas es 409 con la negativa del contrato,
// sin cuerpo o sin paths es 400, sin token (o con uno equivocado) es 401, y
// con un writer en curso es 409: en ninguno de esos casos hay commit.
func TestCA345_GuardarDesdeLaTarjeta(t *testing.T) {
	atPATH(t, nil)
	dir := tbProyecto(t)
	const slug = "guardable"
	wt := atConEspacio(t, dir, slug)
	tbEscribir(t, wt, "app.go", "package app\n\nfunc Cambio() int { return 2 }\n")
	tbEscribir(t, wt, "nuevo.go", "package app\n\nfunc Nuevo() {}\n")

	enEspacio := []string{".hoom/worktrees/" + slug + "/app.go", ".hoom/worktrees/" + slug + "/nuevo.go"}
	enRaiz := []string{".hoom/items/" + slug + ".yaml"}
	unsynced := atOrdenadas(append(append([]string{}, enEspacio...), enRaiz...))
	if got := atTarjeta(t, dir, slug).Unsynced; !atMismas(got, unsynced) {
		t.Fatalf("CA-345: fixture: unsynced de la tarjeta es %v, se esperaba %v", got, unsynced)
	}

	s := newServer(t, dir)
	ruta := "/api/board/" + slug + "/save"
	antes := atEstadoGit(t, dir, wt)

	casos := []struct {
		nombre, token, body string
		code                int
		msg                 string
	}{
		{"sin token", "", atCuerpoPaths(unsynced), http.StatusUnauthorized, ""},
		{"token equivocado", strings.Repeat("0", len(s.Token())), atCuerpoPaths(unsynced), http.StatusUnauthorized, ""},
		{"sin cuerpo", s.Token(), "", http.StatusBadRequest, ""},
		{"sin paths", s.Token(), `{}`, http.StatusBadRequest, ""},
		{"paths null", s.Token(), `{"paths": null}`, http.StatusBadRequest, ""},
		{"menos rutas", s.Token(), atCuerpoPaths(enRaiz), http.StatusConflict, atCambiaron},
		{"rutas de mas", s.Token(), atCuerpoPaths(append(append([]string{}, unsynced...), ".hoom/worktrees/"+slug+"/otro.go")), http.StatusConflict, atCambiaron},
		{"otras rutas", s.Token(), atCuerpoPaths([]string{"app.go"}), http.StatusConflict, atCambiaron},
	}
	for _, c := range casos {
		ca := "CA-345 (" + c.nombre + ")"
		msg := tbErrorJSON(t, ca, atPedir(t, ca, s, http.MethodPost, ruta, c.token, c.body), c.code)
		if c.msg != "" && !strings.Contains(msg, c.msg) {
			t.Fatalf("%s: el 409 dice %q, dijo %q", ca, c.msg, msg)
		}
		atMismoGit(t, ca, antes, dir, wt)
	}

	// con un writer en curso se niega, sin commitear
	vivo := atEnCurso(t, dir, slug)
	if atTarjeta(t, dir, slug).Running == nil {
		t.Fatal("CA-345: fixture: la tarjeta tiene que estar en curso")
	}
	msg := tbErrorJSON(t, "CA-345 (en curso)", atPedir(t, "CA-345", s, http.MethodPost, ruta, s.Token(), atCuerpoPaths(unsynced)), http.StatusConflict)
	if !strings.Contains(msg, "espera a que termine el writer que esta trabajando") {
		t.Fatalf("CA-345: con running el 409 dice que espere al writer: %q", msg)
	}
	atMismoGit(t, "CA-345 (en curso)", antes, dir, wt)
	if err := os.Remove(vivo); err != nil {
		t.Fatal(err)
	}

	// con las rutas de unsynced: 200 y el SaveResult
	rec := atPedir(t, "CA-345", s, http.MethodPost, ruta, s.Token(), atCuerpoPaths(unsynced))
	var res struct {
		Slug    string `json:"slug"`
		Message string `json:"message"`
		Commits []struct {
			Dir   string   `json:"dir"`
			SHA   string   `json:"sha"`
			Paths []string `json:"paths"`
		} `json:"commits"`
	}
	claves := atOK(t, "CA-345", rec, &res)
	atTieneClaves(t, "CA-345", claves, "slug", "message", "commits")
	mensaje := "hoom: guardar la tarjeta " + slug
	if res.Slug != slug || res.Message != mensaje {
		t.Fatalf("CA-345: el SaveResult trae slug %q y message %q: %s", slug, mensaje, rec.Body.String())
	}
	if len(res.Commits) != 2 {
		t.Fatalf("CA-345: un commit por arbol (el espacio de trabajo y el raiz): %s", rec.Body.String())
	}
	sha40 := regexp.MustCompile(`^[0-9a-f]{40}$`)
	arboles := map[string]struct {
		abs     string
		paths   []string
		archivo []string
	}{
		".hoom/worktrees/" + slug: {wt, enEspacio, []string{"app.go", "nuevo.go"}},
		".":                       {dir, enRaiz, enRaiz},
	}
	vistos := map[string]bool{}
	for _, c := range res.Commits {
		arbol, ok := arboles[c.Dir]
		if !ok || vistos[c.Dir] {
			t.Fatalf("CA-345: commit con dir inesperado o repetido %q (relativo a la raiz): %s", c.Dir, rec.Body.String())
		}
		vistos[c.Dir] = true
		if !sha40.MatchString(c.SHA) {
			t.Fatalf("CA-345: el sha del commit de %s tiene 40 hex: %q", c.Dir, c.SHA)
		}
		if !atMismas(c.Paths, arbol.paths) {
			t.Fatalf("CA-345: el commit de %s lleva %v (relativas a la raiz), llevo %v", c.Dir, arbol.paths, c.Paths)
		}
		if head := atGit(t, arbol.abs, "rev-parse", "HEAD"); head != c.SHA {
			t.Fatalf("CA-345: el sha de %s es el HEAD de ese arbol: %s vs %s", c.Dir, c.SHA, head)
		}
		if asunto := atGit(t, arbol.abs, "log", "-1", "--format=%s"); asunto != mensaje {
			t.Fatalf("CA-345: el commit de %s tiene el mensaje fijo %q, tiene %q", c.Dir, mensaje, asunto)
		}
		if autor := atGit(t, arbol.abs, "log", "-1", "--format=%an <%ae>"); autor != "hoom test <test@hoom.dev>" {
			t.Fatalf("CA-345: el commit de %s usa la identidad git configurada: %q", c.Dir, autor)
		}
		archivos := strings.Fields(atGit(t, arbol.abs, "show", "--name-only", "--format=", c.SHA))
		if !atMismas(archivos, arbol.archivo) {
			t.Fatalf("CA-345: el commit de %s contiene exactamente %v, contiene %v", c.Dir, arbol.archivo, archivos)
		}
	}
	if u := atTarjeta(t, dir, slug).Unsynced; len(u) != 0 {
		t.Fatalf("CA-345: despues de guardar, unsynced esta vacio: %v", u)
	}
}

// ---------------------------------------------------------------------------
// CA-346: Descartar (el endpoint)

// CA-346: POST /api/board/{slug}/discard sobre una tarjeta interrumpida (sobre
// abierto con el PID de un proceso muerto en .hoom/envelopes/ de la raiz) con
// las rutas descartables responde {slug, restored, removed}: la ruta que HEAD
// tiene vuelve a HEAD, la nueva se borra, y nada bajo .hoom/ (ni el
// veredicto sin commitear del espacio de trabajo, ni el registro del sobre,
// ni el item) cambia. Con otras rutas es 409 con la negativa de Guardar, sin
// token es 401, y sin cuerpo no descarta nada.
func TestCA346_DescartarDesdeLaTarjeta(t *testing.T) {
	atPATH(t, nil)
	dir := tbProyecto(t)
	const slug = "cortada"
	wt := atConEspacio(t, dir, slug)
	atInterrumpida(t, dir, slug)
	tbEscribir(t, wt, "app.go", "package app\n\n// a medias\nfunc AMedias() {}\n")
	tbEscribir(t, wt, "nuevo.go", "package app\n\nfunc Nuevo() {}\n")
	tbEscribir(t, wt, "sub/hondo.go", "package sub\n")
	v := tbVeredicto(t, wt, slug) // evidencia sin commitear: nunca se descarta
	veredicto := filepath.Join(wt, ".hoom", "verdicts", v.ID+".json")
	if _, err := os.Stat(veredicto); err != nil {
		t.Fatalf("CA-346: fixture: el veredicto sin commitear esta en el espacio de trabajo: %v", err)
	}
	c := atTarjeta(t, dir, slug)
	if c.Interrupted == nil || c.Running != nil {
		t.Fatalf("CA-346: fixture: la tarjeta esta interrumpida y nadie trabaja: %+v %+v", c.Interrupted, c.Running)
	}
	pre := ".hoom/worktrees/" + slug + "/"
	descartables := []string{pre + "app.go", pre + "nuevo.go", pre + "sub/hondo.go"}

	s := newServer(t, dir)
	ruta := "/api/board/" + slug + "/discard"
	antes := tbFoto(t, dir)

	for _, caso := range []struct {
		nombre, token, body string
		code                int
	}{
		{"sin token", "", atCuerpoPaths(descartables), http.StatusUnauthorized},
		{"token equivocado", strings.Repeat("0", len(s.Token())), atCuerpoPaths(descartables), http.StatusUnauthorized},
		{"menos rutas", s.Token(), atCuerpoPaths(descartables[:1]), http.StatusConflict},
		{"con la evidencia", s.Token(), atCuerpoPaths(append(append([]string{}, descartables...), pre+".hoom/verdicts/"+v.ID+".json")), http.StatusConflict},
		{"rutas de mas", s.Token(), atCuerpoPaths(append(append([]string{}, descartables...), pre+"otro.go")), http.StatusConflict},
	} {
		ca := "CA-346 (" + caso.nombre + ")"
		msg := tbErrorJSON(t, ca, atPedir(t, ca, s, http.MethodPost, ruta, caso.token, caso.body), caso.code)
		if caso.code == http.StatusConflict && !strings.Contains(msg, atCambiaron) {
			t.Fatalf("%s: con otras rutas, la misma negativa que Guardar (%q): %q", ca, atCambiaron, msg)
		}
		tbMismaFoto(t, ca, antes, tbFoto(t, dir))
	}
	for _, body := range []string{"", `{}`} {
		rec := atPedir(t, "CA-346", s, http.MethodPost, ruta, s.Token(), body)
		if rec.Code == http.StatusOK || rec.Code == http.StatusAccepted {
			t.Fatalf("CA-346: sin las rutas que la persona vio no se descarta nada (cuerpo %q): %d %s", body, rec.Code, rec.Body.String())
		}
		tbMismaFoto(t, "CA-346 (sin rutas)", antes, tbFoto(t, dir))
	}

	// con las rutas descartables: vuelve a HEAD y borra lo nuevo, sin tocar .hoom/
	hoomRaiz, hoomEspacio := atFotoHoom(t, dir), atFotoHoom(t, wt)
	rec := atPedir(t, "CA-346", s, http.MethodPost, ruta, s.Token(), atCuerpoPaths(descartables))
	var res struct {
		Slug     string   `json:"slug"`
		Restored []string `json:"restored"`
		Removed  []string `json:"removed"`
	}
	claves := atOK(t, "CA-346", rec, &res)
	atTieneClaves(t, "CA-346", claves, "slug", "restored", "removed")
	if res.Slug != slug {
		t.Fatalf("CA-346: el resultado nombra la tarjeta: %s", rec.Body.String())
	}
	if !atMismas(res.Restored, []string{pre + "app.go"}) {
		t.Fatalf("CA-346: restored es la ruta que HEAD tiene, relativa a la raiz: %v", res.Restored)
	}
	if !atMismas(res.Removed, []string{pre + "nuevo.go", pre + "sub/hondo.go"}) {
		t.Fatalf("CA-346: removed son las rutas que HEAD no tiene, relativas a la raiz: %v", res.Removed)
	}
	if raw, err := os.ReadFile(filepath.Join(wt, "app.go")); err != nil || string(raw) != "package app\n" {
		t.Fatalf("CA-346: app.go vuelve a como esta en HEAD: %v %q", err, raw)
	}
	for _, borrado := range []string{"nuevo.go", "sub/hondo.go"} {
		if _, err := os.Stat(filepath.Join(wt, borrado)); !os.IsNotExist(err) {
			t.Fatalf("CA-346: %s, que HEAD no tiene, se borra: %v", borrado, err)
		}
	}
	atMismaFotoHoom(t, "CA-346", "la raiz", hoomRaiz, atFotoHoom(t, dir))
	atMismaFotoHoom(t, "CA-346", "el espacio de trabajo", hoomEspacio, atFotoHoom(t, wt))
	if st := atGit(t, wt, "status", "--porcelain", "--untracked-files=all"); st != "?? .hoom/verdicts/"+v.ID+".json" {
		t.Fatalf("CA-346: en el espacio de trabajo solo queda sin guardar la evidencia: %q", st)
	}
}

// CA-346: el endpoint exige la accion descartar HABILITADA en la tarjeta,
// derivada de nuevo antes de actuar. Sin sobre interrumpido la tarjeta no la
// tiene (409 "la tarjeta ya no esta donde la viste: ..."); con un writer en
// curso la tiene deshabilitada (409 con su why). En los dos casos no se toca
// nada.
func TestCA346_DescartarExigeLaAccionHabilitada(t *testing.T) {
	atPATH(t, nil)
	dir := tbProyecto(t)

	// sin interrumpido: cambios sin guardar, pero nada que descartar desde la tarjeta
	wtSana := atConEspacio(t, dir, "sana")
	tbEscribir(t, wtSana, "app.go", "package app\n\n// cambio de la persona\n")
	if c := atTarjeta(t, dir, "sana"); c.Interrupted != nil {
		t.Fatalf("CA-346: fixture: sana no esta interrumpida: %+v", c.Interrupted)
	}

	// interrumpida, pero con un writer vivo en la misma tarjeta
	wtOcupada := atConEspacio(t, dir, "ocupada")
	tbEscribir(t, wtOcupada, "app.go", "package app\n\n// a medias\n")
	atInterrumpida(t, dir, "ocupada")
	atEnCurso(t, dir, "ocupada")
	if c := atTarjeta(t, dir, "ocupada"); c.Interrupted == nil || c.Running == nil {
		t.Fatalf("CA-346: fixture: ocupada esta interrumpida y en curso: %+v %+v", c.Interrupted, c.Running)
	}

	s := newServer(t, dir)
	antes := tbFoto(t, dir)

	quiere := atYaNoEsta(t, dir, "sana")
	msg := tbErrorJSON(t, "CA-346 (sin interrumpido)", atPedir(t, "CA-346", s, http.MethodPost, "/api/board/sana/discard", s.Token(),
		atCuerpoPaths([]string{".hoom/worktrees/sana/app.go"})), http.StatusConflict)
	if msg != quiere {
		t.Fatalf("CA-346: sin la accion descartar en la tarjeta, 409 con %q; fue %q", quiere, msg)
	}
	tbMismaFoto(t, "CA-346 (sin interrumpido)", antes, tbFoto(t, dir))

	msg = tbErrorJSON(t, "CA-346 (en curso)", atPedir(t, "CA-346", s, http.MethodPost, "/api/board/ocupada/discard", s.Token(),
		atCuerpoPaths([]string{".hoom/worktrees/ocupada/app.go"})), http.StatusConflict)
	if msg != "espera a que termine el writer que esta trabajando" {
		t.Fatalf("CA-346: con la accion deshabilitada, 409 con su why; fue %q", msg)
	}
	tbMismaFoto(t, "CA-346 (en curso)", antes, tbFoto(t, dir))

	// y la tarjeta que no existe es 404, un slug invalido 400
	tbErrorJSON(t, "CA-346 (no existe)", atPedir(t, "CA-346", s, http.MethodPost, "/api/board/no-existe/discard", s.Token(), atCuerpoPaths([]string{"x"})), http.StatusNotFound)
	tbErrorJSON(t, "CA-346 (slug invalido)", atPedir(t, "CA-346", s, http.MethodPost, "/api/board/Mal_Slug/discard", s.Token(), atCuerpoPaths([]string{"x"})), http.StatusBadRequest)
	tbMismaFoto(t, "CA-346", antes, tbFoto(t, dir))
}

// ---------------------------------------------------------------------------
// CA-348: Abrir sesion (el endpoint)

type atSesion struct {
	Session  string `json:"session"`
	Created  bool   `json:"created"`
	Provider string `json:"provider"`
	Dir      string `json:"dir"`
	Attach   string `json:"attach"`
}

type atItemSesiones struct {
	Titulo   string `yaml:"titulo"`
	Sesiones []struct {
		Provider   string    `yaml:"provider"`
		AbiertaPor string    `yaml:"abierta_por"`
		AbiertaEn  time.Time `yaml:"abierta_en"`
	} `yaml:"sesiones"`
}

func atLeerItem(t *testing.T, dir, slug string) (atItemSesiones, []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", "items", slug+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var it atItemSesiones
	if err := yaml.Unmarshal(raw, &it); err != nil {
		t.Fatalf("el item %s es yaml: %v\n%s", slug, err, raw)
	}
	return it, raw
}

// CA-348: POST /api/board/{slug}/session con {"provider": "claude"} sobre una
// tarjeta con espacio de trabajo arma la sesion de tmux del cockpit SIN
// adjuntarse y responde {session, created, provider, dir, attach}. El item
// del arbol raiz gana una entrada en sesiones (provider, abierta_por =
// gitx.Identity, abierta_en). La segunda vez responde created false y el
// item no cambia (una sola entrada).
func TestCA348_AbrirSesionDesdeLaTarjeta(t *testing.T) {
	marcas := t.TempDir()
	log, marca := filepath.Join(marcas, "tmux.log"), filepath.Join(marcas, "sesion")
	atPATH(t, map[string]string{"tmux": atTmux(log, marca, atCapturaColor), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	const slug = "con-sesion"
	wt := atConEspacio(t, dir, slug)
	s := newServer(t, dir)

	desde := time.Now().UTC().Add(-time.Minute)
	rec := atPedir(t, "CA-348", s, http.MethodPost, "/api/board/"+slug+"/session", s.Token(), `{"provider": "claude"}`)
	var ses atSesion
	claves := atOK(t, "CA-348", rec, &ses)
	atTieneClaves(t, "CA-348", claves, "session", "created", "provider", "dir", "attach")
	nombre := "hoom-demo-" + slug
	quiero := atSesion{Session: nombre, Created: true, Provider: "claude", Dir: ".hoom/worktrees/" + slug, Attach: "tmux attach -t " + nombre}
	if ses != quiero {
		t.Fatalf("CA-348: la sesion creada es %+v, fue %+v", quiero, ses)
	}

	lineas := atLineas(t, log)
	nuevas := atConVerbo(lineas, "new-session")
	if len(nuevas) != 1 || !strings.Contains(nuevas[0], wt) {
		t.Fatalf("CA-348: se crea UNA sesion de tmux en el espacio de trabajo (%s): %q", wt, lineas)
	}
	for _, prohibido := range []string{"attach-session", "attach", "a", "switch-client"} {
		if l := atConVerbo(lineas, prohibido); len(l) > 0 {
			t.Fatalf("CA-348: desde el Studio la sesion se arma sin adjuntarse (%s): %q", prohibido, l)
		}
	}

	it, crudo := atLeerItem(t, dir, slug)
	if it.Titulo != "Tarjeta "+slug {
		t.Fatalf("CA-348: agregar la sesion no toca las demas claves del item:\n%s", crudo)
	}
	if len(it.Sesiones) != 1 {
		t.Fatalf("CA-348: el item del arbol raiz gana UNA entrada en sesiones:\n%s", crudo)
	}
	e := it.Sesiones[0]
	if e.Provider != "claude" || e.AbiertaPor != gitx.Identity(dir) || e.AbiertaPor != "hoom test <test@hoom.dev>" {
		t.Fatalf("CA-348: la entrada tiene provider claude y abierta_por la identidad git de la raiz: %+v\n%s", e, crudo)
	}
	if e.AbiertaEn.Before(desde) || e.AbiertaEn.After(time.Now().UTC().Add(time.Minute)) {
		t.Fatalf("CA-348: abierta_en es el momento en que se abrio: %v\n%s", e.AbiertaEn, crudo)
	}

	// la segunda vez: ya existe, no se crea ni se registra nada
	rec = atPedir(t, "CA-348", s, http.MethodPost, "/api/board/"+slug+"/session", s.Token(), `{"provider": "claude"}`)
	var otra atSesion
	atOK(t, "CA-348 (segunda)", rec, &otra)
	quiero.Created = false
	if otra != quiero {
		t.Fatalf("CA-348: abrir de nuevo responde created false: %+v", otra)
	}
	if n := len(atConVerbo(atLineas(t, log), "new-session")); n != 1 {
		t.Fatalf("CA-348: con la sesion existente no se crea otra (%d new-session)", n)
	}
	if _, despues := atLeerItem(t, dir, slug); string(despues) != string(crudo) {
		t.Fatalf("CA-348: con la sesion existente el item no cambia:\nantes:\n%s\ndespues:\n%s", crudo, despues)
	}
}

// CA-348: sin espacio de trabajo propio es 409 (la tarjeta no tiene la
// accion sesion); sin provider (o sin cuerpo) es 400; sin token es 401; y un
// provider que no esta instalado es 409 con el mensaje de cockpitcmd. En
// ninguno se crea una sesion ni cambia el item.
func TestCA348_AbrirSesionNegativas(t *testing.T) {
	marcas := t.TempDir()
	log, marca := filepath.Join(marcas, "tmux.log"), filepath.Join(marcas, "sesion")
	atPATH(t, map[string]string{"tmux": atTmux(log, marca, atCapturaColor), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	const slug = "sin-sesion"
	atConEspacio(t, dir, slug)
	tbItem(t, dir, "sin-espacio", "")
	s := newServer(t, dir)
	antes := tbFoto(t, dir)

	sinCambios := func(ca string) {
		t.Helper()
		if n := atConVerbo(atLineas(t, log), "new-session"); len(n) > 0 {
			t.Fatalf("%s: no se crea ninguna sesion: %q", ca, n)
		}
		tbMismaFoto(t, ca, antes, tbFoto(t, dir))
	}

	quiere := atYaNoEsta(t, dir, "sin-espacio")
	msg := tbErrorJSON(t, "CA-348 (sin espacio de trabajo)", atPedir(t, "CA-348", s, http.MethodPost, "/api/board/sin-espacio/session", s.Token(), `{"provider": "claude"}`), http.StatusConflict)
	if msg != quiere {
		t.Fatalf("CA-348: sin espacio de trabajo la tarjeta no tiene sesion: 409 con %q; fue %q", quiere, msg)
	}
	sinCambios("CA-348 (sin espacio de trabajo)")

	for _, caso := range []struct {
		nombre, token, body string
		code                int
	}{
		{"sin token", "", `{"provider": "claude"}`, http.StatusUnauthorized},
		{"token equivocado", strings.Repeat("0", len(s.Token())), `{"provider": "claude"}`, http.StatusUnauthorized},
		{"sin cuerpo", s.Token(), "", http.StatusBadRequest},
		{"sin provider", s.Token(), `{}`, http.StatusBadRequest},
		{"provider vacio", s.Token(), `{"provider": ""}`, http.StatusBadRequest},
	} {
		ca := "CA-348 (" + caso.nombre + ")"
		tbErrorJSON(t, ca, atPedir(t, ca, s, http.MethodPost, "/api/board/"+slug+"/session", caso.token, caso.body), caso.code)
		sinCambios(ca)
	}

	// provider no instalado: el mensaje de cockpitcmd
	msg = tbErrorJSON(t, "CA-348 (provider no instalado)", atPedir(t, "CA-348", s, http.MethodPost, "/api/board/"+slug+"/session", s.Token(), `{"provider": "codex"}`), http.StatusConflict)
	_, errCockpit := cockpitcmd.Open(dir, "demo", cockpitcmd.Options{Provider: "codex", Task: slug}, s.cockpit)
	if !strings.Contains(msg, "codex") || errCockpit == nil || msg != errCockpit.Error() {
		t.Fatalf("CA-348: sin el provider instalado, 409 con el mensaje de cockpitcmd (%v); fue %q", errCockpit, msg)
	}
	sinCambios("CA-348 (provider no instalado)")
}

// CA-348: sin tmux es 409 con el mensaje de cockpitcmd, y no se escribe nada.
func TestCA348_AbrirSesionSinTmux(t *testing.T) {
	atPATH(t, map[string]string{"claude": "exit 0\n"})
	dir := tbProyecto(t)
	const slug = "sin-tmux"
	atConEspacio(t, dir, slug)
	s := newServer(t, dir)
	antes := tbFoto(t, dir)

	msg := tbErrorJSON(t, "CA-348 (sin tmux)", atPedir(t, "CA-348", s, http.MethodPost, "/api/board/"+slug+"/session", s.Token(), `{"provider": "claude"}`), http.StatusConflict)
	var validos []string
	for _, mux := range []string{"", "tmux"} {
		if _, err := cockpitcmd.Open(dir, "demo", cockpitcmd.Options{Provider: "claude", Task: slug, Mux: mux}, s.cockpit); err != nil {
			validos = append(validos, err.Error())
		}
	}
	coincide := false
	for _, v := range validos {
		coincide = coincide || v == msg
	}
	if !strings.Contains(msg, "tmux") || !coincide {
		t.Fatalf("CA-348: sin tmux, 409 con el mensaje de cockpitcmd (%q); fue %q", validos, msg)
	}
	tbMismaFoto(t, "CA-348 (sin tmux)", antes, tbFoto(t, dir))
}

// ---------------------------------------------------------------------------
// CA-350: Ver terminal (el endpoint)

type atTerminal struct {
	Available bool   `json:"available"`
	Session   string `json:"session"`
	Pane      string `json:"pane"`
	Text      string `json:"text"`
	Note      string `json:"note"`
}

// CA-350: GET /api/board/{slug}/terminal, sin token, responde el text de
// tmux capture-pane -p -e del PRIMER pane que lista list-panes -t =<sesion>
// (%3), con sus secuencias de color, cortado en 256 KiB. Sin sesion o sin
// tmux responde 200 con available false y la nota del contrato. Otro metodo
// es 405, un slug invalido 400, una tarjeta inexistente 404, y leer deja el
// repo y .hoom/ byte a byte iguales.
func TestCA350_VerTerminalEsUnaLectura(t *testing.T) {
	marcas := t.TempDir()
	log, marca := filepath.Join(marcas, "tmux.log"), filepath.Join(marcas, "sesion")
	bin := atPATH(t, map[string]string{"tmux": atTmux(log, marca, atCapturaColor), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	const slug = "espejo"
	atConEspacio(t, dir, slug)
	tbItem(t, dir, "sin-espacio", "")
	s := newServer(t, dir)
	ruta := "/api/board/" + slug + "/terminal"
	nombre := "hoom-demo-" + slug
	if err := os.WriteFile(marca, nil, 0o644); err != nil { // la sesion existe
		t.Fatal(err)
	}
	antes := tbFoto(t, dir)

	// con la sesion: el primer pane y su pantalla, con colores
	var term atTerminal
	claves := atOK(t, "CA-350", atPedir(t, "CA-350", s, http.MethodGet, ruta, "", ""), &term)
	atTieneClaves(t, "CA-350", claves, "available", "session", "pane", "text", "note")
	if !term.Available || term.Session != nombre || term.Pane != "%3" || term.Note != "" {
		t.Fatalf("CA-350: con la sesion abierta: available, session %s, pane %%3 y sin nota: %+v", nombre, term)
	}
	if !strings.Contains(term.Text, "hola \x1b[31mrojo\x1b[0m fin") {
		t.Fatalf("CA-350: text es la pantalla con sus secuencias de color: %q", term.Text)
	}
	lineas := atLineas(t, log)
	panes := atConVerbo(lineas, "list-panes")
	if len(panes) == 0 || !strings.Contains(panes[0], "="+nombre) {
		t.Fatalf("CA-350: el pane sale de tmux list-panes -t =%s: %q", nombre, lineas)
	}
	capturas := atConVerbo(lineas, "capture-pane")
	if len(capturas) == 0 {
		t.Fatalf("CA-350: la pantalla sale de tmux capture-pane: %q", lineas)
	}
	campos := map[string]bool{}
	for _, f := range strings.Fields(capturas[len(capturas)-1]) {
		campos[f] = true
	}
	for _, quiero := range []string{"-p", "-e", "%3"} {
		if !campos[quiero] {
			t.Fatalf("CA-350: capture-pane -p -e -t %%3 (el primer pane): %q", capturas)
		}
	}
	for _, prohibido := range []string{"send-keys", "new-session", "kill-session", "split-window"} {
		if l := atConVerbo(lineas, prohibido); len(l) > 0 {
			t.Fatalf("CA-350: ver la terminal no escribe ni crea nada (%s): %q", prohibido, l)
		}
	}

	// una pantalla de mas de 256 KiB se corta
	atFalso(t, bin, "tmux", atTmux(log, marca, atCapturaGrande))
	term = atTerminal{}
	atOK(t, "CA-350 (grande)", atPedir(t, "CA-350", s, http.MethodGet, ruta, "", ""), &term)
	if !term.Available || len(term.Text) == 0 || len(term.Text) > 256<<10 || len(term.Text) > cockpitcmd.TerminalMaxBytes {
		t.Fatalf("CA-350: text se corta en 256 KiB: available=%v, %d bytes", term.Available, len(term.Text))
	}

	// sin la sesion: 200 con available false y la nota
	if err := os.Remove(marca); err != nil {
		t.Fatal(err)
	}
	term = atTerminal{}
	atOK(t, "CA-350 (sin sesion)", atPedir(t, "CA-350", s, http.MethodGet, ruta, "", ""), &term)
	if term.Available || term.Text != "" || term.Note != "no hay una sesion abierta para esta tarjeta" {
		t.Fatalf("CA-350: sin sesion, available false, sin text y la nota del contrato: %+v", term)
	}
	if l := atConVerbo(atLineas(t, log), "new-session"); len(l) > 0 {
		t.Fatalf("CA-350: ver la terminal nunca abre la sesion: %q", l)
	}

	// sin tmux: 200 con available false y la nota
	if err := os.Remove(filepath.Join(bin, "tmux")); err != nil {
		t.Fatal(err)
	}
	term = atTerminal{}
	atOK(t, "CA-350 (sin tmux)", atPedir(t, "CA-350", s, http.MethodGet, ruta, "", ""), &term)
	if term.Available || term.Text != "" || term.Note != "tmux no esta instalado" {
		t.Fatalf("CA-350: sin tmux, available false, sin text y la nota del contrato: %+v", term)
	}

	// otro metodo es 405, con token o sin el
	for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, token := range []string{"", s.Token()} {
			if rec := atPedir(t, "CA-350", s, metodo, ruta, token, `{"keys":"ls\n"}`); rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("CA-350: %s %s (token=%v) es 405, fue %d: %s", metodo, ruta, token != "", rec.Code, rec.Body.String())
			}
		}
	}

	// slug invalido 400, tarjeta inexistente 404
	tbErrorJSON(t, "CA-350 (slug invalido)", atPedir(t, "CA-350", s, http.MethodGet, "/api/board/Mal_Slug/terminal", "", ""), http.StatusBadRequest)
	tbErrorJSON(t, "CA-350 (no existe)", atPedir(t, "CA-350", s, http.MethodGet, "/api/board/no-existe/terminal", "", ""), http.StatusNotFound)

	// leer no escribio nada: repo, espacios de trabajo y .hoom/ byte a byte iguales
	tbMismaFoto(t, "CA-350", antes, tbFoto(t, dir))
}
