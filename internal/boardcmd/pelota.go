package boardcmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/item"
)

// The ball (studio-tu-siguiente-paso): who has it, the rule of `hoom
// tablero` v3 (tablero-tu-siguiente-paso), once. Derive resolves it for
// every card (Card.Pelota) and build for the board (Board.Pelota); the
// Studio paints it and `hoom tablero` takes it from here, adding only what
// it alone knows: the pilot it launched, Seguir solo and its stop, its texts
// that name keys, the expert mode and ASCII. No text of the ball names a
// key, a button or a verb of the terminal: each skin adds how one acts.

// The groups of the ball, in the list's order. Their values are their names.
const (
	GrupoTeToca    = "TE TOCA"
	GrupoCorriendo = "CORRIENDO"
	GrupoDetenida  = "DETENIDA"
	GrupoEnCola    = "EN COLA"
	GrupoHecho     = "HECHO"
)

// GruposPelota are the groups in the list's order.
var GruposPelota = []string{GrupoTeToca, GrupoCorriendo, GrupoDetenida, GrupoEnCola, GrupoHecho}

// The states of an entry of the path.
const (
	CaminoHecho  = "hecho"
	CaminoActual = "actual"
	CaminoFalta  = "falta"
)

// NombresCamino are the steps of the path, one per column of Columns, in
// its order.
var NombresCamino = []string{"Pedido", "Spec", "Tu firma", "Tests", "Código", "Review", "Tu aceptación", "Hecho"}

// Pelota is who has the card's ball, resolved: its group, its step, the
// phrase of the box TU SIGUIENTE PASO, AHORA, SIGUE and the path of 8 steps.
type Pelota struct {
	Grupo  string          `json:"grupo"`
	Paso   Paso            `json:"paso"`
	Frase  string          `json:"frase"` // "" in CORRIENDO, EN COLA and HECHO
	Ahora  FraseGris       `json:"ahora"`
	Sigue  FraseGris       `json:"sigue"`
	Camino []EntradaCamino `json:"camino"`
}

// MarshalJSON emits the path [] instead of null when it has none (a card
// Derive did not make).
func (p Pelota) MarshalJSON() ([]byte, error) {
	type plano Pelota // sin el metodo: evita la recursion
	if p.Camino == nil {
		p.Camino = []EntradaCamino{}
	}
	return json.Marshal(plano(p))
}

// Paso is the card's step: the id of an enabled action of the card and its
// label, or none ("" and "") and the text that says why.
type Paso struct {
	Accion   string `json:"accion"`
	Etiqueta string `json:"etiqueta"`
	Texto    string `json:"texto"`
}

// FraseGris is a phrase and its gray line ("" without one).
type FraseGris struct {
	Frase string `json:"frase"`
	Gris  string `json:"gris"`
}

// EntradaCamino is one step of the path: its name, hecho, actual or falta,
// and only in the actual one the text of its group.
type EntradaCamino struct {
	Nombre string `json:"nombre"`
	Estado string `json:"estado"`
	Texto  string `json:"texto"`
}

// PelotaBoard is the board's ball: the slugs of every card in the list's
// order, the card of the box TU SIGUIENTE PASO ("" without one) and how many
// cards each group has.
type PelotaBoard struct {
	Orden  []string     `json:"orden"`
	Caja   string       `json:"caja"`
	Cuenta CuentaPelota `json:"cuenta"`
}

// MarshalJSON emits orden [] instead of null on a board without cards.
func (p PelotaBoard) MarshalJSON() ([]byte, error) {
	type plano PelotaBoard // sin el metodo: evita la recursion
	if p.Orden == nil {
		p.Orden = []string{}
	}
	return json.Marshal(plano(p))
}

// CuentaPelota is how many cards each group has: its five keys always.
type CuentaPelota struct {
	TeToca    int `json:"te_toca"`
	Corriendo int `json:"corriendo"`
	Detenida  int `json:"detenida"`
	EnCola    int `json:"en_cola"`
	Hecho     int `json:"hecho"`
}

// Piel is how a skin writes the ball's fixed texts. Its zero value is the
// JSON's: the texts as they are, titles between « », the start of a run in
// its own clock. A skin's Fijo touches only the fixed texts: the data they
// carry (a title, a plain, a reason, a why) goes as it is.
type Piel struct {
	Fijo  func(string) string // writes a fixed text (nil: as it is)
	Citar func(string) string // a title between quotes (nil: «t»)
	Hora  *time.Location      // where the start of a run is told (nil: its own)
}

