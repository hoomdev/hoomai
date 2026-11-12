// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md: la
// decision pura del piloto, boardcmd.Piloto(Estado). CA-642 (pureza, nunca
// firma ni integra, los Why de hoy y el del caso 4), CA-643 (rojo esperado
// del test-writer), CA-645 (Delta en la cadena), CA-649 y CA-650 (reintento
// ante un rojo), CA-652 (corte por tiempo), CA-653 a CA-655 y CA-657 (la
// ronda de hallazgos), CA-658 (los pedidos), CA-659 (la ventana de 5 h) y
// CA-660 (la cota de lanzamientos).
//
// Las tarjetas salen de Derive sobre Evidence literales (piEv, acEv, bdEv:
// sin disco, sin reloj, sin git); los sobres cerrados, los veredictos, los
// registros de review, los hallazgos y las ventanas se arman a mano. Toda
// narracion de un run (la nota del sobre, la salida y las notas de cada gate,
// las notas de la review) lleva aeVeneno: ningun pedido puede contenerlo.
package boardcmd

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// aeVeneno es texto de la narracion de un run: nunca llega a un pedido.
const aeVeneno = "VENENO-narracion-del-run"

const (
	aeOK = envelope.StatusDeliverable
	aeNo = envelope.StatusNotDeliverable
)

// aeHora es la hora a la que decide el piloto en los fixtures.
var aeHora = bdT0.Add(6 * time.Hour)

var aePol0 = manifest.AutonomiaPolicy{}

func aeEnt(n int) *int       { return &n }
func aeStr(s string) *string { return &s }

// aeGates son los gates de un veredicto completo de la tarjeta
// (bdGatesVerdes: spec_lint, spec_trace, spec_approved, findings_open, test),
// con los estados que cambian. La salida y las notas de cada gate son
// narracion.
func aeGates(cambios map[string]string) []verdict.GateResult {
	gs := bdGatesVerdes()
	for i := range gs {
		if st, ok := cambios[gs[i].Name]; ok {
			gs[i].Status = st
		}
		gs[i].OutputTail, gs[i].Notes = aeVeneno+": salida de "+gs[i].Name, aeVeneno
	}
	return gs
}

func aeVer(id string, gates []verdict.GateResult) *verdict.Verdict {
	return bdVeredicto(id, bdT0.Add(90*time.Minute), "huella-1", 50, 10, gates)
}

// Veredictos de los fixtures.
func aeVerde(id string) *verdict.Verdict { return aeVer(id, aeGates(nil)) }

// aeRojoTest: rojo solo por el gate test (un rojo esperado del test-writer).
func aeRojoTest(id string) *verdict.Verdict {
	return aeVer(id, aeGates(map[string]string{"test": verdict.StatusFail}))
}

// aeRojoHallazgos: rojo solo por findings_open.
func aeRojoHallazgos(id string) *verdict.Verdict {
	return aeVer(id, aeGates(map[string]string{"findings_open": verdict.StatusFail}))
}

// aeS describe un sobre cerrado de la tarjeta.
type aeS struct {
	rol, prov, col string
	stage, status  string // "" = ok / entregable
	v              *verdict.Verdict
	corte, sesion  string
	pedido         *string // nil = el pedido de siempre del rol
	regla          string
	intento, ronda int
	hallazgos      []string
	bloq           int
	trasp          []string
}

func (o aeS) sobre(n int) Sobre {
	ini := bdT0.Add(time.Duration(60+10*n) * time.Minute)
	id := fmt.Sprintf("20260922T%02d%02d00_ae%04d", 16+n/6, (n%6)*10, n)
	stage, status := o.stage, o.status
	if stage == "" {
		stage = "ok"
	}
	if status == "" {
		status = aeOK
	}
	pasos := map[string]int{"spec": 1, "aislar": 2, "run": 3, "scope": 4, "verify": 5, "check": 6, "ok": 6}
	pedido := map[string]string{"writer": acPedWriter, "test-writer": acPedTW}[o.rol]
	if o.pedido != nil {
		pedido = *o.pedido
	}
	rec := envelope.Record{ID: id, RunID: "run-" + id, Role: o.rol, Provider: o.prov, Task: bdSlug, Dir: bdWtDir,
		Spec: bdSpec, Stage: stage, Step: pasos[stage], Steps: 6, Status: status, StartedAt: ini,
		UpdatedAt: ini.Add(5 * time.Minute), EndedAt: ini.Add(5 * time.Minute), Columna: o.col, Pedido: pedido,
		Corte: o.corte, Regla: o.regla, Intento: o.intento, Ronda: o.ronda, Hallazgos: o.hallazgos,
		Bloqueantes: o.bloq, Trasplantados: o.trasp, Note: aeVeneno + ": la narracion del run"}
	if status != aeOK {
		rec.ExitCode = 1
	}
	if o.regla != "" {
		rec.Pilot = true
	}
	if o.v != nil {
		rec.VerdictID, rec.Verdict = o.v.ID, o.v.Verdict
	}
	return Sobre{Record: rec, Veredicto: o.v, SesionID: o.sesion}
}

// aeSobres arma los sobres, del mas viejo al mas nuevo.
func aeSobres(xs ...aeS) []Sobre {
	var out []Sobre
	for i, o := range xs {
		out = append(out, o.sobre(i+1))
	}
	return out
}

// aeEstado es lo que el piloto ve de la tarjeta de ev: su veredicto, sus
// reviews y sus hallazgos (todos bloquean: son high y el umbral es high).
func aeEstado(ev Evidence, pol manifest.AutonomiaPolicy, sobres []Sobre) Estado {
	return Estado{Tarjeta: Derive(ev), Sobres: sobres, Veredicto: ev.Verdict, Reviews: ev.Reviews,
		Bloqueantes: append([]finding.Item(nil), ev.Findings...), Politica: pol,
		Proveedores: ev.Providers, Hora: aeHora}
}

func aeCol(t *testing.T, ca string, e Estado, col string) {
	t.Helper()
	if e.Tarjeta.Column != col {
		t.Fatalf("%s: fixture: la tarjeta cae en %q, se esperaba %q (missing=%q)", ca, e.Tarjeta.Column, col, e.Tarjeta.Missing)
	}
}

// aeQuiero es un lanzamiento esperado. libre nombra los campos que el
// contrato no fija en ese caso.
type aeQuiero struct {
	accion, rol, prov, regla string
	caso, intento, ronda     int
	resume, pedido           string
	hallazgos                []string
	bloq                     int
	delta                    bool
	libre                    string // "pedido", "delta", "hallazgos", separados por coma
}

func aeLanza(t *testing.T, ca, nombre string, d PilotDecision, q aeQuiero) {
	t.Helper()
	if !d.Launch || d.Paso != PasoLanzar || d.Why != "" {
		t.Fatalf("%s: [%s] el piloto lanza (paso lanzar, why \"\"), fue launch=%v paso=%q why=%q (%+v)", ca, nombre, d.Launch, d.Paso, d.Why, d)
	}
	libre := func(campo string) bool { return strings.Contains(","+q.libre+",", ","+campo+",") }
	mal := d.Action != q.accion || d.Role != q.rol || d.Provider != q.prov || d.Regla != q.regla || d.Caso != q.caso ||
		d.Intento != q.intento || d.Ronda != q.ronda || d.Resume != q.resume || d.Bloqueantes != q.bloq ||
		(!libre("delta") && d.Delta != q.delta) ||
		(!libre("hallazgos") && !reflect.DeepEqual(d.Hallazgos, q.hallazgos)) ||
		(!libre("pedido") && d.Pedido != q.pedido) || d.Evidencias != nil
	if mal {
		t.Fatalf("%s: [%s] lanza\n quiero %+v\n fue    accion=%q rol=%q provider=%q regla=%q caso=%d intento=%d ronda=%d resume=%q bloqueantes=%d delta=%v hallazgos=%q evidencias=%v\n pedido=%q",
			ca, nombre, q, d.Action, d.Role, d.Provider, d.Regla, d.Caso, d.Intento, d.Ronda, d.Resume, d.Bloqueantes, d.Delta, d.Hallazgos, d.Evidencias, d.Pedido)
	}
}

func aeDetiene(t *testing.T, ca, nombre string, d PilotDecision, why string, caso int) {
	t.Helper()
	piDetiene(t, ca, nombre, d, why, caso)
}

// aeDetieneRe exige un detenerse cuyo Why calza con re.
func aeDetieneRe(t *testing.T, ca, nombre string, d PilotDecision, re *regexp.Regexp, caso int) {
	t.Helper()
	if d.Launch || d.Paso != PasoDetenerse || !re.MatchString(d.Why) || d.Regla != "" {
		t.Fatalf("%s: [%s] el piloto se detiene con un why que calza con %q, fue launch=%v paso=%q why=%q regla=%q",
			ca, nombre, re, d.Launch, d.Paso, d.Why, d.Regla)
	}
	if caso > 0 && d.Caso != caso {
		t.Fatalf("%s: [%s] decide el caso %d, fue el %d", ca, nombre, caso, d.Caso)
	}
}

// ===========================================================================
// CA-642: pura, nunca firma ni integra, los Why de hoy y el del caso 4
// ===========================================================================

// CA-642: el caso 4 nombra la tarjeta de ese Estado, no un slug fijo.
func TestCA642_ElCaso4NombraSuTarjeta(t *testing.T) {
	ev := bdArbol(piEv("writer", piConTope, piNada))
	ev.Item.Slug = "otra-tarjeta"
	e := aeEstado(ev, aePol0, aeSobres(aeS{rol: "test-writer", prov: "claude", col: ColTestWriter}))
	if e.Tarjeta.WaitingHuman || e.Tarjeta.Column == ColHecho || e.Tarjeta.Slug != "otra-tarjeta" {
		t.Fatalf("CA-642: fixture: la tarjeta sin espacio propio no es humana ni Hecho: %s %q", e.Tarjeta.Column, e.Tarjeta.Slug)
	}
	aeDetiene(t, "CA-642", "sin tarea abierta", Piloto(e),
		"el piloto corre en el espacio de trabajo de la tarjeta: abre la tarea con hoom task start otra-tarjeta", 4)
}

// CA-642: los casos 1 a 4 deciden antes que el corte, el reintento y la
// ronda, aunque la politica los encienda.
func TestCA642_LosCasos1a4VanAntesQueLaPolitica(t *testing.T) {
	pol := manifest.AutonomiaPolicy{ReintentosAnteRojo: 1, RondasDeHallazgos: 5, VentanaMax: aeEnt(80)}
	rojoW := aeRojoTest("v-w-rojo")
	reintento := aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "verify", status: aeNo, v: rojoW}
	corte := aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "run", status: aeNo, corte: envelope.CorteTiempo, sesion: "ses-1"}
	revisor := aeS{rol: "reviewer", prov: "gemini", col: ColReview}

	ev := piEv("writer", piConTope, piNada)
	ev.Item.Auto = ""
	e := aeEstado(ev, pol, aeSobres(reintento))
	aeDetiene(t, "CA-642", "sin auto, tras un rojo del writer", Piloto(e), piWhyNoActivado, 1)
	e = aeEstado(ev, pol, aeSobres(corte))
	aeDetiene(t, "CA-642", "sin auto, tras un corte", Piloto(e), piWhyNoActivado, 1)

	e = aeEstado(piEv("writer", piConTope, piEnCurso), pol, aeSobres(corte))
	aeDetiene(t, "CA-642", "en curso, tras un corte", Piloto(e), piWhyOtroTrabajo, 2)
	e = aeEstado(piEv("writer", piConTope, piInterrumpido), pol, aeSobres(reintento))
	aeDetiene(t, "CA-642", "interrumpida, tras un rojo del writer", Piloto(e), piWhyOtroTrabajo, 2)

	e = aeEstado(piEv("hecho", piConTope, piNada), pol, aeSobres(revisor))
	e.Bloqueantes = []finding.Item{bdHallazgo("f-hecho", "high", bdSlug)}
	if e.Tarjeta.Column != ColHecho {
		t.Fatalf("CA-642: fixture: la tarjeta cerrada sigue en Hecho: %s", e.Tarjeta.Column)
	}
	aeDetiene(t, "CA-642", "Hecho, tras un reviewer con bloqueantes", Piloto(e), piWhyHecho, 3)
	e = aeEstado(piEv("tu-aceptacion", piConTope, piNada), pol, aeSobres(corte))
	aeDetiene(t, "CA-642", "Tu aceptacion, tras un corte", Piloto(e), piWhyPersona("Tu aceptacion"), 3)

	e = aeEstado(bdArbol(piEv("writer", piConTope, piNada)), pol, aeSobres(corte))
	aeDetiene(t, "CA-642", "sin tarea, tras un corte", Piloto(e), piWhySinTarea, 4)
	e = aeEstado(bdArbol(piEv("writer", piConTope, piNada)), pol, aeSobres(reintento))
	aeDetiene(t, "CA-642", "sin tarea, tras un rojo del writer", Piloto(e), piWhySinTarea, 4)
}

// CA-642: la decision depende de Estado.Hora, nunca del reloj de la
// maquina. Un dato de ventana que reinicia una hora despues de Hora esta
// vigente aunque para el reloj ya paso (bdT0 es de septiembre de 2026), y uno
// que reinicia antes de Hora ya no cuenta aunque para el reloj todavia no
// llego.
func TestCA642_DecideConEstadoHoraNoConElReloj(t *testing.T) {
	ev := piEv("review-exigida", piConTope, piNada)
	ev.Providers = acProvs() // solo codex (sin tope en USD) puede revisar
	pol := manifest.AutonomiaPolicy{VentanaMax: aeEnt(80)}
	e := aeEstado(ev, pol, aeSobres(aeS{rol: "writer", prov: "claude", col: ColWriter}))
	aeCol(t, "CA-642", e, ColReview)

	e.Hora = bdT0.Add(4 * time.Hour)
	e.Ventanas = map[string]runcmd.VentanaRun{"codex": {Nombre: "5h", Inicio: 10, Fin: 90, ReiniciaEn: bdT0.Add(5 * time.Hour)}}
	if d := Piloto(e); d.Launch || d.Caso != 14 {
		t.Fatalf("CA-642: con Hora antes del reinicio la ventana de codex esta vigente y detiene el piloto (aunque el reloj diga que ya paso): %+v", d)
	}
	e.Hora = time.Date(2031, 1, 1, 12, 0, 0, 0, time.UTC)
	e.Ventanas = map[string]runcmd.VentanaRun{"codex": {Nombre: "5h", Inicio: 10, Fin: 90, ReiniciaEn: time.Date(2030, 6, 1, 0, 0, 0, 0, time.UTC)}}
	if d := Piloto(e); !d.Launch || d.Provider != "codex" {
		t.Fatalf("CA-642: con Hora despues del reinicio la ventana ya no cuenta y lanza (aunque el reloj diga que no llego): %+v", d)
	}
}

