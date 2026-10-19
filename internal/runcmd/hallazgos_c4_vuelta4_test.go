// Tests de regresion de la cuarta vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd. Escritos desde el spec
// y la descripcion del hallazgo, sin leer la implementacion. Los helpers de
// este archivo llevan el prefijo c4v4.
//
//   - 20261019T154837_2628ba (resilience): ReadDelegations lee el log linea
//     por linea para no cargarlo entero (d3c75f), pero una linea enorme que
//     es una delegacion (agent o agent_end con un detail de decenas de MiB),
//     o una linea valida de otro kind que lleva la marca "kind":"agent" en un
//     campo extra, se vuelve a cargar entera. La historia de la tarjeta
//     (CA-362) solo necesita las delegaciones: las de tamano normal, antes y
//     despues de esas lineas, salen todas y en orden, nada de la linea de
//     otro kind sale como delegacion, y lo que ReadDelegations asigna no
//     crece con el tamano de la linea enorme. Una delegacion de tamano normal
//     (unos cientos de bytes) siempre se lee entera.
//
//     Caso encontrado al escribir estos tests (caja negra): una delegacion
//     valida escrita con espacios o tabuladores entre las claves y los
//     valores ({"ts": "...", "kind": "agent", ...}) la lee ReadEvents y
//     ReadDelegations no. ReadDelegations es ReadEvents filtrado por agent y
//     agent_end, con cualquier JSON valido de una linea.
package runcmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

const c4v4MiB = 1 << 20

// c4v4TopeDetalle es lo mas que puede guardar ReadDelegations del detail de
// una linea enorme, si la trae: una delegacion no necesita mas para la
// historia, y 1 MiB ya es mucho mas que cualquier detail de tamano normal.
const c4v4TopeDetalle = c4v4MiB

// c4v4Linea es una linea del log: un evento tal como lo escribe json.Marshal
// (como lo escribe runcmd), o una linea escrita a mano en tres partes, cabeza
// + n bytes de relleno + cola, para no armar las enormes en memoria.
type c4v4Linea struct {
	ev           *Event
	cabeza, cola string
	n            int
}

func c4v4Ev(e Event) c4v4Linea { return c4v4Linea{ev: &e} }

// c4v4Volcar escribe las lineas en w, cada una con su salto de linea.
func c4v4Volcar(w io.Writer, ls ...c4v4Linea) error {
	bw := bufio.NewWriterSize(w, 1<<16)
	bloque := strings.Repeat("relleno ", 8192) // 64 KiB
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
		for resta := l.n; resta > 0; {
			k := min(resta, len(bloque))
			bw.WriteString(bloque[:k])
			resta -= k
		}
		bw.WriteString(l.cola)
		bw.WriteByte('\n')
	}
	return bw.Flush()
}

