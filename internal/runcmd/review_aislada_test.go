// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-399, CA-400) en runcmd: StartOptions gana Effort e Isolated y los pasa
// al Request (tambien en Input), y cuando la invocacion trae Stdin, runcmd lo
// escribe en el stdin del proceso. Los CLIs son falsos: guardan su argv
// (separado por NUL) y lo que les llega por stdin, fuera de cualquier arbol.
package runcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/providers"
)

// raEspia instala un CLI falso name que guarda, por invocacion n, su argv en
// <dir>/argv.n y su stdin en <dir>/stdin.n. extra va antes del exit 0.
func raEspia(t *testing.T, name, extra string) string {
	t.Helper()
	dir := t.TempDir()
	installFakeNamed(t, name, "d='"+dir+"'\n"+
		"n=$(cat \"$d/n\" 2>/dev/null || echo 0); n=$((n+1)); echo $n > \"$d/n\"\n"+
		"printf '%s\\000' \"$@\" > \"$d/argv.$n\"\n"+
		"cat > \"$d/stdin.$n\"\n"+
		extra+"exit 0\n")
	return dir
}

func raArgv(t *testing.T, dir string, n int) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "argv."+itoa(n)))
	if err != nil {
		t.Fatalf("la invocacion %d nunca corrio: %v", n, err)
	}
	args := strings.Split(string(raw), "\x00")
	if len(args) > 0 && args[len(args)-1] == "" {
		args = args[:len(args)-1]
	}
	return args
}

func raStdin(t *testing.T, dir string, n int) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "stdin."+itoa(n)))
	if err != nil {
		t.Fatalf("la invocacion %d nunca corrio: %v", n, err)
	}
	return string(raw)
}

func itoa(n int) string { return strconv.Itoa(n) }

func raTiene(args []string, a string) bool {
	for _, x := range args {
		if x == a {
			return true
		}
	}
	return false
}

func raPar(args []string, flag, valor string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == valor {
			return true
		}
	}
	return false
}

// raGrande arma un prompt de n bytes con lo que un shell romperia si viajara
// mal: comillas, $, backticks, saltos de linea, NUL no (un argv no lo lleva)
// y unicode.
func raGrande(n int) string {
	var b strings.Builder
	b.WriteString("Revisa el cambio de esta rama.\n")
	for i := 0; b.Len() < n; i++ {
		b.WriteString("+linea ")
		b.WriteString(itoa(i % 10))
		b.WriteString(" con \"comillas\", 'simples', $HOME, `id` y ñandú €\n")
	}
	s := b.String()
	for len(s) > n {
		s = s[:len(s)-1]
	}
	return s
}

// CA-399: con un prompt de mas de 16 KiB el proceso lo recibe ENTERO por
// stdin y el argv no lo lleva, en claude y en codex; con 16 KiB o menos
// sigue ultimo en argv y el stdin llega vacio (CA-109).
func TestCA399_ElProcesoRecibeElPromptPorStdin(t *testing.T) {
	grande := raGrande(providers.StdinPromptBytes + 4096)
	for _, name := range []string{"claude", "codex"} {
		dir := raEspia(t, name, "")
		m := NewManager(t.TempDir())
		run, err := m.Start(StartOptions{Provider: name, Prompt: grande})
		if err != nil {
			t.Fatalf("CA-399: %s: Start con prompt grande: %v", name, err)
		}
		if fin := waitRun(t, m, run.ID); fin.Status != StatusDone {
			t.Fatalf("CA-399: %s: el run termina bien: %+v", name, fin)
		}
		if got := raStdin(t, dir, 1); got != grande {
			t.Fatalf("CA-399: %s: el proceso recibe el prompt entero por stdin (%d bytes), recibio %d bytes", name, len(grande), len(got))
		}
		for _, a := range raArgv(t, dir, 1) {
			if strings.Contains(a, "con \"comillas\"") {
				t.Fatalf("CA-399: %s: el argv no lleva el prompt grande", name)
			}
		}

		// chico: todo como hoy
		chico := "revisa esto"
		run, err = m.Start(StartOptions{Provider: name, Prompt: chico})
		if err != nil {
			t.Fatal(err)
		}
		waitRun(t, m, run.ID)
		args := raArgv(t, dir, 2)
		if args[len(args)-1] != chico || raStdin(t, dir, 2) != "" {
			t.Fatalf("CA-399: %s: un prompt chico sigue ultimo en argv y el stdin llega vacio: %v", name, args)
		}
	}
}

