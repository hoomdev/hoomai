// Tests adversariales de la ronda 2b de la review de verbos-de-la-tarjeta:
// hallazgo 97284d (CA-503, enmienda 3). En Windows la linea → no se escribe
// para un sh POSIX: un valor que cmd y PowerShell no leen igual va como su
// marcador y la linea "muestra la forma y no se pega". El contrato:
//
//   - con Entorno.GOOS "windows", cada valor de la linea (el pedido,
//     --model, --effort, --resume) va:
//   - tal cual (solo --model, --effort y --resume) cuando cada byte esta en
//     [A-Za-z0-9._:/+-]: sin @, % ni , que en Unix si van tal cual;
//   - si no, y siempre para el pedido (recortado a 60 caracteres con … y
//     los saltos de linea como espacios, como en Unix), entre comillas
//     dobles cuando se imprime (UTF-8 valido, cada runa imprimible) y no
//     tiene ", $, `, %, !, ni las comillas dobles tipograficas U+201C
//     U+201D U+201E U+201F, ni una barra doble \\, ni termina en \ (una \
//     suelta si: C:\dir\x);
//   - si no, su marcador: <pedido>, <modelo>, <esfuerzo> o <sesion> (el
//     flag queda: --model <modelo>);
//   - el resto de la linea es como en Unix (rol, tarea, spec, provider,
//     presupuesto, tiempo limite), y el lanzamiento recibe los valores
//     verdaderos: el marcador esta solo en la linea impresa;
//   - con GOOS "" (o "darwin", "linux") nada cambia: la linea POSIX de las
//     rondas 1 y 2.
//
// Aca no hay cmd ni PowerShell y no se emulan: se fija el contrato. Los
// fixtures son los de tarjetacmd_test.go, hallazgos_ronda1_test.go y
// flecha_posix_ronda2_test.go. (Este comentario no es la doc del paquete.)

package tarjetacmd_test

import (
	"bytes"
	"fmt"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

const w2Windows = "windows"

// w2Correr es vdtCorrer con la plataforma para la que se escriben el editor
// y la linea → (Entorno.GOOS). Un panico es una falla del criterio.
func w2Correr(t *testing.T, ca, goos string, o vdtOpts, verb string, args ...string) vdtSalida {
	t.Helper()
	var out, errb bytes.Buffer
	e := tarjetacmd.Entorno{Dir: o.dir, In: strings.NewReader(o.in), Out: &out, Err: &errb, Terminal: o.terminal,
		Getenv: func(k string) string { return o.env[k] }, GOOS: goos}
	code := -1
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s: hoom %s %q (GOOS %q) entro en panico: %v", ca, verb, args, goos, r)
			}
		}()
		code = tarjetacmd.Run(verb, args, e)
	}()
	return vdtSalida{code: code, out: out.String(), err: errb.String()}
}

// ---------------------------------------------------------------------------
// el contrato de Windows, escrito desde el spec
// ---------------------------------------------------------------------------

// w2Plano dice si cada byte de v esta en [A-Za-z0-9._:/+-].
func w2Plano(v string) bool {
	if v == "" {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if !(c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || strings.IndexByte("._:/+-", c) >= 0) {
			return false
		}
	}
	return true
}

