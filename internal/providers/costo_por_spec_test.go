// Tests adversariales del spec .hoom/specs/costo-por-spec.md (CA-538,
// CA-539, CA-540, CA-541) sobre los adapters: lo que cada CLI dice de su
// modelo, de su esfuerzo y de las ventanas del plan, y los dos campos nuevos
// de Usage. Escritos DESDE EL SPEC y las firmas publicas (Medidor, Medicion,
// Ventana, CodexHome, Usage), sin leer la implementacion.
//
// Las lineas son las de los formatos verificados que fija el spec: Claude
// Code 2.1.319 (stream-json) y Codex 0.160.0 (exec --json y el archivo de
// sesion $CODEX_HOME/sessions/YYYY/MM/DD/rollout-<fecha>-<thread>.jsonl).
package providers

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
	"testing/quick"
	"time"
)

// cpsThread es el thread de Codex de los fixtures (el de la corrida real).
const cpsThread = "01a1b4f6-3082-7b73-ba58-8ae399748f7f"

// cpsInitClaude es el system/init real de Claude Code 2.1.319.
const cpsInitClaude = `{"type":"system","subtype":"init","model":"claude-opus-5-5","session_id":"c449dc75-cc1c-4f32-9863-160dda6baa4f"}`

// cpsResultClaude es el result real de Claude Code 2.1.319.
const cpsResultClaude = `{"type":"result","subtype":"success","is_error":false,"num_turns":1,"duration_ms":5019,` +
	`"total_cost_usd":0.0611242,"usage":{"input_tokens":2,"cache_creation_input_tokens":7026,` +
	`"cache_read_input_tokens":24141,"output_tokens":4,"output_tokens_details":{"thinking_tokens":0}},` +
	`"result":"hola","session_id":"c449dc75-cc1c-4f32-9863-160dda6baa4f"}`

// cpsTurnCompletedCodex es el turn.completed real de Codex 0.160.0.
const cpsTurnCompletedCodex = `{"type":"turn.completed","usage":{"input_tokens":1200,"cached_input_tokens":800,` +
	`"cache_write_input_tokens":100,"output_tokens":50,"reasoning_output_tokens":20}}`

