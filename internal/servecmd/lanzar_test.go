// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-336..CA-341): pedir trabajo desde la tarjeta. POST
// /api/board/{slug}/launch vuelve a derivar la tarjeta, valida en el orden del
// contrato y lanza el MISMO sobre (agentcmd) o la MISMA review (reviewcmd) que
// la CLI, en una goroutine del Studio. Los CLIs de IA son falsos: van al frente
// de un PATH minimo que deja afuera a los que esta maquina tenga instalados de
// verdad, y cada invocacion deja sus argumentos en disco para mirarlos.
package servecmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const lnBase = "main"

// --- los CLIs falsos ---

type lnFakes struct {
	bin  string // al frente del PATH
	argv string // cada invocacion deja aca sus argumentos, separados por NUL
	gate string // los CLIs que esperan terminan cuando este archivo existe
}

// lnPATH deja un PATH minimo: git y sh del sistema y los CLIs falsos que se
// instalen despues. Ningun claude, codex, gemini ni opencode real entra.
func lnPATH(t *testing.T) *lnFakes {
	t.Helper()
	f := &lnFakes{bin: t.TempDir(), argv: t.TempDir()}
	f.gate = filepath.Join(t.TempDir(), "soltar")
	t.Setenv("PATH", f.bin+":/usr/bin:/bin:/usr/sbin:/sbin")
	return f
}

// cli instala un CLI de IA falso: guarda sus argumentos y, si espera, no
// termina hasta que el test lo suelta (con un tope de 30 s).
func (f *lnFakes) cli(t *testing.T, name string, espera bool) {
	t.Helper()
	tmp := filepath.Join(f.argv, ".tmp-"+name)
	final := filepath.Join(f.argv, name)
	s := "#!/bin/sh\n" +
		"printf '%s\\000' \"$@\" > '" + tmp + ".'$$\n" +
		"mv '" + tmp + ".'$$ '" + final + ".'$$\n"
	if espera {
		s += "i=0\nwhile [ ! -f '" + f.gate + "' ] && [ $i -lt 600 ]; do sleep 0.05; i=$((i+1)); done\n"
	}
	s += "exit 0\n"
	if err := os.WriteFile(filepath.Join(f.bin, name), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

// llamadas devuelve los argumentos de cada invocacion del CLI name.
func (f *lnFakes) llamadas(t *testing.T, name string) [][]string {
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

func (f *lnFakes) soltar() { os.WriteFile(f.gate, []byte("ya\n"), 0o644) }

func lnSinLlamadas(t *testing.T, ca, que string, f *lnFakes) {
	t.Helper()
	for _, name := range []string{"claude", "codex"} {
		if n := len(f.llamadas(t, name)); n > 0 {
			t.Fatalf("%s: %s ejecuto %s %d veces", ca, que, name, n)
		}
	}
}

// lnSigue devuelve el argumento que sigue a flag.
func lnSigue(args []string, flag string) (string, bool) {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

func lnTiene(args []string, flag string) bool {
	for _, a := range args {
		if a == flag {
			return true
		}
	}
	return false
}

func lnUltimo(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[len(args)-1]
}

// --- el Studio ---

// lnServidor arma el Studio y, al terminar el test, suelta los CLIs que
// esperan y espera a que cierre todo lo que lanzo: un sobre que sigue
// escribiendo en un TempDir que testing ya esta borrando rompe la limpieza.
func lnServidor(t *testing.T, root string, f *lnFakes) *Server {
	t.Helper()
	s := newServer(t, root)
	t.Cleanup(func() {
		if f != nil {
			f.soltar()
		}
		listo := make(chan struct{})
		go func() { s.waitLaunches(); close(listo) }()
		select {
		case <-listo:
		case <-time.After(60 * time.Second):
			t.Error("CA-336..CA-341: los trabajos que lanzo el Studio no terminaron en 60 s")
		}
	})
	return s
}

func lnEsperarLanzamientos(t *testing.T, ca string, s *Server) {
	t.Helper()
	listo := make(chan struct{})
	go func() { s.waitLaunches(); close(listo) }()
	select {
	case <-listo:
	case <-time.After(60 * time.Second):
		t.Fatalf("%s: los trabajos que lanzo el Studio no terminaron en 60 s", ca)
	}
}

// lnEsperar sondea cond hasta 20 s.
func lnEsperar(cond func() bool) bool {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return cond()
}

func lnCuerpo(action, provider, model string, budget any, pedido string) map[string]any {
	return map[string]any{"action": action, "provider": provider, "model": model, "budget_usd": budget, "pedido": pedido}
}

// lnLaunch hace el POST de launch; un body string viaja tal cual.
func lnLaunch(t *testing.T, s *Server, slug, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var raw []byte
	if str, ok := body.(string); ok {
		raw = []byte(str)
	} else {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			t.Fatal(err)
		}
	}
	return doPOST(t, s, "/api/board/"+slug+"/launch", token, raw, "application/json")
}

type lnLanzado struct {
	EnvelopeID  string `json:"envelope_id"`
	Role        string `json:"role"`
	Provider    string `json:"provider"`
	TaskStarted bool   `json:"task_started"`
}

// lnAceptado exige el 202 con {envelope_id, role, provider, task_started}.
func lnAceptado(t *testing.T, ca string, rec *httptest.ResponseRecorder) lnLanzado {
	t.Helper()
	if rec.Code != http.StatusAccepted {
		t.Fatalf("%s: el launch responde 202, fue %d: %s", ca, rec.Code, rec.Body.String())
	}
	var claves map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &claves); err != nil {
		t.Fatalf("%s: el 202 es JSON: %v (%s)", ca, err, rec.Body.String())
	}
	for _, k := range []string{"envelope_id", "role", "provider", "task_started"} {
		if _, ok := claves[k]; !ok {
			t.Fatalf("%s: al 202 le falta %q: %s", ca, k, rec.Body.String())
		}
	}
	var r lnLanzado
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("%s: el 202 trae tipos del contrato: %v (%s)", ca, err, rec.Body.String())
	}
	if r.EnvelopeID == "" {
		t.Fatalf("%s: el 202 nombra el sobre: %s", ca, rec.Body.String())
	}
	return r
}

// lnRegistro lee el registro del sobre del arbol raiz.
func lnRegistro(t *testing.T, ca, root, id string) envelope.Record {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, id+".json"))
	if err != nil {
		t.Fatalf("%s: el registro del sobre %s esta en .hoom/envelopes/ del proyecto: %v", ca, id, err)
	}
	var rec envelope.Record
	if err := json.Unmarshal(raw, &rec); err != nil {
		t.Fatalf("%s: el registro %s es JSON: %v", ca, id, err)
	}
	return rec
}

// lnRegistros lista los .json de .hoom/envelopes/ del arbol raiz.
func lnRegistros(t *testing.T, root string) []string {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(root, ".hoom", envelope.DirName))
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// --- la tarjeta ---

