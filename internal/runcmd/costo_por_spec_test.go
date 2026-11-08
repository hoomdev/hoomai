// Tests adversariales del spec .hoom/specs/costo-por-spec.md (CA-538,
// CA-539, CA-540, CA-541) sobre el run: el sidecar guarda con que modelo y
// esfuerzo corrio, como se movio cada ventana del plan durante el run, y el
// gasto acumulado con razonamiento y escritura en cache. Escritos DESDE EL
// SPEC y las firmas publicas (Meta, VentanaRun, StartOptions), sin leer la
// implementacion.
//
// Los CLIs son falsos (installFake, installFakeNamed) e imprimen las lineas
// de los formatos verificados: Claude Code 2.1.319 y Codex 0.160.0. Codex no
// dice sus ventanas en stdout: las escribe en su archivo de sesion, que estos
// tests plantan en un CODEX_HOME propio ANTES de arrancar el run, con
// timestamps posteriores al arranque y el thread que imprime el falso.
package runcmd

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

const (
	cpsThread  = "01a1b4f6-3082-7b73-ba58-8ae399748f7f"
	cpsSesion  = "c449dc75-cc1c-4f32-9863-160dda6baa4f"
	cpsReset5h = int64(1794078000)
	cpsReset7d = int64(1794517200)
)

// cpsInit es el system/init real de Claude Code 2.1.319.
const cpsInit = `{"type":"system","subtype":"init","model":"claude-opus-5-5","session_id":"` + cpsSesion + `"}`

// cpsInitSinModelo es un init que no dice modelo.
const cpsInitSinModelo = `{"type":"system","subtype":"init","session_id":"` + cpsSesion + `"}`

// cpsResult es el result real de Claude Code 2.1.319.
const cpsResult = `{"type":"result","subtype":"success","is_error":false,"num_turns":1,"duration_ms":5019,` +
	`"total_cost_usd":0.0611242,"usage":{"input_tokens":2,"cache_creation_input_tokens":7026,` +
	`"cache_read_input_tokens":24141,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0}},` +
	`"result":"hola","session_id":"` + cpsSesion + `"}`

