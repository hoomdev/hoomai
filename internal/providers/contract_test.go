// Tests adversariales del spec .hoom/specs/providers-v2-interfaz-y-claude.md
// (CA-109, CA-110, CA-111, CA-112): suite de contrato consciente de
// capacidades que corre sobre TODOS los providers de Default y sobre dos
// fakes extremos (nueve capacidades / ninguna) que se prueba a si misma.
package providers

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// ctCaps arma unas Capabilities con los nueve booleanos en el mismo valor.
func ctCaps(v bool) Capabilities {
	return Capabilities{
		Structured:   v,
		Continue:     v,
		Resume:       v,
		SessionID:    v,
		Model:        v,
		SystemPrompt: v,
		Tools:        v,
		MaxTurns:     v,
		Budget:       v,
	}
}

// capFake es un Provider de juguete que cumple LAS REGLAS COMUNES de Command
// de la spec: prompt ultimo, nombres canonicos en Ignored, Strict devuelve
// ErrUnsupported. Un fake con todo en true nunca ignora nada; uno con todo en
// false ignora (o rechaza con Strict) cada campo enviado. Asi la suite se
// prueba a si misma sobre los dos extremos.
type capFake struct {
	bin  string
	caps Capabilities
}

func (f capFake) Name() string               { return f.bin }
func (f capFake) Bin() string                { return f.bin }
func (f capFake) Capabilities() Capabilities { return f.caps }
func (f capFake) Normalize(line string) []Event {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	return []Event{{Kind: "text", Detail: line}}
}

func (f capFake) Command(req Request) (Invocation, error) {
	if strings.TrimSpace(req.Prompt) == "" {
		return Invocation{}, errors.New("prompt vacio")
	}
	if strings.HasPrefix(req.Prompt, "-") {
		return Invocation{}, errors.New("prompt no puede empezar con -")
	}
	if strings.HasPrefix(req.ResumeID, "-") {
		return Invocation{}, errors.New("resume id no puede empezar con -")
	}
	if req.MaxTurns < 0 {
		return Invocation{}, errors.New("max_turns negativo")
	}
	if req.BudgetUSD < 0 {
		return Invocation{}, errors.New("budget negativo")
	}

	allow := ctPrune(req.AllowTools)
	deny := ctPrune(req.DenyTools)

	var args []string
	var missing []string

	// Precedencia de sesion: resume por id gana; si no, continue; si ninguna
	// aplica, lo pedido y no honrado cae a Ignored (en orden canonico).
	usedResume := req.ResumeID != "" && f.caps.Resume
	if usedResume {
		args = append(args, "--resume", req.ResumeID)
	}
	usedContinue := !usedResume && req.Continue && f.caps.Continue
	if usedContinue {
		args = append(args, "--continue")
	}
	if req.ResumeID != "" && !usedResume && !usedContinue {
		missing = append(missing, "resume")
	}
	if req.Continue && !usedResume && !usedContinue {
		missing = append(missing, "continue")
	}
	if req.Model != "" {
		if f.caps.Model {
			args = append(args, "--model", req.Model)
		} else {
			missing = append(missing, "model")
		}
	}
	if req.SystemPrompt != "" {
		if f.caps.SystemPrompt {
			args = append(args, "--append-system-prompt", req.SystemPrompt)
		} else {
			missing = append(missing, "system_prompt")
		}
	}
	if len(allow) > 0 || len(deny) > 0 {
		if f.caps.Tools {
			if len(allow) > 0 {
				args = append(args, "--allow", strings.Join(allow, ","))
			}
			if len(deny) > 0 {
				args = append(args, "--deny", strings.Join(deny, ","))
			}
		} else {
			missing = append(missing, "tools") // allow y deny comparten un solo canonico
		}
	}
	if req.MaxTurns > 0 {
		if f.caps.MaxTurns {
			args = append(args, "--max-turns", strconv.Itoa(req.MaxTurns))
		} else {
			missing = append(missing, "max_turns")
		}
	}
	if req.BudgetUSD > 0 {
		if f.caps.Budget {
			args = append(args, "--max-budget-usd", strconv.FormatFloat(req.BudgetUSD, 'f', -1, 64))
		} else {
			missing = append(missing, "budget")
		}
	}

	if req.Strict && len(missing) > 0 {
		return Invocation{}, ErrUnsupported{Provider: f.bin, Fields: missing}
	}
	args = append(args, req.Prompt)
	return Invocation{Bin: f.bin, Args: args, Ignored: missing}, nil
}