// cpsF formatea un float como lo escribiria un JSON (sin notacion rara).
func cpsF(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// cpsRateLimit arma un rate_limit_event de Claude con la forma real, con las
// dos ventanas (utilization en fraccion, resetsAt en epoch segundos).
func cpsRateLimit(u5 float64, r5 int64, u7 float64, r7 int64) string {
	return `{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","resetsAt":` + strconv.FormatInt(r5, 10) +
		`,"rateLimitType":"five_hour","overageStatus":"rejected","overageDisabledReason":"out_of_credits",` +
		`"isUsingOverage":false,"unifiedWindows":{"five_hour":{"utilization":` + cpsF(u5) + `,"resetsAt":` +
		strconv.FormatInt(r5, 10) + `,"status":"allowed"},"seven_day":{"utilization":` + cpsF(u7) +
		`,"resetsAt":` + strconv.FormatInt(r7, 10) + `,"status":"allowed"}}},` +
		`"uuid":"428e0ecf-84ef-4197-a967-7a7680f9a81b","session_id":"c449dc75-cc1c-4f32-9863-160dda6baa4f"}`
}

func cpsMedidor(t *testing.T, name string) Medidor {
	t.Helper()
	p, err := Lookup(name)
	if err != nil {
		t.Fatalf("Lookup(%s): %v", name, err)
	}
	m, ok := p.(Medidor)
	if !ok {
		t.Fatalf("CA-538: el adapter %s implementa Medidor (spec costo-por-spec)", name)
	}
	return m
}

func cpsCasi(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func cpsVentana(vs []Ventana, nombre string) (Ventana, bool) {
	for _, v := range vs {
		if v.Nombre == nombre {
			return v, true
		}
	}
	return Ventana{}, false
}

// cpsSinVentanas exige que la medicion no traiga ninguna ventana.
func cpsSinVentanas(m Medicion) bool {
	for _, s := range m.Muestras {
		if len(s) != 0 {
			return false
		}
	}
	return true
}

// ------------------------------------------------- archivo de sesion de Codex

func cpsTS(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }

func cpsSessionMeta(ts time.Time, id string) string {
	return `{"timestamp":"` + cpsTS(ts) + `","type":"session_meta","payload":{"id":"` + id + `","cli_version":"0.160.0"}}`
}

func cpsTurnContext(ts time.Time, model, effort string) string {
	return `{"timestamp":"` + cpsTS(ts) + `","type":"turn_context","payload":{"model":"` + model + `","effort":"` + effort + `"}}`
}

// cpsRL es una ventana de rate_limits de Codex.
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

// cpsEscribirSesion deja el archivo de sesion de Codex donde Codex lo deja:
// <home>/sessions/YYYY/MM/DD/rollout-<YYYY-MM-DDThh-mm-ss>-<thread>.jsonl.
func cpsEscribirSesion(t *testing.T, home string, cuando time.Time, thread string, lineas ...string) string {
	t.Helper()
	dir := filepath.Join(home, "sessions", cuando.Format("2006"), cuando.Format("01"), cuando.Format("02"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "rollout-"+cuando.Format("2006-01-02T15-04-05")+"-"+thread+".jsonl")
	if err := os.WriteFile(p, []byte(strings.Join(lineas, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// cpsT0 es el mediodia UTC del dia del fixture: la misma fecha local en casi
// cualquier huso, asi el directorio YYYY/MM/DD no depende de donde corre.
var cpsT0 = time.Date(2026, 11, 7, 12, 0, 0, 0, time.UTC)

// ---------------------------------------------------------------- CA-538

// CA-538: Claude dice su modelo en el system/init; esa linea no trae
// ventanas ni esfuerzo.
func TestCA538_ClaudeDiceSuModeloEnElInit(t *testing.T) {
	m := cpsMedidor(t, "claude").MedirLinea(cpsInitClaude)
	if m.Model != "claude-opus-5-5" {
		t.Fatalf("CA-538: el system/init de Claude dice el modelo claude-opus-5-5: %+v", m)
	}
	if m.Effort != "" || !cpsSinVentanas(m) {
		t.Fatalf("CA-538: el init no dice esfuerzo ni ventanas: %+v", m)
	}
}

// CA-538: Codex dice modelo y esfuerzo en el turn_context de su sesion; vale
// el ultimo dentro de lo que se mide.
func TestCA538_CodexDiceModeloYEsfuerzoEnElTurnContext(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	cpsEscribirSesion(t, home, cpsT0, cpsThread,
		cpsSessionMeta(cpsT0, cpsThread),
		cpsTurnContext(cpsT0.Add(time.Second), "gpt-5.5", "high"),
		cpsTurnContext(cpsT0.Add(2*time.Second), "gpt-5.6-sol", "low"),
	)
	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, cpsT0)
	if err != nil {
		t.Fatalf("CA-538: la sesion existe y se lee: %v", err)
	}
	if m.Model != "gpt-5.6-sol" || m.Effort != "low" {
		t.Fatalf("CA-538: model/effort son los del ultimo turn_context (gpt-5.6-sol, low): %+v", m)
	}
}

// CA-538 (caso hostil): un turn_context anterior a desde es de otra corrida
// del mismo thread: no dice con que corrio esta.
func TestCA538_TurnContextAnteriorADesdeNoCuenta(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	desde := cpsT0.Add(time.Hour)
	cpsEscribirSesion(t, home, cpsT0, cpsThread,
		cpsSessionMeta(cpsT0, cpsThread),
		cpsTurnContext(cpsT0.Add(time.Second), "gpt-viejo", "high"),
	)
	m, _ := cpsMedidor(t, "codex").MedirSesion(cpsThread, desde)
	if m.Model != "" || m.Effort != "" {
		t.Fatalf("CA-538: un turn_context anterior a desde no cuenta: %+v", m)
	}

	home2 := t.TempDir()
	t.Setenv("CODEX_HOME", home2)
	cpsEscribirSesion(t, home2, cpsT0, cpsThread,
		cpsSessionMeta(cpsT0, cpsThread),
		cpsTurnContext(cpsT0.Add(time.Second), "gpt-viejo", "high"),
		cpsTurnContext(desde.Add(time.Second), "gpt-5.6-sol", "low"),
		cpsTurnContext(desde.Add(-time.Second), "gpt-casi", "medium"),
	)
	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, desde)
	if err != nil {
		t.Fatal(err)
	}
	if m.Model != "gpt-5.6-sol" || m.Effort != "low" {
		t.Fatalf("CA-538: solo cuenta el turn_context con timestamp >= desde: %+v", m)
	}
}

// ---------------------------------------------------------------- CA-539

// CA-539: Codex trae reasoning_output_tokens y cache_write_input_tokens en
// turn.completed; los demas campos no cambian (input incluye lo cacheado y
// lo escrito en cache: nada se suma aparte).
func TestCA539_CodexRazonamientoYEscrituraEnCache(t *testing.T) {
	p, _ := Lookup("codex")
	evs := p.Normalize(cpsTurnCompletedCodex)
	if len(evs) != 1 || evs[0].Kind != "end" || evs[0].Usage == nil {
		t.Fatalf("CA-539: turn.completed cierra con usage: %+v", evs)
	}
	u := evs[0].Usage
	if u.ReasoningTokens != 20 || u.CacheWriteTokens != 100 {
		t.Fatalf("CA-539: reasoning_output_tokens 20 y cache_write_input_tokens 100: %+v", u)
	}
	if u.InputTokens != 1200 || u.CachedTokens != 800 || u.OutputTokens != 50 || u.Turns != 1 || u.CostUSD != nil {
		t.Fatalf("CA-539: input/cached/output no cambian (1200/800/50, 1 turno, sin costo): %+v", u)
	}
}

// CA-539: Claude informa cache_creation_input_tokens; es cache_write y es un
// SUBCONJUNTO de InputTokens, que sigue siendo input + creacion (CA-197).
func TestCA539_ClaudeEscrituraEnCacheEsLaCreacion(t *testing.T) {
	p, _ := Lookup("claude")
	evs := p.Normalize(cpsResultClaude)
	if len(evs) != 1 || evs[0].Kind != "end" || evs[0].Usage == nil {
		t.Fatalf("CA-539: el result cierra con usage: %+v", evs)
	}
	u := evs[0].Usage
	if u.CacheWriteTokens != 7026 {
		t.Fatalf("CA-539: cache_write_input_tokens de Claude es cache_creation_input_tokens (7026): %+v", u)
	}
	if u.InputTokens != 2+7026 || u.CachedTokens != 24141 || u.OutputTokens != 4 {
		t.Fatalf("CA-539: input (2+7026), cached (24141) y output (4) no cambian: %+v", u)
	}
	if u.ReasoningTokens != 0 {
		t.Fatalf("CA-539: reasoning_output_tokens es de Codex; Claude no lo informa aca: %+v", u)
	}
	if u.CostUSD == nil || !cpsCasi(*u.CostUSD, 0.0611242) || u.Turns != 1 || u.DurationMS != 5019 {
		t.Fatalf("CA-539: costo, turnos y duracion siguen como estaban: %+v", u)
	}
}

// CA-539 (propiedad): para cualquier turn.completed de Codex, los cinco
// numeros pasan tal cual, y lo escrito en cache nunca se suma a la entrada.
func TestCA539_PropiedadCodexPasaLosCincoNumeros(t *testing.T) {
	p, _ := Lookup("codex")
	f := func(in, cached, cw, out, reas uint16) bool {
		i := int(in) + 1
		c := int(cached) % i
		w := int(cw) % i
		line := fmt.Sprintf(`{"type":"turn.completed","usage":{"input_tokens":%d,"cached_input_tokens":%d,`+
			`"cache_write_input_tokens":%d,"output_tokens":%d,"reasoning_output_tokens":%d}}`, i, c, w, out, reas)
		evs := p.Normalize(line)
		if len(evs) != 1 || evs[0].Usage == nil {
			return false
		}
		u := evs[0].Usage
		return u.InputTokens == i && u.CachedTokens == c && u.CacheWriteTokens == w &&
			u.OutputTokens == int(out) && u.ReasoningTokens == int(reas)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(538))}); err != nil {
		t.Fatalf("CA-539: %v", err)
	}
}

// CA-539 (propiedad): para cualquier result de Claude, cache_write es la
// creacion de cache, input sigue siendo input + creacion y cached lo leido.
func TestCA539_PropiedadClaudeEscrituraEsSubconjunto(t *testing.T) {
	p, _ := Lookup("claude")
	f := func(in, creacion, leido, out uint16) bool {
		line := fmt.Sprintf(`{"type":"result","subtype":"success","is_error":false,"num_turns":1,"result":"ok",`+
			`"session_id":"s","usage":{"input_tokens":%d,"cache_creation_input_tokens":%d,`+
			`"cache_read_input_tokens":%d,"output_tokens":%d}}`, int(in)+1, creacion, leido, out)
		evs := p.Normalize(line)
		if len(evs) != 1 || evs[0].Usage == nil {
			return false
		}
		u := evs[0].Usage
		return u.CacheWriteTokens == int(creacion) && u.InputTokens == int(in)+1+int(creacion) &&
			u.CachedTokens == int(leido) && u.OutputTokens == int(out) && u.CacheWriteTokens <= u.InputTokens
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(539))}); err != nil {
		t.Fatalf("CA-539: %v", err)
	}
}

// CA-539: el JSON de Usage gana reasoning_output_tokens y
// cache_write_input_tokens y conserva las claves de siempre.
func TestCA539_ElJSONDeUsageGanaDosClaves(t *testing.T) {
	raw, err := json.Marshal(Usage{InputTokens: 1200, CachedTokens: 800, OutputTokens: 50, ReasoningTokens: 20, CacheWriteTokens: 100})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	quiero := map[string]float64{"input_tokens": 1200, "cached_input_tokens": 800, "output_tokens": 50,
		"reasoning_output_tokens": 20, "cache_write_input_tokens": 100}
	for k, v := range quiero {
		if m[k] != v {
			t.Fatalf("CA-539: el JSON de Usage trae %s=%v: %s", k, v, raw)
		}
	}
}

// CA-539 (propiedad): el texto de Summary no cambia con los dos campos
// nuevos (datos-en-vez-de-prosa fija esa linea).
func TestCA539_PropiedadSummaryNoCambia(t *testing.T) {
	f := func(turns, in, cached, out, dur, reas, cw uint16, conCosto bool, costo uint16) bool {
		base := Usage{Turns: int(turns), InputTokens: int(in), CachedTokens: int(cached), OutputTokens: int(out), DurationMS: int(dur)}
		if conCosto {
			c := float64(costo) / 1000
			base.CostUSD = &c
		}
		otro := base
		otro.ReasoningTokens = int(reas)
		otro.CacheWriteTokens = int(cw)
		return base.Summary() == otro.Summary()
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(1539))}); err != nil {
		t.Fatalf("CA-539: Summary no cambia con reasoning ni cache_write: %v", err)
	}
}

