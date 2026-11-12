// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// el tablero: la review que exige segun el riesgo (CA-622), lo que bloquea
// juzgado con finding.Bloquea (CA-623, CA-624, CA-626, CA-627), los
// pendientes de la tarjeta (CA-629) y el tope de pendientes en la tarjeta
// (CA-633). Derive con Evidence literales y, donde hace falta, el tablero en
// disco contra el gate findings_open. Escritos desde el spec y las firmas
// publicas, sin leer la implementacion.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

var crTres = []string{"risk", "reliability", "resilience"}

const (
	crMotAlto = "la review exige las 4 lentes (riesgo alto) y no hay registro de review"
	crMotHoy  = "la review exige las 4 lentes (612 lineas > 400) y no hay registro de review"
)

func crMotBajo(lineas int) string {
	return fmt.Sprintf("la review exige la lente reliability (%d lineas > %d, riesgo bajo) y no hay registro de review", lineas, 400)
}

// crEv es bdEv con el riesgo r en el item y un veredicto de ins+del lineas
// sobre files (los changed_files del veredicto).
func crEv(r string, ins, del int, files ...string) Evidence {
	ev := bdEv()
	ev.Item.Riesgo = r
	v := &verdict.Verdict{ID: "v-cr", CreatedAt: bdT0.Add(30 * time.Minute), Spec: bdSpec,
		Git:   gitx.Info{IsRepo: true, ChangeFingerprint: "huella-1", Insertions: ins, Deletions: del, ChangedFiles: files},
		Gates: bdGatesVerdes()}
	v.Finalize()
	ev.Verdict, ev.GreenVerdicts = v, []string{v.ID}
	return ev
}

// crRec es un registro anclado de la tarjeta sobre el verde v-cr.
func crRec(id, cobertura, desdeReview string, lentes []string, hallazgos ...string) reviewcmd.Record {
	if hallazgos == nil {
		hallazgos = []string{}
	}
	r := reviewcmd.Record{ID: id, CreatedAt: bdT0.Add(35 * time.Minute), Task: bdSlug, Spec: bdSpec,
		Fingerprint: "huella-1", VerdictID: "v-cr", Verdict: "green", Lenses: lentes,
		Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: hallazgos,
		Desde: bdDesde, Hasta: bdHasta, Cobertura: cobertura, DesdeReview: desdeReview}
	return r
}

// crCadena es una completa c1 con lentesC y un delta d1 encadenado con
// lentesD, sin codigo despues.
func crCadena(ev Evidence, lentesC, lentesD []string) Evidence {
	c := crRec("c1", reviewcmd.CoberturaCompleta, "", lentesC)
	c.Hasta = "3333333333333333333333333333333333333333"
	d := crRec("d1", reviewcmd.CoberturaDelta, "c1", lentesD)
	d.Desde = c.Hasta
	d.CreatedAt = bdT0.Add(40 * time.Minute)
	ev.Reviews = []reviewcmd.Record{c, d}
	ev = bdColas(ev)
	// despues del hasta de la completa cambio codigo (el que revisa el delta)
	ev.ReviewTails["c1"] = ReviewTail{Ancestro: true, Codigo: true}
	return ev
}

// CA-622: con riesgo alto la review se exige siempre (salvo un cambio vacio
// o de solo documentacion); con bajo y normal, con mas de 400 lineas.
func TestCA622_ReviewExigidaSegunElRiesgo(t *testing.T) {
	// alto, cambio chico
	c := Derive(crEv("alto", 50, 10, "a.go"))
	bdCol(t, "CA-622", c, ColReview)
	bdPrimero(t, "CA-622", c, crMotAlto)
	if !c.Evidence.ReviewRequired {
		t.Fatalf("CA-622: con alto la review se exige: %+v", c.Evidence)
	}
	// alto, solo documentacion o sin archivos: no se exige (caso limite)
	for _, files := range [][]string{{"README.md", "docs/guia.md"}, nil} {
		c = Derive(crEv("alto", 50, 10, files...))
		if c.Evidence.ReviewRequired {
			t.Fatalf("CA-622: alto con %v no exige review: %+v", files, c.Evidence)
		}
		bdCol(t, "CA-622", c, ColTuAceptacion)
	}
	// bajo, grande: exige la lente reliability
	c = Derive(crEv("bajo", 500, 112, "a.go"))
	bdCol(t, "CA-622", c, ColReview)
	bdPrimero(t, "CA-622", c, crMotBajo(612))
	// bajo, chico: no exige
	c = Derive(crEv("bajo", 50, 10, "a.go"))
	if c.Evidence.ReviewRequired {
		t.Fatalf("CA-622: bajo chico no exige review: %+v", c.Evidence)
	}
	// bajo que toca una ruta de riesgo: el efectivo es normal, el texto de hoy
	c = Derive(crEv("bajo", 500, 112, "a.go", "internal/auth/x.go"))
	bdCol(t, "CA-622", c, ColReview)
	bdPrimero(t, "CA-622", c, crMotHoy)
	// normal y sin riesgo: el texto de hoy
	for _, r := range []string{"normal", ""} {
		c = Derive(crEv(r, 500, 112, "a.go"))
		bdCol(t, "CA-622", c, ColReview)
		bdPrimero(t, "CA-622", c, crMotHoy)
		c = Derive(crEv(r, 50, 10, "a.go"))
		if c.Evidence.ReviewRequired {
			t.Fatalf("CA-622: normal chico no exige review: %+v", c.Evidence)
		}
	}
}

