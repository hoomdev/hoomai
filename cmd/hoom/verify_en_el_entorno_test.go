// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md contra el
// BINARIO `hoom verify`:
//
//   - CA-760: con --json, un comando que no esta es error con 127, la nota
//     de NotaNoEncontrado y el veredicto rojo;
//   - CA-763: un .hoom/local.yaml invalido sale con su texto exacto, sin
//     veredicto; el error de uso de --gate (exit 2) va antes (aclaracion 5);
//   - CA-766: la linea `entorno: <plantilla> (de .hoom/local.yaml)` debajo
//     de la primera; con --json va a stderr y stdout es solo el veredicto
//     (aclaracion 13); sin ejecutar no hay tal linea;
//   - CA-766 (encargo de la ronda 0b): en esa linea, una plantilla con una
//     runa no imprimible (unicode.IsPrint) va con strconv.Quote, y una toda
//     imprimible va tal cual;
//   - CA-769: la sugerencia del perfil, una vez, debajo del reporte, solo
//     con un gate en error con 127 y sin .hoom/local.yaml; nada con --json
//     (aclaraciones 11 y 12).
//
// Un `ddev` falso primero en el PATH; un PATH sin php para las sugerencias.
// Compila el binario: se omite con -short. Escritos desde el spec, las
// aclaraciones de la ronda 0 y el go doc, sin leer la implementacion.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"

	hentorno "github.com/hoomdev/hoomai/internal/entorno"
	hgates "github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	hprofiles "github.com/hoomdev/hoomai/internal/profiles"
	hverdict "github.com/hoomdev/hoomai/internal/verdict"
	hverifycmd "github.com/hoomdev/hoomai/internal/verifycmd"
)

const veePlantilla = "ddev exec -d /var/www/html/{rel} sh -c {cmd}"

func veeSaltar(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("los comandos falsos son scripts de sh")
	}
}

func veeGate(nombre, cmd string) string {
	return "  " + nombre + ":\n    required: true\n    cmd: " + strconv.Quote(cmd) + "\n"
}

func veeEscribir(t *testing.T, dir, rel, body string, modo os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), modo); err != nil {
		t.Fatal(err)
	}
}

