// Tests adversariales de los hallazgos 20261021T174140_48eeed (medium,
// reliability) y 20261021T174455_44ed99 (medium, resilience) de la delta 1
// de fixes-cabina-c1b, sobre el tablero (.hoom/specs/items-y-columna-derivada.md:
// CA-280 y CA-269) y el doctor (.hoom/specs/historia-doctor-y-cinta.md:
// CA-369). Escritos desde el contrato, nunca desde la implementacion:
//
// Si la carpeta de la tarea no tiene permiso de busqueda, no se puede leer su
// .git: no se sabe si es la tarea (gitx.ErrSinComprobar), y eso no es "no
// tiene espacio de trabajo". El tablero (Build y CardFor) deja la tarjeta
// bloqueada con el motivo de no poder comprobar su espacio de trabajo y sin
// `hoom task start`; el doctor devuelve un error en vez de omitir el espacio.
// Los permisos vuelven a 0755 al terminar.
//
// Reusa b6Tarjeta, b6Bloqueada y b6Now de hallazgos_c1b_6aeb17_test.go, las
// utilidades bd* de gather_test.go y hdEspacioSuelto del doctor.
package boardcmd

import (
	"os"
	"path/filepath"
	"testing"
)

// b48SinPermisos deja dir en 000 y lo devuelve a 0755 al terminar el test.
// Saltea si corre como root o si el sistema deja listarlo igual.
func b48SinPermisos(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("corre como root: un chmod no le quita la lectura, el test no probaria nada")
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("este sistema deja listar un directorio sin permiso de lectura")
	}
}

// Hallazgos 48eeed y 44ed99. CA-280 y CA-269: con .hoom/worktrees/precios
// en 000, Build y CardFor dejan la tarjeta bloqueada con el motivo de no
// poder comprobar su espacio de trabajo, no como si no lo tuviera.
func TestHallazgo48eeed_CA280_TableroConLaCarpetaIlegibleBloqueaLaTarjeta(t *testing.T) {
	root := b6Tarjeta(t)
	b48SinPermisos(t, filepath.Join(root, ".hoom", "worktrees", bdSlug))
	b, err := Build(root, "main", "high", b6Now)
	if err != nil {
		t.Fatalf("CA-280: hallazgo 48eeed: con la carpeta de la tarea ilegible el tablero se arma igual: %v", err)
	}
	b6Bloqueada(t, "carpeta en 000, Build", bdCard(t, b, bdSlug))
	c, err := CardFor(root, "main", "high", bdSlug, b6Now)
	if err != nil {
		t.Fatalf("CA-280: hallazgo 48eeed: CardFor da la tarjeta bloqueada, no un error: %v", err)
	}
	b6Bloqueada(t, "carpeta en 000, CardFor", c)
}

// Hallazgos 48eeed y 44ed99. CA-369: con .hoom/worktrees/suelto (un
// espacio sin item, con cambios y sin veredicto) en 000, el doctor devuelve
// un error en vez de un reporte que lo omite.
func TestHallazgo48eeed_CA369_DoctorConLaCarpetaIlegibleEsUnError(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	hdEspacioSuelto(t, root)
	b48SinPermisos(t, filepath.Join(root, ".hoom", "worktrees", "suelto"))
	if r, err := Doctor(root, "main", "high", b6Now); err == nil {
		t.Errorf("CA-369: hallazgo 48eeed: con .hoom/worktrees/suelto ilegible el doctor devuelve error, no un reporte de %d problemas %v",
			len(r.Problems), drIDs(r.Problems))
	}
}