// w2EntreDobles dice si v puede ir entre comillas dobles: se imprime y no
// tiene ", $, `, %, !, U+201C..U+201F, \\ ni una \ al final.
func w2EntreDobles(v string) bool {
	return !r2NoSeImprime(v) && !strings.ContainsAny(v, "\"$`%!\u201c\u201d\u201e\u201f") &&
		!strings.Contains(v, `\\`) && !strings.HasSuffix(v, `\`)
}

// w2FormaValor es como va en la linea de Windows el valor de --model,
// --effort o --resume.
func w2FormaValor(v, marcador string) string {
	switch {
	case w2Plano(v):
		return v
	case w2EntreDobles(v):
		return `"` + v + `"`
	}
	return marcador
}

// w2FormaPedido es como va en la linea de Windows el pedido ya recortado.
func w2FormaPedido(cut string) string {
	if w2EntreDobles(cut) {
		return `"` + cut + `"`
	}
	return "<pedido>"
}

type w2Rama int

const (
	w2Tal    w2Rama = iota // tal cual
	w2Dobles               // entre comillas dobles
	w2Marca                // su marcador
)

// w2Palabra es la palabra que el contrato pide para v en la rama dada,
// declarada a mano en cada tabla (no la calcula el oraculo).
func w2Palabra(v string, rama w2Rama, marcador string) string {
	switch rama {
	case w2Tal:
		return v
	case w2Dobles:
		return `"` + v + `"`
	}
	return marcador
}

var w2Marcadores = []string{"<pedido>", "<modelo>", "<esfuerzo>", "<sesion>"}

// w2SinMarcador exige que ningun valor de --model, --effort o --resume del
// CLI lanzado sea un marcador: el marcador es solo de la linea impresa.
// (Solo mira los valores: el resto del argv trae los documentos del rol.)
func w2SinMarcador(t *testing.T, ca string, args []string) {
	t.Helper()
	for _, flag := range []string{"--model", "--effort", "--resume"} {
		v, _ := vdtSigue(args, flag)
		for _, m := range w2Marcadores {
			if strings.Contains(v, m) {
				t.Fatalf("%s: el marcador %s es solo de la linea impresa, y el CLI lo recibio en %s: %q", ca, m, flag, args)
			}
		}
	}
}

// w2Trozos son los trozos imprimibles de v (de 3 bytes o mas, sin blancos
// en los bordes), cortando en cada caracter de control, byte invalido o runa
// que no se imprime.
func w2Trozos(v string) []string {
	var out []string
	var b strings.Builder
	corta := func() {
		if s := strings.TrimSpace(b.String()); len(s) >= 3 {
			out = append(out, s)
		}
		b.Reset()
	}
	for i := 0; i < len(v); {
		_, n := utf8.DecodeRuneInString(v[i:])
		if r2NoSeImprime(v[i : i+n]) {
			corta()
		} else {
			b.WriteString(v[i : i+n])
		}
		i += n
	}
	corta()
	return out
}

// w2LanzoElPedido exige que el CLI haya corrido una vez, sin marcadores en
// los valores de sus flags, con el pedido verdadero en su prompt (uno con
// control o bytes invalidos: cada trozo imprimible). Devuelve sus
// argumentos.
func w2LanzoElPedido(t *testing.T, ca, que string, f *vdtFalsos, pedido string) []string {
	t.Helper()
	args := vdtUnaLlamada(t, ca, que, f, "claude")
	w2SinMarcador(t, ca, args)
	ps := f.pedidos(t, "claude")
	if len(ps) != 1 {
		t.Fatalf("%s: fixture: un prompt, fueron %d", ca, len(ps))
	}
	if !r2NoSeImprime(pedido) {
		if !strings.Contains(ps[0], pedido) {
			t.Fatalf("%s: el CLI recibe el pedido verdadero %q (el marcador es solo de la linea): %q", ca, pedido, ps[0])
		}
		return args
	}
	for _, trozo := range w2Trozos(pedido) {
		if !strings.Contains(ps[0], trozo) {
			t.Fatalf("%s: el CLI recibe el pedido verdadero %s (falta %q; el marcador es solo de la linea): %q", ca,
				strconv.QuoteToASCII(pedido), trozo, ps[0])
		}
	}
	return args
}

// w2Valores son valores de --model, --effort con la rama que el contrato
// les da en Windows.
var w2Valores = []struct {
	v    string
	rama w2Rama
}{
	{"claude-opus-4.5", w2Tal},
	{"us.anthropic/claude:4+x_y-1", w2Tal},
	{"a,b", w2Dobles},   // tal cual en Unix
	{"x@y", w2Dobles},   // tal cual en Unix
	{"100%", w2Marca},   // tal cual en Unix
	{"%PATH%", w2Marca}, // tal cual en Unix
	{"m y", w2Dobles},
	{"it's", w2Dobles},
	{"a&b|c<d>e^f(g);h", w2Dobles},
	{`C:\dir\x`, w2Dobles},
	{"\u2018m\u2019 \u2014 x", w2Dobles}, // comillas simples tipograficas y raya: no excluidas
	{"m$x", w2Marca},
	{"m`x", w2Marca},
	{"m!x", w2Marca},
	{`m"x`, w2Marca},
	{"m\u201cx", w2Marca},
	{"m\u201dx", w2Marca},
	{"m\u201ex", w2Marca},
	{"m\u201fx", w2Marca},
	{`m\\x`, w2Marca},
	{`m\`, w2Marca},
	{"m\x01x", w2Marca},
	{"m\tx", w2Marca},
	{"m\x1b[2J", w2Marca},
	{"m\xffx", w2Marca},
	{"m\u202ex", w2Marca},
	{"m\u200bx", w2Marca},
}

// w2Pedidos son pedidos (de menos de 60 caracteres) con la rama que el
// contrato les da en Windows: entre comillas dobles o <pedido>.
var w2Pedidos = []struct {
	pedido string
	rama   w2Rama
}{
	{"Implementa el spec", w2Dobles},
	{"a & b | c < d > e ^ f ( g ) ; h , i @ j # k {l} = m ~ n * o ? [p]", w2Dobles},
	{"it's fine", w2Dobles},
	{`la ruta C:\dir\x y C:\otra`, w2Dobles},
	{"año ñandú ç € \u2018simples\u2019 \u2014 raya", w2Dobles},
	{"Implementa $(touch pwn) ya", w2Marca},
	{"lee $env:PATH", w2Marca},
	{"usa `touch pwn` aca", w2Marca},
	{"el 100% listo", w2Marca},
	{"ver %PATH% aca", w2Marca},
	{"hola mundo!", w2Marca},
	{"!x! retrasada", w2Marca},
	{`di "hola" y listo`, w2Marca},
	{"cita \u201c abierta", w2Marca},
	{"cita \u201d cerrada", w2Marca},
	{"cita \u201e baja", w2Marca},
	{"cita \u201f alta", w2Marca},
	{`doble \\ barra`, w2Marca},
	{`termina en C:\dir\`, w2Marca},
	{"campana \x07 aca", w2Marca},
	{"tab\taca", w2Marca},
	{"\x1b[31m rojo", w2Marca},
	{"borrar \x7f aca", w2Marca},
	{"byte \xff invalido", w2Marca},
	{"oculto \u202e al reves", w2Marca},
	{"cero\u200bancho", w2Marca},
	{"C1 \u0085 NEL", w2Marca},
}

// w2Recortes son los bordes del recorte: la forma se decide sobre el pedido
// como va en la linea (saltos como espacios, 60 caracteres y …), no sobre
// el pedido entero.
var w2Recortes = []struct{ pedido, want string }{
	{strings.Repeat("a", 60) + "$tail", `"` + strings.Repeat("a", 60) + `…"`},
	{strings.Repeat("a", 60) + "%PATH%", `"` + strings.Repeat("a", 60) + `…"`},
	{strings.Repeat("a", 60) + "\x01b", `"` + strings.Repeat("a", 60) + `…"`},
	{strings.Repeat("a", 60) + "\u201cb", `"` + strings.Repeat("a", 60) + `…"`},
	{strings.Repeat("a", 59) + `\`, "<pedido>"},                              // 60 caracteres que terminan en \
	{strings.Repeat("a", 59) + `\b`, `"` + strings.Repeat("a", 59) + `\…"`},  // la \ queda antes de …
	{strings.Repeat("a", 59) + `\\c`, `"` + strings.Repeat("a", 59) + `\…"`}, // el recorte parte la \\
	{strings.Repeat("a", 58) + `\\`, "<pedido>"},                             // 60 caracteres con \\ al final
	{strings.Repeat("a", 59) + "$", "<pedido>"},                              // el $ entra en los 60
	{"linea uno\nlinea dos", `"linea uno linea dos"`},                        // el salto como espacio
	{strings.Repeat("ñ", 70), `"` + strings.Repeat("ñ", 60) + `…"`},          // 60 caracteres, no bytes
}

// hallazgo 97284d, CA-503: el oraculo de este archivo (w2FormaValor,
// w2FormaPedido) da, para cada valor de las tablas, la rama que el contrato
// declara a mano; si estuviera mal, la propiedad no probaria nada. No mira
// a hoom.
func TestHallazgo_97284d_CA503_ElOraculoDeWindows(t *testing.T) {
	for _, c := range w2Valores {
		if got, want := w2FormaValor(c.v, "<modelo>"), w2Palabra(c.v, c.rama, "<modelo>"); got != want {
			t.Fatalf("hallazgo 97284d: el oraculo da %q para %s, el contrato %q", got, strconv.QuoteToASCII(c.v), want)
		}
	}
	for _, c := range w2Pedidos {
		cut := r2Corte(c.pedido)
		if got, want := w2FormaPedido(cut), w2Palabra(cut, c.rama, "<pedido>"); got != want {
			t.Fatalf("hallazgo 97284d: el oraculo da %q para el pedido %s, el contrato %q", got, strconv.QuoteToASCII(c.pedido), want)
		}
	}
	for _, c := range w2Recortes {
		if got := w2FormaPedido(r2Corte(c.pedido)); got != c.want {
			t.Fatalf("hallazgo 97284d: el oraculo da %q para el pedido %s, el contrato %q", got, strconv.QuoteToASCII(c.pedido), c.want)
		}
	}
}

// ---------------------------------------------------------------------------
// el pedido
// ---------------------------------------------------------------------------

// hallazgo 97284d, CA-503: en Windows el pedido va entre comillas dobles
// cuando se imprime y no tiene ", $, `, %, !, comillas dobles tipograficas,
// \\ ni una \ al final; si no, como <pedido>. La linea es exactamente la de
// Unix con esa palabra al final, sin crudos de control, y el CLI recibe el
// pedido verdadero. El pedido propuesto (sin -m y sin terminal) sigue la
// misma regla.
func TestHallazgo_97284d_CA503_PedidoEnWindows(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 97284d", root, "precios", "")
	for _, c := range w2Pedidos {
		ca := "hallazgo 97284d, CA-503 (Windows, pedido " + strconv.QuoteToASCII(c.pedido) + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", c.pedido)
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		if want := r1Base + w2Palabra(r2Corte(c.pedido), c.rama, "<pedido>"); linea != want {
			t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
		}
		w2LanzoElPedido(t, ca, "pedir writer", f, c.pedido)
	}

	ca := "hallazgo 97284d, CA-503 (Windows, pedido propuesto)"
	propuesto := vdtAccion(t, ca, vdtCarta(t, root, "precios"), boardcmd.ActPedirWriter).Pedido
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "writer", "precios")
	linea := vdtFlechaDe(t, ca, s.out)
	r1SinControlCrudo(t, ca, linea)
	if want := r1Base + w2FormaPedido(r2Corte(propuesto)); linea != want {
		t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
	}
	w2LanzoElPedido(t, ca, "pedir writer", f, propuesto)
}

// hallazgo 97284d, CA-503 (borde): en Windows la forma del pedido se decide
// sobre el pedido como va en la linea (los saltos como espacios, recortado a
// 60 caracteres con …), igual que en Unix: lo que el recorte deja afuera no
// cuenta, una \ que queda antes de … no es una \ al final, y una \ o un $
// dentro de los 60 si cuentan. El CLI recibe el pedido entero.
func TestHallazgo_97284d_CA503_BordeDelRecorteEnWindows(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 97284d", root, "precios", "")
	for _, c := range w2Recortes {
		ca := "hallazgo 97284d, CA-503 (Windows, recorte de " + strconv.QuoteToASCII(c.pedido) + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", c.pedido)
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		if want := r1Base + c.want; linea != want {
			t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
		}
		w2LanzoElPedido(t, ca, "pedir writer", f, c.pedido)
	}
}

// ---------------------------------------------------------------------------
// --model y --effort
// ---------------------------------------------------------------------------

// hallazgo 97284d, CA-503: en Windows --model y --effort van tal cual solo
// con bytes de [A-Za-z0-9._:/+-] (a,b, x@y y 100% no: en Unix si); si no,
// entre comillas dobles cuando se imprimen y no tienen ", $, `, %, !,
// comillas dobles tipograficas, \\ ni una \ al final; si no, como <modelo> y
// <esfuerzo> (el flag queda). Cada valor de la tabla va una vez como modelo
// y una como esfuerzo, del comando; la linea es exacta y el CLI recibe los
// valores verdaderos.
func TestHallazgo_97284d_CA503_ModeloYEsfuerzoEnWindows(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 97284d", root, "precios", "")
	n := len(w2Valores)
	for i := range w2Valores {
		m, e := w2Valores[i], w2Valores[(i+1)%n]
		ca := fmt.Sprintf("hallazgo 97284d, CA-503 (Windows, --model %s --effort %s)", strconv.QuoteToASCII(m.v), strconv.QuoteToASCII(e.v))
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "writer", "precios", "--model", m.v, "--effort", e.v, "-m", "Implementa")
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		want := r1Base + "--model " + w2Palabra(m.v, m.rama, "<modelo>") + " --effort " + w2Palabra(e.v, e.rama, "<esfuerzo>") + ` "Implementa"`
		if linea != want {
			t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
		}
		args := w2LanzoElPedido(t, ca, "pedir writer", f, "Implementa")
		if v, _ := vdtSigue(args, "--model"); v != m.v {
			t.Fatalf("%s: el provider recibe el modelo verdadero %q: %q", ca, m.v, args)
		}
		if v, _ := vdtSigue(args, "--effort"); v != e.v {
			t.Fatalf("%s: el provider recibe el esfuerzo verdadero %q: %q", ca, e.v, args)
		}
	}
}

// hallazgo 97284d, CA-503: los de hoom.yaml siguen la misma regla: un modelo
// x@y va entre comillas dobles y un esfuerzo con $ como <esfuerzo>; el
// provider los recibe tal cual.
func TestHallazgo_97284d_CA503_ModeloYEsfuerzoDeHoomYamlEnWindows(t *testing.T) {
	const ca = "hallazgo 97284d, CA-503 (Windows, hoom.yaml)"
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    model: \"x@y\"\n    effort: \"it's $x\"\n")
	vdtWriter(t, ca, root, "precios", "")
	s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa")
	want := r1Base + `--model "x@y" --effort <esfuerzo> "Implementa"`
	if linea := vdtFlechaDe(t, ca, s.out); linea != want {
		t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
	}
	args := w2LanzoElPedido(t, ca, "pedir writer", f, "Implementa")
	if v, _ := vdtSigue(args, "--model"); v != "x@y" {
		t.Fatalf("%s: el provider recibe el modelo de hoom.yaml x@y: %q", ca, args)
	}
	if v, _ := vdtSigue(args, "--effort"); v != "it's $x" {
		t.Fatalf("%s: el provider recibe el esfuerzo de hoom.yaml %q: %q", ca, "it's $x", args)
	}
}

