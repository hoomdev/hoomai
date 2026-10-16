// Tests de los hallazgos b59c30 y e0001e sobre cockpitcmd.Open (CA-347,
// CA-348), con el contrato nuevo de los hallazgos 9c1816, 57bd29 y 03481c
// (decision de Henry), escritos desde .hoom/specs/acciones-desde-la-tarjeta.md:
//
// Open crea la sesion, algo falla despues (el split-window, o registrar la
// sesion en el item) y el cierre (`tmux kill-session -t =<sesion>`) TAMBIEN
// falla. tmux sale con codigo 1 tanto si la sesion no esta como si falla su
// propio cliente (9c1816), asi que ni el codigo de salida del kill-session ni
// un sondeo `tmux has-session` despues prueban si la sesion sigue abierta. El
// error es UNO SOLO para todos los casos, este la sesion viva, muerta o sin
// saberse (ciNoSabeSiSigueAbierta, en tmuxdoble_test.go):
//
//   - conserva el error original del paso que fallo;
//   - no dice que la cerro, ni afirma como hecho que la sesion ya no existe o
//     que quedo abierta;
//   - dice que no se pudo saber si sigue abierta;
//   - muestra `tmux kill-session -t '=<sesion>'` con comillas, que corre tal
//     cual en sh y en zsh (cierreEjecutable);
//   - el item no cambia (CA-348).
//
// Y no depende de ningun sondeo: con el mismo paso fallido y el mismo error
// del kill-session, el mensaje es EL MISMO responda lo que responda tmux a
// has-session (o aunque no se le pregunte), y este la sesion viva o muerta.
// Y no se le pregunta (hallazgo e5f706): despues del kill-session fallido Open
// no llama a has-session (cero sondeos) ni a ningun otro verbo de tmux, porque
// un tmux que no pudo cerrar la sesion puede estar degradado y una segunda
// llamada puede no volver nunca (la CLI no le pone plazo).
//
// Reemplaza al contrato de tres estados (A: has-session con codigo 1 => "ya
// no existe"; B: has-session sale bien => "quedo abierta"; C: el sondeo no
// confirma nada => "no se sabe"): con el cliente de tmux roto (codigo 1 y la
// sesion VIVA, 9c1816) el estado A decia "ya no existe" de una sesion viva y
// escondia el comando para cerrarla. No es debilitar: el mensaje nuevo es
// verdadero en todos los casos, cosa que el de tres estados no lograba.
//
// Los errores de tmux son los que da os/exec con procesos de verdad (/bin/sh,
// /bin/sleep), no errores de Go armados a mano, salvo los casos que son
// justamente eso (el kill-session que falla sin codigo de salida, el error del
// contexto y el sondeo sin codigo de salida). Fixtures de open_capture_test.go
// (cbProyecto, cbLeerItem), hallazgos_c1_test.go (h3Tmux, h3Sesion,
// h3ErrPaso, h3SinHuerfana), hallazgos_c1c_test.go (hc3Tmux, hc3Llamada,
// hc3ItemSoloLectura), hallazgos_db8541_f7efdc_test.go (cierreEjecutable) y
// tmuxdoble_test.go (tmuxSinSesion, tmuxClienteRoto, ciNoSabeSiSigueAbierta),
// y el verificador compartido internal/cierretmuxtest (DiceQueLaCerro,
// DiceQueNoSabe; hallazgo 9accd8). Ningun tmux real.
package cockpitcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cierretmuxtest"
)

var hb5ErrKill = errors.New("tmux falso: kill-session fallo a proposito (hb5)")

// hb5Tmux es el tmux falso con estado h3Tmux con el cierre roto:
//
//   - has-session ANTES de cualquier kill-session responde como tmux: nil si
//     la sesion vive, tmuxSinSesion (codigo 1) si no.
//   - kill-session se registra y falla con kill(); sin sigueViva, la sesion deja de
//     vivir (murio cuando Open intentaba cerrarla: la CLI de la IA salio, o
//     el servidor de tmux se cayo); con sigueViva, sigue viva.
//   - has-session DESPUES del kill-session (un sondeo, si Open lo hace) se
//     registra, se cuenta en sondeos y responde sondeo, sin mirar el estado.
//     Open no debe hacerlo (e5f706): sondeo es la trampa, y sondeos y
//     trasElCierre lo que el test mira.
//
// El verbo en h3Tmux.fallar falla como siempre. Atiende igual por RunCmd,
// QuietCmd y Output, y todas las llamadas quedan en h3Tmux.llamadas.
type hb5Tmux struct {
	*h3Tmux
	sigueViva bool
	kill      func() error
	sondeo    func() error
	cerrar    int // kill-session que se intentaron
	sondeos   int // has-session despues del primer kill-session
}

