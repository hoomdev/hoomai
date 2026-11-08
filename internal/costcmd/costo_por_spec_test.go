// Tests adversariales del spec .hoom/specs/costo-por-spec.md (CA-543 a
// CA-547) sobre `hoom costo`: lo que costo cada tarea, por rol, por ronda de
// review y por provider, en las unidades que cada provider da, con la regla
// de bloques de la ventana y sin un cero donde falta un dato. Escritos DESDE
// EL SPEC y el contrato publico del paquete (go doc: los textos, la regla de
// bloques y el esquema JSON), sin leer la implementacion.
//
// Los fixtures escriben lo que hoom deja en disco: los sidecars de
// .hoom/runs (json de runcmd.Meta), los registros de review
// (reviewcmd.WriteRecord), los items (item.Add + item.MarkDone), los
// veredictos (verdict.Write) y los hallazgos (finding.Register). Un worktree
// vivo es el directorio .hoom/worktrees/<slug>.
package costcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- fixtures

// ccT0 es la hora base de los fixtures.
var ccT0 = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

// ccR5 y ccR7 son los reinicios de la 5h y de la semana de los fixtures.
var (
	ccR5 = time.Unix(1794078000, 0).UTC()
	ccR7 = time.Unix(1794517200, 0).UTC()
)

func ccMin(n int) time.Duration { return time.Duration(n) * time.Minute }

func ccRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

var ccSeq int

// ccID es un id con la forma de los de hoom: <fecha>_<6 hex>.
func ccID(cuando time.Time) string {
	ccSeq++
	return fmt.Sprintf("%s_%06x", cuando.UTC().Format("20060102T150405"), ccSeq)
}

// ccMeta arma el sidecar de un run terminado (done) de dur.
func ccMeta(task, role, prov string, ini time.Time, dur time.Duration) runcmd.Meta {
	return runcmd.Meta{ID: ccID(ini), Provider: prov, Role: role, Task: task, CreatedAt: ini,
		Status: runcmd.StatusDone, EndedAt: ini.Add(dur)}
}

func ccF(f float64) *float64 { return &f }

func ccClaude(in, cached, out int, costo *float64) *providers.Usage {
	return &providers.Usage{InputTokens: in, CachedTokens: cached, OutputTokens: out, CostUSD: costo, Turns: 1}
}

func ccCodex(in, cached, out, reas int) *providers.Usage {
	return &providers.Usage{InputTokens: in, CachedTokens: cached, OutputTokens: out, ReasoningTokens: reas, Turns: 1}
}

func ccV(nombre string, ini, fin float64, reinicia time.Time) runcmd.VentanaRun {
	return runcmd.VentanaRun{Nombre: nombre, Inicio: ini, Fin: fin, ReiniciaEn: reinicia}
}

// ccRun escribe el sidecar en <root>/.hoom/runs/<id>.meta.json.
func ccRun(t *testing.T, root string, m runcmd.Meta) runcmd.Meta {
	t.Helper()
	if m.Dir == "" {
		m.Dir = root
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hoom", "runs", m.ID+".meta.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return m
}

func ccIDs(ms ...runcmd.Meta) []string {
	var out []string
	for _, m := range ms {
		out = append(out, m.ID)
	}
	return out
}

func ccLensRuns(lentes []string, ms ...runcmd.Meta) []reviewcmd.LensRun {
	var out []reviewcmd.LensRun
	for i, m := range ms {
		out = append(out, reviewcmd.LensRun{Lens: lentes[i], Run: m.ID})
	}
	return out
}

// ccRegistro escribe un registro de review de la tarea.
func ccRegistro(t *testing.T, root, task, cobertura string, cuando time.Time, lentes []string, runs []reviewcmd.LensRun, hallazgos []string) reviewcmd.Record {
	t.Helper()
	if hallazgos == nil {
		hallazgos = []string{}
	}
	r, err := reviewcmd.WriteRecord(root, reviewcmd.Record{ID: ccID(cuando), CreatedAt: cuando, Task: task,
		Spec: ".hoom/specs/" + task + ".md", Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: lentes,
		Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: hallazgos, Usage: []reviewcmd.LensUsage{},
		Runs: runs, Cobertura: cobertura, Base: "main"})
	if err != nil {
		t.Fatalf("fixture: WriteRecord: %v", err)
	}
	return r
}

func ccItem(t *testing.T, root, slug string) {
	t.Helper()
	if _, err := item.Add(root, item.Draft{Titulo: "Tarea " + slug, Slug: slug}); err != nil {
		t.Fatalf("fixture: item.Add(%s): %v", slug, err)
	}
}

func ccAceptar(t *testing.T, root, slug, commit string, cuando time.Time) {
	t.Helper()
	if ok, err := item.MarkDone(root, slug, commit, cuando); err != nil || !ok {
		t.Fatalf("fixture: item.MarkDone(%s): %v %v", slug, ok, err)
	}
}

func ccWorktree(t *testing.T, root, slug string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "worktrees", slug), 0o755); err != nil {
		t.Fatal(err)
	}
}

// ccVeredicto escribe un veredicto de la spec de slug y devuelve su id.
func ccVeredicto(t *testing.T, root, slug string, cuando time.Time, verde, parcial bool) string {
	t.Helper()
	estado := verdict.StatusPass
	if !verde {
		estado = verdict.StatusFail
	}
	v := &verdict.Verdict{Project: "demo", CreatedAt: cuando, Git: gitx.Info{},
		Gates: []verdict.GateResult{{Name: "test", Required: true, Status: estado}},
		Spec:  ".hoom/specs/" + slug + ".md", Partial: parcial}
	v.Finalize()
	if (v.Verdict == "green") != verde {
		t.Fatalf("fixture: el veredicto sale %s", v.Verdict)
	}
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatalf("fixture: verdict.Write: %v", err)
	}
	if v.ID == "" {
		t.Fatal("fixture: verdict.Write estampa el id")
	}
	return v.ID
}

func ccHallazgo(t *testing.T, root, slug, sev string) string {
	t.Helper()
	h := "huella"
	f, err := finding.Register(root, "main", finding.Draft{Severity: sev, Lens: "risk", File: "app.go",
		Description: "hallazgo " + sev + " de " + slug, Author: "reviewer@codex", Task: slug, Huella: &h})
	if err != nil {
		t.Fatalf("fixture: finding.Register: %v", err)
	}
	return f.ID
}

func ccBuild(t *testing.T, root, slug string) Reporte {
	t.Helper()
	r, err := Build(root, slug)
	if err != nil {
		t.Fatalf("Build(%q): %v", slug, err)
	}
	return r
}

func ccTarea(t *testing.T, ca string, r Reporte, slug string) Tarea {
	t.Helper()
	var hallada []Tarea
	for _, ta := range r.Tareas {
		if ta.Slug == slug {
			hallada = append(hallada, ta)
		}
	}
	if len(hallada) != 1 {
		t.Fatalf("%s: el reporte tiene una sola tarea %q: %+v", ca, slug, r.Tareas)
	}
	return hallada[0]
}

func ccTexto(r Reporte, detalle bool) string {
	var b bytes.Buffer
	Render(&b, r, detalle)
	return b.String()
}

func ccJSON(t *testing.T, r Reporte) map[string]any {
	t.Helper()
	raw, err := JSONBytes(r)
	if err != nil {
		t.Fatalf("CA-547: JSONBytes: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-547: JSONBytes es JSON valido: %v\n%s", err, raw)
	}
	return m
}

