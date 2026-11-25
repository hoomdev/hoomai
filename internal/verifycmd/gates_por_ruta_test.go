//go:build !windows

// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre
// verifycmd, de punta a punta en repos de git temporales:
//
//   - CA-810 (CAL-9.1): con solo resources/js/app.js cambiado y aplica_en:
//     [app/**] en static y mutation, `hoom verify` (Run sin opciones) los
//     deja no_aplica con el motivo, sin correr su comando (aclaracion 10),
//     summary.no_aplica 2 en el JSON, verde y no parcial; un gate sin
//     aplica_en corre;
//   - CA-811 (CAL-9.3): con app/Models/User.php cambiado, borrado en un
//     commit, sin seguimiento o con nombre no ASCII, el gate con aplica_en:
//     [app/**] corre su diff_cmd;
//   - CA-812: todos los gates aplican con el cambio vacio, fuera de git y con
//     el hoom.yaml en el cambio; cmd "" es absent aunque tenga aplica_en;
//   - CA-813: --cierre, --full y --gate corren sin mirar aplica_en; --cierre
//     usa el diff_cmd y --full no; ParseArgs entiende --cierre (aclaracion
//     3) y UsageText lo nombra (aclaracion 1).
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion.
package verifycmd_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

// ---------------------------------------------------------------- fixtures

