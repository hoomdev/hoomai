package tablero

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/finding"
)

// ---------------------------------------------------------------- [2]

// seccion is one section of [2].
type seccion struct {
	titulo   string
	estilo   estilo
	hecho    bool // the section of the column Hecho
	tarjetas []*boardcmd.Card
}

// pasaFiltro: the card stays while the search filters.
func (m *Modelo) pasaFiltro(c *boardcmd.Card) bool {
	if !m.buscando || len(m.filtro) == 0 {
		return true
	}
	q := plegar(string(m.filtro))
	return strings.Contains(plegar(c.Slug), q) || strings.Contains(plegar(c.Item.Titulo), q)
}

// secciones are [2]'s sections in order: the cards that need your
// decision, the ones running, then one per column of the board.
func (m *Modelo) secciones() []seccion {
	decision := seccion{titulo: m.s.decisionTitulo, estilo: amarillo}
	curso := seccion{titulo: "EN CURSO", estilo: cian}
	var columnas []seccion
	for i := range m.tablero.Columns {
		col := &m.tablero.Columns[i]
		sc := seccion{hecho: col.ID == boardcmd.ColHecho}
		switch {
		case sc.hecho:
			sc.estilo = verde
		case col.Human:
			sc.estilo = amarillo
		}
		for j := range col.Cards {
			c := &col.Cards[j]
			if !m.pasaFiltro(c) {
				continue
			}
			switch {
			case c.NeedsDecision:
				decision.tarjetas = append(decision.tarjetas, c)
			case c.Running != nil:
				curso.tarjetas = append(curso.tarjetas, c)
			default:
				sc.tarjetas = append(sc.tarjetas, c)
			}
		}
		sc.titulo = fmt.Sprintf("%s %d", strings.ToUpper(col.Name), len(sc.tarjetas))
		if sc.hecho {
			signo := m.s.hechoCerrado
			if m.hechoAbierto {
				signo = m.s.hechoAbierto
			}
			sc.titulo += " " + signo
		}
		columnas = append(columnas, sc)
	}
	var out []seccion
	for _, sc := range append([]seccion{decision, curso}, columnas...) {
		if len(sc.tarjetas) > 0 || sc.hecho {
			out = append(out, sc)
		}
	}
	return out
}

// fila2 is a row of [2]: a card or Hecho's line.
type fila2 struct {
	slug  string
	hecho bool
}

func (m *Modelo) filas2() []fila2 {
	if !m.leido || len(m.todas()) == 0 {
		return nil
	}
	var out []fila2
	for _, sc := range m.secciones() {
		if sc.hecho {
			out = append(out, fila2{hecho: true})
			if !m.hechoAbierto {
				continue
			}
		}
		for _, c := range sc.tarjetas {
			out = append(out, fila2{slug: c.Slug})
		}
	}
	return out
}

// contenido2 is [2]'s content for an inner width wi: its lines, and for each
// line the row it is (-1 for a section title).
func (m *Modelo) contenido2(wi int) ([]linea, []int) {
	if !m.leido {
		return []linea{lin(" leyendo el tablero...")}, []int{-1}
	}
	if len(m.todas()) == 0 {
		return []linea{lin(` no hay tarjetas: hoom item add "<titulo>"`)}, []int{-1}
	}
	var out []linea
	var filas []int
	idx := 0
	for _, sc := range m.secciones() {
		if sc.hecho {
			marca := ""
			if !m.sel2.nada && m.sel2.idx == idx {
				marca = m.s.sel + " "
			}
			out = append(out, lin(" "+marca).masCon(sc.titulo, sc.estilo))
			filas = append(filas, idx)
			idx++
			if !m.hechoAbierto {
				continue
			}
		} else {
			out = append(out, lin(" ").masCon(sc.titulo, sc.estilo))
			filas = append(filas, -1)
		}
		for _, c := range sc.tarjetas {
			out = append(out, m.filaTarjeta(c, !m.sel2.nada && m.sel2.idx == idx, wi))
			filas = append(filas, idx)
			idx++
		}
	}
	return out, filas
}

