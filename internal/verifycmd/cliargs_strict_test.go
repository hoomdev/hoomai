// Tests adversariales del spec .hoom/specs/verify-args-estrictos.md
// (CA-217, CA-218, CA-219, CA-220, CA-224, CA-227) sobre el MECANISMO desnudo:
// `cliargs.Strict` no conoce ningun verbo, asi que aca se prueba la disciplina
// de argumentos con un verbo de mentira. Si la disciplina viviera dentro de
// `verifycmd` en vez de en `cliargs`, estos tests no compilarian: esa es la
// intencion (el spec la declara reutilizable), no un detalle.
//
// Nota de ubicacion: el arbol de este agente no permite crear directorios, asi
// que los tests del paquete `cliargs` viven aca como CONSUMIDOR externo (solo
// tocan API exportada: Strict, UsageError, ErrHelp). Mudarlos a
// internal/cliargs/cliargs_test.go es copiar el archivo y cambiar el package.
package verifycmd

import (
	"bytes"
	"errors"
	"flag"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/cliargs"
)

// usoDemo imita un bloque de uso real: multilinea, con linea en blanco en el
// medio y una linea que declara que el verbo no tiene posicionales. Si Strict
// lo recorta, lo reordena o lo cambia por el del paquete flag, se nota.
const usoDemo = `Uso: hoom demo [--full] [--gate a,b]

  --full      corre todo
  --gate a,b  corre solo esos

'hoom demo' no tiene subcomandos ni argumentos posicionales.
Accion: para leer resultados ya emitidos usa 'hoom report'.`

func fsDemo() *flag.FlagSet {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.Bool("full", false, "")
	fs.String("gate", "", "")
	return fs
}

// comoUso exige que el error sea el tipo del contrato y que su exit code sea
// el invariante 2.
func comoUso(t *testing.T, ca string, err error) *cliargs.UsageError {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: esperaba un rechazo, no un nil", ca)
	}
	var ue *cliargs.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("%s: esperaba *cliargs.UsageError, fue %T: %v", ca, err, err)
	}
	if ue.ExitCode() != 2 {
		t.Fatalf("%s: el exit code de un UsageError es 2 e invariante, fue %d", ca, ue.ExitCode())
	}
	return ue
}

func primeraLinea(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// exigeBloque: cada linea util del bloque de uso tiene que estar en el
// mensaje. Compara linea por linea (y no el bloque entero de una) para no
// atarse a como se pega el salto de linea, pero sin permitir que falte nada.
func exigeBloque(t *testing.T, ca, msg, bloque string) {
	t.Helper()
	for _, l := range strings.Split(bloque, "\n") {
		l = strings.TrimRight(l, " \t")
		if strings.TrimSpace(l) == "" {
			continue
		}
		if !strings.Contains(msg, l) {
			t.Fatalf("%s: al mensaje le falta la linea del uso %q\n--- mensaje ---\n%s", ca, l, msg)
		}
	}
}

// CA-217: CUALQUIER argumento posicional es un rechazo, y el rechazo nombra el
// PRIMER token no reconocido. El terminador `--` no habilita operandos: el
// verbo no tiene operandos en ninguna sintaxis.
func TestCA217_StrictRechazaTodoPosicional(t *testing.T) {
	casos := []struct {
		args   []string
		primer string
	}{
		{[]string{"show", "abc123"}, "show"},
		{[]string{"."}, "."},
		{[]string{"show", "--full"}, "show"}, // flag.Parse corta en 'show'
		{[]string{"--full", "show"}, "show"}, // ...y aca lo ve despues de --full
		{[]string{"--", "show"}, "show"},     // '--' no habilita operandos
		{[]string{"--gate", "x", "y"}, "y"},  // el valor de un flag no es posicional
		{[]string{"show", "--help"}, "show"}, // pedir ayuda no rescata un pedido roto
		{[]string{""}, ""},                   // un argumento vacio sigue siendo un argumento
		{[]string{"ñandú", "show"}, "ñandú"}, // unicode: el token viaja tal cual
		{[]string{"   "}, "   "},             // solo espacios
		{[]string{"a b", "c"}, "a b"},        // con espacio adentro
		{[]string{"🚀"}, "🚀"},                 // fuera del BMP
	}
	for _, c := range casos {
		ue := comoUso(t, "CA-217", cliargs.Strict(fsDemo(), c.args, "demo", usoDemo))
		if ue.Verb != "demo" {
			t.Fatalf("CA-217 %v: el error nombra su verbo, Verb=%q", c.args, ue.Verb)
		}
		quiero := "hoom demo: argumento posicional no reconocido: " + strconv.Quote(c.primer)
		if got := primeraLinea(ue.Error()); got != quiero {
			t.Fatalf("CA-217 %v: primera linea\n  quiero: %s\n  tengo:  %s", c.args, quiero, got)
		}
		exigeBloque(t, "CA-217", ue.Error(), usoDemo)
	}
}

// tokenHostil genera posicionales feos pero imprimibles: unicode, espacios,
// emoji, puntos y barras. Excluye comillas y backslash para que citar el token
// sea comparable sin pelearse con el escapado.
type tokenHostil string

func (tokenHostil) Generate(rnd *rand.Rand, size int) reflect.Value {
	alfabeto := []rune("abcXYZ019._/=:@ñéÑÜÇ日本語Ωμ🚀 ")
	n := 1 + rnd.Intn(14)
	out := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, alfabeto[rnd.Intn(len(alfabeto))])
	}
	s := string(out)
	if strings.HasPrefix(s, "-") { // un token con guion es un flag, no un posicional
		s = "x" + s
	}
	return reflect.ValueOf(tokenHostil(s))
}

