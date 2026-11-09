package tablero

import (
	"strconv"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
	"github.com/hoomdev/hoomai/internal/valores"
)

// The form's fields, in tab order.
const (
	campoProvider = iota
	campoModelo
	campoEsfuerzo
	campoPresupuesto
	campoTiempo
	campoPedido
)

// campo is a text field of the form.
type campo struct {
	id        int
	texto     []rune
	cursor    int
	propuesto string // the action's value, to tell what changed
}

func (c *campo) valor() string { return string(c.texto) }

func (c *campo) poner(s string) {
	c.texto = []rune(s)
	c.cursor = len(c.texto)
}

// formulario is the launch form of a role action (or of n).
type formulario struct {
	slug     string
	rol      string
	accion   boardcmd.Action
	verbo    []string // {"pedir", <rol>, <slug>}, {"siguiente", <slug>} or {"reanudar", <slug>}
	desde    vistaV1
	revisor  bool // no Pedido, and its Tiempo limite is review.timeout's: shown, never sent
	reanudar bool // the provider is the session's: it does not change

	opcion, propuesta int // the chosen provider option and the proposed one (-1: none)
	campos            []*campo
	foco              int // index into focos()
	pedidoVacio       bool
}

// focos are the form's fields, in tab order.
func (f *formulario) focos() []int {
	if f.revisor {
		return []int{campoProvider, campoModelo, campoEsfuerzo, campoPresupuesto}
	}
	return []int{campoProvider, campoModelo, campoEsfuerzo, campoPresupuesto, campoTiempo, campoPedido}
}

func (f *formulario) enfocado() int { return f.focos()[f.foco] }

func (f *formulario) enfocar(id int) {
	for i, x := range f.focos() {
		if x == id {
			f.foco = i
		}
	}
}

// campo is the text field id (nil for the provider or one the form has
// not).
func (f *formulario) campo(id int) *campo {
	for _, c := range f.campos {
		if c.id == id {
			return c
		}
	}
	return nil
}

func (m *Modelo) abrirFormulario(c *boardcmd.Card, a boardcmd.Action, verbo []string) {
	f := &formulario{
		slug:     c.Slug,
		rol:      a.Role,
		accion:   a,
		verbo:    verbo,
		desde:    m.origen(),
		revisor:  a.ID == boardcmd.ActPedirReviewer,
		reanudar: a.ID == boardcmd.ActReanudar,
		opcion:   -1,
	}
	for i, o := range a.Providers {
		if o.Default {
			f.opcion = i
			break
		}
	}
	if f.opcion < 0 {
		for i, o := range a.Providers {
			if o.OK {
				f.opcion = i
				break
			}
		}
	}
	f.propuesta = f.opcion
	presupuesto := ""
	if a.BudgetUSD != nil {
		presupuesto = strconv.FormatFloat(*a.BudgetUSD, 'f', -1, 64)
	}
	tiempo := ""
	if a.TimeoutS > 0 {
		tiempo = valores.Duracion(time.Duration(a.TimeoutS) * time.Second)
	}
	nuevo := func(id int, v string) *campo {
		c := &campo{id: id, propuesto: v}
		c.poner(v)
		return c
	}
	f.campos = []*campo{
		nuevo(campoModelo, a.Model),
		nuevo(campoEsfuerzo, a.Effort),
		nuevo(campoPresupuesto, presupuesto),
	}
	f.campos = append(f.campos, nuevo(campoTiempo, tiempo))
	if !f.revisor {
		f.campos = append(f.campos, nuevo(campoPedido, a.Pedido))
		f.enfocar(campoPedido)
	} else {
		f.enfocar(campoProvider)
	}
	m.menu = nil
	m.form = f
	m.vista = vistaFormulario
}

func (m *Modelo) enFormulario(t Tecla) []Orden {
	f := m.form
	foco := f.enfocado()
	c := f.campo(foco)
	switch t {
	case TeclaEsc:
		m.cerrarFormulario()
	case TeclaTab:
		f.foco = (f.foco + 1) % len(f.focos())
	case TeclaShiftTab:
		f.foco = (f.foco + len(f.focos()) - 1) % len(f.focos())
	case TeclaIzq, TeclaDer:
		d := 1
		if t == TeclaIzq {
			d = -1
		}
		switch {
		case foco == campoProvider && !f.reanudar:
			f.cambiarOpcion(d)
		case c != nil:
			c.cursor += d
			if c.cursor < 0 {
				c.cursor = 0
			}
			if c.cursor > len(c.texto) {
				c.cursor = len(c.texto)
			}
		}
	case TeclaBorrar:
		if c != nil && c.cursor > 0 {
			c.texto = append(c.texto[:c.cursor-1], c.texto[c.cursor:]...)
			c.cursor--
		}
	case TeclaCtrlU:
		if c != nil {
			c.poner("")
		}
	case TeclaCtrlE:
		if p := f.campo(campoPedido); p != nil {
			return []Orden{EditarPedido{Texto: p.valor()}}
		}
	case TeclaEnter:
		return m.lanzarFormulario()
	default:
		if c != nil {
			r := []rune(string(t))
			c.texto = append(c.texto[:c.cursor], append(r, c.texto[c.cursor:]...)...)
			c.cursor += len(r)
			if c.id == campoPedido {
				f.pedidoVacio = false
			}
		}
	}
	return nil
}

