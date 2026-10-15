// Tests de regresion de la cuarta vuelta de review de las correcciones de la
// cabina C3 (tarea fixes-cabina-c3) sobre .hoom/specs/acciones-desde-la-tarjeta.md,
// por el Studio:
//
//   - 46e17e (CA-348, enmienda 1 (k)): dos aperturas de la MISMA tarjeta a la
//     vez (dos POST /api/board/<slug>/session de dos pestanas, o el Studio y
//     `hoom cockpit --task`) no dejan la tarjeta sin cockpit. La que llega
//     tarde paso su has-session cuando la sesion todavia no existia; la otra
//     arma la sesion entera; despues el new-session de la tardia recibe de
//     tmux "duplicate session: <sesion>" (exit 1, sin colgarse). La tardia
//     responde created false o un error, pero NO cierra la sesion ajena (ni
//     mata su CLI de IA): la sesion sigue viva y el item tiene UNA entrada en
//     sesiones, la de la sesion que quedo. El caso de 0fc76d (un new-session
//     que crea y se cuelga hasta el plazo: esa sesion a medias SI se cierra)
//     ya lo cubre TestHallazgo_0fc76d_AbrirSesionAMediasNoQuedaYElReintentoLaArma.
//
// Determinista, sin depender del scheduler: el tmux falso retiene la llamada
// de la apertura tardia (su has-session, despues de ver que la sesion no
// existe, o su new-session, antes de mirar) hasta que el test lo suelta, y el
// test lo suelta recien cuando la otra apertura termino.
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
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// v4Orquesta son los archivos del tmux falso de 46e17e.
type v4Orquesta struct {
	log, ses, pids, colgar string // los de v3Marcas
	nombre                 string // la sesion de la tarjeta
	donde                  string // has-session o new-session: la llamada que se retiene
	unaVez                 string // existe desde que una llamada fue retenida
	esperando              string // existe desde que la llamada retenida espera
	seguir                 string // el test lo crea para soltarla
	matadas                string // cada kill-session que cerro la sesion viva
}

// v4Tmux es v3Tmux con una llamada retenida: la PRIMERA invocacion de
// o.donde sobre la sesion o.nombre crea o.esperando y espera hasta que exista
// o.seguir (o unos 20 s). Con donde = has-session mira si la sesion existe
// ANTES de esperar y responde eso despues: el has-session que paso cuando la
// sesion todavia no existia. Con donde = new-session espera ANTES de mirar y
// despues sigue como v3Tmux: crea la sesion, o si ya existe responde
// "duplicate session: <nombre>" con exit 1 sin crear nada. Ademas anota en
// o.matadas cada kill-session que resuelve a o.nombre (exacto con =, por
// prefijo sin =) mientras la sesion esta viva. Solo builtins de sh y
// /bin/sleep (sleep con fraccion: GNU y macOS).
func v4Tmux(o v4Orquesta) string {
	retener := strings.NewReplacer("@LOG@", o.log, "@SES@", o.ses, "@PIDS@", o.pids, "@NOMBRE@", o.nombre,
		"@DONDE@", o.donde, "@UNAVEZ@", o.unaVez, "@ESPERANDO@", o.esperando, "@SEGUIR@", o.seguir,
		"@MATADAS@", o.matadas).Replace(`v4verbo="$1"; v4t=""; v4s=""; v4prev=""
for v4a in "$@"; do
  case "$v4prev" in
  -t) v4t="$v4a" ;;
  -s) v4s="$v4a" ;;
  esac
  v4prev="$v4a"
done
v4d="${v4t%%:*}"; v4d="${v4d%%.*}"; v4d="${v4d#=}"
v4esperar() {
  : > '@ESPERANDO@'
  v4n=0
  while [ ! -f '@SEGUIR@' ] && [ "$v4n" -lt 400 ]; do
    /bin/sleep 0.05
    v4n=$((v4n+1))
  done
}
v4unavez() {
  ( set -C; : > '@UNAVEZ@' ) 2>/dev/null
}
case "$v4verbo" in
has-session)
  if [ '@DONDE@' = has-session ] && [ "$v4d" = '@NOMBRE@' ] && v4unavez; then
    printf '%s\n' "$*" >> '@LOG@'
    printf '%s\n' "$$" >> '@PIDS@'
    v4habia=no
    [ -s '@SES@'/'@NOMBRE@' ] && v4habia=si
    v4esperar
    [ "$v4habia" = si ] && exit 0
    printf "can't find session: %s\n" "$v4t" >&2
    exit 1
  fi ;;
new-session)
  if [ '@DONDE@' = new-session ] && [ "$v4s" = '@NOMBRE@' ] && v4unavez; then
    printf '%s\n' "$$" >> '@PIDS@'
    v4esperar
  fi ;;
kill-session)
  v4es=no
  case "$v4t" in
  =*) [ "$v4d" = '@NOMBRE@' ] && v4es=si ;;
  *)
    if [ -n "$v4d" ]; then
      case '@NOMBRE@' in "$v4d"*) v4es=si ;; esac
    fi ;;
  esac
  if [ "$v4es" = si ] && [ -s '@SES@'/'@NOMBRE@' ]; then
    printf '%s\n' "$*" >> '@MATADAS@'
  fi ;;
esac
`)
	return retener + v3Tmux(o.log, o.ses, o.pids, o.colgar)
}

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