func ccCasi(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func ccInt(p *int) string {
	if p == nil {
		return "null"
	}
	return fmt.Sprint(*p)
}

// ccTokens exige los cuatro numeros (nil = null).
func ccTokens(t *testing.T, ca, que string, got Tokens, nuevos, cache, salida, razon *int) {
	t.Helper()
	eq := func(a, b *int) bool { return (a == nil) == (b == nil) && (a == nil || *a == *b) }
	if !eq(got.Nuevos, nuevos) || !eq(got.CacheLeido, cache) || !eq(got.Salida, salida) || !eq(got.Razonamiento, razon) {
		t.Fatalf("%s: %s: tokens nuevos=%s cache_leido=%s salida=%s razonamiento=%s; fueron %s %s %s %s", ca, que,
			ccInt(nuevos), ccInt(cache), ccInt(salida), ccInt(razon),
			ccInt(got.Nuevos), ccInt(got.CacheLeido), ccInt(got.Salida), ccInt(got.Razonamiento))
	}
}

func ccI(n int) *int { return &n }

func ccMov(t *testing.T, ca string, g Gasto, nombre string) Movimiento {
	t.Helper()
	m, ok := g.Ventanas[nombre]
	if !ok || m.Puntos == nil {
		t.Fatalf("%s: la ventana %q tiene movimiento: %+v", ca, nombre, g.Ventanas)
	}
	return m
}

func ccPorRol(t *testing.T, ca string, ta Tarea, rol, prov string) GastoRol {
	t.Helper()
	var hallada []GastoRol
	for _, g := range ta.PorRol {
		if g.Rol == rol && g.Provider == prov {
			hallada = append(hallada, g)
		}
	}
	if len(hallada) != 1 {
		t.Fatalf("%s: por_rol tiene una sola fila (%q, %s): %+v", ca, rol, prov, ta.PorRol)
	}
	return hallada[0]
}

// ccLineaCon devuelve las lineas del texto que contienen s.
func ccLineasCon(texto, s string) []string {
	var out []string
	for _, l := range strings.Split(texto, "\n") {
		if strings.Contains(l, s) {
			out = append(out, l)
		}
	}
	return out
}

// ---------------------------------------------------------------- la tarea alfa

// ccAlfaFixture es una tarea aceptada con runs de dos providers, 4 roles
// (uno sin rol) y dos rondas de review (completa y delta) que nombran sus
// runs.
type ccAlfaFixture struct {
	root, veredicto string
	r1, r2          reviewcmd.Record
}

// ccAlfa planta la tarea alfa. Las cifras esperadas:
//
//	claude: 4 runs; nuevos 357, cache 3570, salida 36, razonamiento null;
//	        USD 0.85 (sr no informa: sin_costo 1); 5h 5, semana 2; sin
//	        ventana 1 (sr); duracion 33 min.
//	codex:  6 runs; nuevos 9600, cache 1200, salida 108, razonamiento 17;
//	        sin USD (sin_costo 6); 5h 6.5.
func ccAlfa(t *testing.T) ccAlfaFixture {
	t.Helper()
	root := ccRoot(t)
	// claude
	w1 := ccMeta("alfa", "writer", "claude", ccT0, ccMin(10))
	w1.Usage = ccClaude(100, 1000, 10, ccF(0.5))
	w1.Ventanas = []runcmd.VentanaRun{ccV("5h", 10, 12, ccR5), ccV("semana", 20, 20.5, ccR7)}
	tw := ccMeta("alfa", "test-writer", "claude", ccT0.Add(ccMin(12)), ccMin(8))
	tw.Usage = ccClaude(50, 500, 5, ccF(0.1))
	tw.Ventanas = []runcmd.VentanaRun{ccV("5h", 12, 13, ccR5), ccV("semana", 20.5, 21, ccR7)}
	w2 := ccMeta("alfa", "writer", "claude", ccT0.Add(ccMin(30)), ccMin(10))
	w2.Usage = ccClaude(200, 2000, 20, ccF(0.25))
	w2.Ventanas = []runcmd.VentanaRun{ccV("5h", 13, 15, ccR5), ccV("semana", 21, 22, ccR7)}
	sr := ccMeta("alfa", "", "claude", ccT0.Add(ccMin(50)), ccMin(5))
	sr.Usage = ccClaude(7, 70, 1, nil)
	for _, m := range []runcmd.Meta{w1, tw, w2, sr} {
		ccRun(t, root, m)
	}
	// codex: 4 lentes de la ronda completa, 1 de la delta y un writer
	var rv []runcmd.Meta
	for i := 1; i <= 4; i++ {
		m := ccMeta("alfa", "reviewer", "codex", ccT0.Add(ccMin(60+5*(i-1))), ccMin(5))
		m.Usage = ccCodex(1000*i, 100*i, 10*i, i)
		m.Ventanas = []runcmd.VentanaRun{ccV("5h", []float64{40, 41, 42, 44}[i-1], []float64{41, 42, 44, 45}[i-1], ccR5)}
		rv = append(rv, ccRun(t, root, m))
	}
	rv5 := ccMeta("alfa", "reviewer", "codex", ccT0.Add(ccMin(120)), ccMin(5))
	rv5.Usage = ccCodex(500, 100, 5, 0)
	rv5.Ventanas = []runcmd.VentanaRun{ccV("5h", 50, 51, ccR5)}
	ccRun(t, root, rv5)
	wc := ccMeta("alfa", "writer", "codex", ccT0.Add(ccMin(150)), ccMin(10))
	wc.Usage = ccCodex(300, 100, 3, 7)
	wc.Ventanas = []runcmd.VentanaRun{ccV("5h", 51, 51.5, ccR5)}
	ccRun(t, root, wc)

	ccItem(t, root, "alfa")
	h1 := ccHallazgo(t, root, "alfa", "high")
	l1 := ccHallazgo(t, root, "alfa", "low")
	l2 := ccHallazgo(t, root, "alfa", "low")
	r1 := ccRegistro(t, root, "alfa", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(81)), reviewcmd.Lentes,
		ccLensRuns(reviewcmd.Lentes, rv...), []string{h1, l1, l2})
	r2 := ccRegistro(t, root, "alfa", reviewcmd.CoberturaDelta, ccT0.Add(ccMin(126)), []string{"risk"},
		ccLensRuns([]string{"risk"}, rv5), nil)
	ver := ccVeredicto(t, root, "alfa", ccT0.Add(ccMin(170)), true, false)
	ccAceptar(t, root, "alfa", "abc1234def", ccT0.Add(ccMin(180)))
	return ccAlfaFixture{root: root, veredicto: ver, r1: r1, r2: r2}
}

// ---------------------------------------------------------------- CA-543

// CA-543 (y CA-544, CA-547): ParseArgs es puro: a lo sumo un slug valido y
// --json; cualquier otra cosa es un error de uso (exit 2); -h es la ayuda.
func TestCA543_ParseArgs(t *testing.T) {
	for _, c := range []struct {
		args []string
		want Options
	}{
		{nil, Options{}},
		{[]string{}, Options{}},
		{[]string{"alfa"}, Options{Slug: "alfa"}},
		{[]string{"--json"}, Options{JSON: true}},
		{[]string{"alfa", "--json"}, Options{Slug: "alfa", JSON: true}},
		{[]string{"--json", "costo-por-spec"}, Options{Slug: "costo-por-spec", JSON: true}},
	} {
		got, err := ParseArgs(c.args)
		if err != nil || got != c.want {
			t.Fatalf("CA-543: ParseArgs(%q) = %+v, %v; quiero %+v", c.args, got, err, c.want)
		}
	}
	for _, args := range [][]string{
		{"--bogus"}, {"alfa", "beta"}, {"Alfa Mayuscula"}, {"no/slug"}, {""}, {"--json", "a", "b"},
		{"--slug", "alfa"}, {strings.Repeat("a", item.MaxSlug+1)}, {"ñandu"},
	} {
		_, err := ParseArgs(args)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || ue.ExitCode() != 2 {
			t.Fatalf("CA-543: ParseArgs(%q) es un error de uso (exit 2): %v", args, err)
		}
	}
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		if _, err := ParseArgs(args); !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-543: ParseArgs(%q) pide la ayuda: %v", args, err)
		}
	}
}

// CA-543: sin slug, una tarea por slug que aparece como task de un run o de
// un registro; los runs sin task no cuentan; de la mas reciente a la mas
// vieja.
func TestCA543_UnaTareaPorSlugConRunsOReviews(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "writer", "claude", ccT0, ccMin(5)))
	ccRun(t, root, ccMeta("alfa", "writer", "claude", ccT0.Add(ccMin(10)), ccMin(5)))
	ccRun(t, root, ccMeta("beta", "writer", "codex", ccT0.Add(ccMin(60)), ccMin(5)))
	ccRegistro(t, root, "gamma", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(120)), []string{"risk"}, nil, nil)
	ccRun(t, root, ccMeta("", "writer", "claude", ccT0.Add(ccMin(200)), ccMin(5)))
	ccRun(t, root, ccMeta("", "", "codex", ccT0.Add(ccMin(210)), ccMin(5)))
	ccItem(t, root, "delta-sin-nada") // un item sin runs ni reviews no es una tarea con costo

	r := ccBuild(t, root, "")
	var slugs []string
	for _, ta := range r.Tareas {
		slugs = append(slugs, ta.Slug)
	}
	if !reflect.DeepEqual(slugs, []string{"gamma", "beta", "alfa"}) {
		t.Fatalf("CA-543: una tarea por slug con runs o reviews, de la mas reciente a la mas vieja: %q", slugs)
	}
	if r.Ilegibles != 0 {
		t.Fatalf("CA-543: sin sidecars rotos no hay ilegibles: %d", r.Ilegibles)
	}
}

// CA-543: la actividad de una tarea es lo mas nuevo de sus runs y sus
// registros: un registro reciente sube a una tarea de runs viejos.
func TestCA543_UnRegistroRecienteSubeLaTarea(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("vieja", "writer", "claude", ccT0, ccMin(5)))
	ccRun(t, root, ccMeta("nueva", "writer", "claude", ccT0.Add(ccMin(60)), ccMin(5)))
	ccRegistro(t, root, "vieja", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(90)), []string{"risk"}, nil, nil)
	r := ccBuild(t, root, "")
	if len(r.Tareas) != 2 || r.Tareas[0].Slug != "vieja" || r.Tareas[1].Slug != "nueva" {
		t.Fatalf("CA-543: vieja tiene la actividad mas reciente (su registro): %+v", r.Tareas)
	}
}

// CA-543 (propiedad): para cualquier mezcla de runs y registros con fechas
// distintas, el orden es la actividad mas reciente primero.
func TestCA543_PropiedadOrdenPorActividad(t *testing.T) {
	r := rand.New(rand.NewSource(543))
	for caso := 0; caso < 25; caso++ {
		root := ccRoot(t)
		minutos := r.Perm(2000)
		k := 0
		siguiente := func() time.Time { k++; return ccT0.Add(ccMin(minutos[k-1])) }
		n := 2 + r.Intn(5)
		ultima := map[string]time.Time{}
		for i := 0; i < n; i++ {
			slug := fmt.Sprintf("tarea-%d", i)
			runs, regs := r.Intn(3), r.Intn(3)
			if runs+regs == 0 {
				runs = 1
			}
			for j := 0; j < runs; j++ {
				cuando := siguiente()
				ccRun(t, root, ccMeta(slug, "writer", []string{"claude", "codex"}[r.Intn(2)], cuando, ccMin(1)))
				if cuando.After(ultima[slug]) {
					ultima[slug] = cuando
				}
			}
			for j := 0; j < regs; j++ {
				cuando := siguiente()
				ccRegistro(t, root, slug, reviewcmd.CoberturaCompleta, cuando, []string{"risk"}, nil, nil)
				if cuando.After(ultima[slug]) {
					ultima[slug] = cuando
				}
			}
		}
		var quiero []string
		for s := range ultima {
			quiero = append(quiero, s)
		}
		sort.Slice(quiero, func(i, j int) bool { return ultima[quiero[i]].After(ultima[quiero[j]]) })
		var got []string
		for _, ta := range ccBuild(t, root, "").Tareas {
			got = append(got, ta.Slug)
		}
		if !reflect.DeepEqual(got, quiero) {
			t.Fatalf("CA-543: caso %d: de la actividad mas reciente a la mas vieja: %q, fue %q (%v)", caso, quiero, got, ultima)
		}
	}
}

