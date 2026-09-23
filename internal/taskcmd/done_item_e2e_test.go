// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-289, CA-290) contra el BINARIO: 'hoom task done' registra el cierre en
// el item del arbol donde corre (hecho_en, commit_final), y los casos
// limite: sin item, --force, item invalido, item ya hecho. Sin HOOM_TASK en
// el entorno; la tarea se crea con 'hoom task start' y se verifica con
// 'hoom verify' reales.
package taskcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

var (
	tdOnce sync.Once
	tdBin  string
	tdDir  string
	tdErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if tdDir != "" {
		os.RemoveAll(tdDir)
	}
	os.Exit(code)
}

func tdBinario(t *testing.T) string {
	t.Helper()
	tdOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			tdErr = err
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				break
			}
			padre := filepath.Dir(dir)
			if padre == dir {
				tdErr = fmt.Errorf("no encontre go.mod hacia arriba")
				return
			}
			dir = padre
		}
		tdDir, err = os.MkdirTemp("", "hoom-taskcmd-bin-")
		if err != nil {
			tdErr = err
			return
		}
		bin := filepath.Join(tdDir, "hoom")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/hoom")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			tdErr = fmt.Errorf("go build ./cmd/hoom: %v\n%s", err, out)
			return
		}
		tdBin = bin
	})
	if tdErr != nil {
		t.Fatal(tdErr)
	}
	return tdBin
}

type tdSalida struct {
	exit           int
	stdout, stderr string
}

func tdCorrer(t *testing.T, bin, dir string, args ...string) tdSalida {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOOM_TASK=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no pude ejecutar hoom %v: %v", args, err)
	}
	return tdSalida{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

func tdOK(t *testing.T, ca string, r tdSalida, args ...string) {
	t.Helper()
	if r.exit != 0 {
		t.Fatalf("%s: hoom %v salio %d\nstdout: %s\nstderr: %s", ca, args, r.exit, r.stdout, r.stderr)
	}
}

// tdTareaLista: proyecto con la tarea creada por 'hoom task start', codigo
// commiteado y un 'hoom verify' verde commiteado: 'task done' la cerraria.
func tdTareaLista(t *testing.T, bin, slug string) (root, wt string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	rdGit(t, root, "init", "-b", "main")
	rdGit(t, root, "config", "user.email", "test@hoom.dev")
	rdGit(t, root, "config", "user.name", "hoom test")
	rdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	rdEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	rdEscribir(t, root, "app.go", "package app\n")
	rdGit(t, root, "add", "-A")
	rdGit(t, root, "commit", "-q", "-m", "inicial")
	tdOK(t, "CA-289", tdCorrer(t, bin, root, "task", "start", slug), "task", "start", slug)
	wt = filepath.Join(root, ".hoom", "worktrees", slug)
	rdEscribir(t, wt, "feature.go", "package app\n\nfunc Feature() int { return 1 }\n")
	tdOK(t, "CA-289", tdCorrer(t, bin, wt, "verify"), "verify")
	rdGit(t, wt, "add", "-A")
	rdGit(t, wt, "commit", "-q", "-m", "feature y veredicto")
	return root, wt
}

func tdItemYAML(titulo, extra string) string {
	return "titulo: " + titulo + "\ntipo: bug\nprioridad: alta\n" +
		"pedido: |\n  que el cierre quede registrado\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\npresupuesto_usd: 5\n" + extra
}

func tdYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no pude leer %s: %v", path, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s no es YAML: %v\n%s", path, err, raw)
	}
	return m
}

