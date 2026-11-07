// Tests del writer para los hallazgos de la review 1 de costo-por-spec
// (registro 20261107T163525_52e14a). Cada uno fija el arreglo de un hallazgo
// con su id y el criterio que protege.
package costcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

var r1T0 = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

func r1Avisa(t *testing.T, r Reporte, frag string) {
	t.Helper()
	for _, a := range r.Avisos {
		if strings.Contains(a, frag) {
			return
		}
	}
	t.Fatalf("los avisos dicen %q: %q", frag, r.Avisos)
}

// Hallazgos ffdf21 y 6a39f7 — CA-543/CA-544: el costo es hasta quedar
// aceptada. Un run y una ronda posteriores a hecho_en no suman; se cuentan
// aparte y se dicen.
func TestHallazgo_ffdf21_6a39f7_CA544_LoPosteriorAAceptadaNoSuma(t *testing.T) {
	root := ccRoot(t)
	ccItem(t, root, "alfa")
	antes := ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5))
	antes.Usage = ccCodex(1000, 400, 50, 0)
	ccRun(t, root, antes)
	despues := ccMeta("alfa", "reviewer", "codex", r1T0.Add(3*time.Hour), ccMin(5))
	despues.Usage = ccCodex(9000, 0, 900, 0)
	ccRun(t, root, despues)
	ccRegistro(t, root, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, antes), nil)
	ccRegistro(t, root, "alfa", "delta", r1T0.Add(3*time.Hour+ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, despues), nil)
	ccAceptar(t, root, "alfa", "abc123", r1T0.Add(time.Hour))

	rep := ccBuild(t, root, "alfa")
	ta := ccTarea(t, "CA-544", rep, "alfa")
	if g := ta.PorProvider["codex"]; g.Runs != 1 || ccInt(g.Tokens.Nuevos) != "600" {
		t.Fatalf("solo el run anterior a hecho_en suma: %+v nuevos %s", g, ccInt(g.Tokens.Nuevos))
	}
	if ta.Rondas != 1 || len(ta.RondasReview) != 1 {
		t.Fatalf("solo la ronda anterior a hecho_en cuenta: rondas %d, filas %d", ta.Rondas, len(ta.RondasReview))
	}
	if ta.DespuesDeAceptada == nil || ta.DespuesDeAceptada.Runs != 1 || ta.DespuesDeAceptada.Rondas != 1 {
		t.Fatalf("lo de despues se cuenta aparte: %+v", ta.DespuesDeAceptada)
	}
	r1Avisa(t, rep, "despues de aceptada")
	if txt := ccTexto(rep, true); !strings.Contains(txt, "despues de aceptada: 1 run y 1 ronda, fuera del costo") {
		t.Fatalf("el desglose lo dice:\n%s", txt)
	}
	if !strings.Contains(string(mustJSON(t, rep)), "despues de aceptada") {
		t.Fatalf("el JSON lo lleva en avisos: %v", ccJSON(t, rep)["avisos"])
	}
}

func mustJSON(t *testing.T, r Reporte) []byte {
	t.Helper()
	raw, err := JSONBytes(r)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// Hallazgo a6c078 — CA-546: un hallazgo que la ronda nombra y hoom no puede
// leer no es un cero: queda sin dato y se dice.
func TestHallazgo_a6c078_CA546_HallazgoIlegibleQuedaSinDato(t *testing.T) {
	root := ccRoot(t)
	high := ccHallazgo(t, root, "alfa", "high")
	run := ccRun(t, root, ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5)))
	ccRegistro(t, root, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run),
		[]string{high, "20261107T100000_zzzzzz"})
	rep := ccBuild(t, root, "alfa")
	r := ccTarea(t, "CA-546", rep, "alfa").RondasReview[0]
	if r.Hallazgos["high"] != 1 || r.HallazgosSinDato != 1 {
		t.Fatalf("un high legible y uno sin dato: %+v sin dato %d", r.Hallazgos, r.HallazgosSinDato)
	}
	if txt := ccTexto(rep, true); !strings.Contains(txt, "hallazgos sin dato 1") {
		t.Fatalf("el desglose lo dice:\n%s", txt)
	}
}

