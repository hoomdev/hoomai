// Tests adversariales del spec .hoom/specs/providers-v2-interfaz-y-claude.md
// (CA-113, CA-114, CA-115, CA-116, CA-117): el adapter Claude v2 traduce cada
// opcion al flag exacto del CLI (verificado contra Claude Code 2.1.259) y
// normaliza el stream-json a eventos con SessionID.
package providers

import (
	"reflect"
	"strings"
	"testing"
)

// CA-113: sin opciones, el comando base es exactamente
// `-p --output-format stream-json --verbose <prompt>` y las nueve capacidades
// se declaran verdaderas.
func TestCA113_ClaudeBaseYCapacidades(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	inv, err := p.Command(Request{Prompt: "hola"})
	if err != nil {
		t.Fatalf("CA-113: Command base no debe fallar: %v", err)
	}
	if inv.Bin != p.Bin() {
		t.Fatalf("CA-113: el Bin debe ser %q, es %q", p.Bin(), inv.Bin)
	}
	want := []string{"-p", "--output-format", "stream-json", "--verbose", "hola"}
	if !reflect.DeepEqual(inv.Args, want) {
		t.Fatalf("CA-113: el comando base debe ser exactamente %v, es %v", want, inv.Args)
	}
	if len(inv.Ignored) != 0 {
		t.Fatalf("CA-113: sin opciones no hay nada que ignorar: %v", inv.Ignored)
	}
	c := p.Capabilities()
	if !(c.Structured && c.Continue && c.Resume && c.SessionID && c.Model && c.SystemPrompt && c.Tools && c.MaxTurns && c.Budget) {
		t.Fatalf("CA-113: Claude debe declarar las nueve capacidades verdaderas: %+v", c)
	}
}

// CA-114: ResumeID => --resume <id> y sin --continue; Continue sin ResumeID =>
// --continue; ambos => solo --resume. Un ResumeID que empieza con `-` es error.
func TestCA114_ClaudeSesion(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}

	inv, err := p.Command(Request{Prompt: "x", ResumeID: "sess-1"})
	if err != nil {
		t.Fatal(err)
	}
	if i := ctIndex(inv.Args, "--resume"); i < 0 || inv.Args[i+1] != "sess-1" {
		t.Fatalf("CA-114: ResumeID debe dar --resume <id>: %v", inv.Args)
	}
	if ctIndex(inv.Args, "--continue") >= 0 {
		t.Fatalf("CA-114: con ResumeID no debe aparecer --continue: %v", inv.Args)
	}

	inv, err = p.Command(Request{Prompt: "x", Continue: true})
	if err != nil {
		t.Fatal(err)
	}
	if ctIndex(inv.Args, "--continue") < 0 {
		t.Fatalf("CA-114: Continue sin ResumeID debe dar --continue: %v", inv.Args)
	}
	if ctIndex(inv.Args, "--resume") >= 0 {
		t.Fatalf("CA-114: Continue sin ResumeID no debe dar --resume: %v", inv.Args)
	}

	inv, err = p.Command(Request{Prompt: "x", ResumeID: "sess-1", Continue: true})
	if err != nil {
		t.Fatal(err)
	}
	if ctIndex(inv.Args, "--resume") < 0 {
		t.Fatalf("CA-114: con ambos debe ganar --resume: %v", inv.Args)
	}
	if ctIndex(inv.Args, "--continue") >= 0 {
		t.Fatalf("CA-114: con ambos NO debe haber --continue (resume gana): %v", inv.Args)
	}

	if _, err := p.Command(Request{Prompt: "x", ResumeID: "-malicioso"}); err == nil {
		t.Fatal("CA-114: un ResumeID que empieza con - debe ser error")
	}
}

// CA-115: Model => --model <m>; SystemPrompt => --append-system-prompt <texto>
// y jamas --system-prompt; MaxTurns => --max-turns <n>; BudgetUSD =>
// --max-budget-usd <n> decimal sin notacion cientifica; turnos o presupuesto
// negativos => error.
func TestCA115_ClaudeOpcionesEscalares(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}

	inv, _ := p.Command(Request{Prompt: "x", Model: "sonnet"})
	if i := ctIndex(inv.Args, "--model"); i < 0 || inv.Args[i+1] != "sonnet" {
		t.Fatalf("CA-115: Model debe dar --model <m>: %v", inv.Args)
	}

	inv, _ = p.Command(Request{Prompt: "x", SystemPrompt: "sos un rol"})
	if i := ctIndex(inv.Args, "--append-system-prompt"); i < 0 || inv.Args[i+1] != "sos un rol" {
		t.Fatalf("CA-115: SystemPrompt debe dar --append-system-prompt <texto>: %v", inv.Args)
	}
	if ctIndex(inv.Args, "--system-prompt") >= 0 {
		t.Fatalf("CA-115: nunca debe usar --system-prompt (reemplazaria el prompt del CLI): %v", inv.Args)
	}

	inv, _ = p.Command(Request{Prompt: "x", MaxTurns: 5})
	if i := ctIndex(inv.Args, "--max-turns"); i < 0 || inv.Args[i+1] != "5" {
		t.Fatalf("CA-115: MaxTurns debe dar --max-turns <n>: %v", inv.Args)
	}

	// Presupuesto decimal comun.
	inv, _ = p.Command(Request{Prompt: "x", BudgetUSD: 0.1})
	if i := ctIndex(inv.Args, "--max-budget-usd"); i < 0 || inv.Args[i+1] != "0.1" {
		t.Fatalf("CA-115: BudgetUSD 0.1 debe dar --max-budget-usd 0.1: %v", inv.Args)
	}
	// Presupuesto muy chico: decimal, jamas 1e-07.
	inv, _ = p.Command(Request{Prompt: "x", BudgetUSD: 0.0000001})
	i := ctIndex(inv.Args, "--max-budget-usd")
	if i < 0 {
		t.Fatalf("CA-115: falta --max-budget-usd: %v", inv.Args)
	}
	val := inv.Args[i+1]
	if val != "0.0000001" || strings.ContainsAny(val, "eE") {
		t.Fatalf("CA-115: el presupuesto debe formatearse decimal, jamas notacion cientifica: %q", val)
	}

	if _, err := p.Command(Request{Prompt: "x", MaxTurns: -1}); err == nil {
		t.Fatal("CA-115: MaxTurns negativo debe ser error")
	}
	if _, err := p.Command(Request{Prompt: "x", BudgetUSD: -0.5}); err == nil {
		t.Fatal("CA-115: BudgetUSD negativo debe ser error")
	}
}

