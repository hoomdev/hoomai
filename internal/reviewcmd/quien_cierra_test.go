// Tests adversariales del spec .hoom/specs/quien-cierra-un-hallazgo.md
// (CA-438, y CA-435/CA-436 dentro de una review): el reviewer registra
// hallazgos y nunca los cierra. Un .hoom/findings/<id>.res.json creado
// durante su pasada, a mano o con la CLI, es manipulacion y la review termina
// NO ENTREGABLE por territorio, sin registro; con el entorno que le pone la
// corrida (HOOM_ROLE=reviewer, HOOM_RUN, HOOM_PROVIDER), la CLI ya se niega.
// Un hallazgo nuevo sigue siendo el trabajo del reviewer.
//
// Es el caso que abrio el spec: en la review de base-de-la-review el
// reviewer (Codex) cerro dos hallazgos propios con 'finding resolve --as
// refutado' y quedaron firmados por la persona. Providers falsos en el PATH;
// los que corren la CLI compilan el binario (se omiten con -short).
package reviewcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/finding"
)

func qcNoCierra(rol string) string {
	return "un " + rol + " no cierra hallazgos: los cierra el refutador (refutado) o una persona"
}

func qcResRel(id string) string { return ".hoom/findings/" + id + ".res.json" }

func qcComillas(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// qcLimpiarEntorno deja el proceso sin HOOM_ROLE/HOOM_RUN/HOOM_PROVIDER/
// HOOM_TASK (los restaura al final).
func qcLimpiarEntorno(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER", "HOOM_TASK"} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

// qcRepoConHallazgo es repo() (un cambio de codigo commiteado en feature)
// con un hallazgo high abierto de antes de la review.
func qcRepoConHallazgo(t *testing.T) (string, string) {
	t.Helper()
	qcLimpiarEntorno(t)
	root := repo(t)
	f, err := finding.Add(root, "main", "high", "risk", "app.go", "Nuevo() no tiene test", "reviewer@codex")
	if err != nil {
		t.Fatal(err)
	}
	return root, f.ID
}

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

func qcRevisar(t *testing.T, root string) (Result, string) {
	t.Helper()
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Provider: "codex"}, &out)
	if err != nil {
		t.Fatalf("CA-438: la review no se rompe: %v\n%s", err, out.String())
	}
	if len(res.Passes) != 1 {
		t.Fatalf("CA-438: fixture: una pasada de risk: %+v\n%s", res, out.String())
	}
	return res, out.String()
}

// qcNoEntregablePorResolucion exige la review NO ENTREGABLE por territorio:
// la pasada con la manipulacion del contrato en la resolucion, exit 1, sin
// registro de review.
func qcNoEntregablePorResolucion(t *testing.T, caso, root, id string, res Result, out string) {
	t.Helper()
	p := res.Passes[0]
	var v agentcmd.Violation
	ok := false
	for _, x := range p.Scope.Violations {
		if x.Path == qcResRel(id) && x.Rule == agentcmd.RuleTampering {
			v, ok = x, true
		}
	}
	if !ok {
		t.Fatalf("CA-438: %s: la resolucion que creo el reviewer es manipulacion: %+v\n%s", caso, p.Scope, out)
	}
	if !strings.Contains(v.Detail, qcNoCierra("reviewer")) {
		t.Fatalf("CA-438: %s: el detalle dice %q: %q", caso, qcNoCierra("reviewer"), v.Detail)
	}
	if p.Scope.OK || !p.Scope.Tampering {
		t.Fatalf("CA-438: %s: el territorio del reviewer queda roto por manipulacion: %+v", caso, p.Scope)
	}
	if res.Status != "no-entregable" || res.ExitCode != 1 {
		t.Fatalf("CA-438: %s: la review termina NO ENTREGABLE por territorio, exit 1: %s, exit %d\n%s", caso, res.Status, res.ExitCode, out)
	}
	if res.RecordID != "" || rdRegistrosEn(root) != 0 {
		t.Fatalf("CA-438: %s: una review no entregable no deja registro: %q, %d", caso, res.RecordID, rdRegistrosEn(root))
	}
}

// CA-438: el reviewer escribe a mano un .res.json (con otro autor, sin la
// CLI): la review es NO ENTREGABLE por territorio, sin registro.
func TestCA438_ElReviewerQueEscribeUnaResolucionAManoEsNoEntregable(t *testing.T) {
	root, id := qcRepoConHallazgo(t)
	fakeProvider(t, "codex", "cat > "+qcComillas(qcResRel(id))+" <<'EOF'\n"+
		`{"finding_id":"`+id+`","as":"refutado","evidence":"es un duplicado mal escrito",`+
		`"author":"Henry Orellana","resolved_at":"2026-09-30T12:00:00Z"}`+"\nEOF\nexit 0\n")

	res, out := qcRevisar(t, root)
	qcNoEntregablePorResolucion(t, "a mano", root, id, res, out)
}