// CA-399: Input continua la sesion con un prompt grande: tambien por stdin.
func TestCA399_InputConPromptGrandeTambienPorStdin(t *testing.T) {
	dir := raEspia(t, "claude",
		"echo '{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"sess-399\"}'\n"+
			"echo '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"result\":\"ok\",\"session_id\":\"sess-399\"}'\n")
	m := NewManager(t.TempDir())
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "primero"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, run.ID)
	grande := raGrande(providers.StdinPromptBytes + 1)
	if _, err := m.Input(run.ID, grande); err != nil {
		t.Fatalf("CA-399: Input con prompt grande: %v", err)
	}
	waitRun(t, m, run.ID)
	if got := raStdin(t, dir, 2); got != grande {
		t.Fatalf("CA-399: la continuacion recibe el prompt grande por stdin (%d bytes), recibio %d", len(grande), len(got))
	}
	args := raArgv(t, dir, 2)
	if !raPar(args, "--resume", "sess-399") {
		t.Fatalf("CA-399: la continuacion sigue siendo un --resume: %v", args)
	}
	for _, a := range args {
		if a == grande {
			t.Fatalf("CA-399: el argv de la continuacion no lleva el prompt grande")
		}
	}
}

// CA-400: StartOptions.Effort e Isolated llegan al Request: el argv del CLI
// los trae, en el primer lanzamiento y al continuar (Input re-aplica las
// opciones originales).
func TestCA400_StartOptionsPasaEffortEIsolated(t *testing.T) {
	dir := raEspia(t, "codex", "echo '{\"type\":\"thread.started\",\"thread_id\":\"th-400\"}'\n")
	m := NewManager(t.TempDir())
	run, err := m.Start(StartOptions{Provider: "codex", Prompt: "revisa", Effort: "high", Isolated: true, Strict: true,
		SystemPrompt: "# Reviewer", ReadOnly: true, Exec: true})
	if err != nil {
		t.Fatalf("CA-400: Start con Effort e Isolated: %v", err)
	}
	waitRun(t, m, run.ID)
	args := raArgv(t, dir, 1)
	if !raTiene(args, "--ignore-user-config") || !raPar(args, "-c", `model_reasoning_effort="high"`) {
		t.Fatalf("CA-400: StartOptions pasa Isolated y Effort al Request: %v", args)
	}
	if _, err := m.Input(run.ID, "segui"); err != nil {
		t.Fatalf("CA-400: Input: %v", err)
	}
	waitRun(t, m, run.ID)
	args = raArgv(t, dir, 2)
	if !raTiene(args, "--ignore-user-config") || !raPar(args, "-c", `model_reasoning_effort="high"`) {
		t.Fatalf("CA-400: Input re-aplica Isolated y Effort: %v", args)
	}

	// claude: los flags propios
	dirC := raEspia(t, "claude", "")
	run, err = m.Start(StartOptions{Provider: "claude", Prompt: "revisa", Effort: "low", Isolated: true, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, run.ID)
	args = raArgv(t, dirC, 1)
	if !raTiene(args, "--strict-mcp-config") || !raPar(args, "--setting-sources", "project") || !raPar(args, "--effort", "low") {
		t.Fatalf("CA-400: claude recibe --strict-mcp-config, --setting-sources project y --effort low: %v", args)
	}

	// sin pedirlos, nada: el argv de hoy (CA-155)
	dirS := raEspia(t, "codex", "")
	run, err = m.Start(StartOptions{Provider: "codex", Prompt: "implementa", Unattended: true, Dir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, run.ID)
	for _, a := range raArgv(t, dirS, 1) {
		if a == "--ignore-user-config" || strings.HasPrefix(a, "model_reasoning_effort=") {
			t.Fatalf("CA-400: sin Isolated ni Effort el run no pisa la config del usuario: %v", raArgv(t, dirS, 1))
		}
	}
}

// CA-396/CA-400: con Strict, pedir Effort o Isolated a un provider sin la
// capacidad niega el arranque: ErrUnsupported, sin run ni log.
func TestCA400_StrictSinCapacidadNoArranca(t *testing.T) {
	raEspia(t, "gemini", "")
	root := t.TempDir()
	m := NewManager(root)
	for _, so := range []StartOptions{
		{Provider: "gemini", Prompt: "revisa", Effort: "high", Strict: true},
		{Provider: "gemini", Prompt: "revisa", Isolated: true, Strict: true},
	} {
		_, err := m.Start(so)
		var eu providers.ErrUnsupported
		if !errors.As(err, &eu) {
			t.Fatalf("CA-400: Strict con un provider sin la capacidad niega el arranque: %v", err)
		}
		if !raTiene(eu.Fields, providers.FieldEffort) && !raTiene(eu.Fields, providers.FieldIsolation) {
			t.Fatalf("CA-400: el error nombra effort o isolation: %+v", eu)
		}
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".hoom", "runs")); len(entries) != 0 {
		t.Fatalf("CA-400: una negativa no deja run ni log: %d archivos", len(entries))
	}
}
