// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-396..CA-399, CA-418): los providers ganan las capacidades effort e
// isolation; Codex y Claude traducen Isolated y Effort a sus flags sin tocar
// el argv de hoy cuando no se piden; un prompt de mas de 16 KiB viaja por
// stdin; y (enmienda 4) con Request.PromptStdin viaja por stdin a cualquier
// tamano.
package providers

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"unicode"
)

func raCmd(t *testing.T, name string, req Request) Invocation {
	t.Helper()
	p, err := Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	inv, err := p.Command(req)
	if err != nil {
		t.Fatalf("%s: Command(%+v) fallo: %v", name, req, err)
	}
	return inv
}

func raIndex(args []string, a string) int {
	for i, x := range args {
		if x == a {
			return i
		}
	}
	return -1
}

func raCount(args []string, a string) int {
	n := 0
	for _, x := range args {
		if x == a {
			n++
		}
	}
	return n
}

// raPar devuelve la posicion de flag seguida de valor, o -1.
func raPar(args []string, flag, valor string) int {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == valor {
			return i
		}
	}
	return -1
}

// raSobrantes devuelve lo que tiene b de mas respecto de a (multiconjunto) y
// si a esta entero dentro de b.
func raSobrantes(a, b []string) ([]string, bool) {
	cuenta := map[string]int{}
	for _, x := range b {
		cuenta[x]++
	}
	for _, x := range a {
		if cuenta[x] == 0 {
			return nil, false
		}
		cuenta[x]--
	}
	var out []string
	for _, x := range b {
		if cuenta[x] > 0 {
			out = append(out, x)
			cuenta[x]--
		}
	}
	return out, true
}

// ---------------------------------------------------------------- CA-396

// CA-396: codex y claude declaran effort e isolation, y Names() termina en
// effort,isolation (despues de budget, en el orden estable del JSON).
func TestCA396_CodexYClaudeTerminanEnEffortIsolation(t *testing.T) {
	for _, name := range []string{"codex", "claude"} {
		p, _ := Lookup(name)
		c := p.Capabilities()
		if !c.Effort || !c.Isolation {
			t.Fatalf("CA-396: %s declara effort e isolation: %+v", name, c)
		}
		names := c.Names()
		if len(names) < 2 || names[len(names)-2] != "effort" || names[len(names)-1] != "isolation" {
			t.Fatalf("CA-396: Names() de %s termina en effort,isolation: %v", name, names)
		}
		if !strings.HasSuffix(c.Summary(), "effort, isolation") {
			t.Fatalf("CA-396: 'hoom providers' lista effort e isolation al final para %s: %q", name, c.Summary())
		}
	}
	// el orden canonico: con todo encendido, budget, effort, isolation al final
	todo := Capabilities{Structured: true, Continue: true, Resume: true, SessionID: true, Model: true,
		SystemPrompt: true, Tools: true, ReadOnly: true, NoExec: true, Unattended: true, MaxTurns: true,
		Budget: true, Effort: true, Isolation: true}
	names := todo.Names()
	if len(names) != 14 || strings.Join(names[len(names)-3:], ",") != "budget,effort,isolation" {
		t.Fatalf("CA-396: effort e isolation van al final de Names(), despues de budget: %v", names)
	}
	// una sola de las dos tambien aparece, sola
	if got := (Capabilities{Isolation: true}).Names(); !reflect.DeepEqual(got, []string{"isolation"}) {
		t.Fatalf("CA-396: isolation sola se nombra: %v", got)
	}
	if got := (Capabilities{Effort: true}).Names(); !reflect.DeepEqual(got, []string{"effort"}) {
		t.Fatalf("CA-396: effort sola se nombra: %v", got)
	}
}

// CA-396: gemini sigue sin ninguna capacidad y opencode no gana ninguna (solo
// continue, como hoy). Guarda de regresion: el esqueleto ya lo cumple y la
// implementacion no puede romperlo al sumar effort/isolation a codex y claude.
func TestCA396_GeminiYOpencodeNoGananCapacidades(t *testing.T) {
	g, _ := Lookup("gemini")
	if g.Capabilities() != (Capabilities{}) || len(g.Capabilities().Names()) != 0 {
		t.Fatalf("CA-396: gemini no tiene capacidades: %+v", g.Capabilities())
	}
	o, _ := Lookup("opencode")
	if got := o.Capabilities().Names(); !reflect.DeepEqual(got, []string{"continue"}) {
		t.Fatalf("CA-396: opencode no gana ninguna capacidad: %v", got)
	}
}

