// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-306..CA-309): el modo normal no esconde un rojo, lo dice con otras
// palabras. plain y red.plain salen del binario con una tabla cerrada, sin
// vocabulario de git, sin rutas de .hoom/ y sin comandos de hoom, y
// needs_decision es lo que filtra "Necesitan tu decision".
package boardcmd

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// tbProhibidas son las palabras que el modo normal nunca dice (enteras, sin
// distinguir mayusculas).
var tbProhibidas = regexp.MustCompile(`(?i)\b(git|commit|worktree|merge|rama|branch|diff|head|huella)\b`)

// tbNormal exige una frase del modo normal: no vacia, sin palabras de git,
// sin rutas de .hoom/ y sin comandos de hoom.
func tbNormal(t *testing.T, ca, que, s string) {
	t.Helper()
	if strings.TrimSpace(s) == "" {
		t.Fatalf("%s: %s nunca es vacio", ca, que)
	}
	if w := tbProhibidas.FindString(s); w != "" {
		t.Fatalf("%s: %s no dice %q: %q", ca, que, w, s)
	}
	for _, sub := range []string{".hoom/", "hoom "} {
		if strings.Contains(s, sub) {
			t.Fatalf("%s: %s no contiene %q: %q", ca, que, sub, s)
		}
	}
}

// Mensajes reales de taskcmd.Ready: traen rutas, comandos y vocabulario de
// git, justo lo que plain no puede repetir.
const (
	tbMsgSinGuardar = "la tarea \"precios\" tiene cambios sin commitear (incluidos posibles veredictos).\n  Accion: commitea todo dentro de /tmp/x/.hoom/worktrees/precios y repite 'hoom task done precios'"
	tbMsgRojo       = `el ultimo veredicto de "precios" es ROJO (2026-09-22T16-00-00Z_bbbb2222). Accion: corrige y re-ejecuta 'hoom verify' en el worktree`
	tbMsgHuella     = "el arbol de \"precios\" cambio despues del ultimo veredicto verde (huella aaaa vs bbbb).\n  Accion: re-ejecuta 'hoom verify' dentro del worktree y commitea"
	tbMsgSinVer     = `la tarea "precios" no tiene veredictos. Accion: ejecuta 'hoom verify' dentro del worktree`
	tbMsgParciales  = `la tarea "precios" solo tiene veredictos PARCIALES (--gate), que no son referencia. Accion: ejecuta 'hoom verify' completo dentro del worktree`
	tbMsgSinTarea   = `la tarea "precios" no existe (mira 'hoom task list')`
)

// tbSin quita un gate del veredicto verde de la tarjeta.
func tbSin(ev Evidence, nombre string) Evidence {
	var gs []verdict.GateResult
	for _, g := range bdGatesVerdes() {
		if g.Name != nombre {
			gs = append(gs, g)
		}
	}
	ev.Verdict = bdVeredicto("v-sin-"+nombre, bdT0.Add(30*time.Minute), "huella-1", 50, 10, gs)
	ev.GreenVerdicts = []string{ev.Verdict.ID}
	return ev
}

// tbConGate deja un gate del verde de la tarjeta presente pero sin pasar (no
// requerido, asi el veredicto sigue verde).
func tbConGate(ev Evidence, nombre string) Evidence {
	gs := bdGatesVerdes()
	for i := range gs {
		if gs[i].Name == nombre {
			gs[i].Required, gs[i].Status = false, verdict.StatusFail
		}
	}
	ev.Verdict = bdVeredicto("v-"+nombre+"-fail", bdT0.Add(30*time.Minute), "huella-1", 50, 10, gs)
	ev.GreenVerdicts = []string{ev.Verdict.ID}
	return ev
}

func tbListo(ev Evidence, kind, msg string) Evidence {
	ev.ReadyErr, ev.ReadyKind = msg, kind
	return ev
}

type tbCaso struct {
	nombre string
	ev     Evidence
	col    string
	plain  string
}

