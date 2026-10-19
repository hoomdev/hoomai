// Tests de regresion de la quinta vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd. Escritos desde el spec
// y la descripcion de los hallazgos, sin leer la implementacion. Los helpers
// de este archivo llevan el prefijo c4v5.
//
//   - 0ebb3f y 34a37c: CA-362 pide una entrada subagente por cada evento
//     agent de los runs de la tarjeta, y que cada agent_end emparejado por
//     tool_id la cierre con su subagente-fin. ReadDelegations descarta entera
//     una linea de delegacion de mas de 1 MiB, y la delegacion desaparece de
//     la historia o queda abierta.
//
//     El contrato es el de la enmienda 4 del spec (seccion "Fuente
//     telemetria", decision de Henry, que reemplaza a la 3): hoom escribe
//     cada delegacion en una linea de hasta 1 MiB (si con su detail entero
//     pasaria, recorta el detail AL ESCRIBIRLO: sus primeros 128 KiB y un
//     aviso). La historia lee entera, en cualquier orden de claves, cada linea
//     de delegacion de hasta 1 MiB, y salta una mas larga, como salta una mal
//     formada o a medio escribir. Lo que asigna ReadDelegations sigue acotado
//     (el test de memoria de 2628ba, sin cambios). Una linea de otro kind que
//     solo menciona "kind":"agent" (en un campo extra o en su detail) no sale.
//
//     Con la enmienda 3 estos tests exigian que una linea de mas de 1 MiB
//     volviera recortada AL LEERLA. Con la enmienda 4 esas lineas, escritas a
//     mano, ya no son como las escribe hoom: se exige que se salten, en
//     cualquier orden de claves y tambien las que no son JSON valido, sin
//     tapar a las vecinas. No es debilitar: el recorte se movio a la
//     escritura y ahi se exige (hallazgos_c4_vuelta6_test.go, sobre un run de
//     verdad), y una delegacion larga escrita como hoom (detail recortado,
//     linea de hasta 1 MiB) se exige entera aca.
//
//     Caso encontrado al escribir estos tests (caja negra): una delegacion
//     valida con el detail antes de "kind" se pierde en ReadDelegations
//     desde que su detail llega a 64 KiB, aunque su linea no pase de 1 MiB y
//     ReadEvents la lea. Dentro del contrato (hasta 1 MiB, cualquier orden de
//     claves) sale entera.
//
//   - 4d2ae1 (legibilidad): la tabla de variantes de
//     DelegacionEnormeEscritaAManoSeSalta nombra cada escenario con un enum y
//     sus campos por nombre, en vez de literales posicionales con dos
//     booleanos.
package runcmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

const c4v5MiB = 1 << 20

// c4v5BloqueCrudo es el relleno de los details enormes tal como va en la
// linea (JSON escapado: comillas, barras, \u00e9, \t, runas de 2, 3 y 4
// bytes) y c4v5BloqueTexto es lo mismo decodificado, como lo da ReadEvents.
// El largo del bloque no divide 1 MiB: un corte a 1 MiB cae en cualquier
// parte, tambien en medio de una runa.
const (
	c4v5BloqueCrudo = `relleno \"comillas\" \\barra\\ ñandú 漢字 🙂 \u00e9\t`
	c4v5BloqueTexto = "relleno \"comillas\" \\barra\\ ñandú 漢字 🙂 é\t"
)

// c4v5Linea es una linea del log: un evento como lo escribe json.Marshal, o
// una linea cruda en partes: cabeza + bloques de relleno (con medio en la
// mitad) + cola, para no armar las enormes en memoria.
type c4v5Linea struct {
	ev                  *Event
	cabeza, medio, cola string
	bloques             int
}

func c4v5Ev(e Event) c4v5Linea { return c4v5Linea{ev: &e} }

func c4v5Volcar(w io.Writer, ls ...c4v5Linea) error {
	bw := bufio.NewWriterSize(w, 1<<16)
	for _, l := range ls {
		if l.ev != nil {
			raw, err := json.Marshal(l.ev)
			if err != nil {
				return err
			}
			bw.Write(raw)
			bw.WriteByte('\n')
			continue
		}
		bw.WriteString(l.cabeza)
		for i := 0; i < l.bloques; i++ {
			if i == l.bloques/2 {
				bw.WriteString(l.medio)
			}
			bw.WriteString(c4v5BloqueCrudo)
		}
		bw.WriteString(l.cola)
		bw.WriteByte('\n')
	}
	return bw.Flush()
}