// CA-116: AllowTools/DenyTools => --allowedTools / --disallowedTools con UN
// argumento separado por comas, entradas vacias descartadas, ambos ubicados
// ANTES de -p, y el prompt sigue siendo el ultimo argumento.
func TestCA116_ClaudeTools(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}

	const prompt = "hola-prompt"
	inv, err := p.Command(Request{
		Prompt:     prompt,
		AllowTools: []string{"Read", "", "  ", "Bash"}, // vacias descartadas
		DenyTools:  []string{"Write"},
	})
	if err != nil {
		t.Fatal(err)
	}

	idxP := ctIndex(inv.Args, "-p")
	if idxP < 0 {
		t.Fatalf("CA-116: el comando debe seguir teniendo -p: %v", inv.Args)
	}
	idxAllow := ctIndex(inv.Args, "--allowedTools")
	if idxAllow < 0 || inv.Args[idxAllow+1] != "Read,Bash" {
		t.Fatalf("CA-116: --allowedTools debe ser UN argumento separado por comas y sin vacias: %v", inv.Args)
	}
	idxDeny := ctIndex(inv.Args, "--disallowedTools")
	if idxDeny < 0 || inv.Args[idxDeny+1] != "Write" {
		t.Fatalf("CA-116: --disallowedTools debe ser UN argumento separado por comas: %v", inv.Args)
	}
	if idxAllow > idxP || idxDeny > idxP {
		t.Fatalf("CA-116: allowedTools/disallowedTools deben ir ANTES de -p (si no, se tragan el prompt): %v", inv.Args)
	}
	if inv.Args[len(inv.Args)-1] != prompt {
		t.Fatalf("CA-116: el prompt debe seguir siendo el ultimo argumento: %v", inv.Args)
	}
	if ctCount(inv.Args, "") != 0 {
		t.Fatalf("CA-116: ningun argumento puede ser la cadena vacia: %v", inv.Args)
	}

	// Solo entradas vacias: cuenta como no enviado, sin flag y sin aviso.
	inv, err = p.Command(Request{Prompt: prompt, AllowTools: []string{"", "  "}})
	if err != nil {
		t.Fatal(err)
	}
	if ctIndex(inv.Args, "--allowedTools") >= 0 {
		t.Fatalf("CA-116: AllowTools solo con vacias no debe producir flag: %v", inv.Args)
	}
	if ctHas(inv.Ignored, "tools") {
		t.Fatalf("CA-116: AllowTools solo con vacias cuenta como no enviado, sin aviso: %v", inv.Ignored)
	}
}

// CA-117: Normalize de Claude: system/init con session_id => start con
// SessionID; result success => end con el texto y SessionID; result con
// is_error o subtype de error => error con `<subtype>: <texto>`; init sin
// session_id => start sin SessionID.
func TestCA117_ClaudeNormalize(t *testing.T) {
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}

	evs := p.Normalize(`{"type":"system","subtype":"init","session_id":"sess-1"}`)
	if len(evs) != 1 || evs[0].Kind != "start" || evs[0].SessionID != "sess-1" {
		t.Fatalf("CA-117: init con session_id debe ser start con SessionID: %+v", evs)
	}

	evs = p.Normalize(`{"type":"result","subtype":"success","is_error":false,"result":"listo","session_id":"sess-1"}`)
	if len(evs) != 1 || evs[0].Kind != "end" || evs[0].Detail != "listo" || evs[0].SessionID != "sess-1" {
		t.Fatalf("CA-117: result success debe ser end con el texto y SessionID: %+v", evs)
	}

	evs = p.Normalize(`{"type":"result","subtype":"error_max_turns","is_error":true,"session_id":"sess-1"}`)
	if len(evs) != 1 || evs[0].Kind != "error" || evs[0].SessionID != "sess-1" || !strings.Contains(evs[0].Detail, "error_max_turns") {
		t.Fatalf("CA-117: result de error debe ser error con `<subtype>: <texto>` y SessionID: %+v", evs)
	}

	// Defensivo: is_error true aunque el subtype diga success => error.
	evs = p.Normalize(`{"type":"result","subtype":"success","is_error":true,"session_id":"sess-1"}`)
	if len(evs) != 1 || evs[0].Kind != "error" {
		t.Fatalf("CA-117: is_error true debe ser error aunque el subtype sea success: %+v", evs)
	}

	// Init sin session_id: start sin SessionID (CLI vieja / provider sin la capacidad).
	evs = p.Normalize(`{"type":"system","subtype":"init"}`)
	if len(evs) != 1 || evs[0].Kind != "start" || evs[0].SessionID != "" {
		t.Fatalf("CA-117: init sin session_id debe ser start sin SessionID: %+v", evs)
	}
}
