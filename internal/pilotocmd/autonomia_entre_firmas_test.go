// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md sobre
// la vuelta del piloto (pilotocmd), como consumidor externo: proyectos git
// reales, sobres y veredictos escritos en disco, y CLIs de IA falsos en un
// PATH minimo (nunca uno real). Cubre:
//
//   - CA-644: guardar al empezar cada vuelta (en hoom/<slug>, nada en el
//     checkout principal, sin push, sin commits vacios, y la detencion cuando
//     no se puede guardar);
//   - CA-647: `hoom piloto <slug> [--json]` (Run, Correr, Avanzar): exit 0 en
//     una columna humana, 1 en cualquier otra detencion, 2 si no entiende el
//     pedido, y el candado de la tarjeta;
//   - CA-648: AvisoDeFirma;
//   - CA-656: el paso cerrar-hallazgos, con la aclaracion 10;
//   - CA-661: las lineas del piloto y el registro de lo que lanza.
package pilotocmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func TestMain(m *testing.M) {
	for _, k := range []string{"HOOM_TASK", "HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER"} {
		os.Unsetenv(k)
	}
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
	apBase     = "main"
	apBlockOn  = "high"
	apConAuto  = "presupuesto_usd: 5\nauto: hasta-humano\n"
	apSinAuto  = "presupuesto_usd: 5\n"
	apPrefijo  = "piloto automatico: "
	apDetiene  = apPrefijo + "se detiene: "
	apGuardaMs = "hoom: guardar la tarjeta "
)

var (
	apGuarda     = regexp.MustCompile(`^piloto automatico: guarda el trabajo de la tarjeta: ([0-9a-f]{7,40})$`)
	apPide       = regexp.MustCompile(`^piloto automatico: pide al (\S+) con (\S+) \(([0-9]+(?:\.[0-9]+)?) USD\): sobre (\S+)$`)
	apPideSinUSD = regexp.MustCompile(`^piloto automatico: pide al (\S+) con (\S+) \(sin tope en USD\): sobre (\S+)$`)
)

func apAviso(slug string) string {
	return "la tarjeta tiene piloto automatico: sigue con: hoom piloto " + slug
}

func apSinTarea(slug string) string {
	return apDetiene + "el piloto corre en el espacio de trabajo de la tarjeta: abre la tarea con hoom task start " + slug
}

// ningun provider en la maquina: el piloto no puede lanzar nada.
func apNingunProveedor() []providers.Info { return nil }

// --- proyectos git reales ---

func apGit(t *testing.T, dir string, args ...string) string {
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

func apEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func apCommit(t *testing.T, dir, msg string) {
	t.Helper()
	apGit(t, dir, "add", "-A")
	apGit(t, dir, "commit", "-q", "-m", msg)
}

// apProyecto: repo git en main con hoom.yaml (gate barato, findings con
// block_on high y extra), el .gitignore canonico de .hoom y un commit.
func apProyecto(t *testing.T, extraYAML string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	apGit(t, root, "init", "-b", apBase)
	apGit(t, root, "config", "user.email", "test@hoom.dev")
	apGit(t, root, "config", "user.name", "hoom test")
	apEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+
		"findings:\n  block_on: "+apBlockOn+"\n"+extraYAML)
	apEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	apEscribir(t, root, "app.go", "package app\n")
	apCommit(t, root, "inicial")
	return root
}

func apItemTexto(slug, extra string) string {
	return "titulo: Tarjeta " + slug + "\ntipo: feature\nprioridad: media\n" +
		"pedido: el pedido de " + slug + "\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n" + extra
}

// apItem escribe el item de la tarjeta en el proyecto y lo guarda.
func apItem(t *testing.T, root, slug, extra string) {
	t.Helper()
	apEscribir(t, root, ".hoom/items/"+slug+".yaml", apItemTexto(slug, extra))
	apCommit(t, root, "item "+slug)
}

func apTarea(t *testing.T, root, slug string) string {
	t.Helper()
	if err := taskcmd.Start(root, slug, apBase); err != nil {
		t.Fatalf("fixture: hoom task start %s: %v", slug, err)
	}
	return filepath.Join(root, ".hoom", "worktrees", slug)
}

func apSpecRel(slug string) string { return ".hoom/specs/" + slug + ".md" }

func apSpecTexto(criterios string) string {
	return "# Spec\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n" + criterios + "\n\n## Decisiones\nx\n\n## Riesgos\nx\n"
}

func apFirmar(t *testing.T, wt, slug string) {
	t.Helper()
	if _, _, err := approval.Approve(wt, filepath.Join(wt, filepath.FromSlash(apSpecRel(slug)))); err != nil {
		t.Fatalf("fixture: aprobar %s: %v", slug, err)
	}
}

func apColumna(t *testing.T, ca, root, slug, col string) {
	t.Helper()
	c, err := boardcmd.CardFor(root, apBase, apBlockOn, slug, time.Now().UTC())
	if err != nil || c.Column != col {
		t.Fatalf("%s: fixture: la tarjeta %s debia estar en %s: %q %v", ca, slug, col, c.Column, err)
	}
}

// apTuAprobacion: item (con extra), tarea abierta y un spec completo sin
// firma en el espacio de trabajo; guardado o no.
func apTuAprobacion(t *testing.T, root, slug, extra string, guardado bool) string {
	t.Helper()
	apItem(t, root, slug, extra)
	wt := apTarea(t, root, slug)
	apEscribir(t, wt, apSpecRel(slug), apSpecTexto("- CA-950: todavia nadie lo cita."))
	if guardado {
		apCommit(t, wt, "spec sin aprobar")
	}
	apColumna(t, "CA-647", root, slug, boardcmd.ColTuAprobacion)
	return wt
}

// apTestWriter: spec firmado y guardado, con un criterio sin test.
func apTestWriter(t *testing.T, root, slug, extra string) string {
	t.Helper()
	apItem(t, root, slug, extra)
	wt := apTarea(t, root, slug)
	apEscribir(t, wt, apSpecRel(slug), apSpecTexto("- CA-951: todavia nadie lo cita."))
	apFirmar(t, wt, slug)
	apCommit(t, wt, "spec aprobado")
	apColumna(t, "CA-661", root, slug, boardcmd.ColTestWriter)
	return wt
}

// --- CLIs de IA falsos ---

type apFalsos struct{ bin, argv string }

