// Tests adversariales del spec .hoom/specs/tablero-tu-siguiente-paso.md en
// internal/boardcmd: CA-703, el rojo esperado del test-writer en la tarjeta
// (boardcmd.Red.Esperado, "esperado" en el JSON de hoom board --json).
//
// Derive se prueba con Evidence literales y los MISMOS sobres que prueban el
// rojo esperado del piloto (aeS de autonomia_entre_firmas_test.go, con los
// veredictos aeRojoTest y aeGates de CA-643): Esperado es true solo cuando el
// sobre cerrado mas nuevo de la tarjeta (por StartedAt y despues por ID) es el
// del test-writer, cerrado en verify o check, con un veredicto rojo cuyos
// gates del spec (spec_lint, spec_trace, spec_approved) estan todos en pass, y
// false en cualquier otro rojo. Gather y Build se prueban en disco: el
// veredicto que cuenta es el que emitio ese sobre (su VerdictID), no el mas
// nuevo de la tarjeta.
package boardcmd

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// tspEstado convierte los sobres del piloto (aeSobres: del mas viejo al mas
// nuevo) en los registros de la Evidence (del mas nuevo al mas viejo), que es
// el orden en que Gather los entrega.
func tspEstados(sobres []Sobre) []EnvelopeState {
	var out []EnvelopeState
	for i := len(sobres) - 1; i >= 0; i-- {
		out = append(out, EnvelopeState{Record: sobres[i].Record})
	}
	return out
}

// tspEv es la tarjeta del piloto en Writer (piEv), con los sobres xs (del mas
// viejo al mas nuevo), el veredicto de la tarjeta v y VeredictoDelSobre como
// lo deja Gather: el que emitio el sobre cerrado mas nuevo.
func tspEv(v *verdict.Verdict, xs ...aeS) Evidence {
	ev := piEv("writer", piConTope, piNada)
	ev.Verdict = v
	sobres := aeSobres(xs...)
	ev.Envelopes = tspEstados(sobres)
	ev.VeredictoDelSobre = nil
	if n := len(sobres); n > 0 {
		ev.VeredictoDelSobre = sobres[n-1].Veredicto
	}
	return ev
}

// tspTW es el sobre del test-writer que cerro en stage con v (el de
// aeTrasRojoTW de CA-643).
func tspTW(stage string, v *verdict.Verdict) aeS {
	return aeS{rol: "test-writer", prov: "claude", col: ColTestWriter, stage: stage, status: aeNo, v: v,
		trasp: []string{"precios_test.go"}}
}

// tspRojo deriva la tarjeta y exige que tenga rojo: Esperado vive en el rojo.
func tspRojo(t *testing.T, ca, nombre string, ev Evidence) *Red {
	t.Helper()
	c := Derive(ev)
	if c.Red == nil {
		t.Fatalf("%s: [%s] fixture: la tarjeta tiene que quedar en rojo (columna %q, missing=%q)", ca, nombre, c.Column, c.Missing)
	}
	return c.Red
}

// tspEsperado pide Esperado == quiero.
func tspEsperado(t *testing.T, ca, nombre string, ev Evidence, quiero bool) {
	t.Helper()
	r := tspRojo(t, ca, nombre, ev)
	if r.Esperado != quiero {
		t.Fatalf("%s: [%s] red.esperado = %v, se esperaba %v (red=%+v)", ca, nombre, r.Esperado, quiero, *r)
	}
}

// CA-703: el rojo esperado del test-writer (cerro en verify o check, rojo
// solo por el gate test, con los gates del spec en pass) es Esperado, venga el
// rojo de la tarjeta del veredicto o del sobre.
func TestCA703_RojoEsperadoDelTestWriterEsEsperado(t *testing.T) {
	for _, stage := range []string{"verify", "check"} {
		v := aeRojoTest("v-tw-rojo")
		// el rojo de la tarjeta sale del veredicto (el veredicto es posterior al sobre)
		tspEsperado(t, "CA-703", "rojo del veredicto, sobre en "+stage, tspEv(v, tspTW(stage, v)), true)

		// el rojo sale del sobre: la tarjeta no tiene veredicto completo
		tspEsperado(t, "CA-703", "rojo del sobre, sin veredicto de la tarjeta, sobre en "+stage, tspEv(nil, tspTW(stage, v)), true)
	}
	// otro provider: la regla no mira quien lo corrio
	v := aeRojoTest("v-tw-gemini")
	o := tspTW("verify", v)
	o.prov = "gemini"
	tspEsperado(t, "CA-703", "test-writer de gemini", tspEv(v, o), true)
}

