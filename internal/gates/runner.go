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
}

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

	ctx, cancel := context.WithTimeout(context.Background(), opt.Timeout)
	defer cancel()
	start := time.Now()
	// more than the tail is read so that trimming the trailing blank lines
	// still leaves TailSize bytes
	out, cortado, err := correr(ctx, m.Dir, cmdStr, opt.TailSize+margenCola)
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
		} else {
			res.ExitCode = -1
			res.Status = verdict.StatusError // could not even start — fail closed
			res.Notes = err.Error()
		}
	}
	opt.emit(live.Event{Kind: live.KindGateEnd, Gate: name, Status: res.Status,
		DurationMS: res.DurationMS, ExitCode: res.ExitCode})
	return res
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
func correr(ctx context.Context, dir, cmdStr string, limite int) (out []byte, cortado bool, err error) {
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
	err = procesos.ArrancarConGracia(cmd, graciaGate)
	pw.Close() // the gate keeps its copy: EOF comes when the gate's are closed
	if err != nil {
		return nil, false, err // the deferred Close ends the reading
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		procesos.EsperarGrupo(cmd.Process.Pid, time.Now().Add(graciaGate))
	}
	procesos.Liberar(cmd.Process.Pid)
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
	return out, cortado, err
}

// finDeLectura is how long the output is still read once the gate and its
// group are gone: what something that left the group writes is not the
// gate's.
const finDeLectura = 2 * time.Second

// colaAcotada keeps the last limite bytes written to it, and whether more
// came before them. Safe to read while it is still being written.
type colaAcotada struct {
	mu      sync.Mutex
	limite  int
	buf     []byte
	cortado bool
}

func (c *colaAcotada) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := len(p)
	c.buf = append(c.buf, p...)
	if c.limite > 0 && len(c.buf) > 2*c.limite {
		c.buf = append(c.buf[:0], c.buf[len(c.buf)-c.limite:]...)
		c.cortado = true
	}
	return n, nil
}

func (c *colaAcotada) bytes() ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b := c.buf
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

// margenCola is what correr keeps beyond TailSize, for the trailing blank
// lines tail trims.
const margenCola = 64 << 10

// marcaCorte opens a tail that is not the whole output.
const marcaCorte = "...[truncado]...\n"

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return marcaCorte + s[len(s)-n:]
}
