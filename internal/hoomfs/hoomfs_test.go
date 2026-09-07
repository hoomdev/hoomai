// Tests del writer para los hallazgos 20260906T234757_e9ed9b (una sola fuente
// de directorios locales), 20260906T235816_0293b4 y 20260907T000658_19274d
// (escritura atomica) y 20260906T235816_b26478 (.gitignore completo) de la
// review cruzada sobre Spec D + Spec E.
package hoomfs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIsLocal(t *testing.T) {
	for p, want := range map[string]bool{
		".hoom/runs/x.jsonl":     true,
		".hoom/envelopes":        true,
		".hoom/cache/live.jsonl": true,
		".hoom/verdicts/x.json":  false,
		".hoom/findings/x.json":  false,
		".hoom/runsx/y":          false,
		"internal/x.go":          false,
	} {
		if got := IsLocal(p); got != want {
			t.Fatalf("IsLocal(%q) = %v, esperaba %v", p, got, want)
		}
	}
}

// 20260906T235816_b26478: completar el ignore es selectivo por nombre, y un
// archivo que ya tiene lo pedido no se reescribe ni byte a byte.
func TestEnsureIgnoredSelectivoEIdempotente(t *testing.T) {
	root := t.TempDir()
	gi := filepath.Join(root, ".hoom", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gi, []byte("worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	added, err := EnsureIgnored(root, "isolated")
	if err != nil || len(added) != 1 || added[0] != "isolated/" {
		t.Fatalf("agregar solo isolated/: added=%v err=%v", added, err)
	}
	if raw, _ := os.ReadFile(gi); string(raw) != "worktrees/\nisolated/\n" {
		t.Fatalf("contenido inesperado:\n%s", raw)
	}
	before, _ := os.Stat(gi)
	if added, _ := EnsureIgnored(root, "isolated"); added != nil {
		t.Fatalf("segunda vez no debe agregar nada: %v", added)
	}
	after, _ := os.Stat(gi)
	if !after.ModTime().Equal(before.ModTime()) {
		t.Fatal("un archivo completo no se reescribe")
	}
	added, _ = EnsureIgnored(root)
	if strings.Join(added, ",") != "cache/,runs/,envelopes/" {
		t.Fatalf("completar todo agrega lo que falta en orden: %v", added)
	}
	raw, _ := os.ReadFile(gi)
	for _, d := range Local {
		if !strings.Contains(string(raw), d+"/\n") {
			t.Fatalf("falta %s/ en el ignore completo:\n%s", d, raw)
		}
	}
}

// 20260906T235816_0293b4: la escritura pasa por un temporal y un rename, y no
// deja restos ni cambia el modo.
func TestAtomicWriteSinRestos(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.json")
	for _, content := range []string{"{\"a\":1}\n", "{\"a\":2,\"b\":\"largo\"}\n"} {
		if err := AtomicWrite(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(path)
		if err != nil || string(raw) != content {
			t.Fatalf("contenido: %q err=%v", raw, err)
		}
		entries, _ := os.ReadDir(dir)
		if len(entries) != 1 || entries[0].Name() != "x.json" {
			t.Fatalf("quedaron restos en el directorio: %v", entries)
		}
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o644 {
		t.Fatalf("modo: %v", st.Mode())
	}
}
