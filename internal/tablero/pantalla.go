package tablero

import (
	"fmt"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
)

// The smallest screen the TUI draws.
const (
	anchoMinimo = 60
	altoMinimo  = 15
)

const msgAgrandar = "agranda la terminal (minimo 60x15)"

// sangria is the margin of a card's second line.
var sangria = strings.Repeat(" ", 28)

// Pantalla is the screen now, as the package's doc describes it.
func (m *Modelo) Pantalla() []string {
	w, h := m.c.Ancho, m.c.Alto
	if h <= 0 {
		return nil
	}
	out := make([]string, h)
	if w < anchoMinimo || h < altoMinimo {
		out[0] = lin(msgAgrandar).dibujar(w, m.s.corte, false)
		return out
	}
	lineas := make([]linea, h)
	titulo, cuerpo := m.componer()
	lineas[0] = linCon(titulo, negrita)
	for i := 0; i < h-4 && i < len(cuerpo); i++ {
		lineas[1+i] = cuerpo[i]
	}
	lineas[h-3] = lin(strings.Repeat(m.s.separador, w))
	lineas[h-2] = m.barra1()
	lineas[h-1] = m.barra2()
	for i, l := range lineas {
		out[i] = l.dibujar(w, m.s.corte, m.c.Color)
	}
	return out
}

// componer is the title and the body (Alto-4 lines) of the current view.
func (m *Modelo) componer() (string, []linea) {
	switch v := m.vista; {
	case v == VistaMenu:
		titulo, _ := m.componerVista(m.menu.desde)
		return titulo, m.cuerpoMenu()
	case v == VistaFormulario:
		titulo, cuerpo := m.componerVista(m.form.desde)
		return titulo, m.sobreponer(cuerpo, m.cajaFormulario())
	case v == VistaConfirmacion:
		titulo, cuerpo := m.componerVista(m.conf.desde)
		return titulo, m.sobreponer(cuerpo, m.cajaConfirmacion())
	case v == VistaAyuda:
		base := m.ayudaDe
		if base == VistaMenu && m.menu != nil {
			base = m.menu.desde
		}
		titulo, _ := m.componerVista(base)
		return titulo, m.cuerpoAyuda()
	default:
		return m.componerVista(v)
	}
}

// componerVista is the title and body of the list or of a tab.
func (m *Modelo) componerVista(v Vista) (string, []linea) {
	if esPestana(v) && m.abierta != "" {
		return m.tituloTarjeta(), m.cuerpoTarjeta(v)
	}
	lineas, _ := m.cuerpoLista()
	h := m.altoCuerpo()
	ini := m.scrollLista
	if ini > len(lineas) {
		ini = len(lineas)
	}
	fin := ini + h
	if fin > len(lineas) {
		fin = len(lineas)
	}
	return m.tituloLista(), lineas[ini:fin]
}

// ---------------------------------------------------------------- the bars

func (m *Modelo) barra1() linea {
	modo := "NORMAL"
	if m.experto {
		modo = "EXPERTO"
	}
	if m.vista == VistaBusqueda {
		modo = "/" + string(m.filtro) + m.s.cursor
	}
	fijada := "sin tarjeta fijada"
	if m.fijada != "" {
		fijada = "tarjeta: " + m.fijada + " (hoom usar)"
	}
	l := lin(" " + modo + " · " + fijada)
	if msg, e := m.mensaje(); msg != "" {
		l = l.mas(" · ").masCon(msg, e)
	}
	return l
}

func (m *Modelo) barra2() linea {
	var hints []string
	for _, a := range AtajosDe(m.vista) {
		if a.Barra != "" {
			hints = append(hints, a.Barra)
		}
	}
	return lin(" " + strings.Join(hints, " · "))
}

// ---------------------------------------------------------------- the list

func (m *Modelo) tituloLista() string {
	n := len(m.todas())
	if n == 1 {
		return " hoom · " + m.c.Proyecto + " · 1 tarjeta"
	}
	return fmt.Sprintf(" hoom · %s · %d tarjetas", m.c.Proyecto, n)
}

// seccion is one section of the list.
type seccion struct {
	titulo  string
	hecho   bool // the section of the column Hecho
	tarjeta []*boardcmd.Card
}

