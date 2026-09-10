// Tests del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md
// (CA-211, CA-212, CA-213): que hace Input con Strict en un provider sin
// continuacion. Era un accidente —el error generico de campo no soportado— y
// pasa a ser una decision: perder la sesion no es perder un flag, es empezar
// de cero, que es justo la degradacion que Strict prohibe.
package runcmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeConLog instala un provider falso que ANOTA cada invocacion (con sus
// argumentos) en un archivo: asi el test puede afirmar que no se lanzo ningun
// proceso, y con que argv se lanzo cuando si.
func fakeConLog(t *testing.T, name, salida string) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "invocaciones.txt")
	t.Setenv("HOOM_FAKE_LOG", log)
	installFakeNamed(t, name, `printf 'INVOCACION %s\n' "$*" >> "$HOOM_FAKE_LOG"`+"\n"+salida)
	return log
}

func invocaciones(t *testing.T, log string) []string {
	t.Helper()
	raw, err := os.ReadFile(log)
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// CA-211: con Strict, Input sobre un provider que no puede continuar se niega
// con un error de dominio y deja el run EXACTAMENTE como estaba.
func TestCA211_InputStrictSinContinuacionSeNiega(t *testing.T) {
	log := fakeConLog(t, "gemini", "printf 'hola\\n'\nexit 0\n")
	root := t.TempDir()
	m := NewManager(root)

	info, err := m.Start(StartOptions{Provider: "gemini", Prompt: "hola", Strict: true})
	if err != nil {
		t.Fatalf("CA-211: arrancar no es continuar; Start no tiene por que fallar: %v", err)
	}
	antes := waitRun(t, m, info.ID)
	if len(invocaciones(t, log)) != 1 {
		t.Fatalf("CA-211: el run arranco una vez: %v", invocaciones(t, log))
	}

	_, err = m.Input(info.ID, "seguimos")
	var sin ErrNoContinuation
	if !errors.As(err, &sin) {
		t.Fatalf("CA-211: Input con strict debe negarse con ErrNoContinuation, fue %v", err)
	}
	if sin.Provider != "gemini" {
		t.Fatalf("CA-211: el error nombra al provider: %+v", sin)
	}
	for _, quiero := range []string{"gemini", "strict", "Accion:"} {
		if !strings.Contains(err.Error(), quiero) {
			t.Fatalf("CA-211: el mensaje debe decir %q: %v", quiero, err)
		}
	}

	// el run quedo como estaba: terminal, sin eventos nuevos, sin proceso
	despues, evs, err := m.Events(info.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if despues.Status != antes.Status || despues.ExitCode != antes.ExitCode {
		t.Fatalf("CA-211: la negativa no cambia el estado del run: %+v vs %+v", despues, antes)
	}
	if len(evs) != antes.NumEvents {
		t.Fatalf("CA-211: la negativa no escribe eventos: %d vs %d", len(evs), antes.NumEvents)
	}
	if n := len(invocaciones(t, log)); n != 1 {
		t.Fatalf("CA-211: la negativa no lanza ningun proceso, hubo %d invocaciones", n)
	}
	// y el directorio quedo libre: otro run puede arrancar ahi. Se espera a que
	// cierre, o su meta final se escribe sobre el TempDir que el test ya esta
	// borrando —la goroutine del run sobrevive al cuerpo del test.
	otro, err := m.Start(StartOptions{Provider: "gemini", Prompt: "otro", Strict: true})
	if err != nil {
		t.Fatalf("CA-211: la negativa no deja el arbol ocupado: %v", err)
	}
	waitRun(t, m, otro.ID)
}

// CA-212: sin Strict el comportamiento se mantiene —invocacion nueva— con UNA
// sola linea de aviso, que dice lo que importa: que el contexto no viaja.
func TestCA212_InputSinStrictAvisaUnaVez(t *testing.T) {
	log := fakeConLog(t, "gemini", "printf 'hola\\n'\nexit 0\n")
	m := NewManager(t.TempDir())
	info, err := m.Start(StartOptions{Provider: "gemini", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	antes := waitRun(t, m, info.ID)

	if _, err := m.Input(info.ID, "seguimos"); err != nil {
		t.Fatalf("CA-212: sin strict, Input sigue lanzando una invocacion nueva: %v", err)
	}
	waitRun(t, m, info.ID)
	if n := len(invocaciones(t, log)); n != 2 {
		t.Fatalf("CA-212: sin strict se lanza la invocacion nueva, hubo %d", n)
	}

	_, evs, _ := m.Events(info.ID, antes.NumEvents)
	var avisos []string
	for _, ev := range evs {
		if strings.HasPrefix(ev.Detail, "aviso:") {
			avisos = append(avisos, ev.Detail)
		}
	}
	if len(avisos) != 1 {
		t.Fatalf("CA-212: UNA sola linea de aviso, hubo %d: %v", len(avisos), avisos)
	}
	if !strings.Contains(avisos[0], "empieza de cero") || !strings.Contains(avisos[0], "no viaja") {
		t.Fatalf("CA-212: el aviso dice que el contexto anterior no viaja: %q", avisos[0])
	}
}

// CA-213: los positivos bajo Strict. Con sesion capturada y provider con
// resume, se continua por id; un provider con continue y sin resume continua
// la ultima sesion del directorio, que es continuacion real.
func TestCA213_ContinuacionRealBajoStrict(t *testing.T) {
	log := fakeConLog(t, "claude", `printf '{"type":"system","subtype":"init","session_id":"s-1"}\n'`+"\nexit 0\n")
	m := NewManager(t.TempDir())
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	fin := waitRun(t, m, info.ID)
	if fin.ProviderSessionID != "s-1" {
		t.Fatalf("CA-213: la sesion se captura del stream: %+v", fin)
	}
	if _, err := m.Input(info.ID, "seguimos"); err != nil {
		t.Fatalf("CA-213: con resume y sesion capturada, Input continua sin quejarse: %v", err)
	}
	waitRun(t, m, info.ID)
	inv := invocaciones(t, log)
	if len(inv) != 2 || !strings.Contains(inv[1], "--resume s-1") {
		t.Fatalf("CA-213: la segunda invocacion continua por id: %v", inv)
	}

	// opencode: continue si, resume no. Continuar la ultima sesion del
	// directorio ES continuar, asi que Strict no tiene nada que objetar.
	log2 := fakeConLog(t, "opencode", "printf 'listo\\n'\nexit 0\n")
	m2 := NewManager(t.TempDir())
	oc, err := m2.Start(StartOptions{Provider: "opencode", Prompt: "hola", Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m2, oc.ID)
	if _, err := m2.Input(oc.ID, "seguimos"); err != nil {
		t.Fatalf("CA-213: continuar el directorio es continuacion real: %v", err)
	}
	waitRun(t, m2, oc.ID)
	inv2 := invocaciones(t, log2)
	if len(inv2) != 2 || !strings.Contains(inv2[1], "--continue") {
		t.Fatalf("CA-213: opencode continua con --continue: %v", inv2)
	}
}
