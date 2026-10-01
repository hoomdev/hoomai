// Tests adversariales del spec .hoom/specs/quien-cierra-un-hallazgo.md
// (CA-435): el entorno de una corrida. Cada corrida con rol pone en el
// entorno de su provider HOOM_ROLE (el slug del rol), HOOM_RUN (el id de la
// corrida) y HOOM_PROVIDER (el provider que la corre), ademas de HOOM_TASK
// como hoy; un valor heredado del proceso padre nunca se filtra. Un 'hoom
// run' sin rol no tiene HOOM_ROLE.
package runcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// qcVar es lo que vio el provider de una variable: definida o no, y su valor.
type qcVar struct {
	definida bool
	valor    string
}

// qcInvocacion es el entorno que vio UNA invocacion del provider.
type qcInvocacion struct {
	role, run, provider, task qcVar
}

// qcScriptEntorno es la linea de shell que AGREGA a log el entorno de la
// invocacion: definida|valor de HOOM_ROLE, HOOM_RUN, HOOM_PROVIDER y
// HOOM_TASK, separados por '|'.
func qcScriptEntorno(log string) string {
	return "printf '%s|%s|%s|%s|%s|%s|%s|%s\\n' " +
		"\"${HOOM_ROLE+d}\" \"${HOOM_ROLE}\" \"${HOOM_RUN+d}\" \"${HOOM_RUN}\" " +
		"\"${HOOM_PROVIDER+d}\" \"${HOOM_PROVIDER}\" \"${HOOM_TASK+d}\" \"${HOOM_TASK}\" >> '" + log + "'\n"
}

// qcFakeClaude arma un `claude` falso que deja su entorno y reporta una
// sesion, para que Input pueda reanudar.
func qcFakeClaude(t *testing.T) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "entorno.txt")
	installFake(t, qcScriptEntorno(log)+
		`echo '{"type":"system","subtype":"init","session_id":"sess-qc"}'`+"\n"+
		`echo '{"type":"result","subtype":"success","is_error":false,"result":"listo","session_id":"sess-qc"}'`+"\n"+
		"exit 0\n")
	return log
}

// qcFakeCodex arma un `codex` falso que deja su entorno.
func qcFakeCodex(t *testing.T) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "entorno-codex.txt")
	installFakeNamed(t, "codex", qcScriptEntorno(log)+
		`printf '{"type":"thread.started","thread_id":"t-qc"}\n'`+"\n"+
		`printf '{"type":"turn.completed","usage":{"input_tokens":1,"cached_input_tokens":0,"output_tokens":1}}\n'`+"\n"+
		"exit 0\n")
	return log
}

func qcLeer(t *testing.T, log string) []qcInvocacion {
	t.Helper()
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("CA-435: el proceso del provider no dejo su entorno: %v", err)
	}
	var out []qcInvocacion
	for _, l := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		p := strings.Split(l, "|")
		if len(p) != 8 {
			t.Fatalf("CA-435: salida inesperada del provider falso: %q", l)
		}
		v := func(i int) qcVar { return qcVar{definida: p[i] == "d", valor: p[i+1]} }
		out = append(out, qcInvocacion{role: v(0), run: v(2), provider: v(4), task: v(6)})
	}
	return out
}

// qcPadreAjeno exporta en el proceso padre valores ajenos de las cuatro
// variables: los que tendria un humano, o el rol que corre la suite.
func qcPadreAjeno(t *testing.T) {
	t.Helper()
	t.Setenv("HOOM_ROLE", "rol-ajeno")
	t.Setenv("HOOM_RUN", "run-ajeno")
	t.Setenv("HOOM_PROVIDER", "provider-ajeno")
	t.Setenv("HOOM_TASK", "ajena")
}

// qcExacta exige que la variable este definida con ese valor.
func qcExacta(t *testing.T, que, nombre string, v qcVar, quiero string) {
	t.Helper()
	if !v.definida || v.valor != quiero {
		t.Fatalf("CA-435: %s: el provider ve %s=%q, vio definida=%v valor=%q", que, nombre, quiero, v.definida, v.valor)
	}
}

// CA-435: una corrida con rol (y tarea) le da a su provider HOOM_ROLE,
// HOOM_RUN y HOOM_PROVIDER con su rol, su id y su provider, y HOOM_TASK como
// hoy; ninguno de los valores del padre se filtra. La continuacion (Input)
// es la misma corrida: el mismo entorno.
func TestCA435_LaCorridaConRolLeDaSuRolSuIDYSuProvider(t *testing.T) {
	log := qcFakeClaude(t)
	qcPadreAjeno(t)
	root := initRepo(t)
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "worktrees", "cabina-visual"), 0o755); err != nil {
		t.Fatal(err)
	}

	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "revisa", Role: "reviewer", Task: "cabina-visual"})
	if err != nil {
		t.Fatal(err)
	}
	if run.ID == "" {
		t.Fatalf("CA-435: fixture: la corrida tiene id: %+v", run)
	}
	waitRun(t, m, run.ID)
	inv := qcLeer(t, log)
	if len(inv) != 1 {
		t.Fatalf("CA-435: una invocacion del provider, hubo %d: %+v", len(inv), inv)
	}
	qcExacta(t, "primera invocacion", "HOOM_ROLE", inv[0].role, "reviewer")
	qcExacta(t, "primera invocacion", "HOOM_RUN", inv[0].run, run.ID)
	qcExacta(t, "primera invocacion", "HOOM_PROVIDER", inv[0].provider, "claude")
	qcExacta(t, "primera invocacion", "HOOM_TASK", inv[0].task, "cabina-visual")

	if _, err := m.Input(run.ID, "segundo"); err != nil {
		t.Fatalf("CA-435: Input: %v", err)
	}
	waitRun(t, m, run.ID)
	inv = qcLeer(t, log)
	if len(inv) != 2 {
		t.Fatalf("CA-435: Input invoca al provider otra vez, hubo %d: %+v", len(inv), inv)
	}
	qcExacta(t, "continuacion", "HOOM_ROLE", inv[1].role, "reviewer")
	qcExacta(t, "continuacion", "HOOM_RUN", inv[1].run, run.ID)
	qcExacta(t, "continuacion", "HOOM_PROVIDER", inv[1].provider, "claude")
	qcExacta(t, "continuacion", "HOOM_TASK", inv[1].task, "cabina-visual")
}

