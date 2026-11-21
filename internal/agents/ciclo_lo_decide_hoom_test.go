// Tests adversariales del spec .hoom/specs/ciclo-lo-decide-hoom.md (CA-758):
// la descripcion del rol refutador en agents.Roles dice «los hallazgos que
// bloquean» y no «los hallazgos abiertos». Es lo que el orquestador lee para
// delegar: con «abiertos» le mandaria tambien los pendientes.
package agents

import (
	"strings"
	"testing"
)

const (
	cldDiceBloquean = "los hallazgos que bloquean"
	cldDiceAbiertos = "los hallazgos abiertos"
)

func cldDescDelRefutador(t *testing.T, ca, desc string) {
	t.Helper()
	low := strings.ToLower(desc)
	if !strings.Contains(low, cldDiceBloquean) {
		t.Fatalf("%s: la descripcion del refutador dice %q: %q", ca, cldDiceBloquean, desc)
	}
	if strings.Contains(low, cldDiceAbiertos) {
		t.Fatalf("%s: la descripcion del refutador ya no dice %q: %q", ca, cldDiceAbiertos, desc)
	}
}

// CA-758: en la tabla de Roles hay un solo refutador y su Desc dice «los
// hallazgos que bloquean», nunca «los hallazgos abiertos».
func TestCA758_LaDescripcionDelRefutadorDiceLosQueBloquean(t *testing.T) {
	var refutadores []Role
	for _, r := range Roles() {
		if r.Slug == "refutador" {
			refutadores = append(refutadores, r)
		}
	}
	if len(refutadores) != 1 {
		t.Fatalf("CA-758: la tabla tiene un refutador, tiene %d", len(refutadores))
	}
	cldDescDelRefutador(t, "CA-758", refutadores[0].Desc)
}

// CA-758: Lookup da la misma descripcion por el slug (en cualquier caja) y
// por el nombre nativo: no hay una segunda descripcion que diga otra cosa.
func TestCA758_LookupDaLaMismaDescripcion(t *testing.T) {
	r, err := Lookup("refutador")
	if err != nil {
		t.Fatalf("CA-758: Lookup(refutador): %v", err)
	}
	cldDescDelRefutador(t, "CA-758", r.Desc)
	for _, nombre := range []string{"REFUTADOR", "Refutador", r.Native} {
		otro, err := Lookup(nombre)
		if err != nil || otro.Desc != r.Desc || otro.Slug != "refutador" {
			t.Fatalf("CA-758: Lookup(%q) es el mismo refutador con la misma descripcion: %+v %v", nombre, otro, err)
		}
	}
}
