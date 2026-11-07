package costcmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// rolReviewer is the role of the runs `hoom review` starts.
const rolReviewer = "reviewer"

// SinNada is Build's error for a slug with no runs and no reviews: the verb
// prints it and exits 1.
type SinNada struct{ Slug string }

func (e *SinNada) Error() string { return fmt.Sprintf(ErrSinNada, e.Slug) }

// Build reads the project at root and returns the report: every task when
// slug is "", or that task alone. A slug with no runs and no reviews is a
// *SinNada, whose text is fmt.Sprintf(ErrSinNada, slug).
//
// The runs are the project's (.hoom/runs at root): a sidecar inside a task
// worktree goes away with task done, so it is not counted (spec, non-goals).
// The review records and findings are also read from every task worktree:
// while a task is in course they live in its worktree (they reach the
// project with the merge). A worktree only speaks for its own task.
func Build(root, slug string) (Reporte, error) {
	ctx := &contexto{root: root, porID: map[string]runcmd.Meta{}, severidad: map[string]string{}}
	if err := ctx.cargarRuns(); err != nil {
		return Reporte{}, err
	}
	recs, err := ctx.cargarEvidencia()
	if err != nil {
		return Reporte{}, err
	}
	runsDe, recsDe := ctx.asignar(recs)
	slugs, err := elegir(slug, runsDe, recsDe)
	if err != nil {
		return Reporte{}, err
	}
	rep := Reporte{Tareas: []Tarea{}}
	for _, t := range slugs {
		rep.Tareas = append(rep.Tareas, ctx.tarea(t, runsDe[t], recsDe[t]))
	}
	sort.SliceStable(rep.Tareas, func(i, j int) bool {
		a, b := rep.Tareas[i], rep.Tareas[j]
		if !a.UltimaActividad.Equal(b.UltimaActividad) {
			return a.UltimaActividad.After(b.UltimaActividad)
		}
		return a.Slug < b.Slug
	})
	rep.Ilegibles = ctx.ilegibles
	rep.Avisos = append([]string{}, ctx.avisos...)
	return rep, nil
}

// cargarRuns reads the project's run sidecars; the unreadable ones are
// counted, never dropped in silence.
func (c *contexto) cargarRuns() error {
	metas, sinLeer, err := runcmd.MetasConIlegibles(c.root)
	if err != nil {
		return err
	}
	c.ilegibles += len(sinLeer)
	for _, m := range metas {
		c.porID[m.ID] = m
	}
	return nil
}

// fuente is one place the evidence is read from: the project (tarea "")
// or the worktree of one task, which speaks only for that task.
type fuente struct {
	dir   string
	tarea string
}

// cargarEvidencia reads the review records of the project and of every task
// worktree, and the severity of the findings they name. A record that a
// worktree carries for another task, or with a task that is no slug, is left
// out and said; what cannot be read is said too.
func (c *contexto) cargarEvidencia() ([]reviewcmd.Record, error) {
	fuentes := append([]fuente{{dir: c.root}}, c.worktrees()...)
	vistos := map[string]bool{}
	var recs []reviewcmd.Record
	sinLeer := 0
	conHallazgos := false
	for _, f := range fuentes {
		rs, warnings, err := reviewcmd.RecordsErr(f.dir)
		if err != nil {
			if f.tarea == "" {
				return nil, err
			}
			c.avisar(fmt.Sprintf("no se pudo leer .hoom/reviews del worktree %q: sus rondas no se cuentan", f.tarea))
		}
		sinLeer += len(warnings)
		for _, r := range rs {
			if vistos[r.ID] {
				continue
			}
			vistos[r.ID] = true
			switch {
			case r.Task != "" && !item.ValidSlug(r.Task):
				c.avisar(fmt.Sprintf("el registro %s tiene una tarea que no es un slug (%q): no se cuenta", r.ID, r.Task))
				continue
			case f.tarea != "" && r.Task != "" && r.Task != f.tarea:
				c.avisar(fmt.Sprintf("el worktree %q tiene el registro %s de la tarea %q: no se cuenta", f.tarea, r.ID, r.Task))
				continue
			}
			recs = append(recs, r)
			conHallazgos = conHallazgos || len(r.Findings) > 0
		}
	}
	if sinLeer > 0 {
		c.avisar(fmt.Sprintf("%d registros de review ilegibles: las rondas pueden ser mas", sinLeer))
	}
	if conHallazgos {
		c.cargarSeveridades(fuentes)
	}
	return recs, nil
}

