// Package gates executes the deterministic verification layer. No LLM is
// ever involved here: trust lives in tools with exit codes, not in agent
// claims. Missing tools fail closed (error => red under strict policy);
// declared absence (no command) is reported loudly but is not red.
package gates

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/procesos"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Options control a verify run.
type Options struct {
	Full     bool            // ignore diff scoping, run full commands
	Only     map[string]bool // if non-empty, run only these gates (others => skipped)
	Timeout  time.Duration   // per-gate timeout (0 = default)
	TailSize int             // bytes of output tail kept in the verdict
	// Emit narrates the run as live events (nil = silent). Best-effort by
	// contract: emitting never alters gate execution, verdict or exit code.
	Emit func(live.Event)
	// Encierro (contencion-del-writer) is the plan each gate runs under
	// (encierro.Envolver, encierro.Arrancar with the gate's gracia): nil =
	// as today. Only verifycmd.Run with Options.Sobre sets it.
	Encierro *encierro.Plan
	// SinEncierro are the gates that run as today even with Encierro
	// (encierro.gates_sin_encierro of hoom.yaml).
	SinEncierro map[string]bool
	// Local (verify-en-el-entorno) is the .hoom/local.yaml of the run, as
	// entorno.Leer read it. nil, or one without Ejecutar, = as today: the
	// same commands and states, and no GateResult carries Entorno. With
	// Ejecutar, each gate with the template (Local.ConPrefijo) runs
	// `sh -c <Local.Linea(its command, m.Dir)>` instead of
	// `sh -c <its command>`, with the same directory (m.Dir), timeout and
	// gracia, and its GateResult.Entorno is verdict.EntornoPrefijo; the
	// gates of Local.EnElHost run as today, with verdict.EntornoHost. Either
	// way GateResult.Command is the gate's own command (what {cmd} became),
	// not the line with the template, which the verdict keeps once in
	// verdict.Verdict.Entorno.
	Local *entorno.Ajuste
	// ExigirEncierro (verify-en-el-entorno) is encierro.exigir of the
	// project's hoom.yaml (manifest.ExigirCompleto, ExigirParcial or
	// ExigirNo) when the run is the verify of a Sobre in a project with the
	// section `encierro`, whether or not this system can enclose (so also
	// with Encierro nil); "" otherwise. Only verifycmd.Run with
	// Options.Sobre sets it.
	//
	// With it, a gate with Local's template runs unenclosed, as one of
	// SinEncierro, with the note NotaSinEncierroConPrefijo: the container is
	// its limit. With ExigirCompleto or ExigirParcial that gate does not run
	// at all: status error, exit_code -1, its Command and Entorno as if it
	// had run, and the note fmt.Sprintf(NotaExigirConPrefijo, ExigirEncierro).
	// A gate with the template that also gets a note of its own
	// (NotaNoEncontrado, NotaNoEjecutable, the timeout) has Notes
	// "<its note>; " + NotaSinEncierroConPrefijo. The gates of
	// Local.EnElHost are not affected: they run as today, enclosed with
	// Encierro.
	ExigirEncierro string
}

// The notes of a gate that could not run, or that ran unenclosed because of
// .hoom/local.yaml (verify-en-el-entorno), exactly.
const (
	// NotaNoEncontrado opens the note of a gate whose command exited 127
	// (sh: command not found): this text, then ": " and the last line of its
	// output that is not blank, trimmed; this text alone when the output has
	// no such line.
	NotaNoEncontrado = "no pudo correr: comando no encontrado (exit 127)"
	// NotaNoEjecutable opens the note of a gate whose command exited 126
	// (found but not executable), as NotaNoEncontrado.
	NotaNoEjecutable = "no pudo correr: el comando no se puede ejecutar (exit 126)"
	// NotaSinEncierroConPrefijo is the note of a gate with the template of
	// .hoom/local.yaml in the verify of a Sobre with the section encierro
	// (Options.ExigirEncierro): it ran unenclosed.
	NotaSinEncierroConPrefijo = "sin encierro: corre con el prefijo de .hoom/local.yaml"
	// NotaExigirConPrefijo is the note of a gate with the template that did
	// not run because encierro.exigir is completo or parcial; %s is that
	// value.
	NotaExigirConPrefijo = "el prefijo de .hoom/local.yaml no corre encerrado y encierro.exigir es %s"
)

