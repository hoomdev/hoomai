// Tests de regresion de la tercera ronda de la tarea endurecimiento-1 en
// envelope. Escritos desde el texto de cada hallazgo, las specs y el
// contrato que promete la correccion, SIN LEER LA IMPLEMENTACION. Reusan los
// helpers e1* y e1b* de las rondas anteriores; los de este archivo llevan el
// prefijo e1c.
//
//   - 20261023T122536_c85840 (high), 20261023T123049_8c7270 y
//     20261023T123540_b7bb41 (CA-442, CA-369): el lector comun retenia en
//     memoria el contenido de TODOS los registros antes de parsear el
//     primero. ListErr ahora procesa los registros de .hoom/envelopes de a
//     uno: la memoria viva maxima queda en el orden de un archivo, no de N.
//   - 20261023T123057_b36303 (low, CA-442): entre registros con el mismo
//     instante, ListErr devuelve un orden de nombre (creciente o
//     decreciente), nunca el del directorio.
//   - 20261023T123036_b7417b (CA-333): un cierre A que fallo y quedo
//     pendiente se reescribia aunque despues un cierre B del mismo registro
//     se hubiera escrito con exito, y revertia en disco el estado terminal
//     mas nuevo. B queda en disco.
//   - 20261023T123041_eafa2d (CA-333): el backoff era del worker global, no
//     de cada cierre: un cierre nuevo agregado mientras otro viejo llevaba
//     tiempo reintentando esperaba el tramo largo del viejo antes de su
//     primer intento. Ahora el backoff es por cierre: su primer reintento es
//     ~1 s despues de agregarse.
//
// El hallazgo 20261023T123540_1e0674 (una escritura bloqueada frena a todos)
// esta en hallazgos_endurecimiento1c_unix_test.go: el bloqueo se arma con un
// FIFO.
package envelope

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

// e1cID es el id del i-esimo sobre de estos tests.
func e1cID(i int) string { return fmt.Sprintf("20261023T12%02d%02d_e1c%03x", (i/60)%60, i%60, i) }