// CA-622: un registro cuenta si trae las lentes que la regla pide para la
// tarjeta, o mas: con bajo, reliability; con normal y alto, las 4 en la
// completa y las 3 en el delta. Re-expresa CA-427 y CA-428 solo en las
// lentes que cada registro necesita.
func TestCA622_LaCadenaCuentaConLasLentesDeLaTarjeta(t *testing.T) {
	type caso struct {
		nombre     string
		riesgo     string
		lentesC    []string
		lentesD    []string // nil = sin delta
		cuenta     string   // "" = no cuenta
		files      []string
		ins, del   int
		motivoFalt string
	}
	una := []string{"reliability"}
	casos := []caso{
		{"bajo completa reliability", "bajo", una, nil, "c1", []string{"a.go"}, 500, 112, ""},
		{"bajo completa de 4 (o mas)", "bajo", reviewcmd.Lentes, nil, "c1", []string{"a.go"}, 500, 112, ""},
		{"bajo completa y delta reliability", "bajo", una, una, "d1", []string{"a.go"}, 500, 112, ""},
		{"bajo completa risk solo", "bajo", []string{"risk"}, nil, "", []string{"a.go"}, 500, 112, crMotBajo(612)},
		{"normal completa de 4 y delta de 3", "normal", reviewcmd.Lentes, crTres, "d1", []string{"a.go"}, 500, 112, ""},
		{"normal completa de 4 y delta de 4 (o mas)", "normal", reviewcmd.Lentes, reviewcmd.Lentes, "d1", []string{"a.go"}, 500, 112, ""},
		{"normal completa de 3", "normal", crTres, nil, "", []string{"a.go"}, 500, 112, crMotHoy},
		{"normal delta de 1", "normal", reviewcmd.Lentes, una, "", []string{"a.go"}, 500, 112, ""},
		{"alto completa de 4 y delta de 3", "alto", reviewcmd.Lentes, crTres, "d1", []string{"a.go"}, 50, 10, ""},
		{"alto completa reliability", "alto", una, nil, "", []string{"a.go"}, 50, 10, crMotAlto},
		{"alto delta sin resilience", "alto", reviewcmd.Lentes, []string{"risk", "reliability"}, "", []string{"a.go"}, 50, 10, ""},
		{"bajo con ruta de riesgo, completa reliability", "bajo", una, nil, "", []string{"a.go", "internal/auth/x.go"}, 500, 112, crMotHoy},
	}
	for _, c := range casos {
		ev := crEv(c.riesgo, c.ins, c.del, c.files...)
		if c.lentesD == nil {
			ev.Reviews = []reviewcmd.Record{crRec("c1", reviewcmd.CoberturaCompleta, "", c.lentesC)}
			ev = bdColas(ev)
		} else {
			ev = crCadena(ev, c.lentesC, c.lentesD)
		}
		card := Derive(ev)
		if c.cuenta != "" {
			bdCol(t, "CA-622 ("+c.nombre+")", card, ColTuAceptacion)
			if card.Evidence.ReviewID != c.cuenta {
				t.Fatalf("CA-622 (%s): la review de la tarjeta es %s: %+v", c.nombre, c.cuenta, card.Evidence)
			}
			continue
		}
		bdCol(t, "CA-622 ("+c.nombre+")", card, ColReview)
		if card.Evidence.ReviewID != "" {
			t.Fatalf("CA-622 (%s): no cuenta: %+v", c.nombre, card.Evidence)
		}
		if c.motivoFalt != "" {
			bdPrimero(t, "CA-622 ("+c.nombre+")", card, c.motivoFalt)
		}
	}
}