// c4v4Log escribe el log del run id en root/.hoom/runs/<id>.jsonl.
func c4v4Log(t *testing.T, root, id string, ls ...c4v4Linea) string {
	t.Helper()
	p := filepath.Join(root, ".hoom", "runs", id+".jsonl")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := c4v4Volcar(f, ls...); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func c4v4TS(ts time.Time) string { return ts.UTC().Format(time.RFC3339Nano) }

// Las tres lineas enormes. Las dos delegaciones tienen sus campos en el orden
// en que json.Marshal escribe un Event (el tool_id va despues del detail).

// c4v4AgentEnorme es una delegacion con un detail de n bytes.
func c4v4AgentEnorme(ts time.Time, n int) c4v4Linea {
	return c4v4Linea{
		cabeza: `{"ts":"` + c4v4TS(ts) + `","kind":"agent","agent":"Enorme","detail":"Agent: `,
		n:      n,
		cola:   `","tool_id":"toolu_ENORME"}`,
	}
}

// c4v4AgentEndEnorme es el cierre de esa delegacion con un detail de n bytes.
func c4v4AgentEndEnorme(ts time.Time, n int) c4v4Linea {
	return c4v4Linea{
		cabeza: `{"ts":"` + c4v4TS(ts) + `","kind":"agent_end","agent":"Enorme","detail":"Enorme fallo: `,
		n:      n,
		cola:   `","tool_id":"toolu_ENORME"}`,
	}
}

// c4v4TextoConMarca es una linea valida de kind text que lleva, al principio,
// la marca "kind":"agent" en un campo extra, y despues un detail de n bytes.
// No es una delegacion.
func c4v4TextoConMarca(ts time.Time, n int) c4v4Linea {
	return c4v4Linea{
		cabeza: `{"ts":"` + c4v4TS(ts) + `","kind":"text","extra":{"kind":"agent","agent":"Fantasma","tool_id":"toolu_FANTASMA",` +
			`"detail":"Agent: no soy una delegacion"},"detail":"`,
		n:    n,
		cola: `"}`,
	}
}

// c4v4Detalle arma un detail de unos cientos de bytes (al menos n), con
// acentos, comillas y barras que json escapa.
func c4v4Detalle(prefijo string, n int) string {
	frase := "paso de la delegación, con \"comillas\", \\barras\\ y ñandú; "
	var b strings.Builder
	b.WriteString(prefijo)
	for b.Len() < n {
		b.WriteString(frase)
	}
	return b.String()
}

// c4v4Normales son las delegaciones de tamano normal del log, en orden: una
// que abre antes de las lineas enormes y cierra entre ellas, una que abre y
// cierra entre ellas, y una que abre al final y no cierra.
func c4v4Normales(t0 time.Time) []Event {
	m := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Minute) }
	return []Event{
		{TS: m(1), Kind: "agent", Agent: "Explore", ToolID: "toolu_1", Detail: c4v4Detalle("Agent: antes de las lineas enormes; ", 300)},
		{TS: m(3), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_1", Detail: c4v4Detalle("Explore termino: ", 400)},
		{TS: m(5), Kind: "agent", Agent: "Plan", ToolID: "toolu_2", Detail: c4v4Detalle("Agent: entre las lineas enormes; ", 600)},
		{TS: m(7), Kind: "agent_end", Agent: "Plan", ToolID: "toolu_2", Detail: c4v4Detalle("Plan fallo: ", 250)},
		{TS: m(8), Kind: "agent", Agent: "general-purpose", ToolID: "toolu_3", Detail: c4v4Detalle("Agent: despues de las lineas enormes; ", 900)},
	}
}

// c4v4Enormes dice que lineas enormes lleva el log.
type c4v4Enormes struct{ agent, agentEnd, textoConMarca bool }

// c4v4LogConLineasEnormes escribe el log del run id: las delegaciones de
// c4v4Normales intercaladas con las lineas enormes pedidas (de n bytes de
// relleno cada una), mas un start, un text y un end normales.
func c4v4LogConLineasEnormes(t *testing.T, root, id string, n int, cuales c4v4Enormes) {
	t.Helper()
	t0 := time.Date(2026, 10, 19, 16, 0, 0, 0, time.UTC)
	m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
	d := c4v4Normales(t0)
	ls := []c4v4Linea{c4v4Ev(Event{TS: t0, Kind: "start", Detail: "init"}), c4v4Ev(d[0])}
	if cuales.agent {
		ls = append(ls, c4v4AgentEnorme(m(2), n))
	}
	ls = append(ls, c4v4Ev(d[1]))
	if cuales.textoConMarca {
		ls = append(ls, c4v4TextoConMarca(m(4), n))
	}
	ls = append(ls, c4v4Ev(d[2]))
	if cuales.agentEnd {
		ls = append(ls, c4v4AgentEndEnorme(m(6), n))
	}
	ls = append(ls, c4v4Ev(d[3]), c4v4Ev(d[4]),
		c4v4Ev(Event{TS: m(9), Kind: "text", Detail: "listo"}),
		c4v4Ev(Event{TS: m(10), Kind: "end", Detail: "listo"}))
	p := c4v4Log(t, root, id, ls...)
	cuantas := 0
	for _, b := range []bool{cuales.agent, cuales.agentEnd, cuales.textoConMarca} {
		if b {
			cuantas++
		}
	}
	if fi, err := os.Stat(p); err != nil || fi.Size() < int64(cuantas*n) {
		t.Fatalf("fixture: el log pesa al menos %d bytes: %v %v", cuantas*n, fi, err)
	}
}

