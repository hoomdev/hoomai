package boardcmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/providers"
)

// C3: what a person can do with the card. Everything here is part of Derive
// and stays pure: the page shows it and every action endpoint derives the
// card again before acting, so the rule lives once, in Go.

// stationRoles are the roles whose work a column is waiting for: running one
// of them on the card paints the ghost in the next column.
var stationRoles = map[string][]string{
	ColBacklog:    {"arquitecto"},
	ColArquitecto: {"arquitecto"},
	ColTestWriter: {"test-writer"},
	ColWriter:     {"writer"},
	ColReview:     {"reviewer", "writer"},
}

// actionLabels are the binary's labels (no accents; the page has its own).
var actionLabels = map[string]string{
	ActPedirArquitecto: "Pedir al arquitecto",
	ActPedirTestWriter: "Pedir al test-writer",
	ActPedirWriter:     "Pedir al writer",
	ActPedirReviewer:   "Pedir al reviewer",
	ActAprobar:         "Aprobar spec",
	ActIntegrar:        "Integrar",
	ActReanudar:        "Reanudar",
	ActRelanzar:        "Volver a lanzar",
	ActDescartar:       "Descartar cambios",
	ActGuardar:         "Guardar",
	ActSesion:          "Abrir sesion",
	ActTerminal:        "Ver terminal",
}

// pedirRole is the role each "pedir" action launches.
var pedirRole = map[string]string{
	ActPedirArquitecto: "arquitecto",
	ActPedirTestWriter: "test-writer",
	ActPedirWriter:     "writer",
	ActPedirReviewer:   "reviewer",
}

// ReviewLenses is how many passes the budget of a review is split among: the
// worst case, a review that demands the four lenses.
const ReviewLenses = 4

// Why sentences of the actions, in the normal mode's words.
const (
	whyNoWorkspace = "la tarjeta no tiene su propio espacio de trabajo: desde el tablero solo se le pide trabajo a una tarjeta que lo tiene"
	whyNoProvider  = "ningun proveedor instalado puede tomar este trabajo"
	whyNoRun       = "el trabajo se corto antes de que el agente empezara: solo se puede volver a lanzar"
	whyNoSession   = "el agente no dejo una sesion que retomar: solo se puede volver a lanzar"
	whyReviewer    = "una revision cortada se vuelve a lanzar entera"
	whyBack        = "la tarjeta no vuelve atras: su columna la da la evidencia"
)

// Action returns the card's action with that id, if it has it.
func (c Card) Action(id string) (Action, bool) {
	for _, a := range c.Actions {
		if a.ID == id {
			return a, true
		}
	}
	return Action{}, false
}

// IsRoleAction reports whether an action launches a role.
func IsRoleAction(id string) bool {
	_, pedir := pedirRole[id]
	return pedir || id == ActReanudar || id == ActRelanzar
}

// columnActions are the actions of the card's column, the primary first.
func columnActions(c Card) []string {
	switch c.Column {
	case ColBacklog, ColArquitecto:
		return []string{ActPedirArquitecto}
	case ColTuAprobacion:
		return []string{ActAprobar, ActPedirArquitecto}
	case ColTestWriter:
		return []string{ActPedirTestWriter}
	case ColWriter:
		return []string{ActPedirWriter}
	case ColReview:
		var out []string
		if c.Evidence.ReviewRequired && c.Evidence.ReviewID == "" {
			out = append(out, ActPedirReviewer)
		}
		if len(c.Evidence.BlockingFindings) > 0 {
			out = append(out, ActPedirWriter)
		}
		return out
	case ColTuAceptacion:
		return []string{ActIntegrar}
	}
	return nil
}

// actionsOf is the card's action list: its column's, then the ones any card
// may have.
func actionsOf(ev Evidence, c Card) []Action {
	ids := columnActions(c)
	if c.Interrupted != nil {
		ids = append(ids, ActReanudar, ActRelanzar)
		if hasTask(ev) && len(discardable(ev)) > 0 {
			ids = append(ids, ActDescartar)
		}
	}
	if len(c.Unsynced) > 0 {
		ids = append(ids, ActGuardar)
	}
	if hasTask(ev) {
		ids = append(ids, ActSesion, ActTerminal)
	}
	out := make([]Action, 0, len(ids))
	for _, id := range ids {
		out = append(out, buildAction(ev, c, id))
	}
	return out
}

func buildAction(ev Evidence, c Card, id string) Action {
	a := Action{ID: id, Label: actionLabels[id], Enabled: true, Providers: []ProviderOption{}, Paths: []string{}}
	switch id {
	case ActDescartar, ActSesion, ActTerminal:
		a.Expert = true
	}
	switch id {
	case ActGuardar:
		a.Paths = sortedCopy(c.Unsynced)
	case ActDescartar:
		a.Paths = discardable(ev)
	case ActAprobar:
		a.Signer = ev.Signer
	}
	if IsRoleAction(id) {
		roleAction(ev, c, &a)
	}
	if id != ActSesion && id != ActTerminal && c.Running != nil {
		disable(&a, "espera a que termine "+quien(c.Running.Role)+" que esta trabajando")
	}
	if a.Enabled && IsRoleAction(id) {
		roleWhy(ev, c, &a)
	}
	return a
}

