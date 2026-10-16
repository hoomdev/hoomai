// Tests de los hallazgos db8541 y f7efdc sobre Abrir sesion, por el Studio y
// por la CLI, escritos desde .hoom/specs/acciones-desde-la-tarjeta.md (CA-347,
// CA-348):
//
//   - db8541, ENMIENDA 5: "el comando que los errores proponen para cerrar una
//     sesion a mano lleva el destino entre comillas, `tmux kill-session -t
//     '=<session>'`, porque zsh expande una palabra que empieza con `=` como
//     la ruta de un comando". cierreEjecutable extrae del mensaje el comando
//     que propone y lo corre, tal cual, con sh -c y con zsh -c (si zsh esta
//     instalado; en el CI de Linux no lo esta y esa parte se salta) contra un
//     tmux falso que anota sus argumentos: tmux tiene que recibir `-t` con
//     exactamente `=<session>`. Lo usa el test de 0fc76d en
//     hallazgos_c3_vuelta3_test.go. Es la misma comprobacion que la de
//     internal/cockpitcmd/hallazgos_db8541_f7efdc_test.go: los helpers de test
//     no se comparten entre paquetes.
//   - f7efdc (CA-347, CA-348): la sesion que Abrir sesion acaba de crear muere
//     antes del pane de vigilancia (la CLI de la IA sale al arrancar), el
//     split-window falla y cerrarla tambien, porque ya no hay sesion. El error
//     no dice que la sesion quedo abierta ni pide cerrarla a mano: dice que ya
//     no existe y que se puede reintentar, y el reintento la crea entera y la
//     registra.
//
// El tmux falso (tmuxFalso, con destinosComoTmux y la marca morir) y las
// aperturas (abrirSesion) estan en tmuxfalso_test.go. Fixtures de
// acciones_tarjeta_test.go (atPATH, atConEspacio, atLineas, atLeerItem) y
// tablero_test.go (tbProyecto). tmux y el CLI de IA son falsos.
package servecmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cockpitcmd"
)

// dbPATH es el PATH con el que arranco el binario de tests, antes de que
// atPATH lo reemplace por uno con solo los falsos: ahi se buscan sh y zsh.
var dbPATH = os.Getenv("PATH")

// dbBuscar busca el ejecutable nombre en dbPATH ("" si no esta).
func dbBuscar(nombre string) string {
	for _, d := range filepath.SplitList(dbPATH) {
		if d == "" {
			continue
		}
		p := filepath.Join(d, nombre)
		if st, err := os.Stat(p); err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return p
		}
	}
	return ""
}

// comandoDeCierre extrae de msg el comando que propone para cerrar la sesion
// a mano, como lo copiaria una persona: desde `tmux kill-session` hasta el fin
// de la palabra del destino de -t. La palabra es un tramo entre comillas
// simples o dobles, o un tramo suelto de caracteres de nombre (letras,
// digitos, `-`, `_`, `=`, `%` y escapes con `\`). "" si msg no propone ninguno.
func comandoDeCierre(msg string) string {
	const verbo = "tmux kill-session"
	i := strings.Index(msg, verbo)
	if i < 0 {
		return ""
	}
	pos := i + len(verbo)
	espacios := func() int {
		n := 0
		for pos < len(msg) && msg[pos] == ' ' {
			pos++
			n++
		}
		return n
	}
	if espacios() == 0 || !strings.HasPrefix(msg[pos:], "-t") {
		return ""
	}
	pos += len("-t")
	if espacios() == 0 || pos >= len(msg) {
		return ""
	}
	inicio := pos
	if c := msg[pos]; c == '\'' || c == '"' {
		j := strings.IndexByte(msg[pos+1:], c)
		if j < 0 {
			return ""
		}
		pos += j + 2
	} else {
	suelta:
		for pos < len(msg) {
			c := msg[pos]
			switch {
			case c == '\\' && pos+1 < len(msg):
				pos += 2
			case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_', c == '=', c == '%':
				pos++
			default:
				break suelta
			}
		}
	}
	if pos == inicio {
		return ""
	}
	return msg[i:pos]
}

