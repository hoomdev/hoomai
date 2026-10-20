// Tests de regresion de la sexta vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd. Escritos desde el spec
// (seccion "Fuente telemetria") y la descripcion de los hallazgos, sin leer
// la implementacion. Los helpers de este archivo llevan el prefijo c4v6; los
// compartidos con las otras vueltas (prefijo dlg) estan en
// delegaciones_helpers_test.go.
//
// El contrato es el de la enmienda 5 (decision de Henry), que reemplaza a la
// 4: hoom NO recorta las delegaciones.
//
//   - Escritura: hoom escribe cada evento agent y agent_end de un run tal
//     como lo arma el normalizador (que ya recorta a 300 bytes la descripcion
//     de la delegacion y el texto del fallo), con su ts, su agente, su
//     tool_id y su detail enteros, mida lo que mida su linea, y el run guarda
//     en memoria el mismo evento que escribe.
//   - Lectura: ReadDelegations lee entera, en cualquier orden de claves, cada
//     linea de delegacion de hasta 1 MiB, sin contar el salto, y salta una
//     mas larga, como salta una mal formada o a medio escribir.
//   - Fuera del contrato: una delegacion de mas de 1 MiB (solo sale de un
//     agente o un tool_id enormes del provider; con la enmienda 6 el estado
//     de un fallo se recorta a 300 bytes), en un log nuevo o viejo. A su
//     linea estos tests solo le exigen que se salte sin tapar a sus vecinas.
//
// Una delegacion "escrita como hoom" es, en estos tests, una linea de hasta
// 1 MiB.
//
// Hallazgos de la delta 2 de C4 que cubren:
//
//   - 20261019T191412_550f79: el tope de 1 MiB contaba el salto de linea; una
//     delegacion de exactamente 1 MiB no se leia entera.
//   - 20261019T191919_dae2e8: una linea de exactamente 1 MiB salia sin tool_id
//     y la historia no podia emparejar su agent_end.
//   - 20261019T191925_7d07ce: una linea de mas de 1 MiB a medio escribir (sin
//     su salto) se volvia un subagente fantasma.
//
// Y de la delta 3 (los dos tests de escritura, que con la enmienda 4 exigian
// el recorte al escribir):
//
//   - 20261019T213022_b8d4a1 y 20261019T213459_733ac3: el recorte de la
//     enmienda 4 cortaba el detail de un agent_end con agente largo y borraba
//     " fallo", y la historia publicaba el fallo como "termino su parte".
//
// Los runs son de verdad (Manager.Start con un binario falso en el PATH): un
// provider falso registrado en providers.Default que devuelve tal cual el
// evento que trae cada linea (asi un agent o un agent_end puede traer un
// detail de mas de 1 MiB, que el normalizador de Claude no deja pasar: corta
// la descripcion y el texto del fallo), y un claude falso con subagentes de
// nombre largo, que hacen pasar de 1 MiB la linea de su agent_end.
package runcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// c4v6CorteViejo es donde la enmienda 4 cortaba el detail de una delegacion
// larga al escribirla (sus primeros 128 KiB). Con la enmienda 5 no se corta.
const c4v6CorteViejo = 128 << 10

// c4v6AvisoViejo es el comienzo del aviso que la enmienda 4 agregaba al
// detail recortado. Con la enmienda 5 hoom no lo escribe nunca.
const c4v6AvisoViejo = "detalle recortado al escribir"

// c4v6Escapes arma un texto de n bytes crudos que en JSON ocupa al menos el
// triple con cualquier codificador (con o sin escapar <, > y &): caracteres
// de control, comillas, barras, saltos y tabuladores.
func c4v6Escapes(n int) string {
	return strings.Repeat("\x01\x1f\"\\<&\n\t", n/8)
}

// c4v6ConLineaASCII devuelve e con su detail alargado con 'x' para que su
// linea mida exactamente largo bytes (ASCII sin escapes: cualquier
// codificador de JSON da lo mismo).
func c4v6ConLineaASCII(t *testing.T, e Event, largo int) Event {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > largo {
		t.Fatalf("fixture: la linea sin relleno ya mide %d bytes, mas que %d", len(raw), largo)
	}
	e.Detail += strings.Repeat("x", largo-len(raw))
	if menor, mayor := dlgLargoJSON(t, e); menor != largo || mayor != largo {
		t.Fatalf("fixture: la linea mide %d/%d bytes, no %d", menor, mayor, largo)
	}
	return e
}