func (f *hb5Tmux) atender(name string, args []string) ([]byte, error) {
	if name == "tmux" && len(args) > 0 {
		switch args[0] {
		case "kill-session":
			f.mu.Lock()
			f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
			f.cerrar++
			if !f.sigueViva {
				delete(f.vivas, h3Destino(args, "-t"))
			}
			f.mu.Unlock()
			return nil, f.kill()
		case "has-session":
			f.mu.Lock()
			despues := f.cerrar > 0
			if despues {
				f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
				f.sondeos++
			}
			f.mu.Unlock()
			if despues {
				return nil, f.sondeo()
			}
			if d := h3Destino(args, "-t"); !f.viva(d) {
				f.mu.Lock()
				f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
				f.mu.Unlock()
				return nil, tmuxSinSesion(d)
			}
		}
	}
	return f.h3Tmux.atender(name, args)
}

// trasElCierre son las llamadas a tmux despues del primer kill-session, en
// orden (nil si no hubo kill-session).
func (f *hb5Tmux) trasElCierre() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, l := range f.llamadas {
		if strings.HasPrefix(l, "tmux kill-session") {
			var out []string
			for _, d := range f.llamadas[i+1:] {
				if strings.HasPrefix(d, "tmux ") {
					out = append(out, d)
				}
			}
			return out
		}
	}
	return nil
}

func (f *hb5Tmux) deps() Deps {
	d := f.h3Tmux.deps()
	run := func(dir, name string, args ...string) error {
		_, err := f.atender(name, args)
		return err
	}
	d.RunCmd, d.QuietCmd = run, run
	d.Output = func(dir, name string, args ...string) ([]byte, error) {
		return f.atender(name, args)
	}
	return d
}

// hb5Cierre es como falla el kill-session: un nombre (la clave con la que se
// comparan los mensajes) y el error, nuevo en cada llamada.
type hb5Cierre struct {
	nombre string
	err    func() error
}

var (
	// sale con codigo 1 y "can't find session" (la sesion no esta... o eso dice)
	hb5KillSinSesion = hb5Cierre{"kill-session sale con codigo 1 (can't find session)", func() error { return tmuxSinSesion(h3Sesion) }}
	// sale con codigo 1 porque falla el propio cliente (9c1816)
	hb5KillCliente = hb5Cierre{"kill-session sale con codigo 1 (" + tmuxClienteRotoMsg + ")", tmuxClienteRoto}
	// falla sin codigo de salida
	hb5KillGo = hb5Cierre{"kill-session falla sin codigo de salida", func() error { return hb5ErrKill }}
)

// hb5Caso es un caso de la matriz: como falla el kill-session, si la sesion
// sigue viva despues, y que responde has-session despues (nil = sale bien).
// codigo1 dice si el sondeo es un *exec.ExitError con codigo 1 (fixture).
type hb5Caso struct {
	caso    string
	kill    hb5Cierre
	viva    bool
	sondeo  error
	codigo1 bool
}