// CA-703: el rojo esperado sigue siendo esperado cuando antes hubo otros
// sobres cerrados (un writer en rojo, un test-writer cortado): cuenta el mas
// nuevo.
func TestCA703_CuentaElSobreMasNuevo(t *testing.T) {
	v := aeRojoTest("v-tw-rojo")
	viejoW := aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "verify", status: aeNo, v: aeRojoTest("v-w-viejo")}
	viejoTW := tspTW("scope", nil)
	tspEsperado(t, "CA-703", "writer rojo y despues el rojo esperado", tspEv(v, viejoW, tspTW("verify", v)), true)
	tspEsperado(t, "CA-703", "test-writer cortado en scope y despues el rojo esperado", tspEv(v, viejoTW, tspTW("check", v)), true)
}

// CA-703: cualquier otro rojo no es esperado: los gates del spec sin pass,
// un sobre cortado antes de verify, otro rol, el veredicto del sobre que no
// se pudo leer, un sobre mas nuevo que no es el rojo esperado, y un rojo sin
// sobres.
func TestCA703_CualquierOtroRojoNoEsEsperado(t *testing.T) {
	gTest := map[string]string{"test": verdict.StatusFail}
	con := func(extra map[string]string) map[string]string {
		m := map[string]string{}
		for k, v := range gTest {
			m[k] = v
		}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	casos := []struct {
		nombre string
		ev     Evidence
	}{}
	add := func(nombre string, ev Evidence) {
		casos = append(casos, struct {
			nombre string
			ev     Evidence
		}{nombre, ev})
	}
	for _, c := range []struct {
		nombre, stage string
		gates         map[string]string
	}{
		{"spec_trace en fail", "verify", con(map[string]string{"spec_trace": verdict.StatusFail})},
		{"spec_trace en error", "verify", con(map[string]string{"spec_trace": verdict.StatusError})},
		{"spec_lint en fail", "check", con(map[string]string{"spec_lint": verdict.StatusFail})},
		{"spec_approved en fail", "verify", con(map[string]string{"spec_approved": verdict.StatusFail})},
	} {
		v := aeVer("v-"+c.nombre, aeGates(c.gates))
		add(c.nombre, tspEv(v, tspTW(c.stage, v)))
	}
	// cortado antes de verify: sin veredicto del sobre
	add("test-writer cortado en scope", tspEv(nil, tspTW("scope", nil)))
	add("test-writer cortado en run", tspEv(nil, tspTW("run", nil)))
	add("test-writer cortado en scope, con veredicto rojo de la tarjeta", tspEv(aeRojoTest("v-tarjeta"), tspTW("scope", nil)))

	// otro rol con el mismo veredicto
	for _, rol := range []string{"writer", "reviewer"} {
		v := aeRojoTest("v-" + rol)
		o := tspTW("verify", v)
		o.rol, o.col = rol, ColWriter
		add(rol+" cerrado en verify con rojo de test", tspEv(v, o))
	}

	// el sobre dice su veredicto pero Gather no lo pudo leer
	{
		v := aeRojoTest("v-ilegible")
		ev := tspEv(v, tspTW("verify", v))
		ev.VeredictoDelSobre = nil
		add("veredicto del sobre ilegible", ev)
	}

	// un sobre mas nuevo que no es el rojo esperado tapa al viejo
	{
		vTW := aeRojoTest("v-tw-viejo")
		vW := aeRojoTest("v-w-nuevo")
		w := aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "verify", status: aeNo, v: vW}
		add("rojo esperado viejo y un writer en rojo mas nuevo", tspEv(vW, tspTW("verify", vTW), w))
		add("rojo esperado viejo y un test-writer cortado mas nuevo", tspEv(vTW, tspTW("verify", vTW), tspTW("run", nil)))
	}

	// un rojo del veredicto de la tarjeta sin ningun sobre
	add("rojo del veredicto sin sobres", tspEv(aeRojoTest("v-sin-sobre")))
	add("rojo de hallazgos sin sobres", tspEv(aeRojoHallazgos("v-hallazgos")))

	for _, c := range casos {
		tspEsperado(t, "CA-703", c.nombre, c.ev, false)
	}
}

