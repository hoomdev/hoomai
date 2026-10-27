//go:build !windows

// Tercera ronda de endurecimiento-1 en envelope (ver
// hallazgos_endurecimiento1c_test.go), el caso de la escritura colgada:
//
//   - 20261023T123540_1e0674 (resilience, CA-333): todos los reintentos de
//     cierre del proceso, incluso de proyectos distintos, corrian en un unico
//     worker que llamaba de a uno a la escritura, sin cota: si el sistema de
//     archivos bloqueaba UNA operacion, ningun otro cierre pendiente se
//     recuperaba. Ahora una escritura bloqueada no frena los cierres de
//     otros registros.
//
// Endurecimiento-2: la escritura se colgaba con un FIFO en .hoom/.gitignore;
// desde c6c300 ese FIFO ya no cuelga nada, y la escritura se sostiene con el
// gancho de tests antesDeEscribir (ver hallazgos_endurecimiento2_gancho_test.go),
// que corre justo antes de cada escritura de un registro. El test lo suelta y
// lo devuelve a nil al final.
package envelope

import (
	"testing"
	"time"
)

// CA-333 (hallazgo 1e0674): con la escritura del registro A (proyecto
// bloqueado) colgada, el cierre del registro B (otro proyecto) llega igual
// a disco. Cerrar(A) falla sin escritura y queda para reintentar; vuelve la
// escritura, pero el reintento de A (a 1 s) queda colgado en el gancho
// antesDeEscribir. Cerrar(B) tambien falla sin escritura, y la escritura de
// su proyecto vuelve recien cuando el reintento de A ya esta colgado. B
// llega a disco en menos de 10 s. Guarda: el reintento de A llego al gancho
// (si no, el caso no se inyecto, y que B llegue no probaria nada: hallazgo
// e4f7e0). Al final se suelta el gancho y se espera que A y B terminen.
func TestHallazgo_1e0674_CA333_UnaEscrituraBloqueadaNoFrenaLosCierresDeOtrosRegistros(t *testing.T) {
	const ca = "CA-333 (hallazgo 1e0674, una escritura bloqueada)"
	bloqueado, otro := t.TempDir(), t.TempDir()
	a := e1Rec(e1cID(700), "bloqueado")
	b := e1Rec(e1cID(701), "otro")
	e1Escribir(t, bloqueado, a)
	e1Escribir(t, otro, b)
	c := e2Colgar(t, nil)
	t.Cleanup(func() {
		// soltar la escritura colgada y esperar a que los dos cierres
		// terminen, antes de que se borren los directorios del test
		c.Soltar()
		if !e1bEsperar(15*time.Second, func() bool {
			ra, okA := e1EnDisco(bloqueado, a.ID)
			rb, okB := e1EnDisco(otro, b.ID)
			return okA && okB && ra.Status == StatusDeliverable && rb.Status == StatusDeliverable
		}) {
			ra, _ := e1EnDisco(bloqueado, a.ID)
			rb, _ := e1EnDisco(otro, b.ID)
			t.Logf("%s: al final, 15 s despues de soltar el gancho, A esta %q y B %q", ca, ra.Status, rb.Status)
		}
	})

	devolverA := e1SinEscritura(t, bloqueado, a.ID)
	t0 := time.Now()
	if err := Cerrar(bloqueado, e1Cerrado(a)); err == nil {
		t.Fatalf("%s: fixture: sin escritura Cerrar(A) devuelve el error, no nil", ca)
	}
	devolverA()
	c.Elegir(func(r, id string) bool { return r == bloqueado && id == a.ID })

	devolverB := e1SinEscritura(t, otro, b.ID)
	if err := Cerrar(otro, e1Cerrado(b)); err == nil {
		t.Fatalf("%s: fixture: sin escritura Cerrar(B) devuelve el error, no nil", ca)
	}
	// esperar a que el reintento de A se cuelgue en el gancho (a ~1 s; si
	// el trabajador esta en un tramo largo, hasta 40 s); recien entonces
	// vuelve la escritura de B
	if !e1bEsperar(40*time.Second, func() bool { return c.Llegaron(bloqueado, a.ID) > 0 }) {
		t.Fatalf("%s: fixture: en 40 s el reintento de A no llego al gancho antesDeEscribir: la escritura de A no quedo colgada y el caso no se inyecto", ca)
	}
	t.Logf("%s: %s despues de Cerrar(A), la escritura de A esta colgada en el gancho", ca, time.Since(t0).Round(100*time.Millisecond))
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
	if r, ok := e1EnDisco(bloqueado, a.ID); ok && r.Status != StatusRunning && c.Colgadas() > 0 {
		t.Errorf("%s: fixture: con la escritura de A colgada en el gancho, A no puede haber llegado a disco: %q", ca, r.Status)
	}
}
