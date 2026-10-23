// Tests de regresion de la tarea endurecimiento-1 en boardcmd, sobre
// .hoom/specs/historia-doctor-y-cinta.md. Escritos desde el spec, el texto de
// cada hallazgo y el contrato que promete la correccion, SIN LEER LA
// IMPLEMENTACION: repos git reales en t.TempDir(), un git falso en el PATH
// que falla solo en ciertas invocaciones, y telemetria y evidencia escritas a
// mano. Usa los helpers compartidos bd* (gather_test.go), hi*
// (historia_test.go, historia_git_test.go) y drIDs (doctor_test.go); los de
// este archivo llevan el prefijo e1.
//
//   - 20261019T154842_56bc5e (resilience): la historia reducia a "no" tanto
//     la respuesta negativa esperada de git merge-base --is-ancestor y de git
//     rev-parse --verify de la rama de la tarea como un fallo de git. Con el
//     fallo, la tarjeta integrada se procesaba con otro rango, o se agregaba
//     la nota de "sin espacio de trabajo", y la historia salia con entradas
//     parciales. El spec exige todo o nada: si git falla, faltan TODAS las
//     entradas de git y notes dice "sin historial de git: <error>" (CA-359,
//     CA-360).
//   - 20261019T154847_d0d331 (resilience): el doctor solo fallaba cuando no
//     podia listar .hoom/items o .hoom/worktrees; con .hoom/envelopes,
//     .hoom/runs, .hoom/verdicts, .hoom/reviews o .hoom/findings ilegibles
//     degradaba a vacio y podia decir "sin problemas de coherencia" sin haber
//     mirado toda la evidencia. Ahora Doctor devuelve error; el tablero
//     (Build) se arma igual y lleva el problema en Warnings (CA-369).
package boardcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// ---------------------------------------------------------------- 56bc5e

// e1GitRoto pone al frente del PATH un git que falla de verdad (exit 128 con
// un "fatal:" en stderr, nunca la respuesta "no" de exit 1) en toda
// invocacion que tenga un argumento igual a exacto o que contenga contiene
// (el que no este vacio), y que es el git de verdad en todas las demas.
// Devuelve el archivo que marca que fallo al menos una vez.
func e1GitRoto(t *testing.T, exacto, contiene string) (marca string) {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("sin git en el PATH")
	}
	real, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marca = filepath.Join(t.TempDir(), "disparo")
	cond := ""
	if exacto != "" {
		cond = "[ \"$a\" = '" + exacto + "' ]"
	} else {
		cond = "case \"$a\" in *'" + contiene + "'*) true ;; *) false ;; esac"
	}
	s := "#!/bin/sh\nfor a in \"$@\"; do\n  if " + cond + "; then\n" +
		"    : > '" + marca + "'\n" +
		"    echo 'fatal: git roto por la prueba (endurecimiento-1)' >&2\n    exit 128\n  fi\ndone\n" +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marca
}

// e1SinHistoriaGit exige la regla de git que falla: ninguna entrada de git,
// la nota "sin historial de git: <error>" con un error no vacio, y ni la nota
// de fast-forward ni la de "sin espacio de trabajo", que afirmarian algo que
// git no llego a decir.
func e1SinHistoriaGit(t *testing.T, ca string, tl Timeline) {
	t.Helper()
	for _, e := range tl.Entries {
		if e.Source == SourceGit {
			t.Errorf("%s: si git falla, las entradas de git faltan todas (todo o nada); hay %s %s (notes %q):\n%s",
				ca, e.Kind, hiCorto(e.Ref), tl.Notes, hiListado(tl))
			break
		}
	}
	hay := false
	for _, n := range tl.Notes {
		if strings.HasPrefix(n, NoteSinGitPrefijo) && strings.TrimSpace(strings.TrimPrefix(n, NoteSinGitPrefijo)) != "" {
			hay = true
		}
	}
	if !hay {
		t.Errorf("%s: si git falla, notes dice %q seguido del error de git: %q", ca, NoteSinGitPrefijo+"<error>", tl.Notes)
	}
	for _, nota := range []string{NoteFastForward, NoteSinEspacio} {
		if hiTieneNota(tl, nota) {
			t.Errorf("%s: un fallo de git no es una respuesta de git: notes no dice %q: %q", ca, nota, tl.Notes)
		}
	}
}

