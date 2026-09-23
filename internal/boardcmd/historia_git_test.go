// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md sobre
// los commits de la tarea (CA-360) y los efectos de cada entrada sobre el
// medidor del replay (CA-363). Repos git reales, fechas de autor fijas, sin
// ningun CLI de IA. Los helpers compartidos (hi*) estan en historia_test.go.
package boardcmd

import (
	"fmt"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// hiCommitsDe son los refs de las entradas kind commit, en orden.
func hiCommitsDe(tl Timeline) []string {
	out := []string{}
	for _, e := range hiDeKind(tl, KindCommit) {
		out = append(out, e.Ref)
	}
	return out
}

func hiCortos(refs []string) []string {
	out := []string{}
	for _, r := range refs {
		out = append(out, hiCorto(r))
	}
	return out
}

// hiSoloCommits exige exactamente esos commits de la tarea, en orden, y que
// ninguna entrada (de ningun kind) nombre a los que no son de la tarea.
func hiSoloCommits(t *testing.T, ca string, tl Timeline, want []string, ajenos map[string]string) {
	t.Helper()
	if got := hiCommitsDe(tl); !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: los commits de la tarea deben ser %v, fueron %v:\n%s", ca, hiCortos(want), hiCortos(got), hiListado(tl))
	}
	for _, e := range tl.Entries {
		if que, ok := ajenos[e.Ref]; ok && e.Kind == KindCommit {
			t.Fatalf("%s: %s (%s) no es un commit de la tarea:\n%s", ca, que, hiCorto(e.Ref), hiListado(tl))
		}
	}
}

// hiCommitEs exige los campos de una entrada de commit de la tarea.
func hiCommitEs(t *testing.T, ca string, tl Timeline, ref, who, asunto string, archivos int, at time.Time) {
	t.Helper()
	e := hiUna(t, ca, tl, KindCommit, ref, "")
	plain := "se guardaron cambios en 1 archivo"
	if archivos != 1 {
		plain = fmt.Sprintf("se guardaron cambios en %d archivos", archivos)
	}
	if e.Source != SourceGit || e.Who != who || e.Plain != plain || !e.At.Equal(at) || e.Artifact != "" {
		t.Fatalf("%s: el commit %s es git, who %q (su autor), plain %q, at %s y artifact \"\"; fue %s %q %q %s %q",
			ca, hiCorto(ref), who, plain, at.UTC().Format(time.RFC3339), e.Source, e.Who, e.Plain, e.At.UTC().Format(time.RFC3339), e.Artifact)
	}
	if !strings.Contains(e.Summary, asunto) {
		t.Fatalf("%s: el summary del commit %s trae su asunto %q: %q", ca, hiCorto(ref), asunto, e.Summary)
	}
}

// hiNoExisteRama verifica el fixture: la rama de la tarea ya no existe.
func hiNoExisteRama(t *testing.T, root string) {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--verify", "-q", "hoom/"+bdSlug)
	cmd.Dir = root
	if err := cmd.Run(); err == nil {
		t.Fatal("el fixture: la rama hoom/precios ya no existe")
	}
}

