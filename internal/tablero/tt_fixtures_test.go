// Fixtures de los tests adversariales del spec
// .hoom/specs/tablero-en-la-terminal.md (CA-516..CA-536): tableros de ejemplo
// armados a mano (Card literales: ningun provider detectado, ninguna CLI de
// IA en el PATH), helpers para manejar el Modelo sin terminal y para leer su
// pantalla. Los archivos dorados de testdata/ se escribieron a mano desde el
// contrato de doc.go; ningun test los regenera desde el codigo.
package tablero_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttAhora es el reloj fijo de todos los tests.
var ttAhora = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

const ttSigner = "Henry Orellana <henry@hoom.dev>"

// ttPedidoWriter es el pedido propuesto de pedir-writer: un parrafo mas largo
// que el ancho interior de la caja a 80 columnas y cuatro lineas mas, para que
// el formulario muestre solo las ultimas 4 lineas.
const ttPedidoWriter = "Implementa .hoom/specs/infra-api.md hasta que hoom verify --spec .hoom/specs/infra-api.md de verde." +
	"\nLinea dos del pedido.\nLinea tres.\nLinea cuatro.\nLinea cinco."

// ttWhyAgotado y otros why inventados: si la pantalla los muestra, salen de la
// tarjeta (CA-517).
const (
	ttWhyGuardar   = "no hay nada sin guardar"
	ttWhySinEspac  = "la tarjeta no tiene su propio espacio de trabajo: desde el tablero solo se le pide trabajo a una tarjeta que lo tiene"
	ttWhyOpencode  = "opencode no puede recibir el contrato del rol"
	ttPlainPrecios = "el spec no pasa el lint: le faltan las secciones riesgos y decisiones del contrato"
)

func ttF(v float64) *float64     { return &v }
func ttT(t time.Time) *time.Time { return &t }

func ttSeg(id, label, state, detail string) boardcmd.Segment {
	return boardcmd.Segment{ID: id, Label: label, State: state, Detail: detail}
}

// ttNombreColumna es el nombre de una columna como lo trae el tablero.
func ttNombreColumna(id string) string {
	for _, c := range boardcmd.Columns {
		if c.ID == id {
			return c.Name
		}
	}
	return id
}

// ttCard arma una tarjeta en la columna col, con su meter.
func ttCard(slug, titulo, col, plain string, segs ...boardcmd.Segment) boardcmd.Card {
	c := boardcmd.Card{
		Slug:       slug,
		Item:       item.Item{Slug: slug, Titulo: titulo, Tipo: "feature", Prioridad: "media", CreadoEn: ttAhora.Add(-72 * time.Hour)},
		Column:     col,
		ColumnName: ttNombreColumna(col),
		Missing:    []string{},
		Plain:      plain,
		Meter:      segs,
		Evidence:   boardcmd.CardEvidence{Source: boardcmd.SourceArbol, Dir: ".", Spec: ".hoom/specs/" + slug + ".md"},
		Actions:    []boardcmd.Action{},
		Doctor:     []boardcmd.Problem{},
		Unsynced:   []string{},
	}
	for _, k := range boardcmd.Columns {
		if k.ID == col && k.Human {
			c.WaitingHuman, c.NeedsDecision = true, true
		}
	}
	return c
}

// ttTablero pone las tarjetas en las ocho columnas, en el orden de
// boardcmd.Columns, cada tarjeta en su Column.
func ttTablero(cards ...boardcmd.Card) boardcmd.Board {
	var b boardcmd.Board
	for _, k := range boardcmd.Columns {
		col := boardcmd.BoardColumn{ID: k.ID, Name: k.Name, Human: k.Human, Cards: []boardcmd.Card{}}
		for _, c := range cards {
			if c.Column == k.ID {
				col.Cards = append(col.Cards, c)
			}
		}
		b.Columns = append(b.Columns, col)
	}
	b.Warnings = []string{}
	return b
}

// ---------------------------------------------------------------- acciones

func ttProvClaude(def bool) boardcmd.ProviderOption {
	return boardcmd.ProviderOption{Name: "claude", OK: true, Budget: true, MinBudgetUSD: 0.5, Default: def}
}

func ttProvCodex(def bool) boardcmd.ProviderOption {
	return boardcmd.ProviderOption{Name: "codex", OK: true, Default: def}
}