// hb5Casos arma la matriz con os/exec de verdad, una vez por test, y comprueba
// como fixture que cada sondeo sea lo que dice ser.
func hb5Casos(t *testing.T) []hb5Caso {
	t.Helper()
	// no arranca: tmux ya no esta en el PATH (exec.Command no lo encuentra)
	_, noEsta := exec.Command("hoom-tmux-que-no-esta-b59c30-e0001e").Output()
	// no arranca: el binario de tmux que se resolvio desaparecio (fork/exec falla)
	_, sinBinario := exec.Command(filepath.Join(t.TempDir(), "tmux")).Output()
	// sale por una senal
	_, senal := exec.Command("/bin/sh", "-c", "kill -KILL $$").Output()
	// vence su plazo: exec.CommandContext lo mata al vencer
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	_, plazo := exec.CommandContext(ctx, "/bin/sleep", "5").Output()
	cancel()

	casos := []hb5Caso{
		// la sesion murio, y tmux lo dice con codigo 1 (el viejo estado A)
		{"la sesion murio y has-session sale con codigo 1", hb5KillSinSesion, false, tmuxSinSesion(h3Sesion), true},
		// 9c1816: el cliente de tmux falla con codigo 1 y la sesion sigue VIVA
		{"9c1816: el cliente falla con codigo 1 y la sesion vive", hb5KillCliente, true, tmuxClienteRoto(), true},
		{"el cliente falla con codigo 1, la sesion vive y has-session sale bien", hb5KillCliente, true, nil, false},
		{"el servidor se cayo con la sesion y has-session sale con codigo 1", hb5KillCliente, false, tmuxSinSesion(h3Sesion), true},
		// kill-session falla sin codigo de salida
		{"la sesion vive y has-session sale bien", hb5KillGo, true, nil, false},
		{"la sesion vive y has-session sale con codigo 1 (9c1816)", hb5KillGo, true, tmuxClienteRoto(), true},
		{"la sesion murio y has-session sale con codigo 1", hb5KillGo, false, tmuxSinSesion(h3Sesion), true},
		{"la sesion vive y has-session no arranca: tmux ya no esta en el PATH", hb5KillGo, true, noEsta, false},
		{"la sesion vive y has-session no arranca: el binario de tmux desaparecio", hb5KillGo, true, sinBinario, false},
		{"la sesion vive y has-session sale por una senal", hb5KillGo, true, senal, false},
		{"la sesion vive y has-session vence su plazo", hb5KillGo, true, plazo, false},
		{"la sesion vive y has-session vence su plazo (error del contexto)", hb5KillGo, true, fmt.Errorf("tmux has-session -t =%s: %w", h3Sesion, context.DeadlineExceeded), false},
		{"la sesion vive y has-session falla con un error sin codigo de salida", hb5KillGo, true, errors.New("tmux: " + tmuxClienteRotoMsg), false},
	}
	for _, c := range casos {
		var ee *exec.ExitError
		uno := errors.As(c.sondeo, &ee) && ee.ExitCode() == 1
		if uno != c.codigo1 {
			t.Fatalf("fixture: %s: el sondeo es un *exec.ExitError con codigo 1 = %v (quiere %v): %#v", c.caso, uno, c.codigo1, c.sondeo)
		}
	}
	return casos
}

// hb5Tmp es el sufijo al azar del archivo temporal con el que se escribe el
// item, que trae el error de "registrar en el item falla".
var hb5Tmp = regexp.MustCompile(`\.tmp-[0-9]+`)

// hb5Comparable es msg sin lo que cambia de un subtest a otro aunque el
// mensaje sea el mismo: la carpeta temporal del proyecto (root, tambien con
// los enlaces resueltos: en macOS /var es /private/var) y el sufijo al azar
// del archivo temporal del item.
func hb5Comparable(msg, root string) string {
	if real, err := filepath.EvalSymlinks(root); err == nil {
		msg = strings.ReplaceAll(msg, real, "<root>")
	}
	msg = strings.ReplaceAll(msg, root, "<root>")
	return hb5Tmp.ReplaceAllString(msg, ".tmp-N")
}

