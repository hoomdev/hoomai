// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// el piso del sobre (CA-626): un rol, cualquiera y el refutador incluido,
// que crea, cambia o borra un .hoom/findings/<id>.raise.json viola la regla
// manipulacion con el texto `un <rol> no sube hallazgos: lo hace una
// persona`, una sola violacion por archivo; el sobre corta en scope, sin
// veredicto y con un hallazgo high. Fotos de verdad (Take sobre un repo
// real), como quien_cierra_test.go. Escritos desde el spec y las firmas
// publicas, sin leer la implementacion.
package agentcmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
)

func crNoSube(rol string) string { return "un " + rol + " no sube hallazgos: lo hace una persona" }

func crRaiseRel(id string) string { return ".hoom/findings/" + id + ".raise.json" }

func crRaiseAMano(id, reason string) string {
	return `{"finding_id":"` + id + `","reason":"` + reason + `","author":"Henry Orellana","raised_at":"2026-11-01T12:00:00Z"}` + "\n"
}

// crSubidaEsManipulacion exige una sola violacion de manipulacion en ruta,
// con el texto del contrato, y el gate marcado como manipulacion.
func crSubidaEsManipulacion(t *testing.T, caso, rol string, res ScopeResult, ruta string) {
	t.Helper()
	n := 0
	for _, v := range res.Violations {
		if v.Path != ruta {
			continue
		}
		n++
		if v.Rule != RuleTampering || !strings.Contains(v.Detail, crNoSube(rol)) {
			t.Fatalf("CA-626: %s: %s es manipulacion con %q: %+v", caso, ruta, crNoSube(rol), v)
		}
	}
	if n != 1 {
		t.Fatalf("CA-626: %s: una sola violacion por archivo en %s: %+v", caso, ruta, res.Violations)
	}
	if !res.Tampering || res.OK {
		t.Fatalf("CA-626: %s: el gate queda marcado como manipulacion: %+v", caso, res)
	}
}

// CA-626: crear una subida es manipulacion para todo rol de la tabla, el
// refutador incluido.
func TestCA626_CrearUnaSubidaEsManipulacionEnTodoRol(t *testing.T) {
	root, ids := qcRepoConHallazgo(t, 1)
	before := Take(root, "main")
	write(t, root, crRaiseRel(ids[0]), crRaiseAMano(ids[0], "toca el cobro"))
	after := Take(root, "main")
	vistos, conRefutador := 0, false
	for _, r := range agents.Roles() {
		vistos++
		conRefutador = conRefutador || r.Slug == "refutador"
		res := Gate(root, "main", "", r, before, after, PolicyFor(nil, r), nil)
		crSubidaEsManipulacion(t, "rol "+r.Slug, r.Slug, res, crRaiseRel(ids[0]))
	}
	if vistos < 6 || !conRefutador {
		t.Fatalf("CA-626: fixture: la tabla trae los roles de hoom con el refutador: %d", vistos)
	}
}

// CA-626: cambiar o borrar una subida que ya estaba tambien lo es; tambien
// una subida invalida (de un hallazgo que no existe): el archivo cuenta, no
// lo que dice.
func TestCA626_CambiarOBorrarUnaSubidaEsManipulacion(t *testing.T) {
	for _, slug := range []string{"writer", "reviewer", "refutador"} {
		t.Run(slug, func(t *testing.T) {
			r, err := agents.Lookup(slug)
			if err != nil {
				t.Fatal(err)
			}
			root, ids := qcRepoConHallazgo(t, 2)
			write(t, root, crRaiseRel(ids[0]), crRaiseAMano(ids[0], "razon"))
			write(t, root, crRaiseRel(ids[1]), crRaiseAMano(ids[1], "razon"))
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "subidas previas")

			before := Take(root, "main")
			write(t, root, crRaiseRel(ids[0]), crRaiseAMano(ids[0], "otra razon"))
			if err := os.Remove(filepath.Join(root, crRaiseRel(ids[1]))); err != nil {
				t.Fatal(err)
			}
			huerfana := "20260101T000000_dddddd"
			write(t, root, crRaiseRel(huerfana), crRaiseAMano(huerfana, "x"))
			res := Gate(root, "main", "", r, before, Take(root, "main"), PolicyFor(nil, r), nil)
			for _, ruta := range []string{crRaiseRel(ids[0]), crRaiseRel(ids[1]), crRaiseRel(huerfana)} {
				crSubidaEsManipulacion(t, slug, slug, res, ruta)
			}
		})
	}
}

// CA-626 (guarda): un hallazgo nuevo sigue siendo trabajo legitimo del
// refutador; la regla es de .raise.json, no de .hoom/findings/.
func TestCA626_UnHallazgoNuevoNoEsSubida(t *testing.T) {
	ref, err := agents.Lookup("refutador")
	if err != nil {
		t.Fatal(err)
	}
	root, _ := qcRepoConHallazgo(t, 1)
	before := Take(root, "main")
	if _, err := finding.Add(root, "main", "low", "risk", "app.go", "de paso", "refutador@codex"); err != nil {
		t.Fatal(err)
	}
	res := Gate(root, "main", "", ref, before, Take(root, "main"), PolicyFor(nil, ref), nil)
	for _, v := range res.Violations {
		if strings.Contains(v.Detail, "no sube hallazgos") {
			t.Fatalf("CA-626: un hallazgo nuevo no es una subida: %+v", res.Violations)
		}
	}
}

// CA-626: el sobre del writer cuyo provider escribe una subida a mano corta
// en scope, exit 1, sin veredicto, y queda un hallazgo high con el texto.
func TestCA626_ElSobreQueEscribeUnaSubidaCortaEnScope(t *testing.T) {
	root, ids := qcRepoConHallazgo(t, 1)
	fakeProvider(t, "claude", "cat > "+qcComillas(crRaiseRel(ids[0]))+" <<'EOF'\n"+
		crRaiseAMano(ids[0], "toca el cobro")+"EOF\nexit 0\n")
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	crSubidaEsManipulacion(t, "sobre del writer", "writer", res.Scope, crRaiseRel(ids[0]))
	if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status == "entregable" {
		t.Fatalf("CA-626: la manipulacion corta el sobre en scope, exit 1, sin veredicto: %+v", res)
	}
	v, _ := qcViolacion(res.Scope.Violations, crRaiseRel(ids[0]))
	if v.FindingID == "" {
		t.Fatalf("CA-626: la manipulacion registra su hallazgo high del gate: %+v", v)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "findings", v.FindingID+".json"))
	if err != nil || !strings.Contains(string(raw), `"high"`) {
		t.Fatalf("CA-626: el hallazgo del gate %s es high y quedo en .hoom/findings/: %v\n%s", v.FindingID, err, raw)
	}
}