// CA-396: pedir Effort o Isolated con Strict a un provider sin la capacidad es
// ErrUnsupported con esos campos, en orden canonico (despues de budget). Sin
// Strict se ignoran y se reportan, y el argv es el plano.
func TestCA396_SinLaCapacidadEsErrUnsupported(t *testing.T) {
	for _, name := range []string{"gemini", "opencode"} {
		p, _ := Lookup(name)
		casos := []struct {
			req    Request
			fields []string
		}{
			{Request{Prompt: "revisa", Effort: "high", Strict: true}, []string{FieldEffort}},
			{Request{Prompt: "revisa", Isolated: true, Strict: true}, []string{FieldIsolation}},
			{Request{Prompt: "revisa", Effort: "high", Isolated: true, Strict: true}, []string{FieldEffort, FieldIsolation}},
			{Request{Prompt: "revisa", Effort: "high", Isolated: true, BudgetUSD: 1, MaxTurns: 2, Strict: true},
				[]string{FieldMaxTurns, FieldBudget, FieldEffort, FieldIsolation}},
		}
		for _, k := range casos {
			_, err := p.Command(k.req)
			var eu ErrUnsupported
			if !errors.As(err, &eu) {
				t.Fatalf("CA-396: %s con Strict y %v es ErrUnsupported: %v", name, k.fields, err)
			}
			if eu.Provider != name || !reflect.DeepEqual(eu.Fields, k.fields) {
				t.Fatalf("CA-396: %s nombra los campos %v en orden canonico: %+v", name, k.fields, eu)
			}
		}

		plano, err := p.Command(Request{Prompt: "revisa"})
		if err != nil {
			t.Fatal(err)
		}
		inv, err := p.Command(Request{Prompt: "revisa", Effort: "high", Isolated: true})
		if err != nil {
			t.Fatalf("CA-396: sin Strict %s degrada, no falla: %v", name, err)
		}
		if !reflect.DeepEqual(inv.Ignored, []string{FieldEffort, FieldIsolation}) {
			t.Fatalf("CA-396: sin Strict %s reporta effort e isolation en Ignored: %v", name, inv.Ignored)
		}
		if !reflect.DeepEqual(inv.Args, plano.Args) {
			t.Fatalf("CA-396: lo ignorado no toca el argv de %s: %v vs %v", name, inv.Args, plano.Args)
		}
	}
	// codex y claude los honran con Strict: nada que negar
	for _, name := range []string{"codex", "claude"} {
		inv := raCmd(t, name, Request{Prompt: "revisa", Effort: "high", Isolated: true, Strict: true})
		if len(inv.Ignored) != 0 {
			t.Fatalf("CA-396: %s honra effort e isolation: %v", name, inv.Ignored)
		}
	}
}

// ---------------------------------------------------------------- CA-397

// CA-397: codex con Isolated y Effort "high" empieza con exec --json y lleva
// --ignore-user-config y -c model_reasoning_effort="high", despues de --json,
// con el prompt ultimo.
func TestCA397_CodexAisladoYConEsfuerzo(t *testing.T) {
	args := raCmd(t, "codex", Request{Prompt: "revisa el cambio", Isolated: true, Effort: "high",
		SystemPrompt: "# Reviewer", ReadOnly: true, Exec: true}).Args
	if len(args) < 2 || args[0] != "exec" || args[1] != "--json" {
		t.Fatalf("CA-397: el argv empieza con exec --json: %v", args)
	}
	i := raIndex(args, "--ignore-user-config")
	if i < 0 || raCount(args, "--ignore-user-config") != 1 {
		t.Fatalf("CA-397: Isolated agrega --ignore-user-config una vez: %v", args)
	}
	j := raPar(args, "-c", `model_reasoning_effort="high"`)
	if j < 0 {
		t.Fatalf(`CA-397: Effort "high" agrega -c model_reasoning_effort="high": %v`, args)
	}
	json := raIndex(args, "--json")
	if i < json || j < json {
		t.Fatalf("CA-397: los dos van despues de --json: %v", args)
	}
	if args[len(args)-1] != "revisa el cambio" {
		t.Fatalf("CA-397: el prompt sigue ultimo: %v", args)
	}

	// en resume por id, igual: despues de --json
	args = raCmd(t, "codex", Request{Prompt: "seguimos", ResumeID: "abc", Isolated: true, Effort: "high"}).Args
	if strings.Join(args[:4], " ") != "exec resume abc --json" {
		t.Fatalf("CA-397: el resume conserva su forma: %v", args)
	}
	if raIndex(args, "--ignore-user-config") < 3 || raPar(args, "-c", `model_reasoning_effort="high"`) < 3 {
		t.Fatalf("CA-397: en resume los dos van despues de --json: %v", args)
	}
}

