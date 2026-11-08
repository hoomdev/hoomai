// Tests del writer para los hallazgos de la delta 5 de costo-por-spec
// (registro 20261108T044128_e8a7ac).
package costcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

// Hallazgos dbcb9c y 4132e6 — CA-546: una copia que no cuenta nunca se
// serializa. Una tarea que no es un slug hecha de '<' crece seis veces en
// JSON (<): si alguna copia pasara por json.Marshal, lo que Build
// pide de memoria lo diria.
func TestHallazgo_dbcb9c_4132e6_CA546_UnaCopiaRechazadaNoSeSerializa(t *testing.T) {
	const tam = 4 << 20
	root := ccRoot(t)
	ccWorktree(t, root, "alfa")
	wta := filepath.Join(root, ".hoom", "worktrees", "alfa")
	run := ccMeta("alfa", "reviewer", "codex", r1T0, ccMin(5))
	ccRun(t, root, run)
	rec := ccRegistro(t, wta, "alfa", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
	// the project's copy has a task of 4 MiB of '<': rejected, and it differs
	// from the worktree's good copy
	d3Copiar(t, wta, root, rec.ID, func(s string) string {
		return strings.Replace(s, `"task": "alfa"`, `"task": "`+strings.Repeat("<", tam)+`"`, 1)
	})
	pedido := func(f func()) int64 {
		var antes, despues runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&antes)
		f()
		runtime.ReadMemStats(&despues)
		return int64(despues.TotalAlloc - antes.TotalAlloc)
	}
	build := func() {
		r := ccBuild(t, root, "alfa")
		if ccTarea(t, "CA-546", r, "alfa").Rondas != 1 {
			t.Fatal("cuenta la copia buena del worktree")
		}
		r1Avisa(t, r, "tiene una tarea que no es un slug")
	}
	leer := func() {
		if rs, _, err := reviewcmd.RecordsErr(root); err != nil || len(rs) != 1 {
			t.Fatalf("fixture: el proyecto tiene la copia grande: %v %d", err, len(rs))
		}
	}
	build()
	// what Build asks for beyond reading the project's records: one
	// serialization of the big copy alone would be 6 x tam plus its growth
	extra := pedido(build) - pedido(leer)
	t.Logf("Build pidio %d KiB mas que leer los registros del proyecto (registro de %d KiB)", extra>>10, tam>>10)
	if extra > 2*tam {
		t.Fatalf("Build pidio %d MiB mas que leer un registro de %d MiB: alguna copia se serializo", extra>>20, tam>>20)
	}
}

// Hallazgo 7b4027 — CA-546: dos copias distintas con tareas que no son un
// slug y comparten el principio se dicen las dos: el aviso nombra donde esta
// cada una.
func TestHallazgo_7b4027_CA546_DosCopiasRecortadasIgualSeDicenLasDos(t *testing.T) {
	root := ccRoot(t)
	ccWorktree(t, root, "alfa")
	wta := filepath.Join(root, ".hoom", "worktrees", "alfa")
	rec := ccRegistro(t, root, "alfa", "completa", r1T0, []string{"risk"}, nil, nil)
	prefijo := "X" + strings.Repeat("a", maxEnAviso)
	d3Copiar(t, root, wta, rec.ID, func(s string) string { return strings.Replace(s, `"task": "alfa"`, `"task": "`+prefijo+`uno"`, 1) })
	d3Copiar(t, root, root, rec.ID, func(s string) string { return strings.Replace(s, `"task": "alfa"`, `"task": "`+prefijo+`dos"`, 1) })
	r, err := Build(root, "")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range r.Avisos {
		if strings.Contains(a, rec.ID) && strings.Contains(a, "no es un slug") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("las dos copias se dicen: %q", r.Avisos)
	}
	r1Avisa(t, r, `en el worktree "alfa" tiene una tarea`)
	r1Avisa(t, r, "en el proyecto tiene una tarea")
}

// CA-546 (delta 5, y delta 6: hallazgos 627919 y b96090): la copia de otra
// tarea que se lee antes que la de su tarea no dice nada si alguna fuente da
// el registro, igual o distinta (fuera del modelo, Henry 2026-11-08), y no
// se relee nada. Sin fuente que la de, se dice.
func TestHallazgo_627919_CA546_LaCopiaAjenaLeidaAntesCallaSiSuTareaLaDa(t *testing.T) {
	for _, distinta := range []bool{false, true} {
		root := ccRoot(t)
		// "aaa" is read before "beta", and the project has no copy
		ccWorktree(t, root, "aaa")
		ccWorktree(t, root, "beta")
		wtaaa := filepath.Join(root, ".hoom", "worktrees", "aaa")
		wtb := filepath.Join(root, ".hoom", "worktrees", "beta")
		run := ccMeta("beta", "reviewer", "codex", r1T0, ccMin(5))
		ccRun(t, root, run)
		rec := ccRegistro(t, wtb, "beta", "completa", r1T0.Add(ccMin(6)), []string{"risk"}, ccLensRuns([]string{"risk"}, run), nil)
		var cambio func(string) string
		if distinta {
			cambio = func(s string) string { return strings.Replace(s, `"risk"`, `"readability"`, 1) }
		}
		d3Copiar(t, wtb, wtaaa, rec.ID, cambio)
		r := ccBuild(t, root, "beta")
		if ta := ccTarea(t, "CA-546", r, "beta"); ta.Rondas != 1 || ta.RondasReview[0].Lentes[0] != "risk" {
			t.Fatalf("cuenta la copia de beta: %+v", ta.RondasReview)
		}
		if len(r.Avisos) != 0 {
			t.Fatalf("beta da el registro: la copia de aaa no dice nada (distinta %v): %q", distinta, r.Avisos)
		}
		// without beta's copy, the one aaa carries is said
		if err := os.Remove(filepath.Join(wtb, ".hoom", "reviews", rec.ID+".json")); err != nil {
			t.Fatal(err)
		}
		r = ccBuild(t, root, "beta")
		r1Avisa(t, r, `el worktree "aaa" tiene el registro `+rec.ID)
	}
}