// filaTarjeta is a card's row of [2].
func (m *Modelo) filaTarjeta(c *boardcmd.Card, sel bool, wi int) linea {
	marca := " "
	if sel {
		marca = m.s.sel
	}
	estiloSlug := normal
	if c.Red != nil || c.Interrupted != nil {
		estiloSlug = rojo
	}
	lugar := wi - 3
	tira := strings.Join(m.tira(c), "")
	t := celdas(tira)
	l := lin(" " + marca + " ")
	if lugar-t-1 >= 8 {
		s := cortar(c.Slug, lugar-t-1, m.s.corte)
		l = l.masCon(s, estiloSlug).mas(strings.Repeat(" ", lugar-t-1-celdas(s)) + " " + tira)
		return l
	}
	return l.masCon(cortar(c.Slug, lugar, m.s.corte), estiloSlug)
}

// signo is the sign of segment i of the card's meter: ✋ on the first falta
// when the card needs your decision.
func (m *Modelo) signo(c *boardcmd.Card, i int) string {
	seg := c.Meter[i]
	switch seg.State {
	case boardcmd.SegHecho:
		return m.s.hecho
	case boardcmd.SegNoAplica:
		return m.s.noAplica
	}
	if c.NeedsDecision {
		primera := true
		for _, s := range c.Meter[:i] {
			if s.State == boardcmd.SegFalta {
				primera = false
				break
			}
		}
		if primera {
			return m.s.decision
		}
	}
	return m.s.falta
}

// tira is the card's signs, one per segment.
func (m *Modelo) tira(c *boardcmd.Card) []string {
	out := make([]string, len(c.Meter))
	for i := range c.Meter {
		out[i] = m.signo(c, i)
	}
	return out
}

// elegidaDe2 is the card chosen in [2] ("" on Hecho's line or none).
func (m *Modelo) elegidaDe2() string {
	if m.sel2.nada || m.sel2.hecho || m.tarjeta(m.sel2.slug) == nil {
		return ""
	}
	if !m.pasaFiltro(m.tarjeta(m.sel2.slug)) {
		return ""
	}
	return m.sel2.slug
}

