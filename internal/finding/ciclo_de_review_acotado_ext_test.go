// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// lo que de finding necesita registros de review y items (CA-624, CA-627):
// ContextoDe lee el riesgo del item y las reviews completa de la tarea, y el
// gate findings_open falla por el medium de una completa de una tarjeta de
// riesgo alto, no por el de un delta. Paquete externo: reviewcmd importa
// finding. Escritos desde el spec y las firmas publicas, sin leer la
// implementacion.
package finding_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func crxGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func crxRepo(t *testing.T) string {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	crxGit(t, root, "init", "-b", "main")
	crxGit(t, root, "config", "user.email", "rev@hoom.dev")
	crxGit(t, root, "config", "user.name", "Reviewer")
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".hoom", "specs", "t.md"), []byte("# Spec t\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	crxGit(t, root, "add", "-A")
	crxGit(t, root, "commit", "-m", "inicial")
	return root
}

func crxAlta(t *testing.T, root, sev, file, task string) finding.Finding {
	t.Helper()
	f, err := finding.Register(root, "main", finding.Draft{Severity: sev, Lens: "risk", File: file, Description: "h " + sev, Author: "reviewer@test", Task: task})
	if err != nil {
		t.Fatalf("fixture: Register: %v", err)
	}
	return f
}

var crxSeq int

// crxRegistro escribe un registro de review de task con la cobertura y los
// hallazgos dados.
func crxRegistro(t *testing.T, root, task, cobertura string, hallazgos []string) reviewcmd.Record {
	t.Helper()
	crxSeq++
	cuando := time.Date(2026, 11, 10, 10, crxSeq, 0, 0, time.UTC)
	r, err := reviewcmd.WriteRecord(root, reviewcmd.Record{CreatedAt: cuando, Task: task, Spec: ".hoom/specs/" + task + ".md",
		Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: reviewcmd.Lentes, Provider: "codex", Writer: "claude",
		Cross: reviewcmd.CrossYes, Findings: hallazgos, Usage: []reviewcmd.LensUsage{}, Cobertura: cobertura, Base: "main",
		Desde: strings.Repeat("a", 40), Hasta: strings.Repeat("b", 40)})
	if err != nil {
		t.Fatalf("fixture: WriteRecord: %v", err)
	}
	return r
}

func crxItem(t *testing.T, root, slug, riesgo string) {
	t.Helper()
	if _, err := item.Add(root, item.Draft{Titulo: "Tarea " + slug, Slug: slug, Riesgo: riesgo}); err != nil {
		t.Fatalf("fixture: item.Add: %v", err)
	}
}

// CA-624: ContextoDe trae el riesgo del item de cada tarea, las completa (y
// no los delta ni los parcial) con sus hallazgos, y las subidas validas.
func TestCA624_ContextoDeLeeRiesgoYCompletas(t *testing.T) {
	root := crxRepo(t)
	crxItem(t, root, "t", "alto")
	crxItem(t, root, "u", "")
	m := crxAlta(t, root, "medium", "app.go", "t")
	c1 := crxRegistro(t, root, "t", reviewcmd.CoberturaCompleta, []string{m.ID})
	crxRegistro(t, root, "t", reviewcmd.CoberturaDelta, []string{"otro"})
	crxRegistro(t, root, "t", reviewcmd.CoberturaParcial, []string{"otro2"})
	if _, _, err := finding.Raise(root, m.ID, "porque si"); err != nil {
		t.Fatal(err)
	}
	c, err := finding.ContextoDe(root, root, "high")
	if err != nil {
		t.Fatalf("CA-624: ContextoDe: %v", err)
	}
	if c.BlockOn != "high" || c.Riesgos["t"] != "alto" || (c.Riesgos["u"] != "normal" && c.Riesgos["u"] != "") {
		t.Fatalf("CA-624: umbral y riesgos: %+v", c)
	}
	if len(c.Completas) != 1 || c.Completas[0].ID != c1.ID || c.Completas[0].Task != "t" ||
		len(c.Completas[0].Findings) != 1 || c.Completas[0].Findings[0] != m.ID {
		t.Fatalf("CA-624: solo las completa, con sus hallazgos: %+v", c.Completas)
	}
	if s, ok := c.Subidas[m.ID]; !ok || s.Reason != "porque si" || len(c.Subidas) != 1 {
		t.Fatalf("CA-624: las subidas validas: %+v", c.Subidas)
	}
	// sin .hoom/findings no es error
	root2 := crxRepo(t)
	if _, err := finding.ContextoDe(root2, root2, ""); err != nil {
		t.Fatalf("CA-624: sin hallazgos ContextoDe no falla: %v", err)
	}
}

