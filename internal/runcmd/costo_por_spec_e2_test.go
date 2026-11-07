// Tests adversariales de la enmienda 2 del spec .hoom/specs/costo-por-spec.md
// (CA-549) sobre el run: un run sin `task` guarda en su sidecar
// `finding_task`, la tarea a la que hoom ata sus hallazgos (la que le pone al
// provider en HOOM_TASK); un run con `task`, o sin ninguna tarea, no guarda
// la clave. Escritos DESDE EL SPEC y las firmas publicas (StartOptions.Task,
// StartOptions.FindingTask, StartOptions.Dir, Meta.FindingTask, EnvTask), sin
// leer la implementacion.
//
// Lo que se mira es el JSON CRUDO del sidecar (.hoom/runs/<id>.meta.json):
// "se omite" quiere decir que la clave no esta, no que esta vacia. Los CLIs
// son falsos (installFake, installFakeNamed) y anotan el HOOM_TASK que vieron:
// finding_task tiene que ser esa misma tarea.
package runcmd

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fteEntorno arma un CLI falso `name` que anota "definida|valor" de
// HOOM_TASK en un archivo por invocacion y sale con exit. Devuelve el
// directorio de esas anotaciones.
func fteEntorno(t *testing.T, name string, exit int) string {
	t.Helper()
	dir := t.TempDir()
	s := "printf '%s|%s' \"${HOOM_TASK+definida}\" \"${HOOM_TASK}\" > '" + dir + "/entorno.'$$\n" +
		fmt.Sprintf("exit %d\n", exit)
	if name == "claude" {
		installFake(t, s)
	} else {
		installFakeNamed(t, name, s)
	}
	return dir
}

// fteVisto devuelve el HOOM_TASK que vio la unica invocacion anotada en dir
// y borra la anotacion (para la corrida siguiente).
func fteVisto(t *testing.T, ca, dir string) (definida bool, valor string) {
	t.Helper()
	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var anotadas []string
	for _, e := range entradas {
		if strings.HasPrefix(e.Name(), "entorno.") {
			anotadas = append(anotadas, filepath.Join(dir, e.Name()))
		}
	}
	if len(anotadas) != 1 {
		t.Fatalf("%s: fixture: una invocacion del provider, hubo %d", ca, len(anotadas))
	}
	raw, err := os.ReadFile(anotadas[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(anotadas[0]); err != nil {
		t.Fatal(err)
	}
	partes := strings.SplitN(string(raw), "|", 2)
	if len(partes) != 2 {
		t.Fatalf("%s: fixture: salida inesperada del provider falso: %q", ca, raw)
	}
	return partes[0] == "definida", partes[1]
}

// fteWorktree deja un worktree real de la tarea (la puerta de TaskDir no
// acepta una carpeta comun) en .hoom/worktrees/<task>, en la rama hoom/<task>.
func fteWorktree(t *testing.T, root, task string) string {
	t.Helper()
	dir := filepath.Join(root, ".hoom", "worktrees", task)
	git(t, root, "worktree", "add", "-q", "-b", "hoom/"+task, dir)
	return dir
}

// fteCrudoEn devuelve el sidecar crudo del run, este en la raiz o en dir.
func fteCrudoEn(t *testing.T, id string, dirs ...string) map[string]any {
	t.Helper()
	for _, d := range dirs {
		if _, err := os.Stat(filepath.Join(d, ".hoom", "runs", id+".meta.json")); err == nil {
			return cpsCrudo(t, d, id)
		}
	}
	t.Fatalf("CA-549: el sidecar del run %s no esta en %v", id, dirs)
	return nil
}

// fteFindingTask exige que el sidecar crudo tenga finding_task = quiero, o
// que no tenga la clave si quiero es "".
func fteFindingTask(t *testing.T, que string, crudo map[string]any, quiero string) {
	t.Helper()
	v, hay := crudo["finding_task"]
	if quiero == "" {
		if hay {
			t.Fatalf("CA-549: %s: el sidecar no tiene la clave finding_task (se omite), tiene %v: %v", que, v, crudo)
		}
		return
	}
	if !hay || v != quiero {
		t.Fatalf("CA-549: %s: el sidecar guarda finding_task=%q, tiene %v (presente=%v): %v", que, quiero, v, hay, crudo)
	}
}

// fteEsperar espera a que el run cierre y devuelve su sidecar ya cerrado.
func fteEsperar(t *testing.T, m *Manager, root, id string) (Run, Meta) {
	t.Helper()
	fin := waitRun(t, m, id)
	return fin, cpsMetaCerrado(t, root, id)
}

// ---------------------------------------------------------------- CA-549

// CA-549: un run sin task y con FindingTask guarda finding_task con esa
// tarea, que es la que el provider vio en HOOM_TASK (no la "ajena" del padre).
// No tiene task. Vale con cualquier provider y con el directorio implicito o
// explicito (una review --spec sin --task corre en la raiz).
func TestCA549_RunSinTaskGuardaFindingTask(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	for _, prov := range []string{"claude", "codex"} {
		for _, explicito := range []bool{false, true} {
			caso := fmt.Sprintf("%s, Dir explicito=%v", prov, explicito)
			anot := fteEntorno(t, prov, 0)
			t.Setenv(EnvTask, "ajena")
			root := initRepo(t)
			opts := StartOptions{Provider: prov, Prompt: "revisa", Role: "reviewer", FindingTask: "costo-por-spec"}
			if explicito {
				opts.Dir = root
			}
			m := NewManager(root)
			run, err := m.Start(opts)
			if err != nil {
				t.Fatalf("CA-549: %s: un run sin task con FindingTask arranca en la raiz: %v", caso, err)
			}
			_, meta := fteEsperar(t, m, root, run.ID)
			definida, visto := fteVisto(t, "CA-549", anot)
			if !definida || visto != "costo-por-spec" {
				t.Fatalf("CA-549: %s: fixture: el provider ve HOOM_TASK=costo-por-spec, vio definida=%v %q", caso, definida, visto)
			}
			crudo := cpsCrudo(t, root, run.ID)
			fteFindingTask(t, caso, crudo, visto)
			if meta.FindingTask != "costo-por-spec" {
				t.Fatalf("CA-549: %s: Meta.FindingTask es la tarea de HOOM_TASK: %q", caso, meta.FindingTask)
			}
			if _, hay := crudo["task"]; hay || meta.Task != "" {
				t.Fatalf("CA-549: %s: el run sigue sin task (task dice donde corre, y corre en la raiz): %v", caso, crudo)
			}
		}
	}
}

// CA-549: un run CON task no guarda finding_task, ni cuando la tarea de sus
// hallazgos es la misma ni cuando FindingTask nombra otra: task gana y la
// clave se omite.
// Hoy pasa porque nada llena finding_task; fija que el cambio no la escriba
// de mas.
func TestCA549_RunConTaskNoGuardaFindingTask(t *testing.T) {
	anot := fteEntorno(t, "claude", 0)
	t.Setenv(EnvTask, "ajena")
	root := initRepo(t)
	wt := fteWorktree(t, root, "cabina-visual")
	m := NewManager(root)
	for _, ft := range []string{"", "cabina-visual", "otra-tarea"} {
		caso := fmt.Sprintf("Task cabina-visual, FindingTask %q", ft)
		run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Role: "writer", Task: "cabina-visual", FindingTask: ft})
		if err != nil {
			t.Fatalf("CA-549: %s: %v", caso, err)
		}
		waitRun(t, m, run.ID)
		fteVisto(t, "CA-549", anot)
		crudo := fteCrudoEn(t, run.ID, root, wt)
		fteFindingTask(t, caso, crudo, "")
		if crudo["task"] != "cabina-visual" {
			t.Fatalf("CA-549: %s: fixture: el run tiene task: %v", caso, crudo)
		}
	}
}

