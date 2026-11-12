package boardcmd

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// The steps of a PilotDecision (autonomia-entre-firmas).
const (
	// PasoLanzar launches Action with Role and Provider through lanzar.
	PasoLanzar = "lanzar"
	// PasoCerrarHallazgos closes Hallazgos as corregido, without a model and
	// outside a run (whoever runs the pilot does it: see pilotocmd).
	PasoCerrarHallazgos = "cerrar-hallazgos"
	// PasoDetenerse stops the pilot; Why says why.
	PasoDetenerse = "detenerse"
)

// The rules of the pilot: why it launches what it launches. They go to the
// record of every envelope it launches (envelope.Record.Regla) and to the
// line `piloto automatico: regla <regla>`.
const (
	// ReglaCadena: none of the table's cases held; the action of the
	// column's table.
	ReglaCadena = "cadena"
	// ReglaReintento: the writer again after its red, in a new session
	// (case 6).
	ReglaReintento = "reintento"
	// ReglaRelevoPorTiempo: the role again after its run died by its
	// timeout (case 5).
	ReglaRelevoPorTiempo = "relevo-por-tiempo"
	// The rules of a round of findings (case 7), in the order a round runs
	// them; ReglaRondaCierre is the rule of PasoCerrarHallazgos.
	ReglaRondaRefutador  = "ronda-refutador"
	ReglaRondaTestWriter = "ronda-test-writer"
	ReglaRondaWriter     = "ronda-writer"
	ReglaRondaReview     = "ronda-review"
	ReglaRondaCierre     = "ronda-cierre"
)

// Estado is everything Piloto needs about one card, already read by whoever
// runs the pilot (pilotocmd.Armar): Piloto itself reads neither the disk nor
// the clock.
type Estado struct {
	// Tarjeta is the card as Derive gives it (its item with auto and
	// presupuesto_usd, column, actions and PilotActions, running or
	// interrupted job, red, next step).
	Tarjeta Card
	// Sobres are the card's CLOSED envelopes (envelope.Record.Done), oldest
	// first, each with its record and its verdict.
	Sobres []Sobre
	// Veredicto is the card's newest complete verdict, with the status of
	// every gate; nil = none.
	Veredicto *verdict.Verdict
	// Reviews are the card's review records, oldest first.
	Reviews []reviewcmd.Record
	// Bloqueantes are the OPEN findings of the card's task that block under
	// finding.Bloquea, in the order of the card's findings.
	Bloqueantes []finding.Item
	// Citas are, by finding id of Bloqueantes, the protected tests of the
	// card's task (agentcmd.TestsProtegidos) whose file text contains that
	// id, sorted. A finding no protected test cites has no entry.
	Citas map[string][]string
	// Politica is the project's autonomia (manifest.Manifest.AutonomiaOrDefault).
	Politica manifest.AutonomiaPolicy
	// Ventanas are, by provider name, the newest "5h" window datum of the
	// sidecars of the card's runs (runcmd.Meta.Ventanas). A provider without
	// a datum has no entry.
	Ventanas map[string]runcmd.VentanaRun
	// Proveedores are the providers of the machine (providers.Detect): what
	// each one declares (Capabilities.Resume, Capabilities.Budget).
	Proveedores []providers.Info
	// Hora is the time the pilot decides at.
	Hora time.Time
}

// Sobre is one closed envelope of the card as Piloto sees it.
type Sobre struct {
	Record envelope.Record
	// Veredicto is the verdict the envelope emitted (Record.VerdictID), with
	// the status of every gate; nil = it emitted none.
	Veredicto *verdict.Verdict
	// SesionID is the provider's session id the sidecar of the envelope's
	// run kept (runcmd.Meta.ProviderSessionID); "" = none.
	SesionID string
}

