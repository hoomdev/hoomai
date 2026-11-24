// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre el
// paquete spec: la medida (CA-791), el rojo de spec_lint por el tope
// (CA-793), la aprobacion vigente que no se mide (CA-794), la linea de
// `hoom spec status` (CA-797), las citas de spec_trace (CA-801) y el aviso
// de citas (CA-802). Escritos desde el spec, las aclaraciones de la ronda 0 y
// las firmas publicas del esqueleto, sin leer la implementacion.
package spec

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// pprEsqueleto es un spec que pasa Lint: %CONTRATOS% y %CRITERIOS% se
// reemplazan.
const pprEsqueleto = "# Spec de prueba\n## Objetivo\nx\n## No-goals\nx\n## Contratos\n%CONTRATOS%\n## Casos limite\nx\n## Criterios de aceptacion\n%CRITERIOS%\n## Decisiones\nx\n## Riesgos\nx\n"

// pprPorQue es el `<por que>` literal de las lineas del tope.
const pprPorQue = `--motivo "<por que>"`

// pprCriterios son k items `- CA-i: criterio i.`, i desde 1.
func pprCriterios(k int) string {
	var b strings.Builder
	for i := 1; i <= k; i++ {
		fmt.Fprintf(&b, "- CA-%d: criterio %d.\n", i, i)
	}
	return b.String()
}

// pprTexto arma el spec con criterios dados y exactamente palabras
// palabras (strings.Fields sobre el archivo entero, aclaracion 19).
func pprTexto(t *testing.T, palabras int, criterios string) string {
	t.Helper()
	body := strings.Replace(pprEsqueleto, "%CRITERIOS%", criterios, 1)
	base := len(strings.Fields(strings.Replace(body, "%CONTRATOS%", "x", 1)))
	if palabras < base {
		t.Fatalf("fixture: el esqueleto ya tiene %d palabras, piden %d", base, palabras)
	}
	relleno := "x" + strings.Repeat(" palabra", palabras-base)
	body = strings.Replace(body, "%CONTRATOS%", relleno, 1)
	if n := len(strings.Fields(body)); n != palabras {
		t.Fatalf("fixture: el spec tiene %d palabras, no %d", n, palabras)
	}
	return body
}

// pprSpec deja en un arbol nuevo .hoom/specs/precios.md con palabras
// palabras y k criterios, y un archivo de test que cita cada criterio en
// una linea.
func pprSpec(t *testing.T, palabras, k int) (root, path string) {
	t.Helper()
	return pprSpecCon(t, pprTexto(t, palabras, pprCriterios(k)), k)
}

func pprSpecCon(t *testing.T, body string, k int) (root, path string) {
	t.Helper()
	root = t.TempDir()
	path = filepath.Join(root, ".hoom", "specs", "precios.md")
	pprEscribir(t, path, body)
	var citas strings.Builder
	citas.WriteString("package demo\n")
	for i := 1; i <= k; i++ {
		fmt.Fprintf(&citas, "// cubre CA-%d\n", i)
	}
	pprEscribir(t, filepath.Join(root, "demo_test.go"), citas.String())
	return root, path
}

func pprEscribir(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func pprGate(t *testing.T, ca string, gates []verdict.GateResult, name string) verdict.GateResult {
	t.Helper()
	for _, g := range gates {
		if g.Name == name {
			return g
		}
	}
	t.Fatalf("%s: GatesCon devuelve el gate %q: %+v", ca, name, gates)
	return verdict.GateResult{}
}

// pprLineas son las lineas no vacias de s.
func pprLineas(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, strings.TrimRight(l, "\r"))
		}
	}
	return out
}

func pprIndice(lineas []string, l string) int {
	for i, x := range lineas {
		if x == l {
			return i
		}
	}
	return -1
}

// pprNormal es el calibre de una tarjeta normal con el tope por defecto.
func pprNormal() Calibre {
	return Calibre{Tarjeta: "precios", Riesgo: "normal", Tope: Tope{Palabras: 800, Criterios: 8, CitasPorCriterio: 10}}
}

func pprLineaPalabras(p int, r string, t int, slug string) string {
	return fmt.Sprintf("el spec tiene %d palabras y el tope con riesgo %s es %d: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set %s --tope-palabras %d %s", p, r, t, slug, p, pprPorQue)
}

func pprLineaCriterios(c int, r string, t int, slug string) string {
	return fmt.Sprintf("el spec tiene %d criterios y el tope con riesgo %s es %d: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set %s --tope-criterios %d %s", c, r, t, slug, c, pprPorQue)
}

// CA-791: las palabras son las de strings.Fields sobre el archivo entero:
// encabezados, tablas, bloques de codigo, marcadores verifica y Decisiones
// cuentan.
func TestCA791_PalabrasDeTodoElArchivo(t *testing.T) {
	const ca = "CA-791"
	dir := t.TempDir()
	// 1 "#", 2 "Titulo"; tabla: "|" "a" "|" "b" "|" = 5; codigo: "```go"
	// "x" ":=" "1" "```" = 5; "-" "CA-1:" "uno." "[verifica:" "true]" = 5;
	// "##" "Decisiones" "tres" "palabras" "aqui" = 5. Total 22.
	body := "# Titulo\n\n| a | b |\n\n```go\nx := 1\n```\n\n- CA-1: uno. [verifica: true]\n\n##\tDecisiones\n  tres   palabras\t\taqui\n\n\n"
	p := filepath.Join(dir, "a.md")
	pprEscribir(t, p, body)
	m, err := Medir(p)
	if err != nil {
		t.Fatalf("%s: Medir: %v", ca, err)
	}
	if m.Palabras != 22 || m.Palabras != len(strings.Fields(body)) {
		t.Fatalf("%s: 22 palabras (strings.Fields), fueron %d", ca, m.Palabras)
	}
	// un spec que pasa Lint: todas sus secciones cuentan
	_, path := pprSpec(t, 345, 3)
	m, err = Medir(path)
	if err != nil || m.Palabras != 345 || m.Criterios != 3 {
		t.Fatalf("%s: el spec mide 345 palabras y 3 criterios: %+v %v", ca, m, err)
	}
}

