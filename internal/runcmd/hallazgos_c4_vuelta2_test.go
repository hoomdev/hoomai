// Tests de regresion de la segunda vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd. Escritos desde el spec
// y la descripcion del hallazgo, sin leer la implementacion. Los helpers de
// este archivo llevan el prefijo c4v2.
//
//   - 20261016T213325_058fb1 (risk): el id del run se valida como nombre
//     simple, pero abrir .hoom/runs/<id>.jsonl sigue los enlaces simbolicos:
//     un <id>.jsonl de nombre valido que es un enlace a un log de otro arbol
//     se lee como si fuera la narracion del run. La narracion de un run es
//     .hoom/runs/<id>.jsonl (CA-362): lo que esta afuera de .hoom/runs no se
//     lee por ese camino; un enlace asi es un error y no se lee nada.
package runcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// c4v2Enlace crea el enlace simbolico enlace -> destino (destino tal cual:
// absoluto o relativo al directorio del enlace) y exige que resuelva a un
// archivo. En Windows os.Symlink pide privilegios: el caso es de los
// sistemas con enlaces simbolicos.
func c4v2Enlace(t *testing.T, destino, enlace string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	if err := os.MkdirAll(filepath.Dir(enlace), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, enlace); err != nil {
		t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
	}
	fi, err := os.Lstat(enlace)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("fixture: %s es un enlace simbolico: %v %v", enlace, fi, err)
	}
	if fi, err := os.Stat(enlace); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("fixture: el enlace %s resuelve a un archivo regular (%s): %v %v", enlace, destino, fi, err)
	}
}

// c4v2Ajenos es la narracion de otro arbol: delegaciones de verdad.
func c4v2Ajenos(t0 time.Time) []Event {
	return []Event{
		{TS: t0, Kind: "start", Detail: "init"},
		{TS: t0.Add(time.Minute), Kind: "agent", Agent: "espia-ajeno", ToolID: "toolu_AJ", Detail: "Agent: secreto del otro proyecto"},
		{TS: t0.Add(2 * time.Minute), Kind: "agent_end", Agent: "espia-ajeno", ToolID: "toolu_AJ", Detail: "espia-ajeno termino"},
		{TS: t0.Add(3 * time.Minute), Kind: "agent", Agent: "otro-espia", ToolID: "toolu_AJ2", Detail: "Agent: mas secretos"},
	}
}