// CA-543: el estado. aceptada si el item tiene hecho_en (aunque el worktree
// siga); si no, en curso con el worktree vivo y cerrada sin aceptar sin el
// (tenga item o no). Solo la aceptada lleva aceptada_en, veredicto y
// commit_final.
func TestCA543_ElEstadoDeCadaTarea(t *testing.T) {
	root := ccRoot(t)
	slugs := []string{"acepta", "acepta-con-wt", "curso", "curso-sin-item", "cerrada", "cerrada-sin-item"}
	for i, s := range slugs {
		ccRun(t, root, ccMeta(s, "writer", "claude", ccT0.Add(ccMin(10*i)), ccMin(5)))
		ccVeredicto(t, root, s, ccT0.Add(ccMin(10*i+6)), true, false)
	}
	for _, s := range []string{"acepta", "acepta-con-wt", "curso", "cerrada"} {
		ccItem(t, root, s)
	}
	hecho := time.Date(2026, 11, 7, 12, 30, 0, 0, time.UTC)
	ccAceptar(t, root, "acepta", "c0ffee1", hecho)
	ccAceptar(t, root, "acepta-con-wt", "c0ffee2", hecho)
	for _, s := range []string{"acepta-con-wt", "curso", "curso-sin-item"} {
		ccWorktree(t, root, s)
	}
	r := ccBuild(t, root, "")
	quiero := map[string]string{"acepta": EstadoAceptada, "acepta-con-wt": EstadoAceptada, "curso": EstadoEnCurso,
		"curso-sin-item": EstadoEnCurso, "cerrada": EstadoCerradaSinAceptar, "cerrada-sin-item": EstadoCerradaSinAceptar}
	for s, e := range quiero {
		ta := ccTarea(t, "CA-543", r, s)
		if ta.Estado != e {
			t.Fatalf("CA-543: %s esta %s, fue %s", s, e, ta.Estado)
		}
		aceptada := e == EstadoAceptada
		if (ta.AceptadaEn != nil) != aceptada || (ta.Veredicto != nil) != aceptada || (ta.CommitFinal != nil) != aceptada {
			t.Fatalf("CA-543: solo una aceptada lleva aceptada_en, veredicto y commit_final (%s): %+v", s, ta)
		}
		if aceptada && (!ta.AceptadaEn.Equal(hecho) || *ta.CommitFinal != map[string]string{"acepta": "c0ffee1", "acepta-con-wt": "c0ffee2"}[s]) {
			t.Fatalf("CA-543: aceptada_en es hecho_en y commit_final el del item (%s): %+v", s, ta)
		}
	}
}

// CA-543: el veredicto de una tarea aceptada es el verde mas reciente, no
// parcial, de SU spec: ni el rojo posterior, ni el verde parcial, ni el verde
// de otra spec.
func TestCA543_ElVeredictoEsElVerdeMasRecienteDeSuSpec(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "writer", "claude", ccT0, ccMin(5)))
	ccItem(t, root, "alfa")
	ccVeredicto(t, root, "alfa", ccT0.Add(ccMin(10)), true, false)
	quiero := ccVeredicto(t, root, "alfa", ccT0.Add(ccMin(20)), true, false)
	ccVeredicto(t, root, "alfa", ccT0.Add(ccMin(30)), false, false)
	ccVeredicto(t, root, "alfa", ccT0.Add(ccMin(40)), true, true)
	ccVeredicto(t, root, "beta", ccT0.Add(ccMin(50)), true, false)
	ccAceptar(t, root, "alfa", "abc", ccT0.Add(ccMin(60)))
	ta := ccTarea(t, "CA-543", ccBuild(t, root, "alfa"), "alfa")
	if ta.Veredicto == nil || *ta.Veredicto != quiero {
		t.Fatalf("CA-543: el veredicto es el verde no parcial mas reciente de .hoom/specs/alfa.md (%s): %v", quiero, ta.Veredicto)
	}
}

// CA-543: rondas cuenta los registros de review de la tarea, y solo los
// suyos.
func TestCA543_RondasCuentaLosRegistros(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "writer", "claude", ccT0, ccMin(5)))
	ccRun(t, root, ccMeta("beta", "writer", "claude", ccT0, ccMin(5)))
	for i := 0; i < 3; i++ {
		ccRegistro(t, root, "alfa", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(10+i)), []string{"risk"}, nil, nil)
	}
	ccRegistro(t, root, "otra", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(20)), []string{"risk"}, nil, nil)
	r := ccBuild(t, root, "")
	if a, b := ccTarea(t, "CA-543", r, "alfa"), ccTarea(t, "CA-543", r, "beta"); a.Rondas != 3 || b.Rondas != 0 {
		t.Fatalf("CA-543: alfa tiene 3 rondas y beta 0: %d %d", a.Rondas, b.Rondas)
	}
	if o := ccTarea(t, "CA-543", r, "otra"); o.Rondas != 1 {
		t.Fatalf("CA-543: una tarea con solo un registro tiene 1 ronda: %d", o.Rondas)
	}
}

// CA-543: el texto sin slug es una linea por tarea, en el orden del
// reporte, con el slug, el estado, las rondas y, por provider, tokens, USD y
// ventana; la ventana dice que es la de la cuenta.
func TestCA543_TextoUnaLineaPorTarea(t *testing.T) {
	fx := ccAlfa(t)
	root := fx.root
	b := ccMeta("beta", "writer", "codex", ccT0.Add(ccMin(300)), ccMin(5))
	b.Usage = ccCodex(900, 400, 9, 0)
	b.Ventanas = []runcmd.VentanaRun{ccV("5h", 60, 63, ccR5)}
	ccRun(t, root, b)
	ccItem(t, root, "beta")
	ccWorktree(t, root, "beta")
	ccRegistro(t, root, "gamma", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(400)), []string{"risk"}, nil, nil)

	r := ccBuild(t, root, "")
	texto := ccTexto(r, false)
	orden := []string{"gamma", "beta", "alfa"}
	pos := -1
	for _, s := range orden {
		lineas := ccLineasCon(texto, s)
		if len(lineas) != 1 {
			t.Fatalf("CA-543: una linea por tarea: %q aparece en %d lineas:\n%s", s, len(lineas), texto)
		}
		if i := strings.Index(texto, lineas[0]); i < pos {
			t.Fatalf("CA-543: las lineas van de la tarea mas reciente a la mas vieja (%q):\n%s", orden, texto)
		} else {
			pos = i
		}
	}
	alfa := ccLineasCon(texto, "alfa")[0]
	for _, s := range []string{"aceptada 2026-11-07 13:00 UTC", "rondas 2", "nuevos 357", "nuevos 9600",
		"0.85 USD equivalente API", "codex no informa costo", "5h 5 puntos", "5h 6.5 puntos"} {
		if !strings.Contains(alfa, s) {
			t.Fatalf("CA-543: la linea de alfa dice %q: %q", s, alfa)
		}
	}
	beta := ccLineasCon(texto, "beta")[0]
	for _, s := range []string{"en curso", "rondas 0", "nuevos 500", "codex no informa costo", "5h 3 puntos"} {
		if !strings.Contains(beta, s) {
			t.Fatalf("CA-543: la linea de beta dice %q: %q", s, beta)
		}
	}
	gamma := ccLineasCon(texto, "gamma")[0]
	for _, s := range []string{"cerrada sin aceptar", "rondas 1"} {
		if !strings.Contains(gamma, s) {
			t.Fatalf("CA-543: la linea de gamma dice %q: %q", s, gamma)
		}
	}
	if !strings.Contains(texto, "(ventana de la cuenta durante los runs)") {
		t.Fatalf("CA-543: la salida con ventana aclara que es la de la cuenta durante los runs:\n%s", texto)
	}
	// el reporte sin slug trae tambien el desglose (para --json)
	if a := ccTarea(t, "CA-543", r, "alfa"); len(a.PorRol) == 0 || len(a.RondasReview) != 2 {
		t.Fatalf("CA-543: sin slug cada tarea trae por_rol y rondas_review: %+v", a)
	}
}

// ---------------------------------------------------------------- CA-544

