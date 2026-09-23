// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-325, CA-326, CA-327, CA-329, CA-330): la tarjeta sabe que acciones
// valen. La lista y su orden, si cada una esta habilitada y por que no, las
// opciones de provider con el presupuesto, el pedido propuesto y reanudar
// salen de Derive, que sigue siendo pura: Evidence literales, sin disco, sin
// reloj y sin git. Solo el test de punta a punta usa el binario real.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// acSigner es la identidad git del arbol de evidencia de los fixtures.
const acSigner = "Henry Orellana <henry@hoom.dev>"

// Frases del contrato (sin acentos, como las emite el binario).
const (
	acSinEspacio  = "la tarjeta no tiene su propio espacio de trabajo: desde el tablero solo se le pide trabajo a una tarjeta que lo tiene"
	acNingunProv  = "ningun proveedor instalado puede tomar este trabajo"
	acNoVuelve    = "la tarjeta no vuelve atras: su columna la da la evidencia"
	acAntesDelRun = "el trabajo se corto antes de que el agente empezara: solo se puede volver a lanzar"
	acSinSesion   = "el agente no dejo una sesion que retomar: solo se puede volver a lanzar"
	acRevisionYa  = "una revision cortada se vuelve a lanzar entera"
	acOpenSinCtto = "opencode no puede recibir el contrato del rol"
	acPedArq      = `Escribi el spec .hoom/specs/precios.md de la tarjeta "Precios por region".`
	acPedArqConP  = acPedArq + "\nPedido: que el catalogo muestre precios por region"
	acPedWriter   = "Implementa .hoom/specs/precios.md hasta que hoom verify --spec .hoom/specs/precios.md de verde."
	acPedTW       = "Escribi los tests que citan los criterios de .hoom/specs/precios.md que todavia no tienen prueba: CA-3, CA-5."
	acPedReanudar = "El trabajo anterior se corto en el paso 3 de 5 (run). Revisa el arbol como quedo y termina lo que se te pidio."
)

func acEspera(rol string) string { return "espera a que termine el " + rol + " que esta trabajando" }

func acAgotado(gastado, presupuesto string) string {
	return "se agoto el presupuesto de la tarjeta: se gastaron " + gastado + " de " + presupuesto + " USD"
}

// Etiquetas, roles y modo experto de cada accion, segun el contrato.
var (
	acEtiquetas = map[string]string{
		ActPedirArquitecto: "Pedir al arquitecto", ActPedirTestWriter: "Pedir al test-writer",
		ActPedirWriter: "Pedir al writer", ActPedirReviewer: "Pedir al reviewer", ActAprobar: "Aprobar spec",
		ActIntegrar: "Integrar", ActReanudar: "Reanudar", ActRelanzar: "Volver a lanzar",
		ActDescartar: "Descartar cambios", ActGuardar: "Guardar", ActSesion: "Abrir sesion", ActTerminal: "Ver terminal",
	}
	acRolDe = map[string]string{ActPedirArquitecto: "arquitecto", ActPedirTestWriter: "test-writer",
		ActPedirWriter: "writer", ActPedirReviewer: "reviewer"}
	acExperta = map[string]bool{ActDescartar: true, ActSesion: true, ActTerminal: true}
	// acDeRol son las acciones que lanzan un rol.
	acDeRol = map[string]bool{ActPedirArquitecto: true, ActPedirTestWriter: true, ActPedirWriter: true,
		ActPedirReviewer: true, ActReanudar: true, ActRelanzar: true}
)

// acProvs son los providers de la maquina de los fixtures, en el orden del
// registro: claude (tope en USD, minimo 0.5, retoma sesiones), opencode (no
// recibe el contrato del rol), codex (sin tope ni minimo) y gemini, que no
// esta instalado y por eso nunca es opcion.
func acProvs() []providers.Info {
	return []providers.Info{
		{Name: "claude", Installed: true, Bin: "/opt/bin/claude", MinBudgetUSD: 0.5,
			Capabilities: providers.Capabilities{Structured: true, Resume: true, SessionID: true, Model: true,
				SystemPrompt: true, Budget: true}},
		{Name: "opencode", Installed: true, Bin: "/opt/bin/opencode",
			Capabilities: providers.Capabilities{Continue: true}},
		{Name: "codex", Installed: true, Bin: "/opt/bin/codex",
			Capabilities: providers.Capabilities{Structured: true, Resume: true, SessionID: true, Model: true,
				SystemPrompt: true}},
		{Name: "gemini", Installed: false, Capabilities: providers.Capabilities{SystemPrompt: true}},
	}
}

// acEv es bdEv (Tu aceptacion, con espacio de trabajo propio) con los
// providers de la maquina y la identidad que firmaria.
func acEv() Evidence {
	ev := bdEv()
	ev.Providers = acProvs()
	ev.Signer = acSigner
	return ev
}

// acWriter: la tarjeta en Writer (sin veredicto).
func acWriter() Evidence {
	ev := acEv()
	ev.Verdict, ev.GreenVerdicts = nil, nil
	return ev
}

// acTestWriter: la tarjeta en Test-writer, con CA-3 y CA-5 sin prueba.
func acTestWriter() Evidence {
	ev := acEv()
	ev.Criteria = []string{"CA-1", "CA-2", "CA-3", "CA-5"}
	ev.Untraced = []string{"CA-3", "CA-5"}
	return ev
}

// acGrande: el verde de la tarjeta cambia 612 lineas y la review de 4 lentes
// es exigida.
func acGrande(ev Evidence) Evidence {
	v := bdVeredicto("v-grande", bdT0.Add(30*time.Minute), "huella-1", 500, 112, bdGatesVerdes())
	ev.Verdict, ev.GreenVerdicts = v, []string{v.ID}
	return ev
}

// acConHallazgos suma hallazgos abiertos de la tarjeta que bloquean (high).
func acConHallazgos(ev Evidence, ids ...string) Evidence {
	for _, id := range ids {
		ev.Findings = append(ev.Findings, bdHallazgo(id, "high", bdSlug))
	}
	return ev
}

// acRegistroQueCuenta es una review de 4 lentes sobre el verde grande.
func acRegistroQueCuenta() reviewcmd.Record {
	return reviewcmd.Record{ID: "r-cuenta", CreatedAt: bdT0.Add(35 * time.Minute), Task: bdSlug, Spec: bdSpec,
		Fingerprint: "huella-1", VerdictID: "v-grande", Verdict: "green", Lenses: reviewcmd.Lentes,
		Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}}
}

// acGasto es un run cerrado de la tarjeta que reporto (o no) su costo.
func acGasto(id, rol, provider string, costo *float64, en time.Duration) RunState {
	var u *providers.Usage
	if costo != nil {
		u = &providers.Usage{CostUSD: costo}
	}
	return RunState{Meta: runcmd.Meta{ID: id, Provider: provider, Role: rol, Task: bdSlug, Status: runcmd.StatusDone,
		CreatedAt: bdT0.Add(en), EndedAt: bdT0.Add(en + time.Minute), Usage: u}}
}

// acConGasto le pone presupuesto al item y un run del writer (claude) que
// gasto costo.
func acConGasto(ev Evidence, presupuesto, costo float64) Evidence {
	ev.Item.PresupuestoUSD = bdF(presupuesto)
	ev.Runs = append(ev.Runs, acGasto("run-gasto", "writer", "claude", bdF(costo), 1*time.Minute))
	return ev
}

// acRunVivo es un run suelto (sin sobre) cuyo dueno vive.
func acRunVivo(rol, provider string) RunState {
	return RunState{Meta: runcmd.Meta{ID: "run-vivo", Provider: provider, Role: rol, Task: bdSlug,
		Status: runcmd.StatusRunning, PID: 4343, CreatedAt: bdT0.Add(20 * time.Minute)}, Alive: true}
}

// acSobreVivo es un sobre abierto cuyo dueno vive, parado en un paso.
func acSobreVivo(rol, provider, stage string, step, steps int) EnvelopeState {
	return EnvelopeState{Record: envelope.Record{ID: "env-vivo", RunID: "run-del-sobre", Role: rol, Provider: provider,
		Task: bdSlug, Dir: bdWtDir, Stage: stage, Step: step, Steps: steps, Status: envelope.StatusRunning,
		StartedAt: bdT0.Add(20 * time.Minute), UpdatedAt: bdT0.Add(21 * time.Minute)}, Alive: true}
}

// acInterrumpido suma un sobre abierto de rol cuyo dueno murio en el paso 3
// de 5 (run), con el sidecar de su run y el id de sesion del provider. Ningun
// otro sobre de los fixtures empieza despues de su ultimo registro (12 min),
// asi que nada lo releva.
func acInterrumpido(ev Evidence, rol, provider string) Evidence {
	ev.Envelopes = append(ev.Envelopes, EnvelopeState{Record: envelope.Record{ID: "env-caido", RunID: "run-caido",
		Role: rol, Provider: provider, Task: bdSlug, Dir: bdWtDir, Stage: "run", Step: 3, Steps: 5,
		Status: envelope.StatusRunning, StartedAt: bdT0.Add(10 * time.Minute), UpdatedAt: bdT0.Add(12 * time.Minute)}})
	ev.Runs = append(ev.Runs, RunState{Meta: runcmd.Meta{ID: "run-caido", Provider: provider, Role: rol, Task: bdSlug,
		Status: runcmd.StatusRunning, ProviderSessionID: "sesion-abc123", CreatedAt: bdT0.Add(11 * time.Minute), PID: 4242}})
	return ev
}

