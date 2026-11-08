// La tarjeta abierta (CA-521, CA-522, CA-523, CA-524, CA-531): pestanas,
// Resumen, Spec con sus margenes y ]c [c o, Sobre, Hallazgos con enter y la
// Historia.
package tablero_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttAbrir arma un tablero de una sola tarjeta, la abre y le da su detalle.
func ttAbrir(t *testing.T, cfg tablero.Config, c boardcmd.Card, d tablero.Detalle) *tablero.Modelo {
	t.Helper()
	m := ttModelo(cfg, ttTablero(c))
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if s, _ := m.Abierta(); s != c.Slug {
		t.Fatalf("CA-520: enter abre %s, Abierta = %q\n%s", c.Slug, s, ttPantalla(m))
	}
	d.Slug = c.Slug
	if d.Detalle.Card.Slug == "" {
		d.Detalle.Card = c
	}
	m.LeerDetalle(d)
	return m
}

func ttContenido(m *tablero.Modelo) []string {
	c := ttCuerpo(m)
	if len(c) < 1 {
		return nil
	}
	return c[1:]
}

// CA-521: las cinco pestanas se eligen con 1 a 5 y con tab (en ciclo), la
// activa va entre corchetes, y Abierta dice si la Historia es la activa.
func TestCA521_CincoPestanas(t *testing.T) {
	m := ttAbrir(t, ttConfig(80, 24, false), ttInfraAPI(false), tablero.Detalle{})
	nombres := []string{"1 Resumen", "2 Spec", "3 Sobre", "4 Hallazgos", "5 Historia"}
	linea := func(i int) string {
		var p []string
		for k, n := range nombres {
			if k == i {
				n = "[" + n + "]"
			}
			p = append(p, n)
		}
		return " " + strings.Join(p, "  ")
	}
	revisar := func(i int, como string) {
		t.Helper()
		if v := m.Vista(); v != tablero.Pestanas[i] {
			t.Fatalf("CA-521: %s elige %q, la vista es %q", como, tablero.Pestanas[i], v)
		}
		if got := ttLinea(m, 2); got != linea(i) {
			t.Fatalf("CA-521: con %s la linea de pestanas es\n %q\nse esperaba\n %q", como, got, linea(i))
		}
		_, h := m.Abierta()
		if h != (tablero.Pestanas[i] == tablero.VistaHistoria) {
			t.Fatalf("CA-521: Abierta dice historia=%v en %q", h, m.Vista())
		}
	}
	revisar(0, "abrir")
	for _, i := range []int{4, 2, 1, 3, 0} {
		ttSinOrdenes(t, "CA-521", fmt.Sprint(i+1), m.Tecla(tablero.Tecla(fmt.Sprint(i+1)), ttAhora))
		revisar(i, fmt.Sprintf("la tecla %d", i+1))
	}
	for i := 1; i <= 5; i++ {
		ttSinOrdenes(t, "CA-521", "tab", m.Tecla(tablero.TeclaTab, ttAhora))
		revisar(i%5, fmt.Sprintf("tab numero %d", i))
	}
	if len(tablero.Pestanas) != 5 {
		t.Fatalf("CA-521: son cinco pestanas: %v", tablero.Pestanas)
	}
}

