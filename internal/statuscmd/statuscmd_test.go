// Tests adversariales del spec .hoom/specs/eventos-vivos-y-status-watch.md
// (CA-74..CA-80): la ventana del arbitro muestra lo que puede probar, rotula
// lo que no sabe y jamas escribe nada.
package statuscmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

func initProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	body := "schema: hoom/v1\nproject: demo\ngates:\n" +
		"  test:\n    required: true\n    cmd: \"true\"\n"
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("config", "user.email", "test@hoom.dev")
	git("config", "user.name", "hoom test")
	git("add", "-A")
	git("commit", "-m", "inicial")
	return dir
}

func verify(t *testing.T, dir string) {
	t.Helper()
	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := verifycmd.Run(m, verifycmd.Options{}); err != nil {
		t.Fatal(err)
	}
}

// CA-74: el snapshot reune check, ultimo veredicto, verify en curso, runs,
// tareas y hallazgos; el texto rotula cada seccion.
func TestCA74_SnapshotCompleto(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	if _, err := finding.Add(dir, "main", "high", "risk", "", "hallazgo de prueba", "tester"); err != nil {
		t.Fatal(err)
	}
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !s.Check.OK {
		t.Fatalf("CA-74: check verde esperado: %+v", s.Check)
	}
	if s.Last == nil || s.Last.Verdict != "green" || s.Last.Partial {
		t.Fatalf("CA-74: ultimo veredicto verde esperado: %+v", s.Last)
	}
	if s.Live != nil {
		t.Fatalf("CA-74: sin corrida en curso (verify_end emitido): %+v", s.Live)
	}
	if s.Findings.Open != 1 || s.Findings.OpenHigh != 1 {
		t.Fatalf("CA-74: 1 hallazgo abierto high esperado: %+v", s.Findings)
	}
	var buf bytes.Buffer
	Render(&buf, s, false)
	for _, sec := range []string{"check:", "veredicto:", "verify:", "runs:", "tareas:", "hallazgos:"} {
		if !strings.Contains(buf.String(), sec) {
			t.Fatalf("CA-74: falta la seccion %q:\n%s", sec, buf.String())
		}
	}
}

// CA-75: --json emite el mismo snapshot como JSON valido con sus secciones.
func TestCA75_JSONMismoDato(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	raw, err := JSONBytes(s)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-75: JSON invalido: %v", err)
	}
	for _, key := range []string{"check", "last_verdict", "runs", "tasks", "findings"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("CA-75: falta la clave %q en el JSON: %s", key, raw)
		}
	}
}

// CA-76: --watch sin TTY imprime UNA vez, sin escapes ANSI, y avisa.
func TestCA76_WatchSinTTY(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	var buf bytes.Buffer
	if err := Run(dir, "main", &buf, Options{Watch: true, TTY: false}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "\033") {
		t.Fatalf("CA-76: sin TTY no puede haber ANSI:\n%q", out)
	}
	if !strings.Contains(out, "necesita una terminal") {
		t.Fatalf("CA-76: falta el aviso de TTY:\n%s", out)
	}
	if strings.Count(out, "hoomAI - status") != 1 {
		t.Fatalf("CA-76: debe renderizar exactamente una vez:\n%s", out)
	}
}

// CA-77: con eventos de una corrida sin cerrar, el status muestra el estado
// vivo de cada gate (terminado y corriendo) derivado solo de los eventos.
func TestCA77_VerifyEnCurso(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	w := live.NewWriter(dir)
	now := time.Now().UTC()
	w.Emit(live.Event{TS: now.Add(-40 * time.Second), Kind: live.KindVerifyStart, Gates: 2})
	w.Emit(live.Event{TS: now.Add(-39 * time.Second), Kind: live.KindGateStart, Gate: "test", Scope: "full"})
	w.Emit(live.Event{TS: now.Add(-30 * time.Second), Kind: live.KindGateEnd, Gate: "test", Status: "pass", DurationMS: 9000})
	w.Emit(live.Event{TS: now.Add(-29 * time.Second), Kind: live.KindGateStart, Gate: "mutation", Scope: "full"})
	w.Close()

	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Live == nil || s.LiveOrphan {
		t.Fatalf("CA-77: corrida en curso reciente, no huerfana: live=%+v orphan=%v", s.Live, s.LiveOrphan)
	}
	var running string
	for _, g := range s.Live.GateViews {
		if g.Status == "" {
			running = g.Name
		}
	}
	if running != "mutation" {
		t.Fatalf("CA-77: mutation debe figurar corriendo: %+v", s.Live.GateViews)
	}
	var buf bytes.Buffer
	Render(&buf, s, false)
	if !strings.Contains(buf.String(), "corriendo") || !strings.Contains(buf.String(), "mutation") {
		t.Fatalf("CA-77: el render debe mostrar el gate vivo:\n%s", buf.String())
	}
}

