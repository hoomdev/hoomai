// Package lanzar is the ONE way a role is launched on a card, whoever asks:
// the Studio's POST /launch, its belt and the card verbs of the terminal
// (spec verbos-de-la-tarjeta). It takes the card's launch lock, derives the
// card, checks the action and its values in the contract's order, writes the
// envelope's log and creates the task of a card in Backlog. The skins differ
// only in Params: the Studio returns at the first record and runs its belt
// afterwards; the terminal waits for the end and copies the narration to its
// standard output.
package lanzar

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/valores"
)

// MsgLanzando is what the second launch on the same card gets while the
// first one is being set up, from any process.
const MsgLanzando = "ya se esta lanzando un trabajo en esta tarjeta"

// Request is one launch of a role action.
type Request struct {
	Action    string        // pedir-<rol>, reanudar or relanzar
	Provider  string        // "" = the action's default provider
	Model     string        // "" = the action's model (hoom.yaml) or the provider's
	Effort    string        // "" = the action's effort (hoom.yaml) or the provider's
	BudgetUSD *float64      // nil = the action's proposed budget
	Timeout   time.Duration // 0 = the action's timeout (timeout_s)
	Pedido    string        // what the role has to do; the reviewer builds its own
	Pilot     bool          // the belt launched it: its record says so
	// Vista is what the action proposed when the skin showed it (the
	// terminal's valores: and → lines, the Studio's dialog). nil = nothing
	// was shown (the belt).
	Vista *Propuesta

	// autonomia-entre-firmas: only the pilot sets these; the body of POST
	// /launch does not change.
	//
	// Resume is the provider's session id the role's run resumes (the
	// pilot's relevo-por-tiempo); "" = a new session. Delta asks the review
	// to start where the task's newest chainable review ended (reviewcmd
	// Options.Delta); only for the reviewer.
	Resume string
	Delta  bool
	// Regla, Intento, Ronda, Hallazgos and Bloqueantes go to the launched
	// envelope's record (envelope.Record) as given.
	Regla       string
	Intento     int
	Ronda       int
	Hallazgos   []string
	Bloqueantes int
}

// Propuesta is what a card's action proposes: the role it launches, its
// default provider, its values and the session it resumes. A skin shows it and then waits for the
// pedido (the terminal's editor, the Studio's dialog); Lanzar derives the
// card again under the lock and refuses when the action proposes something
// else by then, so what launches is never other than what was shown.
type Propuesta struct {
	Role      string   `json:"role"`
	Provider  string   `json:"provider"`
	Model     string   `json:"model"`
	Effort    string   `json:"effort"`
	BudgetUSD *float64 `json:"budget_usd"`
	TimeoutS  int      `json:"timeout_s"`
	ResumeID  string   `json:"resume_id"`
}

// PropuestaDe is what action a proposes now.
func PropuestaDe(a boardcmd.Action) Propuesta {
	p := Propuesta{Role: a.Role, Model: a.Model, Effort: a.Effort, TimeoutS: a.TimeoutS, ResumeID: a.ResumeID}
	if o, ok := defaultOption(a); ok {
		p.Provider = o.Name
	}
	if a.BudgetUSD != nil {
		v := *a.BudgetUSD
		p.BudgetUSD = &v
	}
	return p
}

// cambios names what differs between two proposals: the role first (a
// reanudar or relanzar takes it from the interruption, which can be another
// one by now: finding a8e103), then in the order of the valores: line.
func cambios(antes, ahora Propuesta) []string {
	var out []string
	if antes.Role != ahora.Role {
		out = append(out, "rol")
	}
	if antes.Provider != ahora.Provider {
		out = append(out, "provider")
	}
	if antes.Model != ahora.Model {
		out = append(out, "modelo")
	}
	if antes.Effort != ahora.Effort {
		out = append(out, "esfuerzo")
	}
	if (antes.BudgetUSD == nil) != (ahora.BudgetUSD == nil) || antes.BudgetUSD != nil && *antes.BudgetUSD != *ahora.BudgetUSD {
		out = append(out, "presupuesto")
	}
	if antes.TimeoutS != ahora.TimeoutS {
		out = append(out, "tiempo limite")
	}
	if antes.ResumeID != ahora.ResumeID {
		out = append(out, "sesion")
	}
	return out
}