// veeRepo es un proyecto hoom en git (main) con hoom.yaml (extra arriba,
// gates despues) y el .hoom/.gitignore canonico con local.yaml, commiteado.
func veeRepo(t *testing.T, extra, gatesYAML string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	qcGit(t, dir, "init", "-q", "-b", "main")
	qcGit(t, dir, "config", "user.email", "test@hoom.dev")
	qcGit(t, dir, "config", "user.name", "hoom test")
	veeEscribir(t, dir, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\n"+extra+"gates:\n"+gatesYAML, 0o644)
	gi := hoomfs.GitignoreBody()
	if !strings.Contains("\n"+gi, "\nlocal.yaml\n") {
		gi += "local.yaml\n"
	}
	veeEscribir(t, dir, ".hoom/.gitignore", gi, 0o644)
	veeEscribir(t, dir, "app.go", "package app\n", 0o644)
	qcGit(t, dir, "add", "-A")
	qcGit(t, dir, "commit", "-q", "-m", "inicial")
	return dir
}

// veeDdev es una carpeta con un `ddev` falso que corre `sh -c` con su
// ultimo argumento, para el frente del PATH.
func veeDdev(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	s := "#!/bin/sh\nfor ultimo do :; done\nVEE_DENTRO=1\nexport VEE_DENTRO\nexec sh -c \"$ultimo\"\n"
	if err := os.WriteFile(filepath.Join(bin, "ddev"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// veeSinPHP es un PATH con las herramientas del sistema que usan hoom y los
// gates (git, sh...) y sin php.
func veeSinPHP(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	for _, n := range []string{"git", "sh", "bash", "env", "cat", "ls", "mkdir", "rm", "cp", "mv", "touch", "printf", "echo",
		"uname", "id", "tr", "sed", "grep", "head", "tail", "wc", "sort", "dirname", "basename", "true", "false", "test",
		"sleep", "ps", "kill", "chmod", "readlink", "realpath", "stat", "date", "bwrap", "setsid", "nproc", "getconf"} {
		if p, err := exec.LookPath(n); err == nil {
			_ = os.Symlink(p, filepath.Join(bin, n))
		}
	}
	return bin
}

// veeVeredictos cuenta los veredictos en disco.
func veeVeredictos(dir string) int {
	es, _ := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts"))
	n := 0
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

func veeLineasNoVacias(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimRight(l, "\r"))
		}
	}
	return out
}

// ---------------------------------------------------------------- CA-760

// CA-760: `hoom verify --json` con un comando que no esta: el gate es error
// con 127 y la nota que lo nombra, Summary.Error lo cuenta, el veredicto es
// rojo y hoom sale distinto de 0.
func TestCA760_VerifyJSONConUnComandoQueNoEsta(t *testing.T) {
	veeSaltar(t)
	bin := qcHoom(t)
	dir := veeRepo(t, "", veeGate("test", "vee-no-existe-760 --version"))
	r := qcCorrer(t, bin, dir, qcSinCorrida, "verify", "--json")
	var v hverdict.Verdict
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
		t.Fatalf("CA-760: stdout es el veredicto: %v\n%s", err, r.todo())
	}
	var g *hverdict.GateResult
	for i := range v.Gates {
		if v.Gates[i].Name == "test" {
			g = &v.Gates[i]
		}
	}
	if g == nil || g.Status != hverdict.StatusError || g.ExitCode != 127 ||
		!strings.HasPrefix(g.Notes, hgates.NotaNoEncontrado+": ") || !strings.Contains(g.Notes, "vee-no-existe-760") {
		t.Fatalf("CA-760: error, 127, la nota %q...: %+v", hgates.NotaNoEncontrado, g)
	}
	if v.Summary.Error != 1 || v.Verdict != "red" || r.exit == 0 {
		t.Fatalf("CA-760: Summary.Error 1, rojo, exit distinto de 0: %+v %s exit %d", v.Summary, v.Verdict, r.exit)
	}
}

// ---------------------------------------------------------------- CA-763

// CA-763: los cuatro errores por el binario, con y sin --json: hoom sale
// distinto de 0 con el texto exacto, ningun gate corre y no queda
// veredicto; el error de uso de --gate (exit 2) va antes.
func TestCA763_VerifyConUnArchivoInvalido(t *testing.T) {
	veeSaltar(t)
	bin := qcHoom(t)
	for _, c := range []struct {
		nombre, body string
		sinIgnorar   bool
		want         string
	}{
		{"clave desconocida", "ejecutar: \"ddev {cmd}\"\nfoo: 1\n", false, fmt.Sprintf(hentorno.ErrClaveDesconocida, "foo")},
		{"sin {cmd}", "ejecutar: \"ddev exec sh -c\"\n", false, hentorno.ErrSinCmd},
		{"gate que no es", "ejecutar: \"ddev {cmd}\"\nen_el_host: [semgrep]\n", false, fmt.Sprintf(hentorno.ErrGateDesconocido, "semgrep")},
		{"no ignorado", "ejecutar: \"ddev {cmd}\"\n", true, hentorno.ErrNoIgnorado},
	} {
		for _, args := range [][]string{{"verify"}, {"verify", "--json"}, {"verify", "--full"}, {"verify", "--gate", "test"}} {
			dir := veeRepo(t, "", veeGate("test", "touch vee-corrio"))
			if c.sinIgnorar {
				veeEscribir(t, dir, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\nenvelopes/\n", 0o644)
				qcGit(t, dir, "commit", "-q", "-am", "sin local.yaml en el ignore")
			}
			veeEscribir(t, dir, hentorno.Ruta, c.body, 0o644)
			r := qcCorrer(t, bin, dir, qcSinCorrida, args...)
			if r.exit == 0 || !strings.Contains(r.todo(), c.want) {
				t.Fatalf("CA-763: %s %q: sale distinto de 0 con %q: exit %d\n%s", c.nombre, args, c.want, r.exit, r.todo())
			}
			if n := veeVeredictos(dir); n != 0 {
				t.Fatalf("CA-763: %s %q: no queda veredicto: %d", c.nombre, args, n)
			}
			if _, err := os.Stat(filepath.Join(dir, "vee-corrio")); err == nil {
				t.Fatalf("CA-763: %s %q: ningun gate corre", c.nombre, args)
			}
			if len(args) > 1 && args[1] == "--json" && json.Valid([]byte(r.stdout)) && strings.Contains(r.stdout, `"gates"`) {
				t.Fatalf("CA-763: %s: con --json no hay veredicto en stdout: %s", c.nombre, r.stdout)
			}
		}
	}
	dir := veeRepo(t, "", veeGate("test", "touch vee-corrio"))
	veeEscribir(t, dir, hentorno.Ruta, "foo: 1\n", 0o644)
	r := qcCorrer(t, bin, dir, qcSinCorrida, "verify", "--gate", "no-es-un-gate")
	if r.exit != 2 || strings.Contains(r.todo(), ".hoom/local.yaml: ") {
		t.Fatalf("CA-763: el error de uso de --gate va antes: exit 2 sin el error del archivo: exit %d\n%s", r.exit, r.todo())
	}
}

// ---------------------------------------------------------------- CA-766

// CA-766 y aclaracion 13: con ejecutar, `hoom verify` imprime la linea del
// entorno justo debajo de su primera linea; con --json va a stderr, justo
// debajo de la primera de stderr, y stdout es solo el veredicto, con la
// clave entorno. Sin ejecutar no hay tal linea en ningun stream.
func TestCA766_LaLineaDelEntorno(t *testing.T) {
	veeSaltar(t)
	bin := qcHoom(t)
	linea := fmt.Sprintf(hverifycmd.LineaEntorno, veePlantilla)
	if linea != "entorno: "+veePlantilla+" (de .hoom/local.yaml)" {
		t.Fatalf("CA-766: la linea: %q", linea)
	}
	env := map[string]string{"PATH": veeDdev(t) + string(os.PathListSeparator) + os.Getenv("PATH")}
	dir := veeRepo(t, "", veeGate("test", "echo dentro=${VEE_DENTRO:-no}"))
	veeEscribir(t, dir, hentorno.Ruta, "ejecutar: \""+veePlantilla+"\"\n", 0o644)

	r := qcCorrer(t, bin, dir, env, "verify")
	ls := veeLineasNoVacias(r.stdout)
	if r.exit != 0 || len(ls) < 2 || ls[1] != linea {
		t.Fatalf("CA-766: la segunda linea de hoom verify es %q: exit %d\n%s", linea, r.exit, r.todo())
	}
	if strings.Count(r.todo(), linea) != 1 {
		t.Fatalf("CA-766: la linea sale una vez:\n%s", r.todo())
	}

	r = qcCorrer(t, bin, dir, env, "verify", "--json")
	var v hverdict.Verdict
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
		t.Fatalf("CA-766: con --json stdout es solo el veredicto: %v\n%s", err, r.stdout)
	}
	if strings.Contains(r.stdout, linea) || v.Entorno == nil || v.Entorno.Ejecutar != veePlantilla {
		t.Fatalf("CA-766: con --json el veredicto lleva entorno y stdout no lleva la linea: %s", r.stdout)
	}
	if es := veeLineasNoVacias(r.stderr); len(es) < 2 || es[1] != linea {
		t.Fatalf("CA-766: con --json la linea va a stderr, debajo de la primera: %q", r.stderr)
	}

	for _, body := range []string{"", "ejecutar:\n", "en_el_host: [test]\n"} {
		dir := veeRepo(t, "", veeGate("test", "true"))
		if body != "" {
			veeEscribir(t, dir, hentorno.Ruta, body, 0o644)
		}
		for _, args := range [][]string{{"verify"}, {"verify", "--json"}} {
			r := qcCorrer(t, bin, dir, env, args...)
			if r.exit != 0 || strings.Contains(r.todo(), "entorno: ") || strings.Contains(r.stdout, `"entorno"`) {
				t.Fatalf("CA-766: sin ejecutar (%q) %q no hay linea ni clave entorno: exit %d\n%s", body, args, r.exit, r.todo())
			}
		}
	}
}

// veeExigeLineaEntorno exige, en una salida de `hoom verify`, la linea want
// una sola vez y, cuando la plantilla va citada, que la plantilla cruda no
// aparezca en ninguna parte. Con un ESC en la plantilla (la runa que abre
// una secuencia de la terminal), ademas: la secuencia cruda de la plantilla,
// desde su ESC, no aparece en la salida, y la linea del entorno (la que
// termina en "(de .hoom/local.yaml)") no lleva ningun ESC crudo. El resto de
// la salida puede llevar ESC: son los colores propios del reporte de hoom
// (ronda 0c).
func veeExigeLineaEntorno(t *testing.T, caso string, r qcSalida, want, plantilla string, citada bool) {
	t.Helper()
	if n := strings.Count(r.todo(), want); n != 1 {
		t.Fatalf("CA-766: %s: la linea %q sale una vez: %d\n%s", caso, want, n, r.todo())
	}
	if !citada {
		return
	}
	if strings.Contains(r.todo(), plantilla) {
		t.Fatalf("CA-766: %s: citada, la salida no lleva la plantilla cruda:\n%q", caso, r.todo())
	}
	i := strings.IndexRune(plantilla, '\x1b')
	if i < 0 {
		return
	}
	if cruda := plantilla[i:]; strings.Contains(r.todo(), cruda) {
		t.Fatalf("CA-766: %s: la salida no lleva la secuencia cruda de la plantilla %q:\n%q", caso, cruda, r.todo())
	}
	for _, l := range strings.Split(r.todo(), "\n") {
		if strings.Contains(l, "(de .hoom/local.yaml)") && strings.ContainsRune(l, '\x1b') {
			t.Fatalf("CA-766: %s: la linea del entorno no lleva ESC crudos: %q", caso, l)
		}
	}
}

// CA-766 (encargo de la ronda 0b): la linea entorno: de `hoom verify` lleva
// la plantilla tal cual cuando todas sus runas son imprimibles
// (unicode.IsPrint, tambien con unicode, comillas y barras), y citada con
// strconv.Quote cuando alguna no lo es (un tab, ESC, un salto de linea): la
// salida no lleva la runa cruda. Los gates corren igual: el tab es un
// blanco para sh, ESC queda en un comentario de sh y el salto separa dos
// comandos. Sin --json es la segunda linea de stdout; con --json, la
// segunda de stderr, y el veredicto guarda la plantilla tal cual.
func TestCA766_LaLineaDelEntornoCitaLaPlantillaNoImprimible(t *testing.T) {
	veeSaltar(t)
	bin := qcHoom(t)
	imprimible := func(s string) bool {
		for _, r := range s {
			if !unicode.IsPrint(r) {
				return false
			}
		}
		return true
	}
	for _, c := range []struct {
		nombre, plantilla string
		citada            bool
	}{
		{"imprimible con unicode, comillas y barras", `sh -c {cmd} # año 日本 "x" 'y' \ z`, false},
		{"un tab al final", "sh -c {cmd}\t", true},
		{"un tab y unicode imprimible", "sh -c {cmd}\t# año 日本", true},
		{"ESC en un comentario", "sh -c {cmd} # \x1b[31mrojo", true},
		{"un salto de linea", "true\nsh -c {cmd}", true},
	} {
		if imprimible(c.plantilla) == c.citada {
			t.Fatalf("fixture: %s: la plantilla es imprimible: %v", c.nombre, !c.citada)
		}
		want := fmt.Sprintf(hverifycmd.LineaEntorno, c.plantilla)
		if c.citada {
			want = fmt.Sprintf(hverifycmd.LineaEntorno, strconv.Quote(c.plantilla))
		}
		dir := veeRepo(t, "", veeGate("test", "echo hola"))
		veeEscribir(t, dir, hentorno.Ruta, "ejecutar: "+strconv.Quote(c.plantilla)+"\n", 0o644)

		r := qcCorrer(t, bin, dir, qcSinCorrida, "verify")
		ls := veeLineasNoVacias(r.stdout)
		if r.exit != 0 || len(ls) < 2 || ls[1] != want {
			t.Fatalf("CA-766: %s: la segunda linea de hoom verify es %q: exit %d\n%s", c.nombre, want, r.exit, r.todo())
		}
		veeExigeLineaEntorno(t, c.nombre, r, want, c.plantilla, c.citada)

		r = qcCorrer(t, bin, dir, qcSinCorrida, "verify", "--json")
		var v hverdict.Verdict
		if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
			t.Fatalf("CA-766: %s: con --json stdout es solo el veredicto: %v\n%s", c.nombre, err, r.stdout)
		}
		if v.Entorno == nil || v.Entorno.Ejecutar != c.plantilla {
			t.Fatalf("CA-766: %s: el veredicto guarda la plantilla tal cual (%q): %+v", c.nombre, c.plantilla, v.Entorno)
		}
		if es := veeLineasNoVacias(r.stderr); r.exit != 0 || len(es) < 2 || es[1] != want {
			t.Fatalf("CA-766: %s: con --json la linea va a stderr, debajo de la primera: %q: exit %d\n%q", c.nombre, want, r.exit, r.stderr)
		}
		veeExigeLineaEntorno(t, c.nombre+" (--json)", r, want, c.plantilla, c.citada)
	}
}

