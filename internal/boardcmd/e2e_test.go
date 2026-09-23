// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// contra el BINARIO real (CA-281, CA-288, CA-299): el mapeo de errores a exit
// codes vive en main, y el ciclo completo de una tarjeta solo se ve de punta
// a punta. Repos temporales, gates reemplazados por `true`, sin HOOM_TASK en
// el entorno y sin ningun CLI de IA.
package boardcmd

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

	"github.com/hoomdev/hoomai/internal/envelope"
)

var (
	bxOnce sync.Once
	bxBin  string
	bxDir  string
	bxErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if bxDir != "" {
		os.RemoveAll(bxDir)
	}
	os.Exit(code)
}

func bxModulo() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		padre := filepath.Dir(dir)
		if padre == dir {
			return "", fmt.Errorf("no encontre go.mod hacia arriba")
		}
		dir = padre
	}
}

// bxBinario compila ./cmd/hoom una sola vez por corrida del paquete.
func bxBinario(t *testing.T) string {
	t.Helper()
	bxOnce.Do(func() {
		mod, err := bxModulo()
		if err != nil {
			bxErr = err
			return
		}
		bxDir, err = os.MkdirTemp("", "hoom-boardcmd-bin-")
		if err != nil {
			bxErr = err
			return
		}
		bin := filepath.Join(bxDir, "hoom")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/hoom")
		cmd.Dir = mod
		if out, err := cmd.CombinedOutput(); err != nil {
			bxErr = fmt.Errorf("go build ./cmd/hoom: %v\n%s", err, out)
			return
		}
		bxBin = bin
	})
	if bxErr != nil {
		t.Fatal(bxErr)
	}
	return bxBin
}

type bxSalida struct {
	exit           int
	stdout, stderr string
}

// bxCorrer ejecuta el binario sin HOOM_TASK en el entorno.
func bxCorrer(t *testing.T, bin, dir string, args ...string) bxSalida {
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
	return bxSalida{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

func bxOK(t *testing.T, ca string, r bxSalida, args ...string) {
	t.Helper()
	if r.exit != 0 {
		t.Fatalf("%s: hoom %v salio %d\nstdout: %s\nstderr: %s", ca, args, r.exit, r.stdout, r.stderr)
	}
}

type bxTablero struct {
	Columns []struct {
		ID    string `json:"id"`
		Name  string `json:"name"`
		Human bool   `json:"human"`
		Cards []Card `json:"cards"`
	} `json:"columns"`
	Warnings []string `json:"warnings"`
}

func bxBoard(t *testing.T, bin, dir string) (bxTablero, string) {
	t.Helper()
	r := bxCorrer(t, bin, dir, "board", "--json")
	bxOK(t, "CA-288", r, "board", "--json")
	var b bxTablero
	if err := json.Unmarshal([]byte(r.stdout), &b); err != nil {
		t.Fatalf("CA-288: 'hoom board --json' es JSON: %v\n%s", err, r.stdout)
	}
	return b, r.stdout
}

func bxCard(t *testing.T, bin, dir, slug string) Card {
	t.Helper()
	b, raw := bxBoard(t, bin, dir)
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if c.Slug == slug {
				if c.Column != col.ID {
					t.Fatalf("CA-288: la tarjeta %s esta bajo %s pero dice %s", slug, col.ID, c.Column)
				}
				return c
			}
		}
	}
	t.Fatalf("CA-299: el tablero no tiene la tarjeta %s:\n%s", slug, raw)
	return Card{}
}

func bxColumna(t *testing.T, ca, bin, dir, slug, want string) Card {
	t.Helper()
	c := bxCard(t, bin, dir, slug)
	if c.Column != want {
		t.Fatalf("%s: la tarjeta %s debe estar en %s, esta en %s (missing=%q next=%q)", ca, slug, want, c.Column, c.Missing, c.Next)
	}
	return c
}

