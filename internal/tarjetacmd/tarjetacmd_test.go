// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md sobre
// los verbos de la tarjeta, como consumidor externo (tarjetacmd.Run con un
// Entorno de mentira, proyectos git reales y CLIs de IA falsos). Este archivo
// trae los fixtures y cubre: la tarjeta de la que se habla (CA-498, CA-499),
// hoom usar (CA-500), la disciplina de argumentos (CA-512) y el hoom.yaml
// invalido (casos limite).
package tarjetacmd_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func TestMain(m *testing.M) {
	os.Unsetenv("HOOM_TASK")
	borrarConfig, err := gittest.AislarGit()
	if err != nil {
		fmt.Fprintln(os.Stderr, "fixture:", err)
		os.Exit(2)
	}
	code := m.Run()
	borrarConfig()
	os.Exit(code)
}

const (
	vdtBase    = "main"
	vdtBlockOn = "high"
	// vdtSinTarjeta es el texto del contrato cuando no hay tarjeta.
	vdtSinTarjeta = "no se de que tarjeta hablas: pasala (hoom pedir writer <slug>), entra a su espacio de trabajo o fija una con hoom usar <slug>"
)

// --- correr un verbo ---

type vdtOpts struct {
	dir      string
	in       string
	terminal bool
	env      map[string]string
}

type vdtSalida struct {
	code     int
	out, err string
}

func (s vdtSalida) String() string {
	return fmt.Sprintf("exit %d\nstdout:\n%s\nstderr:\n%s", s.code, s.out, s.err)
}

// vdtCorrer ejecuta un verbo. Un panico es una falla del criterio, no la
// caida de todo el paquete.
func vdtCorrer(t *testing.T, ca string, o vdtOpts, verb string, args ...string) vdtSalida {
	t.Helper()
	var out, errb bytes.Buffer
	e := tarjetacmd.Entorno{Dir: o.dir, In: strings.NewReader(o.in), Out: &out, Err: &errb, Terminal: o.terminal,
		Getenv: func(k string) string { return o.env[k] }}
	code := -1
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s: hoom %s %q entro en panico: %v", ca, verb, args, r)
			}
		}()
		code = tarjetacmd.Run(verb, args, e)
	}()
	return vdtSalida{code: code, out: out.String(), err: errb.String()}
}

// vdtDeducir llama a Deducir recuperando un panico.
func vdtDeducir(t *testing.T, ca, dir, operando string) (tarjetacmd.Deduccion, error) {
	t.Helper()
	var d tarjetacmd.Deduccion
	var err error
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s: Deducir(%q, %q) entro en panico: %v", ca, dir, operando, r)
			}
		}()
		d, err = tarjetacmd.Deducir(dir, operando)
	}()
	return d, err
}

func vdtExit(t *testing.T, ca string, s vdtSalida, code int) {
	t.Helper()
	if s.code != code {
		t.Fatalf("%s: exit %d, fue:\n%s", ca, code, s)
	}
}

// vdtDiceErr exige que stderr diga `hoom: <msg>`.
func vdtDiceErr(t *testing.T, ca string, s vdtSalida, msg string) {
	t.Helper()
	if !strings.Contains(s.err, "hoom: "+msg) {
		t.Fatalf("%s: stderr dice %q:\n%s", ca, "hoom: "+msg, s)
	}
}

func vdtDiceOut(t *testing.T, ca string, s vdtSalida, msg string) {
	t.Helper()
	if !strings.Contains(s.out, msg) {
		t.Fatalf("%s: stdout dice %q:\n%s", ca, msg, s)
	}
}

// vdtPrimeraLinea es la primera linea no vacia de un texto.
func vdtPrimeraLinea(s string) string {
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			return l
		}
	}
	return ""
}

func vdtLineas(s string) []string { return strings.Split(strings.TrimRight(s, "\n"), "\n") }

// vdtLineaQueEmpieza devuelve la linea de s que empieza con prefijo.
func vdtLineaQueEmpieza(s, prefijo string) (string, int) {
	for i, l := range vdtLineas(s) {
		if strings.HasPrefix(l, prefijo) {
			return l, i
		}
	}
	return "", -1
}

// --- proyectos git reales ---

func vdtGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	if len(args) > 0 && args[0] == "init" {
		gittest.ApagarMantenimiento(t, dir)
	}
	return strings.TrimSpace(string(out))
}

func vdtEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func vdtCommit(t *testing.T, dir, msg string) {
	t.Helper()
	vdtGit(t, dir, "add", "-A")
	vdtGit(t, dir, "commit", "-q", "-m", msg)
}

// vdtProyecto: repo git en main con hoom.yaml (gate barato, findings con
// block_on high y extra), el .gitignore canonico de .hoom y un commit.
func vdtProyecto(t *testing.T, extraYAML string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	vdtGit(t, root, "init", "-b", "main")
	vdtGit(t, root, "config", "user.email", "test@hoom.dev")
	vdtGit(t, root, "config", "user.name", "hoom test")
	vdtEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+
		"findings:\n  block_on: "+vdtBlockOn+"\n"+extraYAML)
	vdtEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	vdtEscribir(t, root, "app.go", "package app\n")
	vdtCommit(t, root, "inicial")
	return root
}