// CA-521: Resumen sin lo que falta ("falta: nada"), sin quien escribio
// ("sin dato"), sin rojo (no hay linea rojo:), sin presupuesto, y el doctor
// con what cuando el plain esta vacio.
func TestCA521_ResumenVariantes(t *testing.T) {
	c := ttCard("lista", "Lista", boardcmd.ColTuAceptacion, "lista para integrar",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"), ttSeg("verde", "verde", boardcmd.SegHecho, "verde"))
	c.Spend = boardcmd.Spend{CostUSD: ttF(1.5)}
	c.Providers = boardcmd.Providers{Writer: "", Reviewer: "codex"}
	c.Doctor = []boardcmd.Problem{{ID: boardcmd.ProbSinVeredicto, What: "sin veredicto en la base", Plain: "", Action: "hoom verify"}}
	m := ttAbrir(t, ttConfig(100, 30, false), c, tablero.Detalle{})
	cont := strings.Join(ttContenido(m), "\n")
	for _, quiere := range []string{
		" columna: Tu aceptacion\n lista para integrar\n falta: nada\n tira: ##\n   # spec: aprobado\n   # verde: verde\n",
		" gasto: 1.50 USD\n",
		" escribio: sin dato · reviso: codex\n",
		" doctor: sin veredicto en la base: hoom verify",
	} {
		if !strings.Contains(cont+"\n", quiere) {
			t.Fatalf("CA-521: el Resumen tiene\n%s\nY su contenido es:\n%s", quiere, cont)
		}
	}
	if strings.Contains(cont, " rojo:") {
		t.Fatalf("CA-521: sin rojo no hay linea rojo:\n%s", cont)
	}
	// sin costo con presupuesto
	c.Spend = boardcmd.Spend{BudgetUSD: ttF(12.25)}
	m = ttAbrir(t, ttConfig(100, 30, false), c, tablero.Detalle{})
	if l, _ := ttLineaCon(m, " gasto: "); l != " gasto: sin dato de costo de 12.25 USD" {
		t.Fatalf("CA-521: sin costo el gasto dice 'sin dato de costo' contra el presupuesto: %q", l)
	}
}

// CA-531: el modo experto muestra en Resumen "siguiente:" y una linea
// "missing:" por cada una, que el modo normal no muestra.
func TestCA531_ExpertoMuestraNextYMissing(t *testing.T) {
	c := ttInfraAPI(true)
	c.Missing = []string{"veredicto verde de la tarjeta", "review de 4 lentes"}
	m := ttAbrir(t, ttConfig(100, 40, false), c, tablero.Detalle{})
	cont := strings.Join(ttContenido(m), "\n")
	if strings.Contains(cont, "siguiente:") || strings.Contains(cont, "missing:") {
		t.Fatalf("CA-531: en modo normal Resumen no muestra next ni missing:\n%s", cont)
	}
	ttSinOrdenes(t, "CA-531", "x", m.Tecla("x", ttAhora))
	if b := ttBarra1(m); !strings.HasPrefix(b, " EXPERTO · ") {
		t.Fatalf("CA-531: x pone el modo experto en la barra: %q", b)
	}
	cont = strings.Join(ttContenido(m), "\n")
	quiere := " columna: Writer\n el writer esta trabajando\n siguiente: hoom agent --role writer --task infra-api\n" +
		" missing: veredicto verde de la tarjeta\n missing: review de 4 lentes\n falta: verde: sin veredicto verde\n"
	if !strings.Contains(cont, quiere) {
		t.Fatalf("CA-531: en modo experto Resumen muestra next y missing despues del plain:\n%s\nSe esperaba:\n%s", cont, quiere)
	}
	m.Tecla("x", ttAhora)
	if cont = strings.Join(ttContenido(m), "\n"); strings.Contains(cont, "siguiente:") {
		t.Fatalf("CA-531: x otra vez vuelve al modo normal:\n%s", cont)
	}
}

// ttSpecLargo: un spec de 3000 lineas con seis criterios; CA-2 (linea 500) y
// CA-5 (linea 2500) sin prueba, CA-4 (linea 1500) por comando.
func ttSpecLargo() (string, []boardcmd.CriterionTrace) {
	en := map[int]string{100: "CA-1", 500: "CA-2", 1000: "CA-3", 1500: "CA-4", 2500: "CA-5", 2800: "CA-6"}
	var ls []string
	for i := 1; i <= 3000; i++ {
		if ca, ok := en[i]; ok {
			ls = append(ls, "- "+ca+": el criterio "+ca+".")
			continue
		}
		ls = append(ls, fmt.Sprintf("linea %d", i))
	}
	var cr []boardcmd.CriterionTrace
	for _, ca := range []string{"CA-1", "CA-2", "CA-3", "CA-4", "CA-5", "CA-6"} {
		tr := boardcmd.TracedTest
		switch ca {
		case "CA-2", "CA-5":
			tr = ""
		case "CA-4":
			tr = boardcmd.TracedComando
		}
		cr = append(cr, boardcmd.CriterionTrace{ID: ca, TracedBy: tr})
	}
	return strings.Join(ls, "\n") + "\n", cr
}

// CA-522: ]c y [c hacen del siguiente y del anterior criterio sin prueba la
// primera linea a la vista, buscando en todo el archivo (miles de lineas); sin
// otro, la barra dice "no hay mas criterios sin prueba"; o abre el spec en el
// editor en la primera linea a la vista.
func TestCA522_SaltosEntreCriteriosSinPruebaYEditor(t *testing.T) {
	md, cr := ttSpecLargo()
	c := ttInfraAPI(false)
	c.Evidence.Untraced = []string{"CA-2", "CA-5"}
	c.Evidence.Criteria = 6
	d := tablero.Detalle{Detalle: boardcmd.Detail{Spec: boardcmd.SpecDetail{Path: ".hoom/specs/infra-api.md", Exists: true, Markdown: md}, Criteria: cr}}
	m := ttAbrir(t, ttConfig(80, 24, false), c, d)
	m.Tecla("2", ttAhora)
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: d.Detalle.Spec, Criteria: cr}})
	primera := func() string { return ttLinea(m, 3) }
	editor := func(linea int) {
		t.Helper()
		os := m.Tecla("o", ttAhora)
		if len(os) != 1 {
			t.Fatalf("CA-522: o da una orden AbrirEditor, dio %#v", os)
		}
		ae, ok := os[0].(tablero.AbrirEditor)
		if !ok || ae.Archivo != ".hoom/specs/infra-api.md" || ae.Linea != linea {
			t.Fatalf("CA-522: o abre el spec en la linea %d: %#v", linea, os[0])
		}
	}
	if got := primera(); got != "  linea 1" {
		t.Fatalf("CA-522: la pestana Spec empieza arriba con su margen de dos celdas: %q", got)
	}
	editor(1)
	salto := func(teclas string, quiere string) {
		t.Helper()
		ttSinOrdenes(t, "CA-522", teclas, ttEscribir(m, teclas))
		if got := primera(); got != quiere {
			t.Fatalf("CA-522: tras %s la primera linea a la vista es %q, se esperaba %q", teclas, got, quiere)
		}
	}
	salto("]c", "x - CA-2: el criterio CA-2.")
	editor(500)
	salto("]c", "x - CA-5: el criterio CA-5.")
	editor(2500)
	ttEscribir(m, "]c")
	if got := primera(); got != "x - CA-5: el criterio CA-5." {
		t.Fatalf("CA-522: sin otro criterio sin prueba la vista no se mueve: %q", got)
	}
	if b := ttBarra1(m); !strings.HasSuffix(b, " · no hay mas criterios sin prueba") {
		t.Fatalf("CA-522: sin otro, la barra dice 'no hay mas criterios sin prueba': %q", b)
	}
	salto("[c", "x - CA-2: el criterio CA-2.")
	ttEscribir(m, "[c")
	if b := ttBarra1(m); !strings.HasSuffix(b, " · no hay mas criterios sin prueba") || primera() != "x - CA-2: el criterio CA-2." {
		t.Fatalf("CA-522: [c sin uno anterior deja la vista y avisa: %q / %q", b, primera())
	}
	// j desplaza una linea
	m.Tecla("j", ttAhora)
	if got := primera(); got != "  linea 501" {
		t.Fatalf("CA-522: j desplaza el spec una linea: %q", got)
	}
	editor(501)
	// el criterio por comando lleva v en ASCII
	for i := 0; i < 999; i++ {
		m.Tecla("j", ttAhora)
	}
	if got := primera(); got != "v - CA-4: el criterio CA-4." {
		t.Fatalf("CA-522: la linea 1500 es el criterio por comando con su v: %q", got)
	}
}

