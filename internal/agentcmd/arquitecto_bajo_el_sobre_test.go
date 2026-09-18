// Tests adversariales del spec .hoom/specs/arquitecto-bajo-el-sobre.md: el
// sobre dejaba pasar como trabajo algo que no lo era. Los autores de specs
// escriben (solo specs, sin shell); un rol que escribe y no entrega no produce
// veredicto; un pedido de relleno no gasta un token; y los hallazgos del gate
// saben de que tarea son.
package agentcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/ratchet"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// ---------------------------------------------------------------- helpers

var abAutores = []string{"arquitecto", "designer", "analista"}

// abNota es la nota exacta que el spec fija para el cierre sin entrega.
func abNota(rol string) string {
	return fmt.Sprintf("el rol %s escribe y el run no dejo ningun archivo: no hay arbol nuevo que certificar", rol)
}

func abCuantos(t *testing.T, dir string) int {
	t.Helper()
	return len(filesIn(t, dir))
}

func abVeredictos(t *testing.T, root string) int {
	t.Helper()
	return abCuantos(t, filepath.Join(root, ".hoom", "verdicts"))
}

func abHallazgos(t *testing.T, root string) int {
	t.Helper()
	return abCuantos(t, filepath.Join(root, ".hoom", "findings"))
}

// abScriptSpec es lo que hace un arquitecto que SI trabaja: escribe un spec.
const abScriptSpec = "mkdir -p .hoom/specs\ncat > .hoom/specs/nuevo.md <<'EOF'\n# Spec nuevo\n\n" +
	"## Objetivo\nAlgo.\n\n## No-goals\nNada.\n\n## Contratos\nNinguno.\n\n## Casos limite\nNinguno.\n\n" +
	"## Criterios de aceptacion\n- CA-1: algo. [verifica: true]\n\n## Decisiones\nNinguna.\n\n## Riesgos\nNinguno.\nEOF\n"

// abScriptHallazgo escribe un hallazgo con el formato exacto del artefacto,
// que es lo que deja `hoom finding add` (o un rol que lo imita).
func abScriptHallazgo(id string) string {
	return "mkdir -p .hoom/findings\ncat > .hoom/findings/" + id + ".json <<'EOF'\n" +
		`{"id":"` + id + `","created_at":"2026-09-18T12:00:00Z","severity":"medium",` +
		`"lens":"reliability","file":"app.go","description":"hallazgo del rol","author":"rol@claude"}` +
		"\nEOF\n"
}

// abArgv lee el argv que el provider falso volco (un argumento por linea) y
// devuelve los valores de --allowedTools y --disallowedTools.
func abArgv(t *testing.T, dump string) (allow, deny []string) {
	t.Helper()
	raw, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("el provider falso debia volcar su argv: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	valor := func(flag string) []string {
		for i, l := range lines {
			if l == flag && i+1 < len(lines) {
				return strings.Split(lines[i+1], ",")
			}
		}
		return nil
	}
	return valor("--allowedTools"), valor("--disallowedTools")
}

