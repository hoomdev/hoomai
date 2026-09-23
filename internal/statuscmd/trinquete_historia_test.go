// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-376, la parte de statuscmd): statuscmd.Ratchet es la unica fuente del
// trinquete, y cada metrica trae su `history` —los movimientos de la linea
// base de esa metrica, del mas viejo al mas nuevo, nunca null—. `hoom status
// --json` lo emite y el texto de `hoom status` no cambia.
package statuscmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/ratchet"
)

var tqT0 = time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

// tqBaseline escribe una linea base con movimientos de dos metricas
// intercalados en el history del archivo (en el orden en que ocurrieron) y una
// tercera declarada sin congelar y sin movimientos.
func tqBaseline(t *testing.T, dir string) []ratchet.Change {
	t.Helper()
	f70, f80, f40 := 70.0, 80.0, 40.0
	cob, mut := 78.0, 45.0
	hist := []ratchet.Change{
		{TS: tqT0, Metric: "cobertura", To: 70, Kind: "frozen"},
		{TS: tqT0.Add(1 * time.Hour), Metric: "mutacion", To: 40, Kind: "frozen"},
		{TS: tqT0.Add(24 * time.Hour), Metric: "cobertura", From: &f70, To: 80, Kind: "tightened"},
		{TS: tqT0.Add(48 * time.Hour), Metric: "cobertura", From: &f80, To: 78, Kind: "loosened", Reason: "flaky en CI del proveedor de pagos"},
		{TS: tqT0.Add(72 * time.Hour), Metric: "mutacion", From: &f40, To: 45, Kind: "tightened"},
	}
	f := &ratchet.File{
		Schema: ratchet.Schema,
		Metrics: map[string]*ratchet.Metric{
			"cobertura": {Cmd: "touch pwned", Direction: "up", Value: &cob},
			"deuda":     {Cmd: "touch pwned", Direction: "down"},
			"mutacion":  {Cmd: "touch pwned", Direction: "up", Tolerance: 1, Value: &mut},
		},
		History: hist,
	}
	if err := os.MkdirAll(filepath.Join(dir, ".hoom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(dir); err != nil {
		t.Fatal(err)
	}
	return hist
}

func tqMetric(t *testing.T, v RatchetView, name string) RatchetMetric {
	t.Helper()
	for _, m := range v.Metrics {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("no esta la metrica %q en %+v", name, v.Metrics)
	return RatchetMetric{}
}

// tqIgual compara dos movimientos campo por campo (ts por instante).
func tqIgual(a, b ratchet.Change) bool {
	if !a.TS.Equal(b.TS) || a.Metric != b.Metric || a.To != b.To || a.Kind != b.Kind || a.Reason != b.Reason {
		return false
	}
	if (a.From == nil) != (b.From == nil) {
		return false
	}
	return a.From == nil || *a.From == *b.From
}

// CA-376: Ratchet trae en cada metrica su history: solo sus movimientos, del
// mas viejo al mas nuevo, con ts, metric, from, to, kind y reason; una
// metrica sin movimientos trae una lista vacia, nunca nil. Lo que ya habia
// (base, direccion, ultimo movimiento) no cambia.
func TestCA376_RatchetTraeElHistorialPorMetrica(t *testing.T) {
	dir := initProject(t)
	hist := tqBaseline(t, dir)

	v := Ratchet(dir)
	if !v.Declared || v.Error != "" || len(v.Metrics) != 3 {
		t.Fatalf("CA-376: fixture: tres metricas declaradas y legibles: %+v", v)
	}

	cob := tqMetric(t, v, "cobertura")
	want := []ratchet.Change{hist[0], hist[2], hist[3]}
	if len(cob.History) != len(want) {
		t.Fatalf("CA-376: cobertura tiene %d movimientos en el archivo, history trae %d: %+v", len(want), len(cob.History), cob.History)
	}
	for i := range want {
		if !tqIgual(cob.History[i], want[i]) {
			t.Fatalf("CA-376: cobertura: el movimiento %d es %+v, esperaba %+v (del mas viejo al mas nuevo, con todos sus campos)", i, cob.History[i], want[i])
		}
	}

	mut := tqMetric(t, v, "mutacion")
	wantMut := []ratchet.Change{hist[1], hist[4]}
	if len(mut.History) != len(wantMut) {
		t.Fatalf("CA-376: mutacion trae solo sus movimientos: %+v", mut.History)
	}
	for i := range wantMut {
		if !tqIgual(mut.History[i], wantMut[i]) {
			t.Fatalf("CA-376: mutacion: el movimiento %d es %+v, esperaba %+v", i, mut.History[i], wantMut[i])
		}
	}
	for _, m := range v.Metrics {
		for _, c := range m.History {
			if c.Metric != m.Name {
				t.Fatalf("CA-376: el history de %s trae un movimiento de %s: %+v", m.Name, c.Metric, c)
			}
		}
	}

	deuda := tqMetric(t, v, "deuda")
	if deuda.History == nil || len(deuda.History) != 0 {
		t.Fatalf("CA-376: una metrica sin movimientos trae history vacio, nunca nil: %#v", deuda.History)
	}

	// lo de antes sigue igual: el ultimo movimiento es el mas nuevo
	if cob.LastKind != "loosened" || cob.LastTo != 78 || cob.LastFrom == nil || *cob.LastFrom != 80 || !cob.LastTS.Equal(hist[3].TS) {
		t.Fatalf("CA-376: el ultimo movimiento de cobertura no cambia: %+v", cob)
	}
	if deuda.Value != nil || deuda.LastKind != "" {
		t.Fatalf("CA-376: deuda sigue declarada sin congelar: %+v", deuda)
	}
}

// CA-376: `hoom status --json` emite el history de cada metrica (lista,
// nunca null) y la clave ratchet del snapshot es exactamente Ratchet(root).
func TestCA376_StatusJSONEmiteElHistorial(t *testing.T) {
	dir := initProject(t)
	tqBaseline(t, dir)

	s, err := BuildFor(dir, "main", "")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s.Ratchet, Ratchet(dir)) {
		t.Fatalf("CA-376: Ratchet es la unica fuente del trinquete de status:\nsnapshot %+v\nRatchet  %+v", s.Ratchet, Ratchet(dir))
	}

	var buf bytes.Buffer
	if err := Run(dir, "main", &buf, Options{JSON: true}); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Ratchet struct {
			Metrics []map[string]json.RawMessage `json:"metrics"`
		} `json:"ratchet"`
	}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("CA-376: --json invalido: %v\n%s", err, buf.String())
	}
	if len(m.Ratchet.Metrics) != 3 {
		t.Fatalf("CA-376: --json trae las tres metricas: %s", buf.String())
	}
	largos := map[string]int{}
	for _, metric := range m.Ratchet.Metrics {
		var name string
		_ = json.Unmarshal(metric["name"], &name)
		raw, ok := metric["history"]
		if !ok {
			t.Fatalf("CA-376: la metrica %s trae la clave history en --json: %s", name, buf.String())
		}
		if strings.TrimSpace(string(raw)) == "null" {
			t.Fatalf("CA-376: history de %s es una lista, nunca null: %s", name, raw)
		}
		var moves []map[string]any
		if err := json.Unmarshal(raw, &moves); err != nil {
			t.Fatalf("CA-376: history de %s es una lista de movimientos: %v\n%s", name, err, raw)
		}
		largos[name] = len(moves)
		for _, mv := range moves {
			for _, k := range []string{"ts", "metric", "to", "kind"} {
				if _, ok := mv[k]; !ok {
					t.Fatalf("CA-376: cada movimiento trae %q: %+v", k, mv)
				}
			}
		}
		if name == "cobertura" {
			if moves[0]["kind"] != "frozen" || moves[2]["kind"] != "loosened" ||
				moves[2]["reason"] != "flaky en CI del proveedor de pagos" || moves[2]["from"] != 80.0 || moves[2]["to"] != 78.0 {
				t.Fatalf("CA-376: el history de cobertura en --json, del mas viejo al mas nuevo, con from, to y reason: %s", raw)
			}
		}
	}
	if largos["cobertura"] != 3 || largos["mutacion"] != 2 || largos["deuda"] != 0 {
		t.Fatalf("CA-376: movimientos por metrica en --json: %v", largos)
	}
}

