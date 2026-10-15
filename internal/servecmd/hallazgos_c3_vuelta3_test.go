// Tests de regresion de la tercera vuelta de review de las correcciones de la
// cabina C3 (tarea fixes-cabina-c3) sobre .hoom/specs/acciones-desde-la-tarjeta.md,
// por el Studio:
//
//   - 1b1fc7 (CA-350, enmienda 1 (f)): el Studio servido en un loopback que
//     no es 127.0.0.1 ni localhost a secas (localhost.:P, 127.0.0.2:P) atiende
//     al navegador que abre la URL impresa, que manda ese mismo Host. Un Host
//     ajeno sigue siendo 421.
//   - 0fc76d (CA-347 y CA-348, ENMIENDA 3): un tmux cuyo new-session CREA la
//     sesion y despues se cuelga deja la sesion a medias (un solo pane, el de
//     la IA): el plazo corta el pedido con un error y NO se cierra nada (la
//     sesion pudo armarla otra apertura de la misma tarjeta). El reintento,
//     con tmux sano, la completa: split-window sin new-session, created true,
//     y la declara en el item. (En la vuelta 3 el contrato era el de la
//     enmienda 1 (k): cerrar la sesion a medias y rearmarla; la enmienda 3 lo
//     reemplazo por decision de Henry.)
//   - fa374d (CA-348 y CA-350): tmux resuelve `-t <nombre>` tambien por
//     PREFIJO (solo `-t =<nombre>` es exacto). Con la sesion de la tarjeta
//     fix-login abierta, Abrir sesion en la tarjeta fix crea SU sesion, y Ver
//     terminal de fix no muestra la pantalla de fix-login.
//
// Aca vive tambien el tmux falso con estado (v3Tmux) que usan las vueltas 3,
// 4 y 5. Fixtures de acciones_tarjeta_test.go (atPATH, atConEspacio, atPedir,
// atOK, atLineas, atConVerbo, atLeerItem, atSesion, atTerminal),
// tablero_test.go (tbProyecto, tbErrorJSON) y hallazgos_c3_test.go (h3Eco,
// h3ConHost, h3PIDs, h3VivoPID, h3EsperarHasta). tmux y el CLI de IA son
// falsos.
package servecmd

