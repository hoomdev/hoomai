// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md sobre
// la evidencia EN DISCO: boardcmd.Gather, Build y CardFor (CA-269..CA-277,
// CA-280..CA-283, CA-286, CA-287). Repos git reales en t.TempDir(), sin
// ningun CLI de IA. Lo que se prueba aca es de donde sale cada pieza de la
// Evidence y que leerla no tiene efectos.
package boardcmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func bdGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func bdEscribir(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// bdRepo: proyecto git con hoom.yaml (un gate barato), el .gitignore
// canonico de .hoom commiteado (como deja 'hoom init') y un commit inicial.
func bdRepo(t *testing.T, extraYAML string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bdGit(t, root, "init", "-b", "main")
	bdGit(t, root, "config", "user.email", "test@hoom.dev")
	bdGit(t, root, "config", "user.name", "hoom test")
	bdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+extraYAML)
	bdEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	bdEscribir(t, root, "app.go", "package app\n")
	bdGit(t, root, "add", "-A")
	bdGit(t, root, "commit", "-m", "inicial")
	return root
}

// bdWorktree crea la tarea como la crea 'hoom task start': rama hoom/<slug>
// y worktree en .hoom/worktrees/<slug>, desde main.
func bdWorktree(t *testing.T, root, slug string) string {
	t.Helper()
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	bdGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	return wt
}

func bdCommitear(t *testing.T, dir, msg string) {
	t.Helper()
	bdGit(t, dir, "add", "-A")
	bdGit(t, dir, "commit", "-q", "-m", msg)
}

// bdSpecTexto arma un spec con las 7 secciones (sin "riesgos" si se pide).
func bdSpecTexto(criterios string, conRiesgos bool) string {
	s := "# Spec: precios\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n" + criterios + "\n\n## Decisiones\nx\n"
	if conRiesgos {
		s += "\n## Riesgos\nx\n"
	}
	return s
}

func bdItem(slug string) item.Item {
	return item.Item{Slug: slug, Titulo: "Titulo de " + slug, Tipo: "feature", Prioridad: "media",
		Pedido: "el pedido de " + slug, CreadoPor: "hoom test <test@hoom.dev>", CreadoEn: bdT0}
}

func bdItemYAML(titulo, extra string) string {
	return "titulo: " + titulo + "\ntipo: feature\nprioridad: media\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n" + extra
}

func bdItemArchivo(t *testing.T, root, slug, titulo, extra string) string {
	t.Helper()
	return bdEscribir(t, root, ".hoom/items/"+slug+".yaml", bdItemYAML(titulo, extra))
}

// bdAprobar firma el spec de la tarjeta en dir (la aprobacion vive en dir).
func bdAprobar(t *testing.T, dir, slug string) {
	t.Helper()
	if _, _, err := approval.Approve(dir, filepath.Join(dir, ".hoom", "specs", slug+".md")); err != nil {
		t.Fatalf("no pude aprobar el spec de %s en %s: %v", slug, dir, err)
	}
}

// bdVeredictoDisco escribe un veredicto real con la huella ACTUAL de dir.
func bdVeredictoDisco(t *testing.T, dir, spec string, at time.Time, partial bool, gates []verdict.GateResult) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: at, Partial: partial, Spec: spec,
		Git: gitx.Snapshot(dir, "main"), Gates: gates}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	return v
}

func bdReviewDisco(t *testing.T, dir string, r reviewcmd.Record) string {
	t.Helper()
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return bdEscribir(t, dir, filepath.Join(".hoom", reviewcmd.RecordsDir, r.ID+".json"), string(raw)+"\n")
}

// bdSobreDisco escribe el registro del sobre TAL CUAL (envelope.Write
// pisaria updated_at con el reloj).
func bdSobreDisco(t *testing.T, root string, rec envelope.Record) string {
	t.Helper()
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return bdEscribir(t, root, filepath.Join(".hoom", envelope.DirName, rec.ID+".json"), string(raw)+"\n")
}

func bdRunDisco(t *testing.T, root string, m runcmd.Meta) string {
	t.Helper()
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return bdEscribir(t, root, filepath.Join(".hoom", "runs", m.ID+".meta.json"), string(raw)+"\n")
}

func bdPIDMuerto(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.ProcessState.Pid()
}

// bdFoto fotografia TODO el arbol salvo .git/: ignorados incluidos
// (worktrees, runs, envelopes, cache). Contenido, modo y mtime.
func bdFoto(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if rel == ".git" {
				return filepath.SkipDir
			}
			out[rel+"/"] = "dir"
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			dest, _ := os.Readlink(p)
			out[rel] = "link " + dest
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = fmt.Sprintf("%v %x %d", info.Mode(), sha256.Sum256(raw), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func bdMismaFoto(t *testing.T, ca string, antes, despues map[string]string) {
	t.Helper()
	for k, v := range antes {
		if w, ok := despues[k]; !ok {
			t.Fatalf("%s: leer el tablero borro %s", ca, k)
		} else if w != v {
			t.Fatalf("%s: leer el tablero modifico %s", ca, k)
		}
	}
	for k := range despues {
		if _, ok := antes[k]; !ok {
			t.Fatalf("%s: leer el tablero creo %s", ca, k)
		}
	}
}

func bdCard(t *testing.T, b Board, slug string) Card {
	t.Helper()
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if c.Slug == slug {
				if c.Column != col.ID {
					t.Fatalf("la tarjeta %s esta en la columna %s pero dice %s", slug, col.ID, c.Column)
				}
				return c
			}
		}
	}
	t.Fatalf("el tablero no tiene la tarjeta %s: %+v", slug, b)
	return Card{}
}