// ---------------------------------------------------------------- CA-769

// veeLaravel es un proyecto con profile laravel cuyos gates del perfil
// quedan ausentes, salvo los de gatesYAML.
func veeLaravel(t *testing.T, gatesYAML string, nombres ...string) string {
	t.Helper()
	perfil, _, err := hprofiles.Resolve("laravel")
	if err != nil {
		t.Fatalf("fixture: el perfil laravel: %v", err)
	}
	propios := map[string]bool{}
	for _, n := range nombres {
		propios[n] = true
	}
	var ns []string
	for n := range perfil {
		if !propios[n] {
			ns = append(ns, n)
		}
	}
	sort.Strings(ns)
	for _, n := range ns {
		gatesYAML += "  " + n + ":\n    cmd: \"\"\n"
	}
	return veeRepo(t, "profile: laravel\n", gatesYAML)
}

// veeSugerencias son las de ddev y sail de laravel.
func veeSugerencias(t *testing.T) (ddev, sail string) {
	t.Helper()
	ents, err := hprofiles.Entornos("laravel")
	if err != nil || len(ents) != 2 {
		t.Fatalf("fixture: laravel declara ddev y sail: %+v %v", ents, err)
	}
	return ents[0].Sugerencia(), ents[1].Sugerencia()
}

