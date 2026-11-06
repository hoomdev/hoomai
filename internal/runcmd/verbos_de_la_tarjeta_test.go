// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md
// (CA-493, CA-494, CA-495): el tiempo limite deja de ser una constante.
// StartOptions.Timeout acota cada invocacion de ESE run (0 = el del
// Manager), el sidecar lo guarda en timeout_s y la regla que decide si un run
// de otro proceso sigue vivo usa el tiempo limite del run mas 5 minutos. Al
// vencer, el detalle dice si la CLI seguia narrando (un evento del provider
// en los 60 s anteriores) o estaba muda. Los CLIs son falsos, al frente del
// PATH, y los tiempos limite de los tests son de segundos.
package runcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// vdtRaiz es un directorio de proyecto sin symlinks (macOS: /var -> /private/var).
func vdtRaiz(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// vdtCorrer arranca un run del provider con opts y espera a que cierre (con
// un tope de tope). Devuelve el estado final, sus eventos y cuanto tardo.
func vdtCorrer(t *testing.T, ca string, m *Manager, opts StartOptions, tope time.Duration) (Run, []Event, time.Duration) {
	t.Helper()
	t0 := time.Now()
	info, err := m.Start(opts)
	if err != nil {
		t.Fatalf("%s: fixture: el run arranca: %v", ca, err)
	}
	listo := make(chan Run, 1)
	go func() { listo <- m.Wait(info.ID) }()
	var fin Run
	select {
	case fin = <-listo:
	case <-time.After(tope):
		m.Cancel(info.ID)
		t.Fatalf("%s: el run no cerro en %s", ca, tope)
	}
	tarda := time.Since(t0)
	_, evs, err := m.Events(info.ID, 0)
	if err != nil {
		t.Fatalf("%s: Events del run: %v", ca, err)
	}
	return fin, evs, tarda
}

// vdtDetalleTimeout es el detail del evento que cierra el run por tiempo
// limite ("" si no hay ninguno).
func vdtDetalleTimeout(evs []Event) string {
	for i := len(evs) - 1; i >= 0; i-- {
		if strings.HasPrefix(evs[i].Detail, "timeout tras ") {
			return evs[i].Detail
		}
	}
	return ""
}

// vdtMetaCruda lee el sidecar del run como JSON crudo.
func vdtMetaCruda(t *testing.T, root, id string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", id+".meta.json"))
	if err != nil {
		t.Fatalf("el sidecar de %s esta en disco: %v", id, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("el sidecar de %s es JSON: %v", id, err)
	}
	return m
}

// ---------------------------------------------------------------- CA-494

// CA-494: un run con Timeout corta a ese tiempo y no a los 30 minutos del
// Manager, y su sidecar trae timeout_s.
func TestCA494_RunConTimeoutCortaASuTiempo(t *testing.T) {
	installFake(t, "exec sleep 60\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	if m.Timeout != DefaultTimeout {
		t.Fatalf("CA-494: fixture: el Manager tiene los 30 minutos de hoom: %s", m.Timeout)
	}
	fin, _, tarda := vdtCorrer(t, "CA-494", m, StartOptions{Provider: "claude", Prompt: "trabaja", Timeout: 2 * time.Second}, 25*time.Second)
	if fin.Status != StatusError {
		t.Fatalf("CA-494: el run que vence su tiempo limite cierra en error: %+v", fin)
	}
	if tarda > 20*time.Second {
		t.Fatalf("CA-494: el run corta a su tiempo limite (2s), no al del Manager: tardo %s", tarda)
	}
	meta, ok := ReadMeta(root, fin.ID)
	if !ok || meta.TimeoutS != 2 {
		t.Fatalf("CA-494: el sidecar trae timeout_s = 2: %+v", meta)
	}
	if v := strings.TrimSpace(string(vdtMetaCruda(t, root, fin.ID)["timeout_s"])); v != "2" {
		t.Fatalf("CA-494: el sidecar en disco tiene \"timeout_s\": 2, tiene %q", v)
	}
}

// CA-494: un run con un Timeout MAYOR que el del Manager no corta al del
// Manager: termina solo.
func TestCA494_RunConTimeoutMayorQueElDelManager(t *testing.T) {
	installFake(t, "sleep 2\nexit 0\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	m.Timeout = 500 * time.Millisecond
	fin, evs, _ := vdtCorrer(t, "CA-494", m, StartOptions{Provider: "claude", Prompt: "trabaja", Timeout: 20 * time.Second}, 25*time.Second)
	if fin.Status != StatusDone || vdtDetalleTimeout(evs) != "" {
		t.Fatalf("CA-494: con Timeout 20s el run no corta a los 0.5s del Manager: %+v\n%s", fin, allDetails(evs))
	}
	if meta, _ := ReadMeta(root, fin.ID); meta.TimeoutS != 20 {
		t.Fatalf("CA-494: el sidecar dice el tiempo limite del run (20): %+v", meta)
	}
}

// CA-494: Timeout 0 es el del Manager.
func TestCA494_TimeoutCeroEsElDelManager(t *testing.T) {
	installFake(t, "exec sleep 60\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	m.Timeout = 2 * time.Second
	fin, evs, tarda := vdtCorrer(t, "CA-494", m, StartOptions{Provider: "claude", Prompt: "trabaja"}, 25*time.Second)
	if fin.Status != StatusError || vdtDetalleTimeout(evs) == "" || tarda > 20*time.Second {
		t.Fatalf("CA-494: Timeout 0 corta al del Manager (2s): %+v tardo %s\n%s", fin, tarda, allDetails(evs))
	}
}

// vdtHuerfano deja un run "running" de otro proceso vivo (este) con su
// narracion: un evento de texto en ultimo y timeout_s = timeoutS.
func vdtHuerfano(t *testing.T, root, id string, ultimo time.Time, timeoutS int) {
	t.Helper()
	dir := filepath.Join(root, ".hoom", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	meta := Meta{ID: id, Provider: "claude", Role: "writer", Dir: root, CreatedAt: ultimo.Add(-time.Minute),
		Status: StatusRunning, ExitCode: -1, PID: os.Getpid(), TimeoutS: timeoutS}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".meta.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	var lineas []string
	for _, ev := range []Event{{TS: ultimo.Add(-time.Minute), Kind: "start", Detail: "run " + id}, {TS: ultimo, Kind: "text", Detail: "pensando"}} {
		enc, _ := json.Marshal(ev)
		lineas = append(lineas, string(enc))
	}
	if err := os.WriteFile(filepath.Join(dir, id+".jsonl"), []byte(strings.Join(lineas, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// vdtSigueVivo dice si otra instancia de hoom (NewManager) dejo el run en
// running.
func vdtSigueVivo(t *testing.T, root, id string) bool {
	t.Helper()
	NewManager(root)
	meta, ok := ReadMeta(root, id)
	if !ok {
		t.Fatalf("el sidecar de %s sigue en disco", id)
	}
	return meta.Status == StatusRunning
}

// CA-494 (y caso limite "un run que vencio con timeout: 2h"): otra instancia
// de hoom lo sigue dando por vivo mientras su proceso viva y su ultima
// narracion tenga menos de 2 h 5 min, no 35 min; pasado eso, lo cierra.
func TestCA494_RunVivoConSuPropioTiempoLimite(t *testing.T) {
	ahora := time.Now().UTC()
	casos := []struct {
		nombre   string
		hace     time.Duration
		timeoutS int
		vivo     bool
	}{
		{"timeout 2h, narro hace 1h", time.Hour, 7200, true},
		{"timeout 2h, narro hace 2h4m", 2*time.Hour + 4*time.Minute, 7200, true},
		{"timeout 2h, narro hace 2h6m", 2*time.Hour + 6*time.Minute, 7200, false},
		{"timeout 2m, narro hace 10m (el del Manager lo daria por vivo)", 10 * time.Minute, 120, false},
		{"timeout 2m, narro hace 6m", 6 * time.Minute, 120, true},
		// sin timeout_s (un sidecar viejo): el del Manager mas 5 minutos
		{"sin timeout_s, narro hace 1h", time.Hour, 0, false},
		{"sin timeout_s, narro hace 30m", 30 * time.Minute, 0, true},
	}
	for i, c := range casos {
		root := vdtRaiz(t)
		id := "20261105T10000" + string(rune('0'+i)) + "_vivo01"
		vdtHuerfano(t, root, id, ahora.Add(-c.hace), c.timeoutS)
		if got := vdtSigueVivo(t, root, id); got != c.vivo {
			t.Fatalf("CA-494: [%s] otra instancia lo da por vivo=%v, fue vivo=%v", c.nombre, c.vivo, got)
		}
	}
}

// ---------------------------------------------------------------- CA-495

const (
	vdtNarrando = `^timeout tras 2s: la CLI seguia narrando \(ultimo evento hace [0-9]+s\), no esperaba un permiso\. Accion: subi el tiempo limite \(`
	vdtFinal    = `\) o pedi menos por run$`
	vdtMudo     = `^timeout tras 2s sin narracion desde hace [0-9]+(m[0-9]+)?s: la CLI puede estar esperando un permiso interactivo\. Accion: preconfigura sus permisos headless$`
)

// CA-495: con un evento del provider en los 60 s anteriores el detalle es
// "seguia narrando", y su accion nombra el campo del rol en hoom.yaml.
func TestCA495_SeguiaNarrandoNombraElTiempoLimiteDelRol(t *testing.T) {
	installFake(t, "while true; do echo 'sigo pensando' >&2; echo 'una linea de narracion'; sleep 0.2; done\n")
	for _, k := range []struct{ rol, accion string }{
		{"writer", `--timeout o agents\.writer\.timeout en hoom\.yaml`},
		{"test-writer", `--timeout o agents\.test-writer\.timeout en hoom\.yaml`},
		{"reviewer", `--timeout o review\.timeout en hoom\.yaml`},
		{"", `--timeout`},
	} {
		root := vdtRaiz(t)
		m := NewManager(root)
		fin, evs, _ := vdtCorrer(t, "CA-495", m, StartOptions{Provider: "claude", Prompt: "trabaja", Role: k.rol, Timeout: 2 * time.Second}, 25*time.Second)
		d := vdtDetalleTimeout(evs)
		if fin.Status != StatusError || !regexp.MustCompile(vdtNarrando+k.accion+vdtFinal).MatchString(d) {
			t.Fatalf("CA-495: [rol %q] una CLI que narro hasta el final vence con\n  %s\nfue\n  %q\n%s", k.rol,
				"timeout tras 2s: la CLI seguia narrando (ultimo evento hace <s>s), no esperaba un permiso. Accion: subi el tiempo limite ("+k.accion+") o pedi menos por run",
				d, allDetails(evs))
		}
		if strings.Contains(d, "¿la CLI espera un permiso interactivo?") {
			t.Fatalf("CA-495: el mensaje de hoy, que culpa a los permisos, desaparece cuando la CLI narraba: %q", d)
		}
	}
}

// CA-495: un solo evento del provider (stderr) al principio y despues
// silencio: sigue dentro de los 60 s, asi que tambien es "seguia narrando".
func TestCA495_UnEventoDentroDeLaVentanaCuenta(t *testing.T) {
	installFake(t, "echo 'arranque' >&2\nexec sleep 60\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	_, evs, _ := vdtCorrer(t, "CA-495", m, StartOptions{Provider: "claude", Prompt: "trabaja", Role: "writer", Timeout: 2 * time.Second}, 25*time.Second)
	d := vdtDetalleTimeout(evs)
	if !regexp.MustCompile(vdtNarrando + `--timeout o agents\.writer\.timeout en hoom\.yaml` + vdtFinal).MatchString(d) {
		t.Fatalf("CA-495: un evento en los 60 s anteriores es 'seguia narrando': %q\n%s", d, allDetails(evs))
	}
	if VentanaNarrando != 60*time.Second {
		t.Fatalf("CA-495: la ventana es de 60 segundos: %s", VentanaNarrando)
	}
}

// CA-495: sin ningun evento del provider el detalle es "sin narracion" y
// manda a preconfigurar los permisos.
func TestCA495_SinNarracionEsperaUnPermiso(t *testing.T) {
	installFake(t, "exec sleep 60\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	fin, evs, _ := vdtCorrer(t, "CA-495", m, StartOptions{Provider: "claude", Prompt: "trabaja", Role: "writer", Timeout: 2 * time.Second}, 25*time.Second)
	d := vdtDetalleTimeout(evs)
	if fin.Status != StatusError || !regexp.MustCompile(vdtMudo).MatchString(d) {
		t.Fatalf("CA-495: una CLI muda vence con\n  %s\nfue\n  %q\n%s",
			"timeout tras 2s sin narracion desde hace <m>: la CLI puede estar esperando un permiso interactivo. Accion: preconfigura sus permisos headless",
			d, allDetails(evs))
	}
}

// CA-495: la linea de arranque de hoom y sus avisos no son narracion del
// provider: un run con un aviso de campo ignorado y un provider mudo sigue
// siendo "sin narracion".
func TestCA495_LosAvisosDeHoomNoSonNarracion(t *testing.T) {
	installFakeNamed(t, "gemini", "exec sleep 60\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	_, evs, _ := vdtCorrer(t, "CA-495", m, StartOptions{Provider: "gemini", Prompt: "trabaja", Role: "writer", Model: "un-modelo", Timeout: 2 * time.Second}, 25*time.Second)
	if !strings.Contains(allDetails(evs), "aviso: gemini no soporta") {
		t.Fatalf("CA-495: fixture: el run tiene el aviso de hoom de un campo ignorado:\n%s", allDetails(evs))
	}
	if d := vdtDetalleTimeout(evs); !regexp.MustCompile(vdtMudo).MatchString(d) {
		t.Fatalf("CA-495: los avisos de hoom no cuentan como narracion: %q\n%s", d, allDetails(evs))
	}
}

// ---------------------------------------------------------------- CA-493

// CA-493: un provider sin la capacidad effort lo ignora con el aviso de
// siempre; con Strict es un error antes de crear el run (ni sidecar ni log).
func TestCA493_EffortEnUnProviderQueNoLoSoporta(t *testing.T) {
	installFakeNamed(t, "gemini", "exit 0\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	fin, evs, _ := vdtCorrer(t, "CA-493", m, StartOptions{Provider: "gemini", Prompt: "hola", Effort: "high"}, 25*time.Second)
	if !strings.Contains(allDetails(evs), "aviso: gemini no soporta effort; se ignora") {
		t.Fatalf("CA-493: sin la capacidad effort el run avisa y sigue: %+v\n%s", fin, allDetails(evs))
	}

	root = vdtRaiz(t)
	m = NewManager(root)
	if _, err := m.Start(StartOptions{Provider: "gemini", Prompt: "hola", Effort: "high", Strict: true}); err == nil || !strings.Contains(err.Error(), "effort") {
		t.Fatalf("CA-493: con Strict, effort en un provider que no lo soporta es un error que lo nombra: %v", err)
	}
	if entries, _ := os.ReadDir(filepath.Join(root, ".hoom", "runs")); len(entries) != 0 {
		t.Fatalf("CA-493: el error es antes de crear el run: hay %d archivos en .hoom/runs", len(entries))
	}
}

// CA-493: el provider recibe el esfuerzo (claude --effort, codex -c
// model_reasoning_effort).
func TestCA493_ElProviderRecibeElEsfuerzo(t *testing.T) {
	argv := filepath.Join(t.TempDir(), "argv")
	installFake(t, "printf '%s\\n' \"$@\" > '"+argv+"'\nexit 0\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	vdtCorrer(t, "CA-493", m, StartOptions{Provider: "claude", Prompt: "hola", Effort: "high", Timeout: 30 * time.Second}, 25*time.Second)
	raw, _ := os.ReadFile(argv)
	if !strings.Contains(string(raw), "--effort\nhigh\n") {
		t.Fatalf("CA-493: claude recibe --effort high:\n%s", raw)
	}
}
