// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-269..CA-279, CA-281..CA-288) sobre boardcmd.Derive, la funcion PURA:
// la tarjeta queda en la columna del PRIMER requisito que no se cumple, y
// los subestados salen de la misma evidencia. Todo con Evidence literales:
// sin disco, sin reloj, sin git.
package boardcmd

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	bdSlug  = "precios"
	bdSpec  = ".hoom/specs/precios.md"
	bdWtDir = ".hoom/worktrees/precios"
)

var bdT0 = time.Date(2026, 9, 22, 15, 0, 0, 0, time.UTC)

func bdF(v float64) *float64 { return &v }

// bdGatesVerdes son los gates de un veredicto completo de la tarjeta.
func bdGatesVerdes() []verdict.GateResult {
	return []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "spec_trace", Required: true, Status: verdict.StatusPass},
		{Name: "spec_approved", Required: true, Status: verdict.StatusPass},
		{Name: "findings_open", Required: true, Status: verdict.StatusPass},
		{Name: "test", Required: true, Status: verdict.StatusPass},
	}
}

func bdVeredicto(id string, at time.Time, huella string, ins, del int, gates []verdict.GateResult) *verdict.Verdict {
	v := &verdict.Verdict{ID: id, CreatedAt: at, Spec: bdSpec,
		Git:   gitx.Info{IsRepo: true, ChangeFingerprint: huella, Insertions: ins, Deletions: del},
		Gates: gates}
	v.Finalize()
	return v
}

// bdEv es una tarjeta que cumple TODO salvo la aceptacion humana: esta en
// Tu aceptacion. Cada test rompe un requisito y mira adonde cae.
func bdEv() Evidence {
	v := bdVeredicto("2026-09-22T15-30-00Z_aaaa1111", bdT0.Add(30*time.Minute), "huella-1", 50, 10, bdGatesVerdes())
	return Evidence{
		Item: item.Item{Slug: bdSlug, Titulo: "Precios por region", Tipo: "feature", Prioridad: "media",
			Pedido: "que el catalogo muestre precios por region", CreadoPor: "hoom test <test@hoom.dev>", CreadoEn: bdT0},
		Now:        bdT0.Add(time.Hour),
		Source:     SourceWorktree,
		Dir:        bdWtDir,
		SpecPath:   bdSpec,
		SpecExists: true,
		Criteria:   []string{"CA-1", "CA-2", "CA-3"},
		Approval:   approval.StatusApproved,
		Verdict:    v, GreenVerdicts: []string{v.ID}, Fingerprint: "huella-1",
		BlockOn:  "high",
		Worktree: true,
	}
}

// bdArbol mueve la evidencia al arbol actual: sin worktree de la tarea.
func bdArbol(ev Evidence) Evidence {
	ev.Source, ev.Dir, ev.Worktree = SourceArbol, ".", false
	ev.ReadyErr = `la tarea "precios" no existe (mira 'hoom task list')`
	return ev
}

func bdCol(t *testing.T, ca string, c Card, want string) {
	t.Helper()
	if c.Column != want {
		t.Fatalf("%s: la columna debe ser %q, fue %q (missing=%q next=%q)", ca, want, c.Column, c.Missing, c.Next)
	}
	for _, col := range Columns {
		if col.ID == want && c.ColumnName != col.Name {
			t.Fatalf("%s: column_name de %s es %q, fue %q", ca, want, col.Name, c.ColumnName)
		}
	}
}

func bdPrimero(t *testing.T, ca string, c Card, want string) {
	t.Helper()
	if len(c.Missing) == 0 || c.Missing[0] != want {
		t.Fatalf("%s: el motivo principal (missing[0]) debe ser %q, fue %q", ca, want, c.Missing)
	}
}

func bdTiene(t *testing.T, ca string, list []string, want string) {
	t.Helper()
	for _, s := range list {
		if strings.Contains(s, want) {
			return
		}
	}
	t.Fatalf("%s: se esperaba una entrada con %q en %q", ca, want, list)
}

func bdNoTiene(t *testing.T, ca string, list []string, prohibido string) {
	t.Helper()
	for _, s := range list {
		if strings.Contains(s, prohibido) {
			t.Fatalf("%s: no debia haber una entrada con %q en %q", ca, prohibido, list)
		}
	}
}

// CA-278: con todas las condiciones cumplidas la columna es tu-aceptacion,
// esperando humano, y el siguiente paso es cerrar la tarea.
func TestCA278_TuAceptacion(t *testing.T) {
	c := Derive(bdEv())
	bdCol(t, "CA-278", c, ColTuAceptacion)
	if !c.WaitingHuman {
		t.Fatal("CA-278: Tu aceptacion espera a un humano")
	}
	if c.Next != "hoom task done precios" {
		t.Fatalf("CA-278: next es 'hoom task done precios', fue %q", c.Next)
	}
	if len(c.Missing) != 1 || c.Missing[0] != "falta tu aceptacion: hoom task done precios" {
		t.Fatalf("CA-278: missing dice que falta la aceptacion: %q", c.Missing)
	}
	if c.Slug != bdSlug || c.Item.Titulo != "Precios por region" {
		t.Fatalf("CA-278: la tarjeta lleva el slug y el item: %+v", c)
	}
	e := c.Evidence
	if e.Source != SourceWorktree || e.Dir != bdWtDir || e.Spec != bdSpec || !e.SpecExists ||
		e.Approval != approval.StatusApproved || e.VerdictID != "2026-09-22T15-30-00Z_aaaa1111" ||
		e.Verdict != "green" || !e.FingerprintMatch || e.ReviewRequired {
		t.Fatalf("CA-278: el medidor de evidencia refleja lo que se leyo: %+v", e)
	}
}

