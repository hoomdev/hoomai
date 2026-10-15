//go:build unix

// Tests de regresion de la review de las correcciones de C3
// (.hoom/specs/acciones-desde-la-tarjeta.md), del lado de runcmd y en unix.
// Escritos desde el contrato (go doc Meta, Active, Manager.Start) y los
// hallazgos, sin leer la implementacion:
//
//   - 20261015T164404_da369a (medium): el registro del run ya no es
//     telemetria, es la identidad con la que otros procesos saben que el
//     arbol esta ocupado. Si no se puede escribir, Start devuelve error y no
//     queda ningun run registrado ni corriendo: el provider no arranca.
//   - 20261015T163651_cea64f (medium): un child_pid que ahora es OTRO proceso
//     (arranco en otro momento que el child_started_at del registro) no hace
//     contar al run como activo, y un Manager nuevo lo marca huerfano.
//   - 20261015T163244_e30013 (medium): la senal que llega mientras un
//     provider esta arrancando igual mata su grupo. Es una ventana de
//     milisegundos: el test la busca con muchos arranques desde varias
//     goroutines y una SIGTERM al dueno en medio. No la fuerza siempre: cada
//     intento la encuentra o no.
//
// Reusa installFake e initRepo (runcmd_test.go), hrSidecar, hrMetaCrudo y
// hrPidMuerto (hallazgos_review_test.go), leerMeta (meta_test.go) y m2Vivo y
// m2Muere (hallazgos_c3_mitad2_unix_test.go).
package runcmd

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// Hallazgo 20261015T164404_da369a (medium): con .hoom/runs donde se pueden
// crear archivos pero no renombrarlos (chflags uappnd, macOS) o sin permiso
// de escritura, el registro del run no se puede escribir: Manager.Start
// devuelve error y no queda ningun run registrado ni corriendo (ni en Busy,
// ni en la memoria del Manager, ni en un sidecar running), y el provider no
// arranca.
func TestHallazgo_da369a_SinRegistroDelRunNoHayRun(t *testing.T) {
	for _, c := range []struct {
		nombre   string
		preparar func(t *testing.T, runs string)
	}{
		{"solo-agregar", func(t *testing.T, runs string) {
			if runtime.GOOS != "darwin" {
				t.Skip("chflags uappnd es de macOS")
			}
			if out, err := exec.Command("chflags", "uappnd", runs).CombinedOutput(); err != nil {
				t.Skipf("chflags uappnd no se pudo poner: %v %s", err, out)
			}
			t.Cleanup(func() { exec.Command("chflags", "nouappnd", runs).Run() })
		}},
		{"sin-permiso-de-escritura", func(t *testing.T, runs string) {
			if os.Geteuid() == 0 {
				t.Skip("como root el permiso de escritura no frena a nadie")
			}
			if err := os.Chmod(runs, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(runs, 0o755) })
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			marca := filepath.Join(t.TempDir(), "provider-corrio")
			installFake(t, "touch '"+marca+"'\necho arranco\nsleep 30\n")
			root := initRepo(t)
			runs := filepath.Join(root, ".hoom", "runs")
			if err := os.MkdirAll(runs, 0o755); err != nil {
				t.Fatal(err)
			}
			m := NewManager(root)
			c.preparar(t, runs)

			r, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabajo largo"})
			if err == nil {
				m2Cancelar(t, m, r.ID, "arranco")
			}
			time.Sleep(700 * time.Millisecond) // un provider que arranco ya dejo su marca
			_, errMarca := os.Stat(marca)
			if err == nil {
				t.Fatalf("hallazgo da369a: si no se puede escribir el registro del run (%s), Start devuelve error; devolvio el run %s (el provider corrio: %v)", c.nombre, r.ID, errMarca == nil)
			}
			if errMarca == nil {
				t.Fatalf("hallazgo da369a: sin registro (%s) el provider no arranca, y arranco", c.nombre)
			}
			if id, busy := m.Busy(root); busy {
				t.Fatalf("hallazgo da369a: sin registro (%s) el arbol no queda ocupado: Busy ve %s", c.nombre, id)
			}
			for _, run := range m.List() {
				if run.Status == StatusRunning {
					t.Fatalf("hallazgo da369a: sin registro (%s) no queda ningun run corriendo en el Manager: %+v", c.nombre, run)
				}
			}
			for _, meta := range Metas(root) {
				if meta.Status == StatusRunning {
					t.Fatalf("hallazgo da369a: sin registro (%s) no queda ningun sidecar running: %+v", c.nombre, meta)
				}
			}
		})
	}
}

// ccArranque es la hora de arranque de pid que da ps (con resolucion de un
// segundo), la misma con la que se lo puede reconocer.
func ccArranque(t *testing.T, pid int) time.Time {
	t.Helper()
	out, err := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		t.Fatalf("CA-346: fixture: ps da la hora de arranque de %d: %v", pid, err)
	}
	cuando, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(strings.Fields(string(out)), " "), time.Local)
	if err != nil {
		t.Fatalf("CA-346: fixture: la hora de arranque de ps (%q): %v", out, err)
	}
	return cuando
}

