// Tests adversariales del spec .hoom/specs/arquitecto-bajo-el-sobre.md
// (CA-235, CA-244): la review cruzada no cuenta a un autor de specs como el
// que escribio el codigo, y los hallazgos que su gate de scope crea llevan la
// tarea de la review.
package reviewcmd

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// CA-235: con un run de writer (codex) y uno POSTERIOR de un autor de specs
// (claude) en el mismo directorio, el writer que la review reconoce es codex.
func TestCA235_UnAutorDeSpecsNoEsElWriter(t *testing.T) {
	for _, autor := range []string{"arquitecto", "designer", "analista"} {
		root := repo(t)
		fakeProvider(t, "claude", "exit 0\n")
		fakeProvider(t, "codex", "exit 0\n")
		ahora := time.Now()
		metaDeRun(t, root, "20260918T100000_aaaaaa", "codex", "writer", ahora.Add(-time.Hour))
		metaDeRun(t, root, "20260918T110000_bbbbbb", "claude", autor, ahora)

		var out bytes.Buffer
		res, err := Run(root, "main", Options{Lens: "risk"}, &out)
		if err != nil {
			t.Fatalf("CA-235: %v\n%s", err, out.String())
		}
		if res.Writer != "codex" {
			t.Fatalf("CA-235: el %s no escribio el codigo revisado; el writer es codex: %+v\n%s", autor, res, out.String())
		}
		if res.Provider != "claude" || res.Cross != CrossYes {
			t.Fatalf("CA-235: la review es cruzada contra el writer real: %+v\n%s", res, out.String())
		}
	}
}

// CA-235: un autor de specs solo no hace de nadie un writer: la cruzada es
// desconocida, como sin runs previos.
func TestCA235_SoloUnAutorDeSpecsNoHayWriter(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	metaDeRun(t, root, "20260918T110000_bbbbbb", "claude", "arquitecto", time.Now())

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk"}, &out)
	if err != nil {
		t.Fatalf("CA-235: %v\n%s", err, out.String())
	}
	if res.Writer != "" || res.Cross != CrossUnknown {
		t.Fatalf("CA-235: un arquitecto no es el writer de nadie: %+v\n%s", res, out.String())
	}

	// y forzar el mismo provider del arquitecto NO es una review no cruzada
	out.Reset()
	res, err = Run(root, "main", Options{Lens: "risk", Provider: "claude"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if res.Cross == CrossNo || res.ExitCode != 0 {
		t.Fatalf("CA-235: revisar con el provider del arquitecto no se niega: %+v\n%s", res, out.String())
	}
}

// CA-235: el salto es para la forma specs, no para todo el que escribe: un
// characterizer posterior SI cuenta como writer.
func TestCA235_ElCharacterizerSiCuenta(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	ahora := time.Now()
	metaDeRun(t, root, "20260918T100000_aaaaaa", "codex", "writer", ahora.Add(-time.Hour))
	metaDeRun(t, root, "20260918T110000_cccccc", "claude", "characterizer", ahora)

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk"}, &out)
	if err != nil {
		t.Fatalf("CA-235: %v\n%s", err, out.String())
	}
	if res.Writer != "claude" || res.Provider != "codex" {
		t.Fatalf("CA-235: el characterizer escribe tests y cuenta como writer: %+v\n%s", res, out.String())
	}
}

// abHallazgoEn busca un hallazgo por id en cualquiera de los directorios.
func abHallazgoEn(t *testing.T, id string, dirs ...string) (finding.Finding, bool) {
	t.Helper()
	for _, d := range dirs {
		items, _, err := finding.List(d, "main", false)
		if err != nil {
			continue
		}
		for _, it := range items {
			if it.ID == id {
				return it.Finding, true
			}
		}
	}
	return finding.Finding{}, false
}

// CA-244: con --task, el hallazgo que el gate de scope de la review crea por
// una violacion lleva esa tarea.
func TestCA244_LaReviewConTareaAtaElHallazgoDelGate(t *testing.T) {
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	if err := taskcmd.Start(root, "cabina-visual", "main"); err != nil {
		t.Fatal(err)
	}
	dir, err := runcmd.TaskDir(root, "cabina-visual")
	if err != nil {
		t.Fatal(err)
	}
	// el cambio a revisar vive en el worktree de la tarea
	write(t, dir, "app.go", "package app\n\nfunc Nuevo() {}\n")
	// el reviewer escribe fuera de .hoom/: violacion, y hoom deja su hallazgo
	fakeProvider(t, "codex", "printf 'x' > colado.txt\nexit 0\n")
	t.Setenv("HOOM_TASK", "ajena")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Provider: "codex", Task: "cabina-visual"}, &out)
	if err != nil {
		t.Fatalf("CA-244: %v\n%s", err, out.String())
	}
	if len(res.Passes) != 1 || len(res.Passes[0].Scope.Violations) != 1 {
		t.Fatalf("CA-244: la escritura fuera de .hoom/ es una violacion: %+v\n%s", res, out.String())
	}
	v := res.Passes[0].Scope.Violations[0]
	if v.FindingID == "" {
		t.Fatalf("CA-244: la violacion deja hallazgo: %+v", v)
	}
	f, ok := abHallazgoEn(t, v.FindingID, dir, root)
	if !ok {
		t.Fatalf("CA-244: el hallazgo %s no quedo registrado", v.FindingID)
	}
	if f.Task != "cabina-visual" {
		t.Fatalf("CA-244: el hallazgo del gate lleva la tarea de la review: %+v", f)
	}
}

// CA-244: sin --task, el hallazgo del gate no lleva ninguna, aunque el
// humano tenga HOOM_TASK exportada.
func TestCA244_LaReviewSinTareaNoEtiqueta(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "codex", "printf 'x' > "+filepath.Join(root, "colado.txt")+"\nexit 0\n")
	t.Setenv("HOOM_TASK", "ajena")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Provider: "codex"}, &out)
	if err != nil {
		t.Fatalf("CA-244: %v\n%s", err, out.String())
	}
	if len(res.Passes) != 1 || len(res.Passes[0].Scope.Violations) != 1 {
		t.Fatalf("CA-244: la escritura fuera de .hoom/ es una violacion: %+v\n%s", res, out.String())
	}
	v := res.Passes[0].Scope.Violations[0]
	f, ok := abHallazgoEn(t, v.FindingID, root)
	if !ok {
		t.Fatalf("CA-244: el hallazgo %s no quedo registrado", v.FindingID)
	}
	if f.Task != "" {
		t.Fatalf("CA-244: sin --task el hallazgo del gate no lleva tarea: %+v", f)
	}
}