// CA-360: con espacio de trabajo los commits de la tarea son <base>..HEAD en
// el espacio de trabajo, uno por commit, sin merges (traer main al espacio de
// trabajo no es una entrada, ni lo que trae), con su autor, su sha, su asunto
// y la cantidad de archivos.
func TestCA360_CommitsConEspacioDeTrabajo(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	alta := hiCommit(t, root, "alta del item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)

	bdEscribir(t, wt, "precios.go", "package app\n\nfunc Precio() int { return 1 }\n")
	bdEscribir(t, wt, "precios_test.go", "package app\n")
	w1 := hiCommit(t, wt, "codigo y prueba de precios", h(1), hiAgente)
	bdEscribir(t, root, "main.go", "package app // en main\n")
	x := hiCommit(t, root, "otro trabajo en main", h(2), "")
	bdEscribir(t, wt, "precios.go", "package app\n\nfunc Precio() int { return 2 }\n")
	w2 := hiCommit(t, wt, "redondeo de precios", h(3), "")
	hiGitEnv(t, wt, hiFechas(h(4)), "merge", "--no-ff", "-q", "-m", "traer main al espacio de trabajo", "main")
	m := bdGit(t, wt, "rev-parse", "HEAD")
	bdEscribir(t, wt, "web/precios.js", "// vista\n")
	w3 := hiCommit(t, wt, "vista de precios", h(5), hiHenry)

	tl := hiTimeline(t, "CA-360", root)
	hiSoloCommits(t, "CA-360", tl, []string{w1, w2, w3},
		map[string]string{alta: "el alta del item en main", x: "un commit de main", m: "el merge de main"})
	hiCommitEs(t, "CA-360", tl, w1, hiAgente, "codigo y prueba de precios", 2, h(1))
	hiCommitEs(t, "CA-360", tl, w2, hiYo, "redondeo de precios", 1, h(3))
	hiCommitEs(t, "CA-360", tl, w3, hiHenry, "vista de precios", 1, h(5))
	for _, e := range tl.Entries {
		if e.Ref == m {
			t.Fatalf("CA-360: un merge no es una entrada: %+v", e)
		}
	}
	if hiTieneNota(tl, NoteSinEspacio) || hiTieneNota(tl, NoteFastForward) {
		t.Fatalf("CA-360: con espacio de trabajo no va ninguna nota de los commits: %q", tl.Notes)
	}
}

// CA-360: sin espacio de trabajo pero con la rama hoom/<slug>, los commits de
// la tarea son <base>..hoom/<slug> leidos en la raiz.
func TestCA360_CommitsDeLaRamaSinEspacio(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	bdGit(t, root, "checkout", "-q", "-b", "hoom/"+bdSlug)
	bdEscribir(t, root, "precios.go", "package app\n")
	b1 := hiCommit(t, root, "codigo de precios", h(1), hiAgente)
	bdEscribir(t, root, "precios_test.go", "package app\n")
	bdEscribir(t, root, "web/precios.js", "// vista\n")
	b2 := hiCommit(t, root, "prueba y vista", h(2), "")
	bdGit(t, root, "checkout", "-q", "main")
	bdEscribir(t, root, "main.go", "package app // en main\n")
	x := hiCommit(t, root, "otro trabajo en main", h(3), "")

	tl := hiTimeline(t, "CA-360", root)
	hiSoloCommits(t, "CA-360", tl, []string{b1, b2}, map[string]string{x: "un commit de main"})
	hiCommitEs(t, "CA-360", tl, b1, hiAgente, "codigo de precios", 1, h(1))
	hiCommitEs(t, "CA-360", tl, b2, hiYo, "prueba y vista", 2, h(2))
	if hiTieneNota(tl, NoteSinEspacio) {
		t.Fatalf("CA-360: con la rama hoom/precios la tarjeta tiene sus commits: %q", tl.Notes)
	}
}

// CA-360: una tarjeta integrada con merge --no-ff tiene como commits de la
// tarea M^1..commit_final, aunque la rama ya no exista: ni los commits de
// main, ni el cierre del item, ni el merge, ni lo que vino despues.
func TestCA360_IntegradaSinFastForwardYSinRama(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	alta := hiCommit(t, root, "alta del item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, "precios.go", "package app\n")
	w1 := hiCommit(t, wt, "codigo de precios", h(1), hiAgente)
	bdEscribir(t, wt, "precios_test.go", "package app\n")
	w2 := hiCommit(t, wt, "prueba de precios", h(2), "")
	bdEscribir(t, root, "main.go", "package app // en main\n")
	x := hiCommit(t, root, "otro trabajo en main", h(3), "")

	// el cierre como lo deja 'hoom task done': sin espacio de trabajo, el
	// item con hecho_en y commit_final = la punta de la rama; despues el merge
	bdGit(t, root, "worktree", "remove", "--force", wt)
	if ok, err := item.MarkDone(root, bdSlug, w2, h(4)); err != nil || !ok {
		t.Fatalf("el fixture: MarkDone: %v %v", ok, err)
	}
	c := hiCommit(t, root, "cierre de precios", h(4), "")
	hiGitEnv(t, root, hiFechas(h(5)), "merge", "--no-ff", "-q", "-m", "integrar precios", "hoom/"+bdSlug)
	merge := bdGit(t, root, "rev-parse", "HEAD")
	bdGit(t, root, "branch", "-D", "hoom/"+bdSlug)
	hiNoExisteRama(t, root)
	bdEscribir(t, root, "despues.go", "package app // despues\n")
	y := hiCommit(t, root, "trabajo despues de integrar", h(6), "")

	tl := hiTimeline(t, "CA-360", root)
	hiSoloCommits(t, "CA-360", tl, []string{w1, w2}, map[string]string{alta: "el alta del item",
		x: "un commit de main", c: "el cierre del item", merge: "el merge que integro", y: "un commit posterior"})
	hiCommitEs(t, "CA-360", tl, w1, hiAgente, "codigo de precios", 1, h(1))
	hiCommitEs(t, "CA-360", tl, w2, hiYo, "prueba de precios", 1, h(2))
	if hiTieneNota(tl, NoteFastForward) || hiTieneNota(tl, NoteSinEspacio) {
		t.Fatalf("CA-360: integrada con merge los commits se separan: %q", tl.Notes)
	}
}

// CA-360: integrada con fast-forward no hay forma honesta de separar los
// commits: ninguna entrada commit y la nota del contrato (y no la de "sin
// espacio de trabajo": la regla de commit_final va primero).
func TestCA360_IntegradaConFastForward(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, "precios.go", "package app\n")
	hiCommit(t, wt, "codigo de precios", h(1), "")
	bdEscribir(t, wt, "precios_test.go", "package app\n")
	w2 := hiCommit(t, wt, "prueba de precios", h(2), "")
	bdGit(t, root, "worktree", "remove", "--force", wt)
	bdGit(t, root, "merge", "--ff-only", "-q", "hoom/"+bdSlug)
	bdGit(t, root, "branch", "-D", "hoom/"+bdSlug)
	if ok, err := item.MarkDone(root, bdSlug, w2, h(3)); err != nil || !ok {
		t.Fatalf("el fixture: MarkDone: %v %v", ok, err)
	}
	hiCommit(t, root, "cierre de precios", h(3), "")

	tl := hiTimeline(t, "CA-360", root)
	if got := hiCommitsDe(tl); len(got) != 0 {
		t.Fatalf("CA-360: integrada con fast-forward no hay commits de la tarea: %v\n%s", hiCortos(got), hiListado(tl))
	}
	if !hiTieneNota(tl, NoteFastForward) {
		t.Fatalf("CA-360: notes dice %q: %q", NoteFastForward, tl.Notes)
	}
	if hiTieneNota(tl, NoteSinEspacio) {
		t.Fatalf("CA-360: con commit_final rige su regla, no la de sin espacio: %q", tl.Notes)
	}
}

// CA-360: con commit_final sin integrar en la base, los commits son
// <base>..<commit_final>, y esa regla va antes que la del espacio de trabajo:
// un commit posterior a commit_final en el espacio de trabajo no entra.
func TestCA360_CommitFinalSinIntegrar(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, "precios.go", "package app\n")
	w1 := hiCommit(t, wt, "codigo de precios", h(1), "")
	bdEscribir(t, wt, "precios_test.go", "package app\n")
	w2 := hiCommit(t, wt, "prueba de precios", h(2), "")
	bdEscribir(t, wt, "extra.go", "package app\n")
	w3 := hiCommit(t, wt, "despues del cierre", h(3), "")
	if ok, err := item.MarkDone(root, bdSlug, w2, h(4)); err != nil || !ok {
		t.Fatalf("el fixture: MarkDone: %v %v", ok, err)
	}
	hiCommit(t, root, "cierre de precios", h(4), "")

	tl := hiTimeline(t, "CA-360", root)
	hiSoloCommits(t, "CA-360", tl, []string{w1, w2}, map[string]string{w3: "un commit posterior a commit_final"})
}