// CA-791: propiedad: para cualquier texto armado con palabras y blancos
// mezclados (espacios, tabs, saltos de linea, \r), Medir da lo mismo que
// strings.Fields.
func TestCA791_PalabrasComoStringsFieldsPropiedad(t *testing.T) {
	const ca = "CA-791"
	dir := t.TempDir()
	trozos := []string{"palabra", "#", "##", "|", "```", "CA-7:", "[verifica:", "x]", "año", "-", "**negrita**", "`codigo`"}
	blancos := []string{" ", "  ", "\t", "\n", "\n\n", "\r\n", " \t "}
	i := 0
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		var b strings.Builder
		for n := r.Intn(200); n > 0; n-- {
			if r.Intn(3) > 0 {
				b.WriteString(trozos[r.Intn(len(trozos))])
			}
			b.WriteString(blancos[r.Intn(len(blancos))])
		}
		i++
		p := filepath.Join(dir, fmt.Sprintf("s%d.md", i))
		pprEscribir(t, p, b.String())
		m, err := Medir(p)
		return err == nil && m.Palabras == len(strings.Fields(b.String()))
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 150}); err != nil {
		t.Fatalf("%s: Medir cuenta como strings.Fields: %v", ca, err)
	}
}

// CA-791: los criterios son los items `- CA-n:` de la seccion de criterios,
// tambien con marcador verifica; un id citado en Decisiones, en Contratos o
// en «Lo que este spec re-expresa» no cuenta.
func TestCA791_CriteriosSoloDeLaSeccion(t *testing.T) {
	const ca = "CA-791"
	body := strings.Replace(pprEsqueleto, "%CRITERIOS%",
		"- CA-1: uno.\n- CA-2: dos. [verifica: true]\n- CA-3: tres,\n  con una linea de continuacion.\n", 1)
	body = strings.Replace(body, "%CONTRATOS%", "Como dice CA-40, y CA-41 tambien.\n- CA-42: un item fuera de la seccion.", 1)
	body = strings.Replace(body, "## Decisiones\nx\n", "## Lo que este spec re-expresa\n\n- **CA-182**: se re-expresa.\n- CA-270: tambien.\n\n## Decisiones\nPor CA-43.\n", 1)
	dir := t.TempDir()
	p := filepath.Join(dir, "s.md")
	pprEscribir(t, p, body)
	m, err := Medir(p)
	if err != nil {
		t.Fatalf("%s: Medir: %v", ca, err)
	}
	if m.Criterios != 3 {
		t.Fatalf("%s: 3 criterios (CA-1, CA-2 con verifica y CA-3), fueron %d", ca, m.Criterios)
	}
	if m.Palabras != len(strings.Fields(body)) {
		t.Fatalf("%s: las palabras incluyen las otras secciones: %d != %d", ca, m.Palabras, len(strings.Fields(body)))
	}
	// lo mismo que Criteria con texto
	cs, err := Criteria(p)
	if err != nil {
		t.Fatal(err)
	}
	con := 0
	for _, c := range cs {
		if c.Text != "" {
			con++
		}
	}
	if con != m.Criterios {
		t.Fatalf("%s: Criterios son los de Criteria con texto (%d): %d", ca, con, m.Criterios)
	}
}

// CA-791: un archivo vacio mide 0 y 0; uno que no se puede leer es el error
// de os.ReadFile.
func TestCA791_VacioYNoLeible(t *testing.T) {
	const ca = "CA-791"
	dir := t.TempDir()
	p := filepath.Join(dir, "vacio.md")
	pprEscribir(t, p, "")
	m, err := Medir(p)
	if err != nil || m.Palabras != 0 || m.Criterios != 0 {
		t.Fatalf("%s: un spec vacio mide 0 y 0: %+v %v", ca, m, err)
	}
	if _, err := Medir(filepath.Join(dir, "no-esta.md")); err == nil || !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s: un spec que no esta es el error de os.ReadFile: %v", ca, err)
	}
	// un spec vacio sigue fallando lint por sus secciones, no por el tope
	root := t.TempDir()
	sp := filepath.Join(root, ".hoom", "specs", "precios.md")
	pprEscribir(t, sp, "")
	gates, _ := GatesCon(root, sp, pprNormal())
	lint := pprGate(t, ca, gates, "spec_lint")
	if lint.Status == verdict.StatusPass || strings.Contains(lint.OutputTail, "el spec tiene") {
		t.Fatalf("%s: el spec vacio falla por sus secciones, sin lineas del tope: %+v", ca, lint)
	}
}

// CA-793: una tarjeta normal con 801 palabras deja spec_lint en fail con la
// linea exacta; 800 palabras y 8 criterios pasan (el tope exacto pasa).
func TestCA793_NormalOchocientasUnaPalabrasFalla(t *testing.T) {
	const ca = "CA-793"
	root, path := pprSpec(t, 801, 3)
	gates, _ := GatesCon(root, path, pprNormal())
	lint := pprGate(t, ca, gates, "spec_lint")
	if lint.Status != verdict.StatusFail {
		t.Fatalf("%s: 801 palabras con tope 800 es fail: %+v", ca, lint)
	}
	ls := pprLineas(lint.OutputTail)
	want := pprLineaPalabras(801, "normal", 800, "precios")
	if pprIndice(ls, want) < 0 {
		t.Fatalf("%s: OutputTail trae la linea exacta\n%s\nfue:\n%s", ca, want, lint.OutputTail)
	}
	for _, l := range ls {
		if strings.Contains(l, "criterios y el tope") {
			t.Fatalf("%s: solo la medida que pasa da linea: %q", ca, l)
		}
	}
	// el tope exacto
	root, path = pprSpec(t, 800, 8)
	gates, _ = GatesCon(root, path, pprNormal())
	lint = pprGate(t, ca, gates, "spec_lint")
	if lint.Status != verdict.StatusPass {
		t.Fatalf("%s: 800 palabras y 8 criterios pasan: %+v", ca, lint)
	}
	if want := "800 palabras de 800 · 8 criterios de 8 · riesgo normal · 8 ids CA-n"; lint.Notes != want {
		t.Fatalf("%s: la nota en pass es\n%q\nfue\n%q", ca, want, lint.Notes)
	}
}