// apPATH deja un PATH minimo: git y sh del sistema y los falsos que se
// instalen despues. Ningun claude, codex, gemini ni opencode real entra.
func apPATH(t *testing.T) *apFalsos {
	t.Helper()
	f := &apFalsos{bin: t.TempDir(), argv: t.TempDir()}
	t.Setenv("PATH", f.bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	return f
}

// cli instala un CLI falso que guarda su stdin y sus argumentos y sale 0.
func (f *apFalsos) cli(t *testing.T, name string) {
	t.Helper()
	final := filepath.Join(f.argv, name)
	tmp := filepath.Join(f.argv, ".tmp-"+name)
	in := filepath.Join(f.argv, "in-"+name)
	s := "#!/bin/sh\n" +
		"cat > '" + in + ".'$$\n" +
		"printf '%s\\000' \"$@\" > '" + tmp + ".'$$\n" +
		"mv '" + tmp + ".'$$ '" + final + ".'$$\nexit 0\n"
	if err := os.WriteFile(filepath.Join(f.bin, name), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (f *apFalsos) llamadas(t *testing.T, name string) int {
	t.Helper()
	entries, err := os.ReadDir(f.argv)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), name+".") {
			n++
		}
	}
	return n
}

func (f *apFalsos) ninguna(t *testing.T, ca, que string) {
	t.Helper()
	for _, name := range []string{"claude", "codex", "gemini", "opencode"} {
		if n := f.llamadas(t, name); n != 0 {
			t.Fatalf("%s: %s no lanza nada, y %s corrio %d veces", ca, que, name, n)
		}
	}
}

// --- correr el piloto ---

// apBuf es un buffer que se puede escribir desde varias goroutines.
type apBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (w *apBuf) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.Write(p)
}

func (w *apBuf) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.b.String()
}

type apSalida struct {
	code     int
	out, err string
}

func (s apSalida) String() string {
	return fmt.Sprintf("exit %d\nstdout:\n%s\nstderr:\n%s", s.code, s.out, s.err)
}

// apRun corre `hoom piloto <args>` en dir. Un panico o una vuelta que no
// termina son fallas del criterio, no la caida del paquete.
func apRun(t *testing.T, ca, dir string, args ...string) apSalida {
	t.Helper()
	var out, errb apBuf
	listo := make(chan int, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(&errb, "PANICO: %v", r)
				listo <- -1
			}
		}()
		listo <- pilotocmd.Run(args, pilotocmd.Entorno{Dir: dir, Out: &out, Err: &errb})
	}()
	select {
	case code := <-listo:
		s := apSalida{code: code, out: out.String(), err: errb.String()}
		if code == -1 {
			t.Fatalf("%s: hoom piloto %q entro en panico:\n%s", ca, args, s)
		}
		return s
	case <-time.After(4 * time.Minute):
		t.Fatalf("%s: hoom piloto %q no termino en 4 minutos:\n%s\n%s", ca, args, out.String(), errb.String())
		return apSalida{}
	}
}

func apLineas(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimRight(l, "\r"))
		}
	}
	return out
}

// apSoloLineasDelPiloto exige que todo lo que imprime sea una linea del
// piloto.
func apSoloLineasDelPiloto(t *testing.T, ca string, s apSalida) []string {
	t.Helper()
	ls := apLineas(s.out)
	for _, l := range ls {
		if !strings.HasPrefix(l, apPrefijo) {
			t.Fatalf("%s: hoom piloto imprime una linea por decision, todas del piloto: %q\n%s", ca, l, s)
		}
	}
	return ls
}

func apUltima(ls []string) string {
	if len(ls) == 0 {
		return ""
	}
	return ls[len(ls)-1]
}

func apHead(t *testing.T, dir string) string { return apGit(t, dir, "rev-parse", "HEAD") }

// ===========================================================================
// CA-647: hoom piloto <slug> [--json]
// ===========================================================================

// CA-647: lo que no entiende (sin slug, un flag desconocido, solo --json)
// es un error de uso: exit 2 con el bloque de uso y sin efectos.
func TestCA647_UsoEsErrorDeUso(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	wt := apTuAprobacion(t, root, "uso", apConAuto, false)
	head := apHead(t, wt)
	uso := strings.SplitN(pilotocmd.UsageText, "\n", 2)[0]
	if uso != "Uso: hoom piloto <slug> [--json]" {
		t.Fatalf("CA-647: el uso empieza con `Uso: hoom piloto <slug> [--json]`: %q", uso)
	}
	for _, args := range [][]string{nil, {}, {"--json"}, {"uso", "--nope"}, {"--nope", "uso"}, {"-x"}} {
		s := apRun(t, "CA-647", root, args...)
		if s.code != 2 || !strings.Contains(s.out+s.err, uso) {
			t.Fatalf("CA-647: hoom piloto %q es un error de uso (exit 2 con el uso):\n%s", args, s)
		}
		if strings.Contains(s.out+s.err, apPrefijo) {
			t.Fatalf("CA-647: un error de uso no corre ninguna vuelta: %q\n%s", args, s)
		}
	}
	if h := apHead(t, wt); h != head {
		t.Fatalf("CA-647: un error de uso no guarda nada: HEAD %s -> %s", head, h)
	}
	if recs := envelope.List(root); len(recs) != 0 {
		t.Fatalf("CA-647: un error de uso no lanza nada: %+v", recs)
	}
	f.ninguna(t, "CA-647", "un error de uso")
}

// CA-647: el candado y el texto del contrato.
func TestCA647_CandadoYTextoDelContrato(t *testing.T) {
	root := "/proyecto/demo"
	if got := pilotocmd.Candado(root, "precios"); got != filepath.Join(root, ".hoom", "cache", "locks", "piloto-precios.lock") {
		t.Fatalf("CA-647: el candado es .hoom/cache/locks/piloto-<slug>.lock: %q", got)
	}
	if got := pilotocmd.MsgYaCorre("precios"); got != "el piloto de precios ya corre en otro proceso" {
		t.Fatalf("CA-647: el texto del contrato: %q", got)
	}
}