// CA-544: con slug, exactamente esa tarea, con el desglose por rol: una fila
// por (rol, provider), roles en orden alfabetico, run sin rol = rol "".
func TestCA544_DesglosePorRol(t *testing.T) {
	fx := ccAlfa(t)
	ccRun(t, fx.root, ccMeta("beta", "writer", "claude", ccT0, ccMin(5)))
	r := ccBuild(t, fx.root, "alfa")
	if len(r.Tareas) != 1 || r.Tareas[0].Slug != "alfa" {
		t.Fatalf("CA-544: con slug el reporte es esa tarea sola: %+v", r.Tareas)
	}
	ta := r.Tareas[0]
	type clave struct{ rol, prov string }
	var claves []clave
	for i, g := range ta.PorRol {
		claves = append(claves, clave{g.Rol, g.Provider})
		if i > 0 && ta.PorRol[i-1].Rol > g.Rol {
			t.Fatalf("CA-544: los roles van en orden alfabetico: %+v", ta.PorRol)
		}
	}
	quiero := []clave{{"", "claude"}, {"reviewer", "codex"}, {"test-writer", "claude"}, {"writer", "claude"}, {"writer", "codex"}}
	sort.Slice(claves, func(i, j int) bool {
		return claves[i].rol < claves[j].rol || (claves[i].rol == claves[j].rol && claves[i].prov < claves[j].prov)
	})
	if !reflect.DeepEqual(claves, quiero) {
		t.Fatalf("CA-544: una fila por (rol, provider): %v, fue %v", quiero, claves)
	}

	sinRol := ccPorRol(t, "CA-544", ta, "", "claude")
	ccTokens(t, "CA-544", "sin rol", sinRol.Tokens, ccI(7), ccI(70), ccI(1), nil)
	if sinRol.Runs != 1 || sinRol.CostoUSD != nil || sinRol.SinCosto != 1 || sinRol.Ventanas != nil || sinRol.SinVentana != 1 {
		t.Fatalf("CA-544: el run sin rol: 1 run, sin costo ni ventana: %+v", sinRol)
	}
	rev := ccPorRol(t, "CA-544", ta, "reviewer", "codex")
	// Codex: nuevos = input - cached
	ccTokens(t, "CA-544", "reviewer", rev.Tokens, ccI(9000+400), ccI(1000+100), ccI(100+5), ccI(10))
	if rev.Runs != 5 || rev.CostoUSD != nil || rev.SinCosto != 5 {
		t.Fatalf("CA-544: el reviewer de codex: 5 runs, ninguno con costo: %+v", rev)
	}
	if m := ccMov(t, "CA-544", rev.Gasto, "5h"); !ccCasi(*m.Puntos, 6) || m.AlMenos {
		t.Fatalf("CA-544: la ventana del reviewer: 5 (ronda completa) + 1 (delta): %+v", m)
	}
	if rev.DuracionMS == nil || *rev.DuracionMS != int64(25*time.Minute/time.Millisecond) {
		t.Fatalf("CA-544: el tiempo del reviewer es 25 min: %v", rev.DuracionMS)
	}
	wcl := ccPorRol(t, "CA-544", ta, "writer", "claude")
	// Claude: nuevos = input
	ccTokens(t, "CA-544", "writer claude", wcl.Tokens, ccI(300), ccI(3000), ccI(30), nil)
	if wcl.Runs != 2 || wcl.CostoUSD == nil || !ccCasi(*wcl.CostoUSD, 0.75) || wcl.SinCosto != 0 {
		t.Fatalf("CA-544: el writer de claude: 2 runs, 0.75 USD: %+v", wcl)
	}
	if m := ccMov(t, "CA-544", wcl.Gasto, "5h"); !ccCasi(*m.Puntos, 4) {
		t.Fatalf("CA-544: el writer de claude movio la 5h 2+2: %+v", m)
	}
	wcx := ccPorRol(t, "CA-544", ta, "writer", "codex")
	ccTokens(t, "CA-544", "writer codex", wcx.Tokens, ccI(200), ccI(100), ccI(3), ccI(7))
	tw := ccPorRol(t, "CA-544", ta, "test-writer", "claude")
	ccTokens(t, "CA-544", "test-writer", tw.Tokens, ccI(50), ccI(500), ccI(5), nil)
}

// CA-544: el total por provider, nunca uno entre providers: claude y codex
// por separado, con sus USD y sus ventanas.
func TestCA544_TotalPorProviderNuncaCruzado(t *testing.T) {
	fx := ccAlfa(t)
	ta := ccTarea(t, "CA-544", ccBuild(t, fx.root, "alfa"), "alfa")
	if len(ta.PorProvider) != 2 {
		t.Fatalf("CA-544: un total por provider (claude y codex), ninguno cruzado: %+v", ta.PorProvider)
	}
	cl, ok1 := ta.PorProvider["claude"]
	cx, ok2 := ta.PorProvider["codex"]
	if !ok1 || !ok2 {
		t.Fatalf("CA-544: por_provider tiene claude y codex: %+v", ta.PorProvider)
	}
	ccTokens(t, "CA-544", "claude", cl.Tokens, ccI(357), ccI(3570), ccI(36), nil)
	ccTokens(t, "CA-544", "codex", cx.Tokens, ccI(9600), ccI(1200), ccI(108), ccI(17))
	if cl.Runs != 4 || cx.Runs != 6 || cl.EnCurso != 0 || cx.EnCurso != 0 {
		t.Fatalf("CA-544: runs por provider 4 y 6: %+v %+v", cl, cx)
	}
	if cl.CostoUSD == nil || !ccCasi(*cl.CostoUSD, 0.85) || cl.SinCosto != 1 || cx.CostoUSD != nil || cx.SinCosto != 6 {
		t.Fatalf("CA-544: USD solo lo que informo el provider (claude 0.85, 1 sin costo; codex null, 6 sin costo): %+v %+v", cl, cx)
	}
	if m := ccMov(t, "CA-544", cl, "5h"); !ccCasi(*m.Puntos, 5) || m.AlMenos {
		t.Fatalf("CA-544: claude movio la 5h 5 puntos: %+v", m)
	}
	if m := ccMov(t, "CA-544", cl, "semana"); !ccCasi(*m.Puntos, 2) {
		t.Fatalf("CA-544: claude movio la semana 2 puntos: %+v", m)
	}
	if m := ccMov(t, "CA-544", cx, "5h"); !ccCasi(*m.Puntos, 6.5) || m.AlMenos {
		t.Fatalf("CA-544: codex movio la 5h 6.5 puntos: %+v", m)
	}
	if _, hay := cx.Ventanas["semana"]; hay {
		t.Fatalf("CA-544: codex no informo semana: no se inventa: %+v", cx.Ventanas)
	}
	if cl.SinVentana != 1 || cx.SinVentana != 0 {
		t.Fatalf("CA-544: sin ventana: claude 1 (el run sin rol), codex 0: %d %d", cl.SinVentana, cx.SinVentana)
	}
	if cl.DuracionMS == nil || *cl.DuracionMS != int64(33*time.Minute/time.Millisecond) {
		t.Fatalf("CA-544: el tiempo de claude es la suma de sus runs (33 min): %v", cl.DuracionMS)
	}
}

// CA-544 (propiedad): para cualquier mezcla de runs de dos providers, cada
// total es la suma de SU provider con su regla de nuevos, y nada suma los
// dos.
func TestCA544_PropiedadTokensPorProvider(t *testing.T) {
	r := rand.New(rand.NewSource(544))
	for caso := 0; caso < 40; caso++ {
		root := ccRoot(t)
		suma := map[string][4]int{}
		hay := map[string]bool{}
		n := 1 + r.Intn(8)
		for i := 0; i < n; i++ {
			prov := []string{"claude", "codex"}[r.Intn(2)]
			m := ccMeta("alfa", []string{"writer", "reviewer", ""}[r.Intn(3)], prov, ccT0.Add(ccMin(10*i)), ccMin(5))
			in := 1 + r.Intn(50000)
			cached := r.Intn(in)
			out := r.Intn(5000)
			reas := 0
			if prov == "codex" {
				reas = r.Intn(3) * r.Intn(900)
			}
			m.Usage = &providers.Usage{InputTokens: in, CachedTokens: cached, OutputTokens: out, ReasoningTokens: reas, Turns: 1}
			ccRun(t, root, m)
			s := suma[prov]
			nuevos := in
			if prov == "codex" {
				nuevos = in - cached
			}
			s[0] += nuevos
			s[1] += cached
			s[2] += out
			s[3] += reas
			suma[prov] = s
			hay[prov] = true
		}
		ta := ccTarea(t, "CA-544", ccBuild(t, root, "alfa"), "alfa")
		if len(ta.PorProvider) != len(hay) {
			t.Fatalf("CA-544: caso %d: un total por provider que tiene runs (%v): %+v", caso, hay, ta.PorProvider)
		}
		for prov, s := range suma {
			g := ta.PorProvider[prov]
			var razon *int
			if s[3] > 0 {
				razon = ccI(s[3])
			}
			ccTokens(t, "CA-544", fmt.Sprintf("caso %d %s", caso, prov), g.Tokens, ccI(s[0]), ccI(s[1]), ccI(s[2]), razon)
		}
	}
}

// CA-544: las rondas de review, de la mas vieja a la mas nueva: id,
// cobertura, lentes, hallazgos por severidad (con las tres claves) y el
// gasto de los runs que el registro nombra.
func TestCA544_RondasDeReview(t *testing.T) {
	fx := ccAlfa(t)
	ta := ccTarea(t, "CA-544", ccBuild(t, fx.root, "alfa"), "alfa")
	if len(ta.RondasReview) != 2 {
		t.Fatalf("CA-544: dos rondas (y ninguna fila sin ronda: todos los reviewer tienen ronda): %+v", ta.RondasReview)
	}
	r1, r2 := ta.RondasReview[0], ta.RondasReview[1]
	if r1.ID == nil || *r1.ID != fx.r1.ID || r2.ID == nil || *r2.ID != fx.r2.ID || r1.SinRonda || r2.SinRonda {
		t.Fatalf("CA-544: las rondas son los registros, del mas viejo al mas nuevo: %+v", ta.RondasReview)
	}
	if r1.Cobertura != reviewcmd.CoberturaCompleta || r2.Cobertura != reviewcmd.CoberturaDelta {
		t.Fatalf("CA-544: completa y delta: %q %q", r1.Cobertura, r2.Cobertura)
	}
	if !reflect.DeepEqual(r1.Lentes, reviewcmd.Lentes) || !reflect.DeepEqual(r2.Lentes, []string{"risk"}) {
		t.Fatalf("CA-544: las lentes de cada ronda: %v %v", r1.Lentes, r2.Lentes)
	}
	if !reflect.DeepEqual(r1.Hallazgos, map[string]int{"high": 1, "medium": 0, "low": 2}) ||
		!reflect.DeepEqual(r2.Hallazgos, map[string]int{"high": 0, "medium": 0, "low": 0}) {
		t.Fatalf("CA-544: hallazgos high/medium/low de cada ronda: %v %v", r1.Hallazgos, r2.Hallazgos)
	}
	if r1.Provider != "codex" || r1.Gasto == nil || r2.Gasto == nil {
		t.Fatalf("CA-544: la ronda dice su provider y su gasto: %+v %+v", r1, r2)
	}
	if r1.Gasto.Runs != 4 || r2.Gasto.Runs != 1 {
		t.Fatalf("CA-544: el gasto de la ronda son los runs que nombra (4 y 1): %d %d", r1.Gasto.Runs, r2.Gasto.Runs)
	}
	ccTokens(t, "CA-544", "ronda 1", r1.Gasto.Tokens, ccI(9000), ccI(1000), ccI(100), ccI(10))
	ccTokens(t, "CA-544", "ronda 2", r2.Gasto.Tokens, ccI(400), ccI(100), ccI(5), nil)
	if m := ccMov(t, "CA-544", *r1.Gasto, "5h"); !ccCasi(*m.Puntos, 5) {
		t.Fatalf("CA-544: la ronda completa movio la 5h 5 puntos: %+v", m)
	}
	if r1.Gasto.DuracionMS == nil || *r1.Gasto.DuracionMS != int64(20*time.Minute/time.Millisecond) {
		t.Fatalf("CA-544: el tiempo de la ronda completa es 20 min: %v", r1.Gasto.DuracionMS)
	}
}