// CA-642: mismo Estado, misma decision, sin leer el disco: correr Piloto
// desde un directorio con un hoom.yaml y telemetria que dirian otra cosa no
// cambia nada, y Piloto no toca el Estado que recibe.
func TestCA642_MismoEstadoMismaDecisionSinDisco(t *testing.T) {
	muestras := aeMuestras(t)
	copias := aeMuestras(t)
	antes := make([]PilotDecision, len(muestras))
	for i, m := range muestras {
		antes[i] = Piloto(m.e)
		if !reflect.DeepEqual(m.e, copias[i].e) {
			t.Fatalf("CA-642: [%s] Piloto no cambia el Estado que recibe", m.nombre)
		}
		if otra := Piloto(m.e); !reflect.DeepEqual(otra, antes[i]) {
			t.Fatalf("CA-642: [%s] la misma decision para el mismo Estado:\n %+v\n %+v", m.nombre, antes[i], otra)
		}
	}
	dir := t.TempDir()
	escribir := func(rel, body string) {
		p := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escribir("hoom.yaml", "project: demo\nautonomia:\n  reintentos_ante_rojo: 1\n  rondas_de_hallazgos: 5\n  ventana_max: 1\n")
	escribir(".hoom/items/precios.yaml", "slug: precios\nauto: no\n")
	escribir(".hoom/envelopes/20260922T230000_zz0001.json", `{"id":"20260922T230000_zz0001","role":"writer","status":"en curso","task":"precios"}`)
	t.Chdir(dir)
	for i, m := range muestras {
		if d := Piloto(m.e); !reflect.DeepEqual(d, antes[i]) {
			t.Fatalf("CA-642: [%s] Piloto no lee el disco: la decision cambio al cambiar de directorio:\n %+v\n %+v", m.nombre, antes[i], d)
		}
	}
}

// CA-642: para ningun Estado la decision es aprobar, integrar ni lanzar al
// arquitecto: todas las formas de tarjeta (Tu aprobacion con una spec que se
// podria firmar, Tu aceptacion que se podria integrar, Backlog y Arquitecto
// que antes lanzaban al arquitecto) por todo ultimo sobre y toda politica.
func TestCA642_NuncaFirmaNiIntegraNiPideAlArquitecto(t *testing.T) {
	politicas := []manifest.AutonomiaPolicy{aePol0, {ReintentosAnteRojo: 1, RondasDeHallazgos: 5, VentanaMax: aeEnt(100)}}
	var ultimos [][]Sobre
	ultimos = append(ultimos, nil)
	for _, rol := range []string{"arquitecto", "test-writer", "writer", "reviewer", "refutador"} {
		for _, o := range []aeS{
			{rol: rol, prov: "claude", col: ColBacklog},
			{rol: rol, prov: "gemini", col: ""},
			{rol: rol, prov: "claude", col: ColTuAprobacion, stage: "verify", status: aeNo, v: aeRojoTest("v-x")},
			{rol: rol, prov: "claude", col: ColArquitecto, stage: "run", status: aeNo, corte: envelope.CorteTiempo, sesion: "s"},
			{rol: rol, prov: "gemini", col: ColReview, ronda: 1, regla: ReglaRondaReview},
		} {
			ultimos = append(ultimos, aeSobres(o))
		}
	}
	prohibidas := map[string]bool{ActAprobar: true, ActIntegrar: true, ActPedirArquitecto: true}
	for _, forma := range piFormas {
		for _, tope := range []string{piConTope, piSinTope} {
			ev := piEv(forma, tope, piNada)
			ev.Findings = append(ev.Findings, bdHallazgo("f-x", "high", bdSlug))
			for _, pol := range politicas {
				for _, u := range ultimos {
					e := aeEstado(ev, pol, u)
					d := Piloto(e)
					if prohibidas[d.Action] || d.Role == "arquitecto" {
						t.Fatalf("CA-642: [%s/%s] el piloto nunca decide aprobar, integrar ni pedir al arquitecto: %+v", forma, tope, d)
					}
					if d.Launch && e.Tarjeta.WaitingHuman {
						t.Fatalf("CA-642: [%s/%s] nunca lanza en una columna humana: %+v", forma, tope, d)
					}
				}
			}
		}
	}
}

// ===========================================================================
// CA-643: el rojo esperado del test-writer
// ===========================================================================

// aeTrasRojoTW es la tarjeta en Writer, en rojo por el veredicto del sobre
// del test-writer que cerro en stage con v.
func aeTrasRojoTW(stage, prov string, v *verdict.Verdict) Estado {
	ev := piEv("writer", piConTope, piNada)
	ev.Verdict = v
	return aeEstado(ev, aePol0, aeSobres(aeS{rol: "test-writer", prov: prov, col: ColTestWriter, stage: stage,
		status: aeNo, v: v, trasp: []string{"precios_test.go"}}))
}

// CA-643: tras un sobre del test-writer que es un rojo esperado (cerro en
// verify o check, rojo, con spec_lint, spec_trace y spec_approved en pass),
// Piloto lanza al writer con la regla cadena aunque la tarjeta este en rojo:
// para los casos 8 a 11 cuenta como entregable. Sin seccion autonomia.
func TestCA643_TrasElRojoEsperadoSigueElWriter(t *testing.T) {
	for _, c := range []struct{ stage, prov string }{{"verify", "claude"}, {"check", "claude"}, {"verify", "gemini"}} {
		e := aeTrasRojoTW(c.stage, c.prov, aeRojoTest("v-tw-rojo"))
		aeCol(t, "CA-643", e, ColWriter)
		if e.Tarjeta.Red == nil {
			t.Fatalf("CA-643: fixture: la tarjeta queda en rojo por el veredicto del test-writer")
		}
		a := acAccion(t, "CA-643", e.Tarjeta, ActPedirWriter)
		d := Piloto(e)
		nombre := "rojo esperado en " + c.stage + " de " + c.prov
		aeLanza(t, "CA-643", nombre, d, aeQuiero{accion: ActPedirWriter, rol: "writer", prov: c.prov, regla: ReglaCadena, pedido: a.Pedido})
		if a.BudgetUSD == nil || !bdCasi(d.BudgetUSD, *a.BudgetUSD) {
			t.Fatalf("CA-643: [%s] el presupuesto es el que propone la accion (%v), fue %v", nombre, a.BudgetUSD, d.BudgetUSD)
		}
	}
}

// CA-643: si en ese veredicto spec_trace (o spec_lint, o spec_approved) no
// esta en pass, o el sobre se corto en scope, no es un rojo esperado: se
// detiene con el Why del caso 8.
func TestCA643_SinLosGatesDelSpecOConScopeEsElCaso8(t *testing.T) {
	why := piWhyNoEntregable("test-writer", "no-entregable")
	casos := []struct {
		nombre string
		stage  string
		v      *verdict.Verdict
	}{
		{"spec_trace en fail", "verify", aeVer("v-trace", aeGates(map[string]string{"test": verdict.StatusFail, "spec_trace": verdict.StatusFail}))},
		{"spec_trace en error", "verify", aeVer("v-trace-e", aeGates(map[string]string{"test": verdict.StatusFail, "spec_trace": verdict.StatusError}))},
		{"spec_lint en fail", "check", aeVer("v-lint", aeGates(map[string]string{"test": verdict.StatusFail, "spec_lint": verdict.StatusFail}))},
		{"spec_approved en fail", "verify", aeVer("v-apr", aeGates(map[string]string{"test": verdict.StatusFail, "spec_approved": verdict.StatusFail}))},
		{"violacion de scope", "scope", nil},
		{"el run fallo", "run", nil},
	}
	for _, c := range casos {
		e := aeTrasRojoTW(c.stage, "claude", c.v)
		if c.v == nil {
			ev := piEv("writer", piConTope, piNada)
			e = aeEstado(ev, aePol0, aeSobres(aeS{rol: "test-writer", prov: "claude", col: ColTestWriter, stage: c.stage, status: aeNo}))
		}
		aeDetiene(t, "CA-643", c.nombre, Piloto(e), why, 8)
	}
}

// CA-643 y caso limite: un rojo esperado cuya tarjeta sigue en Test-writer
// (quedan criterios sin test) no avanzo: caso 10.
func TestCA643_RojoEsperadoSinAvanzarEsElCaso10(t *testing.T) {
	v := aeRojoTest("v-tw-rojo")
	ev := piEv("test-writer", piConTope, piNada)
	ev.Verdict = v
	e := aeEstado(ev, aePol0, aeSobres(aeS{rol: "test-writer", prov: "claude", col: ColTestWriter, stage: "verify", status: aeNo, v: v}))
	aeCol(t, "CA-643", e, ColTestWriter)
	aeDetiene(t, "CA-643", "rojo esperado en Test-writer", Piloto(e), piWhyNoAvanzo("Test-writer"), 10)
}

// ===========================================================================
// CA-645: Delta en la cadena
// ===========================================================================

// CA-645: con la regla cadena la review va con Delta cuando la tarjeta tiene
// una review que se puede encadenar (su siguiente paso propone hoom review
// ... --delta) y sin Delta cuando no.
func TestCA645_LaReviewDeLaCadenaVaConDeltaSiHayReviewEncadenable(t *testing.T) {
	ev := piEv("review-exigida", piConTope, piNada)
	ev.GreenVerdicts = []string{"v-viejo-verde", ev.Verdict.ID}
	ev.Reviews = []reviewcmd.Record{bdAnclado(reviewcmd.Record{ID: "r-viejo", CreatedAt: bdT0.Add(35 * time.Minute),
		Task: bdSlug, Spec: bdSpec, Fingerprint: "huella-1", VerdictID: "v-viejo-verde", Verdict: "green",
		Lenses: reviewcmd.Lentes, Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}})}
	ev.ReviewTails = map[string]ReviewTail{"r-viejo": {Ancestro: true, Codigo: true}}
	con := aeEstado(ev, aePol0, aeSobres(aeS{rol: "writer", prov: "claude", col: ColWriter}))
	aeCol(t, "CA-645", con, ColReview)
	if !strings.HasSuffix(con.Tarjeta.Next, " --delta") {
		t.Fatalf("CA-645: fixture: el siguiente paso propone el delta: %q", con.Tarjeta.Next)
	}
	a := acAccion(t, "CA-645", con.Tarjeta, ActPedirReviewer)
	if !a.Enabled || len(a.Providers) == 0 || a.Providers[0].Name != "claude" || a.Providers[0].OK {
		t.Fatalf("CA-645: fixture: pedir-reviewer habilitada y sin el writer (claude): %+v", a)
	}
	aeLanza(t, "CA-645", "con review encadenable", Piloto(con), aeQuiero{accion: ActPedirReviewer, rol: "reviewer",
		prov: "gemini", regla: ReglaCadena, delta: true, pedido: a.Pedido})

	sin := aeEstado(piEv("review-exigida", piConTope, piNada), aePol0, aeSobres(aeS{rol: "writer", prov: "claude", col: ColWriter}))
	aeCol(t, "CA-645", sin, ColReview)
	if strings.HasSuffix(sin.Tarjeta.Next, "--delta") {
		t.Fatalf("CA-645: fixture: sin review encadenable el siguiente paso no es el delta: %q", sin.Tarjeta.Next)
	}
	a = acAccion(t, "CA-645", sin.Tarjeta, ActPedirReviewer)
	aeLanza(t, "CA-645", "sin review encadenable", Piloto(sin), aeQuiero{accion: ActPedirReviewer, rol: "reviewer",
		prov: "gemini", regla: ReglaCadena, delta: false, pedido: a.Pedido})
}

// ===========================================================================
// CA-649 y CA-650: reintento ante un rojo
// ===========================================================================

// aeGatesWriter: build y los cinco de la tarjeta, mas un gate no requerido
// (semgrep) que no cuenta para los gates en rojo.
func aeGatesWriter(cambios map[string]string) []verdict.GateResult {
	gs := []verdict.GateResult{{Name: "build", Required: true, Status: verdict.StatusPass}}
	gs = append(gs, aeGates(nil)...)
	gs = append(gs, verdict.GateResult{Name: "semgrep", Required: false, Status: verdict.StatusPass})
	for i := range gs {
		if st, ok := cambios[gs[i].Name]; ok {
			gs[i].Status = st
		}
		gs[i].OutputTail = aeVeneno
	}
	return gs
}

// aeTrasRojoWriter: el test-writer dejo su rojo esperado y despues cerraron
// los sobres de writer dados; la tarjeta queda en Writer con el veredicto del
// ultimo.
func aeTrasRojoWriter(pol manifest.AutonomiaPolicy, tope string, writers ...aeS) Estado {
	ev := piEv("writer", piConTope, piNada)
	if tope == piSinTope {
		ev = piEv("writer", piSinTope, piNada)
	}
	xs := []aeS{{rol: "test-writer", prov: "claude", col: ColTestWriter, stage: "verify", status: aeNo, v: aeRojoTest("v-tw")}}
	xs = append(xs, writers...)
	if v := writers[len(writers)-1].v; v != nil {
		ev.Verdict = v
	}
	return aeEstado(ev, pol, aeSobres(xs...))
}

const aePedidoReintentoLiteral = "Intento 2 de precios. El veredicto v-w-rojo quedo ROJO en: build, test. La salida de cada gate esta en .hoom/verdicts/v-w-rojo.json. Corrige sobre el arbol actual hasta que verify quede verde. No toques los tests del test-writer."