// CA-647: con el candado de la tarjeta tomado por otro, hoom piloto sale con
// 1 y `hoom: el piloto de <slug> ya corre en otro proceso` sin correr ninguna
// vuelta (no guarda); tambien desde el worktree de la tarea y con --json.
// Correr devuelve un error con ese texto; Avanzar no lanza ni guarda. Al
// soltar el candado corre, y al terminar lo suelta.
func TestCA647_ConOtroPilotoCorriendoSaleCon1(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	const slug = "ocupada"
	wt := apTuAprobacion(t, root, slug, apConAuto, false)
	head := apHead(t, wt)

	soltar, err := hoomfs.Lock(pilotocmd.Candado(root, slug), 0)
	if err != nil {
		t.Fatalf("fixture: tomar el candado del piloto: %v", err)
	}
	for _, c := range []struct {
		dir  string
		args []string
	}{{root, []string{slug}}, {wt, []string{slug}}, {root, []string{slug, "--json"}}, {root, []string{"--json", slug}}} {
		s := apRun(t, "CA-647", c.dir, c.args...)
		if s.code != 1 || !strings.Contains(s.err, "hoom: "+pilotocmd.MsgYaCorre(slug)) {
			t.Fatalf("CA-647: con otro piloto corriendo (%s %q) sale con 1 y %q:\n%s", c.dir, c.args, "hoom: "+pilotocmd.MsgYaCorre(slug), s)
		}
		if strings.Contains(s.out+s.err, apPrefijo) {
			t.Fatalf("CA-647: con el candado tomado no corre ninguna vuelta:\n%s", s)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if _, err := pilotocmd.Correr(ctx, pilotocmd.Opciones{Root: root, Slug: slug, Proveedores: apNingunProveedor}); err == nil ||
		!strings.Contains(err.Error(), pilotocmd.MsgYaCorre(slug)) {
		t.Fatalf("CA-647: Correr con el candado tomado devuelve un error con %q: %v", pilotocmd.MsgYaCorre(slug), err)
	}
	res, err := pilotocmd.Avanzar(ctx, pilotocmd.Opciones{Root: root, Slug: slug, Proveedores: apNingunProveedor})
	dice := err != nil && strings.Contains(err.Error(), pilotocmd.MsgYaCorre(slug))
	for _, l := range res.Lineas {
		dice = dice || strings.Contains(l, pilotocmd.MsgYaCorre(slug))
	}
	for _, d := range res.Decisiones {
		dice = dice || strings.Contains(d.Why, pilotocmd.MsgYaCorre(slug))
	}
	if !dice || res.Trabajo != nil {
		t.Fatalf("CA-647: Avanzar con el candado tomado se detiene con %q y no lanza: %+v %v", pilotocmd.MsgYaCorre(slug), res, err)
	}
	if h := apHead(t, wt); h != head {
		t.Fatalf("CA-644/CA-647: sin el candado no hay vuelta ni guardado: HEAD %s -> %s", head, h)
	}
	if st := apGit(t, wt, "status", "--porcelain"); !strings.Contains(st, ".hoom/") {
		t.Fatalf("CA-647: el spec sigue sin guardar: %q", st)
	}
	soltar()

	s := apRun(t, "CA-647", root, slug)
	if s.code != 0 {
		t.Fatalf("CA-647: con el candado libre corre y se detiene en Tu aprobacion con 0:\n%s", s)
	}
	otra, err := hoomfs.Lock(pilotocmd.Candado(root, slug), 0)
	if err != nil {
		t.Fatalf("CA-647: al terminar, hoom piloto suelta el candado: %v", err)
	}
	otra()
	f.ninguna(t, "CA-647", "el piloto en una columna humana")
}

// CA-647: detenido en una columna humana (Tu aprobacion) sale con 0, con una
// linea por decision; --json da {slug, lineas, exit_code} con las mismas
// lineas y el mismo exit.
func TestCA647_EnUnaColumnaHumanaSaleCon0(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	const slug = "firma-pendiente"
	wt := apTuAprobacion(t, root, slug, apConAuto, true)
	head := apHead(t, wt)

	s := apRun(t, "CA-647", root, slug)
	ls := apSoloLineasDelPiloto(t, "CA-647", s)
	if s.code != 0 || len(ls) != 1 || !strings.HasPrefix(ls[0], apDetiene) {
		t.Fatalf("CA-647: sin nada que guardar, una decision (se detiene) y exit 0 en Tu aprobacion:\n%s", s)
	}
	if h := apHead(t, wt); h != head {
		t.Fatalf("CA-644: sin cambios no commitea nada: HEAD %s -> %s", head, h)
	}

	s2 := apRun(t, "CA-647", root, slug, "--json")
	if s2.code != 0 {
		t.Fatalf("CA-647: --json sale con el mismo exit (0):\n%s", s2)
	}
	var r struct {
		Slug     *string  `json:"slug"`
		Lineas   []string `json:"lineas"`
		ExitCode *int     `json:"exit_code"`
	}
	if err := json.Unmarshal([]byte(s2.out), &r); err != nil {
		t.Fatalf("CA-647: con --json stdout es el Resultado como JSON, en vez de las lineas: %v\n%s", err, s2)
	}
	if r.Slug == nil || *r.Slug != slug || r.ExitCode == nil || *r.ExitCode != 0 || len(r.Lineas) != 1 || r.Lineas[0] != ls[0] {
		t.Fatalf("CA-647: el JSON trae slug, lineas y exit_code: %s", s2.out)
	}
	f.ninguna(t, "CA-647", "el piloto en Tu aprobacion")
}

// CA-647: cualquier otra detencion sale con 1: sin auto (caso 1, y entonces
// no guarda aunque haya cambios) y sin la tarea abierta (caso 4, con el texto
// del contrato); --json dice el mismo exit_code.
func TestCA647_OtraDetencionSaleCon1(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")

	const manual = "a-mano"
	wt := apTuAprobacion(t, root, manual, apSinAuto, false)
	head := apHead(t, wt)
	s := apRun(t, "CA-647", root, manual)
	ls := apSoloLineasDelPiloto(t, "CA-647", s)
	if s.code != 1 || len(ls) != 1 || !strings.HasPrefix(ls[0], apDetiene) {
		t.Fatalf("CA-647: sin auto se detiene (una linea) con exit 1:\n%s", s)
	}
	if h := apHead(t, wt); h != head {
		t.Fatalf("CA-644: sin auto no guarda: HEAD %s -> %s", head, h)
	}
	if st := apGit(t, wt, "status", "--porcelain"); st == "" {
		t.Fatal("CA-644: sin auto el spec sigue sin guardar")
	}

	const sinTarea = "sin-tarea"
	apItem(t, root, sinTarea, apConAuto)
	s = apRun(t, "CA-647", root, sinTarea)
	ls = apSoloLineasDelPiloto(t, "CA-647", s)
	if s.code != 1 || len(ls) != 1 || ls[0] != apSinTarea(sinTarea) {
		t.Fatalf("CA-647: sin tarea abierta, exit 1 y %q:\n%s", apSinTarea(sinTarea), s)
	}
	s = apRun(t, "CA-647", root, sinTarea, "--json")
	var r struct {
		ExitCode *int     `json:"exit_code"`
		Lineas   []string `json:"lineas"`
	}
	if err := json.Unmarshal([]byte(s.out), &r); err != nil || s.code != 1 || r.ExitCode == nil || *r.ExitCode != 1 ||
		len(r.Lineas) != 1 || r.Lineas[0] != apSinTarea(sinTarea) {
		t.Fatalf("CA-647: --json con exit_code 1 y la linea de la detencion: %v\n%s", err, s)
	}
	f.ninguna(t, "CA-647", "un piloto que se detiene")
}

// ===========================================================================
// CA-644: guardar
// ===========================================================================

// CA-644: con auto y la tarea abierta, antes de decidir commitea lo no
// guardado del worktree en hoom/<slug> con `hoom: guardar la tarjeta
// <slug>` (y lo dice con su commit); no toca el checkout principal (el item
// cambiado ahi queda sin guardar), no hace push, y la vuelta siguiente, sin
// cambios, no commitea nada.
func TestCA644_GuardaEnLaRamaDeLaTarea(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	remoto := filepath.Join(t.TempDir(), "remoto.git")
	apGit(t, root, "init", "-q", "--bare", remoto)
	apGit(t, root, "remote", "add", "origin", remoto)
	apGit(t, root, "push", "-q", "origin", apBase)
	const slug = "guarda"
	wt := apTuAprobacion(t, root, slug, apConAuto, false)
	apEscribir(t, wt, "app.go", "package app\n\n// lo que la persona dejo a medias\n")
	apEscribir(t, wt, "notas/precios.go", "package notas\n")
	// el item cambiado con hoom item set, sin guardar, en el principal
	apEscribir(t, root, ".hoom/items/"+slug+".yaml", strings.Replace(apItemTexto(slug, apConAuto), "prioridad: media", "prioridad: alta", 1))
	headRaiz, ramaRaiz := apHead(t, root), apGit(t, root, "rev-parse", "--abbrev-ref", "HEAD")
	stRaiz := apGit(t, root, "status", "--porcelain")
	if !strings.Contains(stRaiz, ".hoom/items/"+slug+".yaml") {
		t.Fatalf("fixture: el item del principal esta sin guardar: %q", stRaiz)
	}
	refsRemoto := apGit(t, root, "--git-dir="+remoto, "for-each-ref", "--format=%(refname) %(objectname)")
	headTarea := apHead(t, wt)

	s := apRun(t, "CA-644", root, slug)
	ls := apSoloLineasDelPiloto(t, "CA-644", s)
	if s.code != 0 || len(ls) != 2 || !strings.HasPrefix(ls[1], apDetiene) {
		t.Fatalf("CA-644: guarda y despues decide (se detiene en Tu aprobacion, exit 0):\n%s", s)
	}
	m := apGuarda.FindStringSubmatch(ls[0])
	if m == nil {
		t.Fatalf("CA-644/CA-661: la primera linea es `piloto automatico: guarda el trabajo de la tarjeta: <commit>`: %q", ls[0])
	}
	nuevo := apHead(t, wt)
	if nuevo == headTarea || !strings.HasPrefix(nuevo, m[1]) {
		t.Fatalf("CA-644: el commit de la linea (%s) es el nuevo HEAD de la tarea (%s, antes %s)", m[1], nuevo, headTarea)
	}
	if msg := apGit(t, wt, "log", "-1", "--format=%s"); msg != apGuardaMs+slug {
		t.Fatalf("CA-644: el mensaje es %q: %q", apGuardaMs+slug, msg)
	}
	if r := apGit(t, wt, "rev-parse", "--abbrev-ref", "HEAD"); r != "hoom/"+slug {
		t.Fatalf("CA-644: commitea en la rama de la tarea hoom/%s: %q", slug, r)
	}
	if p := apGit(t, wt, "rev-parse", "HEAD^"); p != headTarea {
		t.Fatalf("CA-644: un solo commit de guardado sobre lo que habia: padre %s, antes %s", p, headTarea)
	}
	archivos := "\n" + apGit(t, wt, "show", "--name-only", "--format=", "HEAD") + "\n"
	for _, a := range []string{apSpecRel(slug), "app.go", "notas/precios.go"} {
		if !strings.Contains(archivos, "\n"+a+"\n") {
			t.Fatalf("CA-644: el guardado lleva %s: %q", a, archivos)
		}
	}
	if strings.Contains(archivos, ".hoom/items/") {
		t.Fatalf("CA-644: el guardado de la tarea no lleva el item del principal: %q", archivos)
	}
	if st := apGit(t, wt, "status", "--porcelain"); st != "" {
		t.Fatalf("CA-644: no queda nada sin guardar en la tarea: %q", st)
	}
	if h, r, st := apHead(t, root), apGit(t, root, "rev-parse", "--abbrev-ref", "HEAD"), apGit(t, root, "status", "--porcelain"); h != headRaiz || r != ramaRaiz || st != stRaiz {
		t.Fatalf("CA-644: nada en el checkout principal (el item queda para hoom item save): HEAD %s -> %s, rama %s -> %s, status %q -> %q",
			headRaiz, h, ramaRaiz, r, stRaiz, st)
	}
	if refs := apGit(t, root, "--git-dir="+remoto, "for-each-ref", "--format=%(refname) %(objectname)"); refs != refsRemoto {
		t.Fatalf("CA-644: nunca hace push: el remoto paso de %q a %q", refsRemoto, refs)
	}

	s = apRun(t, "CA-644", root, slug)
	ls = apSoloLineasDelPiloto(t, "CA-644", s)
	if s.code != 0 || len(ls) != 1 || !strings.HasPrefix(ls[0], apDetiene) || apHead(t, wt) != nuevo {
		t.Fatalf("CA-644: sin cambios no commitea nada ni lo dice:\n%s", s)
	}
	f.ninguna(t, "CA-644", "el piloto en Tu aprobacion")
}

// CA-644: si no puede guardar (el worktree fuera de su rama) se detiene con
// `no pudo guardar el trabajo de la tarjeta: <error>`, exit 1, sin commitear
// nada en ninguna rama.
func TestCA644_SinPoderGuardarSeDetiene(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "claude")
	for _, fuera := range [][]string{{"checkout", "-q", "--detach"}, {"checkout", "-q", "-b", "otra-rama"}} {
		root := apProyecto(t, "")
		const slug = "fuera-de-su-rama"
		wt := apTuAprobacion(t, root, slug, apConAuto, true)
		rama := apGit(t, wt, "rev-parse", "hoom/"+slug)
		apGit(t, wt, fuera...)
		apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
		head := apHead(t, wt)

		s := apRun(t, "CA-644", root, slug)
		ls := apSoloLineasDelPiloto(t, "CA-644", s)
		if s.code != 1 || !strings.HasPrefix(apUltima(ls), apDetiene) || !strings.Contains(apUltima(ls), "no pudo guardar el trabajo de la tarjeta: ") {
			t.Fatalf("CA-644: (%q) sin poder guardar se detiene con `no pudo guardar el trabajo de la tarjeta: <error>` y exit 1:\n%s", fuera, s)
		}
		for _, l := range ls {
			if apGuarda.MatchString(l) {
				t.Fatalf("CA-644: (%q) no dice que guardo: %q", fuera, l)
			}
		}
		if h, r := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug); h != head || r != rama {
			t.Fatalf("CA-644: (%q) no commitea nada: HEAD %s -> %s, hoom/%s %s -> %s", fuera, head, h, slug, rama, r)
		}
		if st := apGit(t, wt, "status", "--porcelain"); !strings.Contains(st, "app.go") {
			t.Fatalf("CA-644: (%q) lo no guardado sigue ahi: %q", fuera, st)
		}
	}
	f.ninguna(t, "CA-644", "un piloto que no pudo guardar")
}

