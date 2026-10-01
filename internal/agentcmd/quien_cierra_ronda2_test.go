// Tests adversariales del spec .hoom/specs/quien-cierra-un-hallazgo.md,
// ronda 2 de su review (CA-438: "en la corrida de un rol que no es
// refutador, un .hoom/findings/<id>.res.json creado durante la corrida (por
// la CLI o a mano) es una violacion de manipulacion"). El rol ciego tambien
// es un rol que no es refutador, y su corrida toca dos arboles: la
// cuarentena donde corre (.hoom/isolated/<nombre>) y el arbol REAL, al que
// llega con una ruta relativa (../../../, la misma de la fuga de CA-184).
// Una resolucion creada en el arbol real durante su corrida es manipulacion
// igual:
//
//  1. aunque el provider salga con error: la corrida que falla tambien se
//     mide, el gate registra su hallazgo high, y la resolucion no deja verde
//     a la corrida siguiente;
//  2. aunque salga con 0 despues de esconderla de Git con
//     .git/info/exclude, que la cuarentena (un worktree) comparte con el
//     arbol real, o con una regla que ya estaba antes de la corrida (y sin
//     esconderla: que ademas sea fuga no le quita ser manipulacion).
//
// Controles: la corrida ciega que solo escribe su test sigue como hoy, y una
// resolucion que ya estaba en el arbol real antes de la corrida, intacta, no
// es obra de la corrida. Y la nota del sobre de una corrida fallida que se
// midio no dice que no hubo arbol que medir.
//
// Sobres reales con un claude falso y con reloj; los que corren la CLI
// compilan hoom (se omiten con -short).
package agentcmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// qcr2Real es la ruta, desde la cuarentena (.hoom/isolated/<nombre>), a la
// raiz del arbol real.
const qcr2Real = "../../../"

// qcr2TestLegitimo es el trabajo legitimo del test-writer: un test nuevo.
const qcr2TestLegitimo = "printf 'package app\\n// CA-1 nuevo\\n' > nuevo_test.go\n"