// acSucios son rutas sin guardar de la tarjeta con espacio de trabajo propio:
// el item del arbol raiz, evidencia del espacio (bajo su .hoom/) y codigo.
// Desordenadas a proposito.
func acSucios() []string {
	return []string{
		bdWtDir + "/web/precios.js",
		".hoom/items/precios.yaml",
		bdWtDir + "/precios.go",
		bdWtDir + "/.hoom/verdicts/v-1.json",
	}
}

var (
	acSuciosOrdenados = []string{".hoom/items/precios.yaml", bdWtDir + "/.hoom/verdicts/v-1.json",
		bdWtDir + "/precios.go", bdWtDir + "/web/precios.js"}
	acDescartables = []string{bdWtDir + "/precios.go", bdWtDir + "/web/precios.js"}
)

type acCaso struct {
	nombre string
	ev     Evidence
	col    string
	ids    []string // las acciones esperadas, en orden
}

// acFix es un fixture con nombre, para los chequeos que valen en todos.
type acFix struct {
	nombre string
	ev     Evidence
}

// acColumnas recorre las ocho columnas (Review en sus cinco formas), con
// espacio de trabajo propio, sin nada en curso, interrumpido ni sin guardar.
func acColumnas() []acCaso {
	cuando := bdT0.Add(48 * time.Hour)
	con := func(ids ...string) []string { return append(append([]string{}, ids...), ActSesion, ActTerminal) }
	var cs []acCaso
	add := func(nombre string, ev Evidence, col string, ids []string) {
		cs = append(cs, acCaso{nombre, ev, col, ids})
	}

	ev := acEv()
	ev.SpecExists = false
	add("backlog", ev, ColBacklog, con(ActPedirArquitecto))
	ev = acEv()
	ev.LintIssues = []string{`falta la seccion "riesgos"`}
	add("arquitecto", ev, ColArquitecto, con(ActPedirArquitecto))
	ev = acEv()
	ev.Approval = approval.StatusNotApproved
	add("tu-aprobacion", ev, ColTuAprobacion, con(ActAprobar, ActPedirArquitecto))
	add("test-writer", acTestWriter(), ColTestWriter, con(ActPedirTestWriter))
	add("writer", acWriter(), ColWriter, con(ActPedirWriter))
	add("review exigida sin registro", acGrande(acEv()), ColReview, con(ActPedirReviewer))
	add("review con hallazgos", acConHallazgos(acEv(), "f-a"), ColReview, con(ActPedirWriter))
	add("review exigida y con hallazgos", acConHallazgos(acGrande(acEv()), "f-a"), ColReview,
		con(ActPedirReviewer, ActPedirWriter))
	ev = acConHallazgos(acGrande(acEv()), "f-a")
	ev.Reviews = []reviewcmd.Record{acRegistroQueCuenta()}
	add("review exigida con registro que cuenta y con hallazgos", ev, ColReview, con(ActPedirWriter))
	add("review sin exigir ni hallazgos", tbListo(acEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar), ColReview, con())
	add("tu-aceptacion", acEv(), ColTuAceptacion, con(ActIntegrar))
	ev = acEv()
	ev.Item.HechoEn = &cuando
	add("hecho", ev, ColHecho, con())
	return cs
}

// acColumnasArbol son las mismas columnas sin espacio de trabajo propio: la
// evidencia vive en el arbol del proyecto.
func acColumnasArbol() []acCaso {
	cuando := bdT0.Add(48 * time.Hour)
	var cs []acCaso
	add := func(nombre string, ev Evidence, col string, ids ...string) {
		if ids == nil {
			ids = []string{}
		}
		cs = append(cs, acCaso{nombre, ev, col, ids})
	}
	ev := bdArbol(acEv())
	ev.SpecExists = false
	add("backlog sin espacio propio", ev, ColBacklog, ActPedirArquitecto)
	ev = bdArbol(acEv())
	ev.LintIssues = []string{`falta la seccion "riesgos"`}
	add("arquitecto sin espacio propio", ev, ColArquitecto, ActPedirArquitecto)
	ev = bdArbol(acEv())
	ev.Approval = approval.StatusNotApproved
	add("tu-aprobacion sin espacio propio", ev, ColTuAprobacion, ActAprobar, ActPedirArquitecto)
	add("test-writer sin espacio propio", bdArbol(acTestWriter()), ColTestWriter, ActPedirTestWriter)
	add("writer sin espacio propio", bdArbol(acWriter()), ColWriter, ActPedirWriter)
	add("review exigida sin espacio propio", bdArbol(acGrande(acEv())), ColReview, ActPedirReviewer)
	add("review con hallazgos sin espacio propio", bdArbol(acConHallazgos(acEv(), "f-a")), ColReview, ActPedirWriter)
	add("review sin espacio propio y sin accion", bdArbol(acEv()), ColReview)
	ev = bdArbol(acEv())
	ev.Item.HechoEn = &cuando
	add("hecho sin espacio propio", ev, ColHecho)
	return cs
}

func acIDs(c Card) []string {
	out := []string{}
	for _, a := range c.Actions {
		out = append(out, a.ID)
	}
	return out
}

// acAccion busca una accion por id sin pasar por Card.Action.
func acAccion(t *testing.T, ca string, c Card, id string) Action {
	t.Helper()
	for _, a := range c.Actions {
		if a.ID == id {
			return a
		}
	}
	t.Fatalf("%s: la tarjeta (columna %s) no tiene la accion %q; tiene %v", ca, c.Column, id, acIDs(c))
	return Action{}
}

func acNoTiene(t *testing.T, ca string, c Card, id string) {
	t.Helper()
	for _, a := range c.Actions {
		if a.ID == id {
			t.Fatalf("%s: la tarjeta (columna %s) no debe tener la accion %q; tiene %v", ca, c.Column, id, acIDs(c))
		}
	}
}

func acCol(t *testing.T, ca, nombre string, c Card, want string) {
	t.Helper()
	if c.Column != want {
		t.Fatalf("%s: [%s] el fixture debe caer en %q, cayo en %q (missing=%q)", ca, nombre, want, c.Column, c.Missing)
	}
}

func acHabilitada(t *testing.T, ca string, a Action) {
	t.Helper()
	if !a.Enabled || a.Why != "" {
		t.Fatalf("%s: la accion %s debe estar habilitada con why \"\": enabled=%v why=%q", ca, a.ID, a.Enabled, a.Why)
	}
}

func acDeshabilitada(t *testing.T, ca string, a Action, why string) {
	t.Helper()
	if a.Enabled || a.Why != why {
		t.Fatalf("%s: la accion %s debe estar deshabilitada con why %q: enabled=%v why=%q", ca, a.ID, why, a.Enabled, a.Why)
	}
}

// acBienFormada: etiqueta, rol y experto del contrato; why vacio si y solo si
// habilitada; listas nunca nil; signer solo en aprobar y resume_id solo en
// reanudar; drops con una entrada por columna ajena.
func acBienFormada(t *testing.T, ca, nombre string, c Card) {
	t.Helper()
	if c.Actions == nil {
		t.Fatalf("%s: [%s] actions nunca es nil", ca, nombre)
	}
	vistas := map[string]bool{}
	for _, a := range c.Actions {
		if vistas[a.ID] {
			t.Fatalf("%s: [%s] la accion %s aparece dos veces: %v", ca, nombre, a.ID, acIDs(c))
		}
		vistas[a.ID] = true
		label, ok := acEtiquetas[a.ID]
		if !ok {
			t.Fatalf("%s: [%s] accion fuera del contrato: %q", ca, nombre, a.ID)
		}
		if a.Label != label {
			t.Fatalf("%s: [%s] la etiqueta de %s es %q, fue %q", ca, nombre, a.ID, label, a.Label)
		}
		if a.Expert != acExperta[a.ID] {
			t.Fatalf("%s: [%s] expert es true solo en descartar, sesion y terminal: %s tiene expert=%v", ca, nombre, a.ID, a.Expert)
		}
		rol := acRolDe[a.ID]
		if a.ID == ActReanudar || a.ID == ActRelanzar {
			if c.Interrupted == nil {
				t.Fatalf("%s: [%s] %s solo aparece con interrupted", ca, nombre, a.ID)
			}
			rol = c.Interrupted.Role
		}
		if a.Role != rol {
			t.Fatalf("%s: [%s] el rol de %s es %q, fue %q", ca, nombre, a.ID, rol, a.Role)
		}
		if a.Enabled != (a.Why == "") {
			t.Fatalf("%s: [%s] why es \"\" si y solo si la accion esta habilitada: %s enabled=%v why=%q", ca, nombre, a.ID, a.Enabled, a.Why)
		}
		if a.Providers == nil || a.Paths == nil {
			t.Fatalf("%s: [%s] providers y paths de %s nunca son nil: %+v", ca, nombre, a.ID, a)
		}
		if a.ID != ActGuardar && a.ID != ActDescartar && len(a.Paths) != 0 {
			t.Fatalf("%s: [%s] paths es [] salvo en guardar y descartar: %s %q", ca, nombre, a.ID, a.Paths)
		}
		if (a.ID == ActAprobar) != (a.Signer != "") {
			t.Fatalf("%s: [%s] signer va solo en aprobar: %s signer=%q", ca, nombre, a.ID, a.Signer)
		}
		if a.ID != ActReanudar && a.ResumeID != "" {
			t.Fatalf("%s: [%s] resume_id va solo en reanudar: %s resume_id=%q", ca, nombre, a.ID, a.ResumeID)
		}
		if a.ID == ActAprobar && a.Signer != acSigner {
			t.Fatalf("%s: [%s] el signer de aprobar es la identidad del arbol de evidencia %q, fue %q", ca, nombre, acSigner, a.Signer)
		}
	}
	if c.Drops == nil || len(c.Drops) != len(Columns)-1 {
		t.Fatalf("%s: [%s] drops trae una entrada por cada columna que no es la de la tarjeta (7): %+v", ca, nombre, c.Drops)
	}
}