// tbCasos recorre TODAS las ramas de column (derive.go): cada columna y, en
// Writer y Review, cada condicion.
func tbCasos() []tbCaso {
	cuando := bdT0.Add(48 * time.Hour)
	var cs []tbCaso
	add := func(nombre string, ev Evidence, col, plain string) {
		cs = append(cs, tbCaso{nombre, ev, col, plain})
	}

	// CA-279: hecho gana sobre todo
	ev := bdArbol(bdEv())
	ev.Item.HechoEn = &cuando
	ev.SpecExists = false
	add("hecho", ev, ColHecho, "terminada: su cierre quedo registrado")
	ev = bdEv()
	ev.Item.HechoEn = &cuando
	ev.Verdict = bdVeredicto("v-rojo", bdT0, "huella-1", 1, 1, []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
	add("hecho con rojo", ev, ColHecho, "terminada: su cierre quedo registrado")

	// CA-269: backlog, sin y con tarea
	ev = bdArbol(bdEv())
	ev.SpecExists = false
	add("backlog sin tarea", ev, ColBacklog, "falta el spec de la tarjeta")
	ev = bdEv()
	ev.SpecExists = false
	add("backlog con tarea", ev, ColBacklog, "falta el spec de la tarjeta")

	// CA-270: arquitecto, singular y plural
	ev = bdEv()
	ev.LintIssues = []string{`falta la seccion "riesgos"`}
	add("arquitecto 1", ev, ColArquitecto, "el spec no esta completo (1 problema de formato)")
	ev = bdArbol(bdEv())
	ev.LintIssues = []string{`falta la seccion "objetivo"`, `falta la seccion "riesgos"`,
		"no hay criterios de aceptacion identificados como CA-1, CA-2, ... (la trazabilidad spec->test los necesita)"}
	ev.Criteria = nil
	add("arquitecto 3", ev, ColArquitecto, "el spec no esta completo (3 problemas de formato)")

	// CA-271: tu aprobacion
	ev = bdEv()
	ev.Approval = approval.StatusNotApproved
	add("sin aprobacion", ev, ColTuAprobacion, "el spec espera tu aprobacion")
	ev = bdEv()
	ev.Approval = ""
	add("sin estado de aprobacion", ev, ColTuAprobacion, "el spec espera tu aprobacion")
	ev = bdEv()
	ev.Approval = approval.StatusInvalidated
	add("invalidada", ev, ColTuAprobacion, "el spec cambio despues de tu aprobacion: hay que aprobarlo de nuevo")

	// CA-272: test-writer (siempre "faltan")
	ev = bdEv()
	ev.Untraced = []string{"CA-2"}
	add("test-writer 1 de 3", ev, ColTestWriter, "faltan pruebas para 1 de 3 criterios")
	ev = bdEv()
	ev.Criteria = []string{"CA-1", "CA-2", "CA-3", "CA-5"}
	ev.Untraced = []string{"CA-3", "CA-5"}
	add("test-writer 2 de 4", ev, ColTestWriter, "faltan pruebas para 2 de 4 criterios")

	// CA-273: writer
	ev = bdEv()
	ev.Verdict, ev.GreenVerdicts = nil, nil
	add("writer sin veredicto", ev, ColWriter, "falta verificar el trabajo contra el spec")
	ev = bdEv()
	ev.Verdict = bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10, []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "test", Required: true, Status: verdict.StatusFail},
		{Name: "lint", Required: false, Status: verdict.StatusFail},
	})
	add("writer rojo 1", ev, ColWriter, "la verificacion dio rojo: fallo el gate test")
	ev = bdEv()
	ev.Verdict = bdVeredicto("v-rojo2", bdT0.Add(40*time.Minute), "huella-1", 50, 10, []verdict.GateResult{
		{Name: "test", Required: true, Status: verdict.StatusFail},
		{Name: "lint", Required: false, Status: verdict.StatusFail},
		{Name: "vet", Required: true, Status: verdict.StatusPass},
		{Name: "build", Required: true, Status: verdict.StatusError},
	})
	add("writer rojo 2", ev, ColWriter, "la verificacion dio rojo: fallaron los gates test, build")
	ev = bdEv()
	v := bdVeredicto("v-rojo0", bdT0.Add(40*time.Minute), "huella-1", 50, 10, []verdict.GateResult{
		{Name: "test", Required: false, Status: verdict.StatusFail},
	})
	v.Verdict = "red" // rojo sin ningun requerido en fail/error
	ev.Verdict = v
	add("writer rojo sin gates", ev, ColWriter, "la verificacion dio rojo")
	ev = bdEv()
	ev.Fingerprint = "huella-2"
	add("writer otra huella", ev, ColWriter, "el codigo cambio despues del ultimo verde")
	ev = bdEv()
	ev.Fingerprint = ""
	add("writer sin huella actual", ev, ColWriter, "el codigo cambio despues del ultimo verde")

	// CA-274..CA-277: review, cada condicion sola
	grande := func() Evidence {
		ev := bdEv()
		v := bdVeredicto("v-grande", bdT0.Add(30*time.Minute), "huella-1", 500, 112, bdGatesVerdes())
		ev.Verdict, ev.GreenVerdicts = v, []string{v.ID}
		return ev
	}
	add("review exigida", grande(), ColReview, "falta la revision de 4 lentes (612 lineas)")
	ev = bdEv()
	ev.Findings = []finding.Item{bdHallazgo("f-a", "high", bdSlug)}
	add("review 1 hallazgo", ev, ColReview, "hay 1 hallazgo que bloquea")
	ev = bdEv()
	ev.Findings = []finding.Item{bdHallazgo("f-a", "high", bdSlug), bdHallazgo("f-m", "medium", bdSlug), bdHallazgo("f-b", "high", bdSlug)}
	add("review 2 hallazgos", ev, ColReview, "hay 2 hallazgos que bloquean")
	add("review sin spec_approved", tbSin(bdEv(), "spec_approved"), ColReview,
		"hay que verificar de nuevo: el ultimo verde no incluye la aprobacion del spec")
	add("review spec_approved sin pasar", tbConGate(bdEv(), "spec_approved"), ColReview,
		"hay que verificar de nuevo: el ultimo verde no incluye la aprobacion del spec")
	add("review sin findings_open", tbSin(bdEv(), "findings_open"), ColReview,
		"el proyecto no declara que hallazgos bloquean")
	add("review findings_open sin pasar", tbConGate(bdEv(), "findings_open"), ColReview,
		"hay que verificar de nuevo: el control de hallazgos no paso")
	add("review sin tarea", bdArbol(bdEv()), ColReview, "falta el espacio de trabajo de la tarjeta")
	add("review ready sin-tarea", tbListo(bdEv(), taskcmd.ReadySinTarea, tbMsgSinTarea), ColReview,
		"falta el espacio de trabajo de la tarjeta")
	add("review ready sin-guardar", tbListo(bdEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar), ColReview,
		"hay cambios sin guardar en el espacio de trabajo")
	add("review ready sin-veredicto", tbListo(bdEv(), taskcmd.ReadySinVeredicto, tbMsgSinVer), ColReview,
		"falta verificar el espacio de trabajo completo")
	add("review ready solo-parciales", tbListo(bdEv(), taskcmd.ReadySoloParciales, tbMsgParciales), ColReview,
		"falta verificar el espacio de trabajo completo")
	add("review ready rojo", tbListo(bdEv(), taskcmd.ReadyRojo, tbMsgRojo), ColReview,
		"la ultima verificacion del espacio de trabajo dio rojo")
	add("review ready huella", tbListo(bdEv(), taskcmd.ReadyHuella, tbMsgHuella), ColReview,
		"el espacio de trabajo cambio despues del ultimo verde")
	add("review ready sin kind", tbListo(bdEv(), "", "algo impide cerrar la tarea en el worktree"), ColReview,
		"el espacio de trabajo no esta listo para cerrar")

	// CA-274..CA-277: varias condiciones: la primera y cuantas mas faltan
	ev = bdArbol(bdEv())
	ev.Findings = []finding.Item{bdHallazgo("f-a", "high", bdSlug)}
	add("review 2 condiciones", ev, ColReview, "hay 1 hallazgo que bloquea (y 1 pendiente mas)")
	ev = bdArbol(bdEv())
	var gs []verdict.GateResult
	for _, g := range bdGatesVerdes() {
		if g.Name != "findings_open" && g.Name != "spec_approved" {
			gs = append(gs, g)
		}
	}
	ev.Verdict = bdVeredicto("v-todo", bdT0.Add(30*time.Minute), "huella-1", 500, 112, gs)
	ev.GreenVerdicts = []string{"v-todo"}
	ev.Findings = []finding.Item{bdHallazgo("f1", "high", bdSlug), bdHallazgo("f2", "high", bdSlug)}
	add("review 5 condiciones", ev, ColReview, "falta la revision de 4 lentes (612 lineas) (y 4 pendientes mas)")
	ev = tbListo(tbSin(bdEv(), "spec_approved"), taskcmd.ReadySinGuardar, tbMsgSinGuardar)
	add("review 2 condiciones con ready", ev, ColReview,
		"hay que verificar de nuevo: el ultimo verde no incluye la aprobacion del spec (y 1 pendiente mas)")

	// CA-278: tu aceptacion
	add("tu aceptacion", bdEv(), ColTuAceptacion, "espera tu aceptacion para integrar")
	return cs
}