// CA-280: la evidencia sale del worktree de la tarea si existe, y del arbol
// actual si no. Un spec aprobado solo en el arbol con un worktree sin spec
// da backlog con source worktree.
func TestCA280_EvidenciaDelWorktreeODelArbol(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	it := bdItem(bdSlug)
	bdItemArchivo(t, root, bdSlug, it.Titulo, "pedido: el pedido de precios\n")

	ev := Gather(root, "main", "high", it, now)
	if ev.Source != SourceArbol || ev.Dir != "." || ev.Worktree || ev.SpecExists || ev.SpecPath != bdSpec {
		t.Fatalf("CA-280: sin worktree la evidencia es el arbol actual (source arbol, dir \".\"): %+v", ev)
	}
	c := Derive(ev)
	bdCol(t, "CA-269", c, ColBacklog)
	if c.Next != "hoom task start precios" {
		t.Fatalf("CA-269: sin worktree el siguiente paso es la tarea: %q", c.Next)
	}

	// spec aprobado en el arbol actual: el arbol es la evidencia
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdAprobar(t, root, bdSlug)
	ev = Gather(root, "main", "high", it, now)
	if !ev.SpecExists || ev.Approval != approval.StatusApproved || ev.Source != SourceArbol {
		t.Fatalf("CA-280: el spec y su aprobacion se leen del arbol: %+v", ev)
	}

	// con worktree (sin el spec, que no se commiteo): Backlog, y la
	// evidencia dice que miro el worktree
	bdWorktree(t, root, bdSlug)
	ev = Gather(root, "main", "high", it, now)
	if ev.Source != SourceWorktree || ev.Dir != bdWtDir || !ev.Worktree || ev.SpecExists {
		t.Fatalf("CA-280: con worktree la evidencia es el worktree (source worktree, dir %s, sin spec): %+v", bdWtDir, ev)
	}
	c = Derive(ev)
	bdCol(t, "CA-280", c, ColBacklog)
	if !strings.HasPrefix(c.Next, "hoom agent --role arquitecto --task precios") {
		t.Fatalf("CA-269: con worktree el siguiente paso es el arquitecto de la tarea: %q", c.Next)
	}

	card, err := CardFor(root, "main", "high", bdSlug, now)
	if err != nil {
		t.Fatalf("CA-280: CardFor: %v", err)
	}
	if card.Column != ColBacklog || card.Evidence.Source != SourceWorktree || card.Evidence.Dir != bdWtDir {
		t.Fatalf("CA-280: CardFor da la misma lectura (backlog, source worktree): %+v", card)
	}
	raw, _ := json.Marshal(card)
	if !strings.Contains(string(raw), `"source":"worktree"`) || !strings.Contains(string(raw), `"dir":".hoom/worktrees/precios"`) {
		t.Fatalf("CA-280: el JSON de la tarjeta dice donde miro: %s", raw)
	}

	// el worktree trae otra copia del item: manda la del arbol actual; un
	// item que existe solo en el worktree no aparece en el tablero del arbol
	wt := filepath.Join(root, bdWtDir)
	bdEscribir(t, wt, ".hoom/items/precios.yaml", bdItemYAML("Copia del worktree", ""))
	bdEscribir(t, wt, ".hoom/items/solo-wt.yaml", bdItemYAML("Solo en el worktree", ""))
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-280: Build: %v", err)
	}
	if got := bdCard(t, b, bdSlug); got.Item.Titulo != it.Titulo {
		t.Fatalf("CA-280: manda el item del arbol actual (%q), fue %q", it.Titulo, got.Item.Titulo)
	}
	for _, col := range b.Columns {
		for _, cc := range col.Cards {
			if cc.Slug == "solo-wt" {
				t.Fatal("CA-280: un item creado dentro del worktree no aparece en el tablero del arbol principal")
			}
		}
	}
}

// CA-270: un spec vacio o sin la seccion riesgos, leido del disco, es
// Arquitecto con los issues de spec.Lint tal cual.
func TestCA270_ArquitectoDesdeElDisco(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", false))
	c := Derive(Gather(root, "main", "high", bdItem(bdSlug), now))
	bdCol(t, "CA-270", c, ColArquitecto)
	if !reflect.DeepEqual(c.Missing, []string{`falta la seccion "riesgos"`}) {
		t.Fatalf("CA-270: el issue de lint tal cual: %q", c.Missing)
	}
	if !reflect.DeepEqual(c.Evidence.LintIssues, []string{`falta la seccion "riesgos"`}) || !c.Evidence.SpecExists {
		t.Fatalf("CA-270: la evidencia trae los issues: %+v", c.Evidence)
	}

	bdEscribir(t, root, bdSpec, "")
	c = Derive(Gather(root, "main", "high", bdItem(bdSlug), now))
	bdCol(t, "CA-270", c, ColArquitecto)
	bdTiene(t, "CA-270", c.Missing, `falta la seccion "objetivo"`)
	bdTiene(t, "CA-270", c.Missing, "no hay criterios de aceptacion")
}

// CA-271: sin aprobacion, y con una aprobacion de un contenido anterior.
func TestCA271_AprobacionDesdeElDisco(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	c := Derive(Gather(root, "main", "high", bdItem(bdSlug), now))
	bdCol(t, "CA-271", c, ColTuAprobacion)
	bdPrimero(t, "CA-271", c, "el spec no tiene aprobacion humana")
	if c.Evidence.Approval != approval.StatusNotApproved || !c.WaitingHuman {
		t.Fatalf("CA-271: approval no-aprobado y esperando humano: %+v", c)
	}

	bdAprobar(t, root, bdSlug)
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo distinto. [verifica: true]", true))
	c = Derive(Gather(root, "main", "high", bdItem(bdSlug), now))
	bdCol(t, "CA-271", c, ColTuAprobacion)
	bdPrimero(t, "CA-271", c, "el spec cambio despues de tu aprobacion")
	if c.Evidence.Approval != approval.StatusInvalidated {
		t.Fatalf("CA-271: approval invalidado: %+v", c.Evidence)
	}
}