func ttProvOpencode() boardcmd.ProviderOption {
	return boardcmd.ProviderOption{Name: "opencode", OK: false, Why: ttWhyOpencode}
}

// ttAccionRol es una accion de rol habilitada con los valores propuestos.
func ttAccionRol(id, rol, label, pedido string) boardcmd.Action {
	return boardcmd.Action{ID: id, Role: rol, Label: label, Enabled: true, Pedido: pedido,
		BudgetUSD: ttF(2.75), Providers: []boardcmd.ProviderOption{ttProvClaude(true), ttProvCodex(false)},
		Model: "opus", Effort: "high", TimeoutS: 2700, Paths: []string{}}
}

func ttAccionWriter() boardcmd.Action {
	return ttAccionRol(boardcmd.ActPedirWriter, "writer", "Pedir al writer", ttPedidoWriter)
}

func ttAccionAprobar() boardcmd.Action {
	return boardcmd.Action{ID: boardcmd.ActAprobar, Label: "Aprobar spec", Enabled: true, Signer: ttSigner, Paths: []string{}}
}

func ttAccionIntegrar() boardcmd.Action {
	return boardcmd.Action{ID: boardcmd.ActIntegrar, Label: "Integrar", Enabled: true, Paths: []string{}}
}

func ttAccionGuardar(enabled bool, why string, paths ...string) boardcmd.Action {
	return boardcmd.Action{ID: boardcmd.ActGuardar, Label: "Guardar", Enabled: enabled, Why: why, Paths: paths}
}

// ---------------------------------------------------------------- tarjetas

// ttInfraAPI: la tarjeta en Writer de casi todos los tests. Con running es
// la del contrato: writer en run 3/6, 12:41 de 45:00, narro hace 2s, 3.88 USD.
func ttInfraAPI(running bool) boardcmd.Card {
	c := ttCard("infra-api", "Convenciones de la API", boardcmd.ColWriter, "el writer esta trabajando",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"),
		ttSeg("pruebas", "pruebas", boardcmd.SegHecho, "27/27 con prueba"),
		ttSeg("verde", "verde", boardcmd.SegFalta, "sin veredicto verde"),
		ttSeg("review", "review", boardcmd.SegNoAplica, "no hace falta todavia"))
	c.Next = "hoom agent --role writer --task infra-api"
	c.Missing = []string{"veredicto verde de la tarjeta"}
	c.Evidence.Criteria = 4
	c.Evidence.Untraced = []string{"CA-2", "CA-4"}
	c.Spend = boardcmd.Spend{CostUSD: ttF(3.88), Runs: 1, BudgetUSD: ttF(12.25), RemainingUSD: ttF(8.37)}
	c.Providers = boardcmd.Providers{Writer: "claude", Reviewer: "codex"}
	c.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v-1", Reason: "test: FAIL", Plain: "la verificacion dio rojo"}
	c.Doctor = []boardcmd.Problem{{ID: boardcmd.ProbSpecEditado, Slug: "infra-api", What: "spec editado",
		Plain: "el spec cambio despues de aprobarlo", Action: "hoom aprobar infra-api"}}
	c.Actions = []boardcmd.Action{
		ttAccionWriter(),
		ttAccionGuardar(false, ttWhyGuardar),
		{ID: boardcmd.ActRelanzar, Role: "writer", Label: "Volver a lanzar", Enabled: true, Paths: []string{}},
		ttAccionAprobar(),
		{ID: boardcmd.ActDescartar, Label: "Descartar cambios", Expert: true, Enabled: true, Paths: []string{"x.go"}},
	}
	if running {
		c.Running = &boardcmd.Running{EnvelopeID: "env-api", RunID: "run-api", Role: "writer", Provider: "claude",
			Stage: "run", Step: 3, Steps: 6, StartedAt: ttAhora.Add(-(12*time.Minute + 41*time.Second)),
			TimeoutS: 2700, LastEventAt: ttT(ttAhora.Add(-2 * time.Second))}
	}
	return c
}

