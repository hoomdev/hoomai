// Tests de regresion de la review cruzada (Codex, 2026-09-06) sobre Spec D + Spec E.
package isolate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// lo que HEAD tiene en las rutas que estos tests tocan (ver proyecto)
const (
	hrTestEnHead = "package x\n\n// CA-1\nfunc TestSuma(t *testing.T) {}\n"
	hrSpecEnHead = "# spec\nCA-1: sumar\n"
)

// hrLeer devuelve el contenido de un archivo, o "" si no existe.
func hrLeer(t *testing.T, root, rel string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if os.IsNotExist(err) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Hallazgos 20260906T235816_63a17d y 20260907T001832_20d561: Tree.Apply
// sobrescribe (y borra) el destino en el arbol real sin comprobar que ese
// archivo siga igual a HEAD. El arbol ciego nace de HEAD y beforeReal solo
// detecta cambios ocurridos DURANTE el run, asi que un cambio local previo sin
// commitear, o un archivo sin trackear en la misma ruta, se destruye en
// silencio cuando el rol toca esa ruta. Apply debe negarse nombrando la ruta
// y dejar el archivo como estaba.
func TestHallazgo_63a17d_ApplyNoPisaCambiosLocalesPrevios(t *testing.T) {
	casos := []struct {
		nombre, ruta string
		local        string                         // lo que el arbol real tiene ANTES de arrancar, distinto de HEAD
		rol          func(t *testing.T, tree *Tree) // lo que el rol hizo en el arbol ciego
	}{
		{"modificado-sin-commitear", "internal/x/x_test.go",
			"package x\n\n// CA-1 con un cambio local que nadie commiteo\n",
			func(t *testing.T, tree *Tree) {
				write(t, tree.Dir, "internal/x/x_test.go", "package x\n\n// CA-1 reescrito por el rol\n")
			}},
		{"sin-trackear", "internal/x/nuevo_test.go",
			"package x\n\n// borrador local, nunca agregado a git\n",
			func(t *testing.T, tree *Tree) {
				write(t, tree.Dir, "internal/x/nuevo_test.go", "package x\n\n// CA-2 del rol\n")
			}},
		{"borrado-por-el-rol", ".hoom/specs/s.md",
			"# spec\nCA-1: sumar\nCA-2: restar (nota local sin commitear)\n",
			func(t *testing.T, tree *Tree) {
				if err := os.Remove(filepath.Join(tree.Dir, ".hoom/specs/s.md")); err != nil {
					t.Fatal(err)
				}
			}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			root := proyecto(t)
			write(t, root, c.ruta, c.local)
			tree := abrir(t, root)
			c.rol(t, tree)

			applied, removed, err := tree.Apply(root, []string{c.ruta})
			if err == nil {
				t.Fatalf("63a17d: %s difiere de HEAD en el arbol real y el trasplante lo piso sin avisar (applied=%v removed=%v)", c.ruta, applied, removed)
			}
			if !strings.Contains(err.Error(), c.ruta) {
				t.Fatalf("63a17d: la negativa debe nombrar la ruta %s: %v", c.ruta, err)
			}
			if len(applied)+len(removed) != 0 {
				t.Fatalf("63a17d: un trasplante negado no aplica nada: %v %v", applied, removed)
			}
			if got := hrLeer(t, root, c.ruta); got != c.local {
				t.Fatalf("63a17d: el cambio local de %s debe quedar intacto:\n got=%q\nwant=%q", c.ruta, got, c.local)
			}
		})
	}
}

