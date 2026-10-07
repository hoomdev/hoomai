// El grupo de procesos de un comando que lanza un test, y sus pruebas (tercera
// review de estabilizar-ci: hallazgos 20261007T142544_ef9963,
// 20261007T141141_5b4172 / 20261007T142507_4c2a57 y 20261007T142727_b942f0;
// cuarta: 20261007T171925_139218 / 20261007T172254_c89063; quinta:
// 20261007T184715_8710cd, 20261007T185147_579ef7 / 20261007T185512_d45edb).
//
// Un comando que lanza a otros (un CLI falso, cuyo cuerpo corre en un
// subshell; el medidor, que lanza a hoom; este binario vuelto a ejecutar) no
// se corta matando al proceso que se lanzo: los demas siguen, con sus pipes
// abiertos. Se corta su GRUPO de procesos. Por convencion, quien tenga que
// cortar a un comando y a lo que lanzo lo hace por aca; la excepcion, a
// proposito, es el caso "lo matan desde afuera" de cli_falso_test.go, que mata
// SOLO al shell del script (cmd.Process.Kill) para mostrar que no alcanza.
// (Hubo una guarda que lo hacia cumplir. Nacio del hallazgo
// 20261007T141236_786f65, que quedo refutado, y mirando el fuente no se puede
// ser completo y preciso a la vez: cada arreglo le abria otro hueco.)
//
// El grupo se nombra con el pid de su lider, que deja de ser suyo cuando al
// lider lo recolectan y no queda nadie mas en el grupo. Lo que se cumple:
// NUNCA sale una senal al grupo con el lider ya recolectado, y cuando esperar
// o recolectar vuelven no queda nadie del grupo. Como: al lider se lo espera
// SIN recolectarlo (grupoEsperarSinRecolectar, por sistema operativo); con el
// lider terminado y todavia sin recolectar se corta al grupo y se anota, bajo
// un candado, que ya no sale ninguna senal mas; y recien entonces se lo
// recolecta (cmd.Wait).
//
// Lo que no alcanza ningun corte: al que se fue del grupo. Y un corte puede no
// alcanzar al proceso que el grupo esta lanzando en ese instante: medido aca
// (macOS, dash), cortando apenas el shell avisaba quedo vivo en 60 de 3000 con
// un solo corte, y en 0 de 3000 con los dos de recolectar (con /bin/sh, 0 de
// 3000 de las dos maneras). Es una medicion, no una garantia.
package reviewcmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// grupoPlazoDeLosPipes es el WaitDelay de un comando que no trae el suyo:
// cuanto se espera, con el lider ya terminado, a que llegue el fin de archivo
// de su stdout y de su stderr. Que no llegue es un error (esperar), no una
// salida mas.
const grupoPlazoDeLosPipes = 2 * time.Second

// grupoDeProcesos es el grupo de procesos de un comando que lanzo un test: el
// comando, que es su lider, y lo que el comando haya lanzado.
type grupoDeProcesos struct {
	t   testing.TB
	cmd *exec.Cmd
	// senal es syscall.Kill y esperaSinRecolectar, grupoEsperarSinRecolectar:
	// campos, para que un test vea que senales salen y que pasa si la espera
	// falla
	senal               func(pid int, sig syscall.Signal) error
	esperaSinRecolectar func(pid int) error

	pipes        []*grupoPipe // el stdout y el stderr del comando
	una          sync.Once    // la espera del lider, y err lo que dio
	err          error
	mu           sync.Mutex // cuida recolectando
	recolectando bool       // al lider ya se lo va a recolectar: no sale ninguna senal mas
}

// grupoPipe es el stdout o el stderr de un comando, mirado por su grupo.
// exec.Cmd copia cada pipe con io.Copy, que lee por aca (ReadFrom): cuando esa
// copia termina, como sea que termine, fin se cierra.
type grupoPipe struct {
	io.Writer
	fin chan struct{}
}

func (p *grupoPipe) ReadFrom(r io.Reader) (int64, error) {
	defer close(p.fin)
	return io.Copy(p.Writer, r)
}

