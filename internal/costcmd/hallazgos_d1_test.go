// Tests del writer para los hallazgos de la delta 1 de costo-por-spec
// (registro 20261107T173245_7f0aa5). Cada uno fija el arreglo de un hallazgo
// con su id y el criterio que protege.
package costcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Hallazgo 6b9362 — spec, no-objetivos: un sidecar que vive dentro del
// worktree de la tarea se borra con task done, asi que no se cuenta; el run
// de la raiz si.
func TestHallazgo_6b9362_CA544_RunDelWorktreeNoSeCuenta(t *testing.T) {
	root := ccRoot(t)
	ccWorktree(t, root, "alfa")
	wt := filepath.Join(root, ".hoom", "worktrees", "alfa")
	if err := os.MkdirAll(filepath.Join(wt, ".hoom", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	enRaiz := ccMeta("alfa", "writer", "codex", r1T0, ccMin(5))
	enRaiz.Usage = ccCodex(1000, 400, 50, 0)
	ccRun(t, root, enRaiz)
	enWT := ccMeta("alfa", "writer", "codex", r1T0.Add(ccMin(10)), ccMin(5))
	enWT.Usage = ccCodex(9000, 0, 9, 0)
	ccRun(t, wt, enWT)
	ta := ccTarea(t, "CA-544", ccBuild(t, root, "alfa"), "alfa")
	g := ta.PorProvider["codex"]
	if g.Runs != 1 || g.Tokens.Nuevos == nil || *g.Tokens.Nuevos != 600 {
		t.Fatalf("solo cuenta el run de la raiz: %+v", g)
	}
	// a slug whose only run lives in a worktree has nothing to report
	solo := ccRoot(t)
	ccWorktree(t, solo, "beta")
	wtb := filepath.Join(solo, ".hoom", "worktrees", "beta")
	if err := os.MkdirAll(filepath.Join(wtb, ".hoom", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	ccRun(t, wtb, ccMeta("beta", "writer", "codex", r1T0, ccMin(1)))
	if _, err := Build(solo, "beta"); err == nil {
		t.Fatal("un run que solo vive en el worktree no hace que la tarea tenga runs")
	}
}

// Hallazgo ca7aac — un worktree habla solo por su tarea: el registro que un
// worktree trae de otra tarea, o con una tarea que no es un slug, no se
// cuenta y se avisa; un hallazgo del worktree de otra tarea no da severidad,
// y uno de la raiz no se reemplaza.
func TestHallazgo_ca7aac_CA544_UnWorktreeHablaSoloPorSuTarea(t *testing.T) {
	root := ccRoot(t)
	ccWorktree(t, root, "alfa")
	wt := filepath.Join(root, ".hoom", "worktrees", "alfa")
	run := ccMeta("beta", "reviewer", "codex", r1T0, ccMin(5))
	run.Usage = ccCodex(1000, 400, 50, 0)
	ccRun(t, root, run)
	ajeno := ccRegistro(t, wt, "beta", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
	r := ccBuild(t, root, "beta")
	ta := ccTarea(t, "CA-544", r, "beta")
	if ta.Rondas != 0 {
		t.Fatalf("el registro de beta que trae el worktree de alfa no es una ronda de beta: %+v", ta.RondasReview)
	}
	r1Avisa(t, r, ajeno.ID)

	// a task that is no slug never reaches the report
	root2 := ccRoot(t)
	malo := ccRegistro(t, root2, "ok", "completa", r1T0, []string{"risk"}, nil, nil)
	path := filepath.Join(root2, ".hoom", "reviews", malo.ID+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte(strings.Replace(string(raw), `"task": "ok"`, `"task": "ok\u001b[2J"`, 1))
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	r2, err := Build(root2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Tareas) != 0 {
		t.Fatalf("un registro con una tarea que no es un slug no crea una tarea: %+v", r2.Tareas)
	}
	r1Avisa(t, r2, malo.ID)

	// findings: the project's are never replaced, and a worktree adds only its
	// task's own
	root3 := ccRoot(t)
	ccWorktree(t, root3, "alfa")
	wt3 := filepath.Join(root3, ".hoom", "worktrees", "alfa")
	low := ccHallazgo(t, root3, "alfa", "low")
	ajenoHigh := ccHallazgo(t, wt3, "beta", "high")
	propio := ccHallazgo(t, wt3, "alfa", "medium")
	copiar := func(id, sev string) {
		// the worktree carries the project's finding with another severity
		src := filepath.Join(root3, ".hoom", "findings", id+".json")
		raw, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		raw = []byte(strings.Replace(string(raw), `"severity": "low"`, `"severity": "`+sev+`"`, 1))
		if err := os.WriteFile(filepath.Join(wt3, ".hoom", "findings", id+".json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	copiar(low, "high")
	rv := ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5))
	ccRun(t, root3, rv)
	ccRegistro(t, wt3, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, rv),
		[]string{low, ajenoHigh, propio})
	ta3 := ccTarea(t, "CA-544", ccBuild(t, root3, "alfa"), "alfa")
	if len(ta3.RondasReview) != 1 {
		t.Fatalf("una ronda: %+v", ta3.RondasReview)
	}
	h := ta3.RondasReview[0]
	if h.Hallazgos["low"] != 1 || h.Hallazgos["medium"] != 1 || h.Hallazgos["high"] != 0 || h.HallazgosSinDato != 1 {
		t.Fatalf("low de la raiz, medium propio del worktree, el de beta sin dato: %+v sin dato %d", h.Hallazgos, h.HallazgosSinDato)
	}
}

// Hallazgo 5294dd — un .hoom/worktrees que existe y no se puede listar se
// avisa: las rondas de las tareas en curso pueden faltar.
func TestHallazgo_5294dd_CA546_WorktreesIlegiblesSeAvisan(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("alfa", "writer", "codex", r1T0, ccMin(1)))
	if err := os.WriteFile(filepath.Join(root, ".hoom", "worktrees"), []byte("no soy un directorio"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := ccBuild(t, root, "alfa")
	r1Avisa(t, r, ".hoom/worktrees")
	// no directory at all is no worktrees, and nothing to say
	limpio := ccRoot(t)
	ccRun(t, limpio, ccMeta("alfa", "writer", "codex", r1T0, ccMin(1)))
	if r := ccBuild(t, limpio, "alfa"); len(r.Avisos) != 0 {
		t.Fatalf("sin .hoom/worktrees no hay aviso: %q", r.Avisos)
	}
}

// Hallazgo ca8e4a — hecho_en se guarda al segundo: una ronda del mismo
// segundo que la aceptacion es de antes; una del segundo siguiente, de
// despues.
func TestHallazgo_ca8e4a_CA544_HechoEnSeComparaAlSegundo(t *testing.T) {
	root := ccRoot(t)
	ccItem(t, root, "alfa")
	acepta := r1T0.Add(900 * time.Millisecond)
	ccAceptar(t, root, "alfa", "abc123", acepta)
	mismoSegundo := r1T0.Add(500 * time.Millisecond)
	run := ccMeta("alfa", "reviewer", "codex", mismoSegundo.Add(-ccMin(5)), ccMin(5))
	ccRun(t, root, run)
	ccRegistro(t, root, "alfa", "completa", mismoSegundo, []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
	tarde := ccMeta("alfa", "writer", "codex", r1T0.Add(1200*time.Millisecond), ccMin(1))
	ccRun(t, root, tarde)
	ta := ccTarea(t, "CA-544", ccBuild(t, root, "alfa"), "alfa")
	if ta.Rondas != 1 {
		t.Fatalf("la ronda del mismo segundo cuenta: %+v", ta.RondasReview)
	}
	if ta.DespuesDeAceptada == nil || ta.DespuesDeAceptada.Runs != 1 || ta.DespuesDeAceptada.Rondas != 0 {
		t.Fatalf("solo el run del segundo siguiente queda fuera: %+v", ta.DespuesDeAceptada)
	}
}

// Hallazgo ca7aac — lo que se imprime viene de la evidencia; ningun control
// ni caracter de direccion llega crudo a la terminal.
func TestHallazgo_ca7aac_CA544_LaSalidaEscapaLosControles(t *testing.T) {
	root := ccRoot(t)
	run := ccMeta("alfa", "reviewer", "co\u202edex", r1T0, ccMin(5))
	run.Ventanas = []runcmd.VentanaRun{ccV("5h\x1b[2J", 10, 12, time.Time{})}
	ccRun(t, root, run)
	ccRegistro(t, root, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk\x1b[31m"}, ccLensRuns([]string{"risk"}, run), nil)
	for _, detalle := range []bool{false, true} {
		r, err := Build(root, map[bool]string{false: "", true: "alfa"}[detalle])
		if err != nil {
			t.Fatal(err)
		}
		out := ccTexto(r, detalle)
		if strings.ContainsAny(out, "\x1b\u202e") {
			t.Fatalf("la salida no lleva controles crudos:\n%q", out)
		}
		if detalle && (!strings.Contains(out, `risk\x1b[31m`) || !strings.Contains(out, "co\\u202edex")) {
			t.Fatalf("los controles salen escapados, a la vista:\n%s", out)
		}
	}
}