// Params are the skin of a launch.
type Params struct {
	Root     string             // the project: the main tree, the one the Studio serves
	Manifest *manifest.Manifest // the project's hoom.yaml
	// Runs is the Studio's own manager: a run it is starting counts as
	// active before its sidecar says so. nil = only the sidecars.
	Runs *runcmd.Manager
	// Eco also gets the job's narration (the terminal's stdout); the log of
	// the envelope always gets it. nil = only the log.
	Eco io.Writer
	// Candado is an extra in-process lock taken before the file lock (the
	// Studio's); nil = only the file lock. ok=false means taken.
	Candado func(slug string) (soltar func(), ok bool)
	// Despues runs in the job's goroutine once the job ended (the Studio's
	// belt), with the card as it was before the launch, the envelope's id and
	// the envelope's log. nil = nothing.
	Despues func(antes boardcmd.Card, envelopeID string, log io.Writer)
	// Grupo, when given, counts the job's goroutine (the Studio waits for its
	// launches with it).
	Grupo *sync.WaitGroup
	// ProviderExplicito: the skin always names the provider (the Studio's
	// dialog), so an empty one is not an option of the action; false = ""
	// takes the action's default (the terminal).
	ProviderExplicito bool
}

// Rechazo is why a launch did not happen: the same motive for every skin, and
// the HTTP code the Studio answers with (the terminal exits with 1).
type Rechazo struct {
	Code   int
	Motivo string
}

func (r *Rechazo) Error() string { return r.Motivo }

// Trabajo is a launched job: its envelope already wrote its first record (or
// the job already ended and said why it did not run).
type Trabajo struct {
	EnvelopeID  string
	Role        string
	Provider    string
	TaskStarted bool
	Valores     valores.Elegidos // the values it was launched with
	fin         chan Fin
	once        sync.Once
	final       Fin
}

// Fin is how a job ended.
type Fin struct {
	Agent   *agentcmd.Result  // the envelope's result (roles of the envelope)
	Review  *reviewcmd.Result // the review's result (the reviewer)
	Err     error             // a broken setup
	Nota    string            // the note of a review decided without passes
	Arranco bool              // it reached its first record
}

// Wait blocks until the job ends. It can be called more than once.
func (t *Trabajo) Wait() Fin {
	t.once.Do(func() { t.final = <-t.fin })
	return t.final
}

// Candado is the path of the card's launch lock: a file lock in
// .hoom/cache/locks/ that the Studio and the terminal take.
func Candado(root, slug string) string {
	return hoomfs.LockPath(root, "lanzar-"+slug)
}

// Valores are the values a launch of action a takes for req: what req asks
// over what the action proposes (which already applied hoom.yaml), each with
// its origin, by the one rule (valores.Sobre). The card verbs print them.
func Valores(a boardcmd.Action, req Request) valores.Elegidos {
	p := valores.Pedido{Provider: strings.TrimSpace(req.Provider), Model: strings.TrimSpace(req.Model),
		Effort: strings.TrimSpace(req.Effort), BudgetUSD: req.BudgetUSD}
	if req.Timeout > 0 {
		p.Timeout = req.Timeout
	}
	return valores.Sobre(p, elegidosDe(a))
}

