// Tests adversariales del spec .hoom/specs/hoom-agent-sobre-determinista.md:
// el gate de scope. Mide el ARBOL, no la narracion; el piso append-only no se
// afloja desde el manifiesto.
package agentcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/ratchet"
)

func snap(touched map[string]string, evidencia ...string) Snapshot {
	s := Snapshot{Touched: touched, Evidence: map[string]bool{}, Manifest: "m0"}
	for _, e := range evidencia {
		s.Evidence[e] = true
	}
	return s
}

func todo() Policy { return Policy{Allow: []string{"**"}} }

func rutas(vs []Violation) map[string]Violation {
	out := map[string]Violation{}
	for _, v := range vs {
		out[v.Path] = v
	}
	return out
}

// CA-134: el delta del run es la diferencia por CONTENIDO entre las dos fotos.
func TestCA134_DeltaDelRun(t *testing.T) {
	root := repo(t)
	write(t, root, "viejo.go", "package viejo\n")
	write(t, root, "revertido.go", "package r\n")
	write(t, root, "sucio.go", "package s\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "base")

	// ya sucio ANTES del run
	write(t, root, "sucio.go", "package s // editado a mano\n")
	before := Take(root, "main")

	// lo que hace el "run"
	write(t, root, "nuevo.go", "package nuevo\n")
	write(t, root, "revertido.go", "package r // tocado\n")
	write(t, root, "revertido.go", "package r\n") // ...y devuelto
	write(t, root, "sucio.go", "package s // y ahora el run\n")
	if err := os.Remove(filepath.Join(root, "viejo.go")); err != nil {
		t.Fatal(err)
	}

	res := CheckScope(before, Take(root, "main"), todo())
	got := map[string]bool{}
	for _, p := range res.Touched {
		got[p] = true
	}
	for _, p := range []string{"nuevo.go", "viejo.go", "sucio.go"} {
		if !got[p] {
			t.Fatalf("CA-134: %s deberia estar en el delta: %v", p, res.Touched)
		}
	}
	if got["revertido.go"] {
		t.Fatalf("CA-134: un archivo devuelto a su contenido original no cambio el arbol: %v", res.Touched)
	}
	// la narracion que escribe hoom mientras el run ocurre no es obra del rol
	write(t, root, ".hoom/runs/20260101T000000_aa.jsonl", "{}\n")
	write(t, root, ".hoom/cache/live.jsonl", "{}\n")
	res = CheckScope(before, Take(root, "main"), todo())
	for _, p := range res.Touched {
		if p == ".hoom/runs/20260101T000000_aa.jsonl" || p == ".hoom/cache/live.jsonl" {
			t.Fatalf("CA-134: hoom no puede imputarle al rol lo que escribe hoom: %v", res.Touched)
		}
	}
}

// CA-135: el matcher de globs propio.
func TestCA135_MatchGlob(t *testing.T) {
	casos := []struct {
		pat, path string
		quiere    bool
	}{
		{".hoom/**", ".hoom/a.json", true},
		{".hoom/**", ".hoom/x/y/z.json", true},
		{".hoom/**", "hoom.yaml", false},
		{".hoom/**", "internal/.hoom/a.json", false},
		{"**", "cualquier/cosa.go", true},
		{"tests/**", "tests/unit/a_test.php", true},
		{"tests/**", "app/tests/a.php", false},
		{"**/*_test.go", "x_test.go", true},
		{"**/*_test.go", "internal/pkg/x_test.go", true},
		{"**/*_test.go", "internal/pkg/x.go", false},
		{"*.go", "a.go", true},
		{"*.go", "dir/a.go", false}, // * no cruza /
		{"src/?/a.go", "src/x/a.go", true},
		{".hoom/specs/**", ".hoom/specs/x.md", true},
		{".hoom/specs/**", ".hoom/findings/x.json", false},
	}
	for _, c := range casos {
		if got := matchGlob(c.pat, c.path); got != c.quiere {
			t.Fatalf("CA-135: matchGlob(%q, %q) = %v, se esperaba %v", c.pat, c.path, got, c.quiere)
		}
	}
}

// CA-136: cada forma de scope defiende su territorio.
func TestCA136_ScopePorFormaDeRol(t *testing.T) {
	casos := []struct {
		scope, path string
		violado     bool
	}{
		{agents.ScopeEvidencia, ".hoom/findings/f1.json", false},
		{agents.ScopeEvidencia, "internal/x.go", true},
		{agents.ScopeEvidencia, "README.md", true},
		{agents.ScopeTests, "tests/x_test.go", false},
		{agents.ScopeTests, "internal/x_test.go", false},
		{agents.ScopeTests, "internal/x.go", true},
		{agents.ScopeCodigo, "internal/x.go", false},
		{agents.ScopeCodigo, ".hoom/specs/y.md", true},
	}
	for _, c := range casos {
		rol := agents.Role{Slug: "r", Scope: c.scope}
		pol := PolicyFor(nil, rol)
		res := CheckScope(snap(nil), snap(map[string]string{c.path: "h1"}), pol)
		if c.violado != (len(res.Violations) == 1) {
			t.Fatalf("CA-136: scope %s con %s: violaciones = %+v", c.scope, c.path, res.Violations)
		}
		if !c.violado {
			continue
		}
		v := res.Violations[0]
		if v.Path != c.path || v.Rule != RuleOutOfScope || v.Detail == "" {
			t.Fatalf("CA-136: la violacion debe traer ruta, regla y detalle: %+v", v)
		}
		if res.Tampering {
			t.Fatalf("CA-136: escribir fuera de scope no es manipulacion: %+v", res)
		}
	}
}

// CA-137: el piso append-only. Agregar evidencia es trabajo; reescribirla,
// borrarla o firmar por el humano, no.
func TestCA137_ReglasUniversalesAppendOnly(t *testing.T) {
	existente := ".hoom/verdicts/2026-01-01T00-00-00Z_abc.json"
	hallazgo := ".hoom/findings/f-viejo.json"
	before := snap(map[string]string{existente: "h0", hallazgo: "h0"}, existente, hallazgo)

	// modificar un veredicto y un hallazgo existentes
	res := CheckScope(before, snap(map[string]string{existente: "h1", hallazgo: "-"}, existente, hallazgo), todo())
	vs := rutas(res.Violations)
	if vs[existente].Rule != RuleTampering || vs[hallazgo].Rule != RuleTampering {
		t.Fatalf("CA-137: modificar o borrar evidencia existente es manipulacion: %+v", res.Violations)
	}
	if !res.Tampering || res.OK {
		t.Fatalf("CA-137: el resultado debe quedar marcado como manipulacion: %+v", res)
	}

	// crear evidencia nueva: hoom verify / hoom finding add
	res = CheckScope(snap(nil), snap(map[string]string{
		".hoom/verdicts/nuevo.json":   "h1",
		".hoom/findings/f-nuevo.json": "h1",
	}), todo())
	if len(res.Violations) != 0 {
		t.Fatalf("CA-137: crear evidencia nueva es trabajo legitimo: %+v", res.Violations)
	}

	// aprobaciones: crear tambien es manipulacion
	res = CheckScope(snap(nil), snap(map[string]string{".hoom/approvals/spec.json": "h1"}), todo())
	if len(res.Violations) != 1 || res.Violations[0].Rule != RuleTampering {
		t.Fatalf("CA-137: firmar una aprobacion es manipulacion siempre: %+v", res.Violations)
	}

	// hoom.yaml: cualquier cambio
	res = CheckScope(snap(nil), snap(map[string]string{manifest.FileName: "h1"}), todo())
	if len(res.Violations) != 1 || res.Violations[0].Rule != RuleTampering {
		t.Fatalf("CA-137: cambiar hoom.yaml es manipulacion: %+v", res.Violations)
	}
	// ...incluso si no aparece en el diff: el contenido decide
	b, a := snap(nil), snap(nil)
	b.Manifest, a.Manifest = "antes", "despues"
	res = CheckScope(b, a, todo())
	if len(res.Violations) != 1 || res.Violations[0].Path != manifest.FileName {
		t.Fatalf("CA-137: el hash de hoom.yaml decide aunque git no lo vea: %+v", res.Violations)
	}
}

// CA-138: un trinquete aflojado es manipulacion; apretado, no.
func TestCA138_TrinqueteAflojadoEsManipulacion(t *testing.T) {
	v := func(x float64) *float64 { return &x }
	mk := func(val float64) *ratchet.File {
		return &ratchet.File{Schema: ratchet.Schema, Metrics: map[string]*ratchet.Metric{
			"cobertura": {Direction: "up", Value: v(val)},
		}}
	}
	before := snap(nil)
	before.Ratchet = mk(80)

	after := snap(map[string]string{ratchetPath: "h1"})
	after.Ratchet = mk(70)
	res := CheckScope(before, after, todo())
	if len(res.Violations) != 1 || res.Violations[0].Rule != RuleTampering ||
		res.Violations[0].Path != ratchetPath {
		t.Fatalf("CA-138: aflojar el trinquete es manipulacion: %+v", res.Violations)
	}

	apretado := snap(map[string]string{ratchetPath: "h1"})
	apretado.Ratchet = mk(90)
	if res := CheckScope(before, apretado, todo()); len(res.Violations) != 0 {
		t.Fatalf("CA-138: apretar el trinquete es lo que verify --full hace bien: %+v", res.Violations)
	}

	// aflojado sin que el archivo aparezca en el delta: igual se caza
	oculto := snap(nil)
	oculto.Ratchet = mk(70)
	res = CheckScope(before, oculto, todo())
	if len(res.Violations) != 1 || res.Violations[0].Path != ratchetPath {
		t.Fatalf("CA-138: la aflojada se juzga por significado, no por el diff: %+v", res.Violations)
	}
}

// CA-139: el manifiesto puede reapuntar el territorio de un rol, jamas bajar
// el piso.
func TestCA139_OverrideDelManifiestoYPisoInamovible(t *testing.T) {
	rol := agents.Role{Slug: "test-writer", Scope: agents.ScopeTests}

	// allow declarado REEMPLAZA los defaults
	m := &manifest.Manifest{Agents: map[string]manifest.AgentPolicy{
		"test-writer": {Write: manifest.WriteScope{Allow: []string{"Tests/**"}, Deny: []string{"Tests/prohibido/**"}}},
	}}
	pol := PolicyFor(m, rol)
	if len(pol.Allow) != 1 || pol.Allow[0] != "Tests/**" {
		t.Fatalf("CA-139: el allow declarado reemplaza los defaults: %+v", pol)
	}
	res := CheckScope(snap(nil), snap(map[string]string{"tests/x_test.go": "h1"}), pol)
	if len(res.Violations) != 1 {
		t.Fatalf("CA-139: con el layout redeclarado, el default deja de valer: %+v", res.Violations)
	}
	res = CheckScope(snap(nil), snap(map[string]string{"Tests/prohibido/x.php": "h1"}), pol)
	if len(res.Violations) != 1 || res.Violations[0].Rule != RuleOutOfScope {
		t.Fatalf("CA-139: el deny se SUMA y gana sobre el allow: %+v", res.Violations)
	}

	// allow vacio = no declarado: valen los defaults
	vacio := &manifest.Manifest{Agents: map[string]manifest.AgentPolicy{
		"test-writer": {Write: manifest.WriteScope{Allow: []string{"", "  "}}},
	}}
	if pol := PolicyFor(vacio, rol); len(pol.Allow) < 2 {
		t.Fatalf("CA-139: un allow vacio deja los defaults en pie: %+v", pol)
	}

	// el piso no se alcanza desde el manifiesto
	abierto := &manifest.Manifest{Agents: map[string]manifest.AgentPolicy{
		"test-writer": {Write: manifest.WriteScope{Allow: []string{"**", ".hoom/**", "hoom.yaml"}}},
	}}
	existente := ".hoom/verdicts/viejo.json"
	before := snap(map[string]string{existente: "h0"}, existente)
	after := snap(map[string]string{
		existente:                   "h1",
		".hoom/approvals/spec.json": "h1",
		manifest.FileName:           "h1",
	}, existente)
	res = CheckScope(before, after, PolicyFor(abierto, rol))
	if len(res.Violations) != 3 || !res.Tampering {
		t.Fatalf("CA-139: ningun manifiesto afloja el piso append-only: %+v", res.Violations)
	}
	for _, v := range res.Violations {
		if v.Rule != RuleTampering {
			t.Fatalf("CA-139: las tres deben ser manipulacion: %+v", res.Violations)
		}
	}
}
