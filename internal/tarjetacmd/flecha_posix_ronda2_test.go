// Tests adversariales de la ronda 2 de la review de verbos-de-la-tarjeta en
// los verbos de la tarjeta: hallazgos 522ed0 y 1ccc66 (CA-503). La linea →
// se escribe para un sh POSIX, valida tambien en dash: ya no usa $'...'
// (dash lo deja literal, y con un modelo "\x01'$(touch pwn)" y un esfuerzo
// "it's" pegar la linea en dash CORRIA touch pwn; ksh93 leia $'a\x09eb' como
// otro valor). El contrato:
//
//   - un valor (el pedido, --model, --effort, --resume) con un caracter de
//     control, un byte que no es UTF-8 o una runa que no se imprime va como
//     "$(printf '<fmt>')": en el formato las runas que se imprimen van tal
//     cual salvo ' ('\''), \ (\\), % (%%), ! (\041) y un - al principio
//     (\055); cada byte de una runa que no se imprime, o un byte invalido, es
//     \NNN con exactamente tres digitos octales;
//   - si no, el pedido (recortado a 60 caracteres con …, los saltos de linea
//     como espacios) va entre comillas dobles cuando no tiene $, `, \, " ni !,
//     y si no entre simples (una ' como '\''); --model, --effort y --resume
//     van tal cual cuando cada byte esta en [A-Za-z0-9._:/@%+,-], y si no
//     entre simples.
//
// La prueba fuerte es la vuelta: las palabras pegadas se evaluan en cada
// shell que haya entre /bin/sh, dash, bash, zsh y ksh (r1Shells), en una
// carpeta vacia; cada valor vuelve byte a byte y la carpeta sigue vacia. Los
// fixtures son los de tarjetacmd_test.go y hallazgos_ronda1_test.go. (Este
// comentario no es la doc del paquete: gofmt no le cambia las comillas.)

package tarjetacmd_test

import (
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// ---------------------------------------------------------------------------
// el contrato de la linea, escrito desde el spec
// ---------------------------------------------------------------------------

// r2NoSeImprime dice si v lleva un caracter de control, un byte que no es
// UTF-8 o una runa que no se imprime.
func r2NoSeImprime(v string) bool {
	for i := 0; i < len(v); {
		r, n := utf8.DecodeRuneInString(v[i:])
		if (r == utf8.RuneError && n == 1) || !unicode.IsPrint(r) {
			return true
		}
		i += n
	}
	return false
}

// r2Printf es la forma "$(printf '<fmt>')" del contrato.
func r2Printf(v string) string {
	var b strings.Builder
	b.WriteString(`"$(printf '`)
	for i := 0; i < len(v); {
		r, n := utf8.DecodeRuneInString(v[i:])
		switch {
		case (r == utf8.RuneError && n == 1) || !unicode.IsPrint(r):
			for _, c := range []byte(v[i : i+n]) {
				fmt.Fprintf(&b, `\%03o`, c)
			}
		case r == '\'':
			b.WriteString(`'\''`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '%':
			b.WriteString(`%%`)
		case r == '!':
			b.WriteString(`\041`)
		case r == '-' && i == 0:
			b.WriteString(`\055`)
		default:
			b.WriteString(v[i : i+n])
		}
		i += n
	}
	b.WriteString(`')"`)
	return b.String()
}

// r2Seguro dice si cada byte de v esta en [A-Za-z0-9._:/@%+,-].
func r2Seguro(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("._:/@%+,-", c) >= 0) {
			return false
		}
	}
	return v != ""
}

// r2FormaValor es como va en la linea el valor de --model, --effort o
// --resume.
func r2FormaValor(v string) string {
	switch {
	case r2NoSeImprime(v):
		return r2Printf(v)
	case r2Seguro(v):
		return v
	default:
		return r1EntreSimples(v)
	}
}

