//go:build linux

// Ronda 3 del test-writer de contencion-del-writer: regresiones de la review
// delta de codex sobre la cancelacion de un comando encerrado (aclaracion
// 31): nunca demora a los demas grupos de procesos (aacb4e) y la gracia
// cuenta desde el SIGTERM aunque el comando aparezca tarde (75396e). El
// arranque lento se fuerza con un bwrap falso al frente del PATH que, cuando
// lo llaman con --info-fd (un lanzamiento, no la sonda), deja una marca,
// duerme y recien despues ejecuta el bwrap de verdad. Sin bwrap fallan
// (aclaracion 1).
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
	"github.com/hoomdev/hoomai/internal/procesos"
)

// r3BwrapLento pone al frente del PATH un bwrap que, en un lanzamiento (con
// --info-fd), deja la marca que devuelve y duerme demora antes de ejecutar
// el bwrap de verdad.
func r3BwrapLento(t *testing.T, demora string) (marca string) {
	t.Helper()
	deVerdad, err := exec.LookPath("bwrap")
	if err != nil {
		t.Fatalf("bwrap no esta en el PATH (aclaracion 1): %v", err)
	}
	dir := cdwDir(t)
	marca = filepath.Join(dir, "lanzado")
	script := "#!/bin/sh\n" +
		"case \" $* \" in *\" --info-fd \"*) : > " + r1q(marca) + "; sleep " + demora + " ;; esac\n" +
		"exec " + r1q(deVerdad) + " \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "bwrap"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marca
}