// cargarSeveridades reads the findings of every source. The project's come
// first and are never replaced; a worktree adds only its task's own.
func (c *contexto) cargarSeveridades(fuentes []fuente) {
	sinLeer := 0
	for _, f := range fuentes {
		items, ilegibles, _, err := finding.ListConIlegibles(f.dir, "", false)
		if err != nil {
			c.avisar("no se pudieron leer los hallazgos: los de las rondas quedan sin dato")
			continue
		}
		sinLeer += len(ilegibles)
		for _, it := range items {
			if _, ya := c.severidad[it.ID]; ya || (f.tarea != "" && it.Task != f.tarea) {
				continue
			}
			c.severidad[it.ID] = it.Severity
		}
	}
	if sinLeer > 0 {
		c.avisar(fmt.Sprintf("%d hallazgos ilegibles: quedan sin dato en sus rondas", sinLeer))
	}
}

// asignar gives every run and every record its task. A run is its task's,
// else its finding_task's; a run with neither (an older review with --spec
// and no --task) is the task of the record that names it, so a total is
// never less than its rounds. A record that names a run of ANOTHER task does
// not take it: it would be counted twice.
func (c *contexto) asignar(recs []reviewcmd.Record) (map[string][]runcmd.Meta, map[string][]reviewcmd.Record) {
	ids := make([]string, 0, len(c.porID))
	for id := range c.porID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	runsDe := map[string][]runcmd.Meta{}
	for _, id := range ids {
		if m := c.porID[id]; tareaDe(m) != "" {
			runsDe[tareaDe(m)] = append(runsDe[tareaDe(m)], m)
		}
	}
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].CreatedAt.Before(recs[j].CreatedAt) })
	recsDe := map[string][]reviewcmd.Record{}
	for _, r := range recs {
		if r.Task != "" {
			recsDe[r.Task] = append(recsDe[r.Task], r)
		}
	}
	for _, r := range recs {
		t := r.Task
		if t == "" {
			continue
		}
		suyos := map[string]bool{}
		for _, m := range runsDe[t] {
			suyos[m.ID] = true
		}
		for _, lr := range r.Runs {
			m, ok := c.porID[lr.Run]
			switch {
			case !ok || suyos[m.ID]:
			case tareaDe(m) != "" && tareaDe(m) != t:
				c.avisar(fmt.Sprintf("el registro %s nombra el run %s, que es de la tarea %q: no se suma a %q", r.ID, m.ID, tareaDe(m), t))
			default:
				suyos[m.ID] = true
				runsDe[t] = append(runsDe[t], m)
			}
		}
	}
	return runsDe, recsDe
}

// elegir is the tasks of the report: slug alone (a *SinNada when it has no
// runs and no reviews), or every task with either.
func elegir(slug string, runsDe map[string][]runcmd.Meta, recsDe map[string][]reviewcmd.Record) ([]string, error) {
	if slug != "" {
		if len(runsDe[slug]) == 0 && len(recsDe[slug]) == 0 {
			return nil, &SinNada{Slug: slug}
		}
		return []string{slug}, nil
	}
	vistos := map[string]bool{}
	for t := range runsDe {
		vistos[t] = true
	}
	for t := range recsDe {
		vistos[t] = true
	}
	slugs := make([]string, 0, len(vistos))
	for t := range vistos {
		slugs = append(slugs, t)
	}
	sort.Strings(slugs)
	return slugs, nil
}

// worktrees lists the task worktrees under root/.hoom/worktrees, by name.
// No such directory is no worktrees; one that cannot be listed is said,
// because the rounds of the tasks in course may then be missing.
func (c *contexto) worktrees() []fuente {
	dir := filepath.Join(c.root, ".hoom", "worktrees")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			c.avisar("no se pudo listar .hoom/worktrees: las rondas de las tareas en curso pueden faltar")
		}
		return nil
	}
	var out []fuente
	for _, e := range entries {
		if e.IsDir() && item.ValidSlug(e.Name()) {
			out = append(out, fuente{dir: filepath.Join(dir, e.Name()), tarea: e.Name()})
		}
	}
	return out
}