// CA-438: el reviewer se saca el rol del entorno (HOOM_ROLE vacia) y la CLI
// escribe la resolucion como fuera de una corrida: el gate la marca igual.
func TestCA438_ElReviewerQueSeSacaElRolYUsaLaCLIEsNoEntregable(t *testing.T) {
	bin := qcHoom(t)
	root, id := qcRepoConHallazgo(t)
	fakeProvider(t, "codex", "HOOM_ROLE= "+qcComillas(bin)+" finding resolve "+id+
		" --as refutado --evidence 'es un duplicado mal escrito' --author 'Henry Orellana' >/dev/null 2>&1\nexit 0\n")

	res, out := qcRevisar(t, root)
	if _, err := os.Stat(filepath.Join(root, qcResRel(id))); err != nil {
		t.Fatalf("CA-438: fixture: con HOOM_ROLE vacia la CLI escribe la resolucion: %v", err)
	}
	qcNoEntregablePorResolucion(t, "con la CLI sin rol", root, id, res, out)
}

// CA-436 y CA-435 dentro de una review: el reviewer corre el 'hoom finding
// resolve --as refutado' real, como en base-de-la-review. Su provider ve
// HOOM_ROLE=reviewer, HOOM_RUN=<el run de la pasada> y HOOM_PROVIDER=codex
// (no los del padre), la CLI se niega con exit 1, no queda .res.json, el
// hallazgo sigue abierto y la review, sin nada que marcar, termina revisada.
func TestCA436_ElReviewerDentroDeLaReviewNoCierraConLaCLI(t *testing.T) {
	bin := qcHoom(t)
	root, id := qcRepoConHallazgo(t)
	t.Setenv("HOOM_ROLE", "refutador") // el padre no le presta su rol
	t.Setenv("HOOM_RUN", "run-ajeno")
	t.Setenv("HOOM_PROVIDER", "provider-ajeno")
	rastro := t.TempDir()
	entorno := filepath.Join(rastro, "entorno.txt")
	salida := filepath.Join(rastro, "resolve.txt")
	fakeProvider(t, "codex", "printf '%s|%s|%s' \"$HOOM_ROLE\" \"$HOOM_RUN\" \"$HOOM_PROVIDER\" > "+qcComillas(entorno)+"\n"+
		qcComillas(bin)+" finding resolve "+id+" --as refutado --evidence 'es un duplicado mal escrito' > "+qcComillas(salida)+" 2>&1\n"+
		"echo \"exit=$?\" >> "+qcComillas(salida)+"\nexit 0\n")

	res, out := qcRevisar(t, root)
	raw, err := os.ReadFile(entorno)
	if err != nil {
		t.Fatalf("CA-435: el reviewer falso dejo su entorno: %v", err)
	}
	if res.Passes[0].RunID == "" || string(raw) != "reviewer|"+res.Passes[0].RunID+"|codex" {
		t.Fatalf("CA-435: el reviewer ve HOOM_ROLE=reviewer, HOOM_RUN=%s, HOOM_PROVIDER=codex: %q", res.Passes[0].RunID, raw)
	}
	dijo, err := os.ReadFile(salida)
	if err != nil {
		t.Fatalf("CA-436: el reviewer falso corrio la CLI: %v", err)
	}
	if !strings.Contains(string(dijo), "hoom finding: "+qcNoCierra("reviewer")) || !strings.Contains(string(dijo), "exit=1") {
		t.Fatalf("CA-436: dentro de la review resolve se niega con exit 1 y %q:\n%s", qcNoCierra("reviewer"), dijo)
	}
	if _, err := os.Stat(filepath.Join(root, qcResRel(id))); err == nil {
		t.Fatalf("CA-436: la negativa no escribe %s", qcResRel(id))
	}
	items, _, err := finding.List(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	abierto := false
	for _, it := range items {
		if it.ID == id && it.Status == finding.StatusOpen {
			abierto = true
		}
	}
	if !abierto {
		t.Fatalf("CA-436: el hallazgo %s sigue abierto: %+v", id, items)
	}
	if !res.Passes[0].Scope.OK || res.Status != "revisado" || res.ExitCode != 0 {
		t.Fatalf("CA-436: sin resolucion no hay nada que marcar: la review termina revisada: %s, exit %d, %+v\n%s",
			res.Status, res.ExitCode, res.Passes[0].Scope, out)
	}
}

// CA-438 (control): registrar un hallazgo nuevo sigue siendo el trabajo del
// reviewer: revisado, con el hallazgo, sin violacion.
func TestCA438_ElReviewerQueRegistraUnHallazgoSigueRevisado(t *testing.T) {
	root, _ := qcRepoConHallazgo(t)
	nuevo := "20260930T120000_c0ffee"
	fakeProvider(t, "codex", hallazgoFalso(root, nuevo, "reviewer@codex")+"exit 0\n")

	res, out := qcRevisar(t, root)
	if !res.Passes[0].Scope.OK || len(res.Passes[0].Scope.Violations) != 0 {
		t.Fatalf("CA-438: un hallazgo nuevo no es violacion: %+v\n%s", res.Passes[0].Scope, out)
	}
	if res.Status != "revisado" || res.ExitCode != 0 || len(res.Findings) != 1 || res.Findings[0] != nuevo {
		t.Fatalf("CA-438: la review termina revisada con el hallazgo del reviewer: %+v\n%s", res, out)
	}
}