// e1TimelineConGitRoto lee la historia de precios con el git roto ya en el
// PATH. Si la falla no se disparo, el caso no se puede inyectar asi (Skip).
// Si CardFor tambien falla con ese git, la historia falla como CardFor
// (CA-359) y este caso no aplica (Skip).
func e1TimelineConGitRoto(t *testing.T, ca, root, marca string) Timeline {
	t.Helper()
	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if _, e := os.Stat(marca); e != nil {
		t.Skipf("%s: la historia ya no corre esa invocacion de git: la falla no se puede inyectar asi", ca)
	}
	if err != nil {
		if _, errC := CardFor(root, "main", "high", bdSlug, hiAhora); errC != nil {
			t.Skipf("%s: con ese git falla tambien CardFor (%v): la historia falla como CardFor y el caso no aplica", ca, errC)
		}
		t.Fatalf("%s: TimelineFor solo falla como CardFor; con git fallando la historia se lee igual, sin historial de git: %v", ca, err)
	}
	hiBienFormada(t, ca, tl)
	return tl
}

// e1IntegradaNoFF: la tarjeta precios integrada con merge --no-ff y sin
// rama (como TestCA360_IntegradaSinFastForwardYSinRama). Devuelve sus dos
// commits.
func e1IntegradaNoFF(t *testing.T) (root, w1, w2 string) {
	t.Helper()
	root = bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, "precios.go", "package app\n")
	w1 = hiCommit(t, wt, "codigo de precios", h(1), hiAgente)
	bdEscribir(t, wt, "precios_test.go", "package app\n")
	w2 = hiCommit(t, wt, "prueba de precios", h(2), "")
	bdEscribir(t, root, "main.go", "package app // en main\n")
	hiCommit(t, root, "otro trabajo en main", h(3), "")
	bdGit(t, root, "worktree", "remove", "--force", wt)
	if ok, err := item.MarkDone(root, bdSlug, w2, h(4)); err != nil || !ok {
		t.Fatalf("fixture: MarkDone: %v %v", ok, err)
	}
	hiCommit(t, root, "cierre de precios", h(4), "")
	hiGitEnv(t, root, hiFechas(h(5)), "merge", "--no-ff", "-q", "-m", "integrar precios", "hoom/"+bdSlug)
	bdGit(t, root, "branch", "-D", "hoom/"+bdSlug)
	hiNoExisteRama(t, root)
	return root, w1, w2
}

// CA-360, CA-359 (hallazgo 56bc5e): para la tarjeta integrada, la historia
// pregunta a git si commit_final esta en la rama base (merge-base
// --is-ancestor). Si esa pregunta FALLA (exit 128, no el "no" de exit 1) y
// los git que siguen funcionan, la historia no sigue como si la respuesta
// fuera "no" (con el rango base..commit_final, que deja la tarjeta sin sus
// commits y con el resto de las entradas de git): es un fallo de git, asi
// que faltan todas las entradas de git y notes dice "sin historial de git:
// <error>". Sin la falla, la misma tarjeta separa sus dos commits.
func TestHallazgo_56bc5e_CA360_MergeBaseIsAncestorQueFallaNoEsUnNo(t *testing.T) {
	const ca = "CA-360 (hallazgo 56bc5e, merge-base --is-ancestor falla)"
	root, w1, w2 := e1IntegradaNoFF(t)
	ok := hiTimeline(t, "CA-360", root)
	hiSoloCommits(t, "CA-360 (fixture, sin la falla)", ok, []string{w1, w2}, nil)
	for _, n := range ok.Notes {
		if strings.HasPrefix(n, NoteSinGitPrefijo) || n == NoteFastForward || n == NoteSinEspacio {
			t.Fatalf("fixture: sin la falla la tarjeta integrada con --no-ff no tiene notas de git: %q", ok.Notes)
		}
	}

	marca := e1GitRoto(t, "--is-ancestor", "")
	tl := e1TimelineConGitRoto(t, ca, root, marca)
	e1SinHistoriaGit(t, ca, tl)
}

