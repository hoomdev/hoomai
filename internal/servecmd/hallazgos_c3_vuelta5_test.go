// Tests de regresion de la quinta vuelta de las correcciones de la cabina C3
// (tarea fixes-cabina-c3) sobre .hoom/specs/acciones-desde-la-tarjeta.md, con
// el contrato de la ENMIENDA 3 (m): Abrir sesion nunca cierra una sesion
// despues de que `tmux new-session` fallo o vencio su plazo, porque no puede
// saber si la creo ella u otra apertura de la misma tarjeta; una sesion de la
// tarjeta con un solo pane quedo a medias y la proxima apertura la completa
// (split-window, created true, el item gana su entrada); una sesion completa
// (dos panes) sigue siendo existente.
//
//   - 6a6780 y d8d246 (CA-348): un new-session que vence su plazo, o que
//     falla con un error que no es "duplicate session", mientras OTRA apertura
//     de la misma tarjeta arma la sesion entera, no la cierra.
//   - 0fc76d (CA-347 y CA-348): la sesion de dos panes sigue siendo
//     existente (guarda), y la de un solo pane la completa la proxima
//     apertura, sea del Studio o de la CLI.
//
// El tmux falso es el de la vuelta 3 (v3Tmux, v3Marcas, v3Sembrar, v3Panes) y
// las aperturas las de la vuelta 4 (v4Lanzar, v4Esperar, v4Abrir,
// v4Apertura); los demas fixtures, los de acciones_tarjeta_test.go,
// tablero_test.go y hallazgos_c3_test.go. tmux y el CLI de IA son falsos.
package servecmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// Hallazgos 6a6780 y d8d246 (CA-348, enmienda 3: "si tmux new-session falla o
// vence su plazo, Open devuelve el error y no cierra nada: la sesion pudo
// crearla otra apertura de la misma tarjeta, y cerrarla mataria su CLI"). La
// apertura A llega primero: su sondeo no ve la sesion y su new-session queda
// retenido SIN crear nada. Mientras tanto la apertura B (otra pestana del
// Studio, o la CLI) arma la sesion entera y termina. Despues el new-session de
// A vence su plazo (se cuelga hasta que el Studio lo corta) o falla con un
// error que no es "duplicate session" (exit 1 con otro mensaje, exit 2, o
// muerto por una senal antes de cualquier plazo, desde el Studio o la CLI). Al
// final: B respondio created true; A respondio un error; nadie intento cerrar
// la sesion (ningun kill-session ni kill-pane de ella) ni quedo cerrada; la
// sesion de B sigue viva con sus dos panes; y el item tiene UNA entrada en
// sesiones (claude), la de B.
func TestHallazgo_6a6780_d8d246_UnNewSessionCortadoNoCierraLaSesionDeOtraApertura(t *testing.T) {
	for _, c := range []struct {
		caso    string
		luego   string // lo que hace el new-session retenido de A (v3Retencion)
		salida  int
		mensaje string
		a, b    string // "studio" o "cli"
	}{
		{"el new-session de A (Studio) vence su plazo y B es otra pestana", "colgar", 0, "", "studio", "studio"},
		{"el new-session de A (Studio) vence su plazo y B es la CLI", "colgar", 0, "", "studio", "cli"},
		{"el new-session de A (Studio) falla con exit 1 y otro mensaje", "fallar", 1, "server exited unexpectedly", "studio", "studio"},
		{"el new-session de A (CLI) falla con exit 2", "fallar", 2, "error connecting to /tmp/tmux-hoom/default (Connection refused)", "cli", "studio"},
		{"el new-session de A (Studio) muere por una senal antes del plazo", "matar", 0, "", "studio", "studio"},
		{"el new-session de A (CLI) muere por una senal", "matar", 0, "", "cli", "studio"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			ca := "6a6780 y d8d246 (CA-348, enmienda 3): " + c.caso
			const slug = "cortada"
			nombre := cockpitcmd.SessionName("demo", slug)
			f := v3Marcas(t)
			f.ret.nombre, f.ret.donde, f.ret.luego, f.ret.salida, f.ret.mensaje = nombre, "new-session", c.luego, c.salida, c.mensaje
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			s := newServer(t, dir)

			// A llega primero y su new-session queda retenido sin crear nada
			chA := v4Lanzar(s, dir, slug, c.a)
			if !h3EsperarHasta(10*time.Second, func() bool { _, err := os.Stat(f.ret.esperando); return err == nil }) {
				t.Fatalf("%s: fixture: el new-session de A (%s) llego a tmux y espera:\n%s", ca, c.a, strings.Join(atLineas(t, f.log), "\n"))
			}
			if v3Viva(f.ses, nombre) {
				t.Fatalf("%s: fixture: el new-session retenido de A no crea la sesion", ca)
			}
			// B arma la sesion entera mientras A sigue en su new-session
			b := v4Esperar(t, ca, f, v4Lanzar(s, dir, slug, c.b), "B")
			select {
			case a := <-chA:
				t.Fatalf("%s: fixture: A termino antes de que B armara la sesion (su plazo vencio antes de tiempo): %v\n%s", ca, a, strings.Join(atLineas(t, f.log), "\n"))
			default:
			}
			if c.luego != "colgar" {
				if err := os.WriteFile(f.ret.seguir, nil, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			a := v4Esperar(t, ca, f, chA, "A")
			lineas := strings.Join(atLineas(t, f.log), "\n")
			if c.luego != "colgar" {
				if _, err := os.Stat(f.ret.fallo); err != nil {
					t.Fatalf("%s: fixture: el new-session de A llego a su falla (%s), no lo corto el plazo antes:\n  %v\ntmux:\n%s", ca, c.luego, a, lineas)
				}
			}

			for _, x := range []v4Apertura{a, b} {
				if x.panico != nil {
					t.Fatalf("%s: %v", ca, x)
				}
			}
			if !b.creo(nombre) {
				t.Errorf("%s: B, que armo la sesion %s entera, responde created true: %v\ntmux:\n%s", ca, nombre, b, lineas)
			}
			if !a.fallo() {
				t.Errorf("%s: A, cuyo new-session %s, responde un error: %v\ntmux:\n%s", ca, map[string]string{"colgar": "vencio su plazo", "fallar": "fallo", "matar": "murio por una senal"}[c.luego], a, lineas)
			}
			if k := f.cierres(t, nombre); len(k) > 0 {
				t.Errorf("%s: A no cierra la sesion %s que armo B (mataria su CLI de IA), y hubo:\n%s\naperturas:\n  %v\n  %v\ntmux:\n%s", ca, nombre, strings.Join(k, "\n"), a, b, lineas)
			}
			if n := v3Panes(f.ses, nombre); n != 2 {
				t.Errorf("%s: la sesion %s de B sigue viva con sus dos panes, y tiene %d\naperturas:\n  %v\n  %v\ntmux:\n%s", ca, nombre, n, a, b, lineas)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
				t.Errorf("%s: el item tiene UNA entrada en sesiones (claude), la de B:\n%s", ca, crudo)
			}
		})
	}
}

// Guarda de 0fc76d (CA-347, enmienda 3: "una sesion completa (dos panes)
// sigue siendo existente"). Con la sesion de la tarjeta ya armada con sus dos
// panes, Abrir sesion (del Studio y de la CLI) responde created false con esa
// sesion, sin new-session ni split-window ni ningun cierre; la sesion queda con
// sus dos panes y el item no gana entradas (CA-348: con la sesion existente no
// escribe nada).
func TestHallazgo_0fc76d_CA347_UnaSesionConDosPanesSigueSiendoExistente(t *testing.T) {
	for _, quien := range []string{"studio", "cli"} {
		t.Run(quien, func(t *testing.T) {
			ca := "0fc76d (CA-347, guarda de la enmienda 3), desde " + quien
			const slug = "completa"
			nombre := cockpitcmd.SessionName("demo", slug)
			f := v3Marcas(t)
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			v3Sembrar(t, f.ses, nombre, 2)
			s := newServer(t, dir)

			a := v4Abrir(s, dir, slug, quien)
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if !a.cedio(nombre) || a.fallo() {
				t.Errorf("%s: con la sesion %s completa, Abrir sesion responde created false con esa sesion: %v\ntmux:\n%s", ca, nombre, a, tmux)
			}
			for _, verbo := range []string{"new-session", "split-window"} {
				if v := atConVerbo(lineas, verbo); len(v) > 0 {
					t.Errorf("%s: una sesion completa no se crea ni se completa (sin %s): %q", ca, verbo, v)
				}
			}
			if k := f.cierres(t, nombre); len(k) > 0 {
				t.Errorf("%s: abrir una sesion existente no la cierra: %q", ca, k)
			}
			if n := v3Panes(f.ses, nombre); n != 2 {
				t.Errorf("%s: la sesion %s sigue con sus dos panes, y tiene %d:\n%s", ca, nombre, n, tmux)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 0 {
				t.Errorf("%s: con la sesion existente el item no gana entradas (CA-348):\n%s", ca, crudo)
			}
		})
	}
}

// Hallazgo 0fc76d (CA-347 y CA-348, enmienda 3: "una sesion de la tarjeta con
// un solo pane quedo a medias: la proxima apertura le agrega el pane de
// vigilancia (split-window), la cuenta como creada (created true, y el item
// gana su entrada) y no la vuelve a crear"; "lo hace igual desde la CLI y
// desde el Studio"). Con la sesion de la tarjeta viva con un solo pane (el de
// la IA), Abrir sesion del Studio y de la CLI: sin new-session, con
// split-window sobre esa sesion (que queda con dos panes y el watch), created
// true con esa sesion, sin ningun cierre, y el item con exactamente una
// entrada (claude).
func TestHallazgo_0fc76d_CA348_LaProximaAperturaCompletaUnaSesionConUnSoloPane(t *testing.T) {
	for _, quien := range []string{"studio", "cli"} {
		t.Run(quien, func(t *testing.T) {
			ca := "0fc76d (CA-347, CA-348, enmienda 3), desde " + quien
			const slug = "a-medias"
			nombre := cockpitcmd.SessionName("demo", slug)
			f := v3Marcas(t)
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			v3Sembrar(t, f.ses, nombre, 1)
			s := newServer(t, dir)

			a := v4Abrir(s, dir, slug, quien)
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if a.panico != nil {
				t.Fatalf("%s: %v", ca, a)
			}
			if !a.creo(nombre) {
				t.Errorf("%s: la apertura que completa la sesion %s a medias la cuenta como creada (created true): %v\ntmux:\n%s", ca, nombre, a, tmux)
			}
			if v := atConVerbo(lineas, "new-session"); len(v) > 0 {
				t.Errorf("%s: la sesion a medias no se vuelve a crear (sin new-session): %q", ca, v)
			}
			if len(atConVerbo(lineas, "split-window")) == 0 || !strings.Contains(tmux, "status --watch") {
				t.Errorf("%s: la apertura le agrega el pane de vigilancia (split-window con `hoom status --watch`):\n%s", ca, tmux)
			}
			if n := v3Panes(f.ses, nombre); n != 2 {
				t.Errorf("%s: despues de completarla la sesion %s tiene dos panes, y tiene %d:\n%s", ca, nombre, n, tmux)
			}
			if k := f.cierres(t, nombre); len(k) > 0 {
				t.Errorf("%s: completar la sesion no la cierra: %q", ca, k)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
				t.Errorf("%s: el item gana su entrada: exactamente una en sesiones (claude):\n%s", ca, crudo)
			}
		})
	}
}
