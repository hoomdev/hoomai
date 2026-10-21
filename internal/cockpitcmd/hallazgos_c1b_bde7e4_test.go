// Tests del hallazgo 20261020T201503_bde7e4 sobre cockpitcmd.Open (CA-347,
// CA-348), escritos desde la ENMIENDA 6 de
// .hoom/specs/acciones-desde-la-tarjeta.md (decision de Henry: no tocar y
// avisar):
//
//	"Si lo que falla es el `split-window` o el registro en el item, despues de
//	que esta apertura creo la sesion, tampoco la cierra y devuelve el error con
//	el mismo comando de cierre: entre la creacion y el cierre la sesion pudo
//	morir y otra apertura crear una con el mismo nombre, y cerrarla por nombre
//	mataria esa."
//
// y su caso limite: "la apertura responde el error, que dice como cerrar la
// sesion a mano, y no la cierra; el reintento la ve existente y no registra
// nada". Antes, Open cerraba por su nombre (`tmux kill-session -t =<sesion>`)
// la sesion que acababa de crear, y el `=` evita coincidencias parciales pero
// no prueba que sea la misma sesion.
//
// Aca viven tambien las piezas del contrato de la enmienda 6 que usan los
// tests que antes exigian cerrar la sesion y que la enmienda 6 cambio
// (hallazgos_c1_test.go, hallazgos_c1c_test.go,
// hallazgos_b59c30_e0001e_test.go y hallazgos_db8541_f7efdc_test.go):
//
//   - incumplimientosEnmienda6: en que incumple el error. Conserva el del paso
//     que fallo ("devuelve el error"), no dice que la cerro (no la cierra) y
//     dice como cerrarla a mano con el comando exacto `tmux kill-session -t
//     '=<sesion>'`, con el destino entre comillas como el de un new-session
//     fallido (enmienda 5, hallazgo db8541), que nombra la sesion. Que ese
//     comando corra tal cual en sh y en zsh lo exige aparte cierreEjecutable.
//     No exige que diga "no se si sigue abierta": eso era del contrato del
//     kill-session fallido (9c1816), y con la enmienda 6 ya no hay
//     kill-session.
//   - h6NoLaCerro: Open fallo, no corrio ningun kill-session ni kill-pane y su
//     error cumple lo de arriba.
//   - h6ReintentoLaVeExistente: con tmux sano, el reintento responde created
//     false sin error, no crea, no completa ni cierra la sesion, y no registra
//     nada en el item.
//   - h6CerrarAMano: lo que hace una persona con el comando del error; despues,
//     el reintento la crea y la registra (h3Reintento): el riesgo "Una sesion
//     a medias queda como esta" dice que el writer queda sin declarar "hasta
//     que alguien la cierra a mano (el error dice como) y la vuelve a abrir".
//
// Fixtures de open_capture_test.go (cbProyecto, cbLeerItem),
// hallazgos_c1_test.go (h3Tmux, h3Sesion, h3ErrPaso, h3Destino, h3Reintento),
// hallazgos_c1c_test.go (hc3ItemSoloLectura) y hallazgos_db8541_f7efdc_test.go
// (cierreEjecutable), y el verificador compartido internal/cierretmuxtest
// (DiceQueLaCerro). Ningun tmux real.
package cockpitcmd

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/cierretmuxtest"
)

// h6Comando es el comando con el que una persona cierra a mano la sesion
// sesion, el que propone el error (con el destino entre comillas: enmienda
// 5, hallazgo db8541).
func h6Comando(sesion string) string {
	return "tmux kill-session -t '=" + sesion + "'"
}

// incumplimientosEnmienda6 devuelve en que incumple msg el contrato de la
// enmienda 6: msg es el error de Open cuando creo la sesion sesion y despues
// fallo el split-window o el registro en el item (original es el error de ese
// paso). No es un predicado: devuelve la lista de incumplimientos, vacia si
// msg cumple todo.
func incumplimientosEnmienda6(msg, original, sesion string) []string {
	var mal []string
	if !strings.Contains(msg, original) {
		mal = append(mal, "conserva el error del paso que fallo ("+original+")")
	}
	if cierretmuxtest.DiceQueLaCerro(msg) {
		mal = append(mal, "no dice que la cerro: con la enmienda 6 Abrir sesion no la cierra")
	}
	if cmd := h6Comando(sesion); !strings.Contains(msg, cmd) {
		mal = append(mal, "dice como cerrarla a mano con el comando exacto `"+cmd+"`, que nombra la sesion")
	}
	return mal
}

