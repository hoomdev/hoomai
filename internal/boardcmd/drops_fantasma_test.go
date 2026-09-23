// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-331, CA-332): lo que pasa al soltar la tarjeta en otra columna y el
// fantasma que aparece en la columna siguiente mientras un rol trabaja. Los
// dos salen de Derive (pura), asi que la pagina no decide nada: soltar abre
// la accion principal o dice por que no, y la columna la sigue dando la
// evidencia.
package boardcmd

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// acPrincipal es la accion principal de una lista de ids: la primera, si es
// una accion de columna.
func acPrincipal(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	switch ids[0] {
	case ActPedirArquitecto, ActPedirTestWriter, ActPedirWriter, ActPedirReviewer, ActAprobar, ActIntegrar:
		return ids[0]
	}
	return ""
}

func acIndice(col string) int {
	for i, c := range Columns {
		if c.ID == col {
			return i
		}
	}
	return -1
}

// acDropsEsperados arma los drops del contrato para una tarjeta.
func acDropsEsperados(c Card, principal string) []Drop {
	idx := acIndice(c.Column)
	out := []Drop{}
	for i, col := range Columns {
		switch {
		case i == idx:
		case i < idx:
			out = append(out, Drop{Column: col.ID, Why: acNoVuelve})
		case i == idx+1 && principal != "":
			out = append(out, Drop{Column: col.ID, Action: principal})
		case i == idx+1:
			out = append(out, Drop{Column: col.ID, Why: c.Plain})
		default:
			out = append(out, Drop{Column: col.ID, Why: "primero tiene que llegar a " + Columns[idx+1].Name + ": " + c.Plain})
		}
	}
	return out
}

func acDrop(t *testing.T, c Card, col string) Drop {
	t.Helper()
	for _, d := range c.Drops {
		if d.Column == col {
			return d
		}
	}
	t.Fatalf("CA-331: la tarjeta en %s no tiene drop para %s: %+v", c.Column, col, c.Drops)
	return Drop{}
}

func acDropEs(t *testing.T, que string, d Drop, action, why string) {
	t.Helper()
	if d.Action != action || d.Why != why {
		t.Fatalf("CA-331: %s: el drop en %s debe ser {action %q, why %q}, fue %+v", que, d.Column, action, why, d)
	}
}

// CA-331: una entrada por cada columna que no es la de la tarjeta, en el
// orden fijo; la siguiente con la accion principal (o plain sin ella), las
// anteriores "no vuelve atras" y las de mas alla "primero tiene que llegar".
func TestCA331_DropsPorColumna(t *testing.T) {
	casos := append(acColumnas(), acColumnasArbol()...)
	vistas := map[string]bool{}
	for _, cs := range casos {
		c := Derive(cs.ev)
		acCol(t, "CA-331", cs.nombre, c, cs.col)
		vistas[c.Column] = true
		want := acDropsEsperados(c, acPrincipal(cs.ids))
		if !reflect.DeepEqual(c.Drops, want) {
			t.Fatalf("CA-331: [%s] los drops de la tarjeta en %s:\nquiere %+v\nfue    %+v", cs.nombre, cs.col, want, c.Drops)
		}
		for _, d := range c.Drops {
			if d.Column == c.Column {
				t.Fatalf("CA-331: [%s] no hay drop a la propia columna: %+v", cs.nombre, c.Drops)
			}
		}
	}
	if len(vistas) != len(Columns) {
		t.Fatalf("CA-331: los fixtures recorren las ocho columnas: %v", vistas)
	}
}

