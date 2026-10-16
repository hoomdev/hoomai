// Tests del hallazgo 337b23 sobre .hoom/specs/acciones-desde-la-tarjeta.md,
// con el contrato de la ENMIENDA 5 (n): "los comandos de tmux que apuntan a la
// sesion de la tarjeta usan el destino exacto `=<session>:` (con los dos
// puntos), porque tmux resuelve un nombre suelto, y tambien `=<session>` en
// `list-panes`, como ventana o como prefijo de otra sesion: Ver terminal lista
// los panes con `tmux list-panes -t '=<session>:'`, y el pane de vigilancia se
// agrega con ese mismo destino". Precisa CA-350 (y, por el pane de vigilancia,
// CA-347 y CA-348).
//
//   - (a) CA-347 y CA-348: la sesion que Abrir sesion acaba de crear muere
//     antes del pane de vigilancia (la CLI de la IA sale al arrancar) mientras
//     la sesion de fix-login esta abierta. El pane de vigilancia no cae en
//     fix-login, y la apertura no responde created true ni declara en el item
//     una sesion que no existe: responde un error.
//   - (b) CA-350: la sesion de la tarjeta fix se cierra entre el sondeo y el
//     listado de sus panes, con fix-login abierta. Ver terminal de fix no
//     muestra la pantalla de fix-login: responde available false con la nota.
//
// El tmux falso corre con tmuxFalso.destinosComoTmux (en tmuxfalso_test.go,
// que dice todo lo que cambia ese modo; hasta el hallazgo 5cee1e se llamaba
// real): resuelve cada destino como tmux 3.x (visto con tmux 3.6b): un nombre
// suelto es la sesion exacta o la UNICA con ese prefijo, `=N` es exacto en
// has-session y kill-session pero en list-panes no, y `=N:` es exacto en
// todos y falla con "can't find session" si N no vive. Fixtures de
// acciones_tarjeta_test.go (atPATH, atConEspacio, atOK, atLineas, atConVerbo,
// atLeerItem, atTerminal), tablero_test.go (tbProyecto) y hallazgos_c3_test.go
// (h3EsperarHasta). tmux y el CLI de IA son falsos.
package servecmd

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// estadoDe es el archivo de la sesion nombre en el tmux falso: sus panes, uno
// por linea ("" si no vive).
func (f *tmuxFalso) estadoDe(nombre string) string {
	raw, _ := os.ReadFile(filepath.Join(f.ses, nombre))
	return string(raw)
}

// idsDePanes son los ids (%<n>) de los panes de la sesion nombre.
func (f *tmuxFalso) idsDePanes(nombre string) []string {
	var out []string
	for _, l := range strings.Split(f.estadoDe(nombre), "\n") {
		if c := strings.Fields(l); len(c) > 0 {
			out = append(out, c[0])
		}
	}
	return out
}

