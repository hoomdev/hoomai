// El catalogo de modelos (spec tablero-con-paneles, "El catalogo de
// modelos"; CA-572 y CA-569): el catalogo de internal/modelos es exactamente
// el de las tablas del spec (los modelos de claude y de codex, sus
// esfuerzos, sus lineas, su orden y la version de la CLI contra la que se
// verifico), las lineas sin acentos, y Modelos junta el catalogo con los
// models.<provider> de hoom.yaml. Las tablas se copiaron a mano del spec.
package modelos_test

import (
	"reflect"
	"testing"

	"github.com/hoomdev/hoomai/internal/modelos"
)

type ttFila struct{ nombre, linea string }

var (
	ttClaude = []ttFila{
		{"fable", "para los problemas mas dificiles"},
		{"opus", "para trabajo complejo"},
		{"sonnet", "para la mayoria de las tareas"},
		{"haiku", "para preguntas rapidas"},
		{"opusplan", "opus mientras planea, sonnet despues"},
		{"fable[1m]", "fable con contexto de 1M, para sesiones largas"},
		{"opus[1m]", "opus con contexto de 1M, para sesiones largas"},
		{"sonnet[1m]", "sonnet con contexto de 1M, para sesiones largas"},
	}
	ttCodex = []ttFila{
		{"gpt-6.1-sol", "el de todos los dias para programar, el mas nuevo"},
		{"gpt-6-astra", "el mas capaz, para el trabajo mas exigente"},
		{"gpt-6.1-terra", "Terra 6.1 de codex"},
		{"gpt-6-sol", "el de todos los dias, de la generacion anterior"},
		{"gpt-6-luna", "rapido y barato, para tareas faciles"},
		{"gpt-5.6-sol", "el de todos los dias, de una generacion mas vieja"},
		{"gpt-5.6-terra", "equilibrado, para trabajo directo, mas viejo"},
		{"gpt-5.6-luna", "rapido y eficiente, mas viejo"},
	}
	ttEsfuerzos = []ttFila{
		{"low", "rapido, razona poco"},
		{"medium", "equilibra velocidad y profundidad"},
		{"high", "mas profundidad para problemas complejos"},
		{"xhigh", "profundidad extra"},
		{"max", "la maxima profundidad, el mas lento"},
	}
	ttUltra = ttFila{"ultra", "el maximo y delega tareas solo; no todos los modelos lo aceptan"}
)

func ttOpciones(fs []ttFila) []modelos.Opcion {
	var out []modelos.Opcion
	for _, f := range fs {
		out = append(out, modelos.Opcion{Nombre: f.nombre, Linea: f.linea})
	}
	return out
}

// CA-572: Catalogos es exactamente el de las tablas del spec, en el orden
// claude, codex, con la version de cada CLI, y Verificado es 2026-11-09.
func TestCA572_ElCatalogoEsElDelSpec(t *testing.T) {
	quiere := []modelos.Catalogo{
		{Provider: "claude", Version: "Claude Code 2.1.319", Modelos: ttOpciones(ttClaude), Esfuerzos: ttOpciones(ttEsfuerzos)},
		{Provider: "codex", Version: "codex-cli 0.160.0", Modelos: ttOpciones(ttCodex), Esfuerzos: ttOpciones(append(append([]ttFila{}, ttEsfuerzos...), ttUltra))},
	}
	if got := modelos.Catalogos(); !reflect.DeepEqual(got, quiere) {
		t.Fatalf("CA-572: el catalogo es el de las tablas del spec:\n got %#v\nwant %#v", got, quiere)
	}
	if modelos.Verificado != "2026-11-09" {
		t.Fatalf("CA-572: el catalogo se verifico el 2026-11-09: %q", modelos.Verificado)
	}
}