// CA-331: los casos que nombra el contrato, con sus textos.
func TestCA331_DropsCasosDelContrato(t *testing.T) {
	// Writer: la siguiente es Review con pedir-writer; Hecho esta mas alla;
	// Arquitecto es anterior
	c := Derive(acWriter())
	acDropEs(t, "writer", acDrop(t, c, ColReview), ActPedirWriter, "")
	acDropEs(t, "writer", acDrop(t, c, ColHecho), "", "primero tiene que llegar a Review: falta verificar el trabajo contra el spec")
	acDropEs(t, "writer", acDrop(t, c, ColTuAceptacion), "", "primero tiene que llegar a Review: falta verificar el trabajo contra el spec")
	acDropEs(t, "writer", acDrop(t, c, ColArquitecto), "", acNoVuelve)
	acDropEs(t, "writer", acDrop(t, c, ColBacklog), "", acNoVuelve)

	// Tu aprobacion -> Test-writer: aprobar (no pedir-arquitecto, que es la
	// segunda)
	ev := acEv()
	ev.Approval = approval.StatusNotApproved
	c = Derive(ev)
	acCol(t, "CA-331", "tu-aprobacion", c, ColTuAprobacion)
	acDropEs(t, "tu-aprobacion", acDrop(t, c, ColTestWriter), ActAprobar, "")
	acDropEs(t, "tu-aprobacion", acDrop(t, c, ColArquitecto), "", acNoVuelve)
	acDropEs(t, "tu-aprobacion", acDrop(t, c, ColWriter), "", "primero tiene que llegar a Test-writer: "+c.Plain)

	// Tu aceptacion -> Hecho: integrar
	c = Derive(acEv())
	acDropEs(t, "tu-aceptacion", acDrop(t, c, ColHecho), ActIntegrar, "")
	acDropEs(t, "tu-aceptacion", acDrop(t, c, ColReview), "", acNoVuelve)

	// Hecho: solo columnas anteriores, las siete
	cuando := bdT0.Add(48 * time.Hour)
	ev = acEv()
	ev.Item.HechoEn = &cuando
	c = Derive(ev)
	if len(c.Drops) != 7 {
		t.Fatalf("CA-331: una tarjeta en Hecho tiene las siete columnas anteriores: %+v", c.Drops)
	}
	for i, d := range c.Drops {
		if d.Column != Columns[i].ID {
			t.Fatalf("CA-331: los drops van en el orden fijo de las columnas: %+v", c.Drops)
		}
		acDropEs(t, "hecho", d, "", acNoVuelve)
	}

	// Backlog: la siguiente es Arquitecto con pedir-arquitecto; el resto,
	// mas alla
	ev = acEv()
	ev.SpecExists = false
	c = Derive(ev)
	acDropEs(t, "backlog", acDrop(t, c, ColArquitecto), ActPedirArquitecto, "")
	acDropEs(t, "backlog", acDrop(t, c, ColTuAprobacion), "", "primero tiene que llegar a Arquitecto: falta el spec de la tarjeta")
	acDropEs(t, "backlog", acDrop(t, c, ColHecho), "", "primero tiene que llegar a Arquitecto: falta el spec de la tarjeta")
	var cols []string
	for _, d := range c.Drops {
		cols = append(cols, d.Column)
	}
	if strings.Join(cols, ",") != "arquitecto,tu-aprobacion,test-writer,writer,review,tu-aceptacion,hecho" {
		t.Fatalf("CA-331: los drops de Backlog en el orden fijo: %v", cols)
	}

	// Review sin accion principal (lo que falta es guardar): la siguiente
	// dice plain
	c = Derive(tbListo(acEv(), taskcmd.ReadySinGuardar, tbMsgSinGuardar))
	acCol(t, "CA-331", "review sin accion", c, ColReview)
	acDropEs(t, "review sin accion", acDrop(t, c, ColTuAceptacion), "", c.Plain)
	acDropEs(t, "review sin accion", acDrop(t, c, ColHecho), "", "primero tiene que llegar a Tu aceptacion: "+c.Plain)

	// Review con hallazgos: la siguiente abre pedir-writer; con la review
	// exigida ademas, pedir-reviewer (la principal)
	c = Derive(acConHallazgos(acEv(), "f-a"))
	acDropEs(t, "review con hallazgos", acDrop(t, c, ColTuAceptacion), ActPedirWriter, "")
	c = Derive(acConHallazgos(acGrande(acEv()), "f-a"))
	acDropEs(t, "review exigida y con hallazgos", acDrop(t, c, ColTuAceptacion), ActPedirReviewer, "")
}

