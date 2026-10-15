// Tests de regresion de la tercera vuelta de review de las correcciones de la
// cabina C3 (tarea fixes-cabina-c3) sobre .hoom/specs/acciones-desde-la-tarjeta.md,
// por el Studio:
//
//   - 1b1fc7 (CA-350, enmienda 1 (f)): el Studio servido en un loopback que
//     no es 127.0.0.1 ni localhost a secas (localhost.:P, 127.0.0.2:P) atiende
//     al navegador que abre la URL impresa, que manda ese mismo Host. Un Host
//     ajeno sigue siendo 421.
//   - 0fc76d (CA-347 y CA-348, ENMIENDA 4): un tmux cuyo new-session CREA la
//     sesion y despues se cuelga hasta el plazo (o falla) deja la sesion a
//     medias (un solo pane, el de la IA). La apertura responde el error, que
//     nombra la sesion y dice que se cierra con `tmux kill-session -t
//     =<session>`, y no la toca: ni la cierra ni la completa. El reintento, con
//     tmux sano, la ve existente: created false, sin new-session ni
//     split-window, y no registra nada en el item. (En la vuelta 3 el contrato
//     era el de la enmienda 1 (k): cerrar la sesion a medias y rearmarla; la
//     enmienda 3 lo cambio a completarla en el reintento, y la enmienda 4, que
//     reemplazo a la 3, a dejarla como esta. Las dos las decidio Henry.)
//   - fa374d (CA-348 y CA-350): tmux resuelve `-t <nombre>` tambien por
//     PREFIJO (solo `-t =<nombre>` es exacto). Con la sesion de la tarjeta
//     fix-login abierta, Abrir sesion en la tarjeta fix crea SU sesion, y Ver
//     terminal de fix no muestra la pantalla de fix-login.
//
// El tmux falso con estado (tmuxFalso, nuevoTmuxFalso) y las aperturas
// (apertura, abrirSesion, lanzarApertura, esperarApertura) estan en
// tmuxfalso_test.go. Fixtures de
// acciones_tarjeta_test.go (atPATH, atConEspacio, atPedir, atOK, atLineas,
// atConVerbo, atLeerItem, atSesion, atTerminal), tablero_test.go (tbProyecto)
// y hallazgos_c3_test.go (h3Eco, h3ConHost, h3PIDs, h3VivoPID,
// h3EsperarHasta). tmux y el CLI de IA son falsos.
package servecmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// ---------------------------------------------------------------------------
// 1b1fc7: el Host del propio addr loopback llega al Studio

// Hallazgo 1b1fc7 (CA-350, enmienda 1 (f): el Studio servido en loopback solo
// atiende pedidos cuyo Host es ese loopback). Con addr "localhost.:P" el
// navegador que abre la URL impresa manda Host "localhost.:P" (o en
// mayusculas), y con addr "127.0.0.2:P" manda "127.0.0.2:P": los dos llegan al
// handler, igual que 127.0.0.1:P, localhost:P y [::1]:P. Un Host ajeno, vacio
// o con otro puerto sigue siendo 421 y no llega al handler.
func TestHallazgo_1b1fc7_SoloLocalAtiendeElHostDeSuPropioAddr(t *testing.T) {
	const p = "5170"
	for _, c := range []struct {
		addr    string
		propios []string // el Host que manda el navegador que abre la URL impresa
	}{
		{"localhost.:" + p, []string{"localhost.:" + p, "LOCALHOST.:" + p}},
		{"127.0.0.2:" + p, []string{"127.0.0.2:" + p}},
	} {
		llegan := append(append([]string{}, c.propios...), "127.0.0.1:"+p, "localhost:"+p, "[::1]:"+p)
		for _, host := range llegan {
			h := &h3Eco{}
			rec := httptest.NewRecorder()
			SoloLocal(h, c.addr).ServeHTTP(rec, h3ConHost("/api/board", host))
			if h.llamadas() != 1 || rec.Code != http.StatusTeapot {
				t.Errorf("1b1fc7 (CA-350): con addr %s, un pedido con Host %q llega al Studio: llamadas %d, codigo %d", c.addr, host, h.llamadas(), rec.Code)
			}
		}
		otroPuerto := strings.TrimSuffix(c.propios[0], p) + "9999"
		for _, host := range []string{"atacante.example:" + p, "rebind.atacante.example:" + p, otroPuerto, ""} {
			h := &h3Eco{}
			rec := httptest.NewRecorder()
			SoloLocal(h, c.addr).ServeHTTP(rec, h3ConHost("/api/board", host))
			if rec.Code != http.StatusMisdirectedRequest || h.llamadas() != 0 {
				t.Errorf("1b1fc7 (CA-350): con addr %s, un pedido con Host %q es 421 y no llega al Studio: codigo %d, llamadas %d", c.addr, host, rec.Code, h.llamadas())
			}
		}
	}
}