func (o Options) emit(ev live.Event) {
	if o.Emit != nil {
		o.Emit(ev)
	}
}

const (
	defaultTimeout = 30 * time.Minute
	defaultTail    = 4000
)

// Run executes every gate in canonical order and returns results.
//
// How a gate's exit becomes its status (verify-en-el-entorno): exit 0 is
// pass. Exit 127 (sh: command not found) and exit 126 (found but not
// executable) are error, with that exit_code and the note NotaNoEncontrado
// or NotaNoEjecutable followed by ": " and the last line of the gate's output
// that is not blank, which is where sh and env name the command (the
// constant alone when there is no such line). Any other non-zero exit is
// fail with its code, a gate killed by a signal without a timeout included,
// and the timeout is error with exit_code -1 and its note, as before. It
// holds with Options.Local's template and without it: a ddev missing from
// the host gives 127 just like a missing php, and a 127 from inside the
// container that the template passes on is the gate's 127.
func Run(m *manifest.Manifest, git gitx.Info, opt Options) []verdict.GateResult {
	if opt.Timeout == 0 {
		opt.Timeout = defaultTimeout
	}
	if opt.TailSize == 0 {
		opt.TailSize = defaultTail
	}
	var results []verdict.GateResult
	for _, name := range m.SortedGateNames() {
		results = append(results, runOne(name, m.Gates[name], m, git, opt))
	}
	return results
}

