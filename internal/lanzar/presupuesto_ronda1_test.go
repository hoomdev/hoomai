// Tests adversariales de la ronda 1 de la review de verbos-de-la-tarjeta en
// la funcion de lanzamiento compartida: hallazgo 896eff (CA-504). Un
// presupuesto que sale de hoom.yaml por debajo del piso del provider se
// rechaza igual que uno pedido, en Lanzar (que usan el Studio y la terminal)
// y en Comprobar (lo que la terminal mira antes de abrir el editor), sin
// efectos. Este archivo usa solo la API que ya existia antes de la ronda 1,
// para poder correrlo contra el codigo de antes.
package lanzar_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/manifest"
)

const p1Piso = "claude necesita al menos 0.5 USD por run"

// p1Claude deja un claude falso, que anota cada invocacion, al frente de un
// PATH minimo. Devuelve el directorio de las anotaciones.
func p1Claude(t *testing.T) string {
	t.Helper()
	bin, marcas := t.TempDir(), t.TempDir()
	s := "#!/bin/sh\ncat > /dev/null\n: > '" + marcas + "/claude.'$$\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	return marcas
}

// p1ConAgents es vdtProyecto con agents: en el hoom.yaml del proyecto.
func p1ConAgents(t *testing.T, agents string) (string, *manifest.Manifest) {
	t.Helper()
	root, _ := vdtProyecto(t)
	raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	vdtEscribir(t, root, "hoom.yaml", string(raw)+agents)
	vdtGit(t, root, "add", "-A")
	vdtGit(t, root, "commit", "-q", "-m", "agents")
	m, err := manifest.Load(root, nil)
	if err != nil {
		t.Fatalf("fixture: hoom.yaml: %v", err)
	}
	return root, m
}

// hallazgo 896eff, CA-504: agents.arquitecto.budget_usd: 0.1 (bajo el piso
// de claude) se rechaza en Lanzar con un 409 y el motivo del piso, como un
// --budget-usd 0.1 pedido; nada se lanza (ni la tarea de Backlog, ni un
// sobre, ni el CLI). Comprobar da el mismo motivo antes de pedir el pedido.
func TestHallazgo_896eff_CA504_LanzarRechazaElPresupuestoDeHoomYamlBajoElPiso(t *testing.T) {
	marcas := p1Claude(t)
	root, m := p1ConAgents(t, "agents:\n  arquitecto:\n    budget_usd: 0.1\n")
	c, err := boardcmd.CardFor(root, "main", "", "precios", time.Now().UTC())
	if err != nil {
		t.Fatalf("fixture: CardFor: %v", err)
	}
	a, ok := c.Action(boardcmd.ActPedirArquitecto)
	if !ok || a.BudgetUSD == nil || *a.BudgetUSD != 0.1 {
		t.Fatalf("hallazgo 896eff: fixture: pedir-arquitecto propone el presupuesto de hoom.yaml (0.1): %+v", a)
	}
	req := lanzar.Request{Action: boardcmd.ActPedirArquitecto, Pedido: "Escribi el spec de precios por region."}
	if err := lanzar.Comprobar(c, a, req); err == nil || !strings.Contains(err.Error(), p1Piso) {
		t.Fatalf("hallazgo 896eff, CA-504: Comprobar rechaza el presupuesto de hoom.yaml bajo el piso con %q: %v", p1Piso, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, "precios", req)
	if tr != nil {
		tr.Wait()
	}
	var r *lanzar.Rechazo
	if !errors.As(err, &r) {
		t.Fatalf("hallazgo 896eff, CA-504: el presupuesto de hoom.yaml bajo el piso es un *Rechazo, fue %v (trabajo %+v)", err, tr)
	}
	if r.Code != 409 || !strings.Contains(r.Motivo, p1Piso) {
		t.Fatalf("hallazgo 896eff, CA-504: el rechazo es 409 con %q: %d %q", p1Piso, r.Code, r.Motivo)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "precios")); !os.IsNotExist(err) {
		t.Fatalf("hallazgo 896eff, CA-504: no se lanzo nada: tampoco la tarea de Backlog: %v", err)
	}
	if ents, _ := os.ReadDir(filepath.Join(root, ".hoom", "envelopes")); len(ents) != 0 {
		t.Fatalf("hallazgo 896eff, CA-504: no se lanzo nada: ningun sobre, hay %d", len(ents))
	}
	if ents, _ := os.ReadDir(marcas); len(ents) != 0 {
		t.Fatalf("hallazgo 896eff, CA-504: no se lanzo nada: claude corrio %d veces", len(ents))
	}

	// el mismo presupuesto pedido da el mismo motivo
	pedido := 0.1
	root2, m2 := vdtProyecto(t)
	tr, err = lanzar.Lanzar(ctx, lanzar.Params{Root: root2, Manifest: m2}, "precios",
		lanzar.Request{Action: boardcmd.ActPedirArquitecto, BudgetUSD: &pedido, Pedido: "Escribi el spec de precios por region."})
	if tr != nil {
		tr.Wait()
	}
	var r2 *lanzar.Rechazo
	if !errors.As(err, &r2) || r2.Motivo != r.Motivo || r2.Code != r.Code {
		t.Fatalf("hallazgo 896eff, CA-504: el de hoom.yaml (%q) se rechaza como el pedido: %v %+v", r.Motivo, err, r2)
	}
}