// Piloto is the pilot's decision (autonomia-entre-firmas): it replaces Pilot.
// It is pure — the same Estado gives the same decision, and it reads neither
// the disk nor the clock (e.Hora is its time) — and it never decides to sign
// the spec, to integrate, or to launch the arquitecto: for no Estado is the
// decision aprobar, integrar or pedir-arquitecto.
//
// It decides by the first case of the spec's table that holds ("La tabla del
// piloto"), and Caso says which. The cases that say "como hoy" stop with
// Pilot's exact Why:
//
//  1. auto is not hasta-humano: `el piloto automatico no esta activado en la tarjeta`;
//  2. a job running or interrupted: `la tarjeta tiene otro trabajo en curso o interrumpido`;
//  3. a human column or Hecho: `la tarjeta llego a <Columna>: le toca a una persona`,
//     or `la tarjeta llego a Hecho: no queda trabajo`;
//  4. no open task: `el piloto corre en el espacio de trabajo de la tarjeta: abre la tarea con hoom task start <slug>`;
//  5. the newest envelope closed with `corte: tiempo`: the relevo (ReglaRelevoPorTiempo);
//  6. reintentos_ante_rojo is 1 and the newest envelope is the writer's, out of
//     a round, closed in verify or check with a red verdict: the retry
//     (ReglaReintento);
//  7. rondas_de_hallazgos above 0 and either the newest envelope is the
//     reviewer's with blocking findings, or it carries `ronda`: the round of
//     findings (ReglaRonda*);
//  8. the newest envelope did not close entregable: Pilot's case 2,
//     `el trabajo del <rol> no cerro entregable (<status>): el piloto se detiene ante un rojo`;
//  9. the card in red: `la tarjeta esta en rojo: <plain>`;
//  10. the column is not after the newest envelope's `columna`:
//     `la tarjeta no avanzo (sigue en <Columna>): el piloto no reintenta`;
//  11. the primary action is not the table's, its role is the newest
//     envelope's, or it is disabled: Pilot's cases 7, 8 and 9;
//  12. no presupuesto_usd: `la tarjeta no tiene presupuesto: el piloto automatico solo corre con un tope`;
//  13. no provider of the action is ok and has a cap:
//     `ningun proveedor con tope de presupuesto puede tomar este trabajo`;
//  14. the chosen provider's 5 h window reached ventana_max:
//     `la ventana de 5h de <provider> va en <x>% (tope <y>%): el piloto espera; reinicia <hora UTC>`.
//
// With none it launches the column's table action (test-writer, writer or
// reviewer: the table no longer has the Arquitecto column) with ReglaCadena.
// Every launch, from whatever case, also goes through cases 12, 13 and 14. A
// provider "has a cap" when it declares a USD cap or autonomia.ventana_max is
// declared; in the second case it launches without a USD budget
// (BudgetUSD 0). A review launch has Delta when the card's next step
// (Card.Next) proposes `hoom review ... --delta`, and always inside a round.
//
// The expected red of the test-writer (its envelope closed in verify or
// check with a red verdict in which spec_lint, spec_trace and spec_approved
// are pass) and the envelope that ends a round count as entregable and
// leave the card out of red for cases 8 to 11. Without closed envelopes
// cases 5 to 8 and 10 do not apply and 11 does not compare with the newest
// envelope's role; a newest envelope without `columna` does not apply to
// case 10.
//
// The Why texts of cases 5, 6 and 7, the pedidos (PedidoReintento,
// PedidoRelevo, PedidoRondaRefutador, PedidoRondaTestWriter,
// PedidoRondaWriter) and who runs each step of a round are the spec's
// sections "Corte por tiempo", "Reintento ante un rojo" and "La ronda de
// hallazgos", literally. A launch of the pilot never carries text that does
// not come from a template, an id, a path, a gate name, a finding on disk
// or the `pedido` of the envelope it repeats. Lists inside a Why or a pedido
// (<gates>, <ids>, <rutas>) are joined with ", ".
func Piloto(e Estado) PilotDecision {
	p := piloto{e: e, c: e.Tarjeta}
	return p.decidir()
}

// tablaPiloto is the pilot's table of columns (autonomia-entre-firmas):
// PilotTable without the Arquitecto column.
var tablaPiloto = map[string]string{
	ColTestWriter: ActPedirTestWriter,
	ColWriter:     ActPedirWriter,
	ColReview:     ActPedirReviewer,
}

// accionDeRol is the pedir-* action that launches each role the pilot
// launches.
var accionDeRol = map[string]string{
	"test-writer": ActPedirTestWriter,
	"writer":      ActPedirWriter,
	"reviewer":    ActPedirReviewer,
	"refutador":   ActPedirRefutador,
}

