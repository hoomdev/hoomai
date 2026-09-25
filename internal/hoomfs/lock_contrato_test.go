// Tests de contrato de hoomfs.Lock para el hallazgo 20260925T041933_51d902
// (medium, reliability): el candado no se asociaba con su dueno, dos procesos
// podian entrar sobre un candado "vencido" y soltar borraba el candado de un
// sucesor. Lo usan item.MarkDone/AddSession (CA-349, CA-348) y task done
// (CA-343). Escritos desde el contrato, sin leer la implementacion.
package hoomfs

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const (
	lcEnvModo = "HOOMFS_LC_MODO"
	lcEnvPath = "HOOMFS_LC_PATH"
	lcEnvDir  = "HOOMFS_LC_DIR"
)

// lcTomar toma el candado o corta el test.
func lcTomar(t *testing.T, path string) func() {
	t.Helper()
	rel, err := Lock(path, 2*time.Second)
	if err != nil {
		t.Fatalf("Lock(%s) sin nadie adentro: %v", path, err)
	}
	return rel
}

// lcNoEntra exige que, con un dueno adentro, otro Lock no entre y devuelva
// ErrLocked.
func lcNoEntra(t *testing.T, path, contexto string) {
	t.Helper()
	rel, err := Lock(path, 100*time.Millisecond)
	if err == nil {
		rel()
		t.Fatalf("%s: un tercero entro con el dueno actual adentro", contexto)
	}
	if !errors.Is(err, ErrLocked) {
		t.Fatalf("%s: el error no cumple errors.Is(err, ErrLocked): %v", contexto, err)
	}
}

// lcDueno es un subproceso (el binario de test re-ejecutado) que toma el
// candado.
type lcDueno struct {
	t      *testing.T
	dir    string
	cmd    *exec.Cmd
	out    bytes.Buffer
	exited chan struct{}
	err    error
}

// lcLanzar arranca el subproceso en modo "sostiene" (toma, avisa y suelta
// cuando el padre se lo pide) o "muere" (toma, avisa y termina sin soltar).
func lcLanzar(t *testing.T, modo, path string) *lcDueno {
	t.Helper()
	d := &lcDueno{t: t, dir: t.TempDir(), exited: make(chan struct{})}
	d.cmd = exec.Command(os.Args[0], "-test.run=^TestLcSubproceso$", "-test.count=1")
	d.cmd.Env = append(os.Environ(), lcEnvModo+"="+modo, lcEnvPath+"="+path, lcEnvDir+"="+d.dir)
	d.cmd.Stdout = &d.out
	d.cmd.Stderr = &d.out
	if err := d.cmd.Start(); err != nil {
		t.Fatalf("no arranco el subproceso: %v", err)
	}
	go func() {
		d.err = d.cmd.Wait()
		close(d.exited)
	}()
	t.Cleanup(func() {
		d.soltar()
		select {
		case <-d.exited:
		case <-time.After(10 * time.Second):
			_ = d.cmd.Process.Kill()
			<-d.exited
		}
	})
	return d
}

// esperarListo espera a que el subproceso tenga el candado.
func (d *lcDueno) esperarListo() {
	d.t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(filepath.Join(d.dir, "listo")); err == nil {
			return
		}
		select {
		case <-d.exited:
			d.t.Fatalf("el subproceso termino sin tomar el candado: %v\n%s", d.err, d.out.String())
		default:
		}
		time.Sleep(5 * time.Millisecond)
	}
	d.t.Fatal("el subproceso no tomo el candado en 20s")
}

func (d *lcDueno) soltar() {
	_ = os.WriteFile(filepath.Join(d.dir, "soltar"), nil, 0o644)
}

func (d *lcDueno) solto() bool {
	_, err := os.Stat(filepath.Join(d.dir, "soltando"))
	return err == nil
}

// esperarFin espera a que el subproceso termine y devuelve su error de Wait.
func (d *lcDueno) esperarFin() error {
	d.t.Helper()
	select {
	case <-d.exited:
		return d.err
	case <-time.After(20 * time.Second):
		d.t.Fatal("el subproceso no termino en 20s")
		return nil
	}
}

