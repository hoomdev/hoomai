// Tests adversariales del spec .hoom/specs/quien-cierra-un-hallazgo.md,
// ronda 1 de su review (CA-438, "por la CLI o a mano"), dentro de 'hoom
// review'. El reviewer registra hallazgos y nunca los cierra; su pasada
// termina NO ENTREGABLE por territorio si crea un .res.json, y eso vale:
//
//  1. aunque Git no liste la resolucion: el reviewer que crea un
//     .hoom/findings/.gitignore con *.res.json (o escribe la regla en
//     .git/info/exclude) antes de escribirla no escapa del gate;
//  2. aunque su run falle: si cierra un hallazgo y su provider sale con
//     error, la pasada igual trae la manipulacion, el gate registra su
//     hallazgo high (que sigue bloqueando findings_open) y la review queda
//     no-entregable sin registro.
//
// Providers falsos en el PATH; los que corren la CLI compilan hoom (se
// omiten con -short). Las reviews corren con reloj.
package reviewcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// qcr1Reloj es cuanto puede tardar una review con un reviewer falso.
const qcr1Reloj = 2 * time.Minute

// qcr1Ignorado dice si Git ignora rel en root (git check-ignore).
func qcr1Ignorado(root, rel string) bool {
	cmd := exec.Command("git", "check-ignore", "-q", "--no-index", rel)
	cmd.Dir = root
	return cmd.Run() == nil
}

// qcr1AMano es el trozo de script con el que el reviewer escribe la
// resolucion sin la CLI, firmada por la persona.
func qcr1AMano(id string) string {
	return "mkdir -p .hoom/findings\ncat > " + qcComillas(qcResRel(id)) + " <<'EOF'\n" +
		`{"finding_id":"` + id + `","as":"refutado","evidence":"es un duplicado mal escrito",` +
		`"author":"Henry Orellana","resolved_at":"2026-09-30T12:00:00Z"}` + "\nEOF\n"
}

// qcr1HallazgoDelGate devuelve el id del hallazgo que el gate registro por
// la manipulacion en ruta, o por que no esta (abierto y high).
func qcr1HallazgoDelGate(root string, sc agentcmd.ScopeResult, ruta string) (string, string) {
	var v agentcmd.Violation
	ok := false
	for _, x := range sc.Violations {
		if x.Path == ruta && x.Rule == agentcmd.RuleTampering {
			v, ok = x, true
		}
	}
	if !ok {
		return "", "no hay violacion de manipulacion en " + ruta
	}
	if v.FindingID == "" {
		return "", "la violacion en " + ruta + " registra su hallazgo (finding_id vacio)"
	}
	items, _, err := finding.List(root, "main", false)
	if err != nil {
		return "", "finding.List: " + err.Error()
	}
	for _, it := range items {
		if it.ID != v.FindingID {
			continue
		}
		if it.Severity != "high" || it.Status != finding.StatusOpen {
			return "", "el hallazgo del gate " + it.ID + " es high y queda abierto: " + it.Severity + "/" + it.Status
		}
		return it.ID, ""
	}
	return "", "el hallazgo del gate " + v.FindingID + " no quedo en .hoom/findings/"
}

// ---------------------------------------------------------------- 1. lo que Git no lista

// CA-438: el reviewer esconde las resoluciones de Git — un
// .hoom/findings/.gitignore con *.res.json, o la regla en .git/info/exclude —
// y despues escribe a mano la que cierra el hallazgo high. La review termina
// NO ENTREGABLE por territorio, exit 1, sin registro, y el gate registra el
// hallazgo high de la manipulacion.
func TestCA438_ElReviewerQueEscondeLaResolucionDeGitEsNoEntregable(t *testing.T) {
	for _, c := range []struct{ caso, esconder string }{
		{".hoom/findings/.gitignore", "mkdir -p .hoom/findings\nprintf '*.res.json\\n' > .hoom/findings/.gitignore\n"},
		{"info/exclude", "ex=\"$(git rev-parse --git-path info/exclude)\"\nmkdir -p \"$(dirname \"$ex\")\"\n" +
			"printf '\\n%s\\n' '.hoom/findings/*.res.json' >> \"$ex\"\n"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, id := qcRepoConHallazgo(t)
			fakeProvider(t, "codex", c.esconder+qcr1AMano(id)+"exit 0\n")

			res, err, out := raRunConReloj(t, "CA-438", qcr1Reloj, root, Options{Lens: "risk", Provider: "codex"})
			if err != nil {
				t.Fatalf("CA-438: %s: la review no se rompe: %v\n%s", c.caso, err, out)
			}
			if _, err := os.Stat(filepath.Join(root, qcResRel(id))); err != nil {
				t.Fatalf("CA-438: fixture: el reviewer falso escribio la resolucion: %v", err)
			}
			if !qcr1Ignorado(root, qcResRel(id)) {
				t.Fatalf("CA-438: fixture: con %s Git ignora %s", c.caso, qcResRel(id))
			}
			if len(res.Passes) != 1 {
				t.Fatalf("CA-438: fixture: una pasada de risk: %+v\n%s", res, out)
			}
			qcNoEntregablePorResolucion(t, c.caso, root, id, res, out)
			if _, porque := qcr1HallazgoDelGate(root, res.Passes[0].Scope, qcResRel(id)); porque != "" {
				t.Fatalf("CA-438: %s: %s", c.caso, porque)
			}
		})
	}
}