func c4v5Log(t *testing.T, root, id string, ls ...c4v5Linea) string {
	t.Helper()
	p := filepath.Join(root, ".hoom", "runs", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := c4v5Volcar(f, ls...); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

// c4v5Bloques es la cantidad de bloques para al menos n bytes crudos.
func c4v5Bloques(n int) int { return (n + len(c4v5BloqueCrudo) - 1) / len(c4v5BloqueCrudo) }

// c4v5Largo es el largo de una linea cruda, sin el salto de linea.
func c4v5Largo(l c4v5Linea) int {
	return len(l.cabeza) + len(l.medio) + l.bloques*len(c4v5BloqueCrudo) + len(l.cola)
}

// Los estilos de escritura de una linea cruda.
const (
	c4v5Compacto  = iota // como json.Marshal: ts, kind, agent, detail, tool_id
	c4v5Espaciado        // las mismas claves con espacios despues de : y ,
	c4v5OtroOrden        // detail primero; kind, agent, tool_id y ts despues
)

var c4v5Estilos = map[int]string{c4v5Compacto: "compacto", c4v5Espaciado: "espaciado", c4v5OtroOrden: "otro orden"}

// c4v5Campo es una clave y su valor como token JSON crudo.
type c4v5Campo struct{ clave, valor string }

func c4v5Str(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// c4v5Cruda arma una linea con los campos en el orden del estilo y un
// detail enorme: prefijo (crudo) + n bytes de relleno. Con el estilo
// c4v5OtroOrden el detail va primero y el resto despues, en orden inverso.
func c4v5Cruda(estilo int, campos []c4v5Campo, prefijo string, n int) c4v5Linea {
	dp, coma := ":", ","
	if estilo == c4v5Espaciado {
		dp, coma = ": ", ", "
	}
	var antes, despues []c4v5Campo
	switch estilo {
	case c4v5OtroOrden:
		for i := len(campos) - 1; i >= 0; i-- {
			despues = append(despues, campos[i])
		}
	default:
		// el detail va despues de los tres primeros (ts, kind, agent o extra)
		k := min(3, len(campos))
		antes, despues = campos[:k], campos[k:]
	}
	var cab strings.Builder
	cab.WriteString("{")
	for _, c := range antes {
		cab.WriteString(`"` + c.clave + `"` + dp + c.valor + coma)
	}
	cab.WriteString(`"detail"` + dp + `"` + prefijo)
	var cola strings.Builder
	cola.WriteString(`"`)
	for _, c := range despues {
		cola.WriteString(coma + `"` + c.clave + `"` + dp + c.valor)
	}
	cola.WriteString("}")
	return c4v5Linea{cabeza: cab.String(), bloques: c4v5Bloques(n), cola: cola.String()}
}

func c4v5TS(ts time.Time) string { return ts.UTC().Format(time.RFC3339Nano) }

// c4v5Delegacion es una delegacion (agent o agent_end) con un detail enorme.
func c4v5Delegacion(estilo int, ts time.Time, kind, agente, tool, prefijo string, n int) c4v5Linea {
	return c4v5Cruda(estilo, []c4v5Campo{
		{"ts", c4v5Str(c4v5TS(ts))}, {"kind", c4v5Str(kind)}, {"agent", c4v5Str(agente)}, {"tool_id", c4v5Str(tool)},
	}, prefijo, n)
}

// c4v5TextoMarcaExtra es una linea text enorme que lleva la marca
// "kind":"agent" (con agent y tool_id) en un campo extra.
func c4v5TextoMarcaExtra(estilo int, ts time.Time, n int) c4v5Linea {
	return c4v5Cruda(estilo, []c4v5Campo{
		{"ts", c4v5Str(c4v5TS(ts))}, {"kind", c4v5Str("text")},
		{"extra", `{"kind":"agent","agent":"Fantasma","tool_id":"toolu_FANTASMA","detail":"Agent: no soy una delegacion"}`},
	}, "", n)
}

// c4v5TextoMarcaDetalle es una linea text enorme que solo menciona
// "kind":"agent" (y agent_end) en su detail.
func c4v5TextoMarcaDetalle(estilo int, ts time.Time, n int) c4v5Linea {
	return c4v5Cruda(estilo, []c4v5Campo{{"ts", c4v5Str(c4v5TS(ts))}, {"kind", c4v5Str("text")}},
		`{\"kind\":\"agent\",\"agent\":\"Fantasma\",\"tool_id\":\"toolu_FANTASMA\"} {\"kind\":\"agent_end\"} `, n)
}

func c4v5Resumen(e Event) string {
	d := e.Detail
	if len(d) > 60 {
		d = fmt.Sprintf("%.60s... (%d bytes)", d, len(e.Detail))
	}
	return fmt.Sprintf("%s %s agent=%q tool_id=%q detail=%q", c4v5TS(e.TS), e.Kind, e.Agent, e.ToolID, d)
}

func c4v5Listado(ds []Event) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString("  " + c4v5Resumen(d) + "\n")
	}
	return b.String()
}

// c4v5Filtradas es ReadEvents filtrado por agent y agent_end.
func c4v5Filtradas(t *testing.T, root, id string) []Event {
	t.Helper()
	evs, err := ReadEvents(root, id)
	if err != nil {
		t.Fatalf("fixture: ReadEvents lee el log: %v", err)
	}
	var out []Event
	for _, e := range evs {
		if e.Kind == "agent" || e.Kind == "agent_end" {
			out = append(out, e)
		}
	}
	return out
}

// c4v5Comparar exige que ReadDelegations traiga exactamente las delegaciones
// de ref (ReadEvents filtrado) cuya linea es de hasta 1 MiB, cada una una vez
// y en orden, enteras: ts, kind, agent, tool_id y detail iguales. Las de mas
// de 1 MiB no salen (enmienda 4). larga dice si la linea de la delegacion i
// pasa de 1 MiB (nil: ninguna pasa).
//
// Enmienda 4 (decision de Henry): con la 3 una linea de mas de 1 MiB salia
// recortada al leerla; ahora se salta, porque hoom la recorta al escribirla
// (y ahi se exige, en hallazgos_c4_vuelta6_test.go). No es debilitar: cada
// delegacion que ReadDelegations trae se exige entera.
func c4v5Comparar(t *testing.T, ca string, ds, ref []Event, larga func(i int) bool) {
	t.Helper()
	var want []Event
	for i, e := range ref {
		if larga != nil && larga(i) {
			continue
		}
		if len(e.Detail) > c4v5MiB {
			t.Fatalf("fixture: %s: la delegacion %d (%s) trae mas de 1 MiB de detail: su linea pasa de 1 MiB", ca, i, c4v5Resumen(e))
		}
		want = append(want, e)
	}
	ok := len(ds) == len(want)
	for i := 0; ok && i < len(want); i++ {
		g, w := ds[i], want[i]
		ok = g.TS.Equal(w.TS) && g.Kind == w.Kind && g.Agent == w.Agent && g.ToolID == w.ToolID && g.SessionID == w.SessionID &&
			g.Detail == w.Detail
	}
	if !ok {
		t.Errorf("%s: ReadDelegations trae entera, una vez y en orden, cada delegacion de hasta 1 MiB del log (%d), y salta las de mas de 1 MiB (%d); trajo %d:\n%squiere (ReadEvents filtrado, sin las de mas de 1 MiB):\n%s",
			ca, len(want), len(ref)-len(want), len(ds), c4v5Listado(ds), c4v5Listado(want))
	}
}

// c4v5Fantasmas exige que nada de las lineas text con la marca salga.
func c4v5Fantasmas(t *testing.T, ca string, ds []Event) {
	t.Helper()
	for _, d := range ds {
		if d.Agent == "Fantasma" || d.ToolID == "toolu_FANTASMA" || (d.Kind != "agent" && d.Kind != "agent_end") {
			t.Errorf("%s: una linea text que solo menciona \"kind\":\"agent\" no es una delegacion; salio %s\n%s",
				ca, c4v5Resumen(d), c4v5Listado(ds))
		}
	}
}

// c4v5Clave es lo que identifica a una delegacion en el log.
type c4v5Clave struct {
	min                 int
	kind, agent, toolID string
}

// c4v5Escenario dice como esta escrita la delegacion a Enorme (su agent y su
// agent_end) en el log de DelegacionEnormeEscritaAManoSeSalta.
type c4v5Escenario int

const (
	// el agent con un detail enorme (linea de mas de 1 MiB) y su agent_end
	// de tamano normal
	c4v5SoloAgentEnorme c4v5Escenario = iota
	// el agent y el agent_end con un detail enorme
	c4v5AgentYAgentEndEnormes
	// el agent de tamano normal y su agent_end con un detail enorme
	c4v5SoloAgentEndEnorme
	// el agent y el agent_end como los escribe hoom: el detail enorme
	// recortado al escribirlo (los primeros 128 KiB y el aviso), en lineas de
	// hasta 1 MiB
	c4v5EscritaComoHoom
)

func (e c4v5Escenario) String() string {
	return map[c4v5Escenario]string{
		c4v5SoloAgentEnorme:       "agent enorme y agent_end normal",
		c4v5AgentYAgentEndEnormes: "agent y agent_end enormes",
		c4v5SoloAgentEndEnorme:    "agent normal y agent_end enorme",
		c4v5EscritaComoHoom:       "agent y agent_end escritos como hoom, recortados",
	}[e]
}

// agentEnorme y agentEndEnorme dicen si esa linea pasa de 1 MiB.
func (e c4v5Escenario) agentEnorme() bool {
	return e == c4v5SoloAgentEnorme || e == c4v5AgentYAgentEndEnormes
}
func (e c4v5Escenario) agentEndEnorme() bool {
	return e == c4v5SoloAgentEndEnorme || e == c4v5AgentYAgentEndEnormes
}

// CA-362 (hallazgos 0ebb3f y 34a37c; enmienda 4; 4d2ae1): un log con
// delegaciones de tamano normal, una delegacion a Enorme y dos lineas text
// enormes que solo mencionan "kind":"agent" (en un campo extra y en su
// detail). La delegacion a Enorme, segun el escenario: su agent, su
// agent_end o los dos escritos a mano con un detail de 4 MiB o de 32 MiB
// (lineas de mas de 1 MiB, en los tres estilos: compacto, espaciado y con
// las claves en otro orden), o los dos escritos como hoom, con el detail
// recortado al escribirlo. ReadDelegations trae cada delegacion de hasta
// 1 MiB entera, una vez y en orden, con ts, kind, agent, tool_id y detail
// iguales a los de ReadEvents (la escrita como hoom, entera con su aviso), y
// salta cada linea de mas de 1 MiB sin tapar a las vecinas. Nada de las
// lineas text sale.
//
// Antes se llamaba DelegacionEnormeSaleRecortada y exigia el recorte AL LEER
// (enmienda 3). El contrato viene ahora de la enmienda 4 (decision de
// Henry): las lineas enormes escritas a mano no son como las escribe hoom y
// se saltan. No es debilitar: el recorte se movio a la escritura y ahi se
// exige (hallazgos_c4_vuelta6_test.go); lo que hoom escribe de una
// delegacion enorme (el escenario c4v5EscritaComoHoom) se exige entero; y
// las lineas de mas de 1 MiB con las claves en otro orden, que la enmienda 3
// dejaba fuera, vuelven a la tabla: tambien se saltan.
func TestHallazgo_0ebb3f_34a37c_CA362_DelegacionEnormeEscritaAManoSeSalta(t *testing.T) {
	type variante struct {
		estilo    int // como van escritas las lineas enormes
		n         int // bytes del detail enorme
		escenario c4v5Escenario
	}
	var vs []variante
	for _, n := range []int{4 * c4v5MiB, 32 * c4v5MiB} {
		for _, esc := range []c4v5Escenario{c4v5SoloAgentEnorme, c4v5AgentYAgentEndEnormes, c4v5SoloAgentEndEnorme, c4v5EscritaComoHoom} {
			vs = append(vs, variante{estilo: c4v5Compacto, n: n, escenario: esc})
		}
	}
	for _, estilo := range []int{c4v5Espaciado, c4v5OtroOrden} {
		for _, esc := range []c4v5Escenario{c4v5SoloAgentEnorme, c4v5AgentYAgentEndEnormes, c4v5SoloAgentEndEnorme} {
			vs = append(vs, variante{estilo: estilo, n: 4 * c4v5MiB, escenario: esc})
		}
	}
	for _, v := range vs {
		nombre := fmt.Sprintf("%s, detail de %d MiB, %s", c4v5Estilos[v.estilo], v.n/c4v5MiB, v.escenario)
		t.Run(nombre, func(t *testing.T) {
			ca := "CA-362 (hallazgos 0ebb3f y 34a37c, enmienda 4, " + nombre + ")"
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t0 := time.Date(2026, 10, 19, 20, 0, 0, 0, time.UTC)
			m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
			// el detail enorme decodificado, para la delegacion escrita como hoom
			enorme := strings.Repeat(c4v5BloqueTexto, c4v5Bloques(v.n))
			agentEnorme := c4v5Ev(Event{TS: m(2), Kind: "agent", Agent: "Enorme", ToolID: "toolu_ENORME", Detail: "Agent: leer el catalogo"})
			fin := c4v5Ev(Event{TS: m(7), Kind: "agent_end", Agent: "Enorme", ToolID: "toolu_ENORME", Detail: "Enorme termino"})
			switch {
			case v.escenario == c4v5EscritaComoHoom:
				agentEnorme = c4v5Ev(Event{TS: m(2), Kind: "agent", Agent: "Enorme", ToolID: "toolu_ENORME", Detail: c4v6Recortado("Agent: " + enorme)})
				fin = c4v5Ev(Event{TS: m(7), Kind: "agent_end", Agent: "Enorme", ToolID: "toolu_ENORME", Detail: c4v6Recortado("Enorme fallo: " + enorme)})
				for _, l := range []c4v5Linea{agentEnorme, fin} {
					if raw, _ := json.Marshal(l.ev); len(raw) > c4v5MiB {
						t.Fatalf("fixture: la delegacion escrita como hoom va en una linea de hasta 1 MiB: %d bytes", len(raw))
					}
				}
			default:
				if v.escenario.agentEnorme() {
					agentEnorme = c4v5Delegacion(v.estilo, m(2), "agent", "Enorme", "toolu_ENORME", "Agent: ", v.n)
				}
				if v.escenario.agentEndEnorme() {
					fin = c4v5Delegacion(v.estilo, m(7), "agent_end", "Enorme", "toolu_ENORME", "Enorme fallo: ", v.n)
				}
			}
			const id = "20261019T200000_enorme"
			c4v5Log(t, root, id,
				c4v5Ev(Event{TS: m(0), Kind: "start", Detail: "init"}),
				c4v5Ev(Event{TS: m(1), Kind: "agent", Agent: "Explore", ToolID: "toolu_1", Detail: "Agent: antes de la enorme"}),
				agentEnorme,
				c4v5Ev(Event{TS: m(3), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_1", Detail: "Explore termino"}),
				c4v5TextoMarcaExtra(v.estilo, m(4), v.n),
				c4v5Ev(Event{TS: m(5), Kind: "agent", Agent: "Plan", ToolID: "toolu_2", Detail: "Agent: entre las enormes"}),
				c4v5TextoMarcaDetalle(v.estilo, m(6), v.n),
				fin,
				c4v5Ev(Event{TS: m(8), Kind: "agent_end", Agent: "Plan", ToolID: "toolu_2", Detail: "Plan fallo: se corto"}),
				c4v5Ev(Event{TS: m(9), Kind: "agent", Agent: "general-purpose", ToolID: "toolu_3", Detail: "Agent: sin cierre"}),
				c4v5Ev(Event{TS: m(10), Kind: "text", Detail: "listo"}),
				c4v5Ev(Event{TS: m(11), Kind: "end", Detail: "listo"}),
			)

			// la referencia: ReadEvents entiende las lineas enormes, con el
			// detail decodificado entero
			ref := c4v5Filtradas(t, root, id)
			want := []c4v5Clave{
				{1, "agent", "Explore", "toolu_1"}, {2, "agent", "Enorme", "toolu_ENORME"},
				{3, "agent_end", "Explore", "toolu_1"}, {5, "agent", "Plan", "toolu_2"},
				{7, "agent_end", "Enorme", "toolu_ENORME"}, {8, "agent_end", "Plan", "toolu_2"},
				{9, "agent", "general-purpose", "toolu_3"},
			}
			ok := len(ref) == len(want)
			for i := 0; ok && i < len(want); i++ {
				w := want[i]
				ok = ref[i].TS.Equal(m(w.min)) && ref[i].Kind == w.kind && ref[i].Agent == w.agent && ref[i].ToolID == w.toolID
			}
			if !ok {
				t.Fatalf("fixture: ReadEvents entiende las %d delegaciones del log:\n%s", len(want), c4v5Listado(ref))
			}
			if d := ref[1].Detail; v.escenario.agentEnorme() && (len(d) < v.n*len(c4v5BloqueTexto)/len(c4v5BloqueCrudo) || !strings.HasPrefix(d, "Agent: "+c4v5BloqueTexto)) {
				t.Fatalf("fixture: ReadEvents da el detail enorme decodificado (%d bytes): %.120q", len(d), d)
			}

			ds, err := ReadDelegations(root, id)
			if err != nil {
				t.Fatalf("%s: ReadDelegations lee el log sin error: %v", ca, err)
			}
			c4v5Fantasmas(t, ca, ds)
			// el agent de Enorme es la delegacion 1 de ref y su agent_end la 4
			c4v5Comparar(t, ca, ds, ref, func(i int) bool {
				return (i == 1 && v.escenario.agentEnorme()) || (i == 4 && v.escenario.agentEndEnorme())
			})
		})
	}
}

// CA-362 (hallazgos 0ebb3f y 34a37c; enmienda 4): lineas enormes (4 MiB)
// que parecen una delegacion pero no son JSON valido despues del primer MiB
// (un escape invalido, un tabulador crudo dentro del string, una linea que
// termina sin cerrar el detail y una con basura despues del objeto), y una
// delegacion enorme valida entre ellas, se saltan todas: no sale nada de
// ninguna, y las delegaciones de tamano normal de alrededor salen enteras,
// en orden y con sus campos.
//
// El contrato viene de la enmienda 4 (decision de Henry): toda linea de
// delegacion de mas de 1 MiB se salta, valida o no. Con la enmienda 3 este
// test dejaba que las invalidas salieran o no y exigia la valida recortada
// al leerla; ahora exige que no salga ninguna. No es debilitar: es mas
// estricto con las invalidas, y el recorte de la valida se movio a la
// escritura, donde se exige (hallazgos_c4_vuelta6_test.go).
func TestHallazgo_0ebb3f_34a37c_CA362_LineaEnormeInvalidaNoTapaLasValidas(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const n = 4 * c4v5MiB
	t0 := time.Date(2026, 10, 19, 21, 0, 0, 0, time.UTC)
	m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
	invalida := func(k int, tool, medio string, cola *string) c4v5Linea {
		l := c4v5Delegacion(c4v5Compacto, m(k), "agent", "Rota", tool, "Agent: ", n)
		l.medio = medio
		if cola != nil {
			l.cola = *cola
		}
		return l
	}
	sinCierre, basura := "", `","tool_id":"toolu_BASURA"} basura`
	const id = "20261019T210000_rotas0"
	c4v5Log(t, root, id,
		c4v5Ev(Event{TS: m(0), Kind: "start", Detail: "init"}),
		c4v5Ev(Event{TS: m(1), Kind: "agent", Agent: "Explore", ToolID: "toolu_1", Detail: "Agent: antes"}),
		invalida(2, "toolu_ESCAPE", `\x`, nil),
		invalida(3, "toolu_TAB", "\t", nil),
		c4v5Delegacion(c4v5Compacto, m(4), "agent", "Enorme", "toolu_ENORME", "Agent: ", n),
		invalida(5, "toolu_SINCIERRE", "", &sinCierre),
		invalida(6, "toolu_BASURA", "", &basura),
		c4v5Ev(Event{TS: m(7), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_1", Detail: "Explore termino"}),
		c4v5Ev(Event{TS: m(8), Kind: "agent_end", Agent: "Enorme", ToolID: "toolu_ENORME", Detail: "Enorme termino"}),
		c4v5Ev(Event{TS: m(9), Kind: "end", Detail: "listo"}),
	)
	ref := c4v5Filtradas(t, root, id)
	var claves []string
	for _, e := range ref {
		claves = append(claves, e.Kind+" "+e.ToolID)
	}
	if got, want := strings.Join(claves, ", "), "agent toolu_1, agent toolu_ENORME, agent_end toolu_1, agent_end toolu_ENORME"; got != want {
		t.Fatalf("fixture: ReadEvents salta las 4 lineas invalidas y lee las demas: %s, quiere %s", got, want)
	}
	ds, err := ReadDelegations(root, id)
	if err != nil {
		t.Fatalf("CA-362 (hallazgos 0ebb3f y 34a37c): ReadDelegations lee el log sin error: %v", err)
	}
	for _, d := range ds {
		if d.Agent == "Rota" {
			t.Errorf("CA-362 (hallazgos 0ebb3f y 34a37c, enmienda 4): una linea de delegacion de mas de 1 MiB que no es JSON valido se salta; salio %s\n%s",
				c4v5Resumen(d), c4v5Listado(ds))
		}
	}
	// la delegacion enorme valida es la 1 de ref: su linea de 4 MiB se salta
	c4v5Comparar(t, "CA-362 (hallazgos 0ebb3f y 34a37c, enmienda 4, lineas invalidas alrededor)", ds, ref,
		func(i int) bool { return i == 1 })
}

// CA-362 (hallazgos 0ebb3f y 34a37c; enmienda 4; por propiedad): para logs
// al azar con delegaciones cuyo detail anda alrededor de 1 MiB (de
// 1 MiB - 8 KiB a 3 MiB, con agentes y tool_id con acentos, comillas, barras
// y emojis, en los tres estilos, y con la marca "kind":"agent" en lineas
// text cercanas), ReadDelegations trae lo mismo que ReadEvents filtrado sin
// las delegaciones cuya linea pasa de 1 MiB: cada una una vez, en orden y
// entera (ts, kind, agent, tool_id y detail iguales); las de mas de 1 MiB no
// salen.
//
// El contrato viene de la enmienda 4 (decision de Henry): con la enmienda 3
// una linea de mas de 1 MiB salia recortada al leerla y las de otro orden de
// claves quedaban fuera; ahora toda linea de delegacion de mas de 1 MiB se
// salta, en cualquier orden de claves, asi que la propiedad vuelve a
// generarlas en los tres estilos. No es debilitar: el recorte se movio a la
// escritura, donde se exige (hallazgos_c4_vuelta6_test.go), y cada linea de
// hasta 1 MiB se sigue exigiendo entera.
func TestHallazgo_0ebb3f_34a37c_CA362_AlrededorDeUnMiB(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 19, 22, 0, 0, 0, time.UTC)
	agentes := []string{"Explore", "Plan", "ñandú \"el\" agente", `barra\invertida`, "🙂 emoji", "general-purpose"}
	caso := 0
	prop := func(semilla int64) bool {
		caso++
		r := rand.New(rand.NewSource(semilla))
		id := fmt.Sprintf("20261019T220000_q%05d", caso)
		var ls []c4v5Linea
		var larga []bool // por delegacion, en orden: su linea pasa de 1 MiB
		k := 0
		for i, total := 0, 2+r.Intn(4); i < total; i++ {
			ts := t0.Add(time.Duration(caso)*time.Hour + time.Duration(i)*time.Minute + time.Duration(r.Intn(1000))*time.Millisecond)
			kind := []string{"agent", "agent_end"}[r.Intn(2)]
			agente := agentes[r.Intn(len(agentes))]
			tool := fmt.Sprintf("toolu_%x_ñ\"%d", r.Int63(), i)
			var n int
			switch r.Intn(4) {
			case 0: // la linea alrededor de 1 MiB
				n = c4v5MiB - 8<<10 + r.Intn(16<<10)
			case 1: // el detail decodificado alrededor de 1 MiB
				n = (c4v5MiB - 8<<10 + r.Intn(16<<10)) * len(c4v5BloqueCrudo) / len(c4v5BloqueTexto)
			case 2:
				n = c4v5MiB + r.Intn(2*c4v5MiB)
			default:
				n = r.Intn(4 << 10)
			}
			l := c4v5Delegacion(r.Intn(3), ts, kind, agente, tool, "Agent: ", n)
			larga = append(larga, c4v5Largo(l) > c4v5MiB)
			k++
			ls = append(ls, l)
			if r.Intn(3) == 0 {
				ls = append(ls, c4v5TextoMarcaExtra(r.Intn(3), ts.Add(time.Second), r.Intn(2*c4v5MiB)))
			}
		}
		c4v5Log(t, root, id, ls...)
		ref := c4v5Filtradas(t, root, id)
		if len(ref) != k {
			t.Errorf("fixture: con la semilla %d ReadEvents lee las %d delegaciones; leyo %d", semilla, k, len(ref))
			return false
		}
		ds, err := ReadDelegations(root, id)
		if err != nil {
			t.Errorf("CA-362 (hallazgos 0ebb3f y 34a37c): con la semilla %d ReadDelegations lee el log sin error: %v", semilla, err)
			return false
		}
		ca := fmt.Sprintf("CA-362 (hallazgos 0ebb3f y 34a37c, semilla %d)", semilla)
		antes := t.Failed()
		c4v5Fantasmas(t, ca, ds)
		c4v5Comparar(t, ca, ds, ref, func(i int) bool { return larga[i] })
		return t.Failed() == antes
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 12, Rand: rand.New(rand.NewSource(0xebb3f))}); err != nil {
		t.Fatalf("CA-362 (hallazgos 0ebb3f y 34a37c, por propiedad): %v", err)
	}
}

// CA-362 (hallazgos 0ebb3f y 34a37c, caso encontrado en caja negra): una
// delegacion valida de hasta 1 MiB escrita con las claves en otro orden (el
// detail primero y kind, agent, tool_id y ts despues) se lee entera, con su
// ts, su kind, su agent y su tool_id (un detail de 64 KiB, de 512 KiB y de
// casi 1 MiB); su agent_end emparejado, despues, tambien sale.
//
// Enmienda 4 (decision de Henry, reemplaza a la 3): una linea de hasta 1 MiB
// se lee entera en cualquier orden de claves y una de mas de 1 MiB se salta
// en cualquier orden de claves, asi que vuelve el caso de 4 MiB (que la
// enmienda 3 dejaba fuera): su agent no sale y su agent_end, normal, si.
func TestHallazgo_0ebb3f_34a37c_CA362_KindDespuesDelDetail(t *testing.T) {
	for _, n := range []int{64 << 10, 512 << 10, c4v5MiB - 4<<10, 4 * c4v5MiB} {
		t.Run(fmt.Sprintf("detail de %d KiB", n>>10), func(t *testing.T) {
			ca := fmt.Sprintf("CA-362 (hallazgos 0ebb3f y 34a37c, kind despues de un detail de %d KiB)", n>>10)
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t0 := time.Date(2026, 10, 19, 23, 30, 0, 0, time.UTC)
			m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
			const id = "20261019T233000_orden0"
			l := c4v5Delegacion(c4v5OtroOrden, m(1), "agent", "Plan", "toolu_ORDEN", "Agent: ", n)
			c4v5Log(t, root, id,
				c4v5Ev(Event{TS: m(0), Kind: "start", Detail: "init"}),
				l,
				c4v5Ev(Event{TS: m(2), Kind: "agent_end", Agent: "Plan", ToolID: "toolu_ORDEN", Detail: "Plan termino"}),
				c4v5Ev(Event{TS: m(3), Kind: "end", Detail: "listo"}),
			)
			ref := c4v5Filtradas(t, root, id)
			if len(ref) != 2 || ref[0].Kind != "agent" || ref[0].ToolID != "toolu_ORDEN" || ref[1].Kind != "agent_end" {
				t.Fatalf("fixture: ReadEvents lee la delegacion con las claves en otro orden y su cierre:\n%s", c4v5Listado(ref))
			}
			if (c4v5Largo(l) > c4v5MiB) != (n > c4v5MiB) {
				t.Fatalf("fixture: la linea con las claves en otro orden pasa de 1 MiB solo con el detail de 4 MiB: %d bytes", c4v5Largo(l))
			}
			ds, err := ReadDelegations(root, id)
			if err != nil {
				t.Fatalf("%s: ReadDelegations lee el log sin error: %v", ca, err)
			}
			c4v5Comparar(t, ca, ds, ref, func(i int) bool { return i == 0 && c4v5Largo(l) > c4v5MiB })
		})
	}
}
