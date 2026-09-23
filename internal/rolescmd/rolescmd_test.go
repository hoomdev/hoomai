// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-295..CA-298): la matriz de enforcement. Se DERIVA de la tabla de roles,
// de PolicyFor y de las Capabilities de cada provider, y lo que dice sobre un
// mecanismo tiene que coincidir con el argv real del adapter.
package rolescmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
)

func rcMatriz(t *testing.T, m *manifest.Manifest) []Row {
	t.Helper()
	rows := Matrix(m, agents.Roles(), providers.All())
	if len(rows) != len(agents.Roles())*len(providers.All()) {
		t.Fatalf("CA-295: una fila por rol x provider (%d x %d), hay %d",
			len(agents.Roles()), len(providers.All()), len(rows))
	}
	return rows
}

func rcFila(t *testing.T, rows []Row, rol, prov string) Row {
	t.Helper()
	for _, r := range rows {
		if r.Role == rol && r.Provider == prov {
			return r
		}
	}
	t.Fatalf("CA-295: falta la fila %s x %s", rol, prov)
	return Row{}
}

func rcCategoria(c string) bool {
	for _, k := range Categories {
		if c == k {
			return true
		}
	}
	return false
}

// CA-295: 10 x 4 filas, en el orden de agents.Roles() y providers.All(), y
// ninguna celda sin clasificar: categoria valida, al menos un mecanismo y gap
// vacio solo en ENFORCED.
func TestCA295_CadaCombinacionClasificada(t *testing.T) {
	if !reflect.DeepEqual(Categories, []string{"ENFORCED", "POST-VERIFIED", "BEST-EFFORT", "UNSUPPORTED"}) {
		t.Fatalf("CA-295: las cuatro categorias, en orden de leyenda: %v", Categories)
	}
	roles, provs := agents.Roles(), providers.All()
	if len(roles) != 10 || len(provs) != 4 {
		t.Fatalf("CA-295: hoy son 10 roles y 4 providers: %d x %d", len(roles), len(provs))
	}
	rows := rcMatriz(t, nil)
	for i, r := range roles {
		for j, p := range provs {
			row := rows[i*len(provs)+j]
			if row.Role != r.Slug || row.Provider != p.Name() {
				t.Fatalf("CA-295: la fila %d es %s x %s, fue %s x %s", i*len(provs)+j, r.Slug, p.Name(), row.Role, row.Provider)
			}
			for dim, c := range map[string]Cell{"read": row.Read, "write": row.Write, "exec": row.Exec} {
				if !rcCategoria(c.Category) {
					t.Fatalf("CA-295: %s x %s %s sin clasificar: %+v", r.Slug, p.Name(), dim, c)
				}
				if len(c.Mechanisms) == 0 {
					t.Fatalf("CA-295: %s x %s %s sin mecanismo: %+v", r.Slug, p.Name(), dim, c)
				}
				for _, m := range c.Mechanisms {
					if strings.TrimSpace(m) == "" {
						t.Fatalf("CA-295: %s x %s %s con un mecanismo vacio: %+v", r.Slug, p.Name(), dim, c)
					}
				}
				if (c.Category == Enforced) != (strings.TrimSpace(c.Gap) == "") {
					t.Fatalf("CA-295: %s x %s %s: gap vacio solo en ENFORCED: %+v", r.Slug, p.Name(), dim, c)
				}
				if c.Category != Unsupported && strings.TrimSpace(c.Allows) == "" {
					t.Fatalf("CA-295: %s x %s %s dice que puede hacer (allows vacio): %+v", r.Slug, p.Name(), dim, c)
				}
			}
		}
	}
}

