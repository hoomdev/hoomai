package reviewcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
)

// finding.RevisionesCompletas reads .hoom/reviews/ itself (finding cannot
// import reviewcmd): on the same records it has to give exactly the
// completa records Records trusts, in the same order and with the same
// task and findings.
func TestRevisionesCompletasLeeLoMismoQueRecords(t *testing.T) {
	dir := t.TempDir()
	base := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	escribir := func(id, task, cobertura string, findings []string, at time.Time) {
		t.Helper()
		if _, err := WriteRecord(dir, Record{ID: id, Task: task, Cobertura: cobertura, Findings: findings, CreatedAt: at}); err != nil {
			t.Fatal(err)
		}
	}
	// written out of order: both readings must sort by created_at
	escribir("20260501T130000_bbbbbb", "t1", CoberturaCompleta, []string{"f2"}, base.Add(time.Hour))
	escribir("20260501T120000_aaaaaa", "t1", CoberturaCompleta, []string{"f1", "f3"}, base)
	escribir("20260501T140000_cccccc", "t1", CoberturaDelta, []string{"f4"}, base.Add(2*time.Hour))
	escribir("20260501T150000_dddddd", "t2", CoberturaCompleta, nil, base.Add(3*time.Hour))
	rd := filepath.Join(dir, ".hoom", RecordsDir)
	// untrusted by both: an id that is not its name, a bad id shape, garbage
	for name, body := range map[string]string{
		"20260501T160000_eeeeee.json": `{"id":"20260501T160000_ffffff","cobertura":"completa","task":"t1"}`,
		"x.json":                      `{"id":"x","cobertura":"completa","task":"t1"}`,
		"20260501T170000_gggggg.json": `{no es json`,
	} {
		if err := os.WriteFile(filepath.Join(rd, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	recs, _ := Records(dir)
	want := []finding.RevisionCompleta{}
	for _, r := range recs {
		if r.Cobertura == CoberturaCompleta {
			want = append(want, finding.RevisionCompleta{ID: r.ID, Task: r.Task, Findings: append([]string{}, r.Findings...)})
		}
	}
	got := finding.RevisionesCompletas(dir)
	if len(want) != 3 {
		t.Fatalf("Records dio %d completas, el test espera 3: %+v", len(want), want)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("las dos lecturas difieren:\nfinding:   %+v\nreviewcmd: %+v", got, want)
	}
}

// The copy of validRecordID in finding has to accept and refuse the same ids.
func TestIdDeRegistroIgualQueValidRecordID(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"20260501T120000_abc123", "20260501T120000_ABC123", "20260501x120000_abc123",
		"20260501T120000-abc123", "20260501T120000_abc12", "20260501T12000a_abc123", "", "20260501T120000_abc12\n"} {
		rd := filepath.Join(dir, ".hoom", RecordsDir)
		_ = os.RemoveAll(rd)
		if err := os.MkdirAll(rd, 0o755); err != nil {
			t.Fatal(err)
		}
		body := `{"id":` + quote(id) + `,"cobertura":"completa"}`
		if err := os.WriteFile(filepath.Join(rd, id+".json"), []byte(body), 0o644); err != nil {
			continue // a name the filesystem refuses
		}
		got := len(finding.RevisionesCompletas(dir)) == 1
		if got != validRecordID(id) {
			t.Errorf("id %q: finding acepta=%v, reviewcmd acepta=%v", id, got, validRecordID(id))
		}
	}
}

func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