// CA-793: 9 criterios con tope 8 fallan con la linea de criterios; con las
// dos medidas pasadas van las dos lineas, palabras primero.
func TestCA793_NueveCriteriosFallaYLasDosLineas(t *testing.T) {
	const ca = "CA-793"
	root, path := pprSpec(t, 300, 9)
	gates, _ := GatesCon(root, path, pprNormal())
	lint := pprGate(t, ca, gates, "spec_lint")
	if lint.Status != verdict.StatusFail {
		t.Fatalf("%s: 9 criterios con tope 8 es fail: %+v", ca, lint)
	}
	want := pprLineaCriterios(9, "normal", 8, "precios")
	if pprIndice(pprLineas(lint.OutputTail), want) < 0 {
		t.Fatalf("%s: OutputTail trae\n%s\nfue:\n%s", ca, want, lint.OutputTail)
	}
	if strings.Contains(lint.OutputTail, "palabras y el tope") {
		t.Fatalf("%s: 300 palabras no dan linea: %s", ca, lint.OutputTail)
	}
	root, path = pprSpec(t, 900, 10)
	gates, _ = GatesCon(root, path, pprNormal())
	lint = pprGate(t, ca, gates, "spec_lint")
	ls := pprLineas(lint.OutputTail)
	ip := pprIndice(ls, pprLineaPalabras(900, "normal", 800, "precios"))
	ic := pprIndice(ls, pprLineaCriterios(10, "normal", 8, "precios"))
	if lint.Status != verdict.StatusFail || ip < 0 || ic < 0 || ip > ic {
		t.Fatalf("%s: las dos lineas, palabras primero (%d, %d):\n%s", ca, ip, ic, lint.OutputTail)
	}
	got := LineasDelTope(Medida{Palabras: 900, Criterios: 10}, pprNormal())
	if len(got) != 2 || got[0] != pprLineaPalabras(900, "normal", 800, "precios") || got[1] != pprLineaCriterios(10, "normal", 8, "precios") {
		t.Fatalf("%s: LineasDelTope son las dos lineas, palabras primero: %q", ca, got)
	}
}

// CA-793: bajo y alto usan sus topes (400 y 4; 2000 y 15), con su riesgo en
// la linea; un riesgo "" se lee como normal.
func TestCA793_BajoYAltoUsanSusTopes(t *testing.T) {
	const ca = "CA-793"
	bajo := Calibre{Tarjeta: "precios", Riesgo: "bajo", Tope: Tope{Palabras: 400, Criterios: 4, CitasPorCriterio: 10}}
	alto := Calibre{Tarjeta: "precios", Riesgo: "alto", Tope: Tope{Palabras: 2000, Criterios: 15}}
	for _, c := range []struct {
		pal, crit int
		cal       Calibre
		lineas    []string
	}{
		{400, 4, bajo, nil},
		{401, 4, bajo, []string{pprLineaPalabras(401, "bajo", 400, "precios")}},
		{400, 5, bajo, []string{pprLineaCriterios(5, "bajo", 4, "precios")}},
		{2000, 15, alto, nil},
		{2001, 15, alto, []string{pprLineaPalabras(2001, "alto", 2000, "precios")}},
		{1500, 16, alto, []string{pprLineaCriterios(16, "alto", 15, "precios")}},
	} {
		root, path := pprSpec(t, c.pal, c.crit)
		gates, _ := GatesCon(root, path, c.cal)
		lint := pprGate(t, ca, gates, "spec_lint")
		ls := pprLineas(lint.OutputTail)
		if c.lineas == nil {
			if lint.Status != verdict.StatusPass {
				t.Fatalf("%s: %d palabras y %d criterios con riesgo %s pasan: %+v", ca, c.pal, c.crit, c.cal.Riesgo, lint)
			}
			continue
		}
		if lint.Status != verdict.StatusFail {
			t.Fatalf("%s: %d/%d con riesgo %s es fail: %+v", ca, c.pal, c.crit, c.cal.Riesgo, lint)
		}
		for _, w := range c.lineas {
			if pprIndice(ls, w) < 0 {
				t.Fatalf("%s: falta la linea\n%s\nen:\n%s", ca, w, lint.OutputTail)
			}
		}
	}
	vacio := pprNormal()
	vacio.Riesgo = ""
	got := LineasDelTope(Medida{Palabras: 801, Criterios: 1}, vacio)
	if len(got) != 1 || got[0] != pprLineaPalabras(801, "normal", 800, "precios") {
		t.Fatalf("%s: Riesgo \"\" se lee como normal: %q", ca, got)
	}
}