// TestLcSubproceso no es un test: es el dueno en otro proceso que usan los
// tests entre procesos (hallazgo 20260925T041933_51d902, CA-349). Sin la
// variable de entorno no hace nada.
func TestLcSubproceso(t *testing.T) {
	modo := os.Getenv(lcEnvModo)
	if modo == "" {
		return
	}
	path, dir := os.Getenv(lcEnvPath), os.Getenv(lcEnvDir)
	rel, err := Lock(path, 15*time.Second)
	if err != nil {
		fmt.Fprintf(os.Stderr, "subproceso: Lock(%s): %v\n", path, err)
		os.Exit(2)
	}
	if err := os.WriteFile(filepath.Join(dir, "listo"), nil, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "subproceso: listo: %v\n", err)
		os.Exit(2)
	}
	switch modo {
	case "muere":
		os.Exit(3) // termina sin soltar
	case "sostiene":
		deadline := time.Now().Add(30 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(filepath.Join(dir, "soltar")); err == nil {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		_ = os.WriteFile(filepath.Join(dir, "soltando"), nil, 0o644)
		rel()
		os.Exit(0)
	}
	fmt.Fprintf(os.Stderr, "subproceso: modo desconocido %q\n", modo)
	os.Exit(2)
}

// Hallazgo 20260925T041933_51d902, CA-349: N goroutines del mismo proceso,
// como mucho una adentro a la vez.
func TestLcExclusionEntreGoroutines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.lock")
	const n, vueltas = 8, 10
	var adentro, maximo atomic.Int32
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for g := 0; g < n; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < vueltas; i++ {
				rel, err := Lock(path, 30*time.Second)
				if err != nil {
					errs <- err
					return
				}
				v := adentro.Add(1)
				for {
					m := maximo.Load()
					if v <= m || maximo.CompareAndSwap(m, v) {
						break
					}
				}
				time.Sleep(300 * time.Microsecond)
				adentro.Add(-1)
				rel()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("una goroutine no consiguio el candado: %v", err)
	}
	if m := maximo.Load(); m != 1 {
		t.Fatalf("hubo %d goroutines adentro a la vez; el candado admite una", m)
	}
}

// Hallazgo 20260925T041933_51d902, CA-349: soltar solo suelta el candado de
// quien lo tomo. Llamar la funcion dos veces, o tarde (con otro ya adentro),
// no deja entrar a un tercero mientras el dueno actual lo tiene.
func TestLcSoltarTardeODosVecesNoDejaEntrarATercero(t *testing.T) {
	t.Run("dos veces antes de que otro entre", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "item.lock")
		relA := lcTomar(t, path)
		relA()
		relA()
		relB := lcTomar(t, path)
		defer relB()
		lcNoEntra(t, path, "A solto dos veces y B entro despues")
	})
	t.Run("tarde con otro adentro", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "item.lock")
		relA := lcTomar(t, path)
		relA()
		relB := lcTomar(t, path)
		defer relB()
		relA() // A suelta tarde: B es el dueno
		lcNoEntra(t, path, "A solto tarde con B adentro")
	})
	t.Run("tarde con otro proceso adentro", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "item.lock")
		relA := lcTomar(t, path)
		relA()
		d := lcLanzar(t, "sostiene", path)
		d.esperarListo()
		relA() // A suelta tarde: el subproceso es el dueno
		lcNoEntra(t, path, "A solto tarde con otro proceso adentro")
		d.soltar()
		if err := d.esperarFin(); err != nil {
			t.Fatalf("el subproceso fallo: %v\n%s", err, d.out.String())
		}
	})
}

// Hallazgo 20260925T041933_51d902, CA-349: exclusion entre procesos. Un
// subproceso toma el candado y lo mantiene; el padre no entra antes de que
// suelte, y entra despues.
func TestLcExclusionEntreProcesos(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.lock")
	d := lcLanzar(t, "sostiene", path)
	d.esperarListo()
	lcNoEntra(t, path, "con el subproceso adentro")

	type resultado struct {
		rel   func()
		err   error
		solto bool
		tardo time.Duration
	}
	ch := make(chan resultado, 1)
	inicio := time.Now()
	go func() {
		rel, err := Lock(path, 20*time.Second)
		ch <- resultado{rel: rel, err: err, solto: d.solto(), tardo: time.Since(inicio)}
	}()
	select {
	case r := <-ch:
		if r.err == nil {
			r.rel()
		}
		t.Fatalf("el padre volvio de Lock con el subproceso adentro (err=%v, solto=%v)", r.err, r.solto)
	case <-time.After(300 * time.Millisecond):
	}
	d.soltar()
	var r resultado
	select {
	case r = <-ch:
	case <-time.After(25 * time.Second):
		t.Fatal("el padre no consiguio el candado despues de que el subproceso lo solto")
	}
	if r.err != nil {
		t.Fatalf("el padre no consiguio el candado tras soltar el subproceso: %v", r.err)
	}
	defer r.rel()
	if !r.solto {
		t.Fatalf("el padre entro (a los %v) antes de que el subproceso soltara", r.tardo)
	}
	if err := d.esperarFin(); err != nil {
		t.Fatalf("el subproceso fallo: %v\n%s", err, d.out.String())
	}
}

