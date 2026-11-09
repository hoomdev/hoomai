package tablero

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/modelos"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
	"github.com/hoomdev/hoomai/internal/valores"
)

// The fields of the Pedir modal, in tab order.
const (
	campoPedido = iota
	campoProvider
	campoModelo
	campoEsfuerzo
	campoTope
	campoTiempo
	campoBotones
)

// selectores are the selector fields, in order.
var selectores = []int{campoProvider, campoModelo, campoEsfuerzo, campoTope, campoTiempo}

// texto is a text being edited: the Pedido, or a selector turned text.
type texto struct {
	r      []rune
	cursor int
}

func (t *texto) valor() string { return string(t.r) }

func (t *texto) poner(s string) {
	t.r = []rune(s)
	t.cursor = len(t.r)
}

// editar applies a key of a text field; it says whether it was one.
func (t *texto) editar(k Tecla) bool {
	switch k {
	case TeclaIzq:
		if t.cursor > 0 {
			t.cursor--
		}
	case TeclaDer:
		if t.cursor < len(t.r) {
			t.cursor++
		}
	case TeclaBorrar:
		if t.cursor > 0 {
			t.r = append(t.r[:t.cursor-1], t.r[t.cursor:]...)
			t.cursor--
		}
	case TeclaCtrlU:
		t.poner("")
	default:
		if !imprimible(k) {
			return false
		}
		r := []rune(string(k))
		t.r = append(t.r[:t.cursor], append(r, t.r[t.cursor:]...)...)
		t.cursor += len(r)
	}
	return true
}

// pedir is the open Pedir modal.
type pedir struct {
	slug     string
	accion   boardcmd.Action
	verbo    []string // {"pedir", <rol>, <slug>} or {"reanudar", <slug>}
	revisor  bool     // no Pedido, and its Tiempo limite is review.timeout's
	reanudar bool     // the provider is the session's

	pedido      texto
	pedidoVacio bool

	prov, provPropuesto int       // the chosen provider option and the proposed one (-1: none)
	valor, propuesto    [7]string // the chosen and the proposed value of Modelo, Esfuerzo, Tope, Tiempo
	escritos            [7][]string

	foco  int
	boton int // 0: the action, 1: Cancelar

	lista     *lista
	otro      *texto // a selector turned text by otro…
	otroCampo int
}

// lista is a selector's open list of options.
type lista struct {
	campo  int
	idx    int
	scroll int
}

// lugar is the Pedir modal's Lugar now.
func (p *pedir) lugar() Lugar {
	switch {
	case p.lista != nil:
		return LugarPedirLista
	case p.otro != nil:
		return LugarPedirTexto
	case p.foco == campoPedido:
		return LugarPedirPedido
	case p.foco == campoBotones:
		return LugarPedirBotones
	}
	return LugarPedirSelector
}

func (m *Modelo) abrirPedir(c *boardcmd.Card, a boardcmd.Action, verbo []string) {
	p := &pedir{
		slug:     c.Slug,
		accion:   a,
		verbo:    verbo,
		revisor:  a.ID == boardcmd.ActPedirReviewer,
		reanudar: a.ID == boardcmd.ActReanudar,
		prov:     -1,
	}
	for i, o := range a.Providers {
		if o.Default {
			p.prov = i
			break
		}
	}
	if p.prov < 0 {
		for i, o := range a.Providers {
			if o.OK {
				p.prov = i
				break
			}
		}
	}
	p.provPropuesto = p.prov
	p.propuesto[campoModelo] = a.Model
	p.propuesto[campoEsfuerzo] = a.Effort
	if a.BudgetUSD != nil {
		p.propuesto[campoTope] = numero(*a.BudgetUSD)
	}
	if a.TimeoutS > 0 {
		p.propuesto[campoTiempo] = valores.Duracion(time.Duration(a.TimeoutS) * time.Second)
	}
	p.valor = p.propuesto
	if !p.revisor {
		p.pedido.poner(a.Pedido)
		p.foco = campoPedido
	} else {
		p.foco = m.focosPedir(p)[0]
	}
	m.pedir = p
	m.modal = ModalPedir
}