// CA-325: los ids de las acciones son los del contrato.
func TestCA325_IdsDeLasAcciones(t *testing.T) {
	got := []string{ActPedirArquitecto, ActPedirTestWriter, ActPedirWriter, ActPedirReviewer, ActAprobar, ActIntegrar,
		ActReanudar, ActRelanzar, ActDescartar, ActGuardar, ActSesion, ActTerminal}
	want := []string{"pedir-arquitecto", "pedir-test-writer", "pedir-writer", "pedir-reviewer", "aprobar", "integrar",
		"reanudar", "relanzar", "descartar", "guardar", "sesion", "terminal"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("CA-325: los ids de las acciones son %v, fueron %v", want, got)
	}
}

// CA-325: actions sigue la tabla de columnas, en su orden y con la principal
// primero, en las ocho columnas (Review con y sin review exigida y con y sin
// hallazgos que bloquean), con y sin espacio de trabajo propio.
func TestCA325_AccionesPorColumna(t *testing.T) {
	casos := append(acColumnas(), acColumnasArbol()...)
	vistas := map[string]bool{}
	for _, cs := range casos {
		c := Derive(cs.ev)
		acCol(t, "CA-325", cs.nombre, c, cs.col)
		vistas[c.Column] = true
		if got := acIDs(c); !reflect.DeepEqual(got, cs.ids) {
			t.Fatalf("CA-325: [%s] las acciones de la columna %s son %v, fueron %v", cs.nombre, cs.col, cs.ids, got)
		}
		acBienFormada(t, "CA-325", cs.nombre, c)
		for _, a := range c.Actions {
			b, ok := c.Action(a.ID)
			if !ok || !reflect.DeepEqual(a, b) {
				t.Fatalf("CA-325: [%s] Card.Action(%q) devuelve la accion de la lista: ok=%v %+v", cs.nombre, a.ID, ok, b)
			}
		}
		if _, ok := c.Action("mover"); ok {
			t.Fatalf("CA-325: [%s] Card.Action de un id que la tarjeta no tiene es false", cs.nombre)
		}
	}
	if len(vistas) != len(Columns) {
		t.Fatalf("CA-325: los fixtures recorren las ocho columnas, recorrieron %v", vistas)
	}
}

// CA-325: reanudar y relanzar solo con interrupted; descartar solo con
// interrupted, espacio de trabajo propio y rutas descartables; guardar solo
// con unsynced; sesion y terminal solo con espacio de trabajo propio. Despues
// de las de la columna, en ese orden.
func TestCA325_AccionesDeCualquierColumna(t *testing.T) {
	casos := []acCaso{}
	add := func(nombre string, ev Evidence, ids ...string) {
		casos = append(casos, acCaso{nombre, ev, ColWriter, ids})
	}
	ev := acInterrumpido(acWriter(), "writer", "claude")
	ev.Uncommitted = acSucios()
	add("interrumpida, con cambios descartables", ev,
		ActPedirWriter, ActReanudar, ActRelanzar, ActDescartar, ActGuardar, ActSesion, ActTerminal)

	ev = acInterrumpido(acWriter(), "writer", "claude")
	ev.Uncommitted = []string{".hoom/items/precios.yaml", bdWtDir + "/.hoom/verdicts/v-1.json"}
	add("interrumpida, solo evidencia sin guardar", ev,
		ActPedirWriter, ActReanudar, ActRelanzar, ActGuardar, ActSesion, ActTerminal)

	add("interrumpida y guardada", acInterrumpido(acWriter(), "writer", "claude"),
		ActPedirWriter, ActReanudar, ActRelanzar, ActSesion, ActTerminal)

	ev = acWriter()
	ev.Uncommitted = acSucios()
	add("sin interrumpir, con cambios", ev, ActPedirWriter, ActGuardar, ActSesion, ActTerminal)

	// un sobre CERRADO no es interrumpido: ni reanudar ni relanzar
	ev = acWriter()
	ev.Uncommitted = acSucios()
	ev.Envelopes = []EnvelopeState{tbSobreCerrado("env-cerrado", "writer", envelope.StatusNotDeliverable, "verify", "nota")}
	add("sobre cerrado", ev, ActPedirWriter, ActGuardar, ActSesion, ActTerminal)

	ev = bdArbol(acInterrumpido(acWriter(), "writer", "claude"))
	ev.Uncommitted = []string{".hoom/specs/precios.md", ".hoom/items/precios.yaml"}
	add("interrumpida sin espacio propio", ev, ActPedirWriter, ActReanudar, ActRelanzar, ActGuardar)

	for _, cs := range casos {
		c := Derive(cs.ev)
		acCol(t, "CA-325", cs.nombre, c, cs.col)
		if got := acIDs(c); !reflect.DeepEqual(got, cs.ids) {
			t.Fatalf("CA-325: [%s] las acciones son %v, fueron %v", cs.nombre, cs.ids, got)
		}
		acBienFormada(t, "CA-325", cs.nombre, c)
	}

	// en Hecho y en Tu aceptacion tambien: las de cualquier columna no
	// dependen de la columna
	cuando := bdT0.Add(48 * time.Hour)
	ev = acEv()
	ev.Item.HechoEn = &cuando
	ev.Uncommitted = acSucios()
	c := Derive(ev)
	if got := acIDs(c); !reflect.DeepEqual(got, []string{ActGuardar, ActSesion, ActTerminal}) {
		t.Fatalf("CA-325: en Hecho con cambios sin guardar: [guardar sesion terminal], fue %v", got)
	}
	ev = acEv()
	ev.Uncommitted = acSucios()
	c = Derive(ev)
	if got := acIDs(c); !reflect.DeepEqual(got, []string{ActIntegrar, ActGuardar, ActSesion, ActTerminal}) {
		t.Fatalf("CA-325: en Tu aceptacion con cambios sin guardar: [integrar guardar sesion terminal], fue %v", got)
	}
}

// CA-325: paths de guardar es unsynced; el de descartar, las rutas de
// unsynced dentro del espacio de trabajo y fuera de su .hoom/.
func TestCA325_PathsDeGuardarYDescartar(t *testing.T) {
	ev := acInterrumpido(acWriter(), "writer", "claude")
	ev.Uncommitted = acSucios()
	c := Derive(ev)
	g := acAccion(t, "CA-325", c, ActGuardar)
	if !reflect.DeepEqual(g.Paths, c.Unsynced) || !reflect.DeepEqual(g.Paths, acSuciosOrdenados) {
		t.Fatalf("CA-325: paths de guardar es unsynced %q, fue %q", acSuciosOrdenados, g.Paths)
	}
	d := acAccion(t, "CA-325", c, ActDescartar)
	if !reflect.DeepEqual(d.Paths, acDescartables) {
		t.Fatalf("CA-325: paths de descartar son las rutas del espacio de trabajo fuera de su .hoom/ %q, fue %q", acDescartables, d.Paths)
	}
	for _, id := range []string{ActPedirWriter, ActReanudar, ActRelanzar, ActSesion, ActTerminal} {
		if a := acAccion(t, "CA-325", c, id); a.Paths == nil || len(a.Paths) != 0 {
			t.Fatalf("CA-325: paths de %s es [] (no nil): %#v", id, a.Paths)
		}
	}
	// la tarjeta no comparte su lista con la evidencia
	g.Paths[0] = "mutado"
	if ev.Uncommitted[0] == "mutado" || ev.Uncommitted[1] == "mutado" {
		t.Fatal("CA-325: paths no comparte el arreglo de la evidencia")
	}
}

// acJSON serializa la tarjeta y la vuelve a leer como mapa generico.
func acJSON(t *testing.T, c Card) (map[string]any, string) {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m, string(raw)
}

