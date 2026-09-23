// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md, pieza
// 6 (CA-379, CA-380): la decision del piloto automatico, boardcmd.Pilot. La
// cinta es una cinta transportadora, no un orquestador: pura, sin juicio y
// sin reintentos. Se detiene en el primer caso de la tabla que se cumple, con
// su Why exacto, y si no, lanza la accion de la tabla con el provider, el
// presupuesto y el pedido que propone la accion. Las tarjetas salen de
// Derive sobre Evidence literales (sin disco, sin reloj, sin git) y el
// registro del sobre que cerro se arma a mano.
package boardcmd

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Los Why del contrato (sin acentos, como los emite el binario).
const (
	piWhyNoActivado     = "el piloto automatico no esta activado en la tarjeta"
	piWhyOtroTrabajo    = "la tarjeta tiene otro trabajo en curso o interrumpido"
	piWhyHecho          = "la tarjeta llego a Hecho: no queda trabajo"
	piWhySinPresupuesto = "la tarjeta no tiene presupuesto: el piloto automatico solo corre con un tope"
	piWhySinProveedor   = "ningun proveedor con tope de presupuesto puede tomar este trabajo"
)

func piWhyNoEntregable(rol, status string) string {
	return "el trabajo del " + rol + " no cerro entregable (" + status + "): el piloto se detiene ante un rojo"
}
func piWhyRojo(plain string) string { return "la tarjeta esta en rojo: " + plain }
func piWhyPersona(nombre string) string {
	return "la tarjeta llego a " + nombre + ": le toca a una persona"
}
func piWhyNoAvanzo(nombre string) string {
	return "la tarjeta no avanzo (sigue en " + nombre + "): el piloto no reintenta"
}
func piWhyFueraDeTabla(nombre string) string {
	return "lo que sigue en " + nombre + " no esta en la tabla del piloto: le toca a una persona"
}
func piWhyReintento(rol string) string {
	return "seria volver a pedirle al " + rol + ": el piloto no reintenta"
}

// piTabla es la tabla del contrato, copiada aca a proposito: si PilotTable
// cambia, los tests lo ven.
var piTabla = map[string]string{
	ColArquitecto: ActPedirArquitecto,
	ColTestWriter: ActPedirTestWriter,
	ColWriter:     ActPedirWriter,
	ColReview:     ActPedirReviewer,
}

// piProvs son los providers de la maquina de los fixtures, en el orden del
// registro: claude (con tope, minimo 0.5 por run), codex (sin tope), gemini
// (en estos fixtures acepta tope y no tiene minimo) y opencode (no recibe el
// contrato del rol: nunca es ok).
func piProvs() []providers.Info {
	return []providers.Info{
		{Name: "claude", Installed: true, Bin: "/opt/bin/claude", MinBudgetUSD: 0.5,
			Capabilities: providers.Capabilities{Structured: true, Resume: true, SessionID: true, Model: true,
				SystemPrompt: true, Budget: true}},
		{Name: "codex", Installed: true, Bin: "/opt/bin/codex",
			Capabilities: providers.Capabilities{Structured: true, Resume: true, SessionID: true, Model: true,
				SystemPrompt: true}},
		{Name: "gemini", Installed: true, Bin: "/opt/bin/gemini",
			Capabilities: providers.Capabilities{Structured: true, SystemPrompt: true, Budget: true}},
		{Name: "opencode", Installed: true, Bin: "/opt/bin/opencode",
			Capabilities: providers.Capabilities{Continue: true}},
	}
}

// Las formas de las tarjetas de los fixtures: las ocho columnas, con Review
// en dos formas (review exigida: la accion principal es pedir-reviewer; con
// un hallazgo que bloquea: es pedir-writer, que no es la de la tabla).
var piFormas = []string{"backlog", "arquitecto", "tu-aprobacion", "test-writer", "writer",
	"review-exigida", "review-hallazgos", "tu-aceptacion", "hecho"}

// Presupuestos de los fixtures.
const (
	piSinTope = "sin presupuesto"
	piConTope = "con presupuesto"
	piAgotado = "presupuesto agotado"
)

var piTopes = []string{piSinTope, piConTope, piAgotado}

// Lo que la tarjeta tiene ademas de su columna.
const (
	piNada         = "nada"
	piRojo         = "rojo"
	piEnCurso      = "en curso"
	piInterrumpido = "interrumpido"
)

var piExtras = []string{piNada, piRojo, piEnCurso, piInterrumpido}

