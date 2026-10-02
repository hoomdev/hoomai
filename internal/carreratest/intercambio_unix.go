//go:build !windows

package carreratest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

// Forma crea en p (una ruta que todavia no existe, en un directorio temporal
// del test) la entrada que un intercambiador pone despues en su destino.
type Forma func(p string) error

// Regular es un archivo regular con contenido.
func Regular(contenido []byte) Forma {
	return func(p string) error { return os.WriteFile(p, contenido, 0o644) }
}

// Symlink es un symlink a destino (el texto tal cual: absoluto o relativo al
// directorio donde termina el enlace).
func Symlink(destino string) Forma {
	return func(p string) error { return os.Symlink(destino, p) }
}

// EnlaceDuro es otro nombre del mismo origen (un enlace duro): con un FIFO,
// cualquier lectura colgada en el destino se suelta por la ruta de origen.
func EnlaceDuro(origen string) Forma {
	return func(p string) error { return os.Link(origen, p) }
}

// detenedor arma la funcion que para la goroutine de un intercambiador: la
// primera llamada la para y espera que termine, todas devuelven cuantos
// cambios hizo. Tambien la registra para el final del test.
func detenedor(t testing.TB, parar *atomic.Bool, wg *sync.WaitGroup, cambios *atomic.Int64) func() int {
	var una sync.Once
	stop := func() int {
		una.Do(func() {
			parar.Store(true)
			wg.Wait()
		})
		return int(cambios.Load())
	}
	t.Cleanup(func() { stop() })
	return stop
}

// Intercambiar pone en destino, sin parar y con rename atomico, cada forma
// por turno (la del cambio i es formas[i%len(formas)]). Un rename reemplaza
// un archivo o un symlink sin que la ruta falte nunca; un directorio no se
// reemplaza asi (para eso, IntercambiarDir). Las formas se arman en un
// directorio temporal del test: en el mismo disco que destino si destino
// tambien vive en uno, que es lo que hace atomico el rename.
//
// Devuelve la funcion que lo para (se puede llamar varias veces y tambien
// corre al final del test) y dice cuantos cambios hizo. Un cambio que falla
// es un error del fixture (t.Errorf) y para la goroutine.
func Intercambiar(t testing.TB, destino string, formas ...Forma) func() int {
	t.Helper()
	if len(formas) == 0 {
		t.Fatalf("fixture: Intercambiar %s sin formas", destino)
	}
	escenario := t.TempDir()
	var parar atomic.Bool
	var cambios atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !parar.Load(); i++ {
			p := filepath.Join(escenario, fmt.Sprintf("e%d", i))
			err := formas[i%len(formas)](p)
			if err == nil {
				err = os.Rename(p, destino)
			}
			if err != nil {
				t.Errorf("fixture: el intercambiador no pudo cambiar %s: %v", destino, err)
				return
			}
			cambios.Add(1)
		}
	}()
	return detenedor(t, &parar, &wg, &cambios)
}

// RegularYFifo es la carrera del hallazgo 98faf2: destino cambia, sin parar,
// entre un archivo regular con contenido regular, un symlink a fifo, el
// mismo archivo regular y el fifo mismo (un enlace duro: cualquier lectura
// colgada se suelta por la ruta de fifo, ver Fifo).
func RegularYFifo(t testing.TB, destino, fifo string, regular []byte) func() int {
	t.Helper()
	return Intercambiar(t, destino, Regular(regular), Symlink(fifo), Regular(regular), EnlaceDuro(fifo))
}

// IntercambiarDir es la carrera de los hallazgos 545ddc y a081cb: cambia sin
// parar el directorio destino por cada forma, por turno (un symlink a otro
// directorio, a un FIFO o afuera del arbol; el FIFO mismo, con EnlaceDuro;
// un archivo regular), y lo devuelve. Un rename no pone otra cosa sobre un
// directorio, asi que cada vuelta es un baile de cuatro renames: aparta el
// directorio, pone la forma, la saca y devuelve el directorio. destino es
// entonces, por turno, el directorio, nada, la forma, nada, el directorio. El
// que vuelve es siempre el mismo (el mismo inodo, con lo que tenga adentro).
//
// Devuelve la funcion que lo para —con el directorio de vuelta en destino—
// y dice cuantas vueltas dio. Un paso que falla es un error del fixture
// (t.Errorf) y para la goroutine, despues de intentar devolver el
// directorio.
func IntercambiarDir(t testing.TB, destino string, formas ...Forma) func() int {
	t.Helper()
	if len(formas) == 0 {
		t.Fatalf("fixture: IntercambiarDir %s sin formas", destino)
	}
	if fi, err := os.Lstat(destino); err != nil || !fi.IsDir() {
		t.Fatalf("fixture: IntercambiarDir: %s es un directorio: %v", destino, err)
	}
	escenario := t.TempDir()
	apartado := filepath.Join(escenario, "apartado")
	var parar atomic.Bool
	var vueltas atomic.Int64
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; !parar.Load(); i++ {
			l := filepath.Join(escenario, fmt.Sprintf("l%d", i))
			if err := formas[i%len(formas)](l); err != nil {
				t.Errorf("fixture: el intercambiador no pudo armar la forma %d para %s: %v", i%len(formas), destino, err)
				return
			}
			if err := os.Rename(destino, apartado); err != nil {
				t.Errorf("fixture: el intercambiador no pudo apartar %s: %v", destino, err)
				return
			}
			poner := os.Rename(l, destino)
			var sacar error
			if poner == nil {
				sacar = os.Rename(destino, l)
			}
			devolver := os.Rename(apartado, destino)
			if err := errors.Join(poner, sacar, devolver); err != nil {
				t.Errorf("fixture: el intercambiador no pudo cambiar %s por la forma %d y devolverlo: %v", destino, i%len(formas), err)
				return
			}
			_ = os.Remove(l)
			vueltas.Add(1)
		}
	}()
	return detenedor(t, &parar, &wg, &vueltas)
}
