//go:build !windows

// Ronda 5 de gates-en-grupo, hallazgos 20261027T175322_c61685 (medium) y
// 20261027T175556_4a9763 (low): la salida del gate se acotaba a la cola mas
// 64 KiB antes de sacarle los blancos del final. Un gate que imprimia un
// diagnostico y despues mas de 64 KiB de lineas vacias o espacios quedaba en
// el veredicto solo con la marca de truncado: perdia justo la evidencia de
// por que fallo.
//
// Escritos desde el contrato (go doc ./internal/gates: Run, Options.TailSize;
// GateResult.OutputTail) y los hallazgos, sin leer la implementacion. Usan
// gegLanzarConPlazo (gates_en_grupo_ayudantes_test.go, cola de 16 KiB).
package gates

import (
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// gegDiagnostico es lo que el gate imprime antes de los blancos: la unica
// evidencia de por que fallo.
const gegDiagnostico = "DIAGNOSTICO-c61685: FAIL internal/x/x_test.go:12: esperaba 3, vino 4"

// gegColaCorta resume una cola larga para los mensajes.
func gegColaCorta(s string) string {
	if len(s) <= 300 {
		return strconv.Quote(s)
	}
	return strconv.Quote(s[:150]) + " ... " + strconv.Quote(s[len(s)-150:])
}

// gegCorrerUnGate corre el gate test con cmd y un plazo de un minuto, y
// devuelve su resultado.
func gegCorrerUnGate(t *testing.T, cmd string) verdict.GateResult {
	t.Helper()
	c := gegLanzarConPlazo(t, map[string]string{"test": cmd}, time.Minute)
	if !c.volvio(gegTope) {
		t.Fatalf("fixture: un gate que termina enseguida vuelve en a lo sumo %v", gegTope)
	}
	return c.gate(t, "test")
}

// gegBlancos es la linea de sh que imprime tam bytes de blancos del juego
// nombre: lineas vacias, espacios, o una mezcla de espacios, tabs y saltos
// de linea.
func gegBlancos(nombre string, tam int) string {
	ceros := func(n int, a string) string {
		return fmt.Sprintf("head -c %d /dev/zero | tr '\\0' '%s'\n", n, a)
	}
	switch nombre {
	case "lineas vacias":
		return ceros(tam, `\n`)
	case "espacios":
		return ceros(tam, " ")
	default:
		return ceros(tam/3, " ") + ceros(tam/3, `\t`) + ceros(tam-2*(tam/3), `\n`)
	}
}

// Hallazgos c61685 y 4a9763: un gate que imprime lineas, un diagnostico y
// DESPUES mas de 64 KiB de blancos (100 KiB o 1 MiB de lineas vacias,
// espacios o una mezcla con tabs), y sale con exit 1, conserva el
// diagnostico en la cola del veredicto: la cola termina en lo ultimo que
// imprimio sin contar los blancos del final, no es la marca de truncado sola.
func TestHallazgoC61685_DiagnosticoSeguidoDeBlancosQuedaEnLaCola(t *testing.T) {
	for _, nombre := range []string{"lineas vacias", "espacios", "mezcla"} {
		for _, tam := range []int{100 << 10, 1 << 20} {
			t.Run(fmt.Sprintf("%s %d KiB", nombre, tam>>10), func(t *testing.T) {
				// 3000 lineas antes (mas que la cola), el diagnostico (por
				// stderr, como lo deja go test o un linter) y los blancos
				r := gegCorrerUnGate(t, "i=1; while [ $i -le 3000 ]; do echo linea-$i; i=$((i+1)); done\n"+
					"echo '"+gegDiagnostico+"' >&2\n"+
					gegBlancos(nombre, tam)+
					"exit 1\n")
				if r.Status != verdict.StatusFail || r.ExitCode != 1 {
					t.Fatalf("hallazgos c61685, 4a9763: el gate sale con exit 1: falla con exit 1: %+v", r)
				}
				if !strings.Contains(r.OutputTail, gegDiagnostico) {
					t.Fatalf("hallazgos c61685, 4a9763: el diagnostico que el gate imprimio antes de %d bytes de blancos (%s) queda en su cola; la cola es %s", tam, nombre, gegColaCorta(r.OutputTail))
				}
				if !strings.HasSuffix(strings.TrimSpace(r.OutputTail), gegDiagnostico) {
					t.Fatalf("hallazgos c61685, 4a9763: la cola termina en el diagnostico, lo ultimo que imprimio el gate sin contar los blancos: %s", gegColaCorta(r.OutputTail))
				}
				if len(r.OutputTail) > 16384+len(gegMarcaTruncado)+1 {
					t.Fatalf("hallazgos c61685, 4a9763: la cola sigue acotada a la cola configurada (16 KiB) mas la marca: tiene %d bytes", len(r.OutputTail))
				}
			})
		}
	}
}

// Guarda de c61685: un bloque de 2 MiB de blancos en el medio, entre dos
// marcas, no se come la segunda: la cola termina en ella.
func TestGatesEnGrupo5_C61685_BlancosEnElMedioConservanLaSegundaMarca(t *testing.T) {
	r := gegCorrerUnGate(t, "echo MARCA-ANTES-c61685\n"+
		gegBlancos("lineas vacias", 1<<20)+
		gegBlancos("espacios", 1<<20)+
		"echo\necho MARCA-DESPUES-c61685\n"+
		"exit 1\n")
	if r.Status != verdict.StatusFail || r.ExitCode != 1 {
		t.Fatalf("hallazgos c61685, 4a9763: el gate sale con exit 1: falla con exit 1: %+v", r)
	}
	if !strings.HasSuffix(strings.TrimSpace(r.OutputTail), "MARCA-DESPUES-c61685") {
		t.Fatalf("hallazgos c61685, 4a9763: con blancos en el medio, la cola termina en la segunda marca: %s", gegColaCorta(r.OutputTail))
	}
}

// Guarda de c61685: un gate normal da su salida completa como antes: sin los
// blancos de los extremos, stdout y stderr en orden, entera aunque tenga una
// linea vacia adentro.
func TestGatesEnGrupo5_C61685_GateNormalDaSuSalidaCompleta(t *testing.T) {
	r := gegCorrerUnGate(t, "echo\necho '   '\necho uno\necho dos >&2\necho\necho tres\necho\necho '  '\nexit 0\n")
	if r.Status != verdict.StatusPass || r.ExitCode != 0 {
		t.Fatalf("hallazgos c61685, 4a9763: el gate con exit 0 pasa: %+v", r)
	}
	if r.OutputTail != "uno\ndos\n\ntres" {
		t.Fatalf("hallazgos c61685, 4a9763: un gate normal da su salida completa, sin los blancos de los extremos: %q", r.OutputTail)
	}
}
