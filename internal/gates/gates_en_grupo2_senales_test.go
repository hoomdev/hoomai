//go:build !windows

// Ronda 2 de gates-en-grupo, hallazgo 20261027T164705_3f106e (medium): cuando
// hoom recibia Ctrl-C o SIGTERM con un gate corriendo, mataba el grupo del
// gate con SIGKILL, sin SIGTERM ni gracia; un cliente docker no alcanzaba a
// pasarle la senal a su contenedor y el contenedor seguia (af0383 al cancelar
// en vez de al vencer el plazo). En 8900379 el Ctrl-C de la terminal si le
// llegaba al cliente: era una regresion.
//
// El contrato: ante SIGINT (al PID o al grupo de hoom) o SIGTERM, el grupo
// del gate recibe SIGTERM antes del SIGKILL; si el grupo termina antes de la
// gracia, hoom no la espera entera; una segunda senal durante la espera la
// corta y el grupo muere con SIGKILL. El hoom es este mismo binario corriendo
// TestAyudanteGatesEnGrupo2Hoom. Escritos desde el contrato y el hallazgo,
// sin leer la implementacion.
package gates

import (
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
)

const (
	// geg2EnvDir y geg2EnvCmd llevan al hoom ayudante su carpeta y el
	// comando de su gate.
	geg2EnvDir = "HOOM_TEST_GATES_EN_GRUPO2_DIR"
	geg2EnvCmd = "HOOM_TEST_GATES_EN_GRUPO2_CMD"
)

// TestAyudanteGatesEnGrupo2Hoom no es un test: es el hoom verify de los tests
// de 3f106e. Corre el gate security con el comando que le pasan y un plazo de
// 5 minutos, y se queda ahi hasta que lo terminen.
func TestAyudanteGatesEnGrupo2Hoom(t *testing.T) {
	d, cmd := os.Getenv(geg2EnvDir), os.Getenv(geg2EnvCmd)
	if d == "" || cmd == "" {
		t.Skip("solo corre como proceso ayudante de los tests de 3f106e")
	}
	req := true
	m := &manifest.Manifest{Dir: d, Gates: map[string]manifest.Gate{"security": {Cmd: &cmd, Required: &req}}}
	Run(m, gitx.Info{}, Options{Timeout: 5 * time.Minute, TailSize: 4096})
}

// geg2Proc es un hoom ayudante en marcha.
type geg2Proc struct {
	cmd *exec.Cmd
	log string
	fin chan struct{}
}

// geg2Hoom arranca el hoom ayudante en un grupo propio (como una terminal:
// la senal "terminal" que le llega a su grupo no le llega al test) con el
// gate cmd.
func geg2Hoom(t *testing.T, d, cmd string) *geg2Proc {
	t.Helper()
	hoom := exec.Command(os.Args[0], "-test.run=^TestAyudanteGatesEnGrupo2Hoom$", "-test.count=1")
	hoom.Env = append(os.Environ(), geg2EnvDir+"="+d, geg2EnvCmd+"="+cmd)
	hoom.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log := filepath.Join(t.TempDir(), "hoom.log")
	salida, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	defer salida.Close()
	hoom.Stdout, hoom.Stderr = salida, salida
	if err := hoom.Start(); err != nil {
		t.Fatal(err)
	}
	p := &geg2Proc{cmd: hoom, log: log, fin: make(chan struct{})}
	go func() { hoom.Wait(); close(p.fin) }()
	t.Cleanup(func() {
		syscall.Kill(-hoom.Process.Pid, syscall.SIGKILL)
		<-p.fin
		// una carrera de datos en el hoom ayudante no se ve en la salida
		// del test: se lee de su log
		if raw, _ := os.ReadFile(log); strings.Contains(string(raw), "DATA RACE") {
			t.Errorf("hallazgo 3f106e: el hoom ayudante tuvo una carrera de datos:\n%s", raw)
		}
	})
	return p
}

// senal le manda sig a hoom, solo a su PID o (terminal) a su grupo entero.
func (p *geg2Proc) senal(t *testing.T, sig syscall.Signal, terminal bool) {
	t.Helper()
	destino := p.cmd.Process.Pid
	if terminal {
		destino = -destino
	}
	if err := syscall.Kill(destino, sig); err != nil && !(err == syscall.ESRCH && p.termino(0)) {
		t.Fatal(err)
	}
}

