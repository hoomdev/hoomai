// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md sobre
// el sobre:
//   - CA-637: el registro del test-writer guarda `trasplantados` (lo que el
//     trasplante aplico, no lo que borro), tambien en un cierre rojo;
//   - CA-638/CA-639: el writer no cambia, borra ni renombra un test
//     protegido (regla tests-del-test-writer), y nada mas es violacion;
//   - CA-651: un run que muere por su timeout deja corte tiempo en el sobre;
//   - CA-661: Options.Columna/Regla/Intento/Ronda/Hallazgos/Bloqueantes
//     llegan al registro.
//
// CLIs falsos (fakeProvider): ninguno real.
package agentcmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const (
	aeaTarea     = "demo"
	aeaProtegido = "precios_test.go"
	aeaTestTW    = "package app\n\n// CA-1 precios\n"
	// aeaEscribeTW es el test-writer falso: deja el test que la tarea va a
	// proteger en su arbol ciego.
	aeaEscribeTW = "printf 'package app\\n\\n// CA-1 precios\\n' > " + aeaProtegido + "\nexit 0\n"
)

// aeaProyecto es un proyecto hoom con un gate (gate es su comando), un spec,
// un test existente y la tarea demo iniciada (.hoom/worktrees/demo).
func aeaProyecto(t *testing.T, gate, extra string) (root, wt string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \""+gate+"\"\n"+extra)
	write(t, root, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\n")
	write(t, root, "app.go", "package app\n")
	write(t, root, "app_test.go", "package app\n\n// CA-1\n")
	write(t, root, "viejo_test.go", "package app\n\n// CA-1 obsoleto\n")
	specDemo(t, root)
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "inicial")
	if err := taskcmd.Start(root, aeaTarea, "main"); err != nil {
		t.Fatalf("fixture: hoom task start %s: %v", aeaTarea, err)
	}
	return root, filepath.Join(root, ".hoom", "worktrees", aeaTarea)
}

// aeaCorrer corre el rol en la tarea con el CLI falso script.
func aeaCorrer(t *testing.T, root, rol, script string, o Options) (Result, string) {
	t.Helper()
	fakeProvider(t, "claude", script)
	o.Role, o.Task = rol, aeaTarea
	if o.Prompt == "" {
		o.Prompt = "trabaja en la tarea demo"
	}
	var buf bytes.Buffer
	res, err := Run(root, "main", o, &buf)
	if err != nil {
		t.Fatalf("fixture: el sobre del %s corre: %v\n%s", rol, err, buf.String())
	}
	if res.EnvelopeID == "" {
		t.Fatalf("fixture: el sobre del %s tiene registro: %+v", rol, res)
	}
	return res, buf.String()
}

// aeaGuardar commitea lo que el worktree tiene sin guardar (lo que haria el
// piloto antes de la vuelta siguiente).
func aeaGuardar(t *testing.T, wt string) {
	t.Helper()
	git(t, wt, "add", "-A")
	git(t, wt, "commit", "-q", "--allow-empty", "-m", "hoom: guardar la tarjeta "+aeaTarea)
}

// aeaClaves es el registro del sobre id como mapa crudo.
func aeaClaves(t *testing.T, root, id string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, id+".json"))
	if err != nil {
		t.Fatalf("el registro vive en .hoom/envelopes/%s.json de la raiz: %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("el registro es JSON: %v\n%s", err, raw)
	}
	return m
}

// aeaRegistro es el registro leido por envelope.List.
func aeaRegistro(t *testing.T, root, id string) envelope.Record {
	t.Helper()
	for _, r := range envelope.List(root) {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("envelope.List de la raiz trae el sobre %s", id)
	return envelope.Record{}
}

// aeaTrasplantados es la clave cruda del registro: sus rutas y si esta.
func aeaTrasplantados(t *testing.T, root, id string) ([]string, bool) {
	t.Helper()
	v, ok := aeaClaves(t, root, id)["trasplantados"]
	if !ok {
		return nil, false
	}
	xs, _ := v.([]any)
	out := []string{}
	for _, x := range xs {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out, true
}

// aeaVeredictos cuenta los veredictos en disco de los arboles.
func aeaVeredictos(dirs ...string) int {
	n := 0
	for _, d := range dirs {
		es, _ := os.ReadDir(filepath.Join(d, ".hoom", "verdicts"))
		n += len(es)
	}
	return n
}

// aeaHallazgo lee el hallazgo id de .hoom/findings/ de cualquiera de los
// arboles.
func aeaHallazgo(t *testing.T, id string, dirs ...string) (finding.Finding, bool) {
	t.Helper()
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(d, ".hoom", "findings", id+".json"))
		if err != nil {
			continue
		}
		var f finding.Finding
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("el hallazgo %s es JSON: %v", id, err)
		}
		return f, true
	}
	return finding.Finding{}, false
}