// CA-522: "- CA-10:" no es la linea de CA-1; un spec que no existe dice
// " sin spec: <path>".
func TestCA522_MargenesHostiles(t *testing.T) {
	c := ttInfraAPI(false)
	c.Evidence.Untraced = []string{"CA-1"}
	md := "- CA-10: no es CA-1.\n- CA-1: este si.\n   - CA-1: tambien, con sangria.\n-CA-1: sin espacio no.\n"
	d := tablero.Detalle{Detalle: boardcmd.Detail{Spec: boardcmd.SpecDetail{Path: ".hoom/specs/infra-api.md", Exists: true, Markdown: md},
		Criteria: []boardcmd.CriterionTrace{{ID: "CA-1"}}}}
	m := ttAbrir(t, ttConfig(80, 24, false), c, d)
	m.Tecla("2", ttAhora)
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: d.Detalle.Spec, Criteria: d.Detalle.Criteria}})
	quiere := []string{"  - CA-10: no es CA-1.", "x - CA-1: este si.", "x    - CA-1: tambien, con sangria.", "  -CA-1: sin espacio no."}
	for i, q := range quiere {
		if got := ttLinea(m, 3+i); got != q {
			t.Fatalf("CA-522: linea %d del spec es %q, se esperaba %q\n%s", i+1, got, q, ttPantalla(m))
		}
	}
	d.Detalle.Spec = boardcmd.SpecDetail{Path: ".hoom/specs/infra-api.md", Exists: false}
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: d.Detalle.Spec}})
	if got := ttLinea(m, 3); got != " sin spec: .hoom/specs/infra-api.md" {
		t.Fatalf("CA-522: sin spec la pestana dice ' sin spec: <path>': %q", got)
	}
}

