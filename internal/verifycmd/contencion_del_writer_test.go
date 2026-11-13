// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre
// verifycmd, en cualquier sistema: la opcion Sobre fotografia el material de
// git alrededor de los gates y, si cambio, el veredicto lleva el gate
// material_de_git en fail y queda rojo (CA-687, CA-686); sin Sobre, o sin
// cambio, no hay tal gate; ParseArgs nunca pone Sobre (CA-683).
package verifycmd

import (
	"errors"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const cdwTarea = "demo"

func cdwGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func cdwEscribir(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// cdwHome deja un $HOME del test y la config global de git en un archivo
// vacio del test.
func cdwHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	global := filepath.Join(home, ".gitconfig-del-test")
	cdwEscribir(t, global, "")
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	return home
}

// cdwProyecto es un proyecto con los gates dados (cmd entre comillas dobles
// en el YAML) y extra en su hoom.yaml, y el worktree de la tarea demo como
// lo crea `hoom task start` (rama hoom/demo en .hoom/worktrees/demo). El
// manifiesto es el del worktree.
func cdwProyecto(t *testing.T, gate, extra string) (root, wt string, m *manifest.Manifest) {
	t.Helper()
	cdwHome(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cdwGit(t, root, "init", "-q", "-b", "main")
	cdwGit(t, root, "config", "user.email", "test@hoom.dev")
	cdwGit(t, root, "config", "user.name", "hoom test")
	cdwEscribir(t, filepath.Join(root, manifest.FileName), "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \""+gate+"\"\n"+extra)
	cdwEscribir(t, filepath.Join(root, ".hoom", ".gitignore"), "cache/\nworktrees/\nruns/\n")
	cdwEscribir(t, filepath.Join(root, "app.go"), "package app\n")
	cdwGit(t, root, "add", "-A")
	cdwGit(t, root, "commit", "-q", "-m", "inicial")
	wt = filepath.Join(root, ".hoom", "worktrees", cdwTarea)
	cdwGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+cdwTarea, wt, "main")
	m, err = manifest.Load(wt, nil)
	if err != nil {
		t.Fatal(err)
	}
	return root, wt, m
}

func cdwSobre(root string) *Sobre { return &Sobre{Root: root, Task: cdwTarea, Rol: "writer"} }

func cdwGateMaterial(v *verdict.Verdict) *verdict.GateResult {
	for i := range v.Gates {
		if v.Gates[i].Name == GateMaterialDeGit {
			return &v.Gates[i]
		}
	}
	return nil
}

func cdwComun(t *testing.T, wt string) string {
	t.Helper()
	p := cdwGit(t, wt, "rev-parse", "--git-common-dir")
	if !filepath.IsAbs(p) {
		p = filepath.Join(wt, p)
	}
	return p
}

// cdwUltimoCompletoVerde llama a verdict.LatestComplete sin fijar su firma
// (el go doc no la da; anotado en el informe) y dice si da un verde.
func cdwUltimoCompletoVerde(t *testing.T, dir string) bool {
	t.Helper()
	all, err := verdict.LoadAll(dir)
	if err != nil {
		t.Fatal(err)
	}
	f := reflect.ValueOf(verdict.LatestComplete)
	ft := f.Type()
	if ft.NumIn() != 1 {
		t.Fatalf("verdict.LatestComplete: firma inesperada %s (anotado en el informe)", ft)
	}
	var arg reflect.Value
	switch in := ft.In(0); {
	case reflect.TypeOf(all).AssignableTo(in):
		arg = reflect.ValueOf(all)
	case in.Kind() == reflect.String:
		arg = reflect.ValueOf(dir).Convert(in)
	case in.Kind() == reflect.Slice:
		src := reflect.ValueOf(all)
		vals := reflect.MakeSlice(in, 0, src.Len())
		for i := 0; i < src.Len(); i++ {
			e := src.Index(i)
			switch {
			case e.Type().AssignableTo(in.Elem()):
			case e.Kind() == reflect.Ptr && e.Elem().Type().AssignableTo(in.Elem()):
				e = e.Elem()
			case e.CanAddr() && e.Addr().Type().AssignableTo(in.Elem()):
				e = e.Addr()
			default:
				t.Fatalf("verdict.LatestComplete: firma inesperada %s", ft)
			}
			vals = reflect.Append(vals, e)
		}
		arg = vals
	default:
		t.Fatalf("verdict.LatestComplete: firma inesperada %s (anotado en el informe)", ft)
	}
	out := f.Call([]reflect.Value{arg})
	if len(out) == 0 {
		return false
	}
	for _, o := range out[1:] {
		if o.Kind() == reflect.Bool && !o.Bool() {
			return false
		}
		if o.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) && !o.IsNil() {
			return false
		}
	}
	o := out[0]
	if o.Kind() == reflect.Ptr {
		if o.IsNil() {
			return false
		}
		o = o.Elem()
	}
	return o.Kind() == reflect.Struct && o.FieldByName("Verdict").String() == "green"
}

// ---------------------------------------------------------------- CA-687