// CA-649: con reintentos_ante_rojo: 1, tras el primer sobre del writer
// cerrado en verify (o check) con veredicto rojo y sin gates en error, lanza
// al writer con el mismo provider (gemini, aunque claude sea el primero del
// registro), en sesion nueva aunque haya id de sesion, Intento 2, regla
// reintento, caso 6, y el pedido del contrato con el id del veredicto y los
// gates requeridos en fail.
func TestCA649_ReintentaAlWriterUnaVez(t *testing.T) {
	pol := manifest.AutonomiaPolicy{ReintentosAnteRojo: 1}
	vW := aeVer("v-w-rojo", aeGatesWriter(map[string]string{"build": verdict.StatusFail, "test": verdict.StatusFail, "semgrep": verdict.StatusFail}))
	for _, stage := range []string{"verify", "check"} {
		e := aeTrasRojoWriter(pol, piConTope, aeS{rol: "writer", prov: "gemini", col: ColWriter, stage: stage, status: aeNo, v: vW, sesion: "ses-gemini-1"})
		aeCol(t, "CA-649", e, ColWriter)
		d := Piloto(e)
		aeLanza(t, "CA-649", "rojo del writer en "+stage, d, aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "gemini",
			regla: ReglaReintento, caso: 6, intento: 2, pedido: aePedidoReintentoLiteral})
		if d.Pedido != PedidoReintento(bdSlug, "v-w-rojo", []string{"build", "test"}) {
			t.Fatalf("CA-649: el pedido es PedidoReintento con el veredicto y los gates en rojo: %q", d.Pedido)
		}
		if !(d.BudgetUSD > 0) || d.BudgetUSD > 9+1e-9 {
			t.Fatalf("CA-649: gemini aplica tope en USD: lanza con presupuesto, sin pasar lo que queda (9): %v", d.BudgetUSD)
		}
	}
	// los sobres del writer dentro de una ronda no cuentan para n
	e := aeTrasRojoWriter(pol, piConTope,
		aeS{rol: "writer", prov: "claude", col: ColReview, regla: ReglaRondaWriter, ronda: 1, v: aeVerde("v-w-ronda")},
		aeS{rol: "writer", prov: "gemini", col: ColWriter, stage: "verify", status: aeNo, v: vW})
	aeLanza(t, "CA-649", "un writer de ronda no cuenta", Piloto(e), aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "gemini",
		regla: ReglaReintento, caso: 6, intento: 2, pedido: aePedidoReintentoLiteral})
}

// CA-650: en el caso 6, un gate en error detiene con entorno (antes que el
// rojo final); con dos sobres de writer fuera de ronda (entregables o no),
// rojo final; con reintentos_ante_rojo: 0, con un writer cortado en scope o
// con un run que fallo, el Why del caso 8. El reintento pasa por los casos 12
// y 14.
func TestCA650_ElReintentoSeDetiene(t *testing.T) {
	pol := manifest.AutonomiaPolicy{ReintentosAnteRojo: 1}
	vW := aeVer("v-w-rojo", aeGatesWriter(map[string]string{"build": verdict.StatusFail, "test": verdict.StatusFail}))
	vE := aeVer("v-w-error", aeGatesWriter(map[string]string{"build": verdict.StatusFail, "test": verdict.StatusError}))
	rojo := func(v *verdict.Verdict) aeS {
		return aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "verify", status: aeNo, v: v}
	}
	entorno := "entorno: el gate test no pudo correr: no se reintenta con modelos"

	aeDetiene(t, "CA-650", "un gate en error", Piloto(aeTrasRojoWriter(pol, piConTope, rojo(vE))), entorno, 6)
	aeDetiene(t, "CA-650", "un gate en error le gana al rojo final", Piloto(aeTrasRojoWriter(pol, piConTope, rojo(vW), rojo(vE))), entorno, 6)
	aeDetiene(t, "CA-650", "dos intentos rojos", Piloto(aeTrasRojoWriter(pol, piConTope, rojo(aeVer("v-w-1", aeGatesWriter(map[string]string{"test": verdict.StatusFail}))), rojo(vW))),
		"rojo final: 2 intentos del writer y el veredicto v-w-rojo sigue rojo en build, test", 6)
	aeDetiene(t, "CA-650", "n cuenta tambien un writer entregable", Piloto(aeTrasRojoWriter(pol, piConTope,
		aeS{rol: "writer", prov: "claude", col: ColWriter, v: aeVerde("v-w-verde")}, rojo(vW))),
		"rojo final: 2 intentos del writer y el veredicto v-w-rojo sigue rojo en build, test", 6)
	aeDetiene(t, "CA-650", "tres intentos", Piloto(aeTrasRojoWriter(pol, piConTope, rojo(vW), rojo(vW), rojo(vW))),
		"rojo final: 3 intentos del writer y el veredicto v-w-rojo sigue rojo en build, test", 6)

	caso8 := piWhyNoEntregable("writer", "no-entregable")
	aeDetiene(t, "CA-650", "reintentos_ante_rojo: 0", Piloto(aeTrasRojoWriter(aePol0, piConTope, rojo(vW))), caso8, 8)
	aeDetiene(t, "CA-650", "writer cortado en scope", Piloto(aeTrasRojoWriter(pol, piConTope,
		aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "scope", status: aeNo})), caso8, 8)
	aeDetiene(t, "CA-650", "el run del writer fallo", Piloto(aeTrasRojoWriter(pol, piConTope,
		aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "run", status: aeNo})), caso8, 8)
	aeDetiene(t, "CA-650", "el rojo no es del writer", Piloto(aeTrasRojoWriter(pol, piConTope,
		aeS{rol: "reviewer", prov: "gemini", col: ColReview, stage: "run", status: aeNo})), piWhyNoEntregable("reviewer", "no-entregable"), 8)

	aeDetiene(t, "CA-650", "el reintento pasa por el caso 12", Piloto(aeTrasRojoWriter(pol, piSinTope, rojo(vW))), piWhySinPresupuesto, 0)
	e := aeTrasRojoWriter(manifest.AutonomiaPolicy{ReintentosAnteRojo: 1, VentanaMax: aeEnt(50)}, piConTope, rojo(vW))
	e.Ventanas = map[string]runcmd.VentanaRun{"claude": {Nombre: "5h", Inicio: 20, Fin: 60, ReiniciaEn: aeHora.Add(2 * time.Hour)}}
	aeDetieneRe(t, "CA-650", "el reintento pasa por el caso 14", Piloto(e),
		regexp.MustCompile(`^la ventana de 5h de claude va en .*60.*% \(tope 50%\): el piloto espera; reinicia .+$`), 0)
}

// ===========================================================================
// CA-652: corte por tiempo
// ===========================================================================

// aeTrasCorte es la tarjeta del rol cortado (Writer, Test-writer o Review)
// tras los sobres dados; el ultimo es el cortado.
func aeTrasCorte(pol manifest.AutonomiaPolicy, forma, tope string, xs ...aeS) Estado {
	return aeEstado(piEv(forma, tope, piNada), pol, aeSobres(xs...))
}

func aeCortado(rol, prov, sesion string) aeS {
	return aeS{rol: rol, prov: prov, col: piColDelRol[rol], stage: "run", status: aeNo, corte: envelope.CorteTiempo, sesion: sesion}
}

// CA-652 y CA-658: tras el primer sobre de un rol con corte tiempo, si el rol
// no es ciego ni reviewer, el provider declara Resume y hay id de sesion,
// lanza con Resume y el pedido de relevo, regla relevo-por-tiempo, caso 5;
// conserva intento, ronda y hallazgos del sobre cortado.
func TestCA652_RelevaConResume(t *testing.T) {
	if PedidoRelevo != "Te cortaste por tiempo. Continua donde quedaste y termina el pedido original." {
		t.Fatalf("CA-658: el pedido de relevo es el del contrato: %q", PedidoRelevo)
	}
	tw := aeS{rol: "test-writer", prov: "claude", col: ColTestWriter}
	e := aeTrasCorte(aePol0, "writer", piConTope, tw, aeCortado("writer", "claude", "ses-w-1"))
	d := Piloto(e)
	aeLanza(t, "CA-652", "writer de claude con sesion", d, aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "claude",
		regla: ReglaRelevoPorTiempo, caso: 5, resume: "ses-w-1", pedido: PedidoRelevo})
	if !(d.BudgetUSD > 0) {
		t.Fatalf("CA-652: claude aplica tope en USD: el relevo lleva presupuesto: %v", d.BudgetUSD)
	}
	// sin pedido en el registro igual retoma: el pedido es el de relevo
	sinPedido := aeCortado("writer", "claude", "ses-w-2")
	sinPedido.pedido = aeStr("")
	aeLanza(t, "CA-652", "con Resume no hace falta el pedido guardado", Piloto(aeTrasCorte(aePol0, "writer", piConTope, tw, sinPedido)),
		aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "claude", regla: ReglaRelevoPorTiempo, caso: 5, resume: "ses-w-2", pedido: PedidoRelevo})
	// conserva el intento del reintento que se corto
	cortado := aeCortado("writer", "claude", "ses-w-3")
	cortado.regla, cortado.intento = ReglaReintento, 2
	aeLanza(t, "CA-652", "relevo de un reintento", Piloto(aeTrasCorte(manifest.AutonomiaPolicy{ReintentosAnteRojo: 1}, "writer", piConTope, tw, cortado)),
		aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "claude", regla: ReglaRelevoPorTiempo, caso: 5, intento: 2, resume: "ses-w-3", pedido: PedidoRelevo})
}

// CA-652: si no puede retomar (sin id de sesion, provider sin Resume, rol
// ciego o reviewer), repite el lanzamiento que se corto con el pedido de su
// registro: mismo rol, provider, intento, ronda y hallazgos. Al reviewer, las
// mismas opciones de review (sin Resume aunque codex declare Resume).
func TestCA652_RepiteConElPedidoGuardado(t *testing.T) {
	tw := aeS{rol: "test-writer", prov: "claude", col: ColTestWriter}
	sinSesion := aeCortado("writer", "claude", "")
	sinSesion.pedido = aeStr("el pedido que recibio el writer")
	aeLanza(t, "CA-652", "sin id de sesion", Piloto(aeTrasCorte(aePol0, "writer", piConTope, tw, sinSesion)),
		aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "claude", regla: ReglaRelevoPorTiempo, caso: 5, pedido: "el pedido que recibio el writer"})

	gemini := aeCortado("writer", "gemini", "ses-gemini")
	aeLanza(t, "CA-652", "gemini no declara Resume", Piloto(aeTrasCorte(aePol0, "writer", piConTope, tw, gemini)),
		aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "gemini", regla: ReglaRelevoPorTiempo, caso: 5, pedido: acPedWriter})

	ciego := aeCortado("test-writer", "claude", "ses-tw")
	aeLanza(t, "CA-652", "el test-writer es ciego", Piloto(aeTrasCorte(aePol0, "test-writer", piConTope, ciego)),
		aeQuiero{accion: ActPedirTestWriter, rol: "test-writer", prov: "claude", regla: ReglaRelevoPorTiempo, caso: 5, pedido: acPedTW})

	ronda := aeCortado("writer", "claude", "")
	ronda.regla, ronda.ronda, ronda.hallazgos = ReglaRondaWriter, 1, []string{"f1"}
	ronda.pedido = aeStr(PedidoRondaWriter(1, bdSlug, []string{"pkg/f1_test.go"}))
	aeLanza(t, "CA-652", "relevo dentro de una ronda", Piloto(aeTrasCorte(manifest.AutonomiaPolicy{RondasDeHallazgos: 2}, "writer", piConTope, tw, ronda)),
		aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "claude", regla: ReglaRelevoPorTiempo, caso: 5, ronda: 1,
			hallazgos: []string{"f1"}, pedido: PedidoRondaWriter(1, bdSlug, []string{"pkg/f1_test.go"})})

	revisor := aeCortado("reviewer", "codex", "ses-codex")
	revisor.pedido = aeStr("")
	d := Piloto(aeTrasCorte(manifest.AutonomiaPolicy{VentanaMax: aeEnt(80)}, "review-exigida", piConTope,
		aeS{rol: "writer", prov: "claude", col: ColWriter}, revisor))
	aeLanza(t, "CA-652", "el reviewer nunca retoma", d, aeQuiero{accion: ActPedirReviewer, rol: "reviewer", prov: "codex",
		regla: ReglaRelevoPorTiempo, caso: 5, libre: "pedido,delta"})
	if d.BudgetUSD != 0 {
		t.Fatalf("CA-652: codex no aplica tope en USD: el relevo va sin presupuesto en USD: %v", d.BudgetUSD)
	}
}

// CA-652: sin pedido en el registro (y sin poder retomar) se detiene con el
// Why del caso 8; con el segundo corte del mismo rol se detiene con la clave
// del timeout de ese rol (review.timeout para el reviewer). k cuenta los
// cortes de ese rol, no los de otro. El relevo pasa por el caso 12. Un corte
// que no es el ultimo sobre no decide.
func TestCA652_ElRelevoSeDetiene(t *testing.T) {
	tw := aeS{rol: "test-writer", prov: "claude", col: ColTestWriter}
	sinPedido := aeCortado("writer", "claude", "")
	sinPedido.pedido = aeStr("")
	d := Piloto(aeTrasCorte(aePol0, "writer", piConTope, tw, sinPedido))
	if d.Launch || d.Paso != PasoDetenerse || d.Why != piWhyNoEntregable("writer", "no-entregable") {
		t.Fatalf("CA-652: sin pedido en el registro se detiene como el caso 2 de hoy: %+v", d)
	}

	for _, c := range []struct{ rol, forma, prov, clave string }{
		{"writer", "writer", "claude", "agents.writer.timeout"},
		{"test-writer", "test-writer", "claude", "agents.test-writer.timeout"},
		{"reviewer", "review-exigida", "gemini", "review.timeout"},
	} {
		why := "el " + c.rol + " se corto por tiempo dos veces: sube " + c.clave + " o parte la tarea"
		e := aeTrasCorte(aePol0, c.forma, piConTope, aeCortado(c.rol, c.prov, "s1"), aeCortado(c.rol, c.prov, "s2"))
		aeDetiene(t, "CA-652", "dos cortes del "+c.rol, Piloto(e), why, 5)
		e = aeTrasCorte(aePol0, c.forma, piConTope, aeCortado(c.rol, c.prov, "s1"), aeS{rol: "writer", prov: "claude", col: ColWriter},
			aeCortado(c.rol, c.prov, "s2"))
		aeDetiene(t, "CA-652", "dos cortes del "+c.rol+" no seguidos", Piloto(e), why, 5)
	}
	// k es por rol: un corte del test-writer no cuenta para el writer
	e := aeTrasCorte(aePol0, "writer", piConTope, aeCortado("test-writer", "claude", "s1"), aeCortado("writer", "claude", "ses-w"))
	aeLanza(t, "CA-652", "un corte de otro rol no cuenta", Piloto(e), aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "claude",
		regla: ReglaRelevoPorTiempo, caso: 5, resume: "ses-w", pedido: PedidoRelevo})
	// el relevo pasa por el caso 12
	e = aeTrasCorte(aePol0, "writer", piSinTope, tw, aeCortado("writer", "claude", "ses-w"))
	aeDetiene(t, "CA-652", "relevo sin presupuesto", Piloto(e), piWhySinPresupuesto, 0)
	// un corte viejo no decide: el ultimo sobre es el que manda
	e = aeTrasCorte(aePol0, "review-exigida", piConTope, aeCortado("writer", "claude", "s1"), aeS{rol: "writer", prov: "claude", col: ColWriter})
	a := acAccion(t, "CA-652", e.Tarjeta, ActPedirReviewer)
	aeLanza(t, "CA-652", "corte viejo", Piloto(e), aeQuiero{accion: ActPedirReviewer, rol: "reviewer", prov: "gemini", regla: ReglaCadena, pedido: a.Pedido})
}