// CA-331: soltar en la siguiente abre la accion principal aunque este
// deshabilitada (el dialogo muestra su why): el drop lleva la accion.
func TestCA331_DropConAccionPrincipalDeshabilitada(t *testing.T) {
	// sin espacio propio
	c := Derive(bdArbol(acWriter()))
	if a := acAccion(t, "CA-331", c, ActPedirWriter); a.Enabled {
		t.Fatalf("CA-331: el fixture tiene pedir-writer deshabilitada: %+v", a)
	}
	acDropEs(t, "writer sin espacio propio", acDrop(t, c, ColReview), ActPedirWriter, "")
	// con un rol en curso
	ev := acTestWriter()
	ev.Envelopes = []EnvelopeState{acSobreVivo("test-writer", "claude", "run", 4, 6)}
	c = Derive(ev)
	if a := acAccion(t, "CA-331", c, ActPedirTestWriter); a.Enabled {
		t.Fatalf("CA-331: el fixture tiene pedir-test-writer deshabilitada: %+v", a)
	}
	acDropEs(t, "test-writer en curso", acDrop(t, c, ColWriter), ActPedirTestWriter, "")
	// en JSON: action es el id y why ""
	raw, _ := json.Marshal(c.Drops)
	if !strings.Contains(string(raw), `{"column":"writer","action":"pedir-test-writer","why":""}`) {
		t.Fatalf("CA-331: el drop en JSON es {column, action, why}: %s", raw)
	}
}

// acGhost exige el fantasma.
func acGhost(t *testing.T, nombre string, c Card, want Ghost) {
	t.Helper()
	if c.Ghost == nil || *c.Ghost != want {
		t.Fatalf("CA-332: [%s] el fantasma debe ser %+v, fue %+v (running=%+v)", nombre, want, c.Ghost, c.Running)
	}
}

func acSinGhost(t *testing.T, nombre string, c Card) {
	t.Helper()
	if c.Ghost != nil {
		t.Fatalf("CA-332: [%s] sin fantasma (ghost null), fue %+v (running=%+v)", nombre, c.Ghost, c.Running)
	}
}

// acConSobre deja la tarjeta con un unico sobre vivo del rol.
func acConSobre(ev Evidence, rol, stage string, step, steps int) Evidence {
	ev.Envelopes = []EnvelopeState{acSobreVivo(rol, "claude", stage, step, steps)}
	return ev
}

func acArquitectoCol() Evidence {
	ev := acEv()
	ev.LintIssues = []string{`falta la seccion "riesgos"`}
	return ev
}

func acBacklogCol() Evidence {
	ev := acEv()
	ev.SpecExists = false
	return ev
}

// acFantasmaCasos: tarjetas con alguien trabajando, con y sin fantasma.
func acFantasmaCasos() []acFix {
	cuando := bdT0.Add(48 * time.Hour)
	hecho := acEv()
	hecho.Item.HechoEn = &cuando
	aprob := acEv()
	aprob.Approval = approval.StatusNotApproved
	return []acFix{
		{"arquitecto en backlog", acConSobre(acBacklogCol(), "arquitecto", "run", 2, 5)},
		{"arquitecto en backlog sin espacio propio", acConSobre(bdArbol(acBacklogCol()), "arquitecto", "run", 2, 5)},
		{"arquitecto en arquitecto", acConSobre(acArquitectoCol(), "arquitecto", "verify", 4, 5)},
		{"test-writer en test-writer", acConSobre(acTestWriter(), "test-writer", "run", 4, 6)},
		{"writer en writer", acConSobre(acWriter(), "writer", "run", 3, 5)},
		{"reviewer en review", acConSobre(acGrande(acEv()), "reviewer", "run", 2, 4)},
		{"writer en review", acConSobre(acConHallazgos(acEv(), "f-a"), "writer", "scope", 4, 5)},
		{"scout en writer", acConSobre(acWriter(), "scout", "run", 2, 3)},
		{"arquitecto en tu aprobacion", acConSobre(aprob, "arquitecto", "run", 2, 5)},
		{"writer en tu aceptacion", acConSobre(acEv(), "writer", "run", 3, 5)},
		{"writer en hecho", acConSobre(hecho, "writer", "run", 3, 5)},
		{"test-writer en writer", acConSobre(acWriter(), "test-writer", "run", 4, 6)},
		{"writer en test-writer", acConSobre(acTestWriter(), "writer", "run", 3, 5)},
		{"reviewer en writer", acConSobre(acWriter(), "reviewer", "run", 1, 4)},
		{"arquitecto en writer", acConSobre(acWriter(), "arquitecto", "run", 2, 5)},
	}
}

