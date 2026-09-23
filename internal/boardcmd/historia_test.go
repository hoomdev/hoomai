// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md,
// seccion 1 "La historia de la tarjeta" (CA-359..CA-364): boardcmd.TimelineFor
// sobre repos git reales en t.TempDir(), con fechas de autor fijas (y una
// fecha de commit distinta y fija, para que leer la fecha equivocada se
// note), y la telemetria escrita a mano en .hoom/envelopes y .hoom/runs. Sin
// ningun CLI de IA. Todo identificador de paquete de estos archivos lleva el
// prefijo hi: otros test-writers escriben en paralelo en el mismo paquete.
//
// Este archivo tiene los helpers compartidos, la fuente git por archivo
// (CA-359) y las reglas de plain (CA-364). Los commits de la tarea y el
// medidor estan en historia_git_test.go; la telemetria y las delegaciones en
// historia_telemetria_test.go.
package boardcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	hiHenry    = "Henry Orellana <henry@hoom.dev>"
	hiYo       = "hoom test <test@hoom.dev>" // la identidad que configura bdRepo
	hiAgente   = "Agente Writer <writer@hoom.dev>"
	hiFirmante = "Firmante Real <firma@hoom.dev>"
	hiWtp      = bdWtDir + "/" // prefijo de lo leido en el espacio de trabajo
)

var (
	// hiAhora es el now de todas las lecturas: la historia no depende del reloj.
	hiAhora = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	// hiCommitter es la fecha de commit de TODOS los commits: la historia usa
	// la fecha de autor, y con esta una implementacion que lea %cI ve todos
	// los commits en el mismo instante.
	hiCommitter = time.Date(2026, 9, 23, 6, 0, 0, 0, time.UTC)

	hiProhibidas = regexp.MustCompile(`(?i)\b(git|commit|worktree|merge|rama|branch|diff|head|huella)\b`)
	hiSha        = regexp.MustCompile(`^[0-9a-f]{40}$`)

	// hiOrdenKind es el orden de desempate de kind del contrato.
	hiOrdenKind = map[string]int{KindItem: 0, KindSpec: 1, KindAprobacion: 2, KindCommit: 3, KindVeredicto: 4,
		KindHallazgo: 5, KindResolucion: 6, KindReview: 7, KindSobre: 8, KindRun: 9, KindSubagente: 10, KindSubagenteFin: 11}
	hiKindsGit = map[string]bool{KindItem: true, KindSpec: true, KindAprobacion: true, KindCommit: true,
		KindVeredicto: true, KindHallazgo: true, KindResolucion: true, KindReview: true}
	hiKindsTel = map[string]bool{KindSobre: true, KindRun: true, KindSubagente: true, KindSubagenteFin: true}

	hiSpecV1 = bdSpecTexto("- CA-1: el precio sale por region.\n- CA-2: la vista muestra el precio.\n- CA-3: el redondeo es a dos decimales.", true)
	hiSpecV2 = bdSpecTexto("- CA-1: el precio sale por region y por moneda.\n- CA-2: la vista muestra el precio.\n- CA-3: el redondeo es a dos decimales.", true)
)

// hiGitEnv corre git en dir con variables de entorno extra.
func hiGitEnv(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// hiFechas: fecha de autor at (en su zona) y la fecha de commit fija.
func hiFechas(at time.Time) []string {
	f := func(x time.Time) string { return fmt.Sprintf("%d %s", x.Unix(), x.Format("-0700")) }
	return []string{"GIT_AUTHOR_DATE=" + f(at), "GIT_COMMITTER_DATE=" + f(hiCommitter)}
}

// hiCommit commitea todo lo de dir con fecha de autor at y, si se pide, otro
// autor ("Nombre <correo>"; el committer sigue siendo hoom test). Devuelve el
// sha completo.
func hiCommit(t *testing.T, dir, asunto string, at time.Time, autor string) string {
	t.Helper()
	bdGit(t, dir, "add", "-A")
	env := hiFechas(at)
	if autor != "" {
		i := strings.Index(autor, " <")
		env = append(env, "GIT_AUTHOR_NAME="+autor[:i], "GIT_AUTHOR_EMAIL="+strings.TrimSuffix(autor[i+2:], ">"))
	}
	hiGitEnv(t, dir, env, "commit", "-q", "-m", asunto)
	return bdGit(t, dir, "rev-parse", "HEAD")
}

// hiAprobar escribe la aprobacion del contenido ACTUAL del spec de slug en dir
// como la escribe approval.Approve, pero con un firmante elegido (distinto del
// autor del commit que la guarda). Devuelve su ruta relativa a dir.
func hiAprobar(t *testing.T, dir, slug, firmante string, at time.Time) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", "specs", slug+".md"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	h := hex.EncodeToString(sum[:])
	rec := approval.Record{Spec: ".hoom/specs/" + slug + ".md", SHA256: h, ApprovedBy: firmante, ApprovedAt: at}
	body, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	rel := ".hoom/approvals/" + slug + "_" + h[:8] + ".json"
	bdEscribir(t, dir, rel, string(body))
	if st, _, err := approval.Status(dir, filepath.Join(dir, ".hoom", "specs", slug+".md")); err != nil || st != approval.StatusApproved {
		t.Fatalf("el fixture: la aprobacion escrita a mano vale para approval.Status (%s, %v)", st, err)
	}
	return rel
}

// hiVeredicto escribe un veredicto real con el tamano de cambio pedido.
func hiVeredicto(t *testing.T, dir, spec string, at time.Time, partial bool, lineas int, gates []verdict.GateResult) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: at, Partial: partial, Spec: spec,
		Git:   gitx.Info{IsRepo: true, ChangeFingerprint: "huella-del-fixture", Insertions: lineas},
		Gates: gates}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	return v
}

// hiGates arma gates requeridos con pares nombre, estado.
func hiGates(pares ...string) []verdict.GateResult {
	var out []verdict.GateResult
	for i := 0; i+1 < len(pares); i += 2 {
		out = append(out, verdict.GateResult{Name: pares[i], Required: true, Status: pares[i+1]})
	}
	return out
}