func ttEvento(seg int, kind, detail string) runcmd.Event {
	return runcmd.Event{TS: ttAhora.Add(time.Duration(seg) * time.Second), Kind: kind, Detail: detail}
}

// CA-523: la pestana Sobre: arriba rol, paso, tiempo y "ultimo evento hace
// <n>s"; despues un evento por linea (hora UTC, kind, detalle con sus saltos
// como espacios); se actualiza con cada lectura del detalle.
func TestCA523_SobreEnCurso(t *testing.T) {
	c := ttInfraAPI(true)
	sobre := tablero.Sobre{EnvelopeID: "env-api", RunID: "run-api", Eventos: []runcmd.Event{
		ttEvento(-30, "start", "arranca"), ttEvento(-10, "text", "linea uno\nlinea dos"), ttEvento(-5, "tool", "go test ./..."),
	}}
	m := ttAbrir(t, ttConfig(100, 24, false), c, tablero.Detalle{Sobre: sobre})
	m.Tecla("3", ttAhora)
	if m.Vista() != tablero.VistaSobre {
		t.Fatalf("CA-521: 3 es Sobre, la vista es %q", m.Vista())
	}
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c}, Sobre: sobre})
	quiere := []string{
		" writer · run 3/6 · 12:41 de 45:00 · ultimo evento hace 5s",
		" 11:59:30 start arranca",
		" 11:59:50 text linea uno linea dos",
		" 11:59:55 tool go test ./...",
	}
	revisar := func(q []string) {
		t.Helper()
		_, n := ttLineaCon(m, q[0])
		if n == 0 {
			t.Fatalf("CA-523: la primera linea de Sobre es %q:\n%s", q[0], ttPantalla(m))
		}
		for i, l := range q {
			if got := ttLinea(m, n+i); got != l {
				t.Fatalf("CA-523: linea %d de Sobre es %q, se esperaba %q\n%s", i, got, l, ttPantalla(m))
			}
		}
	}
	revisar(quiere)
	// otra lectura trae un evento nuevo
	sobre.Eventos = append(sobre.Eventos, ttEvento(-1, "text", "listo"))
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c}, Sobre: sobre})
	revisar([]string{" writer · run 3/6 · 12:41 de 45:00 · ultimo evento hace 1s", quiere[1], quiere[2], quiere[3], " 11:59:59 text listo"})
}

