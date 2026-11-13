// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md, pieza
// 6 (CA-379, CA-380), RE-EXPRESADOS por .hoom/specs/autonomia-entre-firmas.md
// (CA-642, CA-660): boardcmd.Pilot(before, after, closed) desaparece y la
// decision es boardcmd.Piloto(Estado). La tabla pierde la columna Arquitecto,
// los once casos de hoy se reordenan en los catorce del contrato (los que
// dicen "como hoy" conservan su Why exacto) y la cota de la cinta deja de ser
// tres. Las tarjetas salen de Derive sobre Evidence literales (sin disco, sin
// reloj, sin git) y los sobres cerrados se arman a mano.
package boardcmd

import (
	"encoding/json"
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
	// CA-642: el caso 4, nuevo.
	piWhySinTarea = "el piloto corre en el espacio de trabajo de la tarjeta: abre la tarea con hoom task start " + bdSlug
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

// piTabla es la tabla del contrato, copiada aca a proposito. CA-379
// re-expresado (autonomia-entre-firmas): pierde la columna Arquitecto; desde
// sus columnas el piloto lanza solo test-writer, writer y reviewer.
var piTabla = map[string]string{
	ColTestWriter: ActPedirTestWriter,
	ColWriter:     ActPedirWriter,
	ColReview:     ActPedirReviewer,
}

// piColDelRol es la columna desde la que se lanza cada rol: lo que lleva
// `columna` en su sobre.
var piColDelRol = map[string]string{
	"arquitecto": ColArquitecto, "test-writer": ColTestWriter, "writer": ColWriter, "reviewer": ColReview,
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

// piSobre es un sobre cerrado de la tarjeta (Estado.Sobres), lanzado desde
// columna ("" = un sobre sin columna: los de antes del spec o un hoom agent
// directo). Un cierre que no es entregable se corta en run: no es un rojo
// esperado ni un reintento.
func piSobre(rol, provider, status, columna string) Sobre {
	rec := envelope.Record{ID: "20260922T170000_pi0001", RunID: "run-cerrado", Role: rol, Provider: provider,
		Task: bdSlug, Dir: bdWtDir, Spec: bdSpec, Stage: "ok", Step: 5, Steps: 5, Status: status,
		StartedAt: bdT0.Add(2 * time.Hour), UpdatedAt: bdT0.Add(3 * time.Hour), EndedAt: bdT0.Add(3 * time.Hour),
		Columna: columna}
	if status != envelope.StatusDeliverable {
		rec.Stage, rec.Step, rec.ExitCode = "run", 3, 1
	}
	return Sobre{Record: rec}
}

// piEstado es lo que el piloto ve de una tarjeta: sin politica (los dos
// contadores en 0 y sin ventana_max), con los providers de los fixtures.
func piEstado(c Card, sobres ...Sobre) Estado {
	return Estado{Tarjeta: c, Sobres: sobres, Proveedores: piProvs(), Hora: bdT0.Add(4 * time.Hour)}
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

func piContiene(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

// piDetiene exige un detenerse con su Why exacto y, si caso > 0, el numero
// del caso de la tabla que decidio.
func piDetiene(t *testing.T, ca, nombre string, d PilotDecision, why string, caso int) {
	t.Helper()
	if d.Launch || d.Paso != PasoDetenerse || d.Why != why || d.Regla != "" {
		t.Fatalf("%s: [%s] el piloto se detiene con\n  why %q\nfue launch=%v paso=%q why=%q regla=%q (%+v)",
			ca, nombre, why, d.Launch, d.Paso, d.Why, d.Regla, d)
	}
	if caso > 0 && d.Caso != caso {
		t.Fatalf("%s: [%s] decide el caso %d de la tabla, fue el %d (%q)", ca, nombre, caso, d.Caso, d.Why)
	}
}

// piLanza exige un lanzamiento con la regla cadena de la accion de la tabla
// con lo que propone la accion de la tarjeta.
func piLanza(t *testing.T, ca, nombre string, d PilotDecision, c Card, accion, rol, provider string, budget float64) {
	t.Helper()
	if !d.Launch || d.Paso != PasoLanzar || d.Why != "" || d.Regla != ReglaCadena || d.Caso != 0 {
		t.Fatalf("%s: [%s] el piloto lanza con la regla cadena (why \"\", caso 0), fue launch=%v paso=%q why=%q regla=%q caso=%d",
			ca, nombre, d.Launch, d.Paso, d.Why, d.Regla, d.Caso)
	}
	a, ok := c.Action(accion)
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
	if d.Intento != 0 || d.Ronda != 0 || d.Resume != "" || d.Hallazgos != nil || d.Bloqueantes != 0 || d.Evidencias != nil {
		t.Fatalf("%s: [%s] la cadena no lleva intento, ronda, resume, hallazgos, bloqueantes ni evidencias: %+v", ca, nombre, d)
	}
}

// CA-379 (re-expresado por CA-642): la tabla del piloto pierde la columna
// Arquitecto. Desde cualquier tarjeta y con cualquier ultimo sobre, la regla
// cadena lanza solo la accion de la tabla de su columna (test-writer, writer
// o reviewer); nunca pide al arquitecto. Las acciones del piloto son las
// cuatro del contrato, en orden, nunca van al JSON, y pedir-refutador nunca
// se le ofrece a una persona.
func TestCA379_LaTablaDelPilotoSinArquitecto(t *testing.T) {
	ultimos := []*Sobre{nil}
	for _, rol := range []string{"arquitecto", "test-writer", "writer", "reviewer"} {
		for _, col := range []string{"", ColBacklog, piColDelRol[rol]} {
			s := piSobre(rol, "claude", envelope.StatusDeliverable, col)
			ultimos = append(ultimos, &s)
		}
	}
	lanzados := 0
	for _, forma := range piFormas {
		c := piCard(t, forma, piConTope, piNada)
		var ids []string
		for _, a := range c.PilotActions {
			ids = append(ids, a.ID)
		}
		if want := []string{ActPedirTestWriter, ActPedirWriter, ActPedirReviewer, ActPedirRefutador}; !reflect.DeepEqual(ids, want) {
			t.Fatalf("CA-379: [%s] las acciones del piloto son %v, fueron %v", forma, want, ids)
		}
		for _, a := range c.Actions {
			if a.ID == ActPedirRefutador {
				t.Fatalf("CA-379: [%s] pedir-refutador nunca se le ofrece a una persona: %v", forma, acIDs(c))
			}
		}
		if _, ok := c.PilotAction(ActPedirRefutador); !ok {
			t.Fatalf("CA-379: [%s] PilotAction encuentra pedir-refutador", forma)
		}
		js, err := json.Marshal(c)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(js), ActPedirRefutador) {
			t.Fatalf("CA-379: [%s] las acciones del piloto no van al JSON de la tarjeta", forma)
		}
		for _, u := range ultimos {
			e := piEstado(c)
			nombre := forma + " sin sobres"
			if u != nil {
				e.Sobres = []Sobre{*u}
				nombre = fmt.Sprintf("%s tras %s (columna %q)", forma, u.Record.Role, u.Record.Columna)
			}
			d := Piloto(e)
			if d.Action == ActPedirArquitecto || d.Role == "arquitecto" {
				t.Fatalf("CA-379: [%s] el piloto nunca pide al arquitecto: %+v", nombre, d)
			}
			if !d.Launch {
				continue
			}
			lanzados++
			tabla, enTabla := piTabla[c.Column]
			if !enTabla || d.Action != tabla || d.Regla != ReglaCadena {
				t.Fatalf("CA-379: [%s] la cadena lanza la accion de la tabla de %s (%q), fue %q con regla %q", nombre, c.Column, tabla, d.Action, d.Regla)
			}
		}
	}
	if lanzados == 0 {
		t.Fatal("CA-379: la tabla lanza en alguna columna")
	}
	// desde Arquitecto la cinta ya no sigue: la accion principal no es de la tabla
	arq := piCard(t, "arquitecto", piConTope, piNada)
	piDetiene(t, "CA-379", "Arquitecto tras el arquitecto", Piloto(piEstado(arq,
		piSobre("arquitecto", "claude", envelope.StatusDeliverable, ColBacklog))), piWhyFueraDeTabla("Arquitecto"), 11)
}

// CA-379 (re-expresado por CA-642): Piloto se detiene en cada caso de la
// tabla nueva con el Why exacto de hoy (el del caso 4 es el del contrato), un
// fixture por caso: el primero que se cumple. El orden cambia: un trabajo en
// curso y una columna humana ganan ahora a un cierre no-entregable, y una
// tarjeta sin espacio de trabajo propio se detiene en el caso 4.
func TestCA379_SeDetieneEnCadaCaso(t *testing.T) {
	ok := envelope.StatusDeliverable
	no := envelope.StatusNotDeliverable
	tw := piCard(t, "test-writer", piConTope, piNada)
	wr := piCard(t, "writer", piConTope, piNada)
	rex := piCard(t, "review-exigida", piConTope, piNada)
	rha := piCard(t, "review-hallazgos", piConTope, piNada)
	tuAc := piCard(t, "tu-aceptacion", piConTope, piNada)

	// 9: la tarjeta en rojo por un veredicto rojo posterior
	rojoEv := piEv("writer", piConTope, piNada)
	rojoEv.Verdict = bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10,
		[]verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
	rojo := Derive(rojoEv)
	if rojo.Column != ColWriter || rojo.Red == nil || rojo.Red.Plain == "" {
		t.Fatalf("fixture: writer en rojo por veredicto: %s %+v", rojo.Column, rojo.Red)
	}
	// 11: Review sin accion principal (solo falta guardar el espacio de trabajo)
	sinAccion := Derive(tbListo(piEv("tu-aceptacion", piConTope, piNada), taskcmd.ReadySinGuardar, tbMsgSinGuardar))
	if sinAccion.Column != ColReview || (len(sinAccion.Actions) > 0 && sinAccion.Actions[0].ID == ActPedirReviewer) {
		t.Fatalf("fixture: Review sin accion de rol: %s %v", sinAccion.Column, acIDs(sinAccion))
	}
	// 11: presupuesto agotado
	agotado := piCard(t, "writer", piAgotado, piNada)
	if a := acAccion(t, "CA-379", agotado, ActPedirWriter); a.Enabled || a.Why != acAgotado("12", "10") {
		t.Fatalf("fixture: pedir-writer deshabilitada por el presupuesto agotado: %v %q", a.Enabled, a.Why)
	}
	// 4: sin espacio de trabajo propio
	wrArbol := Derive(bdArbol(piEv("writer", piConTope, piNada)))
	if wrArbol.Column == ColTuAprobacion || wrArbol.Column == ColTuAceptacion || wrArbol.Column == ColHecho {
		t.Fatalf("fixture: la tarjeta sin espacio propio no cae en una columna humana: %s", wrArbol.Column)
	}
	hechoArbol := Derive(bdArbol(piEv("hecho", piConTope, piNada)))
	if hechoArbol.Column != ColHecho {
		t.Fatalf("fixture: hecho sin espacio propio cae en Hecho: %s", hechoArbol.Column)
	}
	// 13: sin gemini, el unico otro provider para revisar es codex (sin tope)
	sinGeminiEv := piEv("review-exigida", piConTope, piNada)
	sinGeminiEv.Providers = acProvs()
	sinGemini := Derive(sinGeminiEv)
	if a := acAccion(t, "CA-379", sinGemini, ActPedirReviewer); !a.Enabled {
		t.Fatalf("fixture: pedir-reviewer habilitada con codex: %q", a.Why)
	}

	casos := []struct {
		nombre string
		c      Card
		sobres []Sobre
		why    string
		caso   int
	}{
		{"1: la tarjeta no tiene auto", piSinAuto(wr), []Sobre{piSobre("test-writer", "claude", ok, ColTestWriter)}, piWhyNoActivado, 1},
		{"1: la persona apago auto mientras corria el writer", piSinAuto(rex), []Sobre{piSobre("writer", "claude", ok, ColWriter)}, piWhyNoActivado, 1},
		{"1: auto apagado le gana a un cierre no-entregable", piSinAuto(wr), []Sobre{piSobre("test-writer", "claude", no, ColTestWriter)}, piWhyNoActivado, 1},
		{"1: auto apagado le gana a un trabajo en curso", piSinAuto(piCard(t, "writer", piConTope, piEnCurso)), nil, piWhyNoActivado, 1},
		{"1: sin auto y sin sobres", piSinAuto(tw), nil, piWhyNoActivado, 1},
		{"2: otro trabajo en curso", piCard(t, "writer", piConTope, piEnCurso), []Sobre{piSobre("test-writer", "claude", ok, ColTestWriter)}, piWhyOtroTrabajo, 2},
		{"2: un trabajo interrumpido", piCard(t, "writer", piConTope, piInterrumpido), []Sobre{piSobre("test-writer", "claude", ok, ColTestWriter)}, piWhyOtroTrabajo, 2},
		{"2: le gana a un cierre no-entregable", piCard(t, "writer", piConTope, piEnCurso), []Sobre{piSobre("test-writer", "claude", no, ColTestWriter)}, piWhyOtroTrabajo, 2},
		{"2: le gana a la columna humana", piCard(t, "tu-aceptacion", piConTope, piEnCurso), nil, piWhyOtroTrabajo, 2},
		{"3: Tu aprobacion", piCard(t, "tu-aprobacion", piConTope, piNada), []Sobre{piSobre("arquitecto", "claude", ok, ColBacklog)},
			piWhyPersona("Tu aprobacion"), 3},
		{"3: Tu aceptacion", tuAc, []Sobre{piSobre("reviewer", "gemini", ok, ColReview)}, piWhyPersona("Tu aceptacion"), 3},
		{"3: Hecho", piCard(t, "hecho", piConTope, piNada), []Sobre{piSobre("writer", "claude", ok, ColWriter)}, piWhyHecho, 3},
		{"3: le gana a un cierre no-entregable", tuAc, []Sobre{piSobre("reviewer", "gemini", no, ColReview)}, piWhyPersona("Tu aceptacion"), 3},
		{"3: le gana a la tarea cerrada", hechoArbol, nil, piWhyHecho, 3},
		{"4: sin espacio de trabajo propio", wrArbol, []Sobre{piSobre("test-writer", "claude", ok, ColTestWriter)}, piWhySinTarea, 4},
		{"4: le gana a un cierre no-entregable", wrArbol, []Sobre{piSobre("test-writer", "claude", no, ColTestWriter)}, piWhySinTarea, 4},
		{"4: sin sobres", wrArbol, nil, piWhySinTarea, 4},
		{"8: no-entregable", wr, []Sobre{piSobre("test-writer", "claude", no, ColTestWriter)},
			piWhyNoEntregable("test-writer", "no-entregable"), 8},
		{"8: sin-entrega", wr, []Sobre{piSobre("test-writer", "claude", envelope.StatusNoDelivery, ColTestWriter)},
			piWhyNoEntregable("test-writer", "sin-entrega"), 8},
		{"8: le gana a la tarjeta en rojo", rojo, []Sobre{piSobre("test-writer", "claude", no, ColTestWriter)},
			piWhyNoEntregable("test-writer", "no-entregable"), 8},
		{"8: mira el ultimo sobre, no el primero", wr, []Sobre{piSobre("test-writer", "claude", ok, ColTestWriter),
			piSobre("writer", "claude", no, ColWriter)}, piWhyNoEntregable("writer", "no-entregable"), 8},
		{"9: la tarjeta quedo en rojo", rojo, []Sobre{piSobre("test-writer", "claude", ok, ColTestWriter)}, piWhyRojo(rojo.Red.Plain), 9},
		{"9: en rojo y sin sobres", rojo, nil, piWhyRojo(rojo.Red.Plain), 9},
		{"10: el writer no gano su columna", wr, []Sobre{piSobre("writer", "claude", ok, ColWriter)}, piWhyNoAvanzo("Writer"), 10},
		{"10: el reviewer abrio un hallazgo high", rha, []Sobre{piSobre("reviewer", "gemini", ok, ColReview)}, piWhyNoAvanzo("Review"), 10},
		{"10: la tarjeta volvio atras", tw, []Sobre{piSobre("writer", "claude", ok, ColWriter)}, piWhyNoAvanzo("Test-writer"), 10},
		{"11: corregir hallazgos no es de la cinta", rha, []Sobre{piSobre("writer", "claude", ok, ColWriter)}, piWhyFueraDeTabla("Review"), 11},
		{"11: Review sin accion principal", sinAccion, []Sobre{piSobre("writer", "claude", ok, ColWriter)}, piWhyFueraDeTabla("Review"), 11},
		{"11: Arquitecto ya no esta en la tabla", piCard(t, "arquitecto", piConTope, piNada),
			[]Sobre{piSobre("arquitecto", "claude", ok, ColBacklog)}, piWhyFueraDeTabla("Arquitecto"), 11},
		{"11: sin columna en el sobre no aplica el 10: seria volver a pedirle al writer", wr,
			[]Sobre{piSobre("writer", "claude", ok, "")}, piWhyReintento("writer"), 11},
		{"11: presupuesto agotado", agotado, []Sobre{piSobre("test-writer", "claude", ok, ColTestWriter)}, acAgotado("12", "10"), 11},
		{"12: sin presupuesto", piCard(t, "writer", piSinTope, piNada), []Sobre{piSobre("test-writer", "claude", ok, ColTestWriter)},
			piWhySinPresupuesto, 12},
		{"13: solo codex (sin tope) puede revisar", sinGemini, []Sobre{piSobre("writer", "claude", ok, ColWriter)}, piWhySinProveedor, 13},
	}
	for _, cs := range casos {
		e := piEstado(cs.c, cs.sobres...)
		if cs.nombre[:3] == "13:" {
			e.Proveedores = acProvs()
		}
		piDetiene(t, "CA-379", cs.nombre, Piloto(e), cs.why, cs.caso)
	}
}

// CA-379 (re-expresado por CA-642): con la regla cadena lanza la accion de
// la tabla con el provider del ultimo sobre si es ok y tiene tope, o la
// primera opcion ok con tope en el orden del registro; con el budget_usd y el
// pedido que propone la accion. En la review nunca elige al writer. Sin
// sobres (una spec escrita a mano y recien firmada) lanza al test-writer.
func TestCA379_LanzaLosEncadenamientosDeLaTabla(t *testing.T) {
	ok := envelope.StatusDeliverable
	tw := piCard(t, "test-writer", piConTope, piNada)
	wr := piCard(t, "writer", piConTope, piNada)
	rex := piCard(t, "review-exigida", piConTope, piNada)

	// un test-writer entregable cuya tarjeta pasa a Writer lanza al writer,
	// con el provider del test-writer
	d := Piloto(piEstado(wr, piSobre("test-writer", "claude", ok, ColTestWriter)))
	piLanza(t, "CA-379", "test-writer -> writer", d, wr, ActPedirWriter, "writer", "claude", 9)
	if d.Pedido != acPedWriter {
		t.Fatalf("CA-379: el pedido del writer es el que propone la accion %q, fue %q", acPedWriter, d.Pedido)
	}

	// un writer entregable cuya tarjeta pasa a Review con la review exigida
	// lanza al reviewer, con lo que queda dividido 4 y nunca con el writer
	d = Piloto(piEstado(rex, piSobre("writer", "claude", ok, ColWriter)))
	piLanza(t, "CA-379", "writer -> reviewer", d, rex, ActPedirReviewer, "reviewer", "gemini", 9.0/ReviewLenses)
	if d.Provider == rex.Providers.Writer || d.Provider == "claude" || d.Delta {
		t.Fatalf("CA-379: en la review nunca elige al writer (%s), y sin review encadenable no hay Delta: %+v", rex.Providers.Writer, d)
	}

	// sin sobres: la spec escrita a mano y recien firmada; va el primero ok
	// con tope (claude)
	d = Piloto(piEstado(tw))
	piLanza(t, "CA-379", "firma -> test-writer", d, tw, ActPedirTestWriter, "test-writer", "claude", 9)
	if d.Pedido != acPedTW {
		t.Fatalf("CA-379: el pedido del test-writer es el que propone la accion %q, fue %q", acPedTW, d.Pedido)
	}

	// un arquitecto entregable de codex (sin tope) cuya tarjeta salta a
	// Test-writer: va el primero ok con tope
	d = Piloto(piEstado(tw, piSobre("arquitecto", "codex", ok, ColBacklog)))
	piLanza(t, "CA-379", "arquitecto de codex -> test-writer", d, tw, ActPedirTestWriter, "test-writer", "claude", 9)

	// el provider del ultimo sobre gana aunque no sea el primero del registro
	d = Piloto(piEstado(wr, piSobre("test-writer", "gemini", ok, ColTestWriter)))
	piLanza(t, "CA-379", "test-writer de gemini -> writer", d, wr, ActPedirWriter, "writer", "gemini", 9)

	// sin columna en el sobre (un hoom agent directo) no aplica el caso 10
	d = Piloto(piEstado(wr, piSobre("test-writer", "claude", ok, "")))
	piLanza(t, "CA-379", "test-writer sin columna -> writer", d, wr, ActPedirWriter, "writer", "claude", 9)

	// el provider del ultimo sobre no es ok (quedan 0.3 USD y claude pide
	// 0.5): va el primero ok con tope
	justoEv := piEv("writer", piSinTope, piNada)
	justoEv.Item.PresupuestoUSD = bdF(1)
	justoEv.Runs = []RunState{acGasto("run-writer", "writer", "claude", bdF(0.7), time.Minute)}
	justo := Derive(justoEv)
	a := acAccion(t, "CA-379", justo, ActPedirWriter)
	if !a.Enabled || len(a.Providers) == 0 || a.Providers[0].Name != "claude" || a.Providers[0].OK {
		t.Fatalf("fixture: pedir-writer habilitada con claude no ok por el minimo: %+v", a)
	}
	d = Piloto(piEstado(justo, piSobre("test-writer", "claude", ok, ColTestWriter)))
	piLanza(t, "CA-379", "claude sin minimo -> gemini", d, justo, ActPedirWriter, "writer", "gemini", 0.3)
}

// piWhyConocidos son los Why posibles del contrato sin politica.
var piWhyConocidos = []*regexp.Regexp{
	regexp.MustCompile(`^el piloto automatico no esta activado en la tarjeta$`),
	regexp.MustCompile(`^la tarjeta tiene otro trabajo en curso o interrumpido$`),
	regexp.MustCompile(`^la tarjeta llego a (Tu aprobacion|Tu aceptacion): le toca a una persona$`),
	regexp.MustCompile(`^la tarjeta llego a Hecho: no queda trabajo$`),
	regexp.MustCompile(`^el piloto corre en el espacio de trabajo de la tarjeta: abre la tarea con hoom task start [a-z0-9-]+$`),
	regexp.MustCompile(`^el trabajo del [a-z-]+ no cerro entregable \([^)]+\): el piloto se detiene ante un rojo$`),
	regexp.MustCompile(`^la tarjeta esta en rojo: .+$`),
	regexp.MustCompile(`^la tarjeta no avanzo \(sigue en [^)]+\): el piloto no reintenta$`),
	regexp.MustCompile(`^lo que sigue en .+ no esta en la tabla del piloto: le toca a una persona$`),
	regexp.MustCompile(`^seria volver a pedirle al [a-z-]+: el piloto no reintenta$`),
	regexp.MustCompile(`^la tarjeta no tiene presupuesto: el piloto automatico solo corre con un tope$`),
	regexp.MustCompile(`^ningun proveedor con tope de presupuesto puede tomar este trabajo$`),
}

// piOraculo es la tabla del contrato leida al pie de la letra para un Estado
// sin politica (sin reintentos, sin rondas, sin ventana_max) y sin cortes
// por tiempo: el primer caso que se cumple con su Why, o el lanzamiento con
// su provider. La accion principal es la primera de la tarjeta.
func piOraculo(e Estado) (bool, string, string, int) {
	c := e.Tarjeta
	if c.Item.Auto != item.AutoHastaHumano {
		return false, piWhyNoActivado, "", 1
	}
	if c.Running != nil || c.Interrupted != nil {
		return false, piWhyOtroTrabajo, "", 2
	}
	ai := piIndice(c.Column)
	nombre := Columns[ai].Name
	if c.Column == ColHecho {
		return false, piWhyHecho, "", 3
	}
	if Columns[ai].Human {
		return false, piWhyPersona(nombre), "", 3
	}
	var ultimo *envelope.Record
	if len(e.Sobres) > 0 {
		ultimo = &e.Sobres[len(e.Sobres)-1].Record
	}
	if ultimo != nil && ultimo.Status != envelope.StatusDeliverable {
		return false, piWhyNoEntregable(ultimo.Role, ultimo.Status), "", 8
	}
	if c.Red != nil {
		return false, piWhyRojo(c.Red.Plain), "", 9
	}
	if ultimo != nil && ultimo.Columna != "" && ai <= piIndice(ultimo.Columna) {
		return false, piWhyNoAvanzo(nombre), "", 10
	}
	tabla, enTabla := piTabla[c.Column]
	if !enTabla || len(c.Actions) == 0 || c.Actions[0].ID != tabla {
		return false, piWhyFueraDeTabla(nombre), "", 11
	}
	a := c.Actions[0]
	if ultimo != nil && a.Role == ultimo.Role {
		return false, piWhyReintento(a.Role), "", 11
	}
	if !a.Enabled {
		return false, a.Why, "", 11
	}
	if c.Item.PresupuestoUSD == nil {
		return false, piWhySinPresupuesto, "", 12
	}
	prov := ""
	for _, o := range a.Providers {
		if ultimo != nil && o.OK && o.Budget && o.Name == ultimo.Provider {
			prov = o.Name
		}
	}
	for _, o := range a.Providers {
		if prov == "" && o.OK && o.Budget {
			prov = o.Name
		}
	}
	if prov == "" {
		return false, piWhySinProveedor, "", 13
	}
	return true, "", prov, 0
}

var (
	piEstados   = []string{envelope.StatusDeliverable, envelope.StatusNotDeliverable, envelope.StatusNoDelivery}
	piRoles     = []string{"arquitecto", "test-writer", "writer", "reviewer"}
	piCerradoEn = []string{"claude", "codex", "gemini"}
)

// CA-380 (re-expresado por CA-642 y CA-660): el piloto sigue siendo una
// cinta, no un orquestador. Sobre todas las combinaciones de fixtures (las
// ocho columnas, con y sin auto, rojo, running o interrupted, con y sin
// presupuesto) y todo ultimo sobre (ninguno, o de cada rol, cada cierre y
// cada provider, con y sin columna), sin politica Piloto decide lo que dice la
// tabla nueva, y cada vez que lanza: es la regla cadena, la accion de la
// tabla para la columna de la tarjeta, el ultimo sobre cerro entregable, la
// tarjeta no esta en rojo, avanzo, no es humana ni Hecho, el rol no es el
// del ultimo sobre, hay presupuesto y nunca pide al arquitecto.
func TestCA380_UnaCintaTransportadoraNoUnOrquestador(t *testing.T) {
	base := piTarjetas(t)
	tarjetas := append([]piTarjeta{}, base...)
	for _, b := range base {
		tarjetas = append(tarjetas, piTarjeta{b.nombre + " sin auto", piSinAuto(b.c)})
	}
	type ultimo struct {
		nombre string
		sobres []Sobre
	}
	ultimos := []ultimo{{"sin sobres", nil}}
	for _, rol := range piRoles {
		for _, st := range piEstados {
			for _, p := range piCerradoEn {
				for _, col := range []string{"", piColDelRol[rol]} {
					ultimos = append(ultimos, ultimo{fmt.Sprintf("%s de %s (%s, columna %q)", rol, p, st, col),
						[]Sobre{piSobre(rol, p, st, col)}})
				}
			}
		}
	}
	lanzados := 0
	for _, tj := range tarjetas {
		for _, u := range ultimos {
			e := piEstado(tj.c, u.sobres...)
			nombre := tj.nombre + " tras " + u.nombre
			d := Piloto(e)
			launch, why, prov, caso := piOraculo(e)
			if d.Launch != launch {
				t.Fatalf("CA-380: [%s] launch=%v, la tabla dice %v %q (caso %d): %+v", nombre, d.Launch, launch, why, caso, d)
			}
			if !d.Launch {
				if d.Why != why || d.Caso != caso || d.Paso != PasoDetenerse {
					t.Fatalf("CA-380: [%s] se detiene en el primer caso que se cumple: caso %d %q, fue caso %d %q (paso %q)",
						nombre, caso, why, d.Caso, d.Why, d.Paso)
				}
				continue
			}
			lanzados++
			c := tj.c
			accion, enTabla := piTabla[c.Column]
			switch {
			case d.Why != "" || d.Paso != PasoLanzar || d.Regla != ReglaCadena:
				t.Fatalf("CA-380: [%s] cuando lanza, why es \"\", el paso es lanzar y la regla cadena: %+v", nombre, d)
			case !enTabla || d.Action != accion:
				t.Fatalf("CA-380: [%s] la accion es la de la tabla para %s, fue %q", nombre, c.Column, d.Action)
			case len(u.sobres) > 0 && u.sobres[0].Record.Status != envelope.StatusDeliverable:
				t.Fatalf("CA-380: [%s] nunca relanza tras un rojo", nombre)
			case c.Red != nil || c.Running != nil || c.Interrupted != nil:
				t.Fatalf("CA-380: [%s] la tarjeta sin rojo ni otro trabajo", nombre)
			case len(u.sobres) > 0 && u.sobres[0].Record.Columna != "" && piIndice(c.Column) <= piIndice(u.sobres[0].Record.Columna):
				t.Fatalf("CA-380: [%s] la columna tiene que avanzar", nombre)
			case c.WaitingHuman || c.Column == ColHecho:
				t.Fatalf("CA-380: [%s] nunca lanza en una columna humana ni en Hecho", nombre)
			case len(u.sobres) > 0 && d.Role == u.sobres[0].Record.Role:
				t.Fatalf("CA-380: [%s] nunca reintenta: el rol no es el del ultimo sobre", nombre)
			case d.Action == ActPedirArquitecto || d.Role == "arquitecto":
				t.Fatalf("CA-380: [%s] la cinta nunca pide al arquitecto", nombre)
			case c.Item.PresupuestoUSD == nil:
				t.Fatalf("CA-380: [%s] nunca corre sin tope", nombre)
			case d.Provider != prov:
				t.Fatalf("CA-380: [%s] el provider es el del ultimo sobre si es ok con tope, o el primero ok con tope: %q, fue %q", nombre, prov, d.Provider)
			}
			act, _ := c.Action(d.Action)
			if !act.Enabled || act.BudgetUSD == nil || !bdCasi(d.BudgetUSD, *act.BudgetUSD) || d.Pedido != act.Pedido || d.Role != act.Role {
				t.Fatalf("CA-380: [%s] lanza con el rol, el presupuesto y el pedido de la accion: %+v vs %+v", nombre, d, act)
			}
			if d.Action == ActPedirReviewer && (d.Provider == c.Providers.Writer || piContiene(c.Providers.Declared, d.Provider)) {
				t.Fatalf("CA-380: [%s] en la review nunca elige al writer: %s", nombre, d.Provider)
			}
		}
	}
	if lanzados == 0 {
		t.Fatal("CA-380: en todas las combinaciones la cinta lanza alguna vez (si no, no es una cinta)")
	}
	// y todo Why posible es uno del contrato
	for _, tj := range tarjetas {
		d := Piloto(piEstado(tj.c, piSobre("test-writer", "claude", envelope.StatusDeliverable, ColTestWriter)))
		if d.Launch {
			continue
		}
		conocido := false
		for _, re := range piWhyConocidos {
			conocido = conocido || re.MatchString(d.Why)
		}
		if act, ok := tj.c.Action(piTabla[tj.c.Column]); ok && !act.Enabled && d.Why == act.Why {
			conocido = true
		}
		if !conocido {
			t.Fatalf("CA-380: [%s] %q no es un Why del contrato", tj.nombre, d.Why)
		}
	}
}

// CA-380 (re-expresado por CA-660): sin politica y sin cortes por tiempo,
// encadenada sobre cualquier secuencia de fixtures (cada trabajo que lanza
// cierra entregable y la tarjeta cae en cualquier otra), la cinta lanza como
// mucho tres veces seguidas (test-writer, writer y reviewer, en ese orden) y
// siempre termina: 3 + 0 + 4*0 + 4 es la cota con los relevos, y sin cortes
// no hay relevo.
func TestCA380_CA660_SinPoliticaLaCintaLanzaTresComoMucho(t *testing.T) {
	var cards []piTarjeta
	for _, tj := range piTarjetas(t) {
		if strings.HasSuffix(tj.nombre, "/"+piNada) || strings.HasSuffix(tj.nombre, "/"+piRojo) {
			cards = append(cards, tj)
		}
	}
	// lo que decide la vuelta siguiente es solo el ultimo sobre: rol,
	// provider y columna desde la que se lanzo ("" = sin sobres)
	type estado struct{ rol, prv, col string }
	memo := map[estado]int{}
	camino := map[estado][]string{}
	var largo func(e estado, prof int) (int, []string)
	largo = func(e estado, prof int) (int, []string) {
		if prof > 8 {
			t.Fatalf("CA-380: la cinta no termina: mas de 8 lanzamientos seguidos (ultimo %+v)", e)
		}
		if n, ok := memo[e]; ok {
			return n, camino[e]
		}
		best, bestCamino := 0, []string{}
		for _, a := range cards {
			est := piEstado(a.c)
			if e.rol != "" {
				est.Sobres = []Sobre{piSobre(e.rol, e.prv, envelope.StatusDeliverable, e.col)}
			}
			d := Piloto(est)
			if !d.Launch {
				continue
			}
			n, sub := largo(estado{d.Role, d.Provider, a.c.Column}, prof+1)
			if n+1 > best {
				best, bestCamino = n+1, append([]string{d.Role}, sub...)
			}
		}
		memo[e], camino[e] = best, bestCamino
		return best, bestCamino
	}
	max, maxCamino := 0, []string{}
	inicios := []estado{{}}
	for _, rol := range piRoles {
		for _, p := range piCerradoEn {
			for _, col := range []string{"", piColDelRol[rol]} {
				inicios = append(inicios, estado{rol, p, col})
			}
		}
	}
	for _, ini := range inicios {
		n, c := largo(ini, 0)
		if n > 3 {
			t.Fatalf("CA-380: desde %+v la cinta lanza %d veces seguidas: %v", ini, n, c)
		}
		if n > max {
			max, maxCamino = n, c
		}
	}
	if max != 3 || !reflect.DeepEqual(maxCamino, []string{"test-writer", "writer", "reviewer"}) {
		t.Fatalf("CA-380: sin politica la cinta encadena como mucho tres: test-writer, writer y reviewer; fue %d %v", max, maxCamino)
	}
}
