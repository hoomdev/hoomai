package pilotocmd

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

// UsageText is the usage block of `hoom piloto`.
const UsageText = `Uso: hoom piloto <slug> [--json]

  Corre el piloto automatico de la tarjeta en primer plano hasta que se
  detiene: guarda, decide y lanza (o cierra hallazgos con su test), una linea
  por decision. Sale con 0 si se detuvo en una columna tuya o en Hecho, y con
  1 en cualquier otra detencion. Nunca aprueba ni integra.
  --json   Emite el resultado como JSON en stdout`

// Candado is the path of the lock `hoom piloto` holds while it runs on the
// card slug of the project root: .hoom/cache/locks/piloto-<slug>.lock.
func Candado(root, slug string) string {
	return hoomfs.LockPath(root, "piloto-"+slug)
}

// MsgYaCorre is what a pilot gets when another process already holds the
// card's lock: `el piloto de <slug> ya corre en otro proceso`.
func MsgYaCorre(slug string) string {
	return "el piloto de " + slug + " ya corre en otro proceso"
}

// AvisoDeFirma is the line `hoom aprobar <slug>` and `hoom spec approve
// <ruta>` add after signing, when the card of spec (the card whose spec path
// is spec, .hoom/specs/<slug>.md) has `auto: hasta-humano` in its item (read
// like the riesgo: the main checkout's item when it has it):
// `la tarjeta tiene piloto automatico: sigue con: hoom piloto <slug>`. ""
// when the spec is no card's or its card has no pilot. root is the project;
// spec is relative to it or absolute. It launches nothing.
func AvisoDeFirma(root, spec string) string {
	slug, ok := tarjetaDelSpec(root, spec)
	if !ok {
		return ""
	}
	it, err := itemDe(root, slug)
	if err != nil || it.Auto != item.AutoHastaHumano {
		return ""
	}
	return "la tarjeta tiene piloto automatico: sigue con: hoom piloto " + slug
}

// tarjetaDelSpec is the card whose spec is spec: .hoom/specs/<slug>.md of
// the project or of the card's workspace.
func tarjetaDelSpec(root, spec string) (string, bool) {
	p := spec
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	p = filepath.Clean(p)
	dir, name := filepath.Split(p)
	slug := strings.TrimSuffix(name, ".md")
	if slug == name || !item.ValidSlug(slug) {
		return "", false
	}
	if filepath.Base(filepath.Clean(dir)) != "specs" || filepath.Base(filepath.Dir(filepath.Clean(dir))) != ".hoom" {
		return "", false
	}
	return slug, true
}

// itemDe reads the card's item like the riesgo: the main checkout's when it
// has it, if not the one of the card's workspace.
func itemDe(root, slug string) (item.Item, error) {
	principal := root
	if p, ok := gitx.CheckoutPrincipal(root); ok {
		principal = p
	}
	it, err := item.Load(principal, slug)
	if err == nil {
		return it, nil
	}
	if dir, derr := runcmd.TaskDir(principal, slug); derr == nil && dir != principal {
		return item.Load(dir, slug)
	}
	return it, err
}

// Opciones are one run of the pilot on a card.
type Opciones struct {
	// Root is the project: the main checkout, the tree the Studio serves.
	Root string
	// Manifest is the project's hoom.yaml (its base, block_on and the
	// autonomia policy of LoadRoles); nil = read from Root.
	Manifest *manifest.Manifest
	Slug     string
	// Lanzar is the skin every launch goes through (lanzar.Params): the
	// Studio's own (its Runs, Candado, Grupo and a Despues that runs the next
	// turn), or the terminal's. Root and Manifest are taken from Opciones
	// when empty.
	Lanzar lanzar.Params
	// Salida also gets every line of the pilot (the output of `hoom
	// piloto`); the log of the card's newest closed envelope always gets
	// them. nil = only the log.
	Salida io.Writer
	// Ahora is the clock the turns read (Estado.Hora); nil = time.Now.
	Ahora func() time.Time
	// Proveedores are the providers of the machine (Estado.Proveedores); nil
	// = providers.Detect.
	Proveedores func() []providers.Info
}

