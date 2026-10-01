// Tests adversariales del spec .hoom/specs/quien-cierra-un-hallazgo.md,
// ronda 1 de su review (CA-438, "por la CLI o a mano"). Dos caminos por los
// que una resolucion creada en la corrida de un rol que no es refutador
// podia escapar del gate de territorio:
//
//  1. Una resolucion que Git no lista sigue contando. El gate mira el arbol,
//     no lo que Git decide mostrar: un .hoom/findings/<id>.res.json creado
//     durante la corrida es manipulacion aunque este ignorado — por
//     .git/info/exclude, por el .gitignore de la raiz (que un writer puede
//     tocar), por un .hoom/findings/.gitignore, o por una regla que ya
//     estaba antes de la corrida. verify si lo lee del disco: si el gate no
//     lo ve, un hallazgo alto queda cerrado y el sobre termina verde.
//  2. Una corrida que falla tambien se mide. Si el provider cierra un
//     hallazgo y despues sale con error, el gate corre igual: la
//     manipulacion queda en el resultado y su hallazgo alto queda
//     registrado, asi la resolucion no deja verde a la corrida siguiente.
//
// Fotos reales (Take sobre un repo real) y sobres reales con un claude
// falso; los que corren la CLI compilan hoom (se omiten con -short). Los
// sobres corren con reloj.
package agentcmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// qcr1Patron es la regla con la que un rol esconde las resoluciones de Git.
const qcr1Patron = ".hoom/findings/*.res.json"

// qcr1Reloj es cuanto puede tardar un sobre con un provider falso.
const qcr1Reloj = 2 * time.Minute

// qcr1Exclude devuelve la ruta absoluta del .git/info/exclude de root.
func qcr1Exclude(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--git-path", "info/exclude")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse --git-path info/exclude: %v", err)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p
}

// qcr1Anexar agrega una linea al final de un archivo (lo crea si falta).
func qcr1Anexar(t *testing.T, path, linea string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + linea + "\n"); err != nil {
		t.Fatal(err)
	}
}

// qcr1Ignorado dice si Git ignora rel en root (git check-ignore).
func qcr1Ignorado(t *testing.T, root, rel string) bool {
	t.Helper()
	cmd := exec.Command("git", "check-ignore", "-q", "--no-index", rel)
	cmd.Dir = root
	return cmd.Run() == nil
}

// qcr1Escondite es una forma de que Git no liste la resolucion.
type qcr1Escondite struct {
	caso   string
	previo bool // la regla ya estaba antes de la corrida
	armar  func(t *testing.T, root string)
}

func qcr1Escondites() []qcr1Escondite {
	return []qcr1Escondite{
		{"info/exclude con " + qcr1Patron, false, func(t *testing.T, root string) {
			qcr1Anexar(t, qcr1Exclude(t, root), qcr1Patron)
		}},
		{"info/exclude con *.res.json", false, func(t *testing.T, root string) {
			qcr1Anexar(t, qcr1Exclude(t, root), "*.res.json")
		}},
		{".gitignore de la raiz", false, func(t *testing.T, root string) {
			qcr1Anexar(t, filepath.Join(root, ".gitignore"), qcr1Patron)
		}},
		{".hoom/findings/.gitignore", false, func(t *testing.T, root string) {
			write(t, root, ".hoom/findings/.gitignore", "*.res.json\n")
		}},
		{"info/exclude con .hoom/findings/ entero", false, func(t *testing.T, root string) {
			qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
		}},
		{"regla de info/exclude de antes de la corrida", true, func(t *testing.T, root string) {
			qcr1Anexar(t, qcr1Exclude(t, root), qcr1Patron)
		}},
	}
}

// qcr1HallazgoAlto exige que la violacion en ruta traiga el hallazgo que el
// gate registro, y que ese hallazgo este abierto y sea high. Devuelve su id.
func qcr1HallazgoAlto(t *testing.T, caso, root string, res ScopeResult, ruta string) string {
	t.Helper()
	id, err := qcr1HallazgoDelGate(root, res, ruta)
	if err != "" {
		t.Fatalf("CA-438: %s: %s", caso, err)
	}
	return id
}

// qcr1HallazgoDelGate es qcr1HallazgoAlto sin cortar el test: devuelve el
// id del hallazgo, o por que no esta.
func qcr1HallazgoDelGate(root string, res ScopeResult, ruta string) (string, string) {
	v, ok := qcViolacion(res.Violations, ruta)
	if !ok || v.Rule != RuleTampering {
		return "", "no hay violacion de manipulacion en " + ruta + ": " + qcr1Violaciones(res.Violations)
	}
	if v.FindingID == "" {
		return "", "la violacion en " + ruta + " registra su hallazgo (finding_id vacio)"
	}
	items, _, err := finding.List(root, "main", false)
	if err != nil {
		return "", "finding.List: " + err.Error()
	}
	for _, it := range items {
		if it.ID != v.FindingID {
			continue
		}
		if it.Severity != "high" || it.Status != finding.StatusOpen {
			return "", "el hallazgo del gate " + it.ID + " es high y queda abierto: " + it.Severity + "/" + it.Status
		}
		return it.ID, ""
	}
	return "", "el hallazgo del gate " + v.FindingID + " no quedo en .hoom/findings/"
}