func tdExiste(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// CA-289: con las condiciones cumplidas, 'task done' escribe hecho_en (UTC)
// y commit_final (sha completo de hoom/<slug>) en el item del arbol donde
// corre, conserva los demas campos, dice que hay que commitearlo, y el
// tablero da Hecho.
func TestCA289_E2ETaskDoneRegistraElCierre(t *testing.T) {
	bin := tdBinario(t)
	const slug = "precios"
	root, wt := tdTareaLista(t, bin, slug)
	itemPath := filepath.Join(root, ".hoom", "items", slug+".yaml")
	rdEscribir(t, root, ".hoom/items/"+slug+".yaml", tdItemYAML("Precios", ""))
	antes := tdYAML(t, itemPath)
	sha := rdGit(t, root, "rev-parse", "hoom/"+slug)

	t0 := time.Now().UTC().Truncate(time.Second)
	r := tdCorrer(t, bin, root, "task", "done", slug)
	t1 := time.Now().UTC()
	tdOK(t, "CA-289", r, "task", "done", slug)
	if tdExiste(wt) {
		t.Fatal("CA-289: task done quita el worktree")
	}
	want := fmt.Sprintf("hoom: item %s hecho (commit_final %s) - commitea .hoom/items/%s.yaml", slug, sha[:12], slug)
	if !strings.Contains(r.stdout, want) {
		t.Fatalf("CA-289: la salida dice %q:\n%s", want, r.stdout)
	}

	despues := tdYAML(t, itemPath)
	if despues["commit_final"] != sha || len(sha) != 40 {
		t.Fatalf("CA-289: commit_final es el sha completo de hoom/%s (%s): %v", slug, sha, despues["commit_final"])
	}
	s := tdFecha(despues["hecho_en"])
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).MatchString(s) {
		t.Fatalf("CA-289: hecho_en en UTC RFC3339 con segundos: %v", despues["hecho_en"])
	}
	hecho, _ := time.Parse(time.RFC3339, s)
	if hecho.Before(t0) || hecho.After(t1) {
		t.Fatalf("CA-289: hecho_en es el momento del cierre (%v..%v), fue %v", t0, t1, hecho)
	}
	for k, v := range antes {
		if despues[k] != v {
			t.Fatalf("CA-289: el cierre conserva %s: %v -> %v", k, v, despues[k])
		}
	}
	if len(despues) != len(antes)+2 {
		t.Fatalf("CA-289: solo se agregan hecho_en y commit_final: %v", despues)
	}

	b := tdCorrer(t, bin, root, "board", "--json")
	tdOK(t, "CA-289", b, "board", "--json")
	var tablero struct {
		Columns []struct {
			ID    string `json:"id"`
			Cards []struct {
				Slug   string `json:"slug"`
				Column string `json:"column"`
			} `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(b.stdout), &tablero); err != nil {
		t.Fatalf("CA-289: board --json: %v\n%s", err, b.stdout)
	}
	encontrada := false
	for _, col := range tablero.Columns {
		for _, c := range col.Cards {
			if c.Slug == slug {
				encontrada = true
				if col.ID != "hecho" || c.Column != "hecho" {
					t.Fatalf("CA-289: despues de task done la tarjeta es Hecho, esta en %s", col.ID)
				}
			}
		}
	}
	if !encontrada {
		t.Fatalf("CA-289: el tablero trae la tarjeta cerrada:\n%s", b.stdout)
	}
}

// CA-290: sin item, 'task done' cierra igual y lo dice.
func TestCA290_E2ETaskDoneSinItem(t *testing.T) {
	bin := tdBinario(t)
	root, wt := tdTareaLista(t, bin, "sin-item")
	r := tdCorrer(t, bin, root, "task", "done", "sin-item")
	tdOK(t, "CA-290", r, "task", "done")
	if tdExiste(wt) {
		t.Fatal("CA-290: sin item la tarea se cierra igual")
	}
	if !strings.Contains(r.stdout, "hoom: sin item .hoom/items/sin-item.yaml en este arbol: no hay tarjeta que cerrar") {
		t.Fatalf("CA-290: la salida dice que no hay item:\n%s", r.stdout)
	}
	if tdExiste(filepath.Join(root, ".hoom", "items", "sin-item.yaml")) {
		t.Fatal("CA-290: task done no inventa un item")
	}
}

// CA-290: con --force el worktree se va y el item queda byte a byte igual.
func TestCA290_E2ETaskDoneForceNoGanaHecho(t *testing.T) {
	bin := tdBinario(t)
	root, wt := tdTareaLista(t, bin, "forzada")
	rdEscribir(t, wt, "sucio.go", "package app\n") // sin las condiciones
	cuerpo := tdItemYAML("Forzada", "")
	rdEscribir(t, root, ".hoom/items/forzada.yaml", cuerpo)
	r := tdCorrer(t, bin, root, "task", "done", "forzada", "--force")
	tdOK(t, "CA-290", r, "task", "done", "--force")
	if tdExiste(wt) {
		t.Fatal("CA-290: --force quita el worktree")
	}
	if raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "items", "forzada.yaml")); string(raw) != cuerpo {
		t.Fatalf("CA-290: con --force el item queda byte a byte igual:\n%s", raw)
	}
	if !strings.Contains(r.stdout, "con --force el item forzada no se marca hecho") {
		t.Fatalf("CA-290: la salida dice que --force no gana Hecho:\n%s", r.stdout)
	}
}

// CA-290: un item invalido hace fallar 'task done' ANTES de tocar el
// worktree, con el motivo; el item queda igual.
func TestCA290_E2ETaskDoneItemInvalido(t *testing.T) {
	bin := tdBinario(t)
	root, wt := tdTareaLista(t, bin, "invalida")
	cuerpo := tdItemYAML("Invalida", "columna: hecho\n")
	rdEscribir(t, root, ".hoom/items/invalida.yaml", cuerpo)
	r := tdCorrer(t, bin, root, "task", "done", "invalida")
	if r.exit == 0 {
		t.Fatalf("CA-290: con un item invalido task done falla:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, "invalido") {
		t.Fatalf("CA-290: el error dice que el item es invalido:\n%s", r.stderr)
	}
	if !tdExiste(wt) {
		t.Fatal("CA-290: falla ANTES de quitar el worktree: el worktree sigue ahi")
	}
	if raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "items", "invalida.yaml")); string(raw) != cuerpo {
		t.Fatalf("CA-290: el item invalido queda igual:\n%s", raw)
	}
	// un item ilegible tambien
	rdEscribir(t, root, ".hoom/items/invalida.yaml", "titulo: [roto\n")
	if r := tdCorrer(t, bin, root, "task", "done", "invalida"); r.exit == 0 || !tdExiste(wt) {
		t.Fatalf("CA-290: un item ilegible tambien frena el cierre (exit %d)", r.exit)
	}
}

// CA-290: un item que ya tiene hecho_en no se reescribe, y la salida lo dice.
func TestCA290_E2ETaskDoneItemYaHecho(t *testing.T) {
	bin := tdBinario(t)
	root, wt := tdTareaLista(t, bin, "ya-hecha")
	cuerpo := tdItemYAML("Ya hecha", "hecho_en: 2026-09-20T10:00:00Z\ncommit_final: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\n")
	rdEscribir(t, root, ".hoom/items/ya-hecha.yaml", cuerpo)
	r := tdCorrer(t, bin, root, "task", "done", "ya-hecha")
	tdOK(t, "CA-290", r, "task", "done")
	if tdExiste(wt) {
		t.Fatal("CA-290: la tarea se cierra igual")
	}
	if raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "items", "ya-hecha.yaml")); string(raw) != cuerpo {
		t.Fatalf("CA-290: un item ya hecho no se reescribe:\n%s", raw)
	}
	if !strings.Contains(r.stdout, "ya estaba hecho") {
		t.Fatalf("CA-290: la salida dice que ya estaba hecho:\n%s", r.stdout)
	}
}

// tdFecha lee una fecha del YAML: sin comillas yaml.v3 la entrega como
// time.Time; con comillas, como string. Las dos formas valen.
func tdFecha(v any) string {
	if tt, ok := v.(time.Time); ok {
		return tt.Format(time.RFC3339Nano)
	}
	s, _ := v.(string)
	return s
}