// CA-523: sin running, la primera linea es la del trabajo mas nuevo
// (Work[0]) con su duracion y su status; sin eventos no hay "ultimo evento";
// sin trabajos, " sin sobres todavia"; en UTF-8 dice "último".
func TestCA523_SobreSinRunning(t *testing.T) {
	c := ttInfraAPI(false)
	dur := int64(12*60*1000 + 34*1000)
	work := []boardcmd.WorkRow{{Kind: boardcmd.WorkSobre, ID: "env-2", RunID: "run-2", Role: "reviewer", Status: "entregable",
		Stage: "check", Step: 6, Steps: 6, DurationMS: &dur}, {Kind: boardcmd.WorkSobre, ID: "env-1", Role: "writer", Status: "rojo"}}
	sobre := tablero.Sobre{EnvelopeID: "env-2", RunID: "run-2", Eventos: []runcmd.Event{ttEvento(-5, "end", "fin")}}
	d := tablero.Detalle{Detalle: boardcmd.Detail{Work: work}, Sobre: sobre}
	m := ttAbrir(t, ttConfig(100, 24, true), c, d)
	m.Tecla("3", ttAhora)
	d.Slug, d.Detalle.Card = c.Slug, c
	m.LeerDetalle(d)
	if _, n := ttLineaCon(m, " reviewer · check 6/6 · 12:34 · entregable · último evento hace 5s"); n == 0 {
		t.Fatalf("CA-523: sin running la primera linea es la del trabajo mas nuevo:\n%s", ttPantalla(m))
	}
	d.Sobre.Eventos = nil
	m.LeerDetalle(d)
	if l, n := ttLineaCon(m, " reviewer · check 6/6 · 12:34 · entregable"); n == 0 || strings.Contains(l, "evento") {
		t.Fatalf("CA-523: sin eventos no hay 'último evento': %q\n%s", l, ttPantalla(m))
	}
	d.Detalle.Work = nil
	d.Sobre = tablero.Sobre{}
	m.LeerDetalle(d)
	if got := ttLinea(m, 3); got != " sin sobres todavia" {
		t.Fatalf("CA-523: sin trabajos la pestana dice ' sin sobres todavia': %q\n%s", got, ttPantalla(m))
	}
}

// CA-523: el contenido de Sobre empieza desplazado a su final: con muchos
// eventos, el ultimo es la ultima linea del cuerpo; k sube.
func TestCA523_SobreEmpiezaAlFinal(t *testing.T) {
	c := ttInfraAPI(true)
	var evs []runcmd.Event
	for i := 0; i < 60; i++ {
		evs = append(evs, ttEvento(-600+i, "text", fmt.Sprintf("evento %02d", i)))
	}
	m := ttAbrir(t, ttConfig(80, 24, false), c, tablero.Detalle{})
	m.Tecla("3", ttAhora)
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c}, Sobre: tablero.Sobre{EnvelopeID: "env-api", Eventos: evs}})
	cuerpo := ttCuerpo(m)
	if got := cuerpo[len(cuerpo)-1]; !strings.HasSuffix(got, " text evento 59") {
		t.Fatalf("CA-523: Sobre empieza al final: la ultima linea del cuerpo es el ultimo evento, es %q\n%s", got, ttPantalla(m))
	}
	if _, n := ttLineaCon(m, "evento 00"); n != 0 {
		t.Fatalf("CA-523: al final, el primer evento no se ve:\n%s", ttPantalla(m))
	}
	m.Tecla("k", ttAhora)
	if got := ttCuerpo(m)[len(cuerpo)-1]; !strings.HasSuffix(got, " text evento 58") {
		t.Fatalf("CA-523: k sube una linea: %q", got)
	}
}

func ttHallazgo(id, status, sev, file, desc string) finding.Item {
	return finding.Item{Finding: finding.Finding{ID: id, Severity: sev, File: file, Description: desc, Task: "infra-api",
		CreatedAt: ttAhora.Add(-time.Hour)}, Status: status}
}