// acJSONBien exige las claves nuevas sin listas null.
func acJSONBien(t *testing.T, ca, nombre string, m map[string]any, raw string) {
	t.Helper()
	acts, ok := m["actions"].([]any)
	if !ok {
		t.Fatalf("%s: [%s] actions es una lista (no null): %s", ca, nombre, raw)
	}
	for _, x := range acts {
		a, _ := x.(map[string]any)
		for _, k := range []string{"id", "role", "label", "expert", "enabled", "why", "pedido", "budget_usd",
			"providers", "paths", "resume_id", "signer"} {
			if _, ok := a[k]; !ok {
				t.Fatalf("%s: [%s] cada accion trae %q: %v", ca, nombre, k, a)
			}
		}
		for _, k := range []string{"providers", "paths"} {
			if _, ok := a[k].([]any); !ok {
				t.Fatalf("%s: [%s] %s de una accion es una lista (no null): %v", ca, nombre, k, a)
			}
		}
		ops, _ := a["providers"].([]any)
		for _, o := range ops {
			op, _ := o.(map[string]any)
			for _, k := range []string{"name", "ok", "why", "budget", "min_budget_usd", "default"} {
				if _, ok := op[k]; !ok {
					t.Fatalf("%s: [%s] cada opcion de provider trae %q: %v", ca, nombre, k, op)
				}
			}
		}
	}
	drops, ok := m["drops"].([]any)
	if !ok || len(drops) != 7 {
		t.Fatalf("%s: [%s] drops es una lista de 7 entradas: %s", ca, nombre, raw)
	}
	for _, x := range drops {
		d, _ := x.(map[string]any)
		for _, k := range []string{"column", "action", "why"} {
			if _, ok := d[k].(string); !ok {
				t.Fatalf("%s: [%s] cada drop trae %q como texto: %v", ca, nombre, k, d)
			}
		}
	}
	if _, ok := m["ghost"]; !ok {
		t.Fatalf("%s: [%s] la tarjeta trae la clave ghost (null sin fantasma): %s", ca, nombre, raw)
	}
	prov, _ := m["providers"].(map[string]any)
	if _, ok := prov["declared"].([]any); !ok {
		t.Fatalf("%s: [%s] providers.declared es una lista (no null): %s", ca, nombre, raw)
	}
	sp, _ := m["spend"].(map[string]any)
	if _, ok := sp["remaining_usd"]; !ok {
		t.Fatalf("%s: [%s] spend trae remaining_usd: %s", ca, nombre, raw)
	}
	// las claves de C1 y C2 siguen ahi
	for _, k := range []string{"slug", "item", "column", "column_name", "missing", "next", "evidence", "running",
		"interrupted", "waiting_human", "red", "unsynced", "spend", "plain", "needs_decision", "meter", "providers"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("%s: [%s] a la tarjeta le falta la clave %q de C1/C2: %s", ca, nombre, k, raw)
		}
	}
}

// CA-325: en JSON la tarjeta trae actions, drops y ghost, sin listas null
// (actions [] en una tarjeta sin acciones), y ghost null sin fantasma.
func TestCA325_CardJSONSinListasNull(t *testing.T) {
	casos := append(acColumnas(), acColumnasArbol()...)
	ev := acInterrumpido(acWriter(), "writer", "claude")
	ev.Uncommitted = acSucios()
	casos = append(casos, acCaso{"interrumpida con cambios", ev, ColWriter, nil})
	for _, cs := range casos {
		m, raw := acJSON(t, Derive(cs.ev))
		acJSONBien(t, "CA-325", cs.nombre, m, raw)
		if m["ghost"] != nil {
			t.Fatalf("CA-325: [%s] sin nada en curso ghost es null: %s", cs.nombre, raw)
		}
	}
	// una tarjeta sin ninguna accion: "actions": []
	cuando := bdT0.Add(48 * time.Hour)
	hecho := bdArbol(acEv())
	hecho.Item.HechoEn = &cuando
	_, raw := acJSON(t, Derive(hecho))
	if !strings.Contains(raw, `"actions":[]`) {
		t.Fatalf("CA-325: una tarjeta sin acciones trae \"actions\":[] (no null): %s", raw)
	}
	if !strings.Contains(raw, `"ghost":null`) {
		t.Fatalf("CA-325: sin fantasma, \"ghost\":null: %s", raw)
	}
}

// CA-325: el texto de hoom board y de hoom item show no cambia: no depende de
// actions, drops, ghost, remaining_usd ni declared, y no nombra acciones.
func TestCA325_TextoDeBoardYShowNoCambia(t *testing.T) {
	ev := acConGasto(acInterrumpido(acWriter(), "writer", "claude"), 5, 0.8)
	ev.Uncommitted = acSucios()
	ev.Runs = append(ev.Runs, acRunVivo("writer", "claude"))
	c := Derive(ev)
	if len(c.Actions) == 0 || len(c.Drops) == 0 || c.Ghost == nil || c.Spend.RemainingUSD == nil {
		t.Fatalf("CA-325: el fixture tiene acciones, drops, fantasma y lo que queda del presupuesto: %+v", c)
	}
	sin := c
	sin.Actions, sin.Drops, sin.Ghost = nil, nil, nil
	sin.Spend.RemainingUSD = nil
	sin.Providers.Declared = nil

	var a, b bytes.Buffer
	RenderCard(&a, c)
	RenderCard(&b, sin)
	if a.String() != b.String() {
		t.Fatalf("CA-325: el texto de hoom item show no cambia con los campos nuevos:\ncon:\n%s\nsin:\n%s", a.String(), b.String())
	}
	var ta, tb bytes.Buffer
	Render(&ta, bdTablero(c))
	Render(&tb, bdTablero(sin))
	if ta.String() != tb.String() {
		t.Fatalf("CA-325: el texto de hoom board no cambia con los campos nuevos:\ncon:\n%s\nsin:\n%s", ta.String(), tb.String())
	}
	for _, txt := range []string{a.String(), ta.String()} {
		for _, label := range acEtiquetas {
			if label == "Guardar" || label == "Integrar" || label == "Reanudar" {
				continue
			}
			if strings.Contains(txt, label) {
				t.Fatalf("CA-325: el texto no nombra la accion %q:\n%s", label, txt)
			}
		}
	}
}

