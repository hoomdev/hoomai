//go:build !windows

// Tests de regresion de la cuarta ronda de la tarea endurecimiento-1 en
// envelope (los hallazgos de la delta 2), los casos que solo se arman en
// unix. Escritos desde el texto de cada hallazgo y el contrato de la
// correccion, SIN LEER LA IMPLEMENTACION. Reusan los helpers e1*, e1b* y e1c*
// de las rondas anteriores; los de este archivo llevan el prefijo e1d.
//
//   - 20261023T133131_4647ba (high) y 20261023T132848_22806c (CA-333):
//     Cerrar soltaba el candado del registro antes de agregar o quitar su
//     cierre pendiente. Con dos Cerrar del mismo registro, el exito del que
//     escribio primero podia borrar el pendiente del que fallo despues (y su
//     cierre no llegaba nunca), o el pendiente del que fallo primero podia
//     agregarse despues del exito del que escribio despues y pisarlo en un
//     reintento. Al final de los reintentos queda en disco el cierre del que
//     escribio ultimo.
//   - 20261023T132848_da912f (CA-333): cada cierre vencido lanzaba su propia
//     goroutine, sin tope: con muchas escrituras colgadas de un proyecto, una
//     goroutine colgada por cada una. La cantidad de goroutines tiene un tope
//     que no crece con los cierres colgados del proyecto.
//   - 20261023T144607_ab66c7 (CA-333): la correccion de da912f puso un tope
//     GLOBAL de cierres en vuelo, y con tantos proyectos colgados como el
//     tope ningun otro cierre del proceso se recuperaba (el bloqueo de
//     1e0674 de vuelta). Ningun proyecto frena a otro: con varios proyectos
//     colgados, el cierre pendiente de un proyecto sano llega a disco.
//
// Como se fuerza el intercalado de 4647ba/22806c, observado de afuera, sin
// leer la implementacion:
//
//  1. Write de un sobre lee .hoom/.gitignore. Si es un FIFO, la escritura se
//     cuelga ahi, con el candado del registro tomado, hasta que el test le da
//     el contenido. Si recibe "envelopes/\n" el FIFO queda en su lugar; si
//     recibe un EOF vacio, Write lo reemplaza por un archivo comun.
//  2. Un tope al tamano de archivo que el proceso puede escribir
//     (RLIMIT_FSIZE, 16 KiB) hace fallar la escritura de un registro con una
//     nota de 64 KiB ("file too large") y no la de uno chico: que cierre
//     falla lo decide su contenido, no el momento. Al devolver el tope, el
//     reintento del grande llega a disco. Go ignora SIGXFSZ.
//  3. Con GOMAXPROCS(1), el modo inanicion de sync.Mutex (el candado del
//     registro es un *sync.Mutex, segun 8bc636) pone al que espera a correr
//     JUSTO entre el Unlock del otro y lo que ese otro haga despues: un Lock
//     que espero mas de 1 ms y al despertar encuentra el candado tomado lo
//     pasa a modo inanicion, y desde ahi Unlock le entrega el candado al que
//     espera y le cede el procesador.
//  4. Una goroutine que ocupa el procesador sin llamadas al sistema mientras
//     dura la carrera deja a sysmon (el hilo del runtime que le quita el
//     procesador a quien lleva mas de una vuelta suya en una llamada al
//     sistema) sin nada que hacer, y su vuelta se estira a 10 ms. Sin ella,
//     sysmon da vueltas de 20 us, le quita el procesador a Y en medio de su
//     escritura (~250 us de llamadas al sistema) y se lo da a X: medido, la
//     carrera se forzaba 1 de cada 30 veces; con ella, 28 a 30 de 30.
//
// La goroutine 1 cierra H y enseguida X; la 2 cierra Y. H se cuelga en el
// FIFO con el candado; Y espera el candado mas de 1 ms; se suelta H, que
// suelta el candado (Y despierta, pero el procesador sigue siendo de la
// goroutine 1) y la goroutine 1 cierra X, que toma el candado antes que Y y
// se cuelga en el FIFO; Y corre, lo encuentra tomado y lo pasa a modo
// inanicion. El test confirma por los stacks que Y espera un candado y X no
// (el orden de escritura es H, X, Y), cambia el FIFO por un archivo y suelta
// a X: el Unlock de X le da el candado a Y, que cierra entero antes de que X
// siga. El intercalado se forzo si Cerrar(Y) volvio antes que Cerrar(X).
//
// RLIMIT_FSIZE y GOMAXPROCS son de todo el proceso: la carrera se arma en un
// proceso hijo (este mismo binario de test, re-ejecutado con -test.run y
// HOOM_E1D_CARRERA_HIJO), que observa el registro en disco y deja un informe;
// el padre lo juzga con las aserciones del caso. Asi el tope de tamano nunca
// alcanza al proceso de go test (su testlog, por ejemplo).
package envelope

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

const (
	// e1dTopeArchivo es RLIMIT_FSIZE mientras dura la carrera, y
	// e1dNotaGrande el largo de la nota del cierre que tiene que fallar.
	e1dTopeArchivo = 16 << 10
	e1dNotaGrande  = 64 << 10
	// e1dIntentos son las veces que se arma la carrera hasta forzar el
	// intercalado una vez.
	e1dIntentos = 5
)