func hiHallazgo(t *testing.T, dir, sev, task string) string {
	t.Helper()
	f, err := finding.Register(dir, "main", finding.Draft{Severity: sev, Lens: "risk",
		Description: "hallazgo de " + task, Author: "reviewer", Task: task})
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}

func hiResolver(t *testing.T, dir, id, as string) {
	t.Helper()
	if _, err := finding.Resolve(dir, id, as, "la prueba TestPrecios lo muestra", "writer"); err != nil {
		t.Fatal(err)
	}
}

func hiReview(t *testing.T, dir, id, task string, lentes []string) string {
	t.Helper()
	bdReviewDisco(t, dir, reviewcmd.Record{ID: id, CreatedAt: hiAhora.Add(-time.Hour), Task: task,
		Spec: ".hoom/specs/" + task + ".md", Fingerprint: "h", VerdictID: "v", Verdict: "green",
		Lenses: lentes, Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}})
	return id
}

// hiEventos escribe la narracion de un run (.hoom/runs/<id>.jsonl) en dir.
func hiEventos(t *testing.T, dir, runID string, evs ...providers.Event) string {
	t.Helper()
	var b strings.Builder
	for _, e := range evs {
		raw, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(raw)
		b.WriteByte('\n')
	}
	return bdEscribir(t, dir, filepath.Join(".hoom", "runs", runID+".jsonl"), b.String())
}

func hiCasi(a, b float64) bool { return math.Abs(a-b) < 1e-9 }

func hiCorto(ref string) string {
	if hiSha.MatchString(ref) {
		return ref[:12]
	}
	return ref
}

// hiListado muestra la historia para los mensajes de error.
func hiListado(tl Timeline) string {
	var b strings.Builder
	for _, e := range tl.Entries {
		fmt.Fprintf(&b, "  %s %-10s %-13s %-22s %q %q %v\n", e.At.UTC().Format(time.RFC3339), e.Source, e.Kind,
			hiCorto(e.Ref), e.Artifact, e.Plain, e.Meter)
	}
	return b.String()
}

// hiEnOrden dice si a puede ir antes que b: at ascendente; a igual at, git
// antes que telemetria; despues el orden de kind; despues ref.
func hiEnOrden(a, b TimelineEntry) bool {
	if !a.At.Equal(b.At) {
		return a.At.Before(b.At)
	}
	if a.Source != b.Source {
		return a.Source == SourceGit
	}
	if a.Kind != b.Kind {
		return hiOrdenKind[a.Kind] < hiOrdenKind[b.Kind]
	}
	return a.Ref <= b.Ref
}

// hiPlainOK: el plain de una entrada sigue las reglas de C2 (CA-364).
func hiPlainOK(t *testing.T, e TimelineEntry) {
	t.Helper()
	if strings.TrimSpace(e.Plain) == "" {
		t.Fatalf("CA-364: el plain de la entrada %s %s nunca es vacio: %+v", e.Kind, hiCorto(e.Ref), e)
	}
	if w := hiProhibidas.FindString(e.Plain); w != "" {
		t.Fatalf("CA-364: el plain de la entrada %s %s no dice %q: %q", e.Kind, hiCorto(e.Ref), w, e.Plain)
	}
	for _, sub := range []string{".hoom/", "hoom "} {
		if strings.Contains(e.Plain, sub) {
			t.Fatalf("CA-364: el plain de la entrada %s %s no contiene %q: %q", e.Kind, hiCorto(e.Ref), sub, e.Plain)
		}
	}
}

