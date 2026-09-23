// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-334, CA-335) sobre `hoom review`: la review deja registro de sobre como
// `hoom agent` (rol reviewer, un paso run por lente, cierre entregable o
// no-entregable con su nota, el PID del dueno y sin gasto propio), y acepta
// EnvelopeID y Started con el mismo contrato. El registro vive en
// .hoom/envelopes/ del proyecto (como el de agentcmd). Providers falsos en el
// PATH: ningun CLI de IA real.
package reviewcmd

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// cbRepo arma un proyecto con la telemetria ya escondida en
// .hoom/.gitignore, hoom.yaml y codigo commiteado.
func cbRepo(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "inicial")
	return root
}

// cbTarea crea la tarea slug como la crea 'hoom task start' (rama
// hoom/<slug>, worktree en .hoom/worktrees/<slug>) y le planta un cambio de
// codigo: hay algo que revisar.
func cbTarea(t *testing.T, root, slug string) string {
	t.Helper()
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	git(t, root, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	write(t, wt, "app.go", "package app\n\nfunc EnLaTarea() {}\n")
	return wt
}

// cbMeta deja el sidecar de un run terminado de dir: quien corrio y con que
// rol. Vive en .hoom/runs/ del proyecto, como el de un run real.
func cbMeta(t *testing.T, root, dir, id, provider, role string) {
	t.Helper()
	raw, _ := json.MarshalIndent(runcmd.Meta{
		ID: id, Provider: provider, Role: role, Dir: dir,
		CreatedAt: time.Now().UTC().Add(-time.Minute), Status: "done",
	}, "", "  ")
	write(t, root, filepath.Join(".hoom", "runs", id+".meta.json"), string(raw))
}

// cbItem escribe el item de la tarjeta en el arbol raiz, con una sesion
// interactiva por provider declarado.
func cbItem(t *testing.T, root, slug string, sesiones ...string) {
	t.Helper()
	body := "titulo: Precios por region\ntipo: feature\nprioridad: media\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-20T10:00:00Z\n"
	if len(sesiones) > 0 {
		body += "sesiones:\n"
		for _, p := range sesiones {
			body += "  - provider: " + p + "\n" +
				"    abierta_por: \"hoom test <test@hoom.dev>\"\n" +
				"    abierta_en: 2026-09-23T18:00:00Z\n"
		}
	}
	write(t, root, filepath.Join(".hoom", "items", slug+".yaml"), body)
}

// cbPathAislado deja en el PATH SOLO los CLIs de IA falsos que el test pone
// (mas git y las herramientas del sistema): un claude o un codex reales de la
// maquina no pueden decidir la eleccion automatica del reviewer.
func cbPathAislado(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	gitBin, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitBin, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	return bin
}

func cbFake(t *testing.T, bin, name, script string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// cbResultado es lo que un claude real imprime al cerrar: un gasto. Si la
// review lo copiara al registro del sobre, la tarjeta lo contaria dos veces.
const cbResultado = `printf '{"type":"system","subtype":"init","session_id":"s-rev"}\n'` + "\n" +
	`printf '{"type":"result","subtype":"success","result":"listo","num_turns":1,"total_cost_usd":0.25,` +
	`"usage":{"input_tokens":10,"output_tokens":5}}\n'` + "\n"

// cbFoto es un reviewer falso que, DURANTE cada pasada, espera a que el
// registro del sobre nombre su run y lo copia a snap/pass-<n>.json. Asi el
// test ve el registro tal como estaba mientras la pasada corria.
func cbFoto(root, snap string) string {
	cnt := filepath.Join(snap, "n.txt")
	runs := filepath.Join(root, ".hoom", "runs")
	envs := filepath.Join(root, ".hoom", envelope.DirName)
	return "n=$(cat '" + cnt + "' 2>/dev/null || echo 0)\nn=$((n+1)); echo $n > '" + cnt + "'\n" +
		"id=''\n" +
		"for f in '" + runs + "'/*.meta.json; do\n" +
		"  if grep -q '\"status\": \"running\"' \"$f\" 2>/dev/null; then id=$(basename \"$f\" .meta.json); fi\n" +
		"done\n" +
		"i=0\n" +
		"while [ -d '" + envs + "' ] && [ $i -lt 40 ]; do\n" +
		"  if grep -q \"\\\"run_id\\\": \\\"$id\\\"\" '" + envs + "'/*.json 2>/dev/null; then break; fi\n" +
		"  sleep 0.05; i=$((i+1))\n" +
		"done\n" +
		"cat '" + envs + "'/*.json > '" + snap + "'/pass-$n.json 2>/dev/null\n" +
		cbResultado + "exit 0\n"
}

func cbMetasEn(root string) int {
	entries, err := os.ReadDir(filepath.Join(root, ".hoom", "runs"))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".meta.json") {
			n++
		}
	}
	return n
}

