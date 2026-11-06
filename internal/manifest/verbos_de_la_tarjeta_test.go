// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md
// (CA-490, CA-491): los valores por rol en hoom.yaml. agents.<rol> gana
// provider, model, effort, budget_usd y timeout; review: gana timeout. La
// carga es estricta: una clave mal escrita, un provider fuera del registro,
// un presupuesto negativo o un tiempo limite sin unidad o fuera de 1m..12h son
// un hoom.yaml invalido con el texto del contrato. RoleDefaults es el unico
// lugar que dice lo que hoom.yaml declara para un rol, y LoadRoles lee solo
// esas secciones del proyecto.
package manifest

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

const vdtValidas = "(validas: provider, model, effort, budget_usd, timeout, write)"

// vdtRango es el final del texto de un tiempo limite que no sirve.
const vdtRango = `no es una duracion entre 1m y 12h (ej. 45m, 1h30m)`

// vdtCargar carga un hoom.yaml minimo con extra al final, con manifest.Load.
func vdtCargar(t *testing.T, extra string) (*Manifest, error) {
	t.Helper()
	return Load(foManifiesto(t, extra), nil)
}

// vdtInvalido exige que la carga falle como hoom.yaml invalido y con el texto
// del contrato adentro.
func vdtInvalido(t *testing.T, ca, extra, quiero string) {
	t.Helper()
	_, err := vdtCargar(t, extra)
	if err == nil {
		t.Fatalf("%s: hoom.yaml con\n%s\ndebe ser un error de carga", ca, extra)
	}
	if !strings.Contains(err.Error(), "hoom.yaml invalido") {
		t.Fatalf("%s: el error es un hoom.yaml invalido: %v", ca, err)
	}
	if quiero != "" && !strings.Contains(err.Error(), quiero) {
		t.Fatalf("%s: el error dice\n  %q\nfue\n  %q", ca, quiero, err.Error())
	}
}

// CA-490: las seis claves de agents.<rol> y review.timeout se leen tal cual;
// el ejemplo del contrato carga y RoleDefaults lo devuelve.
func TestCA490_ClavesDeAgentsYReviewTimeout(t *testing.T) {
	m, err := vdtCargar(t, "agents:\n"+
		"  arquitecto:\n"+
		"    provider: claude\n"+
		"    model: opus\n"+
		"    effort: high\n"+
		"    budget_usd: 5\n"+
		"    timeout: 45m\n"+
		"  test-writer:\n"+
		"    effort: high\n"+
		"    timeout: 45m\n"+
		"    write: { allow: [\"tests/**\"] }\n"+
		"review:\n"+
		"  timeout: 60m\n")
	if err != nil {
		t.Fatalf("CA-490: el ejemplo del contrato es un hoom.yaml valido: %v", err)
	}
	a := m.Agents["arquitecto"]
	if a.Provider != "claude" || a.Model != "opus" || a.Effort != "high" || a.BudgetUSD == nil || *a.BudgetUSD != 5 || a.Timeout != 45*time.Minute {
		t.Fatalf("CA-490: agents.arquitecto se lee tal cual: %+v (budget %v)", a, a.BudgetUSD)
	}
	tw := m.Agents["test-writer"]
	if tw.Effort != "high" || tw.Timeout != 45*time.Minute || tw.Provider != "" || tw.Model != "" || tw.BudgetUSD != nil {
		t.Fatalf("CA-490: agents.test-writer trae solo lo declarado: %+v", tw)
	}
	if len(tw.Write.Allow) != 1 || tw.Write.Allow[0] != "tests/**" {
		t.Fatalf("CA-490: write sigue como hoy: %+v", tw.Write)
	}
	if m.Review == nil || m.Review.Timeout != 60*time.Minute {
		t.Fatalf("CA-490: review.timeout se lee: %+v", m.Review)
	}

	rd := m.RoleDefaults("arquitecto")
	if rd.Provider != "claude" || rd.Model != "opus" || rd.Effort != "high" || rd.BudgetUSD == nil || *rd.BudgetUSD != 5 || rd.Timeout != 45*time.Minute {
		t.Fatalf("CA-490: RoleDefaults(arquitecto) es lo de agents.arquitecto: %+v", rd)
	}
	rd = m.RoleDefaults("test-writer")
	if rd.Provider != "" || rd.Model != "" || rd.Effort != "high" || rd.BudgetUSD != nil || rd.Timeout != 45*time.Minute {
		t.Fatalf("CA-490: RoleDefaults(test-writer) trae solo lo declarado: %+v", rd)
	}
	if rd := m.RoleDefaults("writer"); rd.Provider != "" || rd.Model != "" || rd.Effort != "" || rd.BudgetUSD != nil || rd.Timeout != 0 {
		t.Fatalf("CA-490: un rol sin agents.<rol> no declara nada: %+v", rd)
	}
}