func ctPrune(in []string) []string {
	var out []string
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}

// providersUnderTest: los cuatro providers reales de Default mas los dos
// fakes extremos. La suite de contrato corre identica sobre todos.
func providersUnderTest() []Provider {
	ps := append([]Provider{}, All()...)
	ps = append(ps, capFake{bin: "fake-todo", caps: ctCaps(true)}, capFake{bin: "fake-nada", caps: ctCaps(false)})
	return ps
}

// --- helpers compartidos del paquete de tests providers ---

func ctHas(sl []string, s string) bool {
	for _, x := range sl {
		if x == s {
			return true
		}
	}
	return false
}

func ctIndex(sl []string, s string) int {
	for i, x := range sl {
		if x == s {
			return i
		}
	}
	return -1
}

func ctCount(sl []string, s string) int {
	n := 0
	for _, x := range sl {
		if x == s {
			n++
		}
	}
	return n
}

func ctSameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := map[string]int{}
	for _, s := range a {
		m[s]++
	}
	for _, s := range b {
		m[s]--
	}
	for _, v := range m {
		if v != 0 {
			return false
		}
	}
	return true
}

// CA-109: reglas comunes de Command sobre todos los providers y los fakes:
// prompt vacio o que empieza con `-` es error; un prompt valido devuelve el
// Bin del provider, el prompt como ultimo argumento exactamente una vez,
// Ignored vacio y ningun argumento vacio.
func TestCA109_ReglasComunesDeCommand(t *testing.T) {
	const prompt = "prompt-unico-xyz"
	for _, p := range providersUnderTest() {
		if _, err := p.Command(Request{Prompt: ""}); err == nil {
			t.Fatalf("CA-109: %s: prompt vacio debe ser error", p.Name())
		}
		if _, err := p.Command(Request{Prompt: "   "}); err == nil {
			t.Fatalf("CA-109: %s: prompt en blanco debe ser error", p.Name())
		}
		if _, err := p.Command(Request{Prompt: "-r"}); err == nil {
			t.Fatalf("CA-109: %s: prompt que empieza con - debe ser error (evita leerlo como flag)", p.Name())
		}

		inv, err := p.Command(Request{Prompt: prompt})
		if err != nil {
			t.Fatalf("CA-109: %s: prompt valido no debe fallar: %v", p.Name(), err)
		}
		if inv.Bin != p.Bin() {
			t.Fatalf("CA-109: %s: el Bin de la invocacion debe ser %q, es %q", p.Name(), p.Bin(), inv.Bin)
		}
		if len(inv.Ignored) != 0 {
			t.Fatalf("CA-109: %s: sin campos opcionales Ignored debe ser vacio: %v", p.Name(), inv.Ignored)
		}
		if len(inv.Args) == 0 || inv.Args[len(inv.Args)-1] != prompt {
			t.Fatalf("CA-109: %s: el prompt debe ser el ultimo argumento: %v", p.Name(), inv.Args)
		}
		if ctCount(inv.Args, prompt) != 1 {
			t.Fatalf("CA-109: %s: el prompt debe aparecer exactamente una vez: %v", p.Name(), inv.Args)
		}
		for _, a := range inv.Args {
			if a == "" {
				t.Fatalf("CA-109: %s: ningun argumento puede ser la cadena vacia: %v", p.Name(), inv.Args)
			}
		}
	}
}

