// Tests de los hallazgos b59c30, e0001e, 9c1816 y e5f706 sobre Abrir sesion,
// de punta a punta (POST /api/board/<slug>/session del Studio y
// cockpitcmd.Open de la CLI), escritos desde
// .hoom/specs/acciones-desde-la-tarjeta.md (CA-347, CA-348), con la ENMIENDA 6
// (decision de Henry, hallazgo bde7e4):
//
// Abrir sesion crea la sesion y el pane de vigilancia (split-window) falla.
// Con la enmienda 6 la apertura NO la cierra: no corre ningun kill-session
// (cerrarla por su nombre no prueba que sea la que creo: pudo morir y otra
// apertura crear una con el mismo nombre), responde el error, que dice como
// cerrarla a mano con `tmux kill-session -t '=<sesion>'` (lo verifica
// incumplimientosEnmienda6, en tmuxfalso_test.go: no dice que la cerro y
// trae ese comando con el nombre exacto de la sesion; que corra tal cual en
// sh y en zsh lo exige cierreEjecutable), y la sesion cuenta como existente.
// El item queda byte a byte igual.
//
// Lo que sigue del contrato de 9c1816 y e5f706: el error es UNO SOLO, este la
// sesion viva o muerta, y responda lo que responda tmux a has-session (tmux
// sale con codigo 1 tanto si la sesion no esta como si falla su propio
// cliente): y no se le pregunta. Despues del split-window fallido la apertura
// no le vuelve a hablar a tmux.
//
// Antes (contrato de 9c1816, 57bd29 y 03481c) la apertura intentaba cerrar la
// sesion, el doble rompia ese kill-session y el error tenia que decir que no
// se pudo saber si sigue abierta. La enmienda 6 saco el cierre: "no se si
// sigue abierta" era la respuesta a un cierre fallido que ya no existe. No es
// debilitar: ahora se exige no tocar una sesion que la apertura no puede
// probar suya, y el mensaje unico se sigue exigiendo.
//
// El tmux es el falso con estado de tmuxfalso_test.go (con destinosComoTmux),
// envuelto por un script que rompe split-window y cambia lo que responde
// has-session despues. Fixtures de acciones_tarjeta_test.go (atPATH, atFalso,
// atConEspacio, atLineas, atConVerbo, atLeerItem), tablero_test.go
// (tbProyecto), hallazgos_c3_test.go (h3PIDs, h3VivoPID, h3EsperarHasta),
// hallazgos_c3_vuelta3_test.go (contieneElAviso),
// hallazgos_c3_db8541_f7efdc_test.go (cierreEjecutable) y el verificador
// compartido internal/cierretmuxtest (DiceQueLaCerro, hallazgo 9accd8). Ni
// tmux ni el CLI de IA son de verdad.
package servecmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// hb5Envoltorio es el tmux que ve Abrir sesion: split-window falla (la sesion
// queda viva con un solo pane), corre @ALFALLAR@ y deja la marca @FALLO@, y
// desde esa marca has-session (un sondeo, si la apertura lo hace; no debe:
// e5f706) corre @SONDEO@. Lo demas (kill-session tambien, que cierra la
// sesion como tmux sano: con la enmienda 6 la apertura no debe correrlo), y
// lo que @SONDEO@ deja pasar, lo atiende el tmux falso con estado (@BASE@).
// Lo que atiende el envoltorio lo anota en el mismo log, antes de correr
// nada.
const hb5Envoltorio = `case "$1" in
split-window)
  printf '%s\n' "$*" >> '@LOG@'
  : > '@FALLO@'
  @ALFALLAR@
  printf 'create pane failed: pane too small\n' >&2
  exit 1 ;;
has-session)
  if [ -f '@FALLO@' ]; then
    printf '%s\n' "$*" >> '@LOG@'
    @SONDEO@
  fi ;;
esac
exec /bin/sh '@BASE@' "$@"
`

// hb5Limite es cuanto espera el test la respuesta de una apertura: la CLI no
// le pone plazo a tmux, asi que con un tmux colgado solo este limite externo
// la corta (e5f706).
const hb5Limite = 10 * time.Second

// hb5PlazoDeTmux es el plazo de cada llamada a tmux del Studio (enmienda 1,
// (k)). Con has-session colgado para siempre, una respuesta que tarda eso o
// mas es la de una apertura que espero a has-session (e5f706).
const hb5PlazoDeTmux = 5 * time.Second