func runOne(name string, g manifest.Gate, m *manifest.Manifest, git gitx.Info, opt Options) verdict.GateResult {
	res := verdict.GateResult{
		Name:     name,
		Required: g.IsRequired(),
		Partial:  g.Partial,
		Scope:    "none",
	}
	prefijo := opt.Local.ConPrefijo(name)
	if opt.Local != nil && opt.Local.Ejecutar != "" {
		// where it runs, or would have run (aclaracion 2)
		res.Entorno = verdict.EntornoHost
		if prefijo {
			res.Entorno = verdict.EntornoPrefijo
		}
	}
	if len(opt.Only) > 0 && !opt.Only[name] {
		res.Status = verdict.StatusSkipped
		opt.emit(live.Event{Kind: live.KindGateStart, Gate: name, Scope: res.Scope})
		opt.emit(live.Event{Kind: live.KindGateEnd, Gate: name, Status: res.Status})
		return res
	}
	cmdStr, scope := selectCommand(g, git, opt)
	if strings.TrimSpace(cmdStr) == "" {
		res.Status = verdict.StatusAbsent // declared degradation
		res.Notes = g.Notes
		opt.emit(live.Event{Kind: live.KindGateStart, Gate: name, Scope: res.Scope})
		opt.emit(live.Event{Kind: live.KindGateEnd, Gate: name, Status: res.Status})
		return res
	}
	res.Command = cmdStr
	res.Scope = scope
	opt.emit(live.Event{Kind: live.KindGateStart, Gate: name, Scope: scope})
	// the container is the limit of a gate with the template: in the verify
	// of a Sobre it runs unenclosed, or not at all when exigir asks for more
	sinEncierro := prefijo && opt.ExigirEncierro != ""
	if sinEncierro && (opt.ExigirEncierro == manifest.ExigirCompleto || opt.ExigirEncierro == manifest.ExigirParcial) {
		res.Status = verdict.StatusError
		res.ExitCode = -1
		res.Notes = fmt.Sprintf(NotaExigirConPrefijo, opt.ExigirEncierro)
		opt.emit(live.Event{Kind: live.KindGateEnd, Gate: name, Status: res.Status, ExitCode: res.ExitCode})
		return res
	}
	linea := cmdStr
	if prefijo {
		linea = opt.Local.Linea(cmdStr, m.Dir)
	}

	ctx, cancel := context.WithTimeout(context.Background(), opt.Timeout)
	defer cancel()
	start := time.Now()
	// correr keeps the last TailSize+margenCola bytes; tail then trims the
	// blanks at both ends and keeps the last TailSize of what is left
	plan := opt.Encierro
	if opt.SinEncierro[name] || prefijo {
		plan = nil
	}
	out, cortado, err := correr(ctx, m.Dir, linea, opt.TailSize+margenCola, plan)
	res.DurationMS = time.Since(start).Milliseconds()
	res.OutputTail = tail(string(out), opt.TailSize)
	if cortado && !strings.HasPrefix(res.OutputTail, marcaCorte) {
		res.OutputTail = marcaCorte + res.OutputTail
	}

	switch {
	case ctx.Err() == context.DeadlineExceeded:
		res.Status = verdict.StatusError
		res.ExitCode = -1
		res.Notes = fmt.Sprintf("timeout tras %s", opt.Timeout)
	case err == nil:
		res.Status = verdict.StatusPass
	default:
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
			res.Status = verdict.StatusFail
			switch res.ExitCode {
			case 127:
				res.Status = verdict.StatusError
				res.Notes = notaNoPudo(NotaNoEncontrado, res.OutputTail)
			case 126:
				res.Status = verdict.StatusError
				res.Notes = notaNoPudo(NotaNoEjecutable, res.OutputTail)
			}
		} else {
			res.ExitCode = -1
			res.Status = verdict.StatusError // could not even start — fail closed
			res.Notes = err.Error()
		}
	}
	if sinEncierro {
		res.Notes = juntarNotas(res.Notes, NotaSinEncierroConPrefijo)
	}
	opt.emit(live.Event{Kind: live.KindGateEnd, Gate: name, Status: res.Status,
		DurationMS: res.DurationMS, ExitCode: res.ExitCode})
	return res
}

// notaNoPudo is the note of a gate that exited 127 or 126: constante, then
// ": " and the last line of its output that is not blank, which is where sh
// and env name the command; constante alone when there is none.
func notaNoPudo(constante, salida string) string {
	lineas := strings.Split(salida, "\n")
	for i := len(lineas) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lineas[i]); l != "" {
			return constante + ": " + l
		}
	}
	return constante
}

// juntarNotas is a gate's own note followed by another one (aclaracion 8).
func juntarNotas(propia, otra string) string {
	if propia == "" {
		return otra
	}
	return propia + "; " + otra
}

// graciaGate is how long a gate's group has, once its deadline passed,
// between SIGTERM and SIGKILL: enough for a docker client to stop its
// container and for a tool to clean up.
const graciaGate = 10 * time.Second

