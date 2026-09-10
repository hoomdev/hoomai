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
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
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
	// Usage is what the run spent so far, ACCUMULATED across invocations:
	// every CLI verified reports per invocation (measured on a --resume,
	// which reported its own turn and its own cost, not the session total).
	Usage *providers.Usage `json:"usage,omitempty"`
}

// accumulate sums one invocation's usage into the run's total. A cost that
// nobody reported stays ABSENT: "spent 0" and "does not report cost" are
// different facts and only one of them is Codex.
func accumulate(total, ev *providers.Usage) *providers.Usage {
	if ev == nil {
		return total
	}
	sum := providers.Usage{}
	if total != nil {
		sum = *total
	}
	// el costo solo se suma entre los que SI lo reportaron: nil + 0.11 da
	// 0.11, y nil + nil sigue siendo nil — Codex no reporta y hoom no inventa
	if ev.CostUSD != nil {
		v := *ev.CostUSD
		if sum.CostUSD != nil {
			v += *sum.CostUSD
		}
		sum.CostUSD = &v
	}
	sum.Turns += ev.Turns
	sum.InputTokens += ev.InputTokens
	sum.CachedTokens += ev.CachedTokens
	sum.OutputTokens += ev.OutputTokens
	sum.DurationMS += ev.DurationMS
	return &sum
}

// StartOptions is everything a run needs to start: provider and prompt,
// where to run, and the request fields the provider may or may not honor
// (see providers.Capabilities). The run remembers them so a later Input
// re-applies them.
type StartOptions struct {
	Provider     string
	Prompt       string
	Task         string // task slug: run inside its worktree; "" = project root
	Dir          string // working directory, explicit; "" = se resuelve desde Task
	Role         string // role slug this run embodies; "" = `hoom run`, no role
	ResumeID     string // provider session id to resume in this new run
	Model        string
	SystemPrompt string
	AllowTools   []string
	DenyTools    []string
	ReadOnly     bool // the role does not write: every provider imposes it its own way
	Exec         bool // ...but it does run commands
	Unattended   bool // nobody answers prompts: the provider gets the role's tools up front
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
		ReadOnly: o.ReadOnly, Exec: o.Exec, Unattended: o.Unattended,
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

// ErrNoContinuation is what Input returns under Strict when the provider can
// neither resume a session by id nor continue the directory's last one. It is
// a DECISION, not an accident: continuing is the whole point of Input, and a
// fresh invocation would silently start from zero.
type ErrNoContinuation struct {
	Provider string
	RunID    string
}

func (e ErrNoContinuation) Error() string {
	return fmt.Sprintf("el provider %q no puede continuar una sesion (ni --resume ni --continue) y con strict una invocacion nueva no es continuar: el modelo empezaria de cero. Accion: reintenta sin strict, o usa un provider con sesion (mira 'hoom providers')", e.Provider)
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
	// Isolated turns the guarantee into a durable fact: these tests were
	// written BLIND, from this commit. A terminal line scrolls away; the
	// sidecar is what someone can check months later.
	Isolated     bool   `json:"isolated,omitempty"`
	IsolatedFrom string `json:"isolated_from,omitempty"`
	// Usage is the accumulated cost of the run. In the sidecar because it is
	// the only place that survives the process: a finished run has to be able
	// to say what it cost long after its manager is gone.
	Usage *providers.Usage `json:"usage,omitempty"`
	// PID is the process that owns the run while it runs: the only way another
	// process can tell a live run from one whose owner died.
	PID int `json:"pid,omitempty"`
}

// idRe is what an id may look like: one file name, no separators, no dots
// that could climb. A sidecar is addressed by its id, so an id is never
// allowed to carry a path.
var idRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// slugRe is the shape of roles and task slugs.
var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func validID(id string) bool {
	return idRe.MatchString(id) && id != "." && id != ".." && !strings.Contains(id, "..")
}

func slugOK(s string) bool { return s == "" || slugRe.MatchString(s) }

// validStatus accepts the vocabulary and the empty string (a sidecar that
// never said): anything else is not telemetry hoom wrote and never reaches a
// renderer.
func validStatus(s string) bool {
	switch s {
	case "", StatusRunning, StatusDone, StatusError, StatusCanceled:
		return true
	}
	return false
}

func metaPath(root, id string) string { return filepath.Join(runsDir(root), id+".meta.json") }

// writeMeta records a run's identity. Best-effort by contract: telemetry that
// cannot be written never breaks the run it describes.
func writeMeta(root string, meta Meta) {
	if !validID(meta.ID) {
		return // the id is the address: content never chooses the path
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return
	}
	hoomfs.AtomicWrite(metaPath(root, meta.ID), append(raw, '\n'), 0o644)
}

// readMeta loads one sidecar by id and refuses one that lies about its
// identity or its shape: the filename is the address, the content only
// describes it. A sidecar planted under another id, or with a status outside
// the vocabulary, is broken telemetry: ignored, never trusted, never
// rendered. Role and task that are not slugs are blanked, not trusted either.
func readMeta(root, id string) (Meta, bool) {
	if !validID(id) {
		return Meta{}, false
	}
	raw, err := os.ReadFile(metaPath(root, id))
	if err != nil {
		return Meta{}, false
	}
	var meta Meta
	if json.Unmarshal(raw, &meta) != nil || meta.ID != id || !validStatus(meta.Status) {
		return Meta{}, false
	}
	if !slugOK(meta.Role) {
		meta.Role = ""
	}
	if !slugOK(meta.Task) {
		meta.Task = ""
	}
	return meta, true
}

// ReadMeta loads one run's sidecar by id, with the same refusals as Metas:
// what the Studio needs to show a run it does not own, without listing all.
func ReadMeta(root, id string) (Meta, bool) { return readMeta(root, id) }

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
		meta, ok := readMeta(root, strings.TrimSuffix(e.Name(), ".meta.json"))
		if !ok {
			continue
		}
		out = append(out, meta)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out
}