// hiBienFormada exige lo que vale para toda historia: ninguna lista null en
// el JSON, el orden y su desempate, el vocabulario de fuentes, kinds y
// estados, los campos fijos de las entradas de git, y las reglas de plain.
func hiBienFormada(t *testing.T, ca string, tl Timeline) {
	t.Helper()
	raw, err := json.Marshal(tl)
	if err != nil {
		t.Fatalf("%s: la historia se serializa: %v", ca, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: la historia es JSON: %v", ca, err)
	}
	for _, k := range []string{"entries", "notes", "meter"} {
		if _, ok := m[k].([]any); !ok {
			t.Fatalf("%s: %q es una lista, nunca null: %s", ca, k, raw)
		}
	}
	if _, ok := m["telemetry"].(map[string]any); !ok {
		t.Fatalf("%s: telemetry es un objeto {sobres, runs}: %s", ca, raw)
	}
	if _, ok := m["card"].(map[string]any); !ok {
		t.Fatalf("%s: card es la tarjeta de hoy: %s", ca, raw)
	}
	for i, e := range m["entries"].([]any) {
		em, _ := e.(map[string]any)
		if _, ok := em["meter"].([]any); !ok {
			t.Fatalf("%s: el meter de la entrada %d es una lista, nunca null: %v", ca, i, em)
		}
		for _, k := range []string{"at", "ended_at", "source", "kind", "who", "role", "provider", "artifact",
			"ref", "summary", "plain", "cost_usd", "tokens", "piloto"} {
			if _, ok := em[k]; !ok {
				t.Fatalf("%s: la entrada %d trae la clave %q: %v", ca, i, k, em)
			}
		}
	}

	for i, e := range tl.Entries {
		if i > 0 && !hiEnOrden(tl.Entries[i-1], e) {
			t.Fatalf("%s: entries va en orden ascendente por at (desempate: git antes que telemetria, el orden de kind, ref); la %d no puede ir despues de la %d:\n%s",
				ca, i, i-1, hiListado(tl))
		}
		switch e.Source {
		case SourceGit:
			if !hiKindsGit[e.Kind] {
				t.Fatalf("%s: kind %q no es de la fuente git: %+v", ca, e.Kind, e)
			}
			if !hiSha.MatchString(e.Ref) {
				t.Fatalf("%s: el ref de una entrada de git es el sha completo: %q", ca, e.Ref)
			}
			if e.CostUSD != nil || e.Tokens != 0 {
				t.Fatalf("%s: una entrada de git no tiene costo (cost_usd null, tokens 0): %+v", ca, e)
			}
			if e.EndedAt != nil || e.Role != "" || e.Provider != "" || e.Piloto {
				t.Fatalf("%s: una entrada de git no trae ended_at, role, provider ni piloto: %+v", ca, e)
			}
			if strings.TrimSpace(e.Who) == "" {
				t.Fatalf("%s: who de una entrada de git es el autor del commit: %+v", ca, e)
			}
			if e.Kind == KindCommit && e.Artifact != "" {
				t.Fatalf("%s: un commit de la tarea tiene artifact \"\": %+v", ca, e)
			}
			if e.Kind != KindCommit && !strings.HasPrefix(e.Artifact, ".hoom/") {
				t.Fatalf("%s: el artifact es relativo a la raiz del proyecto: %q", ca, e.Artifact)
			}
		case SourceTelemetria:
			if !hiKindsTel[e.Kind] {
				t.Fatalf("%s: kind %q no es de la fuente telemetria: %+v", ca, e.Kind, e)
			}
			if strings.TrimSpace(e.Who) == "" || !strings.HasPrefix(e.Artifact, ".hoom/") || e.Ref == "" {
				t.Fatalf("%s: una entrada de telemetria trae who, ref y artifact relativo a la raiz: %+v", ca, e)
			}
		default:
			t.Fatalf("%s: source es git o telemetria, fue %q", ca, e.Source)
		}
		for _, ef := range e.Meter {
			if ef.ID == "" || (ef.State != SegHecho && ef.State != SegFalta && ef.State != SegNoAplica) {
				t.Fatalf("CA-363: cada efecto trae id y un estado hecho|falta|no-aplica: %+v en %s %s", ef, e.Kind, hiCorto(e.Ref))
			}
		}
		hiPlainOK(t, e)
	}
	if tl.Telemetry.Sobres < 0 || tl.Telemetry.Runs < 0 {
		t.Fatalf("%s: telemetry cuenta: %+v", ca, tl.Telemetry)
	}
}

// hiTimeline lee la historia de "precios" y exige lo general: bien formada,
// card igual a la de CardFor y meter igual al esqueleto de card.meter.
func hiTimeline(t *testing.T, ca, root string) Timeline {
	t.Helper()
	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("%s: TimelineFor(%s): %v", ca, bdSlug, err)
	}
	if tl.Slug != bdSlug {
		t.Fatalf("%s: slug es el de la tarjeta: %q", ca, tl.Slug)
	}
	hiBienFormada(t, ca, tl)
	card, err := CardFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("CA-363: CardFor(%s): %v", bdSlug, err)
	}
	a, _ := json.Marshal(tl.Card)
	b, _ := json.Marshal(card)
	if string(a) != string(b) {
		t.Fatalf("CA-363: card es la tarjeta de CardFor:\nhistoria: %s\nCardFor:  %s", a, b)
	}
	if len(tl.Meter) != len(card.Meter) {
		t.Fatalf("CA-363: meter es el esqueleto de card.meter (%d segmentos), fue %+v", len(card.Meter), tl.Meter)
	}
	for i, s := range card.Meter {
		if tl.Meter[i].ID != s.ID || tl.Meter[i].Label != s.Label {
			t.Fatalf("CA-363: meter[%d] es {%s, %s} de card.meter, en su orden; fue %+v", i, s.ID, s.Label, tl.Meter[i])
		}
	}
	return tl
}

// hiEsp es una entrada esperada, por su identidad.
type hiEsp struct{ source, kind, ref, artifact string }

// hiSecuencia exige las entradas exactas, en orden.
func hiSecuencia(t *testing.T, ca string, tl Timeline, quiere []hiEsp) {
	t.Helper()
	var got, want []string
	for _, e := range tl.Entries {
		got = append(got, fmt.Sprintf("%s %s %s %q", e.Source, e.Kind, e.Ref, e.Artifact))
	}
	for _, q := range quiere {
		want = append(want, fmt.Sprintf("%s %s %s %q", q.source, q.kind, q.ref, q.artifact))
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: las entradas de la historia y su orden:\nquiere:\n  %s\nfue:\n  %s\n%s", ca,
			strings.Join(want, "\n  "), strings.Join(got, "\n  "), hiListado(tl))
	}
}