// The gates of the expected red of the test-writer.
var gatesDelSpec = []string{"spec_lint", "spec_trace", "spec_approved"}

// gateHallazgos is the gate a round of findings closes (finding.GateName).
const gateHallazgos = finding.GateName

// piloto is one decision of Piloto: the Estado and its card.
type piloto struct {
	e Estado
	c Card
}

// lanzamiento is what a launch of the pilot asks, before cases 12 to 14.
type lanzamiento struct {
	caso        int
	accion      string
	provider    string // "" = the cadena's choice
	regla       string
	pedido      string
	intento     int
	ronda       int
	resume      string
	delta       bool
	hallazgos   []string
	bloqueantes int
}

func parar(caso int, why string) PilotDecision {
	return PilotDecision{Paso: PasoDetenerse, Why: why, Caso: caso}
}

func (p piloto) nombreColumna() string {
	for _, col := range Columns {
		if col.ID == p.c.Column {
			return col.Name
		}
	}
	return p.c.Column
}

func (p piloto) ultimo() (Sobre, bool) {
	if len(p.e.Sobres) == 0 {
		return Sobre{}, false
	}
	return p.e.Sobres[len(p.e.Sobres)-1], true
}

func rolDe(rec envelope.Record) string {
	if rec.Role == "" {
		return "rol"
	}
	return rec.Role
}

// noEntregable is the Why of Pilot's case 2 (the spec's case 8).
func noEntregable(rec envelope.Record) string {
	status := rec.Status
	if status == "" {
		status = "sin cerrar"
	}
	return "el trabajo del " + rolDe(rec) + " no cerro entregable (" + status + "): el piloto se detiene ante un rojo"
}

func (p piloto) decidir() PilotDecision {
	c := p.c
	if c.Item.Auto != item.AutoHastaHumano {
		return parar(1, "el piloto automatico no esta activado en la tarjeta")
	}
	if c.Running != nil || c.Interrupted != nil {
		return parar(2, "la tarjeta tiene otro trabajo en curso o interrumpido")
	}
	if c.Column == ColHecho {
		return parar(3, "la tarjeta llego a Hecho: no queda trabajo")
	}
	if isHumanColumn(c.Column) {
		return parar(3, "la tarjeta llego a "+p.nombreColumna()+": le toca a una persona")
	}
	if c.Evidence.Source != SourceWorktree {
		return parar(4, "el piloto corre en el espacio de trabajo de la tarjeta: abre la tarea con hoom task start "+c.Slug)
	}
	last, hay := p.ultimo()
	// the newest envelope counts as entregable and leaves the card out of
	// red for cases 8 to 11: the test-writer's expected red, or the one
	// that ends a round
	pasa := false
	if hay {
		if last.Record.Corte == envelope.CorteTiempo {
			return p.relevo(last)
		}
		if p.e.Politica.ReintentosAnteRojo == 1 && last.Record.Role == "writer" && last.Record.Ronda == 0 &&
			enVerifyOCheck(last.Record) && rojo(last) {
			return p.reintento(last)
		}
		if p.e.Politica.RondasDeHallazgos > 0 &&
			((last.Record.Role == "reviewer" && len(p.e.Bloqueantes) > 0) || last.Record.Ronda > 0) {
			d, sigue := p.ronda(last)
			if !sigue {
				return d
			}
			pasa = true
		}
		pasa = pasa || rojoEsperado(last)
		if !pasa && last.Record.Status != envelope.StatusDeliverable {
			return parar(8, noEntregable(last.Record))
		}
	}
	if c.Red != nil && !pasa {
		return parar(9, "la tarjeta esta en rojo: "+c.Red.Plain)
	}
	name := p.nombreColumna()
	if hay && last.Record.Columna != "" && columnIndex(c.Column) <= columnIndex(last.Record.Columna) {
		return parar(10, "la tarjeta no avanzo (sigue en "+name+"): el piloto no reintenta")
	}
	want, enTabla := tablaPiloto[c.Column]
	if !enTabla || len(c.Actions) == 0 || c.Actions[0].ID != want {
		return parar(11, "lo que sigue en "+name+" no esta en la tabla del piloto: le toca a una persona")
	}
	a := c.Actions[0]
	if hay && a.Role == last.Record.Role {
		return parar(11, "seria volver a pedirle al "+rolDe(last.Record)+": el piloto no reintenta")
	}
	if !a.Enabled {
		return parar(11, a.Why)
	}
	l := lanzamiento{accion: a.ID, regla: ReglaCadena, pedido: a.Pedido}
	if a.Role == "reviewer" {
		l.pedido = ""
		l.delta = strings.Contains(c.Next, "--delta")
	}
	return p.lanzar(l, a)
}

