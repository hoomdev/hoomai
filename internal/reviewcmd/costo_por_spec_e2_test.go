// Tests adversariales de la enmienda 2 del spec .hoom/specs/costo-por-spec.md
// (CA-549) sobre `hoom review`: una review --spec sin --task corre en la
// raiz, asi que el run de cada lente no tiene task; hoom ata sus hallazgos a
// la tarea del spec (HOOM_TASK), y esa tarea queda en el sidecar del run como
// finding_task. Si la review termina NO ENTREGABLE no deja registro, y el
// sidecar es lo unico que la ata a la tarea. Con --task el run tiene task y
// no guarda finding_task; sin --task ni --spec no hay tarea que guardar.
// Escritos DESDE EL SPEC y las firmas publicas (Options, Result, Pass,
// runcmd.Meta.FindingTask), sin leer la implementacion.
//
// Los CLIs de IA falsos salen de fakeProvider (cliFalso). Anotan el
// HOOM_TASK que vieron FUERA del arbol revisado. CODEX_HOME apunta a un
// directorio vacio propio.
package reviewcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const (
	cfSlug = "costo-e2e"
	cfSpec = ".hoom/specs/" + cfSlug + ".md"
)

// cfCodexQueFalla pone un codex falso que anota su HOOM_TASK en anot (fuera
// del arbol), informa consumo y sale con 3: el run del reviewer falla.
func cfCodexQueFalla(t *testing.T) (anot string) {
	t.Helper()
	anot = t.TempDir()
	fakeProvider(t, "codex",
		"printf '%s' \"$HOOM_TASK\" > '"+anot+"/task.'$$\n"+
			"printf '{\"type\":\"turn.completed\",\"usage\":{\"input_tokens\":1500,\"cached_input_tokens\":500,\"output_tokens\":15}}\\n'\n"+
			"echo fallando >&2\nexit 3\n")
	return anot
}

// cfVistos devuelve los HOOM_TASK que vio cada invocacion del falso.
func cfVistos(t *testing.T, anot string) []string {
	t.Helper()
	entradas, err := os.ReadDir(anot)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entradas {
		raw, err := os.ReadFile(filepath.Join(anot, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(raw))
	}
	return out
}

// cfSidecar devuelve el sidecar crudo del run, busque donde lo busque.
func cfSidecar(t *testing.T, id string, dirs ...string) map[string]any {
	t.Helper()
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(d, ".hoom", "runs", id+".meta.json"))
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("CA-549: el sidecar %s es JSON: %v\n%s", id, err, raw)
		}
		return m
	}
	t.Fatalf("CA-549: el sidecar del run %s no esta en %v", id, dirs)
	return nil
}

// cfSpecCommiteado deja el spec de la tarea en el arbol, commiteado.
func cfSpecCommiteado(t *testing.T, dir string) {
	t.Helper()
	write(t, dir, cfSpec, "# Spec: "+cfSlug+"\n\n## Criterios de aceptación\n\n- CA-1: algo.\n")
	git(t, dir, "add", "-f", "--", cfSpec)
	git(t, dir, "commit", "-q", "-m", "spec de "+cfSlug, "--", cfSpec)
}

