// Tests de los hallazgos b59c30, e0001e, 9c1816 y e5f706 sobre
// cockpitcmd.Open (CA-347, CA-348), escritos desde
// .hoom/specs/acciones-desde-la-tarjeta.md, con la ENMIENDA 6 (decision de
// Henry, hallazgo bde7e4):
//
// Open crea la sesion y despues falla el split-window o el registro de la
// sesion en el item. Con la enmienda 6, Open NO la cierra: no corre ningun
// kill-session (cerrarla por su nombre no prueba que sea la que creo: pudo
// morir y otra apertura crear una con el mismo nombre), devuelve el error, que
// dice como cerrarla a mano con `tmux kill-session -t '=<sesion>'`, y la
// sesion cuenta como existente. Lo verifican h6NoLaCerro e
// incumplimientosEnmienda6, en hallazgos_c1b_bde7e4_test.go: conserva el error
// del paso que fallo, no dice que la cerro y muestra el comando exacto, que
// corre tal cual en sh y en zsh (cierreEjecutable).
//
// Lo que sigue del contrato de 9c1816 y e5f706: el error es UNO SOLO, este la
// sesion viva o muerta despues del paso que fallo, y responda lo que responda
// tmux a has-session (tmux sale con codigo 1 tanto si la sesion no esta como
// si falla su propio cliente, asi que nada de lo que responda prueba si sigue
// abierta). Y no se le pregunta: despues de crear la sesion, Open no llama a
// has-session (cero sondeos) ni a ningun verbo de tmux fuera del plan
// (split-window y select-pane), porque un tmux que fallo puede estar degradado
// y una llamada mas puede no volver nunca (la CLI no le pone plazo).
//
// Antes (contrato de 9c1816, 57bd29 y 03481c) Open intentaba cerrar la sesion
// y la matriz rompia ese cierre de tres maneras; el error tenia que decir que
// no se pudo saber si sigue abierta. La enmienda 6 saco el cierre: un
// kill-session roto ya no tiene nada que romper, y "no se si sigue abierta"
// era la respuesta a un cierre fallido que ya no existe. No es debilitar:
// ahora se exige no tocar una sesion que Open no puede probar suya, y el
// mensaje unico se sigue exigiendo con la sesion viva y muerta.
//
// Los errores de tmux son los que da os/exec con procesos de verdad (/bin/sh,
// /bin/sleep), no errores de Go armados a mano, salvo los casos que son
// justamente eso (el error del contexto y el sondeo sin codigo de salida).
// Fixtures de open_capture_test.go (cbProyecto, cbLeerItem),
// hallazgos_c1_test.go (h3Tmux, h3Sesion, h3ErrPaso, h3Destino),
// hallazgos_c1c_test.go (hc3Tmux, hc3ItemSoloLectura),
// hallazgos_c1b_bde7e4_test.go (h6NoLaCerro, h6Cierres,
// h6ReintentoLaVeExistente, incumplimientosEnmienda6),
// hallazgos_db8541_f7efdc_test.go (cierreEjecutable) y tmuxdoble_test.go
// (tmuxSinSesion, tmuxClienteRoto). Ningun tmux real.
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
)

// hb5Tmux es el tmux falso con estado h3Tmux con las trampas de la matriz:
//
//   - has-session ANTES del new-session responde como tmux: nil si la sesion
//     vive, tmuxSinSesion (codigo 1) si no.
//   - has-session DESPUES del new-session (un sondeo: el plan de CA-82 y CA-85
//     no lo usa) se registra, se cuenta en sondeos y responde sondeo, sin
//     mirar el estado. Open no debe hacerlo (e5f706).
//   - kill-session se registra, se cuenta en cerrar y cierra la sesion, como
//     tmux sano. Open no debe correrlo (enmienda 6).
//   - en el verbo muere, despues de que h3Tmux lo atiende (falle o no), la
//     sesion deja de vivir: la CLI de la IA salio.
//
// El verbo en h3Tmux.fallar falla como siempre. Atiende igual por RunCmd,
// QuietCmd y Output, y todas las llamadas quedan en h3Tmux.llamadas.
type hb5Tmux struct {
	*h3Tmux
	muere   string
	sondeo  func() error
	creada  bool // ya corrio el new-session
	cerrar  int  // kill-session que se corrieron
	sondeos int  // has-session despues del new-session
}