// Guarda del tmux falso con destinosComoTmux (337b23; lo usan los tests de
// CA-347, CA-348 y CA-350 de abajo): resuelve los destinos como tmux 3.6b, que es lo
// que hace que el destino de antes de la enmienda 5 caiga en otra sesion. Si
// el doble dejara de emularlo, los tests de 337b23 pasarian sin probar nada.
func TestHallazgo_337b23_CA350_ElTmuxFalsoResuelveLosDestinosComoTmux(t *testing.T) {
	f := nuevoTmuxFalso(t)
	f.destinosComoTmux = true
	bin := atPATH(t, map[string]string{"tmux": f.script()})
	login := cockpitcmd.SessionName("demo", "fix-login")
	propia := cockpitcmd.SessionName("demo", "fix")
	f.sembrar(t, login, 2)
	tmux := func(args ...string) (string, bool) {
		out, err := exec.Command(filepath.Join(bin, "tmux"), args...).CombinedOutput()
		return strings.TrimSpace(string(out)), err == nil
	}
	type caso struct {
		args []string
		ok   bool
		sale string // lo que imprime (stdout y stderr)
	}
	verificar := func(cuando string, casos []caso) {
		t.Helper()
		for _, c := range casos {
			if sale, ok := tmux(c.args...); ok != c.ok || sale != c.sale {
				t.Errorf("337b23: %s: tmux %s: tmux 3.6b sale con ok=%v e imprime %q; el falso, ok=%v y %q",
					cuando, strings.Join(c.args, " "), c.ok, c.sale, ok, sale)
			}
		}
	}
	verificar("solo con "+login, []caso{
		{[]string{"has-session", "-t", propia}, true, ""},
		{[]string{"has-session", "-t", "=" + propia}, false, "can't find session: " + propia},
		{[]string{"has-session", "-t", "=" + propia + ":"}, false, "can't find session: " + propia},
		{[]string{"list-panes", "-t", propia, "-F", "#{session_name}"}, true, login + "\n" + login},
		{[]string{"list-panes", "-t", "=" + propia, "-F", "#{session_name}"}, true, login + "\n" + login},
		{[]string{"list-panes", "-t", "=" + propia + ":", "-F", "#{session_name}"}, false, "can't find session: " + propia},
		{[]string{"select-pane", "-t", "=" + propia}, false, "can't find pane: =" + propia},
		{[]string{"select-pane", "-t", "=" + propia + ":"}, false, "can't find session: " + propia},
		{[]string{"split-window", "-d", "-t", "=" + propia, "-P", "-F", "#{session_name}", "sleep 1"}, false, "can't find pane: =" + propia},
		{[]string{"split-window", "-d", "-t", "=" + propia + ":", "-P", "-F", "#{session_name}", "sleep 1"}, false, "can't find session: " + propia},
		{[]string{"kill-session", "-t", "=" + propia}, false, "can't find session: " + propia},
		{[]string{"kill-session", "-t", "=" + propia + ":"}, false, "can't find session: " + propia},
	})
	if n := f.panes(login); n != 2 {
		t.Fatalf("337b23: ningun destino exacto toca la sesion %s: tiene %d panes", login, n)
	}
	// un nombre suelto en split-window cae en la sesion de otra tarjeta
	verificar("solo con "+login+", nombre suelto", []caso{
		{[]string{"split-window", "-d", "-t", propia, "-P", "-F", "#{session_name}", "sleep 1"}, true, login},
	})
	if n := f.panes(login); n != 3 {
		t.Fatalf("337b23: split-window -t %s (suelto) le agrega un pane a %s, como tmux: tiene %d", propia, login, n)
	}

	f.sembrar(t, login, 2)
	f.sembrar(t, propia, 2)
	verificar("con las dos sesiones", []caso{
		{[]string{"has-session", "-t", "=" + propia + ":"}, true, ""},
		{[]string{"list-panes", "-t", "=" + propia + ":", "-F", "#{session_name}"}, true, propia + "\n" + propia},
		{[]string{"select-pane", "-t", "=" + propia + ":"}, true, ""},
		{[]string{"select-pane", "-t", "=" + propia}, false, "can't find pane: =" + propia},
		{[]string{"split-window", "-d", "-t", "=" + propia + ":", "-P", "-F", "#{session_name}", "sleep 1"}, true, propia},
	})
	if a, b := f.panes(propia), f.panes(login); a != 3 || b != 2 {
		t.Fatalf("337b23: split-window -t =%s: agrega el pane en %s (3 panes, tiene %d) y no en %s (2, tiene %d)", propia, propia, a, login, b)
	}

	// la marca morir: new-session crea la sesion, sale con 0 y la sesion muere
	if err := os.WriteFile(f.morir, []byte("hoom-demo-muere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	verificar("con morir", []caso{
		{[]string{"new-session", "-d", "-s", "hoom-demo-muere", "-P", "-F", "#{session_name}", "claude"}, true, "hoom-demo-muere"},
		{[]string{"has-session", "-t", "=hoom-demo-muere:"}, false, "can't find session: hoom-demo-muere"},
	})
}