func (p Piel) fijo(s string) string {
	if p.Fijo == nil {
		return s
	}
	return p.Fijo(s)
}

func (p Piel) citar(t string) string {
	if p.Citar == nil {
		return "«" + t + "»"
	}
	return p.Citar(t)
}

// frase is a fixed text with the title t between quotes.
func (p Piel) frase(antes, t, despues string) string {
	return p.fijo(antes) + p.citar(t) + p.fijo(despues)
}

// PelotaDe is the ball of c as the skin p writes it.
func PelotaDe(c *Card, p Piel) Pelota {
	g := GrupoDe(c)
	paso := PasoDe(c)
	return Pelota{
		Grupo:  g,
		Paso:   paso,
		Frase:  FraseDe(c, g, paso.Accion, p),
		Ahora:  AhoraDe(c, p),
		Sigue:  SigueDe(c, p),
		Camino: CaminoDe(c, g, p),
	}
}

// pelotaBoard is the ball of b, from its cards' balls.
func pelotaBoard(b Board) PelotaBoard {
	var cards []*Card
	for i := range b.Columns {
		for j := range b.Columns[i].Cards {
			cards = append(cards, &b.Columns[i].Cards[j])
		}
	}
	grupo := func(c *Card) string { return c.Pelota.Grupo }
	out := PelotaBoard{Orden: make([]string, 0, len(cards))}
	for _, c := range OrdenPelota(cards, grupo) {
		out.Orden = append(out.Orden, c.Slug)
		switch g := grupo(c); g {
		case GrupoTeToca:
			if out.Cuenta.TeToca == 0 {
				out.Caja = c.Slug
			}
			out.Cuenta.TeToca++
		case GrupoCorriendo:
			out.Cuenta.Corriendo++
		case GrupoDetenida:
			if out.Cuenta.TeToca == 0 && out.Cuenta.Detenida == 0 {
				out.Caja = c.Slug
			}
			out.Cuenta.Detenida++
		case GrupoEnCola:
			out.Cuenta.EnCola++
		case GrupoHecho:
			out.Cuenta.Hecho++
		}
	}
	return out
}

// ---------------------------------------------------------------- the groups

// GrupoDe is the group of c, the first row of the table that holds: HECHO
// (its column is Hecho), CORRIENDO (running), DETENIDA (interrupted, or a
// red that is not the expected one), EN COLA (Backlog), TE TOCA (any other).
// A pilot a board launched is not here: boardcmd cannot know it.
func GrupoDe(c *Card) string {
	switch {
	case c.Column == ColHecho:
		return GrupoHecho
	case c.Running != nil:
		return GrupoCorriendo
	case c.Interrupted != nil || c.Red != nil && !c.Red.Esperado:
		return GrupoDetenida
	case c.Column == ColBacklog:
		return GrupoEnCola
	}
	return GrupoTeToca
}

// OrdenPelota is cards in the list's order: TE TOCA from the column nearest
// Hecho to the farthest, then CORRIENDO, DETENIDA, EN COLA and HECHO, and
// inside each the order cards come in. cards come in the board's order,
// column by column, and the board's last column is the nearest Hecho (a
// board that brings its columns in another order is listed by it). grupo is
// the group of a card: a skin passes its own.
func OrdenPelota(cards []*Card, grupo func(*Card) string) []*Card {
	lugar := map[string]int{} // where each column comes in the board
	por := make(map[string][]*Card, len(GruposPelota))
	for _, c := range cards {
		if _, ok := lugar[c.Column]; !ok {
			lugar[c.Column] = len(lugar)
		}
		g := grupo(c)
		por[g] = append(por[g], c)
	}
	te := por[GrupoTeToca]
	sort.SliceStable(te, func(i, j int) bool { return lugar[te[i].Column] > lugar[te[j].Column] })
	out := make([]*Card, 0, len(cards))
	for _, g := range GruposPelota {
		out = append(out, por[g]...)
	}
	return out
}

// ---------------------------------------------------------------- the step

// AccionDelPaso is the action rules 4 and 5 of the step choose for c:
// aprobar in Tu aprobacion, integrar in Tu aceptacion, else its first action
// when it launches a role or is guardar; "" when there is none (rule 6).
// Rules 1 to 3 (Hecho, running, interrupted) go before it (PasoDe).
func AccionDelPaso(c *Card) string {
	switch {
	case c.Column == ColTuAprobacion:
		return ActAprobar
	case c.Column == ColTuAceptacion:
		return ActIntegrar
	case len(c.Actions) == 0:
		return ""
	}
	if id := c.Actions[0].ID; IsRoleAction(id) || id == ActGuardar {
		return id
	}
	return ""
}

