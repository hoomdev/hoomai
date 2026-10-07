// El grupo de procesos de un comando que lanza un test, y sus pruebas (tercera
// review de estabilizar-ci: hallazgos 20261007T142544_ef9963,
// 20261007T141141_5b4172 / 20261007T142507_4c2a57 y 20261007T142727_b942f0; y
// cuarta: 20261007T171925_139218 / 20261007T172254_c89063 y
// 20261007T172629_954d37).
//
// Un comando que lanza a otros (un CLI falso, cuyo cuerpo corre en un
// subshell; el medidor, que lanza a hoom; este binario vuelto a ejecutar) no
// se corta matando al proceso que se lanzo: los demas siguen, con sus pipes
// abiertos. Se corta su GRUPO de procesos, y este archivo es el unico del
// paquete que arma uno y le manda una senal.
//
// El grupo se nombra con el pid de su lider, que deja de ser suyo cuando al
// lider lo recolectan y no queda nadie mas en el grupo. Por eso los cortes son
// dos y nada mas: uno ANTES de recolectar al lider (recolectar, que es tambien
// lo que corre cuando el test termina) y uno apenas vuelve su Wait (esperar),
// para lo que un lider que termino SOLO haya dejado en su grupo (hallazgos
// 139218 y c89063). Con el lider ya anotado como recolectado, cortar no manda
// nada.
//
// El corte de despues del Wait sale con el lider ya recolectado, y solo hace
// algo cuando el numero sigue siendo del grupo. Si queda alguien del grupo, el
// numero es del grupo: POSIX no deja reusar un pid mientras exista un grupo
// con ese id (medido aca: el pid de un lider recolectado con un miembro vivo
// no volvio en toda la vuelta de numeros). Si no queda nadie, la senal no le
// llega a nadie: el kill da ESRCH (401 de 401 medidos), o EPERM en macOS si
// del grupo solo quedan zombies. Y para que el numero fuera de OTRO grupo el
// kernel tendria que dar la vuelta entera de pids entre que recolecta al lider
// y que sale la senal: el pid de un lider recolectado y sin miembros tardo 74
// s en volver, y de que cmd.Wait vuelve a que sale el kill hay menos de 1
// microsegundo de mediana y 74 de maximo (383 medidos, todos sin nadie a quien
// alcanzar). Cerrar esa ventana es lo que pedia el hallazgo
// 20261007T141236_786f65, que quedo REFUTADO con mediciones (la peor ventana
// entre recolectar y cortar: 9,7 ms).
//
// El limite de eso: cmd.Wait recolecta al lider y despues espera los pipes del
// comando, hasta el WaitDelay. Si los conserva alguien del grupo, el numero
// sigue siendo del grupo. Si los conserva un descendiente que se FUE del grupo
// (setsid), el corte de despues sale hasta 2 s despues de que el lider murio,
// con el grupo vacio todo ese tiempo: 2 s contra los 74 s de la vuelta. Hoy
// ningun fixture ni hoom se sale de su grupo; y al que se fue, ademas, ningun
// corte lo alcanza.
//
// Y un corte solo puede no alcanzar al proceso que el grupo esta lanzando en
// ese instante: medido aca (macOS, dash), cortando apenas el shell avisaba
// quedo vivo en 13 de 3000; con los dos cortes de recolectar, en 0 de 3000
// (con /bin/sh, 0 de 3000 de las dos maneras). Es una medicion, no una
// garantia.
package reviewcmd

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// grupoPlazoDeLosPipes es el WaitDelay de un comando que no trae el suyo: su
// Wait no se queda esperando un pipe que conserva alguien que el comando dejo
// detras. Que se cumpla es un error (esperar), no una salida mas.
const grupoPlazoDeLosPipes = 2 * time.Second

// grupoDeProcesos es el grupo de procesos de un comando que lanzo un test: el
// comando, que es su lider, y lo que el comando haya lanzado.
type grupoDeProcesos struct {
	cmd *exec.Cmd
	// senal es syscall.Kill: un campo, para que un test vea que senales salen
	senal func(pid int, sig syscall.Signal) error

	pipes       []*grupoPipe // el stdout y el stderr del comando
	una         sync.Once    // el Wait del lider, y err lo que devolvio
	err         error
	mu          sync.Mutex // cuida recolectado
	recolectado bool       // el Wait del lider ya volvio y el corte de despues ya salio
}

