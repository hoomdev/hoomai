// Package runcmd drives headless AI-CLI runs: hoomAI's cockpit engine.
// A run is one chat session bound to one directory (project root or a task
// worktree), executed as a sequence of subprocess invocations of the user's
// own CLI. Narration events land in .hoom/runs/<id>.jsonl — LOCAL telemetry,
// excluded from Git and from the change-candidate fingerprint: what travels
// is evidence (verdicts), never narration. One active run per directory,
// mirroring the harness rule of one writer per task.
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
}

// ErrBusy signals that the target directory already has an active run.
type ErrBusy struct{ RunID string }

func (e ErrBusy) Error() string {
	return fmt.Sprintf("ya hay un run en curso sobre este arbol (%s); espera o cancelalo (un writer por tarea)", e.RunID)
}

type run struct {
	info     Run
	dir      string
	spec     providers.Spec
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
func (m *Manager) dirFor(task string) (string, error) {
	if task == "" {
		return m.root, nil
	}
	wt := filepath.Join(m.root, ".hoom", "worktrees", task)
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
// returns ErrBusy with the running id.
func (m *Manager) Start(provider, prompt, task string) (Run, error) {
	if strings.TrimSpace(prompt) == "" {
		return Run{}, fmt.Errorf("prompt vacio")
	}
	spec, err := providers.Lookup(provider)
	if err != nil {
		return Run{}, err
	}
	if _, err := exec.LookPath(spec.Bin); err != nil {
		return Run{}, fmt.Errorf("el provider %q no esta instalado (no se encontro %q en PATH); instala su CLI primero", provider, spec.Bin)
	}
	dir, err := m.dirFor(task)
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
		info: Run{ID: id, Provider: provider, Task: task, Status: StatusRunning, ExitCode: -1, CreatedAt: time.Now().UTC()},
		dir:  dir, spec: spec, log: logFile,
	}
	m.runs[id] = r
	m.byDir[dir] = id
	m.mu.Unlock()

	m.append(r, Event{TS: time.Now().UTC(), Kind: "start",
		Detail: fmt.Sprintf("run %s: %s en %s", id, provider, displayDir(task))})
	// snapshot ANTES de lanzar la goroutine: execute escribe r.info en
	// paralelo y una copia sin lock seria una carrera de datos.
	m.mu.Lock()
	info := r.info
	m.mu.Unlock()
	go m.execute(r, prompt, false)
	return info, nil
}

// Input continues a finished run's session with the next prompt (the "spec
// aprobado, adelante" of the tutorial). Providers without native headless
// resume start fresh and the run says so — declared degradation.
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
	r.info.Status = StatusRunning
	r.info.ExitCode = -1
	r.canceled = false
	m.byDir[r.dir] = id
	m.mu.Unlock()

	cont := true
	if !r.spec.CanContinue {
		cont = false
		m.append(r, Event{TS: time.Now().UTC(), Kind: "text",
			Detail: fmt.Sprintf("aviso: %s no soporta continuar sesion en headless; se lanza una invocacion nueva", r.spec.Name)})
	}
	m.mu.Lock()
	info := r.info
	m.mu.Unlock()
	go m.execute(r, prompt, cont)
	return info, nil
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
func (m *Manager) execute(r *run, prompt string, cont bool) {
	ctx, cancel := context.WithTimeout(context.Background(), m.Timeout)
	m.mu.Lock()
	r.cancel = cancel
	m.mu.Unlock()
	defer cancel()

	bin, args := r.spec.Command(prompt, cont)
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = r.dir

	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	if err1 != nil || err2 != nil || cmd.Start() != nil {
		m.settle(r, StatusError, -1, "no se pudo lanzar el provider "+r.spec.Name)
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
			for _, ev := range r.spec.Normalize(line) {
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
	if m.byDir[r.dir] == r.info.ID {
		delete(m.byDir, r.dir)
	}
	m.mu.Unlock()
}

// append records one event in memory and in the append-only jsonl log.
func (m *Manager) append(r *run, ev Event) {
	m.mu.Lock()
	r.events = append(r.events, ev)
	r.info.NumEvents = len(r.events)
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
