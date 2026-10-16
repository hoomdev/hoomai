// Tests de regresion de la quinta vuelta de las correcciones de la cabina C3
// (tarea fixes-cabina-c3) sobre .hoom/specs/acciones-desde-la-tarjeta.md, con
// el contrato de la ENMIENDA 4, que reemplazo a la 3 por decision de Henry:
// despues de que `tmux new-session` fallo o vencio su plazo, Abrir sesion
// devuelve el error y no toca la sesion (ni la cierra ni la completa), porque
// no puede saber si la creo ella u otra apertura de la misma tarjeta, ni que
// provider corre en su pane; una sesion de la tarjeta con un solo pane (a
// medias) cuenta como existente, igual que una completa (dos panes).
//
//   - 6a6780 y d8d246 (CA-348): un new-session que vence su plazo, o que
//     falla con un error que no es "duplicate session", mientras OTRA apertura
//     de la misma tarjeta arma la sesion entera, no la cierra.
//   - 0fc76d (CA-347 y CA-348): la sesion de dos panes sigue siendo
//     existente (guarda), y la de un solo pane tambien: la proxima apertura,
//     sea del Studio o de la CLI, responde created false y no la completa.
//
// El tmux falso (tmuxFalso, nuevoTmuxFalso) y las aperturas (abrirSesion,
// lanzarApertura, esperarApertura, apertura) estan en tmuxfalso_test.go; los
// demas fixtures, en acciones_tarjeta_test.go, tablero_test.go y
// hallazgos_c3_test.go. tmux y el CLI de IA son falsos.
package servecmd

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// Hallazgos 6a6780 y d8d246 (CA-348, enmienda 4, que en esto repite a la 3:
// "si tmux new-session falla o vence su plazo, Open devuelve el error y no
// toca la sesion: ni la cierra ni la completa. La sesion pudo crearla otra
// apertura de la misma tarjeta, y cerrarla mataria su CLI"). La apertura A
// llega primero: su sondeo no ve la sesion y su new-session queda retenido SIN
// crear nada. Mientras tanto la apertura B (otra pestana del
// Studio, o la CLI) arma la sesion entera y termina. Despues el new-session de
// A vence su plazo (se cuelga hasta que el Studio lo corta) o falla con un
// error que no es "duplicate session" (exit 1 con otro mensaje, exit 2, o
// muerto por una senal antes de cualquier plazo, desde el Studio o la CLI). Al
// final: B respondio created true; A respondio un error; nadie intento cerrar
// la sesion (ningun kill-session ni kill-pane de ella) ni quedo cerrada; la
// sesion de B sigue viva con sus dos panes (A tampoco le agrega uno); y el
// item tiene UNA entrada en sesiones (claude), la de B. Lo que dice el error
// de A no se mira aca: el aviso de como cerrar la sesion lo prueba
// TestHallazgo_0fc76d_AbrirSesionAMediasQuedaComoEstaYElErrorDiceComoCerrarla.
func TestHallazgo_6a6780_d8d246_UnNewSessionCortadoNoCierraLaSesionDeOtraApertura(t *testing.T) {
	for _, c := range []struct {
		caso    string
		luego   string // lo que hace el new-session retenido de A (retencion)
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
			ca := "6a6780 y d8d246 (CA-348, enmienda 4): " + c.caso
			const slug = "cortada"
			nombre := cockpitcmd.SessionName("demo", slug)
			f := nuevoTmuxFalso(t)
			f.ret.nombre, f.ret.donde, f.ret.luego, f.ret.salida, f.ret.mensaje = nombre, "new-session", c.luego, c.salida, c.mensaje
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			s := newServer(t, dir)

			// A llega primero y su new-session queda retenido sin crear nada
			chA := lanzarApertura(s, dir, slug, c.a)
			if !h3EsperarHasta(10*time.Second, func() bool { _, err := os.Stat(f.ret.esperando); return err == nil }) {
				t.Fatalf("%s: fixture: el new-session de A (%s) llego a tmux y espera:\n%s", ca, c.a, strings.Join(atLineas(t, f.log), "\n"))
			}
			if f.viva(nombre) {
				t.Fatalf("%s: fixture: el new-session retenido de A no crea la sesion", ca)
			}
			// B arma la sesion entera mientras A sigue en su new-session
			b := esperarApertura(t, ca, f, lanzarApertura(s, dir, slug, c.b), "B")
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
			a := esperarApertura(t, ca, f, chA, "A")
			lineas := strings.Join(atLineas(t, f.log), "\n")
			if c.luego != "colgar" {
				if _, err := os.Stat(f.ret.fallo); err != nil {
					t.Fatalf("%s: fixture: el new-session de A llego a su falla (%s), no lo corto el plazo antes:\n  %v\ntmux:\n%s", ca, c.luego, a, lineas)
				}
			}

			for _, x := range []apertura{a, b} {
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
			if n := f.panes(nombre); n != 2 {
				t.Errorf("%s: la sesion %s de B sigue viva con sus dos panes, y tiene %d\naperturas:\n  %v\n  %v\ntmux:\n%s", ca, nombre, n, a, b, lineas)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
				t.Errorf("%s: el item tiene UNA entrada en sesiones (claude), la de B:\n%s", ca, crudo)
			}
		})
	}
}