// CA-622 (caso limite: el riesgo cambia despues de una review): el tablero
// recalcula. De bajo a alto, un registro de una sola lente deja de cubrir;
// de alto a bajo, lo que ya se reviso de mas sigue contando.
func TestCA622_ElRiesgoCambiaDespuesDeLaReview(t *testing.T) {
	ev := crEv("bajo", 500, 112, "a.go")
	ev.Reviews = []reviewcmd.Record{crRec("c1", reviewcmd.CoberturaCompleta, "", []string{"reliability"})}
	ev = bdColas(ev)
	bdCol(t, "CA-622 (fixture bajo)", Derive(ev), ColTuAceptacion)
	ev.Item.Riesgo = "alto"
	c := Derive(ev)
	bdCol(t, "CA-622 (bajo a alto)", c, ColReview)
	bdPrimero(t, "CA-622 (bajo a alto)", c, crMotAlto)

	ev = crEv("alto", 500, 112, "a.go")
	ev.Reviews = []reviewcmd.Record{crRec("c1", reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes)}
	ev = bdColas(ev)
	bdCol(t, "CA-622 (fixture alto)", Derive(ev), ColTuAceptacion)
	ev.Item.Riesgo = "bajo"
	c = Derive(ev)
	bdCol(t, "CA-622 (alto a bajo)", c, ColTuAceptacion)
	if c.Evidence.ReviewID != "c1" {
		t.Fatalf("CA-622: lo revisado de mas sigue contando: %+v", c.Evidence)
	}
}

// CA-622: con riesgo bajo los textos de la tarjeta que dicen `4 lentes`
// dicen `la lente reliability`; con normal y alto no cambian.
func TestCA622_TextosDeLaTarjetaBajo(t *testing.T) {
	textos := func(c Card) string {
		raw, _ := json.Marshal(c)
		var b bytes.Buffer
		RenderCard(&b, c)
		return string(raw) + "\n" + b.String()
	}
	for nombre, ev := range map[string]Evidence{
		"bajo grande sin review": crEv("bajo", 500, 112, "a.go"),
		"bajo chico":             crEv("bajo", 50, 10, "a.go"),
		"bajo con review": func() Evidence {
			e := crEv("bajo", 500, 112, "a.go")
			e.Reviews = []reviewcmd.Record{crRec("c1", reviewcmd.CoberturaCompleta, "", []string{"reliability"})}
			return bdColas(e)
		}(),
	} {
		c := Derive(ev)
		s := textos(c)
		if strings.Contains(s, "4 lentes") {
			t.Fatalf("CA-622 (%s): una tarjeta bajo no dice `4 lentes`:\n%s", nombre, s)
		}
		if !strings.Contains(s, "la lente reliability") {
			t.Fatalf("CA-622 (%s): una tarjeta bajo dice `la lente reliability`:\n%s", nombre, s)
		}
	}
	// el segmento de review de una tarjeta bajo
	tbSegEs(t, "CA-622", Derive(crEv("bajo", 500, 112, "a.go")), "review", SegFalta, "falta la revision de la lente reliability")
	ev := crEv("bajo", 500, 112, "a.go")
	ev.Reviews = []reviewcmd.Record{crRec("c1", reviewcmd.CoberturaCompleta, "", []string{"reliability"})}
	tbSegEs(t, "CA-622", Derive(bdColas(ev)), "review", SegHecho, "revision de la lente reliability registrada")
	// con normal y alto, los textos de hoy
	tbSegEs(t, "CA-622", Derive(crEv("normal", 500, 112, "a.go")), "review", SegFalta, "falta la revision de 4 lentes")
	tbSegEs(t, "CA-622", Derive(crEv("alto", 500, 112, "a.go")), "review", SegFalta, "falta la revision de 4 lentes")
	ev = crEv("alto", 50, 10, "a.go")
	ev.Reviews = []reviewcmd.Record{crRec("c1", reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes)}
	tbSegEs(t, "CA-622", Derive(bdColas(ev)), "review", SegHecho, "revision de 4 lentes registrada")
}