// CA-572: las lineas van sin acentos (ASCII), iguales en UTF-8 y en ASCII; y
// Catalogos da una copia.
func TestCA572_SinAcentosYUnaCopia(t *testing.T) {
	for _, c := range modelos.Catalogos() {
		for _, o := range append(append([]modelos.Opcion{}, c.Modelos...), c.Esfuerzos...) {
			for _, r := range o.Linea + o.Nombre {
				if r > 127 {
					t.Fatalf("CA-572: la linea de %s/%s va sin acentos: %q", c.Provider, o.Nombre, o.Linea)
				}
			}
		}
	}
	a := modelos.Catalogos()
	a[0].Modelos[0].Nombre = "cambiado"
	a[0].Version = "otra"
	a[1].Esfuerzos = nil
	if b := modelos.Catalogos(); b[0].Modelos[0].Nombre != "fable" || b[0].Version != "Claude Code 2.1.319" || len(b[1].Esfuerzos) != 6 {
		t.Fatalf("CA-572: cambiar el resultado de Catalogos no cambia el catalogo: %#v", b[0])
	}
	c, _ := modelos.De("claude")
	c.Modelos[1].Linea = "otra"
	if d, _ := modelos.De("claude"); d.Modelos[1].Linea != "para trabajo complejo" {
		t.Fatalf("CA-572: De tampoco comparte el catalogo")
	}
}

// CA-572, CA-569: De da la entrada de claude y de codex; ninguna para gemini,
// opencode ni un provider desconocido. ConModelo son claude y codex.
// RecibeModelo y RecibeEsfuerzo, true solo para ellos.
func TestCA572_CA569_DeYLosQueRecibenModelo(t *testing.T) {
	for _, p := range []string{"claude", "codex"} {
		c, ok := modelos.De(p)
		if !ok || c.Provider != p {
			t.Fatalf("CA-572: De(%q) da su entrada: %v %#v", p, ok, c)
		}
		if !modelos.RecibeModelo(p) || !modelos.RecibeEsfuerzo(p) {
			t.Fatalf("CA-569: hoom le pasa modelo y esfuerzo a %s", p)
		}
	}
	for _, p := range []string{"gemini", "opencode", "no-existe", ""} {
		if _, ok := modelos.De(p); ok {
			t.Fatalf("CA-572: De(%q) no tiene entrada", p)
		}
		if modelos.RecibeModelo(p) || modelos.RecibeEsfuerzo(p) {
			t.Fatalf("CA-569: hoom no le pasa modelo ni esfuerzo a %q", p)
		}
	}
	if got := modelos.ConModelo(); !reflect.DeepEqual(got, []string{"claude", "codex"}) {
		t.Fatalf("CA-572: ConModelo son claude y codex, ordenados: %q", got)
	}
}

// CA-569: Modelos es el catalogo del provider y despues los propios que no
// son del catalogo, en su orden, con su descripcion como linea; un propio con
// el nombre de un modelo del catalogo no lo repite (su descripcion, si no
// esta vacia, le cambia la linea); dos propios con el mismo nombre cuentan una
// vez (el primero); un provider sin entrada da solo los propios; el
// resultado es nuevo.
func TestCA569_Modelos(t *testing.T) {
	propios := []modelos.Propio{
		{Nombre: "gpt-7-sol", Descripcion: "el nuevo de todos los dias"},
		{Nombre: "gpt-6-astra", Descripcion: "el de la casa"},
		{Nombre: "gpt-6-luna"},
		{Nombre: "gpt-7-sol", Descripcion: "repetido"},
		{Nombre: "sin-desc"},
	}
	got := modelos.Modelos("codex", propios)
	quiere := ttOpciones(ttCodex)
	quiere[1].Linea = "el de la casa"
	quiere = append(quiere, modelos.Opcion{Nombre: "gpt-7-sol", Linea: "el nuevo de todos los dias"}, modelos.Opcion{Nombre: "sin-desc"})
	if !reflect.DeepEqual(got, quiere) {
		t.Fatalf("CA-569: Modelos de codex con los propios:\n got %#v\nwant %#v", got, quiere)
	}
	if got := modelos.Modelos("claude", nil); !reflect.DeepEqual(got, ttOpciones(ttClaude)) {
		t.Fatalf("CA-569: sin propios, el catalogo: %#v", got)
	}
	if got := modelos.Modelos("gemini", []modelos.Propio{{Nombre: "x", Descripcion: "y"}}); !reflect.DeepEqual(got, []modelos.Opcion{{Nombre: "x", Linea: "y"}}) {
		t.Fatalf("CA-569: un provider sin entrada da solo los propios: %#v", got)
	}
	if got := modelos.Modelos("gemini", nil); len(got) != 0 {
		t.Fatalf("CA-569: sin entrada y sin propios, nada: %#v", got)
	}
	a := modelos.Modelos("claude", nil)
	a[0].Nombre = "cambiado"
	if b := modelos.Modelos("claude", nil); b[0].Nombre != "fable" {
		t.Fatalf("CA-569: el resultado de Modelos es nuevo")
	}
}
