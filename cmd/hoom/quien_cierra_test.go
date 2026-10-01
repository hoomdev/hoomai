// Tests adversariales del spec .hoom/specs/quien-cierra-un-hallazgo.md
// (CA-436, CA-437) contra el BINARIO: `hoom finding resolve` dentro y fuera
// de una corrida. Dentro de una corrida (HOOM_ROLE no vacio) solo el
// refutador cierra, solo como refutado, y el autor lo pone hoom
// (refutador@<provider> (run <id>)); la resolucion lleva role y run. Fuera de
// una corrida (HOOM_ROLE vacio o ausente) resolve es el de hoy y la
// resolucion no lleva role ni run.
//
// Cada invocacion arma su entorno a mano: HOOM_ROLE, HOOM_RUN, HOOM_PROVIDER
// y HOOM_TASK del proceso que corre la suite (por ejemplo, un rol de hoom que
// corre 'hoom verify') nunca se cuelan. Compila el binario: se omite con
// -short.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// qcRunID es un id de corrida con la forma de los de hoom.
const qcRunID = "20260930T120000_ab12cd"

// qcSinCorrida es el entorno de afuera de una corrida: sin ninguna HOOM_*.
var qcSinCorrida = map[string]string{}

// qcEnCorrida es el entorno que pone una corrida de rol en su provider.
func qcEnCorrida(rol, run, provider string) map[string]string {
	return map[string]string{"HOOM_ROLE": rol, "HOOM_RUN": run, "HOOM_PROVIDER": provider}
}

// qcNoCierra es el texto del contrato para un rol que no es refutador.
func qcNoCierra(rol string) string {
	return "hoom finding: un " + rol + " no cierra hallazgos: los cierra el refutador (refutado) o una persona"
}

const qcSoloRefuta = "hoom finding: el refutador solo refuta: corregido lo cierra el Orquestador o una persona, con el gate verde"

// qcFirma es el autor que pone hoom dentro de una corrida del refutador.
func qcFirma(provider, run string) string {
	return "refutador@" + provider + " (run " + run + ")"
}

func qcAutorAjeno(provider, run string) string {
	return "hoom finding: dentro de un run el autor lo pone hoom (" + qcFirma(provider, run) + ")"
}

type qcSalida struct {
	exit           int
	stdout, stderr string
}

func (s qcSalida) todo() string { return s.stdout + s.stderr }

// qcHoom compila el binario real de hoom.
func qcHoom(t *testing.T) string {
	t.Helper()
	if testing.Short() {
		t.Skip("compila el binario de hoom: se omite con -short")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("sin toolchain de Go en el PATH")
	}
	bin := filepath.Join(t.TempDir(), "hoom")
	cmd := exec.Command(goBin, "build", "-o", bin, "github.com/hoomdev/hoomai/cmd/hoom")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("no pude compilar hoom: %v\n%s", err, out)
	}
	return bin
}

// qcCorrer ejecuta el binario en dir con el entorno del proceso SIN ninguna
// HOOM_ROLE/HOOM_RUN/HOOM_PROVIDER/HOOM_TASK heredada, mas las de env (una
// clave con valor "" queda definida y vacia).
func qcCorrer(t *testing.T, bin, dir string, env map[string]string, args ...string) qcSalida {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "HOOM_ROLE=") || strings.HasPrefix(kv, "HOOM_RUN=") ||
			strings.HasPrefix(kv, "HOOM_PROVIDER=") || strings.HasPrefix(kv, "HOOM_TASK=") {
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no pude ejecutar hoom %v: %v", args, err)
	}
	return qcSalida{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

func qcGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// qcRepo arma un proyecto real con hoom.yaml y la identidad de git
// "Persona Git".
func qcRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	qcGit(t, dir, "init", "-q", "-b", "main")
	qcGit(t, dir, "config", "user.email", "persona@hoom.dev")
	qcGit(t, dir, "config", "user.name", "Persona Git")
	body := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	qcGit(t, dir, "add", "-A")
	qcGit(t, dir, "commit", "-q", "-m", "inicial")
	return dir
}

var qcIDRe = regexp.MustCompile(`registrado (\S+) \(`)