// Hallazgo 1b1fc7 (CA-350) de punta a punta: el Studio real envuelto en
// SoloLocal con addr "localhost.:P". GET /api/status con el Host que manda el
// navegador ("localhost.:P") responde 200, y con un Host ajeno 421.
func TestHallazgo_1b1fc7_ElStudioEnLocalhostConPuntoAtiendeASuNavegador(t *testing.T) {
	const addr = "localhost.:5170"
	dir := tbProyecto(t)
	h := SoloLocal(newServer(t, dir).Handler(), addr)

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, h3ConHost("/api/status", addr))
	if rec.Code != http.StatusOK {
		t.Errorf("1b1fc7 (CA-350): servido en %s, GET /api/status con Host %q (el de la URL impresa) responde 200, fue %d: %s", addr, addr, rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, h3ConHost("/api/status", "atacante.example:5170"))
	if rec.Code != http.StatusMisdirectedRequest {
		t.Errorf("1b1fc7 (CA-350): servido en %s, GET /api/status con Host ajeno es 421, fue %d", addr, rec.Code)
	}
}

// ---------------------------------------------------------------------------
// 0fc76d: la sesion a medias queda como esta (enmienda 4)

// Hallazgo 0fc76d (CA-347 y CA-348). El contrato es el de la ENMIENDA 4 de la
// spec: "si tmux new-session falla o vence su plazo, Open devuelve el error y
// no toca la sesion: ni la cierra ni la completa"; "el error nombra la sesion
// y dice que, si quedo a medias (sin el pane de vigilancia), se cierra con
// `tmux kill-session -t =<session>` antes de reintentar"; "una sesion a medias
// cuenta como existente: el reintento responde created: false y no registra
// nada". Y su caso limite: "Abrir sesion con un tmux new-session que crea la
// sesion y vence su plazo: la apertura responde el error, que dice como cerrar
// la sesion a mano, y la sesion queda a medias, con un solo pane; el reintento
// la ve existente, responde created: false y no registra nada".
//
// Este test REEMPLAZA a TestHallazgo_0fc76d_AbrirSesionAMediasSeCompletaEnElReintento,
// escrito para la enmienda 3 (el reintento completaba la sesion a medias:
// split-window, created true y una entrada en el item). La enmienda 4
// reemplazo a la 3 por decision de Henry: reescribir lo que exigia completar
// no es debilitar el test, es el contrato aprobado nuevo.
//
// Con un tmux cuyo new-session CREA la sesion (un pane, el de la IA) y despues
// se cuelga hasta el plazo, o sale con un error, la apertura responde un error
// en menos de 8 s (el plazo del Studio es 5 s) que contiene `tmux kill-session
// -t =<sesion>` con el nombre exacto de la sesion; no hay kill-session,
// kill-pane ni split-window, y la sesion sigue viva con un solo pane; el item
// no declara nada, y el tmux colgado no queda vivo. Despues, con tmux sano, el
// reintento responde created false con esa sesion (no un error), sin
// new-session ni split-window ni ningun cierre; la sesion sigue con un solo
// pane y el item sigue sin entradas en sesiones. Desde el Studio (plazo y
// falla) y desde la CLI (falla: la CLI no tiene plazo).
func TestHallazgo_0fc76d_AbrirSesionAMediasQuedaComoEstaYElErrorDiceComoCerrarla(t *testing.T) {
	for _, c := range []struct {
		caso  string
		quien string // "studio" o "cli"
		corte string // "colgar": crea la sesion y se cuelga hasta el plazo; "fallar": la crea y sale con 1
	}{
		{"el new-session del Studio crea la sesion y vence su plazo", "studio", "colgar"},
		{"el new-session del Studio crea la sesion y falla", "studio", "fallar"},
		{"el new-session de la CLI crea la sesion y falla", "cli", "fallar"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			ca := "0fc76d (CA-347, CA-348, enmienda 4): " + c.caso
			const slug = "a-medias"
			nombre := cockpitcmd.SessionName("demo", slug)
			f := nuevoTmuxFalso(t)
			marca := f.colgar
			if c.corte == "fallar" {
				marca = f.crearYFallar
			}
			if err := os.WriteFile(marca, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			s := newServer(t, dir)

			// primer intento: new-session crea la sesion y se cuelga (o falla)
			var a apertura
			select {
			case a = <-lanzarApertura(s, dir, slug, c.quien):
			case <-time.After(8 * time.Second):
				t.Fatalf("%s: Abrir sesion con new-session cortado se quedo colgado mas de 8 s (el plazo es 5 s):\n%s", ca, strings.Join(atLineas(t, f.log), "\n"))
			}
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if a.panico != nil {
				t.Fatalf("%s: %v", ca, a)
			}
			if indiceDeTmux(lineas, 0, "new-session", "-s", nombre) < 0 {
				t.Fatalf("%s: fixture: Abrir sesion llego a tmux new-session -s %s:\n%s", ca, nombre, tmux)
			}
			if c.corte == "colgar" && len(h3PIDs(t, f.pids)) == 0 {
				t.Fatalf("%s: fixture: new-session -s %s creo la sesion y se colgo:\n%s", ca, nombre, tmux)
			}
			if !a.fallo() {
				t.Fatalf("%s: con new-session cortado, Abrir sesion responde un error: %v\ntmux:\n%s", ca, a, tmux)
			}
			aviso := "tmux kill-session -t =" + nombre
			if !contieneElAviso(a.mensaje(), aviso) {
				t.Errorf("%s: el error nombra la sesion y dice que, si quedo a medias, se cierra con `%s` antes de reintentar: %q", ca, aviso, a.mensaje())
			}

			// no se toca la sesion: ni se cierra ni se completa
			if k := f.cierres(t, nombre); len(k) > 0 {
				t.Errorf("%s: despues de un new-session cortado no se cierra la sesion (pudo armarla otra apertura), y hubo:\n%s\ntmux:\n%s", ca, strings.Join(k, "\n"), tmux)
			}
			if v := atConVerbo(lineas, "split-window"); len(v) > 0 {
				t.Errorf("%s: despues de un new-session cortado no se completa la sesion (sin split-window): %q", ca, v)
			}
			if n := f.panes(nombre); n != 1 {
				t.Errorf("%s: la sesion %s queda a medias: viva con un solo pane (el de la IA), y tiene %d\ntmux:\n%s", ca, nombre, n, tmux)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 0 {
				t.Errorf("%s: una apertura que respondio un error no declara la sesion en el item:\n%s", ca, crudo)
			}
			for _, pid := range h3PIDs(t, f.pids) {
				if !h3EsperarHasta(3*time.Second, func() bool { return !h3VivoPID(pid) }) {
					t.Errorf("%s: el tmux colgado (pid %d) sigue vivo despues de la respuesta", ca, pid)
				}
			}

			// el reintento, con tmux sano, la ve existente
			if err := os.Remove(marca); err != nil {
				t.Fatal(err)
			}
			desde := len(lineas)
			r := esperarApertura(t, ca, f, lanzarApertura(s, dir, slug, c.quien), "del reintento")
			despues := atLineas(t, f.log)[desde:]
			tmux = strings.Join(despues, "\n")
			if r.panico != nil {
				t.Fatalf("%s: el reintento: %v", ca, r)
			}
			if r.fallo() || !r.cedio(nombre) {
				t.Errorf("%s: el reintento ve la sesion %s a medias existente: responde created false con esa sesion, no un error: %v\ntmux:\n%s", ca, nombre, r, tmux)
			}
			for _, verbo := range []string{"new-session", "split-window"} {
				if v := atConVerbo(despues, verbo); len(v) > 0 {
					t.Errorf("%s: el reintento no crea ni completa la sesion a medias (sin %s): %q", ca, verbo, v)
				}
			}
			if k := f.cierres(t, nombre); len(k) > 0 {
				t.Errorf("%s: el reintento tampoco cierra la sesion: %q", ca, k)
			}
			if n := f.panes(nombre); n != 1 {
				t.Errorf("%s: despues del reintento la sesion %s sigue a medias, con un solo pane, y tiene %d:\n%s", ca, nombre, n, tmux)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 0 {
				t.Errorf("%s: el reintento no registra nada: el item sigue sin entradas en sesiones:\n%s", ca, crudo)
			}
		})
	}
}

// contieneElAviso dice si msg trae aviso (`tmux kill-session -t =<sesion>`)
// con el nombre exacto: lo que le sigue no puede continuar el nombre de una
// sesion (letra, digito, - o _), para que el de otra sesion cuyo nombre empieza
// igual no cuente.
func contieneElAviso(msg, aviso string) bool {
	for i := 0; ; {
		j := strings.Index(msg[i:], aviso)
		if j < 0 {
			return false
		}
		fin := i + j + len(aviso)
		if fin == len(msg) {
			return true
		}
		switch c := msg[fin]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
		default:
			return true
		}
		i = fin
	}
}

// ---------------------------------------------------------------------------
// fa374d: la sesion de otra tarjeta cuyo nombre empieza igual

// Hallazgo fa374d (CA-348): con la sesion de la tarjeta fix-login abierta
// (cuyo nombre empieza con el de la sesion de fix), Abrir sesion en la tarjeta
// fix crea SU sesion: created true, el nombre de SessionName, new-session
// para ella en su espacio de trabajo y una entrada en su item; la de fix-login
// sigue abierta y no se cierra. Guarda: abrir fix otra vez la encuentra
// (created false, sin otro new-session).
func TestHallazgo_fa374d_AbrirSesionNoConfundeLaSesionDeOtraTarjeta(t *testing.T) {
	const ca = "fa374d (CA-348)"
	f := nuevoTmuxFalso(t)
	atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	atConEspacio(t, dir, "fix-login")
	wtFix := atConEspacio(t, dir, "fix")
	s := newServer(t, dir)
	pedir := func(slug string) atSesion {
		t.Helper()
		var got atSesion
		atOK(t, ca+": abrir "+slug, atPedir(t, ca, s, http.MethodPost, "/api/board/"+slug+"/session", s.Token(), `{"provider": "claude"}`), &got)
		return got
	}

	// la sesion de fix-login ya esta abierta
	login := pedir("fix-login")
	propia := cockpitcmd.SessionName("demo", "fix")
	if !login.Created || !f.viva(login.Session) || !strings.HasPrefix(login.Session, propia) || login.Session == propia {
		t.Fatalf("%s: fixture: la sesion de fix-login (%+v) esta abierta y su nombre empieza con el de la de fix (%s)", ca, login, propia)
	}
	desde := len(atLineas(t, f.log))

	fix := pedir("fix")
	despues := atLineas(t, f.log)[desde:]
	if fix.Session != propia || !fix.Created {
		t.Fatalf("%s: con la sesion de fix-login abierta, la tarjeta fix crea SU sesion %s (created true): %+v\n%s", ca, propia, fix, strings.Join(despues, "\n"))
	}
	i := indiceDeTmux(despues, 0, "new-session", "-s", propia)
	if i < 0 || !strings.Contains(despues[i], wtFix) {
		t.Fatalf("%s: new-session -s %s en el espacio de trabajo de fix (%s):\n%s", ca, propia, wtFix, strings.Join(despues, "\n"))
	}
	if !f.viva(propia) || !f.viva(login.Session) {
		t.Fatalf("%s: quedan abiertas las dos sesiones, la de fix (%v) y la de fix-login (%v):\n%s", ca, f.viva(propia), f.viva(login.Session), strings.Join(despues, "\n"))
	}
	if k := atConVerbo(despues, "kill-session"); len(k) > 0 {
		t.Fatalf("%s: abrir la sesion de fix no cierra ninguna: %q", ca, k)
	}
	if it, crudo := atLeerItem(t, dir, "fix"); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
		t.Fatalf("%s: la sesion creada de fix se declara en SU item (una entrada, claude):\n%s", ca, crudo)
	}
	if it, crudo := atLeerItem(t, dir, "fix-login"); len(it.Sesiones) != 1 {
		t.Fatalf("%s: el item de fix-login sigue con su unica entrada:\n%s", ca, crudo)
	}

	// guarda: la segunda vez la encuentra
	desde = len(atLineas(t, f.log))
	if otra := pedir("fix"); otra.Created || otra.Session != propia {
		t.Fatalf("%s: guarda: abrir fix de nuevo encuentra su sesion (created false): %+v", ca, otra)
	}
	if n := atConVerbo(atLineas(t, f.log)[desde:], "new-session"); len(n) > 0 {
		t.Fatalf("%s: guarda: con su sesion abierta no se crea otra: %q", ca, n)
	}
}