// Hallazgo 46e17e (CA-348, enmienda 1 (k): cada llamada a tmux del Studio
// tiene un plazo, y la sesion a medias se cierra; pero solo la propia). Dos
// aperturas de la MISMA tarjeta a la vez: la tardia llega a tmux primero y su
// llamada queda retenida (su has-session ya vio que la sesion no existe, o su
// new-session todavia no miro); la otra arma la sesion entera y termina; recien
// ahi la tardia sigue y su new-session recibe "duplicate session" (exit 1).
// Al final: exactamente una de las dos respondio created true con la sesion de
// SessionName; la otra created false o un error; nadie cerro la sesion ya
// armada (ningun kill-session que la resuelva mientras vive); la sesion sigue
// viva; y el item tiene UNA entrada en sesiones (claude), que apunta a esa
// sesion viva. Se prueba con dos pestanas del Studio (retenida en has-session
// y en new-session) y con el Studio y la CLI, ganando cada uno.
func TestHallazgo_46e17e_DosAperturasALaVezNoDejanLaTarjetaSinCockpit(t *testing.T) {
	for _, c := range []struct {
		caso         string
		donde        string // la llamada a tmux de la tardia que se retiene
		tardia, otra string // "studio" o "cli"
	}{
		{"dos pestanas, la tardia paso has-session sin la sesion", "has-session", "studio", "studio"},
		{"dos pestanas, el new-session de la tardia llega despues", "new-session", "studio", "studio"},
		{"la CLI arma la sesion y el Studio llega tarde", "has-session", "studio", "cli"},
		{"el Studio arma la sesion y la CLI llega tarde", "has-session", "cli", "studio"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			ca := "46e17e (CA-348): " + c.caso
			const slug = "a-la-vez"
			log, ses, pids, colgar := v3Marcas(t)
			marcas := filepath.Dir(log)
			o := v4Orquesta{log: log, ses: ses, pids: pids, colgar: colgar,
				nombre: cockpitcmd.SessionName("demo", slug), donde: c.donde,
				unaVez: filepath.Join(marcas, "una-vez"), esperando: filepath.Join(marcas, "esperando"),
				seguir: filepath.Join(marcas, "seguir"), matadas: filepath.Join(marcas, "matadas")}
			atPATH(t, map[string]string{"tmux": v4Tmux(o), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			atConEspacio(t, dir, slug)
			s := newServer(t, dir)
			// si el test corta antes, la llamada retenida se suelta (y despues
			// v3Marcas mata lo que quede)
			t.Cleanup(func() { os.WriteFile(o.seguir, nil, 0o644) })

			abrir := func(quien string) (a v4Apertura) {
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
			lanzar := func(quien string) <-chan v4Apertura {
				ch := make(chan v4Apertura, 1)
				go func() { ch <- abrir(quien) }()
				return ch
			}
			esperar := func(ch <-chan v4Apertura, cual string) v4Apertura {
				t.Helper()
				select {
				case a := <-ch:
					return a
				case <-time.After(20 * time.Second):
					t.Fatalf("%s: la apertura %s no termino en 20 s:\n%s", ca, cual, strings.Join(atLineas(t, o.log), "\n"))
				}
				return v4Apertura{}
			}

			// la tardia llega a tmux primero y su llamada queda retenida
			chTardia := lanzar(c.tardia)
			if !h3EsperarHasta(10*time.Second, func() bool { _, err := os.Stat(o.esperando); return err == nil }) {
				t.Fatalf("%s: fixture: la apertura tardia llego a tmux %s de la sesion %s y espera:\n%s", ca, c.donde, o.nombre, strings.Join(atLineas(t, o.log), "\n"))
			}
			// la otra arma la sesion y termina; recien ahi la tardia sigue
			otra := esperar(lanzar(c.otra), "que no espera")
			if err := os.WriteFile(o.seguir, nil, 0o644); err != nil {
				t.Fatal(err)
			}
			tardia := esperar(chTardia, "tardia")
			lineas := strings.Join(atLineas(t, o.log), "\n")

			for _, a := range []v4Apertura{otra, tardia} {
				if a.panico != nil {
					t.Fatalf("%s: %v", ca, a)
				}
			}
			var cedidas []v4Apertura
			for _, a := range []v4Apertura{otra, tardia} {
				if !a.creo(o.nombre) {
					cedidas = append(cedidas, a)
				}
			}
			if len(cedidas) != 1 {
				t.Errorf("%s: exactamente una de las dos aperturas crea la sesion %s (created true):\n  %v\n  %v\ntmux:\n%s", ca, o.nombre, otra, tardia, lineas)
			}
			for _, a := range cedidas {
				if !a.cedio(o.nombre) {
					t.Errorf("%s: la apertura que no creo la sesion responde created false (con la sesion %s) o un error: %v\ntmux:\n%s", ca, o.nombre, a, lineas)
				}
			}
			if raw, _ := os.ReadFile(o.matadas); strings.TrimSpace(string(raw)) != "" {
				t.Errorf("%s: nadie cierra la sesion %s ya armada por la otra apertura (mataria su CLI de IA), y hubo:\n%s\naperturas:\n  %v\n  %v\ntmux:\n%s", ca, o.nombre, raw, otra, tardia, lineas)
			}
			if !v3Viva(o.ses, o.nombre) {
				t.Errorf("%s: al terminar las dos aperturas la sesion %s tiene que seguir abierta (si no, la tarjeta queda sin cockpit y su item declara una sesion muerta), y no lo esta\naperturas:\n  %v\n  %v\ntmux:\n%s", ca, o.nombre, otra, tardia, lineas)
			}
			if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
				t.Errorf("%s: el item declara UNA sesion (claude), la que quedo abierta:\n%s", ca, crudo)
			}
		})
	}
}
