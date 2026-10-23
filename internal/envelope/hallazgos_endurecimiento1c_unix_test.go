//go:build !windows

// Tercera ronda de endurecimiento-1 en envelope (ver
// hallazgos_endurecimiento1c_test.go), el caso que solo se arma en unix:
//
//   - 20261023T123540_1e0674 (resilience, CA-333): todos los reintentos de
//     cierre del proceso, incluso de proyectos distintos, corrian en un unico
//     worker que llamaba de a uno a la escritura, sin cota: si el sistema de
//     archivos bloqueaba UNA operacion, ningun otro cierre pendiente se
//     recuperaba. Ahora una escritura bloqueada no frena los cierres de
//     otros registros.
//
// Como se bloquea una escritura de forma deterministica en macOS y linux
// (observado de afuera, sin leer la implementacion): Write de un sobre
// completa .hoom/.gitignore y lo lee; si .hoom/.gitignore es un FIFO que
// nadie escribe, esa lectura se queda colgada hasta que alguien lo abre para
// escribir. El test tiene un lector colgado ahi mientras dura y lo suelta al
// final.
package envelope

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// e1cHayLector abre el FIFO para escribir sin bloquear: si alguien lo tiene
// abierto para leer (una escritura de un sobre colgada en el), lo devuelve
// abierto, y mientras siga abierto (sin escribir nada) esa lectura sigue
// colgada; si nadie lo lee, nil.
func e1cHayLector(fifo string) *os.File {
	f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil
	}
	return f
}

// CA-333 (hallazgo 1e0674): con la escritura del registro A (proyecto
// bloqueado) colgada, el cierre del registro B (otro proyecto) llega igual
// a disco. Cerrar(A) falla sin escritura y queda para reintentar; vuelve la
// escritura pero .hoom/.gitignore de su proyecto pasa a ser un FIFO, asi que
// el reintento de A (a 1 s) se queda colgado leyendolo. Cerrar(B) tambien
// falla sin escritura, y la escritura de su proyecto vuelve recien cuando el
// reintento de A ya esta colgado en el FIFO (hay un lector). B llega a disco
// en menos de 10 s. Al final se suelta el FIFO y se espera que A y B
// terminen.
func TestHallazgo_1e0674_CA333_UnaEscrituraBloqueadaNoFrenaLosCierresDeOtrosRegistros(t *testing.T) {
	const ca = "CA-333 (hallazgo 1e0674, una escritura bloqueada)"
	bloqueado, otro := t.TempDir(), t.TempDir()
	a := e1Rec(e1cID(700), "bloqueado")
	b := e1Rec(e1cID(701), "otro")
	e1Escribir(t, bloqueado, a)
	e1Escribir(t, otro, b)

	devolverA := e1SinEscritura(t, bloqueado, a.ID)
	t0 := time.Now()
	if err := Cerrar(bloqueado, e1Cerrado(a)); err == nil {
		t.Fatalf("%s: fixture: sin escritura Cerrar(A) devuelve el error, no nil", ca)
	}
	devolverA()
	gi := filepath.Join(bloqueado, ".hoom", ".gitignore")
	if err := os.Remove(gi); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(gi, 0o644); err != nil {
		t.Skipf("este sistema no deja crear un FIFO: %v", err)
	}
	var sostener *os.File
	t.Cleanup(func() {
		// soltar la lectura colgada (EOF) y esperar a que los dos cierres
		// terminen, antes de que se borren los directorios del test
		if sostener != nil {
			sostener.Close()
		}
		fin := time.Now().Add(15 * time.Second)
		for time.Now().Before(fin) {
			if f := e1cHayLector(gi); f != nil {
				f.Close()
			}
			ra, okA := e1EnDisco(bloqueado, a.ID)
			rb, okB := e1EnDisco(otro, b.ID)
			if okA && okB && ra.Status == StatusDeliverable && rb.Status == StatusDeliverable {
				time.Sleep(200 * time.Millisecond)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		ra, _ := e1EnDisco(bloqueado, a.ID)
		rb, _ := e1EnDisco(otro, b.ID)
		t.Logf("%s: al final, 15 s despues de soltar el FIFO, A esta %q y B %q", ca, ra.Status, rb.Status)
	})

	devolverB := e1SinEscritura(t, otro, b.ID)
	if err := Cerrar(otro, e1Cerrado(b)); err == nil {
		t.Fatalf("%s: fixture: sin escritura Cerrar(B) devuelve el error, no nil", ca)
	}
	// esperar a que el reintento de A se cuelgue en el FIFO (a ~1 s con un
	// backoff por cierre; si un worker de otro test esta en un tramo largo,
	// hasta 40 s); recien entonces vuelve la escritura de B
	// sin la escritura de A colgada el caso no se inyecto, y que B llegue
	// no probaria nada (hallazgo e4f7e0)
	if !e1bEsperar(40*time.Second, func() bool {
		sostener = e1cHayLector(gi)
		return sostener != nil
	}) {
		t.Fatalf("%s: fixture: en 40 s nadie leyo el FIFO %s: la escritura de A no quedo colgada y el caso no se inyecto", ca, gi)
	}
	t.Logf("%s: %s despues de Cerrar(A), la escritura de A esta colgada leyendo el FIFO %s", ca, time.Since(t0).Round(100*time.Millisecond), gi)
	if r, ok := e1EnDisco(otro, b.ID); !ok || r.Status != StatusRunning {
		t.Fatalf("%s: fixture: sin escritura, B sigue abierto en disco: %q %v", ca, r.Status, ok)
	}
	devolverB()
	tb := time.Now()

	llego := e1bEsperar(10*time.Second, func() bool {
		r, ok := e1EnDisco(otro, b.ID)
		return ok && r.Status == StatusDeliverable && !r.EndedAt.IsZero()
	})
	if !llego {
		r, _ := e1EnDisco(otro, b.ID)
		t.Errorf("%s: con la escritura del registro A colgada, el cierre del registro B (otro proyecto) llega igual a disco: 10 s despues de volver la escritura, B sigue %q en %q",
			ca, r.Status, r.Stage)
		return
	}
	t.Logf("%s: B llego a disco %s despues de volver la escritura", ca, time.Since(tb).Round(100*time.Millisecond))
	if r, ok := e1EnDisco(bloqueado, a.ID); ok && r.Status != StatusRunning && sostener != nil {
		t.Errorf("%s: fixture: con el FIFO sostenido, A no puede haber llegado a disco: %q", ca, r.Status)
	}
}