// r3Aparece espera hasta plazo a que exista p.
func r3Aparece(p string, plazo time.Duration) bool {
	hasta := time.Now().Add(plazo)
	for time.Now().Before(hasta) {
		if _, err := os.Stat(p); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// ---------------------------------------------------------------- aacb4e

// CA-679 (id aacb4e, medium; aclaracion 31; de mejor esfuerzo): mientras un
// comando encerrado se esta cancelando (cancelado con su bwrap todavia
// dormido en el arranque: su comando no aparecio), otro comando arrancado
// con procesos.ArrancarConGracia o con procesos.Arrancar arranca en menos de
// 1 s, y su terminacion (cancelar, Wait y procesos.Liberar) tambien tarda
// menos de 1 s: no esperan a esa cancelacion. El bwrap lento duerme 3 s.
func TestR3_aacb4e_CA679_CancelarUnEncerradoNoDemoraALosDemas(t *testing.T) {
	const ca = "CA-679"
	info := cdwInfo(t, ca)
	marca := r3BwrapLento(t, "3")
	tok := cdwToken()
	t.Cleanup(func() { cdwMatar(tok) })
	v := cdwVistaSimple(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", "trap 'exit 0' TERM\nwhile :; do sleep 0.05; done\n", tok)
	cmd.Dir = v.Dir
	cmd.WaitDelay = 20 * time.Second
	if err := encierro.Envolver(cmd, encierro.Plan{Info: info, Vista: v}); err != nil {
		t.Fatalf("%s: Envolver: %v", ca, err)
	}
	arranco := make(chan error, 1)
	go func() { arranco <- encierro.Arrancar(cmd, 2*time.Second) }()
	if !r3Aparece(marca, 10*time.Second) {
		t.Logf("%s: no se pudo forzar el arranque lento: el bwrap del PATH no corrio con --info-fd", ca)
	}
	cancel()
	time.Sleep(50 * time.Millisecond) // que la cancelacion empiece
	for _, conGracia := range []bool{true, false} {
		que := "procesos.Arrancar"
		if conGracia {
			que = "procesos.ArrancarConGracia"
		}
		ctx2, cancel2 := context.WithCancel(context.Background())
		otro := exec.CommandContext(ctx2, "sleep", "30")
		desde := time.Now()
		var err error
		if conGracia {
			err = procesos.ArrancarConGracia(otro, 2*time.Second)
		} else {
			err = procesos.Arrancar(otro)
		}
		if err != nil {
			cancel2()
			t.Fatalf("%s: %s: %v", ca, que, err)
		}
		if d := time.Since(desde); d > time.Second {
			t.Errorf("%s: %s no espera a la cancelacion del encerrado: tardo %s", ca, que, d)
		}
		desde = time.Now()
		cancel2()
		_ = otro.Wait()
		procesos.Liberar(otro.Process.Pid)
		if d := time.Since(desde); d > time.Second {
			t.Errorf("%s: la terminacion del comando de %s no espera a la cancelacion del encerrado: tardo %s", ca, que, d)
		}
	}
	select {
	case err := <-arranco:
		if err == nil {
			_ = cmd.Wait()
			encierro.Liberar(cmd)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("%s: el encerrado cancelado termina en un tiempo acotado", ca)
	}
	cdwNadieVivo(t, ca, "el encerrado cancelado", tok, 10*time.Second)
}

// ---------------------------------------------------------------- 75396e

// CA-679 (id 75396e, medium; aclaracion 31): un comando encerrado que
// aparece tarde (su bwrap duerme 1.5 s en el arranque y se cancela antes de
// que despierte) recibe su SIGTERM y la gracia completa de 2 s desde ahi:
// su trap, que duerme 1 s, deja su marca. Si la gracia contara desde la
// cancelacion, el SIGKILL llegaria a los 2 s, en medio del trap. Si el
// comando no llega a aparecer (no dejo su marca de arranque, que va despues
// del trap), no hay nada que probar en esa vuelta: queda en el log. Dos
// vueltas (no se escribe el camino real de 10 s).
func TestR3_75396e_CA679_UnComandoQueApareceTardeTieneSuGraciaCompleta(t *testing.T) {
	const ca = "CA-679"
	info := cdwInfo(t, ca)
	marca := r3BwrapLento(t, "1.5")
	script := "trap 'sleep 1; echo term > \"$PWD/term\"; exit 0' TERM\n" +
		": > \"$PWD/arranco\"\n" +
		"while :; do sleep 0.05; done\n"
	for vuelta := 0; vuelta < 2; vuelta++ {
		que := "vuelta " + strconv.Itoa(vuelta)
		_ = os.Remove(marca)
		v := cdwVistaSimple(t)
		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, "/bin/sh", "-c", script)
		cmd.Dir = v.Dir
		cmd.WaitDelay = 20 * time.Second
		if err := encierro.Envolver(cmd, encierro.Plan{Info: info, Vista: v}); err != nil {
			cancel()
			t.Fatalf("%s: Envolver: %v", ca, err)
		}
		arranco := make(chan error, 1)
		go func() { arranco <- encierro.Arrancar(cmd, 2*time.Second) }()
		if !r3Aparece(marca, 10*time.Second) {
			cancel()
			<-arranco
			t.Fatalf("%s: %s: fixture: no se pudo forzar que el comando aparezca tarde (el bwrap del PATH no corrio con --info-fd)", ca, que)
		}
		desde := time.Now()
		cancel()
		if err := <-arranco; err != nil {
			t.Logf("%s: %s: Arrancar cancelado antes de que el comando aparezca: %v: nada que probar", ca, que, err)
			continue
		}
		_ = cmd.Wait()
		encierro.Liberar(cmd)
		dur := time.Since(desde)
		if _, ok := cdwLeer(filepath.Join(v.Dir, "arranco")); !ok {
			t.Logf("%s: %s: el comando no llego a aparecer (termino a los %s de cancelar): nada que probar", ca, que, dur)
			continue
		}
		if _, ok := cdwLeer(filepath.Join(v.Dir, "term")); !ok {
			t.Fatalf("%s: %s: el comando que aparecio tarde recibe la gracia completa desde su SIGTERM: su trap de 1 s no dejo la marca (termino a los %s de cancelar)", ca, que, dur)
		}
		if dur > 15*time.Second {
			t.Fatalf("%s: %s: termina en un tiempo acotado: %s", ca, que, dur)
		}
	}
}