// CA-549: un run sin ninguna tarea no guarda finding_task, aunque el proceso
// padre tenga un HOOM_TASK exportado: el run no hereda una tarea ajena, asi
// que no tiene tarea de hallazgos que guardar.
// Hoy pasa porque nada llena finding_task.
func TestCA549_RunSinNingunaTareaNoGuardaFindingTask(t *testing.T) {
	for _, padre := range []string{"ajena", ""} {
		anot := fteEntorno(t, "claude", 0)
		t.Setenv(EnvTask, padre)
		root := initRepo(t)
		m := NewManager(root)
		run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Role: "reviewer"})
		if err != nil {
			t.Fatal(err)
		}
		_, meta := fteEsperar(t, m, root, run.ID)
		if _, visto := fteVisto(t, "CA-549", anot); visto != "" {
			t.Fatalf("CA-549: fixture: sin tarea el provider ve HOOM_TASK vacia: %q", visto)
		}
		fteFindingTask(t, "sin tarea, HOOM_TASK del padre "+padre, cpsCrudo(t, root, run.ID), "")
		if meta.FindingTask != "" {
			t.Fatalf("CA-549: sin tarea Meta.FindingTask es vacio: %q", meta.FindingTask)
		}
	}
}

// CA-549 (caso limite "Ronda NO ENTREGABLE"): el costo se gasto igual, asi
// que finding_task queda en el sidecar de un run que falla y en el de uno
// cancelado: es lo unico que ata a la tarea una review que no termino.
func TestCA549_FindingTaskQuedaEnRunFallidoYCancelado(t *testing.T) {
	// falla: el provider sale con error
	anot := fteEntorno(t, "claude", 3)
	root := initRepo(t)
	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "revisa", Role: "reviewer", FindingTask: "tarea-cortada"})
	if err != nil {
		t.Fatal(err)
	}
	fin, meta := fteEsperar(t, m, root, run.ID)
	fteVisto(t, "CA-549", anot)
	if fin.Status != StatusError || meta.Status != StatusError {
		t.Fatalf("CA-549: fixture: el run del provider que sale con 3 termina en error: %+v %+v", fin, meta)
	}
	fteFindingTask(t, "run fallido", cpsCrudo(t, root, run.ID), "tarea-cortada")

	// cancelado: el provider no termina solo
	installFake(t, "echo arranco\nsleep 30\n")
	root2 := t.TempDir()
	m2 := NewManager(root2)
	run2, err := m2.Start(StartOptions{Provider: "claude", Prompt: "revisa", Role: "reviewer", FindingTask: "tarea-cortada"})
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if r, err := m2.Get(run2.ID); err != nil || r.Status != StatusRunning {
			break
		}
		m2.Cancel(run2.ID)
		time.Sleep(20 * time.Millisecond)
	}
	fin2, meta2 := fteEsperar(t, m2, root2, run2.ID)
	if fin2.Status != StatusCanceled || meta2.Status != StatusCanceled {
		t.Fatalf("CA-549: fixture: el run cancelado cierra canceled: %+v %+v", fin2, meta2)
	}
	fteFindingTask(t, "run cancelado", cpsCrudo(t, root2, run2.ID), "tarea-cortada")
}