// CA-360, CA-359 (hallazgo 56bc5e): sin espacio de trabajo pero con la rama
// hoom/precios, la historia pregunta a git si la rama existe (rev-parse
// --verify). Si git FALLA en las invocaciones que nombran la rama de la
// tarea (exit 128, no el "no existe" de exit 1), la historia no concluye que
// la tarjeta no tiene espacio de trabajo propio (la nota de "sin espacio")
// ni sale con el resto de sus entradas de git: es un fallo de git, todo o
// nada. Sin la falla, la tarjeta trae los commits de la rama.
func TestHallazgo_56bc5e_CA360_RevParseVerifyDeLaRamaQueFallaNoEsSinEspacio(t *testing.T) {
	const ca = "CA-360 (hallazgo 56bc5e, git falla con la rama de la tarea)"
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	bdGit(t, root, "checkout", "-q", "-b", "hoom/"+bdSlug)
	bdEscribir(t, root, "precios.go", "package app\n")
	b1 := hiCommit(t, root, "codigo de precios", h(1), hiAgente)
	bdEscribir(t, root, "precios_test.go", "package app\n")
	b2 := hiCommit(t, root, "prueba de precios", h(2), "")
	bdGit(t, root, "checkout", "-q", "main")
	bdEscribir(t, root, "main.go", "package app // en main\n")
	hiCommit(t, root, "otro trabajo en main", h(3), "")

	ok := hiTimeline(t, "CA-360", root)
	hiSoloCommits(t, "CA-360 (fixture, sin la falla)", ok, []string{b1, b2}, nil)
	if hiTieneNota(ok, NoteSinEspacio) {
		t.Fatalf("fixture: sin la falla, con la rama hoom/precios la tarjeta tiene sus commits: %q", ok.Notes)
	}

	marca := e1GitRoto(t, "", "hoom/"+bdSlug)
	tl := e1TimelineConGitRoto(t, ca, root, marca)
	e1SinHistoriaGit(t, ca, tl)
}

// ---------------------------------------------------------------- d0d331

// e1Ahora es el now del doctor en estos tests.
var e1Ahora = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// e1Fuente es una de las fuentes que el doctor certifica, con lo que el
// fixture deja adentro.
type e1Fuente struct {
	dir      string
	problema string // el problema que el doctor ve con la fuente legible ("" = ninguno)
	armar    func(t *testing.T, root string)
}

func e1Fuentes() []e1Fuente {
	return []e1Fuente{
		{".hoom/envelopes", ProbSobreHuerfano, func(t *testing.T, root string) {
			// un sobre abierto sin tarea y sin dueno vivo: sobre-huerfano del proyecto
			bdSobreDisco(t, root, envelope.Record{ID: "20261022T090000_e1d001", Role: "writer", Provider: "claude",
				Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
				StartedAt: e1Ahora.Add(-2 * time.Hour), UpdatedAt: e1Ahora.Add(-2 * time.Hour), PID: bdPIDMuerto(t)})
		}},
		{".hoom/runs", "", func(t *testing.T, root string) {
			bdRunDisco(t, root, runcmd.Meta{ID: "20261022T090000_e1d002", Provider: "claude", Task: "huerfana",
				Dir: root, CreatedAt: e1Ahora.Add(-time.Hour), Status: runcmd.StatusDone, EndedAt: e1Ahora.Add(-50 * time.Minute)})
		}},
		{".hoom/verdicts", ProbEvidenciaSinItem, func(t *testing.T, root string) {
			// un veredicto de una tarea sin item: evidencia-sin-item
			bdVeredictoDisco(t, root, ".hoom/specs/huerfana.md", e1Ahora.Add(-time.Hour), false, hiGates("test", "pass"))
		}},
		{".hoom/reviews", "", func(t *testing.T, root string) {
			bdReviewDisco(t, root, reviewcmd.Record{ID: "20261022T090000_e1d003", CreatedAt: e1Ahora.Add(-time.Hour),
				Task: "huerfana", Spec: ".hoom/specs/huerfana.md", Lenses: []string{"risk"}, Provider: "claude"})
		}},
		{".hoom/findings", ProbEvidenciaSinItem, func(t *testing.T, root string) {
			// un hallazgo de una tarea sin item: evidencia-sin-item
			if _, err := finding.Register(root, "main", finding.Draft{Severity: "medium", Lens: "risk",
				Description: "hallazgo de una tarea sin item", Author: "reviewer", Task: "huerfana"}); err != nil {
				t.Fatal(err)
			}
		}},
	}
}

// e1SinLectura deja dir en 000 hasta el final del test. Saltea si corre como
// root o si el sistema deja listarlo igual.
func e1SinLectura(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("corre como root: un chmod no le quita la lectura, el test no probaria nada")
	}
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("este sistema deja listar un directorio sin permiso de lectura")
	}
}

// e1Nuevos son los avisos de despues que no estaban antes.
func e1Nuevos(antes, despues []string) []string {
	vistos := map[string]int{}
	for _, w := range antes {
		vistos[w]++
	}
	var out []string
	for _, w := range despues {
		if vistos[w] > 0 {
			vistos[w]--
			continue
		}
		out = append(out, w)
	}
	return out
}