// CA-544 (caso limite): una ronda NO ENTREGABLE o con lentes fallidas cuenta
// igual: los runs que el registro nombra cuentan sea cual sea su status.
func TestCA544_RondaConLentesFallidasCuentaIgual(t *testing.T) {
	root := ccRoot(t)
	var rv []runcmd.Meta
	for i, st := range []string{runcmd.StatusDone, runcmd.StatusError, runcmd.StatusCanceled, runcmd.StatusError} {
		m := ccMeta("alfa", "reviewer", "codex", ccT0.Add(ccMin(10*i)), ccMin(5))
		m.Status = st
		m.ExitCode = map[string]int{runcmd.StatusDone: 0, runcmd.StatusError: 1, runcmd.StatusCanceled: 130}[st]
		m.Usage = ccCodex(1000, 0, 10, 0)
		rv = append(rv, ccRun(t, root, m))
	}
	ccRegistro(t, root, "alfa", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(50)), reviewcmd.Lentes, ccLensRuns(reviewcmd.Lentes, rv...), nil)
	ta := ccTarea(t, "CA-544", ccBuild(t, root, "alfa"), "alfa")
	if len(ta.RondasReview) != 1 || ta.RondasReview[0].Gasto == nil || ta.RondasReview[0].Gasto.Runs != 4 {
		t.Fatalf("CA-544: la ronda con lentes fallidas cuenta sus 4 runs: %+v", ta.RondasReview)
	}
	ccTokens(t, "CA-544", "ronda fallida", ta.RondasReview[0].Gasto.Tokens, ccI(4000), ccI(0), ccI(40), nil)
	if g := ta.PorProvider["codex"]; g.Runs != 4 {
		t.Fatalf("CA-544: el costo se gasto igual: el total cuenta los 4 runs: %+v", g)
	}
}

// CA-544: el texto del desglose: tokens por provider con sus rotulos, USD
// rotulado equivalente API, el provider que no informa costo, las rondas con
// cobertura, lentes y hallazgos, los roles (sin rol incluido), el veredicto
// y el commit_final de la tarea aceptada. Ningun numero suma dos providers.
func TestCA544_TextoDelDesglose(t *testing.T) {
	fx := ccAlfa(t)
	texto := ccTexto(ccBuild(t, fx.root, "alfa"), true)
	for _, s := range []string{
		"alfa", "aceptada 2026-11-07 13:00 UTC", fx.veredicto, "abc1234def",
		"nuevos 357", "cache leido 3570", "salida 36", "razonamiento sin dato",
		"nuevos 9600", "cache leido 1200", "salida 108", "razonamiento 17",
		"0.85 USD equivalente API", "codex no informa costo", "costo: sin dato en 1 de 4 runs",
		"writer", "test-writer", "reviewer", "sin rol", "nuevos 9400", "nuevos 300",
		fx.r1.ID, "completa", "lentes risk,reliability,resilience,readability", "high 1 medium 0 low 2",
		fx.r2.ID, "delta", "lentes risk", "high 0 medium 0 low 0",
		"5h 5 puntos", "semana 2 puntos", "5h 6.5 puntos", "(ventana de la cuenta durante los runs)",
		"ventana: sin dato en 1 de 4 runs",
	} {
		if !strings.Contains(texto, s) {
			t.Fatalf("CA-544: el desglose dice %q:\n%s", s, texto)
		}
	}
	// 357+9600, 3570+1200, 36+108: sumas entre providers; 300+200: el
	// writer de los dos providers
	for _, s := range []string{"nuevos 9957", "cache leido 4770", "salida 144", "nuevos 500\n", "nuevos 500 "} {
		if strings.Contains(texto, s) {
			t.Fatalf("CA-544: nunca se suman tokens de dos providers (%q):\n%s", s, texto)
		}
	}
	if strings.Contains(texto, "USD equivalente API") && strings.Contains(texto, "0.00 USD") {
		t.Fatalf("CA-544: codex no informa costo: nunca 0.00 USD:\n%s", texto)
	}
}

// ---------------------------------------------------------------- CA-545

// ccVentanaDe arma una tarea con los runs dados (todos codex salvo que
// digan otro provider) y devuelve el movimiento total de la 5h de codex.
type ccRunV struct {
	ini, fin   int // minutos desde ccT0
	vIni, vFin float64
	reinicia   time.Time
	reiniciada bool
}

func ccMovimientoDe(t *testing.T, runs []ccRunV) Movimiento {
	t.Helper()
	root := ccRoot(t)
	for _, rv := range runs {
		m := ccMeta("alfa", "writer", "codex", ccT0.Add(ccMin(rv.ini)), ccMin(rv.fin-rv.ini))
		v := ccV("5h", rv.vIni, rv.vFin, rv.reinicia)
		v.Reiniciada = rv.reiniciada
		m.Ventanas = []runcmd.VentanaRun{v}
		ccRun(t, root, m)
	}
	return ccMov(t, "CA-545", ccTarea(t, "CA-545", ccBuild(t, root, "alfa"), "alfa").PorProvider["codex"], "5h")
}

// CA-545: la regla de bloques. Los runs del mismo provider que se
// superponen se unen; cada bloque va del inicio del primero en arrancar al
// fin del ultimo en terminar; los bloques se suman. Un reinicio dentro del
// bloque (un run reiniciada o un reinicia_en que cambia mas de 60 s entre
// runs consecutivos) hace que el bloque aporte solo el fin del ultimo en
// terminar y el total diga al menos. Un salto de 1 s no es reinicio.
func TestCA545_ReglaDeBloques(t *testing.T) {
	R := ccR5
	for _, c := range []struct {
		nombre  string
		runs    []ccRunV
		puntos  float64
		alMenos bool
	}{
		{"un run", []ccRunV{{0, 10, 10, 12, R, false}}, 2, false},
		{"dos runs separados se suman", []ccRunV{{0, 10, 10, 12, R, false}, {20, 30, 15, 18, R, false}}, 5, false},
		{"dos runs superpuestos son un bloque (no 4+5)", []ccRunV{{0, 20, 10, 14, R, false}, {5, 30, 11, 16, R, false}}, 6, false},
		{"arrancar cuando el otro termina une el bloque", []ccRunV{{0, 10, 10, 12, R, false}, {10, 20, 12, 15, R, false}}, 5, false},
		{"el fin es el del ultimo en terminar, no el del ultimo en arrancar",
			[]ccRunV{{0, 30, 10, 20, R, false}, {5, 10, 11, 13, R, false}}, 10, false},
		{"un run que arranca antes del ultimo fin del bloque se une",
			[]ccRunV{{0, 30, 10, 14, R, false}, {5, 10, 11, 12, R, false}, {20, 40, 13, 17, R, false}}, 7, false},
		{"un run reiniciada aporta su fin", []ccRunV{{0, 10, 80, 3, R, true}}, 3, true},
		{"un reinicio entre runs del bloque aporta el fin del ultimo",
			[]ccRunV{{0, 20, 90, 95, R, false}, {10, 40, 96, 4, R.Add(5 * time.Hour), false}}, 4, true},
		{"un bloque reiniciado y uno normal se suman y el total es al menos",
			[]ccRunV{{0, 10, 80, 3, R, true}, {60, 70, 5, 7, R, false}}, 5, true},
		{"un salto de 1 s entre runs del bloque no es reinicio",
			[]ccRunV{{0, 20, 10, 14, R, false}, {5, 30, 11, 16, R.Add(time.Second), false}}, 6, false},
		{"60 s exactos no es reinicio",
			[]ccRunV{{0, 20, 10, 14, R, false}, {5, 30, 11, 16, R.Add(60 * time.Second), false}}, 6, false},
		{"61 s es reinicio",
			[]ccRunV{{0, 20, 10, 14, R, false}, {5, 30, 11, 16, R.Add(61 * time.Second), false}}, 16, true},
		{"el reinicio se mira entre runs consecutivos por created_at",
			[]ccRunV{{0, 60, 10, 20, R, false}, {5, 50, 11, 15, R.Add(40 * time.Second), false}, {10, 55, 12, 18, R.Add(80 * time.Second), false}}, 10, false},
		{"un reinicia_en distinto entre bloques separados no es reinicio",
			[]ccRunV{{0, 10, 90, 95, R, false}, {360, 370, 2, 5, R.Add(5 * time.Hour), false}}, 8, false},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			m := ccMovimientoDe(t, c.runs)
			if !ccCasi(*m.Puntos, c.puntos) || m.AlMenos != c.alMenos {
				t.Fatalf("CA-545: %s: %v puntos (al menos %v), fue %v (al menos %v)", c.nombre, c.puntos, c.alMenos, *m.Puntos, m.AlMenos)
			}
		})
	}
}

