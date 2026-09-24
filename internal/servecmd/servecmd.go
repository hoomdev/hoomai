// Package servecmd implements `hoom serve`: the HoomAI Studio, a dashboard
// embedded in the hoom binary itself. The Studio is a remote control of the
// harness, never a second brain: every endpoint reuses the exact same
// internal functions (and byte-identical JSON) as the CLI verbs it maps to.
// Logic the CLI does not have does not exist here. Reads are open on
// loopback; every action (POST) demands the session token printed once at
// startup, so a random local page cannot drive the harness (CSRF).
package servecmd

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/cockpitcmd"
	"github.com/hoomdev/hoomai/internal/contextcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/filesearch"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/report"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/statuscmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

// DefaultAddr is loopback-only by construction: exposing the Studio to a
// network is a conscious decision via --addr, never the default.
const DefaultAddr = "127.0.0.1:4666"

// TokenHeader carries the action token on every POST.
const TokenHeader = "X-Hoom-Token"

// Listing caps: how many items an unqualified request returns.
const (
	defaultVerdictsN = 50
	defaultReportN   = 10
	maxUploadBytes   = 32 << 20
)

//go:embed ui
var uiFS embed.FS

// Server serves the Studio for one project.
type Server struct {
	m        *manifest.Manifest
	token    string
	verifyMu sync.Mutex // one verify at a time per tree; TryLock => 409
	runs     *runcmd.Manager

	// logs is what this process remembers of the runs it does NOT own: the
	// narration already parsed and the byte it ends at. The UI polls every
	// second; without this each poll re-read and re-parsed the whole file,
	// a cost that grew with every line the run spoke.
	logMu sync.Mutex
	logs  map[string]*foreignLog

	// cabina (C3): the process boundary of the cockpit session and the
	// terminal mirror (tests swap it), and the roles this Studio launched
	// and is still running.
	cockpit   cockpitcmd.Deps
	launches  sync.WaitGroup
	launchMu  sync.Mutex
	launching map[string]bool // slug -> a launch of that card is being set up
}

type foreignLog struct {
	offset int64
	events []runcmd.Event
}

// New loads the project's manifest and mints the per-session action token.
// Outside a hoom project it fails with the exact action to take (hoom init),
// which the CLI turns into exit 1.
func New(dir string) (*Server, error) {
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		return nil, err
	}
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return nil, err
	}
	return &Server{m: m, token: hex.EncodeToString(raw), runs: runcmd.NewManager(m.Dir), logs: map[string]*foreignLog{},
		cockpit: cockpitcmd.DefaultDeps(), launching: map[string]bool{}}, nil
}

// Token returns the per-session action token. It is printed exactly once at
// startup; there is no endpoint that reveals it.
func (s *Server) Token() string { return s.token }

type statusResp struct {
	Project    string          `json:"project"`
	Profile    string          `json:"profile"`
	Policy     string          `json:"policy"`
	BaseBranch string          `json:"base_branch"`
	Check      checkcmd.Result `json:"check"`
	// FindingsBlockOn is the findings_open threshold ("" = findings never block).
	FindingsBlockOn string `json:"findings_block_on"`
}

type verdictsResp struct {
	Total    int                `json:"total"`
	Warnings []string           `json:"warnings"`
	Verdicts []*verdict.Verdict `json:"verdicts"`
}

type specItem struct {
	Name     string           `json:"name"`
	Path     string           `json:"path"`
	State    string           `json:"state"` // aprobado | no-aprobado | invalidado
	Approval *approval.Record `json:"approval,omitempty"`
}

type specDetail struct {
	specItem
	Markdown string   `json:"markdown"`
	Criteria []string `json:"criteria"`
	Review   string   `json:"review,omitempty"`
}

// Handler returns the Studio's HTTP handler: embedded UI at /, reads under
// /api/ open, actions token-gated.
func (s *Server) Handler() http.Handler {
	mux, _ := s.handler()
	return mux
}

