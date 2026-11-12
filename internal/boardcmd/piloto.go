package boardcmd

import (
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
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
	return PilotDecision{}
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
	return ""
}

// PedidoRondaRefutador is the refutador's pedido of round n
// (ReglaRondaRefutador), with ids joined with ", ":
//
//	Ronda <n> de hallazgos de <slug>. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: <ids>. No edites codigo.
func PedidoRondaRefutador(n int, slug string, ids []string) string {
	return ""
}

// PedidoRondaTestWriter is the test-writer's pedido of round n
// (ReglaRondaTestWriter): the first line, then one line per finding, in the
// order given:
//
//	Ronda <n> de hallazgos de <slug>. Por cada hallazgo escribe un test que falle hoy y cita su id en el test.
//	<id> [<severidad>] <file>: <descripcion>
func PedidoRondaTestWriter(n int, slug string, hallazgos []finding.Item) string {
	return ""
}

// PedidoRondaWriter is the writer's pedido of round n (ReglaRondaWriter),
// with rutas joined with ", ":
//
//	Ronda <n> de hallazgos de <slug>. Estos tests fallan y cada uno cita su hallazgo: <rutas>. Corrige el codigo hasta que pasen. No toques esos tests.
func PedidoRondaWriter(n int, slug string, rutas []string) string {
	return ""
}
