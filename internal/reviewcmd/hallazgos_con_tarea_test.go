// Tests adversariales del contrato "review-hallazgos-con-tarea" sobre CA-293
// (.hoom/specs/items-y-columna-derivada.md: el task de la review sale de
// --task o del --spec) y CA-243 (.hoom/specs/arquitecto-bajo-el-sobre.md: el
// proceso del provider recibe HOOM_TASK). 'hoom review --spec <slug>' sin
// --task ata su registro a <slug>; los hallazgos que el reviewer registra con
// 'hoom finding add' durante su run solo se atan a la misma tarjeta si el
// reviewer ve HOOM_TASK=<slug>. Providers falsos en el PATH.
package reviewcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const (
	htSlug = "items-y-columna-derivada"
	htSpec = ".hoom/specs/" + htSlug + ".md"
)

// htPasada es lo que vio UNA invocacion del reviewer.
type htPasada struct {
	definida bool
	valor    string
	dir      string // cwd fisico
}

// htReviewer pone codex y claude falsos que AGREGAN una linea por invocacion
// (fuera del arbol revisado, para no disparar el gate de scope) con HOOM_TASK
// y el cwd fisico; extra corre despues, antes de salir con 0.
func htReviewer(t *testing.T, extra string) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "pasadas.txt")
	script := "printf '%s|%s|%s\\n' \"${HOOM_TASK+definida}\" \"${HOOM_TASK}\" \"$(pwd -P)\" >> '" + log + "'\n" +
		extra + "exit 0\n"
	fakeProvider(t, "codex", script)
	fakeProvider(t, "claude", script)
	return log
}

func htPasadas(t *testing.T, log string) []htPasada {
	t.Helper()
	raw, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("CA-243: el reviewer nunca corrio (no dejo rastro): %v", err)
	}
	var out []htPasada
	for _, l := range strings.Split(strings.TrimRight(string(raw), "\n"), "\n") {
		p := strings.SplitN(l, "|", 3)
		if len(p) != 3 {
			t.Fatalf("CA-243: salida inesperada del reviewer falso: %q", l)
		}
		out = append(out, htPasada{definida: p[0] == "definida", valor: p[1], dir: p[2]})
	}
	return out
}