// CA-306: plain da el texto del contrato en cada caso de la tabla, con
// fixtures que recorren todas las ramas de la columna de C1; nunca vacio,
// sin las palabras de git (enteras, sin distinguir mayusculas), sin .hoom/
// y sin comandos de hoom.
func TestCA306_PlainPorCadaRamaDeLaColumna(t *testing.T) {
	vistas := map[string]bool{}
	for _, caso := range tbCasos() {
		ca := "CA-306 (" + caso.nombre + ")"
		c := Derive(caso.ev)
		bdCol(t, ca, c, caso.col)
		vistas[c.Column] = true
		if c.Plain != caso.plain {
			t.Errorf("%s: plain debe ser %q, fue %q (missing=%q)", ca, caso.plain, c.Plain, c.Missing)
			continue
		}
		tbNormal(t, ca, "plain", c.Plain)
		if c.Red != nil {
			tbNormal(t, ca, "red.plain", c.Red.Plain)
		}
	}
	for _, col := range Columns {
		if !vistas[col.ID] {
			t.Fatalf("CA-306: los fixtures recorren todas las columnas; falta %s", col.ID)
		}
	}
}

// CA-306: la regla de las palabras es entera y sin mayusculas: el propio
// chequeo del test no deja pasar las variantes ni rechaza palabras que solo
// las contienen.
func TestCA306_ChequeoDePalabrasEnteras(t *testing.T) {
	for _, mala := range []string{"el Git dice", "un COMMIT", "la rama main", "HEAD movido", "cambio la Huella", "hay un diff", "merge pendiente", "el worktree", "branch x"} {
		if tbProhibidas.FindString(mala) == "" {
			t.Fatalf("CA-306: el chequeo debe rechazar %q", mala)
		}
	}
	for _, buena := range []string{"digital", "programa", "comitente", "cabeza", "diferencia"} {
		if w := tbProhibidas.FindString(buena); w != "" {
			t.Fatalf("CA-306: %q no contiene una palabra prohibida entera (%q)", buena, w)
		}
	}
}

