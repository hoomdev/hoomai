//go:build !windows

// Tests de regresion de la segunda ronda de la tarea endurecimiento-1 en
// boardcmd que necesitan un FIFO (mkfifo es de unix; el FIFO es el de
// internal/carreratest, que nadie escribe y que se suelta al final del
// test). Escritos desde el spec, el texto del hallazgo y el contrato de la
// correccion, SIN LEER LA IMPLEMENTACION. Los helpers llevan el prefijo e1b
// (los compartidos estan en hallazgos_endurecimiento1b_test.go).
//
//   - 20261022T202055_53c2af (high, risk): el tablero y el doctor abrian cada
//     .json de .hoom/reviews (y de .hoom/verdicts y .hoom/findings) sin
//     exigir un archivo regular: un FIFO con ese nombre bloqueaba Build y
//     Doctor (y el Studio) para siempre. Ahora vuelven en tiempo acotado; y
//     un FIFO con nombre de registro no es un registro valido: Doctor
//     devuelve error y Build lo lleva en Warnings (CA-442: un FIFO "nunca se
//     sigue ni se lee"; CA-369).
package boardcmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// e1bReloj es lo mas que pueden tardar Build y Doctor con el FIFO: el
// fixture es chico y sin el FIFO tardan menos de un segundo.
const e1bReloj = 10 * time.Second

// e1bConReloj corre f con reloj: si no vuelve, el test falla en vez de
// colgar la suite (el FIFO se suelta al final del test).
func e1bConReloj[T any](t *testing.T, ca, que string, f func() T) T {
	t.Helper()
	ch := make(chan T, 1)
	t0 := time.Now()
	go func() { ch <- f() }()
	select {
	case v := <-ch:
		t.Logf("%s: %s volvio en %s", ca, que, time.Since(t0).Round(time.Millisecond))
		return v
	case <-time.After(e1bReloj):
		t.Fatalf("%s: %s no volvio en %s: se quedo abriendo un FIFO de la evidencia", ca, que, e1bReloj)
	}
	var cero T
	return cero
}

type e1bDoctorRes struct {
	r   DoctorReport
	err error
}

type e1bBuildRes struct {
	b   Board
	err error
}

// CA-442, CA-369 (hallazgo 53c2af): un FIFO que nadie escribe en
// .hoom/reviews, .hoom/verdicts o .hoom/findings, con el nombre de un
// registro de hoom (en el lugar del registro valido de la tarjeta) o
// llamado x.json, no cuelga el tablero ni el doctor: Build y Doctor vuelven
// en tiempo acotado (un FIFO nunca se lee). Con el nombre de un registro:
// no es un registro valido, asi que Doctor devuelve error y el tablero se
// arma igual y lo lleva en Warnings. Los seis casos corren en paralelo.
func TestHallazgo_53c2af_CA442_FIFOEnLaEvidenciaNoCuelgaTableroNiDoctor(t *testing.T) {
	for _, f := range e1bFuentes() {
		for _, nombre := range []string{"registro", "x.json"} {
			t.Run(f.dir+"/"+nombre, func(t *testing.T) {
				t.Parallel()
				ca := "CA-442 (hallazgo 53c2af, FIFO " + nombre + " en " + f.dir + ")"
				root := e1bProyecto(t)
				ruta, id := f.crear(t, root)
				antes := e1bAvisos(t, ca, root)
				e1bDoctorSano(t, ca, root, "sin el FIFO")

				fifo := filepath.Join(root, f.dir, "x.json")
				if nombre == "registro" {
					if err := os.Remove(ruta); err != nil {
						t.Fatal(err)
					}
					fifo = ruta
				}
				carreratest.Fifo(t, fifo)
				if fi, err := os.Lstat(fifo); err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
					t.Fatalf("fixture: %s es un FIFO: %v %v", fifo, fi, err)
				}

				b := e1bConReloj(t, ca, "Build", func() e1bBuildRes {
					b, err := Build(root, "main", "high", e1Ahora)
					return e1bBuildRes{b, err}
				})
				d := e1bConReloj(t, ca, "Doctor", func() e1bDoctorRes {
					r, err := Doctor(root, "main", "high", e1Ahora)
					return e1bDoctorRes{r, err}
				})
				if b.err != nil {
					t.Errorf("%s: el tablero no falla por un FIFO en la evidencia: Build se arma igual: %v", ca, b.err)
				}
				if nombre != "registro" {
					return
				}
				if d.err == nil {
					t.Errorf("%s: un FIFO con el nombre del registro %s no es un registro valido: Doctor devuelve error; dio un reporte de %d problemas %v",
						ca, id, len(d.r.Problems), drIDs(d.r.Problems))
				}
				if b.err == nil {
					nuevos := e1Nuevos(antes, b.b.Warnings)
					if !e1bNombra(nuevos, id, f.dir) {
						t.Errorf("%s: el tablero lleva el FIFO %s en Warnings: un aviso nuevo que nombra %s o %s; avisos nuevos: %q (antes %q)",
							ca, id, id, filepath.Base(f.dir), nuevos, antes)
					}
				}
			})
		}
	}
}