// ---------------------------------------------------------------- CA-540

// CA-540: el rate_limit_event real de Claude da UN reporte con 5h y semana,
// utilization pasada a % (0.05 -> 5) y resetsAt en UTC.
func TestCA540_ClaudeRateLimitEventDaLasDosVentanas(t *testing.T) {
	m := cpsMedidor(t, "claude").MedirLinea(cpsRateLimit(0.05, 1794078000, 0.22, 1794517200))
	if len(m.Muestras) != 1 {
		t.Fatalf("CA-540: un rate_limit_event es un reporte: %+v", m)
	}
	cinco, ok5 := cpsVentana(m.Muestras[0], "5h")
	semana, ok7 := cpsVentana(m.Muestras[0], "semana")
	if !ok5 || !ok7 || len(m.Muestras[0]) != 2 {
		t.Fatalf("CA-540: five_hour es 5h y seven_day es semana: %+v", m.Muestras[0])
	}
	if !cpsCasi(cinco.Usado, 5) || !cpsCasi(semana.Usado, 22) {
		t.Fatalf("CA-540: utilization en fraccion pasa a %% (5 y 22): %+v", m.Muestras[0])
	}
	if !cinco.ReiniciaEn.Equal(time.Unix(1794078000, 0)) || !semana.ReiniciaEn.Equal(time.Unix(1794517200, 0)) {
		t.Fatalf("CA-540: resetsAt (epoch s) es la hora del reinicio: %+v", m.Muestras[0])
	}
	if cinco.ReiniciaEn.Location() != time.UTC {
		t.Fatalf("CA-540: reinicia_en en UTC: %v", cinco.ReiniciaEn.Location())
	}
	if m.Model != "" || m.Effort != "" {
		t.Fatalf("CA-540: el rate_limit_event no dice modelo: %+v", m)
	}
}