// CA-296: la tabla del contrato para claude y codex; opencode y gemini
// UNSUPPORTED en las tres celdas.
func TestCA296_TablaDelContrato(t *testing.T) {
	E, P, B := Enforced, PostVerified, BestEffort
	type fila struct{ read, wClaude, wCodex, xClaude, xCodex string }
	quiere := map[string]fila{
		"orquestador":   {E, P, P, E, E},
		"reviewer":      {E, P, P, E, E},
		"refutador":     {E, P, P, E, E},
		"scout":         {E, E, E, E, B},
		"arquitecto":    {E, P, P, E, B},
		"designer":      {E, P, P, E, B},
		"analista":      {E, P, P, E, B},
		"writer":        {E, P, P, E, E},
		"characterizer": {E, P, P, E, E},
		"test-writer":   {B, P, P, E, E},
	}
	rows := rcMatriz(t, nil)
	for rol, f := range quiere {
		cl, cx := rcFila(t, rows, rol, "claude"), rcFila(t, rows, rol, "codex")
		got := fila{cl.Read.Category, cl.Write.Category, cx.Write.Category, cl.Exec.Category, cx.Exec.Category}
		if got != f {
			t.Fatalf("CA-296: %s: read/write claude/write codex/exec claude/exec codex = %+v, esperaba %+v", rol, got, f)
		}
		if cx.Read.Category != f.read {
			t.Fatalf("CA-296: %s read en codex es %s, esperaba %s", rol, cx.Read.Category, f.read)
		}
		for _, prov := range []string{"opencode", "gemini"} {
			r := rcFila(t, rows, rol, prov)
			for dim, c := range map[string]Cell{"read": r.Read, "write": r.Write, "exec": r.Exec} {
				if c.Category != Unsupported {
					t.Fatalf("CA-296: %s x %s %s es UNSUPPORTED (sin system_prompt), fue %+v", rol, prov, dim, c)
				}
				if !reflect.DeepEqual(c.Mechanisms, []string{MechNoSystemPrompt}) && !contiene(c.Mechanisms, MechNoSystemPrompt) {
					t.Fatalf("CA-296: %s x %s %s dice por que: %s: %+v", rol, prov, dim, MechNoSystemPrompt, c)
				}
			}
		}
	}
	// la lectura del test-writer: BEST-EFFORT, arbol ciego, y el gap nombra git show
	for _, prov := range []string{"claude", "codex"} {
		c := rcFila(t, rows, "test-writer", prov).Read
		if c.Category != BestEffort || !strings.Contains(c.Gap, "git show") {
			t.Fatalf("CA-296: la lectura del test-writer en %s es BEST-EFFORT con 'git show' en gap: %+v", prov, c)
		}
		if c.Allows != "el spec y los tests, sin la implementacion en disco" ||
			!reflect.DeepEqual(c.Mechanisms, []string{MechBlind, MechGateIsolation}) {
			t.Fatalf("CA-296: allows y mecanismos de la lectura ciega en %s: %+v", prov, c)
		}
	}
}

