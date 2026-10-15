// Tests de regresion de la cuarta vuelta de review de las correcciones de la
// cabina C3 (tarea fixes-cabina-c3) sobre .hoom/specs/acciones-desde-la-tarjeta.md,
// por el Studio:
//
//   - 46e17e (CA-348, enmienda 1 (k), y su caso limite de la enmienda 3): dos
//     aperturas de la MISMA tarjeta a la vez (dos POST
//     /api/board/<slug>/session de dos pestanas, o el Studio y `hoom cockpit
//     --task`) no dejan la tarjeta sin cockpit. La que llega tarde paso su
//     sondeo (has-session) cuando la sesion todavia no existia; la otra arma la
//     sesion entera; despues el new-session de la tardia recibe de tmux
//     "duplicate session: <sesion>" (exit 1, sin colgarse). La tardia responde
//     created false o un error, pero NO cierra la sesion ajena (ni mata su CLI
//     de IA): la sesion sigue viva y el item tiene UNA entrada en sesiones, la
//     de la sesion que quedo. El caso de 0fc76d (un new-session que crea y se
//     cuelga hasta el plazo: con la enmienda 3 esa sesion a medias tampoco se
//     cierra, y el reintento la completa) lo cubre
//     TestHallazgo_0fc76d_AbrirSesionAMediasSeCompletaEnElReintento, y el de un
//     new-session que vence su plazo o falla con otro error mientras la otra
//     arma la sesion, los tests de 6a6780 y d8d246 (vuelta 5).
//
// Determinista, sin depender del scheduler: el tmux falso de la vuelta 3
// (v3Tmux) retiene la llamada de la apertura tardia (v3Retencion: su primer
// sondeo de la sesion, despues de ver que no existe, o su new-session, antes
// de mirar) hasta que el test lo suelta, y el test lo suelta recien cuando la
// otra apertura termino.
//
// Fixtures de hallazgos_c3_vuelta3_test.go (v3Tmux, v3Marcas, v3Viva),
// acciones_tarjeta_test.go (atPATH, atConEspacio, atLineas, atLeerItem,
// atSesion), tablero_test.go (tbProyecto) y hallazgos_c3_test.go
// (h3EsperarHasta). tmux y el CLI de IA son falsos.
package servecmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// v4Apertura es como termino una apertura: la del Studio (POST
// /api/board/<slug>/session) o la de la CLI (cockpitcmd.Open, lo que hace
// `hoom cockpit --task <slug>` antes de adjuntarse).
type v4Apertura struct {
	quien  string
	rec    *httptest.ResponseRecorder // el Studio
	ses    cockpitcmd.Session         // la CLI
	err    error                      // la CLI
	panico any
}

func (a v4Apertura) String() string {
	if a.panico != nil {
		return fmt.Sprintf("%s: panico %v", a.quien, a.panico)
	}
	if a.rec != nil {
		return fmt.Sprintf("%s: %d %s", a.quien, a.rec.Code, strings.TrimSpace(a.rec.Body.String()))
	}
	return fmt.Sprintf("%s: %+v, error %v", a.quien, a.ses, a.err)
}

// creo dice si esta apertura creo la sesion nombre (created true).
func (a v4Apertura) creo(nombre string) bool {
	if a.panico != nil {
		return false
	}
	if a.rec != nil {
		var s atSesion
		return a.rec.Code == http.StatusOK && json.Unmarshal(a.rec.Body.Bytes(), &s) == nil && s.Created && s.Session == nombre
	}
	return a.err == nil && a.ses.Created && a.ses.Name == nombre
}

// cedio dice si la apertura que no creo la sesion termino como permite el
// contrato: created false con la sesion nombre, o un error ({"error": ...}
// JSON en el Studio).
func (a v4Apertura) cedio(nombre string) bool {
	if a.panico != nil {
		return false
	}
	if a.rec != nil {
		switch {
		case a.rec.Code == http.StatusOK:
			var s atSesion
			return json.Unmarshal(a.rec.Body.Bytes(), &s) == nil && !s.Created && s.Session == nombre
		case a.rec.Code >= 400:
			var e map[string]string
			return strings.Contains(a.rec.Header().Get("Content-Type"), "application/json") &&
				json.Unmarshal(a.rec.Body.Bytes(), &e) == nil && e["error"] != ""
		}
		return false
	}
	return a.err != nil || (!a.ses.Created && a.ses.Name == nombre)
}

// fallo dice si la apertura termino con un error: {"error": ...} JSON con un
// codigo 4xx o 5xx en el Studio, un error de cockpitcmd.Open en la CLI.
func (a v4Apertura) fallo() bool {
	if a.panico != nil {
		return false
	}
	if a.rec != nil {
		var e map[string]string
		return a.rec.Code >= 400 && strings.Contains(a.rec.Header().Get("Content-Type"), "application/json") &&
			json.Unmarshal(a.rec.Body.Bytes(), &e) == nil && e["error"] != ""
	}
	return a.err != nil
}