// CA-622: la linea de tiempo marca hecho el paso de review con un registro
// que cuenta en la cadena (la tarjeta bajo con una sola lente), no solo con
// uno de 4 lentes. Lo mira el medidor: el segmento review hecho.
func TestCA622_UnaLenteQueCuentaLlenaElPaso(t *testing.T) {
	ev := crEv("bajo", 500, 112, "a.go")
	ev.Reviews = []reviewcmd.Record{crRec("c1", reviewcmd.CoberturaCompleta, "", []string{"reliability"})}
	c := Derive(bdColas(ev))
	if s := tbSeg(t, "CA-622", c, "review"); s.State != SegHecho {
		t.Fatalf("CA-622: el paso de review queda hecho con un registro que cuenta: %+v", s)
	}
}

// ---------------------------------------------------------------- que bloquea en la tarjeta

func crHallazgo(id, sev, file string) finding.Item {
	it := bdHallazgo(id, sev, bdSlug)
	it.File = file
	return it
}

// CA-623/CA-627: un medium en ruta de riesgo bloquea la tarjeta y el motivo
// lo nombra; un medium fuera de ruta no; una severidad desconocida bloquea.
func TestCA623_TarjetaMediumEnRutaDeRiesgo(t *testing.T) {
	ev := crEv("normal", 50, 10, "a.go")
	ev.Findings = []finding.Item{
		crHallazgo("f-ruta", "medium", "internal/auth/login.go"),
		crHallazgo("f-pend", "medium", "app.go"),
		crHallazgo("f-sin", "medium", ""),
		crHallazgo("f-low", "low", "internal/auth/login.go"),
	}
	c := Derive(ev)
	bdCol(t, "CA-623", c, ColReview)
	if !reflect.DeepEqual(c.Evidence.BlockingFindings, []string{"f-ruta"}) {
		t.Fatalf("CA-623: bloquea solo el medium en ruta de riesgo: %v", c.Evidence.BlockingFindings)
	}
	bdTiene(t, "CA-627", c.Missing, "f-ruta")
	bdTiene(t, "CA-627", c.Missing, "medium en ruta de riesgo: internal/auth/login.go")

	ev = crEv("normal", 50, 10, "a.go")
	ev.Findings = []finding.Item{crHallazgo("f-raro", "critical", "app.go")}
	c = Derive(ev)
	bdCol(t, "CA-623", c, ColReview)
	if !reflect.DeepEqual(c.Evidence.BlockingFindings, []string{"f-raro"}) {
		t.Fatalf("CA-623: una severidad desconocida bloquea la tarjeta viva: %v", c.Evidence.BlockingFindings)
	}
}

// CA-624: con una tarjeta de riesgo alto, el medium de una completa bloquea
// con el motivo; el que figura solo en un delta no; con normal o bajo
// ninguno.
func TestCA624_TarjetaAltoMediumDeLaCompleta(t *testing.T) {
	for _, r := range []string{"alto", "normal", "bajo"} {
		ev := crEv(r, 50, 10, "a.go")
		c1 := crRec("c1", reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes, "m-compl")
		c1.Hasta = "3333333333333333333333333333333333333333"
		d1 := crRec("d1", reviewcmd.CoberturaDelta, "c1", reviewcmd.Lentes, "m-delta")
		d1.Desde = c1.Hasta
		ev.Reviews = []reviewcmd.Record{c1, d1}
		ev = bdColas(ev)
		ev.Findings = []finding.Item{crHallazgo("m-compl", "medium", "app.go"), crHallazgo("m-delta", "medium", "app.go")}
		c := Derive(ev)
		if r != "alto" {
			if len(c.Evidence.BlockingFindings) != 0 {
				t.Fatalf("CA-624: con %s ningun medium bloquea: %v", r, c.Evidence.BlockingFindings)
			}
			bdCol(t, "CA-624 ("+r+")", c, ColTuAceptacion)
			continue
		}
		bdCol(t, "CA-624", c, ColReview)
		if !reflect.DeepEqual(c.Evidence.BlockingFindings, []string{"m-compl"}) {
			t.Fatalf("CA-624: con alto bloquea solo el de la completa: %v", c.Evidence.BlockingFindings)
		}
		bdTiene(t, "CA-627", c.Missing, "medium de la review completa c1: riesgo alto")
		if !reflect.DeepEqual(c.Pendientes.IDs, []string{"m-delta"}) {
			t.Fatalf("CA-624/CA-629: el del delta queda pendiente: %+v", c.Pendientes)
		}
	}
}