// CA-524: Hallazgos lista los de la tarea con los abiertos primero (cada
// grupo en el orden del detalle); enter abre el editor en el archivo
// (relativo a la raiz: dir + "/" + file) y la linea del hallazgo; sin
// archivo, la barra lo dice.
func TestCA524_HallazgosAbiertosPrimeroYEnter(t *testing.T) {
	c := ttInfraAPI(false)
	c.Evidence.Dir = ".hoom/worktrees/infra-api"
	hs := []finding.Item{
		ttHallazgo("f1", finding.StatusCorrected, "low", "a.go", "ya corregido"),
		ttHallazgo("f2", finding.StatusOpen, "high", "internal/x.go:12", "falla x"),
		ttHallazgo("f3", finding.StatusOpen, "medium", "", "sin archivo"),
		ttHallazgo("f4", finding.StatusRefuted, "high", "b.go", "refutado"),
		ttHallazgo("f5", finding.StatusOpen, "low", "b.go", "en b.go:40 se rompe"),
		ttHallazgo("f6", finding.StatusOpen, "low", "c.go", "nada que ver"),
	}
	m := ttAbrir(t, ttConfig(100, 24, false), c, tablero.Detalle{})
	m.Tecla("4", ttAhora)
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Findings: hs}})
	orden := []string{"falla x", "sin archivo", "en b.go:40 se rompe", "nada que ver", "ya corregido", "refutado"}
	ant := 0
	for _, d := range orden {
		l, n := ttLineaCon(m, d)
		if n == 0 || n <= ant {
			t.Fatalf("CA-524: los abiertos primero y cada grupo en su orden: %q no esta despues de la linea %d\n%s", d, ant, ttPantalla(m))
		}
		ant = n
		if !strings.HasPrefix(l, " > ") && !strings.HasPrefix(l, "   ") {
			t.Fatalf("CA-524: cada hallazgo es ' ' + M + ' ' + ...: %q", l)
		}
	}
	if l, _ := ttLineaCon(m, "falla x"); !strings.HasPrefix(l, " > abierto high internal/x.go") {
		t.Fatalf("CA-524: el primero esta elegido y dice status, severidad y archivo: %q", l)
	}
	if l, _ := ttLineaCon(m, "ya corregido"); !strings.HasPrefix(l, "   corregido low a.go") {
		t.Fatalf("CA-524: un hallazgo cerrado dice su status: %q", l)
	}
	if l, _ := ttLineaCon(m, "sin archivo"); !strings.HasPrefix(l, "   abierto medium - ") {
		t.Fatalf("CA-524: un hallazgo sin archivo muestra '-': %q", l)
	}
	enter := func(archivo string, linea int) {
		t.Helper()
		os := m.Tecla(tablero.TeclaEnter, ttAhora)
		if len(os) != 1 {
			t.Fatalf("CA-524: enter da una orden AbrirEditor, dio %#v", os)
		}
		if ae, ok := os[0].(tablero.AbrirEditor); !ok || ae.Archivo != archivo || ae.Linea != linea {
			t.Fatalf("CA-524: enter abre %s en la linea %d: %#v", archivo, linea, os[0])
		}
	}
	enter(".hoom/worktrees/infra-api/internal/x.go", 12)
	m.Tecla("j", ttAhora)
	ttSinOrdenes(t, "CA-524", "enter sobre un hallazgo sin archivo", m.Tecla(tablero.TeclaEnter, ttAhora))
	if b := ttBarra1(m); !strings.HasSuffix(b, " · el hallazgo no nombra un archivo") {
		t.Fatalf("CA-524: la barra dice 'el hallazgo no nombra un archivo': %q", b)
	}
	m.Tecla("j", ttAhora)
	enter(".hoom/worktrees/infra-api/b.go", 40)
	m.Tecla(tablero.TeclaAbajo, ttAhora)
	enter(".hoom/worktrees/infra-api/c.go", 0)
	m.Tecla("k", ttAhora)
	enter(".hoom/worktrees/infra-api/b.go", 40)
	// con dir "." el archivo va solo
	c.Evidence.Dir = "."
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Findings: hs}})
	ttTeclas(m, "k", "k", "k", "k")
	enter("internal/x.go", 12)
}

