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
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/contextcmd"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/report"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/spec"
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
	return &Server{m: m, token: hex.EncodeToString(raw), runs: runcmd.NewManager(m.Dir)}, nil
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
	mux := http.NewServeMux()

	ui, _ := fs.Sub(uiFS, "ui")
	mux.Handle("GET /", http.FileServerFS(ui))

	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		check, err := checkcmd.Run(s.m.Dir, s.m.BaseBranch)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, statusResp{
			Project:    s.m.Project,
			Profile:    s.m.Profile,
			Policy:     s.m.Policy,
			BaseBranch: s.m.BaseBranch,
			Check:      check,
		})
	})

	mux.HandleFunc("GET /api/verdicts", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("GET /api/verdicts/{id}", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		raw, err := taskcmd.JSONBytes(s.m.Dir, s.m.BaseBranch)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	mux.HandleFunc("GET /api/report", func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("GET /api/context", func(w http.ResponseWriter, r *http.Request) {
		raw, err := contextcmd.JSONBytes(s.m.Dir)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	mux.HandleFunc("GET /api/specs", func(w http.ResponseWriter, r *http.Request) {
		items, err := s.listSpecs()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, items)
	})

	mux.HandleFunc("GET /api/specs/{name}", func(w http.ResponseWriter, r *http.Request) {
		d, code, err := s.specDetail(r.PathValue("name"))
		if err != nil {
			writeError(w, code, err.Error())
			return
		}
		writeJSON(w, d)
	})

	// --- cockpit: providers y runs headless. Los eventos de un run son
	// NARRACION local (.hoom/runs/, fuera de huella y de Git); la evidencia
	// sigue siendo el veredicto.

	mux.HandleFunc("GET /api/providers", func(w http.ResponseWriter, r *http.Request) {
		raw, err := providers.JSONBytes()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeRaw(w, raw)
	})

	mux.HandleFunc("GET /api/runs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.runs.List())
	})

	mux.HandleFunc("GET /api/runs/{id}", func(w http.ResponseWriter, r *http.Request) {
		after := 0
		if raw := r.URL.Query().Get("after"); raw != "" {
			if n, err := strconv.Atoi(raw); err == nil && n > 0 {
				after = n
			}
		}
		info, evs, err := s.runs.Events(r.PathValue("id"), after)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, map[string]any{"run": info, "events": evs, "next": after + len(evs)})
	})

	mux.HandleFunc("GET /api/runs/{id}/stage", func(w http.ResponseWriter, r *http.Request) {
		info, evs, err := s.runs.Events(r.PathValue("id"), 0)
		if err != nil {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, runcmd.Stage(info, evs))
	})

	mux.HandleFunc("POST /api/runs", s.authed(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Provider string `json:"provider"`
			Prompt   string `json:"prompt"`
			Task     string `json:"task"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		info, err := s.runs.Start(body.Provider, body.Prompt, body.Task)
		if err != nil {
			writeError(w, runErrCode(err), err.Error())
			return
		}
		writeJSON(w, info)
	}))

	mux.HandleFunc("POST /api/runs/{id}/input", s.authed(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Prompt string `json:"prompt"`
		}
		if err := decodeJSON(r, &body); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		info, err := s.runs.Input(r.PathValue("id"), body.Prompt)
		if err != nil {
			writeError(w, runErrCode(err), err.Error())
			return
		}
		writeJSON(w, info)
	}))

	mux.HandleFunc("POST /api/runs/{id}/cancel", s.authed(func(w http.ResponseWriter, r *http.Request) {
		info, err := s.runs.Cancel(r.PathValue("id"))
		if err != nil {
			writeError(w, runErrCode(err), err.Error())
			return
		}
		writeJSON(w, info)
	}))

	// --- acciones: cada POST exige el token y ejecuta el MISMO codigo que
	// su verbo CLI. Sin token valido no hay efectos secundarios.

	mux.HandleFunc("POST /api/verify", s.authed(func(w http.ResponseWriter, r *http.Request) {
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
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, v)
	}))

	mux.HandleFunc("POST /api/tasks", s.authed(func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /api/tasks/{slug}/done", s.authed(func(w http.ResponseWriter, r *http.Request) {
		// El CLI decide; su rechazo viaja VERBATIM y la tarea queda abierta.
		if err := taskcmd.Done(s.m.Dir, r.PathValue("slug"), s.m.BaseBranch, false); err != nil {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("POST /api/intake", s.authed(func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /api/specs/{name}/approve", s.authed(func(w http.ResponseWriter, r *http.Request) {
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

	mux.HandleFunc("POST /api/specs/{name}/review", s.authed(func(w http.ResponseWriter, r *http.Request) {
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

	return mux
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

// runErrCode maps run-domain errors to HTTP: busy trees are 409, unknown
// runs/tasks 404, bad input 400.
func runErrCode(err error) int {
	var busy runcmd.ErrBusy
	if errors.As(err, &busy) {
		return http.StatusConflict
	}
	msg := err.Error()
	if strings.Contains(msg, "no encontrado") || strings.Contains(msg, "no existe") {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
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
	ids, _, err := spec.Lint(path)
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