func qcr1Violaciones(vs []Violation) string {
	var b strings.Builder
	b.WriteString("[")
	for i, v := range vs {
		if i > 0 {
			b.WriteString(" ")
		}
		b.WriteString(v.Path + "(" + v.Rule + ")")
	}
	b.WriteString("]")
	return b.String()
}

// qcr1RepoBloqueante es un proyecto con findings.block_on: high y un
// hallazgo high abierto commiteado: sin cerrarlo, verify es rojo.
func qcr1RepoBloqueante(t *testing.T) (string, string) {
	t.Helper()
	qcLimpiarEntorno(t)
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "inicial")
	f, err := finding.Add(root, "main", "high", "risk", "app.go", "el retry no respeta el backoff", "reviewer@codex")
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "hallazgo alto abierto")
	return root, f.ID
}

// qcr1Correr corre el sobre con reloj: si no vuelve en d, el test falla (la
// goroutine queda colgada; el test no).
func qcr1Correr(t *testing.T, caso, root string, opt Options) Result {
	t.Helper()
	type corrida struct {
		res Result
		err error
	}
	ch := make(chan corrida, 1)
	go func() {
		res, err := Run(root, "main", opt, io.Discard)
		ch <- corrida{res, err}
	}()
	select {
	case c := <-ch:
		if c.err != nil {
			t.Fatalf("CA-438: %s: el sobre no se rompe: %v", caso, c.err)
		}
		return c.res
	case <-time.After(qcr1Reloj):
		t.Fatalf("CA-438: %s: el sobre no volvio en %s", caso, qcr1Reloj)
	}
	return Result{}
}

// qcr1EscribirAMano es el trozo de script que escribe la resolucion sin la
// CLI, con otro autor.
func qcr1EscribirAMano(id string) string {
	return "mkdir -p .hoom/findings\ncat > " + qcComillas(qcResRel(id)) + " <<'EOF'\n" +
		qcResolucionAMano(id, finding.StatusRefuted) + "EOF\n"
}

// qcr1ExcluirEnScript es el trozo de script con el que el rol agrega la
// regla al .git/info/exclude de su repo.
func qcr1ExcluirEnScript(patron string) string {
	return "ex=\"$(git rev-parse --git-path info/exclude)\"\nmkdir -p \"$(dirname \"$ex\")\"\n" +
		"printf '\\n%s\\n' " + qcComillas(patron) + " >> \"$ex\"\n"
}

// qcr1Implementar es el trozo de script con el trabajo legitimo del writer.
const qcr1Implementar = "printf 'package app // implementado\\n' > app.go\n"

// ---------------------------------------------------------------- 1. el gate

// CA-438: una resolucion creada durante la corrida es manipulacion aunque
// Git no la liste. Para cada forma de esconderla (.git/info/exclude con el
// patron o con *.res.json, el .gitignore de la raiz, un
// .hoom/findings/.gitignore, el directorio entero, o una regla que ya estaba
// antes de la corrida) y para CADA rol que no es refutador: manipulacion en
// esa ruta con el detalle del contrato, y el hallazgo high del gate
// registrado y abierto.
func TestCA438_UnaResolucionQueGitNoListaSigueSiendoManipulacion(t *testing.T) {
	for _, e := range qcr1Escondites() {
		t.Run(e.caso, func(t *testing.T) {
			root, ids := qcRepoConHallazgo(t, 1)
			res := qcResRel(ids[0])
			if e.previo {
				e.armar(t, root)
			}
			before := Take(root, "main")
			if !e.previo {
				e.armar(t, root)
			}
			write(t, root, res, qcResolucionAMano(ids[0], finding.StatusRefuted))
			after := Take(root, "main")
			if !qcr1Ignorado(t, root, res) {
				t.Fatalf("CA-438: fixture: con %s Git ignora %s", e.caso, res)
			}

			vistos := 0
			for _, r := range agents.Roles() {
				if r.Slug == finding.RolQueRefuta {
					continue
				}
				vistos++
				got := Gate(root, "main", "", r, before, after, PolicyFor(nil, r), nil)
				qcManipulacionDe(t, e.caso+", rol "+r.Slug, r.Slug, got, res)
				qcr1HallazgoAlto(t, e.caso+", rol "+r.Slug, root, got, res)
			}
			if vistos < 5 {
				t.Fatalf("CA-438: fixture: la tabla trae los roles de hoom, trajo %d sin el refutador", vistos)
			}
		})
	}
}

