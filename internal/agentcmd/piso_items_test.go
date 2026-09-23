// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-292): el piso del sobre suma .hoom/items/ y .hoom/reviews/. Crear,
// editar o borrar cualquier archivo ahi es 'manipulacion' para cualquier
// rol, aunque hoom.yaml le abra la ruta, y el sobre corta antes de verify:
// un rol no mueve su tarjeta, no afloja su presupuesto ni firma su review.
package agentcmd

import (
	"io"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// CA-292: el gate de scope, sobre fotos: crear, editar y borrar bajo
// .hoom/items/ y .hoom/reviews/ es manipulacion aun con allow "**".
func TestCA292_ItemsYReviewsSonManipulacion(t *testing.T) {
	piRutas := []string{".hoom/items/precios.yaml", ".hoom/reviews/20260922T150405_ab12cd.json"}
	for _, p := range piRutas {
		casos := map[string][2]Snapshot{
			"crear":  {snap(nil), snap(map[string]string{p: "h1"})},
			"editar": {snap(map[string]string{p: "h0"}), snap(map[string]string{p: "h1"})},
			"borrar": {snap(map[string]string{p: "h0"}), snap(map[string]string{p: "-"})},
		}
		for accion, fotos := range casos {
			res := CheckScope(fotos[0], fotos[1], todo())
			v, ok := rutas(res.Violations)[p]
			if !ok || v.Rule != RuleTampering {
				t.Fatalf("CA-292: %s %s es manipulacion aun con allow \"**\": %+v", accion, p, res.Violations)
			}
			if v.Detail == "" {
				t.Fatalf("CA-292: la violacion explica por que (%s %s): %+v", accion, p, v)
			}
			if !res.Tampering || res.OK || !res.Cuts() {
				t.Fatalf("CA-292: %s %s corta el sobre: %+v", accion, p, res)
			}
		}
	}
	// el territorio que un hoom.yaml le abre a un rol no llega al piso
	m := &manifest.Manifest{Agents: map[string]manifest.AgentPolicy{}}
	for _, r := range agents.Roles() {
		m.Agents[r.Slug] = manifest.AgentPolicy{Write: manifest.WriteScope{Allow: []string{".hoom/items/**", ".hoom/reviews/**", "**"}}}
	}
	for _, r := range agents.Roles() {
		pol := PolicyFor(m, r)
		for _, p := range piRutas {
			res := CheckScope(snap(nil), snap(map[string]string{p: "h1"}), pol)
			if v := rutas(res.Violations)[p]; v.Rule != RuleTampering {
				t.Fatalf("CA-292: para %s, %s es manipulacion aunque hoom.yaml lo permita: %+v", r.Slug, p, res.Violations)
			}
		}
	}
	// control: lo vecino no es manipulacion (el piso no se come .hoom/ entero)
	res := CheckScope(snap(nil), snap(map[string]string{".hoom/specs/precios.md": "h1", ".hoom/itemsx/a.yaml": "h1"}), todo())
	for _, v := range res.Violations {
		if v.Rule == RuleTampering {
			t.Fatalf("CA-292: fuera de .hoom/items/ y .hoom/reviews/ no hay manipulacion nueva: %+v", res.Violations)
		}
	}
}

// piRepo: un proyecto donde hoom.yaml le abre TODO a cada rol.
func piRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n"+
		"agents:\n  writer:\n    write:\n      allow: [\"**\", \".hoom/items/**\", \".hoom/reviews/**\"]\n"+
		"  scout:\n    write:\n      allow: [\".hoom/**\"]\n")
	write(t, root, "app.go", "package app\n")
	write(t, root, ".hoom/items/precios.yaml", "titulo: Precios\ntipo: feature\nprioridad: media\n"+
		"pedido: el original\ncreado_por: hoom test\ncreado_en: 2026-09-22T15:04:05Z\npresupuesto_usd: 5\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	return root
}

// CA-292: bajo el sobre real (provider falso), un rol que crea o edita un
// item, o escribe un registro de review, corta en scope: no hay veredicto.
func TestCA292_ElSobreCortaAntesDeVerify(t *testing.T) {
	casos := []struct {
		nombre, rol, script string
	}{
		{"writer crea un item", "writer",
			"mkdir -p .hoom/items && printf 'titulo: x\\n' > .hoom/items/nuevo.yaml\nprintf 'package app // ok\\n' > app.go\nexit 0\n"},
		{"writer afloja su presupuesto", "writer",
			"sed -i.bak 's/presupuesto_usd: 5/presupuesto_usd: 500/' .hoom/items/precios.yaml && rm -f .hoom/items/precios.yaml.bak\nexit 0\n"},
		{"writer edita su pedido", "writer",
			"printf 'pedido: otro\\n' >> .hoom/items/precios.yaml\nexit 0\n"},
		{"writer se marca hecho", "writer",
			"printf 'hecho_en: 2026-09-25T10:00:00Z\\n' >> .hoom/items/precios.yaml\nexit 0\n"},
		{"writer borra su item", "writer",
			"rm .hoom/items/precios.yaml\nexit 0\n"},
		{"writer firma una review", "writer",
			"mkdir -p .hoom/reviews && printf '{}\\n' > .hoom/reviews/20260922T150405_ab12cd.json\nexit 0\n"},
		{"scout crea un item", "scout",
			"mkdir -p .hoom/items && printf 'titulo: x\\n' > .hoom/items/nuevo.yaml\nexit 0\n"},
	}
	for _, c := range casos {
		root := piRepo(t)
		fakeProvider(t, "claude", c.script)
		res, err := Run(root, "main", Options{Role: c.rol, Prompt: "hace tu trabajo"}, io.Discard)
		if err != nil {
			t.Fatalf("CA-292 (%s): %v", c.nombre, err)
		}
		if !res.Scope.Tampering || res.Stage != "scope" || res.ExitCode != 1 {
			t.Fatalf("CA-292 (%s): tocar .hoom/items/ o .hoom/reviews/ corta en scope como manipulacion: %+v", c.nombre, res)
		}
		if res.VerdictID != "" || res.Check != nil {
			t.Fatalf("CA-292 (%s): no se verifica un arbol manipulado: %+v", c.nombre, res)
		}
		if n := len(filesIn(t, filepath.Join(root, ".hoom", "verdicts"))); n != 0 {
			t.Fatalf("CA-292 (%s): verify no corrio: %d veredictos", c.nombre, n)
		}
		hay := false
		for _, v := range res.Scope.Violations {
			if v.Rule == RuleTampering && (filepath.Dir(v.Path) == ".hoom/items" || filepath.Dir(v.Path) == ".hoom/reviews") {
				hay = true
			}
		}
		if !hay {
			t.Fatalf("CA-292 (%s): la violacion nombra la ruta del item o de la review: %+v", c.nombre, res.Scope.Violations)
		}
	}
}