// CA-644 y CA-647: Avanzar (la piel del Studio) tambien guarda antes de
// decidir y devuelve la detencion como Resultado: sus lineas, sus
// decisiones (la ultima, detenerse), ningun trabajo y exit 0.
func TestCA644_AvanzarGuardaYDevuelveLaDetencion(t *testing.T) {
	apPATH(t)
	root := apProyecto(t, "")
	const slug = "avanza"
	wt := apTuAprobacion(t, root, slug, apConAuto, false)
	head := apHead(t, wt)
	var salida apBuf
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := pilotocmd.Avanzar(ctx, pilotocmd.Opciones{Root: root, Slug: slug, Salida: &salida, Proveedores: apNingunProveedor})
	if err != nil {
		t.Fatalf("CA-647: Avanzar sobre una tarjeta legible no es un error: %v", err)
	}
	if res.Slug != slug || res.ExitCode != 0 || res.Trabajo != nil || len(res.Lineas) != 2 || !apGuarda.MatchString(res.Lineas[0]) ||
		!strings.HasPrefix(res.Lineas[1], apDetiene) {
		t.Fatalf("CA-644/CA-647: Avanzar guarda, decide y se detiene: %+v", res)
	}
	if len(res.Decisiones) == 0 || res.Decisiones[len(res.Decisiones)-1].Paso != boardcmd.PasoDetenerse ||
		res.Decisiones[len(res.Decisiones)-1].Launch {
		t.Fatalf("CA-647: la ultima decision es la detencion: %+v", res.Decisiones)
	}
	if got := apLineas(salida.String()); strings.Join(got, "\n") != strings.Join(res.Lineas, "\n") {
		t.Fatalf("CA-647: Salida recibe cada linea del piloto: %q vs %q", got, res.Lineas)
	}
	if apHead(t, wt) == head || apGit(t, wt, "log", "-1", "--format=%s") != apGuardaMs+slug {
		t.Fatalf("CA-644: Avanzar guardo en la rama de la tarea")
	}
}