// aeaViolacion es la violacion de la regla en la ruta, y si esta.
func aeaViolacion(s ScopeResult, regla, ruta string) (Violation, bool) {
	for _, v := range s.Violations {
		if v.Rule == regla && (ruta == "" || v.Path == ruta) {
			return v, true
		}
	}
	return Violation{}, false
}

// ---------------------------------------------------------------- CA-637

// CA-637: el camino limpio: trasplantados son las altas y sobrescrituras,
// ordenadas, con "/", relativas al arbol de la tarea; lo borrado no entra.
func TestCA637_TrasplantadosEsLoQueElTrasplanteAplico(t *testing.T) {
	root, wt := aeaProyecto(t, "true", "")
	res, out := aeaCorrer(t, root, "test-writer",
		"mkdir -p sub/hondo\nprintf 'package hondo\\n// CA-1\\n' > sub/hondo/beta_test.go\n"+
			"printf 'package app\\n// CA-1 zeta\\n' > zeta_test.go\n"+
			"printf 'package app\\n// CA-1 reescrito\\n' > app_test.go\n"+
			"rm viejo_test.go\nexit 0\n", Options{})
	if res.Isolation == nil || res.Scope.Cuts() {
		t.Fatalf("CA-637: fixture: el test-writer ciego trasplanta: %+v\n%s", res, out)
	}
	want := []string{"app_test.go", "sub/hondo/beta_test.go", "zeta_test.go"}
	got, ok := aeaTrasplantados(t, root, res.EnvelopeID)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("CA-637: \"trasplantados\" es %q (sin viejo_test.go, que se borro); fue %q (%v)", want, got, ok)
	}
	if r := aeaRegistro(t, root, res.EnvelopeID); !reflect.DeepEqual(r.Trasplantados, want) || r.Task != aeaTarea {
		t.Fatalf("CA-637: Record.Trasplantados: %+v", r)
	}
	if !existeEn(t, filepath.Join(wt, "sub", "hondo", "beta_test.go")) || existeEn(t, filepath.Join(root, "zeta_test.go")) {
		t.Fatal("CA-637: fixture: el trasplante va al arbol de la tarea, no al principal")
	}
	// la union de la tarea es lo trasplantado
	prot, err := TestsProtegidos(root, aeaTarea)
	if err != nil || !reflect.DeepEqual(prot, want) {
		t.Fatalf("CA-637/CA-639: TestsProtegidos de la tarea son %q: %q %v", want, prot, err)
	}
}

// CA-637: tambien cuando el sobre cierra rojo en verify o check, y en scope
// por fuera-de-scope (con solo lo que el gate dejo viajar).
func TestCA637_TrasplantadosTambienEnUnCierreRojo(t *testing.T) {
	root, _ := aeaProyecto(t, "false", "")
	res, out := aeaCorrer(t, root, "test-writer", aeaEscribeTW, Options{})
	if res.VerdictID == "" || res.Verdict == "green" || res.ExitCode == 0 {
		t.Fatalf("CA-637: fixture: con el gate en rojo el sobre del test-writer cierra rojo con veredicto: %+v\n%s", res, out)
	}
	if got, ok := aeaTrasplantados(t, root, res.EnvelopeID); !ok || !reflect.DeepEqual(got, []string{aeaProtegido}) {
		t.Fatalf("CA-637: cierre rojo en %s: trasplantados [%s]; fue %q (%v)", res.Stage, aeaProtegido, got, ok)
	}

	root, wt := aeaProyecto(t, "true", "")
	res, out = aeaCorrer(t, root, "test-writer",
		"printf 'package app\\n// CA-1\\n' > nuevo_test.go\nprintf 'package nuevo\\n' > nuevo.go\nexit 0\n", Options{})
	if res.Stage != "scope" || res.Scope.Cuts() || res.ExitCode != 1 {
		t.Fatalf("CA-637: fixture: fuera-de-scope cierra en scope sin cortar: %+v\n%s", res, out)
	}
	if got, ok := aeaTrasplantados(t, root, res.EnvelopeID); !ok || !reflect.DeepEqual(got, []string{"nuevo_test.go"}) {
		t.Fatalf("CA-637: fuera-de-scope: trasplantados es lo que viajo (nuevo_test.go), no nuevo.go: %q (%v)", got, ok)
	}
	if existeEn(t, filepath.Join(wt, "nuevo.go")) {
		t.Fatal("CA-637: fixture: lo fuera de scope no viaja")
	}
}

