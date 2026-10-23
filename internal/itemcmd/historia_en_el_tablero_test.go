// Tests adversariales del spec .hoom/specs/historia-en-el-tablero.md sobre el
// verbo `hoom item backfill [--dry-run] [--json]` (CA-446..CA-455): crea el
// item con historico de cada tarea con evidencia y sin item que califica,
// lista las que no con su motivo y no toca las que ya tienen item; con
// --dry-run no escribe nada; argumentos de mas son exit 2 sin efectos. El
// proyecto de prueba imita el caso real de hoomai (CA-454): specs con su
// verde aprobado e integrado (en main, desde una rama con merge, con el spec
// editado despues), una sin spec_approved, una con su verde solo en una rama
// sin integrar, una tarea de correccion sin spec con sus hallazgos cerrados y
// otra con un high abierto, y tareas que ya tienen item. Repos git reales en
// t.TempDir(); veredictos, hallazgos y specs escritos a mano; el binario
// real para los exit codes. Sin ningun CLI de IA. Todo identificador de este
// archivo lleva el prefijo ht.
package itemcmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
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

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	htMotivoVeredicto = "sin veredicto verde aprobado en la base"
	htUsoBackfill     = "hoom item backfill [--dry-run] [--json]"
)

var htT0 = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

// htRepo: proyecto git con hoom.yaml (un gate barato, block_on high), el
// .gitignore canonico de .hoom (como deja 'hoom init') y un commit inicial.
func htRepo(t *testing.T, extraYAML string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cbGit(t, root, "init", "-b", "main")
	cbGit(t, root, "config", "user.email", "test@hoom.dev")
	cbGit(t, root, "config", "user.name", "hoom test")
	icEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+extraYAML)
	icEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	icEscribir(t, root, "app.go", "package app\n")
	htCommitear(t, root, "inicial")
	return root
}

func htCommitear(t *testing.T, dir, msg string) {
	t.Helper()
	cbGit(t, dir, "add", "-A")
	cbGit(t, dir, "commit", "-q", "-m", msg)
}

// htWorktree crea la tarea como 'hoom task start': rama hoom/<slug> y
// worktree en .hoom/worktrees/<slug>, desde main.
func htWorktree(t *testing.T, root, slug string) string {
	t.Helper()
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	cbGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	return wt
}

// htSpec escribe en dir un spec valido de slug con ese primer encabezado.
func htSpec(t *testing.T, dir, slug, encabezado string) {
	t.Helper()
	icEscribir(t, dir, ".hoom/specs/"+slug+".md", encabezado+"\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n"+
		"## Casos limite\nx\n\n## Criterios de aceptacion\n\n- CA-1: algo. [verifica: true]\n\n## Decisiones\nx\n\n## Riesgos\nx\n")
}

func htAprobar(t *testing.T, dir, slug string) {
	t.Helper()
	if _, _, err := approval.Approve(dir, filepath.Join(dir, ".hoom", "specs", slug+".md")); err != nil {
		t.Fatalf("precondicion: aprobar el spec de %s en %s: %v", slug, dir, err)
	}
}

func htGatesVerdes() []verdict.GateResult {
	return []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "spec_trace", Required: true, Status: verdict.StatusPass},
		{Name: "spec_approved", Required: true, Status: verdict.StatusPass},
		{Name: "findings_open", Required: true, Status: verdict.StatusPass},
		{Name: "test", Required: true, Status: verdict.StatusPass},
	}
}

func htGatesSinAprobacion() []verdict.GateResult {
	var out []verdict.GateResult
	for _, g := range htGatesVerdes() {
		if g.Name != "spec_approved" {
			out = append(out, g)
		}
	}
	return out
}

func htGatesRojos() []verdict.GateResult {
	out := htGatesVerdes()
	out[len(out)-1].Status = verdict.StatusFail
	return out
}

// htVeredicto escribe en dir un veredicto completo del spec de slug con el
// estado git de dir (su HEAD es el git.commit), y exige su color.
func htVeredicto(t *testing.T, dir, slug string, at time.Time, gates []verdict.GateResult, verde bool) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: at, Spec: ".hoom/specs/" + slug + ".md",
		Git: gitx.Snapshot(dir, "main"), Gates: gates}
	v.Finalize()
	if (v.Verdict == "green") != verde || v.IsPartial() {
		t.Fatalf("precondicion: el veredicto de %s es completo y verde=%v; fue %s parcial=%v", slug, verde, v.Verdict, v.IsPartial())
	}
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	return v
}

func htHallazgo(t *testing.T, root, task, sev string) string {
	t.Helper()
	f, err := finding.Register(root, "main", finding.Draft{Severity: sev, Lens: "risk",
		Description: "hallazgo " + sev + " de " + task, Author: "reviewer", Task: task})
	if err != nil {
		t.Fatalf("precondicion: registrar un hallazgo %s de %s: %v", sev, task, err)
	}
	return f.ID
}

func htCerrar(t *testing.T, root, id, as string) {
	t.Helper()
	if _, err := finding.Resolve(root, id, as, "TestHT lo cierra con evidencia", "hoom test <test@hoom.dev>"); err != nil {
		t.Fatalf("precondicion: cerrar %s como %s: %v", id, as, err)
	}
}

