// Tests de regresion de la tercera vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en runcmd. Escritos desde el spec
// y la descripcion del hallazgo, sin leer la implementacion. Los helpers de
// este archivo llevan el prefijo c4v3.
//
//   - 20261016T213325_058fb1 (risk), caso que la vuelta 2 no cubria: el
//     .jsonl del run es un archivo REGULAR, pero el directorio que lo
//     contiene llega por un enlace simbolico: .hoom/runs entero (o .hoom
//     entero) es un enlace al .hoom/runs (o al .hoom) de otro proyecto. La
//     narracion de un run es .hoom/runs/<id>.jsonl de la raiz (CA-362): lo
//     que el enlace trae de otro arbol no es la narracion de este proyecto;
//     leerlo por ese camino es un error y no se lee nada. La raiz misma (o un
//     directorio por encima de ella) nombrada a traves de un enlace, como el
//     TMPDIR de macOS bajo /var, no es eso: sus runs se siguen leyendo.
package runcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// c4v3EnlaceDir crea el enlace simbolico enlace -> destino (destino tal cual:
// absoluto o relativo al directorio del enlace) y exige que resuelva a un
// directorio. En Windows os.Symlink pide privilegios: el caso es de los
// sistemas con enlaces simbolicos.
func c4v3EnlaceDir(t *testing.T, destino, enlace string) {
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
	if fi, err := os.Stat(enlace); err != nil || !fi.IsDir() {
		t.Fatalf("fixture: el enlace %s resuelve a un directorio (%s): %v %v", enlace, destino, fi, err)
	}
}

// c4v3Ajenos es la narracion del proyecto vecino: delegaciones de verdad.
func c4v3Ajenos(t0 time.Time) []Event {
	return []Event{
		{TS: t0, Kind: "start", Detail: "init"},
		{TS: t0.Add(time.Minute), Kind: "agent", Agent: "espia-ajeno", ToolID: "toolu_AJ", Detail: "Agent: secreto del otro proyecto"},
		{TS: t0.Add(2 * time.Minute), Kind: "agent_end", Agent: "espia-ajeno", ToolID: "toolu_AJ", Detail: "espia-ajeno termino"},
		{TS: t0.Add(3 * time.Minute), Kind: "agent", Agent: "otro-espia", ToolID: "toolu_AJ2", Detail: "Agent: mas secretos"},
	}
}