// CA-217 (propiedad): no hay token posicional afortunado. Para CUALQUIER token
// que no empiece con guion, un solo argumento alcanza para el rechazo, con
// exit 2 y el token citado. Es una propiedad, no una lista de ejemplos.
func TestCA217_PropiedadTodoTokenEsRechazado(t *testing.T) {
	f := func(tok tokenHostil) bool {
		err := cliargs.Strict(fsDemo(), []string{string(tok)}, "demo", usoDemo)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || ue.ExitCode() != 2 {
			t.Logf("CA-217: %q no fue rechazado con exit 2: %v", string(tok), err)
			return false
		}
		if !strings.Contains(ue.Error(), strconv.Quote(string(tok))) {
			t.Logf("CA-217: el mensaje no cita %q: %s", string(tok), ue.Error())
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("CA-217: %v", err)
	}
}

// CA-218: el bloque de uso que recibe Strict viaja VERBATIM al mensaje. Strict
// no lo resume, no lo reescribe y no le inventa una sintaxis.
func TestCA218_StrictNoTocaElBloqueDeUso(t *testing.T) {
	ue := comoUso(t, "CA-218", cliargs.Strict(fsDemo(), []string{"show", "abc"}, "demo", usoDemo))
	if ue.Usage != usoDemo {
		t.Fatalf("CA-218: Usage debe ser el bloque recibido, sin tocar:\n--- quiero ---\n%s\n--- tengo ---\n%s", usoDemo, ue.Usage)
	}
	if !strings.Contains(ue.Error(), "no tiene subcomandos ni argumentos posicionales") {
		t.Fatalf("CA-218: el rechazo debe incluir la linea que declara que el verbo no tiene subcomandos:\n%s", ue.Error())
	}
	exigeBloque(t, "CA-218", ue.Error(), usoDemo)
}

// CA-219: un flag no definido sale 2 CON EL MENSAJE DE HOOM. El texto del
// paquete flag ("Usage of demo:", "flag provided but not defined") no aparece
// jamas: ni en el error, ni escrito por cuenta del propio paquete.
func TestCA219_StrictFlagDesconocidoConMensajePropio(t *testing.T) {
	// El caller le pone un buffer al FlagSet: si Strict no silencia la salida
	// del paquete flag, el uso ajeno aparece aca.
	var ajeno bytes.Buffer
	fs := fsDemo()
	fs.SetOutput(&ajeno)

	ue := comoUso(t, "CA-219", cliargs.Strict(fs, []string{"--bogus"}, "demo", usoDemo))
	if ajeno.Len() != 0 {
		t.Fatalf("CA-219: el paquete flag escribio por su cuenta; Strict debe silenciarlo:\n%s", ajeno.String())
	}
	if got, quiero := primeraLinea(ue.Error()), "hoom demo: flag desconocido: --bogus"; got != quiero {
		t.Fatalf("CA-219: primera linea\n  quiero: %s\n  tengo:  %s", quiero, got)
	}
	exigeBloque(t, "CA-219", ue.Error(), usoDemo)

	// -bogus, --bogus=1 y una mayuscula (los flags de Go son sensibles a
	// mayusculas) son el mismo rechazo; el nombre tiene que estar en la linea.
	for _, args := range [][]string{{"-bogus"}, {"--bogus=1"}, {"--Full"}, {"--full", "--bogus"}} {
		ue := comoUso(t, "CA-219", cliargs.Strict(fsDemo(), args, "demo", usoDemo))
		msg := ue.Error()
		if !strings.HasPrefix(msg, "hoom demo: flag desconocido: ") {
			t.Fatalf("CA-219 %v: esperaba la razon 'flag desconocido', fue %q", args, primeraLinea(msg))
		}
		nombre := strings.TrimPrefix(strings.Split(args[len(args)-1], "=")[0], "--")
		nombre = strings.TrimPrefix(nombre, "-")
		if !strings.Contains(primeraLinea(msg), nombre) {
			t.Fatalf("CA-219 %v: el mensaje debe nombrar el flag %q: %q", args, nombre, primeraLinea(msg))
		}
		exigeBloque(t, "CA-219", msg, usoDemo)
	}
}

// argvMalo genera invocaciones invalidas de varias familias: posicionales,
// flags inventados y flags sin valor.
type argvMalo []string

func (argvMalo) Generate(rnd *rand.Rand, size int) reflect.Value {
	familias := [][]string{
		{"show", "abc"}, {"."}, {"--", "x"}, {"--full", "algo"},
		{"--bogus"}, {"-bogus"}, {"--bogus=1"}, {"--Full"},
		{"--gate"}, {"--full", "--gate"},
		{""}, {"   "}, {"日本語"}, {"🚀", "--full"},
	}
	return reflect.ValueOf(argvMalo(familias[rnd.Intn(len(familias))]))
}

// CA-219 (propiedad): pase lo que pase, el mensaje ajeno NUNCA aparece y el
// bloque de hoom SIEMPRE aparece completo. La grieta original era exactamente
// esta: hoom no era dueño de su propio mensaje.
func TestCA219_PropiedadNuncaElMensajeAjeno(t *testing.T) {
	ajenas := []string{"Usage of demo:", "Usage of ", "flag provided but not defined", "flag needs an argument"}
	f := func(args argvMalo) bool {
		err := cliargs.Strict(fsDemo(), []string(args), "demo", usoDemo)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || ue.ExitCode() != 2 {
			t.Logf("CA-219: %v no fue rechazado con exit 2: %v", []string(args), err)
			return false
		}
		msg := ue.Error()
		for _, a := range ajenas {
			if strings.Contains(msg, a) {
				t.Logf("CA-219: %v filtro texto ajeno %q:\n%s", []string(args), a, msg)
				return false
			}
		}
		for _, l := range strings.Split(usoDemo, "\n") {
			if strings.TrimSpace(l) == "" {
				continue
			}
			if !strings.Contains(msg, strings.TrimRight(l, " \t")) {
				t.Logf("CA-219: %v perdio la linea %q del uso", []string(args), l)
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("CA-219: %v", err)
	}
}

// CA-220: un flag DEFINIDO al que le falta el valor es un rechazo de hoom, no
// del paquete flag.
func TestCA220_StrictFlagDefinidoSinValor(t *testing.T) {
	for _, args := range [][]string{{"--gate"}, {"-gate"}, {"--full", "--gate"}} {
		ue := comoUso(t, "CA-220", cliargs.Strict(fsDemo(), args, "demo", usoDemo))
		if got, quiero := primeraLinea(ue.Error()), "hoom demo: --gate necesita un valor"; got != quiero {
			t.Fatalf("CA-220 %v: primera linea\n  quiero: %s\n  tengo:  %s", args, quiero, got)
		}
		if strings.Contains(ue.Error(), "flag needs an argument") {
			t.Fatalf("CA-220 %v: filtro el mensaje del paquete flag:\n%s", args, ue.Error())
		}
		exigeBloque(t, "CA-220", ue.Error(), usoDemo)
	}
}

// CA-224: pedir ayuda no es un error. -h y --help devuelven ErrHelp, que NO es
// un UsageError: un UsageError sale 2 y la ayuda sale 0.
func TestCA224_StrictHelpNoEsError(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"--full", "--help"}} {
		err := cliargs.Strict(fsDemo(), args, "demo", usoDemo)
		if !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-224 %v: esperaba ErrHelp, fue %T: %v", args, err, err)
		}
		var ue *cliargs.UsageError
		if errors.As(err, &ue) {
			t.Fatalf("CA-224 %v: la ayuda no puede ser un UsageError (saldria 2, y sale 0)", args)
		}
	}
}

