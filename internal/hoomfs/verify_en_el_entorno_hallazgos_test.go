// Regresiones de los hallazgos de la delta 1 de la review de
// verify-en-el-entorno (ad7fac), escritas por el writer en la ronda a mano,
// sobre completarGitignore (EnsureIgnored y EnsureIgnoredFiles):
//
//   - 3faa18: completar un .hoom/.gitignore que existe no le cambia el modo
//     ni el inodo (un 0600 queda en 0600); uno nuevo queda en 0644;
//   - 4583f7: lo que otra mano hace entre la lectura y la escritura (anexar
//     una linea en sitio, reemplazar el archivo por rename, crearlo, o
//     llamar EnsureIgnored) sobrevive, y lo agregado esta en el archivo.
package hoomfs

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
)

// vrhGitignore escribe <root>/.hoom/.gitignore con body y modo exactos (sin
// la umask) y devuelve su ruta; body "" = sin el archivo.
func vrhGitignore(t *testing.T, root, body string, modo os.FileMode) string {
	t.Helper()
	gi := filepath.Join(root, ".hoom", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
		t.Fatal(err)
	}
	if body == "" {
		return gi
	}
	if err := os.WriteFile(gi, []byte(body), modo); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(gi, modo); err != nil {
		t.Fatal(err)
	}
	return gi
}

// vrhTiene dice si raw tiene la linea l (recortada, como la cuenta hoomfs).
func vrhTiene(raw []byte, l string) bool {
	for _, x := range strings.Split(string(raw), "\n") {
		if strings.TrimSpace(x) == l {
			return true
		}
	}
	return false
}

// 3faa18: EnsureIgnoredFiles y EnsureIgnored completan un .hoom/.gitignore
// 0600 sin cambiarle el modo ni el inodo, con o sin salto de linea al final;
// uno nuevo queda en 0644, con solo las lineas pedidas.
func TestHallazgo3faa18_CompletarConservaModoEInodo(t *testing.T) {
	for _, c := range []struct {
		nombre string
		llamar func(root string) ([]string, error)
		linea  string
	}{
		{"EnsureIgnoredFiles", func(root string) ([]string, error) { return EnsureIgnoredFiles(root, "local.yaml") }, "local.yaml"},
		{"EnsureIgnored", func(root string) ([]string, error) { return EnsureIgnored(root, "worktrees") }, "worktrees/"},
	} {
		for _, antes := range []string{"cache/\n", "cache/"} {
			caso := c.nombre + " sobre " + strings.ReplaceAll(antes, "\n", `\n`)
			root := t.TempDir()
			gi := vrhGitignore(t, root, antes, 0o600)
			st0, err := os.Lstat(gi)
			if err != nil {
				t.Fatal(err)
			}
			added, err := c.llamar(root)
			if err != nil || !slices.Equal(added, []string{c.linea}) {
				t.Fatalf("3faa18: %s: agrega %q: %q %v", caso, c.linea, added, err)
			}
			raw, err := os.ReadFile(gi)
			if err != nil {
				t.Fatal(err)
			}
			if want := "cache/\n" + c.linea + "\n"; string(raw) != want {
				t.Fatalf("3faa18: %s: el archivo queda %q: %q", caso, want, raw)
			}
			st1, err := os.Lstat(gi)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(st0, st1) {
				t.Fatalf("3faa18: %s: el .hoom/.gitignore conserva su inodo", caso)
			}
			if runtime.GOOS != "windows" && st1.Mode().Perm() != 0o600 {
				t.Fatalf("3faa18: %s: el .hoom/.gitignore conserva su modo 0600: %v", caso, st1.Mode().Perm())
			}
		}
		root := t.TempDir()
		gi := vrhGitignore(t, root, "", 0)
		added, err := c.llamar(root)
		if err != nil || !slices.Equal(added, []string{c.linea}) {
			t.Fatalf("3faa18: %s sin el archivo: lo crea con %q: %q %v", c.nombre, c.linea, added, err)
		}
		st, err := os.Lstat(gi)
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && st.Mode().Perm() != 0o644 {
			t.Fatalf("3faa18: %s sin el archivo: el nuevo queda en 0644: %v", c.nombre, st.Mode().Perm())
		}
		if raw, _ := os.ReadFile(gi); string(raw) != c.linea+"\n" {
			t.Fatalf("3faa18: %s sin el archivo: el nuevo trae solo %q: %q", c.nombre, c.linea, raw)
		}
	}
}