// CA-279: hecho_en gana sobre cualquier evidencia: sin spec, sin veredicto y
// sin worktree la tarjeta es Hecho, con next vacio y missing [].
func TestCA279_Hecho(t *testing.T) {
	cuando := bdT0.Add(48 * time.Hour)
	ev := bdArbol(bdEv())
	ev.Item.HechoEn = &cuando
	ev.Item.CommitFinal = "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c"
	ev.SpecExists = false
	ev.Verdict, ev.GreenVerdicts, ev.Criteria = nil, nil, nil
	ev.Approval = approval.StatusNotApproved
	c := Derive(ev)
	bdCol(t, "CA-279", c, ColHecho)
	if c.Next != "" {
		t.Fatalf("CA-279: Hecho no tiene siguiente paso, fue %q", c.Next)
	}
	if c.Missing == nil || len(c.Missing) != 0 {
		t.Fatalf("CA-279: missing es [] (vacio, no nil): %#v", c.Missing)
	}
	if c.WaitingHuman {
		t.Fatal("CA-279: Hecho no espera a nadie")
	}
	raw, _ := json.Marshal(c)
	if !strings.Contains(string(raw), `"missing":[]`) || !strings.Contains(string(raw), `"next":""`) {
		t.Fatalf("CA-279: el JSON de Hecho trae missing [] y next \"\": %s", raw)
	}

	// hecho_en gana tambien sobre una tarjeta que estaria en cualquier otra
	ev = bdEv()
	ev.Item.HechoEn = &cuando
	ev.Verdict = bdVeredicto("v-rojo", bdT0, "huella-1", 1, 1,
		[]verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
	bdCol(t, "CA-279", Derive(ev), ColHecho)
}

// CA-269: sin spec, backlog; el motivo nombra la ruta y el siguiente paso
// depende de si existe el worktree.
func TestCA269_Backlog(t *testing.T) {
	ev := bdArbol(bdEv())
	ev.SpecExists = false
	c := Derive(ev)
	bdCol(t, "CA-269", c, ColBacklog)
	bdPrimero(t, "CA-269", c, "no hay spec: .hoom/specs/precios.md")
	if c.Next != "hoom task start precios" {
		t.Fatalf("CA-269: sin worktree el siguiente paso es 'hoom task start precios', fue %q", c.Next)
	}
	if c.WaitingHuman {
		t.Fatal("CA-269: Backlog no espera a un humano")
	}

	ev = bdEv()
	ev.SpecExists = false
	c = Derive(ev)
	bdCol(t, "CA-269", c, ColBacklog)
	bdPrimero(t, "CA-269", c, "no hay spec: .hoom/specs/precios.md")
	if !strings.HasPrefix(c.Next, "hoom agent --role arquitecto --task precios") {
		t.Fatalf("CA-269: con worktree el siguiente paso es el arquitecto de la tarea, fue %q", c.Next)
	}
	if c.Evidence.Source != SourceWorktree || c.Evidence.Dir != bdWtDir || c.Evidence.SpecExists {
		t.Fatalf("CA-269: la evidencia dice donde miro: %+v", c.Evidence)
	}
}

// CA-270: lint con issues (o spec ilegible): arquitecto, con los issues tal
// cual en missing.
func TestCA270_Arquitecto(t *testing.T) {
	ev := bdEv()
	ev.LintIssues = []string{`falta la seccion "riesgos"`}
	c := Derive(ev)
	bdCol(t, "CA-270", c, ColArquitecto)
	if !reflect.DeepEqual(c.Missing, []string{`falta la seccion "riesgos"`}) {
		t.Fatalf("CA-270: missing son los issues de lint tal cual: %q", c.Missing)
	}
	if !strings.HasPrefix(c.Next, "hoom agent --role arquitecto --task precios") {
		t.Fatalf("CA-270: next es el arquitecto de la tarea, fue %q", c.Next)
	}
	if len(c.Evidence.LintIssues) != 1 {
		t.Fatalf("CA-270: la evidencia trae los issues: %+v", c.Evidence)
	}

	// spec vacio: todos los issues, y sin worktree el comando no lleva --task
	ev = bdArbol(bdEv())
	ev.LintIssues = []string{`falta la seccion "objetivo"`, `falta la seccion "riesgos"`,
		"no hay criterios de aceptacion identificados como CA-1, CA-2, ... (la trazabilidad spec->test los necesita)"}
	ev.Criteria = nil
	c = Derive(ev)
	bdCol(t, "CA-270", c, ColArquitecto)
	if !reflect.DeepEqual(c.Missing, ev.LintIssues) {
		t.Fatalf("CA-270: missing son todos los issues, en orden: %q", c.Missing)
	}
	if !strings.HasPrefix(c.Next, "hoom agent --role arquitecto") || strings.Contains(c.Next, "--task") {
		t.Fatalf("CA-270: sin worktree, 'hoom agent --role arquitecto' sin --task: %q", c.Next)
	}
}

// CA-271: lint OK sin aprobacion vigente: tu-aprobacion, esperando humano.
func TestCA271_TuAprobacion(t *testing.T) {
	ev := bdEv()
	ev.Approval = approval.StatusNotApproved
	c := Derive(ev)
	bdCol(t, "CA-271", c, ColTuAprobacion)
	bdPrimero(t, "CA-271", c, "el spec no tiene aprobacion humana")
	if !c.WaitingHuman {
		t.Fatal("CA-271: Tu aprobacion espera a un humano")
	}
	if c.Next != "hoom spec approve .hoom/specs/precios.md" {
		t.Fatalf("CA-271: next es 'hoom spec approve .hoom/specs/precios.md', fue %q", c.Next)
	}

	ev.Approval = approval.StatusInvalidated
	c = Derive(ev)
	bdCol(t, "CA-271", c, ColTuAprobacion)
	bdPrimero(t, "CA-271", c, "el spec cambio despues de tu aprobacion")
	if c.Next != "hoom spec approve .hoom/specs/precios.md" || !c.WaitingHuman {
		t.Fatalf("CA-271: invalidada, misma columna y mismo comando: %q %v", c.Next, c.WaitingHuman)
	}
}

// CA-272: aprobado con CA sin test: test-writer, con los CA en missing (en
// el orden del spec) y criteria/traced/untraced en la evidencia.
func TestCA272_TestWriter(t *testing.T) {
	ev := bdEv()
	ev.Criteria = []string{"CA-1", "CA-2", "CA-3", "CA-5"}
	ev.Untraced = []string{"CA-3", "CA-5"}
	c := Derive(ev)
	bdCol(t, "CA-272", c, ColTestWriter)
	bdPrimero(t, "CA-272", c, "criterios sin test: CA-3, CA-5")
	if c.Evidence.Criteria != 4 || c.Evidence.Traced != 2 || !reflect.DeepEqual(c.Evidence.Untraced, []string{"CA-3", "CA-5"}) {
		t.Fatalf("CA-272: criteria 4, traced 2, untraced [CA-3 CA-5]: %+v", c.Evidence)
	}
	if !strings.HasPrefix(c.Next, "hoom agent --role test-writer --task precios --spec .hoom/specs/precios.md") {
		t.Fatalf("CA-272: next es el test-writer de la tarea con el spec, fue %q", c.Next)
	}
	c = Derive(bdArbol(ev))
	if !strings.HasPrefix(c.Next, "hoom agent --role test-writer --spec .hoom/specs/precios.md") {
		t.Fatalf("CA-272: sin worktree el comando no lleva --task: %q", c.Next)
	}
	// trazado del todo: la evidencia dice traced == criteria
	c = Derive(bdEv())
	if c.Evidence.Criteria != 3 || c.Evidence.Traced != 3 || c.Evidence.Untraced == nil || len(c.Evidence.Untraced) != 0 {
		t.Fatalf("CA-272: trazado entero: criteria 3, traced 3, untraced []: %+v", c.Evidence)
	}
}

// CA-273: trazado sin veredicto verde de la tarjeta con la huella actual:
// writer. Sin veredicto, con uno rojo, y con uno verde de otra huella.
func TestCA273_Writer(t *testing.T) {
	ev := bdEv()
	ev.Verdict, ev.GreenVerdicts = nil, nil
	c := Derive(ev)
	bdCol(t, "CA-273", c, ColWriter)
	if len(c.Missing) == 0 || !strings.HasPrefix(c.Missing[0], "no hay veredicto de la tarjeta") {
		t.Fatalf("CA-273: sin veredicto el motivo empieza con 'no hay veredicto de la tarjeta': %q", c.Missing)
	}
	if !strings.HasPrefix(c.Next, "hoom agent --role writer --task precios --spec .hoom/specs/precios.md") {
		t.Fatalf("CA-273: next es el writer de la tarea con el spec, fue %q", c.Next)
	}
	if c.Evidence.VerdictID != "" || c.Evidence.Verdict != "" || c.Evidence.FingerprintMatch {
		t.Fatalf("CA-273: sin veredicto la evidencia no inventa uno: %+v", c.Evidence)
	}
	if c.WaitingHuman {
		t.Fatal("CA-273: Writer no espera a un humano")
	}
	if cc := Derive(bdArbol(ev)); !strings.HasPrefix(cc.Next, "hoom agent --role writer --spec .hoom/specs/precios.md") {
		t.Fatalf("CA-273: sin worktree el comando no lleva --task: %q", cc.Next)
	}

	// rojo con UN gate requerido que fallo
	ev = bdEv()
	ev.Verdict = bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10, []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "test", Required: true, Status: verdict.StatusFail},
		{Name: "lint", Required: false, Status: verdict.StatusFail},
	})
	c = Derive(ev)
	bdCol(t, "CA-273", c, ColWriter)
	bdPrimero(t, "CA-273", c, "veredicto rojo: fallo el gate test")
	if c.Evidence.Verdict != "red" || c.Evidence.VerdictID != "v-rojo" {
		t.Fatalf("CA-273: la evidencia muestra el rojo: %+v", c.Evidence)
	}

	// rojo con varios: los requeridos en fail o error, en el orden del veredicto
	ev.Verdict = bdVeredicto("v-rojo2", bdT0.Add(40*time.Minute), "huella-1", 50, 10, []verdict.GateResult{
		{Name: "test", Required: true, Status: verdict.StatusFail},
		{Name: "lint", Required: false, Status: verdict.StatusFail},
		{Name: "vet", Required: true, Status: verdict.StatusPass},
		{Name: "build", Required: true, Status: verdict.StatusError},
	})
	c = Derive(ev)
	bdCol(t, "CA-273", c, ColWriter)
	bdPrimero(t, "CA-273", c, "veredicto rojo: fallaron los gates test, build")

	// verde de otra huella: el codigo cambio
	ev = bdEv()
	ev.Fingerprint = "huella-2"
	c = Derive(ev)
	bdCol(t, "CA-273", c, ColWriter)
	bdPrimero(t, "CA-273", c, "el codigo cambio despues del ultimo verde")
	if c.Evidence.FingerprintMatch {
		t.Fatalf("CA-273: fingerprint_match false con otra huella: %+v", c.Evidence)
	}
}

