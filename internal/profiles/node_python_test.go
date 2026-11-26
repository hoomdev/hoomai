package profiles_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/profiles"
)

func TestPerfilesNodeYPython(t *testing.T) {
	// Resolve both profiles cleanly
	for _, name := range []string{"node", "python"} {
		gates, desc, err := profiles.Resolve(name)
		if err != nil {
			t.Fatalf("Resolve(%q): %v", name, err)
		}
		if desc == "" || gates["test"].CmdStr() == "" {
			t.Fatalf("perfil %q incompleto: desc=%q gates=%+v", name, desc, gates)
		}
		cache, err := profiles.Cache(name)
		if err != nil || len(cache) == 0 {
			t.Fatalf("Cache(%q): %v %v", name, cache, err)
		}
		rutas, err := profiles.Rutas(name)
		if err != nil || len(rutas.Generadas) == 0 {
			t.Fatalf("Rutas(%q): %v %+v", name, err, rutas)
		}
	}

	// Node: npx --no-install (no descarga paquete tsc equivocado) y static/build opcionales por defecto
	nodeGates, _, err := profiles.Resolve("node")
	if err != nil {
		t.Fatal(err)
	}
	if nodeGates["static"].CmdStr() != "npx --no-install tsc --noEmit" || nodeGates["static"].IsRequired() {
		t.Fatalf("node static debe usar --no-install y ser opcional por defecto: %+v", nodeGates["static"])
	}
	if nodeGates["build"].IsRequired() {
		t.Fatalf("node build debe ser opcional por defecto: %+v", nodeGates["build"])
	}

	// Python: mypy opcional por defecto para no dar rojo en repos sin tipado estricto
	pyGates, _, err := profiles.Resolve("python")
	if err != nil {
		t.Fatal(err)
	}
	if pyGates["static"].IsRequired() {
		t.Fatalf("python static (mypy) debe ser opcional por defecto: %+v", pyGates["static"])
	}

	// DetectStack: Node project
	nodeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(nodeDir, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := profiles.DetectStack(nodeDir); err != nil || got != "node" {
		t.Fatalf("DetectStack(node) = %q, %v", got, err)
	}

	// DetectStack: Python project
	pyDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(pyDir, "pyproject.toml"), []byte("[project]\nname='demo'\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, err := profiles.DetectStack(pyDir); err != nil || got != "python" {
		t.Fatalf("DetectStack(python) = %q, %v", got, err)
	}

	// DetectStack: Laravel with package.json still detects laravel (score 2 > 1)
	larDir := t.TempDir()
	for _, f := range []string{"composer.json", "artisan", "package.json"} {
		if err := os.WriteFile(filepath.Join(larDir, f), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := profiles.DetectStack(larDir); err != nil || got != "laravel" {
		t.Fatalf("DetectStack(laravel+package.json) = %q, %v", got, err)
	}
}
