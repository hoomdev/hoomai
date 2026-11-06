// Tests adversariales de la ronda 1 de la review de verbos-de-la-tarjeta en
// la funcion de lanzamiento compartida: hallazgo faff7c (CA-503, CA-504).
// Lo que una piel muestra es lo que se lanza: Request.Vista es lo que la
// accion proponia cuando la piel lo mostro (lanzar.PropuestaDe), y Lanzar,
// que vuelve a derivar la tarjeta bajo su candado, se rechaza con un 409 que
// nombra lo que cambio, en el orden de la linea valores:, sin efectos.
package lanzar_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
)

// r1Cambio es el motivo del contrato cuando la tarjeta cambio entre lo que
// se mostro y el lanzamiento.
func r1Cambio(lista string) string {
	return "la tarjeta cambio mientras escribias el pedido (" + lista + "): no se lanzo nada, vuelve a pedirlo"
}

// r1Claude deja un claude falso, que anota cada invocacion, al frente de un
// PATH minimo (ningun CLI de IA real entra). Devuelve el directorio de las
// anotaciones.
func r1Claude(t *testing.T) string {
	t.Helper()
	bin, marcas := t.TempDir(), t.TempDir()
	s := "#!/bin/sh\ncat > /dev/null\n: > '" + marcas + "/claude.'$$\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	return marcas
}

func r1Llamadas(t *testing.T, marcas string) int {
	t.Helper()
	ents, err := os.ReadDir(marcas)
	if err != nil {
		t.Fatal(err)
	}
	return len(ents)
}

// r1Accion deriva la tarjeta precios y devuelve su accion id.
func r1Accion(t *testing.T, root, id string) boardcmd.Action {
	t.Helper()
	c, err := boardcmd.CardFor(root, "main", "", "precios", time.Now().UTC())
	if err != nil {
		t.Fatalf("fixture: CardFor precios: %v", err)
	}
	for _, a := range c.Actions {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("fixture: la tarjeta precios (%s) no tiene %s: %+v", c.Column, id, c.Actions)
	return boardcmd.Action{}
}

func r1F(v float64) *float64 { return &v }

func r1JSON(t *testing.T, v any) (map[string]any, string) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m, string(raw)
}

// hallazgo faff7c: PropuestaDe es lo que la accion propone ahora: su
// provider default, su modelo, esfuerzo, presupuesto, tiempo limite y la
// sesion que retoma; en JSON con las seis claves del contrato.
func TestHallazgo_faff7c_PropuestaDeEsLoQueProponeLaAccion(t *testing.T) {
	a := boardcmd.Action{ID: boardcmd.ActReanudar, Role: "writer", Enabled: true, Model: "m-a", Effort: "e-a",
		TimeoutS: 2700, BudgetUSD: r1F(2), ResumeID: "ses-1",
		Providers: []boardcmd.ProviderOption{{Name: "claude", OK: true}, {Name: "codex", OK: true, Default: true}}}
	p := lanzar.PropuestaDe(a)
	if p.Provider != "codex" || p.Model != "m-a" || p.Effort != "e-a" || p.BudgetUSD == nil || *p.BudgetUSD != 2 ||
		p.TimeoutS != 2700 || p.ResumeID != "ses-1" {
		t.Fatalf("hallazgo faff7c: PropuestaDe trae el default (codex), m-a, e-a, 2, 2700 y ses-1: %+v", p)
	}
	m, _ := r1JSON(t, p)
	for _, k := range []string{"provider", "model", "effort", "budget_usd", "timeout_s", "resume_id"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("hallazgo faff7c: la propuesta en JSON trae %q: %v", k, m)
		}
	}
}