// CA-624/CA-627: con una tarjeta de riesgo alto, el gate con el spec de la
// tarea falla por el medium de una completa, con el motivo, y no por uno que
// figura solo en un delta; con normal y bajo no falla por ninguno.
func TestCA624_GateRiesgoAltoMediumDeLaCompleta(t *testing.T) {
	for _, r := range []string{"alto", "normal", "bajo", ""} {
		root := crxRepo(t)
		crxItem(t, root, "t", r)
		enCompleta := crxAlta(t, root, "medium", "app.go", "t")
		enDelta := crxAlta(t, root, "medium", "app.go", "t")
		rc := crxRegistro(t, root, "t", reviewcmd.CoberturaCompleta, []string{enCompleta.ID})
		crxRegistro(t, root, "t", reviewcmd.CoberturaDelta, []string{enDelta.ID})
		g := finding.Gate(root, "main", "high", ".hoom/specs/t.md")
		if r != "alto" {
			if g.Status != verdict.StatusPass {
				t.Fatalf("CA-624: con riesgo %q ningun medium bloquea: %s %s\n%s", r, g.Status, g.Notes, g.OutputTail)
			}
			continue
		}
		if g.Status != verdict.StatusFail || !strings.HasPrefix(g.Notes, "1 abiertos que bloquean") ||
			!strings.Contains(g.Notes, enCompleta.ID) || strings.Contains(g.Notes, enDelta.ID) {
			t.Fatalf("CA-624: con riesgo alto bloquea solo el de la completa: %s %q", g.Status, g.Notes)
		}
		if !strings.Contains(g.OutputTail, "medium de la review completa "+rc.ID+": riesgo alto") {
			t.Fatalf("CA-627: el detalle nombra el motivo del caso 4:\n%s", g.OutputTail)
		}
	}
}

// CA-624 (caso limite: el riesgo cambia despues de una review): el gate
// recalcula con el item de ahora. De alto a bajo, el medium deja de
// bloquear; de bajo a alto, pasa a bloquear.
func TestCA624_ElRiesgoCambiaDespuesDeLaReview(t *testing.T) {
	root := crxRepo(t)
	crxItem(t, root, "t", "bajo")
	m := crxAlta(t, root, "medium", "app.go", "t")
	crxRegistro(t, root, "t", reviewcmd.CoberturaCompleta, []string{m.ID})
	if g := finding.Gate(root, "main", "high", ".hoom/specs/t.md"); g.Status != verdict.StatusPass {
		t.Fatalf("CA-624: con bajo no bloquea: %s %s", g.Status, g.Notes)
	}
	if _, _, err := item.SetRiesgo(root, "t", "alto"); err != nil {
		t.Fatal(err)
	}
	if g := finding.Gate(root, "main", "high", ".hoom/specs/t.md"); g.Status != verdict.StatusFail || !strings.Contains(g.Notes, m.ID) {
		t.Fatalf("CA-624: de bajo a alto el medium de la completa pasa a bloquear: %s %s", g.Status, g.Notes)
	}
	if _, _, err := item.SetRiesgo(root, "t", "bajo"); err != nil {
		t.Fatal(err)
	}
	if g := finding.Gate(root, "main", "high", ".hoom/specs/t.md"); g.Status != verdict.StatusPass {
		t.Fatalf("CA-624: de alto a bajo deja de bloquear: %s %s", g.Status, g.Notes)
	}
}

// CA-628: ListConUmbral juzga con el riesgo y las completa del arbol.
func TestCA628_ListaJuzgaConElRiesgoDelItem(t *testing.T) {
	root := crxRepo(t)
	crxItem(t, root, "t", "alto")
	m := crxAlta(t, root, "medium", "app.go", "t")
	rc := crxRegistro(t, root, "t", reviewcmd.CoberturaCompleta, []string{m.ID})
	items, _, err := finding.ListConUmbral(root, "main", "high", true)
	if err != nil || len(items) != 1 || !items[0].Bloquea || items[0].Motivo != "medium de la review completa "+rc.ID+": riesgo alto" {
		t.Fatalf("CA-628: %+v %v", items, err)
	}
}