// GUARDA del doble (b59c30, e0001e, 9c1816): tmuxSinSesion y tmuxClienteRoto
// responden como tmux de verdad, los dos con un *exec.ExitError con
// ExitCode() == 1 (por eso el codigo de salida no prueba nada), con "can't
// find session: <sesion>" y "lost server" en Stderr. Y hb5Tmux responde asi a
// has-session antes del kill-session, nil si la sesion vive.
func TestCA347_HallazgosB59c30E0001e_ElDobleRespondeComoTmux(t *testing.T) {
	for _, c := range []struct {
		caso, stderr string
		err          error
	}{
		{"la sesion no esta", "can't find session: " + h3Sesion, tmuxSinSesion(h3Sesion)},
		{"falla el cliente (9c1816)", tmuxClienteRotoMsg, tmuxClienteRoto()},
	} {
		var ee *exec.ExitError
		if !errors.As(c.err, &ee) || ee.ExitCode() != 1 {
			t.Fatalf("CA-347: b59c30/e0001e/9c1816: %s: tmux sale con codigo 1: %#v", c.caso, c.err)
		}
		if !strings.Contains(string(ee.Stderr), c.stderr) {
			t.Fatalf("CA-347: b59c30/e0001e/9c1816: %s: tmux dice %q por stderr: %q", c.caso, c.stderr, ee.Stderr)
		}
	}
	f := &hb5Tmux{h3Tmux: &h3Tmux{}, kill: hb5KillGo.err, sondeo: func() error { return nil }}
	d := f.deps()
	var ee *exec.ExitError
	if err := d.QuietCmd("", "tmux", "has-session", "-t", "="+h3Sesion); !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("CA-347: b59c30/e0001e: fixture: hb5Tmux responde has-session de una sesion que no vive con codigo 1: %#v", err)
	}
	if err := d.RunCmd("", "tmux", "new-session", "-d", "-s", h3Sesion); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Output("", "tmux", "has-session", "-t", "="+h3Sesion); err != nil {
		t.Fatalf("CA-347: b59c30/e0001e: fixture: hb5Tmux responde has-session de una sesion viva sin error: %v", err)
	}
}

