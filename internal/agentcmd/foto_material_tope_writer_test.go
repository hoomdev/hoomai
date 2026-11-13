package agentcmd

import (
	"bytes"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
)

// sinTopeDeFoto shrinks topeFotoMaterialGit so no photograph of the git
// material can end, until the test ends.
func sinTopeDeFoto(t *testing.T) {
	t.Helper()
	tope := topeFotoMaterialGit
	topeFotoMaterialGit = time.Nanosecond
	t.Cleanup(func() { topeFotoMaterialGit = tope })
}

// Finding 20261113T051843_192f73 (aclaracion 28): a photograph of the git
// material that does not end within its bound is FotoSinTerminar, and
// against a complete one it is one violation of RuleMaterialDeGit naming
// it, not one per path of the complete photograph.
func TestWriter192f73UnaFotoSinTerminarCortaContraUnaCompleta(t *testing.T) {
	_, wt := aeaProyecto(t, "true", "")
	antes := FotoMaterialGit(wt)
	if _, sin := antes[FotoSinTerminar]; sin || len(antes) < 4 {
		t.Fatalf("fixture: with its bound the photograph ends: %v", antes)
	}
	sinTopeDeFoto(t)
	if f := FotoMaterialGit(wt); len(f) != 1 || f[FotoSinTerminar] == "" {
		t.Fatalf("a photograph past its bound is FotoSinTerminar alone: %v", f)
	}
	sc := sinTocarMaterialGit(ScopeResult{OK: true}, wt, agents.Role{Scope: agents.ScopeCodigo}, antes)
	if sc.OK || len(sc.Violations) != 1 || sc.Violations[0].Path != FotoSinTerminar || sc.Violations[0].Rule != RuleMaterialDeGit {
		t.Fatalf("an unfinished photograph cuts as changed git material, once: %+v", sc)
	}
}

// Finding 20261113T051843_192f73 (aclaraciones 25 y 28): with no photograph
// of the git material able to end, a writer that only changes code cuts its
// envelope as one that changed it (fail closed): scope, no-entregable, exit
// 1, no verdict, and a high finding of the scope gate naming
// FotoSinTerminar.
func TestWriter192f73LaFotoQueNoTerminaCortaElSobre(t *testing.T) {
	sinTopeDeFoto(t)
	root, wt := aeaProyecto(t, "true", "")
	fakeProvider(t, "claude", ar1Codigo+"exit 0\n")
	antes := aeaVeredictos(root, wt)
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Task: aeaTarea, Prompt: "trabaja en la tarea demo"}, &buf)
	if err != nil || res.EnvelopeID == "" {
		t.Fatalf("fixture: the writer's envelope runs: %v %+v\n%s", err, res, buf.String())
	}
	ar6Cortado(t, "192f73 (tope de la foto)", "la foto no termina", root, wt, res, buf.String(), antes, FotoSinTerminar)
	if res.Note != NotaMaterialDeGit {
		t.Fatalf("its note is the git material one: %q", res.Note)
	}
}