// CA-626/CA-627: un low con una subida valida bloquea la tarjeta con
// `subido a mano por <autor>`.
func TestCA626_TarjetaLowSubido(t *testing.T) {
	ev := crEv("normal", 50, 10, "a.go")
	ev.Findings = []finding.Item{crHallazgo("f-low", "low", "app.go"), crHallazgo("f-otro", "low", "app.go")}
	ev.Subidas = map[string]finding.Subida{"f-low": {FindingID: "f-low", Reason: "rompe", Author: "Ana <ana@hoom.dev>"}}
	c := Derive(ev)
	bdCol(t, "CA-626", c, ColReview)
	if !reflect.DeepEqual(c.Evidence.BlockingFindings, []string{"f-low"}) {
		t.Fatalf("CA-626: el low subido bloquea: %v", c.Evidence.BlockingFindings)
	}
	bdTiene(t, "CA-627", c.Missing, "subido a mano por Ana <ana@hoom.dev>")
	if !reflect.DeepEqual(c.Pendientes.IDs, []string{"f-otro"}) || c.Pendientes.Low != 1 {
		t.Fatalf("CA-626/CA-629: el low no subido sigue pendiente: %+v", c.Pendientes)
	}
}

type crConjunto struct {
	Findings []finding.Item
	Subidas  map[string]finding.Subida
	Riesgo   string
	Reviews  []reviewcmd.Record
	BlockOn  string
}

func (crConjunto) Generate(r *rand.Rand, _ int) reflect.Value {
	sevs := []string{"low", "medium", "high"}
	files := []string{"", "app.go", "internal/auth/x.go", "pagos/f.php"}
	var c crConjunto
	n := r.Intn(7)
	for i := 0; i < n; i++ {
		c.Findings = append(c.Findings, crHallazgo(fmt.Sprintf("f%d", i), sevs[r.Intn(3)], files[r.Intn(len(files))]))
	}
	c.Subidas = map[string]finding.Subida{}
	for i := 0; i < n; i++ {
		if r.Intn(4) == 0 {
			id := fmt.Sprintf("f%d", i)
			c.Subidas[id] = finding.Subida{FindingID: id, Reason: "r", Author: "P"}
		}
	}
	c.Riesgo = []string{"", "bajo", "normal", "alto"}[r.Intn(4)]
	c.BlockOn = []string{"high", "medium", "low"}[r.Intn(3)]
	for k := r.Intn(3); k > 0; k-- {
		cob := []string{reviewcmd.CoberturaCompleta, reviewcmd.CoberturaDelta, reviewcmd.CoberturaParcial}[r.Intn(3)]
		var fs []string
		for i := 0; i < n; i++ {
			if r.Intn(2) == 0 {
				fs = append(fs, fmt.Sprintf("f%d", i))
			}
		}
		c.Reviews = append(c.Reviews, crRec(fmt.Sprintf("r%d", k), cob, "", reviewcmd.Lentes, fs...))
	}
	return reflect.ValueOf(c)
}

// CA-627 (propiedad): para un mismo conjunto de hallazgos, subidas, riesgo y
// registros, la tarjeta bloquea exactamente por los hallazgos que
// finding.Bloquea dice que bloquean, y sus pendientes son los demas medium y
// low; ningun high es pendiente; Texto vacio si y solo si no hay ninguno.
func TestCA627_PropiedadTarjetaEsBloquea(t *testing.T) {
	prop := func(cj crConjunto) bool {
		ev := crEv(cj.Riesgo, 50, 10, "a.go")
		ev.BlockOn = cj.BlockOn
		ev.Findings = cj.Findings
		ev.Subidas = cj.Subidas
		ev.Reviews = cj.Reviews
		ev = bdColas(ev)
		ctx := finding.Contexto{BlockOn: cj.BlockOn, Subidas: cj.Subidas, Riesgos: map[string]string{bdSlug: cj.Riesgo}}
		for _, r := range cj.Reviews {
			if r.Cobertura == reviewcmd.CoberturaCompleta {
				ctx.Completas = append(ctx.Completas, finding.RevisionCompleta{ID: r.ID, Task: r.Task, Findings: r.Findings})
			}
		}
		var quiero, pend []string
		med, low := 0, 0
		for _, f := range cj.Findings {
			if b, _ := finding.Bloquea(f.Finding, ctx); b {
				quiero = append(quiero, f.ID)
			} else {
				pend = append(pend, f.ID)
				if f.Severity == "medium" {
					med++
				} else if f.Severity == "low" {
					low++
				} else {
					return false
				}
			}
		}
		c := Derive(ev)
		got := append([]string(nil), c.Evidence.BlockingFindings...)
		sort.Strings(got)
		q := append([]string(nil), quiero...)
		sort.Strings(q)
		if len(got) != len(q) || (len(q) > 0 && !reflect.DeepEqual(got, q)) {
			t.Logf("bloquean %v, quiero %v", got, q)
			return false
		}
		p := c.Pendientes
		if p.Medium != med || p.Low != low || len(p.IDs) != len(pend) || (len(pend) > 0 && !reflect.DeepEqual(p.IDs, pend)) {
			t.Logf("pendientes %+v, quiero %v (%d medium, %d low)", p, pend, med, low)
			return false
		}
		return (p.Texto == "") == (len(pend) == 0)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 800}); err != nil {
		t.Fatalf("CA-627: %v", err)
	}
}