// vdtItem escribe el item de la tarjeta en el proyecto y lo guarda.
func vdtItem(t *testing.T, root, slug, extra string) {
	t.Helper()
	vdtEscribir(t, root, ".hoom/items/"+slug+".yaml", "titulo: Tarjeta "+slug+"\ntipo: feature\nprioridad: media\n"+
		"pedido: el pedido de "+slug+"\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n"+extra)
	vdtCommit(t, root, "item "+slug)
}

func vdtTarea(t *testing.T, root, slug string) string {
	t.Helper()
	if err := taskcmd.Start(root, slug, vdtBase); err != nil {
		t.Fatalf("fixture: hoom task start %s: %v", slug, err)
	}
	return filepath.Join(root, ".hoom", "worktrees", slug)
}

func vdtSpecTexto(criterios string) string {
	return "# Spec\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n" + criterios + "\n\n## Decisiones\nx\n\n## Riesgos\nx\n"
}

func vdtSpecRel(slug string) string { return ".hoom/specs/" + slug + ".md" }

func vdtAprobar(t *testing.T, dir, slug string) {
	t.Helper()
	if _, _, err := approval.Approve(dir, filepath.Join(dir, ".hoom", "specs", slug+".md")); err != nil {
		t.Fatalf("fixture: aprobar %s: %v", slug, err)
	}
}

func vdtGatesVerdes() []verdict.GateResult {
	return []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "spec_trace", Required: true, Status: verdict.StatusPass},
		{Name: "spec_approved", Required: true, Status: verdict.StatusPass},
		{Name: "findings_open", Required: true, Status: verdict.StatusPass},
		{Name: "test", Required: true, Status: verdict.StatusPass},
	}
}

func vdtVeredicto(t *testing.T, dir, slug string) {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: time.Now().UTC().Add(-time.Minute),
		Spec: vdtSpecRel(slug), Git: gitx.Snapshot(dir, vdtBase), Gates: vdtGatesVerdes()}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
}

func vdtCarta(t *testing.T, root, slug string) boardcmd.Card {
	t.Helper()
	c, err := boardcmd.CardFor(root, vdtBase, vdtBlockOn, slug, time.Now().UTC())
	if err != nil {
		t.Fatalf("fixture: CardFor %s: %v", slug, err)
	}
	return c
}

func vdtColumna(t *testing.T, ca, root, slug, col string) boardcmd.Card {
	t.Helper()
	c := vdtCarta(t, root, slug)
	if c.Column != col {
		t.Fatalf("%s: fixture: la tarjeta %s debia estar en %s y esta en %s (%s)", ca, slug, col, c.Column, c.Plain)
	}
	return c
}