// CA-793: un spec sin tarjeta usa normal y su linea nombra spec.topes en
// hoom.yaml en lugar de hoom item set.
func TestCA793_SinTarjetaNombraSpecTopes(t *testing.T) {
	const ca = "CA-793"
	sin := Calibre{Riesgo: "normal", Tope: Tope{Palabras: 800, Criterios: 8, CitasPorCriterio: 10}}
	root, path := pprSpec(t, 801, 9)
	gates, _ := GatesCon(root, path, sin)
	lint := pprGate(t, ca, gates, "spec_lint")
	ls := pprLineas(lint.OutputTail)
	wp := "el spec tiene 801 palabras y el tope con riesgo normal es 800: reduce el alcance o sube spec.topes.normal.palabras en hoom.yaml"
	wc := "el spec tiene 9 criterios y el tope con riesgo normal es 8: reduce el alcance o sube spec.topes.normal.criterios en hoom.yaml"
	if lint.Status != verdict.StatusFail || pprIndice(ls, wp) < 0 || pprIndice(ls, wc) < 0 {
		t.Fatalf("%s: sin tarjeta las lineas nombran spec.topes:\n%s\n%s\nfue:\n%s", ca, wp, wc, lint.OutputTail)
	}
	if strings.Contains(lint.OutputTail, "hoom item set") {
		t.Fatalf("%s: sin tarjeta no hay hoom item set que sugerir: %s", ca, lint.OutputTail)
	}
	// sin tarjeta con riesgo bajo (un calibre armado a mano): la clave lleva el riesgo
	got := LineasDelTope(Medida{Palabras: 10, Criterios: 5}, Calibre{Riesgo: "bajo", Tope: Tope{Palabras: 400, Criterios: 4}})
	want := "el spec tiene 5 criterios y el tope con riesgo bajo es 4: reduce el alcance o sube spec.topes.bajo.criterios en hoom.yaml"
	if len(got) != 1 || got[0] != want {
		t.Fatalf("%s: LineasDelTope sin tarjeta:\n%q\nfue %q", ca, want, got)
	}
}

// CA-793: el tope de la tarjeta (mayor o menor que el de hoom.yaml) se nombra
// `el tope de la tarjeta es <t>`; el 0 de la tarjeta es sin tope.
func TestCA793_TopeDeLaTarjeta(t *testing.T) {
	const ca = "CA-793"
	menor := pprNormal()
	menor.Tope.Palabras = 500
	menor.PalabrasDeLaTarjeta = true
	root, path := pprSpec(t, 501, 2)
	gates, _ := GatesCon(root, path, menor)
	lint := pprGate(t, ca, gates, "spec_lint")
	want := `el spec tiene 501 palabras y el tope de la tarjeta es 500: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set precios --tope-palabras 501 --motivo "<por que>"`
	if lint.Status != verdict.StatusFail || pprIndice(pprLineas(lint.OutputTail), want) < 0 {
		t.Fatalf("%s: la linea con el tope de la tarjeta:\n%s\nfue:\n%s", ca, want, lint.OutputTail)
	}
	crit := pprNormal()
	crit.Tope.Criterios = 12
	crit.CriteriosDeLaTarjeta = true
	got := LineasDelTope(Medida{Palabras: 100, Criterios: 13}, crit)
	wc := `el spec tiene 13 criterios y el tope de la tarjeta es 12: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set precios --tope-criterios 13 --motivo "<por que>"`
	if len(got) != 1 || got[0] != wc {
		t.Fatalf("%s: la linea de criterios con el tope de la tarjeta:\n%q\nfue %q", ca, wc, got)
	}
	// la tarjeta en su tope: pass con la nota «de la tarjeta»
	root, path = pprSpec(t, 500, 2)
	gates, _ = GatesCon(root, path, menor)
	lint = pprGate(t, ca, gates, "spec_lint")
	if w := "500 palabras de 500 de la tarjeta · 2 criterios de 8 · riesgo normal · 2 ids CA-n"; lint.Status != verdict.StatusPass || lint.Notes != w {
		t.Fatalf("%s: la nota en pass con el tope de la tarjeta:\n%q\nfue %+v", ca, w, lint)
	}
	// 0 de la tarjeta: sin tope
	cero := pprNormal()
	cero.Tope.Palabras, cero.PalabrasDeLaTarjeta = 0, true
	cero.Tope.Criterios, cero.CriteriosDeLaTarjeta = 0, true
	root, path = pprSpec(t, 3000, 20)
	gates, _ = GatesCon(root, path, cero)
	lint = pprGate(t, ca, gates, "spec_lint")
	if w := "3000 palabras sin tope · 20 criterios sin tope · riesgo normal · 20 ids CA-n"; lint.Status != verdict.StatusPass || lint.Notes != w {
		t.Fatalf("%s: con tope 0 no hay tope:\n%q\nfue %+v", ca, w, lint)
	}
}

// CA-793: la tarjeta ilegible da spec_lint en error con el texto exacto y no
// mide (ni con un spec enorme hay lineas del tope).
func TestCA793_TarjetaIlegibleEsError(t *testing.T) {
	const ca = "CA-793"
	cal := Calibre{Tarjeta: "precios", Ilegible: "yaml: line 3: mapping values are not allowed", Riesgo: "alto", Tope: Tope{Palabras: 2000, Criterios: 15}}
	root, path := pprSpec(t, 5000, 30)
	gates, _ := GatesCon(root, path, cal)
	lint := pprGate(t, ca, gates, "spec_lint")
	want := "no se puede leer la tarjeta precios: yaml: line 3: mapping values are not allowed"
	if lint.Status != verdict.StatusError || lint.Notes != want {
		t.Fatalf("%s: spec_lint en error con %q: %+v", ca, want, lint)
	}
	if strings.Contains(lint.OutputTail, "el spec tiene") {
		t.Fatalf("%s: con la tarjeta ilegible la medida no se hace: %s", ca, lint.OutputTail)
	}
	if fmt.Sprintf(FormatoTarjetaIlegible, "precios", "boom") != "no se puede leer la tarjeta precios: boom" {
		t.Fatalf("%s: FormatoTarjetaIlegible: %q", ca, FormatoTarjetaIlegible)
	}
	if got := LineasDelTope(Medida{Palabras: 5000, Criterios: 30}, cal); got != nil {
		t.Fatalf("%s: LineasDelTope es nil con la tarjeta ilegible: %q", ca, got)
	}
	// spec_trace corre igual (aclaracion 4)
	trace := pprGate(t, ca, gates, "spec_trace")
	if trace.Status != verdict.StatusPass {
		t.Fatalf("%s: spec_trace corre igual con la tarjeta ilegible: %+v", ca, trace)
	}
}

