// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-293): la review limpia deja rastro. 'hoom review' que termina
// revisado escribe .hoom/reviews/<id>.json en el arbol revisado, append-only;
// sin-revisar y no-entregable no escriben nada. Providers falsos en el PATH:
// ningun CLI de IA real.
package reviewcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func rcRegistros(t *testing.T, dir string) []string {
	t.Helper()
	entradas, err := os.ReadDir(filepath.Join(dir, ".hoom", RecordsDir))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entradas {
		out = append(out, e.Name())
	}
	return out
}

func rcVeredicto(t *testing.T, dir string) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: time.Now().UTC(), Git: gitx.Snapshot(dir, "main"),
		Gates: []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass}}}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	return v
}

func rcLeer(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("CA-293: no pude leer el registro %s: %v", path, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-293: el registro es JSON: %v\n%s", err, raw)
	}
	return m
}

// CA-293: WriteRecord estampa id y created_at, escribe en
// dir/.hoom/reviews/<id>.json, nunca pisa uno existente; Records los lee del
// mas viejo al mas nuevo y avisa de los ilegibles sin fallar.
func TestCA293_WriteRecordYRecords(t *testing.T) {
	dir := t.TempDir()
	r, err := WriteRecord(dir, Record{Task: "precios", Spec: ".hoom/specs/precios.md", Fingerprint: "h1",
		VerdictID: "v1", Verdict: "green", Lenses: Lentes, Provider: "codex", Writer: "claude", Cross: CrossYes,
		Findings: []string{}})
	if err != nil {
		t.Fatalf("CA-293: WriteRecord: %v", err)
	}
	if r.ID == "" || r.CreatedAt.IsZero() {
		t.Fatalf("CA-293: WriteRecord estampa id y created_at: %+v", r)
	}
	path := filepath.Join(dir, ".hoom", "reviews", r.ID+".json")
	m := rcLeer(t, path)
	for _, k := range []string{"id", "created_at", "task", "spec", "fingerprint", "verdict_id", "verdict",
		"lenses", "provider", "writer", "cross", "findings"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("CA-293: al registro le falta %q: %v", k, m)
		}
	}
	if m["id"] != r.ID || m["task"] != "precios" || m["verdict_id"] != "v1" || m["cross"] != CrossYes {
		t.Fatalf("CA-293: el registro guarda lo recibido: %v", m)
	}
	original, _ := os.ReadFile(path)

	// append-only: el mismo id no se pisa
	if _, err := WriteRecord(dir, Record{ID: r.ID, Task: "otra"}); err == nil {
		t.Fatal("CA-293: WriteRecord nunca pisa un registro existente")
	}
	if ahora, _ := os.ReadFile(path); !bytes.Equal(ahora, original) {
		t.Fatalf("CA-293: el registro existente queda byte a byte igual:\n%s", ahora)
	}

	time.Sleep(1100 * time.Millisecond)
	r2, err := WriteRecord(dir, Record{Task: "precios", Lenses: []string{"risk"}, Findings: []string{"f1"}})
	if err != nil || r2.ID == r.ID {
		t.Fatalf("CA-293: un segundo registro tiene su propio id: %+v %v", r2, err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".hoom", "reviews", "roto.json"), []byte("{no es json"), 0o644); err != nil {
		t.Fatal(err)
	}
	recs, warns := Records(dir)
	if len(recs) != 2 || recs[0].ID != r.ID || recs[1].ID != r2.ID {
		t.Fatalf("CA-293: Records del mas viejo al mas nuevo: %+v", recs)
	}
	if !reflect.DeepEqual(recs[0].Lenses, Lentes) || recs[1].Findings[0] != "f1" {
		t.Fatalf("CA-293: Records devuelve lo escrito: %+v", recs)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "roto.json") {
		t.Fatalf("CA-293: un registro ilegible es un aviso que lo nombra: %q", warns)
	}
	if recs, warns := Records(t.TempDir()); len(recs) != 0 || len(warns) != 0 {
		t.Fatalf("CA-293: sin .hoom/reviews/ no hay registros ni avisos: %v %v", recs, warns)
	}
}