// CA-397: cada campo solo agrega exactamente lo suyo; sin esos campos el argv
// es identico al de hoy (CA-152, CA-155).
func TestCA397_CodexCadaCampoAgregaSoloLoSuyo(t *testing.T) {
	bases := []Request{
		{Prompt: "hola"},
		{Prompt: "revisa", Model: "gpt-5", SystemPrompt: "C", ReadOnly: true, Exec: true},
		{Prompt: "implementa", Unattended: true},
		{Prompt: "seguimos", ResumeID: "abc", ReadOnly: true},
	}
	for _, base := range bases {
		plano := raCmd(t, "codex", base).Args

		iso := base
		iso.Isolated = true
		extra, ok := raSobrantes(plano, raCmd(t, "codex", iso).Args)
		if !ok || !reflect.DeepEqual(extra, []string{"--ignore-user-config"}) {
			t.Fatalf("CA-397: Isolated solo agrega --ignore-user-config a %v: extra=%v", plano, extra)
		}

		ef := base
		ef.Effort = "high"
		extra, ok = raSobrantes(plano, raCmd(t, "codex", ef).Args)
		if !ok || !reflect.DeepEqual(extra, []string{"-c", `model_reasoning_effort="high"`}) {
			t.Fatalf(`CA-397: Effort solo agrega -c model_reasoning_effort="high" a %v: extra=%v`, plano, extra)
		}
	}
	// el argv de hoy, literal
	if got := raCmd(t, "codex", Request{Prompt: "hola"}).Args; !reflect.DeepEqual(got, []string{"exec", "--json", "hola"}) {
		t.Fatalf("CA-397: sin esos campos el argv es el de hoy: %v", got)
	}
	want := []string{"exec", "--json", "-m", "gpt-5", "-c", `developer_instructions="C"`, "-c", `sandbox_mode="workspace-write"`, "revisa"}
	if got := raCmd(t, "codex", Request{Prompt: "revisa", Model: "gpt-5", SystemPrompt: "C", ReadOnly: true, Exec: true}).Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("CA-397: sin esos campos el argv del reviewer es el de hoy:\nquiere %v\nfue    %v", want, got)
	}
}

// CA-397 (propiedad): el esfuerzo viaja codificado como el resto de los -c
// (cadena basica TOML), sin validar el vocabulario: comillas, barras y
// unicode vuelven exactos.
func TestCA397_EsfuerzoCodificadoComoTOML(t *testing.T) {
	for _, e := range []string{`x"y`, `a\b`, "xhigh", "mínimo", "con espacio"} {
		args := raCmd(t, "codex", Request{Prompt: "hola", Effort: e}).Args
		if raPar(args, "-c", "model_reasoning_effort="+tomlString(e)) < 0 {
			t.Fatalf("CA-397: el esfuerzo %q viaja como model_reasoning_effort=%s: %v", e, tomlString(e), args)
		}
	}
	p, _ := Lookup("codex")
	prop := func(s string) bool {
		e := strings.TrimFunc(s, unicode.IsSpace)
		if e == "" {
			return true
		}
		inv, err := p.Command(Request{Prompt: "hola", Effort: e})
		return err == nil && raPar(inv.Args, "-c", "model_reasoning_effort="+tomlString(e)) >= 0
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200, Rand: rand.New(rand.NewSource(397))}); err != nil {
		t.Fatalf("CA-397: todo esfuerzo viaja codificado como TOML: %v", err)
	}
}