// gprAislar deja fuera las variables de una corrida de hoom y la config de
// git de quien corre los tests.
func gprAislar(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER", "HOOM_TASK",
		"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	global := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

func gprGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func gprEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gprCommit(t *testing.T, dir, msg string) {
	t.Helper()
	gprGit(t, dir, "add", "-A")
	gprGit(t, dir, "commit", "-q", "-m", msg)
}

// gprQ es s como una palabra de sh entre comillas simples.
func gprQ(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// gprToque es un comando que deja marca/nombre; con falla sale con 1.
func gprToque(marca, nombre string, falla bool) string {
	c := "touch " + gprQ(filepath.Join(marca, nombre))
	if falla {
		c += "; exit 1"
	}
	return c
}

func gprHay(marca, nombre string) bool {
	_, err := os.Stat(filepath.Join(marca, nombre))
	return err == nil
}

// gprVaciar borra las marcas de una corrida anterior.
func gprVaciar(t *testing.T, marca string) {
	t.Helper()
	es, err := os.ReadDir(marca)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if err := os.Remove(filepath.Join(marca, e.Name())); err != nil {
			t.Fatal(err)
		}
	}
}

// gprGates es la seccion gates de los fixtures: test sin aplica_en, y static
// y mutation requeridos con aplica_en: [app/**], cmd y diff_cmd que dejan
// marca/<gate>-cmd y marca/<gate>-diff (con falla, salen con 1). extra va al
// final, dentro de gates.
func gprGates(marca string, falla bool, extra string) string {
	s := "gates:\n  test:\n    required: true\n    cmd: " + strconv.Quote(gprToque(marca, "test-cmd", false)) + "\n"
	for _, n := range []string{"static", "mutation"} {
		s += "  " + n + ":\n    required: true\n" +
			"    cmd: " + strconv.Quote(gprToque(marca, n+"-cmd", falla)) + "\n" +
			"    diff_cmd: " + strconv.Quote(gprToque(marca, n+"-diff", falla)) + "\n" +
			"    aplica_en: [\"app/**\"]\n"
	}
	return s + extra
}

// gprRepo es un proyecto git con base main: hoom.yaml (base_branch main y
// gatesYAML), el .gitignore canonico de .hoom y archivos, commiteado y
// parado en la rama feature recien creada.
func gprRepo(t *testing.T, gatesYAML string, archivos map[string]string) string {
	t.Helper()
	gprAislar(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gprGit(t, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	gprGit(t, root, "config", "user.email", "test@hoom.dev")
	gprGit(t, root, "config", "user.name", "hoom test")
	gprEscribir(t, root, manifest.FileName, "schema: hoom/v1\nproject: demo\nbase_branch: main\n"+gatesYAML)
	gprEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	gprEscribir(t, root, "resources/js/app.js", "console.log(1)\n")
	for rel, body := range archivos {
		gprEscribir(t, root, rel, body)
	}
	gprCommit(t, root, "inicial")
	gprGit(t, root, "checkout", "-q", "-b", "feature")
	return root
}

func gprVerify(t *testing.T, ca, dir string, opt verifycmd.Options) (*verdict.Verdict, string) {
	t.Helper()
	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatalf("%s: fixture: manifest.Load(%s): %v", ca, dir, err)
	}
	v, path, err := verifycmd.Run(m, opt)
	if err != nil {
		t.Fatalf("%s: verify corre: %v", ca, err)
	}
	return v, path
}

func gprGate(t *testing.T, ca string, v *verdict.Verdict, nombre string) verdict.GateResult {
	t.Helper()
	for _, g := range v.Gates {
		if g.Name == nombre {
			return g
		}
	}
	t.Fatalf("%s: el veredicto no trae el gate %q: %+v", ca, nombre, v.Gates)
	return verdict.GateResult{}
}

// gprJSON lee el veredicto de path como mapas de claves.
func gprJSON(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("el veredicto es JSON: %v\n%s", err, raw)
	}
	return m
}

// gprCorrio exige que static y mutation hayan corrido (no no_aplica).
func gprCorrio(t *testing.T, ca, caso string, v *verdict.Verdict, marca string) {
	t.Helper()
	for _, n := range []string{"static", "mutation"} {
		g := gprGate(t, ca, v, n)
		if g.Status != verdict.StatusPass || !(gprHay(marca, n+"-cmd") || gprHay(marca, n+"-diff")) {
			t.Fatalf("%s (%s): %s aplica y corre: %+v", ca, caso, n, g)
		}
	}
	if v.Summary.NoAplica != 0 || v.Verdict != "green" {
		t.Fatalf("%s (%s): ningun no_aplica y verde: %s %+v", ca, caso, v.Verdict, v.Summary)
	}
}

// ---------------------------------------------------------------- CA-810

// CA-810 (CAL-9.1) y aclaraciones 10 y 11: con solo resources/js/app.js
// cambiado, static y mutation quedan no_aplica con el motivo exacto, sin
// correr su comando (que fallaria y dejaria rastro), y summary.no_aplica 2,
// tambien en el JSON; el veredicto es verde y no parcial, y LatestComplete lo
// da; test, sin aplica_en, corre.
func TestCA810_SoloResourcesJsCambiadoDejaNoAplica(t *testing.T) {
	const ca = "CA-810"
	marca := t.TempDir()
	root := gprRepo(t, gprGates(marca, true, ""), map[string]string{"app/Models/User.php": "<?php\n"})
	gprEscribir(t, root, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, root, "solo el js")

	v, path := gprVerify(t, ca, root, verifycmd.Options{})
	motivo := fmt.Sprintf(gates.MotivoNoAplica, "app/**")
	for _, n := range []string{"static", "mutation"} {
		g := gprGate(t, ca, v, n)
		if g.Status != verdict.StatusNoAplica || g.Scope != "none" || g.Command != "" || g.Notes != motivo {
			t.Fatalf("%s: %s queda no_aplica, scope none, sin Command y con el motivo %q: %+v", ca, n, motivo, g)
		}
		if gprHay(marca, n+"-cmd") || gprHay(marca, n+"-diff") {
			t.Fatalf("%s: el comando de %s no corre", ca, n)
		}
	}
	if g := gprGate(t, ca, v, "test"); g.Status != verdict.StatusPass || !gprHay(marca, "test-cmd") {
		t.Fatalf("%s: el gate sin aplica_en corre: %+v", ca, g)
	}
	if v.Summary.NoAplica != 2 || v.Verdict != "green" || v.Partial || v.IsPartial() {
		t.Fatalf("%s: summary.no_aplica 2, verde y no parcial: %s parcial=%v %+v", ca, v.Verdict, v.IsPartial(), v.Summary)
	}

	claves := gprJSON(t, path)
	var summary map[string]int
	if err := json.Unmarshal(claves["summary"], &summary); err != nil {
		t.Fatalf("%s: summary es un objeto: %v", ca, err)
	}
	if n, ok := summary["no_aplica"]; !ok || n != 2 {
		t.Fatalf("%s: el JSON trae summary.no_aplica: 2: %s", ca, claves["summary"])
	}
	var verde string
	if err := json.Unmarshal(claves["verdict"], &verde); err != nil || verde != "green" {
		t.Fatalf("%s: el JSON dice green: %s", ca, claves["verdict"])
	}
	if _, ok := claves["partial"]; ok {
		t.Fatalf("%s: el JSON no trae partial", ca)
	}
	var gs []map[string]any
	if err := json.Unmarshal(claves["gates"], &gs); err != nil {
		t.Fatal(err)
	}
	for _, g := range gs {
		if g["name"] == "static" || g["name"] == "mutation" {
			if g["status"] != "no_aplica" || g["scope"] != "none" || g["notes"] != motivo {
				t.Fatalf("%s: en el JSON, %v es no_aplica, scope none, con el motivo: %v", ca, g["name"], g)
			}
		}
	}

	all, err := verdict.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	if l := verdict.LatestComplete(all); l == nil || l.ID != v.ID {
		t.Fatalf("%s: un veredicto con no_aplica es completo: LatestComplete lo da: %+v", ca, l)
	}
}

// ---------------------------------------------------------------- CA-811

// CA-811 (CAL-9.3): con app/Models/User.php cambiado, borrado en un commit,
// sin seguimiento o con nombre no ASCII, el gate con aplica_en: [app/**]
// corre su diff_cmd (sin placeholders: aclaracion 2), con scope diff.
func TestCA811_HoomVerifyCorreElDiffCmd(t *testing.T) {
	const ca = "CA-811"
	const user = "app/Models/User.php"
	casos := []struct {
		nombre  string
		inicial map[string]string
		cambio  func(t *testing.T, root string)
	}{
		{"cambiado", map[string]string{user: "<?php // v1\n"}, func(t *testing.T, root string) {
			gprEscribir(t, root, user, "<?php // v2\n")
			gprCommit(t, root, "cambia User")
		}},
		{"borrado en un commit", map[string]string{user: "<?php // v1\n"}, func(t *testing.T, root string) {
			gprGit(t, root, "rm", "-q", user)
			gprGit(t, root, "commit", "-q", "-m", "borra User")
		}},
		{"sin seguimiento", nil, func(t *testing.T, root string) {
			gprEscribir(t, root, user, "<?php // nuevo\n")
		}},
		{"no ASCII", nil, func(t *testing.T, root string) {
			gprEscribir(t, root, "app/Modelos/Año.php", "<?php // año\n")
			gprCommit(t, root, "agrega Año")
		}},
	}
	for _, c := range casos {
		marca := t.TempDir()
		root := gprRepo(t, gprGates(marca, false, ""), c.inicial)
		c.cambio(t, root)
		v, _ := gprVerify(t, ca, root, verifycmd.Options{})
		g := gprGate(t, ca, v, "mutation")
		diff := gprToque(marca, "mutation-diff", false)
		if g.Status != verdict.StatusPass || g.Scope != "diff" || g.Command != diff {
			t.Fatalf("%s (%s): mutation corre su diff_cmd %q con scope diff: %+v", ca, c.nombre, diff, g)
		}
		if !gprHay(marca, "mutation-diff") || gprHay(marca, "mutation-cmd") {
			t.Fatalf("%s (%s): corrio el diff_cmd y no el cmd", ca, c.nombre)
		}
		if v.Summary.NoAplica != 0 || v.Verdict != "green" {
			t.Fatalf("%s (%s): ningun no_aplica y verde: %s %+v", ca, c.nombre, v.Verdict, v.Summary)
		}
	}
}

// ---------------------------------------------------------------- CA-812

// CA-812: todos los gates aplican (corren) con el cambio vacio, fuera de git
// y con el hoom.yaml en el cambio.
func TestCA812_TodosLosGatesAplican(t *testing.T) {
	const ca = "CA-812"

	marca := t.TempDir()
	root := gprRepo(t, gprGates(marca, false, ""), nil)
	v, _ := gprVerify(t, ca, root, verifycmd.Options{})
	gprCorrio(t, ca, "cambio vacio", v, marca)

	marca = t.TempDir()
	gprAislar(t)
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(fuera))
	gprEscribir(t, fuera, manifest.FileName, "schema: hoom/v1\nproject: demo\n"+gprGates(marca, false, ""))
	gprEscribir(t, fuera, "resources/js/app.js", "console.log(1)\n")
	v, _ = gprVerify(t, ca, fuera, verifycmd.Options{})
	gprCorrio(t, ca, "fuera de git", v, marca)

	marca = t.TempDir()
	root = gprRepo(t, gprGates(marca, false, ""), nil)
	raw, err := os.ReadFile(filepath.Join(root, manifest.FileName))
	if err != nil {
		t.Fatal(err)
	}
	gprEscribir(t, root, manifest.FileName, string(raw)+"# un cambio del hoom.yaml\n")
	gprEscribir(t, root, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, root, "toca el hoom.yaml y el js")
	v, _ = gprVerify(t, ca, root, verifycmd.Options{})
	gprCorrio(t, ca, "hoom.yaml en el cambio", v, marca)
}