// CA-293: 'hoom review' revisado escribe el registro en el arbol revisado,
// con task sacado del --spec, la huella del arbol, el ultimo veredicto
// completo al empezar y lo que la review observo; el Result trae record_id.
func TestCA293_ReviewRevisadoDejaRegistro(t *testing.T) {
	root := repo(t)
	write(t, root, ".hoom/specs/precios.md", "# Spec: precios\n")
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	metaDeRun(t, root, "20260904T190000_aaaaaa", "claude", "writer", time.Now())
	v := rcVeredicto(t, root)
	huella := gitx.Snapshot(root, "main").ChangeFingerprint

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Spec: ".hoom/specs/precios.md"}, &out)
	if err != nil {
		t.Fatalf("CA-293: %v\n%s", err, out.String())
	}
	if res.Status != "revisado" || res.ExitCode != 0 {
		t.Fatalf("CA-293: la review de prueba termina revisado: %+v\n%s", res, out.String())
	}
	if res.RecordID == "" {
		t.Fatalf("CA-293: el Result de una review revisada trae record_id: %+v", res)
	}
	path := filepath.Join(root, ".hoom", "reviews", res.RecordID+".json")
	m := rcLeer(t, path)
	if m["id"] != res.RecordID || m["task"] != "precios" || m["spec"] != ".hoom/specs/precios.md" {
		t.Fatalf("CA-293: id, task (del --spec) y spec del registro: %v", m)
	}
	if m["fingerprint"] != huella || huella == "" {
		t.Fatalf("CA-293: fingerprint es la huella del arbol revisado (%s): %v", huella, m["fingerprint"])
	}
	if m["verdict_id"] != v.ID || m["verdict"] != "green" {
		t.Fatalf("CA-293: verdict_id/verdict son los del ultimo veredicto completo al empezar (%s): %v", v.ID, m)
	}
	if m["provider"] != res.Provider || m["writer"] != res.Writer || m["cross"] != res.Cross || res.Cross != CrossYes {
		t.Fatalf("CA-293: provider, writer y cross del registro son los de la review: %v vs %+v", m, res)
	}
	if lentes, _ := m["lenses"].([]any); len(lentes) != 1 || lentes[0] != "risk" {
		t.Fatalf("CA-293: lenses son las que corrieron: %v", m["lenses"])
	}
	if f, ok := m["findings"].([]any); !ok || len(f) != 0 {
		t.Fatalf("CA-293: findings es la lista (vacia) de lo que hoom vio aparecer: %v", m["findings"])
	}
	raw, _ := json.Marshal(res)
	if !strings.Contains(string(raw), `"record_id":"`+res.RecordID+`"`) {
		t.Fatalf("CA-293: el JSON del Result trae record_id: %s", raw)
	}
	if n := len(rcRegistros(t, root)); n != 1 {
		t.Fatalf("CA-293: un registro por review: %d", n)
	}
	// el registro de review no mueve la huella (queda fuera del candidato)
	if h := gitx.Snapshot(root, "main").ChangeFingerprint; h != huella {
		t.Fatalf("CA-291: el registro de review no cambia la huella: %s -> %s", huella, h)
	}
}