// CA-545 (propiedad): con una cuenta cuyo uso U(t) solo crece y el mismo
// reinicio para todos, el total es la suma, sobre la union de los intervalos
// de los runs (dos se unen si uno arranca antes o justo cuando termina el
// ultimo del bloque), de U(fin) - U(inicio): nunca cuenta dos veces lo que
// dos runs superpuestos vieron juntos.
func TestCA545_PropiedadUnionDeIntervalos(t *testing.T) {
	r := rand.New(rand.NewSource(545))
	for caso := 0; caso < 60; caso++ {
		// U(minuto): prefijo de tasas por minuto, 0..72 puntos
		u := make([]float64, 721)
		for i := 1; i <= 720; i++ {
			u[i] = u[i-1] + []float64{0, 0.02, 0.05, 0.1}[r.Intn(4)]
		}
		n := 1 + r.Intn(8)
		inicios := r.Perm(600)[:n]
		var runs []ccRunV
		for _, ini := range inicios {
			fin := ini + 1 + r.Intn(120)
			runs = append(runs, ccRunV{ini, fin, u[ini], u[fin], ccR5, false})
		}
		orden := append([]ccRunV(nil), runs...)
		sort.Slice(orden, func(i, j int) bool { return orden[i].ini < orden[j].ini })
		quiero := 0.0
		bIni, bFin := orden[0].ini, orden[0].fin
		for _, rv := range orden[1:] {
			if rv.ini <= bFin {
				if rv.fin > bFin {
					bFin = rv.fin
				}
				continue
			}
			quiero += u[bFin] - u[bIni]
			bIni, bFin = rv.ini, rv.fin
		}
		quiero += u[bFin] - u[bIni]
		m := ccMovimientoDe(t, runs)
		if !ccCasi(*m.Puntos, quiero) || m.AlMenos {
			t.Fatalf("CA-545: caso %d: la union de %+v movio %v, fue %v (al menos %v)", caso, runs, quiero, *m.Puntos, m.AlMenos)
		}
	}
}

// CA-545: los runs de dos providers nunca se unen en un bloque: cada
// provider tiene su total aunque se superpongan.
func TestCA545_DosProvidersNoSeUnen(t *testing.T) {
	root := ccRoot(t)
	a := ccMeta("alfa", "writer", "claude", ccT0, ccMin(30))
	a.Ventanas = []runcmd.VentanaRun{ccV("5h", 10, 14, ccR5)}
	b := ccMeta("alfa", "reviewer", "codex", ccT0.Add(ccMin(5)), ccMin(30))
	b.Ventanas = []runcmd.VentanaRun{ccV("5h", 50, 53, ccR5)}
	ccRun(t, root, a)
	ccRun(t, root, b)
	ta := ccTarea(t, "CA-545", ccBuild(t, root, "alfa"), "alfa")
	if m := ccMov(t, "CA-545", ta.PorProvider["claude"], "5h"); !ccCasi(*m.Puntos, 4) {
		t.Fatalf("CA-545: claude movio su 5h 4 puntos: %+v", m)
	}
	if m := ccMov(t, "CA-545", ta.PorProvider["codex"], "5h"); !ccCasi(*m.Puntos, 3) {
		t.Fatalf("CA-545: codex movio su 5h 3 puntos: %+v", m)
	}
}

// CA-545 y caso limite: un run en curso cuenta en runs y en_curso, pero no
// suma ventana ni tiempo (todavia no tiene fin).
func TestCA545_RunEnCursoNoSumaVentanaNiTiempo(t *testing.T) {
	root := ccRoot(t)
	hecho := ccMeta("alfa", "writer", "codex", ccT0, ccMin(10))
	hecho.Ventanas = []runcmd.VentanaRun{ccV("5h", 10, 12, ccR5)}
	ccRun(t, root, hecho)
	vivo := ccMeta("alfa", "writer", "codex", ccT0.Add(ccMin(5)), 0)
	vivo.Status = runcmd.StatusRunning
	vivo.EndedAt = time.Time{}
	vivo.Ventanas = []runcmd.VentanaRun{ccV("5h", 11, 40, ccR5)}
	ccRun(t, root, vivo)
	r := ccBuild(t, root, "alfa")
	g := ccTarea(t, "CA-545", r, "alfa").PorProvider["codex"]
	if g.Runs != 2 || g.EnCurso != 1 {
		t.Fatalf("CA-545: el run en curso cuenta en runs (2) y en_curso (1): %+v", g)
	}
	if m := ccMov(t, "CA-545", g, "5h"); !ccCasi(*m.Puntos, 2) {
		t.Fatalf("CA-545: el run en curso no suma ventana: %+v", m)
	}
	if g.DuracionMS == nil || *g.DuracionMS != int64(10*time.Minute/time.Millisecond) {
		t.Fatalf("CA-545: el run en curso no suma tiempo: %v", g.DuracionMS)
	}
	if g.SinVentana != 0 {
		t.Fatalf("CA-545: el run en curso no es un run terminado sin ventana: %d", g.SinVentana)
	}
	if texto := ccTexto(r, true); !strings.Contains(texto, "1 en curso") {
		t.Fatalf("CA-545: el desglose dice que hay 1 en curso:\n%s", texto)
	}

	// solo runs en curso: sin ventana ni tiempo, null y no 0
	root2 := ccRoot(t)
	ccRun(t, root2, vivo)
	g2 := ccTarea(t, "CA-545", ccBuild(t, root2, "alfa"), "alfa").PorProvider["codex"]
	if g2.Ventanas != nil || g2.DuracionMS != nil || g2.EnCurso != 1 {
		t.Fatalf("CA-545: con solo un run en curso, ventanas y duracion son null: %+v", g2)
	}
}

// CA-545: el texto de la ventana: "<nombre> <x> puntos" con a lo sumo un
// decimal (3, no 3.0), "<1 punto" para menos de un punto, "al menos " delante
// de la cifra cuando hubo reinicio, y siempre la aclaracion de la cuenta.
func TestCA545_TextoDeLaVentana(t *testing.T) {
	root := ccRoot(t)
	a := ccMeta("alfa", "writer", "codex", ccT0, ccMin(10))
	a.Ventanas = []runcmd.VentanaRun{ccV("5h", 52, 52.4, ccR5), ccV("semana", 22, 25.5, ccR7), ccV("60min", 1, 7, ccR5)}
	ccRun(t, root, a)
	b := ccMeta("alfa", "writer", "claude", ccT0, ccMin(10))
	r := ccV("5h", 97, 3, ccR5.Add(5*time.Hour))
	r.Reiniciada = true
	b.Ventanas = []runcmd.VentanaRun{r, ccV("semana", 10, 12.04, ccR7)}
	ccRun(t, root, b)
	rep := ccBuild(t, root, "alfa")
	ta := ccTarea(t, "CA-545", rep, "alfa")
	if m := ccMov(t, "CA-545", ta.PorProvider["codex"], "5h"); !ccCasi(*m.Puntos, 0.4) {
		t.Fatalf("CA-545: el dato no se redondea: 0.4 puntos: %+v", m)
	}
	for _, detalle := range []bool{true, false} {
		texto := ccTexto(rep, detalle)
		for _, s := range []string{"5h <1 punto", "semana 3.5 puntos", "60min 6 puntos", "al menos 3 puntos",
			"semana 2 puntos", "(ventana de la cuenta durante los runs)"} {
			if !strings.Contains(texto, s) {
				t.Fatalf("CA-545: la salida (detalle=%v) dice %q:\n%s", detalle, s, texto)
			}
		}
		for _, s := range []string{"6.0 puntos", "0.4 puntos", "2.04", "3.50"} {
			if strings.Contains(texto, s) {
				t.Fatalf("CA-545: a lo sumo un decimal, sin .0, y <1 para menos de un punto (%q):\n%s", s, texto)
			}
		}
	}
}

// ---------------------------------------------------------------- CA-546

// CA-546: lo que falta se dice con el conteo: ventana y costo sin dato en n
// de m runs; un provider sin ninguna ventana dice ventana: sin dato.
func TestCA546_SinDatoConElConteo(t *testing.T) {
	root := ccRoot(t)
	for i := 0; i < 3; i++ {
		m := ccMeta("alfa", "writer", "claude", ccT0.Add(ccMin(20*i)), ccMin(10))
		if i < 2 {
			m.Ventanas = []runcmd.VentanaRun{ccV("5h", float64(10+i*5), float64(12+i*5), ccR5)}
			m.Usage = ccClaude(100, 0, 1, ccF(0.5))
		} else {
			m.Usage = ccClaude(100, 0, 1, nil)
		}
		ccRun(t, root, m)
	}
	for i := 0; i < 2; i++ {
		m := ccMeta("alfa", "reviewer", "codex", ccT0.Add(ccMin(100+20*i)), ccMin(10))
		m.Usage = ccCodex(100, 50, 1, 0)
		ccRun(t, root, m)
	}
	r := ccBuild(t, root, "alfa")
	ta := ccTarea(t, "CA-546", r, "alfa")
	cl, cx := ta.PorProvider["claude"], ta.PorProvider["codex"]
	if cl.SinVentana != 1 || cl.SinCosto != 1 || cl.CostoUSD == nil || !ccCasi(*cl.CostoUSD, 1) {
		t.Fatalf("CA-546: claude: 1 de 3 sin ventana, 1 de 3 sin costo, 1 USD: %+v", cl)
	}
	if cx.Ventanas != nil || cx.SinVentana != 2 || cx.CostoUSD != nil || cx.SinCosto != 2 {
		t.Fatalf("CA-546: codex: ninguna ventana (null), 2 sin ventana, sin costo (null): %+v", cx)
	}
	texto := ccTexto(r, true)
	for _, s := range []string{"ventana: sin dato en 1 de 3 runs", "costo: sin dato en 1 de 3 runs", "ventana: sin dato",
		"codex no informa costo", "1.00 USD equivalente API"} {
		if !strings.Contains(texto, s) {
			t.Fatalf("CA-546: lo que falta se dice: %q:\n%s", s, texto)
		}
	}
	lista := ccTexto(ccBuild(t, root, ""), false)
	if !strings.Contains(lista, "ventana: sin dato") || !strings.Contains(lista, "codex no informa costo") {
		t.Fatalf("CA-546: la lista tambien dice lo que falta:\n%s", lista)
	}
}