// CA-812: cmd "" da absent aunque el gate tenga aplica_en (y un diff_cmd que
// no corre), tambien en un verify de trabajo donde no coincide.
func TestCA812_CmdVacioEsAbsentAunqueTengaAplicaEn(t *testing.T) {
	const ca = "CA-812"
	marca := t.TempDir()
	lint := "  lint:\n    required: true\n    cmd: \"\"\n" +
		"    diff_cmd: " + strconv.Quote(gprToque(marca, "lint-diff", false)) + "\n" +
		"    aplica_en: [\"app/**\"]\n"
	root := gprRepo(t, gprGates(marca, false, lint), nil)
	gprEscribir(t, root, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, root, "solo el js")
	v, _ := gprVerify(t, ca, root, verifycmd.Options{})
	if g := gprGate(t, ca, v, "lint"); g.Status != verdict.StatusAbsent {
		t.Fatalf("%s: cmd \"\" con aplica_en es absent, no %q: %+v", ca, g.Status, g)
	}
	if gprHay(marca, "lint-diff") {
		t.Fatalf("%s: el diff_cmd de un gate absent no corre", ca)
	}
	if v.Summary.Absent != 1 || v.Summary.NoAplica != 2 {
		t.Fatalf("%s: lint cuenta como absent y no como no_aplica: %+v", ca, v.Summary)
	}
}