// contexto is what every task of one report reads from the same project.
type contexto struct {
	root      string
	porID     map[string]runcmd.Meta
	severidad map[string]string // finding id -> low | medium | high
	ilegibles int
	avisos    []string
	verdicts  []*verdict.Verdict
	leidos    bool // verdicts were loaded
}

func (c *contexto) avisar(s string) {
	for _, a := range c.avisos {
		if a == s {
			return
		}
	}
	c.avisos = append(c.avisos, s)
}

func (c *contexto) tarea(slug string, runs []runcmd.Meta, recs []reviewcmd.Record) Tarea {
	t := Tarea{Slug: slug, PorProvider: map[string]Gasto{}, PorRol: []GastoRol{}, RondasReview: []Ronda{}}
	c.estado(&t)
	// the cost is until the task was accepted: what came after hecho_en is
	// counted apart and left out
	if t.AceptadaEn != nil {
		hasta := *t.AceptadaEn
		fuera := Fuera{}
		var antes []runcmd.Meta
		for _, m := range runs {
			if despuesDe(m.CreatedAt, hasta) {
				fuera.Runs++
				continue
			}
			antes = append(antes, m)
		}
		var recsAntes []reviewcmd.Record
		for _, r := range recs {
			if despuesDe(r.CreatedAt, hasta) {
				fuera.Rondas++
				continue
			}
			recsAntes = append(recsAntes, r)
		}
		runs, recs = antes, recsAntes
		t.DespuesDeAceptada = &fuera
		if fuera.Runs > 0 || fuera.Rondas > 0 {
			c.avisar(fmt.Sprintf("la tarea %q tiene %s y %s despues de aceptada: fuera del costo", slug,
				plural(fuera.Runs, "run", "runs"), plural(fuera.Rondas, "ronda", "rondas")))
		}
	}
	t.Rondas = len(recs)
	enLaTarea := map[string]bool{}
	for _, m := range runs {
		enLaTarea[m.ID] = true
		if m.CreatedAt.After(t.UltimaActividad) {
			t.UltimaActividad = m.CreatedAt
		}
	}
	for _, r := range recs {
		if r.CreatedAt.After(t.UltimaActividad) {
			t.UltimaActividad = r.CreatedAt
		}
	}

	for prov, rs := range porProvider(runs) {
		t.PorProvider[prov] = gasto(prov, rs)
	}

	type clave struct{ rol, prov string }
	grupos := map[clave][]runcmd.Meta{}
	for _, m := range runs {
		k := clave{m.Role, m.Provider}
		grupos[k] = append(grupos[k], m)
	}
	for k, rs := range grupos {
		t.PorRol = append(t.PorRol, GastoRol{Rol: k.rol, Provider: k.prov, Gasto: gasto(k.prov, rs)})
	}
	sort.Slice(t.PorRol, func(i, j int) bool {
		if t.PorRol[i].Rol != t.PorRol[j].Rol {
			return t.PorRol[i].Rol < t.PorRol[j].Rol
		}
		return t.PorRol[i].Provider < t.PorRol[j].Provider
	})

	nombrados := map[string]bool{}
	for _, r := range recs {
		ronda := Ronda{Cobertura: r.Cobertura, Lentes: append([]string{}, r.Lenses...),
			Hallazgos: map[string]int{"high": 0, "medium": 0, "low": 0}, Provider: r.Provider}
		id := r.ID
		ronda.ID = &id
		for _, f := range r.Findings {
			sev, ok := c.severidad[f]
			if _, conocida := ronda.Hallazgos[sev]; ok && conocida {
				ronda.Hallazgos[sev]++
			} else {
				ronda.HallazgosSinDato++
			}
		}
		var suyos []runcmd.Meta
		for _, lr := range r.Runs {
			nombrados[lr.Run] = true
			// only the task's own runs: one of another task was left out
			// (and said) when the task's runs were gathered
			if m, ok := c.porID[lr.Run]; ok && enLaTarea[m.ID] {
				suyos = append(suyos, m)
			}
		}
		if len(r.Runs) > 0 {
			prov := ronda.Provider
			if prov == "" && len(suyos) > 0 {
				prov = suyos[0].Provider
				ronda.Provider = prov
			}
			g := gasto(prov, suyos)
			ronda.Gasto = &g
		}
		t.RondasReview = append(t.RondasReview, ronda)
	}
	var sueltos []runcmd.Meta
	for _, m := range runs {
		if m.Role == rolReviewer && !nombrados[m.ID] {
			sueltos = append(sueltos, m)
		}
	}
	grupoSuelto := porProvider(sueltos)
	provs := make([]string, 0, len(grupoSuelto))
	for p := range grupoSuelto {
		provs = append(provs, p)
	}
	sort.Strings(provs)
	for _, p := range provs {
		g := gasto(p, grupoSuelto[p])
		t.RondasReview = append(t.RondasReview, Ronda{SinRonda: true, Lentes: []string{},
			Hallazgos: map[string]int{"high": 0, "medium": 0, "low": 0}, Provider: p, Gasto: &g})
	}
	return t
}

