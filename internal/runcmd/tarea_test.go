// Tests adversariales del spec .hoom/specs/arquitecto-bajo-el-sobre.md
// (CA-243): la tarea viaja por el entorno. runcmd es el unico ejecutor, asi
// que pone HOOM_TASK en el proceso del provider SIEMPRE: con la tarea del run,
// o vacia, para no heredar una ajena.
package runcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// abEntorno arma un `claude` falso que deja en un archivo si HOOM_TASK estaba
// definida y con que valor: "definida|valor" o "|".
func abEntorno(t *testing.T) string {
	t.Helper()
	salida := filepath.Join(t.TempDir(), "entorno.txt")
	installFake(t, "printf '%s|%s' \"${HOOM_TASK+definida}\" \"${HOOM_TASK}\" > "+salida+"\nexit 0\n")
	return salida
}

func abLeer(t *testing.T, path string) (definida bool, valor string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("CA-243: el proceso del provider no dejo su entorno: %v", err)
	}
	partes := strings.SplitN(string(raw), "|", 2)
	if len(partes) != 2 {
		t.Fatalf("CA-243: salida inesperada del provider falso: %q", raw)
	}
	return partes[0] == "definida", partes[1]
}

// CA-243: el proceso del provider recibe HOOM_TASK igual a la tarea del run,
// aunque el proceso padre tenga otra.
func TestCA243_ElProviderRecibeLaTareaDelRun(t *testing.T) {
	if EnvTask != "HOOM_TASK" {
		t.Fatalf("CA-243: la variable es HOOM_TASK, es %q", EnvTask)
	}
	salida := abEntorno(t)
	t.Setenv("HOOM_TASK", "ajena") // el humano la tenia exportada
	root := initRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "worktrees", "cabina-visual"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Task: "cabina-visual"})
	if err != nil {
		t.Fatal(err)
	}
	m.Wait(run.ID)
	definida, valor := abLeer(t, salida)
	if valor != "cabina-visual" || !definida {
		t.Fatalf("CA-243: el provider debe ver HOOM_TASK=cabina-visual, vio definida=%v valor=%q", definida, valor)
	}
}

// CA-243: un run sin tarea NO hereda la del proceso padre: HOOM_TASK va
// vacia, y va SIEMPRE (el contrato la pone, no la borra).
func TestCA243_SinTareaNoSeHeredaUnaAjena(t *testing.T) {
	salida := abEntorno(t)
	t.Setenv("HOOM_TASK", "ajena")
	root := initRepo(t)

	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	m.Wait(run.ID)
	definida, valor := abLeer(t, salida)
	if valor == "ajena" {
		t.Fatal("CA-243: un run sin tarea heredo la tarea del proceso padre")
	}
	if valor != "" {
		t.Fatalf("CA-243: un run sin tarea lleva HOOM_TASK vacia, lleva %q", valor)
	}
	if !definida {
		t.Fatal("CA-243: HOOM_TASK se pone SIEMPRE en el proceso del provider, vacia sin tarea")
	}

	// y con el padre sin la variable, tambien va vacia
	if err := os.Unsetenv("HOOM_TASK"); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(salida); err != nil {
		t.Fatal(err)
	}
	run, err = m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	m.Wait(run.ID)
	if _, valor := abLeer(t, salida); valor != "" {
		t.Fatalf("CA-243: sin tarea y sin padre, HOOM_TASK vacia: %q", valor)
	}
}

// CA-243: CurrentTask. El flag gana; sin flag manda HOOM_TASK; sin ambos, "".
func TestCA243_CurrentTaskElFlagManda(t *testing.T) {
	t.Setenv(EnvTask, "del-entorno")
	if got := CurrentTask("del-flag"); got != "del-flag" {
		t.Fatalf("CA-243: el flag gana sobre HOOM_TASK: %q", got)
	}
	if got := CurrentTask(""); got != "del-entorno" {
		t.Fatalf("CA-243: sin flag manda HOOM_TASK: %q", got)
	}

	t.Setenv(EnvTask, "")
	if got := CurrentTask(""); got != "" {
		t.Fatalf("CA-243: HOOM_TASK vacia y sin flag es \"\": %q", got)
	}
	if got := CurrentTask("solo-flag"); got != "solo-flag" {
		t.Fatalf("CA-243: el flag solo alcanza: %q", got)
	}

	if err := os.Unsetenv(EnvTask); err != nil { // t.Setenv la restaura al final
		t.Fatal(err)
	}
	if got := CurrentTask(""); got != "" {
		t.Fatalf("CA-243: sin flag y sin HOOM_TASK es \"\": %q", got)
	}
}