// Hallazgos b59c30, e0001e y 9c1816 (CA-347, CA-348). Open crea la sesion de
// la tarea facturas, algo falla despues (el split-window, o registrar la
// sesion en el item, de solo lectura) y el cierre con `tmux kill-session -t
// =<sesion>` TAMBIEN falla. La matriz cambia como falla el kill-session
// (codigo 1 con "can't find session", codigo 1 del cliente roto, sin codigo
// de salida), si la sesion sigue viva, y que responde has-session despues:
// codigo 1 (con la sesion muerta, y con la sesion VIVA porque fallo el
// cliente: 9c1816), sale bien, no arranca, sale por una senal, vence su plazo
// o falla sin codigo de salida.
//
// En todos, el error cumple ciNoSabeSiSigueAbierta (conserva el del paso que
// fallo, no dice que la cerro, no afirma que la sesion ya no existe ni que
// quedo abierta, dice que no se pudo saber si sigue abierta y muestra el
// comando de cierre con comillas), el comando corre tal cual en sh y en zsh, y
// el item queda byte a byte igual (CA-348). Y con el mismo paso fallido y el
// mismo error del kill-session, el mensaje es identico en todos los casos:
// no depende de has-session ni de si la sesion vive.
//
// Hallazgo e5f706 (CA-347, CA-348): en todas las filas, despues del
// kill-session fallido no hay ningun has-session (sondeos == 0, y ninguno en
// las llamadas) ni ninguna otra llamada a tmux. Las respuestas de has-session
// de la matriz son trampas: sin esta guarda, una regresion que volviera a
// sondear y descartara el resultado dejaria el mismo mensaje y pasaria.
//
// Reemplaza a TestCA347_CA348_HallazgosB59c30E0001e_ElErrorDiceLaVerdadSobreLaSesion,
// que exigia tres estados segun el sondeo: el contrato de tres estados quedo
// reemplazado por decision de Henry (hallazgos 9c1816, 57bd29 y 03481c). No es
// debilitar: el mensaje nuevo es verdadero en todos los casos, cosa que el
// anterior no lograba.
func TestCA347_CA348_Hallazgo9c1816_SiElCierreFallaElErrorEsUnoSoloYNoSabeSiLaSesionSigueAbierta(t *testing.T) {
	casos := hb5Casos(t)
	type visto struct{ caso, msg string }
	mensajes := map[string][]visto{} // por paso y kill-session
	for _, paso := range []struct {
		caso, fallar, original string
		soloLectura            bool
	}{
		{"split-window roto", "split-window", h3ErrPaso.Error(), false},
		{"registrar en el item falla", "", "permission denied", true},
	} {
		for _, c := range casos {
			t.Run(paso.caso+"/"+c.kill.nombre+"/"+c.caso, func(t *testing.T) {
				ca := "CA-347, CA-348: 9c1816 (b59c30/e0001e): " + paso.caso + ": " + c.kill.nombre + ": " + c.caso
				if paso.soloLectura && os.Geteuid() == 0 {
					t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
				}
				root, _ := cbProyecto(t, true)
				crudo := cbLeerItem(t, root)
				if paso.soloLectura {
					hc3ItemSoloLectura(t, root)
				}
				sondeo := c.sondeo
				f := &hb5Tmux{h3Tmux: &h3Tmux{fallar: paso.fallar}, sigueViva: c.viva, kill: c.kill.err, sondeo: func() error { return sondeo }}

				_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
				nueva := f.indice("tmux new-session", 0)
				if nueva < 0 || f.indice(hc3Llamada, nueva) < 0 {
					t.Fatalf("%s: fixture: Open creo la sesion e intento cerrarla con `%s`:\n%s", ca, hc3Llamada, f.todas())
				}
				if viva := f.viva(h3Sesion); viva != c.viva {
					t.Fatalf("%s: fixture: la sesion %s viva=%v (quiere %v):\n%s", ca, h3Sesion, viva, c.viva, f.todas())
				}
				if err == nil {
					t.Fatalf("%s: Open devuelve el error del paso que fallo:\n%s", ca, f.todas())
				}
				// e5f706: despues del kill-session fallido, ni un sondeo ni otra
				// llamada a tmux
				tras := f.trasElCierre()
				f.mu.Lock()
				sondeos := f.sondeos
				f.mu.Unlock()
				var hs []string
				for _, l := range tras {
					if strings.HasPrefix(l, "tmux has-session") {
						hs = append(hs, l)
					}
				}
				if sondeos != 0 || len(hs) > 0 {
					t.Errorf("%s: e5f706: despues del kill-session fallido Open no llama a has-session (el resultado no prueba nada y tmux puede estar colgado), y hubo %d sondeos: %q\ntmux:\n%s", ca, sondeos, hs, f.todas())
				}
				if len(tras) > 0 {
					t.Errorf("%s: e5f706: despues del kill-session fallido Open no le vuelve a hablar a tmux, y le hizo %d llamadas: %q\ntmux:\n%s", ca, len(tras), tras, f.todas())
				}
				msg := err.Error()
				clave := paso.caso + " / " + c.kill.nombre
				mensajes[clave] = append(mensajes[clave], visto{c.caso, hb5Comparable(msg, root)})
				if mal := ciNoSabeSiSigueAbierta(msg, paso.original, h3Sesion); len(mal) > 0 {
					t.Errorf("%s: el cierre fallo y tmux no prueba si la sesion sigue abierta; el error:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, f.todas())
				}
				cierreEjecutable(t, ca, msg, h3Sesion)
				if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
					t.Errorf("%s: el item no declara una sesion que no quedo armada:\n%s", ca, ahora)
				}
			})
		}
	}

	// el mensaje no depende de has-session ni de si la sesion vive
	claves := make([]string, 0, len(mensajes))
	for k := range mensajes {
		claves = append(claves, k)
	}
	sort.Strings(claves)
	for _, k := range claves {
		vs := mensajes[k]
		distintos := map[string]bool{}
		for _, v := range vs {
			distintos[v.msg] = true
		}
		if len(distintos) > 1 {
			var b strings.Builder
			for _, v := range vs {
				fmt.Fprintf(&b, "\n- %s: %q", v.caso, v.msg)
			}
			t.Errorf("CA-347, CA-348: 9c1816: %s: el error es UNO SOLO, responda lo que responda tmux a has-session y este la sesion viva o muerta, y hay %d distintos:%s", k, len(distintos), b.String())
		}
	}
}

