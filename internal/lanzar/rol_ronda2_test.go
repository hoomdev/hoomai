// Tests adversariales de la ronda 2 de la review de verbos-de-la-tarjeta en
// la funcion de lanzamiento compartida: hallazgo a8e103 (CA-503, CA-504).
// Lo que una piel muestra incluye el ROL que la accion lanza: reanudar y
// relanzar lo toman de la interrupcion de la tarjeta, que puede ser otra
// cuando se termina de escribir el pedido. Propuesta trae el rol (clave JSON
// "role"), PropuestaDe lo llena con el de la accion, y una Vista con otro rol
// es el 409 de faff7c con `rol` primero en la lista, sin efectos.
//
// El rol se lee y se escribe por su clave JSON, la del contrato: asi el
// archivo compila tambien contra una Propuesta sin el campo y el test falla
// por lo que hace, no por compilar.
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

// r2ConRol es la propuesta p con el rol rol, puesto por la clave JSON
// "role" del contrato.
func r2ConRol(t *testing.T, p lanzar.Propuesta, rol string) lanzar.Propuesta {
	t.Helper()
	m, _ := r1JSON(t, p)
	m["role"] = rol
	raw, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var out lanzar.Propuesta
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// hallazgo a8e103: PropuestaDe trae el rol que la accion lanza, en la
// clave JSON "role" (la septima del contrato, junto a las seis de faff7c).
func TestHallazgo_a8e103_PropuestaDeTraeElRol(t *testing.T) {
	for _, a := range []boardcmd.Action{
		{ID: boardcmd.ActReanudar, Role: "test-writer", Enabled: true, TimeoutS: 1800, ResumeID: "ses-1",
			Providers: []boardcmd.ProviderOption{{Name: "claude", OK: true, Default: true}}},
		{ID: boardcmd.ActRelanzar, Role: "writer", Enabled: true, TimeoutS: 2700, BudgetUSD: r1F(2),
			Providers: []boardcmd.ProviderOption{{Name: "claude", OK: true, Default: true}}},
		{ID: boardcmd.ActPedirArquitecto, Role: "arquitecto", Enabled: true, TimeoutS: 1800,
			Providers: []boardcmd.ProviderOption{{Name: "codex", OK: true, Default: true}}},
	} {
		m, raw := r1JSON(t, lanzar.PropuestaDe(a))
		if m["role"] != a.Role {
			t.Fatalf("hallazgo a8e103: PropuestaDe(%s) trae \"role\": %q, trae %s", a.ID, a.Role, raw)
		}
		for _, k := range []string{"provider", "model", "effort", "budget_usd", "timeout_s", "resume_id"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("hallazgo a8e103: la propuesta sigue trayendo %q: %s", k, raw)
			}
		}
	}
}

// r2SinEfectos: nada se lanzo, ni la tarea de Backlog, ni un sobre, ni el
// CLI.
func r2SinEfectos(t *testing.T, ca, root, marcas string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "precios")); !os.IsNotExist(err) {
		t.Fatalf("%s: no se lanzo nada: tampoco la tarea de Backlog: %v", ca, err)
	}
	if ents, _ := os.ReadDir(filepath.Join(root, ".hoom", "envelopes")); len(ents) != 0 {
		t.Fatalf("%s: no se lanzo nada: ningun sobre, hay %d", ca, len(ents))
	}
	if n := r1Llamadas(t, marcas); n != 0 {
		t.Fatalf("%s: no se lanzo nada: el CLI corrio %d veces", ca, n)
	}
}

