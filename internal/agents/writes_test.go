// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-310): "este rol escribe codigo" es UNA regla, agents.WritesCode, y sale
// de la tabla de roles: falso para los ReadOnly y los de scope specs,
// verdadero para los demas, para el rol vacio (hoom run) y para uno
// desconocido.
package agents

import "testing"

// CA-310: la regla por rol, con los nombres cortos, los nativos y sin
// distinguir mayusculas (como Lookup, que es lo que usaba writerOf).
func TestCA310_WritesCodePorRol(t *testing.T) {
	casos := map[string]bool{
		// escriben codigo
		"writer":        true,
		"test-writer":   true,
		"characterizer": true,
		// autores de specs: escriben, pero no codigo
		"arquitecto": false,
		"designer":   false,
		"analista":   false,
		// solo lectura
		"reviewer":    false,
		"scout":       false,
		"refutador":   false,
		"orquestador": false,
		// sin rol (hoom run) o uno que la tabla no conoce: nada dice que no
		// escribio
		"":            true,
		"desconocido": true,
		"hoom-nadie":  true,
		// los nombres nativos y las mayusculas resuelven al mismo rol
		"hoom-reviewer":   false,
		"hoom-arquitecto": false,
		"hoom-writer":     true,
		"Reviewer":        false,
		" arquitecto ":    false,
		"WRITER":          true,
	}
	for rol, quiere := range casos {
		if got := WritesCode(rol); got != quiere {
			t.Errorf("CA-310: WritesCode(%q) debe ser %v, fue %v", rol, quiere, got)
		}
	}
}

// CA-310: la regla es la tabla, no una lista aparte: para cada rol de
// Roles(), WritesCode es "ni ReadOnly ni scope specs".
func TestCA310_WritesCodeSaleDeLaTabla(t *testing.T) {
	escriben, noEscriben := 0, 0
	for _, r := range Roles() {
		quiere := !(r.ReadOnly || r.Scope == ScopeSpecs)
		if got := WritesCode(r.Slug); got != quiere {
			t.Errorf("CA-310: WritesCode(%q) debe ser %v (read_only=%v scope=%s), fue %v", r.Slug, quiere, r.ReadOnly, r.Scope, got)
		}
		if got := WritesCode(r.Native); got != quiere {
			t.Errorf("CA-310: WritesCode(%q) (nombre nativo) debe ser %v, fue %v", r.Native, quiere, got)
		}
		if quiere {
			escriben++
		} else {
			noEscriben++
		}
	}
	if escriben == 0 || noEscriben == 0 {
		t.Fatalf("CA-310: la tabla tiene roles de los dos lados (escriben %d, no escriben %d)", escriben, noEscriben)
	}
}
