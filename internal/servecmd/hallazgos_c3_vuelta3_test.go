// Tests de regresion de la tercera vuelta de review de las correcciones de la
// cabina C3 (tarea fixes-cabina-c3) sobre .hoom/specs/acciones-desde-la-tarjeta.md,
// por el Studio:
//
//   - 1b1fc7 (CA-350, enmienda 1 (f)): el Studio servido en un loopback que
//     no es 127.0.0.1 ni localhost a secas (localhost.:P, 127.0.0.2:P) atiende
//     al navegador que abre la URL impresa, que manda ese mismo Host. Un Host
//     ajeno sigue siendo 421.
//   - 0fc76d (CA-348, enmienda 1 (k)): un tmux cuyo new-session CREA la sesion
//     y despues se cuelga no deja una sesion a medias: el plazo corta el
//     pedido con un error y la sesion se cierra (kill-session -t =<sesion>).
//     El reintento, con tmux sano, la arma entera (new-session y split-window,
//     created true) y la declara en el item.
//   - fa374d (CA-348 y CA-350): tmux resuelve `-t <nombre>` tambien por
//     PREFIJO (solo `-t =<nombre>` es exacto). Con la sesion de la tarjeta
//     fix-login abierta, Abrir sesion en la tarjeta fix crea SU sesion, y Ver
//     terminal de fix no muestra la pantalla de fix-login.
//
// Fixtures de acciones_tarjeta_test.go (atPATH, atConEspacio, atPedir, atOK,
// atLineas, atConVerbo, atLeerItem, atSesion, atTerminal), tablero_test.go
// (tbProyecto, tbErrorJSON) y hallazgos_c3_test.go (h3Eco, h3ConHost, h3PIDs,
// h3VivoPID, h3EsperarHasta). tmux y el CLI de IA son falsos.
package servecmd

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
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
// El tmux falso con sesiones de verdad (para 0fc76d y fa374d)

// v3Tmux es un tmux falso con estado y la resolucion de destinos de tmux:
// cada sesion es un archivo en ses, viva mientras no este vacio. `-t =N` es
// exacto; `-t N` busca N exacta y, si no esta, la UNICA sesion cuyo nombre
// empieza con N (lo que hace tmux con un destino de sesion). has-session,
// list-panes y kill-session resuelven asi su -t; new-session -s N crea la
// sesion (y se niega si N ya existe); kill-session la vacia; capture-pane
// imprime una pantalla fija. Con el archivo colgar presente, new-session crea
// la sesion, anota su PID en pids y se cuelga. Anota cada invocacion (sus
// argumentos, una linea) en log. Solo usa builtins de sh y /bin/sleep.
func v3Tmux(log, ses, pids, colgar string) string {
	return strings.NewReplacer("@LOG@", log, "@SES@", ses, "@PIDS@", pids, "@COLGAR@", colgar).Replace(`printf '%s\n' "$*" >> '@LOG@'
verbo="$1"
[ $# -gt 0 ] && shift
t=""; s=""
while [ $# -gt 0 ]; do
  case "$1" in
  -t) [ $# -ge 2 ] && { t="$2"; shift; } ;;
  -s) [ $# -ge 2 ] && { s="$2"; shift; } ;;
  esac
  shift
done
hallada=""
buscar() {
  d="${1%%:*}"
  d="${d%%.*}"
  case "$d" in
  =*)
    d="${d#=}"
    [ -n "$d" ] && [ -s '@SES@'/"$d" ] && { hallada="$d"; return 0; }
    return 1 ;;
  esac
  [ -n "$d" ] || return 1
  [ -s '@SES@'/"$d" ] && { hallada="$d"; return 0; }
  n=0; unica=""
  for f in '@SES@'/"$d"*; do
    [ -s "$f" ] || continue
    unica="${f##*/}"; n=$((n+1))
  done
  [ "$n" -eq 1 ] && { hallada="$unica"; return 0; }
  return 1
}
case "$verbo" in
has-session)
  buscar "$t" && exit 0
  printf "can't find session: %s\n" "$t" >&2; exit 1 ;;
new-session)
  [ -n "$s" ] || { printf 'falta -s\n' >&2; exit 1; }
  if [ -s '@SES@'/"$s" ]; then printf 'duplicate session: %s\n' "$s" >&2; exit 1; fi
  printf 'viva\n' > '@SES@'/"$s"
  if [ -f '@COLGAR@' ]; then
    printf '%s\n' "$$" >> '@PIDS@'
    exec /bin/sleep 20
  fi
  exit 0 ;;
kill-session)
  buscar "$t" || { printf "can't find session: %s\n" "$t" >&2; exit 1; }
  : > '@SES@'/"$hallada"
  exit 0 ;;
list-panes)
  buscar "$t" || { printf "can't find session: %s\n" "$t" >&2; exit 1; }
  printf '%%3\n%%4\n'
  exit 0 ;;
capture-pane)
  printf 'pantalla de %s\n' 'otra tarjeta'
  exit 0 ;;
esac
exit 0
`)
}