// correr runs one gate's command in dir and returns the end of its combined
// output (at most limite bytes; cortado says some was dropped). The gate
// runs in a process group of its own, registered so that a Ctrl-C to hoom
// ends it too (procesos.ArrancarConGracia): when ctx ends, the WHOLE group
// gets SIGTERM and, after graciaGate, SIGKILL — not only the sh, which used
// to leave git, go test or semgrep (or the container a docker client
// started) running after verify had marked the gate (findings 862449,
// af0383).
//
// The output goes through a pipe of correr's own, whose write end the gate
// gets as a file: Wait returns when the sh ends, even if something it left
// behind still holds the output, so a gate that exits 0 is never kept
// waiting nor turned into an error by an orphan (finding 5a05b4). Nothing
// goes to disk (findings 6599fe, 47764f, fb2f6d, e55193) and only the end of
// the output is kept in memory (finding 5a3741). Then whatever the gate left
// in its group dies with it (procesos.Liberar) — after the rest of its
// gracia when the deadline passed (finding 01d565) — and the reading ends
// with it; something that left the group and still holds the output gets
// finDeLectura more, and no more.
//
// With the plan of an encierro (contencion-del-writer; nil = as today) the
// gate's sh runs enclosed (encierro.Envolver), with the same gracia
// (encierro.Arrancar), and its end kills whatever it left inside
// (encierro.Liberar). When encierro.Comprobar kills it on start (Windows
// without the job, Plan.Exigir above ninguno), err is its encierro.SinJob:
// the gate ends in error with that text.
func correr(ctx context.Context, dir, cmdStr string, limite int, plan *encierro.Plan) (out []byte, cortado bool, err error) {
	pr, pw, err := os.Pipe()
	if err != nil {
		return nil, false, err
	}
	defer pr.Close()
	cola := &colaAcotada{limite: limite}
	leido := make(chan struct{})
	go func() {
		io.Copy(cola, pr)
		close(leido)
	}()
	cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- el gate es el comando que el proyecto declara en hoom.yaml: ejecutarlo es el trabajo de verify
	cmd.Dir = dir
	cmd.Stdout = pw
	cmd.Stderr = pw
	cmd.WaitDelay = graciaGate + 5*time.Second
	if plan != nil {
		if err := encierro.Envolver(cmd, *plan); err != nil {
			pw.Close()
			return nil, false, err
		}
		err = encierro.Arrancar(cmd, graciaGate)
	} else {
		err = procesos.ArrancarConGracia(cmd, graciaGate)
	}
	pw.Close() // the gate keeps its copy: EOF comes when the gate's are closed
	if err != nil {
		return nil, false, err // the deferred Close ends the reading
	}
	// on Windows without the job and exigir above ninguno (ronda 1, decision
	// 22) its tree is already dead: the gate ends in error with why
	var sinJob error
	if plan != nil {
		_, sinJob = encierro.Comprobar(cmd, *plan)
	}
	err = cmd.Wait()
	if plan != nil {
		encierro.Liberar(cmd) // the rest of its gracia, then the whole encierro
	} else {
		if ctx.Err() != nil {
			procesos.EsperarGrupo(cmd.Process.Pid, time.Now().Add(graciaGate))
		}
		procesos.Liberar(cmd.Process.Pid)
	}
	select {
	case <-leido:
	case <-time.After(finDeLectura):
		// someone outside the group holds the output: stop reading. On
		// Windows a pending read on a pipe may outlive its Close, so the
		// wait is bounded too and the tail is taken as it is
		pr.Close()
		select {
		case <-leido:
		case <-time.After(finDeLectura):
		}
	}
	out, cortado = cola.bytes()
	if sinJob != nil {
		err = sinJob
	}
	return out, cortado, err
}

// finDeLectura is how long the output is still read once the gate and its
// group are gone: what something that left the group writes is not the
// gate's.
const finDeLectura = 2 * time.Second

// colaAcotada keeps the end of what is written to it: bytes() gives the
// last limite bytes of the output without its trailing blanks, and whether
// more came before them. To copy less, it lets its buffer grow up to
// 2*limite before dropping the oldest half (finding d3331e).
//
// A run of blanks (blancosASCII) is held aside until something that is not
// a blank follows it, and only then added (its last limite bytes are enough
// for the tail): blanks in the middle stay exactly where they were (finding
// 77dc9a), and the run at the end, which tail trims anyway, never pushes the
// output before it out of the tail — a gate that prints its diagnostic and
// then 64 KiB of blank lines keeps the diagnostic (findings c61685, 4a9763).
type colaAcotada struct {
	mu      sync.Mutex
	limite  int
	buf     []byte
	blancos []byte // the run of blanks after buf, not added yet: its end
	enRacha int    // how long that run really is
	cortado bool
}