// CA-546: runs sin usage: los tokens son null y el texto dice sin dato;
// nunca 0.
func TestCA546_TokensSinUsageSonNull(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "writer", "codex", ccT0, ccMin(10)))
	ccRun(t, root, ccMeta("alfa", "writer", "codex", ccT0.Add(ccMin(20)), ccMin(10)))
	r := ccBuild(t, root, "alfa")
	g := ccTarea(t, "CA-546", r, "alfa").PorProvider["codex"]
	ccTokens(t, "CA-546", "sin usage", g.Tokens, nil, nil, nil, nil)
	texto := ccTexto(r, true)
	for _, s := range []string{"nuevos sin dato", "cache leido sin dato", "salida sin dato", "razonamiento sin dato"} {
		if !strings.Contains(texto, s) {
			t.Fatalf("CA-546: sin usage se dice %q:\n%s", s, texto)
		}
	}
	if strings.Contains(texto, "nuevos 0") || strings.Contains(texto, "salida 0") {
		t.Fatalf("CA-546: lo que falta nunca es 0:\n%s", texto)
	}
}

// CA-546 (caso limite): los sidecars ilegibles se cuentan (N runs
// ilegibles) y el comando sigue, con slug y sin el.
func TestCA546_SidecarsIlegiblesSeCuentanSinAbortar(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "writer", "claude", ccT0, ccMin(10)))
	for nombre, cuerpo := range map[string]string{
		"20261107T110000_aaaaa1.meta.json": "{no es json",
		"20261107T110000_aaaaa2.meta.json": "",
		"20261107T110000_aaaaa3.meta.json": "\x00\x01\x02",
	} {
		if err := os.WriteFile(filepath.Join(root, ".hoom", "runs", nombre), []byte(cuerpo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, slug := range []string{"", "alfa"} {
		r, err := Build(root, slug)
		if err != nil {
			t.Fatalf("CA-546: un sidecar ilegible no aborta (slug %q): %v", slug, err)
		}
		if r.Ilegibles != 3 {
			t.Fatalf("CA-546: 3 sidecars ilegibles (slug %q): %d", slug, r.Ilegibles)
		}
		if ta := ccTarea(t, "CA-546", r, "alfa"); ta.PorProvider["claude"].Runs != 1 {
			t.Fatalf("CA-546: los legibles se cuentan igual: %+v", ta.PorProvider)
		}
		if texto := ccTexto(r, slug != ""); !strings.Contains(texto, "3 runs ilegibles") {
			t.Fatalf("CA-546: la salida dice 3 runs ilegibles (slug %q):\n%s", slug, texto)
		}
		if m := ccJSON(t, r); m["ilegibles"] != 3.0 {
			t.Fatalf("CA-546: el JSON cuenta los ilegibles: %v", m["ilegibles"])
		}
	}
}

// CA-546: una ronda de un registro viejo (sin runs) tiene gasto null; los
// runs del reviewer de la tarea que ningun registro nombra van, al final, en
// la fila reviewer (sin ronda), con id null. Los reviewer de otra tarea y los
// nombrados por un registro no entran.
func TestCA546_RondaSinRunsYReviewerSinRonda(t *testing.T) {
	root := ccRoot(t)
	var huerfanos []runcmd.Meta
	for i := 0; i < 3; i++ {
		m := ccMeta("alfa", "reviewer", "codex", ccT0.Add(ccMin(10*i)), ccMin(5))
		m.Usage = ccCodex(1000, 500, 10, 0)
		huerfanos = append(huerfanos, ccRun(t, root, m))
	}
	viejo := ccRegistro(t, root, "alfa", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(30)), []string{"risk"}, nil, nil)
	nombrado := ccMeta("alfa", "reviewer", "codex", ccT0.Add(ccMin(40)), ccMin(5))
	nombrado.Usage = ccCodex(7000, 0, 70, 0)
	ccRun(t, root, nombrado)
	nuevo := ccRegistro(t, root, "alfa", reviewcmd.CoberturaDelta, ccT0.Add(ccMin(46)), []string{"risk"},
		ccLensRuns([]string{"risk"}, nombrado), nil)
	ccRun(t, root, ccMeta("beta", "reviewer", "codex", ccT0, ccMin(5)))

	r := ccBuild(t, root, "alfa")
	ta := ccTarea(t, "CA-546", r, "alfa")
	if len(ta.RondasReview) != 3 {
		t.Fatalf("CA-546: dos rondas y la fila sin ronda: %+v", ta.RondasReview)
	}
	r0, r1, sin := ta.RondasReview[0], ta.RondasReview[1], ta.RondasReview[2]
	if r0.ID == nil || *r0.ID != viejo.ID || r0.Gasto != nil || r0.SinRonda {
		t.Fatalf("CA-546: la ronda del registro viejo, sin runs, tiene gasto null: %+v", r0)
	}
	if r1.ID == nil || *r1.ID != nuevo.ID || r1.Gasto == nil || r1.Gasto.Runs != 1 {
		t.Fatalf("CA-546: la ronda nueva tiene su run: %+v", r1)
	}
	if !sin.SinRonda || sin.ID != nil || sin.Cobertura != "" || sin.Gasto == nil || sin.Gasto.Runs != 3 {
		t.Fatalf("CA-546: la ultima fila es reviewer (sin ronda), id null, con los 3 runs huerfanos de alfa: %+v", sin)
	}
	ccTokens(t, "CA-546", "sin ronda", sin.Gasto.Tokens, ccI(1500), ccI(1500), ccI(30), nil)
	if !reflect.DeepEqual(sin.Hallazgos, map[string]int{"high": 0, "medium": 0, "low": 0}) && sin.Hallazgos != nil {
		t.Fatalf("CA-546: la fila sin ronda no tiene hallazgos propios: %v", sin.Hallazgos)
	}
	if ta.Rondas != 2 {
		t.Fatalf("CA-546: la fila sin ronda no es una ronda: rondas 2: %d", ta.Rondas)
	}
	if texto := ccTexto(r, true); !strings.Contains(texto, "reviewer (sin ronda)") {
		t.Fatalf("CA-546: el desglose muestra reviewer (sin ronda):\n%s", texto)
	}
	m := ccJSON(t, r)
	rr := m["tareas"].([]any)[0].(map[string]any)["rondas_review"].([]any)
	if e := rr[0].(map[string]any); e["gasto"] != nil {
		t.Fatalf("CA-547: la ronda sin runs tiene gasto null en el JSON: %v", e)
	}
	if e := rr[2].(map[string]any); e["id"] != nil || e["sin_ronda"] != true || e["gasto"] == nil {
		t.Fatalf("CA-547: la fila sin ronda tiene id null, sin_ronda true y su gasto: %v", e)
	}
	// el reviewer de beta va en la fila sin ronda de beta, no en la de alfa
	if b := ccTarea(t, "CA-546", ccBuild(t, root, "beta"), "beta"); len(b.RondasReview) != 1 || !b.RondasReview[0].SinRonda {
		t.Fatalf("CA-546: beta tiene su reviewer sin ronda: %+v", b.RondasReview)
	}
}

// CA-546: sin runs de reviewer huerfanos no hay fila sin ronda.
func TestCA546_SinHuerfanosNoHayFilaSinRonda(t *testing.T) {
	fx := ccAlfa(t)
	ta := ccTarea(t, "CA-546", ccBuild(t, fx.root, "alfa"), "alfa")
	for _, rr := range ta.RondasReview {
		if rr.SinRonda || rr.ID == nil {
			t.Fatalf("CA-546: todos los reviewer tienen ronda: no hay fila sin ronda: %+v", ta.RondasReview)
		}
	}
	if texto := ccTexto(ccBuild(t, fx.root, "alfa"), true); strings.Contains(texto, "sin ronda") {
		t.Fatalf("CA-546: sin huerfanos el texto no dice sin ronda:\n%s", texto)
	}
}

// CA-546: un slug sin runs ni reviews es el error del contrato, aunque
// tenga item, worktree o veredicto; los runs sin task no son de ningun slug.
func TestCA546_SlugSinRunsNiReviewsEsError(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("", "writer", "claude", ccT0, ccMin(5)))
	ccRun(t, root, ccMeta("otra", "writer", "claude", ccT0, ccMin(5)))
	ccItem(t, root, "nada")
	ccWorktree(t, root, "nada")
	ccVeredicto(t, root, "nada", ccT0, true, false)
	for _, slug := range []string{"nada", "inexistente"} {
		_, err := Build(root, slug)
		if err == nil || err.Error() != fmt.Sprintf(ErrSinNada, slug) {
			t.Fatalf("CA-546: %s sin runs ni reviews: %q, fue %v", slug, fmt.Sprintf(ErrSinNada, slug), err)
		}
	}
	if fmt.Sprintf(ErrSinNada, "x") != "hoom costo: no hay runs ni reviews de la tarea x" {
		t.Fatalf("CA-546: el texto del contrato: %q", fmt.Sprintf(ErrSinNada, "x"))
	}
}

// ---------------------------------------------------------------- CA-547