// c4v4MismoEvento compara los campos de un evento de la narracion.
func c4v4MismoEvento(a, b Event) bool {
	return a.TS.Equal(b.TS) && a.Kind == b.Kind && a.Agent == b.Agent && a.ToolID == b.ToolID &&
		a.Detail == b.Detail && a.SessionID == b.SessionID && reflect.DeepEqual(a.Usage, b.Usage)
}

// c4v4Resumen describe un evento sin volcar un detail enorme.
func c4v4Resumen(e Event) string {
	d := e.Detail
	if len(d) > 60 {
		d = fmt.Sprintf("%.60s... (%d bytes)", d, len(e.Detail))
	}
	return fmt.Sprintf("%s %s agent=%q tool_id=%q detail=%q", c4v4TS(e.TS), e.Kind, e.Agent, e.ToolID, d)
}

func c4v4Listado(ds []Event) string {
	var b strings.Builder
	for _, d := range ds {
		b.WriteString("  " + c4v4Resumen(d) + "\n")
	}
	return b.String()
}

// c4v4ExigirDelegaciones exige lo que ReadDelegations trae de un log de
// c4v4LogConLineasEnormes: las delegaciones normales, todas, en orden y con
// sus campos exactos; nada de la linea text con la marca; y de las dos
// delegaciones enormes, cada una a lo sumo una vez y, si esta, con su kind,
// su agente, su tool_id y su ts (una delegacion sin su tool_id no se empareja
// con su cierre, CA-362), y a lo sumo c4v4TopeDetalle de su detail.
func c4v4ExigirDelegaciones(t *testing.T, ca string, ds []Event) {
	t.Helper()
	t0 := time.Date(2026, 10, 19, 16, 0, 0, 0, time.UTC)
	normales := c4v4Normales(t0)
	enormes := map[string]time.Time{"agent": t0.Add(2 * time.Minute), "agent_end": t0.Add(6 * time.Minute)}
	vistas := map[string]bool{}
	sig := 0
	for _, d := range ds {
		switch {
		case d.Agent == "Fantasma" || d.ToolID == "toolu_FANTASMA" || d.Kind == "text":
			t.Errorf("%s: la linea text con la marca \"kind\":\"agent\" en un campo extra no es una delegacion; salio %s\n%s",
				ca, c4v4Resumen(d), c4v4Listado(ds))
		case d.Agent == "Enorme":
			ts, ok := enormes[d.Kind]
			if !ok || vistas[d.Kind] || !d.TS.Equal(ts) || d.ToolID != "toolu_ENORME" {
				t.Errorf("%s: la delegacion enorme sale a lo sumo una vez y, si sale, con su kind, su ts y su tool_id toolu_ENORME; salio %s\n%s",
					ca, c4v4Resumen(d), c4v4Listado(ds))
			}
			vistas[d.Kind] = true
			if len(d.Detail) > c4v4TopeDetalle {
				t.Errorf("%s: de la delegacion enorme %s se guarda a lo sumo %d bytes del detail, no la linea: guardo %d bytes",
					ca, d.Kind, c4v4TopeDetalle, len(d.Detail))
			}
		case sig < len(normales) && c4v4MismoEvento(d, normales[sig]):
			sig++
		default:
			t.Errorf("%s: ReadDelegations trae las delegaciones normales en orden y con sus campos exactos; la siguiente era %s y salio %s\n%s",
				ca, c4v4Resumen(normales[min(sig, len(normales)-1)]), c4v4Resumen(d), c4v4Listado(ds))
		}
	}
	if sig != len(normales) {
		t.Errorf("%s: las %d delegaciones normales (antes, entre y despues de las lineas enormes) salen todas; salieron %d:\n%s",
			ca, len(normales), sig, c4v4Listado(ds))
	}
}