// CA-490: el reviewer sale de review: (provider, model, effort, timeout) y no
// tiene presupuesto ahi; un *Manifest nil no declara nada.
func TestCA490_RoleDefaultsDelReviewerSaleDeReview(t *testing.T) {
	m, err := vdtCargar(t, "review:\n  provider: codex\n  model: gpt-r\n  effort: xhigh\n  timeout: 2h\n")
	if err != nil {
		t.Fatalf("CA-490: review: con timeout es valido: %v", err)
	}
	rd := m.RoleDefaults("reviewer")
	if rd.Provider != "codex" || rd.Model != "gpt-r" || rd.Effort != "xhigh" || rd.Timeout != 2*time.Hour || rd.BudgetUSD != nil {
		t.Fatalf("CA-490: RoleDefaults(reviewer) sale de review: y sin presupuesto: %+v", rd)
	}
	var nada *Manifest
	if rd := nada.RoleDefaults("writer"); rd.Provider != "" || rd.Model != "" || rd.Effort != "" || rd.BudgetUSD != nil || rd.Timeout != 0 {
		t.Fatalf("CA-490: un manifiesto nil no declara nada: %+v", rd)
	}
	if rd := (&Manifest{}).RoleDefaults("reviewer"); rd.Provider != "" || rd.Timeout != 0 {
		t.Fatalf("CA-490: un manifiesto sin review: no declara nada para el reviewer: %+v", rd)
	}
}

// CA-490: budget_usd 0 es "sin tope" declarado (no lo mismo que no declararlo).
func TestCA490_BudgetCeroEsSinTopeDeclarado(t *testing.T) {
	m, err := vdtCargar(t, "agents:\n  writer:\n    budget_usd: 0\n")
	if err != nil {
		t.Fatalf("CA-490: budget_usd 0 es valido (sin tope): %v", err)
	}
	if b := m.Agents["writer"].BudgetUSD; b == nil || *b != 0 {
		t.Fatalf("CA-490: budget_usd 0 queda declarado como 0: %v", b)
	}
	if b := m.RoleDefaults("writer").BudgetUSD; b == nil || *b != 0 {
		t.Fatalf("CA-490: RoleDefaults distingue 0 (sin tope) de nil (sin declarar): %v", b)
	}
	m, err = vdtCargar(t, "agents:\n  writer:\n    budget_usd: 2.5\n")
	if err != nil || m.RoleDefaults("writer").BudgetUSD == nil || *m.RoleDefaults("writer").BudgetUSD != 2.5 {
		t.Fatalf("CA-490: budget_usd 2.5 se lee: %v %+v", err, m)
	}
}

// CA-490: una clave desconocida en agents.<rol> es un error de carga que la
// nombra y nombra las validas; hoy se ignoraba en silencio (efort: low).
func TestCA490_ClaveDesconocidaEnAgentsFalla(t *testing.T) {
	for _, clave := range []string{"efort", "modelo", "Model", "budget", "budgetUsd", "time_out", "timeouts", "presupuesto_usd", "same_provider"} {
		vdtInvalido(t, "CA-490", "agents:\n  writer:\n    "+clave+": low\n",
			`agents.writer: clave desconocida "`+clave+`" `+vdtValidas)
	}
	// junto a claves validas, y en otro rol
	vdtInvalido(t, "CA-490", "agents:\n  test-writer:\n    effort: high\n    efort: low\n",
		`agents.test-writer: clave desconocida "efort" `+vdtValidas)
}

// CA-490: un provider fuera del registro es un error de carga; los cuatro
// del registro valen aunque no esten instalados (se comprueba al lanzar).
func TestCA490_ProviderFueraDelRegistroFalla(t *testing.T) {
	for _, p := range []string{"claude", "codex", "gemini", "opencode"} {
		if _, err := vdtCargar(t, "agents:\n  writer:\n    provider: "+p+"\n"); err != nil {
			t.Fatalf("CA-490: %s es un provider de hoom: %v", p, err)
		}
	}
	for _, p := range []string{"chatgpt", "Claude", "claude-code", "copilot", "mudo"} {
		vdtInvalido(t, "CA-490", "agents:\n  writer:\n    provider: "+p+"\n",
			`agents.writer.provider: "`+p+`" no es un provider de hoom (claude, codex, gemini, opencode)`)
	}
}

// CA-490: un budget_usd negativo es un error de carga.
func TestCA490_BudgetNegativoFalla(t *testing.T) {
	for _, v := range []string{"-1", "-0.01", "-500"} {
		vdtInvalido(t, "CA-490", "agents:\n  arquitecto:\n    budget_usd: "+v+"\n", "agents.arquitecto")
	}
}