// CA-540 (propiedad): cualquier utilization en fraccion se guarda *100, sin
// redondear.
func TestCA540_PropiedadUtilizationAPorciento(t *testing.T) {
	med := cpsMedidor(t, "claude")
	r := rand.New(rand.NewSource(540))
	for i := 0; i < 300; i++ {
		u5 := r.Float64()
		u7 := float64(r.Intn(100001)) / 100000
		m := med.MedirLinea(cpsRateLimit(u5, 1794078000, u7, 1794517200))
		if len(m.Muestras) != 1 {
			t.Fatalf("CA-540: un reporte por evento (%v, %v): %+v", u5, u7, m)
		}
		cinco, _ := cpsVentana(m.Muestras[0], "5h")
		semana, _ := cpsVentana(m.Muestras[0], "semana")
		if !cpsCasi(cinco.Usado, u5*100) || !cpsCasi(semana.Usado, u7*100) {
			t.Fatalf("CA-540: utilization %v y %v pasan a %v y %v sin redondear: %+v", u5, u7, u5*100, u7*100, m.Muestras[0])
		}
	}
}

// CA-540 (caso hostil): una linea que no dice nada de modelo ni de ventanas
// da la Medicion vacia: el result, un texto del agente, un rate_limit_event
// sin unifiedWindows (por ejemplo con una API key sin plan), basura.
func TestCA540_LineasQueNoDicenNada(t *testing.T) {
	med := cpsMedidor(t, "claude")
	for _, l := range []string{
		cpsResultClaude,
		`{"type":"assistant","message":{"content":[{"type":"text","text":"five_hour utilization 0.9 model x"}]},"session_id":"s"}`,
		`{"type":"rate_limit_event","rate_limit_info":{"status":"allowed","rateLimitType":"five_hour"},"session_id":"s"}`,
		`{"type":"system","subtype":"hook_response","session_id":"s"}`,
		`no es json`,
		``,
	} {
		m := med.MedirLinea(l)
		if m.Model != "" || m.Effort != "" || !cpsSinVentanas(m) {
			t.Fatalf("CA-540: %q no dice modelo ni ventanas: %+v", l, m)
		}
	}
}

