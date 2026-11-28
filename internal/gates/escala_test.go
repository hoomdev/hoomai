package gates

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func boolPtr(b bool) *bool { return &b }

func TestThreads_EnvYDefaultConservador(t *testing.T) {
	t.Setenv("HOOM_THREADS", "")
	wantDefault := runtime.NumCPU() / 2
	if wantDefault < 1 {
		wantDefault = 1
	}
	if got := Threads(); got != wantDefault {
		t.Fatalf("default Threads() = %d, esperaba %d", got, wantDefault)
	}

	t.Setenv("HOOM_THREADS", "6")
	if got := Threads(); got != 6 {
		t.Fatalf("HOOM_THREADS=6: Threads() = %d, esperaba 6", got)
	}

	for _, bad := range []string{"0", "-3", "max", "abc"} {
		t.Setenv("HOOM_THREADS", bad)
		if got := Threads(); got != wantDefault {
			t.Fatalf("HOOM_THREADS=%q invalido debia caer al default %d, fue %d", bad, wantDefault, got)
		}
	}
}

func TestSelectCommand_ThreadsYGateFilesYEnv(t *testing.T) {
	g := manifest.Gate{
		Required: boolPtr(true),
		Cmd:      strPtr("tool --threads={threads}"),
		DiffCmd:  strPtr("tool --threads={threads} --base={base} --files={gate_files}"),
		AplicaEn: []string{"app/**"},
	}
	git := gitx.Info{
		IsRepo:       true,
		Base:         "main",
		ChangedFiles: []string{"app/Foo.php", "resources/js/app.js"},
	}
	cmdStr, scope := selectCommand(g, git, Options{Threads: 3})
	want := "tool --threads=3 --base=main --files=app/Foo.php"
	if scope != "diff" || cmdStr != want {
		t.Fatalf("selectCommand = (%q, %q), esperaba (%q, \"diff\")", cmdStr, scope, want)
	}

	// Si ningun archivo coincide con aplica_en, {gate_files} vacio cae a cmd completo.
	gitJS := gitx.Info{IsRepo: true, Base: "main", ChangedFiles: []string{"resources/js/app.js"}}
	cmdStr, scope = selectCommand(g, gitJS, Options{Threads: 3})
	if scope != "full" || cmdStr != "tool --threads=3" {
		t.Fatalf("fallback de {gate_files} vacio = (%q, %q), esperaba (\"tool --threads=3\", \"full\")", cmdStr, scope)
	}

	// Los subprocesos de gate reciben HOOM_THREADS y HOOM_BASE en su entorno.
	dir := t.TempDir()
	outFile := filepath.Join(dir, "env.txt")
	m := &manifest.Manifest{
		Dir: dir,
		Gates: map[string]manifest.Gate{
			"test": {
				Required: boolPtr(true),
				Cmd:      strPtr(fmt.Sprintf("printf '%%s:%%s' \"$HOOM_THREADS\" \"$HOOM_BASE\" > %q", outFile)),
			},
		},
	}
	res := Run(m, gitx.Info{IsRepo: true, Base: "develop"}, Options{Threads: 5, Timeout: time.Minute})
	if len(res) != 1 || res[0].Status != verdict.StatusPass {
		t.Fatalf("gate test debia pasar: %+v", res)
	}
	raw, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "5:develop" {
		t.Fatalf("env en gate = %q, esperaba \"5:develop\"", string(raw))
	}
}

func TestMutationOmitidaCuandoTestRequeridoFalla(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "mutation-ran")
	m := &manifest.Manifest{
		Dir: dir,
		Gates: map[string]manifest.Gate{
			"test": {
				Required: boolPtr(true),
				Cmd:      strPtr("exit 1"),
			},
			"mutation": {
				Required: boolPtr(true),
				Cmd:      strPtr(fmt.Sprintf("touch %q", marker)),
			},
		},
	}
	res := Run(m, gitx.Info{}, Options{Timeout: time.Minute})
	var testRes, mutRes verdict.GateResult
	for _, r := range res {
		switch r.Name {
		case "test":
			testRes = r
		case "mutation":
			mutRes = r
		}
	}
	if testRes.Status != verdict.StatusFail {
		t.Fatalf("test debia fallar: %+v", testRes)
	}
	if mutRes.Status != verdict.StatusNoAplica {
		t.Fatalf("mutation debia quedar no_aplica tras fallo de test: %+v", mutRes)
	}
	if want := fmt.Sprintf(MotivoTestFallo, verdict.StatusFail); mutRes.Notes != want {
		t.Fatalf("nota de mutation = %q, esperaba %q", mutRes.Notes, want)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("mutation NO debia ejecutar su comando cuando test fallo")
	}
	v := &verdict.Verdict{Gates: res}
	v.Finalize()
	if v.Verdict != "red" || v.IsPartial() {
		t.Fatalf("el veredicto debe ser ROJO y completo: verdict=%s partial=%v", v.Verdict, v.IsPartial())
	}

	// Con --gate mutation explicito, mutation SI corre.
	resOnly := Run(m, gitx.Info{}, Options{Only: map[string]bool{"mutation": true}, Timeout: time.Minute})
	for _, r := range resOnly {
		if r.Name == "mutation" && r.Status != verdict.StatusPass {
			t.Fatalf("--gate mutation debe correr aunque test fallaria: %+v", r)
		}
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("--gate mutation debia ejecutar el comando: %v", err)
	}
}

func TestMutationCorreSiTestEsOpcional(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "mutation-ran")
	m := &manifest.Manifest{
		Dir: dir,
		Gates: map[string]manifest.Gate{
			"test": {
				Required: boolPtr(false),
				Cmd:      strPtr("exit 1"),
			},
			"mutation": {
				Required: boolPtr(true),
				Cmd:      strPtr(fmt.Sprintf("touch %q", marker)),
			},
		},
	}
	res := Run(m, gitx.Info{}, Options{Timeout: time.Minute})
	for _, r := range res {
		if r.Name == "mutation" && r.Status != verdict.StatusPass {
			t.Fatalf("si test es opcional, mutation requerida debe correr: %+v", r)
		}
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("mutation debia correr cuando test no es requerido: %v", err)
	}
	// Silenciar imports usados si cambiara algo.
	if !strings.Contains(strconv.Itoa(Threads()), "") {
		t.Fatal("unreachable")
	}
}
