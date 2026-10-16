// Tests de los hallazgos b59c30 y e0001e sobre cockpitcmd.Open (CA-347,
// CA-348), escritos desde .hoom/specs/acciones-desde-la-tarjeta.md y el
// contrato de los hallazgos:
//
// Open crea la sesion, algo falla despues (el split-window, o registrar la
// sesion en el item) y el cierre (`tmux kill-session -t =<sesion>`) TAMBIEN
// falla. Para saber que decir, Open sondea con `tmux has-session -t
// =<sesion>`, y el error tiene que decir la verdad sobre la sesion en uno de
// tres estados:
//
//	(A) tmux CONFIRMA que la sesion no esta: has-session sale con codigo 1
//	    (asi responde tmux de verdad, "can't find session: <sesion>"). El
//	    error dice que la sesion ya no existe y que se puede reintentar, sin
//	    pedir cerrarla a mano ni mostrar el comando de cierre (f7efdc).
//	(B) has-session sale bien: la sesion sigue viva. El error dice que quedo
//	    abierta y muestra `tmux kill-session -t '=<sesion>'` (363285, db8541).
//	(C) el sondeo NO confirma nada: no arranca (tmux ya no esta en el PATH o
//	    su binario desaparecio entre el kill y el sondeo), sale por una senal,
//	    vence su plazo (el Studio pone 5 s a cada llamada a tmux) o falla con
//	    un error que no es un codigo de salida. El error NO dice que la sesion
//	    ya no existe: muestra el comando de cierre con comillas (ejecutable en
//	    sh y zsh) y dice que no se pudo saber si sigue abierta.
//
// Antes del arreglo, cualquier fallo del sondeo daba "ya no existe": con la
// sesion viva, el error mentia y escondia el comando para cerrarla.
//
// Los errores del sondeo son los que da os/exec con procesos de verdad
// (/bin/sh, /bin/sleep), no errores de Go armados a mano, salvo los dos casos
// que son justamente eso (el error del contexto y uno sin codigo de salida).
// Fixtures de open_capture_test.go (cbProyecto, cbLeerItem),
// hallazgos_c1_test.go (h3Tmux, h3Sesion, h3ErrPaso), hallazgos_c1c_test.go
// (hc3Manual, hc3Llamada, hc3ItemSoloLectura) y
// hallazgos_db8541_f7efdc_test.go (cierreEjecutable). Ningun tmux real.
package cockpitcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tmuxSinSesion es el error con el que tmux de verdad responde has-session de
// una sesion que no existe: sale con codigo 1 y dice "can't find session:
// <sesion>" por stderr. Es el *exec.ExitError (ExitCode() == 1, con Stderr)
// de un proceso de verdad, no un error de Go armado a mano (b59c30, e0001e).
// Lo usan hb5Tmux y hf7Tmux (hallazgos_db8541_f7efdc_test.go).
func tmuxSinSesion(sesion string) error {
	_, err := exec.Command("/bin/sh", "-c", `printf "can't find session: %s\n" "$1" >&2; exit 1`, "sh", sesion).Output()
	return err
}

var hb5ErrKill = errors.New("tmux falso: kill-session fallo a proposito (hb5)")

// hb5Tmux es el tmux falso con estado h3Tmux con el cierre roto:
//
//   - has-session ANTES de cualquier kill-session responde como tmux: nil si
//     la sesion vive, tmuxSinSesion (codigo 1) si no.
//   - kill-session se registra y falla sin cerrar nada (hb5ErrKill); con
//     muere, la sesion ya habia muerto cuando Open intenta cerrarla (la CLI
//     de la IA salio): deja de vivir y kill-session falla como tmux, con
//     codigo 1.
//   - has-session DESPUES del kill-session (el sondeo) se registra, se cuenta
//     en sondeos y responde sondeo, sin mirar el estado.
//
// El verbo en h3Tmux.fallar falla como siempre. Atiende igual por RunCmd,
// QuietCmd y Output.
type hb5Tmux struct {
	*h3Tmux
	muere   bool
	sondeo  func() error
	cerrar  int // kill-session que se intentaron
	sondeos int // has-session despues del primer kill-session
}

