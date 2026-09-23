// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-313..CA-315): boardcmd.DetailFor lee con el mismo Gather que CardFor,
// falla igual que el, es de solo lectura, y junta por slug lo que ya tiene
// verbo: el spec y sus criterios con su traza, el veredicto completo, los
// hallazgos de la tarjeta (abiertos y resueltos), sus reviews, lo que corrio
// (work, con su usage y su duracion) y las rutas de cada artefacto. El diff
// base...HEAD solo con withDiff y con el worktree de la tarea.
package boardcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const tbNotaSinWorktree = "sin espacio de trabajo de la tarea: no hay diff base...HEAD que mostrar"

// tbSpecCon arma un spec con las 7 secciones y algo escrito en Contratos.
func tbSpecCon(contratos, criterios string) string {
	return "# Spec: precios\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\n" + contratos + "\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n" + criterios + "\n\n## Decisiones\nx\n\n## Riesgos\nx\n"
}

func tbSha8(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])[:8]
}

// tbExisten exige que cada ruta no vacia de paths exista bajo root.
func tbExisten(t *testing.T, ca, root string, p Paths) {
	t.Helper()
	todas := append([]string{p.Item, p.Dir, p.Spec, p.Approval, p.Verdict}, p.Reviews...)
	todas = append(todas, p.Findings...)
	for _, rel := range todas {
		if rel == "" {
			continue
		}
		if filepath.IsAbs(rel) || strings.HasPrefix(rel, "..") || strings.Contains(rel, `\`) {
			t.Fatalf("%s: las rutas de paths son relativas a root, con barras: %q", ca, rel)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("%s: la ruta %q de paths existe en disco: %v", ca, rel, err)
		}
	}
}

func tbOrdenadas(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}

// tbJSONMap serializa y vuelve a leer como mapa.
func tbJSONMap(t *testing.T, v any) (map[string]any, string) {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m, string(raw)
}

// CA-313: el detalle de una tarjeta con la evidencia en el arbol actual:
// card igual a la de CardFor, el spec con su markdown y la aprobacion
// vigente, los criterios con texto, traza y archivos, el veredicto completo,
// los hallazgos de la tarjeta (abiertos y resueltos, ninguno ajeno), sus
// reviews y las rutas de cada artefacto. Sin withDiff no hay diff; con
// withDiff y sin worktree, el diff no esta disponible y lo dice.
func TestCA313_DetalleDesdeElArbol(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	bdItemArchivo(t, root, bdSlug, "Precios", "pedido: el pedido de precios\n")
	specBody := tbSpecCon("El redondeo de CA-7 se discute aca.",
		"- CA-1: el precio sale por region\n  y se redondea.\n- CA-2: se verifica por comando. [verifica: true]\n- CA-3: sin prueba todavia.")
	bdEscribir(t, root, bdSpec, specBody)
	bdEscribir(t, root, "precios_test.go", "package app\n\n// CA-1: por region\n")
	bdEscribir(t, root, "otro/otro_test.go", "package otro\n\n// CA-1 tambien\n")
	bdEscribir(t, root, "z_test.txt", "CA-10 y CA-12, ninguno es el uno\n")
	bdAprobar(t, root, bdSlug)
	gates := bdGatesVerdes()
	gates[len(gates)-1].OutputTail = "ok 1\nok 2"
	v := bdVeredictoDisco(t, root, bdSpec, bdT0, false, gates)
	otroV := bdVeredictoDisco(t, root, ".hoom/specs/otra.md", bdT0.Add(time.Minute), false, bdGatesVerdes())

	alta := func(task, desc string) finding.Finding {
		f, err := finding.Register(root, "main", finding.Draft{Severity: "high", Lens: "risk", Description: desc,
			Author: "reviewer", Task: task})
		if err != nil {
			t.Fatal(err)
		}
		return f
	}
	fAbierto := alta(bdSlug, "abierto de la tarjeta")
	fResuelto := alta(bdSlug, "resuelto de la tarjeta")
	if _, err := finding.Resolve(root, fResuelto.ID, finding.StatusRefuted, "TestPrecios lo refuta", "refutador"); err != nil {
		t.Fatal(err)
	}
	fOtra := alta("otra", "de otra tarea")
	fSin := alta("", "sin tarea")
	rv := reviewcmd.Record{ID: "20260922T150405_ab12cd", CreatedAt: bdT0.Add(time.Hour), Task: bdSlug, Spec: bdSpec,
		Fingerprint: v.Git.ChangeFingerprint, VerdictID: v.ID, Verdict: "green", Lenses: reviewcmd.Lentes,
		Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{fAbierto.ID}}
	bdReviewDisco(t, root, rv)
	bdReviewDisco(t, root, reviewcmd.Record{ID: "20260922T150500_cd34ef", CreatedAt: bdT0.Add(time.Hour), Task: "otra",
		Lenses: reviewcmd.Lentes, Findings: []string{}})

	antes := bdFoto(t, root)
	d, err := DetailFor(root, "main", "high", bdSlug, now, false)
	if err != nil {
		t.Fatalf("CA-313: DetailFor de una tarjeta valida: %v", err)
	}
	dd, err := DetailFor(root, "main", "high", bdSlug, now, true)
	if err != nil {
		t.Fatalf("CA-315: DetailFor con diff: %v", err)
	}
	bdMismaFoto(t, "CA-313", antes, bdFoto(t, root))

	// card: la misma que CardFor (igualdad profunda)
	card, err := CardFor(root, "main", "high", bdSlug, now)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Card, card) {
		a, _ := json.Marshal(d.Card)
		b, _ := json.Marshal(card)
		t.Fatalf("CA-313: la card del detalle es la de CardFor:\ndetalle: %s\nCardFor: %s", a, b)
	}
	if d.Card.Evidence.Source != SourceArbol || d.Card.Column != ColTestWriter {
		t.Fatalf("CA-313: la tarjeta del fixture esta en test-writer leyendo el arbol: %s %s", d.Card.Column, d.Card.Evidence.Source)
	}

	// spec: ruta relativa al arbol de evidencia, markdown y aprobacion vigente
	if d.Spec.Path != bdSpec || d.Spec.Path != d.Card.Evidence.Spec || !d.Spec.Exists || d.Spec.Markdown != specBody {
		t.Fatalf("CA-313: spec trae ruta, exists y el markdown tal cual: %+v", d.Spec)
	}
	if d.Spec.Approval == nil || d.Spec.Approval.SHA256[:8] != tbSha8(specBody) || d.Spec.Approval.Spec != bdSpec {
		t.Fatalf("CA-313: spec trae la aprobacion vigente del contenido actual: %+v", d.Spec.Approval)
	}

	// criteria: uno por id de spec.Lint, en su orden, con texto, traza y archivos
	type crit struct {
		text, by string
		files    []string
	}
	quiere := []string{"CA-1", "CA-2", "CA-3", "CA-7"}
	porID := map[string]crit{
		"CA-1": {"el precio sale por region y se redondea.", TracedTest, []string{"otro/otro_test.go", "precios_test.go"}},
		"CA-2": {"se verifica por comando.", TracedComando, []string{}},
		"CA-3": {"sin prueba todavia.", "", []string{}},
		"CA-7": {"", "", []string{}},
	}
	var ids []string
	for _, c := range d.Criteria {
		ids = append(ids, c.ID)
		w := porID[c.ID]
		if c.Text != w.text || c.TracedBy != w.by {
			t.Fatalf("CA-313: el criterio %s trae text %q y traced_by %q, fue %q y %q", c.ID, w.text, w.by, c.Text, c.TracedBy)
		}
		if c.Files == nil || !reflect.DeepEqual(c.Files, w.files) {
			t.Fatalf("CA-313: los archivos de test de %s son %q (relativos, ordenados, nunca null), fue %#v", c.ID, w.files, c.Files)
		}
	}
	if strings.Join(ids, ",") != strings.Join(quiere, ",") {
		t.Fatalf("CA-313: los criterios en el orden de spec.Lint %v, fueron %v", quiere, ids)
	}

	// verdict: el veredicto completo de la tarjeta, con sus colas de salida
	if d.Verdict == nil || d.Verdict.ID != v.ID || d.Verdict.ID == otroV.ID {
		t.Fatalf("CA-313: verdict es el veredicto de la tarjeta (%s): %+v", v.ID, d.Verdict)
	}
	all, _, _ := verdict.LoadAllWithWarnings(root)
	for _, w := range all {
		if w.ID == v.ID {
			a, _ := json.Marshal(d.Verdict)
			b, _ := json.Marshal(w)
			if string(a) != string(b) {
				t.Fatalf("CA-313: verdict es el veredicto completo, tal cual esta en disco:\ndetalle: %s\ndisco:   %s", a, b)
			}
		}
	}
	if !strings.Contains(func() string { raw, _ := json.Marshal(d.Verdict); return string(raw) }(), `"output_tail":"ok 1\nok 2"`) {
		t.Fatalf("CA-313: el veredicto trae las colas de salida de sus gates: %+v", d.Verdict.Gates)
	}
	if fp := gitx.Snapshot(root, "main").ChangeFingerprint; d.Fingerprint != fp || fp == "" {
		t.Fatalf("CA-313: fingerprint es la huella actual del arbol de evidencia %q, fue %q", fp, d.Fingerprint)
	}

	// findings: los de la tarjeta, abiertos y resueltos, en el orden de finding.List
	lista, _, err := finding.List(root, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	var quiereF, gotF []string
	for _, f := range lista {
		if f.Task == bdSlug {
			quiereF = append(quiereF, f.ID)
		}
	}
	for _, f := range d.Findings {
		gotF = append(gotF, f.ID)
		if f.ID == fOtra.ID || f.ID == fSin.ID || f.Task != bdSlug {
			t.Fatalf("CA-313: ningun hallazgo de otra tarea ni sin tarea: %+v", f)
		}
		if f.ID == fResuelto.ID && (f.Status != finding.StatusRefuted || f.Resolution == nil || f.Resolution.Evidence != "TestPrecios lo refuta") {
			t.Fatalf("CA-313: el resuelto viene con su resolucion: %+v %+v", f, f.Resolution)
		}
		if f.ID == fAbierto.ID && f.Status != finding.StatusOpen {
			t.Fatalf("CA-313: el abierto viene abierto: %+v", f)
		}
	}
	if len(quiereF) != 2 || strings.Join(gotF, ",") != strings.Join(quiereF, ",") {
		t.Fatalf("CA-313: findings son el abierto y el resuelto de la tarjeta, en el orden de finding.List %v, fueron %v", quiereF, gotF)
	}

	// reviews: los registros de la tarjeta
	if len(d.Reviews) != 1 || d.Reviews[0].ID != rv.ID {
		t.Fatalf("CA-313: reviews son los registros de la tarjeta: %+v", d.Reviews)
	}

	// paths: relativas a root, que existen
	p := d.Paths
	sha8 := tbSha8(specBody)
	if p.Item != ".hoom/items/precios.yaml" || p.Dir != "." || p.Dir != d.Card.Evidence.Dir || p.Spec != bdSpec ||
		p.Approval != ".hoom/approvals/precios_"+sha8+".json" || p.Verdict != ".hoom/verdicts/"+v.ID+".json" {
		t.Fatalf("CA-313: las rutas de la tarjeta en el arbol actual: %+v", p)
	}
	if !reflect.DeepEqual(p.Reviews, []string{".hoom/reviews/" + rv.ID + ".json"}) {
		t.Fatalf("CA-313: paths.reviews son los registros de la tarjeta: %q", p.Reviews)
	}
	quierePF := tbOrdenadas([]string{".hoom/findings/" + fAbierto.ID + ".json", ".hoom/findings/" + fResuelto.ID + ".json",
		".hoom/findings/" + fResuelto.ID + ".res.json"})
	if !reflect.DeepEqual(tbOrdenadas(p.Findings), quierePF) {
		t.Fatalf("CA-313: paths.findings son %q, fueron %q", quierePF, p.Findings)
	}
	tbExisten(t, "CA-313", root, p)

	// sin withDiff no hay diff (ni la clave); con withDiff y sin worktree, lo dice
	m, raw := tbJSONMap(t, d)
	for _, k := range []string{"card", "spec", "criteria", "verdict", "fingerprint", "findings", "reviews", "work", "paths"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("CA-313: al detalle le falta %q: %s", k, raw)
		}
	}
	if _, ok := m["diff"]; ok || d.Diff != nil {
		t.Fatalf("CA-315: sin withDiff el detalle no trae diff: %s", raw)
	}
	if dd.Diff == nil || dd.Diff.Available || dd.Diff.Note != tbNotaSinWorktree {
		t.Fatalf("CA-315: sin worktree, available false y la nota %q: %+v", tbNotaSinWorktree, dd.Diff)
	}
	if !reflect.DeepEqual(dd.Card, d.Card) {
		t.Fatal("CA-315: pedir el diff no cambia la tarjeta")
	}
}

// CA-313: sin spec, con lint que falla y con la aprobacion invalidada; y
// DetailFor falla igual que CardFor con un item inexistente o invalido.
func TestCA313_DetalleSinSpecYErrores(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	bdItemArchivo(t, root, bdSlug, "Precios", "")

	d, err := DetailFor(root, "main", "high", bdSlug, now, false)
	if err != nil {
		t.Fatalf("CA-313: DetailFor de un item sin spec: %v", err)
	}
	if d.Spec.Exists || d.Spec.Markdown != "" || d.Spec.Approval != nil || d.Spec.Path != d.Card.Evidence.Spec {
		t.Fatalf("CA-313: sin spec, exists false, markdown vacio y sin aprobacion: %+v", d.Spec)
	}
	if d.Verdict != nil || d.Fingerprint != "" {
		t.Fatalf("CA-313: sin veredicto, verdict null y fingerprint vacio: %+v %q", d.Verdict, d.Fingerprint)
	}
	if d.Paths.Spec != "" || d.Paths.Approval != "" || d.Paths.Verdict != "" || d.Paths.Item != ".hoom/items/precios.yaml" || d.Paths.Dir != "." {
		t.Fatalf("CA-313: las rutas que no aplican van vacias: %+v", d.Paths)
	}
	tbExisten(t, "CA-313", root, d.Paths)
	m, raw := tbJSONMap(t, d)
	if c, ok := m["criteria"].([]any); !ok || len(c) != 0 {
		t.Fatalf("CA-313: sin spec, criteria es [] (no null): %s", raw)
	}
	if v, ok := m["verdict"]; !ok || v != nil {
		t.Fatalf("CA-313: sin veredicto, verdict es null: %s", raw)
	}

	// lint con issues: criteria [] aunque el spec cite ids
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo.\n- CA-2: otro.", false))
	d, err = DetailFor(root, "main", "high", bdSlug, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Criteria == nil || len(d.Criteria) != 0 || !d.Spec.Exists {
		t.Fatalf("CA-313: con issues de lint, criteria es []: %+v", d.Criteria)
	}

	// aprobacion invalidada: la aprobacion vigente no existe
	body := bdSpecTexto("- CA-1: algo.", true)
	bdEscribir(t, root, bdSpec, body)
	bdAprobar(t, root, bdSlug)
	d, err = DetailFor(root, "main", "high", bdSlug, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Spec.Approval == nil || d.Paths.Approval != ".hoom/approvals/precios_"+tbSha8(body)+".json" {
		t.Fatalf("CA-313: aprobado, spec.approval y paths.approval la nombran: %+v %q", d.Spec.Approval, d.Paths.Approval)
	}
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo distinto.", true))
	d, err = DetailFor(root, "main", "high", bdSlug, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Spec.Approval != nil || d.Paths.Approval != "" {
		t.Fatalf("CA-313: con la aprobacion invalidada no hay aprobacion vigente: %+v %q", d.Spec.Approval, d.Paths.Approval)
	}
	tbExisten(t, "CA-313", root, d.Paths)

	// errores: los de CardFor, tal cual
	for _, slug := range []string{"no-existe", "malo"} {
		if slug == "malo" {
			bdEscribir(t, root, ".hoom/items/malo.yaml", bdItemYAML("Malo", "columna: hecho\n"))
		}
		_, cerr := CardFor(root, "main", "high", slug, now)
		_, derr := DetailFor(root, "main", "high", slug, now, true)
		if cerr == nil || derr == nil || derr.Error() != cerr.Error() {
			t.Fatalf("CA-313: DetailFor(%s) falla igual que CardFor:\nCardFor:   %v\nDetailFor: %v", slug, cerr, derr)
		}
	}
}

// CA-313, CA-315: con el worktree de la tarea, las rutas son relativas a
// root (dentro del worktree), los archivos de test de cada criterio son
// relativos al arbol de evidencia, y el diff es base...HEAD del worktree sin
// lo sin commitear.
func TestCA313_DetalleDesdeElWorktree(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt := bdWorktree(t, root, bdSlug)
	specBody := bdSpecTexto("- CA-1: con test.", true)
	bdEscribir(t, wt, bdSpec, specBody)
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1\n")
	bdEscribir(t, wt, "precios.go", "package app\n\nfunc Precio() int { return 1 }\n")
	bdAprobar(t, wt, bdSlug)
	v := bdVeredictoDisco(t, wt, bdSpec, bdT0, false, bdGatesVerdes())
	f, err := finding.Register(wt, "main", finding.Draft{Severity: "low", Lens: "risk", Description: "de la tarjeta",
		Author: "reviewer", Task: bdSlug})
	if err != nil {
		t.Fatal(err)
	}
	rv := reviewcmd.Record{ID: "20260922T150405_ab12cd", CreatedAt: bdT0.Add(time.Hour), Task: bdSlug, Spec: bdSpec,
		VerdictID: v.ID, Verdict: "green", Lenses: reviewcmd.Lentes, Findings: []string{}}
	bdReviewDisco(t, wt, rv)
	bdCommitear(t, wt, "spec, test, codigo, veredicto, hallazgo y review")
	bdEscribir(t, wt, "sucio.go", "package app\n\nvar Sucio = 1\n")

	antes := bdFoto(t, root)
	d, err := DetailFor(root, "main", "high", bdSlug, now, true)
	if err != nil {
		t.Fatalf("CA-313: DetailFor con worktree: %v", err)
	}
	bdMismaFoto(t, "CA-313", antes, bdFoto(t, root))

	card, err := CardFor(root, "main", "high", bdSlug, now)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(d.Card, card) {
		t.Fatalf("CA-313: la card del detalle es la de CardFor")
	}
	if d.Card.Evidence.Source != SourceWorktree {
		t.Fatalf("CA-313: la evidencia es el worktree: %+v", d.Card.Evidence)
	}
	if d.Spec.Path != bdSpec || d.Spec.Markdown != specBody || d.Spec.Approval == nil {
		t.Fatalf("CA-313: spec.path es relativa al arbol de evidencia y trae el markdown y la aprobacion: %+v", d.Spec)
	}
	if len(d.Criteria) != 1 || d.Criteria[0].TracedBy != TracedTest || !reflect.DeepEqual(d.Criteria[0].Files, []string{"precios_test.go"}) {
		t.Fatalf("CA-313: los archivos de test son relativos al arbol de evidencia: %+v", d.Criteria)
	}
	w := bdWtDir
	p := d.Paths
	if p.Item != ".hoom/items/precios.yaml" || p.Dir != w || p.Dir != d.Card.Evidence.Dir ||
		p.Spec != w+"/"+bdSpec ||
		p.Approval != w+"/.hoom/approvals/precios_"+tbSha8(specBody)+".json" ||
		p.Verdict != w+"/.hoom/verdicts/"+v.ID+".json" ||
		!reflect.DeepEqual(p.Reviews, []string{w + "/.hoom/reviews/" + rv.ID + ".json"}) ||
		!reflect.DeepEqual(p.Findings, []string{w + "/.hoom/findings/" + f.ID + ".json"}) {
		t.Fatalf("CA-313: las rutas con worktree son relativas a root:\n%+v", p)
	}
	tbExisten(t, "CA-313", root, p)

	// el diff: base...HEAD del worktree, sin lo sin commitear
	if d.Diff == nil || !d.Diff.Available || d.Diff.Note != "" || d.Diff.Base != "main" || d.Diff.Truncated {
		t.Fatalf("CA-315: con worktree el diff esta disponible: %+v", d.Diff)
	}
	if head := bdGit(t, wt, "rev-parse", "--short=12", "HEAD"); d.Diff.Head != head {
		t.Fatalf("CA-315: head es el sha corto de HEAD del worktree %q, fue %q", head, d.Diff.Head)
	}
	var hayCodigo bool
	for _, df := range d.Diff.Files {
		if df.Path == "precios.go" {
			hayCodigo = df.Insertions == 3 && df.Deletions == 0
		}
		if df.Path == "sucio.go" {
			t.Fatalf("CA-315: lo sin commitear no entra en el diff: %+v", d.Diff.Files)
		}
	}
	if !hayCodigo || d.Diff.Insertions < 3 || !strings.Contains(d.Diff.Patch, "+func Precio() int { return 1 }") ||
		strings.Contains(d.Diff.Patch, "Sucio") {
		t.Fatalf("CA-315: el diff trae precios.go (3 inserciones) y su parche, sin sucio.go: %+v", d.Diff)
	}
}

// CA-315: un parche de mas de 256 KiB se corta en un fin de linea con
// truncated true.
func TestCA315_DiffTruncadoEnElDetalle(t *testing.T) {
	if DiffMaxBytes != 256<<10 {
		t.Fatalf("CA-315: el tope del parche es 256 KiB, es %d", DiffMaxBytes)
	}
	root := bdRepo(t, "")
	now := time.Now().UTC()
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt := bdWorktree(t, root, bdSlug)
	var b strings.Builder
	for i := 0; b.Len() < 300<<10; i++ {
		b.WriteString("una linea larga del cambio de ocho mil lineas, numero ")
		b.WriteString(strings.Repeat("y", i%23))
		b.WriteString("\n")
	}
	bdEscribir(t, wt, "grande.txt", b.String())
	bdCommitear(t, wt, "cambio grande")
	d, err := DetailFor(root, "main", "high", bdSlug, now, true)
	if err != nil {
		t.Fatal(err)
	}
	if d.Diff == nil || !d.Diff.Available || !d.Diff.Truncated {
		t.Fatalf("CA-315: el diff de mas de 256 KiB se corta y lo dice: %+v", d.Diff)
	}
	if len(d.Diff.Patch) > DiffMaxBytes || len(d.Diff.Patch) < DiffMaxBytes-200 || !strings.HasSuffix(d.Diff.Patch, "\n") {
		t.Fatalf("CA-315: el parche se corta en un fin de linea dentro de los 256 KiB: len=%d", len(d.Diff.Patch))
	}
	if len(d.Diff.Files) != 1 || d.Diff.Files[0].Path != "grande.txt" || d.Diff.Insertions != strings.Count(b.String(), "\n") {
		t.Fatalf("CA-315: el numstat va entero aunque el parche se corte: %+v %d", d.Diff.Files, d.Diff.Insertions)
	}
}

func tbCasi(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

// tbTrabajo arma la telemetria de la tarjeta "trabajo": sobres cerrados
// (uno con sidecar, uno sin), dos abiertos (vivo por latido y sin dueño),
// runs sueltos (cerrado, vivo, sin cerrar sin dueño) y un run de otra tarea.
func tbTrabajo(t *testing.T) (string, time.Time) {
	t.Helper()
	root := bdRepo(t, "")
	now := time.Now().UTC()
	const slug = "trabajo"
	bdItemArchivo(t, root, slug, "Trabajo", "")
	at := func(d time.Duration) time.Time { return now.Add(-d) }
	yo := os.Getpid()
	muerto := bdPIDMuerto(t)

	// run A (con sobre A): su sidecar manda sobre el usage del sobre
	bdRunDisco(t, root, runcmd.Meta{ID: "20260923T100000_runa01", Provider: "claude", Role: "writer", Task: slug, Dir: root,
		CreatedAt: at(90 * time.Minute), EndedAt: at(70 * time.Minute), Status: runcmd.StatusDone,
		Usage: &providers.Usage{CostUSD: bdF(0.8), InputTokens: 1000, OutputTokens: 100}})
	bdSobreDisco(t, root, envelope.Record{ID: "20260923T100000_sobrea", Role: "writer", Provider: "claude", Task: slug,
		Dir: root, RunID: "20260923T100000_runa01", Isolated: true, Stage: "ok", Step: 7, Steps: 7,
		Status: envelope.StatusDeliverable, Usage: &providers.Usage{CostUSD: bdF(0.1), InputTokens: 1, OutputTokens: 1},
		StartedAt: at(90 * time.Minute), UpdatedAt: at(70 * time.Minute), EndedAt: at(70 * time.Minute)})
	// sobre B: su run no tiene sidecar, cuenta su propio usage
	bdSobreDisco(t, root, envelope.Record{ID: "20260923T100000_sobreb", Role: "test-writer", Provider: "claude", Task: slug,
		Dir: root, RunID: "20260923T100000_perdid", Stage: "verify", Step: 5, Steps: 7,
		Status: envelope.StatusNotDeliverable, Note: "la verificacion dio rojo",
		Usage:     &providers.Usage{CostUSD: bdF(0.5), InputTokens: 10, OutputTokens: 1},
		StartedAt: at(65 * time.Minute), UpdatedAt: at(60 * time.Minute), EndedAt: at(60 * time.Minute)})
	// sobre C: abierto, sin run, latido fresco (vivo): usage null aunque traiga uno
	bdSobreDisco(t, root, envelope.Record{ID: "20260923T100000_sobrec", Role: "arquitecto", Provider: "claude", Task: slug,
		Dir: root, Stage: "spec", Step: 1, Steps: 7, Status: envelope.StatusRunning,
		Usage:     &providers.Usage{CostUSD: bdF(0.3), InputTokens: 3, OutputTokens: 3},
		StartedAt: at(5 * time.Minute), UpdatedAt: at(1 * time.Minute)})
	// sobre D: abierto, sin run, latido viejo (sin dueño)
	bdSobreDisco(t, root, envelope.Record{ID: "20260923T100000_sobred", Role: "writer", Provider: "codex", Task: slug,
		Dir: root, Stage: "verify", Step: 5, Steps: 7, Status: envelope.StatusRunning,
		StartedAt: at(50 * time.Minute), UpdatedAt: at(20 * time.Minute)})
	// runs sueltos: cerrado, vivo, y sin cerrar sin dueño; y uno de otra tarea
	bdRunDisco(t, root, runcmd.Meta{ID: "20260923T100000_runc01", Provider: "codex", Role: "reviewer", Task: slug, Dir: root,
		CreatedAt: at(45 * time.Minute), EndedAt: at(35 * time.Minute), Status: runcmd.StatusDone,
		Usage: &providers.Usage{InputTokens: 50000, OutputTokens: 2900}})
	bdRunDisco(t, root, runcmd.Meta{ID: "20260923T100000_rund01", Provider: "claude", Role: "writer", Task: slug, Dir: root,
		CreatedAt: at(3 * time.Minute), Status: runcmd.StatusRunning, ExitCode: -1, PID: yo,
		Usage: &providers.Usage{CostUSD: bdF(0.2), InputTokens: 7, OutputTokens: 7}})
	bdRunDisco(t, root, runcmd.Meta{ID: "20260923T100000_rune01", Provider: "claude", Task: slug, Dir: root,
		CreatedAt: at(30 * time.Minute), Status: runcmd.StatusRunning, ExitCode: -1, PID: muerto})
	bdRunDisco(t, root, runcmd.Meta{ID: "20260923T100000_otra01", Provider: "claude", Role: "writer", Task: "otra", Dir: root,
		CreatedAt: at(2 * time.Minute), EndedAt: at(time.Minute), Status: runcmd.StatusDone,
		Usage: &providers.Usage{CostUSD: bdF(9), InputTokens: 9, OutputTokens: 9}})

	return root, now
}

// CA-314: work trae una fila por sobre de la tarjeta y una por run que
// ningun sobre referencia, del mas nuevo al mas viejo; el usage de un sobre
// es el del sidecar de su run, el propio si el run no tiene sidecar, y null
// si el sobre no tiene run; la suma de las filas es card.spend; duration_ms
// cumple los cuatro casos y alive es el que resolvio Gather.
func TestCA314_WorkFilasUsageYDuracion(t *testing.T) {
	root, now := tbTrabajo(t)
	const slug = "trabajo"
	at := func(d time.Duration) time.Time { return now.Add(-d) }

	d, err := DetailFor(root, "main", "high", slug, now, false)
	if err != nil {
		t.Fatal(err)
	}
	type fila struct {
		kind, id string
		dur      int64 // -1 = null
		alive    bool
		status   string
	}
	m := func(x time.Duration) int64 { return x.Milliseconds() }
	quiere := []fila{
		{WorkRun, "20260923T100000_rund01", m(3 * time.Minute), true, runcmd.StatusRunning},
		{WorkSobre, "20260923T100000_sobrec", m(5 * time.Minute), true, envelope.StatusRunning},
		{WorkRun, "20260923T100000_rune01", -1, false, runcmd.StatusRunning},
		{WorkRun, "20260923T100000_runc01", m(10 * time.Minute), false, runcmd.StatusDone},
		{WorkSobre, "20260923T100000_sobred", m(30 * time.Minute), false, envelope.StatusRunning},
		{WorkSobre, "20260923T100000_sobreb", m(5 * time.Minute), false, envelope.StatusNotDeliverable},
		{WorkSobre, "20260923T100000_sobrea", m(20 * time.Minute), false, envelope.StatusDeliverable},
	}
	if len(d.Work) != len(quiere) {
		var got []string
		for _, w := range d.Work {
			got = append(got, w.Kind+":"+w.ID)
		}
		t.Fatalf("CA-314: una fila por sobre (4) y por run sin sobre (3), sin el run del sobre A ni el de otra tarea: %v", got)
	}
	for i, q := range quiere {
		w := d.Work[i]
		if w.Kind != q.kind || w.ID != q.id {
			t.Fatalf("CA-314: fila %d debe ser %s %s (del mas nuevo al mas viejo), fue %s %s", i, q.kind, q.id, w.Kind, w.ID)
		}
		if q.dur < 0 {
			if w.DurationMS != nil {
				t.Fatalf("CA-314: %s: un run sin cerrar y sin dueño tiene duration_ms null, fue %d", q.id, *w.DurationMS)
			}
		} else if w.DurationMS == nil || *w.DurationMS != q.dur {
			t.Fatalf("CA-314: %s: duration_ms debe ser %d, fue %v", q.id, q.dur, w.DurationMS)
		}
		if w.Alive != q.alive {
			t.Fatalf("CA-314: %s: alive debe ser %v, fue %v", q.id, q.alive, w.Alive)
		}
		if w.Status != q.status {
			t.Fatalf("CA-314: %s: status debe ser %q, fue %q", q.id, q.status, w.Status)
		}
	}

	// los campos del sobre A y del run C
	a := d.Work[6]
	if a.RunID != "20260923T100000_runa01" || a.Role != "writer" || a.Provider != "claude" || a.Stage != "ok" ||
		a.Step != 7 || a.Steps != 7 || !a.Isolated || !a.StartedAt.Equal(at(90*time.Minute)) ||
		a.EndedAt == nil || !a.EndedAt.Equal(at(70*time.Minute)) {
		t.Fatalf("CA-314: la fila del sobre trae run_id, rol, provider, paso, aislado y fechas: %+v", a)
	}
	if b := d.Work[5]; b.Note != "la verificacion dio rojo" || b.Role != "test-writer" {
		t.Fatalf("CA-314: la fila del sobre trae su nota: %+v", b)
	}
	if c := d.Work[3]; c.Role != "reviewer" || c.Provider != "codex" || !c.StartedAt.Equal(at(45*time.Minute)) {
		t.Fatalf("CA-314: la fila del run trae rol, provider y started_at (su created_at): %+v", c)
	}

	// usage: sidecar del run, el propio sin sidecar, null sin run
	usage := map[string]*providers.Usage{}
	for _, w := range d.Work {
		usage[w.ID] = w.Usage
	}
	if u := usage["20260923T100000_sobrea"]; u == nil || u.CostUSD == nil || !tbCasi(*u.CostUSD, 0.8) || u.InputTokens != 1000 {
		t.Fatalf("CA-314: el usage del sobre A es el del sidecar de su run (0.8, 1000): %+v", u)
	}
	if u := usage["20260923T100000_sobreb"]; u == nil || u.CostUSD == nil || !tbCasi(*u.CostUSD, 0.5) || u.InputTokens != 10 {
		t.Fatalf("CA-314: el usage del sobre B (run sin sidecar) es el propio (0.5, 10): %+v", u)
	}
	for _, id := range []string{"20260923T100000_sobrec", "20260923T100000_sobred", "20260923T100000_rune01"} {
		if usage[id] != nil {
			t.Fatalf("CA-314: %s no tiene usage (sin run, o run sin usage): %+v", id, usage[id])
		}
	}

	// la suma de las filas es card.spend
	var costo float64
	hayCosto := false
	in, out := 0, 0
	for _, w := range d.Work {
		if w.Usage == nil {
			continue
		}
		if w.Usage.CostUSD != nil {
			costo += *w.Usage.CostUSD
			hayCosto = true
		}
		in += w.Usage.InputTokens
		out += w.Usage.OutputTokens
	}
	s := d.Card.Spend
	if !hayCosto || s.CostUSD == nil || !tbCasi(costo, *s.CostUSD) || !tbCasi(costo, 1.5) {
		t.Fatalf("CA-314: la suma del costo de las filas (%v) es card.spend.cost_usd (%v) = 1.5", costo, s.CostUSD)
	}
	if in != s.InputTokens || out != s.OutputTokens || in != 51017 || out != 3008 {
		t.Fatalf("CA-314: la suma de tokens de las filas (%d/%d) es la de card.spend (%d/%d)", in, out, s.InputTokens, s.OutputTokens)
	}

	// alive es el que resolvio Gather
	ev := Gather(root, "main", "high", bdItem(slug), now)
	vivo := map[string]bool{}
	for _, e := range ev.Envelopes {
		vivo[e.Record.ID] = e.Alive
	}
	for _, r := range ev.Runs {
		vivo[r.Meta.ID] = r.Alive
	}
	for _, w := range d.Work {
		if g, ok := vivo[w.ID]; !ok || g != w.Alive {
			t.Fatalf("CA-314: alive de %s es el de Gather (%v), fue %v", w.ID, g, w.Alive)
		}
	}

	// en JSON: duration_ms y usage null cuando no hay dato
	raw, _ := json.Marshal(d.Work)
	var filas []map[string]any
	if err := json.Unmarshal(raw, &filas); err != nil {
		t.Fatal(err)
	}
	for _, f := range filas {
		if f["id"] == "20260923T100000_rune01" {
			if v, ok := f["duration_ms"]; !ok || v != nil {
				t.Fatalf("CA-314: duration_ms null en JSON: %v", f)
			}
		}
		if f["id"] == "20260923T100000_sobrec" {
			if v, ok := f["usage"]; !ok || v != nil {
				t.Fatalf("CA-314: usage null en JSON: %v", f)
			}
		}
	}
}

// CA-314: si ninguna fila trae costo, card.spend.cost_usd es null (no 0).
func TestCA314_WorkSinCosto(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	const slug = "sin-costo"
	bdItemArchivo(t, root, slug, "Sin costo", "")
	for i, id := range []string{"20260923T100000_codex1", "20260923T100000_codex2"} {
		bdRunDisco(t, root, runcmd.Meta{ID: id, Provider: "codex", Role: "writer", Task: slug, Dir: root,
			CreatedAt: now.Add(-time.Duration(20+i) * time.Minute), EndedAt: now.Add(-time.Duration(10+i) * time.Minute),
			Status: runcmd.StatusDone, Usage: &providers.Usage{InputTokens: 100, OutputTokens: 10}})
	}
	d, err := DetailFor(root, "main", "high", slug, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Work) != 2 || d.Work[0].ID != "20260923T100000_codex1" || d.Work[1].ID != "20260923T100000_codex2" {
		t.Fatalf("CA-314: dos filas de run, la mas nueva primero: %+v", d.Work)
	}
	for _, w := range d.Work {
		if w.Kind != WorkRun || w.Usage == nil || w.Usage.CostUSD != nil || w.DurationMS == nil || *w.DurationMS != (10*time.Minute).Milliseconds() {
			t.Fatalf("CA-314: fila de run sin costo, con tokens y cerrada (10 min): %+v", w)
		}
	}
	if d.Card.Spend.CostUSD != nil || d.Card.Spend.InputTokens != 200 {
		t.Fatalf("CA-314: sin costo en ninguna fila, card.spend.cost_usd es null: %+v", d.Card.Spend)
	}
}