// CA-325: con el binario real, hoom board --json y hoom item show --json
// traen actions, drops y ghost (la misma tarjeta en los dos), sin null, y el
// texto no nombra acciones.
func TestCA325_E2EBoardYShowTraenAccionesDropsYGhost(t *testing.T) {
	bin := bxBinario(t)
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "pedido: el pedido de precios\n")
	bdCommitear(t, root, "item")

	revisar := func(etapa string, conEspacio bool) {
		t.Helper()
		r := bxCorrer(t, bin, root, "board", "--json")
		bxOK(t, "CA-325", r, "board", "--json")
		var b struct {
			Columns []struct {
				Cards []map[string]any `json:"cards"`
			} `json:"columns"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &b); err != nil {
			t.Fatalf("CA-325: [%s] board --json es JSON: %v\n%s", etapa, err, r.stdout)
		}
		var deBoard map[string]any
		for _, col := range b.Columns {
			for _, c := range col.Cards {
				if c["slug"] == bdSlug {
					deBoard = c
				}
			}
		}
		if deBoard == nil {
			t.Fatalf("CA-325: [%s] el tablero trae la tarjeta:\n%s", etapa, r.stdout)
		}
		r = bxCorrer(t, bin, root, "item", "show", bdSlug, "--json")
		bxOK(t, "CA-325", r, "item", "show", bdSlug, "--json")
		var show struct {
			Card map[string]any `json:"card"`
		}
		if err := json.Unmarshal([]byte(r.stdout), &show); err != nil || show.Card == nil {
			t.Fatalf("CA-325: [%s] item show --json trae la tarjeta: %v\n%s", etapa, err, r.stdout)
		}
		for nombre, c := range map[string]map[string]any{"board": deBoard, "item show": show.Card} {
			raw, _ := json.Marshal(c)
			acJSONBien(t, "CA-325", etapa+", "+nombre, c, string(raw))
			if c["ghost"] != nil {
				t.Fatalf("CA-325: [%s, %s] sin nada en curso ghost es null: %s", etapa, nombre, raw)
			}
			var ids []string
			for _, x := range c["actions"].([]any) {
				a, _ := x.(map[string]any)
				id, _ := a["id"].(string)
				ids = append(ids, id)
			}
			want := []string{ActPedirArquitecto}
			if u, _ := c["unsynced"].([]any); len(u) > 0 {
				want = append(want, ActGuardar)
			}
			if conEspacio {
				want = append(want, ActSesion, ActTerminal)
			}
			if !reflect.DeepEqual(ids, want) {
				t.Fatalf("CA-325: [%s, %s] las acciones de la tarjeta en Backlog son %v, fueron %v", etapa, nombre, want, ids)
			}
		}
		x, _ := json.Marshal(deBoard)
		y, _ := json.Marshal(show.Card)
		if string(x) != string(y) {
			t.Fatalf("CA-325: [%s] board y item show dan la misma tarjeta:\nboard: %s\nshow:  %s", etapa, x, y)
		}
		for _, args := range [][]string{{"board"}, {"item", "show", bdSlug}} {
			r := bxCorrer(t, bin, root, args...)
			bxOK(t, "CA-325", r, args...)
			for _, label := range []string{"Pedir al", "Volver a lanzar", "Abrir sesion", "Ver terminal", "Descartar cambios"} {
				if strings.Contains(r.stdout, label) {
					t.Fatalf("CA-325: [%s] el texto de hoom %s no cambia (no nombra %q):\n%s", etapa, strings.Join(args, " "), label, r.stdout)
				}
			}
		}
	}
	revisar("sin tarea", false)
	bxOK(t, "CA-325", bxCorrer(t, bin, root, "task", "start", bdSlug), "task", "start")
	revisar("con tarea", true)
}

// acConEnCurso le suma a la tarjeta un run suelto vivo del rol.
func acConEnCurso(ev Evidence, rol string) Evidence {
	ev.Runs = append(ev.Runs, acRunVivo(rol, "claude"))
	return ev
}

// CA-326: con running, toda accion menos sesion y terminal esta
// deshabilitada con "espera a que termine el <rol> que esta trabajando"; sin
// rol, "el agente". sesion y terminal siguen habilitadas.
func TestCA326_EnCursoDeshabilitaTodoMenosSesionYTerminal(t *testing.T) {
	type caso struct {
		nombre string
		ev     Evidence
		rol    string
	}
	var casos []caso
	for _, cs := range acColumnas() {
		ev := acConEnCurso(cs.ev, "writer")
		ev.Uncommitted = acSucios()
		casos = append(casos, caso{cs.nombre + " con un writer en curso", ev, "writer"})
	}
	for _, cs := range acColumnasArbol() {
		// con el item sin guardar, para que toda tarjeta tenga al menos guardar
		ev := acConEnCurso(cs.ev, "")
		ev.Uncommitted = []string{".hoom/items/precios.yaml"}
		casos = append(casos, caso{cs.nombre + " con un run sin rol en curso", ev, "agente"})
	}
	ev := acConEnCurso(acInterrumpido(acWriter(), "writer", "claude"), "writer")
	ev.Uncommitted = acSucios()
	casos = append(casos, caso{"interrumpida con otro writer en curso", ev, "writer"})
	ev = acEv()
	ev.Approval = approval.StatusNotApproved
	ev.Envelopes = []EnvelopeState{acSobreVivo("arquitecto", "claude", "run", 2, 5)}
	casos = append(casos, caso{"arquitecto corrigiendo en tu aprobacion", ev, "arquitecto"})
	ev = acTestWriter()
	ev.Envelopes = []EnvelopeState{acSobreVivo("test-writer", "claude", "verify", 5, 6)}
	casos = append(casos, caso{"test-writer en curso", ev, "test-writer"})

	vistas := map[string]bool{}
	for _, cs := range casos {
		c := Derive(cs.ev)
		if c.Running == nil {
			t.Fatalf("CA-326: [%s] el fixture tiene algo en curso: %+v", cs.nombre, c)
		}
		if len(c.Actions) == 0 {
			t.Fatalf("CA-326: [%s] la tarjeta en curso sigue teniendo acciones (deshabilitadas): %+v", cs.nombre, c)
		}
		acBienFormada(t, "CA-326", cs.nombre, c)
		for _, a := range c.Actions {
			vistas[a.ID] = true
			if a.ID == ActSesion || a.ID == ActTerminal {
				acHabilitada(t, "CA-326: ["+cs.nombre+"]", a)
				continue
			}
			acDeshabilitada(t, "CA-326: ["+cs.nombre+"]", a, acEspera(cs.rol))
		}
	}
	for id := range acEtiquetas {
		if !vistas[id] {
			t.Fatalf("CA-326: los fixtures en curso recorren todas las acciones; falto %s", id)
		}
	}
}

// CA-326: sin espacio de trabajo propio y fuera de Backlog, las acciones de
// rol estan deshabilitadas con su frase; aprobar y guardar no.
func TestCA326_SinEspacioPropioFueraDeBacklog(t *testing.T) {
	for _, cs := range acColumnasArbol() {
		c := Derive(cs.ev)
		acBienFormada(t, "CA-326", cs.nombre, c)
		for _, a := range c.Actions {
			switch {
			case acDeRol[a.ID] && c.Column != ColBacklog:
				acDeshabilitada(t, "CA-326: ["+cs.nombre+"]", a, acSinEspacio)
			default:
				acHabilitada(t, "CA-326: ["+cs.nombre+"]", a)
			}
		}
	}
	// reanudar y volver a lanzar tambien son de rol
	ev := bdArbol(acInterrumpido(acWriter(), "writer", "claude"))
	ev.Uncommitted = []string{".hoom/items/precios.yaml"}
	c := Derive(ev)
	for _, id := range []string{ActPedirWriter, ActReanudar, ActRelanzar} {
		acDeshabilitada(t, "CA-326", acAccion(t, "CA-326", c, id), acSinEspacio)
	}
	acHabilitada(t, "CA-326", acAccion(t, "CA-326", c, ActGuardar))

	// en Backlog, sin espacio propio, se le pide al arquitecto (crea la tarea)
	ev = bdArbol(acEv())
	ev.SpecExists = false
	acHabilitada(t, "CA-326", acAccion(t, "CA-326", Derive(ev), ActPedirArquitecto))
	// y con espacio propio en cualquier columna, habilitadas
	for _, cs := range acColumnas() {
		for _, a := range Derive(cs.ev).Actions {
			acHabilitada(t, "CA-326: ["+cs.nombre+"]", a)
		}
	}
}

// CA-326: los casos se evaluan en el orden de la tabla y gana el primero.
func TestCA326_OrdenDeLosCasos(t *testing.T) {
	// en curso gana sobre sin espacio propio y sobre el presupuesto agotado
	ev := acConEnCurso(bdArbol(acConGasto(acWriter(), 5, 6)), "writer")
	acDeshabilitada(t, "CA-326", acAccion(t, "CA-326", Derive(ev), ActPedirWriter), acEspera("writer"))

	// sin espacio propio gana sobre el presupuesto agotado
	ev = bdArbol(acConGasto(acWriter(), 5, 6))
	acDeshabilitada(t, "CA-326", acAccion(t, "CA-326", Derive(ev), ActPedirWriter), acSinEspacio)

	// en Backlog no se pide espacio propio: manda el presupuesto
	ev = bdArbol(acConGasto(acEv(), 5, 6))
	ev.SpecExists = false
	acDeshabilitada(t, "CA-326", acAccion(t, "CA-326", Derive(ev), ActPedirArquitecto), acAgotado("6", "5"))

	// el presupuesto agotado gana sobre no tener proveedor
	ev = acConGasto(acWriter(), 5, 6)
	ev.Providers = nil
	acDeshabilitada(t, "CA-326", acAccion(t, "CA-326", Derive(ev), ActPedirWriter), acAgotado("6", "5"))

	// ningun provider instalado: la accion esta, deshabilitada y sin opciones
	ev = acWriter()
	ev.Providers = nil
	a := acAccion(t, "CA-326", Derive(ev), ActPedirWriter)
	acDeshabilitada(t, "CA-326", a, acNingunProv)
	if a.Providers == nil || len(a.Providers) != 0 {
		t.Fatalf("CA-326: sin providers instalados las opciones son [] (no nil): %#v", a.Providers)
	}
	// uno instalado que no sirve: la accion lo lista y dice por que no
	ev = acWriter()
	ev.Providers = []providers.Info{acProvs()[1], acProvs()[3]}
	a = acAccion(t, "CA-326", Derive(ev), ActPedirWriter)
	acDeshabilitada(t, "CA-326", a, acNingunProv)
	acOpciones(t, "CA-326", a, acOp{"opencode", false, acOpenSinCtto, false, 0, false})

	// reanudar: sin proveedor gana sobre sus casos propios (sin run_id)
	ev = acSinRunID()
	ev.Providers = nil
	acDeshabilitada(t, "CA-326", acAccion(t, "CA-326", Derive(ev), ActReanudar), acNingunProv)
	// y en curso gana sobre todos los de reanudar
	ev = acConEnCurso(acSinRunID(), "")
	acDeshabilitada(t, "CA-326", acAccion(t, "CA-326", Derive(ev), ActReanudar), acEspera("agente"))
}

// CA-326: en TODOS los fixtures, un why no vacio (de accion, de opcion de
// provider o de drop) no dice git, commit, worktree, merge, rama, branch,
// diff, HEAD ni huella (enteras, sin distinguir mayusculas), ni .hoom/ ni
// "hoom ". why es "" si y solo si la accion esta habilitada (o la opcion ok).
func TestCA326_PalabrasProhibidasEnTodosLosWhy(t *testing.T) {
	vistos := 0
	for _, f := range acTodos() {
		c := Derive(f.ev)
		for _, a := range c.Actions {
			if a.Enabled != (a.Why == "") {
				t.Fatalf("CA-326: [%s] why es \"\" si y solo si %s esta habilitada: enabled=%v why=%q", f.nombre, a.ID, a.Enabled, a.Why)
			}
			if a.Why != "" {
				tbNormal(t, "CA-326", fmt.Sprintf("[%s] el why de %s", f.nombre, a.ID), a.Why)
				vistos++
			}
			for _, p := range a.Providers {
				if p.OK != (p.Why == "") {
					t.Fatalf("CA-326: [%s] why de una opcion es \"\" si y solo si es ok: %s/%s %+v", f.nombre, a.ID, p.Name, p)
				}
				if p.Why != "" {
					tbNormal(t, "CA-326", fmt.Sprintf("[%s] el why de la opcion %s de %s", f.nombre, p.Name, a.ID), p.Why)
					vistos++
				}
			}
		}
		for _, d := range c.Drops {
			if (d.Action == "") == (d.Why == "") {
				t.Fatalf("CA-326: [%s] un drop lleva una accion o un why, no los dos ni ninguno: %+v", f.nombre, d)
			}
			if d.Why != "" {
				tbNormal(t, "CA-326", fmt.Sprintf("[%s] el why del drop en %s", f.nombre, d.Column), d.Why)
				vistos++
			}
		}
	}
	if vistos < 100 {
		t.Fatalf("CA-326: los fixtures tienen que producir frases que revisar (al menos 100), vi %d", vistos)
	}
}

// acTodos son todos los fixtures de CA-325..CA-332.
func acTodos() []acFix {
	var out []acFix
	for _, cs := range append(acColumnas(), acColumnasArbol()...) {
		out = append(out, acFix{cs.nombre, cs.ev})
		ev := acConEnCurso(cs.ev, "writer")
		ev.Uncommitted = acSucios()
		out = append(out, acFix{cs.nombre + " en curso", ev})
		out = append(out, acFix{cs.nombre + " con presupuesto agotado", acConGasto(cs.ev, 5, 6)})
		out = append(out, acFix{cs.nombre + " con presupuesto justo", acConGasto(cs.ev, 5, 4.8)})
		sin := cs.ev
		sin.Providers = nil
		out = append(out, acFix{cs.nombre + " sin providers", sin})
	}
	out = append(out, acReanudarCasos()...)
	out = append(out, acReviewCasos()...)
	out = append(out, acFantasmaCasos()...)
	return out
}

// CA-327: spend.remaining_usd es el presupuesto menos el costo reportado (un
// costo null cuenta 0), puede ser negativo, y es null sin presupuesto.
func TestCA327_RemainingUSD(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		ev     Evidence
		want   *float64
	}{
		{"sin presupuesto", acWriter(), nil},
		{"sin presupuesto y con gasto", func() Evidence {
			ev := acConGasto(acWriter(), 5, 0.8)
			ev.Item.PresupuestoUSD = nil
			return ev
		}(), nil},
		{"presupuesto sin runs", func() Evidence { ev := acWriter(); ev.Item.PresupuestoUSD = bdF(5); return ev }(), bdF(5)},
		{"presupuesto con runs sin costo", func() Evidence {
			ev := acWriter()
			ev.Item.PresupuestoUSD = bdF(5)
			ev.Runs = []RunState{acGasto("run-codex", "writer", "codex", nil, time.Minute)}
			return ev
		}(), bdF(5)},
		{"gastado en parte", acConGasto(acWriter(), 5, 0.8), bdF(4.2)},
		{"gastado de mas", acConGasto(acWriter(), 5, 6), bdF(-1)},
		{"gastado justo", acConGasto(acWriter(), 5, 5), bdF(0)},
	} {
		s := Derive(caso.ev).Spend
		switch {
		case caso.want == nil && s.RemainingUSD != nil:
			t.Fatalf("CA-327: [%s] sin presupuesto remaining_usd es null, fue %v", caso.nombre, *s.RemainingUSD)
		case caso.want != nil && (s.RemainingUSD == nil || !bdCasi(*s.RemainingUSD, *caso.want)):
			t.Fatalf("CA-327: [%s] remaining_usd es %v, fue %v", caso.nombre, *caso.want, s.RemainingUSD)
		}
	}
	_, raw := acJSON(t, Derive(acWriter()))
	if !strings.Contains(raw, `"remaining_usd":null`) {
		t.Fatalf("CA-327: sin presupuesto, \"remaining_usd\":null: %s", raw)
	}
	_, raw = acJSON(t, Derive(acConGasto(acWriter(), 5, 6)))
	if !strings.Contains(raw, `"remaining_usd":-1`) {
		t.Fatalf("CA-327: gastado de mas, \"remaining_usd\":-1: %s", raw)
	}
}

// CA-327: con remaining_usd <= 0 las acciones de rol dicen que se agoto el
// presupuesto, con lo gastado y el presupuesto; las demas no.
func TestCA327_PresupuestoAgotado(t *testing.T) {
	for _, caso := range []struct {
		nombre  string
		costo   float64
		gastado string
	}{{"gastado de mas", 6, "6"}, {"gastado justo", 5, "5"}, {"con decimales", 5.25, "5.25"}} {
		ev := acConGasto(acInterrumpido(acWriter(), "writer", "claude"), 5, caso.costo)
		ev.Uncommitted = acSucios()
		c := Derive(ev)
		for _, id := range []string{ActPedirWriter, ActReanudar, ActRelanzar} {
			acDeshabilitada(t, "CA-327: ["+caso.nombre+"]", acAccion(t, "CA-327", c, id), acAgotado(caso.gastado, "5"))
		}
		for _, id := range []string{ActDescartar, ActGuardar, ActSesion, ActTerminal} {
			acHabilitada(t, "CA-327: ["+caso.nombre+"]", acAccion(t, "CA-327", c, id))
		}
	}
	// en Tu aprobacion: aprobar sigue, pedir al arquitecto no
	ev := acConGasto(acEv(), 5, 6)
	ev.Approval = approval.StatusNotApproved
	c := Derive(ev)
	acHabilitada(t, "CA-327", acAccion(t, "CA-327", c, ActAprobar))
	acDeshabilitada(t, "CA-327", acAccion(t, "CA-327", c, ActPedirArquitecto), acAgotado("6", "5"))
	// en Tu aceptacion integrar no depende del presupuesto
	acHabilitada(t, "CA-327", acAccion(t, "CA-327", Derive(acConGasto(acEv(), 5, 6)), ActIntegrar))
}

type acOp struct {
	name   string
	ok     bool
	why    string
	budget bool
	min    float64
	def    bool
}

// acOpciones exige las opciones de provider de una accion, en orden.
func acOpciones(t *testing.T, ca string, a Action, want ...acOp) {
	t.Helper()
	if len(a.Providers) != len(want) {
		t.Fatalf("%s: %s trae %d opciones de provider, trajo %+v", ca, a.ID, len(want), a.Providers)
	}
	for i, w := range want {
		g := a.Providers[i]
		if g.Name != w.name || g.OK != w.ok || g.Why != w.why || g.Budget != w.budget ||
			!bdCasi(g.MinBudgetUSD, w.min) || g.Default != w.def {
			t.Fatalf("%s: la opcion %d de %s debe ser %+v, fue %+v", ca, i, a.ID, w, g)
		}
	}
}

// acBudget exige el budget_usd propuesto (nil = sin tope).
func acBudget(t *testing.T, ca string, a Action, want *float64) {
	t.Helper()
	switch {
	case want == nil && a.BudgetUSD != nil:
		t.Fatalf("%s: sin presupuesto en el item, el budget_usd propuesto de %s es null, fue %v", ca, a.ID, *a.BudgetUSD)
	case want != nil && (a.BudgetUSD == nil || !bdCasi(*a.BudgetUSD, *want)):
		t.Fatalf("%s: el budget_usd propuesto de %s es %v, fue %v", ca, a.ID, *want, a.BudgetUSD)
	}
}

// CA-327: una opcion por provider instalado, en el orden del registro, con
// budget, min_budget_usd, ok/why y default en la primera ok; por debajo del
// minimo, "quedan <r> USD y <p> necesita al menos <m> USD por run".
func TestCA327_OpcionesDeProvider(t *testing.T) {
	// sin presupuesto: el minimo no aplica, y no hay tope que proponer
	a := acAccion(t, "CA-327", Derive(acWriter()), ActPedirWriter)
	acOpciones(t, "CA-327", a,
		acOp{"claude", true, "", true, 0.5, true},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, false})
	acBudget(t, "CA-327", a, nil)
	acHabilitada(t, "CA-327", a)

	// quedan 4.2: claude alcanza, se propone lo que queda
	a = acAccion(t, "CA-327", Derive(acConGasto(acWriter(), 5, 0.8)), ActPedirWriter)
	acOpciones(t, "CA-327", a,
		acOp{"claude", true, "", true, 0.5, true},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, false})
	acBudget(t, "CA-327", a, bdF(4.2))

	// quedan 0.2 (5 de presupuesto, 4.8 gastados): claude no, codex si y es
	// el default
	a = acAccion(t, "CA-327", Derive(acConGasto(acWriter(), 5, 4.8)), ActPedirWriter)
	acOpciones(t, "CA-327", a,
		acOp{"claude", false, "quedan 0.2 USD y claude necesita al menos 0.5 USD por run", true, 0.5, false},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, true})
	acBudget(t, "CA-327", a, bdF(0.2))
	acHabilitada(t, "CA-327", a)

	// quedan 0.5 justos: no es menor que el minimo
	a = acAccion(t, "CA-327", Derive(acConGasto(acWriter(), 5, 4.5)), ActPedirWriter)
	if a.Providers[0].Name != "claude" || !a.Providers[0].OK || !a.Providers[0].Default {
		t.Fatalf("CA-327: con 0.5 USD justos claude sigue ok: %+v", a.Providers)
	}

	// el minimo sale de providers.Info, no del nombre: un claude sin minimo
	// declarado pasa con 0.2
	ev := acConGasto(acWriter(), 5, 4.8)
	ev.Providers[0].MinBudgetUSD = 0
	a = acAccion(t, "CA-327", Derive(ev), ActPedirWriter)
	if !a.Providers[0].OK || a.Providers[0].MinBudgetUSD != 0 {
		t.Fatalf("CA-327: min_budget_usd es el que declara el provider: %+v", a.Providers[0])
	}
	// y la capacidad budget tambien: un codex con tope es budget true
	ev = acWriter()
	ev.Providers[2].Capabilities.Budget = true
	a = acAccion(t, "CA-327", Derive(ev), ActPedirWriter)
	if !a.Providers[2].Budget {
		t.Fatalf("CA-327: budget es la capacidad budget del provider: %+v", a.Providers[2])
	}

	// en todas las acciones de rol de todas las columnas, las mismas opciones
	for _, cs := range acColumnas() {
		c := Derive(cs.ev)
		for _, a := range c.Actions {
			if acDeRol[a.ID] && a.ID != ActPedirReviewer {
				acOpciones(t, "CA-327: ["+cs.nombre+"]", a,
					acOp{"claude", true, "", true, 0.5, true},
					acOp{"opencode", false, acOpenSinCtto, false, 0, false},
					acOp{"codex", true, "", false, 0, false})
			}
		}
	}
}

// acReviewCasos: la tarjeta en Review con la review exigida, con distintos
// writers y presupuestos.
func acReviewCasos() []acFix {
	var out []acFix
	ev := acGrande(acEv())
	ev.Runs = []RunState{acGasto("run-w", "writer", "claude", nil, 5*time.Minute)}
	out = append(out, acFix{"review con writer claude", ev})
	ev = acGrande(acEv())
	ev.Item.PresupuestoUSD = bdF(5)
	ev.Runs = []RunState{acGasto("run-w", "writer", "codex", nil, 5*time.Minute),
		acGasto("run-s", "scout", "claude", bdF(3.4), 6*time.Minute)}
	out = append(out, acFix{"review por lente sin alcanzar", ev})
	ev = acGrande(acEv())
	ev.Runs = []RunState{acGasto("run-w", "writer", "claude", nil, 5*time.Minute)}
	ev.Item.Sesiones = []item.Sesion{{Provider: "codex", AbiertaPor: acSigner, AbiertaEn: bdT0}}
	out = append(out, acFix{"review con writer declarado", ev})
	return out
}

// CA-327: en la review lo que queda para un run es remaining_usd / 4 (el
// presupuesto es por lente), y un writer de la tarjeta (observado o
// declarado) no puede revisar: "<p> escribio el codigo: la revision tiene
// que ser de otro proveedor".
func TestCA327_ReviewPorLenteYSinElWriter(t *testing.T) {
	escribio := func(p string) string { return p + " escribio el codigo: la revision tiene que ser de otro proveedor" }

	// writer observado claude, sin presupuesto: codex revisa
	ev := acGrande(acEv())
	ev.Runs = []RunState{acGasto("run-w", "writer", "claude", nil, 5*time.Minute)}
	a := acAccion(t, "CA-327", Derive(ev), ActPedirReviewer)
	acOpciones(t, "CA-327", a,
		acOp{"claude", false, escribio("claude"), true, 0.5, false},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, true})
	acBudget(t, "CA-327", a, nil)
	acHabilitada(t, "CA-327", a)

	// writer observado codex, quedan 1.6: 0.4 por lente no alcanza a claude,
	// y no queda nadie
	ev = acGrande(acEv())
	ev.Item.PresupuestoUSD = bdF(5)
	ev.Runs = []RunState{acGasto("run-w", "writer", "codex", nil, 5*time.Minute),
		acGasto("run-s", "scout", "claude", bdF(3.4), 6*time.Minute)}
	c := Derive(ev)
	if c.Spend.RemainingUSD == nil || !bdCasi(*c.Spend.RemainingUSD, 1.6) {
		t.Fatalf("CA-327: el fixture deja 1.6 USD: %v", c.Spend.RemainingUSD)
	}
	a = acAccion(t, "CA-327", c, ActPedirReviewer)
	acOpciones(t, "CA-327", a,
		acOp{"claude", false, "quedan 0.4 USD y claude necesita al menos 0.5 USD por run", true, 0.5, false},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", false, escribio("codex"), false, 0, false})
	acBudget(t, "CA-327", a, bdF(0.4))
	acDeshabilitada(t, "CA-327", a, acNingunProv)

	// quedan 2: 0.5 por lente alcanza
	ev.Runs[1] = acGasto("run-s", "scout", "claude", bdF(3), 6*time.Minute)
	a = acAccion(t, "CA-327", Derive(ev), ActPedirReviewer)
	acOpciones(t, "CA-327", a,
		acOp{"claude", true, "", true, 0.5, true},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", false, escribio("codex"), false, 0, false})
	acBudget(t, "CA-327", a, bdF(0.5))
	acHabilitada(t, "CA-327", a)

	// writer declarado (una sesion de codex) ademas del observado claude:
	// ninguno de los dos revisa
	ev = acGrande(acEv())
	ev.Runs = []RunState{acGasto("run-w", "writer", "claude", nil, 5*time.Minute)}
	ev.Item.Sesiones = []item.Sesion{{Provider: "codex", AbiertaPor: acSigner, AbiertaEn: bdT0}}
	a = acAccion(t, "CA-327", Derive(ev), ActPedirReviewer)
	acOpciones(t, "CA-327", a,
		acOp{"claude", false, escribio("claude"), true, 0.5, false},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", false, escribio("codex"), false, 0, false})
	acDeshabilitada(t, "CA-327", a, acNingunProv)

	// solo declarado, sin writer observado: el declarado tampoco revisa
	ev = acGrande(acEv())
	ev.Item.Sesiones = []item.Sesion{{Provider: "claude", AbiertaPor: acSigner, AbiertaEn: bdT0}}
	a = acAccion(t, "CA-327", Derive(ev), ActPedirReviewer)
	acOpciones(t, "CA-327", a,
		acOp{"claude", false, escribio("claude"), true, 0.5, false},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, true})

	// en Review con hallazgos, pedirle al writer NO excluye al writer, y su
	// presupuesto propuesto es lo que queda entero; el del reviewer, un cuarto
	ev = acConHallazgos(acGrande(acEv()), "f-a")
	ev.Item.PresupuestoUSD = bdF(5)
	ev.Runs = []RunState{acGasto("run-w", "writer", "claude", bdF(1), 5*time.Minute)}
	c = Derive(ev)
	w := acAccion(t, "CA-327", c, ActPedirWriter)
	acOpciones(t, "CA-327", w,
		acOp{"claude", true, "", true, 0.5, true},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, false})
	acBudget(t, "CA-327", w, bdF(4))
	acBudget(t, "CA-327", acAccion(t, "CA-327", c, ActPedirReviewer), bdF(1))

	// volver a lanzar una review cortada: tambien sin el writer, y por lente
	ev = acRevisorCortado()
	ev.Item.PresupuestoUSD = bdF(5)
	r := acAccion(t, "CA-327", Derive(ev), ActRelanzar)
	acOpciones(t, "CA-327", r,
		acOp{"claude", false, escribio("claude"), true, 0.5, false},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, true})
	acBudget(t, "CA-327", r, bdF(1.25))
}

// CA-329: el pedido propuesto de cada accion es el texto del contrato.
func TestCA329_PedidoPropuesto(t *testing.T) {
	pedido := func(ev Evidence, id string) string {
		t.Helper()
		return acAccion(t, "CA-329", Derive(ev), id).Pedido
	}
	igual := func(que, got, want string) {
		t.Helper()
		if got != want {
			t.Fatalf("CA-329: el pedido de %s es %q, fue %q", que, want, got)
		}
	}
	// arquitecto, con y sin pedido en el item, en Backlog, Arquitecto y Tu
	// aprobacion
	ev := acEv()
	ev.SpecExists = false
	igual("pedir-arquitecto con pedido", pedido(ev, ActPedirArquitecto), acPedArqConP)
	ev.Item.Pedido = ""
	igual("pedir-arquitecto sin pedido", pedido(ev, ActPedirArquitecto), acPedArq)
	ev = bdArbol(acEv())
	ev.SpecExists = false
	igual("pedir-arquitecto sin espacio propio", pedido(ev, ActPedirArquitecto), acPedArqConP)
	ev = acEv()
	ev.LintIssues = []string{`falta la seccion "riesgos"`}
	igual("pedir-arquitecto en Arquitecto", pedido(ev, ActPedirArquitecto), acPedArqConP)
	ev = acEv()
	ev.Approval = approval.StatusInvalidated
	igual("pedir-arquitecto en Tu aprobacion", pedido(ev, ActPedirArquitecto), acPedArqConP)
	ev = acEv()
	ev.SpecExists = false
	ev.Item.Titulo = "Otra tarjeta"
	ev.Item.Pedido = "hacer otra cosa"
	igual("pedir-arquitecto de otro item", pedido(ev, ActPedirArquitecto),
		`Escribi el spec .hoom/specs/precios.md de la tarjeta "Otra tarjeta".`+"\nPedido: hacer otra cosa")

	// test-writer: los criterios sin prueba, en el orden del spec
	igual("pedir-test-writer", pedido(acTestWriter(), ActPedirTestWriter), acPedTW)
	ev = acEv()
	ev.Untraced = []string{"CA-2"}
	igual("pedir-test-writer con un criterio", pedido(ev, ActPedirTestWriter),
		"Escribi los tests que citan los criterios de .hoom/specs/precios.md que todavia no tienen prueba: CA-2.")

	// writer, y en Review con los hallazgos que bloquean (no el medium)
	igual("pedir-writer", pedido(acWriter(), ActPedirWriter), acPedWriter)
	ev = acConHallazgos(acEv(), "f-a", "f-b")
	ev.Findings = append(ev.Findings, bdHallazgo("f-m", "medium", bdSlug))
	igual("pedir-writer en Review", pedido(ev, ActPedirWriter), "Corregi los hallazgos que bloquean la tarjeta: f-a, f-b.")

	// la review arma su propio pedido
	igual("pedir-reviewer", pedido(acGrande(acEv()), ActPedirReviewer), "")

	// reanudar: el paso donde se corto
	igual("reanudar", pedido(acReanudable(), ActReanudar), acPedReanudar)
	ev = acReanudable()
	ev.Envelopes[0].Record.Stage, ev.Envelopes[0].Record.Step = "verify", 4
	igual("reanudar en verify", pedido(ev, ActReanudar),
		"El trabajo anterior se corto en el paso 4 de 5 (verify). Revisa el arbol como quedo y termina lo que se te pidio.")

	// volver a lanzar: el de pedir-<rol> para los cuatro roles, "" para otro
	ev = acEv()
	ev.LintIssues = []string{`falta la seccion "riesgos"`}
	igual("relanzar de un arquitecto", pedido(acInterrumpido(ev, "arquitecto", "claude"), ActRelanzar), acPedArqConP)
	igual("relanzar de un test-writer", pedido(acInterrumpido(acTestWriter(), "test-writer", "claude"), ActRelanzar), acPedTW)
	igual("relanzar de un writer", pedido(acReanudable(), ActRelanzar), acPedWriter)
	igual("relanzar de un reviewer", pedido(acRevisorCortado(), ActRelanzar), "")
	igual("relanzar de un scout", pedido(acInterrumpido(acWriter(), "scout", "claude"), ActRelanzar), "")
}

// acReanudable: Writer, con un writer de claude interrumpido que dejo sesion.
func acReanudable() Evidence { return acInterrumpido(acWriter(), "writer", "claude") }

// acSinRunID: el sobre murio antes del paso run.
func acSinRunID() Evidence {
	ev := acReanudable()
	ev.Envelopes[0].Record.RunID, ev.Envelopes[0].Record.Stage, ev.Envelopes[0].Record.Step = "", "spec", 1
	ev.Runs = nil
	return ev
}

// acSinSesionEnSidecar: el run existe pero no dejo id de sesion.
func acSinSesionEnSidecar() Evidence {
	ev := acReanudable()
	ev.Runs[0].Meta.ProviderSessionID = ""
	return ev
}

// acSinSidecar: el sobre nombra un run cuyo sidecar no esta.
func acSinSidecar() Evidence {
	ev := acReanudable()
	ev.Runs = nil
	return ev
}

// acCiego: un test-writer ciego interrumpido en su paso run (4 de 6).
func acCiego() Evidence {
	ev := acInterrumpido(acTestWriter(), "test-writer", "claude")
	r := &ev.Envelopes[0].Record
	r.Isolated, r.IsolatedFrom, r.Step, r.Steps = true, "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c", 4, 6
	ev.Runs[0].Meta.Isolated = true
	return ev
}

// acRevisorCortado: la review exigida de un writer claude, cortada con codex
// en la lente 2 de 4.
func acRevisorCortado() Evidence {
	ev := acGrande(acEv())
	ev.Runs = []RunState{acGasto("run-w", "writer", "claude", nil, 5*time.Minute)}
	ev = acInterrumpido(ev, "reviewer", "codex")
	ev.Envelopes[0].Record.Step, ev.Envelopes[0].Record.Steps = 2, 4
	return ev
}

// acSinResume: el provider del sobre (aider) recibe el contrato del rol pero
// no puede retomar una sesion.
func acSinResume() Evidence {
	ev := acWriter()
	ev.Providers = append(ev.Providers, providers.Info{Name: "aider", Installed: true, Bin: "/opt/bin/aider",
		Capabilities: providers.Capabilities{SystemPrompt: true}})
	return acInterrumpido(ev, "writer", "aider")
}

func acReanudarCasos() []acFix {
	return []acFix{
		{"reanudable", acReanudable()},
		{"sin run_id", acSinRunID()},
		{"sin sesion en el sidecar", acSinSesionEnSidecar()},
		{"sin sidecar", acSinSidecar()},
		{"ciego", acCiego()},
		{"reviewer cortado", acRevisorCortado()},
		{"provider sin resume", acSinResume()},
		{"interrumpido de codex", acInterrumpido(acWriter(), "writer", "codex")},
		{"arquitecto interrumpido en backlog", func() Evidence {
			ev := acEv()
			ev.SpecExists = false
			return acInterrumpido(ev, "arquitecto", "claude")
		}()},
	}
}

// CA-330: reanudar trae el resume_id del sidecar del run del sobre
// interrumpido y una sola opcion de provider, la de ese sobre. relanzar no
// trae resume_id y ofrece todos los instalados, con default en el del sobre.
func TestCA330_ReanudarTraeLaSesionYSuProvider(t *testing.T) {
	c := Derive(acReanudable())
	if c.Interrupted == nil || c.Interrupted.EnvelopeID != "env-caido" {
		t.Fatalf("CA-330: el fixture esta interrumpido: %+v", c.Interrupted)
	}
	re := acAccion(t, "CA-330", c, ActReanudar)
	acHabilitada(t, "CA-330", re)
	if re.ResumeID != "sesion-abc123" || re.Role != "writer" {
		t.Fatalf("CA-330: reanudar trae el id de sesion del sidecar (sesion-abc123) y el rol del sobre: %+v", re)
	}
	acOpciones(t, "CA-330", re, acOp{"claude", true, "", true, 0.5, true})

	rl := acAccion(t, "CA-330", c, ActRelanzar)
	acHabilitada(t, "CA-330", rl)
	if rl.ResumeID != "" || rl.Role != "writer" {
		t.Fatalf("CA-330: volver a lanzar no retoma ninguna sesion: %+v", rl)
	}
	acOpciones(t, "CA-330", rl,
		acOp{"claude", true, "", true, 0.5, true},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, false})

	// el sobre era de codex: reanudar solo con codex, y relanzar propone codex
	c = Derive(acInterrumpido(acWriter(), "writer", "codex"))
	re = acAccion(t, "CA-330", c, ActReanudar)
	acHabilitada(t, "CA-330", re)
	acOpciones(t, "CA-330", re, acOp{"codex", true, "", false, 0, true})
	acOpciones(t, "CA-330", acAccion(t, "CA-330", c, ActRelanzar),
		acOp{"claude", true, "", true, 0.5, false},
		acOp{"opencode", false, acOpenSinCtto, false, 0, false},
		acOp{"codex", true, "", false, 0, true})

	// el rol es el del sobre interrumpido, no el de la columna
	c = Derive(acInterrumpido(acWriter(), "scout", "claude"))
	if acAccion(t, "CA-330", c, ActReanudar).Role != "scout" || acAccion(t, "CA-330", c, ActRelanzar).Role != "scout" {
		t.Fatalf("CA-330: reanudar y relanzar llevan el rol del sobre interrumpido (scout): %+v", c.Actions)
	}
}

// CA-330: reanudar esta deshabilitada con la frase del contrato sin run_id,
// sin id de sesion en el sidecar, con un sobre ciego, con el rol reviewer y
// con un provider sin la capacidad resume; relanzar esta disponible en los
// cinco casos.
func TestCA330_ReanudarDeshabilitadaYRelanzarDisponible(t *testing.T) {
	for _, caso := range []struct {
		nombre string
		ev     Evidence
		why    string
	}{
		{"sin run_id", acSinRunID(), acAntesDelRun},
		{"sin id de sesion en el sidecar", acSinSesionEnSidecar(), acSinSesion},
		{"sin sidecar del run", acSinSidecar(), acSinSesion},
		{"sobre ciego", acCiego(), "el test-writer trabaja a ciegas y su espacio ciego ya no existe: solo se puede volver a lanzar"},
		{"rol reviewer", acRevisorCortado(), acRevisionYa},
		{"provider sin resume", acSinResume(), "aider no puede retomar una sesion: solo se puede volver a lanzar"},
	} {
		c := Derive(caso.ev)
		if c.Interrupted == nil {
			t.Fatalf("CA-330: [%s] el fixture esta interrumpido: %+v", caso.nombre, c)
		}
		acBienFormada(t, "CA-330", caso.nombre, c)
		re := acAccion(t, "CA-330: ["+caso.nombre+"]", c, ActReanudar)
		acDeshabilitada(t, "CA-330: ["+caso.nombre+"]", re, caso.why)
		if len(re.Providers) != 1 || re.Providers[0].Name != c.Interrupted.Provider {
			t.Fatalf("CA-330: [%s] reanudar ofrece solo el provider del sobre (%s): %+v", caso.nombre, c.Interrupted.Provider, re.Providers)
		}
		rl := acAccion(t, "CA-330: ["+caso.nombre+"]", c, ActRelanzar)
		acHabilitada(t, "CA-330: ["+caso.nombre+"]", rl)
		if rl.ResumeID != "" {
			t.Fatalf("CA-330: [%s] volver a lanzar no trae resume_id: %q", caso.nombre, rl.ResumeID)
		}
		def := ""
		for _, p := range rl.Providers {
			if p.Default {
				if def != "" {
					t.Fatalf("CA-330: [%s] una sola opcion es default: %+v", caso.nombre, rl.Providers)
				}
				def = p.Name
			}
		}
		if def != c.Interrupted.Provider {
			t.Fatalf("CA-330: [%s] volver a lanzar propone el provider del sobre (%s), propuso %q", caso.nombre, c.Interrupted.Provider, def)
		}
	}

	// el orden de reanudar: sin run_id gana sobre el rol reviewer
	ev := acRevisorCortado()
	ev.Envelopes[0].Record.RunID = ""
	acDeshabilitada(t, "CA-330", acAccion(t, "CA-330", Derive(ev), ActReanudar), acAntesDelRun)
	// sin sesion gana sobre ciego
	ev = acCiego()
	ev.Runs[0].Meta.ProviderSessionID = ""
	acDeshabilitada(t, "CA-330", acAccion(t, "CA-330", Derive(ev), ActReanudar), acSinSesion)
	// ciego gana sobre el provider sin resume
	ev = acCiego()
	ev.Providers[0].Capabilities.Resume = false
	acDeshabilitada(t, "CA-330", acAccion(t, "CA-330", Derive(ev), ActReanudar),
		"el test-writer trabaja a ciegas y su espacio ciego ya no existe: solo se puede volver a lanzar")
}