// pasaFiltro: the card stays while the search filters.
func (m *Modelo) pasaFiltro(c *boardcmd.Card) bool {
	if m.vista != VistaBusqueda || len(m.filtro) == 0 {
		return true
	}
	q := plegar(string(m.filtro))
	return strings.Contains(plegar(c.Slug), q) || strings.Contains(plegar(c.Item.Titulo), q)
}

// secciones are the list's sections in order: the cards that need your
// decision, the ones running, then one per column of the board.
func (m *Modelo) secciones() []seccion {
	decision := seccion{titulo: m.s.decisionTitulo}
	curso := seccion{titulo: "EN CURSO"}
	var columnas []seccion
	for i := range m.tablero.Columns {
		col := &m.tablero.Columns[i]
		sc := seccion{hecho: col.ID == boardcmd.ColHecho}
		for j := range col.Cards {
			c := &col.Cards[j]
			if !m.pasaFiltro(c) {
				continue
			}
			switch {
			case c.NeedsDecision:
				decision.tarjeta = append(decision.tarjeta, c)
			case c.Running != nil:
				curso.tarjeta = append(curso.tarjeta, c)
			default:
				sc.tarjeta = append(sc.tarjeta, c)
			}
		}
		sc.titulo = fmt.Sprintf("%s %d", strings.ToUpper(col.Name), len(sc.tarjeta))
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
		if len(sc.tarjeta) > 0 || sc.hecho {
			out = append(out, sc)
		}
	}
	return out
}

// elegible is a line of the list the selection can be on.
type elegible struct {
	slug  string
	hecho bool // Hecho's line
}

func (m *Modelo) elegibles() []elegible {
	if !m.leido || len(m.todas()) == 0 {
		return nil
	}
	var out []elegible
	for _, sc := range m.secciones() {
		if sc.hecho {
			out = append(out, elegible{hecho: true})
			if !m.hechoAbierto {
				continue
			}
		}
		for _, c := range sc.tarjeta {
			out = append(out, elegible{slug: c.Slug})
		}
	}
	return out
}

// lugar is where a line that can be chosen is in the list's body: its first
// line and its last one (its second line).
type lugar struct{ ini, fin int }