// CA-274: la review se exige por tamaño (> 400 lineas) y se cumple con un
// registro de la tarjeta con las 4 lentes sobre un verde de la tarjeta.
func TestCA274_ReviewPorTamano(t *testing.T) {
	grande := func() Evidence {
		ev := bdEv()
		v := bdVeredicto("v-grande", bdT0.Add(30*time.Minute), "huella-1", 500, 112, bdGatesVerdes())
		ev.Verdict, ev.GreenVerdicts = v, []string{"v-viejo-verde", v.ID}
		return ev
	}
	ev := grande()
	c := Derive(ev)
	bdCol(t, "CA-274", c, ColReview)
	bdPrimero(t, "CA-274", c, "la review exige las 4 lentes (612 lineas > 400) y no hay registro de review")
	if !c.Evidence.ReviewRequired || c.Evidence.ReviewID != "" {
		t.Fatalf("CA-274: review_required true sin review_id: %+v", c.Evidence)
	}
	if c.Next != "hoom review --task precios --spec .hoom/specs/precios.md" {
		t.Fatalf("CA-274: next es la review de la tarea con el spec, fue %q", c.Next)
	}
	if c.WaitingHuman {
		t.Fatal("CA-274: Review no espera a un humano")
	}

	rec := func(id string, lentes []string, verdictID string) reviewcmd.Record {
		return reviewcmd.Record{ID: id, CreatedAt: bdT0.Add(35 * time.Minute), Task: bdSlug, Spec: bdSpec,
			Fingerprint: "huella-1", VerdictID: verdictID, Verdict: "green", Lenses: lentes,
			Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}}
	}
	// cumple: 4 lentes sobre el verde actual
	ev = grande()
	ev.Reviews = []reviewcmd.Record{rec("r-ok", reviewcmd.Lentes, "v-grande")}
	c = Derive(ev)
	bdCol(t, "CA-274", c, ColTuAceptacion)
	if c.Evidence.ReviewID != "r-ok" || !c.Evidence.ReviewRequired {
		t.Fatalf("CA-274: la evidencia muestra el registro que cumplio: %+v", c.Evidence)
	}

	// cumple: 4 lentes sobre un verde ANTERIOR de la tarjeta (el codigo
	// cambio despues de la review y volvio a verde)
	ev = grande()
	ev.Reviews = []reviewcmd.Record{rec("r-viejo", reviewcmd.Lentes, "v-viejo-verde")}
	c = Derive(ev)
	bdCol(t, "CA-274", c, ColTuAceptacion)
	if c.Evidence.ReviewID != "r-viejo" {
		t.Fatalf("CA-274: el registro sigue valiendo tras corregir: %+v", c.Evidence)
	}

	// no cumplen: una sola lente, un verdict_id rojo, uno de otro spec
	for nombre, r := range map[string]reviewcmd.Record{
		"una lente":       rec("r-1", []string{"reliability"}, "v-grande"),
		"tres lentes":     rec("r-3", []string{"readability", "reliability", "risk"}, "v-grande"),
		"sobre un rojo":   rec("r-rojo", reviewcmd.Lentes, "v-rojo-de-la-tarjeta"),
		"sobre otro spec": rec("r-otro", reviewcmd.Lentes, "v-verde-de-otro-spec"),
		"sin verdict_id":  rec("r-vacio", reviewcmd.Lentes, ""),
	} {
		ev = grande()
		ev.Reviews = []reviewcmd.Record{r}
		c = Derive(ev)
		bdCol(t, "CA-274 ("+nombre+")", c, ColReview)
		bdPrimero(t, "CA-274 ("+nombre+")", c, "la review exige las 4 lentes (612 lineas > 400) y no hay registro de review")
		if c.Evidence.ReviewID != "" {
			t.Fatalf("CA-274 (%s): un registro que no cumple no es review_id: %+v", nombre, c.Evidence)
		}
	}

	// 400 lineas o menos: no se exige; 401 si
	for _, caso := range []struct {
		ins, del int
		exige    bool
	}{{300, 100, false}, {400, 0, false}, {0, 0, false}, {400, 1, true}, {201, 200, true}} {
		ev = bdEv()
		ev.Verdict = bdVeredicto("v-tam", bdT0.Add(30*time.Minute), "huella-1", caso.ins, caso.del, bdGatesVerdes())
		ev.GreenVerdicts = []string{"v-tam"}
		c = Derive(ev)
		if c.Evidence.ReviewRequired != caso.exige {
			t.Fatalf("CA-274: con %d+%d lineas review_required=%v, fue %v", caso.ins, caso.del, caso.exige, c.Evidence.ReviewRequired)
		}
		want := ColTuAceptacion
		if caso.exige {
			want = ColReview
		}
		bdCol(t, "CA-274", c, want)
	}
}