// qcHallazgo registra un hallazgo high con el binario, fuera de toda corrida.
func qcHallazgo(t *testing.T, bin, dir string) string {
	t.Helper()
	res := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "add", "--sev", "high", "--lens", "risk",
		"--file", "app.go", "--author", "reviewer@codex", "el retry no respeta el backoff")
	if res.exit != 0 {
		t.Fatalf("fixture: hoom finding add salio %d\n%s", res.exit, res.todo())
	}
	m := qcIDRe.FindStringSubmatch(res.stdout)
	if m == nil {
		t.Fatalf("fixture: no encontre el id en la salida de finding add:\n%s", res.stdout)
	}
	return m[1]
}

func qcResPath(dir, id string) string {
	return filepath.Join(dir, ".hoom", "findings", id+".res.json")
}

// qcSinResolucion exige que no exista el .res.json del hallazgo.
func qcSinResolucion(t *testing.T, ca, caso, dir, id string) {
	t.Helper()
	if raw, err := os.ReadFile(qcResPath(dir, id)); err == nil {
		t.Fatalf("%s: %s: una negativa no escribe nada, y quedo %s:\n%s", ca, caso, id+".res.json", raw)
	}
}

// qcResolucion lee el .res.json crudo del hallazgo.
func qcResolucion(t *testing.T, ca, dir, id string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(qcResPath(dir, id))
	if err != nil {
		t.Fatalf("%s: falta la resolucion %s.res.json: %v", ca, id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: la resolucion es JSON: %v\n%s", ca, err, raw)
	}
	return m
}

type qcItem struct {
	ID         string         `json:"id"`
	Status     string         `json:"status"`
	Resolution map[string]any `json:"resolution"`
}

// qcLista corre 'hoom finding list [--open] --json' fuera de una corrida.
func qcLista(t *testing.T, bin, dir string, open bool) []qcItem {
	t.Helper()
	args := []string{"finding", "list", "--json"}
	if open {
		args = []string{"finding", "list", "--open", "--json"}
	}
	res := qcCorrer(t, bin, dir, qcSinCorrida, args...)
	if res.exit != 0 {
		t.Fatalf("hoom %v salio %d\n%s", args, res.exit, res.todo())
	}
	var v struct {
		Warnings []string `json:"warnings"`
		Findings []qcItem `json:"findings"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &v); err != nil {
		t.Fatalf("hoom %v devuelve JSON: %v\n%s", args, err, res.stdout)
	}
	if len(v.Warnings) != 0 {
		t.Fatalf("hoom %v sin avisos: %v", args, v.Warnings)
	}
	return v.Findings
}

// qcSigueAbierto exige que el hallazgo este en 'finding list --open' y
// abierto.
func qcSigueAbierto(t *testing.T, ca, caso, bin, dir, id string) {
	t.Helper()
	for _, it := range qcLista(t, bin, dir, true) {
		if it.ID == id {
			if it.Status != "abierto" || it.Resolution != nil {
				t.Fatalf("%s: %s: el hallazgo sigue abierto: %+v", ca, caso, it)
			}
			return
		}
	}
	t.Fatalf("%s: %s: el hallazgo %s sigue en 'hoom finding list --open'", ca, caso, id)
}

// CA-436: con HOOM_ROLE de cualquier rol que no es refutador, resolve se
// niega con el texto del contrato, exit 1, sin .res.json, y el hallazgo sigue
// abierto. Da igual --as (refutado o corregido), que traiga --author, o que
// el rol sea el orquestador: dentro de una corrida nadie mas cierra.
func TestCA436_UnRolQueNoEsRefutadorNoCierraHallazgos(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	id := qcHallazgo(t, bin, dir)

	for _, rol := range []string{"reviewer", "writer", "test-writer", "orquestador", "scout", "arquitecto", "characterizer"} {
		for _, as := range []string{"refutado", "corregido"} {
			for _, autor := range []string{"", "Henry Orellana", rol + "@codex"} {
				caso := rol + "/" + as + "/autor=" + autor
				args := []string{"finding", "resolve", id, "--as", as, "--evidence", "TestRetry lo refuta"}
				if autor != "" {
					args = append(args, "--author", autor)
				}
				res := qcCorrer(t, bin, dir, qcEnCorrida(rol, qcRunID, "codex"), args...)
				if res.exit != 1 {
					t.Fatalf("CA-436: %s: un %s no cierra hallazgos: exit 1, salio %d\n%s", caso, rol, res.exit, res.todo())
				}
				if !strings.Contains(res.todo(), qcNoCierra(rol)) {
					t.Fatalf("CA-436: %s: la negativa dice %q:\n%s", caso, qcNoCierra(rol), res.todo())
				}
				qcSinResolucion(t, "CA-436", caso, dir, id)
			}
		}
	}
	qcSigueAbierto(t, "CA-436", "tras todas las negativas", bin, dir, id)

	// y la persona, fuera de la corrida, lo sigue pudiendo cerrar
	res := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "resolve", id, "--as", "refutado",
		"--evidence", "TestRetry lo refuta", "--author", "Henry Orellana")
	if res.exit != 0 {
		t.Fatalf("CA-436: fuera de una corrida la persona cierra: salio %d\n%s", res.exit, res.todo())
	}
}

// CA-436: un rol que no es de la tabla tambien es "un rol que no es
// refutador": HOOM_ROLE no vacio basta para negarse.
func TestCA436_CualquierHOOMROLENoVacioQueNoEsRefutadorSeNiega(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	id := qcHallazgo(t, bin, dir)

	for _, rol := range []string{"analista", "designer", "inventado"} {
		res := qcCorrer(t, bin, dir, qcEnCorrida(rol, qcRunID, "claude"),
			"finding", "resolve", id, "--as", "refutado", "--evidence", "TestRetry lo refuta")
		if res.exit != 1 || !strings.Contains(res.todo(), qcNoCierra(rol)) {
			t.Fatalf("CA-436: HOOM_ROLE=%s se niega con %q y exit 1: salio %d\n%s", rol, qcNoCierra(rol), res.exit, res.todo())
		}
		qcSinResolucion(t, "CA-436", rol, dir, id)
	}
	qcSigueAbierto(t, "CA-436", "roles fuera de la tabla", bin, dir, id)
}

// CA-436: la negativa no depende de que la corrida diga su id o su
// provider: con solo HOOM_ROLE=reviewer (HOOM_RUN y HOOM_PROVIDER ausentes)
// tampoco cierra.
func TestCA436_SoloConHOOMROLEYaSeNiega(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	id := qcHallazgo(t, bin, dir)

	res := qcCorrer(t, bin, dir, map[string]string{"HOOM_ROLE": "reviewer"},
		"finding", "resolve", id, "--as", "refutado", "--evidence", "TestRetry lo refuta")
	if res.exit != 1 || !strings.Contains(res.todo(), qcNoCierra("reviewer")) {
		t.Fatalf("CA-436: HOOM_ROLE=reviewer sin HOOM_RUN ni HOOM_PROVIDER se niega igual: salio %d\n%s", res.exit, res.todo())
	}
	qcSinResolucion(t, "CA-436", "solo HOOM_ROLE", dir, id)
	qcSigueAbierto(t, "CA-436", "solo HOOM_ROLE", bin, dir, id)
}

// CA-437: con HOOM_ROLE=refutador, --as refutado escribe la resolucion con
// author refutador@<provider> (run <id>), role refutador y run <id>, y el
// hallazgo queda refutado en 'finding list --json' con esos mismos datos.
func TestCA437_ElRefutadorRefutaYLaFirmaLaPoneHoom(t *testing.T) {
	bin := qcHoom(t)
	for _, prov := range []string{"codex", "claude"} {
		t.Run(prov, func(t *testing.T) {
			dir := qcRepo(t)
			id := qcHallazgo(t, bin, dir)
			res := qcCorrer(t, bin, dir, qcEnCorrida("refutador", qcRunID, prov),
				"finding", "resolve", id, "--as", "refutado", "--evidence", "TestRetry cubre el backoff y pasa")
			if res.exit != 0 {
				t.Fatalf("CA-437: el refutador refuta: exit 0, salio %d\n%s", res.exit, res.todo())
			}
			m := qcResolucion(t, "CA-437", dir, id)
			if m["author"] != qcFirma(prov, qcRunID) {
				t.Fatalf("CA-437: dentro de un run el autor es %q: %v", qcFirma(prov, qcRunID), m["author"])
			}
			if m["role"] != "refutador" || m["run"] != qcRunID {
				t.Fatalf("CA-437: la resolucion lleva role refutador y run %s: %v", qcRunID, m)
			}
			if m["as"] != "refutado" || m["evidence"] != "TestRetry cubre el backoff y pasa" || m["finding_id"] != id {
				t.Fatalf("CA-437: lo demas de la resolucion es el de hoy: %v", m)
			}
			if strings.Contains(m["author"].(string), "Persona Git") {
				t.Fatalf("CA-437: dentro de un run la resolucion no la firma la identidad de git: %v", m["author"])
			}

			var visto *qcItem
			todos := qcLista(t, bin, dir, false)
			for i := range todos {
				if todos[i].ID == id {
					visto = &todos[i]
				}
			}
			if visto == nil || visto.Status != "refutado" || visto.Resolution == nil {
				t.Fatalf("CA-437: 'finding list' lo ve refutado con su resolucion: %+v", visto)
			}
			if visto.Resolution["author"] != qcFirma(prov, qcRunID) || visto.Resolution["role"] != "refutador" ||
				visto.Resolution["run"] != qcRunID {
				t.Fatalf("CA-437: 'finding list --json' muestra author, role y run de la resolucion: %v", visto.Resolution)
			}
			for _, it := range qcLista(t, bin, dir, true) {
				if it.ID == id {
					t.Fatalf("CA-437: refutado, ya no esta en 'finding list --open': %+v", it)
				}
			}
		})
	}
}

// CA-437: un --author igual al que pone hoom no es "un --author distinto":
// se acepta, y la resolucion queda igual que sin --author.
func TestCA437_ElRefutadorConElAutorQuePoneHoomSeAcepta(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	id := qcHallazgo(t, bin, dir)

	res := qcCorrer(t, bin, dir, qcEnCorrida("refutador", qcRunID, "codex"),
		"finding", "resolve", id, "--as", "refutado", "--evidence", "TestRetry lo refuta",
		"--author", qcFirma("codex", qcRunID))
	if res.exit != 0 {
		t.Fatalf("CA-437: el --author que coincide con el de hoom no es distinto: exit 0, salio %d\n%s", res.exit, res.todo())
	}
	m := qcResolucion(t, "CA-437", dir, id)
	if m["author"] != qcFirma("codex", qcRunID) || m["role"] != "refutador" || m["run"] != qcRunID {
		t.Fatalf("CA-437: la resolucion lleva la firma, el rol y la corrida: %v", m)
	}
}

// CA-437: el refutador con --as corregido se niega con el texto del
// contrato, exit 1, sin escribir; el hallazgo sigue abierto.
func TestCA437_ElRefutadorSoloRefuta(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	id := qcHallazgo(t, bin, dir)

	for _, autor := range []string{"", qcFirma("codex", qcRunID)} {
		args := []string{"finding", "resolve", id, "--as", "corregido", "--evidence", "commit abc123 con el gate verde"}
		if autor != "" {
			args = append(args, "--author", autor)
		}
		res := qcCorrer(t, bin, dir, qcEnCorrida("refutador", qcRunID, "codex"), args...)
		if res.exit != 1 {
			t.Fatalf("CA-437: autor=%q: el refutador no cierra corregido: exit 1, salio %d\n%s", autor, res.exit, res.todo())
		}
		if !strings.Contains(res.todo(), qcSoloRefuta) {
			t.Fatalf("CA-437: autor=%q: la negativa dice %q:\n%s", autor, qcSoloRefuta, res.todo())
		}
		qcSinResolucion(t, "CA-437", "corregido autor="+autor, dir, id)
	}
	qcSigueAbierto(t, "CA-437", "refutador con corregido", bin, dir, id)
}

// CA-437: dentro de la corrida del refutador un --author distinto del que
// pone hoom se niega, sin escribir: ni la persona, ni otro rol, ni otra
// corrida u otro provider.
func TestCA437_DentroDeUnRunElAutorLoPoneHoom(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	id := qcHallazgo(t, bin, dir)

	for _, autor := range []string{
		"Henry Orellana",
		"Persona Git <persona@hoom.dev>",
		"reviewer@codex",
		"refutador@codex",
		"refutador@claude (run " + qcRunID + ")",
		"refutador@codex (run 20260101T000000_000000)",
	} {
		res := qcCorrer(t, bin, dir, qcEnCorrida("refutador", qcRunID, "codex"),
			"finding", "resolve", id, "--as", "refutado", "--evidence", "TestRetry lo refuta", "--author", autor)
		if res.exit != 1 {
			t.Fatalf("CA-437: --author %q distinto del de hoom se niega: exit 1, salio %d\n%s", autor, res.exit, res.todo())
		}
		if !strings.Contains(res.todo(), qcAutorAjeno("codex", qcRunID)) {
			t.Fatalf("CA-437: --author %q: la negativa dice %q:\n%s", autor, qcAutorAjeno("codex", qcRunID), res.todo())
		}
		qcSinResolucion(t, "CA-437", "autor "+autor, dir, id)
	}
	qcSigueAbierto(t, "CA-437", "autores distintos", bin, dir, id)
}

// CA-437 (guarda): sin HOOM_ROLE resolve es el de hoy: --author libre, o la
// identidad de git; corregido y refutado; y la resolucion no lleva las
// claves role ni run.
func TestCA437_SinHOOMROLEResolveEsElDeHoy(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)

	// --author libre, refutado
	a := qcHallazgo(t, bin, dir)
	res := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "resolve", a, "--as", "refutado",
		"--evidence", "TestRetry lo refuta", "--author", "Henry Orellana")
	if res.exit != 0 {
		t.Fatalf("CA-437: sin HOOM_ROLE refutar con --author funciona como hoy: salio %d\n%s", res.exit, res.todo())
	}
	m := qcResolucion(t, "CA-437", dir, a)
	if m["author"] != "Henry Orellana" {
		t.Fatalf("CA-437: sin HOOM_ROLE el autor es el de --author: %v", m["author"])
	}
	if _, ok := m["role"]; ok {
		t.Fatalf("CA-437: sin HOOM_ROLE la resolucion no lleva role: %v", m)
	}
	if _, ok := m["run"]; ok {
		t.Fatalf("CA-437: sin HOOM_ROLE la resolucion no lleva run: %v", m)
	}

	// sin --author: la identidad de git; corregido lo cierra la persona
	b := qcHallazgo(t, bin, dir)
	res = qcCorrer(t, bin, dir, qcSinCorrida, "finding", "resolve", b, "--as", "corregido",
		"--evidence", "commit abc123 con el gate verde")
	if res.exit != 0 {
		t.Fatalf("CA-437: sin HOOM_ROLE corregido funciona como hoy: salio %d\n%s", res.exit, res.todo())
	}
	m = qcResolucion(t, "CA-437", dir, b)
	if autor, _ := m["author"].(string); !strings.Contains(autor, "Persona Git") {
		t.Fatalf("CA-437: sin HOOM_ROLE ni --author el autor sale de la identidad de git: %v", m["author"])
	}
	if _, ok := m["role"]; ok {
		t.Fatalf("CA-437: sin HOOM_ROLE la resolucion no lleva role: %v", m)
	}
	if _, ok := m["run"]; ok {
		t.Fatalf("CA-437: sin HOOM_ROLE la resolucion no lleva run: %v", m)
	}
}

// CA-437 (guarda): HOOM_ROLE definida y VACIA es como ausente, aunque
// HOOM_RUN y HOOM_PROVIDER traigan valor (lo que ve algo lanzado por un
// 'hoom run' sin rol): resolve es el de hoy, sin role ni run.
func TestCA437_HOOMROLEVaciaEsComoHoy(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	env := map[string]string{"HOOM_ROLE": "", "HOOM_RUN": qcRunID, "HOOM_PROVIDER": "claude"}

	for _, c := range []struct{ as, autor string }{
		{"refutado", "Henry Orellana"},
		{"corregido", "Henry Orellana"},
	} {
		id := qcHallazgo(t, bin, dir)
		res := qcCorrer(t, bin, dir, env, "finding", "resolve", id, "--as", c.as,
			"--evidence", "la evidencia", "--author", c.autor)
		if res.exit != 0 {
			t.Fatalf("CA-437: HOOM_ROLE vacia: --as %s con --author funciona como hoy: salio %d\n%s", c.as, res.exit, res.todo())
		}
		m := qcResolucion(t, "CA-437", dir, id)
		if m["author"] != c.autor || m["as"] != c.as {
			t.Fatalf("CA-437: HOOM_ROLE vacia: la resolucion es la de hoy: %v", m)
		}
		if _, ok := m["role"]; ok {
			t.Fatalf("CA-437: HOOM_ROLE vacia: la resolucion no lleva role: %v", m)
		}
		if _, ok := m["run"]; ok {
			t.Fatalf("CA-437: HOOM_ROLE vacia: la resolucion no lleva run: %v", m)
		}
	}
}