func contiene(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// CA-296: los mecanismos y allows exactos que fijan las reglas del contrato.
func TestCA296_MecanismosPorRegla(t *testing.T) {
	rows := rcMatriz(t, nil)
	type celda struct {
		allows string
		mecs   []string
	}
	chequear := func(rol, prov, dim string, c Cell, w celda) {
		t.Helper()
		if w.allows != "" && c.Allows != w.allows {
			t.Fatalf("CA-296: %s x %s %s allows %q, fue %q", rol, prov, dim, w.allows, c.Allows)
		}
		if !reflect.DeepEqual(c.Mechanisms, w.mecs) {
			t.Fatalf("CA-296: %s x %s %s mecanismos %q, fue %q", rol, prov, dim, w.mecs, c.Mechanisms)
		}
	}
	for _, r := range agents.Roles() {
		for _, prov := range []string{"claude", "codex"} {
			row := rcFila(t, rows, r.Slug, prov)
			// read
			if !r.Isolated {
				chequear(r.Slug, prov, "read", row.Read, celda{"todo el arbol", []string{MechNone}})
			}
			// exec
			switch {
			case r.Exec:
				chequear(r.Slug, prov, "exec", row.Exec, celda{"comandos", []string{MechNone}})
			case prov == "claude":
				chequear(r.Slug, prov, "exec", row.Exec, celda{"ningun comando", []string{MechToolDeny}})
			default:
				chequear(r.Slug, prov, "exec", row.Exec, celda{"ningun comando (lo pide el contrato)", []string{MechContract}})
			}
			// write
			var w celda
			switch {
			case r.ReadOnly && !r.Exec && prov == "claude":
				w = celda{"nada", []string{MechToolDeny, MechGate}}
			case r.ReadOnly && !r.Exec:
				w = celda{"nada", []string{MechSandbox, MechGate}}
			case r.ReadOnly && prov == "claude":
				w = celda{"", []string{MechToolDenyShell, MechGate}}
			case r.ReadOnly:
				w = celda{"", []string{MechGate}}
			case r.Isolated:
				w = celda{"", []string{MechQuarantine, MechGate}}
			default:
				w = celda{"", []string{MechGate}}
			}
			chequear(r.Slug, prov, "write", row.Write, w)
		}
	}
}

// CA-297: donde la matriz dice 'deny de tools' o 'sandbox', el argv real
// del adapter lo trae, armado como lo arma el sobre (ReadOnlyFor,
// NoExecFor).
func TestCA297_MecanismoCoincideConElArgv(t *testing.T) {
	rows := rcMatriz(t, nil)
	vistosDeny, vistosSandbox := 0, 0
	for _, row := range rows {
		if row.Provider != "claude" && row.Provider != "codex" {
			continue
		}
		p, err := providers.Lookup(row.Provider)
		if err != nil {
			t.Fatal(err)
		}
		role, err := agents.Lookup(row.Role)
		if err != nil {
			t.Fatal(err)
		}
		readOnly, exec, _ := agentcmd.ReadOnlyFor(p, role)
		noExec, _ := agentcmd.NoExecFor(p, role)
		inv, err := p.Command(providers.Request{Prompt: "x", ReadOnly: readOnly, Exec: exec, NoExec: noExec, Unattended: true})
		if err != nil {
			t.Fatalf("CA-297: %s x %s: Command: %v", row.Role, row.Provider, err)
		}
		for dim, c := range map[string]Cell{"read": row.Read, "write": row.Write, "exec": row.Exec} {
			for _, m := range c.Mechanisms {
				switch {
				case strings.HasPrefix(m, MechToolDeny):
					vistosDeny++
					deny := rcValor(inv.Args, "--disallowedTools")
					if deny == "" {
						t.Fatalf("CA-297: %s x %s %s dice %q y el argv no trae --disallowedTools: %q", row.Role, row.Provider, dim, m, inv.Args)
					}
					tieneBash := contiene(strings.Split(deny, ","), "Bash")
					if !role.Exec && !tieneBash {
						t.Fatalf("CA-297: %s no ejecuta: --disallowedTools incluye Bash: %q", row.Role, deny)
					}
					if role.Exec && tieneBash {
						t.Fatalf("CA-297: %s ejecuta: --disallowedTools no le niega Bash: %q", row.Role, deny)
					}
				case m == MechSandbox:
					vistosSandbox++
					want := `sandbox_mode="read-only"`
					if role.Exec {
						want = `sandbox_mode="workspace-write"`
					}
					if !contiene(inv.Args, want) {
						t.Fatalf("CA-297: %s x %s %s dice sandbox y el argv no trae %s: %q", row.Role, row.Provider, dim, want, inv.Args)
					}
				}
			}
		}
	}
	if vistosDeny == 0 || vistosSandbox == 0 {
		t.Fatalf("CA-297: la matriz declara 'deny de tools' (%d) y 'sandbox' (%d) en algun lado: el test no puede ser vacuo", vistosDeny, vistosSandbox)
	}
}

func rcValor(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, flag+"=") {
			return strings.TrimPrefix(a, flag+"=")
		}
	}
	return ""
}

// CA-298: la celda write del writer muestra los globs de PolicyFor, tambien
// los que declara hoom.yaml (manifest.Load), mas el piso.
func TestCA298_AllowsSaleDePolicyFor(t *testing.T) {
	rows := rcMatriz(t, nil)
	for _, r := range agents.Roles() {
		if r.ReadOnly && !r.Exec {
			continue // "nada": lo impone el provider
		}
		pol := agentcmd.PolicyFor(nil, r)
		for _, prov := range []string{"claude", "codex"} {
			c := rcFila(t, rows, r.Slug, prov).Write
			for _, g := range append(append([]string{}, pol.Allow...), pol.Deny...) {
				if !strings.Contains(c.Allows, g) {
					t.Fatalf("CA-298: write de %s x %s muestra el glob %q de PolicyFor: %q", r.Slug, prov, g, c.Allows)
				}
			}
			if len(pol.Deny) > 0 && !strings.Contains(c.Allows, "menos") {
				t.Fatalf("CA-298: los deny de %s van con 'menos': %q", r.Slug, c.Allows)
			}
			if !strings.Contains(c.Allows, "nunca el piso") {
				t.Fatalf("CA-298: write de %s x %s dice que el piso nunca se escribe: %q", r.Slug, prov, c.Allows)
			}
		}
	}

	dir := t.TempDir()
	yml := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n" +
		"agents:\n  writer:\n    write:\n      allow: [\"src/**\"]\n"
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(yml), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		t.Fatalf("CA-298: hoom.yaml de prueba: %v", err)
	}
	rows = rcMatriz(t, m)
	for _, prov := range []string{"claude", "codex"} {
		c := rcFila(t, rows, "writer", prov).Write
		if !strings.Contains(c.Allows, "src/**") {
			t.Fatalf("CA-298: el agents.writer.write.allow de hoom.yaml aparece en allows (%s): %q", prov, c.Allows)
		}
	}
	if c := rcFila(t, rows, "characterizer", "claude").Write; strings.Contains(c.Allows, "src/**") {
		t.Fatalf("CA-298: el override del writer no se filtra a otro rol: %q", c.Allows)
	}
}