// CA-360: sin commit_final, sin espacio de trabajo y sin la rama, la tarjeta
// no tiene commits de la tarea (los de main no lo son) y notes lo dice.
func TestCA360_SinEspacioPropio(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	bdEscribir(t, root, "precios.go", "package app\n")
	hiCommit(t, root, "codigo en main", h(1), "")

	tl := hiTimeline(t, "CA-360", root)
	if got := hiCommitsDe(tl); len(got) != 0 {
		t.Fatalf("CA-360: sin espacio propio no hay commits de la tarea: %v\n%s", hiCortos(got), hiListado(tl))
	}
	if !hiTieneNota(tl, NoteSinEspacio) || hiTieneNota(tl, NoteFastForward) {
		t.Fatalf("CA-360: notes dice %q: %q", NoteSinEspacio, tl.Notes)
	}
}

// CA-363: cada entrada trae lo que hace al medidor del replay. spec: una
// entrada spec se enciende solo con una aprobacion anterior (o del mismo
// commit) de ese contenido, se apaga con una edicion sin aprobar y vuelve con
// la segunda aprobacion (y con volver a un contenido ya aprobado). Criterios:
// el primer commit de la tarea que agrega el token en una linea de un
// archivo de test (no el codigo, no el spec, no una linea de contexto, no
// CA-10 por CA-1, no un token que ya aparecio). Gates y review: los
// veredictos completos segun sus gates, spec_trace y el tamano; el registro
// de 4 lentes. Todo lo demas trae [].
func TestCA363_EfectosDelMedidorEnElReplay(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdEscribir(t, root, "legacy_test.go", "package app\n\n// CA-3 heredado de main\n// linea vieja\n")
	r0 := hiCommit(t, root, "item y prueba heredada", h(0), "")
	wt := bdWorktree(t, root, bdSlug)

	bdEscribir(t, wt, bdSpec, hiSpecV1)
	bdEscribir(t, wt, "otros_test.go", "package app\n\n// CA-10 y CA-30 no son criterios de este spec\n")
	w1 := hiCommit(t, wt, "spec y prueba ajena", h(1), "")
	a1 := hiAprobar(t, wt, bdSlug, hiFirmante, h(2))
	w2 := hiCommit(t, wt, "aprobacion de la primera version", h(2), "")
	bdEscribir(t, wt, "precios.go", "package app\n\n// CA-2 en el codigo no es una prueba\nfunc Precio() int { return 1 }\n")
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n")
	bdEscribir(t, wt, "legacy_test.go", "package app\n\n// CA-3 heredado de main\n// linea nueva\n")
	w3 := hiCommit(t, wt, "codigo y prueba del uno", h(3), "")
	bdEscribir(t, wt, bdSpec, hiSpecV2)
	w4 := hiCommit(t, wt, "edicion del spec", h(4), "")
	a2 := hiAprobar(t, wt, bdSlug, hiFirmante, h(5))
	w5 := hiCommit(t, wt, "aprobacion de la segunda version", h(5), "")
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n// CA-1 otra vez\n")
	bdEscribir(t, wt, "web/precios.spec.js", "// CA-2: la vista muestra el precio\n")
	w6 := hiCommit(t, wt, "pruebas del dos", h(6), "")
	rojo := hiVeredicto(t, wt, bdSpec, h(7), false, 30, hiGates(
		"spec_lint", verdict.StatusPass, "spec_trace", verdict.StatusFail, "spec_approved", verdict.StatusPass,
		"findings_open", verdict.StatusPass, "build", verdict.StatusFail, "static", verdict.StatusError, "test", verdict.StatusPass))
	w7 := hiCommit(t, wt, "veredicto rojo", h(7), "")
	parcial := hiVeredicto(t, wt, bdSpec, h(8), true, 30, hiGates("test", verdict.StatusPass))
	w8 := hiCommit(t, wt, "veredicto parcial", h(8), "")
	verde := hiVeredicto(t, wt, bdSpec, h(9), false, reviewcmd.UmbralLineas+120, hiGates(
		"spec_lint", verdict.StatusPass, "spec_trace", verdict.StatusPass, "spec_approved", verdict.StatusPass,
		"findings_open", verdict.StatusPass, "build", verdict.StatusPass, "test", verdict.StatusPass))
	w9 := hiCommit(t, wt, "veredicto verde grande", h(9), "")
	f := hiHallazgo(t, wt, "medium", bdSlug)
	w10 := hiCommit(t, wt, "hallazgo", h(10), "")
	hiResolver(t, wt, f, finding.StatusCorrected)
	w11 := hiCommit(t, wt, "resolucion", h(11), "")
	rv2 := hiReview(t, wt, "20260922T170000_aaaaaa", bdSlug, []string{"risk", "readability"})
	w12 := hiCommit(t, wt, "review de dos lentes", h(12), "")
	rv4 := hiReview(t, wt, "20260922T180000_bbbbbb", bdSlug, reviewcmd.Lentes)
	w13 := hiCommit(t, wt, "review de cuatro lentes", h(13), "")
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n// CA-1 otra vez\n// CA-3 con prueba propia\n")
	w14 := hiCommit(t, wt, "prueba propia del tres", h(14), "")
	bdEscribir(t, wt, bdSpec, hiSpecV1)
	w15 := hiCommit(t, wt, "vuelta a la primera version del spec", h(15), "")

	// telemetria: un sobre con su run y una delegacion, y un run suelto
	const sobre, run, suelto = "20260922T220000_sobre1", "20260922T220000_run001", "20260922T230000_run002"
	bdRunDisco(t, root, runcmd.Meta{ID: run, Provider: "claude", Role: "writer", Task: bdSlug, Dir: wt,
		CreatedAt: h(16), EndedAt: h(16).Add(30 * time.Minute), Status: runcmd.StatusDone,
		Usage: &providers.Usage{CostUSD: bdF(0.3), InputTokens: 5, OutputTokens: 5}})
	hiEventos(t, root, run,
		providers.Event{TS: h(16).Add(time.Minute), Kind: "agent", Agent: "Explore", ToolID: "toolu_1"},
		providers.Event{TS: h(16).Add(2 * time.Minute), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_1", Detail: "Explore termino"})
	bdSobreDisco(t, root, envelope.Record{ID: sobre, Role: "writer", Provider: "claude", Task: bdSlug, Dir: wt, RunID: run,
		Stage: "ok", Step: 7, Steps: 7, Status: envelope.StatusDeliverable,
		StartedAt: h(16), UpdatedAt: h(16).Add(31 * time.Minute), EndedAt: h(16).Add(31 * time.Minute)})
	bdRunDisco(t, root, runcmd.Meta{ID: suelto, Provider: "codex", Role: "reviewer", Task: bdSlug, Dir: wt,
		CreatedAt: h(17), EndedAt: h(17).Add(10 * time.Minute), Status: runcmd.StatusDone})

	tl := hiTimeline(t, "CA-363", root)
	var ids []string
	for _, s := range tl.Meter {
		ids = append(ids, s.ID)
	}
	if strings.Join(ids, ",") != "spec,CA-1,CA-2,CA-3,build,static,test,review" {
		t.Fatalf("CA-363: el esqueleto de este fixture es spec, CA-1..3, build, static, test y review: %v", ids)
	}

	vacio := map[string]string{}
	ver := func(v *verdict.Verdict) string { return hiWtp + ".hoom/verdicts/" + v.ID + ".json" }
	casos := []struct {
		que, kind, ref, artifact string
		ef                       map[string]string
	}{
		{"el item", KindItem, r0, ".hoom/items/precios.yaml", vacio},
		{"un spec sin aprobacion (la que llega despues no cuenta)", KindSpec, w1, hiWtp + bdSpec, map[string]string{"spec": SegFalta}},
		{"el commit del spec: el spec no es un archivo de test, y CA-10/CA-30 no son CA-1/CA-3", KindCommit, w1, "", vacio},
		{"la aprobacion del contenido del spec de ese punto", KindAprobacion, w2, hiWtp + a1, map[string]string{"spec": SegHecho}},
		{"el commit de la aprobacion", KindCommit, w2, "", vacio},
		{"el primer test del uno (CA-2 en el codigo y CA-3 de contexto no cuentan)", KindCommit, w3, "", map[string]string{"CA-1": SegHecho}},
		{"una edicion sin aprobacion", KindSpec, w4, hiWtp + bdSpec, map[string]string{"spec": SegFalta}},
		{"el commit de la edicion", KindCommit, w4, "", vacio},
		{"la segunda aprobacion", KindAprobacion, w5, hiWtp + a2, map[string]string{"spec": SegHecho}},
		{"el commit de la segunda aprobacion", KindCommit, w5, "", vacio},
		{"el primer test del dos, en un .spec.js (el uno ya habia aparecido)", KindCommit, w6, "", map[string]string{"CA-2": SegHecho}},
		{"un veredicto completo rojo, chico, sin spec_trace", KindVeredicto, w7, ver(rojo),
			map[string]string{"build": SegFalta, "static": SegFalta, "test": SegHecho, "review": SegNoAplica}},
		{"el commit del veredicto rojo", KindCommit, w7, "", vacio},
		{"un veredicto parcial", KindVeredicto, w8, ver(parcial), vacio},
		{"el commit del parcial", KindCommit, w8, "", vacio},
		{"un veredicto completo verde con spec_trace, grande, sin static", KindVeredicto, w9, ver(verde),
			map[string]string{"CA-1": SegHecho, "CA-2": SegHecho, "CA-3": SegHecho, "build": SegHecho, "static": SegNoAplica, "test": SegHecho}},
		{"el commit del verde", KindCommit, w9, "", vacio},
		{"un hallazgo", KindHallazgo, w10, hiWtp + ".hoom/findings/" + f + ".json", vacio},
		{"el commit del hallazgo", KindCommit, w10, "", vacio},
		{"una resolucion", KindResolucion, w11, hiWtp + ".hoom/findings/" + f + ".res.json", vacio},
		{"el commit de la resolucion", KindCommit, w11, "", vacio},
		{"un registro de review sin las 4 lentes", KindReview, w12, hiWtp + ".hoom/reviews/" + rv2 + ".json", vacio},
		{"el commit de la review de dos lentes", KindCommit, w12, "", vacio},
		{"un registro de review con las 4 lentes", KindReview, w13, hiWtp + ".hoom/reviews/" + rv4 + ".json", map[string]string{"review": SegHecho}},
		{"el commit de la review de 4 lentes", KindCommit, w13, "", vacio},
		{"el primer test propio del tres (el de main no es de la tarea)", KindCommit, w14, "", map[string]string{"CA-3": SegHecho}},
		{"el spec vuelve a un contenido que ya tenia aprobacion", KindSpec, w15, hiWtp + bdSpec, map[string]string{"spec": SegHecho}},
		{"el commit de la vuelta", KindCommit, w15, "", vacio},
	}
	for _, c := range casos {
		hiEfectos(t, "CA-363", c.que, hiUna(t, "CA-363", tl, c.kind, c.ref, c.artifact), c.ef)
	}
	git, tel := 0, 0
	for _, e := range tl.Entries {
		if e.Source == SourceGit {
			git++
			continue
		}
		tel++
		hiEfectos(t, "CA-363", "la telemetria no toca el medidor", e, vacio)
	}
	if git != len(casos) {
		t.Fatalf("CA-363: el fixture tiene %d entradas de git, hubo %d:\n%s", len(casos), git, hiListado(tl))
	}
	if tel != 4 {
		t.Fatalf("CA-363: el fixture tiene 4 entradas de telemetria (sobre, subagente, subagente-fin, run), hubo %d:\n%s", tel, hiListado(tl))
	}
	if hiTieneNota(tl, NoteTestsCortados) {
		t.Fatalf("CA-363: un parche chico no se corta: %q", tl.Notes)
	}
}

// CA-363: spec y aprobacion en el mismo commit: spec termina encendido (la
// entrada spec y la entrada aprobacion lo encienden), y el item no lo toca.
func TestCA363_SpecYAprobacionEnElMismoCommit(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdEscribir(t, root, bdSpec, hiSpecV1)
	a := hiAprobar(t, root, bdSlug, hiFirmante, d0)
	c := hiCommit(t, root, "item, spec y aprobacion juntos", d0, "")

	tl := hiTimeline(t, "CA-363", root)
	hiSecuencia(t, "CA-363", tl, []hiEsp{
		{SourceGit, KindItem, c, ".hoom/items/precios.yaml"},
		{SourceGit, KindSpec, c, bdSpec},
		{SourceGit, KindAprobacion, c, a},
	})
	hiEfectos(t, "CA-363", "el item", tl.Entries[0], map[string]string{})
	hiEfectos(t, "CA-363", "el spec con su aprobacion en el mismo commit", tl.Entries[1], map[string]string{"spec": SegHecho})
	hiEfectos(t, "CA-363", "la aprobacion del spec de ese commit", tl.Entries[2], map[string]string{"spec": SegHecho})
}

// CA-363: el parche de los commits de la tarea se lee hasta 8 MiB; si se
// corta, notes lo dice.
func TestCA363_ParcheDeMasDe8MiB(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, hiSpecV1)
	hiCommit(t, wt, "spec", h(1), "")
	linea := "relleno de prueba sin ningun criterio citado\n"
	grande := strings.Repeat(linea, (TimelinePatchMax+(1<<20))/len(linea)) + "// CA-1 al final\n"
	bdEscribir(t, wt, "datos_test.txt", grande)
	hiCommit(t, wt, "datos de prueba grandes", h(2), "")

	tl := hiTimeline(t, "CA-363", root)
	if !hiTieneNota(tl, NoteTestsCortados) {
		t.Fatalf("CA-363: con un parche de mas de 8 MiB notes dice %q: %q", NoteTestsCortados, tl.Notes)
	}
}