// piEv arma la evidencia de una tarjeta con auto: hasta-humano, con espacio
// de trabajo propio y un run del writer (claude) en la telemetria.
func piEv(forma, tope, extra string) Evidence {
	ev := acEv() // Tu aceptacion, con espacio de trabajo propio
	ev.Providers = piProvs()
	ev.Item.Auto = item.AutoHastaHumano
	switch forma {
	case "backlog":
		ev.SpecExists, ev.Approval, ev.Criteria = false, "", nil
	case "arquitecto":
		ev.LintIssues = []string{`falta la seccion "riesgos"`}
	case "tu-aprobacion":
		ev.Approval = approval.StatusNotApproved
	case "test-writer":
		ev.Criteria = []string{"CA-1", "CA-2", "CA-3", "CA-5"}
		ev.Untraced = []string{"CA-3", "CA-5"}
	case "writer":
		ev.Verdict, ev.GreenVerdicts = nil, nil
	case "review-exigida":
		ev = acGrande(ev)
	case "review-hallazgos":
		ev = acConHallazgos(ev, "f-alto")
	case "tu-aceptacion":
	case "hecho":
		cuando := bdT0.Add(48 * time.Hour)
		ev.Item.HechoEn = &cuando
	default:
		panic("forma desconocida " + forma)
	}
	var costo *float64
	switch tope {
	case piConTope:
		ev.Item.PresupuestoUSD, costo = bdF(10), bdF(1)
	case piAgotado:
		ev.Item.PresupuestoUSD, costo = bdF(10), bdF(12)
	}
	ev.Runs = append(ev.Runs, acGasto("run-writer", "writer", "claude", costo, time.Minute))
	switch extra {
	case piRojo: // un sobre de la tarjeta cerro no-entregable despues del ultimo veredicto
		fin := bdT0.Add(50 * time.Minute)
		ev.Envelopes = append(ev.Envelopes, EnvelopeState{Record: envelope.Record{ID: "env-rojo", Role: "test-writer",
			Provider: "claude", Task: bdSlug, Dir: bdWtDir, Stage: "verify", Step: 4, Steps: 5,
			Status: envelope.StatusNotDeliverable, Note: "fallo el gate test",
			StartedAt: bdT0.Add(40 * time.Minute), UpdatedAt: fin, EndedAt: fin}})
	case piEnCurso:
		ev.Envelopes = append(ev.Envelopes, acSobreVivo("writer", "claude", "run", 3, 5))
	case piInterrumpido:
		ev = acInterrumpido(ev, "writer", "claude")
	}
	return ev
}

// piColEsperada es la columna de cada forma.
var piColEsperada = map[string]string{
	"backlog": ColBacklog, "arquitecto": ColArquitecto, "tu-aprobacion": ColTuAprobacion,
	"test-writer": ColTestWriter, "writer": ColWriter, "review-exigida": ColReview, "review-hallazgos": ColReview,
	"tu-aceptacion": ColTuAceptacion, "hecho": ColHecho,
}

// piCard deriva la tarjeta de una forma y exige que el fixture caiga donde
// dice.
func piCard(t *testing.T, forma, tope, extra string) Card {
	t.Helper()
	c := Derive(piEv(forma, tope, extra))
	if c.Column != piColEsperada[forma] {
		t.Fatalf("fixture %s/%s/%s: cae en %q, se esperaba %q (missing=%q)", forma, tope, extra, c.Column, piColEsperada[forma], c.Missing)
	}
	return c
}

func piSinAuto(c Card) Card {
	c.Item.Auto = ""
	return c
}

// piCerrado es el registro del sobre (o de la review) cuando la funcion del
// trabajo volvio.
func piCerrado(rol, provider, status string) envelope.Record {
	rec := envelope.Record{ID: "env-cerrado", RunID: "run-cerrado", Role: rol, Provider: provider, Task: bdSlug, Dir: bdWtDir,
		Stage: "ok", Step: 5, Steps: 5, Status: status, StartedAt: bdT0.Add(2 * time.Hour), UpdatedAt: bdT0.Add(3 * time.Hour)}
	if status != "" && status != envelope.StatusRunning {
		rec.EndedAt = rec.UpdatedAt
	}
	return rec
}

// piEvidencias son las evidencias de los fixtures del piloto (para los
// chequeos que valen en todos los fixtures de C4).
func piEvidencias() []acFix {
	var out []acFix
	for _, forma := range piFormas {
		for _, tope := range piTopes {
			for _, extra := range piExtras {
				out = append(out, acFix{"piloto " + forma + "/" + tope + "/" + extra, piEv(forma, tope, extra)})
			}
		}
	}
	return out
}

type piTarjeta struct {
	nombre string
	c      Card
}

