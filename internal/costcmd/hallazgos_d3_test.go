// Tests del writer para los hallazgos de la delta 3 de costo-por-spec
// (registro 20261108T041941_a0a465).
package costcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// d3Copiar escribe en el worktree dst la copia del registro id de src,
// cambiada por cambio (nil = identica).
func d3Copiar(t *testing.T, src, dst, id string, cambio func(string) string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(src, ".hoom", "reviews", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	if cambio != nil {
		if s2 := cambio(s); s2 != s {
			s = s2
		} else {
			t.Fatal("fixture: el cambio no cambio nada")
		}
	}
	if err := os.MkdirAll(filepath.Join(dst, ".hoom", "reviews"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dst, ".hoom", "reviews", id+".json"), []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Hallazgos 75b0ce y a503df — CA-546: una copia de un registro se calla
// solo cuando es la misma que la que cuenta. Una copia distinta se dice,
// sea porque su fuente no podia darla (otra tarea, tarea que no es slug) o
// porque llego despues.
func TestHallazgo_75b0ce_a503df_CA546_UnaCopiaDistintaSeAvisa(t *testing.T) {
	lente := func(s string) string { return strings.Replace(s, `"risk"`, `"readability"`, 1) }

	// identical copies, in the project and in two worktrees: nothing to say
	root := ccRoot(t)
	ccWorktree(t, root, "alfa")
	ccWorktree(t, root, "beta")
	wta := filepath.Join(root, ".hoom", "worktrees", "alfa")
	wtb := filepath.Join(root, ".hoom", "worktrees", "beta")
	run := ccMeta("beta", "reviewer", "codex", r1T0, ccMin(5))
	ccRun(t, root, run)
	rec := ccRegistro(t, root, "beta", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
	d3Copiar(t, root, wta, rec.ID, nil)
	d3Copiar(t, root, wtb, rec.ID, nil)
	if r := ccBuild(t, root, "beta"); len(r.Avisos) != 0 || ccTarea(t, "CA-546", r, "beta").Rondas != 1 {
		t.Fatalf("copias identicas: una ronda y sin avisos: %q", r.Avisos)
	}

	// a foreign copy that differs is said, though beta gave the record
	root = ccRoot(t)
	ccWorktree(t, root, "alfa")
	ccWorktree(t, root, "beta")
	wta = filepath.Join(root, ".hoom", "worktrees", "alfa")
	wtb = filepath.Join(root, ".hoom", "worktrees", "beta")
	run = ccMeta("beta", "reviewer", "codex", r1T0, ccMin(5))
	ccRun(t, root, run)
	rec = ccRegistro(t, wtb, "beta", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
	d3Copiar(t, wtb, wta, rec.ID, lente)
	r := ccBuild(t, root, "beta")
	if ta := ccTarea(t, "CA-546", r, "beta"); ta.Rondas != 1 || ta.RondasReview[0].Lentes[0] != "risk" {
		t.Fatalf("cuenta la copia de beta: %+v", ta.RondasReview)
	}
	r1Avisa(t, r, `el worktree "alfa" tiene el registro `+rec.ID)

	// a project copy whose task is no slug is said, though a worktree gives
	// a good copy
	root = ccRoot(t)
	ccWorktree(t, root, "alfa")
	wta = filepath.Join(root, ".hoom", "worktrees", "alfa")
	run = ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5))
	ccRun(t, root, run)
	rec = ccRegistro(t, wta, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
	d3Copiar(t, wta, root, rec.ID, func(s string) string { return strings.Replace(s, `"task": "alfa"`, `"task": "alfa\u001b[2J"`, 1) })
	r = ccBuild(t, root, "alfa")
	if ccTarea(t, "CA-546", r, "alfa").Rondas != 1 {
		t.Fatal("la copia buena del worktree cuenta")
	}
	r1Avisa(t, r, "tiene una tarea que no es un slug")

	// a later good copy that differs is said too: the first one counts
	root = ccRoot(t)
	ccWorktree(t, root, "alfa")
	wta = filepath.Join(root, ".hoom", "worktrees", "alfa")
	run = ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5))
	ccRun(t, root, run)
	rec = ccRegistro(t, root, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
	d3Copiar(t, root, wta, rec.ID, lente)
	r = ccBuild(t, root, "alfa")
	if ta := ccTarea(t, "CA-546", r, "alfa"); ta.Rondas != 1 || ta.RondasReview[0].Lentes[0] != "risk" {
		t.Fatalf("cuenta la copia del proyecto: %+v", ta.RondasReview)
	}
	r1Avisa(t, r, "tiene otra copia, distinta, en el worktree")
}

// Hallazgo a9b269 — linea imprime el texto tal cual (un % es un %), y el
// formato va por lineaf.
func TestHallazgo_a9b269_CA544_LineaNoEsUnFormato(t *testing.T) {
	var b bytes.Buffer
	s := salida{&b}
	s.linea("100% del %s")
	s.lineaf("%d%% de %s", 5, "x")
	if got := b.String(); got != "100% del %s\n5% de x\n" {
		t.Fatalf("linea es texto y lineaf es formato: %q", got)
	}
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "reviewer", "co%ddex", r1T0, ccMin(5)))
	if out := ccTexto(ccBuild(t, root, "alfa"), true); !strings.Contains(out, "co%ddex") || strings.Contains(out, "%!") {
		t.Fatalf("un %% de la evidencia sale tal cual:\n%s", out)
	}
}