// CA-490 (y caso limite "agents.test-writer.timeout: 45 sin unidad"): un
// timeout sin unidad, fuera de 1m..12h o que no es una duracion es un error
// de carga con el texto del contrato; los extremos valen.
func TestCA490_TimeoutSinUnidadOFueraDeRangoFalla(t *testing.T) {
	for _, v := range []string{"45", "0", "30s", "59s", "12h1m", "13h", "-5m", "abc", "45 m", "1d"} {
		vdtInvalido(t, "CA-490", "agents:\n  test-writer:\n    timeout: "+v+"\n",
			`agents.test-writer.timeout: "`+v+`" `+vdtRango)
	}
	for v, d := range map[string]time.Duration{"1m": time.Minute, "12h": 12 * time.Hour, "1h30m": 90 * time.Minute, "45m": 45 * time.Minute, "90s": 90 * time.Second} {
		m, err := vdtCargar(t, "agents:\n  writer:\n    timeout: "+v+"\n")
		if err != nil {
			t.Fatalf("CA-490: timeout %s esta entre 1m y 12h: %v", v, err)
		}
		if got := m.Agents["writer"].Timeout; got != d {
			t.Fatalf("CA-490: timeout %s se lee como %s, fue %s", v, d, got)
		}
	}
}

// CA-490: review.timeout sigue las mismas reglas.
func TestCA490_ReviewTimeoutMismasReglas(t *testing.T) {
	for _, v := range []string{"60", "30s", "13h"} {
		vdtInvalido(t, "CA-490", "review:\n  timeout: "+v+"\n", `review.timeout: "`+v+`" `+vdtRango)
	}
	m, err := vdtCargar(t, "review:\n  timeout: 1h30m\n  model: m\n")
	if err != nil || m.Review == nil || m.Review.Timeout != 90*time.Minute {
		t.Fatalf("CA-490: review.timeout 1h30m es valido: %v %+v", err, m)
	}
}

// CA-490: agents.reviewer acepta solo write; una clave de lanzamiento ahi es
// el error que manda a review:. Un solo lugar por rol.
func TestCA490_AgentsReviewerSoloWrite(t *testing.T) {
	if _, err := vdtCargar(t, "agents:\n  reviewer:\n    write: { allow: [\"docs/**\"] }\n"); err != nil {
		t.Fatalf("CA-490: agents.reviewer con write sigue valiendo: %v", err)
	}
	for _, clave := range []string{"provider: codex", "model: m", "effort: high", "budget_usd: 2", "timeout: 45m"} {
		vdtInvalido(t, "CA-490", "agents:\n  reviewer:\n    "+clave+"\n",
			"agents.reviewer: la review se configura en review: (provider, model, effort, timeout)")
	}
}

// CA-490: un hoom.yaml sin las claves nuevas carga igual que hoy, con o sin
// agents: y review:.
func TestCA490_SinLasClavesNuevasCargaComoHoy(t *testing.T) {
	for nombre, extra := range map[string]string{
		"sin agents ni review": "",
		"agents con write":     "agents:\n  test-writer:\n    write: { allow: [\"tests/**\"] }\n",
		"agents vacio":         "agents: {}\n",
		"review sin timeout":   "review:\n  model: m\n  effort: e\n",
	} {
		m, err := vdtCargar(t, extra)
		if err != nil {
			t.Fatalf("CA-490 (%s): carga como hoy: %v", nombre, err)
		}
		for _, rol := range []string{"arquitecto", "test-writer", "writer"} {
			if rd := m.RoleDefaults(rol); rd.Provider != "" || rd.Model != "" || rd.Effort != "" || rd.BudgetUSD != nil || rd.Timeout != 0 {
				t.Fatalf("CA-490 (%s): %s no declara valores de lanzamiento: %+v", nombre, rol, rd)
			}
		}
		if m.Review != nil && m.Review.Timeout != 0 {
			t.Fatalf("CA-490 (%s): review sin timeout no declara timeout: %v", nombre, m.Review.Timeout)
		}
	}
}

// CA-490: ParseTimeout es la regla de los flags: el mismo texto con el campo
// que se le da.
func TestCA490_ParseTimeoutTextoDelContrato(t *testing.T) {
	for _, v := range []string{"0", "30", "13h", "59s", "", "abc", "-1m"} {
		_, err := ParseTimeout("--timeout", v)
		if err == nil {
			t.Fatalf("CA-490: --timeout %q no es un tiempo limite valido", v)
		}
		if want := `--timeout: "` + v + `" ` + vdtRango; err.Error() != want {
			t.Fatalf("CA-490: el error es\n  %q\nfue\n  %q", want, err.Error())
		}
	}
	for v, d := range map[string]time.Duration{"1m": time.Minute, "12h": 12 * time.Hour, "45m": 45 * time.Minute, "1h30m": 90 * time.Minute, "2h": 2 * time.Hour} {
		got, err := ParseTimeout("agents.writer.timeout", v)
		if err != nil || got != d {
			t.Fatalf("CA-490: ParseTimeout(%q) = %s (%v), se esperaba %s", v, got, err, d)
		}
	}
	if MinTimeout != time.Minute || MaxTimeout != 12*time.Hour {
		t.Fatalf("CA-490: los extremos son 1m y 12h: %s %s", MinTimeout, MaxTimeout)
	}
}

