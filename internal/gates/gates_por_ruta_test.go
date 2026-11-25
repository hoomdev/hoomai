//go:build !windows

// Tests adversariales del spec .hoom/specs/gates-por-ruta.md sobre el paquete
// gates, como consumidor externo:
//
//   - CA-810 (CAL-9.1): en un verify de trabajo (Options.Cambio, sin Full,
//     Cierre ni Only), un gate con aplica_en sin coincidencias no corre su
//     comando y queda no_aplica, scope none, sin Command y con el motivo
//     exacto; un gate sin aplica_en corre (aclaracion 10: los comandos del
//     gate que no aplica fallarian y dejarian rastro);
//   - CA-811 (CAL-9.3): CambioDe trae la ruta cambiada, borrada en un commit,
//     sin seguimiento o no ASCII, relativa a la raiz y sin comillas, y el
//     gate con aplica_en: [app/**] corre su diff_cmd (aclaraciones 2 y 7);
//   - CA-812: Todos con el cambio vacio, fuera de git y con el hoom.yaml del
//     proyecto en el cambio (aclaraciones 6 y 9); con Todos o sin archivos
//     todo gate aplica; cmd "" es absent aunque tenga aplica_en;
//   - CA-813: con Cierre, Full u Only nadie mira aplica_en; Cierre usa el
//     diff_cmd y Full el cmd.
//
// Repos temporales, sin red. Escritos desde el spec, las aclaraciones de la
// ronda 0 y el go doc, sin leer la implementacion.
package gates_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- fixtures

func gprPtr(s string) *string { return &s }

