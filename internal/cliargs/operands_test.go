// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-268): `cliargs.Operands`, la disciplina de argumentos de los verbos
// que SI toman operandos (`item add "<titulo>"`, `item show <slug>`). Se
// prueba con un verbo de mentira, como consumidor externo: el mecanismo no
// conoce `item`.
package cliargs_test

import (
	"bytes"
	"errors"
	"flag"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/cliargs"
)

const opUso = `Uso: hoom demo add "<titulo>" [--tipo t] [--json]

  --tipo t   el tipo de la cosa
  --json     emite JSON

Un titulo que empieza con '-' va despues de '--'.`

func opFS() (*flag.FlagSet, *string, *bool) {
	fs := flag.NewFlagSet("demo add", flag.ContinueOnError)
	tipo := fs.String("tipo", "", "")
	js := fs.Bool("json", false, "")
	return fs, tipo, js
}

// opRechazo exige el tipo del contrato, el exit 2 invariante, y que el
// mensaje lleve el verbo y el bloque de uso exacto.
func opRechazo(t *testing.T, caso string, err error) *cliargs.UsageError {
	t.Helper()
	if err == nil {
		t.Fatalf("CA-268 %s: esperaba un *cliargs.UsageError, no nil", caso)
	}
	if errors.Is(err, cliargs.ErrHelp) {
		t.Fatalf("CA-268 %s: esperaba un rechazo, no ErrHelp", caso)
	}
	var ue *cliargs.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("CA-268 %s: esperaba *cliargs.UsageError, fue %T: %v", caso, err, err)
	}
	if ue.ExitCode() != 2 {
		t.Fatalf("CA-268 %s: el exit de un UsageError es 2, fue %d", caso, ue.ExitCode())
	}
	if ue.Verb != "demo add" {
		t.Fatalf("CA-268 %s: el error lleva el verbo recibido, fue %q", caso, ue.Verb)
	}
	if ue.Usage != opUso {
		t.Fatalf("CA-268 %s: el bloque de uso viaja exacto, fue:\n%s", caso, ue.Usage)
	}
	if strings.TrimSpace(ue.Reason) == "" {
		t.Fatalf("CA-268 %s: el rechazo dice que no se entendio (Reason vacio)", caso)
	}
	return ue
}

// CA-268: flags antes y despues del operando, y `--` que habilita operandos
// que empiezan con '-'.
func TestCA268_OperandsAceptaFlagsAntesYDespues(t *testing.T) {
	casos := []struct {
		nombre string
		args   []string
		n      int
		ops    []string
		tipo   string
		json   bool
	}{
		{"solo el operando", []string{"Precios"}, 1, []string{"Precios"}, "", false},
		{"flag antes", []string{"--tipo", "bug", "Precios"}, 1, []string{"Precios"}, "bug", false},
		{"flag despues", []string{"Precios", "--tipo", "bug"}, 1, []string{"Precios"}, "bug", false},
		{"flag con = despues", []string{"Precios", "--tipo=bug"}, 1, []string{"Precios"}, "bug", false},
		{"bool despues", []string{"Precios", "--json"}, 1, []string{"Precios"}, "", true},
		{"bool antes y flag despues", []string{"--json", "Precios", "--tipo", "docs"}, 1, []string{"Precios"}, "docs", true},
		{"operando con espacios", []string{"Precios por region (v2)!"}, 1, []string{"Precios por region (v2)!"}, "", false},
		{"-- habilita un guion", []string{"--", "-x"}, 1, []string{"-x"}, "", false},
		{"flags y luego --", []string{"--tipo", "bug", "--", "-titulo raro"}, 1, []string{"-titulo raro"}, "bug", false},
		{"despues de -- un flag es operando", []string{"--", "--tipo"}, 1, []string{"--tipo"}, "", false},
		{"dos operandos intercalados", []string{"a", "--json", "b"}, 2, []string{"a", "b"}, "", true},
		{"cero operandos", []string{"--json"}, 0, nil, "", true},
		{"cero operandos sin nada", nil, 0, nil, "", false},
	}
	for _, c := range casos {
		fs, tipo, js := opFS()
		ops, err := cliargs.Operands(fs, c.args, "demo add", opUso, c.n)
		if err != nil {
			t.Fatalf("CA-268 %s: %v no debe rechazarse: %v", c.nombre, c.args, err)
		}
		if len(ops) != len(c.ops) {
			t.Fatalf("CA-268 %s: operandos esperados %q, fueron %q", c.nombre, c.ops, ops)
		}
		for i := range c.ops {
			if ops[i] != c.ops[i] {
				t.Fatalf("CA-268 %s: operandos esperados %q, fueron %q", c.nombre, c.ops, ops)
			}
		}
		if *tipo != c.tipo || *js != c.json {
			t.Fatalf("CA-268 %s: los flags se parsean igual antes o despues del operando: tipo=%q json=%v (esperaba %q %v)",
				c.nombre, *tipo, *js, c.tipo, c.json)
		}
	}
}