// accion is the card's action id: the one it offers a person or, if not,
// its pilot action — the same lookup lanzar does.
func (p piloto) accion(id string) (Action, bool) {
	if a, ok := p.c.Action(id); ok {
		return a, true
	}
	return p.c.PilotAction(id)
}

// lanzarRol launches l with the card's action of l.accion: a launch from
// the cases 5, 6 and 7, which name their role and provider.
func (p piloto) lanzarRol(l lanzamiento) PilotDecision {
	a, ok := p.accion(l.accion)
	if !ok {
		return parar(11, "lo que sigue en "+p.nombreColumna()+" no esta en la tabla del piloto: le toca a una persona")
	}
	if !a.Enabled {
		return parar(11, a.Why)
	}
	return p.lanzar(l, a)
}

// lanzar takes every launch through cases 12, 13 and 14.
func (p piloto) lanzar(l lanzamiento, a Action) PilotDecision {
	c := p.c
	if c.Item.PresupuestoUSD == nil || a.BudgetUSD == nil {
		return parar(12, "la tarjeta no tiene presupuesto: el piloto automatico solo corre con un tope")
	}
	ventana := p.e.Politica.VentanaMax != nil
	conTope := func(o ProviderOption) bool { return o.OK && (o.Budget || ventana) }
	var elegido *ProviderOption
	elegir := func(name string) {
		for i := range a.Providers {
			if o := a.Providers[i]; elegido == nil && conTope(o) && o.Name == name {
				elegido = &a.Providers[i]
			}
		}
	}
	if l.provider != "" {
		elegir(l.provider)
	} else {
		if last, hay := p.ultimo(); hay {
			elegir(last.Record.Provider)
		}
		for i := range a.Providers {
			if elegido == nil && conTope(a.Providers[i]) {
				elegido = &a.Providers[i]
			}
		}
	}
	if elegido == nil {
		return parar(13, "ningun proveedor con tope de presupuesto puede tomar este trabajo")
	}
	if why := p.ventanaLlena(elegido.Name); why != "" {
		return parar(14, why)
	}
	d := PilotDecision{
		Launch: true, Paso: PasoLanzar, Action: a.ID, Role: a.Role, Provider: elegido.Name,
		Pedido: l.pedido, Regla: l.regla, Intento: l.intento, Ronda: l.ronda, Resume: l.resume,
		Delta: l.delta, Hallazgos: l.hallazgos, Bloqueantes: l.bloqueantes, Caso: l.caso,
	}
	if elegido.Budget {
		d.BudgetUSD = *a.BudgetUSD
	}
	return d
}

// ventanaLlena is case 14: the newest current 5 h datum of provider at or
// above ventana_max. A datum without its reset time, or whose reset already
// passed, is not current: without one it launches.
func (p piloto) ventanaLlena(provider string) string {
	tope := p.e.Politica.VentanaMax
	if tope == nil {
		return ""
	}
	v, ok := p.e.Ventanas[provider]
	if !ok || v.ReiniciaEn.IsZero() || !v.ReiniciaEn.After(p.e.Hora) {
		return ""
	}
	if v.Fin < float64(*tope) {
		return ""
	}
	return fmt.Sprintf("la ventana de 5h de %s va en %d%% (tope %d%%): el piloto espera; reinicia %s",
		provider, int(math.Round(v.Fin)), *tope, v.ReiniciaEn.UTC().Format(time.RFC3339))
}

