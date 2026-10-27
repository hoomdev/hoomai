//go:build !windows

// Ronda 6 de gates-en-grupo, hallazgos de la delta 2:
//
//   - 20261027T183126_77dc9a (low): las rachas de blancos se acotaban a 4 KiB
//     aunque estuvieran en el medio de una salida que entraba entera en la
//     cola: con TailSize 16384, 'INICIO' + 5000 espacios + 'FIN' perdia 904
//     espacios y llevaba la marca de truncado.
//   - 20261027T183416_5fc8ff (low): solo se acotaban los blancos ASCII, pero
//     al final se recortaba todo blanco Unicode: un diagnostico seguido de
//     mas de TailSize+64 KiB de U+00A0 o U+2028 dejaba la cola en la marca de
//     truncado sola.
//
// Escritos desde el contrato (go doc ./internal/gates: Options.TailSize, los
// bytes de cola que guarda el veredicto) y los hallazgos, sin leer la
// implementacion. Usan gegLanzarConPlazo (cola de 16 KiB), gegCorrerUnGate,
// gegBlancos y gegColaCorta de las rondas 4 y 5.
package gates

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode"
)

// gegCola es la cola de gegLanzarConPlazo.
const gegCola = 16384

// Hallazgo 77dc9a: con TailSize 16384, una salida que entra entera en la cola
// vuelve COMPLETA, byte a byte, y sin la marca de truncado, aunque tenga una
// racha larga de blancos en el medio (espacios, saltos de linea, tabs o una
// mezcla; de 5000 y de 15000 bytes).
func TestHallazgo77dc9a_SalidaQueEntraEnLaColaVuelveCompletaConSusBlancos(t *testing.T) {
	for _, nombre := range []string{"espacios", "lineas vacias", "tabs", "mezcla"} {
		for _, tam := range []int{5000, 15000} {
			t.Run(fmt.Sprintf("%s %d", nombre, tam), func(t *testing.T) {
				medio, cmd := gegRacha(nombre, tam)
				quiere := "INICIO" + medio + "FIN"
				r := gegCorrerUnGate(t, "printf INICIO\n"+cmd+"printf FIN\n")
				if strings.Contains(r.OutputTail, gegMarcaTruncado) {
					t.Fatalf("hallazgo 77dc9a: una salida de %d bytes entra en la cola de %d: no lleva la marca de truncado: %s", len(quiere), gegCola, gegColaCorta(r.OutputTail))
				}
				if r.OutputTail != quiere {
					t.Fatalf("hallazgo 77dc9a: una salida de %d bytes que entra en la cola vuelve completa, byte a byte; volvieron %d bytes: %s", len(quiere), len(r.OutputTail), gegColaCorta(r.OutputTail))
				}
			})
		}
	}
}

// gegRacha devuelve una racha de tam blancos del juego nombre y la linea de
// sh que la imprime.
func gegRacha(nombre string, tam int) (string, string) {
	juegos := map[string]struct{ c, esc string }{
		"espacios":      {" ", " "},
		"lineas vacias": {"\n", `\n`},
		"tabs":          {"\t", `\t`},
	}
	if j, ok := juegos[nombre]; ok {
		return strings.Repeat(j.c, tam), gegCeros(tam, j.esc)
	}
	// mezcla: espacios, tabs y saltos de linea
	a, b := tam/3, tam/3
	c := tam - a - b
	return strings.Repeat(" ", a) + strings.Repeat("\t", b) + strings.Repeat("\n", c),
		gegCeros(a, " ") + gegCeros(b, `\t`) + gegCeros(c, `\n`)
}

// gegCeros es la linea de sh que imprime n veces el caracter a (escapado
// para tr).
func gegCeros(n int, a string) string {
	return fmt.Sprintf("head -c %d /dev/zero | tr '\\0' '%s'\n", n, a)
}

