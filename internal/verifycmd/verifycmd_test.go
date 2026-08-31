// Tests adversariales de los specs gates-ausentes-parciales-verifica.md
// (CA-63) y eventos-vivos-y-status-watch.md (CA-71, CA-72, CA-73): veredictos
// parciales marcados, y la narracion viva que jamas toca la evidencia.
package verifycmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
)

func loadProject(t *testing.T) *manifest.Manifest {
	t.Helper()
	dir := t.TempDir()
	body := "schema: hoom/v1\nproject: demo\ngates:\n" +
		"  test:\n    required: true\n    cmd: \"true\"\n" +
		"  build:\n    required: true\n    cmd: \"true\"\n"
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// CA-63: --gate marca partial:true y agrega la nota de diagnostico.
func TestCA63_GateSeleccionadoMarcaParcial(t *testing.T) {
	m := loadProject(t)
	v, path, err := Run(m, Options{Gates: []string{"test"}})
	if err != nil {
		t.Fatal(err)
	}
	if !v.Partial || !v.IsPartial() {
		t.Fatalf("CA-63: un veredicto de --gate debe ser parcial: %+v", v)
	}
	nota := strings.Join(v.Notes, "\n")
	if !strings.Contains(nota, "PARCIAL") || !strings.Contains(nota, "--gate test") {
		t.Fatalf("CA-63: falta la nota de diagnostico: %q", nota)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "\"partial\": true") {
		t.Fatalf("CA-63: el artefacto debe llevar partial:true: %s", raw)
	}
}

func readLiveEvents(t *testing.T, dir string) []live.Event {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", "cache", live.FileName))
	if err != nil {
		t.Fatal(err)
	}
	var out []live.Event
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var ev live.Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("CA-71: linea de evento invalida %q: %v", line, err)
		}
		out = append(out, ev)
	}
	return out
}

// CA-71: verify narra la corrida a .hoom/cache/verify-live.jsonl (start,
// gate_start/gate_end por gate, end con veredicto) y trunca por corrida.
func TestCA71_EventosVivosYTruncado(t *testing.T) {
	m := loadProject(t)
	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	evs := readLiveEvents(t, m.Dir)
	if evs[0].Kind != live.KindVerifyStart || evs[0].Gates != 2 {
		t.Fatalf("CA-71: el primer evento debe ser verify_start con el total de gates: %+v", evs[0])
	}
	kinds := map[string]int{}
	for _, ev := range evs {
		kinds[ev.Kind]++
		if ev.Schema != live.Schema {
			t.Fatalf("CA-71: evento sin schema: %+v", ev)
		}
	}
	if kinds[live.KindGateStart] != 2 || kinds[live.KindGateEnd] != 2 {
		t.Fatalf("CA-71: esperaba start/end por cada gate: %v", kinds)
	}
	last := evs[len(evs)-1]
	if last.Kind != live.KindVerifyEnd || last.Verdict != "green" || last.VerdictID != v.ID || last.Partial {
		t.Fatalf("CA-71: verify_end debe traer veredicto e id: %+v", last)
	}

	// Segunda corrida (--gate): el archivo se TRUNCA y narra solo la nueva.
	if _, _, err := Run(m, Options{Gates: []string{"test"}}); err != nil {
		t.Fatal(err)
	}
	evs = readLiveEvents(t, m.Dir)
	if evs[0].Kind != live.KindVerifyStart {
		t.Fatalf("CA-71: tras truncar, el primer evento debe ser verify_start: %+v", evs[0])
	}
	if got := evs[len(evs)-1]; !got.Partial {
		t.Fatalf("CA-71: la corrida --gate debe narrar verify_end parcial: %+v", got)
	}
	if kinds := len(evs); kinds >= 10 {
		t.Fatalf("CA-71: el archivo acumulo corridas anteriores (%d eventos): no trunco", kinds)
	}
}

// CA-72: cache no escribible => la narracion se pierde, el veredicto no.
func TestCA72_CacheRotoNoTocaElVeredicto(t *testing.T) {
	m := loadProject(t)
	// .hoom/cache como ARCHIVO: MkdirAll y Create fallan si o si.
	if err := os.MkdirAll(filepath.Join(m.Dir, ".hoom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, ".hoom", "cache"), []byte("bloqueo"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, path, err := Run(m, Options{})
	if err != nil {
		t.Fatalf("CA-72: verify debe completar sin narracion: %v", err)
	}
	if v.Verdict != "green" || path == "" {
		t.Fatalf("CA-72: mismo veredicto que con narracion: %+v", v)
	}
}

func initRepo(t *testing.T) *manifest.Manifest {
	t.Helper()
	m := loadProject(t)
	git := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = m.Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("init", "-b", "main")
	git("config", "user.email", "test@hoom.dev")
	git("config", "user.name", "hoom test")
	git("add", "-A")
	git("commit", "-m", "inicial")
	return m
}

// CA-73: narrar no altera la huella ni rompe un check verde.
func TestCA73_EventosFueraDeLaHuella(t *testing.T) {
	m := initRepo(t)
	before := gitx.Snapshot(m.Dir, "main").ChangeFingerprint
	if _, _, err := Run(m, Options{}); err != nil {
		t.Fatal(err)
	}
	after := gitx.Snapshot(m.Dir, "main").ChangeFingerprint
	if before != after {
		t.Fatalf("CA-73: la huella cambio por narrar eventos: %q vs %q", before, after)
	}
	res, err := checkcmd.Run(m.Dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("CA-73: el check debe quedar verde tras narrar: %+v", res)
	}
}

// CA-63 (control): un verify completo NO lleva la marca.
func TestCA63_VerifyCompletoNoEsParcial(t *testing.T) {
	m := loadProject(t)
	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if v.Partial || v.IsPartial() {
		t.Fatalf("CA-63: un verify completo no es parcial: %+v", v)
	}
	if v.Verdict != "green" {
		t.Fatalf("CA-63: gates 'true' deberian dar verde: %+v", v.Summary)
	}
}
