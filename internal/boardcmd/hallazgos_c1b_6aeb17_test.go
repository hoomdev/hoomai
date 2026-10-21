// Tests adversariales del hallazgo 20261021T150458_6aeb17 (medium,
// resilience) de la review 1 de fixes-cabina-c1b, sobre el tablero
// (.hoom/specs/items-y-columna-derivada.md: CA-280, la evidencia sale del
// worktree de la tarea si existe; CA-269, el siguiente paso) y el doctor
// (.hoom/specs/historia-doctor-y-cinta.md: CA-369, los espacios de trabajo
// sin item). Escritos desde el contrato, nunca desde la implementacion:
//
// Cuando git no puede responder si la carpeta de la tarea es un worktree de
// la raiz (git fuera del PATH), eso no es "la tarea no tiene espacio de
// trabajo":
//   - el tablero (Build y CardFor) no esconde la tarjeta ni la muestra como
//     si no tuviera espacio de trabajo (su siguiente paso no es `hoom task
//     start`): la tarjeta queda bloqueada con un motivo (Card.Missing) que
//     dice que no se pudo comprobar el espacio de trabajo;
//   - el doctor devuelve un error en vez de omitir la tarea (como con un
//     directorio ilegible, hallazgo e5d1e4).
//
// Guardas: con git sano la tarjeta lee su worktree y el doctor ve el espacio
// suelto, sin error. Reusa las utilidades bd* de gather_test.go.
package boardcmd

import (
	"os/exec"
	"strings"
	"testing"
	"time"
)

var b6Now = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

// b6SinGit saca git del PATH hasta el final del test.
func b6SinGit(t *testing.T) {
	t.Helper()
	t.Setenv("PATH", t.TempDir())
	if _, err := exec.LookPath("git"); err == nil {
		t.Fatal("fixture: git no esta en el PATH")
	}
}

// b6Tarjeta: la tarjeta precios con su worktree (registrado, rama
// hoom/precios) y su spec commiteado ahi, nada en el arbol actual.
func b6Tarjeta(t *testing.T) string {
	t.Helper()
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "pedido: el pedido de precios\n")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdCommitear(t, wt, "spec de precios")
	return root
}

// b6MotivoSinComprobar devuelve el motivo de c que dice que no se pudo
// comprobar el espacio de trabajo, o "".
func b6MotivoSinComprobar(c Card) string {
	for _, m := range c.Missing {
		l := strings.ToLower(m)
		if strings.Contains(l, "comprobar") && strings.Contains(l, "espacio de trabajo") {
			return m
		}
	}
	return ""
}

// b6Bloqueada exige que la tarjeta este bloqueada con el motivo de no poder
// comprobar su espacio de trabajo, y no presentada como sin espacio.
func b6Bloqueada(t *testing.T, como string, c Card) {
	t.Helper()
	if b6MotivoSinComprobar(c) == "" {
		t.Errorf("CA-280: hallazgo 6aeb17 (%s): sin poder comprobar el espacio de trabajo, la tarjeta queda bloqueada con un motivo que lo dice; missing=%q column=%s next=%q source=%s",
			como, c.Missing, c.Column, c.Next, c.Evidence.Source)
	}
	if strings.Contains(c.Next, "hoom task start") {
		t.Errorf("CA-269: hallazgo 6aeb17 (%s): la tarjeta no se muestra como si no tuviera espacio de trabajo: su siguiente paso no es %q", como, c.Next)
	}
}

