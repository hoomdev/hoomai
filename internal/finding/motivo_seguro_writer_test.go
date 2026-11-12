package finding

import (
	"strings"
	"testing"
)

// Finding 04b283: a motivo carries no raw control or direction character
// from a raise's author, a finding's file or a record id.
func TestMotivoEscapaAutorYFile(t *testing.T) {
	sucio := "x\x1b[2J‮evil"
	c := Contexto{BlockOn: "high",
		Subidas:   map[string]Subida{"s": {Author: sucio}},
		Riesgos:   map[string]string{"t": "alto"},
		Completas: []RevisionCompleta{{ID: sucio, Task: "t", Findings: []string{"c"}}}}
	for _, f := range []Finding{
		{ID: "s", Severity: "low", File: "app.go"},
		{ID: "r", Severity: "medium", File: "auth/" + sucio},
		{ID: "c", Severity: "medium", File: "app.go", Task: "t"},
	} {
		b, m := Bloquea(f, c)
		if !b || strings.ContainsAny(m, "\x1b‮") || !strings.Contains(m, `\x1b`) {
			t.Fatalf("%s: (%v, %q): el motivo escapa el valor", f.ID, b, m)
		}
	}
	if _, m := Bloquea(Finding{ID: "s", Severity: "low"}, Contexto{Subidas: map[string]Subida{"s": {Author: "Ana <a@b>"}}}); m != "subido a mano por Ana <a@b>" {
		t.Fatalf("un autor de texto plano queda como esta: %q", m)
	}
}
