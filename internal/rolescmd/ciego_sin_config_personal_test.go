// Tests adversariales del spec .hoom/specs/ciego-sin-config-personal.md
// (CA-593) en rolescmd: con un provider que tiene system prompt y no declara
// Isolation, las tres celdas del test-writer son UNSUPPORTED, "no corre el
// rol ciego", [sin aislamiento] y el gap es la primera linea del error de
// agentcmd.IsolationFor (CA-591). Los demas roles quedan como con Isolation, y
// claude y codex no cambian.
//
// El provider falso NO se registra: rcMatriz y CA-295 cuentan cuatro
// providers en providers.All().
package rolescmd

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/providers"
)

// cscProv es un provider falso con las capacidades que se le den.
type cscProv struct {
	name string
	caps providers.Capabilities
}

func (p cscProv) Name() string                         { return p.name }
func (p cscProv) Bin() string                          { return "hoom-ca593-nunca-en-el-path" }
func (p cscProv) Capabilities() providers.Capabilities { return p.caps }
func (p cscProv) Command(providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: p.Bin()}, nil
}
func (p cscProv) Normalize(string) []providers.Event { return nil }

// cscCapsSinAislar es lo que declara claude, sin Isolation.
func cscCapsSinAislar(t *testing.T) providers.Capabilities {
	t.Helper()
	c := mustProv(t, "claude").Capabilities()
	c.Isolation = false
	if !c.SystemPrompt {
		t.Fatal("CA-593: fixture: claude tiene system prompt")
	}
	return c
}

// cscGap es la primera linea del error de CA-591, literal del contrato.
func cscGap(name string) string {
	return name + " no puede correr sin tu configuracion personal (MCP, memoria, hooks): un rol ciego no corre ahi."
}

func cscCeldas(r Row) map[string]Cell {
	return map[string]Cell{"read": r.Read, "write": r.Write, "exec": r.Exec}
}

// cscTestWriterSinAislar comprueba las tres celdas del test-writer en p.
func cscTestWriterSinAislar(t *testing.T, p providers.Provider) {
	t.Helper()
	tw := mustRole(t, "test-writer")
	rows := Matrix(nil, []agents.Role{tw}, []providers.Provider{p})
	if len(rows) != 1 {
		t.Fatalf("CA-593: una fila: %+v", rows)
	}
	gap := cscGap(p.Name())
	// el gap es la primera linea del error de IsolationFor: la misma frase
	// en los dos lugares
	err := agentcmd.IsolationFor(p, tw)
	if err == nil {
		t.Fatalf("CA-593/CA-591: IsolationFor niega el test-writer con %s (sin Isolation)", p.Name())
	}
	if primera := strings.SplitN(err.Error(), "\n", 2)[0]; primera != gap {
		t.Fatalf("CA-593/CA-591: la primera linea del error es la del contrato:\nquiere %q\nfue    %q", gap, primera)
	}
	for dim, c := range cscCeldas(rows[0]) {
		if c.Category != Unsupported {
			t.Fatalf("CA-593: test-writer x %s %s es UNSUPPORTED: %+v", p.Name(), dim, c)
		}
		if c.Allows != "no corre el rol ciego" {
			t.Fatalf("CA-593: test-writer x %s %s: allows %q", p.Name(), dim, c.Allows)
		}
		if !reflect.DeepEqual(c.Mechanisms, []string{MechNoIsolation}) {
			t.Fatalf("CA-593: test-writer x %s %s: mecanismos exactamente [%s]: %v", p.Name(), dim, MechNoIsolation, c.Mechanisms)
		}
		if c.Gap != gap {
			t.Fatalf("CA-593: test-writer x %s %s: gap\nquiere %q\nfue    %q", p.Name(), dim, gap, c.Gap)
		}
	}
}

// CA-593: el mecanismo nuevo se llama "sin aislamiento".
func TestCA593_MecanismoSinAislamiento(t *testing.T) {
	if MechNoIsolation != "sin aislamiento" {
		t.Fatalf("CA-593: MechNoIsolation = %q", MechNoIsolation)
	}
}

// CA-593: con un provider que tiene todo lo de claude menos Isolation, el
// test-writer no corre: tres celdas UNSUPPORTED con [sin aislamiento].
func TestCA593_TestWriterSinAislamientoEsUnsupported(t *testing.T) {
	cscTestWriterSinAislar(t, cscProv{name: "sin-aislar-ca593", caps: cscCapsSinAislar(t)})
	// y con lo minimo para cargar un contrato
	cscTestWriterSinAislar(t, cscProv{name: "solo-prompt-ca593", caps: providers.Capabilities{SystemPrompt: true}})
	// el nombre del provider viaja tal cual al gap, unicode incluido
	cscTestWriterSinAislar(t, cscProv{name: "ñandú-ca593", caps: providers.Capabilities{SystemPrompt: true, ReadOnly: true}})
}

// CA-593: con ese provider, la fila de cada rol que no es ciego es igual a la
// de un provider con las mismas capacidades mas Isolation.
func TestCA593_LosDemasRolesComoConIsolation(t *testing.T) {
	sin := cscCapsSinAislar(t)
	con := sin
	con.Isolation = true
	cscCompararDemas(t, sin, con)
	cscCompararDemas(t, providers.Capabilities{SystemPrompt: true}, providers.Capabilities{SystemPrompt: true, Isolation: true})
}