// CA-540: Claude no tiene archivo de sesion que hoom lea: MedirSesion da la
// Medicion vacia y ningun error.
func TestCA540_ClaudeNoTieneArchivoDeSesion(t *testing.T) {
	m, err := cpsMedidor(t, "claude").MedirSesion("c449dc75-cc1c-4f32-9863-160dda6baa4f", cpsT0)
	if err != nil || m.Model != "" || !cpsSinVentanas(m) {
		t.Fatalf("CA-540: Claude sin archivo de sesion: medicion vacia, sin error: %+v %v", m, err)
	}
}

// ---------------------------------------------------------------- CA-541

// CA-541: el archivo de sesion real de Codex da las ventanas por
// window_minutes (300 = 5h, 10080 = semana), con used_percent tal cual,
// resets_at en UTC y plan_type como plan; modelo y esfuerzo del turn_context.
func TestCA541_CodexLeeLaSesionDelThread(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	cpsEscribirSesion(t, home, cpsT0, cpsThread,
		cpsSessionMeta(cpsT0, cpsThread),
		cpsTurnContext(cpsT0.Add(time.Second), "gpt-5.6-sol", "low"),
		cpsTokenCount(cpsT0.Add(3*time.Second), &cpsRL{52.0, 300, 1794053319}, &cpsRL{30.0, 10080, 1794549884}, "plus"),
	)
	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, cpsT0)
	if err != nil {
		t.Fatalf("CA-541: %v", err)
	}
	if m.Model != "gpt-5.6-sol" || m.Effort != "low" {
		t.Fatalf("CA-541: model y effort del turn_context: %+v", m)
	}
	if len(m.Muestras) != 1 || len(m.Muestras[0]) != 2 {
		t.Fatalf("CA-541: un token_count es un reporte con primary y secondary: %+v", m.Muestras)
	}
	cinco, ok5 := cpsVentana(m.Muestras[0], "5h")
	semana, ok7 := cpsVentana(m.Muestras[0], "semana")
	if !ok5 || !ok7 {
		t.Fatalf("CA-541: 300 min es 5h y 10080 min es semana: %+v", m.Muestras[0])
	}
	if !cpsCasi(cinco.Usado, 52) || !cpsCasi(semana.Usado, 30) {
		t.Fatalf("CA-541: used_percent ya viene en %%: %+v", m.Muestras[0])
	}
	if !cinco.ReiniciaEn.Equal(time.Unix(1794053319, 0)) || !semana.ReiniciaEn.Equal(time.Unix(1794549884, 0)) ||
		cinco.ReiniciaEn.Location() != time.UTC {
		t.Fatalf("CA-541: resets_at (epoch s) en UTC: %+v", m.Muestras[0])
	}
	if cinco.Plan != "plus" || semana.Plan != "plus" {
		t.Fatalf("CA-541: plan_type es el plan: %+v", m.Muestras[0])
	}
}