// CA-272: el marcador verifica cuenta como cubierto y el tablero NO corre el
// comando: 'touch marca' no crea 'marca', y un comando que falla igual
// cuenta como cubierto.
func TestCA272_MarcadorVerificaNoSeEjecuta(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	bdEscribir(t, root, bdSpec, bdSpecTexto(
		"- CA-1: se marca. [verifica: touch marca]\n- CA-2: con test.\n- CA-3: comando que falla. [verifica: false]", true))
	bdAprobar(t, root, bdSlug)
	bdItemArchivo(t, root, bdSlug, "Precios", "")

	ev := Gather(root, "main", "high", bdItem(bdSlug), now)
	if !reflect.DeepEqual(ev.Criteria, []string{"CA-1", "CA-2", "CA-3"}) || !reflect.DeepEqual(ev.Untraced, []string{"CA-2"}) {
		t.Fatalf("CA-272: criterios CA-1..3; sin test solo CA-2 (los de verifica cuentan): %v %v", ev.Criteria, ev.Untraced)
	}
	c := Derive(ev)
	bdCol(t, "CA-272", c, ColTestWriter)
	bdPrimero(t, "CA-272", c, "criterios sin test: CA-2")
	if c.Evidence.Criteria != 3 || c.Evidence.Traced != 2 {
		t.Fatalf("CA-272: criteria 3, traced 2: %+v", c.Evidence)
	}
	if _, err := Build(root, "main", "high", now); err != nil {
		t.Fatal(err)
	}
	if _, err := CardFor(root, "main", "high", bdSlug, now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "marca")); !os.IsNotExist(err) {
		t.Fatalf("CA-272: el tablero no ejecuta el marcador: 'marca' no debe existir (err=%v)", err)
	}

	// un test que cita CA-2: trazado entero, la tarjeta pasa a Writer
	bdEscribir(t, root, "precios_test.go", "package app\n\n// CA-2: con test\n")
	c = Derive(Gather(root, "main", "high", bdItem(bdSlug), now))
	bdCol(t, "CA-272", c, ColWriter)
	if c.Evidence.Traced != 3 || len(c.Evidence.Untraced) != 0 {
		t.Fatalf("CA-272: trazado entero: %+v", c.Evidence)
	}
}

// CA-273: el veredicto de la tarjeta es el mas nuevo NO parcial cuyo spec,
// limpio y relativo, es el de la tarjeta. Uno verde de otro spec o uno verde
// parcial no la sacan de Writer; './' y la ruta absoluta si cuentan.
func TestCA273_VeredictoDeLaTarjetaEnDisco(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdAprobar(t, root, bdSlug)
	it := bdItem(bdSlug)

	otro := bdVeredictoDisco(t, root, ".hoom/specs/otra.md", bdT0.Add(1*time.Minute), false, bdGatesVerdes())
	parcial := bdVeredictoDisco(t, root, bdSpec, bdT0.Add(2*time.Minute), true, bdGatesVerdes())
	ev := Gather(root, "main", "high", it, now)
	if ev.Verdict != nil {
		t.Fatalf("CA-273: ni el verde de otro spec (%s) ni el parcial (%s) son de la tarjeta: %+v", otro.ID, parcial.ID, ev.Verdict)
	}
	for _, id := range ev.GreenVerdicts {
		if id == otro.ID || id == parcial.ID {
			t.Fatalf("CA-273: green_verdicts solo tiene verdes completos de la tarjeta: %v", ev.GreenVerdicts)
		}
	}
	c := Derive(ev)
	bdCol(t, "CA-273", c, ColWriter)
	if len(c.Missing) == 0 || !strings.HasPrefix(c.Missing[0], "no hay veredicto de la tarjeta") {
		t.Fatalf("CA-273: el motivo: %q", c.Missing)
	}

	punto := bdVeredictoDisco(t, root, "./"+bdSpec, bdT0.Add(3*time.Minute), false, bdGatesVerdes())
	ev = Gather(root, "main", "high", it, now)
	if ev.Verdict == nil || ev.Verdict.ID != punto.ID {
		t.Fatalf("CA-273: un veredicto con spec './.hoom/specs/precios.md' cuenta: %+v", ev.Verdict)
	}
	abs := bdVeredictoDisco(t, root, filepath.Join(root, bdSpec), bdT0.Add(4*time.Minute), false, bdGatesVerdes())
	// un parcial ROJO mas nuevo no pisa al completo, en ninguna direccion
	bdVeredictoDisco(t, root, bdSpec, bdT0.Add(5*time.Minute), true,
		[]verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
	ev = Gather(root, "main", "high", it, now)
	if ev.Verdict == nil || ev.Verdict.ID != abs.ID {
		t.Fatalf("CA-273: la ruta absoluta cuenta y el parcial rojo no pisa: %+v", ev.Verdict)
	}
	sort.Strings(ev.GreenVerdicts)
	want := []string{punto.ID, abs.ID}
	sort.Strings(want)
	if !reflect.DeepEqual(ev.GreenVerdicts, want) {
		t.Fatalf("CA-273: green_verdicts son los verdes completos de la tarjeta %v, fue %v", want, ev.GreenVerdicts)
	}
	if ev.Fingerprint != gitx.Snapshot(root, "main").ChangeFingerprint || ev.Fingerprint == "" {
		t.Fatalf("CA-273: la huella actual de E: %q", ev.Fingerprint)
	}
}

// CA-274: los registros de review son los de E con task = S.
func TestCA274_RegistrosDeReviewDeLaTarjeta(t *testing.T) {
	root := bdRepo(t, "")
	rec := func(id, task string) reviewcmd.Record {
		return reviewcmd.Record{ID: id, CreatedAt: bdT0, Task: task, Spec: ".hoom/specs/" + task + ".md",
			Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: reviewcmd.Lentes,
			Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}}
	}
	bdReviewDisco(t, root, rec("20260922T150000_aaaaaa", "otra"))
	bdReviewDisco(t, root, rec("20260922T150100_bbbbbb", bdSlug))
	bdReviewDisco(t, root, rec("20260922T150200_cccccc", ""))
	ev := Gather(root, "main", "high", bdItem(bdSlug), time.Now().UTC())
	if len(ev.Reviews) != 1 || ev.Reviews[0].ID != "20260922T150100_bbbbbb" || ev.Reviews[0].Task != bdSlug {
		t.Fatalf("CA-274: solo el registro con task = precios (no el de otra tarea ni el sin tarea): %+v", ev.Reviews)
	}
}

