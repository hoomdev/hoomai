// Tests adversariales del spec .hoom/specs/quien-cierra-un-hallazgo.md
// (CA-438, y CA-435..CA-437 a traves del sobre): el gate de territorio. En la
// corrida de cualquier rol que no sea refutador, un
// .hoom/findings/<id>.res.json creado durante la corrida es una violacion de
// manipulacion ("un <rol> no cierra hallazgos: los cierra el refutador
// (refutado) o una persona"), la escriba el rol a mano o con la CLI. Un
// hallazgo nuevo sigue siendo trabajo legitimo, y en la corrida del refutador
// crear una resolucion no es violacion.
//
// Las fotos son las de verdad (Take sobre un repo real): el gate mira el
// arbol, no un mapa armado a mano. Los E2E del sobre compilan el binario de
// hoom (se omiten con -short) y usan un claude falso.
package agentcmd

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
)

// qcNoCierra es el detalle del contrato para un rol que no es refutador.
func qcNoCierra(rol string) string {
	return "un " + rol + " no cierra hallazgos: los cierra el refutador (refutado) o una persona"
}

// qcResRel es la ruta relativa (la de las violaciones) de la resolucion.
func qcResRel(id string) string { return ".hoom/findings/" + id + ".res.json" }

// qcResolucionAMano es el .res.json que un rol escribiria sin pasar por la
// CLI: con otro autor, como si lo hubiera firmado la persona.
func qcResolucionAMano(id, as string) string {
	return `{"finding_id":"` + id + `","as":"` + as + `","evidence":"es un duplicado",` +
		`"author":"Henry Orellana","resolved_at":"2026-09-30T12:00:00Z"}` + "\n"
}

// qcLimpiarEntorno deja el proceso sin HOOM_ROLE/HOOM_RUN/HOOM_PROVIDER/
// HOOM_TASK (los restaura al final): la suite puede correr dentro de la
// corrida de un rol.
func qcLimpiarEntorno(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER", "HOOM_TASK"} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

// qcRepoConHallazgo arma un proyecto con n hallazgos abiertos commiteados.
func qcRepoConHallazgo(t *testing.T, n int) (string, []string) {
	t.Helper()
	qcLimpiarEntorno(t)
	root := repo(t)
	var ids []string
	for i := 0; i < n; i++ {
		f, err := finding.Add(root, "main", "high", "risk", "app.go", "el retry no respeta el backoff", "reviewer@codex")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, f.ID)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "hallazgos abiertos")
	return root, ids
}

// qcViolacion devuelve la violacion en ruta, si la hay.
func qcViolacion(vs []Violation, ruta string) (Violation, bool) {
	for _, v := range vs {
		if v.Path == ruta {
			return v, true
		}
	}
	return Violation{}, false
}

// qcManipulacionDe exige la violacion de manipulacion del contrato en ruta.
func qcManipulacionDe(t *testing.T, caso, rol string, res ScopeResult, ruta string) {
	t.Helper()
	var v Violation
	encontrada := false
	for _, x := range res.Violations {
		if x.Path == ruta && x.Rule == RuleTampering {
			v, encontrada = x, true
			break
		}
	}
	if !encontrada {
		t.Fatalf("CA-438: %s: %s creado en la corrida de un %s es manipulacion: %+v", caso, ruta, rol, res.Violations)
	}
	if !strings.Contains(v.Detail, qcNoCierra(rol)) {
		t.Fatalf("CA-438: %s: el detalle dice %q: %q", caso, qcNoCierra(rol), v.Detail)
	}
	if !res.Tampering || res.OK {
		t.Fatalf("CA-438: %s: el gate queda marcado como manipulacion: %+v", caso, res)
	}
}

func qcAbierto(t *testing.T, ca, root, id string) {
	t.Helper()
	items, _, err := finding.List(root, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID == id {
			if it.Status != finding.StatusOpen || it.Resolution != nil {
				t.Fatalf("%s: el hallazgo %s sigue abierto: %+v", ca, id, it)
			}
			return
		}
	}
	t.Fatalf("%s: el hallazgo %s sigue en la lista", ca, id)
}

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

