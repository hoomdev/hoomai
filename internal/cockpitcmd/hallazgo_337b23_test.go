// Test del hallazgo 337b23 sobre cockpitcmd, con el contrato de la ENMIENDA 5
// (n) de .hoom/specs/acciones-desde-la-tarjeta.md: "los comandos de tmux que
// apuntan a la sesion de la tarjeta usan el destino exacto `=<session>:` (con
// los dos puntos), porque tmux resuelve un nombre suelto, y tambien
// `=<session>` en `list-panes`, como ventana o como prefijo de otra sesion: Ver
// terminal lista los panes con `tmux list-panes -t '=<session>:'`, y el pane de
// vigilancia se agrega con ese mismo destino". Precisa CA-350 y, por el pane de
// vigilancia que arma Open, CA-347.
//
// En tmux (visto con 3.6b) `-t hoom-demo-facturas` es esa sesion o, si no
// vive, la UNICA cuyo nombre empieza asi (hoom-demo-facturas-v2); `-t
// =hoom-demo-facturas` es exacto en has-session y kill-session, pero en
// list-panes cae en el prefijo y en split-window y select-pane falla siempre;
// `-t =hoom-demo-facturas:` es exacto en todos. Contra grabadoras: ningun tmux
// real. Fixtures de open_capture_test.go (cbGrabadora, cbProyecto) y
// hallazgos_c1_test.go (h3Tmux).
package cockpitcmd

import (
	"strings"
	"testing"
)

// Hallazgo 337b23 (CA-347, CA-350, enmienda 5): cada comando de tmux de Open
// (al crear la sesion de la tarea facturas, y cuando el split-window falla:
// con la enmienda 6 Open ya no la cierra, y si corriera un kill-session
// tambien tendria que usar el destino exacto) y de Capture que apunta a la
// sesion de la tarjeta usa un destino exacto: `=hoom-demo-facturas:` (con lo
// que siga) en todos, y en has-session y kill-session tambien vale
// `=hoom-demo-facturas`. Nunca el
// nombre suelto ni `=hoom-demo-facturas` en list-panes, split-window o
// select-pane. El pane de vigilancia (split-window) siempre lleva `-t` con ese
// destino, y Capture lista los panes con el. Un destino `%<id>` es un pane, no
// la sesion (capture-pane -t <pane>), salvo en split-window y select-pane, que
// la spec fija en `=<session>:`.
func TestCA347_CA350_Hallazgo337b23_LosComandosALaSesionUsanElDestinoExacto(t *testing.T) {
	const sesion = "hoom-demo-facturas"
	root, _ := cbProyecto(t, true)

	var llamadas []string // "tmux <args>", de todos los canales
	agregar := func(c []call, o [][]string) {
		for _, x := range c {
			if x.name == "tmux" {
				llamadas = append(llamadas, "tmux "+x.args)
			}
		}
		for _, x := range o {
			if len(x) > 0 && x[0] == "tmux" {
				llamadas = append(llamadas, strings.Join(x, " "))
			}
		}
	}

	// Open crea la sesion (has-session no la encuentra) y arma el pane de vigilancia
	g := &cbGrabadora{}
	if _, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, g.deps(nil)); err != nil {
		t.Fatalf("CA-347: Open: %v", err)
	}
	agregar(g.calls, g.outputs)
	desdeOpen := len(llamadas)

	// Open con el split-window roto: con la enmienda 6 no cierra la sesion que
	// acaba de crear; los comandos que si corre tambien van al destino exacto
	f := &h3Tmux{fallar: "split-window"}
	root2, _ := cbProyecto(t, true)
	if _, err := Open(root2, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps()); err == nil {
		t.Fatalf("CA-347: fixture: con split-window roto Open devuelve el error:\n%s", f.todas())
	}
	for _, l := range f.llamadas {
		if strings.HasPrefix(l, "tmux ") {
			llamadas = append(llamadas, l)
		}
	}

	// Capture con la sesion abierta
	gc := &cbGrabadora{sesion: true, panes: "%3\n%7\n", pantalla: "hola\n"}
	if term, err := Capture(root, "demo", "facturas", gc.deps(nil)); err != nil || !term.Available {
		t.Fatalf("CA-350: fixture: Capture con la sesion abierta: %+v %v", term, err)
	}
	desdeCapture := len(llamadas)
	agregar(gc.calls, gc.outputs)

	var mal []string
	split, lista := false, false
	for i, l := range llamadas {
		c := strings.Fields(l)
		if len(c) < 2 {
			continue
		}
		verbo, d := c[1], ""
		for j := 2; j+1 < len(c); j++ {
			if c[j] == "-t" {
				d = c[j+1]
				break
			}
		}
		if verbo == "split-window" && i < desdeOpen {
			split = true
		}
		if verbo == "list-panes" && i >= desdeCapture {
			lista = true
		}
		pane := verbo == "split-window" || verbo == "select-pane"
		switch {
		case d == "" && pane:
			mal = append(mal, l+": sin -t, tmux usa la sesion actual, que puede ser la de otra tarjeta")
		case d == "":
		case strings.HasPrefix(d, "%") && !pane:
		case strings.HasPrefix(d, "="+sesion+":"):
		case d == "="+sesion && (verbo == "has-session" || verbo == "kill-session"):
		default:
			mal = append(mal, l+": el destino "+d+" no es el exacto ="+sesion+":")
		}
	}
	if !split {
		mal = append(mal, "Open agrega el pane de vigilancia con split-window -t ="+sesion+":")
	}
	if !lista {
		mal = append(mal, "Capture lista los panes con list-panes -t ="+sesion+":")
	}
	if len(mal) > 0 {
		t.Fatalf("337b23 (CA-347, CA-350, enmienda 5): los comandos de tmux a la sesion de la tarjeta usan el destino exacto =%s: (con los dos puntos):\n- %s\ntmux:\n%s",
			sesion, strings.Join(mal, "\n- "), strings.Join(llamadas, "\n"))
	}
}