// ---------------------------------------------------------------- CA-813

// CA-813: con solo resources/js/app.js cambiado, --cierre corre static y
// mutation con su diff_cmd (scope diff) y deja un veredicto completo y verde
// sin no_aplica; --full corre su cmd (scope full); --gate mutation corre
// mutation, y el veredicto es parcial.
func TestCA813_CierreFullYGateCorrenSinMirarAplicaEn(t *testing.T) {
	const ca = "CA-813"
	marca := t.TempDir()
	root := gprRepo(t, gprGates(marca, false, ""), nil)
	gprEscribir(t, root, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, root, "solo el js")

	v, _ := gprVerify(t, ca, root, verifycmd.Options{Cierre: true})
	for _, n := range []string{"static", "mutation"} {
		g := gprGate(t, ca, v, n)
		if g.Status != verdict.StatusPass || g.Scope != "diff" || g.Command != gprToque(marca, n+"-diff", false) || !gprHay(marca, n+"-diff") {
			t.Fatalf("%s: --cierre corre %s con su diff_cmd y scope diff: %+v", ca, n, g)
		}
	}
	if v.Summary.NoAplica != 0 || v.Verdict != "green" || v.IsPartial() {
		t.Fatalf("%s: --cierre deja un verde completo sin no_aplica: %s parcial=%v %+v", ca, v.Verdict, v.IsPartial(), v.Summary)
	}

	gprVaciar(t, marca)
	v, _ = gprVerify(t, ca, root, verifycmd.Options{Full: true})
	for _, n := range []string{"static", "mutation"} {
		g := gprGate(t, ca, v, n)
		if g.Status != verdict.StatusPass || g.Scope != "full" || g.Command != gprToque(marca, n+"-cmd", false) || !gprHay(marca, n+"-cmd") {
			t.Fatalf("%s: --full corre %s con su cmd y scope full: %+v", ca, n, g)
		}
		if gprHay(marca, n+"-diff") {
			t.Fatalf("%s: --full no usa el diff_cmd de %s", ca, n)
		}
	}
	if v.Summary.NoAplica != 0 {
		t.Fatalf("%s: --full no deja no_aplica: %+v", ca, v.Summary)
	}

	gprVaciar(t, marca)
	v, _ = gprVerify(t, ca, root, verifycmd.Options{Gates: []string{"mutation"}})
	if g := gprGate(t, ca, v, "mutation"); g.Status != verdict.StatusPass || !(gprHay(marca, "mutation-diff") || gprHay(marca, "mutation-cmd")) {
		t.Fatalf("%s: el gate de --gate corre sin mirar aplica_en: %+v", ca, g)
	}
	if v.Summary.NoAplica != 0 || !v.IsPartial() {
		t.Fatalf("%s: --gate no deja no_aplica y es parcial: parcial=%v %+v", ca, v.IsPartial(), v.Summary)
	}
}