// CA-637: un sobre sin trasplante no lleva la clave: el run fallo, el
// aislamiento se rompio, o no habia nada que trasplantar.
func TestCA637_SinTrasplanteNoHayClave(t *testing.T) {
	for _, c := range []struct {
		nombre, script string
	}{
		{"el run falla", "printf 'package app\\n// CA-1\\n' > roto_test.go\nexit 1\n"},
		{"el testigo vuelve", "printf 'package app\\n' > app.go\nprintf 'package app\\n// CA-1\\n' > nuevo_test.go\nexit 0\n"},
		{"nada que trasplantar", "exit 0\n"},
	} {
		root, wt := aeaProyecto(t, "true", "")
		res, out := aeaCorrer(t, root, "test-writer", c.script, Options{})
		if got, ok := aeaTrasplantados(t, root, res.EnvelopeID); ok {
			t.Fatalf("CA-637: %s: sin trasplante el registro no lleva trasplantados, lleva %q: %+v\n%s", c.nombre, got, res, out)
		}
		if r := aeaRegistro(t, root, res.EnvelopeID); r.Trasplantados != nil {
			t.Fatalf("CA-637: %s: Record.Trasplantados nil: %q", c.nombre, r.Trasplantados)
		}
		if existeEn(t, filepath.Join(wt, "nuevo_test.go")) || existeEn(t, filepath.Join(wt, "roto_test.go")) {
			t.Fatalf("CA-637: %s: fixture: no hubo trasplante", c.nombre)
		}
		if prot, err := TestsProtegidos(root, aeaTarea); err != nil || len(prot) != 0 {
			t.Fatalf("CA-637: %s: la tarea no protege nada: %q %v", c.nombre, prot, err)
		}
	}
}

// ---------------------------------------------------------------- CA-638

// aeaProtegida es la tarea con el test del test-writer trasplantado (y
// commiteado si guardado).
func aeaProtegida(t *testing.T, guardado bool, extra string) (root, wt string) {
	t.Helper()
	root, wt = aeaProyecto(t, "true", extra)
	res, out := aeaCorrer(t, root, "test-writer", aeaEscribeTW, Options{})
	if got, _ := aeaTrasplantados(t, root, res.EnvelopeID); !reflect.DeepEqual(got, []string{aeaProtegido}) {
		t.Fatalf("fixture: el test-writer protege %s: %q %+v\n%s", aeaProtegido, got, res, out)
	}
	if guardado {
		aeaGuardar(t, wt)
	}
	return root, wt
}

// aeaCortado exige el cierre del contrato para un writer que toco el test
// protegido ruta.
func aeaCortado(t *testing.T, ca, caso, root, wt string, res Result, out string, antes int, ruta string) {
	t.Helper()
	if res.Stage != "scope" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 1 || !res.Scope.Cuts() {
		t.Fatalf("%s: %s: el sobre del writer cierra en scope, no-entregable, exit 1: %+v\n%s", ca, caso, res, out)
	}
	if res.VerdictID != "" || res.Check != nil || aeaVeredictos(root, wt) != antes {
		t.Fatalf("%s: %s: sin verify ni veredicto nuevo: %+v (veredictos %d -> %d)", ca, caso, res, antes, aeaVeredictos(root, wt))
	}
	if res.Note != NotaTestsDelTestWriter {
		t.Fatalf("%s: %s: la nota es %q; fue %q", ca, caso, NotaTestsDelTestWriter, res.Note)
	}
	if r := aeaRegistro(t, root, res.EnvelopeID); r.Note != NotaTestsDelTestWriter || r.Stage != "scope" || r.Status != envelope.StatusNotDeliverable {
		t.Fatalf("%s: %s: el registro dice lo mismo: %+v", ca, caso, r)
	}
	v, ok := aeaViolacion(res.Scope, RuleTestsDelTestWriter, ruta)
	if !ok || v.Detail != DetalleTestsDelTestWriter || v.FindingID == "" {
		t.Fatalf("%s: %s: violacion %s en %s con su detalle y su hallazgo: %+v", ca, caso, RuleTestsDelTestWriter, ruta, res.Scope.Violations)
	}
	f, ok := aeaHallazgo(t, v.FindingID, wt, root)
	if !ok {
		t.Fatalf("%s: %s: el hallazgo %s del gate quedo en disco", ca, caso, v.FindingID)
	}
	pre := "tests-del-test-writer: el rol writer escribio " + ruta
	if f.Severity != "high" || f.Author != "hoom gate de scope" || !strings.HasPrefix(f.Description, pre) ||
		!strings.Contains(f.Description, DetalleTestsDelTestWriter) {
		t.Fatalf("%s: %s: hallazgo high de `hoom gate de scope` que empieza con %q: %+v", ca, caso, pre, f)
	}
}

