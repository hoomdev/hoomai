//go:build unix

// Tests de regresion del hallazgo 20261014T204543_b5e187 (high) de la mitad 2
// de la review de C3 (.hoom/specs/acciones-desde-la-tarjeta.md): el provider
// sobrevivia a hoom serve. Escritos desde el contrato (go doc Meta, Active,
// Manager.Cancel) y el hallazgo, sin leer la implementacion:
//
//  1. El provider de un run corre en su propio grupo de procesos, y cancelar
//     el run (Manager.Cancel o su plazo) mata el grupo entero: un proceso que
//     el provider lanzo (un "nieto") tambien muere.
//  2. Un proceso dueno de runs que recibe SIGTERM, SIGINT o SIGHUP mata los
//     grupos de sus runs antes de terminar como terminaria siempre.
//  3. El sidecar del run guarda child_pid, el PID del provider. Si el dueno
//     muere sin matarlo (SIGKILL), mientras el provider viva el run cuenta
//     como activo (Active, Busy) y un Manager nuevo no lo marca huerfano;
//     cuando el provider muere, deja de contar.
//
// El dueno de los casos 2 y 3 es este mismo binario de tests corriendo
// TestAyudanteDuenoDeRuns. Reusa installFake e initRepo de runcmd_test.go.
package runcmd

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// m2Script es un provider falso que deja su PID en hijo.pid, lanza un nieto
// (un sleep largo que no hereda los pipes del run) y deja su PID en
// nieto.pid, y se queda esperandolo.
func m2Script(d string) string {
	escribir := func(nombre, valor string) string {
		tmp := filepath.Join(d, nombre+".tmp")
		return "echo " + valor + " > '" + tmp + "' && mv '" + tmp + "' '" + filepath.Join(d, nombre) + "'\n"
	}
	return escribir("hijo.pid", "$$") +
		"sleep 297 </dev/null >/dev/null 2>&1 &\n" +
		escribir("nieto.pid", "$!") +
		"echo arranco\n" +
		"wait\n"
}