// CA-227: el exit code de un UsageError es 2 y es invariante — no depende del
// verbo, de la razon ni del texto. Y el mensaje respeta el formato declarado:
// "hoom <verb>: <reason>\n\n<usage>\n<action>". Propiedad sobre cualquier
// contenido, incluidos vacios, unicode y saltos de linea.
func TestCA227_ExitCodeInvarianteYFormato(t *testing.T) {
	f := func(verb, reason, usage, action string) bool {
		e := &cliargs.UsageError{Verb: verb, Reason: reason, Usage: usage, Action: action}
		if e.ExitCode() != 2 {
			t.Logf("CA-227: ExitCode devolvio %d para %+v", e.ExitCode(), e)
			return false
		}
		quiero := "hoom " + verb + ": " + reason + "\n\n" + usage + "\n" + action
		if e.Error() != quiero {
			t.Logf("CA-227: formato\n  quiero: %q\n  tengo:  %q", quiero, e.Error())
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("CA-227: %v", err)
	}
}

// CA-225 (control del mecanismo): Strict no inventa rechazos. Sin argumentos,
// con flags definidos, con `--` solo y con el dialecto de Go (un guion, dos
// guiones, con `=`), devuelve nil y deja el FlagSet parseado de verdad.
func TestCA225_StrictNoInventaRechazos(t *testing.T) {
	casos := [][]string{
		{}, {"--"}, {"--full"}, {"-full"},
		{"--gate", "test"}, {"--gate=test"}, {"-gate", "test"},
		{"--full", "--gate", "test"},
	}
	for _, args := range casos {
		if err := cliargs.Strict(fsDemo(), args, "demo", usoDemo); err != nil {
			t.Fatalf("CA-225: %v es valido y fue rechazado: %v", args, err)
		}
	}
	fs := fsDemo()
	if err := cliargs.Strict(fs, []string{"--full", "--gate", "test"}, "demo", usoDemo); err != nil {
		t.Fatal(err)
	}
	if fs.Lookup("full").Value.String() != "true" || fs.Lookup("gate").Value.String() != "test" {
		t.Fatalf("CA-225: Strict debe dejar el FlagSet parseado: full=%q gate=%q",
			fs.Lookup("full").Value.String(), fs.Lookup("gate").Value.String())
	}
	if fs.NArg() != 0 {
		t.Fatalf("CA-225: sin posicionales, NArg debe ser 0, es %d (%v)", fs.NArg(), fs.Args())
	}
}