// ===========================================================================
// CA-648: AvisoDeFirma
// ===========================================================================

// CA-648: el aviso es la linea del contrato solo cuando la tarjeta de esa
// spec tiene auto: hasta-humano en su item, leido como el riesgo (el del
// principal cuando lo tiene); "" si la spec no es de ninguna tarjeta o su
// tarjeta no tiene piloto. Ruta relativa al proyecto o absoluta. No lanza ni
// escribe nada.
func TestCA648_AvisoDeFirma(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	apItem(t, root, "con-piloto", apConAuto)
	apItem(t, root, "sin-piloto", apSinAuto)
	wt := apTuAprobacion(t, root, "en-su-tarea", apConAuto, false)
	apTuAprobacion(t, root, "auto-en-la-copia", apSinAuto, true)
	wtCopia := filepath.Join(root, ".hoom", "worktrees", "auto-en-la-copia")
	apEscribir(t, wtCopia, ".hoom/items/auto-en-la-copia.yaml", apItemTexto("auto-en-la-copia", apConAuto))
	apCommit(t, wtCopia, "auto solo en la copia de la tarea")
	apItem(t, root, "auto-sin-guardar", apSinAuto)
	apEscribir(t, root, ".hoom/items/auto-sin-guardar.yaml", apItemTexto("auto-sin-guardar", apConAuto))
	headRaiz, stRaiz, headWt := apHead(t, root), apGit(t, root, "status", "--porcelain"), apHead(t, wt)

	casos := map[string]string{
		apSpecRel("con-piloto"):                                 apAviso("con-piloto"),
		filepath.Join(root, ".hoom", "specs", "con-piloto.md"):  apAviso("con-piloto"),
		apSpecRel("en-su-tarea"):                                apAviso("en-su-tarea"),
		filepath.Join(root, ".hoom", "specs", "en-su-tarea.md"): apAviso("en-su-tarea"),
		apSpecRel("auto-sin-guardar"):                           apAviso("auto-sin-guardar"),
		apSpecRel("sin-piloto"):                                 "",
		filepath.Join(root, ".hoom", "specs", "sin-piloto.md"):  "",
		apSpecRel("auto-en-la-copia"):                           "",
		apSpecRel("de-nadie"):                                   "",
		filepath.Join(root, ".hoom", "specs", "de-nadie.md"):    "",
		"app.go": "",
		"":       "",
	}
	for spec, want := range casos {
		if got := pilotocmd.AvisoDeFirma(root, spec); got != want {
			t.Fatalf("CA-648: AvisoDeFirma(root, %q) = %q, quiero %q", spec, got, want)
		}
	}
	if recs := envelope.List(root); len(recs) != 0 {
		t.Fatalf("CA-648: el aviso no lanza nada: %+v", recs)
	}
	if h, st, hw := apHead(t, root), apGit(t, root, "status", "--porcelain"), apHead(t, wt); h != headRaiz || st != stRaiz || hw != headWt {
		t.Fatalf("CA-648: el aviso no escribe ni guarda nada: %s/%q/%s -> %s/%q/%s", headRaiz, stRaiz, headWt, h, st, hw)
	}
	f.ninguna(t, "CA-648", "AvisoDeFirma")
}

// ===========================================================================
// CA-661: las lineas del piloto y lo que lanza
// ===========================================================================

