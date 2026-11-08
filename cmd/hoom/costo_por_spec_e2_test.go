// Tests adversariales de la enmienda 2 del spec .hoom/specs/costo-por-spec.md
// (CA-549) de punta a punta, contra el BINARIO: una `hoom review --spec
// .hoom/specs/<slug>.md` sin --task cuyo reviewer falla termina NO
// ENTREGABLE y no deja registro; `hoom costo <slug>` igual suma su gasto como
// `reviewer (sin ronda)` de esa tarea, porque el run guardo finding_task.
// Escritos DESDE EL SPEC y el contrato publico de costcmd y de reviewcmd
// (Result en --json), sin leer la implementacion.
//
// El codex es falso (lee su stdin entero, informa consumo y sale con 3) y
// vive en un PATH minimo: ningun CLI de IA real entra. Compila el binario:
// se omite con -short.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cfCodexQueFalla pone al frente de un PATH minimo un codex falso que lee su
// stdin (el pedido de la review), informa entrada 1500, cache 500 y salida 15,
// y sale con 3.
func cfCodexQueFalla(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	s := "#!/bin/sh\ncat >/dev/null\n" +
		"printf '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1500,\"cached_input_tokens\":500,\"output_tokens\":15}}\\n'\n" +
		"echo fallando >&2\nexit 3\n"
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
}

func cfNum(v any) (float64, bool) {
	f, ok := v.(float64)
	return f, ok
}

// CA-549 (caso limite "Ronda NO ENTREGABLE"), de punta a punta.
func TestCA549_HoomReviewSpecSinTaskNoEntregableSumaComoReviewerSinRonda(t *testing.T) {
	bin := qcHoom(t)
	const slug = "costo-e2e"
	spec := ".hoom/specs/" + slug + ".md"
	dir := vdtRepo(t, "base_branch: main\n")
	qcGit(t, dir, "checkout", "-q", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "app.go"), []byte("package app\n\nfunc Nuevo() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".hoom", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, spec), []byte("# Spec: "+slug+"\n\n## Criterios de aceptación\n\n- CA-1: algo.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	qcGit(t, dir, "add", "-f", "--", "app.go", spec)
	qcGit(t, dir, "commit", "-q", "-m", "cambio y spec")
	cfCodexQueFalla(t)
	env := map[string]string{"HOOM_TASK": "ajena", "CODEX_HOME": t.TempDir()}

	// la review: NO ENTREGABLE, sin registro
	rv := qcCorrer(t, bin, dir, env, "review", "--spec", spec, "--lens", "risk", "--provider", "codex", "--json")
	var res struct {
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
		RecordID string `json:"record_id"`
		Passes   []struct {
			RunID string `json:"run_id"`
		} `json:"passes"`
	}
	if err := json.Unmarshal([]byte(rv.stdout), &res); err != nil {
		t.Fatalf("CA-549: fixture: hoom review --json imprime el resultado: %v\n%s", err, rv.todo())
	}
	if rv.exit == 0 || res.Status != "no-entregable" || res.RecordID != "" || len(res.Passes) == 0 {
		t.Fatalf("CA-549: fixture: el reviewer falla: la review es NO ENTREGABLE, sale con error y no deja registro: exit %d %+v\n%s",
			rv.exit, res, rv.todo())
	}
	if regs, _ := os.ReadDir(filepath.Join(dir, ".hoom", "reviews")); len(regs) != 0 {
		t.Fatalf("CA-549: fixture: la review NO ENTREGABLE no deja registro: %v", regs)
	}
	n := float64(len(res.Passes))

	// hoom costo <slug> --json: los runs de la review son de la tarea, en la
	// fila reviewer (sin ronda), con su gasto
	c := qcCorrer(t, bin, dir, nil, "costo", slug, "--json")
	if c.exit != 0 {
		t.Fatalf("CA-549: hoom costo %s sale con 0 (la tarea tiene los runs de su review): %d\n%s", slug, c.exit, c.todo())
	}
	var rep struct {
		Tareas []struct {
			Slug         string                    `json:"slug"`
			Rondas       float64                   `json:"rondas"`
			PorProvider  map[string]map[string]any `json:"por_provider"`
			PorRol       []map[string]any          `json:"por_rol"`
			RondasReview []map[string]any          `json:"rondas_review"`
		} `json:"tareas"`
	}
	if err := json.Unmarshal([]byte(c.stdout), &rep); err != nil {
		t.Fatalf("CA-549: hoom costo --json imprime JSON valido: %v\n%s", err, c.stdout)
	}
	if len(rep.Tareas) != 1 || rep.Tareas[0].Slug != slug {
		t.Fatalf("CA-549: hoom costo %s trae su tarea: %s", slug, c.stdout)
	}
	ta := rep.Tareas[0]
	if ta.Rondas != 0 || len(ta.RondasReview) != 1 {
		t.Fatalf("CA-549: sin registro no hay rondas, y hay una fila sin ronda: %s", c.stdout)
	}
	fila := ta.RondasReview[0]
	gasto, _ := fila["gasto"].(map[string]any)
	if fila["sin_ronda"] != true || fila["id"] != nil || gasto == nil {
		t.Fatalf("CA-549: la fila es reviewer (sin ronda), id null, con gasto: %v", fila)
	}
	if runs, _ := cfNum(gasto["runs"]); runs != n {
		t.Fatalf("CA-549: la fila sin ronda tiene los %v runs de la review: %v", n, gasto)
	}
	if tok, _ := gasto["tokens"].(map[string]any); tok == nil || tok["nuevos"] != 1000*n || tok["cache_leido"] != 500*n || tok["salida"] != 15*n {
		t.Fatalf("CA-549: la fila sin ronda suma el gasto de la review (nuevos %v, cache %v, salida %v): %v", 1000*n, 500*n, 15*n, gasto)
	}
	if g := ta.PorProvider["codex"]; g == nil || g["runs"] != n {
		t.Fatalf("CA-549: el total de codex cuenta los runs de la review: %v", ta.PorProvider)
	}
	rol := 0
	for _, gr := range ta.PorRol {
		if gr["rol"] == "reviewer" && gr["provider"] == "codex" && gr["runs"] == n {
			rol++
		}
	}
	if rol != 1 {
		t.Fatalf("CA-549: el rol reviewer de codex tiene los runs de la review: %v", ta.PorRol)
	}

	// el texto del desglose y la lista
	if s := qcCorrer(t, bin, dir, nil, "costo", slug); s.exit != 0 || !strings.Contains(s.stdout, "reviewer (sin ronda)") {
		t.Fatalf("CA-549: hoom costo %s muestra reviewer (sin ronda): %d\n%s", slug, s.exit, s.todo())
	}
	if s := qcCorrer(t, bin, dir, nil, "costo"); s.exit != 0 || !strings.Contains(s.stdout, slug) {
		t.Fatalf("CA-549: hoom costo lista la tarea %s: %d\n%s", slug, s.exit, s.todo())
	}
}