// CA-332: con running de un rol de estacion de la columna, ghost esta en la
// columna siguiente con el rol, el provider, los ids y el stage, step y
// steps de running. La tarjeta sigue en su columna, en curso.
func TestCA332_FantasmaEnLaColumnaSiguiente(t *testing.T) {
	g := func(col, rol, stage string, step, steps int) Ghost {
		return Ghost{Column: col, Role: rol, Provider: "claude", EnvelopeID: "env-vivo", RunID: "run-del-sobre",
			Stage: stage, Step: step, Steps: steps}
	}
	for _, caso := range []struct {
		nombre string
		ev     Evidence
		col    string
		want   Ghost
	}{
		{"arquitecto en backlog", acConSobre(acBacklogCol(), "arquitecto", "run", 2, 5), ColBacklog,
			g(ColArquitecto, "arquitecto", "run", 2, 5)},
		{"arquitecto en backlog sin espacio propio", acConSobre(bdArbol(acBacklogCol()), "arquitecto", "run", 2, 5), ColBacklog,
			g(ColArquitecto, "arquitecto", "run", 2, 5)},
		{"arquitecto en arquitecto", acConSobre(acArquitectoCol(), "arquitecto", "verify", 4, 5), ColArquitecto,
			g(ColTuAprobacion, "arquitecto", "verify", 4, 5)},
		{"test-writer en test-writer", acConSobre(acTestWriter(), "test-writer", "run", 4, 6), ColTestWriter,
			g(ColWriter, "test-writer", "run", 4, 6)},
		{"writer en writer", acConSobre(acWriter(), "writer", "run", 3, 5), ColWriter,
			g(ColReview, "writer", "run", 3, 5)},
		{"reviewer en review", acConSobre(acGrande(acEv()), "reviewer", "run", 2, 4), ColReview,
			g(ColTuAceptacion, "reviewer", "run", 2, 4)},
		{"writer en review", acConSobre(acConHallazgos(acEv(), "f-a"), "writer", "scope", 4, 5), ColReview,
			g(ColTuAceptacion, "writer", "scope", 4, 5)},
	} {
		c := Derive(caso.ev)
		acCol(t, "CA-332", caso.nombre, c, caso.col)
		if c.Running == nil {
			t.Fatalf("CA-332: [%s] la tarjeta sigue en su columna, en curso: %+v", caso.nombre, c)
		}
		acGhost(t, caso.nombre, c, caso.want)
	}

	// un run suelto (sin sobre) de un rol de estacion tambien: sin sobre ni
	// paso, con su run
	ev := acGrande(acEv())
	ev.Runs = []RunState{acRunVivo("reviewer", "codex")}
	c := Derive(ev)
	acGhost(t, "reviewer suelto en review", c, Ghost{Column: ColTuAceptacion, Role: "reviewer", Provider: "codex",
		RunID: "run-vivo", Stage: "run"})
	ev = acWriter()
	ev.Runs = []RunState{acRunVivo("writer", "claude")}
	acGhost(t, "writer suelto en writer", Derive(ev), Ghost{Column: ColReview, Role: "writer", Provider: "claude",
		RunID: "run-vivo", Stage: "run"})

	// en JSON, un objeto con todas sus claves
	m, raw := acJSON(t, Derive(acConSobre(acWriter(), "writer", "run", 3, 5)))
	gh, ok := m["ghost"].(map[string]any)
	if !ok {
		t.Fatalf("CA-332: ghost es un objeto mientras el writer trabaja: %s", raw)
	}
	for _, k := range []string{"column", "role", "provider", "envelope_id", "run_id", "stage", "step", "steps"} {
		if _, ok := gh[k]; !ok {
			t.Fatalf("CA-332: el fantasma trae %q: %v", k, gh)
		}
	}
	if gh["column"] != ColReview || gh["step"] != float64(3) || gh["steps"] != float64(5) {
		t.Fatalf("CA-332: el fantasma en JSON: %v", gh)
	}
}

