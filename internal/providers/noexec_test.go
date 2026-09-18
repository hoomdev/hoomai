// Tests adversariales del spec .hoom/specs/arquitecto-bajo-el-sobre.md
// (CA-231): la intencion NoExec. Un rol que escribe sin shell es una
// intencion del vocabulario comun, aditiva: el valor cero conserva el
// comportamiento de hoy, y cada adapter la traduce o la declara ignorada.
package providers

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func abLista(csv string) []string {
	if csv == "" {
		return nil
	}
	return strings.Split(csv, ",")
}

// CA-231: la capacidad se nombra no_exec, justo despues de read_only.
func TestCA231_NoExecSeNombraDespuesDeReadOnly(t *testing.T) {
	if FieldNoExec != "no_exec" {
		t.Fatalf("CA-231: el nombre canonico es no_exec, es %q", FieldNoExec)
	}
	caps := Capabilities{Tools: true, ReadOnly: true, NoExec: true, MaxTurns: true}
	if got := strings.Join(caps.Names(), ","); got != "tools,read_only,no_exec,max_turns" {
		t.Fatalf("CA-231: no_exec va justo despues de read_only: %q", got)
	}
	// sin read_only conserva el mismo lugar relativo
	caps = Capabilities{Tools: true, NoExec: true, Unattended: true}
	if got := strings.Join(caps.Names(), ","); got != "tools,no_exec,unattended" {
		t.Fatalf("CA-231: no_exec va antes de unattended: %q", got)
	}
	// el valor cero no la nombra
	if contains(Capabilities{ReadOnly: true}.Names(), "no_exec") {
		t.Fatal("CA-231: sin la capacidad no se nombra")
	}
	raw, err := json.Marshal(Capabilities{NoExec: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"no_exec":true`) {
		t.Fatalf("CA-231: el JSON de la capacidad es no_exec: %s", raw)
	}
}

// CA-231: Claude la declara; Codex, OpenCode y Gemini no.
func TestCA231_SoloClaudeDeclaraNoExec(t *testing.T) {
	quiere := map[string]bool{"claude": true, "codex": false, "opencode": false, "gemini": false}
	for name, want := range quiere {
		p, err := Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		if p.Capabilities().NoExec != want {
			t.Fatalf("CA-231: %s NoExec deberia ser %v", name, want)
		}
	}
	claude, _ := Lookup("claude")
	names := claude.Capabilities().Names()
	for i, n := range names {
		if n == "read_only" {
			if i+1 >= len(names) || names[i+1] != "no_exec" {
				t.Fatalf("CA-231: claude nombra no_exec justo despues de read_only: %v", names)
			}
		}
	}
	if !contains(names, "no_exec") {
		t.Fatalf("CA-231: claude debe nombrar no_exec: %v", names)
	}
}

// CA-231: Claude con Unattended y NoExec concede la escritura por nombre y
// niega el shell; el prompt sigue ultimo.
func TestCA231_ClaudeEscribeSinShell(t *testing.T) {
	claude, _ := Lookup("claude")
	inv, err := claude.Command(Request{Prompt: "escribi el spec", Unattended: true, NoExec: true, Strict: true})
	if err != nil {
		t.Fatalf("CA-231: claude declara no_exec, bajo Strict no puede fallar: %v", err)
	}
	if len(inv.Ignored) != 0 {
		t.Fatalf("CA-231: nada que ignorar en claude: %v", inv.Ignored)
	}
	allow := abLista(flagValue(inv.Args, "--allowedTools"))
	deny := abLista(flagValue(inv.Args, "--disallowedTools"))
	for _, want := range []string{"Edit", "Write"} {
		if !contains(allow, want) {
			t.Fatalf("CA-231: %s debe estar en --allowedTools: %v", want, inv.Args)
		}
		if contains(deny, want) {
			t.Fatalf("CA-231: un rol que escribe no puede tener %s prohibido: %v", want, inv.Args)
		}
	}
	if contains(allow, "Bash") {
		t.Fatalf("CA-231: Bash no puede estar en --allowedTools: %v", inv.Args)
	}
	if !contains(deny, "Bash") {
		t.Fatalf("CA-231: Bash debe estar en --disallowedTools: %v", inv.Args)
	}
	// el contrato fija el set exacto: el de escritura de hoy menos Bash
	if got := flagValue(inv.Args, "--allowedTools"); got != "Read,Grep,Glob,Edit,Write,MultiEdit,NotebookEdit" {
		t.Fatalf("CA-231: --allowedTools debe ser el set de escritura sin Bash: %q", got)
	}
	if inv.Args[len(inv.Args)-1] != "escribi el spec" {
		t.Fatalf("CA-231: el prompt debe seguir ultimo: %v", inv.Args)
	}

	// sin Unattended: igual se niega el shell, y no se niega la escritura
	inv, err = claude.Command(Request{Prompt: "escribi el spec", NoExec: true, Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	deny = abLista(flagValue(inv.Args, "--disallowedTools"))
	if !contains(deny, "Bash") {
		t.Fatalf("CA-231: con NoExec y sin Unattended, Bash sigue prohibido: %v", inv.Args)
	}
	for _, w := range []string{"Edit", "Write", "MultiEdit"} {
		if contains(deny, w) {
			t.Fatalf("CA-231: NoExec no prohibe la escritura (%s): %v", w, inv.Args)
		}
	}
	if contains(abLista(flagValue(inv.Args, "--allowedTools")), "Bash") {
		t.Fatalf("CA-231: Bash no puede estar permitido y prohibido a la vez: %v", inv.Args)
	}
	if inv.Args[len(inv.Args)-1] != "escribi el spec" {
		t.Fatalf("CA-231: el prompt debe seguir ultimo: %v", inv.Args)
	}

	// el valor cero conserva lo de hoy: el writer desatendido sigue con Bash
	inv, _ = claude.Command(Request{Prompt: "implementa", Unattended: true})
	if !contains(abLista(flagValue(inv.Args, "--allowedTools")), "Bash") ||
		flagValue(inv.Args, "--disallowedTools") != "" {
		t.Fatalf("CA-231: sin NoExec el rol que escribe conserva su shell: %v", inv.Args)
	}
}

// CA-231: bajo ReadOnly, NoExec no dice nada: el argv es identico al de
// ReadOnly solo, con y sin Exec, con y sin Unattended.
func TestCA231_NoExecConReadOnlyNoCambiaElArgv(t *testing.T) {
	claude, _ := Lookup("claude")
	for _, exec := range []bool{false, true} {
		for _, unattended := range []bool{false, true} {
			base := Request{Prompt: "lee", ReadOnly: true, Exec: exec, Unattended: unattended, Strict: true}
			con := base
			con.NoExec = true
			a, errA := claude.Command(base)
			b, errB := claude.Command(con)
			if errA != nil || errB != nil {
				t.Fatalf("CA-231: exec=%v unattended=%v: %v / %v", exec, unattended, errA, errB)
			}
			if !reflect.DeepEqual(a.Args, b.Args) {
				t.Fatalf("CA-231: exec=%v unattended=%v: NoExec bajo ReadOnly cambio el argv:\n%v\n%v",
					exec, unattended, a.Args, b.Args)
			}
			if !reflect.DeepEqual(a.Ignored, b.Ignored) {
				t.Fatalf("CA-231: NoExec bajo ReadOnly no puede ignorarse distinto: %v vs %v", a.Ignored, b.Ignored)
			}
		}
	}
}

// CA-231: sin la capacidad se ignora con su nombre canonico, y bajo Strict es
// ErrUnsupported que la nombra; sin NoExec no se nombra nada.
func TestCA231_SinCapacidadSeIgnoraOSeNiega(t *testing.T) {
	for _, name := range []string{"codex", "opencode", "gemini"} {
		p, err := Lookup(name)
		if err != nil {
			t.Fatal(err)
		}
		inv, err := p.Command(Request{Prompt: "escribi el spec", NoExec: true})
		if err != nil {
			t.Fatalf("CA-231: %s sin Strict degrada, no falla: %v", name, err)
		}
		if len(inv.Ignored) != 1 || inv.Ignored[0] != FieldNoExec {
			t.Fatalf("CA-231: %s declara la degradacion como no_exec: %v", name, inv.Ignored)
		}
		if !strings.Contains(strings.Join(inv.Args, " "), "escribi el spec") {
			t.Fatalf("CA-231: %s igual lleva el pedido: %v", name, inv.Args)
		}

		_, err = p.Command(Request{Prompt: "escribi el spec", NoExec: true, Strict: true})
		var eu ErrUnsupported
		if !errors.As(err, &eu) || !contains(eu.Fields, FieldNoExec) {
			t.Fatalf("CA-231: %s bajo Strict se niega nombrando no_exec: %v", name, err)
		}

		// el valor cero no pide nada
		if inv, err := p.Command(Request{Prompt: "hola", Strict: true}); err != nil || contains(inv.Ignored, FieldNoExec) {
			t.Fatalf("CA-231: %s sin NoExec no nombra no_exec: %v %v", name, inv.Ignored, err)
		}
	}

	// codex con Unattended y NoExec: el sandbox de escritura sigue, y el shell
	// que no puede quitar se declara ignorado
	codex, _ := Lookup("codex")
	inv, err := codex.Command(Request{Prompt: "escribi el spec", Unattended: true, NoExec: true})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(inv.Ignored, FieldNoExec) {
		t.Fatalf("CA-231: codex no puede quitar el shell y lo declara: %v", inv.Ignored)
	}
	if !strings.Contains(strings.Join(inv.Args, " "), `sandbox_mode="workspace-write"`) {
		t.Fatalf("CA-231: codex sigue fijando workspace-write para un rol que escribe: %v", inv.Args)
	}
}