// e1dTopeDeArchivo pone el tope del tamano de archivo que el proceso puede
// escribir (RLIMIT_FSIZE) en tope y devuelve la funcion que lo devuelve (una
// sola vez; tambien al final del test).
func e1dTopeDeArchivo(t *testing.T, tope uint64) func() {
	t.Helper()
	var antes syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
		t.Skipf("este sistema no deja leer RLIMIT_FSIZE: %v", err)
	}
	if antes.Cur < tope || antes.Max < tope {
		t.Skipf("RLIMIT_FSIZE ya es menor que %d: %+v", tope, antes)
	}
	nuevo := antes
	nuevo.Cur = tope
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &nuevo); err != nil {
		t.Skipf("este sistema no deja bajar RLIMIT_FSIZE: %v", err)
	}
	var una sync.Once
	devolver := func() {
		una.Do(func() {
			if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
				t.Errorf("no se pudo devolver RLIMIT_FSIZE a %+v: %v", antes, err)
			}
		})
	}
	t.Cleanup(devolver)
	return devolver
}

// e1dPila es el stack de la goroutine viva que pasa por la funcion marca de
// este paquete ("" si no hay ninguna).
func e1dPila(marca string) string {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	for _, g := range strings.Split(string(buf), "\n\n") {
		if strings.Contains(g, "/internal/envelope."+marca+"(") {
			return g
		}
	}
	return ""
}

// e1dEsperaCandado dice si el stack esta esperando un sync.Mutex.
func e1dEsperaCandado(pila string) bool {
	return strings.Contains(pila, "Mutex).Lock") || strings.Contains(pila, "[sync.Mutex.Lock") ||
		strings.Contains(pila, "[sync.RWMutex.Lock")
}

// e1dCierres guarda lo que devolvio cada Cerrar de la carrera y en que orden
// volvio.
type e1dCierres struct {
	orden           atomic.Int64
	hListo          atomic.Bool
	eh, ex, ey      error
	ox, oy          int64
	uno, dos        chan struct{}
	pilaX, pilaY    string
	armado, forzado bool
}

// e1dGoroutinaUno cierra h y enseguida x, desde la misma goroutine.
func e1dGoroutinaUno(root string, h, x Record, c *e1dCierres) {
	defer close(c.uno)
	c.eh = Cerrar(root, h)
	c.hListo.Store(true)
	c.ex = Cerrar(root, x)
	c.ox = c.orden.Add(1)
}

// e1dGoroutinaDos cierra y.
func e1dGoroutinaDos(root string, y Record, c *e1dCierres) {
	defer close(c.dos)
	c.ey = Cerrar(root, y)
	c.oy = c.orden.Add(1)
}

// e1dEsperarLector espera hasta limite a que alguien tenga el FIFO abierto
// para leer, y devuelve abierto para escribir (sin escribir nada: la lectura
// sigue colgada mientras este abierto); nil si nadie lo leyo.
func e1dEsperarLector(fifo string, limite time.Duration, listo func() bool) *os.File {
	var f *os.File
	e1bEsperar(limite, func() bool {
		if listo != nil && !listo() {
			return false
		}
		f = e1cHayLector(fifo)
		return f != nil
	})
	return f
}

