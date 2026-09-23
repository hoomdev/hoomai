// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-302..CA-305): el medidor de evidencia de la tarjeta se calcula en
// Derive (pura), segmento por segmento, y un segmento se llena solo con un
// hecho que existe y vale hoy. Evidence literales para Derive, y el disco y
// el binario real para ver que hoom board --json y hoom item show --json
// emiten los campos nuevos.
package boardcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// tbIDs son los ids de los segmentos, en orden.
func tbIDs(c Card) []string {
	var out []string
	for _, s := range c.Meter {
		out = append(out, s.ID)
	}
	return out
}

// tbSeg busca un segmento por id.
func tbSeg(t *testing.T, ca string, c Card, id string) Segment {
	t.Helper()
	for _, s := range c.Meter {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("%s: el medidor no tiene el segmento %q: %+v", ca, id, c.Meter)
	return Segment{}
}

// tbSegEs exige estado y detalle exactos de un segmento.
func tbSegEs(t *testing.T, ca string, c Card, id, state, detail string) {
	t.Helper()
	s := tbSeg(t, ca, c, id)
	if s.State != state || s.Detail != detail {
		t.Fatalf("%s: el segmento %s debe ser %s con %q, fue %s con %q", ca, id, state, detail, s.State, s.Detail)
	}
}

// tbMeterBienFormado: estados del vocabulario, etiqueta y detalle no vacios.
func tbMeterBienFormado(t *testing.T, ca string, c Card) {
	t.Helper()
	if c.Meter == nil {
		t.Fatalf("%s: meter nunca es nil", ca)
	}
	for _, s := range c.Meter {
		switch s.State {
		case SegHecho, SegFalta, SegNoAplica:
		default:
			t.Fatalf("%s: el segmento %s tiene un estado fuera del vocabulario: %q", ca, s.ID, s.State)
		}
		if strings.TrimSpace(s.Detail) == "" || strings.TrimSpace(s.Label) == "" {
			t.Fatalf("%s: el segmento %s lleva etiqueta y detalle: %+v", ca, s.ID, s)
		}
	}
}

// tbGates arma un veredicto con los gates de spec y findings en pass mas
// build, static y test con el estado pedido ("" = el veredicto no lo trae).
func tbGates(build, static, test string) []verdict.GateResult {
	gs := []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "spec_trace", Required: true, Status: verdict.StatusPass},
		{Name: "spec_approved", Required: true, Status: verdict.StatusPass},
		{Name: "findings_open", Required: true, Status: verdict.StatusPass},
	}
	for _, g := range []struct{ name, status string }{{"build", build}, {"static", static}, {"test", test}} {
		if g.status != "" {
			gs = append(gs, verdict.GateResult{Name: g.name, Required: true, Status: g.status})
		}
	}
	return gs
}

// CA-302: el medidor trae spec, un segmento por criterio en el orden de
// Evidence.Criteria (el de spec.Lint, que no es el orden alfabetico), build,
// static, test y review, con sus etiquetas; los criterios se nombran por su
// id.
func TestCA302_MeterOrdenYEtiquetas(t *testing.T) {
	ev := bdEv()
	ev.Criteria = []string{"CA-1", "CA-2", "CA-10"}
	ev.Untraced = []string{"CA-10"}
	c := Derive(ev)
	quiere := []string{"spec", "CA-1", "CA-2", "CA-10", "build", "static", "test", "review"}
	if strings.Join(tbIDs(c), ",") != strings.Join(quiere, ",") {
		t.Fatalf("CA-302: los segmentos en orden fijo %v, fueron %v", quiere, tbIDs(c))
	}
	etiquetas := map[string]string{"spec": "spec aprobado", "CA-1": "CA-1", "CA-2": "CA-2", "CA-10": "CA-10",
		"build": "build", "static": "static", "test": "test", "review": "review"}
	for _, s := range c.Meter {
		if s.Label != etiquetas[s.ID] {
			t.Fatalf("CA-302: la etiqueta de %s es %q, fue %q", s.ID, etiquetas[s.ID], s.Label)
		}
	}
	tbMeterBienFormado(t, "CA-302", c)

	// sin spec, o con issues de lint: cinco segmentos, sin criterios
	sin := bdArbol(bdEv())
	sin.SpecExists = false
	lint := bdEv()
	lint.LintIssues = []string{`falta la seccion "riesgos"`}
	for nombre, e := range map[string]Evidence{"sin spec": sin, "lint con issues": lint} {
		c := Derive(e)
		if strings.Join(tbIDs(c), ",") != "spec,build,static,test,review" {
			t.Fatalf("CA-302 (%s): sin criterios validos el medidor tiene cinco segmentos: %v", nombre, tbIDs(c))
		}
		tbMeterBienFormado(t, "CA-302 ("+nombre+")", c)
	}
}

