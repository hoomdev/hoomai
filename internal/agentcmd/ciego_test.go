// Tests adversariales del spec .hoom/specs/test-writer-en-arbol-ciego.md:
// el sobre con un rol ciego. Seis pasos, una cuarentena y un trasplante que
// solo mueve lo que el gate aprobo.
package agentcmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/isolate"
)

// repoCiego arma un proyecto donde un rol ciego tiene de que agarrarse: un
// spec, un test existente y una implementacion que NO debe ver.
func repoCiego(t *testing.T) string {
	t.Helper()
	root := repo(t)
	write(t, root, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\n")
	write(t, root, "app_test.go", "package app\n\n// CA-1\n")
	write(t, root, "viejo_test.go", "package app\n\n// CA-1 obsoleto\n")
	specDemo(t, root)
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "spec y test iniciales")
	return root
}

// specAprobado deja el spec commiteado Y aprobado, que es el unico estado en
// el que un rol que escribe llega al paso de aislamiento.
func specAprobado(t *testing.T, root string) string {
	t.Helper()
	spec := ".hoom/specs/demo.md"
	if _, _, err := approval.Approve(root, filepath.Join(root, spec)); err != nil {
		t.Fatal(err)
	}
	return spec
}

func existeEn(t *testing.T, path string) bool {
	t.Helper()
	_, err := os.Lstat(path)
	return err == nil
}