// gprQ es s como una palabra de sh entre comillas simples.
func gprQ(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// gprToque es un comando que deja el archivo marca/nombre y sale con exit.
func gprToque(marca, nombre string, exit int) string {
	c := "touch " + gprQ(filepath.Join(marca, nombre))
	if exit != 0 {
		c += fmt.Sprintf("; exit %d", exit)
	}
	return c
}

func gprHay(marca, nombre string) bool {
	_, err := os.Stat(filepath.Join(marca, nombre))
	return err == nil
}

// gprGate es un gate requerido con cmd, diff_cmd ("" = sin diff_cmd) y
// aplica_en (sin patrones = sin la clave).
func gprGate(cmd, diff string, aplica ...string) manifest.Gate {
	req := true
	g := manifest.Gate{Required: &req, Cmd: gprPtr(cmd)}
	if diff != "" {
		g.DiffCmd = gprPtr(diff)
	}
	if len(aplica) > 0 {
		g.AplicaEn = aplica
	}
	return g
}

// gprManifest arma, en dir, static y mutation con aplica_en: [app/**] y
// comandos que dejan marca/<gate>-cmd y marca/<gate>-diff, y test sin
// aplica_en que deja marca/test-cmd. Con falla, los de static y mutation
// salen con 1.
func gprManifest(dir, marca string, falla bool) *manifest.Manifest {
	exit := 0
	if falla {
		exit = 1
	}
	return &manifest.Manifest{Dir: dir, BaseBranch: "main", Gates: map[string]manifest.Gate{
		"static":   gprGate(gprToque(marca, "static-cmd", exit), gprToque(marca, "static-diff", exit), "app/**"),
		"mutation": gprGate(gprToque(marca, "mutation-cmd", exit), gprToque(marca, "mutation-diff", exit), "app/**"),
		"test":     gprGate(gprToque(marca, "test-cmd", 0), ""),
	}}
}

func gprRun(m *manifest.Manifest, git gitx.Info, opt gates.Options) []verdict.GateResult {
	if opt.Timeout == 0 {
		opt.Timeout = time.Minute
	}
	if opt.TailSize == 0 {
		opt.TailSize = 4096
	}
	return gates.Run(m, git, opt)
}

func gprRes(t *testing.T, ca string, res []verdict.GateResult, nombre string) verdict.GateResult {
	t.Helper()
	for _, r := range res {
		if r.Name == nombre {
			return r
		}
	}
	t.Fatalf("%s: Run no devolvio el gate %q: %+v", ca, nombre, res)
	return verdict.GateResult{}
}

// gprGitInfo es el git de un repo cuyo cambio es files.
func gprGitInfo(files ...string) gitx.Info {
	return gitx.Info{IsRepo: true, Base: "main", Branch: "feature", ChangedFiles: files}
}

// gprAislar deja git sin la config de quien corre los tests.
func gprAislar(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_COMMON_DIR", "GIT_CEILING_DIRECTORIES"} {
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

// gprRepo es un repo git con base main y archivos (ruta -> contenido)
// commiteados alli, parado en la rama feature recien creada. proyecto es
// donde va el hoom.yaml, relativo a la raiz ("" = la raiz).
func gprRepo(t *testing.T, proyecto string, archivos map[string]string) string {
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
	gprEscribir(t, root, filepath.ToSlash(filepath.Join(proyecto, manifest.FileName)),
		"schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	gprEscribir(t, root, "README.md", "# demo\n")
	for rel, body := range archivos {
		gprEscribir(t, root, rel, body)
	}
	gprCommit(t, root, "inicial")
	gprGit(t, root, "checkout", "-q", "-b", "feature")
	return root
}

func gprContiene(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- CA-810

// CA-810: el motivo es el texto del spec, con los patrones como estan
// escritos unidos por ", " (aclaracion 8), y el estado es no_aplica.
func TestCA810_ElMotivoYElEstadoExactos(t *testing.T) {
	const ca = "CA-810"
	if got := fmt.Sprintf(gates.MotivoNoAplica, "app/**, src/**"); got != "ningun archivo del cambio coincide con aplica_en (app/**, src/**)" {
		t.Fatalf("%s: el motivo del spec: %q", ca, got)
	}
	if verdict.StatusNoAplica != "no_aplica" {
		t.Fatalf("%s: el estado es no_aplica: %q", ca, verdict.StatusNoAplica)
	}
}

// CA-810: Aplica dice que no, con el motivo exacto, cuando ningun archivo del
// cambio coincide; que si cuando alguno coincide o el gate no tiene
// aplica_en.
func TestCA810_Aplica(t *testing.T) {
	const ca = "CA-810"
	soloJS := gates.Cambio{Archivos: []string{"resources/js/app.js"}}
	g := gprGate("true", "", "app/**")
	if ok, motivo := gates.Aplica(g, soloJS); ok || motivo != fmt.Sprintf(gates.MotivoNoAplica, "app/**") {
		t.Fatalf("%s: con solo resources/js/app.js, aplica_en: [app/**] no aplica, con el motivo: %v %q", ca, ok, motivo)
	}
	dos := gprGate("true", "", "app/**", "src/**")
	if ok, motivo := gates.Aplica(dos, soloJS); ok || motivo != "ningun archivo del cambio coincide con aplica_en (app/**, src/**)" {
		t.Fatalf("%s: el motivo lleva los patrones unidos por \", \": %v %q", ca, ok, motivo)
	}
	conApp := gates.Cambio{Archivos: []string{"app/Models/User.php", "resources/js/app.js"}}
	if ok, motivo := gates.Aplica(g, conApp); !ok || motivo != "" {
		t.Fatalf("%s: con un archivo que coincide, aplica y sin motivo: %v %q", ca, ok, motivo)
	}
	if ok, motivo := gates.Aplica(gprGate("true", ""), soloJS); !ok || motivo != "" {
		t.Fatalf("%s: un gate sin aplica_en aplica siempre: %v %q", ca, ok, motivo)
	}
}

// CA-810 (CAL-9.1) y aclaracion 10: con solo resources/js/app.js cambiado y
// aplica_en: [app/**] en static y mutation, ninguno de los dos corre (sus
// comandos fallarian y dejarian rastro): no_aplica, scope none, sin
// Command, exit 0, duracion 0, sin salida, el motivo exacto y su Required;
// el gate sin aplica_en corre. El veredicto queda verde, no parcial, con
// summary.no_aplica 2.
func TestCA810_LosGatesQueNoAplicanNoCorren(t *testing.T) {
	const ca = "CA-810"
	marca := t.TempDir()
	m := gprManifest(t.TempDir(), marca, true)
	res := gprRun(m, gprGitInfo("resources/js/app.js"), gates.Options{Cambio: &gates.Cambio{Archivos: []string{"resources/js/app.js"}}})
	motivo := fmt.Sprintf(gates.MotivoNoAplica, "app/**")
	for _, n := range []string{"static", "mutation"} {
		r := gprRes(t, ca, res, n)
		if r.Status != verdict.StatusNoAplica || r.Scope != "none" || r.Command != "" || r.Notes != motivo {
			t.Fatalf("%s: %s queda no_aplica, scope none, sin Command y con el motivo %q: %+v", ca, n, motivo, r)
		}
		if r.ExitCode != 0 || r.DurationMS != 0 || r.OutputTail != "" || !r.Required {
			t.Fatalf("%s: %s no corrio: exit 0, duracion 0, sin salida y su Required: %+v", ca, n, r)
		}
		for _, archivo := range []string{n + "-cmd", n + "-diff"} {
			if gprHay(marca, archivo) {
				t.Fatalf("%s: el comando de %s corrio (dejo %s)", ca, n, archivo)
			}
		}
	}
	if r := gprRes(t, ca, res, "test"); r.Status != verdict.StatusPass || !gprHay(marca, "test-cmd") {
		t.Fatalf("%s: un gate sin aplica_en corre: %+v", ca, r)
	}
	v := &verdict.Verdict{Gates: res}
	v.Finalize()
	if v.Verdict != "green" || v.IsPartial() || v.Summary.NoAplica != 2 {
		t.Fatalf("%s: verde, no parcial y summary.no_aplica 2: %s parcial=%v %+v", ca, v.Verdict, v.IsPartial(), v.Summary)
	}
}

// ---------------------------------------------------------------- CA-811

// CA-811 (CAL-9.3) y aclaraciones 2 y 7: en cada caso CambioDe trae la ruta,
// relativa a la raiz, sin comillas y tal cual (no ASCII incluida), con
// Todos false; y con ese cambio el gate con aplica_en: [app/**] corre su
// diff_cmd (sin placeholders, deja rastro): pass, scope diff, Command el
// diff_cmd, y no su cmd.
func TestCA811_ElGateCorreSuDiffCmd(t *testing.T) {
	const ca = "CA-811"
	const user = "app/Models/User.php"
	const ano = "app/Modelos/Año.php"
	casos := []struct {
		nombre  string
		inicial map[string]string
		cambio  func(t *testing.T, root string)
		desde   string // subdirectorio desde el que se pide el cambio
		ruta    string
	}{
		{"cambiado en un commit", map[string]string{user: "<?php // v1\n"}, func(t *testing.T, root string) {
			gprEscribir(t, root, user, "<?php // v2\n")
			gprCommit(t, root, "cambia User")
		}, "", user},
		{"cambiado sin commitear", map[string]string{user: "<?php // v1\n"}, func(t *testing.T, root string) {
			gprEscribir(t, root, user, "<?php // v2\n")
		}, "", user},
		{"borrado en un commit", map[string]string{user: "<?php // v1\n"}, func(t *testing.T, root string) {
			gprGit(t, root, "rm", "-q", user)
			gprGit(t, root, "commit", "-q", "-m", "borra User")
		}, "", user},
		{"sin seguimiento", nil, func(t *testing.T, root string) {
			gprEscribir(t, root, user, "<?php // nuevo\n")
		}, "", user},
		{"sin seguimiento desde un subdirectorio", map[string]string{"resources/js/app.js": "x\n"}, func(t *testing.T, root string) {
			gprEscribir(t, root, user, "<?php // nuevo\n")
		}, "resources", user},
		{"no ASCII en un commit", nil, func(t *testing.T, root string) {
			gprEscribir(t, root, ano, "<?php // año\n")
			gprCommit(t, root, "agrega Año")
		}, "", ano},
		{"no ASCII sin seguimiento", nil, func(t *testing.T, root string) {
			gprEscribir(t, root, ano, "<?php // año\n")
		}, "", ano},
	}
	for _, c := range casos {
		root := gprRepo(t, "", c.inicial)
		c.cambio(t, root)
		cambio := gates.CambioDe(filepath.Join(root, filepath.FromSlash(c.desde)), "main")
		if cambio.Todos || !gprContiene(cambio.Archivos, c.ruta) {
			t.Fatalf("%s (%s): CambioDe trae %q tal cual y Todos false: %+v", ca, c.nombre, c.ruta, cambio)
		}
		for _, a := range cambio.Archivos {
			if strings.HasPrefix(a, `"`) || strings.HasPrefix(a, "/") || strings.Contains(a, `\`) {
				t.Fatalf("%s (%s): las rutas van relativas a la raiz, con / y sin comillas: %q", ca, c.nombre, cambio.Archivos)
			}
		}

		marca := t.TempDir()
		m := gprManifest(root, marca, false)
		res := gprRun(m, gitx.Snapshot(root, "main"), gates.Options{Cambio: &cambio})
		r := gprRes(t, ca, res, "mutation")
		diff := gprToque(marca, "mutation-diff", 0)
		if r.Status != verdict.StatusPass || r.Scope != "diff" || r.Command != diff {
			t.Fatalf("%s (%s): mutation corre su diff_cmd %q con scope diff: %+v", ca, c.nombre, diff, r)
		}
		if !gprHay(marca, "mutation-diff") || gprHay(marca, "mutation-cmd") {
			t.Fatalf("%s (%s): corrio el diff_cmd y no el cmd", ca, c.nombre)
		}
	}
}

// ---------------------------------------------------------------- CA-812

// CA-812: con el cambio vacio, Todos es true.
func TestCA812_TodosConElCambioVacio(t *testing.T) {
	const ca = "CA-812"
	root := gprRepo(t, "", map[string]string{"app/Models/User.php": "<?php\n"})
	if c := gates.CambioDe(root, "main"); !c.Todos || len(c.Archivos) != 0 {
		t.Fatalf("%s: sin cambios, Todos y sin archivos: %+v", ca, c)
	}
}

// CA-812 y aclaracion 9: fuera de git, Todos es true y no hay archivos.
func TestCA812_TodosFueraDeGit(t *testing.T) {
	const ca = "CA-812"
	gprAislar(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	gprEscribir(t, dir, manifest.FileName, "schema: hoom/v1\nproject: demo\n")
	gprEscribir(t, dir, "app/Models/User.php", "<?php\n")
	if c := gates.CambioDe(dir, "main"); !c.Todos || len(c.Archivos) != 0 {
		t.Fatalf("%s: fuera de git, Todos y sin archivos: %+v", ca, c)
	}
}

// CA-812 y aclaracion 5: si git no puede decir cual es el cambio (sin
// merge-base con base_branch: una base que no existe, o una rama huerfana),
// Todos es true aunque haya un archivo cambiado.
func TestCA812_TodosSinMergeBase(t *testing.T) {
	const ca = "CA-812"
	root := gprRepo(t, "", map[string]string{"app/Models/User.php": "<?php\n"})
	gprEscribir(t, root, "app/Models/User.php", "<?php // cambiado\n")
	gprCommit(t, root, "cambia el modelo")
	if c := gates.CambioDe(root, "no-existe"); !c.Todos {
		t.Fatalf("%s: con una base_branch que no existe, Todos: %+v", ca, c)
	}

	root = gprRepo(t, "", map[string]string{"app/Models/User.php": "<?php\n"})
	gprGit(t, root, "checkout", "-q", "--orphan", "huerfana")
	gprEscribir(t, root, "app/Models/User.php", "<?php // huerfano\n")
	gprCommit(t, root, "rama huerfana")
	if c := gates.CambioDe(root, "main"); !c.Todos {
		t.Fatalf("%s: sin merge-base con main (rama huerfana), Todos: %+v", ca, c)
	}
}

// CA-812 y aclaracion 6: con el hoom.yaml del proyecto en el cambio, Todos es
// true: en un commit, sin commitear, con el proyecto en un subdirectorio del
// repo y pidiendo el cambio desde un subdirectorio del proyecto.
func TestCA812_TodosConElHoomYamlEnElCambio(t *testing.T) {
	const ca = "CA-812"
	casos := []struct {
		nombre, proyecto, desde string
		commit                  bool
	}{
		{"en un commit", "", "", true},
		{"sin commitear", "", "", false},
		{"proyecto en un subdirectorio", "web", "web", true},
		{"desde un subdirectorio del proyecto", "web", "web/resources", false},
	}
	for _, c := range casos {
		js := filepath.ToSlash(filepath.Join(c.proyecto, "resources/js/app.js"))
		root := gprRepo(t, c.proyecto, map[string]string{js: "x\n"})
		yaml := filepath.Join(root, filepath.FromSlash(c.proyecto), manifest.FileName)
		raw, err := os.ReadFile(yaml)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(yaml, append(raw, []byte("# un cambio del hoom.yaml\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		gprEscribir(t, root, js, "y\n")
		if c.commit {
			gprCommit(t, root, "toca el hoom.yaml")
		}
		if got := gates.CambioDe(filepath.Join(root, filepath.FromSlash(c.desde)), "main"); !got.Todos {
			t.Fatalf("%s (%s): con el hoom.yaml del proyecto en el cambio, Todos: %+v", ca, c.nombre, got)
		}
	}
}

// CA-812: con Todos, o con un cambio sin archivos, todo gate aplica y corre
// aunque su aplica_en no coincida.
func TestCA812_ConTodosOSinArchivosTodoGateAplica(t *testing.T) {
	const ca = "CA-812"
	g := gprGate("true", "", "app/**")
	for _, c := range []gates.Cambio{
		{Todos: true},
		{Todos: true, Archivos: []string{"resources/js/app.js"}},
		{},
	} {
		if ok, motivo := gates.Aplica(g, c); !ok || motivo != "" {
			t.Fatalf("%s: con %+v el gate aplica, sin motivo: %v %q", ca, c, ok, motivo)
		}
		cambio := c
		marca := t.TempDir()
		res := gprRun(gprManifest(t.TempDir(), marca, false), gprGitInfo(c.Archivos...), gates.Options{Cambio: &cambio})
		for _, n := range []string{"static", "mutation"} {
			if r := gprRes(t, ca, res, n); r.Status != verdict.StatusPass || !(gprHay(marca, n+"-cmd") || gprHay(marca, n+"-diff")) {
				t.Fatalf("%s: con %+v, %s corre: %+v", ca, c, n, r)
			}
		}
	}
}

// CA-812: cmd "" da absent aunque el gate tenga aplica_en, coincida o no con
// el cambio, y aunque tenga un diff_cmd (que no corre).
func TestCA812_CmdVacioEsAbsentAunqueTengaAplicaEn(t *testing.T) {
	const ca = "CA-812"
	for _, archivos := range [][]string{{"resources/js/app.js"}, {"app/Models/User.php"}} {
		marca := t.TempDir()
		m := &manifest.Manifest{Dir: t.TempDir(), Gates: map[string]manifest.Gate{
			"mutation": gprGate("", gprToque(marca, "mutation-diff", 0), "app/**"),
			"test":     gprGate("true", ""),
		}}
		res := gprRun(m, gprGitInfo(archivos...), gates.Options{Cambio: &gates.Cambio{Archivos: archivos}})
		if r := gprRes(t, ca, res, "mutation"); r.Status != verdict.StatusAbsent {
			t.Fatalf("%s: con %q, cmd \"\" y aplica_en es absent, no %q: %+v", ca, archivos, r.Status, r)
		}
		if gprHay(marca, "mutation-diff") {
			t.Fatalf("%s: con %q, el diff_cmd de un gate absent no corre", ca, archivos)
		}
	}
}

// ---------------------------------------------------------------- CA-813

// CA-813: con Cierre, Full u Only nadie mira aplica_en: con solo
// resources/js/app.js en el cambio, static y mutation corren igual. Cierre
// usa el diff_cmd (scope diff) y Full el cmd (scope full); el gate de Only
// corre y los demas quedan skipped.
func TestCA813_CierreFullYGateCorrenSinMirarAplicaEn(t *testing.T) {
	const ca = "CA-813"
	cambio := gates.Cambio{Archivos: []string{"resources/js/app.js"}}
	git := gprGitInfo("resources/js/app.js")

	marca := t.TempDir()
	res := gprRun(gprManifest(t.TempDir(), marca, false), git, gates.Options{Cambio: &cambio, Cierre: true})
	for _, n := range []string{"static", "mutation"} {
		r := gprRes(t, ca, res, n)
		if r.Status != verdict.StatusPass || r.Scope != "diff" || r.Command != gprToque(marca, n+"-diff", 0) || !gprHay(marca, n+"-diff") {
			t.Fatalf("%s: con Cierre, %s corre su diff_cmd con scope diff: %+v", ca, n, r)
		}
	}

	marca = t.TempDir()
	res = gprRun(gprManifest(t.TempDir(), marca, false), git, gates.Options{Cambio: &cambio, Full: true})
	for _, n := range []string{"static", "mutation"} {
		r := gprRes(t, ca, res, n)
		if r.Status != verdict.StatusPass || r.Scope != "full" || r.Command != gprToque(marca, n+"-cmd", 0) || !gprHay(marca, n+"-cmd") {
			t.Fatalf("%s: con Full, %s corre su cmd con scope full: %+v", ca, n, r)
		}
		if gprHay(marca, n+"-diff") {
			t.Fatalf("%s: con Full, %s no corre su diff_cmd", ca, n)
		}
	}

	marca = t.TempDir()
	res = gprRun(gprManifest(t.TempDir(), marca, false), git, gates.Options{Cambio: &cambio, Only: map[string]bool{"mutation": true}})
	if r := gprRes(t, ca, res, "mutation"); r.Status != verdict.StatusPass || !(gprHay(marca, "mutation-diff") || gprHay(marca, "mutation-cmd")) {
		t.Fatalf("%s: el gate de --gate corre sin mirar aplica_en: %+v", ca, r)
	}
	for _, n := range []string{"static", "test"} {
		if r := gprRes(t, ca, res, n); r.Status != verdict.StatusSkipped {
			t.Fatalf("%s: con --gate mutation, %s queda skipped: %+v", ca, n, r)
		}
	}
}
