//go:build linux

// Ronda 4 del test-writer de contencion-del-writer: regresion de la review
// delta de codex 707220 (aclaracion 32). El WaitDelay de os/exec de un
// comando encerrado nunca mata a bwrap antes de que el comando reciba su
// SIGTERM y su gracia completa, aunque quien lo arranca fije un WaitDelay
// chico (el guardado: gracia + 1 s; un gate: gracia + 5 s) y aunque el paso
// tarde en lanzar el comando. El arranque lento se fuerza con el mismo bwrap
// falso de 75396e (ronda 3), que ademas deja una marca al despertar. Sin
// bwrap fallan (aclaracion 1).
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

// r4Script es el comando encerrado: su trap de TERM duerme 0.6 s y deja la
// marca term; la marca arranco va despues del trap (si esta, el trap ya
// estaba puesto).
const r4Script = "trap 'sleep 0.6; echo term > \"$PWD/term\"; exit 0' TERM\n" +
	": > \"$PWD/arranco\"\n" +
	"while :; do sleep 0.05; done\n"

// r4BwrapLento es el bwrap lento de r3BwrapLento: al frente del PATH, en un
// lanzamiento (con --info-fd) deja la marca lanzado, duerme demora, deja la
// marca desperto y recien ahi ejecuta el bwrap de verdad. Sin desperto, el
// bwrap murio dormido: el comando nunca se lanzo.
func r4BwrapLento(t *testing.T, demora string) (lanzado, desperto string) {
	t.Helper()
	deVerdad, err := exec.LookPath("bwrap")
	if err != nil {
		t.Fatalf("bwrap no esta en el PATH (aclaracion 1): %v", err)
	}
	dir := cdwDir(t)
	lanzado = filepath.Join(dir, "lanzado")
	desperto = filepath.Join(dir, "desperto")
	script := "#!/bin/sh\n" +
		"case \" $* \" in *\" --info-fd \"*) : > " + r1q(lanzado) + "; sleep " + demora + "; : > " + r1q(desperto) + " ;; esac\n" +
		"exec " + r1q(deVerdad) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "bwrap"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return lanzado, desperto
}

// r4Esperar corre cmd.Wait con un tope: volvio es false si no volvio a
// tiempo.
func r4Esperar(cmd *exec.Cmd, tope time.Duration) (volvio bool, err error) {
	hecho := make(chan error, 1)
	go func() { hecho <- cmd.Wait() }()
	select {
	case e := <-hecho:
		return true, e
	case <-time.After(tope):
		return false, nil
	}
}

// ---------------------------------------------------------------- c85a2a

