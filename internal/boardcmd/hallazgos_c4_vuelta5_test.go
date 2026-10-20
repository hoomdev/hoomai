// Tests de regresion de la quinta vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en boardcmd: la historia de la
// tarjeta (CA-362). Escritos desde el spec y la descripcion de los
// hallazgos, sin leer la implementacion; repos git reales en t.TempDir() y
// la narracion del run escrita a mano. Los helpers compartidos (hi*, bd*)
// estan en historia_test.go, historia_telemetria_test.go y gather_test.go;
// los de este archivo llevan el prefijo c4v5.
//
//   - 0ebb3f y 34a37c: la historia trae una entrada subagente por cada
//     evento agent de los runs de la tarjeta, y cada agent_end emparejado por
//     tool_id la cierra (ended_at) con su subagente-fin. Lo que asigna leerla
//     no crece con el tamano de una linea enorme.
//
//     El contrato es el de la enmienda 5 del spec (decision de Henry, que
//     reemplaza a la 4): hoom no recorta las delegaciones, escribe cada una
//     tal como la arma el normalizador, y la historia lee entera cada linea de
//     delegacion de hasta 1 MiB y salta una mas larga. Una delegacion de mas
//     de 1 MiB, en un log nuevo o en uno de un hoom anterior, queda fuera de
//     contrato: se le exige que no tape a sus vecinas, que la memoria quede
//     acotada y que nunca se publique distinta de lo que fue. Con la enmienda
//     3 este test exigia que una delegacion enorme escrita a mano saliera
//     igual que una normal (recortada al leerla); con la 4, la escrita como
//     hoom traia el detail recortado al escribirlo. Ahora la linea de mas de
//     1 MiB se salta sin tapar a las vecinas, y una delegacion escrita como
//     hoom (una linea de hasta 1 MiB, aca de exactamente 1 MiB) da su entrada
//     cerrada y su subagente-fin. No es debilitar: lo que hoom escribe se
//     exige tal cual sobre un run de verdad
//     (internal/runcmd/hallazgos_c4_vuelta6_test.go y _vuelta7_test.go, y
//     hallazgos_c4_vuelta7_test.go de este paquete).
package boardcmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

const c4v5MiB = 1 << 20

// c4v5Linea es una linea de la narracion: un evento como lo escribe
// json.Marshal, o una linea cruda cabeza + n bytes de relleno + cola.
type c4v5Linea struct {
	ev           *providers.Event
	cabeza, cola string
	n            int
}

func c4v5Ev(e providers.Event) c4v5Linea { return c4v5Linea{ev: &e} }

// c4v5Relleno es el relleno de n bytes de las lineas enormes.
func c4v5Relleno(n int) string { return strings.Repeat("relleno ", n/8+1)[:n] }

// c4v5DeUnMiB es la delegacion e con su detail alargado con c4v5Relleno
// (ASCII sin escapes) para que su linea, como la escribe json.Marshal, mida
// exactamente 1 MiB: una delegacion larga escrita como hoom, en el borde de
// lo que la historia lee entero.
func c4v5DeUnMiB(t *testing.T, e providers.Event) providers.Event {
	t.Helper()
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	e.Detail += c4v5Relleno(c4v5MiB - len(raw))
	if raw, _ := json.Marshal(e); len(raw) != c4v5MiB {
		t.Fatalf("fixture: la delegacion escrita como hoom mide exactamente 1 MiB: %d bytes", len(raw))
	}
	return e
}

// c4v5Delegacion es una delegacion escrita a mano como la escribe hoom (ts,
// kind, agent, detail, tool_id al final) con un detail enorme: prefijo + n
// bytes de relleno, en una linea de mas de 1 MiB.
func c4v5Delegacion(minuto int, kind, agente, tool, prefijo string, n int) c4v5Linea {
	ts := hiMin(minuto).UTC().Format(time.RFC3339Nano)
	return c4v5Linea{
		cabeza: `{"ts":"` + ts + `","kind":"` + kind + `","agent":"` + agente + `","detail":"` + prefijo,
		n:      n,
		cola:   `","tool_id":"` + tool + `"}`,
	}
}