// CA-78: corrida sin verify_end y sin actividad reciente => posible huerfano.
func TestCA78_PosibleHuerfano(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	w := live.NewWriter(dir)
	old := time.Now().UTC().Add(-live.OrphanAfter - time.Minute)
	w.Emit(live.Event{TS: old, Kind: live.KindVerifyStart, Gates: 1})
	w.Emit(live.Event{TS: old.Add(time.Second), Kind: live.KindGateStart, Gate: "test", Scope: "full"})
	w.Close()

	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Live == nil || !s.LiveOrphan {
		t.Fatalf("CA-78: esperaba posible huerfano: live=%+v orphan=%v", s.Live, s.LiveOrphan)
	}
	var buf bytes.Buffer
	Render(&buf, s, false)
	if !strings.Contains(buf.String(), "posible huerfano") {
		t.Fatalf("CA-78: el render debe rotular el posible huerfano:\n%s", buf.String())
	}
}

func writeRun(t *testing.T, dir, id string, evs []providers.Event) {
	t.Helper()
	runs := filepath.Join(dir, ".hoom", "runs")
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, ev := range evs {
		raw, _ := json.Marshal(ev)
		b.Write(raw)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(runs, id+".jsonl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// CA-79: rol real cuando el provider lo reporto; "sin delegacion visible"
// cuando no hay datos — jamas un rol inventado.
func TestCA79_RolesSoloConDatos(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	now := time.Now().UTC()
	writeRun(t, dir, "20260830T010101_aaa", []providers.Event{
		{TS: now.Add(-time.Minute), Kind: "start", Detail: "run 20260830T010101_aaa: claude en el proyecto"},
		{TS: now.Add(-30 * time.Second), Kind: "agent", Agent: "hoom-test-writer", Detail: "delegacion"},
		{TS: now.Add(-5 * time.Second), Kind: "tool", Detail: "Bash: go test"},
	})
	writeRun(t, dir, "20260830T010102_bbb", []providers.Event{
		{TS: now.Add(-time.Minute), Kind: "start", Detail: "run 20260830T010102_bbb: opencode en el proyecto"},
		{TS: now.Add(-10 * time.Second), Kind: "text", Detail: "trabajando"},
	})
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Runs) != 2 {
		t.Fatalf("CA-79: dos runs activos esperados: %+v", s.Runs)
	}
	byID := map[string]RunView{}
	for _, r := range s.Runs {
		byID[r.ID] = r
	}
	if r := byID["20260830T010101_aaa"]; r.Role != "hoom-test-writer" || r.Provider != "claude" {
		t.Fatalf("CA-79: rol y provider reales esperados: %+v", r)
	}
	if r := byID["20260830T010102_bbb"]; r.Role != "" {
		t.Fatalf("CA-79: sin datos de delegacion el rol queda vacio: %+v", r)
	}
	var buf bytes.Buffer
	Render(&buf, s, false)
	if !strings.Contains(buf.String(), "rol: hoom-test-writer") || !strings.Contains(buf.String(), "sin delegacion visible") {
		t.Fatalf("CA-79: el render debe distinguir rol real de ausencia de datos:\n%s", buf.String())
	}
}

func hashTree(t *testing.T, root string) string {
	t.Helper()
	var lines []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		lines = append(lines, fmt.Sprintf("%s %x", rel, sha256.Sum256(raw)))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return fmt.Sprintf("%x", sum)
}

// CA-80: status es SOLO LECTURA: .hoom queda byte a byte identico.
func TestCA80_SoloLectura(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	hoomDir := filepath.Join(dir, ".hoom")
	before := hashTree(t, hoomDir)
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	Render(&buf, s, true)
	if _, err := JSONBytes(s); err != nil {
		t.Fatal(err)
	}
	if err := Run(dir, "main", &buf, Options{Watch: true, TTY: false}); err != nil {
		t.Fatal(err)
	}
	if after := hashTree(t, hoomDir); after != before {
		t.Fatalf("CA-80: status modifico .hoom (hash %s -> %s)", before, after)
	}
}