// cbStarted cuenta las llamadas a Started y fotografia lo que habia en disco
// en el momento de cada una.
type cbStarted struct {
	mu          sync.Mutex
	root, id    string
	calls       int
	rec         envelope.Record
	enDisco     bool
	metasAlLlam int
}

func (s *cbStarted) fn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.metasAlLlam = cbMetasEn(s.root)
	for _, r := range envelope.List(s.root) {
		if r.ID == s.id {
			s.enDisco, s.rec = true, r
		}
	}
}

func (s *cbStarted) snapshot() (int, bool, envelope.Record, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.enDisco, s.rec, s.metasAlLlam
}

// cbCaso es una review que se decide ANTES de la primera pasada.
type cbCaso struct {
	nombre   string
	root, wt string
	opt      Options
	esError  bool // se decide con error (y no con un Result)
}

// cbCasosSinPasada arma, cada uno en su proyecto, los casos que se deciden
// antes de la primera pasada: sin lentes, review no cruzada y sin provider.
func cbCasosSinPasada(t *testing.T) []cbCaso {
	t.Helper()
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	fakeProvider(t, "gemini", "exit 0\n")
	var casos []cbCaso

	// sin lentes: el cambio de la tarea es solo documentacion
	root := cbRepo(t)
	wt := filepath.Join(root, ".hoom", "worktrees", "precios")
	git(t, root, "worktree", "add", "-q", "-b", "hoom/precios", wt, "main")
	write(t, wt, "README.md", "# solo prosa\n")
	casos = append(casos, cbCaso{nombre: "sin lentes", root: root, wt: wt,
		opt: Options{Task: "precios", Spec: ".hoom/specs/precios.md", Provider: "codex"}})

	// no cruzada: el writer observado corrio en claude y revisa claude
	root = cbRepo(t)
	wt = cbTarea(t, root, "precios")
	cbMeta(t, root, wt, "20260923T110000_cbw001", "claude", "writer")
	casos = append(casos, cbCaso{nombre: "no cruzada", root: root, wt: wt,
		opt: Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "claude"}})

	// sin provider que pueda cargar el contrato
	root = cbRepo(t)
	wt = cbTarea(t, root, "precios")
	casos = append(casos, cbCaso{nombre: "provider sin system_prompt", root: root, wt: wt, esError: true,
		opt: Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "gemini"}})

	root = cbRepo(t)
	wt = cbTarea(t, root, "precios")
	casos = append(casos, cbCaso{nombre: "provider desconocido", root: root, wt: wt, esError: true,
		opt: Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "nadie"}})
	return casos
}

// CA-334: una review que termina revisada deja UN registro de sobre: rol
// reviewer, su provider, task, dir, spec y pid; steps = cantidad de lentes;
// mientras corre cada pasada el registro esta en el paso run con step = su
// numero y run_id = su run; cierra entregable con stage "ok" y sin usage
// (el gasto de cada pasada ya esta en su sidecar).
func TestCA334_ReviewDejaRegistroDeSobre(t *testing.T) {
	root := cbRepo(t)
	wt := cbTarea(t, root, "precios")
	write(t, wt, "internal/auth/login.go", "package auth\n") // ruta de riesgo: las 4 lentes
	cbMeta(t, root, wt, "20260923T110000_cbw002", "codex", "writer")
	snap := t.TempDir()
	fakeProvider(t, "claude", cbFoto(root, snap))

	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Provider: "claude"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-334: %v", err)
	}
	if res.Status != "revisado" || len(res.Lenses) != 4 || len(res.Passes) != 4 {
		t.Fatalf("CA-334: fixture: la review de las 4 lentes termina revisada: %+v", res)
	}

	recs := envelope.List(root)
	if len(recs) != 1 {
		t.Fatalf("CA-334: una review, un registro de sobre en .hoom/envelopes/ del proyecto: %+v", recs)
	}
	rec := recs[0]
	if rec.Role != "reviewer" || rec.Provider != "claude" || rec.Task != "precios" || rec.Dir != wt {
		t.Fatalf("CA-334: el registro identifica la review (rol reviewer, provider, task, dir %s): %+v", wt, rec)
	}
	if !strings.HasSuffix(rec.Spec, "precios.md") {
		t.Fatalf("CA-334: el registro nombra el spec de la review: %q", rec.Spec)
	}
	if rec.PID != os.Getpid() {
		t.Fatalf("CA-334: el registro trae el PID de su dueno (%d): %+v", os.Getpid(), rec)
	}
	if rec.Steps != 4 {
		t.Fatalf("CA-334: steps es la cantidad de lentes (4): %+v", rec)
	}
	if rec.Status != envelope.StatusDeliverable || rec.Stage != "ok" || rec.ExitCode != 0 {
		t.Fatalf("CA-334: una review revisada cierra entregable con stage ok: %+v", rec)
	}
	if rec.EndedAt.IsZero() || rec.StartedAt.IsZero() {
		t.Fatalf("CA-334: el registro dice cuando empezo y cuando termino: %+v", rec)
	}
	if !rec.Usage.Empty() {
		t.Fatalf("CA-334: el registro de la review no lleva gasto (ya esta en el sidecar de cada pasada): %+v", rec.Usage)
	}
	if extra := envelope.List(wt); len(extra) != 0 {
		t.Fatalf("CA-334: el registro va al proyecto, no al arbol revisado: %+v", extra)
	}

	// lo que el registro decia MIENTRAS cada pasada corria
	for i := 1; i <= 4; i++ {
		raw, err := os.ReadFile(filepath.Join(snap, "pass-"+string(rune('0'+i))+".json"))
		if err != nil || len(raw) == 0 {
			t.Fatalf("CA-334: durante la pasada %d el registro ya existia en .hoom/envelopes/: %v", i, err)
		}
		var foto envelope.Record
		if err := json.Unmarshal(raw, &foto); err != nil {
			t.Fatalf("CA-334: durante la pasada %d hay UN registro legible: %v\n%s", i, err, raw)
		}
		want := res.Passes[i-1].RunID
		if foto.Stage != "run" || foto.Step != i || foto.Steps != 4 || foto.RunID != want {
			t.Fatalf("CA-334: durante la pasada %d el registro esta en el paso run, step %d de 4, run_id %s: %+v",
				i, i, want, foto)
		}
		if foto.Status != envelope.StatusRunning || foto.PID != os.Getpid() || foto.ID != rec.ID {
			t.Fatalf("CA-334: durante la pasada %d el registro esta en curso, con el PID del dueno y su id: %+v", i, foto)
		}
	}
}