func (f *hb5Tmux) atender(name string, args []string) ([]byte, error) {
	if name == "tmux" && len(args) > 0 {
		switch args[0] {
		case "kill-session":
			f.mu.Lock()
			f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
			f.cerrar++
			d := h3Destino(args, "-t")
			if f.muere {
				delete(f.vivas, d)
			}
			f.mu.Unlock()
			if f.muere {
				return nil, tmuxSinSesion(d)
			}
			return nil, hb5ErrKill
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

// sondeados es cuantos has-session hubo despues del primer kill-session.
func (f *hb5Tmux) sondeados() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sondeos
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

// hb5Sondeo es una respuesta del sondeo has-session despues del kill-session
// fallido: estado A, B o C, y el error (nil = la sesion vive).
type hb5Sondeo struct {
	caso   string
	estado string // "A", "B" o "C"
	err    error
}

// hb5Sondeos arma las respuestas del sondeo con os/exec de verdad, una vez por
// test. Los de C se comprueban como fixture: ninguno es un *exec.ExitError
// con codigo 1.
func hb5Sondeos(t *testing.T) []hb5Sondeo {
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

	sondeos := []hb5Sondeo{
		{"A: has-session sale con codigo 1 (la sesion no esta)", "A", tmuxSinSesion(h3Sesion)},
		{"B: has-session sale bien (la sesion vive)", "B", nil},
		{"C: has-session no arranca: tmux ya no esta en el PATH", "C", noEsta},
		{"C: has-session no arranca: el binario de tmux desaparecio", "C", sinBinario},
		{"C: has-session sale por una senal", "C", senal},
		{"C: has-session vence su plazo", "C", plazo},
		{"C: has-session vence su plazo (error del contexto)", "C", fmt.Errorf("tmux has-session -t =%s: %w", h3Sesion, context.DeadlineExceeded)},
		{"C: has-session falla con un error sin codigo de salida", "C", errors.New("tmux: lost server")},
	}
	for _, s := range sondeos {
		var ee *exec.ExitError
		uno := errors.As(s.err, &ee) && ee.ExitCode() == 1
		switch {
		case s.estado == "A" && !uno:
			t.Fatalf("fixture: %s: el sondeo responde un *exec.ExitError con codigo 1: %#v", s.caso, s.err)
		case s.estado == "B" && s.err != nil:
			t.Fatalf("fixture: %s: el sondeo sale bien: %v", s.caso, s.err)
		case s.estado == "C" && (s.err == nil || uno):
			t.Fatalf("fixture: %s: el sondeo falla sin el codigo 1 de tmux: %#v", s.caso, s.err)
		}
	}
	return sondeos
}

// hb5SinTildes pasa msg a minusculas sin tildes, para buscar frases.
func hb5SinTildes(msg string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u").Replace(strings.ToLower(msg))
}

// hb5Incierto son las maneras de decir que no se pudo saber si la sesion sigue
// abierta (sobre el mensaje sin tildes, en minusculas).
var hb5Incierto = []string{
	"no se pudo saber", "no pude saber", "no se puede saber", "no se sabe", "no se si",
	"no se pudo confirmar", "no pude confirmar", "no se puede confirmar",
	"no se pudo comprobar", "no pude comprobar", "no se pudo verificar", "no pude verificar",
	"puede que siga", "quiza siga", "quizas siga", "tal vez siga", "podria seguir",
}

// hb5Verdad exige que msg diga la verdad sobre la sesion segun el estado del
// sondeo, y devuelve que esta mal ([] si nada). original es el texto del error
// del paso que fallo, que el error conserva en los tres estados.
func hb5Verdad(t *testing.T, ca, estado, msg, original string) []string {
	t.Helper()
	bajo := hb5SinTildes(msg)
	var mal []string
	if !strings.Contains(msg, original) {
		mal = append(mal, fmt.Sprintf("conserva el error del paso que fallo (%q)", original))
	}
	if strings.Contains(bajo, "la cerre") {
		mal = append(mal, "no dice que la cerro: kill-session fallo")
	}
	switch estado {
	case "A":
		if !strings.Contains(bajo, "ya no existe") {
			mal = append(mal, "tmux confirmo que la sesion no esta (has-session salio con codigo 1): dice que ya no existe")
		}
		if !strings.Contains(bajo, "reintent") {
			mal = append(mal, "dice que se puede reintentar")
		}
		if strings.Contains(bajo, "quedo abierta") {
			mal = append(mal, "no dice que la sesion quedo abierta: ya no existe")
		}
		for _, no := range []string{"kill-session -t", "a mano", "cerrala"} {
			if strings.Contains(bajo, no) {
				mal = append(mal, fmt.Sprintf("no pide cerrarla a mano (contiene %q): no hay nada que cerrar", no))
			}
		}
	case "B":
		if strings.Contains(bajo, "ya no existe") {
			mal = append(mal, "la sesion sigue viva (has-session salio bien): no dice que ya no existe")
		}
		if !strings.Contains(bajo, "abierta") {
			mal = append(mal, "la sesion sigue viva: dice que quedo abierta")
		}
		if !strings.Contains(msg, hc3Manual) {
			mal = append(mal, "muestra como cerrarla a mano: "+hc3Manual)
		}
	case "C":
		if strings.Contains(bajo, "ya no existe") {
			mal = append(mal, "el sondeo no confirmo nada (no salio con el codigo 1 de tmux): no dice que la sesion ya no existe")
		}
		if !strings.Contains(msg, hc3Manual) {
			mal = append(mal, "muestra como cerrarla a mano, con el destino entre comillas: "+hc3Manual)
		}
		incierto := false
		for _, frase := range hb5Incierto {
			if strings.Contains(bajo, frase) {
				incierto = true
				break
			}
		}
		if !incierto {
			mal = append(mal, "dice que no se pudo saber si la sesion sigue abierta")
		}
	}
	if estado == "B" || estado == "C" {
		cierreEjecutable(t, ca, msg, h3Sesion)
	}
	return mal
}

// GUARDA del doble (b59c30, e0001e): tmuxSinSesion responde como tmux de
// verdad a has-session de una sesion que no existe, un *exec.ExitError con
// ExitCode() == 1 y "can't find session: <sesion>" en Stderr. Y hb5Tmux
// responde asi a has-session antes del kill-session, nil si la sesion vive.
func TestCA347_HallazgosB59c30E0001e_ElDobleRespondeComoTmux(t *testing.T) {
	err := tmuxSinSesion(h3Sesion)
	var ee *exec.ExitError
	if !errors.As(err, &ee) || ee.ExitCode() != 1 {
		t.Fatalf("CA-347: b59c30/e0001e: has-session de una sesion que no existe sale con codigo 1, como tmux: %#v", err)
	}
	if quiere := "can't find session: " + h3Sesion; !strings.Contains(string(ee.Stderr), quiere) {
		t.Fatalf("CA-347: b59c30/e0001e: tmux dice %q por stderr: %q", quiere, ee.Stderr)
	}
	f := &hb5Tmux{h3Tmux: &h3Tmux{}, sondeo: func() error { return nil }}
	d := f.deps()
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

// Hallazgos b59c30 y e0001e (CA-347, CA-348). Open crea la sesion de la tarea
// facturas, algo falla despues (el split-window, o registrar la sesion en el
// item, de solo lectura) y el cierre con `tmux kill-session -t =<sesion>`
// TAMBIEN falla. Con el mismo tmux falso, solo cambia lo que responde el
// sondeo has-session despues del kill-session:
//
//   - A (codigo 1, la sesion murio): el error dice que ya no existe y que se
//     puede reintentar, sin el comando de cierre ni "a mano".
//   - B (sale bien, la sesion vive): dice que quedo abierta y muestra `tmux
//     kill-session -t '=<sesion>'`, que corre tal cual en sh y en zsh.
//   - C (no arranca, senal, plazo, error sin codigo de salida; la sesion
//     vive): NO dice que ya no existe, muestra el comando de cierre con
//     comillas (que corre tal cual en sh y en zsh) y dice que no se pudo saber
//     si sigue abierta.
//
// En los tres, el error conserva el del paso que fallo, no dice que la cerro,
// y el item queda byte a byte igual (CA-348).
func TestCA347_CA348_HallazgosB59c30E0001e_ElErrorDiceLaVerdadSobreLaSesion(t *testing.T) {
	sondeos := hb5Sondeos(t)
	for _, paso := range []struct {
		caso, fallar, original string
		soloLectura            bool
	}{
		{"split-window roto", "split-window", h3ErrPaso.Error(), false},
		{"registrar en el item falla", "", "permission denied", true},
	} {
		for _, s := range sondeos {
			t.Run(paso.caso+"/"+s.caso, func(t *testing.T) {
				ca := "CA-347, CA-348: b59c30/e0001e: " + paso.caso + ": " + s.caso
				if paso.soloLectura && os.Geteuid() == 0 {
					t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
				}
				root, _ := cbProyecto(t, true)
				crudo := cbLeerItem(t, root)
				if paso.soloLectura {
					hc3ItemSoloLectura(t, root)
				}
				err0 := s.err
				f := &hb5Tmux{h3Tmux: &h3Tmux{fallar: paso.fallar}, muere: s.estado == "A", sondeo: func() error { return err0 }}

				_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
				nueva := f.indice("tmux new-session", 0)
				if nueva < 0 || f.indice(hc3Llamada, nueva) < 0 {
					t.Fatalf("%s: fixture: Open creo la sesion e intento cerrarla con `%s`:\n%s", ca, hc3Llamada, f.todas())
				}
				if f.indice("tmux has-session -t ="+h3Sesion, f.indice(hc3Llamada, nueva)) < 0 || f.sondeados() == 0 {
					t.Fatalf("%s: fixture: despues del kill-session fallido, Open sondea la sesion con `tmux has-session -t =%s`:\n%s", ca, h3Sesion, f.todas())
				}
				if viva := f.viva(h3Sesion); viva != (s.estado != "A") {
					t.Fatalf("%s: fixture: la sesion %s viva=%v:\n%s", ca, h3Sesion, viva, f.todas())
				}
				if err == nil {
					t.Fatalf("%s: Open devuelve el error del paso que fallo:\n%s", ca, f.todas())
				}
				msg := err.Error()
				if mal := hb5Verdad(t, ca, s.estado, msg, paso.original); len(mal) > 0 {
					t.Errorf("%s: el error no dice la verdad sobre la sesion (estado %s):\n- %s\nerror: %q\ntmux:\n%s", ca, s.estado, strings.Join(mal, "\n- "), msg, f.todas())
				}
				if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
					t.Errorf("%s: el item no declara una sesion que no quedo armada:\n%s", ca, ahora)
				}
			})
		}
	}
}
