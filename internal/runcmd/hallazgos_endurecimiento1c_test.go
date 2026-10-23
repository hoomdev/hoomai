// Tests de regresion de la tercera ronda de la tarea endurecimiento-1 en
// runcmd, sobre .hoom/specs/evidencia-en-disco.md (CA-442) y
// .hoom/specs/historia-doctor-y-cinta.md (CA-369). Escritos desde el texto de
// cada hallazgo y el contrato de la correccion, SIN LEER LA IMPLEMENTACION.
// Los helpers de este archivo llevan el prefijo e1c.
//
//   - 20261023T122536_c85840 (high), 20261023T123049_8c7270 y
//     20261023T123540_b7bb41: el lector comun de la evidencia y la
//     telemetria retenia en memoria el contenido de TODOS los archivos antes
//     de parsear el primero. MetasErr ahora procesa los sidecars de
//     .hoom/runs de a uno: la memoria viva maxima queda en el orden de un
//     archivo, no de N.
//   - 20261023T123057_b36303 (low): el lector entregaba el orden del
//     directorio, no el de nombre. Entre metas con el mismo CreatedAt,
//     MetasErr los devuelve en orden de nombre (creciente o decreciente:
//     "del mas nuevo al mas viejo" no dice como desempatar), nunca en el del
//     directorio.
package runcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	e1cMiB = 1 << 20
	// e1cTam y e1cN: 192 registros validos de casi 1 MiB (el tope por
	// archivo de esta telemetria es 1 MiB): ~191 MiB en total.
	e1cTam = e1cMiB - 4<<10
	e1cN   = 192
	// e1cTope es lo mas que puede crecer la memoria viva mientras se leen:
	// un cuarto del total. Procesar de a uno deja vivos uno o dos; retenerlos
	// todos, ~191 MiB.
	e1cTope = 48 * e1cMiB
)

// e1cPicoVivo corre f y devuelve cuanto crecio, como maximo, la memoria viva
// del heap mientras f corria, y cuantas muestras tomo: otra goroutine fuerza
// un GC completo una y otra vez y anota el HeapAlloc que queda despues de
// cada uno (lo vivo, mas lo poco que f asigno entre el GC y la lectura).
func e1cPicoVivo(f func()) (pico uint64, muestras int) {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	base := m.HeapAlloc
	var max atomic.Uint64
	var n atomic.Int64
	parar, listo := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(listo)
		var m runtime.MemStats
		for {
			runtime.GC()
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > max.Load() {
				max.Store(m.HeapAlloc)
			}
			n.Add(1)
			select {
			case <-parar:
				return
			default:
			}
		}
	}()
	f()
	close(parar)
	<-listo
	if v := max.Load(); v > base {
		pico = v - base
	}
	return pico, int(n.Load())
}

// e1cGuardaMedidor calibra el medidor en las dos direcciones, con los mismos
// archivos: leerlos y retenerlos todos juntos hace crecer la memoria viva
// por lo menos a la mitad del total; leerlos de a uno (os.ReadFile y
// json.Unmarshal, soltando cada uno) la deja por debajo de la mitad de
// e1cTope. Si no, el test no puede decir nada (fixture).
func e1cGuardaMedidor(t *testing.T, ca string, archivos []string, total int) {
	t.Helper()
	var todos [][]byte
	pico, muestras := e1cPicoVivo(func() {
		for _, p := range archivos {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Errorf("fixture: %v", err)
				return
			}
			todos = append(todos, b)
		}
		runtime.KeepAlive(todos)
	})
	todos = nil
	if pico < uint64(total/2) {
		t.Fatalf("%s: fixture: el medidor no ve la retencion: leer y retener los %d archivos (%d MiB) solo subio la memoria viva %.1f MiB (%d muestras)",
			ca, len(archivos), total/e1cMiB, float64(pico)/e1cMiB, muestras)
	}
	pico, muestras = e1cPicoVivo(func() {
		for _, p := range archivos {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Errorf("fixture: %v", err)
				return
			}
			var x map[string]any
			if err := json.Unmarshal(b, &x); err != nil {
				t.Errorf("fixture: %s no es JSON valido: %v", p, err)
				return
			}
		}
	})
	if pico > e1cTope/2 {
		t.Fatalf("%s: fixture: el medidor ve retencion donde no la hay: leer de a uno los %d archivos subio la memoria viva %.1f MiB (mas de %d MiB, %d muestras)",
			ca, len(archivos), float64(pico)/e1cMiB, e1cTope/2/e1cMiB, muestras)
	}
	t.Logf("%s: calibracion: de a uno, la memoria viva crecio %.1f MiB (%d muestras)", ca, float64(pico)/e1cMiB, muestras)
}