func vdtAccion(t *testing.T, ca string, c boardcmd.Card, id string) boardcmd.Action {
	t.Helper()
	for _, a := range c.Actions {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("%s: fixture: la tarjeta %s (%s) no tiene la accion %s: %+v", ca, c.Slug, c.Column, id, c.Actions)
	return boardcmd.Action{}
}

// --- las tarjetas de cada columna ---

func vdtBacklog(t *testing.T, ca, root, slug string) {
	t.Helper()
	vdtItem(t, root, slug, "")
	vdtColumna(t, ca, root, slug, boardcmd.ColBacklog)
}

// vdtTuAprobacion: espacio de trabajo con un spec completo sin aprobar.
func vdtTuAprobacion(t *testing.T, ca, root, slug, criterios string) string {
	t.Helper()
	vdtItem(t, root, slug, "")
	wt := vdtTarea(t, root, slug)
	vdtEscribir(t, wt, vdtSpecRel(slug), vdtSpecTexto(criterios))
	vdtCommit(t, wt, "spec sin aprobar")
	vdtColumna(t, ca, root, slug, boardcmd.ColTuAprobacion)
	return wt
}

// vdtTestWriter: spec aprobado y guardado, con un criterio sin prueba.
func vdtTestWriter(t *testing.T, ca, root, slug, extraItem string) string {
	t.Helper()
	vdtItem(t, root, slug, extraItem)
	wt := vdtTarea(t, root, slug)
	vdtEscribir(t, wt, vdtSpecRel(slug), vdtSpecTexto("- CA-902: todavia nadie lo cita."))
	vdtAprobar(t, wt, slug)
	vdtCommit(t, wt, "spec aprobado")
	vdtColumna(t, ca, root, slug, boardcmd.ColTestWriter)
	return wt
}

func vdtTestDe(slug string) string { return strings.ReplaceAll(slug, "-", "_") + "_test.go" }

// vdtWriter: spec aprobado y sus criterios citados, sin veredicto.
func vdtWriter(t *testing.T, ca, root, slug, extraItem string) string {
	t.Helper()
	vdtItem(t, root, slug, extraItem)
	wt := vdtTarea(t, root, slug)
	vdtEscribir(t, wt, vdtSpecRel(slug), vdtSpecTexto("- CA-901: citado por un test."))
	vdtEscribir(t, wt, vdtTestDe(slug), "package app\n\n// CA-901\n")
	vdtAprobar(t, wt, slug)
	vdtCommit(t, wt, "spec aprobado y sus tests")
	vdtColumna(t, ca, root, slug, boardcmd.ColWriter)
	return wt
}

// vdtTuAceptacion: verde chico, todo guardado, sin hallazgos.
func vdtTuAceptacion(t *testing.T, ca, root, slug string) string {
	t.Helper()
	vdtItem(t, root, slug, "")
	wt := vdtTarea(t, root, slug)
	vdtEscribir(t, wt, vdtSpecRel(slug), vdtSpecTexto("- CA-970: citado por un test."))
	vdtEscribir(t, wt, vdtTestDe(slug), "package app\n\n// CA-970\n")
	vdtEscribir(t, wt, "lista.go", "package app\n\nfunc Lista() int { return 1 }\n")
	vdtAprobar(t, wt, slug)
	vdtVeredicto(t, wt, slug)
	vdtCommit(t, wt, "lista para cerrar")
	vdtColumna(t, ca, root, slug, boardcmd.ColTuAceptacion)
	return wt
}

// vdtReview: verde con mas de 400 lineas y sin registro de review.
func vdtReview(t *testing.T, ca, root, slug string) string {
	t.Helper()
	vdtItem(t, root, slug, "")
	wt := vdtTarea(t, root, slug)
	vdtEscribir(t, wt, vdtSpecRel(slug), vdtSpecTexto("- CA-903: citado por un test."))
	vdtEscribir(t, wt, vdtTestDe(slug), "package app\n\n// CA-903\n")
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; i < 450; i++ {
		fmt.Fprintf(&b, "var grande%d = %d\n", i, i)
	}
	vdtEscribir(t, wt, "grande.go", b.String())
	vdtAprobar(t, wt, slug)
	vdtCommit(t, wt, "cambio grande")
	vdtVeredicto(t, wt, slug)
	vdtCommit(t, wt, "veredicto")
	c := vdtColumna(t, ca, root, slug, boardcmd.ColReview)
	vdtAccion(t, ca, c, boardcmd.ActPedirReviewer)
	return wt
}

func vdtPIDMuerto(t *testing.T) int {
	t.Helper()
	for i := 0; i < 5; i++ {
		cmd := exec.Command("/usr/bin/true")
		if err := cmd.Run(); err != nil {
			t.Fatal(err)
		}
		if pid := cmd.Process.Pid; !runcmd.Alive(pid) {
			return pid
		}
	}
	t.Fatal("fixture: no consegui el PID de un proceso muerto")
	return 0
}

func vdtSidecar(t *testing.T, root string, m runcmd.Meta) {
	t.Helper()
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	vdtEscribir(t, root, ".hoom/runs/"+m.ID+".meta.json", string(raw)+"\n")
}

func vdtSobre(t *testing.T, root string, rec envelope.Record) {
	t.Helper()
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	vdtEscribir(t, root, ".hoom/envelopes/"+rec.ID+".json", string(raw)+"\n")
}

const vdtSesion = "sesion-vieja-123"

// vdtInterrumpida: una tarjeta en Writer cuyo sobre de writer quedo abierto
// con el PID de un proceso muerto en el paso 3 de 5 (run), con el sidecar de
// su run (con o sin el id de sesion de claude).
func vdtInterrumpida(t *testing.T, ca, root, slug string, conSesion bool) string {
	t.Helper()
	wt := vdtWriter(t, ca, root, slug, "")
	muerto := vdtPIDMuerto(t)
	ahora := time.Now().UTC()
	runID, envID := "20260923T080000_run0ld", "20260923T075959_env0ld"
	m := runcmd.Meta{ID: runID, Provider: "claude", Role: "writer", Task: slug, Dir: wt,
		CreatedAt: ahora.Add(-2 * time.Hour), Status: runcmd.StatusRunning, ExitCode: -1, PID: muerto}
	if conSesion {
		m.ProviderSessionID = vdtSesion
	}
	vdtSidecar(t, root, m)
	vdtSobre(t, root, envelope.Record{ID: envID, Role: "writer", Provider: "claude", Task: slug, Dir: wt,
		Spec: filepath.Join(wt, ".hoom", "specs", slug+".md"), Approval: "aprobado", RunID: runID,
		Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: ahora.Add(-2 * time.Hour), UpdatedAt: ahora.Add(-time.Hour), PID: muerto})
	if c := vdtCarta(t, root, slug); c.Interrupted == nil || c.Interrupted.EnvelopeID != envID {
		t.Fatalf("%s: fixture: la tarjeta %s esta interrumpida por %s: %+v", ca, slug, envID, c.Interrupted)
	}
	return wt
}

// vdtEnCurso: una tarjeta en Writer con un writer trabajando (sobre abierto
// con el PID de este proceso y latido fresco), en el paso 3 de 5 (run).
func vdtEnCurso(t *testing.T, ca, root, slug string) string {
	t.Helper()
	wt := vdtWriter(t, ca, root, slug, "")
	ahora := time.Now().UTC()
	vdtSobre(t, root, envelope.Record{ID: "20260923T090000_envviv", Role: "writer", Provider: "claude", Task: slug, Dir: wt,
		Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: ahora.Add(-10 * time.Minute), UpdatedAt: ahora, PID: os.Getpid()})
	return wt
}

func vdtHecho(t *testing.T, ca, root, slug string) {
	t.Helper()
	vdtItem(t, root, slug, "hecho_en: 2026-09-23T10:00:00Z\ncommit_final: 3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c\n")
	vdtColumna(t, ca, root, slug, boardcmd.ColHecho)
}

// --- CLIs de IA y editores falsos ---

type vdtFalsos struct {
	bin  string // al frente del PATH
	argv string // cada invocacion deja aca sus argumentos, separados por NUL
}

// vdtPATH deja un PATH minimo: git y sh del sistema y los falsos que se
// instalen despues. Ningun claude, codex, gemini ni opencode real entra.
func vdtPATH(t *testing.T) *vdtFalsos {
	t.Helper()
	f := &vdtFalsos{bin: t.TempDir(), argv: t.TempDir()}
	t.Setenv("PATH", f.bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	return f
}

// cli instala un CLI de IA falso que guarda su stdin y sus argumentos y corre
// extra antes de terminar con 0.
func (f *vdtFalsos) cli(t *testing.T, name, extra string) {
	t.Helper()
	final := filepath.Join(f.argv, name)
	tmp := filepath.Join(f.argv, ".tmp-"+name)
	in := filepath.Join(f.argv, "in-"+name)
	s := "#!/bin/sh\n" +
		"cat > '" + in + ".'$$\n" +
		"printf '%s\\000' \"$@\" > '" + tmp + ".'$$\n" +
		"mv '" + tmp + ".'$$ '" + final + ".'$$\n" + extra + "\nexit 0\n"
	if err := os.WriteFile(filepath.Join(f.bin, name), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (f *vdtFalsos) llamadas(t *testing.T, name string) [][]string {
	t.Helper()
	entries, err := os.ReadDir(f.argv)
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), name+".") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(f.argv, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Split(string(raw), "\x00")
		if len(args) > 0 && args[len(args)-1] == "" {
			args = args[:len(args)-1]
		}
		out = append(out, args)
	}
	return out
}

// pedidos es el prompt de cada invocacion: su stdin si llego algo por ahi o
// el argv termina en "-"; si no, el ultimo argumento.
func (f *vdtFalsos) pedidos(t *testing.T, name string) []string {
	t.Helper()
	entries, _ := os.ReadDir(f.argv)
	var out []string
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), name+".") {
			continue
		}
		raw, _ := os.ReadFile(filepath.Join(f.argv, e.Name()))
		args := strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
		in, _ := os.ReadFile(filepath.Join(f.argv, "in-"+e.Name()))
		ult := ""
		if len(args) > 0 {
			ult = args[len(args)-1]
		}
		if ult == "-" || len(in) > 0 {
			out = append(out, string(in))
		} else {
			out = append(out, ult)
		}
	}
	return out
}