// CA-298: --role/--provider filtran; un valor desconocido es *UsageError con
// los validos; el parseo es estricto.
func TestCA298_SelectYParseArgs(t *testing.T) {
	roles, provs, err := Select(Options{})
	if err != nil || len(roles) != 10 || len(provs) != 4 {
		t.Fatalf("CA-298: sin filtros, todos: %d roles %d providers %v", len(roles), len(provs), err)
	}
	roles, provs, err = Select(Options{Role: "writer"})
	if err != nil || len(roles) != 1 || roles[0].Slug != "writer" || len(provs) != 4 {
		t.Fatalf("CA-298: --role writer filtra el rol: %v %v %v", roles, provs, err)
	}
	roles, provs, err = Select(Options{Provider: "codex"})
	if err != nil || len(roles) != 10 || len(provs) != 1 || provs[0].Name() != "codex" {
		t.Fatalf("CA-298: --provider codex filtra el provider: %v %v", len(roles), err)
	}
	roles, provs, err = Select(Options{Role: "test-writer", Provider: "claude"})
	if err != nil || len(roles) != 1 || len(provs) != 1 {
		t.Fatalf("CA-298: los dos filtros juntos: %v %v %v", roles, provs, err)
	}
	for _, o := range []struct {
		opt    Options
		nombra []string
	}{
		{Options{Role: "bogus"}, []string{"bogus", "writer", "test-writer", "refutador"}},
		{Options{Provider: "bogus"}, []string{"bogus", "claude", "codex", "opencode", "gemini"}},
	} {
		_, _, err := Select(o.opt)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || ue.ExitCode() != 2 {
			t.Fatalf("CA-298: %+v desconocido es *cliargs.UsageError: %T %v", o.opt, err, err)
		}
		for _, s := range o.nombra {
			if !strings.Contains(ue.Error(), s) {
				t.Fatalf("CA-298: el rechazo de %+v nombra %q (los validos): %v", o.opt, s, ue)
			}
		}
	}

	opt, err := ParseArgs([]string{"--role", "writer", "--provider", "claude", "--json"})
	if err != nil || opt.Role != "writer" || opt.Provider != "claude" || !opt.JSON {
		t.Fatalf("CA-298: ParseArgs: %+v %v", opt, err)
	}
	for _, args := range [][]string{{"x"}, {"--role"}, {"--role", ""}, {"--provider="}, {"--bogus"}, {"--json", "writer"}} {
		_, err := ParseArgs(args)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || ue.ExitCode() != 2 {
			t.Fatalf("CA-298: 'hoom roles %s' es *cliargs.UsageError: %T %v", strings.Join(args, " "), err, err)
		}
	}
	if _, err := ParseArgs([]string{"-h"}); !errors.Is(err, cliargs.ErrHelp) {
		t.Fatalf("CA-298: -h es ErrHelp: %v", err)
	}
}

// CA-298: el texto: un bloque por rol x provider con lee/escribe/ejecuta, y
// al final la leyenda de una linea por categoria.
func TestCA298_RenderConLeyenda(t *testing.T) {
	rows := Matrix(nil, []agents.Role{mustRole(t, "writer"), mustRole(t, "test-writer")},
		[]providers.Provider{mustProv(t, "claude"), mustProv(t, "codex")})
	var buf bytes.Buffer
	Render(&buf, rows)
	out := buf.String()
	for _, want := range []string{"writer", "test-writer", "claude", "codex", "lee", "escribe", "ejecuta",
		Enforced, PostVerified, BestEffort, MechGate, MechBlind} {
		if !strings.Contains(out, want) {
			t.Fatalf("CA-298: al texto le falta %q:\n%s", want, out)
		}
	}
	var lineas []string
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) != "" {
			lineas = append(lineas, l)
		}
	}
	if len(lineas) < 4 {
		t.Fatalf("CA-298: el texto termina con la leyenda:\n%s", out)
	}
	cola := lineas[len(lineas)-4:]
	for i, cat := range Categories {
		if !strings.Contains(cola[i], cat) {
			t.Fatalf("CA-298: la leyenda cierra el texto, una linea por categoria en orden (%s):\n%s", cat, strings.Join(cola, "\n"))
		}
	}
}

func mustRole(t *testing.T, slug string) agents.Role {
	t.Helper()
	r, err := agents.Lookup(slug)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func mustProv(t *testing.T, name string) providers.Provider {
	t.Helper()
	p, err := providers.Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}