func apRegistro(t *testing.T, ca, root, id string) envelope.Record {
	t.Helper()
	for _, r := range envelope.List(root) {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("%s: el sobre %s de la linea esta en .hoom/envelopes/", ca, id)
	return envelope.Record{}
}

// apLanzo busca la linea `regla <regla>` seguida de la de pedir; devuelve la
// de pedir.
func apLanzo(t *testing.T, ca string, ls []string, regla string) string {
	t.Helper()
	for i, l := range ls {
		if l == apPrefijo+"regla "+regla {
			if i+1 >= len(ls) || !strings.HasPrefix(ls[i+1], apPrefijo+"pide al ") {
				t.Fatalf("%s: despues de la linea de regla viene la de pedir: %q", ca, ls)
			}
			return ls[i+1]
		}
	}
	t.Fatalf("%s: antes de lanzar el piloto escribe %q: %q", ca, apPrefijo+"regla "+regla, ls)
	return ""
}

// CA-661: hoom piloto en una tarjeta en Test-writer lanza por lanzar (regla
// cadena, claude con lo que queda del presupuesto: 5 USD), espera el trabajo
// y sigue hasta detenerse; el sobre lleva piloto, regla y columna, y la
// linea de la detencion queda tambien en el log del sobre cerrado.
func TestCA661_LaTerminalLanzaConSuRegla(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	const slug = "cadena"
	apTestWriter(t, root, slug, apConAuto)

	s := apRun(t, "CA-661", root, slug)
	ls := apSoloLineasDelPiloto(t, "CA-661", s)
	pide := apLanzo(t, "CA-661", ls, boardcmd.ReglaCadena)
	m := apPide.FindStringSubmatch(pide)
	if m == nil {
		t.Fatalf("CA-661: la linea de pedir es la de hoy, `pide al <rol> con <provider> (<x> USD): sobre <id>`: %q", pide)
	}
	if usd, _ := strconv.ParseFloat(m[3], 64); m[1] != "test-writer" || m[2] != "claude" || usd != 5 {
		t.Fatalf("CA-661: pide al test-writer con claude (5 USD): %q", pide)
	}
	rec := apRegistro(t, "CA-661", root, m[4])
	if !rec.Pilot || rec.Regla != boardcmd.ReglaCadena || rec.Columna != boardcmd.ColTestWriter || rec.Role != "test-writer" ||
		rec.Provider != "claude" || rec.Task != slug || rec.Pedido == "" || rec.Intento != 0 || rec.Ronda != 0 {
		t.Fatalf("CA-661: el sobre del piloto lleva piloto, regla cadena, columna y pedido, sin intento ni ronda: %+v", rec)
	}
	if !rec.Done() {
		t.Fatalf("CA-647: hoom piloto espera cada trabajo que lanza: %+v", rec)
	}
	if s.code != 1 || !strings.HasPrefix(apUltima(ls), apDetiene) {
		t.Fatalf("CA-647: despues del test-writer (que no escribio nada) se detiene con exit 1:\n%s", s)
	}
	if n := f.llamadas(t, "claude"); n != 1 {
		t.Fatalf("CA-661: un lanzamiento, claude corrio %d veces", n)
	}
	log, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, rec.ID+".log"))
	if err != nil || !strings.Contains(string(log), apUltima(ls)) {
		t.Fatalf("CA-661: las lineas del piloto van al log del sobre cerrado mas nuevo de la tarjeta: %v\n%s", err, log)
	}
}

// CA-661 y CA-659: con ventana_max declarada, un provider sin tope en USD
// (codex) se lanza sin presupuesto, y la linea lo dice `(sin tope en USD)`.
// Sin la seccion autonomia, el piloto no le pide nada a ese provider.
func TestCA661_SinTopeEnUSDLoDice(t *testing.T) {
	f := apPATH(t)
	f.cli(t, "codex")
	root := apProyecto(t, "autonomia:\n  ventana_max: 80\n")
	const slug = "sin-usd"
	apTestWriter(t, root, slug, apConAuto)
	s := apRun(t, "CA-661", root, slug)
	ls := apSoloLineasDelPiloto(t, "CA-661", s)
	pide := apLanzo(t, "CA-661", ls, boardcmd.ReglaCadena)
	m := apPideSinUSD.FindStringSubmatch(pide)
	if m == nil || m[1] != "test-writer" || m[2] != "codex" {
		t.Fatalf("CA-661: `pide al test-writer con codex (sin tope en USD): sobre <id>`: %q", pide)
	}
	if rec := apRegistro(t, "CA-661", root, m[3]); !rec.Pilot || rec.Regla != boardcmd.ReglaCadena || rec.Provider != "codex" {
		t.Fatalf("CA-661: el sobre es del piloto, con su regla: %+v", rec)
	}

	f2 := apPATH(t)
	f2.cli(t, "codex")
	root2 := apProyecto(t, "")
	apTestWriter(t, root2, slug, apConAuto)
	s = apRun(t, "CA-661", root2, slug)
	ls = apSoloLineasDelPiloto(t, "CA-661", s)
	if s.code != 1 || len(ls) != 1 || !strings.HasPrefix(ls[0], apDetiene) || f2.llamadas(t, "codex") != 0 {
		t.Fatalf("CA-661: sin seccion autonomia no le pide nada a un provider sin tope en USD:\n%s", s)
	}
}

// ===========================================================================
// CA-656: el paso cerrar-hallazgos
// ===========================================================================

type apRondaFix struct {
	root, wt, slug string
	f1, f2, f0     string // f0: un bloqueante sin tarea ("" si no hay)
	vTW, vW        string // el veredicto del test-writer de la ronda y el del writer
	veredictos     map[string]bool
}

func apVeredicto(t *testing.T, wt, slug string, cuando time.Time, falla string) string {
	t.Helper()
	var gates []verdict.GateResult
	for _, n := range []string{"spec_lint", "spec_trace", "spec_approved", finding.GateName, "test"} {
		st := verdict.StatusPass
		if n == falla {
			st = verdict.StatusFail
		}
		gates = append(gates, verdict.GateResult{Name: n, Required: true, Status: st})
	}
	v := &verdict.Verdict{Project: "demo", CreatedAt: cuando, Spec: apSpecRel(slug), Git: gitx.Snapshot(wt, apBase), Gates: gates}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	if v.ID == "" {
		t.Fatal("fixture: el veredicto tiene id")
	}
	return v.ID
}

func apVeredictos(t *testing.T, wt string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	entries, err := os.ReadDir(filepath.Join(wt, ".hoom", "verdicts"))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out[strings.TrimSuffix(e.Name(), ".json")] = true
		}
	}
	return out
}

