// Helpers compartidos por los tests de este paquete. Son COPIA VERBATIM de los
// que acompañan a los tests del mecanismo en internal/cliargs/cliargs_test.go:
// duplicarlos es el precio de que cada paquete pruebe lo suyo (`go test
// ./internal/cliargs` corre `cliargs`, `go test ./internal/verifycmd` corre el
// verbo) sin que uno le preste su archivo de tests al otro.
package verifycmd

import (
	"errors"
	"math/rand"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/cliargs"
)

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
