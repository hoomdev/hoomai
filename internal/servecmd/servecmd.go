// Package servecmd implements `hoom serve`: the HoomAI Studio v1, a
// read-only dashboard embedded in the hoom binary itself. The Studio is a
// mirror of the harness, never a second brain: every endpoint reuses the
// exact same internal functions (and byte-identical JSON) as the CLI verbs
// it maps to. Logic the CLI does not have does not exist here.
package servecmd

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"strconv"

	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/report"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// DefaultAddr is loopback-only by construction: exposing the Studio to a
// network is a conscious decision via --addr, never the default.
const DefaultAddr = "127.0.0.1:4666"

// Listing caps: how many items an unqualified request returns.
const (
	defaultVerdictsN = 50
	defaultReportN   = 10
)

//go:embed ui
var uiFS embed.FS

// Server serves the Studio for one project.
type Server struct {
	m *manifest.Manifest
}

// New loads the project's manifest. Outside a hoom project it fails with
// the exact action to take (hoom init), which the CLI turns into exit 1.
func New(dir string) (*Server, error) {
	m, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		return nil, err
	}
	return &Server{m: m}, nil
}

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

// Handler returns the Studio's HTTP handler: the embedded UI at / and the
// read-only JSON API under /api/.
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

	return mux
}

// Run serves the Studio for the project at dir, blocking until the process
// ends. A non-loopback addr prints a loud warning: v1 has no auth.
func Run(dir, addr string) error {
	s, err := New(dir)
	if err != nil {
		return err
	}
	if host, _, err := net.SplitHostPort(addr); err == nil && !isLoopback(host) {
		fmt.Printf("hoom serve: ADVERTENCIA - %s no es loopback: el Studio queda expuesto SIN autenticacion (v1 es solo lectura)\n", addr)
	}
	fmt.Printf("hoom serve: HoomAI Studio para %s en http://%s/ (solo lectura; Ctrl-C para salir)\n", s.m.Project, addr)
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