// ---------------------------------------------------------------- pendientes

// CA-629: la tarjeta trae pendientes con la cuenta, los ids en el orden de
// sus hallazgos y el texto, solo con las severidades presentes.
func TestCA629_PendientesDeLaTarjeta(t *testing.T) {
	var fs []finding.Item
	for i := 0; i < 3; i++ {
		fs = append(fs, crHallazgo(fmt.Sprintf("m%d", i), "medium", "app.go"))
	}
	for i := 0; i < 5; i++ {
		fs = append(fs, crHallazgo(fmt.Sprintf("l%d", i), "low", "app.go"))
	}
	fs = append(fs, crHallazgo("h0", "high", "app.go"))
	ev := crEv("normal", 50, 10, "a.go")
	ev.Findings = fs
	c := Derive(ev)
	p := c.Pendientes
	if p.Medium != 3 || p.Low != 5 || p.Texto != "pendientes: 3 medium, 5 low (no bloquean)" {
		t.Fatalf("CA-629: %+v", p)
	}
	if !reflect.DeepEqual(p.IDs, []string{"m0", "m1", "m2", "l0", "l1", "l2", "l3", "l4"}) {
		t.Fatalf("CA-629: los ids de los que no bloquean, en el orden de los hallazgos (un high nunca): %v", p.IDs)
	}
	for _, c := range []struct {
		fs    []finding.Item
		texto string
	}{
		{[]finding.Item{crHallazgo("l0", "low", "a.go"), crHallazgo("l1", "low", "a.go")}, "pendientes: 2 low (no bloquean)"},
		{[]finding.Item{crHallazgo("m0", "medium", "a.go")}, "pendientes: 1 medium (no bloquean)"},
		{[]finding.Item{crHallazgo("l0", "low", "a.go"), crHallazgo("m0", "medium", "a.go")}, "pendientes: 1 medium, 1 low (no bloquean)"},
		{[]finding.Item{crHallazgo("h0", "high", "a.go")}, ""},
		{nil, ""},
	} {
		ev := crEv("normal", 50, 10, "a.go")
		ev.Findings = c.fs
		if got := Derive(ev).Pendientes.Texto; got != c.texto {
			t.Fatalf("CA-629: el texto es %q, fue %q", c.texto, got)
		}
	}
}

// CA-629: pendientes esta siempre en el JSON de la tarjeta, con ids [] y
// texto "" sin ninguno.
func TestCA629_PendientesEnElJSON(t *testing.T) {
	raw, _ := json.Marshal(Derive(crEv("normal", 50, 10, "a.go")))
	if !strings.Contains(string(raw), `"pendientes":{"medium":0,"low":0,"ids":[],"texto":""}`) {
		t.Fatalf("CA-629: pendientes siempre presente, ids [] y texto vacio: %s", raw)
	}
	ev := crEv("normal", 50, 10, "a.go")
	ev.Findings = []finding.Item{crHallazgo("l0", "low", "a.go")}
	raw, _ = json.Marshal(Derive(ev))
	var m map[string]any
	_ = json.Unmarshal(raw, &m)
	p, ok := m["pendientes"].(map[string]any)
	if !ok || p["medium"] != 0.0 || p["low"] != 1.0 || !reflect.DeepEqual(p["ids"], []any{"l0"}) || p["texto"] != "pendientes: 1 low (no bloquean)" {
		t.Fatalf("CA-629: %v", m["pendientes"])
	}
	// MarshalJSON de Pendientes vacio
	raw, _ = json.Marshal(Pendientes{})
	if string(raw) != `{"medium":0,"low":0,"ids":[],"texto":""}` {
		t.Fatalf("CA-629: Pendientes{} emite ids []: %s", raw)
	}
}