func ttNucTenancy() boardcmd.Card {
	c := ttCard("nuc-tenancy", "Tenancy del nucleo", boardcmd.ColTuAprobacion, "te toca aprobar el spec",
		ttSeg("spec", "spec", boardcmd.SegHecho, "pasa el lint"),
		ttSeg("aprobacion", "aprobacion", boardcmd.SegFalta, "sin tu firma"),
		ttSeg("pruebas", "pruebas", boardcmd.SegFalta, "0/5"),
		ttSeg("codigo", "codigo", boardcmd.SegNoAplica, "todavia no"))
	c.Evidence.Criteria = 5
	c.Actions = []boardcmd.Action{ttAccionAprobar()}
	return c
}

func ttPagosCaidos() boardcmd.Card {
	c := ttCard("pagos-caidos", "Pagos caidos", boardcmd.ColWriter, "el trabajo se corto: decide como sigue",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"),
		ttSeg("pruebas", "pruebas", boardcmd.SegHecho, "5/5 con prueba"),
		ttSeg("verde", "verde", boardcmd.SegFalta, "sin veredicto verde"),
		ttSeg("review", "review", boardcmd.SegFalta, "sin review"))
	c.Interrupted = &boardcmd.Interrupted{EnvelopeID: "env-pagos", RunID: "run-pagos", Role: "writer", Provider: "claude",
		Stage: "run", Step: 3, Steps: 5, UpdatedAt: ttAhora.Add(-time.Hour)}
	c.NeedsDecision = true
	c.Actions = []boardcmd.Action{
		{ID: boardcmd.ActReanudar, Role: "writer", Label: "Reanudar", Enabled: true, ResumeID: "sesion-abc123",
			Pedido:    "El trabajo anterior se corto en el paso 3 de 5 (run). Termina lo que se te pidio.",
			Providers: []boardcmd.ProviderOption{ttProvClaude(true)}, TimeoutS: 1800, Paths: []string{}},
	}
	return c
}

func ttBacklog(slug, titulo string) boardcmd.Card {
	c := ttCard(slug, titulo, boardcmd.ColBacklog, "falta el spec de la tarjeta",
		ttSeg("spec", "spec", boardcmd.SegFalta, "no hay spec"),
		ttSeg("aprobacion", "aprobacion", boardcmd.SegFalta, "sin aprobar"),
		ttSeg("pruebas", "pruebas", boardcmd.SegFalta, "sin pruebas"),
		ttSeg("codigo", "codigo", boardcmd.SegFalta, "sin veredicto"))
	c.Actions = []boardcmd.Action{{ID: boardcmd.ActPedirArquitecto, Role: "arquitecto", Label: "Pedir al arquitecto",
		Enabled: false, Why: ttWhySinEspac, Paths: []string{}}}
	return c
}

func ttPrecios() boardcmd.Card {
	return ttCard("precios", "Precios por region", boardcmd.ColArquitecto, ttPlainPrecios,
		ttSeg("item", "item", boardcmd.SegHecho, "existe"),
		ttSeg("spec", "spec", boardcmd.SegFalta, "le faltan secciones"),
		ttSeg("pruebas", "pruebas", boardcmd.SegFalta, "sin pruebas"),
		ttSeg("codigo", "codigo", boardcmd.SegFalta, "sin veredicto"))
}

func ttViejo(n int) boardcmd.Card {
	return ttCard(fmt.Sprintf("viejo-%d", n), fmt.Sprintf("Viejo %d", n), boardcmd.ColHecho, "terminada: su cierre quedo registrado",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"),
		ttSeg("pruebas", "pruebas", boardcmd.SegHecho, "todas"),
		ttSeg("verde", "verde", boardcmd.SegHecho, "verde"),
		ttSeg("review", "review", boardcmd.SegHecho, "hecha"))
}

// ttTableroPrincipal: el tablero de los dorados de la lista. Secciones
// esperadas: NECESITA TU DECISION (nuc-tenancy, pagos-caidos), EN CURSO
// (infra-api), BACKLOG 2, ARQUITECTO 1 y HECHO 3 cerrada.
func ttTableroPrincipal() boardcmd.Board {
	return ttTablero(ttBacklog("infra-ci-cd", "CI y CD de la infra"), ttBacklog("docs-guia", "Guia de la documentacion"),
		ttPrecios(), ttNucTenancy(), ttInfraAPI(true), ttPagosCaidos(), ttViejo(1), ttViejo(2), ttViejo(3))
}

// ---------------------------------------------------------------- el modelo