// qcComillas cita s como una palabra de shell entre comillas simples.
func qcComillas(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// ---------------------------------------------------------------- el gate

// CA-438: para CADA rol de la tabla que no es refutador, un .res.json creado
// durante la corrida (a mano, con otro autor) es manipulacion con el detalle
// del contrato, aunque el rol pueda escribir en .hoom/findings/.
func TestCA438_UnaResolucionNuevaEsManipulacionEnTodoRolQueNoEsRefutador(t *testing.T) {
	root, ids := qcRepoConHallazgo(t, 1)
	before := Take(root, "main")
	write(t, root, qcResRel(ids[0]), qcResolucionAMano(ids[0], finding.StatusRefuted))
	after := Take(root, "main")

	vistos := 0
	for _, r := range agents.Roles() {
		if r.Slug == "refutador" {
			continue
		}
		vistos++
		res := Gate(root, "main", "", r, before, after, PolicyFor(nil, r), nil)
		qcManipulacionDe(t, "rol "+r.Slug, r.Slug, res, qcResRel(ids[0]))
	}
	if vistos < 5 {
		t.Fatalf("CA-438: fixture: la tabla trae los roles de hoom, trajo %d sin el refutador", vistos)
	}
}

// CA-438 (propiedad): la regla es "cualquier rol que no sea refutador", no
// una lista: para todo slug distinto de refutador, aun uno que no esta en la
// tabla y con el territorio mas amplio (codigo), la resolucion nueva es
// manipulacion con el detalle del contrato que nombra ese slug.
func TestCA438_PropiedadTodoSlugQueNoEsRefutadorNoCierra(t *testing.T) {
	root, ids := qcRepoConHallazgo(t, 1)
	before := Take(root, "main")
	write(t, root, qcResRel(ids[0]), qcResolucionAMano(ids[0], finding.StatusRefuted))
	after := Take(root, "main")

	const letras = "abcdefghijklmnopqrstuvwxyz-"
	prop := func(semilla []byte) bool {
		var b strings.Builder
		b.WriteByte('a' + semilla0(semilla)%26)
		for i, c := range semilla {
			if i >= 20 {
				break
			}
			b.WriteByte(letras[int(c)%len(letras)])
		}
		slug := b.String()
		if slug == "refutador" {
			return true
		}
		r := agents.Role{Slug: slug, Scope: agents.ScopeCodigo}
		res := Gate(root, "main", "", r, before, after, PolicyFor(nil, r), nil)
		for _, v := range res.Violations {
			if v.Path == qcResRel(ids[0]) && v.Rule == RuleTampering && strings.Contains(v.Detail, qcNoCierra(slug)) {
				return res.Tampering && !res.OK
			}
		}
		t.Logf("CA-438: rol %q: %+v", slug, res.Violations)
		return false
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 40}); err != nil {
		t.Fatalf("CA-438: una resolucion nueva es manipulacion para todo rol que no es refutador: %v", err)
	}
}

func semilla0(s []byte) byte {
	if len(s) == 0 {
		return 0
	}
	return s[0]
}

// CA-438 (guarda, caso limite del spec): borrar o modificar un .res.json que
// ya existia es manipulacion hoy, y lo sigue siendo, tambien para el
// refutador (la evidencia es append-only).
func TestCA438_ModificarOBorrarUnaResolucionExistenteSigueSiendoManipulacion(t *testing.T) {
	for _, slug := range []string{"reviewer", "refutador"} {
		t.Run(slug, func(t *testing.T) {
			r, err := agents.Lookup(slug)
			if err != nil {
				t.Fatal(err)
			}
			root, ids := qcRepoConHallazgo(t, 2)
			write(t, root, qcResRel(ids[0]), qcResolucionAMano(ids[0], finding.StatusRefuted))
			write(t, root, qcResRel(ids[1]), qcResolucionAMano(ids[1], finding.StatusRefuted))
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "resoluciones previas")

			before := Take(root, "main")
			write(t, root, qcResRel(ids[0]), qcResolucionAMano(ids[0], finding.StatusCorrected))
			if err := os.Remove(filepath.Join(root, qcResRel(ids[1]))); err != nil {
				t.Fatal(err)
			}
			res := Gate(root, "main", "", r, before, Take(root, "main"), PolicyFor(nil, r), nil)
			for _, id := range ids {
				v, ok := qcViolacion(res.Violations, qcResRel(id))
				if !ok || v.Rule != RuleTampering {
					t.Fatalf("CA-438: %s: modificar o borrar %s es manipulacion: %+v", slug, qcResRel(id), res.Violations)
				}
			}
			if !res.Tampering || res.OK {
				t.Fatalf("CA-438: %s: el gate queda marcado como manipulacion: %+v", slug, res)
			}
		})
	}
}