// CA-332: sin running, o con un rol que no es de estacion de la columna (un
// scout, un arquitecto que corrige el spec en Tu aprobacion), no hay
// fantasma.
func TestCA332_SinFantasma(t *testing.T) {
	for _, f := range acFantasmaCasos() {
		switch f.nombre {
		case "scout en writer", "arquitecto en tu aprobacion", "writer en tu aceptacion", "writer en hecho",
			"test-writer en writer", "writer en test-writer", "reviewer en writer", "arquitecto en writer":
		default:
			continue
		}
		c := Derive(f.ev)
		if c.Running == nil {
			t.Fatalf("CA-332: [%s] el fixture tiene a alguien trabajando: %+v", f.nombre, c)
		}
		acSinGhost(t, f.nombre, c)
		_, raw := acJSON(t, c)
		if !strings.Contains(raw, `"ghost":null`) {
			t.Fatalf("CA-332: [%s] sin fantasma, \"ghost\":null: %s", f.nombre, raw)
		}
	}
	// un run suelto de un rol que no es de estacion, y uno sin rol
	ev := acWriter()
	ev.Runs = []RunState{acRunVivo("scout", "claude")}
	acSinGhost(t, "scout suelto en writer", Derive(ev))
	// sin nadie trabajando, ni aun interrumpida
	acSinGhost(t, "interrumpida", Derive(acReanudable()))
	for _, cs := range acColumnas() {
		acSinGhost(t, cs.nombre, Derive(cs.ev))
	}
}

// CA-332: cuando el sobre cierra, el fantasma desaparece con running. Si
// cerro no-entregable, red.plain da el motivo; si cerro entregable sin la
// evidencia (el spec no pasa lint), la tarjeta sigue donde estaba y plain
// dice lo que falta.
func TestCA332_FantasmaDesapareceAlCerrar(t *testing.T) {
	// el writer trabaja: fantasma en Review
	ev := acConSobre(acWriter(), "writer", "verify", 4, 5)
	acGhost(t, "writer trabajando", Derive(ev), Ghost{Column: ColReview, Role: "writer", Provider: "claude",
		EnvelopeID: "env-vivo", RunID: "run-del-sobre", Stage: "verify", Step: 4, Steps: 5})
	// cierra no-entregable en verify
	r := &ev.Envelopes[0]
	r.Alive = false
	r.Record.Status, r.Record.Note = envelope.StatusNotDeliverable, "test: 2 fallan en .hoom/worktrees/precios"
	r.Record.EndedAt = bdT0.Add(30 * time.Minute)
	r.Record.UpdatedAt = r.Record.EndedAt
	c := Derive(ev)
	acSinGhost(t, "writer no-entregable", c)
	acCol(t, "CA-332", "writer no-entregable", c, ColWriter)
	if c.Running != nil || c.Red == nil || c.Red.Plain != "el writer no entrego: la verificacion dio rojo" {
		t.Fatalf("CA-332: al cerrar no-entregable no hay running y red.plain da el motivo: running=%+v red=%+v", c.Running, c.Red)
	}

	// el arquitecto trabaja en Arquitecto: fantasma en Tu aprobacion
	ev = acConSobre(acArquitectoCol(), "arquitecto", "run", 2, 5)
	acGhost(t, "arquitecto trabajando", Derive(ev), Ghost{Column: ColTuAprobacion, Role: "arquitecto", Provider: "claude",
		EnvelopeID: "env-vivo", RunID: "run-del-sobre", Stage: "run", Step: 2, Steps: 5})
	// cierra entregable, pero el spec sigue sin pasar lint
	r = &ev.Envelopes[0]
	r.Alive = false
	r.Record.Status, r.Record.Stage, r.Record.Step = envelope.StatusDeliverable, "ok", 5
	r.Record.EndedAt = bdT0.Add(30 * time.Minute)
	r.Record.UpdatedAt = r.Record.EndedAt
	c = Derive(ev)
	acSinGhost(t, "arquitecto entregable sin evidencia", c)
	acCol(t, "CA-332", "arquitecto entregable sin evidencia", c, ColArquitecto)
	if c.Running != nil || c.Red != nil || c.Plain != "el spec no esta completo (1 problema de formato)" {
		t.Fatalf("CA-332: al cerrar entregable sin la evidencia la tarjeta dice lo que falta con plain: running=%+v red=%+v plain=%q",
			c.Running, c.Red, c.Plain)
	}

	// un run suelto que termina: el fantasma se va con el
	ev = acWriter()
	ev.Runs = []RunState{acRunVivo("writer", "claude")}
	if Derive(ev).Ghost == nil {
		t.Fatal("CA-332: el run suelto vivo del writer tiene fantasma")
	}
	ev.Runs[0].Alive = false
	ev.Runs[0].Meta.Status = runcmd.StatusDone
	acSinGhost(t, "run suelto terminado", Derive(ev))
}