// Resultado is what a run of the pilot did.
type Resultado struct {
	Slug string `json:"slug"`
	// Decisiones are the decisions of every turn, in order: the last one is
	// the stop (or, for Avanzar, the launch it returned on).
	Decisiones []boardcmd.PilotDecision `json:"-"`
	// Lineas are the pilot's lines, in the order they were written.
	Lineas []string `json:"lineas"`
	// Trabajo is the job the last turn launched, when Avanzar returned on a
	// launch; nil otherwise.
	Trabajo *lanzar.Trabajo `json:"-"`
	// ExitCode is `hoom piloto`'s: 0 when it stopped in a human column or in
	// Hecho (the table's case 3), 1 on any other stop.
	ExitCode int `json:"exit_code"`
}

// esperaAvanzar is how long the Studio's turn waits for the card's lock:
// the turn that launched a job lets go of it right after the launch, and a
// job that ends at once runs the next turn meanwhile.
const esperaAvanzar = 3 * time.Second

// Avanzar runs turns on the card until one stops or launches a job, and
// returns then, without waiting for that job (the Studio's skin: its
// lanzar.Params.Despues runs Avanzar again when the job ends). It holds
// Candado(root, slug) while it decides and acts, never while the job runs;
// with the lock taken by another process it stops with MsgYaCorre(slug).
// The error is a broken setup (the project or the card cannot be read); a
// stop is a Resultado.
func Avanzar(ctx context.Context, o Opciones) (Resultado, error) {
	r := &corrida{o: completar(o), res: Resultado{Slug: o.Slug}}
	soltar, err := hoomfs.Lock(Candado(r.o.Root, r.o.Slug), esperaAvanzar)
	if err != nil {
		if errors.Is(err, hoomfs.ErrLocked) {
			r.detener(boardcmd.PilotDecision{Paso: boardcmd.PasoDetenerse, Why: MsgYaCorre(r.o.Slug)})
			return r.res, nil
		}
		return r.res, err
	}
	defer soltar()
	for {
		d, t, err := r.vuelta(ctx)
		if err != nil {
			return r.res, err
		}
		switch d.Paso {
		case boardcmd.PasoLanzar:
			r.res.Trabajo = t
			return r.res, nil
		case boardcmd.PasoCerrarHallazgos:
			continue
		}
		return r.res, nil
	}
}

// Correr runs turns on the card until one stops, waiting for every job it
// launches (`hoom piloto`), holding Candado(root, slug) the whole time. With
// the lock taken by another process it does not run a turn: it returns an
// error with the text MsgYaCorre(slug).
func Correr(ctx context.Context, o Opciones) (Resultado, error) {
	r := &corrida{o: completar(o), res: Resultado{Slug: o.Slug}}
	soltar, err := hoomfs.Lock(Candado(r.o.Root, r.o.Slug), 0)
	if err != nil {
		if errors.Is(err, hoomfs.ErrLocked) {
			return r.res, errors.New(MsgYaCorre(r.o.Slug))
		}
		return r.res, err
	}
	defer soltar()
	for {
		if err := ctx.Err(); err != nil {
			return r.res, err
		}
		d, t, err := r.vuelta(ctx)
		if err != nil {
			return r.res, err
		}
		switch d.Paso {
		case boardcmd.PasoLanzar:
			if t != nil {
				t.Wait()
			}
			continue
		case boardcmd.PasoCerrarHallazgos:
			continue
		}
		return r.res, nil
	}
}

// completar fills what Opciones leaves to the defaults.
func completar(o Opciones) Opciones {
	if o.Ahora == nil {
		o.Ahora = func() time.Time { return time.Now().UTC() }
	}
	if o.Proveedores == nil {
		o.Proveedores = providers.Detect
	}
	if o.Lanzar.Root == "" {
		o.Lanzar.Root = o.Root
	}
	return o
}

// corrida is one run of the pilot: its options and what it did so far.
type corrida struct {
	o   Opciones
	res Resultado
	log string // the log of the card's newest closed envelope; "" = none
}

// linea writes one line of the pilot to the log of the card's newest closed
// envelope and to Salida.
func (r *corrida) linea(format string, args ...any) {
	l := fmt.Sprintf(format, args...)
	r.res.Lineas = append(r.res.Lineas, l)
	if r.log != "" {
		if f, err := os.OpenFile(r.log, os.O_WRONLY|os.O_APPEND, 0); err == nil {
			fmt.Fprintln(f, l)
			f.Close()
		}
	}
	if r.o.Salida != nil {
		fmt.Fprintln(r.o.Salida, l)
	}
}

