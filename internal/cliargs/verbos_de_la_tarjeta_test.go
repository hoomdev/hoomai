// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md
// (CA-512): `cliargs.OperandsRango`, la disciplina de los verbos de la
// tarjeta que toman entre min y max operandos (`hoom pedir <rol> [<slug>]`).
// Un flag que no existe, un flag sin valor o un operando de mas es exit 2 con
// el bloque de uso; -h es ErrHelp; los flags de vacios pueden ir vacios
// (`-m ""` es un pedido vacio, no un typo). Se prueba con un verbo de
// mentira, como consumidor externo.
package cliargs_test

import (
	"errors"
	"flag"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/cliargs"
)

const vdtUso = `Uso: hoom demo <rol> [<slug>] [-m "<pedido>"] [--model m] [--json]

  -m <pedido>   lo que el rol tiene que hacer
  --model m     modelo`

func vdtFlags() (*flag.FlagSet, *string, *string, *bool) {
	fs := flag.NewFlagSet("demo", flag.ContinueOnError)
	m := fs.String("m", "", "pedido")
	model := fs.String("model", "", "modelo")
	js := fs.Bool("json", false, "json")
	return fs, m, model, js
}

// vdtUsoErr exige un *UsageError de exit 2 con el bloque de uso exacto.
func vdtUsoErr(t *testing.T, ca string, args []string, err error, razon string) {
	t.Helper()
	var ue *cliargs.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("%s: %q es un error de uso (exit 2), fue %v", ca, args, err)
	}
	if ue.ExitCode() != 2 || ue.Usage != vdtUso || ue.Verb != "demo" {
		t.Fatalf("%s: %q: exit 2 con el uso del verbo: %+v", ca, args, ue)
	}
	if razon != "" && !strings.Contains(ue.Reason, razon) {
		t.Fatalf("%s: %q: la razon dice %q, fue %q", ca, args, razon, ue.Reason)
	}
}

// CA-512: entre min y max operandos, con los flags antes, entre o despues.
func TestCA512_OperandsRangoAceptaElRango(t *testing.T) {
	casos := []struct {
		args []string
		ops  []string
		m    string
	}{
		{[]string{"writer"}, []string{"writer"}, ""},
		{[]string{"writer", "precios"}, []string{"writer", "precios"}, ""},
		{[]string{"-m", "Implementa", "writer"}, []string{"writer"}, "Implementa"},
		{[]string{"writer", "-m", "Implementa", "precios"}, []string{"writer", "precios"}, "Implementa"},
		{[]string{"writer", "precios", "--model", "opus", "-m", "x y"}, []string{"writer", "precios"}, "x y"},
		{[]string{"writer", "--", "precios"}, []string{"writer", "precios"}, ""},
	}
	for _, c := range casos {
		fs, m, _, _ := vdtFlags()
		ops, err := cliargs.OperandsRango(fs, c.args, "demo", vdtUso, 1, 2, "m")
		if err != nil {
			t.Fatalf("CA-512: %q es un pedido valido: %v", c.args, err)
		}
		if !reflect.DeepEqual(ops, c.ops) || *m != c.m {
			t.Fatalf("CA-512: %q da operandos %q y -m %q, fue %q y %q", c.args, c.ops, c.m, ops, *m)
		}
	}
}

// CA-512: un flag desconocido, un flag sin valor, un operando de mas o de
// menos es un error de uso con el bloque del verbo.
func TestCA512_OperandsRangoRechazaConUso(t *testing.T) {
	casos := []struct {
		args  []string
		razon string
	}{
		{[]string{"writer", "--bogus"}, "flag desconocido: --bogus"},
		{[]string{"--bogus", "writer"}, "flag desconocido: --bogus"},
		{[]string{"writer", "--model"}, "--model necesita un valor"},
		{[]string{"writer", "precios", "otra"}, `"otra"`},
		{[]string{}, ""},
		{[]string{"writer", "--model", ""}, "--model"},
	}
	for _, c := range casos {
		fs, _, _, _ := vdtFlags()
		_, err := cliargs.OperandsRango(fs, c.args, "demo", vdtUso, 1, 2, "m")
		vdtUsoErr(t, "CA-512", c.args, err, c.razon)
	}
}

// CA-502 y CA-512: el flag nombrado en vacios puede ir vacio (-m "" es un
// pedido vacio, que juzga el verbo); otro flag vacio sigue siendo un typo.
func TestCA512_OperandsRangoVaciosPermitidos(t *testing.T) {
	for _, args := range [][]string{{"writer", "-m", ""}, {"writer", "-m="}, {"-m", "", "writer", "precios"}} {
		fs, m, _, _ := vdtFlags()
		ops, err := cliargs.OperandsRango(fs, args, "demo", vdtUso, 1, 2, "m")
		if err != nil {
			t.Fatalf("CA-502: %q: -m vacio no es un error de uso (es un pedido vacio): %v", args, err)
		}
		if *m != "" || len(ops) == 0 || ops[0] != "writer" {
			t.Fatalf("CA-502: %q: -m queda vacio y el rol se lee: %q %q", args, *m, ops)
		}
	}
	// sin vacios, -m "" vuelve a ser un typo
	fs, _, _, _ := vdtFlags()
	_, err := cliargs.OperandsRango(fs, []string{"writer", "-m", ""}, "demo", vdtUso, 1, 2)
	vdtUsoErr(t, "CA-512", []string{"writer", "-m", ""}, err, "-m")
}

// CA-512: -h y --help son ErrHelp (el uso, exit 0), tambien junto a operandos.
func TestCA512_OperandsRangoAyuda(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"writer", "-h"}} {
		fs, _, _, _ := vdtFlags()
		if _, err := cliargs.OperandsRango(fs, args, "demo", vdtUso, 0, 2, "m"); !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-512: %q es pedir el uso (ErrHelp), fue %v", args, err)
		}
	}
}

// CA-512: un rango 0..1 (hoom guardar [<slug>]) acepta cero y uno.
func TestCA512_OperandsRangoCeroAUno(t *testing.T) {
	for _, args := range [][]string{{}, {"precios"}} {
		fs, _, _, _ := vdtFlags()
		ops, err := cliargs.OperandsRango(fs, args, "demo", vdtUso, 0, 1)
		if err != nil || len(ops) != len(args) {
			t.Fatalf("CA-512: %q en un rango 0..1: %q %v", args, ops, err)
		}
	}
	fs, _, _, _ := vdtFlags()
	_, err := cliargs.OperandsRango(fs, []string{"a", "b"}, "demo", vdtUso, 0, 1)
	vdtUsoErr(t, "CA-512", []string{"a", "b"}, err, `"b"`)
}
