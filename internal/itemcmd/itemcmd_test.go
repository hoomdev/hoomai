// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-263..CA-267) sobre el verbo `hoom item` a nivel paquete: Parse es puro
// y estricto (cliargs.Operands), Execute escribe el archivo, lista y muestra
// el item con su tarjeta.
package itemcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
)

func icGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func icEscribir(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func icRepo(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	icGit(t, root, "init", "-b", "main")
	icGit(t, root, "config", "user.email", "test@hoom.dev")
	icGit(t, root, "config", "user.name", "hoom test")
	icEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	icGit(t, root, "add", "-A")
	icGit(t, root, "commit", "-m", "inicial")
	return root
}

func icItemYAML(titulo, creado string, extra string) string {
	return "titulo: " + titulo + "\ntipo: feature\nprioridad: media\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: " + creado + "\n" + extra
}

// icUso exige *cliargs.UsageError con exit 2.
func icUso(t *testing.T, ca string, args []string, err error) *cliargs.UsageError {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: %q debe ser *cliargs.UsageError, fue nil", ca, args)
	}
	var ue *cliargs.UsageError
	if !errors.As(err, &ue) {
		t.Fatalf("%s: %q debe ser *cliargs.UsageError, fue %T: %v", ca, args, err, err)
	}
	if ue.ExitCode() != 2 {
		t.Fatalf("%s: exit 2, fue %d", ca, ue.ExitCode())
	}
	return ue
}

func icExec(t *testing.T, root string, args ...string) (string, string, error) {
	t.Helper()
	req, err := Parse(args)
	if err != nil {
		t.Fatalf("Parse(%q) no debia fallar: %v", args, err)
	}
	var out, errb bytes.Buffer
	err = Execute(root, "main", "high", req, &out, &errb)
	return out.String(), errb.String(), err
}