// blancosASCII are the blanks colaAcotada holds aside and tail trims: the
// same set on both sides (finding 5fc8ff).
const blancosASCII = " \t\n\r\v\f"

func (c *colaAcotada) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, b := range p {
		if strings.IndexByte(blancosASCII, b) >= 0 {
			c.blancos = append(c.blancos, b)
			c.enRacha++
			if c.limite > 0 && len(c.blancos) > 2*c.limite {
				c.blancos = append(c.blancos[:0], c.blancos[len(c.blancos)-c.limite:]...)
			}
			continue
		}
		// a run that lost its start is a cut only now that output follows
		// it: a run at the very end is trimmed whole anyway
		if c.enRacha > len(c.blancos) {
			c.cortado = true
		}
		c.buf = append(c.buf, c.blancos...)
		c.blancos, c.enRacha = c.blancos[:0], 0
		c.buf = append(c.buf, b)
	}
	if c.limite > 0 && len(c.buf) > 2*c.limite {
		c.buf = append(c.buf[:0], c.buf[len(c.buf)-c.limite:]...)
		c.cortado = true
	}
	return len(p), nil
}

func (c *colaAcotada) bytes() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.buf // the trailing blanks (c.blancos) are left out: tail trims them
	cortado := c.cortado
	if c.limite > 0 && len(b) > c.limite {
		b, cortado = b[len(b)-c.limite:], true
	}
	return append([]byte(nil), b...), cortado
}

// selectCommand picks diff_cmd when a diff scope exists and --full was not
// requested, then substitutes template variables:
//
//	{base}     base branch
//	{files}    space-separated changed files
//	{packages} ./dir style package list derived from changed .go files
func selectCommand(g manifest.Gate, git gitx.Info, opt Options) (string, string) {
	// A gate whose cmd resolves empty is ABSENT by declaration: diff_cmd is
	// only an optimization of cmd, never a gate on its own, so a diff_cmd
	// inherited from the profile must not resurrect a gate the project
	// disabled with `cmd: ""`.
	if strings.TrimSpace(g.CmdStr()) == "" {
		return "", "none"
	}
	cmdStr, scope := g.CmdStr(), "full"
	if !opt.Full && g.DiffCmdStr() != "" && git.IsRepo && len(git.ChangedFiles) > 0 {
		cmdStr, scope = g.DiffCmdStr(), "diff"
	}
	pkgs := gitx.GoPackages(git.ChangedFiles)
	if scope == "diff" {
		// A diff command whose variables expand to nothing falls back to the
		// full command rather than executing a broken line.
		if strings.Contains(cmdStr, "{packages}") && len(pkgs) == 0 {
			cmdStr, scope = g.CmdStr(), "full"
		}
		if strings.Contains(cmdStr, "{files}") && len(git.ChangedFiles) == 0 {
			cmdStr, scope = g.CmdStr(), "full"
		}
	}
	cmdStr = strings.ReplaceAll(cmdStr, "{base}", git.Base)
	cmdStr = strings.ReplaceAll(cmdStr, "{files}", strings.Join(git.ChangedFiles, " "))
	cmdStr = strings.ReplaceAll(cmdStr, "{packages}", strings.Join(pkgs, " "))
	return cmdStr, scope
}

// margenCola is how much more than TailSize correr keeps. tail trims the
// blanks at both ends before taking its last TailSize bytes: keeping only
// TailSize would leave the verdict's tail shorter than TailSize whenever the
// kept bytes start with blanks. The margin makes sure that, after the trim,
// there are still TailSize bytes to take (finding 3d3af4).
const margenCola = 64 << 10

// marcaCorte opens a tail that is not the whole output.
const marcaCorte = "...[truncado]...\n"

func tail(s string, n int) string {
	s = strings.Trim(s, blancosASCII)
	if len(s) <= n {
		return s
	}
	return marcaCorte + s[len(s)-n:]
}
