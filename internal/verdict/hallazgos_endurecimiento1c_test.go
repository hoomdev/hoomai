// Tests de regresion de la tercera ronda de la tarea endurecimiento-1 en
// verdict, sobre .hoom/specs/evidencia-en-disco.md (CA-442) y
// .hoom/specs/historia-doctor-y-cinta.md (CA-369). Escritos desde el texto de
// cada hallazgo y el contrato de la correccion, SIN LEER LA IMPLEMENTACION.
// Los helpers de este archivo llevan el prefijo e1c.
//
//   - 20261023T122536_c85840 (high), 20261023T123049_8c7270 y
//     20261023T123540_b7bb41: el lector comun de la evidencia retenia en
//     memoria el contenido de TODOS los veredictos antes de parsear el
//     primero; con un tope de 256 MiB por archivo, N veredictos validos
//     retenian N x el tamano. LoadAllWithWarnings ahora procesa de a uno: la
//     memoria viva maxima queda en el orden de un archivo, no de N.
//   - 20261023T123057_b36303 (low): el lector entregaba el orden del
//     directorio, no el de nombre. Entre veredictos con el mismo CreatedAt,
//     LoadAllWithWarnings los devuelve en orden de nombre, sin depender del
//     orden en que el sistema de archivos lista .hoom/verdicts.
package verdict_test

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

	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	e1cMiB = 1 << 20
	// e1cTam y e1cN: 48 veredictos validos de 4 MiB (192 MiB en total).
	e1cTam = 4 * e1cMiB
	e1cN   = 48
	// e1cTope es lo mas que puede crecer la memoria viva mientras se leen:
	// 12 archivos (un cuarto del total). Procesar de a uno deja vivos uno o
	// dos (el que se lee y el buffer que crece); retenerlos todos, 192 MiB.
	e1cTope = 48 * e1cMiB
)

// e1cRellenar agrega espacios al final de p hasta que pese tam bytes: el
// JSON sigue siendo el mismo y valido (espacio en blanco al final).
func e1cRellenar(t *testing.T, p string, tam int) {
	t.Helper()
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if int(fi.Size()) >= tam {
		t.Fatalf("fixture: %s ya pesa %d bytes (>= %d)", p, fi.Size(), tam)
	}
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(bytes.Repeat([]byte(" "), tam-int(fi.Size()))); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, fi.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
}

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

// e1cVeredictos escribe n veredictos validos con CreatedAt = cuando y
// devuelve sus rutas.
func e1cVeredictos(t *testing.T, root string, n int, cuando time.Time) []string {
	t.Helper()
	var out []string
	for i := 0; i < n; i++ {
		v := &verdict.Verdict{Schema: verdict.SchemaID, CreatedAt: cuando, Project: "demo", Verdict: "red",
			Notes: []string{fmt.Sprintf("e1c veredicto %03d", i)}}
		p, err := verdict.Write(root, v)
		if err != nil {
			t.Fatalf("fixture: verdict.Write: %v", err)
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		out = append(out, p)
	}
	return out
}

// CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41): con 48 veredictos
// validos de 4 MiB cada uno (192 MiB; el tope por archivo es 256 MiB),
// LoadAllWithWarnings los trae todos y, mientras los lee, la memoria viva no
// crece mas de 48 MiB: lo que retiene queda en el orden de un archivo, no de
// N. Retener el contenido de todos antes de parsear el primero la sube a
// ~192 MiB. Guarda: el medidor ve esa retencion cuando la hay.
func TestHallazgo_c85840_8c7270_b7bb41_CA442_LoadAllNoRetieneTodosLosVeredictosALaVez(t *testing.T) {
	const ca = "CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41, verdict.LoadAllWithWarnings)"
	root := t.TempDir()
	rutas := e1cVeredictos(t, root, e1cN, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))
	for _, p := range rutas {
		e1cRellenar(t, p, e1cTam)
	}
	e1cGuardaMedidor(t, ca, rutas, e1cN*e1cTam)

	var vs []*verdict.Verdict
	var err error
	pico, muestras := e1cPicoVivo(func() { vs, _, err = verdict.LoadAllWithWarnings(root) })
	t.Logf("%s: %d veredictos de %d MiB: la memoria viva crecio %.1f MiB (%d muestras)", ca, e1cN, e1cTam/e1cMiB, float64(pico)/e1cMiB, muestras)
	if err != nil || len(vs) != e1cN {
		t.Fatalf("%s: fixture: LoadAllWithWarnings trae los %d veredictos validos (espacios al final): %d, err %v", ca, e1cN, len(vs), err)
	}
	if pico > e1cTope {
		t.Errorf("%s: con %d veredictos de %d MiB, LoadAllWithWarnings no retiene el contenido de todos a la vez: la memoria viva crecio %.1f MiB (tope %d MiB, el total es %d MiB)",
			ca, e1cN, e1cTam/e1cMiB, float64(pico)/e1cMiB, e1cTope/e1cMiB, e1cN*e1cTam/e1cMiB)
	}
}

// CA-442 (hallazgo b36303): 10 veredictos con el MISMO CreatedAt (los
// nombres difieren en el sufijo hex) salen de LoadAllWithWarnings en orden
// de nombre, no en el que el sistema de archivos lista .hoom/verdicts (en
// APFS, el de un hash del nombre). Diez a proposito: con hasta 12 empates
// cualquier sort de Go deja los empates como vienen. Guarda: si el
// directorio ya se lista en orden de nombre el caso no distingue (Skip).
func TestHallazgo_b36303_CA442_LoadAllConEmpatesEnOrdenDeNombre(t *testing.T) {
	const ca = "CA-442 (hallazgo b36303, verdict.LoadAllWithWarnings con empates)"
	root := t.TempDir()
	rutas := e1cVeredictos(t, root, 10, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC))
	var quiero []string
	for _, p := range rutas {
		quiero = append(quiero, strings.TrimSuffix(filepath.Base(p), ".json"))
	}
	sort.Strings(quiero)

	f, err := os.Open(filepath.Join(root, ".hoom", "verdicts"))
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := f.Readdirnames(-1)
	f.Close()
	if sort.StringsAreSorted(crudo) {
		t.Skipf("%s: este sistema de archivos ya lista .hoom/verdicts en orden de nombre: el caso no distingue", ca)
	}

	vs, _, err := verdict.LoadAllWithWarnings(root)
	if err != nil || len(vs) != len(quiero) {
		t.Fatalf("%s: fixture: LoadAllWithWarnings trae los %d veredictos: %d, err %v", ca, len(quiero), len(vs), err)
	}
	var got []string
	for _, v := range vs {
		got = append(got, v.ID)
	}
	if strings.Join(got, ",") != strings.Join(quiero, ",") {
		t.Errorf("%s: entre veredictos con el mismo CreatedAt el orden es el de nombre, determinista, no el del directorio:\n got %v\nquiere %v\n  dir %v",
			ca, got, quiero, crudo)
	}
}
