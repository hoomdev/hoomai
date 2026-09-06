// Tests adversariales del spec .hoom/specs/codex-v2-y-review-cruzada.md:
// el adapter Codex v2. Todo lo que se afirma aca se verifico a mano contra
// Codex CLI 0.151.0 antes de escribirlo.
package providers

import (
	"errors"
	"strconv"
	"strings"
	"testing"
)

func codexArgs(t *testing.T, req Request) []string {
	t.Helper()
	p, _ := Lookup("codex")
	inv, err := p.Command(req)
	if err != nil {
		t.Fatalf("Command fallo: %v", err)
	}
	if inv.Bin != "codex" {
		t.Fatalf("el binario es codex, no %q", inv.Bin)
	}
	return inv.Args
}

// CA-151: las capacidades que codex declara son las que se probaron; las que
// no tiene se degradan con su nombre canonico y se niegan bajo Strict.
func TestCA151_CapacidadesDeCodex(t *testing.T) {
	p, _ := Lookup("codex")
	c := p.Capabilities()
	for _, tc := range []struct {
		nombre string
		tiene  bool
	}{
		{"structured", c.Structured}, {"continue", c.Continue}, {"resume", c.Resume},
		{"session_id", c.SessionID}, {"model", c.Model}, {"system_prompt", c.SystemPrompt},
		{"read_only", c.ReadOnly},
	} {
		if !tc.tiene {
			t.Fatalf("CA-151: codex declara %s", tc.nombre)
		}
	}
	if c.Tools || c.MaxTurns || c.Budget {
		t.Fatalf("CA-151: codex no nombra herramientas ni tiene topes: %+v", c)
	}

	inv, err := p.Command(Request{Prompt: "hola", AllowTools: []string{"Read"}, MaxTurns: 3, BudgetUSD: 1})
	if err != nil {
		t.Fatalf("CA-151: sin Strict se degrada: %v", err)
	}
	if got := strings.Join(inv.Ignored, ","); got != "tools,max_turns,budget" {
		t.Fatalf("CA-151: los ignorados van en orden canonico: %q", got)
	}
	_, err = p.Command(Request{Prompt: "hola", MaxTurns: 3, Strict: true})
	var eu ErrUnsupported
	if !errors.As(err, &eu) || eu.Provider != "codex" || len(eu.Fields) != 1 || eu.Fields[0] != FieldMaxTurns {
		t.Fatalf("CA-151: con Strict se niega nombrando el campo: %v", err)
	}
}

// CA-152: la forma base es `codex exec --json ... <prompt>` y el prompt es
// SIEMPRE el ultimo argumento; la validacion comun corta antes de armar nada.
func TestCA152_FormaBase(t *testing.T) {
	args := codexArgs(t, Request{Prompt: "revisa el diff"})
	if args[0] != "exec" || args[1] != "--json" {
		t.Fatalf("CA-152: la forma base es `exec --json`: %v", args)
	}
	if args[len(args)-1] != "revisa el diff" {
		t.Fatalf("CA-152: el prompt va ultimo: %v", args)
	}
	args = codexArgs(t, Request{Prompt: "revisa", Model: "gpt-5", SystemPrompt: "C", ReadOnly: true})
	if args[len(args)-1] != "revisa" {
		t.Fatalf("CA-152: el prompt va ultimo aunque haya banderas: %v", args)
	}

	p, _ := Lookup("codex")
	if _, err := p.Command(Request{Prompt: "   "}); err == nil {
		t.Fatal("CA-152: prompt vacio se rechaza antes de armar nada")
	}
	if _, err := p.Command(Request{Prompt: "-p mentira"}); err == nil {
		t.Fatal("CA-152: un prompt que empieza con '-' se rechaza")
	}
}

// CA-153: el contrato viaja como cadena basica TOML y vuelve byte a byte.
// El valor de `-c` se parsea como TOML: sin codificar, un contrato entre
// comillas las perderia y uno que parsee como bool o numero seria un error
// duro del CLI.
func TestCA153_ContratoComoTOML(t *testing.T) {
	for _, contrato := range []string{
		"# Reviewer\n\nRol: revision \"con contexto fresco\".\n\tTab y backslash \\ adentro.\nAcentos: revisión y ñ.\n",
		"true",
		"42",
		"\"ya venia entre comillas\"",
		"con = igual y # numeral",
	} {
		args := codexArgs(t, Request{Prompt: "hola", SystemPrompt: contrato})
		valor := ""
		for i, a := range args {
			if a == "-c" && i+1 < len(args) && strings.HasPrefix(args[i+1], "developer_instructions=") {
				valor = strings.TrimPrefix(args[i+1], "developer_instructions=")
			}
		}
		if valor == "" {
			t.Fatalf("CA-153: falta -c developer_instructions: %v", args)
		}
		vuelta, ok := decodeTOMLBasic(valor)
		if !ok {
			t.Fatalf("CA-153: el valor no es una cadena basica TOML: %q", valor)
		}
		if vuelta != contrato {
			t.Fatalf("CA-153: el round-trip debe ser byte a byte:\n  ida:    %q\n  vuelta: %q", contrato, vuelta)
		}
	}

	// sin system prompt no aparece la bandera
	for _, a := range codexArgs(t, Request{Prompt: "hola"}) {
		if strings.HasPrefix(a, "developer_instructions=") {
			t.Fatal("CA-153: sin contrato no se manda developer_instructions")
		}
	}
}