func htReal(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// htRevisar corre la review y exige que termine revisado: nada de lo que se
// prueba aca es motivo para que una review se rompa.
func htRevisar(t *testing.T, root string, opt Options) Result {
	t.Helper()
	var out bytes.Buffer
	res, err := Run(root, "main", opt, &out)
	if err != nil {
		t.Fatalf("CA-293: la review no se rompe: %v\n%s", err, out.String())
	}
	if res.Status != "revisado" || res.ExitCode != 0 {
		t.Fatalf("CA-293: la review de prueba termina revisado: %+v\n%s", res, out.String())
	}
	return res
}

// htTodas exige n pasadas, cada una con HOOM_TASK definida = valor y en dir.
func htTodas(t *testing.T, pasadas []htPasada, n int, valor, dir string) {
	t.Helper()
	if len(pasadas) != n {
		t.Fatalf("CA-243: esperaba %d pasadas del reviewer, hubo %d: %+v", n, len(pasadas), pasadas)
	}
	for i, p := range pasadas {
		if !p.definida || p.valor != valor {
			t.Fatalf("CA-243: la pasada %d debe ver HOOM_TASK=%q, vio definida=%v valor=%q", i+1, valor, p.definida, p.valor)
		}
		if p.dir != dir {
			t.Fatalf("CA-243: la pasada %d corre en %s, corrio en %s", i+1, dir, p.dir)
		}
	}
}

func htRegistro(t *testing.T, dir, id string) map[string]any {
	t.Helper()
	if id == "" {
		t.Fatal("CA-293: una review revisada trae record_id")
	}
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", RecordsDir, id+".json"))
	if err != nil {
		t.Fatalf("CA-293: falta el registro de review %s: %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-293: el registro es JSON: %v\n%s", err, raw)
	}
	return m
}

// htRepoLimpio arma un proyecto con todo commiteado, para poder abrir tareas.
func htRepoLimpio(t *testing.T) string {
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
	return root
}

// CA-293 / CA-243: con --spec y sin --task, cada provider revisor ve
// HOOM_TASK=<slug del spec> (no la "ajena" del padre, no vacia), corre en la
// raiz, y el registro de review lleva la misma task.
func TestCA293_SoloSpecElReviewerVeLaTareaDelSpec(t *testing.T) {
	for _, prov := range []string{"codex", "claude"} {
		t.Run(prov, func(t *testing.T) {
			root := repo(t)
			write(t, root, htSpec, "# Spec: "+htSlug+"\n")
			log := htReviewer(t, "")
			t.Setenv("HOOM_TASK", "ajena")

			res := htRevisar(t, root, Options{Lens: "risk", Provider: prov, Spec: htSpec})
			if len(res.Passes) != 1 || res.Provider != prov {
				t.Fatalf("CA-293: una pasada de risk con %s: %+v", prov, res)
			}
			htTodas(t, htPasadas(t, log), 1, htSlug, htReal(t, root))
			if m := htRegistro(t, root, res.RecordID); m["task"] != htSlug {
				t.Fatalf("CA-293: el registro lleva task %q (del --spec): %v", htSlug, m["task"])
			}
		})
	}
}

// CA-293 / CA-243: con varias lentes (una ruta de riesgo pide las 4), CADA
// pasada ve HOOM_TASK=<slug del spec> y corre en la raiz.
func TestCA293_SoloSpecCadaPasadaVeLaTarea(t *testing.T) {
	root := repo(t)
	write(t, root, htSpec, "# Spec: "+htSlug+"\n")
	write(t, root, "internal/auth/login.go", "package auth\n\nfunc Login() {}\n")
	log := htReviewer(t, "")
	t.Setenv("HOOM_TASK", "ajena")

	res := htRevisar(t, root, Options{Provider: "codex", Spec: htSpec})
	if len(res.Lenses) != len(Lentes) || len(res.Passes) != len(Lentes) {
		t.Fatalf("CA-293: una ruta de riesgo corre las 4 lentes: %+v", res)
	}
	htTodas(t, htPasadas(t, log), len(Lentes), htSlug, htReal(t, root))
	if m := htRegistro(t, root, res.RecordID); m["task"] != htSlug {
		t.Fatalf("CA-293: el registro lleva task %q (del --spec): %v", htSlug, m["task"])
	}
}

// CA-293 / CA-243: aunque exista el worktree de la tarea del spec, sin --task
// el run corre en la RAIZ (el directorio lo decide --task), y HOOM_TASK es el
// slug del spec.
func TestCA293_SoloSpecConWorktreeDelSlugCorreEnLaRaiz(t *testing.T) {
	root := htRepoLimpio(t)
	if err := taskcmd.Start(root, htSlug, "main"); err != nil {
		t.Fatal(err)
	}
	write(t, root, "app.go", "package app\n\nfunc Nuevo() {}\n")
	log := htReviewer(t, "")
	t.Setenv("HOOM_TASK", "ajena")

	res := htRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: htSpec})
	htTodas(t, htPasadas(t, log), 1, htSlug, htReal(t, root))
	if m := htRegistro(t, root, res.RecordID); m["task"] != htSlug {
		t.Fatalf("CA-293: el registro lleva task %q (del --spec): %v", htSlug, m["task"])
	}
}

// CA-293 / CA-243 (guarda): con --task y un --spec de OTRO slug, gana --task:
// HOOM_TASK es la tarea y el run corre en su worktree.
func TestCA293_ConTareaGanaLaTareaSobreElSpec(t *testing.T) {
	root := htRepoLimpio(t)
	if err := taskcmd.Start(root, "cabina-visual", "main"); err != nil {
		t.Fatal(err)
	}
	dir, err := runcmd.TaskDir(root, "cabina-visual")
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, "app.go", "package app\n\nfunc Nuevo() {}\n")
	log := htReviewer(t, "")
	t.Setenv("HOOM_TASK", "ajena")

	res := htRevisar(t, root, Options{Lens: "risk", Provider: "codex", Task: "cabina-visual", Spec: htSpec})
	htTodas(t, htPasadas(t, log), 1, "cabina-visual", htReal(t, dir))
	if m := htRegistro(t, dir, res.RecordID); m["task"] != "cabina-visual" {
		t.Fatalf("CA-293: --task manda sobre el --spec en el registro: %v", m["task"])
	}
}

// CA-243 (guarda): sin --task ni --spec no hay tarea: HOOM_TASK va vacia (y
// definida) aunque el padre exporte otra.
func TestCA243_ReviewSinTareaNiSpecNoHereda(t *testing.T) {
	root := repo(t)
	log := htReviewer(t, "")
	t.Setenv("HOOM_TASK", "ajena")

	htRevisar(t, root, Options{Lens: "risk", Provider: "codex"})
	htTodas(t, htPasadas(t, log), 1, "", htReal(t, root))
}

