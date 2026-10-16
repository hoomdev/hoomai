// Tests de los hallazgos b59c30 y e0001e sobre Abrir sesion, de punta a punta
// (POST /api/board/<slug>/session del Studio y cockpitcmd.Open de la CLI),
// escritos desde .hoom/specs/acciones-desde-la-tarjeta.md (CA-347, CA-348) y
// el contrato de los hallazgos:
//
// Abrir sesion crea la sesion, el pane de vigilancia (split-window) falla y el
// cierre (`tmux kill-session -t =<sesion>`) TAMBIEN falla: la sesion sigue
// viva, a medias. Para saber que decir, Abrir sesion sondea con `tmux
// has-session -t =<sesion>`, y ese sondeo NO puede confirmar nada (estado C):
// se cuelga hasta el plazo de 5 s que el Studio pone a cada llamada a tmux,
// sale por una senal, o no arranca porque tmux desaparecio del PATH entre el
// kill-session y el sondeo. El error NO dice que la sesion ya no existe (eso
// solo vale cuando tmux lo confirma, con has-session saliendo con codigo 1:
// estado A, el test de f7efdc en hallazgos_c3_db8541_f7efdc_test.go): muestra
// el comando de cierre `tmux kill-session -t '=<sesion>'` con comillas, que
// corre tal cual en sh y en zsh (cierreEjecutable), y dice que no se pudo
// saber si sigue abierta. El item queda byte a byte igual. Antes del arreglo,
// cualquier fallo del sondeo daba "ya no existe": con la sesion viva, el error
// mentia y escondia el comando para cerrarla.
//
// El tmux es el falso con estado de tmuxfalso_test.go (con destinosComoTmux),
// envuelto por un script que rompe split-window y kill-session y corta el
// sondeo. Fixtures de acciones_tarjeta_test.go (atPATH, atFalso, atConEspacio,
// atLineas, atLeerItem), tablero_test.go (tbProyecto), hallazgos_c3_test.go
// (h3PIDs, h3VivoPID, h3EsperarHasta) y hallazgos_c3_vuelta3_test.go
// (contieneElAviso). Ni tmux ni el CLI de IA son de verdad.
package servecmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// hb5Envoltorio es el tmux que ve Abrir sesion: split-window falla (la sesion
// queda viva con un solo pane), kill-session falla sin cerrar nada y deja la
// marca @CERRO@ despues de correr @ALCERRAR@, y desde esa marca has-session
// (el sondeo) corre @SONDEO@. Lo demas lo atiende el tmux falso con estado
// (@BASE@). Lo que atiende el envoltorio lo anota en el mismo log.
const hb5Envoltorio = `case "$1" in
split-window)
  printf '%s\n' "$*" >> '@LOG@'
  printf 'create pane failed: pane too small\n' >&2
  exit 1 ;;
kill-session)
  printf '%s\n' "$*" >> '@LOG@'
  : > '@CERRO@'
  @ALCERRAR@
  printf 'lost server\n' >&2
  exit 1 ;;
has-session)
  if [ -f '@CERRO@' ]; then
    printf '%s\n' "$*" >> '@LOG@'
    @SONDEO@
  fi ;;
esac
exec /bin/sh '@BASE@' "$@"
`

// hb5SinTildes pasa msg a minusculas sin tildes, para buscar frases.
func hb5SinTildes(msg string) string {
	return strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u").Replace(strings.ToLower(msg))
}

// hb5Incierto son las maneras de decir que no se pudo saber si la sesion sigue
// abierta (sobre el mensaje sin tildes, en minusculas).
var hb5Incierto = []string{
	"no se pudo saber", "no pude saber", "no se puede saber", "no se sabe", "no se si",
	"no se pudo confirmar", "no pude confirmar", "no se puede confirmar",
	"no se pudo comprobar", "no pude comprobar", "no se pudo verificar", "no pude verificar",
	"puede que siga", "quiza siga", "quizas siga", "tal vez siga", "podria seguir",
}

