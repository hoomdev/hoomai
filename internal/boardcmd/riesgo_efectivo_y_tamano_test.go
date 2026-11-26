// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// el tablero (CA-785, CA-786, CA-787): la review requerida sale del riesgo de
// la tarjeta (el mas alto entre el efectivo de su ultimo veredicto y el
// declarado de ahora) y de las lineas de producto, nunca de las de git; el
// segmento no-aplica trae DetalleNoAplica; un veredicto anterior al spec se
// mide con las reglas sobre sus changed_files y con insertions + deletions;
// ` · riesgo <r>` en `hoom board` y `hoom item show` con el efectivo; el
// JSON con riesgo y riesgo_motivo y sin tamano; y los hallazgos de la tarjeta
// juzgados con las rutas del clasificador del proyecto y con el riesgo
// efectivo. Escritos desde el spec, las aclaraciones de la ronda 0 y las
// firmas publicas, sin leer la implementacion.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	retyMotAuth = "el cambio toca internal/auth/x.go: riesgo alto"
	retyMotApp  = "el cambio toca a.go, fuera de las rutas de bajo riesgo: riesgo normal"
	retyLaUna   = "la lente risk"
	retyLas2    = "2 lentes"
	retyLas4    = "4 lentes"
)

var retyDocs = []string{"README.md", "docs/guia.md", "LICENSE"}

// retyEv es una tarjeta con el riesgo declarado decl y un veredicto nuevo
// (con riesgo y tamano) de efectivo ef y motivo mot, p lineas de producto,
// sobre files. git dice +9300/-81: si el tablero mirara git, todo cambio
// seria grande.
func retyEv(decl, ef, mot string, p int, files ...string) Evidence {
	ev := bdEv()
	ev.Item.Riesgo = decl
	d := decl
	if d == "" {
		d = "normal"
	}
	v := &verdict.Verdict{ID: "v-cr", CreatedAt: bdT0.Add(30 * time.Minute), Spec: bdSpec,
		Git:    gitx.Info{IsRepo: true, ChangeFingerprint: "huella-1", Insertions: 9300, Deletions: 81, ChangedFiles: files},
		Gates:  bdGatesVerdes(),
		Riesgo: &riesgo.Evaluacion{Declarado: d, Efectivo: ef, Motivo: mot},
		Tamano: &riesgo.Tamano{Producto: p, Pruebas: 3000, Hoom: 40}}
	v.Finalize()
	ev.Verdict, ev.GreenVerdicts = v, []string{v.ID}
	return ev
}

// retyFalta4 es el motivo de hoy de una review que falta por tamano, con el
// numero de producto (aclaracion 18).
func retyFalta4(p int) string {
	return fmt.Sprintf("la review exige 2 lentes (risk y reliability, %d lineas > 400) y no hay registro de review", p)
}

func retyNoAplica(p int, lentes string) string {
	return fmt.Sprintf("el cambio es chico (%d lineas de producto): no exige la revision de %s", p, lentes)
}

func retyCompleta(ev Evidence, lentes []string) Evidence {
	ev.Reviews = []reviewcmd.Record{crRec("c1", reviewcmd.CoberturaCompleta, "", lentes)}
	return bdColas(ev)
}

// retyExige mira la review requerida de c: si exige, la columna Review con
// el motivo principal; si no, Tu aceptacion (bdEv cumple todo lo demas).
func retyExige(t *testing.T, ca string, c Card, exige bool, motivo string) {
	t.Helper()
	if c.Evidence.ReviewRequired != exige {
		t.Fatalf("%s: review_required debe ser %v: %+v", ca, exige, c.Evidence)
	}
	if exige {
		bdCol(t, ca, c, ColReview)
		if motivo != "" {
			bdPrimero(t, ca, c, motivo)
		}
		return
	}
	bdCol(t, ca, c, ColTuAceptacion)
}

// CA-785: el texto exacto de no-aplica.
func TestCA785_DetalleNoAplicaExacto(t *testing.T) {
	if DetalleNoAplica != "el cambio es chico (%d lineas de producto): no exige la revision de %s" {
		t.Fatalf("CA-785: DetalleNoAplica es %q", DetalleNoAplica)
	}
	tbSegEs(t, "CA-785", Derive(retyEv("normal", "normal", "declarado normal", 90, "a.go", "a_test.go")),
		"review", SegNoAplica, "el cambio es chico (90 lineas de producto): no exige la revision de 2 lentes")
	tbSegEs(t, "CA-785", Derive(retyEv("bajo", "bajo", "declarado bajo", 400, "internal/precio_test.go")),
		"review", SegNoAplica, "el cambio es chico (400 lineas de producto): no exige la revision de la lente risk")
}