func bdHallazgo(id, sev, task string) finding.Item {
	return finding.Item{Finding: finding.Finding{ID: id, CreatedAt: bdT0.Add(45 * time.Minute), Severity: sev,
		Lens: "risk", Description: "algo " + sev, Author: "reviewer", Task: task}, Status: finding.StatusOpen}
}

// CA-275: un hallazgo abierto de la tarjeta que bloquea la deja en review
// aunque el cambio sea chico; uno por debajo del umbral no. El umbral es el
// de block_on.
func TestCA275_ReviewPorHallazgos(t *testing.T) {
	ev := bdEv()
	ev.Findings = []finding.Item{bdHallazgo("f-high", "high", bdSlug)}
	c := Derive(ev)
	bdCol(t, "CA-275", c, ColReview)
	bdPrimero(t, "CA-275", c, "hallazgos abiertos que bloquean: f-high")
	if !reflect.DeepEqual(c.Evidence.BlockingFindings, []string{"f-high"}) {
		t.Fatalf("CA-275: blocking_findings nombra el hallazgo: %+v", c.Evidence)
	}
	if !strings.HasPrefix(c.Next, "hoom finding resolve f-high") {
		t.Fatalf("CA-275: next es resolver el hallazgo, fue %q", c.Next)
	}

	// dos que bloquean y uno que no, con umbral high
	ev.Findings = []finding.Item{bdHallazgo("f-a", "high", bdSlug), bdHallazgo("f-m", "medium", bdSlug), bdHallazgo("f-b", "high", bdSlug)}
	c = Derive(ev)
	bdCol(t, "CA-275", c, ColReview)
	bdPrimero(t, "CA-275", c, "hallazgos abiertos que bloquean: f-a, f-b")
	if !reflect.DeepEqual(c.Evidence.BlockingFindings, []string{"f-a", "f-b"}) {
		t.Fatalf("CA-275: solo los que bloquean: %v", c.Evidence.BlockingFindings)
	}

	// por debajo del umbral: no mantiene la tarjeta en review
	ev.Findings = []finding.Item{bdHallazgo("f-m", "medium", bdSlug), bdHallazgo("f-l", "low", bdSlug)}
	c = Derive(ev)
	bdCol(t, "CA-275", c, ColTuAceptacion)
	if c.Evidence.BlockingFindings == nil || len(c.Evidence.BlockingFindings) != 0 {
		t.Fatalf("CA-275: blocking_findings [] (no nil): %#v", c.Evidence.BlockingFindings)
	}

	// con block_on medium, un medium bloquea
	ev.BlockOn = "medium"
	c = Derive(ev)
	bdCol(t, "CA-275", c, ColReview)
	bdPrimero(t, "CA-275", c, "hallazgos abiertos que bloquean: f-m")
}

// CA-276: el veredicto verde de la tarjeta necesita findings_open y
// spec_approved en pass para salir de review.
func TestCA276_ReviewPorGates(t *testing.T) {
	sin := func(nombre string) Evidence {
		ev := bdEv()
		var gs []verdict.GateResult
		for _, g := range bdGatesVerdes() {
			if g.Name != nombre {
				gs = append(gs, g)
			}
		}
		ev.Verdict = bdVeredicto("v-sin-"+nombre, bdT0.Add(30*time.Minute), "huella-1", 50, 10, gs)
		ev.GreenVerdicts = []string{ev.Verdict.ID}
		return ev
	}
	c := Derive(sin("findings_open"))
	bdCol(t, "CA-276", c, ColReview)
	bdPrimero(t, "CA-276", c, `falta el gate findings_open: agrega "findings: { block_on: high }" a hoom.yaml y vuelve a verificar`)
	bdTiene(t, "CA-276", []string{c.Next}, "block_on: high")

	c = Derive(sin("spec_approved"))
	bdCol(t, "CA-276", c, ColReview)
	bdPrimero(t, "CA-276", c, "el veredicto no trae spec_approved en pass")

	// spec_approved presente pero no en pass (no requerido: el verde no lo ve)
	ev := bdEv()
	gs := bdGatesVerdes()
	for i := range gs {
		if gs[i].Name == "spec_approved" {
			gs[i].Required, gs[i].Status = false, verdict.StatusFail
		}
	}
	ev.Verdict = bdVeredicto("v-sa-fail", bdT0.Add(30*time.Minute), "huella-1", 50, 10, gs)
	ev.GreenVerdicts = []string{ev.Verdict.ID}
	c = Derive(ev)
	bdCol(t, "CA-276", c, ColReview)
	bdTiene(t, "CA-276", c.Missing, "el veredicto no trae spec_approved en pass")
}

