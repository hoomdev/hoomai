// Tests adversariales del spec .hoom/specs/ciego-sin-config-personal.md
// (CA-589, y la parte de Codex de CA-590) en providers: con Isolated, el argv
// de claude lleva --strict-mcp-config --setting-sources project --settings
// {"autoMemoryEnabled":false}, seguidos, una sola vez, entre -p y el prompt;
// Isolated no agrega nada mas, y sin Isolated no aparece ninguno de los
// cinco. Codex no cambia.
//
// Re-expresa CA-398 (lo que agrega Isolated en claude): ver
// TestCA398_ClaudeCadaCampoAgregaSoloLoSuyo en review_aislada_test.go.
package providers

import (
	"encoding/json"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
)

// cscCinco son los cinco argumentos del aislamiento de claude, en orden.
var cscCinco = []string{"--strict-mcp-config", "--setting-sources", "project", "--settings", `{"autoMemoryEnabled":false}`}

// cscBloques devuelve las posiciones donde empieza el bloque contiguo de los
// cinco.
func cscBloques(args []string) []int {
	var out []int
	for i := 0; i+len(cscCinco) <= len(args); i++ {
		if reflect.DeepEqual(args[i:i+len(cscCinco)], cscCinco) {
			out = append(out, i)
		}
	}
	return out
}

// cscBases son pedidos variados: plano, con el sobre de un reviewer, de un
// test-writer, en resume, con continue, con limites, con esfuerzo y con el
// prompt por stdin.
func cscBases() []Request {
	return []Request{
		{Prompt: "hola"},
		{Prompt: "revisa el cambio", SystemPrompt: "# Reviewer", ReadOnly: true, Exec: true, Model: "opus"},
		{Prompt: "escribi los tests", SystemPrompt: "# Test-writer", Exec: true, Unattended: true, Strict: true},
		{Prompt: "implementa", Unattended: true, NoExec: true},
		{Prompt: "seguimos", ResumeID: "sess-ca589"},
		{Prompt: "seguimos", Continue: true},
		{Prompt: "con topes", MaxTurns: 7, BudgetUSD: 1.5, Effort: "high"},
		{Prompt: "por stdin", PromptStdin: true},
		{Prompt: "ñandú € \"comillas\" 'simples' $HOME `id`"},
	}
}

// cscPromptEnArgv dice si el prompt de req viaja como ultimo argumento.
func cscPromptEnArgv(req Request) bool {
	return !req.PromptStdin && len(req.Prompt) <= StdinPromptBytes
}

// CA-589: con Isolated, el bloque de cinco aparece seguido, en ese orden, una
// sola vez, despues de -p y antes del prompt; cada uno de los cinco una sola
// vez en todo el argv.
func TestCA589_ClaudeAisladoLlevaLosCincoSeguidos(t *testing.T) {
	for _, base := range cscBases() {
		req := base
		req.Isolated = true
		inv := raCmd(t, "claude", req)
		args := inv.Args
		bl := cscBloques(args)
		if len(bl) != 1 {
			t.Fatalf("CA-589: con Isolated el argv lleva %v seguidos una sola vez (%d veces): %v", cscCinco, len(bl), args)
		}
		for _, a := range cscCinco {
			if raCount(args, a) != 1 {
				t.Fatalf("CA-589: %q aparece una sola vez en el argv: %v", a, args)
			}
		}
		p := raIndex(args, "-p")
		if p < 0 || bl[0] <= p {
			t.Fatalf("CA-589: los cinco van despues de -p: %v", args)
		}
		if cscPromptEnArgv(req) {
			last := len(args) - 1
			if args[last] != req.Prompt {
				t.Fatalf("CA-589: el prompt sigue ultimo: %v", args)
			}
			if bl[0]+len(cscCinco)-1 >= last {
				t.Fatalf("CA-589: los cinco van antes del prompt: %v", args)
			}
		}
		if len(inv.Ignored) != 0 {
			t.Fatalf("CA-589: claude honra Isolated, no lo ignora: %v", inv.Ignored)
		}
	}
}

// CA-589: el JSON viaja como UN argumento, exactamente
// {"autoMemoryEnabled":false}, inmediatamente despues de --settings (no como
// --settings=..., no partido, no con espacios), y es JSON valido que apaga la
// memoria automatica.
func TestCA589_ElJSONDeSettingsEsUnSoloArgumento(t *testing.T) {
	args := raCmd(t, "claude", Request{Prompt: "hola", Isolated: true}).Args
	i := raIndex(args, "--settings")
	if i < 0 || i+1 >= len(args) || args[i+1] != `{"autoMemoryEnabled":false}` {
		t.Fatalf(`CA-589: --settings seguido de {"autoMemoryEnabled":false} como un solo argumento: %v`, args)
	}
	for _, a := range args {
		if strings.HasPrefix(a, "--settings=") || (strings.Contains(a, "autoMemoryEnabled") && a != `{"autoMemoryEnabled":false}`) {
			t.Fatalf("CA-589: el JSON no viaja en otra forma: %q en %v", a, args)
		}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(args[i+1]), &m); err != nil {
		t.Fatalf("CA-589: el valor de --settings es JSON valido: %v", err)
	}
	if v, ok := m["autoMemoryEnabled"].(bool); !ok || v || len(m) != 1 {
		t.Fatalf("CA-589: el JSON solo apaga autoMemoryEnabled: %v", m)
	}
}