// relevo is case 5: the newest envelope's run died by its timeout.
func (p piloto) relevo(last Sobre) PilotDecision {
	rec := last.Record
	rol := rolDe(rec)
	k := 0
	for _, s := range p.e.Sobres {
		if s.Record.Role == rec.Role && s.Record.Corte == envelope.CorteTiempo {
			k++
		}
	}
	if k >= 2 {
		clave := "agents." + rol + ".timeout"
		if rec.Role == "reviewer" {
			clave = "review.timeout"
		}
		return parar(5, "el "+rol+" se corto por tiempo dos veces: sube "+clave+" o parte la tarea")
	}
	accion, ok := accionDeRol[rec.Role]
	if !ok {
		return parar(5, noEntregable(rec))
	}
	l := lanzamiento{caso: 5, accion: accion, provider: rec.Provider, regla: ReglaRelevoPorTiempo,
		intento: rec.Intento, ronda: rec.Ronda, hallazgos: copia(rec.Hallazgos)}
	switch {
	case rec.Role == "reviewer":
		l.delta = rec.Delta
	case !ciego(rec) && p.retoma(rec.Provider) && last.SesionID != "":
		l.resume, l.pedido = last.SesionID, PedidoRelevo
	case strings.TrimSpace(rec.Pedido) == "":
		return parar(5, noEntregable(rec))
	default:
		l.pedido = rec.Pedido
	}
	return p.lanzarRol(l)
}

// ciego says whether the envelope's role runs blind.
func ciego(rec envelope.Record) bool {
	if rec.Isolated {
		return true
	}
	r, err := agents.Lookup(rec.Role)
	return err == nil && r.Isolated
}

// retoma says whether the provider declares Resume on this machine.
func (p piloto) retoma(name string) bool {
	for _, info := range p.e.Proveedores {
		if info.Name == name {
			return info.Capabilities.Resume
		}
	}
	return false
}

// reintento is case 6: the writer's red, out of a round.
func (p piloto) reintento(last Sobre) PilotDecision {
	v := veredictoDe(last)
	if v != nil {
		for _, g := range v.Gates {
			if g.Required && g.Status == verdict.StatusError {
				return parar(6, "entorno: el gate "+g.Name+" no pudo correr: no se reintenta con modelos")
			}
		}
	}
	n := 0
	for _, s := range p.e.Sobres {
		if s.Record.Role == "writer" && s.Record.Ronda == 0 {
			n++
		}
	}
	id := last.Record.VerdictID
	if v != nil && v.ID != "" {
		id = v.ID
	}
	gates := gatesEnRojo(v)
	if n >= 2 {
		return parar(6, fmt.Sprintf("rojo final: %d intentos del writer y el veredicto %s sigue rojo en %s",
			n, id, strings.Join(gates, ", ")))
	}
	return p.lanzarRol(lanzamiento{caso: 6, accion: ActPedirWriter, provider: last.Record.Provider,
		regla: ReglaReintento, intento: 2, pedido: PedidoReintento(p.c.Slug, id, gates)})
}

// ronda is case 7. sigue is true when the round ends and the table goes
// on (cases 8 onward, with the newest envelope counting as entregable).
func (p piloto) ronda(last Sobre) (d PilotDecision, sigue bool) {
	rec := last.Record
	n := rec.Ronda
	switch rec.Role {
	case "reviewer":
		if rec.Status != envelope.StatusDeliverable {
			return parar(7, noEntregable(rec)), false
		}
		if len(p.e.Bloqueantes) > 0 {
			return p.empezarRonda(), false
		}
		return PilotDecision{}, true
	case "refutador":
		if rec.Status != envelope.StatusDeliverable {
			return parar(7, noEntregable(rec)), false
		}
		if len(p.e.Bloqueantes) == 0 {
			return PilotDecision{}, true
		}
		return p.testWriterDeRonda(n, 0), false
	case "test-writer":
		if rojoEsperado(last) {
			return p.lanzarConProvider(lanzamiento{caso: 7, accion: ActPedirWriter, regla: ReglaRondaWriter,
				ronda: n, pedido: PedidoRondaWriter(n, p.c.Slug, rec.Trasplantados)}, "writer"), false
		}
		if v := veredictoDe(last); rec.Status == envelope.StatusDeliverable && v != nil && v.Verdict == "green" {
			return parar(7, fmt.Sprintf("los tests de la ronda %d no fallan: los hallazgos no se reproducen: te toca", n)), false
		}
		return parar(7, noEntregable(rec)), false
	case "writer":
		return p.writerDeRonda(last), false
	}
	return parar(7, noEntregable(rec)), false
}