// ccAjeno arranca un proceso ajeno al run (un sleep) y devuelve su PID y la
// hora real en que arranco.
func ccAjeno(t *testing.T) (int, time.Time) {
	t.Helper()
	ajeno := exec.Command("sleep", "60")
	if err := ajeno.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ajeno.Process.Kill()
		ajeno.Wait()
	})
	return ajeno.Process.Pid, ccArranque(t, ajeno.Process.Pid)
}

// ccRunConHijo deja en root un run en curso cuyo dueno murio y cuyo
// child_pid es pid, con child_started_at = hijoArranco. La narracion es
// reciente: lo unico que distingue al provider de un proceso ajeno es la
// hora en que arranco.
func ccRunConHijo(t *testing.T, root, id string, pid int, hijoArranco time.Time) {
	t.Helper()
	ahora := time.Now().UTC()
	sidecar := hrMetaCrudo(id, root, hrPidMuerto(t), ahora.Add(-3*time.Hour))
	sidecar["child_pid"] = pid
	sidecar["child_started_at"] = hijoArranco.UTC()
	hrSidecar(t, root, id, sidecar, ahora)
}

// Hallazgo 20261015T163651_cea64f (medium): el dueno del run murio y su
// child_pid es hoy OTRO proceso vivo, que arranco ahora y no hace 3 horas
// como dice child_started_at (el PID se reuso). Ese run NO cuenta como
// activo: Active da false y un Manager nuevo lo marca huerfano.
func TestHallazgo_cea64f_ChildPIDReusadoNoCuentaComoActivo(t *testing.T) {
	root := initRepo(t)
	pid, _ := ccAjeno(t)
	const id = "20261015T163651_cc0001"
	ccRunConHijo(t, root, id, pid, time.Now().Add(-3*time.Hour))

	if got, ok := Active(root, root); ok {
		t.Fatalf("hallazgo cea64f: un child_pid (%d) que es otro proceso (arranco despues de child_started_at) no hace activo al run: Active dice %q", pid, got)
	}
	m := NewManager(root)
	if meta := leerMeta(t, root, id); meta.Status == StatusRunning {
		t.Fatalf("hallazgo cea64f: un Manager nuevo marca huerfano al run cuyo child_pid se reuso: %+v", meta)
	}
	if got, busy := m.Busy(root); busy {
		t.Fatalf("hallazgo cea64f: el arbol no queda ocupado por un PID reusado: Busy dice %q", got)
	}
}

// Hallazgo 20261015T163651_cea64f (guarda): con child_started_at = la hora
// real de arranque de ese proceso, es el provider del run: cuenta como
// activo y un Manager nuevo no lo marca huerfano.
func TestHallazgo_cea64f_GuardaElMismoProviderSiCuenta(t *testing.T) {
	root := initRepo(t)
	pid, arranco := ccAjeno(t)
	const id = "20261015T163651_cc0002"
	ccRunConHijo(t, root, id, pid, arranco)

	if got, ok := Active(root, root); !ok || got != id {
		t.Fatalf("hallazgo cea64f: guarda: el provider vivo (%d, arranco %s) hace activo al run: Active dice %q, %v", pid, arranco, got, ok)
	}
	m := NewManager(root)
	if meta := leerMeta(t, root, id); meta.Status != StatusRunning {
		t.Fatalf("hallazgo cea64f: guarda: un Manager nuevo no marca huerfano al run cuyo provider vive: %+v", meta)
	}
	if got, busy := m.Busy(root); !busy || got != id {
		t.Fatalf("hallazgo cea64f: guarda: con el provider vivo el arbol sigue ocupado: Busy dice %q, %v", got, busy)
	}
}

// ccEnvArranques lleva al proceso ayudante la raiz donde arranca sus runs.
const ccEnvArranques = "HOOM_TEST_ARRANQUES_RAIZ"

// TestAyudanteMuchosArranques no es un test: es el proceso dueno de
// TestHallazgo_e30013_SenalDuranteLosArranquesMataATodos. Arranca runs sin
// parar, desde 4 goroutines y cada uno en su propio directorio, diciendo
// "run" por cada uno, hasta 15 por goroutine; despues espera.
func TestAyudanteMuchosArranques(t *testing.T) {
	root := os.Getenv(ccEnvArranques)
	if root == "" {
		t.Skip("solo corre como proceso ayudante de TestHallazgo_e30013_SenalDuranteLosArranquesMataATodos")
	}
	m := NewManager(root)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 15; i++ {
				dir := filepath.Join(root, fmt.Sprintf("arbol-%d-%d", g, i))
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return
				}
				_, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabajo largo", Dir: dir})
				mu.Lock()
				if err != nil {
					fmt.Printf("error %v\n", err)
				} else {
					fmt.Println("run")
				}
				mu.Unlock()
			}
		}(g)
	}
	wg.Wait()
	time.Sleep(2 * time.Minute)
}

