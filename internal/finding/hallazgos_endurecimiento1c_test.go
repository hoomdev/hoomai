// Tests de regresion de la tercera ronda de la tarea endurecimiento-1 en
// finding, sobre .hoom/specs/evidencia-en-disco.md (CA-442) y
// .hoom/specs/historia-doctor-y-cinta.md (CA-369). Escritos desde el texto de
// cada hallazgo y el contrato de la correccion, SIN LEER LA IMPLEMENTACION.
// Reusan initRepo de finding_test.go; los helpers de este archivo llevan el
// prefijo e1c.
//
//   - 20261023T122536_c85840 (high), 20261023T123049_8c7270 y
//     20261023T123540_b7bb41: el lector comun de la evidencia retenia en
//     memoria el contenido de TODOS los hallazgos antes de parsear el
//     primero; con un tope de 256 MiB por archivo, N hallazgos validos
//     retenian N x el tamano. List y ListConIlegibles ahora procesan de a
//     uno: la memoria viva maxima queda en el orden de un archivo, no de N.
//   - 20261023T123057_b36303 (low): el lector entregaba el orden del
//     directorio, no el de nombre. Entre hallazgos con el mismo CreatedAt,
//     List los devuelve en orden de nombre (su id), sin depender del orden
//     en que el sistema de archivos lista .hoom/findings.
package finding

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
	// e1cTam y e1cN: 48 hallazgos validos de 4 MiB (192 MiB en total).
	e1cTam = 4 * e1cMiB
	e1cN   = 48
	// e1cTope es lo mas que puede crecer la memoria viva mientras se leen:
	// 12 archivos (un cuarto del total). Procesar de a uno deja vivos uno o
	// dos (el que se lee y el buffer que crece); retenerlos todos, 192 MiB.
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

// e1cHallazgo escribe en .hoom/findings/<id>.json un hallazgo abierto valido
// con ese id y CreatedAt, relleno con espacios al final hasta tam bytes (0:
// sin relleno), y devuelve la ruta.
func e1cHallazgo(t *testing.T, root, id string, cuando time.Time, tam int) string {
	t.Helper()
	raw, err := json.MarshalIndent(Finding{ID: id, CreatedAt: cuando, Severity: "low", Lens: "reliability",
		Description: "hallazgo " + id + " de la prueba e1c", Author: "e1c", Task: "endurecimiento-1"}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if tam > 0 {
		if len(raw) >= tam {
			t.Fatalf("fixture: el hallazgo ya pesa %d bytes", len(raw))
		}
		raw = append(raw, bytes.Repeat([]byte(" "), tam-len(raw))...)
	}
	p := filepath.Join(root, ".hoom", "findings", id+".json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if !EsNombre(filepath.Base(p)) {
		t.Fatalf("fixture: %s no tiene la forma del nombre de un hallazgo", filepath.Base(p))
	}
	return p
}

// CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41): con 48 hallazgos
// validos de 4 MiB cada uno (192 MiB; el tope por archivo es 256 MiB), List
// y ListConIlegibles los traen todos y, mientras los leen, la memoria viva
// no crece mas de 48 MiB: lo que retienen queda en el orden de un archivo,
// no de N. Retener el contenido de todos antes de parsear el primero la sube
// a ~192 MiB. Guarda: el medidor ve esa retencion cuando la hay, y no la ve
// leyendo de a uno.
func TestHallazgo_c85840_8c7270_b7bb41_CA442_ListNoRetieneTodosLosHallazgosALaVez(t *testing.T) {
	const ca = "CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41, finding.List)"
	root := initRepo(t)
	var rutas []string
	for i := 0; i < e1cN; i++ {
		id := fmt.Sprintf("20261023T12%02d%02d_e1c%03x", i/60, i%60, i)
		rutas = append(rutas, e1cHallazgo(t, root, id, time.Date(2026, 10, 23, 12, 0, i, 0, time.UTC), e1cTam))
	}
	e1cGuardaMedidor(t, ca, rutas, e1cN*e1cTam)

	for _, cs := range []struct {
		nombre string
		leer   func() (int, error)
	}{
		{"List", func() (int, error) { items, _, err := List(root, "main", false); return len(items), err }},
		{"ListConIlegibles", func() (int, error) {
			items, ilegibles, _, err := ListConIlegibles(root, "main", false)
			if len(ilegibles) > 0 && err == nil {
				err = fmt.Errorf("ilegibles: %v", ilegibles)
			}
			return len(items), err
		}},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			var n int
			var err error
			pico, muestras := e1cPicoVivo(func() { n, err = cs.leer() })
			t.Logf("%s: %s con %d hallazgos de %d MiB: la memoria viva crecio %.1f MiB (%d muestras)",
				ca, cs.nombre, e1cN, e1cTam/e1cMiB, float64(pico)/e1cMiB, muestras)
			if err != nil || n != e1cN {
				t.Fatalf("%s: fixture: %s trae los %d hallazgos validos (espacios al final): %d, err %v", ca, cs.nombre, e1cN, n, err)
			}
			if pico > e1cTope {
				t.Errorf("%s: con %d hallazgos de %d MiB, %s no retiene el contenido de todos a la vez: la memoria viva crecio %.1f MiB (tope %d MiB, el total es %d MiB)",
					ca, e1cN, e1cTam/e1cMiB, cs.nombre, float64(pico)/e1cMiB, e1cTope/e1cMiB, e1cN*e1cTam/e1cMiB)
			}
		})
	}
}

// CA-442 (hallazgo b36303): 10 hallazgos con el MISMO CreatedAt salen de
// List en orden de nombre (su id), no en el que el sistema de archivos lista
// .hoom/findings (en APFS, el de un hash del nombre). Diez a proposito: con
// hasta 12 empates cualquier sort de Go deja los empates como vienen.
// Guarda: si el directorio ya se lista en orden de nombre, Skip.
func TestHallazgo_b36303_CA442_ListConEmpatesEnOrdenDeNombre(t *testing.T) {
	const ca = "CA-442 (hallazgo b36303, finding.List con empates)"
	root := initRepo(t)
	cuando := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	var quiero []string
	for i := 0; i < 10; i++ {
		// sufijos hex en desorden: el orden de creacion no es el de nombre
		id := fmt.Sprintf("20261023T120000_%06x", (i*0x9e3779+0x5bd1e9)&0xffffff)
		e1cHallazgo(t, root, id, cuando, 0)
		quiero = append(quiero, id)
	}
	sort.Strings(quiero)
	f, err := os.Open(filepath.Join(root, ".hoom", "findings"))
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := f.Readdirnames(-1)
	f.Close()
	if sort.StringsAreSorted(crudo) {
		t.Skipf("%s: este sistema de archivos ya lista .hoom/findings en orden de nombre: el caso no distingue", ca)
	}

	items, _, err := List(root, "main", false)
	if err != nil || len(items) != len(quiero) {
		t.Fatalf("%s: fixture: List trae los %d hallazgos: %d, err %v", ca, len(quiero), len(items), err)
	}
	var got []string
	for _, it := range items {
		got = append(got, it.ID)
	}
	if strings.Join(got, ",") != strings.Join(quiero, ",") {
		t.Errorf("%s: entre hallazgos con el mismo CreatedAt el orden es el de nombre, determinista, no el del directorio:\n got %v\nquiere %v\n  dir %v",
			ca, got, quiero, crudo)
	}
}