// handler builds the mux and the list of what it registered. Nothing is kept
// on the Server: Handler may be called from several goroutines at once.
func (s *Server) handler() (*http.ServeMux, []string) {
	mux := http.NewServeMux()
	var routes []string
	// handle is the ONLY way a route is registered: Routes() reads the same
	// list, and the golden rule (no route receives a column) is asserted on it.
	handle := func(pattern string, h http.HandlerFunc) {
		mux.HandleFunc(pattern, h)
		routes = append(routes, pattern)
	}

	ui, _ := fs.Sub(uiFS, "ui")
	handle("GET /", http.FileServerFS(ui).ServeHTTP)

	handle("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		check, err := checkcmd.Run(s.m.Dir, s.m.BaseBranch)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, statusResp{
			Project:         s.m.Project,
			Profile:         s.m.Profile,
			Policy:          s.m.Policy,
			BaseBranch:      s.m.BaseBranch,
			Check:           check,
			FindingsBlockOn: s.m.FindingsBlockOn(),
		})
	})

	handle("GET /api/verdicts", func(w http.ResponseWriter, r *http.Request) {
		all, warnings, err := verdict.LoadAllWithWarnings(s.m.Dir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		n := queryN(r, defaultVerdictsN)
		resp := verdictsResp{Total: len(all), Warnings: warnings, Verdicts: []*verdict.Verdict{}}
		if resp.Warnings == nil {
			resp.Warnings = []string{}
		}
		// LoadAll is oldest-first; the Studio reads newest-first.
		for i := len(all) - 1; i >= 0 && len(resp.Verdicts) < n; i-- {
			resp.Verdicts = append(resp.Verdicts, all[i])
		}
		writeJSON(w, resp)
	})

	handle("GET /api/verdicts/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		all, _, err := verdict.LoadAllWithWarnings(s.m.Dir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for _, v := range all {
			if v.ID == id {
				writeJSON(w, v)
				return
			}
		}
		writeError(w, http.StatusNotFound, "veredicto no encontrado: "+id)
	})

	handle("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		raw, err := taskcmd.JSONBytes(s.m.Dir, s.m.BaseBranch)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	handle("GET /api/report", func(w http.ResponseWriter, r *http.Request) {
		all, _, err := verdict.LoadAllWithWarnings(s.m.Dir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		raw, err := report.JSONBytes(all, queryN(r, defaultReportN))
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	handle("GET /api/files", func(w http.ResponseWriter, r *http.Request) {
		// solo RUTAS, jamas contenido: el indice es git ls-files
		matches := filesearch.Match(filesearch.List(s.m.Dir), r.URL.Query().Get("q"), 20)
		if matches == nil {
			matches = []string{}
		}
		writeJSON(w, matches)
	})

	handle("GET /api/findings", func(w http.ResponseWriter, r *http.Request) {
		raw, err := finding.JSONBytes(s.m.Dir, s.m.BaseBranch, r.URL.Query().Get("open") == "1")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	handle("GET /api/context", func(w http.ResponseWriter, r *http.Request) {
		raw, err := contextcmd.JSONBytes(s.m.Dir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	handle("GET /api/specs", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.listSpecs()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, items)
	})

	handle("GET /api/specs/{name}", func(w http.ResponseWriter, r *http.Request) {
		d, code, err := s.specDetail(r.PathValue("name"))
		if err != nil {
			writeError(w, code, err.Error())
			return
		}
		writeJSON(w, d)
	})

	// --- cabina: el tablero. Solo lectura: la columna y todo lo que la
	// tarjeta muestra lo calcula boardcmd; la pagina pinta. Ningun metodo
	// que no sea GET llega aca (el mux responde 405).

	handle("GET /api/board", func(w http.ResponseWriter, r *http.Request) {
		b, err := boardcmd.Build(s.m.Dir, s.m.BaseBranch, s.m.FindingsBlockOn(), time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		// los mismos bytes que `hoom board --json`: una representacion, dos pieles
		raw, err := boardcmd.JSONBytes(b)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	handle("GET /api/board/{slug}", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if !item.ValidSlug(slug) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", slug))
			return
		}
		d, err := boardcmd.DetailFor(s.m.Dir, s.m.BaseBranch, s.m.FindingsBlockOn(), slug,
			time.Now().UTC(), r.URL.Query().Get("diff") == "1")
		if err != nil {
			writeError(w, itemStatus(err), err.Error())
			return
		}
		writeJSON(w, d)
	})

	// la historia de la tarjeta (C4): git + telemetria local, calculada cada
	// vez que alguien la pide. Solo lectura, como el detalle.
	handle("GET /api/board/{slug}/timeline", func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("slug")
		if !item.ValidSlug(slug) {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", slug))
			return
		}
		tl, err := boardcmd.TimelineFor(s.m.Dir, s.m.BaseBranch, s.m.FindingsBlockOn(), slug, time.Now().UTC())
		if err != nil {
			writeError(w, itemStatus(err), err.Error())
			return
		}
		writeJSON(w, tl)
	})

	// el trinquete (C4): la misma vista que la seccion de `hoom status`; lee
	// el archivo y nunca mide.
	handle("GET /api/ratchet", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, statuscmd.Ratchet(s.m.Dir))
	})

	// --- cabina (C3): las acciones de la tarjeta. Ninguna recibe una
	// columna: producen evidencia y la columna la recalcula boardcmd.
	handle("POST /api/board/{slug}/launch", s.authed(s.launch))
	handle("POST /api/board/{slug}/save", s.authed(s.save))
	handle("POST /api/board/{slug}/discard", s.authed(s.discard))
	handle("POST /api/board/{slug}/session", s.authed(s.session))
	handle("GET /api/board/{slug}/terminal", s.terminal)

	// --- cockpit: providers y runs headless. Los eventos de un run son
	// NARRACION local (.hoom/runs/, fuera de huella y de Git); la evidencia
	// sigue siendo el veredicto.

	handle("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		raw, err := providers.JSONBytes()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	handle("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.runRows())
	})

	// los sobres: lo que `hoom agent` esta haciendo AHORA, aunque lo corra
	// otro proceso. El Studio los mira; no los maneja.
	handle("GET /api/envelopes", func(w http.ResponseWriter, r *http.Request) {
		recs := envelope.List(s.m.Dir)
		if recs == nil {
			recs = []envelope.Record{}
		}
		writeJSON(w, recs)
	})

	handle("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		after := 0
		if raw := r.URL.Query().Get("after"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				after = n
			}
		}
		row, evs, err := s.runEvents(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		if after > len(evs) {
			after = len(evs)
		}
		writeJSON(w, map[string]any{"run": row, "events": evs[after:], "next": len(evs)})
	})

	handle("GET /api/runs/{id}/stage", func(w http.ResponseWriter, r *http.Request) {
		row, evs, err := s.runEvents(r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, runcmd.Stage(row.Run, evs))
	})

	handle("POST /api/runs", s.authed(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Provider string `json:"provider"`
			Prompt   string `json:"prompt"`
			Task     string `json:"task"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		info, err := s.runs.Start(runcmd.StartOptions{Provider: body.Provider, Prompt: body.Prompt, Task: body.Task})
		if err != nil {
			writeError(w, runErrCode(err), err.Error())
			return
		}
		writeJSON(w, info)
	}))

	handle("POST /api/runs/{id}/input", s.authed(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Prompt string `json:"prompt"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if code, err := s.readOnlyRun(r.PathValue("id")); err != nil {
			writeError(w, code, err.Error())
			return
		}
		info, err := s.runs.Input(r.PathValue("id"), body.Prompt)
		if err != nil {
			writeError(w, runErrCode(err), err.Error())
			return
		}
		writeJSON(w, info)
	}))

	handle("POST /api/runs/{id}/cancel", s.authed(func(w http.ResponseWriter, r *http.Request) {
		if code, err := s.readOnlyRun(r.PathValue("id")); err != nil {
			writeError(w, code, err.Error())
			return
		}
		info, err := s.runs.Cancel(r.PathValue("id"))
		if err != nil {
			writeError(w, runErrCode(err), err.Error())
			return
		}
		writeJSON(w, info)
	}))

	// --- acciones: cada POST exige el token y ejecuta el MISMO codigo que
	// su verbo CLI. Sin token valido no hay efectos secundarios.

	handle("POST /api/verify", s.authed(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Full  bool     `json:"full"`
			Gates []string `json:"gates"`
			Spec  string   `json:"spec"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if !s.verifyMu.TryLock() {
			writeError(w, http.StatusConflict, "ya hay un verify en curso sobre este arbol; espera a que termine")
			return
		}
		defer s.verifyMu.Unlock()
		v, _, err := verifycmd.Run(s.m, verifycmd.Options{Full: body.Full, Gates: body.Gates, Spec: body.Spec})
		if err != nil {
			// El pedido que hoom no entendio es 400, no 500: la misma
			// validacion que el CLI, y sin veredicto ni eventos vivos.
			var ue *cliargs.UsageError
			if errors.As(err, &ue) {
				writeError(w, http.StatusBadRequest, ue.Reason)
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, v)
	}))

	handle("POST /api/tasks", s.authed(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Slug string `json:"slug"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := taskcmd.Start(s.m.Dir, body.Slug, s.m.BaseBranch); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		raw, err := taskcmd.JSONBytes(s.m.Dir, s.m.BaseBranch)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	}))

	handle("POST /api/tasks/{slug}/done", s.authed(func(w http.ResponseWriter, r *http.Request) {
		// El CLI decide; su rechazo viaja VERBATIM y la tarea queda abierta.
		if err := taskcmd.Done(s.m.Dir, r.PathValue("slug"), s.m.BaseBranch, false); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))

	handle("POST /api/intake", s.authed(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
		file, header, err := r.FormFile("file")
		if err != nil {
			writeError(w, http.StatusBadRequest, "falta el archivo (campo multipart 'file'): "+err.Error())
			return
		}
		defer file.Close()
		rel, err := s.saveIntake(header.Filename, file)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, map[string]string{"path": rel})
	}))

	handle("POST /api/specs/{name}/approve", s.authed(func(w http.ResponseWriter, r *http.Request) {
		// Con {"card": "<slug>"} es "Aprobar spec" desde la tarjeta: el mismo
		// verbo, corrido en el arbol de evidencia de la tarjeta. Sin cuerpo (o
		// con {}, que es lo que manda act), lo de siempre.
		var body struct {
			Card *string `json:"card"`
		}
		if _, err := decodeStrict(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if body.Card != nil {
			if strings.TrimSpace(*body.Card) == "" {
				writeError(w, http.StatusBadRequest, "falta la tarjeta (card)")
				return
			}
			s.approveCard(w, r.PathValue("name"), *body.Card)
			return
		}
		path, code, err := s.specPath(r.PathValue("name"))
		if err != nil {
			writeError(w, code, err.Error())
			return
		}
		rec, already, err := approval.Approve(s.m.Dir, path)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]any{"approval": rec, "already": already})
	}))

	handle("POST /api/specs/{name}/review", s.authed(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Comments string `json:"comments"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(body.Comments) == "" {
			writeError(w, http.StatusBadRequest, "comments vacio")
			return
		}
		path, code, err := s.specPath(r.PathValue("name"))
		if err != nil {
			writeError(w, code, err.Error())
			return
		}
		rel, err := appendReview(path, body.Comments)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, map[string]string{"path": rel})
	}))

	return mux, routes
}

// authed gates an action behind the session token: wrong or missing token
// means 401 and ZERO side effects (the wrapped handler never runs). The
// comparison is constant-time so the token cannot be probed byte a byte.
func (s *Server) authed(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get(TokenHeader)
		if len(got) != len(s.token) || subtle.ConstantTimeCompare([]byte(got), []byte(s.token)) != 1 {
			writeError(w, http.StatusUnauthorized, "token invalido o ausente (header "+TokenHeader+"; se imprime al arrancar 'hoom serve')")
			return
		}
		h(w, r)
	}
}

// RunRow is a run as the Studio lists it: the run itself plus what only the
// sidecar knows (the role it embodies, whether it ran blind) and whether it
// belongs to ANOTHER process — a run started by `hoom agent` or `hoom run` in
// a terminal is visible here, and read-only.
type RunRow struct {
	runcmd.Run
	Role     string `json:"role,omitempty"`
	Isolated bool   `json:"isolated,omitempty"`
	Foreign  bool   `json:"foreign,omitempty"`
}

// runRows merges the runs this process owns with the sidecars on disk. In
// memory wins: it is the live truth for its own runs; the sidecar fills in
// everything else, which is how a run of another terminal becomes visible.
func (s *Server) runRows() []RunRow {
	metas := map[string]runcmd.Meta{}
	for _, m := range runcmd.Metas(s.m.Dir) {
		metas[m.ID] = m
	}
	rows := []RunRow{}
	seen := map[string]bool{}
	for _, run := range s.runs.List() {
		row := RunRow{Run: run}
		if meta, ok := metas[run.ID]; ok {
			row.Role, row.Isolated = meta.Role, meta.Isolated
		}
		rows = append(rows, row)
		seen[run.ID] = true
	}
	for _, meta := range runcmd.Metas(s.m.Dir) {
		if seen[meta.ID] {
			continue
		}
		rows = append(rows, rowFromMeta(meta))
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
	return rows
}

// rowFromMeta is how a run of another process is shown: what its sidecar
// proves, and nothing this process could know.
func rowFromMeta(meta runcmd.Meta) RunRow {
	return RunRow{Run: runcmd.Run{
		ID: meta.ID, Provider: meta.Provider, Task: meta.Task, Status: meta.Status,
		ExitCode: meta.ExitCode, CreatedAt: meta.CreatedAt,
		ProviderSessionID: meta.ProviderSessionID, Usage: meta.Usage,
	}, Role: meta.Role, Isolated: meta.Isolated, Foreign: true}
}

// foreignEvents returns the narration of a run another process owns, reading
// only what the log gained since the last call.
func (s *Server) foreignEvents(id string) ([]runcmd.Event, error) {
	s.logMu.Lock()
	defer s.logMu.Unlock()
	c := s.logs[id]
	if c == nil {
		c = &foreignLog{}
		s.logs[id] = c
	}
	evs, next, err := runcmd.ReadEventsFrom(s.m.Dir, id, c.offset)
	if err != nil {
		return nil, err
	}
	c.events = append(c.events, evs...)
	c.offset = next
	out := make([]runcmd.Event, len(c.events))
	copy(out, c.events)
	return out, nil
}

// runEvents returns a run with its full narration, from memory when this
// process owns it and from the jsonl when another one does.
func (s *Server) runEvents(id string) (RunRow, []runcmd.Event, error) {
	if info, evs, err := s.runs.Events(id, 0); err == nil {
		row := RunRow{Run: info}
		for _, meta := range runcmd.Metas(s.m.Dir) {
			if meta.ID == id {
				row.Role, row.Isolated = meta.Role, meta.Isolated
				break
			}
		}
		return row, evs, nil
	}
	meta, ok := runcmd.ReadMeta(s.m.Dir, id)
	if !ok {
		return RunRow{}, nil, fmt.Errorf("run no encontrado: %s", id)
	}
	evs, err := s.foreignEvents(id)
	if err != nil {
		return RunRow{}, nil, err
	}
	row := rowFromMeta(meta)
	row.NumEvents = len(evs)
	return row, evs, nil
}

// readOnlyRun refuses to DRIVE a run that belongs to another process. Seeing
// it is telemetry; continuing or cancelling it is control, and controlling a
// session this process never opened is another feature entirely.
func (s *Server) readOnlyRun(id string) (int, error) {
	if _, err := s.runs.Get(id); err == nil {
		return 0, nil
	}
	for _, row := range s.runRows() {
		if row.ID == id {
			return http.StatusConflict, fmt.Errorf("el run %s lo corre otro proceso (hoom agent / hoom run); desde el Studio es de solo lectura", id)
		}
	}
	return 0, nil // no existe: que conteste el manager, con su 404
}

// runErrCode maps run-domain errors to HTTP: busy trees are 409, unknown
// runs/tasks 404, a provider that cannot continue 400, bad input 400.
func runErrCode(err error) int {
	var busy runcmd.ErrBusy
	if errors.As(err, &busy) {
		return http.StatusConflict
	}
	// la negativa de Input bajo strict es un pedido invalido, no un run
	// perdido: el run existe y esta perfecto, lo que no se puede es continuarlo
	var sinSesion runcmd.ErrNoContinuation
	if errors.As(err, &sinSesion) {
		return http.StatusBadRequest
	}
	msg := err.Error()
	if strings.Contains(msg, "no encontrado") || strings.Contains(msg, "no existe") {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}

// itemStatus maps the board's read of one card to HTTP: a missing or
// invalid item is 404 (CA-317, CA-365), an item file the server could not
// read is 500.
func itemStatus(err error) int {
	var read *fs.PathError
	if errors.As(err, &read) {
		return http.StatusInternalServerError
	}
	return http.StatusNotFound
}

// specPath resolves a spec name to its file, refusing anything that could
// escape .hoom/specs/.
func (s *Server) specPath(name string) (string, int, error) {
	if name == "" || strings.ContainsAny(name, "/\\") || strings.Contains(name, "..") {
		return "", http.StatusBadRequest, fmt.Errorf("nombre de spec invalido %q", name)
	}
	path := filepath.Join(s.m.Dir, ".hoom", "specs", name+".md")
	if _, err := os.Stat(path); err != nil {
		return "", http.StatusNotFound, fmt.Errorf("spec no encontrado: %s", name)
	}
	return path, 0, nil
}

func (s *Server) listSpecs() ([]specItem, error) {
	dir := filepath.Join(s.m.Dir, ".hoom", "specs")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return []specItem{}, nil
	}
	if err != nil {
		return nil, err
	}
	items := []specItem{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") || strings.HasSuffix(e.Name(), ".review.md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		state, rec, err := approval.Status(s.m.Dir, path)
		if err != nil {
			return nil, err
		}
		items = append(items, specItem{
			Name:     strings.TrimSuffix(e.Name(), ".md"),
			Path:     filepath.ToSlash(filepath.Join(".hoom", "specs", e.Name())),
			State:    state,
			Approval: rec,
		})
	}
	return items, nil
}

func (s *Server) specDetail(name string) (*specDetail, int, error) {
	path, code, err := s.specPath(name)
	if err != nil {
		return nil, code, err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	// La lista de criterios sale de la MISMA regex que spec_lint: una sola
	// definicion de "criterio" en todo el sistema.
	ids, _, _, err := spec.Lint(path)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	state, rec, err := approval.Status(s.m.Dir, path)
	if err != nil {
		return nil, http.StatusInternalServerError, err
	}
	d := &specDetail{
		specItem: specItem{
			Name:     name,
			Path:     filepath.ToSlash(filepath.Join(".hoom", "specs", name+".md")),
			State:    state,
			Approval: rec,
		},
		Markdown: string(raw),
		Criteria: ids,
	}
	if review, err := os.ReadFile(reviewPath(path)); err == nil {
		d.Review = string(review)
	}
	return d, 0, nil
}

// saveIntake writes an uploaded client document under .hoom/intake/ and
// NOWHERE else: the name is reduced to a sanitized basename, so no crafted
// filename can escape the directory.
func (s *Server) saveIntake(name string, src io.Reader) (string, error) {
	base := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	base = strings.Trim(base, ". ")
	if base == "" {
		return "", fmt.Errorf("nombre de archivo invalido %q", name)
	}
	dir := filepath.Join(s.m.Dir, ".hoom", "intake")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stamped := time.Now().UTC().Format("2006-01-02") + "_" + base
	dst, err := os.Create(filepath.Join(dir, stamped))
	if err != nil {
		return "", err
	}
	defer dst.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return "", err
	}
	return filepath.ToSlash(filepath.Join(".hoom", "intake", stamped)), nil
}

func reviewPath(specPath string) string {
	return strings.TrimSuffix(specPath, ".md") + ".review.md"
}

// appendReview persists reviewer comments as a versionable markdown sidecar
// next to the spec — the arquitecto reads it from the repo like everything
// else in the harness.
func appendReview(specPath, comments string) (string, error) {
	path := reviewPath(specPath)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	entry := fmt.Sprintf("## Review %s\n\n%s\n\n", time.Now().UTC().Format("2006-01-02 15:04 UTC"), strings.TrimSpace(comments))
	if _, err := f.WriteString(entry); err != nil {
		return "", err
	}
	return filepath.Base(path), nil
}

// Run serves the Studio for the project at dir, blocking until the process
// ends. The action token is printed exactly once here. A non-loopback addr
// prints a loud warning: the token travels in clear over plain HTTP.
func Run(dir, addr string) error {
	s, err := New(dir)
	if err != nil {
		return err
	}
	if host, _, err := net.SplitHostPort(addr); err == nil && !isLoopback(host) {
		fmt.Printf("hoom serve: ADVERTENCIA - %s no es loopback: el Studio queda expuesto y el token viaja en claro (HTTP). Uso previsto: loopback o tunel SSH\n", addr)
	}
	fmt.Printf("hoom serve: HoomAI Studio para %s en http://%s/ (Ctrl-C para salir)\n", s.m.Project, addr)
	fmt.Printf("hoom serve: token de acciones (header %s, se muestra SOLO ahora): %s\n", TokenHeader, s.token)
	if err := http.ListenAndServe(addr, s.Handler()); err != nil {
		return fmt.Errorf("no se pudo escuchar en %s: %w (elegi otro puerto con --addr host:puerto)", addr, err)
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func queryN(r *http.Request, def int) int {
	if raw := r.URL.Query().Get("n"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return def
}

func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("cuerpo JSON invalido: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeRaw(w, raw)
}

func writeRaw(w http.ResponseWriter, raw []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write(raw)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// Routes lists every pattern Handler registers, in registration order: the
// golden rule is asserted over it (no route receives a column).
func Routes() []string {
	_, routes := (&Server{launching: map[string]bool{}}).handler()
	return routes
}

// waitLaunches blocks until every role this Studio launched has finished.
func (s *Server) waitLaunches() { s.launches.Wait() }