// ccScriptMuchos es el provider falso de los arranques: deja su PID en
// h.<pid> y lanza un nieto (un sleep que no hereda los pipes) que deja el
// suyo en n.<pid> antes de dormir: asi un nieto queda anotado aunque su
// provider muera apenas lo lanzo.
func ccScriptMuchos(d string) string {
	return "tmp='" + d + "'/.t.$$\n" +
		"echo $$ > \"$tmp\" && mv \"$tmp\" '" + d + "'/h.$$\n" +
		"sh -c 't=\"$1\"/.t.$$; echo $$ > \"$t\" && mv \"$t\" \"$1\"/n.$$; exec sleep 296' sh '" + d + "' </dev/null >/dev/null 2>&1 &\n" +
		"echo arranco\n" +
		"wait\n"
}

// ccPID es un PID que dejo un provider (h.<pid>) o su nieto (n.<pid>).
type ccPID struct {
	pid     int
	archivo string
}

// ccPIDs lee los PIDs que dejaron los providers y sus nietos.
func ccPIDs(t *testing.T, d string) []ccPID {
	t.Helper()
	entradas, err := os.ReadDir(d)
	if err != nil {
		t.Fatal(err)
	}
	var pids []ccPID
	for _, e := range entradas {
		if !strings.HasPrefix(e.Name(), "h.") && !strings.HasPrefix(e.Name(), "n.") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(d, e.Name()))
		if err != nil {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && n > 0 {
			pids = append(pids, ccPID{n, e.Name()})
		}
	}
	return pids
}

// ccPs es lo que ps dice de pid, para el mensaje de un fallo.
func ccPs(pid int) string {
	out, _ := exec.Command("ps", "-o", "pid=,ppid=,pgid=,stat=,lstart=,command=", "-p", strconv.Itoa(pid)).CombinedOutput()
	return strings.TrimSpace(string(out))
}

// Hallazgo 20261015T163244_e30013 (medium): un dueno que arranca
// runs sin parar recibe SIGTERM en medio de los arranques. Ningun provider ni
// nieto queda vivo: ni los que ya estaban registrados ni el que estaba
// arrancando cuando llego la senal. La ventana del hallazgo es de
// milisegundos, asi que cada intento la busca mandando la senal despues de
// una cantidad distinta de arranques.
func TestHallazgo_e30013_SenalDuranteLosArranquesMataATodos(t *testing.T) {
	for _, despuesDe := range []int{1, 7, 19, 33} {
		t.Run(fmt.Sprintf("senal-tras-%d-arranques", despuesDe), func(t *testing.T) {
			d := t.TempDir()
			installFake(t, ccScriptMuchos(d))
			root := t.TempDir()
			t.Cleanup(func() {
				for _, p := range ccPIDs(t, d) {
					if m2Vivo(p.pid) {
						syscall.Kill(p.pid, syscall.SIGKILL)
					}
				}
			})

			cmd := exec.Command(os.Args[0], "-test.run=^TestAyudanteMuchosArranques$", "-test.count=1")
			cmd.Env = append(os.Environ(), ccEnvArranques+"="+root)
			stdout, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			fin := make(chan struct{})
			arranques := make(chan string, 128)
			go func() {
				s := bufio.NewScanner(stdout)
				for s.Scan() {
					select {
					case arranques <- s.Text():
					default:
					}
				}
				cmd.Wait()
				close(fin)
			}()
			t.Cleanup(func() {
				cmd.Process.Kill()
				<-fin
			})

			vistos := 0
			for vistos < despuesDe {
				select {
				case l := <-arranques:
					if l != "run" {
						t.Fatalf("CA-346: fixture: el proceso dueno arranca sus runs: %q", l)
					}
					vistos++
				case <-fin:
					t.Fatalf("CA-346: fixture: el proceso dueno termino despues de %d arranques", vistos)
				case <-time.After(30 * time.Second):
					t.Fatalf("CA-346: fixture: el proceso dueno no arranco %d runs en 30 s (van %d)", despuesDe, vistos)
				}
			}
			if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
				t.Fatal(err)
			}
			select {
			case <-fin:
			case <-time.After(30 * time.Second):
				t.Fatal("hallazgo e30013: el proceso dueno no termino en 30 s despues de SIGTERM")
			}
			time.Sleep(time.Second) // un provider que sobrevivio ya dejo sus PIDs
			// La senal puede matar a un provider antes de que deje su PID:
			// eso es lo que se pide, y entonces no hay nada que mirar de el.
			pids := ccPIDs(t, d)
			t.Logf("%d PIDs de providers y nietos despues de %d arranques", len(pids), vistos)
			for _, p := range pids {
				if !m2Muere(p.pid, 5*time.Second) {
					t.Fatalf("hallazgo e30013: con SIGTERM al dueno en medio de los arranques, ningun provider ni nieto queda vivo: %d (%s) sigue vivo, de %d PIDs\n%s", p.pid, p.archivo, len(pids), ccPs(p.pid))
				}
			}
		})
	}
}