// CA-302: la tarjeta en JSON conserva todas las claves de C1 y suma plain,
// needs_decision, meter y providers, sin listas null, en cualquier columna.
func TestCA302_CardJSONClavesNuevas(t *testing.T) {
	cuando := bdT0.Add(2 * time.Hour)
	hecho := bdArbol(bdEv())
	hecho.Item.HechoEn = &cuando
	sinSpec := bdArbol(bdEv())
	sinSpec.SpecExists = false
	for nombre, ev := range map[string]Evidence{"tu-aceptacion": bdEv(), "hecho": hecho, "backlog": sinSpec} {
		raw, err := json.Marshal(Derive(ev))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		for _, k := range []string{"slug", "item", "column", "column_name", "missing", "next", "evidence", "running",
			"interrupted", "waiting_human", "red", "unsynced", "spend", "plain", "needs_decision", "meter", "providers"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("CA-302 (%s): a la tarjeta le falta la clave %q: %s", nombre, k, raw)
			}
		}
		if p, ok := m["plain"].(string); !ok || p == "" {
			t.Fatalf("CA-302 (%s): plain es un texto no vacio: %s", nombre, raw)
		}
		if _, ok := m["needs_decision"].(bool); !ok {
			t.Fatalf("CA-302 (%s): needs_decision es un booleano: %s", nombre, raw)
		}
		meter, ok := m["meter"].([]any)
		if !ok || len(meter) < 5 {
			t.Fatalf("CA-302 (%s): meter es una lista (no null) de al menos cinco segmentos: %s", nombre, raw)
		}
		for _, s := range meter {
			seg, _ := s.(map[string]any)
			for _, k := range []string{"id", "label", "state", "detail"} {
				if _, ok := seg[k].(string); !ok {
					t.Fatalf("CA-302 (%s): cada segmento trae %q: %v", nombre, k, seg)
				}
			}
		}
		prov, ok := m["providers"].(map[string]any)
		if !ok {
			t.Fatalf("CA-302 (%s): providers es un objeto: %s", nombre, raw)
		}
		for _, k := range []string{"writer", "reviewer", "cross"} {
			if _, ok := prov[k].(string); !ok {
				t.Fatalf("CA-302 (%s): providers.%s es un texto (\"\" sin dato): %s", nombre, k, raw)
			}
		}
	}
}