// CA-334: una review que corta cierra el registro no-entregable con el paso
// y la nota del contrato: `run` / "el run del reviewer fallo" y `scope` /
// "el reviewer escribio fuera de su territorio".
func TestCA334_ReviewNoEntregableCierraConStageYNota(t *testing.T) {
	// el run del reviewer falla
	root := cbRepo(t)
	wt := cbTarea(t, root, "precios")
	cbMeta(t, root, wt, "20260923T110000_cbw003", "claude", "writer")
	fakeProvider(t, "codex", "echo fallando\nexit 3\n")
	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "codex"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-334: %v", err)
	}
	if res.Status != "no-entregable" || len(res.Passes) != 1 {
		t.Fatalf("CA-334: fixture: el run fallido corta la review: %+v", res)
	}
	recs := envelope.List(root)
	if len(recs) != 1 {
		t.Fatalf("CA-334: la review que corta en la pasada deja su registro: %+v", recs)
	}
	rec := recs[0]
	if rec.Status != envelope.StatusNotDeliverable || rec.Stage != "run" || rec.Note != "el run del reviewer fallo" {
		t.Fatalf("CA-334: cierra no-entregable en run con \"el run del reviewer fallo\": %+v", rec)
	}
	if rec.ExitCode == 0 || rec.EndedAt.IsZero() || rec.Steps != 1 || rec.RunID != res.Passes[0].RunID {
		t.Fatalf("CA-334: el cierre tiene exit distinto de 0, fin, steps 1 y el run que fallo: %+v", rec)
	}
	if rec.Role != "reviewer" || rec.PID != os.Getpid() {
		t.Fatalf("CA-334: rol reviewer y pid del dueno: %+v", rec)
	}

	// el reviewer escribe codigo: fuera de su territorio
	root2 := cbRepo(t)
	wt2 := cbTarea(t, root2, "precios")
	cbMeta(t, root2, wt2, "20260923T110000_cbw004", "claude", "writer")
	fakeProvider(t, "codex", "printf 'package app\\n' > colado.go\nexit 0\n")
	res, err = Run(root2, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "codex"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-334: %v", err)
	}
	if res.Status != "no-entregable" || len(res.Passes) != 1 || res.Passes[0].Scope.OK {
		t.Fatalf("CA-334: fixture: escribir codigo es una violacion de scope: %+v", res)
	}
	recs = envelope.List(root2)
	if len(recs) != 1 {
		t.Fatalf("CA-334: la review que viola el scope deja su registro: %+v", recs)
	}
	rec = recs[0]
	if rec.Status != envelope.StatusNotDeliverable || rec.Stage != "scope" || rec.Note != "el reviewer escribio fuera de su territorio" {
		t.Fatalf("CA-334: cierra no-entregable en scope con \"el reviewer escribio fuera de su territorio\": %+v", rec)
	}
	if rec.ExitCode == 0 || rec.EndedAt.IsZero() {
		t.Fatalf("CA-334: el cierre tiene exit distinto de 0 y fin: %+v", rec)
	}
}