// CA-110: por cada campo opcional enviado solo, el comportamiento depende de
// la capacidad declarada: capacidad verdadera => los args difieren de la
// invocacion plana y el campo NO esta en Ignored; capacidad falsa => el campo
// aparece en Ignored con su nombre canonico y los args son los planos.
// AllowTools y DenyTools juntos sin capacidad producen `tools` una sola vez.
func TestCA110_CampoOpcionalSegunCapacidad(t *testing.T) {
	const prompt = "prompt-unico-xyz"
	type fieldCase struct {
		canon string
		has   func(Capabilities) bool
		set   func(*Request)
	}
	cases := []fieldCase{
		{"resume", func(c Capabilities) bool { return c.Resume }, func(r *Request) { r.ResumeID = "sess-xyz" }},
		{"continue", func(c Capabilities) bool { return c.Continue }, func(r *Request) { r.Continue = true }},
		{"model", func(c Capabilities) bool { return c.Model }, func(r *Request) { r.Model = "modelo-x" }},
		{"system_prompt", func(c Capabilities) bool { return c.SystemPrompt }, func(r *Request) { r.SystemPrompt = "texto de rol" }},
		{"tools", func(c Capabilities) bool { return c.Tools }, func(r *Request) { r.AllowTools = []string{"Read"} }},
		{"max_turns", func(c Capabilities) bool { return c.MaxTurns }, func(r *Request) { r.MaxTurns = 3 }},
		{"budget", func(c Capabilities) bool { return c.Budget }, func(r *Request) { r.BudgetUSD = 0.5 }},
	}
	for _, p := range providersUnderTest() {
		caps := p.Capabilities()
		flat, err := p.Command(Request{Prompt: prompt})
		if err != nil {
			t.Fatalf("CA-110: %s: invocacion plana fallo: %v", p.Name(), err)
		}
		for _, fc := range cases {
			req := Request{Prompt: prompt}
			fc.set(&req)
			inv, err := p.Command(req)
			if err != nil {
				t.Fatalf("CA-110: %s: campo %s sin Strict no debe fallar: %v", p.Name(), fc.canon, err)
			}
			has := ctHas(inv.Ignored, fc.canon)
			if fc.has(caps) {
				if has {
					t.Fatalf("CA-110: %s: soporta %s, no debe ir a Ignored: %v", p.Name(), fc.canon, inv.Ignored)
				}
				if reflect.DeepEqual(inv.Args, flat.Args) {
					t.Fatalf("CA-110: %s: soporta %s, los args deben diferir de la invocacion plana: %v", p.Name(), fc.canon, inv.Args)
				}
			} else {
				if !has {
					t.Fatalf("CA-110: %s: no soporta %s, debe ir a Ignored con su nombre canonico: %v", p.Name(), fc.canon, inv.Ignored)
				}
				if !reflect.DeepEqual(inv.Args, flat.Args) {
					t.Fatalf("CA-110: %s: no soporta %s, los args deben ser los de la invocacion plana: %v vs %v", p.Name(), fc.canon, inv.Args, flat.Args)
				}
			}
		}
		if !caps.Tools {
			req := Request{Prompt: prompt, AllowTools: []string{"Read"}, DenyTools: []string{"Bash"}}
			inv, err := p.Command(req)
			if err != nil {
				t.Fatalf("CA-110: %s: allow+deny sin Strict no debe fallar: %v", p.Name(), err)
			}
			if ctCount(inv.Ignored, "tools") != 1 {
				t.Fatalf("CA-110: %s: allow+deny sin soporte debe producir 'tools' una sola vez: %v", p.Name(), inv.Ignored)
			}
		}
	}
}