func cscCompararDemas(t *testing.T, sin, con providers.Capabilities) {
	t.Helper()
	// el mismo nombre en los dos: si la matriz mirara el nombre, no cambia
	pSin, pCon := cscProv{name: "x-ca593", caps: sin}, cscProv{name: "x-ca593", caps: con}
	for _, r := range agents.Roles() {
		if r.Isolated {
			continue
		}
		a := Matrix(nil, []agents.Role{r}, []providers.Provider{pSin})
		b := Matrix(nil, []agents.Role{r}, []providers.Provider{pCon})
		if !reflect.DeepEqual(a, b) {
			t.Fatalf("CA-593: %s sin Isolation es igual que con Isolation (%+v):\nsin %+v\ncon %+v", r.Slug, sin, a, b)
		}
		for dim, c := range cscCeldas(a[0]) {
			if contiene(c.Mechanisms, MechNoIsolation) || c.Allows == "no corre el rol ciego" {
				t.Fatalf("CA-593: %s no es ciego: %s no dice sin aislamiento: %+v", r.Slug, dim, c)
			}
		}
	}
}

// CA-593 (propiedad): para cualquier juego de capacidades con system prompt y
// sin Isolation, el test-writer es UNSUPPORTED [sin aislamiento] en las tres
// celdas, y cada rol no ciego da la misma fila que con Isolation.
func TestCA593_PropiedadCualquierProviderSinIsolation(t *testing.T) {
	tw := mustRole(t, "test-writer")
	prop := func(c providers.Capabilities) bool {
		c.SystemPrompt, c.Isolation = true, false
		p := cscProv{name: "prop-ca593", caps: c}
		row := Matrix(nil, []agents.Role{tw}, []providers.Provider{p})[0]
		for _, cell := range cscCeldas(row) {
			if cell.Category != Unsupported || cell.Allows != "no corre el rol ciego" ||
				!reflect.DeepEqual(cell.Mechanisms, []string{MechNoIsolation}) || cell.Gap != cscGap(p.Name()) {
				return false
			}
		}
		con := c
		con.Isolation = true
		pc := cscProv{name: "prop-ca593", caps: con}
		for _, r := range agents.Roles() {
			if r.Isolated {
				continue
			}
			if !reflect.DeepEqual(Matrix(nil, []agents.Role{r}, []providers.Provider{p}),
				Matrix(nil, []agents.Role{r}, []providers.Provider{pc})) {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 150, Rand: rand.New(rand.NewSource(593))}); err != nil {
		t.Fatalf("CA-593: todo provider con system prompt y sin Isolation: %v", err)
	}
}

// CA-593: las filas de claude y codex no cambian: el test-writer sigue con sus
// celdas de hoy (CA-296) y ninguna celda de claude o codex dice sin
// aislamiento; sumar el provider falso a la lista no les mueve nada.
func TestCA593_ClaudeYCodexNoCambian(t *testing.T) {
	cc := []providers.Provider{mustProv(t, "claude"), mustProv(t, "codex")}
	solos := Matrix(nil, agents.Roles(), cc)
	falso := cscProv{name: "sin-aislar-ca593", caps: cscCapsSinAislar(t)}
	con := Matrix(nil, agents.Roles(), append(append([]providers.Provider{}, cc...), falso))
	var filtradas []Row
	for _, r := range con {
		if r.Provider != falso.name {
			filtradas = append(filtradas, r)
		}
	}
	if !reflect.DeepEqual(solos, filtradas) {
		t.Fatalf("CA-593: el provider falso no cambia las filas de claude y codex")
	}
	for _, r := range solos {
		for dim, c := range cscCeldas(r) {
			if contiene(c.Mechanisms, MechNoIsolation) || c.Allows == "no corre el rol ciego" {
				t.Fatalf("CA-593: %s x %s %s no dice sin aislamiento: %+v", r.Role, r.Provider, dim, c)
			}
			if r.Role == "test-writer" && c.Category == Unsupported {
				t.Fatalf("CA-593: el test-writer corre con %s: %s %+v", r.Provider, dim, c)
			}
		}
	}
	for _, prov := range []string{"claude", "codex"} {
		c := rcFila(t, solos, "test-writer", prov).Read
		if !reflect.DeepEqual(c.Mechanisms, []string{MechBlind, MechGateIsolation}) || c.Category != BestEffort {
			t.Fatalf("CA-593: la lectura del test-writer en %s queda como hoy: %+v", prov, c)
		}
	}
}

// CA-593 (hostil): sin system prompt manda la regla de siempre: gemini y
// opencode siguen con [sin system_prompt] y no ganan sin aislamiento.
func TestCA593_SinSystemPromptSigueComoHoy(t *testing.T) {
	tw := mustRole(t, "test-writer")
	ps := []providers.Provider{mustProv(t, "gemini"), mustProv(t, "opencode"),
		cscProv{name: "nada-ca593", caps: providers.Capabilities{}}}
	for _, p := range ps {
		row := Matrix(nil, []agents.Role{tw}, []providers.Provider{p})[0]
		for dim, c := range cscCeldas(row) {
			if c.Category != Unsupported || !contiene(c.Mechanisms, MechNoSystemPrompt) || contiene(c.Mechanisms, MechNoIsolation) {
				t.Fatalf("CA-593: test-writer x %s %s sigue con sin system_prompt: %+v", p.Name(), dim, c)
			}
		}
	}
}