// piTarjetas son todas las combinaciones: las nueve formas (ocho columnas),
// con y sin presupuesto (y agotado), sin nada, en rojo, con otro trabajo en
// curso o interrumpido.
func piTarjetas(t *testing.T) []piTarjeta {
	t.Helper()
	var out []piTarjeta
	for _, forma := range piFormas {
		for _, tope := range piTopes {
			for _, extra := range piExtras {
				c := piCard(t, forma, tope, extra)
				switch {
				case extra == piRojo && c.Red == nil:
					t.Fatalf("fixture %s/%s/%s: tiene que estar en rojo", forma, tope, extra)
				case extra == piEnCurso && c.Running == nil:
					t.Fatalf("fixture %s/%s/%s: tiene que tener running", forma, tope, extra)
				case extra == piInterrumpido && c.Interrupted == nil:
					t.Fatalf("fixture %s/%s/%s: tiene que tener interrupted", forma, tope, extra)
				case extra == piNada && (c.Red != nil || c.Running != nil || c.Interrupted != nil):
					t.Fatalf("fixture %s/%s/%s: no tiene rojo ni otro trabajo", forma, tope, extra)
				}
				out = append(out, piTarjeta{forma + "/" + tope + "/" + extra, c})
			}
		}
	}
	return out
}

func piIndice(col string) int {
	for i, c := range Columns {
		if c.ID == col {
			return i
		}
	}
	return -1
}

func piNombre(col string) string { return Columns[piIndice(col)].Name }

func piDetiene(t *testing.T, ca, nombre string, d PilotDecision, why string) {
	t.Helper()
	if d.Launch || d.Why != why {
		t.Fatalf("%s: [%s] el piloto se detiene con\n  why %q\nfue launch=%v why=%q (%+v)", ca, nombre, why, d.Launch, d.Why, d)
	}
}

// piLanza exige un lanzamiento de la accion de la tabla con lo que propone la
// accion de after.
func piLanza(t *testing.T, ca, nombre string, d PilotDecision, after Card, accion, rol, provider string, budget float64) {
	t.Helper()
	if !d.Launch || d.Why != "" {
		t.Fatalf("%s: [%s] el piloto lanza (why \"\"), fue launch=%v why=%q", ca, nombre, d.Launch, d.Why)
	}
	a, ok := after.Action(accion)
	if !ok {
		t.Fatalf("%s: [%s] el fixture tiene la accion %s", ca, nombre, accion)
	}
	if d.Action != accion || d.Role != rol || d.Provider != provider || !bdCasi(d.BudgetUSD, budget) {
		t.Fatalf("%s: [%s] lanza %s del %s con %s y %v USD, fue %s del %s con %s y %v USD",
			ca, nombre, accion, rol, provider, budget, d.Action, d.Role, d.Provider, d.BudgetUSD)
	}
	if a.BudgetUSD == nil || !bdCasi(d.BudgetUSD, *a.BudgetUSD) || d.Pedido != a.Pedido || d.Role != a.Role {
		t.Fatalf("%s: [%s] el presupuesto, el pedido y el rol son los que propone la accion (%v, %q, %q): %+v",
			ca, nombre, a.BudgetUSD, a.Pedido, a.Role, d)
	}
}

// CA-379: la tabla es el orden fijo de C1 para las columnas de rol, y nada
// mas.
func TestCA379_LaTablaDelPiloto(t *testing.T) {
	if !reflect.DeepEqual(PilotTable, piTabla) {
		t.Fatalf("CA-379: PilotTable es %v, fue %v", piTabla, PilotTable)
	}
}