// CA-793: con issues de lint de hoy y el tope pasado, las lineas del tope van
// despues de los issues.
func TestCA793_LineasDelTopeDespuesDeLosIssues(t *testing.T) {
	const ca = "CA-793"
	body := pprTexto(t, 900, pprCriterios(2))
	for _, sec := range []string{"## No-goals\nx\n", "## Decisiones\nx\n", "## Riesgos\nx\n"} {
		body = strings.Replace(body, sec, "", 1)
	}
	root, path := pprSpecCon(t, body+strings.Repeat(" relleno", 3), 2)
	_, _, issues, err := Lint(path)
	if err != nil || len(issues) == 0 {
		t.Fatalf("fixture: el spec sin Riesgos tiene issues: %v %v", issues, err)
	}
	m, err := Medir(path)
	if err != nil || m.Palabras <= 800 {
		t.Fatalf("fixture: el spec pasa las 800 palabras: %+v %v", m, err)
	}
	gates, _ := GatesCon(root, path, pprNormal())
	lint := pprGate(t, ca, gates, "spec_lint")
	tail := lint.OutputTail
	ii := strings.Index(tail, issues[0])
	it := strings.Index(tail, pprLineaPalabras(m.Palabras, "normal", 800, "precios"))
	if lint.Status != verdict.StatusFail || ii < 0 || it < 0 || it < ii {
		t.Fatalf("%s: el issue (%d) y luego la linea del tope (%d):\n%s", ca, ii, it, tail)
	}
}

// CA-793: propiedad: una medida da linea si y solo si pasa su tope (mayor que
// t, con t > 0); el tope 0 nunca da linea.
func TestCA793_LineaSiYSoloSiPasaPropiedad(t *testing.T) {
	const ca = "CA-793"
	prop := func(p, tp, c, tc uint16) bool {
		cal := Calibre{Tarjeta: "precios", Riesgo: "normal", Tope: Tope{Palabras: int(tp % 3000), Criterios: int(tc % 40)}}
		m := Medida{Palabras: int(p % 3000), Criterios: int(c % 40)}
		got := LineasDelTope(m, cal)
		want := 0
		if cal.Tope.Palabras > 0 && m.Palabras > cal.Tope.Palabras {
			want++
		}
		if cal.Tope.Criterios > 0 && m.Criterios > cal.Tope.Criterios {
			want++
		}
		return len(got) == want
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("%s: LineasDelTope da una linea por medida que pasa: %v", ca, err)
	}
}

// CA-793: el Calibre cero (Gates de hoy) no tiene tope: un spec enorme pasa
// spec_lint.
func TestCA793_CalibreCeroSinTope(t *testing.T) {
	const ca = "CA-793"
	root, path := pprSpec(t, 4000, 25)
	lint := pprGate(t, ca, Gates(root, path), "spec_lint")
	if lint.Status != verdict.StatusPass {
		t.Fatalf("%s: Gates es GatesCon con el Calibre cero, sin tope: %+v", ca, lint)
	}
	gates, aviso := GatesCon(root, path, Calibre{})
	if g := pprGate(t, ca, gates, "spec_lint"); g.Status != verdict.StatusPass || aviso != "" {
		t.Fatalf("%s: GatesCon con el Calibre cero: %+v %q", ca, g, aviso)
	}
	if len(gates) != 2 || gates[0].Name != "spec_lint" || gates[1].Name != "spec_trace" {
		t.Fatalf("%s: GatesCon da spec_lint y spec_trace, en ese orden: %+v", ca, gates)
	}
}

// CA-793: la nota en pass lleva el conteo de criterios con verifica, y el
// criterio con verifica cuenta para el tope de criterios.
func TestCA793_VerificaCuentaParaElTope(t *testing.T) {
	const ca = "CA-793"
	crit := pprCriterios(8) + "- CA-9: por comando. [verifica: true]\n"
	root, path := pprSpecCon(t, pprTexto(t, 200, crit), 8)
	gates, _ := GatesCon(root, path, pprNormal())
	lint := pprGate(t, ca, gates, "spec_lint")
	if lint.Status != verdict.StatusFail || pprIndice(pprLineas(lint.OutputTail), pprLineaCriterios(9, "normal", 8, "precios")) < 0 {
		t.Fatalf("%s: el criterio con verifica cuenta: 9 criterios con tope 8:\n%s", ca, lint.OutputTail)
	}
	root, path = pprSpecCon(t, pprTexto(t, 200, pprCriterios(7)+"- CA-8: por comando. [verifica: true]\n"), 7)
	gates, _ = GatesCon(root, path, pprNormal())
	lint = pprGate(t, ca, gates, "spec_lint")
	if want := "200 palabras de 800 · 8 criterios de 8 · riesgo normal · 8 ids CA-n, 1 con [verifica:]"; lint.Status != verdict.StatusPass || !strings.HasPrefix(lint.Notes, want) {
		t.Fatalf("%s: la nota en pass con comandos empieza con\n%q\nfue %+v", ca, want, lint)
	}
}