// El provider falso: cada linea de su salida es un evento en JSON, y
// Normalize lo devuelve tal cual (ts incluido). Una linea que no es un evento
// sale como text.
type c4v6Proveedor struct{}

const c4v6Nombre = "c4v6-falso"

func (c4v6Proveedor) Name() string { return c4v6Nombre }
func (c4v6Proveedor) Bin() string  { return c4v6Nombre }
func (c4v6Proveedor) Capabilities() providers.Capabilities {
	return providers.Capabilities{Structured: true}
}
func (c4v6Proveedor) Command(providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: c4v6Nombre}, nil
}
func (c4v6Proveedor) Normalize(line string) []providers.Event {
	var e providers.Event
	if err := json.Unmarshal([]byte(line), &e); err != nil || e.Kind == "" {
		return []providers.Event{{TS: time.Now().UTC(), Kind: "text", Detail: line}}
	}
	return []providers.Event{e}
}

var c4v6Registro sync.Once

// c4v6Correr hace un run de verdad del provider falso cuya salida son los
// eventos evs, uno por linea, y devuelve root, el id del run y lo que el run
// tiene en memoria (Manager.Events).
func c4v6Correr(t *testing.T, evs []Event) (root, id string, mem []Event) {
	t.Helper()
	var errReg error
	c4v6Registro.Do(func() { errReg = providers.Default.Register(c4v6Proveedor{}) })
	if errReg != nil {
		t.Fatalf("fixture: registrar el provider falso: %v", errReg)
	}
	var salida bytes.Buffer
	for _, e := range evs {
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) >= 4*dlgMiB-1024 {
			t.Fatalf("fixture: la linea del provider queda debajo de 4 MiB: %d bytes", len(raw))
		}
		salida.Write(raw)
		salida.WriteByte('\n')
	}
	return dlgCorrerSalida(t, c4v6Nombre, salida.Bytes())
}

