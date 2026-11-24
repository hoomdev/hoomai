// Regresion de los hallazgos 4b4b61 y 7918f8 (ronda 1 de
// proporcion-por-riesgo): la review lanzada desde la tarjeta (la funcion de
// lanzamiento compartida, la del Studio, el piloto y los verbos) con
// review.model: fuerte conserva el nivel: la cabecera de la review dice
// `modelo <m> (nivel fuerte)` y su registro trae Nivel fuerte, con el modelo
// traducido para su provider.
package servecmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/modelos"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

func TestHallazgo_4b4b61_LaReviewDeLaTarjetaConservaElNivel(t *testing.T) {
	const h = "hallazgo 4b4b61"
	f := lnPATH(t)
	f.cli(t, "codex", false)
	root := raProyectoConReview(t, "review:\n  model: fuerte\n")
	wt := lnCartaReview(t, h, root, "con-nivel")
	s := lnServidor(t, root, f)
	modelo := modelos.Traducir("codex", modelos.Fuerte).Modelo
	if a, ok := lnAccion(lnCarta(t, root, "con-nivel"), "pedir-reviewer"); !ok || a.Nivel != modelos.Fuerte || a.Model != modelo {
		t.Fatalf("%s: fixture: pedir-reviewer propone %s desde el nivel fuerte: %+v", h, modelo, a)
	}

	r := lnAceptado(t, h, lnLaunch(t, s, "con-nivel", s.Token(), raCuerpo("pedir-reviewer", "codex", "", "", "")))
	lnEsperarLanzamientos(t, h, s)
	for _, args := range f.llamadas(t, "codex") {
		if m, _ := lnSigue(args, "-m"); m != modelo {
			t.Fatalf("%s: la review corre con el modelo traducido %q, fue %q", h, modelo, m)
		}
	}
	log, err := os.ReadFile(filepath.Join(root, ".hoom", "envelopes", r.EnvelopeID+".log"))
	if err != nil {
		t.Fatal(err)
	}
	if want := "  modelo      " + modelo + " (nivel fuerte)\n"; !strings.Contains(string(log), want) {
		t.Fatalf("hallazgo 7918f8: la cabecera de la review dice %q:\n%s", want, log)
	}
	recs, _ := reviewcmd.Records(wt)
	if len(recs) != 1 || recs[0].Model != modelo || recs[0].Nivel != modelos.Fuerte {
		t.Fatalf("%s: el registro de la review trae Model %s y Nivel fuerte: %+v", h, modelo, recs)
	}
}