// mirar devuelve w mirado por el grupo, si exec.Cmd le va a armar un pipe: ni
// nil ni un archivo.
func (g *grupoDeProcesos) mirar(w io.Writer) io.Writer {
	if _, archivo := w.(*os.File); w == nil || archivo {
		return w
	}
	p := &grupoPipe{Writer: w, fin: make(chan struct{})}
	g.pipes = append(g.pipes, p)
	return p
}

// grupoMismoWriter dice si a y b son el mismo Writer, como lo mira exec.Cmd
// para darles un solo pipe. Ante un tipo que no se puede comparar (un struct
// por valor con un slice adentro) el == de dos interfaces entra en panico:
// aca, como en exec.Cmd, eso es "distintos".
func grupoMismoWriter(a, b io.Writer) (mismo bool) {
	defer func() { _ = recover() }()
	return a == b
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
	g := &grupoDeProcesos{t: t, cmd: cmd, senal: syscall.Kill, esperaSinRecolectar: grupoEsperarSinRecolectar}
	junta := cmd.Stdout != nil && grupoMismoWriter(cmd.Stdout, cmd.Stderr) // un solo pipe para los dos
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
// su grupo. Si al lider ya se lo va a recolectar, o ya se lo recolecto, no
// manda nada.
func (g *grupoDeProcesos) cortar() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.recolectando {
		_ = g.senal(-g.cmd.Process.Pid, syscall.SIGKILL)
	}
}

// salidaTomada dice, con el lider ya terminado, si alguien mas conserva el
// stdout o el stderr del comando: lo que el lider dejo escrito llega enseguida,
// y con eso el fin de archivo; si pasado el WaitDelay del comando alguna de
// las copias de exec.Cmd no termino, es que del otro lado queda otro.
func (g *grupoDeProcesos) salidaTomada() bool {
	plazo := time.NewTimer(g.cmd.WaitDelay)
	defer plazo.Stop()
	for _, p := range g.pipes {
		select {
		case <-p.fin:
		case <-plazo.C:
			return true
		}
	}
	return false
}

// esperar espera a que el lider termine SOLO, corta lo que haya dejado en su
// grupo y recien entonces lo recolecta: cuando vuelve no queda nadie del
// grupo. Devuelve lo que devuelve cmd.Wait, se lo llame las veces que sea y
// desde donde sea; salvo que alguien que el lider dejo detras conservara el
// stdout o el stderr del comando (salidaTomada, o el exec.ErrWaitDelay de
// cmd.Wait): entonces es un exec.ErrWaitDelay, haya salido el lider con lo que
// haya salido, que trae ademas el error del lider (su *exec.ExitError). Esa
// salida no esta entera.
//
// Si no puede esperar sin recolectar es un error, del que la llama y de t: no
// sabe si el lider termino, lo corta igual y lo recolecta.
func (g *grupoDeProcesos) esperar() error {
	g.una.Do(func() {
		pid := g.cmd.Process.Pid
		errEspera := g.esperaSinRecolectar(pid)
		if errEspera != nil {
			errEspera = fmt.Errorf("fixture: no pude esperar sin recolectar al lider del grupo %d (%s): %w", pid, g.cmd.Path, errEspera)
			g.t.Error(errEspera)
		}
		tomada := errEspera == nil && g.salidaTomada()
		g.mu.Lock()
		_ = g.senal(-pid, syscall.SIGKILL)
		g.recolectando = true
		g.mu.Unlock()
		switch g.err = g.cmd.Wait(); {
		case errEspera != nil:
			g.err = errEspera
		case g.err == nil && tomada:
			g.err = exec.ErrWaitDelay
		case tomada && !errors.Is(g.err, exec.ErrWaitDelay):
			g.err = fmt.Errorf("%w (el lider: %w)", exec.ErrWaitDelay, g.err)
		}
	})
	return g.err
}

// recolectar corta al grupo sin esperar a que su lider termine solo, y lo
// espera: son dos cortes, los dos antes de recolectarlo. Devuelve lo mismo
// que esperar.
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
// es el de esperar (el de cmd.Wait, o el exec.ErrWaitDelay que arma el grupo
// cuando alguien conservo la salida del comando), o el de no haber podido
// lanzarlo (y entonces cmd.ProcessState es nil).
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
// si en ese momento su lider seguia SIN recolectar: todavia era un proceso,
// vivo o terminado (kill con la senal 0, que no manda nada, lo encuentra).
type grupoSenalVista struct {
	pid           int
	sinRecolectar bool
}