func disable(a *Action, why string) {
	if a.Enabled {
		a.Enabled, a.Why = false, why
	}
}

func quien(role string) string {
	if strings.TrimSpace(role) == "" {
		return "el agente"
	}
	return "el " + role
}

// roleAction fills what a role action proposes: role, request, budget and
// the providers that can take it.
func roleAction(ev Evidence, c Card, a *Action) {
	intr := c.Interrupted
	switch a.ID {
	case ActReanudar, ActRelanzar:
		if intr != nil {
			a.Role = intr.Role
		}
	default:
		a.Role = pedirRole[a.ID]
	}
	switch a.ID {
	case ActReanudar:
		if intr != nil {
			a.Pedido = fmt.Sprintf("El trabajo anterior se corto en el paso %d de %d (%s). Revisa el arbol como quedo y termina lo que se te pidio.",
				intr.Step, intr.Steps, intr.Stage)
			a.ResumeID = sessionOf(ev, intr.RunID)
		}
	default:
		a.Pedido = defaultPedido(ev, c, a.Role)
	}
	review := a.Role == "reviewer"
	if b := ev.Item.PresupuestoUSD; b != nil && c.Spend.RemainingUSD != nil {
		v := *c.Spend.RemainingUSD
		if review {
			v /= ReviewLenses
		}
		a.BudgetUSD = &v
	}
	a.Providers = providerOptions(ev, c, a)
}

// roleWhy disables a role action that cannot run now, in the contract's
// order: no workspace, no budget, no provider, and what resuming needs.
func roleWhy(ev Evidence, c Card, a *Action) {
	switch {
	case !hasTask(ev) && c.Column != ColBacklog:
		disable(a, whyNoWorkspace)
		return
	case ev.Item.PresupuestoUSD != nil && c.Spend.RemainingUSD != nil && *c.Spend.RemainingUSD <= 0:
		disable(a, fmt.Sprintf("se agoto el presupuesto de la tarjeta: se gastaron %s de %s USD",
			money(spentOf(c.Spend)), money(*ev.Item.PresupuestoUSD)))
		return
	}
	ok := false
	for _, p := range a.Providers {
		ok = ok || p.OK
	}
	if !ok {
		disable(a, whyNoProvider)
		return
	}
	if a.ID != ActReanudar || c.Interrupted == nil {
		return
	}
	intr := c.Interrupted
	switch {
	case intr.RunID == "":
		disable(a, whyNoRun)
	case sessionOf(ev, intr.RunID) == "":
		disable(a, whyNoSession)
	case envelopeOf(ev, intr.EnvelopeID).Isolated:
		disable(a, fmt.Sprintf("el %s trabaja a ciegas y su espacio ciego ya no existe: solo se puede volver a lanzar", intr.Role))
	case intr.Role == "reviewer":
		disable(a, whyReviewer)
	case !infoOf(ev, intr.Provider).Capabilities.Resume:
		disable(a, intr.Provider+" no puede retomar una sesion: solo se puede volver a lanzar")
	}
}

// providerOptions lists the installed providers as this action sees them.
func providerOptions(ev Evidence, c Card, a *Action) []ProviderOption {
	out := []ProviderOption{}
	only := ""
	if a.ID == ActReanudar && c.Interrupted != nil {
		only = c.Interrupted.Provider // la sesion es suya
	}
	review := a.Role == "reviewer"
	writers := append([]string{}, c.Providers.Declared...)
	if c.Providers.Writer != "" {
		writers = append(writers, c.Providers.Writer)
	}
	for _, info := range ev.Providers {
		if !info.Installed || (only != "" && info.Name != only) {
			continue
		}
		o := ProviderOption{Name: info.Name, OK: true, Budget: info.Capabilities.Budget, MinBudgetUSD: info.MinBudgetUSD}
		perRun := 0.0
		if a.BudgetUSD != nil {
			perRun = *a.BudgetUSD
		}
		switch {
		case !info.Capabilities.SystemPrompt:
			o.OK, o.Why = false, info.Name+" no puede recibir el contrato del rol"
		case ev.Item.PresupuestoUSD != nil && info.MinBudgetUSD > 0 && perRun < info.MinBudgetUSD:
			o.OK, o.Why = false, fmt.Sprintf("quedan %s USD y %s necesita al menos %s USD por run",
				money(perRun), info.Name, money(info.MinBudgetUSD))
		case review && containsStr(writers, info.Name):
			o.OK, o.Why = false, info.Name+" escribio el codigo: la revision tiene que ser de otro proveedor"
		}
		out = append(out, o)
	}
	def := -1
	if a.ID == ActRelanzar && c.Interrupted != nil {
		for i, o := range out {
			if o.OK && o.Name == c.Interrupted.Provider {
				def = i
			}
		}
	}
	for i, o := range out {
		if def < 0 && o.OK {
			def = i
		}
	}
	if def >= 0 {
		out[def].Default = true
	}
	return out
}

