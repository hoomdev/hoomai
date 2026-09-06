// Tests adversariales del spec .hoom/specs/codex-v2-y-review-cruzada.md:
// la review cruzada. Lo que hoom afirma de una review sale de la evidencia —
// la lente del cambio, quien escribio, y los hallazgos que VE aparecer.
package reviewcmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func write(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// repo arma un proyecto real con un cambio de CODIGO sin commitear: hay algo
// que revisar y una lente que calcular.
func repo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-m", "inicial")
	write(t, root, "app.go", "package app\n\nfunc Nuevo() {}\n")
	return root
}

// fakeProvider pone al frente del PATH un CLI de IA falso.
func fakeProvider(t *testing.T, name, script string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\n"+script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

// metaDeRun deja el rastro que deja un run real: quien corrio y con que rol.
func metaDeRun(t *testing.T, root, id, provider, role string, cuando time.Time) {
	t.Helper()
	raw, _ := json.MarshalIndent(runcmd.Meta{
		ID: id, Provider: provider, Role: role, Dir: root,
		CreatedAt: cuando, Status: "done",
	}, "", "  ")
	write(t, root, filepath.Join(".hoom", "runs", id+".meta.json"), string(raw))
}

// hallazgoFalso es lo que escribe un reviewer real cuando corre
// `hoom finding add`, en el formato exacto del artefacto.
func hallazgoFalso(root, id, autor string) string {
	return "mkdir -p " + filepath.Join(root, ".hoom", "findings") + "\n" +
		"cat > " + filepath.Join(root, ".hoom", "findings", id+".json") + " <<'EOF'\n" +
		`{"id":"` + id + `","created_at":"2026-09-04T21:00:00Z","severity":"medium",` +
		`"lens":"reliability","file":"app.go","description":"Nuevo() no tiene test","author":"` + autor + `"}` + "\n" +
		"EOF\n"
}

func verdictosEn(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".hoom", "verdicts"))
	if err != nil {
		return 0
	}
	return len(entries)
}

// CA-160: las lentes salen de la evidencia y se fijan ANTES del primer run.
func TestCA160_LentesDeterministas(t *testing.T) {
	// explicita manda
	lentes, motivo, err := Lenses(gitx.Info{ChangedFiles: []string{"a.go"}}, "risk")
	if err != nil || len(lentes) != 1 || lentes[0] != "risk" || motivo == "" {
		t.Fatalf("CA-160: la lente pedida a mano manda: %v %q %v", lentes, motivo, err)
	}
	if _, _, err := Lenses(gitx.Info{ChangedFiles: []string{"a.go"}}, "profundidad"); err == nil ||
		!strings.Contains(err.Error(), "readability") {
		t.Fatalf("CA-160: una lente invalida es error listando las 4: %v", err)
	}

	// solo documentacion: cero lentes, ningun CLI
	for _, docs := range [][]string{
		{"README.md"}, {".hoom/specs/algo.md", "docs/guia.txt"}, {"LICENSE"},
	} {
		lentes, motivo, _ = Lenses(gitx.Info{ChangedFiles: docs, Insertions: 900}, "")
		if len(lentes) != 0 || motivo == "" {
			t.Fatalf("CA-160: %v es solo documentacion: %v (%s)", docs, lentes, motivo)
		}
	}
	if lentes, _, _ = Lenses(gitx.Info{}, ""); len(lentes) != 0 {
		t.Fatalf("CA-160: sin cambios no hay nada que revisar: %v", lentes)
	}

	// riesgo: las 4
	lentes, motivo, _ = Lenses(gitx.Info{ChangedFiles: []string{"internal/auth/login.go"}, Insertions: 3}, "")
	if len(lentes) != 4 || strings.Join(lentes, ",") != strings.Join(Lentes, ",") {
		t.Fatalf("CA-160: una ruta de riesgo pide las 4 lentes en orden: %v", lentes)
	}
	if !strings.Contains(motivo, "login.go") {
		t.Fatalf("CA-160: el motivo nombra la ruta que lo disparo: %q", motivo)
	}

	// tamano: las 4, con el umbral del contrato 06 (401 pasa, 400 no)
	if lentes, _, _ = Lenses(gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 400, Deletions: 1}, ""); len(lentes) != 4 {
		t.Fatalf("CA-160: 401 lineas pide las 4 lentes: %v", lentes)
	}
	lentes, motivo, _ = Lenses(gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 300, Deletions: 100}, "")
	if len(lentes) != 1 || lentes[0] != LenteDominante {
		t.Fatalf("CA-160: 400 lineas es cambio estandar: %v (%s)", lentes, motivo)
	}
}

