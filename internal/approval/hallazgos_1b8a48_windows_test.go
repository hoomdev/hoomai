//go:build windows

// Regresion del hallazgo 1b8a48 (ronda 2 de proporcion-por-riesgo), el caso
// literal: en Windows filepath.Rel no distingue mayusculas, y la tarjeta del
// spec sale de la misma ruta que el registro (relSpec). Con root C:\Repo,
// c:\repo\.hoom\specs\x.md es el spec de la tarjeta x y su registro es
// .hoom/specs/x.md; otra unidad o un .hoom/specs anidado no tienen tarjeta
// (647f9a). Sin disco: rutaEn, tarjetaDelSpec y relSpec son de rutas.
package approval

import "testing"

func TestHallazgo_1b8a48_WindowsLaTarjetaNoDistingueMayusculas(t *testing.T) {
	root := `C:\Repo`
	for _, sp := range []string{`.hoom\specs\x.md`, `c:\repo\.hoom\specs\x.md`, `C:\REPO\.hoom\specs\x.md`, `c:\Repo\.hoom\specs\x.md`} {
		ruta := rutaEn(root, sp)
		slug, ok := tarjetaDelSpec(root, ruta)
		if rel := relSpec(root, ruta); !ok || slug != "x" || rel != ".hoom/specs/x.md" {
			t.Fatalf("hallazgo 1b8a48: %s es el spec de la tarjeta x y firma .hoom/specs/x.md: (%q, %v), %q", sp, slug, ok, rel)
		}
	}
	for _, sp := range []string{`D:\otro\.hoom\specs\x.md`, `C:\Repo\sub\.hoom\specs\x.md`} {
		if _, ok := tarjetaDelSpec(root, rutaEn(root, sp)); ok {
			t.Fatalf("hallazgo 1b8a48: %s es otra ruta, sin tarjeta (647f9a)", sp)
		}
	}
}