// CA-275: los hallazgos de la tarjeta son los ABIERTOS de E con task = S. No
// entran uno resuelto, uno de otra tarea ni uno sin tarea. El umbral
// efectivo es el block_on del proyecto, o high si no declara.
func TestCA275_HallazgosDeLaTarjetaEnDisco(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	alta := func(sev, task, desc string) string {
		f, err := finding.Register(root, "main", finding.Draft{Severity: sev, Lens: "risk", Description: desc,
			Author: "reviewer", Task: task})
		if err != nil {
			t.Fatal(err)
		}
		return f.ID
	}
	abierto := alta("high", bdSlug, "abierto de la tarjeta")
	bajo := alta("low", bdSlug, "bajo de la tarjeta")
	resuelto := alta("high", bdSlug, "resuelto de la tarjeta")
	if _, err := finding.Resolve(root, resuelto, finding.StatusRefuted, "TestX lo refuta", "refutador"); err != nil {
		t.Fatal(err)
	}
	alta("high", "otra", "de otra tarea")
	alta("high", "", "sin tarea")

	ev := Gather(root, "main", "high", bdItem(bdSlug), time.Now().UTC())
	var ids []string
	for _, f := range ev.Findings {
		ids = append(ids, f.Finding.ID)
		if f.Finding.Task != bdSlug || f.Status != finding.StatusOpen {
			t.Fatalf("CA-275: solo hallazgos abiertos de la tarjeta: %+v", f)
		}
	}
	sort.Strings(ids)
	want := []string{abierto, bajo}
	sort.Strings(want)
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("CA-275: hallazgos de la tarjeta %v, fueron %v", want, ids)
	}
	if ev.BlockOn != "high" {
		t.Fatalf("CA-275: block_on del proyecto: %q", ev.BlockOn)
	}
	if ev = Gather(root, "main", "", bdItem(bdSlug), time.Now().UTC()); ev.BlockOn != "high" {
		t.Fatalf("CA-275: sin block_on el umbral efectivo es high, fue %q", ev.BlockOn)
	}
	if ev = Gather(root, "main", "medium", bdItem(bdSlug), time.Now().UTC()); ev.BlockOn != "medium" {
		t.Fatalf("CA-275: con block_on medium el umbral es medium, fue %q", ev.BlockOn)
	}
}

