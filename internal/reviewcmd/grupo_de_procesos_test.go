// El grupo de procesos de un comando que lanza un test, y sus pruebas (tercera
// review de estabilizar-ci: hallazgos 20261007T142544_ef9963,
// 20261007T141141_5b4172 / 20261007T142507_4c2a57 y 20261007T142727_b942f0).
//
// Un comando que lanza a otros (un CLI falso, cuyo cuerpo corre en un
// subshell; el medidor, que lanza a hoom; este binario vuelto a ejecutar) no
// se corta matando al proceso que se lanzo: los demas siguen, con sus pipes
// abiertos. Se corta su GRUPO de procesos, y este archivo es el unico del
// paquete que arma uno y le manda una senal.
//
// El grupo se nombra con el pid de su lider, que deja de ser suyo cuando al
// lider lo recolectan y no queda nadie mas en el grupo. Por eso el corte va
// ANTES de recolectar al lider (recolectar, que es tambien lo que corre cuando
// el test termina), y con el lider ya anotado como recolectado cortar no manda
// nada. Lo que NO cierra: entre que el kernel recolecta al lider, adentro de
// cmd.Wait, y que cmd.Wait vuelve y queda anotado, un cortar todavia manda la
// senal: son microsegundos, o hasta el WaitDelay si un descendiente conserva
// un pipe del comando (y entonces el grupo lo tiene de miembro). El hallazgo
// 20261007T141236_786f65 pedia cerrar esa ventana y quedo REFUTADO con
// mediciones: el pid de un lider recolectado y sin miembros recien vuelve a
// usarse despues de la vuelta entera de numeros (74 s donde se midio), el de
// uno con un miembro vivo no vuelve, y la peor ventana medida entre recolectar
// y cortar fue de 9,7 ms. Tampoco alcanza a lo que un lider deja detras cuando
// termina SOLO (ya esta recolectado) ni al que se fue del grupo.
package reviewcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// grupoEsteArchivo es este archivo: el unico del paquete que puede armar un
// grupo de procesos y mandarle una senal.
const grupoEsteArchivo = "grupo_de_procesos_test.go"

// grupoPlazoDeLosPipes es el WaitDelay de un comando que no trae el suyo: su
// Wait no se queda esperando un pipe que conserva alguien a quien el corte no
// alcanzo.
const grupoPlazoDeLosPipes = 2 * time.Second

// grupoDeProcesos es el grupo de procesos de un comando que lanzo un test: el
// comando, que es su lider, y lo que el comando haya lanzado.
type grupoDeProcesos struct {
	cmd *exec.Cmd
	// senal es syscall.Kill: un campo, para que un test vea que senales salen
	senal func(pid int, sig syscall.Signal) error

	una         sync.Once // el Wait del lider, y err lo que devolvio
	err         error
	mu          sync.Mutex // cuida recolectado
	recolectado bool       // el Wait del lider ya volvio
}

// grupoArrancar lanza cmd en un grupo de procesos propio, del que es el lider.
// Quien lo llama no lo espera ni le manda senales por su cuenta: lo hace por
// el grupo. Cuando t termina, haya salido por donde haya salido, el grupo
// queda cortado y su lider recolectado.
func grupoArrancar(t testing.TB, cmd *exec.Cmd) (*grupoDeProcesos, error) {
	t.Helper()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = grupoPlazoDeLosPipes
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	g := &grupoDeProcesos{cmd: cmd, senal: syscall.Kill}
	t.Cleanup(func() { _ = g.recolectar() })
	return g, nil
}

