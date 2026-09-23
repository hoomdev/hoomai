// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-294): roles que dejan rastro. Cada Desc termina con la oracion
// "Deja: <artefacto>." y las que mueven una columna nombran exactamente lo
// que lee la derivacion. CA-229 (sin "Solo lectura" en los autores de specs,
// ninguna vacia) se sigue cumpliendo.
package agents

import (
	"strings"
	"testing"
)

// CA-294: la tabla de rastros del contrato.
func TestCA294_CadaRolNombraSuRastro(t *testing.T) {
	nombra := map[string][]string{
		"arquitecto":  {".hoom/specs/<slug>.md"},
		"test-writer": {"CA-n"},
		"writer":      {"hoom verify --spec"},
		"reviewer":    {".hoom/reviews/", ".hoom/findings/"},
		"refutador":   {".res.json"},
		"analista":    {".hoom/specs/00-vision.md", ".hoom/specs/backlog.md"},
		"designer":    {".hoom/specs/"},
	}
	roles := Roles()
	if len(roles) != 10 {
		t.Fatalf("CA-294: la tabla tiene 10 roles, tiene %d", len(roles))
	}
	for _, r := range roles {
		desc := strings.TrimSpace(r.Desc)
		if desc == "" {
			t.Fatalf("CA-294: la Desc de %s no puede quedar vacia", r.Slug)
		}
		i := strings.LastIndex(desc, "Deja:")
		if i < 0 {
			t.Fatalf("CA-294: la Desc de %s contiene 'Deja:': %q", r.Slug, desc)
		}
		if !strings.HasSuffix(desc, ".") {
			t.Fatalf("CA-294: la Desc de %s termina con la oracion 'Deja: <artefacto>.': %q", r.Slug, desc)
		}
		rastro := desc[i:]
		if strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(rastro, "Deja:"), ".")) == "" {
			t.Fatalf("CA-294: 'Deja:' de %s nombra algo: %q", r.Slug, rastro)
		}
		for _, s := range nombra[r.Slug] {
			if !strings.Contains(rastro, s) {
				t.Fatalf("CA-294: el rastro de %s nombra %q: %q", r.Slug, s, rastro)
			}
		}
		if (r.Slug == "arquitecto" || r.Slug == "designer" || r.Slug == "analista") && strings.Contains(r.Desc, "Solo lectura") {
			t.Fatalf("CA-229: la Desc de %s no dice 'Solo lectura': %q", r.Slug, r.Desc)
		}
	}
	// los que no dejan nada propio lo dicen, no lo callan
	for _, slug := range []string{"orquestador", "scout"} {
		r, err := Lookup(slug)
		if err != nil {
			t.Fatal(err)
		}
		rastro := r.Desc[strings.LastIndex(r.Desc, "Deja:"):]
		if !strings.Contains(strings.ToLower(rastro), "nada") {
			t.Fatalf("CA-294: %s dice que no deja nada propio: %q", slug, rastro)
		}
	}
}