// GUARDA del contrato de C1 (hallazgos c1a9da y 363285), que el contrato
// nuevo no cambia: con kill-session SANO, Open cierra la sesion que acaba de
// crear y el error, ademas de conservar el del paso que fallo, dice que la
// cerro, para que el proximo intento la arme entera; no dice que no se pudo
// saber si sigue abierta. En los dos caminos (split-window roto, y registrar
// en el item que falla), con el mismo falso hc3Tmux de C1.
func TestCA347_CA348_Hallazgo9c1816_GuardaConElCierreSanoElErrorDiceQueLaCerro(t *testing.T) {
	for _, paso := range []struct {
		caso, fallar, original string
		soloLectura            bool
	}{
		{"split-window roto", "split-window", h3ErrPaso.Error(), false},
		{"registrar en el item falla", "", "permission denied", true},
	} {
		t.Run(paso.caso, func(t *testing.T) {
			ca := "CA-347, CA-348: guarda de C1: " + paso.caso + " (kill-session sano)"
			if paso.soloLectura && os.Geteuid() == 0 {
				t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
			}
			root, _ := cbProyecto(t, true)
			if paso.soloLectura {
				hc3ItemSoloLectura(t, root)
			}
			f := &hc3Tmux{h3Tmux: &h3Tmux{fallar: paso.fallar}}
			_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
			h3SinHuerfana(t, ca, f.h3Tmux, 0)
			if err == nil {
				t.Fatalf("%s: Open devuelve el error del paso que fallo:\n%s", ca, f.todas())
			}
			msg := err.Error()
			if !strings.Contains(msg, paso.original) {
				t.Errorf("%s: el error conserva el del paso que fallo (%q): %q", ca, paso.original, msg)
			}
			if !cierretmuxtest.DiceQueLaCerro(msg) {
				t.Errorf("%s: kill-session salio bien: el error dice que cerro la sesion, para que el proximo intento la arme entera: %q", ca, msg)
			}
			if cierretmuxtest.DiceQueNoSabe(msg) {
				t.Errorf("%s: kill-session salio bien: el error no dice que no se pudo saber si la sesion sigue abierta: %q", ca, msg)
			}
		})
	}
}

// Guarda del verificador (9c1816): ciNoSabeSiSigueAbierta acepta mensajes que
// cumplen el contrato nuevo aunque lo digan con otras palabras (un
// condicional, una disyuncion, con o sin tildes) y rechaza los del contrato
// de tres estados y los que no muestran el comando con comillas; y
// DiceQueLaCerro reconoce el "la cerre" de C1 y no una negacion. Sin esto,
// los tests de arriba podrian pasar o fallar por el verificador y no por el
// mensaje.
func TestCA347_Hallazgo9c1816_ElVerificadorDelMensaje(t *testing.T) {
	const s, orig = h3Sesion, "split-window: fallo"
	cmd := "tmux kill-session -t '=" + s + "'"
	for _, c := range []struct {
		msg  string
		vale bool
	}{
		{orig + "; no pude cerrar la sesion " + s + " (exit status 1) y no pude saber si sigue abierta: si sigue, cerrala y reintenta: " + cmd, true},
		{orig + "; no pude cerrar la sesión " + s + " y no sé si quedó abierta: " + cmd, true},
		{orig + "; no pude saber si la sesion " + s + " sigue abierta o ya no existe; si sigue, " + cmd, true},
		{orig + "; no pude cerrarla: puede que siga abierta (" + cmd + ")", true},
		{orig + "; la sesion " + s + " ya no existe: reintenta", false},
		{orig + "; la sesion " + s + " quedo abierta y no pude cerrarla (exit status 1): cerrala y reintenta: " + cmd, false},
		{orig + "; la sesión " + s + " quedó abierta, no sé si podrás cerrarla: " + cmd, false},
		{orig + "; no pude saber si sigue abierta: cerrala con tmux kill-session -t =" + s, false},
		{orig + "; no pude cerrar la sesion " + s + ": " + cmd, false},
		{orig + "; cerre la sesion " + s + ", pero no pude saber si sigue abierta: " + cmd, false},
		{"otro error; no pude saber si sigue abierta: " + cmd, false},
	} {
		if mal := ciNoSabeSiSigueAbierta(c.msg, orig, s); (len(mal) == 0) != c.vale {
			t.Errorf("9c1816: ciNoSabeSiSigueAbierta(%q) = %q, quiere que valga = %v", c.msg, mal, c.vale)
		}
	}
	for _, c := range []struct {
		msg   string
		cerro bool
	}{
		{orig + "; cerré la sesión " + s + " para que el próximo intento la arme entera", true},
		{orig + "; la cerre: reintenta", true},
		{orig + "; no la cerré: " + cmd, false},
		{orig + "; no pude cerrar la sesion " + s, false},
	} {
		if got := cierretmuxtest.DiceQueLaCerro(c.msg); got != c.cerro {
			t.Errorf("C1: DiceQueLaCerro(%q) = %v, quiere %v", c.msg, got, c.cerro)
		}
	}
}
