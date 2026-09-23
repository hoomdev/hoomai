// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// contra el BINARIO (CA-263..CA-267, CA-291): exit codes reales (uso = 2,
// error = 1, informar = 0), la identidad que registra 'hoom spec approve', y
// que un item no pone rojo 'hoom check'. Sin HOOM_TASK en el entorno.
package itemcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	ixOnce sync.Once
	ixBin  string
	ixDir  string
	ixErr  error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if ixDir != "" {
		os.RemoveAll(ixDir)
	}
	os.Exit(code)
}

func ixBinario(t *testing.T) string {
	t.Helper()
	ixOnce.Do(func() {
		dir, err := os.Getwd()
		if err != nil {
			ixErr = err
			return
		}
		for {
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				break
			}
			padre := filepath.Dir(dir)
			if padre == dir {
				ixErr = fmt.Errorf("no encontre go.mod hacia arriba")
				return
			}
			dir = padre
		}
		ixDir, err = os.MkdirTemp("", "hoom-itemcmd-bin-")
		if err != nil {
			ixErr = err
			return
		}
		bin := filepath.Join(ixDir, "hoom")
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/hoom")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			ixErr = fmt.Errorf("go build ./cmd/hoom: %v\n%s", err, out)
			return
		}
		ixBin = bin
	})
	if ixErr != nil {
		t.Fatal(ixErr)
	}
	return ixBin
}

type ixSalida struct {
	exit           int
	stdout, stderr string
}

func ixCorrer(t *testing.T, bin, dir string, args ...string) ixSalida {
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
	return ixSalida{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

// ixFotoItems lee .hoom/items/ entero: nombre -> contenido.
func ixFotoItems(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entradas, err := os.ReadDir(filepath.Join(root, ".hoom", "items"))
	if err != nil {
		if os.IsNotExist(err) {
			return out
		}
		t.Fatal(err)
	}
	for _, e := range entradas {
		raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "items", e.Name()))
		out[e.Name()] = string(raw)
	}
	return out
}