// CA-785: la review requerida con un veredicto nuevo: alto siempre salvo
// vacio o solo documentacion; bajo y normal con mas de 400 de producto y un
// cambio que no es solo documentacion. Las inserciones de git (9381) no
// cuentan nunca.
func TestCA785_LaReviewRequeridaSaleDelEfectivoYDelProducto(t *testing.T) {
	type caso struct {
		nombre         string
		decl, ef, mot  string
		ahora          string // "-" = el declarado del veredicto; si no, el item dice esto ahora
		p              int
		files          []string
		exige          bool
		motivo         string
		estado, detail string
	}
	casos := []caso{
		{"alto chico", "normal", "alto", retyMotAuth, "-", 10, []string{"internal/auth/x.go"}, true, crMotAlto, SegFalta, "falta la revision de 4 lentes"},
		{"alto de cero lineas", "normal", "alto", retyMotAuth, "-", 0, []string{"internal/auth/x.go"}, true, crMotAlto, SegFalta, "falta la revision de 4 lentes"},
		{"alto de solo docs", "alto", "alto", "declarado alto", "-", 10, retyDocs, false, "", SegNoAplica, retyNoAplica(10, retyLas4)},
		{"alto de un doc en ruta de riesgo", "normal", "alto", "el cambio toca docs/auth.md: riesgo alto", "-", 30, []string{"docs/auth.md"}, false, "", SegNoAplica, retyNoAplica(30, retyLas4)},
		{"alto sin archivos", "alto", "alto", "declarado alto", "-", 900, nil, false, "", SegNoAplica, retyNoAplica(900, retyLas4)},
		{"normal 401", "normal", "normal", "declarado normal", "-", 401, []string{"a.go"}, true, retyFalta4(401), SegFalta, "falta la revision de 2 lentes"},
		{"normal 400", "normal", "normal", "declarado normal", "-", 400, []string{"a.go"}, false, "", SegNoAplica, retyNoAplica(400, retyLas2)},
		{"normal 90 de producto y 3000 de pruebas", "", "normal", "declarado normal", "-", 90, []string{"a.go", "a_test.go"}, false, "", SegNoAplica, retyNoAplica(90, retyLas2)},
		{"normal 5000 de solo docs", "normal", "normal", "declarado normal", "-", 5000, retyDocs, false, "", SegNoAplica, retyNoAplica(5000, retyLas2)},
		{"bajo 401", "bajo", "bajo", "declarado bajo", "-", 401, []string{"internal/precio_test.go"}, true, crMotBajo(401), SegFalta, "falta la revision de la lente risk"},
		{"bajo 400", "bajo", "bajo", "declarado bajo", "-", 400, []string{"internal/precio_test.go"}, false, "", SegNoAplica, retyNoAplica(400, retyLaUna)},
		{"bajo 900 de solo docs", "bajo", "bajo", "declarado bajo", "-", 900, retyDocs, false, "", SegNoAplica, retyNoAplica(900, retyLaUna)},
		{"bajo que sube a normal, 401", "bajo", "normal", retyMotApp, "-", 401, []string{"a.go"}, true, retyFalta4(401), SegFalta, "falta la revision de 2 lentes"},
		{"bajo que sube a normal, 60", "bajo", "normal", retyMotApp, "-", 60, []string{"a.go"}, false, "", SegNoAplica, retyNoAplica(60, retyLas2)},
		{"subir el declarado a alto se ve enseguida", "normal", "normal", "declarado normal", "alto", 10, []string{"a.go"}, true, crMotAlto, SegFalta, "falta la revision de 4 lentes"},
		{"bajar el declarado espera al verify", "normal", "alto", retyMotAuth, "bajo", 10, []string{"internal/auth/x.go"}, true, crMotAlto, SegFalta, "falta la revision de 4 lentes"},
		{"subir de bajo a normal pide 2 lentes", "bajo", "bajo", "declarado bajo", "normal", 401, []string{"internal/precio_test.go"}, true, retyFalta4(401), SegFalta, "falta la revision de 2 lentes"},
	}
	for _, k := range casos {
		ca := "CA-785 (" + k.nombre + ")"
		ev := retyEv(k.decl, k.ef, k.mot, k.p, k.files...)
		if k.ahora != "-" {
			ev.Item.Riesgo = k.ahora
		}
		c := Derive(ev)
		retyExige(t, ca, c, k.exige, k.motivo)
		tbSegEs(t, ca, c, "review", k.estado, k.detail)
	}
}