// CA-794: un spec con aprobacion vigente que pasa su tope queda en pass con
// la nota que termina en « · aprobado: el tope no se mide».
func TestCA794_AprobadoQuePasaSuTopeQuedaEnPass(t *testing.T) {
	const ca = "CA-794"
	cal := pprNormal()
	cal.Aprobado = true
	root, path := pprSpec(t, 2500, 12)
	gates, _ := GatesCon(root, path, cal)
	lint := pprGate(t, ca, gates, "spec_lint")
	want := "2500 palabras de 800 · 12 criterios de 8 · riesgo normal · 12 ids CA-n" + NotaAprobado
	if lint.Status != verdict.StatusPass || lint.Notes != want {
		t.Fatalf("%s: aprobado queda en pass con\n%q\nfue %+v", ca, want, lint)
	}
	if NotaAprobado != " · aprobado: el tope no se mide" {
		t.Fatalf("%s: NotaAprobado: %q", ca, NotaAprobado)
	}
	if got := LineasDelTope(Medida{Palabras: 2500, Criterios: 12}, cal); got != nil {
		t.Fatalf("%s: un spec aprobado no da lineas del tope: %q", ca, got)
	}
	// el mismo spec sin aprobacion vigente (cambio despues de aprobarse)
	cal.Aprobado = false
	gates, _ = GatesCon(root, path, cal)
	if lint := pprGate(t, ca, gates, "spec_lint"); lint.Status != verdict.StatusFail {
		t.Fatalf("%s: sin aprobacion vigente se mide y falla: %+v", ca, lint)
	}
}

// CA-797: DescribirMedida y LineaDeStatus con sus variantes exactas.
func TestCA797_LineaDeStatusVariantes(t *testing.T) {
	const ca = "CA-797"
	m := Medida{Palabras: 120, Criterios: 2}
	tarjeta := Calibre{Tarjeta: "precios", Riesgo: "alto", Tope: Tope{Palabras: 2000, Criterios: 15}}
	deLaTarjeta := Calibre{Tarjeta: "precios", Riesgo: "alto", Tope: Tope{Palabras: 500, Criterios: 15}, PalabrasDeLaTarjeta: true}
	sinTope := Calibre{Tarjeta: "precios", Riesgo: "bajo"}
	sinTarjeta := Calibre{Riesgo: "normal", Tope: Tope{Palabras: 800, Criterios: 8}}
	aprobado := tarjeta
	aprobado.Aprobado = true
	sinTarjetaAprobado := sinTarjeta
	sinTarjetaAprobado.Aprobado = true
	ilegible := Calibre{Tarjeta: "precios", Ilegible: "boom", Riesgo: "alto", Tope: Tope{Palabras: 2000, Criterios: 15}}
	for _, c := range []struct {
		cal       Calibre
		describir string
		linea     string
	}{
		{tarjeta, "120 palabras de 2000 · 2 criterios de 15", "hoom spec: 120 palabras de 2000 · 2 criterios de 15 (riesgo alto)"},
		{deLaTarjeta, "120 palabras de 500 de la tarjeta · 2 criterios de 15", "hoom spec: 120 palabras de 500 de la tarjeta · 2 criterios de 15 (riesgo alto)"},
		{sinTope, "120 palabras sin tope · 2 criterios sin tope", "hoom spec: 120 palabras sin tope · 2 criterios sin tope (riesgo bajo)"},
		{sinTarjeta, "120 palabras de 800 · 2 criterios de 8", "hoom spec: 120 palabras de 800 · 2 criterios de 8 (riesgo normal, sin tarjeta)"},
		{aprobado, "120 palabras de 2000 · 2 criterios de 15", "hoom spec: 120 palabras de 2000 · 2 criterios de 15 (riesgo alto) · aprobado: el tope no se mide"},
		{sinTarjetaAprobado, "120 palabras de 800 · 2 criterios de 8", "hoom spec: 120 palabras de 800 · 2 criterios de 8 (riesgo normal, sin tarjeta) · aprobado: el tope no se mide"},
	} {
		if got := DescribirMedida(m, c.cal); got != c.describir {
			t.Fatalf("%s: DescribirMedida(%+v)\n%q\nfue %q", ca, c.cal, c.describir, got)
		}
		if got := LineaDeStatus(m, c.cal); got != c.linea {
			t.Fatalf("%s: LineaDeStatus(%+v)\n%q\nfue %q", ca, c.cal, c.linea, got)
		}
	}
	if got, want := LineaDeStatus(m, ilegible), "hoom spec: no se puede medir el tope: no se puede leer la tarjeta precios: boom"; got != want {
		t.Fatalf("%s: LineaDeStatus con la tarjeta ilegible\n%q\nfue %q", ca, want, got)
	}
	// un spec que pasa su tope tambien se describe (la linea no es el rojo)
	if got, want := LineaDeStatus(Medida{Palabras: 900, Criterios: 9}, Calibre{Tarjeta: "precios", Riesgo: "normal", Tope: Tope{Palabras: 800, Criterios: 8}}),
		"hoom spec: 900 palabras de 800 · 9 criterios de 8 (riesgo normal)"; got != want {
		t.Fatalf("%s: la linea de un spec que pasa su tope\n%q\nfue %q", ca, want, got)
	}
}