// CA-161: la review es cruzada por construccion, y cuando no puede serlo lo
// dice en vez de fingirlo.
func TestCA161_ReviewCruzada(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")

	// sin run previo: desconocida, y la review corre igual
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk"}, &out)
	if err != nil {
		t.Fatalf("CA-161: %v\n%s", err, out.String())
	}
	if res.Cross != CrossUnknown || res.Writer != "" || res.ExitCode != 0 {
		t.Fatalf("CA-161: sin meta previo la cruzada es desconocida: %+v\n%s", res, out.String())
	}

	// con un writer en claude: se elige otro provider sin que nadie lo pida
	metaDeRun(t, root, "20260904T190000_aaaaaa", "claude", "writer", time.Now())
	out.Reset()
	res, err = Run(root, "main", Options{Lens: "risk"}, &out)
	if err != nil {
		t.Fatalf("CA-161: %v\n%s", err, out.String())
	}
	if res.Writer != "claude" || res.Provider != "codex" || res.Cross != CrossYes {
		t.Fatalf("CA-161: el reviewer se elige distinto del writer: %+v\n%s", res, out.String())
	}

	// una review anterior no es el writer: el rol de solo lectura se saltea
	metaDeRun(t, root, "20260904T193000_bbbbbb", "codex", "reviewer", time.Now().Add(time.Minute))
	out.Reset()
	res, _ = Run(root, "main", Options{Lens: "risk"}, &out)
	if res.Writer != "claude" || res.Provider != "codex" {
		t.Fatalf("CA-161: una review previa no cuenta como writer: %+v", res)
	}

	// forzar el mismo provider que escribio: se niega, sin lanzar ningun CLI
	out.Reset()
	res, err = Run(root, "main", Options{Lens: "risk", Provider: "claude"}, &out)
	if err != nil {
		t.Fatalf("CA-161: %v", err)
	}
	if res.Cross != CrossNo || res.ExitCode != 1 || len(res.Passes) != 0 {
		t.Fatalf("CA-161: el mismo modelo que escribio no revisa: %+v\n%s", res, out.String())
	}
	if !strings.Contains(out.String(), "--same-provider") {
		t.Fatalf("CA-161: la negativa trae el escape declarado: %s", out.String())
	}

	// ...y con el escape explicito corre, marcada como no cruzada
	out.Reset()
	res, err = Run(root, "main", Options{Lens: "risk", Provider: "claude", SameProvider: true}, &out)
	if err != nil {
		t.Fatalf("CA-161: %v", err)
	}
	if res.Cross != CrossNo || res.ExitCode != 0 || len(res.Passes) != 1 {
		t.Fatalf("CA-161: con --same-provider corre y queda marcada: %+v\n%s", res, out.String())
	}
}

// CA-162: el reviewer corre con su contrato, su limite de solo lectura y el
// gate de scope de la forma evidencia.
func TestCA162_ElReviewerEsUnRol(t *testing.T) {
	root := repo(t)
	// el fake imprime su argv: se puede leer que le llego
	fakeProvider(t, "codex", "echo \"argv: $@\" > "+filepath.Join(root, "argv.txt")+"\nexit 0\n")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "reliability", Provider: "codex"}, &out)
	if err != nil {
		t.Fatalf("CA-162: %v\n%s", err, out.String())
	}
	argv, err := os.ReadFile(filepath.Join(root, "argv.txt"))
	if err != nil {
		t.Fatal(err)
	}
	linea := string(argv)
	if !strings.Contains(linea, "developer_instructions=") || !strings.Contains(linea, "# Reviewer") {
		t.Fatalf("CA-162: el contrato del rol viaja como system prompt: %s", linea)
	}
	if !strings.Contains(linea, `sandbox_mode="workspace-write"`) {
		t.Fatalf("CA-162: el reviewer es solo lectura CON exec: %s", linea)
	}
	if !strings.Contains(linea, "hoom finding add") || !strings.Contains(linea, "--author reviewer@codex") {
		t.Fatalf("CA-162: el pedido trae el comando exacto de registro con su procedencia: %s", linea)
	}
	if !strings.Contains(linea, "lente reliability") {
		t.Fatalf("CA-162: el pedido nombra la lente asignada: %s", linea)
	}

	// el argv.txt que escribio el propio fake queda fuera de .hoom/: es una
	// escritura fuera del territorio del rol y el gate la caza
	if res.ExitCode != 1 || len(res.Passes) != 1 || res.Passes[0].Scope.OK {
		t.Fatalf("CA-162: una escritura fuera de .hoom/ es violacion: %+v\n%s", res, out.String())
	}
	v := res.Passes[0].Scope.Violations
	if len(v) != 1 || v[0].Path != "argv.txt" || v[0].FindingID == "" {
		t.Fatalf("CA-162: la violacion nombra la ruta y deja hallazgo: %+v", v)
	}
}