// ===========================================================================
// CA-653 a CA-655 y CA-657: la ronda de hallazgos
// ===========================================================================

func aeHallazgo(id, file, desc string) finding.Item {
	return finding.Item{Finding: finding.Finding{ID: id, CreatedAt: bdT0.Add(45 * time.Minute), Severity: "high",
		Lens: "risk", File: file, Description: desc, Author: "reviewer", Task: bdSlug}, Status: finding.StatusOpen}
}

var (
	aeF1 = aeHallazgo("f1", "precios.go", "el total redondea mal con descuentos")
	aeF2 = aeHallazgo("f2", "web/precios.js", "la region se pierde al recargar")
	aeF3 = aeHallazgo("f3", "precios.go", "falta validar la moneda")
	aeF4 = aeHallazgo("f4", "api/precios.go", "el endpoint no pagina")
)

func aeRev(id, prov string, min int) reviewcmd.Record {
	return reviewcmd.Record{ID: id, CreatedAt: bdT0.Add(time.Duration(min) * time.Minute), Task: bdSlug, Spec: bdSpec,
		Provider: prov, Writer: "claude", Findings: []string{}, Notes: []string{aeVeneno}}
}

// aeRonda es la tarjeta de una ronda: en Review por sus bloqueantes (o, sin
// bloqueantes, con la review exigida), con dos registros de review: el mas
// nuevo es de gemini.
func aeRonda(pol manifest.AutonomiaPolicy, bloq []finding.Item, xs ...aeS) Estado {
	ev := piEv("tu-aceptacion", piConTope, piNada)
	if len(bloq) == 0 {
		ev = piEv("review-exigida", piConTope, piNada)
	}
	ev.Findings = append([]finding.Item(nil), bloq...)
	ev.Reviews = []reviewcmd.Record{aeRev("r-viejo", "codex", 35), aeRev("r-nuevo", "gemini", 50)}
	return aeEstado(ev, pol, aeSobres(xs...))
}

// aeBase son los sobres antes de la primera ronda: el test-writer (gemini)
// con su rojo esperado, dos writers (el mas nuevo, de claude, verde) y el
// reviewer que encontro los bloqueantes.
func aeBase() []aeS {
	return []aeS{
		{rol: "test-writer", prov: "gemini", col: ColTestWriter, stage: "verify", status: aeNo, v: aeRojoTest("v-tw0"), trasp: []string{"precios_test.go"}},
		{rol: "writer", prov: "gemini", col: ColWriter, stage: "verify", status: aeNo, v: aeRojoTest("v-w-viejo")},
		{rol: "writer", prov: "claude", col: ColWriter, v: aeVerde("v-w0")},
		{rol: "reviewer", prov: "gemini", col: ColReview},
	}
}

func aeMas(base []aeS, mas ...aeS) []aeS { return append(append([]aeS{}, base...), mas...) }

func aeSin(base []aeS, rol string) []aeS {
	var out []aeS
	for _, o := range base {
		if o.rol != rol {
			out = append(out, o)
		}
	}
	return out
}

// Los sobres de la ronda 1.
func aeRef1(ids []string, bloq int) aeS {
	return aeS{rol: "refutador", prov: "gemini", col: ColReview, regla: ReglaRondaRefutador, ronda: 1, hallazgos: ids,
		bloq: bloq, pedido: aeStr(PedidoRondaRefutador(1, bdSlug, ids))}
}

func aeTW1(stage, status string, v *verdict.Verdict, trasp ...string) aeS {
	return aeS{rol: "test-writer", prov: "claude", col: ColReview, stage: stage, status: status, v: v,
		regla: ReglaRondaTestWriter, ronda: 1, hallazgos: []string{"f1"}, trasp: trasp}
}

func aeW1(stage, status string, v *verdict.Verdict) aeS {
	return aeS{rol: "writer", prov: "claude", col: ColReview, stage: stage, status: status, v: v, regla: ReglaRondaWriter, ronda: 1}
}

var aeRV1 = aeS{rol: "reviewer", prov: "gemini", col: ColReview, regla: ReglaRondaReview, ronda: 1}

const aePedidoRefutadorLiteral = "Ronda 1 de hallazgos de precios. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: f1, f2. No edites codigo."

// CA-653: al empezar la primera ronda (el ultimo sobre es del reviewer y hay
// bloqueantes) lanza al refutador con el provider del registro de review mas
// nuevo (gemini, no codex), los ids que ningun refutador vio, Ronda 1,
// Bloqueantes k y su pedido. Sin rondas en la politica no hay ronda: la
// tarjeta no avanzo.
func TestCA653_EmpiezaLaRondaConElRefutador(t *testing.T) {
	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	e := aeRonda(pol, []finding.Item{aeF1, aeF2}, aeBase()...)
	aeCol(t, "CA-653", e, ColReview)
	d := Piloto(e)
	aeLanza(t, "CA-653", "primera ronda", d, aeQuiero{accion: ActPedirRefutador, rol: "refutador", prov: "gemini",
		regla: ReglaRondaRefutador, caso: 7, ronda: 1, hallazgos: []string{"f1", "f2"}, bloq: 2, pedido: aePedidoRefutadorLiteral})
	if d.Pedido != PedidoRondaRefutador(1, bdSlug, []string{"f1", "f2"}) {
		t.Fatalf("CA-653: el pedido es PedidoRondaRefutador: %q", d.Pedido)
	}

	e = aeRonda(aePol0, []finding.Item{aeF1, aeF2}, aeBase()...)
	aeDetiene(t, "CA-653", "sin rondas en la politica", Piloto(e), piWhyNoAvanzo("Review"), 10)
}

// CA-653: se detiene cuando n pasa de rondas_de_hallazgos y cuando los
// bloqueantes no bajan respecto de los que habia al empezar la ronda
// anterior (los `bloqueantes` del primer sobre de esa ronda).
func TestCA653_LaRondaSeDetiene(t *testing.T) {
	ronda1 := aeMas(aeBase(), aeRef1([]string{"f1", "f2"}, 2), aeTW1("verify", aeNo, aeRojoTest("v-tw1"), "pkg/f1_test.go"),
		aeW1("ok", aeOK, aeVerde("v-w1")), aeRV1)
	e := aeRonda(manifest.AutonomiaPolicy{RondasDeHallazgos: 1}, []finding.Item{aeF3}, ronda1...)
	aeDetiene(t, "CA-653", "n pasa de rondas_de_hallazgos", Piloto(e), "se hicieron 1 rondas de hallazgos y quedan 1 que bloquean: te toca", 7)

	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	e = aeRonda(pol, []finding.Item{aeF1, aeF3}, ronda1...)
	aeDetiene(t, "CA-653", "los bloqueantes no bajan", Piloto(e), "los hallazgos que bloquean no bajan (2 -> 2): te toca", 7)
	e = aeRonda(pol, []finding.Item{aeF1, aeF3, aeF4}, ronda1...)
	aeDetiene(t, "CA-653", "los bloqueantes suben", Piloto(e), "los hallazgos que bloquean no bajan (2 -> 3): te toca", 7)
}

// CA-653: en la ronda 2 el refutador recibe solo los ids que todavia no
// nombra ningun sobre de refutador que no corto en run; cuando todos ya
// pasaron, la ronda sigue con el test-writer (el provider del sobre de
// test-writer mas nuevo), que es su primer lanzamiento y lleva Bloqueantes.
func TestCA653_LaSegundaRondaFiltraLoQueYaRefuto(t *testing.T) {
	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	ronda1 := aeMas(aeBase(), aeRef1([]string{"f1", "f2", "f3"}, 3), aeTW1("verify", aeNo, aeRojoTest("v-tw1"), "pkg/f1_test.go"),
		aeW1("ok", aeOK, aeVerde("v-w1")), aeRV1)

	e := aeRonda(pol, []finding.Item{aeF1, aeF4}, ronda1...)
	aeLanza(t, "CA-653", "solo f4 no paso por un refutador", Piloto(e), aeQuiero{accion: ActPedirRefutador, rol: "refutador",
		prov: "gemini", regla: ReglaRondaRefutador, caso: 7, ronda: 2, hallazgos: []string{"f4"}, bloq: 2,
		pedido: PedidoRondaRefutador(2, bdSlug, []string{"f4"})})

	e = aeRonda(pol, []finding.Item{aeF1, aeF2}, ronda1...)
	aeLanza(t, "CA-653", "todos ya pasaron", Piloto(e), aeQuiero{accion: ActPedirTestWriter, rol: "test-writer",
		prov: "claude", regla: ReglaRondaTestWriter, caso: 7, ronda: 2, hallazgos: []string{"f1", "f2"}, bloq: 2,
		pedido: PedidoRondaTestWriter(2, bdSlug, []finding.Item{aeF1, aeF2})})

	// un refutador que corto en run no cuenta: f1 y f2 no pasaron
	cortado := aeRef1([]string{"f1", "f2", "f3"}, 3)
	cortado.stage, cortado.status = "run", aeNo
	mano := aeS{rol: "reviewer", prov: "gemini", col: ColReview}
	e = aeRonda(pol, []finding.Item{aeF1, aeF2}, aeMas(aeBase(), cortado, mano)...)
	aeLanza(t, "CA-653", "el refutador que corto en run no cuenta", Piloto(e), aeQuiero{accion: ActPedirRefutador, rol: "refutador",
		prov: "gemini", regla: ReglaRondaRefutador, caso: 7, ronda: 2, hallazgos: []string{"f1", "f2"}, bloq: 2,
		pedido: PedidoRondaRefutador(2, bdSlug, []string{"f1", "f2"})})
}

// CA-654: despues del refutador lanza al test-writer de la ronda con los
// bloqueantes que quedan y el provider del test-writer mas nuevo (gemini, no
// el primero ok); sin bloqueantes la ronda termina y sigue la tabla; sin
// sobre de test-writer se detiene.
func TestCA654_DespuesDelRefutador(t *testing.T) {
	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	tras := aeMas(aeBase(), aeRef1([]string{"f1", "f2"}, 2))
	e := aeRonda(pol, []finding.Item{aeF1}, tras...)
	aeLanza(t, "CA-654", "queda f1", Piloto(e), aeQuiero{accion: ActPedirTestWriter, rol: "test-writer", prov: "gemini",
		regla: ReglaRondaTestWriter, caso: 7, ronda: 1, hallazgos: []string{"f1"},
		pedido: PedidoRondaTestWriter(1, bdSlug, []finding.Item{aeF1})})

	e = aeRonda(pol, []finding.Item{aeF1}, aeMas(aeSin(aeBase(), "test-writer"), aeRef1([]string{"f1", "f2"}, 2))...)
	aeDetiene(t, "CA-654", "sin sobre de test-writer", Piloto(e), "la tarjeta no tiene un sobre de test-writer: el piloto no elige provider por su cuenta", 0)

	// el refutador refuto todo: la tarjeta queda en Tu aceptacion y sigue la tabla
	ev := piEv("tu-aceptacion", piConTope, piNada)
	ev.Reviews = []reviewcmd.Record{aeRev("r-nuevo", "gemini", 50)}
	e = aeEstado(ev, pol, aeSobres(tras...))
	aeDetiene(t, "CA-654", "el refutador refuto todo", Piloto(e), piWhyPersona("Tu aceptacion"), 3)
}

// CA-654: despues del test-writer de la ronda: con su rojo esperado lanza al
// writer de la ronda (el provider del writer mas nuevo: claude, no gemini)
// con el pedido de la ronda; con su veredicto verde se detiene; cualquier
// otro cierre se detiene como el caso 2 de hoy; sin sobre de writer se
// detiene.
func TestCA654_DespuesDelTestWriterDeLaRonda(t *testing.T) {
	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	antes := aeMas(aeBase(), aeRef1([]string{"f1", "f2"}, 2))
	conCitas := func(e Estado) Estado {
		e.Citas = map[string][]string{"f1": {"pkg/f1_test.go"}}
		return e
	}
	e := conCitas(aeRonda(pol, []finding.Item{aeF1}, aeMas(antes, aeTW1("verify", aeNo, aeRojoTest("v-tw1"), "pkg/f1_test.go"))...))
	d := Piloto(e)
	aeLanza(t, "CA-654", "rojo esperado del test-writer", d, aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "claude",
		regla: ReglaRondaWriter, caso: 7, ronda: 1, libre: "hallazgos",
		pedido: "Ronda 1 de hallazgos de precios. Estos tests fallan y cada uno cita su hallazgo: pkg/f1_test.go. Corrige el codigo hasta que pasen. No toques esos tests."})

	e = conCitas(aeRonda(pol, []finding.Item{aeF1}, aeMas(antes, aeTW1("ok", aeOK, aeVerde("v-tw1-verde"), "pkg/f1_test.go"))...))
	aeDetiene(t, "CA-654", "los tests de la ronda no fallan", Piloto(e), "los tests de la ronda 1 no fallan: los hallazgos no se reproducen: te toca", 7)

	for nombre, tw1 := range map[string]aeS{
		"el run del test-writer fallo": aeTW1("run", aeNo, nil),
		"spec_trace en fail":           aeTW1("verify", aeNo, aeVer("v-tw1-trace", aeGates(map[string]string{"test": verdict.StatusFail, "spec_trace": verdict.StatusFail})), "pkg/f1_test.go"),
		"cortado en scope":             aeTW1("scope", aeNo, nil),
	} {
		e = conCitas(aeRonda(pol, []finding.Item{aeF1}, aeMas(antes, tw1)...))
		d := Piloto(e)
		if d.Launch || d.Paso != PasoDetenerse || d.Why != piWhyNoEntregable("test-writer", "no-entregable") {
			t.Fatalf("CA-654: [%s] cualquier otro cierre dentro de una ronda se detiene como el caso 2 de hoy: %+v", nombre, d)
		}
	}

	e = conCitas(aeRonda(pol, []finding.Item{aeF1}, aeMas(aeSin(antes, "writer"), aeTW1("verify", aeNo, aeRojoTest("v-tw1"), "pkg/f1_test.go"))...))
	aeDetiene(t, "CA-654", "la tarjeta escrita fuera del sobre", Piloto(e), "la tarjeta no tiene un sobre de writer: el piloto no elige provider por su cuenta", 0)
}