// elegidosDe is what action a proposes, as values with their origins.
func elegidosDe(a boardcmd.Action) valores.Elegidos {
	e := valores.Elegidos{Model: a.Model, Effort: a.Effort, Origen: a.Origen}
	if o, ok := defaultOption(a); ok {
		e.Provider = o.Name
	}
	if a.BudgetUSD != nil {
		v := *a.BudgetUSD
		e.BudgetUSD = &v
	}
	e.Timeout = time.Duration(a.TimeoutS) * time.Second
	if e.Timeout <= 0 {
		e.Timeout, e.Origen.Timeout = valores.DefaultTimeout, valores.OrigenProvider
	}
	// an action derived elsewhere (no origins): what it carries is the card's
	fill := func(origen *string, tiene bool) {
		if *origen == "" {
			*origen = valores.OrigenProvider
			if tiene {
				*origen = valores.OrigenTarjeta
			}
		}
	}
	fill(&e.Origen.Provider, e.Provider != "")
	fill(&e.Origen.Model, e.Model != "")
	fill(&e.Origen.Effort, e.Effort != "")
	fill(&e.Origen.BudgetUSD, e.BudgetUSD != nil)
	fill(&e.Origen.Timeout, false)
	return e
}

// defaultOption is the provider the action proposes.
func defaultOption(a boardcmd.Action) (boardcmd.ProviderOption, bool) {
	for _, o := range a.Providers {
		if o.Default {
			return o, true
		}
	}
	return boardcmd.ProviderOption{}, false
}

// optionOf is the action's option for the provider name.
func optionOf(a boardcmd.Action, name string) (boardcmd.ProviderOption, bool) {
	for _, o := range a.Providers {
		if o.Name == name {
			return o, true
		}
	}
	return boardcmd.ProviderOption{}, false
}

// estadoLanzamiento is one launch as Lanzar sees it, apart from the Trabajo
// handle it hands back (findings a46d0c, a0c783). Until it
// is enMarcha the card's launch lock belongs to Lanzar, which releases it on
// any early exit; from then on the job releases it, once, when its envelope
// writes the first record (empezo) or when it ends without writing it
// (termino). listo and fin say which of the two happened; fin also says
// whether the first record came before the end.
type estadoLanzamiento struct {
	soltar   func() // releases the card's launch locks
	soltado  sync.Once
	enMarcha bool
	arranco  atomic.Bool
	listo    chan struct{}
	fin      chan Fin
}

func (t *estadoLanzamiento) soltarCandado() { t.soltado.Do(t.soltar) }

// empezo is the envelope's Started: its first record is on disk.
func (t *estadoLanzamiento) empezo() {
	t.arranco.Store(true)
	t.soltarCandado()
	t.listo <- struct{}{}
}

// termino closes the job with how it ended.
func (t *estadoLanzamiento) termino(f Fin) {
	f.Arranco = t.arranco.Load()
	t.soltarCandado()
	t.fin <- f
}

// tomarCandados takes the skin's lock and then the card's file lock: the
// second launch of the card, from this process or another, is refused with
// the same motive.
func tomarCandados(p Params, slug string) (func(), error) {
	soltarMem := func() {}
	if p.Candado != nil {
		s, ok := p.Candado(slug)
		if !ok {
			return nil, &Rechazo{Code: http.StatusConflict, Motivo: MsgLanzando}
		}
		soltarMem = s
	}
	soltarArchivo, err := hoomfs.Lock(Candado(p.Root, slug), 0)
	if err != nil {
		soltarMem()
		if errors.Is(err, hoomfs.ErrLocked) {
			return nil, &Rechazo{Code: http.StatusConflict, Motivo: MsgLanzando}
		}
		return nil, &Rechazo{Code: http.StatusInternalServerError, Motivo: "no pude tomar el candado de la tarjeta: " + err.Error()}
	}
	return func() {
		soltarArchivo()
		soltarMem()
	}, nil
}