func icItemsEnDisco(t *testing.T, root string) []string {
	t.Helper()
	entradas, err := os.ReadDir(filepath.Join(root, ".hoom", "items"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entradas {
		out = append(out, e.Name())
	}
	return out
}

// CA-264: Parse acepta la sintaxis del uso, con flags antes o despues.
func TestCA264_ParseAdd(t *testing.T) {
	req, err := Parse([]string{"add", "Arreglar login", "--tipo", "bug", "--prioridad", "alta",
		"--presupuesto-usd", "2.5", "--pedido", "que no se cuelgue", "--slug", "login", "--json"})
	if err != nil {
		t.Fatalf("CA-264: pedido valido: %v", err)
	}
	d := req.Draft
	if req.Sub != SubAdd || !req.JSON || d.Titulo != "Arreglar login" || d.Tipo != "bug" || d.Prioridad != "alta" ||
		d.Pedido != "que no se cuelgue" || d.Slug != "login" || d.PresupuestoUSD == nil || *d.PresupuestoUSD != 2.5 {
		t.Fatalf("CA-264: el Request trae cada flag: %+v %+v", req, d)
	}
	req, err = Parse([]string{"add", "--tipo", "docs", "Guia"})
	if err != nil || req.Draft.Titulo != "Guia" || req.Draft.Tipo != "docs" {
		t.Fatalf("CA-264: flags antes del titulo: %+v %v", req, err)
	}
	req, err = Parse([]string{"add", "--", "-titulo con guion"})
	if err != nil || req.Draft.Titulo != "-titulo con guion" {
		t.Fatalf("CA-264: un titulo que empieza con '-' va despues de '--': %+v %v", req, err)
	}
	for _, args := range [][]string{{"list"}, {"list", "--json"}, {"show", "precios"}, {"show", "precios", "--json"}, {"show", "--json", "precios"}} {
		req, err := Parse(args)
		if err != nil {
			t.Fatalf("CA-264: %q es valido: %v", args, err)
		}
		if args[0] == "show" && req.Slug != "precios" {
			t.Fatalf("CA-264: show lleva el slug: %+v", req)
		}
		if req.JSON != (len(args) == 3 || args[len(args)-1] == "--json") {
			t.Fatalf("CA-264: --json en %q: %+v", args, req)
		}
	}
}

// CA-264: todo lo que el verbo no entiende es *UsageError, sin efectos.
// CA-267: sin subcomando o con uno desconocido tambien, con el bloque de uso.
func TestCA264_ParseRechaza(t *testing.T) {
	malos := [][]string{
		{"add"},
		{"add", "A", "B"},
		{"add", ""},
		{"add", "   "},
		{"add", "¡¡¡"},
		{"add", "X", "--tipo", "epica"},
		{"add", "X", "--prioridad", "urgente"},
		{"add", "X", "--presupuesto-usd", "0"},
		{"add", "X", "--presupuesto-usd", "-1"},
		{"add", "X", "--presupuesto-usd", "cinco"},
		{"add", "X", "--tipo", ""},
		{"add", "X", "--pedido="},
		{"add", "X", "--slug", ""},
		{"add", "X", "--slug", "Foo Bar"},
		{"add", "X", "--bogus"},
		{"add", "X", "--tipo"},
		{"list", "x"},
		{"list", "--bogus"},
		{"show"},
		{"show", "a", "b"},
		{"show", "Foo Bar"},
		{"show", "-x"},
		{"show", ""},
	}
	for _, args := range malos {
		_, err := Parse(args)
		icUso(t, "CA-264", args, err)
	}
	_, err := Parse([]string{"add", "¡¡¡"})
	ue := icUso(t, "CA-264", []string{"add", "¡¡¡"}, err)
	if !strings.Contains(ue.Error(), "del titulo no sale ningun slug; usa --slug") {
		t.Fatalf("CA-264: el rechazo del titulo sin slug lo dice: %v", ue)
	}

	for _, args := range [][]string{nil, {}, {"bogus"}, {"--json"}} {
		_, err := Parse(args)
		ue := icUso(t, "CA-267", args, err)
		for _, l := range strings.Split(UsageText, "\n") {
			if l = strings.TrimRight(l, " "); strings.TrimSpace(l) != "" && !strings.Contains(ue.Error(), l) {
				t.Fatalf("CA-267: 'hoom item %s' lleva el bloque de uso; falta %q:\n%s", strings.Join(args, " "), l, ue.Error())
			}
		}
	}
	for _, args := range [][]string{{"-h"}, {"--help"}, {"add", "-h"}, {"show", "--help"}} {
		if _, err := Parse(args); !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-264: %q es ErrHelp, fue %v", args, err)
		}
	}
}

// CA-263: `item add "Precios por region (v2)!"` crea el archivo con los
// defaults, la identidad del proyecto y creado_en en UTC; el texto dice que
// hay que commitearlo.
func TestCA263_ExecuteAdd(t *testing.T) {
	root := icRepo(t)
	antes := time.Now().UTC().Truncate(time.Second)
	out, _, err := icExec(t, root, "add", "Precios por región (v2)!")
	if err != nil {
		t.Fatalf("CA-263: item add: %v", err)
	}
	path := filepath.Join(root, ".hoom", "items", "precios-por-region-v2.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("CA-263: falta .hoom/items/precios-por-region-v2.yaml: %v", err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["titulo"] != "Precios por región (v2)!" || m["tipo"] != "feature" || m["prioridad"] != "media" ||
		m["creado_por"] != gitx.Identity(root) {
		t.Fatalf("CA-263: contenido del item: %v", m)
	}
	for _, k := range []string{"presupuesto_usd", "hecho_en", "commit_final"} {
		if _, ok := m[k]; ok {
			t.Fatalf("CA-263: el item nuevo no lleva %s: %v", k, m)
		}
	}
	s := icFecha(m["creado_en"])
	creado, err := time.Parse(time.RFC3339, s)
	if err != nil || !strings.HasSuffix(s, "Z") || strings.Contains(s, ".") || creado.Before(antes) {
		t.Fatalf("CA-263: creado_en en UTC RFC3339 con segundos: %q (%v)", s, err)
	}
	if !strings.HasPrefix(out, "hoom item: creado .hoom/items/precios-por-region-v2.yaml") {
		t.Fatalf("CA-263: la primera linea dice que archivo creo:\n%s", out)
	}
	if !strings.Contains(out, "Precios por región (v2)!") || !strings.Contains(out, "commitealo") ||
		!strings.Contains(strings.ToLower(out), "backlog") {
		t.Fatalf("CA-263: el texto trae el titulo, la columna (backlog) y 'commitealo':\n%s", out)
	}

	// --slug fija el slug
	if _, _, err := icExec(t, root, "add", "Precios por región (v2)!", "--slug", "otro"); err != nil {
		t.Fatalf("CA-263: --slug otro: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "items", "otro.yaml")); err != nil {
		t.Fatalf("CA-263: --slug otro crea .hoom/items/otro.yaml: %v", err)
	}
}

// CA-264: los flags quedan en el archivo. --json emite el item con las
// claves del YAML mas slug; pedido siempre presente.
func TestCA264_ExecuteAddConFlagsYJSON(t *testing.T) {
	root := icRepo(t)
	out, _, err := icExec(t, root, "add", "Arreglar login", "--tipo", "bug", "--prioridad", "alta",
		"--presupuesto-usd", "5", "--pedido", "que el login no se cuelgue", "--json")
	if err != nil {
		t.Fatalf("CA-264: %v", err)
	}
	var js map[string]any
	if err := json.Unmarshal([]byte(out), &js); err != nil {
		t.Fatalf("CA-264: --json emite el item como JSON: %v\n%s", err, out)
	}
	if js["slug"] != "arreglar-login" || js["tipo"] != "bug" || js["prioridad"] != "alta" ||
		js["pedido"] != "que el login no se cuelgue" || js["presupuesto_usd"] != float64(5) {
		t.Fatalf("CA-264: el JSON del item: %v", js)
	}
	var m map[string]any
	raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "items", "arreglar-login.yaml"))
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["tipo"] != "bug" || m["prioridad"] != "alta" || m["pedido"] != "que el login no se cuelgue" {
		t.Fatalf("CA-264: los flags quedan en el archivo: %v", m)
	}
	if _, ok := m["presupuesto_usd"].(int); !ok {
		if _, ok := m["presupuesto_usd"].(float64); !ok {
			t.Fatalf("CA-264: presupuesto_usd es un numero en el YAML: %T %v", m["presupuesto_usd"], m["presupuesto_usd"])
		}
	}

	// sin pedido: la clave igual esta en el JSON; sin presupuesto no
	out, _, err = icExec(t, root, "add", "Otro", "--json")
	if err != nil {
		t.Fatal(err)
	}
	js = map[string]any{}
	if err := json.Unmarshal([]byte(out), &js); err != nil {
		t.Fatalf("CA-264: %v\n%s", err, out)
	}
	quiere := []string{"slug", "titulo", "tipo", "prioridad", "pedido", "creado_por", "creado_en"}
	for _, k := range quiere {
		if _, ok := js[k]; !ok {
			t.Fatalf("CA-264: al JSON del item le falta %q: %v", k, js)
		}
	}
	if len(js) != len(quiere) {
		t.Fatalf("CA-264: sin presupuesto_usd/hecho_en/commit_final el JSON trae solo %v: %v", quiere, js)
	}
}

// CA-265: un slug que ya existe es error (envuelve item.ErrExists), nombra
// el archivo y sugiere --slug; el item queda byte a byte igual. Con un spec
// de ese slug, el item se crea.
func TestCA265_ExecuteAddExistente(t *testing.T) {
	root := icRepo(t)
	original := icItemYAML("Precios", "2026-09-22T15:04:05Z", "pedido: el de antes\n")
	path := icEscribir(t, root, ".hoom/items/precios.yaml", original)
	_, _, err := icExec(t, root, "add", "Precios")
	if err == nil || !errors.Is(err, item.ErrExists) {
		t.Fatalf("CA-265: un slug existente envuelve item.ErrExists: %v", err)
	}
	var ue *cliargs.UsageError
	if errors.As(err, &ue) {
		t.Fatalf("CA-265: el slug existente es error comun (exit 1), no de uso: %v", err)
	}
	if !strings.Contains(err.Error(), ".hoom/items/precios.yaml") || !strings.Contains(err.Error(), "--slug") {
		t.Fatalf("CA-265: el error nombra el archivo y sugiere --slug: %v", err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != original {
		t.Fatalf("CA-265: el item existente queda byte a byte igual:\n%s", raw)
	}

	icEscribir(t, root, ".hoom/specs/catalogo.md", "# Spec: catalogo\n")
	if _, _, err := icExec(t, root, "add", "Catalogo"); err != nil {
		t.Fatalf("CA-265: adoptar un spec existente no es error: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "items", "catalogo.yaml")); err != nil {
		t.Fatalf("CA-265: el item se crea: %v", err)
	}
}

// CA-266: list ordena por creado_en y slug, una linea por item; los
// invalidos van como aviso por stderr y el comando no falla. --json emite
// {"warnings", "items"} sin null.
func TestCA266_ExecuteList(t *testing.T) {
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/beta.yaml", icItemYAML("Beta", "2026-09-22T10:00:00Z", ""))
	icEscribir(t, root, ".hoom/items/alfa.yaml", icItemYAML("Alfa", "2026-09-22T10:00:00Z", ""))
	icEscribir(t, root, ".hoom/items/zeta.yaml", strings.Replace(icItemYAML("Zeta", "2026-09-21T10:00:00Z", ""), "tipo: feature", "tipo: bug", 1))
	icEscribir(t, root, ".hoom/items/malo.yaml", icItemYAML("Malo", "2026-09-22T10:00:00Z", "columna: hecho\n"))
	icEscribir(t, root, ".hoom/items/Foo Bar.yaml", icItemYAML("Foo", "2026-09-22T10:00:00Z", ""))

	out, errOut, err := icExec(t, root, "list")
	if err != nil {
		t.Fatalf("CA-266: list informa y no bloquea: %v", err)
	}
	iz, ia, ib := strings.Index(out, "zeta"), strings.Index(out, "alfa"), strings.Index(out, "beta")
	if iz < 0 || ia < 0 || ib < 0 || !(iz < ia && ia < ib) {
		t.Fatalf("CA-266: orden zeta, alfa, beta (creado_en y despues slug):\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "zeta") && (!strings.Contains(l, "bug") || !strings.Contains(l, "media") || !strings.Contains(l, "Zeta")) {
			t.Fatalf("CA-266: una linea por item con slug, tipo, prioridad y titulo: %q", l)
		}
	}
	if strings.Contains(out, "Malo") || strings.Contains(out, "Foo Bar") {
		t.Fatalf("CA-266: los invalidos no se listan:\n%s", out)
	}
	if !strings.Contains(errOut, "malo.yaml") || !strings.Contains(errOut, "Foo Bar.yaml") {
		t.Fatalf("CA-266: el aviso por stderr nombra cada archivo invalido:\n%s", errOut)
	}

	out, _, err = icExec(t, root, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var js struct {
		Warnings []string         `json:"warnings"`
		Items    []map[string]any `json:"items"`
	}
	var crudo map[string]any
	if err := json.Unmarshal([]byte(out), &crudo); err != nil {
		t.Fatalf("CA-266: list --json es JSON: %v\n%s", err, out)
	}
	if _, ok := crudo["warnings"].([]any); !ok {
		t.Fatalf("CA-266: warnings es una lista: %v", crudo["warnings"])
	}
	if err := json.Unmarshal([]byte(out), &js); err != nil {
		t.Fatal(err)
	}
	if len(js.Items) != 3 || js.Items[0]["slug"] != "zeta" || js.Items[1]["slug"] != "alfa" || js.Items[2]["slug"] != "beta" {
		t.Fatalf("CA-266: items en orden con slug: %v", js.Items)
	}
	if _, ok := js.Items[0]["pedido"]; !ok {
		t.Fatalf("CA-266: cada item trae las claves del YAML (pedido incluido): %v", js.Items[0])
	}
	unidos := strings.Join(js.Warnings, "\n")
	if !strings.Contains(unidos, "malo.yaml") || !strings.Contains(unidos, "Foo Bar.yaml") {
		t.Fatalf("CA-266: los avisos viajan en warnings: %v", js.Warnings)
	}

	// sin avisos: warnings es [] y no null
	limpio := icRepo(t)
	out, _, err = icExec(t, limpio, "list", "--json")
	if err != nil {
		t.Fatal(err)
	}
	crudo = map[string]any{}
	if err := json.Unmarshal([]byte(out), &crudo); err != nil {
		t.Fatalf("CA-266: %v\n%s", err, out)
	}
	if w, ok := crudo["warnings"].([]any); !ok || len(w) != 0 {
		t.Fatalf("CA-266: sin avisos, \"warnings\": [] (nunca null): %s", out)
	}
	if it, ok := crudo["items"].([]any); !ok || len(it) != 0 {
		t.Fatalf("CA-266: sin items, \"items\": []: %s", out)
	}
}

// CA-267: show --json emite {"item", "card"} con la MISMA tarjeta que el
// tablero para ese slug. Un slug valido sin archivo es error comun.
func TestCA267_ExecuteShow(t *testing.T) {
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", "pedido: por region\n"))
	icEscribir(t, root, ".hoom/items/otro.yaml", icItemYAML("Otro", "2026-09-22T16:04:05Z", ""))

	out, _, err := icExec(t, root, "show", "precios", "--json")
	if err != nil {
		t.Fatalf("CA-267: show --json: %v", err)
	}
	var js map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &js); err != nil {
		t.Fatalf("CA-267: show --json es JSON: %v\n%s", err, out)
	}
	if _, ok := js["item"]; !ok || len(js) != 2 {
		t.Fatalf("CA-267: show --json es {\"item\", \"card\"}: %s", out)
	}
	var it map[string]any
	if err := json.Unmarshal(js["item"], &it); err != nil || it["slug"] != "precios" || it["titulo"] != "Precios" {
		t.Fatalf("CA-267: el item del show: %v %v", it, err)
	}
	var card any
	if err := json.Unmarshal(js["card"], &card); err != nil {
		t.Fatalf("CA-267: la tarjeta del show: %v", err)
	}

	b, err := boardcmd.Build(root, "main", "high", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var delTablero any
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if c.Slug == "precios" {
				raw, _ := json.Marshal(c)
				_ = json.Unmarshal(raw, &delTablero)
			}
		}
	}
	if delTablero == nil {
		t.Fatalf("CA-267: el tablero tiene la tarjeta de precios: %+v", b)
	}
	if !reflect.DeepEqual(card, delTablero) {
		t.Fatalf("CA-267: show y board dan la misma tarjeta:\nshow:  %v\nboard: %v", card, delTablero)
	}

	txt, _, err := icExec(t, root, "show", "precios")
	if err != nil || !strings.Contains(txt, "precios") || !strings.Contains(txt, "Precios") {
		t.Fatalf("CA-267: show en texto trae el item: %v\n%s", err, txt)
	}

	_, _, err = icExec(t, root, "show", "no-existe")
	if err == nil || !strings.Contains(err.Error(), "no existe el item") {
		t.Fatalf("CA-267: un slug valido sin archivo es error 'no existe el item': %v", err)
	}
	var ue *cliargs.UsageError
	if errors.As(err, &ue) {
		t.Fatalf("CA-267: el item inexistente es exit 1, no de uso: %v", err)
	}
	if got := icItemsEnDisco(t, root); len(got) != 2 {
		t.Fatalf("CA-267: show no crea items: %v", got)
	}
}

// icFecha lee una fecha del YAML: sin comillas yaml.v3 la entrega como
// time.Time; con comillas, como string. Las dos formas valen.
func icFecha(v any) string {
	if tt, ok := v.(time.Time); ok {
		return tt.Format(time.RFC3339Nano)
	}
	s, _ := v.(string)
	return s
}