// v3Marcas arma los archivos del tmux falso: log, la carpeta de sesiones, el
// archivo de pids y la marca colgar (que no existe hasta que se cree). Al
// terminar el test mata lo que el tmux falso haya dejado colgado.
func v3Marcas(t *testing.T) (log, ses, pids, colgar string) {
	t.Helper()
	marcas := t.TempDir()
	log, ses, pids, colgar = filepath.Join(marcas, "tmux.log"), filepath.Join(marcas, "sesiones"), filepath.Join(marcas, "pids"), filepath.Join(marcas, "colgar")
	if err := os.Mkdir(ses, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, pid := range h3PIDs(t, pids) {
			if h3VivoPID(pid) {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	return log, ses, pids, colgar
}

// v3Viva dice si el tmux falso tiene viva la sesion nombre (exacta).
func v3Viva(ses, nombre string) bool {
	raw, err := os.ReadFile(filepath.Join(ses, nombre))
	return err == nil && len(raw) > 0
}

// v3Flag devuelve el valor de flag en una linea del log del tmux falso.
func v3Flag(linea, flag string) string {
	f := strings.Fields(linea)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == flag {
			return f[i+1]
		}
	}
	return ""
}

// v3Indice es la posicion de la primera linea de lineas, desde la posicion
// desde, con ese verbo y cuyo flag vale valor (-1 si no hay).
func v3Indice(lineas []string, desde int, verbo, flag, valor string) int {
	for i := desde; i < len(lineas); i++ {
		if f := strings.Fields(lineas[i]); len(f) > 0 && f[0] == verbo && v3Flag(lineas[i], flag) == valor {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// 0fc76d: una sesion a medias no queda

// Hallazgo 0fc76d (CA-348, enmienda 1 (k): cada llamada a tmux del Studio
// tiene un plazo). Con un tmux cuyo new-session CREA la sesion y despues se
// cuelga, POST /api/board/<slug>/session responde un error en menos de 8 s (el
// plazo es 5 s) y, al terminar, la sesion a medias NO queda: se cerro con
// `kill-session -t =<sesion>` despues del new-session, y el item no declara
// nada. Despues, con tmux sano, un segundo POST arma la sesion entera:
// new-session y split-window, created true, y una entrada en sesiones.
func TestHallazgo_0fc76d_AbrirSesionAMediasNoQuedaYElReintentoLaArma(t *testing.T) {
	const ca = "0fc76d (CA-348)"
	log, ses, pids, colgar := v3Marcas(t)
	if err := os.WriteFile(colgar, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	atPATH(t, map[string]string{"tmux": v3Tmux(log, ses, pids, colgar), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	const slug = "a-medias"
	wt := atConEspacio(t, dir, slug)
	s := newServer(t, dir)
	ruta := "/api/board/" + slug + "/session"

	// primer intento: new-session crea la sesion y se cuelga
	type respuesta struct {
		rec *httptest.ResponseRecorder
		d   time.Duration
	}
	llego := make(chan respuesta, 1)
	go func() {
		inicio := time.Now()
		rec := atPedir(t, ca, s, http.MethodPost, ruta, s.Token(), `{"provider": "claude"}`)
		llego <- respuesta{rec, time.Since(inicio)}
	}()
	var r respuesta
	select {
	case r = <-llego:
	case <-time.After(8 * time.Second):
		t.Fatalf("%s: Abrir sesion con tmux colgado en new-session se quedo colgado mas de 8 s (el plazo es 5 s)", ca)
	}
	if r.rec.Code < 400 {
		t.Fatalf("%s: con new-session colgado, Abrir sesion responde un error, fue %d: %s", ca, r.rec.Code, r.rec.Body.String())
	}
	tbErrorJSON(t, ca, r.rec, r.rec.Code)

	lineas := atLineas(t, log)
	nueva := -1
	for i, l := range lineas {
		if f := strings.Fields(l); len(f) > 0 && f[0] == "new-session" {
			nueva = i
			break
		}
	}
	if nueva < 0 {
		t.Fatalf("%s: fixture: Abrir sesion llego a tmux new-session: %q", ca, lineas)
	}
	nombre := v3Flag(lineas[nueva], "-s")
	if nombre == "" || len(h3PIDs(t, pids)) == 0 {
		t.Fatalf("%s: fixture: new-session -s <sesion> creo la sesion y se colgo: %q", ca, lineas)
	}

	// la sesion a medias no queda: se cerro, exacta, despues de crearla
	if v3Viva(ses, nombre) {
		t.Errorf("%s: el pedido termino con la sesion %s a medio armar todavia abierta (sin el pane del watch):\n%s", ca, nombre, strings.Join(atLineas(t, log), "\n"))
	}
	if v3Indice(atLineas(t, log), nueva+1, "kill-session", "-t", "="+nombre) < 0 {
		t.Errorf("%s: la sesion a medias se cierra con `tmux kill-session -t =%s` despues del new-session:\n%s", ca, nombre, strings.Join(atLineas(t, log), "\n"))
	}
	if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 0 {
		t.Errorf("%s: una sesion que no se pudo armar no se declara en el item:\n%s", ca, crudo)
	}
	for _, pid := range h3PIDs(t, pids) {
		if !h3EsperarHasta(3*time.Second, func() bool { return !h3VivoPID(pid) }) {
			t.Errorf("%s: el tmux colgado (pid %d) sigue vivo despues de la respuesta", ca, pid)
		}
	}

	// el reintento, con tmux sano, arma la sesion entera
	if err := os.Remove(colgar); err != nil {
		t.Fatal(err)
	}
	desde := len(atLineas(t, log))
	var otra atSesion
	atOK(t, ca+": el reintento con tmux sano", atPedir(t, ca, s, http.MethodPost, ruta, s.Token(), `{"provider": "claude"}`), &otra)
	despues := atLineas(t, log)[desde:]
	if !otra.Created || otra.Session != nombre {
		t.Fatalf("%s: el reintento CREA la sesion %s (created true), no encuentra la de antes a medio armar: %+v\n%s", ca, nombre, otra, strings.Join(despues, "\n"))
	}
	nuevas := atConVerbo(despues, "new-session")
	if len(nuevas) != 1 || v3Flag(nuevas[0], "-s") != nombre || !strings.Contains(nuevas[0], wt) {
		t.Fatalf("%s: el reintento corre new-session -s %s en el espacio de trabajo (%s):\n%s", ca, nombre, wt, strings.Join(despues, "\n"))
	}
	if len(atConVerbo(despues, "split-window")) == 0 {
		t.Fatalf("%s: el reintento arma el pane del watch (split-window):\n%s", ca, strings.Join(despues, "\n"))
	}
	if !v3Viva(ses, nombre) {
		t.Fatalf("%s: despues del reintento la sesion %s queda abierta", ca, nombre)
	}
	if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
		t.Fatalf("%s: el reintento declara la sesion en el item: una entrada, claude:\n%s", ca, crudo)
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
	log, ses, pids, colgar := v3Marcas(t)
	atPATH(t, map[string]string{"tmux": v3Tmux(log, ses, pids, colgar), "claude": "exit 0\n"})
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
	if !login.Created || !v3Viva(ses, login.Session) || !strings.HasPrefix(login.Session, propia) || login.Session == propia {
		t.Fatalf("%s: fixture: la sesion de fix-login (%+v) esta abierta y su nombre empieza con el de la de fix (%s)", ca, login, propia)
	}
	desde := len(atLineas(t, log))

	fix := pedir("fix")
	despues := atLineas(t, log)[desde:]
	if fix.Session != propia || !fix.Created {
		t.Fatalf("%s: con la sesion de fix-login abierta, la tarjeta fix crea SU sesion %s (created true): %+v\n%s", ca, propia, fix, strings.Join(despues, "\n"))
	}
	i := v3Indice(despues, 0, "new-session", "-s", propia)
	if i < 0 || !strings.Contains(despues[i], wtFix) {
		t.Fatalf("%s: new-session -s %s en el espacio de trabajo de fix (%s):\n%s", ca, propia, wtFix, strings.Join(despues, "\n"))
	}
	if !v3Viva(ses, propia) || !v3Viva(ses, login.Session) {
		t.Fatalf("%s: quedan abiertas las dos sesiones, la de fix (%v) y la de fix-login (%v):\n%s", ca, v3Viva(ses, propia), v3Viva(ses, login.Session), strings.Join(despues, "\n"))
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
	desde = len(atLineas(t, log))
	if otra := pedir("fix"); otra.Created || otra.Session != propia {
		t.Fatalf("%s: guarda: abrir fix de nuevo encuentra su sesion (created false): %+v", ca, otra)
	}
	if n := atConVerbo(atLineas(t, log)[desde:], "new-session"); len(n) > 0 {
		t.Fatalf("%s: guarda: con su sesion abierta no se crea otra: %q", ca, n)
	}
}

// Hallazgo fa374d (CA-350): con solo la sesion de fix-login abierta, Ver
// terminal de la tarjeta fix responde 200 con available false, sin text y la
// nota del contrato: nunca la pantalla de la sesion de otra tarjeta cuyo
// nombre empieza igual. Guarda: Ver terminal de fix-login muestra su pantalla.
func TestHallazgo_fa374d_VerTerminalNoMuestraLaSesionDeOtraTarjeta(t *testing.T) {
	const ca = "fa374d (CA-350)"
	log, ses, pids, colgar := v3Marcas(t)
	atPATH(t, map[string]string{"tmux": v3Tmux(log, ses, pids, colgar), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	atConEspacio(t, dir, "fix-login")
	atConEspacio(t, dir, "fix")
	login := cockpitcmd.SessionName("demo", "fix-login")
	if err := os.WriteFile(filepath.Join(ses, login), []byte("viva\n"), 0o644); err != nil { // la sesion de fix-login existe
		t.Fatal(err)
	}
	s := newServer(t, dir)

	var term atTerminal
	atOK(t, ca, atPedir(t, ca, s, http.MethodGet, "/api/board/fix/terminal", "", ""), &term)
	if term.Available || term.Text != "" || term.Note != "no hay una sesion abierta para esta tarjeta" {
		t.Fatalf("%s: fix no tiene sesion: available false, sin text y la nota del contrato, no la pantalla de %s: %+v\n%s", ca, login, term, strings.Join(atLineas(t, log), "\n"))
	}

	// guarda: el tmux falso sirve la pantalla de la sesion que si existe
	term = atTerminal{}
	atOK(t, ca+" (guarda)", atPedir(t, ca, s, http.MethodGet, "/api/board/fix-login/terminal", "", ""), &term)
	if !term.Available || term.Session != login || !strings.Contains(term.Text, "pantalla de otra tarjeta") {
		t.Fatalf("%s: guarda: Ver terminal de fix-login muestra su pantalla: %+v", ca, term)
	}
}