// Hallazgo 6aeb17. CA-280 y CA-269: con git fuera del PATH, Build no
// esconde la tarjeta y la deja bloqueada con el motivo de no poder comprobar
// su espacio de trabajo; CardFor da lo mismo.
func TestHallazgo6aeb17_CA280_TableroSinGitBloqueaLaTarjeta(t *testing.T) {
	root := b6Tarjeta(t)
	c, err := CardFor(root, "main", "high", bdSlug, b6Now)
	if err != nil || c.Evidence.Source != SourceWorktree || b6MotivoSinComprobar(c) != "" {
		t.Fatalf("precondicion con git: la tarjeta lee su worktree sin motivos de comprobacion: err=%v source=%s missing=%q", err, c.Evidence.Source, c.Missing)
	}

	b6SinGit(t)
	b, err := Build(root, "main", "high", b6Now)
	if err != nil {
		t.Fatalf("CA-280: hallazgo 6aeb17: sin git el tablero se arma igual (la tarjeta queda bloqueada, no se cae todo): %v", err)
	}
	b6Bloqueada(t, "Build", bdCard(t, b, bdSlug))

	c, err = CardFor(root, "main", "high", bdSlug, b6Now)
	if err != nil {
		t.Fatalf("CA-280: hallazgo 6aeb17: sin git CardFor da la tarjeta bloqueada, no un error: %v", err)
	}
	b6Bloqueada(t, "CardFor", c)
}

// Hallazgo 6aeb17. CA-369: con git fuera del PATH, el doctor no puede saber
// si .hoom/worktrees/suelto (un espacio de trabajo sin item, con cambios y
// sin veredicto) es una tarea: devuelve un error en vez de un reporte que la
// omite.
func TestHallazgo6aeb17_CA369_DoctorSinGitEsUnError(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	hdEspacioSuelto(t, root)
	r, err := Doctor(root, "main", "high", b6Now)
	if err != nil || !b6TieneSlug(r, "suelto") {
		t.Fatalf("precondicion con git: el doctor ve el espacio suelto: err=%v problemas=%v", err, drIDs(r.Problems))
	}

	b6SinGit(t)
	r, err = Doctor(root, "main", "high", b6Now)
	if err == nil {
		t.Errorf("CA-369: hallazgo 6aeb17: sin git el doctor devuelve error, no un reporte de %d problemas %v que omite el espacio suelto",
			len(r.Problems), drIDs(r.Problems))
	}
}

func b6TieneSlug(r DoctorReport, slug string) bool {
	for _, p := range r.Problems {
		if p.Slug == slug {
			return true
		}
	}
	return false
}

// Guarda del hallazgo 6aeb17. CA-280 y CA-369: con git sano, una tarjeta
// sin worktree sigue en el arbol actual con `hoom task start` y sin motivo
// de comprobacion; una con worktree lo lee; y el doctor de un proyecto con
// una carpeta comun en .hoom/worktrees (que no es una tarea) responde sin
// error.
func TestHallazgo6aeb17_CA280_GuardaConGitSano(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, "sin-tarea", "Sin tarea", "pedido: x\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "pedido: el pedido de precios\n")
	bdWorktree(t, root, bdSlug)
	bdEscribir(t, root, ".hoom/worktrees/comun/x.go", "package x\n")

	b, err := Build(root, "main", "high", b6Now)
	if err != nil {
		t.Fatalf("CA-280: guarda 6aeb17: Build con git sano: %v", err)
	}
	sin := bdCard(t, b, "sin-tarea")
	if sin.Evidence.Source != SourceArbol || sin.Next != "hoom task start sin-tarea" || b6MotivoSinComprobar(sin) != "" {
		t.Fatalf("CA-269: guarda 6aeb17: sin worktree la tarjeta sigue en el arbol con `hoom task start`: source=%s next=%q missing=%q",
			sin.Evidence.Source, sin.Next, sin.Missing)
	}
	con := bdCard(t, b, bdSlug)
	if con.Evidence.Source != SourceWorktree || b6MotivoSinComprobar(con) != "" {
		t.Fatalf("CA-280: guarda 6aeb17: con worktree la tarjeta lo lee, sin motivo de comprobacion: source=%s missing=%q", con.Evidence.Source, con.Missing)
	}
	if _, err := Doctor(root, "main", "high", b6Now); err != nil {
		t.Fatalf("CA-369: guarda 6aeb17: con git sano una carpeta comun en .hoom/worktrees no es un error del doctor: %v", err)
	}
}
