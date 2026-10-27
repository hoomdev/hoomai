//go:build !windows

// Ronda 2 de gates-en-grupo: tests de los hallazgos que dejo la ronda 1.
//
//   - 20261027T164705_01d565 (low): al vencer el plazo, si el lider del gate
//     moria enseguida con la SIGTERM y la salida se cerraba, el resto del
//     grupo recibia SIGKILL sin la gracia.
//   - 20261027T164705_5a05b4 (low): un gate que salia con exit 0 dejando un
//     proceso con su salida abierta tardaba 15 s fijos y quedaba en error,
//     con una nota de timeout que no ocurrio.
//
// Escritos desde el contrato (go doc ./internal/gates y ./internal/procesos)
// y los hallazgos, sin leer la implementacion. Reusan los ayudantes de
// gates_en_grupo_test.go (gegLanzar, gegLeerPID, gegVivo, ...).
package gates

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// Hallazgo 01d565: vencido el plazo, el lider del gate (un sh sin trap) muere
// enseguida con la SIGTERM y la salida se cierra; un hijo del grupo que
// atrapa TERM, sin la salida del gate, necesita 2 s para limpiar. Completa
// su limpieza (deja la marca del final) antes de que el runner devuelva, y
// despues no queda nada vivo.
func TestHallazgo01d565_HijoQueAtrapaTERMTerminaSuLimpiezaAunqueElLiderMuera(t *testing.T) {
	d := t.TempDir()
	marca := filepath.Join(d, "marca_limpieza")
	// el hijo: sin la salida del gate, limpia en 2 s al recibir TERM
	hijoSh := gegScript(t, d, "hijo.sh", "trap 'sleep 2; echo limpio > \""+marca+"\"; exit 0' TERM\n"+
		gegEscribir(d, "hijo.pid", "$$")+
		"sleep 300 &\nwait\n")
	c := gegLanzar(t, map[string]string{"test": gegEscribir(d, "lider.pid", "$$") +
		"sh '" + hijoSh + "' >/dev/null 2>&1 &\n" +
		"sleep 300\n"}) // el lider: muere con la SIGTERM, sin trap
	lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

	if !c.volvio(gegTope) {
		t.Fatalf("hallazgo 01d565: el gate vencido vuelve en a lo sumo %v", gegTope)
	}
	if !gegHay(marca, 300*time.Millisecond) {
		t.Fatalf("hallazgo 01d565: el hijo %d que atrapa TERM termina su limpieza de 2 s antes de que el runner devuelva, aunque el lider %d haya muerto enseguida; no dejo su marca (el runner volvio %v despues del plazo: se corto la gracia)", hijo, lider, c.dur-gegPlazo)
	}
	if r := c.gate(t, "test"); !gegEsTimeout(r) {
		t.Fatalf("hallazgo 01d565: el gate vencido termina como timeout: %+v", r)
	}
	for _, pid := range []int{lider, hijo} {
		if !gegMuere(pid, 2*time.Second) {
			t.Fatalf("hallazgo 01d565: vuelto el gate no queda nada suyo vivo: %d sigue vivo", pid)
		}
	}
}

// Hallazgo 01d565: con el lider muerto enseguida, un hijo del grupo que
// ignora SIGTERM (y no tiene la salida) tiene la gracia entera: sigue vivo a
// los pocos segundos del plazo. Y nunca se espera mas que la gracia: muere
// con SIGKILL y el runner vuelve a mas tardar en el plazo mas la gracia mas
// un margen chico.
func TestHallazgo01d565_LaGraciaNoSeCortaNiSeExcedeConElLiderMuerto(t *testing.T) {
	d := t.TempDir()
	hijoSh := gegScript(t, d, "hijo.sh", "trap '' TERM\n"+gegEscribir(d, "hijo.pid", "$$")+"exec sleep 300\n")
	c := gegLanzar(t, map[string]string{"test": gegEscribir(d, "lider.pid", "$$") +
		"sh '" + hijoSh + "' >/dev/null 2>&1 &\n" +
		"sleep 300\n"}) // el lider: muere con la SIGTERM, sin trap
	lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

	// a la mitad de la gracia el hijo que ignora SIGTERM sigue vivo
	time.Sleep(time.Until(c.inicio.Add(gegPlazo + gegGracia/2)))
	if !gegVivo(hijo) {
		t.Fatalf("hallazgo 01d565: un hijo que ignora SIGTERM tiene la gracia entera (%v) aunque el lider %d muera enseguida; a %v del plazo el hijo %d ya esta muerto", gegGracia, lider, gegGracia/2, hijo)
	}
	const margen = 5 * time.Second
	if !c.volvio(gegPlazo + gegGracia + margen) {
		t.Fatalf("hallazgo 01d565: nunca se espera mas que la gracia: el runner vuelve en a lo sumo %v", gegPlazo+gegGracia+margen)
	}
	if espero := c.dur - gegPlazo; espero < gegGracia*8/10 {
		t.Fatalf("hallazgo 01d565: el runner espera la gracia (%v) por el hijo que ignora SIGTERM; volvio %v despues del plazo", gegGracia, espero)
	}
	if !gegMuere(hijo, 2*time.Second) {
		t.Fatalf("hallazgo 01d565: pasada la gracia el hijo %d que ignora SIGTERM muere con SIGKILL", hijo)
	}
	if r := c.gate(t, "test"); !gegEsTimeout(r) {
		t.Fatalf("hallazgo 01d565: el gate vencido termina como timeout: %+v", r)
	}
}