// hallazgo faff7c, CA-504: una Vista distinta de lo que la accion propone
// bajo el candado es un 409 que nombra lo que cambio, en el orden
// provider, modelo, esfuerzo, presupuesto, tiempo limite, sesion; nada se
// lanza (ni la tarea de Backlog, ni un sobre, ni el CLI).
func TestHallazgo_faff7c_CA504_VistaDistintaSeRechazaSinEfectos(t *testing.T) {
	marcas := r1Claude(t)
	root, m := vdtProyecto(t)
	a := r1Accion(t, root, boardcmd.ActPedirArquitecto)
	if !a.Enabled {
		t.Fatalf("fixture: pedir-arquitecto habilitada en Backlog: %+v", a)
	}
	base := lanzar.PropuestaDe(a)
	if base.Provider != "claude" {
		t.Fatalf("fixture: el provider que propone la accion es el claude falso: %+v", base)
	}
	cambia := map[string]func(p *lanzar.Propuesta){
		"provider":      func(p *lanzar.Propuesta) { p.Provider = "codex" },
		"modelo":        func(p *lanzar.Propuesta) { p.Model = p.Model + "-otro" },
		"esfuerzo":      func(p *lanzar.Propuesta) { p.Effort = p.Effort + "-otro" },
		"presupuesto":   func(p *lanzar.Propuesta) { p.BudgetUSD = r1F(97.5) },
		"tiempo limite": func(p *lanzar.Propuesta) { p.TimeoutS = p.TimeoutS + 60 },
		"sesion":        func(p *lanzar.Propuesta) { p.ResumeID = p.ResumeID + "otra-sesion" },
	}
	orden := []string{"provider", "modelo", "esfuerzo", "presupuesto", "tiempo limite", "sesion"}
	casos := [][]string{{"provider"}, {"modelo"}, {"esfuerzo"}, {"presupuesto"}, {"tiempo limite"}, {"sesion"},
		{"modelo", "tiempo limite"}, {"presupuesto", "sesion"}, orden}
	for _, campos := range casos {
		vista := base
		// los cambios se aplican al reves del orden: el motivo los nombra igual en el orden del contrato
		for i := len(campos) - 1; i >= 0; i-- {
			cambia[campos[i]](&vista)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, "precios",
			lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: "Escribi el spec de precios por region.", Vista: &vista})
		cancel()
		var r *lanzar.Rechazo
		if !errors.As(err, &r) {
			if tr != nil {
				tr.Wait()
			}
			t.Fatalf("hallazgo faff7c (%v): una vista distinta es un *Rechazo, fue %v (trabajo %+v)", campos, err, tr)
		}
		want := r1Cambio(strings.Join(campos, ", "))
		if r.Code != 409 || r.Motivo != want {
			t.Fatalf("hallazgo faff7c (%v): el rechazo es 409 %q, fue %d %q", campos, want, r.Code, r.Motivo)
		}
		if tr != nil {
			t.Fatalf("hallazgo faff7c (%v): un rechazo antes de lanzar no devuelve trabajo: %+v", campos, tr)
		}
		if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "precios")); !os.IsNotExist(err) {
			t.Fatalf("hallazgo faff7c (%v): no se lanzo nada: tampoco la tarea de Backlog: %v", campos, err)
		}
		if ents, _ := os.ReadDir(filepath.Join(root, ".hoom", "envelopes")); len(ents) != 0 {
			t.Fatalf("hallazgo faff7c (%v): no se lanzo nada: ningun sobre, hay %d", campos, len(ents))
		}
		if n := r1Llamadas(t, marcas); n != 0 {
			t.Fatalf("hallazgo faff7c (%v): no se lanzo nada: el CLI corrio %d veces", campos, n)
		}
	}
}

// hallazgo faff7c: una Vista igual a lo que la accion propone (el
// presupuesto comparado por valor, no por puntero) lanza; Vista nil (la
// cinta) no se compara.
func TestHallazgo_faff7c_VistaIgualLanza(t *testing.T) {
	marcas := r1Claude(t)
	root, m := vdtProyecto(t)
	a := r1Accion(t, root, boardcmd.ActPedirArquitecto)
	vista := lanzar.PropuestaDe(a)
	if vista.BudgetUSD != nil {
		vista.BudgetUSD = r1F(*vista.BudgetUSD)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, "precios",
		lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: "Escribi el spec de precios por region.", Vista: &vista})
	if tr != nil {
		defer tr.Wait()
	}
	var r *lanzar.Rechazo
	if errors.As(err, &r) && strings.HasPrefix(r.Motivo, "la tarjeta cambio mientras escribias el pedido") {
		t.Fatalf("hallazgo faff7c: lo que se mostro es lo que la accion propone: se lanza, y fue %d %q", r.Code, r.Motivo)
	}
	if err != nil || tr == nil {
		t.Fatalf("hallazgo faff7c: con la vista igual el lanzamiento arranca: %v %+v", err, tr)
	}
	tr.Wait()
	if n := r1Llamadas(t, marcas); n != 1 {
		t.Fatalf("hallazgo faff7c: con la vista igual el arquitecto corre una vez, fueron %d", n)
	}
}