// CA-179: sin git no hay arbol ciego, y el sobre se niega en vez de degradar.
func TestCA179_SinGitNoHayRolCiego(t *testing.T) {
	fakeProvider(t, "claude", "exit 0\n")

	sinGit := t.TempDir()
	write(t, sinGit, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	var buf bytes.Buffer
	res, err := Run(sinGit, "main", Options{Role: "test-writer", Prompt: "escribi tests"}, &buf)
	if err != nil {
		t.Fatalf("CA-179: el sobre reporta, no explota: %v", err)
	}
	if res.Stage != "aislar" || res.ExitCode != 1 || res.RunID != "" {
		t.Fatalf("CA-179: debe cortar en aislar sin correr nada: %+v", res)
	}
	if !strings.Contains(buf.String(), "hoom run") {
		t.Fatalf("CA-179: la salida debe nombrar la via sin garantia:\n%s", buf.String())
	}
	if runsCount(t, sinGit) != 0 {
		t.Fatal("CA-179: no puede quedar run ni log")
	}

	sinCommits := t.TempDir()
	write(t, sinCommits, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	git(t, sinCommits, "init", "-b", "main")
	buf.Reset()
	res, err = Run(sinCommits, "main", Options{Role: "test-writer", Prompt: "escribi tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "aislar" || res.ExitCode != 1 {
		t.Fatalf("CA-179: un repo sin commits tampoco puede armar el arbol: %+v", res)
	}
	if !strings.Contains(buf.String(), "git add -A && git commit") {
		t.Fatalf("CA-179: falta la accion exacta:\n%s", buf.String())
	}

	// un rol NO ciego corre igual fuera de git: la exigencia es del rol
	res, err = Run(sinGit, "main", Options{Role: "scout", Prompt: "explora"}, io.Discard)
	if err != nil || res.Stage == "aislar" {
		t.Fatalf("CA-179: el scout no exige arbol ciego: %+v %v", res, err)
	}
}

// CA-180: el arbol ciego se arma desde un COMMIT, asi que un spec que no esta
// en el commit no existe para el rol.
func TestCA180_SpecSinCommitear(t *testing.T) {
	fakeProvider(t, "claude", "exit 0\n")

	// existe en disco pero no en HEAD
	root := repo(t)
	write(t, root, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\n")
	spec := specDemo(t, root)
	specAprobado(t, root)
	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "test-writer", Spec: spec, Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "aislar" || res.ExitCode != 1 || res.RunID != "" {
		t.Fatalf("CA-180: un spec fuera del commit corta en aislar: %+v", res)
	}
	if !strings.Contains(buf.String(), "git add") || !strings.Contains(buf.String(), "git commit") {
		t.Fatalf("CA-180: falta la accion exacta:\n%s", buf.String())
	}
	if existeEn(t, filepath.Join(root, ".hoom", "isolated")) {
		t.Fatal("CA-180: corta ANTES de armar el arbol ciego")
	}

	// commiteado pero editado despues: el rol veria la version vieja
	root2 := repoCiego(t)
	write(t, root2, ".hoom/specs/demo.md", "# Spec demo editado\n\n## Objetivo\nOtra cosa.\n\n"+
		"## No-goals\nNada.\n\n## Contratos\nNinguno.\n\n## Casos limite\nNinguno.\n\n"+
		"## Criterios de aceptacion\n- CA-1: corre. [verifica: true]\n\n## Decisiones\nNinguna.\n\n## Riesgos\nNinguno.\n")
	specAprobado(t, root2) // se aprueba el contenido NUEVO: la aprobacion esta vigente
	buf.Reset()
	res, err = Run(root2, "main", Options{Role: "test-writer", Spec: ".hoom/specs/demo.md", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "aislar" || res.ExitCode != 1 {
		t.Fatalf("CA-180: un spec que difiere del commiteado corta en aislar: %+v\n%s", res, buf.String())
	}
	if !strings.Contains(buf.String(), "difiere del commiteado") {
		t.Fatalf("CA-180: el mensaje debe decir por que:\n%s", buf.String())
	}
}

// CA-181: un run activo en el arbol de trabajo corta antes de armar nada: el
// trasplante pisaria ediciones en curso.
func TestCA181_RunActivoEnElArbolDeTrabajo(t *testing.T) {
	fakeProvider(t, "claude", "exit 0\n")
	root := repoCiego(t)
	write(t, root, ".hoom/runs/20260905T000000_aaaaaa.meta.json",
		`{"id":"20260905T000000_aaaaaa","provider":"claude","dir":`+
			jsonStr(root)+`,"status":"running","exit_code":-1}`)

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "aislar" || res.ExitCode != 1 {
		t.Fatalf("CA-181: un run activo corta en aislar: %+v", res)
	}
	if !strings.Contains(buf.String(), "20260905T000000_aaaaaa") {
		t.Fatalf("CA-181: el corte debe nombrar el run activo:\n%s", buf.String())
	}
	if existeEn(t, filepath.Join(root, ".hoom", "isolated")) {
		t.Fatal("CA-181: corta ANTES de armar el arbol ciego")
	}
}

func jsonStr(s string) string {
	raw, _ := json.Marshal(s)
	return string(raw)
}

// CA-182: seis pasos con un rol ciego, cinco con los demas; la linea de
// aislamiento nombra la cuarentena, el commit y cuanto quedo fuera; y el
// contrato del rol viaja con el parrafo que explica el arbol.
func TestCA182_SeisPasosYElParrafoDelArbolCiego(t *testing.T) {
	dump := filepath.Join(t.TempDir(), "args.txt")
	// el rol entrega un test: sin entrega no habria pasos 5 y 6 que mostrar
	// (.hoom/specs/arquitecto-bajo-el-sobre.md)
	fakeProvider(t, "claude", "printf '%s\\n' \"$@\" > "+dump+"\nprintf 'package app\\n// CA-1\\n' > nuevo_test.go\nexit 0\n")
	root := repoCiego(t)

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, quiere := range []string{"[1/6] spec", "[2/6] aislar", "[3/6] run", "[4/6] scope", "[5/6] verify", "[6/6] check"} {
		if !strings.Contains(out, quiere) {
			t.Fatalf("CA-182: falta el paso %q en:\n%s", quiere, out)
		}
	}
	if res.Isolation == nil || res.Isolation.Hidden == 0 {
		t.Fatalf("CA-182: el aislamiento debe reportar cuanto quedo fuera: %+v", res.Isolation)
	}
	if !strings.Contains(out, shortSHA(res.Isolation.Commit)) {
		t.Fatalf("CA-182: la linea de aislar nombra el commit:\n%s", out)
	}
	if !strings.Contains(out, ".hoom/isolated/test-writer_") {
		t.Fatalf("CA-182: la linea de aislar nombra la cuarentena:\n%s", out)
	}

	// el system prompt que recibio el provider: contrato + parrafo
	raw, err := os.ReadFile(dump)
	if err != nil {
		t.Fatalf("CA-182: el provider falso debia registrar sus argumentos: %v", err)
	}
	got := string(raw)
	if !strings.Contains(got, "Regla de oro") {
		t.Fatalf("CA-182: el contrato del rol debe seguir viajando entero:\n%s", got)
	}
	if !strings.Contains(got, "Este arbol es CIEGO") || !strings.Contains(got, shortSHA(res.Isolation.Commit)) {
		t.Fatalf("CA-182: falta el parrafo del arbol ciego con su commit:\n%s", got)
	}

	// el contrato en disco NO se toco: el subagente nativo no corre ciego
	contrato, err := os.ReadFile(filepath.Join(root, ".hoom", "agents", "05-test-writer.md"))
	if err == nil && strings.Contains(string(contrato), "CIEGO") {
		t.Fatal("CA-182: el parrafo se agrega al run, no al contrato")
	}

	// un rol no ciego mantiene los cinco pasos
	buf.Reset()
	if _, err := Run(repo(t), "main", Options{Role: "scout", Prompt: "explora"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[1/5] spec") || strings.Contains(buf.String(), "aislar") {
		t.Fatalf("CA-182: un rol no ciego sigue teniendo cinco pasos:\n%s", buf.String())
	}
}

// CA-183: un testigo de vuelta en el disco rompe el aislamiento: no hay
// veredicto, no hay trasplante, y la cuarentena queda para mirarla.
func TestCA183_TestigoQueVuelve(t *testing.T) {
	// el rol devuelve app.go con el contenido EXACTO de HEAD, que es el caso
	// en el que git no dice nada.
	fakeProvider(t, "claude", "printf 'package app\\n' > app.go\nprintf 'package app\\n// CA-1\\n' > nuevo_test.go\nexit 0\n")
	root := repoCiego(t)

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scope.Broken() || !res.Scope.Cuts() {
		t.Fatalf("CA-183: devolver un testigo rompe el aislamiento: %+v", res.Scope)
	}
	if res.Stage != "scope" || res.ExitCode != 1 || res.Status != "no-entregable" {
		t.Fatalf("CA-183: debe cortar en scope: %+v", res)
	}
	if res.VerdictID != "" || res.Check != nil {
		t.Fatalf("CA-183: no se emite veredicto sobre este arbol: %+v", res)
	}
	if existeEn(t, filepath.Join(root, "nuevo_test.go")) {
		t.Fatal("CA-183: sin aislamiento no se trasplanta nada")
	}
	if res.Isolation == nil || !res.Isolation.Kept || !existeEn(t, res.Isolation.Dir) {
		t.Fatalf("CA-183: la cuarentena se conserva: %+v", res.Isolation)
	}
	if !strings.Contains(buf.String(), "worktree remove --force") {
		t.Fatalf("CA-183: hay que decir como descartarla:\n%s", buf.String())
	}

	var v Violation
	for _, x := range res.Scope.Violations {
		if x.Rule == RuleIsolation {
			v = x
		}
	}
	if v.Path != "app.go" || v.Detail == "" || v.FindingID == "" {
		t.Fatalf("CA-183: la violacion nombra la ruta y deja hallazgo: %+v", res.Scope.Violations)
	}
	items, _, err := finding.List(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	var visto bool
	for _, it := range items {
		if it.Finding.ID == v.FindingID {
			visto = true
			if it.Finding.Severity != "high" || it.Finding.Lens != "risk" || it.Finding.File != "app.go" {
				t.Fatalf("CA-183: el hallazgo debe ser high/risk con la ruta: %+v", it.Finding)
			}
		}
	}
	if !visto {
		t.Fatalf("CA-183: el hallazgo %s no quedo registrado", v.FindingID)
	}
}

// CA-184: escribir en el arbol REAL durante un run ciego es fuga, y corta
// igual; lo que escribe HOOM mientras tanto no cuenta.
func TestCA184_FugaAlArbolReal(t *testing.T) {
	fakeProvider(t, "claude", "printf 'package app // tocado\\n' > ../../../app.go\nexit 0\n")
	root := repoCiego(t)

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scope.Broken() || res.Stage != "scope" || res.ExitCode != 1 {
		t.Fatalf("CA-184: salir de la cuarentena rompe el aislamiento: %+v\n%s", res.Scope, buf.String())
	}
	var v Violation
	for _, x := range res.Scope.Violations {
		if x.Rule == RuleIsolation {
			v = x
		}
	}
	if v.Path != "app.go" || !strings.Contains(v.Detail, "fuera de la cuarentena") {
		t.Fatalf("CA-184: la fuga tiene su propio detalle: %+v", res.Scope.Violations)
	}
	if res.VerdictID != "" {
		t.Fatal("CA-184: tampoco se emite veredicto")
	}

	// lo que hoom escribe mientras corre no se le imputa al rol
	for _, p := range []string{".hoom/runs/", ".hoom/cache/", ".hoom/worktrees/", ".hoom/isolated/"} {
		if !hoomOwn(p + "x.json") {
			t.Fatalf("CA-184: %s es de hoom, no del rol", p)
		}
	}
}

// CA-185: fuera de scope no corta, pero tampoco viaja: la ruta violadora se
// queda en la cuarentena y el arbol certificable nunca la ve.
func TestCA185_FueraDeScopeSeQuedaEnLaCuarentena(t *testing.T) {
	fakeProvider(t, "claude",
		"printf 'package app\\n// CA-1\\n' > nuevo_test.go\nprintf 'package nuevo\\n' > nuevo.go\nexit 0\n")
	root := repoCiego(t)

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if res.Scope.Broken() || res.Scope.OK {
		t.Fatalf("CA-185: nuevo.go esta fuera de scope y no es aislamiento: %+v", res.Scope)
	}
	if res.VerdictID == "" || res.Check == nil {
		t.Fatalf("CA-185: verify y check corren igual: %+v", res)
	}
	if res.Stage != "scope" || res.Status != "no-entregable" || res.ExitCode != 1 {
		t.Fatalf("CA-185: el cierre es rojo y el paso culpable es scope: %+v", res)
	}
	if !existeEn(t, filepath.Join(root, "nuevo_test.go")) {
		t.Fatal("CA-185: lo aprobado por el gate si viaja")
	}
	if existeEn(t, filepath.Join(root, "nuevo.go")) {
		t.Fatal("CA-185: lo que violo el scope NO viaja al arbol real")
	}
	if !res.Isolation.Kept || !existeEn(t, filepath.Join(res.Isolation.Dir, "nuevo.go")) {
		t.Fatalf("CA-185: se queda en la cuarentena, que se conserva: %+v", res.Isolation)
	}
}

// CA-186: el camino limpio. Todo el delta viaja, la cuarentena se cierra y
// verify corre en el arbol de trabajo sobre los tests trasplantados.
func TestCA186_CaminoLimpioYTrasplante(t *testing.T) {
	fakeProvider(t, "claude", "printf 'package app\\n// CA-1 nuevo\\n' > nuevo_test.go\n"+
		"printf 'package app\\n// CA-1 reescrito\\n' > app_test.go\n"+
		"rm viejo_test.go\nexit 0\n")
	root := repoCiego(t)

	var buf bytes.Buffer
	res, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Scope.OK {
		t.Fatalf("CA-186: nada de esto viola el scope tests: %+v\n%s", res.Scope.Violations, buf.String())
	}
	if res.Stage != "ok" || res.Status != "entregable" || res.ExitCode != 0 {
		t.Fatalf("CA-186: con verify y check verdes el sobre entrega: %+v\n%s", res, buf.String())
	}
	iso := res.Isolation
	if strings.Join(iso.Applied, ",") != "app_test.go,nuevo_test.go" {
		t.Fatalf("CA-186: altas y sobrescrituras trasplantadas: %v", iso.Applied)
	}
	if strings.Join(iso.Removed, ",") != "viejo_test.go" {
		t.Fatalf("CA-186: la baja tambien viaja: %v", iso.Removed)
	}
	if iso.Kept || existeEn(t, iso.Dir) {
		t.Fatalf("CA-186: sin violaciones la cuarentena se cierra: %+v", iso)
	}
	raw, err := os.ReadFile(filepath.Join(root, "app_test.go"))
	if err != nil || !strings.Contains(string(raw), "reescrito") {
		t.Fatalf("CA-186: el arbol real recibe el trabajo del rol: %q %v", raw, err)
	}
	if existeEn(t, filepath.Join(root, "viejo_test.go")) {
		t.Fatal("CA-186: lo que el rol borro se borra tambien en el arbol real")
	}
	if res.VerdictID == "" || res.Verdict != "green" || res.Check == nil || !res.Check.OK {
		t.Fatalf("CA-186: verify y check cierran verdes sobre el arbol ya trasplantado: %+v", res)
	}
}

// CA-187: el JSON lleva el bloque de aislamiento, y solo cuando hubo uno.
func TestCA187_JSONDelAislamiento(t *testing.T) {
	fakeProvider(t, "claude", "printf 'package app\\n// CA-1\\n' > nuevo_test.go\nexit 0\n")
	root := repoCiego(t)
	res, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	iso, ok := got["isolation"].(map[string]any)
	if !ok {
		t.Fatalf("CA-187: falta el bloque isolation: %s", raw)
	}
	for _, k := range []string{"dir", "commit", "patterns", "hidden", "applied", "kept"} {
		if _, ok := iso[k]; !ok {
			t.Fatalf("CA-187: falta %q en el bloque isolation: %v", k, iso)
		}
	}
	if res.ExitCode != 0 {
		t.Fatalf("CA-187: el JSON respeta el mismo exit que el texto: %+v", res)
	}

	res2, err := Run(repo(t), "main", Options{Role: "scout", Prompt: "explora"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	raw2, _ := json.Marshal(res2)
	if strings.Contains(string(raw2), "isolation") {
		t.Fatalf("CA-187: un rol no ciego no lleva el bloque: %s", raw2)
	}
}

// CA-188: la cuarentena no ensucia lo que hoom certifica.
func TestCA188_LaCuarentenaNoEntraEnLaHuella(t *testing.T) {
	root := repoCiego(t)
	antes := gitx.Snapshot(root, "main")

	tree, err := isolate.Open(root, "test-writer_huella", isolate.Patterns([]string{".hoom/**", "**/*_test.go"}, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer tree.Close()
	write(t, tree.Dir, "otro_test.go", "package app\n")

	despues := gitx.Snapshot(root, "main")
	if antes.ChangeFingerprint != despues.ChangeFingerprint {
		t.Fatalf("CA-188: abrir una cuarentena no puede mover la huella: %s -> %s",
			antes.ChangeFingerprint, despues.ChangeFingerprint)
	}
	for _, f := range despues.ChangedFiles {
		if strings.HasPrefix(f, ".hoom/isolated/") {
			t.Fatalf("CA-188: la cuarentena entro en el candidato de cambio: %v", despues.ChangedFiles)
		}
	}
	gi, err := os.ReadFile(filepath.Join(root, ".hoom", ".gitignore"))
	if err != nil || !strings.Contains(string(gi), "isolated/") {
		t.Fatalf("CA-188: .hoom/.gitignore debe esconder isolated/: %q %v", gi, err)
	}

	// y si el proyecto no lo tenia, hoom lo deja escrito
	limpio := repo(t)
	tree2, err := isolate.Open(limpio, "test-writer_gi", isolate.Patterns([]string{"hoom.yaml"}, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer tree2.Close()
	gi2, err := os.ReadFile(filepath.Join(limpio, ".hoom", ".gitignore"))
	if err != nil || !strings.Contains(string(gi2), "isolated/") {
		t.Fatalf("CA-188: hoom debe crear la regla si falta: %q %v", gi2, err)
	}
}