func (f *hb5Tmux) atender(name string, args []string) ([]byte, error) {
	if name == "tmux" && len(args) > 0 {
		switch args[0] {
		case "kill-session":
			f.mu.Lock()
			f.cerrar++
			f.mu.Unlock()
		case "has-session":
			f.mu.Lock()
			creada := f.creada
			if creada {
				f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
				f.sondeos++
			}
			f.mu.Unlock()
			if creada {
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
	out, err := f.h3Tmux.atender(name, args)
	if name == "tmux" && len(args) > 0 {
		f.mu.Lock()
		if args[0] == "new-session" {
			f.creada = true
		}
		if args[0] == f.muere {
			delete(f.vivas, h3Sesion)
		}
		f.mu.Unlock()
	}
	return out, err
}

// trasLaCreacion son las llamadas a tmux despues del primer new-session, en
// orden (nil si no hubo new-session).
func (f *hb5Tmux) trasLaCreacion() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, l := range f.llamadas {
		if strings.HasPrefix(l, "tmux new-session") {
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

// hb5Caso es un caso de la matriz: si la sesion sigue viva despues del paso
// que fallo (si no, muere en el split-window), y que responde has-session
// despues del new-session (nil = sale bien). codigo1 dice si el sondeo es un
// *exec.ExitError con codigo 1 (fixture).
type hb5Caso struct {
	caso    string
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
		{"la sesion vive y has-session sale bien", true, nil, false},
		{"9c1816: la sesion vive y has-session sale con codigo 1 (falla el cliente)", true, tmuxClienteRoto(), true},
		{"la sesion murio y has-session sale con codigo 1 (can't find session)", false, tmuxSinSesion(h3Sesion), true},
		{"la sesion murio y has-session sale con codigo 1 (falla el cliente)", false, tmuxClienteRoto(), true},
		{"la sesion murio y has-session sale bien", false, nil, false},
		{"la sesion vive y has-session no arranca: tmux ya no esta en el PATH", true, noEsta, false},
		{"la sesion vive y has-session no arranca: el binario de tmux desaparecio", true, sinBinario, false},
		{"la sesion vive y has-session sale por una senal", true, senal, false},
		{"la sesion vive y has-session vence su plazo", true, plazo, false},
		{"la sesion vive y has-session vence su plazo (error del contexto)", true, fmt.Errorf("tmux has-session -t =%s: %w", h3Sesion, context.DeadlineExceeded), false},
		{"la sesion vive y has-session falla con un error sin codigo de salida", true, errors.New("tmux: " + tmuxClienteRotoMsg), false},
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
// find session: <sesion>" y "lost server" en Stderr. Y hb5Tmux (con las
// trampas de la enmienda 6) responde a has-session antes del new-session
// como tmux (codigo 1 si la sesion no vive); despues del new-session, con su
// trampa sondeo (contada en sondeos); deja morir la sesion en el verbo muere,
// y cuenta y cumple un kill-session.
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
	trampa := errors.New("trampa del sondeo")
	f := &hb5Tmux{h3Tmux: &h3Tmux{}, muere: "split-window", sondeo: func() error { return trampa }}
	d := f.deps()
	var ee *exec.ExitError
	if err := d.QuietCmd("", "tmux", "has-session", "-t", "="+h3Sesion); !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("CA-347: b59c30/e0001e: fixture: hb5Tmux responde has-session de una sesion que no vive, antes del new-session, con codigo 1: %#v", err)
	}
	if err := d.RunCmd("", "tmux", "new-session", "-d", "-s", h3Sesion); err != nil || !f.viva(h3Sesion) {
		t.Fatalf("CA-347: fixture: hb5Tmux crea la sesion: %v", err)
	}
	if _, err := d.Output("", "tmux", "has-session", "-t", "="+h3Sesion); !errors.Is(err, trampa) || f.sondeos != 1 {
		t.Fatalf("CA-347: e5f706: fixture: despues del new-session hb5Tmux responde has-session con la trampa y lo cuenta: %v, %d sondeos", err, f.sondeos)
	}
	if err := d.RunCmd("", "tmux", "split-window", "-t", "="+h3Sesion+":"); err != nil || f.viva(h3Sesion) {
		t.Fatalf("CA-347: fixture: en el verbo muere la sesion deja de vivir: %v, viva=%v", err, f.viva(h3Sesion))
	}
	g := &hb5Tmux{h3Tmux: &h3Tmux{}, sondeo: func() error { return nil }}
	dg := g.deps()
	if err := dg.RunCmd("", "tmux", "new-session", "-d", "-s", h3Sesion); err != nil {
		t.Fatal(err)
	}
	if err := dg.RunCmd("", "tmux", "kill-session", "-t", "="+h3Sesion); err != nil || g.viva(h3Sesion) || g.cerrar != 1 {
		t.Fatalf("CA-347: fixture: hb5Tmux cuenta el kill-session y cierra la sesion como tmux sano: %v, viva=%v, cerrar=%d", err, g.viva(h3Sesion), g.cerrar)
	}
}

// Hallazgos b59c30, e0001e, 9c1816 y e5f706 (CA-347, CA-348), con la
// ENMIENDA 6. Open crea la sesion de la tarea facturas y despues falla el
// split-window, o el registro de la sesion en el item (de solo lectura). La
// matriz cambia si la sesion sigue viva despues (o muere en el split-window:
// la CLI de la IA salio) y que responderia tmux a un has-session: codigo 1
// (con la sesion muerta, y con la sesion VIVA porque fallo el cliente:
// 9c1816), sale bien, no arranca, sale por una senal, vence su plazo o falla
// sin codigo de salida.
//
// En todas las filas: Open devuelve el error y no corre ningun kill-session
// ni kill-pane (cerrar == 0), asi que la sesion viva sigue viva; el error no
// tiene incumplimientos para incumplimientosEnmienda6 (conserva el del paso
// que fallo, no dice que la cerro y dice como cerrarla a mano con `tmux
// kill-session -t '=<sesion>'`), ese comando corre tal cual en sh y en zsh,
// y el item queda byte a byte igual (CA-348). Con el mismo paso fallido, el
// mensaje es identico en todas las filas: no depende de has-session ni de si
// la sesion vive.
//
// Hallazgo e5f706: despues del new-session no hay ningun has-session
// (sondeos == 0) ni ninguna llamada a tmux fuera del plan: con el
// split-window roto, nada despues del split-window; con el registro roto,
// solo split-window y select-pane, una vez cada uno. Las respuestas de
// has-session de la matriz son trampas: sin esta guarda, una regresion que
// sondeara y descartara el resultado dejaria el mismo mensaje y pasaria.
//
// Reemplaza a TestCA347_CA348_Hallazgo9c1816_SiElCierreFallaElErrorEsUnoSoloYNoSabeSiLaSesionSigueAbierta,
// que rompia el kill-session de Open de tres maneras y exigia que el error
// dijera que no se pudo saber si la sesion sigue abierta: el contrato cambio
// por la enmienda 6 (decision de Henry, hallazgo bde7e4), que saco el cierre.
// No es debilitar: ahora se exige no tocar una sesion que Open no puede
// probar suya, y el mensaje unico se sigue exigiendo.
func TestCA347_CA348_Hallazgo9c1816_Enmienda6_SinCerrarlaElErrorEsUnoSoloYDiceComoCerrarla(t *testing.T) {
	casos := hb5Casos(t)
	type visto struct{ caso, msg string }
	mensajes := map[string][]visto{} // por paso
	for _, paso := range []struct {
		caso, fallar, original string
		soloLectura            bool
	}{
		{"split-window roto", "split-window", h3ErrPaso.Error(), false},
		{"registrar en el item falla", "", "permission denied", true},
	} {
		for _, c := range casos {
			t.Run(paso.caso+"/"+c.caso, func(t *testing.T) {
				ca := "CA-347, CA-348: 9c1816 (enmienda 6): " + paso.caso + ": " + c.caso
				if paso.soloLectura && os.Geteuid() == 0 {
					t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
				}
				root, _ := cbProyecto(t, true)
				crudo := cbLeerItem(t, root)
				if paso.soloLectura {
					hc3ItemSoloLectura(t, root)
				}
				sondeo := c.sondeo
				f := &hb5Tmux{h3Tmux: &h3Tmux{fallar: paso.fallar}, sondeo: func() error { return sondeo }}
				if !c.viva {
					f.muere = "split-window"
				}

				_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
				nueva := f.indice("tmux new-session", 0)
				if nueva < 0 || f.indice("tmux split-window", nueva) < 0 {
					t.Fatalf("%s: fixture: Open creo la sesion y llego al split-window:\n%s", ca, f.todas())
				}
				h6NoLaCerro(t, ca, f.h3Tmux, err, paso.original)
				f.mu.Lock()
				cerrar, sondeos := f.cerrar, f.sondeos
				f.mu.Unlock()
				if cerrar != 0 {
					t.Errorf("%s: enmienda 6: Open no corre kill-session, y corrio %d:\n%s", ca, cerrar, f.todas())
				}
				if viva := f.viva(h3Sesion); viva != c.viva {
					t.Errorf("%s: la sesion %s viva=%v (quiere %v): Open no la toca:\n%s", ca, h3Sesion, viva, c.viva, f.todas())
				}
				// e5f706: ni un sondeo ni una llamada fuera del plan
				tras := f.trasLaCreacion()
				var hs []string
				for _, l := range tras {
					if strings.HasPrefix(l, "tmux has-session") {
						hs = append(hs, l)
					}
				}
				if sondeos != 0 || len(hs) > 0 {
					t.Errorf("%s: e5f706: despues de crear la sesion Open no llama a has-session (el resultado no prueba nada y tmux puede estar colgado), y hubo %d sondeos: %q\ntmux:\n%s", ca, sondeos, hs, f.todas())
				}
				var fuera []string
				vistos := map[string]int{}
				for i, l := range tras {
					verbo := strings.Fields(l)[1]
					vistos[verbo]++
					switch {
					case paso.fallar == "split-window" && (i > 0 || verbo != "split-window"):
						fuera = append(fuera, l)
					case paso.fallar == "" && ((verbo != "split-window" && verbo != "select-pane") || vistos[verbo] > 1):
						fuera = append(fuera, l)
					}
				}
				if len(fuera) > 0 {
					t.Errorf("%s: e5f706: despues del paso que fallo Open no le vuelve a hablar a tmux, y le hizo %d llamadas fuera del plan: %q\ntmux:\n%s", ca, len(fuera), fuera, f.todas())
				}
				if err != nil {
					mensajes[paso.caso] = append(mensajes[paso.caso], visto{c.caso, hb5Comparable(err.Error(), root)})
				}
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
			t.Errorf("CA-347, CA-348: 9c1816 (enmienda 6): %s: el error es UNO SOLO, responda lo que responda tmux a has-session y este la sesion viva o muerta, y hay %d distintos:%s", k, len(distintos), b.String())
		}
	}
}

// GUARDA de la ENMIENDA 6 (antes, del contrato de C1, hallazgos c1a9da y
// 363285): con kill-session SANO (que cerraria la sesion si Open lo
// corriera), Open tampoco la cierra, en los dos caminos (split-window roto, y
// registrar en el item que falla), con el mismo falso hc3Tmux de C1: no corre
// kill-session ni kill-pane, la sesion sigue viva, el error conserva el del
// paso que fallo, no dice que la cerro y dice como cerrarla a mano; el
// reintento con tmux sano la ve existente (created false) y no registra nada.
//
// Reemplaza a TestCA347_CA348_Hallazgo9c1816_GuardaConElCierreSanoElErrorDiceQueLaCerro,
// que exigia que Open la cerrara y lo dijera: el contrato cambio por la
// enmienda 6 (decision de Henry, hallazgo bde7e4). No es debilitar: ahora se
// exige no tocar una sesion que Open no puede probar suya.
func TestCA347_CA348_Hallazgo9c1816_Enmienda6_GuardaConElCierreSanoTampocoLaCierra(t *testing.T) {
	for _, paso := range []struct {
		caso, fallar, original string
		soloLectura            bool
	}{
		{"split-window roto", "split-window", h3ErrPaso.Error(), false},
		{"registrar en el item falla", "", "permission denied", true},
	} {
		t.Run(paso.caso, func(t *testing.T) {
			ca := "CA-347, CA-348: guarda de la enmienda 6: " + paso.caso + " (kill-session sano)"
			if paso.soloLectura && os.Geteuid() == 0 {
				t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
			}
			root, _ := cbProyecto(t, true)
			crudo := cbLeerItem(t, root)
			restaurar := func() {}
			if paso.soloLectura {
				restaurar = hc3ItemSoloLectura(t, root)
			}
			f := &hc3Tmux{h3Tmux: &h3Tmux{fallar: paso.fallar}}
			_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
			h6NoLaCerro(t, ca, f.h3Tmux, err, paso.original)
			if !f.viva(h3Sesion) {
				t.Errorf("%s: la sesion %s sigue viva: Open no la toca:\n%s", ca, h3Sesion, f.todas())
			}
			restaurar()
			h6ReintentoLaVeExistente(t, ca, root, f.h3Tmux, crudo)
		})
	}
}