// detener records a stop: its line and its exit code (0 only in a human
// column or Hecho, the table's case 3).
func (r *corrida) detener(d boardcmd.PilotDecision) {
	if d.Paso == "" {
		d.Paso = boardcmd.PasoDetenerse
	}
	r.res.Decisiones = append(r.res.Decisiones, d)
	r.linea("piloto automatico: se detiene: %s", d.Why)
	r.res.ExitCode = 1
	if d.Caso == 3 {
		r.res.ExitCode = 0
	}
}

func parada(why string) boardcmd.PilotDecision {
	return boardcmd.PilotDecision{Paso: boardcmd.PasoDetenerse, Why: why}
}

// manifiesto is the project's hoom.yaml: Opciones' or read from Root.
func (r *corrida) manifiesto() (*manifest.Manifest, error) {
	if r.o.Manifest != nil {
		return r.o.Manifest, nil
	}
	m, err := manifest.Load(r.o.Root, profiles.Resolve)
	if err != nil {
		return nil, err
	}
	r.o.Manifest = m
	return m, nil
}

// vuelta is one turn: save, build the Estado, ask Piloto, carry out the
// step. The decision it returns is the one it carried out (a stop when the
// save or the launch failed).
func (r *corrida) vuelta(ctx context.Context) (boardcmd.PilotDecision, *lanzar.Trabajo, error) {
	m, err := r.manifiesto()
	if err != nil {
		return boardcmd.PilotDecision{}, nil, err
	}
	root, slug := r.o.Root, r.o.Slug
	c, err := boardcmd.CardFor(root, m.BaseBranch, m.FindingsBlockOn(), slug, r.o.Ahora())
	if err != nil {
		return boardcmd.PilotDecision{}, nil, err
	}
	r.log = r.logDelUltimo(slug)
	// 1. save, only with auto and the task open
	if c.Item.Auto == item.AutoHastaHumano && c.Evidence.Source == boardcmd.SourceWorktree {
		g, err := itemcmd.SaveTarea(root, m.BaseBranch, m.FindingsBlockOn(), slug)
		if err != nil {
			d := parada("no pudo guardar el trabajo de la tarjeta: " + err.Error())
			r.detener(d)
			return d, nil, nil
		}
		for _, cm := range g.Commits {
			r.linea("piloto automatico: guarda el trabajo de la tarjeta: %s", cm.SHA)
		}
	}
	// 2. the Estado; 3. the decision
	e, err := Armar(r.o)
	if err != nil {
		return boardcmd.PilotDecision{}, nil, err
	}
	d := boardcmd.Piloto(e)
	// 4. the step
	switch d.Paso {
	case boardcmd.PasoLanzar:
		t, ok := r.lanzar(ctx, m, d)
		if !ok {
			return parada(""), nil, nil
		}
		return d, t, nil
	case boardcmd.PasoCerrarHallazgos:
		if !r.cerrarHallazgos(m, d) {
			return parada(""), nil, nil
		}
		return d, nil, nil
	}
	r.detener(d)
	return d, nil, nil
}

// logDelUltimo is the log of the card's newest closed envelope.
func (r *corrida) logDelUltimo(slug string) string {
	sobres := sobresCerrados(r.o.Root, slug)
	if len(sobres) == 0 {
		return ""
	}
	return filepath.Join(r.o.Root, ".hoom", envelope.DirName, sobres[len(sobres)-1].ID+".log")
}