// Hallazgo 337b23 (a), CA-347 y CA-348 con la ENMIENDA 5 (n): "el pane de
// vigilancia se agrega con ese mismo destino" (`=<session>:`), y la enmienda
// 4: "si el que falla es el split-window justo despues de que esta apertura
// creo la sesion, la cierra como hasta ahora". CA-348: "cuando Open crea la
// sesion de una tarea con item, el item del arbol raiz gana una entrada en
// sesiones"; una sesion que no existe no es una sesion creada.
//
// Con la sesion de la tarjeta fix-login abierta (dos panes) y un tmux en el que
// la sesion recien creada de fix muere enseguida (la CLI de la IA sale al
// arrancar, antes de que se agregue el pane de vigilancia), Abrir sesion en fix,
// desde el Studio y desde la CLI (cockpitcmd.Open): la sesion de fix-login
// sigue igual (sus dos panes, ninguno nuevo, sin cerrarse), la apertura
// responde un error (no created true) y los items de fix y de fix-login quedan
// byte a byte iguales. Guarda: con tmux sano, abrir fix crea SU sesion con sus
// dos panes y la declara en su item.
func TestHallazgo_337b23_CA347_CA348_UnaSesionQueMuereAlCrearseNoCaeEnLaDeOtraTarjeta(t *testing.T) {
	for _, quien := range []string{"studio", "cli"} {
		t.Run(quien, func(t *testing.T) {
			ca := "337b23 (CA-347, CA-348, enmienda 5), desde " + quien
			f := nuevoTmuxFalso(t)
			f.destinosComoTmux = true
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, "fix-login")
			wtFix := atConEspacio(t, dir, "fix")
			s := newServer(t, dir)
			login := cockpitcmd.SessionName("demo", "fix-login")
			propia := cockpitcmd.SessionName("demo", "fix")
			if !strings.HasPrefix(login, propia) || login == propia {
				t.Fatalf("%s: fixture: el nombre de la sesion de fix-login (%s) empieza con el de la de fix (%s)", ca, login, propia)
			}
			f.sembrar(t, login, 2)
			antesLogin := f.estadoDe(login)
			_, itemFix := atLeerItem(t, dir, "fix")
			_, itemLogin := atLeerItem(t, dir, "fix-login")

			// la sesion de fix muere apenas se crea
			if err := os.WriteFile(f.morir, []byte(propia+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			a := abrirSesion(s, dir, "fix", quien)
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if a.panico != nil {
				t.Fatalf("%s: la apertura entra en panico: %v\ntmux:\n%s", ca, a.panico, tmux)
			}
			if i := indiceDeTmux(lineas, 0, "new-session", "-s", propia); i < 0 || !strings.Contains(lineas[i], wtFix) {
				t.Fatalf("%s: fixture: la apertura crea la sesion %s (new-session -s %s en %s), que muere enseguida:\n%s", ca, propia, propia, wtFix, tmux)
			}
			var mal []string
			if ahora := f.estadoDe(login); ahora != antesLogin {
				mal = append(mal, "el pane de vigilancia de fix cae en la sesion de otra tarjeta: "+login+
					" tenia los panes\n"+antesLogin+"y ahora tiene\n"+ahora)
			}
			if c := f.cerradas(t, login); len(c) > 0 {
				mal = append(mal, "la sesion de fix-login no se cierra: "+strings.Join(c, "; "))
			}
			if a.creo(propia) || !a.fallo() {
				mal = append(mal, "la sesion de fix no existe: la apertura responde un error, no created true: "+a.String())
			}
			if _, raw := atLeerItem(t, dir, "fix"); !bytes.Equal(raw, itemFix) {
				mal = append(mal, "el item de fix no declara una sesion que no existe:\n"+string(raw))
			}
			if _, raw := atLeerItem(t, dir, "fix-login"); !bytes.Equal(raw, itemLogin) {
				mal = append(mal, "el item de fix-login no cambia:\n"+string(raw))
			}
			if len(mal) > 0 {
				t.Fatalf("%s: con la sesion %s abierta y la de fix muerta al crearse:\n- %s\ntmux:\n%s", ca, login, strings.Join(mal, "\n- "), tmux)
			}

			// guarda: con tmux sano, fix crea SU sesion entera y la declara
			if err := os.Remove(f.morir); err != nil {
				t.Fatal(err)
			}
			desde := len(atLineas(t, f.log))
			b := abrirSesion(s, dir, "fix", quien)
			tmux = strings.Join(atLineas(t, f.log)[desde:], "\n")
			if !b.creo(propia) || f.panes(propia) != 2 || f.estadoDe(login) != antesLogin {
				t.Fatalf("%s: guarda: con tmux sano, abrir fix crea su sesion %s con dos panes (%d) y deja la de fix-login igual: %v\ntmux:\n%s",
					ca, propia, f.panes(propia), b, tmux)
			}
			if it, crudo := atLeerItem(t, dir, "fix"); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
				t.Fatalf("%s: guarda: la sesion creada de fix se declara en su item (una entrada, claude):\n%s", ca, crudo)
			}
		})
	}
}