// CA-277: la condicion 5 es taskcmd.Ready. Sin worktree la falta es "cerrar
// exige la tarea"; con worktree, el mensaje de Ready tal cual.
func TestCA277_ReviewPorCierre(t *testing.T) {
	c := Derive(bdArbol(bdEv()))
	bdCol(t, "CA-277", c, ColReview)
	bdPrimero(t, "CA-277", c, "cerrar exige la tarea: hoom task start precios")
	bdNoTiene(t, "CA-277", c.Missing, "no existe")
	bdTiene(t, "CA-277", []string{c.Next}, "hoom task start precios")

	ev := bdEv()
	ev.ReadyErr = "la tarea \"precios\" tiene cambios sin commitear (incluidos posibles veredictos).\n  Accion: commitea todo dentro de .hoom/worktrees/precios y repite 'hoom task done precios'"
	c = Derive(ev)
	bdCol(t, "CA-277", c, ColReview)
	bdPrimero(t, "CA-277", c, ev.ReadyErr)

	ev.ReadyErr = `el ultimo veredicto de "precios" es ROJO (2026-09-22T16-00-00Z_bbbb2222). Accion: corrige y re-ejecuta 'hoom verify' en el worktree`
	c = Derive(ev)
	bdCol(t, "CA-277", c, ColReview)
	bdPrimero(t, "CA-277", c, ev.ReadyErr)
}

// CA-274..CA-277: en Review van TODAS las condiciones que fallan, en el
// orden del contrato.
func TestCA277_ReviewListaTodasEnOrden(t *testing.T) {
	ev := bdArbol(bdEv())
	var gs []verdict.GateResult
	for _, g := range bdGatesVerdes() {
		if g.Name != "findings_open" && g.Name != "spec_approved" {
			gs = append(gs, g)
		}
	}
	ev.Verdict = bdVeredicto("v-todo", bdT0.Add(30*time.Minute), "huella-1", 500, 112, gs)
	ev.GreenVerdicts = []string{"v-todo"}
	ev.Findings = []finding.Item{bdHallazgo("f1", "high", bdSlug), bdHallazgo("f2", "high", bdSlug)}
	c := Derive(ev)
	bdCol(t, "CA-274", c, ColReview)
	want := []string{
		"la review exige las 4 lentes (612 lineas > 400) y no hay registro de review",
		"hallazgos abiertos que bloquean: f1, f2",
		"el veredicto no trae spec_approved en pass",
		`falta el gate findings_open: agrega "findings: { block_on: high }" a hoom.yaml y vuelve a verificar`,
		"cerrar exige la tarea: hoom task start precios",
	}
	if !reflect.DeepEqual(c.Missing, want) {
		t.Fatalf("CA-277: todas las faltas de Review, en orden:\nquiere %q\nfue    %q", want, c.Missing)
	}
	if c.Next != "hoom review --spec .hoom/specs/precios.md" {
		t.Fatalf("CA-274: next es el comando de la PRIMERA condicion (sin --task en el arbol), fue %q", c.Next)
	}
}

// CA-273/CA-275: el orden de las columnas manda: un requisito anterior que
// falla gana sobre los de despues.
func TestCA273_PrimerRequisitoQueFallaGana(t *testing.T) {
	ev := bdEv()
	ev.Approval = approval.StatusNotApproved
	ev.Untraced = []string{"CA-2"}
	ev.Verdict = nil
	ev.Findings = []finding.Item{bdHallazgo("f1", "high", bdSlug)}
	bdCol(t, "CA-271", Derive(ev), ColTuAprobacion)

	ev = bdEv()
	ev.LintIssues = []string{"falta la seccion \"riesgos\""}
	ev.Approval = approval.StatusNotApproved
	bdCol(t, "CA-270", Derive(ev), ColArquitecto)

	// un hallazgo que bloquea registrado despues del verde: Review, no Writer
	ev = bdEv()
	ev.Findings = []finding.Item{bdHallazgo("f-tarde", "high", bdSlug)}
	bdCol(t, "CA-275", Derive(ev), ColReview)
}

// CA-284: waiting_human es true exactamente en las dos columnas humanas.
func TestCA284_WaitingHumanSoloEnLasHumanas(t *testing.T) {
	cuando := bdT0.Add(time.Hour)
	casos := map[string]func(*Evidence){
		ColBacklog:      func(e *Evidence) { e.SpecExists = false },
		ColArquitecto:   func(e *Evidence) { e.LintIssues = []string{"falta la seccion \"riesgos\""} },
		ColTuAprobacion: func(e *Evidence) { e.Approval = approval.StatusNotApproved },
		ColTestWriter:   func(e *Evidence) { e.Untraced = []string{"CA-2"} },
		ColWriter:       func(e *Evidence) { e.Verdict, e.GreenVerdicts = nil, nil },
		ColReview:       func(e *Evidence) { e.ReadyErr = "algo impide cerrar" },
		ColTuAceptacion: func(e *Evidence) {},
		ColHecho:        func(e *Evidence) { e.Item.HechoEn = &cuando },
	}
	vistas := 0
	for _, col := range Columns {
		mut, ok := casos[col.ID]
		if !ok {
			t.Fatalf("CA-284: columna sin caso: %s", col.ID)
		}
		ev := bdEv()
		mut(&ev)
		c := Derive(ev)
		bdCol(t, "CA-284", c, col.ID)
		humana := col.ID == ColTuAprobacion || col.ID == ColTuAceptacion
		if c.WaitingHuman != humana {
			t.Fatalf("CA-284: waiting_human en %s debe ser %v, fue %v", col.ID, humana, c.WaitingHuman)
		}
		if col.Human != humana {
			t.Fatalf("CA-284: Columns marca human solo en las dos humanas: %+v", col)
		}
		vistas++
	}
	if vistas != 8 || len(Columns) != 8 {
		t.Fatalf("CA-284: son 8 columnas, vi %d (Columns tiene %d)", vistas, len(Columns))
	}
}