func (f *vdtFalsos) ninguna(t *testing.T, ca, que string) {
	t.Helper()
	for _, name := range []string{"claude", "codex", "gemini", "opencode"} {
		if ll := f.llamadas(t, name); len(ll) != 0 {
			t.Fatalf("%s: %s no lanza nada, y %s corrio %d veces: %q", ca, que, name, len(ll), ll)
		}
	}
}

func vdtUnaLlamada(t *testing.T, ca, que string, f *vdtFalsos, name string) []string {
	t.Helper()
	ll := f.llamadas(t, name)
	if len(ll) != 1 {
		t.Fatalf("%s: %s corre %s una vez, fueron %d: %q", ca, que, name, len(ll), ll)
	}
	return ll[0]
}

func vdtSigue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// vdtEditor instala un editor falso que anota sus argumentos y la
// plantilla que recibio en capt, deja contenido en el archivo y sale con
// code. Devuelve su ruta y capt.
func vdtEditor(t *testing.T, nombre, contenido string, code int) (string, string) {
	t.Helper()
	dir := t.TempDir()
	capt := t.TempDir()
	s := "#!/bin/sh\n" +
		"printf '%s\\000' \"$@\" > '" + capt + "/args'\n" +
		"for ultimo; do :; done\n" +
		"cp \"$ultimo\" '" + capt + "/plantilla'\n" +
		"cat > \"$ultimo\" <<'VDT_FIN'\n" + contenido + "\nVDT_FIN\n" +
		fmt.Sprintf("exit %d\n", code)
	p := filepath.Join(dir, nombre)
	if err := os.WriteFile(p, []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return p, capt
}

func vdtEditorLlamado(capt string) bool {
	_, err := os.Stat(filepath.Join(capt, "args"))
	return err == nil
}

func vdtEditorArgs(t *testing.T, capt string) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(capt, "args"))
	if err != nil {
		t.Fatalf("el editor no corrio: %v", err)
	}
	return strings.Split(strings.TrimSuffix(string(raw), "\x00"), "\x00")
}

func vdtPlantilla(t *testing.T, capt string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(capt, "plantilla"))
	if err != nil {
		t.Fatalf("el editor no recibio la plantilla: %v", err)
	}
	return string(raw)
}

// --- sin efectos ---

// vdtFoto fotografia el proyecto entero (con sus espacios de trabajo), sin
// .git ni los candados, mas las ramas y los worktrees de git.
func vdtFoto(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if rel == ".git" || rel == ".hoom/cache/locks" {
				return filepath.SkipDir
			}
			return nil
		}
		raw, _ := os.ReadFile(p)
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(raw))
		return nil
	})
	out["(ramas)"] = vdtGit(t, root, "branch", "--list")
	out["(worktrees)"] = vdtGit(t, root, "worktree", "list", "--porcelain")
	out["(head)"] = vdtGit(t, root, "rev-parse", "HEAD")
	return out
}