// CA-541: otro window_minutes se nombra <n>min; una ventana null no es
// ventana.
func TestCA541_NombreDeLaVentanaPorWindowMinutes(t *testing.T) {
	for _, c := range []struct {
		minutos int
		nombre  string
	}{{300, "5h"}, {10080, "semana"}, {60, "60min"}, {1440, "1440min"}, {301, "301min"}} {
		home := t.TempDir()
		t.Setenv("CODEX_HOME", home)
		cpsEscribirSesion(t, home, cpsT0, cpsThread,
			cpsSessionMeta(cpsT0, cpsThread),
			cpsTokenCount(cpsT0.Add(time.Second), &cpsRL{7, c.minutos, 1794053319}, nil, "plus"),
		)
		m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, cpsT0)
		if err != nil {
			t.Fatal(err)
		}
		if len(m.Muestras) != 1 || len(m.Muestras[0]) != 1 || m.Muestras[0][0].Nombre != c.nombre {
			t.Fatalf("CA-541: window_minutes %d se nombra %q, y secondary null no es ventana: %+v", c.minutos, c.nombre, m.Muestras)
		}
	}
}

// CA-541: cada token_count es un reporte, del mas viejo al mas nuevo, y solo
// cuentan los de timestamp >= desde (el de desde exacto cuenta).
func TestCA541_SoloCuentanLosReportesDesde(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	desde := cpsT0.Add(10 * time.Minute)
	cpsEscribirSesion(t, home, cpsT0, cpsThread,
		cpsSessionMeta(cpsT0, cpsThread),
		cpsTokenCount(cpsT0.Add(time.Second), &cpsRL{10, 300, 1794053319}, &cpsRL{20, 10080, 1794549884}, "plus"),
		cpsTokenCount(desde.Add(-time.Millisecond), &cpsRL{11, 300, 1794053319}, &cpsRL{21, 10080, 1794549884}, "plus"),
		cpsTokenCount(desde, &cpsRL{52, 300, 1794053319}, &cpsRL{30, 10080, 1794549884}, "plus"),
		cpsTokenCount(desde.Add(time.Minute), &cpsRL{53, 300, 1794053320}, &cpsRL{30, 10080, 1794549884}, "plus"),
		cpsTokenCount(desde.Add(2*time.Minute), &cpsRL{55, 300, 1794053319}, &cpsRL{31, 10080, 1794549885}, "plus"),
	)
	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, desde)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Muestras) != 3 {
		t.Fatalf("CA-541: solo los 3 reportes con timestamp >= desde (el exacto incluido): %+v", m.Muestras)
	}
	for i, quiero := range []float64{52, 53, 55} {
		v, ok := cpsVentana(m.Muestras[i], "5h")
		if !ok || !cpsCasi(v.Usado, quiero) {
			t.Fatalf("CA-541: el reporte %d (del mas viejo al mas nuevo) dice 5h %v: %+v", i, quiero, m.Muestras)
		}
	}
}