// NadaQueHacer is the text of a card without a step by rule 6.
func NadaQueHacer(c *Card) string {
	return fmt.Sprintf("en %s no hay nada que hacer ahora: %s", c.ColumnName, c.Plain)
}

// PasoDe is the step of c, the six rules: none in Hecho, with running or with
// interrupted; else the action AccionDelPaso chooses when the card has it
// enabled; without it, the text that says why (the action the card lacks, the
// why of the disabled one, or rule 6). Never Seguir solo: that one is the
// board's.
func PasoDe(c *Card) Paso {
	switch {
	case c.Column == ColHecho:
		return Paso{Texto: "la tarjeta ya esta en Hecho"}
	case c.Running != nil:
		return Paso{Texto: "el " + rolPelota(c.Running.Role) + " esta trabajando: espera a que termine"}
	case c.Interrupted != nil:
		return Paso{Texto: "el trabajo se corto: reanudalo o pedilo de nuevo"}
	}
	id := AccionDelPaso(c)
	if id == "" {
		return Paso{Texto: NadaQueHacer(c)}
	}
	a, ok := c.Action(id)
	switch {
	case !ok:
		return Paso{Texto: fmt.Sprintf("en %s no hay %s", c.ColumnName, strings.ToLower(acciones[id].label))}
	case !a.Enabled:
		return Paso{Texto: a.Why}
	}
	return Paso{Accion: a.ID, Etiqueta: a.Label}
}

// rolPelota is a role as the ball names it: "agente" when it has none.
func rolPelota(r string) string {
	if r == "" {
		return "agente"
	}
	return r
}

// ---------------------------------------------------------------- the phrases

// TituloDe is the card's title on one line: the item's titulo, or the slug
// when it is empty.
func TituloDe(c *Card) string {
	if c.Item.Titulo == "" {
		return c.Slug
	}
	return strings.NewReplacer("\r\n", " ", "\n", " ").Replace(c.Item.Titulo)
}

// FraseDe is the phrase of the box TU SIGUIENTE PASO for c of group g whose
// step runs the action accion ("" without a step): "" outside TE TOCA and
// DETENIDA.
func FraseDe(c *Card, g, accion string, p Piel) string {
	t := TituloDe(c)
	switch {
	case g == GrupoDetenida:
		return p.frase("Decidí cómo sigue ", t, ".")
	case g != GrupoTeToca:
		return ""
	}
	switch accion {
	case ActAprobar:
		return p.frase("Firmá el spec de ", t, ".")
	case ActIntegrar:
		return p.frase("Aceptá ", t, ".")
	case ActPedirArquitecto:
		return p.frase("Pedile el spec al arquitecto para ", t, ".")
	case ActPedirTestWriter:
		return p.frase("Pedile los tests al test-writer para ", t, ".")
	case ActPedirWriter:
		return p.frase("Pedile el código al writer para ", t, ".")
	case ActPedirReviewer:
		return p.frase("Pedile la review al reviewer para ", t, ".")
	case ActReanudar:
		return p.frase("Reanudá el trabajo de ", t, ".")
	case ActGuardar:
		return p.frase("Guardá el trabajo de ", t, ".")
	}
	return p.frase("Mirá ", t, ".")
}

// fraseDelRol is AHORA's phrase of a card whose role runs.
func fraseDelRol(rol string) string {
	switch rol {
	case "arquitecto":
		return "El arquitecto está escribiendo el spec."
	case "test-writer":
		return "El test-writer está escribiendo los tests."
	case "writer":
		return "El writer está escribiendo el código."
	case "reviewer":
		return "El reviewer está revisando el cambio."
	}
	return "El " + rolPelota(rol) + " está trabajando."
}

// etapaDe is where a run is: "<stage> <step>/<steps>", or the stage alone
// without steps.
func etapaDe(stage string, step, steps int) string {
	if steps == 0 {
		return stage
	}
	return strings.TrimSpace(fmt.Sprintf("%s %d/%d", stage, step, steps))
}

