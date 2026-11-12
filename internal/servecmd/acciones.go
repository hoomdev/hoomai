package servecmd

// Las acciones de la tarjeta (spec acciones-desde-la-tarjeta, C3). Cada una
// vuelve a derivar la tarjeta antes de actuar y llama a la MISMA funcion que
// su verbo: hoom agent / hoom review (lanzar), hoom spec approve (aprobar),
// hoom item save (guardar), hoom task discard (descartar) y hoom cockpit
// --task (abrir sesion). Ninguna recibe una columna: la columna la vuelve a
// calcular la evidencia.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cockpitcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// launchResp is the 202 of a launch: the envelope can be named at once.
type launchResp struct {
	EnvelopeID  string `json:"envelope_id"`
	Role        string `json:"role"`
	Provider    string `json:"provider"`
	TaskStarted bool   `json:"task_started"`
}

// decodeStrict reads an optional JSON body refusing unknown keys: an action
// body carries exactly what it says, and a "column" key is a 400. present
// reports whether there was a body at all.
func decodeStrict(r *http.Request, v any) (present bool, err error) {
	if r.Body == nil {
		return false, nil
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return false, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return false, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return true, fmt.Errorf("cuerpo JSON invalido: %w", err)
	}
	// nothing may follow the object, not even a closing bracket: More only
	// looks inside the current value (finding 78f960)
	if _, err := dec.Token(); err != io.EOF {
		return true, fmt.Errorf("cuerpo JSON invalido: sobra contenido despues del objeto")
	}
	return true, nil
}

// cardOf derives the card of the request's slug: 400 with a slug of the wrong
// shape, 404 without a valid item.
func (s *Server) cardOf(w http.ResponseWriter, slug string) (boardcmd.Card, bool) {
	if !item.ValidSlug(slug) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", slug))
		return boardcmd.Card{}, false
	}
	c, err := boardcmd.CardFor(s.m.Dir, s.m.BaseBranch, s.m.FindingsBlockOn(), slug, time.Now().UTC())
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return boardcmd.Card{}, false
	}
	return c, true
}

// actionOf demands the action be on the card, and enabled, right now.
func actionOf(w http.ResponseWriter, c boardcmd.Card, id string) (boardcmd.Action, bool) {
	a, ok := c.Action(id)
	if !ok {
		writeError(w, http.StatusConflict, fmt.Sprintf("la tarjeta ya no esta donde la viste: ahora esta en %s (%s)", c.ColumnName, c.Plain))
		return a, false
	}
	if !a.Enabled {
		writeError(w, http.StatusConflict, a.Why)
		return a, false
	}
	return a, true
}

// evidenceDir is where the card's evidence lives: its task worktree, or the
// project.
func (s *Server) evidenceDir(c boardcmd.Card) string {
	if c.Evidence.Source == boardcmd.SourceWorktree {
		return filepath.Join(s.m.Dir, filepath.FromSlash(c.Evidence.Dir))
	}
	return s.m.Dir
}