// cierreEjecutable exige el contrato de db8541 (enmienda 5): el comando que
// msg propone para cerrar la sesion a mano, copiado tal cual, corre en sh y en
// zsh (si esta instalado; si no, esa parte se salta) y tmux recibe
// `kill-session -t =<sesion>`, con `=<sesion>` exacto como argumento de -t.
// El tmux es un falso que anota sus argumentos, uno por linea.
func cierreEjecutable(t *testing.T, ca, msg, sesion string) {
	t.Helper()
	cmd := comandoDeCierre(msg)
	if cmd == "" {
		t.Errorf("%s: hallazgo db8541 (enmienda 5): el error propone un comando `tmux kill-session -t '=%s'` para cerrar la sesion a mano: %q", ca, sesion, msg)
		return
	}
	quiere := []string{"kill-session", "-t", "=" + sesion}
	for _, sh := range []struct{ nombre, flags string }{{"sh", ""}, {"zsh", "-f"}} {
		t.Run(sh.nombre, func(t *testing.T) {
			bin := dbBuscar(sh.nombre)
			if bin == "" {
				t.Skipf("%s: %s no esta instalado: el comando propuesto no se prueba en %s", ca, sh.nombre, sh.nombre)
			}
			dir := t.TempDir()
			args := filepath.Join(dir, "args")
			falso := "#!/bin/sh\n: > '" + args + "'\nfor a in \"$@\"; do printf '%s\\n' \"$a\" >> '" + args + "'; done\nexit 0\n"
			if err := os.WriteFile(filepath.Join(dir, "tmux"), []byte(falso), 0o755); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			argv := []string{"-c", cmd}
			if sh.flags != "" {
				argv = append([]string{sh.flags}, argv...)
			}
			c := exec.CommandContext(ctx, bin, argv...)
			var env []string
			for _, kv := range os.Environ() {
				if !strings.HasPrefix(kv, "PATH=") {
					env = append(env, kv)
				}
			}
			c.Env = append(env, "PATH="+dir+string(os.PathListSeparator)+"/usr/bin"+string(os.PathListSeparator)+"/bin")
			c.Dir = dir
			out, err := c.CombinedOutput()
			raw, lerr := os.ReadFile(args)
			if lerr != nil {
				t.Fatalf("%s: hallazgo db8541 (enmienda 5): el comando que propone el error, `%s`, copiado tal cual en %s, ni llega a tmux (zsh expande una palabra que empieza con `=`; el destino va entre comillas): %v\n%s", ca, cmd, sh.nombre, err, out)
			}
			got := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
			if err != nil || !reflect.DeepEqual(got, quiere) {
				t.Fatalf("%s: hallazgo db8541 (enmienda 5): el comando que propone el error, `%s`, en %s le pasa a tmux %q (quiere %q), salida %v:\n%s", ca, cmd, sh.nombre, got, quiere, err, out)
			}
		})
	}
}

// Hallazgo f7efdc (CA-347, CA-348), de punta a punta. Con el tmux falso en
// destinosComoTmux y la marca morir, la sesion de la tarjeta fix que Abrir
// sesion acaba de crear muere enseguida: el pane de vigilancia (split-window
// -t =<sesion>:) falla con "can't find session" y cerrarla (kill-session) falla
// igual, porque ya no hay sesion. Desde el Studio y desde la CLI
// (cockpitcmd.Open): la apertura responde un error que NO dice que la sesion
// quedo abierta ni pide cerrarla a mano (ningun `kill-session -t` propuesto,
// nada "a mano"), y SI dice que ya no existe y que se puede reintentar; el
// item queda byte a byte igual. El reintento con tmux sano crea la sesion
// (created true, dos panes) y la declara en el item.
func TestCA347_CA348_HallazgoF7efdc_AbrirSesionQueMurioNoPideCerrarlaAMano(t *testing.T) {
	for _, quien := range []string{"studio", "cli"} {
		t.Run(quien, func(t *testing.T) {
			ca := "f7efdc (CA-347, CA-348), desde " + quien
			f := nuevoTmuxFalso(t)
			f.destinosComoTmux = true
			atPATH(t, map[string]string{"tmux": f.script(), "claude": "exit 0\n"})
			dir := tbProyecto(t)
			wtFix := atConEspacio(t, dir, "fix")
			s := newServer(t, dir)
			propia := cockpitcmd.SessionName("demo", "fix")
			_, itemFix := atLeerItem(t, dir, "fix")

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
			if f.viva(propia) {
				t.Fatalf("%s: fixture: la sesion %s murio al crearse:\ntmux:\n%s", ca, propia, tmux)
			}
			if !a.fallo() {
				t.Fatalf("%s: la sesion de fix no existe: la apertura responde un error: %v\ntmux:\n%s", ca, a, tmux)
			}
			msg := a.mensaje()
			bajo := strings.ToLower(msg)
			var mal []string
			for _, no := range []string{"quedo abierta", "quedó abierta"} {
				if strings.Contains(bajo, no) {
					mal = append(mal, "no dice que la sesion "+no+": ya no existe")
				}
			}
			for _, no := range []string{"kill-session -t", "a mano", "cerrala", "ciérrala"} {
				if strings.Contains(bajo, no) {
					mal = append(mal, fmt.Sprintf("no pide cerrarla a mano (contiene %q): no hay nada que cerrar", no))
				}
			}
			if !strings.Contains(bajo, "ya no existe") {
				mal = append(mal, "dice que la sesion ya no existe")
			}
			if !strings.Contains(bajo, "reintent") {
				mal = append(mal, "dice que se puede reintentar")
			}
			if len(mal) > 0 {
				t.Errorf("%s: la sesion murio y no se pudo cerrar porque ya no existe; el error:\n- %s\nerror: %q\ntmux:\n%s", ca, strings.Join(mal, "\n- "), msg, tmux)
			}
			if _, raw := atLeerItem(t, dir, "fix"); !bytes.Equal(raw, itemFix) {
				t.Fatalf("%s: el item de fix no declara una sesion que no existe:\n%s", ca, raw)
			}

			// el reintento, con tmux sano, crea la sesion entera y la declara
			if err := os.Remove(f.morir); err != nil {
				t.Fatal(err)
			}
			desde := len(lineas)
			b := abrirSesion(s, dir, "fix", quien)
			tmux = strings.Join(atLineas(t, f.log)[desde:], "\n")
			if !b.creo(propia) || f.panes(propia) != 2 {
				t.Fatalf("%s: se puede reintentar: con tmux sano, abrir fix crea su sesion %s con dos panes (%d): %v\ntmux:\n%s", ca, propia, f.panes(propia), b, tmux)
			}
			if it, crudo := atLeerItem(t, dir, "fix"); len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
				t.Fatalf("%s: el reintento declara la sesion creada en el item (una entrada, claude):\n%s", ca, crudo)
			}
		})
	}
}