// elegir2 puts [2]'s selection on its i-th row (clamped).
func (m *Modelo) elegir2(i int) {
	fs := m.filas2()
	if len(fs) == 0 {
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= len(fs) {
		i = len(fs) - 1
	}
	m.sel2 = elegido2{slug: fs[i].slug, hecho: fs[i].hecho, idx: i}
}

// acomodar2 keeps [2]'s selection on its card or line; when it is no longer
// there, it takes the row with its index (the last one when there are
// fewer). At the start it is the first card row.
func (m *Modelo) acomodar2() {
	fs := m.filas2()
	if len(fs) == 0 {
		return
	}
	if m.sel2.nada {
		for i, f := range fs {
			if !f.hecho {
				m.elegir2(i)
				return
			}
		}
		m.elegir2(0)
		return
	}
	for i, f := range fs {
		if f.hecho == m.sel2.hecho && (f.hecho || f.slug == m.sel2.slug) {
			m.sel2.idx = i
			return
		}
	}
	m.elegir2(m.sel2.idx)
}

// alFiltrar puts the selection on the first row that stays.
func (m *Modelo) alFiltrar() {
	m.elegir2(0)
}

func (m *Modelo) abrirBusqueda() {
	m.ponerFoco(PanelTarjetas)
	m.buscando = true
	m.filtro = nil
}

func (m *Modelo) cerrarBusqueda() {
	m.buscando = false
	m.filtro = nil
	m.acomodar2()
}

// ---------------------------------------------------------------- [3]

// filas3 are [3]'s cards: those with running and those with interrupted,
// in the board's order.
func (m *Modelo) filas3() []*boardcmd.Card {
	var out []*boardcmd.Card
	for _, c := range m.todas() {
		if c.Running != nil || c.Interrupted != nil {
			out = append(out, c)
		}
	}
	return out
}

func (m *Modelo) elegidaDe3() string {
	for _, c := range m.filas3() {
		if c.Slug == m.sel3 {
			return c.Slug
		}
	}
	return ""
}

func (m *Modelo) elegir3(i int) {
	fs := m.filas3()
	if len(fs) == 0 {
		m.sel3, m.idx3 = "", 0
		return
	}
	i = max(0, min(i, len(fs)-1))
	m.sel3, m.idx3 = fs[i].Slug, i
}

func (m *Modelo) acomodar3() {
	fs := m.filas3()
	for i, c := range fs {
		if c.Slug == m.sel3 {
			m.idx3 = i
			return
		}
	}
	m.elegir3(m.idx3)
}

// contenido3 is [3]'s content for an inner width wi.
func (m *Modelo) contenido3(wi int) []linea {
	fs := m.filas3()
	if len(fs) == 0 {
		return []linea{lin(" nada corriendo")}
	}
	var out []linea
	for i, c := range fs {
		marca := " "
		if i == m.idx3 {
			marca = m.s.sel
		}
		lugar := wi - 3
		if c.Running == nil {
			i := c.Interrupted
			suf := " " + rolDe(i.Role) + " interrumpido"
			s := cortar(c.Slug, max(1, lugar-celdas(suf)), m.s.corte)
			out = append(out, linCon(" "+marca+" "+s+suf, rojo))
			continue
		}
		r := c.Running
		tiempo := reloj(m.ahora.Sub(r.StartedAt))
		suf := " " + rolDe(r.Role) + " " + tiempo
		if r.TimeoutS != 0 {
			con := suf + " de " + reloj(time.Duration(r.TimeoutS)*time.Second)
			if lugar-celdas(con) >= 8 {
				suf = con
			}
		}
		s := cortar(c.Slug, max(1, lugar-celdas(suf)), m.s.corte)
		estiloSlug := normal
		if c.Red != nil {
			estiloSlug = rojo
		}
		out = append(out, lin(" "+marca+" ").masCon(s, estiloSlug).mas(suf))
	}
	return out
}

// ---------------------------------------------------------------- [4]

func (m *Modelo) hallazgoElegido() (finding.Item, bool) {
	for _, h := range m.hallazgos {
		if h.ID == m.sel4 {
			return h, true
		}
	}
	return finding.Item{}, false
}

func (m *Modelo) elegir4(i int) {
	if len(m.hallazgos) == 0 {
		m.sel4, m.idx4 = "", 0
		return
	}
	i = max(0, min(i, len(m.hallazgos)-1))
	m.sel4, m.idx4 = m.hallazgos[i].ID, i
}

func (m *Modelo) acomodar4() {
	for i, h := range m.hallazgos {
		if h.ID == m.sel4 {
			m.idx4 = i
			return
		}
	}
	m.elegir4(m.idx4)
}

// estiloSeveridad is a severity's color.
func estiloSeveridad(s string) estilo {
	switch s {
	case "high":
		return rojo
	case "medium":
		return amarillo
	case "low":
		return gris
	}
	return normal
}

// donde is a finding's place as the rows write it: the base name of its file
// and ":<linea>", or "-" without a file.
func donde(f finding.Item) string {
	archivo, n := archivoYLinea(f)
	if archivo == "" {
		return "-"
	}
	if i := strings.LastIndexAny(archivo, `/\`); i >= 0 {
		archivo = archivo[i+1:]
	}
	if n > 0 {
		archivo += ":" + strconv.Itoa(n)
	}
	return archivo
}

// contenido4 is [4]'s content for an inner width wi.
func (m *Modelo) contenido4(wi int) []linea {
	if !m.hallLeidos {
		return []linea{lin(" leyendo los hallazgos...")}
	}
	if len(m.hallazgos) == 0 {
		return []linea{lin(" sin hallazgos abiertos")}
	}
	lugar := wi - 10
	tw := lugar / 2
	dw := lugar - tw - 1
	var out []linea
	for i, h := range m.hallazgos {
		marca := " "
		if i == m.idx4 {
			marca = m.s.sel
		}
		tarea := h.Task
		if tarea == "" {
			tarea = "-"
		}
		l := lin(" "+marca+" ").masCon(h.Severity, estiloSeveridad(h.Severity)).
			mas(strings.Repeat(" ", max(0, 6-celdas(h.Severity))) + " " + rellenar(cortar(tarea, tw, m.s.corte), tw) + " " +
				cortar(donde(h), dw, m.s.corte))
		out = append(out, l)
	}
	return out
}

// archivoYLinea is the finding's file and its line (0 without one).
func archivoYLinea(f finding.Item) (string, int) {
	archivo := f.File
	if i := strings.LastIndex(archivo, ":"); i >= 0 && digitos(archivo[i+1:]) {
		n, _ := strconv.Atoi(archivo[i+1:])
		return archivo[:i], n
	}
	if archivo == "" {
		return "", 0
	}
	if i := strings.Index(f.Description, archivo+":"); i >= 0 {
		resto := f.Description[i+len(archivo)+1:]
		j := 0
		for j < len(resto) && resto[j] >= '0' && resto[j] <= '9' {
			j++
		}
		if j > 0 {
			n, _ := strconv.Atoi(resto[:j])
			return archivo, n
		}
	}
	return archivo, 0
}

func digitos(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// ---------------------------------------------------------------- [1]

// lineaCheck is [1]'s second line, without its margin, in pieces.
func (m *Modelo) lineaCheck() linea {
	p := m.proyecto
	switch {
	case p.Check.OK:
		l := lin("check ").masCon("VERDE", verde)
		v := p.VeredictoDelCheck
		if v == nil && p.Veredicto != nil && p.Veredicto.ID == p.Check.VerdictID {
			v = p.Veredicto // the newest is the one the check refers to
		}
		if v != nil {
			l = l.mas(" · verify hace " + hace(m.ahora.Sub(v.CreatedAt)))
		}
		return l
	case p.Check.Reason == checkcmd.ReasonNoVerdict:
		return lin("sin veredicto todavia")
	}
	return lin("check ").masCon("ROJO", rojo).mas(": " + checkcmd.Motivo(p.Check))
}

// contenido1 is [1]'s content.
func (m *Modelo) contenido1() []linea {
	fijada := " sin tarjeta fijada"
	if m.fijada != "" {
		fijada = " fijada: " + m.fijada
	}
	p := m.proyecto
	if p == nil {
		return []linea{lin(" " + m.c.Proyecto), lin(" leyendo el proyecto..."), lin(""), lin(fijada)}
	}
	uno := " " + m.c.Proyecto + " · sin git"
	if p.Git {
		uno = " " + m.c.Proyecto + " · rama " + p.Rama
	}
	hoy := " hoy sin trabajos"
	if p.Hoy.Runs > 0 {
		hoy = fmt.Sprintf(" hoy %.2f USD en %d trabajos", p.Hoy.CostoUSD, p.Hoy.Runs)
		if p.Hoy.SinCosto > 0 {
			hoy += fmt.Sprintf(" · %d sin dato", p.Hoy.SinCosto)
		}
	}
	return []linea{lin(uno), append(lin(" "), m.lineaCheck()...), lin(hoy), lin(fijada)}
}

// estadoCheck is what [1] folded says after its title.
func (m *Modelo) estadoCheck() string {
	p := m.proyecto
	switch {
	case p == nil:
		return ""
	case p.Check.OK:
		return " · check VERDE"
	case p.Check.Reason == checkcmd.ReasonNoVerdict:
		return " · sin veredicto"
	}
	return " · check ROJO"
}

// ---------------------------------------------------------------- times

// hace is a time ago: "<n>s" below a minute, else the biggest whole unit
// among m, h and d.
func hace(d time.Duration) string {
	s := segundos(d)
	switch {
	case s < 60:
		return fmt.Sprintf("%ds", s)
	case s < 3600:
		return fmt.Sprintf("%dm", s/60)
	case s < 86400:
		return fmt.Sprintf("%dh", s/3600)
	}
	return fmt.Sprintf("%dd", s/86400)
}

// reloj is a duration as m:ss below an hour and h:mm:ss from an hour on.
func reloj(d time.Duration) string {
	s := segundos(d)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// segundos are the whole seconds of d (0 when negative).
func segundos(d time.Duration) int {
	if d < 0 {
		return 0
	}
	return int(d / time.Second)
}

// gasto is the card's cost with two decimals, or "sin dato de costo".
func gasto(s boardcmd.Spend) string {
	if s.CostUSD == nil {
		return "sin dato de costo"
	}
	return fmt.Sprintf("%.2f USD", *s.CostUSD)
}

// paso is "<stage> <step>/<steps>", or the stage alone when steps is 0.
func paso(stage string, step, steps int) string {
	if steps == 0 {
		return stage
	}
	return strings.TrimSpace(fmt.Sprintf("%s %d/%d", stage, step, steps))
}
