// Tests de regresion de la tarea endurecimiento-2 en envelope que cuelgan
// una escritura con el gancho de tests antesDeEscribir. Escritos desde el
// texto del hallazgo y el contrato del gancho (su go doc), SIN LEER LA
// IMPLEMENTACION. Los helpers de este archivo llevan el prefijo e2 y los
// usan tambien los tests de 1e0674, da912f, ab66c7, 4647ba y 22806c, que
// antes colgaban la escritura con un FIFO en .hoom/.gitignore (c6c300 hizo
// que ese FIFO ya no cuelgue nada).
//
// El gancho: antesDeEscribir, si no es nil, corre justo antes de CADA
// escritura de un registro (el primer intento de Write o Cerrar y cada
// reintento del trabajador), con el proyecto y el id. El trabajador es
// global y sobrevive al test: quien lo pone lo devuelve a nil al terminar.
//
//   - 20261024T141937_332b11 (medium, CA-333): el reintento miraba que el
//     registro existiera y despues lo escribia por nombre; un borrado entre
//     las dos cosas hacia que el reintento recreara el registro que el
//     operador habia borrado. Ahora un registro borrado no reaparece.
package envelope

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// e2Escritura es una escritura de un registro: su proyecto y su id.
type e2Escritura struct{ root, id string }

// e2Colgador es el gancho antesDeEscribir puesto por un test: cuelga las
// escrituras que elige hasta Soltar, y anota cuales llegaron.
type e2Colgador struct {
	mu       sync.Mutex
	elegir   func(root, id string) bool
	llegadas map[e2Escritura]int
	suelta   chan struct{}
	una      sync.Once
	colgadas atomic.Int64
}

// e2Colgar pone el gancho con elegir (nil: no cuelga nada, hasta Elegir).
// Al final del test lo suelta, lo devuelve a nil y espera a que no quede
// ninguna escritura colgada en el. Falla si otro test lo dejo puesto.
func e2Colgar(t *testing.T, elegir func(root, id string) bool) *e2Colgador {
	t.Helper()
	if antesDeEscribir.Load() != nil {
		t.Fatal("fixture: el gancho antesDeEscribir ya esta puesto: otro test no lo devolvio a nil")
	}
	c := &e2Colgador{elegir: elegir, llegadas: map[e2Escritura]int{}, suelta: make(chan struct{})}
	f := func(root, id string) {
		c.mu.Lock()
		el := c.elegir
		c.mu.Unlock()
		if el == nil || !el(root, id) {
			return
		}
		select {
		case <-c.suelta:
			return
		default:
		}
		c.colgadas.Add(1)
		defer c.colgadas.Add(-1)
		c.mu.Lock()
		c.llegadas[e2Escritura{root, id}]++
		c.mu.Unlock()
		<-c.suelta
	}
	antesDeEscribir.Store(&f)
	t.Cleanup(func() {
		c.Soltar()
		if !e1bEsperar(10*time.Second, func() bool { return c.colgadas.Load() == 0 }) {
			t.Errorf("fixture: 10 s despues de soltar el gancho siguen %d escrituras colgadas en el", c.colgadas.Load())
		}
	})
	return c
}

// Elegir cambia que escrituras cuelga el gancho.
func (c *e2Colgador) Elegir(elegir func(root, id string) bool) {
	c.mu.Lock()
	c.elegir = elegir
	c.mu.Unlock()
}

// Soltar devuelve el gancho a nil y suelta todas las escrituras colgadas.
func (c *e2Colgador) Soltar() {
	c.una.Do(func() {
		antesDeEscribir.Store(nil)
		close(c.suelta)
	})
}

// Llegaron es cuantas escrituras de root (de cualquier id si id es "")
// llegaron al gancho y se colgaron.
func (c *e2Colgador) Llegaron(root, id string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for e, k := range c.llegadas {
		if e.root == root && (id == "" || e.id == id) {
			n += k
		}
	}
	return n
}

// Colgadas es cuantas escrituras estan colgadas en el gancho ahora.
func (c *e2Colgador) Colgadas() int { return int(c.colgadas.Load()) }

// CA-333 (hallazgo 332b11): el reintento de un cierre pendiente no resucita
// un registro borrado. Cerrar falla (sin escritura) y queda pendiente; vuelve
// la escritura y el reintento (a 1 s) queda sostenido por el gancho justo
// antes de publicar, despues de haber mirado el registro. Ahi se borra el
// registro (o .hoom/envelopes entero) y se suelta el reintento: durante 4.5 s
// (ese reintento y los que vengan) el registro no reaparece. Guarda: el
// reintento llego al gancho (si no, el caso no se inyecto).
func TestHallazgo_332b11_CA333_ElReintentoNoResucitaUnRegistroBorrado(t *testing.T) {
	const ca = "CA-333 (hallazgo 332b11, registro borrado durante el reintento)"
	for _, cs := range []struct {
		nombre string
		dir    bool
	}{
		{"se borra el registro", false},
		{"se borra .hoom/envelopes", true},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "proyecto")
			rec := e1Rec(e1IDPropio, "propio")
			e1Escribir(t, root, rec)
			c := e2Colgar(t, nil)
			devolver := e1SinEscritura(t, root, rec.ID)
			if err := Cerrar(root, e1Cerrado(rec)); err == nil {
				t.Fatalf("%s: fixture: con .hoom/envelopes sin escritura Cerrar devuelve el error, no nil", ca)
			}
			devolver()
			c.Elegir(func(r, id string) bool { return r == root && id == rec.ID })
			if !e1bEsperar(10*time.Second, func() bool { return c.Llegaron(root, rec.ID) > 0 }) {
				t.Fatalf("%s: fixture: en 10 s el reintento del cierre pendiente no llego al gancho antesDeEscribir: el caso no se inyecto", ca)
			}
			reg := filepath.Join(root, ".hoom", DirName, rec.ID+".json")
			if _, err := os.Lstat(reg); err != nil {
				t.Fatalf("%s: fixture: con el reintento sostenido antes de publicar, el registro sigue en disco: %v", ca, err)
			}
			borrado := reg
			if cs.dir {
				borrado = filepath.Dir(reg)
			}
			if err := os.RemoveAll(borrado); err != nil {
				t.Fatal(err)
			}
			c.Soltar()

			t0 := time.Now()
			for time.Since(t0) < 4500*time.Millisecond {
				if _, err := os.Lstat(reg); err == nil {
					r, _ := e1EnDisco(root, rec.ID)
					t.Fatalf("%s: %s despues de soltar el reintento, el registro borrado reaparecio (%q en %q): el reintento lo recreo",
						ca, time.Since(t0).Round(10*time.Millisecond), r.Status, r.Stage)
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}