// Hallazgo 20260925T041933_51d902, CA-349: un dueno que muere sin soltar no
// deja el candado tomado. El siguiente lo consigue dentro de wait, sin
// depender de la antiguedad del archivo (el archivo queda recien tocado).
func TestLcDuenoQueMuereNoDejaElCandado(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.lock")
	d := lcLanzar(t, "muere", path)
	d.esperarListo()
	err := d.esperarFin()
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 3 {
		t.Fatalf("el subproceso debia morir con codigo 3 sin soltar: %v\n%s", err, d.out.String())
	}
	ahora := time.Now()
	_ = os.Chtimes(path, ahora, ahora) // si el archivo quedo, es reciente
	inicio := time.Now()
	rel, err := Lock(path, 2*time.Second)
	if err != nil {
		t.Fatalf("un dueno muerto dejo el candado tomado: Lock(path, 2s) = %v tras %v", err, time.Since(inicio))
	}
	rel()
}

// Hallazgo 20260925T041933_51d902, CA-349: no hay nocion de "vencido". Un
// dueno vivo con el archivo del candado viejo (horas) sigue siendo dueno.
func TestLcDuenoVivoConArchivoViejoSigueSiendoDueno(t *testing.T) {
	viejo := time.Now().Add(-6 * time.Hour)
	t.Run("mismo proceso", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "item.lock")
		rel := lcTomar(t, path)
		defer rel()
		if err := os.Chtimes(path, viejo, viejo); err != nil {
			t.Fatalf("con el candado tomado, el archivo %s deberia existir: %v", path, err)
		}
		lcNoEntra(t, path, "dueno vivo en este proceso con archivo de hace 6h")
	})
	t.Run("otro proceso", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "item.lock")
		d := lcLanzar(t, "sostiene", path)
		d.esperarListo()
		if err := os.Chtimes(path, viejo, viejo); err != nil {
			t.Fatalf("con el candado tomado, el archivo %s deberia existir: %v", path, err)
		}
		lcNoEntra(t, path, "dueno vivo en otro proceso con archivo de hace 6h")
		d.soltar()
		if err := d.esperarFin(); err != nil {
			t.Fatalf("el subproceso fallo: %v\n%s", err, d.out.String())
		}
	})
}

// Hallazgo 20260925T041933_51d902, CA-349: la espera es acotada. Con el
// candado tomado, Lock(path, 50ms) vuelve en poco mas de 50ms con ErrLocked
// y no toma nada.
func TestLcEsperaAcotada(t *testing.T) {
	path := filepath.Join(t.TempDir(), "item.lock")
	rel := lcTomar(t, path)
	const wait = 50 * time.Millisecond
	inicio := time.Now()
	rel2, err := Lock(path, wait)
	demora := time.Since(inicio)
	if err == nil {
		rel2()
		rel()
		t.Fatal("Lock entro con el candado tomado")
	}
	if !errors.Is(err, ErrLocked) {
		rel()
		t.Fatalf("el error no cumple errors.Is(err, ErrLocked): %v", err)
	}
	if demora < wait-5*time.Millisecond || demora > wait+450*time.Millisecond {
		rel()
		t.Fatalf("Lock(path, %v) volvio a los %v; esperaba poco mas de %v", wait, demora, wait)
	}
	lcNoEntra(t, path, "tras un intento fallido el dueno sigue adentro")
	rel()
	rel3, err := Lock(path, time.Second)
	if err != nil {
		t.Fatalf("el intento fallido tomo algo: tras soltar el dueno, Lock = %v", err)
	}
	rel3()
}

// Hallazgo 20260925T041933_51d902, CA-349: Lock crea lo que haga falta,
// tambien los directorios del camino.
func TestLcCreaLoQueHagaFalta(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no", "existe", "item.lock")
	rel, err := Lock(path, time.Second)
	if err != nil {
		t.Fatalf("Lock sobre un directorio que no existe: %v", err)
	}
	lcNoEntra(t, path, "candado recien creado")
	rel()
}