// CA-379: Pilot se detiene en cada uno de los once casos con su Why exacto,
// un fixture por caso (el primero que se cumple).
func TestCA379_SeDetieneEnCadaCaso(t *testing.T) {
	ok := envelope.StatusDeliverable
	tw := piCard(t, "test-writer", piConTope, piNada)
	wr := piCard(t, "writer", piConTope, piNada)
	rex := piCard(t, "review-exigida", piConTope, piNada)
	rha := piCard(t, "review-hallazgos", piConTope, piNada)
	bl := piCard(t, "backlog", piConTope, piNada)

	// 3: la tarjeta en rojo por un veredicto rojo posterior
	rojoEv := piEv("writer", piConTope, piNada)
	rojoEv.Verdict = bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10,
		[]verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
	rojo := Derive(rojoEv)
	if rojo.Column != ColWriter || rojo.Red == nil || rojo.Red.Plain == "" {
		t.Fatalf("fixture: writer en rojo por veredicto: %s %+v", rojo.Column, rojo.Red)
	}
	// 7: Review sin accion principal (solo falta guardar el espacio de trabajo)
	sinAccion := Derive(tbListo(piEv("tu-aceptacion", piConTope, piNada), taskcmd.ReadySinGuardar, tbMsgSinGuardar))
	if sinAccion.Column != ColReview || (len(sinAccion.Actions) > 0 && sinAccion.Actions[0].ID == ActPedirReviewer) {
		t.Fatalf("fixture: Review sin accion de rol: %s %v", sinAccion.Column, acIDs(sinAccion))
	}
	// 9: presupuesto agotado, y sin espacio de trabajo propio
	agotado := piCard(t, "writer", piAgotado, piNada)
	twArbol := Derive(bdArbol(piEv("test-writer", piConTope, piNada)))
	wrArbol := Derive(bdArbol(piEv("writer", piConTope, piNada)))
	// 11: sin gemini, el unico otro provider para revisar es codex (sin tope)
	sinGeminiEv := piEv("review-exigida", piConTope, piNada)
	sinGeminiEv.Providers = acProvs()
	sinGemini := Derive(sinGeminiEv)
	if a := acAccion(t, "CA-379", sinGemini, ActPedirReviewer); !a.Enabled {
		t.Fatalf("fixture: pedir-reviewer habilitada con codex: %q", a.Why)
	}

	casos := []struct {
		nombre        string
		before, after Card
		closed        envelope.Record
		why           string
	}{
		{"1: la tarjeta no tiene auto", tw, piSinAuto(wr), piCerrado("test-writer", "claude", ok), piWhyNoActivado},
		{"1: la persona apago auto mientras corria el writer", wr, piSinAuto(rex), piCerrado("writer", "claude", ok), piWhyNoActivado},
		{"1: auto apagado le gana a un cierre no-entregable", tw, piSinAuto(wr), piCerrado("test-writer", "claude", envelope.StatusNotDeliverable), piWhyNoActivado},
		{"2: no-entregable", tw, wr, piCerrado("test-writer", "claude", envelope.StatusNotDeliverable),
			piWhyNoEntregable("test-writer", "no-entregable")},
		{"2: sin-entrega", tw, wr, piCerrado("test-writer", "claude", envelope.StatusNoDelivery),
			piWhyNoEntregable("test-writer", "sin-entrega")},
		{"2: sin status", tw, wr, piCerrado("test-writer", "claude", ""), piWhyNoEntregable("test-writer", "sin cerrar")},
		{"2: le gana a la tarjeta en rojo", tw, rojo, piCerrado("test-writer", "claude", envelope.StatusNotDeliverable),
			piWhyNoEntregable("test-writer", "no-entregable")},
		{"3: la tarjeta quedo en rojo", tw, rojo, piCerrado("test-writer", "claude", ok), piWhyRojo(rojo.Red.Plain)},
		{"4: otro trabajo en curso", tw, piCard(t, "writer", piConTope, piEnCurso), piCerrado("test-writer", "claude", ok), piWhyOtroTrabajo},
		{"4: un trabajo interrumpido", tw, piCard(t, "writer", piConTope, piInterrumpido), piCerrado("test-writer", "claude", ok), piWhyOtroTrabajo},
		{"5: Tu aprobacion", bl, piCard(t, "tu-aprobacion", piConTope, piNada), piCerrado("arquitecto", "claude", ok),
			piWhyPersona("Tu aprobacion")},
		{"5: Tu aceptacion", rex, piCard(t, "tu-aceptacion", piConTope, piNada), piCerrado("reviewer", "gemini", ok),
			piWhyPersona("Tu aceptacion")},
		{"5: Hecho", piCard(t, "tu-aceptacion", piConTope, piNada), piCard(t, "hecho", piConTope, piNada),
			piCerrado("writer", "claude", ok), piWhyHecho},
		{"6: el writer no gano su columna", wr, wr, piCerrado("writer", "claude", ok), piWhyNoAvanzo("Writer")},
		{"6: el reviewer abrio un hallazgo high", rex, rha, piCerrado("reviewer", "gemini", ok), piWhyNoAvanzo("Review")},
		{"6: la tarjeta volvio atras", wr, tw, piCerrado("writer", "claude", ok), piWhyNoAvanzo("Test-writer")},
		{"7: corregir hallazgos no es de la cinta", wr, rha, piCerrado("writer", "claude", ok), piWhyFueraDeTabla("Review")},
		{"7: Review sin accion principal", wr, sinAccion, piCerrado("writer", "claude", ok), piWhyFueraDeTabla("Review")},
		{"8: el arquitecto entrego un spec que no pasa lint", bl, piCard(t, "arquitecto", piConTope, piNada),
			piCerrado("arquitecto", "claude", ok), piWhyReintento("arquitecto")},
		{"9: presupuesto agotado", tw, agotado, piCerrado("test-writer", "claude", ok), acAgotado("12", "10")},
		{"9: sin espacio de trabajo propio", twArbol, wrArbol, piCerrado("test-writer", "claude", ok), acSinEspacio},
		{"10: sin presupuesto", piCard(t, "test-writer", piSinTope, piNada), piCard(t, "writer", piSinTope, piNada),
			piCerrado("test-writer", "claude", ok), piWhySinPresupuesto},
		{"11: solo codex (sin tope) puede revisar", wr, sinGemini, piCerrado("writer", "claude", ok), piWhySinProveedor},
	}
	for _, cs := range casos {
		piDetiene(t, "CA-379", cs.nombre, Pilot(cs.before, cs.after, cs.closed), cs.why)
	}

	// el caso 9 es el why de la accion, sea cual sea
	if a := acAccion(t, "CA-379", agotado, ActPedirWriter); a.Enabled || a.Why != acAgotado("12", "10") {
		t.Fatalf("fixture: pedir-writer deshabilitada por el presupuesto agotado: %v %q", a.Enabled, a.Why)
	}

	// 2 con el sobre todavia en curso: su status, o 'sin cerrar' (el
	// contrato no distingue un sobre sin status de uno en curso)
	d := Pilot(tw, wr, piCerrado("test-writer", "claude", envelope.StatusRunning))
	if d.Launch || (d.Why != piWhyNoEntregable("test-writer", envelope.StatusRunning) && d.Why != piWhyNoEntregable("test-writer", "sin cerrar")) {
		t.Fatalf("CA-379: un sobre en curso no cerro entregable: %+v", d)
	}
}

