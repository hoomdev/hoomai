// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-310): reviewcmd.writerOf usa agents.WritesCode, asi que la review
// cruzada y el logo del tablero no pueden discrepar sobre quien escribio.
package reviewcmd

import (
	"fmt"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
)

// CA-310: para cada rol de la tabla (nombre corto y nativo), para el rol
// vacio y para uno desconocido, writerOf reconoce un writer exactamente
// cuando agents.WritesCode dice que ese rol escribe codigo, y la regla es la
// de siempre (ni ReadOnly ni scope specs).
func TestCA310_WriterOfUsaWritesCode(t *testing.T) {
	var roles []string
	esperado := map[string]bool{"": true, "desconocido": true}
	for _, r := range agents.Roles() {
		roles = append(roles, r.Slug, r.Native)
		esperado[r.Slug] = !(r.ReadOnly || r.Scope == agents.ScopeSpecs)
		esperado[r.Native] = esperado[r.Slug]
	}
	roles = append(roles, "", "desconocido")

	for i, rol := range roles {
		root := t.TempDir()
		id := fmt.Sprintf("20260923T10%04d_wc%04d", i, i)
		metaDeRun(t, root, id, "codex", rol, time.Now())
		meta, ok := writerOf(root, root)
		if ok != esperado[rol] {
			t.Errorf("CA-310: con un solo run de rol %q, writerOf encuentra writer=%v; debe ser %v", rol, ok, esperado[rol])
		}
		if ok != agents.WritesCode(rol) {
			t.Errorf("CA-310: writerOf y agents.WritesCode discrepan sobre el rol %q (writerOf=%v, WritesCode=%v)", rol, ok, agents.WritesCode(rol))
		}
		if ok && meta.Provider != "codex" {
			t.Errorf("CA-310: el writer que reconoce writerOf es el provider del run: %+v", meta)
		}
	}
}
