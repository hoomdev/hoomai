// Tests de regresion de la review de la cabina C3 sobre
// .hoom/specs/acciones-desde-la-tarjeta.md (CA-350): cuando la salida de
// `tmux capture-pane -p -e` supera TerminalMaxBytes, Capture devuelve un
// PREFIJO de ella de a lo sumo TerminalMaxBytes bytes que no parte un
// caracter UTF-8 ni termina dentro de una secuencia de escape (CSI `ESC [`
// ... byte final 0x40-0x7E; OSC `ESC ]` ... BEL o `ESC \`). Solo se descarta
// el pedazo incompleto: len(Text) >= TerminalMaxBytes - largo de ese pedazo.
// Si la salida entra, Text es identica.
package cockpitcmd

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"unicode/utf8"
)

// hcRelleno son unidades completas (texto, runas de 2 a 4 bytes, SGR y un
// hipervinculo OSC 8) para que el relleno se parezca a una pantalla real.
const hcRelleno = "\x1b[1;32mverde\x1b[0m ñandú 20€ 😀 \x1b]8;;https://ejemplo.com/x\x07enlace\x1b]8;;\x1b\\ \x1b[38;5;208mnaranja\x1b[0m\n"

// hcPantalla arma una salida donde la unidad u empieza k bytes antes del
// borde de TerminalMaxBytes (u queda partida si 0 < k < len(u)), seguida de
// mas pantalla, de modo que la salida supera el tope. Devuelve la salida y el
// prefijo que debe quedar: todo lo anterior a u.
func hcPantalla(u string, k int) (salida, antesDeU string) {
	inicio := TerminalMaxBytes - k
	var b strings.Builder
	for b.Len()+len(hcRelleno) <= inicio {
		b.WriteString(hcRelleno)
	}
	b.WriteString(strings.Repeat("x", inicio-b.Len()))
	antesDeU = b.String()
	b.WriteString(u)
	b.WriteString("despues del borde\n")
	b.WriteString(strings.Repeat(hcRelleno, 64))
	return b.String(), antesDeU
}

// hcCapturar corre Capture con la grabadora de CA-350 y la pantalla dada.
func hcCapturar(t *testing.T, pantalla string) Terminal {
	t.Helper()
	root, _ := cbProyecto(t, false)
	g := &cbGrabadora{sesion: true, panes: "%3\n", pantalla: pantalla}
	term, err := Capture(root, "demo", "facturas", g.deps(nil))
	if err != nil {
		t.Fatalf("CA-350: Capture: %v", err)
	}
	if !term.Available || term.Session != "hoom-demo-facturas" || term.Pane != "%3" || term.Note != "" {
		t.Fatalf("CA-350: con sesion, available, session, pane y sin nota: %+v", term)
	}
	return term
}

// hcTerminaDentroDeEscape dice si s termina adentro de una secuencia de
// escape sin cerrar: un ESC suelto, un CSI sin su byte final 0x40-0x7E, o un
// OSC sin su BEL ni su `ESC \`.
func hcTerminaDentroDeEscape(s string) bool {
	i := strings.LastIndexByte(s, 0x1b)
	if i < 0 {
		return false
	}
	resto := s[i+1:]
	switch {
	case resto == "":
		// un ESC al final: suelto, o el ESC del `ESC \` de un OSC abierto;
		// los dos son una secuencia sin cerrar
		return true
	case resto[0] == '[':
		for j := 1; j < len(resto); j++ {
			if resto[j] >= 0x40 && resto[j] <= 0x7e {
				return false
			}
		}
		return true
	case resto[0] == ']':
		return !strings.ContainsRune(resto, 0x07)
	default:
		// `ESC \` (el ST que cierra un OSC) u otra secuencia de dos bytes
		return false
	}
}

