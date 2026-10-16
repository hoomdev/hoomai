// Tests de los hallazgos b59c30 y e0001e sobre Abrir sesion, de punta a punta
// (POST /api/board/<slug>/session del Studio y cockpitcmd.Open de la CLI), con
// el contrato nuevo de los hallazgos 9c1816, 57bd29 y 03481c (decision de
// Henry), escritos desde .hoom/specs/acciones-desde-la-tarjeta.md (CA-347,
// CA-348):
//
// Abrir sesion crea la sesion, el pane de vigilancia (split-window) falla y el
// cierre (`tmux kill-session -t =<sesion>`) TAMBIEN falla, con codigo 1. tmux
// sale con codigo 1 tanto si la sesion no esta como si falla su propio
// cliente (9c1816), asi que ni el kill-session ni un sondeo `tmux has-session`
// despues prueban si la sesion sigue abierta. El error es UNO SOLO, este la
// sesion viva, muerta o sin saberse, y responda lo que responda tmux a
// has-session (o aunque no se le pregunte): no tiene incumplimientos para
// incumplimientosDelCierreFallido (en tmuxfalso_test.go: no dice que la
// cerro, no afirma que la sesion ya no existe ni que quedo abierta, dice que
// no se pudo saber si sigue abierta y muestra `tmux kill-session -t
// '=<sesion>'` con comillas, que corre tal cual en sh y en zsh:
// cierreEjecutable). El item queda byte a byte igual.
//
// Reemplaza al contrato de tres estados (has-session con codigo 1 => "ya no
// existe"; sale bien => "quedo abierta"; no confirma nada => "no se sabe"):
// con el cliente de tmux roto (codigo 1 y la sesion VIVA) decia "ya no
// existe" de una sesion viva y escondia el comando para cerrarla. No es
// debilitar: el mensaje nuevo es verdadero en todos los casos, cosa que el de
// tres estados no lograba.
//
// El tmux es el falso con estado de tmuxfalso_test.go (con destinosComoTmux),
// envuelto por un script que rompe split-window y kill-session y cambia lo que
// responde has-session despues del kill-session. Fixtures de
// acciones_tarjeta_test.go (atPATH, atFalso, atConEspacio, atLineas,
// atLeerItem), tablero_test.go (tbProyecto), hallazgos_c3_test.go (h3PIDs,
// h3VivoPID, h3EsperarHasta), hallazgos_c3_vuelta3_test.go (contieneElAviso),
// hallazgos_c3_db8541_f7efdc_test.go (cierreEjecutable) y el verificador
// compartido internal/cierretmuxtest (DiceQueLaCerro, DiceQueNoSabe; hallazgo
// 9accd8). Ni tmux ni el CLI de IA son de verdad.
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

	"github.com/hoomdev/hoomai/internal/cierretmuxtest"
	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// hb5Envoltorio es el tmux que ve Abrir sesion: split-window falla (la sesion
// queda viva con un solo pane), kill-session corre @ALCERRAR@, deja la marca
// @CERRO@ y falla con codigo 1 ("lost server") sin cerrar nada, y desde esa
// marca has-session (un sondeo, si la apertura lo hace; no debe: e5f706) corre
// @SONDEO@. Lo demas, y lo que @SONDEO@ deja pasar, lo atiende el tmux falso
// con estado (@BASE@). Lo que atiende el envoltorio lo anota en el mismo log,
// antes de correr nada.
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