// despuesDe says whether t came after the acceptance hasta. hecho_en is
// kept to the second (item.MarkDone), so t is compared at that resolution:
// a round of 10:00:00.500 is not after an acceptance kept as 10:00:00.
func despuesDe(t, hasta time.Time) bool { return t.Truncate(time.Second).After(hasta) }

// tareaDe is the task a run belongs to: its task (where it ran) or, when it
// ran at the root, the task of its findings (costo-por-spec, amendment 2).
// "" leaves it to the record that names it.
func tareaDe(m runcmd.Meta) string {
	if m.Task != "" {
		return m.Task
	}
	return m.FindingTask
}

// estado: aceptada when the item has hecho_en; otherwise en curso while the
// task's worktree exists, and cerrada sin aceptar when it is gone. An item
// that exists and cannot be read is said: the state then comes from the
// worktree alone.
func (c *contexto) estado(t *Tarea) {
	it, err := item.Load(c.root, t.Slug)
	if err == nil && it.HechoEn != nil {
		t.Estado = EstadoAceptada
		at := it.HechoEn.UTC()
		t.AceptadaEn = &at
		if it.CommitFinal != "" {
			cf := it.CommitFinal
			t.CommitFinal = &cf
		}
		if v := c.veredicto(t.Slug); v != "" {
			t.Veredicto = &v
		}
		return
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		c.avisar(fmt.Sprintf("el item de %q no se pudo leer: su estado no sale de hecho_en", t.Slug))
	}
	if fi, err := os.Stat(filepath.Join(c.root, ".hoom", "worktrees", t.Slug)); err == nil && fi.IsDir() {
		t.Estado = EstadoEnCurso
		return
	}
	t.Estado = EstadoCerradaSinAceptar
}

// veredicto is the id of the newest green, complete verdict of the task's
// spec, "" when there is none.
func (c *contexto) veredicto(slug string) string {
	if !c.leidos {
		vs, warnings, err := verdict.LoadAllWithWarnings(c.root)
		c.verdicts, c.leidos = vs, true
		if err != nil || len(warnings) > 0 {
			c.avisar("hay veredictos ilegibles: el veredicto de una tarea aceptada puede faltar")
		}
	}
	var mejor *verdict.Verdict
	for _, v := range c.verdicts {
		if v == nil || v.Verdict != "green" || v.Partial || v.Spec == "" || finding.TaskOfSpec(v.Spec) != slug {
			continue
		}
		if mejor == nil || v.CreatedAt.After(mejor.CreatedAt) {
			mejor = v
		}
	}
	if mejor == nil {
		return ""
	}
	return mejor.ID
}

func porProvider(runs []runcmd.Meta) map[string][]runcmd.Meta {
	out := map[string][]runcmd.Meta{}
	for _, m := range runs {
		out[m.Provider] = append(out[m.Provider], m)
	}
	return out
}

// conTokens says whether u reported any token figure: a usage with only a
// cost, a duration or turns says nothing about tokens, and its zeros are
// not zeros.
func conTokens(u *providers.Usage) bool {
	return u != nil && (u.InputTokens > 0 || u.CachedTokens > 0 || u.OutputTokens > 0)
}