// cambiarOpcion moves the provider to the previous or next option that
// can, wrapping around.
func (f *formulario) cambiarOpcion(d int) {
	n := len(f.accion.Providers)
	if n == 0 {
		return
	}
	i := f.opcion
	for k := 0; k < n; k++ {
		i = ((i+d)%n + n) % n
		if f.accion.Providers[i].OK {
			f.opcion = i
			return
		}
	}
}

func (m *Modelo) cerrarFormulario() {
	if m.form != nil {
		m.vista = m.form.desde
	}
	m.form = nil
}

// lanzarFormulario is enter on the form: it checks the fields and orders
// the launch.
func (m *Modelo) lanzarFormulario() []Orden {
	f := m.form
	if p := f.campo(campoPedido); p != nil && strings.TrimSpace(p.valor()) == "" {
		f.pedidoVacio = true
		f.enfocar(campoPedido)
		m.avisar("el pedido esta vacio", rojo, DuracionAviso)
		return nil
	}
	if f.opcion >= 0 && f.opcion < len(f.accion.Providers) {
		if o := f.accion.Providers[f.opcion]; !o.OK {
			m.avisar(o.Why, rojo, DuracionAviso)
			return nil
		}
	}
	if t := f.campo(campoTiempo); t != nil && strings.TrimSpace(t.valor()) != strings.TrimSpace(t.propuesto) {
		if _, err := manifest.ParseTimeout("--timeout", t.valor()); err != nil {
			m.avisar(err.Error(), rojo, DuracionAviso)
			return nil
		}
	}
	if p := f.campo(campoPresupuesto); p != nil {
		if v := strings.TrimSpace(p.valor()); v != "" {
			if _, err := strconv.ParseFloat(v, 64); err != nil {
				m.avisar("el presupuesto no es un numero", rojo, DuracionAviso)
				return nil
			}
		}
	}
	o := m.lanzar(f.slug, f.rol, f.args())
	m.cerrarFormulario()
	m.avisar("lanzando hoom "+palabrasVerbo(o.Args), normal, DuracionAviso)
	return []Orden{o}
}

// args are the order's Args: the verb, -m and the pedido (not for the
// reviewer), then only the flags whose value differs from the proposal.
func (f *formulario) args() []string {
	out := append([]string{}, f.verbo...)
	if p := f.campo(campoPedido); p != nil {
		out = append(out, "-m", strings.TrimSpace(p.valor()))
	}
	if !f.reanudar && f.opcion != f.propuesta && f.opcion >= 0 {
		out = append(out, "--provider", f.accion.Providers[f.opcion].Name)
	}
	cambio := func(id int, flag string) {
		c := f.campo(id)
		if c == nil {
			return
		}
		v := strings.TrimSpace(c.valor())
		if v == strings.TrimSpace(c.propuesto) {
			return
		}
		if id == campoPresupuesto && v == "" {
			v = "0"
		}
		out = append(out, flag, v)
	}
	cambio(campoModelo, "--model")
	cambio(campoEsfuerzo, "--effort")
	cambio(campoPresupuesto, "--budget-usd")
	if !f.revisor {
		// the reviewer's timeout is review.timeout: capa 1 takes no --timeout
		cambio(campoTiempo, "--timeout")
	}
	return out
}

// marcadores are the markers tarjetacmd.Citar writes on Windows for a value
// it cannot quote.
var marcadores = map[string]string{
	"--provider": "<provider>", "--model": "<modelo>", "--effort": "<esfuerzo>",
	"--budget-usd": "<presupuesto>", "--timeout": "<tiempo>",
}

// lineaVerbo is the → line's command: "hoom" and the Args, each value
// quoted as the capa-1 verbs do it.
func lineaVerbo(args []string, windows bool) string {
	parts := []string{"hoom"}
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-m" && i+1 < len(args):
			parts = append(parts, a, tarjetacmd.CitarPedido(args[i+1], windows))
			i++
		case strings.HasPrefix(a, "--") && i+1 < len(args) && marcadores[a] != "":
			parts = append(parts, a, tarjetacmd.Citar(args[i+1], marcadores[a], windows))
			i++
		default:
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