// veeExigeSugerencias exige las sugerencias want en ese orden, una vez cada
// una, en un solo stream y al final de el.
func veeExigeSugerencias(t *testing.T, caso string, r qcSalida, want ...string) {
	t.Helper()
	if n := strings.Count(r.todo(), "sugerencia: "); n != len(want) {
		t.Fatalf("CA-769: %s: %d sugerencias: %d\n%s", caso, len(want), n, r.todo())
	}
	if len(want) == 0 {
		return
	}
	stream := r.stdout
	if !strings.Contains(stream, want[0]) {
		stream = r.stderr
	}
	fin := 0
	for _, s := range want {
		i := strings.Index(stream, s)
		if i < fin {
			t.Fatalf("CA-769: %s: la sugerencia, en el orden del perfil y en el mismo stream:\n%s\n--- salida ---\n%s", caso, s, r.todo())
		}
		fin = i + len(s)
	}
	if strings.TrimSpace(stream[fin:]) != "" {
		t.Fatalf("CA-769: %s: las sugerencias van debajo del reporte, al final: sobra %q", caso, stream[fin:])
	}
}

// CA-769 y aclaraciones 11 y 12: con un gate en error con 127, sin php en
// el PATH y sin .hoom/local.yaml, `hoom verify` sugiere ddev, sail o los
// dos (en ese orden), una vez, debajo del reporte. Con --json, con php, con
// un exit 1 o 126, sin los archivos, o con un .hoom/local.yaml (aunque no
// tenga ejecutar), no sugiere nada. Nunca escribe .hoom/local.yaml.
func TestCA769_VerifySugiereElEntorno(t *testing.T) {
	veeSaltar(t)
	bin := qcHoom(t)
	ddev, sail := veeSugerencias(t)
	sinPHP := veeSinPHP(t)
	env := map[string]string{"PATH": sinPHP}
	const falta = "php-vee-no-esta artisan test"
	for _, c := range []struct {
		nombre   string
		gates    string
		nombres  []string
		archivos []string
		local    string
		args     []string
		want     []string
	}{
		{"ddev", veeGate("test", falta), []string{"test"}, []string{".ddev/config.yaml"}, "", nil, []string{ddev}},
		{"sail", veeGate("test", falta), []string{"test"}, []string{"vendor/bin/sail"}, "", nil, []string{sail}},
		{"los dos", veeGate("test", falta), []string{"test"}, []string{".ddev/config.yaml", "vendor/bin/sail"}, "", nil, []string{ddev, sail}},
		{"dos gates con 127, una vez", veeGate("test", falta) + veeGate("lint", "otro-vee-no-esta"), []string{"test", "lint"},
			[]string{".ddev/config.yaml"}, "", nil, []string{ddev}},
		{"con --full", veeGate("test", falta), []string{"test"}, []string{".ddev/config.yaml"}, "", []string{"--full"}, []string{ddev}},
		{"con --json", veeGate("test", falta), []string{"test"}, []string{".ddev/config.yaml", "vendor/bin/sail"}, "", []string{"--json"}, nil},
		{"exit 1", veeGate("test", "exit 1"), []string{"test"}, []string{".ddev/config.yaml"}, "", nil, nil},
		{"exit 126", veeGate("test", "exit 126"), []string{"test"}, []string{".ddev/config.yaml"}, "", nil, nil},
		{"verde", veeGate("test", "true"), []string{"test"}, []string{".ddev/config.yaml"}, "", nil, nil},
		{"sin los archivos", veeGate("test", falta), []string{"test"}, nil, "", nil, nil},
		{"con un local.yaml sin ejecutar", veeGate("test", falta), []string{"test"}, []string{".ddev/config.yaml"}, "en_el_host: [test]\n", nil, nil},
		{"con un local.yaml con ejecutar vacio", veeGate("test", falta), []string{"test"}, []string{".ddev/config.yaml"}, "ejecutar:\n", nil, nil},
	} {
		dir := veeLaravel(t, c.gates, c.nombres...)
		for _, a := range c.archivos {
			veeEscribir(t, dir, a, "x\n", 0o755)
		}
		if len(c.archivos) > 0 {
			qcGit(t, dir, "add", "-A")
			qcGit(t, dir, "commit", "-q", "-m", "archivos del entorno")
		}
		if c.local != "" {
			veeEscribir(t, dir, hentorno.Ruta, c.local, 0o644)
		}
		r := qcCorrer(t, bin, dir, env, append([]string{"verify"}, c.args...)...)
		if c.nombre != "verde" && r.exit == 0 {
			t.Fatalf("CA-769: %s: fixture: el verify queda rojo: %s", c.nombre, r.todo())
		}
		veeExigeSugerencias(t, c.nombre, r, c.want...)
		if c.local == "" {
			if _, err := os.Lstat(filepath.Join(dir, ".hoom", "local.yaml")); err == nil {
				t.Fatalf("CA-769: %s: hoom verify nunca escribe .hoom/local.yaml", c.nombre)
			}
		}
	}

	// con php en el PATH, ninguno se cumple
	conPHP := veeSinPHP(t)
	veeEscribir(t, conPHP, "php", "#!/bin/sh\nexit 0\n", 0o755)
	dir := veeLaravel(t, veeGate("test", falta), "test")
	veeEscribir(t, dir, ".ddev/config.yaml", "x\n", 0o644)
	r := qcCorrer(t, bin, dir, map[string]string{"PATH": conPHP}, "verify")
	veeExigeSugerencias(t, "con php en el PATH", r)
}