// CA-435: HOOM_PROVIDER es el provider que corre la corrida (codex), y el
// rol es el de esta corrida (refutador), sin tarea: HOOM_TASK vacia.
func TestCA435_ElProviderEsElQueCorreLaCorrida(t *testing.T) {
	log := qcFakeCodex(t)
	qcPadreAjeno(t)
	root := initRepo(t)

	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "codex", Prompt: "refuta", Role: "refutador"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, run.ID)
	inv := qcLeer(t, log)
	if len(inv) != 1 {
		t.Fatalf("CA-435: una invocacion de codex, hubo %d: %+v", len(inv), inv)
	}
	qcExacta(t, "codex", "HOOM_ROLE", inv[0].role, "refutador")
	qcExacta(t, "codex", "HOOM_RUN", inv[0].run, run.ID)
	qcExacta(t, "codex", "HOOM_PROVIDER", inv[0].provider, "codex")
	if inv[0].task.valor != "" {
		t.Fatalf("CA-435: HOOM_TASK como hoy: sin tarea va vacia, nunca la ajena: %q", inv[0].task.valor)
	}
}

// CA-435: cada corrida lleva SU id: dos corridas seguidas del mismo manager
// (roles distintos) ven cada una el suyo, y no son el mismo.
func TestCA435_CadaCorridaLlevaSuPropioID(t *testing.T) {
	log := qcFakeClaude(t)
	qcPadreAjeno(t)
	root := initRepo(t)

	m := NewManager(root)
	a, err := m.Start(StartOptions{Provider: "claude", Prompt: "implementa", Role: "writer"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, a.ID)
	b, err := m.Start(StartOptions{Provider: "claude", Prompt: "revisa", Role: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, b.ID)
	if a.ID == b.ID {
		t.Fatalf("CA-435: fixture: dos corridas, dos ids: %s", a.ID)
	}
	inv := qcLeer(t, log)
	if len(inv) != 2 {
		t.Fatalf("CA-435: dos invocaciones, hubo %d: %+v", len(inv), inv)
	}
	qcExacta(t, "corrida del writer", "HOOM_ROLE", inv[0].role, "writer")
	qcExacta(t, "corrida del writer", "HOOM_RUN", inv[0].run, a.ID)
	qcExacta(t, "corrida del reviewer", "HOOM_ROLE", inv[1].role, "reviewer")
	qcExacta(t, "corrida del reviewer", "HOOM_RUN", inv[1].run, b.ID)
}

// CA-435: un 'hoom run' sin rol no tiene HOOM_ROLE: ni la del padre (un
// humano, o el rol que lanzo este proceso) ni ninguna. HOOM_RUN y
// HOOM_PROVIDER nunca son las heredadas: vacias o las de esta corrida.
func TestCA435_SinRolNoHayHOOMROLEYNadaSeHereda(t *testing.T) {
	log := qcFakeClaude(t)
	qcPadreAjeno(t)
	t.Setenv("HOOM_ROLE", "reviewer") // el padre es la corrida de un reviewer
	root := initRepo(t)

	m := NewManager(root)
	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, run.ID)

	// y con el padre sin ninguna de las variables
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER"} {
		if err := os.Unsetenv(k); err != nil { // t.Setenv las restaura al final
			t.Fatal(err)
		}
	}
	run2, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, run2.ID)

	inv := qcLeer(t, log)
	if len(inv) != 2 {
		t.Fatalf("CA-435: dos invocaciones, hubo %d: %+v", len(inv), inv)
	}
	for i, id := range []string{run.ID, run2.ID} {
		if inv[i].role.valor != "" {
			t.Fatalf("CA-435: corrida %d: un 'hoom run' sin rol no tiene HOOM_ROLE, vio %q", i+1, inv[i].role.valor)
		}
		if v := inv[i].run.valor; v != "" && v != id {
			t.Fatalf("CA-435: corrida %d: HOOM_RUN vacia o la de esta corrida (%s), nunca la heredada: %q", i+1, id, v)
		}
		if v := inv[i].provider.valor; v != "" && v != "claude" {
			t.Fatalf("CA-435: corrida %d: HOOM_PROVIDER vacia o el de esta corrida (claude), nunca el heredado: %q", i+1, v)
		}
	}
}