// CA-438: el archivo cuenta, no lo que dice: una resolucion "corregido", o la
// de un hallazgo que no existe, tambien es manipulacion en la corrida del
// reviewer.
func TestCA438_CualquierResolucionNuevaCuentaAunqueDigaOtraCosa(t *testing.T) {
	rev, err := agents.Lookup("reviewer")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ caso, id, as string }{
		{"corregido", "", finding.StatusCorrected},
		{"huerfana", "20260101T000000_000000", finding.StatusRefuted},
		{"basura", "", "cualquier-cosa"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, ids := qcRepoConHallazgo(t, 1)
			id := c.id
			if id == "" {
				id = ids[0]
			}
			before := Take(root, "main")
			write(t, root, qcResRel(id), qcResolucionAMano(id, c.as))
			res := Gate(root, "main", "", rev, before, Take(root, "main"), PolicyFor(nil, rev), nil)
			qcManipulacionDe(t, c.caso, "reviewer", res, qcResRel(id))
		})
	}
}

// CA-438: un hallazgo nuevo sigue siendo trabajo legitimo; en la misma
// corrida, solo la resolucion nueva es violacion. Una resolucion que ya
// estaba antes de la corrida, intacta, no cuenta.
func TestCA438_ElHallazgoNuevoSigueSiendoLegitimo(t *testing.T) {
	for _, slug := range []string{"reviewer", "writer"} {
		t.Run(slug, func(t *testing.T) {
			r, err := agents.Lookup(slug)
			if err != nil {
				t.Fatal(err)
			}
			root, ids := qcRepoConHallazgo(t, 2)
			// una resolucion previa, commiteada: historia, no obra de esta corrida
			write(t, root, qcResRel(ids[0]), qcResolucionAMano(ids[0], finding.StatusRefuted))
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "resolucion previa")

			// control: solo un hallazgo nuevo
			before := Take(root, "main")
			nuevo, err := finding.Add(root, "main", "medium", "risk", "app.go", "otro hallazgo", slug+"@codex")
			if err != nil {
				t.Fatal(err)
			}
			res := Gate(root, "main", "", r, before, Take(root, "main"), PolicyFor(nil, r), nil)
			if len(res.Violations) != 0 || !res.OK || res.Tampering {
				t.Fatalf("CA-438: un hallazgo nuevo (%s) sigue siendo legitimo para el %s: %+v", nuevo.ID, slug, res)
			}

			// hallazgo nuevo + resolucion nueva: solo la resolucion
			before = Take(root, "main")
			otro, err := finding.Add(root, "main", "low", "readability", "app.go", "un tercero", slug+"@codex")
			if err != nil {
				t.Fatal(err)
			}
			write(t, root, qcResRel(ids[1]), qcResolucionAMano(ids[1], finding.StatusRefuted))
			res = Gate(root, "main", "", r, before, Take(root, "main"), PolicyFor(nil, r), nil)
			qcManipulacionDe(t, "hallazgo y resolucion", slug, res, qcResRel(ids[1]))
			if v, ok := qcViolacion(res.Violations, ".hoom/findings/"+otro.ID+".json"); ok {
				t.Fatalf("CA-438: el hallazgo nuevo no es violacion: %+v", v)
			}
			if v, ok := qcViolacion(res.Violations, qcResRel(ids[0])); ok {
				t.Fatalf("CA-438: la resolucion previa e intacta no es obra de la corrida: %+v", v)
			}
		})
	}
}