// Hallazgo 20260924T052244_32db75. CA-350: el corte en TerminalMaxBytes no
// parte una runa de 2, 3 ni 4 bytes, ni una SGR `ESC [31m` en ninguno de sus
// bytes, ni un OSC 8 con terminador BEL, ni uno con `ESC \` partido entre el
// ESC y el `\`: descarta solo ese pedazo. Una secuencia o una runa completa
// que termina justo en el borde no se toca, y una salida que entra (aunque
// termine a mitad de una secuencia) vuelve identica.
func TestHallazgo_32db75_CaptureNoParteRunasNiSecuencias(t *testing.T) {
	if TerminalMaxBytes != 256<<10 {
		t.Fatalf("CA-350: el tope del espejo es 256 KiB: %d", TerminalMaxBytes)
	}
	type caso struct {
		nombre string
		u      string
		k      int // bytes de u que entran antes del borde
	}
	var partidos []caso
	for _, u := range []string{"ñ", "€", "😀"} {
		for k := 1; k < len(u); k++ {
			partidos = append(partidos, caso{fmt.Sprintf("runa de %d bytes, %d antes del borde", len(u), k), u, k})
		}
	}
	for k := 1; k < len("\x1b[31m"); k++ {
		partidos = append(partidos, caso{fmt.Sprintf("SGR ESC[31m partida en el byte %d", k), "\x1b[31m", k})
	}
	oscBEL := "\x1b]8;;https://ejemplo.com/precios?id=7\x07"
	for _, k := range []int{1, 2, 5, len(oscBEL) - 1} {
		partidos = append(partidos, caso{fmt.Sprintf("OSC 8 con BEL, %d de %d bytes antes del borde", k, len(oscBEL)), oscBEL, k})
	}
	oscST := "\x1b]8;;https://ejemplo.com/precios?id=7\x1b\\"
	for _, k := range []int{1, 2, 9, len(oscST) - 1} {
		partidos = append(partidos, caso{fmt.Sprintf("OSC 8 con ESC \\, %d de %d bytes antes del borde", k, len(oscST)), oscST, k})
	}

	for _, c := range partidos {
		t.Run(c.nombre, func(t *testing.T) {
			salida, want := hcPantalla(c.u, c.k)
			if len(salida) <= TerminalMaxBytes || !utf8.ValidString(salida) {
				t.Fatalf("fixture: la salida supera el tope y es UTF-8 valido")
			}
			term := hcCapturar(t, salida)
			got := term.Text
			if len(got) > TerminalMaxBytes || !strings.HasPrefix(salida, got) {
				t.Fatalf("32db75 CA-350: Text es un prefijo de la salida de a lo sumo %d bytes: len %d, prefijo %v",
					TerminalMaxBytes, len(got), strings.HasPrefix(salida, got))
			}
			if !utf8.ValidString(got) {
				t.Fatalf("32db75 CA-350: %s: la salida es UTF-8 valido y Text tambien; termina en %q", c.nombre, hcColaT(got))
			}
			if hcTerminaDentroDeEscape(got) {
				t.Fatalf("32db75 CA-350: %s: Text no termina dentro de una secuencia de escape; termina en %q", c.nombre, hcColaT(got))
			}
			if got != want {
				t.Fatalf("32db75 CA-350: %s: se descarta solo el pedazo incompleto (%d bytes): len(Text) %d, quiere %d; termina en %q",
					c.nombre, c.k, len(got), len(want), hcColaT(got))
			}
		})
	}

	// completas justo antes del borde: no se tocan
	for _, u := range []string{"\x1b[0m", "ñ", "😀", oscBEL, oscST} {
		t.Run(fmt.Sprintf("completa de %d bytes termina en el borde", len(u)), func(t *testing.T) {
			salida, _ := hcPantalla(u, len(u))
			term := hcCapturar(t, salida)
			if want := salida[:TerminalMaxBytes]; term.Text != want {
				t.Fatalf("32db75 CA-350: una unidad completa que termina justo en el borde no se toca: len(Text) %d, quiere %d; termina en %q",
					len(term.Text), len(want), hcColaT(term.Text))
			}
		})
	}

	// si entra, identica (aun terminando en una secuencia sin cerrar)
	for _, cola := range []string{"\x1b[0m\n", "\x1b[31"} {
		t.Run(fmt.Sprintf("entra entera y termina en %q", cola), func(t *testing.T) {
			var b strings.Builder
			for b.Len()+len(hcRelleno) <= TerminalMaxBytes-len(cola) {
				b.WriteString(hcRelleno)
			}
			b.WriteString(strings.Repeat("x", TerminalMaxBytes-len(cola)-b.Len()))
			b.WriteString(cola)
			salida := b.String()
			if len(salida) != TerminalMaxBytes {
				t.Fatalf("fixture: la salida mide justo el tope: %d", len(salida))
			}
			if term := hcCapturar(t, salida); term.Text != salida {
				t.Fatalf("32db75 CA-350: una salida de %d bytes entra y vuelve identica: len(Text) %d", len(salida), len(term.Text))
			}
		})
	}
}