// CA-541 (caso limite): dos archivos con el thread en el nombre; vale el que
// tiene session_meta.id igual al thread, sea cual sea el que se lista
// primero.
func TestCA541_DosArchivosValeElDelSessionMeta(t *testing.T) {
	for _, ajenoPrimero := range []bool{true, false} {
		home := t.TempDir()
		t.Setenv("CODEX_HOME", home)
		ajeno, propio := cpsT0, cpsT0.Add(time.Hour)
		if !ajenoPrimero {
			ajeno, propio = propio, ajeno
		}
		cpsEscribirSesion(t, home, ajeno, cpsThread,
			cpsSessionMeta(ajeno, "99999999-0000-0000-0000-000000000000"),
			cpsTurnContext(ajeno.Add(time.Second), "gpt-ajeno", "high"),
			cpsTokenCount(ajeno.Add(2*time.Second), &cpsRL{99, 300, 1794053319}, nil, "pro"),
		)
		cpsEscribirSesion(t, home, propio, cpsThread,
			cpsSessionMeta(propio, cpsThread),
			cpsTurnContext(propio.Add(time.Second), "gpt-5.6-sol", "low"),
			cpsTokenCount(propio.Add(2*time.Second), &cpsRL{52, 300, 1794053319}, nil, "plus"),
		)
		m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, cpsT0.Add(-time.Hour))
		if err != nil {
			t.Fatal(err)
		}
		if m.Model != "gpt-5.6-sol" || len(m.Muestras) != 1 || len(m.Muestras[0]) != 1 ||
			!cpsCasi(m.Muestras[0][0].Usado, 52) || m.Muestras[0][0].Plan != "plus" {
			t.Fatalf("CA-541: vale el archivo cuyo session_meta.id es el thread (ajeno primero=%v): %+v", ajenoPrimero, m)
		}
	}
}

// CA-541 (caso limite): archivo de sesion ausente (otro CODEX_HOME, borrado)
// o ilegible (basura, sin permiso, un directorio): la Medicion queda vacia,
// con error o sin el.
func TestCA541_SesionAusenteOIlegibleDaMedicionVacia(t *testing.T) {
	med := cpsMedidor(t, "codex")
	casos := map[string]func(t *testing.T, home string){
		"sin CODEX_HOME": func(t *testing.T, home string) {},
		"sin el archivo del thread": func(t *testing.T, home string) {
			cpsEscribirSesion(t, home, cpsT0, "otro-thread",
				cpsSessionMeta(cpsT0, "otro-thread"),
				cpsTokenCount(cpsT0.Add(time.Second), &cpsRL{52, 300, 1794053319}, nil, "plus"))
		},
		"basura": func(t *testing.T, home string) {
			p := cpsEscribirSesion(t, home, cpsT0, cpsThread)
			if err := os.WriteFile(p, []byte("\x00\x01 no es jsonl {{{\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"sin permiso": func(t *testing.T, home string) {
			if os.Geteuid() == 0 {
				t.Skip("como root un archivo sin permiso se lee igual")
			}
			p := cpsEscribirSesion(t, home, cpsT0, cpsThread,
				cpsSessionMeta(cpsT0, cpsThread),
				cpsTokenCount(cpsT0.Add(time.Second), &cpsRL{52, 300, 1794053319}, nil, "plus"))
			if err := os.Chmod(p, 0o000); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(p, 0o644) })
		},
		"un directorio con el nombre": func(t *testing.T, home string) {
			p := cpsEscribirSesion(t, home, cpsT0, cpsThread)
			os.Remove(p)
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatal(err)
			}
		},
	}
	for nombre, armar := range casos {
		t.Run(nombre, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "codex")
			if nombre != "sin CODEX_HOME" {
				if err := os.MkdirAll(home, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			t.Setenv("CODEX_HOME", home)
			armar(t, home)
			m, _ := med.MedirSesion(cpsThread, cpsT0)
			if m.Model != "" || m.Effort != "" || !cpsSinVentanas(m) {
				t.Fatalf("CA-541: %s: medicion vacia: %+v", nombre, m)
			}
		})
	}
}