// Hallazgo 5a05b4: un gate que sale con exit 0 dejando un proceso con su
// salida abierta ('sleep 300 &' heredando stdout) vuelve enseguida, muy por
// debajo de 15 s, en pass, con exit 0, con su salida completa y sin nota de
// timeout; y el sleep no queda vivo. El plazo (5 s) es menor que 15 s: la
// nota de timeout que no ocurrio aparecia justo en ese caso.
func TestHallazgo5a05b4_GateConExit0QueDejaUnProcesoConSuSalidaVuelveEnPass(t *testing.T) {
	d := t.TempDir()
	const plazo = 5 * time.Second
	const enseguida = 3 * time.Second
	c := gegLanzarConPlazo(t, map[string]string{"test": "sleep 300 &\n" + // hereda la salida
		gegEscribir(d, "hijo.pid", "$!") +
		"echo uno\necho dos >&2\necho tres\n" +
		"exit 0\n"}, plazo)
	hijo := gegLeerPID(t, d, "hijo.pid")

	if !c.volvio(enseguida) {
		t.Fatalf("hallazgo 5a05b4: un gate que sale con exit 0 vuelve enseguida (en menos de %v) aunque deje un proceso (%d) con su salida abierta", enseguida, hijo)
	}
	r := c.gate(t, "test")
	if r.Status != verdict.StatusPass || r.ExitCode != 0 {
		t.Fatalf("hallazgo 5a05b4: el gate salio con exit 0: pasa, aunque haya dejado un proceso con su salida: %+v", r)
	}
	if strings.Contains(r.Notes, "timeout") || strings.Contains(r.Notes, "WaitDelay") {
		t.Fatalf("hallazgo 5a05b4: un gate que termino en milisegundos no lleva nota de timeout: %+v", r)
	}
	if got := strings.TrimSpace(r.OutputTail); got != "uno\ndos\ntres" {
		t.Fatalf("hallazgo 5a05b4: la salida del gate esta completa y en orden: %q", r.OutputTail)
	}
	if !gegMuere(hijo, 2*time.Second) {
		t.Fatalf("hallazgo 5a05b4: el sleep %d que dejo el gate no queda vivo despues de que el runner devuelve", hijo)
	}
}

// Guarda de 5a05b4: la salida combinada de un gate conserva stdout y stderr
// intercalados en el orden en que se escribieron, y lo que escriben a la vez
// el gate y un hijo suyo esta entero (nada se pisa).
func TestGatesEnGrupo2_5a05b4_SalidaCombinadaEnOrdenYEntera(t *testing.T) {
	t.Run("intercalada", func(t *testing.T) {
		c := gegLanzarConPlazo(t, map[string]string{"test": "i=1; while [ $i -le 100 ]; do echo o$i; echo e$i >&2; i=$((i+1)); done"}, time.Minute)
		if !c.volvio(gegTope) {
			t.Fatalf("hallazgo 5a05b4: un gate que sale enseguida vuelve enseguida")
		}
		var quiere []string
		for i := 1; i <= 100; i++ {
			quiere = append(quiere, fmt.Sprintf("o%d", i), fmt.Sprintf("e%d", i))
		}
		r := c.gate(t, "test")
		if r.Status != verdict.StatusPass {
			t.Fatalf("hallazgo 5a05b4: el gate pasa: %+v", r)
		}
		if got := strings.TrimSpace(r.OutputTail); got != strings.Join(quiere, "\n") {
			t.Fatalf("hallazgo 5a05b4: stdout y stderr quedan intercalados en orden en la salida del gate:\n%s", r.OutputTail)
		}
	})
	t.Run("a la vez", func(t *testing.T) {
		c := gegLanzarConPlazo(t, map[string]string{"test": "(i=1; while [ $i -le 200 ]; do echo b$i; i=$((i+1)); done) &\n" +
			"i=1; while [ $i -le 200 ]; do echo a$i >&2; i=$((i+1)); done\n" +
			"wait\necho FIN\n"}, time.Minute)
		if !c.volvio(gegTope) {
			t.Fatalf("hallazgo 5a05b4: un gate que sale enseguida vuelve enseguida")
		}
		r := c.gate(t, "test")
		lineas := map[string]bool{}
		for _, l := range strings.Split(strings.TrimSpace(r.OutputTail), "\n") {
			lineas[l] = true
		}
		for i := 1; i <= 200; i++ {
			for _, l := range []string{fmt.Sprintf("a%d", i), fmt.Sprintf("b%d", i)} {
				if !lineas[l] {
					t.Fatalf("hallazgo 5a05b4: lo que escriben a la vez el gate y su hijo esta entero en la salida; falta %q:\n%s", l, r.OutputTail)
				}
			}
		}
		if !strings.HasSuffix(strings.TrimSpace(r.OutputTail), "FIN") {
			t.Fatalf("hallazgo 5a05b4: la salida termina con lo ultimo que escribio el gate:\n%s", r.OutputTail)
		}
	})
}