// numero is a number of USD as the selectors write it.
func numero(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// provider is the chosen provider option.
func (p *pedir) provider() (boardcmd.ProviderOption, bool) {
	if p.prov < 0 || p.prov >= len(p.accion.Providers) {
		return boardcmd.ProviderOption{}, false
	}
	return p.accion.Providers[p.prov], true
}

func (p *pedir) nombreProvider() string {
	o, _ := p.provider()
	return o.Name
}

// opcion is one option of a selector.
type opcion struct {
	nombre string // as the selector and the list show it
	valor  string
	linea  string
	nota   bool // its line is a note, shown alone
	gris   bool
	why    string
	otro   bool
}

// gris says whether the selector campo is gray (it takes no value: its why
// goes in the origin column) and the value it shows then.
func (m *Modelo) gris(p *pedir, campo int) (bool, string, string) {
	prov := p.nombreProvider()
	switch campo {
	case campoProvider:
		if p.reanudar {
			return true, "la sesion es de " + prov, prov
		}
	case campoModelo:
		if !modelos.RecibeModelo(prov) {
			return true, "hoom no le pasa el modelo a " + prov, "del provider"
		}
	case campoEsfuerzo:
		if !modelos.RecibeEsfuerzo(prov) {
			return true, "hoom no le pasa el esfuerzo a " + prov, "del provider"
		}
	case campoTope:
		if o, _ := p.provider(); !o.Budget {
			return true, prov + " no acepta tope", "sin tope"
		}
	case campoTiempo:
		if p.revisor {
			return true, "de review.timeout", p.propuesto[campoTiempo]
		}
	}
	return false, "", ""
}

// opciones are the options of the selector campo.
func (m *Modelo) opciones(p *pedir, campo int) []opcion {
	prov := p.nombreProvider()
	var out []opcion
	en := map[string]bool{}
	agregar := func(o opcion) {
		if en[o.valor] && !o.otro {
			return
		}
		en[o.valor] = true
		out = append(out, o)
	}
	escritos := func(nombre func(string) string) {
		for _, e := range p.escritos[campo] {
			agregar(opcion{nombre: nombre(e), valor: e})
		}
		out = append(out, opcion{nombre: "otro" + m.s.puntos, otro: true})
	}
	switch campo {
	case campoProvider:
		for i, o := range p.accion.Providers {
			out = append(out, opcion{nombre: o.Name, valor: strconv.Itoa(i), gris: !o.OK, why: o.Why})
		}
		return out
	case campoModelo, campoEsfuerzo:
		origen, propio := p.accion.Origen.Model, p.accion.Model
		var lista []modelos.Opcion
		if campo == campoModelo {
			lista = modelos.Modelos(prov, m.c.Modelos[prov])
		} else {
			origen, propio = p.accion.Origen.Effort, p.accion.Effort
			if cat, ok := modelos.De(prov); ok {
				lista = cat.Esfuerzos
			}
		}
		defecto := opcion{nombre: "por defecto del provider", valor: "",
			linea: fmt.Sprintf("el que elige %s: su configuracion o tu cuenta", prov)}
		if origen == valores.OrigenHoomYAML && propio != "" {
			defecto.gris = true
			defecto.why = fmt.Sprintf("hoom.yaml fija %s para el %s", propio, rolDe(p.accion.Role))
		}
		agregar(defecto)
		for _, o := range lista {
			agregar(opcion{nombre: o.Nombre, valor: o.Nombre, linea: o.Linea})
		}
		if propio != "" && !en[propio] {
			o := opcion{nombre: propio, valor: propio, linea: "de hoom.yaml"}
			if p.prov != p.provPropuesto {
				o.linea, o.nota = fmt.Sprintf("%s no esta en la lista de %s: puede rechazarlo", propio, prov), true
			}
			agregar(o)
		}
		escritos(func(s string) string { return s })
	case campoTope:
		o, _ := p.provider()
		nombre := func(v string) string {
			if v == "" {
				return "sin tope"
			}
			return v + " USD"
		}
		agregar(opcion{nombre: nombre(p.propuesto[campoTope]), valor: p.propuesto[campoTope]})
		for _, v := range []float64{1, 2, 5, 10, 20} {
			agregar(opcion{nombre: nombre(numero(v)), valor: numero(v)})
		}
		escritos(nombre)
		for i := range out {
			if v, err := strconv.ParseFloat(out[i].valor, 64); err == nil && !out[i].otro && v < o.MinBudgetUSD {
				out[i].gris = true
				out[i].why = fmt.Sprintf("%s pide al menos %s USD", prov, numero(o.MinBudgetUSD))
			}
		}
	case campoTiempo:
		if v := p.propuesto[campoTiempo]; v != "" {
			agregar(opcion{nombre: v, valor: v})
		}
		for _, d := range []time.Duration{15 * time.Minute, 30 * time.Minute, 45 * time.Minute, time.Hour, 2 * time.Hour} {
			v := valores.Duracion(d)
			agregar(opcion{nombre: v, valor: v})
		}
		escritos(func(s string) string { return s })
	}
	return out
}

// elegida is the index of the chosen option of the selector campo (-1:
// none).
func (m *Modelo) elegida(p *pedir, campo int, ops []opcion) int {
	v := p.valor[campo]
	if campo == campoProvider {
		v = strconv.Itoa(p.prov)
	}
	for i, o := range ops {
		if !o.otro && o.valor == v {
			return i
		}
	}
	return -1
}

// elegir puts the option o on the selector campo.
func (m *Modelo) elegirOpcion(p *pedir, campo int, o opcion) {
	if campo == campoProvider {
		i, _ := strconv.Atoi(o.valor)
		m.cambiarProvider(p, i)
		return
	}
	p.valor[campo] = o.valor
}

// cambiarProvider chooses the provider option i, with the rules of
// CA-570.
func (m *Modelo) cambiarProvider(p *pedir, i int) {
	if i == p.prov {
		return
	}
	p.prov = i
	for _, c := range []int{campoModelo, campoEsfuerzo} {
		origen := p.accion.Origen.Model
		if c == campoEsfuerzo {
			origen = p.accion.Origen.Effort
		}
		switch {
		case origen == valores.OrigenHoomYAML && p.propuesto[c] != "":
			// hoom.yaml's value stays chosen
		case i == p.provPropuesto:
			p.valor[c] = p.propuesto[c]
		default:
			p.valor[c] = ""
		}
	}
}

// focosPedir are the fields the focus goes over, in tab order: the gray
// selectors are skipped.
func (m *Modelo) focosPedir(p *pedir) []int {
	var out []int
	if !p.revisor {
		out = append(out, campoPedido)
	}
	for _, c := range selectores {
		if g, _, _ := m.gris(p, c); !g {
			out = append(out, c)
		}
	}
	return append(out, campoBotones)
}

// enPedir is a key of the Pedir modal.
func (m *Modelo) enPedir(l Lugar, t Tecla) []Orden {
	p := m.pedir
	switch l {
	case LugarPedirLista:
		ops := m.opciones(p, p.lista.campo)
		switch t {
		case TeclaArriba:
			p.lista.idx = max(0, p.lista.idx-1)
		case TeclaAbajo:
			p.lista.idx = min(len(ops)-1, p.lista.idx+1)
		case TeclaEsc:
			p.lista = nil
		case TeclaEnter:
			m.elegirDeLista(p, p.lista.idx)
		}
		return nil
	case LugarPedirTexto:
		switch t {
		case TeclaEnter:
			v := strings.TrimSpace(p.otro.valor())
			if v != "" {
				p.valor[p.otroCampo] = v
				ya := false
				for _, e := range p.escritos[p.otroCampo] {
					ya = ya || e == v
				}
				if !ya {
					p.escritos[p.otroCampo] = append(p.escritos[p.otroCampo], v)
				}
			}
			p.otro = nil
		case TeclaEsc:
			p.otro = nil
		default:
			p.otro.editar(t)
		}
		return nil
	}
	switch t {
	case TeclaEsc:
		m.cerrarModal()
		return nil
	case TeclaTab, TeclaShiftTab:
		fs := m.focosPedir(p)
		i := 0
		for j, f := range fs {
			if f == p.foco {
				i = j
			}
		}
		d := 1
		if t == TeclaShiftTab {
			d = len(fs) - 1
		}
		p.foco = fs[(i+d)%len(fs)]
		return nil
	case TeclaCtrlE:
		if !p.revisor {
			return []Orden{EditarPedido{Texto: p.pedido.valor()}}
		}
		return nil
	}
	switch l {
	case LugarPedirPedido:
		if t == TeclaEnter {
			p.foco = campoBotones
			return nil
		}
		if p.pedido.editar(t) {
			p.pedidoVacio = false
		}
	case LugarPedirSelector:
		switch t {
		case TeclaIzq, TeclaDer:
			d := 1
			if t == TeclaIzq {
				d = -1
			}
			m.cambiarOpcion(p, p.foco, d)
		case TeclaEnter, TeclaEspacio:
			ops := m.opciones(p, p.foco)
			i := max(0, m.elegida(p, p.foco, ops))
			p.lista = &lista{campo: p.foco, idx: i}
		}
	case LugarPedirBotones:
		switch t {
		case TeclaIzq, TeclaDer:
			p.boton = 1 - p.boton
		case TeclaEnter:
			if p.boton == 1 {
				m.cerrarModal()
				return nil
			}
			return m.lanzarPedir()
		}
	}
	return nil
}

// cambiarOpcion moves the selector campo to the previous or next option,
// wrapping around and skipping the gray ones and otro….
func (m *Modelo) cambiarOpcion(p *pedir, campo, d int) {
	ops := m.opciones(p, campo)
	n := len(ops)
	if n == 0 {
		return
	}
	i := m.elegida(p, campo, ops)
	if i < 0 {
		i = 0
	}
	for k := 0; k < n; k++ {
		i = ((i+d)%n + n) % n
		if !ops[i].gris && !ops[i].otro {
			m.elegirOpcion(p, campo, ops[i])
			return
		}
	}
}

// elegirDeLista is enter (or a click) on the option i of the open list.
func (m *Modelo) elegirDeLista(p *pedir, i int) {
	ops := m.opciones(p, p.lista.campo)
	if i < 0 || i >= len(ops) || ops[i].gris {
		return
	}
	campo := p.lista.campo
	p.lista = nil
	if ops[i].otro {
		p.otro = &texto{}
		p.otroCampo = campo
		return
	}
	m.elegirOpcion(p, campo, ops[i])
}

// argsPedir are the order's Args: the verb, -m and the pedido (not for the
// reviewer), then only the flags whose chosen value differs from the
// proposal.
func (m *Modelo) argsPedir(p *pedir) []string {
	out := append([]string{}, p.verbo...)
	if !p.revisor {
		out = append(out, "-m", strings.TrimSpace(p.pedido.valor()))
	}
	if !p.reanudar && p.prov != p.provPropuesto && p.prov >= 0 {
		out = append(out, "--provider", p.nombreProvider())
	}
	flag := func(campo int, nombre string) {
		if g, _, _ := m.gris(p, campo); g {
			return
		}
		v := p.valor[campo]
		if v == "" || v == p.propuesto[campo] {
			return
		}
		out = append(out, nombre, v)
	}
	flag(campoModelo, "--model")
	flag(campoEsfuerzo, "--effort")
	flag(campoTope, "--budget-usd")
	if !p.revisor {
		flag(campoTiempo, "--timeout")
	}
	return out
}

// lanzarPedir is enter on the action's button.
func (m *Modelo) lanzarPedir() []Orden {
	p := m.pedir
	if !p.revisor && strings.TrimSpace(p.pedido.valor()) == "" {
		p.pedidoVacio = true
		p.foco = campoPedido
		m.avisar("el pedido esta vacio", rojo, DuracionAviso)
		return nil
	}
	args := m.argsPedir(p)
	la := m.lanzar(p.slug, p.accion.Role, args)
	m.registrarVerbo(la, p.accion.Label, lineaVerbo(args, m.c.Windows))
	m.cerrarModal()
	m.avisar("lanzando hoom "+palabrasVerbo(args), normal, DuracionAviso)
	return []Orden{la.o}
}

// lineasComando are the command's lines in c cells, the first after
// prefijo: by words, a continuation starting with two spaces; when it
// takes more than two lines, the argument of -m is shortened to its start
// and "…".
func lineasComando(args []string, c int, prefijo string, windows bool, corte string) []string {
	palabras := palabrasComando(args, windows)
	armar := func(ps []string) []string {
		var out []string
		actual := prefijo + ps[0]
		for _, w := range ps[1:] {
			if celdas(actual)+1+celdas(w) <= c {
				actual += " " + w
				continue
			}
			out = append(out, actual)
			actual = "  " + w
		}
		return append(out, actual)
	}
	cabe := func(ls []string) bool {
		if len(ls) > 2 {
			return false
		}
		for _, l := range ls {
			if celdas(l) > c {
				return false
			}
		}
		return true
	}
	ls := armar(palabras)
	if cabe(ls) {
		return ls
	}
	im := -1
	pedido := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-m" {
			pedido = args[i+1]
		}
	}
	for i := 0; i+1 < len(palabras); i++ {
		if palabras[i] == "-m" {
			im = i + 1
		}
	}
	if im >= 0 {
		q := []rune(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(strings.TrimSpace(pedido)))
		n := min(len(q), 60) - 1
		for ; n >= 1; n-- {
			palabras[im] = tarjetacmd.CitarPedido(string(q[:n])+"…", windows)
			if ls = armar(palabras); cabe(ls) {
				return ls
			}
		}
	}
	if len(ls) > 2 {
		ls = []string{ls[0], strings.Join(ls[1:], " ")}
	}
	for i := range ls {
		ls[i] = cortar(ls[i], c, corte)
	}
	return ls
}
