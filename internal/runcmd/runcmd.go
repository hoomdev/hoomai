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
	"crypto/sha256"
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
	"github.com/hoomdev/hoomai/internal/procesos"
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
	FindingTask  string // task the run's findings belong to (EnvTask); "" = Task
	Dir          string // working directory, explicit; "" = se resuelve desde Task
	Role         string // role slug this run embodies; "" = `hoom run`, no role
	ResumeID     string // provider session id to resume in this new run
	Model        string
	SystemPrompt string
	AllowTools   []string
	DenyTools    []string
	ReadOnly     bool // the role does not write: every provider imposes it its own way
	Exec         bool // ...but it does run commands
	NoExec       bool // the role writes but runs no commands: the provider withholds the shell
	Unattended   bool // nobody answers prompts: the provider gets the role's tools up front
	MaxTurns     int
	BudgetUSD    float64
	Strict       bool   // unsupported field = refuse to start instead of a warning
	Effort       string // reasoning effort, provider vocabulary; "" = provider default
	Isolated     bool   // run without the user's personal provider config
	PromptStdin  bool   // the prompt travels by stdin at any size
}

// request builds the provider request for these options.
func (o StartOptions) request(prompt, resumeID string, cont bool) providers.Request {
	return providers.Request{
		Prompt: prompt, ResumeID: resumeID, Continue: cont,
		Model: o.Model, SystemPrompt: o.SystemPrompt,
		AllowTools: o.AllowTools, DenyTools: o.DenyTools,
		ReadOnly: o.ReadOnly, Exec: o.Exec, NoExec: o.NoExec, Unattended: o.Unattended,
		MaxTurns: o.MaxTurns, BudgetUSD: o.BudgetUSD, Strict: o.Strict,
		Effort: o.Effort, Isolated: o.Isolated, PromptStdin: o.PromptStdin,
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
	// normalize is the run's normalizer, made once per run and not per
	// invocation: a delegation left open at the end of one invocation is
	// closed by a line that comes when Input continues the session
	// (finding 3bdc20)
	normalize func(line string) []Event
}

// normalizerFor is the normalizer of one run of p: a Correlating provider
// pairs each delegation with its end, so it remembers this run's lines.
func normalizerFor(p providers.Provider) func(line string) []Event {
	if c, ok := p.(providers.Correlating); ok {
		return c.NewNormalizer()
	}
	return p.Normalize
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
	// ChildPID is the provider's own process. If the owner dies without
	// killing it (hoom serve killed with SIGKILL), the run is still at work
	// while it lives, and nothing may act on its tree (finding b5e187).
	ChildPID int `json:"child_pid,omitempty"`
	// ChildStartedAt is when the provider started: a ChildPID that another
	// process reused started at another time (finding cea64f).
	ChildStartedAt time.Time `json:"child_started_at,omitempty"`
}

// vivo says whether the run of meta still has someone working: its owner, or
// the provider its owner left behind while its PID is still that provider
// (not another process that reused it: finding cea64f).
func vivo(meta Meta) bool {
	return meta.PID == 0 || alive(meta.PID) || provider(meta)
}

// provider says whether the provider of meta is still alive and still the
// same process.
func provider(meta Meta) bool {
	return meta.ChildPID > 0 && alive(meta.ChildPID) && procesos.MismoProceso(meta.ChildPID, meta.ChildStartedAt)
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

// writeMeta records a run's identity and says whether it could. Opening a run
// (Start, Input) and naming its provider (execute) need the record: without
// it another process would not see the run working on its tree, so they fail
// (findings da369a, 1da9c4). Closing records (settle, settleMeta) are
// best-effort: the run is already over, and a close that could not be
// written leaves the sidecar saying "running", the safe side: the tree looks
// busy until markOrphans settles it (finding 989bfd).
func writeMeta(root string, meta Meta) error {
	if !validID(meta.ID) {
		return fmt.Errorf("id de run invalido %q", meta.ID) // the id is the address: content never chooses the path
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return hoomfs.AtomicWrite(metaPath(root, meta.ID), append(raw, '\n'), 0o644)
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

// openLog opens the narration of run id in root only when it is a regular
// file of root's own runs: a valid name that is a symlink, or a .hoom or a
// runs that is one, would take the reading to another project (finding
// 058fb1). The file is opened without following a symlink (and without
// blocking on a FIFO), so what is read is what was checked.
func openLog(root, id string) (*os.File, error) {
	if !validID(id) {
		return nil, fmt.Errorf("run no encontrado: %s", id)
	}
	for _, d := range []string{filepath.Join(root, ".hoom"), runsDir(root)} {
		if st, err := os.Lstat(d); err != nil || !st.IsDir() {
			return nil, fmt.Errorf("run no encontrado: %s", id)
		}
	}
	f, err := hoomfs.AbrirRegular(filepath.Join(runsDir(root), id+".jsonl"))
	if err != nil {
		return nil, fmt.Errorf("run no encontrado: %s", id)
	}
	return f, nil
}

// ReadDelegations reads only the delegation events of a run (agent and
// agent_end), line by line: the card's history needs no more, and a log that
// carries long tool results is never loaded whole (finding d3c75f). A
// malformed line is skipped, as in ReadEvents.
func ReadDelegations(root, id string) ([]Event, error) {
	f, err := openLog(root, id)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Event
	keep := func(line []byte) {
		var ev Event
		if json.Unmarshal(bytes.TrimSpace(line), &ev) == nil && (ev.Kind == "agent" || ev.Kind == "agent_end") {
			out = append(out, ev)
		}
	}
	br := bufio.NewReaderSize(f, 64<<10)
	for {
		frag, err := br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			// a line longer than the buffer. Its first piece says whether it
			// is a candidate (kind is near the start); any other line is
			// consumed without being kept. A candidate is kept whole up to
			// maxDelegacion and decoded as usual; past that, only its head
			// (ts, kind, agent) and its last two pieces (tool_id) are kept, and it comes
			// back with its detail cut: memory stays bounded and the
			// delegation is still in the history (findings 2628ba, 0ebb3f,
			// 34a37c, c391ba)
			candidata := bytes.Contains(frag, marcaDelegacion)
			var cabeza, whole, prev, cur []byte
			if candidata {
				cabeza = append([]byte(nil), frag...)
			}
			dentroDelTope, size := true, 0
			for err == bufio.ErrBufferFull {
				size += len(frag)
				if candidata && dentroDelTope {
					if len(whole)+len(frag) > maxDelegacion {
						dentroDelTope, whole = false, nil
					} else {
						whole = append(whole, frag...)
					}
				}
				if candidata && !dentroDelTope {
					// the last two pieces, in two buffers that are reused:
					// reading a longer line allocates nothing more
					prev, cur = cur, prev
					cur = append(cur[:0], frag...)
				}
				frag, err = br.ReadSlice('\n')
			}
			size += len(frag)
			switch {
			case !candidata:
			case dentroDelTope && len(whole)+len(frag) <= maxDelegacion:
				keep(append(whole, frag...))
			default:
				cola := append(append(append([]byte(nil), prev...), cur...), frag...)
				if ev, ok := delegacionRecortada(cabeza, cola, size); ok {
					out = append(out, ev)
				}
			}
		} else if bytes.Contains(frag, marcaDelegacion) {
			keep(frag)
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

// maxDelegacion bounds the line of one delegation that ReadDelegations keeps
// whole: an agent or agent_end event says who and when, a few hundred bytes.
// A longer one comes back with its detail cut (delegacionRecortada).
const maxDelegacion = 1 << 20

var (
	campoKind   = regexp.MustCompile(`"kind"\s*:\s*"([a-z_]+)"`)
	campoTS     = regexp.MustCompile(`"ts"\s*:\s*"([^"\\]+)"`)
	campoAgent  = regexp.MustCompile(`"agent"\s*:\s*"([^"\\]*)"`)
	campoToolID = regexp.MustCompile(`"tool_id"\s*:\s*"([^"\\]*)"`)
)

// delegacionRecortada is the event of a delegation line too long to keep:
// its kind, ts and agent from the head of the line, its tool_id from the
// tail, and a detail that says it was cut. The top-level kind is the first
// one in the line (an event writes it before its detail), so a text event
// that only mentions "agent" in its detail is not one.
func delegacionRecortada(cabeza, cola []byte, size int) (Event, bool) {
	k := campoKind.FindSubmatch(cabeza)
	if k == nil || (string(k[1]) != "agent" && string(k[1]) != "agent_end") {
		return Event{}, false
	}
	ev := Event{Kind: string(k[1]),
		Detail: fmt.Sprintf("(detalle de %d bytes, recortado al leer la historia)", size)}
	if m := campoTS.FindSubmatch(cabeza); m != nil {
		ev.TS, _ = time.Parse(time.RFC3339Nano, string(m[1]))
	}
	if m := campoAgent.FindSubmatch(cabeza); m != nil {
		ev.Agent = string(m[1])
	}
	if all := campoToolID.FindAllSubmatch(cola, -1); len(all) > 0 {
		ev.ToolID = string(all[len(all)-1][1])
	}
	return ev, true
}

// marcaDelegacion is a cheap first look at a line: every agent or agent_end
// event has it however its JSON is spaced ("kind":"agent" or "kind": "agent"),
// and only lines that have it are decoded; decoding decides by kind, so a
// line that merely mentions it is decoded and dropped.
var marcaDelegacion = []byte(`"agent`)

// ReadEventsFrom reads the narration from a byte offset and returns the
// events plus the offset of the next unread byte. The Studio polls the logs
// of runs it does not own every second; re-parsing the whole file each time
// would cost more the longer the run talks. A trailing line that does not
// parse is left for the next read: a writer may be mid-line.
func ReadEventsFrom(root, id string, offset int64) ([]Event, int64, error) {
	// the id is the address, as for a sidecar: a run_id that climbs
	// (../../other/.hoom/runs/x) never reaches a log outside runs, nor does
	// a symlink (findings f47433, 058fb1: the history reads run ids from
	// envelopes)
	f, err := openLog(root, id)
	if err != nil {
		return nil, offset, err
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
	_ = writeMeta(m.root, meta) // best-effort: a close (see writeMeta)
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
	// the provider its owner left behind is still at work, as long as it
	// lives and is the same process: no narration speaks for it, nobody is
	// left to write it (findings b5e187 and cea64f)
	if provider(meta) {
		return true
	}
	if !lastTS.IsZero() && time.Since(lastTS) > m.Timeout+5*time.Minute {
		return false // too old: a live PID may be another process by now
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

// EnvTask is the variable a run puts in its provider's environment with the
// task it belongs to. Whatever the role runs inside — `hoom finding add`
// above all — inherits it, so a finding lands tied to its task without the
// role's contract having to know.
const EnvTask = "HOOM_TASK"

// EnvRole, EnvRun and EnvProvider are what a run puts in its provider's
// environment about itself: the role it embodies, its id and its provider.
// `hoom finding resolve` reads them to know it runs inside a role's run.
const (
	EnvRole     = "HOOM_ROLE"
	EnvRun      = "HOOM_RUN"
	EnvProvider = "HOOM_PROVIDER"
)

// CurrentTask answers which task this process works for: an explicit flag
// wins; without one, the task the run that launched us put in EnvTask;
// without either, none.
func CurrentTask(flag string) string {
	if t := strings.TrimSpace(flag); t != "" {
		return t
	}
	return strings.TrimSpace(os.Getenv(EnvTask))
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
	return Active(m.root, dir)
}

// Check is Start's translation of opts without starting anything: the
// provider exists and takes every option asked for (Strict refuses what it
// does not). A caller that must decide before writing anything asks here.
func Check(opts StartOptions) error {
	p, err := providers.Lookup(opts.Provider)
	if err != nil {
		return err
	}
	_, err = p.Command(opts.request(opts.Prompt, opts.ResumeID, false))
	return err
}

// EsperaArbol is how long a discard or a save waits for a run that is being
// registered on the same tree.
const EsperaArbol = 2 * time.Second

// esperaRegistro is how long registering a run waits for a discard or a save
// of its tree to finish.
const esperaRegistro = 30 * time.Second

// LockArbol takes the lock of the working tree dir. Registering a run on it
// (Start, Input) and checking-then-acting on it (Discard, Save) take it, so
// no run starts between a "no run is active" check and the action that
// relies on it (findings b6e55b and ac2feb); a run does not hold it while it
// works. It lives in the repository's common git dir, keyed by the tree's
// real path: the CLI in the project, the CLI in a task worktree and the
// Studio find the same one, and nothing is written inside the tree (a blind
// tree stays as it is). A dir outside any git repository has nothing a
// discard or a save could race with: the lock is then a no-op. Any other
// failure of git (missing, unsafe directory, no permission) is an error:
// two processes that disagree on where the lock is would not exclude each
// other (finding 7a04de).
func LockArbol(dir string, wait time.Duration) (func(), error) {
	real, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command("git", "-C", real, "rev-parse", "--path-format=absolute", "--git-common-dir")
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C") // the message is read: untranslated
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if strings.Contains(stderr.String(), "not a git repository") {
			return func() {}, nil
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("no pude ubicar el candado del arbol %s: git rev-parse: %s", real, msg)
	}
	sum := sha256.Sum256([]byte(real))
	path := filepath.Join(strings.TrimSpace(string(out)), "hoom-locks", hex.EncodeToString(sum[:8])+".lock")
	return hoomfs.Lock(path, wait)
}

// lockRegistro is LockArbol for registering a run: it waits longer, and says
// what it waited for.
func lockRegistro(dir string) (func(), error) {
	unlock, err := LockArbol(dir, esperaRegistro)
	if errors.Is(err, hoomfs.ErrLocked) {
		return nil, fmt.Errorf("se esta guardando o descartando el trabajo de %s: el run no arranco. Accion: vuelve a lanzarlo cuando termine", dir)
	}
	return unlock, err
}

// Active is the live run whose tree is dir, from its sidecar: in root's runs
// and in dir's own, where a run started from inside a task's worktree keeps
// it (finding f237b9).
func Active(root, dir string) (string, bool) {
	for _, base := range []string{root, dir} {
		for _, meta := range Metas(base) {
			if meta.Status == StatusRunning && meta.Dir == dir && vivo(meta) {
				return meta.ID, true
			}
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
	// the run is registered under its tree's lock: a discard or a save that
	// checked "no run is active" has finished acting when it gets it
	unlock, err := lockRegistro(dir)
	if err != nil {
		return Run{}, err
	}
	defer unlock()
	// a run another process registered on the tree counts too: under the
	// lock, nobody registers one between this check and ours (finding
	// 57eff9)
	if id, ok := Active(m.root, dir); ok {
		return Run{}, ErrBusy{RunID: id}
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
		dir:  dir, provider: p, opts: opts, log: logFile, normalize: normalizerFor(p),
	}
	r.meta = Meta{
		ID: id, Provider: p.Name(), Role: opts.Role, Task: opts.Task, Dir: dir,
		CreatedAt: r.info.CreatedAt, Status: StatusRunning, ExitCode: -1, PID: os.Getpid(),
	}
	r.meta.IsolatedFrom, r.meta.Isolated = blindFrom(dir)
	// the sidecar is how another process sees this run: without it the run
	// would not exclude anybody, so it does not start (finding da369a)
	if err := writeMeta(m.root, r.meta); err != nil {
		m.mu.Unlock()
		logFile.Close()
		os.Remove(filepath.Join(runsDir(m.root), id+".jsonl"))
		return Run{}, fmt.Errorf("no se pudo registrar el run: %v", err)
	}
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
	m.mu.Unlock()
	if !ok {
		return Run{}, fmt.Errorf("run no encontrado: %s", id)
	}
	// like Start: the run goes back to running under its tree's lock, and
	// not while another process's run works on the tree
	unlock, err := lockRegistro(r.dir)
	if err != nil {
		return Run{}, err
	}
	defer unlock()
	if active, ok := Active(m.root, r.dir); ok && active != id {
		return Run{}, ErrBusy{RunID: active}
	}
	m.mu.Lock()
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
	// like Start: the sidecar is how another process sees the run working
	// again, so without it the run does not continue, and the refusal leaves
	// it exactly as it was (finding 1da9c4)
	meta := r.meta
	meta.Status, meta.ExitCode = StatusRunning, -1
	if err := writeMeta(m.root, meta); err != nil {
		m.mu.Unlock()
		return Run{}, fmt.Errorf("no se pudo registrar la continuacion del run: %v", err)
	}
	r.info.Status = StatusRunning
	r.info.ExitCode = -1
	r.canceled = false
	r.meta = meta
	m.byDir[r.dir] = id
	m.mu.Unlock()

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
	// The run tells whatever it launches which task it belongs to, which
	// role it embodies, its id and its provider — always, empty without a
	// value, so one inherited from the parent never leaks in. exec keeps the
	// LAST value of a repeated key.
	task := r.opts.FindingTask
	if task == "" {
		task = r.opts.Task
	}
	cmd.Env = append(os.Environ(), EnvTask+"="+task, EnvRole+"="+r.opts.Role,
		EnvRun+"="+r.info.ID, EnvProvider+"="+r.opts.Provider)
	// a prompt too large for argv (E2BIG on Linux, the 32 KiB command line on
	// Windows) travels by stdin; the adapter left it out of Args. It is a
	// pipe WE write and close, not an exec copier: an orphan that inherits
	// stdin and never reads cannot keep Wait from returning.
	var stdin io.WriteCloser
	if inv.Stdin != "" {
		w, err := cmd.StdinPipe()
		if err != nil {
			m.settle(r, StatusError, -1, "no se pudo lanzar el provider "+r.provider.Name()+": sin pipe de entrada")
			return
		}
		stdin = w
	}

	stdout, err1 := cmd.StdoutPipe()
	stderr, err2 := cmd.StderrPipe()
	if err1 != nil || err2 != nil {
		m.settle(r, StatusError, -1, "no se pudo lanzar el provider "+r.provider.Name()+": sin pipes")
		return
	}
	if err := arrancar(cmd); err != nil {
		// the cause matters: a missing binary and an argv too large (E2BIG
		// with a huge --system-prompt) look the same otherwise
		m.settle(r, StatusError, -1, fmt.Sprintf("no se pudo lanzar el provider %s: %v", r.provider.Name(), err))
		return
	}
	defer soltarGrupo(cmd.Process.Pid)
	// the provider's PID keeps the run alive for other processes if this
	// one dies: without it on disk the run is not protected, so it does not
	// go on (finding da369a)
	m.mu.Lock()
	r.meta.ChildPID, r.meta.ChildStartedAt = cmd.Process.Pid, time.Now().UTC()
	errPID := writeMeta(m.root, r.meta)
	m.mu.Unlock()
	if errPID != nil {
		procesos.MatarGrupo(cmd.Process.Pid)
	}
	// Si un hijo huerfano del provider retiene los pipes tras el kill,
	// los cerramos por la fuerza pasada una gracia: cancelar (o el timeout)
	// nunca puede colgar el cockpit esperando un EOF que no llega.
	go func() {
		<-ctx.Done()
		time.Sleep(3 * time.Second)
		stdout.Close()
		stderr.Close()
		if stdin != nil {
			stdin.Close() // destraba la escritura del prompt si nadie lo lee
		}
	}()
	// escrito dice si el pedido entero entro por stdin: un CLI que lee un
	// prefijo y sale con 0 no puede cerrar el run como hecho
	var escrito chan error
	if stdin != nil {
		escrito = make(chan error, 1)
		go func() {
			_, err := io.WriteString(stdin, inv.Stdin)
			stdin.Close() // su error no importa: Wait tambien lo cierra
			escrito <- err
		}()
	}

	// the run's normalizer, the same for every invocation of the run
	normalize := r.normalize
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
			for _, ev := range normalize(line) {
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
	if errPID != nil {
		status, exit = StatusError, -1
		detail = fmt.Sprintf("no se pudo registrar el PID del provider (%v): el run se corto", errPID)
	}
	if escrito != nil {
		// Wait ya cerro el pipe: la escritura termino, bien o mal
		if werr := <-escrito; werr != nil && status == StatusDone {
			status = StatusError
			detail = fmt.Sprintf("el provider no leyo el pedido entero por stdin (%d bytes): %v", len(inv.Stdin), werr)
		}
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
	_ = writeMeta(m.root, r.meta) // best-effort: a close (see writeMeta)
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

// Alive reports whether a process with this pid exists: how another process
// tells a live run from one whose owner died.
func Alive(pid int) bool { return alive(pid) }
