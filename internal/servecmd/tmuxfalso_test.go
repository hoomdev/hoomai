// El tmux falso con estado y las aperturas de sesion (del Studio y de la CLI)
// que usan los tests de Abrir sesion y Ver terminal de
// .hoom/specs/acciones-desde-la-tarjeta.md (CA-347, CA-348, CA-350) en
// hallazgos_c3_vuelta3_test.go, hallazgos_c3_vuelta4_test.go,
// hallazgos_c3_vuelta5_test.go, hallazgos_c3_337b23_test.go y
// hallazgos_c3_db8541_f7efdc_test.go (estos dos, con el modo
// tmuxFalso.destinosComoTmux) y hallazgos_c3_b59c30_e0001e_test.go. Aca no hay
// tests: solo el doble de tmux, como se abre una sesion contra el, y que dice
// el error de una apertura que creo la sesion y fallo despues: con la
// ENMIENDA 6 (hallazgo bde7e4) la apertura no la cierra, y su error lo
// verifica incumplimientosEnmienda6, que comparten los tests de f7efdc y de
// b59c30/e0001e/9c1816 (el analizador DiceQueLaCerro es comun con cockpitcmd
// y vive en internal/cierretmuxtest, hallazgo 9accd8).
//
// Usa fixtures de acciones_tarjeta_test.go (atLineas, atSesion),
// hallazgos_c3_test.go (h3PIDs, h3VivoPID) y hallazgos_c3_vuelta3_test.go
// (contieneElAviso).
package servecmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cierretmuxtest"
	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// ---------------------------------------------------------------------------
// El tmux falso

// tmuxFalso es UN tmux falso con estado, en un solo script de sh (script()).
// Todo su estado esta en archivos:
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
//     `.` en el destino no cuenta. Sin -t no hay sesion. Asi es con
//     destinosComoTmux en false (el modo de siempre); en true, ver abajo.
//   - destinosComoTmux (hallazgo 5cee1e; antes se llamaba real, un nombre
//     enganoso): el modo en que el doble imita como tmux 3.x resuelve los
//     destinos (visto con tmux 3.6b, enmienda 5 de la spec, hallazgo 337b23).
//     En ningun modo se ejecuta un tmux de verdad. En true cambian A LA VEZ
//     cuatro cosas:
//     1. La resolucion de -t, segun la clase de destino del verbo. De sesion
//     (has-session, kill-session): `=N` y `=N:` son exactos; `N` y `N:` son
//     N exacta o el UNICO prefijo. De ventana (list-panes) y de pane
//     (split-window, select-pane, kill-pane, capture-pane, display-message):
//     `=N:` (con lo que siga) es exacto; `N` y `N:` son N exacta o el UNICO
//     prefijo; `=N` SIN los dos puntos NO es exacto en list-panes (N o el
//     prefijo, como un nombre suelto) y en los de pane falla siempre. Sin -t,
//     la unica sesion viva (si hay una sola).
//     2. Los errores de un destino que no se resuelve: el de tmux para su
//     clase ("can't find session: N", "can't find window: <destino>",
//     "can't find pane: <destino>", "no current session") en vez del fijo de
//     cada verbo.
//     3. display-message y select-pane: display-message con un destino que no
//     se resuelve imprime el formato vacio y sale con 0, como tmux (en false
//     sale con 1); select-pane resuelve su destino y falla si no puede (en
//     false sale siempre con 0, sin mirarlo).
//     4. capture-pane resuelve su destino como pane e imprime "pantalla de
//     [<sesion>] <pane>", para que el test vea DE QUE sesion es la pantalla
//     (en false no resuelve nada e imprime siempre "pantalla de otra
//     tarjeta").
//   - morir: si ese archivo existe y nombra la sesion N, new-session -s N la
//     crea (con su pane, y -P -F lo imprime), sale con 0 y la sesion muere
//     enseguida: la CLI de la IA salio al arrancar (337b23).
//   - log: cada invocacion (sus argumentos), una linea, al empezar.
//   - matadas: cada sesion viva que un kill-session o un kill-pane cerro, una
//     linea "<sesion> <verbo>".
//   - colgar: mientras ese archivo exista, new-session crea la sesion (con su
//     pane) y despues se cuelga (anota su PID en pids). Es el new-session que
//     vence el plazo con la sesion ya creada (0fc76d).
//   - crearYFallar: mientras ese archivo exista, new-session crea la sesion
//     (con su pane) y despues sale con 1 y "server exited unexpectedly". Es el
//     new-session que falla con la sesion ya creada (0fc76d).
//   - ret: la llamada retenida, para orquestar dos aperturas sin depender del
//     scheduler (46e17e, 6a6780, d8d246). Ver retencion.
//
// Solo builtins de sh (kill tambien) y /bin/sleep (sleep con fraccion: GNU y
// macOS).
type tmuxFalso struct {
	log, ses, pids, colgar, crearYFallar, matadas, morir string
	destinosComoTmux                                     bool
	ret                                                  retencion
}