// CA-379: lanza en los encadenamientos de la tabla, con el provider de
// closed si es ok y tiene tope, o la primera opcion ok con tope en el orden
// del registro; con el budget_usd y el pedido que propone la accion. En la
// review nunca elige al writer.
func TestCA379_LanzaLosEncadenamientosDeLaTabla(t *testing.T) {
	ok := envelope.StatusDeliverable
	tw := piCard(t, "test-writer", piConTope, piNada)
	wr := piCard(t, "writer", piConTope, piNada)
	rex := piCard(t, "review-exigida", piConTope, piNada)
	bl := piCard(t, "backlog", piConTope, piNada)

	// un test-writer entregable cuya tarjeta pasa a Writer lanza al writer,
	// con el provider del test-writer
	d := Pilot(tw, wr, piCerrado("test-writer", "claude", ok))
	piLanza(t, "CA-379", "test-writer -> writer", d, wr, ActPedirWriter, "writer", "claude", 9)
	if d.Pedido != acPedWriter {
		t.Fatalf("CA-379: el pedido del writer es el que propone la accion %q, fue %q", acPedWriter, d.Pedido)
	}

	// un writer entregable cuya tarjeta pasa a Review con la review exigida
	// lanza al reviewer, con lo que queda dividido 4 y nunca con el writer
	d = Pilot(wr, rex, piCerrado("writer", "claude", ok))
	piLanza(t, "CA-379", "writer -> reviewer", d, rex, ActPedirReviewer, "reviewer", "gemini", 9.0/ReviewLenses)
	if d.Provider == rex.Providers.Writer || d.Provider == "claude" {
		t.Fatalf("CA-379: en la review nunca elige al writer (%s): %+v", rex.Providers.Writer, d)
	}

	// un arquitecto entregable cuya tarjeta salta a Test-writer (su spec ya
	// tenia aprobacion vigente) lanza al test-writer; codex no tiene tope, asi
	// que va el primero ok con tope: claude
	d = Pilot(bl, tw, piCerrado("arquitecto", "codex", ok))
	piLanza(t, "CA-379", "arquitecto -> test-writer", d, tw, ActPedirTestWriter, "test-writer", "claude", 9)
	if d.Pedido != acPedTW {
		t.Fatalf("CA-379: el pedido del test-writer es el que propone la accion %q, fue %q", acPedTW, d.Pedido)
	}

	// el provider de closed gana aunque no sea el primero del registro
	d = Pilot(tw, wr, piCerrado("test-writer", "gemini", ok))
	piLanza(t, "CA-379", "test-writer de gemini -> writer", d, wr, ActPedirWriter, "writer", "gemini", 9)

	// el provider de closed no es ok (quedan 0.3 USD y claude pide 0.5): va el
	// primero ok con tope
	justoEv := piEv("writer", piSinTope, piNada)
	justoEv.Item.PresupuestoUSD = bdF(1)
	justoEv.Runs = []RunState{acGasto("run-writer", "writer", "claude", bdF(0.7), time.Minute)}
	justo := Derive(justoEv)
	a := acAccion(t, "CA-379", justo, ActPedirWriter)
	if !a.Enabled || len(a.Providers) == 0 || a.Providers[0].Name != "claude" || a.Providers[0].OK {
		t.Fatalf("fixture: pedir-writer habilitada con claude no ok por el minimo: %+v", a)
	}
	d = Pilot(tw, justo, piCerrado("test-writer", "claude", ok))
	piLanza(t, "CA-379", "claude sin minimo -> gemini", d, justo, ActPedirWriter, "writer", "gemini", 0.3)

	// el caso 1 mira after: una tarjeta que activo auto durante el trabajo sigue
	d = Pilot(piSinAuto(tw), wr, piCerrado("test-writer", "claude", ok))
	piLanza(t, "CA-379", "auto activado durante el trabajo", d, wr, ActPedirWriter, "writer", "claude", 9)
}