// vrhGancho arma antesDeCompletar para que corra otra una sola vez, en la
// primera ventana entre la lectura y la escritura; se desarma solo y al
// terminar el test.
func vrhGancho(t *testing.T, otra func(root string)) {
	t.Helper()
	var una sync.Once
	f := func(root string) {
		una.Do(func() {
			antesDeCompletar.Store(nil)
			otra(root)
		})
	}
	antesDeCompletar.Store(&f)
	t.Cleanup(func() { antesDeCompletar.Store(nil) })
}

// 4583f7: con otra mano que, entre la lectura y la escritura de
// EnsureIgnoredFiles, anexa una linea en sitio, reemplaza el archivo por
// rename (como un editor), lo crea cuando no estaba, o llama
// EnsureIgnored(root, "worktrees") (otro hoom), la linea de la otra mano
// sobrevive. Si EnsureIgnoredFiles dice que agrego local.yaml, esta en el
// archivo; si devuelve un error, la version ajena queda intacta.
func TestHallazgo4583f7_LoQueOtraManoEscribeEnLaVentanaSobrevive(t *testing.T) {
	const ajena = "regla-de-la-persona"
	for _, c := range []struct {
		nombre, antes string
		otra          func(t *testing.T, gi string)
		suya          string // la linea de la otra mano
	}{
		{"anexa en sitio", "cache/\n", func(t *testing.T, gi string) {
			f, err := os.OpenFile(gi, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			if _, err := f.WriteString(ajena + "\n"); err != nil {
				t.Error(err)
			}
		}, ajena},
		{"reemplaza por rename", "cache/\n", func(t *testing.T, gi string) {
			if err := os.WriteFile(gi+".nuevo", []byte("cache/\n"+ajena+"\n"), 0o644); err != nil {
				t.Error(err)
				return
			}
			if err := os.Rename(gi+".nuevo", gi); err != nil {
				t.Error(err)
			}
		}, ajena},
		{"lo crea", "", func(t *testing.T, gi string) {
			if err := os.WriteFile(gi, []byte(ajena+"\n"), 0o644); err != nil {
				t.Error(err)
			}
		}, ajena},
		{"otro hoom con EnsureIgnored", "cache/\n", func(t *testing.T, gi string) {
			root := filepath.Dir(filepath.Dir(gi))
			if added, err := EnsureIgnored(root, "worktrees"); err != nil || !slices.Equal(added, []string{"worktrees/"}) {
				t.Errorf("4583f7: el otro hoom agrega worktrees/: %q %v", added, err)
			}
		}, "worktrees/"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := t.TempDir()
			gi := vrhGitignore(t, root, c.antes, 0o644)
			vrhGancho(t, func(string) { c.otra(t, gi) })
			added, err := EnsureIgnoredFiles(root, "local.yaml")
			raw, rerr := os.ReadFile(gi)
			if rerr != nil {
				t.Fatal(rerr)
			}
			if !vrhTiene(raw, c.suya) {
				t.Fatalf("4583f7: %s: la linea %q de la otra mano sobrevive (err %v): %q", c.nombre, c.suya, err, raw)
			}
			if err != nil {
				return // un error, con la version ajena intacta, tambien vale
			}
			if !slices.Equal(added, []string{"local.yaml"}) || !vrhTiene(raw, "local.yaml") {
				t.Fatalf("4583f7: %s: dice que agrego %q y el archivo lo tiene: %q", c.nombre, added, raw)
			}
			if c.antes != "" && !vrhTiene(raw, "cache/") {
				t.Fatalf("4583f7: %s: lo de antes sigue: %q", c.nombre, raw)
			}
		})
	}
}