// launch is "Pedir al <rol>", "Reanudar" and "Volver a lanzar": the envelope
// (or the review) runs in this process, with the same function as the CLI,
// and the answer is 202 as soon as its first record is on disk.
func (s *Server) launch(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !item.ValidSlug(slug) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", slug))
		return
	}
	var body struct {
		Action    string   `json:"action"`
		Provider  string   `json:"provider"`
		Model     string   `json:"model"`
		BudgetUSD *float64 `json:"budget_usd"`
		Pedido    string   `json:"pedido"`
		Effort    string   `json:"effort"`
		// what the dialog showed of the action: a card that changed while
		// it was open is refused, never launched with other values
		Vista *lanzar.Propuesta `json:"vista"`
	}
	if _, err := decodeStrict(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !boardcmd.IsRoleAction(body.Action) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("accion desconocida %q (validas: %s)", body.Action,
			strings.Join([]string{boardcmd.ActPedirArquitecto, boardcmd.ActPedirTestWriter, boardcmd.ActPedirWriter,
				boardcmd.ActPedirReviewer, boardcmd.ActReanudar, boardcmd.ActRelanzar}, ", ")))
		return
	}
	resp, code, err := s.start(r.Context(), slug, launchReq{Action: body.Action, Provider: body.Provider, Model: body.Model,
		BudgetUSD: body.BudgetUSD, Pedido: body.Pedido, Effort: body.Effort, Vista: body.Vista})
	if err != nil {
		writeError(w, code, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(resp)
}

// launchReq is one launch, from a person (the endpoint) or from the belt.
type launchReq struct {
	Action    string
	Provider  string
	Model     string
	BudgetUSD *float64
	Pedido    string
	Pilot     bool              // the belt launched it: its record says so
	Effort    string            // reasoning effort; every role action takes it
	Vista     *lanzar.Propuesta // what the dialog showed; nil = the belt
}

// start is the Studio's skin of the ONE launch function (lanzar.Lanzar): the
// endpoint and the belt go through the same checks as the card verbs of the
// terminal. Its own in-memory lock goes first (a launch the belt and a tab
// race for), then the card's file lock, which another process sees too. It
// answers when the envelope's first record is on disk (or with the HTTP code
// of why not), or when whoever asked (ctx) is gone: the job then goes on by
// itself, and the belt runs after it.
func (s *Server) start(ctx context.Context, slug string, req launchReq) (launchResp, int, error) {
	t, err := lanzar.Lanzar(ctx, lanzar.Params{
		Root: s.m.Dir, Manifest: s.m, Runs: s.runs, Grupo: &s.launches, ProviderExplicito: true,
		Candado: s.candado,
		Despues: func(antes boardcmd.Card, envelopeID string, log io.Writer) { s.belt(slug, antes, envelopeID, log) },
	}, slug, lanzar.Request{Action: req.Action, Provider: req.Provider, Model: req.Model, Effort: req.Effort,
		BudgetUSD: req.BudgetUSD, Pedido: req.Pedido, Pilot: req.Pilot, Vista: req.Vista})
	if err != nil {
		var r *lanzar.Rechazo
		if errors.As(err, &r) {
			return launchResp{}, r.Code, r
		}
		return launchResp{}, http.StatusInternalServerError, err
	}
	return launchResp{EnvelopeID: t.EnvelopeID, Role: t.Role, Provider: t.Provider, TaskStarted: t.TaskStarted}, 0, nil
}

// candado is the Studio's in-memory launch lock of a card.
func (s *Server) candado(slug string) (func(), bool) {
	s.launchMu.Lock()
	defer s.launchMu.Unlock()
	if s.launching[slug] {
		return nil, false
	}
	s.launching[slug] = true
	return func() {
		s.launchMu.Lock()
		delete(s.launching, slug)
		s.launchMu.Unlock()
	}, true
}

// belt is the autopilot of a card with `auto: hasta-humano`. When a job the
// Studio launched returns, it runs the pilot's turns (pilotocmd.Avanzar,
// over the pure boardcmd.Piloto), which launch through the SAME function
// as the endpoint; what it decided goes to the log of the job that closed.
// A job that never reached its first record, or a card without auto,
// leaves nothing in the log, as before.
func (s *Server) belt(slug string, before boardcmd.Card, envelopeID string, log io.Writer) {
	var closed *envelope.Record
	for _, dir := range []string{s.m.Dir, s.evidenceDir(before)} {
		for _, rec := range envelope.List(dir) {
			if rec.ID == envelopeID {
				r := rec
				closed = &r
			}
		}
		if closed != nil {
			break
		}
	}
	if closed == nil {
		return // no llego a su primer registro: no corrio nada
	}
	after, err := boardcmd.CardFor(s.m.Dir, s.m.BaseBranch, s.m.FindingsBlockOn(), slug, time.Now().UTC())
	if err != nil || after.Item.Auto != item.AutoHastaHumano {
		return
	}
	if _, err := pilotocmd.Avanzar(context.Background(), s.piloto(slug)); err != nil {
		fmt.Fprintf(log, "piloto automatico: se detiene: %s\n", err)
	}
}

// piloto is the Studio's skin of the pilot of a card: its launches go
// through start's same Params, and the job they launch runs the next turn
// when it ends.
func (s *Server) piloto(slug string) pilotocmd.Opciones {
	return pilotocmd.Opciones{Root: s.m.Dir, Manifest: s.m, Slug: slug, Lanzar: lanzar.Params{
		Root: s.m.Dir, Manifest: s.m, Runs: s.runs, Grupo: &s.launches, ProviderExplicito: true,
		Candado: s.candado,
		Despues: func(antes boardcmd.Card, envelopeID string, log io.Writer) { s.belt(slug, antes, envelopeID, log) },
	}}
}

// lineaDeDecision is the line of the pilot's first decision among its
// lines: the launch, the stop, the failed launch or the closing of
// findings — never the rule or the save that come before it.
func lineaDeDecision(lineas []string) string {
	for _, l := range lineas {
		for _, pre := range []string{"piloto automatico: pide al ", "piloto automatico: se detiene: ",
			"piloto automatico: no pudo lanzar al ", "piloto automatico: cierra "} {
			if strings.HasPrefix(l, pre) {
				return l
			}
		}
	}
	return ""
}

// approveCard is "Aprobar spec" from the card: `hoom spec approve` run in
// the card's evidence tree, signed by the git identity of that tree.
//
// autonomia-entre-firmas: with `auto: hasta-humano` on the card, approving
// runs the pilot's turn (pilotocmd.Avanzar), which launches the
// test-writer, and the answer gains `piloto`: the line of that first
// decision. Without auto it launches nothing and the answer is
// `{"approval": ..., "already": ...}` as before.
func (s *Server) approveCard(w http.ResponseWriter, name, slug string) {
	if slug != name {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("la tarjeta %q no es la del spec %q", slug, name))
		return
	}
	c, ok := s.cardOf(w, slug)
	if !ok {
		return
	}
	if _, ok := actionOf(w, c, boardcmd.ActAprobar); !ok {
		return
	}
	dir := s.evidenceDir(c)
	path := filepath.Join(dir, ".hoom", "specs", slug+".md")
	if _, err := os.Stat(path); err != nil {
		writeError(w, http.StatusNotFound, "spec no encontrado: "+slug)
		return
	}
	rec, already, err := approval.Approve(dir, path)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	resp := map[string]any{"approval": rec, "already": already}
	if c.Item.Auto == item.AutoHastaHumano {
		res, err := pilotocmd.Avanzar(context.Background(), s.piloto(slug))
		linea := lineaDeDecision(res.Lineas)
		if err != nil {
			linea = "piloto automatico: se detiene: " + err.Error()
		}
		resp["piloto"] = linea
	}
	writeJSON(w, resp)
}