// ---------------------------------------------------------------------------
// --resume, hoom siguiente y el reviewer
// ---------------------------------------------------------------------------

// hallazgo 97284d, CA-503 y CA-511: en Windows el valor de --resume de hoom
// reanudar va tal cual con bytes de [A-Za-z0-9._:/+-], entre comillas dobles
// si se puede (ses@1,2) y si no como <sesion>; el pedido de reanudar sigue
// la regla del pedido. El provider retoma la sesion verdadera.
func TestHallazgo_97284d_CA503_ResumeEnWindows(t *testing.T) {
	casos := []struct {
		sesion     string
		rama       w2Rama
		pedido     string
		ramaPedido w2Rama
	}{
		{"ses-1.2_3:4/5+6", w2Tal, "Termina lo que quedo", w2Dobles},
		{"ses@1,2", w2Dobles, "Termina lo que quedo", w2Dobles},
		{"ses 1&2", w2Dobles, "Termina el 100%", w2Marca},
		{"ses%1", w2Marca, "Termina lo que quedo", w2Dobles},
		{"ses$x", w2Marca, "Termina `ya`", w2Marca},
		{"ses\x01", w2Marca, "Termina lo que quedo", w2Dobles},
		{"ses\u201c1", w2Marca, "Termina lo que quedo", w2Dobles},
	}
	for _, c := range casos {
		ca := "hallazgo 97284d, CA-503 (Windows, --resume " + strconv.QuoteToASCII(c.sesion) + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, "")
		r1Interrumpida(t, ca, root, "precios", c.sesion)
		s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "reanudar", "precios", "-m", c.pedido)
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		if !strings.HasPrefix(linea, r1Base) {
			t.Fatalf("%s: la linea → empieza con %q: %q", ca, r1Base, linea)
		}
		if w := " --resume " + w2Palabra(c.sesion, c.rama, "<sesion>") + " "; !strings.Contains(linea, w) {
			t.Fatalf("%s: la linea lleva %q:\n%s", ca, strings.TrimSpace(w), linea)
		}
		if w := " " + w2Palabra(c.pedido, c.ramaPedido, "<pedido>"); !strings.HasSuffix(linea, w) {
			t.Fatalf("%s: la linea termina con el pedido %q:\n%s", ca, strings.TrimSpace(w), linea)
		}
		args := w2LanzoElPedido(t, ca, "reanudar", f, c.pedido)
		if v, _ := vdtSigue(args, "--resume"); v != c.sesion {
			t.Fatalf("%s: el provider retoma la sesion verdadera %q: %q", ca, c.sesion, args)
		}
	}
}