// CA-362 (hallazgos 550f79, dae2e8 y 7d07ce de la delta 2, y b8d4a1 y 733ac3
// de la delta 3; enmienda 5): un run de verdad cuyo provider emite
// delegaciones (cada agent con su agent_end) de todos los tamanos, y hoom
// las escribe TAL CUAL las arma el normalizador, sin recortar nada ni
// agregar un aviso:
//   - una delegacion normal y una con un detail de 200 KiB, cuyas lineas
//     entran en 1 MiB;
//   - una con un detail de 1.5 MiB (ASCII, comillas, barras y caracteres de
//     2, 3 y 4 bytes) y una con 400 KiB crudos que escapados en JSON pasan de
//     1 MiB (caracteres de control, comillas, barras, <, &);
//   - una con un caracter de 2, 3 o 4 bytes que cruza el byte 128 KiB del
//     detail, donde cortaba la enmienda 4;
//   - en el borde, con detail ASCII: lineas de exactamente 1 MiB y de
//     1 MiB + 1, de un agent y de un agent_end que falla;
//   - un text de 1.5 MiB, que no es una delegacion, tambien sale entero.
//
// ts, agente, tool_id y detail quedan enteros; lo que el run tiene en
// memoria (Manager.Events) es lo que escribio; y ReadDelegations trae entera
// cada delegacion escrita en una linea de hasta 1 MiB y salta las mas
// largas sin tapar a las vecinas.
//
// Antes se llamaba HoomEscribeLaDelegacionEnUnMiB y exigia el recorte AL
// ESCRIBIR de la enmienda 4 (los primeros 128 KiB del detail y un aviso). El
// contrato viene ahora de la enmienda 5 (decision de Henry): ese recorte
// borraba la marca " fallo" de un agent_end con agente largo y la historia
// publicaba el fallo como exito (b8d4a1, 733ac3). No es debilitar: lo que se
// exige ahora es que hoom no altere el evento, y cada linea de hasta 1 MiB se
// sigue exigiendo leida entera.
func TestHallazgo_550f79_dae2e8_7d07ce_CA362_HoomEscribeLaDelegacionTalCual(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 21, 0, 0, 123456789, time.UTC)
	seg := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Second) }
	type caso struct {
		dlgCaso
		entra bool // su linea entra en 1 MiB con cualquier codificador
	}
	var casos []caso
	var evs []Event
	k := 0
	agregar := func(nombre string, entra bool, es ...Event) {
		for _, e := range es {
			casos = append(casos, caso{dlgCaso{nombre + " (" + e.Kind + ")", e}, entra})
		}
	}
	par := func(nombre, agente, tool, detAgent, detFin string, entra bool) {
		a := Event{TS: seg(k), Kind: "agent", Agent: agente, ToolID: tool, Detail: detAgent}
		f := Event{TS: seg(k + 1), Kind: "agent_end", Agent: agente, ToolID: tool, Detail: detFin}
		k += 2
		evs = append(evs, a, Event{TS: seg(k), Kind: "text", Detail: "sigo"}, f)
		k++
		agregar(nombre, entra, a, f)
	}

	par("delegacion normal", "Explore", "toolu_NORMAL", "Agent: buscar precios", "Explore termino", true)
	par("detail de 1.5 MiB", "Enorme", "toolu_ENORME",
		"Agent: "+dlgMezcla(3*dlgMiB/2), "Enorme fallo: "+dlgMezcla(3*dlgMiB/2), false)
	par("detail de 400 KiB que escapado pasa de 1 MiB", "Escapado", "toolu_ESCAPADO",
		"Agent: "+c4v6Escapes(400<<10), "Escapado fallo: "+c4v6Escapes(400<<10), false)
	par("detail de 200 KiB que entra", "Entra", "toolu_ENTRA",
		"Agent: "+dlgMezcla(200<<10), "Entra termino: "+dlgMezcla(200<<10), true)
	for _, r := range []string{"ñ", "漢", "🙂"} {
		// el caracter r empieza en el byte 128 KiB - 1 del detail y cruza el
		// corte de la enmienda 4; el resto lleva la linea por encima de 1 MiB
		inicio := c4v6CorteViejo - 1
		runaEn := func(prefijo string) string {
			return prefijo + strings.Repeat("x", inicio-len(prefijo)) + r + dlgMezcla(dlgMiB-c4v6CorteViejo+16<<10)
		}
		detAgent, detFin := runaEn("Agent: "), runaEn("Runas fallo: ")
		for _, d := range []string{detAgent, detFin} {
			if d[inicio:inicio+len(r)] != r || inicio+len(r) <= c4v6CorteViejo {
				t.Fatalf("fixture: %q empieza en el byte %d y cruza el byte %d", r, inicio, c4v6CorteViejo)
			}
		}
		par(fmt.Sprintf("%q de %d bytes que cruza el byte 128 KiB", r, len(r)), "Runas", fmt.Sprintf("toolu_RUNA_%d", len(r)),
			detAgent, detFin, false)
	}
	// el borde: lineas de exactamente 1 MiB (entran) y de 1 MiB + 1 (no)
	for _, largo := range []int{dlgMiB, dlgMiB + 1} {
		tool := fmt.Sprintf("toolu_BORDE_%+d", largo-dlgMiB)
		a := c4v6ConLineaASCII(t, Event{TS: seg(k), Kind: "agent", Agent: "Borde", ToolID: tool, Detail: "Agent: "}, largo)
		f := c4v6ConLineaASCII(t, Event{TS: seg(k + 1), Kind: "agent_end", Agent: "Borde", ToolID: tool, Detail: "Borde fallo: "}, largo)
		k += 2
		evs = append(evs, a, f)
		agregar(fmt.Sprintf("linea de %d bytes", largo), largo <= dlgMiB, a, f)
	}
	texto := Event{TS: seg(k), Kind: "text", Detail: "un text enorme: " + dlgMezcla(3*dlgMiB/2)}
	ultimo := Event{TS: seg(k + 1), Kind: "agent", Agent: "general-purpose", ToolID: "toolu_ULTIMO", Detail: "Agent: sin cierre"}
	evs = append(evs, texto, ultimo)
	agregar("delegacion sin cierre despues del text enorme", true, ultimo)

	// los casos son los que dicen ser
	var dc []dlgCaso
	for _, c := range casos {
		menor, mayor := dlgLargoJSON(t, c.ev)
		if (c.entra && mayor > dlgMiB) || (!c.entra && menor <= dlgMiB) {
			t.Fatalf("fixture: %s: la linea entra en 1 MiB = %v con cualquier codificador: mide %d/%d", c.nombre, c.entra, menor, mayor)
		}
		dc = append(dc, c.dlgCaso)
	}
	for _, c := range casos {
		if c.ev.ToolID == "toolu_ESCAPADO" && len(c.ev.Detail) >= dlgMiB {
			t.Fatalf("fixture: el detail que pasa de 1 MiB escapado mide menos de 1 MiB crudo: %d bytes", len(c.ev.Detail))
		}
	}

	root, id, mem := c4v6Correr(t, evs)
	ca := "CA-362 (hallazgos 550f79, dae2e8, 7d07ce, b8d4a1 y 733ac3, enmienda 5)"
	disco := dlgExigirTalCual(t, ca, root, id, mem, dc)

	// el borde se escribio en el borde: con la delegacion tal cual, la linea
	// de exactamente 1 MiB se lee y la de 1 MiB + 1 se salta (lo exige
	// dlgExigirTalCual segun el largo en el disco)
	for _, l := range disco {
		if l.ok && strings.HasPrefix(l.ev.ToolID, "toolu_BORDE_") {
			if want := dlgMiB + map[string]int{"toolu_BORDE_+0": 0, "toolu_BORDE_+1": 1}[l.ev.ToolID]; len(l.raw) != want {
				t.Logf("aviso: la linea de %s se escribio con %d bytes, no %d: el borde no se probo en el disco", dlgResumen(l.ev), len(l.raw), want)
			}
		}
	}

	// el text enorme no es una delegacion: tambien se escribe entero
	var textos []Event
	for _, l := range disco {
		if l.ok && l.ev.Kind == "text" && strings.HasPrefix(l.ev.Detail, "un text enorme: ") {
			textos = append(textos, l.ev)
		}
	}
	if len(textos) != 1 || textos[0].Detail != texto.Detail || strings.Contains(textos[0].Detail, c4v6AvisoViejo) {
		t.Errorf("%s: un text de %d bytes se escribe entero, sin aviso de recorte; se escribio %s",
			ca, len(texto.Detail), dlgListado(textos))
	}
}