// grupoEspiar anota las senales que salen de g, que salen igual. Devuelve
// donde quedan anotadas. Es para el test que usa a g desde una sola goroutine.
func grupoEspiar(g *grupoDeProcesos) *[]grupoSenalVista {
	vistas, lider := new([]grupoSenalVista), g.cmd.Process.Pid
	g.senal = func(pid int, sig syscall.Signal) error {
		*vistas = append(*vistas, grupoSenalVista{pid, syscall.Kill(lider, 0) == nil})
		return syscall.Kill(pid, sig)
	}
	return vistas
}

// Hallazgo 8710cd (y lo que quedo del 786f65, que pedia lo mismo y se habia
// dado por refutado con mediciones): que senales salen de un grupo, y cuando.
// Todas son al grupo (-pid), y TODAS salen con el lider todavia sin
// recolectar: el espia lo mira en el momento de cada una.
//
//   - por el camino natural (esperar a un lider que termina solo), una: con
//     el lider ya terminado, que no le cambia con que salio;
//   - por recolectar, dos: la que mata al lider, y la de esperar. El comando
//     no termina solo: si recolectar lo esperara antes de cortarlo, saldria
//     con 0 medio minuto despues;
//   - una vez que esperar volvio, con el lider recolectado, ninguna mas, se
//     llame a lo que se llame y las veces que sea.
func TestHallazgo_786f65_DespuesDeRecolectarCortarNoMandaNada(t *testing.T) {
	despues := func(t *testing.T, g *grupoDeProcesos, vistas *[]grupoSenalVista) {
		t.Helper()
		if lider := g.cmd.Process.Pid; syscall.Kill(lider, 0) == nil {
			t.Fatalf("fixture: cuando esperar vuelve el lider esta recolectado: su pid %d sigue nombrando a un proceso", lider)
		}
		salieron := len(*vistas)
		for range 3 {
			g.cortar()
			_ = g.esperar()
			_ = g.recolectar()
		}
		if len(*vistas) != salieron {
			t.Fatalf("hallazgo 8710cd: con el lider recolectado no sale ninguna senal mas (el numero del grupo ya puede no ser suyo): salieron %+v", (*vistas)[salieron:])
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
		if !slices.Equal(*vistas, []grupoSenalVista{{grupo, true}, {grupo, true}}) || cmd.ProcessState.ExitCode() != -1 {
			t.Fatalf("hallazgo 8710cd: recolectar corta al grupo (%d) dos veces, las dos con su lider sin recolectar, y la primera lo mata (exit -1): salieron %+v, %v",
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
			t.Fatalf("esperar devuelve lo que devuelve el Wait de un comando que salio con 3, y el corte no se lo cambia: %v, %v", err, cmd.ProcessState)
		}
		if !slices.Equal(*vistas, []grupoSenalVista{{grupo, true}}) {
			t.Fatalf("hallazgo 8710cd: por el camino natural sale UN corte al grupo (%d), con su lider terminado y todavia sin recolectar: salieron %+v", grupo, *vistas)
		}
		despues(t, g, vistas)
	})
}

// grupoConReloj corre f, que espera a un proceso, y dice si volvio en d y con
// que: una espera rota no cuelga al test.
func grupoConReloj(d time.Duration, f func() error) (volvio bool, err error) {
	fin := make(chan error, 1)
	go func() { fin <- f() }()
	select {
	case err := <-fin:
		return true, err
	case <-time.After(d):
		return false, nil
	}
}

// Hallazgo 8710cd: la espera sin recolectar, que es distinta en cada sistema
// operativo. No vuelve mientras el proceso vive; vuelve cuando termina; para
// uno que ya termino vuelve enseguida, las veces que sea; y despues de todo
// eso el proceso sigue ahi para quien lo recolecte, con su codigo de salida.
// Y de uno que ya no es un hijo sin recolectar no dice "ya termino": es un
// error. El proceso vive hasta que le cierran el stdin, y sale con 5.
func TestHallazgo_8710cd_LaEsperaSinRecolectarNoRecolecta(t *testing.T) {
	cmd := exec.Command("/bin/sh", "-c", "read nada\nexit 5\n")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("fixture: no pude lanzar /bin/sh: %v", err)
	}
	pid, recolectado := cmd.Process.Pid, false
	defer func() {
		if !recolectado {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	esperar := func() error { return grupoEsperarSinRecolectar(pid) }

	primera := make(chan error, 1)
	go func() { primera <- esperar() }()
	select {
	case err := <-primera:
		t.Fatalf("hallazgo 8710cd: la espera sin recolectar no vuelve mientras el proceso vive: volvio con %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	_ = stdin.Close()
	select {
	case err := <-primera:
		if err != nil {
			t.Fatalf("hallazgo 8710cd: la espera sin recolectar vuelve sin error cuando el proceso termina: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatalf("hallazgo 8710cd: la espera sin recolectar vuelve cuando el proceso termina: 30 s despues sigue esperando")
	}
	for range 2 {
		if volvio, err := grupoConReloj(30*time.Second, esperar); !volvio || err != nil {
			t.Fatalf("hallazgo 8710cd: para un proceso que ya termino y sigue sin recolectar, la espera vuelve enseguida y sin error: volvio %v, %v", volvio, err)
		}
	}

	err = cmd.Wait()
	recolectado = true
	var salida *exec.ExitError
	if !errors.As(err, &salida) || salida.ExitCode() != 5 {
		t.Fatalf("hallazgo 8710cd: despues de esperarlo sin recolectar el proceso sigue ahi para su Wait, con su codigo de salida (5): %v", err)
	}
	if volvio, err := grupoConReloj(30*time.Second, esperar); !volvio || err == nil {
		t.Fatalf("hallazgo 8710cd: de un proceso ya recolectado la espera no dice \"ya termino\": es un error; volvio %v, %v", volvio, err)
	}
}

// grupoTAnotada es el t de un grupo que anota los errores que el grupo le
// marca, en vez de fallar: para el test que mira justamente eso.
type grupoTAnotada struct {
	testing.TB
	errores []string
}

func (a *grupoTAnotada) Error(args ...any) {
	a.errores = append(a.errores, fmt.Sprint(args...))
}

// Hallazgo 8710cd: si la espera sin recolectar falla (el sistema dice que no,
// o es uno donde no la hay), eso no es "el lider ya termino": es un error, que
// esperar devuelve y que ademas le marca a su test, para el que no mira lo que
// esperar devuelve. Y el grupo queda igual cortado, con su lider recolectado
// y sin nadie.
func TestHallazgo_8710cd_SiLaEsperaSinRecolectarFallaEsUnError(t *testing.T) {
	testigo := nuevoTestigoDeVida(t)
	anotada := &grupoTAnotada{TB: t}
	cmd := exec.Command("/bin/sh", "-c", testigo.shConUnoDetras("")+"wait\n")
	g, err := grupoArrancar(anotada, cmd)
	if err != nil {
		t.Fatalf("fixture: no pude lanzar /bin/sh: %v", err)
	}
	falla := errors.New("aca no se puede esperar sin recolectar")
	g.esperaSinRecolectar = func(int) error { return falla }
	<-testigo.alTomarlo(t)

	if err := g.esperar(); !errors.Is(err, falla) || len(anotada.errores) != 1 || !strings.Contains(anotada.errores[0], falla.Error()) {
		t.Fatalf("hallazgo 8710cd: una espera sin recolectar que falla es un error de esperar y de su test, no un lider que ya termino: esperar dio %v, y al test le marco %q", err, anotada.errores)
	}
	if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != -1 {
		t.Fatalf("hallazgo 8710cd: sin saber si el lider termino, esperar lo corta y lo recolecta igual (exit -1): %v", cmd.ProcessState)
	}
	if arranco, nadie := testigo.espera(10 * time.Second); !arranco || !nadie {
		t.Fatalf("hallazgo 8710cd: y no queda nadie del grupo: el comando tomo el testigo %v, 10 s despues no queda nadie %v", arranco, nadie)
	}
}

// grupoWriterRoto es un Writer que falla siempre.
type grupoWriterRoto struct{ err error }

func (w grupoWriterRoto) Write([]byte) (int, error) { return 0, w.err }

// Hallazgos 579ef7 y d45edb: un Writer que falla al escribir no es una salida
// tomada. La copia de exec.Cmd termina con ese error y nadie conserva el
// pipe: esperar devuelve lo que da cmd.Wait (el error del Writer si el lider
// salio con 0; el del lider si no), nunca un exec.ErrWaitDelay. Antes el
// grupo tomaba cualquier error de la copia por un pipe que cerro el WaitDelay.
func TestHallazgo_579ef7_d45edb_UnWriterQueFallaNoEsUnaSalidaTomada(t *testing.T) {
	roto := errors.New("este writer no escribe")
	for _, exit := range []int{0, 3} {
		t.Run("exit-"+strconv.Itoa(exit), func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command("/bin/sh", "-c", "echo dicho\nexit "+strconv.Itoa(exit)+"\n")
			cmd.Stdout = grupoWriterRoto{roto}
			vencio, err := grupoCorrer(t, cmd, time.After(30*time.Second))
			if vencio || errors.Is(err, exec.ErrWaitDelay) {
				t.Fatalf("hallazgos 579ef7 y d45edb: un Writer que falla no es una salida que alguien conservo: vencio %v, %v", vencio, err)
			}
			var delLider *exec.ExitError
			switch {
			case exit == 0 && !errors.Is(err, roto):
				t.Fatalf("con el lider salido con 0, el error es el del Writer, como en cmd.Wait: %v", err)
			case exit != 0 && (!errors.As(err, &delLider) || delLider.ExitCode() != exit):
				t.Fatalf("con el lider salido con %d, el error es el del lider, como en cmd.Wait: %v", exit, err)
			}
		})
	}
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
// con un exec.ErrWaitDelay, salga el lider con 0 o con otra cosa, con lo que
// llego a imprimir y sin dejar vivo al que conservaba el pipe.
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
				var delLider *exec.ExitError
				if exit != 0 && (!errors.As(err, &delLider) || delLider.ExitCode() != exit) {
					t.Fatalf("ese exec.ErrWaitDelay sigue trayendo el error del lider, un *exec.ExitError con su %d: %v", exit, err)
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

// grupoSalidaPorValor es un Writer por valor de un tipo que no se puede
// comparar (lleva un slice): el == de dos interfaces que lo traen entra en
// panico. Escribe en su unico buffer, con candado: exec.Cmd, que tampoco los
// puede comparar, le arma un pipe a cada uno y escribe de los dos a la vez.
type grupoSalidaPorValor struct {
	mu  *sync.Mutex
	buf []*bytes.Buffer
}

func (s grupoSalidaPorValor) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf[0].Write(p)
}

// El stdout y el stderr de un comando pueden ser un Writer por valor de un
// tipo que no se puede comparar. grupoArrancar, que mira si son el mismo para
// darles un solo pipe, no se rompe: los toma por distintos, como exec.Cmd; el
// comando corre, sale con lo suyo y lo que imprimio por los dos llega.
func TestHallazgo_ef9963_UnWriterQueNoSePuedeCompararNoRompeAlGrupo(t *testing.T) {
	var buf bytes.Buffer
	salida := grupoSalidaPorValor{mu: new(sync.Mutex), buf: []*bytes.Buffer{&buf}}
	cmd := exec.Command("/bin/sh", "-c", "echo a stdout\necho a stderr >&2\nexit 7\n")
	cmd.Stdout, cmd.Stderr = salida, salida
	vencio, err := grupoCorrer(t, cmd, time.After(30*time.Second))
	if vencio || cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 7 {
		t.Fatalf("un comando con un Writer que no se puede comparar en su stdout y en su stderr corre y sale con lo suyo (7): vencio %v, %v, %v", vencio, cmd.ProcessState, err)
	}
	// van por dos pipes: llegan las dos lineas, en el orden que sea
	if got := buf.String(); got != "a stdout\na stderr\n" && got != "a stderr\na stdout\n" {
		t.Fatalf("lo que el comando imprimio por su stdout y por su stderr llega entero: %q", got)
	}
}