// piWhyConocidos son los Why posibles del contrato.
var piWhyConocidos = []*regexp.Regexp{
	regexp.MustCompile(`^el piloto automatico no esta activado en la tarjeta$`),
	regexp.MustCompile(`^el trabajo del [a-z-]+ no cerro entregable \([^)]+\): el piloto se detiene ante un rojo$`),
	regexp.MustCompile(`^la tarjeta esta en rojo: .+$`),
	regexp.MustCompile(`^la tarjeta tiene otro trabajo en curso o interrumpido$`),
	regexp.MustCompile(`^la tarjeta llego a (Tu aprobacion|Tu aceptacion): le toca a una persona$`),
	regexp.MustCompile(`^la tarjeta llego a Hecho: no queda trabajo$`),
	regexp.MustCompile(`^la tarjeta no avanzo \(sigue en [^)]+\): el piloto no reintenta$`),
	regexp.MustCompile(`^lo que sigue en .+ no esta en la tabla del piloto: le toca a una persona$`),
	regexp.MustCompile(`^seria volver a pedirle al [a-z-]+: el piloto no reintenta$`),
	regexp.MustCompile(`^la tarjeta no tiene presupuesto: el piloto automatico solo corre con un tope$`),
	regexp.MustCompile(`^ningun proveedor con tope de presupuesto puede tomar este trabajo$`),
}

// piOraculo es la tabla del contrato leida al pie de la letra: el primer caso
// que se cumple (con los Why aceptables) o el lanzamiento con su provider.
// La accion principal es la primera de la tarjeta.
func piOraculo(before, after Card, closed envelope.Record) (bool, []string, string) {
	if after.Item.Auto != item.AutoHastaHumano {
		return false, []string{piWhyNoActivado}, ""
	}
	if closed.Status != envelope.StatusDeliverable {
		switch closed.Status {
		case "":
			return false, []string{piWhyNoEntregable(closed.Role, "sin cerrar")}, ""
		case envelope.StatusRunning:
			return false, []string{piWhyNoEntregable(closed.Role, closed.Status), piWhyNoEntregable(closed.Role, "sin cerrar")}, ""
		}
		return false, []string{piWhyNoEntregable(closed.Role, closed.Status)}, ""
	}
	if after.Red != nil {
		return false, []string{piWhyRojo(after.Red.Plain)}, ""
	}
	if after.Running != nil || after.Interrupted != nil {
		return false, []string{piWhyOtroTrabajo}, ""
	}
	ai, bi := piIndice(after.Column), piIndice(before.Column)
	nombre := Columns[ai].Name
	if after.Column == ColHecho {
		return false, []string{piWhyHecho}, ""
	}
	if Columns[ai].Human {
		return false, []string{piWhyPersona(nombre)}, ""
	}
	if ai <= bi {
		return false, []string{piWhyNoAvanzo(nombre)}, ""
	}
	tabla, enTabla := piTabla[after.Column]
	if !enTabla || len(after.Actions) == 0 || after.Actions[0].ID != tabla {
		return false, []string{piWhyFueraDeTabla(nombre)}, ""
	}
	a := after.Actions[0]
	if a.Role == closed.Role {
		return false, []string{piWhyReintento(a.Role)}, ""
	}
	if !a.Enabled {
		return false, []string{a.Why}, ""
	}
	if after.Item.PresupuestoUSD == nil {
		return false, []string{piWhySinPresupuesto}, ""
	}
	prov := ""
	for _, o := range a.Providers {
		if o.OK && o.Budget && o.Name == closed.Provider {
			prov = o.Name
		}
	}
	for _, o := range a.Providers {
		if prov == "" && o.OK && o.Budget {
			prov = o.Name
		}
	}
	if prov == "" {
		return false, []string{piWhySinProveedor}, ""
	}
	return true, nil, prov
}