// r2FormaPedido es como va en la linea el pedido ya recortado.
func r2FormaPedido(cut string) string {
	switch {
	case r2NoSeImprime(cut):
		return r2Printf(cut)
	case strings.ContainsAny(cut, "$`\\\"!"):
		return r1EntreSimples(cut)
	default:
		return `"` + cut + `"`
	}
}

// r2Corte es el pedido como va en la linea: los saltos de linea como
// espacios y recortado a 60 caracteres con … (los casos de este archivo no
// recortan un pedido con bytes que no son UTF-8).
func r2Corte(p string) string {
	return r1Recorte(strings.ReplaceAll(p, "\n", " "))
}

// r2Pegar evalua la linea (sin el prefijo) en cada shell y devuelve las
// palabras de cada uno; exige que todos den las mismas.
func r2Pegar(t *testing.T, ca, resto string) []string {
	t.Helper()
	shells := r1Shells(false)
	if len(shells) == 0 {
		t.Fatalf("%s: no hay ningun shell para pegar la linea", ca)
	}
	var primera []string
	for i, sh := range shells {
		ws := r1Evaluar(t, ca+" ("+sh+")", sh, resto)
		if i == 0 {
			primera = ws
			continue
		}
		if strings.Join(ws, "\x00") != strings.Join(primera, "\x00") {
			t.Fatalf("%s: %s y %s leen distinto la linea pegada:\n %s %q\n %s %q\n%q", ca, shells[0], sh, shells[0], primera, sh, ws, resto)
		}
	}
	return primera
}

// TestHallazgo_522ed0_ElContratoDeLaFormaPrintf comprueba el codificador
// del contrato (hallazgo 522ed0, CA-503) contra los shells: si la forma que
// este archivo exige no diera el valor al pegarla, el resto de los tests no
// probaria nada. No mira a hoom: es verde antes y despues.
func TestHallazgo_522ed0_ElContratoDeLaFormaPrintf(t *testing.T) {
	for _, v := range []string{"\x01'$(touch pwn)", "a\teb", "-x\x01", "100%\x07", "hola!\x01", "\xff\xfe", "\u202e",
		"a\\b\x01", "%s%n\x01", "\\c\x01", "ctl\x017 y tab\t9", "\x1b[2J ñ €", "\"x\" ) ( # ; `touch pwn`\x01"} {
		ws := r2Pegar(t, "hallazgo 522ed0 (contrato "+strconv.QuoteToASCII(v)+")", r2Printf(v)+" fin")
		if len(ws) != 2 || ws[0] != v || ws[1] != "fin" {
			t.Fatalf("hallazgo 522ed0: la forma del contrato %s da %q, quiero %q", r2Printf(v), ws, v)
		}
	}
}

// ---------------------------------------------------------------------------
// el pedido
// ---------------------------------------------------------------------------