// gasto adds up the runs of ONE provider. A figure no run reported stays
// null, and one only some runs reported says how many lack it.
func gasto(provider string, runs []runcmd.Meta) Gasto {
	g := Gasto{Runs: len(runs)}
	var nue, cache, sal, razon int
	var usd float64
	var dur int64
	conUso, conNuevos, conRazon, conCosto, conDur := false, true, false, false, false
	var conVentana []runcmd.Meta
	for _, m := range runs {
		if u := m.Usage; conTokens(u) {
			conUso = true
			if n, ok := providers.EntradaNueva(provider, *u); ok {
				nue += n
			} else {
				conNuevos = false
			}
			cache += u.CachedTokens
			sal += u.OutputTokens
		} else {
			g.SinUso++
		}
		if u := m.Usage; u != nil && u.ReasoningTokens > 0 {
			conRazon = true
			razon += u.ReasoningTokens
		}
		if m.Usage != nil && m.Usage.CostUSD != nil {
			conCosto = true
			usd += *m.Usage.CostUSD
		} else {
			g.SinCosto++
		}
		if m.Status == runcmd.StatusRunning {
			g.EnCurso++
			continue
		}
		if !m.EndedAt.IsZero() {
			conDur = true
			if m.EndedAt.After(m.CreatedAt) {
				dur += m.EndedAt.Sub(m.CreatedAt).Milliseconds()
			}
		}
		if len(m.Ventanas) == 0 {
			g.SinVentana++
			continue
		}
		conVentana = append(conVentana, m)
	}
	if conUso {
		g.Tokens = Tokens{CacheLeido: &cache, Salida: &sal}
		if conNuevos {
			g.Tokens.Nuevos = &nue
		}
	}
	if conRazon {
		g.Tokens.Razonamiento = &razon
	}
	if conCosto {
		g.CostoUSD = &usd
	}
	if conDur {
		g.DuracionMS = &dur
	}
	g.Ventanas = movimientos(conVentana)
	return g
}

// fin is when a finished run ended: its ended_at, or its start when an old
// sidecar did not say.
func fin(m runcmd.Meta) time.Time {
	if m.EndedAt.IsZero() || m.EndedAt.Before(m.CreatedAt) {
		return m.CreatedAt
	}
	return m.EndedAt
}

// movimientos applies the window rule of the package doc to finished runs
// of one provider. Nil when none has a window.
func movimientos(runs []runcmd.Meta) map[string]Movimiento {
	if len(runs) == 0 {
		return nil
	}
	nombres := map[string]bool{}
	for _, m := range runs {
		for _, v := range m.Ventanas {
			nombres[v.Nombre] = true
		}
	}
	out := map[string]Movimiento{}
	for n := range nombres {
		out[n] = movimiento(n, runs)
	}
	return out
}

type tramo struct {
	meta runcmd.Meta
	v    runcmd.VentanaRun
}

func movimiento(nombre string, runs []runcmd.Meta) Movimiento {
	var ts []tramo
	for _, m := range runs {
		for _, v := range m.Ventanas {
			if v.Nombre == nombre {
				ts = append(ts, tramo{m, v})
				break
			}
		}
	}
	sort.SliceStable(ts, func(i, j int) bool {
		if !ts[i].meta.CreatedAt.Equal(ts[j].meta.CreatedAt) {
			return ts[i].meta.CreatedAt.Before(ts[j].meta.CreatedAt)
		}
		return ts[i].meta.ID < ts[j].meta.ID
	})
	var total float64
	alMenos := false
	for i := 0; i < len(ts); {
		bloque := []tramo{ts[i]}
		hasta := fin(ts[i].meta)
		j := i + 1
		for ; j < len(ts) && !ts[j].meta.CreatedAt.After(hasta); j++ {
			bloque = append(bloque, ts[j])
			if f := fin(ts[j].meta); f.After(hasta) {
				hasta = f
			}
		}
		puntos, reinicio := bloqueMovido(bloque)
		total += puntos
		alMenos = alMenos || reinicio
		i = j
	}
	return Movimiento{Puntos: &total, AlMenos: alMenos}
}

// bloqueMovido is what one block of overlapping runs moved a window, and
// whether the window reset inside it (then the figure is a lower bound).
func bloqueMovido(b []tramo) (float64, bool) {
	ultimo := b[0]
	reinicio := false
	for k, t := range b {
		if t.v.Reiniciada {
			reinicio = true
		}
		if k > 0 && runcmd.CambioDeReinicio(b[k-1].v.ReiniciaEn, t.v.ReiniciaEn) {
			reinicio = true
		}
		if !fin(t.meta).Before(fin(ultimo.meta)) {
			ultimo = t
		}
	}
	movido := ultimo.v.Fin - b[0].v.Inicio
	if reinicio || movido < 0 {
		// after the reset the window started over: what the block spent
		// since is at least where it ended
		return max(ultimo.v.Fin, 0), true
	}
	return movido, false
}