// h6Cierres son las llamadas a tmux desde la posicion desde que cierran una
// sesion o un pane: kill-session y kill-pane, con cualquier destino.
func h6Cierres(f *h3Tmux, desde int) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for i := desde; i < len(f.llamadas); i++ {
		if l := f.llamadas[i]; strings.HasPrefix(l, "tmux kill-session") || strings.HasPrefix(l, "tmux kill-pane") {
			out = append(out, l)
		}
	}
	return out
}

// h6NoLaCerro exige el contrato de la enmienda 6 sobre un Open que creo la
// sesion y fallo despues: devuelve el error, no corrio ningun kill-session
// ni kill-pane, y el error conserva el original, no dice que la cerro y dice
// como cerrarla a mano con el comando exacto, que corre tal cual en sh y en
// zsh. Si la sesion sigue viva lo mira cada test: depende del escenario.
func h6NoLaCerro(t *testing.T, ca string, f *h3Tmux, err error, original string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: Open devuelve el error del paso que fallo:\n%s", ca, f.todas())
	}
	if k := h6Cierres(f, 0); len(k) > 0 {
		t.Errorf("%s: enmienda 6 (bde7e4): Open no cierra la sesion que creo (cerrarla por su nombre puede matar otra con el mismo nombre), y corrio %q\ntmux:\n%s", ca, k, f.todas())
	}
	msg := err.Error()
	if mal := incumplimientosEnmienda6(msg, original, h3Sesion); len(mal) > 0 {
		t.Errorf("%s: enmienda 6: el error:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, f.todas())
	}
	cierreEjecutable(t, ca, msg, h3Sesion)
}

// h6ReintentoLaVeExistente exige el reintento de la enmienda 6: con tmux sano
// (y el item escribible), Open ve la sesion que quedo existente: created
// false con su nombre y sin error, sin new-session, split-window,
// kill-session ni kill-pane; la sesion sigue viva y el item queda byte a byte
// como crudo (no registra nada).
func h6ReintentoLaVeExistente(t *testing.T, ca, root string, f *h3Tmux, crudo []byte) {
	t.Helper()
	f.mu.Lock()
	f.fallar = ""
	desde := len(f.llamadas)
	f.mu.Unlock()
	s, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
	if err != nil {
		t.Fatalf("%s: enmienda 6: el reintento con tmux sano ve la sesion existente y no falla: %v\n%s", ca, err, f.todas())
	}
	if s.Created || s.Name != h3Sesion {
		t.Errorf("%s: enmienda 6: la sesion cuenta como existente: el reintento responde created false con %s: %+v\n%s", ca, h3Sesion, s, f.todas())
	}
	for _, v := range []string{"tmux new-session", "tmux split-window"} {
		if f.indice(v, desde) >= 0 {
			t.Errorf("%s: enmienda 6: el reintento no crea ni completa la sesion existente (sin %s):\n%s", ca, strings.TrimPrefix(v, "tmux "), f.todas())
		}
	}
	if k := h6Cierres(f, desde); len(k) > 0 {
		t.Errorf("%s: enmienda 6: el reintento tampoco cierra la sesion: %q\n%s", ca, k, f.todas())
	}
	if !f.viva(h3Sesion) {
		t.Errorf("%s: enmienda 6: despues del reintento la sesion %s sigue viva:\n%s", ca, h3Sesion, f.todas())
	}
	if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
		t.Errorf("%s: CA-348, enmienda 6: con la sesion existente el reintento no registra nada en el item:\n%s", ca, ahora)
	}
}