// qcr2RepoCiegoBloqueante es un proyecto donde el test-writer corre ciego
// (repo con commits, un spec y un test) con findings.block_on: high y un
// hallazgo high abierto commiteado: sin cerrarlo, verify es rojo.
func qcr2RepoCiegoBloqueante(t *testing.T) (string, string) {
	t.Helper()
	qcLimpiarEntorno(t)
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	write(t, root, "app.go", "package app\n")
	write(t, root, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\n")
	write(t, root, "app_test.go", "package app\n\n// CA-1\n")
	specDemo(t, root)
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

// qcr2EscribirEnElReal es el trozo de script con el que el rol, desde la
// cuarentena, escribe a mano en el arbol real la resolucion que cierra id.
func qcr2EscribirEnElReal(id string) string {
	return "mkdir -p " + qcComillas(qcr2Real+".hoom/findings") + "\n" +
		"cat > " + qcComillas(qcr2Real+qcResRel(id)) + " <<'EOF'\n" +
		qcResolucionAMano(id, finding.StatusRefuted) + "EOF\n"
}

// qcr2Donde es el trozo de script que deja en archivo el directorio donde
// corre el provider: la prueba de que corrio en la cuarentena.
func qcr2Donde(archivo string) string {
	return "pwd > " + qcComillas(archivo) + "\n"
}

// qcr2EnLaCuarentena exige que el provider haya corrido en la cuarentena.
func qcr2EnLaCuarentena(t *testing.T, caso, archivo string) {
	t.Helper()
	raw, err := os.ReadFile(archivo)
	if err != nil {
		t.Fatalf("CA-438: %s: fixture: el provider falso dejo su directorio: %v", caso, err)
	}
	if !strings.Contains(filepath.ToSlash(string(raw)), ".hoom/isolated/") {
		t.Fatalf("CA-438: %s: fixture: el test-writer corre en la cuarentena, corrio en %q", caso, strings.TrimSpace(string(raw)))
	}
}

// qcr2Correr corre el sobre con reloj, escribiendo la salida en w.
func qcr2Correr(t *testing.T, caso, root string, opt Options, w io.Writer) Result {
	t.Helper()
	type corrida struct {
		res Result
		err error
	}
	ch := make(chan corrida, 1)
	go func() {
		res, err := Run(root, "main", opt, w)
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

// qcr2Manipulacion busca, entre las violaciones, la de MANIPULACION en ruta
// (puede haber tambien una de aislamiento en la misma ruta: la fuga), exige
// el detalle del contrato para rol y el gate marcado, y que su hallazgo este
// registrado en el arbol real, high y abierto. Devuelve el id del hallazgo,
// o por que no esta.
func qcr2Manipulacion(root string, res ScopeResult, ruta, rol string) (string, string) {
	var v Violation
	encontrada := false
	for _, x := range res.Violations {
		if x.Path == ruta && x.Rule == RuleTampering {
			v, encontrada = x, true
			break
		}
	}
	if !encontrada {
		return "", "no hay violacion de manipulacion en " + ruta + ": " + qcr1Violaciones(res.Violations)
	}
	if !strings.Contains(v.Detail, qcNoCierra(rol)) {
		return "", "el detalle de la manipulacion en " + ruta + " dice " + qcComillas(qcNoCierra(rol)) + ": " + qcComillas(v.Detail)
	}
	if !res.Tampering || res.OK {
		return "", "el gate queda marcado como manipulacion (tampering y no ok)"
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
	return "", "el hallazgo del gate " + v.FindingID + " no quedo en .hoom/findings/ del arbol real"
}

// qcr2FindingsOpen devuelve el gate findings_open del veredicto id.
func qcr2FindingsOpen(t *testing.T, root, id string) *verdict.GateResult {
	t.Helper()
	all, err := verdict.LoadAll(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range all {
		if v.ID != id {
			continue
		}
		for i := range v.Gates {
			if v.Gates[i].Name == finding.GateName {
				return &v.Gates[i]
			}
		}
	}
	return nil
}

// ---------------------------------------------------------------- 1. el rol ciego que cierra y falla

// CA-438: 'hoom agent --role test-writer' corre ciego en .hoom/isolated, pero
// con shell: desde la cuarentena cierra en el arbol REAL el hallazgo high
// (a mano con ../../../.hoom/findings/<id>.res.json, o con la CLI tras
// sacarse HOOM_ROLE) y su provider sale con 1. La corrida que falla tambien
// se mide: el resultado trae la manipulacion en esa ruta con "un test-writer
// no cierra hallazgos" (puede venir ademas la fuga), el gate registra su
// hallazgo high, y el sobre queda no-entregable, distinto de 0, sin
// veredicto. Y la resolucion no deja verde a la corrida siguiente: un writer
// legitimo despues termina rojo en verify, porque findings_open cuenta el
// hallazgo high del gate.
func TestCA438_ElRolCiegoQueCierraEnElArbolRealYFallaQuedaMedido(t *testing.T) {
	for _, c := range []struct {
		caso string
		cli  bool
	}{
		{"a mano", false},
		{"con la CLI sin rol", true},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, id := qcr2RepoCiegoBloqueante(t)
			cerrar := qcr2EscribirEnElReal(id)
			if c.cli {
				bin := qcHoom(t)
				cerrar = "(cd " + qcComillas(qcr2Real) + " && HOOM_ROLE= " + qcComillas(bin) + " finding resolve " + id +
					" --as refutado --evidence 'es un duplicado' --author 'Henry Orellana' >/dev/null 2>&1)\n"
			}
			donde := filepath.Join(t.TempDir(), "pwd.txt")
			fakeProvider(t, "claude", qcr2Donde(donde)+cerrar+"echo 'se me acabo el presupuesto' >&2\nexit 1\n")

			res := qcr2Correr(t, c.caso, root, Options{Role: "test-writer", Prompt: "escribi los tests"}, io.Discard)
			qcr2EnLaCuarentena(t, c.caso, donde)
			if _, err := os.Stat(filepath.Join(root, qcResRel(id))); err != nil {
				t.Fatalf("CA-438: %s: fixture: el test-writer falso cerro el hallazgo en el arbol real antes de fallar: %v", c.caso, err)
			}
			if res.ExitCode == 0 || res.Status == "entregable" || res.VerdictID != "" || res.Verdict == "green" {
				t.Errorf("CA-438: %s: el run ciego fallido queda no-entregable, distinto de 0, sin veredicto: %+v", c.caso, res)
			}
			gid, porque := qcr2Manipulacion(root, res.Scope, qcResRel(id), "test-writer")
			if porque != "" {
				t.Errorf("CA-438: %s: el run ciego que falla tambien se mide en el arbol real: %s (scope %+v, stage %s, exit %d)",
					c.caso, porque, res.Scope, res.Stage, res.ExitCode)
			}

			// la corrida siguiente, legitima
			fakeProvider(t, "claude", qcr1Implementar+"exit 0\n")
			res2 := qcr2Correr(t, c.caso+", siguiente", root, Options{Role: "writer", Prompt: "implementa"}, io.Discard)
			if res2.Status == "entregable" || res2.Verdict == "green" || res2.ExitCode == 0 {
				t.Fatalf("CA-438: %s: la resolucion de una corrida ciega fallida no deja verde a la siguiente: status=%s verdict=%s exit=%d",
					c.caso, res2.Status, res2.Verdict, res2.ExitCode)
			}
			if gid == "" {
				return
			}
			if res2.Stage != "verify" || res2.Verdict != "red" || res2.VerdictID == "" {
				t.Fatalf("CA-438: %s: la siguiente corta en verify con veredicto rojo: %+v", c.caso, res2)
			}
			if g := qcr2FindingsOpen(t, root, res2.VerdictID); g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.Notes, gid) {
				t.Fatalf("CA-438: %s: %s falla por el hallazgo high del gate %s: %+v", c.caso, finding.GateName, gid, g)
			}
		})
	}
}

// ---------------------------------------------------------------- 2. el rol ciego que cierra y sale con 0

// CA-438: el test-writer ciego sale con 0 despues de escribir su test (trabajo
// legitimo) y, en el arbol REAL, la resolucion que cierra el hallazgo high,
// escondida de Git con .git/info/exclude — que la cuarentena comparte con el
// arbol real —: la regla la agrega el rol durante la corrida, o ya estaba
// antes. Git no lista la resolucion, pero el gate mira el arbol: el sobre NO
// termina verde ni entregable, y el resultado trae la manipulacion en esa
// ruta con "un test-writer no cierra hallazgos" y su hallazgo high
// registrado y abierto. Sin esconderla, lo mismo: que ademas sea fuga no
// le quita ser manipulacion.
func TestCA438_ElRolCiegoQueCierraEnElArbolRealYSaleConCeroNoTerminaVerde(t *testing.T) {
	const (
		durante = iota // el rol agrega la regla a info/exclude
		previa         // la regla ya estaba antes de la corrida
		visible        // sin esconderla: Git la lista
	)
	for _, c := range []struct {
		caso string
		modo int
	}{
		{"regla en info/exclude durante la corrida", durante},
		{"regla en info/exclude de antes de la corrida", previa},
		{"sin esconderla de Git", visible},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, id := qcr2RepoCiegoBloqueante(t)
			esconder := ""
			switch c.modo {
			case previa:
				qcr1Anexar(t, qcr1Exclude(t, root), qcr1Patron)
			case durante:
				esconder = qcr1ExcluirEnScript(qcr1Patron)
			}
			donde := filepath.Join(t.TempDir(), "pwd.txt")
			fakeProvider(t, "claude", qcr2Donde(donde)+qcr2TestLegitimo+esconder+qcr2EscribirEnElReal(id)+"exit 0\n")

			res := qcr2Correr(t, c.caso, root, Options{Role: "test-writer", Prompt: "escribi los tests"}, io.Discard)
			qcr2EnLaCuarentena(t, c.caso, donde)
			if _, err := os.Stat(filepath.Join(root, qcResRel(id))); err != nil {
				t.Fatalf("CA-438: %s: fixture: el test-writer falso escribio la resolucion en el arbol real: %v", c.caso, err)
			}
			if qcr1Ignorado(t, root, qcResRel(id)) != (c.modo != visible) {
				t.Fatalf("CA-438: %s: fixture: Git ignora %s en el arbol real solo si se escondio", c.caso, qcResRel(id))
			}
			if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
				t.Errorf("CA-438: %s: una resolucion creada en el arbol real no deja verde al sobre ciego: status=%s verdict=%s exit=%d stage=%s scope=%+v",
					c.caso, res.Status, res.Verdict, res.ExitCode, res.Stage, res.Scope)
			}
			if _, porque := qcr2Manipulacion(root, res.Scope, qcResRel(id), "test-writer"); porque != "" {
				t.Fatalf("CA-438: %s: %s (scope %+v, stage %s, exit %d)", c.caso, porque, res.Scope, res.Stage, res.ExitCode)
			}
		})
	}
}