// CA-303: el segmento spec en sus cinco casos, con el detalle del contrato.
func TestCA303_SegmentoSpec(t *testing.T) {
	sinSpec := bdArbol(bdEv())
	sinSpec.SpecExists = false
	sinSpec.Approval = ""
	sinSpecAprobado := bdArbol(bdEv()) // la aprobacion de otro contenido no llena un spec que no esta
	sinSpecAprobado.SpecExists = false
	lint := bdEv()
	lint.LintIssues = []string{`falta la seccion "riesgos"`}
	lint.Approval = approval.StatusNotApproved
	sinFirma := bdEv()
	sinFirma.Approval = approval.StatusNotApproved
	invalidada := bdEv()
	invalidada.Approval = approval.StatusInvalidated
	casos := []struct {
		nombre        string
		ev            Evidence
		state, detail string
	}{
		{"sin spec", sinSpec, SegFalta, "todavia no hay spec"},
		{"sin spec (aprobacion vieja)", sinSpecAprobado, SegFalta, "todavia no hay spec"},
		{"lint con issues", lint, SegFalta, "el spec no esta completo"},
		{"sin aprobacion", sinFirma, SegFalta, "espera tu aprobacion"},
		{"invalidada", invalidada, SegFalta, "el spec cambio despues de tu aprobacion"},
		{"aprobada", bdEv(), SegHecho, "aprobado para el contenido actual"},
	}
	for _, caso := range casos {
		c := Derive(caso.ev)
		tbSegEs(t, "CA-303 ("+caso.nombre+")", c, "spec", caso.state, caso.detail)
		if c.Meter[0].ID != "spec" {
			t.Fatalf("CA-303 (%s): spec es el primer segmento: %v", caso.nombre, tbIDs(c))
		}
	}
}

// CA-303: un criterio con test es hecho "con prueba", uno cubierto por un
// marcador verifica es hecho "con comando de verificacion", y uno sin nada
// es falta "sin prueba".
func TestCA303_SegmentosDeCriterios(t *testing.T) {
	ev := bdEv()
	ev.Criteria = []string{"CA-1", "CA-2", "CA-3"}
	ev.ByCommand = []string{"CA-2"}
	ev.Untraced = []string{"CA-3"}
	c := Derive(ev)
	tbSegEs(t, "CA-303", c, "CA-1", SegHecho, "con prueba")
	tbSegEs(t, "CA-303", c, "CA-2", SegHecho, "con comando de verificacion")
	tbSegEs(t, "CA-303", c, "CA-3", SegFalta, "sin prueba")

	// todos por comando, ninguno sin prueba
	ev = bdEv()
	ev.ByCommand = []string{"CA-1", "CA-2", "CA-3"}
	c = Derive(ev)
	for _, id := range []string{"CA-1", "CA-2", "CA-3"} {
		tbSegEs(t, "CA-303", c, id, SegHecho, "con comando de verificacion")
	}
}

// CA-303: Gather junta ByCommand (los criterios de un marcador verifica, sin
// correrlo) y el medidor lo muestra, leido del disco.
func TestCA303_ByCommandDesdeElDisco(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	bdEscribir(t, root, bdSpec, bdSpecTexto(
		"- CA-1: por comando. [verifica: touch marca]\n- CA-2: con test.\n- CA-3: sin nada.\n- CA-4: otro comando. [verifica: true]", true))
	bdAprobar(t, root, bdSlug)
	bdEscribir(t, root, "precios_test.go", "package app\n\n// CA-2: con test\n")
	ev := Gather(root, "main", "high", bdItem(bdSlug), now)
	got := strings.Join(ev.ByCommand, ",")
	if got != "CA-1,CA-4" && got != "CA-4,CA-1" {
		t.Fatalf("CA-303: ByCommand son los criterios con marcador verifica (CA-1, CA-4): %v", ev.ByCommand)
	}
	c := Derive(ev)
	tbSegEs(t, "CA-303", c, "CA-1", SegHecho, "con comando de verificacion")
	tbSegEs(t, "CA-303", c, "CA-2", SegHecho, "con prueba")
	tbSegEs(t, "CA-303", c, "CA-3", SegFalta, "sin prueba")
	tbSegEs(t, "CA-303", c, "CA-4", SegHecho, "con comando de verificacion")
	if _, err := Build(root, "main", "high", now); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "marca")); !os.IsNotExist(err) {
		t.Fatal("CA-303: juntar ByCommand no corre el marcador ('marca' aparecio)")
	}
}