func ixMismos(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// CA-263: con el binario, creado_por es exactamente la identidad que
// registra 'hoom spec approve' en el mismo repo.
func TestCA263_E2EIdentidadComoSpecApprove(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/specs/otra.md", "# Spec: otra\n")
	if r := ixCorrer(t, bin, root, "spec", "approve", ".hoom/specs/otra.md"); r.exit != 0 {
		t.Fatalf("CA-263: spec approve salio %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	entradas, _ := os.ReadDir(filepath.Join(root, ".hoom", "approvals"))
	if len(entradas) != 1 {
		t.Fatalf("CA-263: una aprobacion esperada: %v", entradas)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "approvals", entradas[0].Name()))
	var ap struct {
		ApprovedBy string `json:"approved_by"`
	}
	if err := json.Unmarshal(raw, &ap); err != nil || ap.ApprovedBy == "" {
		t.Fatalf("CA-263: la aprobacion trae approved_by: %v %s", err, raw)
	}

	r := ixCorrer(t, bin, root, "item", "add", "Precios por región (v2)!")
	if r.exit != 0 {
		t.Fatalf("CA-263: item add salio %d\nstdout: %s\nstderr: %s", r.exit, r.stdout, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "hoom item: creado .hoom/items/precios-por-region-v2.yaml") || !strings.Contains(r.stdout, "commitealo") {
		t.Fatalf("CA-263: el texto del alta:\n%s", r.stdout)
	}
	item, err := os.ReadFile(filepath.Join(root, ".hoom", "items", "precios-por-region-v2.yaml"))
	if err != nil {
		t.Fatalf("CA-263: el archivo del item: %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(item, &m); err != nil {
		t.Fatal(err)
	}
	if m["creado_por"] != ap.ApprovedBy {
		t.Fatalf("CA-263: creado_por %q es la identidad de spec approve %q", m["creado_por"], ap.ApprovedBy)
	}

	r = ixCorrer(t, bin, root, "item", "add", "Precios por región (v2)!", "--slug", "otro", "--json")
	var js map[string]any
	if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &js) != nil || js["slug"] != "otro" {
		t.Fatalf("CA-263: --slug otro --json: exit %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
}

// CA-264: los rechazos son exit 2 por stderr y .hoom/items/ queda igual.
// CA-267: `hoom item` sin subcomando, con uno desconocido o show sin slug.
func TestCA264_E2ERechazosSonExit2SinEfectos(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/existente.yaml", icItemYAML("Existente", "2026-09-22T15:04:05Z", ""))
	antes := ixFotoItems(t, root)
	casos := [][]string{
		{"item"},
		{"item", "bogus"},
		{"item", "add"},
		{"item", "add", "A", "B"},
		{"item", "add", ""},
		{"item", "add", "¡¡¡"},
		{"item", "add", "X", "--tipo", "epica"},
		{"item", "add", "X", "--prioridad", "urgente"},
		{"item", "add", "X", "--presupuesto-usd", "0"},
		{"item", "add", "X", "--presupuesto-usd", "-1"},
		{"item", "add", "X", "--presupuesto-usd", "cinco"},
		{"item", "add", "X", "--tipo", ""},
		{"item", "add", "X", "--bogus"},
		{"item", "show"},
		{"item", "show", "Foo Bar"},
		{"item", "list", "x"},
	}
	for _, args := range casos {
		r := ixCorrer(t, bin, root, args...)
		if r.exit != 2 {
			t.Fatalf("CA-264: 'hoom %s' es error de uso (exit 2), salio %d\nstdout: %s\nstderr: %s",
				strings.Join(args, " "), r.exit, r.stdout, r.stderr)
		}
		if strings.TrimSpace(r.stderr) == "" {
			t.Fatalf("CA-264: 'hoom %s' explica el rechazo por stderr", strings.Join(args, " "))
		}
		if despues := ixFotoItems(t, root); !ixMismos(antes, despues) {
			t.Fatalf("CA-264: 'hoom %s' no deja efectos en .hoom/items/: %v", strings.Join(args, " "), despues)
		}
	}
	r := ixCorrer(t, bin, root, "item", "add", "¡¡¡")
	if !strings.Contains(r.stderr, "del titulo no sale ningun slug; usa --slug") {
		t.Fatalf("CA-264: el rechazo del titulo sin slug lo dice:\n%s", r.stderr)
	}
	r = ixCorrer(t, bin, root, "item")
	if !strings.Contains(r.stderr, "hoom item add \"<titulo>\"") {
		t.Fatalf("CA-267: 'hoom item' sin subcomando muestra el bloque de uso:\n%s", r.stderr)
	}
	// la ayuda no es un error
	if h := ixCorrer(t, bin, root, "item", "-h"); h.exit != 0 || !strings.Contains(h.stdout, "Uso: hoom item add") {
		t.Fatalf("CA-264: 'hoom item -h' es el uso por stdout con exit 0: %d\n%s", h.exit, h.stdout)
	}
}

// CA-265: un slug existente es exit 1, nombra el archivo, y el item existente
// queda byte a byte igual.
func TestCA265_E2ESlugExistenteExit1(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	original := icItemYAML("Precios", "2026-09-22T15:04:05Z", "pedido: el original\n")
	path := icEscribir(t, root, ".hoom/items/precios.yaml", original)
	r := ixCorrer(t, bin, root, "item", "add", "Precios")
	if r.exit != 1 {
		t.Fatalf("CA-265: el slug existente sale 1, salio %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, ".hoom/items/precios.yaml") || !strings.Contains(r.stderr, "--slug") {
		t.Fatalf("CA-265: el error nombra el archivo y sugiere --slug:\n%s", r.stderr)
	}
	if raw, _ := os.ReadFile(path); string(raw) != original {
		t.Fatalf("CA-265: el item existente queda byte a byte igual:\n%s", raw)
	}
	icEscribir(t, root, ".hoom/specs/catalogo.md", "# Spec: catalogo\n")
	if r := ixCorrer(t, bin, root, "item", "add", "Catalogo"); r.exit != 0 {
		t.Fatalf("CA-265: con el spec existente el item se crea (exit 0), salio %d\n%s", r.exit, r.stderr)
	}
}

// CA-266, CA-267: list y show informan con exit 0 aunque haya avisos; show
// de un slug valido sin archivo es exit 1.
func TestCA266_E2EListYShowExitCodes(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	icEscribir(t, root, ".hoom/items/malo.yaml", icItemYAML("Malo", "2026-09-22T15:04:05Z", "columna: hecho\n"))
	icEscribir(t, root, ".hoom/items/x.yml", icItemYAML("X", "2026-09-22T15:04:05Z", ""))

	r := ixCorrer(t, bin, root, "item", "list")
	if r.exit != 0 || !strings.Contains(r.stdout, "precios") {
		t.Fatalf("CA-266: list sale 0 con avisos y lista el valido: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	if !strings.Contains(r.stderr, "malo.yaml") || !strings.Contains(r.stderr, "clave desconocida") || !strings.Contains(r.stderr, "x.yml") {
		t.Fatalf("CA-266: los avisos por stderr nombran cada archivo y el motivo:\n%s", r.stderr)
	}
	r = ixCorrer(t, bin, root, "item", "list", "--json")
	var js struct {
		Warnings []string         `json:"warnings"`
		Items    []map[string]any `json:"items"`
	}
	if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &js) != nil || len(js.Items) != 1 || len(js.Warnings) < 2 {
		t.Fatalf("CA-266: list --json sale 0 con items y warnings: %d\n%s", r.exit, r.stdout)
	}
	if r := ixCorrer(t, bin, root, "item", "show", "precios"); r.exit != 0 {
		t.Fatalf("CA-267: show sale 0 aunque haya items invalidos: %d\n%s", r.exit, r.stderr)
	}
	r = ixCorrer(t, bin, root, "item", "show", "no-existe")
	if r.exit != 1 || !strings.Contains(r.stderr, "no existe el item") {
		t.Fatalf("CA-267: show de un slug sin archivo sale 1 con 'no existe el item': %d\n%s", r.exit, r.stderr)
	}
}

// CA-291: crear un item (sin commitear o commiteado) y un registro de
// review no pone rojo 'hoom check'.
func TestCA291_E2ECheckSigueVerde(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	if r := ixCorrer(t, bin, root, "verify"); r.exit != 0 {
		t.Fatalf("CA-291: verify verde de partida, salio %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	if r := ixCorrer(t, bin, root, "check"); r.exit != 0 {
		t.Fatalf("CA-291: check verde de partida, salio %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	if r := ixCorrer(t, bin, root, "item", "add", "Precios"); r.exit != 0 {
		t.Fatalf("CA-291: item add salio %d\n%s", r.exit, r.stderr)
	}
	icEscribir(t, root, ".hoom/reviews/20260922T150405_ab12cd.json",
		`{"id":"20260922T150405_ab12cd","task":"precios","lenses":["reliability"],"findings":[]}`+"\n")
	if r := ixCorrer(t, bin, root, "check"); r.exit != 0 {
		t.Fatalf("CA-291: un item y un registro de review SIN commitear no ponen rojo check, salio %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	icGit(t, root, "add", "-A")
	icGit(t, root, "commit", "-m", "item y review")
	if r := ixCorrer(t, bin, root, "check"); r.exit != 0 {
		t.Fatalf("CA-291: commiteados tampoco, salio %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
}