// empezarRonda starts round n: the newest envelope is the reviewer's and
// findings block.
func (p piloto) empezarRonda() PilotDecision {
	n := 1
	for _, s := range p.e.Sobres {
		if s.Record.Ronda+1 > n {
			n = s.Record.Ronda + 1
		}
	}
	k := len(p.e.Bloqueantes)
	if n > p.e.Politica.RondasDeHallazgos {
		return parar(7, fmt.Sprintf("se hicieron %d rondas de hallazgos y quedan %d que bloquean: te toca", n-1, k))
	}
	if n >= 2 {
		if kAntes, ok := p.bloqueantesAlEmpezar(n - 1); ok && k >= kAntes {
			return parar(7, fmt.Sprintf("los hallazgos que bloquean no bajan (%d -> %d): te toca", kAntes, k))
		}
	}
	refutados := map[string]bool{}
	for _, s := range p.e.Sobres {
		if s.Record.Role == "refutador" && s.Record.Stage != "run" {
			for _, id := range s.Record.Hallazgos {
				refutados[id] = true
			}
		}
	}
	var ids []string
	for _, f := range p.e.Bloqueantes {
		if !refutados[f.ID] {
			ids = append(ids, f.ID)
		}
	}
	if len(ids) == 0 {
		return p.testWriterDeRonda(n, k)
	}
	return p.lanzarConProvider(lanzamiento{caso: 7, accion: ActPedirRefutador, regla: ReglaRondaRefutador,
		ronda: n, hallazgos: ids, bloqueantes: k, pedido: PedidoRondaRefutador(n, p.c.Slug, ids)}, "refutador")
}

// bloqueantesAlEmpezar is k of round n: the bloqueantes of its first
// envelope that has them.
func (p piloto) bloqueantesAlEmpezar(n int) (int, bool) {
	for _, s := range p.e.Sobres {
		if s.Record.Ronda == n && s.Record.Bloqueantes > 0 {
			return s.Record.Bloqueantes, true
		}
	}
	return 0, false
}

// testWriterDeRonda launches the round's test-writer with the findings
// that block now; bloqueantes > 0 when it is the round's first launch.
func (p piloto) testWriterDeRonda(n, bloqueantes int) PilotDecision {
	ids := make([]string, 0, len(p.e.Bloqueantes))
	for _, f := range p.e.Bloqueantes {
		ids = append(ids, f.ID)
	}
	return p.lanzarConProvider(lanzamiento{caso: 7, accion: ActPedirTestWriter, regla: ReglaRondaTestWriter,
		ronda: n, hallazgos: ids, bloqueantes: bloqueantes,
		pedido: PedidoRondaTestWriter(n, p.c.Slug, p.e.Bloqueantes)}, "test-writer")
}