// Hallazgo 20260907T000658_7dbbdb: Apply valida las rutas antes de mutar, pero
// lee y escribe o borra cada archivo dentro del MISMO bucle: si la ruta N
// falla (permisos, E/S, un destino que no es un archivo), las 1..N-1 ya
// quedaron aplicadas en el arbol real y no hay rollback; transplant descarta
// las listas parciales, asi que el sobre queda roto sin registrar que cambio.
// El trasplante tiene que ser todo o nada.
func TestHallazgo_7dbbdb_ApplyEsTodoONada(t *testing.T) {
	// tres rutas que a priori estan bien (una baja, una sobrescritura y un
	// alta) y una cuarta que falla recien al escribir en el destino
	previas := []string{".hoom/specs/s.md", "internal/x/x_test.go", "internal/z/z_test.go"}
	preparar := func(t *testing.T) (string, *Tree) {
		t.Helper()
		root := proyecto(t)
		tree := abrir(t, root)
		if err := os.Remove(filepath.Join(tree.Dir, ".hoom/specs/s.md")); err != nil {
			t.Fatal(err)
		}
		write(t, tree.Dir, "internal/x/x_test.go", "package x\n\n// CA-1 reescrito\n")
		write(t, tree.Dir, "internal/z/z_test.go", "package z\n\n// CA-2\n")
		return root, tree
	}
	comprobar := func(t *testing.T, root string, applied, removed []string, err error) {
		t.Helper()
		if err == nil {
			t.Fatal("7dbbdb: el escenario necesita que la ultima ruta falle; si no fallo, el test perdio su gracia")
		}
		if got := hrLeer(t, root, ".hoom/specs/s.md"); got != hrSpecEnHead {
			t.Fatalf("7dbbdb: la baja de .hoom/specs/s.md quedo aplicada aunque el trasplante fallo despues: %q", got)
		}
		if got := hrLeer(t, root, "internal/x/x_test.go"); got != hrTestEnHead {
			t.Fatalf("7dbbdb: la sobrescritura de internal/x/x_test.go quedo aplicada aunque el trasplante fallo despues: %q", got)
		}
		if existe(t, filepath.Join(root, "internal/z/z_test.go")) {
			t.Fatal("7dbbdb: el alta de internal/z/z_test.go quedo aplicada aunque el trasplante fallo despues")
		}
		if len(applied)+len(removed) != 0 {
			t.Fatalf("7dbbdb: un trasplante fallido no reporta rutas aplicadas: %v %v", applied, removed)
		}
	}

	t.Run("destino-es-directorio", func(t *testing.T) {
		root, tree := preparar(t)
		// en el arbol real hay un directorio VACIO donde el rol dejo un archivo
		// (vacio para que git no lo vea: la falla es de E/S, no de scope)
		write(t, tree.Dir, "internal/w/w_test.go", "package w\n")
		if err := os.MkdirAll(filepath.Join(root, "internal/w/w_test.go"), 0o755); err != nil {
			t.Fatal(err)
		}
		rutas := append(append([]string(nil), previas...), "internal/w/w_test.go")
		applied, removed, err := tree.Apply(root, rutas)
		comprobar(t, root, applied, removed, err)
	})

	t.Run("sin-permiso-de-escritura", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignora los permisos del directorio")
		}
		root, tree := preparar(t)
		// el alta cae en un directorio del arbol real sin permiso de escritura
		write(t, tree.Dir, "internal/y/y_test.go", "package y\n")
		dirY := filepath.Join(root, "internal/y")
		if err := os.Chmod(dirY, 0o555); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dirY, 0o755) })
		rutas := append(append([]string(nil), previas...), "internal/y/y_test.go")
		applied, removed, err := tree.Apply(root, rutas)
		comprobar(t, root, applied, removed, err)
	})
}

// Hallazgo 20260907T001831_18b191: Apply solo rechaza enlaces simbolicos en el
// ORIGEN; safeJoin valida el destino lexicalmente sin resolverlo, y
// os.MkdirAll/os.WriteFile siguen un symlink ya presente en el archivo destino
// o en un directorio ancestro. Como ese symlink puede existir antes de
// beforeReal, el gate no lo marca como fuga y un test aprobado trunca o crea
// un archivo FUERA del arbol real, contra CA-176.
func TestHallazgo_18b191_ApplyNoSigueSymlinksDelDestino(t *testing.T) {
	const intacto = "package fuera // no me toques\n"

	t.Run("archivo-destino", func(t *testing.T) {
		root := proyecto(t)
		fuera := t.TempDir()
		victima := filepath.Join(fuera, "victima.go")
		if err := os.WriteFile(victima, []byte(intacto), 0o644); err != nil {
			t.Fatal(err)
		}
		destino := filepath.Join(root, "internal/x/x_test.go")
		if err := os.Remove(destino); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(victima, destino); err != nil {
			t.Skipf("sin symlinks en este entorno: %v", err)
		}
		tree := abrir(t, root)
		write(t, tree.Dir, "internal/x/x_test.go", "package x\n\n// CA-1 reescrito\n")

		if _, _, err := tree.Apply(root, []string{"internal/x/x_test.go"}); err == nil {
			t.Fatal("18b191: el destino es un symlink que apunta fuera del arbol y el trasplante debe negarse")
		}
		if got := hrLeer(t, fuera, "victima.go"); got != intacto {
			t.Fatalf("18b191: la escritura salio del arbol real y piso el archivo externo: %q", got)
		}
		if st, err := os.Lstat(destino); err != nil || st.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("18b191: un trasplante negado deja el destino como estaba (el symlink): %v %v", st, err)
		}
	})

	t.Run("directorio-ancestro", func(t *testing.T) {
		root := proyecto(t)
		fuera := t.TempDir()
		if err := os.Symlink(fuera, filepath.Join(root, "internal/w")); err != nil {
			t.Skipf("sin symlinks en este entorno: %v", err)
		}
		tree := abrir(t, root)
		write(t, tree.Dir, "internal/w/w_test.go", "package w\n")

		if _, _, err := tree.Apply(root, []string{"internal/w/w_test.go"}); err == nil {
			t.Fatal("18b191: un directorio ancestro del destino es un symlink hacia fuera del arbol y el trasplante debe negarse")
		}
		if existe(t, filepath.Join(fuera, "w_test.go")) {
			t.Fatal("18b191: el alta se creo fuera del arbol real, a traves del symlink")
		}
	})
}
