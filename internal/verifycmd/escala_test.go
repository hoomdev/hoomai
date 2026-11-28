package verifycmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func initRepoConContador(t *testing.T, gatesYAML string) (*manifest.Manifest, string) {
	t.Helper()
	dir := t.TempDir()
	counter := filepath.Join(t.TempDir(), "runs.log")
	body := "schema: hoom/v1\nproject: demo\nbase_branch: main\npolicy: strict\n" +
		"findings:\n  block_on: high\n" +
		"gates:\n" + strings.ReplaceAll(gatesYAML, "{COUNTER}", counter)
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.txt"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("config", "user.email", "test@hoom.dev")
	git("config", "user.name", "hoom test")
	git("add", "-A")
	git("commit", "-m", "inicial")

	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m, counter
}

func contarLineas(t *testing.T, path, prefijo string) int {
	t.Helper()
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if l == prefijo {
			n++
		}
	}
	return n
}

func TestReutilizacionPorHuellaReevaluaHallazgosYRespetaEscape(t *testing.T) {
	m, counter := initRepoConContador(t,
		"  test:\n    required: true\n    cmd: \"echo test >> {COUNTER}\"\n"+
			"  lint:\n    required: true\n    cmd: \"echo lint >> {COUNTER}\"\n")

	// 1. Primera corrida: ejecuta ambos gates.
	v1, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if v1.Verdict != "green" {
		t.Fatalf("v1 debia ser verde: %+v", v1.Summary)
	}
	if got := contarLineas(t, counter, "test"); got != 1 {
		t.Fatalf("v1 debia ejecutar test 1 vez, fue %d", got)
	}

	// 2. Nuevo hallazgo high (sin cambiar codigo): verify DEBE dar ROJO por
	// findings_open pero SIN volver a ejecutar test ni lint.
	f, err := finding.Register(m.Dir, "main", finding.Draft{
		Severity:    "high",
		Lens:        "risk",
		File:        "app.txt",
		Description: "bug detectado en review",
		Author:      "reviewer@test",
	})
	if err != nil {
		t.Fatal(err)
	}
	v2, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if v2.Verdict != "red" {
		t.Fatalf("v2 debia dar ROJO por findings_open: %+v", v2.Summary)
	}
	if g := gateNamed(v2, "test"); g == nil || g.ReusedFrom != v1.ID {
		t.Fatalf("v2 debia reutilizar test de %s: %+v", v1.ID, g)
	}
	if got := contarLineas(t, counter, "test"); got != 1 {
		t.Fatalf("v2 no debia re-ejecutar test (misma huella): ejecuciones=%d", got)
	}

	// 3. Resolver el hallazgo y correr como piloto cerrar-hallazgos (Sobre con Reutilizar=true):
	// vuelve a VERDE sin re-ejecutar test.
	if _, err := finding.Resolve(m.Dir, f.ID, finding.StatusCorrected, "cubierto por test", "piloto"); err != nil {
		t.Fatal(err)
	}
	v3, _, err := Run(m, Options{Sobre: &Sobre{Root: m.Dir, Task: "demo", Rol: "writer", Reutilizar: true}})
	if err != nil {
		t.Fatal(err)
	}
	if v3.Verdict != "green" {
		t.Fatalf("v3 debia volver a VERDE: %+v", v3.Summary)
	}
	if g := gateNamed(v3, "test"); g == nil || g.ReusedFrom != v1.ID {
		t.Fatalf("v3 debia mantener el origen original %s: %+v", v1.ID, g)
	}
	if got := contarLineas(t, counter, "test"); got != 1 {
		t.Fatalf("v3 (cerrar-hallazgos) no debia re-ejecutar test: ejecuciones=%d", got)
	}

	// 4. --full, --gate y HOOM_FORCE_VERIFY=1 SIEMPRE ejecutan en fresco.
	if _, _, err := Run(m, Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if got := contarLineas(t, counter, "test"); got != 2 {
		t.Fatalf("--full debia ejecutar en fresco: ejecuciones=%d", got)
	}

	t.Setenv("HOOM_FORCE_VERIFY", "1")
	vForce, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if g := gateNamed(vForce, "test"); g == nil || g.ReusedFrom != "" {
		t.Fatalf("HOOM_FORCE_VERIFY=1 no debe reutilizar: %+v", g)
	}
	if got := contarLineas(t, counter, "test"); got != 3 {
		t.Fatalf("HOOM_FORCE_VERIFY=1 debia ejecutar en fresco: ejecuciones=%d", got)
	}
	t.Setenv("HOOM_FORCE_VERIFY", "")

	// 5. Cambiar un archivo invalida la huella y obliga a re-ejecutar.
	if err := os.WriteFile(filepath.Join(m.Dir, "app.txt"), []byte("v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	v4, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if g := gateNamed(v4, "test"); g == nil || g.ReusedFrom != "" {
		t.Fatalf("tras cambiar app.txt no debe reutilizar: %+v", g)
	}
	if got := contarLineas(t, counter, "test"); got != 4 {
		t.Fatalf("tras cambiar app.txt debia re-ejecutar test: ejecuciones=%d", got)
	}
}

func TestCierreReutilizaPassYCorreLosNoAplica(t *testing.T) {
	m, counter := initRepoConContador(t,
		"  test:\n    required: true\n    cmd: \"echo test >> {COUNTER}\"\n"+
			"  mutation:\n    required: true\n    cmd: \"echo mut-full >> {COUNTER}\"\n"+
			"    diff_cmd: \"echo mut-diff >> {COUNTER}\"\n"+
			"    aplica_en: [\"app/**\"]\n")

	// Cambio solo en frontend: en verify de trabajo, mutation queda no_aplica.
	if err := os.MkdirAll(filepath.Join(m.Dir, "resources", "js"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, "resources", "js", "app.js"), []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vWork, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if g := gateNamed(vWork, "mutation"); g == nil || g.Status != verdict.StatusNoAplica {
		t.Fatalf("en trabajo, mutation debia quedar no_aplica: %+v", g)
	}
	if contarLineas(t, counter, "test") != 1 || contarLineas(t, counter, "mut-diff") != 0 {
		t.Fatalf("conteos inesperados tras trabajo: test=%d mut=%d",
			contarLineas(t, counter, "test"), contarLineas(t, counter, "mut-diff"))
	}

	// En --cierre sobre la misma huella: test se reutiliza y mutation SI corre.
	vCierre, _, err := Run(m, Options{Cierre: true})
	if err != nil {
		t.Fatal(err)
	}
	if g := gateNamed(vCierre, "test"); g == nil || g.ReusedFrom != vWork.ID {
		t.Fatalf("--cierre debia reutilizar test que ya paso: %+v", g)
	}
	if g := gateNamed(vCierre, "mutation"); g == nil || g.Status != verdict.StatusPass || g.ReusedFrom != "" {
		t.Fatalf("--cierre debia ejecutar mutation en fresco: %+v", g)
	}
	if contarLineas(t, counter, "test") != 1 || contarLineas(t, counter, "mut-diff") != 1 {
		t.Fatalf("conteos inesperados tras cierre: test=%d mut=%d",
			contarLineas(t, counter, "test"), contarLineas(t, counter, "mut-diff"))
	}
}

func TestColaDeVerifySerializaCorridasConcurrentes(t *testing.T) {
	overlapFile := filepath.Join(t.TempDir(), "overlap")
	activeDir := filepath.Join(t.TempDir(), "active")
	script := fmt.Sprintf(
		"if ! mkdir %q 2>/dev/null; then touch %q; fi; sleep 0.15; rmdir %q",
		activeDir, overlapFile, activeDir,
	)
	m, _ := initRepoConContador(t,
		fmt.Sprintf("  test:\n    required: true\n    cmd: %q\n", script))

	t.Setenv("HOOM_FORCE_VERIFY", "1")
	var wg sync.WaitGroup
	var errs atomic.Int32
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			v, _, err := Run(m, Options{})
			if err != nil || v.Verdict != "green" {
				errs.Add(1)
			}
		}()
	}
	wg.Wait()
	if errs.Load() != 0 {
		t.Fatalf("ambos verifies concurrentes debian terminar en verde, errores=%d", errs.Load())
	}
	if _, err := os.Stat(overlapFile); !os.IsNotExist(err) {
		t.Fatalf("dos verifies concurrentes sobre el mismo proyecto se solaparon (el lock no los serializo)")
	}
}