func cpsF(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// cpsRateLimit es un rate_limit_event de Claude con la forma real: las dos
// ventanas con utilization en fraccion y resetsAt en epoch segundos.
func cpsRateLimit(u5 float64, r5 int64, u7 float64, r7 int64) string {
	return `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":` + strconv.FormatInt(r5, 10) +
		`,"rateLimitType":"five_hour","overageStatus":"rejected","overageDisabledReason":"out_of_credits",` +
		`"isUsingOverage":false,"unifiedWindows":{"five_hour":{"utilization":` + cpsF(u5) + `,"resetsAt":` +
		strconv.FormatInt(r5, 10) + `,"status":"allowed"},"seven_day":{"utilization":` + cpsF(u7) +
		`,"resetsAt":` + strconv.FormatInt(r7, 10) + `,"status":"allowed"}}},` +
		`"uuid":"428e0ecf-84ef-4197-a967-7a7680f9a81b","session_id":"` + cpsSesion + `"}`
}

// cpsRateLimit5h es un rate_limit_event que solo trae five_hour.
func cpsRateLimit5h(u5 float64, r5 int64) string {
	return `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":` + strconv.FormatInt(r5, 10) +
		`,"rateLimitType":"five_hour","unifiedWindows":{"five_hour":{"utilization":` + cpsF(u5) + `,"resetsAt":` +
		strconv.FormatInt(r5, 10) + `,"status":"allowed"}}},"session_id":"` + cpsSesion + `"}`
}

// cpsScript arma el cuerpo de un CLI falso que, en su invocacion n (1, 2,
// ...), imprime las lineas de invocaciones[n-1] y sale con exit.
func cpsScript(t *testing.T, exit int, invocaciones ...[]string) string {
	t.Helper()
	d := t.TempDir()
	var b strings.Builder
	b.WriteString("d='" + d + "'\nn=$(cat \"$d/n\" 2>/dev/null || echo 0); n=$((n+1)); echo $n > \"$d/n\"\n")
	for i, lineas := range invocaciones {
		fmt.Fprintf(&b, "if [ $n -eq %d ]; then\ncat <<'CPS_EOF'\n%s\nCPS_EOF\nfi\n", i+1, strings.Join(lineas, "\n"))
	}
	fmt.Fprintf(&b, "exit %d\n", exit)
	return b.String()
}

func cpsClaude(t *testing.T, invocaciones ...[]string) {
	t.Helper()
	installFake(t, cpsScript(t, 0, invocaciones...))
}

// cpsCodex pone un codex falso que abre el thread cpsThread y cierra el
// turno con el gasto real de Codex 0.160.0.
func cpsCodex(t *testing.T, exit int) {
	t.Helper()
	installFakeNamed(t, "codex", cpsScript(t, exit, []string{
		`{"type":"thread.started","thread_id":"` + cpsThread + `"}`,
		`{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":800,"cache_write_input_tokens":100,"output_tokens":50,"reasoning_output_tokens":20}}`,
	}, []string{
		`{"type":"thread.started","thread_id":"` + cpsThread + `"}`,
		`{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":800,"cache_write_input_tokens":100,"output_tokens":50,"reasoning_output_tokens":20}}`,
	}))
}

// cpsCorrer arranca un run y espera a que su sidecar quede cerrado.
func cpsCorrer(t *testing.T, root string, opts StartOptions) (Run, Meta) {
	t.Helper()
	m := NewManager(root)
	info, err := m.Start(opts)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	fin := waitRun(t, m, info.ID)
	return fin, cpsMetaCerrado(t, root, info.ID)
}

// cpsMetaCerrado lee el sidecar cuando dejo de estar en running.
func cpsMetaCerrado(t *testing.T, root, id string) Meta {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		meta := leerMeta(t, root, id)
		if meta.Status != StatusRunning || time.Now().After(deadline) {
			return meta
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// cpsCrudo devuelve el sidecar como JSON crudo: para ver que claves faltan.
func cpsCrudo(t *testing.T, root, id string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", id+".meta.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("el sidecar es JSON: %v\n%s", err, raw)
	}
	return m
}

func cpsCasi(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func cpsVentana(t *testing.T, ca string, meta Meta, nombre string) VentanaRun {
	t.Helper()
	var hallada []VentanaRun
	for _, v := range meta.Ventanas {
		if v.Nombre == nombre {
			hallada = append(hallada, v)
		}
	}
	if len(hallada) != 1 {
		t.Fatalf("%s: el sidecar tiene una sola entrada %q en ventanas: %+v", ca, nombre, meta.Ventanas)
	}
	return hallada[0]
}

func cpsEpoch(s int64) time.Time { return time.Unix(s, 0).UTC() }

// ------------------------------------------------- archivo de sesion de Codex

func cpsTS(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func cpsSessionMeta(ts time.Time, id string) string {
	return `{"timestamp":"` + cpsTS(ts) + `","type":"session_meta","payload":{"id":"` + id + `","cli_version":"0.160.0"}}`
}

func cpsTurnContext(ts time.Time, model, effort string) string {
	return `{"timestamp":"` + cpsTS(ts) + `","type":"turn_context","payload":{"model":"` + model + `","effort":"` + effort + `"}}`
}

type cpsRL struct {
	used    float64
	minutes int
	resets  int64
}

func (r *cpsRL) json() string {
	if r == nil {
		return "null"
	}
	return fmt.Sprintf(`{"used_percent":%s,"window_minutes":%d,"resets_at":%d}`, cpsF(r.used), r.minutes, r.resets)
}

func cpsTokenCount(ts time.Time, primary, secondary *cpsRL, plan string) string {
	return `{"timestamp":"` + cpsTS(ts) + `","type":"event_msg","payload":{"type":"token_count","info":null,` +
		`"rate_limits":{"limit_id":"codex","primary":` + primary.json() + `,"secondary":` + secondary.json() +
		`,"plan_type":"` + plan + `"}}}`
}

// cpsSesionCodex planta el archivo de sesion del thread en home, en el
// directorio del dia de hoy (local y, si difiere, tambien UTC: el mismo
// contenido, asi que cualquiera de los dos dice lo mismo).
func cpsSesionCodex(t *testing.T, home, thread string, lineas ...string) {
	t.Helper()
	ahora := time.Now()
	for _, cuando := range []time.Time{ahora, ahora.UTC()} {
		dir := filepath.Join(home, "sessions", cuando.Format("2006"), cuando.Format("01"), cuando.Format("02"))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, "rollout-"+cuando.Format("2006-01-02T15-04-05")+"-"+thread+".jsonl")
		if err := os.WriteFile(p, []byte(strings.Join(lineas, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// cpsSesionReal es la sesion de la corrida real de Codex 0.160.0, corrida a
// despues del arranque del run (+1h): turn_context gpt-5.6-sol/low y dos
// token_count.
func cpsSesionReal(t *testing.T, home string) {
	t.Helper()
	d := time.Now().Add(time.Hour)
	cpsSesionCodex(t, home, cpsThread,
		cpsSessionMeta(d, cpsThread),
		cpsTurnContext(d.Add(time.Second), "gpt-5.6-sol", "low"),
		cpsTokenCount(d.Add(3*time.Second), &cpsRL{52.0, 300, 1794053319}, &cpsRL{30.0, 10080, 1794549884}, "plus"),
		cpsTokenCount(d.Add(9*time.Second), &cpsRL{54.0, 300, 1794053320}, &cpsRL{31.0, 10080, 1794549883}, "plus"),
	)
}

// ---------------------------------------------------------------- CA-538

// CA-538: sin modelo pedido, el sidecar guarda el que Claude dijo en su
// system/init; Claude no dice esfuerzo, asi que effort se omite.
func TestCA538_ClaudeSinModeloPedidoGuardaElDelInit(t *testing.T) {
	cpsClaude(t, []string{cpsInit, cpsResult})
	root := t.TempDir()
	fin, meta := cpsCorrer(t, root, StartOptions{Provider: "claude", Prompt: "hola"})
	if fin.Status != StatusDone {
		t.Fatalf("CA-538: fixture: el run termina bien: %+v", fin)
	}
	if meta.Model != "claude-opus-5-5" {
		t.Fatalf("CA-538: sin modelo pedido vale el del system/init (claude-opus-5-5): %q", meta.Model)
	}
	if _, hay := cpsCrudo(t, root, fin.ID)["effort"]; hay || meta.Effort != "" {
		t.Fatalf("CA-538: sin esfuerzo pedido ni informado, effort se omite: %+v", meta)
	}
}

// CA-538: lo que hoom le paso al provider manda sobre lo que el provider
// dice haber usado.
func TestCA538_ClaudeConModeloYEsfuerzoGuardaLosDeHoom(t *testing.T) {
	cpsClaude(t, []string{cpsInit, cpsResult})
	root := t.TempDir()
	_, meta := cpsCorrer(t, root, StartOptions{Provider: "claude", Prompt: "hola", Model: "sonnet", Effort: "high"})
	if meta.Model != "sonnet" || meta.Effort != "high" {
		t.Fatalf("CA-538: model/effort son los que hoom paso (sonnet, high), no los del init: %q %q", meta.Model, meta.Effort)
	}
	crudo := cpsCrudo(t, root, meta.ID)
	if crudo["model"] != "sonnet" || crudo["effort"] != "high" {
		t.Fatalf("CA-538: el JSON del sidecar trae model y effort: %v", crudo)
	}
}

// CA-538: sin modelo ni esfuerzo pedidos ni informados, el sidecar no tiene
// las claves model ni effort.
func TestCA538_SinNingunoLosOmite(t *testing.T) {
	cpsClaude(t, []string{cpsInitSinModelo, cpsResult})
	root := t.TempDir()
	fin, meta := cpsCorrer(t, root, StartOptions{Provider: "claude", Prompt: "hola"})
	crudo := cpsCrudo(t, root, fin.ID)
	for _, k := range []string{"model", "effort"} {
		if _, hay := crudo[k]; hay {
			t.Fatalf("CA-538: sin dato, %q se omite del sidecar: %v", k, crudo)
		}
	}
	if meta.Model != "" || meta.Effort != "" {
		t.Fatalf("CA-538: sin dato no se inventa: %+v", meta)
	}
}

// CA-538 y CA-541: sin modelo ni esfuerzo pedidos, el run de Codex guarda los
// del turn_context de su sesion.
func TestCA538_CodexSinModeloGuardaLosDelTurnContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	cpsSesionReal(t, home)
	cpsCodex(t, 0)
	root := t.TempDir()
	fin, meta := cpsCorrer(t, root, StartOptions{Provider: "codex", Prompt: "hola"})
	if fin.Status != StatusDone || meta.ProviderSessionID != cpsThread {
		t.Fatalf("CA-538: fixture: el run de codex termina con el thread %s: %+v", cpsThread, meta)
	}
	if meta.Model != "gpt-5.6-sol" || meta.Effort != "low" {
		t.Fatalf("CA-538: sin pedido, model/effort del turn_context (gpt-5.6-sol, low): %q %q", meta.Model, meta.Effort)
	}
}

// CA-538: con modelo y esfuerzo pedidos, el run de Codex guarda los de hoom.
func TestCA538_CodexConModeloYEsfuerzoGuardaLosDeHoom(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	cpsSesionReal(t, home)
	cpsCodex(t, 0)
	root := t.TempDir()
	_, meta := cpsCorrer(t, root, StartOptions{Provider: "codex", Prompt: "hola", Model: "gpt-elegido", Effort: "high"})
	if meta.Model != "gpt-elegido" || meta.Effort != "high" {
		t.Fatalf("CA-538: con pedido, model/effort son los de hoom (gpt-elegido, high): %q %q", meta.Model, meta.Effort)
	}
}

// CA-538 (lectura campo por campo): hoom paso el modelo y no el esfuerzo; el
// modelo es el de hoom y el esfuerzo el que informo Codex.
func TestCA538_CodexCampoPorCampo(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	cpsSesionReal(t, home)
	cpsCodex(t, 0)
	root := t.TempDir()
	_, meta := cpsCorrer(t, root, StartOptions{Provider: "codex", Prompt: "hola", Model: "gpt-elegido"})
	if meta.Model != "gpt-elegido" || meta.Effort != "low" {
		t.Fatalf("CA-538: el modelo de hoom y el esfuerzo de Codex (gpt-elegido, low): %q %q", meta.Model, meta.Effort)
	}
}

// ---------------------------------------------------------------- CA-539

// CA-539: el gasto acumulado del run de Codex suma reasoning_output_tokens y
// cache_write_input_tokens de cada invocacion, sin tocar input/cached/output.
func TestCA539_ElRunDeCodexAcumulaRazonamientoYEscritura(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	cpsCodex(t, 0)
	root := t.TempDir()
	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: "codex", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, info.ID)
	if _, err := m.Input(info.ID, "segundo"); err != nil {
		t.Fatalf("CA-539: fixture: Input reanuda el thread: %v", err)
	}
	fin := waitRun(t, m, info.ID)
	u := fin.Usage
	if u == nil || u.ReasoningTokens != 40 || u.CacheWriteTokens != 200 {
		t.Fatalf("CA-539: dos invocaciones suman razonamiento (40) y escritura en cache (200): %+v", u)
	}
	if u.InputTokens != 2400 || u.CachedTokens != 1600 || u.OutputTokens != 100 || u.Turns != 2 {
		t.Fatalf("CA-539: input/cached/output se suman como siempre, sin sumar aparte lo escrito en cache: %+v", u)
	}
	meta := cpsMetaCerrado(t, root, info.ID)
	if meta.Usage == nil || meta.Usage.ReasoningTokens != 40 || meta.Usage.CacheWriteTokens != 200 {
		t.Fatalf("CA-539: el sidecar guarda el acumulado con los dos campos: %+v", meta.Usage)
	}
	usage, _ := cpsCrudo(t, root, info.ID)["usage"].(map[string]any)
	if usage["reasoning_output_tokens"] != 40.0 || usage["cache_write_input_tokens"] != 200.0 || usage["input_tokens"] != 2400.0 {
		t.Fatalf("CA-539: el JSON del sidecar trae reasoning_output_tokens y cache_write_input_tokens: %v", usage)
	}
}

// CA-539: el run de Claude acumula cache_creation_input_tokens como
// escritura en cache; input sigue siendo input + creacion.
func TestCA539_ElRunDeClaudeAcumulaLaEscrituraEnCache(t *testing.T) {
	cpsClaude(t, []string{cpsInit, cpsResult}, []string{cpsInit, cpsResult})
	root := t.TempDir()
	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, info.ID)
	if _, err := m.Input(info.ID, "segundo"); err != nil {
		t.Fatal(err)
	}
	fin := waitRun(t, m, info.ID)
	u := fin.Usage
	if u == nil || u.CacheWriteTokens != 2*7026 || u.InputTokens != 2*7028 || u.CachedTokens != 2*24141 || u.OutputTokens != 8 {
		t.Fatalf("CA-539: Claude acumula escritura en cache (14052) dentro de input (14056): %+v", u)
	}
	if u.ReasoningTokens != 0 {
		t.Fatalf("CA-539: Claude no informa reasoning_output_tokens: %+v", u)
	}
}

// ---------------------------------------------------------------- CA-540

// CA-540: dos rate_limit_event en el run: inicio del primero, fin del
// ultimo, reinicia_en del ultimo, utilization pasada a %; un salto de 1 s en
// resetsAt no marca reiniciada. El run termina igual.
func TestCA540_ClaudeGuardaInicioFinYReiniciaEn(t *testing.T) {
	cpsClaude(t, []string{
		cpsInit,
		cpsRateLimit(0.05, cpsReset5h, 0.22, cpsReset7d),
		`{"type":"assistant","message":{"content":[{"type":"text","text":"trabajando"}]},"session_id":"` + cpsSesion + `"}`,
		cpsRateLimit(0.07, cpsReset5h+1, 0.23, cpsReset7d-1),
		cpsResult,
	})
	root := t.TempDir()
	fin, meta := cpsCorrer(t, root, StartOptions{Provider: "claude", Prompt: "hola"})
	if fin.Status != StatusDone || fin.ExitCode != 0 {
		t.Fatalf("CA-540: el run termina igual: %+v", fin)
	}
	if len(meta.Ventanas) != 2 {
		t.Fatalf("CA-540: una entrada por ventana (5h y semana): %+v", meta.Ventanas)
	}
	cinco := cpsVentana(t, "CA-540", meta, "5h")
	semana := cpsVentana(t, "CA-540", meta, "semana")
	if !cpsCasi(cinco.Inicio, 5) || !cpsCasi(cinco.Fin, 7) || !cpsCasi(semana.Inicio, 22) || !cpsCasi(semana.Fin, 23) {
		t.Fatalf("CA-540: inicio del primer evento y fin del ultimo, en %% (5->7, 22->23): %+v", meta.Ventanas)
	}
	if !cinco.ReiniciaEn.Equal(cpsEpoch(cpsReset5h+1)) || !semana.ReiniciaEn.Equal(cpsEpoch(cpsReset7d-1)) {
		t.Fatalf("CA-540: reinicia_en es el del ultimo evento: %+v", meta.Ventanas)
	}
	if cinco.Reiniciada || semana.Reiniciada {
		t.Fatalf("CA-540: un salto de 1 s en resetsAt no es un reinicio: %+v", meta.Ventanas)
	}
	if cinco.Plan != "" {
		t.Fatalf("CA-540: Claude no dice el plan: no se inventa: %+v", cinco)
	}
	crudo := cpsCrudo(t, root, fin.ID)
	vs, _ := crudo["ventanas"].([]any)
	if len(vs) != 2 {
		t.Fatalf("CA-540: el JSON trae ventanas como lista: %v", crudo["ventanas"])
	}
	for _, v := range vs {
		e, _ := v.(map[string]any)
		for _, k := range []string{"nombre", "inicio", "fin", "reinicia_en"} {
			if _, hay := e[k]; !hay {
				t.Fatalf("CA-540: cada ventana trae %q: %v", k, e)
			}
		}
		if _, hay := e["reiniciada"]; hay {
			t.Fatalf("CA-540 (enmienda 1): reiniciada se omite si no cambio: %v", e)
		}
	}
}

// CA-540: resetsAt que cambia mas de 60 s entre dos eventos marca reiniciada
// en ESA ventana, y solo en esa.
func TestCA540_UnReinicioEnMedioMarcaReiniciada(t *testing.T) {
	cpsClaude(t, []string{
		cpsInit,
		cpsRateLimit(0.97, cpsReset5h, 0.40, cpsReset7d),
		cpsRateLimit(0.02, cpsReset5h+5*3600, 0.41, cpsReset7d),
		cpsResult,
	})
	root := t.TempDir()
	fin, meta := cpsCorrer(t, root, StartOptions{Provider: "claude", Prompt: "hola"})
	cinco := cpsVentana(t, "CA-540", meta, "5h")
	semana := cpsVentana(t, "CA-540", meta, "semana")
	if !cinco.Reiniciada {
		t.Fatalf("CA-540: la 5h se reinicio en medio del run: reiniciada true: %+v", cinco)
	}
	if semana.Reiniciada {
		t.Fatalf("CA-540: la semana no se reinicio: %+v", semana)
	}
	if !cpsCasi(cinco.Inicio, 97) || !cpsCasi(cinco.Fin, 2) || !cinco.ReiniciaEn.Equal(cpsEpoch(cpsReset5h+5*3600)) {
		t.Fatalf("CA-540: inicio, fin y reinicia_en siguen siendo los del primer y el ultimo dato: %+v", cinco)
	}
	for _, v := range cpsCrudo(t, root, fin.ID)["ventanas"].([]any) {
		if e := v.(map[string]any); e["nombre"] == "5h" && e["reiniciada"] != true {
			t.Fatalf("CA-540: el JSON dice reiniciada true: %v", e)
		}
	}
}

// CA-540 (caso limite): el umbral es de 60 s: 60 exactos no es reinicio, 61
// si, hacia adelante o hacia atras.
func TestCA540_ElUmbralEsDe60Segundos(t *testing.T) {
	for _, c := range []struct {
		salto  int64
		quiero bool
	}{{1, false}, {-1, false}, {60, false}, {-60, false}, {61, true}, {-61, true}, {3600, true}} {
		cpsClaude(t, []string{
			cpsInit,
			cpsRateLimit(0.10, cpsReset5h, 0.20, cpsReset7d),
			cpsRateLimit(0.12, cpsReset5h+c.salto, 0.21, cpsReset7d),
			cpsResult,
		})
		_, meta := cpsCorrer(t, t.TempDir(), StartOptions{Provider: "claude", Prompt: "hola"})
		if v := cpsVentana(t, "CA-540", meta, "5h"); v.Reiniciada != c.quiero {
			t.Fatalf("CA-540: un salto de %d s en resetsAt: reiniciada=%v, fue %+v", c.salto, c.quiero, v)
		}
	}
}

// CA-540 (propiedad): para cualquier secuencia de eventos, inicio es el %
// del primero, fin el del ultimo, reinicia_en el del ultimo, y reiniciada es
// true si y solo si dos eventos consecutivos difieren en mas de 60 s.
func TestCA540_PropiedadReiniciada(t *testing.T) {
	r := rand.New(rand.NewSource(1540))
	saltos := []int64{0, 1, -1, 2, 30, -30, 45, 60, -60, 61, -61, 120, 5 * 3600}
	for caso := 0; caso < 14; caso++ {
		n := 2 + r.Intn(4)
		lineas := []string{cpsInit}
		reset := cpsReset5h
		var usos []float64
		reiniciada := false
		for i := 0; i < n; i++ {
			if i > 0 {
				s := saltos[r.Intn(len(saltos))]
				if s > 60 || s < -60 {
					reiniciada = true
				}
				reset += s
			}
			u := float64(r.Intn(1000)) / 1000
			usos = append(usos, u)
			lineas = append(lineas, cpsRateLimit(u, reset, 0.5, cpsReset7d))
		}
		lineas = append(lineas, cpsResult)
		cpsClaude(t, lineas)
		_, meta := cpsCorrer(t, t.TempDir(), StartOptions{Provider: "claude", Prompt: "hola"})
		v := cpsVentana(t, "CA-540", meta, "5h")
		if v.Reiniciada != reiniciada || !cpsCasi(v.Inicio, usos[0]*100) || !cpsCasi(v.Fin, usos[n-1]*100) ||
			!v.ReiniciaEn.Equal(cpsEpoch(reset)) {
			t.Fatalf("CA-540: caso %d (%v): inicio %v, fin %v, reinicia_en %v, reiniciada %v; fue %+v\n%s",
				caso, usos, usos[0]*100, usos[n-1]*100, cpsEpoch(reset), reiniciada, v, strings.Join(lineas, "\n"))
		}
		if s := cpsVentana(t, "CA-540", meta, "semana"); s.Reiniciada {
			t.Fatalf("CA-540: caso %d: la semana no cambio su reinicio: %+v", caso, s)
		}
	}
}

// CA-540: sin rate_limit_event (por ejemplo con una API key sin plan) el
// sidecar no tiene la clave ventanas.
func TestCA540_SinEventosNoHayVentanas(t *testing.T) {
	cpsClaude(t, []string{cpsInit, cpsResult})
	root := t.TempDir()
	fin, meta := cpsCorrer(t, root, StartOptions{Provider: "claude", Prompt: "hola"})
	if _, hay := cpsCrudo(t, root, fin.ID)["ventanas"]; hay || len(meta.Ventanas) != 0 {
		t.Fatalf("CA-540: sin eventos el sidecar no tiene ventanas: %+v", meta.Ventanas)
	}
	// un rate_limit_event sin unifiedWindows tampoco es una ventana
	cpsClaude(t, []string{cpsInit,
		`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","rateLimitType":"five_hour"},"session_id":"` + cpsSesion + `"}`,
		cpsResult})
	root2 := t.TempDir()
	fin2, _ := cpsCorrer(t, root2, StartOptions{Provider: "claude", Prompt: "hola"})
	if _, hay := cpsCrudo(t, root2, fin2.ID)["ventanas"]; hay {
		t.Fatalf("CA-540: un rate_limit_event sin unifiedWindows no deja ventanas: %v", cpsCrudo(t, root2, fin2.ID))
	}
}

// CA-540: el run son todas sus invocaciones: inicio es el del primer evento
// de la primera, fin y reinicia_en los del ultimo de la ultima; una ventana
// que solo aparece en un evento tiene inicio igual a fin.
func TestCA540_InicioDelPrimerDatoDelRunFinDelUltimo(t *testing.T) {
	cpsClaude(t,
		[]string{cpsInit, cpsRateLimit5h(0.05, cpsReset5h), cpsRateLimit(0.06, cpsReset5h, 0.30, cpsReset7d), cpsResult},
		[]string{cpsInit, cpsRateLimit5h(0.09, cpsReset5h+1), cpsResult},
	)
	root := t.TempDir()
	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, info.ID)
	if _, err := m.Input(info.ID, "segundo"); err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, info.ID)
	meta := cpsMetaCerrado(t, root, info.ID)
	cinco := cpsVentana(t, "CA-540", meta, "5h")
	if !cpsCasi(cinco.Inicio, 5) || !cpsCasi(cinco.Fin, 9) || !cinco.ReiniciaEn.Equal(cpsEpoch(cpsReset5h+1)) || cinco.Reiniciada {
		t.Fatalf("CA-540: 5h va del primer dato del run (5) al ultimo (9), con el reinicia_en del ultimo: %+v", cinco)
	}
	semana := cpsVentana(t, "CA-540", meta, "semana")
	if !cpsCasi(semana.Inicio, 30) || !cpsCasi(semana.Fin, 30) {
		t.Fatalf("CA-540: la semana tuvo un solo dato: inicio = fin = 30: %+v", semana)
	}
}

// ---------------------------------------------------------------- CA-541

// CA-541: al cerrar el run de Codex, hoom lee la sesion del thread en
// $CODEX_HOME/sessions: primary/secondary por window_minutes, con plan,
// inicio del primer token_count y fin y reinicia_en del ultimo; un salto de
// 1 s no es reinicio.
func TestCA541_CodexGuardaLasVentanasDeSuSesion(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	cpsSesionReal(t, home)
	cpsCodex(t, 0)
	root := t.TempDir()
	fin, meta := cpsCorrer(t, root, StartOptions{Provider: "codex", Prompt: "hola"})
	if fin.Status != StatusDone || fin.ExitCode != 0 {
		t.Fatalf("CA-541: el run termina bien: %+v", fin)
	}
	if len(meta.Ventanas) != 2 {
		t.Fatalf("CA-541: 5h y semana: %+v", meta.Ventanas)
	}
	cinco := cpsVentana(t, "CA-541", meta, "5h")
	semana := cpsVentana(t, "CA-541", meta, "semana")
	if !cpsCasi(cinco.Inicio, 52) || !cpsCasi(cinco.Fin, 54) || !cpsCasi(semana.Inicio, 30) || !cpsCasi(semana.Fin, 31) {
		t.Fatalf("CA-541: inicio del primer token_count y fin del ultimo (52->54, 30->31): %+v", meta.Ventanas)
	}
	if !cinco.ReiniciaEn.Equal(cpsEpoch(1794053320)) || !semana.ReiniciaEn.Equal(cpsEpoch(1794549883)) {
		t.Fatalf("CA-541: reinicia_en del ultimo token_count: %+v", meta.Ventanas)
	}
	if cinco.Reiniciada || semana.Reiniciada {
		t.Fatalf("CA-541: resets_at que salta 1 s (Codex lo hace) no es reinicio: %+v", meta.Ventanas)
	}
	if cinco.Plan != "plus" || semana.Plan != "plus" {
		t.Fatalf("CA-541: plan_type es plan: %+v", meta.Ventanas)
	}
	if meta.Usage == nil || meta.Usage.InputTokens != 1200 {
		t.Fatalf("CA-541: el gasto del run sigue como siempre: %+v", meta.Usage)
	}
}

// CA-541 (caso limite): los reportes del thread anteriores al arranque del
// run (otra corrida del mismo thread) no cuentan.
func TestCA541_ReportesAnterioresAlRunNoCuentan(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	antes, despues := time.Now().Add(-time.Hour), time.Now().Add(time.Hour)
	cpsSesionCodex(t, home, cpsThread,
		cpsSessionMeta(antes, cpsThread),
		cpsTurnContext(antes.Add(time.Second), "gpt-viejo", "high"),
		cpsTokenCount(antes.Add(2*time.Second), &cpsRL{10, 300, 1794053319}, nil, "plus"),
		cpsTurnContext(despues, "gpt-5.6-sol", "low"),
		cpsTokenCount(despues.Add(time.Second), &cpsRL{52, 300, 1794053319}, nil, "plus"),
		cpsTokenCount(despues.Add(2*time.Second), &cpsRL{53, 300, 1794053319}, nil, "plus"),
	)
	cpsCodex(t, 0)
	_, meta := cpsCorrer(t, t.TempDir(), StartOptions{Provider: "codex", Prompt: "hola"})
	cinco := cpsVentana(t, "CA-541", meta, "5h")
	if !cpsCasi(cinco.Inicio, 52) || !cpsCasi(cinco.Fin, 53) {
		t.Fatalf("CA-541: el reporte anterior al run (10) no cuenta: %+v", cinco)
	}
	if meta.Model != "gpt-5.6-sol" || meta.Effort != "low" {
		t.Fatalf("CA-541: el turn_context anterior al run no cuenta: %q %q", meta.Model, meta.Effort)
	}
}

// CA-541: reiniciada con la misma regla que Claude: mas de 60 s entre dos
// token_count consecutivos; otra ventana (60 min) se nombra 60min.
func TestCA541_ReiniciadaYOtraVentana(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	d := time.Now().Add(time.Hour)
	cpsSesionCodex(t, home, cpsThread,
		cpsSessionMeta(d, cpsThread),
		cpsTokenCount(d.Add(time.Second), &cpsRL{98, 300, 1794053319}, &cpsRL{5, 60, 1794050000}, "plus"),
		cpsTokenCount(d.Add(2*time.Second), &cpsRL{1, 300, 1794053319 + 5*3600}, &cpsRL{6, 60, 1794050001}, "plus"),
	)
	cpsCodex(t, 0)
	_, meta := cpsCorrer(t, t.TempDir(), StartOptions{Provider: "codex", Prompt: "hola"})
	cinco := cpsVentana(t, "CA-541", meta, "5h")
	hora := cpsVentana(t, "CA-541", meta, "60min")
	if !cinco.Reiniciada || !cpsCasi(cinco.Inicio, 98) || !cpsCasi(cinco.Fin, 1) {
		t.Fatalf("CA-541: la 5h se reinicio en medio: reiniciada true: %+v", cinco)
	}
	if hora.Reiniciada || !cpsCasi(hora.Inicio, 5) || !cpsCasi(hora.Fin, 6) {
		t.Fatalf("CA-541: window_minutes 60 se nombra 60min y salta 1 s: no reiniciada: %+v", hora)
	}
}

// CA-541 (caso limite): con el archivo de sesion ausente o ilegible el run
// termina con el mismo resultado (status, exit, gasto), sin ventanas, y model
// es el que paso hoom si paso uno.
func TestCA541_SesionAusenteOIlegibleNoCambiaElResultado(t *testing.T) {
	casos := map[string]func(t *testing.T, home string){
		"sin archivo": func(t *testing.T, home string) {},
		"otro CODEX_HOME": func(t *testing.T, home string) {
			cpsSesionReal(t, t.TempDir())
		},
		"basura": func(t *testing.T, home string) {
			cpsSesionCodex(t, home, cpsThread, "\x00\x01 {{{ no es jsonl", `{"type":`)
		},
		"sin permiso": func(t *testing.T, home string) {
			if os.Geteuid() == 0 {
				t.Skip("como root un archivo sin permiso se lee igual")
			}
			cpsSesionReal(t, home)
			filepath.Walk(home, func(p string, fi os.FileInfo, err error) error {
				if err == nil && !fi.IsDir() {
					os.Chmod(p, 0o000)
				}
				return nil
			})
			t.Cleanup(func() {
				filepath.Walk(home, func(p string, fi os.FileInfo, err error) error {
					if err == nil && !fi.IsDir() {
						os.Chmod(p, 0o644)
					}
					return nil
				})
			})
		},
	}
	for _, exit := range []int{0, 3} {
		// la referencia: el mismo codex con una sesion legible
		hRef := t.TempDir()
		t.Setenv("CODEX_HOME", hRef)
		cpsSesionReal(t, hRef)
		cpsCodex(t, exit)
		ref, refMeta := cpsCorrer(t, t.TempDir(), StartOptions{Provider: "codex", Prompt: "hola"})
		for nombre, armar := range casos {
			for _, modelo := range []string{"", "gpt-elegido"} {
				t.Run(fmt.Sprintf("%s exit %d modelo %q", nombre, exit, modelo), func(t *testing.T) {
					home := t.TempDir()
					t.Setenv("CODEX_HOME", home)
					armar(t, home)
					cpsCodex(t, exit)
					root := t.TempDir()
					fin, meta := cpsCorrer(t, root, StartOptions{Provider: "codex", Prompt: "hola", Model: modelo})
					if fin.Status != ref.Status || fin.ExitCode != ref.ExitCode || meta.Status != refMeta.Status || meta.ExitCode != refMeta.ExitCode {
						t.Fatalf("CA-541: %s: el run termina con el mismo resultado (%s/%d), fue %s/%d", nombre, ref.Status, ref.ExitCode, fin.Status, fin.ExitCode)
					}
					if meta.Usage == nil || refMeta.Usage == nil || *meta.Usage != *refMeta.Usage {
						t.Fatalf("CA-541: %s: el gasto del run no cambia: %+v vs %+v", nombre, meta.Usage, refMeta.Usage)
					}
					crudo := cpsCrudo(t, root, fin.ID)
					if _, hay := crudo["ventanas"]; hay {
						t.Fatalf("CA-541: %s: sin sesion legible no hay ventanas: %v", nombre, crudo["ventanas"])
					}
					if meta.Model != modelo {
						t.Fatalf("CA-541: %s: model es el que paso hoom (%q): %q", nombre, modelo, meta.Model)
					}
					if _, hay := crudo["model"]; hay != (modelo != "") {
						t.Fatalf("CA-541: %s: sin modelo pasado ni informado, model se omite: %v", nombre, crudo)
					}
				})
			}
		}
	}
}

// CA-541 (caso limite): dos archivos con el thread en el nombre; vale el que
// tiene session_meta.id igual al thread.
func TestCA541_DosArchivosValeElDelSessionMeta(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	d := time.Now().Add(time.Hour)
	ahora := time.Now()
	dir := filepath.Join(home, "sessions", ahora.Format("2006"), ahora.Format("01"), ahora.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	escribir := func(nombre string, lineas ...string) {
		if err := os.WriteFile(filepath.Join(dir, nombre), []byte(strings.Join(lineas, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// el ajeno se lista antes y despues del propio
	escribir("rollout-"+ahora.Add(-time.Minute).Format("2006-01-02T15-04-05")+"-"+cpsThread+".jsonl",
		cpsSessionMeta(d, "99999999-0000-0000-0000-000000000001"),
		cpsTurnContext(d, "gpt-ajeno", "high"),
		cpsTokenCount(d.Add(time.Second), &cpsRL{99, 300, 1794053319}, nil, "pro"))
	escribir("rollout-"+ahora.Format("2006-01-02T15-04-05")+"-"+cpsThread+".jsonl",
		cpsSessionMeta(d, cpsThread),
		cpsTurnContext(d, "gpt-5.6-sol", "low"),
		cpsTokenCount(d.Add(time.Second), &cpsRL{52, 300, 1794053319}, nil, "plus"))
	escribir("rollout-"+ahora.Add(time.Minute).Format("2006-01-02T15-04-05")+"-"+cpsThread+".jsonl",
		cpsSessionMeta(d, "99999999-0000-0000-0000-000000000002"),
		cpsTurnContext(d, "gpt-ajeno2", "high"),
		cpsTokenCount(d.Add(time.Second), &cpsRL{77, 300, 1794053319}, nil, "pro"))
	cpsCodex(t, 0)
	_, meta := cpsCorrer(t, t.TempDir(), StartOptions{Provider: "codex", Prompt: "hola"})
	cinco := cpsVentana(t, "CA-541", meta, "5h")
	if !cpsCasi(cinco.Inicio, 52) || cinco.Plan != "plus" || meta.Model != "gpt-5.6-sol" {
		t.Fatalf("CA-541: vale el archivo cuyo session_meta.id es el thread: %+v %q", cinco, meta.Model)
	}
}

// CA-541: sin CODEX_HOME, la sesion se lee de ~/.codex/sessions.
func TestCA541_SinCodexHomeLeeDeHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	os.Unsetenv("CODEX_HOME")
	if providers.CodexHome() != filepath.Join(home, ".codex") {
		t.Fatalf("CA-541: fixture: CodexHome es ~/.codex: %q", providers.CodexHome())
	}
	cpsSesionReal(t, filepath.Join(home, ".codex"))
	cpsCodex(t, 0)
	_, meta := cpsCorrer(t, t.TempDir(), StartOptions{Provider: "codex", Prompt: "hola"})
	if cinco := cpsVentana(t, "CA-541", meta, "5h"); !cpsCasi(cinco.Inicio, 52) {
		t.Fatalf("CA-541: por defecto ~/.codex/sessions: %+v", cinco)
	}
}