// hallazgos 522ed0 y 1ccc66, CA-503: un pedido con un caracter de control,
// un byte que no es UTF-8 o una runa que no se imprime va en la linea → como
// "$(printf '...')" con la forma exacta del contrato; pegada en cada shell
// (sh, dash, bash, zsh, ksh), la linea da el pedido recortado byte a byte y
// no ejecuta nada. La linea no lleva crudo ningun caracter de control. Un
// pedido que empieza con - (\055 en la forma) se imprime igual, aunque hoom
// despues no lo lanza (el provider lo leeria como flag).
func TestHallazgo_522ed0_1ccc66_CA503_PedidoConPrintfEnTodosLosShells(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 522ed0", root, "precios", "")
	casos := []struct {
		pedido     string
		cualquiera bool // la forma no es decidible desde el contrato: solo la vuelta
	}{
		{"\x01'$(touch pwn)", false},
		{"a\teb", false},
		{"tab\tseguido de x09eb y \t0x41", false},
		{"el 100% listo\x07", false},
		{"hola!\x01", false},
		{"-n\x01 al frente", false},
		{"-e \\x41\x1b", false},
		{"byte \xff y \xc3 sueltos", false},
		{"U+202E \u202e al reves", false},
		{"it's \x01 $(touch pwn) `touch pwn` \"x\" ) ( # ; & | < > *", false},
		{"%s%d%n\x01", false},
		{"\\c\x01 corta", false},
		{"fin con barra \\\x01", false},
		{strings.Repeat("a", 59) + "\x01" + strings.Repeat("b", 10), false},
		{"año ñandú\x01 €", false},
		{"ancho\u200b cero", false},
		{"C1 \u0085 NEL", false},
		{"\x7f DEL", false},
		{"linea1\nlinea2\x01", false},
		{"ctl\x017 y tab\t9", false},
		{"\x1b[2J\x1b[H borra la pantalla", false},
		{"!! historia\x01", false},
		{strings.Repeat("a", 60) + "\x01", true},
	}
	for _, c := range casos {
		ca := "hallazgos 522ed0 y 1ccc66, CA-503 (pedido " + strconv.QuoteToASCII(c.pedido) + ")"
		s := vdtCorrer(t, ca, vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", c.pedido)
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		if !strings.HasPrefix(linea, r1Base) {
			t.Fatalf("%s: la linea → empieza con %q: %q", ca, r1Base, linea)
		}
		cut := r2Corte(c.pedido)
		quiero := []string{"--role", "writer", "--task", "precios", "--spec", ".hoom/specs/precios.md", "--provider", "claude", cut}
		ws := r2Pegar(t, ca, strings.TrimPrefix(linea, "→ hoom agent "))
		if strings.Join(ws, "\x00") != strings.Join(quiero, "\x00") {
			t.Fatalf("%s: pegada en un shell, la linea da\n  %q\nquiero\n  %q\nlinea %q", ca, ws, quiero, linea)
		}
		if c.cualquiera {
			continue
		}
		if got, want := strings.TrimPrefix(linea, r1Base), r2FormaPedido(cut); got != want {
			t.Fatalf("%s: el pedido va como\n  %s\nfue\n  %s", ca, want, got)
		}
	}
	lanzables := 0
	for _, c := range casos {
		if !strings.HasPrefix(c.pedido, "-") {
			lanzables++
		}
	}
	if p := f.pedidos(t, "claude"); len(p) != lanzables {
		t.Fatalf("hallazgo 522ed0: fixture: cada pedido que no empieza con - se lanzo (%d de %d)", len(p), lanzables)
	}
}

// ---------------------------------------------------------------------------
// --model y --effort
// ---------------------------------------------------------------------------

// hallazgos 522ed0 y 1ccc66, CA-503: --model y --effort con un caracter de
// control, un byte que no es UTF-8 o una runa que no se imprime van como
// "$(printf '...')"; sin ellos, tal cual si son seguros y si no entre
// simples. Incluye el caso del refutador en dash (modelo "\x01'$(touch
// pwn)", esfuerzo "it's"), que pegado en dash corria touch pwn. Vale para
// los del comando y los de hoom.yaml; cada valor vuelve byte a byte en cada
// shell, y el provider los recibe tal cual. Un esfuerzo que empieza con -
// (\055 en la forma) se imprime igual, aunque hoom despues no lo lanza.
func TestHallazgo_522ed0_1ccc66_CA503_ModeloYEsfuerzoEnTodosLosShells(t *testing.T) {
	casos := []struct {
		nombre, yaml  string
		flags         []string
		model, effort string
		noLanza       bool // hoom rechaza el valor despues de imprimir la linea
	}{
		{"el refutador de dash", "", []string{"--model", "\x01'$(touch pwn)", "--effort", "it's"}, "\x01'$(touch pwn)", "it's", false},
		{"un tab y letras de hexadecimal", "", []string{"--model", "a\teb", "--effort", "e\x01f"}, "a\teb", "e\x01f", false},
		{"% y ! con control", "", []string{"--model", "100%\x07", "--effort", "hola!\x01"}, "100%\x07", "hola!\x01", false},
		{"bytes invalidos y U+202E", "", []string{"--model", "\xff\xfe", "--effort", "\u202e"}, "\xff\xfe", "\u202e", false},
		{"barras y formatos de printf", "", []string{"--model", "a\\b\x01", "--effort", "%s%n\x01"}, "a\\b\x01", "%s%n\x01", false},
		{"\\c y un octal seguido de digito", "", []string{"--model", "\\c\x01", "--effort", "ctl\x017"}, "\\c\x01", "ctl\x017", false},
		{"un - al principio del modelo, del comando", "", []string{"--model", "-x\x01", "--effort", "e\t9"}, "-x\x01", "e\t9", false},
		{"un - al principio del modelo, de hoom.yaml", "agents:\n  writer:\n    model: \"-m\\x01$(touch pwn)\"\n    effort: \"e\\t9\"\n", nil,
			"-m\x01$(touch pwn)", "e\t9", false},
		{"un - al principio del esfuerzo", "", []string{"--model", "m", "--effort", "-e\t9"}, "m", "-e\t9", true},
		{"de hoom.yaml, con escape y backtick", "agents:\n  writer:\n    model: \"m\\x1b[2J$(touch pwn)\"\n    effort: \"e\\u202e`touch pwn`\"\n", nil,
			"m\x1b[2J$(touch pwn)", "e\u202e`touch pwn`", false},
		{"sin control: seguros y entre simples", "", []string{"--model", "us.anthropic/claude@4:5+x,y%z_w-1", "--effort", "it's $x"},
			"us.anthropic/claude@4:5+x,y%z_w-1", "it's $x", false},
	}
	for _, c := range casos {
		ca := "hallazgos 522ed0 y 1ccc66, CA-503 (" + c.nombre + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, c.yaml)
		vdtWriter(t, ca, root, "precios", "")
		args := append([]string{"writer", "precios"}, c.flags...)
		args = append(args, "-m", "Implementa")
		s := vdtCorrer(t, ca, vdtOpts{dir: root}, "pedir", args...)
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		want := r1Base + "--model " + r2FormaValor(c.model) + " --effort " + r2FormaValor(c.effort) + ` "Implementa"`
		ws := r2Pegar(t, ca, strings.TrimPrefix(linea, "→ hoom agent "))
		if v, _ := r1Despues(ws, "--model"); v != c.model {
			t.Fatalf("%s: pegada, --model da %q, quiero %q byte a byte:\n%q", ca, v, c.model, linea)
		}
		if v, _ := r1Despues(ws, "--effort"); v != c.effort {
			t.Fatalf("%s: pegada, --effort da %q, quiero %q byte a byte:\n%q", ca, v, c.effort, linea)
		}
		if ws[len(ws)-1] != "Implementa" {
			t.Fatalf("%s: pegada, la ultima palabra es el pedido: %q", ca, ws)
		}
		if linea != want {
			t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
		}
		if c.noLanza {
			continue
		}
		args0 := vdtUnaLlamada(t, ca, "pedir writer", f, "claude")
		if v, _ := vdtSigue(args0, "--model"); v != c.model {
			t.Fatalf("%s: el provider recibe el modelo %q: %q", ca, c.model, args0)
		}
		if v, _ := vdtSigue(args0, "--effort"); v != c.effort {
			t.Fatalf("%s: el provider recibe el esfuerzo %q: %q", ca, c.effort, args0)
		}
	}
}