// CA-277: la condicion de cierre es taskcmd.Ready sobre el worktree real:
// limpio y verde da Tu aceptacion; sucio, el mensaje de cambios sin
// commitear; con el ultimo completo rojo (de otro verify), el de rojo.
func TestCA277_CierreDesdeElWorktree(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdEscribir(t, wt, "precios.go", "package app\n\nfunc Precio() int { return 1 }\n")
	bdAprobar(t, wt, bdSlug)
	v := bdVeredictoDisco(t, wt, bdSpec, bdT0.Add(time.Minute), false, bdGatesVerdes())
	bdCommitear(t, wt, "spec, codigo y veredicto")

	ev := Gather(root, "main", "high", bdItem(bdSlug), now)
	if !ev.Worktree || ev.Source != SourceWorktree || ev.Verdict == nil || ev.Verdict.ID != v.ID || ev.ReadyErr != "" {
		t.Fatalf("CA-277: el worktree limpio y verde esta listo para cerrar: worktree=%v source=%s verdict=%+v ready=%q",
			ev.Worktree, ev.Source, ev.Verdict, ev.ReadyErr)
	}
	c := Derive(ev)
	bdCol(t, "CA-278", c, ColTuAceptacion)

	// sucio con algo que no mueve la huella (un hallazgo sin tarea, que
	// tampoco bloquea a la tarjeta): Review por cierre
	if _, err := finding.Register(wt, "main", finding.Draft{Severity: "low", Lens: "risk", Description: "nota",
		Author: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	ev = Gather(root, "main", "high", bdItem(bdSlug), now)
	if !strings.Contains(ev.ReadyErr, "tiene cambios sin commitear") {
		t.Fatalf("CA-277: ReadyErr trae el mensaje de taskcmd.Ready sobre cambios sin commitear: %q", ev.ReadyErr)
	}
	c = Derive(ev)
	bdCol(t, "CA-277", c, ColReview)
	bdTiene(t, "CA-277", c.Missing, "tiene cambios sin commitear")
	bdCommitear(t, wt, "hallazgo")

	// alguien corrio 'hoom verify' sin --spec y salio rojo: el ultimo
	// completo del worktree es rojo aunque el de la tarjeta sea verde
	rojo := bdVeredictoDisco(t, wt, "", bdT0.Add(2*time.Minute), false,
		[]verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
	bdCommitear(t, wt, "veredicto rojo sin spec")
	ev = Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.Verdict == nil || ev.Verdict.ID != v.ID {
		t.Fatalf("CA-277: el veredicto de la tarjeta sigue siendo el verde con spec: %+v", ev.Verdict)
	}
	if !strings.Contains(ev.ReadyErr, "ROJO") || !strings.Contains(ev.ReadyErr, rojo.ID) {
		t.Fatalf("CA-277: ReadyErr es el mensaje de task done sobre el rojo (%s): %q", rojo.ID, ev.ReadyErr)
	}
	c = Derive(ev)
	bdCol(t, "CA-277", c, ColReview)
	bdPrimero(t, "CA-277", c, ev.ReadyErr)
	if c.Red != nil {
		t.Fatalf("CA-285: el rojo de la tarjeta es el de SU veredicto (verde): %+v", c.Red)
	}
}

// CA-282, CA-283: la vida del dueño se resuelve en Gather: sidecar running
// con PID vivo, o latido fresco si el sobre no tiene run o el run ya cerro.
// Leer no toca los registros.
func TestCA282_VidaDelDuenoEnDisco(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	fresco := now.Add(-1 * time.Minute)
	viejo := now.Add(-live.OrphanAfter - time.Minute)
	yo := os.Getpid()
	muerto := bdPIDMuerto(t)
	sobre := func(id, task, runID, stage string, step int, updated time.Time) string {
		return bdSobreDisco(t, root, envelope.Record{ID: id, Role: "writer", Provider: "claude", Task: task,
			Dir: root, RunID: runID, Stage: stage, Step: step, Steps: 5, Status: envelope.StatusRunning,
			StartedAt: now.Add(-30 * time.Minute), UpdatedAt: updated})
	}
	run := func(id, task, status string, pid int) string {
		return bdRunDisco(t, root, runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: task, Dir: root,
			CreatedAt: now.Add(-30 * time.Minute), Status: status, ExitCode: -1, PID: pid})
	}
	var registros []string
	// vivo por PID aunque el latido sea viejo
	registros = append(registros, sobre("20260922T100000_aaaaa1", "vivo", "20260922T100000_rrrrr1", "run", 2, viejo))
	registros = append(registros, run("20260922T100000_rrrrr1", "vivo", runcmd.StatusRunning, yo))
	// PID muerto aunque el latido sea fresco
	registros = append(registros, sobre("20260922T100000_aaaaa2", "muerto", "20260922T100000_rrrrr2", "run", 3, fresco))
	registros = append(registros, run("20260922T100000_rrrrr2", "muerto", runcmd.StatusRunning, muerto))
	// sin run y latido viejo
	registros = append(registros, sobre("20260922T100000_aaaaa3", "latido-viejo", "", "spec", 1, viejo))
	// run ya cerrado (paso verify) y latido fresco
	registros = append(registros, sobre("20260922T100000_aaaaa4", "paso-verify", "20260922T100000_rrrrr4", "verify", 4, fresco))
	registros = append(registros, run("20260922T100000_rrrrr4", "paso-verify", runcmd.StatusDone, 0))
	// run suelto vivo
	registros = append(registros, run("20260922T100000_rrrrr5", "suelto", runcmd.StatusRunning, yo))
	// run suelto sin PID (version vieja): no cuenta
	registros = append(registros, run("20260922T100000_rrrrr6", "pid-cero", runcmd.StatusRunning, 0))
	// sobre con run cuyo sidecar desaparecio: se juzga por el latido
	registros = append(registros, sobre("20260922T100000_aaaaa7", "sin-sidecar", "20260922T100000_fantas", "run", 2, fresco))
	registros = append(registros, sobre("20260922T100000_aaaaa8", "sin-sidecar-viejo", "20260922T100000_fanta2", "run", 2, viejo))
	// sidecar con PID 0 y sobre: el latido decide
	registros = append(registros, sobre("20260922T100000_aaaaa9", "pid0-sobre", "20260922T100000_rrrrr9", "run", 2, fresco))
	registros = append(registros, run("20260922T100000_rrrrr9", "pid0-sobre", runcmd.StatusRunning, 0))
	antes := map[string][]byte{}
	for _, p := range registros {
		antes[p], _ = os.ReadFile(p)
	}

	card := func(slug string) Card {
		t.Helper()
		ev := Gather(root, "main", "high", bdItem(slug), now)
		for _, e := range ev.Envelopes {
			if e.Record.Task != slug {
				t.Fatalf("CA-282: la evidencia de %s trae solo sus sobres: %+v", slug, e.Record)
			}
		}
		for _, r := range ev.Runs {
			if r.Meta.Task != slug {
				t.Fatalf("CA-282: la evidencia de %s trae solo sus runs: %+v", slug, r.Meta)
			}
		}
		return Derive(ev)
	}
	enCurso := func(slug, envID, stage string) {
		t.Helper()
		c := card(slug)
		if c.Running == nil || c.Running.Stage != stage || (envID != "" && c.Running.EnvelopeID != envID) {
			t.Fatalf("CA-282 (%s): en curso con stage %s y envelope %q: running=%+v interrupted=%+v", slug, stage, envID, c.Running, c.Interrupted)
		}
		if c.Interrupted != nil {
			t.Fatalf("CA-282 (%s): en curso no es interrumpido: %+v", slug, c.Interrupted)
		}
	}
	interrumpido := func(slug, envID string, step int) {
		t.Helper()
		c := card(slug)
		if c.Running != nil {
			t.Fatalf("CA-283 (%s): sin dueño vivo running es null: %+v", slug, c.Running)
		}
		if c.Interrupted == nil || c.Interrupted.EnvelopeID != envID || c.Interrupted.Step != step || c.Interrupted.Steps != 5 {
			t.Fatalf("CA-283 (%s): interrumpido en el paso %d de 5 (%s): %+v", slug, step, envID, c.Interrupted)
		}
	}
	enCurso("vivo", "20260922T100000_aaaaa1", "run")
	if c := card("vivo"); c.Running.Role != "writer" || c.Running.Provider != "claude" || c.Running.Step != 2 || c.Running.Steps != 5 {
		t.Fatalf("CA-282: running trae role, provider, step y steps del sobre: %+v", c.Running)
	}
	interrumpido("muerto", "20260922T100000_aaaaa2", 3)
	interrumpido("latido-viejo", "20260922T100000_aaaaa3", 1)
	if c := card("latido-viejo"); c.Interrupted.Stage != "spec" || c.Interrupted.UpdatedAt.IsZero() {
		t.Fatalf("CA-283: interrumpido trae stage y updated_at: %+v", c.Interrupted)
	}
	enCurso("paso-verify", "20260922T100000_aaaaa4", "verify")
	enCurso("suelto", "", "run")
	if c := card("suelto"); c.Running.RunID != "20260922T100000_rrrrr5" {
		t.Fatalf("CA-282: el run suelto en curso trae su run_id: %+v", c.Running)
	}
	if c := card("pid-cero"); c.Running != nil || c.Interrupted != nil {
		t.Fatalf("CA-282: un run suelto sin PID no cuenta como en curso: %+v %+v", c.Running, c.Interrupted)
	}
	enCurso("sin-sidecar", "20260922T100000_aaaaa7", "run")
	interrumpido("sin-sidecar-viejo", "20260922T100000_aaaaa8", 2)
	enCurso("pid0-sobre", "20260922T100000_aaaaa9", "run")

	for _, p := range registros {
		if ahora, _ := os.ReadFile(p); !bytes.Equal(ahora, antes[p]) {
			t.Fatalf("CA-283: hoom rotula, no cierra: %s quedo distinto\nantes: %s\nahora: %s", p, antes[p], ahora)
		}
	}
}

// CA-286: sin sincronizar. Con E = arbol actual: las rutas de la tarjeta sin
// commitear, y nada ajeno. Con E = worktree: todo lo sucio del worktree, y
// siempre el item del arbol actual.
func TestCA286_SinSincronizarEnDisco(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	bdEscribir(t, root, "ajeno.go", "package app\n")
	bdCommitear(t, root, "ajeno")

	itemP := ".hoom/items/precios.yaml"
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdAprobar(t, root, bdSlug)
	v := bdVeredictoDisco(t, root, bdSpec, bdT0, false, bdGatesVerdes())
	f, err := finding.Register(root, "main", finding.Draft{Severity: "low", Lens: "risk", Description: "de la tarjeta",
		Author: "reviewer", Task: bdSlug})
	if err != nil {
		t.Fatal(err)
	}
	rv := reviewcmd.Record{ID: "20260922T150405_ab12cd", CreatedAt: bdT0, Task: bdSlug, Spec: bdSpec,
		Lenses: []string{"reliability"}, Findings: []string{}}
	bdReviewDisco(t, root, rv)
	// lo ajeno: codigo sucio, otro spec, un hallazgo de otra tarea
	bdEscribir(t, root, "ajeno.go", "package app // sucio\n")
	bdEscribir(t, root, "suelto.go", "package app\n")
	bdEscribir(t, root, ".hoom/specs/otra.md", "# otra\n")
	fo, err := finding.Register(root, "main", finding.Draft{Severity: "low", Lens: "risk", Description: "ajeno",
		Author: "reviewer", Task: "otra"})
	if err != nil {
		t.Fatal(err)
	}

	var aprob string
	entradas, _ := os.ReadDir(filepath.Join(root, ".hoom", "approvals"))
	for _, e := range entradas {
		if strings.HasPrefix(e.Name(), bdSlug+"_") {
			aprob = ".hoom/approvals/" + e.Name()
		}
	}
	if aprob == "" {
		t.Fatal("no encontre la aprobacion de precios")
	}
	c := Derive(Gather(root, "main", "high", bdItem(bdSlug), now))
	quiere := []string{itemP, bdSpec, aprob, ".hoom/verdicts/" + v.ID + ".json",
		".hoom/findings/" + f.ID + ".json", ".hoom/reviews/" + rv.ID + ".json"}
	for _, p := range quiere {
		found := false
		for _, u := range c.Unsynced {
			if u == p {
				found = true
			}
		}
		if !found {
			t.Fatalf("CA-286: con E = arbol, %s sin commitear aparece en unsynced: %q", p, c.Unsynced)
		}
	}
	for _, p := range []string{"ajeno.go", "suelto.go", ".hoom/specs/otra.md", ".hoom/findings/" + fo.ID + ".json"} {
		for _, u := range c.Unsynced {
			if u == p {
				t.Fatalf("CA-286: lo ajeno a la tarjeta (%s) no aparece en unsynced: %q", p, c.Unsynced)
			}
		}
	}
	if !sort.StringsAreSorted(c.Unsynced) {
		t.Fatalf("CA-286: unsynced ordenado: %q", c.Unsynced)
	}

	// con worktree: lo sucio del worktree, y el item sin commitear del arbol
	otro := bdRepo(t, "")
	bdItemArchivo(t, otro, bdSlug, "Precios", "")
	bdEscribir(t, otro, "ajeno.go", "package app // sucio en el arbol\n")
	wt := bdWorktree(t, otro, bdSlug)
	bdEscribir(t, wt, "nuevo.go", "package app\n")
	c = Derive(Gather(otro, "main", "high", bdItem(bdSlug), now))
	hayNuevo, hayItem := false, false
	for _, u := range c.Unsynced {
		if strings.HasSuffix(u, "nuevo.go") {
			hayNuevo = true
		}
		if u == itemP {
			hayItem = true
		}
		if u == "ajeno.go" {
			t.Fatalf("CA-286: con E = worktree, lo sucio del arbol principal (salvo el item) no cuenta: %q", c.Unsynced)
		}
	}
	if !hayNuevo || !hayItem {
		t.Fatalf("CA-286: con E = worktree aparecen nuevo.go del worktree y el item del arbol: %q", c.Unsynced)
	}

	// commiteado todo: sincronizada
	limpio := bdRepo(t, "")
	bdItemArchivo(t, limpio, bdSlug, "Precios", "")
	bdCommitear(t, limpio, "item")
	c = Derive(Gather(limpio, "main", "high", bdItem(bdSlug), now))
	if len(c.Unsynced) != 0 {
		t.Fatalf("CA-286: sin nada sin commitear, unsynced vacio: %q", c.Unsynced)
	}
}

// CA-287: el gasto sale de los sidecars de la tarjeta (un run de otra tarea
// no suma) y del usage de los sobres cuyo run no tiene sidecar.
func TestCA287_GastoEnDisco(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	run := func(id, task, prov string, u *providers.Usage) {
		bdRunDisco(t, root, runcmd.Meta{ID: id, Provider: prov, Role: "writer", Task: task, Dir: root,
			CreatedAt: now.Add(-time.Hour), Status: runcmd.StatusDone, Usage: u, EndedAt: now.Add(-50 * time.Minute)})
	}
	run("20260922T100000_gasto1", "gasto", "claude", &providers.Usage{CostUSD: bdF(0.8), InputTokens: 1000, OutputTokens: 100})
	run("20260922T100000_gasto2", "gasto", "codex", &providers.Usage{InputTokens: 2000, OutputTokens: 200})
	run("20260922T100000_otra01", "otra", "claude", &providers.Usage{CostUSD: bdF(9), InputTokens: 7, OutputTokens: 7})
	bdSobreDisco(t, root, envelope.Record{ID: "20260922T100000_sobre1", Role: "writer", Provider: "claude", Task: "gasto",
		Dir: root, RunID: "20260922T100000_gasto1", Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
		Usage:     &providers.Usage{CostUSD: bdF(0.8), InputTokens: 1000, OutputTokens: 100},
		StartedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-40 * time.Minute), EndedAt: now.Add(-40 * time.Minute)})
	bdSobreDisco(t, root, envelope.Record{ID: "20260922T100000_sobre2", Role: "writer", Provider: "claude", Task: "gasto-sobre",
		Dir: root, RunID: "20260922T100000_perdid", Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
		Usage:     &providers.Usage{CostUSD: bdF(0.5), InputTokens: 10, OutputTokens: 1},
		StartedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-40 * time.Minute), EndedAt: now.Add(-40 * time.Minute)})
	run("20260922T100000_nocost", "sin-costo", "codex", &providers.Usage{InputTokens: 5, OutputTokens: 5})

	it := bdItem("gasto")
	it.PresupuestoUSD = bdF(5)
	s := Derive(Gather(root, "main", "high", it, now)).Spend
	if s.CostUSD == nil || !bdCasi(*s.CostUSD, 0.8) || s.Runs != 2 || s.RunsWithoutCost != 1 ||
		s.InputTokens != 3000 || s.OutputTokens != 300 {
		t.Fatalf("CA-287: 2 runs de la tarjeta (0.8 y sin costo), sin contar el sobre dos veces ni el run de otra tarea: %+v cost=%v", s, s.CostUSD)
	}
	if s.BudgetUSD == nil || *s.BudgetUSD != 5 {
		t.Fatalf("CA-287: budget_usd = presupuesto_usd: %v", s.BudgetUSD)
	}
	s = Derive(Gather(root, "main", "high", bdItem("gasto-sobre"), now)).Spend
	if s.CostUSD == nil || !bdCasi(*s.CostUSD, 0.5) || s.InputTokens != 10 || s.OutputTokens != 1 {
		t.Fatalf("CA-287: el sobre cuyo run no tiene sidecar suma su usage: %+v cost=%v", s, s.CostUSD)
	}
	s = Derive(Gather(root, "main", "high", bdItem("sin-costo"), now)).Spend
	if s.CostUSD != nil || s.Runs != 1 || s.RunsWithoutCost != 1 {
		t.Fatalf("CA-287: solo runs sin costo dan cost_usd null: %+v", s)
	}
}