// CA-547: el esquema: arriba tareas (lista); por tarea exactamente slug,
// estado, aceptada_en, veredicto, commit_final, rondas, por_provider,
// por_rol y rondas_review.
func TestCA547_EsquemaDelJSON(t *testing.T) {
	fx := ccAlfa(t)
	ccRun(t, fx.root, ccMeta("beta", "writer", "codex", ccT0.Add(ccMin(500)), ccMin(5)))
	for _, slug := range []string{"", "alfa"} {
		m := ccJSON(t, ccBuild(t, fx.root, slug))
		tareas, ok := m["tareas"].([]any)
		if !ok || len(tareas) == 0 {
			t.Fatalf("CA-547: arriba hay tareas (lista): %v", m)
		}
		claves := []string{"aceptada_en", "commit_final", "estado", "por_provider", "por_rol", "rondas", "rondas_review", "slug", "veredicto"}
		for _, e := range tareas {
			ta := e.(map[string]any)
			var got []string
			for k := range ta {
				got = append(got, k)
			}
			sort.Strings(got)
			if !reflect.DeepEqual(got, claves) {
				t.Fatalf("CA-547: cada tarea tiene exactamente %v: %v", claves, got)
			}
			// la forma: objeto por provider y listas. Si una lista vacia sale
			// [] o null no lo fija el spec: solo se exige la forma cuando hay
			// algo (las tareas de este fixture tienen runs; beta no tiene
			// rondas)
			if _, ok := ta["por_provider"].(map[string]any); !ok {
				t.Fatalf("CA-547: por_provider es un objeto por provider: %v", ta["por_provider"])
			}
			if _, ok := ta["por_rol"].([]any); !ok {
				t.Fatalf("CA-547: por_rol es una lista: %v", ta["por_rol"])
			}
			if v := ta["rondas_review"]; v != nil {
				if _, ok := v.([]any); !ok {
					t.Fatalf("CA-547: rondas_review es una lista: %v", v)
				}
			} else if ta["slug"] == "alfa" {
				t.Fatalf("CA-547: alfa tiene rondas: rondas_review es su lista: %v", ta)
			}
		}
	}
	m := ccJSON(t, ccBuild(t, fx.root, "alfa"))
	ta := m["tareas"].([]any)[0].(map[string]any)
	if ta["slug"] != "alfa" || ta["estado"] != EstadoAceptada || ta["veredicto"] != fx.veredicto ||
		ta["commit_final"] != "abc1234def" || ta["rondas"] != 2.0 || ta["aceptada_en"] != "2026-11-07T13:00:00Z" {
		t.Fatalf("CA-547: los valores de la tarea aceptada: %v", ta)
	}
	cl := ta["por_provider"].(map[string]any)["claude"].(map[string]any)
	for _, k := range []string{"runs", "en_curso", "tokens", "costo_usd", "sin_costo", "ventanas", "sin_ventana", "duracion_ms"} {
		if _, hay := cl[k]; !hay {
			t.Fatalf("CA-547: el gasto de un provider trae %q: %v", k, cl)
		}
	}
	tok := cl["tokens"].(map[string]any)
	for _, k := range []string{"nuevos", "cache_leido", "salida", "razonamiento"} {
		if _, hay := tok[k]; !hay {
			t.Fatalf("CA-547: tokens trae %q: %v", k, tok)
		}
	}
	rol := ta["por_rol"].([]any)[0].(map[string]any)
	for _, k := range []string{"rol", "provider", "runs", "tokens", "costo_usd", "ventanas"} {
		if _, hay := rol[k]; !hay {
			t.Fatalf("CA-547: una fila de por_rol trae %q: %v", k, rol)
		}
	}
	ronda := ta["rondas_review"].([]any)[0].(map[string]any)
	for _, k := range []string{"id", "sin_ronda", "cobertura", "lentes", "hallazgos", "provider", "gasto"} {
		if _, hay := ronda[k]; !hay {
			t.Fatalf("CA-547: una ronda trae %q: %v", k, ronda)
		}
	}
	h := ronda["hallazgos"].(map[string]any)
	if !reflect.DeepEqual(h, map[string]any{"high": 1.0, "medium": 0.0, "low": 2.0}) {
		t.Fatalf("CA-547: hallazgos siempre con high, medium y low (0 es un conteo real): %v", h)
	}
	if _, hay := ta["UltimaActividad"]; hay {
		t.Fatalf("CA-547: la actividad no es parte del JSON: %v", ta)
	}
}

// CA-547: cada cifra que falta es null, nunca 0: la tarea no aceptada
// (aceptada_en, veredicto, commit_final), el costo de codex, el razonamiento
// de claude, la ventana y la duracion de lo que no la tiene.
func TestCA547_LoQueFaltaEsNullNuncaCero(t *testing.T) {
	root := ccRoot(t)
	a := ccMeta("beta", "writer", "claude", ccT0, ccMin(10))
	a.Usage = ccClaude(100, 10, 1, nil)
	ccRun(t, root, a)
	b := ccMeta("beta", "reviewer", "codex", ccT0.Add(ccMin(20)), ccMin(10))
	ccRun(t, root, b)
	c := ccMeta("beta", "reviewer", "codex", ccT0.Add(ccMin(40)), 0)
	c.Status = runcmd.StatusRunning
	c.EndedAt = time.Time{}
	ccRun(t, root, c)
	ccVeredicto(t, root, "beta", ccT0.Add(ccMin(30)), true, false)
	m := ccJSON(t, ccBuild(t, root, "beta"))
	ta := m["tareas"].([]any)[0].(map[string]any)
	for _, k := range []string{"aceptada_en", "veredicto", "commit_final"} {
		if v, hay := ta[k]; !hay || v != nil {
			t.Fatalf("CA-547: una tarea no aceptada tiene %s null: %v", k, ta)
		}
	}
	pp := ta["por_provider"].(map[string]any)
	cl := pp["claude"].(map[string]any)
	if cl["costo_usd"] != nil || cl["ventanas"] != nil {
		t.Fatalf("CA-547: claude sin costo informado ni ventana: costo_usd y ventanas null: %v", cl)
	}
	if tok := cl["tokens"].(map[string]any); tok["razonamiento"] != nil || tok["nuevos"] != 100.0 || tok["cache_leido"] != 10.0 {
		t.Fatalf("CA-547: claude no informa razonamiento: null; lo informado se suma: %v", tok)
	}
	cx := pp["codex"].(map[string]any)
	tok := cx["tokens"].(map[string]any)
	for _, k := range []string{"nuevos", "cache_leido", "salida", "razonamiento"} {
		if v, hay := tok[k]; !hay || v != nil {
			t.Fatalf("CA-547: codex sin usage: tokens.%s null, nunca 0: %v", k, tok)
		}
	}
	if cx["costo_usd"] != nil || cx["ventanas"] != nil {
		t.Fatalf("CA-547: codex sin costo ni ventana: null: %v", cx)
	}
	if cx["duracion_ms"] == nil || cx["duracion_ms"] == 0.0 {
		t.Fatalf("CA-547: el run terminado de codex tiene duracion: %v", cx)
	}
}

// CA-547 (propiedad): para cualquier mezcla de runs con y sin usage, costo,
// razonamiento y ventana, toda cifra del JSON es null exactamente cuando
// ningun run la informo, y si no es la suma de lo informado.
func TestCA547_PropiedadNuncaCeroDondeFalta(t *testing.T) {
	r := rand.New(rand.NewSource(547))
	for caso := 0; caso < 40; caso++ {
		root := ccRoot(t)
		type acum struct {
			runs, enCurso, sinVentana, terminados int
			usage, costo, razon, ventana          bool
			nuevos, costoUSD, razonN              float64
			durMS                                 int64
		}
		esperado := map[string]*acum{}
		n := 1 + r.Intn(7)
		for i := 0; i < n; i++ {
			prov := []string{"claude", "codex"}[r.Intn(2)]
			if esperado[prov] == nil {
				esperado[prov] = &acum{}
			}
			e := esperado[prov]
			m := ccMeta("alfa", "writer", prov, ccT0.Add(ccMin(30*i)), ccMin(1+r.Intn(20)))
			e.runs++
			if r.Intn(6) == 0 {
				m.Status = runcmd.StatusRunning
				m.EndedAt = time.Time{}
				e.enCurso++
				ccRun(t, root, m)
				continue
			}
			e.terminados++
			e.durMS += m.EndedAt.Sub(m.CreatedAt).Milliseconds()
			if r.Intn(5) < 3 {
				in := 1 + r.Intn(9000)
				cached := r.Intn(in)
				u := &providers.Usage{InputTokens: in, CachedTokens: cached, OutputTokens: r.Intn(900), Turns: 1}
				if prov == "claude" && r.Intn(2) == 0 {
					c := float64(1+r.Intn(1000)) / 1000
					u.CostUSD = &c
					e.costo = true
					e.costoUSD += c
				}
				if prov == "codex" && r.Intn(2) == 0 {
					u.ReasoningTokens = 1 + r.Intn(500)
					e.razon = true
					e.razonN += float64(u.ReasoningTokens)
				}
				if prov == "codex" {
					e.nuevos += float64(in - cached)
				} else {
					e.nuevos += float64(in)
				}
				e.usage = true
				m.Usage = u
			}
			if r.Intn(2) == 0 {
				m.Ventanas = []runcmd.VentanaRun{ccV("5h", 10, 11, ccR5)}
				e.ventana = true
			} else {
				e.sinVentana++
			}
			ccRun(t, root, m)
		}
		ta := ccJSON(t, ccBuild(t, root, "alfa"))["tareas"].([]any)[0].(map[string]any)
		pp := ta["por_provider"].(map[string]any)
		if len(pp) != len(esperado) {
			t.Fatalf("CA-547: caso %d: un total por provider: %v", caso, pp)
		}
		for prov, e := range esperado {
			g := pp[prov].(map[string]any)
			tok := g["tokens"].(map[string]any)
			nulo := func(que string, v any, hay bool, quiero float64) {
				t.Helper()
				if !hay {
					if v != nil {
						t.Fatalf("CA-547: caso %d %s: %s sin dato es null, nunca 0: %v", caso, prov, que, v)
					}
					return
				}
				f, ok := v.(float64)
				if !ok || !ccCasi(f, quiero) {
					t.Fatalf("CA-547: caso %d %s: %s es %v, fue %v", caso, prov, que, quiero, v)
				}
			}
			nulo("tokens.nuevos", tok["nuevos"], e.usage, e.nuevos)
			nulo("tokens.razonamiento", tok["razonamiento"], e.razon, e.razonN)
			nulo("costo_usd", g["costo_usd"], e.costo, e.costoUSD)
			nulo("duracion_ms", g["duracion_ms"], e.terminados > 0, float64(e.durMS))
			if (g["ventanas"] == nil) == e.ventana {
				t.Fatalf("CA-547: caso %d %s: ventanas null solo si ningun run terminado tiene ventana (%v): %v", caso, prov, e.ventana, g["ventanas"])
			}
			if g["runs"] != float64(e.runs) || g["en_curso"] != float64(e.enCurso) || g["sin_ventana"] != float64(e.sinVentana) {
				t.Fatalf("CA-547: caso %d %s: runs %d, en_curso %d, sin_ventana %d: %v", caso, prov, e.runs, e.enCurso, e.sinVentana, g)
			}
		}
	}
}