// CA-785: la cadena cuenta con las lentes del riesgo de la tarjeta: en bajo,
// reliability alcanza; en normal y alto, no.
func TestCA785_LaCadenaConElRiesgoDeLaTarjeta(t *testing.T) {
	una := []string{"risk"}
	dos := reviewcmd.LentesNormalGrande
	// bajo con 401 de producto: una completa de risk cuenta
	c := Derive(retyCompleta(retyEv("bajo", "bajo", "declarado bajo", 401, "internal/precio_test.go"), una))
	bdCol(t, "CA-785 (bajo)", c, ColTuAceptacion)
	if c.Evidence.ReviewID != "c1" || !c.Evidence.ReviewRequired {
		t.Fatalf("CA-785: con bajo cuenta la completa de risk: %+v", c.Evidence)
	}
	tbSegEs(t, "CA-785 (bajo)", c, "review", SegHecho, "revision de la lente risk registrada")
	// la misma completa de 1 lente NO cuenta si el veredicto subio la tarjeta a normal (>400 exige 2 lentes)
	c = Derive(retyCompleta(retyEv("bajo", "normal", retyMotApp, 401, "a.go"), una))
	retyExige(t, "CA-785 (bajo que sube a normal con 1 lente)", c, true, retyFalta4(401))
	tbSegEs(t, "CA-785", c, "review", SegFalta, "falta la revision de 2 lentes")
	// con 2 lentes (risk y reliability) si cuenta en normal >400
	c = Derive(retyCompleta(retyEv("bajo", "normal", retyMotApp, 401, "a.go"), dos))
	bdCol(t, "CA-785 (bajo que sube a normal con 2 lentes)", c, ColTuAceptacion)
	if c.Evidence.ReviewID != "c1" {
		t.Fatalf("CA-785: con efectivo normal, 2 lentes cuentan: %+v", c.Evidence)
	}
	tbSegEs(t, "CA-785", c, "review", SegHecho, "revision de 2 lentes registrada")
	// ni si el veredicto la subio a alto, aunque el cambio sea chico
	c = Derive(retyCompleta(retyEv("bajo", "alto", "el cambio toca internal/auth/x_test.go: riesgo alto", 10, "internal/auth/x_test.go"), una))
	retyExige(t, "CA-785 (bajo que sube a alto)", c, true, crMotAlto)
	// alto chico con las 4 lentes: cuenta
	c = Derive(retyCompleta(retyEv("normal", "alto", retyMotAuth, 10, "internal/auth/x.go"), reviewcmd.Lentes))
	bdCol(t, "CA-785 (alto con 4)", c, ColTuAceptacion)
	if c.Evidence.ReviewID != "c1" || !c.Evidence.ReviewRequired {
		t.Fatalf("CA-785: alto con 4 lentes cuenta: %+v", c.Evidence)
	}
	tbSegEs(t, "CA-785 (alto con 4)", c, "review", SegHecho, "revision de 4 lentes registrada")
	// subir el declarado a alto despues de una review de reliability: vuelve a Review
	ev := retyCompleta(retyEv("bajo", "bajo", "declarado bajo", 401, "internal/precio_test.go"), una)
	ev.Item.Riesgo = "alto"
	retyExige(t, "CA-785 (bajo a alto)", Derive(ev), true, crMotAlto)
}

