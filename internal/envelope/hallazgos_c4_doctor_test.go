// Tests adversariales del hallazgo f34bc3 de la review de C4
// (.hoom/specs/historia-doctor-y-cinta.md, CA-369): el doctor arma 'rm
// .hoom/envelopes/<id>.json' con el id de cada registro que devuelve List. Un
// registro plantado a mano cuyo id no es el nombre de su archivo, o que no
// tiene forma de id, no es un sobre de hoom: List no lo devuelve.
package envelope

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"
)

func hdPlantar(t *testing.T, root, archivo string, rec Record) {
	t.Helper()
	dir := filepath.Join(root, ".hoom", DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, archivo), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hdIDs(rs []Record) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.ID)
	}
	sort.Strings(out)
	return out
}

// CA-369 (hallazgo f34bc3): List saltea los registros plantados: id que no es
// el nombre de su archivo (aunque tenga forma de id) o sin forma de id
// (metacaracteres de shell, separadores, '..'), con su nombre o con otro.
func TestCA369_f34bc3_ListSalteaRegistrosPlantados(t *testing.T) {
	root := t.TempDir()
	t0 := time.Date(2026, 9, 23, 11, 0, 0, 0, time.UTC)
	rec := func(id string) Record {
		return Record{ID: id, Role: "writer", Provider: "claude", Stage: "run", Step: 3, Steps: 5,
			Status: StatusRunning, StartedAt: t0, UpdatedAt: t0}
	}
	normal, nuevo := "20260923T090000_norm01", NewID()
	hdPlantar(t, root, normal+".json", rec(normal))
	hdPlantar(t, root, nuevo+".json", rec(nuevo))

	for _, id := range []string{"x; touch PWNED; #", "$(touch PWNED)", "`touch PWNED`", "a b"} {
		hdPlantar(t, root, id+".json", rec(id))
	}
	hdPlantar(t, root, "20260923T090000_inye01.json", rec("x; touch PWNED; #"))
	hdPlantar(t, root, "20260923T090000_vict01.json", rec("../../../VICTIMA"))
	hdPlantar(t, root, "20260923T090000_vict02.json", rec("../../VICTIMA"))
	hdPlantar(t, root, "20260923T090000_vict03.json", rec("/etc/VICTIMA"))
	// con forma de id, pero no la de su archivo
	hdPlantar(t, root, "20260923T090000_arch01.json", rec("20260923T090000_otro01"))
	hdPlantar(t, root, "20260923T090000_arch02.json", rec(normal))

	want := []string{normal, nuevo}
	sort.Strings(want)
	got := List(root)
	if ids := hdIDs(got); !reflect.DeepEqual(ids, want) {
		t.Fatalf("CA-369: List devuelve solo los registros cuyo id es el nombre de su archivo y tiene forma de id\nquiere: %q\nfue:    %q", want, ids)
	}
}

// CA-369 (hallazgo f34bc3, guarda): los registros que escribe hoom (Write con
// un id de NewID o con el id de siempre) siguen en List.
func TestCA369_f34bc3_Guarda_ListDevuelveLosDeHoom(t *testing.T) {
	root := t.TempDir()
	t0 := time.Now().UTC()
	nuevo := NewID()
	for _, id := range []string{nuevo, "20260923T090000_norm01"} {
		if err := Write(root, Record{ID: id, Role: "writer", Provider: "claude", Stage: "run", Step: 3, Steps: 5,
			Status: StatusRunning, StartedAt: t0}); err != nil {
			t.Fatalf("precondicion: Write(%s): %v", id, err)
		}
	}
	want := []string{nuevo, "20260923T090000_norm01"}
	sort.Strings(want)
	if ids := hdIDs(List(root)); !reflect.DeepEqual(ids, want) {
		t.Fatalf("CA-369: los registros de hoom siguen en List\nquiere: %q\nfue:    %q", want, ids)
	}
}
