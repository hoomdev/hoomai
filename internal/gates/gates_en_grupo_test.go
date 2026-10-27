//go:build !windows

// Tests de regresion de los hallazgos 20261023T140133_862449 (medium) y
// 20261023T175558_af0383 (medium): cada gate de hoom verify corre en un grupo
// de procesos propio, y terminarlo termina todo lo que lanzo.
//
//   - 862449: al vencer el plazo de un gate, hoom mataba solo el sh; un hijo
//     que retenia la salida quedaba huerfano y verify lo esperaba.
//   - af0383: semgrep corre en el CI como 'exec docker run --rm ...'; al
//     vencer el plazo el cliente docker recibia SIGKILL, que no le pasa a su
//     contenedor, y el contenedor seguia.
//
// Escritos desde el contrato publico (go doc ./internal/gates: Run y
// Options.Timeout; go doc ./internal/verdict: GateResult y sus Status) y los
// hallazgos, sin leer la implementacion. Lo que se mide por caja negra: un
// gate vencido termina con Status error y una nota que dice "timeout"; la
// gracia entre el SIGTERM y el SIGKILL es de unos 10 s.
package gates

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	// gegPlazo es el plazo de cada gate de estos tests: corto, pero con
	// lugar para que el sh arranque y deje sus PID con la maquina ocupada.
	gegPlazo = 2 * time.Second
	// gegGracia es la gracia medida entre el SIGTERM y el SIGKILL.
	gegGracia = 10 * time.Second
	// gegMargen absorbe la maquina ocupada (-race, el resto de la suite).
	gegMargen = 10 * time.Second
	// gegTope es lo maximo que un gate vencido puede tardar en volver:
	// nunca los 300 s del hijo que se queda.
	gegTope = gegPlazo + gegGracia + gegMargen
)