// CA-785 (caso limite): un veredicto anterior al spec (sin riesgo ni tamano)
// toma insertions + deletions como producto, aunque sean pruebas, y su riesgo
// sale de las reglas sobre changed_files con el clasificador del proyecto.
func TestCA785_UnVeredictoViejoSeMideConLasReglas(t *testing.T) {
	// producto = ins + del, sin mirar la clase de los archivos
	retyExige(t, "CA-785 (viejo 401 de pruebas)", Derive(crEv("normal", 300, 101, "a_test.go")), true, retyFalta4(401))
	c := Derive(crEv("normal", 300, 100, "a.go"))
	retyExige(t, "CA-785 (viejo 400)", c, false, "")
	tbSegEs(t, "CA-785 (viejo 400)", c, "review", SegNoAplica, retyNoAplica(400, retyLas2))
	// bajo que toca solo pruebas se sostiene
	retyExige(t, "CA-785 (viejo bajo grande)", Derive(crEv("bajo", 500, 112, "internal/precio_test.go")), true, crMotBajo(612))
	c = Derive(retyCompleta(crEv("bajo", 500, 112, "internal/precio_test.go"), []string{"risk"}))
	bdCol(t, "CA-785 (viejo bajo con risk)", c, ColTuAceptacion)
	tbSegEs(t, "CA-785 (viejo bajo chico)", Derive(crEv("bajo", 50, 10, "internal/precio_test.go")), "review", SegNoAplica, retyNoAplica(60, retyLaUna))
	// bajo que toca .hoom/ se sostiene, y un .md es solo documentacion
	tbSegEs(t, "CA-785 (viejo bajo en .hoom)", Derive(crEv("bajo", 500, 112, ".hoom/specs/auth.md")), "review", SegNoAplica, retyNoAplica(612, retyLaUna))
	// bajo con producto fuera de las rutas de bajo riesgo: normal
	c = Derive(crEv("bajo", 50, 10, "a.go"))
	retyExige(t, "CA-785 (viejo bajo con a.go)", c, false, "")
	tbSegEs(t, "CA-785 (viejo bajo con a.go)", c, "review", SegNoAplica, retyNoAplica(60, retyLas2))
	// ruta de riesgo, en producto o en una prueba: alto
	retyExige(t, "CA-785 (viejo con auth)", Derive(crEv("", 50, 10, "internal/auth/x.go")), true, crMotAlto)
	retyExige(t, "CA-785 (viejo bajo con una prueba de auth)", Derive(crEv("bajo", 50, 10, "internal/auth/x_test.go")), true, crMotAlto)

	// con el clasificador del proyecto
	base := riesgo.RutasBase()
	propio := &riesgo.Clasificador{Rutas: riesgo.Rutas{
		DeRiesgo:     []string{"cobros"},
		DeBajoRiesgo: append(base.DeBajoRiesgo, "lang"),
		Generadas:    []string{"vendor"},
	}, Pruebas: []string{"**/*_test.go"}}
	for _, k := range []struct {
		nombre             string
		decl, file         string
		conPropio, conNada bool // exige review con el clasificador propio / con nil
	}{
		{"ruta propia", "normal", "cobros/f.go", true, false},
		{"un generado no sube", "normal", "vendor/auth/x.go", false, true},
		{"de bajo riesgo propia", "bajo", "lang/es.php", false, false},
	} {
		ev := crEv(k.decl, 50, 10, k.file)
		ev.Clasificador = propio
		retyExige(t, "CA-785 ("+k.nombre+", propio)", Derive(ev), k.conPropio, "")
		ev.Clasificador = nil
		retyExige(t, "CA-785 ("+k.nombre+", sin hoom.yaml)", Derive(ev), k.conNada, "")
	}
	ev := crEv("bajo", 500, 112, "lang/es.php")
	ev.Clasificador = propio
	retyExige(t, "CA-785 (lang con el propio)", Derive(ev), true, crMotBajo(612))
	ev.Clasificador = nil
	retyExige(t, "CA-785 (lang sin hoom.yaml)", Derive(ev), true, retyFalta4(612))
}

// retyOrden es el orden del vocabulario ("" es normal).
var retyOrden = map[string]int{"bajo": 0, "": 1, "normal": 1, "alto": 2}