// Lanzar launches req on the card slug. It answers when the envelope's first
// record is on disk, with a *Rechazo when the launch did not happen, or when
// ctx is done (the job then goes on by itself). A job that ended before its
// first record comes back too, already ended, next to its *Rechazo.
//
// autonomia-entre-firmas: every envelope Lanzar launches has in its record
// the card's column at the launch (envelope.Record.Columna) and the pedido
// the role received (Record.Pedido), plus req's Regla, Intento, Ronda,
// Hallazgos and Bloqueantes. A req with Pilot may name an action the card
// does not offer a person (boardcmd.ActPedirRefutador, or a role action
// outside the card's column inside a round of findings): Lanzar takes it
// from boardcmd.Card.PilotAction, with the same checks as any other. A
// provider without a USD cap launches without a budget, as today.
func Lanzar(ctx context.Context, p Params, slug string, req Request) (*Trabajo, error) {
	fail := func(code int, format string, args ...any) (*Trabajo, error) {
		return nil, &Rechazo{Code: code, Motivo: fmt.Sprintf(format, args...)}
	}
	m := p.Manifest
	if m == nil {
		m = &manifest.Manifest{Dir: p.Root}
	}
	// Un lanzamiento por tarjeta a la vez, de cualquier proceso. El candado
	// se toma ANTES de derivar la tarjeta y lo suelta el trabajo cuando su
	// sobre ya escribio el registro, o cuando termina sin escribirlo: el
	// segundo pedido o encuentra el candado, o encuentra la tarjeta en curso.
	// No lo suelta quien pidio: el pedido se puede cancelar antes (hallazgo
	// 0cf41f).
	soltar, err := tomarCandados(p, slug)
	if err != nil {
		return nil, err
	}
	t := &estadoLanzamiento{soltar: soltar, listo: make(chan struct{}, 1), fin: make(chan Fin, 1)}
	// hasta que el trabajo arranca, el candado es de Lanzar: cualquier
	// salida antes lo suelta
	defer func() {
		if !t.enMarcha {
			t.soltarCandado()
		}
	}()
	c, err := boardcmd.CardFor(p.Root, m.BaseBranch, m.FindingsBlockOn(), slug, time.Now().UTC())
	if err != nil {
		return fail(http.StatusNotFound, "%s", err.Error())
	}
	a, ok := c.Action(req.Action)
	if !ok && req.Pilot {
		// the actions only the pilot launches (autonomia-entre-firmas)
		a, ok = c.PilotAction(req.Action)
	}
	if !ok {
		return fail(http.StatusConflict, "la tarjeta ya no esta donde la viste: ahora esta en %s (%s)", c.ColumnName, c.Plain)
	}
	if !a.Enabled {
		return fail(http.StatusConflict, "%s", a.Why)
	}
	if p.ProviderExplicito && strings.TrimSpace(req.Provider) == "" {
		return fail(http.StatusBadRequest, "el proveedor %q no es una opcion de esta accion", "")
	}
	if req.Vista != nil {
		if dif := cambios(*req.Vista, PropuestaDe(a)); len(dif) > 0 {
			return fail(http.StatusConflict, "la tarjeta cambio mientras escribias el pedido (%s): no se lanzo nada, vuelve a pedirlo", strings.Join(dif, ", "))
		}
	}
	// in the contract's order: the provider, the pedido, the budget
	v, opt, err := opcion(a, req)
	if err != nil {
		return nil, err
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
	budget, code, err := budgetFor(c, a, opt, req.BudgetUSD)
	if err != nil {
		return nil, &Rechazo{Code: code, Motivo: err.Error()}
	}

	// y ningun run activo en su arbol, de ningun proceso: dos roles en el
	// mismo arbol se pisan
	if dir, err := runcmd.TaskDir(p.Root, slug); err == nil {
		id, busy := runcmd.Active(p.Root, dir)
		if p.Runs != nil {
			id, busy = p.Runs.Busy(dir)
		}
		if busy {
			return fail(http.StatusConflict, "ya hay un run activo (%s) en el espacio de trabajo de la tarjeta: espera a que termine", id)
		}
	}

	// the log first: a launch that cannot write it fails before creating
	// anything (finding dda1cc)
	id := envelope.NewID()
	logPath := filepath.Join(p.Root, ".hoom", envelope.DirName, id+".log")
	logFile, err := openLog(p.Root, logPath)
	if err != nil {
		return fail(http.StatusInternalServerError, "%s", err.Error())
	}
	out := io.Writer(logFile)
	if p.Eco != nil {
		out = io.MultiWriter(logFile, p.Eco)
	}
	// a card in Backlog without a workspace gets its task first: inside the
	// job, so a request canceled meanwhile does not wait for git (finding
	// 0cf41f)
	crearTarea := c.Evidence.Source != boardcmd.SourceWorktree && c.Column == boardcmd.ColBacklog
	var tareaCreada atomic.Bool

	spec := ".hoom/specs/" + slug + ".md"

	t.enMarcha = true
	if p.Grupo != nil {
		p.Grupo.Add(1)
	}
	go func() {
		if p.Grupo != nil {
			defer p.Grupo.Done()
		}
		defer logFile.Close()
		var f Fin
		if crearTarea {
			if f.Err = taskcmd.Start(p.Root, slug, m.BaseBranch); f.Err == nil {
				tareaCreada.Store(true)
			}
		}
		switch {
		case f.Err != nil:
			// la tarea no se pudo crear: no corre nada, y el 409 lo dice
		case review:
			var res reviewcmd.Result
			res, f.Err = reviewcmd.Run(p.Root, m.BaseBranch, reviewcmd.Options{
				Provider: opt.Name, Task: slug, Spec: spec, Model: v.Model, Effort: v.Effort, BudgetUSD: budget,
				Timeout: v.Timeout, EnvelopeID: id, Started: t.empezo, Pilot: req.Pilot,
				Delta: req.Delta, Columna: c.Column, Regla: req.Regla, Intento: req.Intento, Ronda: req.Ronda,
				Hallazgos: req.Hallazgos, Bloqueantes: req.Bloqueantes,
			}, out)
			f.Nota = res.Nota
			if f.Err == nil {
				f.Review = &res
			}
		default:
			o := agentcmd.Options{
				Role: a.Role, Provider: opt.Name, Task: slug, Prompt: pedido, Model: v.Model,
				Effort: v.Effort, Timeout: v.Timeout, BudgetUSD: budget, BudgetSet: true,
				EnvelopeID: id, Started: t.empezo, Pilot: req.Pilot,
				Columna: c.Column, Regla: req.Regla, Intento: req.Intento, Ronda: req.Ronda,
				Hallazgos: req.Hallazgos, Bloqueantes: req.Bloqueantes,
			}
			if !boardcmd.WritesSpecs(a.Role) {
				o.Spec = spec
			}
			if a.ID == boardcmd.ActReanudar {
				o.ResumeID = a.ResumeID
			}
			if r := strings.TrimSpace(req.Resume); r != "" {
				o.ResumeID = r
			}
			var res agentcmd.Result
			res, f.Err = agentcmd.Run(p.Root, m.BaseBranch, o, out)
			if f.Err == nil {
				f.Agent = &res
			}
		}
		if !t.arranco.Load() {
			// termino antes de su primer registro: no corrio nada, y no
			// queda nada (tampoco su log)
			logFile.Close()
			os.Remove(logPath)
		}
		t.termino(f)
		if p.Despues != nil {
			// la cinta: despues del trabajo, y ella decide si llego a dejar
			// su registro
			p.Despues(c, id, logFile)
		}
	}()

	listo := func(fin *Fin) (*Trabajo, error) {
		tr := &Trabajo{EnvelopeID: id, Role: a.Role, Provider: opt.Name, TaskStarted: tareaCreada.Load(),
			Valores: v, fin: t.fin}
		tr.Valores.Provider, tr.Valores.BudgetUSD = opt.Name, nil
		if budget > 0 {
			b := budget
			tr.Valores.BudgetUSD = &b
		}
		if fin != nil {
			tr.once.Do(func() { tr.final = *fin })
		}
		return tr, nil
	}
	select {
	case <-t.listo:
		return listo(nil)
	case f := <-t.fin:
		if f.Arranco {
			// llego a su primer registro y termino antes de esta respuesta:
			// corrio, y su registro dice como (hallazgo 17225a)
			return listo(&f)
		}
		// el error, o la nota de una review que se decidio sin pasadas: lo
		// que el resultado dice, no la ultima linea de su texto (hallazgo
		// fdccff)
		msg := f.Nota
		if f.Err != nil {
			msg = f.Err.Error()
		}
		if msg == "" {
			msg = "el trabajo no arranco"
		}
		// the job that ended goes with the refusal: a review decided without
		// passes has a result the terminal answers with, as hoom review does
		tr, _ := listo(&f)
		return tr, &Rechazo{Code: http.StatusConflict, Motivo: msg}
	case <-ctx.Done():
		// quien pidio ya no espera: el trabajo sigue (o termina) solo, y la
		// tarjeta lo muestra (hallazgo 0cf41f)
		return fail(http.StatusRequestTimeout, "el pedido se cancelo antes de que el trabajo arrancara; la tarjeta muestra como sigue")
	}
}

// Comprobar says whether req's values (provider and budget) can launch
// action a of card c, with the motive Lanzar would give: a skin that asks
// for the pedido afterwards (the terminal's editor) asks only when they can.
func Comprobar(c boardcmd.Card, a boardcmd.Action, req Request) error {
	_, opt, err := opcion(a, req)
	if err != nil {
		return err
	}
	if _, code, err := budgetFor(c, a, opt, req.BudgetUSD); err != nil {
		return &Rechazo{Code: code, Motivo: err.Error()}
	}
	return nil
}

// opcion resolves req's values and the provider option they name, which
// has to be one with ok.
func opcion(a boardcmd.Action, req Request) (valores.Elegidos, boardcmd.ProviderOption, error) {
	v := Valores(a, req)
	opt, ok := optionOf(a, v.Provider)
	if !ok {
		return v, opt, &Rechazo{Code: http.StatusBadRequest, Motivo: fmt.Sprintf("el proveedor %q no es una opcion de esta accion", v.Provider)}
	}
	if !opt.OK {
		return v, opt, &Rechazo{Code: http.StatusConflict, Motivo: opt.Why}
	}
	return v, opt, nil
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
		// hoom.yaml's budget_usd lands on the action after its options were
		// judged: the provider's floor holds for it as for an asked one
		if v := *a.BudgetUSD; v > 0 && v < o.MinBudgetUSD {
			return 0, http.StatusConflict, pocoParaElProvider(o)
		}
		return *a.BudgetUSD, 0, nil
	}
	v := *asked
	if math.IsNaN(v) || math.IsInf(v, 0) || !(v > 0) {
		return 0, http.StatusBadRequest, fmt.Errorf("el presupuesto tiene que ser un numero mayor que 0 (vacio = %s)", sinTope(a))
	}
	if v < o.MinBudgetUSD {
		return 0, http.StatusConflict, pocoParaElProvider(o)
	}
	if left := c.Spend.RemainingUSD; left != nil {
		if a.Role == "reviewer" && v*boardcmd.ReviewLenses > *left {
			return 0, http.StatusConflict, fmt.Errorf("la revision puede usar %d lentes y %d veces %s USD supera lo que queda de la tarjeta (%s USD)",
				boardcmd.ReviewLenses, boardcmd.ReviewLenses, USD(v), USD(*left))
		}
		if v > *left {
			return 0, http.StatusConflict, fmt.Errorf("el presupuesto pedido (%s USD) supera lo que queda de la tarjeta (%s USD)", USD(v), USD(*left))
		}
	}
	return v, 0, nil
}

func pocoParaElProvider(o boardcmd.ProviderOption) error {
	return fmt.Errorf("%s necesita al menos %s USD por run", o.Name, USD(o.MinBudgetUSD))
}

func sinTope(a boardcmd.Action) string {
	if a.BudgetUSD == nil {
		return "sin tope"
	}
	return "lo que queda de la tarjeta"
}

// USD renders an amount the way the cards say it: up to 4 decimals, no
// trailing zeros.
func USD(v float64) string {
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