// CA-362 (hallazgo 2628ba): las lineas enormes escritas a mano son lineas de
// la narracion validas, como las que escribe runcmd: ReadEvents las entiende
// con su kind, su agente y su tool_id (con un relleno chico, en memoria).
func TestHallazgo_2628ba_CA362_Fixture_LasLineasEnormesSonValidas(t *testing.T) {
	ts := time.Date(2026, 10, 19, 16, 2, 0, 0, time.UTC)
	for _, cs := range []struct {
		nombre string
		l      c4v4Linea
		want   Event
	}{
		{"agent", c4v4AgentEnorme(ts, 16), Event{TS: ts, Kind: "agent", Agent: "Enorme", ToolID: "toolu_ENORME"}},
		{"agent_end", c4v4AgentEndEnorme(ts, 16), Event{TS: ts, Kind: "agent_end", Agent: "Enorme", ToolID: "toolu_ENORME"}},
		{"text con la marca", c4v4TextoConMarca(ts, 16), Event{TS: ts, Kind: "text"}},
	} {
		var buf bytes.Buffer
		if err := c4v4Volcar(&buf, cs.l); err != nil {
			t.Fatal(err)
		}
		var e Event
		if err := json.Unmarshal(buf.Bytes(), &e); err != nil {
			t.Fatalf("fixture: la linea %s es JSON valido: %v\n%s", cs.nombre, err, buf.String())
		}
		if !e.TS.Equal(cs.want.TS) || e.Kind != cs.want.Kind || e.Agent != cs.want.Agent || e.ToolID != cs.want.ToolID ||
			!strings.Contains(e.Detail, "relleno") {
			t.Fatalf("fixture: la linea %s es %+v, quiere %+v con el relleno en el detail", cs.nombre, e, cs.want)
		}
		if i := strings.Index(buf.String(), `"kind":"agent"`); cs.nombre == "text con la marca" && (i < 0 || i > 100) {
			t.Fatalf("fixture: la linea text lleva la marca \"kind\":\"agent\" al principio: %s", buf.String())
		}
		root := t.TempDir()
		c4v4Log(t, root, "20261019T160000_fixtur", cs.l)
		evs, err := ReadEvents(root, "20261019T160000_fixtur")
		if err != nil || len(evs) != 1 || !evs[0].TS.Equal(cs.want.TS) || evs[0].Kind != cs.want.Kind ||
			evs[0].Agent != cs.want.Agent || evs[0].ToolID != cs.want.ToolID {
			t.Fatalf("fixture: ReadEvents entiende la linea %s: err=%v %+v", cs.nombre, err, evs)
		}
	}
}

// CA-362 (hallazgo 2628ba): un log con una delegacion enorme (un agent y un
// agent_end con un detail de 4 MiB y de 32 MiB) y una linea text de ese
// tamano con la marca "kind":"agent" en un campo extra, intercaladas con
// delegaciones de unos cientos de bytes: ReadDelegations trae todas las
// delegaciones normales, en orden y con sus campos exactos; la linea text no
// sale como delegacion; y la delegacion enorme, si sale, sale una vez, con su
// kind, su agente, su tool_id y su ts, y sin guardar su detail entero.
func TestHallazgo_2628ba_CA362_DelegacionEnormeNoTapaLasNormales(t *testing.T) {
	for _, n := range []int{4 * c4v4MiB, 32 * c4v4MiB} {
		t.Run(fmt.Sprintf("lineas de %d MiB", n/c4v4MiB), func(t *testing.T) {
			root, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			const id = "20261019T160000_enorme"
			c4v4LogConLineasEnormes(t, root, id, n, c4v4Enormes{agent: true, agentEnd: true, textoConMarca: true})
			ds, err := ReadDelegations(root, id)
			if err != nil {
				t.Fatalf("CA-362 (hallazgo 2628ba): ReadDelegations lee un log con lineas de %d MiB sin error: %v", n/c4v4MiB, err)
			}
			c4v4ExigirDelegaciones(t, fmt.Sprintf("CA-362 (hallazgo 2628ba, lineas de %d MiB)", n/c4v4MiB), ds)
		})
	}
}