// CA-524: sin hallazgos la pestana dice " sin hallazgos"; un detalle que no
// se pudo leer lo dice y no deja nada viejo a la vista.
func TestCA524_SinHallazgosYErrorDeLectura(t *testing.T) {
	c := ttInfraAPI(false)
	m := ttAbrir(t, ttConfig(100, 24, false), c, tablero.Detalle{})
	m.Tecla("4", ttAhora)
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c}})
	if got := ttLinea(m, 3); got != " sin hallazgos" {
		t.Fatalf("CA-524: sin hallazgos la pestana dice ' sin hallazgos': %q", got)
	}
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
		Findings: []finding.Item{ttHallazgo("f1", finding.StatusOpen, "high", "a.go", "un hallazgo viejo")}}})
	if _, n := ttLineaCon(m, "un hallazgo viejo"); n == 0 {
		t.Fatalf("CA-524: el hallazgo se ve:\n%s", ttPantalla(m))
	}
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Err: errors.New("no se pudo leer la tarjeta")})
	if _, n := ttLineaCon(m, "un hallazgo viejo"); n != 0 {
		t.Fatalf("CA-524: un detalle que no se pudo leer no deja nada viejo a la vista:\n%s", ttPantalla(m))
	}
	if _, n := ttLineaCon(m, "no se pudo"); n == 0 {
		t.Fatalf("CA-524: la pestana dice que el detalle no se pudo leer:\n%s", ttPantalla(m))
	}
	// el detalle de otra tarjeta se ignora
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
		Findings: []finding.Item{ttHallazgo("f1", finding.StatusOpen, "high", "a.go", "de la abierta")}}})
	antes := ttPantalla(m)
	m.LeerDetalle(tablero.Detalle{Slug: "otra", Detalle: boardcmd.Detail{Findings: []finding.Item{ttHallazgo("f9", finding.StatusOpen, "high", "z.go", "de otra")}}})
	if ttPantalla(m) != antes {
		t.Fatalf("CA-524: LeerDetalle de otra tarjeta se ignora:\n%s", ttPantalla(m))
	}
}

// CA-521: la Historia: una linea por entrada " <YYYY-MM-DD HH:MM> <who>
// <plain>", con summary en modo experto; sin entradas " sin historia
// todavia".
func TestCA521_Historia(t *testing.T) {
	c := ttInfraAPI(false)
	m := ttAbrir(t, ttConfig(100, 24, false), c, tablero.Detalle{})
	m.Tecla("5", ttAhora)
	if _, h := m.Abierta(); !h {
		t.Fatalf("CA-521: con la Historia activa, Abierta lo dice (Run la lee solo entonces)")
	}
	tl := boardcmd.Timeline{Slug: c.Slug, Entries: []boardcmd.TimelineEntry{
		{At: time.Date(2026, 11, 7, 10, 30, 0, 0, time.UTC), Kind: boardcmd.KindItem, Who: "Henry Orellana", Plain: "creo la tarjeta", Summary: "item infra-api"},
		{At: time.Date(2026, 11, 7, 11, 5, 0, 0, time.UTC), Kind: boardcmd.KindSpec, Who: "arquitecto (claude)", Plain: "escribio el spec", Summary: "spec .hoom/specs/infra-api.md"},
	}}
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c}, Historia: tl})
	if got := ttLinea(m, 3) + "\n" + ttLinea(m, 4); got != " 2026-11-07 10:30 Henry Orellana creo la tarjeta\n 2026-11-07 11:05 arquitecto (claude) escribio el spec" {
		t.Fatalf("CA-521: la historia es una linea por entrada:\n%s", ttPantalla(m))
	}
	m.Tecla("x", ttAhora)
	if got := ttLinea(m, 3); got != " 2026-11-07 10:30 Henry Orellana item infra-api" {
		t.Fatalf("CA-521: en modo experto la historia dice el summary: %q", got)
	}
	m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c}})
	if got := ttLinea(m, 3); got != " sin historia todavia" {
		t.Fatalf("CA-521: sin entradas: ' sin historia todavia': %q", got)
	}
}