// aeAntesDelArreglo son los sobres hasta el test-writer de la ronda 1, cuyo
// trasplante dejo dos tests que citan a f1.
func aeAntesDelArreglo() []aeS {
	return aeMas(aeBase(), aeRef1([]string{"f1", "f2"}, 2),
		aeTW1("verify", aeNo, aeRojoTest("v-tw1"), "pkg/a_test.go", "pkg/f1_test.go"))
}

// CA-655: despues del writer de la ronda, con el veredicto rojo solo por
// findings_open y f1 citado por tests protegidos de esta ronda, el paso es
// cerrar-hallazgos con f1 (f2 no esta citado), regla ronda-cierre y la
// evidencia literal: el primer test en orden, el veredicto rojo del
// test-writer y el del writer. No lanza nada.
func TestCA655_CierraLosHallazgosCitados(t *testing.T) {
	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	vW1 := aeRojoHallazgos("v-w1")
	e := aeRonda(pol, []finding.Item{aeF1, aeF2}, aeMas(aeAntesDelArreglo(), aeW1("verify", aeNo, vW1))...)
	e.Veredicto = vW1
	e.Citas = map[string][]string{"f1": {"pkg/a_test.go", "pkg/f1_test.go"}}
	d := Piloto(e)
	want := map[string]string{"f1": "ronda 1: el test pkg/a_test.go cita el hallazgo; veredicto v-tw1 rojo antes del arreglo; despues, veredicto v-w1 con todos los demas gates en pass"}
	if d.Launch || d.Paso != PasoCerrarHallazgos || d.Regla != ReglaRondaCierre || d.Ronda != 1 || d.Why != "" || d.Caso != 7 ||
		!reflect.DeepEqual(d.Hallazgos, []string{"f1"}) || !reflect.DeepEqual(d.Evidencias, want) {
		t.Fatalf("CA-655: cerrar-hallazgos con f1:\n quiero hallazgos [f1] evidencias %q\n fue launch=%v paso=%q regla=%q ronda=%d caso=%d why=%q hallazgos=%q evidencias=%q",
			want, d.Launch, d.Paso, d.Regla, d.Ronda, d.Caso, d.Why, d.Hallazgos, d.Evidencias)
	}

	// los dos citados, en el orden de los bloqueantes
	e.Citas = map[string][]string{"f1": {"pkg/f1_test.go"}, "f2": {"pkg/a_test.go"}}
	d = Piloto(e)
	if d.Paso != PasoCerrarHallazgos || !reflect.DeepEqual(d.Hallazgos, []string{"f1", "f2"}) || len(d.Evidencias) != 2 ||
		!strings.Contains(d.Evidencias["f2"], "el test pkg/a_test.go cita el hallazgo") ||
		!strings.Contains(d.Evidencias["f1"], "el test pkg/f1_test.go cita el hallazgo") {
		t.Fatalf("CA-655: cierra los dos citados, cada uno con su test: %+v", d)
	}

	// f2 lo cita un test de antes de la ronda: no es de esta ronda
	e.Citas = map[string][]string{"f1": {"pkg/f1_test.go"}, "f2": {"precios_test.go"}}
	d = Piloto(e)
	if d.Paso != PasoCerrarHallazgos || !reflect.DeepEqual(d.Hallazgos, []string{"f1"}) || len(d.Evidencias) != 1 {
		t.Fatalf("CA-655: solo cuentan los tests protegidos de esta ronda (precios_test.go es del test-writer de antes): %+v", d)
	}
}

// CA-655: ningun bloqueante citado se detiene con los ids; rojo por otro
// gate se detiene con los gates; sin reintentos dentro de una ronda aunque
// la politica los tenga.
func TestCA655_ElArregloSeDetiene(t *testing.T) {
	for _, pol := range []manifest.AutonomiaPolicy{{RondasDeHallazgos: 2}, {RondasDeHallazgos: 2, ReintentosAnteRojo: 1}} {
		vW1 := aeRojoHallazgos("v-w1")
		e := aeRonda(pol, []finding.Item{aeF1, aeF2}, aeMas(aeAntesDelArreglo(), aeW1("verify", aeNo, vW1))...)
		e.Veredicto = vW1
		aeDetiene(t, "CA-655", "ninguno citado", Piloto(e), "quedan hallazgos sin test en la ronda 1: f1, f2: te toca", 7)

		vT := aeRojoTest("v-w1-test")
		e = aeRonda(pol, []finding.Item{aeF1, aeF2}, aeMas(aeAntesDelArreglo(), aeW1("verify", aeNo, vT))...)
		e.Veredicto = vT
		e.Citas = map[string][]string{"f1": {"pkg/f1_test.go"}}
		aeDetiene(t, "CA-655", "rojo por el gate test", Piloto(e), "el arreglo de la ronda 1 quedo rojo en test: te toca", 7)

		e = aeRonda(pol, []finding.Item{aeF1, aeF2}, aeMas(aeAntesDelArreglo(), aeW1("scope", aeNo, nil))...)
		d := Piloto(e)
		if d.Launch || d.Paso != PasoDetenerse || d.Why != piWhyNoEntregable("writer", "no-entregable") {
			t.Fatalf("CA-655: un writer de ronda cortado en scope se detiene como el caso 2 de hoy: %+v", d)
		}
	}
}

// CA-657: despues del writer de la ronda con el ultimo veredicto de la
// tarjeta verde, lanza al reviewer con Delta, regla ronda-review y el
// provider del registro de review mas nuevo; despues de ese reviewer empieza
// otra ronda si hay bloqueantes y sigue la tabla si no.
func TestCA657_LaRondaTerminaConLaReview(t *testing.T) {
	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	vW1 := aeVerde("v-w1")
	e := aeRonda(pol, nil, aeMas(aeAntesDelArreglo(), aeW1("ok", aeOK, vW1))...)
	e.Veredicto = vW1
	aeCol(t, "CA-657", e, ColReview)
	aeLanza(t, "CA-657", "writer de la ronda en verde", Piloto(e), aeQuiero{accion: ActPedirReviewer, rol: "reviewer",
		prov: "gemini", regla: ReglaRondaReview, caso: 7, ronda: 1, delta: true, libre: "pedido,hallazgos"})

	// tras el cierre de hoom: el sobre del writer sigue rojo por findings_open,
	// pero el ultimo veredicto de la tarjeta es el verde de despues de cerrar
	e = aeRonda(pol, nil, aeMas(aeAntesDelArreglo(), aeW1("verify", aeNo, aeRojoHallazgos("v-w1-rojo")))...)
	e.Veredicto = aeVerde("v-tras-cerrar")
	aeLanza(t, "CA-657", "tras cerrar-hallazgos", Piloto(e), aeQuiero{accion: ActPedirReviewer, rol: "reviewer",
		prov: "gemini", regla: ReglaRondaReview, caso: 7, ronda: 1, delta: true, libre: "pedido,hallazgos"})

	// el reviewer de la ronda 1 encontro f4: empieza la ronda 2
	ronda1 := aeMas(aeBase(), aeRef1([]string{"f1", "f2", "f3"}, 3), aeTW1("verify", aeNo, aeRojoTest("v-tw1"), "pkg/f1_test.go"),
		aeW1("ok", aeOK, vW1), aeRV1)
	e = aeRonda(pol, []finding.Item{aeF4}, ronda1...)
	aeLanza(t, "CA-657", "otra ronda", Piloto(e), aeQuiero{accion: ActPedirRefutador, rol: "refutador", prov: "gemini",
		regla: ReglaRondaRefutador, caso: 7, ronda: 2, hallazgos: []string{"f4"}, bloq: 1,
		pedido: PedidoRondaRefutador(2, bdSlug, []string{"f4"})})

	// sin bloqueantes: la ronda termina y sigue la tabla (Tu aceptacion)
	ev := piEv("tu-aceptacion", piConTope, piNada)
	e = aeEstado(ev, pol, aeSobres(ronda1...))
	aeDetiene(t, "CA-657", "el reviewer de la ronda no encontro nada", Piloto(e), piWhyPersona("Tu aceptacion"), 3)
}

// CA-657: quien corre cada paso sale del disco: el registro de review mas
// nuevo para el refutador y el reviewer; sin registro de review se detiene.
// El provider que sale del disco pasa por el caso 13: codex sin ventana_max
// no tiene tope; con ventana_max lanza sin presupuesto en USD.
func TestCA657_ElProviderSaleDelDisco(t *testing.T) {
	pol := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	e := aeRonda(pol, []finding.Item{aeF1, aeF2}, aeBase()...)
	e.Reviews = nil
	aeDetieneRe(t, "CA-657", "sin registro de review", Piloto(e),
		regexp.MustCompile(`^la tarjeta no tiene un sobre de (refutador|reviewer): el piloto no elige provider por su cuenta$`), 0)

	e = aeRonda(pol, []finding.Item{aeF1, aeF2}, aeBase()...)
	e.Reviews = []reviewcmd.Record{aeRev("r-nuevo", "gemini", 35), aeRev("r-codex", "codex", 55)}
	aeDetiene(t, "CA-657", "codex sin ventana_max", Piloto(e), piWhySinProveedor, 0)

	e.Politica.VentanaMax = aeEnt(80)
	d := Piloto(e)
	aeLanza(t, "CA-657", "codex con ventana_max", d, aeQuiero{accion: ActPedirRefutador, rol: "refutador", prov: "codex",
		regla: ReglaRondaRefutador, caso: 7, ronda: 1, hallazgos: []string{"f1", "f2"}, bloq: 2, pedido: aePedidoRefutadorLiteral})
	if d.BudgetUSD != 0 {
		t.Fatalf("CA-657: codex no aplica tope en USD: va sin presupuesto en USD: %v", d.BudgetUSD)
	}
}

// ===========================================================================
// CA-658: los pedidos
// ===========================================================================

// CA-658: los pedidos de reintento, relevo y ronda son los textos del
// contrato con sus valores; las listas van unidas con ", ".
func TestCA658_LosPedidosSonLosDelContrato(t *testing.T) {
	casos := []struct{ nombre, got, want string }{
		{"reintento", PedidoReintento("precios", "2026-09-22T15-30-00Z_aaaa1111", []string{"lint", "test"}),
			"Intento 2 de precios. El veredicto 2026-09-22T15-30-00Z_aaaa1111 quedo ROJO en: lint, test. La salida de cada gate esta en .hoom/verdicts/2026-09-22T15-30-00Z_aaaa1111.json. Corrige sobre el arbol actual hasta que verify quede verde. No toques los tests del test-writer."},
		{"reintento con un gate", PedidoReintento("otra", "v-9", []string{"build"}),
			"Intento 2 de otra. El veredicto v-9 quedo ROJO en: build. La salida de cada gate esta en .hoom/verdicts/v-9.json. Corrige sobre el arbol actual hasta que verify quede verde. No toques los tests del test-writer."},
		{"relevo", PedidoRelevo, "Te cortaste por tiempo. Continua donde quedaste y termina el pedido original."},
		{"refutador", PedidoRondaRefutador(3, "precios", []string{"a1b2c3", "d4e5f6"}),
			"Ronda 3 de hallazgos de precios. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: a1b2c3, d4e5f6. No edites codigo."},
		{"writer de la ronda", PedidoRondaWriter(2, "precios", []string{"internal/x/a_test.go", "web/b.test.js"}),
			"Ronda 2 de hallazgos de precios. Estos tests fallan y cada uno cita su hallazgo: internal/x/a_test.go, web/b.test.js. Corrige el codigo hasta que pasen. No toques esos tests."},
	}
	for _, c := range casos {
		if c.got != c.want {
			t.Fatalf("CA-658: el pedido de %s es el del contrato:\n quiero %q\n fue    %q", c.nombre, c.want, c.got)
		}
	}
	tw := PedidoRondaTestWriter(1, "precios", []finding.Item{aeF1, aeF2})
	want := "Ronda 1 de hallazgos de precios. Por cada hallazgo escribe un test que falle hoy y cita su id en el test.\n" +
		"f1 [high] precios.go: el total redondea mal con descuentos\n" +
		"f2 [high] web/precios.js: la region se pierde al recargar"
	if strings.TrimRight(tw, "\n") != want {
		t.Fatalf("CA-658: el pedido del test-writer de la ronda es el del contrato, un hallazgo por linea en el orden dado:\n quiero %q\n fue    %q", want, tw)
	}
	if strings.Contains(tw, aeVeneno) || strings.Contains(PedidoRondaTestWriter(1, "precios", []finding.Item{aeF2, aeF1}), "f1 [high] precios.go: el total redondea mal con descuentos\nf2") {
		t.Fatalf("CA-658: el orden es el dado")
	}
}