// Hallazgos b59c30, e0001e y 9c1816 (CA-347, CA-348), de punta a punta: Abrir
// sesion de la tarjeta fix crea la sesion, el split-window falla y el
// kill-session falla con codigo 1. Despues, segun el caso:
//
//   - la sesion sigue viva con un solo pane, y has-session se cuelga para
//     siempre, sale por una senal (SIGKILL), no arranca (el kill-session
//     fallido se lleva el tmux del PATH), sale bien, o sale con codigo 1
//     porque falla el cliente (9c1816);
//   - la sesion murio con el kill-session (el servidor se cayo), y has-session
//     sale con codigo 1 ("can't find session").
//
// Desde el Studio y desde la CLI, la apertura responde un error sin
// incumplimientos para incumplimientosDelCierreFallido, con el comando de
// cierre que corre tal cual en sh y en zsh, y el item de fix queda byte a
// byte igual. Y para cada quien, el mensaje es el MISMO en todos los casos:
// no depende de has-session ni de si la sesion vive.
//
// Hallazgo e5f706 (CA-347, CA-348): en todas las filas, despues del
// kill-session fallido el log del tmux falso no tiene ningun has-session ni
// ninguna otra llamada (un tmux que no pudo cerrar la sesion puede estar
// degradado). En la fila "sin tmux" un sondeo no dejaria rastro en el log
// (tmux ya no esta en el PATH); las demas lo ven. Y con has-session colgado
// para siempre, desde el Studio y desde la CLI: la respuesta llega antes de
// hb5Limite (10 s; la CLI no pone plazo, asi que un sondeo la colgaria) y
// antes del plazo de 5 s de cada llamada a tmux del Studio (no espero a
// has-session), ningun has-session llego a colgarse (no anoto su PID) y no
// queda ningun tmux colgado. La fila colgada reemplaza a la de "el has-session
// del Studio vence su plazo de 5 s", que toleraba 15 s y solo miraba el
// colgado si habia dejado un PID.
//
// Reemplaza a TestCA347_CA348_HallazgosB59c30E0001e_AbrirSesionConUnSondeoQueNoConfirmaNada:
// el contrato de tres estados quedo reemplazado por decision de Henry
// (hallazgos 9c1816, 57bd29 y 03481c). No es debilitar: el mensaje nuevo es
// verdadero en todos los casos, cosa que el anterior no lograba.
func TestCA347_CA348_Hallazgo9c1816_AbrirSesionConElCierreFallidoDiceQueNoSabeSiSigueAbierta(t *testing.T) {
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
		{"la sesion murio con el kill-session y el has-session del Studio sale con codigo 1", "studio", "murio"},
		{"la sesion murio con el kill-session y el has-session de la CLI sale con codigo 1", "cli", "murio"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			ca := "9c1816, b59c30/e0001e (CA-347, CA-348): " + c.caso
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
			alCerrar, sondeo := ":", ":"
			switch c.corte {
			case "cuelga":
				sondeo = `printf '%s\n' "$$" >> '` + f.pids + `'; exec /bin/sleep 3600`
			case "senal":
				sondeo = `kill -9 $$`
			case "sin tmux":
				alCerrar = `/bin/rm -f '` + yo + `'`
			case "codigo 1":
				sondeo = `printf 'lost server\n' >&2; exit 1`
			case "murio":
				alCerrar = `: > '` + filepath.Join(f.ses, propia) + `'`
			}
			atFalso(t, bin, "tmux", strings.NewReplacer("@LOG@", f.log, "@CERRO@", filepath.Join(m, "cerro"),
				"@ALCERRAR@", alCerrar, "@SONDEO@", sondeo, "@BASE@", base).Replace(hb5Envoltorio))
			dir := tbProyecto(t)
			wtFix := atConEspacio(t, dir, "fix")
			s := newServer(t, dir)
			_, itemFix := atLeerItem(t, dir, "fix")

			var a apertura
			inicio := time.Now()
			select {
			case a = <-lanzarApertura(s, dir, "fix", c.quien):
			case <-time.After(hb5Limite):
				t.Fatalf("%s: e5f706: Abrir sesion con el cierre roto se quedo colgado mas de %v (la CLI no le pone plazo a tmux: no puede volver a llamarlo despues del kill-session fallido):\n%s", ca, hb5Limite, strings.Join(atLineas(t, f.log), "\n"))
			}
			tardo := time.Since(inicio)
			lineas := atLineas(t, f.log)
			tmux := strings.Join(lineas, "\n")
			if a.panico != nil {
				t.Fatalf("%s: la apertura entra en panico: %v\ntmux:\n%s", ca, a.panico, tmux)
			}

			// fixture: creo la sesion, el pane de vigilancia y el cierre fallaron
			nueva := indiceDeTmux(lineas, 0, "new-session", "-s", propia)
			if nueva < 0 || !strings.Contains(lineas[nueva], wtFix) {
				t.Fatalf("%s: fixture: la apertura crea la sesion %s (new-session -s %s en %s):\n%s", ca, propia, propia, wtFix, tmux)
			}
			kill := indiceDeTmux(lineas, nueva, "kill-session", "-t", "="+propia)
			if indiceDeTmux(lineas, nueva, "split-window", "-t", "="+propia+":") < 0 || kill < 0 {
				t.Fatalf("%s: fixture: el split-window fallo y la apertura intento cerrar la sesion con kill-session -t =%s:\n%s", ca, propia, tmux)
			}
			quiere := 1 // la sesion sigue viva a medias
			if c.corte == "murio" {
				quiere = 0
			}
			if n := f.panes(propia); n != quiere {
				t.Fatalf("%s: fixture: despues del kill-session fallido la sesion %s tiene %d panes (quiere %d):\n%s", ca, propia, n, quiere, tmux)
			}

			// e5f706: despues del kill-session fallido, ni un sondeo ni otra
			// llamada a tmux
			var hs []string
			for _, l := range lineas[kill+1:] {
				if campos := strings.Fields(l); len(campos) > 0 && campos[0] == "has-session" {
					hs = append(hs, l)
				}
			}
			if len(hs) > 0 {
				t.Errorf("%s: e5f706: despues del kill-session fallido la apertura no llama a has-session (el resultado no prueba nada y tmux puede estar colgado), y hubo %d: %q\ntmux:\n%s", ca, len(hs), hs, tmux)
			}
			if tras := lineas[kill+1:]; len(tras) > 0 {
				t.Errorf("%s: e5f706: despues del kill-session fallido la apertura no le vuelve a hablar a tmux, y le hizo %d llamadas: %q\ntmux:\n%s", ca, len(tras), tras, tmux)
			}
			if c.corte == "cuelga" {
				if tardo >= hb5PlazoDeTmux {
					t.Errorf("%s: e5f706: con has-session colgado, %s responde sin esperarlo: tardo %v (el plazo de una llamada a tmux del Studio es %v)", ca, a.quien, tardo, hb5PlazoDeTmux)
				}
				if pids := h3PIDs(t, f.pids); len(pids) > 0 {
					t.Errorf("%s: e5f706: ningun has-session llega a colgarse, y se colgaron %d (pids %v)", ca, len(pids), pids)
				}
			}
			if c.corte == "sin tmux" {
				if _, err := os.Stat(yo); !os.IsNotExist(err) {
					t.Fatalf("%s: fixture: el kill-session fallido se llevo tmux del PATH (%s): %v", ca, yo, err)
				}
			}

			if !a.fallo() {
				t.Fatalf("%s: con el pane de vigilancia y el cierre rotos, la apertura responde un error: %v\ntmux:\n%s", ca, a, tmux)
			}
			msg := a.mensaje()
			mensajes[c.quien] = append(mensajes[c.quien], visto{c.caso, hb5Comparable(msg, wtFix, dir)})
			if mal := incumplimientosDelCierreFallido(msg, propia); len(mal) > 0 {
				t.Errorf("%s: el cierre fallo y tmux no prueba si la sesion sigue abierta; el error:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, tmux)
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
			t.Errorf("CA-347, CA-348: 9c1816: desde %s: con el mismo split-window y el mismo kill-session fallidos, el error es UNO SOLO, responda lo que responda tmux a has-session y este la sesion viva o muerta, y hay %d distintos:%s", q, len(distintos), b.String())
		}
	}
}

// hb5SoloSplitRoto es el tmux que ve Abrir sesion en la guarda de C1:
// split-window falla y todo lo demas (kill-session tambien, que sale bien) lo
// atiende el tmux falso con estado (@BASE@).
const hb5SoloSplitRoto = `case "$1" in
split-window)
  printf '%s\n' "$*" >> '@LOG@'
  printf 'create pane failed: pane too small\n' >&2
  exit 1 ;;
esac
exec /bin/sh '@BASE@' "$@"
`

// GUARDA del contrato de C1 (hallazgos c1a9da y 363285), que el contrato
// nuevo no cambia, de punta a punta: con el split-window roto y kill-session
// SANO, Abrir sesion (desde el Studio y desde la CLI) cierra la sesion que
// acaba de crear y responde un error que dice que la cerro, para que el
// proximo intento la arme entera, y no dice que no se pudo saber si sigue
// abierta. El item de fix queda byte a byte igual.
func TestCA347_CA348_Hallazgo9c1816_GuardaAbrirSesionConElCierreSanoDiceQueLaCerro(t *testing.T) {
	for _, quien := range []string{"studio", "cli"} {
		t.Run(quien, func(t *testing.T) {
			ca := "CA-347, CA-348: guarda de C1 (kill-session sano), desde " + quien
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
			tmux := strings.Join(atLineas(t, f.log), "\n")
			if a.panico != nil {
				t.Fatalf("%s: la apertura entra en panico: %v\ntmux:\n%s", ca, a.panico, tmux)
			}
			if len(f.cerradas(t, propia)) == 0 || f.viva(propia) {
				t.Fatalf("%s: fixture: la apertura creo la sesion %s y la cerro con kill-session:\n%s", ca, propia, tmux)
			}
			if !a.fallo() {
				t.Fatalf("%s: con el split-window roto la apertura responde un error: %v\ntmux:\n%s", ca, a, tmux)
			}
			msg := a.mensaje()
			if !cierretmuxtest.DiceQueLaCerro(msg) {
				t.Errorf("%s: kill-session salio bien: el error dice que cerro la sesion, para que el proximo intento la arme entera: %q", ca, msg)
			}
			if cierretmuxtest.DiceQueNoSabe(msg) {
				t.Errorf("%s: kill-session salio bien: el error no dice que no se pudo saber si la sesion sigue abierta: %q", ca, msg)
			}
			if _, raw := atLeerItem(t, dir, "fix"); !bytes.Equal(raw, itemFix) {
				t.Errorf("%s: el item de fix no declara una sesion que no quedo armada:\n%s", ca, raw)
			}
		})
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