// CA-362 (hallazgos 550f79, dae2e8 y 7d07ce, y b8d4a1 y 733ac3; enmienda 5),
// con el claude de CA-374: un run de Claude que delega en dos subagentes de
// nombre largo (subagent_type de unos 660 KiB), uno termina bien y el otro
// falla (tool_result con is_error), entre dos delegaciones normales. hoom
// escribe cada agent y agent_end tal como lo arma el normalizador de Claude
// para la misma salida, sin recortar nada: el agent_end ("<agente> termino" y
// "<agente> fallo: ...") con el agente y el detail enteros, aunque su linea
// pase de 1 MiB. Lo que el run tiene en memoria es lo escrito; ReadDelegations
// trae enteros los agent de nombre largo (lineas de unos 660 KiB) y las
// delegaciones normales, y salta los dos agent_end de mas de 1 MiB sin tapar
// a sus vecinas.
//
// Con la enmienda 4 este test exigia el agent_end recortado al escribirlo, y
// ese recorte borraba " fallo" (b8d4a1, 733ac3). El contrato viene ahora de
// la enmienda 5 (decision de Henry), y no es debilitar: lo que se exige es
// que hoom no altere el evento.
func TestHallazgo_550f79_dae2e8_7d07ce_CA362_RunDeClaudeConSubagenteDeNombreLargo(t *testing.T) {
	agenteOK := "hoom-" + strings.Repeat("largo-ñ-", 75000)
	agenteErr := "hoom-" + strings.Repeat("otro-漢-", 68000)
	salida := dlgClaudeSalida(
		dlgClaudeToolUse("toolu_ANTES", "Explore", false),
		dlgClaudeResultado("toolu_ANTES", false, "los precios"),
		dlgClaudeToolUse("toolu_LARGO_OK", agenteOK, false),
		dlgClaudeResultado("toolu_LARGO_OK", false, "el mapa del modulo"),
		dlgClaudeToolUse("toolu_LARGO_ERR", agenteErr, false),
		dlgClaudeResultado("toolu_LARGO_ERR", true, "el subagente se quedo sin turnos"),
		dlgClaudeToolUse("toolu_DESPUES", "general-purpose", false),
		dlgClaudeResultado("toolu_DESPUES", false, "listo"),
	)

	// la referencia: el normalizador de Claude de un run, con la misma salida
	ref := dlgNormalizarClaude(t, salida)
	if len(ref) != 8 || ref[2].Agent != agenteOK || ref[3].Kind != "agent_end" || ref[3].ToolID != "toolu_LARGO_OK" ||
		ref[5].Agent != agenteErr || ref[5].ToolID != "toolu_LARGO_ERR" || !strings.HasPrefix(ref[5].Detail, agenteErr+" fallo") ||
		ref[0].ToolID != "toolu_ANTES" || ref[7].ToolID != "toolu_DESPUES" {
		t.Fatalf("fixture: el normalizador de Claude da agent y agent_end de cada subagente, en orden:\n%s", dlgListado(ref))
	}
	var casos []dlgCaso
	for i, e := range ref {
		menor, mayor := dlgLargoJSON(t, e)
		larga := e.Kind == "agent_end" && len(e.Agent) > 1000
		if (larga && menor <= dlgMiB) || (!larga && mayor > dlgMiB) {
			t.Fatalf("fixture: solo la linea del agent_end de nombre largo pasa de 1 MiB: %s mide %d", dlgResumen(e), menor)
		}
		casos = append(casos, dlgCaso{fmt.Sprintf("delegacion %d", i), e})
	}

	root, id, mem := dlgCorrerSalida(t, "claude", salida)
	ca := "CA-362 (hallazgos 550f79, dae2e8, 7d07ce, b8d4a1 y 733ac3, enmienda 5, run de Claude)"
	dlgTSDelDisco(t, ca, root, id, casos)
	dlgExigirTalCual(t, ca, root, id, mem, casos)
}