// pathsBody is the body of save and discard: the paths the person saw.
type pathsBody struct {
	Paths *[]string `json:"paths"`
}

func readPaths(w http.ResponseWriter, r *http.Request) ([]string, bool) {
	var body pathsBody
	if _, err := decodeStrict(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return nil, false
	}
	if body.Paths == nil {
		writeError(w, http.StatusBadRequest, "faltan las rutas que viste (paths)")
		return nil, false
	}
	return append([]string{}, *body.Paths...), true
}

// save is "Guardar en git": `hoom item save <slug>` with the paths the person
// saw.
func (s *Server) save(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !item.ValidSlug(slug) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", slug))
		return
	}
	paths, ok := readPaths(w, r)
	if !ok {
		return
	}
	if _, ok := s.cardOf(w, slug); !ok {
		return
	}
	res, err := itemcmd.Save(s.m.Dir, s.m.BaseBranch, s.m.FindingsBlockOn(), slug, paths)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, res)
}

// discard is "Descartar cambios": `hoom task discard <slug> --yes` with the
// paths the person saw. Evidence is never discarded.
func (s *Server) discard(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !item.ValidSlug(slug) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", slug))
		return
	}
	paths, ok := readPaths(w, r)
	if !ok {
		return
	}
	c, ok := s.cardOf(w, slug)
	if !ok {
		return
	}
	if _, ok := actionOf(w, c, boardcmd.ActDescartar); !ok {
		return
	}
	res, err := taskcmd.Discard(s.m.Dir, slug, paths)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, res)
}

// session is "Abrir sesion": the tmux cockpit of the card's task, without
// attaching; creating it declares the writer in the item.
func (s *Server) session(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if !item.ValidSlug(slug) {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", slug))
		return
	}
	var body struct {
		Provider string `json:"provider"`
	}
	if _, err := decodeStrict(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(body.Provider) == "" {
		writeError(w, http.StatusBadRequest, "falta el provider: desde el Studio la sesion no se autodetecta")
		return
	}
	c, ok := s.cardOf(w, slug)
	if !ok {
		return
	}
	if _, ok := actionOf(w, c, boardcmd.ActSesion); !ok {
		return
	}
	sess, err := cockpitcmd.Open(s.m.Dir, s.m.Project, cockpitcmd.Options{Provider: body.Provider, Task: slug, Mux: "tmux"}, s.cockpit)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeJSON(w, sess)
}

// terminal is "Ver terminal": what tmux already painted in the AI pane of
// the card's session. A read: no token, and it writes nothing.
func (s *Server) terminal(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	if _, ok := s.cardOf(w, slug); !ok {
		return
	}
	// nosemgrep: trailofbits.go.invalid-usage-of-modified-variable.invalid-usage-of-modified-variable -- con error, Capture igual devuelve la sesion que miro, y la respuesta la conserva
	t, err := cockpitcmd.Capture(s.m.Dir, s.m.Project, slug, s.cockpit)
	if err != nil {
		t.Available, t.Text, t.Note = false, "", err.Error()
	}
	writeJSON(w, t)
}