// CA-801: las citas son lineas: una linea que cita dos veces un criterio
// cuenta una; una que cita dos criterios cuenta una para cada uno; un token
// dentro de otro con mas digitos no cuenta; los criterios con verifica no
// tienen entrada; uno sin test tiene 0.
func TestCA801_CitasPorLinea(t *testing.T) {
	const ca = "CA-801"
	root := t.TempDir()
	pprEscribir(t, filepath.Join(root, "a_test.go"), "package a\n"+
		"// CA-1 y otra vez CA-1 en la misma linea\n"+
		"// CA-1 junto a CA-2\n"+
		"func TestCA1_x() {} // CA-12 y CA-10 no citan al uno\n"+
		"// CA-2\n")
	pprEscribir(t, filepath.Join(root, "sub", "b_test.go"), "package b\n// CA-1\n// CA-1\n")
	// fuera de los archivos que spec_trace escanea: no cuentan
	pprEscribir(t, filepath.Join(root, "README.md"), "CA-1\nCA-1\nCA-2\n")
	pprEscribir(t, filepath.Join(root, ".hoom", "specs", "x_test.go"), "// CA-1\n// CA-2\n")
	ids := []string{"CA-1", "CA-2", "CA-3", "CA-4"}
	res, err := Trace(root, ids, map[string][]string{"CA-3": {"true"}})
	if err != nil {
		t.Fatalf("%s: Trace: %v", ca, err)
	}
	want := map[string]int{"CA-1": 4, "CA-2": 2, "CA-4": 0}
	if len(res.Citas) != len(want) {
		t.Fatalf("%s: Citas tiene una entrada por id buscado, sin CA-3 (verifica): %v", ca, res.Citas)
	}
	for id, n := range want {
		if got, ok := res.Citas[id]; !ok || got != n {
			t.Fatalf("%s: Citas[%s] es %d: %v", ca, id, n, res.Citas)
		}
	}
	if _, ok := res.Citas["CA-3"]; ok {
		t.Fatalf("%s: un criterio con verifica no tiene citas: %v", ca, res.Citas)
	}
	if len(res.MissingTests) != 1 || res.MissingTests[0] != "CA-4" {
		t.Fatalf("%s: CA-4 sigue sin test: %v", ca, res.MissingTests)
	}
	// solo verifica: Citas es nil
	res, err = Trace(root, []string{"CA-3"}, map[string][]string{"CA-3": {"true"}})
	if err != nil || res.Citas != nil {
		t.Fatalf("%s: sin ids buscados en los tests Citas es nil: %v %v", ca, res.Citas, err)
	}
}

// CA-801: propiedad: n lineas que citan un criterio (repartidas en archivos,
// con o sin repeticion en la linea) son n citas.
func TestCA801_CitasPropiedad(t *testing.T) {
	const ca = "CA-801"
	prop := func(seed int64) bool {
		r := rand.New(rand.NewSource(seed))
		root := t.TempDir()
		total := 0
		for f := r.Intn(4) + 1; f > 0; f-- {
			var b strings.Builder
			b.WriteString("package p\n")
			for l := r.Intn(15); l > 0; l-- {
				switch r.Intn(4) {
				case 0:
					b.WriteString("// CA-5\n")
					total++
				case 1:
					b.WriteString("// CA-5, CA-5 y CA-5\n")
					total++
				case 2:
					b.WriteString("// CA-50 CA-55 CA-500\n")
				default:
					b.WriteString("// nada\n")
				}
			}
			pprEscribir(t, filepath.Join(root, fmt.Sprintf("f%d_test.go", f)), b.String())
		}
		res, err := Trace(root, []string{"CA-5"}, nil)
		return err == nil && res.Citas["CA-5"] == total
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 60}); err != nil {
		t.Fatalf("%s: las citas son las lineas: %v", ca, err)
	}
}

// CA-801: la nota de spec_trace en pass trae « · citas por criterio: » en el
// orden de los ids, solo con los criterios trazados por test.
func TestCA801_NotaDeSpecTraceConLasCitas(t *testing.T) {
	const ca = "CA-801"
	crit := "- CA-1: uno.\n- CA-2: dos. [verifica: true]\n- CA-3: tres.\n"
	root := t.TempDir()
	path := filepath.Join(root, ".hoom", "specs", "precios.md")
	pprEscribir(t, path, pprTexto(t, 150, crit))
	pprEscribir(t, filepath.Join(root, "p_test.go"), "package p\n// CA-1\n// CA-1\n// CA-1 CA-3\n")
	gates, aviso := GatesCon(root, path, pprNormal())
	tr := pprGate(t, ca, gates, "spec_trace")
	if tr.Status != verdict.StatusPass {
		t.Fatalf("%s: spec_trace pasa: %+v", ca, tr)
	}
	if !strings.Contains(tr.Notes, " · citas por criterio: CA-1 3, CA-3 1") {
		t.Fatalf("%s: la nota trae las citas en el orden de los ids, sin CA-2: %q", ca, tr.Notes)
	}
	if strings.Contains(tr.Notes, "CA-2 ") || aviso != "" || strings.Contains(tr.Notes, "pasan el tope") {
		t.Fatalf("%s: sin CA-2 (verifica) y sin aviso: %q %q", ca, tr.Notes, aviso)
	}
	if fmt.Sprintf(FormatoNotaCitas, "CA-1 3") != " · citas por criterio: CA-1 3" {
		t.Fatalf("%s: FormatoNotaCitas: %q", ca, FormatoNotaCitas)
	}
}

// CA-801: un criterio sin test sigue dejando rojo spec_trace con riesgo
// bajo, normal y alto.
func TestCA801_SinTestRojoEnTodoRiesgo(t *testing.T) {
	const ca = "CA-801"
	root := t.TempDir()
	path := filepath.Join(root, ".hoom", "specs", "precios.md")
	pprEscribir(t, path, pprTexto(t, 120, pprCriterios(2)))
	pprEscribir(t, filepath.Join(root, "p_test.go"), "package p\n// CA-1\n")
	for _, cal := range []Calibre{
		{Tarjeta: "precios", Riesgo: "bajo", Tope: Tope{Palabras: 400, Criterios: 4, CitasPorCriterio: 10}},
		pprNormal(),
		{Tarjeta: "precios", Riesgo: "alto", Tope: Tope{Palabras: 2000, Criterios: 15}},
	} {
		gates, _ := GatesCon(root, path, cal)
		if tr := pprGate(t, ca, gates, "spec_trace"); tr.Status != verdict.StatusFail || !strings.Contains(tr.OutputTail+tr.Notes, "CA-2") {
			t.Fatalf("%s: CA-2 sin test deja rojo spec_trace con riesgo %s: %+v", ca, cal.Riesgo, tr)
		}
	}
}