// CA-638: el writer cambia, borra o renombra el test protegido, commiteado
// o no: el sobre corta en scope con la nota y el hallazgo del contrato.
func TestCA638_ElWriterQueTocaUnTestProtegidoCorta(t *testing.T) {
	for _, c := range []struct {
		nombre   string
		guardado bool
		script   string
	}{
		{"lo cambia sin commitear", true, "printf 'package app\\n\\n// CA-1 precios, arreglado por el writer\\n' > " + aeaProtegido + "\nexit 0\n"},
		{"lo cambia y lo commitea", true, "printf 'package app\\n// otro\\n' > " + aeaProtegido + "\ngit add -A && git commit -qm 'el writer arregla el test'\nexit 0\n"},
		{"lo cambia antes de guardar el trasplante", false, "printf 'package app\\n// otro\\n' > " + aeaProtegido + "\nexit 0\n"},
		{"lo borra", true, "rm " + aeaProtegido + "\nexit 0\n"},
		{"lo borra y lo commitea", true, "git rm -q " + aeaProtegido + " && git commit -qm 'fuera'\nexit 0\n"},
		{"lo renombra y lo commitea", true, "git mv " + aeaProtegido + " renombrado_test.go && git commit -qm 'renombre'\nexit 0\n"},
		{"lo renombra sin commitear", false, "mv " + aeaProtegido + " renombrado_test.go\nexit 0\n"},
	} {
		root, wt := aeaProtegida(t, c.guardado, "")
		antes := aeaVeredictos(root, wt)
		res, out := aeaCorrer(t, root, "writer", "printf 'package app // implementado\\n' > app.go\n"+c.script, Options{})
		aeaCortado(t, "CA-638", c.nombre, root, wt, res, out, antes, aeaProtegido)
	}
}

// CA-638: si el mismo run ademas manipulo la evidencia, corta igual y la
// nota es la de la manipulacion, no la nueva.
func TestCA638_ConManipulacionLaNotaEsLaDeHoy(t *testing.T) {
	root, wt := aeaProtegida(t, true, "")
	res, out := aeaCorrer(t, root, "writer",
		"printf 'package app\\n// otro\\n' > "+aeaProtegido+"\nmkdir -p .hoom/verdicts\n"+
			"printf '{\"id\":\"falso\"}\\n' > .hoom/verdicts/2026-11-12T00-00-00Z_falso.json\nexit 0\n", Options{})
	if res.Stage != "scope" || res.ExitCode != 1 || !res.Scope.Cuts() || !res.Scope.Tampering || res.VerdictID != "" {
		t.Fatalf("CA-638: manipular y tocar el test protegido corta en scope: %+v\n%s", res, out)
	}
	if res.Note == NotaTestsDelTestWriter || res.Note == "" {
		t.Fatalf("CA-638: con manipulacion la nota es la de la manipulacion: %q", res.Note)
	}
	if _, ok := aeaViolacion(res.Scope, RuleTestsDelTestWriter, aeaProtegido); !ok {
		t.Fatalf("CA-638: la violacion del test protegido sigue en el resultado: %+v", res.Scope.Violations)
	}
	if raw, _ := os.ReadFile(filepath.Join(wt, aeaProtegido)); string(raw) == aeaTestTW {
		t.Fatal("CA-638: fixture: el writer cambio el test protegido")
	}
}

// CA-638/CA-639: agents.writer.write del hoom.yaml que permite la ruta no
// levanta la regla.
func TestCA639_AgentsWriterWriteNoLevantaLaRegla(t *testing.T) {
	root, wt := aeaProtegida(t, true, "agents:\n  writer:\n    write:\n      allow: [\"**\", \""+aeaProtegido+"\", \"*_test.go\"]\n")
	antes := aeaVeredictos(root, wt)
	res, out := aeaCorrer(t, root, "writer", "printf 'package app\\n// otro\\n' > "+aeaProtegido+"\nexit 0\n", Options{})
	aeaCortado(t, "CA-639", "con write.allow", root, wt, res, out, antes, aeaProtegido)
}

// ---------------------------------------------------------------- CA-639

