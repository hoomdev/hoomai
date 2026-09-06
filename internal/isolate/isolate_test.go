// Tests adversariales del spec .hoom/specs/test-writer-en-arbol-ciego.md:
// el arbol ciego. No alcanza con que el archivo no este: hay que poder probar
// que no estuvo, incluso en el caso que git no reporta.
package isolate

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitc(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v en %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func existe(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	return err == nil
}

// proyecto arma un repo con el layout que hace interesante al problema: el
// test y su implementacion son VECINOS en el mismo directorio.
func proyecto(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	gitc(t, root, "init", "-b", "main")
	gitc(t, root, "config", "user.email", "test@hoom.dev")
	gitc(t, root, "config", "user.name", "hoom test")
	write(t, root, "go.mod", "module demo\n\ngo 1.24\n")
	write(t, root, ".gitignore", "tmp/\n")
	write(t, root, "internal/x/x.go", "package x\n\nfunc Suma(a, b int) int { return a + b }\n")
	write(t, root, "internal/x/x_test.go", "package x\n\n// CA-1\nfunc TestSuma(t *testing.T) {}\n")
	write(t, root, "internal/y/y.go", "package y\n")
	write(t, root, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\n")
	write(t, root, ".hoom/specs/s.md", "# spec\nCA-1: sumar\n")
	write(t, root, ".hoom/verdicts/v.json", `{"verdict":"red","gate":"internal/x/x.go:3: undefined"}`)
	write(t, root, ".hoom/findings/f.json", `{"desc":"return a + b esta mal"}`)
	gitc(t, root, "add", "-A")
	gitc(t, root, "commit", "-m", "inicial")
	return root
}

// patronesDeTests son los globs de la forma `tests` tal como los declara la
// politica de escritura del rol.
var patronesDeTests = []string{".hoom/**", "tests/**", "**/*_test.go"}

func abrir(t *testing.T, root string) *Tree {
	t.Helper()
	tree, err := Open(root, "test-writer_x", Patterns(patronesDeTests, nil, []string{"go.mod"}))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { tree.Close() })
	return tree
}

// CA-172: los globs del rol viajan tal cual, los marcadores se anclan y la
// evidencia que CITA codigo se excluye al final, donde una exclusion manda.
func TestCA172_PatronesDelArbolCiego(t *testing.T) {
	got := Patterns([]string{".hoom/**", "**/*_test.go"}, []string{"secreto/**"}, []string{"go.mod", "/artisan"})
	want := []string{
		".hoom/**", "**/*_test.go", // allow tal cual y en orden
		"/go.mod", "/artisan", // marcadores anclados
		".gitignore",
		"!secreto/**", "!.hoom/verdicts/**", "!.hoom/findings/**", // negativos al final
	}
	if len(got) != len(want) {
		t.Fatalf("CA-172: patrones\n got=%v\nwant=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("CA-172: patron %d es %q y deberia ser %q (%v)", i, got[i], want[i], got)
		}
	}
	// un marcador sin anclar matchearia vendor/<algo>/go.mod
	for _, p := range got {
		if p == "go.mod" {
			t.Fatal("CA-172: los marcadores se anclan con / o matchean a cualquier profundidad")
		}
	}
	// determinista y sin repetidos
	again := Patterns([]string{".hoom/**", ".hoom/**"}, nil, []string{"go.mod"})
	if n := strings.Count(strings.Join(again, "\n"), ".hoom/**\n"); n != 1 {
		t.Fatalf("CA-172: la salida no debe repetir patrones: %v", again)
	}
	if len(Patterns(nil, nil, nil)) == 0 {
		t.Fatal("CA-172: siempre viajan .gitignore y los negativos de la evidencia")
	}
}

// CA-173: el arbol ciego tiene el test y no su implementacion, aunque sean
// vecinos; el spec entra, la evidencia que cita codigo no; y el checkout del
// padre queda intacto.
func TestCA173_OpenArmaElArbolCiego(t *testing.T) {
	root := proyecto(t)
	tree := abrir(t, root)

	presentes := map[string]bool{
		"internal/x/x_test.go": true, ".hoom/specs/s.md": true, "go.mod": true, ".gitignore": true,
	}
	ausentes := map[string]bool{
		"internal/x/x.go": true, "internal/y/y.go": true,
		".hoom/verdicts/v.json": true, ".hoom/findings/f.json": true,
	}
	for p := range presentes {
		if !existe(t, filepath.Join(tree.Dir, p)) {
			t.Fatalf("CA-173: %s deberia estar en el arbol ciego", p)
		}
	}
	for p := range ausentes {
		if existe(t, filepath.Join(tree.Dir, p)) {
			t.Fatalf("CA-173: %s NO deberia estar en el arbol ciego", p)
		}
	}
	if tree.Commit != gitc(t, root, "rev-parse", "HEAD") {
		t.Fatalf("CA-173: el arbol ciego se arma desde HEAD del padre: %q", tree.Commit)
	}
	if len(tree.Hidden) == 0 {
		t.Fatal("CA-173: los testigos son las rutas que git dejo afuera; no puede estar vacio")
	}
	for p := range ausentes {
		var visto bool
		for _, h := range tree.Hidden {
			if h == p {
				visto = true
			}
		}
		if !visto {
			t.Fatalf("CA-173: %s deberia estar entre los testigos: %v", p, tree.Hidden)
		}
	}

	// el padre no se entera: la config es del worktree
	if !existe(t, filepath.Join(root, "internal/x/x.go")) {
		t.Fatal("CA-173: el checkout del padre no se toca")
	}
	if st := gitc(t, root, "status", "--porcelain"); st != "" {
		t.Fatalf("CA-173: el padre debe quedar limpio: %q", st)
	}
}

// CA-174: cada error de Open trae su accion y no deja un arbol a medias.
func TestCA174_ErroresDeOpen(t *testing.T) {
	sinGit := t.TempDir()
	_, err := Open(sinGit, "n", patronesDeTests)
	if err == nil || !strings.Contains(err.Error(), "git init") {
		t.Fatalf("CA-174: fuera de git el error debe traer 'git init': %v", err)
	}

	sinCommits := t.TempDir()
	gitc(t, sinCommits, "init", "-b", "main")
	_, err = Open(sinCommits, "n", patronesDeTests)
	if err == nil || !strings.Contains(err.Error(), "git commit") {
		t.Fatalf("CA-174: sin commits el error debe traer 'git commit': %v", err)
	}

	root := proyecto(t)
	tree := abrir(t, root)
	_, err = Open(root, "test-writer_x", patronesDeTests)
	if err == nil || !strings.Contains(err.Error(), "worktree remove --force") {
		t.Fatalf("CA-174: una cuarentena existente debe traer el comando para descartarla: %v", err)
	}
	if !existe(t, tree.Dir) {
		t.Fatal("CA-174: el error no puede borrar la cuarentena que ya estaba")
	}

	otro := proyecto(t)
	_, err = Open(otro, "vacio", []string{"no/existe/**"})
	if err == nil || !strings.Contains(err.Error(), "agents.<rol>.write.allow") {
		t.Fatalf("CA-174: patrones que no matchean nada deben remitir a hoom.yaml: %v", err)
	}
	if existe(t, filepath.Join(otro, ".hoom", "isolated", "vacio")) {
		t.Fatal("CA-174: un Open fallido no deja el worktree a medias")
	}
	if lst := gitc(t, otro, "worktree", "list"); strings.Contains(lst, "vacio") {
		t.Fatalf("CA-174: tampoco lo deja registrado en git: %s", lst)
	}
}

// CA-175: la prueba del aislamiento mira el DISCO, y el conjunto de testigos
// se congela al abrir.
func TestCA175_BreachesMiraElDisco(t *testing.T) {
	root := proyecto(t)
	tree := abrir(t, root)
	if b := tree.Breaches(); len(b) != 0 {
		t.Fatalf("CA-175: recien abierto el aislamiento esta intacto: %v", b)
	}

	// el caso que git NO reporta: el archivo vuelve con el contenido de HEAD,
	// git limpia el bit skip-worktree y `git status` se queda callado.
	original := "package x\n\nfunc Suma(a, b int) int { return a + b }\n"
	write(t, tree.Dir, "internal/x/x.go", original)
	if st := gitc(t, tree.Dir, "status", "--porcelain"); st != "" {
		t.Fatalf("CA-175: este es el caso en que git se calla; si reporta algo, el test perdio su gracia: %q", st)
	}
	b := tree.Breaches()
	if len(b) != 1 || b[0] != "internal/x/x.go" {
		t.Fatalf("CA-175: escribir el testigo a mano es romper el aislamiento: %v", b)
	}
	if err := os.Remove(filepath.Join(tree.Dir, "internal/x/x.go")); err != nil {
		t.Fatal(err)
	}
	gitc(t, tree.Dir, "sparse-checkout", "reapply")
	if b := tree.Breaches(); len(b) != 0 {
		t.Fatalf("CA-175: devuelto a su lugar, el aislamiento vuelve a estar intacto: %v", b)
	}

	// el camino real de evasion
	gitc(t, tree.Dir, "sparse-checkout", "disable")
	if b := tree.Breaches(); len(b) < 2 {
		t.Fatalf("CA-175: 'sparse-checkout disable' restaura el arbol entero: %v", b)
	}

	// y ensanchar los patrones no achica la lista congelada
	antes := len(tree.Hidden)
	gitc(t, tree.Dir, "sparse-checkout", "set", "--no-cone", "/**")
	if len(tree.Hidden) != antes {
		t.Fatalf("CA-175: los testigos se congelan al abrir: %d -> %d", antes, len(tree.Hidden))
	}
	if len(tree.Breaches()) == 0 {
		t.Fatal("CA-175: una prueba que el sujeto puede reescribir no es una prueba")
	}
}

// CA-176: el trasplante mueve exactamente lo pedido y no sale del destino.
func TestCA176_ApplyTrasplantaLoJusto(t *testing.T) {
	root := proyecto(t)
	tree := abrir(t, root)

	write(t, tree.Dir, "internal/z/z_test.go", "package z\n\n// CA-2\n") // alta con directorio nuevo
	write(t, tree.Dir, "internal/x/x_test.go", "package x\n\n// CA-1 reescrito\n")
	if err := os.Remove(filepath.Join(tree.Dir, ".hoom/specs/s.md")); err != nil { // baja
		t.Fatal(err)
	}
	write(t, tree.Dir, "internal/x/otro_test.go", "package x\n// no viaja\n") // no esta en la lista

	applied, removed, err := tree.Apply(root, []string{
		"internal/z/z_test.go", "internal/x/x_test.go", ".hoom/specs/s.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(applied, ",") != "internal/x/x_test.go,internal/z/z_test.go" {
		t.Fatalf("CA-176: applied ordenado con las altas y sobrescrituras: %v", applied)
	}
	if strings.Join(removed, ",") != ".hoom/specs/s.md" {
		t.Fatalf("CA-176: removed con lo que el rol borro: %v", removed)
	}
	raw, err := os.ReadFile(filepath.Join(root, "internal/x/x_test.go"))
	if err != nil || !strings.Contains(string(raw), "reescrito") {
		t.Fatalf("CA-176: la sobrescritura debe llegar al destino: %q %v", raw, err)
	}
	if !existe(t, filepath.Join(root, "internal/z/z_test.go")) {
		t.Fatal("CA-176: el alta debe crear sus directorios intermedios")
	}
	if existe(t, filepath.Join(root, ".hoom/specs/s.md")) {
		t.Fatal("CA-176: la baja debe reproducirse en el destino")
	}
	if existe(t, filepath.Join(root, "internal/x/otro_test.go")) {
		t.Fatal("CA-176: lo que no esta en la lista no viaja")
	}

	// nada sale del destino, y una ruta invalida no escribe NADA
	write(t, tree.Dir, "internal/nuevo_test.go", "package x\n")
	for _, mala := range []string{"../fuera.go", "/etc/passwd", "internal/../../fuera.go"} {
		a, r, err := tree.Apply(root, []string{"internal/nuevo_test.go", mala})
		if err == nil {
			t.Fatalf("CA-176: %q deberia rechazarse", mala)
		}
		if len(a) != 0 || len(r) != 0 {
			t.Fatalf("CA-176: un trasplante rechazado no escribe nada: %v %v", a, r)
		}
		if existe(t, filepath.Join(root, "internal/nuevo_test.go")) {
			t.Fatalf("CA-176: %q dejo escribir la ruta valida antes de fallar", mala)
		}
	}

	// un enlace simbolico puede apuntar afuera: no viaja
	if err := os.Symlink("/etc/passwd", filepath.Join(tree.Dir, "internal/link_test.go")); err != nil {
		t.Skipf("sin symlinks en este entorno: %v", err)
	}
	if _, _, err := tree.Apply(root, []string{"internal/link_test.go"}); err == nil {
		t.Fatal("CA-176: el trasplante no mueve enlaces simbolicos")
	}
}

// CA-177: cerrar la cuarentena funciona con archivos sin seguimiento, que es
// como siempre queda una que hizo su trabajo.
func TestCA177_CloseConArchivosSinSeguimiento(t *testing.T) {
	root := proyecto(t)
	tree, err := Open(root, "test-writer_cierre", Patterns(patronesDeTests, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	write(t, tree.Dir, "internal/x/nuevo_test.go", "package x\n\n// CA-3\n")
	if st := gitc(t, tree.Dir, "status", "--porcelain"); st == "" {
		t.Fatal("CA-177: el escenario es una cuarentena con archivos sin seguimiento")
	}
	if err := tree.Close(); err != nil {
		t.Fatalf("CA-177: cerrar debe funcionar igual: %v", err)
	}
	if existe(t, tree.Dir) {
		t.Fatal("CA-177: la cuarentena debe desaparecer del disco")
	}
	if lst := gitc(t, root, "worktree", "list"); strings.Contains(lst, "test-writer_cierre") {
		t.Fatalf("CA-177: git ya no debe listarla: %s", lst)
	}
}