// termino dice si hoom termino antes de que pase plazo.
func (p *geg2Proc) termino(plazo time.Duration) bool {
	select {
	case <-p.fin:
		return true
	case <-time.After(plazo):
		return false
	}
}

// porLaSenal dice si hoom termino como terminaria siempre con sig: por la
// senal, o con 128+sig como un shell.
func (p *geg2Proc) porLaSenal(sig syscall.Signal) bool {
	ws, _ := p.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return (ws.Signaled() && ws.Signal() == sig) || (ws.Exited() && ws.ExitStatus() == 128+int(sig))
}

// geg2Casos son las senales de 3f106e: el Ctrl-C (al PID de hoom o a su
// grupo, como la terminal) y SIGTERM.
var geg2Casos = []struct {
	nombre   string
	senal    syscall.Signal
	terminal bool
}{
	{"SIGINT", syscall.SIGINT, false},
	{"SIGINT terminal", syscall.SIGINT, true},
	{"SIGTERM", syscall.SIGTERM, false},
	{"SIGTERM terminal", syscall.SIGTERM, true},
}

// Hallazgo 3f106e: hoom que recibe Ctrl-C o SIGTERM con un gate corriendo le
// manda SIGTERM al grupo del gate antes del SIGKILL: el gate que lo atrapa
// deja su marca. Como el grupo termina enseguida, hoom no espera la gracia
// entera; termina como terminaria con esa senal y no queda nada vivo.
func TestHallazgo3f106e_SenalAHoomLeMandaSIGTERMAlGate(t *testing.T) {
	for _, caso := range geg2Casos {
		t.Run(caso.nombre, func(t *testing.T) {
			d := t.TempDir()
			marca := filepath.Join(d, "marca_gate")
			lider := geg2Script(t, d, "lider.sh",
				"trap 'echo term > \""+marca+"\"; exit 0' TERM\n"+
					gegEscribir(d, "lider.pid", "$$")+
					"sleep 300 >/dev/null 2>&1 &\n"+
					gegEscribir(d, "hijo.pid", "$!")+
					"wait\n")
			hoom := geg2Hoom(t, d, "exec sh '"+lider+"'")
			pidLider, pidHijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

			enviada := time.Now()
			hoom.senal(t, caso.senal, caso.terminal)
			if !hoom.termino(gegGracia + gegMargen) {
				t.Fatalf("hallazgo 3f106e: hoom que recibe %v termina en a lo sumo la gracia mas un margen (%v)", caso.senal, gegGracia+gegMargen)
			}
			tardo := time.Since(enviada)
			if !gegHay(marca, time.Second) {
				t.Fatalf("hallazgo 3f106e: hoom que recibe %v le manda SIGTERM al grupo del gate antes del SIGKILL; el gate %d, que atrapa TERM, no dejo su marca (hoom termino a los %v)", caso.senal, pidLider, tardo)
			}
			if tardo > gegGracia/2 {
				t.Fatalf("hallazgo 3f106e: el gate termino enseguida con el SIGTERM: hoom no espera la gracia entera; tardo %v", tardo)
			}
			for _, pid := range []int{pidLider, pidHijo} {
				if !gegMuere(pid, 2*time.Second) {
					t.Fatalf("hallazgo 3f106e: terminado hoom no queda nada del gate vivo: %d sigue vivo", pid)
				}
			}
			if !hoom.porLaSenal(caso.senal) {
				t.Fatalf("hallazgo 3f106e: terminado el gate, hoom termina como terminaria siempre con %v: %v", caso.senal, hoom.cmd.ProcessState)
			}
		})
	}
}

