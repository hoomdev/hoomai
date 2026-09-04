// Tests adversariales de los specs gates-ausentes-parciales-verifica.md
// (CA-63), eventos-vivos-y-status-watch.md (CA-71, CA-72, CA-73) y
// trinquete-ratchet.md (CA-90, CA-95, CA-98): veredictos parciales
// marcados, narracion viva que jamas toca la evidencia, y el trinquete
// viviendo solo en corridas --full completas.
package verifycmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/ratchet"
	"github.com/hoomdev/hoomai/internal/verdict"
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

func writeRatchet(t *testing.T, dir, cmd string, value *float64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, ".hoom"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &ratchet.File{Schema: ratchet.Schema, Metrics: map[string]*ratchet.Metric{
		"cobertura": {Cmd: cmd, Direction: "up", Value: value},
	}}
	if err := f.Save(dir); err != nil {
		t.Fatal(err)
	}
}

func gateNamed(v *verdict.Verdict, name string) *verdict.GateResult {
	for i := range v.Gates {
		if v.Gates[i].Name == name {
			return &v.Gates[i]
		}
	}
	return nil
}

// CA-90: la primera corrida --full congela la base con nota y registro.
func TestCA90_PrimerFullCongela(t *testing.T) {
	m := loadProject(t)
	writeRatchet(t, m.Dir, "echo 73.4", nil)
	v, _, err := Run(m, Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	g := gateNamed(v, "ratchet")
	if g == nil || g.Status != verdict.StatusPass || !strings.Contains(g.OutputTail, "congelada") {
		t.Fatalf("CA-90: esperaba gate ratchet pass con nota de congelado: %+v", g)
	}
	f, err := ratchet.Load(m.Dir)
	if err != nil || f == nil {
		t.Fatal(err)
	}
	mt := f.Metrics["cobertura"]
	if mt.Value == nil || *mt.Value != 73.4 {
		t.Fatalf("CA-90: la base debia congelarse en 73.4: %+v", mt)
	}
	if last := f.History[len(f.History)-1]; last.Kind != "frozen" {
		t.Fatalf("CA-90: falta el registro frozen: %+v", last)
	}
}

// CA-95: el trinquete solo existe en corridas --full sin --gate.
func TestCA95_SoloEnFullCompleto(t *testing.T) {
	m := loadProject(t)
	base := 70.0
	writeRatchet(t, m.Dir, "echo 99", &base)

	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gateNamed(v, "ratchet") != nil {
		t.Fatalf("CA-95: un verify normal no incluye el gate ratchet: %+v", v.Gates)
	}
	v, _, err = Run(m, Options{Full: true, Gates: []string{"test"}})
	if err != nil {
		t.Fatal(err)
	}
	if gateNamed(v, "ratchet") != nil {
		t.Fatalf("CA-95: una corrida parcial jamas corre el trinquete: %+v", v.Gates)
	}
	f, _ := ratchet.Load(m.Dir)
	if *f.Metrics["cobertura"].Value != 70 {
		t.Fatalf("CA-95: la base no debe moverse fuera de --full completo: %+v", f.Metrics["cobertura"])
	}

	v, _, err = Run(m, Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if gateNamed(v, "ratchet") == nil {
		t.Fatalf("CA-95: en --full completo el gate debe existir: %+v", v.Gates)
	}
}

// CA-98: la base esta fuera de la huella: apretar durante --full deja el
// check VERDE.
func TestCA98_ApreteNoRompeElCheck(t *testing.T) {
	m := initRepo(t)
	base := 50.0
	writeRatchet(t, m.Dir, "echo 80", &base)
	before := gitx.Snapshot(m.Dir, "main").ChangeFingerprint
	v, _, err := Run(m, Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if v.Verdict != "green" {
		t.Fatalf("CA-98: la mejora debia dar verde: %+v", v.Summary)
	}
	f, _ := ratchet.Load(m.Dir)
	if *f.Metrics["cobertura"].Value != 80 {
		t.Fatalf("CA-98: la base debia apretarse a 80: %+v", f.Metrics["cobertura"])
	}
	if after := gitx.Snapshot(m.Dir, "main").ChangeFingerprint; after != before {
		t.Fatalf("CA-98: apretar la base cambio la huella: %q vs %q", before, after)
	}
	res, err := checkcmd.Run(m.Dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK {
		t.Fatalf("CA-98: el check debe quedar VERDE tras el apriete: %+v", res)
	}
}

func writeSpecFile(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, ".hoom", "specs")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p, "item.md")
	body := "# Spec\n## Objetivo\nx\n## No-goals\nx\n## Contratos\nx\n## Casos limite\nx\n" +
		"## Criterios de aceptacion\n- CA-1: x. [verifica: true]\n## Decisiones\nx\n## Riesgos\nx\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// CA-99: spec aprobado y vigente => gate spec_approved PASS con quien,
// cuando y sha en la nota.
func TestCA99_AprobadoVigente(t *testing.T) {
	m := loadProject(t)
	path := writeSpecFile(t, m.Dir)
	rec, _, err := approval.Approve(m.Dir, path)
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := Run(m, Options{Spec: path})
	if err != nil {
		t.Fatal(err)
	}
	g := gateNamed(v, "spec_approved")
	if g == nil || g.Status != verdict.StatusPass {
		t.Fatalf("CA-99: esperaba spec_approved PASS: %+v", g)
	}
	if !strings.Contains(g.Notes, "aprobado por") || !strings.Contains(g.Notes, rec.SHA256[:8]) {
		t.Fatalf("CA-99: la nota debe traer autor y sha: %q", g.Notes)
	}
}

// CA-100: spec sin aprobacion => FAIL con la accion exacta y veredicto ROJO.
func TestCA100_SinAprobacionEsRojo(t *testing.T) {
	m := loadProject(t)
	path := writeSpecFile(t, m.Dir)
	v, _, err := Run(m, Options{Spec: path})
	if err != nil {
		t.Fatal(err)
	}
	g := gateNamed(v, "spec_approved")
	if g == nil || g.Status != verdict.StatusFail {
		t.Fatalf("CA-100: esperaba spec_approved FAIL: %+v", g)
	}
	if !strings.Contains(g.OutputTail, "hoom spec approve") {
		t.Fatalf("CA-100: falta la accion exacta: %q", g.OutputTail)
	}
	if v.Verdict != "red" {
		t.Fatalf("CA-100: el veredicto debe ser rojo: %+v", v.Summary)
	}
}

// CA-101: editar el spec tras aprobar invalida; re-aprobar lo devuelve a PASS.
func TestCA101_EdicionInvalida(t *testing.T) {
	m := loadProject(t)
	path := writeSpecFile(t, m.Dir)
	if _, _, err := approval.Approve(m.Dir, path); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	f.WriteString("\n<!-- editado tras aprobar -->\n")
	f.Close()

	v, _, err := Run(m, Options{Spec: path})
	if err != nil {
		t.Fatal(err)
	}
	g := gateNamed(v, "spec_approved")
	if g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, "invalidada") {
		t.Fatalf("CA-101: esperaba FAIL por aprobacion invalidada: %+v", g)
	}

	if _, _, err := approval.Approve(m.Dir, path); err != nil {
		t.Fatal(err)
	}
	v, _, err = Run(m, Options{Spec: path})
	if err != nil {
		t.Fatal(err)
	}
	if g := gateNamed(v, "spec_approved"); g == nil || g.Status != verdict.StatusPass {
		t.Fatalf("CA-101: re-aprobar debe devolver PASS: %+v", g)
	}
}

// CA-102: sin --spec el gate no existe.
func TestCA102_SinSpecNoHayGate(t *testing.T) {
	m := loadProject(t)
	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gateNamed(v, "spec_approved") != nil {
		t.Fatalf("CA-102: sin --spec no debe existir spec_approved: %+v", v.Gates)
	}
}

// CA-103: spec_approved narra eventos vivos y cuenta en el total declarado.
func TestCA103_EventosDelGate(t *testing.T) {
	m := loadProject(t)
	path := writeSpecFile(t, m.Dir)
	if _, _, err := Run(m, Options{Spec: path}); err != nil {
		t.Fatal(err)
	}
	evs := readLiveEvents(t, m.Dir)
	if evs[0].Kind != live.KindVerifyStart || evs[0].Gates != len(m.Gates)+3 {
		t.Fatalf("CA-103: verify_start debe declarar gates+3 (lint, trace, approved): %+v", evs[0])
	}
	var starts, ends int
	for _, ev := range evs {
		if ev.Gate == "spec_approved" {
			switch ev.Kind {
			case live.KindGateStart:
				starts++
			case live.KindGateEnd:
				ends++
			}
		}
	}
	if starts != 1 || ends != 1 {
		t.Fatalf("CA-103: esperaba un gate_start y un gate_end de spec_approved: starts=%d ends=%d", starts, ends)
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
