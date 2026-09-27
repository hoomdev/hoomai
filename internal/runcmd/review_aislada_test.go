// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-399, CA-400, CA-418) en runcmd: StartOptions gana Effort, Isolated y
// (enmienda 4) PromptStdin y los pasa al Request (Effort e Isolated tambien
// en Input), y cuando la invocacion trae Stdin, runcmd lo escribe en el stdin
// del proceso, entero: un CLI que no lo lee entero no termina bien. Los CLIs son falsos: guardan su argv
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

// CA-399 / CA-418 (enmienda 4): StartOptions.PromptStdin llega al Request:
// con un prompt CHICO el proceso lo recibe entero por stdin y el argv no lo
// lleva (codex termina en "-"), en claude y en codex. Sin PromptStdin, el
// mismo prompt sigue ultimo en argv con el stdin vacio (CA-109).
func TestCA418_StartOptionsPromptStdinMandaElPromptChicoPorStdin(t *testing.T) {
	const chico = "Revisa el cambio de esta rama: pedido chico con \"comillas\" y ñandú"
	for _, name := range []string{"claude", "codex"} {
		dir := raEspia(t, name, "")
		m := NewManager(t.TempDir())
		run, err := m.Start(StartOptions{Provider: name, Prompt: chico, PromptStdin: true, Dir: t.TempDir(),
			SystemPrompt: "# Reviewer", ReadOnly: true, Exec: true, Isolated: true, Strict: true})
		if err != nil {
			t.Fatalf("CA-418: %s: Start con PromptStdin: %v", name, err)
		}
		if fin := waitRun(t, m, run.ID); fin.Status != StatusDone {
			t.Fatalf("CA-418: %s: el run termina bien: %+v", name, fin)
		}
		if got := raStdin(t, dir, 1); got != chico {
			t.Fatalf("CA-418: %s: con PromptStdin el proceso recibe el prompt chico entero por stdin, recibio %q", name, got)
		}
		args := raArgv(t, dir, 1)
		for _, a := range args {
			if strings.Contains(a, "pedido chico") {
				t.Fatalf("CA-418: %s: con PromptStdin el argv no lleva el prompt: %q", name, args)
			}
		}
		if name == "codex" && args[len(args)-1] != "-" {
			t.Fatalf("CA-418: codex termina su argv en '-': %q", args)
		}

		// sin PromptStdin: CA-109 tal cual
		run, err = m.Start(StartOptions{Provider: name, Prompt: chico, Dir: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		waitRun(t, m, run.ID)
		args = raArgv(t, dir, 2)
		if args[len(args)-1] != chico || raStdin(t, dir, 2) != "" {
			t.Fatalf("CA-399: %s: sin PromptStdin un prompt chico sigue ultimo en argv y el stdin llega vacio: %q", name, args)
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

// CA-399 (el prompt entero): "Invocation.Stdin es el prompt entero y el
// proceso lo recibe por stdin". Un CLI que lee solo los primeros 100 bytes de
// un prompt de 1 MiB, dice que termino bien y sale con 0 NO recibio el
// pedido: una review asi daria un resultado limpio sobre una vista cortada
// (el spec: negarse en vez de truncar). El run no termina done: termina en
// error, y su narracion lo dice con un evento de error que nombra el prompt
// o el stdin. En claude y en codex.
func TestCA399_ProviderQueNoLeeElPromptEnteroEsError(t *testing.T) {
	grande := raGrande(1 << 20)
	if len(grande) <= providers.StdinPromptBytes {
		t.Fatalf("CA-399: fixture: el prompt (%d bytes) va por stdin: pasa %d", len(grande), providers.StdinPromptBytes)
	}
	exito := map[string]string{
		"claude": `{"type":"result","subtype":"success","is_error":false,"result":"ok","session_id":"sess-corto"}`,
		"codex":  `{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1}}`,
	}
	for _, name := range []string{"claude", "codex"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			installFakeNamed(t, name, "d='"+dir+"'\nhead -c 100 > \"$d/leido\"\necho '"+exito[name]+"'\nexit 0\n")
			m := NewManager(t.TempDir())
			run, err := m.Start(StartOptions{Provider: name, Prompt: grande, Dir: t.TempDir()})
			if err != nil {
				t.Fatalf("CA-399: %s: Start con prompt grande: %v", name, err)
			}
			fin := waitRun(t, m, run.ID)
			if leido, err := os.ReadFile(filepath.Join(dir, "leido")); err != nil || string(leido) != grande[:100] {
				t.Fatalf("CA-399: %s: fixture: el CLI falso corrio y leyo solo los primeros 100 bytes del prompt: %v %q", name, err, leido)
			}
			if fin.Status != StatusError {
				t.Fatalf("CA-399: %s: un CLI que leyo 100 de %d bytes del prompt no termino bien: el run termina en %q, termino en %q: %+v",
					name, len(grande), StatusError, fin.Status, fin)
			}
			_, evs, err := m.Events(run.ID, 0)
			if err != nil {
				t.Fatalf("CA-399: %s: Events: %v", name, err)
			}
			dice := false
			for _, e := range evs {
				d := strings.ToLower(e.Detail)
				if e.Kind == "error" && (strings.Contains(d, "prompt") || strings.Contains(d, "stdin")) {
					dice = true
				}
			}
			if !dice {
				t.Fatalf("CA-399: %s: la narracion dice que el provider no leyo el prompt entero (un evento error que nombra el prompt o el stdin): %+v", name, evs)
			}
		})
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