// Hallazgo 77dc9a: la marca de truncado va solo si se perdio salida. Un
// diagnostico corto seguido de 1 MiB de blancos al final vuelve como el
// diagnostico solo, sin la marca (los blancos del final se recortan igual,
// eso no es perder salida). Un gate con mas de TailSize de salida de verdad
// si la lleva, y su cola termina en lo ultimo que imprimio.
func TestHallazgo77dc9a_MarcaDeTruncadoSoloSiSePerdioSalida(t *testing.T) {
	for _, nombre := range []string{"lineas vacias", "espacios", "mezcla"} {
		t.Run("diagnostico y 1 MiB de "+nombre, func(t *testing.T) {
			r := gegCorrerUnGate(t, "echo '"+gegDiagnostico+"' >&2\n"+gegBlancos(nombre, 1<<20)+"exit 1\n")
			if r.OutputTail != gegDiagnostico {
				t.Fatalf("hallazgo 77dc9a: un diagnostico corto seguido de 1 MiB de blancos al final vuelve como el diagnostico solo, sin la marca de truncado: %s", gegColaCorta(r.OutputTail))
			}
		})
	}
	t.Run("mas salida que la cola", func(t *testing.T) {
		r := gegCorrerUnGate(t, "i=1; while [ $i -le 3000 ]; do echo linea-$i; i=$((i+1)); done\necho FIN-DE-LA-SALIDA\n")
		if !strings.HasPrefix(r.OutputTail, gegMarcaTruncado) {
			t.Fatalf("hallazgo 77dc9a: un gate con mas de %d bytes de salida de verdad lleva la marca de truncado: %s", gegCola, gegColaCorta(r.OutputTail))
		}
		if !strings.HasSuffix(r.OutputTail, "linea-3000\nFIN-DE-LA-SALIDA") || len(r.OutputTail) > gegCola+len(gegMarcaTruncado)+1 {
			t.Fatalf("hallazgo 77dc9a: la cola truncada es el final de la salida, de a lo sumo %d bytes mas la marca (%d): %s", gegCola, len(r.OutputTail), gegColaCorta(r.OutputTail))
		}
	})
}

// Hallazgo 5fc8ff: un gate que imprime un diagnostico y despues mas de
// TailSize+64 KiB de un espacio Unicode (U+00A0, U+2028, U+3000) no deja una
// cola que sea solo la marca de truncado: lo que sigue a la marca es el
// final real de la salida (esos espacios Unicode, o el diagnostico si se los
// recorta como blancos).
func TestHallazgo5fc8ff_EspaciosUnicodeAlFinalNoDejanSoloLaMarca(t *testing.T) {
	// escapes explicitos: en el codigo los tres se ven en blanco
	casos := []struct {
		codigo string
		r      rune
	}{
		{"U+00A0", '\u00a0'}, // NO-BREAK SPACE
		{"U+2028", '\u2028'}, // LINE SEPARATOR
		{"U+3000", '\u3000'}, // IDEOGRAPHIC SPACE
	}
	for _, caso := range casos {
		r := caso.r
		for _, tam := range []int{200 << 10, 1 << 20} {
			t.Run(fmt.Sprintf("%s %d KiB", caso.codigo, tam>>10), func(t *testing.T) {
				if fmt.Sprintf("U+%04X", r) != caso.codigo || !unicode.IsSpace(r) {
					t.Fatalf("fixture: %s es U+%04X y un espacio para strings.TrimSpace", caso.codigo, r)
				}
				d := t.TempDir()
				espacios := strings.Repeat(string(r), tam/len(string(r)))
				real := gegDiagnostico + "\n" + espacios
				archivo := filepath.Join(d, "salida")
				if err := os.WriteFile(archivo, []byte(real), 0o644); err != nil {
					t.Fatal(err)
				}
				res := gegCorrerUnGate(t, "cat '"+archivo+"' >&2\nexit 1\n")
				cuerpo, _ := strings.CutPrefix(res.OutputTail, gegMarcaTruncado)
				cuerpo, _ = strings.CutPrefix(cuerpo, "\n")
				if cuerpo == "" {
					t.Fatalf("hallazgo 5fc8ff: un diagnostico seguido de %d bytes de U+%04X no deja una cola que sea solo la marca de truncado: %s", len(espacios), r, gegColaCorta(res.OutputTail))
				}
				// el final real: la cola entera de la salida (los espacios
				// Unicode), o de la salida sin ellos (el diagnostico). Hasta
				// 3 bytes menos, por un caracter cortado en el borde
				recortada := strings.TrimRightFunc(real, unicode.IsSpace)
				final := ""
				for _, s := range []string{real, recortada} {
					if strings.HasSuffix(s, cuerpo) && len(cuerpo) >= min(len(s), gegCola)-3 {
						final = s
					}
				}
				if final == "" {
					t.Fatalf("hallazgo 5fc8ff: lo que sigue a la marca es el final real de la salida, hasta la cola (%d bytes): los U+%04X, o el diagnostico si se recortan; siguen %d bytes: %s", gegCola, r, len(cuerpo), gegColaCorta(res.OutputTail))
				}
				if len(res.OutputTail) > gegCola+len(gegMarcaTruncado)+1 {
					t.Fatalf("hallazgo 5fc8ff: la cola sigue acotada a %d bytes mas la marca: tiene %d", gegCola, len(res.OutputTail))
				}
			})
		}
	}
}