// cortar mata al grupo entero (SIGKILL): al lider y a lo que lanzo y sigue en
// su grupo. Con el lider ya recolectado no manda nada.
func (g *grupoDeProcesos) cortar() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.recolectado {
		_ = g.senal(-g.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// esperar espera a que el lider termine SOLO y lo recolecta: devuelve lo que
// devuelve cmd.Wait, se lo llame las veces que sea y desde donde sea.
func (g *grupoDeProcesos) esperar() error {
	g.una.Do(func() {
		g.err = g.cmd.Wait()
		g.mu.Lock()
		g.recolectado = true
		g.mu.Unlock()
	})
	return g.err
}

// recolectar corta al grupo y RECIEN ENTONCES recolecta a su lider: cuando
// vuelve no queda nadie del grupo. Devuelve lo mismo que esperar.
func (g *grupoDeProcesos) recolectar() error {
	g.cortar()
	return g.esperar()
}

// grupoCorrer es el Run de un comando que lanza a otros (y su CombinedOutput,
// con un mismo buffer en cmd.Stdout y cmd.Stderr): lo corre en un grupo propio
// hasta que su lider termina o hasta que llega algo por vence (un time.After),
// y entonces corta al grupo ENTERO. El reloj de un exec.CommandContext mata
// solo al hijo directo: lo que lanzo sigue vivo, y su Wait espera el fin de
// archivo de los pipes que conserva (hallazgo ef9963). vencio dice que hubo
// que cortarlo; err es el de cmd.Wait, o el de no haber podido lanzarlo (y
// entonces cmd.ProcessState es nil).
func grupoCorrer(t testing.TB, cmd *exec.Cmd, vence <-chan time.Time) (vencio bool, err error) {
	t.Helper()
	g, err := grupoArrancar(t, cmd)
	if err != nil {
		return false, err
	}
	termino := make(chan struct{})
	go func() {
		defer close(termino)
		_ = g.esperar()
	}()
	select {
	case <-termino:
	case <-vence:
		vencio = true
	}
	return vencio, g.recolectar()
}

// grupoSeguirAQuienLoLanzo es para el lider de un grupo que es este mismo
// binario (el medidor de hoom): si quien lo lanzo muere sin haberlo cortado
// (lo corto a el un reloj de mas afuera, que a este grupo no llega), el lider
// mata a su propio grupo (kill con pid 0), que si no quedaria vivo y sin nadie
// que lo corte. Solo si es el lider: si no, ese grupo es de otro.
func grupoSeguirAQuienLoLanzo() {
	lanzo := os.Getppid()
	go func() {
		for os.Getppid() == lanzo {
			time.Sleep(50 * time.Millisecond)
		}
		if syscall.Getpgrp() == os.Getpid() {
			_ = syscall.Kill(0, syscall.SIGKILL)
		}
	}()
}

// testigoDeVida le dice a un test si queda vivo alguno de los procesos que
// lanzo, sin contar pids: es un FIFO que el test tiene abierto para leer. El
// proceso que quiere ser visto lo abre (sh, las lineas que el proceso corre) y
// escribe testigoListo; el descriptor lo heredan sus hijos. Mientras alguno lo
// tenga abierto, leer del FIFO no da fin de archivo; cuando no queda ninguno,
// lo da. Un proceso muerto no tiene descriptores, aunque nadie lo haya
// esperado todavia.
type testigoDeVida struct {
	ruta  string
	fd    int
	dicho strings.Builder // lo que escribieron los que lo abrieron
}

// testigoListo es lo que escribe en el testigo el proceso que lo abre.
const testigoListo = "listo\n"

func nuevoTestigoDeVida(t *testing.T) *testigoDeVida {
	t.Helper()
	w := &testigoDeVida{ruta: filepath.Join(t.TempDir(), "testigo")}
	if err := syscall.Mkfifo(w.ruta, 0o600); err != nil {
		t.Fatalf("fixture: no pude crear el FIFO testigo: %v", err)
	}
	// sin bloquear: no hay todavia quien escriba, y se lee de a ratos
	fd, err := syscall.Open(w.ruta, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		t.Fatalf("fixture: no pude abrir el FIFO testigo: %v", err)
	}
	w.fd = fd
	t.Cleanup(func() { _ = syscall.Close(fd) })
	return w
}

// sh son las lineas de sh con las que un proceso toma el testigo: lo deja
// abierto en su descriptor 8 y avisa. Lo abre para leer y escribir, que no
// espera a nadie: abierto solo para escribir, un proceso cuyo test ya murio se
// quedaria esperando un lector para siempre.
func (w *testigoDeVida) sh() string {
	return "exec 8<>'" + w.ruta + "'\nprintf 'listo\\n' >&8\n"
}

// espera lee del testigo hasta que no queda ningun proceso que lo tenga
// abierto (nadie) o hasta que pasa plazo. arranco dice si alguno llego a
// tomarlo: sin eso, que no quede nadie no dice nada.
func (w *testigoDeVida) espera(plazo time.Duration) (arranco, nadie bool) {
	buf := make([]byte, 256)
	for fin := time.Now().Add(plazo); ; {
		n, err := syscall.Read(w.fd, buf)
		switch {
		case n > 0:
			w.dicho.Write(buf[:n])
			continue
		case err == nil: // fin de archivo: nadie lo tiene abierto para escribir
			return w.dicho.String() == testigoListo, true
		case err != syscall.EAGAIN && err != syscall.EINTR:
			return w.dicho.String() == testigoListo, false
		}
		// alguien lo tiene abierto y no escribio nada mas
		if !time.Now().Before(fin) {
			return w.dicho.String() == testigoListo, false
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// shConUnoDetras son las lineas de sh de un proceso que toma el testigo, deja
// detras un sleep de 30 s que lo hereda (como hereda su stdout y su stderr;
// masRedirecciones va en esa linea) y recien entonces avisa. Para quedarse el
// tambien, el proceso sigue con `wait`.
//
// Antes de avisar le da 50 ms al sleep para arrancar, y despues no lanza nada
// mas: en macOS un SIGKILL al grupo puede no alcanzar a un proceso que esta en
// medio de su exec (medido aca con dash: en 5 de 600 cortes hechos apenas el
// shell avisaba quedo vivo el sleep; en ninguno de 300 con 5 ms de por medio).
func (w *testigoDeVida) shConUnoDetras(masRedirecciones string) string {
	return "exec 8<>'" + w.ruta + "'\nsleep 30 " + masRedirecciones + " &\nsleep 0.05\nprintf 'listo\\n' >&8\n"
}

// alTomarlo es un reloj (el vence de grupoCorrer) que vence cuando algun
// proceso toma el testigo y avisa: para cortar a un comando recien cuando ya
// corre, sin acertarle a cuanto tarda en arrancar. Si nadie lo toma vence
// igual, a los 30 s. Hasta que vence, el testigo es de este reloj: no se lo
// lee de otro lado.
func (w *testigoDeVida) alTomarlo(t *testing.T) <-chan time.Time {
	vence, fin := make(chan time.Time, 1), make(chan struct{})
	go func() {
		defer close(fin)
		// antes de que alguien lo abra, leer da fin de archivo: es "todavia no"
		for hasta := time.Now().Add(30 * time.Second); time.Now().Before(hasta); time.Sleep(time.Millisecond) {
			if arranco, _ := w.espera(0); arranco {
				break
			}
		}
		vence <- time.Now()
	}()
	t.Cleanup(func() { <-fin }) // corre antes que el Cleanup que cierra el descriptor
	return vence
}

// ---------------------------------------------------------------- tests

// grupoShells son los shells con los que corren las pruebas del grupo: el
// arbol de procesos de un mismo `-c` no es igual en todos (cual hace exec del
// ultimo comando, cual lanza un subshell). En ubuntu (el CI) /bin/sh ES dash.
func grupoShells() []string {
	if _, err := os.Stat("/bin/dash"); err != nil {
		return []string{"/bin/sh"}
	}
	return []string{"/bin/sh", "/bin/dash"}
}

// Lo que queda del hallazgo 786f65 (refutado: lo cuenta la cabecera de este
// archivo): recolectar corta al grupo ANTES de recolectar a su lider, y no
// queda nadie; y una vez recolectado, cortar no manda ninguna senal, se lo
// llame las veces que sea. El comando no termina solo: si recolectar lo
// esperara antes de cortarlo, saldria con 0 medio minuto despues.
func TestHallazgo_786f65_DespuesDeRecolectarCortarNoMandaNada(t *testing.T) {
	testigo := nuevoTestigoDeVida(t)
	cmd := exec.Command("/bin/sh", "-c", testigo.shConUnoDetras("")+"wait\n")
	g, err := grupoArrancar(t, cmd)
	if err != nil {
		t.Fatalf("fixture: no pude lanzar /bin/sh: %v", err)
	}
	var salieron []int // a que pid salio cada senal
	g.senal = func(pid int, sig syscall.Signal) error {
		salieron = append(salieron, pid)
		return syscall.Kill(pid, sig)
	}
	grupo := -cmd.Process.Pid
	<-testigo.alTomarlo(t)

	_ = g.recolectar()
	if !slices.Equal(salieron, []int{grupo}) || cmd.ProcessState.ExitCode() != -1 {
		t.Fatalf("recolectar corta al grupo (una senal a %d) y recien entonces recolecta a su lider, al que mato esa senal (exit -1): salieron %v, %v",
			grupo, salieron, cmd.ProcessState)
	}
	if arranco, nadie := testigo.espera(10 * time.Second); !arranco || !nadie {
		t.Fatalf("cuando recolectar vuelve no queda nadie del grupo: el comando tomo el testigo %v, 10 s despues no queda nadie %v", arranco, nadie)
	}
	for range 3 {
		g.cortar()
		_ = g.recolectar()
	}
	if len(salieron) != 1 {
		t.Fatalf("una vez recolectado el lider, cortar no manda ninguna senal (el numero del grupo ya puede no ser suyo): salieron %v", salieron[1:])
	}
}

// Hallazgo ef9963: cuando vence el reloj de grupoCorrer muere el grupo ENTERO
// y grupoCorrer vuelve enseguida, aunque un descendiente del comando tenga
// abierto su stdout. El comando imprime, deja un sleep de 30 s detras (con el
// testigo, y con el stdout que hereda, un pipe), avisa y se queda esperandolo.
// Con un exec.CommandContext moria solo el shell: el sleep seguia vivo y Wait
// esperaba el fin de archivo de ese pipe.
//
// Control: el que termina solo antes del reloj no vence, sale con lo suyo y
// su salida llega entera.
func TestHallazgo_ef9963_AlVencerElRelojMuereElGrupoEnteroYNadieEsperaUnPipe(t *testing.T) {
	for _, sh := range grupoShells() {
		t.Run(filepath.Base(sh), func(t *testing.T) {
			testigo := nuevoTestigoDeVida(t)
			t.Parallel()
			cmd := exec.Command(sh, "-c", "echo corriendo\n"+testigo.shConUnoDetras("")+"wait\necho no-llega\n")
			var salida bytes.Buffer
			cmd.Stdout, cmd.Stderr = &salida, &salida
			antes := time.Now()
			vencio, err := grupoCorrer(t, cmd, testigo.alTomarlo(t))
			tardo := time.Since(antes)
			if !vencio || cmd.ProcessState.ExitCode() != -1 {
				t.Fatalf("hallazgo ef9963: un comando que no termina queda cortado por el reloj, sin codigo de salida (-1): vencio %v, %v (%v)\n%s", vencio, cmd.ProcessState, err, salida.String())
			}
			if tardo > 15*time.Second {
				t.Fatalf("hallazgo ef9963: cuando vence el reloj grupoCorrer vuelve enseguida: tardo %v, lo que tarda en soltar el pipe el sleep de 30 s", tardo)
			}
			if salida.String() != "corriendo\n" {
				t.Fatalf("hallazgo ef9963: lo que el comando imprimio antes del corte queda en su salida, y nada mas: %q", salida.String())
			}
			if arranco, nadie := testigo.espera(10 * time.Second); !arranco || !nadie {
				t.Fatalf("hallazgo ef9963: cuando vence el reloj muere el grupo entero: el comando tomo el testigo %v, 10 s despues no queda vivo ni el ni el sleep que lanzo %v", arranco, nadie)
			}
		})

		t.Run(filepath.Base(sh)+"/control-termina-solo", func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(sh, "-c", "echo a stdout\n"+sh+" -c 'echo del hijo'\necho a stderr >&2\nexit 7\n")
			var o, e bytes.Buffer
			cmd.Stdout, cmd.Stderr = &o, &e
			vencio, err := grupoCorrer(t, cmd, time.After(30*time.Second))
			if vencio || err == nil || cmd.ProcessState.ExitCode() != 7 {
				t.Fatalf("un comando que sale con 7 antes del reloj no vence y sale con 7: vencio %v, %v", vencio, err)
			}
			if o.String() != "a stdout\ndel hijo\n" || e.String() != "a stderr\n" {
				t.Fatalf("la salida de un comando que termino solo llega entera: stdout %q, stderr %q", o.String(), e.String())
			}
			if cmd.WaitDelay != grupoPlazoDeLosPipes {
				t.Fatalf("un comando que no trae su WaitDelay arranca con el del grupo (%v): tiene %v", grupoPlazoDeLosPipes, cmd.WaitDelay)
			}
		})
	}
}

// Lo que queda del hallazgo 786f65: ningun otro _test.go del paquete le manda
// una senal a un grupo de procesos (syscall.Kill con un pid negativo) ni arma
// uno (Setpgid), ni los nombra. El que tiene que cortar a un comando y a lo
// que lanzo lo arranca con grupoArrancar o lo corre con grupoCorrer: ahi el
// corte va antes de recolectar al lider, y despues no sale.
func TestHallazgo_786f65_NingunTestMandaSenalesAUnGrupoAMano(t *testing.T) {
	archivos, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(archivos, grupoEsteArchivo) {
		t.Skipf("los fuentes de los tests no estan en el directorio de trabajo: no se puede mirar quien manda senales (falta %s)", grupoEsteArchivo)
	}
	for _, a := range archivos {
		if a == grupoEsteArchivo {
			continue
		}
		raw, err := os.ReadFile(a)
		if err != nil {
			t.Fatal(err)
		}
		for _, aMano := range []string{"syscall.Kill", "Setpgid"} {
			if n := strings.Count(string(raw), aMano); n != 0 {
				t.Errorf("%s nombra %s %d veces: a un comando y a lo que lanzo se los corta por su grupo de procesos, y eso lo hace solo %s "+
					"(grupoArrancar o grupoCorrer, y despues cortar o recolectar): ahi el corte va antes de recolectar al lider y despues no sale", a, aMano, n, grupoEsteArchivo)
			}
		}
	}
}