// c4v4Asigna mide lo que asigna ReadDelegations (TotalAlloc): el minimo de
// tres lecturas, despues de un GC, para que el ruido no cuente.
func c4v4Asigna(t *testing.T, root, id string) uint64 {
	t.Helper()
	mejor := uint64(math.MaxUint64)
	for i := 0; i < 3; i++ {
		runtime.GC()
		var antes, despues runtime.MemStats
		runtime.ReadMemStats(&antes)
		ds, err := ReadDelegations(root, id)
		runtime.ReadMemStats(&despues)
		if err != nil {
			t.Fatalf("ReadDelegations: %v", err)
		}
		runtime.KeepAlive(ds)
		if d := despues.TotalAlloc - antes.TotalAlloc; d < mejor {
			mejor = d
		}
	}
	return mejor
}

// CA-362 (hallazgo 2628ba): ReadDelegations no carga entera una linea enorme
// que es (o parece) una delegacion. Con el mismo log salvo el tamano de esa
// linea (4 MiB contra 32 MiB de detail), lo que asigna ReadDelegations no
// crece con la linea: la diferencia queda por debajo de 8 MiB. Cargar la
// linea entera asigna al menos los 28 MiB de diferencia. Un caso por linea:
// un agent enorme, un agent_end enorme y una linea text enorme con la marca
// "kind":"agent" en un campo extra.
func TestHallazgo_2628ba_CA362_ReadDelegationsNoCargaLaLineaEntera(t *testing.T) {
	const tope = 8 * c4v4MiB
	for _, cs := range []struct {
		nombre string
		cuales c4v4Enormes
	}{
		{"agent enorme", c4v4Enormes{agent: true}},
		{"agent_end enorme", c4v4Enormes{agentEnd: true}},
		{"text enorme con la marca en un campo extra", c4v4Enormes{textoConMarca: true}},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			const id = "20261019T160000_enorme"
			chico, grande := t.TempDir(), t.TempDir()
			c4v4LogConLineasEnormes(t, chico, id, 4*c4v4MiB, cs.cuales)
			c4v4LogConLineasEnormes(t, grande, id, 32*c4v4MiB, cs.cuales)
			a4 := c4v4Asigna(t, chico, id)
			a32 := c4v4Asigna(t, grande, id)
			if a32 > a4 && a32-a4 >= tope {
				t.Errorf("CA-362 (hallazgo 2628ba): con un %s de 32 MiB ReadDelegations asigna %.1f MiB y con uno de 4 MiB %.1f MiB: crece %.1f MiB con la linea (tope %d MiB); lo que guarda de una linea esta acotado",
					cs.nombre, float64(a32)/c4v4MiB, float64(a4)/c4v4MiB, float64(a32-a4)/c4v4MiB, tope/c4v4MiB)
			}
		})
	}
}