// ---------------------------------------------------------------- 2. la pasada que falla

// CA-438: el reviewer cierra el hallazgo high (a mano, o con la CLI tras
// sacarse HOOM_ROLE) y su provider sale con 1. La pasada se mide igual: trae
// la manipulacion en la resolucion con el detalle del contrato (Scope no OK,
// Tampering), el gate registra su hallazgo high — que sigue bloqueando
// findings_open aunque la resolucion quede en disco — y la review queda
// no-entregable, distinta de 0, sin registro.
func TestCA438_ElReviewerQueCierraYFallaEsNoEntregablePorTerritorio(t *testing.T) {
	for _, c := range []struct {
		caso string
		cli  bool
	}{
		{"a mano", false},
		{"con la CLI sin rol", true},
	} {
		t.Run(c.caso, func(t *testing.T) {
			var cerrar string
			root, id := qcRepoConHallazgo(t)
			if c.cli {
				bin := qcHoom(t)
				cerrar = "HOOM_ROLE= " + qcComillas(bin) + " finding resolve " + id +
					" --as refutado --evidence 'es un duplicado mal escrito' --author 'Henry Orellana' >/dev/null 2>&1\n"
			} else {
				cerrar = qcr1AMano(id)
			}
			fakeProvider(t, "codex", cerrar+"echo 'se corto la sesion' >&2\nexit 1\n")

			res, err, out := raRunConReloj(t, "CA-438", qcr1Reloj, root, Options{Lens: "risk", Provider: "codex"})
			if err != nil {
				t.Fatalf("CA-438: %s: la review no se rompe: %v\n%s", c.caso, err, out)
			}
			if _, err := os.Stat(filepath.Join(root, qcResRel(id))); err != nil {
				t.Fatalf("CA-438: fixture: el reviewer falso cerro el hallazgo antes de fallar: %v", err)
			}
			if res.Status != "no-entregable" || res.ExitCode == 0 || res.RecordID != "" || rdRegistrosEn(root) != 0 {
				t.Errorf("CA-438: %s: la review queda no-entregable, distinta de 0, sin registro: %s, exit %d, %q, %d\n%s",
					c.caso, res.Status, res.ExitCode, res.RecordID, rdRegistrosEn(root), out)
			}
			if len(res.Passes) != 1 {
				t.Fatalf("CA-438: %s: la pasada que fallo queda en el resultado: %+v\n%s", c.caso, res.Passes, out)
			}
			p := res.Passes[0]
			gid, porque := qcr1HallazgoDelGate(root, p.Scope, qcResRel(id))
			if porque != "" {
				t.Fatalf("CA-438: %s: la pasada que falla tambien se mide: %s (scope %+v, run %s)\n%s",
					c.caso, porque, p.Scope, p.RunStatus, out)
			}
			var detalle string
			for _, v := range p.Scope.Violations {
				if v.Path == qcResRel(id) {
					detalle = v.Detail
				}
			}
			if !strings.Contains(detalle, qcNoCierra("reviewer")) {
				t.Fatalf("CA-438: %s: el detalle dice %q: %q", c.caso, qcNoCierra("reviewer"), detalle)
			}
			if p.Scope.OK || !p.Scope.Tampering {
				t.Fatalf("CA-438: %s: el territorio del reviewer queda roto por manipulacion: %+v", c.caso, p.Scope)
			}
			// la resolucion queda en disco, pero el hallazgo del gate bloquea
			g := finding.Gate(root, "main", "high", "")
			if g.Status != verdict.StatusFail || !strings.Contains(g.Notes, gid) {
				t.Fatalf("CA-438: %s: %s sigue fallando por el hallazgo high del gate %s: %+v", c.caso, finding.GateName, gid, g)
			}
		})
	}
}
