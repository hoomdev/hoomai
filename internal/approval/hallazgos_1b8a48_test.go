// Regresion del hallazgo 1b8a48 (ronda 2 de proporcion-por-riesgo): la
// tarjeta que mide un spec y el registro que firma su aprobacion salen de la
// misma ruta canonica (relSpec). Si el registro se llama
// .hoom/specs/precios.md, el spec se midio con la tarjeta precios, con
// cualquier grafia de la ruta; si no la tiene, el registro es de otra ruta.
// El caso literal de Windows (C:\Repo contra c:\repo\...) esta en
// hallazgos_1b8a48_windows_test.go.
package approval_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
)

func TestHallazgo_1b8a48_LaTarjetaYElRegistroSonDeLaMismaRuta(t *testing.T) {
	const canonica = ".hoom/specs/precios.md"
	// la tarjeta precios es de riesgo bajo: tope 400 palabras
	root, sp, m := pprProyecto(t, "", 600, 2, "riesgo: bajo\n")
	for _, ruta := range []string{
		sp,
		canonica,
		"./" + canonica,
		".hoom/./specs/precios.md",
		"docs/../.hoom/specs/precios.md",
		root + "/./" + canonica,
		root + "//.hoom/specs//precios.md",
	} {
		if c := approval.CalibreDe(m, root, ruta); c.Tarjeta != "precios" {
			t.Fatalf("hallazgo 1b8a48: %q es %s del root: se mide con su tarjeta: %+v", ruta, canonica, c)
		}
		var neg *approval.Negativa
		if _, _, err := approval.Aprobar(m, root, ruta); !errors.As(err, &neg) {
			t.Fatalf("hallazgo 1b8a48: %q pasa el tope de su tarjeta (400): *Negativa, fue %v", ruta, err)
		}
	}
	if n := pprRegistros(t, root); n != 0 {
		t.Fatalf("hallazgo 1b8a48: ninguna grafia firma %s sin su tarjeta: hay %d registros", canonica, n)
	}
	// sin tarjeta (otra ruta, 647f9a), el registro no es el de la ruta canonica
	for _, ruta := range []string{
		pprEscribir(t, t.TempDir(), canonica, pprTexto(t, 600, 2)),
		pprEscribir(t, root, filepath.Join("docs", canonica), pprTexto(t, 600, 2)),
	} {
		if c := approval.CalibreDe(m, root, ruta); c.Tarjeta != "" {
			t.Fatalf("hallazgo 1b8a48: %s es otra ruta, sin tarjeta: %+v", ruta, c)
		}
		rec, _, err := approval.Aprobar(m, root, ruta)
		if err != nil || rec.Spec == canonica {
			t.Fatalf("hallazgo 1b8a48: %s se aprueba sin tarjeta con otro nombre que %s: %+v %v", ruta, canonica, rec, err)
		}
	}
}