// ---------------------------------------------------------------- CA-398

// CA-398: claude con Isolated y Effort "high" lleva --strict-mcp-config,
// --setting-sources project y --effort high entre -p y el prompt.
func TestCA398_ClaudeAisladoYConEsfuerzo(t *testing.T) {
	for _, base := range []Request{
		{Prompt: "revisa el cambio"},
		{Prompt: "revisa el cambio", SystemPrompt: "# Reviewer", ReadOnly: true, Exec: true, Model: "opus", MaxTurns: 9, BudgetUSD: 2},
		{Prompt: "revisa el cambio", ResumeID: "sess-1"},
	} {
		req := base
		req.Isolated, req.Effort = true, "high"
		args := raCmd(t, "claude", req).Args
		p := raIndex(args, "-p")
		last := len(args) - 1
		if p < 0 || args[last] != "revisa el cambio" {
			t.Fatalf("CA-398: -p y el prompt ultimo: %v", args)
		}
		s := raIndex(args, "--strict-mcp-config")
		src := raPar(args, "--setting-sources", "project")
		ef := raPar(args, "--effort", "high")
		if s < 0 || src < 0 || ef < 0 {
			t.Fatalf("CA-398: faltan --strict-mcp-config, --setting-sources project o --effort high: %v", args)
		}
		for _, i := range []int{s, src, src + 1, ef, ef + 1} {
			if i <= p || i >= last {
				t.Fatalf("CA-398: los flags van despues de -p y antes del prompt: %v", args)
			}
		}
		if raCount(args, "--strict-mcp-config") != 1 || raCount(args, "--setting-sources") != 1 || raCount(args, "--effort") != 1 {
			t.Fatalf("CA-398: cada flag una sola vez: %v", args)
		}
	}
}

// CA-398: cada campo agrega solo lo suyo y, sin ellos, CA-113 tal cual.
func TestCA398_ClaudeCadaCampoAgregaSoloLoSuyo(t *testing.T) {
	want := []string{"-p", "--output-format", "stream-json", "--verbose", "hola"}
	if got := raCmd(t, "claude", Request{Prompt: "hola"}).Args; !reflect.DeepEqual(got, want) {
		t.Fatalf("CA-398: sin Isolated ni Effort el argv es el de CA-113: %v", got)
	}
	for _, base := range []Request{
		{Prompt: "hola"},
		{Prompt: "revisa", SystemPrompt: "C", ReadOnly: true, Exec: true},
		{Prompt: "implementa", Unattended: true, NoExec: true},
	} {
		plano := raCmd(t, "claude", base).Args
		iso := base
		iso.Isolated = true
		extra, ok := raSobrantes(plano, raCmd(t, "claude", iso).Args)
		if !ok || !reflect.DeepEqual(extra, []string{"--strict-mcp-config", "--setting-sources", "project"}) {
			t.Fatalf("CA-398: Isolated solo agrega --strict-mcp-config --setting-sources project a %v: extra=%v", plano, extra)
		}
		ef := base
		ef.Effort = "high"
		extra, ok = raSobrantes(plano, raCmd(t, "claude", ef).Args)
		if !ok || !reflect.DeepEqual(extra, []string{"--effort", "high"}) {
			t.Fatalf("CA-398: Effort solo agrega --effort high a %v: extra=%v", plano, extra)
		}
	}
}

// ---------------------------------------------------------------- CA-399

func raPromptDe(n int, relleno string) string {
	var b strings.Builder
	b.WriteString("Revisa el cambio.\n")
	for b.Len() < n {
		b.WriteString(relleno)
	}
	s := b.String()
	for len(s) > n { // relleno de un byte al final para caer justo en n
		s = s[:len(s)-1]
	}
	for len(s) < n {
		s += "x"
	}
	return s
}