// CA-299: el ciclo completo con el binario real, leyendo cada paso de
// 'hoom board --json'.
func TestCA299_E2ECicloCompletoDeUnaTarjeta(t *testing.T) {
	bin := bxBinario(t)
	root := bdRepo(t, "findings:\n  block_on: high\n")
	const slug = "demo-e2e"
	spec := ".hoom/specs/" + slug + ".md"

	r := bxCorrer(t, bin, root, "item", "add", "Demo E2E", "--pedido", "que la tarjeta recorra el tablero")
	bxOK(t, "CA-299", r, "item", "add")
	c := bxColumna(t, "CA-299 (item add)", bin, root, slug, ColBacklog)
	if c.Next != "hoom task start "+slug {
		t.Fatalf("CA-299: en backlog sin tarea el siguiente paso es 'hoom task start %s', fue %q", slug, c.Next)
	}

	bxOK(t, "CA-299", bxCorrer(t, bin, root, "task", "start", slug), "task", "start")
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	c = bxColumna(t, "CA-299 (task start)", bin, root, slug, ColBacklog)
	if c.Evidence.Source != SourceWorktree {
		t.Fatalf("CA-299: con la tarea creada la evidencia es el worktree: %+v", c.Evidence)
	}

	criterios := "- CA-1: la tarjeta recorre el tablero."
	bdEscribir(t, wt, spec, bdSpecTexto(criterios, false))
	c = bxColumna(t, "CA-299 (spec sin seccion)", bin, root, slug, ColArquitecto)
	bdTiene(t, "CA-299", c.Missing, `falta la seccion "riesgos"`)

	bdEscribir(t, wt, spec, bdSpecTexto(criterios, true))
	bxColumna(t, "CA-299 (spec completo)", bin, root, slug, ColTuAprobacion)

	bxOK(t, "CA-299", bxCorrer(t, bin, wt, "spec", "approve", spec), "spec", "approve")
	c = bxColumna(t, "CA-299 (spec approve)", bin, root, slug, ColTestWriter)
	bdTiene(t, "CA-299", c.Missing, "CA-1")

	bdEscribir(t, wt, "demo_e2e_test.go", "package app\n\n// CA-1: la tarjeta recorre el tablero\n")
	bxColumna(t, "CA-299 (test que cita los CA)", bin, root, slug, ColWriter)

	bxOK(t, "CA-299", bxCorrer(t, bin, wt, "verify", "--spec", spec), "verify", "--spec")
	bdCommitear(t, wt, "spec, aprobacion, test y veredicto")
	c = bxColumna(t, "CA-299 (verify verde y commit)", bin, root, slug, ColTuAceptacion)
	if !c.WaitingHuman || c.Next != "hoom task done "+slug {
		t.Fatalf("CA-299: en Tu aceptacion espera al humano con 'hoom task done %s': %+v", slug, c)
	}

	r = bxCorrer(t, bin, root, "task", "done", slug)
	bxOK(t, "CA-299", r, "task", "done")
	c = bxColumna(t, "CA-299 (task done)", bin, root, slug, ColHecho)
	if c.Next != "" {
		t.Fatalf("CA-299: Hecho no tiene siguiente paso: %q", c.Next)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "items", slug+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	sha := bdGit(t, root, "rev-parse", "hoom/"+slug)
	if m["commit_final"] != sha {
		t.Fatalf("CA-299: commit_final es el sha completo de hoom/%s (%s): %v", slug, sha, m["commit_final"])
	}
	if s := bdFecha(m["hecho_en"]); !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`).MatchString(s) {
		t.Fatalf("CA-299: hecho_en en UTC RFC3339 con segundos: %v", m["hecho_en"])
	}
	if m["titulo"] != "Demo E2E" || m["pedido"] != "que la tarjeta recorra el tablero" {
		t.Fatalf("CA-299: el cierre conserva los demas campos: %v", m)
	}
}

// CA-288: con el binario: 8 columnas en orden, human solo en las humanas,
// avisos de items invalidos, texto con los conteos y exit 0; 'board x' y
// 'board --bogus' son exit 2.
func TestCA288_E2EBoard(t *testing.T) {
	bin := bxBinario(t)
	root := bdRepo(t, "findings:\n  block_on: high\n")

	txt := bxCorrer(t, bin, root, "board")
	bxOK(t, "CA-288", txt, "board")
	if strings.TrimSpace(txt.stdout) != `hoom board: sin items (crea uno con 'hoom item add "<titulo>"')` {
		t.Fatalf("CA-288: sin items el tablero lo dice:\n%s", txt.stdout)
	}
	b, raw := bxBoard(t, bin, root)
	if len(b.Columns) != 8 || b.Warnings == nil || strings.Contains(raw, `"cards": null`) || strings.Contains(raw, `"cards":null`) {
		t.Fatalf("CA-288: sin items igual van las 8 columnas y warnings []:\n%s", raw)
	}

	bdItemArchivo(t, root, "uno", "Uno", "")
	bdItemArchivo(t, root, "dos", "Dos", "")
	bdEscribir(t, root, ".hoom/items/malo.yaml", bdItemYAML("Malo", "columna: hecho\n"))
	b, raw = bxBoard(t, bin, root)
	var ids []string
	for _, col := range b.Columns {
		ids = append(ids, col.ID)
		if col.Human != (col.ID == ColTuAprobacion || col.ID == ColTuAceptacion) {
			t.Fatalf("CA-288: human solo en tu-aprobacion y tu-aceptacion: %+v", col)
		}
	}
	if strings.Join(ids, ",") != "backlog,arquitecto,tu-aprobacion,test-writer,writer,review,tu-aceptacion,hecho" {
		t.Fatalf("CA-288: el orden de las columnas: %v", ids)
	}
	bdTiene(t, "CA-288: el item invalido va a warnings", b.Warnings, "malo.yaml")
	if len(b.Columns[0].Cards) != 2 {
		t.Fatalf("CA-288: dos tarjetas en backlog:\n%s", raw)
	}

	txt = bxCorrer(t, bin, root, "board")
	bxOK(t, "CA-288", txt, "board")
	for _, want := range []string{
		"hoom board: 2 tarjetas (columna derivada de la evidencia; nada guardado)",
		"Backlog (2)", "Arquitecto (0)", "Tu aprobacion (0) - esperando humano", "Test-writer (0)",
		"Writer (0)", "Review (0)", "Tu aceptacion (0) - esperando humano", "Hecho (0)",
		"no hay spec: .hoom/specs/uno.md", "siguiente: hoom task start uno", "siguiente: hoom task start dos",
	} {
		if !strings.Contains(txt.stdout, want) {
			t.Fatalf("CA-288: al texto del tablero le falta %q:\n%s", want, txt.stdout)
		}
	}

	for _, args := range [][]string{{"board", "x"}, {"board", "--bogus"}, {"board", "--json", "x"}} {
		r := bxCorrer(t, bin, root, args...)
		if r.exit != 2 {
			t.Fatalf("CA-288: 'hoom %s' es error de uso (exit 2), salio %d\n%s%s", strings.Join(args, " "), r.exit, r.stdout, r.stderr)
		}
	}
	if r := bxCorrer(t, bin, root, "board", "-h"); r.exit != 0 || !strings.Contains(r.stdout, "Uso: hoom board [--json]") {
		t.Fatalf("CA-288: 'hoom board -h' es el uso por stdout con exit 0: %d\n%s", r.exit, r.stdout)
	}
}

// CA-281: 'hoom board' y 'hoom item show' no crean ni modifican ningun
// archivo del repo ni de .hoom/ (ignorados incluidos).
func TestCA281_E2EBoardYShowSoloLeen(t *testing.T) {
	bin := bxBinario(t)
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdItemArchivo(t, root, "arbol", "Arbol", "")
	bdEscribir(t, root, ".hoom/specs/arbol.md", bdSpecTexto("- CA-1: se marca. [verifica: touch marca]\n- CA-2: sin test.", true))
	bdAprobar(t, root, "arbol")
	bdVeredictoDisco(t, root, ".hoom/specs/arbol.md", bdT0, false, bdGatesVerdes())
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, "sucio.go", "package app\n")
	// un sobre sin cerrar con un latido viejo (interrumpido): el tablero lo
	// rotula, no lo cierra ni lo reescribe
	viejo := time.Now().UTC().Add(-2 * time.Hour)
	bdSobreDisco(t, root, envelope.Record{ID: "20260922T100000_sobre1", Role: "writer", Provider: "claude", Task: bdSlug,
		Dir: wt, Stage: "run", Step: 2, Steps: 5, Status: envelope.StatusRunning, StartedAt: viejo, UpdatedAt: viejo})

	antes := bdFoto(t, root)
	for _, args := range [][]string{{"board"}, {"board", "--json"}, {"item", "show", bdSlug}, {"item", "show", bdSlug, "--json"},
		{"item", "show", "arbol", "--json"}, {"item", "list"}, {"item", "list", "--json"}} {
		r := bxCorrer(t, bin, root, args...)
		bxOK(t, "CA-281", r, args...)
	}
	bdMismaFoto(t, "CA-281", antes, bdFoto(t, root))
	if _, err := os.Stat(filepath.Join(root, "marca")); !os.IsNotExist(err) {
		t.Fatal("CA-272: 'hoom board' no ejecuta el marcador verifica ('marca' aparecio)")
	}

	// y show --json trae la MISMA tarjeta que board --json
	r := bxCorrer(t, bin, root, "item", "show", bdSlug, "--json")
	var show struct {
		Item map[string]any  `json:"item"`
		Card json.RawMessage `json:"card"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &show); err != nil || show.Item["slug"] != bdSlug {
		t.Fatalf("CA-267: 'hoom item show --json' es {\"item\", \"card\"}: %v\n%s", err, r.stdout)
	}
	var deShow, deBoard any
	_ = json.Unmarshal(show.Card, &deShow)
	c := bxCard(t, bin, root, bdSlug)
	cr, _ := json.Marshal(c)
	_ = json.Unmarshal(cr, &deBoard)
	sa, _ := json.Marshal(deShow)
	sb, _ := json.Marshal(deBoard)
	if string(sa) != string(sb) {
		t.Fatalf("CA-267: show y board dan la misma tarjeta:\nshow:  %s\nboard: %s", sa, sb)
	}
}

// bdFecha lee una fecha del YAML: sin comillas yaml.v3 la entrega como
// time.Time; con comillas, como string. Las dos formas valen.
func bdFecha(v any) string {
	if tt, ok := v.(time.Time); ok {
		return tt.Format(time.RFC3339Nano)
	}
	s, _ := v.(string)
	return s
}