// defaultPedido is the request the dialog proposes for a role.
func defaultPedido(ev Evidence, c Card, role string) string {
	spec := specOf(ev)
	switch role {
	case "arquitecto":
		p := fmt.Sprintf("Escribi el spec %s de la tarjeta %q.", spec, ev.Item.Titulo)
		if pedido := strings.TrimSpace(ev.Item.Pedido); pedido != "" {
			p += "\nPedido: " + pedido
		}
		return p
	case "test-writer":
		p := "Escribi los tests que citan los criterios de " + spec + " que todavia no tienen prueba"
		if len(ev.Untraced) > 0 {
			p += ": " + strings.Join(ev.Untraced, ", ")
		}
		return p + "."
	case "writer":
		if c.Column == ColReview && len(c.Evidence.BlockingFindings) > 0 {
			return "Corregi los hallazgos que bloquean la tarjeta: " + strings.Join(c.Evidence.BlockingFindings, ", ") + "."
		}
		return "Implementa " + spec + " hasta que hoom verify --spec " + spec + " de verde."
	}
	return ""
}

// discardable are the card's unsynced paths inside its workspace and outside
// its .hoom/: evidence is append-only and never goes back.
func discardable(ev Evidence) []string {
	out := []string{}
	if !hasTask(ev) || ev.Dir == "." || ev.Dir == "" {
		return out
	}
	pre, hoom := ev.Dir+"/", ev.Dir+"/.hoom/"
	for _, p := range sortedCopy(ev.Uncommitted) {
		if strings.HasPrefix(p, pre) && !strings.HasPrefix(p, hoom) {
			out = append(out, p)
		}
	}
	return out
}

// dropsOf says, for every other column, what dropping the card there does.
func dropsOf(c Card) []Drop {
	out := []Drop{}
	at := columnIndex(c.Column)
	primary := ""
	if ids := columnActions(c); len(ids) > 0 {
		primary = ids[0]
	}
	for i, col := range Columns {
		switch {
		case i == at:
			continue
		case i < at:
			out = append(out, Drop{Column: col.ID, Why: whyBack})
		case i == at+1 && primary != "":
			out = append(out, Drop{Column: col.ID, Action: primary})
		case i == at+1:
			out = append(out, Drop{Column: col.ID, Why: c.Plain})
		default:
			out = append(out, Drop{Column: col.ID, Why: "primero tiene que llegar a " + Columns[at+1].Name + ": " + c.Plain})
		}
	}
	return out
}

// ghostOf is the card in the next column while a role of its station works.
func ghostOf(c Card) *Ghost {
	r := c.Running
	if r == nil || !containsStr(stationRoles[c.Column], r.Role) {
		return nil
	}
	at := columnIndex(c.Column)
	if at < 0 || at+1 >= len(Columns) {
		return nil
	}
	return &Ghost{Column: Columns[at+1].ID, Role: r.Role, Provider: r.Provider, EnvelopeID: r.EnvelopeID,
		RunID: r.RunID, Stage: r.Stage, Step: r.Step, Steps: r.Steps}
}

// declaredOf are the providers of the item's interactive sessions, distinct,
// in order of appearance.
func declaredOf(ev Evidence) []string {
	out := []string{}
	for _, s := range ev.Item.Sesiones {
		if !containsStr(out, s.Provider) {
			out = append(out, s.Provider)
		}
	}
	return out
}

func columnIndex(id string) int {
	for i, col := range Columns {
		if col.ID == id {
			return i
		}
	}
	return -1
}

func sessionOf(ev Evidence, runID string) string {
	if runID == "" {
		return ""
	}
	for _, r := range ev.Runs {
		if r.Meta.ID == runID {
			return r.Meta.ProviderSessionID
		}
	}
	return ""
}

func envelopeOf(ev Evidence, id string) envelope.Record {
	for _, e := range ev.Envelopes {
		if e.Record.ID == id {
			return e.Record
		}
	}
	return envelope.Record{}
}

func infoOf(ev Evidence, name string) providers.Info {
	for _, info := range ev.Providers {
		if info.Name == name {
			return info
		}
	}
	return providers.Info{}
}

// WritesSpecs reports whether a role writes specs (and so takes no --spec).
func WritesSpecs(role string) bool {
	r, err := agents.Lookup(role)
	return err == nil && r.Scope == agents.ScopeSpecs
}

func spentOf(s Spend) float64 {
	if s.CostUSD == nil {
		return 0
	}
	return *s.CostUSD
}

// money prints an amount the way providers.Usage does: no scientific
// notation, no tail of zeros.
func money(v float64) string {
	s := strconv.FormatFloat(v, 'f', 4, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