// htHallazgoConTarea registra un hallazgo y reescribe a mano su "task", como
// llegaria por git sin pasar por Register.
func htHallazgoConTarea(t *testing.T, root, tarea string) {
	t.Helper()
	f, err := finding.Register(root, "main", finding.Draft{Severity: "low", Lens: "risk", Description: "hostil " + tarea,
		Author: "reviewer", Task: "temporal"})
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".hoom", "findings", f.ID+".json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m["task"] != "temporal" {
		t.Fatalf("precondicion: el JSON del hallazgo trae \"task\": %v %s", err, raw)
	}
	m["task"] = tarea
	out, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(p, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// htFoto fotografia TODO el arbol salvo .git/ y los .hoom/cache/ (el cache
// local regenerable, donde hoom deja sus candados): ignorados incluidos
// (worktrees, findings, verdicts, items). Contenido, modo y mtime.
func htFoto(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if rel == ".git" || (d.Name() == "cache" && filepath.Base(filepath.Dir(p)) == ".hoom") {
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

// htDiff devuelve lo creado, lo modificado y lo borrado entre dos fotos.
func htDiff(antes, despues map[string]string) (creados, modificados, borrados []string) {
	for k, v := range antes {
		if w, ok := despues[k]; !ok {
			borrados = append(borrados, k)
		} else if w != v {
			modificados = append(modificados, k)
		}
	}
	for k := range despues {
		if _, ok := antes[k]; !ok {
			creados = append(creados, k)
		}
	}
	sort.Strings(creados)
	sort.Strings(modificados)
	sort.Strings(borrados)
	return
}

// htSinCambios exige que el arbol no haya cambiado en nada.
func htSinCambios(t *testing.T, ca, que string, antes, despues map[string]string) {
	t.Helper()
	c, m, b := htDiff(antes, despues)
	if len(c)+len(m)+len(b) > 0 {
		t.Fatalf("%s: %s no escribe nada; creo %v, modifico %v, borro %v", ca, que, c, m, b)
	}
}

// htCaso es lo que el proyecto espera de una tarea.
type htCaso struct {
	historico string // califica: el historico del item
	tipo      string
	titulo    string
	motivo    string // no califica: lo que su motivo tiene que decir
}

// htEsp es lo que se espera del proyecto de prueba.
type htEsp struct {
	califican   map[string]htCaso
	noCalifican map[string]htCaso
	conItem     map[string]string // slug -> contenido original del item
	seguimiento string            // el hallazgo low abierto de estabilizar-ci
}

func htSlugs(m map[string]htCaso) []string {
	out := []string{}
	for s := range m {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

func (e htEsp) sinItem() []string {
	out := append(htSlugs(e.califican), htSlugs(e.noCalifican)...)
	sort.Strings(out)
	return out
}

// htProyecto arma el proyecto que imita hoomai.
func htProyecto(t *testing.T) (string, htEsp) {
	t.Helper()
	root := htRepo(t, "findings:\n  block_on: high\n")
	esp := htEsp{califican: map[string]htCaso{}, noCalifican: map[string]htCaso{}, conItem: map[string]string{}}
	en := func(h int) time.Time { return htT0.Add(time.Duration(h) * time.Hour) }

	// precios: spec aprobado y su verde en main
	htSpec(t, root, "precios", "# Spec: Precios por region")
	htAprobar(t, root, "precios")
	// catalogo: dos verdes integrados y un rojo despues; gana el verde mas nuevo
	htSpec(t, root, "catalogo", "# Spec: Catalogo con dos verdes")
	htAprobar(t, root, "catalogo")
	// envios: aprobado, verde, y el spec editado despues de integrarlo
	htSpec(t, root, "envios", "# Spec: Envios a domicilio")
	htAprobar(t, root, "envios")
	// pagos-sin-aprobar: su unico verde no tiene spec_approved
	htSpec(t, root, "pagos-sin-aprobar", "# Spec: Pagos sin aprobar")
	// tablero-viejo: ya tiene item
	htSpec(t, root, "tablero-viejo", "# Spec: Tablero viejo")
	itemViejo := icItemYAML("Tablero viejo", "2026-09-02T10:00:00Z", "pedido: el que ya estaba\n")
	icEscribir(t, root, ".hoom/items/tablero-viejo.yaml", itemViejo)
	esp.conItem["tablero-viejo"] = itemViejo
	// item-roto: un item invalido cuenta como item
	htSpec(t, root, "item-roto", "# Spec: Item roto")
	itemRoto := icItemYAML("Item roto", "2026-09-02T10:00:00Z", "columna: hecho\n")
	icEscribir(t, root, ".hoom/items/item-roto.yaml", itemRoto)
	esp.conItem["item-roto"] = itemRoto
	htCommitear(t, root, "specs e items")

	vPrecios := htVeredicto(t, root, "precios", en(1), htGatesVerdes(), true)
	htVeredicto(t, root, "catalogo", en(1), htGatesVerdes(), true)
	vEnvios := htVeredicto(t, root, "envios", en(1), htGatesVerdes(), true)
	htVeredicto(t, root, "pagos-sin-aprobar", en(1), htGatesSinAprobacion(), true)
	htVeredicto(t, root, "tablero-viejo", en(1), htGatesVerdes(), true)
	htVeredicto(t, root, "item-roto", en(1), htGatesVerdes(), true)
	htCommitear(t, root, "primeros veredictos")
	vCatalogo := htVeredicto(t, root, "catalogo", en(2), htGatesVerdes(), true)
	htCommitear(t, root, "segundo verde de catalogo")
	htVeredicto(t, root, "catalogo", en(3), htGatesRojos(), false)
	icEscribir(t, root, ".hoom/specs/envios.md", "# Spec: Envios a domicilio\n\n## Objetivo\nx, editado despues\n\n## No-goals\nx\n\n"+
		"## Contratos\nx\n\n## Casos limite\nx\n\n## Criterios de aceptacion\n\n- CA-1: algo. [verifica: true]\n\n## Decisiones\nx\n\n## Riesgos\nx\n")
	htCommitear(t, root, "rojo de catalogo y envios editado")

	// estabilizar-ci: tarea de correccion sin spec, sus bloqueantes cerrados
	// y un seguimiento low abierto
	htCerrar(t, root, htHallazgo(t, root, "estabilizar-ci", "high"), finding.StatusCorrected)
	htCerrar(t, root, htHallazgo(t, root, "estabilizar-ci", "medium"), finding.StatusRefuted)
	esp.seguimiento = htHallazgo(t, root, "estabilizar-ci", "low")
	// fixes-c4: uno cerrado y un high abierto
	htCerrar(t, root, htHallazgo(t, root, "fixes-c4", "high"), finding.StatusRefuted)
	abierto := htHallazgo(t, root, "fixes-c4", "high")
	htCommitear(t, root, "hallazgos de las correcciones")

	// carrito: trabajada en su rama e integrada con merge, sin "Spec: "
	wt := htWorktree(t, root, "carrito")
	htSpec(t, wt, "carrito", "# Carrito de compras")
	htAprobar(t, wt, "carrito")
	icEscribir(t, wt, "carrito.go", "package app\n")
	htCommitear(t, wt, "carrito: spec y codigo")
	vCarrito := htVeredicto(t, wt, "carrito", en(4), htGatesVerdes(), true)
	htCommitear(t, wt, "carrito: veredicto")
	cbGit(t, root, "merge", "--no-ff", "-q", "-m", "Merge hoom/carrito", "hoom/carrito")
	cbGit(t, root, "worktree", "remove", wt)

	// cupones-en-rama: su verde vive solo en su rama, nunca integrada
	wt = htWorktree(t, root, "cupones-en-rama")
	htSpec(t, wt, "cupones-en-rama", "# Spec: Cupones en su rama")
	htAprobar(t, wt, "cupones-en-rama")
	icEscribir(t, wt, "cupones.go", "package app\n")
	htCommitear(t, wt, "cupones: spec y codigo")
	htVeredicto(t, wt, "cupones-en-rama", en(5), htGatesVerdes(), true)
	htCommitear(t, wt, "cupones: veredicto")

	esp.califican["precios"] = htCaso{historico: "veredicto " + vPrecios.ID, tipo: "feature", titulo: "Precios por region"}
	esp.califican["catalogo"] = htCaso{historico: "veredicto " + vCatalogo.ID, tipo: "feature", titulo: "Catalogo con dos verdes"}
	esp.califican["envios"] = htCaso{historico: "veredicto " + vEnvios.ID, tipo: "feature", titulo: "Envios a domicilio"}
	esp.califican["carrito"] = htCaso{historico: "veredicto " + vCarrito.ID, tipo: "feature", titulo: "Carrito de compras"}
	esp.califican["estabilizar-ci"] = htCaso{historico: item.HistoricoHallazgos, tipo: "bug", titulo: "estabilizar-ci"}
	esp.noCalifican["pagos-sin-aprobar"] = htCaso{motivo: htMotivoVeredicto}
	esp.noCalifican["cupones-en-rama"] = htCaso{motivo: htMotivoVeredicto}
	esp.noCalifican["fixes-c4"] = htCaso{motivo: abierto}
	return root, esp
}

// htDoctorSinItem son las tareas con evidencia-sin-item del doctor.
func htDoctorSinItem(t *testing.T, root string) []string {
	t.Helper()
	r, err := boardcmd.Doctor(root, "main", "high", time.Now().UTC())
	if err != nil {
		t.Fatalf("Doctor sobre el proyecto: %v", err)
	}
	out := []string{}
	for _, p := range r.Problems {
		if p.ID == boardcmd.ProbEvidenciaSinItem {
			out = append(out, p.Slug)
		}
	}
	sort.Strings(out)
	return out
}

func htCandSlugs(cs []boardcmd.Candidata) []string {
	out := []string{}
	for _, c := range cs {
		out = append(out, c.Slug)
	}
	sort.Strings(out)
	return out
}

// htResultado exige que el resultado de Backfill sea el del proyecto: los
// que califican con su historico, tipo y titulo; los que no con su motivo;
// y los que ya tienen item.
func htResultado(t *testing.T, ca string, res BackfillResult, esp htEsp) {
	t.Helper()
	if got := htCandSlugs(res.Creados); !reflect.DeepEqual(got, htSlugs(esp.califican)) {
		t.Fatalf("%s: creados (o que crearia) %v, fueron %v", ca, htSlugs(esp.califican), got)
	}
	for _, c := range res.Creados {
		w := esp.califican[c.Slug]
		if c.Historico != w.historico || c.Tipo != w.tipo || c.Titulo != w.titulo || c.Motivo != "" || c.ConItem {
			t.Fatalf("%s: %s califica con historico %q, tipo %q, titulo %q; fue %+v", ca, c.Slug, w.historico, w.tipo, w.titulo, c)
		}
	}
	if got := htCandSlugs(res.NoCalifican); !reflect.DeepEqual(got, htSlugs(esp.noCalifican)) {
		t.Fatalf("%s: no califican %v, fueron %v", ca, htSlugs(esp.noCalifican), got)
	}
	for _, c := range res.NoCalifican {
		if w := esp.noCalifican[c.Slug]; c.Historico != "" || !strings.Contains(c.Motivo, w.motivo) {
			t.Fatalf("%s: %s no califica y su motivo dice %q; fue %+v", ca, c.Slug, w.motivo, c)
		}
	}
	got := append([]string{}, res.ConItem...)
	sort.Strings(got)
	want := []string{}
	for s := range esp.conItem {
		want = append(want, s)
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CA-453: con item %v, fueron %v", want, got)
	}
}

// htLineas parte la salida en lineas (sin la ultima vacia).
func htLineas(out string) []string {
	return strings.Split(strings.TrimRight(out, "\n"), "\n")
}

// htLineaDe devuelve la unica linea (despues del resumen) que nombra slug.
func htLineaDe(t *testing.T, ca string, lineas []string, slug string) string {
	t.Helper()
	var hay []string
	for _, l := range lineas[1:] {
		if strings.Contains(l, slug) {
			hay = append(hay, l)
		}
	}
	if len(hay) != 1 {
		t.Fatalf("%s: una linea por tarea: %s aparece en %d lineas:\n%s", ca, slug, len(hay), strings.Join(lineas, "\n"))
	}
	return hay[0]
}

// htTexto exige el texto del contrato: una linea de resumen "hoom item
// backfill: ..." y despues una linea por tarea.
func htTexto(t *testing.T, ca, out string, esp htEsp, dryRun bool) {
	t.Helper()
	lineas := htLineas(out)
	if !strings.HasPrefix(lineas[0], "hoom item backfill:") {
		t.Fatalf("%s: la primera linea es el resumen \"hoom item backfill: ...\":\n%s", ca, out)
	}
	n := len(esp.califican) + len(esp.noCalifican) + len(esp.conItem)
	if len(lineas) != 1+n {
		t.Fatalf("%s: el resumen y una linea por tarea (%d), fueron %d lineas:\n%s", ca, 1+n, len(lineas), out)
	}
	verbo, otro := "creado", "crearia"
	if dryRun {
		verbo, otro = "crearia", "creado"
	}
	for s, w := range esp.califican {
		l := htLineaDe(t, ca, lineas, s)
		if !strings.Contains(l, verbo) || strings.Contains(l, otro) || !strings.Contains(l, "historico: "+w.historico) {
			t.Fatalf("%s: la linea de %s dice %q y \"historico: %s\": %q", ca, s, verbo, w.historico, l)
		}
	}
	for s, w := range esp.noCalifican {
		l := htLineaDe(t, ca, lineas, s)
		if !strings.Contains(l, "no crea") || !strings.Contains(l, w.motivo) || strings.Contains(l, "historico:") {
			t.Fatalf("%s: la linea de %s dice \"no crea\" y su motivo (%q): %q", ca, s, w.motivo, l)
		}
	}
	for s := range esp.conItem {
		l := htLineaDe(t, ca, lineas, s)
		if !strings.Contains(l, "intacto") {
			t.Fatalf("CA-453: la linea de %s, que ya tiene item, dice \"intacto\": %q", s, l)
		}
	}
}

func htExec(t *testing.T, root string, args ...string) (string, string, error) {
	t.Helper()
	req, err := Parse(args)
	if err != nil {
		t.Fatalf("Parse(%q) no debia fallar: %v", args, err)
	}
	var out, errb bytes.Buffer
	err = Execute(root, "main", "high", req, &out, &errb)
	return out.String(), errb.String(), err
}

// CA-455: Parse acepta backfill con --dry-run y --json en cualquier orden, y
// rechaza (*cliargs.UsageError, exit 2, con el uso de backfill) cualquier
// argumento de mas: posicionales, flags desconocidos o de otro subcomando.
// CA-447: ningun flag de 'item add' escribe historico.
func TestCA455_ParseBackfill(t *testing.T) {
	for _, c := range []struct {
		args        []string
		dryRun, js_ bool
	}{
		{[]string{"backfill"}, false, false},
		{[]string{"backfill", "--dry-run"}, true, false},
		{[]string{"backfill", "--json"}, false, true},
		{[]string{"backfill", "--dry-run", "--json"}, true, true},
		{[]string{"backfill", "--json", "--dry-run"}, true, true},
	} {
		req, err := Parse(c.args)
		if err != nil || req.Sub != SubBackfill || req.DryRun != c.dryRun || req.JSON != c.js_ {
			t.Fatalf("CA-455: Parse(%q) es backfill con dry-run=%v json=%v: %+v %v", c.args, c.dryRun, c.js_, req, err)
		}
	}
	for _, args := range [][]string{
		{"backfill", "x"}, {"backfill", "precios"}, {"backfill", "--dry-run", "x"}, {"backfill", "x", "--json"},
		{"backfill", "--", "x"}, {"backfill", "--bogus"}, {"backfill", "--slug", "precios"}, {"backfill", "--tipo", "bug"},
		{"backfill", "--historico", "hallazgos"}, {"backfill", "--auto", "hasta-humano"}, {"backfill", "--force"},
	} {
		_, err := Parse(args)
		ue := icUso(t, "CA-455", args, err)
		if !strings.Contains(ue.Error(), htUsoBackfill) {
			t.Fatalf("CA-455: el rechazo de %q lleva el uso de backfill (%q):\n%s", args, htUsoBackfill, ue.Error())
		}
	}
	for _, args := range [][]string{{"backfill", "-h"}, {"backfill", "--help"}} {
		if _, err := Parse(args); !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-455: %q es la ayuda (cliargs.ErrHelp), fue %v", args, err)
		}
	}
	_, err := Parse([]string{"add", "Algo", "--historico", "hallazgos"})
	icUso(t, "CA-447", []string{"add", "Algo", "--historico", "hallazgos"}, err)
}

// La lectura (--dry-run) se mira entera sobre UN proyecto: es read-only, asi
// que el texto, el JSON y el resultado salen de la misma foto.
func TestCA446_BackfillDryRunNoEscribe(t *testing.T) {
	root, esp := htProyecto(t)
	if doc := htDoctorSinItem(t, root); !reflect.DeepEqual(doc, esp.sinItem()) {
		t.Fatalf("precondicion: el doctor da evidencia-sin-item de %v, dio %v", esp.sinItem(), doc)
	}
	antes := htFoto(t, root)
	head := cbHead(t, root)
	nada := func(t *testing.T, ca, que string) {
		t.Helper()
		htSinCambios(t, ca, que, antes, htFoto(t, root))
		if h := cbHead(t, root); h != head {
			t.Fatalf("%s: %s no commitea: HEAD %s -> %s", ca, que, head, h)
		}
	}

	// CA-446: Backfill con dryRun lista lo que crearia (con su historico) y
	// lo que no (con su motivo), no escribe nada (ni items ni commits), y lo
	// que mira es exactamente la lista del evidencia-sin-item del doctor.
	// CA-450: sin block_on el umbral es high: el mismo resultado.
	t.Run("CA-446 Backfill con dryRun", func(t *testing.T) {
		for _, blockOn := range []string{"high", ""} {
			ca := "CA-446 (block_on " + blockOn + ")"
			res, err := Backfill(root, "main", blockOn, true, time.Now().UTC())
			if err != nil {
				t.Fatalf("%s: Backfill --dry-run: %v", ca, err)
			}
			nada(t, ca, "backfill --dry-run")
			if !res.DryRun {
				t.Fatalf("%s: el resultado dice que fue --dry-run: %+v", ca, res)
			}
			htResultado(t, ca, res, esp)
			vistas := append(htCandSlugs(res.Creados), htCandSlugs(res.NoCalifican)...)
			sort.Strings(vistas)
			if doc := htDoctorSinItem(t, root); !reflect.DeepEqual(vistas, doc) {
				t.Fatalf("%s: --dry-run lista cada tarea de evidencia-sin-item:\ndoctor:   %v\nbackfill: %v", ca, doc, vistas)
			}
		}
	})

	// CA-446: el texto de 'hoom item backfill --dry-run': el resumen y una
	// linea por tarea, "crearia" con su historico, "no crea" con su motivo,
	// "intacto" para la que ya tiene item; y nada escrito.
	t.Run("CA-446 el texto de --dry-run", func(t *testing.T) {
		out, _, err := htExec(t, root, "backfill", "--dry-run")
		if err != nil {
			t.Fatalf("CA-446: backfill --dry-run: %v", err)
		}
		nada(t, "CA-446", "backfill --dry-run")
		htTexto(t, "CA-446", out, esp, true)
	})

	// CA-446: --json emite el BackfillResult con dry_run, creados,
	// no_califican y con_item; con --dry-run tampoco escribe.
	t.Run("CA-446 el JSON de --dry-run", func(t *testing.T) {
		out, _, err := htExec(t, root, "backfill", "--dry-run", "--json")
		if err != nil {
			t.Fatalf("CA-446: backfill --dry-run --json: %v", err)
		}
		nada(t, "CA-446", "backfill --dry-run --json")
		var crudo map[string]json.RawMessage
		if err := json.Unmarshal([]byte(out), &crudo); err != nil {
			t.Fatalf("CA-446: --json es JSON: %v\n%s", err, out)
		}
		for _, k := range []string{"dry_run", "creados", "no_califican", "con_item"} {
			if _, ok := crudo[k]; !ok {
				t.Fatalf("CA-446: --json trae %q:\n%s", k, out)
			}
		}
		var res BackfillResult
		if err := json.Unmarshal([]byte(out), &res); err != nil || !res.DryRun {
			t.Fatalf("CA-446: --json con --dry-run trae dry_run true: %v\n%s", err, out)
		}
		htResultado(t, "CA-446", res, esp)
	})
}

// htItemCreado lee y valida el item que backfill creo para slug.
func htItemCreado(t *testing.T, ca, root, slug string, w htCaso, desde, hasta time.Time) {
	t.Helper()
	raw, err := os.ReadFile(item.Path(root, slug))
	if err != nil {
		t.Fatalf("%s: backfill crea .hoom/items/%s.yaml: %v", ca, slug, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: el item de %s es YAML: %v\n%s", ca, slug, err, raw)
	}
	if m["titulo"] != w.titulo || m["tipo"] != w.tipo || m["prioridad"] != "media" || m["historico"] != w.historico ||
		m["creado_por"] != gitx.Identity(root) {
		t.Fatalf("%s: el item de %s trae titulo %q, tipo %q, prioridad media, historico %q y creado_por %q:\n%s",
			ca, slug, w.titulo, w.tipo, w.historico, gitx.Identity(root), raw)
	}
	for _, k := range []string{"hecho_en", "commit_final", "auto", "presupuesto_usd", "sesiones"} {
		if _, ok := m[k]; ok {
			t.Fatalf("%s: el item historico de %s nunca trae %s:\n%s", ca, slug, k, raw)
		}
	}
	s := icFecha(m["creado_en"])
	creado, err := time.Parse(time.RFC3339, s)
	if err != nil || !strings.HasSuffix(s, "Z") || strings.Contains(s, ".") || creado.Before(desde) || creado.After(hasta) {
		t.Fatalf("%s: creado_en de %s en UTC RFC3339 con segundos, del momento de correrlo: %q (%v)", ca, slug, s, err)
	}
	it, err := item.Load(root, slug)
	if err != nil || it.Historico != w.historico || it.HechoEn != nil || it.CommitFinal != "" {
		t.Fatalf("%s: el item de %s es valido para el tablero y se lee con su historico: %+v %v", ca, slug, it, err)
	}
}

// La escritura se mira sobre UN proyecto y UNA corrida de 'hoom item
// backfill' (texto), y cada subtest afirma un criterio.
func TestCA447_BackfillCreaLoQueCalificaYNadaMas(t *testing.T) {
	root, esp := htProyecto(t)
	antes := htFoto(t, root)
	desde := time.Now().UTC().Truncate(time.Second)
	out, _, err := htExec(t, root, "backfill")
	if err != nil {
		t.Fatalf("CA-447: backfill: %v", err)
	}
	hasta := time.Now().UTC().Add(time.Second)
	creados, modificados, borrados := htDiff(antes, htFoto(t, root))

	// CA-447: backfill crea .hoom/items/<slug>.yaml para cada tarea con spec
	// que califica, con titulo, tipo feature, prioridad media, creado_por (la
	// identidad git), creado_en e historico "veredicto <id>", y sin hecho_en
	// ni commit_final. Lo unico que cambia en el arbol son esos archivos. El
	// texto dice "creado" con el historico.
	t.Run("CA-447 crea el item con su veredicto", func(t *testing.T) {
		htTexto(t, "CA-447", out, esp, false)
		want := []string{}
		for _, s := range htSlugs(esp.califican) {
			want = append(want, filepath.Join(".hoom", "items", s+".yaml"))
		}
		sort.Strings(want)
		if !reflect.DeepEqual(creados, want) || len(modificados) > 0 || len(borrados) > 0 {
			t.Fatalf("CA-447: backfill solo crea los items de las que califican %v; creo %v, modifico %v, borro %v",
				want, creados, modificados, borrados)
		}
		for _, s := range []string{"precios", "catalogo", "envios", "carrito"} {
			htItemCreado(t, "CA-447", root, s, esp.califican[s], desde, hasta)
		}
	})

	// CA-448: para la tarea sin spec que califica, el item lleva historico
	// hallazgos, tipo bug y el slug como titulo; su seguimiento low sigue
	// abierto y visible.
	t.Run("CA-448 sin spec: hallazgos y bug", func(t *testing.T) {
		htItemCreado(t, "CA-448", root, "estabilizar-ci", esp.califican["estabilizar-ci"], desde, hasta)
		abiertos, _, err := finding.List(root, "main", true)
		if err != nil {
			t.Fatal(err)
		}
		visto := false
		for _, f := range abiertos {
			visto = visto || f.Finding.ID == esp.seguimiento
		}
		if !visto {
			t.Fatalf("CA-448: el seguimiento %s de estabilizar-ci sigue abierto despues de backfill", esp.seguimiento)
		}
	})

	// CA-449: la tarea cuyo unico verde no tiene spec_approved, y la que solo
	// tiene su verde en una rama sin integrar, no se crean (ni en el
	// proyecto ni en el espacio de trabajo) y su motivo lo dice (en el
	// texto, que ya miro htTexto).
	t.Run("CA-449 no crea sin veredicto verde aprobado en la base", func(t *testing.T) {
		for _, s := range []string{"pagos-sin-aprobar", "cupones-en-rama"} {
			if _, err := os.Stat(item.Path(root, s)); !os.IsNotExist(err) {
				t.Fatalf("CA-449: %s no califica y no se crea: %v", s, err)
			}
			l := htLineaDe(t, "CA-449", htLineas(out), s)
			if !strings.Contains(l, htMotivoVeredicto) {
				t.Fatalf("CA-449: la linea de %s dice %q: %q", s, htMotivoVeredicto, l)
			}
		}
		if _, err := os.Stat(item.Path(filepath.Join(root, ".hoom", "worktrees", "cupones-en-rama"), "cupones-en-rama")); !os.IsNotExist(err) {
			t.Fatalf("CA-449: tampoco en el espacio de trabajo de la tarea: %v", err)
		}
	})

	// CA-450: la tarea sin spec con un high abierto (block_on high) no se
	// crea y su motivo nombra el hallazgo.
	t.Run("CA-450 no crea con un high abierto", func(t *testing.T) {
		if _, err := os.Stat(item.Path(root, "fixes-c4")); !os.IsNotExist(err) {
			t.Fatalf("CA-450: fixes-c4 no se crea: %v", err)
		}
		l := htLineaDe(t, "CA-450", htLineas(out), "fixes-c4")
		if w := esp.noCalifican["fixes-c4"].motivo; !strings.Contains(l, w) {
			t.Fatalf("CA-450: la linea de fixes-c4 nombra el hallazgo abierto %s: %q", w, l)
		}
	})
}

// CA-453: un item existente con el mismo slug (valido o invalido) no se
// modifica y la salida lo dice; correr backfill otra vez no cambia nada: lo
// que creo la primera vez ahora es "intacto". La primera corrida es --json
// (CA-447: sin --dry-run trae dry_run false y crea).
func TestCA453_BackfillNoTocaItemsExistentes(t *testing.T) {
	root, esp := htProyecto(t)
	fotoItems := func() map[string]string {
		out := map[string]string{}
		for k, v := range htFoto(t, root) {
			if strings.HasPrefix(k, filepath.Join(".hoom", "items")+string(filepath.Separator)) {
				out[k] = v
			}
		}
		return out
	}
	antes := fotoItems()
	out, _, err := htExec(t, root, "backfill", "--json")
	if err != nil {
		t.Fatalf("CA-447: backfill --json: %v", err)
	}
	var res BackfillResult
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.DryRun {
		t.Fatalf("CA-447: --json sin --dry-run trae dry_run false: %v\n%s", err, out)
	}
	htResultado(t, "CA-447", res, esp)
	despues := fotoItems()
	for s, original := range esp.conItem {
		k := filepath.Join(".hoom", "items", s+".yaml")
		if despues[k] != antes[k] {
			t.Fatalf("CA-453: el item existente %s no se toca (ni contenido, ni modo, ni mtime)", s)
		}
		if raw, _ := os.ReadFile(item.Path(root, s)); string(raw) != original {
			t.Fatalf("CA-453: el item existente %s queda byte a byte igual:\n%s", s, raw)
		}
	}
	for s := range esp.califican {
		if _, err := os.Stat(item.Path(root, s)); err != nil {
			t.Fatalf("CA-447: backfill --json crea .hoom/items/%s.yaml: %v", s, err)
		}
	}

	// la segunda vez: todo intacto, nada escrito
	todo := htFoto(t, root)
	out, _, err = htExec(t, root, "backfill")
	if err != nil {
		t.Fatalf("CA-453: backfill otra vez: %v", err)
	}
	htSinCambios(t, "CA-453", "backfill sobre tareas que ya tienen item", todo, htFoto(t, root))
	segunda := htEsp{califican: map[string]htCaso{}, noCalifican: esp.noCalifican, conItem: map[string]string{}}
	for s, v := range esp.conItem {
		segunda.conItem[s] = v
	}
	for s := range esp.califican {
		segunda.conItem[s] = ""
	}
	htTexto(t, "CA-453", out, segunda, false)
	res, err = Backfill(root, "main", "high", true, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Creados) != 0 {
		t.Fatalf("CA-453: la segunda vez no hay nada que crear: %+v", res.Creados)
	}
	htResultado(t, "CA-453", res, segunda)
}

// CA-447 (caso limite): .hoom/items/ no escribible es un error y no deja
// nada a medias: ningun item nuevo.
func TestCA447_BackfillItemsNoEscribibleEsErrorSinEfectos(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root los permisos no impiden escribir")
	}
	root, _ := htProyecto(t)
	dir := filepath.Join(root, ".hoom", "items")
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	antes := htFoto(t, root)
	_, err := Backfill(root, "main", "high", false, time.Now().UTC())
	if err == nil {
		t.Fatal("CA-447: con .hoom/items/ no escribible backfill es un error")
	}
	htSinCambios(t, "CA-447", "backfill sobre .hoom/items/ no escribible", antes, htFoto(t, root))
}

// CA-447 (hostil): una tarea escrita a mano en un hallazgo que no es un slug
// valido nunca crea un archivo: ni fuera de .hoom/items/ ni con otro nombre;
// la tarea valida de al lado si se crea, y nada se ejecuta.
func TestCA447_BackfillTareasHostilesNoCreanNada(t *testing.T) {
	root := htRepo(t, "findings:\n  block_on: high\n")
	for _, h := range []string{"../fuera", "../../fuera2", "../../../fuera3", "a/b", "x;touch PWNED_HT;#", "$(touch PWNED_HT)", "Con Mayus", "-guion"} {
		htHallazgoConTarea(t, root, h)
	}
	htHallazgo(t, root, "valida", "low")
	htCommitear(t, root, "hallazgos hostiles")
	antes := htFoto(t, root)
	padre := htFoto(t, filepath.Dir(root))
	if _, err := Backfill(root, "main", "high", false, time.Now().UTC()); err != nil {
		t.Fatalf("CA-447: backfill sale bien aunque haya tareas que no son slugs: %v", err)
	}
	creados, modificados, borrados := htDiff(antes, htFoto(t, root))
	// el proyecto no tenia .hoom/items/: aparece la carpeta y el item de valida
	want := []string{filepath.Join(".hoom", "items") + "/", filepath.Join(".hoom", "items", "valida.yaml")}
	if !reflect.DeepEqual(creados, want) || len(modificados)+len(borrados) > 0 {
		t.Fatalf("CA-447: solo se crea el item de la tarea valida; creo %v, modifico %v, borro %v", creados, modificados, borrados)
	}
	pc, pm, pb := htDiff(padre, htFoto(t, filepath.Dir(root)))
	var fuera []string
	for _, p := range append(append(pc, pm...), pb...) {
		if !strings.HasPrefix(p, filepath.Base(root)+string(filepath.Separator)) {
			fuera = append(fuera, p)
		}
	}
	if len(fuera) > 0 {
		t.Fatalf("CA-447: backfill no escribe fuera del proyecto: %v", fuera)
	}
}

// htCorrer ejecuta el binario sin HOOM_ROLE, HOOM_RUN, HOOM_PROVIDER ni
// HOOM_TASK heredadas (la suite puede correr dentro de un rol de hoom).
func htCorrer(t *testing.T, bin, dir string, args ...string) ixSalida {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		switch k {
		case "HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER", "HOOM_TASK":
			continue
		}
		cmd.Env = append(cmd.Env, kv)
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no pude ejecutar hoom %v: %v", args, err)
	}
	return ixSalida{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

// CA-455: con el binario, argumentos de mas son exit 2, explicados por
// stderr con el uso, y CERO efectos: el proyecto tiene tareas que califican
// y ningun item aparece. CA-446: sobre el mismo proyecto, --dry-run sale con
// 0, lista cada tarea del evidencia-sin-item del doctor y no escribe nada.
func TestCA455_E2EArgumentosDeMasYDryRunSinEfectos(t *testing.T) {
	bin := ixBinario(t)
	root, esp := htProyecto(t)
	antes := htFoto(t, root)
	head := cbHead(t, root)
	for _, args := range [][]string{
		{"item", "backfill", "x"}, {"item", "backfill", "precios"}, {"item", "backfill", "--dry-run", "x"},
		{"item", "backfill", "x", "--json"}, {"item", "backfill", "--", "x"}, {"item", "backfill", "--bogus"},
		{"item", "backfill", "--slug", "precios"}, {"item", "backfill", "--historico", "hallazgos"},
	} {
		r := htCorrer(t, bin, root, args...)
		if r.exit != 2 {
			t.Fatalf("CA-455: 'hoom %s' es error de uso (exit 2), salio %d\nstdout: %s\nstderr: %s",
				strings.Join(args, " "), r.exit, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stderr, htUsoBackfill) {
			t.Fatalf("CA-455: 'hoom %s' explica el rechazo por stderr con el uso de backfill:\n%s", strings.Join(args, " "), r.stderr)
		}
		htSinCambios(t, "CA-455", "'hoom "+strings.Join(args, " ")+"'", antes, htFoto(t, root))
		if h := cbHead(t, root); h != head {
			t.Fatalf("CA-455: 'hoom %s' no commitea", strings.Join(args, " "))
		}
	}
	if r := htCorrer(t, bin, root, "item", "backfill", "-h"); r.exit != 0 || !strings.Contains(r.stdout, htUsoBackfill) {
		t.Fatalf("CA-455: 'hoom item backfill -h' es el uso con exit 0: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}

	r := htCorrer(t, bin, root, "item", "backfill", "--dry-run")
	if r.exit != 0 {
		t.Fatalf("CA-446: 'hoom item backfill --dry-run' sale 0: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	htSinCambios(t, "CA-446", "'hoom item backfill --dry-run'", antes, htFoto(t, root))
	htTexto(t, "CA-446", r.stdout, esp, true)
	for _, s := range htDoctorSinItem(t, root) {
		if !strings.Contains(r.stdout, s) {
			t.Fatalf("CA-446: --dry-run lista %s, que el doctor da como evidencia-sin-item:\n%s", s, r.stdout)
		}
	}
}

// htProblemas parsea 'hoom board doctor --json'.
func htProblemas(t *testing.T, ca string, r ixSalida) []boardcmd.Problem {
	t.Helper()
	if r.exit != 0 {
		t.Fatalf("%s: 'hoom board doctor --json' sale 0: %d\n%s", ca, r.exit, r.stderr)
	}
	var rep struct {
		Problems []boardcmd.Problem `json:"problems"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("%s: el doctor --json es JSON: %v\n%s", ca, err, r.stdout)
	}
	return rep.Problems
}

func htDeProblema(ps []boardcmd.Problem, id string) []string {
	out := []string{}
	for _, p := range ps {
		if p.ID == id {
			out = append(out, p.Slug)
		}
	}
	sort.Strings(out)
	return out
}

// htTarjetas parsea 'hoom board --json': slug -> tarjeta.
func htTarjetas(t *testing.T, ca string, r ixSalida) map[string]boardcmd.Card {
	t.Helper()
	if r.exit != 0 {
		t.Fatalf("%s: 'hoom board --json' sale 0: %d\n%s", ca, r.exit, r.stderr)
	}
	var b struct {
		Columns []struct {
			ID    string           `json:"id"`
			Cards []map[string]any `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &b); err != nil {
		t.Fatalf("%s: el tablero --json es JSON: %v", ca, err)
	}
	out := map[string]boardcmd.Card{}
	for _, col := range b.Columns {
		for _, m := range col.Cards {
			raw, _ := json.Marshal(m)
			var c boardcmd.Card
			if err := json.Unmarshal(raw, &c); err != nil {
				t.Fatal(err)
			}
			if c.Column != col.ID {
				t.Fatalf("%s: la tarjeta %s esta bajo %s pero dice %s", ca, c.Slug, col.ID, c.Column)
			}
			if h, ok := m["historica"]; ok && h != true && h != false {
				t.Fatalf("%s: historica es un booleano: %v", ca, h)
			}
			out[c.Slug] = c
		}
	}
	return out
}

// CA-454: en un proyecto como hoomai, despues de 'hoom item backfill' (exit
// 0 aunque algunas no califiquen), 'hoom board doctor' no da ningun
// evidencia-sin-item de las que calificaron y si de las que no, y ninguna
// historica-sin-sosten; el tablero las pone en Hecho con la marca historica
// (CA-451) en --json, en el texto y en 'item show'. Despues, si el veredicto
// de una desaparece y se abre un high de otra, esas dejan Hecho y el doctor
// da historica-sin-sosten con su accion (CA-452).
func TestCA454_E2EBackfillEnUnProyectoComoHoomai(t *testing.T) {
	bin := ixBinario(t)
	root, esp := htProyecto(t)
	ps := htProblemas(t, "CA-454", htCorrer(t, bin, root, "board", "doctor", "--json"))
	if got := htDeProblema(ps, boardcmd.ProbEvidenciaSinItem); !reflect.DeepEqual(got, esp.sinItem()) {
		t.Fatalf("precondicion: antes de backfill el doctor da evidencia-sin-item de %v, dio %v", esp.sinItem(), got)
	}
	if r := htCorrer(t, bin, root, "board"); r.exit != 0 || strings.Contains(r.stdout, "historica") {
		t.Fatalf("precondicion: antes de backfill ninguna tarjeta es historica: %d\n%s", r.exit, r.stdout)
	}

	r := htCorrer(t, bin, root, "item", "backfill")
	if r.exit != 0 {
		t.Fatalf("CA-454: 'hoom item backfill' sale 0 aunque haya tareas que no califican: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	htTexto(t, "CA-454", r.stdout, esp, false)

	ps = htProblemas(t, "CA-454", htCorrer(t, bin, root, "board", "doctor", "--json"))
	if got := htDeProblema(ps, boardcmd.ProbEvidenciaSinItem); !reflect.DeepEqual(got, htSlugs(esp.noCalifican)) {
		t.Fatalf("CA-454: despues de backfill el doctor da evidencia-sin-item solo de las que no califican %v, dio %v",
			htSlugs(esp.noCalifican), got)
	}
	if got := htDeProblema(ps, boardcmd.ProbHistoricaSinSosten); len(got) != 0 {
		t.Fatalf("CA-454: lo que backfill crea se sostiene: ninguna historica-sin-sosten, hubo %v", got)
	}

	cards := htTarjetas(t, "CA-451", htCorrer(t, bin, root, "board", "--json"))
	for s := range esp.califican {
		c, ok := cards[s]
		if !ok || c.Column != boardcmd.ColHecho || !c.Historica {
			t.Fatalf("CA-451: %s esta en Hecho con historica: true en --json; fue %+v", s, c)
		}
	}
	if c := cards["tablero-viejo"]; c.Historica {
		t.Fatalf("CA-451: el item que ya existia no es historico: %+v", c)
	}
	txt := htCorrer(t, bin, root, "board")
	if txt.exit != 0 || !strings.Contains(txt.stdout, "historica") {
		t.Fatalf("CA-451: el texto de 'hoom board' marca las historicas: %d\n%s", txt.exit, txt.stdout)
	}
	show := htCorrer(t, bin, root, "item", "show", "precios")
	if show.exit != 0 || !strings.Contains(show.stdout, "historica") {
		t.Fatalf("CA-451: 'hoom item show precios' marca la tarjeta historica: %d\n%s", show.exit, show.stdout)
	}
	show = htCorrer(t, bin, root, "item", "show", "precios", "--json")
	var js struct {
		Card map[string]any `json:"card"`
	}
	if err := json.Unmarshal([]byte(show.stdout), &js); err != nil || js.Card["historica"] != true {
		t.Fatalf("CA-451: 'hoom item show --json' trae historica: true: %v\n%s", err, show.stdout)
	}

	// CA-452: se cae la evidencia de dos historicas
	vid := strings.TrimPrefix(esp.califican["precios"].historico, "veredicto ")
	if err := os.Remove(filepath.Join(root, ".hoom", "verdicts", vid+".json")); err != nil {
		t.Fatalf("precondicion: el veredicto %s vive en .hoom/verdicts: %v", vid, err)
	}
	alto := htHallazgo(t, root, "estabilizar-ci", "high")
	cards = htTarjetas(t, "CA-452", htCorrer(t, bin, root, "board", "--json"))
	for _, s := range []string{"precios", "estabilizar-ci"} {
		if c := cards[s]; c.Column == boardcmd.ColHecho || c.Historica {
			t.Fatalf("CA-452: %s deja Hecho y la marca: %s historica=%v", s, c.Column, c.Historica)
		}
	}
	for _, s := range []string{"catalogo", "envios", "carrito"} {
		if c := cards[s]; c.Column != boardcmd.ColHecho || !c.Historica {
			t.Fatalf("CA-452: %s, cuya evidencia sigue, sigue en Hecho historica: %s historica=%v", s, c.Column, c.Historica)
		}
	}
	ps = htProblemas(t, "CA-452", htCorrer(t, bin, root, "board", "doctor", "--json"))
	if got := htDeProblema(ps, boardcmd.ProbHistoricaSinSosten); !reflect.DeepEqual(got, []string{"estabilizar-ci", "precios"}) {
		t.Fatalf("CA-452: el doctor da historica-sin-sosten de estabilizar-ci y precios, dio %v", got)
	}
	for _, p := range ps {
		if p.ID == boardcmd.ProbHistoricaSinSosten {
			if p.Action == "" {
				t.Fatalf("CA-452: historica-sin-sosten trae su accion: %+v", p)
			}
			id := vid
			if p.Slug == "estabilizar-ci" {
				id = alto
			}
			if !strings.Contains(p.What+" "+p.Ref+" "+p.Action, id) {
				t.Fatalf("CA-452: historica-sin-sosten de %s nombra %s: %+v", p.Slug, id, p)
			}
		}
	}
	doc := htCorrer(t, bin, root, "board", "doctor")
	if doc.exit != 0 || !strings.Contains(doc.stdout, "historica-sin-sosten  precios") ||
		!strings.Contains(doc.stdout, "historica-sin-sosten  estabilizar-ci") {
		t.Fatalf("CA-452: el texto del doctor lista historica-sin-sosten de cada una: %d\n%s", doc.exit, doc.stdout)
	}
}