// CA-438 (controles de lo mismo, escondido): mirar el disco no convierte en
// violacion lo que es legitimo. En la corrida del refutador, una resolucion
// ignorada no es violacion; en la del writer, un hallazgo nuevo ignorado
// sigue siendo legitimo, y una resolucion ignorada que ya estaba antes de la
// corrida, intacta, no es obra de la corrida.
func TestCA438_LoLegitimoQueGitNoListaSigueSinViolacion(t *testing.T) {
	t.Run("refutador con la resolucion ignorada", func(t *testing.T) {
		ref, err := agents.Lookup(finding.RolQueRefuta)
		if err != nil {
			t.Fatal(err)
		}
		root, ids := qcRepoConHallazgo(t, 1)
		before := Take(root, "main")
		qcr1Anexar(t, qcr1Exclude(t, root), qcr1Patron)
		write(t, root, qcResRel(ids[0]), qcResolucionAMano(ids[0], finding.StatusRefuted))
		if !qcr1Ignorado(t, root, qcResRel(ids[0])) {
			t.Fatalf("CA-438: fixture: Git ignora %s", qcResRel(ids[0]))
		}
		got := Gate(root, "main", "", ref, before, Take(root, "main"), PolicyFor(nil, ref), nil)
		if len(got.Violations) != 0 || !got.OK || got.Tampering {
			t.Fatalf("CA-438: en la corrida del refutador crear una resolucion, ignorada o no, no es violacion: %+v", got)
		}
	})

	t.Run("writer con un hallazgo nuevo y una resolucion previa, ignorados", func(t *testing.T) {
		wr, err := agents.Lookup("writer")
		if err != nil {
			t.Fatal(err)
		}
		root, ids := qcRepoConHallazgo(t, 1)
		// antes de la corrida: la regla y una resolucion, las dos fuera de Git
		qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
		write(t, root, qcResRel(ids[0]), qcResolucionAMano(ids[0], finding.StatusRefuted))

		before := Take(root, "main")
		nuevo, err := finding.Add(root, "main", "medium", "risk", "app.go", "otro hallazgo", "writer@claude")
		if err != nil {
			t.Fatal(err)
		}
		if !qcr1Ignorado(t, root, ".hoom/findings/"+nuevo.ID+".json") {
			t.Fatalf("CA-438: fixture: Git ignora el hallazgo nuevo")
		}
		got := Gate(root, "main", "", wr, before, Take(root, "main"), PolicyFor(nil, wr), nil)
		if len(got.Violations) != 0 || !got.OK || got.Tampering {
			t.Fatalf("CA-438: un hallazgo nuevo sigue siendo legitimo y una resolucion previa intacta no es obra de la corrida: %+v", got)
		}
	})
}

// ---------------------------------------------------------------- 1. el sobre

// CA-438 de punta a punta: 'hoom agent --role writer' en un proyecto con
// findings.block_on: high y un hallazgo high abierto. El writer implementa,
// esconde las resoluciones de Git (.git/info/exclude, o el .gitignore de la
// raiz, que su territorio le deja tocar) y escribe a mano la resolucion que
// cierra el hallazgo. El sobre NO termina verde ni entregable: el gate marca
// la manipulacion, registra su hallazgo high, y el sobre corta en scope sin
// veredicto.
func TestCA438_ElWriterQueEscondeLaResolucionDeGitNoTerminaVerde(t *testing.T) {
	for _, c := range []struct{ caso, esconder string }{
		{"info/exclude", qcr1ExcluirEnScript(qcr1Patron)},
		{".gitignore de la raiz", "printf '\\n%s\\n' " + qcComillas(qcr1Patron) + " >> .gitignore\n"},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, id := qcr1RepoBloqueante(t)
			fakeProvider(t, "claude", qcr1Implementar+c.esconder+qcr1EscribirAMano(id)+"exit 0\n")

			res := qcr1Correr(t, c.caso, root, Options{Role: "writer", Prompt: "implementa"})
			if _, err := os.Stat(filepath.Join(root, qcResRel(id))); err != nil {
				t.Fatalf("CA-438: fixture: el writer falso escribio la resolucion: %v", err)
			}
			if !qcr1Ignorado(t, root, qcResRel(id)) {
				t.Fatalf("CA-438: fixture: Git ignora %s", qcResRel(id))
			}
			if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
				t.Fatalf("CA-438: %s: una resolucion escondida de Git no deja verde al sobre del writer: status=%s verdict=%s exit=%d scope=%+v",
					c.caso, res.Status, res.Verdict, res.ExitCode, res.Scope)
			}
			qcManipulacionDe(t, c.caso, "writer", res.Scope, qcResRel(id))
			qcr1HallazgoAlto(t, c.caso, root, res.Scope, qcResRel(id))
			if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status != "no-entregable" {
				t.Fatalf("CA-438: %s: la manipulacion corta el sobre en scope, no-entregable, exit 1, sin veredicto: %+v", c.caso, res)
			}
		})
	}
}

