// Package runcmd drives headless AI-CLI runs: hoomAI's cockpit engine.
// A run is one chat session bound to one directory (project root or a task
// worktree), executed as a sequence of subprocess invocations of the user's
// own CLI. Narration events land in .hoom/runs/<id>.jsonl — LOCAL telemetry,
// excluded from Git and from the change-candidate fingerprint: what travels
// is evidence (verdicts), never narration. One active run per directory,
// mirroring the harness rule of one writer per task. runcmd is the ONLY
// executor: providers translate requests into commands and parse output;
// this package owns processes, timeouts, cancellation and logs.
package runcmd

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// Event re-exports the normalized narration event.
type Event = providers.Event

// Run status values.
const (
	StatusRunning  = "running"
	StatusDone     = "done"
	StatusError    = "error"
	StatusCanceled = "canceled"
)

// DefaultTimeout bounds one subprocess invocation, like a gate: a CLI stuck
// waiting for an interactive permission in headless mode must not hang the
// cockpit forever.
const DefaultTimeout = 30 * time.Minute

// Run is one cockpit session's public state.
type Run struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Task      string    `json:"task,omitempty"`
	Status    string    `json:"status"`
	ExitCode  int       `json:"exit_code"`
	CreatedAt time.Time `json:"created_at"`
	NumEvents int       `json:"num_events"`
	// ProviderSessionID is the last session id the provider reported in
	// its stream: the handle Input uses to resume EXACTLY this session.
	ProviderSessionID string `json:"provider_session_id,omitempty"`
}

// StartOptions is everything a run needs to start: provider and prompt,
// where to run, and the request fields the provider may or may not honor
// (see providers.Capabilities). The run remembers them so a later Input
// re-applies them.
type StartOptions struct {
	Provider     string
	Prompt       string
	Task         string // task slug: run inside its worktree; "" = project root
	Role         string // role slug this run embodies; "" = `hoom run`, no role
	ResumeID     string // provider session id to resume in this new run
	Model        string
	SystemPrompt string
	AllowTools   []string
	DenyTools    []string
	ReadOnly     bool // the role does not write: every provider imposes it its own way
	Exec         bool // ...but it does run commands
	MaxTurns     int
	BudgetUSD    float64
	Strict       bool // unsupported field = refuse to start instead of a warning
}

// request builds the provider request for these options.
func (o StartOptions) request(prompt, resumeID string, cont bool) providers.Request {
	return providers.Request{
		Prompt: prompt, ResumeID: resumeID, Continue: cont,
		Model: o.Model, SystemPrompt: o.SystemPrompt,
		AllowTools: o.AllowTools, DenyTools: o.DenyTools,
		ReadOnly: o.ReadOnly, Exec: o.Exec,
		MaxTurns: o.MaxTurns, BudgetUSD: o.BudgetUSD, Strict: o.Strict,
	}
}

// ResolveSystemPrompt turns the CLI flag value into prompt text: "@<ruta>"
// reads the file (typically a role contract); anything else is literal,
// a lone "@" included.
func ResolveSystemPrompt(arg string) (string, error) {
	if !strings.HasPrefix(arg, "@") || len(arg) == 1 {
		return arg, nil
	}
	path := arg[1:]
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("no se pudo leer el system prompt %q: %w", path, err)
	}
	return string(raw), nil
}

// ErrBusy signals that the target directory already has an active run.
type ErrBusy struct{ RunID string }

func (e ErrBusy) Error() string {
	return fmt.Sprintf("ya hay un run en curso sobre este arbol (%s); espera o cancelalo (un writer por tarea)", e.RunID)
}

type run struct {
	info     Run
	meta     Meta
	dir      string
	provider providers.Provider
	opts     StartOptions
	events   []Event
	cancel   context.CancelFunc
	canceled bool // pedido de cancelacion; el estado terminal lo fija settle
	log      *os.File
}

// Manager owns every run of one project. In-memory state, file-backed logs.
type Manager struct {
	root    string
	Timeout time.Duration

	mu    sync.Mutex
	runs  map[string]*run
	byDir map[string]string // dir -> id of the ACTIVE run (running only)
}

func runsDir(root string) string { return filepath.Join(root, ".hoom", "runs") }

// Meta is the DURABLE identity of a run: the jsonl NARRATES, the meta
// IDENTIFIES. Without it a finished run is anonymous the moment the process
// exits — and "which provider wrote this tree?" is exactly the question the
// cross review has to answer without guessing prose. Local telemetry like the
// narration: outside Git, outside the fingerprint, outside the envelope's
// delta.
type Meta struct {
	ID                string    `json:"id"`
	Provider          string    `json:"provider"`
	Role              string    `json:"role,omitempty"`
	Task              string    `json:"task,omitempty"`
	Dir               string    `json:"dir"`
	CreatedAt         time.Time `json:"created_at"`
	Status            string    `json:"status"` // running | done | error | canceled
	ExitCode          int       `json:"exit_code"`
	ProviderSessionID string    `json:"provider_session_id,omitempty"`
	EndedAt           time.Time `json:"ended_at,omitempty"`
}