// c4v4Azar arma un texto de al menos n bytes (y a lo sumo unos pocos mas) con
// runas de varios tamanos y lo que json escapa: comillas, barras, saltos de
// linea, <, >, &, y la marca "kind":"agent" como texto.
func c4v4Azar(r *rand.Rand, n int) string {
	piezas := []string{"a", "Z", "7", " ", "-", "ñ", "é", "漢", "🙂", `"`, `\`, "\n", "\t", "<", ">", "&",
		`"kind":"agent"`, `{"kind":"agent_end"}`, "Agent: ", "toolu_"}
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(piezas[r.Intn(len(piezas))])
	}
	return b.String()
}

// CA-362 (hallazgo 2628ba, guarda, por propiedad): una delegacion de tamano
// normal siempre se lee entera. Para logs al azar como los escribe runcmd
// (agent, agent_end, text, tool, system, start y end, con details de 0 a 900
// bytes con acentos, emojis, comillas, barras, saltos de linea y la marca
// "kind":"agent" como texto, con y sin tool_id), mas lineas text chicas con
// la marca "kind":"agent" en un campo extra: ReadDelegations trae
// exactamente los agent y agent_end del log, en orden y con todos sus
// campos, y es lo mismo que ReadEvents filtrado por esos dos kinds.
func TestHallazgo_2628ba_CA362_Guarda_DelegacionNormalSiempreSeLee(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 19, 17, 0, 0, 0, time.UTC)
	kinds := []string{"agent", "agent_end", "agent", "agent_end", "text", "tool", "system", "start", "end", "marca"}
	agentes := []string{"Explore", "Plan", "general-purpose", "hoom-scout", "agente con espacios", "ñandú"}
	caso := 0
	prop := func(semilla int64) bool {
		caso++
		r := rand.New(rand.NewSource(semilla))
		id := fmt.Sprintf("20261019T170000_p%05d", caso)
		var ls []c4v4Linea
		var want []Event
		for i, total := 0, 1+r.Intn(40); i < total; i++ {
			ts := t0.Add(time.Duration(caso)*time.Hour + time.Duration(i)*time.Second + time.Duration(r.Intn(1000))*time.Millisecond)
			k := kinds[r.Intn(len(kinds))]
			detalle := c4v4Azar(r, r.Intn(900))
			if k == "marca" {
				raw, _ := json.Marshal(detalle)
				ls = append(ls, c4v4Linea{cabeza: `{"ts":"` + c4v4TS(ts) +
					`","kind":"text","extra":{"kind":"agent","agent":"Fantasma","tool_id":"toolu_FANTASMA"},"detail":` + string(raw) + `}`})
				continue
			}
			e := Event{TS: ts, Kind: k, Detail: detalle}
			if k == "agent" || k == "agent_end" {
				e.Agent = agentes[r.Intn(len(agentes))]
				if r.Intn(4) > 0 { // uno de cada cuatro, de un run viejo sin tool_id
					e.ToolID = fmt.Sprintf("toolu_%x", r.Int63())
				}
				want = append(want, e)
			}
			ls = append(ls, c4v4Ev(e))
		}
		c4v4Log(t, root, id, ls...)

		ds, err := ReadDelegations(root, id)
		ok := err == nil && len(ds) == len(want)
		for i := 0; ok && i < len(want); i++ {
			ok = c4v4MismoEvento(ds[i], want[i])
		}
		if !ok {
			t.Errorf("CA-362 (hallazgo 2628ba, guarda): con la semilla %d ReadDelegations trae las %d delegaciones de tamano normal del log, exactas y en orden; err=%v, trajo %d:\n%s\nquiere:\n%s",
				semilla, len(want), err, len(ds), c4v4Listado(ds), c4v4Listado(want))
			return false
		}
		evs, err := ReadEvents(root, id)
		if err != nil {
			t.Errorf("CA-362 (hallazgo 2628ba, guarda): con la semilla %d ReadEvents lee el log: %v", semilla, err)
			return false
		}
		var filtradas []Event
		for _, e := range evs {
			if e.Kind == "agent" || e.Kind == "agent_end" {
				filtradas = append(filtradas, e)
			}
		}
		ok = len(filtradas) == len(ds)
		for i := 0; ok && i < len(ds); i++ {
			ok = c4v4MismoEvento(ds[i], filtradas[i])
		}
		if !ok {
			t.Errorf("CA-362 (hallazgo 2628ba, guarda): con la semilla %d ReadDelegations es ReadEvents filtrado por agent y agent_end:\n%s\nReadEvents filtrado:\n%s",
				semilla, c4v4Listado(ds), c4v4Listado(filtradas))
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 150, Rand: rand.New(rand.NewSource(2628))}); err != nil {
		t.Fatalf("CA-362 (hallazgo 2628ba, guarda): %v", err)
	}
}