func ttConfig(ancho, alto int, utf8 bool) tablero.Config {
	return tablero.Config{Proyecto: "demo", Ancho: ancho, Alto: alto, Color: false, UTF8: utf8}
}

// ttModelo es el modelo tras la primera lectura del tablero.
func ttModelo(c tablero.Config, b boardcmd.Board) *tablero.Modelo {
	m := tablero.Nuevo(c)
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: b})
	return m
}

// ttTeclas aprieta las teclas en orden y junta las ordenes.
func ttTeclas(m *tablero.Modelo, teclas ...tablero.Tecla) []tablero.Orden {
	var out []tablero.Orden
	for _, k := range teclas {
		out = append(out, m.Tecla(k, ttAhora)...)
	}
	return out
}

// ttEscribir aprieta una tecla por runa de s.
func ttEscribir(m *tablero.Modelo, s string) []tablero.Orden {
	var out []tablero.Orden
	for _, r := range s {
		out = append(out, m.Tecla(tablero.Tecla(string(r)), ttAhora)...)
	}
	return out
}

// ttPantalla es la pantalla como un texto, una linea por renglon.
func ttPantalla(m *tablero.Modelo) string { return strings.Join(m.Pantalla(), "\n") + "\n" }

// ttLinea es la linea n (1 = el titulo) de la pantalla, o "" si no hay.
func ttLinea(m *tablero.Modelo, n int) string {
	p := m.Pantalla()
	if n < 1 || n > len(p) {
		return ""
	}
	return p[n-1]
}

// ttBarra1 es la barra 1 (linea A-1).
func ttBarra1(m *tablero.Modelo) string {
	p := m.Pantalla()
	if len(p) < 2 {
		return ""
	}
	return p[len(p)-2]
}

// ttBarra2 es la barra 2 (linea A).
func ttBarra2(m *tablero.Modelo) string {
	p := m.Pantalla()
	if len(p) < 1 {
		return ""
	}
	return p[len(p)-1]
}

// ttCuerpo son las lineas 2..A-3.
func ttCuerpo(m *tablero.Modelo) []string {
	p := m.Pantalla()
	if len(p) < 5 {
		return nil
	}
	return p[1 : len(p)-3]
}

// ttLineaCon es la primera linea de la pantalla que contiene s, y su numero
// (1 = el titulo); 0 si ninguna.
func ttLineaCon(m *tablero.Modelo, s string) (string, int) {
	for i, l := range m.Pantalla() {
		if strings.Contains(l, s) {
			return l, i + 1
		}
	}
	return "", 0
}

// ttSeleccionada es el slug de la fila de tarjeta marcada con el signo de la
// seleccion ASCII (">"), o "" si ninguna lo esta.
func ttSeleccionada(m *tablero.Modelo) string {
	for _, l := range ttCuerpo(m) {
		if strings.HasPrefix(l, " > ") {
			campos := strings.Fields(l[3:])
			if len(campos) > 0 {
				return campos[0]
			}
		}
	}
	return ""
}

// ttIrA mueve la seleccion de la lista (con j) hasta la tarjeta slug.
func ttIrA(t *testing.T, m *tablero.Modelo, slug string) {
	t.Helper()
	for i := 0; i < 40; i++ {
		if ttSeleccionada(m) == slug {
			return
		}
		m.Tecla("j", ttAhora)
	}
	t.Fatalf("no se llego a seleccionar %s con j; pantalla:\n%s", slug, ttPantalla(m))
}

// ttLanzares son las ordenes Lanzar de una lista de ordenes.
func ttLanzares(ords []tablero.Orden) []tablero.Lanzar {
	var out []tablero.Lanzar
	for _, o := range ords {
		if l, ok := o.(tablero.Lanzar); ok {
			out = append(out, l)
		}
	}
	return out
}

// ttSinOrdenes exige que no haya ninguna orden.
func ttSinOrdenes(t *testing.T, ca, que string, ords []tablero.Orden) {
	t.Helper()
	if len(ords) != 0 {
		t.Fatalf("%s: %s no ejecuta nada, y dio las ordenes %#v", ca, que, ords)
	}
}