func metaPath(root, id string) string { return filepath.Join(runsDir(root), id+".meta.json") }

// writeMeta records a run's identity. Best-effort by contract: telemetry that
// cannot be written never breaks the run it describes.
func writeMeta(root string, meta Meta) {
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}
	os.WriteFile(metaPath(root, meta.ID), append(raw, '\n'), 0o644)
}

// Metas lists the project's run metas, newest first. A file that is
// unreadable, of another shape or without an id is skipped: broken telemetry
// never breaks a command.
func Metas(root string) []Meta {
	entries, err := os.ReadDir(runsDir(root))
	if err != nil {
		return nil
	}
	var out []Meta
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(runsDir(root), e.Name()))
		if err != nil {
			continue
		}
		var meta Meta
		if json.Unmarshal(raw, &meta) != nil || strings.TrimSpace(meta.ID) == "" {
			continue
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// NewManager creates the manager and settles orphan logs: a previous serve
// that died mid-run leaves a log without a terminal event; those get an
// explicit error line instead of looking forever alive.
func NewManager(root string) *Manager {
	m := &Manager{root: root, Timeout: DefaultTimeout, runs: map[string]*run{}, byDir: map[string]string{}}
	m.markOrphans()
	return m
}

func (m *Manager) markOrphans() {
	entries, err := os.ReadDir(runsDir(m.root))
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(runsDir(m.root), e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		var last Event
		if len(lines) == 0 || json.Unmarshal([]byte(lines[len(lines)-1]), &last) != nil {
			continue
		}
		if last.Kind == "end" || last.Kind == "error" {
			continue
		}
		orphan := Event{TS: time.Now().UTC(), Kind: "error", Detail: "run huerfano: hoom serve termino mientras corria"}
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			enc, _ := json.Marshal(orphan)
			f.Write(append(enc, '\n'))
			f.Close()
		}
	}
}

// dirFor resolves where a run executes: the project root, or the task's
// isolated worktree.
func (m *Manager) dirFor(task string) (string, error) { return TaskDir(m.root, task) }

// TaskDir resolves where work for a task happens: its isolated worktree, or
// the project root when no task is given. The envelope needs the same answer
// the run uses, so both ask here.
func TaskDir(root, task string) (string, error) {
	if task == "" {
		return root, nil
	}
	wt := filepath.Join(root, ".hoom", "worktrees", task)
	if st, err := os.Stat(wt); err != nil || !st.IsDir() {
		return "", fmt.Errorf("la tarea %q no existe (mira 'hoom task list')", task)
	}
	return wt, nil
}

func newID() string {
	raw := make([]byte, 3)
	rand.Read(raw)
	return time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(raw)
}

// Start launches a new run. One active run per directory: a busy directory
// returns ErrBusy with the running id. The provider translates the request
// BEFORE anything is created, so a Strict refusal leaves no run and no log.
func (m *Manager) Start(opts StartOptions) (Run, error) {
	if strings.TrimSpace(opts.Prompt) == "" {
		return Run{}, fmt.Errorf("prompt vacio")
	}
	p, err := providers.Lookup(opts.Provider)
	if err != nil {
		return Run{}, err
	}
	if _, err := exec.LookPath(p.Bin()); err != nil {
		return Run{}, fmt.Errorf("el provider %q no esta instalado (no se encontro %q en PATH); instala su CLI primero", opts.Provider, p.Bin())
	}
	dir, err := m.dirFor(opts.Task)
	if err != nil {
		return Run{}, err
	}
	inv, err := p.Command(opts.request(opts.Prompt, opts.ResumeID, false))
	if err != nil {
		return Run{}, err
	}

	m.mu.Lock()
	if active, ok := m.byDir[dir]; ok {
		m.mu.Unlock()
		return Run{}, ErrBusy{RunID: active}
	}
	id := newID()
	if err := os.MkdirAll(runsDir(m.root), 0o755); err != nil {
		m.mu.Unlock()
		return Run{}, err
	}
	logFile, err := os.Create(filepath.Join(runsDir(m.root), id+".jsonl"))
	if err != nil {
		m.mu.Unlock()
		return Run{}, err
	}
	r := &run{
		info: Run{ID: id, Provider: p.Name(), Task: opts.Task, Status: StatusRunning, ExitCode: -1, CreatedAt: time.Now().UTC()},
		dir:  dir, provider: p, opts: opts, log: logFile,
	}
	r.meta = Meta{
		ID: id, Provider: p.Name(), Role: opts.Role, Task: opts.Task, Dir: dir,
		CreatedAt: r.info.CreatedAt, Status: StatusRunning, ExitCode: -1,
	}
	writeMeta(m.root, r.meta)
	m.runs[id] = r
	m.byDir[dir] = id
	m.mu.Unlock()

	m.append(r, Event{TS: time.Now().UTC(), Kind: "start",
		Detail: fmt.Sprintf("run %s: %s en %s", id, p.Name(), displayDir(opts.Task))})
	m.warnIgnored(r, inv.Ignored)
	// snapshot ANTES de lanzar la goroutine: execute escribe r.info en
	// paralelo y una copia sin lock seria una carrera de datos.
	m.mu.Lock()
	info := r.info
	m.mu.Unlock()
	go m.execute(r, inv)
	return info, nil
}