import (
	"fmt"
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
// El tmux falso con estado (vueltas 3, 4 y 5)

// v3Tmux es UN tmux falso con estado, en un solo script de sh (script()), que
// usan las vueltas 3, 4 y 5. Todo su estado esta en archivos:
//
//   - sesiones: la carpeta ses tiene un archivo por sesion, con un pane por
//     linea ("%<id> <comando>", el primero es el de la IA). La sesion vive
//     mientras su archivo no este vacio. new-session -s N la crea con UN pane
//     (y se niega con "duplicate session: N", exit 1, si N ya vive);
//     split-window le agrega uno; kill-session la vacia; kill-pane le saca un
//     pane (y sin panes la sesion muere). has-session, list-panes (con o sin
//     -F) y display-message -p responden desde ese estado; -P -F de
//     new-session y split-window imprime el pane nuevo; capture-pane imprime
//     una pantalla fija.
//   - destinos: `-t =N` es exacto; `-t N` busca N exacta y, si no esta, la
//     UNICA sesion cuyo nombre empieza con N (lo que hace tmux con un destino
//     de sesion); `-t %<id>` es la sesion de ese pane. Lo que sigue a `:` o
//     `.` en el destino no cuenta.
//   - log: cada invocacion (sus argumentos), una linea, al empezar.
//   - matadas: cada sesion viva que un kill-session o un kill-pane cerro, una
//     linea "<sesion> <verbo>".
//   - colgar: mientras ese archivo exista, new-session crea la sesion (con su
//     pane) y despues se cuelga (anota su PID en pids). Es el new-session que
//     vence el plazo con la sesion ya creada (0fc76d).
//   - ret: la llamada retenida, para orquestar dos aperturas sin depender del
//     scheduler (46e17e, 6a6780, d8d246). Ver v3Retencion.
//
// Solo builtins de sh (kill tambien) y /bin/sleep (sleep con fraccion: GNU y
// macOS).
type v3Tmux struct {
	log, ses, pids, colgar, matadas string
	ret                             v3Retencion
}

// v3Retencion es la UNICA llamada que el tmux falso retiene: la primera, de la
// clase donde, sobre la sesion nombre. Con donde = "" no retiene nada.
//
//   - donde = "sondeo": la primera de has-session, list-panes o
//     display-message cuyo destino es nombre. Mira el estado ANTES de
//     esperar, espera a que exista seguir, y responde lo que vio: el sondeo
//     que paso cuando la sesion todavia no existia.
//   - donde = "new-session": el primer new-session -s nombre, que espera ANTES
//     de mirar nada. Despues, segun luego: "seguir" sigue como siempre (crea la
//     sesion, o "duplicate session" si ya existe); "colgar" se cuelga sin
//     crear nada hasta que lo maten (el plazo); "fallar" sale con salida y
//     mensaje sin crear nada; "matar" muere por una senal (SIGKILL, antes de
//     cualquier plazo) sin crear nada. "fallar" y "matar" crean fallo justo
//     antes.
//
// La llamada retenida anota su PID en pids, crea esperando cuando empieza a
// esperar, y espera hasta que exista seguir (o unos 20 s).
type v3Retencion struct {
	nombre, donde, luego string
	salida               int
	mensaje              string
	unaVez, esperando    string // marcas: unaVez existe desde que se retuvo una llamada
	seguir, fallo        string // el test crea seguir para soltarla
}

// v3Marcas arma los archivos del tmux falso en un TempDir: log, la carpeta de
// sesiones, pids, matadas, la marca colgar (que no existe hasta que se cree) y
// las de la retencion (que no retiene nada hasta que se le ponga donde). Al
// terminar el test suelta la llamada retenida y mata lo que el tmux falso
// haya dejado colgado.
func v3Marcas(t *testing.T) *v3Tmux {
	t.Helper()
	m := t.TempDir()
	f := &v3Tmux{log: filepath.Join(m, "tmux.log"), ses: filepath.Join(m, "sesiones"), pids: filepath.Join(m, "pids"),
		colgar: filepath.Join(m, "colgar"), matadas: filepath.Join(m, "matadas"),
		ret: v3Retencion{unaVez: filepath.Join(m, "una-vez"), esperando: filepath.Join(m, "esperando"),
			seguir: filepath.Join(m, "seguir"), fallo: filepath.Join(m, "fallo")}}
	if err := os.Mkdir(f.ses, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		os.WriteFile(f.ret.seguir, nil, 0o644)
		for _, pid := range h3PIDs(t, f.pids) {
			if h3VivoPID(pid) {
				syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	return f
}

// script es el cuerpo del tmux falso (sin el #!/bin/sh, que pone atFalso).
func (f *v3Tmux) script() string {
	return strings.NewReplacer("@LOG@", f.log, "@SES@", f.ses, "@PIDS@", f.pids, "@COLGAR@", f.colgar,
		"@MATADAS@", f.matadas, "@RNOMBRE@", f.ret.nombre, "@RDONDE@", f.ret.donde, "@RLUEGO@", f.ret.luego,
		"@RSALIDA@", fmt.Sprint(f.ret.salida), "@RMENSAJE@", f.ret.mensaje, "@RUNAVEZ@", f.ret.unaVez,
		"@RESPERANDO@", f.ret.esperando, "@RSEGUIR@", f.ret.seguir, "@RFALLO@", f.ret.fallo).Replace(v3Programa)
}

const v3Programa = `printf '%s\n' "$*" >> '@LOG@'
verbo="$1"
[ $# -gt 0 ] && shift
# los argumentos, como getopt de tmux: los flags hasta el primer posicional
t=""; s=""; F=""; P=no; p=no; cmd=""; fin=no
while [ $# -gt 0 ]; do
  if [ "$fin" = no ]; then
    case "$verbo:$1" in
    *:-t|*:-F|new-session:-[sncxyef]|split-window:-[clpe]|list-panes:-f|display-message:-[cd]|capture-pane:-[SEb])
      if [ $# -ge 2 ]; then
        case "$1" in
        -t) t="$2" ;;
        -s) s="$2" ;;
        -F) F="$2" ;;
        esac
        shift
      fi
      shift; continue ;;
    *:--) fin=si; shift; continue ;;
    *:-?*)
      case "$verbo:$1" in
      new-session:*P*|split-window:*P*) P=si ;;
      display-message:*p*) p=si ;;
      esac
      shift; continue ;;
    esac
  fi
  fin=si
  if [ -z "$cmd" ]; then cmd="$1"; else cmd="$cmd $1"; fi
  shift
done

# buscar resuelve un destino a una sesion viva (hallada) y, con %<id>, a su pane
hallada=""; panel=""
buscar() {
  hallada=""; panel=""
  b_d="$1"
  case "$b_d" in
  %*)
    for b_f in '@SES@'/*; do
      [ -s "$b_f" ] || continue
      while IFS= read -r b_l || [ -n "$b_l" ]; do
        if [ "${b_l%% *}" = "$b_d" ]; then hallada="${b_f##*/}"; panel="$b_d"; return 0; fi
      done < "$b_f"
    done
    return 1 ;;
  esac
  b_d="${b_d%%:*}"
  b_d="${b_d%%.*}"
  case "$b_d" in
  =*)
    b_d="${b_d#=}"
    [ -n "$b_d" ] && [ -s '@SES@'/"$b_d" ] && { hallada="$b_d"; return 0; }
    return 1 ;;
  esac
  [ -n "$b_d" ] || return 1
  [ -s '@SES@'/"$b_d" ] && { hallada="$b_d"; return 0; }
  b_n=0; b_u=""
  for b_f in '@SES@'/"$b_d"*; do
    [ -s "$b_f" ] || continue
    b_u="${b_f##*/}"; b_n=$((b_n+1))
  done
  [ "$b_n" -eq 1 ] && { hallada="$b_u"; return 0; }
  return 1
}

# expandir imprime el formato $1 para el pane $2 (indice $3 de $4) de la sesion $5 con comando $6
expandir() {
  e_f="$1"; e_r=""
  while :; do
    case "$e_f" in
    *'#{'*'}'*) ;;
    *) break ;;
    esac
    e_pre=${e_f%%"#{"*}
    e_f=${e_f#*"#{"}
    e_v=${e_f%%"}"*}
    e_f=${e_f#*"}"}
    e_x=""
    case "$e_v" in
    pane_id) e_x="$2" ;;
    pane_index) e_x="$3" ;;
    window_panes) e_x="$4" ;;
    session_name) e_x="$5" ;;
    window_index) e_x=0 ;;
    window_id) e_x=@0 ;;
    session_windows) e_x=1 ;;
    pane_dead) e_x=0 ;;
    pane_active) if [ "$3" = 0 ]; then e_x=1; else e_x=0; fi ;;
    pane_start_command) e_x="$6" ;;
    pane_current_command) e_x=${6%% *}; e_x=${e_x#"'"}; e_x=${e_x%"'"}; e_x=${e_x##*/} ;;
    esac
    e_r="$e_r$e_pre$e_x"
  done
  printf '%s\n' "$e_r$e_f"
}

# listar imprime los panes de la sesion $1 con el formato $2 ("" = el de tmux);
# con $3 = un pane, solo ese; con $3 = primero, solo el primero
listar() {
  l_total=0
  while IFS= read -r l_l || [ -n "$l_l" ]; do
    [ -n "$l_l" ] && l_total=$((l_total+1))
  done < '@SES@'/"$1"
  l_i=0
  while IFS= read -r l_l || [ -n "$l_l" ]; do
    [ -n "$l_l" ] || continue
    l_id="${l_l%% *}"; l_c=""
    case "$l_l" in *' '*) l_c="${l_l#* }" ;; esac
    if [ -z "$3" ] || [ "$3" = primero ] || [ "$3" = "$l_id" ]; then
      if [ -n "$2" ]; then
        expandir "$2" "$l_id" "$l_i" "$l_total" "$1" "$l_c"
      elif [ "$l_i" = 0 ]; then
        printf '%s: [80x24] [history 0/2000, 0 bytes] %s (active)\n' "$l_i" "$l_id"
      else
        printf '%s: [80x24] [history 0/2000, 0 bytes] %s\n' "$l_i" "$l_id"
      fi
      [ -n "$3" ] && return 0
    fi
    l_i=$((l_i+1))
  done < '@SES@'/"$1"
}

# la retencion (v3Retencion)
unavez() {
  ( set -C; : > '@RUNAVEZ@' ) 2>/dev/null
}
esperar() {
  : > '@RESPERANDO@'
  w_n=0
  while [ ! -f '@RSEGUIR@' ] && [ "$w_n" -lt 400 ]; do
    /bin/sleep 0.05
    w_n=$((w_n+1))
  done
}

sondear() {
  buscar "$t" || { printf "can't find session: %s\n" "$t" >&2; return 1; }
  case "$verbo" in
  list-panes) listar "$hallada" "$F" "" ;;
  display-message)
    if [ "$p" = si ] && [ -n "$cmd" ]; then
      if [ -n "$panel" ]; then listar "$hallada" "$cmd" "$panel"; else listar "$hallada" "$cmd" primero; fi
    fi ;;
  esac
  return 0
}

case "$verbo" in
has-session|list-panes|display-message)
  r_d="${t%%:*}"; r_d="${r_d%%.*}"; r_d="${r_d#=}"
  retener=no
  if [ '@RDONDE@' = sondeo ] && [ "$r_d" = '@RNOMBRE@' ] && unavez; then
    retener=si
    printf '%s\n' "$$" >> '@PIDS@'
  fi
  salida=$(sondear)
  codigo=$?
  [ "$retener" = si ] && esperar
  [ -n "$salida" ] && printf '%s\n' "$salida"
  exit "$codigo" ;;
new-session)
  [ -n "$s" ] || { printf 'falta -s\n' >&2; exit 1; }
  if [ '@RDONDE@' = new-session ] && [ "$s" = '@RNOMBRE@' ] && unavez; then
    printf '%s\n' "$$" >> '@PIDS@'
    case '@RLUEGO@' in
    colgar) : > '@RESPERANDO@'; exec /bin/sleep 20 ;;
    fallar) esperar; : > '@RFALLO@'; printf '%s\n' '@RMENSAJE@' >&2; exit @RSALIDA@ ;;
    matar) esperar; : > '@RFALLO@'; kill -9 $$ ;;
    *) esperar ;;
    esac
  fi
  if [ -s '@SES@'/"$s" ]; then printf 'duplicate session: %s\n' "$s" >&2; exit 1; fi
  printf '%%%s %s\n' "$$" "$cmd" > '@SES@'/"$s"
  if [ -f '@COLGAR@' ]; then
    printf '%s\n' "$$" >> '@PIDS@'
    exec /bin/sleep 20
  fi
  if [ "$P" = si ]; then
    [ -n "$F" ] || F='#{session_name}:'
    expandir "$F" "%$$" 0 1 "$s" "$cmd"
  fi
  exit 0 ;;
split-window)
  buscar "$t" || { printf "can't find pane: %s\n" "$t" >&2; exit 1; }
  printf '%%%s %s\n' "$$" "$cmd" >> '@SES@'/"$hallada"
  if [ "$P" = si ]; then
    [ -n "$F" ] || F='#{session_name}:#{window_index}.#{pane_index}'
    listar "$hallada" "$F" "%$$"
  fi
  exit 0 ;;
kill-session)
  buscar "$t" || { printf "can't find session: %s\n" "$t" >&2; exit 1; }
  printf '%s kill-session\n' "$hallada" >> '@MATADAS@'
  : > '@SES@'/"$hallada"
  exit 0 ;;
kill-pane)
  buscar "$t" || { printf "can't find pane: %s\n" "$t" >&2; exit 1; }
  k_quitar="$panel"
  if [ -z "$k_quitar" ]; then
    IFS= read -r k_l < '@SES@'/"$hallada"
    k_quitar="${k_l%% *}"
  fi
  k_resto=""
  while IFS= read -r k_l || [ -n "$k_l" ]; do
    [ -n "$k_l" ] || continue
    [ "${k_l%% *}" = "$k_quitar" ] && continue
    k_resto="$k_resto$k_l
"
  done < '@SES@'/"$hallada"
  printf '%s' "$k_resto" > '@SES@'/"$hallada"
  [ -s '@SES@'/"$hallada" ] || printf '%s kill-pane\n' "$hallada" >> '@MATADAS@'
  exit 0 ;;
capture-pane)
  printf 'pantalla de %s\n' 'otra tarjeta'
  exit 0 ;;
esac
exit 0
`

// v3Sembrar deja en el tmux falso la sesion nombre viva con panes panes (el
// primero, el de la IA): una sesion que armo otro, o que quedo a medias.
func v3Sembrar(t *testing.T, ses, nombre string, panes int) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < panes; i++ {
		cmd := "claude"
		if i > 0 {
			cmd = "'hoom' status --watch"
		}
		fmt.Fprintf(&b, "%%9%d%02d %s\n", len(nombre), i, cmd)
	}
	if err := os.WriteFile(filepath.Join(ses, nombre), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// v3Viva dice si el tmux falso tiene viva la sesion nombre (exacta).
func v3Viva(ses, nombre string) bool {
	return v3Panes(ses, nombre) > 0
}

// v3Panes es la cantidad de panes de la sesion nombre (0 = no vive).
func v3Panes(ses, nombre string) int {
	raw, err := os.ReadFile(filepath.Join(ses, nombre))
	if err != nil {
		return 0
	}
	n := 0
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// cerradas son las veces que la sesion nombre, viva, quedo cerrada por un
// kill-session o un kill-pane (por cualquier destino, tambien %<id>): lo que
// el tmux falso registro en matadas.
func (f *v3Tmux) cerradas(t *testing.T, nombre string) []string {
	t.Helper()
	var out []string
	for _, l := range atLineas(t, f.matadas) {
		if c := strings.Fields(l); len(c) > 0 && c[0] == nombre {
			out = append(out, "cerrada por "+l)
		}
	}
	return out
}

// cierres son los intentos de cerrar la sesion nombre: cada kill-session o
// kill-pane cuyo destino la nombra (exacto con =, o un prefijo de nombre sin
// =), viva o no, y cada vez que quedo cerrada (cerradas).
func (f *v3Tmux) cierres(t *testing.T, nombre string) []string {
	t.Helper()
	var out []string
	for _, l := range atLineas(t, f.log) {
		c := strings.Fields(l)
		if len(c) == 0 || (c[0] != "kill-session" && c[0] != "kill-pane") {
			continue
		}
		d := v3Flag(l, "-t")
		exacto := strings.HasPrefix(d, "=")
		d = strings.TrimPrefix(d, "=")
		if i := strings.IndexAny(d, ":."); i >= 0 {
			d = d[:i]
		}
		if d != "" && (d == nombre || (!exacto && strings.HasPrefix(nombre, d))) {
			out = append(out, "tmux "+l)
		}
	}
	return append(out, f.cerradas(t, nombre)...)
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
// 0fc76d: la sesion a medias no se cierra: se completa (enmienda 3)

// Hallazgo 0fc76d (CA-347 y CA-348). El contrato es el de la ENMIENDA 3 de la
// spec ("si tmux new-session falla o vence su plazo, Open devuelve el error y
// no cierra nada"; "una sesion de la tarjeta con un solo pane quedo a medias:
// la proxima apertura le agrega el pane de vigilancia, la cuenta como creada
// y no la vuelve a crear"), que reemplazo al de la enmienda 1 (k) con el que
// se escribio este test en la vuelta 3 (cerrar la sesion a medias y rearmarla
// con new-session): lo decidio Henry, no es un test debilitado.
//
// Con un tmux cuyo new-session CREA la sesion (un pane, el de la IA) y
// despues se cuelga, POST /api/board/<slug>/session responde un error en
// menos de 8 s (el plazo es 5 s) y NO cierra nada: ningun kill-session (ni
// kill-pane) de esa sesion, que sigue viva con un solo pane; el item no
// declara nada y el tmux colgado no queda vivo. Despues, con tmux sano, un
// segundo POST la completa: NO corre new-session, SI split-window sobre esa
// sesion (que pasa a tener dos panes, con el watch), responde created true con
// esa sesion, y el item tiene exactamente una entrada (claude).
func TestHallazgo_0fc76d_AbrirSesionAMediasSeCompletaEnElReintento(t *testing.T) {
	const ca = "0fc76d (CA-347, CA-348, enmienda 3)"
	f := v3Marcas(t)
	if err := os.WriteFile(f.colgar, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	const slug = "a-medias"
	atConEspacio(t, dir, slug)
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

	lineas := atLineas(t, f.log)
	nueva := -1
	for i, l := range lineas {
		if c := strings.Fields(l); len(c) > 0 && c[0] == "new-session" {
			nueva = i
			break
		}
	}
	if nueva < 0 {
		t.Fatalf("%s: fixture: Abrir sesion llego a tmux new-session: %q", ca, lineas)
	}
	nombre := v3Flag(lineas[nueva], "-s")
	if nombre == "" || len(h3PIDs(t, f.pids)) == 0 {
		t.Fatalf("%s: fixture: new-session -s <sesion> creo la sesion y se colgo: %q", ca, lineas)
	}

	// no se cierra nada: la sesion queda a medias, con el pane de la IA
	if k := f.cierres(t, nombre); len(k) > 0 {
		t.Errorf("%s: despues de un new-session que vencio su plazo no se cierra nada (la sesion pudo armarla otra apertura), y hubo:\n%s\ntmux:\n%s", ca, strings.Join(k, "\n"), strings.Join(atLineas(t, f.log), "\n"))
	}
	if n := v3Panes(f.ses, nombre); n != 1 {
		t.Errorf("%s: la sesion %s queda a medias: viva con un solo pane (el de la IA), y tiene %d\ntmux:\n%s", ca, nombre, n, strings.Join(atLineas(t, f.log), "\n"))
	}
	if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 0 {
		t.Errorf("%s: una apertura que respondio un error no declara la sesion en el item:\n%s", ca, crudo)
	}
	for _, pid := range h3PIDs(t, f.pids) {
		if !h3EsperarHasta(3*time.Second, func() bool { return !h3VivoPID(pid) }) {
			t.Errorf("%s: el tmux colgado (pid %d) sigue vivo despues de la respuesta", ca, pid)
		}
	}

	// el reintento, con tmux sano, la completa: split-window, sin new-session
	if err := os.Remove(f.colgar); err != nil {
		t.Fatal(err)
	}
	desde := len(atLineas(t, f.log))
	var otra atSesion
	atOK(t, ca+": el reintento con tmux sano", atPedir(t, ca, s, http.MethodPost, ruta, s.Token(), `{"provider": "claude"}`), &otra)
	despues := atLineas(t, f.log)[desde:]
	tmux := strings.Join(despues, "\n")
	if n := atConVerbo(despues, "new-session"); len(n) > 0 {
		t.Errorf("%s: el reintento NO vuelve a crear la sesion a medias (sin new-session): %q", ca, n)
	}
	if len(atConVerbo(despues, "split-window")) == 0 {
		t.Errorf("%s: el reintento le agrega el pane de vigilancia (split-window):\n%s", ca, tmux)
	}
	if !otra.Created || otra.Session != nombre {
		t.Errorf("%s: el reintento cuenta la sesion %s completada como creada (created true): %+v\n%s", ca, nombre, otra, tmux)
	}
	if n := v3Panes(f.ses, nombre); n != 2 {
		t.Errorf("%s: despues del reintento la sesion %s tiene sus dos panes (la IA y el watch), y tiene %d:\n%s", ca, nombre, n, tmux)
	}
	if !strings.Contains(tmux, "status --watch") {
		t.Errorf("%s: el pane que agrega el reintento corre `hoom status --watch`:\n%s", ca, tmux)
	}
	if k := f.cierres(t, nombre); len(k) > 0 {
		t.Errorf("%s: el reintento tampoco cierra la sesion: %q", ca, k)
	}
	if it, crudo := atLeerItem(t, dir, slug); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
		t.Errorf("%s: el item tiene exactamente una entrada en sesiones (claude), la del reintento que la completo:\n%s", ca, crudo)
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
	f := v3Marcas(t)
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
	if !login.Created || !v3Viva(f.ses, login.Session) || !strings.HasPrefix(login.Session, propia) || login.Session == propia {
		t.Fatalf("%s: fixture: la sesion de fix-login (%+v) esta abierta y su nombre empieza con el de la de fix (%s)", ca, login, propia)
	}
	desde := len(atLineas(t, f.log))

	fix := pedir("fix")
	despues := atLineas(t, f.log)[desde:]
	if fix.Session != propia || !fix.Created {
		t.Fatalf("%s: con la sesion de fix-login abierta, la tarjeta fix crea SU sesion %s (created true): %+v\n%s", ca, propia, fix, strings.Join(despues, "\n"))
	}
	i := v3Indice(despues, 0, "new-session", "-s", propia)
	if i < 0 || !strings.Contains(despues[i], wtFix) {
		t.Fatalf("%s: new-session -s %s en el espacio de trabajo de fix (%s):\n%s", ca, propia, wtFix, strings.Join(despues, "\n"))
	}
	if !v3Viva(f.ses, propia) || !v3Viva(f.ses, login.Session) {
		t.Fatalf("%s: quedan abiertas las dos sesiones, la de fix (%v) y la de fix-login (%v):\n%s", ca, v3Viva(f.ses, propia), v3Viva(f.ses, login.Session), strings.Join(despues, "\n"))
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
	f := v3Marcas(t)
	atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
	dir := tbProyecto(t)
	atConEspacio(t, dir, "fix-login")
	atConEspacio(t, dir, "fix")
	login := cockpitcmd.SessionName("demo", "fix-login")
	v3Sembrar(t, f.ses, login, 2) // la sesion de fix-login existe
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