// CA-306: plain no cambia con los subestados de vida (en curso,
// interrumpido): es el motivo de la columna, no del sobre.
func TestCA306_PlainIgnoraLaNotaYLosSobres(t *testing.T) {
	ev := bdEv()
	ev.Verdict, ev.GreenVerdicts = nil, nil
	ev.Envelopes = []EnvelopeState{{Record: envelope.Record{ID: "env-1", Role: "writer", Provider: "claude",
		Task: bdSlug, Stage: "verify", Step: 4, Steps: 6, Status: envelope.StatusNotDeliverable,
		Note: "git diff en .hoom/worktrees/precios: hoom verify fallo", StartedAt: bdT0.Add(40 * time.Minute),
		UpdatedAt: bdT0.Add(50 * time.Minute), EndedAt: bdT0.Add(50 * time.Minute)}}}
	c := Derive(ev)
	if c.Plain != "falta verificar el trabajo contra el spec" {
		t.Fatalf("CA-306: plain es el de la columna, no la nota del sobre: %q", c.Plain)
	}
	tbNormal(t, "CA-306", "plain", c.Plain)
}

// tbSobreCerrado es un sobre de la tarjeta que cerro despues del veredicto.
func tbSobreCerrado(id, rol, status, stage, nota string) EnvelopeState {
	fin := bdT0.Add(50 * time.Minute)
	return EnvelopeState{Record: envelope.Record{ID: id, Role: rol, Provider: "claude", Task: bdSlug,
		Stage: stage, Step: 4, Steps: 6, Status: status, Note: nota,
		StartedAt: fin.Add(-10 * time.Minute), UpdatedAt: fin, EndedAt: fin}}
}