// CA-288: el orden fijo de las columnas, con sus nombres.
func TestCA288_ColumnasFijas(t *testing.T) {
	want := []Column{
		{"backlog", "Backlog", false}, {"arquitecto", "Arquitecto", false},
		{"tu-aprobacion", "Tu aprobacion", true}, {"test-writer", "Test-writer", false},
		{"writer", "Writer", false}, {"review", "Review", false},
		{"tu-aceptacion", "Tu aceptacion", true}, {"hecho", "Hecho", false},
	}
	if !reflect.DeepEqual(Columns, want) {
		t.Fatalf("CA-288: las 8 columnas en orden: %+v", Columns)
	}
}

// CA-281: Derive es pura: la misma Evidence da la misma Card (igualdad
// profunda, dos llamadas), y no toca la Evidence que recibe.
func TestCA281_DeriveEsPura(t *testing.T) {
	armar := func() Evidence {
		ev := bdEv()
		ev.Verdict = bdVeredicto("v-grande", bdT0.Add(30*time.Minute), "huella-1", 500, 112, bdGatesVerdes())
		ev.GreenVerdicts = []string{"v-grande"}
		ev.Findings = []finding.Item{bdHallazgo("f2", "high", bdSlug), bdHallazgo("f1", "high", bdSlug)}
		ev.Uncommitted = []string{".hoom/specs/precios.md", ".hoom/items/precios.yaml"}
		ev.Envelopes = []EnvelopeState{{Record: envelope.Record{ID: "e1", Role: "writer", Provider: "claude",
			Task: bdSlug, Stage: "run", Step: 2, Steps: 5, Status: envelope.StatusRunning,
			StartedAt: bdT0, UpdatedAt: bdT0.Add(time.Minute)}, Alive: true}}
		ev.Runs = []RunState{{Meta: runcmd.Meta{ID: "r1", Provider: "claude", Task: bdSlug, Status: runcmd.StatusDone,
			Usage: &providers.Usage{CostUSD: bdF(0.3), InputTokens: 10, OutputTokens: 5}}}}
		return ev
	}
	ev := armar()
	a := Derive(ev)
	b := Derive(ev)
	// no vale la pureza de una tarjeta vacia: la evidencia pone la tarjeta en
	// Review (612 lineas, dos hallazgos que bloquean) y en curso
	bdCol(t, "CA-281", a, ColReview)
	if a.Running == nil || len(a.Unsynced) != 2 || len(a.Missing) < 2 {
		t.Fatalf("CA-281: la tarjeta de prueba no es trivial: %+v", a)
	}
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("CA-281: dos llamadas con la misma evidencia dan la misma tarjeta:\n%+v\n%+v", a, b)
	}
	if !reflect.DeepEqual(ev, armar()) {
		t.Fatalf("CA-281: Derive no modifica la evidencia que recibe (ni ordena sus listas en el lugar)")
	}
	// la tarjeta no comparte listas con la evidencia: mutar una no mueve la otra
	if len(a.Unsynced) > 0 {
		a.Unsynced[0] = "mutado"
		if ev.Uncommitted[0] == "mutado" || ev.Uncommitted[1] == "mutado" {
			t.Fatal("CA-281: la tarjeta no comparte el arreglo de la evidencia")
		}
	}
}

// CA-282: en curso: un sobre sin cerrar cuyo dueño vive; un run suelto
// vivo tambien, con stage run; si hay varios va el mas nuevo.
func TestCA282_EnCurso(t *testing.T) {
	ev := bdEv()
	ev.Envelopes = []EnvelopeState{{Record: envelope.Record{ID: "env-1", RunID: "run-1", Role: "writer", Provider: "claude",
		Task: bdSlug, Stage: "run", Step: 2, Steps: 5, Status: envelope.StatusRunning,
		StartedAt: bdT0.Add(10 * time.Minute), UpdatedAt: bdT0.Add(11 * time.Minute)}, Alive: true}}
	ev.Runs = []RunState{{Meta: runcmd.Meta{ID: "run-1", Provider: "claude", Role: "writer", Task: bdSlug,
		Status: runcmd.StatusRunning, PID: 4242, CreatedAt: bdT0.Add(10 * time.Minute)}, Alive: true}}
	c := Derive(ev)
	r := c.Running
	if r == nil {
		t.Fatalf("CA-282: un sobre abierto con dueño vivo esta en curso: %+v", c)
	}
	if r.EnvelopeID != "env-1" || r.RunID != "run-1" || r.Role != "writer" || r.Provider != "claude" ||
		r.Stage != "run" || r.Step != 2 || r.Steps != 5 || !r.StartedAt.Equal(bdT0.Add(10*time.Minute)) {
		t.Fatalf("CA-282: running trae envelope_id, run_id, role, provider, stage, step, steps, started_at: %+v", r)
	}
	if c.Interrupted != nil {
		t.Fatalf("CA-282: en curso no es interrumpido: %+v", c.Interrupted)
	}
	bdCol(t, "CA-282", c, ColTuAceptacion) // el subestado es ortogonal a la columna

	// un run suelto (sin sobre) vivo: stage run
	ev = bdEv()
	ev.Runs = []RunState{{Meta: runcmd.Meta{ID: "run-suelto", Provider: "codex", Role: "reviewer", Task: bdSlug,
		Status: runcmd.StatusRunning, PID: 4343, CreatedAt: bdT0.Add(20 * time.Minute)}, Alive: true}}
	c = Derive(ev)
	if c.Running == nil || c.Running.Stage != "run" || c.Running.RunID != "run-suelto" || c.Running.EnvelopeID != "" ||
		c.Running.Provider != "codex" || c.Running.Role != "reviewer" {
		t.Fatalf("CA-282: un run suelto vivo esta en curso con stage run: %+v", c.Running)
	}

	// un run suelto muerto no esta en curso ni interrumpido
	ev.Runs[0].Alive = false
	c = Derive(ev)
	if c.Running != nil || c.Interrupted != nil {
		t.Fatalf("CA-282: un run suelto sin dueño vivo no es subestado: %+v %+v", c.Running, c.Interrupted)
	}

	// varios vivos: el mas nuevo (la evidencia viene del mas nuevo al mas viejo)
	ev = bdEv()
	ev.Envelopes = []EnvelopeState{
		{Record: envelope.Record{ID: "env-nuevo", Role: "writer", Provider: "claude", Task: bdSlug, Stage: "verify",
			Step: 4, Steps: 5, Status: envelope.StatusRunning, StartedAt: bdT0.Add(30 * time.Minute)}, Alive: true},
		{Record: envelope.Record{ID: "env-viejo", Role: "test-writer", Provider: "claude", Task: bdSlug, Stage: "run",
			Step: 3, Steps: 6, Status: envelope.StatusRunning, StartedAt: bdT0.Add(5 * time.Minute)}, Alive: true},
	}
	c = Derive(ev)
	if c.Running == nil || c.Running.EnvelopeID != "env-nuevo" || c.Running.Stage != "verify" {
		t.Fatalf("CA-282: con varios en curso va el mas nuevo: %+v", c.Running)
	}
}

