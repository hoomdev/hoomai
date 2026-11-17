package tablero

import (
	"fmt"
	"strings"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
)

// ordenGrupos are the groups in the list's order: boardcmd's.
var ordenGrupos = func() []Grupo {
	out := make([]Grupo, len(boardcmd.GruposPelota))
	for i, g := range boardcmd.GruposPelota {
		out[i] = Grupo(g)
	}
	return out
}()

// etiquetaSeguir is Seguir solo's label (the step's, the Registro's).
const etiquetaSeguir = "Seguir solo"

// grupoDe is the group of the card c (the package doc, "The groups"):
// boardcmd's, and CORRIENDO while a pilot this board launched is alive for
// it outside Hecho (boardcmd cannot know that pilot).
func (m *Modelo) grupoDe(c *boardcmd.Card) Grupo {
	if c.Column != boardcmd.ColHecho && m.pilotoVivo(c.Slug) {
		return GrupoCorriendo
	}
	return Grupo(boardcmd.GrupoDe(c))
}

// pilotoVivo says whether a pilot this board launched is alive for slug:
// from its Lanzar to its FinProceso.
func (m *Modelo) pilotoVivo(slug string) bool {
	for _, la := range m.lanzados {
		if la.o.Rol == rolPiloto && la.o.Slug == slug {
			return true
		}
	}
	return false
}

// grupos are the board's cards by group, each in the list's order
// (boardcmd.OrdenPelota with the board's groups): TE TOCA by column from the
// board's last column to its first, the others in the board's order. With
// filtrar only the cards the search leaves.
func (m *Modelo) grupos(filtrar bool) map[Grupo][]*boardcmd.Card {
	var todas []*boardcmd.Card
	cols := m.tablero.Columns
	for i := range cols {
		for j := range cols[i].Cards {
			if c := &cols[i].Cards[j]; !filtrar || m.pasaFiltro(c) {
				todas = append(todas, c)
			}
		}
	}
	grupo := func(c *boardcmd.Card) string { return string(m.grupoDe(c)) }
	out := map[Grupo][]*boardcmd.Card{}
	for _, c := range boardcmd.OrdenPelota(todas, grupo) {
		g := m.grupoDe(c)
		out[g] = append(out[g], c)
	}
	return out
}

// tarjetaDeCaja is the box's card: the first of TE TOCA, else the first of
// DETENIDA (nil without one); and the groups of every card.
func (m *Modelo) tarjetaDeCaja() (*boardcmd.Card, map[Grupo][]*boardcmd.Card) {
	if !m.leido {
		return nil, nil
	}
	gs := m.grupos(false)
	if t := gs[GrupoTeToca]; len(t) > 0 {
		return t[0], gs
	}
	if d := gs[GrupoDetenida]; len(d) > 0 {
		return d[0], gs
	}
	return nil, gs
}

// tarjetaDeN is the card n opens: the box's, or the first of EN COLA when
// the box offers it (nothing in TE TOCA, DETENIDA nor CORRIENDO); and the
// text n says without one.
func (m *Modelo) tarjetaDeN() (*boardcmd.Card, string) {
	c, gs := m.tarjetaDeCaja()
	switch {
	case c != nil:
		return c, ""
	case !m.leido:
		return nil, ""
	case len(gs[GrupoCorriendo]) > 0:
		return nil, "nada te espera: hoom esta trabajando"
	case len(gs[GrupoEnCola]) > 0:
		return gs[GrupoEnCola][0], ""
	}
	return nil, "no hay nada pendiente"
}

// ---------------------------------------------------------------- the step

// paso is the step of a card (the package doc, "The step of a card"): the
// card action it runs and its key's row, or Seguir solo, or none and the
// text that says why.
type paso struct {
	act    *boardcmd.Action
	fila   Atajo
	seguir bool
	texto  string
}

func (p paso) hay() bool { return p.act != nil || p.seguir }

// etiqueta is the step's label: the action's, or "Seguir solo".
func (p paso) etiqueta() string {
	if p.seguir {
		return etiquetaSeguir
	}
	if p.act != nil {
		return p.act.Label
	}
	return ""
}

// filaDeAccion is the row of the table that runs the card action id.
func filaDeAccion(id string) (Atajo, bool) {
	for _, a := range Atajos {
		if a.Accion == id && a.Verbo != "" {
			return a, true
		}
	}
	return Atajo{}, false
}