// hallazgo 97284d, CA-503 y CA-507: hoom siguiente imprime la linea de hoom
// pedir, con la regla de Windows, y lanza con los valores verdaderos.
func TestHallazgo_97284d_CA503_SiguienteEnWindows(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 97284d", root, "precios", "")
	casos := []struct {
		args                  []string
		want                  string
		model, effort, pedido string
	}{
		{[]string{"-m", "Implementa $(touch pwn) ya"}, r1Base + "<pedido>", "", "", "Implementa $(touch pwn) ya"},
		{[]string{"-m", "Implementa a,b y x@y"}, r1Base + `"Implementa a,b y x@y"`, "", "", "Implementa a,b y x@y"},
		{[]string{"--model", "a,b", "--effort", "100%", "-m", "Implementa"}, r1Base + `--model "a,b" --effort <esfuerzo> "Implementa"`,
			"a,b", "100%", "Implementa"},
	}
	for _, c := range casos {
		ca := "hallazgo 97284d, CA-503 (Windows, hoom siguiente " + strconv.QuoteToASCII(strings.Join(c.args, " ")) + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "siguiente", append([]string{"precios"}, c.args...)...)
		if linea := vdtFlechaDe(t, ca, s.out); linea != c.want {
			t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, c.want, linea)
		}
		args := w2LanzoElPedido(t, ca, "siguiente", f, c.pedido)
		if c.model != "" {
			if v, _ := vdtSigue(args, "--model"); v != c.model {
				t.Fatalf("%s: el provider recibe el modelo verdadero %q: %q", ca, c.model, args)
			}
			if v, _ := vdtSigue(args, "--effort"); v != c.effort {
				t.Fatalf("%s: el provider recibe el esfuerzo verdadero %q: %q", ca, c.effort, args)
			}
		}
	}
}

