// El gasto de hoy de hoom costo (spec tablero-con-paneles, "hoom costo: el
// gasto de hoy"; CA-561): sin slug, la primera linea dice "hoy: <usd> USD en
// <n> trabajos" (con ", <k> sin dato de costo") o "hoy: sin trabajos", y el
// JSON suma "hoy": {"runs", "costo_usd", "sin_costo"}; cuenta los runs que
// empezaron hoy en la zona del reloj; con slug nada cambia. Escrito desde el
// spec y el go doc de costcmd, en un paquete de test externo.
package costcmd_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// hyZona es la zona de los relojes de estos tests: UTC-6, sin horario de
// verano.
var hyZona = time.FixedZone("UTC-6", -6*3600)

// hyAhora es el 9 de noviembre a las 10:00 en UTC-6 (16:00 UTC).
var hyAhora = time.Date(2026, 11, 9, 10, 0, 0, 0, hyZona)

var hySeq int

// hyRun escribe en root el sidecar de un run que empezo en ini.
func hyRun(t *testing.T, root, task string, ini time.Time, costo *float64, status string) {
	t.Helper()
	hySeq++
	id := fmt.Sprintf("%s_%06x", ini.UTC().Format("20060102T150405"), hySeq)
	m := runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: task, Dir: root, CreatedAt: ini, Status: status}
	if status != runcmd.StatusRunning {
		m.EndedAt = ini.Add(time.Minute)
	}
	if costo != nil {
		m.Usage = &providers.Usage{InputTokens: 10, OutputTokens: 1, CostUSD: costo, Turns: 1}
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, ".hoom", "runs", id+".meta.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func hyF(v float64) *float64 { return &v }

// hyProyecto: cinco runs del 9 de noviembre en UTC-6 (uno corriendo, dos sin
// costo) y dos que no son de hoy en UTC-6 aunque lo son en UTC (o al reves).
func hyProyecto(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	d := func(h, m int) time.Time { return time.Date(2026, 11, 9, h, m, 0, 0, hyZona) }
	hyRun(t, root, "alfa", d(0, 5), hyF(0.5), runcmd.StatusDone)
	hyRun(t, root, "alfa", d(9, 0), hyF(1.25), runcmd.StatusDone)
	hyRun(t, root, "beta", d(9, 30), nil, runcmd.StatusDone)
	hyRun(t, root, "beta", d(23, 30), nil, runcmd.StatusError)
	hyRun(t, root, "beta", d(9, 59), nil, runcmd.StatusRunning)
	// 2026-11-09 03:00 UTC es el 8 de noviembre en UTC-6: no es de hoy
	hyRun(t, root, "alfa", time.Date(2026, 11, 9, 3, 0, 0, 0, time.UTC), hyF(9), runcmd.StatusDone)
	// 2026-11-10 06:30 UTC es el 10 en UTC-6 (00:30): tampoco
	hyRun(t, root, "alfa", time.Date(2026, 11, 10, 6, 30, 0, 0, time.UTC), hyF(9), runcmd.StatusDone)
	for _, s := range []string{"alfa", "beta"} {
		if _, err := item.Add(root, item.Draft{Titulo: s, Slug: s}); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// CA-561: HoyEn cuenta los runs cuyo created_at, leido en la zona de ahora,
// cae en la fecha de ahora (los que corren tambien, sin costo todavia), suma
// el costo de los que lo informaron y cuenta los que no.
func TestCA561_HoyEn(t *testing.T) {
	root := hyProyecto(t)
	h, err := costcmd.HoyEn(root, hyAhora)
	if err != nil {
		t.Fatalf("CA-561: HoyEn: %v", err)
	}
	if h.Runs != 5 || h.SinCosto != 3 || h.CostoUSD < 1.749 || h.CostoUSD > 1.751 {
		t.Fatalf("CA-561: hoy en UTC-6: 5 runs, 1.75 USD, 3 sin costo: %+v", h)
	}
	// el mismo instante en UTC es otro dia para otros runs
	u, err := costcmd.HoyEn(root, hyAhora.UTC())
	if err != nil {
		t.Fatal(err)
	}
	// en UTC entra el de las 03:00 (9 USD) y sale el de las 23:30 de UTC-6
	if u.Runs != 5 || u.SinCosto != 2 || u.CostoUSD < 10.749 || u.CostoUSD > 10.751 {
		t.Fatalf("CA-561: el dia se cuenta en la zona del reloj: en UTC el 9 son otros runs (10.75 USD, 2 sin costo): %+v", u)
	}
	// sin .hoom/runs, ceros y sin error
	vacio := t.TempDir()
	if h, err := costcmd.HoyEn(vacio, hyAhora); err != nil || h != (costcmd.Hoy{}) {
		t.Fatalf("CA-561: sin runs, un Hoy de ceros: %+v %v", h, err)
	}
	// un sidecar ilegible no cuenta
	if err := os.WriteFile(filepath.Join(root, ".hoom", "runs", "20261109T150000_dead00.meta.json"), []byte("no es json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if h, err := costcmd.HoyEn(root, hyAhora); err != nil || h.Runs != 5 {
		t.Fatalf("CA-561: un sidecar ilegible no se cuenta: %+v %v", h, err)
	}
}

// CA-561: LineaHoy, para cada forma del spec ("1 trabajos" tambien: literal,
// decision del orquestador).
func TestCA561_LineaHoy(t *testing.T) {
	casos := map[costcmd.Hoy]string{
		{}:                                     "hoy: sin trabajos",
		{Runs: 1, CostoUSD: 0.5}:               "hoy: 0.50 USD en 1 trabajos",
		{Runs: 4, CostoUSD: 3.876}:             "hoy: 3.88 USD en 4 trabajos",
		{Runs: 5, CostoUSD: 1.75, SinCosto: 3}: "hoy: 1.75 USD en 5 trabajos, 3 sin dato de costo",
		{Runs: 2, CostoUSD: 0, SinCosto: 2}:    "hoy: 0.00 USD en 2 trabajos, 2 sin dato de costo",
	}
	for h, q := range casos {
		if got := costcmd.LineaHoy(h); got != q {
			t.Fatalf("CA-561: LineaHoy(%+v) = %q, se esperaba %q", h, got, q)
		}
	}
}

// CA-561: sin slug, el reporte trae Hoy y su texto empieza con LineaHoy; con
// slug, Hoy es nil y el texto no la tiene; el JSON sin slug lleva "hoy" con
// runs, costo_usd y sin_costo despues de "avisos", y con slug no lo lleva.
func TestCA561_ReporteTextoYJSON(t *testing.T) {
	root := hyProyecto(t)
	r, err := costcmd.BuildEn(root, "", hyAhora)
	if err != nil {
		t.Fatalf("CA-561: BuildEn: %v", err)
	}
	h, _ := costcmd.HoyEn(root, hyAhora)
	if r.Hoy == nil || *r.Hoy != h {
		t.Fatalf("CA-561: el reporte sin slug trae Hoy de HoyEn: %+v", r.Hoy)
	}
	var b bytes.Buffer
	costcmd.Render(&b, r, false)
	if primera := strings.SplitN(b.String(), "\n", 2)[0]; primera != costcmd.LineaHoy(h) {
		t.Fatalf("CA-561: la primera linea es %q: %q", costcmd.LineaHoy(h), primera)
	}
	raw, err := costcmd.JSONBytes(r)
	if err != nil {
		t.Fatal(err)
	}
	var j map[string]json.RawMessage
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	var hoy map[string]any
	if err := json.Unmarshal(j["hoy"], &hoy); err != nil || len(hoy) != 3 || hoy["runs"] != float64(5) || hoy["sin_costo"] != float64(3) {
		t.Fatalf("CA-561: \"hoy\": {\"runs\", \"costo_usd\", \"sin_costo\"}: %s (%v)", j["hoy"], err)
	}
	if c, ok := hoy["costo_usd"].(float64); !ok || c < 1.749 || c > 1.751 {
		t.Fatalf("CA-561: costo_usd es un numero: %v", hoy["costo_usd"])
	}
	if strings.Index(string(raw), `"hoy"`) < strings.Index(string(raw), `"avisos"`) {
		t.Fatalf("CA-561: \"hoy\" va despues de \"avisos\":\n%s", raw)
	}
	// sin runs con costo, costo_usd es 0 (no null)
	vacio := t.TempDir()
	rv, err := costcmd.BuildEn(vacio, "", hyAhora)
	if err != nil {
		t.Fatal(err)
	}
	rawv, _ := costcmd.JSONBytes(rv)
	if !strings.Contains(strings.ReplaceAll(string(rawv), " ", ""), `"costo_usd":0`) {
		t.Fatalf("CA-561: sin runs, costo_usd es 0: %s", rawv)
	}
	var bv bytes.Buffer
	costcmd.Render(&bv, rv, false)
	if !strings.HasPrefix(bv.String(), "hoy: sin trabajos\n") {
		t.Fatalf("CA-561: sin runs, la primera linea es 'hoy: sin trabajos' (antes de todo): %q", bv.String())
	}
	// con slug, nada cambia
	rs, err := costcmd.BuildEn(root, "alfa", hyAhora)
	if err != nil {
		t.Fatal(err)
	}
	if rs.Hoy != nil {
		t.Fatalf("CA-561: con slug el reporte no trae Hoy: %+v", rs.Hoy)
	}
	var bs bytes.Buffer
	costcmd.Render(&bs, rs, true)
	if strings.Contains(bs.String(), "hoy:") {
		t.Fatalf("CA-561: con slug no hay linea hoy: %q", bs.String())
	}
	raws, _ := costcmd.JSONBytes(rs)
	if strings.Contains(string(raws), `"hoy"`) {
		t.Fatalf("CA-561: con slug el JSON no tiene hoy: %s", raws)
	}
	// Build es BuildEn con el reloj de la computadora: trae Hoy
	if rb, err := costcmd.Build(root, ""); err != nil || rb.Hoy == nil {
		t.Fatalf("CA-561: Build sin slug trae Hoy: %v %+v", err, rb.Hoy)
	}
}