// CA-304: build, static y test son hecho solo con el gate en pass en el
// veredicto de la tarjeta y su huella actual; un veredicto sin el gate es
// no-aplica; fail, error y absent nombran el estado; sin veredicto, falta.
func TestCA304_GatesSoloConHuellaVigente(t *testing.T) {
	ev := bdEv()
	ev.Verdict = bdVeredicto("v-ok", bdT0.Add(30*time.Minute), "huella-1", 50, 10, tbGates(verdict.StatusPass, verdict.StatusPass, verdict.StatusPass))
	ev.GreenVerdicts = []string{"v-ok"}
	c := Derive(ev)
	for _, g := range []string{"build", "static", "test"} {
		tbSegEs(t, "CA-304", c, g, SegHecho, "paso en el veredicto vigente")
	}

	// la misma evidencia con otra huella: nada se llena
	ev.Fingerprint = "huella-2"
	c = Derive(ev)
	for _, g := range []string{"build", "static", "test"} {
		tbSegEs(t, "CA-304", c, g, SegFalta, "el codigo cambio despues del veredicto")
	}

	// el veredicto no trae build ni static: no-aplica, no falta
	ev = bdEv() // bdGatesVerdes: trae test y no build ni static
	c = Derive(ev)
	tbSegEs(t, "CA-304", c, "build", SegNoAplica, "el proyecto no declara el gate build")
	tbSegEs(t, "CA-304", c, "static", SegNoAplica, "el proyecto no declara el gate static")
	tbSegEs(t, "CA-304", c, "test", SegHecho, "paso en el veredicto vigente")

	// fail, error y absent: falta nombrando el estado
	ev = bdEv()
	ev.Verdict = bdVeredicto("v-mal", bdT0.Add(40*time.Minute), "huella-1", 50, 10, tbGates(verdict.StatusError, verdict.StatusAbsent, verdict.StatusFail))
	ev.GreenVerdicts = nil
	c = Derive(ev)
	tbSegEs(t, "CA-304", c, "build", SegFalta, "el gate build no paso (error)")
	tbSegEs(t, "CA-304", c, "static", SegFalta, "el gate static no paso (absent)")
	tbSegEs(t, "CA-304", c, "test", SegFalta, "el gate test no paso (fail)")

	// sin veredicto de la tarjeta
	ev = bdEv()
	ev.Verdict, ev.GreenVerdicts, ev.Fingerprint = nil, nil, ""
	c = Derive(ev)
	for _, g := range []string{"build", "static", "test"} {
		tbSegEs(t, "CA-304", c, g, SegFalta, "todavia no hay veredicto")
	}
}

// CA-304: en un veredicto ROJO con la huella actual, el gate que paso llena
// su segmento aunque otro haya fallado.
func TestCA304_RojoConHuellaActualLlenaLoQuePaso(t *testing.T) {
	ev := bdEv()
	ev.Verdict = bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10, tbGates(verdict.StatusPass, verdict.StatusPass, verdict.StatusFail))
	ev.GreenVerdicts = nil
	c := Derive(ev)
	bdCol(t, "CA-304", c, ColWriter)
	tbSegEs(t, "CA-304", c, "build", SegHecho, "paso en el veredicto vigente")
	tbSegEs(t, "CA-304", c, "static", SegHecho, "paso en el veredicto vigente")
	tbSegEs(t, "CA-304", c, "test", SegFalta, "el gate test no paso (fail)")
}

// CA-304: en la columna hecho no se exige la huella actual: una tarjeta
// cerrada no se reevalua contra lo que paso despues en main.
func TestCA304_EnHechoNoSeExigeLaHuella(t *testing.T) {
	cuando := bdT0.Add(48 * time.Hour)
	ev := bdArbol(bdEv())
	ev.Item.HechoEn = &cuando
	ev.Item.CommitFinal = "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c"
	ev.Verdict = bdVeredicto("v-cierre", bdT0.Add(30*time.Minute), "huella-del-cierre", 50, 10, tbGates(verdict.StatusPass, verdict.StatusPass, verdict.StatusPass))
	ev.GreenVerdicts = []string{"v-cierre"}
	ev.Fingerprint = "main-se-movio"
	c := Derive(ev)
	bdCol(t, "CA-304", c, ColHecho)
	for _, g := range []string{"build", "static", "test"} {
		if s := tbSeg(t, "CA-304", c, g); s.State != SegHecho {
			t.Fatalf("CA-304: en hecho el gate %s en pass llena su segmento aunque la huella haya cambiado: %+v", g, s)
		}
	}
	// y fuera de hecho, la misma evidencia no los llena
	ev.Item.HechoEn = nil
	c = Derive(ev)
	for _, g := range []string{"build", "static", "test"} {
		tbSegEs(t, "CA-304", c, g, SegFalta, "el codigo cambio despues del veredicto")
	}
}