// e1cMeta escribe .hoom/runs/<id>.meta.json valido con ese id y CreatedAt,
// relleno con espacios al final hasta tam bytes (0: sin relleno), y devuelve
// la ruta.
func e1cMeta(t *testing.T, root, id string, cuando time.Time, tam int) string {
	t.Helper()
	raw, err := json.MarshalIndent(Meta{ID: id, Provider: "claude", Role: "writer", Task: "endurecimiento-1",
		Dir: root, CreatedAt: cuando, Status: "done"}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if tam > 0 {
		raw = append(raw, bytes.Repeat([]byte(" "), tam-len(raw))...)
	}
	p := filepath.Join(root, ".hoom", "runs", id+".meta.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// e1cOrdenDeNombre dice si got es quiero (ordenado) en orden creciente o
// decreciente.
func e1cOrdenDeNombre(got, quiero []string) bool {
	if strings.Join(got, ",") == strings.Join(quiero, ",") {
		return true
	}
	al := append([]string{}, quiero...)
	sort.Sort(sort.Reverse(sort.StringSlice(al)))
	return strings.Join(got, ",") == strings.Join(al, ",")
}

// CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41): con 192 sidecars de
// run validos de casi 1 MiB cada uno (~191 MiB; el tope por archivo es 1
// MiB), MetasErr los trae todos y, mientras los lee, la memoria viva no
// crece mas de 48 MiB: lo que retiene queda en el orden de un archivo, no de
// N. Retener el contenido de todos antes de parsear el primero la sube a
// ~191 MiB. Guarda: el medidor ve esa retencion cuando la hay, y no la ve
// leyendo de a uno.
func TestHallazgo_c85840_8c7270_b7bb41_CA442_MetasErrNoRetieneTodosLosSidecarsALaVez(t *testing.T) {
	const ca = "CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41, runcmd.MetasErr)"
	root := t.TempDir()
	var rutas []string
	for i := 0; i < e1cN; i++ {
		id := fmt.Sprintf("20261023T12%02d%02d_e1c%03x", i/60, i%60, i)
		rutas = append(rutas, e1cMeta(t, root, id, time.Date(2026, 10, 23, 12, 0, i, 0, time.UTC), e1cTam))
	}
	e1cGuardaMedidor(t, ca, rutas, e1cN*e1cTam)

	var ms []Meta
	var err error
	pico, muestras := e1cPicoVivo(func() { ms, err = MetasErr(root) })
	t.Logf("%s: %d sidecars de ~1 MiB: la memoria viva crecio %.1f MiB (%d muestras)", ca, e1cN, float64(pico)/e1cMiB, muestras)
	if err != nil || len(ms) != e1cN {
		t.Fatalf("%s: fixture: MetasErr trae los %d sidecars validos (espacios al final): %d, err %v", ca, e1cN, len(ms), err)
	}
	if pico > e1cTope {
		t.Errorf("%s: con %d sidecars de ~1 MiB, MetasErr no retiene el contenido de todos a la vez: la memoria viva crecio %.1f MiB (tope %d MiB, el total es %d MiB)",
			ca, e1cN, float64(pico)/e1cMiB, e1cTope/e1cMiB, e1cN*e1cTam/e1cMiB)
	}
}

// CA-442 (hallazgo b36303): 10 sidecars con el MISMO CreatedAt salen de
// MetasErr en orden de nombre (su id; creciente o decreciente), no en el que
// el sistema de archivos lista .hoom/runs (en APFS, el de un hash del
// nombre). Diez a proposito: con hasta 12 empates cualquier sort de Go deja
// los empates como vienen. Guarda: si el directorio ya se lista en orden de
// nombre (o al reves), Skip.
func TestHallazgo_b36303_CA442_MetasErrConEmpatesEnOrdenDeNombre(t *testing.T) {
	const ca = "CA-442 (hallazgo b36303, runcmd.MetasErr con empates)"
	root := t.TempDir()
	cuando := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	var quiero []string
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("20261023T120000_%06x", (i*0x9e3779+0x5bd1e9)&0xffffff)
		e1cMeta(t, root, id, cuando, 0)
		quiero = append(quiero, id)
	}
	sort.Strings(quiero)
	f, err := os.Open(filepath.Join(root, ".hoom", "runs"))
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := f.Readdirnames(-1)
	f.Close()
	var crudoIDs []string
	for _, n := range crudo {
		crudoIDs = append(crudoIDs, strings.TrimSuffix(n, ".meta.json"))
	}
	if e1cOrdenDeNombre(crudoIDs, quiero) {
		t.Skipf("%s: este sistema de archivos ya lista .hoom/runs en orden de nombre: el caso no distingue", ca)
	}

	ms, err := MetasErr(root)
	if err != nil || len(ms) != len(quiero) {
		t.Fatalf("%s: fixture: MetasErr trae los %d sidecars: %d, err %v", ca, len(quiero), len(ms), err)
	}
	var got []string
	for _, m := range ms {
		got = append(got, m.ID)
	}
	if !e1cOrdenDeNombre(got, quiero) {
		t.Errorf("%s: entre metas con el mismo CreatedAt el orden es el de nombre, determinista, no el del directorio:\n got %v\nquiere %v (o al reves)\n  dir %v",
			ca, got, quiero, crudoIDs)
	}
}