// hallazgo 97284d, CA-503: la → del reviewer (hoom review ...) en Windows:
// el modelo y el esfuerzo de review: siguen la regla (r,1 entre comillas
// dobles, e% como <esfuerzo>; r$1 como <modelo>, high tal cual); cada CLI
// que lanza la review recibe el modelo verdadero (y el esfuerzo verdadero
// si lo recibe), nunca un marcador.
func TestHallazgo_97284d_CA503_FlechaDelReviewerEnWindows(t *testing.T) {
	const pre = "→ hoom review --task precios --spec .hoom/specs/precios.md --provider claude"
	casos := []struct{ yaml, model, effort, wantM, wantE string }{
		{"review:\n  model: \"r,1\"\n  effort: \"e%\"\n", "r,1", "e%", `"r,1"`, "<esfuerzo>"},
		{"review:\n  model: \"r$1\"\n  effort: high\n", "r$1", "high", "<modelo>", "high"},
	}
	for _, c := range casos {
		ca := "hallazgo 97284d, CA-503 (Windows, reviewer --model " + strconv.QuoteToASCII(c.model) + ")"
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, c.yaml)
		vdtReview(t, ca, root, "precios")
		s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "reviewer", "precios")
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		if !strings.HasPrefix(linea, pre) {
			t.Fatalf("%s: la → del reviewer empieza con %q: %q", ca, pre, linea)
		}
		for _, k := range []struct{ flag, w string }{{"--model", c.wantM}, {"--effort", c.wantE}} {
			if !strings.Contains(linea+" ", " "+k.flag+" "+k.w+" ") {
				t.Fatalf("%s: la → del reviewer lleva %s %s:\n%s", ca, k.flag, k.w, linea)
			}
		}
		ll := f.llamadas(t, "claude")
		if len(ll) == 0 {
			t.Fatalf("%s: fixture: la review lanza el CLI:\n%s", ca, s)
		}
		for _, args := range ll {
			w2SinMarcador(t, ca, args)
			if v, ok := vdtSigue(args, "--model"); !ok || v != c.model {
				t.Fatalf("%s: el CLI de la review recibe el modelo verdadero %q: %q", ca, c.model, args)
			}
			if v, ok := vdtSigue(args, "--effort"); ok && v != c.effort {
				t.Fatalf("%s: el CLI de la review recibe el esfuerzo verdadero %q: %q", ca, c.effort, args)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// el resto de la linea, y fuera de Windows
// ---------------------------------------------------------------------------

// hallazgo 97284d, CA-503: en Windows el resto de la linea es como en Unix:
// rol, tarea, spec, provider, --budget-usd y --timeout (de hoom.yaml, de la
// tarjeta o del comando) en su lugar; el arquitecto sin --spec. Solo cambian
// las palabras de los valores. El provider recibe los valores verdaderos.
func TestHallazgo_97284d_CA503_ElRestoDeLaLineaComoEnUnix(t *testing.T) {
	ca := "hallazgo 97284d, CA-503 (Windows, hoom.yaml y tarjeta)"
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    model: \"a,b\"\n    effort: \"x@y\"\n    timeout: 45m\n")
	vdtWriter(t, ca, root, "precios", "presupuesto_usd: 5\n")
	s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa el 100%")
	want := `→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude --model "a,b" --effort "x@y" --budget-usd 5 --timeout 45m <pedido>`
	if linea := vdtFlechaDe(t, ca, s.out); linea != want {
		t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
	}
	args := w2LanzoElPedido(t, ca, "pedir writer", f, "Implementa el 100%")
	for _, k := range []struct{ flag, v string }{{"--model", "a,b"}, {"--effort", "x@y"}, {"--max-budget-usd", "5"}} {
		if v, _ := vdtSigue(args, k.flag); v != k.v {
			t.Fatalf("%s: el provider recibe %s %s: %q", ca, k.flag, k.v, args)
		}
	}

	ca = "hallazgo 97284d, CA-503 (Windows, del comando)"
	f = vdtPATH(t)
	f.cli(t, "claude", "")
	s = w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "writer", "precios", "--provider", "claude", "--model", "m-cmd",
		"--effort", "low", "--budget-usd", "2.5", "--timeout", "1h30m", "-m", "Implementa")
	want = `→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude --model m-cmd --effort low --budget-usd 2.5 --timeout 1h30m "Implementa"`
	if linea := vdtFlechaDe(t, ca, s.out); linea != want {
		t.Fatalf("%s: con valores planos la linea de Windows es la de Unix:\n  %s\nfue\n  %s", ca, want, linea)
	}
	w2LanzoElPedido(t, ca, "pedir writer", f, "Implementa")

	ca = "hallazgo 97284d, CA-503 (Windows, arquitecto en Backlog)"
	f = vdtPATH(t)
	f.cli(t, "claude", "")
	root = vdtProyecto(t, "")
	vdtBacklog(t, ca, root, "precios")
	s = w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "arquitecto", "precios", "-m", "Escribi el spec, 100% listo")
	want = `→ hoom agent --role arquitecto --task precios --provider claude <pedido>`
	if linea := vdtFlechaDe(t, ca, s.out); linea != want {
		t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
	}
	w2LanzoElPedido(t, ca, "pedir arquitecto", f, "Escribi el spec, 100% listo")
}

