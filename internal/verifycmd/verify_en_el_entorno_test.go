// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre
// verifycmd.Run, la funcion que corren `hoom verify`, el sobre de `hoom
// agent`, el cerrar-hallazgos del piloto y POST /api/verify:
//
//   - CA-760: un 127 deja el gate en error y el veredicto rojo;
//   - CA-762: se lee el .hoom/local.yaml del principal, tambien desde la
//     tarea; uno dentro del worktree no se lee;
//   - CA-763: un .hoom/local.yaml invalido es el error de Run, exacto, sin
//     gate corrido, sin veredicto y sin eventos vivos; el error de uso de
//     --gate va antes (aclaracion 5);
//   - CA-764 y CA-765: con y sin Sobre, los gates corren con la plantilla,
//     con {rel} .hoom/worktrees/<slug> y {dir} el arbol de la tarea;
//   - CA-766: Verdict.Entorno y el Entorno de cada gate, tambien los de
//     hoom; sin ejecutar, el JSON es el de hoy, igual byte a byte salvo id,
//     fecha y duraciones; la huella no cambia;
//   - CA-767: con Sobre y la seccion encierro, la nota de sin encierro o la
//     negativa de exigir; sin Sobre, nada;
//   - CA-773: el gate entorno_local cuando un gate crea, cambia o borra el
//     .hoom/local.yaml del principal, con y sin Sobre, con y sin ejecutar, y
//     con --gate; sin tocarlo (o reescribiendolo igual), no hay tal gate.
//   - CA-771 (enmienda, ronda 0b): un .hoom/local.yaml con solo
//     worktrees_relativos: true deja el verify como hoy, aun con un git que
//     dice 2.47: con veredicto y sin la clave entorno.
//
// Prefijo falso: un `ddev` primero en el PATH que anota sus argumentos fuera
// del proyecto. Escritos desde el spec, las aclaraciones de la ronda 0 y el
// go doc, sin leer la implementacion.
package verifycmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- fixtures

const veePlantilla = "ddev exec -d /var/www/html/{rel} sh -c {cmd}"

func veeSaltarSinSh(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("el prefijo falso es un script de sh")
	}
}