// ReadEvents reads a run's narration from disk. The Studio needs it for runs
// that ANOTHER process started: their events are on disk, so they can be
// watched even though this manager never owned them. A malformed line is
// skipped, never fatal.
func ReadEvents(root, id string) ([]Event, error) {
	evs, _, err := ReadEventsFrom(root, id, 0)
	return evs, err
}

// ReadEventsFrom reads the narration from a byte offset and returns the
// events plus the offset of the next unread byte. The Studio polls the logs
// of runs it does not own every second; re-parsing the whole file each time
// would cost more the longer the run talks. A trailing line that does not
// parse is left for the next read: a writer may be mid-line.
func ReadEventsFrom(root, id string, offset int64) ([]Event, int64, error) {
	f, err := os.Open(filepath.Join(runsDir(root), id+".jsonl"))
	if err != nil {
		return nil, offset, fmt.Errorf("run no encontrado: %s", id)
	}
	defer f.Close()
	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			return nil, offset, err
		}
	}
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, offset, err
	}
	var out []Event
	consumed := 0
	for consumed < len(raw) {
		nl := bytes.IndexByte(raw[consumed:], '\n')
		line := raw[consumed:]
		if nl >= 0 {
			line = raw[consumed : consumed+nl]
		}
		var ev Event
		blank := len(bytes.TrimSpace(line)) == 0
		parsed := !blank && json.Unmarshal(line, &ev) == nil
		if nl < 0 && !parsed && !blank {
			break // a line still being written: read it next time, whole
		}
		if parsed {
			out = append(out, ev)
		}
		if nl < 0 {
			consumed = len(raw)
		} else {
			consumed += nl + 1
		}
	}
	return out, offset + int64(consumed), nil
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
		if m.liveElsewhere(strings.TrimSuffix(e.Name(), ".jsonl"), last.TS) {
			continue // its owner is alive and talking: a neighbor, not an orphan
		}
		orphan := Event{TS: time.Now().UTC(), Kind: "error", Detail: "run huerfano: hoom serve termino mientras corria"}
		if f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644); err == nil {
			enc, _ := json.Marshal(orphan)
			f.Write(append(enc, '\n'))
			f.Close()
		}
		m.settleMeta(strings.TrimSuffix(e.Name(), ".jsonl"))
	}
}

// settleMeta closes a sidecar left saying "running" by a process that died.
// Without this a dead run would keep a directory busy forever, and the
// envelope refuses to build a blind tree over a busy tree.
func (m *Manager) settleMeta(id string) {
	meta, ok := readMeta(m.root, id)
	if !ok || meta.Status != StatusRunning {
		return
	}
	meta.Status, meta.ExitCode, meta.EndedAt = StatusError, -1, time.Now().UTC()
	writeMeta(m.root, meta)
}

// liveElsewhere reports whether a run still in "running" belongs to a process
// that is alive and whose narration is recent enough to be a live invocation:
// every invocation is bounded by Timeout, so a longer silence is a dead run
// whose pid was reused. Without a pid (older sidecars) the log alone cannot
// prove life, and the run counts as an orphan as it always did.
func (m *Manager) liveElsewhere(id string, lastTS time.Time) bool {
	meta, ok := readMeta(m.root, id)
	if !ok || meta.PID == 0 || meta.Status != StatusRunning {
		return false
	}
	if !lastTS.IsZero() && time.Since(lastTS) > m.Timeout+5*time.Minute {
		return false
	}
	return alive(meta.PID)
}