// hallazgo 97284d, CA-503 (control): con GOOS "", "darwin" o "linux" nada
// cambia: los mismos valores dan la linea POSIX de las rondas 1 y 2 (a,b y
// 100% tal cual, el pedido entre simples o "$(printf ...)"), igual en las
// tres; con "windows", la de Windows. La linea valores: no depende de la
// plataforma (el spec solo cambia la →), y el provider recibe lo mismo.
func TestHallazgo_97284d_CA503_FueraDeWindowsLaLineaPOSIX(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 97284d", root, "precios", "")
	casos := []struct{ model, effort, pedido string }{
		{"a,b", "100%", `di "hola" en C:\dir\x al 100%`},
		{"x@y", "it's", "hola mundo!"},
		{"m\x01x", "e\u201cx", "Implementa $(touch pwn) ya"},
	}
	for _, c := range casos {
		valores := map[string]string{}
		plataformas := []string{"", "darwin", "linux", w2Windows}
		for _, goos := range plataformas {
			ca := fmt.Sprintf("hallazgo 97284d, CA-503 (GOOS %q, --model %s --effort %s -m %s)", goos,
				strconv.QuoteToASCII(c.model), strconv.QuoteToASCII(c.effort), strconv.QuoteToASCII(c.pedido))
			f := vdtPATH(t)
			f.cli(t, "claude", "")
			s := w2Correr(t, ca, goos, vdtOpts{dir: root}, "pedir", "writer", "precios", "--model", c.model, "--effort", c.effort, "-m", c.pedido)
			linea := vdtFlechaDe(t, ca, s.out)
			want := r1Base + "--model " + r2FormaValor(c.model) + " --effort " + r2FormaValor(c.effort) + " " + r2FormaPedido(r2Corte(c.pedido))
			if goos == w2Windows {
				want = r1Base + "--model " + w2FormaValor(c.model, "<modelo>") + " --effort " + w2FormaValor(c.effort, "<esfuerzo>") +
					" " + w2FormaPedido(r2Corte(c.pedido))
			}
			if linea != want {
				t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
			}
			valores[goos] = vdtValoresDe(t, ca, s.out)
			args := w2LanzoElPedido(t, ca, "pedir writer", f, c.pedido)
			if v, _ := vdtSigue(args, "--model"); v != c.model {
				t.Fatalf("%s: el provider recibe el modelo verdadero %q: %q", ca, c.model, args)
			}
			if v, _ := vdtSigue(args, "--effort"); v != c.effort {
				t.Fatalf("%s: el provider recibe el esfuerzo verdadero %q: %q", ca, c.effort, args)
			}
		}
		for _, goos := range plataformas[1:] {
			if valores[goos] != valores[""] {
				t.Fatalf("hallazgo 97284d, CA-503: la linea valores: no depende de la plataforma:\n GOOS \"\"  %q\n GOOS %q %q", valores[""], goos, valores[goos])
			}
		}
	}
}