// hallazgo a8e103, CA-504: una Vista igual a PropuestaDe(a) salvo el rol es
// el 409 de faff7c con `rol` en la lista, primero y antes que el orden de la
// linea valores: (provider, modelo, esfuerzo, presupuesto, tiempo limite,
// sesion); una Vista sin rol tambien es distinta. Nada se lanza.
func TestHallazgo_a8e103_CA504_VistaConOtroRolSeRechazaSinEfectos(t *testing.T) {
	marcas := r1Claude(t)
	root, m := vdtProyecto(t)
	a := r1Accion(t, root, boardcmd.ActPedirArquitecto)
	if !a.Enabled || a.Role != "arquitecto" {
		t.Fatalf("fixture: pedir-arquitecto habilitada en Backlog, con rol arquitecto: %+v", a)
	}
	base := lanzar.PropuestaDe(a)
	casos := []struct {
		nombre string
		vista  func() lanzar.Propuesta
		lista  string
	}{
		{"otro rol", func() lanzar.Propuesta { return r2ConRol(t, base, "test-writer") }, "rol"},
		{"el writer", func() lanzar.Propuesta { return r2ConRol(t, base, "writer") }, "rol"},
		{"rol vacio (una piel que no lo manda)", func() lanzar.Propuesta { return r2ConRol(t, base, "") }, "rol"},
		{"el rol con otra mayuscula", func() lanzar.Propuesta { return r2ConRol(t, base, "Arquitecto") }, "rol"},
		{"rol y modelo", func() lanzar.Propuesta {
			v := r2ConRol(t, base, "reviewer")
			v.Model += "-otro"
			return v
		}, "rol, modelo"},
		{"sesion y rol", func() lanzar.Propuesta {
			v := base
			v.ResumeID += "otra-sesion"
			return r2ConRol(t, v, "writer")
		}, "rol, sesion"},
		{"los siete", func() lanzar.Propuesta {
			v := base
			v.Provider = "codex"
			v.Model += "-otro"
			v.Effort += "-otro"
			v.BudgetUSD = r1F(97.5)
			v.TimeoutS += 60
			v.ResumeID += "otra-sesion"
			return r2ConRol(t, v, "test-writer")
		}, "rol, provider, modelo, esfuerzo, presupuesto, tiempo limite, sesion"},
	}
	for _, c := range casos {
		vista := c.vista()
		ca := "hallazgo a8e103, CA-504 (" + c.nombre + ")"
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, "precios",
			lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: "Escribi el spec de precios por region.", Vista: &vista})
		cancel()
		var r *lanzar.Rechazo
		if !errors.As(err, &r) {
			if tr != nil {
				tr.Wait()
			}
			t.Fatalf("%s: una vista con otro rol es un *Rechazo, fue %v (trabajo %+v)", ca, err, tr)
		}
		if want := r1Cambio(c.lista); r.Code != 409 || r.Motivo != want {
			t.Fatalf("%s: el rechazo es 409\n  %q\nfue %d\n  %q", ca, want, r.Code, r.Motivo)
		}
		if tr != nil {
			t.Fatalf("%s: un rechazo antes de lanzar no devuelve trabajo: %+v", ca, tr)
		}
		r2SinEfectos(t, ca, root, marcas)
	}
}

// hallazgo a8e103: una Vista que trae el rol de la accion (y lo demas igual)
// lanza: el rol no rechaza de mas.
func TestHallazgo_a8e103_VistaConElRolDeLaAccionLanza(t *testing.T) {
	marcas := r1Claude(t)
	root, m := vdtProyecto(t)
	a := r1Accion(t, root, boardcmd.ActPedirArquitecto)
	vista := r2ConRol(t, lanzar.PropuestaDe(a), "arquitecto")
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, "precios",
		lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: "Escribi el spec de precios por region.", Vista: &vista})
	if tr != nil {
		defer tr.Wait()
	}
	var r *lanzar.Rechazo
	if errors.As(err, &r) && strings.HasPrefix(r.Motivo, "la tarjeta cambio mientras escribias el pedido") {
		t.Fatalf("hallazgo a8e103: la vista trae el rol de la accion: se lanza, y fue %d %q", r.Code, r.Motivo)
	}
	if err != nil || tr == nil {
		t.Fatalf("hallazgo a8e103: con la vista igual (rol incluido) el lanzamiento arranca: %v %+v", err, tr)
	}
	tr.Wait()
	if n := r1Llamadas(t, marcas); n != 1 {
		t.Fatalf("hallazgo a8e103: con la vista igual el arquitecto corre una vez, fueron %d", n)
	}
}
