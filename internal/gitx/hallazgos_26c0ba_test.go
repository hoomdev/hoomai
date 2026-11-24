package gitx

// Regresion de los hallazgos 85c48e y d4c535 (review 26c0ba): Snapshot
// lista lo commiteado con --diff-filter=ACMR y no trae un borrado;
// ArchivosConBorrados lo suma desde LineasDelCambio, sin el estado de hoom
// que el candidato deja fuera.

import (
	"reflect"
	"testing"
)

func TestHallazgo85c48e_ArchivosConBorrados(t *testing.T) {
	const ca = "hallazgo 85c48e"
	dir := retyRepo(t)
	retyEscribir(t, dir, "internal/auth/token.go", retyLineas(30, "token"))
	retyEscribir(t, dir, "README.md", "# demo\n")
	retyGit(t, dir, "add", "-A")
	retyGit(t, dir, "commit", "-q", "-m", "base")
	retyGit(t, dir, "checkout", "-q", "-b", "rama")
	retyGit(t, dir, "rm", "-q", "--", "internal/auth/token.go")
	retyEscribir(t, dir, "README.md", "# demo\n\notra\n")
	retyEscribir(t, dir, ".hoom/verdicts/v.json", "{}\n")
	retyEscribir(t, dir, ".hoom/items/precios.yaml", "titulo: x\n")
	retyGit(t, dir, "add", "-A")
	retyGit(t, dir, "commit", "-q", "-m", "borra token")

	snap := Snapshot(dir, "main")
	if !reflect.DeepEqual(snap.ChangedFiles, []string{"README.md"}) {
		t.Fatalf("%s: premisa: el snapshot no trae el borrado commiteado: %q", ca, snap.ChangedFiles)
	}
	lineas, err := LineasDelCambio(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	got := ArchivosConBorrados(snap.ChangedFiles, lineas)
	want := []string{"README.md", "internal/auth/token.go"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: los archivos del cambio con sus borrados, sin veredictos ni items: %q, quiero %q", ca, got, want)
	}
	if !reflect.DeepEqual(snap.ChangedFiles, []string{"README.md"}) {
		t.Fatalf("%s: ArchivosConBorrados no toca changed: %q", ca, snap.ChangedFiles)
	}
	if got := ArchivosConBorrados(nil, nil); got == nil || len(got) != 0 {
		t.Fatalf("%s: sin nada, una lista vacia: %#v", ca, got)
	}
}