// ---------------------------------------------------------------- 2. la corrida que falla

// CA-438: el writer cierra el hallazgo high (a mano, o con la CLI tras
// sacarse HOOM_ROLE) y su provider sale con 1. El territorio se mide igual:
// el resultado trae la manipulacion en la resolucion, el gate registra su
// hallazgo high, y el sobre queda no-entregable, distinto de 0, sin
// veredicto. Y la resolucion no deja verde a la corrida siguiente: un writer
// legitimo despues termina rojo en verify, porque findings_open cuenta el
// hallazgo high del gate.
func TestCA438_ElWriterQueCierraYFallaQuedaMedido(t *testing.T) {
	casos := []struct {
		caso string
		cli  bool
	}{
		{"a mano", false},
		{"con la CLI sin rol", true},
	}
	for _, c := range casos {
		t.Run(c.caso, func(t *testing.T) {
			var cerrar string
			root, id := qcr1RepoBloqueante(t)
			if c.cli {
				bin := qcHoom(t)
				cerrar = "HOOM_ROLE= " + qcComillas(bin) + " finding resolve " + id +
					" --as refutado --evidence 'es un duplicado' --author 'Henry Orellana' >/dev/null 2>&1\n"
			} else {
				cerrar = qcr1EscribirAMano(id)
			}
			fakeProvider(t, "claude", cerrar+"echo 'se me acabo el presupuesto' >&2\nexit 1\n")

			res := qcr1Correr(t, c.caso, root, Options{Role: "writer", Prompt: "implementa"})
			if _, err := os.Stat(filepath.Join(root, qcResRel(id))); err != nil {
				t.Fatalf("CA-438: fixture: el writer falso cerro el hallazgo antes de fallar: %v", err)
			}
			if res.ExitCode == 0 || res.Status == "entregable" || res.VerdictID != "" || res.Verdict == "green" {
				t.Errorf("CA-438: %s: el run fallido queda no-entregable, distinto de 0, sin veredicto: %+v", c.caso, res)
			}
			gid, porque := qcr1HallazgoDelGate(root, res.Scope, qcResRel(id))
			if porque != "" {
				t.Errorf("CA-438: %s: el run que falla tambien se mide: %s (scope %+v, stage %s, exit %d)",
					c.caso, porque, res.Scope, res.Stage, res.ExitCode)
			} else {
				v, _ := qcViolacion(res.Scope.Violations, qcResRel(id))
				if !strings.Contains(v.Detail, qcNoCierra("writer")) || !res.Scope.Tampering || res.Scope.OK {
					t.Errorf("CA-438: %s: la manipulacion dice %q y marca el gate: %+v", c.caso, qcNoCierra("writer"), res.Scope)
				}
			}

			// la corrida siguiente, legitima
			fakeProvider(t, "claude", qcr1Implementar+"exit 0\n")
			res2 := qcr1Correr(t, c.caso+", siguiente", root, Options{Role: "writer", Prompt: "implementa"})
			if res2.Status == "entregable" || res2.Verdict == "green" || res2.ExitCode == 0 {
				t.Fatalf("CA-438: %s: la resolucion de una corrida fallida no deja verde a la siguiente: status=%s verdict=%s exit=%d",
					c.caso, res2.Status, res2.Verdict, res2.ExitCode)
			}
			if gid == "" {
				return
			}
			if res2.Stage != "verify" || res2.Verdict != "red" || res2.VerdictID == "" {
				t.Fatalf("CA-438: %s: la siguiente corta en verify con veredicto rojo: %+v", c.caso, res2)
			}
			all, err := verdict.LoadAll(root)
			if err != nil {
				t.Fatal(err)
			}
			var gate *verdict.GateResult
			for _, v := range all {
				if v.ID != res2.VerdictID {
					continue
				}
				for i := range v.Gates {
					if v.Gates[i].Name == finding.GateName {
						gate = &v.Gates[i]
					}
				}
			}
			if gate == nil || gate.Status != verdict.StatusFail || !strings.Contains(gate.Notes, gid) {
				t.Fatalf("CA-438: %s: %s falla por el hallazgo high del gate %s: %+v", c.caso, finding.GateName, gid, gate)
			}
		})
	}
}