// CA-813 y aclaracion 3: --cierre es un flag booleano que pone
// Options.Cierre; junto a --full o a --gate no es un error de uso.
func TestCA813_ParseArgsEntiendeCierre(t *testing.T) {
	const ca = "CA-813"
	casos := []struct {
		args []string
		want verifycmd.Options
	}{
		{[]string{"--cierre"}, verifycmd.Options{Cierre: true}},
		{nil, verifycmd.Options{}},
		{[]string{"--full"}, verifycmd.Options{Full: true}},
		{[]string{"--cierre", "--full"}, verifycmd.Options{Cierre: true, Full: true}},
		{[]string{"--cierre", "--gate", "test"}, verifycmd.Options{Cierre: true, Gates: []string{"test"}}},
	}
	for _, c := range casos {
		got, err := verifycmd.ParseArgs(c.args)
		if err != nil {
			t.Fatalf("%s: %q no es un error de uso: %v", ca, c.args, err)
		}
		if got.Cierre != c.want.Cierre || got.Full != c.want.Full || !reflect.DeepEqual(append([]string(nil), got.Gates...), append([]string(nil), c.want.Gates...)) {
			t.Fatalf("%s: ParseArgs(%q) = %+v, se esperaba %+v", ca, c.args, got, c.want)
		}
		if got.Sobre != nil {
			t.Fatalf("%s: ParseArgs nunca pone Sobre: %+v", ca, got)
		}
	}
}

// CA-813 y aclaracion 1: el uso de `hoom verify` nombra --cierre en su
// primera linea, despues de --full, y lo explica en dos lineas.
func TestCA813_UsageTextNombraCierre(t *testing.T) {
	const ca = "CA-813"
	if l := strings.SplitN(verifycmd.UsageText, "\n", 2)[0]; l != "Uso: hoom verify [--full] [--cierre] [--gate a,b] [--spec <ruta>] [--json]" {
		t.Fatalf("%s: la linea Uso nombra --cierre despues de --full: %q", ca, l)
	}
	bloque := "  --full          Ignora el scoping por diff (corrida completa, ej. nocturna)\n" +
		"  --cierre        Corre todos los gates sin mirar aplica_en, con su\n" +
		"                  diff_cmd: el verify que exigen 'hoom task done' e integrar\n"
	if !strings.Contains(verifycmd.UsageText, bloque) {
		t.Fatalf("%s: las dos lineas de --cierre van despues de --full:\n%s", ca, verifycmd.UsageText)
	}
}