// CA-785/CA-786 (propiedad, exhaustiva): para todo declarado de ahora,
// efectivo del veredicto, producto y archivos, el riesgo de la tarjeta es el
// mas alto de los dos (con el motivo del veredicto en un empate), y la review
// se exige si y solo si el cambio no es solo documentacion y la tarjeta es
// alto o pasa de 400 lineas de producto; el segmento lo dice con las lentes
// de ese riesgo.
func TestCA785_PropiedadRequeridaYRiesgo(t *testing.T) {
	motivos := map[string]string{"alto": retyMotAuth, "normal": "declarado normal", "bajo": "declarado bajo"}
	conjuntos := []struct {
		files []string
		docs  bool
	}{
		{nil, true},
		{[]string{"README.md"}, true},
		{[]string{"docs/guia.md", "LICENSE", "NOTICE", "notas.TXT", "manual.rst", "x.adoc"}, true},
		{[]string{"a.go"}, false},
		{[]string{"README.md", "a.go"}, false},
		{[]string{"internal/precio_test.go"}, false},
	}
	for _, ahora := range []string{"", "bajo", "normal", "alto"} {
		for _, ef := range []string{"bajo", "normal", "alto"} {
			for _, p := range []int{0, 1, 399, 400, 401, 402, 5000} {
				for i, cj := range conjuntos {
					declV := "normal"
					if ef == "bajo" {
						declV = "bajo"
					}
					ev := retyEv(declV, ef, motivos[ef], p, cj.files...)
					ev.Item.Riesgo = ahora
					c := Derive(ev)
					ca := fmt.Sprintf("CA-785 (ahora %q, efectivo %s, producto %d, archivos %d)", ahora, ef, p, i)
					tarjeta, motivo := ef, motivos[ef]
					if retyOrden[ahora] > retyOrden[ef] {
						tarjeta = ahora
						if tarjeta == "" {
							tarjeta = "normal"
						}
						motivo = "declarado " + tarjeta
					}
					if c.Evidence.Riesgo != tarjeta || c.Evidence.RiesgoMotivo != motivo {
						t.Fatalf("%s: el riesgo de la tarjeta es %s (%q): %q (%q)", ca, tarjeta, motivo, c.Evidence.Riesgo, c.Evidence.RiesgoMotivo)
					}
					exige := !cj.docs && (tarjeta == "alto" || p > 400)
					lentes := retyLas2
					if tarjeta == "bajo" {
						lentes = retyLaUna
					} else if tarjeta == "alto" {
						lentes = retyLas4
					}
					retyExige(t, ca, c, exige, "")
					if exige {
						tbSegEs(t, ca, c, "review", SegFalta, "falta la revision de "+lentes)
					} else {
						tbSegEs(t, ca, c, "review", SegNoAplica, retyNoAplica(p, lentes))
					}
					if c.Evidence.Tamano == nil || *c.Evidence.Tamano != (riesgo.Tamano{Producto: p, Pruebas: 3000, Hoom: 40}) {
						t.Fatalf("%s: el tamano de la tarjeta es el del veredicto: %+v", ca, c.Evidence.Tamano)
					}
				}
			}
		}
	}
}

// CA-786: el riesgo de la tarjeta y su motivo, sin veredicto, con uno nuevo
// y con uno anterior al spec; el tamano solo con uno nuevo.
func TestCA786_ElRiesgoDeLaTarjeta(t *testing.T) {
	sin := func(r, tipo string) Evidence {
		ev := bdEv()
		ev.Item.Riesgo = r
		if tipo != "" {
			ev.Item.Tipo = tipo
		}
		ev.Verdict, ev.GreenVerdicts = nil, nil
		return ev
	}
	nuevo := func(decl, ef, mot, ahora string) Evidence {
		ev := retyEv(decl, ef, mot, 10, "a.go")
		ev.Item.Riesgo = ahora
		return ev
	}
	for _, k := range []struct {
		nombre         string
		ev             Evidence
		riesgo, motivo string
		conTamano      bool
	}{
		{"sin veredicto, alto", sin("alto", ""), "alto", "declarado alto", false},
		{"sin veredicto, sin riesgo", sin("", ""), "normal", "declarado normal", false},
		{"sin veredicto, bajo", sin("bajo", ""), "bajo", "declarado bajo", false},
		{"sin veredicto, seguridad al pie de la letra", sin("", "seguridad"), "normal", "declarado normal", false},
		{"nuevo, el veredicto sube", nuevo("normal", "alto", retyMotAuth, "normal"), "alto", retyMotAuth, true},
		{"nuevo, empate: el motivo del veredicto", nuevo("normal", "alto", retyMotAuth, "alto"), "alto", retyMotAuth, true},
		{"nuevo, gana el declarado", nuevo("normal", "normal", "declarado normal", "alto"), "alto", "declarado alto", true},
		{"nuevo, bajar no se ve", nuevo("normal", "alto", retyMotAuth, "bajo"), "alto", retyMotAuth, true},
		{"nuevo, bajo que el veredicto subio", nuevo("bajo", "normal", retyMotApp, "bajo"), "normal", retyMotApp, true},
		{"nuevo, normal sobre un bajo", nuevo("bajo", "bajo", "declarado bajo", "normal"), "normal", "declarado normal", true},
		{"nuevo, sin riesgo sobre un bajo", nuevo("bajo", "bajo", "declarado bajo", ""), "normal", "declarado normal", true},
		{"viejo, bajo con a.go", crEv("bajo", 50, 10, "a.go"), "normal", retyMotApp, false},
		{"viejo, auth", crEv("", 50, 10, "internal/auth/x.go"), "alto", retyMotAuth, false},
		{"viejo, normal", crEv("normal", 50, 10, "a.go"), "normal", "declarado normal", false},
		{"viejo, bajo con pruebas", crEv("bajo", 50, 10, "internal/precio_test.go"), "bajo", "declarado bajo", false},
		{"viejo, la primera ruta en orden de bytes", crEv("bajo", 50, 10, "a/token.go", "a.go", "B/billing.go"), "alto", "el cambio toca B/billing.go: riesgo alto", false},
	} {
		ca := "CA-786 (" + k.nombre + ")"
		c := Derive(k.ev)
		if c.Evidence.Riesgo != k.riesgo || c.Evidence.RiesgoMotivo != k.motivo {
			t.Fatalf("%s: riesgo %q (%q), fue %q (%q)", ca, k.riesgo, k.motivo, c.Evidence.Riesgo, c.Evidence.RiesgoMotivo)
		}
		if k.conTamano {
			if c.Evidence.Tamano == nil || *c.Evidence.Tamano != *k.ev.Verdict.Tamano {
				t.Fatalf("%s: el tamano es el del veredicto: %+v", ca, c.Evidence.Tamano)
			}
		} else if c.Evidence.Tamano != nil {
			t.Fatalf("%s: sin veredicto nuevo no hay tamano: %+v", ca, *c.Evidence.Tamano)
		}
		// El item sigue con su declarado: el efectivo es de la evidencia.
		if c.Item.Riesgo != k.ev.Item.Riesgo {
			t.Fatalf("%s: el item conserva el riesgo declarado %q: %q", ca, k.ev.Item.Riesgo, c.Item.Riesgo)
		}
	}
}