// CA-639: el writer que crea un test nuevo, cambia uno no protegido o
// reescribe el protegido con el mismo contenido no viola la regla. Cuando el
// run entrega algo, el sobre no corta y verify corre. Un run que solo
// reescribe el protegido con su mismo contenido (y touch) no entrega nada:
// cierra sin-entrega por el contrato de arquitecto-bajo-el-sobre (aclaracion
// 11), y ahi lo que se fija es que no hay violacion ni la nota de la regla.
func TestCA639_NoEsViolacionLoQueNoTocaUnTestProtegido(t *testing.T) {
	const mismo = "printf 'package app\\n\\n// CA-1 precios\\n' > " + aeaProtegido + "\ntouch " + aeaProtegido + "\n"
	for _, c := range []struct {
		nombre, script string
		entrega        bool
	}{
		{"test nuevo", "printf 'package app\\n// CA-1 del writer\\n' > nuevo_test.go\n", true},
		{"test no protegido", "printf 'package app\\n\\n// CA-1 cambiado\\n' > app_test.go\n", true},
		{"mismo contenido, con codigo", mismo + "printf 'package app // implementado\\n' > app.go\n", true},
		{"solo codigo", "printf 'package app // implementado\\n' > app.go\n", true},
		{"mismo contenido, nada mas", mismo, false},
	} {
		root, wt := aeaProtegida(t, true, "")
		res, out := aeaCorrer(t, root, "writer", c.script+"exit 0\n", Options{})
		if v, ok := aeaViolacion(res.Scope, RuleTestsDelTestWriter, ""); ok {
			t.Fatalf("CA-639: %s: no es violacion de %s: %+v\n%s", c.nombre, RuleTestsDelTestWriter, v, out)
		}
		if res.Note == NotaTestsDelTestWriter {
			t.Fatalf("CA-639: %s: la nota no es la de %s: %+v\n%s", c.nombre, RuleTestsDelTestWriter, res, out)
		}
		if r := aeaRegistro(t, root, res.EnvelopeID); r.Note == NotaTestsDelTestWriter {
			t.Fatalf("CA-639: %s: el registro no lleva la nota de %s: %+v", c.nombre, RuleTestsDelTestWriter, r)
		}
		if c.entrega && (res.Scope.Cuts() || res.VerdictID == "") {
			t.Fatalf("CA-639: %s: el sobre no corta y verify corre: %+v\n%s", c.nombre, res, out)
		}
		raw, _ := os.ReadFile(filepath.Join(wt, aeaProtegido))
		if string(raw) != aeaTestTW {
			t.Fatalf("CA-639: %s: fixture: el test protegido queda igual: %q", c.nombre, raw)
		}
	}
}

// CA-639: un sobre de test-writer que cambia sus propios tests protegidos
// no viola la regla, y lo que trasplanta se suma a los protegidos.
func TestCA639_ElTestWriterCambiaLosSuyos(t *testing.T) {
	root, wt := aeaProtegida(t, true, "")
	res, out := aeaCorrer(t, root, "test-writer",
		"printf 'package app\\n\\n// CA-1 precios, segunda version\\n' > "+aeaProtegido+"\n"+
			"printf 'package app\\n// CA-1 ronda\\n' > ronda_test.go\nexit 0\n", Options{})
	if _, ok := aeaViolacion(res.Scope, RuleTestsDelTestWriter, ""); ok || res.Scope.Cuts() {
		t.Fatalf("CA-639: el test-writer cambia los suyos sin violar la regla: %+v\n%s", res.Scope, out)
	}
	raw, _ := os.ReadFile(filepath.Join(wt, aeaProtegido))
	if !strings.Contains(string(raw), "segunda version") {
		t.Fatalf("CA-639: fixture: el cambio del test-writer se trasplanta: %q", raw)
	}
	prot, err := TestsProtegidos(root, aeaTarea)
	if err != nil || !reflect.DeepEqual(prot, []string{aeaProtegido, "ronda_test.go"}) {
		t.Fatalf("CA-639: la union de los dos sobres, sin repetir: %q %v", prot, err)
	}
}

// CA-639: una tarea cuyos sobres de test-writer no traen trasplantados (los
// de antes de este spec) no protege nada: el writer cambia ese test sin
// cortar.
func TestCA639_SinTrasplantadosNoSeProtegeNada(t *testing.T) {
	root, wt := aeaProyecto(t, "true", "")
	write(t, wt, aeaProtegido, aeaTestTW)
	aeaGuardar(t, wt)
	ahora := time.Now().UTC().Add(-time.Hour)
	if err := envelope.Write(root, envelope.Record{ID: "20261112T090000_aea001", Role: "test-writer", Provider: "claude",
		Task: aeaTarea, Dir: wt, Stage: "ok", Step: 6, Steps: 6, Status: envelope.StatusDeliverable,
		StartedAt: ahora, UpdatedAt: ahora, EndedAt: ahora}); err != nil {
		t.Fatal(err)
	}
	if prot, err := TestsProtegidos(root, aeaTarea); err != nil || len(prot) != 0 {
		t.Fatalf("CA-639: sin trasplantados no hay protegidos: %q %v", prot, err)
	}
	res, out := aeaCorrer(t, root, "writer", "printf 'package app\\n// otro\\n' > "+aeaProtegido+"\nexit 0\n", Options{})
	if _, ok := aeaViolacion(res.Scope, RuleTestsDelTestWriter, ""); ok || res.Scope.Cuts() || res.VerdictID == "" {
		t.Fatalf("CA-639: sin trasplantados el writer no corta: %+v\n%s", res, out)
	}
}