// ---------------------------------------------------------------------------
// propiedad
// ---------------------------------------------------------------------------

// Las piezas de los valores al azar: planas, las que pueden ir entre
// comillas dobles, y las que no.
var (
	w2Planas   = []string{"a", "Z", "0", "9", ".", "_", ":", "/", "+", "-"}
	w2Citables = append(append([]string{}, w2Planas...), " ", "'", "&", "|", "^", "<", ">", "(", ")", ";", ",", "@", "#", "=",
		"~", "*", "?", "[", "]", "{", "}", `\`, "ñ", "€", "\u2018", "\u2019", "\u2014", "…")
	w2Hostiles = append(append([]string{}, w2Citables...), "\x01", "\t", "\x1b", "\x7f", "\xff", "\u202e", "\u200b", "\u0085",
		`"`, "$", "`", "%", "!", "\u201c", "\u201d", "\u201e", "\u201f", `\\`, "%PATH%", "!x!", "$env:X", "$(touch pwn)", "`touch pwn`")
)

// w2AlAzar arma un valor de una de las tres bolsas: empieza con una letra y
// termina con una letra o, a veces, con \.
func w2AlAzar(r *rand.Rand, max int) string {
	bolsa := [][]string{w2Planas, w2Citables, w2Hostiles}[r.IntN(3)]
	var b strings.Builder
	b.WriteString(string(rune('a' + r.IntN(26))))
	for i, n := 0, r.IntN(max); i < n; i++ {
		b.WriteString(bolsa[r.IntN(len(bolsa))])
	}
	if r.IntN(6) == 0 {
		b.WriteString(`\`)
	} else {
		b.WriteString(string(rune('a' + r.IntN(26))))
	}
	return b.String()
}

// hallazgo 97284d, CA-503, como propiedad: para modelos, esfuerzos y
// pedidos al azar (semilla fija), en Windows la linea → es exactamente la
// del contrato (cada valor tal cual, entre comillas dobles o su marcador),
// no lleva crudo ningun caracter de control, y el provider recibe los
// valores verdaderos. Las tres ramas aparecen (se comprueba antes de correr
// hoom).
func TestHallazgo_97284d_CA503_PropiedadEnWindows(t *testing.T) {
	r := rand.New(rand.NewPCG(97284, 0xd))
	type caso struct{ model, effort, pedido string }
	var casos []caso
	ramas := map[string]int{}
	for i := 0; i < 16; i++ {
		c := caso{model: w2AlAzar(r, 6), effort: w2AlAzar(r, 6), pedido: w2AlAzar(r, 10)}
		for utf8.RuneCountInString(c.pedido) > 60 {
			c.pedido = w2AlAzar(r, 10)
		}
		casos = append(casos, c)
		for _, v := range []string{c.model, c.effort} {
			switch w2FormaValor(v, "<marca>") {
			case v:
				ramas["valor tal cual"]++
			case "<marca>":
				ramas["valor marcador"]++
			default:
				ramas["valor entre dobles"]++
			}
		}
		if w2FormaPedido(c.pedido) == "<pedido>" {
			ramas["pedido marcador"]++
		} else {
			ramas["pedido entre dobles"]++
		}
	}
	for _, k := range []string{"valor tal cual", "valor marcador", "valor entre dobles", "pedido marcador", "pedido entre dobles"} {
		if ramas[k] == 0 {
			t.Fatalf("hallazgo 97284d: fixture: la semilla no cubre la rama %q: %v", k, ramas)
		}
	}

	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 97284d", root, "precios", "")
	for i, c := range casos {
		ca := fmt.Sprintf("hallazgo 97284d, CA-503 (Windows, al azar %d: --model %s --effort %s -m %s)", i,
			strconv.QuoteToASCII(c.model), strconv.QuoteToASCII(c.effort), strconv.QuoteToASCII(c.pedido))
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		s := w2Correr(t, ca, w2Windows, vdtOpts{dir: root}, "pedir", "writer", "precios", "--model", c.model, "--effort", c.effort, "-m", c.pedido)
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		want := r1Base + "--model " + w2FormaValor(c.model, "<modelo>") + " --effort " + w2FormaValor(c.effort, "<esfuerzo>") +
			" " + w2FormaPedido(r2Corte(c.pedido))
		if linea != want {
			t.Fatalf("%s: la linea → es\n  %s\nfue\n  %s", ca, want, linea)
		}
		args := w2LanzoElPedido(t, ca, "pedir writer", f, c.pedido)
		if v, _ := vdtSigue(args, "--model"); v != c.model {
			t.Fatalf("%s: el provider recibe el modelo verdadero %q: %q", ca, c.model, args)
		}
		if v, _ := vdtSigue(args, "--effort"); v != c.effort {
			t.Fatalf("%s: el provider recibe el esfuerzo verdadero %q: %q", ca, c.effort, args)
		}
	}
}