// AhoraDe is AHORA: what is happening with c now, and its gray line.
func AhoraDe(c *Card, p Piel) FraseGris {
	switch {
	case c.Running != nil:
		r := c.Running
		var partes []string
		if r.Provider != "" {
			partes = append(partes, r.Provider)
		}
		inicio := r.StartedAt
		if p.Hora != nil {
			inicio = inicio.In(p.Hora)
		}
		partes = append(partes, "empezo "+inicio.Format("15:04"))
		if e := etapaDe(r.Stage, r.Step, r.Steps); e != "" {
			partes = append(partes, e)
		}
		return FraseGris{Frase: p.fijo(fraseDelRol(r.Role)), Gris: strings.Join(partes, " · ")}
	case c.Interrupted != nil:
		i := c.Interrupted
		return FraseGris{Frase: p.fijo(fmt.Sprintf("El trabajo del %s se cortó en el paso %d de %d.", rolPelota(i.Role), i.Step, i.Steps)),
			Gris: p.fijo("ya no corre")}
	case c.Red != nil && !c.Red.Esperado:
		return FraseGris{Frase: c.Red.Plain, Gris: c.Red.Reason}
	case c.Red != nil:
		return FraseGris{Frase: p.fijo("Los tests están escritos y fallan, como se espera."), Gris: p.fijo("falta el código")}
	case c.Column == ColTuAprobacion:
		return FraseGris{Frase: p.fijo("El spec está listo y espera tu firma."), Gris: c.Plain}
	case c.Column == ColTuAceptacion:
		return FraseGris{Frase: p.fijo("Está lista para que la pruebes y la aceptes."), Gris: c.Plain}
	case c.Column == ColBacklog:
		return FraseGris{Frase: p.fijo("Todavía no empezó."), Gris: c.Plain}
	case c.Column == ColHecho:
		return FraseGris{Frase: p.fijo("Terminada."), Gris: c.Plain}
	}
	return FraseGris{Frase: c.Plain}
}

// SigueDe is SIGUE: what happens after, and without the pilot, in the
// columns where it would carry the card, the gray line that says how to turn
// it on.
func SigueDe(c *Card, p Piel) FraseGris {
	on := c.Item.Auto == item.AutoHastaHumano
	elegir := func(sin, con string) FraseGris {
		if on {
			return FraseGris{Frase: p.fijo(con)}
		}
		return FraseGris{Frase: p.fijo(sin), Gris: "con el piloto seguiria sola: hoom item set " + c.Slug + " --auto hasta-humano"}
	}
	switch c.Column {
	case ColBacklog:
		return FraseGris{Frase: p.fijo("el arquitecto escribe el spec; despues te toca firmarlo")}
	case ColArquitecto:
		return FraseGris{Frase: p.fijo("te toca firmar el spec")}
	case ColTuAprobacion:
		return elegir("al firmar, te toca pedir los tests al test-writer",
			"al firmar, el piloto pide los tests y sigue solo hasta tu aceptación")
	case ColTestWriter:
		return elegir("despues, te toca pedir el código al writer",
			"el piloto pide el código al writer y sigue hasta tu aceptación")
	case ColWriter:
		return elegir("despues, te toca pedir la review al reviewer",
			"el piloto pide la review; si algo bloquea, hace una ronda de hallazgos")
	case ColReview:
		return elegir("si nada bloquea, te toca aceptar",
			"si nada bloquea, te toca aceptar; si algo bloquea, el piloto hace una ronda de hallazgos")
	case ColTuAceptacion:
		return FraseGris{Frase: p.fijo("al aceptar, hoom integra y la tarjeta pasa a Hecho")}
	case ColHecho:
		return FraseGris{Frase: p.fijo("nada: la tarjeta esta terminada")}
	}
	return FraseGris{}
}

// ---------------------------------------------------------------- the path

// TextoCamino is what the actual step of the path says for a card of group
// g: "" in HECHO, and in CORRIENDO without a running (a skin's pilot), too.
func TextoCamino(c *Card, g string) string {
	switch g {
	case GrupoTeToca:
		return "te toca a vos"
	case GrupoCorriendo:
		if c.Running == nil {
			return ""
		}
		return rolPelota(c.Running.Role) + ", ahora"
	case GrupoDetenida:
		return "se detuvo"
	case GrupoEnCola:
		return "sin empezar"
	}
	return ""
}

// CaminoDe is the path of c of group g: one entry per column of Columns,
// hecho before the card's column, actual in it and falta after; in HECHO
// all eight are hecho.
func CaminoDe(c *Card, g string, p Piel) []EntradaCamino {
	i := columnIndex(c.Column)
	out := make([]EntradaCamino, len(Columns))
	for j := range Columns {
		e := EntradaCamino{Nombre: p.fijo(NombresCamino[j]), Estado: CaminoFalta}
		switch {
		case g == GrupoHecho || j < i:
			e.Estado = CaminoHecho
		case j == i:
			e.Estado, e.Texto = CaminoActual, p.fijo(TextoCamino(c, g))
		}
		out[j] = e
	}
	return out
}