// CA-629 (caso limite: una tarjeta en Hecho con pendientes): sigue
// mostrandolos.
func TestCA629_HechoSigueMostrandoPendientes(t *testing.T) {
	cuando := bdT0.Add(48 * time.Hour)
	ev := crEv("normal", 50, 10, "a.go")
	ev.Item.HechoEn = &cuando
	ev.Item.CommitFinal = "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c"
	ev.Findings = []finding.Item{crHallazgo("m0", "medium", "a.go")}
	c := Derive(ev)
	bdCol(t, "CA-629", c, ColHecho)
	if c.Pendientes.Medium != 1 || c.Pendientes.Texto != "pendientes: 1 medium (no bloquean)" {
		t.Fatalf("CA-629: en Hecho sigue mostrando los pendientes: %+v", c.Pendientes)
	}
	var b bytes.Buffer
	RenderCard(&b, c)
	if !strings.Contains(b.String(), "pendientes: 1 medium (no bloquean)") {
		t.Fatalf("CA-629: RenderCard muestra el texto:\n%s", b.String())
	}
}

// CA-629: RenderCard muestra la linea solo cuando no esta vacia.
func TestCA629_RenderCardSoloConPendientes(t *testing.T) {
	var b bytes.Buffer
	RenderCard(&b, Derive(crEv("normal", 50, 10, "a.go")))
	if strings.Contains(b.String(), "pendientes:") {
		t.Fatalf("CA-629: sin pendientes no hay linea:\n%s", b.String())
	}
	ev := crEv("normal", 50, 10, "a.go")
	ev.Findings = []finding.Item{crHallazgo("m0", "medium", "a.go"), crHallazgo("l0", "low", "a.go")}
	b.Reset()
	RenderCard(&b, Derive(ev))
	if !strings.Contains(b.String(), "pendientes: 1 medium, 1 low (no bloquean)") {
		t.Fatalf("CA-629: con pendientes RenderCard muestra la linea:\n%s", b.String())
	}
}

// ---------------------------------------------------------------- tope

// CA-633: el tablero muestra el motivo del tope con el texto llano y la
// accion del contrato.
func TestCA633_TarjetaConElTope(t *testing.T) {
	ev := crEv("normal", 50, 10, "a.go")
	msg := `aceptar "precios" dejaria 3 medium abiertos en el proyecto (tope 2): corrige o refuta alguno (hoom finding list --open) o sube findings.max_pendientes`
	ev.ReadyErr, ev.ReadyKind = msg, taskcmd.ReadyTopePendientes
	c := Derive(ev)
	bdCol(t, "CA-633", c, ColReview)
	bdPrimero(t, "CA-633", c, msg)
	if c.Plain != "aceptarla dejaria demasiados medium abiertos en el proyecto" {
		t.Fatalf("CA-633: el texto llano: %q", c.Plain)
	}
	if !strings.Contains(c.Next, "hoom finding list --open") {
		t.Fatalf("CA-633: la accion es hoom finding list --open: %q", c.Next)
	}
	if taskcmd.ReadyTopePendientes != "tope-pendientes" {
		t.Fatalf("CA-633: el kind: %q", taskcmd.ReadyTopePendientes)
	}
}

// ---------------------------------------------------------------- en disco