// CA-281: Gather, Build y CardFor son de solo lectura: foto de TODO el arbol
// (ignorados incluidos) antes y despues.
func TestCA281_GatherBuildCardForSoloLeen(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdItemArchivo(t, root, "arbol", "En el arbol", "")
	bdEscribir(t, root, ".hoom/items/roto.yaml", "titulo: [roto\n")
	bdEscribir(t, root, ".hoom/specs/arbol.md", bdSpecTexto("- CA-1: se marca. [verifica: touch marca]\n- CA-2: otro.", true))
	bdAprobar(t, root, "arbol")
	bdVeredictoDisco(t, root, ".hoom/specs/arbol.md", bdT0, false, bdGatesVerdes())
	if _, err := finding.Register(root, "main", finding.Draft{Severity: "high", Lens: "risk", Description: "x",
		Author: "reviewer", Task: "arbol"}); err != nil {
		t.Fatal(err)
	}
	bdReviewDisco(t, root, reviewcmd.Record{ID: "20260922T150405_ab12cd", CreatedAt: bdT0, Task: "arbol",
		Lenses: reviewcmd.Lentes, Findings: []string{}})
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: touch marca]", true))
	bdEscribir(t, wt, "sucio.go", "package app\n")
	bdSobreDisco(t, root, envelope.Record{ID: "20260922T100000_sobre1", Role: "writer", Provider: "claude", Task: bdSlug,
		Dir: wt, Stage: "run", Step: 2, Steps: 5, Status: envelope.StatusRunning,
		StartedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour)})
	bdRunDisco(t, root, runcmd.Meta{ID: "20260922T100000_run001", Provider: "claude", Task: bdSlug, Dir: wt,
		CreatedAt: now.Add(-time.Hour), Status: runcmd.StatusRunning, PID: bdPIDMuerto(t)})

	antes := bdFoto(t, root)
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-281: Build: %v", err)
	}
	for _, slug := range []string{bdSlug, "arbol"} {
		if _, err := CardFor(root, "main", "high", slug, now); err != nil {
			t.Fatalf("CA-281: CardFor(%s): %v", slug, err)
		}
		Gather(root, "main", "", bdItem(slug), now)
	}
	bdMismaFoto(t, "CA-281", antes, bdFoto(t, root))
	// y leyo de verdad: no vale la lectura que no mira nada
	if c := bdCard(t, b, "arbol"); c.Column != ColTestWriter || c.Evidence.Source != SourceArbol {
		t.Fatalf("CA-281: la tarjeta del arbol esta en test-writer (CA-2 sin test): %+v", c)
	}
	if c := bdCard(t, b, bdSlug); c.Evidence.Source != SourceWorktree || c.Interrupted == nil || len(c.Unsynced) == 0 {
		t.Fatalf("CA-281: la tarjeta del worktree lee el worktree, el sobre caido y lo sucio: %+v", c)
	}
	bdTiene(t, "CA-281: el item roto va a warnings", b.Warnings, "roto.yaml")
	if _, err := os.Stat(filepath.Join(root, "marca")); !os.IsNotExist(err) {
		t.Fatal("CA-272: el tablero no corre marcadores verifica")
	}
	if _, err := os.Stat(filepath.Join(wt, "marca")); !os.IsNotExist(err) {
		t.Fatal("CA-272: el tablero no corre marcadores verifica en el worktree")
	}
}