// writerDeRonda is the step after the round's writer.
func (p piloto) writerDeRonda(last Sobre) PilotDecision {
	rec := last.Record
	n := rec.Ronda
	if !enVerifyOCheck(rec) && rec.Stage != "ok" {
		return parar(7, noEntregable(rec))
	}
	v := p.e.Veredicto
	if v == nil {
		v = last.Veredicto
	}
	if v == nil {
		return parar(7, noEntregable(rec))
	}
	if v.Verdict == "green" {
		return p.lanzarConProvider(lanzamiento{caso: 7, accion: ActPedirReviewer, regla: ReglaRondaReview,
			ronda: n, delta: true}, "reviewer")
	}
	if !rojoSoloPorHallazgos(v) {
		return parar(7, fmt.Sprintf("el arreglo de la ronda %d quedo rojo en %s: te toca", n, strings.Join(gatesEnRojo(v), ", ")))
	}
	deLaRonda := trasplantadosDeRonda(p.e, n)
	antes := ""
	for _, s := range p.e.Sobres {
		if s.Record.Role == "test-writer" && s.Record.Ronda == n {
			if s.Record.VerdictID != "" {
				antes = s.Record.VerdictID
			}
			if s.Veredicto != nil && s.Veredicto.ID != "" {
				antes = s.Veredicto.ID
			}
		}
	}
	var ids, sinTest []string
	evidencias := map[string]string{}
	for _, f := range p.e.Bloqueantes {
		ruta := citaDeRonda(p.e, f.ID, deLaRonda)
		if ruta == "" {
			sinTest = append(sinTest, f.ID)
			continue
		}
		ids = append(ids, f.ID)
		evidencias[f.ID] = fmt.Sprintf("ronda %d: el test %s cita el hallazgo; veredicto %s rojo antes del arreglo; despues, veredicto %s con todos los demas gates en pass",
			n, ruta, antes, v.ID)
	}
	if len(ids) == 0 {
		return parar(7, fmt.Sprintf("quedan hallazgos sin test en la ronda %d: %s: te toca", n, strings.Join(sinTest, ", ")))
	}
	return PilotDecision{Paso: PasoCerrarHallazgos, Regla: ReglaRondaCierre, Ronda: n, Hallazgos: ids,
		Evidencias: evidencias, Caso: 7}
}

// SinTestDeRonda are the ids of the card's blocking findings
// (e.Bloqueantes) that no protected test of round n cites: none of the
// paths in e.Citas[id] is among the trasplantados of the round's test-writer.
// After cerrar-hallazgos, a red verdict with any of them stops with
// `quedan hallazgos sin test en la ronda <n>: <ids>: te toca`.
func SinTestDeRonda(e Estado, n int) []string {
	deLaRonda := trasplantadosDeRonda(e, n)
	var out []string
	for _, f := range e.Bloqueantes {
		if citaDeRonda(e, f.ID, deLaRonda) == "" {
			out = append(out, f.ID)
		}
	}
	return out
}

// trasplantadosDeRonda are the paths the test-writer of round n
// transplanted: the round's protected tests.
func trasplantadosDeRonda(e Estado, n int) map[string]bool {
	out := map[string]bool{}
	for _, s := range e.Sobres {
		if s.Record.Role == "test-writer" && s.Record.Ronda == n {
			for _, r := range s.Record.Trasplantados {
				out[r] = true
			}
		}
	}
	return out
}

// citaDeRonda is the first test in e.Citas[id] that is among deLaRonda; ""
// when none is.
func citaDeRonda(e Estado, id string, deLaRonda map[string]bool) string {
	for _, r := range e.Citas[id] {
		if deLaRonda[r] {
			return r
		}
	}
	return ""
}

// lanzarConProvider launches a step of a round with the provider the disk
// says: the newest review record's for the refutador and the reviewer, the
// newest envelope of the role's for the test-writer and the writer.
func (p piloto) lanzarConProvider(l lanzamiento, rol string) PilotDecision {
	switch rol {
	case "refutador", "reviewer":
		if len(p.e.Reviews) > 0 {
			l.provider = p.e.Reviews[len(p.e.Reviews)-1].Provider
		}
	default:
		for _, s := range p.e.Sobres {
			if s.Record.Role == rol {
				l.provider = s.Record.Provider
			}
		}
	}
	if l.provider == "" {
		return parar(7, "la tarjeta no tiene un sobre de "+rol+": el piloto no elige provider por su cuenta")
	}
	return p.lanzarRol(l)
}

func enVerifyOCheck(rec envelope.Record) bool { return rec.Stage == "verify" || rec.Stage == "check" }

// veredictoDe is the envelope's verdict, nil when it emitted none.
func veredictoDe(s Sobre) *verdict.Verdict { return s.Veredicto }

// rojo says whether the envelope's verdict is red.
func rojo(s Sobre) bool {
	if s.Veredicto != nil {
		return s.Veredicto.Verdict == "red"
	}
	return s.Record.Verdict == "red"
}

// rojoEsperado is the test-writer's expected red: closed in verify or check
// with a red verdict (so with no scope violation: those close in scope) in
// which the spec's gates are pass.
func rojoEsperado(s Sobre) bool {
	if s.Record.Role != "test-writer" || !enVerifyOCheck(s.Record) || s.Veredicto == nil || s.Veredicto.Verdict != "red" {
		return false
	}
	for _, name := range gatesDelSpec {
		if estadoDe(s.Veredicto, name) != verdict.StatusPass {
			return false
		}
	}
	return true
}