// CA-399: con un prompt de mas de 16 KiB, codex termina en "-" y claude no
// lleva prompt posicional; Invocation.Stdin es el prompt entero.
func TestCA399_PromptGrandeViajaPorStdin(t *testing.T) {
	if StdinPromptBytes != 16384 {
		t.Fatalf("CA-399: el umbral es 16 KiB (16384 bytes), la constante dice %d", StdinPromptBytes)
	}
	grande := raPromptDe(StdinPromptBytes+1, "linea del diff con \"comillas\" y 'simples' y $VAR\n")
	for _, name := range []string{"codex", "claude"} {
		for _, req := range []Request{
			{Prompt: grande},
			{Prompt: grande, SystemPrompt: "C", ReadOnly: true, Exec: true, Isolated: true, Effort: "high"},
			{Prompt: grande, ResumeID: "sess-1"},
		} {
			inv := raCmd(t, name, req)
			if inv.Stdin != grande {
				t.Fatalf("CA-399: %s: Invocation.Stdin es el prompt entero (%d bytes), fue %d bytes", name, len(grande), len(inv.Stdin))
			}
			for _, a := range inv.Args {
				if a == grande || strings.Contains(a, "linea del diff") {
					t.Fatalf("CA-399: %s: el argv no lleva el prompt grande", name)
				}
			}
			if name == "codex" && inv.Args[len(inv.Args)-1] != "-" {
				t.Fatalf("CA-399: codex termina su argv en '-': %v", inv.Args)
			}
			// el resto del argv es el mismo que con un prompt chico: solo
			// cambia donde va el prompt (codex lo reemplaza por "-", claude
			// no lleva prompt posicional)
			chico := req
			chico.Prompt = "chico"
			base := raCmd(t, name, chico).Args
			base = base[:len(base)-1]
			if name == "codex" {
				base = append(base, "-")
			}
			if !reflect.DeepEqual(inv.Args, base) {
				t.Fatalf("CA-399: %s: con el prompt por stdin el argv es el de siempre sin el prompt:\nquiere %v\nfue    %v",
					name, base, inv.Args)
			}
		}
	}
}

// CA-399: el limite es en BYTES y estricto: 16384 bytes van por argv (CA-109
// igual), 16385 por stdin; con unicode de dos bytes cuenta el byte, no la
// runa. gemini y opencode no cambian.
func TestCA399_LimiteExactoEnBytes(t *testing.T) {
	justo := raPromptDe(StdinPromptBytes, "abc\n")
	unoMas := raPromptDe(StdinPromptBytes+1, "abc\n")
	// 8192 enes = 16384 bytes (argv); 8193 = 16386 bytes (stdin)
	enesJusto := strings.Repeat("ñ", StdinPromptBytes/2)
	enesMas := strings.Repeat("ñ", StdinPromptBytes/2+1)
	for _, name := range []string{"codex", "claude"} {
		for _, p := range []string{justo, enesJusto} {
			inv := raCmd(t, name, Request{Prompt: p})
			if inv.Stdin != "" || inv.Args[len(inv.Args)-1] != p || raCount(inv.Args, p) != 1 {
				t.Fatalf("CA-399: %s: con %d bytes el prompt sigue ultimo en argv y Stdin vacio (CA-109)", name, len(p))
			}
		}
		for _, p := range []string{unoMas, enesMas} {
			inv := raCmd(t, name, Request{Prompt: p})
			if inv.Stdin != p || raCount(inv.Args, p) != 0 {
				t.Fatalf("CA-399: %s: con %d bytes el prompt va por stdin", name, len(p))
			}
		}
	}
	for _, name := range []string{"gemini", "opencode"} {
		inv := raCmd(t, name, Request{Prompt: unoMas})
		if inv.Stdin != "" || inv.Args[len(inv.Args)-1] != unoMas {
			t.Fatalf("CA-399: %s no cambia: el prompt sigue en argv", name)
		}
	}
}

