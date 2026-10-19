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
//     tool_id la cierra (ended_at) con su subagente-fin. Una delegacion cuya
//     linea pasa de 1 MiB (un agent con un detail enorme, y su agent_end,
//     tambien enorme en otra variante) desaparece de la historia o queda
//     abierta. Con una delegacion enorme la historia es la misma que con una
//     de tamano normal, y lo que asigna leerla no crece con la linea. Las
//     lineas enormes van escritas como las escribe hoom (kind antes que
//     detail), dentro del contrato de la enmienda 3 del spec.
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

// c4v5Delegacion es una delegacion escrita como la escribe hoom (ts, kind,
// agent, detail, tool_id al final). Con n > 0 el detail es prefijo + n bytes
// de relleno; con n == 0 es prefijo + cola, de tamano normal.
func c4v5Delegacion(minuto int, kind, agente, tool, prefijo, colaNormal string, n int) c4v5Linea {
	if n == 0 {
		return c4v5Ev(hiEv(minuto, kind, agente, tool, prefijo+colaNormal))
	}
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

// c4v5Tarjeta arma la tarjeta "precios" con un run suyo (sidecar con la
// tarea) cuya narracion tiene: una delegacion normal a Explore que cierra,
// la delegacion a Enorme (su agent con un detail de n bytes; su agent_end
// con "Enorme termino" o, si finFallo, "Enorme fallo: " y m bytes) y una
// delegacion normal a general-purpose que no cierra. n y m en 0: la
// delegacion a Enorme es de tamano normal.
func c4v5Tarjeta(t *testing.T, n int, finFallo bool, m int) string {
	t.Helper()
	root := bdRepo(t, "")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0.Add(-24*time.Hour), "")
	bdRunDisco(t, root, runcmd.Meta{ID: hiR1, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
		CreatedAt: hiMin(10), EndedAt: hiMin(30), Status: runcmd.StatusDone})
	fin := c4v5Ev(hiEv(14, "agent_end", "Enorme", "toolu_ENORME", "Enorme termino"))
	if finFallo {
		fin = c4v5Delegacion(14, "agent_end", "Enorme", "toolu_ENORME", "Enorme fallo: ", "se corto", m)
	}
	p := c4v5Log(t, root, hiR1,
		c4v5Ev(hiEv(10, "start", "", "", "init")),
		c4v5Ev(hiEv(11, "agent", "Explore", "toolu_1", "Agent: buscar precios")),
		c4v5Delegacion(12, "agent", "Enorme", "toolu_ENORME", "Agent: ", "leer el catalogo", n),
		c4v5Ev(hiEv(13, "agent_end", "Explore", "toolu_1", "Explore termino")),
		fin,
		c4v5Ev(hiEv(15, "agent", "general-purpose", "toolu_3", "Agent: sin cierre")),
		c4v5Ev(hiEv(29, "end", "", "", "listo")),
	)
	if fi, err := os.Stat(p); err != nil || fi.Size() < int64(n+m) {
		t.Fatalf("fixture: la narracion del run pesa al menos %d bytes: %v %v", n+m, fi, err)
	}
	return root
}

// c4v5Exigir exige las delegaciones de la historia de c4v5Tarjeta: las tres
// entradas subagente, en orden, con su ended_at (el ts de su agent_end, o
// null la que no cerro), y las dos subagente-fin, con el plain de CA-362
// (con error: "<agente> fallo").
func c4v5Exigir(t *testing.T, ca string, tl Timeline, finFallo bool) {
	t.Helper()
	artefacto := ".hoom/runs/" + hiR1 + ".jsonl"
	fin := func(n int) *time.Time { x := hiMin(n); return &x }
	sub := hiDeKind(tl, KindSubagente)
	want := []struct {
		at      int
		agente  string
		termino *time.Time
	}{
		{11, "Explore", fin(13)},
		{12, "Enorme", fin(14)},
		{15, "general-purpose", nil},
	}
	if len(sub) != len(want) {
		t.Fatalf("%s: cada agent del run da una entrada subagente (Explore, Enorme, general-purpose); hubo %d:\n%s", ca, len(sub), hiListado(tl))
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
			t.Fatalf("%s: la delegacion a %s no cerro: ended_at null, fue %s", ca, w.agente, e.EndedAt.Format(time.RFC3339))
		case w.termino != nil && (e.EndedAt == nil || !e.EndedAt.Equal(*w.termino)):
			t.Fatalf("%s: la delegacion a %s queda cerrada: su ended_at es el ts de su agent_end (%s), fue %v\n%s",
				ca, w.agente, w.termino.Format(time.RFC3339), e.EndedAt, hiListado(tl))
		}
	}
	plainEnorme := "Enorme termino su parte"
	if finFallo {
		plainEnorme = "Enorme fallo"
	}
	fins := hiDeKind(tl, KindSubagenteFin)
	if len(fins) != 2 || !fins[0].At.Equal(hiMin(13)) || fins[0].Plain != "Explore termino su parte" || fins[0].Who != "Explore (claude)" ||
		!fins[1].At.Equal(hiMin(14)) || fins[1].Plain != plainEnorme || fins[1].Who != "Enorme (claude)" {
		t.Fatalf("%s: cada agent_end emparejado da su subagente-fin (Explore termino su parte, %s), en orden:\n%s", ca, plainEnorme, hiListado(tl))
	}
	for _, e := range fins {
		if e.Source != SourceTelemetria || e.Artifact != artefacto || e.CostUSD != nil || e.EndedAt != nil {
			t.Fatalf("%s: la subagente-fin de %s sale del run, sin costo y con ended_at null: %+v", ca, e.Who, e)
		}
	}
}