// Input continues a finished run's session with the next prompt (the "spec
// aprobado, adelante" of the tutorial). The run's original options travel
// again (a system prompt applies per launch) and the provider picks the
// strongest session mechanism it supports: resume by id when the session
// was captured, else continue-in-directory, else a fresh invocation — and
// the log says which. Declared degradation, never silent.
func (m *Manager) Input(id, prompt string) (Run, error) {
	if strings.TrimSpace(prompt) == "" {
		return Run{}, fmt.Errorf("prompt vacio")
	}
	m.mu.Lock()
	r, ok := m.runs[id]
	if !ok {
		m.mu.Unlock()
		return Run{}, fmt.Errorf("run no encontrado: %s", id)
	}
	if r.info.Status == StatusRunning {
		m.mu.Unlock()
		return Run{}, ErrBusy{RunID: id}
	}
	if active, busy := m.byDir[r.dir]; busy {
		m.mu.Unlock()
		return Run{}, ErrBusy{RunID: active}
	}
	// Command is a pure translation: safe under the lock, and a refusal
	// leaves the run exactly as it was.
	// the session to resume: the one the provider reported, else the one
	// this run was asked to resume from the start
	resumeID := r.info.ProviderSessionID
	if resumeID == "" {
		resumeID = r.opts.ResumeID
	}
	inv, err := r.provider.Command(r.opts.request(prompt, resumeID, true))
	if err != nil {
		m.mu.Unlock()
		return Run{}, err
	}
	r.info.Status = StatusRunning
	r.info.ExitCode = -1
	r.canceled = false
	r.meta.Status, r.meta.ExitCode = StatusRunning, -1
	meta := r.meta
	m.byDir[r.dir] = id
	m.mu.Unlock()
	writeMeta(m.root, meta)

	m.warnIgnored(r, inv.Ignored)
	if contains(inv.Ignored, providers.FieldContinue) {
		m.append(r, Event{TS: time.Now().UTC(), Kind: "text",
			Detail: fmt.Sprintf("aviso: %s no soporta continuar sesion en headless; se lanza una invocacion nueva", r.provider.Name())})
	}
	m.mu.Lock()
	info := r.info
	m.mu.Unlock()
	go m.execute(r, inv)
	return info, nil
}

// warnIgnored records declared degradation: one visible line per request
// field the provider could not honor.
func (m *Manager) warnIgnored(r *run, ignored []string) {
	for _, f := range ignored {
		m.append(r, Event{TS: time.Now().UTC(), Kind: "text",
			Detail: fmt.Sprintf("aviso: %s no soporta %s; se ignora", r.provider.Name(), f)})
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Cancel terminates the active subprocess. The log survives complete.
func (m *Manager) Cancel(id string) (Run, error) {
	m.mu.Lock()
	r, ok := m.runs[id]
	if !ok {
		m.mu.Unlock()
		return Run{}, fmt.Errorf("run no encontrado: %s", id)
	}
	if r.info.Status != StatusRunning || r.cancel == nil {
		info := r.info
		m.mu.Unlock()
		return info, fmt.Errorf("el run %s no esta corriendo (estado %s)", id, info.Status)
	}
	r.canceled = true
	cancel := r.cancel
	m.mu.Unlock()
	// El estado terminal (canceled) y el evento de cierre los escribe
	// settle cuando el subproceso muere: el log siempre queda completo.
	cancel()
	return m.Get(id)
}

// Get returns a run's current public state.
func (m *Manager) Get(id string) (Run, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, fmt.Errorf("run no encontrado: %s", id)
	}
	return r.info, nil
}

// Events returns the run's events from index `after` on (incremental poll).
func (m *Manager) Events(id string, after int) (Run, []Event, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return Run{}, nil, fmt.Errorf("run no encontrado: %s", id)
	}
	if after < 0 {
		after = 0
	}
	if after > len(r.events) {
		after = len(r.events)
	}
	evs := make([]Event, len(r.events)-after)
	copy(evs, r.events[after:])
	return r.info, evs, nil
}