// CA-163: el resultado del reviewer son los hallazgos que hoom VE aparecer, y
// los que escribe hoom por violaciones no cuentan como suyos.
func TestCA163_HallazgosObservados(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "codex", hallazgoFalso(root, "20260904T210000_abcdef", "reviewer@codex")+"exit 0\n")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "reliability", Provider: "codex"}, &out)
	if err != nil {
		t.Fatalf("CA-163: %v\n%s", err, out.String())
	}
	if len(res.Findings) != 1 || res.Findings[0] != "20260904T210000_abcdef" {
		t.Fatalf("CA-163: el hallazgo que aparecio durante el run es el resultado: %+v\n%s", res, out.String())
	}
	if res.ExitCode != 0 || res.Status != "revisado" {
		t.Fatalf("CA-163: un hallazgo no cambia el exit: es narracion calificada, no un gate: %+v", res)
	}
	if !strings.Contains(out.String(), "20260904T210000_abcdef") {
		t.Fatalf("CA-163: la salida nombra los hallazgos nuevos: %s", out.String())
	}

	// cero hallazgos se dice, no se calla
	root2 := repo(t)
	fakeProvider(t, "codex", "exit 0\n")
	out.Reset()
	res, err = Run(root2, "main", Options{Lens: "reliability", Provider: "codex"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) != 0 || !strings.Contains(out.String(), "0 nuevos") {
		t.Fatalf("CA-163: una review sin hallazgos lo dice: %+v\n%s", res, out.String())
	}

	// un reviewer que escribe codigo: el hallazgo que escribe HOOM por la
	// violacion no se le acredita al reviewer
	root3 := repo(t)
	fakeProvider(t, "codex", "printf 'package app\\n' > "+filepath.Join(root3, "colado.go")+"\nexit 0\n")
	out.Reset()
	res, err = Run(root3, "main", Options{Lens: "reliability", Provider: "codex"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Passes) != 1 || len(res.Passes[0].Scope.Violations) != 1 {
		t.Fatalf("CA-163: la escritura de codigo es violacion: %+v", res)
	}
	if len(res.Findings) != 0 {
		t.Fatalf("CA-163: el hallazgo del gate no es un hallazgo del reviewer: %v", res.Findings)
	}
}

// CA-164: la review no re-verifica ni escribe veredictos: el reviewer no
// cambia el arbol certificado.
func TestCA164_LaReviewNoVerifica(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "codex", "exit 0\n")
	antes := verdictosEn(t, root)

	res, err := Run(root, "main", Options{Provider: "codex"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("CA-164: %+v", res)
	}
	if despues := verdictosEn(t, root); despues != antes {
		t.Fatalf("CA-164: la review no emite veredicto: %d -> %d", antes, despues)
	}
}

// CA-165: exit codes y --json.
func TestCA165_ExitCodesYJSON(t *testing.T) {
	// las 4 lentes y la SEGUNDA pasada falla: quedan las que corrieron. El
	// contador vive FUERA del repo: un fake que escribiera adentro seria una
	// violacion de scope y cortaria en la primera pasada.
	root := repo(t)
	write(t, root, "internal/auth/login.go", "package auth\n")
	contador := filepath.Join(t.TempDir(), "n.txt")
	fakeProvider(t, "codex", "n=$(cat "+contador+" 2>/dev/null || echo 0)\n"+
		"n=$((n+1)); echo $n > "+contador+"\n"+
		"if [ $n -eq 2 ]; then exit 7; fi\nexit 0\n")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Provider: "codex"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Lenses) != 4 {
		t.Fatalf("CA-165: una ruta de riesgo pide las 4 lentes: %+v", res.Lenses)
	}
	if res.ExitCode != 1 || res.Status != "no-entregable" {
		t.Fatalf("CA-165: un run fallido cierra en rojo: %+v\n%s", res, out.String())
	}
	if len(res.Passes) != 2 || res.Passes[0].RunStatus != "done" || res.Passes[1].RunStatus == "done" {
		t.Fatalf("CA-165: quedan las pasadas que corrieron, con la que fallo: %+v", res.Passes)
	}

	// el JSON trae el cuadro completo
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{`"provider"`, `"cross"`, `"reason"`, `"lenses"`, `"passes"`,
		`"findings"`, `"status"`, `"exit_code"`, `"scope"`, `"run_id"`} {
		if !strings.Contains(string(raw), k) {
			t.Fatalf("CA-165: al JSON le falta %s: %s", k, raw)
		}
	}

	// cero lentes: no se lanza ningun CLI y el exit es 0
	root3 := t.TempDir()
	git(t, root3, "init", "-b", "main")
	git(t, root3, "config", "user.email", "test@hoom.dev")
	git(t, root3, "config", "user.name", "hoom test")
	write(t, root3, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, root3, "app.go", "package app\n")
	git(t, root3, "add", "-A")
	git(t, root3, "commit", "-m", "inicial")
	write(t, root3, "README.md", "solo documentacion\n")
	fakeProvider(t, "codex", "exit 9\n") // si se lanzara, el exit lo delataria
	out.Reset()
	res, err = Run(root3, "main", Options{Provider: "codex"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 || res.Status != "sin-revisar" || len(res.Passes) != 0 {
		t.Fatalf("CA-165: solo documentacion no gasta una sesion: %+v\n%s", res, out.String())
	}
}