// Los ordenes de las claves de una linea de delegacion escrita a mano.
type c4v6Orden int

const (
	c4v6OrdenHoom    c4v6Orden = iota // como json.Marshal: ts, kind, agent, detail, tool_id
	c4v6OrdenInverso                  // tool_id, detail, agent, kind, ts
)

func (o c4v6Orden) String() string {
	if o == c4v6OrdenInverso {
		return "claves en otro orden"
	}
	return "claves como las escribe hoom"
}

// c4v6Comienzo es el comienzo del detail de las lineas escritas a mano, como
// va en el JSON (con comillas escapadas y caracteres de varios bytes).
const c4v6Comienzo = `Agent: ñandú 漢字 🙂 \"comillas\" `

// c4v6LineaDeLargo arma una linea de delegacion compacta de exactamente
// largo bytes sin el salto, con el detail relleno de ASCII.
func c4v6LineaDeLargo(t *testing.T, largo int, orden c4v6Orden, ts time.Time, kind, agente, tool string) string {
	t.Helper()
	tsS := ts.UTC().Format(time.RFC3339Nano)
	cab := `{"ts":"` + tsS + `","kind":"` + kind + `","agent":"` + agente + `","detail":"` + c4v6Comienzo
	cola := `","tool_id":"` + tool + `"}`
	if orden == c4v6OrdenInverso {
		cab = `{"tool_id":"` + tool + `","detail":"` + c4v6Comienzo
		cola = `","agent":"` + agente + `","kind":"` + kind + `","ts":"` + tsS + `"}`
	}
	n := largo - len(cab) - len(cola)
	if n < 0 {
		t.Fatalf("fixture: una linea de %d bytes no entra la cabeza y la cola", largo)
	}
	relleno := strings.Repeat("abcdefghij", n/10+1)[:n]
	l := cab + relleno + cola
	if len(l) != largo {
		t.Fatalf("fixture: la linea mide %d bytes, no %d", len(l), largo)
	}
	return l
}