// Hallazgo 3f106e: el caso de af0383 al cancelar. El gate es un cliente (como
// 'exec docker run') de un "contenedor" que NO es de su arbol de procesos.
// Cuando hoom recibe Ctrl-C o SIGTERM, el cliente recibe SIGTERM, se lo pasa
// al contenedor y espera a que pare: el contenedor para.
func TestHallazgo3f106e_SenalAHoomDejaQueElClientePareElContenedor(t *testing.T) {
	for _, caso := range geg2Casos {
		t.Run(caso.nombre, func(t *testing.T) {
			d := t.TempDir()
			marca := filepath.Join(d, "marca_contenedor")
			contenedor := exec.Command("sh", "-c",
				"trap 'echo term > \""+marca+"\"; exit 0' TERM\n"+
					"while :; do sleep 0.1; done\n")
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
			cliente := geg2Script(t, d, "cliente.sh",
				"trap 'kill -TERM "+cont+"; while kill -0 "+cont+" 2>/dev/null; do sleep 0.1; done; exit 143' TERM\n"+
					gegEscribir(d, "cliente.pid", "$$")+
					"while :; do sleep 0.1; done\n")
			hoom := geg2Hoom(t, d, "exec sh '"+cliente+"'")
			pidCliente := gegLeerPID(t, d, "cliente.pid")

			hoom.senal(t, caso.senal, caso.terminal)
			select {
			case <-paro:
			case <-time.After(gegGracia + gegMargen):
				t.Fatalf("hallazgo 3f106e: hoom que recibe %v deja que el cliente le pase el SIGTERM a su contenedor; a los %v el contenedor %s sigue corriendo (el cliente %d murio sin poder avisarle)", caso.senal, gegGracia+gegMargen, cont, pidCliente)
			}
			if !gegHay(marca, time.Second) {
				t.Fatalf("hallazgo 3f106e: el contenedor para por el SIGTERM que le paso el cliente")
			}
			if !hoom.termino(gegGracia + gegMargen) {
				t.Fatalf("hallazgo 3f106e: con el contenedor parado, hoom que recibio %v termina", caso.senal)
			}
			if !gegMuere(pidCliente, 2*time.Second) {
				t.Fatalf("hallazgo 3f106e: terminado hoom el cliente %d no sigue vivo", pidCliente)
			}
		})
	}
}

// Hallazgo 3f106e: con un gate que ignora SIGTERM, hoom que recibe Ctrl-C o
// SIGTERM le da la gracia (el gate sigue vivo y hoom esperando un segundo
// despues); una SEGUNDA senal durante esa espera la corta: hoom termina
// enseguida y el grupo del gate muere con SIGKILL.
func TestHallazgo3f106e_SegundaSenalCortaLaGracia(t *testing.T) {
	for _, caso := range geg2Casos {
		t.Run(caso.nombre, func(t *testing.T) {
			d := t.TempDir()
			hoom := geg2Hoom(t, d, "trap '' TERM\n"+ // lo heredan sus hijos
				gegEscribir(d, "lider.pid", "$$")+
				"sleep 300 >/dev/null 2>&1 &\n"+
				gegEscribir(d, "hijo.pid", "$!")+
				"wait\n")
			lider, hijo := gegLeerPID(t, d, "lider.pid"), gegLeerPID(t, d, "hijo.pid")

			hoom.senal(t, caso.senal, caso.terminal)
			time.Sleep(time.Second)
			if !gegVivo(lider) || !gegVivo(hijo) {
				t.Fatalf("hallazgo 3f106e: hoom que recibe %v le manda SIGTERM al grupo del gate antes del SIGKILL, con gracia; al segundo el gate que ignora SIGTERM ya murio (sh %d vivo=%v, hijo %d vivo=%v)", caso.senal, lider, gegVivo(lider), hijo, gegVivo(hijo))
			}
			if hoom.termino(0) {
				t.Fatalf("hallazgo 3f106e: hoom que recibe %v espera la gracia del gate antes de terminar; termino al segundo: %v", caso.senal, hoom.cmd.ProcessState)
			}

			segunda := time.Now()
			hoom.senal(t, caso.senal, caso.terminal)
			if !hoom.termino(3 * time.Second) {
				t.Fatalf("hallazgo 3f106e: una segunda %v durante la gracia la corta: hoom termina enseguida; sigue a los %v", caso.senal, time.Since(segunda))
			}
			for _, pid := range []int{lider, hijo} {
				if !gegMuere(pid, 2*time.Second) {
					t.Fatalf("hallazgo 3f106e: con la segunda %v el grupo del gate muere con SIGKILL: %d sigue vivo", caso.senal, pid)
				}
			}
			if !hoom.porLaSenal(caso.senal) {
				t.Fatalf("hallazgo 3f106e: hoom termina como terminaria siempre con %v: %v", caso.senal, hoom.cmd.ProcessState)
			}
		})
	}
}
