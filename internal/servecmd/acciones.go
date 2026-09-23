package servecmd

// Las acciones de la tarjeta (spec acciones-desde-la-tarjeta, C3). Cada una
// vuelve a derivar la tarjeta antes de actuar y llama a la MISMA funcion que
// su verbo: hoom agent / hoom review (lanzar), hoom spec approve (aprobar),
// hoom item save (guardar), hoom task discard (descartar) y hoom cockpit
// --task (abrir sesion). Ninguna recibe una columna: la columna la vuelve a
// calcular la evidencia.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cockpitcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
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
	if dec.More() {
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
	resp, code, err := s.start(slug, launchReq{Action: body.Action, Provider: body.Provider, Model: body.Model,
		BudgetUSD: body.BudgetUSD, Pedido: body.Pedido})
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
	Pilot     bool // the belt launched it: its record says so
}

// start is the ONE way the Studio launches a role: the endpoint and the belt
// go through the same checks, in the contract's order. It answers when the
// envelope's first record is on disk (or with the HTTP code of why not).
func (s *Server) start(slug string, req launchReq) (launchResp, int, error) {
	fail := func(code int, format string, args ...any) (launchResp, int, error) {
		return launchResp{}, code, fmt.Errorf(format, args...)
	}
	// Un lanzamiento por tarjeta a la vez. El candado se toma ANTES de derivar
	// la tarjeta y se suelta cuando el sobre ya escribio su registro: el
	// segundo pedido o encuentra el candado, o encuentra la tarjeta en curso.
	s.launchMu.Lock()
	if s.launching[slug] {
		s.launchMu.Unlock()
		return fail(http.StatusConflict, "ya se esta lanzando un trabajo en esta tarjeta")
	}
	s.launching[slug] = true
	s.launchMu.Unlock()
	defer func() {
		s.launchMu.Lock()
		delete(s.launching, slug)
		s.launchMu.Unlock()
	}()
	c, err := boardcmd.CardFor(s.m.Dir, s.m.BaseBranch, s.m.FindingsBlockOn(), slug, time.Now().UTC())
	if err != nil {
		return fail(http.StatusNotFound, "%s", err.Error())
	}
	a, ok := c.Action(req.Action)
	if !ok {
		return fail(http.StatusConflict, "la tarjeta ya no esta donde la viste: ahora esta en %s (%s)", c.ColumnName, c.Plain)
	}
	if !a.Enabled {
		return fail(http.StatusConflict, "%s", a.Why)
	}
	var opt *boardcmd.ProviderOption
	for i := range a.Providers {
		if a.Providers[i].Name == strings.TrimSpace(req.Provider) {
			opt = &a.Providers[i]
		}
	}
	if opt == nil {
		return fail(http.StatusBadRequest, "el proveedor %q no es una opcion de esta accion", req.Provider)
	}
	if !opt.OK {
		return fail(http.StatusConflict, "%s", opt.Why)
	}
	review := a.Role == "reviewer"
	pedido := strings.TrimSpace(req.Pedido)
	switch {
	case review && pedido != "":
		return fail(http.StatusBadRequest, "la revision arma su propio pedido: deja el pedido vacio")
	case !review && pedido == "":
		return fail(http.StatusBadRequest, "falta el pedido: lo que el rol tiene que hacer")
	case !review && agentcmd.IsPlaceholder(pedido):
		return fail(http.StatusBadRequest, "el pedido %q no dice nada: escribi lo que el rol tiene que hacer", req.Pedido)
	}
	budget, code, err := budgetFor(c, a, *opt, req.BudgetUSD)
	if err != nil {
		return launchResp{}, code, err
	}

	// y ningun run activo en su arbol, de ningun proceso: dos roles en el
	// mismo arbol se pisan
	if dir, err := runcmd.TaskDir(s.m.Dir, slug); err == nil {
		if id, busy := s.runs.Busy(dir); busy {
			return fail(http.StatusConflict, "ya hay un run activo (%s) en el espacio de trabajo de la tarjeta: espera a que termine", id)
		}
	}

	started := false
	if c.Evidence.Source != boardcmd.SourceWorktree && c.Column == boardcmd.ColBacklog {
		if err := taskcmd.Start(s.m.Dir, slug, s.m.BaseBranch); err != nil {
			return fail(http.StatusConflict, "%s", err.Error())
		}
		started = true
	}

	id := envelope.NewID()
	logPath := filepath.Join(s.m.Dir, ".hoom", envelope.DirName, id+".log")
	logFile, err := openLog(s.m.Dir, logPath)
	if err != nil {
		return fail(http.StatusInternalServerError, "%s", err.Error())
	}
	early := &tail{}
	out := io.MultiWriter(logFile, early)
	ready := make(chan struct{}, 1)
	onStart := func() { ready <- struct{}{} }
	type fin struct {
		err  error
		line string
	}
	done := make(chan fin, 1)
	spec := ".hoom/specs/" + slug + ".md"

	s.launches.Add(1)
	go func() {
		defer s.launches.Done()
		defer logFile.Close()
		var f fin
		if review {
			_, f.err = reviewcmd.Run(s.m.Dir, s.m.BaseBranch, reviewcmd.Options{
				Provider: opt.Name, Task: slug, Spec: spec, Model: req.Model, BudgetUSD: budget,
				EnvelopeID: id, Started: onStart, Pilot: req.Pilot,
			}, out)
		} else {
			o := agentcmd.Options{
				Role: a.Role, Provider: opt.Name, Task: slug, Prompt: pedido, Model: req.Model,
				BudgetUSD: budget, EnvelopeID: id, Started: onStart, Pilot: req.Pilot,
			}
			if !boardcmd.WritesSpecs(a.Role) {
				o.Spec = spec
			}
			if a.ID == boardcmd.ActReanudar {
				o.ResumeID = a.ResumeID
			}
			_, f.err = agentcmd.Run(s.m.Dir, s.m.BaseBranch, o, out)
		}
		f.line = lastLine(early.String())
		done <- f
		// la cinta: despues de un trabajo que lanzo el Studio, y solo si
		// ese trabajo llego a dejar su registro
		s.belt(slug, c, id, logFile)
	}()

	select {
	case <-ready:
		return launchResp{EnvelopeID: id, Role: a.Role, Provider: opt.Name, TaskStarted: started}, 0, nil
	case f := <-done:
		// termino antes de su primer registro: no corrio nada, y no queda nada
		os.Remove(logPath)
		msg := f.line
		if f.err != nil {
			msg = f.err.Error()
		}
		if msg == "" {
			msg = "el trabajo no arranco"
		}
		return fail(http.StatusConflict, "%s", msg)
	}
}