// CA-283: interrumpido: un sobre sin cerrar cuyo dueño no vive, con el paso
// donde quedo; running null.
func TestCA283_Interrumpido(t *testing.T) {
	ev := bdEv()
	ev.Envelopes = []EnvelopeState{{Record: envelope.Record{ID: "env-caido", RunID: "run-caido", Role: "writer",
		Provider: "claude", Task: bdSlug, Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning,
		StartedAt: bdT0, UpdatedAt: bdT0.Add(2 * time.Minute)}, Alive: false}}
	c := Derive(ev)
	if c.Running != nil {
		t.Fatalf("CA-283: un sobre sin dueño vivo no esta en curso: %+v", c.Running)
	}
	i := c.Interrupted
	if i == nil || i.EnvelopeID != "env-caido" || i.Stage != "run" || i.Step != 3 || i.Steps != 5 ||
		!i.UpdatedAt.Equal(bdT0.Add(2*time.Minute)) {
		t.Fatalf("CA-283: interrupted trae envelope_id, stage, step, steps y updated_at: %+v", i)
	}

	// un sobre CERRADO nunca es interrumpido
	ev.Envelopes[0].Record.Status = envelope.StatusDeliverable
	ev.Envelopes[0].Record.EndedAt = bdT0.Add(3 * time.Minute)
	c = Derive(ev)
	if c.Interrupted != nil || c.Running != nil {
		t.Fatalf("CA-283: un sobre cerrado no es subestado de vida: %+v %+v", c.Interrupted, c.Running)
	}
}

// CA-285: rojo: gana lo mas nuevo entre el veredicto de la tarjeta y su
// ultimo sobre cerrado.
func TestCA285_Rojo(t *testing.T) {
	rojo := bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10, []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "test", Required: true, Status: verdict.StatusFail},
	})
	ev := bdEv()
	ev.Verdict = rojo
	c := Derive(ev)
	if c.Red == nil || c.Red.Source != RedVerdict || c.Red.ID != "v-rojo" || c.Red.Reason != "veredicto rojo: fallo el gate test" {
		t.Fatalf("CA-285: un veredicto rojo es el rojo de la tarjeta: %+v", c.Red)
	}

	sobre := func(id, status, stage, note string, fin time.Time) EnvelopeState {
		return EnvelopeState{Record: envelope.Record{ID: id, Role: "test-writer", Provider: "claude", Task: bdSlug,
			Stage: stage, Step: 4, Steps: 6, Status: status, Note: note, StartedAt: fin.Add(-10 * time.Minute),
			UpdatedAt: fin, EndedAt: fin}}
	}
	// sobre no-entregable mas nuevo que el veredicto verde
	ev = bdEv()
	ev.Envelopes = []EnvelopeState{sobre("env-rojo", envelope.StatusNotDeliverable, "verify", "spec_trace: 2 criterios sin test", bdT0.Add(50*time.Minute))}
	c = Derive(ev)
	if c.Red == nil || c.Red.Source != RedEnvelope || c.Red.ID != "env-rojo" ||
		c.Red.Reason != "el sobre de test-writer corto en verify: spec_trace: 2 criterios sin test" {
		t.Fatalf("CA-285: el sobre fallido mas nuevo es el rojo: %+v", c.Red)
	}
	// sin nota: sin ": <note>"
	ev.Envelopes = []EnvelopeState{sobre("env-sin-nota", envelope.StatusNoDelivery, "scope", "", bdT0.Add(50*time.Minute))}
	c = Derive(ev)
	if c.Red == nil || c.Red.Source != RedEnvelope || c.Red.Reason != "el sobre de test-writer corto en scope" {
		t.Fatalf("CA-285: un sobre sin-entrega sin nota: %+v", c.Red)
	}
	// un verde POSTERIOR al sobre fallido deja red null
	ev = bdEv()
	ev.Envelopes = []EnvelopeState{sobre("env-rojo", envelope.StatusNotDeliverable, "verify", "x", bdT0.Add(20*time.Minute))}
	c = Derive(ev)
	if c.Red != nil {
		t.Fatalf("CA-285: el verde posterior al sobre fallido deja red null: %+v", c.Red)
	}
	// un entregable posterior a un veredicto rojo tampoco es rojo
	ev = bdEv()
	ev.Verdict = rojo
	ev.Envelopes = []EnvelopeState{sobre("env-ok", envelope.StatusDeliverable, "ok", "", bdT0.Add(50*time.Minute))}
	c = Derive(ev)
	if c.Red != nil {
		t.Fatalf("CA-285: si lo mas nuevo es entregable no hay rojo: %+v", c.Red)
	}
	// un sobre abierto no compite: el rojo sigue siendo el veredicto
	ev.Envelopes = []EnvelopeState{{Record: envelope.Record{ID: "env-abierto", Role: "writer", Task: bdSlug,
		Status: envelope.StatusRunning, Stage: "run", StartedAt: bdT0.Add(55 * time.Minute)}, Alive: true}}
	c = Derive(ev)
	if c.Red == nil || c.Red.Source != RedVerdict {
		t.Fatalf("CA-285: un sobre sin cerrar no pisa el rojo del veredicto: %+v", c.Red)
	}
	// sin veredicto ni sobres: sin rojo
	ev = bdEv()
	ev.Verdict, ev.GreenVerdicts = nil, nil
	if c = Derive(ev); c.Red != nil {
		t.Fatalf("CA-285: sin evidencia de falla no hay rojo: %+v", c.Red)
	}
}