// retyEvidenciaJSON es el objeto evidence de una tarjeta en JSON.
func retyEvidenciaJSON(t *testing.T, ca string, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: JSON de la tarjeta: %v\n%s", ca, err, raw)
	}
	e, ok := m["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("%s: la tarjeta trae evidence: %s", ca, raw)
	}
	return e
}

// CA-786 y aclaracion 23: el JSON de la tarjeta (el de `hoom item show
// --json`) y el del tablero (`hoom board --json`, /api/board) traen riesgo y
// riesgo_motivo, y nunca tamano.
func TestCA786_ElJSONTraeRiesgoYMotivoSinTamano(t *testing.T) {
	const ca = "CA-786"
	c := Derive(retyEv("normal", "alto", retyMotAuth, 10, "internal/auth/x.go"))
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	e := retyEvidenciaJSON(t, ca, raw)
	if e["riesgo"] != "alto" || e["riesgo_motivo"] != retyMotAuth {
		t.Fatalf("%s: evidence trae riesgo y riesgo_motivo: %v", ca, e)
	}
	if _, ok := e["tamano"]; ok {
		t.Fatalf("%s: el tamano no va en el JSON del tablero: %v", ca, e)
	}
	if strings.Contains(string(raw), `"producto"`) || strings.Contains(string(raw), `"tamano"`) {
		t.Fatalf("%s: ni el tamano ni sus numeros van en el JSON: %s", ca, raw)
	}
	b, err := JSONBytes(bdTablero(c))
	if err != nil {
		t.Fatal(err)
	}
	var tab struct {
		Columns []struct {
			Cards []json.RawMessage `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(b, &tab); err != nil {
		t.Fatalf("%s: JSON del tablero: %v\n%s", ca, err, b)
	}
	n := 0
	for _, col := range tab.Columns {
		for _, card := range col.Cards {
			n++
			e := retyEvidenciaJSON(t, ca, card)
			if e["riesgo"] != "alto" || e["riesgo_motivo"] != retyMotAuth {
				t.Fatalf("%s: la tarjeta del tablero trae riesgo y riesgo_motivo: %v", ca, e)
			}
			if _, ok := e["tamano"]; ok {
				t.Fatalf("%s: el tamano no va en el JSON del tablero: %v", ca, e)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%s: el tablero tiene la tarjeta: %s", ca, b)
	}
	// normal tambien lleva su riesgo y su motivo
	raw, _ = json.Marshal(Derive(retyEv("bajo", "normal", retyMotApp, 10, "a.go")))
	if e := retyEvidenciaJSON(t, ca, raw); e["riesgo"] != "normal" || e["riesgo_motivo"] != retyMotApp {
		t.Fatalf("%s: con normal el JSON trae riesgo y riesgo_motivo: %v", ca, e)
	}
}

// CA-786 y aclaracion 17: ` · riesgo <efectivo>` va despues del titulo en
// `hoom board` y en la linea de tipo y prioridad de `hoom item show`, solo
// cuando no es normal; con efectivo normal no sale aunque el item declare
// bajo (hoy el tablero muestra el declarado).
func TestCA786_BoardEItemShowConElEfectivo(t *testing.T) {
	sinVeredicto := bdEv()
	sinVeredicto.Item.Riesgo = "alto"
	sinVeredicto.Verdict, sinVeredicto.GreenVerdicts = nil, nil
	for _, k := range []struct {
		nombre string
		ev     Evidence
		quiero string // "" = sin el borde
	}{
		{"el veredicto sube a alto", retyEv("normal", "alto", retyMotAuth, 10, "internal/auth/x.go"), "alto"},
		{"bajo que el veredicto subio a normal", retyEv("bajo", "normal", retyMotApp, 10, "a.go"), ""},
		{"bajo que se sostiene", retyEv("bajo", "bajo", "declarado bajo", 10, "internal/precio_test.go"), "bajo"},
		{"sin veredicto, alto declarado", sinVeredicto, "alto"},
		{"viejo con auth", crEv("", 50, 10, "internal/auth/x.go"), "alto"},
		{"viejo bajo con a.go", crEv("bajo", 50, 10, "a.go"), ""},
	} {
		ca := "CA-786 (" + k.nombre + ")"
		c := Derive(k.ev)
		var tab, card bytes.Buffer
		Render(&tab, bdTablero(c))
		RenderCard(&card, c)
		for nombre, s := range map[string]string{"hoom board": tab.String(), "hoom item show": card.String()} {
			if strings.Contains(s, "· riesgo normal") {
				t.Fatalf("%s: %s nunca dice `· riesgo normal`:\n%s", ca, nombre, s)
			}
			if k.quiero == "" && strings.Contains(s, "· riesgo") {
				t.Fatalf("%s: %s no muestra el riesgo con efectivo normal:\n%s", ca, nombre, s)
			}
		}
		if k.quiero == "" {
			continue
		}
		borde := " · riesgo " + k.quiero
		linea := ""
		for _, l := range strings.Split(tab.String(), "\n") {
			if strings.Contains(l, "Precios por region") {
				linea = l
				break
			}
		}
		if i := strings.Index(linea, borde); i < 0 || i < strings.Index(linea, "Precios por region") {
			t.Fatalf("%s: en hoom board, %q va despues del titulo: %q\n%s", ca, borde, linea, tab.String())
		}
		linea = ""
		for _, l := range strings.Split(card.String(), "\n") {
			if strings.Contains(l, "feature") && strings.Contains(l, "media") {
				linea = l
				break
			}
		}
		if !strings.Contains(linea, borde) {
			t.Fatalf("%s: en hoom item show, %q va en la linea de tipo y prioridad: %q\n%s", ca, borde, linea, card.String())
		}
	}
}

// CA-787: la tarjeta juzga el medium «en ruta de riesgo» con las rutas del
// clasificador del proyecto (Evidence.Clasificador; nil = RutasBase), al pie
// de la letra tambien para .hoom/ y generados (aclaracion 20), y el motivo no
// cambia.
func TestCA787_LaTarjetaJuzgaConLasRutasDelProyecto(t *testing.T) {
	type caso struct {
		nombre    string
		c         *riesgo.Clasificador
		bloquean  []string
		pendiente []string
		motivos   []string
	}
	base := riesgo.RutasBase()
	hallazgos := []finding.Item{
		crHallazgo("f-cobros", "medium", "cobros/f.go"),
		crHallazgo("f-auth", "medium", "internal/auth/login.go"),
		crHallazgo("f-design", "medium", "app/DESIGN.kt"),
		crHallazgo("f-signin", "medium", "internal/signin/x.go"),
		crHallazgo("f-hoom", "medium", ".hoom/specs/auth.md"),
		crHallazgo("f-vendor", "medium", "vendor/auth/x.go"),
		crHallazgo("f-mayus", "medium", "Internal/AUTH/X.go"),
	}
	casos := []caso{
		{"sin hoom.yaml", nil,
			[]string{"f-auth", "f-signin", "f-hoom", "f-vendor", "f-mayus"}, []string{"f-cobros", "f-design"},
			[]string{"medium en ruta de riesgo: internal/auth/login.go", "medium en ruta de riesgo: .hoom/specs/auth.md", "medium en ruta de riesgo: vendor/auth/x.go"}},
		{"de_riesgo propia", &riesgo.Clasificador{Rutas: riesgo.Rutas{DeRiesgo: []string{"cobros"}, DeBajoRiesgo: base.DeBajoRiesgo}},
			[]string{"f-cobros"}, []string{"f-auth", "f-design", "f-signin", "f-hoom", "f-vendor", "f-mayus"},
			[]string{"medium en ruta de riesgo: cobros/f.go"}},
		{"de_riesgo vacia", &riesgo.Clasificador{Rutas: riesgo.Rutas{DeRiesgo: []string{}, DeBajoRiesgo: base.DeBajoRiesgo}},
			nil, []string{"f-cobros", "f-auth", "f-design", "f-signin", "f-hoom", "f-vendor", "f-mayus"}, nil},
		{"generado con las marcas", &riesgo.Clasificador{Rutas: riesgo.Rutas{DeRiesgo: base.DeRiesgo, DeBajoRiesgo: base.DeBajoRiesgo, Generadas: []string{"vendor"}}},
			[]string{"f-auth", "f-signin", "f-hoom", "f-vendor", "f-mayus"}, []string{"f-cobros", "f-design"},
			[]string{"medium en ruta de riesgo: vendor/auth/x.go"}},
	}
	for _, k := range casos {
		ca := "CA-787 (" + k.nombre + ")"
		ev := crEv("normal", 50, 10, "a.go")
		ev.Clasificador = k.c
		ev.Findings = hallazgos
		c := Derive(ev)
		if !retyMismos(c.Evidence.BlockingFindings, k.bloquean) {
			t.Fatalf("%s: bloquean %v, fueron %v", ca, k.bloquean, c.Evidence.BlockingFindings)
		}
		if !retyMismos(c.Pendientes.IDs, k.pendiente) {
			t.Fatalf("%s: pendientes %v, fueron %+v", ca, k.pendiente, c.Pendientes)
		}
		for _, m := range k.motivos {
			bdTiene(t, ca, c.Missing, m)
		}
		if len(k.bloquean) == 0 {
			bdCol(t, ca, c, ColTuAceptacion)
		} else {
			bdCol(t, ca, c, ColReview)
		}
	}
}

func retyMismos(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ma := map[string]int{}
	for _, x := range a {
		ma[x]++
	}
	for _, x := range b {
		ma[x]--
	}
	for _, n := range ma {
		if n != 0 {
			return false
		}
	}
	return true
}

// CA-787: el medium de una review completa bloquea la tarjeta con su riesgo
// efectivo alto, tambien con el declarado normal; con efectivo normal no.
func TestCA787_ElMediumDeLaCompletaConElEfectivo(t *testing.T) {
	conCompleta := func(ev Evidence) Evidence {
		c1 := crRec("c1", reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes, "m-compl")
		ev.Reviews = []reviewcmd.Record{c1}
		ev = bdColas(ev)
		ev.Findings = []finding.Item{crHallazgo("m-compl", "medium", "app.go")}
		return ev
	}
	for _, k := range []struct {
		nombre  string
		ev      Evidence
		bloquea bool
	}{
		{"veredicto alto, declarado normal", retyEv("normal", "alto", retyMotAuth, 10, "internal/auth/x.go"), true},
		{"veredicto alto, declarado bajado a bajo", func() Evidence {
			e := retyEv("normal", "alto", retyMotAuth, 10, "internal/auth/x.go")
			e.Item.Riesgo = "bajo"
			return e
		}(), true},
		{"veredicto normal, declarado normal", retyEv("normal", "normal", "declarado normal", 10, "a.go"), false},
		{"veredicto viejo con auth, sin riesgo declarado", crEv("", 50, 10, "internal/auth/x.go"), true},
		{"veredicto viejo bajo con a.go", crEv("bajo", 50, 10, "a.go"), false},
	} {
		ca := "CA-787 (" + k.nombre + ")"
		c := Derive(conCompleta(k.ev))
		if !k.bloquea {
			if len(c.Evidence.BlockingFindings) != 0 || !reflect.DeepEqual(c.Pendientes.IDs, []string{"m-compl"}) {
				t.Fatalf("%s: el medium queda pendiente: %v %+v", ca, c.Evidence.BlockingFindings, c.Pendientes)
			}
			continue
		}
		if !reflect.DeepEqual(c.Evidence.BlockingFindings, []string{"m-compl"}) {
			t.Fatalf("%s: bloquea el medium de la completa: %v", ca, c.Evidence.BlockingFindings)
		}
		bdTiene(t, ca, c.Missing, "medium de la review completa c1: riesgo alto")
		bdCol(t, ca, c, ColReview)
	}
}