// ---------------------------------------------------------------------------
// --resume y el reviewer
// ---------------------------------------------------------------------------

// hallazgos 522ed0 y 1ccc66, CA-503 y CA-511: el valor de --resume con un
// caracter de control va como "$(printf '...')" (un - al principio como
// \055, % como %%, ! como \041); pegado en cada shell da la sesion byte a
// byte, y el provider la retoma (una sesion que empieza con - se imprime
// igual, aunque hoom despues no la retoma).
func TestHallazgo_522ed0_1ccc66_CA503_ResumeEnTodosLosShells(t *testing.T) {
	for _, sesion := range []string{"ses\x01'$(touch pwn)", "-s\t7%!", "ses\u202e`touch pwn`"} {
		ca := "hallazgos 522ed0 y 1ccc66, CA-503 (--resume " + strconv.QuoteToASCII(sesion) + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, "")
		r1Interrumpida(t, ca, root, "precios", sesion)
		s := vdtCorrer(t, ca, vdtOpts{dir: root}, "reanudar", "precios", "-m", "Termina lo que quedo")
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		if !strings.Contains(linea, " --resume "+r2FormaValor(sesion)+" ") {
			t.Fatalf("%s: la linea lleva --resume %s:\n%s", ca, r2FormaValor(sesion), linea)
		}
		ws := r2Pegar(t, ca, strings.TrimPrefix(linea, "→ hoom agent "))
		if v, _ := r1Despues(ws, "--resume"); v != sesion {
			t.Fatalf("%s: pegada, --resume da %q, quiero %q: %q", ca, v, sesion, linea)
		}
		if ws[len(ws)-1] != "Termina lo que quedo" {
			t.Fatalf("%s: pegada, la ultima palabra es el pedido: %q", ca, ws)
		}
		if strings.HasPrefix(sesion, "-") {
			continue
		}
		args := vdtUnaLlamada(t, ca, "reanudar", f, "claude")
		if v, _ := vdtSigue(args, "--resume"); v != sesion {
			t.Fatalf("%s: el provider retoma la sesion %q: %q", ca, sesion, args)
		}
	}
}