// v4Abrir abre la sesion de la tarjeta slug como quien: "studio" (POST
// /api/board/<slug>/session de s, con su token) o "cli" (cockpitcmd.Open sobre
// dir con las deps de la CLI). Un panico es un resultado, no la caida del test.
func v4Abrir(s *Server, dir, slug, quien string) (a v4Apertura) {
	defer func() {
		if p := recover(); p != nil {
			a.panico = p
		}
	}()
	if quien == "cli" {
		a.quien = "la CLI"
		a.ses, a.err = cockpitcmd.Open(dir, "demo", cockpitcmd.Options{Provider: "claude", Task: slug}, cockpitcmd.DefaultDeps())
		return a
	}
	a.quien = "el Studio"
	req := httptest.NewRequest(http.MethodPost, "/api/board/"+slug+"/session", strings.NewReader(`{"provider": "claude"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, s.Token())
	a.rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(a.rec, req)
	return a
}

// v4Lanzar abre la sesion en una goroutine; el canal trae como termino.
func v4Lanzar(s *Server, dir, slug, quien string) <-chan v4Apertura {
	ch := make(chan v4Apertura, 1)
	go func() { ch <- v4Abrir(s, dir, slug, quien) }()
	return ch
}

// v4Esperar espera el resultado de una apertura hasta 20 s.
func v4Esperar(t *testing.T, ca string, f *v3Tmux, ch <-chan v4Apertura, cual string) v4Apertura {
	t.Helper()
	select {
	case a := <-ch:
		return a
	case <-time.After(20 * time.Second):
		t.Fatalf("%s: la apertura %s no termino en 20 s:\n%s", ca, cual, strings.Join(atLineas(t, f.log), "\n"))
	}
	return v4Apertura{}
}

// Hallazgo 46e17e (CA-348, enmienda 1 (k), y el caso limite de la enmienda
// 3 "dos aperturas a la vez de la misma tarjeta: una crea la sesion y la otra
// recibe duplicate session y responde el error sin tocarla"). Dos aperturas de
// la MISMA tarjeta a la vez: la tardia llega a tmux primero y su llamada queda
// retenida (su primer sondeo de la sesion, has-session, ya vio que no existe;
// o su new-session todavia no miro); la otra arma la sesion entera y termina;
// recien ahi la tardia sigue y su new-session recibe "duplicate session" (exit
// 1). Al final: exactamente una de las dos respondio created true con la
// sesion de SessionName; la otra created false o un error; nadie cerro la
// sesion ya armada (el tmux falso no registro ningun cierre de ella mientras
// vivia); la sesion sigue viva; y el item tiene UNA entrada en sesiones
// (claude), que apunta a esa sesion viva. Se prueba con dos pestanas del
// Studio (retenida en el sondeo y en new-session) y con el Studio y la CLI,
// ganando cada uno.
func TestHallazgo_46e17e_DosAperturasALaVezNoDejanLaTarjetaSinCockpit(t *testing.T) {
	for _, c := range []struct {
		caso         string
		donde        string // la llamada a tmux de la tardia que se retiene (v3Retencion)
		tardia, otra string // "studio" o "cli"
	}{
		{"dos pestanas, la tardia paso has-session sin la sesion", "sondeo", "studio", "studio"},
		{"dos pestanas, el new-session de la tardia llega despues", "new-session", "studio", "studio"},
		{"la CLI arma la sesion y el Studio llega tarde", "sondeo", "studio", "cli"},
		{"el Studio arma la sesion y la CLI llega tarde", "sondeo", "cli", "studio"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			ca := "46e17e (CA-348): " + c.caso
			const slug = "a-la-vez"
			nombre := cockpitcmd.SessionName("demo", slug)
			f := v3Marcas(t)
			f.ret.nombre, f.ret.donde, f.ret.luego = nombre, c.donde, "seguir"
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			s := newServer(t, dir)

			// la tardia llega a tmux primero y su llamada queda retenida
			chTardia := v4Lanzar(s, dir, slug, c.tardia)
			if !h3EsperarHasta(10*time.Second, func() bool { _, err := os.Stat(f.ret.esperando); return err == nil }) {
				t.Fatalf("%s: fixture: la apertura tardia llego a tmux (%s) de la sesion %s y espera:\n%s", ca, c.donde, nombre, strings.Join(atLineas(t, f.log), "\n"))
			}
			// la otra arma la sesion y termina; recien ahi la tardia sigue
			otra := v4Esperar(t, ca, f, v4Lanzar(s, dir, slug, c.otra), "que no espera")
			if err := os.WriteFile(f.ret.seguir, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			tardia := v4Esperar(t, ca, f, chTardia, "tardia")
			lineas := strings.Join(atLineas(t, f.log), "\n")

			for _, a := range []v4Apertura{otra, tardia} {
				if a.panico != nil {
					t.Fatalf("%s: %v", ca, a)
				}
			}
			var cedidas []v4Apertura
			for _, a := range []v4Apertura{otra, tardia} {
				if !a.creo(nombre) {
					cedidas = append(cedidas, a)
				}
			}
			if len(cedidas) != 1 {
				t.Errorf("%s: exactamente una de las dos aperturas crea la sesion %s (created true):\n  %v\n  %v\ntmux:\n%s", ca, nombre, otra, tardia, lineas)
			}
			for _, a := range cedidas {
				if !a.cedio(nombre) {
					t.Errorf("%s: la apertura que no creo la sesion responde created false (con la sesion %s) o un error: %v\ntmux:\n%s", ca, nombre, a, lineas)
				}
			}
			if k := f.cerradas(t, nombre); len(k) > 0 {
				t.Errorf("%s: nadie cierra la sesion %s ya armada por la otra apertura (mataria su CLI de IA), y hubo:\n%s\naperturas:\n  %v\n  %v\ntmux:\n%s", ca, nombre, strings.Join(k, "\n"), otra, tardia, lineas)
			}
			if !v3Viva(f.ses, nombre) {
				t.Errorf("%s: al terminar las dos aperturas la sesion %s tiene que seguir abierta (si no, la tarjeta queda sin cockpit y su item declara una sesion muerta), y no lo esta\naperturas:\n  %v\n  %v\ntmux:\n%s", ca, nombre, otra, tardia, lineas)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
				t.Errorf("%s: el item declara UNA sesion (claude), la que quedo abierta:\n%s", ca, crudo)
			}
		})
	}
}