// CA-293: --task manda sobre el --spec, y el registro va al arbol revisado
// (el worktree de la tarea), no al principal.
func TestCA293_ReviewConTareaEscribeEnElWorktree(t *testing.T) {
	root := repo(t)
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "cambio de codigo")
	wt := filepath.Join(root, ".hoom", "worktrees", "tarea-a")
	git(t, root, "worktree", "add", "-q", "-b", "hoom/tarea-a", wt, "main")
	write(t, wt, "app.go", "package app\n\nfunc EnLaTarea() {}\n")
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "reliability", Task: "tarea-a", Spec: ".hoom/specs/otra.md"}, &out)
	if err != nil {
		t.Fatalf("CA-293: %v\n%s", err, out.String())
	}
	if res.Status != "revisado" || res.RecordID == "" {
		t.Fatalf("CA-293: revisado con record_id: %+v\n%s", res, out.String())
	}
	m := rcLeer(t, filepath.Join(wt, ".hoom", "reviews", res.RecordID+".json"))
	if m["task"] != "tarea-a" {
		t.Fatalf("CA-293: task sale de --task antes que del --spec: %v", m["task"])
	}
	if n := len(rcRegistros(t, root)); n != 0 {
		t.Fatalf("CA-293: el registro va al arbol revisado, no al principal: %v", rcRegistros(t, root))
	}

	// sin --task ni --spec, task queda vacio
	root2 := repo(t)
	res, err = Run(root2, "main", Options{Lens: "risk"}, &out)
	if err != nil || res.RecordID == "" {
		t.Fatalf("CA-293: %+v %v", res, err)
	}
	m = rcLeer(t, filepath.Join(root2, ".hoom", "reviews", res.RecordID+".json"))
	if m["task"] != "" {
		t.Fatalf("CA-293: sin --task ni --spec, task vacio: %v", m["task"])
	}
}

// CA-293: sin-revisar (cambio solo de documentacion) y no-entregable (el
// run fallo, o la review no seria cruzada) no escriben nada en .hoom/reviews/.
func TestCA293_SinRevisarYNoEntregableNoDejanRegistro(t *testing.T) {
	// solo documentacion: cero lentes, sin-revisar
	docs := t.TempDir()
	git(t, docs, "init", "-b", "main")
	git(t, docs, "config", "user.email", "test@hoom.dev")
	git(t, docs, "config", "user.name", "hoom test")
	write(t, docs, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, docs, "README.md", "# demo\n")
	git(t, docs, "add", "-A")
	git(t, docs, "commit", "-m", "inicial")
	write(t, docs, "README.md", "# demo\n\nmas prosa\n")
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	var out bytes.Buffer
	res, err := Run(docs, "main", Options{Spec: ".hoom/specs/docs.md"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "sin-revisar" || res.RecordID != "" {
		t.Fatalf("CA-293: un cambio solo de docs es sin-revisar y sin record_id: %+v", res)
	}
	if got := rcRegistros(t, docs); len(got) != 0 {
		t.Fatalf("CA-293: sin-revisar no escribe en .hoom/reviews/: %v", got)
	}
	if raw, _ := json.Marshal(res); strings.Contains(string(raw), "record_id") {
		t.Fatalf("CA-293: sin registro, record_id se omite del JSON: %s", raw)
	}

	// el run del reviewer falla: no-entregable
	roto := repo(t)
	fakeProvider(t, "claude", "echo fallando\nexit 3\n")
	fakeProvider(t, "codex", "echo fallando\nexit 3\n")
	res, err = Run(roto, "main", Options{Lens: "risk", Spec: ".hoom/specs/x.md"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "no-entregable" || res.RecordID != "" {
		t.Fatalf("CA-293: un run fallido es no-entregable sin record_id: %+v", res)
	}
	if got := rcRegistros(t, roto); len(got) != 0 {
		t.Fatalf("CA-293: no-entregable no escribe en .hoom/reviews/: %v", got)
	}

	// no seria cruzada: se niega sin lanzar nada, y tampoco deja registro
	mismo := repo(t)
	fakeProvider(t, "claude", "exit 0\n")
	metaDeRun(t, mismo, "20260904T190000_aaaaaa", "claude", "writer", time.Now())
	res, err = Run(mismo, "main", Options{Lens: "risk", Provider: "claude"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "no-entregable" || res.RecordID != "" || len(rcRegistros(t, mismo)) != 0 {
		t.Fatalf("CA-293: la negativa por no cruzada no deja registro: %+v %v", res, rcRegistros(t, mismo))
	}
}
