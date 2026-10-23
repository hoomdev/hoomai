// Tests de regresion de la tercera ronda de la tarea endurecimiento-1 en
// reviewcmd, sobre .hoom/specs/evidencia-en-disco.md (CA-442) y
// .hoom/specs/historia-doctor-y-cinta.md (CA-369). Escritos desde el texto de
// cada hallazgo y el contrato de la correccion, SIN LEER LA IMPLEMENTACION.
// Los helpers de este archivo llevan el prefijo e1c. En este paquete ningun
// test corre en paralelo: lo que mide e1cPicoVivo es del lector.
//
//   - 20261023T122536_c85840 (high), 20261023T123049_8c7270 y
//     20261023T123540_b7bb41: el lector comun de la evidencia retenia en
//     memoria el contenido de TODOS los registros de review antes de parsear
//     el primero; con un tope de 256 MiB por archivo, N registros validos
//     retenian N x el tamano. RecordsErr (y Records) ahora procesan de a uno:
//     la memoria viva maxima queda en el orden de un archivo, no de N.
//   - 20261023T123057_b36303 (low): el lector entregaba el orden del
//     directorio, no el de nombre. Entre registros con el mismo CreatedAt,
//     RecordsErr los devuelve en orden de nombre (su id), sin depender del
//     orden en que el sistema de archivos lista .hoom/reviews.
package reviewcmd

import (
	"bytes"
	"encoding/json"
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
	// e1cTam y e1cN: 48 registros de review validos de 4 MiB (192 MiB en total).
	e1cTam = 4 * e1cMiB
	e1cN   = 48
	// e1cTope es lo mas que puede crecer la memoria viva mientras se leen:
	// 12 archivos (un cuarto del total). Procesar de a uno deja vivos uno o
	// dos (el que se lee y el buffer que crece); retenerlos todos, 192 MiB.
	e1cTope = 48 * e1cMiB
)

// e1cVueltas es cuantas veces, a lo sumo, se repite una medicion de
// memoria (e1cPicoMinimo, e1cPicoMaximo).
const e1cVueltas = 3