// Hallazgo 337b23 (b), CA-350 con la ENMIENDA 5 (n): "el pane es el primero
// que lista `tmux list-panes -t '=<session>:' -F '#{pane_id}'`"; "el destino
// `=<session>:` es exacto: `=<session>` sin los dos puntos no lo es para
// list-panes, y una sesion cerrada justo antes listaria los panes de otra
// tarjeta". CA-350: "sin sesion o sin tmux responde 200 con available: false y
// la nota del contrato" (`no hay una sesion abierta para esta tarjeta`).
//
// Con las sesiones de fix-login y de fix abiertas, el primer sondeo de tmux de
// Ver terminal de fix sobre su sesion la ve viva y, antes de que siga, la
// sesion de fix se cierra. Ver terminal de fix responde 200 sin la pantalla de
// fix-login (ni su sesion ni sus panes, y sin capture-pane de un pane de
// fix-login): available false, sin text y la nota del contrato. Guarda: con la
// sesion de fix abierta otra vez, muestra SU pantalla.
func TestHallazgo_337b23_CA350_VerTerminalNoMuestraOtraTarjetaSiLaSesionSeCierraAlLeerla(t *testing.T) {
	const ca = "337b23 (CA-350, enmienda 5)"
	login := cockpitcmd.SessionName("demo", "fix-login")
	propia := cockpitcmd.SessionName("demo", "fix")
	f := nuevoTmuxFalso(t)
	f.destinosComoTmux = true
	f.ret.nombre, f.ret.donde = propia, "sondeo"
	atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	atConEspacio(t, dir, "fix-login")
	atConEspacio(t, dir, "fix")
	f.sembrar(t, login, 2)
	f.sembrar(t, propia, 2)
	panesLogin := f.idsDePanes(login)
	s := newServer(t, dir)

	ch := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		defer func() {
			if p := recover(); p != nil {
				rec = httptest.NewRecorder()
				rec.Code = -1 // no es un codigo HTTP: el handler entro en panico
				fmt.Fprintf(rec.Body, "panico: %v", p)
			}
			ch <- rec
		}()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/board/fix/terminal", nil))
	}()
	if !h3EsperarHasta(10*time.Second, func() bool { _, err := os.Stat(f.ret.esperando); return err == nil }) {
		f.ret = retencion{}
		t.Fatalf("%s: fixture: Ver terminal de fix sondea su sesion %s con tmux (has-session, list-panes o display-message):\n%s",
			ca, propia, strings.Join(atLineas(t, f.log), "\n"))
	}
	// la sesion de fix se cierra entre el sondeo y lo que sigue
	if err := os.WriteFile(filepath.Join(f.ses, propia), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.ret.seguir, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	var rec *httptest.ResponseRecorder
	select {
	case rec = <-ch:
	case <-time.After(20 * time.Second):
		t.Fatalf("%s: Ver terminal de fix no responde en 20 s:\n%s", ca, strings.Join(atLineas(t, f.log), "\n"))
	}
	lineas := atLineas(t, f.log)
	tmux := strings.Join(lineas, "\n")
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: Ver terminal responde 200 aunque la sesion se cierre mientras la lee, fue %d: %s\ntmux:\n%s", ca, rec.Code, rec.Body.String(), tmux)
	}
	var term atTerminal
	atOK(t, ca, rec, &term)

	var ajena []string
	if strings.Contains(term.Text, "["+login+"]") {
		ajena = append(ajena, "text es la pantalla de "+login+": "+term.Text)
	}
	if term.Session == login {
		ajena = append(ajena, "session es "+login)
	}
	for _, p := range panesLogin {
		if term.Pane == p {
			ajena = append(ajena, "pane es "+p+", de "+login)
		}
		for _, c := range atConVerbo(lineas, "capture-pane") {
			if flagDeTmux(c, "-t") == p {
				ajena = append(ajena, "tmux "+c+" captura un pane de "+login)
			}
		}
	}
	if len(ajena) > 0 {
		t.Fatalf("%s: la sesion de fix se cerro: Ver terminal de fix no muestra la de otra tarjeta (%s):\n- %s\nrespuesta: %+v\ntmux:\n%s",
			ca, login, strings.Join(ajena, "\n- "), term, tmux)
	}
	if term.Available || term.Text != "" || term.Note != "no hay una sesion abierta para esta tarjeta" {
		t.Fatalf("%s: sin la sesion de fix: available false, sin text y la nota del contrato: %+v\ntmux:\n%s", ca, term, tmux)
	}
	if n := f.panes(login); n != 2 {
		t.Fatalf("%s: Ver terminal es una lectura: la sesion de fix-login sigue con sus dos panes, tiene %d", ca, n)
	}

	// guarda: con la sesion de fix abierta otra vez, muestra SU pantalla
	f.sembrar(t, propia, 2)
	term = atTerminal{}
	atOK(t, ca+" (guarda)", atPedir(t, ca, s, http.MethodGet, "/api/board/fix/terminal", "", ""), &term)
	if !term.Available || term.Session != propia || !strings.Contains(term.Text, "pantalla de ["+propia+"]") {
		t.Fatalf("%s: guarda: con la sesion de fix abierta, Ver terminal de fix muestra su pantalla: %+v\ntmux:\n%s",
			ca, term, strings.Join(atLineas(t, f.log), "\n"))
	}
}
