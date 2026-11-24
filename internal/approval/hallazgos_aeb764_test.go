// Regresion del hallazgo aeb764 (ronda 3 de proporcion-por-riesgo): el
// nombre del spec en su registro y el que Status busca pasan por la misma
// grafia canonica. Con cualquier grafia de la ruta (relativa, con ./, o
// absoluta no canonica, dentro o fuera del arbol), si Approve o Aprobar dicen
// exito o already, Status de esa misma ruta dice aprobado.
package approval_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHallazgo_aeb764_CualquierGrafiaQueSeApruebaQuedaAprobada(t *testing.T) {
	const h = "aeb764"
	const spec = ".hoom/specs/precios.md"
	// relativas: del cwd para Approve y Status, del root para Aprobar; aqui son el mismo
	for _, ruta := range []string{spec, "./" + spec} {
		root, _, m := pprProyecto(t, "", 100, 2, "-")
		t.Chdir(root)
		h6dAprobado(t, h, m, root, ruta, spec)
	}
	// absoluta no canonica dentro del arbol
	root, _, m := pprProyecto(t, "", 100, 2, "-")
	if err := os.MkdirAll(filepath.Join(root, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	h6dAprobado(t, h, m, root, root+"/docs/../"+spec, spec)
	// absoluta no canonica fuera del arbol: <base>/repo/../otro/x.md
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	pprEscribir(t, base, "otro/x.md", pprTexto(t, 120, 2))
	h6dAprobado(t, h, m, root, base+"/repo/../otro/x.md", filepath.ToSlash(filepath.Join(base, "otro", "x.md")))
}