// CA-111: con Strict, un campo no soportado devuelve ErrUnsupported nombrando
// el provider y TODOS los campos no soportados, sin Invocation; sin Strict, la
// misma request produce Invocation con esos mismos campos en Ignored.
func TestCA111_StrictVsIgnored(t *testing.T) {
	const prompt = "prompt-unico-xyz"
	for _, p := range providersUnderTest() {
		caps := p.Capabilities()
		req := Request{
			Prompt:       prompt,
			ResumeID:     "sess-xyz",
			Model:        "modelo-x",
			SystemPrompt: "texto de rol",
			AllowTools:   []string{"Read"},
			DenyTools:    []string{"Bash"},
			MaxTurns:     3,
			BudgetUSD:    0.5,
		}
		var expected []string
		if !caps.Resume {
			expected = append(expected, "resume")
		}
		if !caps.Model {
			expected = append(expected, "model")
		}
		if !caps.SystemPrompt {
			expected = append(expected, "system_prompt")
		}
		if !caps.Tools {
			expected = append(expected, "tools")
		}
		if !caps.MaxTurns {
			expected = append(expected, "max_turns")
		}
		if !caps.Budget {
			expected = append(expected, "budget")
		}

		reqS := req
		reqS.Strict = true
		invS, errS := p.Command(reqS)
		if len(expected) > 0 {
			var e ErrUnsupported
			if !errors.As(errS, &e) {
				t.Fatalf("CA-111: %s: con Strict y campos no soportados debe devolver ErrUnsupported, dio: %v", p.Name(), errS)
			}
			if e.Provider != p.Name() {
				t.Fatalf("CA-111: %s: ErrUnsupported debe nombrar el provider, nombra %q", p.Name(), e.Provider)
			}
			if !ctSameSet(e.Fields, expected) {
				t.Fatalf("CA-111: %s: ErrUnsupported debe listar TODOS los no soportados: got %v want %v", p.Name(), e.Fields, expected)
			}
			if len(invS.Args) != 0 || len(invS.Ignored) != 0 {
				t.Fatalf("CA-111: %s: con Strict no debe devolver Invocation: %+v", p.Name(), invS)
			}
		} else if errS != nil {
			t.Fatalf("CA-111: %s: todo soportado, Strict no debe fallar: %v", p.Name(), errS)
		}

		inv, err := p.Command(req)
		if err != nil {
			t.Fatalf("CA-111: %s: sin Strict no debe fallar: %v", p.Name(), err)
		}
		if !ctSameSet(inv.Ignored, expected) {
			t.Fatalf("CA-111: %s: sin Strict los mismos campos van a Ignored: got %v want %v", p.Name(), inv.Ignored, expected)
		}
	}
}

// CA-112: Normalize en todos los providers: linea vacia o solo espacios no
// produce eventos; linea no reconocida es exactamente un text con la linea
// integra; un provider sin structured degrada hasta un JSON valido a text; una
// linea de 1 MB no entra en panico y produce al menos un evento.
func TestCA112_NormalizeComun(t *testing.T) {
	for _, p := range providersUnderTest() {
		if evs := p.Normalize(""); len(evs) != 0 {
			t.Fatalf("CA-112: %s: linea vacia no debe producir eventos: %v", p.Name(), evs)
		}
		if evs := p.Normalize("   \t "); len(evs) != 0 {
			t.Fatalf("CA-112: %s: linea en blanco no debe producir eventos: %v", p.Name(), evs)
		}

		const raw = "esta linea no es json y no debe perderse"
		evs := p.Normalize(raw)
		if len(evs) != 1 || evs[0].Kind != "text" || evs[0].Detail != raw {
			t.Fatalf("CA-112: %s: linea no reconocida debe ser un unico text con la linea integra: %v", p.Name(), evs)
		}

		if !p.Capabilities().Structured {
			evs := p.Normalize(`{"type":"assistant","message":{"content":[]}}`)
			if len(evs) != 1 || evs[0].Kind != "text" {
				t.Fatalf("CA-112: %s: sin structured hasta un JSON valido es text: %v", p.Name(), evs)
			}
		}

		big := strings.Repeat("a", 1<<20)
		evs = p.Normalize(big)
		if len(evs) < 1 {
			t.Fatalf("CA-112: %s: una linea de 1 MB debe producir al menos un evento", p.Name())
		}
	}
}