// CA-549: finding_task ya esta en el sidecar mientras el run corre. El meta
// se escribe al arrancar (CA-158); si el proceso de hoom muere en medio de
// la review, el sidecar que queda es ese, y sin finding_task la review cortada
// no quedaria atada a ninguna tarea (decision "La tarea de un run sin
// worktree").
func TestCA549_FindingTaskEstaDesdeQueElRunArranca(t *testing.T) {
	sigue := filepath.Join(t.TempDir(), "sigue")
	installFake(t, "while [ ! -f '"+sigue+"' ]; do sleep 0.05; done\nexit 0\n")
	root := t.TempDir()
	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "revisa", Role: "reviewer", FindingTask: "en-vuelo"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.WriteFile(sigue, nil, 0o644)
		waitRun(t, m, run.ID)
	}()
	enCurso := leerMeta(t, root, run.ID)
	if enCurso.Status != StatusRunning {
		t.Fatalf("CA-549: fixture: el run sigue corriendo: %+v", enCurso)
	}
	fteFindingTask(t, "run en curso", cpsCrudo(t, root, run.ID), "en-vuelo")
}

// CA-549 (propiedad, semilla fija): para cualquier combinacion de Task (una
// de dos tareas con worktree, o ninguna), FindingTask (vacia, la misma task u
// otra) y HOOM_TASK del padre, el sidecar tiene finding_task si y solo si el
// run no tiene task y tiene tarea de hallazgos; y entonces es exactamente el
// HOOM_TASK que vio el provider. Nunca la del padre.
func TestCA549_PropiedadFindingTaskSoloSinTask(t *testing.T) {
	anot := fteEntorno(t, "claude", 0)
	root := initRepo(t)
	wts := map[string]string{"tarea-a": fteWorktree(t, root, "tarea-a"), "tarea-b": fteWorktree(t, root, "tarea-b")}
	m := NewManager(root)
	r := rand.New(rand.NewSource(549))
	tasks := []string{"", "tarea-a", "tarea-b"}
	fts := []string{"", "tarea-a", "tarea-b", "costo-por-spec", "x"}
	padres := []string{"", "ajena", "tarea-a"}
	for caso := 0; caso < 30; caso++ {
		task, ft, padre := tasks[r.Intn(len(tasks))], fts[r.Intn(len(fts))], padres[r.Intn(len(padres))]
		t.Setenv(EnvTask, padre)
		que := fmt.Sprintf("caso %d: Task %q, FindingTask %q, HOOM_TASK del padre %q", caso, task, ft, padre)
		run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Role: "reviewer", Task: task, FindingTask: ft})
		if err != nil {
			t.Fatalf("CA-549: %s: %v", que, err)
		}
		waitRun(t, m, run.ID)
		_, visto := fteVisto(t, "CA-549", anot)
		dirs := []string{root}
		if task != "" {
			dirs = append(dirs, wts[task])
		}
		crudo := fteCrudoEn(t, run.ID, dirs...)
		quiero := ""
		if task == "" && ft != "" {
			quiero = ft
			if visto != ft {
				t.Fatalf("CA-549: %s: fixture: el provider ve HOOM_TASK=%q, vio %q", que, ft, visto)
			}
		}
		fteFindingTask(t, que, crudo, quiero)
		if v, hay := crudo["finding_task"]; hay && v == padre && padre != ft {
			t.Fatalf("CA-549: %s: finding_task nunca es la tarea del proceso padre: %v", que, crudo)
		}
	}
}