// aePedidoDelContrato dice si el pedido de una decision viene solo de una
// plantilla, de ids, de rutas, de nombres de gate, de hallazgos en disco o
// del pedido del sobre que se repite (CA-658).
func aePedidoDelContrato(e Estado, d PilotDecision) error {
	if strings.Contains(d.Pedido, aeVeneno) {
		return fmt.Errorf("el pedido lleva narracion de un run: %q", d.Pedido)
	}
	for id, ev := range d.Evidencias {
		if strings.Contains(ev, aeVeneno) {
			return fmt.Errorf("la evidencia de %s lleva narracion de un run: %q", id, ev)
		}
	}
	if !d.Launch {
		return nil
	}
	slug := e.Tarjeta.Slug
	var ultimo Sobre
	if len(e.Sobres) > 0 {
		ultimo = e.Sobres[len(e.Sobres)-1]
	}
	switch d.Regla {
	case ReglaCadena:
		a, ok := e.Tarjeta.Action(d.Action)
		if !ok {
			a, ok = e.Tarjeta.PilotAction(d.Action)
		}
		if !ok || d.Pedido != a.Pedido {
			return fmt.Errorf("con la regla cadena el pedido es el de la accion %q, fue %q", a.Pedido, d.Pedido)
		}
	case ReglaReintento:
		v := ultimo.Veredicto
		if v == nil {
			v = e.Veredicto
		}
		if v == nil {
			return fmt.Errorf("un reintento sin veredicto")
		}
		var gates []string
		for _, g := range v.Gates {
			if g.Required && g.Status == verdict.StatusFail {
				gates = append(gates, g.Name)
			}
		}
		if d.Pedido != PedidoReintento(slug, v.ID, gates) {
			return fmt.Errorf("el pedido del reintento es PedidoReintento(%s, %s, %v), fue %q", slug, v.ID, gates, d.Pedido)
		}
	case ReglaRelevoPorTiempo:
		if !(d.Resume != "" && d.Pedido == PedidoRelevo) && d.Pedido != ultimo.Record.Pedido {
			return fmt.Errorf("el relevo lleva el pedido de relevo (con Resume) o el pedido guardado %q, fue %q", ultimo.Record.Pedido, d.Pedido)
		}
	case ReglaRondaRefutador:
		if d.Pedido != PedidoRondaRefutador(d.Ronda, slug, d.Hallazgos) {
			return fmt.Errorf("el pedido del refutador es el de la plantilla con %v, fue %q", d.Hallazgos, d.Pedido)
		}
	case ReglaRondaTestWriter:
		var items []finding.Item
		for _, id := range d.Hallazgos {
			for _, b := range e.Bloqueantes {
				if b.ID == id {
					items = append(items, b)
				}
			}
		}
		if strings.TrimRight(d.Pedido, "\n") != strings.TrimRight(PedidoRondaTestWriter(d.Ronda, slug, items), "\n") {
			return fmt.Errorf("el pedido del test-writer de la ronda es la plantilla con los hallazgos en disco %v, fue %q", d.Hallazgos, d.Pedido)
		}
	case ReglaRondaWriter:
		pre := fmt.Sprintf("Ronda %d de hallazgos de %s. Estos tests fallan y cada uno cita su hallazgo: ", d.Ronda, slug)
		suf := ". Corrige el codigo hasta que pasen. No toques esos tests."
		if !strings.HasPrefix(d.Pedido, pre) || !strings.HasSuffix(d.Pedido, suf) || len(d.Pedido) < len(pre)+len(suf) {
			return fmt.Errorf("el pedido del writer de la ronda es la plantilla, fue %q", d.Pedido)
		}
		rutas := map[string]bool{}
		for _, s := range e.Sobres {
			for _, r := range s.Record.Trasplantados {
				rutas[r] = true
			}
		}
		for _, rs := range e.Citas {
			for _, r := range rs {
				rutas[r] = true
			}
		}
		for _, r := range strings.Split(d.Pedido[len(pre):len(d.Pedido)-len(suf)], ", ") {
			if !rutas[r] {
				return fmt.Errorf("el pedido del writer de la ronda nombra %q, que no es un test protegido", r)
			}
		}
	}
	return nil
}

// CA-658: para cualquier Estado (las muestras de los fixtures y los de la
// simulacion de CA-660), el pedido no lleva texto que no venga del contrato:
// la nota del sobre, la salida de los gates y las notas de la review llevan
// aeVeneno y nunca aparece.
func TestCA658_NingunPedidoLlevaNarracion(t *testing.T) {
	lanzados := 0
	for _, m := range aeMuestras(t) {
		d := Piloto(m.e)
		if err := aePedidoDelContrato(m.e, d); err != nil {
			t.Fatalf("CA-658: [%s] %v", m.nombre, err)
		}
		if d.Launch {
			lanzados++
		}
	}
	if lanzados == 0 {
		t.Fatal("CA-658: alguna muestra lanza")
	}
	cartas := aeSimCartas(t)
	f := func(semilla int64) bool {
		_, _, traza, err := aeSimular(cartas, semilla, aePedidoDelContrato)
		if err != nil {
			t.Errorf("CA-658: [semilla %d] %v\n traza: %s", semilla, err, strings.Join(traza, " -> "))
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 800, Rand: rand.New(rand.NewSource(658))}); err != nil {
		t.Fatal(err)
	}
}

// ===========================================================================
// CA-659: la ventana de 5 h
// ===========================================================================

// aeSoloCodex: la tarjeta en Review con la review exigida y solo codex (sin
// tope en USD) para revisar (claude escribio, gemini no esta instalado).
func aeSoloCodex(pol manifest.AutonomiaPolicy, ventanas map[string]runcmd.VentanaRun) Estado {
	ev := piEv("review-exigida", piConTope, piNada)
	ev.Providers = acProvs()
	e := aeEstado(ev, pol, aeSobres(aeS{rol: "writer", prov: "claude", col: ColWriter}))
	e.Ventanas = ventanas
	return e
}

var aeReinicio = time.Date(2026, 9, 22, 22, 47, 0, 0, time.UTC) // una hora y 47 despues de aeHora

// CA-659: con ventana_max declarada, un provider sin tope en USD cuenta como
// con tope y la review se lanza con codex sin presupuesto en USD; sin
// ventana_max se detiene con el Why del caso 13.
func TestCA659_ConVentanaMaxCodexRevisaSinUSD(t *testing.T) {
	e := aeSoloCodex(aePol0, nil)
	aeCol(t, "CA-659", e, ColReview)
	aeDetiene(t, "CA-659", "sin ventana_max", Piloto(e), piWhySinProveedor, 13)

	e = aeSoloCodex(manifest.AutonomiaPolicy{VentanaMax: aeEnt(80)}, nil)
	a := acAccion(t, "CA-659", e.Tarjeta, ActPedirReviewer)
	d := Piloto(e)
	aeLanza(t, "CA-659", "sin dato de ventana", d, aeQuiero{accion: ActPedirReviewer, rol: "reviewer", prov: "codex",
		regla: ReglaCadena, pedido: a.Pedido})
	if d.BudgetUSD != 0 {
		t.Fatalf("CA-659: codex se lanza sin presupuesto en USD: %v", d.BudgetUSD)
	}
}

// CA-659: se detiene con el texto del contrato cuando el ultimo dato vigente
// de la ventana del provider elegido llego al tope (80 incluido); lanza con
// 79.9, con un dato cuyo reinicio ya paso, con un dato sin reinicia_en o con
// el dato de otro provider.
func TestCA659_LaVentanaDetieneAlProviderElegido(t *testing.T) {
	pol := manifest.AutonomiaPolicy{VentanaMax: aeEnt(80)}
	why := regexp.MustCompile(`^la ventana de 5h de codex va en (.+)% \(tope 80%\): el piloto espera; reinicia (.+)$`)
	for _, fin := range []float64{80, 93} {
		e := aeSoloCodex(pol, map[string]runcmd.VentanaRun{"codex": {Nombre: "5h", Inicio: 12, Fin: fin, ReiniciaEn: aeReinicio}})
		d := Piloto(e)
		aeDetieneRe(t, "CA-659", fmt.Sprintf("codex en %v%%", fin), d, why, 14)
		m := why.FindStringSubmatch(d.Why)
		if entero := fmt.Sprintf("%d", int(fin)); !strings.Contains(m[1], entero) {
			t.Fatalf("CA-659: el porcentaje de la ventana aparece en el why (%s): %q", entero, d.Why)
		}
		if !strings.Contains(m[2], "22") || !strings.Contains(m[2], "47") {
			t.Fatalf("CA-659: la hora UTC del reinicio (22:47) aparece en el why: %q", d.Why)
		}
	}
	lanza := map[string]map[string]runcmd.VentanaRun{
		"79.9":                {"codex": {Nombre: "5h", Inicio: 12, Fin: 79.9, ReiniciaEn: aeReinicio}},
		"reinicio que paso":   {"codex": {Nombre: "5h", Inicio: 12, Fin: 99, ReiniciaEn: aeHora.Add(-time.Minute)}},
		"sin reinicia_en":     {"codex": {Nombre: "5h", Inicio: 12, Fin: 99}},
		"la de otro provider": {"claude": {Nombre: "5h", Inicio: 12, Fin: 99, ReiniciaEn: aeReinicio}},
	}
	for nombre, v := range lanza {
		e := aeSoloCodex(pol, v)
		d := Piloto(e)
		if !d.Launch || d.Provider != "codex" || d.Caso != 0 {
			t.Fatalf("CA-659: [%s] sin dato vigente que llegue al tope, lanza con codex: %+v", nombre, d)
		}
	}
}

// CA-659: el caso 14 mira tambien al provider con tope en USD que eligio la
// cadena, solo con ventana_max declarada; claude sigue con su presupuesto en
// USD.
func TestCA659_LaVentanaMiraAlProviderConUSD(t *testing.T) {
	enCurso := map[string]runcmd.VentanaRun{"claude": {Nombre: "5h", Inicio: 40, Fin: 90, ReiniciaEn: aeReinicio}}
	tras := aeSobres(aeS{rol: "test-writer", prov: "claude", col: ColTestWriter})
	e := aeEstado(piEv("writer", piConTope, piNada), manifest.AutonomiaPolicy{VentanaMax: aeEnt(85)}, tras)
	e.Ventanas = enCurso
	aeDetieneRe(t, "CA-659", "claude en 90 con tope 85", Piloto(e),
		regexp.MustCompile(`^la ventana de 5h de claude va en .*90.*% \(tope 85%\): el piloto espera; reinicia .+$`), 14)

	e = aeEstado(piEv("writer", piConTope, piNada), aePol0, tras)
	e.Ventanas = enCurso
	a := acAccion(t, "CA-659", e.Tarjeta, ActPedirWriter)
	aeLanza(t, "CA-659", "sin ventana_max la ventana no detiene", Piloto(e), aeQuiero{accion: ActPedirWriter, rol: "writer",
		prov: "claude", regla: ReglaCadena, pedido: a.Pedido})

	e = aeEstado(piEv("writer", piConTope, piNada), manifest.AutonomiaPolicy{VentanaMax: aeEnt(95)}, tras)
	e.Ventanas = enCurso
	d := Piloto(e)
	aeLanza(t, "CA-659", "debajo del tope", d, aeQuiero{accion: ActPedirWriter, rol: "writer", prov: "claude", regla: ReglaCadena, pedido: a.Pedido})
	if !bdCasi(d.BudgetUSD, 9) {
		t.Fatalf("CA-659: claude declara tope en USD: lanza con su presupuesto (9), fue %v", d.BudgetUSD)
	}
}

// ===========================================================================
// CA-660: la cota de lanzamientos
// ===========================================================================