// decodeTOMLBasic deshace lo que tomlString hizo: el test no confia en la
// funcion que esta probando, lo lee como lo leeria un parser TOML.
func decodeTOMLBasic(s string) (string, bool) {
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return "", false
	}
	body := []rune(s[1 : len(s)-1])
	var out strings.Builder
	for i := 0; i < len(body); i++ {
		if body[i] != '\\' {
			if body[i] == '"' {
				return "", false // una comilla sin escapar cerraria la cadena
			}
			out.WriteRune(body[i])
			continue
		}
		i++
		if i >= len(body) {
			return "", false
		}
		switch body[i] {
		case '\\':
			out.WriteByte('\\')
		case '"':
			out.WriteByte('"')
		case 'n':
			out.WriteByte('\n')
		case 'r':
			out.WriteByte('\r')
		case 't':
			out.WriteByte('\t')
		case 'b':
			out.WriteByte('\b')
		case 'f':
			out.WriteByte('\f')
		case 'u':
			if i+4 >= len(body) {
				return "", false
			}
			n, err := strconv.ParseInt(string(body[i+1:i+5]), 16, 32)
			if err != nil {
				return "", false
			}
			out.WriteRune(rune(n))
			i += 4
		default:
			return "", false
		}
	}
	return out.String(), true
}

// CA-154: la sesion de codex. `resume <id>` con el id pegado al subcomando,
// `resume --last` para continuar, y la regla comun de precedencia sin
// duplicar subcomandos.
func TestCA154_SesionDeCodex(t *testing.T) {
	args := codexArgs(t, Request{Prompt: "seguimos", ResumeID: "01a06e3f-5720-7843-b638-7d141171776d"})
	if args[0] != "exec" || args[1] != "resume" || args[2] != "01a06e3f-5720-7843-b638-7d141171776d" {
		t.Fatalf("CA-154: el id va inmediatamente despues de resume: %v", args)
	}
	if args[len(args)-1] != "seguimos" {
		t.Fatalf("CA-154: el prompt sigue yendo ultimo: %v", args)
	}

	args = codexArgs(t, Request{Prompt: "seguimos", Continue: true})
	if strings.Join(args[:3], " ") != "exec resume --last" {
		t.Fatalf("CA-154: continuar es resume --last: %v", args)
	}

	// id + continue: gana el id y no aparecen dos resume
	args = codexArgs(t, Request{Prompt: "seguimos", ResumeID: "abc", Continue: true})
	if strings.Join(args[:3], " ") != "exec resume abc" {
		t.Fatalf("CA-154: el id le gana a continue: %v", args)
	}
	if strings.Count(strings.Join(args, " "), "resume") != 1 {
		t.Fatalf("CA-154: un solo subcomando resume: %v", args)
	}
}

// CA-155: el limite del rol es el sandbox, y viaja igual en las dos formas
// porque `codex exec resume` NO tiene -s/--sandbox.
func TestCA155_SandboxDeCodex(t *testing.T) {
	tieneSandbox := func(args []string, modo string) bool {
		for i, a := range args {
			if a == "-c" && i+1 < len(args) && args[i+1] == `sandbox_mode="`+modo+`"` {
				return true
			}
		}
		return false
	}

	if args := codexArgs(t, Request{Prompt: "explora", ReadOnly: true}); !tieneSandbox(args, "read-only") {
		t.Fatalf("CA-155: un rol que no ejecuta va en read-only: %v", args)
	}
	if args := codexArgs(t, Request{Prompt: "revisa", ReadOnly: true, Exec: true}); !tieneSandbox(args, "workspace-write") {
		t.Fatalf("CA-155: un rol que ejecuta necesita workspace-write: %v", args)
	}
	args := codexArgs(t, Request{Prompt: "seguimos", ResumeID: "abc", ReadOnly: true})
	if !tieneSandbox(args, "read-only") {
		t.Fatalf("CA-155: en resume el sandbox viaja igual (no hay -s): %v", args)
	}
	if strings.Contains(strings.Join(args, " "), " -s ") {
		t.Fatalf("CA-155: no se usa -s, que resume no acepta: %v", args)
	}
	for _, a := range codexArgs(t, Request{Prompt: "implementa"}) {
		if strings.HasPrefix(a, "sandbox_mode=") {
			t.Fatal("CA-155: un rol de escritura no pisa la config del usuario")
		}
	}
}