// ---------------------------------------------------------------- 3. controles

// CA-438 (controles del rol ciego): mirar el arbol real no convierte en
// violacion lo que es legitimo.
//
//   - La corrida ciega que solo escribe su test sigue como hoy: sin
//     violaciones, el test viaja al arbol real, el hallazgo sigue abierto y
//     lo que deja rojo a verify es ese hallazgo high, no la corrida.
//   - Una resolucion que ya estaba en el arbol real antes de la corrida
//     (escondida de Git por una regla previa), intacta, no es obra de la
//     corrida: sin violaciones, el sobre entrega verde.
func TestCA438_ElRolCiegoQueSoloEscribeSuTestSigueComoHoy(t *testing.T) {
	t.Run("sin resoluciones", func(t *testing.T) {
		root, id := qcr2RepoCiegoBloqueante(t)
		fakeProvider(t, "claude", qcr2TestLegitimo+"exit 0\n")

		res := qcr2Correr(t, "control", root, Options{Role: "test-writer", Prompt: "escribi los tests"}, io.Discard)
		if len(res.Scope.Violations) != 0 || !res.Scope.OK || res.Scope.Tampering {
			t.Fatalf("CA-438: el test-writer que solo escribe su test no tiene violaciones: %+v", res.Scope)
		}
		if res.Isolation == nil {
			t.Fatalf("CA-438: fixture: el test-writer corrio ciego: %+v", res)
		}
		if !existeEn(t, filepath.Join(root, "nuevo_test.go")) {
			t.Fatalf("CA-438: el test legitimo viaja al arbol real: %+v", res.Isolation)
		}
		if existeEn(t, filepath.Join(root, qcResRel(id))) {
			t.Fatalf("CA-438: nadie escribio %s", qcResRel(id))
		}
		qcAbierto(t, "CA-438", root, id)
		if res.VerdictID == "" || res.Verdict != "red" || res.Status == "entregable" || res.ExitCode == 0 {
			t.Fatalf("CA-438: el sobre llega a verify, que queda rojo por el hallazgo high abierto: %+v", res)
		}
		if g := qcr2FindingsOpen(t, root, res.VerdictID); g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.Notes, id) {
			t.Fatalf("CA-438: %s falla por el hallazgo original %s: %+v", finding.GateName, id, g)
		}
	})

	t.Run("con una resolucion previa escondida en el arbol real", func(t *testing.T) {
		root, id := qcr2RepoCiegoBloqueante(t)
		// antes de la corrida: la regla y la resolucion, las dos fuera de Git
		qcr1Anexar(t, qcr1Exclude(t, root), qcr1Patron)
		write(t, root, qcResRel(id), qcResolucionAMano(id, finding.StatusRefuted))
		if !qcr1Ignorado(t, root, qcResRel(id)) {
			t.Fatalf("CA-438: fixture: Git ignora %s", qcResRel(id))
		}
		fakeProvider(t, "claude", qcr2TestLegitimo+"exit 0\n")

		res := qcr2Correr(t, "control previo", root, Options{Role: "test-writer", Prompt: "escribi los tests"}, io.Discard)
		if len(res.Scope.Violations) != 0 || !res.Scope.OK || res.Scope.Tampering {
			t.Fatalf("CA-438: una resolucion previa e intacta del arbol real no es obra de la corrida: %+v", res.Scope)
		}
		if res.Status != "entregable" || res.Verdict != "green" || res.ExitCode != 0 {
			t.Fatalf("CA-438: sin violaciones y con el hallazgo ya cerrado antes, el sobre entrega verde: %+v", res)
		}
		if !existeEn(t, filepath.Join(root, "nuevo_test.go")) {
			t.Fatalf("CA-438: el test legitimo viaja al arbol real: %+v", res.Isolation)
		}
	})
}

