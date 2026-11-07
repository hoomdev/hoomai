package costcmd

import (
	"fmt"
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

// Build reads the project at root and returns the report: every task when
// slug is "", or that task alone. A slug with no runs and no reviews is an
// error whose text is fmt.Sprintf(ErrSinNada, slug).
func Build(root, slug string) (Reporte, error) {
	metas, ilegibles, err := runcmd.MetasConIlegibles(root)
	if err != nil {
		return Reporte{}, err
	}
	recs, _, err := reviewcmd.RecordsErr(root)
	if err != nil {
		return Reporte{}, err
	}
	porID := map[string]runcmd.Meta{}
	runsDe := map[string][]runcmd.Meta{}
	for _, m := range metas {
		porID[m.ID] = m
		if m.Task != "" {
			runsDe[m.Task] = append(runsDe[m.Task], m)
		}
	}
	recsDe := map[string][]reviewcmd.Record{}
	conHallazgos := false
	for _, r := range recs {
		if r.Task == "" {
			continue
		}
		recsDe[r.Task] = append(recsDe[r.Task], r)
		conHallazgos = conHallazgos || len(r.Findings) > 0
	}
	var slugs []string
	if slug != "" {
		if len(runsDe[slug]) == 0 && len(recsDe[slug]) == 0 {
			return Reporte{}, &SinNada{Slug: slug}
		}
		slugs = []string{slug}
	} else {
		vistos := map[string]bool{}
		for t := range runsDe {
			vistos[t] = true
		}
		for t := range recsDe {
			vistos[t] = true
		}
		for t := range vistos {
			slugs = append(slugs, t)
		}
	}
	ctx := contexto{root: root, porID: porID}
	if conHallazgos {
		ctx.severidad = severidades(root)
	}
	rep := Reporte{Tareas: []Tarea{}, Ilegibles: len(ilegibles)}
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
	return rep, nil
}

// SinNada is Build's error for a slug with no runs and no reviews: the verb
// prints it and exits 1.
type SinNada struct{ Slug string }

func (e *SinNada) Error() string { return fmt.Sprintf(ErrSinNada, e.Slug) }

// contexto is what every task of one report reads from the same project.
type contexto struct {
	root      string
	porID     map[string]runcmd.Meta
	severidad map[string]string // finding id -> low | medium | high
	verdicts  []*verdict.Verdict
	leidos    bool // verdicts were loaded
}

// severidades maps every readable finding to its severity. What cannot be
// read is not counted: the round shows what hoom could see.
func severidades(root string) map[string]string {
	items, _, _ := finding.List(root, "", false)
	out := make(map[string]string, len(items))
	for _, it := range items {
		out[it.ID] = it.Severity
	}
	return out
}

func (c *contexto) tarea(slug string, runs []runcmd.Meta, recs []reviewcmd.Record) Tarea {
	t := Tarea{Slug: slug, Rondas: len(recs), PorProvider: map[string]Gasto{},
		PorRol: []GastoRol{}, RondasReview: []Ronda{}}
	c.estado(&t)
	for _, m := range runs {
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
			if sev, ok := c.severidad[f]; ok {
				if _, conocida := ronda.Hallazgos[sev]; conocida {
					ronda.Hallazgos[sev]++
				}
			}
		}
		var suyos []runcmd.Meta
		for _, lr := range r.Runs {
			nombrados[lr.Run] = true
			if m, ok := c.porID[lr.Run]; ok {
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

// estado: aceptada when the item has hecho_en; otherwise en curso while the
// task's worktree exists, and cerrada sin aceptar when it is gone.
func (c *contexto) estado(t *Tarea) {
	if it, err := item.Load(c.root, t.Slug); err == nil && it.HechoEn != nil {
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
		c.verdicts, _, _ = verdict.LoadAllWithWarnings(c.root)
		c.leidos = true
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

// nuevos is the input that did not come from the cache: Codex counts the
// cached tokens inside input_tokens, Claude (as hoom stores it) does not.
func nuevos(provider string, u *providers.Usage) int {
	if provider == "codex" {
		if n := u.InputTokens - u.CachedTokens; n > 0 {
			return n
		}
		return 0
	}
	return u.InputTokens
}

// gasto adds up the runs of ONE provider. A figure no run reported stays
// null.
func gasto(provider string, runs []runcmd.Meta) Gasto {
	g := Gasto{Runs: len(runs)}
	var nue, cache, sal, razon int
	var usd float64
	var dur int64
	conUso, conRazon, conCosto, conDur := false, false, false, false
	var conVentana []runcmd.Meta
	for _, m := range runs {
		if u := m.Usage; u != nil {
			conUso = true
			nue += nuevos(provider, u)
			cache += u.CachedTokens
			sal += u.OutputTokens
			if u.ReasoningTokens > 0 {
				conRazon = true
				razon += u.ReasoningTokens
			}
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
		if !m.EndedAt.IsZero() && m.EndedAt.After(m.CreatedAt) {
			conDur = true
			dur += m.EndedAt.Sub(m.CreatedAt).Milliseconds()
		} else if !m.EndedAt.IsZero() {
			conDur = true
		}
		if len(m.Ventanas) == 0 {
			g.SinVentana++
			continue
		}
		conVentana = append(conVentana, m)
	}
	if conUso {
		g.Tokens = Tokens{Nuevos: &nue, CacheLeido: &cache, Salida: &sal}
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
