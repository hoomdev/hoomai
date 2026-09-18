// Tests adversariales del spec .hoom/specs/arquitecto-bajo-el-sobre.md
// (CA-228, CA-229, CA-233): los autores de specs escriben, y solo specs. La
// tabla dice quien escribe, donde y si ejecuta; los contratos y los
// subagentes nativos salen de ese mismo dato y no pueden decir otra cosa.
package agents

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var abAutores = []string{"arquitecto", "designer", "analista"}

// CA-228: la tabla completa, rol por rol. En negrita del spec: arquitecto,
// designer y analista pasan a escribir sin shell en la forma specs; writer,
// test-writer y characterizer declaran el shell que siempre usaron.
func TestCA228_TablaDeRolesConExecHonesto(t *testing.T) {
	if ScopeSpecs != "specs" {
		t.Fatalf("CA-228: ScopeSpecs debe ser \"specs\", es %q", ScopeSpecs)
	}
	type fila struct {
		readOnly, exec bool
		scope          string
	}
	quiere := map[string]fila{
		"orquestador":   {true, true, ScopeEvidencia},
		"arquitecto":    {false, false, ScopeSpecs},
		"designer":      {false, false, ScopeSpecs},
		"scout":         {true, false, ScopeEvidencia},
		"writer":        {false, true, ScopeCodigo},
		"test-writer":   {false, true, ScopeTests},
		"reviewer":      {true, true, ScopeEvidencia},
		"characterizer": {false, true, ScopeTests},
		"analista":      {false, false, ScopeSpecs},
		"refutador":     {true, true, ScopeEvidencia},
	}
	all := Roles()
	if len(all) != len(quiere) {
		t.Fatalf("CA-228: la tabla debe seguir teniendo %d roles, tiene %d", len(quiere), len(all))
	}
	vistos := map[string]bool{}
	for _, r := range all {
		w, ok := quiere[r.Slug]
		if !ok {
			t.Fatalf("CA-228: rol inesperado %q", r.Slug)
		}
		vistos[r.Slug] = true
		if r.ReadOnly != w.readOnly || r.Exec != w.exec || r.Scope != w.scope {
			t.Fatalf("CA-228: %s deberia ser ReadOnly=%v Exec=%v Scope=%q, es ReadOnly=%v Exec=%v Scope=%q",
				r.Slug, w.readOnly, w.exec, w.scope, r.ReadOnly, r.Exec, r.Scope)
		}
	}
	if len(vistos) != len(quiere) {
		t.Fatalf("CA-228: faltan roles en la tabla: %v", vistos)
	}

	// lo que no cambia: solo el orquestador es primario y solo el test-writer
	// corre ciego
	for _, r := range all {
		if r.Primary != (r.Slug == "orquestador") {
			t.Fatalf("CA-228: Primary se movio en %s: %+v", r.Slug, r)
		}
		if r.Isolated != (r.Slug == "test-writer") {
			t.Fatalf("CA-228: Isolated se movio en %s: %+v", r.Slug, r)
		}
	}

	// Lookup por nombre nativo devuelve la misma fila
	for _, slug := range abAutores {
		r, err := Lookup("hoom-" + slug)
		if err != nil {
			t.Fatal(err)
		}
		if r.ReadOnly || r.Exec || r.Scope != ScopeSpecs {
			t.Fatalf("CA-228: Lookup(hoom-%s) debe traer la fila nueva: %+v", slug, r)
		}
	}

	// el JSON de la tabla sigue nombrando exec
	raw, err := json.Marshal(all[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"exec":false`) || !strings.Contains(string(raw), `"scope":"specs"`) {
		t.Fatalf("CA-228: el JSON del arquitecto debe decir exec y scope: %s", raw)
	}
}

// CA-228: Exec significa lo mismo en toda la tabla. Propiedad sobre la tabla
// entera: un rol que escribe sin ejecutar es exactamente un autor de specs, y
// todo otro rol que escribe declara su shell.
func TestCA228_ExecSignificaLoMismoEnTodaLaTabla(t *testing.T) {
	for _, r := range Roles() {
		escribeSinShell := !r.ReadOnly && !r.Exec
		if escribeSinShell != (r.Scope == ScopeSpecs) {
			t.Fatalf("CA-228: escribir sin shell y la forma specs van juntos: %+v", r)
		}
		if r.Scope == ScopeSpecs && r.ReadOnly {
			t.Fatalf("CA-228: un autor de specs no puede ser de solo lectura: %+v", r)
		}
		if !r.ReadOnly && r.Scope != ScopeSpecs && !r.Exec {
			t.Fatalf("CA-228: un rol que escribe y no es autor de specs usa shell y lo declara: %+v", r)
		}
	}
	// mutar la copia no puede tocar la tabla
	all := Roles()
	for i := range all {
		all[i].Exec = true
		all[i].Scope = "roto"
	}
	if a, _ := Lookup("arquitecto"); a.Exec || a.Scope != ScopeSpecs {
		t.Fatalf("CA-228: Roles() debe devolver una copia: %+v", a)
	}
}

// abSoloEnSpecs: el contrato ata la escritura a .hoom/specs/ con un "solo" (o
// "unicamente") cerca, aunque el markdown parta la frase en dos lineas.
var abSoloEnSpecs = regexp.MustCompile(`(?i)(solo|unicamente|únicamente).{0,80}\.hoom/specs/`)

// abNoEjecuta: el contrato dice que el rol no ejecuta comandos.
var abNoEjecuta = regexp.MustCompile(`(?i)((no|nunca|jamas|jamás)\s+(\S+\s+)?ejecut|sin\s+ejecutar)`)

func abColapsar(s string) string { return strings.Join(strings.Fields(s), " ") }

// CA-229: los contratos embebidos de los autores dicen donde escriben (solo
// .hoom/specs/) y que no ejecutan comandos; ni ellos ni la Desc de arquitecto
// y designer siguen diciendo "Solo lectura".
func TestCA229_ContratosDeLosAutoresDeSpecs(t *testing.T) {
	for _, slug := range abAutores {
		r, err := Lookup(slug)
		if err != nil {
			t.Fatal(err)
		}
		// directorio sin .hoom/agents/: Contract cae al embebido
		c, err := Contract(t.TempDir(), r)
		if err != nil {
			t.Fatalf("CA-229: el contrato embebido de %s debe cargar: %v", slug, err)
		}
		plano := abColapsar(c)
		if !strings.Contains(c, ".hoom/specs/") {
			t.Fatalf("CA-229: el contrato de %s debe nombrar .hoom/specs/", slug)
		}
		if !abSoloEnSpecs.MatchString(plano) {
			t.Fatalf("CA-229: el contrato de %s debe decir que escribe SOLO en .hoom/specs/", slug)
		}
		if !abNoEjecuta.MatchString(plano) || !strings.Contains(strings.ToLower(plano), "comando") {
			t.Fatalf("CA-229: el contrato de %s debe decir que no ejecuta comandos", slug)
		}
		if strings.Contains(c, "Solo lectura") {
			t.Fatalf("CA-229: el contrato de %s sigue diciendo \"Solo lectura\"", slug)
		}
	}
	for _, slug := range []string{"arquitecto", "designer"} {
		r, _ := Lookup(slug)
		if strings.Contains(r.Desc, "Solo lectura") {
			t.Fatalf("CA-229: la Desc de %s sigue diciendo \"Solo lectura\": %q", slug, r.Desc)
		}
		if strings.TrimSpace(r.Desc) == "" {
			t.Fatalf("CA-229: la Desc de %s no puede quedar vacia", slug)
		}
	}
}

// abFrontmatter devuelve las lineas entre los dos '---' del archivo generado.
func abFrontmatter(t *testing.T, path string) []string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("CA-233: falta el subagente generado %s: %v", path, err)
	}
	lines := strings.Split(string(raw), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		t.Fatalf("CA-233: %s no empieza con frontmatter", path)
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return lines[1:i]
		}
	}
	t.Fatalf("CA-233: %s tiene el frontmatter sin cerrar", path)
	return nil
}

func abLineaCon(lines []string, prefijo string) (string, bool) {
	for _, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), prefijo) {
			return strings.TrimSpace(l), true
		}
	}
	return "", false
}

// abHerramientasGemini lee la lista YAML "tools:" del frontmatter de Gemini.
func abHerramientasGemini(lines []string) []string {
	var out []string
	dentro := false
	for _, l := range lines {
		tl := strings.TrimSpace(l)
		if strings.HasPrefix(tl, "tools:") {
			dentro = true
			continue
		}
		if !dentro {
			continue
		}
		if !strings.HasPrefix(tl, "- ") {
			break
		}
		out = append(out, strings.TrimSpace(strings.TrimPrefix(tl, "- ")))
	}
	return out
}

func abTiene(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// CA-233: los subagentes nativos salen de la misma tabla: los autores de
// specs escriben y no tienen shell en Claude, OpenCode y Gemini; los que
// escriben con shell salen como hoy.
func TestCA233_SubagentesNativosDeLosAutores(t *testing.T) {
	dir := t.TempDir()
	if err := Install(dir); err != nil {
		t.Fatal(err)
	}
	if err := GenerateTargets(dir, []string{"claude", "opencode", "gemini"}); err != nil {
		t.Fatalf("CA-233: generar los subagentes no debe fallar: %v", err)
	}

	for _, slug := range abAutores {
		// Claude: escritura por nombre, sin Bash
		fm := abFrontmatter(t, filepath.Join(dir, ".claude", "agents", "hoom-"+slug+".md"))
		tools, ok := abLineaCon(fm, "tools:")
		if !ok || tools != "tools: Read, Grep, Glob, Edit, Write, MultiEdit" {
			t.Fatalf("CA-233: claude/%s debe tener 'tools: Read, Grep, Glob, Edit, Write, MultiEdit', tiene %q", slug, tools)
		}
		if strings.Contains(strings.Join(fm, "\n"), "Bash") {
			t.Fatalf("CA-233: claude/%s no puede tener Bash: %v", slug, fm)
		}

		// OpenCode: edit allow, bash deny
		fm = abFrontmatter(t, filepath.Join(dir, ".opencode", "agents", "hoom-"+slug+".md"))
		if l, _ := abLineaCon(fm, "edit:"); l != "edit: allow" {
			t.Fatalf("CA-233: opencode/%s debe tener 'edit: allow', tiene %q", slug, l)
		}
		if l, _ := abLineaCon(fm, "bash:"); l != "bash: deny" {
			t.Fatalf("CA-233: opencode/%s debe tener 'bash: deny', tiene %q", slug, l)
		}

		// Gemini: write_file y replace, sin shell ni comodin
		fm = abFrontmatter(t, filepath.Join(dir, ".gemini", "agents", "hoom-"+slug+".md"))
		gt := abHerramientasGemini(fm)
		for _, want := range []string{"write_file", "replace"} {
			if !abTiene(gt, want) {
				t.Fatalf("CA-233: gemini/%s debe tener %s: %v", slug, want, gt)
			}
		}
		for _, prohibida := range []string{"run_shell_command", `"*"`, "*", `'*'`} {
			if abTiene(gt, prohibida) {
				t.Fatalf("CA-233: gemini/%s no puede tener %s: %v", slug, prohibida, gt)
			}
		}
	}

	// los que escriben con shell salen como hoy
	for _, slug := range []string{"writer", "test-writer", "characterizer"} {
		fm := abFrontmatter(t, filepath.Join(dir, ".claude", "agents", "hoom-"+slug+".md"))
		if l, ok := abLineaCon(fm, "tools:"); ok {
			t.Fatalf("CA-233: claude/%s sale sin lista de herramientas (todas), como hoy: %q", slug, l)
		}
		fm = abFrontmatter(t, filepath.Join(dir, ".opencode", "agents", "hoom-"+slug+".md"))
		if l, _ := abLineaCon(fm, "edit:"); l != "edit: allow" {
			t.Fatalf("CA-233: opencode/%s conserva 'edit: allow': %q", slug, l)
		}
		if l, _ := abLineaCon(fm, "bash:"); l != "bash: allow" {
			t.Fatalf("CA-233: opencode/%s conserva 'bash: allow': %q", slug, l)
		}
		fm = abFrontmatter(t, filepath.Join(dir, ".gemini", "agents", "hoom-"+slug+".md"))
		if gt := abHerramientasGemini(fm); len(gt) != 1 || gt[0] != `"*"` {
			t.Fatalf("CA-233: gemini/%s conserva el comodin \"*\": %v", slug, gt)
		}
	}

	// y los de solo lectura no se movieron
	fm := abFrontmatter(t, filepath.Join(dir, ".claude", "agents", "hoom-scout.md"))
	if l, _ := abLineaCon(fm, "tools:"); l != "tools: Read, Grep, Glob" {
		t.Fatalf("CA-233: el scout sigue sin escritura ni shell: %q", l)
	}
	fm = abFrontmatter(t, filepath.Join(dir, ".claude", "agents", "hoom-reviewer.md"))
	if l, _ := abLineaCon(fm, "tools:"); l != "tools: Read, Grep, Glob, Bash" {
		t.Fatalf("CA-233: el reviewer sigue leyendo con shell: %q", l)
	}
}