// CA-687 y CA-686: con Sobre, un gate que cambia (crea) una pieza del
// material deja el gate material_de_git requerido, en fail, de scope full,
// con las rutas en su salida, y el veredicto rojo; LatestComplete no lo da
// como verde. Sin la seccion encierro (los gates corren como hoy).
func TestCA687_ConSobreUnGateQueCambiaElMaterialDejaRojo(t *testing.T) {
	const ca = "CA-687"
	for _, c := range []struct {
		nombre, gate string
		ruta         func(home, wt, comun string) string
		antes        func(t *testing.T, home string)
	}{
		{nombre: "info/attributes", gate: "c=$(git rev-parse --git-common-dir); mkdir -p $c/info; printf x > $c/info/attributes; true",
			ruta: func(h, w, c string) string { return filepath.Join(c, "info", "attributes") }},
		{nombre: "un hook nuevo", gate: "h=$(git rev-parse --git-path hooks); mkdir -p $h; printf 'exit 0' > $h/pre-commit; chmod +x $h/pre-commit; true",
			ruta: func(h, w, c string) string { return "" }},
		{nombre: "el config comun", gate: "git config --local alias.cdw status; true",
			ruta: func(h, w, c string) string { return filepath.Join(c, "config") }},
		{nombre: "~/.gitconfig", gate: "printf '[alias]\\n\\tcdw2 = log\\n' >> $HOME/.gitconfig; true",
			antes: func(t *testing.T, home string) {
				t.Setenv("GIT_CONFIG_GLOBAL", "")
				os.Unsetenv("GIT_CONFIG_GLOBAL")
				cdwEscribir(t, filepath.Join(home, ".gitconfig"), "[alias]\n\tst = status\n")
			},
			ruta: func(h, w, c string) string { return filepath.Join(h, ".gitconfig") }},
	} {
		gate := c.gate
		if strings.Contains(gate, `\`) {
			gate = strings.ReplaceAll(gate, `\`, `\\`) // YAML entre comillas dobles
		}
		root, wt, m := cdwProyecto(t, gate, "")
		home := os.Getenv("HOME")
		if c.antes != nil {
			c.antes(t, home)
		}
		v, _, err := Run(m, Options{Full: true, Sobre: cdwSobre(root)})
		if err != nil {
			t.Fatalf("%s: %s: verify con Sobre corre: %v", ca, c.nombre, err)
		}
		g := cdwGateMaterial(v)
		if g == nil || !g.Required || g.Status != verdict.StatusFail || g.Scope != "full" || strings.TrimSpace(g.OutputTail) == "" {
			t.Fatalf("%s: %s: el veredicto lleva material_de_git requerido, en fail, scope full, con las rutas: %+v (gates %+v)", ca, c.nombre, g, v.Gates)
		}
		if r := c.ruta(home, wt, cdwComun(t, wt)); r != "" {
			lineas := strings.Split(strings.TrimSpace(g.OutputTail), "\n")
			hay := false
			for _, l := range lineas {
				if strings.TrimSpace(l) == r {
					hay = true
				}
			}
			if !hay {
				t.Fatalf("%s: %s: la salida de material_de_git trae %s, una ruta por linea: %q", ca, c.nombre, r, g.OutputTail)
			}
		}
		if v.Verdict != "red" {
			t.Fatalf("%s: %s: el veredicto queda rojo: %s", ca, c.nombre, v.Verdict)
		}
		if cdwUltimoCompletoVerde(t, wt) {
			t.Fatalf("%s: %s: verdict.LatestComplete no lo da como verde", ca, c.nombre)
		}
	}
}

// CA-687: sin Sobre (hoom verify de la persona) el mismo gate no agrega
// material_de_git; con Sobre y sin cambio, tampoco.
func TestCA687_SinSobreOSinCambioNoHayGateDelMaterial(t *testing.T) {
	const ca = "CA-687"
	gate := "c=$(git rev-parse --git-common-dir); mkdir -p $c/info; printf x > $c/info/attributes; true"
	_, _, m := cdwProyecto(t, gate, "")
	v, _, err := Run(m, Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if g := cdwGateMaterial(v); g != nil || v.Verdict != "green" {
		t.Fatalf("%s: sin Sobre no hay material_de_git y el veredicto es el de hoy: %+v %s", ca, g, v.Verdict)
	}
	root, _, m := cdwProyecto(t, "git status --porcelain >/dev/null; true", "encierro:\n  exigir: no\n  gates_sin_encierro:\n    - test\n")
	v, _, err = Run(m, Options{Full: true, Sobre: cdwSobre(root)})
	if err != nil {
		t.Fatal(err)
	}
	if g := cdwGateMaterial(v); g != nil || v.Verdict != "green" {
		t.Fatalf("%s: con Sobre y sin cambio no hay material_de_git: %+v %s", ca, g, v.Verdict)
	}
	if GateMaterialDeGit != "material_de_git" {
		t.Fatalf("%s: el gate se llama material_de_git: %q", ca, GateMaterialDeGit)
	}
}

// ---------------------------------------------------------------- CA-683

// CA-683: ParseArgs nunca pone Sobre, con ninguna combinacion de los flags
// del verbo; y no hay flag que lo pida.
func TestCA683_ParseArgsNuncaPoneSobre(t *testing.T) {
	const ca = "CA-683"
	flags := [][]string{{"--full"}, {"--gate", "test"}, {"--gate", "a,b"}, {"--spec", ".hoom/specs/x.md"}, {"--json"}}
	f := func(sel uint8, orden int64) bool {
		var args []string
		r := rand.New(rand.NewSource(orden))
		for _, i := range r.Perm(len(flags)) {
			if sel&(1<<uint(i)) != 0 {
				args = append(args, flags[i]...)
			}
		}
		opt, err := ParseArgs(args)
		if err != nil {
			return true // un rechazo tampoco pone Sobre
		}
		if opt.Sobre != nil {
			t.Logf("%q -> %+v", args, opt.Sobre)
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
	for _, args := range [][]string{{"--sobre"}, {"--sobre", "writer"}, {"--sobre=writer"}, {"--encierro"}} {
		opt, err := ParseArgs(args)
		if err == nil || opt.Sobre != nil {
			t.Fatalf("%s: %q no es un flag del verbo: %+v %v", ca, args, opt, err)
		}
		_ = errors.Unwrap(err)
	}
}