// h6CerrarAMano corre en el tmux falso lo que el comando del error le pasa a
// tmux (`kill-session -t =<sesion>`, ver cierreEjecutable): una persona cerro
// la sesion a mano.
func h6CerrarAMano(t *testing.T, ca string, f *h3Tmux) {
	t.Helper()
	if _, err := f.atender("tmux", []string{"kill-session", "-t", "=" + h3Sesion}); err != nil || f.viva(h3Sesion) {
		t.Fatalf("%s: fixture: cerrar la sesion a mano con %s: %v\n%s", ca, h6Comando(h3Sesion), err, f.todas())
	}
}

// Guarda del verificador de la enmienda 6: incumplimientosEnmienda6 acepta un
// error que conserva el original, no dice que la cerro y propone el comando
// exacto (con o sin negar que la cerro, como el de un new-session fallido), y
// rechaza, cada uno por su regla, el que pierde el original, el que dice que
// la cerro y el que propone el comando sin comillas o con otra sesion. Sin
// esto, los tests de la enmienda 6 podrian pasar o fallar por el verificador
// y no por el mensaje.
func TestCA347_CA348_HallazgoBde7e4_ElVerificadorDeLaEnmienda6(t *testing.T) {
	const s, orig = h3Sesion, "split-window: fallo"
	cmd := h6Comando(s)
	for _, c := range []struct {
		msg, porque string
		regla       string // "" = vale; si no, la palabra del unico incumplimiento
	}{
		{orig + "; si la sesion " + s + " quedo a medias (sin el pane de vigilancia), cerrala y reintenta: " + cmd, "como el error de un new-session fallido", ""},
		{orig + "; no cerre la sesion " + s + ": si sigue abierta, cerrala a mano con " + cmd + " antes de reintentar", "niega que la cerro", ""},
		{"otro error; no cerre la sesion " + s + ": " + cmd, "pierde el error del paso que fallo", orig},
		{orig + "; cerre la sesion " + s + " para que el proximo intento la arme entera: " + cmd, "dice que la cerro", "la cerro"},
		{orig + "; si quedo a medias, cerrala: tmux kill-session -t =" + s, "el comando sin comillas", cmd},
		{orig + "; si quedo a medias, cerrala: tmux kill-session -t '=hoom-demo-otra'", "el comando de otra sesion", cmd},
	} {
		mal := incumplimientosEnmienda6(c.msg, orig, s)
		if c.regla == "" {
			if len(mal) != 0 {
				t.Errorf("bde7e4 (%s): incumplimientosEnmienda6(%q) = %q, quiere que valga", c.porque, c.msg, mal)
			}
			continue
		}
		if len(mal) != 1 || !strings.Contains(mal[0], c.regla) {
			t.Errorf("bde7e4 (%s): incumplimientosEnmienda6(%q) = %q, quiere un solo incumplimiento, el de %q", c.porque, c.msg, mal, c.regla)
		}
	}
}

// hbdTmux es el tmux falso con estado h3Tmux en el que, despues del verbo
// cuando (que h3Tmux atiende primero, falle o no), la sesion que creo Open
// muere (la CLI de la IA salio) y otra apertura de la misma tarjeta (otra
// pestana, o la CLI) crea una sustituta con el MISMO nombre: la carrera del
// hallazgo bde7e4. Lleva la generacion de la sesion h3Sesion viva (gen) para
// saber cual es: la original es la 1 y la sustituta la 2. kill-session cierra
// la que este viva con ese nombre, como tmux: la sustituta.
type hbdTmux struct {
	*h3Tmux
	cuando    string
	gen       int // generacion de la sesion h3Sesion viva (0 = ninguna)
	creadas   int
	sustituta int // generacion de la sustituta (0 = todavia no aparecio)
}