// CA-438: en la corrida del refutador, crear una resolucion no es violacion,
// ni aunque diga corregido (la CLI lo niega; el gate no), ni junto a un
// hallazgo nuevo.
func TestCA438_EnLaCorridaDelRefutadorCrearUnaResolucionNoEsViolacion(t *testing.T) {
	ref, err := agents.Lookup("refutador")
	if err != nil {
		t.Fatal(err)
	}
	root, ids := qcRepoConHallazgo(t, 2)
	before := Take(root, "main")
	write(t, root, qcResRel(ids[0]), qcResolucionAMano(ids[0], finding.StatusRefuted))
	write(t, root, qcResRel(ids[1]), qcResolucionAMano(ids[1], finding.StatusCorrected))
	if _, err := finding.Add(root, "main", "low", "risk", "app.go", "de paso", "refutador@codex"); err != nil {
		t.Fatal(err)
	}
	res := Gate(root, "main", "", ref, before, Take(root, "main"), PolicyFor(nil, ref), nil)
	if len(res.Violations) != 0 || !res.OK || res.Tampering {
		t.Fatalf("CA-438: el refutador puede crear resoluciones: %+v", res)
	}
}

// ---------------------------------------------------------------- el sobre

// CA-438: 'hoom agent --role writer' con un provider que escribe la
// resolucion a mano: manipulacion en el gate, el sobre corta en scope, exit
// 1, sin veredicto.
func TestCA438_ElSobreDelWriterQueEscribeUnaResolucionEsManipulacion(t *testing.T) {
	root, ids := qcRepoConHallazgo(t, 1)
	fakeProvider(t, "claude", "cat > "+qcComillas(qcResRel(ids[0]))+" <<'EOF'\n"+
		qcResolucionAMano(ids[0], finding.StatusRefuted)+"EOF\nexit 0\n")

	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	qcManipulacionDe(t, "sobre del writer", "writer", res.Scope, qcResRel(ids[0]))
	if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status == "entregable" {
		t.Fatalf("CA-438: la manipulacion corta el sobre en scope, exit 1, sin veredicto: %+v", res)
	}
}

// CA-436 y CA-435 dentro del sobre: el writer corre el 'hoom finding
// resolve' real; el sobre le puso HOOM_ROLE=writer, asi que la CLI se niega,
// no queda .res.json y el hallazgo sigue abierto.
func TestCA436_ElWriterDentroDelSobreNoCierraConLaCLI(t *testing.T) {
	bin := qcHoom(t)
	root, ids := qcRepoConHallazgo(t, 1)
	salida := filepath.Join(t.TempDir(), "resolve.txt")
	fakeProvider(t, "claude", qcComillas(bin)+" finding resolve "+ids[0]+
		" --as corregido --evidence 'commit abc con el gate verde' > "+qcComillas(salida)+" 2>&1\n"+
		"echo \"exit=$?\" >> "+qcComillas(salida)+"\nexit 0\n")

	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(salida)
	if err != nil {
		t.Fatalf("CA-436: el provider falso corrio la CLI: %v", err)
	}
	if !strings.Contains(string(raw), "hoom finding: "+qcNoCierra("writer")) || !strings.Contains(string(raw), "exit=1") {
		t.Fatalf("CA-436: dentro del sobre del writer resolve se niega con exit 1 y %q:\n%s", qcNoCierra("writer"), raw)
	}
	if _, err := os.Stat(filepath.Join(root, qcResRel(ids[0]))); err == nil {
		t.Fatalf("CA-436: la negativa no escribe %s", qcResRel(ids[0]))
	}
	qcAbierto(t, "CA-436", root, ids[0])
	if v, ok := qcViolacion(res.Scope.Violations, qcResRel(ids[0])); ok {
		t.Fatalf("CA-436: sin archivo no hay violacion que marcar: %+v", v)
	}
}