// dirFor resolves where a run executes: an explicit directory (the envelope
// passes the blind tree there), the task's worktree, or the project root.
func (m *Manager) dirFor(opts StartOptions) (string, error) {
	if d := strings.TrimSpace(opts.Dir); d != "" {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			return "", fmt.Errorf("el directorio de trabajo %q no existe", d)
		}
		return d, nil
	}
	return TaskDir(m.root, opts.Task)
}

// blindDirName is where the envelope keeps its quarantines. A run whose
// directory hangs from there IS a blind run: that is all runcmd needs to know
// about the isolation, and the sidecar records it.
const blindDirName = "isolated"

// blindFrom reports whether dir is a blind tree and, if so, the commit it was
// built from.
func blindFrom(dir string) (string, bool) {
	parent := filepath.Dir(filepath.Clean(dir))
	if filepath.Base(parent) != blindDirName || filepath.Base(filepath.Dir(parent)) != ".hoom" {
		return "", false
	}
	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", true // es ciego aunque git no conteste; el commit es lo que falta
	}
	return strings.TrimSpace(string(out)), true
}

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

// Busy reports the run currently active in dir, if any. In-process state is
// authoritative; the sidecars cover a run another process started and has not
// settled yet. The envelope asks BEFORE building a blind tree: transplanting
// into a tree somebody else is editing would blame the role for the collision.
func (m *Manager) Busy(dir string) (string, bool) {
	m.mu.Lock()
	id, ok := m.byDir[dir]
	m.mu.Unlock()
	if ok {
		return id, true
	}
	for _, meta := range Metas(m.root) {
		if meta.Status == StatusRunning && meta.Dir == dir && (meta.PID == 0 || alive(meta.PID)) {
			return meta.ID, true
		}
	}
	return "", false
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
	dir, err := m.dirFor(opts)
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
		CreatedAt: r.info.CreatedAt, Status: StatusRunning, ExitCode: -1, PID: os.Getpid(),
	}
	r.meta.IsolatedFrom, r.meta.Isolated = blindFrom(dir)
	writeMeta(m.root, r.meta)
	m.runs[id] = r
	m.byDir[dir] = id
	m.mu.Unlock()

	m.append(r, Event{TS: time.Now().UTC(), Kind: "start",
		Detail: fmt.Sprintf("run %s: %s en %s", id, p.Name(), displayDir(opts, r.meta.Isolated))})
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
		// Strict forbids silent degradation, and losing the session is the
		// loudest one: a fresh invocation does not continue anything, the
		// model starts from zero. The refusal leaves the run exactly as it
		// was — terminal, its directory free, its log untouched.
		var unsup providers.ErrUnsupported
		if errors.As(err, &unsup) && contains(unsup.Fields, providers.FieldContinue) {
			return Run{}, ErrNoContinuation{Provider: r.provider.Name(), RunID: id}
		}
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

	// una sola linea sobre la sesion perdida, y que dice lo que importa: no
	// que se ignoro un campo, sino que el contexto anterior no viaja
	m.warnIgnored(r, without(inv.Ignored, providers.FieldContinue))
	if contains(inv.Ignored, providers.FieldContinue) {
		m.append(r, Event{TS: time.Now().UTC(), Kind: "text",
			Detail: fmt.Sprintf("aviso: %s no continua sesiones en headless; esta invocacion empieza de cero (el contexto anterior no viaja)", r.provider.Name())})
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

// without drops one field from an Ignored list: the caller that reports it
// with its own words does not want the generic line too.
func without(list []string, drop string) []string {
	var out []string
	for _, s := range list {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
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
	if m.byDir[r.dir] == r.info.ID {
		delete(m.byDir, r.dir)
	}
	// el meta se escribe ANTES de soltar el lock, como en Start: quien ve el
	// run cerrado (Get, Wait, Events) ya puede leer su meta cerrado, y ningun
	// archivo aparece despues de que el run dejo de estar en curso.
	writeMeta(m.root, r.meta)
	m.mu.Unlock()
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
	if ev.Usage != nil {
		r.info.Usage = accumulate(r.info.Usage, ev.Usage)
		r.meta.Usage = r.info.Usage
	}
	log := r.log
	m.mu.Unlock()
	if log != nil {
		if enc, err := json.Marshal(ev); err == nil {
			log.Write(append(enc, '\n'))
		}
	}
}

func displayDir(opts StartOptions, blind bool) string {
	if blind {
		return "un arbol ciego (sin implementacion en disco)"
	}
	if opts.Task == "" {
		return "el proyecto"
	}
	return "el worktree de la tarea " + opts.Task
}