// belt is the autopilot of a card with `auto: hasta-humano`: a conveyor
// belt, not an orchestrator. When a job the Studio launched returns, it asks
// boardcmd.Pilot (pure) what comes next and, if anything, launches it with
// the SAME function as the endpoint. What it decided goes to the log of the
// job that closed.
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
	d := boardcmd.Pilot(before, after, *closed)
	if !d.Launch {
		fmt.Fprintf(log, "piloto automatico: se detiene: %s\n", d.Why)
		return
	}
	budget := d.BudgetUSD
	resp, _, err := s.start(slug, launchReq{Action: d.Action, Provider: d.Provider, BudgetUSD: &budget,
		Pedido: d.Pedido, Pilot: true})
	if err != nil {
		fmt.Fprintf(log, "piloto automatico: no pudo lanzar al %s: %s\n", d.Role, err)
		return
	}
	fmt.Fprintf(log, "piloto automatico: pide al %s con %s (%s USD): sobre %s\n", d.Role, d.Provider, usd(budget), resp.EnvelopeID)
}

// budgetFor resolves the budget of a launch against the provider and what
// is left of the card's. 0 = no cap.
func budgetFor(c boardcmd.Card, a boardcmd.Action, o boardcmd.ProviderOption, asked *float64) (float64, int, error) {
	if !o.Budget {
		if asked != nil {
			return 0, http.StatusBadRequest, fmt.Errorf("%s no acepta tope de presupuesto: deja el presupuesto vacio", o.Name)
		}
		return 0, 0, nil
	}
	if asked == nil {
		if a.BudgetUSD == nil {
			return 0, 0, nil
		}
		return *a.BudgetUSD, 0, nil
	}
	v := *asked
	if math.IsNaN(v) || math.IsInf(v, 0) || !(v > 0) {
		return 0, http.StatusBadRequest, fmt.Errorf("el presupuesto tiene que ser un numero mayor que 0 (vacio = %s)", sinTope(a))
	}
	if v < o.MinBudgetUSD {
		return 0, http.StatusConflict, fmt.Errorf("%s necesita al menos %s USD por run", o.Name, usd(o.MinBudgetUSD))
	}
	if left := c.Spend.RemainingUSD; left != nil {
		if a.Role == "reviewer" && v*boardcmd.ReviewLenses > *left {
			return 0, http.StatusConflict, fmt.Errorf("la revision puede usar %d lentes y %d veces %s USD supera lo que queda de la tarjeta (%s USD)",
				boardcmd.ReviewLenses, boardcmd.ReviewLenses, usd(v), usd(*left))
		}
		if v > *left {
			return 0, http.StatusConflict, fmt.Errorf("el presupuesto pedido (%s USD) supera lo que queda de la tarjeta (%s USD)", usd(v), usd(*left))
		}
	}
	return v, 0, nil
}

func sinTope(a boardcmd.Action) string {
	if a.BudgetUSD == nil {
		return "sin tope"
	}
	return "lo que queda de la tarjeta"
}

func usd(v float64) string {
	s := fmt.Sprintf("%.4f", v)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// openLog creates the envelope's text output next to its record, in the
// directory hoom keeps out of git.
func openLog(root, path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	hoomfs.EnsureIgnored(root, envelope.DirName)
	return os.Create(path)
}

// tail keeps only the end of what the envelope says: its last line is the
// answer when it stops before its first record.
type tail struct{ b []byte }

func (t *tail) Write(p []byte) (int, error) {
	t.b = append(t.b, p...)
	if n := len(t.b); n > 8<<10 {
		t.b = append([]byte{}, t.b[n-(4<<10):]...)
	}
	return len(p), nil
}

func (t *tail) String() string { return string(t.b) }

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// approveCard is "Aprobar spec" from the card: `hoom spec approve` run in
// the card's evidence tree, signed by the git identity of that tree.
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
	writeJSON(w, map[string]any{"approval": rec, "already": already})
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
	t, err := cockpitcmd.Capture(s.m.Dir, s.m.Project, slug, s.cockpit)
	if err != nil {
		t.Available, t.Text, t.Note = false, "", err.Error()
	}
	writeJSON(w, t)
}