// CA-308: red.plain de un veredicto rojo es el mismo texto de plain en
// Writer, y reason no cambia.
func TestCA308_RedPlainDeUnVeredicto(t *testing.T) {
	casos := []struct {
		gates  []verdict.GateResult
		reason string
		plain  string
	}{
		{[]verdict.GateResult{{Name: "build", Required: true, Status: verdict.StatusPass}, {Name: "test", Required: true, Status: verdict.StatusFail}},
			"veredicto rojo: fallo el gate test", "la verificacion dio rojo: fallo el gate test"},
		{[]verdict.GateResult{{Name: "build", Required: true, Status: verdict.StatusFail}, {Name: "test", Required: true, Status: verdict.StatusError}},
			"veredicto rojo: fallaron los gates build, test", "la verificacion dio rojo: fallaron los gates build, test"},
	}
	for _, caso := range casos {
		ev := bdEv()
		ev.Verdict = bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10, caso.gates)
		c := Derive(ev)
		if c.Red == nil || c.Red.Source != RedVerdict || c.Red.ID != "v-rojo" {
			t.Fatalf("CA-308: el veredicto rojo es el rojo de la tarjeta: %+v", c.Red)
		}
		if c.Red.Reason != caso.reason {
			t.Fatalf("CA-308: reason queda como lo fijo C1 (%q), fue %q", caso.reason, c.Red.Reason)
		}
		if c.Red.Plain != caso.plain {
			t.Fatalf("CA-308: red.plain debe ser %q, fue %q", caso.plain, c.Red.Plain)
		}
		if c.Plain != caso.plain {
			t.Fatalf("CA-308: en Writer, plain y red.plain dicen lo mismo: %q vs %q", c.Plain, c.Red.Plain)
		}
	}
	// rojo sin requeridos en fail/error
	ev := bdEv()
	v := bdVeredicto("v-rojo0", bdT0.Add(40*time.Minute), "huella-1", 50, 10, []verdict.GateResult{{Name: "lint", Required: false, Status: verdict.StatusFail}})
	v.Verdict = "red"
	ev.Verdict = v
	c := Derive(ev)
	if c.Red == nil || c.Red.Plain != "la verificacion dio rojo" || c.Red.Reason != "veredicto rojo" {
		t.Fatalf("CA-308: sin gates requeridos que fallaron, red.plain es 'la verificacion dio rojo': %+v", c.Red)
	}
}

// CA-308: red.plain de un sobre: sin-entrega dice que no dejo ningun
// archivo (en cualquier paso), no-entregable usa la glosa de su paso, y
// ninguno repite la nota; reason sigue siendo el de C1, con la nota.
func TestCA308_RedPlainDeUnSobre(t *testing.T) {
	nota := "escribio .hoom/worktrees/precios/x.go; git diff y hoom verify lo muestran"
	ev := bdEv()
	ev.Envelopes = []EnvelopeState{tbSobreCerrado("env-se", "test-writer", envelope.StatusNoDelivery, "scope", nota)}
	c := Derive(ev)
	if c.Red == nil || c.Red.Source != RedEnvelope || c.Red.ID != "env-se" {
		t.Fatalf("CA-308: el sobre sin entrega mas nuevo es el rojo: %+v", c.Red)
	}
	if c.Red.Plain != "el test-writer no entrego: no dejo ningun archivo" {
		t.Fatalf("CA-308: red.plain de sin-entrega: %q", c.Red.Plain)
	}
	if c.Red.Reason != "el sobre de test-writer corto en scope: "+nota {
		t.Fatalf("CA-308: reason no cambia (con la nota): %q", c.Red.Reason)
	}

	glosas := map[string]string{
		"spec":   "el spec no tenia aprobacion vigente",
		"aislar": "no se pudo preparar el espacio ciego",
		"run":    "el agente termino con error",
		"scope":  "escribio fuera de lo que le toca",
		"verify": "la verificacion dio rojo",
		"check":  "el control final no paso",
		"raro":   "corto en el paso raro",
	}
	for stage, glosa := range glosas {
		ev := bdEv()
		ev.Envelopes = []EnvelopeState{tbSobreCerrado("env-ne", "writer", envelope.StatusNotDeliverable, stage, nota)}
		c := Derive(ev)
		if c.Red == nil || c.Red.Source != RedEnvelope {
			t.Fatalf("CA-308 (%s): el sobre no entregable mas nuevo es el rojo: %+v", stage, c.Red)
		}
		if want := "el writer no entrego: " + glosa; c.Red.Plain != want {
			t.Fatalf("CA-308 (%s): red.plain debe ser %q, fue %q", stage, want, c.Red.Plain)
		}
		if c.Red.Reason != "el sobre de writer corto en "+stage+": "+nota {
			t.Fatalf("CA-308 (%s): reason no cambia: %q", stage, c.Red.Reason)
		}
		if strings.Contains(c.Red.Plain, "x.go") || strings.Contains(c.Plain, "x.go") || strings.Contains(c.Red.Plain, nota) {
			t.Fatalf("CA-308 (%s): ni red.plain ni plain incluyen la nota: %q / %q", stage, c.Red.Plain, c.Plain)
		}
		tbNormal(t, "CA-308 ("+stage+")", "plain", c.Plain)
	}
}