// Hallazgo eb2509 — CA-546: un registro de review ilegible o un item que no
// se puede leer no pasan por hechos negativos: se dicen como avisos.
func TestHallazgo_eb2509_CA546_LoIlegibleSeAvisa(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "writer", "claude", r1T0, ccMin(5)))
	dir := filepath.Join(root, ".hoom", "reviews")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "20261107T100000_abcdef.json"), []byte("{basura"), 0o644); err != nil {
		t.Fatal(err)
	}
	items := filepath.Join(root, ".hoom", "items")
	if err := os.MkdirAll(items, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(items, "alfa.yaml"), []byte("titulo: [sin cerrar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep := ccBuild(t, root, "alfa")
	r1Avisa(t, rep, "1 registros de review ilegibles")
	r1Avisa(t, rep, `el item de "alfa" no se pudo leer`)
	if txt := ccTexto(rep, true); !strings.Contains(txt, "aviso: 1 registros de review ilegibles") {
		t.Fatalf("el texto muestra los avisos:\n%s", txt)
	}
	if j := ccJSON(t, rep); j["avisos"] == nil {
		t.Fatalf("el JSON tiene avisos: %v", j)
	}
}

// Hallazgo 0cc0ae — CA-544: un registro que nombra el run de OTRA tarea no
// lo suma a la suya (lo contaria dos veces): se dice.
func TestHallazgo_0cc0ae_CA544_RunDeOtraTareaNoSeSuma(t *testing.T) {
	root := ccRoot(t)
	ajeno := ccMeta("beta", "reviewer", "codex", r1T0, ccMin(5))
	ajeno.Usage = ccCodex(1000, 0, 10, 0)
	ccRun(t, root, ajeno)
	ccRegistro(t, root, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, ajeno), nil)
	rep := ccBuild(t, root, "")
	alfa := ccTarea(t, "CA-544", rep, "alfa")
	if len(alfa.PorProvider) != 0 {
		t.Fatalf("alfa no suma el run de beta: %+v", alfa.PorProvider)
	}
	if g := alfa.RondasReview[0].Gasto; g == nil || g.Runs != 0 {
		t.Fatalf("la ronda de alfa no suma el run de beta: %+v", g)
	}
	if beta := ccTarea(t, "CA-544", rep, "beta"); beta.PorProvider["codex"].Runs != 1 {
		t.Fatalf("beta conserva su run: %+v", beta.PorProvider)
	}
	r1Avisa(t, rep, `que es de la tarea "beta"`)
}

// Hallazgos ec01ff y 77285c — CA-546: tokens de una mezcla de runs con y sin
// cifras dicen cuantos no las tienen, y un usage que solo trae costo no
// aporta ceros de tokens.
func TestHallazgo_ec01ff_77285c_CA546_CoberturaDeTokens(t *testing.T) {
	root := ccRoot(t)
	con := ccMeta("alfa", "writer", "claude", r1T0, ccMin(5))
	con.Usage = ccClaude(100, 50, 10, ccF(0.1))
	ccRun(t, root, con)
	soloCosto := ccMeta("alfa", "writer", "claude", r1T0.Add(time.Hour), ccMin(5))
	soloCosto.Usage = &providers.Usage{CostUSD: ccF(0.2), DurationMS: 1000}
	ccRun(t, root, soloCosto)
	rep := ccBuild(t, root, "alfa")
	g := ccTarea(t, "CA-546", rep, "alfa").PorProvider["claude"]
	if g.SinUso != 1 || ccInt(g.Tokens.Nuevos) != "100" {
		t.Fatalf("un run con tokens y uno sin: sin_uso %d nuevos %s", g.SinUso, ccInt(g.Tokens.Nuevos))
	}
	if txt := ccTexto(rep, true); !strings.Contains(txt, "tokens: sin dato en 1 de 2 runs") {
		t.Fatalf("el desglose dice la cobertura:\n%s", txt)
	}

	root2 := ccRoot(t)
	solo := ccMeta("alfa", "writer", "claude", r1T0, ccMin(5))
	solo.Usage = &providers.Usage{CostUSD: ccF(0.2), Turns: 1}
	ccRun(t, root2, solo)
	g2 := ccTarea(t, "CA-546", ccBuild(t, root2, "alfa"), "alfa").PorProvider["claude"]
	if g2.Tokens.Nuevos != nil || g2.Tokens.CacheLeido != nil || g2.Tokens.Salida != nil {
		t.Fatalf("un usage sin tokens no da ceros: %+v", g2.Tokens)
	}
}