// CA-267, CA-288: CardFor de un slug sin item es error; Build con un item
// invalido lo pasa a warnings y sigue.
func TestCA288_BuildOchoColumnasYAvisos(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	if _, err := CardFor(root, "main", "high", "no-existe", now); err == nil {
		t.Fatal("CA-267: CardFor de un item inexistente es error")
	}
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-288: Build sin items no falla: %v", err)
	}
	if len(b.Columns) != 8 {
		t.Fatalf("CA-288: las 8 columnas siempre, aun sin items: %+v", b.Columns)
	}
	bdItemArchivo(t, root, "uno", "Uno", "")
	bdItemArchivo(t, root, "dos", "Dos", "")
	bdEscribir(t, root, ".hoom/items/malo.yaml", bdItemYAML("Malo", "columna: hecho\n"))
	b, err = Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-288: un item invalido no rompe el tablero: %v", err)
	}
	for i, col := range b.Columns {
		if col.ID != Columns[i].ID || col.Name != Columns[i].Name || col.Human != Columns[i].Human {
			t.Fatalf("CA-288: columna %d fuera de orden: %+v", i, col)
		}
		if col.Cards == nil {
			t.Fatalf("CA-288: cards de %s es [] (no nil)", col.ID)
		}
	}
	if len(b.Columns[0].Cards) != 2 {
		t.Fatalf("CA-288: los dos items validos sin spec estan en Backlog: %+v", b.Columns[0].Cards)
	}
	bdTiene(t, "CA-288: el item invalido va a warnings nombrando el archivo", b.Warnings, "malo.yaml")
	raw, err := JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	var js struct {
		Columns []struct {
			ID    string            `json:"id"`
			Name  string            `json:"name"`
			Human bool              `json:"human"`
			Cards []json.RawMessage `json:"cards"`
		} `json:"columns"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(raw, &js); err != nil {
		t.Fatalf("CA-288: JSONBytes es JSON: %v\n%s", err, raw)
	}
	var ids []string
	for _, c := range js.Columns {
		ids = append(ids, c.ID)
		if (c.ID == ColTuAprobacion || c.ID == ColTuAceptacion) != c.Human {
			t.Fatalf("CA-288: human solo en las dos humanas: %+v", c)
		}
	}
	if strings.Join(ids, ",") != "backlog,arquitecto,tu-aprobacion,test-writer,writer,review,tu-aceptacion,hecho" {
		t.Fatalf("CA-288: el orden de las columnas en JSON: %v", ids)
	}
	if strings.Contains(string(raw), `"cards": null`) || strings.Contains(string(raw), `"cards":null`) ||
		strings.Contains(string(raw), `"warnings": null`) || strings.Contains(string(raw), `"warnings":null`) {
		t.Fatalf("CA-288: ninguna lista del tablero es null:\n%s", raw)
	}
}