func vdtSinEfectos(t *testing.T, ca, que string, antes, despues map[string]string) {
	t.Helper()
	for k, v := range despues {
		w, ok := antes[k]
		switch {
		case !ok:
			t.Fatalf("%s: %s dejo %s en disco", ca, que, k)
		case w != v:
			t.Fatalf("%s: %s modifico %s", ca, que, k)
		}
	}
	for k := range antes {
		if _, ok := despues[k]; !ok {
			t.Fatalf("%s: %s borro %s", ca, que, k)
		}
	}
}

// ===========================================================================
// CA-498 / CA-499: la tarjeta de la que se habla
// ===========================================================================

// CA-498: el operando gana; un slug que no es tarjeta es exit 1 con el
// texto del contrato.
func TestCA498_DeducirPorOperando(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	vdtItem(t, root, "otra", "")
	vdtTarea(t, root, "precios")
	vdtEscribir(t, root, tarjetacmd.ArchivoFijada, "otra\n")

	d, err := vdtDeducir(t, "CA-498", root, "precios")
	if err != nil || d.Slug != "precios" || d.Origen != tarjetacmd.OrigenArgumento || d.Root != root {
		t.Fatalf("CA-498: el operando gana a hoom usar: %+v %v", d, err)
	}
	// el operando gana a la carpeta
	d, err = vdtDeducir(t, "CA-498", filepath.Join(root, ".hoom", "worktrees", "precios"), "otra")
	if err != nil || d.Slug != "otra" || d.Origen != tarjetacmd.OrigenArgumento || d.Root != root {
		t.Fatalf("CA-498: el operando gana a la carpeta, y el proyecto es el arbol principal: %+v %v", d, err)
	}
	_, err = vdtDeducir(t, "CA-498", root, "nada")
	var f *tarjetacmd.Fallo
	if !errors.As(err, &f) || f.Code != 1 || f.Msg != "no hay tarjeta nada: hoom item list" {
		t.Fatalf("CA-498: un slug que no es tarjeta es exit 1 %q: %#v", "no hay tarjeta nada: hoom item list", err)
	}
}

// CA-498 y CA-499: dentro de .hoom/worktrees/<slug>/ a cualquier
// profundidad la tarjeta es <slug>, la carpeta gana a hoom usar y el
// proyecto es el arbol principal.
func TestCA499_DeducirPorCarpeta(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	vdtItem(t, root, "otra", "")
	wt := vdtTarea(t, root, "precios")
	vdtEscribir(t, root, tarjetacmd.ArchivoFijada, "otra\n")
	hondo := filepath.Join(wt, "internal", "pkg", "sub")
	if err := os.MkdirAll(hondo, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{wt, hondo, wt + "/internal/../internal/pkg"} {
		d, err := vdtDeducir(t, "CA-499", dir, "")
		if err != nil || d.Slug != "precios" || d.Origen != tarjetacmd.OrigenCarpeta || d.Root != root {
			t.Fatalf("CA-499: desde %s la tarjeta es precios (carpeta) y el proyecto %s: %+v %v", dir, root, d, err)
		}
	}
}

// CA-499: un arbol ciego anidado (.hoom/isolated/ del espacio) cuenta como
// el espacio que lo contiene; y un .hoom/worktrees/<otro> anidado mas
// adentro no cambia la tarjeta: manda el de mas afuera.
func TestCA499_ArbolCiegoAnidadoEsDelEspacioQueLoContiene(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	vdtItem(t, root, "otra", "")
	wt := vdtTarea(t, root, "precios")
	ciego := filepath.Join(wt, ".hoom", "isolated", "20260923T080000_abc", "internal")
	anidado := filepath.Join(wt, ".hoom", "isolated", "20260923T080000_abc", ".hoom", "worktrees", "otra", "x")
	for _, dir := range []string{ciego, anidado} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		d, err := vdtDeducir(t, "CA-499", dir, "")
		if err != nil || d.Slug != "precios" || d.Origen != tarjetacmd.OrigenCarpeta || d.Root != root {
			t.Fatalf("CA-499: desde %s la tarjeta es precios y el proyecto %s: %+v %v", dir, root, d, err)
		}
	}
}

// CA-498: fuera de un espacio, la tarjeta fijada; el proyecto es el que
// tiene hoom.yaml, subiendo.
func TestCA498_DeducirPorHoomUsar(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	vdtEscribir(t, root, tarjetacmd.ArchivoFijada, "precios\n")
	sub := filepath.Join(root, "internal", "pkg")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, sub} {
		d, err := vdtDeducir(t, "CA-498", dir, "")
		if err != nil || d.Slug != "precios" || d.Origen != tarjetacmd.OrigenUsar || d.Root != root {
			t.Fatalf("CA-498: desde %s, la tarjeta fijada (hoom usar) en %s: %+v %v", dir, root, d, err)
		}
	}
}

// CA-498: sin operando, carpeta ni tarjeta fijada: exit 2 con el texto del
// contrato.
func TestCA498_SinTarjetaEsExit2(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	_, err := vdtDeducir(t, "CA-498", root, "")
	var f *tarjetacmd.Fallo
	if !errors.As(err, &f) || f.Code != 2 || f.Msg != vdtSinTarjeta {
		t.Fatalf("CA-498: sin tarjeta es exit 2 %q: %#v", vdtSinTarjeta, err)
	}
}