// lanzar carries out a PasoLanzar through lanzar.Lanzar. ok is false when
// it could not launch (the pilot stopped).
func (r *corrida) lanzar(ctx context.Context, m *manifest.Manifest, d boardcmd.PilotDecision) (*lanzar.Trabajo, bool) {
	regla := "piloto automatico: regla " + d.Regla
	switch {
	case d.Intento > 0:
		regla += fmt.Sprintf(" intento %d", d.Intento)
	case d.Ronda > 0:
		regla += fmt.Sprintf(" ronda %d", d.Ronda)
	}
	r.linea("%s", regla)
	p := r.o.Lanzar
	if p.Manifest == nil {
		p.Manifest = m
	}
	req := lanzar.Request{Action: d.Action, Provider: d.Provider, Pedido: d.Pedido, Pilot: true,
		Resume: d.Resume, Delta: d.Delta, Regla: d.Regla, Intento: d.Intento, Ronda: d.Ronda,
		Hallazgos: d.Hallazgos, Bloqueantes: d.Bloqueantes}
	if d.BudgetUSD > 0 {
		b := d.BudgetUSD
		req.BudgetUSD = &b
	}
	r.res.Decisiones = append(r.res.Decisiones, d)
	t, err := lanzar.Lanzar(ctx, p, r.o.Slug, req)
	if err != nil {
		r.linea("piloto automatico: no pudo lanzar al %s: %s", d.Role, err)
		r.res.ExitCode = 1
		return nil, false
	}
	if d.BudgetUSD > 0 {
		r.linea("piloto automatico: pide al %s con %s (%s USD): sobre %s", d.Role, d.Provider, lanzar.USD(d.BudgetUSD), t.EnvelopeID)
	} else {
		r.linea("piloto automatico: pide al %s con %s (sin tope en USD): sobre %s", d.Role, d.Provider, t.EnvelopeID)
	}
	return t, true
}

// cerrarHallazgos carries out a PasoCerrarHallazgos: it closes each finding
// with its evidence, runs the card's verify, saves, and stops when the new
// verdict is not green. ok is false when the pilot stopped.
func (r *corrida) cerrarHallazgos(m *manifest.Manifest, d boardcmd.PilotDecision) bool {
	r.res.Decisiones = append(r.res.Decisiones, d)
	root, slug := r.o.Root, r.o.Slug
	dir, err := runcmd.TaskDir(root, slug)
	if err != nil {
		r.detener(parada(err.Error()))
		return false
	}
	for _, id := range d.Hallazgos {
		if _, err := finding.Resolve(dir, id, finding.StatusCorrected, d.Evidencias[id], finding.AutorPiloto); err != nil {
			r.detener(parada("no pudo cerrar el hallazgo " + id + ": " + err.Error()))
			return false
		}
	}
	r.linea("piloto automatico: cierra %d hallazgos con su test (ronda %d): %s", len(d.Hallazgos), d.Ronda, strings.Join(d.Hallazgos, ", "))
	tm, err := manifest.Load(dir, profiles.Resolve)
	if err != nil {
		r.detener(parada(err.Error()))
		return false
	}
	v, _, err := verifycmd.Run(tm, verifycmd.Options{Spec: filepath.Join(dir, ".hoom", "specs", slug+".md")})
	if err != nil {
		r.detener(parada(err.Error()))
		return false
	}
	g, err := itemcmd.SaveTarea(root, m.BaseBranch, m.FindingsBlockOn(), slug)
	if err != nil {
		r.detener(parada("no pudo guardar el trabajo de la tarjeta: " + err.Error()))
		return false
	}
	for _, cm := range g.Commits {
		r.linea("piloto automatico: guarda el trabajo de la tarjeta: %s", cm.SHA)
	}
	if v.Verdict != "green" {
		var gates []string
		for _, gr := range v.Gates {
			if gr.Required && (gr.Status == verdict.StatusFail || gr.Status == verdict.StatusError) {
				gates = append(gates, gr.Name)
			}
		}
		r.detener(boardcmd.PilotDecision{Paso: boardcmd.PasoDetenerse, Caso: 7, Ronda: d.Ronda,
			Why: fmt.Sprintf("despues de cerrar los hallazgos de la ronda %d, el veredicto %s sigue rojo en %s: te toca",
				d.Ronda, v.ID, strings.Join(gates, ", "))})
		return false
	}
	return true
}