// CA-660 (y CA-643, CA-649, CA-653 a CA-657 encadenados): una cadena larga
// con reintentos_ante_rojo: 1 y rondas_de_hallazgos: 1, paso a paso, con la
// tarjeta y los cierres que dejaria cada trabajo: test-writer, writer (rojo),
// reintento, reviewer (dos bloqueantes), refutador (refuta f2), test-writer
// de la ronda, writer de la ronda (rojo solo por findings_open),
// cerrar-hallazgos, reviewer de la ronda y Tu aceptacion. Ocho lanzamientos,
// dentro de la cota 3 + 1 + 4*1 + 4.
func TestCA660_UnaCadenaLargaCabeEnLaCota(t *testing.T) {
	pol := manifest.AutonomiaPolicy{ReintentosAnteRojo: 1, RondasDeHallazgos: 1}
	var hechos []aeS
	var reviews []reviewcmd.Record
	paso := func(nombre string, ev Evidence, bloq []finding.Item, citas map[string][]string, ver *verdict.Verdict) PilotDecision {
		t.Helper()
		ev.Reviews = reviews
		ev.Findings = append([]finding.Item(nil), bloq...)
		e := aeEstado(ev, pol, aeSobres(hechos...))
		if ver != nil {
			e.Veredicto = ver
		}
		e.Citas = citas
		d := Piloto(e)
		if err := aePedidoDelContrato(e, d); err != nil {
			t.Fatalf("CA-658: [%s] %v", nombre, err)
		}
		return d
	}
	var roles []string
	lanzo := func(nombre string, d PilotDecision, rol, regla string) {
		t.Helper()
		if !d.Launch || d.Role != rol || d.Regla != regla {
			t.Fatalf("CA-660: [%s] lanza al %s con la regla %s, fue launch=%v rol=%q regla=%q why=%q", nombre, rol, regla, d.Launch, d.Role, d.Regla, d.Why)
		}
		roles = append(roles, rol)
	}

	// 1. la firma: Test-writer sin sobres
	d := paso("firma", piEv("test-writer", piConTope, piNada), nil, nil, nil)
	lanzo("firma", d, "test-writer", ReglaCadena)
	vTW := aeRojoTest("v-1-tw")
	hechos = append(hechos, aeS{rol: d.Role, prov: d.Provider, col: ColTestWriter, stage: "verify", status: aeNo, v: vTW,
		trasp: []string{"precios_test.go"}, regla: d.Regla, pedido: aeStr(d.Pedido)})

	// 2. el rojo esperado deja la tarjeta en Writer
	evW := piEv("writer", piConTope, piNada)
	evW.Verdict = vTW
	d = paso("rojo esperado", evW, nil, nil, nil)
	lanzo("rojo esperado", d, "writer", ReglaCadena)
	vW := aeRojoTest("v-2-w")
	hechos = append(hechos, aeS{rol: d.Role, prov: d.Provider, col: ColWriter, stage: "verify", status: aeNo, v: vW, regla: d.Regla, pedido: aeStr(d.Pedido)})

	// 3. el writer quedo rojo: reintento
	evW.Verdict = vW
	d = paso("writer rojo", evW, nil, nil, nil)
	lanzo("writer rojo", d, "writer", ReglaReintento)
	if d.Intento != 2 || d.Pedido != PedidoReintento(bdSlug, "v-2-w", []string{"test"}) {
		t.Fatalf("CA-649: el reintento lleva Intento 2 y su pedido: %+v", d)
	}
	hechos = append(hechos, aeS{rol: d.Role, prov: d.Provider, col: ColWriter, v: aeVerde("v-3-w"), regla: d.Regla, intento: 2, pedido: aeStr(d.Pedido)})

	// 4. verde y grande: la review
	d = paso("verde", piEv("review-exigida", piConTope, piNada), nil, nil, nil)
	lanzo("verde", d, "reviewer", ReglaCadena)
	revisor := d.Provider
	hechos = append(hechos, aeS{rol: d.Role, prov: d.Provider, col: ColReview, regla: d.Regla})
	reviews = append(reviews, aeRev("r-4", revisor, 200))

	// 5. dos bloqueantes: empieza la ronda 1 con el refutador del registro
	evR := piEv("tu-aceptacion", piConTope, piNada)
	d = paso("bloqueantes", evR, []finding.Item{aeF1, aeF2}, nil, nil)
	lanzo("bloqueantes", d, "refutador", ReglaRondaRefutador)
	if d.Provider != revisor || d.Ronda != 1 || d.Bloqueantes != 2 || !reflect.DeepEqual(d.Hallazgos, []string{"f1", "f2"}) {
		t.Fatalf("CA-653: el refutador es la CLI que reviso, ronda 1, dos bloqueantes: %+v", d)
	}
	hechos = append(hechos, aeS{rol: d.Role, prov: d.Provider, col: ColReview, regla: d.Regla, ronda: 1, hallazgos: d.Hallazgos,
		bloq: d.Bloqueantes, pedido: aeStr(d.Pedido)})

	// 6. refuto f2: el test-writer de la ronda con f1
	d = paso("refutado f2", evR, []finding.Item{aeF1}, nil, nil)
	lanzo("refutado f2", d, "test-writer", ReglaRondaTestWriter)
	if d.Provider != hechos[0].prov || d.Ronda != 1 || !reflect.DeepEqual(d.Hallazgos, []string{"f1"}) {
		t.Fatalf("CA-654: el test-writer de la ronda es el del sobre de test-writer, con f1: %+v", d)
	}
	vTW1 := aeRojoTest("v-6-tw")
	hechos = append(hechos, aeS{rol: d.Role, prov: d.Provider, col: ColReview, stage: "verify", status: aeNo, v: vTW1,
		trasp: []string{"pkg/f1_test.go"}, regla: d.Regla, ronda: 1, hallazgos: d.Hallazgos, pedido: aeStr(d.Pedido)})
	citas := map[string][]string{"f1": {"pkg/f1_test.go"}}

	// 7. rojo esperado de la ronda: el writer de la ronda
	d = paso("rojo de la ronda", evR, []finding.Item{aeF1}, citas, vTW1)
	lanzo("rojo de la ronda", d, "writer", ReglaRondaWriter)
	if d.Pedido != PedidoRondaWriter(1, bdSlug, []string{"pkg/f1_test.go"}) {
		t.Fatalf("CA-654: el pedido del writer de la ronda nombra el test que cita f1: %q", d.Pedido)
	}
	vW1 := aeRojoHallazgos("v-7-w")
	hechos = append(hechos, aeS{rol: d.Role, prov: d.Provider, col: ColReview, stage: "verify", status: aeNo, v: vW1, regla: d.Regla, ronda: 1, pedido: aeStr(d.Pedido)})

	// 8. rojo solo por findings_open con f1 citado: cerrar-hallazgos
	d = paso("arreglo", evR, []finding.Item{aeF1}, citas, vW1)
	if d.Paso != PasoCerrarHallazgos || d.Launch || !reflect.DeepEqual(d.Hallazgos, []string{"f1"}) ||
		d.Evidencias["f1"] != "ronda 1: el test pkg/f1_test.go cita el hallazgo; veredicto v-6-tw rojo antes del arreglo; despues, veredicto v-7-w con todos los demas gates en pass" {
		t.Fatalf("CA-655: cerrar-hallazgos con f1 y su evidencia: %+v", d)
	}

	// 9. hoom cerro f1 y verify quedo verde: el reviewer de la ronda con Delta
	d = paso("cerrado", piEv("review-exigida", piConTope, piNada), nil, nil, aeVerde("v-8-verde"))
	lanzo("cerrado", d, "reviewer", ReglaRondaReview)
	if !d.Delta || d.Provider != revisor || d.Ronda != 1 {
		t.Fatalf("CA-657: el reviewer de la ronda va con Delta y la CLI del registro: %+v", d)
	}
	hechos = append(hechos, aeS{rol: d.Role, prov: d.Provider, col: ColReview, regla: d.Regla, ronda: 1})

	// 10. sin bloqueantes: sigue la tabla y llega a Tu aceptacion
	d = paso("final", piEv("tu-aceptacion", piConTope, piNada), nil, nil, aeVerde("v-8-verde"))
	aeDetiene(t, "CA-660", "final", d, piWhyPersona("Tu aceptacion"), 3)

	want := []string{"test-writer", "writer", "writer", "reviewer", "refutador", "test-writer", "writer", "reviewer"}
	if !reflect.DeepEqual(roles, want) || len(roles) > 3+1+4*1+4 {
		t.Fatalf("CA-660: la cadena lanza %v, dentro de la cota %d; fue %v", want, 3+1+4+4, roles)
	}
}

// CA-660 (re-expresa CA-380 en la cota): para cualquier politica valida y
// cualquier secuencia de cierres, los lanzamientos del piloto en una tarjeta
// no pasan de 3 + reintentos_ante_rojo + 4*rondas_de_hallazgos + 4. La
// simulacion encadena Piloto: cada trabajo que lanza cierra al azar
// (entregable, rojo en verify o check, rojo solo por findings_open, rojo de
// spec_trace, run fallido, corte por tiempo con o sin sesion, scope), la
// tarjeta avanza o se queda (nunca vuelve), el reviewer abre bloqueantes, el
// refutador refuta, el test-writer los cita; cerrar-hallazgos no cuenta como
// lanzamiento. En cada vuelta tambien valen las invariantes de CA-642 y
// CA-659 (aeInvariantes).
func TestCA660_LaCotaParaCualquierPoliticaYCierres(t *testing.T) {
	cartas := aeSimCartas(t)
	maximo := 0
	f := func(semilla int64) bool {
		lanz, cota, traza, err := aeSimular(cartas, semilla, aeInvariantes)
		if err != nil {
			t.Errorf("CA-660: [semilla %d] %v\n traza: %s", semilla, err, strings.Join(traza, " -> "))
			return false
		}
		if lanz > cota {
			t.Errorf("CA-660: [semilla %d] %d lanzamientos pasan la cota %d: %s", semilla, lanz, cota, strings.Join(traza, " -> "))
			return false
		}
		if lanz > maximo {
			maximo = lanz
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 3000, Rand: rand.New(rand.NewSource(660))}); err != nil {
		t.Fatal(err)
	}
	if maximo < 2 {
		t.Fatalf("CA-660: la simulacion no encadena nada (maximo %d lanzamientos): no prueba la cota", maximo)
	}
}

// aeInvariantes son las que valen para toda decision (CA-642, CA-658,
// CA-659, CA-660).
func aeInvariantes(e Estado, d PilotDecision) error {
	if d.Action == ActAprobar || d.Action == ActIntegrar || d.Action == ActPedirArquitecto || d.Role == "arquitecto" {
		return fmt.Errorf("CA-642: nunca firma, integra ni pide al arquitecto: %+v", d)
	}
	if d.Launch != (d.Paso == PasoLanzar) {
		return fmt.Errorf("CA-642: Launch es verdadero solo con el paso lanzar: %+v", d)
	}
	c := e.Tarjeta
	switch d.Paso {
	case PasoDetenerse:
		if d.Why == "" || d.Regla != "" {
			return fmt.Errorf("CA-642: un detenerse dice por que y no lleva regla: %+v", d)
		}
	case PasoLanzar:
		roles := map[string]string{ActPedirTestWriter: "test-writer", ActPedirWriter: "writer", ActPedirReviewer: "reviewer", ActPedirRefutador: "refutador"}
		reglas := map[string]bool{ReglaCadena: true, ReglaReintento: true, ReglaRelevoPorTiempo: true, ReglaRondaRefutador: true,
			ReglaRondaTestWriter: true, ReglaRondaWriter: true, ReglaRondaReview: true}
		if roles[d.Action] == "" || roles[d.Action] != d.Role || d.Why != "" || !reglas[d.Regla] {
			return fmt.Errorf("CA-642: un lanzamiento es de un rol del piloto, con su regla y sin why: %+v", d)
		}
		if c.Item.Auto != item.AutoHastaHumano || c.Running != nil || c.Interrupted != nil || c.WaitingHuman || c.Column == ColHecho {
			return fmt.Errorf("CA-642: los casos 1 a 3 van antes que todo lanzamiento: %+v", d)
		}
		if c.Item.PresupuestoUSD == nil {
			return fmt.Errorf("CA-660: todo lanzamiento pasa por el caso 12: %+v", d)
		}
		usd, conocido := false, false
		for _, p := range e.Proveedores {
			if p.Name == d.Provider {
				usd, conocido = p.Capabilities.Budget, true
			}
		}
		if !conocido {
			return fmt.Errorf("CA-657: el provider %q no es de la maquina", d.Provider)
		}
		if usd && !(d.BudgetUSD > 0) {
			return fmt.Errorf("CA-659: %s aplica tope en USD: lanza con presupuesto: %+v", d.Provider, d)
		}
		if !usd && (e.Politica.VentanaMax == nil || d.BudgetUSD != 0) {
			return fmt.Errorf("CA-659: %s no aplica tope en USD: solo con ventana_max y sin presupuesto en USD: %+v", d.Provider, d)
		}
		if vm := e.Politica.VentanaMax; vm != nil {
			if v, ok := e.Ventanas[d.Provider]; ok && !v.ReiniciaEn.IsZero() && v.ReiniciaEn.After(e.Hora) && v.Fin >= float64(*vm) {
				return fmt.Errorf("CA-659: la ventana de %s va en %v (tope %d): no lanza: %+v", d.Provider, v.Fin, *vm, d)
			}
		}
		if d.Regla == ReglaReintento && (e.Politica.ReintentosAnteRojo == 0 || d.Intento != 2 || d.Resume != "" || d.Role != "writer") {
			return fmt.Errorf("CA-649: el reintento es del writer, con la politica, Intento 2 y sesion nueva: %+v", d)
		}
		if strings.HasPrefix(d.Regla, "ronda-") && (e.Politica.RondasDeHallazgos == 0 || d.Ronda < 1 || d.Ronda > e.Politica.RondasDeHallazgos) {
			return fmt.Errorf("CA-653: una ronda solo con rondas_de_hallazgos y n entre 1 y la politica: %+v", d)
		}
		if d.Resume != "" && (d.Regla != ReglaRelevoPorTiempo || d.Role == "test-writer" || d.Role == "reviewer" || d.Pedido != PedidoRelevo) {
			return fmt.Errorf("CA-652: Resume solo en el relevo de un rol que no es ciego ni reviewer, con el pedido de relevo: %+v", d)
		}
	case PasoCerrarHallazgos:
		if d.Regla != ReglaRondaCierre || len(d.Hallazgos) == 0 || d.Why != "" || d.Ronda < 1 {
			return fmt.Errorf("CA-655: cerrar-hallazgos lleva la regla ronda-cierre, su ronda y los ids: %+v", d)
		}
		for _, id := range d.Hallazgos {
			if len(e.Citas[id]) == 0 {
				return fmt.Errorf("CA-655: cierra solo bloqueantes citados por un test: %s (%+v)", id, d)
			}
			if !strings.HasPrefix(d.Evidencias[id], fmt.Sprintf("ronda %d: el test ", d.Ronda)) {
				return fmt.Errorf("CA-655: cada hallazgo cerrado lleva su evidencia: %s %q", id, d.Evidencias[id])
			}
		}
	default:
		return fmt.Errorf("CA-642: el paso es lanzar, cerrar-hallazgos o detenerse: %q", d.Paso)
	}
	return aePedidoDelContrato(e, d)
}

// aeSimCols son las columnas por las que pasa la tarjeta en la simulacion,
// en orden.
var aeSimCols = []string{ColTestWriter, ColWriter, ColReview, ColTuAceptacion, ColHecho}

// aeSimCartas son las tarjetas de cada columna (casi todas con presupuesto y
// sin nada; algunas sin presupuesto, en rojo o con otro trabajo).
func aeSimCartas(t *testing.T) map[string][]Card {
	t.Helper()
	formas := map[string][]string{ColTestWriter: {"test-writer"}, ColWriter: {"writer"},
		ColReview: {"review-exigida", "review-hallazgos"}, ColTuAceptacion: {"tu-aceptacion"}, ColHecho: {"hecho"}}
	out := map[string][]Card{}
	for _, col := range aeSimCols {
		for _, forma := range formas[col] {
			for _, tope := range []string{piConTope, piConTope, piConTope, piSinTope} {
				for _, extra := range []string{piNada, piNada, piNada, piNada, piRojo, piEnCurso} {
					out[col] = append(out[col], piCard(t, forma, tope, extra))
				}
			}
		}
	}
	return out
}

type aeSim struct {
	n, nv, sigF int
	sobres      []Sobre
	reviews     []reviewcmd.Record
	bloq        []finding.Item
	citas       map[string][]string
	veredicto   *verdict.Verdict
	ventanas    map[string]runcmd.VentanaRun
}

// estado es una copia: Piloto no puede cambiar la simulacion.
func (s *aeSim) estado(c Card, pol manifest.AutonomiaPolicy) Estado {
	citas := map[string][]string{}
	for k, v := range s.citas {
		citas[k] = append([]string(nil), v...)
	}
	ventanas := map[string]runcmd.VentanaRun{}
	for k, v := range s.ventanas {
		ventanas[k] = v
	}
	return Estado{Tarjeta: c, Sobres: append([]Sobre(nil), s.sobres...), Veredicto: s.veredicto,
		Reviews: append([]reviewcmd.Record(nil), s.reviews...), Bloqueantes: append([]finding.Item(nil), s.bloq...),
		Citas: citas, Politica: pol, Ventanas: ventanas, Proveedores: piProvs(), Hora: aeHora}
}