// c4v6Log escribe el log del run id con las lineas dadas tal cual; cada una
// lleva su salto salvo que sinSaltoFinal y sea la ultima.
func c4v6Log(t *testing.T, root, id string, sinSaltoFinal bool, lineas ...string) {
	t.Helper()
	p := filepath.Join(root, ".hoom", "runs", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	for i, l := range lineas {
		b.WriteString(l)
		if !sinSaltoFinal || i < len(lineas)-1 {
			b.WriteByte('\n')
		}
	}
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// c4v6JSON es la linea de un evento como la escribe json.Marshal.
func c4v6JSON(t *testing.T, e Event) string {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// c4v6Filtradas es ReadEvents filtrado por agent y agent_end.
func c4v6Filtradas(t *testing.T, root, id string) []Event {
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

func c4v6Iguales(a, b []Event) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !dlgMismoEvento(a[i], b[i]) {
			return false
		}
	}
	return true
}

// c4v6Vecinas arma un log del run id con una linea de delegacion en el medio
// (borde) entre delegaciones de tamano normal, y devuelve ReadEvents filtrado
// y las delegaciones vecinas.
func c4v6Vecinas(t *testing.T, root, id, borde string) (ref, vecinas []Event) {
	t.Helper()
	t0 := time.Date(2026, 10, 19, 22, 0, 0, 0, time.UTC)
	m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
	vecinas = []Event{
		{TS: m(1), Kind: "agent", Agent: "Explore", ToolID: "toolu_1", Detail: "Agent: antes del borde"},
		{TS: m(3), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_1", Detail: "Explore termino"},
		{TS: m(4), Kind: "agent", Agent: "general-purpose", ToolID: "toolu_3", Detail: "Agent: despues del borde"},
	}
	c4v6Log(t, root, id, false,
		c4v6JSON(t, Event{TS: m(0), Kind: "start", Detail: "init"}),
		c4v6JSON(t, vecinas[0]),
		borde,
		c4v6JSON(t, vecinas[1]),
		c4v6JSON(t, vecinas[2]),
		c4v6JSON(t, Event{TS: m(5), Kind: "end", Detail: "listo"}),
	)
	return c4v6Filtradas(t, root, id), vecinas
}

// CA-362 (hallazgos 550f79 y dae2e8; enmienda 5, como la 4): una linea de
// delegacion compacta de exactamente 1 MiB (y de 1 MiB - 1) mas su salto se lee ENTERA:
// el tope de 1 MiB no cuenta el salto. ReadDelegations la trae con su
// tool_id, su agente, su ts y su detail iguales a los de ReadEvents, entre
// sus vecinas, para un agent y un agent_end, con las claves como las escribe
// hoom y en otro orden.
func TestHallazgo_550f79_dae2e8_CA362_LineaDeHastaUnMiBSeLeeEntera(t *testing.T) {
	ts := time.Date(2026, 10, 19, 22, 2, 0, 0, time.UTC)
	for _, largo := range []int{dlgMiB, dlgMiB - 1} {
		for _, kind := range []string{"agent", "agent_end"} {
			for _, orden := range []c4v6Orden{c4v6OrdenHoom, c4v6OrdenInverso} {
				nombre := fmt.Sprintf("%s de %d bytes, %s", kind, largo, orden)
				t.Run(nombre, func(t *testing.T) {
					ca := "CA-362 (hallazgos 550f79 y dae2e8, " + nombre + ")"
					root := t.TempDir()
					const id = "20261019T220000_borde0"
					ref, vecinas := c4v6Vecinas(t, root, id, c4v6LineaDeLargo(t, largo, orden, ts, kind, "Borde", "toolu_BORDE"))
					if len(ref) != 4 || ref[1].Kind != kind || ref[1].ToolID != "toolu_BORDE" || ref[1].Agent != "Borde" ||
						!ref[1].TS.Equal(ts) || !strings.HasPrefix(ref[1].Detail, "Agent: ñandú 漢字 🙂 \"comillas\" abcdefghij") ||
						!dlgMismoEvento(ref[0], vecinas[0]) {
						t.Fatalf("fixture: ReadEvents lee la linea de %d bytes entre sus vecinas:\n%s", largo, dlgListado(ref))
					}
					ds, err := ReadDelegations(root, id)
					if err != nil {
						t.Fatalf("%s: ReadDelegations lee el log: %v", ca, err)
					}
					if !c4v6Iguales(ds, ref) {
						t.Errorf("%s: una linea de delegacion de hasta 1 MiB sin contar el salto se lee entera, con su tool_id y su detail, como la lee ReadEvents; trajo %d:\n%squiere:\n%s",
							ca, len(ds), dlgListado(ds), dlgListado(ref))
					}
				})
			}
		}
	}
}

// CA-362 (hallazgos 550f79 y dae2e8; enmienda 5, como la 4): una linea de
// delegacion de 1 MiB + 1 (y de 1 MiB + 2 y 3 MiB) queda fuera de lo que la
// historia lee: ReadDelegations la salta entera, sin sacar nada de ella (ni
// recortada ni sin tool_id), y trae sus vecinas exactas y en orden. ReadEvents
// si la entiende: es JSON valido, la salta solo la historia. Con la enmienda 5
// hoom puede escribir una linea asi (un agente o un tool_id enormes del
// provider; con la enmienda 6 el estado de un fallo se recorta a 300 bytes):
// esa delegacion queda fuera de contrato, y lo que se le exige es que no tape
// a sus vecinas.
func TestHallazgo_550f79_dae2e8_CA362_LineaDeMasDeUnMiBSeSalta(t *testing.T) {
	ts := time.Date(2026, 10, 19, 22, 2, 0, 0, time.UTC)
	for _, largo := range []int{dlgMiB + 1, dlgMiB + 2, 3 * dlgMiB} {
		for _, kind := range []string{"agent", "agent_end"} {
			for _, orden := range []c4v6Orden{c4v6OrdenHoom, c4v6OrdenInverso} {
				nombre := fmt.Sprintf("%s de %d bytes, %s", kind, largo, orden)
				t.Run(nombre, func(t *testing.T) {
					ca := "CA-362 (hallazgos 550f79 y dae2e8, " + nombre + ")"
					root := t.TempDir()
					const id = "20261019T220000_borde1"
					ref, vecinas := c4v6Vecinas(t, root, id, c4v6LineaDeLargo(t, largo, orden, ts, kind, "Borde", "toolu_BORDE"))
					if len(ref) != 4 || ref[1].ToolID != "toolu_BORDE" || ref[1].Kind != kind {
						t.Fatalf("fixture: ReadEvents lee la linea de %d bytes (JSON valido):\n%s", largo, dlgListado(ref))
					}
					ds, err := ReadDelegations(root, id)
					if err != nil {
						t.Fatalf("%s: ReadDelegations lee el log: %v", ca, err)
					}
					if !c4v6Iguales(ds, vecinas) {
						t.Errorf("%s: una linea de delegacion de mas de 1 MiB se salta sin tapar a sus vecinas: trae solo las %d vecinas, exactas y en orden; trajo %d:\n%squiere:\n%s",
							ca, len(vecinas), len(ds), dlgListado(ds), dlgListado(vecinas))
					}
				})
			}
		}
	}
}

// CA-362 (hallazgos 550f79 y dae2e8; enmienda 5, como la 4), la ventana donde el
// refutador vio perderse el tool_id: para cada largo de 1 MiB - 16 a
// 1 MiB + 64 bytes (agent y agent_end, con las claves como las escribe hoom
// y en otro orden, alternados), la linea de hasta 1 MiB sale entera, como la
// lee ReadEvents, y la de mas de 1 MiB no sale; nada sale sin su tool_id, y
// las vecinas salen siempre exactas y en orden.
func TestHallazgo_550f79_dae2e8_CA362_VentanaAlrededorDeUnMiB(t *testing.T) {
	ts := time.Date(2026, 10, 19, 22, 2, 0, 0, time.UTC)
	var malos []string
	for largo := dlgMiB - 16; largo <= dlgMiB+64; largo++ {
		kind := []string{"agent", "agent_end"}[largo%2]
		orden := []c4v6Orden{c4v6OrdenHoom, c4v6OrdenInverso}[(largo/2)%2]
		root := t.TempDir()
		id := fmt.Sprintf("20261019T220000_v%05d", largo-dlgMiB+16)
		ref, vecinas := c4v6Vecinas(t, root, id, c4v6LineaDeLargo(t, largo, orden, ts, kind, "Borde", "toolu_BORDE"))
		if len(ref) != 4 || ref[1].ToolID != "toolu_BORDE" {
			t.Fatalf("fixture: ReadEvents lee la linea de %d bytes:\n%s", largo, dlgListado(ref))
		}
		want := vecinas
		if largo <= dlgMiB {
			want = ref
		}
		ds, err := ReadDelegations(root, id)
		if err != nil {
			t.Fatalf("CA-362: ReadDelegations lee el log de %d bytes: %v", largo, err)
		}
		if !c4v6Iguales(ds, want) {
			que := "se salta"
			if largo <= dlgMiB {
				que = "sale entera"
			}
			var sinTool []string
			for _, d := range ds {
				if d.ToolID == "" {
					sinTool = append(sinTool, dlgResumen(d))
				}
			}
			malos = append(malos, fmt.Sprintf("  %d bytes (%+d), %s, %s: %s; trajo %d delegaciones, %d sin tool_id %v",
				largo, largo-dlgMiB, kind, orden, que, len(ds), len(sinTool), sinTool))
		}
	}
	if len(malos) > 0 {
		t.Errorf("CA-362 (hallazgos 550f79 y dae2e8): alrededor de 1 MiB la linea de hasta 1 MiB sale entera y la de mas no sale, sin tapar a sus vecinas; fallan %d largos:\n%s",
			len(malos), strings.Join(malos, "\n"))
	}
}

// CA-362 (hallazgo 7d07ce; enmienda 5, como la 4): un log que termina en una
// linea de delegacion a medio escribir (cortada y sin su salto: un append en
// curso o un write corto) no da ningun evento por ella, y ReadDelegations trae
// todas las anteriores exactas y en orden (una de ellas larga, escrita como
// hoom: su agent y su agent_end en lineas de casi 1 MiB, sin pasarse). Para un agent y un agent_end
// (este del tool_id de una delegacion anterior abierta, que no debe cerrar),
// de mas de 1 MiB y de menos, cortados en el detail, en medio de un caracter
// de varios bytes, justo antes de la llave final y apenas despues del agente.
func TestHallazgo_7d07ce_CA362_LineaAMedioEscribirNoSale(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 23, 0, 0, 0, time.UTC)
	m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
	anteriores := []Event{
		{TS: m(1), Kind: "agent", Agent: "Explore", ToolID: "toolu_1", Detail: "Agent: buscar precios"},
		dlgDeLinea(Event{TS: m(2), Kind: "agent", Agent: "Larga", ToolID: "toolu_LARGA", Detail: "Agent: "}, dlgMiB),
		{TS: m(3), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_1", Detail: "Explore termino"},
		dlgDeLinea(Event{TS: m(4), Kind: "agent_end", Agent: "Larga", ToolID: "toolu_LARGA", Detail: "Larga fallo: "}, dlgMiB),
		{TS: m(5), Kind: "agent", Agent: "Abierta", ToolID: "toolu_ABIERTA", Detail: "Agent: sigue trabajando"},
	}
	for _, e := range anteriores {
		if _, mayor := dlgLargoJSON(t, e); mayor > dlgMiB {
			t.Fatalf("fixture: la delegacion %s esta escrita como hoom: linea de hasta 1 MiB (%d)", dlgResumen(e), mayor)
		}
	}
	if menor, _ := dlgLargoJSON(t, anteriores[1]); menor < dlgMiB-3 {
		t.Fatalf("fixture: la delegacion larga mide casi 1 MiB: %d bytes", menor)
	}
	type corte struct {
		nombre string
		largo  int                    // largo de la linea entera, sin el salto
		hasta  func(linea string) int // donde se corta
	}
	cortes := []corte{
		{"linea de 3 MiB cortada a los 2 MiB", 3 * dlgMiB, func(string) int { return 2 * dlgMiB }},
		{"linea de 1.5 MiB cortada en 1 MiB + 10", 3 * dlgMiB / 2, func(string) int { return dlgMiB + 10 }},
		{"linea de 1.5 MiB cortada justo antes de la llave final", 3 * dlgMiB / 2, func(l string) int { return len(l) - 1 }},
		{"linea de 500 KiB cortada a los 400 KiB", 500 << 10, func(string) int { return 400 << 10 }},
		{"linea de 500 KiB cortada en medio de un caracter", 500 << 10, func(l string) int { return strings.Index(l, "🙂") + 2 }},
		{"linea de 300 KiB cortada justo antes de la llave final", 300 << 10, func(l string) int { return len(l) - 1 }},
		{"linea cortada apenas despues del agente", 300 << 10, func(l string) int { return strings.Index(l, `,"detail"`) }},
	}
	for _, kind := range []string{"agent", "agent_end"} {
		for _, c := range cortes {
			nombre := kind + ", " + c.nombre
			t.Run(nombre, func(t *testing.T) {
				ca := "CA-362 (hallazgo 7d07ce, " + nombre + ")"
				agente, tool := "Cortada", "toolu_CORTADA"
				if kind == "agent_end" {
					agente, tool = "Abierta", "toolu_ABIERTA"
				}
				linea := c4v6LineaDeLargo(t, c.largo, c4v6OrdenHoom, m(6), kind, agente, tool)
				hasta := c.hasta(linea)
				if hasta <= 0 || hasta >= len(linea) {
					t.Fatalf("fixture: el corte cae dentro de la linea: %d de %d", hasta, len(linea))
				}
				var lineas []string
				lineas = append(lineas, c4v6JSON(t, Event{TS: m(0), Kind: "start", Detail: "init"}))
				for _, e := range anteriores {
					lineas = append(lineas, c4v6JSON(t, e))
				}
				lineas = append(lineas, linea[:hasta])
				root := t.TempDir()
				const id = "20261019T230000_cortad"
				c4v6Log(t, root, id, true, lineas...)
				if ref := c4v6Filtradas(t, root, id); !c4v6Iguales(ref, anteriores) {
					t.Fatalf("fixture: ReadEvents salta la linea a medio escribir y lee las anteriores:\n%s", dlgListado(ref))
				}
				ds, err := ReadDelegations(root, id)
				if err != nil {
					t.Fatalf("%s: ReadDelegations lee el log: %v", ca, err)
				}
				if !c4v6Iguales(ds, anteriores) {
					t.Errorf("%s: la linea a medio escribir del final no da ningun evento (ni un subagente fantasma ni el cierre de %s), y las %d anteriores salen exactas y en orden; trajo %d:\n%squiere:\n%s",
						ca, tool, len(anteriores), len(ds), dlgListado(ds), dlgListado(anteriores))
				}
			})
		}
	}
}