// hallazgos 522ed0 y 1ccc66, CA-503: la → del reviewer (hoom review ...) con
// un modelo y un esfuerzo de review: con caracteres de control usa la misma
// forma, y pegada en cada shell no ejecuta nada.
func TestHallazgo_522ed0_1ccc66_CA503_FlechaDelReviewerEnTodosLosShells(t *testing.T) {
	const model, effort = "r\x01$(touch pwn)", "\x1b`touch pwn`'"
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "review:\n  model: \"r\\x01$(touch pwn)\"\n  effort: \"\\e`touch pwn`'\"\n")
	vdtReview(t, "hallazgo 522ed0", root, "precios")
	s := vdtCorrer(t, "hallazgo 522ed0", vdtOpts{dir: root}, "pedir", "reviewer", "precios")
	linea := vdtFlechaDe(t, "hallazgo 522ed0 (reviewer)", s.out)
	r1SinControlCrudo(t, "hallazgo 522ed0 (reviewer)", linea)
	const pre = "→ hoom review --task precios --spec .hoom/specs/precios.md --provider claude"
	if !strings.HasPrefix(linea, pre) {
		t.Fatalf("hallazgo 522ed0, CA-503: la → del reviewer empieza con %q: %q", pre, linea)
	}
	for _, k := range []struct{ flag, v string }{{"--model", model}, {"--effort", effort}} {
		if !strings.Contains(linea+" ", " "+k.flag+" "+r2FormaValor(k.v)+" ") {
			t.Fatalf("hallazgo 522ed0, CA-503: la → del reviewer lleva %s %s:\n%s", k.flag, r2FormaValor(k.v), linea)
		}
	}
	ws := r2Pegar(t, "hallazgo 522ed0 (reviewer)", strings.TrimPrefix(linea, "→ hoom review "))
	if v, _ := r1Despues(ws, "--model"); v != model {
		t.Fatalf("hallazgo 522ed0 (reviewer): pegada, --model da %q, quiero %q: %q", v, model, linea)
	}
	if v, _ := r1Despues(ws, "--effort"); v != effort {
		t.Fatalf("hallazgo 522ed0 (reviewer): pegada, --effort da %q, quiero %q: %q", v, effort, linea)
	}
}