// e1cRellenar agrega espacios al final de p hasta que pese tam bytes: el
// JSON sigue siendo el mismo y valido.
func e1cRellenar(t *testing.T, p string, tam int) {
	t.Helper()
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

// ---------------------------------------------------------------- c85840 / 8c7270 / b7bb41

// CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41): con 192 registros de
// sobre validos de casi 1 MiB cada uno (~191 MiB; el tope por archivo es 1
// MiB), ListErr los trae todos y, mientras los lee, la memoria viva no crece
// mas de 48 MiB: lo que retiene queda en el orden de un archivo, no de N.
// Retener el contenido de todos antes de parsear el primero la sube a ~191
// MiB. Guarda: el medidor ve esa retencion cuando la hay, y no la ve leyendo
// de a uno.
// La medicion es el minimo de hasta e1cVueltas vueltas (e1cPicoMinimo):
// el ruido de la medida solo suma, y un lector que retiene el contenido de
// todos pasa el tope en cada vuelta.
func TestHallazgo_c85840_8c7270_b7bb41_CA442_ListErrNoRetieneTodosLosRegistrosALaVez(t *testing.T) {
	const ca = "CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41, envelope.ListErr)"
	root := t.TempDir()
	var rutas []string
	for i := 0; i < e1cN; i++ {
		rec := e1Rec(e1cID(i), "propio")
		e1Escribir(t, root, rec)
		p := filepath.Join(root, ".hoom", DirName, rec.ID+".json")
		e1cRellenar(t, p, e1cTam)
		rutas = append(rutas, p)
	}
	e1cGuardaMedidor(t, ca, rutas, e1cN*e1cTam)

	var rs []Record
	var err error
	pico, muestras, vueltas := e1cPicoMinimo(e1cTope, func() { rs, err = ListErr(root) })
	t.Logf("%s: %d registros de ~1 MiB: la memoria viva crecio %.1f MiB (minimo de %d vueltas, %d muestras)", ca, e1cN, float64(pico)/e1cMiB, vueltas, muestras)
	if err != nil || len(rs) != e1cN {
		t.Fatalf("%s: fixture: ListErr trae los %d registros validos (espacios al final): %d, err %v", ca, e1cN, len(rs), err)
	}
	if pico > e1cTope {
		t.Errorf("%s: con %d registros de ~1 MiB, ListErr no retiene el contenido de todos a la vez: la memoria viva crecio %.1f MiB (minimo de %d vueltas; tope %d MiB, el total es %d MiB)",
			ca, e1cN, float64(pico)/e1cMiB, vueltas, e1cTope/e1cMiB, e1cN*e1cTam/e1cMiB)
	}
}

// ---------------------------------------------------------------- b36303

// CA-442 (hallazgo b36303): 10 registros con los MISMOS instantes (inicio,
// ultima actualizacion) salen de ListErr en orden de nombre (su id;
// creciente o decreciente: "del mas nuevo al mas viejo" no dice como
// desempatar), no en el que el sistema de archivos lista .hoom/envelopes (en
// APFS, el de un hash del nombre). Diez a proposito: con hasta 12 empates
// cualquier sort de Go deja los empates como vienen. Guarda: si el
// directorio ya se lista en orden de nombre (o al reves), Skip.
func TestHallazgo_b36303_CA442_ListErrConEmpatesEnOrdenDeNombre(t *testing.T) {
	const ca = "CA-442 (hallazgo b36303, envelope.ListErr con empates)"
	root := t.TempDir()
	dir := filepath.Join(root, ".hoom", DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	var quiero []string
	for i := 0; i < 10; i++ {
		id := fmt.Sprintf("20261023T120000_%06x", (i*0x9e3779+0x5bd1e9)&0xffffff)
		rec := e1Rec(id, "propio")
		rec.UpdatedAt = rec.StartedAt
		raw, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, id+".json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		quiero = append(quiero, id)
	}
	sort.Strings(quiero)
	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	crudo, _ := f.Readdirnames(-1)
	f.Close()
	var crudoIDs []string
	for _, n := range crudo {
		crudoIDs = append(crudoIDs, strings.TrimSuffix(n, ".json"))
	}
	if e1cOrdenDeNombre(crudoIDs, quiero) {
		t.Skipf("%s: este sistema de archivos ya lista .hoom/envelopes en orden de nombre: el caso no distingue", ca)
	}

	rs, err := ListErr(root)
	if err != nil || len(rs) != len(quiero) {
		t.Fatalf("%s: fixture: ListErr trae los %d registros: %v, err %v", ca, len(quiero), e1IDs(rs), err)
	}
	var got []string
	for _, r := range rs {
		got = append(got, r.ID)
	}
	if !e1cOrdenDeNombre(got, quiero) {
		t.Errorf("%s: entre registros con los mismos instantes el orden es el de nombre, determinista, no el del directorio:\n got %v\nquiere %v (o al reves)\n  dir %v",
			ca, got, quiero, crudoIDs)
	}
}

// ---------------------------------------------------------------- b7417b

// CA-333 (hallazgo b7417b): un cierre A que fallo y quedo pendiente no pisa
// a un cierre B del mismo registro que se escribio con exito despues. Con
// .hoom/envelopes sin escritura, Cerrar(A) (no-entregable) falla y queda
// para reintentar (el primero, a 1 s); enseguida vuelve la escritura y
// Cerrar(B) (entregable, otra nota) da nil: B esta en disco. Durante 4.5 s
// (los reintentos de 1 s y 3 s) el registro en disco sigue siendo B, nunca
// A: el reintento de un cierre viejo no revierte el estado terminal mas
// nuevo.
func TestHallazgo_b7417b_CA333_UnCierrePendienteNoPisaElCierreQueSeEscribioDespues(t *testing.T) {
	const ca = "CA-333 (hallazgo b7417b, cierre pendiente contra un cierre escrito despues)"
	root := t.TempDir()
	rec := e1Rec(e1cID(500), "propio")
	e1Escribir(t, root, rec)
	devolver := e1SinEscritura(t, root, rec.ID)

	a := e1Cerrado(rec)
	a.Status, a.ExitCode, a.Stage, a.Note = StatusNotDeliverable, 1, "verify", "cierre A: fallo y quedo pendiente"
	if err := Cerrar(root, a); err == nil {
		t.Fatalf("%s: fixture: con .hoom/envelopes sin escritura Cerrar(A) devuelve el error, no nil", ca)
	}
	devolver()
	b := e1Cerrado(rec)
	b.Note = "cierre B: escrito con exito despues de A"
	b.EndedAt = a.EndedAt.Add(time.Second)
	if err := Cerrar(root, b); err != nil {
		t.Fatalf("%s: fixture: con la escritura de vuelta Cerrar(B) da nil: %v", ca, err)
	}
	if r, ok := e1EnDisco(root, rec.ID); !ok || r.Note != b.Note {
		t.Fatalf("%s: fixture: cuando Cerrar(B) vuelve, B esta en disco: %q %v", ca, r.Note, ok)
	}

	t0 := time.Now()
	for time.Since(t0) < 4500*time.Millisecond {
		r, ok := e1EnDisco(root, rec.ID)
		if !ok || r.Note != b.Note || r.Status != StatusDeliverable {
			t.Fatalf("%s: %s despues de escribirse B, el registro en disco ya no es B: %q en %q con nota %q (el reintento del cierre A pendiente revirtio el estado terminal mas nuevo)",
				ca, time.Since(t0).Round(10*time.Millisecond), r.Status, r.Stage, r.Note)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ---------------------------------------------------------------- eafa2d

// CA-333 (hallazgo eafa2d): el backoff es de cada cierre, no del worker. Un
// cierre viejo X (de otro proyecto) falla desde hace 7.5 s, sin escritura
// en todo el test: con un backoff global de 1 s, 2 s, 4 s, el worker ya
// esta en su espera de 8 s. Entonces un cierre nuevo Y falla (sin
// escritura) y enseguida vuelve la escritura de su proyecto: el primer
// reintento de Y es ~1 s despues de agregarse, asi que antes de 4 s Y esta
// en disco; no espera el tramo largo de X.
func TestHallazgo_eafa2d_CA333_ElPrimerReintentoDeUnCierreNuevoNoEsperaElBackoffDeOtro(t *testing.T) {
	const ca = "CA-333 (hallazgo eafa2d, backoff por cierre)"
	viejo, nuevo := t.TempDir(), t.TempDir()
	x := e1Rec(e1cID(600), "viejo")
	y := e1Rec(e1cID(601), "nuevo")
	e1Escribir(t, viejo, x)
	e1Escribir(t, nuevo, y)
	e1SinEscritura(t, viejo, x.ID) // X no se puede escribir en todo el test

	tx := time.Now()
	if err := Cerrar(viejo, e1Cerrado(x)); err == nil {
		t.Fatalf("%s: fixture: sin escritura Cerrar(X) devuelve el error, no nil", ca)
	}
	time.Sleep(7500*time.Millisecond - time.Since(tx))
	if r, ok := e1EnDisco(viejo, x.ID); !ok || r.Status != StatusRunning {
		t.Fatalf("%s: fixture: sin escritura, X sigue abierto en disco: %q %v", ca, r.Status, ok)
	}

	devolverY := e1SinEscritura(t, nuevo, y.ID)
	ty := time.Now()
	if err := Cerrar(nuevo, e1Cerrado(y)); err == nil {
		t.Fatalf("%s: fixture: sin escritura Cerrar(Y) devuelve el error, no nil", ca)
	}
	devolverY()
	cerrado := func() bool {
		r, ok := e1EnDisco(nuevo, y.ID)
		return ok && r.Status == StatusDeliverable && !r.EndedAt.IsZero()
	}
	if !e1bEsperar(4*time.Second, cerrado) {
		r, _ := e1EnDisco(nuevo, y.ID)
		t.Errorf("%s: el primer reintento de un cierre nuevo es ~1 s despues de agregarse aunque otro cierre lleve %s reintentando: 4 s despues de agregarse (y con su proyecto escribible) Y sigue %q en %q",
			ca, ty.Sub(tx).Round(100*time.Millisecond), r.Status, r.Stage)
		if e1bEsperar(30*time.Second, cerrado) {
			t.Logf("%s: Y llego a disco %s despues de agregarse", ca, time.Since(ty).Round(100*time.Millisecond))
		}
		return
	}
	t.Logf("%s: Y llego a disco %s despues de agregarse", ca, time.Since(ty).Round(100*time.Millisecond))
}