func lnCarta(t *testing.T, root, slug string) boardcmd.Card {
	t.Helper()
	c, err := boardcmd.CardFor(root, lnBase, tbBlockOn, slug, time.Now().UTC())
	if err != nil {
		t.Fatalf("CA-336..CA-343: CardFor %s: %v", slug, err)
	}
	return c
}

func lnColumna(t *testing.T, ca, root, slug, col string) boardcmd.Card {
	t.Helper()
	c := lnCarta(t, root, slug)
	if c.Column != col {
		t.Fatalf("%s: fixture: la tarjeta %s debia estar en %s y esta en %s (%s)", ca, slug, col, c.Column, c.Plain)
	}
	return c
}

func lnAccion(c boardcmd.Card, id string) (boardcmd.Action, bool) {
	for _, a := range c.Actions {
		if a.ID == id {
			return a, true
		}
	}
	return boardcmd.Action{}, false
}

// lnYaNoEsta es el 409 de una accion que la tarjeta ya no tiene.
func lnYaNoEsta(c boardcmd.Card) string {
	return "la tarjeta ya no esta donde la viste: ahora esta en " + c.ColumnName + " (" + c.Plain + ")"
}

// --- fixtures ---

func lnTarea(t *testing.T, ca, root, slug string) string {
	t.Helper()
	if err := taskcmd.Start(root, slug, lnBase); err != nil {
		t.Fatalf("%s: fixture: hoom task start %s: %v", ca, slug, err)
	}
	return filepath.Join(root, ".hoom", "worktrees", slug)
}

func lnTestDe(slug string) string { return strings.ReplaceAll(slug, "-", "_") + "_test.go" }

func lnCommit(t *testing.T, dir, msg string) {
	t.Helper()
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", msg)
}