// c4v3SeLee exige que el id se lea entero por los tres caminos: ReadEvents
// (todos los eventos), ReadEventsFrom desde 0 (todos, y el offset al final
// del archivo) y ReadDelegations (solo agent y agent_end).
func c4v3SeLee(t *testing.T, ca, root, id string, eventos, delegaciones int) {
	t.Helper()
	evs, err := ReadEvents(root, id)
	if err != nil || len(evs) != eventos {
		t.Fatalf("%s: ReadEvents(%q, %q) lee los %d eventos: err=%v, %d eventos %+v", ca, root, id, eventos, err, len(evs), evs)
	}
	fi, err := os.Stat(filepath.Join(root, ".hoom", "runs", id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	evs, next, err := ReadEventsFrom(root, id, 0)
	if err != nil || len(evs) != eventos || next != fi.Size() {
		t.Fatalf("%s: ReadEventsFrom(%q, %q, 0) lee los %d eventos y deja el offset en %d: err=%v, %d eventos, offset %d",
			ca, root, id, eventos, fi.Size(), err, len(evs), next)
	}
	ds, err := ReadDelegations(root, id)
	if err != nil || len(ds) != delegaciones {
		t.Fatalf("%s: ReadDelegations(%q, %q) lee las %d delegaciones: err=%v, %d %+v", ca, root, id, delegaciones, err, len(ds), ds)
	}
	for _, d := range ds {
		if d.Kind != "agent" && d.Kind != "agent_end" {
			t.Fatalf("%s: ReadDelegations(%q) trae solo agent y agent_end: %+v", ca, id, d)
		}
	}
}

// c4v3NoSeLee exige que el id no se lea por ningun camino: ReadEvents y
// ReadDelegations devuelven un error y nada, y ReadEventsFrom (desde 0 y
// desde mas adelante) un error, nada, y el offset donde estaba.
func c4v3NoSeLee(t *testing.T, ca, root, id, como string) {
	t.Helper()
	evs, err := ReadEvents(root, id)
	if err == nil || len(evs) != 0 {
		t.Errorf("%s: ReadEvents(%q), con %s, es un error y no lee nada; fue err=%v y %d eventos: %+v",
			ca, id, como, err, len(evs), evs)
	}
	for _, desde := range []int64{0, 10} {
		evs, next, err := ReadEventsFrom(root, id, desde)
		if err == nil || len(evs) != 0 || next != desde {
			t.Errorf("%s: ReadEventsFrom(%q, %d), con %s, es un error, no lee nada y no avanza el offset; fue err=%v, %d eventos, offset %d",
				ca, id, desde, como, err, len(evs), next)
		}
	}
	ds, err := ReadDelegations(root, id)
	if err == nil || len(ds) != 0 {
		t.Errorf("%s: ReadDelegations(%q), con %s, es un error y no trae delegaciones; fue err=%v y %d: %+v",
			ca, id, como, err, len(ds), ds)
	}
}

// CA-361, CA-362 (hallazgo 058fb1): la narracion de un run es
// .hoom/runs/<id>.jsonl de la raiz. Si .hoom/runs entero es un enlace
// simbolico al .hoom/runs del proyecto vecino (por ruta absoluta, por ruta
// relativa, o por una cadena de enlaces de directorio), el <id>.jsonl que se
// ve a traves de el es un archivo REGULAR del vecino, con delegaciones de
// verdad, y no se lee: ReadEvents, ReadEventsFrom (sin avanzar el offset) y
// ReadDelegations con ese id devuelven un error y ningun evento.
//
// Guarda en el mismo fixture: con .hoom/runs como directorio real y una
// copia del mismo .jsonl, el id se lee entero por los tres caminos (el
// contenido y el nombre no son el problema; el enlace es lo unico que
// cambia).
func TestHallazgo_058fb1_CA361_CA362_RunsEnlazadoAlVecinoNoSeLee(t *testing.T) {
	const id = "20260921T092000_vecin1"
	t0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	for _, cs := range []struct{ nombre, destino string }{
		{"enlace absoluto al .hoom/runs del vecino", ""}, // se completa con la ruta absoluta
		{"enlace relativo al .hoom/runs del vecino", filepath.Join("..", "..", "otro", ".hoom", "runs")},
		{"cadena de enlaces de directorio hasta el vecino", "puente-runs"},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(base, "proyecto")
			runs := filepath.Join(root, ".hoom", "runs")
			vecinoRuns := filepath.Join(base, "otro", ".hoom", "runs")
			c4Narracion(t, filepath.Join(vecinoRuns, id+".jsonl"), c4v3Ajenos(t0)...)

			// guarda: .hoom/runs real con una copia regular del mismo .jsonl
			c4Narracion(t, filepath.Join(runs, id+".jsonl"), c4v3Ajenos(t0)...)
			c4v3SeLee(t, "CA-362 (hallazgo 058fb1, guarda: .hoom/runs es un directorio real)", root, id, 4, 3)
			if err := os.RemoveAll(runs); err != nil {
				t.Fatal(err)
			}

			// el ataque: .hoom/runs entero es un enlace al del vecino
			destino := cs.destino
			switch destino {
			case "":
				destino = vecinoRuns
			case "puente-runs":
				// .hoom/runs -> puente-runs (en .hoom) -> el .hoom/runs del vecino
				c4v3EnlaceDir(t, vecinoRuns, filepath.Join(root, ".hoom", "puente-runs"))
			}
			c4v3EnlaceDir(t, destino, runs)
			if fi, err := os.Stat(filepath.Join(runs, id+".jsonl")); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("fixture: a traves del enlace, %s.jsonl es un archivo regular del vecino: %v %v", id, fi, err)
			}

			c4v3NoSeLee(t, "CA-362 (hallazgo 058fb1)", root, id, ".hoom/runs como enlace a "+destino)
		})
	}
}