// CA-490 (propiedad): toda duracion entera en segundos dentro de [1m, 12h]
// vuelve tal cual de su texto de Go; toda duracion afuera es un error.
func TestCA490_PropiedadTimeoutEnRango(t *testing.T) {
	adentro := func(s uint32) bool {
		d := MinTimeout + time.Duration(int64(s)%int64((MaxTimeout-MinTimeout)/time.Second+1))*time.Second
		got, err := ParseTimeout("--timeout", d.String())
		return err == nil && got == d
	}
	if err := quick.Check(adentro, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(490))}); err != nil {
		t.Fatalf("CA-490: una duracion entre 1m y 12h es valida: %v", err)
	}
	afuera := func(s uint32, arriba bool) bool {
		var d time.Duration
		if arriba {
			d = MaxTimeout + time.Duration(1+int64(s)%(48*3600))*time.Second
		} else {
			d = time.Duration(int64(s)%60) * time.Second // 0s..59s
		}
		_, err := ParseTimeout("--timeout", d.String())
		return err != nil
	}
	if err := quick.Check(afuera, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(491))}); err != nil {
		t.Fatalf("CA-490: una duracion fuera de 1m..12h es un error: %v", err)
	}
}

// CA-491: LoadRoles lee agents: y review: del hoom.yaml del proyecto, con la
// misma estrictez; sin hoom.yaml no declara nada y no es un error.
func TestCA491_LoadRolesLeeElProyecto(t *testing.T) {
	root := foManifiesto(t, "agents:\n  writer:\n    model: m-proyecto\n    timeout: 45m\nreview:\n  timeout: 60m\n")
	m, err := LoadRoles(root)
	if err != nil {
		t.Fatalf("CA-491: LoadRoles del proyecto: %v", err)
	}
	if rd := m.RoleDefaults("writer"); rd.Model != "m-proyecto" || rd.Timeout != 45*time.Minute {
		t.Fatalf("CA-491: LoadRoles trae agents.writer del proyecto: %+v", rd)
	}
	if rd := m.RoleDefaults("reviewer"); rd.Timeout != 60*time.Minute {
		t.Fatalf("CA-491: LoadRoles trae review.timeout del proyecto: %+v", rd)
	}

	vacio := t.TempDir()
	m, err = LoadRoles(vacio)
	if err != nil {
		t.Fatalf("CA-491: un root sin hoom.yaml no es un error: %v", err)
	}
	if rd := m.RoleDefaults("writer"); rd.Model != "" || rd.Timeout != 0 || rd.Provider != "" {
		t.Fatalf("CA-491: sin hoom.yaml no se declara nada: %+v", rd)
	}

	malo := foManifiesto(t, "agents:\n  writer:\n    efort: low\n")
	if _, err := LoadRoles(malo); err == nil || !strings.Contains(err.Error(), `agents.writer: clave desconocida "efort"`) {
		t.Fatalf("CA-491: LoadRoles es tan estricto como Load con agents: %v", err)
	}
	sinUnidad := foManifiesto(t, "agents:\n  test-writer:\n    timeout: 45\n")
	if _, err := LoadRoles(sinUnidad); err == nil || !strings.Contains(err.Error(), `agents.test-writer.timeout: "45" `+vdtRango) {
		t.Fatalf("CA-491: LoadRoles rechaza un timeout sin unidad: %v", err)
	}
}

// CA-491: LoadRoles lee el hoom.yaml de ESE directorio: el de un espacio de
// trabajo de la tarjeta (otra rama) no se mezcla con el del proyecto.
func TestCA491_LoadRolesNoMezclaOtroArbol(t *testing.T) {
	proyecto := foManifiesto(t, "agents:\n  writer:\n    budget_usd: 1\n")
	rama := filepath.Join(proyecto, ".hoom", "worktrees", "tarjeta")
	if err := os.MkdirAll(rama, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nagents:\n  writer:\n    budget_usd: 99\n"
	if err := os.WriteFile(filepath.Join(rama, FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadRoles(proyecto)
	if err != nil {
		t.Fatal(err)
	}
	if b := m.RoleDefaults("writer").BudgetUSD; b == nil || *b != 1 {
		t.Fatalf("CA-491: el presupuesto es el del proyecto (1), no el de la rama (99): %v", b)
	}
}