// CA-679 y CA-684 (id c85a2a, tambien f60133; aclaracion 32): un comando
// encerrado con un WaitDelay chico (la gracia de 1 s + 0.5 s) cuyo bwrap
// duerme 2 s en el arranque, mas que ese WaitDelay, y que se cancela
// enseguida (apenas el bwrap empezo a dormir). El WaitDelay no mata a bwrap
// dormido (la marca desperto aparece), el comando recibe su SIGTERM y su
// gracia completa (su trap de 0.6 s deja la marca term) y Wait vuelve. Dos
// vueltas: una vuelta en que el comando no llego a poner su trap (no dejo
// arranco) queda en el log, pero al menos una tiene que probarlo.
func TestR4_c85a2a_CA679_UnWaitDelayChicoNoMataAlBwrapAntesDeLaGracia(t *testing.T) {
	const ca = "CA-679"
	const gracia = time.Second
	const espera = gracia + 500*time.Millisecond // el WaitDelay chico
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
		desde := time.Now()
		cancel()
		if err := <-arranco; err != nil {
			t.Logf("%s: %s: Arrancar cancelado antes de que el comando aparezca: %v: nada que probar", ca, que, err)
			continue
		}
		volvio, err := r4Esperar(cmd, 15*time.Second)
		dur := time.Since(desde)
		if !volvio {
			t.Fatalf("%s: %s: Wait vuelve en un tiempo acotado: a los %s de cancelar no habia vuelto", ca, que, dur)
		}
		encierro.Liberar(cmd)
		if _, ok := cdwLeer(desperto); !ok {
			t.Fatalf("%s: %s: el WaitDelay de %s mato a bwrap dormido, antes de que lanzara el comando: Wait volvio a los %s de cancelar (%v) y el bwrap lento duerme 2 s; el comando tiene que recibir su SIGTERM y su gracia de %s completa (aclaracion 32)", ca, que, espera, dur, err, gracia)
		}
		if _, ok := cdwLeer(filepath.Join(v.Dir, "arranco")); !ok {
			t.Logf("%s: %s: el comando no llego a poner su trap (termino a los %s de cancelar): nada que probar en esta vuelta", ca, que, dur)
			continue
		}
		if _, ok := cdwLeer(filepath.Join(v.Dir, "term")); !ok {
			t.Fatalf("%s: %s: con un WaitDelay de %s, el comando que aparecio tarde recibe su SIGTERM y su gracia de %s completa: su trap de 0.6 s no dejo la marca (Wait volvio a los %s de cancelar: %v; aclaracion 32)", ca, que, espera, gracia, dur, err)
		}
		t.Logf("%s: %s: probada: Wait volvio a los %s de cancelar", ca, que, dur)
		probadas++
	}
	if probadas == 0 {
		t.Fatalf("%s: ninguna vuelta llego a probar la gracia con un WaitDelay chico (en ninguna el comando puso su trap): ver el log", ca)
	}
}

// CA-679 (id c85a2a; aclaracion 32), control: el mismo comando con el mismo
// WaitDelay chico, sin bwrap lento. Se cancela cuando el comando ya puso su
// trap (dejo arranco): la marca term aparece y Wait vuelve dentro de la
// gracia mas un margen de 2 s.
func TestR4_c85a2a_CA679_UnWaitDelayChicoSinArranqueLento(t *testing.T) {
	const ca = "CA-679"
	const gracia = time.Second
	const espera = gracia + 500*time.Millisecond
	const margen = 2 * time.Second
	info := cdwInfo(t, ca)
	tok := cdwToken()
	t.Cleanup(func() { cdwMatar(tok) })
	v := cdwVistaSimple(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", r4Script, tok)
	cmd.Dir = v.Dir
	cmd.WaitDelay = espera
	if err := encierro.Envolver(cmd, encierro.Plan{Info: info, Vista: v}); err != nil {
		t.Fatalf("%s: Envolver: %v", ca, err)
	}
	if err := encierro.Arrancar(cmd, gracia); err != nil {
		t.Fatalf("%s: Arrancar: %v", ca, err)
	}
	if !r3Aparece(filepath.Join(v.Dir, "arranco"), 10*time.Second) {
		cancel()
		if volvio, _ := r4Esperar(cmd, 15*time.Second); volvio {
			encierro.Liberar(cmd)
		}
		t.Fatalf("%s: fixture: el comando encerrado no arranco en 10 s", ca)
	}
	desde := time.Now()
	cancel()
	volvio, err := r4Esperar(cmd, 15*time.Second)
	dur := time.Since(desde)
	if !volvio {
		t.Fatalf("%s: Wait vuelve en un tiempo acotado: a los %s de cancelar no habia vuelto", ca, dur)
	}
	encierro.Liberar(cmd)
	if _, ok := cdwLeer(filepath.Join(v.Dir, "term")); !ok {
		t.Fatalf("%s: con un WaitDelay de %s y sin arranque lento, el trap de 0.6 s deja su marca dentro de la gracia de %s (Wait volvio a los %s de cancelar: %v)", ca, espera, gracia, dur, err)
	}
	if dur > gracia+margen {
		t.Fatalf("%s: Wait vuelve dentro de la gracia de %s mas %s: volvio a los %s de cancelar", ca, gracia, margen, dur)
	}
}