// e1dCarrera arma la carrera de H, X e Y sobre el registro de root (ver el
// comentario del archivo) y devuelve lo que paso: armado si el orden de
// escritura quedo H, X, Y; forzado si ademas Cerrar(Y) volvio antes que
// Cerrar(X). Al volver no queda ninguna escritura colgada, GOMAXPROCS y
// RLIMIT_FSIZE estan como estaban, y los cierres pendientes pueden llegar.
func e1dCarrera(t *testing.T, ca, root string, h, x, y Record) *e1dCierres {
	t.Helper()
	dirHoom := filepath.Join(root, ".hoom")
	gi := filepath.Join(dirHoom, ".gitignore")
	contenido, err := os.ReadFile(gi)
	if err != nil || len(contenido) == 0 {
		t.Fatalf("%s: fixture: Write deja .hoom/.gitignore con su contenido: %q %v", ca, contenido, err)
	}
	if err := os.Remove(gi); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(gi, 0o644); err != nil {
		t.Skipf("este sistema no deja crear un FIFO: %v", err)
	}
	// un segundo nombre del FIFO, fuera de .hoom, para soltar lecturas
	// colgadas aunque .hoom/.gitignore ya sea un archivo comun
	enlace := filepath.Join(root, "e1d-fifo")
	if err := os.Link(gi, enlace); err != nil {
		t.Fatal(err)
	}
	comun := func() { // .hoom/.gitignore vuelve a ser un archivo comun, con su contenido
		tmp := filepath.Join(dirHoom, ".gitignore.e1d")
		if err := os.WriteFile(tmp, contenido, 0o644); err != nil {
			t.Error(err)
			return
		}
		if err := os.Rename(tmp, gi); err != nil {
			t.Error(err)
		}
	}

	c := &e1dCierres{uno: make(chan struct{}), dos: make(chan struct{})}
	var tenidos []*os.File
	var dosLanzada bool
	devolverTope := e1dTopeDeArchivo(t, e1dTopeArchivo)
	procs := runtime.GOMAXPROCS(1)
	// ocupa el procesador sin llamadas al sistema: sysmon se queda sin
	// trabajo y duerme 10 ms entre vueltas (ver el comentario del archivo)
	var pararAcaparador atomic.Bool
	acaparadorListo := make(chan struct{})
	go func() {
		defer close(acaparadorListo)
		for !pararAcaparador.Load() {
		}
	}()
	terminaron := func() bool {
		select {
		case <-c.uno:
		default:
			return false
		}
		if !dosLanzada {
			return true
		}
		select {
		case <-c.dos:
		default:
			return false
		}
		return true
	}
	defer func() {
		// nada queda colgado en el FIFO: archivo comun, se sueltan las
		// lecturas que se tienen y se tocan las que quedaran (macOS pierde a
		// veces el EOF de un FIFO)
		if fi, err := os.Lstat(gi); err == nil && fi.Mode()&os.ModeNamedPipe != 0 {
			comun()
		}
		for _, f := range tenidos {
			f.Close()
		}
		if !e1bEsperar(15*time.Second, func() bool {
			if f := e1cHayLector(enlace); f != nil {
				f.Close()
			}
			return terminaron()
		}) {
			t.Errorf("%s: fixture: 15 s despues de soltar el FIFO, los cierres de la carrera no terminaron", ca)
		}
		pararAcaparador.Store(true)
		<-acaparadorListo
		devolverTope()
		runtime.GOMAXPROCS(procs)
	}()

	go e1dGoroutinaUno(root, h, x, c)
	w1 := e1dEsperarLector(enlace, 10*time.Second, nil)
	if w1 == nil {
		t.Fatalf("%s: fixture: en 10 s la escritura de H no se colgo leyendo el FIFO: el caso no se inyecta", ca)
	}
	tenidos = append(tenidos, w1)
	dosLanzada = true
	go e1dGoroutinaDos(root, y, c)
	// Y llega al candado y espera mas de 1 ms
	time.Sleep(50 * time.Millisecond)
	if p := e1dPila("e1dGoroutinaDos"); !e1dEsperaCandado(p) {
		t.Logf("%s: con H colgada en el FIFO, Cerrar(Y) no espera un candado; su stack:\n%s", ca, p)
		return c
	}

	// se suelta H; la goroutine 1 cierra X, que toma el candado y se cuelga
	if _, err := w1.Write(contenido); err != nil {
		t.Fatal(err)
	}
	w1.Close()
	tenidos = tenidos[:0]
	w2 := e1dEsperarLector(enlace, 10*time.Second, c.hListo.Load)
	if w2 == nil {
		t.Logf("%s: en 10 s, despues de volver Cerrar(H), nadie se colgo leyendo el FIFO", ca)
		return c
	}
	tenidos = append(tenidos, w2)
	// Y despierta, encuentra el candado tomado por X y lo pasa a modo
	// inanicion
	time.Sleep(80 * time.Millisecond)
	c.pilaX, c.pilaY = e1dPila("e1dGoroutinaUno"), e1dPila("e1dGoroutinaDos")
	c.armado = c.pilaX != "" && !e1dEsperaCandado(c.pilaX) && e1dEsperaCandado(c.pilaY)
	if !c.armado {
		t.Logf("%s: el orden de escritura no quedo H, X, Y: stack de X\n%s\nstack de Y\n%s", ca, c.pilaX, c.pilaY)
	}

	// se suelta X con .hoom/.gitignore ya comun: Y no se cuelga
	comun()
	if _, err := w2.Write(contenido); err != nil {
		t.Fatal(err)
	}
	w2.Close()
	tenidos = tenidos[:0]
	if !e1bEsperar(15*time.Second, func() bool {
		if f := e1cHayLector(enlace); f != nil {
			f.Close()
		}
		return terminaron()
	}) {
		t.Fatalf("%s: fixture: 15 s despues de soltar a X, Cerrar(X) y Cerrar(Y) no volvieron", ca)
	}
	c.forzado = c.armado && c.oy < c.ox
	return c
}

// e1dCierre es rec cerrado con estado, nota y fin propios.
func e1dCierre(rec Record, estado, nota string, fin time.Time) Record {
	r := e1Cerrado(rec)
	r.Status, r.Note, r.EndedAt = estado, nota, fin
	if estado != StatusDeliverable {
		r.ExitCode, r.Stage = 1, "verify"
	}
	return r
}

// e1dQue nombra un cierre por su nota, sin los 64 KiB del grande.
func e1dQue(nota string) string {
	if len(nota) > 60 {
		return fmt.Sprintf("%s... (%d bytes)", nota[:40], len(nota))
	}
	return nota
}

// ---------------------------------------------------------------- 4647ba / 22806c en un proceso hijo

const (
	// e1dEnvHijo dice que este proceso es el hijo que arma la carrera ("A"
	// o "B"), y e1dEnvDir donde crea los proyectos y deja su informe.
	e1dEnvHijo = "HOOM_E1D_CARRERA_HIJO"
	e1dEnvDir  = "HOOM_E1D_CARRERA_DIR"
	// e1dArchivoInforme es el informe del hijo, en e1dEnvDir.
	e1dArchivoInforme = "informe.json"
)

// e1dMuestra es el registro en disco en un momento de la observacion: que
// cierre es (H, X, Y, otro o ninguno) y su estado, a Ms ms de que la carrera
// volvio.
type e1dMuestra struct {
	Ms     int64
	Que    string
	Estado string
}