// Hallazgo fa374d (CA-350): con solo la sesion de fix-login abierta, Ver
// terminal de la tarjeta fix responde 200 con available false, sin text y la
// nota del contrato: nunca la pantalla de la sesion de otra tarjeta cuyo
// nombre empieza igual. Guarda: Ver terminal de fix-login muestra su pantalla.
func TestHallazgo_fa374d_VerTerminalNoMuestraLaSesionDeOtraTarjeta(t *testing.T) {
	const ca = "fa374d (CA-350)"
	f := nuevoTmuxFalso(t)
	atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	atConEspacio(t, dir, "fix-login")
	atConEspacio(t, dir, "fix")
	login := cockpitcmd.SessionName("demo", "fix-login")
	f.sembrar(t, login, 2) // la sesion de fix-login existe
	s := newServer(t, dir)

	var term atTerminal
	atOK(t, ca, atPedir(t, ca, s, http.MethodGet, "/api/board/fix/terminal", "", ""), &term)
	if term.Available || term.Text != "" || term.Note != "no hay una sesion abierta para esta tarjeta" {
		t.Fatalf("%s: fix no tiene sesion: available false, sin text y la nota del contrato, no la pantalla de %s: %+v\n%s", ca, login, term, strings.Join(atLineas(t, f.log), "\n"))
	}

	// guarda: el tmux falso sirve la pantalla de la sesion que si existe
	term = atTerminal{}
	atOK(t, ca+" (guarda)", atPedir(t, ca, s, http.MethodGet, "/api/board/fix-login/terminal", "", ""), &term)
	if !term.Available || term.Session != login || !strings.Contains(term.Text, "pantalla de otra tarjeta") {
		t.Fatalf("%s: guarda: Ver terminal de fix-login muestra su pantalla: %+v", ca, term)
	}
}
