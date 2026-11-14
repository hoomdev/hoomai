//go:build linux

// Ronda 5 del test-writer de contencion-del-writer: regresion de la review
// delta de codex 28b8d3 (aclaracion 33). Para un comando encerrado,
// encierro.Arrancar sube un WaitDelay distinto de cero hasta cubrir la espera
// maxima de la entrega del SIGTERM, mas la gracia, mas un margen, contado
// desde el fin del contexto (lo que documenta os/exec). Con el bwrap lento de
// r4 (duerme 2 s y deja desperto), la marca term del trap llega antes de
// t0 + cmd.WaitDelay (e9c31c). Sin bwrap falla (aclaracion 1).
package encierro_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
)

// ---------------------------------------------------------------- e9c31c

// CA-679 y CA-684 (id e9c31c; aclaraciones 32 y 33): el caso de c85a2a
// (WaitDelay de 1.5 s, gracia de 1 s, r4Script, bwrap que duerme 2 s) con
// W = cmd.WaitDelay leido despues de que encierro.Arrancar volvio, y el
// contexto cancelado en t0, apenas aparece lanzado. La marca term aparece,
// Wait vuelve, y la marca (su mtime) es anterior a t0 + W: el comando tuvo su
// SIGTERM y su gracia dentro del WaitDelay contado desde el fin del contexto.
// Dos vueltas: una en que el comando no llego a poner su trap queda en el log,
// pero al menos una tiene que probarlo.
func TestR5_e9c31c_CA679_LaMarcaTermLlegaAntesDeT0MasElWaitDelay(t *testing.T) {
	const ca = "CA-679"
	const gracia = time.Second
	const espera = gracia + 500*time.Millisecond // el WaitDelay que pone quien arranca
	info := cdwInfo(t, ca)
	lanzado, desperto := r4BwrapLento(t, "2")
	probadas := 0
	for vuelta := 0; vuelta < 2; vuelta++ {
		que := "vuelta " + strconv.Itoa(vuelta)
		_ = os.Remove(lanzado)
		_ = os.Remove(desperto)
		tok := cdwToken()
		t.Cleanup(func() { cdwMatar(tok) })
		v := cdwVistaSimple(t)
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", r4Script, tok)
		cmd.Dir = v.Dir
		cmd.WaitDelay = espera
		if err := encierro.Envolver(cmd, encierro.Plan{Info: info, Vista: v}); err != nil {
			cancel()
			t.Fatalf("%s: %s: Envolver: %v", ca, que, err)
		}
		arranco := make(chan error, 1)
		go func() { arranco <- encierro.Arrancar(cmd, gracia) }()
		if !r3Aparece(lanzado, 10*time.Second) {
			cancel()
			<-arranco
			t.Fatalf("%s: %s: fixture: no se pudo forzar el arranque lento (el bwrap del PATH no corrio con --info-fd)", ca, que)
		}
		t0 := time.Now()
		cancel()
		if err := <-arranco; err != nil {
			t.Logf("%s: %s: Arrancar cancelado antes de que el comando aparezca: %v: nada que probar", ca, que, err)
			continue
		}
		w := cmd.WaitDelay // despues de que Arrancar volvio
		volvio, err := r4Esperar(cmd, 15*time.Second)
		dur := time.Since(t0)
		if !volvio {
			t.Fatalf("%s: %s: Wait vuelve en un tiempo acotado: a los %s de cancelar no habia vuelto (WaitDelay %s)", ca, que, dur, w)
		}
		encierro.Liberar(cmd)
		if _, ok := cdwLeer(desperto); !ok {
			t.Fatalf("%s: %s: el WaitDelay (%s) mato a bwrap dormido, antes de que lanzara el comando: Wait volvio a los %s de cancelar (%v); aclaracion 32", ca, que, w, dur, err)
		}
		if _, ok := cdwLeer(filepath.Join(v.Dir, "arranco")); !ok {
			t.Logf("%s: %s: el comando no llego a poner su trap (termino a los %s de cancelar): nada que probar en esta vuelta", ca, que, dur)
			continue
		}
		st, errMarca := os.Stat(filepath.Join(v.Dir, "term"))
		if errMarca != nil {
			t.Fatalf("%s: %s: el comando que aparecio tarde recibe su SIGTERM y su gracia de %s: su trap de 0.6 s no dejo la marca (Wait volvio a los %s de cancelar, WaitDelay %s: %v)", ca, que, gracia, dur, w, err)
		}
		llego := st.ModTime().Sub(t0) // reloj de pared, como la mtime
		if !st.ModTime().Before(t0.Add(w)) {
			t.Fatalf("%s: %s: la marca term llego a los %s de cancelar, no antes de t0 + WaitDelay (%s): el WaitDelay que deja encierro.Arrancar cubre la espera de la entrega del SIGTERM, la gracia de %s y un margen, contado desde el fin del contexto (aclaracion 33)", ca, que, llego, w, gracia)
		}
		t.Logf("%s: %s: probada: la marca llego a los %s de cancelar, WaitDelay %s, Wait volvio a los %s", ca, que, llego, w, dur)
		probadas++
	}
	if probadas == 0 {
		t.Fatalf("%s: ninguna vuelta llego a probar la marca contra t0 + WaitDelay (en ninguna el comando puso su trap): ver el log", ca)
	}
}