// CA-369 (hallazgo d0d331): el doctor certifica la coherencia de toda la
// evidencia, asi que si .hoom/envelopes, .hoom/runs, .hoom/verdicts,
// .hoom/reviews o .hoom/findings existe pero no se puede listar (000),
// Doctor devuelve un error y no un reporte (que podia callar un sobre
// huerfano o una evidencia sin item, o decir cero problemas). El tablero
// (Build) no falla por eso: se arma igual y lleva el problema en Warnings
// (un aviso nuevo que nombra el directorio). Legible, el mismo fixture da un
// reporte sin error.
func TestHallazgo_d0d331_CA369_FuenteDeEvidenciaIlegibleEsUnErrorDelDoctor(t *testing.T) {
	for _, f := range e1Fuentes() {
		t.Run(f.dir, func(t *testing.T) {
			ca := "CA-369 (hallazgo d0d331, " + f.dir + " ilegible)"
			root := bdRepo(t, "findings:\n  block_on: high\n")
			f.armar(t, root)
			if fi, err := os.Stat(filepath.Join(root, f.dir)); err != nil || !fi.IsDir() {
				t.Fatalf("fixture: %s existe: %v %v", f.dir, fi, err)
			}
			r, err := Doctor(root, "main", "high", e1Ahora)
			if err != nil {
				t.Fatalf("%s: guarda: legible, Doctor da un reporte sin error: %v", ca, err)
			}
			if f.problema != "" && !strings.Contains(strings.Join(drIDs(r.Problems), " "), f.problema) {
				t.Fatalf("%s: guarda: legible, el doctor ve el %s que deja el fixture: %v", ca, f.problema, drIDs(r.Problems))
			}
			b, err := Build(root, "main", "high", e1Ahora)
			if err != nil {
				t.Fatalf("%s: guarda: legible, Build arma el tablero: %v", ca, err)
			}
			antes := b.Warnings

			e1SinLectura(t, filepath.Join(root, f.dir))
			if r, err := Doctor(root, "main", "high", e1Ahora); err == nil {
				t.Errorf("%s: %s existe y no se puede listar: Doctor devuelve error, no un reporte de %d problemas %v que calla lo que no pudo ver",
					ca, f.dir, len(r.Problems), drIDs(r.Problems))
			}
			b, err = Build(root, "main", "high", e1Ahora)
			if err != nil {
				t.Fatalf("%s: el tablero no falla por una fuente ilegible: Build se arma igual: %v", ca, err)
			}
			nuevos := e1Nuevos(antes, b.Warnings)
			nombra := false
			for _, w := range nuevos {
				if strings.Contains(w, filepath.Base(f.dir)) {
					nombra = true
				}
			}
			if !nombra {
				t.Errorf("%s: el tablero lleva el problema en Warnings: un aviso nuevo que nombra %s; avisos nuevos: %q (antes %q)",
					ca, filepath.Base(f.dir), nuevos, antes)
			}
		})
	}
}

// CA-369 (hallazgo d0d331, guarda): que esas fuentes no existan sigue siendo
// "nada": Doctor da su reporte sin error y sin problemas, y Build no gana
// avisos por ellas.
func TestHallazgo_d0d331_CA369_Guarda_FuentesQueNoExistenSonNada(t *testing.T) {
	const ca = "CA-369 (hallazgo d0d331, guarda)"
	root := bdRepo(t, "")
	for _, f := range e1Fuentes() {
		if _, err := os.Stat(filepath.Join(root, f.dir)); err == nil {
			t.Fatalf("fixture: el repo no tiene %s", f.dir)
		}
	}
	r, err := Doctor(root, "main", "high", e1Ahora)
	if err != nil || len(r.Problems) != 0 {
		t.Fatalf("%s: sin esas fuentes, Doctor da un reporte sin error y sin problemas: err=%v problemas=%v", ca, err, drIDs(r.Problems))
	}
	b, err := Build(root, "main", "high", e1Ahora)
	if err != nil {
		t.Fatalf("%s: sin esas fuentes, Build arma el tablero: %v", ca, err)
	}
	for _, w := range b.Warnings {
		for _, f := range e1Fuentes() {
			if strings.Contains(w, filepath.Base(f.dir)) {
				t.Errorf("%s: una fuente que no existe no es un aviso del tablero: %q", ca, w)
			}
		}
	}
}
