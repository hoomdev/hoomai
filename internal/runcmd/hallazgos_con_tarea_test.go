// Tests adversariales del contrato "review-hallazgos-con-tarea" sobre CA-243
// (.hoom/specs/arquitecto-bajo-el-sobre.md): StartOptions.FindingTask es la
// tarea a la que pertenecen los hallazgos del run. El proceso del provider
// recibe HOOM_TASK = FindingTask si no es vacio, y si no Task (como hoy). Task
// sigue decidiendo el directorio del run y el resto igual; un HOOM_TASK del
// proceso padre nunca se filtra.
package runcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// htInvocacion es lo que vio UNA invocacion del provider.
type htInvocacion struct {
	definida bool
	valor    string
	dir      string // cwd fisico
}

// htFake arma un `claude` falso que AGREGA una linea por invocacion con
// HOOM_TASK (definida o no), su valor y el cwd fisico; reporta una sesion
// para que Input pueda reanudar.
func htFake(t *testing.T) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "invocaciones.txt")
	installFake(t, "printf '%s|%s|%s\\n' \"${HOOM_TASK+definida}\" \"${HOOM_TASK}\" \"$(pwd -P)\" >> '"+log+"'\n"+
		`echo '{"type":"system","subtype":"init","session_id":"sess-ht"}'`+"\n"+
		`echo '{"type":"result","subtype":"success","is_error":false,"result":"listo","session_id":"sess-ht"}'`+"\n"+
		"exit 0\n")
	return log
}

func htLeer(t *testing.T, log string) []htInvocacion {
	t.Helper()
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("CA-243: el proceso del provider no dejo su entorno: %v", err)
	}
	var out []htInvocacion
	for _, l := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		p := strings.SplitN(l, "|", 3)
		if len(p) != 3 {
			t.Fatalf("CA-243: salida inesperada del provider falso: %q", l)
		}
		out = append(out, htInvocacion{definida: p[0] == "definida", valor: p[1], dir: p[2]})
	}
	return out
}

func htReal(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// htWorktree simula el worktree de una tarea como lo hace CA-243.
func htWorktree(t *testing.T, root, task string) string {
	t.Helper()
	dir := filepath.Join(root, ".hoom", "worktrees", task)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// CA-243: con FindingTask "b" y sin Task, el provider ve HOOM_TASK=b (no la
// "ajena" del padre, no vacia), y el run corre en la RAIZ: FindingTask no es
// un directorio (no existe ningun worktree "b" y el run no falla por eso).
// Task sigue mandando en el resto: el run y su meta no tienen tarea. Un Input
// posterior re-aplica la misma HOOM_TASK.
func TestCA243_FindingTaskSinTareaVaAlEntornoYCorreEnLaRaiz(t *testing.T) {
	log := htFake(t)
	t.Setenv("HOOM_TASK", "ajena")
	root := initRepo(t)

	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", FindingTask: "b"})
	if err != nil {
		t.Fatalf("CA-243: FindingTask no es un worktree: el run arranca en la raiz: %v", err)
	}
	final := waitRun(t, m, run.ID)
	inv := htLeer(t, log)
	if len(inv) != 1 {
		t.Fatalf("CA-243: una invocacion del provider, hubo %d: %+v", len(inv), inv)
	}
	if !inv[0].definida || inv[0].valor != "b" {
		t.Fatalf("CA-243: con FindingTask=b el provider ve HOOM_TASK=b, vio definida=%v valor=%q", inv[0].definida, inv[0].valor)
	}
	if inv[0].dir != htReal(t, root) {
		t.Fatalf("CA-243: sin Task el run corre en la raiz (%s), corrio en %s", htReal(t, root), inv[0].dir)
	}
	if final.Task != "" {
		t.Fatalf("CA-243: Task decide el resto igual que hoy: el run no tiene tarea, tiene %q", final.Task)
	}
	if meta := leerMeta(t, root, run.ID); meta.Task != "" || htReal(t, meta.Dir) != htReal(t, root) {
		t.Fatalf("CA-243: el meta sigue a Task (sin tarea, en la raiz): %+v", meta)
	}

	// la continuacion re-aplica las opciones del run: la misma HOOM_TASK
	if _, err := m.Input(run.ID, "segundo"); err != nil {
		t.Fatalf("CA-243: Input: %v", err)
	}
	waitRun(t, m, run.ID)
	inv = htLeer(t, log)
	if len(inv) != 2 {
		t.Fatalf("CA-243: Input invoca al provider otra vez, hubo %d: %+v", len(inv), inv)
	}
	if !inv[1].definida || inv[1].valor != "b" || inv[1].dir != htReal(t, root) {
		t.Fatalf("CA-243: la continuacion ve HOOM_TASK=b en la raiz: %+v", inv[1])
	}
}

// CA-243 (guarda): sin FindingTask manda Task, como hoy: HOOM_TASK=a y el run
// corre en el worktree de a, aunque el padre exporte otra.
func TestCA243_SinFindingTaskMandaLaTareaDelRun(t *testing.T) {
	log := htFake(t)
	t.Setenv("HOOM_TASK", "ajena")
	root := initRepo(t)
	wt := htWorktree(t, root, "a")

	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Task: "a"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, run.ID)
	inv := htLeer(t, log)
	if len(inv) != 1 || !inv[0].definida || inv[0].valor != "a" {
		t.Fatalf("CA-243: sin FindingTask el provider ve HOOM_TASK=Task (a): %+v", inv)
	}
	if inv[0].dir != htReal(t, wt) {
		t.Fatalf("CA-243: Task decide el directorio: el worktree %s, corrio en %s", htReal(t, wt), inv[0].dir)
	}
}

// CA-243: con FindingTask "b" y Task "a", HOOM_TASK=b (FindingTask gana en el
// entorno) pero el run corre en el worktree de a y sigue siendo de la tarea a.
func TestCA243_FindingTaskGanaEnElEntornoTaskEnElDirectorio(t *testing.T) {
	log := htFake(t)
	t.Setenv("HOOM_TASK", "ajena")
	root := initRepo(t)
	wt := htWorktree(t, root, "a")

	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Task: "a", FindingTask: "b"})
	if err != nil {
		t.Fatal(err)
	}
	final := waitRun(t, m, run.ID)
	inv := htLeer(t, log)
	if len(inv) != 1 || !inv[0].definida || inv[0].valor != "b" {
		t.Fatalf("CA-243: FindingTask no vacio gana sobre Task en HOOM_TASK: %+v", inv)
	}
	if inv[0].dir != htReal(t, wt) {
		t.Fatalf("CA-243: Task sigue decidiendo el directorio (%s), corrio en %s", htReal(t, wt), inv[0].dir)
	}
	if final.Task != "a" {
		t.Fatalf("CA-243: el run sigue siendo de su Task (a): %q", final.Task)
	}
	if meta := leerMeta(t, root, run.ID); meta.Task != "a" {
		t.Fatalf("CA-243: el meta sigue siendo de su Task (a): %+v", meta)
	}
}
