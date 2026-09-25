// Tests de regresion de la review cruzada de la cabina (C1, 2026-09-24),
// hallazgo 20260924T200412_151300 sobre `hoom agent`: con Started (el
// Studio) el sobre prometia "el primer registro ya esta en disco" (CA-335)
// sin saberlo, porque envelope.Write tragaba el error. Si ese primer registro
// no se puede escribir, Started no se llama, Run devuelve un error que lo
// dice y no arranca ningun run. Sin Started (la CLI) el registro sigue
// siendo best-effort (CA-202) y el sobre corre igual. Providers falsos en el
// PATH: ningun CLI real.
package agentcmd

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// h3Repo es repo() con la telemetria escondida y commiteada en
// .hoom/.gitignore: lo que se plante en .hoom/envelopes/ no ensucia el arbol.
func h3Repo(t *testing.T) string {
	t.Helper()
	root := repo(t)
	write(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "telemetria escondida")
	return root
}

// h3Bloquear planta un DIRECTORIO (no vacio) donde va
// .hoom/envelopes/<id>.json: el registro del sobre no se puede escribir.
func h3Bloquear(t *testing.T, root, id string) {
	t.Helper()
	p := filepath.Join(root, ".hoom", envelope.DirName, id+".json")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p, "ocupado"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// h3Provider instala un claude falso que deja una marca FUERA del proyecto
// cada vez que lo invocan, y escribe codigo como un writer. Devuelve la
// ruta de la marca.
func h3Provider(t *testing.T) string {
	t.Helper()
	marca := filepath.Join(t.TempDir(), "claude-invocado")
	fakeProvider(t, "claude", "echo invocado >> '"+marca+"'\n"+
		"printf 'package app // implementado\\n' > app.go\nexit 0\n")
	return marca
}

func h3Invocado(marca string) bool {
	_, err := os.Stat(marca)
	return err == nil
}

// h3Metas cuenta los sidecars .hoom/runs/*.meta.json del proyecto.
func h3Metas(t *testing.T, root string) []string {
	t.Helper()
	m, err := filepath.Glob(filepath.Join(root, ".hoom", "runs", "*.meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// Hallazgo 20260924T200412_151300 (CA-335, CA-202): con Started no nil y
// EnvelopeID preasignado, si el primer registro del sobre no se puede
// escribir (hay un directorio en .hoom/envelopes/<id>.json), Started nunca
// se llama, Run devuelve un error que nombra el registro del sobre, y no
// arranca ningun run: el provider no se invoca y no aparece ningun
// .hoom/runs/*.meta.json.
func TestHallazgo_151300_AgentConStartedNoArrancaSinPrimerRegistro(t *testing.T) {
	root := h3Repo(t)
	marca := h3Provider(t)
	s := &cbStarted{root: root, id: "20260924T200412_h3ag01"}
	h3Bloquear(t, root, s.id)

	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa", EnvelopeID: s.id, Started: s.fn}, io.Discard)
	if calls, _, _ := s.snapshot(); calls != 0 {
		t.Fatalf("CA-335: hallazgo 151300: el primer registro no esta en disco, asi que Started no se llama (llamadas: %d, res %+v)", calls, res)
	}
	if err == nil {
		t.Fatalf("CA-335: hallazgo 151300: sin primer registro el sobre con Started devuelve un error, no un Result: %+v", res)
	}
	if msg := strings.ToLower(err.Error()); !strings.Contains(msg, "registro") || !strings.Contains(msg, "sobre") {
		t.Fatalf("CA-335: hallazgo 151300: el error nombra el registro del sobre: %q", err)
	}
	if h3Invocado(marca) {
		t.Fatal("CA-335: hallazgo 151300: sin primer registro no arranca ningun run: el provider se invoco")
	}
	if m := h3Metas(t, root); len(m) != 0 {
		t.Fatalf("CA-335: hallazgo 151300: sin primer registro no aparece ningun .hoom/runs/*.meta.json: %v", m)
	}
	if recs := envelope.List(root); len(recs) != 0 {
		t.Fatalf("CA-335: no queda registro: %+v", recs)
	}
}

// Hallazgo 20260924T200412_151300 (CA-202), GUARDA: con Started nil (la
// CLI) nada cambia aunque el registro no se pueda escribir: el sobre sigue
// best-effort, no hay error, el provider corre y el run deja su sidecar.
func TestHallazgo_151300_AgentSinStartedSigueBestEffort(t *testing.T) {
	// (a) EnvelopeID preasignado y tapado, Started nil
	root := h3Repo(t)
	marca := h3Provider(t)
	const id = "20260924T200412_h3ag02"
	h3Bloquear(t, root, id)
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa", EnvelopeID: id}, io.Discard)
	if err != nil {
		t.Fatalf("CA-202: sin Started el registro es best-effort: el sobre no falla por no poder escribirlo: %v", err)
	}
	if !h3Invocado(marca) {
		t.Fatalf("CA-202: sin Started el run corre aunque el registro no se pueda escribir: %+v", res)
	}
	if m := h3Metas(t, root); len(m) == 0 {
		t.Fatalf("CA-202: el run corrio y dejo su sidecar en .hoom/runs/: %+v", res)
	}
	if res.ExitCode != 0 || res.RunID == "" {
		t.Fatalf("CA-202: el sobre termina como siempre (exit 0, con su run): %+v", res)
	}

	// (b) la CLI de verdad: sin EnvelopeID ni Started, .hoom/envelopes/ de
	// solo lectura
	if os.Geteuid() == 0 {
		t.Log("CA-202: corriendo como root los permisos no frenan la escritura: se saltea (b)")
		return
	}
	root2 := h3Repo(t)
	marca2 := h3Provider(t)
	dir := filepath.Join(root2, ".hoom", envelope.DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	res, err = Run(root2, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-202: la CLI con .hoom/envelopes/ de solo lectura no falla: %v", err)
	}
	if !h3Invocado(marca2) || len(h3Metas(t, root2)) == 0 || res.ExitCode != 0 {
		t.Fatalf("CA-202: la CLI corre el run igual y termina como siempre: %+v", res)
	}
}