// CA-362 (hallazgos 0ebb3f y 34a37c): con una delegacion cuyo agent trae un
// detail enorme (4 MiB y 32 MiB), cuyo agent_end es enorme (un fallo con un
// mensaje de 4 MiB y de 32 MiB), o las dos cosas, la historia de la tarjeta
// trae la entrada subagente de esa delegacion, cerrada (ended_at = el ts de
// su agent_end), y su subagente-fin, igual que con una delegacion de tamano
// normal; las delegaciones normales de alrededor no cambian. Las guardas
// (tamano normal) pasan con y sin el arreglo.
func TestHallazgo_0ebb3f_34a37c_CA362_HistoriaConDelegacionEnorme(t *testing.T) {
	for _, cs := range []struct {
		n, m     int // bytes de relleno del agent y del agent_end de Enorme (0: tamano normal)
		finFallo bool
	}{
		{0, 0, false}, {0, 0, true}, // guardas: la delegacion a Enorme es de tamano normal
		{4 * c4v5MiB, 0, false}, {32 * c4v5MiB, 0, false}, // el agent enorme desaparece de la historia
		{0, 4 * c4v5MiB, true}, {0, 32 * c4v5MiB, true}, // el agent_end enorme deja la delegacion abierta
		{4 * c4v5MiB, 4 * c4v5MiB, true}, {32 * c4v5MiB, 32 * c4v5MiB, true}, // los dos enormes
	} {
		agent, fin := "agent normal", "agent_end normal (termino)"
		if cs.n > 0 {
			agent = fmt.Sprintf("agent de %d MiB", cs.n/c4v5MiB)
		}
		if cs.finFallo {
			fin = "agent_end normal (fallo)"
		}
		if cs.m > 0 {
			fin = fmt.Sprintf("agent_end de %d MiB (fallo)", cs.m/c4v5MiB)
		}
		nombre := agent + ", " + fin
		if cs.n == 0 && cs.m == 0 {
			nombre += " (guarda)"
		}
		t.Run(nombre, func(t *testing.T) {
			ca := "CA-362 (hallazgos 0ebb3f y 34a37c, " + nombre + ")"
			root := c4v5Tarjeta(t, cs.n, cs.finFallo, cs.m)
			c4v5Exigir(t, ca, hiTimeline(t, "CA-362", root), cs.finFallo)
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

// CA-362 (hallazgos 0ebb3f y 34a37c, guarda de memoria): que la delegacion
// enorme salga no hace que la historia cargue su linea entera. Con la misma
// tarjeta salvo el tamano del agent y del agent_end enormes (4 MiB contra
// 32 MiB cada uno), lo que asigna leer la historia crece menos de 16 MiB;
// cargar las dos lineas enteras asigna al menos los 56 MiB de diferencia.
func TestHallazgo_0ebb3f_34a37c_CA362_HistoriaNoCargaLaDelegacionEntera(t *testing.T) {
	chico := c4v5Tarjeta(t, 4*c4v5MiB, true, 4*c4v5MiB)
	grande := c4v5Tarjeta(t, 32*c4v5MiB, true, 32*c4v5MiB)
	a4 := c4v5Asigna(t, chico)
	a32 := c4v5Asigna(t, grande)
	const tope = 16 * c4v5MiB
	if a32 > a4 && a32-a4 >= tope {
		t.Fatalf("CA-362 (hallazgos 0ebb3f y 34a37c): leer la historia con una delegacion de 32 MiB asigna %.1f MiB y con una de 4 MiB %.1f MiB: crece %.1f MiB con la linea (tope %d MiB)",
			float64(a32)/c4v5MiB, float64(a4)/c4v5MiB, float64(a32-a4)/c4v5MiB, tope/c4v5MiB)
	}
}
