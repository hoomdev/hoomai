// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// `hoom pedir`: la linea valores: sale una sola vez (CA-806), con el nivel y
// el origen del riesgo de la tarjeta (CA-804), y el comando largo y el
// provider reciben el modelo traducido, nunca el nivel (CA-805). Escritos
// desde el spec, las aclaraciones de la ronda 0 y las firmas publicas, sin
// leer la implementacion.
package tarjetacmd_test

import (
	"strings"
	"testing"
)

const pprAgentsWriter = "agents:\n  writer:\n    model: medio\n    effort: high\n    timeout: 45m\n    alto: { model: fuerte }\n"

// pprPedir corre `hoom pedir writer precios` con la tarjeta de itemExtra y
// devuelve la salida y los argumentos del claude falso.
func pprPedir(t *testing.T, ca, itemExtra string, extra ...string) (vdtSalida, []string) {
	t.Helper()
	f := vdtPATH(t)
	f.cli(t, "claude", vdtNarra)
	root := vdtProyecto(t, pprAgentsWriter)
	vdtWriter(t, ca, root, "precios", "presupuesto_usd: 5\n"+itemExtra)
	args := append([]string{"writer", "precios"}, extra...)
	args = append(args, "-m", "Implementa el spec")
	s := vdtCorrer(t, ca, vdtOpts{dir: root}, "pedir", args...)
	return s, vdtUnaLlamada(t, ca, "pedir writer", f, "claude")
}

// pprUnaValores exige una sola linea valores: (ni el sobre la repite) y la
// devuelve.
func pprUnaValores(t *testing.T, ca string, s vdtSalida) string {
	t.Helper()
	n := strings.Count(s.out+s.err, "valores: ")
	if n != 1 {
		t.Fatalf("%s: pedir imprime la linea valores: una sola vez, fueron %d:\n%s", ca, n, s)
	}
	return vdtValoresDe(t, ca, s.out)
}

// CA-806: hoom pedir imprime valores: una sola vez; con la tarjeta alto el
// modelo es el del nivel fuerte con su origen, y el comando largo y el
// provider reciben opus (CA-805, CA-804).
func TestCA806_PedirImprimeValoresUnaVezConElNivel(t *testing.T) {
	const ca = "CA-806"
	s, args := pprPedir(t, ca, "riesgo: alto\n")
	v := pprUnaValores(t, ca, s)
	want := "valores: provider claude (tarjeta) · modelo opus (nivel fuerte, hoom.yaml, riesgo alto) · esfuerzo high (hoom.yaml) · presupuesto 5 (tarjeta) · tiempo limite 45m (hoom.yaml)"
	if v != want {
		t.Fatalf("%s: la linea de valores con el nivel del riesgo:\n quiero %q\n fue    %q", ca, want, v)
	}
	fl := vdtFlechaDe(t, ca, s.out)
	if !strings.Contains(fl, " --model opus ") || strings.Contains(fl, "fuerte") {
		t.Fatalf("CA-805: el comando largo lleva el modelo traducido: %q", fl)
	}
	if m, _ := vdtSigue(args, "--model"); m != "opus" {
		t.Fatalf("CA-805: el provider recibe opus: %q", args)
	}
}

// CA-805: en una tarjeta normal, medio llega como sonnet a la linea, al
// comando largo y al provider.
func TestCA805_PedirTarjetaNormalMedioEsSonnet(t *testing.T) {
	const ca = "CA-805"
	s, args := pprPedir(t, ca, "")
	v := pprUnaValores(t, ca, s)
	if !strings.Contains(v, " · modelo sonnet (nivel medio, hoom.yaml) · ") {
		t.Fatalf("%s: modelo sonnet (nivel medio, hoom.yaml): %q", ca, v)
	}
	if fl := vdtFlechaDe(t, ca, s.out); !strings.Contains(fl, " --model sonnet ") || strings.Contains(fl, "medio") {
		t.Fatalf("%s: el comando largo lleva sonnet: %q", ca, fl)
	}
	if m, _ := vdtSigue(args, "--model"); m != "sonnet" {
		t.Fatalf("%s: el provider recibe sonnet: %q", ca, args)
	}
}

// CA-805: --model barato del comando se traduce con su origen; --model Fuerte
// pasa como nombre, igual que hoy.
func TestCA805_PedirModelDelComando(t *testing.T) {
	const ca = "CA-805"
	s, args := pprPedir(t, ca, "riesgo: alto\n", "--model", "barato")
	v := pprUnaValores(t, ca, s)
	if !strings.Contains(v, " · modelo haiku (nivel barato, comando) · ") {
		t.Fatalf("%s: --model barato: %q", ca, v)
	}
	if fl := vdtFlechaDe(t, ca, s.out); !strings.Contains(fl, " --model haiku ") || strings.Contains(fl, "barato") {
		t.Fatalf("%s: el comando largo lleva haiku: %q", ca, fl)
	}
	if m, _ := vdtSigue(args, "--model"); m != "haiku" {
		t.Fatalf("%s: el provider recibe haiku: %q", ca, args)
	}
	s, args = pprPedir(t, ca, "", "--model", "Fuerte")
	v = pprUnaValores(t, ca, s)
	if !strings.Contains(v, " · modelo Fuerte (comando) · ") || strings.Contains(v, "nivel") {
		t.Fatalf("%s: Fuerte es un nombre: %q", ca, v)
	}
	if m, _ := vdtSigue(args, "--model"); m != "Fuerte" {
		t.Fatalf("%s: Fuerte llega tal cual: %q", ca, args)
	}
}