// CA-498 y CA-500: una tarjeta fijada que dejo de existir es exit 1 con el
// texto del contrato.
func TestCA500_TarjetaFijadaQueYaNoExiste(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	vdtEscribir(t, root, tarjetacmd.ArchivoFijada, "borrada\n")
	_, err := vdtDeducir(t, "CA-500", root, "")
	var f *tarjetacmd.Fallo
	want := "la tarjeta fijada borrada ya no existe: hoom usar <otra>"
	if !errors.As(err, &f) || f.Code != 1 || f.Msg != want {
		t.Fatalf("CA-500: la tarjeta fijada que ya no existe es exit 1 %q: %#v", want, err)
	}
	s := vdtCorrer(t, "CA-500", vdtOpts{dir: root}, "guardar")
	vdtExit(t, "CA-500", s, 1)
	vdtDiceErr(t, "CA-500", s, want)
}

// Caso limite: el espacio de trabajo de una tarea sin item (como una tarea
// de borradores) no es una tarjeta.
func TestCA498_EspacioDeUnaTareaSinItem(t *testing.T) {
	root := vdtProyecto(t, "")
	wt := vdtTarea(t, root, "borradores")
	_, err := vdtDeducir(t, "CA-498", wt, "")
	var f *tarjetacmd.Fallo
	if !errors.As(err, &f) || f.Code != 1 || f.Msg != "no hay tarjeta borradores: hoom item list" {
		t.Fatalf("CA-498: una tarea sin item es exit 1 %q: %#v", "no hay tarjeta borradores: hoom item list", err)
	}
	s := vdtCorrer(t, "CA-498", vdtOpts{dir: wt}, "guardar")
	vdtExit(t, "CA-498", s, 1)
	vdtDiceErr(t, "CA-498", s, "no hay tarjeta borradores: hoom item list")
}

// CA-498: cada verbo (menos usar) imprime primero `tarjeta: <slug>
// (<origen>)`, y sin tarjeta sale con 2 y el texto del contrato, sin efectos.
func TestCA498_LineaTarjetaPrimeroYSinTarjetaSinEfectos(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtHecho(t, "CA-498", root, "precios")
	verbos := [][]string{{"pedir", "writer"}, {"siguiente"}, {"aprobar", "--si"}, {"integrar", "--si"}, {"guardar"}, {"reanudar", "-m", "x"}}
	for _, v := range verbos {
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "CA-498", vdtOpts{dir: root}, v[0], v[1:]...)
		vdtExit(t, "CA-498 (hoom "+strings.Join(v, " ")+" sin tarjeta)", s, 2)
		if !strings.Contains(s.err, vdtSinTarjeta) {
			t.Fatalf("CA-498: hoom %s sin tarjeta dice %q:\n%s", v[0], vdtSinTarjeta, s)
		}
		if strings.Contains(s.out+s.err, "tarjeta: ") {
			t.Fatalf("CA-498: sin tarjeta no hay linea de tarjeta:\n%s", s)
		}
		vdtSinEfectos(t, "CA-498", "hoom "+v[0]+" sin tarjeta", antes, vdtFoto(t, root))

		// con el operando: la primera linea es la de la tarjeta
		args := append([]string{}, v[1:]...)
		if v[0] == "pedir" {
			args = append(args, "precios")
		} else {
			args = append([]string{"precios"}, args...)
		}
		s = vdtCorrer(t, "CA-498", vdtOpts{dir: root}, v[0], args...)
		if got := vdtPrimeraLinea(s.out); got != "tarjeta: precios (argumento)" {
			t.Fatalf("CA-498: hoom %s %q imprime primero %q, fue %q:\n%s", v[0], args, "tarjeta: precios (argumento)", got, s)
		}
	}
	f.ninguna(t, "CA-498", "un verbo sobre una tarjeta en Hecho")
}

// CA-498: el origen de la linea dice de donde salio la tarjeta (carpeta,
// hoom usar), y dos terminales con distinta tarjeta fijada: gana el ultimo
// hoom usar.
func TestCA498_OrigenEnLaLinea(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtHecho(t, "CA-498", root, "precios")
	vdtHecho(t, "CA-498", root, "otra")
	wt := vdtTarea(t, root, "precios")

	s := vdtCorrer(t, "CA-498", vdtOpts{dir: wt}, "guardar")
	if got := vdtPrimeraLinea(s.out); got != "tarjeta: precios (carpeta)" {
		t.Fatalf("CA-498: desde el espacio de trabajo: %q:\n%s", got, s)
	}
	vdtExit(t, "CA-498", vdtCorrer(t, "CA-498", vdtOpts{dir: root}, "usar", "precios"), 0)
	vdtExit(t, "CA-498", vdtCorrer(t, "CA-498", vdtOpts{dir: root}, "usar", "otra"), 0)
	s = vdtCorrer(t, "CA-498", vdtOpts{dir: root}, "guardar")
	if got := vdtPrimeraLinea(s.out); got != "tarjeta: otra (hoom usar)" {
		t.Fatalf("CA-498: gana el ultimo hoom usar: %q:\n%s", got, s)
	}
}

// ===========================================================================
// CA-500: hoom usar
// ===========================================================================

func vdtFijada(t *testing.T, root string) (string, bool) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(tarjetacmd.ArchivoFijada)))
	if os.IsNotExist(err) {
		return "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(raw), true
}