// ---------------------------------------------------------------- 4. la nota del run fallido

// qcr2SinAcentos normaliza un texto para buscar frases sin depender de
// tildes ni mayusculas.
func qcr2SinAcentos(s string) string {
	return strings.ToLower(strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
		"Á", "a", "É", "e", "Í", "i", "Ó", "o", "Ú", "u",
	).Replace(s))
}

// qcr2NoMedido dice si s afirma que no hubo arbol que medir.
func qcr2NoMedido(s string) bool {
	n := qcr2SinAcentos(s)
	return strings.Contains(n, "no hay arbol confiable") || strings.Contains(n, "arbol confiable que medir")
}

// CA-438 (hallazgo a68562): la corrida que falla se mide — la del writer en
// el arbol de trabajo y la del test-writer ciego en el arbol real —, asi que
// la nota con la que cierra el sobre no puede decir que no hubo arbol
// confiable que medir: dice que el run fallo. Ni el registro del sobre ni su
// salida afirman lo contrario de lo que el sobre acaba de hacer.
func TestCA438_LaNotaDeUnRunFallidoQueSeMidioNoDiceQueNoHayArbol(t *testing.T) {
	for _, c := range []struct {
		caso, rol string
		ciego     bool
	}{
		{"writer en el arbol de trabajo", "writer", false},
		{"test-writer en la cuarentena", "test-writer", true},
	} {
		t.Run(c.caso, func(t *testing.T) {
			qcLimpiarEntorno(t)
			var root string
			if c.ciego {
				root = repoCiego(t)
			} else {
				root = repo(t)
			}
			fakeProvider(t, "claude", "echo 'se me acabo el presupuesto' >&2\nexit 1\n")

			var buf bytes.Buffer
			res := qcr2Correr(t, c.caso, root, Options{Role: c.rol, Prompt: "trabaja"}, &buf)
			if res.ExitCode == 0 || res.Status == "entregable" || res.RunID == "" {
				t.Fatalf("CA-438: %s: fixture: el run corrio y fallo: %+v", c.caso, res)
			}
			if c.ciego && res.Isolation == nil {
				t.Fatalf("CA-438: %s: fixture: el test-writer corrio ciego: %+v", c.caso, res)
			}

			var rec *envelope.Record
			for _, r := range envelope.List(root) {
				if r.ID == res.EnvelopeID {
					rec = &r
				}
			}
			if rec == nil {
				t.Fatalf("CA-438: %s: fixture: el sobre %q dejo su registro", c.caso, res.EnvelopeID)
			}
			if rec.Note == "" {
				t.Fatalf("CA-438: %s: el registro del run fallido dice por que cerro: %+v", c.caso, rec)
			}
			if qcr2NoMedido(rec.Note) {
				t.Errorf("CA-438: %s: el run fallido se mide; la nota no dice que no hubo arbol que medir: %q", c.caso, rec.Note)
			}
			n := qcr2SinAcentos(rec.Note)
			if !strings.Contains(n, "fall") && !strings.Contains(n, "error") && !strings.Contains(n, "exit") {
				t.Errorf("CA-438: %s: la nota dice que el run fallo: %q", c.caso, rec.Note)
			}
			if qcr2NoMedido(buf.String()) {
				t.Errorf("CA-438: %s: la salida del sobre tampoco dice que no hubo arbol que medir:\n%s", c.caso, buf.String())
			}
		})
	}
}