// CA-156: Normalize del JSONL de codex. Ninguna linea se pierde: lo que no
// esta mapeado degrada a text NOMBRANDO su tipo.
func TestCA156_NormalizeDeCodex(t *testing.T) {
	p, _ := Lookup("codex")
	uno := func(line string) Event {
		t.Helper()
		evs := p.Normalize(line)
		if len(evs) != 1 {
			t.Fatalf("CA-156: se esperaba UN evento de %s: %+v", line, evs)
		}
		return evs[0]
	}

	if ev := uno(`{"type":"thread.started","thread_id":"01a0-abc"}`); ev.Kind != "start" || ev.SessionID != "01a0-abc" {
		t.Fatalf("CA-156: thread.started abre la sesion con su id: %+v", ev)
	}
	if ev := uno(`{"type":"turn.completed","usage":{"input_tokens":34739,"output_tokens":216}}`); ev.Kind != "end" || !strings.Contains(ev.Detail, "34739") {
		t.Fatalf("CA-156: turn.completed cierra y dice el gasto: %+v", ev)
	}
	if ev := uno(`{"type":"turn.failed","error":{"message":"400 invalid_request"}}`); ev.Kind != "error" || !strings.Contains(ev.Detail, "400") {
		t.Fatalf("CA-156: turn.failed es error con su mensaje: %+v", ev)
	}
	if ev := uno(`{"type":"error","message":"se cayo la red"}`); ev.Kind != "error" || ev.Detail != "se cayo la red" {
		t.Fatalf("CA-156: el error de nivel superior es error: %+v", ev)
	}
	if ev := uno(`{"type":"item.completed","item":{"id":"i2","type":"agent_message","text":"listo"}}`); ev.Kind != "text" || ev.Detail != "listo" {
		t.Fatalf("CA-156: agent_message es text: %+v", ev)
	}
	if ev := uno(`{"type":"item.completed","item":{"id":"i0","type":"error","message":"hooks duplicados"}}`); ev.Kind != "error" {
		t.Fatalf("CA-156: un item error es error: %+v", ev)
	}
	if ev := uno(`{"type":"item.started","item":{"id":"i1","type":"command_execution","command":"/bin/zsh -lc ls","status":"in_progress"}}`); ev.Kind != "tool" || !strings.Contains(ev.Detail, "ls") {
		t.Fatalf("CA-156: una accion se narra cuando arranca: %+v", ev)
	}
	if ev := uno(`{"type":"item.started","item":{"id":"i1","type":"file_change","changes":[{"path":"/tmp/b.txt","kind":"add"}],"status":"in_progress"}}`); ev.Kind != "tool" || !strings.Contains(ev.Detail, "add /tmp/b.txt") {
		t.Fatalf("CA-156: file_change nombra la ruta y el tipo de cambio: %+v", ev)
	}

	// una accion que salio bien no se narra dos veces
	if evs := p.Normalize(`{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"ls","exit_code":0,"status":"completed"}}`); len(evs) != 0 {
		t.Fatalf("CA-156: el exito ya se narro al arrancar: %+v", evs)
	}
	// ...y una que fallo, si
	if ev := uno(`{"type":"item.completed","item":{"id":"i1","type":"command_execution","command":"ls x","exit_code":2,"status":"completed"}}`); ev.Kind != "tool" || !strings.Contains(ev.Detail, "exit 2") {
		t.Fatalf("CA-156: el fallo es noticia: %+v", ev)
	}

	// lo que no esta mapeado se nombra, no se adivina ni se pierde
	ev := uno(`{"type":"item.completed","item":{"id":"i3","type":"web_search","query":"golang toml"}}`)
	if ev.Kind != "text" || !strings.HasPrefix(ev.Detail, "web_search: ") || !strings.Contains(ev.Detail, "golang toml") {
		t.Fatalf("CA-156: un tipo no mapeado se nombra: %+v", ev)
	}
	if evs := p.Normalize(`{"type":"item.started","item":{"id":"i3","type":"web_search","query":"x"}}`); len(evs) != 0 {
		t.Fatalf("CA-156: un tipo no mapeado narra UNA vez, al completarse: %+v", evs)
	}

	// silencio a proposito
	for _, line := range []string{`{"type":"turn.started"}`, `{"type":"item.updated","item":{"id":"i1","type":"command_execution"}}`} {
		if evs := p.Normalize(line); len(evs) != 0 {
			t.Fatalf("CA-156: %s no narra: %+v", line, evs)
		}
	}

	// una linea que no es JSON, o un JSON de un tipo desconocido: text integro
	raw := "esto no es json"
	if ev := uno(raw); ev.Kind != "text" || ev.Detail != raw {
		t.Fatalf("CA-156: una linea que no es JSON viaja integra: %+v", ev)
	}
	desconocido := `{"type":"cosa.nueva","payload":1}`
	if ev := uno(desconocido); ev.Kind != "text" || ev.Detail != desconocido {
		t.Fatalf("CA-156: un tipo desconocido viaja integro: %+v", ev)
	}
}
