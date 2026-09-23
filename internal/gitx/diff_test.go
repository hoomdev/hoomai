// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-315): gitx.BranchDiff es base...HEAD de un arbol (lo commiteado de su
// rama desde la base, nunca lo sin commitear ni lo que la base avanzo), con
// numstat, head y el parche cortado en un fin de linea. Si git falla,
// available es false y la nota trae el error.
package gitx

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func dfGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// dfRepo: main con dos archivos, y una rama "tarea" con un cambio
// commiteado (un archivo nuevo de 3 lineas y uno modificado), mas cambios
// sin commitear que el diff no debe ver. Despues main avanza con un archivo
// que la rama no toco: base...HEAD no lo muestra.
func dfRepo(t *testing.T) string {
	t.Helper()
	idAislarGit(t) // sin la config global del que corre el test (prefijos, color)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dfGit(t, dir, "init", "-b", "main")
	dfGit(t, dir, "config", "user.email", "test@hoom.dev")
	dfGit(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "app.go", "package app\n\nfunc A() int { return 1 }\n")
	write(t, dir, "lib.go", "package app\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "inicial")
	dfGit(t, dir, "checkout", "-q", "-b", "tarea")
	write(t, dir, "nuevo.go", "package app\n\nfunc Nuevo() {}\n")
	write(t, dir, "app.go", "package app\n\nfunc A() int { return 2 }\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "cambio de la tarea")
	// main avanza con algo que la rama no toco
	dfGit(t, dir, "checkout", "-q", "main")
	write(t, dir, "solo_main.go", "package app\n\nvar SoloMain = 1\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "main avanza")
	dfGit(t, dir, "checkout", "-q", "tarea")
	// lo sin commitear: un archivo nuevo y una modificacion
	write(t, dir, "sucio.go", "package app\n\nvar Sucio = 1\n")
	write(t, dir, "lib.go", "package app\n\nvar SinCommitear = 1\n")
	return dir
}

// CA-315: con la rama de la tarea, BranchDiff trae archivos, inserciones,
// borrados, head y el parche de base...HEAD, sin lo sin commitear y sin lo
// que la base avanzo despues.
func TestCA315_BranchDiffBaseTresPuntosHead(t *testing.T) {
	dir := dfRepo(t)
	d, _ := BranchDiff(dir, "main", 256<<10)
	if !d.Available || d.Note != "" {
		t.Fatalf("CA-315: con la rama y la base, el diff esta disponible y sin nota: %+v", d)
	}
	if d.Base != "main" {
		t.Fatalf("CA-315: base es la rama base pedida: %q", d.Base)
	}
	if head := dfGit(t, dir, "rev-parse", "--short=12", "HEAD"); d.Head != head || len(d.Head) != 12 {
		t.Fatalf("CA-315: head es el sha corto (12) de HEAD %q, fue %q", head, d.Head)
	}
	porRuta := map[string]DiffFile{}
	for _, f := range d.Files {
		porRuta[f.Path] = f
	}
	if len(d.Files) != 2 || len(porRuta) != 2 {
		t.Fatalf("CA-315: solo los dos archivos commiteados de la rama (nuevo.go y app.go): %+v", d.Files)
	}
	if f, ok := porRuta["nuevo.go"]; !ok || f.Insertions != 3 || f.Deletions != 0 {
		t.Fatalf("CA-315: nuevo.go con 3 inserciones y 0 borrados: %+v", d.Files)
	}
	if f, ok := porRuta["app.go"]; !ok || f.Insertions != 1 || f.Deletions != 1 {
		t.Fatalf("CA-315: app.go con 1 insercion y 1 borrado: %+v", d.Files)
	}
	if d.Insertions != 4 || d.Deletions != 1 {
		t.Fatalf("CA-315: los totales son la suma del numstat (4 y 1): %d %d", d.Insertions, d.Deletions)
	}
	for _, ajeno := range []string{"sucio.go", "lib.go", "solo_main.go", "SinCommitear", "SoloMain"} {
		if strings.Contains(d.Patch, ajeno) {
			t.Fatalf("CA-315: el parche es base...HEAD commiteado: no trae %q\n%s", ajeno, d.Patch)
		}
	}
	for _, quiero := range []string{"diff --git a/app.go b/app.go", "diff --git a/nuevo.go b/nuevo.go", "+func Nuevo() {}", "-func A() int { return 1 }"} {
		if !strings.Contains(d.Patch, quiero) {
			t.Fatalf("CA-315: el parche trae %q:\n%s", quiero, d.Patch)
		}
	}
	if d.Truncated {
		t.Fatalf("CA-315: un parche chico no se corta: %+v", d)
	}
	if want := dfGit(t, dir, "diff", "--no-color", "--no-ext-diff", "main...HEAD"); strings.TrimRight(d.Patch, "\n") != want {
		t.Fatalf("CA-315: el parche es exactamente git diff main...HEAD:\nquiere:\n%s\nfue:\n%s", want, d.Patch)
	}
}

// CA-315: un parche de mas de maxBytes se corta en el ultimo fin de linea
// que entra, con truncated true; los archivos y los totales siguen enteros.
func TestCA315_BranchDiffCortaEnFinDeLinea(t *testing.T) {
	dir := dfRepo(t)
	var b strings.Builder
	for i := 0; i < 3000; i++ {
		b.WriteString("linea numero ")
		b.WriteString(strings.Repeat("x", i%37))
		b.WriteString(" fin\n")
	}
	write(t, dir, "grande.txt", b.String())
	dfGit(t, dir, "add", "grande.txt")
	dfGit(t, dir, "commit", "-q", "-m", "grande")

	entero, _ := BranchDiff(dir, "main", 64<<20)
	if !entero.Available || entero.Truncated || len(entero.Patch) < 50000 {
		t.Fatalf("CA-315: con un limite holgado el parche va entero: available=%v truncated=%v len=%d", entero.Available, entero.Truncated, len(entero.Patch))
	}
	// un limite que cae en medio de una linea (ni el byte max-1 ni el max son
	// fin de linea): el corte es el ultimo '\n' antes del limite
	max := 20000
	for entero.Patch[max-1] == '\n' || entero.Patch[max] == '\n' {
		max++
	}
	d, _ := BranchDiff(dir, "main", max)
	if !d.Available || !d.Truncated {
		t.Fatalf("CA-315: un parche de mas de %d bytes se corta y lo dice: available=%v truncated=%v", max, d.Available, d.Truncated)
	}
	want := entero.Patch[:strings.LastIndexByte(entero.Patch[:max], '\n')+1]
	if d.Patch != want {
		t.Fatalf("CA-315: el corte es en el ultimo fin de linea antes de %d bytes: len %d, quiere %d", max, len(d.Patch), len(want))
	}
	if len(d.Patch) > max || !strings.HasSuffix(d.Patch, "\n") {
		t.Fatalf("CA-315: el parche cortado no pasa el limite y termina en fin de linea: len=%d", len(d.Patch))
	}
	if d.Insertions != entero.Insertions || d.Deletions != entero.Deletions || len(d.Files) != len(entero.Files) || d.Insertions < 3000 {
		t.Fatalf("CA-315: cortar el parche no corta el numstat: %d/%d %d vs %d/%d %d",
			d.Insertions, d.Deletions, len(d.Files), entero.Insertions, entero.Deletions, len(entero.Files))
	}
	if d.Head != entero.Head || d.Base != "main" {
		t.Fatalf("CA-315: head y base no cambian con el corte: %+v", d)
	}
}

// CA-315: si git falla (la base no existe, o no es un repo), available es
// false con el error de git en la nota.
func TestCA315_BranchDiffSiGitFalla(t *testing.T) {
	dir := dfRepo(t)
	d, _ := BranchDiff(dir, "no-existe-esta-base", 256<<10)
	if d.Available || strings.TrimSpace(d.Note) == "" {
		t.Fatalf("CA-315: sin la rama base, available false y la nota con el error: %+v", d)
	}
	if d.Patch != "" || len(d.Files) != 0 {
		t.Fatalf("CA-315: sin diff no hay parche ni archivos: %+v", d)
	}
	d, _ = BranchDiff(t.TempDir(), "main", 256<<10)
	if d.Available || strings.TrimSpace(d.Note) == "" {
		t.Fatalf("CA-315: fuera de un repo, available false y la nota con el error: %+v", d)
	}
}

// CA-315: una rama sin cambios contra la base es un diff disponible y vacio.
func TestCA315_BranchDiffSinCambios(t *testing.T) {
	idAislarGit(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dfGit(t, dir, "init", "-b", "main")
	dfGit(t, dir, "config", "user.email", "test@hoom.dev")
	dfGit(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "app.go", "package app\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "inicial")
	dfGit(t, dir, "checkout", "-q", "-b", "tarea")
	write(t, dir, "sucio.go", "package app\n")
	d, _ := BranchDiff(dir, "main", 256<<10)
	if !d.Available || d.Patch != "" || len(d.Files) != 0 || d.Insertions != 0 || d.Deletions != 0 || d.Truncated {
		t.Fatalf("CA-315: sin commits propios el diff esta disponible y vacio (lo sin commitear no entra): %+v", d)
	}
}