// CA-500: fijar, mostrar y quitar; el archivo es una linea con el slug.
func TestCA500_UsarFijaMuestraYQuita(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	o := vdtOpts{dir: root}

	s := vdtCorrer(t, "CA-500", o, "usar")
	vdtExit(t, "CA-500", s, 0)
	vdtDiceOut(t, "CA-500", s, "sin tarjeta fijada")

	s = vdtCorrer(t, "CA-500", o, "usar", "precios")
	vdtExit(t, "CA-500", s, 0)
	vdtDiceOut(t, "CA-500", s, "tarjeta fijada: precios")
	if got, ok := vdtFijada(t, root); !ok || got != "precios\n" {
		t.Fatalf("CA-500: .hoom/cache/tarjeta es una linea con el slug: %q %v", got, ok)
	}

	s = vdtCorrer(t, "CA-500", o, "usar")
	vdtExit(t, "CA-500", s, 0)
	if !strings.Contains(s.out, "tarjeta: precios") || strings.Contains(s.out, "sin tarjeta fijada") {
		t.Fatalf("CA-500: hoom usar muestra la fijada:\n%s", s)
	}

	s = vdtCorrer(t, "CA-500", o, "usar", "--ninguna")
	vdtExit(t, "CA-500", s, 0)
	vdtDiceOut(t, "CA-500", s, "sin tarjeta fijada")
	if _, ok := vdtFijada(t, root); ok {
		t.Fatalf("CA-500: --ninguna quita el archivo")
	}
	s = vdtCorrer(t, "CA-500", o, "usar")
	vdtDiceOut(t, "CA-500", s, "sin tarjeta fijada")
}

// CA-500: fijar un slug que no es tarjeta es exit 1 y no cambia nada.
func TestCA500_UsarSlugInexistenteNoCambiaNada(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	o := vdtOpts{dir: root}
	vdtExit(t, "CA-500", vdtCorrer(t, "CA-500", o, "usar", "precios"), 0)
	s := vdtCorrer(t, "CA-500", o, "usar", "nada")
	vdtExit(t, "CA-500", s, 1)
	vdtDiceErr(t, "CA-500", s, "no hay tarjeta nada: hoom item list")
	if got, _ := vdtFijada(t, root); got != "precios\n" {
		t.Fatalf("CA-500: un slug inexistente no cambia la fijada: %q", got)
	}
	vdtExit(t, "CA-500", vdtCorrer(t, "CA-500", o, "usar", "--ninguna"), 0)
	vdtExit(t, "CA-500", vdtCorrer(t, "CA-500", o, "usar", "nada"), 1)
	if _, ok := vdtFijada(t, root); ok {
		t.Fatalf("CA-500: un slug inexistente no crea el archivo")
	}
}

// CA-500: el archivo vive en el proyecto (aunque se fije desde un espacio
// de trabajo), no entra en la huella ni en git.
func TestCA500_UsarNoEntraEnLaHuellaNiEnGit(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	vdtItem(t, root, "otra", "")
	wt := vdtTarea(t, root, "precios")
	huellaRaiz := gitx.Snapshot(root, vdtBase).ChangeFingerprint
	huellaWt := gitx.Snapshot(wt, vdtBase).ChangeFingerprint
	estadoRaiz := vdtGit(t, root, "status", "--porcelain")
	estadoWt := vdtGit(t, wt, "status", "--porcelain")

	vdtExit(t, "CA-500", vdtCorrer(t, "CA-500", vdtOpts{dir: wt}, "usar", "otra"), 0)
	if got, ok := vdtFijada(t, root); !ok || got != "otra\n" {
		t.Fatalf("CA-500: desde el espacio de trabajo se fija en el proyecto: %q %v", got, ok)
	}
	if _, ok := vdtFijada(t, wt); ok {
		t.Fatalf("CA-500: el espacio de trabajo no es un proyecto: no gana .hoom/cache/tarjeta")
	}
	if gitx.Snapshot(root, vdtBase).ChangeFingerprint != huellaRaiz || gitx.Snapshot(wt, vdtBase).ChangeFingerprint != huellaWt {
		t.Fatalf("CA-500: el archivo no entra en la huella")
	}
	if vdtGit(t, root, "status", "--porcelain") != estadoRaiz || vdtGit(t, wt, "status", "--porcelain") != estadoWt {
		t.Fatalf("CA-500: el archivo no viaja en git")
	}
}

// ===========================================================================
// CA-512: argumentos
// ===========================================================================

