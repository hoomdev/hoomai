// Tests del writer para los hallazgos de la delta 2 de costo-por-spec
// (registro 20261107T180547_fe87f0).
package costcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Hallazgos e4ed1b, 8562a7 y b68eab — CA-544: una fuente que no puede dar
// un registro no se lo esconde a la que si. El worktree alfa trae la copia
// del registro de beta (integro su rama) y se recorre antes que beta: la
// ronda de beta sigue contando, y no hay aviso porque alguien la dio.
func TestHallazgo_e4ed1b_8562a7_b68eab_CA544_UnaCopiaAjenaNoEscondeLaPropia(t *testing.T) {
	root := ccRoot(t)
	ccWorktree(t, root, "alfa")
	ccWorktree(t, root, "beta")
	wta := filepath.Join(root, ".hoom", "worktrees", "alfa")
	wtb := filepath.Join(root, ".hoom", "worktrees", "beta")
	run := ccMeta("beta", "reviewer", "codex", r1T0, ccMin(5))
	ccRun(t, root, run)
	rec := ccRegistro(t, wtb, "beta", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
	src := filepath.Join(wtb, ".hoom", "reviews", rec.ID+".json")
	raw, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(wta, ".hoom", "reviews"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wta, ".hoom", "reviews", rec.ID+".json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	r := ccBuild(t, root, "beta")
	ta := ccTarea(t, "CA-544", r, "beta")
	if ta.Rondas != 1 || len(ta.RondasReview) != 1 || ta.RondasReview[0].SinRonda {
		t.Fatalf("la ronda de beta cuenta aunque alfa traiga su copia: %+v", ta.RondasReview)
	}
	if len(r.Avisos) != 0 {
		t.Fatalf("beta dio el registro: no hay nada que avisar: %q", r.Avisos)
	}
}

// Hallazgos 3868dd, 835709 y ecf8ac — CA-544: un nombre de la evidencia con
// un salto de linea no inventa una linea: sale escapado, y cada linea del
// reporte es una sola.
func TestHallazgo_3868dd_835709_CA544_UnSaltoDeLaEvidenciaNoHaceLinea(t *testing.T) {
	root := ccRoot(t)
	run := ccMeta("alfa", "reviewer", "codex\naviso: falso", r1T0, ccMin(5))
	run.Ventanas = []runcmd.VentanaRun{ccV("5h\r\nmentira", 10, 12, r1T0)}
	ccRun(t, root, run)
	ccRegistro(t, root, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk\nlow 9"}, ccLensRuns([]string{"risk"}, run), nil)
	limpio := ccRoot(t)
	ok := ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5))
	ok.Ventanas = []runcmd.VentanaRun{ccV("5h", 10, 12, r1T0)}
	ccRun(t, limpio, ok)
	ccRegistro(t, limpio, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, ok), nil)
	for _, slug := range []string{"", "alfa"} {
		out := ccTexto(ccBuild(t, root, slug), slug != "")
		base := ccTexto(ccBuild(t, limpio, slug), slug != "")
		if strings.Count(out, "\n") != strings.Count(base, "\n") || strings.Contains(out, "\r") {
			t.Fatalf("las mismas lineas que sin saltos en la evidencia:\n%s\n---\n%s", out, base)
		}
		if !strings.Contains(out, `codex\naviso: falso`) {
			t.Fatalf("el salto sale escapado, a la vista:\n%s", out)
		}
		if slug != "" && (!strings.Contains(out, `risk\nlow 9`) || !strings.Contains(out, `5h\r\nmentira`)) {
			t.Fatalf("lente y ventana escapadas:\n%s", out)
		}
	}
}

// ecf8ac — CA-544: la salida no depende de como se parten las escrituras:
// cada linea llega entera en una escritura que termina en su unico salto.
func TestHallazgo_ecf8ac_CA544_CadaLineaEsUnaEscritura(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "reviewer", "coñdex", r1T0, ccMin(5)))
	var w escrituras
	Render(&w, ccBuild(t, root, "alfa"), true)
	if len(w) == 0 {
		t.Fatal("el reporte escribe algo")
	}
	for _, e := range w {
		if strings.Count(e, "\n") != 1 || !strings.HasSuffix(e, "\n") {
			t.Fatalf("una escritura por linea, con su salto al final: %q", e)
		}
	}
	if !strings.Contains(strings.Join(w, ""), "coñdex") {
		t.Fatalf("el texto valido no se escapa: %q", w)
	}
}

type escrituras []string

func (e *escrituras) Write(p []byte) (int, error) {
	*e = append(*e, string(p))
	return len(p), nil
}