func estadoDe(v *verdict.Verdict, name string) string {
	for _, g := range v.Gates {
		if g.Name == name {
			return g.Status
		}
	}
	return ""
}

// rojoSoloPorHallazgos: findings_open in fail and every other required gate
// in pass.
func rojoSoloPorHallazgos(v *verdict.Verdict) bool {
	hallazgos := false
	for _, g := range v.Gates {
		switch {
		case g.Name == gateHallazgos:
			hallazgos = g.Status == verdict.StatusFail
		case g.Required && g.Status != verdict.StatusPass:
			return false
		}
	}
	return hallazgos
}

// gatesEnRojo are the required gates that make v red (fail, or error), in
// the verdict's order.
func gatesEnRojo(v *verdict.Verdict) []string {
	if v == nil {
		return nil
	}
	var out []string
	for _, g := range v.Gates {
		if g.Required && (g.Status == verdict.StatusFail || g.Status == verdict.StatusError) {
			out = append(out, g.Name)
		}
	}
	return out
}

func copia(s []string) []string {
	if len(s) == 0 {
		return nil
	}
	return append([]string(nil), s...)
}

// PedidoRelevo is the pedido of a relevo-por-tiempo that resumes the role's
// session (PilotDecision.Resume).
const PedidoRelevo = "Te cortaste por tiempo. Continua donde quedaste y termina el pedido original."

// PedidoReintento is the writer's pedido of ReglaReintento:
//
//	Intento 2 de <slug>. El veredicto <id> quedo ROJO en: <gates>. La salida de cada gate esta en .hoom/verdicts/<id>.json. Corrige sobre el arbol actual hasta que verify quede verde. No toques los tests del test-writer.
//
// with gates joined with ", ".
func PedidoReintento(slug, veredicto string, gates []string) string {
	return fmt.Sprintf("Intento 2 de %s. El veredicto %s quedo ROJO en: %s. La salida de cada gate esta en .hoom/verdicts/%s.json. Corrige sobre el arbol actual hasta que verify quede verde. No toques los tests del test-writer.",
		slug, veredicto, strings.Join(gates, ", "), veredicto)
}

// PedidoRondaRefutador is the refutador's pedido of round n
// (ReglaRondaRefutador), with ids joined with ", ":
//
//	Ronda <n> de hallazgos de <slug>. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: <ids>. No edites codigo.
func PedidoRondaRefutador(n int, slug string, ids []string) string {
	return fmt.Sprintf("Ronda %d de hallazgos de %s. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: %s. No edites codigo.",
		n, slug, strings.Join(ids, ", "))
}

// PedidoRondaTestWriter is the test-writer's pedido of round n
// (ReglaRondaTestWriter): the first line, then one line per finding, in the
// order given:
//
//	Ronda <n> de hallazgos de <slug>. Por cada hallazgo escribe un test que falle hoy y cita su id en el test.
//	<id> [<severidad>] <file>: <descripcion>
func PedidoRondaTestWriter(n int, slug string, hallazgos []finding.Item) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Ronda %d de hallazgos de %s. Por cada hallazgo escribe un test que falle hoy y cita su id en el test.", n, slug)
	for _, f := range hallazgos {
		fmt.Fprintf(&b, "\n%s [%s] %s: %s", f.ID, f.Severity, f.File, f.Description)
	}
	return b.String()
}

// PedidoRondaWriter is the writer's pedido of round n (ReglaRondaWriter),
// with rutas joined with ", ":
//
//	Ronda <n> de hallazgos de <slug>. Estos tests fallan y cada uno cita su hallazgo: <rutas>. Corrige el codigo hasta que pasen. No toques esos tests.
func PedidoRondaWriter(n int, slug string, rutas []string) string {
	return fmt.Sprintf("Ronda %d de hallazgos de %s. Estos tests fallan y cada uno cita su hallazgo: %s. Corrige el codigo hasta que pasen. No toques esos tests.",
		n, slug, strings.Join(rutas, ", "))
}