// ---------------------------------------------------------------------------
// propiedad: valores al azar de un alfabeto hostil
// ---------------------------------------------------------------------------

// r2Alfabeto son las piezas con que se arman los valores al azar: control,
// bytes invalidos, runas que no se imprimen, todo lo que un shell
// interpreta y las letras y digitos que engañan a un octal o a un \x.
var r2Alfabeto = []string{"\x01", "\t", "\x1b", "\x7f", "\xff", "\xc3", "\u202e", "\u200b", "\u0085", "ñ", "€", "'", `"`, "$",
	"`", `\`, "%", "!", "-", "(", ")", " ", "#", ";", "&", "|", "*", "?", "~", "{", "}", "=", "<", ">", "0", "7", "9", "a", "e",
	"b", "x", "n", "c", "$(touch pwn)", "`touch pwn`", "'$(touch pwn)'", "%s", "%n", "\\x41", "\\0"}

// r2AlAzar arma un valor de 1 a max piezas que empieza y termina con una
// letra (asi ningun recorte de blancos de los extremos cambia el valor).
func r2AlAzar(r *rand.Rand, max int) string {
	var b strings.Builder
	b.WriteString(string(rune('a' + r.IntN(26))))
	for i, n := 0, r.IntN(max); i < n; i++ {
		b.WriteString(r2Alfabeto[r.IntN(len(r2Alfabeto))])
	}
	b.WriteString(string(rune('a' + r.IntN(26))))
	return b.String()
}

// hallazgos 522ed0 y 1ccc66, CA-503, como propiedad: para modelos, esfuerzos
// y pedidos al azar del alfabeto hostil (semilla fija, para que una falla se
// repita), la linea → no lleva crudo ningun caracter de control, cada valor
// va con la forma del contrato y, pegada en cada shell, da cada valor byte a
// byte sin ejecutar nada.
func TestHallazgo_522ed0_1ccc66_CA503_PropiedadDeLaVuelta(t *testing.T) {
	r := rand.New(rand.NewPCG(522, 0x1ccc66))
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 522ed0", root, "precios", "")
	for i := 0; i < 12; i++ {
		model, effort, pedido := r2AlAzar(r, 8), r2AlAzar(r, 8), r2AlAzar(r, 12)
		for len([]rune(pedido)) > 60 {
			pedido = r2AlAzar(r, 12)
		}
		ca := fmt.Sprintf("hallazgos 522ed0 y 1ccc66, CA-503 (al azar %d: --model %s --effort %s -m %s)", i,
			strconv.QuoteToASCII(model), strconv.QuoteToASCII(effort), strconv.QuoteToASCII(pedido))
		s := vdtCorrer(t, ca, vdtOpts{dir: root}, "pedir", "writer", "precios", "--model", model, "--effort", effort, "-m", pedido)
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		ws := r2Pegar(t, ca, strings.TrimPrefix(linea, "→ hoom agent "))
		quiero := []string{"--role", "writer", "--task", "precios", "--spec", ".hoom/specs/precios.md", "--provider", "claude",
			"--model", model, "--effort", effort, pedido}
		if strings.Join(ws, "\x00") != strings.Join(quiero, "\x00") {
			t.Fatalf("%s: pegada en un shell, la linea da\n  %q\nquiero\n  %q\nlinea %q", ca, ws, quiero, linea)
		}
		want := r1Base + "--model " + r2FormaValor(model) + " --effort " + r2FormaValor(effort) + " " + r2FormaPedido(pedido)
		if linea != want {
			t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
		}
	}
}

// ---------------------------------------------------------------------------
// el borde: un valor que termina en saltos de linea
// ---------------------------------------------------------------------------

// r2DesdeLaFlecha es lo que sigue a prefijo en out hasta fin incluido: la
// "linea" → cuando un valor lleva saltos de linea crudos.
func r2DesdeLaFlecha(t *testing.T, ca, out, prefijo, fin string) string {
	t.Helper()
	i := strings.Index(out, prefijo)
	if i < 0 {
		t.Fatalf("%s: falta %q:\n%s", ca, prefijo, out)
	}
	j := strings.Index(out[i:], fin)
	if j < 0 {
		t.Fatalf("%s: la → no termina con %q:\n%s", ca, fin, out)
	}
	return out[i+len(prefijo) : i+j+len(fin)]
}

// hallazgos 522ed0 y 1ccc66, CA-503 (borde): un valor que termina en saltos
// de linea los conserva al pegarse ($(...) se come los saltos del final,
// asi que van aparte, crudos entre simples). Pasa con la sesion de --resume,
// que hoom toma del sidecar tal cual. Solo se exige la vuelta: la sesion
// vuelve byte a byte en cada shell y nada se ejecuta; la "linea" lleva
// entonces saltos crudos, asi que se toma desde → hasta el pedido.
func TestHallazgo_522ed0_1ccc66_CA503_SesionQueTerminaEnSaltos(t *testing.T) {
	for _, sesion := range []string{"ses\x01\n", "ses\n\n", "it's\x01$(touch pwn)\n"} {
		ca := "hallazgos 522ed0 y 1ccc66, CA-503 (--resume " + strconv.QuoteToASCII(sesion) + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, "")
		r1Interrumpida(t, ca, root, "precios", sesion)
		s := vdtCorrer(t, ca, vdtOpts{dir: root}, "reanudar", "precios", "-m", "Termina")
		resto := r2DesdeLaFlecha(t, ca, s.out, "→ hoom agent ", ` "Termina"`)
		ws := r2Pegar(t, ca, resto)
		if v, _ := r1Despues(ws, "--resume"); v != sesion {
			t.Fatalf("%s: pegada, --resume da %q, quiero %q byte a byte:\n%q", ca, v, sesion, resto)
		}
		if ws[len(ws)-1] != "Termina" {
			t.Fatalf("%s: pegada, la ultima palabra es el pedido: %q", ca, ws)
		}
	}
}

// hallazgos 522ed0 y 1ccc66, CA-503 (borde): --model y --effort con saltos
// de linea al final: la linea pegada da lo mismo que hoom le pasa al
// provider (sea con los saltos o sin ellos), byte a byte, sin ejecutar nada.
func TestHallazgo_522ed0_1ccc66_CA503_ModeloQueTerminaEnSaltos(t *testing.T) {
	for _, k := range []struct{ model, effort string }{
		{"m\x01\n", "e\n\n"},
		{"it's\n", "x\x01$(touch pwn)\n"},
	} {
		ca := "hallazgos 522ed0 y 1ccc66, CA-503 (--model " + strconv.QuoteToASCII(k.model) + " --effort " + strconv.QuoteToASCII(k.effort) + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, "")
		vdtWriter(t, ca, root, "precios", "")
		s := vdtCorrer(t, ca, vdtOpts{dir: root}, "pedir", "writer", "precios", "--model", k.model, "--effort", k.effort, "-m", "Implementa")
		resto := r2DesdeLaFlecha(t, ca, s.out, "→ hoom agent ", ` "Implementa"`)
		ws := r2Pegar(t, ca, resto)
		args := vdtUnaLlamada(t, ca, "pedir writer", f, "claude")
		for _, flag := range []string{"--model", "--effort"} {
			lanzado, _ := vdtSigue(args, flag)
			if v, _ := r1Despues(ws, flag); v != lanzado {
				t.Fatalf("%s: pegada, %s da %q y hoom lanzo %q:\n%q", ca, flag, v, lanzado, resto)
			}
		}
		if ws[len(ws)-1] != "Implementa" {
			t.Fatalf("%s: pegada, la ultima palabra es el pedido: %q", ca, ws)
		}
	}
}