// e1dIntento es lo que el hijo vio en un intento de la carrera.
type e1dIntento struct {
	Root, ID        string
	Armado, Forzado bool
	OX, OY          int64
	// EH, EX y EY son los errores de Cerrar ("" si nil); GrandeEFBIG dice
	// si el del cierre grande es el de la escritura por RLIMIT_FSIZE.
	EH, EX, EY  string
	GrandeEFBIG bool
	// Muestras son los cambios del registro en disco desde que la carrera
	// volvio (la primera es el estado de entonces), durante ObservadoMs.
	Muestras    []e1dMuestra
	ObservadoMs int64
}

// e1dInforme es lo que el hijo le deja al padre.
type e1dInforme struct {
	Intentos []e1dIntento
}

// e1dError es err como texto ("" si nil).
func e1dError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// e1dObservar anota, cada 20 ms, que cierre esta en disco cuando cambia. Con
// Y chico (caso B) observa 4.5 s; con Y grande (caso A), hasta 3 s despues
// de ver Y, o 10 s si no lo ve.
func e1dObservar(root, id string, h, x, y Record, yGrande bool) ([]e1dMuestra, int64) {
	t0 := time.Now()
	var ms []e1dMuestra
	primeraY := time.Duration(-1)
	for {
		r, ok := e1EnDisco(root, id)
		que := "ninguno"
		if ok {
			switch r.Note {
			case h.Note:
				que = "H"
			case x.Note:
				que = "X"
			case y.Note:
				que = "Y"
			default:
				que = "otro: " + e1dQue(r.Note)
			}
		}
		d := time.Since(t0)
		if n := len(ms); n == 0 || ms[n-1].Que != que || ms[n-1].Estado != r.Status {
			ms = append(ms, e1dMuestra{Ms: d.Milliseconds(), Que: que, Estado: r.Status})
		}
		if que == "Y" && primeraY < 0 {
			primeraY = d
		}
		if !yGrande && d >= 4500*time.Millisecond ||
			yGrande && (primeraY >= 0 && d-primeraY >= 3*time.Second || primeraY < 0 && d >= 10*time.Second) {
			return ms, d.Milliseconds()
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// e1dHijoCarreras corre en el proceso hijo: arma la carrera hasta forzar el
// intercalado una vez (a lo sumo e1dIntentos), en cada intento armado
// observa el registro en disco, y deja todo en el informe sin juzgarlo.
// Bajar RLIMIT_FSIZE y fijar GOMAXPROCS(1) afectan a todo el proceso: por
// eso pasa en un hijo y no en el proceso de go test (su testlog, por
// ejemplo, no puede crecer con el tope puesto).
func e1dHijoCarreras(t *testing.T, ca, dir string, xGrande bool) {
	t.Helper()
	var inf e1dInforme
	grande := strings.Repeat("g", e1dNotaGrande)
	for i := 0; i < e1dIntentos; i++ {
		root := filepath.Join(dir, fmt.Sprintf("intento%d", i+1), "proyecto")
		rec := e1Rec(e1cID(800+i), "carrera")
		e1Escribir(t, root, rec)
		t0 := time.Now().UTC().Truncate(time.Second)
		h := e1dCierre(rec, StatusDeliverable, "cierre H: el primero, escrito con exito", t0)
		x := e1dCierre(rec, StatusDeliverable, "cierre X: escrito con exito, antes que Y", t0.Add(time.Second))
		y := e1dCierre(rec, StatusNotDeliverable, "cierre Y: escrito ultimo, fallo y quedo pendiente "+grande, t0.Add(2*time.Second))
		if xGrande {
			x = e1dCierre(rec, StatusNotDeliverable, "cierre X: fallo y quedo pendiente, antes que Y "+grande, t0.Add(time.Second))
			y = e1dCierre(rec, StatusDeliverable, "cierre Y: escrito ultimo, con exito", t0.Add(2*time.Second))
		}

		c := e1dCarrera(t, ca, root, h, x, y)
		in := e1dIntento{Root: root, ID: rec.ID, Armado: c.armado, Forzado: c.forzado, OX: c.ox, OY: c.oy,
			EH: e1dError(c.eh), EX: e1dError(c.ex), EY: e1dError(c.ey)}
		grandeErr := c.ey
		if xGrande {
			grandeErr = c.ex
		}
		in.GrandeEFBIG = grandeErr != nil && (errors.Is(grandeErr, syscall.EFBIG) || strings.Contains(grandeErr.Error(), "file too large"))
		if c.armado {
			in.Muestras, in.ObservadoMs = e1dObservar(root, rec.ID, h, x, y, !xGrande)
		}
		inf.Intentos = append(inf.Intentos, in)
		if c.forzado {
			break
		}
	}
	raw, err := json.MarshalIndent(inf, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, e1dArchivoInforme), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// e1dCarrerasEnHijo arma la carrera en un proceso hijo (este mismo binario
// de test, solo con este test) y devuelve su informe; en el hijo, la arma y
// devuelve nil. Saltea donde el hijo saltea, y falla con su salida si el
// hijo falla.
func e1dCarrerasEnHijo(t *testing.T, ca, caso string, xGrande bool) *e1dInforme {
	t.Helper()
	if os.Getenv(e1dEnvHijo) == caso {
		e1dHijoCarreras(t, ca, os.Getenv(e1dEnvDir), xGrande)
		return nil
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("sin permisos de unix (o como root) no se arma la carrera")
	}
	dir := t.TempDir()
	ctx, cancelar := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancelar()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.count=1", "-test.v", "-test.timeout=4m")
	cmd.Env = append(os.Environ(), e1dEnvHijo+"="+caso, e1dEnvDir+"="+dir)
	salida, err := cmd.CombinedOutput()
	if strings.Contains(string(salida), "--- SKIP") {
		t.Skipf("%s: el hijo salteo el caso:\n%s", ca, salida)
	}
	if err != nil {
		t.Fatalf("%s: fixture: el proceso hijo que arma la carrera fallo (%v):\n%s", ca, err, salida)
	}
	raw, err := os.ReadFile(filepath.Join(dir, e1dArchivoInforme))
	if err != nil {
		t.Fatalf("%s: fixture: el hijo no dejo su informe (%v):\n%s", ca, err, salida)
	}
	var inf e1dInforme
	if err := json.Unmarshal(raw, &inf); err != nil {
		t.Fatalf("%s: fixture: informe del hijo ilegible: %v", ca, err)
	}
	return &inf
}

// e1dComprobarCarreras exige lo comun a los dos casos en cada intento del
// informe (Cerrar(H) y el chico dan nil, el grande da el error de la
// escritura), llama a comprobar con cada intento armado (orden de escritura
// H, X, Y) y, si ninguno forzo el intercalado, falla: el caso no se inyecto.
func e1dComprobarCarreras(t *testing.T, ca string, inf *e1dInforme, xGrande bool, comprobar func(i int, in e1dIntento)) {
	t.Helper()
	forzado := false
	for i, in := range inf.Intentos {
		chico, cual := in.EX, "X"
		if xGrande {
			chico, cual = in.EY, "Y"
		}
		grande := "Y"
		if xGrande {
			grande = "X"
		}
		if in.EH != "" {
			t.Fatalf("%s: intento %d: fixture: Cerrar(H), chico, da nil: %s", ca, i+1, in.EH)
		}
		if chico != "" {
			t.Fatalf("%s: intento %d: fixture: con RLIMIT_FSIZE en %d bytes, Cerrar(%s), chico, da nil: %s", ca, i+1, e1dTopeArchivo, cual, chico)
		}
		if !in.GrandeEFBIG {
			t.Fatalf("%s: intento %d: fixture: con RLIMIT_FSIZE en %d bytes, Cerrar(%s) (nota de %d bytes) devuelve el error de la escritura (EFBIG): X %q, Y %q",
				ca, i+1, e1dTopeArchivo, grande, e1dNotaGrande, in.EX, in.EY)
		}
		t.Logf("%s: intento %d: armado %v, forzado %v (Cerrar(X) volvio %d-o, Cerrar(Y) %d-o); en disco: %v", ca, i+1, in.Armado, in.Forzado, in.OX, in.OY, in.Muestras)
		if !in.Armado {
			continue
		}
		if len(in.Muestras) == 0 {
			t.Fatalf("%s: intento %d: fixture: el hijo no observo el registro", ca, i+1)
		}
		comprobar(i+1, in)
		if r, ok := e1EnDisco(in.Root, in.ID); ok {
			if ult := in.Muestras[len(in.Muestras)-1]; !strings.HasPrefix(r.Note, "cierre "+ult.Que+":") {
				t.Errorf("%s: intento %d: fixture: con el hijo terminado, en disco queda %q y el hijo vio por ultimo %s", ca, i+1, e1dQue(r.Note), ult.Que)
			}
		}
		if t.Failed() {
			return
		}
		forzado = forzado || in.Forzado
	}
	if !forzado {
		t.Fatalf("%s: fixture: en %d intentos no se forzo el intercalado (Cerrar(Y) entero entre el Unlock de X y lo que X hace despues): el caso no se inyecto", ca, len(inf.Intentos))
	}
}

// CA-333 (hallazgos 4647ba y 22806c, caso A): dos Cerrar concurrentes del
// mismo registro; X escribe primero y lo logra, Y escribe despues (ultimo) y
// falla, asi que queda pendiente. El exito de X no borra el pendiente de Y:
// al volver la escritura, en pocos segundos (primer reintento a 1 s) el
// registro en disco es Y, y sigue siendo Y. El intercalado se fuerza: Y
// corre su Cerrar entero (escritura fallida y pendiente) entre el Unlock de X
// y lo que X hace despues. Con el error, X borra el pendiente de Y y el
// registro queda en X para siempre. La carrera se arma en un proceso hijo
// (toca RLIMIT_FSIZE y GOMAXPROCS) que observa el disco y lo informa.
func TestHallazgo_4647ba_22806c_CA333_ElExitoDeUnCierreNoBorraElPendienteDelQueEscribioDespues(t *testing.T) {
	const ca = "CA-333 (hallazgos 4647ba y 22806c, caso A: el exito anterior no borra el pendiente posterior)"
	inf := e1dCarrerasEnHijo(t, ca, "A", false)
	if inf == nil {
		return
	}
	e1dComprobarCarreras(t, ca, inf, false, func(i int, in e1dIntento) {
		if q := in.Muestras[0].Que; q != "X" && q != "Y" {
			t.Fatalf("%s: intento %d: fixture: X escribio con exito y Y fallo: en disco esta X (o ya Y): %s", ca, i, q)
		}
		primera := -1
		for j, m := range in.Muestras {
			if m.Que == "Y" {
				primera = j
				break
			}
		}
		if primera < 0 {
			ult := in.Muestras[len(in.Muestras)-1]
			t.Errorf("%s: intento %d: Y escribio ultimo, fallo y quedo pendiente; %d ms despues de volver la escritura el registro en disco sigue siendo %s (%s): el exito anterior de X borro el pendiente de Y y su cierre se perdio",
				ca, i, in.ObservadoMs, ult.Que, ult.Estado)
			return
		}
		for _, m := range in.Muestras[primera:] {
			if m.Que != "Y" || m.Estado != StatusNotDeliverable {
				t.Errorf("%s: intento %d: %d ms despues de llegar Y, el registro en disco ya no es Y: %s (%s)", ca, i, m.Ms-in.Muestras[primera].Ms, m.Que, m.Estado)
				return
			}
		}
		if in.ObservadoMs-in.Muestras[primera].Ms < 3000 {
			t.Fatalf("%s: intento %d: fixture: el hijo observo Y solo %d ms", ca, i, in.ObservadoMs-in.Muestras[primera].Ms)
		}
	})
}

// CA-333 (hallazgos 4647ba y 22806c, caso B, el inverso): dos Cerrar
// concurrentes del mismo registro; X escribe primero y falla, Y escribe
// despues (ultimo) y lo logra. El pendiente de X no se agrega despues del
// exito de Y ni lo pisa en un reintento: con la escritura de vuelta, durante
// 4.5 s (los reintentos de 1 s y 3 s) el registro en disco sigue siendo Y.
// El intercalado se fuerza: Y corre su Cerrar entero (escritura con exito)
// entre el Unlock de X y lo que X hace despues. Con el error, X agrega su
// pendiente despues del exito de Y y el reintento revierte el registro a X.
// La carrera se arma en un proceso hijo, como en el caso A.
func TestHallazgo_4647ba_22806c_CA333_UnPendienteAnteriorNoPisaAlQueEscribioDespues(t *testing.T) {
	const ca = "CA-333 (hallazgos 4647ba y 22806c, caso B: el pendiente anterior no pisa al posterior)"
	inf := e1dCarrerasEnHijo(t, ca, "B", true)
	if inf == nil {
		return
	}
	e1dComprobarCarreras(t, ca, inf, true, func(i int, in e1dIntento) {
		if q := in.Muestras[0].Que; q != "Y" {
			t.Fatalf("%s: intento %d: fixture: X fallo y Y escribio con exito despues: en disco esta Y: %s", ca, i, q)
		}
		for _, m := range in.Muestras {
			if m.Que != "Y" || m.Estado != StatusDeliverable {
				t.Errorf("%s: intento %d: %d ms despues de volver la escritura, el registro en disco ya no es Y (que escribio ultimo): %s (%s): el pendiente de X se agrego despues del exito de Y y su reintento revirtio el estado terminal",
					ca, i, m.Ms, m.Que, m.Estado)
				return
			}
		}
		if in.ObservadoMs < 4500 {
			t.Fatalf("%s: intento %d: fixture: el hijo observo solo %d ms", ca, i, in.ObservadoMs)
		}
	})
}

// ---------------------------------------------------------------- da912f / ab66c7

const (
	// e1dTanda son los cierres colgados que se suman en cada fase de da912f.
	e1dTanda = 50
	// e1dTopeGoroutinas es lo mas que puede crecer el proceso con 2 x
	// e1dTanda (100) cierres colgados de un proyecto, y e1dTopeTanda lo mas
	// que puede sumar la segunda tanda de 50: el tope no crece con los
	// colgados.
	e1dTopeGoroutinas = 32
	e1dTopeTanda      = 8
)

// e1dColgado es un proyecto con cierres pendientes cuyos reintentos se
// cuelgan leyendo .hoom/.gitignore, que es un FIFO (enlace es otro nombre
// del FIFO, fuera de .hoom).
type e1dColgado struct {
	root, gi, enlace string
	recs             []Record
	// tenido es el FIFO abierto para escribir mientras alguna lectura este
	// colgada: sin escribir nada, todas sus lecturas siguen colgadas.
	tenido *os.File
}

// e1dArmarColgado escribe n sobres abiertos en root, hace fallar sus Cerrar
// (sin escritura), devuelve la escritura y deja .hoom/.gitignore como un
// FIFO que nadie escribe: el reintento de cada cierre se cuelga leyendolo.
// Devuelve el proyecto y el contenido que tenia .hoom/.gitignore.
func e1dArmarColgado(t *testing.T, ca, root string, desde, n int) (*e1dColgado, []byte) {
	t.Helper()
	p := &e1dColgado{root: root, gi: filepath.Join(root, ".hoom", ".gitignore"), enlace: filepath.Join(root, "e1d-fifo")}
	for i := desde; i < desde+n; i++ {
		rec := e1Rec(e1cID(i), "colgado")
		e1Escribir(t, root, rec)
		p.recs = append(p.recs, rec)
	}
	contenido, err := os.ReadFile(p.gi)
	if err != nil || len(contenido) == 0 {
		t.Fatalf("%s: fixture: Write deja .hoom/.gitignore con su contenido: %q %v", ca, contenido, err)
	}
	devolver := e1bSinEscritura(t, root, e1bIDsDe(p.recs))
	e1bCerrarFalla(t, ca, root, p.recs)
	devolver()
	if err := os.Remove(p.gi); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(p.gi, 0o644); err != nil {
		t.Skipf("este sistema no deja crear un FIFO: %v", err)
	}
	if err := os.Link(p.gi, p.enlace); err != nil {
		t.Fatal(err)
	}
	return p, contenido
}

// e1dSoltarColgados deja que todos terminen: .hoom/.gitignore vuelve a ser un
// archivo comun (los reintentos que vengan no se cuelgan), se sueltan las
// lecturas colgadas (EOF) y se tocan las que quedaran (macOS pierde a veces
// el EOF de un FIFO), hasta que todos los cierres estan en disco (a lo sumo
// 60 s).
func e1dSoltarColgados(t *testing.T, ca string, ps []*e1dColgado, contenido []byte) {
	t.Helper()
	total := 0
	for _, p := range ps {
		total += len(p.recs)
		if fi, err := os.Lstat(p.gi); err == nil && fi.Mode()&os.ModeNamedPipe != 0 {
			tmp := p.gi + ".e1d"
			if err := os.WriteFile(tmp, contenido, 0o644); err == nil {
				_ = os.Rename(tmp, p.gi)
			}
		}
		if p.tenido != nil {
			p.tenido.Close()
			p.tenido = nil
		}
	}
	t0 := time.Now()
	cerrados := 0
	ok := e1bEsperar(60*time.Second, func() bool {
		cerrados = 0
		for _, p := range ps {
			if f := e1cHayLector(p.enlace); f != nil {
				f.Close()
			}
			for _, rec := range p.recs {
				if r, ok := e1EnDisco(p.root, rec.ID); ok && r.Status == StatusDeliverable {
					cerrados++
				}
			}
		}
		return cerrados == total
	})
	if !ok {
		t.Logf("%s: 60 s despues de soltar los FIFO, %d de %d cierres llegaron a disco", ca, cerrados, total)
		return
	}
	t.Logf("%s: al soltar los FIFO, los %d cierres llegaron a disco en %s", ca, total, time.Since(t0).Round(100*time.Millisecond))
}

// e1dColgados cuenta los proyectos con alguna lectura colgada en su FIFO, y
// sostiene esas lecturas.
func e1dColgados(ps []*e1dColgado) int {
	n := 0
	for _, p := range ps {
		if p.tenido == nil {
			p.tenido = e1cHayLector(p.enlace)
		}
		if p.tenido != nil {
			n++
		}
	}
	return n
}

// e1dEstable espera a que venzan los reintentos (~1 s despues de cada
// Cerrar: 2.5 s) y despues a que medida no cambie en 1.5 s (a lo sumo 15
// s); devuelve la ultima medida.
func e1dEstable(medida func() int) int {
	time.Sleep(2500 * time.Millisecond)
	n, desde, fin := medida(), time.Now(), time.Now().Add(15*time.Second)
	for time.Since(desde) < 1500*time.Millisecond && time.Now().Before(fin) {
		time.Sleep(100 * time.Millisecond)
		if m := medida(); m != n {
			n, desde = m, time.Now()
		}
	}
	return n
}

// CA-333 (hallazgo da912f): MUCHOS cierres pendientes de UN proyecto cuyos
// reintentos se cuelgan en la escritura (su .hoom/.gitignore es un FIFO que
// nadie escribe; cada Cerrar fallo sin escritura y despues la escritura
// volvio) no suman una goroutine cada uno: el total tiene un tope que no
// crece con ellos. Dos tandas de 50 cierres del mismo proyecto: con los 100
// colgados el proceso no gano mas de 32 goroutines, y la segunda tanda no
// sumo mas de 8. Con el error, cada cierre vencido lanza su goroutine y se
// cuelga: +50 y +100. Guarda: alguna escritura se colgo de verdad en el FIFO
// (si no, el caso no se inyecto). Al final se suelta todo.
func TestHallazgo_da912f_CA333_LosCierresColgadosDeUnProyectoNoSumanUnaGoroutinaCadaUno(t *testing.T) {
	const ca = "CA-333 (hallazgo da912f, cierres colgados de un proyecto)"
	root := filepath.Join(t.TempDir(), "proyecto")
	var ps []*e1dColgado
	var contenido []byte
	// despues de t.TempDir: corre antes de que se borre el proyecto
	t.Cleanup(func() { e1dSoltarColgados(t, ca, ps, contenido) })

	todas0, paq0 := e1bGoroutinas()
	p, cont := e1dArmarColgado(t, ca, root, 1000, e1dTanda)
	ps, contenido = []*e1dColgado{p}, cont
	if p.tenido = e1dEsperarLector(p.enlace, 10*time.Second, nil); p.tenido == nil {
		t.Fatalf("%s: fixture: con %d cierres pendientes y .hoom/.gitignore como FIFO, en 10 s ninguna escritura de reintento se colgo leyendolo: el caso no se inyecto", ca, e1dTanda)
	}
	todas1 := e1dEstable(runtime.NumGoroutine)
	paq1 := e1bPaquete()

	// segunda tanda en el mismo proyecto: mientras se arma, .hoom/.gitignore
	// vuelve a ser comun (Write lo lee: si no, el Cerrar que tiene que fallar
	// se colgaria); las lecturas colgadas siguen en el FIFO (enlace)
	if err := os.Remove(p.gi); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p.gi, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
	var segunda []Record
	for i := 1000 + e1dTanda; i < 1000+2*e1dTanda; i++ {
		rec := e1Rec(e1cID(i), "colgado")
		e1Escribir(t, root, rec)
		segunda = append(segunda, rec)
	}
	devolver := e1bSinEscritura(t, root, e1bIDsDe(segunda))
	e1bCerrarFalla(t, ca, root, segunda)
	devolver()
	if err := os.Remove(p.gi); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(p.enlace, p.gi); err != nil {
		t.Fatal(err)
	}
	p.recs = append(p.recs, segunda...)
	todas2 := e1dEstable(runtime.NumGoroutine)
	paq2 := e1bPaquete()

	t.Logf("%s: goroutines del proceso (del paquete): antes %d (%d); con %d cierres colgados del proyecto, %d (%d); con %d, %d (%d)",
		ca, todas0, paq0, e1dTanda, todas1, paq1, 2*e1dTanda, todas2, paq2)
	if todas2-todas0 > e1dTopeGoroutinas {
		t.Errorf("%s: con %d cierres de un proyecto cuyas escrituras se cuelgan, la cantidad de goroutines tiene un tope: el proceso paso de %d a %d (+%d, tope +%d)",
			ca, 2*e1dTanda, todas0, todas2, todas2-todas0, e1dTopeGoroutinas)
	}
	if todas2-todas1 > e1dTopeTanda {
		t.Errorf("%s: el tope no crece con los cierres colgados: pasar de %d a %d sumo %d goroutines (de %d a %d; tope +%d)",
			ca, e1dTanda, 2*e1dTanda, todas2-todas1, todas1, todas2, e1dTopeTanda)
	}
}

// CA-333 (hallazgo ab66c7): con varios proyectos colgados (20, con 2
// cierres pendientes cada uno cuyos reintentos se cuelgan en su FIFO), el
// cierre pendiente de un proyecto sano llega a disco a tiempo: su Cerrar
// falla sin escritura, la escritura vuelve enseguida y en 8 s (reintentos de
// 1 s, 3 s y 7 s) el registro esta cerrado. Ningun proyecto frena a otro.
// Con un tope global de cierres en vuelo menor que los colgados, el cierre
// sano no llega mientras los otros sigan colgados. Guarda: alguna escritura
// se colgo de verdad (si no, el caso no se inyecto). Al final se suelta
// todo.
func TestHallazgo_ab66c7_CA333_ProyectosColgadosNoFrenanElCierreDeUnProyectoSano(t *testing.T) {
	const ca = "CA-333 (hallazgo ab66c7, proyectos colgados contra uno sano)"
	const proyectos, porProyecto = 20, 2
	base := t.TempDir()
	var ps []*e1dColgado
	var contenido []byte
	// despues de t.TempDir: corre antes de que se borren los proyectos
	t.Cleanup(func() { e1dSoltarColgados(t, ca, ps, contenido) })

	for i := 0; i < proyectos; i++ {
		p, cont := e1dArmarColgado(t, ca, filepath.Join(base, fmt.Sprintf("colgado%02d", i)), 1200+i*porProyecto, porProyecto)
		ps, contenido = append(ps, p), cont
	}
	n := e1dEstable(func() int { return e1dColgados(ps) })
	t.Logf("%s: %d de %d proyectos con escrituras colgadas; goroutines del proceso %d", ca, n, proyectos, runtime.NumGoroutine())
	if n == 0 {
		t.Fatalf("%s: fixture: con %d proyectos con cierres pendientes y .hoom/.gitignore como FIFO, ninguna escritura de reintento se colgo: el caso no se inyecto", ca, proyectos)
	}

	sano := filepath.Join(base, "sano")
	rec := e1Rec(e1cID(1300), "sano")
	e1Escribir(t, sano, rec)
	devolver := e1SinEscritura(t, sano, rec.ID)
	if err := Cerrar(sano, e1Cerrado(rec)); err == nil {
		t.Fatalf("%s: fixture: con .hoom/envelopes sin escritura Cerrar del proyecto sano devuelve el error, no nil", ca)
	}
	devolver()
	t0 := time.Now()
	llego := e1bEsperar(8*time.Second, func() bool {
		r, ok := e1EnDisco(sano, rec.ID)
		return ok && r.Status == StatusDeliverable && !r.EndedAt.IsZero()
	})
	if !llego {
		r, _ := e1EnDisco(sano, rec.ID)
		t.Errorf("%s: con %d proyectos colgados (%d con escrituras colgadas), el cierre pendiente de un proyecto sano llega a disco: 8 s despues de volver su escritura sigue %q en %q",
			ca, proyectos, n, r.Status, r.Stage)
		return
	}
	t.Logf("%s: el cierre del proyecto sano llego %s despues de volver su escritura", ca, time.Since(t0).Round(100*time.Millisecond))
}