// CA-293 / CA-243 (guarda): un --spec cuyo nombre no es un slug valido
// (^[a-z0-9][a-z0-9-]*$, a lo sumo 64) no da tarea: HOOM_TASK va vacia —
// nunca el nombre crudo, que 'hoom finding add' rechazaria — y la review no
// se rompe por eso.
func TestCA293_SpecSinSlugValidoNoDaTarea(t *testing.T) {
	for _, nombre := range []string{
		"Mi Spec", "X_Y", "Mayus", "-empieza-con-guion", "ñandu", "a.b",
		strings.Repeat("a", 65),
	} {
		t.Run(nombre, func(t *testing.T) {
			root := repo(t)
			log := htReviewer(t, "")
			t.Setenv("HOOM_TASK", "ajena")

			htRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: ".hoom/specs/" + nombre + ".md"})
			htTodas(t, htPasadas(t, log), 1, "", htReal(t, root))
		})
	}
}

// CA-293 / CA-243: los bordes validos del slug SI dan tarea: 64 caracteres y
// un slug que empieza con numero.
func TestCA293_SpecConSlugEnElBordeDaTarea(t *testing.T) {
	for _, slug := range []string{strings.Repeat("a", 64), "9-lote"} {
		t.Run(slug, func(t *testing.T) {
			root := repo(t)
			log := htReviewer(t, "")
			t.Setenv("HOOM_TASK", "ajena")

			htRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: ".hoom/specs/" + slug + ".md"})
			htTodas(t, htPasadas(t, log), 1, slug, htReal(t, root))
		})
	}
}

// CA-244 (guarda): con solo --spec, el hallazgo que el gate de scope crea por
// una violacion sigue SIN tarea: FindingTask es para los hallazgos del
// reviewer, no cambia la regla del gate.
func TestCA244_ConSoloSpecElHallazgoDelGateSigueSinTarea(t *testing.T) {
	root := repo(t)
	htReviewer(t, "printf 'x' > colado.txt\n")
	t.Setenv("HOOM_TASK", "ajena")

	var out bytes.Buffer
	res, err := Run(root, "main", Options{Lens: "risk", Provider: "codex", Spec: htSpec}, &out)
	if err != nil {
		t.Fatalf("CA-244: %v\n%s", err, out.String())
	}
	if len(res.Passes) != 1 || len(res.Passes[0].Scope.Violations) != 1 {
		t.Fatalf("CA-244: la escritura fuera de .hoom/ es una violacion: %+v\n%s", res, out.String())
	}
	v := res.Passes[0].Scope.Violations[0]
	f, ok := abHallazgoEn(t, v.FindingID, root)
	if !ok {
		t.Fatalf("CA-244: el hallazgo %s no quedo registrado", v.FindingID)
	}
	if f.Task != "" {
		t.Fatalf("CA-244: sin --task el hallazgo del gate no lleva tarea, ni la del --spec: %+v", f)
	}
}

// htHoomReal compila el binario real de hoom y lo pone al frente del PATH.
func htHoomReal(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("compila el binario de hoom: se omite con -short")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("sin toolchain de Go en el PATH")
	}
	dir := t.TempDir()
	cmd := exec.Command(goBin, "build", "-o", filepath.Join(dir, "hoom"), "github.com/hoomdev/hoomai/cmd/hoom")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("no pude compilar hoom: %v\n%s", err, out)
	}
	t.Setenv("PATH", dir+":"+os.Getenv("PATH"))
}

// CA-293 / CA-243, de punta a punta: el reviewer corre el 'hoom finding add'
// REAL sin --task durante una review con solo --spec, y el artefacto que
// queda en .hoom/findings/ lleva "task": "<slug del spec>", igual que el
// registro de la review.
func TestCA293_E2EElHallazgoDelReviewerLlevaLaTareaDelSpec(t *testing.T) {
	htHoomReal(t)
	root := repo(t)
	htReviewer(t, "hoom finding add --sev medium --lens risk --file app.go --author reviewer@codex \"Nuevo() no tiene test\" || exit 9\n")
	t.Setenv("HOOM_TASK", "ajena")

	res := htRevisar(t, root, Options{Lens: "risk", Provider: "codex", Spec: htSpec})
	if len(res.Findings) != 1 {
		t.Fatalf("CA-293: hoom ve aparecer el hallazgo del reviewer: %+v", res)
	}
	id := res.Findings[0]
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "findings", id+".json"))
	if err != nil {
		t.Fatalf("CA-293: el artefacto del hallazgo %s: %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-293: el hallazgo es JSON: %v\n%s", err, raw)
	}
	if m["task"] != htSlug {
		t.Fatalf("CA-293: el hallazgo del reviewer lleva \"task\": %q, igual que la review:\n%s", htSlug, raw)
	}
	reg := htRegistro(t, root, res.RecordID)
	if reg["task"] != htSlug {
		t.Fatalf("CA-293: el registro lleva task %q: %v", htSlug, reg["task"])
	}
	if f, _ := reg["findings"].([]any); len(f) != 1 || f[0] != id {
		t.Fatalf("CA-293: el registro nombra el hallazgo que hoom vio: %v", reg["findings"])
	}
}