// m2Vivo dice si el proceso pid existe.
func m2Vivo(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// m2Muere espera hasta plazo a que el proceso pid deje de existir.
func m2Muere(pid int, plazo time.Duration) bool {
	hasta := time.Now().Add(plazo)
	for time.Now().Before(hasta) {
		if !m2Vivo(pid) {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return !m2Vivo(pid)
}

// m2PIDs espera a que el provider falso deje su PID y el del nieto, y deja
// para el final del test matarlos si siguen vivos.
func m2PIDs(t *testing.T, d string) (hijo, nieto int) {
	t.Helper()
	leer := func(nombre string) int {
		hasta := time.Now().Add(20 * time.Second)
		for time.Now().Before(hasta) {
			if raw, err := os.ReadFile(filepath.Join(d, nombre)); err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && n > 0 {
					return n
				}
			}
			time.Sleep(20 * time.Millisecond)
		}
		t.Fatalf("CA-346: fixture: el provider falso no dejo %s en 20 s", nombre)
		return 0
	}
	hijo, nieto = leer("hijo.pid"), leer("nieto.pid")
	t.Cleanup(func() {
		for _, pid := range []int{nieto, hijo} {
			if m2Vivo(pid) {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	if !m2Vivo(hijo) || !m2Vivo(nieto) {
		t.Fatalf("CA-346: fixture: el provider (%d) y su nieto (%d) estan vivos: %v %v", hijo, nieto, m2Vivo(hijo), m2Vivo(nieto))
	}
	return hijo, nieto
}

// m2Esperar espera hasta plazo a que el run id termine.
func m2Esperar(t *testing.T, m *Manager, id string, plazo time.Duration) Run {
	t.Helper()
	fin := make(chan Run, 1)
	go func() { fin <- m.Wait(id) }()
	select {
	case r := <-fin:
		return r
	case <-time.After(plazo):
		t.Fatalf("hallazgo b5e187: el run %s no termino en %v", id, plazo)
	}
	return Run{}
}

// Hallazgo 20261014T204543_b5e187 (high), contrato 1: cancelar el run, con
// Manager.Cancel o por su plazo (Manager.Timeout), mata el grupo de procesos
// entero del provider: el provider y el nieto que lanzo mueren los dos.
func TestHallazgo_b5e187_CancelarMataAlNieto(t *testing.T) {
	for _, como := range []string{"cancel", "plazo"} {
		t.Run(como, func(t *testing.T) {
			d := t.TempDir()
			installFake(t, m2Script(d))
			root := initRepo(t)
			m := NewManager(root)
			if como == "plazo" {
				m.Timeout = 2 * time.Second
			}
			r, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabajo largo"})
			if err != nil {
				t.Fatal(err)
			}
			hijo, nieto := m2PIDs(t, d)
			if como == "cancel" {
				if _, err := m.Cancel(r.ID); err != nil {
					t.Fatalf("hallazgo b5e187: Cancel: %v", err)
				}
			}
			m2Esperar(t, m, r.ID, 30*time.Second)
			if !m2Muere(nieto, 5*time.Second) {
				t.Fatalf("hallazgo b5e187: terminar el run (%s) mata su grupo de procesos entero: el nieto %d, que lanzo el provider, sigue vivo", como, nieto)
			}
			if !m2Muere(hijo, 5*time.Second) {
				t.Fatalf("hallazgo b5e187: terminar el run (%s) mata al provider: %d sigue vivo", como, hijo)
			}
		})
	}
}

// m2EnvRaiz lleva al proceso ayudante la raiz donde registra su run.
const m2EnvRaiz = "HOOM_TEST_DUENO_RAIZ"

// TestAyudanteDuenoDeRuns no es un test: es el proceso DUENO de un run de
// TestHallazgo_b5e187_SenalAlDuenoMataSusRuns y
// TestHallazgo_b5e187_DuenoMuertoConProviderVivoSigueActivo (como hoom
// serve). Registra un run en la raiz, dice "run <id>" y se queda hasta que lo
// terminen.
func TestAyudanteDuenoDeRuns(t *testing.T) {
	root := os.Getenv(m2EnvRaiz)
	if root == "" {
		t.Skip("solo corre como proceso ayudante de los tests de b5e187")
	}
	m := NewManager(root)
	r, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabajo largo"})
	if err != nil {
		fmt.Printf("error: %v\n", err)
		return
	}
	fmt.Printf("run %s\n", r.ID)
	time.Sleep(2 * time.Minute)
}

// m2Proc es el proceso dueno lanzado por un test.
type m2Proc struct {
	cmd    *exec.Cmd
	lineas chan string
	fin    chan struct{}
	estado *os.ProcessState
}

// m2Lanzar arranca el proceso dueno sobre root y espera su "run <id>".
func m2Lanzar(t *testing.T, root string) (*m2Proc, string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestAyudanteDuenoDeRuns$", "-test.count=1")
	cmd.Env = append(os.Environ(), m2EnvRaiz+"="+root)
	errlog, err := os.Create(filepath.Join(t.TempDir(), "stderr.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer errlog.Close()
	cmd.Stderr = errlog
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	p := &m2Proc{cmd: cmd, lineas: make(chan string, 16), fin: make(chan struct{})}
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			select {
			case p.lineas <- s.Text():
			default:
			}
		}
		cmd.Wait()
		p.estado = cmd.ProcessState
		close(p.fin)
	}()
	t.Cleanup(func() {
		cmd.Process.Kill()
		select {
		case <-p.fin:
		case <-time.After(20 * time.Second):
		}
	})
	select {
	case l := <-p.lineas:
		id, ok := strings.CutPrefix(l, "run ")
		if !ok {
			raw, _ := os.ReadFile(errlog.Name())
			t.Fatalf("CA-346: fixture: el proceso dueno registra su run: dijo %q\n%s", l, raw)
		}
		return p, id
	case <-p.fin:
		raw, _ := os.ReadFile(errlog.Name())
		t.Fatalf("CA-346: fixture: el proceso dueno termino sin registrar su run\n%s", raw)
	case <-time.After(30 * time.Second):
		t.Fatal("CA-346: fixture: el proceso dueno no registro su run en 30 s")
	}
	return nil, ""
}

// esperar espera hasta plazo a que el proceso dueno termine.
func (p *m2Proc) esperar(t *testing.T, plazo time.Duration) *os.ProcessState {
	t.Helper()
	select {
	case <-p.fin:
		return p.estado
	case <-time.After(plazo):
		t.Fatalf("hallazgo b5e187: el proceso dueno no termino en %v", plazo)
	}
	return nil
}

// Hallazgo 20261014T204543_b5e187 (high), contrato 2: un proceso dueno de
// runs que recibe SIGTERM, SIGINT o SIGHUP mata los grupos de sus runs (el
// provider y su nieto mueren) y termina como terminaria siempre: por esa
// senal.
func TestHallazgo_b5e187_SenalAlDuenoMataSusRuns(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP} {
		t.Run(sig.String(), func(t *testing.T) {
			d := t.TempDir()
			installFake(t, m2Script(d))
			root := initRepo(t)
			p, _ := m2Lanzar(t, root)
			hijo, nieto := m2PIDs(t, d)

			if err := p.cmd.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			estado := p.esperar(t, 20*time.Second)
			if !m2Muere(hijo, 5*time.Second) {
				t.Fatalf("hallazgo b5e187: el dueno que recibe %v mata el grupo de su run: el provider %d sigue vivo", sig, hijo)
			}
			if !m2Muere(nieto, 5*time.Second) {
				t.Fatalf("hallazgo b5e187: el dueno que recibe %v mata el grupo de su run entero: el nieto %d sigue vivo", sig, nieto)
			}
			ws, _ := estado.Sys().(syscall.WaitStatus)
			porLaSenal := ws.Signaled() && ws.Signal() == sig
			comoShell := ws.Exited() && ws.ExitStatus() == 128+int(sig)
			if !porLaSenal && !comoShell {
				t.Fatalf("hallazgo b5e187: despues de matar sus runs el dueno termina como terminaria siempre con %v: %v", sig, estado)
			}
		})
	}
}

// Hallazgo 20261014T204543_b5e187 (high), contrato 3: el sidecar guarda
// child_pid, el PID del provider. Con el dueno muerto por SIGKILL (no pudo
// matar a nadie) y el provider vivo, el run cuenta como activo (Active y
// Busy lo ven) y un Manager nuevo no lo marca huerfano. Matando al provider,
// deja de contar.
func TestHallazgo_b5e187_DuenoMuertoConProviderVivoSigueActivo(t *testing.T) {
	d := t.TempDir()
	installFake(t, m2Script(d))
	root := initRepo(t)
	p, id := m2Lanzar(t, root)
	hijo, _ := m2PIDs(t, d)

	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	p.esperar(t, 20*time.Second)
	if !m2Vivo(hijo) {
		t.Fatalf("CA-346: fixture: con el dueno muerto por SIGKILL nadie mata al provider %d", hijo)
	}

	if got, ok := Active(root, root); !ok || got != id {
		t.Fatalf("hallazgo b5e187: con el dueno muerto y el provider %d vivo, el run %s sigue activo: Active dice %q, %v", hijo, id, got, ok)
	}
	m := NewManager(root)
	if meta, ok := ReadMeta(root, id); !ok || meta.Status != StatusRunning {
		t.Fatalf("hallazgo b5e187: un Manager nuevo no marca huerfano a un run cuyo provider vive: %+v (existe: %v)", meta, ok)
	}
	if got, busy := m.Busy(root); !busy || got != id {
		t.Fatalf("hallazgo b5e187: con el provider vivo el arbol sigue ocupado por %s: Busy dice %q, %v", id, got, busy)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", id+".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sidecar map[string]any
	if err := json.Unmarshal(raw, &sidecar); err != nil {
		t.Fatalf("hallazgo b5e187: el sidecar es JSON: %v\n%s", err, raw)
	}
	if got, _ := sidecar["child_pid"].(float64); int(got) != hijo {
		t.Fatalf("hallazgo b5e187: el sidecar guarda child_pid, el PID del provider (%d):\n%s", hijo, raw)
	}

	if err := syscall.Kill(hijo, syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if !m2Muere(hijo, 5*time.Second) {
		t.Fatalf("CA-346: fixture: el provider %d muere con SIGKILL", hijo)
	}
	if got, ok := Active(root, root); ok {
		t.Fatalf("hallazgo b5e187: muerto el provider, el run deja de contar como activo: Active dice %q", got)
	}
	if got, busy := NewManager(root).Busy(root); busy {
		t.Fatalf("hallazgo b5e187: muerto el provider, el arbol deja de estar ocupado: Busy dice %q", got)
	}
}