// piRolesDe son los roles que el Studio puede haber lanzado desde una
// columna: los de sus acciones de rol (en Tu aceptacion y Hecho ninguno
// lanza; se prueban todos).
func piRolesDe(col string) []string {
	switch col {
	case ColBacklog, ColArquitecto, ColTuAprobacion:
		return []string{"arquitecto"}
	case ColTestWriter:
		return []string{"test-writer"}
	case ColWriter:
		return []string{"writer"}
	case ColReview:
		return []string{"reviewer", "writer"}
	}
	return []string{"arquitecto", "test-writer", "writer", "reviewer"}
}

var (
	piEstados   = []string{envelope.StatusDeliverable, envelope.StatusNotDeliverable, envelope.StatusNoDelivery, envelope.StatusRunning}
	piRoles     = []string{"arquitecto", "test-writer", "writer", "reviewer"}
	piCerradoEn = []string{"claude", "codex", "gemini"}
)

// CA-380: el piloto es una cinta transportadora, no un orquestador. Sobre
// todas las combinaciones de fixtures (las ocho columnas de before y de
// after, los cuatro estados de closed, con y sin rojo, running o
// interrupted, con y sin presupuesto), Pilot decide lo que dice la tabla, y
// cada vez que lanza: la accion es la de PilotTable para la columna de after,
// closed cerro entregable, after no esta en rojo, la columna avanzo, no es
// humana ni Hecho y el rol no es el de closed. Nunca pide al arquitecto.
func TestCA380_UnaCintaTransportadoraNoUnOrquestador(t *testing.T) {
	befores := piTarjetas(t)
	afters := append([]piTarjeta{}, befores...)
	for _, b := range befores {
		afters = append(afters, piTarjeta{b.nombre + " sin auto", piSinAuto(b.c)})
	}
	lanzados := 0
	for _, b := range befores {
		realista := map[string]bool{}
		for _, r := range piRolesDe(b.c.Column) {
			realista[r] = true
		}
		for _, a := range afters {
			for _, st := range piEstados {
				for _, rol := range piRoles {
					for _, p := range piCerradoEn {
						closed := piCerrado(rol, p, st)
						d := Pilot(b.c, a.c, closed)
						launch, whys, prov := piOraculo(b.c, a.c, closed)
						if d.Launch != launch {
							t.Fatalf("CA-380: [before %s, after %s, closed %s de %s (%s)] launch=%v, la tabla dice %v %q: %+v",
								b.nombre, a.nombre, rol, p, st, d.Launch, launch, whys, d)
						}
						if !d.Launch {
							conocido := false
							for _, w := range whys {
								conocido = conocido || d.Why == w
							}
							if !conocido {
								t.Fatalf("CA-380: [before %s, after %s, closed %s de %s (%s)] se detiene en el primer caso que se cumple: %q, fue %q",
									b.nombre, a.nombre, rol, p, st, whys, d.Why)
							}
							continue
						}
						lanzados++
						nombre := fmt.Sprintf("before %s, after %s, closed %s de %s (%s)", b.nombre, a.nombre, rol, p, st)
						// las invariantes de la cinta, una por una
						accion, enTabla := PilotTable[a.c.Column]
						switch {
						case d.Why != "":
							t.Fatalf("CA-380: [%s] cuando lanza, why es \"\": %q", nombre, d.Why)
						case !enTabla || d.Action != accion || d.Action != piTabla[a.c.Column]:
							t.Fatalf("CA-380: [%s] la accion es la de la tabla para %s, fue %q", nombre, a.c.Column, d.Action)
						case closed.Status != envelope.StatusDeliverable:
							t.Fatalf("CA-380: [%s] nunca relanza tras un rojo", nombre)
						case a.c.Red != nil || a.c.Running != nil || a.c.Interrupted != nil:
							t.Fatalf("CA-380: [%s] after sin rojo ni otro trabajo", nombre)
						case piIndice(a.c.Column) <= piIndice(b.c.Column):
							t.Fatalf("CA-380: [%s] la columna tiene que avanzar", nombre)
						case a.c.WaitingHuman || a.c.Column == ColHecho:
							t.Fatalf("CA-380: [%s] nunca lanza en una columna humana ni en Hecho", nombre)
						case d.Role == closed.Role:
							t.Fatalf("CA-380: [%s] nunca reintenta: el rol no es el de closed", nombre)
						case realista[rol] && d.Action == ActPedirArquitecto:
							t.Fatalf("CA-380: [%s] la cinta nunca pide al arquitecto", nombre)
						case a.c.Item.PresupuestoUSD == nil:
							t.Fatalf("CA-380: [%s] nunca corre sin tope", nombre)
						case d.Provider != prov:
							t.Fatalf("CA-380: [%s] el provider es el de closed si es ok con tope, o el primero ok con tope: %q, fue %q", nombre, prov, d.Provider)
						}
						act, _ := a.c.Action(d.Action)
						if !act.Enabled || act.BudgetUSD == nil || !bdCasi(d.BudgetUSD, *act.BudgetUSD) || d.Pedido != act.Pedido || d.Role != act.Role {
							t.Fatalf("CA-380: [%s] lanza con el rol, el presupuesto y el pedido de la accion: %+v vs %+v", nombre, d, act)
						}
						if d.Action == ActPedirReviewer && (d.Provider == a.c.Providers.Writer || containsStr(a.c.Providers.Declared, d.Provider)) {
							t.Fatalf("CA-380: [%s] en la review nunca elige al writer: %s", nombre, d.Provider)
						}
					}
				}
			}
		}
	}
	if lanzados == 0 {
		t.Fatal("CA-380: en todas las combinaciones la cinta lanza alguna vez (si no, no es una cinta)")
	}
	// y todo Why posible es uno del contrato
	for _, b := range befores {
		if !strings.HasSuffix(b.nombre, "/"+piConTope+"/"+piNada) {
			continue
		}
		for _, a := range afters {
			d := Pilot(b.c, a.c, piCerrado("writer", "claude", envelope.StatusDeliverable))
			if d.Launch {
				continue
			}
			conocido := false
			for _, re := range piWhyConocidos {
				conocido = conocido || re.MatchString(d.Why)
			}
			if act, ok := a.c.Action(piTabla[a.c.Column]); ok && !act.Enabled && d.Why == act.Why {
				conocido = true
			}
			if !conocido {
				t.Fatalf("CA-380: [%s -> %s] %q no es un Why del contrato", b.nombre, a.nombre, d.Why)
			}
		}
	}
}