// CA-703: el sobre mas nuevo es el del orden del piloto (StartedAt y despues
// ID), no el primero de la lista: una Evidence con los sobres en otro orden
// da el mismo Esperado.
func TestCA703_ElOrdenEsElDelPiloto(t *testing.T) {
	v := aeRojoTest("v-tw")
	vW := aeRojoTest("v-w")
	tw := tspTW("verify", v).sobre(2)                                                                      // empieza a los 80 min
	w := aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "verify", status: aeNo, v: vW}.sobre(1) // a los 70

	arma := func(primero, segundo Sobre, vs *verdict.Verdict) Evidence {
		ev := piEv("writer", piConTope, piNada)
		ev.Verdict = aeRojoTest("v-tarjeta")
		ev.Envelopes = []EnvelopeState{{Record: primero.Record}, {Record: segundo.Record}}
		ev.VeredictoDelSobre = vs
		return ev
	}
	// el test-writer es el mas nuevo por StartedAt aunque venga segundo
	tspEsperado(t, "CA-703", "test-writer mas nuevo, segundo en la lista", arma(w, tw, v), true)
	tspEsperado(t, "CA-703", "test-writer mas nuevo, primero en la lista", arma(tw, w, v), true)

	// el writer es el mas nuevo por StartedAt aunque venga primero o segundo
	tw2 := tw
	tw2.Record.StartedAt = bdT0.Add(30 * time.Minute)
	tw2.Record.UpdatedAt, tw2.Record.EndedAt = bdT0.Add(35*time.Minute), bdT0.Add(35*time.Minute)
	tspEsperado(t, "CA-703", "writer mas nuevo, segundo en la lista", arma(tw2, w, vW), false)
	tspEsperado(t, "CA-703", "writer mas nuevo, primero en la lista", arma(w, tw2, vW), false)

	// empate de StartedAt: decide el ID (el mayor es el mas nuevo)
	mismo := bdT0.Add(100 * time.Minute)
	a, b := tw, w
	a.Record.StartedAt, b.Record.StartedAt = mismo, mismo
	a.Record.ID, b.Record.ID = "20260922T174000_zz0002", "20260922T174000_aa0001"
	tspEsperado(t, "CA-703", "empate: el test-writer tiene el ID mayor", arma(b, a, v), true)
	tspEsperado(t, "CA-703", "empate: el test-writer tiene el ID mayor, al reves", arma(a, b, v), true)
	a.Record.ID, b.Record.ID = "20260922T174000_aa0001", "20260922T174000_zz0002"
	tspEsperado(t, "CA-703", "empate: el writer tiene el ID mayor", arma(a, b, vW), false)
	tspEsperado(t, "CA-703", "empate: el writer tiene el ID mayor, al reves", arma(b, a, vW), false)
}

// CA-703: un sobre abierto no es un sobre cerrado: un trabajo nuevo en curso
// (vivo o cortado) no cambia cual es el sobre cerrado mas nuevo. El spec no
// fija si una tarjeta con running muestra su rojo: si lo muestra, sigue siendo
// el esperado.
func TestCA703_UnSobreAbiertoNoCuenta(t *testing.T) {
	v := aeRojoTest("v-tw-rojo")
	for _, vivo := range []bool{true, false} {
		ev := tspEv(v, tspTW("verify", v))
		abierto := acSobreVivo("writer", "claude", "run", 3, 5)
		abierto.Alive = vivo
		abierto.Record.StartedAt = bdT0.Add(3 * time.Hour)
		abierto.Record.UpdatedAt = bdT0.Add(3 * time.Hour)
		abierto.Record.EndedAt = time.Time{}
		abierto.Record.VerdictID, abierto.Record.Verdict = "", ""
		ev.Envelopes = append([]EnvelopeState{abierto}, ev.Envelopes...)
		c := Derive(ev)
		if c.Red != nil && !c.Red.Esperado {
			t.Fatalf("CA-703: [sobre abierto vivo=%v] el sobre cerrado mas nuevo sigue siendo el rojo esperado: red=%+v", vivo, *c.Red)
		}
	}
}