// gegVivo dice si el proceso pid existe y no es un zombie.
func gegVivo(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil && !errors.Is(err, syscall.EPERM) {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	var salida *exec.ExitError
	if errors.As(err, &salida) {
		return false // ps no lo encuentra
	}
	if err != nil {
		return true // ps no pudo decir: kill dijo que existe
	}
	return !strings.HasPrefix(strings.TrimSpace(string(out)), "Z")
}

// gegMuere espera hasta plazo a que el proceso pid deje de existir.
func gegMuere(pid int, plazo time.Duration) bool {
	hasta := time.Now().Add(plazo)
	for time.Now().Before(hasta) {
		if !gegVivo(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !gegVivo(pid)
}

// gegEscribir es la linea de sh que deja valor en d/nombre de una vez (un
// archivo temporal y mv), para que el test nunca lea un PID a medias.
func gegEscribir(d, nombre, valor string) string {
	tmp := filepath.Join(d, nombre+".tmp")
	return "echo " + valor + " > '" + tmp + "' && mv '" + tmp + "' '" + filepath.Join(d, nombre) + "'\n"
}

// gegLeerPID espera hasta 20 s a que el gate deje un PID en d/nombre, y deja
// para el final del test matar ese proceso si sigue vivo: ningun test deja
// procesos atras, tampoco cuando falla.
func gegLeerPID(t *testing.T, d, nombre string) int {
	t.Helper()
	hasta := time.Now().Add(20 * time.Second)
	for time.Now().Before(hasta) {
		if raw, err := os.ReadFile(filepath.Join(d, nombre)); err == nil {
			if n, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && n > 0 {
				t.Cleanup(func() {
					if gegVivo(n) {
						syscall.Kill(n, syscall.SIGKILL)
					}
				})
				return n
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("fixture: el gate no dejo %s en 20 s", nombre)
	return 0
}

// gegHay espera hasta plazo a que exista el archivo ruta.
func gegHay(ruta string, plazo time.Duration) bool {
	hasta := time.Now().Add(plazo)
	for time.Now().Before(hasta) {
		if _, err := os.Stat(ruta); err == nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	_, err := os.Stat(ruta)
	return err == nil
}

// gegCorrida es una llamada a Run en curso.
type gegCorrida struct {
	inicio time.Time
	fin    chan struct{}
	res    []verdict.GateResult
	dur    time.Duration
}

// gegLanzar corre Run con los gates dados (nombre => comando, todos
// requeridos) y el plazo gegPlazo, sin esperarlo.
func gegLanzar(t *testing.T, gates map[string]string) *gegCorrida {
	t.Helper()
	req := true
	m := &manifest.Manifest{Dir: t.TempDir(), Gates: map[string]manifest.Gate{}}
	for nombre, cmd := range gates {
		m.Gates[nombre] = manifest.Gate{Cmd: strPtr(cmd), Required: &req}
	}
	c := &gegCorrida{inicio: time.Now(), fin: make(chan struct{})}
	go func() {
		c.res = Run(m, gitx.Info{}, Options{Timeout: gegPlazo, TailSize: 4096})
		c.dur = time.Since(c.inicio)
		close(c.fin)
	}()
	return c
}

// volvio dice si Run volvio antes de que pase plazo desde su inicio.
func (c *gegCorrida) volvio(plazo time.Duration) bool {
	select {
	case <-c.fin:
		return true
	case <-time.After(time.Until(c.inicio.Add(plazo))):
		return false
	}
}

// gate devuelve el resultado del gate nombre (Run ya volvio).
func (c *gegCorrida) gate(t *testing.T, nombre string) verdict.GateResult {
	t.Helper()
	for _, r := range c.res {
		if r.Name == nombre {
			return r
		}
	}
	t.Fatalf("fixture: Run no devolvio el gate %q: %+v", nombre, c.res)
	return verdict.GateResult{}
}

// gegEsTimeout dice si r es el resultado de un gate vencido.
func gegEsTimeout(r verdict.GateResult) bool {
	return r.Status == verdict.StatusError && strings.Contains(r.Notes, "timeout")
}

// Hallazgo 862449: un gate que deja un hijo con la salida heredada (el pipe
// de la salida combinada abierto) y no termina vuelve al vencer el plazo, en
// a lo sumo el plazo mas la gracia mas un margen, nunca en los 300 s del
// hijo; el resultado es el de un timeout y nada del gate queda vivo.
func TestHallazgo862449_PlazoVencidoConHijoQueRetieneLaSalidaVuelveATiempo(t *testing.T) {
	d := t.TempDir()
	c := gegLanzar(t, map[string]string{"test": gegEscribir(d, "lider.pid", "$$") +
		"sleep 300 &\n" + // hereda la salida del gate
		gegEscribir(d, "hijo.pid", "$!") +
		"wait\n"})
	lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

	if !c.volvio(gegTope) {
		t.Fatalf("hallazgo 862449: vencido el plazo (%v) el gate vuelve en a lo sumo %v; Run sigue esperando al hijo %d, que quedo huerfano con la salida abierta", gegPlazo, gegTope, hijo)
	}
	if r := c.gate(t, "test"); !gegEsTimeout(r) {
		t.Fatalf("hallazgo 862449: el gate vencido termina como timeout (status %q con la nota timeout): %+v", verdict.StatusError, r)
	}
	if !gegMuere(lider, 2*time.Second) || !gegMuere(hijo, 2*time.Second) {
		t.Fatalf("hallazgo 862449: vuelto el gate vencido no queda nada suyo vivo: sh %d vivo=%v, hijo %d vivo=%v", lider, gegVivo(lider), hijo, gegVivo(hijo))
	}
}

// Hallazgo af0383 (y 862449): al vencer el plazo, TODO el grupo del gate
// recibe SIGTERM, no solo el sh: el sh que lo atrapa deja su marca y el
// nieto que lo atrapa tambien. Es lo que le permite a un cliente docker
// pasarle la senal a su contenedor.
func TestHallazgoAf0383_PlazoVencidoMandaSIGTERMATodoElGrupo(t *testing.T) {
	d := t.TempDir()
	marcaLider, marcaNieto := filepath.Join(d, "marca_lider"), filepath.Join(d, "marca_nieto")
	nieto := filepath.Join(d, "nieto.sh")
	// el nieto hereda la salida del gate, como el cliente docker de semgrep
	if err := os.WriteFile(nieto, []byte(
		"trap 'echo term > \""+marcaNieto+"\"; exit 0' TERM\n"+
			gegEscribir(d, "nieto.pid", "$$")+
			"sleep 300 &\n"+
			gegEscribir(d, "sleep_nieto.pid", "$!")+
			"wait\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lider := filepath.Join(d, "lider.sh")
	if err := os.WriteFile(lider, []byte(
		"trap 'echo term > \""+marcaLider+"\"; exit 0' TERM\n"+
			gegEscribir(d, "lider.pid", "$$")+
			// el lider lanza un hijo que lanza al nieto
			"sh -c 'sh \""+nieto+"\" & wait' &\n"+
			"wait\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := gegLanzar(t, map[string]string{"security": "exec sh '" + lider + "'"})
	pidLider := gegLeerPID(t, d, "lider.pid")
	pidNieto, pidSleep := gegLeerPID(t, d, "nieto.pid"), gegLeerPID(t, d, "sleep_nieto.pid")

	if !gegHay(marcaLider, gegTope) {
		t.Fatalf("hallazgo af0383: al vencer el plazo el sh del gate recibe SIGTERM y lo atrapa; no dejo su marca en %v (hoom lo mato sin avisar)", gegTope)
	}
	if !gegHay(marcaNieto, 2*time.Second) {
		t.Fatalf("hallazgo af0383: al vencer el plazo TODO el grupo del gate recibe SIGTERM; el nieto %d no dejo su marca", pidNieto)
	}
	if !c.volvio(gegTope) {
		t.Fatalf("hallazgo 862449: vencido el plazo el gate vuelve en a lo sumo %v", gegTope)
	}
	if r := c.gate(t, "security"); !gegEsTimeout(r) {
		t.Fatalf("hallazgo af0383: el gate vencido termina como timeout aunque su grupo salga limpio con el SIGTERM: %+v", r)
	}
	for _, pid := range []int{pidLider, pidNieto, pidSleep} {
		if !gegMuere(pid, 2*time.Second) {
			t.Fatalf("hallazgo af0383: vuelto el gate no queda nada suyo vivo: %d sigue vivo", pid)
		}
	}
}

// Hallazgo af0383: el caso del CI. El gate es un cliente (como 'exec docker
// run') y lo que trabaja es un "contenedor" que NO es de su arbol de
// procesos (lo administra otro, como el daemon de docker). Al vencer el
// plazo el cliente recibe SIGTERM, se lo pasa al contenedor y espera a que
// pare; el contenedor para y verify vuelve a tiempo.
func TestHallazgoAf0383_ElClienteLePasaElSIGTERMAlContenedor(t *testing.T) {
	d := t.TempDir()
	marca := filepath.Join(d, "marca_contenedor")
	contenedor := exec.Command("sh", "-c",
		"trap 'echo term > \""+marca+"\"; exit 0' TERM\n"+
			"while :; do sleep 0.1; done\n")
	// fuera del grupo del test y de cualquier gate, como un contenedor
	contenedor.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := contenedor.Start(); err != nil {
		t.Fatal(err)
	}
	paro := make(chan struct{})
	go func() { contenedor.Wait(); close(paro) }() // lo cosecha apenas termina
	t.Cleanup(func() {
		syscall.Kill(-contenedor.Process.Pid, syscall.SIGKILL)
		<-paro
	})

	cont := strconv.Itoa(contenedor.Process.Pid)
	cliente := filepath.Join(d, "cliente.sh")
	if err := os.WriteFile(cliente, []byte(
		"trap 'kill -TERM "+cont+"; while kill -0 "+cont+" 2>/dev/null; do sleep 0.1; done; exit 143' TERM\n"+
			gegEscribir(d, "cliente.pid", "$$")+
			"while :; do sleep 0.1; done\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// exec, como .github/semgrep-en-docker.sh: el cliente es el proceso del gate
	c := gegLanzar(t, map[string]string{"security": "exec sh '" + cliente + "'"})
	pidCliente := gegLeerPID(t, d, "cliente.pid")

	select {
	case <-paro:
	case <-time.After(time.Until(c.inicio.Add(gegTope))):
		t.Fatalf("hallazgo af0383: vencido el plazo del gate, el cliente le pasa el SIGTERM a su contenedor y el contenedor para; a los %v el contenedor %s sigue corriendo (el cliente %d murio sin poder avisarle)", gegTope, cont, pidCliente)
	}
	if !gegHay(marca, time.Second) {
		t.Fatalf("hallazgo af0383: el contenedor para por el SIGTERM que le paso el cliente, no por otra cosa")
	}
	if !c.volvio(gegTope) {
		t.Fatalf("hallazgo af0383: con el contenedor parado verify vuelve en a lo sumo %v", gegTope)
	}
	if r := c.gate(t, "security"); !gegEsTimeout(r) {
		t.Fatalf("hallazgo af0383: el gate vencido termina como timeout: %+v", r)
	}
	if !gegMuere(pidCliente, 2*time.Second) {
		t.Fatalf("hallazgo af0383: vuelto el gate el cliente %d no sigue vivo", pidCliente)
	}
}

// Hallazgo 862449: un proceso del grupo que ignora SIGTERM muere igual, con
// SIGKILL, despues de la gracia; antes tiene la gracia entera (unos 10 s)
// para terminar solo. Vuelto el gate, ningun proceso del grupo sigue vivo.
func TestHallazgo862449_QuienIgnoraSIGTERMMuereDespuesDeLaGracia(t *testing.T) {
	d := t.TempDir()
	c := gegLanzar(t, map[string]string{"test": "trap '' TERM\n" + // lo heredan sus hijos
		gegEscribir(d, "lider.pid", "$$") +
		"sleep 300 >/dev/null 2>&1 &\n" +
		gegEscribir(d, "hijo.pid", "$!") +
		"wait\n"})
	lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

	if !c.volvio(gegTope) {
		t.Fatalf("hallazgo 862449: con un grupo que ignora SIGTERM el gate vencido vuelve igual, despues de la gracia: en a lo sumo %v", gegTope)
	}
	for _, pid := range []int{lider, hijo} {
		if !gegMuere(pid, 2*time.Second) {
			t.Fatalf("hallazgo 862449: un proceso del grupo que ignora SIGTERM muere con SIGKILL despues de la gracia; %d sigue vivo (sh %d vivo=%v, hijo %d vivo=%v): hoom solo mato al sh", pid, lider, gegVivo(lider), hijo, gegVivo(hijo))
		}
	}
	if r := c.gate(t, "test"); !gegEsTimeout(r) {
		t.Fatalf("hallazgo 862449: el gate vencido termina como timeout: %+v", r)
	}
	// la gracia: el SIGKILL llega despues de dejarle tiempo al grupo, no
	// junto con el SIGTERM (un cliente docker necesita ese tiempo)
	if gracia := c.dur - gegPlazo; gracia < gegGracia/2 || gracia > gegGracia+gegMargen {
		t.Fatalf("hallazgo af0383: entre el SIGTERM y el SIGKILL hay una gracia de unos %v; el gate volvio %v despues del plazo", gegGracia, gracia)
	}
}

// Hallazgo 862449: lo que un gate deja corriendo muere cuando el gate
// termina. Un gate que lanza 'sleep 300 &' y sale enseguida con exit 0 no
// deja ese sleep vivo despues de que el runner devuelve, tenga o no la
// salida del gate.
func TestHallazgo862449_LoQueElGateDejaCorriendoMuereAlTerminar(t *testing.T) {
	t.Run("sin la salida", func(t *testing.T) {
		d := t.TempDir()
		c := gegLanzar(t, map[string]string{"test": "sleep 300 >/dev/null 2>&1 &\n" +
			gegEscribir(d, "hijo.pid", "$!") +
			"echo listo\nexit 0\n"})
		hijo := gegLeerPID(t, d, "hijo.pid")
		if !c.volvio(gegTope) {
			t.Fatalf("hallazgo 862449: un gate que sale enseguida vuelve enseguida")
		}
		if r := c.gate(t, "test"); r.Status != verdict.StatusPass || r.ExitCode != 0 || !strings.Contains(r.OutputTail, "listo") {
			t.Fatalf("hallazgo 862449: el gate salio con exit 0: pasa, aunque hoom termine lo que dejo corriendo: %+v", r)
		}
		if !gegMuere(hijo, 2*time.Second) {
			t.Fatalf("hallazgo 862449: el sleep %d que dejo corriendo el gate sigue vivo despues de que el runner devolvio", hijo)
		}
	})
	t.Run("con la salida", func(t *testing.T) {
		d := t.TempDir()
		c := gegLanzar(t, map[string]string{"test": "sleep 300 &\n" + // hereda la salida
			gegEscribir(d, "hijo.pid", "$!") +
			"echo listo\nexit 0\n"})
		hijo := gegLeerPID(t, d, "hijo.pid")
		if !c.volvio(gegTope) {
			t.Fatalf("hallazgo 862449: un gate que sale enseguida y deja un hijo con su salida vuelve en a lo sumo %v, no cuando el hijo termine (300 s)", gegTope)
		}
		if !gegMuere(hijo, 2*time.Second) {
			t.Fatalf("hallazgo 862449: el sleep %d que dejo corriendo el gate sigue vivo despues de que el runner devolvio", hijo)
		}
	})
}

// Hallazgo af0383: verify no sigue con los gates posteriores mientras lo que
// lanzo un gate vencido sigue corriendo. El gate siguiente (test corre
// despues de security) mira si el hijo de security sigue vivo.
func TestHallazgoAf0383_ElGateSiguienteNoCorreConElAnteriorVivo(t *testing.T) {
	d := t.TempDir()
	hijoPID := filepath.Join(d, "hijo.pid")
	c := gegLanzar(t, map[string]string{
		"security": "sleep 300 >/dev/null 2>&1 &\n" +
			gegEscribir(d, "hijo.pid", "$!") +
			"wait\n",
		// un zombie que nadie cosecho todavia ya no corre: cuenta como muerto
		"test": "e=$(ps -o stat= -p \"$(cat '" + hijoPID + "')\" 2>/dev/null)\n" +
			"case \"$e\" in '' | Z*) echo hijo-muerto ;; *) echo HIJO-VIVO; exit 1 ;; esac\n",
	})
	gegLeerPID(t, d, "hijo.pid")
	if !c.volvio(2 * gegTope) {
		t.Fatalf("hallazgo 862449: dos gates, uno vencido, vuelven en a lo sumo %v", 2*gegTope)
	}
	if r := c.gate(t, "security"); !gegEsTimeout(r) {
		t.Fatalf("hallazgo af0383: el gate vencido termina como timeout: %+v", r)
	}
	if r := c.gate(t, "test"); r.Status != verdict.StatusPass {
		t.Fatalf("hallazgo af0383: el gate siguiente corre con lo del gate vencido ya muerto; vio al hijo de security vivo: %+v", r)
	}
}

// Guarda de 862449 y af0383: correr cada gate en su grupo no cambia lo que
// un gate normal da. Pasa o falla por su exit code, y su salida es la
// combinada, stdout y stderr.
func TestGatesEnGrupo862449_GateNormalSigueSiendoSuExitCodeYSuSalida(t *testing.T) {
	c := gegLanzar(t, map[string]string{
		"lint": "echo por-stdout-lint; echo por-stderr-lint >&2; exit 0",
		"test": "echo por-stdout-test; echo por-stderr-test >&2; exit 3",
	})
	if !c.volvio(gegTope) {
		t.Fatalf("hallazgo 862449: dos gates que salen enseguida vuelven enseguida")
	}
	lint := c.gate(t, "lint")
	if lint.Status != verdict.StatusPass || lint.ExitCode != 0 {
		t.Fatalf("hallazgo 862449: un gate con exit 0 pasa: %+v", lint)
	}
	test := c.gate(t, "test")
	if test.Status != verdict.StatusFail || test.ExitCode != 3 {
		t.Fatalf("hallazgo 862449: un gate con exit 3 falla con exit 3: %+v", test)
	}
	for _, r := range []verdict.GateResult{lint, test} {
		for _, quiere := range []string{"por-stdout-" + r.Name, "por-stderr-" + r.Name} {
			if !strings.Contains(r.OutputTail, quiere) {
				t.Fatalf("hallazgo 862449: la salida del gate %s es la combinada; le falta %q: %q", r.Name, quiere, r.OutputTail)
			}
		}
		if strings.Contains(r.Notes, "timeout") {
			t.Fatalf("hallazgo 862449: un gate que termina a tiempo no es un timeout: %+v", r)
		}
	}
}