// CA-305: review es hecho con un registro que cuenta, no-aplica con un
// veredicto de 400 lineas o menos sin registro, y falta con mas de 400 sin
// registro que cuente (una sola lente no cuenta) o sin veredicto.
func TestCA305_SegmentoReview(t *testing.T) {
	rec := func(id string, lentes []string, verdictID string) reviewcmd.Record {
		return reviewcmd.Record{ID: id, CreatedAt: bdT0.Add(35 * time.Minute), Task: bdSlug, Spec: bdSpec,
			Fingerprint: "huella-1", VerdictID: verdictID, Verdict: "green", Lenses: lentes,
			Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}}
	}
	tam := func(ins, del int) Evidence {
		ev := bdEv()
		ev.Verdict = bdVeredicto("v-tam", bdT0.Add(30*time.Minute), "huella-1", ins, del, bdGatesVerdes())
		ev.GreenVerdicts = []string{"v-tam"}
		return ev
	}

	tbSegEs(t, "CA-305", Derive(bdEv()), "review", SegNoAplica, "el cambio es chico (60 lineas): no exige la revision de 4 lentes")
	tbSegEs(t, "CA-305", Derive(tam(400, 0)), "review", SegNoAplica, "el cambio es chico (400 lineas): no exige la revision de 4 lentes")
	tbSegEs(t, "CA-305", Derive(tam(0, 0)), "review", SegNoAplica, "el cambio es chico (0 lineas): no exige la revision de 4 lentes")

	grande := tam(500, 112)
	tbSegEs(t, "CA-305", Derive(grande), "review", SegFalta, "falta la revision de 4 lentes")
	grande.Reviews = []reviewcmd.Record{rec("r-una", []string{"reliability"}, "v-tam")}
	tbSegEs(t, "CA-305 (una lente)", Derive(grande), "review", SegFalta, "falta la revision de 4 lentes")
	grande.Reviews = []reviewcmd.Record{rec("r-rojo", reviewcmd.Lentes, "v-que-no-es-verde")}
	tbSegEs(t, "CA-305 (sobre un rojo)", Derive(grande), "review", SegFalta, "falta la revision de 4 lentes")
	grande.Reviews = []reviewcmd.Record{rec("r-ok", reviewcmd.Lentes, "v-tam")}
	tbSegEs(t, "CA-305", Derive(grande), "review", SegHecho, "revision de 4 lentes registrada")

	// un registro que cuenta tambien llena el segmento en un cambio chico
	chico := tam(30, 5)
	chico.Reviews = []reviewcmd.Record{rec("r-ok", reviewcmd.Lentes, "v-tam")}
	tbSegEs(t, "CA-305", Derive(chico), "review", SegHecho, "revision de 4 lentes registrada")

	// sin veredicto
	ev := bdEv()
	ev.Verdict, ev.GreenVerdicts = nil, nil
	tbSegEs(t, "CA-305", Derive(ev), "review", SegFalta, "todavia no hay veredicto")
	if s := Derive(ev).Meter; s[len(s)-1].ID != "review" {
		t.Fatalf("CA-305: review es el ultimo segmento: %+v", s)
	}
}