func (s *aeSim) ver(cambios map[string]string) *verdict.Verdict {
	s.nv++
	return bdVeredicto(fmt.Sprintf("v-sim-%04d", s.nv), bdT0.Add(time.Duration(100+s.nv)*time.Minute), "huella-1", 50, 10, aeGates(cambios))
}

func (s *aeSim) hallazgo() finding.Item {
	s.sigF++
	id := fmt.Sprintf("fs%03d", s.sigF)
	return aeHallazgo(id, "pkg/"+id+".go", "descripcion en disco de "+id)
}

// cerrar agrega el sobre cerrado del trabajo de d, lanzado desde columna,
// con un cierre al azar, y sus efectos sobre los bloqueantes, las citas y
// las reviews.
func (s *aeSim) cerrar(r *rand.Rand, d PilotDecision, columna string, piloto bool) {
	s.n++
	id := fmt.Sprintf("20260923T%06d_sim%04d", s.n, s.n)
	ini := bdT0.Add(time.Duration(120+7*s.n) * time.Minute)
	rec := envelope.Record{ID: id, RunID: "run-" + id, Role: d.Role, Provider: d.Provider, Task: bdSlug, Dir: bdWtDir,
		Spec: bdSpec, Steps: 6, StartedAt: ini, UpdatedAt: ini.Add(3 * time.Minute), EndedAt: ini.Add(3 * time.Minute),
		Columna: columna, Pedido: d.Pedido, Pilot: piloto, Regla: d.Regla, Intento: d.Intento, Ronda: d.Ronda,
		Hallazgos: append([]string(nil), d.Hallazgos...), Bloqueantes: d.Bloqueantes, Note: aeVeneno + " " + id}
	so := Sobre{Record: rec}
	codigo := d.Role == "writer" || d.Role == "test-writer"
	cierre := r.Intn(9)
	if !codigo && cierre >= 2 && cierre <= 5 {
		cierre = 0
	}
	var v *verdict.Verdict
	switch cierre {
	case 0, 1:
		so.Record.Stage, so.Record.Step, so.Record.Status = "ok", 6, aeOK
		if codigo {
			v = s.ver(nil)
		}
	case 2:
		so.Record.Stage, so.Record.Step, so.Record.Status = "verify", 5, aeNo
		v = s.ver(map[string]string{"test": verdict.StatusFail})
	case 3:
		so.Record.Stage, so.Record.Step, so.Record.Status = "verify", 5, aeNo
		v = s.ver(map[string]string{"findings_open": verdict.StatusFail})
	case 4:
		so.Record.Stage, so.Record.Step, so.Record.Status = "verify", 5, aeNo
		v = s.ver(map[string]string{"test": verdict.StatusFail, "spec_trace": verdict.StatusFail})
	case 5:
		so.Record.Stage, so.Record.Step, so.Record.Status = "check", 6, aeNo
		v = s.ver(map[string]string{"test": verdict.StatusFail})
	case 6:
		so.Record.Stage, so.Record.Step, so.Record.Status = "run", 3, aeNo
	case 7:
		so.Record.Stage, so.Record.Step, so.Record.Status, so.Record.Corte = "run", 3, aeNo, envelope.CorteTiempo
		if r.Intn(2) == 0 {
			so.SesionID = "ses-" + id
		}
	case 8:
		so.Record.Stage, so.Record.Step, so.Record.Status = "scope", 4, aeNo
	}
	if so.Record.Status != aeOK {
		so.Record.ExitCode = 1
	}
	if v != nil {
		so.Veredicto = v
		so.Record.VerdictID, so.Record.Verdict = v.ID, v.Verdict
		s.veredicto = v
	}
	if d.Role == "test-writer" && (so.Record.Stage == "verify" || so.Record.Stage == "check" || so.Record.Stage == "ok") {
		ruta := fmt.Sprintf("pkg/t%03d_test.go", s.n)
		so.Record.Trasplantados = []string{ruta}
		for _, b := range s.bloq {
			if r.Intn(2) == 0 {
				s.citas[b.ID] = append(s.citas[b.ID], ruta)
				sort.Strings(s.citas[b.ID])
			}
		}
	}
	if so.Record.Status == aeOK {
		switch d.Role {
		case "reviewer":
			var ids []string
			for i := r.Intn(3); i > 0; i-- {
				f := s.hallazgo()
				s.bloq = append(s.bloq, f)
				ids = append(ids, f.ID)
			}
			s.reviews = append(s.reviews, reviewcmd.Record{ID: "r-" + id, CreatedAt: ini, Task: bdSlug, Spec: bdSpec,
				Provider: d.Provider, Writer: "claude", Findings: ids, Notes: []string{aeVeneno}})
		case "refutador":
			var quedan []finding.Item
			for _, b := range s.bloq {
				if piContiene(d.Hallazgos, b.ID) && r.Intn(2) == 0 {
					delete(s.citas, b.ID)
					continue
				}
				quedan = append(quedan, b)
			}
			s.bloq = quedan
		}
	}
	s.sobres = append(s.sobres, so)
}

// aeSimular encadena Piloto sobre una tarjeta hasta que se detiene. Devuelve
// los lanzamientos, la cota de la politica y la traza; err si check falla en
// alguna vuelta, si Piloto no es pura o cambia el Estado, o si no termina.
func aeSimular(cartas map[string][]Card, semilla int64, check func(Estado, PilotDecision) error) (int, int, []string, error) {
	r := rand.New(rand.NewSource(semilla))
	pol := manifest.AutonomiaPolicy{ReintentosAnteRojo: r.Intn(2), RondasDeHallazgos: r.Intn(6)}
	switch r.Intn(4) {
	case 1:
		pol.VentanaMax = aeEnt(50)
	case 2:
		pol.VentanaMax = aeEnt(80)
	case 3:
		pol.VentanaMax = aeEnt(100)
	}
	cota := 3 + pol.ReintentosAnteRojo + 4*pol.RondasDeHallazgos + 4
	traza := []string{fmt.Sprintf("politica r=%d R=%d", pol.ReintentosAnteRojo, pol.RondasDeHallazgos)}
	ci := r.Intn(3)
	carta := func() Card {
		cs := cartas[aeSimCols[ci]]
		return cs[r.Intn(len(cs))]
	}
	s := &aeSim{citas: map[string][]string{}, ventanas: map[string]runcmd.VentanaRun{}}
	s.reviews = []reviewcmd.Record{{ID: "r-sim-0", CreatedAt: bdT0.Add(30 * time.Minute), Task: bdSlug, Spec: bdSpec,
		Provider: []string{"gemini", "codex"}[r.Intn(2)], Writer: "claude", Notes: []string{aeVeneno}}}
	for i := r.Intn(3); i > 0; i-- {
		s.bloq = append(s.bloq, s.hallazgo())
	}
	for _, p := range []string{"claude", "codex", "gemini"} {
		if r.Intn(2) == 0 {
			s.ventanas[p] = runcmd.VentanaRun{Nombre: "5h", Inicio: 1, Fin: float64(r.Intn(101)),
				ReiniciaEn: aeHora.Add(time.Duration(r.Intn(5)-2) * time.Hour)}
		}
	}
	tarjeta := carta()
	if r.Intn(2) == 0 { // un trabajo que pidio una persona antes de encender el piloto
		rol := []string{"test-writer", "writer", "reviewer"}[r.Intn(3)]
		col := ""
		if piIndice(piColDelRol[rol]) <= piIndice(tarjeta.Column) && r.Intn(2) == 0 {
			col = piColDelRol[rol]
		}
		pedido := map[string]string{"writer": acPedWriter, "test-writer": acPedTW}[rol]
		s.cerrar(r, PilotDecision{Role: rol, Provider: []string{"claude", "gemini"}[r.Intn(2)], Pedido: pedido}, col, false)
		traza = append(traza, "a mano: "+rol)
	}
	lanz, cierres := 0, 0
	for vuelta := 0; vuelta < 200; vuelta++ {
		e := s.estado(tarjeta, pol)
		copia := s.estado(tarjeta, pol)
		d := Piloto(e)
		if !reflect.DeepEqual(e, copia) {
			return lanz, cota, traza, fmt.Errorf("CA-642: Piloto cambio el Estado que recibio")
		}
		if otra := Piloto(copia); !reflect.DeepEqual(d, otra) {
			return lanz, cota, traza, fmt.Errorf("CA-642: la misma decision para el mismo Estado:\n %+v\n %+v", d, otra)
		}
		if err := check(e, d); err != nil {
			return lanz, cota, traza, fmt.Errorf("[%s, vuelta %d] %v", tarjeta.Column, vuelta, err)
		}
		switch d.Paso {
		case PasoDetenerse:
			traza = append(traza, "se detiene: "+d.Why)
			return lanz, cota, traza, nil
		case PasoCerrarHallazgos:
			cierres++
			traza = append(traza, "cierra "+strings.Join(d.Hallazgos, ","))
			if cierres > 10 {
				return lanz, cota, traza, fmt.Errorf("cerrar-hallazgos sin fin")
			}
			var quedan []finding.Item
			for _, b := range s.bloq {
				if piContiene(d.Hallazgos, b.ID) {
					delete(s.citas, b.ID)
					continue
				}
				quedan = append(quedan, b)
			}
			s.bloq = quedan
			if len(s.bloq) > 0 || r.Intn(3) == 0 {
				traza = append(traza, "verify sigue rojo: se detiene")
				return lanz, cota, traza, nil
			}
			s.veredicto = s.ver(nil)
		case PasoLanzar:
			lanz++
			traza = append(traza, fmt.Sprintf("%s/%s(%s r%d i%d)", d.Role, d.Provider, d.Regla, d.Ronda, d.Intento))
			if lanz > cota {
				return lanz, cota, traza, fmt.Errorf("CA-660: %d lanzamientos pasan la cota %d", lanz, cota)
			}
			s.cerrar(r, d, tarjeta.Column, true)
			ci += []int{0, 0, 1, 1, 2}[r.Intn(5)]
			if ci >= len(aeSimCols) {
				ci = len(aeSimCols) - 1
			}
			tarjeta = carta()
		default:
			return lanz, cota, traza, fmt.Errorf("paso desconocido %q", d.Paso)
		}
	}
	return lanz, cota, traza, fmt.Errorf("CA-660: el piloto no termina en 200 vueltas")
}

// ===========================================================================
// Muestras: los Estados de los fixtures de este archivo (CA-642, CA-658)
// ===========================================================================

type aeMuestra struct {
	nombre string
	e      Estado
}

// aeMuestras arma de nuevo, en cada llamada, un Estado de cada familia de
// fixtures: cadena, rojo esperado, reintento, corte, ronda, cierre, ventana.
func aeMuestras(t *testing.T) []aeMuestra {
	t.Helper()
	r1 := manifest.AutonomiaPolicy{ReintentosAnteRojo: 1}
	r2 := manifest.AutonomiaPolicy{RondasDeHallazgos: 2}
	vW := aeVer("v-w-rojo", aeGatesWriter(map[string]string{"build": verdict.StatusFail, "test": verdict.StatusFail}))
	out := []aeMuestra{
		{"cadena tw -> writer", aeEstado(piEv("writer", piConTope, piNada), aePol0, aeSobres(aeS{rol: "test-writer", prov: "claude", col: ColTestWriter}))},
		{"firma", aeEstado(piEv("test-writer", piConTope, piNada), aePol0, nil)},
		{"rojo esperado", aeTrasRojoTW("verify", "claude", aeRojoTest("v-tw-rojo"))},
		{"reintento", aeTrasRojoWriter(r1, piConTope, aeS{rol: "writer", prov: "gemini", col: ColWriter, stage: "verify", status: aeNo, v: vW})},
		{"rojo final", aeTrasRojoWriter(r1, piConTope, aeS{rol: "writer", prov: "gemini", col: ColWriter, stage: "verify", status: aeNo, v: vW},
			aeS{rol: "writer", prov: "gemini", col: ColWriter, stage: "verify", status: aeNo, v: vW})},
		{"relevo con resume", aeTrasCorte(aePol0, "writer", piConTope, aeS{rol: "test-writer", prov: "claude", col: ColTestWriter}, aeCortado("writer", "claude", "ses"))},
		{"relevo repetido", aeTrasCorte(aePol0, "test-writer", piConTope, aeCortado("test-writer", "claude", "ses"))},
		{"ronda: refutador", aeRonda(r2, []finding.Item{aeF1, aeF2}, aeBase()...)},
		{"ronda: test-writer", aeRonda(r2, []finding.Item{aeF1}, aeMas(aeBase(), aeRef1([]string{"f1", "f2"}, 2))...)},
		{"ventana", aeSoloCodex(manifest.AutonomiaPolicy{VentanaMax: aeEnt(80)}, map[string]runcmd.VentanaRun{"codex": {Nombre: "5h", Fin: 90, ReiniciaEn: aeReinicio}})},
		{"ventana sin dato", aeSoloCodex(manifest.AutonomiaPolicy{VentanaMax: aeEnt(80)}, nil)},
	}
	conCitas := aeRonda(r2, []finding.Item{aeF1}, aeMas(aeBase(), aeRef1([]string{"f1", "f2"}, 2), aeTW1("verify", aeNo, aeRojoTest("v-tw1"), "pkg/f1_test.go"))...)
	conCitas.Citas = map[string][]string{"f1": {"pkg/f1_test.go"}}
	out = append(out, aeMuestra{"ronda: writer", conCitas})
	cierre := aeRonda(r2, []finding.Item{aeF1, aeF2}, aeMas(aeAntesDelArreglo(), aeW1("verify", aeNo, aeRojoHallazgos("v-w1")))...)
	cierre.Veredicto = aeRojoHallazgos("v-w1")
	cierre.Citas = map[string][]string{"f1": {"pkg/a_test.go", "pkg/f1_test.go"}}
	out = append(out, aeMuestra{"ronda: cierre", cierre})
	for _, tj := range piTarjetas(t) {
		out = append(out, aeMuestra{"piloto " + tj.nombre, piEstado(tj.c, piSobre("writer", "claude", aeOK, ColWriter))})
	}
	return out
}