// CA-309: needs_decision es true en las dos columnas humanas y en cualquier
// columna con un sobre interrumpido; false en las demas, tambien en curso.
func TestCA309_NeedsDecision(t *testing.T) {
	cuando := bdT0.Add(time.Hour)
	casos := map[string]func(*Evidence){
		ColBacklog:      func(e *Evidence) { e.SpecExists = false },
		ColArquitecto:   func(e *Evidence) { e.LintIssues = []string{"falta la seccion \"riesgos\""} },
		ColTuAprobacion: func(e *Evidence) { e.Approval = approval.StatusNotApproved },
		ColTestWriter:   func(e *Evidence) { e.Untraced = []string{"CA-2"} },
		ColWriter:       func(e *Evidence) { e.Verdict, e.GreenVerdicts = nil, nil },
		ColReview:       func(e *Evidence) { e.ReadyErr, e.ReadyKind = tbMsgSinGuardar, taskcmd.ReadySinGuardar },
		ColTuAceptacion: func(e *Evidence) {},
		ColHecho:        func(e *Evidence) { e.Item.HechoEn = &cuando },
	}
	abierto := func(alive bool) []EnvelopeState {
		return []EnvelopeState{{Record: envelope.Record{ID: "env-abierto", Role: "writer", Provider: "claude", Task: bdSlug,
			Stage: "run", Step: 2, Steps: 5, Status: envelope.StatusRunning,
			StartedAt: bdT0.Add(10 * time.Minute), UpdatedAt: bdT0.Add(11 * time.Minute)}, Alive: alive}}
	}
	for _, col := range Columns {
		humana := col.ID == ColTuAprobacion || col.ID == ColTuAceptacion
		for _, vida := range []string{"quieta", "en curso", "interrumpida"} {
			ev := bdEv()
			casos[col.ID](&ev)
			switch vida {
			case "en curso":
				ev.Envelopes = abierto(true)
			case "interrumpida":
				ev.Envelopes = abierto(false)
			}
			c := Derive(ev)
			bdCol(t, "CA-309", c, col.ID)
			quiere := humana || vida == "interrumpida"
			if c.NeedsDecision != quiere {
				t.Fatalf("CA-309: needs_decision en %s (%s) debe ser %v, fue %v (running=%v interrupted=%v)",
					col.ID, vida, quiere, c.NeedsDecision, c.Running != nil, c.Interrupted != nil)
			}
			if vida == "en curso" && c.Running == nil {
				t.Fatalf("CA-309: el fixture en curso tiene running: %+v", c)
			}
			if vida == "interrumpida" && c.Interrupted == nil {
				t.Fatalf("CA-309: el fixture interrumpido tiene interrupted: %+v", c)
			}
		}
	}
	// un run suelto vivo tampoco pide decision
	ev := bdEv()
	ev.Verdict, ev.GreenVerdicts = nil, nil
	ev.Runs = []RunState{{Meta: runcmd.Meta{ID: "run-suelto", Provider: "codex", Role: "writer", Task: bdSlug,
		Status: runcmd.StatusRunning, PID: 4343, CreatedAt: bdT0.Add(20 * time.Minute)}, Alive: true}}
	if c := Derive(ev); c.Running == nil || c.NeedsDecision {
		t.Fatalf("CA-309: una tarjeta en curso por un run suelto no pide decision: running=%+v needs=%v", c.Running, c.NeedsDecision)
	}
}