func abTiene(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// abHallazgoEn busca un hallazgo por id en cualquiera de los directorios.
func abHallazgoEn(t *testing.T, id string, dirs ...string) (finding.Finding, bool) {
	t.Helper()
	for _, d := range dirs {
		items, _, err := finding.List(d, "main", false)
		if err != nil {
			continue
		}
		for _, it := range items {
			if it.ID == id {
				return it.Finding, true
			}
		}
	}
	return finding.Finding{}, false
}

// abSinEntrega verifica el cierre sin entrega de un sobre ya corrido.
func abSinEntrega(t *testing.T, ca string, res Result, out, rol string) {
	t.Helper()
	if res.Stage != "scope" || res.Status != "sin-entrega" || res.ExitCode != 1 {
		t.Fatalf("%s: un rol que escribe y no entrega cierra scope/sin-entrega/1: stage=%q status=%q exit=%d\n%s",
			ca, res.Stage, res.Status, res.ExitCode, out)
	}
	if res.RunID == "" || res.RunStatus != "done" {
		t.Fatalf("%s: el run si corrio y termino bien: %+v", ca, res)
	}
	if res.VerdictID != "" || res.Verdict != "" || res.Check != nil {
		t.Fatalf("%s: sin entrega no corren verify ni check: %+v", ca, res)
	}
	if !strings.Contains(out, "SIN ENTREGA") {
		t.Fatalf("%s: la salida debe decir SIN ENTREGA:\n%s", ca, out)
	}
	if !strings.Contains(out, abNota(rol)) {
		t.Fatalf("%s: la salida debe traer la nota %q:\n%s", ca, abNota(rol), out)
	}
	if strings.Contains(out, "NO ENTREGABLE") {
		t.Fatalf("%s: sin entrega no es NO ENTREGABLE:\n%s", ca, out)
	}
	if len(res.Scope.Delivered()) != 0 {
		t.Fatalf("%s: sin entrega quiere decir Delivered vacio: %v", ca, res.Scope.Delivered())
	}
}

// ---------------------------------------------------------------- CA-230

// CA-230: PolicyFor con forma specs permite exactamente .hoom/specs/**.
func TestCA230_PolicyForFormaSpecs(t *testing.T) {
	rol := agents.Role{Slug: "arquitecto", Scope: agents.ScopeSpecs}
	pol := PolicyFor(nil, rol)
	if len(pol.Allow) != 1 || pol.Allow[0] != ".hoom/specs/**" || len(pol.Deny) != 0 {
		t.Fatalf("CA-230: la forma specs es allow .hoom/specs/** sin deny: %+v", pol)
	}
	casos := []struct {
		path    string
		violado bool
	}{
		{".hoom/specs/x.md", false},
		{".hoom/specs/sub/y-ui.md", false},
		{".hoom/specs/00-vision.md", false},
		{"internal/x.go", true},
		{".hoom/agents/01-arquitecto.md", true},
		{".hoom/intake/doc.md", true},
		{".hoom/findings/20260918T120000_abc123.json", true}, // un hallazgo NUEVO
		{".hoom/verdicts/2026-09-18T12-00-00Z_nuevo.json", true},
		{".hoom/specsx/a.md", true},
		{"docs/.hoom/specs/x.md", true},
		{"README.md", true},
	}
	for _, c := range casos {
		res := CheckScope(snap(nil), snap(map[string]string{c.path: "h1"}), pol)
		if !c.violado {
			if len(res.Violations) != 0 || !res.OK {
				t.Fatalf("CA-230: %s es territorio de un autor de specs: %+v", c.path, res)
			}
			continue
		}
		if len(res.Violations) != 1 {
			t.Fatalf("CA-230: %s debe quedar fuera de scope: %+v", c.path, res.Violations)
		}
		v := res.Violations[0]
		if v.Path != c.path || v.Rule != RuleOutOfScope || v.Detail == "" || res.Tampering {
			t.Fatalf("CA-230: %s es fuera-de-scope, no manipulacion: %+v", c.path, res)
		}
	}

	// los tres autores de la tabla reciben esa misma politica
	for _, slug := range abAutores {
		r, err := agents.Lookup(slug)
		if err != nil {
			t.Fatal(err)
		}
		p := PolicyFor(nil, r)
		if !reflect.DeepEqual(p.Allow, []string{".hoom/specs/**"}) || len(p.Deny) != 0 {
			t.Fatalf("CA-230: %s escribe solo en .hoom/specs/**: %+v", slug, p)
		}
	}

	// un allow declarado en hoom.yaml lo reemplaza
	m := &manifest.Manifest{Agents: map[string]manifest.AgentPolicy{
		"arquitecto": {Write: manifest.WriteScope{Allow: []string{"docs/specs/**"}}},
	}}
	pol = PolicyFor(m, rol)
	if !reflect.DeepEqual(pol.Allow, []string{"docs/specs/**"}) {
		t.Fatalf("CA-230: el allow del manifiesto reemplaza al de la forma: %+v", pol)
	}
	if res := CheckScope(snap(nil), snap(map[string]string{"docs/specs/a.md": "h1"}), pol); len(res.Violations) != 0 {
		t.Fatalf("CA-230: el territorio re-apuntado vale: %+v", res.Violations)
	}
	if res := CheckScope(snap(nil), snap(map[string]string{".hoom/specs/x.md": "h1"}), pol); len(res.Violations) != 1 {
		t.Fatalf("CA-230: con el allow re-apuntado, el default deja de valer: %+v", res.Violations)
	}

	// ["**"] amplia el territorio, pero el piso append-only no se afloja
	abierto := &manifest.Manifest{Agents: map[string]manifest.AgentPolicy{
		"arquitecto": {Write: manifest.WriteScope{Allow: []string{"**"}}},
	}}
	res := CheckScope(snap(nil), snap(map[string]string{manifest.FileName: "h1", "internal/x.go": "h1"}), PolicyFor(abierto, rol))
	vs := rutas(res.Violations)
	if vs[manifest.FileName].Rule != RuleTampering {
		t.Fatalf("CA-230: ningun allow afloja el piso: %+v", res.Violations)
	}
	if _, ok := vs["internal/x.go"]; ok {
		t.Fatalf("CA-230: con allow ** el codigo es territorio del rol: %+v", res.Violations)
	}
}

// CA-230 y CA-232 (caso limite): hoom.yaml con agents.arquitecto.write.allow
// ["**"] amplia el territorio; el shell sigue negado porque sale de Exec en la
// tabla y no del scope.
func TestCA230_ManifiestoAmpliaElTerritorioNoElShell(t *testing.T) {
	root := repo(t)
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n"+
		"agents:\n  arquitecto:\n    write:\n      allow: [\"**\"]\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "territorio ampliado")
	dump := filepath.Join(t.TempDir(), "argv.txt")
	fakeProvider(t, "claude", "printf '%s\\n' \"$@\" > "+dump+"\nmkdir -p internal\nprintf 'package x\\n' > internal/x.go\nexit 0\n")

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "arquitecto", Prompt: "escribi el spec"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scope.OK || len(res.Scope.Violations) != 0 {
		t.Fatalf("CA-230: con allow ** el arquitecto puede escribir codigo: %+v\n%s", res.Scope, buf.String())
	}
	if !abTiene(res.Scope.Delivered(), "internal/x.go") {
		t.Fatalf("CA-230: lo que escribio es entrega: %v", res.Scope.Delivered())
	}
	allow, deny := abArgv(t, dump)
	if abTiene(allow, "Bash") || !abTiene(deny, "Bash") {
		t.Fatalf("CA-232: el territorio ampliado no devuelve el shell: allow=%v deny=%v", allow, deny)
	}
	if !abTiene(allow, "Write") {
		t.Fatalf("CA-232: el arquitecto escribe: allow=%v", allow)
	}
}

// ---------------------------------------------------------------- CA-232

// CA-232: NoExecFor resuelve el "sin shell" contra lo que el provider
// DECLARA: solo un rol que escribe sin Exec lo pide, y quien no puede
// imponerlo avisa.
func TestCA232_NoExecFor(t *testing.T) {
	claude, _ := providers.Lookup("claude")
	codex, _ := providers.Lookup("codex")
	for _, slug := range abAutores {
		r, err := agents.Lookup(slug)
		if err != nil {
			t.Fatal(err)
		}
		if ne, warn := NoExecFor(claude, r); !ne || warn {
			t.Fatalf("CA-232: %s bajo claude escribe sin shell y sin aviso: noExec=%v warn=%v", slug, ne, warn)
		}
		if ne, warn := NoExecFor(codex, r); ne || !warn {
			t.Fatalf("CA-232: %s bajo codex no puede quitarle el shell y avisa: noExec=%v warn=%v", slug, ne, warn)
		}
		if ne, warn := NoExecFor(providerMudo{}, r); ne || !warn {
			t.Fatalf("CA-232: un provider que no declara no_exec avisa: noExec=%v warn=%v", ne, warn)
		}
		// CA-131 no cambia: un rol que escribe no se limita a solo lectura
		if ro, ex, warn := ReadOnlyFor(claude, r); ro || ex || warn {
			t.Fatalf("CA-232: %s escribe, ReadOnlyFor no lo limita: ro=%v exec=%v warn=%v", slug, ro, ex, warn)
		}
	}
	for _, slug := range []string{"writer", "test-writer", "characterizer", "scout", "reviewer", "refutador", "orquestador"} {
		r, err := agents.Lookup(slug)
		if err != nil {
			t.Fatal(err)
		}
		for _, p := range []providers.Provider{claude, codex, providerMudo{}} {
			if ne, warn := NoExecFor(p, r); ne || warn {
				t.Fatalf("CA-232: %s bajo %s no pide sacar el shell ni avisa: noExec=%v warn=%v", slug, p.Name(), ne, warn)
			}
		}
	}
}

// CA-232: startOptions arma NoExec para el arquitecto bajo claude, y su
// traduccion permite Write y prohibe Bash; el writer conserva Bash.
func TestCA232_StartOptionsArmaNoExec(t *testing.T) {
	claude, _ := providers.Lookup("claude")
	codex, _ := providers.Lookup("codex")
	arq, _ := agents.Lookup("arquitecto")
	writer, _ := agents.Lookup("writer")
	scout, _ := agents.Lookup("scout")

	so, warn := startOptions(claude, arq, "C", Options{Prompt: "escribi el spec"})
	if !so.NoExec || so.ReadOnly || so.Exec || warn {
		t.Fatalf("CA-232: el arquitecto bajo claude escribe sin shell: %+v warn=%v", so, warn)
	}
	if !so.Unattended || !so.Strict || so.SystemPrompt != "C" || so.Role != "arquitecto" {
		t.Fatalf("CA-232: lo demas del sobre no cambia: %+v", so)
	}
	inv, err := claude.Command(providers.Request{Prompt: so.Prompt, SystemPrompt: so.SystemPrompt,
		ReadOnly: so.ReadOnly, Exec: so.Exec, NoExec: so.NoExec, Unattended: so.Unattended, Strict: so.Strict})
	if err != nil {
		t.Fatalf("CA-232: claude acepta el pedido del arquitecto bajo Strict: %v", err)
	}
	allow := strings.Split(abFlag(inv.Args, "--allowedTools"), ",")
	deny := strings.Split(abFlag(inv.Args, "--disallowedTools"), ",")
	if !abTiene(allow, "Write") || abTiene(allow, "Bash") || !abTiene(deny, "Bash") {
		t.Fatalf("CA-232: el arquitecto escribe y no ejecuta: %v", inv.Args)
	}

	if so, _ = startOptions(codex, arq, "C", Options{Prompt: "escribi el spec"}); so.NoExec {
		t.Fatalf("CA-232: codex no declara no_exec: el sobre no se lo pide (se negaria bajo Strict): %+v", so)
	}
	if so, _ = startOptions(claude, writer, "C", Options{Prompt: "implementa"}); so.NoExec {
		t.Fatalf("CA-232: el writer conserva su shell: %+v", so)
	}
	if so, _ = startOptions(claude, scout, "C", Options{Prompt: "explora"}); so.NoExec || !so.ReadOnly {
		t.Fatalf("CA-232: un rol de solo lectura no pide NoExec: %+v", so)
	}
}

func abFlag(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

// CA-232: el argv FINAL que recibe el CLI. El del arquitecto permite Write y
// prohibe Bash; el del writer conserva Bash.
func TestCA232_ArgvFinalDelSobre(t *testing.T) {
	root := repo(t)
	dump := filepath.Join(t.TempDir(), "argv.txt")
	fakeProvider(t, "claude", "printf '%s\\n' \"$@\" > "+dump+"\n"+abScriptSpec+"exit 0\n")
	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "arquitecto", Prompt: "escribi el spec"}, &buf); err != nil {
		t.Fatal(err)
	}
	allow, deny := abArgv(t, dump)
	if !abTiene(allow, "Write") || !abTiene(allow, "Edit") {
		t.Fatalf("CA-232: el arquitecto recibe la escritura por nombre: allow=%v", allow)
	}
	if abTiene(allow, "Bash") || !abTiene(deny, "Bash") {
		t.Fatalf("CA-232: el arquitecto no recibe el shell: allow=%v deny=%v", allow, deny)
	}
	if abTiene(deny, "Write") || abTiene(deny, "Edit") {
		t.Fatalf("CA-232: el arquitecto no tiene la escritura prohibida: deny=%v", deny)
	}
	if strings.Contains(buf.String(), "no puede quitarle el shell") {
		t.Fatalf("CA-232: claude puede: no hay aviso:\n%s", buf.String())
	}

	root2 := repo(t)
	dump2 := filepath.Join(t.TempDir(), "argv.txt")
	fakeProvider(t, "claude", "printf '%s\\n' \"$@\" > "+dump2+"\nprintf 'package app // hecho\\n' > app.go\nexit 0\n")
	if _, err := Run(root2, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	allow, deny = abArgv(t, dump2)
	if !abTiene(allow, "Bash") || abTiene(deny, "Bash") {
		t.Fatalf("CA-232: el writer conserva su shell: allow=%v deny=%v", allow, deny)
	}
}

// CA-232: bajo Codex el sobre imprime el aviso y el run arranca igual: el gate
// de scope es la red.
func TestCA232_BajoCodexAvisaYArranca(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "codex", abScriptSpec+"exit 0\n")
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "arquitecto", Provider: "codex", Prompt: "escribi el spec"}, &buf)
	if err != nil {
		t.Fatalf("CA-232: sin la capacidad el sobre no se niega: %v\n%s", err, buf.String())
	}
	aviso := "aviso: codex no puede quitarle el shell a un rol que escribe; su escritura se verifica solo despues del run"
	if !strings.Contains(buf.String(), aviso) {
		t.Fatalf("CA-232: falta el aviso %q en:\n%s", aviso, buf.String())
	}
	if res.RunID == "" || res.Provider != "codex" {
		t.Fatalf("CA-232: el run arranca igual: %+v", res)
	}
}