// Guarda de 0fc76d (CA-347: "con la sesion existente devuelve created: false y
// no crea nada"; una sesion completa es existente con la enmienda 4 igual que
// con la 3). Con la sesion de la tarjeta ya armada con sus dos panes, Abrir
// sesion (del Studio y de la CLI) responde created false con esa
// sesion, sin new-session ni split-window ni ningun cierre; la sesion queda con
// sus dos panes y el item no gana entradas (CA-348: con la sesion existente no
// escribe nada).
func TestHallazgo_0fc76d_CA347_UnaSesionConDosPanesSigueSiendoExistente(t *testing.T) {
	for _, quien := range []string{"studio", "cli"} {
		t.Run(quien, func(t *testing.T) {
			ca := "0fc76d (CA-347, guarda de la enmienda 4), desde " + quien
			const slug = "completa"
			nombre := cockpitcmd.SessionName("demo", slug)
			f := nuevoTmuxFalso(t)
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			f.sembrar(t, nombre, 2)
			s := newServer(t, dir)

			a := abrirSesion(s, dir, slug, quien)
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
			if n := f.panes(nombre); n != 2 {
				t.Errorf("%s: la sesion %s sigue con sus dos panes, y tiene %d:\n%s", ca, nombre, n, tmux)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 0 {
				t.Errorf("%s: con la sesion existente el item no gana entradas (CA-348):\n%s", ca, crudo)
			}
		})
	}
}

// Hallazgo 0fc76d (CA-347 y CA-348, ENMIENDA 4: "una sesion a medias cuenta
// como existente: el reintento responde created: false y no registra nada";
// "si la sesion ya existia, no registra nada. Lo hace igual desde la CLI y
// desde el Studio"; CA-347: "con la sesion existente devuelve created: false y
// no crea nada"; CA-348: "con la sesion existente no escribe nada").
//
// Este test REEMPLAZA a TestHallazgo_0fc76d_CA348_LaProximaAperturaCompletaUnaSesionConUnSoloPane,
// escrito para la enmienda 3 (la proxima apertura completaba la sesion a
// medias con split-window, la contaba como creada y el item ganaba su
// entrada). La enmienda 4 reemplazo a la 3 por decision de Henry: invertir lo
// que exigia completar no es debilitar el test, es el contrato aprobado nuevo.
//
// Con la sesion de la tarjeta viva con un solo pane (el de la IA), Abrir
// sesion del Studio y de la CLI responde created false con esa sesion (no un
// error), sin new-session, sin split-window y sin ningun cierre (kill-session
// ni kill-pane); la sesion sigue con un solo pane y el item no gana entradas en
// sesiones.
func TestHallazgo_0fc76d_CA348_UnaSesionConUnSoloPaneCuentaComoExistente(t *testing.T) {
	for _, quien := range []string{"studio", "cli"} {
		t.Run(quien, func(t *testing.T) {
			ca := "0fc76d (CA-347, CA-348, enmienda 4), desde " + quien
			const slug = "a-medias"
			nombre := cockpitcmd.SessionName("demo", slug)
			f := nuevoTmuxFalso(t)
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			f.sembrar(t, nombre, 1)
			s := newServer(t, dir)

			a := abrirSesion(s, dir, slug, quien)
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if a.panico != nil {
				t.Fatalf("%s: %v", ca, a)
			}
			if a.fallo() || !a.cedio(nombre) {
				t.Errorf("%s: con la sesion %s a medias (un solo pane), Abrir sesion la ve existente: created false con esa sesion, no un error: %v\ntmux:\n%s", ca, nombre, a, tmux)
			}
			for _, verbo := range []string{"new-session", "split-window"} {
				if v := atConVerbo(lineas, verbo); len(v) > 0 {
					t.Errorf("%s: una sesion a medias no se crea de nuevo ni se completa (sin %s): %q", ca, verbo, v)
				}
			}
			if k := f.cierres(t, nombre); len(k) > 0 {
				t.Errorf("%s: abrir una sesion a medias no la cierra: %q", ca, k)
			}
			if n := f.panes(nombre); n != 1 {
				t.Errorf("%s: la sesion %s sigue a medias, con un solo pane, y tiene %d:\n%s", ca, nombre, n, tmux)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 0 {
				t.Errorf("%s: con la sesion existente (a medias) el item no gana entradas:\n%s", ca, crudo)
			}
		})
	}
}