// grupoPipe es el stdout o el stderr de un comando, mirado por su grupo.
// exec.Cmd copia cada pipe con io.Copy, que lee por aca (ReadFrom): si la
// lectura termina con un error y no con el fin de archivo, al pipe lo cerro el
// WaitDelay con alguien todavia del otro lado. cmd.Wait eso lo dice
// (exec.ErrWaitDelay) solo cuando el lider salio con 0: si no, lo calla.
type grupoPipe struct {
	io.Writer
	conservado bool
}

func (p *grupoPipe) ReadFrom(r io.Reader) (int64, error) {
	n, err := io.Copy(p.Writer, r)
	p.conservado = err != nil
	return n, err
}

// mirar devuelve w mirado por el grupo, si exec.Cmd le va a armar un pipe: ni
// nil ni un archivo.
func (g *grupoDeProcesos) mirar(w io.Writer) io.Writer {
	if _, archivo := w.(*os.File); w == nil || archivo {
		return w
	}
	p := &grupoPipe{Writer: w}
	g.pipes = append(g.pipes, p)
	return p
}

// grupoArrancar lanza cmd en un grupo de procesos propio, del que es el lider.
// Quien lo llama no lo espera ni le manda senales por su cuenta: lo hace por
// el grupo. Cuando t termina, haya salido por donde haya salido y haya
// terminado el lider como haya terminado, el grupo queda cortado y su lider
// recolectado.
func grupoArrancar(t testing.TB, cmd *exec.Cmd) (*grupoDeProcesos, error) {
	t.Helper()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if cmd.WaitDelay == 0 {
		cmd.WaitDelay = grupoPlazoDeLosPipes
	}
	g := &grupoDeProcesos{cmd: cmd, senal: syscall.Kill}
	junta := cmd.Stdout != nil && cmd.Stdout == cmd.Stderr // un solo pipe para los dos
	cmd.Stdout = g.mirar(cmd.Stdout)
	if junta {
		cmd.Stderr = cmd.Stdout
	} else {
		cmd.Stderr = g.mirar(cmd.Stderr)
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	t.Cleanup(func() { _ = g.recolectar() })
	return g, nil
}

// cortar mata al grupo entero (SIGKILL): al lider y a lo que lanzo y sigue en
// su grupo. Con el lider ya anotado como recolectado no manda nada.
func (g *grupoDeProcesos) cortar() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.recolectado {
		_ = g.senal(-g.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// esperar espera a que el lider termine SOLO, lo recolecta y corta lo que
// haya dejado en su grupo (el corte de despues: la cabecera dice por que no le
// cae a otro): cuando vuelve no queda nadie del grupo. Devuelve lo que
// devuelve cmd.Wait, se lo llame las veces que sea y desde donde sea; salvo
// que el WaitDelay haya tenido que cerrar el stdout o el stderr del comando
// con alguien todavia del otro lado: entonces es un exec.ErrWaitDelay, haya
// salido el lider con lo que haya salido. Esa salida no esta entera.
func (g *grupoDeProcesos) esperar() error {
	g.una.Do(func() {
		g.err = g.cmd.Wait()
		conservado := slices.ContainsFunc(g.pipes, func(p *grupoPipe) bool { return p.conservado })
		if conservado && !errors.Is(g.err, exec.ErrWaitDelay) {
			g.err = fmt.Errorf("%w (el lider: %v)", exec.ErrWaitDelay, g.err)
		}
		g.mu.Lock()
		defer g.mu.Unlock()
		_ = g.senal(-g.cmd.Process.Pid, syscall.SIGKILL)
		g.recolectado = true
	})
	return g.err
}

// recolectar corta al grupo y RECIEN ENTONCES recolecta a su lider, sin
// esperar a que termine solo. Devuelve lo mismo que esperar.
func (g *grupoDeProcesos) recolectar() error {
	g.cortar()
	return g.esperar()
}

// grupoCorrer es el Run de un comando que lanza a otros (y su CombinedOutput,
// con un mismo buffer en cmd.Stdout y cmd.Stderr): lo corre en un grupo propio
// hasta que su lider termina o hasta que llega algo por vence (un time.After),
// y entonces corta al grupo ENTERO. Termine como termine, cuando vuelve no
// queda nadie del grupo. El reloj de un exec.CommandContext mata solo al hijo
// directo: lo que lanzo sigue vivo, y su Wait espera el fin de archivo de los
// pipes que conserva (hallazgo ef9963). vencio dice que hubo que cortarlo; err
// es el de cmd.Wait, o el de no haber podido lanzarlo (y entonces
// cmd.ProcessState es nil).
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
// mas: un corte puede no alcanzar al proceso que el grupo esta lanzando en ese
// instante (la cabecera de este archivo trae la medicion), y lo que estos
// tests miran no es eso.
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

// grupoSenalVista es una senal que salio de un grupoDeProcesos: a que pid, y
// si salio con el Wait del lider ya vuelto.
type grupoSenalVista struct {
	pid           int
	despuesDeWait bool
}

// grupoEspiar anota las senales que salen de g, que salen igual. Devuelve
// donde quedan anotadas. Es para el test que usa a g desde una sola goroutine.
func grupoEspiar(g *grupoDeProcesos) *[]grupoSenalVista {
	vistas := new([]grupoSenalVista)
	g.senal = func(pid int, sig syscall.Signal) error {
		*vistas = append(*vistas, grupoSenalVista{pid, g.cmd.ProcessState != nil})
		return syscall.Kill(pid, sig)
	}
	return vistas
}

// Lo que queda del hallazgo 786f65 (refutado: lo cuenta la cabecera de este
// archivo), y los hallazgos 139218 y c89063: que senales salen de un grupo, y
// cuando. Todas son al grupo (-pid).
//
//   - por recolectar, dos: una ANTES de recolectar al lider, que es la que lo
//     mata, y el corte de despues. El comando no termina solo: si recolectar
//     lo esperara antes de cortarlo, saldria con 0 medio minuto despues;
//   - por el camino natural (esperar a un lider que termina solo), una: el
//     corte de despues, que no le cambia con que salio;
//   - con el lider ya anotado como recolectado, ninguna mas, se llame a lo
//     que se llame y las veces que sea.
func TestHallazgo_786f65_DespuesDeRecolectarCortarNoMandaNada(t *testing.T) {
	despues := func(t *testing.T, g *grupoDeProcesos, vistas *[]grupoSenalVista) {
		t.Helper()
		salieron := len(*vistas)
		for range 3 {
			g.cortar()
			_ = g.esperar()
			_ = g.recolectar()
		}
		if len(*vistas) != salieron {
			t.Fatalf("una vez recolectado el lider no sale ninguna senal mas (el numero del grupo ya puede no ser suyo): salieron %+v", (*vistas)[salieron:])
		}
	}

	t.Run("por-recolectar", func(t *testing.T) {
		testigo := nuevoTestigoDeVida(t)
		cmd := exec.Command("/bin/sh", "-c", testigo.shConUnoDetras("")+"wait\n")
		g, err := grupoArrancar(t, cmd)
		if err != nil {
			t.Fatalf("fixture: no pude lanzar /bin/sh: %v", err)
		}
		vistas, grupo := grupoEspiar(g), -cmd.Process.Pid
		<-testigo.alTomarlo(t)

		_ = g.recolectar()
		if !slices.Equal(*vistas, []grupoSenalVista{{grupo, false}, {grupo, true}}) || cmd.ProcessState.ExitCode() != -1 {
			t.Fatalf("recolectar corta al grupo (%d) antes de recolectar a su lider, al que mata esa senal (exit -1), y una vez mas despues: salieron %+v, %v",
				grupo, *vistas, cmd.ProcessState)
		}
		if arranco, nadie := testigo.espera(10 * time.Second); !arranco || !nadie {
			t.Fatalf("cuando recolectar vuelve no queda nadie del grupo: el comando tomo el testigo %v, 10 s despues no queda nadie %v", arranco, nadie)
		}
		despues(t, g, vistas)
	})

	t.Run("por-el-camino-natural", func(t *testing.T) {
		cmd := exec.Command("/bin/sh", "-c", "exit 3")
		g, err := grupoArrancar(t, cmd)
		if err != nil {
			t.Fatalf("fixture: no pude lanzar /bin/sh: %v", err)
		}
		vistas, grupo := grupoEspiar(g), -cmd.Process.Pid

		if err := g.esperar(); err == nil || cmd.ProcessState.ExitCode() != 3 {
			t.Fatalf("esperar devuelve lo que devuelve el Wait de un comando que salio con 3: %v, %v", err, cmd.ProcessState)
		}
		if !slices.Equal(*vistas, []grupoSenalVista{{grupo, true}}) {
			t.Fatalf("hallazgos 139218 y c89063: por el camino natural sale UN corte al grupo (%d), apenas vuelve el Wait del lider: salieron %+v", grupo, *vistas)
		}
		despues(t, g, vistas)
	})
}

// Hallazgos 139218 y c89063: un lider que termina SOLO dejando un proceso vivo
// en su grupo no deja a nadie. El comando lanza un sleep de 30 s que no
// conserva ninguno de sus pipes (nadie se queda esperandolos: Wait vuelve
// enseguida) y sale con 0. Antes, apenas volvia ese Wait el lider quedaba
// anotado como recolectado, nadie mas cortaba al grupo y el sleep seguia vivo,
// despues del test.
//
// Por los tres caminos por los que se llega a un lider que termino solo: el de
// grupoCorrer; esperar a mano, y que el test termine; y que el test termine
// sin haber esperado a nadie (ahi corta el Cleanup, antes de recolectar). El
// testigo se mira con el test del comando ya terminado.
//
// Y el que deja a alguien CON su stdout tomado (lo que el arreglo de ef9963
// habia ablandado; lo midio el refutador de la cuarta review). Sin WaitDelay
// su Wait no volvia y lo cortaba el reloj del que lo corria, en rojo; con el
// WaitDelay del grupo volvia a los 2 s como si nada. Ahora vuelve igual, pero
// con un exec.ErrWaitDelay, salga el lider con 0 o con otra cosa (que es
// cuando cmd.Wait lo calla), con lo que llego a imprimir y sin dejar vivo al
// que conservaba el pipe.
func TestHallazgo_139218_c89063_LoQueDejaUnLiderQueTerminaSoloNoQuedaVivo(t *testing.T) {
	caminos := []struct {
		nombre string
		correr func(t *testing.T, cmd *exec.Cmd, testigo *testigoDeVida) error
	}{
		{"grupoCorrer", func(t *testing.T, cmd *exec.Cmd, _ *testigoDeVida) error {
			vencio, err := grupoCorrer(t, cmd, time.After(30*time.Second))
			if vencio {
				t.Fatalf("fixture: un comando que sale solo no llega al reloj de 30 s: %v", err)
			}
			return err
		}},
		{"esperar-a-mano", func(t *testing.T, cmd *exec.Cmd, _ *testigoDeVida) error {
			g, err := grupoArrancar(t, cmd)
			if err != nil {
				return err
			}
			return g.esperar()
		}},
		{"solo-el-Cleanup", func(t *testing.T, cmd *exec.Cmd, testigo *testigoDeVida) error {
			if _, err := grupoArrancar(t, cmd); err != nil {
				return err
			}
			<-testigo.alTomarlo(t)
			return nil
		}},
	}
	for _, sh := range grupoShells() {
		for _, c := range caminos {
			t.Run(filepath.Base(sh)+"/"+c.nombre, func(t *testing.T) {
				testigo := nuevoTestigoDeVida(t)
				t.Parallel()
				t.Run("el-comando", func(t *testing.T) {
					cmd := exec.Command(sh, "-c", testigo.shConUnoDetras(">/dev/null 2>&1")+"exit 0\n")
					var salida bytes.Buffer
					cmd.Stdout, cmd.Stderr = &salida, &salida
					if err := c.correr(t, cmd, testigo); err != nil {
						t.Fatalf("fixture: el comando sale solo y con 0: %v\n%s", err, salida.String())
					}
				})
				if arranco, nadie := testigo.espera(10 * time.Second); !arranco || !nadie {
					t.Fatalf("hallazgos 139218 y c89063: de un lider que termino solo no queda vivo lo que dejo en su grupo: el comando tomo el testigo %v, 10 s despues no queda nadie %v", arranco, nadie)
				}
			})
		}

		// y el que deja a alguien CON su stdout tomado: lo de abajo
		for _, exit := range []int{0, 3} {
			t.Run(filepath.Base(sh)+"/con-su-salida-tomada-exit-"+strconv.Itoa(exit), func(t *testing.T) {
				testigo := nuevoTestigoDeVida(t)
				t.Parallel()
				cmd := exec.Command(sh, "-c", testigo.shConUnoDetras("")+"echo dicho\nexit "+strconv.Itoa(exit)+"\n")
				cmd.WaitDelay = 10 * time.Millisecond
				var salida bytes.Buffer
				cmd.Stdout, cmd.Stderr = &salida, &salida
				vencio, err := grupoCorrer(t, cmd, time.After(30*time.Second))
				if vencio || !errors.Is(err, exec.ErrWaitDelay) {
					t.Fatalf("un comando que sale con %d dejando a otro con su stdout tomado vuelve, sin llegar al reloj, con un exec.ErrWaitDelay: vencio %v, %v", exit, vencio, err)
				}
				if cmd.ProcessState.ExitCode() != exit || salida.String() != "dicho\n" {
					t.Fatalf("del comando quedan su codigo de salida (%d) y lo que llego a imprimir: %v, %q", exit, cmd.ProcessState, salida.String())
				}
				if arranco, nadie := testigo.espera(10 * time.Second); !arranco || !nadie {
					t.Fatalf("hallazgos 139218 y c89063: el que conservaba el stdout tampoco queda vivo: el comando tomo el testigo %v, 10 s despues no queda nadie %v", arranco, nadie)
				}
			})
		}
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

// grupoUsosAMano son los lugares de un fuente de Go (src, de nombre archivo)
// que le mandan una senal a un proceso o arman un grupo de procesos por su
// cuenta: una llamada a Kill del paquete syscall, con el nombre con que ese
// fuente lo importe, y el campo Setpgid, en un literal de syscall.SysProcAttr
// o asignado. Mira el codigo: lo que solo lo nombra (un comentario, una
// cadena, el `kill` del script de un fixture) no es un uso.
func grupoUsosAMano(archivo string, src []byte) ([]string, error) {
	if !bytes.Contains(src, []byte("Kill")) && !bytes.Contains(src, []byte("Setpgid")) {
		return nil, nil // ni los nombra: no hace falta parsearlo
	}
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, archivo, src, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var syscalls []string // los nombres de syscall en este fuente
	for _, imp := range f.Imports {
		switch {
		case imp.Path.Value != strconv.Quote("syscall"):
		case imp.Name == nil:
			syscalls = append(syscalls, "syscall")
		default:
			syscalls = append(syscalls, imp.Name.Name)
		}
	}
	// deSyscall: e nombra a ese nombre del paquete syscall (syscall.Kill, o
	// Kill a secas con un import con punto)
	deSyscall := func(e ast.Expr, nombre string) bool {
		switch e := e.(type) {
		case *ast.SelectorExpr:
			x, ok := e.X.(*ast.Ident)
			return ok && e.Sel.Name == nombre && slices.Contains(syscalls, x.Name)
		case *ast.Ident:
			return e.Name == nombre && slices.Contains(syscalls, ".")
		}
		return false
	}
	var usos []string
	uso := func(n ast.Node, que string) {
		usos = append(usos, archivo+":"+strconv.Itoa(fset.Position(n.Pos()).Line)+": "+que)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if deSyscall(n.Fun, "Kill") {
				uso(n, "llama a syscall.Kill")
			}
		case *ast.CompositeLit:
			for _, e := range n.Elts {
				if kv, ok := e.(*ast.KeyValueExpr); ok && deSyscall(n.Type, "SysProcAttr") {
					if k, ok := kv.Key.(*ast.Ident); ok && k.Name == "Setpgid" {
						uso(kv, "pone Setpgid en un syscall.SysProcAttr")
					}
				}
			}
		case *ast.AssignStmt:
			for _, e := range n.Lhs {
				if sel, ok := e.(*ast.SelectorExpr); ok && sel.Sel.Name == "Setpgid" {
					uso(e, "asigna el campo Setpgid")
				}
			}
		}
		return true
	})
	return usos, nil
}

// grupoOtrosUsosAMano son los usos a mano (grupoUsosAMano) de esos fuentes
// (nombre -> texto) que no son de este, el unico que puede tenerlos. Si este
// no esta entre ellos, o no tiene ninguno, la guarda dejo de mirar: es un
// error, no un "no hay".
func grupoOtrosUsosAMano(este string, fuentes map[string][]byte) ([]string, error) {
	if _, esta := fuentes[este]; !esta {
		return nil, fmt.Errorf("%s, el que arma los grupos, no esta entre los fuentes", este)
	}
	var otros []string
	for _, nombre := range slices.Sorted(maps.Keys(fuentes)) {
		usos, err := grupoUsosAMano(nombre, fuentes[nombre])
		switch {
		case err != nil:
			return nil, fmt.Errorf("%s no parsea: %v", nombre, err)
		case nombre != este:
			otros = append(otros, usos...)
		case len(usos) == 0:
			return nil, fmt.Errorf("la guarda no ve ni las senales de %s, que las manda: dejo de mirar", este)
		}
	}
	return otros, nil
}

// Hallazgo 954d37: la guarda de abajo ve los usos de verdad y nada mas. Sobre
// fuentes chicas, al lado de un helper que se llama de otra manera que el de
// verdad (la guarda no depende de su nombre): la llamada a syscall.Kill, se
// importe syscall como se importe, y el campo Setpgid en un literal o
// asignado quedan anotados con su linea; lo que solo los nombra (un
// comentario, una cadena, el script de un fixture) y un Kill o un Setpgid de
// otro, no. Y una guarda que no encuentra a su helper, o que no le ve las
// senales, no dice "no hay": falla.
func TestHallazgo_954d37_LaGuardaVeLosUsosDeVerdadYNadaMas(t *testing.T) {
	const (
		helper = "el_grupo_con_otro_nombre_test.go"
		cabeza = "package reviewcmd\n\nimport (\n\t\"os/exec\"\n\t\"syscall\"\n)\n\n" // 7 lineas
		// lo que hace el helper de verdad
		delHelper = cabeza + "func f(cmd *exec.Cmd) {\n\tcmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}\n\t_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)\n}\n"
	)
	casos := []struct {
		nombre string
		src    string
		usos   []string // lo que anota, sin el nombre del fuente
	}{
		{
			"la llamada directa",
			cabeza + "func f(cmd *exec.Cmd) {\n\t_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)\n}\n",
			[]string{"9: llama a syscall.Kill"},
		},
		{
			"el import con alias",
			"package reviewcmd\n\nimport sc \"syscall\"\n\nfunc f(pid int) {\n\t_ = sc.Kill(-pid, sc.SIGKILL)\n}\n",
			[]string{"6: llama a syscall.Kill"},
		},
		{
			"el import con punto",
			"package reviewcmd\n\nimport . \"syscall\"\n\nfunc f(pid int) *SysProcAttr {\n\t_ = Kill(-pid, SIGKILL)\n\treturn &SysProcAttr{Setpgid: true}\n}\n",
			[]string{"6: llama a syscall.Kill", "7: pone Setpgid en un syscall.SysProcAttr"},
		},
		{
			"el campo Setpgid en un literal",
			cabeza + "func f(cmd *exec.Cmd) {\n\tcmd.SysProcAttr = &syscall.SysProcAttr{\n\t\tSetpgid: true,\n\t}\n}\n",
			[]string{"10: pone Setpgid en un syscall.SysProcAttr"},
		},
		{
			"el campo Setpgid asignado",
			cabeza + "func f(cmd *exec.Cmd) {\n\tcmd.SysProcAttr = &syscall.SysProcAttr{}\n\tcmd.SysProcAttr.Setpgid = true\n}\n",
			[]string{"10: asigna el campo Setpgid"},
		},
		{
			"solo en un comentario",
			cabeza + "// f no hace syscall.Kill(-pid, syscall.SIGKILL) ni pone Setpgid: true\nfunc f(cmd *exec.Cmd) syscall.Signal {\n\treturn syscall.SIGKILL // syscall.Kill\n}\n",
			nil,
		},
		{
			"solo en una cadena",
			cabeza + "const cuerpo = \"kill -9 $$ # syscall.Kill(-pid, 9), Setpgid: true\"\n\nvar crudo = `syscall.Kill(-pid, syscall.SIGKILL)`\n\nvar _ = exec.Command(\"/bin/sh\", \"-c\", cuerpo+crudo, syscall.SIGKILL.String())\n",
			nil,
		},
		{
			"un Kill y un Setpgid de otro",
			cabeza + "type otro struct{ Setpgid bool }\n\nfunc (otro) Kill(int) {}\n\nfunc f(cmd *exec.Cmd, Kill func(int)) {\n\tp := cmd.Process\n\t_ = p.Kill()\n\totro{Setpgid: true}.Kill(-1)\n\tKill(-1)\n\t_ = syscall.Getpgrp()\n}\n",
			nil,
		},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			got, err := grupoOtrosUsosAMano(helper, map[string][]byte{helper: []byte(delHelper), "x.go": []byte(c.src)})
			if err != nil {
				t.Fatalf("fixture: %v\n%s", err, c.src)
			}
			for i := range got {
				got[i] = strings.TrimPrefix(got[i], "x.go:")
			}
			if !slices.Equal(got, c.usos) {
				t.Fatalf("hallazgo 954d37: la guarda anota %q, y tiene que anotar %q, en:\n%s", got, c.usos, c.src)
			}
		})
	}

	for nombre, fuentes := range map[string]map[string][]byte{
		"no encuentra a su helper":           {"x.go": []byte(delHelper)},
		"no le ve las senales a su helper":   {helper: []byte(cabeza + "var _ = exec.Command\n\nvar _ syscall.Signal\n"), "x.go": []byte(delHelper)},
		"un fuente no parsea":                {helper: []byte(delHelper), "x.go": []byte("package reviewcmd\n\nfunc Kill( {\n")},
		"el helper mismo no parsea (y Kill)": {helper: []byte("package reviewcmd\n\nfunc Kill( {\n")},
	} {
		t.Run("la guarda falla si "+nombre, func(t *testing.T) {
			if usos, err := grupoOtrosUsosAMano(helper, fuentes); err == nil {
				t.Fatalf("hallazgo 954d37: una guarda que %s no puede decir que no hay usos a mano: dijo %q", nombre, usos)
			}
		})
	}
}

// Lo que queda del hallazgo 786f65: ningun otro _test.go del paquete le manda
// una senal a un proceso con syscall.Kill ni arma un grupo de procesos
// (Setpgid). El que tiene que cortar a un comando y a lo que lanzo lo arranca
// con grupoArrancar o lo corre con grupoCorrer: ahi los cortes son los de la
// cabecera de este archivo, y despues no sale ninguno.
//
// Hallazgo 954d37: mira los usos de verdad (grupoUsosAMano), no el texto: un
// comentario o una cadena que los nombre no cuentan. Y "este archivo" es el
// que tiene este test, se llame como se llame.
func TestHallazgo_786f65_NingunTestMandaSenalesAUnGrupoAMano(t *testing.T) {
	_, este, _, _ := runtime.Caller(0)
	este = filepath.Base(este)
	archivos, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(archivos, este) {
		t.Skipf("los fuentes de los tests no estan en el directorio de trabajo: no se puede mirar quien manda senales (falta %s)", este)
	}
	fuentes := map[string][]byte{}
	for _, a := range archivos {
		if fuentes[a], err = os.ReadFile(a); err != nil {
			t.Fatal(err)
		}
	}
	usos, err := grupoOtrosUsosAMano(este, fuentes)
	if err != nil {
		t.Fatalf("fixture: %v", err)
	}
	for _, uso := range usos {
		t.Errorf("%s: a un comando y a lo que lanzo se los corta por su grupo de procesos, y eso lo hace solo %s "+
			"(grupoArrancar o grupoCorrer, y despues cortar o recolectar)", uso, este)
	}
}