func veeQ(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// veeFalso pone primero en el PATH un `ddev` falso que anota en una carpeta
// fuera del proyecto su directorio y sus argumentos (separados por NUL) y
// corre `sh -c` con el ultimo, con VEE_DENTRO=1. Devuelve esa carpeta.
func veeFalso(t *testing.T) string {
	t.Helper()
	bin, log := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\n" +
		"log=" + veeQ(log) + "\n" +
		"{ printf '%s\\000' \"$(pwd -P)\"; for a do printf '%s\\000' \"$a\"; done; } > \"$log/.tmp.$$\"\n" +
		"mv \"$log/.tmp.$$\" \"$log/llamada.$$\"\n" +
		"for ultimo do :; done\n" +
		"VEE_DENTRO=1\nexport VEE_DENTRO\n" +
		"exec sh -c \"$ultimo\"\n"
	if err := os.WriteFile(filepath.Join(bin, "ddev"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// veeLlamadas son las invocaciones del ddev falso: [directorio, args...].
func veeLlamadas(t *testing.T, log string) [][]string {
	t.Helper()
	ents, err := os.ReadDir(log)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "llamada.") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(log, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00"))
	}
	sort.Slice(out, func(i, j int) bool { return strings.Join(out[i], " ") < strings.Join(out[j], " ") })
	return out
}

// veeGate es el bloque YAML de un gate requerido.
func veeGate(nombre, cmd string) string {
	return "  " + nombre + ":\n    required: true\n    cmd: " + strconv.Quote(cmd) + "\n"
}

// veeGitignore es el .hoom/.gitignore canonico con local.yaml.
func veeGitignore() string {
	body := hoomfs.GitignoreBody()
	for _, l := range strings.Split(body, "\n") {
		if strings.TrimSpace(l) == "local.yaml" {
			return body
		}
	}
	return body + "local.yaml\n"
}

// veeProyecto es un repo git (rama main) con hoom.yaml (gatesYAML y extra)
// y el .hoom/.gitignore canonico, todo commiteado. Con tarea, ademas el
// worktree de la tarea demo como lo deja `hoom task start`, y el manifest es
// el del worktree; sin tarea, el del principal.
func veeProyecto(t *testing.T, gatesYAML, extra string, tarea bool) (root, wt string, m *manifest.Manifest) {
	t.Helper()
	cdwHome(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cdwGit(t, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	cdwGit(t, root, "config", "user.email", "test@hoom.dev")
	cdwGit(t, root, "config", "user.name", "hoom test")
	cdwEscribir(t, filepath.Join(root, manifest.FileName), "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+gatesYAML+extra)
	cdwEscribir(t, filepath.Join(root, ".hoom", ".gitignore"), veeGitignore())
	cdwEscribir(t, filepath.Join(root, "app.go"), "package app\n")
	cdwGit(t, root, "add", "-A")
	cdwGit(t, root, "commit", "-q", "-m", "inicial")
	dir := root
	if tarea {
		wt = filepath.Join(root, ".hoom", "worktrees", cdwTarea)
		cdwGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+cdwTarea, wt, "main")
		dir = wt
	}
	m, err = manifest.Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return root, wt, m
}

// veeSinGit es un proyecto fuera de git.
func veeSinGit(t *testing.T, gatesYAML string) *manifest.Manifest {
	t.Helper()
	cdwHome(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cdwEscribir(t, filepath.Join(dir, manifest.FileName), "schema: hoom/v1\nproject: demo\ngates:\n"+gatesYAML)
	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func veeLocal(t *testing.T, principal, body string) {
	t.Helper()
	cdwEscribir(t, filepath.Join(principal, filepath.FromSlash(entorno.Ruta)), body)
}

func veeSha(body string) string {
	s := sha256.Sum256([]byte(body))
	return hex.EncodeToString(s[:])
}

func veeRun(t *testing.T, ca string, m *manifest.Manifest, opt Options) (*verdict.Verdict, string) {
	t.Helper()
	v, path, err := Run(m, opt)
	if err != nil {
		t.Fatalf("%s: verify corre: %v", ca, err)
	}
	return v, path
}

func veeNombres(v *verdict.Verdict) []string {
	var out []string
	for _, g := range v.Gates {
		out = append(out, g.Name)
	}
	sort.Strings(out)
	return out
}

// veeVeredictos cuenta los veredictos escritos en dir.
func veeVeredictos(t *testing.T, dir string) int {
	t.Helper()
	ents, err := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range ents {
		if strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

// veeSinRastro exige que no haya veredicto ni eventos vivos en dirs, y que
// el gate marcador (touch vee-corrio) no haya corrido.
func veeSinRastro(t *testing.T, ca, caso string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if n := veeVeredictos(t, d); n != 0 {
			t.Fatalf("%s: %s: no se escribe veredicto: %d en %s", ca, caso, n, d)
		}
		if _, err := os.Stat(filepath.Join(d, ".hoom", "cache", live.FileName)); err == nil {
			t.Fatalf("%s: %s: no hay eventos vivos (%s)", ca, caso, d)
		}
		if _, err := os.Stat(filepath.Join(d, "vee-corrio")); err == nil {
			t.Fatalf("%s: %s: ningun gate corre", ca, caso)
		}
	}
}

// veeGateEntornoLocal es el gate entorno_local del veredicto, o nil.
func veeGateEntornoLocal(v *verdict.Verdict) *verdict.GateResult {
	return gateNamed(v, GateEntornoLocal)
}

// veeExigeEntornoLocal exige el gate de CA-773 tal como lo fija el spec y
// la aclaracion 16, y el veredicto rojo.
func veeExigeEntornoLocal(t *testing.T, ca, caso string, v *verdict.Verdict, dir, entornoWant string) {
	t.Helper()
	g := veeGateEntornoLocal(v)
	if g == nil {
		t.Fatalf("%s: %s: el veredicto lleva el gate %s: %v", ca, caso, GateEntornoLocal, veeNombres(v))
	}
	if !g.Required || g.Status != verdict.StatusFail || g.Scope != "full" || g.ExitCode != 1 ||
		g.Notes != NotaEntornoLocal || g.OutputTail != entorno.Ruta || g.Entorno != entornoWant {
		t.Fatalf("%s: %s: %s requerido, fail, full, exit 1, nota %q, salida %q, entorno %q: %+v", ca, caso, GateEntornoLocal, NotaEntornoLocal, entorno.Ruta, entornoWant, g)
	}
	if v.Verdict != "red" {
		t.Fatalf("%s: %s: el veredicto queda rojo: %s", ca, caso, v.Verdict)
	}
	if dir != "" && cdwUltimoCompletoVerde(t, dir) {
		t.Fatalf("%s: %s: verdict.LatestComplete no lo da como verde", ca, caso)
	}
}

// ---------------------------------------------------------------- CA-760

// CA-760: un gate requerido cuyo comando no esta queda en error con 127 y la
// nota de NotaNoEncontrado; Summary.Error lo cuenta y el veredicto es rojo.
func TestCA760_ComandoQueNoEstaDejaRojo(t *testing.T) {
	const ca = "CA-760"
	veeSaltarSinSh(t)
	_, _, m := veeProyecto(t, veeGate("test", "vee-no-existe-760 test")+veeGate("build", "true"), "", false)
	v, _ := veeRun(t, ca, m, Options{Full: true})
	g := gateNamed(v, "test")
	if g == nil || g.Status != verdict.StatusError || g.ExitCode != 127 ||
		!strings.HasPrefix(g.Notes, gates.NotaNoEncontrado+": ") || !strings.Contains(g.Notes, "vee-no-existe-760") {
		t.Fatalf("%s: error, 127, nota %q y el comando: %+v", ca, gates.NotaNoEncontrado, g)
	}
	if v.Summary.Error != 1 || v.Summary.Fail != 0 || v.Verdict != "red" {
		t.Fatalf("%s: Summary.Error 1 y rojo: %+v %s", ca, v.Summary, v.Verdict)
	}
}

// ---------------------------------------------------------------- CA-762

// CA-762, CA-764 y CA-765: desde la tarea, sin Sobre (hoom verify) y con
// Sobre (el sobre de un rol, sin seccion encierro), se usa el
// .hoom/local.yaml del principal: el gate corre con el prefijo, en el arbol
// de la tarea, con {rel} .hoom/worktrees/demo y {dir} el worktree.
func TestCA762_DesdeLaTareaConYSinSobreSeUsaElDelPrincipal(t *testing.T) {
	const ca = "CA-762"
	veeSaltarSinSh(t)
	for _, conSobre := range []bool{false, true} {
		log := veeFalso(t)
		cmd := `echo "dentro=${VEE_DENTRO:-no}"`
		root, wt, m := veeProyecto(t, veeGate("test", cmd), "", true)
		tpl := "ddev exec -d /var/www/html/{rel} --dir {dir} sh -c {cmd}"
		body := "ejecutar: " + strconv.Quote(tpl) + "\n"
		veeLocal(t, root, body)
		opt := Options{Full: true}
		if conSobre {
			opt.Sobre = cdwSobre(root)
		}
		v, _ := veeRun(t, ca, m, opt)
		if v.Entorno == nil || v.Entorno.Ejecutar != tpl || v.Entorno.SHA256 != veeSha(body) {
			t.Fatalf("%s: Sobre %v: el veredicto lleva la plantilla y el sha256 del archivo del principal: %+v", ca, conSobre, v.Entorno)
		}
		g := gateNamed(v, "test")
		if g == nil || g.Status != verdict.StatusPass || g.Entorno != verdict.EntornoPrefijo || g.Command != cmd || !strings.Contains(g.OutputTail, "dentro=1") {
			t.Fatalf("%s: Sobre %v: el gate corre con el prefijo: %+v", ca, conSobre, g)
		}
		want := []string{wt, "exec", "-d", "/var/www/html/.hoom/worktrees/demo", "--dir", wt, "sh", "-c", cmd}
		if ll := veeLlamadas(t, log); len(ll) != 1 || !reflect.DeepEqual(ll[0], want) {
			t.Fatalf("%s: Sobre %v: una llamada, en el arbol de la tarea, con %q: %q", ca, conSobre, want, ll)
		}
		if v.Verdict != "green" {
			t.Fatalf("%s: Sobre %v: el veredicto es verde: %s %+v", ca, conSobre, v.Verdict, v.Gates)
		}
	}
}

// CA-762: un .hoom/local.yaml dentro del worktree no se lee: la tarea corre
// como hoy y el veredicto no lleva entorno.
func TestCA762_ElDelWorktreeNoSeLee(t *testing.T) {
	const ca = "CA-762"
	veeSaltarSinSh(t)
	log := veeFalso(t)
	_, wt, m := veeProyecto(t, veeGate("test", "true"), "", true)
	veeLocal(t, wt, "ejecutar: \"ddev exec sh -c {cmd}\"\nclave_rara: 1\n")
	v, path := veeRun(t, ca, m, Options{Full: true})
	if v.Entorno != nil || gateNamed(v, "test").Entorno != "" {
		t.Fatalf("%s: el local.yaml del worktree no se lee: %+v", ca, v.Entorno)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"entorno"`) {
		t.Fatalf("%s: sin el del principal el JSON no lleva entorno: %s", ca, raw)
	}
	if ll := veeLlamadas(t, log); len(ll) != 0 {
		t.Fatalf("%s: el prefijo no se llama: %q", ca, ll)
	}
}

// ---------------------------------------------------------------- CA-763

// CA-763: un .hoom/local.yaml invalido es el error de Run, exacto, desde el
// principal y desde la tarea; ningun gate corre y no hay veredicto ni
// eventos vivos.
func TestCA763_ArchivoInvalidoEsElErrorDeRunSinVeredicto(t *testing.T) {
	const ca = "CA-763"
	veeSaltarSinSh(t)
	for _, c := range []struct {
		nombre, body string
		sinIgnorar   bool
		want         string
	}{
		{"clave desconocida", "ejecutar: \"ddev {cmd}\"\nfoo: bar\n", false, fmt.Sprintf(entorno.ErrClaveDesconocida, "foo")},
		{"sin {cmd}", "ejecutar: \"ddev exec sh -c\"\n", false, entorno.ErrSinCmd},
		{"gate que no es", "ejecutar: \"ddev {cmd}\"\nen_el_host: [semgrep]\n", false, fmt.Sprintf(entorno.ErrGateDesconocido, "semgrep")},
		{"en_el_host sin ejecutar", "en_el_host: [semgrep]\n", false, fmt.Sprintf(entorno.ErrGateDesconocido, "semgrep")},
		{"no ignorado", "ejecutar: \"ddev {cmd}\"\n", true, entorno.ErrNoIgnorado},
		{"no ignorado sin ejecutar", "ejecutar:\n", true, entorno.ErrNoIgnorado},
		{"el orden: la clave primero", "zzz: 1\nejecutar: \"ddev\"\nen_el_host: [semgrep]\n", true, fmt.Sprintf(entorno.ErrClaveDesconocida, "zzz")},
		{"YAML roto", "ejecutar: [\n", false, ""},
		{"raiz que no es mapa", "- ejecutar\n", false, ""},
	} {
		for _, tarea := range []bool{false, true} {
			root, wt, m := veeProyecto(t, veeGate("test", "touch vee-corrio")+veeGate("build", "true"), "", tarea)
			if c.sinIgnorar {
				cdwEscribir(t, filepath.Join(root, ".hoom", ".gitignore"), "cache/\nworktrees/\nruns/\nisolated/\nenvelopes/\n")
			}
			veeLocal(t, root, c.body)
			caso := fmt.Sprintf("%s (tarea %v)", c.nombre, tarea)
			v, path, err := Run(m, Options{Full: true})
			if err == nil {
				t.Fatalf("%s: %s: Run rechaza el archivo: %+v", ca, caso, v)
			}
			if c.want != "" && err.Error() != c.want {
				t.Fatalf("%s: %s: el error es %q; fue %q", ca, caso, c.want, err.Error())
			}
			if !strings.HasPrefix(err.Error(), ".hoom/local.yaml: ") {
				t.Fatalf("%s: %s: el error empieza con el archivo: %q", ca, caso, err.Error())
			}
			if path != "" {
				t.Fatalf("%s: %s: no hay artefacto: %q", ca, caso, path)
			}
			veeSinRastro(t, ca, caso, root, wt)
		}
	}
}

// CA-763 (aclaracion 5): el error de uso de --gate (exit 2) va antes que el
// de .hoom/local.yaml; con un --gate valido, sale el del archivo, sin
// veredicto parcial.
func TestCA763_ElErrorDeUsoDeGateVaPrimero(t *testing.T) {
	const ca = "CA-763"
	veeSaltarSinSh(t)
	root, _, m := veeProyecto(t, veeGate("test", "touch vee-corrio"), "", false)
	veeLocal(t, root, "foo: 1\n")
	_, _, err := Run(m, Options{Gates: []string{"no-es-un-gate"}})
	if ue := comoUso(t, ca, err); strings.HasPrefix(ue.Error(), ".hoom/local.yaml") {
		t.Fatalf("%s: el error de uso no es el del archivo: %q", ca, ue.Error())
	}
	_, _, err = Run(m, Options{Gates: []string{"test"}})
	if err == nil || err.Error() != fmt.Sprintf(entorno.ErrClaveDesconocida, "foo") {
		t.Fatalf("%s: con --gate valido sale el error del archivo: %v", ca, err)
	}
	veeSinRastro(t, ca, "--gate", root)
}

// CA-763 (aclaracion 6): fuera de git no hay ErrNoIgnorado: el archivo vale
// y los gates corren con el prefijo, con {rel} ".".
func TestCA763_FueraDeGitElArchivoVale(t *testing.T) {
	const ca = "CA-763"
	veeSaltarSinSh(t)
	log := veeFalso(t)
	m := veeSinGit(t, veeGate("test", "true"))
	veeLocal(t, m.Dir, "ejecutar: \""+veePlantilla+"\"\n")
	v, _ := veeRun(t, ca, m, Options{Full: true})
	if v.Entorno == nil || gateNamed(v, "test").Entorno != verdict.EntornoPrefijo {
		t.Fatalf("%s: fuera de git el archivo vale: %+v", ca, v.Entorno)
	}
	if ll := veeLlamadas(t, log); len(ll) != 1 || ll[0][0] != m.Dir || ll[0][3] != "/var/www/html/." {
		t.Fatalf("%s: el prefijo corre en %s con {rel} \".\": %q", ca, m.Dir, ll)
	}
}

// ---------------------------------------------------------------- CA-766

// CA-766 (aclaracion 2): con ejecutar, TODOS los gates llevan entorno: los
// de hoom.yaml con prefijo, los de en_el_host y los de hoom (spec_*,
// findings_open, ratchet) host, el ausente donde habria corrido. El JSON
// lleva la clave entorno arriba con ejecutar y sha256, y cada gate la suya.
func TestCA766_TodosLosGatesLlevanEntornoYElJSONLaClave(t *testing.T) {
	const ca = "CA-766"
	veeSaltarSinSh(t)
	veeFalso(t)
	root, _, m := veeProyecto(t, veeGate("test", "true")+veeGate("build", "true")+"  lint:\n    cmd: \"\"\n",
		"findings:\n  block_on: high\n", false)
	spec := writeSpecFile(t, root)
	base := 70.0
	writeRatchet(t, root, "echo 90", &base)
	body := "ejecutar: \"" + veePlantilla + "\"\nen_el_host: [build]\n"
	veeLocal(t, root, body)
	v, path := veeRun(t, ca, m, Options{Full: true, Spec: spec})
	want := map[string]string{"test": verdict.EntornoPrefijo, "lint": verdict.EntornoPrefijo, "build": verdict.EntornoHost}
	for _, propio := range []string{"spec_lint", "spec_trace", "spec_approved", "findings_open", "ratchet"} {
		if gateNamed(v, propio) == nil {
			t.Fatalf("%s: fixture: el veredicto lleva %s: %v", ca, propio, veeNombres(v))
		}
		want[propio] = verdict.EntornoHost
	}
	for _, g := range v.Gates {
		w, ok := want[g.Name]
		if !ok {
			w = verdict.EntornoHost // cualquier otro gate es de hoom
		}
		if g.Entorno != w {
			t.Fatalf("%s: %s lleva entorno %q: %+v", ca, g.Name, w, g)
		}
	}
	if l := gateNamed(v, "lint"); l.Status != verdict.StatusAbsent {
		t.Fatalf("%s: fixture: lint es ausente: %+v", ca, l)
	}
	if v.Entorno == nil || *v.Entorno != (verdict.Entorno{Ejecutar: veePlantilla, SHA256: veeSha(body)}) {
		t.Fatalf("%s: Verdict.Entorno es la plantilla y el sha256: %+v", ca, v.Entorno)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Entorno map[string]any   `json:"entorno"`
		Gates   []map[string]any `json:"gates"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(doc.Entorno, map[string]any{"ejecutar": veePlantilla, "sha256": veeSha(body)}) {
		t.Fatalf("%s: la clave entorno del JSON: %v", ca, doc.Entorno)
	}
	for _, g := range doc.Gates {
		if e, _ := g["entorno"].(string); e != verdict.EntornoPrefijo && e != verdict.EntornoHost {
			t.Fatalf("%s: cada gate del JSON lleva entorno: %v", ca, g)
		}
	}
}

// veeNormal es el JSON del veredicto sin id, fecha ni duraciones.
func veeNormal(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	delete(doc, "id")
	delete(doc, "created_at")
	if gs, ok := doc["gates"].([]any); ok {
		for _, g := range gs {
			if m, ok := g.(map[string]any); ok {
				delete(m, "duration_ms")
			}
		}
	}
	return doc
}

// CA-766 (aclaracion 3): sin ejecutar (sin archivo, con ejecutar vacio o
// nulo, o solo con en_el_host) el JSON es el de hoy: igual al de sin
// archivo salvo id, fecha y duraciones, sin ninguna clave entorno. Y la
// huella (el bloque git) es la misma con ejecutar.
func TestCA766_SinEjecutarElJSONEsElDeHoyYLaHuellaNoCambia(t *testing.T) {
	const ca = "CA-766"
	veeSaltarSinSh(t)
	veeFalso(t)
	root, _, m := veeProyecto(t, veeGate("test", "echo hola")+veeGate("build", "true")+"  lint:\n    cmd: \"\"\n", "", false)
	correr := func(caso string) map[string]any {
		_, path := veeRun(t, ca, m, Options{Full: true})
		doc := veeNormal(t, path)
		if err := os.Remove(path); err != nil { // el arbol queda como antes de correr
			t.Fatal(err)
		}
		return doc
	}
	hoy := correr("sin archivo")
	if raw, _ := json.Marshal(hoy); strings.Contains(string(raw), `"entorno"`) {
		t.Fatalf("%s: sin archivo no hay clave entorno: %s", ca, raw)
	}
	for _, body := range []string{"ejecutar:\n", "ejecutar: ~\n", "ejecutar: \"\"\n", "ejecutar: \"  \"\n", "en_el_host: [test]\n", "ejecutar:\nen_el_host: [build, test]\n"} {
		veeLocal(t, root, body)
		got := correr(body)
		if !reflect.DeepEqual(got, hoy) {
			a, _ := json.MarshalIndent(hoy, "", " ")
			b, _ := json.MarshalIndent(got, "", " ")
			t.Fatalf("%s: con %q el JSON es el de hoy:\n--- hoy ---\n%s\n--- con el archivo ---\n%s", ca, body, a, b)
		}
	}
	veeLocal(t, root, "ejecutar: \""+veePlantilla+"\"\n")
	con := correr("con ejecutar")
	if !reflect.DeepEqual(con["git"], hoy["git"]) {
		t.Fatalf("%s: la huella no cambia con ejecutar: %v vs %v", ca, con["git"], hoy["git"])
	}
	if con["entorno"] == nil {
		t.Fatalf("%s: con ejecutar el JSON lleva entorno: %v", ca, con)
	}
}

// CA-766: los valores y las etiquetas JSON son los del spec.
func TestCA766_ConstantesYEtiquetasJSON(t *testing.T) {
	const ca = "CA-766"
	if LineaEntorno != "entorno: %s (de .hoom/local.yaml)" || verdict.EntornoPrefijo != "prefijo" || verdict.EntornoHost != "host" {
		t.Fatalf("%s: constantes: %q %q %q", ca, LineaEntorno, verdict.EntornoPrefijo, verdict.EntornoHost)
	}
	if got := fmt.Sprintf(LineaEntorno, veePlantilla); got != "entorno: "+veePlantilla+" (de .hoom/local.yaml)" {
		t.Fatalf("%s: la linea: %q", ca, got)
	}
	b, _ := json.Marshal(verdict.Entorno{Ejecutar: "e", SHA256: "s"})
	if string(b) != `{"ejecutar":"e","sha256":"s"}` {
		t.Fatalf("%s: verdict.Entorno en JSON: %s", ca, b)
	}
	b, _ = json.Marshal(verdict.GateResult{Name: "x"})
	c, _ := json.Marshal(verdict.Verdict{})
	if strings.Contains(string(b), "entorno") || strings.Contains(string(c), "entorno") {
		t.Fatalf("%s: sin entorno no hay clave: %s %s", ca, b, c)
	}
	b, _ = json.Marshal(verdict.GateResult{Name: "x", Entorno: verdict.EntornoHost})
	if !strings.Contains(string(b), `"entorno":"host"`) {
		t.Fatalf("%s: GateResult.Entorno en JSON: %s", ca, b)
	}
}

// ---------------------------------------------------------------- CA-767

// CA-767: en la verify de un Sobre con la seccion encierro, el gate con
// prefijo corre sin encierro con la nota (exigir no) o no corre (completo y
// parcial: error -1 y la nota de exigir, veredicto rojo). Sin Sobre, o sin la
// seccion, no hay nota. El de en_el_host (en gates_sin_encierro para que el
// test no dependa del encierro de este sistema) corre como hoy.
func TestCA767_SobreConEncierroYPrefijo(t *testing.T) {
	const ca = "CA-767"
	veeSaltarSinSh(t)
	for _, exigir := range []string{manifest.ExigirNo, manifest.ExigirParcial, manifest.ExigirCompleto} {
		for _, conSobre := range []bool{true, false} {
			log := veeFalso(t)
			extra := "encierro:\n  exigir: " + exigir + "\n  gates_sin_encierro:\n    - build\n"
			root, wt, m := veeProyecto(t, veeGate("test", "touch vee-test-corrio")+veeGate("build", "true"), extra, true)
			veeLocal(t, root, "ejecutar: \""+veePlantilla+"\"\nen_el_host: [build]\n")
			opt := Options{Full: true}
			if conSobre {
				opt.Sobre = cdwSobre(root)
			}
			caso := fmt.Sprintf("exigir %s, Sobre %v", exigir, conSobre)
			v, _ := veeRun(t, ca, m, opt)
			g, b := gateNamed(v, "test"), gateNamed(v, "build")
			_, errCorrio := os.Stat(filepath.Join(wt, "vee-test-corrio"))
			switch {
			case !conSobre:
				if g.Status != verdict.StatusPass || g.Notes != "" || errCorrio != nil {
					t.Fatalf("%s: %s: sin Sobre no hay nota y el gate corre: %+v", ca, caso, g)
				}
			case exigir == manifest.ExigirNo:
				if g.Status != verdict.StatusPass || g.Notes != gates.NotaSinEncierroConPrefijo || g.Entorno != verdict.EntornoPrefijo || errCorrio != nil {
					t.Fatalf("%s: %s: corre sin encierro con la nota: %+v", ca, caso, g)
				}
				if n := len(veeLlamadas(t, log)); n != 1 {
					t.Fatalf("%s: %s: el prefijo corrio una vez: %d", ca, caso, n)
				}
			default:
				if g.Status != verdict.StatusError || g.ExitCode != -1 || g.Notes != fmt.Sprintf(gates.NotaExigirConPrefijo, exigir) ||
					g.Command != "touch vee-test-corrio" || g.Entorno != verdict.EntornoPrefijo || errCorrio == nil {
					t.Fatalf("%s: %s: no corre, error -1 con la nota de exigir: %+v", ca, caso, g)
				}
				if n := len(veeLlamadas(t, log)); n != 0 {
					t.Fatalf("%s: %s: el prefijo no se llama: %d", ca, caso, n)
				}
				if v.Verdict != "red" {
					t.Fatalf("%s: %s: el veredicto es rojo: %s", ca, caso, v.Verdict)
				}
			}
			if b.Status != verdict.StatusPass || b.Entorno != verdict.EntornoHost || strings.Contains(b.Notes, ".hoom/local.yaml") {
				t.Fatalf("%s: %s: el de en_el_host corre como hoy: %+v", ca, caso, b)
			}
		}
	}
}

// CA-767: con Sobre pero sin la seccion encierro, el gate con prefijo corre
// sin ninguna nota.
func TestCA767_SobreSinSeccionEncierroNoHayNota(t *testing.T) {
	const ca = "CA-767"
	veeSaltarSinSh(t)
	veeFalso(t)
	root, _, m := veeProyecto(t, veeGate("test", "true"), "", true)
	veeLocal(t, root, "ejecutar: \""+veePlantilla+"\"\n")
	v, _ := veeRun(t, ca, m, Options{Full: true, Sobre: cdwSobre(root)})
	if g := gateNamed(v, "test"); g.Status != verdict.StatusPass || g.Notes != "" || g.Entorno != verdict.EntornoPrefijo {
		t.Fatalf("%s: sin la seccion no hay nota: %+v", ca, g)
	}
}

// ---------------------------------------------------------------- CA-773

// CA-773: un gate que crea, cambia o borra el .hoom/local.yaml del principal
// deja el gate entorno_local; sin ejecutar al empezar, su entorno es "" (el
// JSON de hoy mas ese gate). Cambiarlo por un directorio cuenta.
func TestCA773_UnGateQueTocaElArchivoDejaEntornoLocal(t *testing.T) {
	const ca = "CA-773"
	veeSaltarSinSh(t)
	for _, c := range []struct {
		nombre, antes, gate string
	}{
		{"lo crea", "", "printf 'foo: 1\\n' > .hoom/local.yaml"},
		{"lo cambia", "en_el_host: [test]\n", "printf 'en_el_host: [build]\\n' > .hoom/local.yaml"},
		{"le agrega un byte", "ejecutar:\n", "printf ' ' >> .hoom/local.yaml"},
		{"lo borra", "en_el_host: [test]\n", "rm .hoom/local.yaml"},
		{"lo cambia por un directorio", "ejecutar:\n", "rm .hoom/local.yaml && mkdir .hoom/local.yaml"},
	} {
		root, _, m := veeProyecto(t, veeGate("test", c.gate)+veeGate("build", "true"), "", false)
		if c.antes != "" {
			veeLocal(t, root, c.antes)
		}
		v, _ := veeRun(t, ca, m, Options{Full: true})
		veeExigeEntornoLocal(t, ca, c.nombre, v, root, "")
		if want := []string{"build", GateEntornoLocal, "test"}; !reflect.DeepEqual(veeNombres(v), want) {
			t.Fatalf("%s: %s: los gates son los de hoom.yaml y entorno_local: %v", ca, c.nombre, veeNombres(v))
		}
		if v.Entorno != nil {
			t.Fatalf("%s: %s: sin ejecutar al empezar no hay Verdict.Entorno: %+v", ca, c.nombre, v.Entorno)
		}
	}
}

// CA-773: un archivo que nadie toca, o que un gate reescribe con los mismos
// bytes, no deja el gate; un .hoom/local.yaml dentro del worktree tampoco
// (no es el del principal).
func TestCA773_SinCambioNoHayGate(t *testing.T) {
	const ca = "CA-773"
	veeSaltarSinSh(t)
	for _, c := range []struct {
		nombre, antes, gate string
		tarea               bool
	}{
		{"sin archivo y sin tocarlo", "", "true", false},
		{"con archivo y sin tocarlo", "en_el_host: [test]\n", "cat .hoom/local.yaml", false},
		{"reescrito con los mismos bytes", "en_el_host: [test]\n", "printf 'en_el_host: [test]\\n' > .hoom/local.yaml", false},
		{"el del worktree", "", "mkdir -p .hoom && printf 'ejecutar: \"x {cmd}\"\\n' > .hoom/local.yaml", true},
	} {
		root, wt, m := veeProyecto(t, veeGate("test", c.gate), "", c.tarea)
		if c.antes != "" {
			veeLocal(t, root, c.antes)
		}
		v, _ := veeRun(t, ca, m, Options{Full: true})
		if g := veeGateEntornoLocal(v); g != nil || v.Verdict != "green" {
			t.Fatalf("%s: %s: no hay %s y el veredicto es el de hoy: %+v %s %+v", ca, c.nombre, GateEntornoLocal, g, v.Verdict, v.Gates)
		}
		if c.tarea {
			if _, err := os.Stat(filepath.Join(wt, ".hoom", "local.yaml")); err != nil {
				t.Fatalf("%s: fixture: el gate escribio el del worktree: %v", ca, err)
			}
		}
	}
}

// CA-773: desde la tarea, con Sobre y sin el, un gate que escribe el
// .hoom/local.yaml del principal deja el gate; tambien en una corrida
// --gate (parcial), y con ejecutar (entonces con entorno host, y
// Verdict.Entorno es el del archivo como se leyo al empezar).
func TestCA773_DesdeLaTareaConSobreConGateYConEjecutar(t *testing.T) {
	const ca = "CA-773"
	veeSaltarSinSh(t)
	for _, c := range []struct {
		nombre             string
		sobre, conEjecutar bool
		gates              []string
	}{
		{"sin Sobre", false, false, nil},
		{"con Sobre", true, false, nil},
		{"--gate", false, false, []string{"test"}},
		{"--gate con Sobre", true, false, []string{"test"}},
		{"con ejecutar", false, true, nil},
		{"con ejecutar y Sobre", true, true, nil},
	} {
		veeFalso(t)
		// el gate escribe el del principal desde el arbol de la tarea
		gate := `c=$(git rev-parse --path-format=absolute --git-common-dir) && printf '# cambio\n' >> "$c/../.hoom/local.yaml"`
		root, wt, m := veeProyecto(t, veeGate("test", gate)+veeGate("build", "true"), "", true)
		body := "en_el_host: [test]\n"
		if c.conEjecutar {
			body = "ejecutar: \"" + veePlantilla + "\"\n"
		}
		veeLocal(t, root, body)
		opt := Options{Full: true, Gates: c.gates}
		if c.sobre {
			opt.Sobre = cdwSobre(root)
		}
		v, _ := veeRun(t, ca, m, opt)
		want := ""
		if c.conEjecutar {
			want = verdict.EntornoHost
		}
		veeExigeEntornoLocal(t, ca, c.nombre, v, "", want)
		if c.gates != nil && !v.Partial {
			t.Fatalf("%s: %s: la corrida --gate es parcial: %+v", ca, c.nombre, v)
		}
		if c.conEjecutar && (v.Entorno == nil || v.Entorno.SHA256 != veeSha(body)) {
			t.Fatalf("%s: %s: Verdict.Entorno es el del archivo al empezar: %+v", ca, c.nombre, v.Entorno)
		}
		if !c.conEjecutar && v.Entorno != nil {
			t.Fatalf("%s: %s: sin ejecutar no hay Verdict.Entorno: %+v", ca, c.nombre, v.Entorno)
		}
		if c.gates == nil && cdwUltimoCompletoVerde(t, wt) {
			t.Fatalf("%s: %s: LatestComplete no da verde", ca, c.nombre)
		}
	}
}

// CA-773: fuera de git, el principal es la raiz del proyecto: un gate que
// crea su .hoom/local.yaml deja el gate, y el archivo invalido que dejo es
// el error de la verify siguiente.
func TestCA773_FueraDeGit(t *testing.T) {
	const ca = "CA-773"
	veeSaltarSinSh(t)
	m := veeSinGit(t, veeGate("test", "mkdir -p .hoom && printf 'foo: 1\\n' > .hoom/local.yaml"))
	v, _ := veeRun(t, ca, m, Options{Full: true})
	veeExigeEntornoLocal(t, ca, "fuera de git", v, m.Dir, "")
	if _, _, err := Run(m, Options{Full: true}); err == nil || err.Error() != fmt.Sprintf(entorno.ErrClaveDesconocida, "foo") {
		t.Fatalf("%s: la verify siguiente lee el archivo nuevo y lo rechaza: %v", ca, err)
	}
}

// CA-773: los textos.
func TestCA773_Constantes(t *testing.T) {
	if GateEntornoLocal != "entorno_local" || NotaEntornoLocal != "manipulacion: .hoom/local.yaml cambio mientras corrian los gates" {
		t.Fatalf("CA-773: %q %q", GateEntornoLocal, NotaEntornoLocal)
	}
}

// ---------------------------------------------------------------- CA-771

// veeGitViejo pone primero en el PATH un git falso, un script de sh, que
// contesta "git version 2.47.0" a toda llamada que lleva el argumento
// version en cualquier posicion (la de gitx.AceptaRutasRelativas es
// `git -c core.fsmonitor=false version`) y pasa las demas al git real, que
// busca antes de cambiar el PATH. Usa t.Setenv: el test no puede ser
// paralelo.
func veeGitViejo(t *testing.T) {
	t.Helper()
	gitReal, err := exec.LookPath("git")
	if err != nil {
		t.Fatalf("fixture: sin git en el PATH: %v", err)
	}
	bin := t.TempDir()
	script := "#!/bin/sh\n" +
		"for a do\n" +
		"  if [ \"$a\" = version ]; then\n" +
		"    echo 'git version 2.47.0'\n" +
		"    exit 0\n" +
		"  fi\n" +
		"done\n" +
		"exec " + veeQ(gitReal) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := exec.Command("git", "-c", "core.fsmonitor=false", "version").Output(); err != nil || strings.TrimSpace(string(out)) != "git version 2.47.0" {
		t.Fatalf("fixture: el git falso dice 2.47.0: %q %v", out, err)
	}
}

// CA-771 (go doc de entorno.Ajuste.WorktreesRelativos y de
// entorno.ErrWorktreesRelativosGitViejo): worktrees_relativos es de `hoom
// task start`, no de verify. Un .hoom/local.yaml que solo trae
// worktrees_relativos: true deja el verify como hoy, desde el principal y
// desde la tarea, aun con un git que dice 2.47 (el archivo es valido; el
// error de git viejo es solo de task start): Run no falla, escribe su
// veredicto, sin Verdict.Entorno ni el entorno de ningun gate, y el JSON es
// el de sin archivo salvo id, fecha y duraciones. Con ejecutar, el entorno
// es el de siempre, con el sha256 del archivo entero.
func TestCA771_WorktreesRelativosNoCambiaElVerify(t *testing.T) {
	const ca = "CA-771"
	veeSaltarSinSh(t)
	veeFalso(t)
	veeGitViejo(t)
	for _, tarea := range []bool{false, true} {
		root, wt, m := veeProyecto(t, veeGate("test", "echo hola")+veeGate("build", "true")+"  lint:\n    cmd: \"\"\n", "", tarea)
		dir := root
		if tarea {
			dir = wt
		}
		correr := func(caso string) (*verdict.Verdict, map[string]any) {
			v, path, err := Run(m, Options{Full: true})
			if err != nil {
				t.Fatalf("%s: %s (tarea %v): verify corre como hoy: %v", ca, caso, tarea, err)
			}
			if path == "" || veeVeredictos(t, dir) != 1 {
				t.Fatalf("%s: %s (tarea %v): verify escribe su veredicto: %q", ca, caso, tarea, path)
			}
			doc := veeNormal(t, path)
			if err := os.Remove(path); err != nil { // el arbol queda como antes de correr
				t.Fatal(err)
			}
			return v, doc
		}
		_, hoy := correr("sin archivo")
		const body = "worktrees_relativos: true\n"
		veeLocal(t, root, body)
		v, got := correr(body)
		if v.Entorno != nil {
			t.Fatalf("%s: %q (tarea %v): sin ejecutar no hay Verdict.Entorno: %+v", ca, body, tarea, v.Entorno)
		}
		for _, g := range v.Gates {
			if g.Entorno != "" {
				t.Fatalf("%s: %q (tarea %v): sin ejecutar ningun gate lleva entorno: %+v", ca, body, tarea, g)
			}
		}
		if raw, _ := json.Marshal(got); strings.Contains(string(raw), `"entorno"`) {
			t.Fatalf("%s: %q (tarea %v): el JSON no lleva ninguna clave entorno: %s", ca, body, tarea, raw)
		}
		if !reflect.DeepEqual(got, hoy) {
			a, _ := json.MarshalIndent(hoy, "", " ")
			b, _ := json.MarshalIndent(got, "", " ")
			t.Fatalf("%s: %q (tarea %v): el JSON es el de hoy:\n--- sin archivo ---\n%s\n--- con el archivo ---\n%s", ca, body, tarea, a, b)
		}
		con := "ejecutar: \"" + veePlantilla + "\"\nworktrees_relativos: true\n"
		veeLocal(t, root, con)
		v, _ = correr("con ejecutar")
		if v.Entorno == nil || *v.Entorno != (verdict.Entorno{Ejecutar: veePlantilla, SHA256: veeSha(con)}) {
			t.Fatalf("%s: con ejecutar y worktrees_relativos (tarea %v): Verdict.Entorno es la plantilla y el sha256 del archivo: %+v", ca, tarea, v.Entorno)
		}
		if g := gateNamed(v, "test"); g == nil || g.Entorno != verdict.EntornoPrefijo || g.Status != verdict.StatusPass {
			t.Fatalf("%s: con ejecutar y worktrees_relativos (tarea %v): el gate corre con el prefijo: %+v", ca, tarea, g)
		}
	}
}
