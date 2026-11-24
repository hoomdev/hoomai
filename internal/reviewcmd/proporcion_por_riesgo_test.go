package reviewcmd

import (
	"strings"
	"testing"
)

// pprModelo es la linea "  modelo      ..." de la cabecera de la review.
func pprModelo(t *testing.T, ca, out string) string {
	t.Helper()
	for _, l := range raCabecera(t, out) {
		if strings.HasPrefix(l, "  modelo      ") {
			return l
		}
	}
	t.Fatalf("%s: la cabecera trae su linea de modelo:\n%s", ca, out)
	return ""
}

// CA-805: review.model: medio llega a codex como gpt-6.1-sol (un solo -m,
// el traducido, nunca el nombre del nivel) y la cabecera dice
// '  modelo      gpt-6.1-sol (nivel medio)'.
func TestCA805_ReviewModelMedioLlegaTraducido(t *testing.T) {
	const ca = "CA-805"
	bin := raPATH(t)
	root := raRepo(t, "review:\n  model: medio\n")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")

	res, out := raRevisar(t, ca, root, Options{Provider: "codex", Lens: "risk"})
	if res.Status != "revisado" {
		t.Fatalf("%s: fixture: la review corre: %+v\n%s", ca, res, out)
	}
	if got, quiero := pprModelo(t, ca, out), "  modelo      gpt-6.1-sol (nivel medio)"; got != quiero {
		t.Fatalf("%s: la cabecera es %q, fue %q", ca, quiero, got)
	}
	if args := cx.argv(t, 1); !raSolo(raValores(args, "-m"), "gpt-6.1-sol") {
		t.Fatalf("%s: a codex le llega un solo -m gpt-6.1-sol, fue %v", ca, raValores(args, "-m"))
	}
	if strings.Contains(out, "modelo      medio") {
		t.Fatalf("%s: la pantalla trae el modelo traducido, no el nivel:\n%s", ca, out)
	}
}

// CA-805: el modelo de la opcion (--model) tambien se traduce, y gana al de
// review.model: fuerte llega a codex como gpt-6-astra.
func TestCA805_ReviewOpcionFuerteGanaYSeTraduce(t *testing.T) {
	const ca = "CA-805"
	bin := raPATH(t)
	root := raRepo(t, "review:\n  model: barato\n")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")

	_, out := raRevisar(t, ca, root, Options{Provider: "codex", Model: "fuerte", Lens: "risk"})
	if got, quiero := pprModelo(t, ca, out), "  modelo      gpt-6-astra (nivel fuerte)"; got != quiero {
		t.Fatalf("%s: la cabecera es %q, fue %q", ca, quiero, got)
	}
	if args := cx.argv(t, 1); !raSolo(raValores(args, "-m"), "gpt-6-astra") {
		t.Fatalf("%s: a codex le llega -m gpt-6-astra, fue %v", ca, raValores(args, "-m"))
	}
}

// CA-805: la tabla completa de codex desde review.model: barato es
// gpt-6-luna.
func TestCA805_ReviewBaratoEsLuna(t *testing.T) {
	const ca = "CA-805"
	bin := raPATH(t)
	root := raRepo(t, "review:\n  model: barato\n")
	raCambio(t, root)
	cx := raInstalar(t, bin, "codex", "")

	_, out := raRevisar(t, ca, root, Options{Provider: "codex", Lens: "risk"})
	if got, quiero := pprModelo(t, ca, out), "  modelo      gpt-6-luna (nivel barato)"; got != quiero {
		t.Fatalf("%s: la cabecera es %q, fue %q", ca, quiero, got)
	}
	if args := cx.argv(t, 1); !raSolo(raValores(args, "-m"), "gpt-6-luna") {
		t.Fatalf("%s: a codex le llega -m gpt-6-luna, fue %v", ca, raValores(args, "-m"))
	}
}

// CA-805 (borde hostil): un nombre que no es nivel pasa sin cambios, aunque
// se parezca a uno: "Fuerte" (mayuscula) y "medio-x" llegan tal cual y la
// cabecera no dice "(nivel".
func TestCA805_ReviewNombreQueNoEsNivelPasa(t *testing.T) {
	const ca = "CA-805"
	for _, nombre := range []string{"Fuerte", "medio-x", "gpt-5.6-sol"} {
		t.Run(nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			raCambio(t, root)
			cx := raInstalar(t, bin, "codex", "")

			_, out := raRevisar(t, ca, root, Options{Provider: "codex", Model: nombre, Lens: "risk"})
			if got, quiero := pprModelo(t, ca, out), "  modelo      "+nombre; got != quiero {
				t.Fatalf("%s: un nombre que no es nivel pasa sin cambios: la cabecera es %q, fue %q", ca, quiero, got)
			}
			if args := cx.argv(t, 1); !raSolo(raValores(args, "-m"), nombre) {
				t.Fatalf("%s: a codex le llega -m %s, fue %v", ca, nombre, raValores(args, "-m"))
			}
		})
	}
}