// pasoDe is the card's step: boardcmd's (the rule, once), plus what only the
// board knows: the pilot it launched, the key that reanuda, Seguir solo, and
// its keys' table (a card action without a row is nothing to do here; one
// the card lacks is told with its row's verb).
func (m *Modelo) pasoDe(c *boardcmd.Card) paso {
	enCurso := c.Column != boardcmd.ColHecho && c.Running == nil
	switch {
	case c.Column != boardcmd.ColHecho && m.pilotoVivo(c.Slug):
		return paso{texto: "el piloto lleva la tarjeta: espera a que se detenga"}
	case enCurso && c.Interrupted != nil:
		return paso{texto: "el trabajo se corto: R reanuda o pide de nuevo"}
	case enCurso && m.motivoSinSeguir(c) == "":
		fila, _ := filaDeAccion(AccionSeguir)
		return paso{seguir: true, fila: fila}
	}
	b := boardcmd.PasoDe(c)
	id := boardcmd.AccionDelPaso(c)
	if !enCurso || id == "" {
		return paso{texto: unaLinea(b.Texto)}
	}
	fila, ok := filaDeAccion(id)
	_, tiene := c.Action(id)
	switch {
	case !ok:
		return paso{texto: unaLinea(boardcmd.NadaQueHacer(c))}
	case !tiene:
		return paso{fila: fila, texto: fmt.Sprintf("en %s no hay %s", c.ColumnName, fila.Hace)}
	case b.Accion == "":
		return paso{fila: fila, texto: b.Texto}
	}
	act, _ := c.Action(b.Accion)
	return paso{act: &act, fila: fila}
}

// ---------------------------------------------------------------- Seguir solo

// motivoSinSeguir is why Seguir solo is not available for c: the message of
// the first reason, or "" when it is (the package doc, "Seguir solo").
func (m *Modelo) motivoSinSeguir(c *boardcmd.Card) string {
	switch {
	case c.Item.Auto != item.AutoHastaHumano:
		return fmt.Sprintf("el piloto no esta encendido en la tarjeta: hoom item set %s --auto hasta-humano", c.Slug)
	case c.Column != boardcmd.ColTestWriter && c.Column != boardcmd.ColWriter && c.Column != boardcmd.ColReview:
		return fmt.Sprintf("en %s el piloto no tiene nada que hacer", c.ColumnName)
	case c.Running != nil || c.Interrupted != nil:
		return "la tarjeta ya tiene un trabajo en curso o cortado"
	case c.Red != nil && !c.Red.Esperado:
		return "la tarjeta esta detenida: mira que fallo antes de seguir"
	case m.pilotoVivo(c.Slug):
		return "el piloto ya lleva la tarjeta"
	}
	if p, ok := m.paradaDe(c); ok && p.code != 0 {
		return "el piloto se detuvo: " + p.motivo
	}
	return ""
}

// paradaDe is the stop of the pilot of c, when it still stands.
func (m *Modelo) paradaDe(c *boardcmd.Card) (parada, bool) {
	p, ok := m.paradas[c.Slug]
	switch {
	case !ok:
		return parada{}, false
	case p.porLeer:
		// its column comes with the first read started after it
		return p, true
	case c.Column != p.columna || c.Running != nil && idDe(c.Running) != p.running:
		return parada{}, false
	}
	return p, true
}

// ---------------------------------------------------------------- fixed texts

// fijo is a fixed text of the TUI as the mode writes it: with UTF-8 as it
// is; in ASCII without its accents, its quotes and its arrow (CA-720).
func (m *Modelo) fijo(s string) string {
	if m.c.UTF8 {
		return s
	}
	return sinAcentos.Replace(s)
}

var sinAcentos = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u",
	"Á", "A", "É", "E", "Í", "I", "Ó", "O", "Ú", "U",
	"«", `"`, "»", `"`, "←", "<-",
)

// citar is t between the quotes of the mode.
func (m *Modelo) citar(t string) string {
	return m.s.comillaAbre + t + m.s.comillaCierra
}

// frase is a fixed text with the title t between quotes: antes and despues
// as the mode writes them, t as it comes.
func (m *Modelo) frase(antes, t, despues string) string {
	return m.fijo(antes) + m.citar(t) + m.fijo(despues)
}

// piel is how boardcmd writes the ball's fixed texts for this board: in the
// mode's letters and quotes, the start of a run in the board's clock.
func (m *Modelo) piel() boardcmd.Piel {
	return boardcmd.Piel{Fijo: m.fijo, Citar: m.citar, Hora: m.ahora.Location()}
}