// CA-286: unsynced son las rutas sin commitear de la evidencia, ordenadas;
// vacio = [] (no null).
func TestCA286_Unsynced(t *testing.T) {
	ev := bdEv()
	ev.Uncommitted = []string{".hoom/specs/precios.md", ".hoom/items/precios.yaml", ".hoom/approvals/precios_abcd1234.json"}
	c := Derive(ev)
	want := []string{".hoom/approvals/precios_abcd1234.json", ".hoom/items/precios.yaml", ".hoom/specs/precios.md"}
	if !reflect.DeepEqual(c.Unsynced, want) {
		t.Fatalf("CA-286: unsynced ordenado: %q", c.Unsynced)
	}
	c = Derive(bdEv())
	if c.Unsynced == nil || len(c.Unsynced) != 0 {
		t.Fatalf("CA-286: sincronizada = unsynced [] (no nil): %#v", c.Unsynced)
	}
}

func bdCasi(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// CA-287: gasto: la suma de los runs de la tarjeta; cost_usd null si nadie
// reporto costo; el usage de un sobre cuenta solo si su run no tiene sidecar.
func TestCA287_Gasto(t *testing.T) {
	ev := bdEv()
	ev.Item.PresupuestoUSD = bdF(5)
	ev.Runs = []RunState{
		{Meta: runcmd.Meta{ID: "r-claude", Provider: "claude", Task: bdSlug, Status: runcmd.StatusDone,
			Usage: &providers.Usage{CostUSD: bdF(0.8), InputTokens: 1000, OutputTokens: 100}}},
		{Meta: runcmd.Meta{ID: "r-codex", Provider: "codex", Task: bdSlug, Status: runcmd.StatusDone,
			Usage: &providers.Usage{InputTokens: 50000, OutputTokens: 2900}}},
	}
	// el sobre del run de claude trae el mismo usage: no se cuenta dos veces
	ev.Envelopes = []EnvelopeState{{Record: envelope.Record{ID: "e-claude", RunID: "r-claude", Role: "writer",
		Task: bdSlug, Status: envelope.StatusDeliverable, Stage: "ok", EndedAt: bdT0.Add(20 * time.Minute),
		Usage: &providers.Usage{CostUSD: bdF(0.8), InputTokens: 1000, OutputTokens: 100}}}}
	s := Derive(ev).Spend
	if s.CostUSD == nil || !bdCasi(*s.CostUSD, 0.8) {
		t.Fatalf("CA-287: cost_usd 0.8 (el run de codex no reporta costo y el sobre no se cuenta dos veces): %+v", s.CostUSD)
	}
	if s.Runs != 2 || s.RunsWithoutCost != 1 || s.InputTokens != 51000 || s.OutputTokens != 3000 {
		t.Fatalf("CA-287: runs 2, runs_without_cost 1, tokens 51000/3000: %+v", s)
	}
	if s.BudgetUSD == nil || *s.BudgetUSD != 5 {
		t.Fatalf("CA-287: budget_usd es el presupuesto_usd del item: %v", s.BudgetUSD)
	}

	// solo runs sin costo: cost_usd null, nunca 0
	ev = bdEv()
	ev.Runs = []RunState{
		{Meta: runcmd.Meta{ID: "r1", Provider: "codex", Task: bdSlug, Status: runcmd.StatusDone,
			Usage: &providers.Usage{InputTokens: 10, OutputTokens: 1}}},
		{Meta: runcmd.Meta{ID: "r2", Provider: "codex", Task: bdSlug, Status: runcmd.StatusDone}},
	}
	s = Derive(ev).Spend
	if s.CostUSD != nil || s.Runs != 2 || s.RunsWithoutCost != 2 || s.BudgetUSD != nil {
		t.Fatalf("CA-287: sin ningun costo reportado cost_usd es null: %+v", s)
	}
	raw, _ := json.Marshal(Derive(ev))
	if !strings.Contains(string(raw), `"cost_usd":null`) || !strings.Contains(string(raw), `"budget_usd":null`) {
		t.Fatalf("CA-287: en JSON, cost_usd y budget_usd null: %s", raw)
	}

	// un sobre cuyo run NO tiene sidecar suma su propio usage
	ev = bdEv()
	ev.Runs = []RunState{{Meta: runcmd.Meta{ID: "r-con", Provider: "claude", Task: bdSlug, Status: runcmd.StatusDone,
		Usage: &providers.Usage{CostUSD: bdF(0.8), InputTokens: 1000, OutputTokens: 100}}}}
	ev.Envelopes = []EnvelopeState{{Record: envelope.Record{ID: "e-huerfano", RunID: "r-sin-sidecar", Role: "writer",
		Task: bdSlug, Status: envelope.StatusDeliverable, Stage: "ok", EndedAt: bdT0.Add(20 * time.Minute),
		Usage: &providers.Usage{CostUSD: bdF(0.5), InputTokens: 10, OutputTokens: 1}}}}
	s = Derive(ev).Spend
	if s.CostUSD == nil || !bdCasi(*s.CostUSD, 1.3) || s.InputTokens != 1010 || s.OutputTokens != 101 {
		t.Fatalf("CA-287: el usage del sobre sin sidecar suma: cost 1.3, tokens 1010/101: %+v %v", s, s.CostUSD)
	}

	// sin runs ni sobres: todo en cero y cost null
	s = Derive(bdEv()).Spend
	if s.CostUSD != nil || s.Runs != 0 || s.RunsWithoutCost != 0 || s.InputTokens != 0 || s.OutputTokens != 0 {
		t.Fatalf("CA-287: sin telemetria no hay gasto: %+v", s)
	}
}

// CA-288: las listas de la tarjeta nunca son null en JSON; los subestados
// ausentes si son null.
func TestCA288_CardJSONSinNull(t *testing.T) {
	ev := bdEv()
	ev.LintIssues, ev.Untraced, ev.Uncommitted, ev.Findings = nil, nil, nil, nil
	raw, err := json.Marshal(Derive(ev))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"missing", "unsynced"} {
		if _, ok := m[k].([]any); !ok {
			t.Fatalf("CA-288: %s es una lista (no null): %s", k, raw)
		}
	}
	e, _ := m["evidence"].(map[string]any)
	for _, k := range []string{"lint_issues", "untraced", "blocking_findings"} {
		if _, ok := e[k].([]any); !ok {
			t.Fatalf("CA-288: evidence.%s es una lista (no null): %s", k, raw)
		}
	}
	for _, k := range []string{"running", "interrupted", "red"} {
		if v, ok := m[k]; !ok || v != nil {
			t.Fatalf("CA-288: %s ausente es null: %s", k, raw)
		}
	}
	for _, k := range []string{"slug", "item", "column", "column_name", "next", "evidence", "waiting_human", "spend"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("CA-288: a la tarjeta le falta %q: %s", k, raw)
		}
	}
}