// List returns every run, newest first.
func (m *Manager) List() []Run {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Run, 0, len(m.runs))
	for _, r := range m.runs {
		out = append(out, r.info)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].CreatedAt.After(out[i].CreatedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// Wait blocks until the run leaves the running state (CLI usage).
func (m *Manager) Wait(id string) Run {
	for {
		info, err := m.Get(id)
		if err != nil || info.Status != StatusRunning {
			return info
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// execute runs ONE subprocess invocation of the provider and settles state.
func (m *Manager) execute(r *run, inv providers.Invocation) {
	ctx, cancel := context.WithTimeout(context.Background(), m.Timeout)
	m.mu.Lock()
	r.cancel = cancel
	m.mu.Unlock()
	defer cancel()

	cmd := exec.CommandContext(ctx, inv.Bin, inv.Args...)
	cmd.Dir = r.dir

	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	if err1 != nil || err2 != nil {
		m.settle(r, StatusError, -1, "no se pudo lanzar el provider "+r.provider.Name()+": sin pipes")
		return
	}
	if err := cmd.Start(); err != nil {
		// the cause matters: a missing binary and an argv too large (E2BIG
		// with a huge --system-prompt) look the same otherwise
		m.settle(r, StatusError, -1, fmt.Sprintf("no se pudo lanzar el provider %s: %v", r.provider.Name(), err))
		return
	}
	// Si un hijo huerfano del provider retiene los pipes tras el kill,
	// los cerramos por la fuerza pasada una gracia: cancelar (o el timeout)
	// nunca puede colgar el cockpit esperando un EOF que no llega.
	go func() {
		<-ctx.Done()
		time.Sleep(3 * time.Second)
		stdout.Close()
		stderr.Close()
	}()

	var wg sync.WaitGroup
	scan := func(src interface{ Read([]byte) (int, error) }, isErr bool) {
		defer wg.Done()
		sc := bufio.NewScanner(src)
		sc.Buffer(make([]byte, 0, 64*1024), 4<<20) // stream-json lines can be huge
		for sc.Scan() {
			line := sc.Text()
			if isErr {
				if strings.TrimSpace(line) != "" {
					m.append(r, Event{TS: time.Now().UTC(), Kind: "text", Detail: "stderr: " + line})
				}
				continue
			}
			for _, ev := range r.provider.Normalize(line) {
				m.append(r, ev)
			}
		}
	}
	wg.Add(2)
	go scan(stdout, false)
	go scan(stderr, true)
	wg.Wait()
	err := cmd.Wait()

	exit := 0
	status := StatusDone
	detail := "invocacion completada"
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		status, exit = StatusError, -1
		detail = fmt.Sprintf("timeout tras %s (¿la CLI espera un permiso interactivo? preconfigura sus permisos headless)", m.Timeout)
	case m.wasCanceled(r):
		status, exit = StatusCanceled, -1
		detail = "cancelado por el operador"
	case err != nil:
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else {
			exit = -1
		}
		status = StatusError
		detail = fmt.Sprintf("el provider termino con exit %d", exit)
	}
	m.settle(r, status, exit, detail)
}

func (m *Manager) wasCanceled(r *run) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return r.canceled
}

func (m *Manager) settle(r *run, status string, exit int, detail string) {
	kind := "end"
	if status != StatusDone {
		kind = "error"
	}
	m.append(r, Event{TS: time.Now().UTC(), Kind: kind, Detail: detail})
	m.mu.Lock()
	r.info.Status = status
	r.info.ExitCode = exit
	r.cancel = nil
	r.meta.Status, r.meta.ExitCode = status, exit
	r.meta.ProviderSessionID, r.meta.EndedAt = r.info.ProviderSessionID, time.Now().UTC()
	meta := r.meta
	if m.byDir[r.dir] == r.info.ID {
		delete(m.byDir, r.dir)
	}
	m.mu.Unlock()
	writeMeta(m.root, meta)
}

// append records one event in memory and in the append-only jsonl log.
// A session id reported by the provider becomes the run's handle.
func (m *Manager) append(r *run, ev Event) {
	m.mu.Lock()
	r.events = append(r.events, ev)
	r.info.NumEvents = len(r.events)
	if ev.SessionID != "" {
		r.info.ProviderSessionID = ev.SessionID
	}
	log := r.log
	m.mu.Unlock()
	if log != nil {
		if enc, err := json.Marshal(ev); err == nil {
			log.Write(append(enc, '\n'))
		}
	}
}

func displayDir(task string) string {
	if task == "" {
		return "el proyecto"
	}
	return "el worktree de la tarea " + task
}