// CA-438: el writer que se saca el rol del entorno (HOOM_ROLE vacia) engana
// a la CLI, que escribe la resolucion como fuera de una corrida; el gate la
// marca igual.
func TestCA438_ElWriterQueSeSacaElRolYUsaLaCLITambienEsManipulacion(t *testing.T) {
	bin := qcHoom(t)
	root, ids := qcRepoConHallazgo(t, 1)
	fakeProvider(t, "claude", "HOOM_ROLE= "+qcComillas(bin)+" finding resolve "+ids[0]+
		" --as refutado --evidence 'es un duplicado' --author 'Henry Orellana' >/dev/null 2>&1\nexit 0\n")

	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, qcResRel(ids[0]))); err != nil {
		t.Fatalf("CA-438: fixture: con HOOM_ROLE vacia la CLI escribe la resolucion: %v", err)
	}
	qcManipulacionDe(t, "writer con la CLI y sin rol", "writer", res.Scope, qcResRel(ids[0]))
	if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" {
		t.Fatalf("CA-438: la manipulacion corta el sobre en scope, exit 1, sin veredicto: %+v", res)
	}
}

// CA-435, CA-437 y CA-438 dentro del sobre: 'hoom agent --role refutador'.
// El provider ve HOOM_ROLE=refutador, HOOM_RUN=<el id del run> y
// HOOM_PROVIDER=claude; corre el 'hoom finding resolve --as refutado' real
// sin --author; la resolucion la firma hoom con esa corrida; y el gate no la
// marca.
func TestCA437_ElRefutadorDentroDelSobreRefutaFirmadoPorSuCorrida(t *testing.T) {
	bin := qcHoom(t)
	root, ids := qcRepoConHallazgo(t, 1)
	t.Setenv("HOOM_ROLE", "rol-ajeno")
	t.Setenv("HOOM_RUN", "run-ajeno")
	t.Setenv("HOOM_PROVIDER", "provider-ajeno")
	entorno := filepath.Join(t.TempDir(), "entorno.txt")
	fakeProvider(t, "claude", "printf '%s|%s|%s' \"$HOOM_ROLE\" \"$HOOM_RUN\" \"$HOOM_PROVIDER\" > "+qcComillas(entorno)+"\n"+
		qcComillas(bin)+" finding resolve "+ids[0]+" --as refutado --evidence 'TestRetry cubre el backoff y pasa' >/dev/null 2>&1\n"+
		"exit 0\n")

	res, err := Run(root, "main", Options{Role: "refutador", Prompt: "refuta los hallazgos abiertos"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.RunID == "" {
		t.Fatalf("CA-435: fixture: el sobre corrio un run: %+v", res)
	}
	raw, err := os.ReadFile(entorno)
	if err != nil {
		t.Fatalf("CA-435: el provider falso dejo su entorno: %v", err)
	}
	if string(raw) != "refutador|"+res.RunID+"|claude" {
		t.Fatalf("CA-435: el provider del refutador ve HOOM_ROLE=refutador, HOOM_RUN=%s, HOOM_PROVIDER=claude: %q", res.RunID, raw)
	}

	cuerpo, err := os.ReadFile(filepath.Join(root, qcResRel(ids[0])))
	if err != nil {
		t.Fatalf("CA-437: el refutador refuta dentro del sobre: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(cuerpo, &m); err != nil {
		t.Fatalf("CA-437: la resolucion es JSON: %v", err)
	}
	firma := "refutador@claude (run " + res.RunID + ")"
	if m["author"] != firma || m["role"] != "refutador" || m["run"] != res.RunID || m["as"] != finding.StatusRefuted {
		t.Fatalf("CA-437: la resolucion lleva author %q, role refutador y run %s: %s", firma, res.RunID, cuerpo)
	}
	if v, ok := qcViolacion(res.Scope.Violations, qcResRel(ids[0])); ok || res.Scope.Tampering {
		t.Fatalf("CA-438: en la corrida del refutador crear una resolucion no es violacion: %+v %+v", v, res.Scope)
	}
}