// CA-302: leido del disco, el tablero trae el medidor de cada tarjeta, y
// JSONBytes lo emite sin null.
func TestCA302_BuildTraeElMedidor(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Now().UTC()
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdItemArchivo(t, root, "vacio", "Vacio", "")
	bdEscribir(t, root, bdSpec, bdSpecTexto("- CA-1: con test.\n- CA-2: por comando. [verifica: true]\n- CA-3: sin nada.", true))
	bdAprobar(t, root, bdSlug)
	bdEscribir(t, root, "precios_test.go", "package app\n\n// CA-1\n")
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatal(err)
	}
	c := bdCard(t, b, bdSlug)
	if strings.Join(tbIDs(c), ",") != "spec,CA-1,CA-2,CA-3,build,static,test,review" {
		t.Fatalf("CA-302: el medidor de la tarjeta leida del disco: %v", tbIDs(c))
	}
	tbSegEs(t, "CA-302", c, "spec", SegHecho, "aprobado para el contenido actual")
	tbSegEs(t, "CA-302", c, "CA-2", SegHecho, "con comando de verificacion")
	tbSegEs(t, "CA-302", c, "test", SegFalta, "todavia no hay veredicto")
	v := bdCard(t, b, "vacio")
	if strings.Join(tbIDs(v), ",") != "spec,build,static,test,review" {
		t.Fatalf("CA-302: sin spec, cinco segmentos: %v", tbIDs(v))
	}
	raw, err := JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibido := range []string{`"meter": null`, `"plain": ""`, `"providers": null`} {
		if strings.Contains(string(raw), prohibido) {
			t.Fatalf("CA-302: el tablero no trae %s:\n%s", prohibido, raw)
		}
	}
}

// CA-302: con el binario real, hoom board --json y hoom item show --json
// traen las claves nuevas en la tarjeta, con la misma tarjeta en los dos.
func TestCA302_E2EBoardYShowTraenLosCamposNuevos(t *testing.T) {
	bin := bxBinario(t)
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "")

	r := bxCorrer(t, bin, root, "board", "--json")
	bxOK(t, "CA-302", r, "board", "--json")
	var b struct {
		Columns []struct {
			ID    string           `json:"id"`
			Cards []map[string]any `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &b); err != nil {
		t.Fatalf("CA-302: board --json es JSON: %v\n%s", err, r.stdout)
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
		t.Fatalf("CA-302: el tablero trae la tarjeta:\n%s", r.stdout)
	}
	r = bxCorrer(t, bin, root, "item", "show", bdSlug, "--json")
	bxOK(t, "CA-302", r, "item", "show", bdSlug, "--json")
	var show struct {
		Card map[string]any `json:"card"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &show); err != nil || show.Card == nil {
		t.Fatalf("CA-302: item show --json trae la tarjeta: %v\n%s", err, r.stdout)
	}
	for nombre, c := range map[string]map[string]any{"board": deBoard, "item show": show.Card} {
		for _, k := range []string{"slug", "column", "missing", "next", "evidence", "unsynced", "spend",
			"plain", "needs_decision", "meter", "providers"} {
			if _, ok := c[k]; !ok {
				t.Fatalf("CA-302 (%s): la tarjeta trae %q: %v", nombre, k, c)
			}
		}
		if p, _ := c["plain"].(string); p != "falta el spec de la tarjeta" {
			t.Fatalf("CA-302 (%s): plain de backlog: %v", nombre, c["plain"])
		}
		m, ok := c["meter"].([]any)
		if !ok || len(m) != 5 {
			t.Fatalf("CA-302 (%s): meter con cinco segmentos (sin spec): %v", nombre, c["meter"])
		}
		if _, ok := c["providers"].(map[string]any); !ok {
			t.Fatalf("CA-302 (%s): providers es un objeto: %v", nombre, c["providers"])
		}
	}
	a, _ := json.Marshal(deBoard)
	s, _ := json.Marshal(show.Card)
	if string(a) != string(s) {
		t.Fatalf("CA-302: board y item show dan la misma tarjeta:\nboard: %s\nshow:  %s", a, s)
	}
}