// CA-362 (hallazgo 2628ba, JSON espaciado): ReadDelegations trae exactamente
// lo mismo que ReadEvents filtrado por agent y agent_end, en orden, aunque las
// lineas del log sean JSON valido escrito de otra forma que la de
// json.Marshal: con espacios despues de los dos puntos y las comas, con
// espacios antes de los dos puntos, con un espacio al principio del objeto, y
// con retornos de carro (\r) y tabuladores entre las claves dentro de la
// linea. Mezcladas con delegaciones compactas, con lineas text que nombran
// "agent" y "kind":"agent" en el detail, y con una text espaciada que lleva
// "kind": "agent" en un campo extra (no es una delegacion).
func TestHallazgo_2628ba_CA362_ReadDelegationsEsReadEventsFiltradoConJSONEspaciado(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Date(2026, 10, 19, 18, 0, 0, 0, time.UTC)
	m := func(k int) time.Time { return t0.Add(time.Duration(k) * time.Minute) }
	crudo := func(s string) c4v4Linea { return c4v4Linea{cabeza: s} }
	const id = "20261019T180000_espaci"
	c4v4Log(t, root, id,
		c4v4Ev(Event{TS: m(0), Kind: "start", Detail: "init"}),
		crudo(`{"ts": "`+c4v4TS(m(1))+`", "kind": "agent", "agent": "Espaciado", "detail": "Agent: con espacios entre las claves", "tool_id": "toolu_S1"}`),
		c4v4Ev(Event{TS: m(2), Kind: "text", Detail: `el agent Espaciado dijo "kind":"agent" en su texto`}),
		crudo(`{"ts": "`+c4v4TS(m(3))+`", "kind": "text", "detail": "un agent con \"kind\": \"agent\" en el texto"}`),
		crudo(`{ "kind" : "agent_end" , "ts" : "`+c4v4TS(m(4))+`" , "agent" : "Espaciado" , "tool_id" : "toolu_S1" , "detail" : "Espaciado termino" }`),
		c4v4Ev(Event{TS: m(5), Kind: "agent", Agent: "Plan", ToolID: "toolu_2", Detail: "Agent: compacta"}),
		crudo("{\r\"ts\":\""+c4v4TS(m(6))+"\",\r\t\"kind\":\t\"agent\",\r\t\"agent\":\"Retorno\",\r\t\"tool_id\":\"toolu_R\"\r}"),
		c4v4Ev(Event{TS: m(7), Kind: "agent_end", Agent: "Plan", ToolID: "toolu_2", Detail: "Plan termino"}),
		crudo(`{"ts": "`+c4v4TS(m(8))+`", "kind": "text", "extra": {"kind": "agent", "agent": "Fantasma", "tool_id": "toolu_F"}, "detail": "no soy una delegacion"}`),
		crudo(`{"ts":"`+c4v4TS(m(9))+`","kind" :"agent_end","agent":"Retorno","tool_id":"toolu_R","detail":"Retorno fallo: se corto"}`),
		c4v4Ev(Event{TS: m(10), Kind: "end", Detail: "listo"}),
	)
	want := []Event{
		{TS: m(1), Kind: "agent", Agent: "Espaciado", ToolID: "toolu_S1", Detail: "Agent: con espacios entre las claves"},
		{TS: m(4), Kind: "agent_end", Agent: "Espaciado", ToolID: "toolu_S1", Detail: "Espaciado termino"},
		{TS: m(5), Kind: "agent", Agent: "Plan", ToolID: "toolu_2", Detail: "Agent: compacta"},
		{TS: m(6), Kind: "agent", Agent: "Retorno", ToolID: "toolu_R"},
		{TS: m(7), Kind: "agent_end", Agent: "Plan", ToolID: "toolu_2", Detail: "Plan termino"},
		{TS: m(9), Kind: "agent_end", Agent: "Retorno", ToolID: "toolu_R", Detail: "Retorno fallo: se corto"},
	}
	mismas := func(a, b []Event) bool {
		if len(a) != len(b) {
			return false
		}
		for i := range a {
			if !c4v4MismoEvento(a[i], b[i]) {
				return false
			}
		}
		return true
	}

	evs, err := ReadEvents(root, id)
	if err != nil {
		t.Fatalf("fixture: ReadEvents lee el log: %v", err)
	}
	var filtradas []Event
	for _, e := range evs {
		if e.Kind == "agent" || e.Kind == "agent_end" {
			filtradas = append(filtradas, e)
		}
	}
	if !mismas(filtradas, want) {
		t.Fatalf("fixture: ReadEvents entiende las %d delegaciones del log, compactas y espaciadas:\n%squiere:\n%s",
			len(want), c4v4Listado(filtradas), c4v4Listado(want))
	}

	ds, err := ReadDelegations(root, id)
	if err != nil || !mismas(ds, filtradas) {
		t.Errorf("CA-362 (hallazgo 2628ba): ReadDelegations trae exactamente ReadEvents filtrado por agent y agent_end, en orden, tambien con JSON espaciado; err=%v, trajo %d:\n%sReadEvents filtrado (%d):\n%s",
			err, len(ds), c4v4Listado(ds), len(filtradas), c4v4Listado(filtradas))
	}
}