// sobresCerrados are the card's closed envelope records of the project,
// oldest first (by start, then id).
func sobresCerrados(root, slug string) []envelope.Record {
	var out []envelope.Record
	for _, rec := range envelope.List(root) {
		if rec.Task == slug && rec.Done() {
			out = append(out, rec)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].StartedAt.Before(out[j].StartedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// Armar reads the card's boardcmd.Estado from the disk: the card derived
// with o.Manifest's base and block_on, its closed envelopes (oldest first)
// with the verdict each one emitted and the session id of its run's
// sidecar, the card's newest complete verdict, its review records, the open
// findings of its task that block (finding.Bloquea) with the protected tests
// (agentcmd.TestsProtegidos) whose text contains each id, the project's
// autonomia policy, the newest "5h" datum per provider of the sidecars of
// the card's runs, the machine's providers and o.Ahora.
func Armar(o Opciones) (boardcmd.Estado, error) {
	o = completar(o)
	m := o.Manifest
	if m == nil {
		var err error
		if m, err = manifest.Load(o.Root, profiles.Resolve); err != nil {
			return boardcmd.Estado{}, err
		}
	}
	root, slug, base, blockOn := o.Root, o.Slug, m.BaseBranch, m.FindingsBlockOn()
	hora := o.Ahora()
	c, err := boardcmd.CardFor(root, base, blockOn, slug, hora)
	if err != nil {
		return boardcmd.Estado{}, err
	}
	e := boardcmd.Estado{Tarjeta: c, Hora: hora, Proveedores: o.Proveedores()}
	roles, err := manifest.LoadRoles(root)
	if err != nil {
		return boardcmd.Estado{}, err
	}
	e.Politica = roles.AutonomiaOrDefault()

	dir := root
	if c.Evidence.Source == boardcmd.SourceWorktree {
		if d, err := runcmd.TaskDir(root, slug); err == nil {
			dir = d
		}
	}
	lugares := []string{root}
	if dir != root {
		lugares = append([]string{dir}, root)
	}
	veredictos := map[string]*verdict.Verdict{}
	for _, l := range lugares {
		vs, _, _ := verdict.LoadAllWithWarnings(l)
		for _, v := range vs {
			if _, ok := veredictos[v.ID]; !ok {
				veredictos[v.ID] = v
			}
		}
	}
	metas := map[string]runcmd.Meta{}
	for _, l := range []string{root, dir} {
		for _, mt := range runcmd.Metas(l) {
			if _, ok := metas[mt.ID]; !ok {
				metas[mt.ID] = mt
			}
		}
	}
	for _, rec := range sobresCerrados(root, slug) {
		s := boardcmd.Sobre{Record: rec, Veredicto: veredictos[rec.VerdictID]}
		if rec.VerdictID == "" {
			s.Veredicto = nil
		}
		if mt, ok := metas[rec.RunID]; ok && rec.RunID != "" {
			s.SesionID = mt.ProviderSessionID
		}
		e.Sobres = append(e.Sobres, s)
	}
	if id := c.Evidence.VerdictID; id != "" {
		e.Veredicto = veredictos[id]
	}
	recs, _, err := reviewcmd.RecordsErr(dir)
	if err != nil {
		return boardcmd.Estado{}, err
	}
	for _, rv := range recs {
		if rv.Task == slug {
			e.Reviews = append(e.Reviews, rv)
		}
	}
	if blockOn != "" {
		items, _, err := finding.List(dir, base, true)
		if err != nil {
			return boardcmd.Estado{}, err
		}
		ctxF, err := finding.ContextoDe(dir, root, blockOn)
		if err != nil {
			return boardcmd.Estado{}, err
		}
		for _, it := range finding.Juzgar(items, ctxF) {
			if it.Task == slug && it.Status == finding.StatusOpen && it.Bloquea {
				e.Bloqueantes = append(e.Bloqueantes, it)
			}
		}
	}
	if len(e.Bloqueantes) > 0 {
		protegidos, err := agentcmd.TestsProtegidos(root, slug)
		if err != nil {
			return boardcmd.Estado{}, err
		}
		e.Citas = citas(dir, protegidos, e.Bloqueantes)
	}
	e.Ventanas = ventanas(metas, slug)
	return e, nil
}

// citas are, by finding id, the protected tests whose text contains it.
func citas(dir string, protegidos []string, bloqueantes []finding.Item) map[string][]string {
	out := map[string][]string{}
	r, err := os.OpenRoot(dir)
	if err != nil {
		return out
	}
	defer r.Close()
	textos := map[string]string{}
	for _, p := range protegidos {
		raw, err := r.ReadFile(filepath.FromSlash(p))
		if err == nil {
			textos[p] = string(raw)
		}
	}
	for _, f := range bloqueantes {
		for _, p := range protegidos {
			if t, ok := textos[p]; ok && strings.Contains(t, f.ID) {
				out[f.ID] = append(out[f.ID], p)
			}
		}
	}
	return out
}

// ventanas is the newest "5h" datum per provider of the card's runs.
func ventanas(metas map[string]runcmd.Meta, slug string) map[string]runcmd.VentanaRun {
	type dato struct {
		v      runcmd.VentanaRun
		cuando time.Time
	}
	mejor := map[string]dato{}
	for _, mt := range metas {
		if mt.Task != slug {
			continue
		}
		cuando := mt.EndedAt
		if cuando.IsZero() {
			cuando = mt.CreatedAt
		}
		for _, v := range mt.Ventanas {
			if v.Nombre != "5h" {
				continue
			}
			if d, ok := mejor[mt.Provider]; !ok || cuando.After(d.cuando) {
				mejor[mt.Provider] = dato{v: v, cuando: cuando}
			}
		}
	}
	out := map[string]runcmd.VentanaRun{}
	for p, d := range mejor {
		out[p] = d.v
	}
	return out
}

// Entorno is the process boundary of `hoom piloto`: where it was called
// from and its standard streams. Tests replace it whole.
type Entorno struct {
	Dir string    // the working directory
	Out io.Writer // the pilot's lines, or the JSON
	Err io.Writer // errors, as `hoom: <error>`
}

// Real is the Entorno of the hoom process.
func Real() Entorno {
	return Entorno{Dir: ".", Out: os.Stdout, Err: os.Stderr}
}

// Run is `hoom piloto <slug> [--json]`, and it owns its exit code. The
// project is the one e.Dir belongs to (from inside a task's worktree, the
// project that contains it). It runs Correr on the card and prints each
// line of the pilot as it is written; with --json it prints the Resultado
// as JSON (`{"slug": ..., "lineas": [...], "exit_code": n}`) instead. It
// returns Resultado.ExitCode: 0 when the pilot stopped in a human column or
// in Hecho, 1 on any other stop. With the card's lock held by another
// process it prints `hoom: el piloto de <slug> ya corre en otro proceso` to
// e.Err and returns 1; a request it does not understand (no slug, an unknown
// flag) is a usage error with UsageText and returns 2.
func Run(args []string, e Entorno) int {
	if e.Out == nil {
		e.Out = io.Discard
	}
	if e.Err == nil {
		e.Err = io.Discard
	}
	if e.Dir == "" {
		e.Dir = "."
	}
	fs := flag.NewFlagSet("piloto", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "")
	ops, err := cliargs.Operands(fs, args, "piloto", UsageText, 1)
	if err != nil {
		if errors.Is(err, cliargs.ErrHelp) {
			fmt.Fprintln(e.Out, UsageText)
			return 0
		}
		fmt.Fprintln(e.Err, err.Error())
		return 2
	}
	slug := ops[0]
	if !item.ValidSlug(slug) {
		fmt.Fprintln(e.Err, cliargs.NewUsageError("piloto", fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", slug), UsageText).Error())
		return 2
	}
	root, err := proyecto(e.Dir)
	if err != nil {
		fmt.Fprintf(e.Err, "hoom: %v\n", err)
		return 1
	}
	o := Opciones{Root: root, Slug: slug}
	if !*asJSON {
		o.Salida = e.Out
	}
	res, err := Correr(context.Background(), o)
	if err != nil {
		fmt.Fprintf(e.Err, "hoom: %v\n", err)
		return 1
	}
	if *asJSON {
		if res.Lineas == nil {
			res.Lineas = []string{}
		}
		enc := json.NewEncoder(e.Out)
		enc.SetIndent("", "  ")
		_ = enc.Encode(res)
	}
	return res.ExitCode
}

// proyecto is the project dir belongs to: inside <proyecto>/.hoom/worktrees/
// <slug>/ at any depth, the tree that contains the workspace; outside, the
// directory that holds hoom.yaml.
func proyecto(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	for _, p := range []string{abs, evalSymlinks(abs)} {
		sep := string(filepath.Separator)
		marca := sep + ".hoom" + sep + "worktrees" + sep
		if i := strings.Index(p+sep, marca); i > 0 {
			return p[:i], nil
		}
	}
	return manifest.Find(abs)
}

func evalSymlinks(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
