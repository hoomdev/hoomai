// Tests adversariales del spec .hoom/specs/hoom-agent-sobre-determinista.md:
// la tabla de roles es la fuente unica (subagentes nativos e invocacion
// headless salen del mismo dato) y el contrato es obligatorio.
package agents

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CA-127: Roles() devuelve los 10 roles con su scope; Lookup resuelve slug y
// nombre nativo, es insensible a mayusculas y falla listando los validos.
func TestCA127_TablaDeRolesYLookup(t *testing.T) {
	all := Roles()
	if len(all) != 10 {
		t.Fatalf("CA-127: se esperaban 10 roles, hay %d", len(all))
	}
	quiere := map[string]string{
		"orquestador": ScopeEvidencia, "arquitecto": ScopeEvidencia,
		"designer": ScopeEvidencia, "scout": ScopeEvidencia,
		"writer": ScopeCodigo, "test-writer": ScopeTests,
		"reviewer": ScopeEvidencia, "characterizer": ScopeTests,
		"analista": ScopeEvidencia, "refutador": ScopeEvidencia,
	}
	for _, r := range all {
		want, ok := quiere[r.Slug]
		if !ok {
			t.Fatalf("CA-127: rol inesperado %q", r.Slug)
		}
		if r.Scope != want {
			t.Fatalf("CA-127: %s deberia tener scope %q, tiene %q", r.Slug, want, r.Scope)
		}
		if r.Native != "hoom-"+r.Slug || r.File == "" || r.Desc == "" {
			t.Fatalf("CA-127: metadata incompleta en %+v", r)
		}
	}
	// mutar la copia no puede tocar la tabla
	all[0].Scope = "roto"
	if Roles()[0].Scope != ScopeEvidencia {
		t.Fatal("CA-127: Roles() debe devolver una copia, no la tabla viva")
	}

	for _, name := range []string{"writer", "hoom-writer", "WRITER", "  writer  "} {
		r, err := Lookup(name)
		if err != nil || r.Slug != "writer" {
			t.Fatalf("CA-127: Lookup(%q) = %+v, %v", name, r, err)
		}
	}
	_, err := Lookup("escritor")
	if err == nil {
		t.Fatal("CA-127: un rol desconocido debe fallar")
	}
	for _, s := range []string{"escritor", "writer", "test-writer", "refutador"} {
		if !strings.Contains(err.Error(), s) {
			t.Fatalf("CA-127: el error debe nombrar el rol pedido y listar los validos: %v", err)
		}
	}
}

// CA-128: Contract usa la copia del proyecto si existe, el embebido si no, y
// se niega ante un contrato vacio.
func TestCA128_ContratoComoSystemPrompt(t *testing.T) {
	r, err := Lookup("writer")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()

	// sin .hoom/agents/: el embebido
	emb, err := Contract(dir, r)
	if err != nil {
		t.Fatalf("CA-128: sin copia local debe caer al contrato embebido: %v", err)
	}
	if !strings.Contains(emb, "Writer") {
		t.Fatalf("CA-128: el contrato embebido no parece el del writer: %q", emb[:min(80, len(emb))])
	}

	// con copia local: gana la copia (el humano pudo editarla)
	local := filepath.Join(dir, ".hoom", "agents")
	if err := os.MkdirAll(local, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(local, r.File)
	if err := os.WriteFile(path, []byte("# Writer del proyecto\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Contract(dir, r)
	if err != nil || !strings.Contains(got, "Writer del proyecto") {
		t.Fatalf("CA-128: debe ganar la copia local: %q, %v", got, err)
	}

	// contrato vacio: un rol sin contrato no es un rol
	if err := os.WriteFile(path, []byte("   \n\t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Contract(dir, r); err == nil {
		t.Fatal("CA-128: un contrato vacio debe ser error, no un system prompt en blanco")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