// CA-399 (propiedad): para cualquier largo cerca del umbral, el prompt va por
// stdin si y solo si pasa de 16384 bytes, y nunca en los dos lados.
func TestCA399_PropiedadStdinSiYSoloSiPasaElUmbral(t *testing.T) {
	prop := func(delta int16, codex bool) bool {
		n := StdinPromptBytes + int(delta%512)
		if n < 20 {
			n = 20
		}
		p := raPromptDe(n, "diff\n")
		name := "claude"
		if codex {
			name = "codex"
		}
		prov, _ := Lookup(name)
		inv, err := prov.Command(Request{Prompt: p, Isolated: true})
		if err != nil {
			return false
		}
		enArgv := raCount(inv.Args, p) == 1
		enStdin := inv.Stdin == p
		if len(p) > StdinPromptBytes {
			return enStdin && !enArgv
		}
		return enArgv && inv.Stdin == ""
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 120, Rand: rand.New(rand.NewSource(399))}); err != nil {
		t.Fatalf("CA-399: stdin si y solo si el prompt pasa de 16384 bytes: %v", err)
	}
}

// CA-399 / CA-418 (enmienda 4): con Request.PromptStdin, codex y claude
// mandan el prompt por stdin A CUALQUIER TAMANO (uno chico, uno unicode, uno
// de 16384 bytes justos, uno de 16385): Invocation.Stdin es el prompt entero,
// ningun argumento lo trae, y el argv es el de siempre sin el prompt (codex
// lo reemplaza por "-", claude no lleva prompt posicional). Tambien en
// resume, con los flags del reviewer y con Strict (PromptStdin no es un campo
// que se pueda negar).
func TestCA399_PromptStdinACualquierTamano(t *testing.T) {
	prompts := []string{
		"hola",
		"revisa esto: \"comillas\", 'simples', $HOME, `id` y ñandú €",
		raPromptDe(StdinPromptBytes, "abc\n"),
		raPromptDe(StdinPromptBytes+1, "abc\n"),
	}
	for _, name := range []string{"codex", "claude"} {
		for _, p := range prompts {
			for _, base := range []Request{
				{Prompt: p},
				{Prompt: p, SystemPrompt: "C", ReadOnly: true, Exec: true, Isolated: true, Effort: "high", Model: "m1", Strict: true},
				{Prompt: p, ResumeID: "sess-1"},
			} {
				req := base
				req.PromptStdin = true
				inv := raCmd(t, name, req)
				if inv.Stdin != p {
					t.Fatalf("CA-399/CA-418: %s: con PromptStdin, Invocation.Stdin es el prompt entero (%d bytes), fue %d bytes", name, len(p), len(inv.Stdin))
				}
				for _, a := range inv.Args {
					if strings.Contains(a, p) {
						t.Fatalf("CA-399/CA-418: %s: con PromptStdin el argv no lleva el prompt (%d bytes): %q", name, len(p), a)
					}
				}
				if len(inv.Ignored) != 0 {
					t.Fatalf("CA-399/CA-418: %s honra PromptStdin: Ignored %v", name, inv.Ignored)
				}
				chico := base
				chico.Prompt = "chico"
				want := raCmd(t, name, chico).Args
				want = want[:len(want)-1]
				if name == "codex" {
					want = append(want, "-")
				}
				if !reflect.DeepEqual(inv.Args, want) {
					t.Fatalf("CA-399/CA-418: %s: con PromptStdin el argv es el de siempre sin el prompt:\nquiere %v\nfue    %v", name, want, inv.Args)
				}
			}
		}
	}
}

// CA-399 / CA-418 (propiedad): con PromptStdin, para cualquier largo (de 1 a
// 40.000 bytes) el prompt va por stdin y nunca por argv, en codex y claude.
func TestCA399_PropiedadPromptStdinSiempreStdin(t *testing.T) {
	prop := func(largo uint16, codex bool) bool {
		n := 1 + int(largo)%40000
		p := raPromptDe(n, "linea del pedido con la evidencia\n")
		name := "claude"
		if codex {
			name = "codex"
		}
		prov, _ := Lookup(name)
		inv, err := prov.Command(Request{Prompt: p, PromptStdin: true, Isolated: true})
		if err != nil || inv.Stdin != p {
			return false
		}
		for _, a := range inv.Args {
			if strings.Contains(a, p) {
				return false
			}
		}
		return !codex || inv.Args[len(inv.Args)-1] == "-"
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 120, Rand: rand.New(rand.NewSource(418))}); err != nil {
		t.Fatalf("CA-399/CA-418: con PromptStdin el prompt va siempre por stdin: %v", err)
	}
}