// Hallazgos b59c30 y e0001e (CA-347, CA-348), de punta a punta: Abrir sesion
// de la tarjeta fix crea la sesion, el split-window falla, el kill-session
// falla y la sesion sigue viva con un solo pane; el sondeo has-session no
// confirma nada (estado C):
//
//   - vence el plazo de 5 s del Studio: el has-session se cuelga. La respuesta
//     llega igual (en menos de 15 s) y el tmux colgado no queda vivo. Solo
//     desde el Studio: la CLI no pone plazo.
//   - sale por una senal (SIGKILL), desde el Studio y desde la CLI.
//   - no arranca: el kill-session fallido se lleva el tmux del PATH, desde el
//     Studio y desde la CLI.
//
// La apertura responde un error que NO dice que la sesion ya no existe,
// muestra `tmux kill-session -t '=<sesion>'` (que corre tal cual en sh y en
// zsh) y dice que no se pudo saber si sigue abierta; el item de fix queda
// byte a byte igual.
func TestCA347_CA348_HallazgosB59c30E0001e_AbrirSesionConUnSondeoQueNoConfirmaNada(t *testing.T) {
	for _, c := range []struct {
		caso, quien string
		corte       string // "plazo", "senal" o "sin tmux"
	}{
		{"el has-session del Studio vence su plazo de 5 s", "studio", "plazo"},
		{"el has-session del Studio sale por una senal", "studio", "senal"},
		{"el has-session de la CLI sale por una senal", "cli", "senal"},
		{"el has-session del Studio no arranca: tmux ya no esta", "studio", "sin tmux"},
		{"el has-session de la CLI no arranca: tmux ya no esta", "cli", "sin tmux"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			ca := "b59c30/e0001e (CA-347, CA-348): " + c.caso
			f := nuevoTmuxFalso(t)
			f.destinosComoTmux = true
			bin := atPATH(t, map[string]string{"claude": "exit 0\n"})
			m := t.TempDir()
			base := filepath.Join(m, "tmux-base")
			if err := os.WriteFile(base, []byte("#!/bin/sh\n"+f.script()), 0o755); err != nil {
				t.Fatal(err)
			}
			yo := filepath.Join(bin, "tmux")
			alCerrar, sondeo := ":", ":"
			switch c.corte {
			case "plazo":
				sondeo = `printf '%s\n' "$$" >> '` + f.pids + `'; exec /bin/sleep 60`
			case "senal":
				sondeo = `kill -9 $$`
			case "sin tmux":
				alCerrar = `/bin/rm -f '` + yo + `'`
			}
			atFalso(t, bin, "tmux", strings.NewReplacer("@LOG@", f.log, "@CERRO@", filepath.Join(m, "cerro"),
				"@ALCERRAR@", alCerrar, "@SONDEO@", sondeo, "@BASE@", base).Replace(hb5Envoltorio))
			dir := tbProyecto(t)
			wtFix := atConEspacio(t, dir, "fix")
			s := newServer(t, dir)
			propia := cockpitcmd.SessionName("demo", "fix")
			_, itemFix := atLeerItem(t, dir, "fix")

			var a apertura
			select {
			case a = <-lanzarApertura(s, dir, "fix", c.quien):
			case <-time.After(15 * time.Second):
				t.Fatalf("%s: Abrir sesion con el sondeo cortado se quedo colgado mas de 15 s (el plazo de cada llamada a tmux es 5 s):\n%s", ca, strings.Join(atLineas(t, f.log), "\n"))
			}
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if a.panico != nil {
				t.Fatalf("%s: la apertura entra en panico: %v\ntmux:\n%s", ca, a.panico, tmux)
			}

			// fixture: creo la sesion, el pane de vigilancia y el cierre fallaron, y la sesion sigue viva a medias
			nueva := indiceDeTmux(lineas, 0, "new-session", "-s", propia)
			if nueva < 0 || !strings.Contains(lineas[nueva], wtFix) {
				t.Fatalf("%s: fixture: la apertura crea la sesion %s (new-session -s %s en %s):\n%s", ca, propia, propia, wtFix, tmux)
			}
			kill := indiceDeTmux(lineas, nueva, "kill-session", "-t", "="+propia)
			if indiceDeTmux(lineas, nueva, "split-window", "-t", "="+propia+":") < 0 || kill < 0 {
				t.Fatalf("%s: fixture: el split-window fallo y la apertura intento cerrar la sesion con kill-session -t =%s:\n%s", ca, propia, tmux)
			}
			if n := f.panes(propia); n != 1 {
				t.Fatalf("%s: fixture: con kill-session roto la sesion %s sigue viva con un solo pane, y tiene %d:\n%s", ca, propia, n, tmux)
			}
			switch c.corte {
			case "plazo", "senal":
				if indiceDeTmux(lineas, kill, "has-session", "-t", "="+propia) < 0 {
					t.Fatalf("%s: fixture: despues del kill-session fallido, la apertura sondea la sesion con has-session -t =%s:\n%s", ca, propia, tmux)
				}
				if c.corte == "plazo" && len(h3PIDs(t, f.pids)) == 0 {
					t.Fatalf("%s: fixture: el has-session se colgo:\n%s", ca, tmux)
				}
			case "sin tmux":
				if _, err := os.Stat(yo); !os.IsNotExist(err) {
					t.Fatalf("%s: fixture: el kill-session fallido se llevo tmux del PATH (%s): %v", ca, yo, err)
				}
			}

			if !a.fallo() {
				t.Fatalf("%s: con el pane de vigilancia y el cierre rotos, la apertura responde un error: %v\ntmux:\n%s", ca, a, tmux)
			}
			msg := a.mensaje()
			bajo := hb5SinTildes(msg)
			var mal []string
			if strings.Contains(bajo, "ya no existe") {
				mal = append(mal, "el sondeo no confirmo nada (tmux no salio con codigo 1): no dice que la sesion ya no existe")
			}
			aviso := "tmux kill-session -t '=" + propia + "'"
			if !contieneElAviso(msg, aviso) {
				mal = append(mal, "muestra como cerrarla a mano, con el destino entre comillas: "+aviso)
			}
			incierto := false
			for _, frase := range hb5Incierto {
				if strings.Contains(bajo, frase) {
					incierto = true
					break
				}
			}
			if !incierto {
				mal = append(mal, "dice que no se pudo saber si la sesion sigue abierta")
			}
			if strings.Contains(bajo, "la cerre") {
				mal = append(mal, "no dice que la cerro: kill-session fallo")
			}
			if len(mal) > 0 {
				t.Errorf("%s: el error no dice la verdad sobre la sesion:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, tmux)
			}
			cierreEjecutable(t, ca, msg, propia)
			if _, raw := atLeerItem(t, dir, "fix"); !bytes.Equal(raw, itemFix) {
				t.Errorf("%s: el item de fix no declara una sesion que no quedo armada:\n%s", ca, raw)
			}
			for _, pid := range h3PIDs(t, f.pids) {
				if !h3EsperarHasta(3*time.Second, func() bool { return !h3VivoPID(pid) }) {
					t.Errorf("%s: el has-session colgado (pid %d) sigue vivo despues de la respuesta", ca, pid)
				}
			}
		})
	}
}