// c4v2SeLee exige que el id se lea entero por los tres caminos: ReadEvents
// (todos los eventos), ReadEventsFrom desde 0 (todos, y el offset al final
// del archivo) y ReadDelegations (solo agent y agent_end).
func c4v2SeLee(t *testing.T, ca, root, id string, eventos, delegaciones int) {
	t.Helper()
	evs, err := ReadEvents(root, id)
	if err != nil || len(evs) != eventos {
		t.Fatalf("%s: ReadEvents(%q) lee los %d eventos: err=%v, %d eventos %+v", ca, id, eventos, err, len(evs), evs)
	}
	fi, err := os.Stat(filepath.Join(root, ".hoom", "runs", id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	evs, next, err := ReadEventsFrom(root, id, 0)
	if err != nil || len(evs) != eventos || next != fi.Size() {
		t.Fatalf("%s: ReadEventsFrom(%q, 0) lee los %d eventos y deja el offset en %d: err=%v, %d eventos, offset %d",
			ca, id, eventos, fi.Size(), err, len(evs), next)
	}
	ds, err := ReadDelegations(root, id)
	if err != nil || len(ds) != delegaciones {
		t.Fatalf("%s: ReadDelegations(%q) lee las %d delegaciones: err=%v, %d %+v", ca, id, delegaciones, err, len(ds), ds)
	}
	for _, d := range ds {
		if d.Kind != "agent" && d.Kind != "agent_end" {
			t.Fatalf("%s: ReadDelegations(%q) trae solo agent y agent_end: %+v", ca, id, d)
		}
	}
}

// CA-362 (hallazgo 058fb1): la narracion de un run es .hoom/runs/<id>.jsonl.
// Un <id>.jsonl de nombre valido que es un enlace simbolico a un .jsonl de
// otro directorio (el proyecto vecino por ruta absoluta o relativa, el mismo
// proyecto fuera de .hoom/runs, o una cadena de enlaces que pasa por otro de
// .hoom/runs) no se lee: ReadEvents, ReadEventsFrom y ReadDelegations con
// ese id devuelven un error y ningun evento, y ReadEventsFrom no avanza el
// offset. Guarda: un .jsonl REGULAR con el mismo contenido si se lee entero
// por los tres caminos (el contenido y el nombre no son el problema).
func TestHallazgo_058fb1_CA362_ReadEventsNoSigueEnlacesFueraDeRuns(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	runs := filepath.Join(root, ".hoom", "runs")
	t0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)

	// los logs ajenos: en el proyecto vecino y en el mismo proyecto, fuera de .hoom/runs
	vecino := filepath.Join(base, "otro", ".hoom", "runs", "ajeno.jsonl")
	c4Narracion(t, vecino, c4v2Ajenos(t0)...)
	notas := filepath.Join(root, ".hoom", "notas", "ajeno.jsonl")
	c4Narracion(t, notas, c4v2Ajenos(t0)...)

	// guarda: el mismo contenido en un .jsonl regular de .hoom/runs se lee
	c4Narracion(t, filepath.Join(runs, "20260921T091000_copia1.jsonl"), c4v2Ajenos(t0)...)
	c4v2SeLee(t, "CA-362 (hallazgo 058fb1, guarda: un .jsonl regular con el mismo contenido)", root, "20260921T091000_copia1", 4, 3)

	// un eslabon de la cadena: un enlace de .hoom/runs al proyecto vecino
	c4v2Enlace(t, vecino, filepath.Join(runs, "20260921T093000_puente.jsonl"))

	for _, cs := range []struct{ nombre, id, destino string }{
		{"enlace absoluto al proyecto vecino", "20260921T092000_enlac1", vecino},
		{"enlace relativo al proyecto vecino", "20260921T092000_enlac2", filepath.Join("..", "..", "..", "otro", ".hoom", "runs", "ajeno.jsonl")},
		{"enlace al mismo proyecto, fuera de .hoom/runs", "20260921T092000_enlac3", filepath.Join("..", "notas", "ajeno.jsonl")},
		{"cadena de enlaces que pasa por otro de .hoom/runs", "20260921T092000_enlac4", "20260921T093000_puente.jsonl"},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			c4v2Enlace(t, cs.destino, filepath.Join(runs, cs.id+".jsonl"))

			evs, err := ReadEvents(root, cs.id)
			if err == nil || len(evs) != 0 {
				t.Errorf("CA-362 (hallazgo 058fb1): ReadEvents(%q), cuyo .jsonl es un enlace a %s, es un error y no lee nada; fue err=%v y %d eventos: %+v",
					cs.id, cs.destino, err, len(evs), evs)
			}
			for _, desde := range []int64{0, 10} {
				evs, next, err := ReadEventsFrom(root, cs.id, desde)
				if err == nil || len(evs) != 0 || next != desde {
					t.Errorf("CA-362 (hallazgo 058fb1): ReadEventsFrom(%q, %d), cuyo .jsonl es un enlace a %s, es un error, no lee nada y no avanza el offset; fue err=%v, %d eventos, offset %d",
						cs.id, desde, cs.destino, err, len(evs), next)
				}
			}
			ds, err := ReadDelegations(root, cs.id)
			if err == nil || len(ds) != 0 {
				t.Errorf("CA-362 (hallazgo 058fb1): ReadDelegations(%q), cuyo .jsonl es un enlace a %s, es un error y no trae delegaciones; fue err=%v y %d: %+v",
					cs.id, cs.destino, err, len(ds), ds)
			}
		})
	}
}

// CA-362 (hallazgo 058fb1, guarda): la raiz del proyecto nombrada a traves
// de un enlace simbolico (como /tmp o /var en macOS) no es un enlace dentro
// de .hoom/runs: un .jsonl regular de .hoom/runs se sigue leyendo entero por
// los tres caminos, con la raiz por el enlace y por su ruta real.
func TestHallazgo_058fb1_CA362_Guarda_RaizPorEnlaceSigueLeyendo(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	t0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	c4Narracion(t, filepath.Join(root, ".hoom", "runs", "20260921T090000_propio.jsonl"),
		Event{TS: t0, Kind: "start", Detail: "init"},
		Event{TS: t0.Add(time.Minute), Kind: "agent", Agent: "hoom-scout", ToolID: "toolu_ok", Detail: "Agent: mapea"},
		Event{TS: t0.Add(2 * time.Minute), Kind: "agent_end", Agent: "hoom-scout", ToolID: "toolu_ok", Detail: "hoom-scout termino"},
		Event{TS: t0.Add(3 * time.Minute), Kind: "end", Detail: "listo"})
	enlace := filepath.Join(base, "enlace-al-proyecto")
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	if err := os.Symlink(root, enlace); err != nil {
		t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
	}
	c4v2SeLee(t, "CA-362 (hallazgo 058fb1, guarda: raiz real)", root, "20260921T090000_propio", 4, 2)
	c4v2SeLee(t, "CA-362 (hallazgo 058fb1, guarda: raiz por un enlace)", enlace, "20260921T090000_propio", 4, 2)
}