// apNuevos son los veredictos de wt que no estaban en antes, leidos.
func apNuevos(t *testing.T, wt string, antes map[string]bool) map[string]verdict.Verdict {
	t.Helper()
	out := map[string]verdict.Verdict{}
	for id := range apVeredictos(t, wt) {
		if antes[id] {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(wt, ".hoom", "verdicts", id+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var v verdict.Verdict
		if err := json.Unmarshal(raw, &v); err != nil {
			t.Fatalf("CA-656: el veredicto nuevo %s es JSON: %v", id, err)
		}
		out[id] = v
	}
	return out
}

// apRonda arma la tarjeta despues del writer de la ronda 1: spec firmado,
// precios_test.go del test-writer de antes de la ronda (cita CA-1),
// hallazgos_test.go del test-writer de la ronda (cita f1 y, si f2En es
// "ronda", f2; con "antes", a f2 lo cita solo precios_test.go), el arreglo
// commiteado, el veredicto rojo esperado del test-writer de la ronda y el
// del writer, rojo solo por findings_open. Los sobres: test-writer, writer,
// reviewer, y la ronda 1 (refutador, test-writer, writer). Todo guardado.
func apRonda(t *testing.T, f2En string, sinTarea bool) apRondaFix {
	t.Helper()
	root := apProyecto(t, "autonomia:\n  rondas_de_hallazgos: 1\n")
	const slug = "ronda"
	apItem(t, root, slug, apConAuto)
	wt := apTarea(t, root, slug)
	apEscribir(t, wt, apSpecRel(slug), apSpecTexto("- CA-1: el total redondea bien."))
	apFirmar(t, wt, slug)
	apEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el total redondea bien\n")
	apCommit(t, wt, "spec firmado y test del test-writer")

	fx := apRondaFix{root: root, wt: wt, slug: slug}
	reg := func(desc string) string {
		h, err := finding.Register(wt, apBase, finding.Draft{Severity: "high", Lens: "correctness", File: "app.go",
			Description: desc, Author: "reviewer", Task: slug})
		if err != nil {
			t.Fatalf("fixture: registrar el hallazgo: %v", err)
		}
		time.Sleep(1100 * time.Millisecond) // ids en orden
		return h.ID
	}
	fx.f1 = reg("el total redondea mal con descuentos")
	fx.f2 = reg("la region se pierde al recargar")
	if sinTarea {
		h, err := finding.Add(wt, apBase, "high", "correctness", "app.go", "un hallazgo que no es de ninguna tarea", "reviewer")
		if err != nil {
			t.Fatalf("fixture: hallazgo sin tarea: %v", err)
		}
		fx.f0 = h.ID
	}
	ronda := "package app\n\n// " + fx.f1 + ": el total con descuento\n"
	switch f2En {
	case "ronda":
		ronda += "// " + fx.f2 + ": la region al recargar\n"
	case "antes":
		apEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el total redondea bien\n// "+fx.f2+": la region al recargar\n")
	}
	apEscribir(t, wt, "hallazgos_test.go", ronda)
	apEscribir(t, wt, "app.go", "package app\n\n// el arreglo de la ronda 1\n")
	apCommit(t, wt, "tests de la ronda y arreglo")
	ahora := time.Now().UTC()
	fx.vTW = apVeredicto(t, wt, slug, ahora.Add(-3*time.Minute), "test")
	fx.vW = apVeredicto(t, wt, slug, ahora.Add(-time.Minute), finding.GateName)
	apCommit(t, wt, "veredictos")
	fx.veredictos = apVeredictos(t, wt)

	ids := []string{fx.f1, fx.f2}
	pasos := map[string]int{"run": 3, "scope": 4, "verify": 5, "ok": 6}
	base := ahora.Add(-2 * time.Hour)
	for i, r := range []envelope.Record{
		{Role: "test-writer", Columna: boardcmd.ColTestWriter, Stage: "verify", Status: envelope.StatusNotDeliverable,
			Trasplantados: []string{"precios_test.go"}, Pedido: "los tests del spec"},
		{Role: "writer", Columna: boardcmd.ColWriter, Stage: "ok", Status: envelope.StatusDeliverable, Pedido: "implementa el spec"},
		{Role: "reviewer", Columna: boardcmd.ColReview, Stage: "ok", Status: envelope.StatusDeliverable},
		{Role: "refutador", Columna: boardcmd.ColReview, Stage: "ok", Status: envelope.StatusDeliverable, Pilot: true,
			Regla: boardcmd.ReglaRondaRefutador, Ronda: 1, Hallazgos: ids, Bloqueantes: len(ids), Pedido: "refuta"},
		{Role: "test-writer", Columna: boardcmd.ColReview, Stage: "verify", Status: envelope.StatusNotDeliverable, Pilot: true,
			Regla: boardcmd.ReglaRondaTestWriter, Ronda: 1, Hallazgos: ids, Trasplantados: []string{"hallazgos_test.go"},
			VerdictID: fx.vTW, Verdict: "red", Pedido: "reproduce los hallazgos"},
		{Role: "writer", Columna: boardcmd.ColReview, Stage: "verify", Status: envelope.StatusNotDeliverable, Pilot: true,
			Regla: boardcmd.ReglaRondaWriter, Ronda: 1, VerdictID: fx.vW, Verdict: "red", Pedido: "corrige el codigo"},
	} {
		ini := base.Add(time.Duration(i*10) * time.Minute)
		r.ID = ini.Format("20060102T150405") + fmt.Sprintf("_ae%04x", i+1)
		r.RunID = ""
		r.Provider, r.Task, r.Dir, r.Spec = "claude", slug, wt, apSpecRel(slug)
		r.Step, r.Steps = pasos[r.Stage], 6
		r.StartedAt, r.UpdatedAt, r.EndedAt = ini, ini.Add(5*time.Minute), ini.Add(5*time.Minute)
		if r.Status != envelope.StatusDeliverable {
			r.ExitCode = 1
		}
		if err := envelope.Write(root, r); err != nil {
			t.Fatalf("fixture: sobre %s: %v", r.Role, err)
		}
	}
	if n := len(envelope.List(root)); n != 6 {
		t.Fatalf("fixture: los seis sobres de la tarjeta estan en disco: %d", n)
	}
	return fx
}

func apItemDe(t *testing.T, wt, id string) finding.Item {
	t.Helper()
	items, _, err := finding.List(wt, apBase, false)
	if err != nil {
		t.Fatalf("CA-656: listar hallazgos: %v", err)
	}
	for _, it := range items {
		if it.ID == id {
			return it
		}
	}
	t.Fatalf("CA-656: el hallazgo %s sigue en disco", id)
	return finding.Item{}
}

func apEvidencia(fx apRondaFix, ruta string) string {
	return "ronda 1: el test " + ruta + " cita el hallazgo; veredicto " + fx.vTW + " rojo antes del arreglo; despues, veredicto " +
		fx.vW + " con todos los demas gates en pass"
}

// apCerrado exige el cierre de hoom: corregido, autor hoom piloto, fuera de
// una corrida y con la evidencia literal.
func apCerrado(t *testing.T, ca string, fx apRondaFix, id, ruta string) {
	t.Helper()
	it := apItemDe(t, fx.wt, id)
	if it.Status != finding.StatusCorrected || it.Resolution == nil || it.Resolution.As != finding.StatusCorrected ||
		it.Resolution.Author != finding.AutorPiloto || it.Resolution.Role != "" || it.Resolution.Run != "" {
		t.Fatalf("%s: %s queda corregido por %q, fuera de una corrida: %+v %+v", ca, id, finding.AutorPiloto, it, it.Resolution)
	}
	if it.Resolution.Evidence != apEvidencia(fx, ruta) {
		t.Fatalf("%s: la evidencia de %s nombra la ronda, el test y los dos veredictos:\n quiero %q\n fue    %q", ca, id, apEvidencia(fx, ruta), it.Resolution.Evidence)
	}
}

func apAbierto(t *testing.T, ca string, fx apRondaFix, id string) {
	t.Helper()
	if it := apItemDe(t, fx.wt, id); it.Status != finding.StatusOpen || it.Resolution != nil {
		t.Fatalf("%s: %s sigue abierto: %+v", ca, id, it)
	}
}

// apTrasCerrar: lo comun despues del paso: una linea de cierre con los ids,
// un solo veredicto nuevo de la tarjeta, el espacio de trabajo sin nada por
// commitear y el ultimo commit es el guardado. Devuelve el veredicto nuevo.
func apTrasCerrar(t *testing.T, ca string, fx apRondaFix, s apSalida, ids []string) (string, verdict.Verdict) {
	t.Helper()
	ls := apSoloLineasDelPiloto(t, ca, s)
	cierre := fmt.Sprintf("%scierra %d hallazgos con su test (ronda 1): %s", apPrefijo, len(ids), strings.Join(ids, ", "))
	n := 0
	for _, l := range ls {
		if l == cierre {
			n++
		}
		if n == 0 && (strings.Contains(l, " pide al ") || strings.Contains(l, "no pudo lanzar") || strings.Contains(l, " regla ")) {
			t.Fatalf("%s: despues del writer de la ronda, el paso es cerrar-hallazgos (lo hace hoom, sin modelo): %q\n%s", ca, l, s)
		}
	}
	if n != 1 {
		t.Fatalf("%s: una linea %q:\n%s", ca, cierre, s)
	}
	nuevos := apNuevos(t, fx.wt, fx.veredictos)
	if len(nuevos) != 1 {
		t.Fatalf("%s: despues de cerrar hay un veredicto nuevo de la tarjeta: %d", ca, len(nuevos))
	}
	var id string
	var v verdict.Verdict
	for k, nv := range nuevos {
		id, v = k, nv
	}
	if v.Spec != apSpecRel(fx.slug) {
		t.Fatalf("%s: el veredicto nuevo es el de la tarjeta (verify --spec): %q", ca, v.Spec)
	}
	if st := apGit(t, fx.wt, "status", "--porcelain"); st != "" {
		t.Fatalf("%s: el espacio de trabajo queda sin cambios por commitear: %q", ca, st)
	}
	if msg := apGit(t, fx.wt, "log", "-1", "--format=%s"); msg != apGuardaMs+fx.slug {
		t.Fatalf("%s: guarda el espacio de trabajo: el ultimo commit es %q", ca, msg)
	}
	enCommit := apGit(t, fx.wt, "ls-files", ".hoom/verdicts/"+id+".json")
	if enCommit == "" {
		t.Fatalf("%s: el veredicto nuevo queda guardado", ca)
	}
	return id, v
}

// CA-656 y caso limite "un hallazgo que el test-writer no logra reproducir
// junto a otros que si": f1 lo cita el test de la ronda y f2 solo un test de
// antes de la ronda. hoom cierra f1 (y nada mas), corre verify, guarda, y se
// detiene con `quedan hallazgos sin test en la ronda 1: <f2>: te toca`
// (aclaracion 10), exit 1. Ningun CLI corre.
func TestCA656_CierraLosCitadosYSeDetieneConLosQueQuedan(t *testing.T) {
	f := apPATH(t)
	fx := apRonda(t, "antes", false)
	s := apRun(t, "CA-656", fx.root, fx.slug)
	apTrasCerrar(t, "CA-656", fx, s, []string{fx.f1})
	apCerrado(t, "CA-656", fx, fx.f1, "hallazgos_test.go")
	apAbierto(t, "CA-656", fx, fx.f2)
	ls := apLineas(s.out)
	if want := apDetiene + "quedan hallazgos sin test en la ronda 1: " + fx.f2 + ": te toca"; s.code != 1 || apUltima(ls) != want {
		t.Fatalf("CA-656: con un bloqueante sin test de la ronda, se detiene con %q y exit 1:\n%s", want, s)
	}
	if n := len(envelope.List(fx.root)); n != 6 {
		t.Fatalf("CA-656: cerrar-hallazgos no lanza ningun trabajo: %d sobres", n)
	}
	f.ninguna(t, "CA-656", "cerrar-hallazgos")
}

// CA-656: con los dos bloqueantes citados por el test de la ronda, hoom los
// cierra a los dos (en el orden de los bloqueantes), el veredicto nuevo
// queda verde y no se detiene con ningun texto de rojo.
func TestCA656_CierraTodosLosCitadosYQuedaVerde(t *testing.T) {
	f := apPATH(t)
	fx := apRonda(t, "ronda", false)
	s := apRun(t, "CA-656", fx.root, fx.slug)
	_, v := apTrasCerrar(t, "CA-656", fx, s, []string{fx.f1, fx.f2})
	apCerrado(t, "CA-656", fx, fx.f1, "hallazgos_test.go")
	apCerrado(t, "CA-656", fx, fx.f2, "hallazgos_test.go")
	if v.Verdict != "green" {
		t.Fatalf("CA-656: sin bloqueantes, el verify de la tarjeta queda verde: %q %+v", v.Verdict, v.Gates)
	}
	for _, l := range apLineas(s.out) {
		if strings.Contains(l, "sigue rojo") || strings.Contains(l, "quedan hallazgos sin test") {
			t.Fatalf("CA-656: con el veredicto nuevo verde no se detiene por rojo: %q\n%s", l, s)
		}
	}
	f.ninguna(t, "CA-656", "cerrar-hallazgos")
}

// CA-656 y caso limite "un hallazgo sin tarea que bloquea": findings_open lo
// cuenta, cerrar-hallazgos no lo toca, el veredicto nuevo sigue rojo y, sin
// bloqueantes de la tarjeta que queden, se detiene con `despues de cerrar los
// hallazgos de la ronda 1, el veredicto <id> sigue rojo en findings_open: te
// toca` (aclaracion 10), con el id del veredicto nuevo.
func TestCA656_UnHallazgoSinTareaSigueRojo(t *testing.T) {
	f := apPATH(t)
	fx := apRonda(t, "ronda", true)
	s := apRun(t, "CA-656", fx.root, fx.slug)
	id, v := apTrasCerrar(t, "CA-656", fx, s, []string{fx.f1, fx.f2})
	apCerrado(t, "CA-656", fx, fx.f1, "hallazgos_test.go")
	apCerrado(t, "CA-656", fx, fx.f2, "hallazgos_test.go")
	apAbierto(t, "CA-656", fx, fx.f0)
	if v.Verdict == "green" {
		t.Fatalf("CA-656: el hallazgo sin tarea deja rojo el veredicto nuevo: %+v", v.Gates)
	}
	want := apDetiene + "despues de cerrar los hallazgos de la ronda 1, el veredicto " + id + " sigue rojo en " + finding.GateName + ": te toca"
	if ls := apLineas(s.out); s.code != 1 || apUltima(ls) != want {
		t.Fatalf("CA-656: se detiene con %q y exit 1:\n%s", want, s)
	}
	if n := len(envelope.List(fx.root)); n != 6 {
		t.Fatalf("CA-656: cerrar-hallazgos no lanza ningun trabajo: %d sobres", n)
	}
	f.ninguna(t, "CA-656", "cerrar-hallazgos")
}
