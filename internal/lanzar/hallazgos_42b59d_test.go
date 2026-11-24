// Regresion del hallazgo 42b59d (ronda 1 de proporcion-por-riesgo):
// Propuesta trae el nivel del modelo (clave JSON "nivel"), PropuestaDe lo
// llena con el de la accion, y una Vista con otro nivel es el 409 de faff7c
// que nombra el modelo, sin efectos: el mismo modelo desde un nivel o desde
// un nombre literal se traduce distinto con otro provider.
//
// El nivel se lee y se escribe por su clave JSON: el archivo compila tambien
// contra una Propuesta sin el campo y el test falla por lo que hace.
package lanzar_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
)

func TestHallazgo_42b59d_PropuestaDeTraeElNivel(t *testing.T) {
	a := boardcmd.Action{ID: boardcmd.ActPedirArquitecto, Role: "arquitecto", Enabled: true, Model: "gpt-6-astra",
		Nivel: "fuerte", TimeoutS: 1800, Providers: []boardcmd.ProviderOption{{Name: "codex", OK: true, Default: true}}}
	if m, raw := r1JSON(t, lanzar.PropuestaDe(a)); m["nivel"] != "fuerte" || m["model"] != "gpt-6-astra" {
		t.Fatalf("hallazgo 42b59d: PropuestaDe trae el modelo y su nivel (\"nivel\": \"fuerte\"): %s", raw)
	}
}

func TestHallazgo_42b59d_VistaConOtroNivelSeRechaza(t *testing.T) {
	marcas := r1Claude(t)
	root, m := vdtProyecto(t)
	a := r1Accion(t, root, boardcmd.ActPedirArquitecto)
	if a.Nivel != "" {
		t.Fatalf("fixture: el arquitecto no propone un nivel: %+v", a)
	}
	// la vista mostro el mismo modelo, pero desde el nivel fuerte
	p, _ := r1JSON(t, lanzar.PropuestaDe(a))
	p["nivel"] = "fuerte"
	raw, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var vista lanzar.Propuesta
	if err := json.Unmarshal(raw, &vista); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, "precios",
		lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: "Escribi el spec de precios por region.", Vista: &vista})
	if tr != nil {
		tr.Wait()
	}
	var r *lanzar.Rechazo
	if !errors.As(err, &r) || r.Code != 409 || r.Motivo != r1Cambio("modelo") {
		t.Fatalf("hallazgo 42b59d: otro nivel es otro modelo: 409 %q, fue %v", r1Cambio("modelo"), err)
	}
	r2SinEfectos(t, "hallazgo 42b59d", root, marcas)
}
