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
// TestAyudanteHoomConUnGate (gates_en_grupo_ayudantes_test.go). Escritos
// desde el contrato y el hallazgo, sin leer la implementacion.
package gates

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"
	"time"
)

// Hallazgo 3f106e: hoom que recibe Ctrl-C o SIGTERM con un gate corriendo le
// manda SIGTERM al grupo del gate antes del SIGKILL: el gate que lo atrapa
// deja su marca. Como el grupo termina enseguida, hoom no espera la gracia
// entera; termina como terminaria con esa senal y no queda nada vivo.
func TestHallazgo3f106e_SenalAHoomLeMandaSIGTERMAlGate(t *testing.T) {
	for _, caso := range gegSenalesAHoom {
		t.Run(caso.nombre, func(t *testing.T) {
			d := t.TempDir()
			marca := filepath.Join(d, "marca_gate")
			lider := gegScript(t, d, "lider.sh",
				"trap 'echo term > \""+marca+"\"; exit 0' TERM\n"+
					gegEscribir(d, "lider.pid", "$$")+
					"sleep 300 >/dev/null 2>&1 &\n"+
					gegEscribir(d, "hijo.pid", "$!")+
					"wait\n")
			hoom := gegArrancarHoom(t, gegConUnGate(d, "exec sh '"+lider+"'", "hallazgo 3f106e"))
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
	for _, caso := range gegSenalesAHoom {
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
			cliente := gegScript(t, d, "cliente.sh",
				"trap 'kill -TERM "+cont+"; while kill -0 "+cont+" 2>/dev/null; do sleep 0.1; done; exit 143' TERM\n"+
					gegEscribir(d, "cliente.pid", "$$")+
					"while :; do sleep 0.1; done\n")
			hoom := gegArrancarHoom(t, gegConUnGate(d, "exec sh '"+cliente+"'", "hallazgo 3f106e"))
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
	for _, caso := range gegSenalesAHoom {
		t.Run(caso.nombre, func(t *testing.T) {
			d := t.TempDir()
			hoom := gegArrancarHoom(t, gegConUnGate(d, "trap '' TERM\n"+ // lo heredan sus hijos
				gegEscribir(d, "lider.pid", "$$")+
				"sleep 300 >/dev/null 2>&1 &\n"+
				gegEscribir(d, "hijo.pid", "$!")+
				"wait\n", "hallazgo 3f106e"))
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