// CA-307: Gather llena ReadyKind con errors.As sobre el error de
// taskcmd.Ready, y plain en Review sale del kind (nunca del mensaje).
func TestCA307_ReadyKindDesdeElDisco(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdEscribir(t, wt, "precios.go", "package app\n\nfunc Precio() int { return 1 }\n")
	bdAprobar(t, wt, bdSlug)
	v := bdVeredictoDisco(t, wt, bdSpec, bdT0.Add(time.Minute), false, bdGatesVerdes())
	bdCommitear(t, wt, "spec, codigo y veredicto")

	ev := Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.ReadyErr != "" || ev.ReadyKind != "" {
		t.Fatalf("CA-307: listo para cerrar: sin ReadyErr ni ReadyKind: %q %q", ev.ReadyErr, ev.ReadyKind)
	}
	if c := Derive(ev); c.Column != ColTuAceptacion || c.Plain != "espera tu aceptacion para integrar" {
		t.Fatalf("CA-307: limpio y verde es tu aceptacion: %s %q", c.Column, c.Plain)
	}

	// sucio (un hallazgo sin tarea no mueve la huella): sin-guardar
	if _, err := finding.Register(wt, "main", finding.Draft{Severity: "low", Lens: "risk", Description: "nota",
		Author: "reviewer"}); err != nil {
		t.Fatal(err)
	}
	ev = Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.ReadyKind != taskcmd.ReadySinGuardar || !strings.Contains(ev.ReadyErr, "tiene cambios sin commitear") {
		t.Fatalf("CA-307: sucio da ReadyKind sin-guardar y el mensaje de siempre: %q %q", ev.ReadyKind, ev.ReadyErr)
	}
	c := Derive(ev)
	bdCol(t, "CA-307", c, ColReview)
	bdPrimero(t, "CA-277", c, ev.ReadyErr) // missing no cambia
	if c.Plain != "hay cambios sin guardar en el espacio de trabajo" {
		t.Fatalf("CA-307: plain sale del kind: %q", c.Plain)
	}
	bdCommitear(t, wt, "hallazgo")

	// el ultimo completo del worktree es rojo (un verify sin --spec): rojo
	bdVeredictoDisco(t, wt, "", bdT0.Add(2*time.Minute), false,
		[]verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
	bdCommitear(t, wt, "veredicto rojo sin spec")
	ev = Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.Verdict == nil || ev.Verdict.ID != v.ID {
		t.Fatalf("CA-307: el veredicto de la tarjeta sigue siendo el verde con spec: %+v", ev.Verdict)
	}
	if ev.ReadyKind != taskcmd.ReadyRojo {
		t.Fatalf("CA-307: el ultimo completo rojo da ReadyKind rojo: %q (%q)", ev.ReadyKind, ev.ReadyErr)
	}
	if c := Derive(ev); c.Column != ColReview || c.Plain != "la ultima verificacion del espacio de trabajo dio rojo" {
		t.Fatalf("CA-307: plain del rojo del espacio de trabajo: %s %q", c.Column, c.Plain)
	}

	// un verde mas nuevo con otra huella: huella
	g := gitx.Snapshot(wt, "main")
	g.ChangeFingerprint = "otra-huella"
	vh := &verdict.Verdict{Project: "demo", CreatedAt: bdT0.Add(3 * time.Minute), Git: g, Gates: bdGatesVerdes()}
	vh.Finalize()
	if _, err := verdict.Write(wt, vh); err != nil {
		t.Fatal(err)
	}
	bdCommitear(t, wt, "verde de otra huella")
	ev = Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.ReadyKind != taskcmd.ReadyHuella {
		t.Fatalf("CA-307: el ultimo verde con otra huella da ReadyKind huella: %q (%q)", ev.ReadyKind, ev.ReadyErr)
	}
	if c := Derive(ev); c.Column != ColReview || c.Plain != "el espacio de trabajo cambio despues del ultimo verde" {
		t.Fatalf("CA-307: plain de la huella del espacio de trabajo: %s %q", c.Column, c.Plain)
	}
}