// CA-703: "esperado" esta en el JSON del rojo, en true y en false.
func TestCA703_EsperadoEnElJSON(t *testing.T) {
	v := aeRojoTest("v-tw-rojo")
	vMal := aeVer("v-trace", aeGates(map[string]string{"test": verdict.StatusFail, "spec_trace": verdict.StatusFail}))
	for _, c := range []struct {
		nombre string
		ev     Evidence
		quiero bool
	}{
		{"rojo esperado", tspEv(v, tspTW("verify", v)), true},
		{"rojo no esperado", tspEv(vMal, tspTW("verify", vMal)), false},
	} {
		card := Derive(c.ev)
		if card.Red == nil {
			t.Fatalf("CA-703: [%s] fixture: la tarjeta tiene que quedar en rojo", c.nombre)
		}
		raw, err := json.Marshal(card)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Red map[string]any `json:"red"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		got, ok := m.Red["esperado"]
		if !ok {
			t.Fatalf("CA-703: [%s] el rojo del JSON tiene la clave \"esperado\": %s", c.nombre, raw)
		}
		if b, es := got.(bool); !es || b != c.quiero {
			t.Fatalf("CA-703: [%s] \"esperado\" = %#v, se esperaba %v", c.nombre, got, c.quiero)
		}
	}
}

// ---------------------------------------------------------------------------
// En disco: Gather lee VeredictoDelSobre del VerdictID del sobre cerrado mas
// nuevo, del arbol de la evidencia y despues de la raiz, parciales incluidos.
// ---------------------------------------------------------------------------

// tspSobreDisco escribe en la raiz un sobre cerrado del rol para la tarjeta.
func tspSobreDisco(t *testing.T, root, dir, id, rol, stage, verdictID string, inicio time.Time) {
	t.Helper()
	fin := inicio.Add(2 * time.Minute)
	rec := envelope.Record{ID: id, RunID: "run-" + id, Role: rol, Provider: "claude", Task: bdSlug, Dir: dir,
		Spec: bdSpec, Stage: stage, Step: 5, Steps: 6, Status: envelope.StatusNotDeliverable, ExitCode: 1,
		StartedAt: inicio, UpdatedAt: fin, EndedAt: fin, Columna: ColTestWriter}
	if verdictID != "" {
		rec.VerdictID, rec.Verdict = verdictID, "red"
	}
	bdSobreDisco(t, root, rec)
}

func tspGatesRojoTest() []verdict.GateResult {
	gs := bdGatesVerdes()
	for i := range gs {
		if gs[i].Name == "test" {
			gs[i].Status = verdict.StatusFail
		}
	}
	return gs
}

// CA-703: el veredicto del sobre es el que emitio ese sobre (su VerdictID),
// aunque sea parcial y aunque la tarjeta tenga un veredicto mas nuevo.
func TestCA703_GatherLeeElVeredictoQueEmitioElSobre(t *testing.T) {
	root := bdRepo(t, "")
	wt := bdWorktree(t, root, bdSlug)
	now := time.Now().UTC().Truncate(time.Second)
	vSobre := bdVeredictoDisco(t, wt, bdSpec, now.Add(-20*time.Minute), true, tspGatesRojoTest())
	vNuevo := bdVeredictoDisco(t, wt, bdSpec, now.Add(-5*time.Minute), false, bdGatesVerdes())
	if vSobre.ID == vNuevo.ID {
		t.Fatal("fixture: dos veredictos distintos")
	}
	tspSobreDisco(t, root, wt, "20260922T100000_tsp001", "test-writer", "verify", vSobre.ID, now.Add(-30*time.Minute))

	ev := Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.VeredictoDelSobre == nil {
		t.Fatalf("CA-703: Gather lee el veredicto %s que emitio el sobre (parcial incluido): VeredictoDelSobre = nil", vSobre.ID)
	}
	if ev.VeredictoDelSobre.ID != vSobre.ID {
		t.Fatalf("CA-703: VeredictoDelSobre es el del sobre (%s), no el mas nuevo de la tarjeta: fue %s", vSobre.ID, ev.VeredictoDelSobre.ID)
	}
	if !ev.VeredictoDelSobre.Partial {
		t.Fatalf("CA-703: VeredictoDelSobre trae el veredicto tal cual (parcial): %+v", ev.VeredictoDelSobre)
	}
	if len(ev.VeredictoDelSobre.Gates) != len(tspGatesRojoTest()) {
		t.Fatalf("CA-703: VeredictoDelSobre trae los gates del veredicto: %+v", ev.VeredictoDelSobre.Gates)
	}
}

// CA-703: si el arbol de la evidencia no lo tiene, el veredicto del sobre se
// lee de la raiz.
func TestCA703_GatherLeeElVeredictoDeLaRaiz(t *testing.T) {
	root := bdRepo(t, "")
	wt := bdWorktree(t, root, bdSlug)
	now := time.Now().UTC().Truncate(time.Second)
	vRaiz := bdVeredictoDisco(t, root, bdSpec, now.Add(-20*time.Minute), false, tspGatesRojoTest())
	tspSobreDisco(t, root, wt, "20260922T100000_tsp002", "test-writer", "check", vRaiz.ID, now.Add(-30*time.Minute))

	ev := Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.Source != SourceWorktree {
		t.Fatalf("fixture: la tarjeta se lee de su worktree, fue %q", ev.Source)
	}
	if ev.VeredictoDelSobre == nil || ev.VeredictoDelSobre.ID != vRaiz.ID {
		t.Fatalf("CA-703: el veredicto del sobre se lee de la raiz cuando E no lo tiene: %+v", ev.VeredictoDelSobre)
	}
}

// CA-703: el sobre cerrado mas nuevo es el de StartedAt mayor, no el de ID
// mayor ni el del archivo que se lee primero; un sobre abierto no cuenta.
func TestCA703_GatherUsaElSobreCerradoMasNuevo(t *testing.T) {
	root := bdRepo(t, "")
	wt := bdWorktree(t, root, bdSlug)
	now := time.Now().UTC().Truncate(time.Second)
	vViejo := bdVeredictoDisco(t, wt, bdSpec, now.Add(-50*time.Minute), false, tspGatesRojoTest())
	vNuevo := bdVeredictoDisco(t, wt, bdSpec, now.Add(-40*time.Minute), true, tspGatesRojoTest())
	// el ID mayor empezo antes
	tspSobreDisco(t, root, wt, "20260922T230000_zzzzzz", "writer", "verify", vViejo.ID, now.Add(-60*time.Minute))
	tspSobreDisco(t, root, wt, "20260922T100000_aaaaaa", "test-writer", "verify", vNuevo.ID, now.Add(-45*time.Minute))
	// un sobre abierto mas nuevo, sin veredicto
	bdSobreDisco(t, root, envelope.Record{ID: "20260922T235900_abiert", RunID: "", Role: "writer", Provider: "claude",
		Task: bdSlug, Dir: wt, Spec: bdSpec, Stage: "run", Step: 3, Steps: 6, Status: envelope.StatusRunning,
		StartedAt: now.Add(-10 * time.Minute), UpdatedAt: now.Add(-10 * time.Minute)})

	ev := Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.VeredictoDelSobre == nil || ev.VeredictoDelSobre.ID != vNuevo.ID {
		t.Fatalf("CA-703: VeredictoDelSobre es el del sobre cerrado de StartedAt mayor (%s): %+v", vNuevo.ID, ev.VeredictoDelSobre)
	}
}

// CA-703: sin veredicto que leer, VeredictoDelSobre es nil: el VerdictID no
// esta en disco, el sobre cerrado mas nuevo no emitio ninguno (aunque uno mas
// viejo si), o la tarjeta no tiene sobres cerrados.
func TestCA703_GatherSinVeredictoDelSobreEsNil(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	root := bdRepo(t, "")
	wt := bdWorktree(t, root, bdSlug)
	bdVeredictoDisco(t, wt, bdSpec, now.Add(-20*time.Minute), false, tspGatesRojoTest())
	tspSobreDisco(t, root, wt, "20260922T100000_tsp010", "test-writer", "verify", "2026-09-22T10-00-00Z_noexiste", now.Add(-30*time.Minute))
	if ev := Gather(root, "main", "high", bdItem(bdSlug), now); ev.VeredictoDelSobre != nil {
		t.Fatalf("CA-703: un VerdictID que no esta en disco da nil, fue %s", ev.VeredictoDelSobre.ID)
	}

	root = bdRepo(t, "")
	wt = bdWorktree(t, root, bdSlug)
	v := bdVeredictoDisco(t, wt, bdSpec, now.Add(-40*time.Minute), false, tspGatesRojoTest())
	tspSobreDisco(t, root, wt, "20260922T100000_tsp011", "test-writer", "verify", v.ID, now.Add(-50*time.Minute))
	tspSobreDisco(t, root, wt, "20260922T110000_tsp012", "test-writer", "scope", "", now.Add(-30*time.Minute))
	if ev := Gather(root, "main", "high", bdItem(bdSlug), now); ev.VeredictoDelSobre != nil {
		t.Fatalf("CA-703: el sobre cerrado mas nuevo no emitio veredicto: nil, fue %s (el del viejo)", ev.VeredictoDelSobre.ID)
	}

	root = bdRepo(t, "")
	wt = bdWorktree(t, root, bdSlug)
	bdVeredictoDisco(t, wt, bdSpec, now.Add(-40*time.Minute), false, tspGatesRojoTest())
	if ev := Gather(root, "main", "high", bdItem(bdSlug), now); ev.VeredictoDelSobre != nil {
		t.Fatalf("CA-703: sin sobres cerrados VeredictoDelSobre es nil, fue %s", ev.VeredictoDelSobre.ID)
	}
}

// CA-703: hoom board (Build) lleva "esperado" en el rojo de la tarjeta: true
// con el rojo esperado del test-writer, false con un rojo cuyo spec_trace
// fallo.
func TestCA703_BuildLlevaEsperadoEnElJSON(t *testing.T) {
	for _, c := range []struct {
		nombre string
		gates  []verdict.GateResult
		quiero bool
	}{
		{"rojo esperado", tspGatesRojoTest(), true},
		{"spec_trace en fail", func() []verdict.GateResult {
			gs := tspGatesRojoTest()
			for i := range gs {
				if gs[i].Name == "spec_trace" {
					gs[i].Status = verdict.StatusFail
				}
			}
			return gs
		}(), false},
	} {
		root := bdRepo(t, "")
		bdItemArchivo(t, root, bdSlug, "Precios", "")
		wt := bdWorktree(t, root, bdSlug)
		now := time.Now().UTC().Truncate(time.Second)
		v := bdVeredictoDisco(t, wt, bdSpec, now.Add(-20*time.Minute), false, c.gates)
		tspSobreDisco(t, root, wt, "20260922T100000_tsp020", "test-writer", "verify", v.ID, now.Add(-30*time.Minute))

		b, err := Build(root, "main", "high", now)
		if err != nil {
			t.Fatalf("CA-703: [%s] Build: %v", c.nombre, err)
		}
		card := bdCard(t, b, bdSlug)
		if card.Red == nil {
			t.Fatalf("CA-703: [%s] fixture: la tarjeta queda en rojo (columna %q)", c.nombre, card.Column)
		}
		if card.Red.Esperado != c.quiero {
			t.Fatalf("CA-703: [%s] red.esperado = %v, se esperaba %v (red=%+v)", c.nombre, card.Red.Esperado, c.quiero, *card.Red)
		}
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatal(err)
		}
		var tablero struct {
			Columns []struct {
				Cards []struct {
					Slug string         `json:"slug"`
					Red  map[string]any `json:"red"`
				} `json:"cards"`
			} `json:"columns"`
		}
		if err := json.Unmarshal(raw, &tablero); err != nil {
			t.Fatal(err)
		}
		vista := false
		for _, col := range tablero.Columns {
			for _, cd := range col.Cards {
				if cd.Slug != bdSlug {
					continue
				}
				vista = true
				if got, ok := cd.Red["esperado"]; !ok || got != c.quiero {
					t.Fatalf("CA-703: [%s] el JSON de hoom board dice red.esperado = %#v (presente=%v), se esperaba %v", c.nombre, got, ok, c.quiero)
				}
			}
		}
		if !vista {
			t.Fatalf("CA-703: [%s] la tarjeta esta en el JSON del tablero", c.nombre)
		}
	}
}