// CA-639: TestsProtegidos es la union ordenada y sin repetir de los
// trasplantados de los sobres de test-writer de ESA tarea; ni otros roles,
// ni otras tareas, ni task "" protegen; un registro ilegible es un error.
func TestCA639_TestsProtegidosEsLaUnionDeLosDelTestWriter(t *testing.T) {
	root := repo(t)
	ahora := time.Now().UTC().Add(-time.Hour)
	for i, r := range []envelope.Record{
		{Role: "test-writer", Task: "demo", Trasplantados: []string{"b_test.go", "sub/a_test.go"}},
		{Role: "test-writer", Task: "demo", Trasplantados: []string{"a_test.go", "b_test.go"}},
		{Role: "test-writer", Task: "demo"},
		{Role: "test-writer", Task: "otra", Trasplantados: []string{"otra_test.go"}},
		{Role: "writer", Task: "demo", Trasplantados: []string{"del_writer_test.go"}},
		{Role: "test-writer", Trasplantados: []string{"sin_tarea_test.go"}},
	} {
		r.ID = "20261112T0" + string(rune('0'+i)) + "0000_aea10" + string(rune('0'+i))
		r.Provider, r.Dir, r.Stage, r.Status = "claude", root, "ok", envelope.StatusDeliverable
		r.StartedAt, r.UpdatedAt, r.EndedAt = ahora, ahora, ahora
		if err := envelope.Write(root, r); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(envelope.List(root)); n != 6 {
		t.Fatalf("fixture: seis registros: %d", n)
	}
	prot, err := TestsProtegidos(root, "demo")
	if err != nil || !reflect.DeepEqual(prot, []string{"a_test.go", "b_test.go", "sub/a_test.go"}) {
		t.Fatalf("CA-639: la union ordenada sin repetir de los test-writer de demo: %q %v", prot, err)
	}
	if prot, err := TestsProtegidos(root, ""); err != nil || len(prot) != 0 {
		t.Fatalf("CA-639: task \"\" no protege nada: %q %v", prot, err)
	}
	if prot, err := TestsProtegidos(root, "nadie"); err != nil || len(prot) != 0 {
		t.Fatalf("CA-639: una tarea sin sobres no protege nada: %q %v", prot, err)
	}
	if prot, err := TestsProtegidos(repo(t), "demo"); err != nil || len(prot) != 0 {
		t.Fatalf("CA-639: sin .hoom/envelopes no hay protegidos ni error: %q %v", prot, err)
	}
	// un registro con nombre de id que no se puede leer no es "nada protegido"
	write(t, root, ".hoom/"+envelope.DirName+"/20261112T090000_aea199.json", "{trunc")
	if _, err := TestsProtegidos(root, "demo"); err == nil {
		t.Fatal("CA-639: un registro ilegible es un error, no \"nada protegido\"")
	}
}

// CA-638/CA-639: FotoProtegidos lee el disco: el sha256 del contenido,
// ProtegidoAusente para lo que no esta, nunca sigue un symlink.
func TestCA638_FotoProtegidosLeeElDisco(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a_test.go", aeaTestTW)
	write(t, dir, "sub/b_test.go", "package sub\n")
	if err := os.Symlink(filepath.Join(dir, "a_test.go"), filepath.Join(dir, "enlace_test.go")); err != nil {
		t.Fatal(err)
	}
	foto := FotoProtegidos(dir, []string{"a_test.go", "sub/b_test.go", "falta_test.go", "enlace_test.go"})
	suma := sha256.Sum256([]byte(aeaTestTW))
	if len(foto) != 4 || foto["a_test.go"] != hex.EncodeToString(suma[:]) || foto["falta_test.go"] != ProtegidoAusente {
		t.Fatalf("CA-638: una entrada por ruta: sha256 del contenido y %q para la ausente: %v", ProtegidoAusente, foto)
	}
	if !strings.HasPrefix(foto["enlace_test.go"], HuellaNoRegular) || foto["enlace_test.go"] == foto["a_test.go"] {
		t.Fatalf("CA-638: un symlink no se sigue: %q", foto["enlace_test.go"])
	}
	if f := FotoProtegidos(dir, nil); len(f) != 0 {
		t.Fatalf("CA-638: sin rutas, una foto vacia: %v", f)
	}
}

// CA-638/CA-639: el gate, con la foto de antes, corta al writer que cambio,
// borro, renombro o creo un protegido aunque Allow lo permita; ignora a un
// rol de otra forma y a uno que no toco nada protegido.
func TestCA638_ElGateJuzgaLosProtegidos(t *testing.T) {
	writer, tw := edr1Rol(t, "writer"), edr1Rol(t, "test-writer")
	for _, c := range []struct {
		nombre string
		hacer  func(root string)
		corta  bool
	}{
		{"cambia", func(root string) { write(t, root, aeaProtegido, "package app\n// otro\n") }, true},
		{"borra", func(root string) { os.Remove(filepath.Join(root, aeaProtegido)) }, true},
		{"renombra", func(root string) {
			git(t, root, "mv", aeaProtegido, "renombrado_test.go")
			git(t, root, "commit", "-qm", "mv")
		}, true},
		{"mismo contenido", func(root string) { write(t, root, aeaProtegido, aeaTestTW) }, false},
		{"otro test", func(root string) { write(t, root, "otro_test.go", "package app\n") }, false},
	} {
		for _, rol := range []string{"writer", "test-writer"} {
			root := repo(t)
			write(t, root, aeaProtegido, aeaTestTW)
			git(t, root, "add", "-A")
			git(t, root, "commit", "-qm", "protegido")
			foto := FotoProtegidos(root, []string{aeaProtegido})
			r := writer
			if rol == "test-writer" {
				r = tw
			}
			pol := PolicyFor(nil, r)
			pol.Allow = append(pol.Allow, "**", aeaProtegido)
			pol.Protegidos = foto
			before := edr1Take(t, "CA-638", c.nombre, root)
			c.hacer(root)
			after := edr1Take(t, "CA-638", c.nombre, root)
			res := Gate(root, "main", "", r, before, after, pol, nil)
			_, viola := aeaViolacion(res, RuleTestsDelTestWriter, aeaProtegido)
			if quiere := c.corta && rol == "writer"; viola != quiere || (quiere && !res.Cuts()) {
				t.Fatalf("CA-638/CA-639: %s por el %s: viola %v (quiero %v), corta %v: %+v", c.nombre, rol, viola, quiere, res.Cuts(), res.Violations)
			}
		}
	}

	// un protegido ausente antes que el writer crea tambien cambio
	root := repo(t)
	pol := PolicyFor(nil, writer)
	pol.Protegidos = FotoProtegidos(root, []string{aeaProtegido})
	if pol.Protegidos[aeaProtegido] != ProtegidoAusente {
		t.Fatalf("fixture: %v", pol.Protegidos)
	}
	before := edr1Take(t, "CA-638", "creado", root)
	write(t, root, aeaProtegido, aeaTestTW)
	after := edr1Take(t, "CA-638", "creado", root)
	if res := Gate(root, "main", "", writer, before, after, pol, nil); !res.Cuts() {
		t.Fatalf("CA-638: el protegido que no estaba y el writer escribe cambio: %+v", res.Violations)
	}
	// sin Protegidos no hay nada que juzgar
	pol.Protegidos = nil
	before = edr1Take(t, "CA-638", "sin protegidos", root)
	write(t, root, aeaProtegido, "package app\n// otro\n")
	after = edr1Take(t, "CA-638", "sin protegidos", root)
	res := Gate(root, "main", "", writer, before, after, pol, nil)
	if v, ok := aeaViolacion(res, RuleTestsDelTestWriter, ""); ok {
		t.Fatalf("CA-639: sin Protegidos no hay violacion de la regla: %+v", v)
	}
}

// ---------------------------------------------------------------- CA-651

// CA-651: un run del sobre que muere por su timeout deja corte tiempo en el
// registro y en su sidecar, con el cierre de siempre (run, no-entregable,
// sin veredicto, exit distinto de 0); un run que falla de otra forma sale
// con su exit N (CA-143, aclaracion 12) y no lleva la clave.
func TestCA651_ElSobreCortadoPorTiempoLoDice(t *testing.T) {
	// aeaMeta lee el sidecar del run de la raiz o del arbol de la tarea.
	aeaMeta := func(id string, dirs ...string) (runcmd.Meta, bool) {
		for _, d := range dirs {
			if m, ok := runcmd.ReadMeta(d, id); ok {
				return m, true
			}
		}
		return runcmd.Meta{}, false
	}
	root, wt := aeaProyecto(t, "true", "")
	res, out := aeaCorrer(t, root, "writer", "exec sleep 60\n", Options{Timeout: 2 * time.Second})
	if res.Stage != "run" || res.Status != envelope.StatusNotDeliverable || res.ExitCode == 0 || res.VerdictID != "" {
		t.Fatalf("CA-651: el cierre no cambia: run, no-entregable, sin veredicto, exit distinto de 0: %+v\n%s", res, out)
	}
	if v, ok := aeaClaves(t, root, res.EnvelopeID)["corte"]; !ok || v != envelope.CorteTiempo {
		t.Fatalf("CA-651: el registro del sobre trae \"corte\": \"tiempo\": %v (%v)", v, ok)
	}
	if r := aeaRegistro(t, root, res.EnvelopeID); r.Corte != envelope.CorteTiempo || r.Stage != "run" {
		t.Fatalf("CA-651: Record.Corte: %+v", r)
	}
	if res.RunID == "" {
		t.Fatalf("CA-651: fixture: el sobre corrio un run: %+v", res)
	}
	if m, ok := aeaMeta(res.RunID, root, wt); !ok || m.Corte != runcmd.CorteTiempo {
		t.Fatalf("CA-651: el sidecar del run trae corte tiempo: %+v (%v)", m, ok)
	}

	root, wt = aeaProyecto(t, "true", "")
	res, out = aeaCorrer(t, root, "writer", "printf 'algo salio mal\\n' >&2\nexit 2\n", Options{Timeout: 20 * time.Second})
	if res.Stage != "run" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 2 || res.VerdictID != "" {
		t.Fatalf("CA-651/CA-143: el run que sale con 2 cierra en run, no-entregable, sin veredicto, y el sobre sale con 2: %+v\n%s", res, out)
	}
	if v, ok := aeaClaves(t, root, res.EnvelopeID)["corte"]; ok {
		t.Fatalf("CA-651: un run que falla sin timeout no lleva corte: %v", v)
	}
	if r := aeaRegistro(t, root, res.EnvelopeID); r.Corte != "" {
		t.Fatalf("CA-651: Record.Corte vacio en un run que falla sin timeout: %+v", r)
	}
	if res.RunID == "" {
		t.Fatalf("CA-651: fixture: el sobre corrio un run: %+v", res)
	}
	if m, ok := aeaMeta(res.RunID, root, wt); !ok || m.Corte != "" {
		t.Fatalf("CA-651: el sidecar de un run que falla sin timeout no lleva corte: %+v (%v)", m, ok)
	}
}

// ---------------------------------------------------------------- CA-661

// CA-661: lo que el piloto pone en Options llega al registro con sus claves;
// sin nada, el registro no trae ninguna.
func TestCA661_LasOpcionesDelPilotoLleganAlRegistro(t *testing.T) {
	root, _ := aeaProyecto(t, "true", "")
	pedido := "implementa el arreglo de la ronda 1"
	res, out := aeaCorrer(t, root, "writer", "printf 'package app // implementado\\n' > app.go\nexit 0\n", Options{
		Prompt: pedido, Pilot: true, Columna: "review", Regla: "ronda-writer", Intento: 1, Ronda: 1,
		Hallazgos: []string{"20261112T010101_f00001", "20261112T010101_f00002"}, Bloqueantes: 3})
	m := aeaClaves(t, root, res.EnvelopeID)
	want := map[string]any{"columna": "review", "pedido": pedido, "regla": "ronda-writer", "intento": 1.0, "ronda": 1.0,
		"hallazgos": []any{"20261112T010101_f00001", "20261112T010101_f00002"}, "bloqueantes": 3.0, "piloto": true}
	for k, v := range want {
		if !reflect.DeepEqual(m[k], v) {
			t.Fatalf("CA-661: el registro trae %q = %v; fue %v\n%v\n%s", k, v, m[k], m, out)
		}
	}
	r := aeaRegistro(t, root, res.EnvelopeID)
	if r.Columna != "review" || r.Pedido != pedido || r.Regla != "ronda-writer" || r.Ronda != 1 || r.Bloqueantes != 3 || !r.Pilot {
		t.Fatalf("CA-661: Record: %+v", r)
	}

	// un hoom agent directo: ni columna ni las claves del piloto
	res, _ = aeaCorrer(t, root, "scout", "exit 0\n", Options{Prompt: "explora"})
	m = aeaClaves(t, root, res.EnvelopeID)
	for _, k := range []string{"columna", "regla", "intento", "ronda", "hallazgos", "bloqueantes", "piloto"} {
		if v, ok := m[k]; ok {
			t.Fatalf("CA-661: un sobre directo no lleva %q: %v", k, v)
		}
	}
}