// CA-541 (caso hostil): solo se leen session_meta, turn_context y
// token_count. Una linea de mensaje que trae "model" o "rate_limits" en su
// contenido no cuenta, y un event_msg de otro tipo tampoco.
func TestCA541_SoloLeeSessionMetaTurnContextYTokenCount(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	ts := cpsTS(cpsT0.Add(2 * time.Second))
	cpsEscribirSesion(t, home, cpsT0, cpsThread,
		cpsSessionMeta(cpsT0, cpsThread),
		cpsTurnContext(cpsT0.Add(time.Second), "gpt-5.6-sol", "low"),
		cpsTokenCount(cpsT0.Add(time.Second), &cpsRL{52, 300, 1794053319}, nil, "plus"),
		`{"timestamp":"`+ts+`","type":"response_item","payload":{"type":"message","role":"user","model":"gpt-inventado","effort":"xhigh",`+
			`"rate_limits":{"primary":{"used_percent":99,"window_minutes":300,"resets_at":1},"plan_type":"pro"},`+
			`"content":[{"type":"input_text","text":"{\"type\":\"turn_context\",\"payload\":{\"model\":\"gpt-texto\"}}"}]}}`,
		`{"timestamp":"`+ts+`","type":"event_msg","payload":{"type":"agent_message","message":"hola",`+
			`"rate_limits":{"primary":{"used_percent":98,"window_minutes":300,"resets_at":2},"plan_type":"pro"}}}`,
		`{"timestamp":"`+ts+`","type":"event_msg","payload":{"type":"user_message","model":"gpt-otro","effort":"xhigh"}}`,
	)
	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, cpsT0)
	if err != nil {
		t.Fatal(err)
	}
	if m.Model != "gpt-5.6-sol" || m.Effort != "low" {
		t.Fatalf("CA-541: modelo y esfuerzo salen solo del turn_context: %+v", m)
	}
	if len(m.Muestras) != 1 || len(m.Muestras[0]) != 1 || !cpsCasi(m.Muestras[0][0].Usado, 52) || m.Muestras[0][0].Plan != "plus" {
		t.Fatalf("CA-541: las ventanas salen solo de token_count: %+v", m.Muestras)
	}
}

// CA-541: CodexHome es $CODEX_HOME o, sin el, ~/.codex.
func TestCA541_CodexHome(t *testing.T) {
	t.Setenv("CODEX_HOME", "/tmp/un codex home")
	if got := CodexHome(); got != "/tmp/un codex home" {
		t.Fatalf("CA-541: CodexHome es $CODEX_HOME: %q", got)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	os.Unsetenv("CODEX_HOME")
	if got := CodexHome(); got != filepath.Join(home, ".codex") {
		t.Fatalf("CA-541: sin CODEX_HOME es ~/.codex (%s): %q", filepath.Join(home, ".codex"), got)
	}
}

// CA-541: sin CODEX_HOME, la sesion se lee de ~/.codex/sessions.
func TestCA541_SesionPorDefectoEnHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")
	os.Unsetenv("CODEX_HOME")
	cpsEscribirSesion(t, filepath.Join(home, ".codex"), cpsT0, cpsThread,
		cpsSessionMeta(cpsT0, cpsThread),
		cpsTurnContext(cpsT0.Add(time.Second), "gpt-5.6-sol", "low"),
		cpsTokenCount(cpsT0.Add(2*time.Second), &cpsRL{52, 300, 1794053319}, nil, "plus"),
	)
	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, cpsT0)
	if err != nil {
		t.Fatal(err)
	}
	if m.Model != "gpt-5.6-sol" || len(m.Muestras) != 1 {
		t.Fatalf("CA-541: por defecto la sesion vive en ~/.codex/sessions: %+v", m)
	}
}

// CA-541: el stdout de codex exec --json no trae ventanas ni modelo: lo que
// se mide sale del archivo de sesion.
func TestCA541_ElStdoutDeCodexNoMide(t *testing.T) {
	med := cpsMedidor(t, "codex")
	for _, l := range []string{
		`{"type":"thread.started","thread_id":"` + cpsThread + `"}`,
		cpsTurnCompletedCodex,
		`{"type":"item.completed","item":{"type":"agent_message","text":"model gpt rate_limits"}}`,
	} {
		if m := med.MedirLinea(l); m.Model != "" || !cpsSinVentanas(m) {
			t.Fatalf("CA-541: %q no mide nada: %+v", l, m)
		}
	}
}