// CA-380: una cinta transportadora, no un orquestador: encadenada sobre
// cualquier secuencia de fixtures (cada trabajo que lanza cierra entregable y
// la tarjeta cae en cualquier otra), lanza como mucho tres veces seguidas
// (test-writer, writer y reviewer, en ese orden) y siempre termina.
func TestCA380_LaCintaTerminaEnTresLanzamientosComoMucho(t *testing.T) {
	cards := piTarjetas(t)
	type estado struct {
		i        int
		rol, prv string
	}
	memo := map[estado]int{}
	camino := map[estado][]string{}
	var largo func(e estado, prof int) (int, []string)
	largo = func(e estado, prof int) (int, []string) {
		if prof > 8 {
			t.Fatalf("CA-380: la cinta no termina: mas de 8 lanzamientos seguidos desde %s", cards[e.i].nombre)
		}
		if n, ok := memo[e]; ok {
			return n, camino[e]
		}
		best, bestCamino := 0, []string{}
		closed := piCerrado(e.rol, e.prv, envelope.StatusDeliverable)
		for j, a := range cards {
			d := Pilot(cards[e.i].c, a.c, closed)
			if !d.Launch {
				continue
			}
			n, sub := largo(estado{j, d.Role, d.Provider}, prof+1)
			if n+1 > best {
				best, bestCamino = n+1, append([]string{d.Role}, sub...)
			}
		}
		memo[e], camino[e] = best, bestCamino
		return best, bestCamino
	}
	max, maxCamino := 0, []string{}
	for i, b := range cards {
		for _, rol := range piRolesDe(b.c.Column) {
			for _, p := range piCerradoEn {
				n, c := largo(estado{i, rol, p}, 0)
				if n > 3 {
					t.Fatalf("CA-380: desde %s (%s de %s) la cinta lanza %d veces seguidas: %v", b.nombre, rol, p, n, c)
				}
				if n > max {
					max, maxCamino = n, c
				}
			}
		}
	}
	if max != 3 || !reflect.DeepEqual(maxCamino, []string{"test-writer", "writer", "reviewer"}) {
		t.Fatalf("CA-380: una persona que pide un trabajo encadena como mucho tres: test-writer, writer y reviewer; fue %d %v", max, maxCamino)
	}
}