// cuerpoLista is the whole body of the list, and the place of each line
// that can be chosen (in the order of elegibles).
func (m *Modelo) cuerpoLista() ([]linea, []lugar) {
	if !m.leido {
		return []linea{lin(" leyendo el tablero...")}, nil
	}
	if len(m.todas()) == 0 {
		return []linea{lin(` no hay tarjetas: hoom item add "<titulo>"`)}, nil
	}
	var out []linea
	var pos []lugar
	idx := 0
	for _, sc := range m.secciones() {
		if sc.hecho {
			pos = append(pos, lugar{len(out), len(out)})
			out = append(out, linCon(" "+sc.titulo, negrita))
			idx++
			if !m.hechoAbierto {
				continue
			}
		} else {
			out = append(out, linCon(" "+sc.titulo, negrita))
		}
		for _, c := range sc.tarjeta {
			ini := len(out)
			sel := !m.sinSeleccion && idx == m.selIdx
			out = append(out, m.lineaTarjeta(c, sel))
			if l, ok := m.segundaLinea(c); ok {
				out = append(out, l)
			}
			pos = append(pos, lugar{ini, len(out) - 1})
			idx++
		}
	}
	return out, pos
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

// etiqueta is "<label>: <detail>", the label alone without a detail.
func etiqueta(s boardcmd.Segment) string {
	if s.Detail == "" {
		return s.Label
	}
	return s.Label + ": " + s.Detail
}

// lineaTarjeta is a card's line of the list.
func (m *Modelo) lineaTarjeta(c *boardcmd.Card, sel bool) linea {
	marca := " "
	if sel {
		marca = m.s.sel
	}
	prefijo := " " + marca + " " + rellenar(c.Slug, 24) + " "
	lugar := m.c.Ancho - celdas(prefijo)
	if m.experto {
		var partes []string
		for i, s := range c.Meter {
			partes = append(partes, m.signo(c, i)+" "+etiqueta(s))
		}
		return lin(prefijo + strings.Join(partes, "  "))
	}
	signos := m.tira(c)
	if len(signos) > lugar {
		n := lugar - 1
		if n < 0 {
			n = 0
		}
		return lin(prefijo + strings.Join(signos[:n], "") + m.s.corte)
	}
	l := prefijo + strings.Join(signos, "")
	if resto := lugar - len(signos) - 2; resto >= 8 && c.Plain != "" {
		l += "  " + c.Plain
	}
	return lin(l)
}

// segundaLinea is the second line of a card with running or interrupted.
func (m *Modelo) segundaLinea(c *boardcmd.Card) (linea, bool) {
	if r := c.Running; r != nil {
		partes := []string{rolDe(r.Role)}
		if p := paso(r.Stage, r.Step, r.Steps); p != "" {
			partes = append(partes, p)
		}
		t := reloj(m.ahora.Sub(r.StartedAt))
		if r.TimeoutS != 0 {
			t += " de " + reloj(time.Duration(r.TimeoutS)*time.Second)
		}
		partes = append(partes, t)
		if r.LastEventAt != nil {
			partes = append(partes, fmt.Sprintf("%s hace %ds", m.s.narro, segundos(m.ahora.Sub(*r.LastEventAt))))
		}
		partes = append(partes, gasto(c.Spend))
		return lin(sangria + strings.Join(partes, " · ")), true
	}
	if i := c.Interrupted; i != nil {
		return linCon(sangria+fmt.Sprintf("interrumpido en el paso %d de %d (%s): el %s ya no corre",
			i.Step, i.Steps, i.Stage, rolDe(i.Role)), rojo), true
	}
	return nil, false
}

// paso is "<stage> <step>/<steps>", or the stage alone when steps is 0.
func paso(stage string, step, steps int) string {
	if steps == 0 {
		return stage
	}
	return strings.TrimSpace(fmt.Sprintf("%s %d/%d", stage, step, steps))
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

// ---------------------------------------------------------------- the open card

func (m *Modelo) tituloTarjeta() string {
	c := m.tarjeta(m.abierta)
	if c == nil {
		return " " + m.abierta
	}
	return " " + c.Slug + " · " + c.Item.Titulo
}

var nombresPestana = map[Vista]string{
	VistaResumen: "Resumen", VistaSpec: "Spec", VistaSobre: "Sobre", VistaHallazgos: "Hallazgos", VistaHistoria: "Historia",
}

func (m *Modelo) cuerpoTarjeta(v Vista) []linea {
	var tabs []string
	for i, p := range Pestanas {
		t := fmt.Sprintf("%d %s", i+1, nombresPestana[p])
		if p == v {
			t = "[" + t + "]"
		}
		tabs = append(tabs, t)
	}
	out := []linea{lin(" " + strings.Join(tabs, "  "))}
	contenido := m.contenido(v)
	ini := m.scrolls[v]
	if ini > len(contenido) {
		ini = len(contenido)
	}
	fin := ini + m.alto()
	if fin > len(contenido) {
		fin = len(contenido)
	}
	return append(out, contenido[ini:fin]...)
}

// contenido is the whole content of a tab of the open card.
func (m *Modelo) contenido(v Vista) []linea {
	c := m.tarjeta(m.abierta)
	if c == nil {
		return nil
	}
	if v == VistaResumen {
		return m.resumen(c)
	}
	d := m.detalle
	if d == nil {
		return []linea{lin(" leyendo la tarjeta...")}
	}
	if d.Err != nil {
		return []linea{linCon(" no se pudo leer la tarjeta: "+d.Err.Error(), rojo)}
	}
	switch v {
	case VistaSpec:
		lineas, _ := m.spec()
		return lineas
	case VistaSobre:
		return m.sobre(c)
	case VistaHallazgos:
		return m.lineasHallazgos()
	case VistaHistoria:
		return m.historia()
	}
	return nil
}

func (m *Modelo) resumen(c *boardcmd.Card) []linea {
	out := []linea{lin(" columna: " + c.ColumnName), lin(" " + c.Plain)}
	if m.experto {
		out = append(out, lin(" siguiente: "+c.Next))
		for _, x := range c.Missing {
			out = append(out, lin(" missing: "+x))
		}
	}
	falta := false
	for _, s := range c.Meter {
		if s.State == boardcmd.SegFalta {
			out = append(out, lin(" falta: "+etiqueta(s)))
			falta = true
		}
	}
	if !falta {
		out = append(out, lin(" falta: nada"))
	}
	out = append(out, lin(" tira: "+strings.Join(m.tira(c), "")))
	for i, s := range c.Meter {
		out = append(out, lin("   "+m.signo(c, i)+" "+etiqueta(s)))
	}
	g := " gasto: " + gasto(c.Spend)
	if c.Spend.BudgetUSD != nil {
		g += fmt.Sprintf(" de %.2f USD", *c.Spend.BudgetUSD)
	}
	out = append(out, lin(g))
	out = append(out, lin(" escribio: "+sinDato(c.Providers.Writer)+" · reviso: "+sinDato(c.Providers.Reviewer)))
	if c.Red != nil {
		motivo := c.Red.Plain
		if m.experto {
			motivo = c.Red.Reason
		}
		out = append(out, linCon(" rojo: "+motivo, rojo))
	}
	for _, p := range c.Doctor {
		que := p.Plain
		if que == "" {
			que = p.What
		}
		out = append(out, lin(" doctor: "+que+": "+p.Action))
	}
	return out
}

func sinDato(s string) string {
	if s == "" {
		return "sin dato"
	}
	return s
}

func tieneSpec(d *Detalle) bool { return d.Detalle.Spec.Exists || d.Detalle.Spec.Markdown != "" }

// rutaSpec is the open card's spec, relative to the project root.
func (m *Modelo) rutaSpec() string {
	d := m.detalle
	if d.Detalle.Paths.Spec != "" {
		return d.Detalle.Paths.Spec
	}
	p := d.Detalle.Spec.Path
	if p == "" {
		if c := m.tarjeta(m.abierta); c != nil {
			p = c.Evidence.Spec
		}
	}
	if absoluta(p) {
		// not under the dir: path.Join would make it relative, and o would
		// open a file the evidence never named
		return p
	}
	return path.Join(m.dirEvidencia(), p)
}

// absoluta says whether p, a path of the evidence, is absolute on some
// system, whichever one reads it: rooted ("/x", "\x", a UNC "\\h\s") or
// with a drive ("C:x", "C:/x"). Evidence written on Windows is read on Unix
// too, and joined under a dir it would stop being absolute.
func absoluta(p string) bool {
	q := strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(q, "/") {
		return true
	}
	if len(q) >= 2 && q[1] == ':' && 'a' <= q[0]|0x20 && q[0]|0x20 <= 'z' {
		return true
	}
	return filepath.IsAbs(filepath.FromSlash(p)) || filepath.VolumeName(filepath.FromSlash(p)) != ""
}

// fueraDelProyecto says whether p, a path of the evidence relative to the
// root, leaves the project on some system: an absolute one (absoluta), or
// one whose ".." climbs out with / or \ as the separator.
func fueraDelProyecto(p string) bool {
	return absoluta(p) || !filepath.IsLocal(filepath.FromSlash(strings.ReplaceAll(p, `\`, "/")))
}

// dirEvidencia is the open card's evidence dir, relative to the root: the
// one of the detail the tabs show, else the board's.
func (m *Modelo) dirEvidencia() string {
	if m.detalle != nil && m.detalle.Detalle.Card.Evidence.Dir != "" {
		return m.detalle.Detalle.Card.Evidence.Dir
	}
	if c := m.tarjeta(m.abierta); c != nil {
		return c.Evidence.Dir
	}
	return ""
}

// spec is the Spec tab's lines and the indexes of the criteria without a
// test.
func (m *Modelo) spec() ([]linea, []int) {
	d := m.detalle
	if !tieneSpec(d) {
		p := d.Detalle.Paths.Spec
		if p == "" {
			p = d.Detalle.Spec.Path
		}
		if p == "" {
			if c := m.tarjeta(m.abierta); c != nil {
				p = c.Evidence.Spec
			}
		}
		return []linea{lin(" sin spec: " + p)}, nil
	}
	sin := map[string]bool{}
	ids := map[string]string{} // id -> traced_by
	for _, u := range m.untraced() {
		sin[u] = true
		ids[u] = ""
	}
	for _, ct := range d.Detalle.Criteria {
		ids[ct.ID] = ct.TracedBy
	}
	var out []linea
	var sinPrueba []int
	texto := strings.TrimSuffix(d.Detalle.Spec.Markdown, "\n")
	for i, l := range strings.Split(texto, "\n") {
		l = strings.TrimRight(l, "\r")
		margen := "  "
		if id, ok := criterioDe(l, ids); ok {
			switch {
			case sin[id]:
				margen = m.s.sinPrueba + " "
				sinPrueba = append(sinPrueba, i)
			case ids[id] == boardcmd.TracedComando:
				margen = m.s.porComando + " "
			default:
				margen = m.s.conPrueba + " "
			}
		}
		out = append(out, lin(margen+l))
	}
	return out, sinPrueba
}

// untraced are the open card's criteria without a test.
func (m *Modelo) untraced() []string {
	var out []string
	if c := m.tarjeta(m.abierta); c != nil {
		out = append(out, c.Evidence.Untraced...)
	}
	if m.detalle != nil {
		out = append(out, m.detalle.Detalle.Card.Evidence.Untraced...)
	}
	return out
}

// criterioDe is the id of the criterion line l is ("- <id>:" trimmed).
func criterioDe(l string, ids map[string]string) (string, bool) {
	t := strings.TrimSpace(l)
	if !strings.HasPrefix(t, "- ") {
		return "", false
	}
	resto := t[2:]
	i := strings.Index(resto, ":")
	if i <= 0 {
		return "", false
	}
	id := resto[:i]
	if _, ok := ids[id]; !ok {
		return "", false
	}
	return id, true
}

func (m *Modelo) sinPrueba() []int {
	if m.detalle == nil || m.detalle.Err != nil || !tieneSpec(m.detalle) {
		return nil
	}
	_, sin := m.spec()
	return sin
}

func (m *Modelo) sobre(c *boardcmd.Card) []linea {
	d := m.detalle
	var partes []string
	switch {
	case c.Running != nil:
		r := c.Running
		partes = append(partes, rolDe(r.Role))
		if p := paso(r.Stage, r.Step, r.Steps); p != "" {
			partes = append(partes, p)
		}
		t := reloj(m.ahora.Sub(r.StartedAt))
		if r.TimeoutS != 0 {
			t += " de " + reloj(time.Duration(r.TimeoutS)*time.Second)
		}
		partes = append(partes, t)
	case len(d.Detalle.Work) > 0:
		w := d.Detalle.Work[0]
		partes = append(partes, rolDe(w.Role))
		if p := paso(w.Stage, w.Step, w.Steps); p != "" {
			partes = append(partes, p)
		}
		if w.DurationMS != nil {
			partes = append(partes, reloj(time.Duration(*w.DurationMS)*time.Millisecond))
		}
		if w.Status != "" {
			partes = append(partes, w.Status)
		}
	default:
		return []linea{lin(" sin sobres todavia")}
	}
	ev := d.Sobre.Eventos
	if n := len(ev); n > 0 {
		partes = append(partes, fmt.Sprintf("%s evento hace %ds", m.s.ultimo, segundos(m.ahora.Sub(ev[n-1].TS))))
	}
	out := []linea{lin(" " + strings.Join(partes, " · "))}
	if d.Sobre.Err != nil {
		out = append(out, linCon(" no se pudo leer el sobre: "+d.Sobre.Err.Error(), rojo))
	}
	for _, e := range ev {
		l := " " + e.TS.UTC().Format("15:04:05") + " " + e.Kind
		if e.Detail != "" {
			l += " " + unaLinea(e.Detail)
		}
		out = append(out, lin(l))
	}
	return out
}

// hallazgos are the task's findings, the open ones first.
func (m *Modelo) hallazgos() []finding.Item {
	if m.detalle == nil || m.detalle.Err != nil {
		return nil
	}
	var abiertos, otros []finding.Item
	for _, f := range m.detalle.Detalle.Findings {
		if f.Status == "abierto" {
			abiertos = append(abiertos, f)
		} else {
			otros = append(otros, f)
		}
	}
	return append(abiertos, otros...)
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

func (m *Modelo) lineasHallazgos() []linea {
	hs := m.hallazgos()
	if len(hs) == 0 {
		return []linea{lin(" sin hallazgos")}
	}
	var out []linea
	for i, f := range hs {
		marca := " "
		if i == m.hallazgo {
			marca = m.s.sel
		}
		archivo, n := archivoYLinea(f)
		donde := archivo
		switch {
		case archivo == "":
			donde = "-"
		case n > 0:
			donde += ":" + strconv.Itoa(n)
		}
		out = append(out, lin(fmt.Sprintf(" %s %s %s %s %s", marca, f.Status, f.Severity, donde, unaLinea(f.Description))))
	}
	return out
}

func (m *Modelo) historia() []linea {
	es := m.detalle.Historia.Entries
	if len(es) == 0 {
		return []linea{lin(" sin historia todavia")}
	}
	var out []linea
	for _, e := range es {
		que := e.Plain
		if m.experto {
			que = e.Summary
		}
		out = append(out, lin(" "+e.At.UTC().Format("2006-01-02 15:04")+" "+e.Who+" "+unaLinea(que)))
	}
	return out
}

// ---------------------------------------------------------------- the menu and the help

func (m *Modelo) cuerpoMenu() []linea {
	c := m.tarjeta(m.menu.slug)
	if c == nil {
		return nil
	}
	out := []linea{lin(" Acciones · " + c.Slug)}
	for i, a := range m.accionesMenu(c) {
		marca := " "
		if i == m.menu.sel {
			marca = m.s.sel
		}
		tecla, verbo := " ", "sin verbo en la terminal"
		fila, conVerbo := filaDeAccion(a.ID)
		if conVerbo {
			tecla = fila.Muestra
			verbo = strings.ReplaceAll(fila.Verbo, "<slug>", c.Slug)
		}
		texto := " " + marca + " " + tecla + " " + a.Label + "  " + verbo
		if !a.Enabled {
			texto += "  (" + a.Why + ")"
		}
		if !a.Enabled || !conVerbo {
			out = append(out, linCon(texto, gris))
		} else {
			out = append(out, lin(texto))
		}
	}
	return out
}

func (m *Modelo) cuerpoAyuda() []linea {
	out := []linea{lin(" Teclas de " + string(m.ayudaDe))}
	filas := AtajosDe(m.ayudaDe)
	texto := func(a Atajo, ancho int) string {
		l := rellenar(a.Muestra, ancho) + a.Hace
		if a.Verbo != "" {
			l += "  " + a.Verbo
		}
		return l
	}
	alto := m.c.Alto - 5 // the body without the header line
	if len(filas) <= alto+1 || alto <= 0 {
		if len(filas) > alto {
			out = nil // one row more than the lines: the header gives way
		}
		for _, a := range filas {
			out = append(out, lin(" "+texto(a, 14)))
		}
		return out
	}
	// more rows than lines: the rows flow down and then across, in columns,
	// so every key of the view stays on the screen
	cols := (len(filas) + alto - 1) / alto
	alto = (len(filas) + cols - 1) / cols // the columns balanced
	ancho := (m.c.Ancho - 1) / cols
	muestra := 0
	for _, a := range filas {
		if n := celdas(a.Muestra); n > muestra {
			muestra = n
		}
	}
	muestra += 2
	for i := 0; i < alto && i < len(filas); i++ {
		l := " "
		for c := 0; c < cols; c++ {
			k := c*alto + i
			if k >= len(filas) {
				break
			}
			celda := cortar(texto(filas[k], muestra), ancho-1, m.s.corte)
			if c < cols-1 {
				celda = rellenar(celda, ancho)
			}
			l += celda
		}
		out = append(out, lin(l))
	}
	return out
}

// ---------------------------------------------------------------- the boxes

// interior is the box's inner width.
func (m *Modelo) interior() int { return m.c.Ancho - 6 }

// caja is a box of Ancho-2 cells: its title on the top line and inner as
// its lines.
func (m *Modelo) caja(titulo string, inner []linea) []linea {
	w := m.c.Ancho - 2
	wi := m.interior()
	arriba := m.s.esqSupIzq + " " + cortar(titulo, w-4, m.s.corte) + " "
	if n := w - 1 - celdas(arriba); n > 0 {
		arriba += strings.Repeat(m.s.horizontal, n)
	}
	arriba += m.s.esqSupDer
	out := []linea{lin(" " + arriba)}
	for _, l := range inner {
		c := l.cortada(wi, m.s.corte)
		resto := wi - celdas(c.plano())
		fila := linea{{texto: " " + m.s.lado + " "}}
		fila = append(fila, c...)
		fila = fila.mas(strings.Repeat(" ", resto) + " " + m.s.lado)
		out = append(out, fila)
	}
	out = append(out, lin(" "+m.s.esqInfIzq+strings.Repeat(m.s.horizontal, w-2)+m.s.esqInfDer))
	return out
}

// sobreponer puts the box over the body, from its first line.
func (m *Modelo) sobreponer(cuerpo, caja []linea) []linea {
	h := m.altoCuerpo()
	out := make([]linea, h)
	copy(out, cuerpo)
	for i := 0; i < len(caja) && i < h; i++ {
		out[i] = caja[i]
	}
	return out
}

// marca is the two-cell mark of a form field.
func (m *Modelo) marca(foco bool) string {
	if foco {
		return m.s.sel + " "
	}
	return "  "
}

// conCursor is the field's text with the text cursor at its cursor.
func (m *Modelo) conCursor(c *campo) string {
	return string(c.texto[:c.cursor]) + m.s.cursor + string(c.texto[c.cursor:])
}

func (m *Modelo) cajaFormulario() []linea {
	f := m.form
	foco := f.enfocado()
	// the provider
	nombre := ""
	if f.opcion >= 0 && f.opcion < len(f.accion.Providers) {
		nombre = f.accion.Providers[f.opcion].Name
	}
	prov := m.s.flechaIzq + " " + nombre + " " + m.s.flechaDer
	if f.reanudar {
		prov = nombre
	}
	var noPueden []linea
	for _, o := range f.accion.Providers {
		if !o.OK {
			noPueden = append(noPueden, linCon(strings.Repeat(" ", 17)+o.Name+": "+o.Why, gris))
		}
	}
	texto := func(id int, vacio, sufijo string) string {
		c := f.campo(id)
		switch {
		case foco == id:
			return m.conCursor(c) + sufijo
		case c.valor() == "" && vacio != "":
			return vacio
		}
		return c.valor() + sufijo
	}
	campos := []linea{
		lin(m.marca(foco == campoModelo) + rellenar("Modelo", 15) + texto(campoModelo, "del provider", "")),
		lin(m.marca(foco == campoEsfuerzo) + rellenar("Esfuerzo", 15) + texto(campoEsfuerzo, "del provider", "")),
		lin(m.marca(foco == campoPresupuesto) + rellenar("Presupuesto", 15) + texto(campoPresupuesto, "sin tope", " USD")),
	}
	if f.revisor {
		// shown as the action brings it, not a field: hoom pedir reviewer
		// takes it from review.timeout and has no --timeout
		campos = append(campos, linCon(m.marca(false)+rellenar("Tiempo limite", 15)+texto(campoTiempo, "de review.timeout", "")+" (de review.timeout)", gris))
	} else {
		campos = append(campos, lin(m.marca(foco == campoTiempo)+rellenar("Tiempo limite", 15)+texto(campoTiempo, "", "")))
	}
	verbo := lin(m.s.flecha + " " + lineaVerbo(f.args(), m.c.Windows))
	ayuda := lin("enter lanzar · tab campos · ctrl-e editor · esc cancelar")

	armar := func(c compacto) []linea {
		linProv := lin(m.marca(foco == campoProvider) + rellenar("Provider", 15) + prov)
		if c.sinNoPueden && len(noPueden) > 0 {
			linProv = linProv.masCon(fmt.Sprintf("  (%d no pueden)", len(noPueden)), gris)
		}
		inner := []linea{linProv}
		if !c.sinNoPueden {
			inner = append(inner, noPueden...)
		}
		inner = append(inner, campos...)
		if !f.revisor {
			if !c.sinBlancos {
				inner = append(inner, nil)
			}
			etiquetaPedido := lin(m.marca(foco == campoPedido) + "Pedido")
			if f.pedidoVacio {
				etiquetaPedido = linCon(m.marca(foco == campoPedido)+"Pedido", rojo)
			}
			inner = append(inner, etiquetaPedido)
			inner = append(inner, m.lineasPedido(c.pedido)...)
		}
		if !c.sinBlancos {
			inner = append(inner, nil)
		}
		inner = append(inner, verbo)
		if !c.sinTeclas {
			inner = append(inner, ayuda)
		}
		return inner
	}
	// the first step that fits in the body; the last one when none does
	cabe := m.altoCuerpo() - bordesCaja
	var inner []linea
	for _, c := range pasosFormulario {
		if inner = armar(c); len(inner) <= cabe {
			break
		}
	}
	return m.caja(f.accion.Label+" · "+f.slug, inner)
}

// compacto is what the form's box gives up to fit in the body.
type compacto struct {
	sinBlancos  bool // its empty lines
	sinNoPueden bool // the providers that cannot, one per line: the Provider line counts them, and a chosen one shows its why
	sinTeclas   bool // the keys line: the bar has them
	pedido      int  // how many of the pedido's lines it shows (the one with the cursor among them)
}

// lineasPedidoForm is how many lines of the pedido the form shows when it
// fits.
const lineasPedidoForm = 4

// pasosFormulario is the order in which the form's box gives up what does
// not fit (doc.go, "A box taller than the body"): the empty lines, the
// providers that cannot, the keys line and then the pedido's lines down to
// one. The → line always stays.
var pasosFormulario = []compacto{
	{pedido: lineasPedidoForm},
	{sinBlancos: true, pedido: lineasPedidoForm},
	{sinBlancos: true, sinNoPueden: true, pedido: lineasPedidoForm},
	{sinBlancos: true, sinNoPueden: true, sinTeclas: true, pedido: lineasPedidoForm},
	{sinBlancos: true, sinNoPueden: true, sinTeclas: true, pedido: 3},
	{sinBlancos: true, sinNoPueden: true, sinTeclas: true, pedido: 2},
	{sinBlancos: true, sinNoPueden: true, sinTeclas: true, pedido: 1},
}

// bordesCaja are the lines of a box's top and bottom borders.
const bordesCaja = 2

// altoCuerpo is how many lines the body has: the screen less the title
// line and the separator, the bar and the keys line under it.
func (m *Modelo) altoCuerpo() int {
	return m.c.Alto - 4
}

// lineasPedido are the pedido's lines in the form: wrapped hard to the
// inner width, the last n (the n that end at the cursor's line when it is
// focused; lineasPedidoForm when the box fits).
func (m *Modelo) lineasPedido(n int) []linea {
	f := m.form
	c := f.campo(campoPedido)
	wi := m.interior()
	var lineas []string
	fin := 0
	if f.enfocado() == campoPedido {
		antes := envolverDuro(string(c.texto[:c.cursor])+m.s.cursor, wi)
		fin = len(antes)
		lineas = envolverDuro(m.conCursor(c), wi)
	} else {
		lineas = envolverDuro(c.valor(), wi)
		fin = len(lineas)
	}
	if fin > len(lineas) {
		fin = len(lineas)
	}
	ini := fin - n
	if ini < 0 {
		ini = 0
	}
	var out []linea
	for _, l := range lineas[ini:fin] {
		out = append(out, lin(l))
	}
	return out
}

func (m *Modelo) cajaConfirmacion() []linea {
	cf := m.conf
	cabe := m.altoCuerpo() - bordesCaja
	// as every box (doc.go, "A box taller than the body"), the empty line
	// is the first thing left out, and only when the box does not fit
	armar := func(vacia bool, paths []string) []linea {
		var inner []linea
		if cf.firma {
			for _, l := range envolverPalabras(cf.texto, m.interior()) {
				inner = append(inner, lin(l))
			}
			if vacia {
				inner = append(inner, nil)
			}
			botones := "[ Cancelar ]   Firmar "
			if cf.firmar {
				botones = "  Cancelar   [ Firmar ]"
			}
			return append(inner, lin(botones))
		}
		inner = append(inner, lin("Se guarda en git:"))
		for _, p := range paths {
			inner = append(inner, lin(p))
		}
		if vacia {
			inner = append(inner, nil)
		}
		return append(inner, lin("enter guardar · esc cancelar"))
	}
	inner := armar(true, cf.paths)
	if len(inner) > cabe {
		inner = armar(false, cf.paths)
	}
	if !cf.firma && len(inner) > cabe {
		// the paths that still do not fit say how many they are; the
		// header and the keys line always stay
		const sinRutas = 2
		if n := cabe - sinRutas; n >= 1 && len(cf.paths) > n {
			paths := append(cf.paths[:n-1:n-1], fmt.Sprintf("y %d rutas mas", len(cf.paths)-(n-1)))
			inner = armar(false, paths)
		}
	}
	return m.caja(cf.accion.Label+" · "+cf.slug, inner)
}

// confirmacionCabe says whether the confirmation's box fits in the body
// whole: a signature is never signed with part of its text out of view.
func (m *Modelo) confirmacionCabe() bool {
	return len(m.cajaConfirmacion()) <= m.altoCuerpo()
}