// CA-627 (en disco): para el mismo arbol, el gate findings_open con el spec
// de la tarea falla por los mismos ids que la tarjeta dice que bloquean, y
// la tarjeta trae los pendientes; CardFor, Build y el JSON del tablero
// llevan pendientes.
func TestCA627_GateYTarjetaNombranLosMismos(t *testing.T) {
	for _, r := range []string{"alto", "normal"} {
		root := bdRepo(t, "findings:\n  block_on: high\n")
		// el item va commiteado en main: el worktree de la tarea lo tiene,
		// asi el gate (que corre en el arbol de la tarea) y el tablero leen el
		// mismo riesgo
		bdItemArchivo(t, root, bdSlug, "Precios", "riesgo: "+r+"\n")
		bdCommitear(t, root, "item")
		wt := bdWorktree(t, root, bdSlug)
		bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
		bdEscribir(t, wt, "precios.go", "package app\n\nfunc Precio() int { return 1 }\n")
		bdAprobar(t, wt, bdSlug)
		bdCommitear(t, wt, "spec y codigo")
		bdVeredictoDisco(t, wt, bdSpec, bdT0, false, bdGatesVerdes())
		bdCommitear(t, wt, "veredicto")
		reg := func(sev, file string) finding.Finding {
			f, err := finding.Register(wt, "main", finding.Draft{Severity: sev, Lens: "risk", File: file,
				Description: "h " + sev, Author: "reviewer@test", Task: bdSlug})
			if err != nil {
				t.Fatal(err)
			}
			return f
		}
		alto := reg("high", "app.go")
		ruta := reg("medium", "internal/auth/x.go")
		compl := reg("medium", "app.go")
		subido := reg("low", "app.go")
		pend := reg("low", "app.go")
		if _, _, err := finding.Raise(wt, subido.ID, "rompe la factura"); err != nil {
			t.Fatal(err)
		}
		rec := reviewcmd.Record{ID: "20261110T100000_c0mp1a", CreatedAt: bdT0, Task: bdSlug, Spec: bdSpec,
			Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: reviewcmd.Lentes, Provider: "codex",
			Writer: "claude", Cross: reviewcmd.CrossYes, WritersDeclared: []string{}, Findings: []string{compl.ID},
			Usage: []reviewcmd.LensUsage{}, Cobertura: reviewcmd.CoberturaCompleta, Desde: bdDesde, Hasta: bdHasta}
		bdReviewDisco(t, wt, rec)
		bdCommitear(t, wt, "hallazgos, subida y registro")

		c, err := CardFor(root, "main", "high", bdSlug, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		quiero := []string{alto.ID, ruta.ID, subido.ID}
		pendientes := []string{compl.ID, pend.ID}
		if r == "alto" {
			quiero = []string{alto.ID, ruta.ID, compl.ID, subido.ID}
			pendientes = []string{pend.ID}
		}
		got := append([]string(nil), c.Evidence.BlockingFindings...)
		sort.Strings(got)
		sort.Strings(quiero)
		if !reflect.DeepEqual(got, quiero) {
			t.Fatalf("CA-627 (%s): la tarjeta bloquea por %v, fue %v", r, quiero, got)
		}
		if !reflect.DeepEqual(c.Pendientes.IDs, pendientes) {
			t.Fatalf("CA-627/CA-629 (%s): pendientes %v, fue %+v", r, pendientes, c.Pendientes)
		}
		bdCol(t, "CA-627 ("+r+")", c, ColReview)
		g := finding.Gate(wt, "main", "high", bdSpec)
		if g.Status != verdict.StatusFail || !strings.HasPrefix(g.Notes, fmt.Sprintf("%d abiertos que bloquean", len(quiero))) {
			t.Fatalf("CA-627 (%s): el gate falla por los mismos %d: %s %q", r, len(quiero), g.Status, g.Notes)
		}
		for _, id := range quiero {
			if !strings.Contains(g.Notes, id) {
				t.Fatalf("CA-627 (%s): el gate nombra %s: %q", r, id, g.Notes)
			}
		}
		for _, id := range pendientes {
			if strings.Contains(g.Notes, id) {
				t.Fatalf("CA-627 (%s): el gate no nombra al pendiente %s: %q", r, id, g.Notes)
			}
		}
		for _, m := range []string{"medium en ruta de riesgo: internal/auth/x.go", "subido a mano por "} {
			if !strings.Contains(g.OutputTail, m) {
				t.Fatalf("CA-627 (%s): el detalle del gate trae %q:\n%s", r, m, g.OutputTail)
			}
			bdTiene(t, "CA-627 ("+r+")", c.Missing, m)
		}
		// el tablero entero y su JSON llevan pendientes
		b, err := Build(root, "main", "high", time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		raw, err := JSONBytes(b)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"pendientes"`) || !strings.Contains(string(raw), pend.ID) {
			t.Fatalf("CA-629 (%s): hoom board --json lleva pendientes: %s", r, raw)
		}
	}
}