// CA-549 (caso limite "Ronda NO ENTREGABLE"): una review --spec sin --task
// cuyo reviewer falla termina NO ENTREGABLE y sin registro; el run de su
// lente no tiene task y guarda finding_task con la tarea del spec, la misma
// que el provider vio en HOOM_TASK (no la "ajena" del humano).
func TestCA549_ReviewSpecSinTaskNoEntregableGuardaFindingTask(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	root := repo(t)
	cfSpecCommiteado(t, root)
	anot := cfCodexQueFalla(t)
	t.Setenv("HOOM_TASK", "ajena")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Provider: "codex", Spec: cfSpec}, &out)
	if err != nil {
		t.Fatalf("CA-549: la review no se rompe: %v\n%s", err, out.String())
	}
	if res.Status != "no-entregable" || res.ExitCode == 0 || res.RecordID != "" {
		t.Fatalf("CA-549: fixture: el reviewer falla y la review es NO ENTREGABLE, sin registro: %+v\n%s", res, out.String())
	}
	if regs := rcRegistros(t, root); len(regs) != 0 {
		t.Fatalf("CA-549: fixture: una review NO ENTREGABLE no deja registro: %v", regs)
	}
	if !strings.Contains(strings.ToUpper(out.String()), "NO ENTREGABLE") {
		t.Fatalf("CA-549: fixture: la salida dice NO ENTREGABLE:\n%s", out.String())
	}
	vistos := cfVistos(t, anot)
	if len(vistos) == 0 || len(res.Passes) == 0 {
		t.Fatalf("CA-549: fixture: el reviewer corrio: %+v %v", res, vistos)
	}
	for _, v := range vistos {
		if v != cfSlug {
			t.Fatalf("CA-549: fixture: el reviewer ve HOOM_TASK=%s (la tarea del spec): %q", cfSlug, vistos)
		}
	}
	for _, p := range res.Passes {
		if p.RunID == "" {
			t.Fatalf("CA-549: la pasada %s nombra su run aunque haya fallado: %+v", p.Lens, p)
		}
		crudo := cfSidecar(t, p.RunID, root)
		if crudo["finding_task"] != cfSlug {
			t.Fatalf("CA-549: el run %s de la review --spec sin --task guarda finding_task=%s: %v", p.RunID, cfSlug, crudo)
		}
		if _, hay := crudo["task"]; hay {
			t.Fatalf("CA-549: el run corre en la raiz: no tiene task: %v", crudo)
		}
		if crudo["role"] != "reviewer" {
			t.Fatalf("CA-549: fixture: el run es del reviewer: %v", crudo)
		}
		meta, ok := runcmd.ReadMeta(root, p.RunID)
		if !ok || meta.FindingTask != cfSlug || meta.Task != "" {
			t.Fatalf("CA-549: ReadMeta lee finding_task=%s y task vacia: %+v %v", cfSlug, meta, ok)
		}
	}
}

// CA-549: una review con --task corre en el worktree de la tarea: su run
// tiene task y no guarda finding_task. Una review sin --task ni --spec no
// tiene tarea: su run no guarda finding_task aunque el humano tenga HOOM_TASK.
// Hoy pasa porque nada llena finding_task; fija que el cambio no la escriba
// de mas.
func TestCA549_ReviewConTaskOSinTareaNoGuardaFindingTask(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("HOOM_TASK", "ajena")

	// con --task (y --spec)
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	if err := taskcmd.Start(root, cfSlug, "main"); err != nil {
		t.Fatal(err)
	}
	dir, err := runcmd.TaskDir(root, cfSlug)
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "app.go", "package app\n\nfunc Nuevo() {}\n")
	raCommit(t, dir, "cambio en la tarea")
	cfCodexQueFalla(t)
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Provider: "codex", Task: cfSlug, Spec: cfSpec}, &out)
	if err != nil {
		t.Fatalf("CA-549: %v\n%s", err, out.String())
	}
	if len(res.Passes) == 0 {
		t.Fatalf("CA-549: fixture: el reviewer corrio: %+v\n%s", res, out.String())
	}
	for _, p := range res.Passes {
		crudo := cfSidecar(t, p.RunID, root, dir)
		if _, hay := crudo["finding_task"]; hay {
			t.Fatalf("CA-549: con --task el run tiene task y no guarda finding_task: %v", crudo)
		}
		if crudo["task"] != cfSlug {
			t.Fatalf("CA-549: fixture: con --task el run tiene task %s: %v", cfSlug, crudo)
		}
	}

	// sin --task ni --spec
	sola := repo(t)
	anot := cfCodexQueFalla(t)
	out.Reset()
	res, err = Run(sola, "main", Options{Lens: "risk", Provider: "codex"}, &out)
	if err != nil {
		t.Fatalf("CA-549: %v\n%s", err, out.String())
	}
	if len(res.Passes) == 0 {
		t.Fatalf("CA-549: fixture: el reviewer corrio: %+v\n%s", res, out.String())
	}
	for _, v := range cfVistos(t, anot) {
		if v != "" {
			t.Fatalf("CA-549: fixture: sin tarea el reviewer ve HOOM_TASK vacia: %q", v)
		}
	}
	for _, p := range res.Passes {
		if crudo := cfSidecar(t, p.RunID, sola); crudo["finding_task"] != nil {
			t.Fatalf("CA-549: sin --task ni --spec el run no guarda finding_task: %v", crudo)
		}
	}
}