func (f *hbdTmux) atender(name string, args []string) ([]byte, error) {
	out, err := f.h3Tmux.atender(name, args)
	if name != "tmux" || len(args) == 0 {
		return out, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	switch args[0] {
	case "new-session":
		if err == nil && h3Destino(args, "-s") == h3Sesion {
			f.creadas++
			f.gen = f.creadas
		}
	case "kill-session":
		if h3Destino(args, "-t") == h3Sesion {
			f.gen = 0
		}
	}
	if args[0] == f.cuando && f.sustituta == 0 && f.gen != 0 {
		f.creadas++
		f.gen, f.sustituta = f.creadas, f.creadas
		f.vivas[h3Sesion] = true
		f.llamadas = append(f.llamadas, "(fuera de Open) la sesion "+h3Sesion+" murio y otra apertura creo una sustituta con el mismo nombre")
	}
	return out, err
}

func (f *hbdTmux) deps() Deps {
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

// estadoSustituta dice si la sustituta aparecio (aparecio) y si es ella la
// sesion h3Sesion viva (vive).
func (f *hbdTmux) estadoSustituta() (aparecio, vive bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.sustituta != 0, f.sustituta != 0 && f.gen == f.sustituta && f.vivas[h3Sesion]
}

// Hallazgo 20261020T201503_bde7e4 (CA-347, CA-348), enmienda 6. Open crea la
// sesion de la tarea facturas y despues falla el split-window, o el registro
// en el item (de solo lectura). Entre el new-session y ese fallo, la sesion
// que creo Open muere y otra apertura de la misma tarjeta crea una sustituta
// con el mismo nombre: justo despues del new-session (la CLI de la IA salio
// al arrancar), o en el split-window. En las dos ramas y los dos momentos:
//
//   - Open devuelve el error y no corre ningun kill-session ni kill-pane: la
//     sustituta, que no es suya, sigue viva (cerrar por nombre la mataria,
//     con su CLI adentro);
//   - el error conserva el del paso que fallo, no dice que la cerro y dice
//     como cerrarla a mano con `tmux kill-session -t '=<sesion>'`, que corre
//     tal cual en sh y en zsh;
//   - el item no declara nada, y el reintento con tmux sano ve la sesion
//     existente (created false), no la toca y no registra nada; la sustituta
//     sigue viva.
func TestCA347_CA348_HallazgoBde7e4_OpenNoCierraUnaSustitutaConElMismoNombre(t *testing.T) {
	for _, paso := range []struct {
		caso, fallar, original string
		soloLectura            bool
	}{
		{"split-window falla", "split-window", h3ErrPaso.Error(), false},
		{"registrar en el item falla", "", "permission denied", true},
	} {
		for _, cuando := range []string{"new-session", "split-window"} {
			t.Run(paso.caso+"/la sustituta aparece tras "+cuando, func(t *testing.T) {
				ca := "CA-347, CA-348: bde7e4 (enmienda 6): " + paso.caso + ", la sustituta aparece tras " + cuando
				if paso.soloLectura && os.Geteuid() == 0 {
					t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
				}
				root, _ := cbProyecto(t, true)
				crudo := cbLeerItem(t, root)
				restaurar := func() {}
				if paso.soloLectura {
					restaurar = hc3ItemSoloLectura(t, root)
				}
				f := &hbdTmux{h3Tmux: &h3Tmux{fallar: paso.fallar}, cuando: cuando}

				_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
				if aparecio, _ := f.estadoSustituta(); f.indice("tmux new-session", 0) < 0 || !aparecio {
					t.Fatalf("%s: fixture: Open creo la sesion y, antes del paso que fallo, murio y aparecio la sustituta:\n%s", ca, f.todas())
				}
				h6NoLaCerro(t, ca, f.h3Tmux, err, paso.original)
				if _, vive := f.estadoSustituta(); !vive {
					t.Errorf("%s: la sustituta %s que creo otra apertura sigue viva: Open no cierra por nombre una sesion que no puede probar suya:\n%s", ca, h3Sesion, f.todas())
				}
				if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
					t.Errorf("%s: CA-348: una apertura que fallo no declara ninguna sesion en el item:\n%s", ca, ahora)
				}

				restaurar()
				h6ReintentoLaVeExistente(t, ca, root, f.h3Tmux, crudo)
				if _, vive := f.estadoSustituta(); !vive {
					t.Errorf("%s: despues del reintento la sustituta sigue viva:\n%s", ca, f.todas())
				}
			})
		}
	}
}