// Hallazgo 1f72e7 — CA-544: un provider cuya contabilidad hoom no conoce no
// recibe la de Claude: sus tokens nuevos quedan sin dato.
func TestHallazgo_1f72e7_CA544_ProviderSinContabilidadConocida(t *testing.T) {
	root := ccRoot(t)
	m := ccMeta("alfa", "writer", "gemini", r1T0, ccMin(5))
	m.Usage = &providers.Usage{InputTokens: 500, CachedTokens: 200, OutputTokens: 30}
	ccRun(t, root, m)
	g := ccTarea(t, "CA-544", ccBuild(t, root, "alfa"), "alfa").PorProvider["gemini"]
	if g.Tokens.Nuevos != nil || ccInt(g.Tokens.CacheLeido) != "200" || ccInt(g.Tokens.Salida) != "30" {
		t.Fatalf("nuevos sin dato, el resto como vino: %+v", g.Tokens)
	}
}

// Hallazgo ba9b59 — CA-547: serializar no cambia el reporte de quien llama.
func TestHallazgo_ba9b59_CA547_JSONBytesNoCambiaElReporte(t *testing.T) {
	id := "20261107T100000_abcdef"
	r := Reporte{Tareas: []Tarea{{Slug: "alfa", RondasReview: []Ronda{{ID: &id}}}}}
	if _, err := JSONBytes(r); err != nil {
		t.Fatal(err)
	}
	ta := r.Tareas[0]
	if ta.PorProvider != nil || ta.PorRol != nil || ta.RondasReview[0].Lentes != nil || ta.RondasReview[0].Hallazgos != nil || r.Avisos != nil {
		t.Fatalf("JSONBytes relleno el reporte original: %+v", ta)
	}
}

// CA-544 (encontrado al medir esta misma spec): mientras la tarea esta en
// curso, su registro y sus hallazgos viven en su worktree; la ronda se ve
// desde la raiz igual, con sus runs.
func TestCA544_LaRondaDelWorktreeSeVeDesdeLaRaiz(t *testing.T) {
	root := ccRoot(t)
	ccWorktree(t, root, "alfa")
	wt := filepath.Join(root, ".hoom", "worktrees", "alfa")
	run := ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5))
	run.Usage = ccCodex(1000, 400, 50, 0)
	ccRun(t, root, run)
	low := ccHallazgo(t, wt, "alfa", "low")
	ccRegistro(t, wt, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), []string{low})
	ta := ccTarea(t, "CA-544", ccBuild(t, root, "alfa"), "alfa")
	if ta.Rondas != 1 || len(ta.RondasReview) != 1 || ta.RondasReview[0].SinRonda {
		t.Fatalf("la ronda del worktree cuenta y no hay fila sin ronda: %+v", ta.RondasReview)
	}
	r := ta.RondasReview[0]
	if r.Gasto == nil || r.Gasto.Runs != 1 || r.Hallazgos["low"] != 1 || r.HallazgosSinDato != 0 {
		t.Fatalf("la ronda trae su run y su hallazgo: %+v %+v", r.Gasto, r.Hallazgos)
	}
}