// e1cPicoVivo corre f y devuelve cuanto crecio, como maximo, la memoria viva
// del heap mientras f corria, y cuantas muestras tomo: otra goroutine fuerza
// un GC completo una y otra vez y anota el HeapAlloc que queda despues de
// cada uno (lo vivo, mas lo poco que f asigno entre el GC y la lectura).
// Cuenta como vivo solo lo que sobrevive a dos GC seguidos: el pico es el
// maximo, sobre cada par de muestras consecutivas, de la menor de las dos.
//
// Ruido de la medida: solo suma. Lo que f asigna mientras un GC marca sale
// marcado (cuenta como vivo en ese ciclo aunque ya sea basura), lo que se
// asigna entre el fin del GC y ReadMemStats tambien cuenta, y lo mismo lo que
// asignan a la vez otras goroutines del proceso (los workers de reintento de
// otros tests, el runtime). En una maquina cargada (otros paquetes de go test
// en paralelo, -race) el marcado dura mas y el ruido crece. Esa basura no
// sobrevive al GC siguiente, y el par la descarta; lo que un lector retiene
// de verdad sigue vivo muestra tras muestra. Lo que puede restar es que no
// caigan dos muestras mientras f retiene. Por eso ademas se mide con
// e1cPicoMinimo (cotas superiores) y e1cPicoMaximo (cotas inferiores).
func e1cPicoVivo(f func()) (pico uint64, muestras int) {
	runtime.GC()
	runtime.GC() // el segundo, despues de los finalizadores que solto el primero
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	base := m.HeapAlloc
	var max atomic.Uint64
	var n atomic.Int64
	parar, listo := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(listo)
		var m runtime.MemStats
		var antes uint64
		for i := 0; ; i++ {
			runtime.GC()
			runtime.ReadMemStats(&m)
			if par := min(antes, m.HeapAlloc); i > 0 && par > max.Load() {
				max.Store(par)
			}
			antes = m.HeapAlloc
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

// e1cPicoMinimo mide f hasta e1cVueltas veces y se queda con el pico MINIMO;
// para en cuanto una vuelta queda en o bajo cota. Como el ruido solo suma,
// el minimo es la mejor cota de lo que f retiene de verdad, y no esconde una
// retencion real: un lector que retiene el contenido de todos los archivos
// pasa la cota en cada vuelta.
func e1cPicoMinimo(cota uint64, f func()) (pico uint64, muestras, vueltas int) {
	for vueltas < e1cVueltas {
		p, m := e1cPicoVivo(f)
		vueltas++
		if vueltas == 1 || p < pico {
			pico, muestras = p, m
		}
		if pico <= cota {
			break
		}
	}
	return pico, muestras, vueltas
}

// e1cPicoMaximo es lo inverso, para el control positivo: mide f hasta
// e1cVueltas veces, se queda con el pico MAXIMO y para en cuanto una vuelta
// llega a cota. Lo unico que le resta al medidor es que ninguna muestra caiga
// mientras f retiene; repetir lo cubre.
func e1cPicoMaximo(cota uint64, f func()) (pico uint64, muestras, vueltas int) {
	for vueltas < e1cVueltas {
		p, m := e1cPicoVivo(f)
		vueltas++
		if p > pico || vueltas == 1 {
			pico, muestras = p, m
		}
		if pico >= cota {
			break
		}
	}
	return pico, muestras, vueltas
}

// e1cGuardaMedidor calibra el medidor en las dos direcciones, con los mismos
// archivos: leerlos y retenerlos todos juntos hace crecer la memoria viva
// por lo menos a la mitad del total (el maximo de hasta e1cVueltas vueltas);
// leerlos de a uno (os.ReadFile y json.Unmarshal, soltando cada uno) la deja
// por debajo de la mitad de e1cTope (el minimo de hasta e1cVueltas vueltas).
// Si no, el test no puede decir nada (fixture).
func e1cGuardaMedidor(t *testing.T, ca string, archivos []string, total int) {
	t.Helper()
	retenerTodos := func() {
		var todos [][]byte
		for _, p := range archivos {
			b, err := os.ReadFile(p)
			if err != nil {
				t.Errorf("fixture: %v", err)
				return
			}
			todos = append(todos, b)
		}
		runtime.KeepAlive(todos)
	}
	deAUno := func() {
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
	}
	todos, mt, vt := e1cPicoMaximo(uint64(total/2), retenerTodos)
	if todos < uint64(total/2) {
		t.Fatalf("%s: fixture: el medidor no ve la retencion: leer y retener los %d archivos (%d MiB) solo subio la memoria viva %.1f MiB (maximo de %d vueltas, %d muestras)",
			ca, len(archivos), total/e1cMiB, float64(todos)/e1cMiB, vt, mt)
	}
	uno, mu, vu := e1cPicoMinimo(e1cTope/2, deAUno)
	if uno > e1cTope/2 {
		t.Fatalf("%s: fixture: el medidor ve retencion donde no la hay: leer de a uno los %d archivos subio la memoria viva %.1f MiB (mas de %d MiB; minimo de %d vueltas, %d muestras)",
			ca, len(archivos), float64(uno)/e1cMiB, e1cTope/2/e1cMiB, vu, mu)
	}
	t.Logf("%s: calibracion: retener todos subio la memoria viva %.1f MiB (%d vueltas); de a uno, %.1f MiB (%d vueltas, %d muestras)",
		ca, float64(todos)/e1cMiB, vt, float64(uno)/e1cMiB, vu, mu)
}

// e1cRegistros escribe n registros de review validos con CreatedAt = cuando
// (WriteRecord les pone el id) y, si tam > 0, los rellena con espacios al
// final hasta tam bytes. Devuelve las rutas y los ids.
func e1cRegistros(t *testing.T, root string, n int, cuando time.Time, tam int) (rutas, ids []string) {
	t.Helper()
	for i := 0; i < n; i++ {
		r, err := WriteRecord(root, Record{CreatedAt: cuando, Task: "endurecimiento-1", Spec: ".hoom/specs/x.md",
			Lenses: []string{"risk"}, Provider: "codex", Writer: "claude", Cross: CrossYes, Findings: []string{}})
		if err != nil {
			t.Fatalf("fixture: WriteRecord: %v", err)
		}
		p := filepath.Join(root, ".hoom", RecordsDir, r.ID+".json")
		if tam > 0 {
			fi, err := os.Stat(p)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p, 0o644); err != nil {
				t.Fatal(err)
			}
			f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				t.Fatal(err)
			}
			_, err = f.Write(bytes.Repeat([]byte(" "), tam-int(fi.Size())))
			f.Close()
			if err != nil {
				t.Fatal(err)
			}
			_ = os.Chmod(p, fi.Mode().Perm())
		}
		rutas, ids = append(rutas, p), append(ids, r.ID)
	}
	return rutas, ids
}

// CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41): con 48 registros de
// review validos de 4 MiB cada uno (192 MiB; el tope por archivo es 256
// MiB), RecordsErr los trae todos y, mientras los lee, la memoria viva no
// crece mas de 48 MiB: lo que retiene queda en el orden de un archivo, no de
// N. Retener el contenido de todos antes de parsear el primero la sube a
// ~192 MiB. Guarda: el medidor ve esa retencion cuando la hay, y no la ve
// leyendo de a uno.
// La medicion es el minimo de hasta e1cVueltas vueltas (e1cPicoMinimo):
// el ruido de la medida solo suma, y un lector que retiene el contenido de
// todos pasa el tope en cada vuelta.
func TestHallazgo_c85840_8c7270_b7bb41_CA442_RecordsErrNoRetieneTodosLosRegistrosALaVez(t *testing.T) {
	const ca = "CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41, reviewcmd.RecordsErr)"
	root := t.TempDir()
	rutas, _ := e1cRegistros(t, root, e1cN, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC), e1cTam)
	e1cGuardaMedidor(t, ca, rutas, e1cN*e1cTam)

	var recs []Record
	var warns []string
	var err error
	pico, muestras, vueltas := e1cPicoMinimo(e1cTope, func() { recs, warns, err = RecordsErr(root) })
	t.Logf("%s: %d registros de %d MiB: la memoria viva crecio %.1f MiB (minimo de %d vueltas, %d muestras)", ca, e1cN, e1cTam/e1cMiB, float64(pico)/e1cMiB, vueltas, muestras)
	if err != nil || len(recs) != e1cN {
		t.Fatalf("%s: fixture: RecordsErr trae los %d registros validos (espacios al final): %d, avisos %v, err %v", ca, e1cN, len(recs), warns, err)
	}
	if pico > e1cTope {
		t.Errorf("%s: con %d registros de %d MiB, RecordsErr no retiene el contenido de todos a la vez: la memoria viva crecio %.1f MiB (minimo de %d vueltas; tope %d MiB, el total es %d MiB)",
			ca, e1cN, e1cTam/e1cMiB, float64(pico)/e1cMiB, vueltas, e1cTope/e1cMiB, e1cN*e1cTam/e1cMiB)
	}
}

// CA-442 (hallazgo b36303): 10 registros de review con el MISMO CreatedAt
// salen de RecordsErr en orden de nombre (su id), no en el que el sistema de
// archivos lista .hoom/reviews (en APFS, el de un hash del nombre). Diez a
// proposito: con hasta 12 empates cualquier sort de Go deja los empates como
// vienen. Guarda: si el directorio ya se lista en orden de nombre, Skip.
func TestHallazgo_b36303_CA442_RecordsErrConEmpatesEnOrdenDeNombre(t *testing.T) {
	const ca = "CA-442 (hallazgo b36303, reviewcmd.RecordsErr con empates)"
	root := t.TempDir()
	_, quiero := e1cRegistros(t, root, 10, time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC), 0)
	sort.Strings(quiero)
	f, err := os.Open(filepath.Join(root, ".hoom", RecordsDir))
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := f.Readdirnames(-1)
	f.Close()
	if sort.StringsAreSorted(crudo) {
		t.Skipf("%s: este sistema de archivos ya lista .hoom/reviews en orden de nombre: el caso no distingue", ca)
	}

	recs, warns, err := RecordsErr(root)
	if err != nil || len(recs) != len(quiero) {
		t.Fatalf("%s: fixture: RecordsErr trae los %d registros: %d, avisos %v, err %v", ca, len(quiero), len(recs), warns, err)
	}
	var got []string
	for _, r := range recs {
		got = append(got, r.ID)
	}
	if strings.Join(got, ",") != strings.Join(quiero, ",") {
		t.Errorf("%s: entre registros con el mismo CreatedAt el orden es el de nombre, determinista, no el del directorio:\n got %v\nquiere %v\n  dir %v",
			ca, got, quiero, crudo)
	}
}