// CA-361, CA-362 (hallazgo 058fb1): lo mismo un nivel mas arriba. Si .hoom
// entero es un enlace simbolico al .hoom del proyecto vecino (por ruta
// absoluta o relativa), .hoom/runs y el <id>.jsonl son un directorio y un
// archivo regulares del vecino, y el id no se lee por ninguno de los tres
// caminos. Guarda en el mismo fixture: con .hoom real y una copia del mismo
// .jsonl, se lee entero.
func TestHallazgo_058fb1_CA361_CA362_HoomEnlazadoAlVecinoNoSeLee(t *testing.T) {
	const id = "20260921T092000_vecin2"
	t0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	for _, cs := range []struct{ nombre, destino string }{
		{"enlace absoluto al .hoom del vecino", ""},
		{"enlace relativo al .hoom del vecino", filepath.Join("..", "otro", ".hoom")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(base, "proyecto")
			hoom := filepath.Join(root, ".hoom")
			vecinoHoom := filepath.Join(base, "otro", ".hoom")
			c4Narracion(t, filepath.Join(vecinoHoom, "runs", id+".jsonl"), c4v3Ajenos(t0)...)

			// guarda: .hoom real con una copia regular del mismo .jsonl
			c4Narracion(t, filepath.Join(hoom, "runs", id+".jsonl"), c4v3Ajenos(t0)...)
			c4v3SeLee(t, "CA-362 (hallazgo 058fb1, guarda: .hoom es un directorio real)", root, id, 4, 3)
			if err := os.RemoveAll(hoom); err != nil {
				t.Fatal(err)
			}

			destino := cs.destino
			if destino == "" {
				destino = vecinoHoom
			}
			c4v3EnlaceDir(t, destino, hoom)
			if fi, err := os.Lstat(filepath.Join(hoom, "runs")); err != nil || !fi.IsDir() {
				t.Fatalf("fixture: a traves del enlace, .hoom/runs es un directorio real del vecino: %v %v", fi, err)
			}
			if fi, err := os.Lstat(filepath.Join(hoom, "runs", id+".jsonl")); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("fixture: a traves del enlace, %s.jsonl es un archivo regular del vecino: %v %v", id, fi, err)
			}

			c4v3NoSeLee(t, "CA-362 (hallazgo 058fb1)", root, id, ".hoom como enlace a "+destino)
		})
	}
}

// CA-361, CA-362 (hallazgo 058fb1, guarda): la raiz nombrada a traves de un
// enlace simbolico no es un enlace dentro de la raiz. Con .hoom y .hoom/runs
// reales, el .jsonl regular de un run se sigue leyendo entero por los tres
// caminos con la raiz: tal como la da t.TempDir() (en macOS el TMPDIR vive
// bajo /var, que es un enlace a /private/var), por su ruta real, por un
// enlace a la raiz misma y por un enlace a un directorio por encima de ella.
func TestHallazgo_058fb1_CA361_CA362_Guarda_RaizPorEnlaceSigueLeyendo(t *testing.T) {
	const id = "20260921T090000_propio"
	t0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	crudo := t.TempDir() // sin resolver: en macOS, /var/folders/...
	real, err := filepath.EvalSymlinks(crudo)
	if err != nil {
		t.Fatal(err)
	}
	c4Narracion(t, filepath.Join(crudo, "proyecto", ".hoom", "runs", id+".jsonl"),
		Event{TS: t0, Kind: "start", Detail: "init"},
		Event{TS: t0.Add(time.Minute), Kind: "agent", Agent: "hoom-scout", ToolID: "toolu_ok", Detail: "Agent: mapea"},
		Event{TS: t0.Add(2 * time.Minute), Kind: "agent_end", Agent: "hoom-scout", ToolID: "toolu_ok", Detail: "hoom-scout termino"},
		Event{TS: t0.Add(3 * time.Minute), Kind: "end", Detail: "listo"})

	c4v3SeLee(t, "CA-362 (hallazgo 058fb1, guarda: raiz como la da t.TempDir)", filepath.Join(crudo, "proyecto"), id, 4, 2)
	c4v3SeLee(t, "CA-362 (hallazgo 058fb1, guarda: raiz por su ruta real)", filepath.Join(real, "proyecto"), id, 4, 2)

	aRaiz := filepath.Join(real, "enlace-al-proyecto")
	c4v3EnlaceDir(t, filepath.Join(real, "proyecto"), aRaiz)
	c4v3SeLee(t, "CA-362 (hallazgo 058fb1, guarda: raiz por un enlace a ella)", aRaiz, id, 4, 2)

	aArriba := filepath.Join(real, "enlace-arriba")
	c4v3EnlaceDir(t, real, aArriba)
	c4v3SeLee(t, "CA-362 (hallazgo 058fb1, guarda: un enlace por encima de la raiz)", filepath.Join(aArriba, "proyecto"), id, 4, 2)
}