// CA-376: Ratchet sin linea base da declared false y metrics [] (no null); con
// un archivo ilegible, declared true y error. Y no escribe nada.
func TestCA376_RatchetSinBaseEIlegible(t *testing.T) {
	dir := t.TempDir()
	v := Ratchet(dir)
	if v.Declared || v.Error != "" || v.Metrics == nil || len(v.Metrics) != 0 {
		t.Fatalf("CA-376: sin linea base: declared false y metrics vacio: %#v", v)
	}
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"metrics":[]`) {
		t.Fatalf("CA-376: sin linea base metrics es [] en JSON: %s", raw)
	}
	if _, err := os.Stat(filepath.Join(dir, ".hoom")); err == nil {
		t.Fatal("CA-376: leer el trinquete no crea .hoom")
	}

	if err := os.MkdirAll(filepath.Join(dir, ".hoom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hoom", ratchet.FileName), []byte("{roto"), 0o644); err != nil {
		t.Fatal(err)
	}
	v = Ratchet(dir)
	if !v.Declared || v.Error == "" {
		t.Fatalf("CA-376: un archivo ilegible es declared true con error: %+v", v)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, ".hoom", ratchet.FileName)); string(got) != "{roto" {
		t.Fatalf("CA-376: Ratchet no toca el archivo: %q", got)
	}
}

// CA-376: el texto de `hoom status` no cambia con el history: el render de un
// snapshot con los movimientos es identico al del mismo snapshot sin ellos, y
// no nombra los movimientos viejos (ni su razon).
func TestCA376_ElTextoDeStatusNoCambia(t *testing.T) {
	dir := initProject(t)
	tqBaseline(t, dir)
	s, err := BuildFor(dir, "main", "")
	if err != nil {
		t.Fatal(err)
	}

	sin := *s
	sin.Ratchet.Metrics = make([]RatchetMetric, len(s.Ratchet.Metrics))
	copy(sin.Ratchet.Metrics, s.Ratchet.Metrics)
	for i := range sin.Ratchet.Metrics {
		sin.Ratchet.Metrics[i].History = nil
	}

	for _, color := range []bool{false, true} {
		var con, base bytes.Buffer
		Render(&con, s, color)
		Render(&base, &sin, color)
		if con.String() != base.String() {
			t.Fatalf("CA-376: el texto de status no cambia con history (color %v):\ncon history:\n%s\nsin history:\n%s", color, con.String(), base.String())
		}
	}

	var out bytes.Buffer
	Render(&out, s, false)
	txt := out.String()
	for _, want := range []string{"trinquete: 3 metrica(s)", "base 78", "loosened 80 -> 78", "sin congelar"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("CA-376: la seccion del trinquete sigue diciendo %q:\n%s", want, txt)
		}
	}
	for _, no := range []string{"flaky en CI", "frozen", "70 -> 80"} {
		if strings.Contains(txt, no) {
			t.Fatalf("CA-376: el texto no suma los movimientos viejos (%q):\n%s", no, txt)
		}
	}
}