// hiUna busca LA entrada con ese kind, ref y artifact.
func hiUna(t *testing.T, ca string, tl Timeline, kind, ref, artifact string) TimelineEntry {
	t.Helper()
	var hits []TimelineEntry
	for _, e := range tl.Entries {
		if e.Kind == kind && e.Ref == ref && e.Artifact == artifact {
			hits = append(hits, e)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("%s: se esperaba una entrada %s ref=%s artifact=%q, hubo %d:\n%s", ca, kind, hiCorto(ref), artifact, len(hits), hiListado(tl))
	}
	return hits[0]
}

// hiDeKind son las entradas de un kind, en orden.
func hiDeKind(tl Timeline, kind string) []TimelineEntry {
	var out []TimelineEntry
	for _, e := range tl.Entries {
		if e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// hiEfectos exige los efectos exactos de una entrada sobre el medidor, sin
// importar su orden y sin ids repetidos.
func hiEfectos(t *testing.T, ca, que string, e TimelineEntry, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, m := range e.Meter {
		if _, dup := got[m.ID]; dup {
			t.Fatalf("%s: %s: la entrada repite el segmento %s: %+v", ca, que, m.ID, e.Meter)
		}
		got[m.ID] = m.State
	}
	ok := len(got) == len(want)
	for id, st := range want {
		if got[id] != st {
			ok = false
		}
	}
	if !ok {
		t.Fatalf("%s: %s (%s %s): los efectos sobre el medidor deben ser %v, fueron %v", ca, que, e.Kind, hiCorto(e.Ref), want, got)
	}
}

func hiTieneNota(tl Timeline, nota string) bool {
	for _, n := range tl.Notes {
		if n == nota {
			return true
		}
	}
	return false
}

// hiNotasSon exige las notas exactas, sin importar su orden.
func hiNotasSon(t *testing.T, ca string, tl Timeline, want ...string) {
	t.Helper()
	got := append([]string{}, tl.Notes...)
	w := append([]string{}, want...)
	sort.Strings(got)
	sort.Strings(w)
	if len(got) != len(w) || (len(w) > 0 && !reflect.DeepEqual(got, w)) {
		t.Fatalf("%s: las notas deben ser %q, fueron %q", ca, want, tl.Notes)
	}
}

// hiResumen: el summary de una entrada de git por archivo nombra el sha de 12,
// la letra (palabra entera) y el asunto del commit.
func hiResumen(t *testing.T, ca string, e TimelineEntry, asunto, letra string) {
	t.Helper()
	if !strings.Contains(e.Summary, e.Ref[:12]) || !strings.Contains(e.Summary, asunto) {
		t.Fatalf("%s: el summary de %s nombra el sha de 12 (%s) y el asunto (%q): %q", ca, e.Kind, e.Ref[:12], asunto, e.Summary)
	}
	if letra != "" && !regexp.MustCompile(`\b`+letra+`\b`).MatchString(e.Summary) {
		t.Fatalf("%s: el summary de %s nombra la letra %s: %q", ca, e.Kind, letra, e.Summary)
	}
}

// CA-359: una entrada de git por cada par (commit, archivo) de la tarjeta,
// con su kind, su plain segun la letra, who = el AUTOR del commit (no quien
// lo commiteo), ref = el sha completo, artifact relativo a la raiz (con el
// prefijo del espacio de trabajo en lo que se lee ahi) y sin costo. El item
// se lee en la raiz y lo demas en el espacio de trabajo: la copia del item en
// el espacio de trabajo, un spec commiteado en la raiz y los archivos de otras
// tarjetas en los mismos commits no aparecen. El orden es por fecha de autor
// (una en otra zona horaria, para que ordenar el texto no alcance) con el
// desempate del contrato. Y leer la historia no crea ni modifica nada.
func TestCA359_EntradasDeGitPorArchivoYOrden(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	h := func(n float64) time.Time { return d0.Add(time.Duration(n * float64(time.Hour))) }

	// raiz: el item de la tarjeta y el de otra tarjeta
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdItemArchivo(t, root, "otra", "Otra", "")
	r1 := hiCommit(t, root, "alta del item de precios", h(0), hiHenry)
	wt := bdWorktree(t, root, bdSlug)

	// espacio de trabajo: cada evidencia en su commit, con evidencia ajena al lado
	bdEscribir(t, wt, bdSpec, hiSpecV1)
	bdEscribir(t, wt, ".hoom/specs/otra.md", bdSpecTexto("- CA-1: otra cosa.", true))
	w1 := hiCommit(t, wt, "spec de precios", h(1), "")
	aprob := hiAprobar(t, wt, bdSlug, hiFirmante, h(2))
	hiAprobar(t, wt, "otra", hiFirmante, h(2))
	w2 := hiCommit(t, wt, "aprobacion del spec", h(2), hiHenry)
	v := hiVeredicto(t, wt, bdSpec, h(2.4), false, 30, bdGatesVerdes())
	hiVeredicto(t, wt, ".hoom/specs/otra.md", h(2.41), false, 30, bdGatesVerdes())
	hiVeredicto(t, wt, "", h(2.42), false, 30, bdGatesVerdes())
	bdEscribir(t, wt, "precios.go", "package app\n")
	// 14:30+02:00 son las 12:30Z: despues de w2 (12:00Z) y antes de w4
	// (13:00Z), aunque su texto ISO ordene despues
	en1230 := time.Date(2026, 9, 20, 14, 30, 0, 0, time.FixedZone("", 2*3600))
	w3 := hiCommit(t, wt, "veredicto de precios", en1230, "")
	f := hiHallazgo(t, wt, "high", bdSlug)
	hiHallazgo(t, wt, "high", "otra")
	hiHallazgo(t, wt, "low", "")
	w4 := hiCommit(t, wt, "hallazgos de la revision", h(3), "")
	hiResolver(t, wt, f, finding.StatusRefuted)
	w5 := hiCommit(t, wt, "resolucion del hallazgo", h(3), "") // la misma fecha de autor que w4
	rv4 := hiReview(t, wt, "20260920T140000_aaaaaa", bdSlug, reviewcmd.Lentes)
	hiReview(t, wt, "20260920T140000_bbbbbb", "otra", reviewcmd.Lentes)
	w6 := hiCommit(t, wt, "review de 4 lentes", h(4), hiAgente)
	rv1 := hiReview(t, wt, "20260920T150000_cccccc", bdSlug, []string{"risk"})
	w7 := hiCommit(t, wt, "review de una lente", h(5), "")
	bdEscribir(t, wt, ".hoom/items/precios.yaml", bdItemYAML("Copia del espacio de trabajo", ""))
	bdEscribir(t, wt, "app.go", "package app // cambio\n")
	w8 := hiCommit(t, wt, "copia del item y codigo", h(6), "")

	// raiz: un spec commiteado en la raiz no es la evidencia (lo es el espacio de trabajo)
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-9: en la raiz.", true))
	hiCommit(t, root, "spec suelto en la raiz", h(6.5), "")
	bdItemArchivo(t, root, bdSlug, "Precios por region", "")
	r2 := hiCommit(t, root, "titulo del item", h(7), "")

	// telemetria con el mismo at que r2: va despues de git, sobre antes que run
	const sobre, run = "20260920T170000_sobre1", "20260920T170000_run001"
	bdSobreDisco(t, root, envelope.Record{ID: sobre, Role: "writer", Provider: "claude", Task: bdSlug, Dir: wt,
		Stage: "ok", Step: 7, Steps: 7, Status: envelope.StatusDeliverable,
		StartedAt: h(7), UpdatedAt: h(7.5), EndedAt: h(7.5)})
	bdRunDisco(t, root, runcmd.Meta{ID: run, Provider: "codex", Role: "reviewer", Task: bdSlug, Dir: wt,
		CreatedAt: h(7), EndedAt: h(7.2), Status: runcmd.StatusDone})
	hiEventos(t, root, run, providers.Event{TS: h(7), Kind: "start"})

	antes := bdFoto(t, root)
	tl := hiTimeline(t, "CA-359", root)
	bdMismaFoto(t, "CA-359", antes, bdFoto(t, root))

	w45 := []string{w4, w5}
	sort.Strings(w45)
	hiSecuencia(t, "CA-359", tl, []hiEsp{
		{SourceGit, KindItem, r1, ".hoom/items/precios.yaml"},
		{SourceGit, KindSpec, w1, hiWtp + bdSpec},
		{SourceGit, KindCommit, w1, ""},
		{SourceGit, KindAprobacion, w2, hiWtp + aprob},
		{SourceGit, KindCommit, w2, ""},
		{SourceGit, KindCommit, w3, ""},
		{SourceGit, KindVeredicto, w3, hiWtp + ".hoom/verdicts/" + v.ID + ".json"},
		{SourceGit, KindCommit, w45[0], ""},
		{SourceGit, KindCommit, w45[1], ""},
		{SourceGit, KindHallazgo, w4, hiWtp + ".hoom/findings/" + f + ".json"},
		{SourceGit, KindResolucion, w5, hiWtp + ".hoom/findings/" + f + ".res.json"},
		{SourceGit, KindCommit, w6, ""},
		{SourceGit, KindReview, w6, hiWtp + ".hoom/reviews/" + rv4 + ".json"},
		{SourceGit, KindCommit, w7, ""},
		{SourceGit, KindReview, w7, hiWtp + ".hoom/reviews/" + rv1 + ".json"},
		{SourceGit, KindCommit, w8, ""},
		{SourceGit, KindItem, r2, ".hoom/items/precios.yaml"},
		{SourceTelemetria, KindSobre, sobre, ".hoom/envelopes/" + sobre + ".json"},
		{SourceTelemetria, KindRun, run, ".hoom/runs/" + run + ".jsonl"},
	})

	type det struct {
		kind, ref, artifact, who, plain, asunto, letra string
		at                                             time.Time
	}
	for _, d := range []det{
		{KindItem, r1, ".hoom/items/precios.yaml", hiHenry, "se creo la tarjeta", "alta del item de precios", "A", h(0)},
		{KindSpec, w1, hiWtp + bdSpec, hiYo, "se escribio el spec", "spec de precios", "A", h(1)},
		{KindCommit, w1, "", hiYo, "se guardaron cambios en 2 archivos", "spec de precios", "", h(1)},
		{KindAprobacion, w2, hiWtp + aprob, hiHenry, "se aprobo el spec", "aprobacion del spec", "A", h(2)},
		{KindCommit, w2, "", hiHenry, "se guardaron cambios en 2 archivos", "aprobacion del spec", "", h(2)},
		{KindVeredicto, w3, hiWtp + ".hoom/verdicts/" + v.ID + ".json", hiYo, "la verificacion dio verde", "veredicto de precios", "A", en1230},
		{KindCommit, w3, "", hiYo, "se guardaron cambios en 4 archivos", "veredicto de precios", "", en1230},
		{KindHallazgo, w4, hiWtp + ".hoom/findings/" + f + ".json", hiYo, "la revision encontro un problema (high)", "hallazgos de la revision", "A", h(3)},
		{KindCommit, w4, "", hiYo, "se guardaron cambios en 3 archivos", "hallazgos de la revision", "", h(3)},
		{KindResolucion, w5, hiWtp + ".hoom/findings/" + f + ".res.json", hiYo, "se resolvio un problema de la revision (refutado)", "resolucion del hallazgo", "A", h(3)},
		{KindCommit, w5, "", hiYo, "se guardaron cambios en 1 archivo", "resolucion del hallazgo", "", h(3)},
		{KindReview, w6, hiWtp + ".hoom/reviews/" + rv4 + ".json", hiAgente, "quedo registrada la revision de 4 lentes", "review de 4 lentes", "A", h(4)},
		{KindCommit, w6, "", hiAgente, "se guardaron cambios en 2 archivos", "review de 4 lentes", "", h(4)},
		{KindReview, w7, hiWtp + ".hoom/reviews/" + rv1 + ".json", hiYo, "quedo registrada una revision", "review de una lente", "A", h(5)},
		{KindCommit, w7, "", hiYo, "se guardaron cambios en 1 archivo", "review de una lente", "", h(5)},
		{KindCommit, w8, "", hiYo, "se guardaron cambios en 2 archivos", "copia del item y codigo", "", h(6)},
		{KindItem, r2, ".hoom/items/precios.yaml", hiYo, "cambio la tarjeta", "titulo del item", "M", h(7)},
	} {
		e := hiUna(t, "CA-359", tl, d.kind, d.ref, d.artifact)
		if e.Source != SourceGit || e.Who != d.who || e.Plain != d.plain || !e.At.Equal(d.at) {
			t.Fatalf("CA-359: la entrada %s de %s debe ser git, who %q, plain %q, at %s (fecha de autor); fue %s, %q, %q, %s",
				d.kind, hiCorto(d.ref), d.who, d.plain, d.at.UTC().Format(time.RFC3339), e.Source, e.Who, e.Plain, e.At.UTC().Format(time.RFC3339))
		}
		if e.CostUSD != nil {
			t.Fatalf("CA-359: toda entrada de git tiene cost_usd null: %+v", e)
		}
		if d.kind == KindCommit {
			if !strings.Contains(e.Summary, d.asunto) {
				t.Fatalf("CA-360: el summary de un commit de la tarea trae su asunto (%q): %q", d.asunto, e.Summary)
			}
			continue
		}
		hiResumen(t, "CA-359", e, d.asunto, d.letra)
	}
	if e := hiUna(t, "CA-359", tl, KindAprobacion, w2, hiWtp+aprob); !strings.Contains(e.Summary, hiFirmante) {
		t.Fatalf("CA-359: el summary de la aprobacion nombra a quien firmo segun el archivo de hoy (%s), no solo al autor del commit: %q", hiFirmante, e.Summary)
	}
	for _, e := range hiDeKind(tl, KindSobre) {
		if !e.At.Equal(h(7)) {
			t.Fatalf("CA-359: el sobre del fixture empieza a las 17:00Z: %+v", e)
		}
	}
	if tl.Telemetry != (TelemetryCount{Sobres: 1, Runs: 1}) {
		t.Fatalf("CA-361: telemetry cuenta 1 sobre y 1 run: %+v", tl.Telemetry)
	}
	// con espacio de trabajo, con telemetria y con git: ninguna nota aplica
	hiNotasSon(t, "CA-359", tl)

	// en JSON: cost_usd y ended_at de git son null, no 0 ni ausentes
	raw, _ := json.Marshal(tl.Entries[0])
	if !strings.Contains(string(raw), `"cost_usd":null`) || !strings.Contains(string(raw), `"ended_at":null`) {
		t.Fatalf("CA-359: una entrada de git lleva cost_usd y ended_at null en JSON: %s", raw)
	}
}

// CA-359: el plain del item y del spec sigue la letra de --name-status (A, M
// y D, tambien un item que se borra y vuelve), el artifact no lleva prefijo
// cuando la evidencia es la raiz, y sin espacio de trabajo ni telemetria las
// notas lo dicen. El replay (CA-363): el item no toca el medidor y un spec
// sin aprobacion o borrado lo apaga.
func TestCA359_LetrasDelItemYDelSpec(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 19, 10, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	itemAbs := filepath.Join(root, ".hoom", "items", bdSlug+".yaml")
	specAbs := filepath.Join(root, filepath.FromSlash(bdSpec))

	bdItemArchivo(t, root, bdSlug, "Precios", "")
	r1 := hiCommit(t, root, "alta del item", h(0), hiHenry)
	if err := os.Remove(itemAbs); err != nil {
		t.Fatal(err)
	}
	r2 := hiCommit(t, root, "baja del item", h(1), "")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	r3 := hiCommit(t, root, "vuelve el item", h(2), "")
	bdEscribir(t, root, bdSpec, hiSpecV1)
	r4 := hiCommit(t, root, "spec de precios", h(3), hiAgente)
	bdEscribir(t, root, bdSpec, hiSpecV2)
	r5 := hiCommit(t, root, "otra version del spec", h(4), "")
	if err := os.Remove(specAbs); err != nil {
		t.Fatal(err)
	}
	r6 := hiCommit(t, root, "sin spec", h(5), "")
	bdItemArchivo(t, root, bdSlug, "Precios por region", "")
	r7 := hiCommit(t, root, "titulo nuevo", h(6), "")

	tl := hiTimeline(t, "CA-359", root)
	const it = ".hoom/items/precios.yaml"
	hiSecuencia(t, "CA-359", tl, []hiEsp{
		{SourceGit, KindItem, r1, it},
		{SourceGit, KindItem, r2, it},
		{SourceGit, KindItem, r3, it},
		{SourceGit, KindSpec, r4, bdSpec},
		{SourceGit, KindSpec, r5, bdSpec},
		{SourceGit, KindSpec, r6, bdSpec},
		{SourceGit, KindItem, r7, it},
	})
	for _, d := range []struct {
		kind, ref, artifact, who, plain, asunto, letra string
	}{
		{KindItem, r1, it, hiHenry, "se creo la tarjeta", "alta del item", "A"},
		{KindItem, r2, it, hiYo, "se borro la tarjeta", "baja del item", "D"},
		{KindItem, r3, it, hiYo, "se creo la tarjeta", "vuelve el item", "A"},
		{KindSpec, r4, bdSpec, hiAgente, "se escribio el spec", "spec de precios", "A"},
		{KindSpec, r5, bdSpec, hiYo, "cambio el spec", "otra version del spec", "M"},
		{KindSpec, r6, bdSpec, hiYo, "se borro el spec", "sin spec", "D"},
		{KindItem, r7, it, hiYo, "cambio la tarjeta", "titulo nuevo", "M"},
	} {
		e := hiUna(t, "CA-359", tl, d.kind, d.ref, d.artifact)
		if e.Who != d.who || e.Plain != d.plain {
			t.Fatalf("CA-359: la entrada %s (%s) debe tener who %q y plain %q, fue %q y %q", d.kind, d.letra, d.who, d.plain, e.Who, e.Plain)
		}
		hiResumen(t, "CA-359", e, d.asunto, d.letra)
		if d.kind == KindItem {
			hiEfectos(t, "CA-363", "el item no toca el medidor", e, map[string]string{})
		} else {
			hiEfectos(t, "CA-363", "un spec sin aprobacion o borrado apaga spec", e, map[string]string{"spec": SegFalta})
		}
	}
	hiNotasSon(t, "CA-360", tl, NoteSinEspacio, NoteSinTelemetria)
	if tl.Telemetry != (TelemetryCount{}) {
		t.Fatalf("CA-361: sin telemetria, telemetry es {0, 0}: %+v", tl.Telemetry)
	}
}

// CA-359: una tarjeta recien creada, sin ningun commit, tiene historia vacia
// ([] y no null) y su tarjeta de hoy; y TimelineFor falla exactamente como
// CardFor con un item que no existe o que es invalido.
func TestCA359_SinCommitsYErroresComoCardFor(t *testing.T) {
	root := bdRepo(t, "")
	bdItemArchivo(t, root, bdSlug, "Precios", "") // sin commitear
	bdEscribir(t, root, ".hoom/items/malo.yaml", bdItemYAML("Malo", "columna: hecho\n"))

	tl := hiTimeline(t, "CA-359", root)
	if len(tl.Entries) != 0 {
		t.Fatalf("CA-359: sin commits ni telemetria la historia no tiene entradas:\n%s", hiListado(tl))
	}
	raw, _ := json.Marshal(tl)
	if !strings.Contains(string(raw), `"entries":[]`) {
		t.Fatalf("CA-359: entries vacia es [] en JSON: %s", raw)
	}
	if tl.Card.Slug != bdSlug || tl.Card.Column != ColBacklog {
		t.Fatalf("CA-359: la tarjeta de hoy igual esta en card: %+v", tl.Card)
	}

	for _, slug := range []string{"no-existe", "malo"} {
		_, errC := CardFor(root, "main", "high", slug, hiAhora)
		if errC == nil {
			t.Fatalf("el fixture: CardFor(%s) falla", slug)
		}
		_, errT := TimelineFor(root, "main", "high", slug, hiAhora)
		if errT == nil || errT.Error() != errC.Error() {
			t.Fatalf("CA-359: TimelineFor(%s) falla como CardFor (%q), fue %v", slug, errC, errT)
		}
	}
}

// CA-359: si git falla (un directorio que no es un repo), las entradas de git
// faltan, la historia no es un error y notes dice "sin historial de git: <error>".
func TestCA359_SinHistorialDeGit(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	t.Setenv("GIT_CEILING_DIRECTORIES", base) // que git no encuentre un repo mas arriba
	bdItemArchivo(t, root, bdSlug, "Precios", "")

	tl := hiTimeline(t, "CA-359", root)
	for _, e := range tl.Entries {
		if e.Source == SourceGit {
			t.Fatalf("CA-359: sin git no hay entradas de git: %+v", e)
		}
	}
	hay := false
	for _, n := range tl.Notes {
		if strings.HasPrefix(n, NoteSinGitPrefijo) && strings.TrimSpace(strings.TrimPrefix(n, NoteSinGitPrefijo)) != "" {
			hay = true
		}
	}
	if !hay {
		t.Fatalf("CA-359: notes dice %q seguido del error de git: %q", NoteSinGitPrefijo+"<error>", tl.Notes)
	}
}

// CA-364: en todos los fixtures de la historia cada plain es no vacio y no
// dice git, commit, worktree, merge, rama, branch, diff, HEAD ni huella
// (palabras enteras, sin distinguir mayusculas), ni .hoom/ ni "hoom ". Este
// fixture tiene todos los kinds, con asuntos de commit, notas de sobre y
// detalles de delegacion llenos de ese vocabulario: el plain no los repite.
func TestCA364_PlainSinVocabularioDeGit(t *testing.T) {
	// el chequeo mismo: palabras enteras, sin mayusculas
	for _, mala := range []string{"el Git dice", "un COMMIT", "la rama main", "HEAD movido", "cambio la Huella",
		"hay un diff", "merge pendiente", "el worktree", "branch x", "ver .hoom/specs", "corre hoom verify"} {
		if hiProhibidas.FindString(mala) == "" && !strings.Contains(mala, ".hoom/") && !strings.Contains(mala, "hoom ") {
			t.Fatalf("CA-364: el chequeo debe rechazar %q", mala)
		}
	}
	for _, buena := range []string{"digital", "programa", "comitente", "cabeza", "diferencia", "general-purpose termino su parte"} {
		if w := hiProhibidas.FindString(buena); w != "" {
			t.Fatalf("CA-364: %q no contiene una palabra prohibida entera (%q)", buena, w)
		}
	}

	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 18, 10, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	itemAbs := filepath.Join(root, ".hoom", "items", bdSlug+".yaml")

	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "commit inicial: merge de la rama main en HEAD", h(0), "")
	bdItemArchivo(t, root, bdSlug, "Precios por region", "")
	hiCommit(t, root, "git diff del item en la branch", h(1), "")
	if err := os.Remove(itemAbs); err != nil {
		t.Fatal(err)
	}
	hiCommit(t, root, "borrar el item: huella nueva", h(2), "")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "revert en el worktree: hoom item add", h(3), "")
	wt := bdWorktree(t, root, bdSlug)

	bdEscribir(t, wt, bdSpec, hiSpecV1)
	hiCommit(t, wt, "git add del spec en .hoom/specs", h(4), "")
	hiAprobar(t, wt, bdSlug, hiFirmante, h(5))
	hiCommit(t, wt, "merge de la aprobacion en HEAD", h(5), "")
	bdEscribir(t, wt, bdSpec, hiSpecV2)
	hiCommit(t, wt, "diff del spec", h(6), "")
	hiVeredicto(t, wt, bdSpec, h(7), false, 10, bdGatesVerdes())
	rojo := hiVeredicto(t, wt, bdSpec, h(7).Add(time.Minute), false, 10,
		hiGates("spec_trace", verdict.StatusFail, "test", verdict.StatusFail))
	pv := hiVeredicto(t, wt, bdSpec, h(7).Add(2*time.Minute), true, 10, hiGates("test", verdict.StatusPass))
	pr := hiVeredicto(t, wt, bdSpec, h(7).Add(3*time.Minute), true, 10, hiGates("test", verdict.StatusFail))
	w7 := hiCommit(t, wt, "commit de veredictos (git diff HEAD)", h(7), "")
	fa := hiHallazgo(t, wt, "high", bdSlug)
	fb := hiHallazgo(t, wt, "low", bdSlug)
	hiCommit(t, wt, "hallazgos en la rama", h(8), "")
	hiResolver(t, wt, fa, finding.StatusCorrected)
	hiResolver(t, wt, fb, finding.StatusRefuted)
	w9 := hiCommit(t, wt, "resoluciones: merge --no-ff", h(9), "")
	hiReview(t, wt, "20260918T200000_aaaaaa", bdSlug, reviewcmd.Lentes)
	hiReview(t, wt, "20260918T200000_bbbbbb", bdSlug, []string{"risk"})
	hiCommit(t, wt, "reviews del worktree", h(10), "")
	if err := os.Remove(filepath.Join(wt, filepath.FromSlash(bdSpec))); err != nil {
		t.Fatal(err)
	}
	hiCommit(t, wt, "rama sin spec: git rm", h(11), "")

	// telemetria de todos los estados, con notas y detalles llenos de git
	const nota = "git diff en .hoom/worktrees/precios: hoom verify fallo en HEAD de la rama"
	m := func(n int) time.Time { return h(12).Add(time.Duration(n) * time.Minute) }
	sobre := func(id, rol, prov, status, stage string, desde int, pid int, cerrado bool) envelope.Record {
		rec := envelope.Record{ID: id, Role: rol, Provider: prov, Task: bdSlug, Dir: wt, Stage: stage, Step: 3, Steps: 7,
			Status: status, Note: nota, StartedAt: m(desde), UpdatedAt: m(desde + 1), PID: pid}
		if cerrado {
			rec.EndedAt = m(desde + 1)
		}
		bdSobreDisco(t, root, rec)
		return rec
	}
	sobre("20260918T220000_sobre1", "writer", "claude", envelope.StatusDeliverable, "ok", 0, 0, true)
	noEnt := sobre("20260918T220000_sobre2", "test-writer", "claude", envelope.StatusNotDeliverable, "scope", 2, 0, true)
	sinEnt := sobre("20260918T220000_sobre3", "writer", "codex", envelope.StatusNoDelivery, "run", 4, 0, true)
	sobre("20260918T220000_sobre4", "arquitecto", "claude", envelope.StatusRunning, "run", 6, os.Getpid(), false)
	sobre("20260918T220000_sobre5", "reviewer", "codex", envelope.StatusRunning, "run", 8, bdPIDMuerto(t), false)
	const runErr, runSinRol = "20260918T221000_run001", "20260918T221000_run002"
	bdRunDisco(t, root, runcmd.Meta{ID: runErr, Provider: "claude", Role: "writer", Task: bdSlug, Dir: wt,
		CreatedAt: m(10), EndedAt: m(20), Status: runcmd.StatusError})
	hiEventos(t, root, runErr,
		providers.Event{TS: m(11), Kind: "agent", Agent: "Explore", ToolID: "toolu_X", Detail: "Agent: git log de la rama"},
		providers.Event{TS: m(12), Kind: "agent_end", Agent: "Explore", ToolID: "toolu_X", Detail: "Explore fallo: git merge en HEAD de la rama"},
		providers.Event{TS: m(13), Kind: "agent", Agent: "general-purpose", ToolID: "toolu_Y", Detail: "Agent: diff"},
		providers.Event{TS: m(14), Kind: "agent_end", Agent: "general-purpose", ToolID: "toolu_Y", Detail: "general-purpose termino"},
		providers.Event{TS: m(20), Kind: "error", Detail: "fallo el commit"})
	bdRunDisco(t, root, runcmd.Meta{ID: runSinRol, Provider: "claude", Task: bdSlug, Dir: wt,
		CreatedAt: m(30), EndedAt: m(31), Status: runcmd.StatusDone})
	hiEventos(t, root, runSinRol, providers.Event{TS: m(30), Kind: "start"})

	tl := hiTimeline(t, "CA-364", root) // hiBienFormada chequea el plain de cada entrada
	vistos := map[string]bool{}
	for _, e := range tl.Entries {
		vistos[e.Kind] = true
		for _, feo := range []string{nota, "git merge", "fallo el commit"} {
			if strings.Contains(e.Plain, feo) {
				t.Fatalf("CA-364: el plain no repite notas ni detalles (%q): %q", feo, e.Plain)
			}
		}
	}
	for k := range hiOrdenKind {
		if !vistos[k] {
			t.Fatalf("CA-364: el fixture ejercita todos los kinds de la historia; falta %s:\n%s", k, hiListado(tl))
		}
	}

	// los plain de este fixture que los otros no fijan
	for _, d := range []struct {
		ca, kind, ref, artifact, plain string
	}{
		{"CA-359", KindVeredicto, w7, hiWtp + ".hoom/verdicts/" + rojo.ID + ".json", plainVerdict(rojo)},
		{"CA-359", KindVeredicto, w7, hiWtp + ".hoom/verdicts/" + pv.ID + ".json", "una verificacion parcial dio verde"},
		{"CA-359", KindVeredicto, w7, hiWtp + ".hoom/verdicts/" + pr.ID + ".json", "una verificacion parcial dio rojo"},
		{"CA-359", KindResolucion, w9, hiWtp + ".hoom/findings/" + fa + ".res.json", "se resolvio un problema de la revision (corregido)"},
		{"CA-359", KindResolucion, w9, hiWtp + ".hoom/findings/" + fb + ".res.json", "se resolvio un problema de la revision (refutado)"},
		{"CA-361", KindSobre, noEnt.ID, ".hoom/envelopes/" + noEnt.ID + ".json", plainEnvelope(noEnt)},
		{"CA-361", KindSobre, sinEnt.ID, ".hoom/envelopes/" + sinEnt.ID + ".json", "el writer no entrego: no dejo ningun archivo"},
		{"CA-361", KindRun, runErr, ".hoom/runs/" + runErr + ".jsonl", "el trabajo del writer fallo"},
		{"CA-361", KindRun, runSinRol, ".hoom/runs/" + runSinRol + ".jsonl", "trabajo el claude"},
	} {
		if e := hiUna(t, d.ca, tl, d.kind, d.ref, d.artifact); e.Plain != d.plain {
			t.Fatalf("%s: el plain de %s %s debe ser %q, fue %q", d.ca, d.kind, hiCorto(d.ref), d.plain, e.Plain)
		}
	}
	if plainVerdict(rojo) != "la verificacion dio rojo: fallaron los gates spec_trace, test" {
		t.Fatalf("el fixture: el rojo completo falla spec_trace y test: %q", plainVerdict(rojo))
	}
	if plainEnvelope(noEnt) != "el test-writer no entrego: escribio fuera de lo que le toca" {
		t.Fatalf("el fixture: el plain de C2 del sobre no entregable: %q", plainEnvelope(noEnt))
	}
	fin := hiDeKind(tl, KindSubagenteFin)
	if len(fin) != 2 || fin[0].Plain != "Explore fallo" || fin[1].Plain != "general-purpose termino su parte" {
		t.Fatalf("CA-362: las salidas de escena dicen \"Explore fallo\" y \"general-purpose termino su parte\", sin el detalle: %+v", fin)
	}
	for _, e := range hiDeKind(tl, KindHallazgo) {
		if e.Plain != "la revision encontro un problema (high)" && e.Plain != "la revision encontro un problema (low)" {
			t.Fatalf("CA-359: el plain de un hallazgo nombra su severidad: %q", e.Plain)
		}
	}
}