// Hallazgo 20260924T052244_32db75. CA-350, propiedad: sobre pantallas al azar
// (semilla fija) hechas de texto, runas de 1 a 4 bytes, SGR, CSI privadas y
// OSC con los dos terminadores, que superan el tope, Text es siempre la
// salida hasta el comienzo de la secuencia o runa que el borde parte, o hasta
// el borde si no parte ninguna (el texto ASCII se corta en cualquier byte):
// prefijo de a lo sumo TerminalMaxBytes, UTF-8 valido, fuera de toda
// secuencia, y sin descartar mas que el pedazo incompleto.
func TestHallazgo_32db75_PropiedadCorteSeguro(t *testing.T) {
	unidades := []string{
		"a", "palabra ", "\n", "ñ", "é", "€", "✓", "😀", "𝄞",
		"\x1b[0m", "\x1b[31m", "\x1b[1;4;38;5;208m", "\x1b[?25h", "\x1b[2K",
		"\x1b]8;;https://ejemplo.com/a?b=c\x07", "\x1b]8;;\x07",
		"\x1b]8;;https://ejemplo.com/d\x1b\\", "\x1b]8;;\x1b\\", "\x1b]0;titulo del pane\x07",
	}
	r := rand.New(rand.NewSource(350))
	root, _ := cbProyecto(t, false)
	partidas := 0
	for n := 0; n < 200; n++ {
		var b strings.Builder
		corteLimpio := TerminalMaxBytes // lo que debe quedar
		for b.Len() <= TerminalMaxBytes+64 {
			u := unidades[r.Intn(len(unidades))]
			ini := b.Len()
			b.WriteString(u)
			// solo una secuencia de escape o una runa de varios bytes son
			// indivisibles: el texto ASCII se puede cortar en cualquier byte
			indivisible := u[0] == 0x1b || u[0] >= 0x80
			if indivisible && ini < TerminalMaxBytes && TerminalMaxBytes < b.Len() {
				corteLimpio = ini // el borde parte esta unidad
			}
		}
		salida := b.String()
		want := salida[:corteLimpio]
		if corteLimpio < TerminalMaxBytes {
			partidas++
		}
		g := &cbGrabadora{sesion: true, panes: "%3\n", pantalla: salida}
		term, err := Capture(root, "demo", "facturas", g.deps(nil))
		if err != nil || !term.Available {
			t.Fatalf("CA-350: Capture con sesion: %v %+v", err, term)
		}
		got := term.Text
		if len(got) > TerminalMaxBytes || !strings.HasPrefix(salida, got) || !utf8.ValidString(got) || hcTerminaDentroDeEscape(got) || got != want {
			t.Fatalf("32db75 CA-350: pantalla %d: Text es la salida hasta el comienzo de la unidad partida por el borde (%d bytes), fue %d bytes (utf8 %v, dentro de escape %v); termina en %q",
				n, len(want), len(got), utf8.ValidString(got), hcTerminaDentroDeEscape(got), hcColaT(got))
		}
	}
	if partidas < 50 {
		t.Fatalf("fixture: la propiedad necesita pantallas con el borde partiendo una unidad: %d de 200", partidas)
	}
}

func hcColaT(s string) string {
	if len(s) > 48 {
		return s[len(s)-48:]
	}
	return s
}
