// Tests adversariales del spec .hoom/specs/costo-por-spec.md (CA-543,
// CA-546, CA-547, CA-548) contra el BINARIO: `hoom costo [<slug>] [--json]`
// solo lee y sale con 0; un slug sin runs ni reviews sale con 1 y el texto
// del contrato; un pedido que no entiende sale con 2; la ayuda general lista
// costo. Escritos DESDE EL SPEC y el contrato publico de costcmd, sin leer
// la implementacion.
//
// Los fixtures escriben lo que hoom deja en disco (sidecars, registros,
// items); ningun provider corre. Los que compilan el binario se omiten con
// -short; el de la ayuda (usage) no.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

var cpsT0 = time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)

// cpsRun escribe el sidecar de un run terminado en dir/.hoom/runs.
func cpsRun(t *testing.T, dir, id, task, role, prov string, ini time.Time, u *providers.Usage, vs ...runcmd.VentanaRun) {
	t.Helper()
	raw, err := json.MarshalIndent(runcmd.Meta{ID: id, Provider: prov, Role: role, Task: task, Dir: dir,
		CreatedAt: ini, Status: runcmd.StatusDone, EndedAt: ini.Add(10 * time.Minute), Usage: u, Ventanas: vs}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, ".hoom", "runs", id+".meta.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// cpsProyecto arma un proyecto real con dos tareas: alfa (aceptada, runs de
// claude y codex, una ronda de review) y beta (en curso, un run de codex).
func cpsProyecto(t *testing.T) string {
	t.Helper()
	dir := vdtRepo(t, "")
	costo := 0.5
	r5 := time.Unix(1794078000, 0).UTC()
	cpsRun(t, dir, "20261107T100000_a00001", "alfa", "writer", "claude", cpsT0,
		&providers.Usage{InputTokens: 100, CachedTokens: 1000, OutputTokens: 10, CostUSD: &costo, Turns: 1},
		runcmd.VentanaRun{Nombre: "5h", Inicio: 10, Fin: 12, ReiniciaEn: r5})
	cpsRun(t, dir, "20261107T110000_a00002", "alfa", "reviewer", "codex", cpsT0.Add(time.Hour),
		&providers.Usage{InputTokens: 1200, CachedTokens: 800, OutputTokens: 50, ReasoningTokens: 20, Turns: 1},
		runcmd.VentanaRun{Nombre: "5h", Inicio: 40, Fin: 43, ReiniciaEn: r5, Plan: "plus"})
	if _, err := reviewcmd.WriteRecord(dir, reviewcmd.Record{ID: "20261107T111500_a00003", CreatedAt: cpsT0.Add(75 * time.Minute),
		Task: "alfa", Spec: ".hoom/specs/alfa.md", Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: []string{"risk"},
		Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}, Usage: []reviewcmd.LensUsage{},
		Runs: []reviewcmd.LensRun{{Lens: "risk", Run: "20261107T110000_a00002"}}, Cobertura: reviewcmd.CoberturaCompleta, Base: "main"}); err != nil {
		t.Fatal(err)
	}
	if _, err := item.Add(dir, item.Draft{Titulo: "Alfa", Slug: "alfa"}); err != nil {
		t.Fatal(err)
	}
	if ok, err := item.MarkDone(dir, "alfa", "abc1234", cpsT0.Add(2*time.Hour)); err != nil || !ok {
		t.Fatalf("fixture: MarkDone: %v %v", ok, err)
	}
	cpsRun(t, dir, "20261107T130000_b00001", "beta", "writer", "codex", cpsT0.Add(3*time.Hour),
		&providers.Usage{InputTokens: 900, CachedTokens: 400, OutputTokens: 9, Turns: 1})
	if err := os.MkdirAll(filepath.Join(dir, ".hoom", "worktrees", "beta"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// cpsFoto es el contenido de todo el proyecto (sin .git): ruta -> sha256.
func cpsFoto(t *testing.T, dir string) map[string]string {
	t.Helper()
	foto := map[string]string{}
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == ".git" {
			return filepath.SkipDir
		}
		if fi.IsDir() {
			foto[rel+"/"] = "dir"
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		h := sha256.Sum256(raw)
		foto[rel] = hex.EncodeToString(h[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return foto
}

func cpsDiferencia(a, b map[string]string) []string {
	var out []string
	for k, v := range a {
		if b[k] != v {
			out = append(out, k)
		}
	}
	for k := range b {
		if _, ok := a[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// CA-543: `hoom costo` sin slug sale con 0 e imprime una linea por tarea,
// la mas reciente primero, con su estado.
func TestCA543_HoomCostoSinSlugImprimeLasTareas(t *testing.T) {
	bin := qcHoom(t)
	dir := cpsProyecto(t)
	s := qcCorrer(t, bin, dir, nil, "costo")
	if s.exit != 0 {
		t.Fatalf("CA-543: hoom costo sale con 0: %d\n%s", s.exit, s.todo())
	}
	i, j := strings.Index(s.stdout, "beta"), strings.Index(s.stdout, "alfa")
	if i < 0 || j < 0 || i > j {
		t.Fatalf("CA-543: una linea por tarea, la mas reciente (beta) primero:\n%s", s.stdout)
	}
	for _, quiero := range []string{"aceptada 2026-11-07 12:00 UTC", "en curso", "rondas 1", "nuevos 100", "nuevos 400",
		"0.50 USD equivalente API", "codex no informa costo", "(ventana de la cuenta durante los runs)"} {
		if !strings.Contains(s.stdout, quiero) {
			t.Fatalf("CA-543: la lista dice %q:\n%s", quiero, s.stdout)
		}
	}
	if s := qcCorrer(t, bin, dir, nil, "costo", "alfa"); s.exit != 0 || !strings.Contains(s.stdout, "razonamiento 20") ||
		!strings.Contains(s.stdout, "lentes risk") || !strings.Contains(s.stdout, "abc1234") {
		t.Fatalf("CA-544: hoom costo alfa sale con 0 e imprime el desglose: %d\n%s", s.exit, s.todo())
	}
}

// CA-547: `hoom costo --json` imprime JSON valido con tareas; con slug, esa
// tarea sola.
func TestCA547_HoomCostoJSON(t *testing.T) {
	bin := qcHoom(t)
	dir := cpsProyecto(t)
	for _, c := range []struct {
		args   []string
		slugs  []string
		estado map[string]string
	}{
		{[]string{"costo", "--json"}, []string{"beta", "alfa"}, map[string]string{"alfa": costcmd.EstadoAceptada, "beta": costcmd.EstadoEnCurso}},
		{[]string{"costo", "alfa", "--json"}, []string{"alfa"}, map[string]string{"alfa": costcmd.EstadoAceptada}},
		{[]string{"costo", "--json", "beta"}, []string{"beta"}, map[string]string{"beta": costcmd.EstadoEnCurso}},
	} {
		s := qcCorrer(t, bin, dir, nil, c.args...)
		if s.exit != 0 {
			t.Fatalf("CA-547: hoom %v sale con 0: %d\n%s", c.args, s.exit, s.todo())
		}
		var m struct {
			Tareas []map[string]any `json:"tareas"`
		}
		if err := json.Unmarshal([]byte(s.stdout), &m); err != nil {
			t.Fatalf("CA-547: hoom %v imprime JSON valido en stdout: %v\n%s", c.args, err, s.stdout)
		}
		var slugs []string
		for _, ta := range m.Tareas {
			slugs = append(slugs, fmt.Sprint(ta["slug"]))
			if ta["estado"] != c.estado[fmt.Sprint(ta["slug"])] {
				t.Fatalf("CA-547: el estado de %v: %v", ta["slug"], ta)
			}
		}
		if strings.Join(slugs, ",") != strings.Join(c.slugs, ",") {
			t.Fatalf("CA-547: hoom %v trae las tareas %v: %v", c.args, c.slugs, slugs)
		}
	}
}

// CA-546: un slug sin runs ni reviews sale con 1 y dice el texto del
// contrato en stderr.
func TestCA546_HoomCostoSlugSinNadaSaleConUno(t *testing.T) {
	bin := qcHoom(t)
	dir := cpsProyecto(t)
	for _, args := range [][]string{{"costo", "nada"}, {"costo", "nada", "--json"}} {
		s := qcCorrer(t, bin, dir, nil, args...)
		quiero := fmt.Sprintf(costcmd.ErrSinNada, "nada")
		if s.exit != 1 || !strings.Contains(s.stderr, quiero) {
			t.Fatalf("CA-546: hoom %v sale con 1 y dice %q en stderr: %d\n%s", args, quiero, s.exit, s.todo())
		}
	}
}

// CA-546 (caso limite): un sidecar ilegible se cuenta y el comando sale con
// 0 igual.
func TestCA546_HoomCostoConSidecarIlegibleSaleConCero(t *testing.T) {
	bin := qcHoom(t)
	dir := cpsProyecto(t)
	if err := os.WriteFile(filepath.Join(dir, ".hoom", "runs", "20261107T150000_c00001.meta.json"), []byte("{basura"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"costo"}, {"costo", "alfa"}} {
		s := qcCorrer(t, bin, dir, nil, args...)
		if s.exit != 0 || !strings.Contains(s.todo(), "1 runs ilegibles") {
			t.Fatalf("CA-546: hoom %v con un sidecar ilegible sale con 0 y lo cuenta: %d\n%s", args, s.exit, s.todo())
		}
	}
}

// CA-543: un pedido que hoom costo no entiende (flag desconocido, dos slugs,
// un slug con otra forma) es exit 2; -h es la ayuda con exit 0.
func TestCA543_HoomCostoArgumentosInvalidosSaleConDos(t *testing.T) {
	bin := qcHoom(t)
	dir := cpsProyecto(t)
	for _, args := range [][]string{{"costo", "--bogus"}, {"costo", "alfa", "beta"}, {"costo", "No Es Slug"}, {"costo", "--json", "a", "b"}} {
		if s := qcCorrer(t, bin, dir, nil, args...); s.exit != 2 {
			t.Fatalf("CA-543: hoom %v es un error de uso (exit 2): %d\n%s", args, s.exit, s.todo())
		}
	}
	s := qcCorrer(t, bin, dir, nil, "costo", "-h")
	if s.exit != 0 || !strings.Contains(s.stdout, costcmd.UsageText) {
		t.Fatalf("CA-543: hoom costo -h imprime el uso y sale con 0: %d\n%s", s.exit, s.todo())
	}
}

// CA-543 (siempre): hoom costo solo lee: el proyecto queda byte a byte
// igual despues de cada forma, la que sale con 1 incluida.
func TestCA543_HoomCostoSoloLee(t *testing.T) {
	bin := qcHoom(t)
	dir := cpsProyecto(t)
	antes := cpsFoto(t, dir)
	for _, args := range [][]string{{"costo"}, {"costo", "alfa"}, {"costo", "--json"}, {"costo", "beta", "--json"}, {"costo", "nada"}} {
		qcCorrer(t, bin, dir, nil, args...)
		if d := cpsDiferencia(antes, cpsFoto(t, dir)); len(d) != 0 {
			t.Fatalf("CA-543: hoom %v solo lee: cambio %v", args, d)
		}
	}
}

// CA-548: la ayuda general lista costo con una linea que dice que el
// movimiento de ventana es el de la cuenta durante los runs.
func TestCA548_LaAyudaListaCosto(t *testing.T) {
	var linea string
	for _, l := range strings.Split(usage, "\n") {
		if strings.HasPrefix(l, "  costo ") {
			linea = l
			break
		}
	}
	if linea == "" || !strings.Contains(linea, "ventana de la cuenta durante los runs") {
		t.Fatalf("CA-548: la ayuda tiene una linea '  costo ...' que dice 'ventana de la cuenta durante los runs': %q\n%s", linea, usage)
	}
}

// CA-548: `hoom help` imprime esa linea.
func TestCA548_HoomHelpListaCosto(t *testing.T) {
	bin := qcHoom(t)
	dir := vdtRepo(t, "")
	s := qcCorrer(t, bin, dir, nil, "help")
	hallada := false
	for _, l := range strings.Split(s.todo(), "\n") {
		if strings.HasPrefix(l, "  costo ") && strings.Contains(l, "ventana de la cuenta durante los runs") {
			hallada = true
		}
	}
	if s.exit != 0 || !hallada {
		t.Fatalf("CA-548: hoom help sale con 0 y lista costo con la ventana de la cuenta: %d\n%s", s.exit, s.todo())
	}
}