// lnCartaWriter: item en la raiz y espacio de trabajo con el spec aprobado y
// todos sus criterios citados, sin veredicto (C1: writer).
func lnCartaWriter(t *testing.T, ca, root, slug, extra string) string {
	t.Helper()
	tbItem(t, root, slug, extra)
	wt := lnTarea(t, ca, root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-901: citado por un test.", true))
	tbEscribir(t, wt, lnTestDe(slug), "package app\n\n// CA-901\n")
	tbAprobar(t, wt, slug)
	lnCommit(t, wt, "spec aprobado y sus tests")
	lnColumna(t, ca, root, slug, boardcmd.ColWriter)
	return wt
}

// lnCartaTestWriter: espacio de trabajo con el spec aprobado y un criterio
// que ningun test cita (C1: test-writer).
func lnCartaTestWriter(t *testing.T, ca, root, slug string) string {
	t.Helper()
	tbItem(t, root, slug, "")
	wt := lnTarea(t, ca, root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-902: todavia nadie lo cita.", true))
	tbAprobar(t, wt, slug)
	lnCommit(t, wt, "spec aprobado")
	lnColumna(t, ca, root, slug, boardcmd.ColTestWriter)
	return wt
}

// lnCartaTestWriterSinEspacio: la misma columna con la evidencia en el arbol
// del proyecto (sin espacio de trabajo propio).
func lnCartaTestWriterSinEspacio(t *testing.T, ca, root, slug string) {
	t.Helper()
	tbItem(t, root, slug, "")
	tbEscribir(t, root, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-906: todavia nadie lo cita.", true))
	tbAprobar(t, root, slug)
	lnColumna(t, ca, root, slug, boardcmd.ColTestWriter)
}

// lnVeredicto escribe un veredicto verde de la tarjeta con la huella actual
// de dir; lineas > 0 fija el tamano que el veredicto declara.
func lnVeredicto(t *testing.T, dir, slug string, lineas int) {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: time.Now().UTC().Add(-time.Minute),
		Spec: ".hoom/specs/" + slug + ".md", Git: gitx.Snapshot(dir, lnBase), Gates: tbGatesVerdes()}
	if lineas > 0 {
		v.Git.Insertions, v.Git.Deletions = lineas, 0
	}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
}

// lnCartaReview: verde con la huella actual, mas de 400 lineas cambiadas y
// sin registro de review (C1: review, con la review de 4 lentes exigida).
func lnCartaReview(t *testing.T, ca, root, slug string) string {
	t.Helper()
	tbItem(t, root, slug, "")
	wt := lnTarea(t, ca, root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-903: citado por un test.", true))
	tbEscribir(t, wt, lnTestDe(slug), "package app\n\n// CA-903\n")
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; i < 450; i++ {
		fmt.Fprintf(&b, "var grande%d = %d\n", i, i)
	}
	tbEscribir(t, wt, "grande.go", b.String())
	tbAprobar(t, wt, slug)
	lnCommit(t, wt, "cambio grande")
	lnVeredicto(t, wt, slug, 0)
	lnCommit(t, wt, "veredicto")
	if c := lnColumna(t, ca, root, slug, boardcmd.ColReview); !c.Evidence.ReviewRequired {
		t.Fatalf("%s: fixture: la review de 4 lentes tiene que ser exigida: %+v", ca, c.Evidence)
	}
	return wt
}

// lnCartaReviewSinCambios: todo vive en main y el espacio de trabajo no
// cambia nada contra la base, pero su veredicto declara 500 lineas. La
// tarjeta pide la review; hoom review decide, antes de la primera pasada,
// que no hay nada que revisar.
func lnCartaReviewSinCambios(t *testing.T, ca, root, slug string) string {
	t.Helper()
	tbItem(t, root, slug, "")
	tbEscribir(t, root, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-907: citado por un test.", true))
	tbEscribir(t, root, lnTestDe(slug), "package app\n\n// CA-907\n")
	tbAprobar(t, root, slug)
	lnCommit(t, root, "todo en main")
	wt := lnTarea(t, ca, root, slug)
	lnVeredicto(t, wt, slug, 500)
	lnCommit(t, wt, "veredicto")
	if c := lnColumna(t, ca, root, slug, boardcmd.ColReview); !c.Evidence.ReviewRequired {
		t.Fatalf("%s: fixture: la review de 4 lentes tiene que ser exigida: %+v", ca, c.Evidence)
	}
	if g := gitx.Snapshot(wt, lnBase); len(g.ChangedFiles) != 0 {
		t.Fatalf("%s: fixture: el espacio de trabajo no cambia nada contra la base: %v", ca, g.ChangedFiles)
	}
	return wt
}

// lnSidecar deja el sidecar de un run como lo habria dejado otro proceso.
func lnSidecar(t *testing.T, root string, m runcmd.Meta) {
	t.Helper()
	dir := filepath.Join(root, ".hoom", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, m.ID+".meta.json"), append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// lnGasto registra un run terminado de la tarjeta que costo costo USD.
func lnGasto(t *testing.T, ca, root, slug, wt string, costo float64) {
	t.Helper()
	c := costo
	lnSidecar(t, root, runcmd.Meta{ID: "20260923T060000_gasto1", Provider: "claude", Role: "writer", Task: slug, Dir: wt,
		CreatedAt: time.Now().UTC().Add(-time.Hour), Status: runcmd.StatusDone, ExitCode: 0,
		EndedAt: time.Now().UTC().Add(-50 * time.Minute), Usage: &providers.Usage{CostUSD: &c, Turns: 3}})
	if got := lnCarta(t, root, slug).Spend.CostUSD; got == nil || *got != costo {
		t.Fatalf("%s: fixture: la tarjeta %s gasto %v USD: %v", ca, slug, costo, got)
	}
}

// lnPIDMuerto devuelve el PID de un proceso que ya termino.
func lnPIDMuerto(t *testing.T) int {
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
	t.Fatal("CA-341: fixture: no consegui el PID de un proceso muerto")
	return 0
}

// lnCartaInterrumpida: una tarjeta en Writer cuyo sobre de writer quedo
// abierto con el PID de un hoom serve que murio en el paso run, y el sidecar
// de su run con el id de sesion de Claude. Devuelve el espacio de trabajo, el
// id del sobre muerto y los bytes de su registro.
func lnCartaInterrumpida(t *testing.T, ca, root, slug string) (string, string, []byte) {
	t.Helper()
	wt := lnCartaWriter(t, ca, root, slug, "")
	muerto := lnPIDMuerto(t)
	ahora := time.Now().UTC()
	runID, envID := "20260923T080000_run0ld", "20260923T075959_env0ld"
	lnSidecar(t, root, runcmd.Meta{ID: runID, Provider: "claude", Role: "writer", Task: slug, Dir: wt,
		CreatedAt: ahora.Add(-2 * time.Hour), Status: runcmd.StatusRunning, ExitCode: -1,
		ProviderSessionID: "sesion-vieja-123", PID: muerto})
	rec := envelope.Record{ID: envID, Role: "writer", Provider: "claude", Task: slug, Dir: wt,
		Spec: filepath.Join(wt, ".hoom", "specs", slug+".md"), Approval: "aprobado", RunID: runID,
		Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: ahora.Add(-2 * time.Hour), UpdatedAt: ahora.Add(-time.Hour), PID: muerto}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	tbEscribir(t, root, ".hoom/envelopes/"+envID+".json", string(raw))
	if c := lnCarta(t, root, slug); c.Interrupted == nil || c.Interrupted.EnvelopeID != envID {
		t.Fatalf("%s: fixture: la tarjeta %s esta interrumpida por %s: %+v", ca, slug, envID, c.Interrupted)
	}
	return wt, envID, raw
}

// lnHuellas fotografia lo que un launch puede dejar: tareas (espacios de
// trabajo y ramas), registros y logs de sobres, runs y arboles ciegos, en la
// raiz y en cada espacio de trabajo.
func lnHuellas(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	add := func(base string) {
		filepath.WalkDir(base, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			raw, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			out[filepath.ToSlash(rel)] = fmt.Sprintf("%x", sha256.Sum256(raw))
			return nil
		})
	}
	hoom := filepath.Join(root, ".hoom")
	for _, d := range []string{"envelopes", "runs", "isolated"} {
		add(filepath.Join(hoom, d))
	}
	entries, _ := os.ReadDir(filepath.Join(hoom, "worktrees"))
	for _, e := range entries {
		out[".hoom/worktrees/"+e.Name()+"/"] = "tarea"
		for _, d := range []string{"envelopes", "runs", "isolated"} {
			add(filepath.Join(hoom, "worktrees", e.Name(), ".hoom", d))
		}
	}
	cmd := exec.Command("git", "branch", "--list", "hoom/*")
	cmd.Dir = root
	ramas, _ := cmd.Output()
	out["(ramas)"] = strings.TrimSpace(string(ramas))
	return out
}

// lnSinEfectos: nada nuevo, nada cambiado, nada borrado. conLog tolera un log
// de sobre nuevo (una review que se decidio sin pasadas no deja registro).
func lnSinEfectos(t *testing.T, ca, que string, antes, despues map[string]string, conLog bool) {
	t.Helper()
	for k, v := range despues {
		w, ok := antes[k]
		switch {
		case !ok && conLog && strings.HasPrefix(k, ".hoom/envelopes/") && strings.HasSuffix(k, ".log"):
		case !ok:
			t.Fatalf("%s: %s dejo %s en disco", ca, que, k)
		case w != v:
			t.Fatalf("%s: %s modifico %s:\nantes:   %s\ndespues: %s", ca, que, k, w, v)
		}
	}
	for k := range antes {
		if _, ok := despues[k]; !ok {
			t.Fatalf("%s: %s borro %s", ca, que, k)
		}
	}
}

// ---------------------------------------------------------------------------

// CA-336: pedir-arquitecto sobre una tarjeta en Backlog sin espacio de
// trabajo corre hoom task start y lanza el sobre del arquitecto con provider,
// modelo, presupuesto, pedido y task = el slug, sin --spec. 202 con
// task_started y el registro ya en disco; mientras el sobre corre, la tarjeta
// sigue en Backlog con su fantasma en Arquitecto; la salida queda en
// .hoom/envelopes/<id>.log.
func TestCA336_PedirAlArquitectoDesdeBacklog(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", true)
	root := tbProyecto(t)
	tbItem(t, root, "nueva", "presupuesto_usd: 5\n")
	lnColumna(t, "CA-336", root, "nueva", boardcmd.ColBacklog)
	wt := filepath.Join(root, ".hoom", "worktrees", "nueva")
	if _, err := os.Stat(wt); err == nil {
		t.Fatal("CA-336: fixture: la tarjeta empieza sin espacio de trabajo")
	}
	s := lnServidor(t, root, f)

	pedido := "Escribi el spec de precios por region: una tabla por pais."
	r := lnAceptado(t, "CA-336", lnLaunch(t, s, "nueva", s.Token(),
		lnCuerpo("pedir-arquitecto", "claude", "opus-prueba", 1.5, pedido)))
	if !r.TaskStarted || r.Role != "arquitecto" || r.Provider != "claude" {
		t.Fatalf("CA-336: el 202 dice rol arquitecto, provider claude y task_started true: %+v", r)
	}

	// hoom task start: el espacio de trabajo y la rama de la tarea
	if st, err := os.Stat(wt); err != nil || !st.IsDir() {
		t.Fatalf("CA-336: el launch corre hoom task start: falta %s (%v)", wt, err)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/hoom/nueva").Run(); err != nil {
		t.Fatalf("CA-336: hoom task start crea la rama hoom/nueva: %v", err)
	}

	// el registro ya esta en disco cuando llega el 202
	reg := lnRegistro(t, "CA-336", root, r.EnvelopeID)
	if reg.Role != "arquitecto" || reg.Provider != "claude" || reg.Task != "nueva" || reg.Dir != wt {
		t.Fatalf("CA-336: el sobre es el del arquitecto con task = el slug, en su espacio de trabajo: %+v", reg)
	}

	// el CLI recibe el modelo, el tope y el pedido
	var args []string
	if !lnEsperar(func() bool {
		if ll := f.llamadas(t, "claude"); len(ll) > 0 {
			args = ll[0]
			return true
		}
		return false
	}) {
		t.Fatal("CA-336: el sobre del arquitecto nunca ejecuto el CLI de claude")
	}
	if v, ok := lnSigue(args, "--model"); !ok || v != "opus-prueba" {
		t.Fatalf("CA-336: el modelo del dialogo llega al CLI (--model opus-prueba): %q", args)
	}
	if v, ok := lnSigue(args, "--max-budget-usd"); !ok || v != "1.5" {
		t.Fatalf("CA-336: el presupuesto del dialogo llega al CLI (--max-budget-usd 1.5): %q", args)
	}
	if lnUltimo(args) != pedido {
		t.Fatalf("CA-336: el pedido del dialogo llega tal cual: %q", lnUltimo(args))
	}

	// mientras corre: la tarjeta sigue en Backlog y su fantasma esta en Arquitecto
	var c boardcmd.Card
	lnEsperar(func() bool {
		c = lnCarta(t, root, "nueva")
		return c.Ghost != nil && c.Ghost.Stage == "run" && c.Ghost.RunID != ""
	})
	if c.Ghost == nil {
		t.Fatalf("CA-336: mientras el sobre corre la tarjeta tiene fantasma: running=%+v", c.Running)
	}
	g := c.Ghost
	if c.Column != boardcmd.ColBacklog {
		t.Fatalf("CA-336: la tarjeta sigue en su columna mientras corre, esta en %s", c.Column)
	}
	if g.Column != boardcmd.ColArquitecto || g.Role != "arquitecto" || g.Provider != "claude" ||
		g.EnvelopeID != r.EnvelopeID || g.Stage != "run" || g.RunID == "" {
		t.Fatalf("CA-336: el fantasma esta en Arquitecto con el rol, el provider y el paso del sobre: %+v", g)
	}
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-336: GET /api/board: %d %s", rec.Code, rec.Body.String())
	}
	var tab struct {
		Columns []struct {
			ID    string `json:"id"`
			Cards []struct {
				Slug  string          `json:"slug"`
				Ghost *boardcmd.Ghost `json:"ghost"`
			} `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tab); err != nil {
		t.Fatalf("CA-336: /api/board es JSON: %v", err)
	}
	visto := false
	for _, col := range tab.Columns {
		for _, card := range col.Cards {
			if card.Slug == "nueva" {
				visto = true
				if col.ID != boardcmd.ColBacklog || card.Ghost == nil || card.Ghost.Column != boardcmd.ColArquitecto {
					t.Fatalf("CA-336: /api/board pinta la tarjeta en Backlog y su fantasma en Arquitecto: %s %+v", col.ID, card.Ghost)
				}
			}
		}
	}
	if !visto {
		t.Fatal("CA-336: /api/board trae la tarjeta nueva")
	}

	f.soltar()
	lnEsperarLanzamientos(t, "CA-336", s)
	log, err := os.ReadFile(filepath.Join(root, ".hoom", "envelopes", r.EnvelopeID+".log"))
	if err != nil || !strings.Contains(string(log), "hoom agent: rol arquitecto") {
		t.Fatalf("CA-336: la salida del sobre queda en .hoom/envelopes/%s.log: %v\n%s", r.EnvelopeID, err, log)
	}
	if reg = lnRegistro(t, "CA-336", root, r.EnvelopeID); !reg.Done() || reg.Spec != "" {
		t.Fatalf("CA-336: el sobre cerro y corrio sin --spec (el arquitecto escribe el spec): %+v", reg)
	}
}

// CA-337: pedir-test-writer y pedir-writer lanzan el sobre con --spec
// .hoom/specs/<slug>.md resuelto en el espacio de trabajo de la tarjeta.
func TestCA337_TestWriterYWriterTrabajanContraElSpec(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	wtTW := lnCartaTestWriter(t, "CA-337", root, "faltan-tests")
	wtW := lnCartaWriter(t, "CA-337", root, "a-implementar", "")
	s := lnServidor(t, root, f)

	casos := []struct{ slug, action, role, wt, pedido string }{
		{"faltan-tests", "pedir-test-writer", "test-writer", wtTW, "Escribi los tests del criterio que falta."},
		{"a-implementar", "pedir-writer", "writer", wtW, "Implementa el spec hasta que verify de verde."},
	}
	for _, k := range casos {
		r := lnAceptado(t, "CA-337", lnLaunch(t, s, k.slug, s.Token(), lnCuerpo(k.action, "claude", "", nil, k.pedido)))
		if r.Role != k.role || r.Provider != "claude" || r.TaskStarted {
			t.Fatalf("CA-337: %s responde rol %s, claude y sin task_started (la tarea ya existia): %+v", k.action, k.role, r)
		}
		lnEsperarLanzamientos(t, "CA-337", s)
		reg := lnRegistro(t, "CA-337", root, r.EnvelopeID)
		spec := filepath.Join(k.wt, ".hoom", "specs", k.slug+".md")
		if reg.Role != k.role || reg.Task != k.slug || reg.Dir != k.wt || reg.Spec != spec {
			t.Fatalf("CA-337: %s corre el sobre con task = el slug y --spec %s en su espacio de trabajo: %+v", k.action, spec, reg)
		}
	}
	hay := false
	for _, args := range f.llamadas(t, "claude") {
		if lnUltimo(args) == casos[1].pedido {
			hay = true
		}
	}
	if !hay {
		t.Fatal("CA-337: el writer recibe el pedido del dialogo tal cual")
	}
}

// CA-337: pedir-reviewer corre hoom review (las 4 lentes, cada una con el
// pedido que arma la review) con task y spec, y rechaza un pedido escrito.
func TestCA337_PedirAlReviewerCorreHoomReview(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	wt := lnCartaReview(t, "CA-337", root, "revisar")
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	tbErrorJSON(t, "CA-337", lnLaunch(t, s, "revisar", s.Token(),
		lnCuerpo("pedir-reviewer", "claude", "", nil, "revisa con cuidado")), http.StatusBadRequest)
	lnSinEfectos(t, "CA-337", "un pedido para el reviewer", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-337", "un pedido para el reviewer", f)

	r := lnAceptado(t, "CA-337", lnLaunch(t, s, "revisar", s.Token(),
		lnCuerpo("pedir-reviewer", "claude", "", nil, "")))
	if r.Role != "reviewer" || r.Provider != "claude" {
		t.Fatalf("CA-337: pedir-reviewer responde rol reviewer: %+v", r)
	}
	lnEsperarLanzamientos(t, "CA-337", s)

	ll := f.llamadas(t, "claude")
	if len(ll) != len(reviewcmd.Lentes) {
		t.Fatalf("CA-337: pedir-reviewer es hoom review: una pasada por lente (%d), hubo %d", len(reviewcmd.Lentes), len(ll))
	}
	for _, args := range ll {
		if !strings.HasPrefix(lnUltimo(args), "Revisa el cambio de esta rama con la lente") {
			t.Fatalf("CA-337: la review arma su propio pedido: %q", lnUltimo(args))
		}
	}
	recs, avisos := reviewcmd.Records(wt) // el segundo valor son avisos, no un error
	if len(avisos) != 0 || len(recs) != 1 {
		t.Fatalf("CA-337: la review deja su registro en el espacio de trabajo: %v %+v", avisos, recs)
	}
	rv := recs[0]
	spec := ".hoom/specs/revisar.md"
	if rv.Task != "revisar" || (rv.Spec != spec && rv.Spec != filepath.Join(wt, spec)) ||
		len(rv.Lenses) != len(reviewcmd.Lentes) || rv.Provider != "claude" {
		t.Fatalf("CA-337: hoom review corre con task y spec de la tarjeta: %+v", rv)
	}
	if reg := lnRegistro(t, "CA-337", root, r.EnvelopeID); reg.Role != "reviewer" || reg.Task != "revisar" || reg.Provider != "claude" {
		t.Fatalf("CA-337: la review deja registro de sobre de reviewer: %+v", reg)
	}
}

// CA-337: una review que se decide antes de la primera pasada (aca, sin
// nada que revisar) no llama a Started: 409 con la nota, sin registro.
func TestCA337_ReviewSinPasadasEs409(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	lnCartaReviewSinCambios(t, "CA-337", root, "sin-cambios")
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	msg := tbErrorJSON(t, "CA-337", lnLaunch(t, s, "sin-cambios", s.Token(),
		lnCuerpo("pedir-reviewer", "claude", "", nil, "")), http.StatusConflict)
	if !strings.Contains(msg, "no hay nada que revisar") {
		t.Fatalf("CA-337: el 409 trae la nota de la review: %q", msg)
	}
	lnEsperarLanzamientos(t, "CA-337", s)
	lnSinEfectos(t, "CA-337", "una review sin pasadas", antes, lnHuellas(t, root), true)
	lnSinLlamadas(t, "CA-337", "una review sin pasadas", f)
}

// CA-337: sin --same-provider. Un writer de claude en el arbol de la tarjeta
// hace que la review con claude no sea cruzada: 409, sin registro ni run. Si
// la tarjeta ya lo sabe, el 409 es el why de la opcion claude (codex queda
// instalado para que la accion siga habilitada); si no, es la nota de la
// review, que se niega antes de la primera pasada.
func TestCA337_ReviewNoCruzadaEs409(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	f.cli(t, "codex", false)
	root := tbProyecto(t)
	wt := lnCartaReview(t, "CA-337", root, "cruce")
	// un writer que corrio en el arbol de la tarjeta con claude (sin tarea en
	// su sidecar: la tarjeta no lo ve, hoom review si)
	lnSidecar(t, root, runcmd.Meta{ID: "20260923T050000_escrib", Provider: "claude", Role: "writer", Dir: wt,
		CreatedAt: time.Now().UTC().Add(-3 * time.Hour), Status: runcmd.StatusDone, ExitCode: 0})
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	msg := tbErrorJSON(t, "CA-337", lnLaunch(t, s, "cruce", s.Token(),
		lnCuerpo("pedir-reviewer", "claude", "", nil, "")), http.StatusConflict)
	if !strings.Contains(msg, "cruzada") && msg != "claude escribio el codigo: la revision tiene que ser de otro proveedor" {
		t.Fatalf("CA-337: la review que no seria cruzada se niega con su nota: %q", msg)
	}
	lnEsperarLanzamientos(t, "CA-337", s)
	lnSinEfectos(t, "CA-337", "una review no cruzada", antes, lnHuellas(t, root), true)
	lnSinLlamadas(t, "CA-337", "una review no cruzada", f)
}

// CA-338: sin token o con un token equivocado, 401 y nada en disco: ni
// tarea, ni registro, ni run, ni log.
func TestCA338_SinTokenNoLanzaNada(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	tbItem(t, root, "nueva", "")
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	for _, token := range []string{"", "token-falso", strings.Repeat("0", len(s.Token()))} {
		rec := lnLaunch(t, s, "nueva", token, lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec."))
		tbErrorJSON(t, "CA-338", rec, http.StatusUnauthorized)
	}
	lnEsperarLanzamientos(t, "CA-338", s)
	lnSinEfectos(t, "CA-338", "un launch sin token", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-338", "un launch sin token", f)
	if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "nueva")); err == nil {
		t.Fatal("CA-338: el 401 no crea la tarea")
	}
}

// CA-338: columna equivocada (409 con donde esta), deshabilitada (409 con su
// why), clave o action desconocidas (400), pedido de relleno (400) y provider
// que no es opcion (400). En ninguno queda tarea, registro, run ni log.
func TestCA338_RechazosSinEfectos(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	tbItem(t, root, "nueva", "") // Backlog, sin espacio de trabajo
	lnCartaWriter(t, "CA-338", root, "en-writer", "")
	lnCartaTestWriterSinEspacio(t, "CA-338", root, "sin-espacio")
	nueva := lnColumna(t, "CA-338", root, "nueva", boardcmd.ColBacklog)
	enWriter := lnCarta(t, root, "en-writer")
	s := lnServidor(t, root, f)
	antes := lnHuellas(t, root)

	casos := []struct {
		que, slug string
		body      any
		code      int
		msg       string // "" = cualquier mensaje
	}{
		// una accion que la tarjeta no tiene: se movio desde que la viste
		{"pedir-writer en Backlog", "nueva", lnCuerpo("pedir-writer", "claude", "", nil, "Implementa."), http.StatusConflict, lnYaNoEsta(nueva)},
		{"pedir-arquitecto en Writer", "en-writer", lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec."), http.StatusConflict, lnYaNoEsta(enWriter)},
		{"pedir-test-writer en Writer", "en-writer", lnCuerpo("pedir-test-writer", "claude", "", nil, "Escribi tests."), http.StatusConflict, lnYaNoEsta(enWriter)},
		// deshabilitada: su why
		{"pedir-test-writer sin espacio de trabajo", "sin-espacio", lnCuerpo("pedir-test-writer", "claude", "", nil, "Escribi tests."), http.StatusConflict,
			"la tarjeta no tiene su propio espacio de trabajo: desde el tablero solo se le pide trabajo a una tarjeta que lo tiene"},
		// clave desconocida y tipo invalido
		{"una clave column", "nueva", map[string]any{"action": "pedir-arquitecto", "provider": "claude", "model": "", "budget_usd": nil,
			"pedido": "Escribi el spec.", "column": "arquitecto"}, http.StatusBadRequest, ""},
		{"action con column", "en-writer", `{"action": "pedir-writer", "column": "review"}`, http.StatusBadRequest, ""},
		{"budget_usd de texto", "nueva", lnCuerpo("pedir-arquitecto", "claude", "", "dos", "Escribi el spec."), http.StatusBadRequest, ""},
		{"cuerpo que no es JSON", "nueva", `{"action": `, http.StatusBadRequest, ""},
		// action desconocida
		{"action pedir-orquestador", "nueva", lnCuerpo("pedir-orquestador", "claude", "", nil, "Orquesta."), http.StatusBadRequest, ""},
		{"action mover", "nueva", lnCuerpo("mover", "claude", "", nil, "Movela."), http.StatusBadRequest, ""},
		{"action vacia", "nueva", lnCuerpo("", "claude", "", nil, "Algo."), http.StatusBadRequest, ""},
		// pedido obligatorio y no de relleno
		{"pedido <pedido>", "nueva", lnCuerpo("pedir-arquitecto", "claude", "", nil, "<pedido>"), http.StatusBadRequest, ""},
		{"pedido de puntos", "nueva", lnCuerpo("pedir-arquitecto", "claude", "", nil, " ... "), http.StatusBadRequest, ""},
		{"pedido vacio", "nueva", lnCuerpo("pedir-arquitecto", "claude", "", nil, ""), http.StatusBadRequest, ""},
		{"pedido <pedido> al writer", "en-writer", lnCuerpo("pedir-writer", "claude", "", nil, "<pedido>"), http.StatusBadRequest, ""},
		// provider que no es opcion de la accion (no instalado o inexistente)
		{"provider gemini sin instalar", "nueva", lnCuerpo("pedir-arquitecto", "gemini", "", nil, "Escribi el spec."), http.StatusBadRequest, ""},
		{"provider inexistente", "nueva", lnCuerpo("pedir-arquitecto", "inventado", "", nil, "Escribi el spec."), http.StatusBadRequest, ""},
		{"provider vacio", "nueva", lnCuerpo("pedir-arquitecto", "", "", nil, "Escribi el spec."), http.StatusBadRequest, ""},
		{"provider opencode sin instalar", "en-writer", lnCuerpo("pedir-writer", "opencode", "", nil, "Implementa."), http.StatusBadRequest, ""},
	}
	for _, k := range casos {
		msg := tbErrorJSON(t, "CA-338 ("+k.que+")", lnLaunch(t, s, k.slug, s.Token(), k.body), k.code)
		if k.msg != "" && msg != k.msg {
			t.Fatalf("CA-338: %s responde %d con\n  %q\nfue\n  %q", k.que, k.code, k.msg, msg)
		}
		lnEsperarLanzamientos(t, "CA-338", s)
		lnSinEfectos(t, "CA-338", k.que, antes, lnHuellas(t, root), false)
	}
	lnSinLlamadas(t, "CA-338", "un launch rechazado", f)
	for _, slug := range []string{"nueva", "sin-espacio"} {
		if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", slug)); err == nil {
			t.Fatalf("CA-338: un launch rechazado no crea la tarea %s", slug)
		}
	}
}

// CA-338: un slug con forma invalida es 400; un item inexistente o invalido
// es 404 con el mensaje de CardFor.
func TestCA338_SlugEItem(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	tbEscribir(t, root, ".hoom/items/malo.yaml", "titulo: Malo\ntipo: feature\nprioridad: media\n"+
		"creado_por: x\ncreado_en: 2026-09-22T15:04:05Z\ncolumna: hecho\n")
	s := lnServidor(t, root, f)
	antes := lnHuellas(t, root)
	cuerpo := lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec.")

	for _, mal := range []string{"Mal_Slug", "-guion", "a_b"} {
		tbErrorJSON(t, "CA-338 (slug "+mal+")", lnLaunch(t, s, mal, s.Token(), cuerpo), http.StatusBadRequest)
	}
	for _, slug := range []string{"no-existe", "malo"} {
		msg := tbErrorJSON(t, "CA-338 ("+slug+")", lnLaunch(t, s, slug, s.Token(), cuerpo), http.StatusNotFound)
		_, err := boardcmd.CardFor(root, lnBase, tbBlockOn, slug, time.Now().UTC())
		if err == nil || msg != err.Error() {
			t.Fatalf("CA-338: el 404 de %s trae el mensaje de CardFor:\napi: %q\ncli: %v", slug, msg, err)
		}
	}
	lnEsperarLanzamientos(t, "CA-338", s)
	lnSinEfectos(t, "CA-338", "un slug o item que no sirve", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-338", "un slug o item que no sirve", f)
}

// CA-339: con el presupuesto agotado (6 de 5 USD) el launch es 409 con la
// frase de CA-327, con cualquier provider.
func TestCA339_PresupuestoAgotadoNoLanza(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	f.cli(t, "codex", false)
	root := tbProyecto(t)
	wt := lnCartaWriter(t, "CA-339", root, "gastada", "presupuesto_usd: 5\n")
	lnGasto(t, "CA-339", root, "gastada", wt, 6)
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	for _, p := range []string{"claude", "codex"} {
		msg := tbErrorJSON(t, "CA-339 ("+p+")", lnLaunch(t, s, "gastada", s.Token(),
			lnCuerpo("pedir-writer", p, "", nil, "Implementa lo que falta.")), http.StatusConflict)
		if msg != "se agoto el presupuesto de la tarjeta: se gastaron 6 de 5 USD" {
			t.Fatalf("CA-339: con el presupuesto agotado, 409 con la frase de CA-327 (%s): %q", p, msg)
		}
	}
	lnEsperarLanzamientos(t, "CA-339", s)
	lnSinEfectos(t, "CA-339", "un launch sin presupuesto", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-339", "un launch sin presupuesto", f)
}

// CA-339: con Claude y presupuesto (5 USD, 1 gastado), un tope bajo el minimo
// (0.5) o mayor que lo que queda es 409, uno que no es mayor que 0 es 400, y
// null lanza con lo que queda.
func TestCA339_ClaudeConPresupuesto(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	wt := lnCartaWriter(t, "CA-339", root, "con-tope", "presupuesto_usd: 5\n")
	lnGasto(t, "CA-339", root, "con-tope", wt, 1)
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	casos := []struct {
		budget any
		code   int
		que    string
	}{
		{0.2, http.StatusConflict, "por debajo del minimo de claude"},
		{0.49, http.StatusConflict, "apenas por debajo del minimo de claude"},
		{4.5, http.StatusConflict, "mas de lo que queda"},
		{0, http.StatusBadRequest, "un tope de 0"},
		{-1, http.StatusBadRequest, "un tope negativo"},
	}
	for _, k := range casos {
		tbErrorJSON(t, "CA-339 ("+k.que+")", lnLaunch(t, s, "con-tope", s.Token(),
			lnCuerpo("pedir-writer", "claude", "", k.budget, "Implementa lo que falta.")), k.code)
		lnEsperarLanzamientos(t, "CA-339", s)
		lnSinEfectos(t, "CA-339", k.que, antes, lnHuellas(t, root), false)
	}
	lnSinLlamadas(t, "CA-339", "un tope rechazado", f)

	lnAceptado(t, "CA-339", lnLaunch(t, s, "con-tope", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa lo que falta.")))
	lnEsperarLanzamientos(t, "CA-339", s)
	ll := f.llamadas(t, "claude")
	if len(ll) != 1 {
		t.Fatalf("CA-339: un launch, un run de claude: %d", len(ll))
	}
	if v, ok := lnSigue(ll[0], "--max-budget-usd"); !ok || v != "4" {
		t.Fatalf("CA-339: null lanza con lo que queda (--max-budget-usd 4): %q", ll[0])
	}
}

// CA-339: con 4.8 de 5 USD gastados Claude no es opcion (quedan 0.2 y
// necesita 0.5) y Codex, que no acepta tope, lanza.
func TestCA339_QuedaMenosQueElMinimoDeClaude(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	f.cli(t, "codex", false)
	root := tbProyecto(t)
	wt := lnCartaWriter(t, "CA-339", root, "casi-gastada", "presupuesto_usd: 5\n")
	lnGasto(t, "CA-339", root, "casi-gastada", wt, 4.8)
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	msg := tbErrorJSON(t, "CA-339", lnLaunch(t, s, "casi-gastada", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa lo que falta.")), http.StatusConflict)
	if msg != "quedan 0.2 USD y claude necesita al menos 0.5 USD por run" {
		t.Fatalf("CA-339: claude no es opcion: 409 con su why: %q", msg)
	}
	lnEsperarLanzamientos(t, "CA-339", s)
	lnSinEfectos(t, "CA-339", "claude bajo su minimo", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-339", "claude bajo su minimo", f)

	r := lnAceptado(t, "CA-339", lnLaunch(t, s, "casi-gastada", s.Token(),
		lnCuerpo("pedir-writer", "codex", "", nil, "Implementa lo que falta.")))
	lnEsperarLanzamientos(t, "CA-339", s)
	if reg := lnRegistro(t, "CA-339", root, r.EnvelopeID); reg.Provider != "codex" || r.Provider != "codex" {
		t.Fatalf("CA-339: codex lanza aunque queden 0.2 USD: %+v", reg)
	}
	if len(f.llamadas(t, "codex")) != 1 || len(f.llamadas(t, "claude")) != 0 {
		t.Fatalf("CA-339: corrio codex y no claude: codex=%d claude=%d", len(f.llamadas(t, "codex")), len(f.llamadas(t, "claude")))
	}
}

// CA-339: sin presupuesto en el item, null lanza sin tope; Codex con un tope
// numerico es 400 y no corre nada; Claude bajo su minimo sigue siendo 409.
func TestCA339_SinPresupuestoYCodex(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	f.cli(t, "codex", false)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-339", root, "sin-tope", "")
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	msg := tbErrorJSON(t, "CA-339", lnLaunch(t, s, "sin-tope", s.Token(),
		lnCuerpo("pedir-writer", "codex", "", 2, "Implementa lo que falta.")), http.StatusBadRequest)
	if msg != "codex no acepta tope de presupuesto: deja el presupuesto vacio" {
		t.Fatalf("CA-339: codex con un tope numerico es 400 con el mensaje del contrato: %q", msg)
	}
	tbErrorJSON(t, "CA-339 (claude 0.2 sin presupuesto)", lnLaunch(t, s, "sin-tope", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", 0.2, "Implementa lo que falta.")), http.StatusConflict)
	lnEsperarLanzamientos(t, "CA-339", s)
	lnSinEfectos(t, "CA-339", "un tope rechazado", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-339", "un tope rechazado", f)

	lnAceptado(t, "CA-339", lnLaunch(t, s, "sin-tope", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa lo que falta.")))
	lnEsperarLanzamientos(t, "CA-339", s)
	ll := f.llamadas(t, "claude")
	if len(ll) != 1 || lnTiene(ll[0], "--max-budget-usd") {
		t.Fatalf("CA-339: sin presupuesto en el item, null lanza sin tope: %q", ll)
	}

	r := lnAceptado(t, "CA-339", lnLaunch(t, s, "sin-tope", s.Token(),
		lnCuerpo("pedir-writer", "codex", "", nil, "Implementa lo que falta.")))
	lnEsperarLanzamientos(t, "CA-339", s)
	if reg := lnRegistro(t, "CA-339", root, r.EnvelopeID); reg.Provider != "codex" || len(f.llamadas(t, "codex")) != 1 {
		t.Fatalf("CA-339: codex con null lanza sin tope: %+v", reg)
	}
}

// CA-340: con la tarjeta en curso (un sobre vivo de otro proceso) el launch
// es 409 con el why de la accion, y no lanza nada.
func TestCA340_TarjetaEnCursoEs409(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	wt := lnCartaWriter(t, "CA-340", root, "ocupada", "")
	envelope.Write(root, envelope.Record{ID: "20260923T100000_vivo01", Role: "writer", Provider: "claude",
		Task: "ocupada", Dir: wt, Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning,
		ExitCode: -1, StartedAt: time.Now().UTC(), PID: os.Getpid()})
	if c := lnCarta(t, root, "ocupada"); c.Running == nil {
		t.Fatal("CA-340: fixture: la tarjeta esta en curso")
	}
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	msg := tbErrorJSON(t, "CA-340", lnLaunch(t, s, "ocupada", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec.")), http.StatusConflict)
	if msg != "espera a que termine el writer que esta trabajando" {
		t.Fatalf("CA-340: con la tarjeta en curso, 409 con el why de la accion: %q", msg)
	}
	lnEsperarLanzamientos(t, "CA-340", s)
	lnSinEfectos(t, "CA-340", "un launch sobre una tarjeta en curso", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-340", "un launch sobre una tarjeta en curso", f)
}

// CA-340: un run activo de otro proceso (de otra tarea) en el arbol de la
// tarjeta: 409 que nombra el run.
func TestCA340_RunDeOtroProcesoEnElArbol(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	wt := lnCartaWriter(t, "CA-340", root, "compartida", "")
	runID := "20260923T100500_ajeno1"
	lnSidecar(t, root, runcmd.Meta{ID: runID, Provider: "codex", Role: "writer", Task: "otra-tarea", Dir: wt,
		CreatedAt: time.Now().UTC(), Status: runcmd.StatusRunning, ExitCode: -1, PID: os.Getpid()})
	if c := lnCarta(t, root, "compartida"); c.Running != nil {
		t.Fatalf("CA-340: fixture: el run de otra tarea no pone en curso a esta tarjeta: %+v", c.Running)
	}
	s := lnServidor(t, root, f)

	antes := lnHuellas(t, root)
	msg := tbErrorJSON(t, "CA-340", lnLaunch(t, s, "compartida", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec.")), http.StatusConflict)
	if !strings.Contains(msg, runID) {
		t.Fatalf("CA-340: el 409 nombra el run activo del arbol (%s): %q", runID, msg)
	}
	lnEsperarLanzamientos(t, "CA-340", s)
	lnSinEfectos(t, "CA-340", "un launch sobre un arbol ocupado", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-340", "un launch sobre un arbol ocupado", f)
}

// CA-340: dos pestanas piden al writer a la vez: un 202 y un 409, y un solo
// sobre.
func TestCA340_DosLanzamientosALaVez(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", true)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-340", root, "disputada", "")
	s := lnServidor(t, root, f)

	raw, err := json.Marshal(lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec."))
	if err != nil {
		t.Fatal(err)
	}
	h := s.Handler()
	recs := make([]*httptest.ResponseRecorder, 2)
	var wg sync.WaitGroup
	for i := range recs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/board/disputada/launch", bytes.NewReader(raw))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set(TokenHeader, s.Token())
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			recs[i] = rec
		}(i)
	}
	listo := make(chan struct{})
	go func() { wg.Wait(); close(listo) }()
	select {
	case <-listo:
	case <-time.After(20 * time.Second):
		f.soltar()
		<-listo
		t.Fatal("CA-340: los dos launches responden sin esperar a que el sobre termine")
	}
	codes := []int{recs[0].Code, recs[1].Code}
	sort.Ints(codes)
	if codes[0] != http.StatusAccepted || codes[1] != http.StatusConflict {
		t.Fatalf("CA-340: dos launches simultaneos dan un 202 y un 409: %v\n%s\n%s", codes, recs[0].Body, recs[1].Body)
	}
	for _, rec := range recs {
		if rec.Code == http.StatusConflict {
			tbErrorJSON(t, "CA-340", rec, http.StatusConflict)
		}
	}
	f.soltar()
	lnEsperarLanzamientos(t, "CA-340", s)
	if regs := lnRegistros(t, root); len(regs) != 1 {
		t.Fatalf("CA-340: un solo sobre: %v", regs)
	}
	if n := len(f.llamadas(t, "claude")); n != 1 {
		t.Fatalf("CA-340: un solo run del writer: %d", n)
	}
}

// CA-341: reanudar lanza un sobre NUEVO con el rol y el provider del
// interrumpido y --resume de la sesion de su sidecar. El registro del sobre
// muerto queda byte a byte igual y la tarjeta deja de estar interrumpida.
func TestCA341_ReanudarRetomaLaSesion(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	wt, viejo, crudo := lnCartaInterrumpida(t, "CA-341", root, "cortada")
	s := lnServidor(t, root, f)

	pedido := "Segui con lo que quedo a medias y termina el spec."
	r := lnAceptado(t, "CA-341", lnLaunch(t, s, "cortada", s.Token(), lnCuerpo("reanudar", "claude", "", nil, pedido)))
	if r.EnvelopeID == viejo || r.Role != "writer" || r.Provider != "claude" {
		t.Fatalf("CA-341: reanudar abre un sobre nuevo con el rol y el provider del interrumpido: %+v", r)
	}
	lnEsperarLanzamientos(t, "CA-341", s)

	ll := f.llamadas(t, "claude")
	if len(ll) != 1 {
		t.Fatalf("CA-341: reanudar corre un run: %d", len(ll))
	}
	if v, ok := lnSigue(ll[0], "--resume"); !ok || v != "sesion-vieja-123" {
		t.Fatalf("CA-341: reanudar pasa ResumeID = el resume_id de la accion (--resume sesion-vieja-123): %q", ll[0])
	}
	if lnUltimo(ll[0]) != pedido {
		t.Fatalf("CA-341: con el pedido del dialogo: %q", lnUltimo(ll[0]))
	}
	reg := lnRegistro(t, "CA-341", root, r.EnvelopeID)
	if reg.Role != "writer" || reg.Provider != "claude" || reg.Task != "cortada" || reg.Spec != filepath.Join(wt, ".hoom", "specs", "cortada.md") {
		t.Fatalf("CA-341: el sobre nuevo es el del writer de la tarjeta, con su spec: %+v", reg)
	}
	lnMismoRegistro(t, root, viejo, crudo)
	if c := lnCarta(t, root, "cortada"); c.Interrupted != nil {
		t.Fatalf("CA-341: la tarjeta deja de estar interrumpida: %+v", c.Interrupted)
	}
}

// CA-341: relanzar abre un sobre nuevo sin ResumeID, con las mismas
// garantias sobre el registro muerto.
func TestCA341_RelanzarEmpiezaDeCero(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	_, viejo, crudo := lnCartaInterrumpida(t, "CA-341", root, "cortada")
	s := lnServidor(t, root, f)

	pedido := "Implementa el spec desde el arbol como quedo."
	r := lnAceptado(t, "CA-341", lnLaunch(t, s, "cortada", s.Token(), lnCuerpo("relanzar", "claude", "", nil, pedido)))
	if r.EnvelopeID == viejo || r.Role != "writer" || r.Provider != "claude" {
		t.Fatalf("CA-341: relanzar abre un sobre nuevo con el rol del interrumpido: %+v", r)
	}
	lnEsperarLanzamientos(t, "CA-341", s)

	ll := f.llamadas(t, "claude")
	if len(ll) != 1 {
		t.Fatalf("CA-341: relanzar corre un run: %d", len(ll))
	}
	if lnTiene(ll[0], "--resume") || lnTiene(ll[0], "--continue") {
		t.Fatalf("CA-341: relanzar no retoma ninguna sesion: %q", ll[0])
	}
	if lnUltimo(ll[0]) != pedido {
		t.Fatalf("CA-341: con el pedido del dialogo: %q", lnUltimo(ll[0]))
	}
	if reg := lnRegistro(t, "CA-341", root, r.EnvelopeID); reg.Role != "writer" || reg.Task != "cortada" {
		t.Fatalf("CA-341: el sobre nuevo es el del writer de la tarjeta: %+v", reg)
	}
	lnMismoRegistro(t, root, viejo, crudo)
	if c := lnCarta(t, root, "cortada"); c.Interrupted != nil {
		t.Fatalf("CA-341: la tarjeta deja de estar interrumpida: %+v", c.Interrupted)
	}
}

// lnMismoRegistro: hoom nunca cierra ni reescribe el registro de un sobre
// que no vio cerrar.
func lnMismoRegistro(t *testing.T, root, id string, crudo []byte) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, id+".json"))
	if err != nil {
		t.Fatalf("CA-341: el registro del sobre interrumpido sigue en disco: %v", err)
	}
	if !bytes.Equal(raw, crudo) {
		t.Fatalf("CA-341: el registro del sobre interrumpido queda byte a byte igual:\nantes:   %s\ndespues: %s", crudo, raw)
	}
}