// hb5Comparable es msg sin lo que cambia de un subtest a otro aunque el
// mensaje sea el mismo: las carpetas temporales del proyecto y del espacio de
// trabajo (tambien con los enlaces resueltos: en macOS /var es /private/var).
func hb5Comparable(msg string, carpetas ...string) string {
	for _, c := range carpetas {
		if real, err := filepath.EvalSymlinks(c); err == nil {
			msg = strings.ReplaceAll(msg, real, "<"+filepath.Base(c)+">")
		}
		msg = strings.ReplaceAll(msg, c, "<"+filepath.Base(c)+">")
	}
	return msg
}

// Hallazgos b59c30, e0001e, 9c1816 y e5f706 (CA-347, CA-348), de punta a
// punta, con la ENMIENDA 6: Abrir sesion de la tarjeta fix crea la sesion y
// el split-window falla. Despues, segun el caso:
//
//   - la sesion sigue viva con un solo pane, y has-session se cuelga para
//     siempre, sale por una senal (SIGKILL), no arranca (el split-window
//     fallido se lleva el tmux del PATH), sale bien, o sale con codigo 1
//     porque falla el cliente (9c1816);
//   - la sesion murio con el split-window (la CLI de la IA salio), y
//     has-session saldria con codigo 1 ("can't find session").
//
// Desde el Studio y desde la CLI: la apertura responde un error y no corre
// ningun kill-session ni kill-pane (f.cierres vacio: ni en el log ni en las
// sesiones cerradas), asi que la sesion viva sigue viva con su unico pane; el
// error no tiene incumplimientos para incumplimientosEnmienda6 (no dice que
// la cerro y trae `tmux kill-session -t '=<sesion>'` con el nombre exacto), el
// comando corre tal cual en sh y en zsh, y el item de fix queda byte a byte
// igual. Y para cada quien, el mensaje es el MISMO en todos los casos: no
// depende de has-session ni de si la sesion vive.
//
// Hallazgo e5f706: en todas las filas, despues del split-window fallido el log
// del tmux falso no tiene ningun has-session ni ninguna otra llamada (un tmux
// que fallo puede estar degradado). En la fila "sin tmux" ni un sondeo ni un
// kill-session dejarian rastro en el log (tmux ya no esta en el PATH); las
// demas los ven. Y con has-session colgado para siempre, desde el Studio y
// desde la CLI: la respuesta llega antes de hb5Limite (10 s; la CLI no pone
// plazo, asi que un sondeo la colgaria) y antes del plazo de 5 s de cada
// llamada a tmux del Studio, ningun has-session llego a colgarse (no anoto su
// PID) y no queda ningun tmux colgado.
//
// Reemplaza a TestCA347_CA348_Hallazgo9c1816_AbrirSesionConElCierreFallidoDiceQueNoSabeSiSigueAbierta,
// que rompia el kill-session de la apertura y exigia que el error dijera que
// no se pudo saber si la sesion sigue abierta: el contrato cambio por la
// enmienda 6 (decision de Henry, hallazgo bde7e4), que saco el cierre. No es
// debilitar: ahora se exige no tocar una sesion que la apertura no puede
// probar suya, y el mensaje unico se sigue exigiendo.
func TestCA347_CA348_Hallazgo9c1816_Enmienda6_AbrirSesionSinCerrarlaDiceComoCerrarlaConUnSoloMensaje(t *testing.T) {
	type visto struct{ caso, msg string }
	mensajes := map[string][]visto{} // por quien
	for _, c := range []struct {
		caso, quien string
		corte       string // "cuelga", "senal", "sin tmux", "sale bien", "codigo 1" o "murio"
	}{
		{"e5f706: el has-session del Studio se cuelga para siempre", "studio", "cuelga"},
		{"e5f706: el has-session de la CLI se cuelga para siempre", "cli", "cuelga"},
		{"el has-session del Studio sale por una senal", "studio", "senal"},
		{"el has-session de la CLI sale por una senal", "cli", "senal"},
		{"el has-session del Studio no arranca: tmux ya no esta", "studio", "sin tmux"},
		{"el has-session de la CLI no arranca: tmux ya no esta", "cli", "sin tmux"},
		{"el has-session del Studio sale bien: la sesion vive", "studio", "sale bien"},
		{"el has-session de la CLI sale bien: la sesion vive", "cli", "sale bien"},
		{"9c1816: el has-session del Studio sale con codigo 1 y la sesion vive", "studio", "codigo 1"},
		{"9c1816: el has-session de la CLI sale con codigo 1 y la sesion vive", "cli", "codigo 1"},
		{"la sesion murio con el split-window y el has-session del Studio sale con codigo 1", "studio", "murio"},
		{"la sesion murio con el split-window y el has-session de la CLI sale con codigo 1", "cli", "murio"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			ca := "9c1816, b59c30/e0001e (CA-347, CA-348, enmienda 6): " + c.caso
			f := nuevoTmuxFalso(t)
			f.destinosComoTmux = true
			bin := atPATH(t, map[string]string{"claude": "exit 0\n"})
			m := t.TempDir()
			base := filepath.Join(m, "tmux-base")
			if err := os.WriteFile(base, []byte("#!/bin/sh\n"+f.script()), 0o755); err != nil {
				t.Fatal(err)
			}
			yo := filepath.Join(bin, "tmux")
			propia := cockpitcmd.SessionName("demo", "fix")
			alFallar, sondeo := ":", ":"
			switch c.corte {
			case "cuelga":
				sondeo = `printf '%s\n' "$$" >> '` + f.pids + `'; exec /bin/sleep 3600`
			case "senal":
				sondeo = `kill -9 $$`
			case "sin tmux":
				alFallar = `/bin/rm -f '` + yo + `'`
			case "codigo 1":
				sondeo = `printf 'lost server\n' >&2; exit 1`
			case "murio":
				alFallar = `: > '` + filepath.Join(f.ses, propia) + `'`
			}
			atFalso(t, bin, "tmux", strings.NewReplacer("@LOG@", f.log, "@FALLO@", filepath.Join(m, "fallo"),
				"@ALFALLAR@", alFallar, "@SONDEO@", sondeo, "@BASE@", base).Replace(hb5Envoltorio))
			dir := tbProyecto(t)
			wtFix := atConEspacio(t, dir, "fix")
			s := newServer(t, dir)
			_, itemFix := atLeerItem(t, dir, "fix")

			var a apertura
			inicio := time.Now()
			select {
			case a = <-lanzarApertura(s, dir, "fix", c.quien):
			case <-time.After(hb5Limite):
				t.Fatalf("%s: e5f706: Abrir sesion con el split-window roto se quedo colgado mas de %v (la CLI no le pone plazo a tmux: no puede volver a llamarlo despues del split-window fallido):\n%s", ca, hb5Limite, strings.Join(atLineas(t, f.log), "\n"))
			}
			tardo := time.Since(inicio)
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if a.panico != nil {
				t.Fatalf("%s: la apertura entra en panico: %v\ntmux:\n%s", ca, a.panico, tmux)
			}

			// fixture: creo la sesion y el pane de vigilancia fallo
			nueva := indiceDeTmux(lineas, 0, "new-session", "-s", propia)
			if nueva < 0 || !strings.Contains(lineas[nueva], wtFix) {
				t.Fatalf("%s: fixture: la apertura crea la sesion %s (new-session -s %s en %s):\n%s", ca, propia, propia, wtFix, tmux)
			}
			sw := indiceDeTmux(lineas, nueva, "split-window", "-t", "="+propia+":")
			if sw < 0 {
				t.Fatalf("%s: fixture: la apertura llego al split-window -t =%s: que fallo:\n%s", ca, propia, tmux)
			}
			if c.corte == "sin tmux" {
				if _, err := os.Stat(yo); !os.IsNotExist(err) {
					t.Fatalf("%s: fixture: el split-window fallido se llevo tmux del PATH (%s): %v", ca, yo, err)
				}
			}

			// enmienda 6: ningun cierre, y la sesion viva sigue como quedo
			if k := f.cierres(t, propia); len(k) > 0 {
				t.Errorf("%s: enmienda 6 (bde7e4): la apertura no cierra la sesion que creo (cerrarla por su nombre puede matar otra con el mismo nombre), y hubo:\n%s\ntmux:\n%s", ca, strings.Join(k, "\n"), tmux)
			}
			quiere := 1 // la sesion sigue viva a medias
			if c.corte == "murio" {
				quiere = 0
			}
			if n := f.panes(propia); n != quiere {
				t.Errorf("%s: despues del split-window fallido la sesion %s tiene %d panes (quiere %d: la apertura no la toca):\n%s", ca, propia, n, quiere, tmux)
			}

			// e5f706: despues del split-window fallido, ni un sondeo ni otra
			// llamada a tmux
			if hs := atConVerbo(lineas[sw+1:], "has-session"); len(hs) > 0 {
				t.Errorf("%s: e5f706: despues del split-window fallido la apertura no llama a has-session (el resultado no prueba nada y tmux puede estar colgado), y hubo %d: %q\ntmux:\n%s", ca, len(hs), hs, tmux)
			}
			if tras := lineas[sw+1:]; len(tras) > 0 {
				t.Errorf("%s: e5f706: despues del split-window fallido la apertura no le vuelve a hablar a tmux, y le hizo %d llamadas: %q\ntmux:\n%s", ca, len(tras), tras, tmux)
			}
			if c.corte == "cuelga" {
				if tardo >= hb5PlazoDeTmux {
					t.Errorf("%s: e5f706: con has-session colgado, %s responde sin esperarlo: tardo %v (el plazo de una llamada a tmux del Studio es %v)", ca, a.quien, tardo, hb5PlazoDeTmux)
				}
				if pids := h3PIDs(t, f.pids); len(pids) > 0 {
					t.Errorf("%s: e5f706: ningun has-session llega a colgarse, y se colgaron %d (pids %v)", ca, len(pids), pids)
				}
			}

			if !a.fallo() {
				t.Fatalf("%s: con el pane de vigilancia roto, la apertura responde un error: %v\ntmux:\n%s", ca, a, tmux)
			}
			msg := a.mensaje()
			mensajes[c.quien] = append(mensajes[c.quien], visto{c.caso, hb5Comparable(msg, wtFix, dir)})
			if mal := incumplimientosEnmienda6(msg, propia); len(mal) > 0 {
				t.Errorf("%s: enmienda 6: el error:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, tmux)
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

	// el mensaje no depende de has-session ni de si la sesion vive
	quienes := make([]string, 0, len(mensajes))
	for q := range mensajes {
		quienes = append(quienes, q)
	}
	sort.Strings(quienes)
	for _, q := range quienes {
		vs := mensajes[q]
		distintos := map[string]bool{}
		for _, v := range vs {
			distintos[v.msg] = true
		}
		if len(distintos) > 1 {
			var b strings.Builder
			for _, v := range vs {
				fmt.Fprintf(&b, "\n- %s: %q", v.caso, v.msg)
			}
			t.Errorf("CA-347, CA-348: 9c1816 (enmienda 6): desde %s: con el mismo split-window fallido, el error es UNO SOLO, responda lo que responda tmux a has-session y este la sesion viva o muerta, y hay %d distintos:%s", q, len(distintos), b.String())
		}
	}
}

// hb5SoloSplitRoto es el tmux que ve Abrir sesion en la guarda: split-window
// falla y todo lo demas (kill-session tambien, que cierra la sesion como tmux
// sano) lo atiende el tmux falso con estado (@BASE@).
const hb5SoloSplitRoto = `case "$1" in
split-window)
  printf '%s\n' "$*" >> '@LOG@'
  printf 'create pane failed: pane too small\n' >&2
  exit 1 ;;
esac
exec /bin/sh '@BASE@' "$@"
`

// GUARDA de la ENMIENDA 6 (antes, del contrato de C1, hallazgos c1a9da y
// 363285), de punta a punta: con el split-window roto y kill-session SANO
// (que cerraria la sesion si la apertura lo corriera), Abrir sesion (desde el
// Studio y desde la CLI) tampoco la cierra: ningun kill-session ni kill-pane,
// la sesion sigue viva a medias (un solo pane), y responde un error que no
// dice que la cerro y dice como cerrarla a mano con `tmux kill-session -t
// '=<sesion>'`, que corre tal cual en sh y en zsh; el item de fix queda byte
// a byte igual. El reintento, con tmux sano, la ve existente: created false
// con esa sesion (no un error), sin new-session, split-window ni ningun
// cierre; la sesion sigue con un solo pane y el item sin entradas.
//
// Reemplaza a TestCA347_CA348_Hallazgo9c1816_GuardaAbrirSesionConElCierreSanoDiceQueLaCerro,
// que exigia que la apertura la cerrara y lo dijera: el contrato cambio por la
// enmienda 6 (decision de Henry, hallazgo bde7e4). No es debilitar: ahora se
// exige no tocar una sesion que la apertura no puede probar suya, y que el
// reintento la vea existente.
func TestCA347_CA348_Hallazgo9c1816_Enmienda6_GuardaAbrirSesionConElCierreSanoTampocoLaCierra(t *testing.T) {
	for _, quien := range []string{"studio", "cli"} {
		t.Run(quien, func(t *testing.T) {
			ca := "CA-347, CA-348: guarda de la enmienda 6 (kill-session sano), desde " + quien
			f := nuevoTmuxFalso(t)
			f.destinosComoTmux = true
			bin := atPATH(t, map[string]string{"claude": "exit 0\n"})
			base := filepath.Join(t.TempDir(), "tmux-base")
			if err := os.WriteFile(base, []byte("#!/bin/sh\n"+f.script()), 0o755); err != nil {
				t.Fatal(err)
			}
			atFalso(t, bin, "tmux", strings.NewReplacer("@LOG@", f.log, "@BASE@", base).Replace(hb5SoloSplitRoto))
			dir := tbProyecto(t)
			atConEspacio(t, dir, "fix")
			s := newServer(t, dir)
			propia := cockpitcmd.SessionName("demo", "fix")
			_, itemFix := atLeerItem(t, dir, "fix")

			a := abrirSesion(s, dir, "fix", quien)
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if a.panico != nil {
				t.Fatalf("%s: la apertura entra en panico: %v\ntmux:\n%s", ca, a.panico, tmux)
			}
			if indiceDeTmux(lineas, 0, "new-session", "-s", propia) < 0 {
				t.Fatalf("%s: fixture: la apertura creo la sesion %s:\n%s", ca, propia, tmux)
			}
			if k := f.cierres(t, propia); len(k) > 0 {
				t.Errorf("%s: enmienda 6 (bde7e4): la apertura no cierra la sesion que creo, y hubo:\n%s\ntmux:\n%s", ca, strings.Join(k, "\n"), tmux)
			}
			if n := f.panes(propia); n != 1 {
				t.Errorf("%s: la sesion %s sigue viva a medias, con un solo pane (la apertura no la toca), y tiene %d:\n%s", ca, propia, n, tmux)
			}
			if !a.fallo() {
				t.Fatalf("%s: con el split-window roto la apertura responde un error: %v\ntmux:\n%s", ca, a, tmux)
			}
			msg := a.mensaje()
			if mal := incumplimientosEnmienda6(msg, propia); len(mal) > 0 {
				t.Errorf("%s: enmienda 6: el error:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, tmux)
			}
			cierreEjecutable(t, ca, msg, propia)
			if _, raw := atLeerItem(t, dir, "fix"); !bytes.Equal(raw, itemFix) {
				t.Errorf("%s: el item de fix no declara una sesion que no quedo armada:\n%s", ca, raw)
			}

			// el reintento, con tmux sano, la ve existente
			atFalso(t, bin, "tmux", f.script())
			desde := len(lineas)
			r := abrirSesion(s, dir, "fix", quien)
			despues := atLineas(t, f.log)[desde:]
			tmux = strings.Join(despues, "\n")
			if r.panico != nil {
				t.Fatalf("%s: el reintento: %v", ca, r)
			}
			if r.fallo() || !r.cedio(propia) {
				t.Errorf("%s: enmienda 6: el reintento ve la sesion %s existente: responde created false con esa sesion, no un error: %v\ntmux:\n%s", ca, propia, r, tmux)
			}
			for _, verbo := range []string{"new-session", "split-window"} {
				if v := atConVerbo(despues, verbo); len(v) > 0 {
					t.Errorf("%s: enmienda 6: el reintento no crea ni completa la sesion existente (sin %s): %q", ca, verbo, v)
				}
			}
			if k := f.cierres(t, propia); len(k) > 0 {
				t.Errorf("%s: enmienda 6: el reintento tampoco cierra la sesion: %q", ca, k)
			}
			if n := f.panes(propia); n != 1 {
				t.Errorf("%s: despues del reintento la sesion %s sigue a medias, con un solo pane, y tiene %d:\n%s", ca, propia, n, tmux)
			}
			if it, crudo := atLeerItem(t, dir, "fix"); len(it.Sesiones) != 0 {
				t.Errorf("%s: CA-348, enmienda 6: el reintento no registra nada: el item sigue sin entradas en sesiones:\n%s", ca, crudo)
			}
		})
	}
}

// Guarda del verificador de la enmienda 6 (hallazgo bde7e4), lo de servecmd:
// incumplimientosEnmienda6 acepta un error que no dice que la cerro y trae el
// comando `tmux kill-session -t '=<sesion>'` con el nombre exacto (con o sin
// negar que la cerro, como el de un new-session fallido), y rechaza, cada uno
// por su regla, el que dice que la cerro, el que trae el comando sin
// comillas y el de otra sesion cuyo nombre empieza igual. Sin esto, los tests
// de la enmienda 6 podrian pasar o fallar por el verificador y no por el
// mensaje.
func TestCA347_CA348_HallazgoBde7e4_ElVerificadorDeLaEnmienda6(t *testing.T) {
	s := cockpitcmd.SessionName("demo", "fix")
	cmd := "tmux kill-session -t '=" + s + "'"
	for _, c := range []struct {
		msg, porque string
		regla       string // "" = vale; si no, la palabra del unico incumplimiento
	}{
		{"exit status 1; si la sesion " + s + " quedo a medias (sin el pane de vigilancia), cerrala y reintenta: " + cmd, "como el error de un new-session fallido", ""},
		{"exit status 1; no cerre la sesion " + s + ": si sigue abierta, cerrala a mano con " + cmd, "niega que la cerro", ""},
		{"exit status 1; cerre la sesion " + s + " para que el proximo intento la arme entera: " + cmd, "dice que la cerro", "la cerro"},
		{"exit status 1; si quedo a medias, cerrala: tmux kill-session -t =" + s, "el comando sin comillas", cmd},
		{"exit status 1; si quedo a medias, cerrala: tmux kill-session -t '=" + s + "-login'", "el comando de otra sesion cuyo nombre empieza igual", cmd},
	} {
		mal := incumplimientosEnmienda6(c.msg, s)
		if c.regla == "" {
			if len(mal) != 0 {
				t.Errorf("bde7e4 (%s): incumplimientosEnmienda6(%q) = %q, quiere que valga", c.porque, c.msg, mal)
			}
			continue
		}
		if len(mal) != 1 || !strings.Contains(mal[0], c.regla) {
			t.Errorf("bde7e4 (%s): incumplimientosEnmienda6(%q) = %q, quiere un solo incumplimiento, el de %q", c.porque, c.msg, mal, c.regla)
		}
	}
}

// Guarda del verificador (9c1816), lo propio de servecmd: el envoltorio
// incumplimientosDelCierreFallido busca el comando de cierre con el nombre
// exacto de la sesion (contieneElAviso), asi que el de otra sesion cuyo
// nombre empieza igual no cuenta. La matriz comun de frases que acepta y
// rechaza el verificador (y la de DiceQueLaCerro) vive en el test del
// paquete compartido, internal/cierretmuxtest/mensaje_test.go (hallazgo
// 1c0773).
func TestCA347_Hallazgo9c1816_ElVerificadorDelMensaje(t *testing.T) {
	s := cockpitcmd.SessionName("demo", "fix")
	cmd := "tmux kill-session -t '=" + s + "'"
	const pre = "exit status 1; no pude saber si sigue abierta: "
	// Con el comando de esta sesion, vale.
	if msg := pre + cmd; len(incumplimientosDelCierreFallido(msg, s)) != 0 {
		t.Errorf("9c1816: incumplimientosDelCierreFallido(%q) = %q, quiere que valga", msg, incumplimientosDelCierreFallido(msg, s))
	}
	// Con el comando de otra sesion cuyo nombre empieza igual no vale, y solo
	// por el comando.
	for _, c := range []struct{ msg, porque string }{
		{pre + "tmux kill-session -t '=" + s + "-login'", "el destino entre comillas es " + s + "-login"},
		// El shell junta '=<s>'-login en =<s>-login: strings.Contains
		// encontraria el comando de esta sesion, contieneElAviso no.
		{pre + cmd + "-login", "el destino '=" + s + "'-login es " + s + "-login"},
	} {
		mal := incumplimientosDelCierreFallido(c.msg, s)
		if len(mal) != 1 || !strings.Contains(mal[0], cmd) {
			t.Errorf("9c1816 (%s): incumplimientosDelCierreFallido(%q) = %q, quiere un solo incumplimiento: que muestre %s", c.porque, c.msg, mal, cmd)
		}
	}
}