// CA-802: con tope 10 de citas y un criterio con 11, GatesCon da el aviso
// exacto y la nota de spec_trace termina con el tope pasado; los estados no
// cambian; 10 citas no avisan.
func TestCA802_AvisoDeCitas(t *testing.T) {
	const ca = "CA-802"
	root := t.TempDir()
	path := filepath.Join(root, ".hoom", "specs", "precios.md")
	pprEscribir(t, path, pprTexto(t, 150, pprCriterios(3)))
	pprEscribir(t, filepath.Join(root, "p_test.go"), "package p\n"+
		strings.Repeat("// CA-1\n", 11)+strings.Repeat("// CA-2\n", 10)+"// CA-3\n")
	gates, aviso := GatesCon(root, path, pprNormal())
	want := "! 1 criterios pasan el tope de 10 citas en pruebas (riesgo normal): CA-1 11"
	if aviso != want {
		t.Fatalf("%s: el aviso es\n%q\nfue\n%q", ca, want, aviso)
	}
	tr := pprGate(t, ca, gates, "spec_trace")
	if tr.Status != verdict.StatusPass || !strings.HasSuffix(tr.Notes, " · 1 pasan el tope de 10 citas") {
		t.Fatalf("%s: spec_trace en pass con la nota que termina en el tope pasado: %+v", ca, tr)
	}
	if !strings.Contains(tr.Notes, " · citas por criterio: CA-1 11, CA-2 10, CA-3 1") {
		t.Fatalf("%s: la nota trae las citas: %q", ca, tr.Notes)
	}
	// el mismo arbol sin tope de citas: mismos estados, sin aviso
	sin := pprNormal()
	sin.Tope.CitasPorCriterio = 0
	gates0, aviso0 := GatesCon(root, path, sin)
	if aviso0 != "" || len(gates0) != len(gates) {
		t.Fatalf("%s: sin tope de citas no hay aviso: %q", ca, aviso0)
	}
	for i := range gates {
		if gates[i].Name != gates0[i].Name || gates[i].Status != gates0[i].Status {
			t.Fatalf("%s: el aviso no cambia ningun estado: %+v vs %+v", ca, gates[i], gates0[i])
		}
	}
	if strings.Contains(pprGate(t, ca, gates0, "spec_trace").Notes, "pasan el tope") {
		t.Fatalf("%s: sin tope de citas la nota no habla del tope", ca)
	}
	if fmt.Sprintf(FormatoAvisoCitas, 2, 10, "bajo", "CA-1 11, CA-2 12") != "! 2 criterios pasan el tope de 10 citas en pruebas (riesgo bajo): CA-1 11, CA-2 12" {
		t.Fatalf("%s: FormatoAvisoCitas: %q", ca, FormatoAvisoCitas)
	}
	if fmt.Sprintf(FormatoNotaPasanCitas, 2, 10) != " · 2 pasan el tope de 10 citas" {
		t.Fatalf("%s: FormatoNotaPasanCitas: %q", ca, FormatoNotaPasanCitas)
	}
}

// CA-802: dos criterios pasados van en el orden de los ids; con alto y los
// valores por defecto (citas_por_criterio 0) no hay aviso.
func TestCA802_DosPasadosYAltoSinAviso(t *testing.T) {
	const ca = "CA-802"
	root := t.TempDir()
	path := filepath.Join(root, ".hoom", "specs", "precios.md")
	pprEscribir(t, path, pprTexto(t, 150, pprCriterios(3)))
	pprEscribir(t, filepath.Join(root, "p_test.go"), "package p\n"+
		strings.Repeat("// CA-1\n", 12)+"// CA-2\n"+strings.Repeat("// CA-3\n", 25))
	_, aviso := GatesCon(root, path, pprNormal())
	if want := "! 2 criterios pasan el tope de 10 citas en pruebas (riesgo normal): CA-1 12, CA-3 25"; aviso != want {
		t.Fatalf("%s: dos criterios pasados, en el orden de los ids:\n%q\nfue\n%q", ca, want, aviso)
	}
	alto := Calibre{Tarjeta: "precios", Riesgo: "alto", Tope: Tope{Palabras: 2000, Criterios: 15, CitasPorCriterio: 0}}
	gates, aviso := GatesCon(root, path, alto)
	if aviso != "" || strings.Contains(pprGate(t, ca, gates, "spec_trace").Notes, "pasan el tope") {
		t.Fatalf("%s: con alto y los valores por defecto no hay aviso: %q", ca, aviso)
	}
	// la tarjeta ilegible: spec_trace corre, sin aviso (aclaracion 4)
	il := pprNormal()
	il.Ilegible = "boom"
	gates, aviso = GatesCon(root, path, il)
	if aviso != "" || pprGate(t, ca, gates, "spec_trace").Status != verdict.StatusPass {
		t.Fatalf("%s: con la tarjeta ilegible spec_trace corre sin aviso: %q", ca, aviso)
	}
}

// CA-802: con spec_trace en fail por un criterio sin test, el aviso se
// calcula igual sobre los trazados por test (aclaracion 3).
func TestCA802_AvisoConSpecTraceEnFail(t *testing.T) {
	const ca = "CA-802"
	root := t.TempDir()
	path := filepath.Join(root, ".hoom", "specs", "precios.md")
	pprEscribir(t, path, pprTexto(t, 150, pprCriterios(2)))
	pprEscribir(t, filepath.Join(root, "p_test.go"), "package p\n"+strings.Repeat("// CA-1\n", 11))
	gates, aviso := GatesCon(root, path, pprNormal())
	if tr := pprGate(t, ca, gates, "spec_trace"); tr.Status != verdict.StatusFail {
		t.Fatalf("%s: CA-2 sin test deja spec_trace en fail: %+v", ca, tr)
	}
	if want := "! 1 criterios pasan el tope de 10 citas en pruebas (riesgo normal): CA-1 11"; aviso != want {
		t.Fatalf("%s: el aviso sale sobre los criterios trazados por test:\n%q\nfue %q", ca, want, aviso)
	}
}