// retencion es la UNICA llamada que el tmux falso retiene: la primera, de la
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
type retencion struct {
	nombre, donde, luego string
	salida               int
	mensaje              string
	unaVez, esperando    string // marcas: unaVez existe desde que se retuvo una llamada
	seguir, fallo        string // el test crea seguir para soltarla
}

// nuevoTmuxFalso arma los archivos del tmux falso en un TempDir: log, la
// carpeta de sesiones, pids, matadas, las marcas colgar, crearYFallar y morir
// (que no existen hasta que se creen) y las de la retencion (que no retiene
// nada hasta que se le ponga donde). destinosComoTmux empieza en false: los
// destinos de siempre. Al terminar el test suelta la llamada retenida y mata lo que el
// tmux falso haya dejado colgado.
func nuevoTmuxFalso(t *testing.T) *tmuxFalso {
	t.Helper()
	m := t.TempDir()
	f := &tmuxFalso{log: filepath.Join(m, "tmux.log"), ses: filepath.Join(m, "sesiones"), pids: filepath.Join(m, "pids"),
		colgar: filepath.Join(m, "colgar"), crearYFallar: filepath.Join(m, "crear-y-fallar"), matadas: filepath.Join(m, "matadas"),
		morir: filepath.Join(m, "morir"),
		ret: retencion{unaVez: filepath.Join(m, "una-vez"), esperando: filepath.Join(m, "esperando"),
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
func (f *tmuxFalso) script() string {
	comoTmux := "no"
	if f.destinosComoTmux {
		comoTmux = "si"
	}
	return strings.NewReplacer("@LOG@", f.log, "@SES@", f.ses, "@PIDS@", f.pids, "@COLGAR@", f.colgar,
		"@CREARYFALLAR@", f.crearYFallar, "@MATADAS@", f.matadas, "@MORIR@", f.morir, "@COMOTMUX@", comoTmux,
		"@RNOMBRE@", f.ret.nombre, "@RDONDE@", f.ret.donde,
		"@RLUEGO@", f.ret.luego, "@RSALIDA@", fmt.Sprint(f.ret.salida), "@RMENSAJE@", f.ret.mensaje,
		"@RUNAVEZ@", f.ret.unaVez, "@RESPERANDO@", f.ret.esperando, "@RSEGUIR@", f.ret.seguir,
		"@RFALLO@", f.ret.fallo).Replace(programaTmuxFalso)
}

const programaTmuxFalso = `printf '%s\n' "$*" >> '@LOG@'
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

# suelta resuelve el nombre $1 como tmux resuelve un nombre de sesion suelto:
# la sesion viva con ese nombre exacto o, si no esta, la UNICA cuyo nombre
# empieza con el
suelta() {
  u_d="$1"
  [ -n "$u_d" ] || return 1
  [ -s '@SES@'/"$u_d" ] && { hallada="$u_d"; return 0; }
  u_n=0; u_u=""
  for u_f in '@SES@'/"$u_d"*; do
    [ -s "$u_f" ] || continue
    u_u="${u_f##*/}"; u_n=$((u_n+1))
  done
  [ "$u_n" -eq 1 ] && { hallada="$u_u"; return 0; }
  return 1
}

# buscarComoTmux resuelve el destino $1 de un verbo con destino de clase $2
# (sesion, ventana o pane) como tmux 3.x (con destinosComoTmux); si no puede,
# deja en motivo el error que da tmux
motivo=""
buscarComoTmux() {
  hallada=""; panel=""; motivo=""
  r_t="$1"; r_c="$2"
  case "$r_t" in
  '')
    r_n=0; r_u=""
    for r_f in '@SES@'/*; do
      [ -s "$r_f" ] || continue
      r_u="${r_f##*/}"; r_n=$((r_n+1))
    done
    [ "$r_n" -eq 1 ] && { hallada="$r_u"; return 0; }
    motivo="no current session"; return 1 ;;
  %*)
    buscar "$r_t" && return 0
    motivo="can't find pane: $r_t"; return 1 ;;
  esac
  case "$r_t" in
  *:*) r_s="${r_t%%:*}"; r_dos=si ;;
  *) r_s="${r_t%%.*}"; r_dos=no ;;
  esac
  case "$r_s" in
  =*)
    r_s="${r_s#=}"
    if [ "$r_c" = sesion ] || [ "$r_dos" = si ]; then
      [ -n "$r_s" ] && [ -s '@SES@'/"$r_s" ] && { hallada="$r_s"; return 0; }
      motivo="can't find session: $r_s"; return 1
    fi
    if [ "$r_c" = pane ]; then
      motivo="can't find pane: $r_t"; return 1
    fi
    suelta "$r_s" && return 0
    motivo="can't find window: $r_t"; return 1 ;;
  esac
  suelta "$r_s" && return 0
  if [ "$r_c" = sesion ] || [ "$r_dos" = si ]; then
    motivo="can't find session: $r_s"
  elif [ "$r_c" = pane ]; then
    motivo="can't find pane: $r_t"
  else
    motivo="can't find window: $r_t"
  fi
  return 1
}

# resolver resuelve el destino $1 de un verbo de clase $2: con
# destinosComoTmux, como tmux (buscarComoTmux); si no, como siempre (buscar),
# con el error $3
resolver() {
  if [ '@COMOTMUX@' = si ]; then
    buscarComoTmux "$1" "$2"
    return $?
  fi
  motivo="$3"
  buscar "$1"
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

# la retencion (retencion en Go)
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
  case "$verbo" in
  has-session) s_cl=sesion ;;
  list-panes) s_cl=ventana ;;
  *) s_cl=pane ;;
  esac
  if ! resolver "$t" "$s_cl" "can't find session: $t"; then
    if [ '@COMOTMUX@' = si ] && [ "$verbo" = display-message ]; then
      [ "$p" = si ] && [ -n "$cmd" ] && expandir "$cmd" "" "" "" "" ""
      return 0
    fi
    printf '%s\n' "$motivo" >&2
    return 1
  fi
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
  if [ -f '@CREARYFALLAR@' ]; then
    printf 'server exited unexpectedly\n' >&2
    exit 1
  fi
  if [ "$P" = si ]; then
    [ -n "$F" ] || F='#{session_name}:'
    expandir "$F" "%$$" 0 1 "$s" "$cmd"
  fi
  if [ -f '@MORIR@' ]; then
    m_n=""
    IFS= read -r m_n < '@MORIR@'
    [ "$m_n" = "$s" ] && : > '@SES@'/"$s"
  fi
  exit 0 ;;
split-window)
  resolver "$t" pane "can't find pane: $t" || { printf '%s\n' "$motivo" >&2; exit 1; }
  printf '%%%s %s\n' "$$" "$cmd" >> '@SES@'/"$hallada"
  if [ "$P" = si ]; then
    [ -n "$F" ] || F='#{session_name}:#{window_index}.#{pane_index}'
    listar "$hallada" "$F" "%$$"
  fi
  exit 0 ;;
kill-session)
  resolver "$t" sesion "can't find session: $t" || { printf '%s\n' "$motivo" >&2; exit 1; }
  printf '%s kill-session\n' "$hallada" >> '@MATADAS@'
  : > '@SES@'/"$hallada"
  exit 0 ;;
kill-pane)
  resolver "$t" pane "can't find pane: $t" || { printf '%s\n' "$motivo" >&2; exit 1; }
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
  if [ '@COMOTMUX@' = si ]; then
    buscarComoTmux "$t" pane || { printf '%s\n' "$motivo" >&2; exit 1; }
    c_p="$panel"
    if [ -z "$c_p" ]; then
      IFS= read -r c_l < '@SES@'/"$hallada"
      c_p="${c_l%% *}"
    fi
    printf 'pantalla de [%s] %s\n' "$hallada" "$c_p"
    exit 0
  fi
  printf 'pantalla de %s\n' 'otra tarjeta'
  exit 0 ;;
select-pane)
  if [ '@COMOTMUX@' = si ]; then
    buscarComoTmux "$t" pane || { printf '%s\n' "$motivo" >&2; exit 1; }
  fi
  exit 0 ;;
esac
exit 0
`

// sembrar deja en el tmux falso la sesion nombre viva con panes panes (el
// primero, el de la IA): una sesion que armo otro, o que quedo a medias.
func (f *tmuxFalso) sembrar(t *testing.T, nombre string, panes int) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < panes; i++ {
		cmd := "claude"
		if i > 0 {
			cmd = "'hoom' status --watch"
		}
		fmt.Fprintf(&b, "%%9%d%02d %s\n", len(nombre), i, cmd)
	}
	if err := os.WriteFile(filepath.Join(f.ses, nombre), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// viva dice si el tmux falso tiene viva la sesion nombre (exacta).
func (f *tmuxFalso) viva(nombre string) bool {
	return f.panes(nombre) > 0
}

// panes es la cantidad de panes de la sesion nombre (0 = no vive).
func (f *tmuxFalso) panes(nombre string) int {
	raw, err := os.ReadFile(filepath.Join(f.ses, nombre))
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
func (f *tmuxFalso) cerradas(t *testing.T, nombre string) []string {
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
func (f *tmuxFalso) cierres(t *testing.T, nombre string) []string {
	t.Helper()
	var out []string
	for _, l := range atLineas(t, f.log) {
		c := strings.Fields(l)
		if len(c) == 0 || (c[0] != "kill-session" && c[0] != "kill-pane") {
			continue
		}
		d := flagDeTmux(l, "-t")
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

// flagDeTmux devuelve el valor de flag en una linea del log del tmux falso.
func flagDeTmux(linea, flag string) string {
	f := strings.Fields(linea)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == flag {
			return f[i+1]
		}
	}
	return ""
}

// indiceDeTmux es la posicion de la primera linea de lineas, desde la posicion
// desde, con ese verbo y cuyo flag vale valor (-1 si no hay).
func indiceDeTmux(lineas []string, desde int, verbo, flag, valor string) int {
	for i := desde; i < len(lineas); i++ {
		if f := strings.Fields(lineas[i]); len(f) > 0 && f[0] == verbo && flagDeTmux(lineas[i], flag) == valor {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------------------
// Las aperturas de sesion

// apertura es como termino una apertura: la del Studio (POST
// /api/board/<slug>/session) o la de la CLI (cockpitcmd.Open, lo que hace
// `hoom cockpit --task <slug>` antes de adjuntarse).
type apertura struct {
	quien  string
	rec    *httptest.ResponseRecorder // el Studio
	ses    cockpitcmd.Session         // la CLI
	err    error                      // la CLI
	panico any
}

func (a apertura) String() string {
	if a.panico != nil {
		return fmt.Sprintf("%s: panico %v", a.quien, a.panico)
	}
	if a.rec != nil {
		return fmt.Sprintf("%s: %d %s", a.quien, a.rec.Code, strings.TrimSpace(a.rec.Body.String()))
	}
	return fmt.Sprintf("%s: %+v, error %v", a.quien, a.ses, a.err)
}

// creo dice si esta apertura creo la sesion nombre (created true).
func (a apertura) creo(nombre string) bool {
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
func (a apertura) cedio(nombre string) bool {
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
func (a apertura) fallo() bool {
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

// mensaje es el texto del error de una apertura que fallo: el "error" del JSON
// en el Studio, el de cockpitcmd.Open en la CLI ("" si no fallo).
func (a apertura) mensaje() string {
	if !a.fallo() {
		return ""
	}
	if a.rec != nil {
		var e map[string]string
		json.Unmarshal(a.rec.Body.Bytes(), &e)
		return e["error"]
	}
	return a.err.Error()
}

// abrirSesion abre la sesion de la tarjeta slug como quien: "studio" (POST
// /api/board/<slug>/session de s, con su token) o "cli" (cockpitcmd.Open sobre
// dir con las deps de la CLI). Un panico es un resultado, no la caida del test.
func abrirSesion(s *Server, dir, slug, quien string) (a apertura) {
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

// lanzarApertura abre la sesion en una goroutine; el canal trae como termino.
func lanzarApertura(s *Server, dir, slug, quien string) <-chan apertura {
	ch := make(chan apertura, 1)
	go func() { ch <- abrirSesion(s, dir, slug, quien) }()
	return ch
}

// esperarApertura espera el resultado de una apertura hasta 20 s.
func esperarApertura(t *testing.T, ca string, f *tmuxFalso, ch <-chan apertura, cual string) apertura {
	t.Helper()
	select {
	case a := <-ch:
		return a
	case <-time.After(20 * time.Second):
		t.Fatalf("%s: la apertura %s no termino en 20 s:\n%s", ca, cual, strings.Join(atLineas(t, f.log), "\n"))
	}
	return apertura{}
}

// ---------------------------------------------------------------------------
// El error cuando algo falla despues de crear la sesion

// incumplimientosEnmienda6 devuelve en que incumple msg el contrato de la
// ENMIENDA 6 (hallazgo bde7e4): msg es el error de una apertura que creo la
// sesion sesion y despues fallo el split-window o el registro en el item. La
// apertura no la cierra (cerrarla por su nombre no prueba que sea la suya),
// asi que el error no dice que la cerro, y dice como cerrarla a mano con
// `tmux kill-session -t '=<sesion>'`, con el destino entre comillas (enmienda
// 5, hallazgo db8541) y el nombre exacto de la sesion (contieneElAviso: el de
// otra sesion cuyo nombre empieza igual no cuenta). Que el comando corra tal
// cual en sh y en zsh lo exige aparte cierreEjecutable; que conserve el error
// original lo exigen los tests de internal/cockpitcmd. No exige que diga "no
// se si sigue abierta": eso era del contrato del cierre fallido, que la
// enmienda 6 elimino. No es un predicado: devuelve la lista de
// incumplimientos, vacia si msg cumple todo.
func incumplimientosEnmienda6(msg, sesion string) []string {
	var mal []string
	if cierretmuxtest.DiceQueLaCerro(msg) {
		mal = append(mal, "no dice que la cerro: con la enmienda 6 Abrir sesion no la cierra")
	}
	if aviso := "tmux kill-session -t '=" + sesion + "'"; !contieneElAviso(msg, aviso) {
		mal = append(mal, "dice como cerrarla a mano con el comando exacto `"+aviso+"`, con el nombre de la sesion")
	}
	return mal
}
