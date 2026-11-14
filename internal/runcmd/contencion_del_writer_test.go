// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre
// runcmd (CA-674): con StartOptions.Encierro el sidecar del run lleva
// `encierro` con la Info del plan; sin plan, no lleva la clave. El plan de
// nivel ninguno deja el comando como esta (Envolver), asi que corre en
// cualquier sistema.
package runcmd

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/encierro"
)

func cdwInfoNinguno() encierro.Info {
	return encierro.Info{Nivel: encierro.NivelNinguno,
		Falta: []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC}, Motivo: encierro.MotivoSinTarea}
}

// CA-674: el sidecar lleva la Info del plan, con falta como lista y motivo;
// Meta.Encierro la lee.
func TestCA674_ElSidecarLlevaElEncierroDelPlan(t *testing.T) {
	const ca = "CA-674"
	installFake(t, "echo hecho\nexit 0\n")
	root := initRepo(t)
	m := NewManager(root)
	info := cdwInfoNinguno()
	r, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Encierro: &encierro.Plan{Info: info}})
	if err != nil {
		t.Fatal(err)
	}
	fin := waitRun(t, m, r.ID)
	if fin.Status != StatusDone {
		t.Fatalf("%s: el run con un plan de nivel ninguno corre como hoy: %+v", ca, fin)
	}
	crudo := cpsCrudo(t, root, r.ID)
	e, ok := crudo["encierro"].(map[string]any)
	if !ok || e["nivel"] != encierro.NivelNinguno || e["motivo"] != encierro.MotivoSinTarea {
		t.Fatalf("%s: el sidecar lleva encierro con nivel y motivo: %v", ca, crudo["encierro"])
	}
	falta, ok := e["falta"].([]any)
	if !ok || len(falta) != 3 || falta[0] != encierro.FaltaProcesos || falta[2] != encierro.FaltaIPC {
		t.Fatalf("%s: falta es la lista del plan: %v", ca, e["falta"])
	}
	meta := cpsMetaCerrado(t, root, r.ID)
	if meta.Encierro == nil || meta.Encierro.Nivel != encierro.NivelNinguno || meta.Encierro.Motivo != encierro.MotivoSinTarea {
		t.Fatalf("%s: Meta.Encierro es la Info del plan: %+v", ca, meta.Encierro)
	}
}

// CA-674: sin plan (como `hoom run`), el sidecar no lleva la clave.
func TestCA674_SinPlanElSidecarNoLlevaLaClave(t *testing.T) {
	const ca = "CA-674"
	installFake(t, "exit 0\n")
	root := initRepo(t)
	m := NewManager(root)
	r, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, r.ID)
	if _, hay := cpsCrudo(t, root, r.ID)["encierro"]; hay {
		t.Fatalf("%s: sin plan el sidecar no lleva encierro", ca)
	}
	if meta := cpsMetaCerrado(t, root, r.ID); meta.Encierro != nil {
		t.Fatalf("%s: Meta.Encierro nil sin plan: %+v", ca, meta.Encierro)
	}
}