// CA-512: los siete verbos rechazan un flag desconocido, un flag sin valor y
// un operando de mas con exit 2, sin efectos y con su bloque de uso; -h
// sale con 0 e imprime el uso.
func TestCA512_DisciplinaDeArgumentos(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-512", root, "precios", "")
	vdtEscribir(t, root, tarjetacmd.ArchivoFijada, "precios\n")
	casos := []struct {
		verbo string
		args  []string
		razon string
	}{
		{"usar", []string{"--bogus"}, "flag desconocido: --bogus"},
		{"usar", []string{"precios", "otra"}, `"otra"`},
		{"pedir", []string{"writer", "--bogus"}, "flag desconocido: --bogus"},
		{"pedir", []string{"writer", "-m"}, "-m necesita un valor"},
		{"pedir", []string{"writer", "precios", "--model"}, "--model necesita un valor"},
		{"pedir", []string{"writer", "precios", "otra"}, `"otra"`},
		{"pedir", []string{}, ""},
		{"siguiente", []string{"--bogus"}, "flag desconocido: --bogus"},
		{"siguiente", []string{"--effort"}, "--effort necesita un valor"},
		{"siguiente", []string{"precios", "otra"}, `"otra"`},
		{"aprobar", []string{"--bogus"}, "flag desconocido: --bogus"},
		{"aprobar", []string{"precios", "otra"}, `"otra"`},
		{"integrar", []string{"--bogus"}, "flag desconocido: --bogus"},
		{"integrar", []string{"precios", "otra"}, `"otra"`},
		{"guardar", []string{"--bogus"}, "flag desconocido: --bogus"},
		{"guardar", []string{"precios", "otra"}, `"otra"`},
		{"reanudar", []string{"--bogus"}, "flag desconocido: --bogus"},
		{"reanudar", []string{"--timeout"}, "--timeout necesita un valor"},
		{"reanudar", []string{"precios", "otra"}, `"otra"`},
		// reanudar retoma con su provider: no se elige otro
		{"reanudar", []string{"--provider", "codex"}, "flag desconocido: --provider"},
	}
	for _, c := range casos {
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "CA-512", vdtOpts{dir: root}, c.verbo, c.args...)
		que := "hoom " + c.verbo + " " + strings.Join(c.args, " ")
		vdtExit(t, "CA-512 ("+que+")", s, 2)
		if !strings.HasPrefix(s.err, "hoom "+c.verbo+": ") || !strings.Contains(s.err, c.razon) {
			t.Fatalf("CA-512: %s: stderr empieza con %q y dice %q:\n%s", que, "hoom "+c.verbo+": ", c.razon, s)
		}
		if !strings.Contains(s.err, tarjetacmd.Uso(c.verbo)) {
			t.Fatalf("CA-512: %s: stderr trae el bloque de uso del verbo:\n%s", que, s)
		}
		vdtSinEfectos(t, "CA-512", que, antes, vdtFoto(t, root))
	}
	f.ninguna(t, "CA-512", "un error de uso")

	for _, v := range tarjetacmd.Verbos {
		if tarjetacmd.Uso(v) == "" {
			t.Fatalf("CA-512: hoom %s tiene bloque de uso", v)
		}
		for _, h := range []string{"-h", "--help"} {
			s := vdtCorrer(t, "CA-512", vdtOpts{dir: root}, v, h)
			vdtExit(t, "CA-512 (hoom "+v+" "+h+")", s, 0)
			if !strings.Contains(s.out+s.err, tarjetacmd.Uso(v)) {
				t.Fatalf("CA-512: hoom %s %s imprime el uso:\n%s", v, h, s)
			}
		}
	}
	f.ninguna(t, "CA-512", "-h")
}

// CA-512 y casos limite: --timeout fuera de 1m..12h o sin unidad es exit 2
// con el texto del contrato, sin lanzar.
func TestCA512_TimeoutInvalidoEsExit2(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-512", root, "precios", "")
	for _, v := range []string{"0", "30", "13h", "30s", "abc"} {
		for _, verbo := range [][]string{{"pedir", "writer", "precios", "-m", "Implementa"}, {"siguiente", "precios", "-m", "Implementa"}} {
			args := append(append([]string{}, verbo[1:]...), "--timeout", v)
			s := vdtCorrer(t, "CA-512", vdtOpts{dir: root}, verbo[0], args...)
			vdtExit(t, "CA-512 (--timeout "+v+")", s, 2)
			want := `--timeout: "` + v + `" no es una duracion entre 1m y 12h (ej. 45m, 1h30m)`
			if !strings.Contains(s.err, want) {
				t.Fatalf("CA-512: --timeout %s dice %q:\n%s", v, want, s)
			}
		}
	}
	f.ninguna(t, "CA-512", "un --timeout invalido")
}

// Caso limite: hoom.yaml con agents.test-writer.timeout: 45 (sin unidad) es
// un error de carga, y ningun verbo que lee el manifiesto arranca.
func TestCA490_HoomYamlInvalidoNoArrancaNingunVerbo(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtTestWriter(t, "CA-490", root, "precios", "")
	vdtEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+
		"agents:\n  test-writer:\n    timeout: 45\n")
	vdtCommit(t, root, "timeout sin unidad")
	for _, v := range [][]string{{"pedir", "test-writer", "precios", "-m", "Escribi los tests"}, {"siguiente", "precios", "-m", "Escribi los tests"}} {
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "CA-490", vdtOpts{dir: root}, v[0], v[1:]...)
		if s.code == 0 {
			t.Fatalf("CA-490: con hoom.yaml invalido hoom %s no arranca:\n%s", v[0], s)
		}
		if !strings.Contains(s.err, `agents.test-writer.timeout: "45" no es una duracion entre 1m y 12h (ej. 45m, 1h30m)`) {
			t.Fatalf("CA-490: hoom %s dice el error de carga del contrato:\n%s", v[0], s)
		}
		vdtSinEfectos(t, "CA-490", "hoom "+v[0]+" con hoom.yaml invalido", antes, vdtFoto(t, root))
	}
	f.ninguna(t, "CA-490", "un hoom.yaml invalido")
}