// CA-334: lo que se decide antes de la primera pasada (sin lentes, review
// no cruzada, sin provider) no escribe registro de sobre.
func TestCA334_ReviewDecididaAntesDeLaPrimeraPasadaNoDejaRegistro(t *testing.T) {
	for _, c := range cbCasosSinPasada(t) {
		res, err := Run(c.root, "main", c.opt, io.Discard)
		if c.esError && err == nil {
			t.Fatalf("CA-334: %s: fixture: la review se niega con un error: %+v", c.nombre, res)
		}
		if !c.esError && (err != nil || len(res.Passes) != 0) {
			t.Fatalf("CA-334: %s: fixture: la review se decide sin pasadas: %+v %v", c.nombre, res, err)
		}
		if recs := envelope.List(c.root); len(recs) != 0 {
			t.Fatalf("CA-334: %s: una review decidida antes de la primera pasada no deja registro de sobre: %+v", c.nombre, recs)
		}
		if recs := envelope.List(c.wt); len(recs) != 0 {
			t.Fatalf("CA-334: %s: tampoco en el arbol revisado: %+v", c.nombre, recs)
		}
	}
}

// CA-335: con EnvelopeID la review usa ese id para su registro, y Started se
// llama UNA vez, al escribir el primer registro: cuando se llama el registro
// ya esta en disco (en curso, rol reviewer, con el PID del dueno) y todavia
// no arranco ninguna pasada.
func TestCA335_ReviewUsaEnvelopeIDYLlamaStartedUnaVez(t *testing.T) {
	root := cbRepo(t)
	wt := cbTarea(t, root, "precios")
	write(t, wt, "internal/auth/login.go", "package auth\n") // las 4 lentes: 4 pasadas
	cbMeta(t, root, wt, "20260923T110000_cbw005", "claude", "writer")
	fakeProvider(t, "codex", "exit 0\n")
	antes := cbMetasEn(root)

	s := &cbStarted{root: root, id: "20260923T120000_cbrev1"}
	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Provider: "codex",
		EnvelopeID: s.id, Started: s.fn}, io.Discard)
	if err != nil {
		t.Fatalf("CA-335: %v", err)
	}
	if res.Status != "revisado" || len(res.Passes) != 4 {
		t.Fatalf("CA-335: fixture: la review de 4 lentes termina revisada: %+v", res)
	}
	calls, enDisco, rec, metas := s.snapshot()
	if calls != 1 {
		t.Fatalf("CA-335: Started se llama una sola vez aunque haya 4 pasadas, no %d", calls)
	}
	if !enDisco {
		t.Fatalf("CA-335: cuando Started se llama, el registro %s ya esta en disco", s.id)
	}
	if rec.Role != "reviewer" || rec.Status != envelope.StatusRunning || rec.PID != os.Getpid() {
		t.Fatalf("CA-335: el primer registro esta en curso, es del reviewer y trae el PID del dueno: %+v", rec)
	}
	if metas != antes {
		t.Fatalf("CA-335: Started se llama al escribir el primer registro, antes de la primera pasada (runs al llamar: %d, antes: %d)", metas, antes)
	}
	recs := envelope.List(root)
	if len(recs) != 1 || recs[0].ID != s.id {
		t.Fatalf("CA-335: el unico registro de la review usa el id preasignado %s: %+v", s.id, recs)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", envelope.DirName, s.id+".json")); err != nil {
		t.Fatalf("CA-335: el registro vive en .hoom/envelopes/%s.json: %v", s.id, err)
	}
}

// CA-335: una review que se decide antes de su primer registro no llama a
// Started ni deja registro con el id preasignado.
func TestCA335_ReviewSinPasadaNoLlamaStarted(t *testing.T) {
	for i, c := range cbCasosSinPasada(t) {
		s := &cbStarted{root: c.root, id: "20260923T120000_cbrev" + string(rune('a'+i))}
		c.opt.EnvelopeID, c.opt.Started = s.id, s.fn
		_, _ = Run(c.root, "main", c.opt, io.Discard)
		if calls, _, _, _ := s.snapshot(); calls != 0 {
			t.Fatalf("CA-335: %s: sin registro, Started no se llama (llamadas: %d)", c.nombre, calls)
		}
		if _, err := os.Stat(filepath.Join(c.root, ".hoom", envelope.DirName, s.id+".json")); err == nil {
			t.Fatalf("CA-335: %s: no queda registro con el id preasignado", c.nombre)
		}
		if recs := envelope.List(c.root); len(recs) != 0 {
			t.Fatalf("CA-335: %s: no queda ningun registro: %+v", c.nombre, recs)
		}
	}
}