// ---------------------------------------------------------------- CA-234

// CA-234: --spec con un autor de specs se rechaza antes de todo: el provider
// nunca se invoca y no quedan run ni registro.
func TestCA234_SpecConAutorDeSpecsSeNiega(t *testing.T) {
	for _, rol := range []string{"arquitecto", "designer", "analista", "hoom-arquitecto"} {
		root := repo(t)
		spec := specDemo(t, root)
		marca := filepath.Join(t.TempDir(), "invocado")
		fakeProvider(t, "claude", "touch "+marca+"\nexit 0\n")

		var buf bytes.Buffer
		_, err := Run(root, "main", Options{Role: rol, Spec: spec, Prompt: "escribi el spec"}, &buf)
		if err == nil {
			t.Fatalf("CA-234: --spec con %s debe ser error:\n%s", rol, buf.String())
		}
		slug := strings.TrimPrefix(rol, "hoom-")
		msg := err.Error()
		if !strings.Contains(msg, slug) || !strings.Contains(msg, "escribe specs") ||
			!strings.Contains(msg, "Nombra el spec en el pedido") {
			t.Fatalf("CA-234: el mensaje nombra el rol y pide nombrar el spec en el pedido: %v", err)
		}
		if rol == slug && !strings.Contains(msg, "el rol "+slug+" escribe specs") {
			t.Fatalf("CA-234: el mensaje empieza nombrando el rol: %v", err)
		}
		if existeEn(t, marca) {
			t.Fatalf("CA-234: el provider no se invoca con %s", rol)
		}
		if n := runsCount(t, root); n != 0 {
			t.Fatalf("CA-234: no queda run: %d archivos en .hoom/runs", n)
		}
		if recs := sobres(t, root); len(recs) != 0 {
			t.Fatalf("CA-234: no queda registro de sobre: %+v", recs)
		}
	}

	// un spec aprobado tampoco lo habilita: no se reinterpreta
	root := repo(t)
	spec := specDemo(t, root)
	if _, _, err := approval.Approve(root, filepath.Join(root, spec)); err != nil {
		t.Fatal(err)
	}
	marca := filepath.Join(t.TempDir(), "invocado")
	fakeProvider(t, "claude", "touch "+marca+"\nexit 0\n")
	if _, err := Run(root, "main", Options{Role: "arquitecto", Spec: spec, Prompt: "reescribi"}, io.Discard); err == nil ||
		!strings.Contains(err.Error(), "escribe specs") {
		t.Fatalf("CA-234: aprobado o no, un autor no trabaja con --spec: %v", err)
	}
	if existeEn(t, marca) || runsCount(t, root) != 0 {
		t.Fatal("CA-234: tampoco se invoca el provider ni queda run")
	}

	// antes de elegir provider: sin ningun CLI instalado el error es el mismo
	t.Setenv("PATH", t.TempDir())
	_, err := Run(root, "main", Options{Role: "designer", Spec: spec, Prompt: "escribi el ui-spec"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "escribe specs") || strings.Contains(err.Error(), "system_prompt") {
		t.Fatalf("CA-234: la validacion va antes de elegir provider: %v", err)
	}
}

// ---------------------------------------------------------------- CA-236

// CA-236: Delivered es Touched sin lo que generan los comandos de hoom, en el
// mismo orden; el JSON de ScopeResult no cambia.
func TestCA236_DeliveredSacaLaEvidenciaDeHoom(t *testing.T) {
	touched := []string{
		"app.go",
		".hoom/verdicts/2026-09-18T12-00-00Z_a.json",
		".hoom/specs/nuevo.md",
		".hoom/findings/20260918T120000_abc123.json",
		".hoom/findings/20260918T120000_abc123.res.json",
		".hoom/ratchet.json",
		"internal/.hoom/verdicts/x.json",
		".hoom/ratchet.json.bak",
		".hoom/verdicts-viejos/x.json",
		".hoom/findings-borrador.md",
		".hoom/approvals/spec.json",
		"b.go",
	}
	s := ScopeResult{Touched: append([]string(nil), touched...)}
	quiere := []string{
		"app.go",
		".hoom/specs/nuevo.md",
		"internal/.hoom/verdicts/x.json",
		".hoom/ratchet.json.bak",
		".hoom/verdicts-viejos/x.json",
		".hoom/findings-borrador.md",
		".hoom/approvals/spec.json",
		"b.go",
	}
	if got := s.Delivered(); !reflect.DeepEqual(got, quiere) {
		t.Fatalf("CA-236: Delivered = %v, se esperaba %v", got, quiere)
	}
	if !reflect.DeepEqual(s.Touched, touched) {
		t.Fatalf("CA-236: Delivered no puede tocar Touched: %v", s.Touched)
	}

	if got := (ScopeResult{}).Delivered(); len(got) != 0 {
		t.Fatalf("CA-236: sin nada tocado no hay entrega: %v", got)
	}
	solo := ScopeResult{Touched: []string{".hoom/verdicts/v.json", ".hoom/findings/f.json", ".hoom/ratchet.json"}}
	if got := solo.Delivered(); len(got) != 0 {
		t.Fatalf("CA-236: veredictos, hallazgos y trinquete no son entrega: %v", got)
	}

	// el JSON conserva EXACTAMENTE sus claves: Delivered es metodo, no campo
	raw, err := json.Marshal(ScopeResult{Touched: touched, Tampering: true,
		Violations: []Violation{{Path: "b.go", Rule: RuleOutOfScope, Detail: "x"}}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	claves := map[string]bool{}
	for k := range got {
		claves[k] = true
	}
	if len(claves) != 4 || !claves["touched"] || !claves["violations"] || !claves["tampering"] || !claves["ok"] {
		t.Fatalf("CA-236: el JSON de ScopeResult son touched, violations, tampering y ok, nada mas: %s", raw)
	}
}

// CA-236: propiedad. Para cualquier Touched sin repetidos, Delivered es la
// subsecuencia de lo que no es evidencia generada por hoom, en el mismo orden.
func TestCA236_DeliveredPropiedad(t *testing.T) {
	pool := []string{
		"app.go", "b/c.go", "README.md", ".hoom/specs/x.md", ".hoom/specs/sub/y-ui.md",
		".hoom/verdicts/v.json", ".hoom/verdicts/sub/w.json", ".hoom/findings/f.json",
		".hoom/findings/f.res.json", ".hoom/ratchet.json", ".hoom/ratchet.jsonx",
		"x/.hoom/findings/f.json", ".hoom/approvals/a.json", ".hoom/agents/01-arquitecto.md",
		"hoom.yaml", ".hoom/intake/doc.md",
	}
	evidencia := func(p string) bool {
		return strings.HasPrefix(p, ".hoom/verdicts/") || strings.HasPrefix(p, ".hoom/findings/") ||
			p == ".hoom/ratchet.json"
	}
	prop := func(idx []uint8) bool {
		visto := map[string]bool{}
		var touched []string
		for _, i := range idx {
			p := pool[int(i)%len(pool)]
			if !visto[p] {
				visto[p] = true
				touched = append(touched, p)
			}
		}
		var quiere []string
		for _, p := range touched {
			if !evidencia(p) {
				quiere = append(quiere, p)
			}
		}
		got := ScopeResult{Touched: touched}.Delivered()
		if len(got) != len(quiere) {
			return false
		}
		for i := range quiere {
			if got[i] != quiere[i] {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("CA-236: Delivered no es Touched menos la evidencia de hoom: %v", err)
	}
}

// CA-236 y CA-230: el dogfood arreglado. El arquitecto escribe su spec, el
// gate lo acepta, Delivered lo nombra y el sobre certifica el arbol nuevo.
func TestCA236_ElArquitectoEntregaSuSpec(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", abScriptSpec+"exit 0\n")
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "arquitecto", Prompt: "escribi el spec"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scope.OK || !reflect.DeepEqual(res.Scope.Delivered(), []string{".hoom/specs/nuevo.md"}) {
		t.Fatalf("CA-236: el spec es la entrega del arquitecto: %+v delivered=%v\n%s",
			res.Scope, res.Scope.Delivered(), buf.String())
	}
	if res.Stage != "ok" || res.Status != "entregable" || res.ExitCode != 0 {
		t.Fatalf("CA-236: un autor que entrega sobre un arbol verde cierra entregable: %+v\n%s", res, buf.String())
	}
	if res.VerdictID == "" || res.Check == nil || !res.Check.OK {
		t.Fatalf("CA-236: el arbol nuevo se certifica: %+v", res)
	}
}

// ---------------------------------------------------------------- CA-237

// CA-237: un writer cuyo run sale 0 sin cambiar el arbol cierra sin entrega;
// lo mismo si solo creo un veredicto, solo un hallazgo o solo apreto el
// trinquete, si el arbol ya estaba sucio o si devolvio su edicion.
func TestCA237_WriterSinEntrega(t *testing.T) {
	t.Run("no toca nada", func(t *testing.T) {
		root := repo(t)
		fakeProvider(t, "claude", "exit 0\n")
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
		if err != nil {
			t.Fatal(err)
		}
		abSinEntrega(t, "CA-237", res, buf.String(), "writer")
		if n := abVeredictos(t, root); n != 0 {
			t.Fatalf("CA-237: no hay archivos nuevos en .hoom/verdicts/: %d", n)
		}
		if n := abHallazgos(t, root); n != 0 {
			t.Fatalf("CA-237: sin-entrega es un estado, no una violacion: no crea hallazgo (%d)", n)
		}
	})

	t.Run("solo corrio hoom verify", func(t *testing.T) {
		root := repo(t)
		propio := "2026-09-18T12-00-00Z_propio.json"
		fakeProvider(t, "claude", "mkdir -p .hoom/verdicts\nprintf '{\"verdict\":\"green\"}\\n' > .hoom/verdicts/"+propio+"\nexit 0\n")
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
		if err != nil {
			t.Fatal(err)
		}
		abSinEntrega(t, "CA-237", res, buf.String(), "writer")
		if vs := filesIn(t, filepath.Join(root, ".hoom", "verdicts")); len(vs) != 1 || vs[0] != propio {
			t.Fatalf("CA-237: el veredicto del rol queda (append-only) y el sobre no agrega otro: %v", vs)
		}
	})

	t.Run("solo registro un hallazgo", func(t *testing.T) {
		root := repo(t)
		fakeProvider(t, "claude", abScriptHallazgo("20260918T120000_aaaaaa")+"exit 0\n")
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
		if err != nil {
			t.Fatal(err)
		}
		abSinEntrega(t, "CA-237", res, buf.String(), "writer")
		if abVeredictos(t, root) != 0 {
			t.Fatal("CA-237: un hallazgo no es entrega: no se corre verify")
		}
		if _, ok := abHallazgoEn(t, "20260918T120000_aaaaaa", root); !ok {
			t.Fatal("CA-237: el hallazgo del rol queda registrado")
		}
	})

	t.Run("solo apreto el trinquete", func(t *testing.T) {
		root := repo(t)
		v80, v90 := 80.0, 90.0
		mk := func(v *float64) string {
			raw, err := json.Marshal(ratchet.File{Schema: ratchet.Schema, Metrics: map[string]*ratchet.Metric{
				"cobertura": {Cmd: "echo 80", Direction: "up", Value: v},
			}})
			if err != nil {
				t.Fatal(err)
			}
			return string(raw)
		}
		write(t, root, ".hoom/ratchet.json", mk(&v80)+"\n")
		git(t, root, "add", "-A")
		git(t, root, "commit", "-m", "linea base")
		fakeProvider(t, "claude", "cat > .hoom/ratchet.json <<'EOF'\n"+mk(&v90)+"\nEOF\nexit 0\n")
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
		if err != nil {
			t.Fatal(err)
		}
		abSinEntrega(t, "CA-237", res, buf.String(), "writer")
	})

	t.Run("arbol sucio antes del run", func(t *testing.T) {
		root := repo(t)
		write(t, root, "app.go", "package app // editado a mano antes del run\n")
		fakeProvider(t, "claude", "exit 0\n")
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
		if err != nil {
			t.Fatal(err)
		}
		abSinEntrega(t, "CA-237", res, buf.String(), "writer")
	})

	t.Run("edicion devuelta a su contenido", func(t *testing.T) {
		root := repo(t)
		fakeProvider(t, "claude", "printf 'package app // tocado\\n' > app.go\nprintf 'package app\\n' > app.go\nexit 0\n")
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
		if err != nil {
			t.Fatal(err)
		}
		abSinEntrega(t, "CA-237", res, buf.String(), "writer")
	})

	// el borde: sin-entrega es para un run que TERMINO BIEN; uno fallido
	// sigue cerrando en run con el exit del provider
	t.Run("run fallido no es sin entrega", func(t *testing.T) {
		root := repo(t)
		fakeProvider(t, "claude", "exit 3\n")
		res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
		if err != nil {
			t.Fatal(err)
		}
		if res.Stage != "run" || res.ExitCode != 3 || res.Status == "sin-entrega" {
			t.Fatalf("CA-237: un run fallido corta en run con su exit: %+v", res)
		}
	})
}

// CA-237: los autores de specs que no escriben tambien cierran sin entrega
// (Analista en Modo C: pide la entrevista y no escribe).
func TestCA237_AutoresQueNoEscribenNoEntregan(t *testing.T) {
	for _, rol := range abAutores {
		root := repo(t)
		fakeProvider(t, "claude", "echo 'necesito la entrevista fundacional'\nexit 0\n")
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: rol, Prompt: "arranca"}, &buf)
		if err != nil {
			t.Fatal(err)
		}
		abSinEntrega(t, "CA-237", res, buf.String(), rol)
		if abVeredictos(t, root) != 0 {
			t.Fatalf("CA-237: el %s no entrego: no hay veredicto de la nada", rol)
		}
	}
}

// CA-237 y CA-230 (caso limite): un autor de specs que solo registro un
// hallazgo (posible bajo Codex). El hallazgo es fuera de scope para la forma
// specs y ademas no es entrega: sin-entrega, con la violacion y el hallazgo
// del gate igual registrados.
func TestCA237_AutorQueSoloRegistraUnHallazgo(t *testing.T) {
	root := repo(t)
	id := "20260918T120000_bbbbbb"
	fakeProvider(t, "claude", abScriptHallazgo(id)+"exit 0\n")
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "arquitecto", Prompt: "escribi el spec"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	abSinEntrega(t, "CA-237", res, buf.String(), "arquitecto")
	if len(res.Scope.Violations) != 1 {
		t.Fatalf("CA-230: el hallazgo del arquitecto esta fuera de su scope: %+v", res.Scope)
	}
	v := res.Scope.Violations[0]
	if v.Path != ".hoom/findings/"+id+".json" || v.Rule != RuleOutOfScope || v.FindingID == "" {
		t.Fatalf("CA-230: la violacion nombra la ruta y deja su hallazgo: %+v", v)
	}
	if _, ok := abHallazgoEn(t, v.FindingID, root); !ok {
		t.Fatalf("CA-237: el hallazgo del gate %s quedo registrado", v.FindingID)
	}
	if _, ok := abHallazgoEn(t, id, root); !ok {
		t.Fatal("CA-237: el hallazgo del rol tambien queda")
	}
}

// ---------------------------------------------------------------- CA-238

// CA-238: un test-writer ciego que no escribio tests cierra sin entrega con
// la cuarentena cerrada; un hallazgo que creo adentro llega al arbol real.
func TestCA238_TestWriterCiegoSinEntrega(t *testing.T) {
	root := repoCiego(t)
	fakeProvider(t, "claude", "exit 0\n")
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	abSinEntrega(t, "CA-238", res, buf.String(), "test-writer")
	if res.Isolation == nil || res.Isolation.Kept || existeEn(t, res.Isolation.Dir) {
		t.Fatalf("CA-238: no quedo nada adentro: la cuarentena se cierra: %+v", res.Isolation)
	}

	// el rol registro un hallazgo en la cuarentena y nada mas
	root2 := repoCiego(t)
	id := "20260918T120000_cccccc"
	fakeProvider(t, "claude", abScriptHallazgo(id)+"exit 0\n")
	buf.Reset()
	res, err = Run(root2, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	abSinEntrega(t, "CA-238", res, buf.String(), "test-writer")
	if _, ok := abHallazgoEn(t, id, root2); !ok {
		t.Fatalf("CA-238: el hallazgo creado en la cuarentena viaja al arbol real:\n%s", buf.String())
	}
	if res.Isolation == nil || res.Isolation.Kept || existeEn(t, res.Isolation.Dir) {
		t.Fatalf("CA-238: tras el trasplante no queda nada adentro: %+v", res.Isolation)
	}
	if abVeredictos(t, root2) != 0 {
		t.Fatal("CA-238: sin entrega no hay veredicto")
	}
}

// ---------------------------------------------------------------- CA-239

// CA-239: lo que no cambia. Los roles de solo lectura entregan hallazgos, no
// archivos; tocar solo una ruta prohibida es un cambio; y la manipulacion
// siempre gana.
func TestCA239_LoQueNoCambia(t *testing.T) {
	// los roles de solo lectura que no tocan nada cierran entregable, con veredicto
	for _, rol := range []string{"scout", "reviewer", "refutador", "orquestador"} {
		root := repo(t)
		fakeProvider(t, "claude", "exit 0\n")
		var buf bytes.Buffer
		res, err := Run(root, "main", Options{Role: rol, Prompt: "mira"}, &buf)
		if err != nil {
			t.Fatal(err)
		}
		if res.Stage != "ok" || res.Status != "entregable" || res.ExitCode != 0 || res.VerdictID == "" {
			t.Fatalf("CA-239: el %s sin tocar nada cierra entregable con veredicto, como hoy: %+v\n%s", rol, res, buf.String())
		}
		if strings.Contains(buf.String(), "SIN ENTREGA") {
			t.Fatalf("CA-239: un rol de solo lectura nunca es sin entrega:\n%s", buf.String())
		}
	}

	// un writer que solo toco una ruta prohibida que no es evidencia
	root := repo(t)
	fakeProvider(t, "claude", "mkdir -p .hoom/specs\nprintf '# intruso\\n' > .hoom/specs/intruso.md\nexit 0\n")
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "scope" || res.Status != "no-entregable" || res.ExitCode != 1 {
		t.Fatalf("CA-239: fuera de scope cierra no-entregable en scope: %+v\n%s", res, buf.String())
	}
	if res.VerdictID == "" || res.Check == nil {
		t.Fatalf("CA-239: hubo cambios: verify y check corren: %+v", res)
	}
	if len(res.Scope.Violations) != 1 || res.Scope.Violations[0].Rule != RuleOutOfScope {
		t.Fatalf("CA-239: la ruta del spec es fuera de scope para el writer: %+v", res.Scope)
	}

	// lo mismo para un autor de specs que solo escribio codigo
	root = repo(t)
	fakeProvider(t, "claude", "mkdir -p internal\nprintf 'package x\\n' > internal/x.go\nexit 0\n")
	buf.Reset()
	res, err = Run(root, "main", Options{Role: "arquitecto", Prompt: "escribi el spec"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Status != "no-entregable" || res.Stage != "scope" || res.VerdictID == "" || res.Check == nil {
		t.Fatalf("CA-239: el codigo del arquitecto es un cambio fuera de scope, no sin entrega: %+v\n%s", res, buf.String())
	}

	// manipulacion sin nada entregado: corta como manipulacion
	root = repo(t)
	write(t, root, ".hoom/verdicts/2026-01-01T00-00-00Z_viejo.json", "{\"verdict\":\"green\"}\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "veredicto previo")
	fakeProvider(t, "claude", "printf 'x' >> .hoom/verdicts/2026-01-01T00-00-00Z_viejo.json\nexit 0\n")
	buf.Reset()
	res, err = Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scope.Tampering || res.Stage != "scope" || res.Status != "no-entregable" || res.ExitCode != 1 {
		t.Fatalf("CA-239: sin-entrega nunca tapa una manipulacion: %+v\n%s", res, buf.String())
	}
	if res.VerdictID != "" || strings.Contains(buf.String(), "SIN ENTREGA") {
		t.Fatalf("CA-239: el corte es por manipulacion:\n%s", buf.String())
	}

	// aislamiento roto sin nada entregado en la cuarentena: corta como aislamiento
	root = repoCiego(t)
	fakeProvider(t, "claude", "printf 'package app // tocado\\n' > ../../../app.go\nexit 0\n")
	buf.Reset()
	res, err = Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scope.Broken() || res.Stage != "scope" || res.Status != "no-entregable" || res.ExitCode != 1 {
		t.Fatalf("CA-239: sin-entrega nunca tapa un aislamiento roto: %+v\n%s", res, buf.String())
	}
	if strings.Contains(buf.String(), "SIN ENTREGA") {
		t.Fatalf("CA-239: el corte es por aislamiento:\n%s", buf.String())
	}
}

// ---------------------------------------------------------------- CA-240

// CA-240: el registro del sobre cierra con sin-entrega; --json lleva el mismo
// status y el mismo exit.
func TestCA240_RegistroYJSONSinEntrega(t *testing.T) {
	if envelope.StatusNoDelivery != "sin-entrega" {
		t.Fatalf("CA-240: el estado es sin-entrega, es %q", envelope.StatusNoDelivery)
	}
	if !(envelope.Record{ID: "x", Status: envelope.StatusNoDelivery}).Done() {
		t.Fatal("CA-240: sin-entrega es un estado terminal: Done() verdadero")
	}

	root := repo(t)
	fakeProvider(t, "claude", "exit 0\n")
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	recs := sobres(t, root)
	if len(recs) != 1 {
		t.Fatalf("CA-240: un sobre, un registro: %+v", recs)
	}
	rec := recs[0]
	if rec.ID != res.EnvelopeID {
		t.Fatalf("CA-240: el Result nombra su registro: %q vs %q", res.EnvelopeID, rec.ID)
	}
	if rec.Status != envelope.StatusNoDelivery || rec.Stage != "scope" || rec.ExitCode != 1 {
		t.Fatalf("CA-240: el registro cierra scope/sin-entrega/1: %+v", rec)
	}
	if strings.TrimSpace(rec.Note) == "" || rec.EndedAt.IsZero() || !rec.Done() {
		t.Fatalf("CA-240: el registro trae nota, EndedAt y esta terminado: %+v", rec)
	}
	if rec.RunID != res.RunID || rec.VerdictID != "" {
		t.Fatalf("CA-240: hubo run y no hubo veredicto: %+v", rec)
	}

	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got["status"] != "sin-entrega" || got["stage"] != "scope" || got["exit_code"] != float64(1) {
		t.Fatalf("CA-240: --json lleva status sin-entrega y el mismo exit: %s", raw)
	}
	if _, ok := got["verdict_id"]; ok {
		t.Fatalf("CA-240: sin veredicto no hay verdict_id: %s", raw)
	}
}

// ---------------------------------------------------------------- CA-241

// CA-241: los pedidos de relleno y los que no lo son.
func TestCA241_IsPlaceholder(t *testing.T) {
	for _, p := range []string{
		"", "   ", "...", "…", ". . .", " \n...\t", "<pedido>", "<PEDIDO>", // los del criterio
		"<Pedido>", "\t\r\n", "………", ".…. …", "\v\f.",
	} {
		if !IsPlaceholder(p) {
			t.Fatalf("CA-241: %q es relleno", p)
		}
	}
	for _, p := range []string{
		"revisa ...", "?", ".gitignore", // los del criterio
		"pedido", "<pedido", "pedido>", "<pedido>x", "<pedido>.", "<>", "‥", "。", "-", "…?", "0", "a",
		"...a...", ". . . revisa",
	} {
		if IsPlaceholder(p) {
			t.Fatalf("CA-241: %q dice algo: no es relleno", p)
		}
	}
}

// CA-241: "todo espacio en blanco" es el de Unicode, no solo el ASCII.
func TestCA241_IsPlaceholderEspaciosUnicode(t *testing.T) {
	for _, p := range []string{"\u00a0...\u2003", "\u3000…\u00a0"} {
		if !IsPlaceholder(p) {
			t.Fatalf("CA-241: %q es relleno: solo espacio en blanco y puntos", p)
		}
	}
}

// CA-241: propiedad. Cualquier mezcla de espacio en blanco, '.' y '…' es
// relleno; agregarle UNA runa que no sea eso la vuelve un pedido.
func TestCA241_IsPlaceholderPropiedad(t *testing.T) {
	relleno := []rune{' ', '\t', '\n', '\r', '.', '…'}
	senal := []rune{'a', '?', '-', '<', '>', 'Z', 'ñ', '‥', '。', '0', '_', '/', '!'}
	armar := func(idx []uint8) []rune {
		out := make([]rune, 0, len(idx))
		for _, i := range idx {
			out = append(out, relleno[int(i)%len(relleno)])
		}
		return out
	}
	esRelleno := func(idx []uint8) bool { return IsPlaceholder(string(armar(idx))) }
	if err := quick.Check(esRelleno, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("CA-241: solo espacios y puntos siempre es relleno: %v", err)
	}
	diceAlgo := func(idx []uint8, pos, s uint8) bool {
		rs := armar(idx)
		k := int(pos) % (len(rs) + 1)
		rs = append(rs[:k], append([]rune{senal[int(s)%len(senal)]}, rs[k:]...)...)
		return !IsPlaceholder(string(rs))
	}
	if err := quick.Check(diceAlgo, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("CA-241: una runa con sentido vuelve el pedido real: %v", err)
	}
}

// CA-241: Run con un pedido de relleno devuelve error sin invocar al provider
// y sin dejar run ni registro; la validacion va antes de elegir provider.
func TestCA241_RunConPedidoDeRellenoNoGasta(t *testing.T) {
	for _, pedido := range []string{"", "   ", "...", "…", ". . .", " \n...\t", "<pedido>", "<PEDIDO>"} {
		root := repo(t)
		marca := filepath.Join(t.TempDir(), "invocado")
		fakeProvider(t, "claude", "touch "+marca+"\nexit 0\n")
		_, err := Run(root, "main", Options{Role: "writer", Prompt: pedido}, io.Discard)
		if err == nil {
			t.Fatalf("CA-241: el pedido %q es relleno y debe ser error", pedido)
		}
		if strings.TrimSpace(pedido) != "" {
			quiere := fmt.Sprintf("el pedido %q no dice nada: escribi lo que el rol tiene que hacer", pedido)
			quiereRecortado := fmt.Sprintf("el pedido %q no dice nada: escribi lo que el rol tiene que hacer", strings.TrimSpace(pedido))
			if !strings.Contains(err.Error(), quiere) && !strings.Contains(err.Error(), quiereRecortado) {
				t.Fatalf("CA-241: el mensaje debe ser %q: %v", quiere, err)
			}
		}
		if existeEn(t, marca) {
			t.Fatalf("CA-241: el provider no se invoca con el pedido %q", pedido)
		}
		if n := runsCount(t, root); n != 0 {
			t.Fatalf("CA-241: el pedido %q no deja run: %d archivos", pedido, n)
		}
		if recs := sobres(t, root); len(recs) != 0 {
			t.Fatalf("CA-241: el pedido %q no deja registro: %+v", pedido, recs)
		}
	}

	// antes de elegir provider: sin ningun CLI instalado, el error es el del pedido
	root := repo(t)
	t.Setenv("PATH", t.TempDir())
	_, err := Run(root, "main", Options{Role: "writer", Prompt: "…"}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "no dice nada") || strings.Contains(err.Error(), "system_prompt") {
		t.Fatalf("CA-241: la validacion del pedido va antes de elegir provider: %v", err)
	}
}

// CA-241: los pedidos que NO son relleno pasan y el run arranca.
func TestCA241_PedidosQueNoSonRellenoPasan(t *testing.T) {
	for _, pedido := range []string{"revisa ...", "?", ".gitignore"} {
		root := repo(t)
		fakeProvider(t, "claude", "exit 0\n")
		res, err := Run(root, "main", Options{Role: "scout", Prompt: pedido}, io.Discard)
		if err != nil {
			t.Fatalf("CA-241: %q dice algo y el sobre debe correr: %v", pedido, err)
		}
		if res.RunID == "" {
			t.Fatalf("CA-241: %q arranca el run: %+v", pedido, res)
		}
	}
}

// ---------------------------------------------------------------- CA-244

// CA-244: Gate gana la tarea; los hallazgos de sus violaciones la llevan, y
// sin tarea no llevan ninguna.
func TestCA244_GateAtaLaTareaASusHallazgos(t *testing.T) {
	root := repo(t)
	scout, _ := agents.Lookup("scout")
	pol := PolicyFor(nil, scout)

	res := Gate(root, "main", "cabina-visual", scout, snap(nil), snap(map[string]string{"nuevo.go": "h1"}), pol, nil)
	if len(res.Violations) != 1 || res.Violations[0].FindingID == "" {
		t.Fatalf("CA-244: la violacion deja hallazgo: %+v", res)
	}
	f, ok := abHallazgoEn(t, res.Violations[0].FindingID, root)
	if !ok || f.Task != "cabina-visual" {
		t.Fatalf("CA-244: el hallazgo del gate lleva la tarea: %+v", f)
	}

	// una manipulacion tambien
	existente := ".hoom/verdicts/2026-01-01T00-00-00Z_viejo.json"
	res = Gate(root, "main", "cabina-visual", scout,
		snap(map[string]string{existente: "h0"}, existente),
		snap(map[string]string{existente: "h1"}, existente), pol, nil)
	if len(res.Violations) != 1 || res.Violations[0].Rule != RuleTampering || res.Violations[0].FindingID == "" {
		t.Fatalf("CA-244: la manipulacion deja hallazgo: %+v", res)
	}
	if f, _ := abHallazgoEn(t, res.Violations[0].FindingID, root); f.Task != "cabina-visual" {
		t.Fatalf("CA-244: el hallazgo de la manipulacion lleva la tarea: %+v", f)
	}

	// sin tarea, sin etiqueta
	res = Gate(root, "main", "", scout, snap(nil), snap(map[string]string{"otro.go": "h1"}), pol, nil)
	if len(res.Violations) != 1 || res.Violations[0].FindingID == "" {
		t.Fatalf("CA-244: %+v", res)
	}
	id := res.Violations[0].FindingID
	if f, ok := abHallazgoEn(t, id, root); !ok || f.Task != "" {
		t.Fatalf("CA-244: sin tarea el hallazgo no lleva ninguna: %+v", f)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "findings", id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"task"`) {
		t.Fatalf("CA-244: sin tarea el artefacto no tiene la clave task: %s", raw)
	}
}

// CA-244 y CA-243: el sobre con --task. El proceso del provider recibe la
// tarea en HOOM_TASK y el hallazgo que el gate crea por una violacion la
// lleva; sin --task, ninguno de los dos hereda la tarea del humano.
func TestCA244_ElSobreConTareaAtaElHallazgoDelGate(t *testing.T) {
	t.Setenv("HOOM_TASK", "ajena")
	root := repo(t)
	if err := taskcmd.Start(root, "cabina-visual", "main"); err != nil {
		t.Fatal(err)
	}
	dir, err := runcmd.TaskDir(root, "cabina-visual")
	if err != nil {
		t.Fatal(err)
	}
	entorno := filepath.Join(t.TempDir(), "hoom_task.txt")
	fakeProvider(t, "claude", "printf '%s' \"$HOOM_TASK\" > "+entorno+"\nprintf 'package nuevo\\n' > nuevo.go\nexit 0\n")

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "scout", Task: "cabina-visual", Prompt: "explora"}, &buf)
	if err != nil {
		t.Fatalf("CA-244: %v\n%s", err, buf.String())
	}
	if len(res.Scope.Violations) != 1 || res.Scope.Violations[0].FindingID == "" {
		t.Fatalf("CA-244: el scout que escribe codigo deja violacion y hallazgo: %+v\n%s", res.Scope, buf.String())
	}
	f, ok := abHallazgoEn(t, res.Scope.Violations[0].FindingID, dir, root)
	if !ok {
		t.Fatalf("CA-244: el hallazgo %s no quedo registrado", res.Scope.Violations[0].FindingID)
	}
	if f.Task != "cabina-visual" {
		t.Fatalf("CA-244: con --task el hallazgo del gate lleva la tarea: %+v", f)
	}
	raw, err := os.ReadFile(entorno)
	if err != nil {
		t.Fatalf("CA-243: el provider falso debia dejar su HOOM_TASK: %v", err)
	}
	if string(raw) != "cabina-visual" {
		t.Fatalf("CA-243: el proceso del provider recibe HOOM_TASK de la tarea del run: %q", raw)
	}

	// sin --task
	root2 := repo(t)
	buf.Reset()
	res, err = Run(root2, "main", Options{Role: "scout", Prompt: "explora"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Scope.Violations) != 1 || res.Scope.Violations[0].FindingID == "" {
		t.Fatalf("CA-244: %+v\n%s", res.Scope, buf.String())
	}
	f, ok = abHallazgoEn(t, res.Scope.Violations[0].FindingID, root2)
	if !ok || f.Task != "" {
		t.Fatalf("CA-244: sin --task el hallazgo del gate no lleva ninguna: %+v", f)
	}
	raw, _ = os.ReadFile(entorno)
	if string(raw) == "ajena" {
		t.Fatal("CA-243: un run sin tarea no hereda la del proceso padre")
	}
}