// CA-589: Isolated no agrega nada mas que los cinco, en ese orden, sobre
// cualquier base; sin Isolated no aparece ninguno de los cinco.
func TestCA589_IsolatedAgregaSoloLosCincoYSinElNinguno(t *testing.T) {
	for _, base := range cscBases() {
		plano := raCmd(t, "claude", base).Args
		for _, a := range cscCinco {
			if raIndex(plano, a) >= 0 {
				t.Fatalf("CA-589: sin Isolated no aparece %q: %v", a, plano)
			}
		}
		iso := base
		iso.Isolated = true
		got := raCmd(t, "claude", iso).Args
		extra, ok := raSobrantes(plano, got)
		if !ok || !reflect.DeepEqual(extra, cscCinco) {
			t.Fatalf("CA-589: Isolated solo agrega %v a %v: extra=%v (argv %v)", cscCinco, plano, extra, got)
		}
		// y el resto del argv conserva su orden: quitar el bloque da el plano
		bl := cscBloques(got)
		if len(bl) != 1 {
			t.Fatalf("CA-589: el bloque aparece una vez: %v", got)
		}
		sin := append(append([]string{}, got[:bl[0]]...), got[bl[0]+len(cscCinco):]...)
		if !reflect.DeepEqual(sin, plano) {
			t.Fatalf("CA-589: sin el bloque, el argv aislado es el plano:\nquiere %v\nfue    %v", plano, sin)
		}
	}
}

// CA-589 (propiedad): para cualquier combinacion de campos del pedido, el
// bloque de cinco aparece una vez, seguido y despues de -p si y solo si se
// pide Isolated; sin Isolated, ninguno de los cinco esta en el argv.
func TestCA589_PropiedadElBloqueSiYSoloSiIsolated(t *testing.T) {
	prov, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	prop := func(iso, ro, ex, noex, un, strict, stdin, resume, cont bool, turns uint8, effort bool) bool {
		req := Request{Prompt: "trabaja", Isolated: iso, ReadOnly: ro, Exec: ex, NoExec: noex,
			Unattended: un, Strict: strict, PromptStdin: stdin, Continue: cont, MaxTurns: int(turns % 5)}
		if resume {
			req.ResumeID = "sess-prop"
		}
		if effort {
			req.Effort = "max"
		}
		inv, err := prov.Command(req)
		if err != nil {
			t.Logf("CA-589: Command(%+v) fallo: %v", req, err)
			return false
		}
		bl := cscBloques(inv.Args)
		if !iso {
			for _, a := range cscCinco {
				if raIndex(inv.Args, a) >= 0 {
					return false
				}
			}
			return len(bl) == 0
		}
		if len(bl) != 1 || bl[0] <= raIndex(inv.Args, "-p") {
			return false
		}
		for _, a := range cscCinco {
			if raCount(inv.Args, a) != 1 {
				return false
			}
		}
		if !stdin && inv.Args[len(inv.Args)-1] != "trabaja" {
			return false
		}
		return true
	}
	// sin Isolated: ninguno de los cinco (y ninguna combinacion hace fallar a
	// Command, asi que un rojo del otro lado es del aislamiento)
	sin := func(ro, ex, noex, un, strict, stdin, resume, cont bool, turns uint8, effort bool) bool {
		return prop(false, ro, ex, noex, un, strict, stdin, resume, cont, turns, effort)
	}
	if err := quick.Check(sin, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(5890))}); err != nil {
		t.Fatalf("CA-589: sin Isolated no aparece ninguno de los cinco: %v", err)
	}
	con := func(ro, ex, noex, un, strict, stdin, resume, cont bool, turns uint8, effort bool) bool {
		return prop(true, ro, ex, noex, un, strict, stdin, resume, cont, turns, effort)
	}
	if err := quick.Check(con, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(589))}); err != nil {
		t.Fatalf("CA-589: con Isolated el bloque de cinco esta una vez, seguido y despues de -p: %v", err)
	}
}

// CA-589 (hostil): un prompt que se llama igual que uno de los cinco no
// cuenta como aislamiento ni duplica el bloque.
func TestCA589_UnPromptConNombreDeFlagNoEsElBloque(t *testing.T) {
	// (un prompt que empieza con '-' ya lo rechaza el adapter: no cuenta aca)
	for _, prompt := range []string{"project", `{"autoMemoryEnabled":false}`, "settings project"} {
		args := raCmd(t, "claude", Request{Prompt: prompt}).Args
		if len(cscBloques(args)) != 0 || raIndex(args, "--settings") >= 0 {
			t.Fatalf("CA-589: sin Isolated no hay bloque aunque el prompt sea %q: %v", prompt, args)
		}
		args = raCmd(t, "claude", Request{Prompt: prompt, Isolated: true}).Args
		if len(cscBloques(args)) != 1 || args[len(args)-1] != prompt {
			t.Fatalf("CA-589: con Isolated un solo bloque y el prompt %q ultimo: %v", prompt, args)
		}
	}
}

// CA-590 (Codex no cambia): con Isolated codex lleva --ignore-user-config, una
// vez, y ninguno de los argumentos de claude.
func TestCA590_CodexAisladoSigueConIgnoreUserConfig(t *testing.T) {
	for _, base := range []Request{
		{Prompt: "escribi los tests", SystemPrompt: "# Test-writer", Exec: true, Unattended: true, Strict: true},
		{Prompt: "seguimos", ResumeID: "th-ca590"},
	} {
		plano := raCmd(t, "codex", base).Args
		iso := base
		iso.Isolated = true
		args := raCmd(t, "codex", iso).Args
		extra, ok := raSobrantes(plano, args)
		if !ok || !reflect.DeepEqual(extra, []string{"--ignore-user-config"}) {
			t.Fatalf("CA-590: codex con Isolated solo agrega --ignore-user-config: extra=%v (%v)", extra, args)
		}
		for _, a := range cscCinco {
			if raIndex(args, a) >= 0 {
				t.Fatalf("CA-590: codex no lleva los argumentos de claude (%q): %v", a, args)
			}
		}
	}
}
