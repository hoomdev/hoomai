//go:build linux

// Ronda 5 del test-writer de contencion-del-writer: regresion de la review
// delta de codex 28b8d3 (aclaracion 33). El Cancel de procesos.ArrancarCon
// vuelve enseguida con cualquier Term: nunca espera la entrega del SIGTERM.
// Un Term que tarda o se traba no cuelga Wait: lo acota el WaitDelay de quien
// arranco el comando (8daed0). Escrito desde el contrato (go doc), sin leer la
// implementacion.
package procesos_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/procesos"
)

func r5q(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// r5Aparece espera hasta plazo a que exista p.
func r5Aparece(p string, plazo time.Duration) bool {
	hasta := time.Now().Add(plazo)
	for time.Now().Before(hasta) {
		if _, err := os.Stat(p); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// ---------------------------------------------------------------- 8daed0

// CA-679 (id 8daed0; aclaracion 33): `sh -c 'sleep 30'` con un WaitDelay de
// 0.5 s, arrancado con procesos.ArrancarCon, gracia de 1 s y un Term que se
// traba (espera un canal que el test cierra recien al final). Al cancelar,
// Cancel no espera a Term, asi que el WaitDelay corre y Wait vuelve en menos
// de 3 s. Wait tiene un tope propio de 6 s: un Cancel que espera a Term es un
// fallo, no un cuelgue.
func TestR5_8daed0_CA679_UnTermQueSeTrabaNoCuelgaWait(t *testing.T) {
	const ca = "CA-679"
	const gracia = time.Second
	traba := make(chan struct{})
	var una sync.Once
	soltar := func() { una.Do(func() { close(traba) }) }
	t.Cleanup(soltar)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "sleep 30")
	cmd.WaitDelay = 500 * time.Millisecond
	term := procesos.Terminacion{
		Term:  func() bool { <-traba; return true },
		Matar: func() {},
	}
	if err := procesos.ArrancarCon(cmd, gracia, term); err != nil {
		t.Fatalf("%s: ArrancarCon: %v", ca, err)
	}
	pid := cmd.Process.Pid
	hecho := make(chan error, 1)
	go func() { hecho <- cmd.Wait() }()
	volvio := false
	defer func() {
		soltar()
		if !volvio {
			// el lider sigue sin cosechar: su numero de grupo no se reuso
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			select {
			case <-hecho:
			case <-time.After(5 * time.Second):
			}
		}
		procesos.Liberar(pid)
	}()
	desde := time.Now()
	cancel()
	var err error
	select {
	case err = <-hecho:
		volvio = true
	case <-time.After(6 * time.Second):
	}
	dur := time.Since(desde)
	if !volvio {
		t.Fatalf("%s: un Term trabado no cuelga Wait: a los %s de cancelar Wait no habia vuelto (el Cancel no puede esperar a Term; lo acota el WaitDelay de %s)", ca, dur, cmd.WaitDelay)
	}
	if dur >= 3*time.Second {
		t.Fatalf("%s: con un Term trabado, Wait vuelve acotado por el WaitDelay de %s: volvio a los %s de cancelar (quiero menos de 3 s; %v)", ca, cmd.WaitDelay, dur, err)
	}
}

// CA-679 (id 8daed0; aclaracion 33), control: un Term que manda SIGTERM al
// grupo del comando y vuelve true enseguida, gracia de 1 s y WaitDelay de la
// gracia + 5 s. El comando pone `trap 'echo term > <marca>; exit 0' TERM` y
// despues deja arranco; se cancela cuando arranco ya esta. La marca aparece y
// Wait vuelve dentro de la gracia mas un margen de 2 s.
func TestR5_8daed0_CA679_UnTermQueVuelveEnseguida(t *testing.T) {
	const ca = "CA-679"
	const gracia = time.Second
	const margen = 2 * time.Second
	dir := t.TempDir()
	marca := filepath.Join(dir, "term")
	arranco := filepath.Join(dir, "arranco")
	script := "trap 'echo term > " + r5q(marca) + "; exit 0' TERM\n" +
		": > " + r5q(arranco) + "\n" +
		"while :; do sleep 0.05; done\n"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
	cmd.WaitDelay = gracia + 5*time.Second
	term := procesos.Terminacion{
		Term: func() bool {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
			return true
		},
		Matar: func() {},
	}
	if err := procesos.ArrancarCon(cmd, gracia, term); err != nil {
		t.Fatalf("%s: ArrancarCon: %v", ca, err)
	}
	pid := cmd.Process.Pid
	hecho := make(chan error, 1)
	go func() { hecho <- cmd.Wait() }()
	volvio := false
	defer func() {
		if !volvio {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			select {
			case <-hecho:
			case <-time.After(5 * time.Second):
			}
		}
		procesos.Liberar(pid)
	}()
	if !r5Aparece(arranco, 10*time.Second) {
		t.Fatalf("%s: fixture: el comando no puso su trap en 10 s", ca)
	}
	desde := time.Now()
	cancel()
	var err error
	select {
	case err = <-hecho:
		volvio = true
	case <-time.After(15 * time.Second):
	}
	dur := time.Since(desde)
	if !volvio {
		t.Fatalf("%s: Wait vuelve en un tiempo acotado: a los %s de cancelar no habia vuelto", ca, dur)
	}
	if _, e := os.Stat(marca); e != nil {
		t.Fatalf("%s: un Term que entrega el SIGTERM al grupo: el trap deja su marca (Wait volvio a los %s de cancelar: %v)", ca, dur, err)
	}
	if dur > gracia+margen {
		t.Fatalf("%s: Wait vuelve dentro de la gracia de %s mas %s: volvio a los %s de cancelar", ca, gracia, margen, dur)
	}
}