// CA-268: todo lo que el verbo no entiende es *UsageError (exit 2).
func TestCA268_OperandsRechazaLoQueNoEntiende(t *testing.T) {
	casos := []struct {
		nombre string
		args   []string
		n      int
	}{
		{"operando de menos", nil, 1},
		{"operando de menos con flags", []string{"--tipo", "bug"}, 1},
		{"operando de mas", []string{"A", "B"}, 1},
		{"operando de mas tras --", []string{"A", "--", "B"}, 1},
		{"operando de mas con n=0", []string{"x"}, 0},
		{"operando vacio", []string{""}, 1},
		{"operando vacio tras --", []string{"--", ""}, 1},
		{"flag desconocido antes", []string{"--bogus", "A"}, 1},
		{"flag desconocido despues", []string{"A", "--bogus"}, 1},
		{"un guion suelto sin -- es un flag desconocido", []string{"-x"}, 1},
		{"flag sin valor al final", []string{"A", "--tipo"}, 1},
		{"flag con valor vacio", []string{"A", "--tipo", ""}, 1},
		{"flag con = vacio", []string{"A", "--tipo="}, 1},
		{"flag vacio antes del operando", []string{"--tipo", "", "A"}, 1},
		{"flag repetido con el primero vacio", []string{"--tipo", "", "--tipo", "bug", "A"}, 1},
	}
	for _, c := range casos {
		fs, _, _ := opFS()
		ops, err := cliargs.Operands(fs, c.args, "demo add", opUso, c.n)
		opRechazo(t, c.nombre, err)
		if len(ops) != 0 {
			t.Fatalf("CA-268 %s: un rechazo no devuelve operandos a medias: %q", c.nombre, ops)
		}
	}
}

// CA-268: el mensaje del rechazo es de hoom: nombra el flag desconocido y
// lleva cada linea del bloque de uso; el paquete flag no escribe nada.
func TestCA268_OperandsMensajePropio(t *testing.T) {
	fs, _, _ := opFS()
	var ajeno bytes.Buffer
	fs.SetOutput(&ajeno)
	_, err := cliargs.Operands(fs, []string{"Precios", "--bogus"}, "demo add", opUso, 1)
	ue := opRechazo(t, "flag desconocido", err)
	if !strings.Contains(ue.Error(), "--bogus") {
		t.Fatalf("CA-268: el rechazo nombra el flag desconocido:\n%s", ue.Error())
	}
	for _, l := range strings.Split(opUso, "\n") {
		if l = strings.TrimRight(l, " "); strings.TrimSpace(l) != "" && !strings.Contains(ue.Error(), l) {
			t.Fatalf("CA-268: al mensaje le falta la linea del uso %q:\n%s", l, ue.Error())
		}
	}
	if ajeno.Len() != 0 {
		t.Fatalf("CA-268: el paquete flag no imprime su propio uso: %q", ajeno.String())
	}

	fs, _, _ = opFS()
	_, err = cliargs.Operands(fs, []string{"Precios", "--tipo", ""}, "demo add", opUso, 1)
	ue = opRechazo(t, "flag vacio", err)
	if !strings.Contains(ue.Reason, "tipo") {
		t.Fatalf("CA-268: el rechazo del valor vacio nombra el flag: %q", ue.Reason)
	}
}

// CA-268: -h/--help es ErrHelp (el uso a stdout con exit 0), no un error de
// uso; un pedido roto no sale 0 por haber pedido ayuda.
func TestCA268_OperandsHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"Precios", "-h"}, {"--tipo", "bug", "--help", "Precios"}} {
		fs, _, _ := opFS()
		_, err := cliargs.Operands(fs, args, "demo add", opUso, 1)
		if !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-268: %v debe devolver ErrHelp, fue %v", args, err)
		}
	}
	fs, _, _ := opFS()
	_, err := cliargs.Operands(fs, []string{"--help", "--bogus"}, "demo add", opUso, 1)
	opRechazo(t, "help con flag desconocido", err)
}

// CA-268: cliargs.Strict no se relajo para dar lugar a los operandos.
func TestCA268_StrictNoCambia(t *testing.T) {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	fs.Bool("json", false, "")
	for _, args := range [][]string{{"x"}, {"--json", "x"}, {"--", "x"}} {
		err := cliargs.Strict(fs, args, "demo", opUso)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("CA-268: Strict sigue rechazando todo posicional %v, fue %v", args, err)
		}
	}
}

// opOperando genera operandos feos pero imprimibles que no empiezan con '-'
// ni son vacios.
type opOperando string

func (opOperando) Generate(rnd *rand.Rand, size int) reflect.Value {
	alfabeto := []rune("abcXYZ019._/=:@ñéÑÜÇ日本語Ωμ🚀 -")
	n := 1 + rnd.Intn(16)
	out := make([]rune, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, alfabeto[rnd.Intn(len(alfabeto))])
	}
	s := string(out)
	if strings.HasPrefix(s, "-") {
		s = "x" + s
	}
	return reflect.ValueOf(opOperando(s))
}

// CA-268 (propiedad): cualquier operando que no parece flag vuelve tal cual,
// con el flag antes o despues; y cualquier operando, detras de `--`.
func TestCA268_PropiedadElOperandoVuelveTalCual(t *testing.T) {
	prop := func(op opOperando, antes bool) bool {
		fs, tipo, _ := opFS()
		args := []string{string(op), "--tipo", "bug"}
		if antes {
			args = []string{"--tipo", "bug", string(op)}
		}
		ops, err := cliargs.Operands(fs, args, "demo add", opUso, 1)
		if err != nil || len(ops) != 1 || ops[0] != string(op) || *tipo != "bug" {
			return false
		}
		fs, _, _ = opFS()
		ops, err = cliargs.Operands(fs, []string{"--", "-" + string(op)}, "demo add", opUso, 1)
		return err == nil && len(ops) == 1 && ops[0] == "-"+string(op)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("CA-268: %v", err)
	}
}