// ttUnaLanzar exige exactamente una orden, Lanzar, y la devuelve.
func ttUnaLanzar(t *testing.T, ca, que string, ords []tablero.Orden) tablero.Lanzar {
	t.Helper()
	if len(ords) != 1 {
		t.Fatalf("%s: %s da exactamente una orden Lanzar, dio %#v", ca, que, ords)
	}
	l, ok := ords[0].(tablero.Lanzar)
	if !ok {
		t.Fatalf("%s: %s da una orden Lanzar, dio %#v", ca, que, ords[0])
	}
	return l
}

// ttMismosArgs compara los Args de un Lanzar.
func ttMismosArgs(t *testing.T, ca, que string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") || len(got) != len(want) {
		t.Fatalf("%s: %s: Args\n got: %q\nwant: %q", ca, que, got, want)
	}
}

// ttDorado compara la pantalla con testdata/<nombre>, escrito a mano desde el
// contrato de doc.go. desde = 1 compara la pantalla entera; desde = 2 deja
// afuera el titulo (el archivo trae entonces las lineas 2..A).
func ttDorado(t *testing.T, ca, nombre string, m *tablero.Modelo, desde int) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", nombre))
	if err != nil {
		t.Fatalf("%s: no pude leer el dorado %s: %v", ca, nombre, err)
	}
	p := m.Pantalla()
	if desde > 1 {
		if len(p) >= desde-1 {
			p = p[desde-1:]
		}
	}
	got := strings.Join(p, "\n") + "\n"
	want := string(raw)
	if got == want {
		return
	}
	gl, wl := strings.Split(got, "\n"), strings.Split(want, "\n")
	var dif []string
	for i := 0; i < len(gl) || i < len(wl); i++ {
		var g, w string
		if i < len(gl) {
			g = gl[i]
		}
		if i < len(wl) {
			w = wl[i]
		}
		if g != w {
			dif = append(dif, fmt.Sprintf("linea %d:\n  got  %q\n  want %q", i+desde, g, w))
		}
	}
	t.Fatalf("%s: la pantalla no es el dorado %s (%d lineas, se esperaban %d):\n%s\n--- pantalla:\n%s",
		ca, nombre, len(m.Pantalla()), len(wl)-1+desde-1, strings.Join(dif, "\n"), got)
}

// ttForma revisa las reglas de toda pantalla: exactamente Alto lineas, cada
// una de a lo sumo Ancho celdas (una runa = una celda), sin espacios al final
// y, sin color, sin ninguna secuencia de escape.
func ttForma(t *testing.T, ca string, m *tablero.Modelo, ancho, alto int, color bool) {
	t.Helper()
	p := m.Pantalla()
	if len(p) != alto {
		t.Fatalf("%s: la pantalla tiene exactamente Alto=%d lineas, tiene %d:\n%s", ca, alto, len(p), strings.Join(p, "\n"))
	}
	for i, l := range p {
		if !color && strings.Contains(l, "\x1b") {
			t.Fatalf("%s: sin color ninguna linea lleva una secuencia de escape (linea %d): %q", ca, i+1, l)
		}
		if strings.TrimRight(l, " ") != l {
			t.Fatalf("%s: la linea %d no termina en espacios: %q", ca, i+1, l)
		}
		if !color && utf8.RuneCountInString(l) > ancho {
			t.Fatalf("%s: la linea %d tiene a lo sumo Ancho=%d celdas, tiene %d: %q", ca, i+1, ancho, utf8.RuneCountInString(l), l)
		}
	}
}

// ttCaja son los textos interiores de la caja (formulario o confirmacion) en
// ASCII: las lineas " | <texto> |", sin los bordes y sin los espacios del
// relleno.
func ttCaja(m *tablero.Modelo) []string {
	var out []string
	for _, l := range m.Pantalla() {
		if strings.HasPrefix(l, " | ") && strings.HasSuffix(l, " |") {
			out = append(out, strings.TrimRight(l[3:len(l)-2], " "))
		}
	}
	return out
}

// ttFlecha es la linea "-> hoom ..." del formulario (ASCII), sin el "-> ".
func ttFlecha(t *testing.T, ca string, m *tablero.Modelo) string {
	t.Helper()
	for _, l := range ttCaja(m) {
		if strings.HasPrefix(l, "-> hoom ") {
			return strings.TrimPrefix(l, "-> ")
		}
	}
	t.Fatalf("%s: el formulario muestra la linea '-> hoom ...'; pantalla:\n%s", ca, ttPantalla(m))
	return ""
}