// c4v5Log escribe la narracion del run id en root/.hoom/runs/<id>.jsonl sin
// armar las lineas enormes en memoria.
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
	bw := bufio.NewWriterSize(f, 1<<16)
	bloque := strings.Repeat("relleno ", 8192) // 64 KiB
	for _, l := range ls {
		if l.ev != nil {
			raw, err := json.Marshal(l.ev)
			if err != nil {
				t.Fatal(err)
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
	if err := bw.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

// c4v5Forma dice como esta escrita en la narracion una linea (el agent o el
// agent_end) de la delegacion a Enorme.
type c4v5Forma int

const (
	c4v5Normal   c4v5Forma = iota // de tamano normal
	c4v5AMano                     // escrita a mano con un detail enorme: la linea pasa de 1 MiB
	c4v5ComoHoom                  // escrita como hoom: una linea de hasta 1 MiB (aca de exactamente 1 MiB)
)

// c4v5Delegaciones es como esta escrita la delegacion a Enorme de la tarjeta.
type c4v5Delegaciones struct {
	agent, fin c4v5Forma
	finFallo   bool // el agent_end normal dice que fallo (el enorme siempre falla)
	n          int  // bytes del detail enorme
}

// c4v5Tarjeta arma la tarjeta "precios" con un run suyo (sidecar con la
// tarea) cuya narracion tiene: una delegacion normal a Explore que cierra,
// la delegacion a Enorme escrita segun d (agent en el minuto 12, agent_end
// en el 14) y una delegacion normal a general-purpose que no cierra.
func c4v5Tarjeta(t *testing.T, d c4v5Delegaciones) string {
	t.Helper()
	root := bdRepo(t, "")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0.Add(-24*time.Hour), "")
	bdRunDisco(t, root, runcmd.Meta{ID: hiR1, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
		CreatedAt: hiMin(10), EndedAt: hiMin(30), Status: runcmd.StatusDone})
	var agent, fin c4v5Linea
	switch d.agent {
	case c4v5AMano:
		agent = c4v5Delegacion(12, "agent", "Enorme", "toolu_ENORME", "Agent: ", d.n)
	case c4v5ComoHoom:
		agent = c4v5Ev(c4v5DeUnMiB(t, hiEv(12, "agent", "Enorme", "toolu_ENORME", "Agent: ")))
	default:
		agent = c4v5Ev(hiEv(12, "agent", "Enorme", "toolu_ENORME", "Agent: leer el catalogo"))
	}
	switch {
	case d.fin == c4v5AMano:
		fin = c4v5Delegacion(14, "agent_end", "Enorme", "toolu_ENORME", "Enorme fallo: ", d.n)
	case d.fin == c4v5ComoHoom:
		fin = c4v5Ev(c4v5DeUnMiB(t, hiEv(14, "agent_end", "Enorme", "toolu_ENORME", "Enorme fallo: ")))
	case d.finFallo:
		fin = c4v5Ev(hiEv(14, "agent_end", "Enorme", "toolu_ENORME", "Enorme fallo: se corto"))
	default:
		fin = c4v5Ev(hiEv(14, "agent_end", "Enorme", "toolu_ENORME", "Enorme termino"))
	}
	for _, l := range []c4v5Linea{agent, fin} {
		if l.ev == nil {
			continue
		}
		if raw, _ := json.Marshal(l.ev); len(raw) > c4v5MiB {
			t.Fatalf("fixture: la delegacion escrita como hoom va en una linea de hasta 1 MiB: %d bytes", len(raw))
		}
	}
	p := c4v5Log(t, root, hiR1,
		c4v5Ev(hiEv(10, "start", "", "", "init")),
		c4v5Ev(hiEv(11, "agent", "Explore", "toolu_1", "Agent: buscar precios")),
		agent,
		c4v5Ev(hiEv(13, "agent_end", "Explore", "toolu_1", "Explore termino")),
		fin,
		c4v5Ev(hiEv(15, "agent", "general-purpose", "toolu_3", "Agent: sin cierre")),
		c4v5Ev(hiEv(29, "end", "", "", "listo")),
	)
	enormes := 0
	for _, f := range []c4v5Forma{d.agent, d.fin} {
		if f == c4v5AMano {
			enormes += d.n
		}
	}
	if fi, err := os.Stat(p); err != nil || fi.Size() < int64(enormes) {
		t.Fatalf("fixture: la narracion del run pesa al menos %d bytes: %v %v", enormes, fi, err)
	}
	return root
}

// c4v5Exigir exige las delegaciones de la historia de c4v5Tarjeta(d): las
// entradas subagente de Explore, de Enorme si su agent no esta en una linea
// de mas de 1 MiB, y de general-purpose, en orden, con su ended_at (el ts de
// su agent_end, o null si no cerro: la de general-purpose, y la de Enorme si
// su agent_end esta en una linea de mas de 1 MiB); y las subagente-fin de
// Explore y, si su agent y su agent_end se leyeron, de Enorme, con el plain
// de CA-362 (con error: "<agente> fallo"). Una linea de delegacion de mas de
// 1 MiB se salta (enmienda 5): ni abre ni cierra, y no tapa a las vecinas.
func c4v5Exigir(t *testing.T, ca string, tl Timeline, d c4v5Delegaciones) {
	t.Helper()
	artefacto := ".hoom/runs/" + hiR1 + ".jsonl"
	fin := func(n int) *time.Time { x := hiMin(n); return &x }
	type subagente struct {
		at      int
		agente  string
		termino *time.Time
	}
	want := []subagente{{11, "Explore", fin(13)}}
	if d.agent != c4v5AMano {
		cierre := fin(14)
		if d.fin == c4v5AMano {
			cierre = nil
		}
		want = append(want, subagente{12, "Enorme", cierre})
	}
	want = append(want, subagente{15, "general-purpose", nil})
	sub := hiDeKind(tl, KindSubagente)
	var nombres []string
	for _, w := range want {
		nombres = append(nombres, w.agente)
	}
	if len(sub) != len(want) {
		t.Fatalf("%s: cada agent de hasta 1 MiB da una entrada subagente (%s); el de mas de 1 MiB se salta; hubo %d:\n%s",
			ca, strings.Join(nombres, ", "), len(sub), hiListado(tl))
	}
	for i, w := range want {
		e := sub[i]
		if !e.At.Equal(hiMin(w.at)) || e.Who != w.agente+" (claude)" || e.Plain != "el writer le paso trabajo a "+w.agente ||
			e.Source != SourceTelemetria || e.Artifact != artefacto || e.CostUSD != nil {
			t.Fatalf("%s: la entrada subagente %d es la de %s del minuto %d, de %s y sin costo; fue %s %q %q %s %q %v\n%s",
				ca, i, w.agente, w.at, artefacto, e.At.Format(time.RFC3339), e.Who, e.Plain, e.Source, e.Artifact, e.CostUSD, hiListado(tl))
		}
		switch {
		case w.termino == nil && e.EndedAt != nil:
			t.Fatalf("%s: la delegacion a %s no cerro (o su agent_end esta en una linea de mas de 1 MiB, que se salta): ended_at null, fue %s\n%s",
				ca, w.agente, e.EndedAt.Format(time.RFC3339), hiListado(tl))
		case w.termino != nil && (e.EndedAt == nil || !e.EndedAt.Equal(*w.termino)):
			t.Fatalf("%s: la delegacion a %s queda cerrada: su ended_at es el ts de su agent_end (%s), fue %v\n%s",
				ca, w.agente, w.termino.Format(time.RFC3339), e.EndedAt, hiListado(tl))
		}
	}
	type finEsp struct {
		at          int
		agente, txt string
	}
	fins := []finEsp{{13, "Explore", "Explore termino su parte"}}
	if d.agent != c4v5AMano && d.fin != c4v5AMano {
		txt := "Enorme termino su parte"
		if d.finFallo || d.fin != c4v5Normal {
			txt = "Enorme fallo"
		}
		fins = append(fins, finEsp{14, "Enorme", txt})
	}
	got := hiDeKind(tl, KindSubagenteFin)
	ok := len(got) == len(fins)
	for i := 0; ok && i < len(fins); i++ {
		ok = got[i].At.Equal(hiMin(fins[i].at)) && got[i].Plain == fins[i].txt && got[i].Who == fins[i].agente+" (claude)"
	}
	if !ok {
		t.Fatalf("%s: cada agent_end emparejado da su subagente-fin, en orden (%v); uno en una linea de mas de 1 MiB, o sin su agent, no da ninguna:\n%s",
			ca, fins, hiListado(tl))
	}
	for _, e := range got {
		if e.Source != SourceTelemetria || e.Artifact != artefacto || e.CostUSD != nil || e.EndedAt != nil {
			t.Fatalf("%s: la subagente-fin de %s sale del run, sin costo y con ended_at null: %+v", ca, e.Who, e)
		}
	}
}

func (f c4v5Forma) nombre(kind string, n int) string {
	switch f {
	case c4v5AMano:
		return fmt.Sprintf("%s de %d MiB escrito a mano", kind, n/c4v5MiB)
	case c4v5ComoHoom:
		return kind + " de 1 MiB escrito como hoom"
	}
	return kind + " normal"
}

// CA-362 (hallazgos 0ebb3f y 34a37c; enmienda 5): la delegacion a Enorme de
// la tarjeta, con su agent, su agent_end o los dos escritos a mano con un
// detail de 4 MiB y de 32 MiB (lineas de mas de 1 MiB, como las de un hoom
// anterior o las de un provider que manda un agente enorme), o escritos como
// hoom (lineas de exactamente 1 MiB):
//   - una linea de mas de 1 MiB se salta sin tapar a las vecinas: con el
//     agent escrito a mano no hay entrada subagente de Enorme (ni
//     subagente-fin, porque su agent_end queda sin su agent), y con el
//     agent_end escrito a mano la delegacion queda abierta;
//   - escritos como hoom, la historia es la misma que con una delegacion de
//     tamano normal: la entrada subagente de Enorme cerrada (ended_at = el ts
//     de su agent_end) y su subagente-fin "Enorme fallo";
//   - las delegaciones normales de alrededor no cambian.
//
// Las guardas (tamano normal) pasan con y sin el arreglo.
//
// El contrato viene de la enmienda 5 (decision de Henry, reemplaza a la 4):
// con la enmienda 3 este test exigia que la delegacion escrita a mano saliera
// recortada al leerla, y con la 4 la escrita como hoom traia el detail
// recortado al escribirlo. Ahora hoom no recorta: una linea de mas de 1 MiB
// queda fuera de contrato y se salta sin tapar a las vecinas, y una de hasta
// 1 MiB se lee entera. No es debilitar: lo que hoom escribe se exige tal cual
// sobre un run de verdad (internal/runcmd/hallazgos_c4_vuelta6_test.go y
// _vuelta7_test.go, y hallazgos_c4_vuelta7_test.go de este paquete, donde
// ademas se exige que un fallo nunca se publique como exito), y la linea de
// exactamente 1 MiB se exige leida y cerrada con su subagente-fin.
func TestHallazgo_0ebb3f_34a37c_CA362_HistoriaConDelegacionEnorme(t *testing.T) {
	var cs []c4v5Delegaciones
	cs = append(cs, c4v5Delegaciones{agent: c4v5Normal, fin: c4v5Normal}, c4v5Delegaciones{agent: c4v5Normal, fin: c4v5Normal, finFallo: true})
	for _, n := range []int{4 * c4v5MiB, 32 * c4v5MiB} {
		cs = append(cs,
			c4v5Delegaciones{agent: c4v5AMano, fin: c4v5Normal, n: n},
			c4v5Delegaciones{agent: c4v5Normal, fin: c4v5AMano, n: n},
			c4v5Delegaciones{agent: c4v5AMano, fin: c4v5AMano, n: n},
		)
	}
	// escritas como hoom, de exactamente 1 MiB: el tamano no depende de n
	cs = append(cs,
		c4v5Delegaciones{agent: c4v5ComoHoom, fin: c4v5ComoHoom},
		c4v5Delegaciones{agent: c4v5ComoHoom, fin: c4v5Normal},
		c4v5Delegaciones{agent: c4v5Normal, fin: c4v5ComoHoom},
	)
	for _, d := range cs {
		nombre := d.agent.nombre("agent", d.n) + ", " + d.fin.nombre("agent_end", d.n)
		if d.fin == c4v5Normal {
			nombre += map[bool]string{false: " (termino)", true: " (fallo)"}[d.finFallo]
		}
		if d.agent == c4v5Normal && d.fin == c4v5Normal {
			nombre += " (guarda)"
		}
		t.Run(nombre, func(t *testing.T) {
			ca := "CA-362 (hallazgos 0ebb3f y 34a37c, enmienda 5, " + nombre + ")"
			root := c4v5Tarjeta(t, d)
			c4v5Exigir(t, ca, hiTimeline(t, "CA-362", root), d)
		})
	}
}

// c4v5Asigna mide lo que asigna leer la historia (TotalAlloc): el minimo de
// tres lecturas, despues de un GC.
func c4v5Asigna(t *testing.T, root string) uint64 {
	t.Helper()
	mejor := uint64(math.MaxUint64)
	for i := 0; i < 3; i++ {
		runtime.GC()
		var antes, despues runtime.MemStats
		runtime.ReadMemStats(&antes)
		tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
		runtime.ReadMemStats(&despues)
		if err != nil {
			t.Fatalf("TimelineFor: %v", err)
		}
		runtime.KeepAlive(tl)
		if d := despues.TotalAlloc - antes.TotalAlloc; d < mejor {
			mejor = d
		}
	}
	return mejor
}

// CA-362 (hallazgos 0ebb3f y 34a37c, guarda de memoria; enmienda 5): saltar
// la linea de una delegacion enorme no hace que la historia la cargue
// entera (la memoria sigue acotada). Con la misma
// tarjeta salvo el tamano del agent y del agent_end enormes (4 MiB contra
// 32 MiB cada uno), lo que asigna leer la historia crece menos de 16 MiB;
// cargar las dos lineas enteras asigna al menos los 56 MiB de diferencia.
func TestHallazgo_0ebb3f_34a37c_CA362_HistoriaNoCargaLaDelegacionEntera(t *testing.T) {
	chico := c4v5Tarjeta(t, c4v5Delegaciones{agent: c4v5AMano, fin: c4v5AMano, n: 4 * c4v5MiB})
	grande := c4v5Tarjeta(t, c4v5Delegaciones{agent: c4v5AMano, fin: c4v5AMano, n: 32 * c4v5MiB})
	a4 := c4v5Asigna(t, chico)
	a32 := c4v5Asigna(t, grande)
	const tope = 16 * c4v5MiB
	if a32 > a4 && a32-a4 >= tope {
		t.Fatalf("CA-362 (hallazgos 0ebb3f y 34a37c): leer la historia con una delegacion de 32 MiB asigna %.1f MiB y con una de 4 MiB %.1f MiB: crece %.1f MiB con la linea (tope %d MiB)",
			float64(a32)/c4v5MiB, float64(a4)/c4v5MiB, float64(a32-a4)/c4v5MiB, tope/c4v5MiB)
	}
}
