// Fixtures de los tests adversariales de hoom tablero: los de la v1 (spec
// .hoom/specs/tablero-en-la-terminal.md, CA-516..CA-537) y los de la v2
// (spec .hoom/specs/tablero-con-paneles.md, CA-550..CA-578). Tableros de
// ejemplo armados a mano (Card literales: ningun provider detectado, ninguna
// CLI de IA en el PATH), helpers para manejar el Modelo sin terminal y para
// leer su pantalla de paneles. Los archivos dorados de testdata/ se
// escribieron a mano desde el contrato de doc.go y el spec; ningun test los
// regenera desde el codigo.
package tablero_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/statuscmd"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/valores"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ttAhora es el reloj fijo de todos los tests.
var ttAhora = time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)

const ttSigner = "Henry Orellana <henry@hoom.dev>"

// ttPedidoWriter es el pedido propuesto de pedir-writer: un parrafo mas largo
// que el ancho de una linea del modal y cuatro lineas mas.
const ttPedidoWriter = "Implementa .hoom/specs/infra-api.md hasta que hoom verify --spec .hoom/specs/infra-api.md de verde." +
	"\nLinea dos del pedido.\nLinea tres.\nLinea cuatro.\nLinea cinco."

// Los why inventados: si la pantalla los muestra, salen de la tarjeta
// (CA-517).
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

// ttOrigenes son los origenes de una accion de rol de los fixtures: el
// provider de la tarjeta, el modelo y el esfuerzo de hoom.yaml, el tope de
// la tarjeta y el tiempo limite de hoom.yaml.
func ttOrigenes() valores.Origenes {
	return valores.Origenes{Provider: valores.OrigenTarjeta, Model: valores.OrigenHoomYAML, Effort: valores.OrigenHoomYAML,
		BudgetUSD: valores.OrigenTarjeta, Timeout: valores.OrigenHoomYAML}
}

// ttAccionRol es una accion de rol habilitada con los valores propuestos.
func ttAccionRol(id, rol, label, pedido string) boardcmd.Action {
	return boardcmd.Action{ID: id, Role: rol, Label: label, Enabled: true, Pedido: pedido,
		BudgetUSD: ttF(2.75), Providers: []boardcmd.ProviderOption{ttProvClaude(true), ttProvCodex(false)},
		Model: "opus", Effort: "high", TimeoutS: 2700, Paths: []string{}, Origen: ttOrigenes()}
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

// ttRelanzar es una accion sin verbo de la capa 1 (sin fila en Atajos).
func ttRelanzar() boardcmd.Action {
	return boardcmd.Action{ID: boardcmd.ActRelanzar, Role: "writer", Label: "Volver a lanzar", Enabled: true, Paths: []string{}}
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
		ttRelanzar(),
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
		ttSeg("pruebas", "pruebas", boardcmd.SegFalta, "1/2"),
		ttSeg("codigo", "codigo", boardcmd.SegNoAplica, "todavia no"))
	// hallazgo 0ee5e4: la tira, los criterios del spec (2, uno sin prueba) y
	// su hallazgo en [4] dicen lo mismo
	c.Evidence.Criteria = 2
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
			Providers: []boardcmd.ProviderOption{ttProvClaude(true)}, TimeoutS: 1800, Paths: []string{},
			Origen: valores.Origenes{Provider: valores.OrigenTarjeta, Model: valores.OrigenProvider, Effort: valores.OrigenProvider,
				BudgetUSD: valores.OrigenProvider, Timeout: valores.OrigenProvider}},
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

// ttTableroPrincipal: el tablero de los dorados. Secciones esperadas en [2]:
// NECESITA TU DECISION (nuc-tenancy, pagos-caidos), EN CURSO (infra-api),
// BACKLOG 2 (infra-ci-cd, docs-guia), ARQUITECTO 1 (precios) y HECHO 3
// cerrada. En [3]: infra-api (running) y pagos-caidos (interrupted).
func ttTableroPrincipal() boardcmd.Board {
	return ttTablero(ttBacklog("infra-ci-cd", "CI y CD de la infra"), ttBacklog("docs-guia", "Guia de la documentacion"),
		ttPrecios(), ttNucTenancy(), ttInfraAPI(true), ttPagosCaidos(), ttViejo(1), ttViejo(2), ttViejo(3))
}

// ttTarjetaCon es una tarjeta en Writer con solo esas acciones.
func ttTarjetaCon(acciones ...boardcmd.Action) boardcmd.Card {
	c := ttCard("tarjeta", "Tarjeta de prueba", boardcmd.ColWriter, "el plain de la tarjeta",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"), ttSeg("verde", "verde", boardcmd.SegFalta, "sin verde"))
	c.Actions = append([]boardcmd.Action{}, acciones...)
	return c
}

// ttHallazgo es un hallazgo de la tarea infra-api, de la lente risk.
func ttHallazgo(id, status, sev, file, desc string) finding.Item {
	return finding.Item{Finding: finding.Finding{ID: id, Severity: sev, Lens: "risk", File: file, Description: desc,
		Task: "infra-api", CreatedAt: ttAhora.Add(-time.Hour)}, Status: status}
}

// ttEvento es un evento de la narracion seg segundos despues de ttAhora.
func ttEvento(seg int, kind, detail string) runcmd.Event {
	return runcmd.Event{TS: ttAhora.Add(time.Duration(seg) * time.Second), Kind: kind, Detail: detail}
}

// ---------------------------------------------------------------- proyecto y hallazgos

// ttVeredicto es un ultimo veredicto verde de hace dos horas.
func ttVeredicto() *statuscmd.LastVerdict {
	return &statuscmd.LastVerdict{ID: "20261108T100000_abc123", Verdict: "green", CreatedAt: ttAhora.Add(-2 * time.Hour),
		Summary: verdict.Summary{Pass: 7, Fail: 0, Error: 0, Absent: 1}}
}

// ttProyectoVerde es el proyecto de los dorados: con git en main, el check
// verde de hace dos horas, cuatro trabajos hoy y sin problemas.
func ttProyectoVerde() tablero.Proyecto {
	v := ttVeredicto()
	return tablero.Proyecto{Git: true, Rama: "main",
		Check:     checkcmd.Result{OK: true, VerdictID: v.ID, LastVerdict: "green", FingerprintMatch: true},
		Veredicto: v, Hoy: costcmd.Hoy{Runs: 4, CostoUSD: 3.88}, Problemas: []boardcmd.Problem{}}
}

// ttHallazgoNuc es el hallazgo low abierto de nuc-tenancy de los dorados: el
// que [4] lista y el que [0] de la tarjeta muestra en su parte Hallazgos.
func ttHallazgoNuc() finding.Item {
	low := ttHallazgo("h-low", finding.StatusOpen, "low", "internal/tenant/tenant.go:7", "el nombre del tenant no se recorta")
	low.Task = "nuc-tenancy"
	return low
}

// ttHallazgosDorados son los dos hallazgos abiertos de los dorados: uno low
// de nuc-tenancy y uno high de infra-api (en ese orden de lectura).
func ttHallazgosDorados() tablero.Hallazgos {
	low := ttHallazgoNuc()
	high := ttHallazgo("h-high", finding.StatusOpen, "high", "internal/api/router.go:41", "el router acepta rutas sin version")
	return tablero.Hallazgos{Items: []finding.Item{low, high}}
}

// ttDetalleNuc es el detalle de nuc-tenancy de los dorados: su spec con dos
// criterios (CA-1 con prueba, CA-2 sin prueba, como dice la tira: "1/2"), el
// hallazgo low de su tarea (el mismo que lista [4]), sin historia y sin
// sobres (hallazgo 0ee5e4: los fixtures no se contradicen).
func ttDetalleNuc() tablero.Detalle {
	c := ttNucTenancy()
	c.Evidence.Untraced = []string{"CA-2"}
	return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c,
		Findings: []finding.Item{ttHallazgoNuc()},
		Spec: boardcmd.SpecDetail{Path: ".hoom/specs/nuc-tenancy.md", Exists: true,
			Markdown: "# Spec\n\n## Criterios de aceptacion\n\n- CA-1: cada tenant ve solo sus filas\n- CA-2: un tenant sin plan responde 402\n"},
		Criteria: []boardcmd.CriterionTrace{
			{ID: "CA-1", Text: "cada tenant ve solo sus filas", TracedBy: boardcmd.TracedTest},
			{ID: "CA-2", Text: "un tenant sin plan responde 402"},
		}}}
}

// ttNucConUntraced es nuc-tenancy con CA-2 en untraced (la del tablero de
// los dorados).
func ttNucConUntraced() boardcmd.Card {
	c := ttNucTenancy()
	c.Evidence.Untraced = []string{"CA-2"}
	return c
}

// ttTableroDorado es ttTableroPrincipal con la nuc-tenancy de ttDetalleNuc.
func ttTableroDorado() boardcmd.Board {
	return ttTablero(ttBacklog("infra-ci-cd", "CI y CD de la infra"), ttBacklog("docs-guia", "Guia de la documentacion"),
		ttPrecios(), ttNucConUntraced(), ttInfraAPI(true), ttPagosCaidos(), ttViejo(1), ttViejo(2), ttViejo(3))
}

// ---------------------------------------------------------------- el modelo

func ttConfig(ancho, alto int, utf8 bool) tablero.Config {
	return tablero.Config{Proyecto: "demo", Ancho: ancho, Alto: alto, Color: false, UTF8: utf8, Editor: "vi"}
}

// ttAncho es una pantalla donde entran los mensajes y los comandos enteros.
func ttAncho() tablero.Config { return ttConfig(200, 40, false) }

// ttModelo es el modelo tras la primera lectura del tablero.
func ttModelo(c tablero.Config, b boardcmd.Board) *tablero.Modelo {
	m := tablero.Nuevo(c)
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: b})
	return m
}

// ttModeloDorado es el modelo de los dorados: el tablero, el proyecto, los
// hallazgos y el detalle de la tarjeta elegida, nuc-tenancy fijada.
func ttModeloDorado(c tablero.Config) *tablero.Modelo {
	m := tablero.Nuevo(c)
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTableroDorado(), Fijada: "nuc-tenancy"})
	m.LeerProyecto(ttProyectoVerde())
	m.LeerHallazgos(ttHallazgosDorados())
	m.LeerDetalle(ttDetalleNuc())
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

// ttLinea es la linea n (1 = la primera) de la pantalla, o "" si no hay.
func ttLinea(m *tablero.Modelo, n int) string {
	p := m.Pantalla()
	if n < 1 || n > len(p) {
		return ""
	}
	return p[n-1]
}

// ttLineaCon es la primera linea de la pantalla (sin SGR) que contiene s, y
// su numero (1 = la primera); 0 si ninguna.
func ttLineaCon(m *tablero.Modelo, s string) (string, int) {
	for i, l := range m.Pantalla() {
		l = ttSinSGR(l)
		if strings.Contains(l, s) {
			return l, i + 1
		}
	}
	return "", 0
}

// ttTiene dice si la pantalla (sin SGR) contiene s.
func ttTiene(m *tablero.Modelo, s string) bool {
	_, n := ttLineaCon(m, s)
	return n > 0
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

// ttHaceDe es el Hace de la fila de Atajos que tiene la tecla k con una
// Accion (la tabla es la del despacho).
func ttHaceDe(t *testing.T, k tablero.Tecla) string {
	t.Helper()
	for _, a := range tablero.Atajos {
		for _, x := range a.Teclas {
			if x == k && a.Accion != "" {
				return a.Hace
			}
		}
	}
	t.Fatalf("la tecla %q no esta en ninguna fila de Atajos con Accion", k)
	return ""
}

// ttEnPantalla es un texto de la capa 1 (CitarPedido, que corta con "…")
// como lo muestra la pantalla: en ASCII el "…" es el signo del corte, "~"
// (decision D3 del orquestador: ninguna pantalla ASCII lleva "…").
func ttEnPantalla(s string, utf bool) string {
	if utf {
		return s
	}
	return strings.ReplaceAll(s, "…", "~")
}

// ttFuera es el mensaje de un archivo que sale del proyecto.
func ttFuera(ruta string) string {
	return "el archivo " + strconv.Quote(ruta) + " esta fuera del proyecto"
}

// ---------------------------------------------------------------- color

var (
	ttSGR   = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	ttRojo  = regexp.MustCompile(`\x1b\[(?:[0-9]+;)*31m`)
	ttVerde = regexp.MustCompile(`\x1b\[(?:[0-9]+;)*32m`)
)

// ttSinSGR es la linea sin las secuencias SGR del color.
func ttSinSGR(s string) string { return ttSGR.ReplaceAllString(s, "") }

// ttConColor dice si en la linea hay una secuencia del color re antes de
// texto, sin un reset entre los dos.
func ttConColor(re *regexp.Regexp, linea, texto string) bool {
	i := strings.Index(linea, texto)
	if i < 0 {
		return false
	}
	locs := re.FindAllStringIndex(linea[:i], -1)
	if len(locs) == 0 {
		return false
	}
	ultimo := locs[len(locs)-1]
	return !strings.Contains(linea[ultimo[1]:i], "\x1b[0m")
}

// ttEstiloDe es la secuencia SGR que rige el texto en la linea (con color):
// la ultima "\x1b[...m" antes de texto que no es un reset, o "" si el texto
// no tiene estilo.
func ttEstiloDe(linea, texto string) string {
	i := strings.Index(linea, texto)
	if i < 0 {
		return "?"
	}
	locs := ttSGR.FindAllStringIndex(linea[:i], -1)
	if len(locs) == 0 {
		return ""
	}
	u := locs[len(locs)-1]
	sgr := linea[u[0]:u[1]]
	if sgr == "\x1b[0m" {
		return ""
	}
	return sgr
}

// ---------------------------------------------------------------- las dos ultimas lineas

// ttMensaje es la linea de mensajes (A-1) sin SGR y sin su margen.
func ttMensaje(m *tablero.Modelo) string {
	p := m.Pantalla()
	if len(p) < 2 {
		return ""
	}
	return strings.TrimPrefix(ttSinSGR(p[len(p)-2]), " ")
}

// ttMensajeCrudo es la linea de mensajes con sus SGR.
func ttMensajeCrudo(m *tablero.Modelo) string {
	p := m.Pantalla()
	if len(p) < 2 {
		return ""
	}
	return p[len(p)-2]
}

// ttDiceMensaje exige que la linea de mensajes diga msg (en modo experto,
// tras "EXPERTO · "); con rojo, que lo diga en rojo.
func ttDiceMensaje(t *testing.T, ca, que string, m *tablero.Modelo, rojo bool, msg string) {
	t.Helper()
	got := ttMensaje(m)
	if got != msg && got != "EXPERTO · "+msg {
		t.Fatalf("%s: %s: la linea de mensajes dice %q, dice %q", ca, que, msg, got)
	}
	if rojo && !ttConColor(ttRojo, ttMensajeCrudo(m), msg) {
		t.Fatalf("%s: %s: la linea de mensajes dice %q en rojo: %q", ca, que, msg, ttMensajeCrudo(m))
	}
}

// ttLineaTeclas es la ultima linea (las teclas), sin SGR.
func ttLineaTeclas(m *tablero.Modelo) string {
	p := m.Pantalla()
	if len(p) < 1 {
		return ""
	}
	return ttSinSGR(p[len(p)-1])
}

// ---------------------------------------------------------------- celdas y cajas

// ttAnchoRuna es el ancho de una runa para los helpers de los tests (una
// tabla propia y corta, independiente de la del paquete): 0 las marcas
// combinantes, 2 los anchos de Asia del Este y los emojis de los fixtures, 1
// el resto.
func ttAnchoRuna(r rune) int {
	switch {
	case unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r):
		return 0
	case r == 0x270B, r >= 0x1100 && r <= 0x115F, r >= 0x2E80 && r <= 0x303E, r >= 0x3041 && r <= 0xA4CF,
		r >= 0xAC00 && r <= 0xD7A3, r >= 0xF900 && r <= 0xFAFF, r >= 0xFE30 && r <= 0xFE4F, r >= 0xFF00 && r <= 0xFF60,
		r >= 0xFFE0 && r <= 0xFFE6, r >= 0x1F300 && r <= 0x1F64F, r >= 0x1F680 && r <= 0x1F6FF, r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD:
		return 2
	}
	return 1
}

// ttCeldasDe parte una linea (sin SGR) en celdas: una runa ancha ocupa su
// celda y la siguiente queda "" (su segunda mitad); una marca combinante se
// pega a la celda anterior.
func ttCeldasDe(l string) []string {
	var out []string
	for _, r := range l {
		switch ttAnchoRuna(r) {
		case 0:
			if len(out) > 0 {
				out[len(out)-1] += string(r)
			} else {
				out = append(out, string(r))
			}
		case 2:
			out = append(out, string(r), "")
		default:
			out = append(out, string(r))
		}
	}
	return out
}

// ttCeldas cuenta las celdas de s con la tabla de los tests.
func ttCeldas(s string) int { return len(ttCeldasDe(s)) }

// ttTramo es el texto de las celdas [desde, hasta) de la linea l (sin SGR).
func ttTramo(l string, desde, hasta int) string {
	c := ttCeldasDe(l)
	if desde > len(c) {
		return ""
	}
	if hasta > len(c) {
		hasta = len(c)
	}
	return strings.Join(c[desde:hasta], "")
}

func ttEsEsquina(s string) bool {
	return s == "+" || s == "#" || s == "┌" || s == "╔" || s == "┐" || s == "╗" || s == "└" || s == "╚" || s == "┘" || s == "╝"
}

func ttEsLinea(s string) bool { return s == "-" || s == "=" || s == "─" || s == "═" }

func ttEsLado(s string) bool { return s == "|" || s == "#" || s == "│" || s == "║" }

// ttCaja es un panel (o el Registro) tal como esta en la pantalla.
type ttCaja struct {
	Nombre  string // "[0]".."[4]" o "Registro"
	Fila    int    // la linea de su borde de arriba (o la plegada), desde 0
	Col     int    // su primera celda, desde 0
	Ancho   int    // sus celdas (plegado: hasta el final de su tramo)
	Alto    int    // sus lineas, bordes incluidos (plegado: 1)
	Plegado bool
	Foco    bool     // signos del foco (= y # en ASCII, doble con UTF-8)
	Arriba  string   // el borde de arriba entero (o la linea plegada)
	Titulo  string   // el titulo: lo que sigue a la esquina hasta los signos de linea del final
	Dentro  []string // sus lineas interiores, Ancho-2 celdas cada una (sin recortar)
	Abajo   string   // el borde de abajo entero
}

// ttLimpia son las lineas de la pantalla sin SGR.
func ttLimpia(m *tablero.Modelo) []string {
	var out []string
	for _, l := range m.Pantalla() {
		out = append(out, ttSinSGR(l))
	}
	return out
}

// ttTituloDe es el titulo en un borde de arriba (desde la celda despues de
// la esquina): el texto sin los signos de linea del final ni la esquina.
func ttTituloDe(celdas []string) string {
	fin := len(celdas)
	if fin > 0 && ttEsEsquina(celdas[fin-1]) {
		fin--
	}
	for fin > 0 && ttEsLinea(celdas[fin-1]) {
		fin--
	}
	return strings.Join(celdas[:fin], "")
}

// ttCajas encuentra los paneles de la pantalla por sus titulos: una esquina
// seguida de "[n]" o de "Registro" (un panel desplegado: su borde de abajo es
// la primera linea mas abajo, en la misma celda, que sigue con un signo de
// linea), o un signo de linea seguido de "[n]" o "Registro" (un panel
// plegado).
func ttCajas(m *tablero.Modelo) map[string]ttCaja {
	ls := ttLimpia(m)
	out := map[string]ttCaja{}
	for i, l := range ls {
		c := ttCeldasDe(l)
		for j := 0; j+1 < len(c); j++ {
			nombre := ""
			resto := strings.Join(c[j+1:], "")
			switch {
			case len(resto) >= 3 && resto[0] == '[' && resto[1] >= '0' && resto[1] <= '4' && resto[2] == ']':
				nombre = resto[:3]
			case strings.HasPrefix(resto, "Registro"):
				nombre = "Registro"
			default:
				continue
			}
			if j > 0 && !ttEsLinea(c[j-1]) && !ttEsEsquina(c[j-1]) && !ttEsLado(c[j-1]) && c[j-1] != " " {
				continue
			}
			if _, ya := out[nombre]; ya {
				continue
			}
			if ttEsLinea(c[j]) {
				// plegado: hasta el final de la linea, o hasta la esquina o el
				// lado del panel de la derecha que sigue a sus signos de linea
				fin := len(c)
				for k := j + 2; k < len(c); k++ {
					if ttEsLinea(c[k-1]) && (ttEsEsquina(c[k]) || ttEsLado(c[k])) {
						fin = k
						break
					}
				}
				tramo := c[j:fin]
				out[nombre] = ttCaja{Nombre: nombre, Fila: i, Col: j, Ancho: len(tramo), Alto: 1, Plegado: true,
					Foco: c[j] == "=" || c[j] == "═", Arriba: strings.Join(tramo, ""), Titulo: ttTituloDe(tramo[1:])}
				continue
			}
			if !ttEsEsquina(c[j]) {
				continue
			}
			// el borde de abajo
			abajo := -1
			for k := i + 1; k < len(ls); k++ {
				ck := ttCeldasDe(ls[k])
				if j+1 >= len(ck) {
					break
				}
				if ttEsEsquina(ck[j]) && ttEsLinea(ck[j+1]) {
					abajo = k
					break
				}
				if !ttEsLado(ck[j]) {
					break
				}
			}
			if abajo < 0 {
				continue
			}
			cb := ttCeldasDe(ls[abajo])
			der := -1
			for k := j + 1; k < len(cb); k++ {
				if ttEsEsquina(cb[k]) {
					der = k
					break
				}
			}
			if der < 0 {
				continue
			}
			w := der - j + 1
			caja := ttCaja{Nombre: nombre, Fila: i, Col: j, Ancho: w, Alto: abajo - i + 1, Foco: c[j] == "#" || c[j] == "╔",
				Arriba: ttTramo(l, j, j+w), Abajo: ttTramo(ls[abajo], j, j+w)}
			caja.Titulo = ttTituloDe(ttCeldasDe(caja.Arriba)[1:])
			for k := i + 1; k < abajo; k++ {
				caja.Dentro = append(caja.Dentro, ttTramo(ls[k], j+1, j+w-1))
			}
			out[nombre] = caja
		}
	}
	return out
}

// ttCajaDe exige el panel nombre en la pantalla y lo devuelve.
func ttCajaDe(t *testing.T, ca string, m *tablero.Modelo, nombre string) ttCaja {
	t.Helper()
	c, ok := ttCajas(m)[nombre]
	if !ok {
		t.Fatalf("%s: la pantalla tiene el panel %s:\n%s", ca, nombre, ttPantalla(m))
	}
	return c
}

// ttTexto son las lineas interiores de una caja sin el relleno de la
// derecha.
func (c ttCaja) ttTexto() []string {
	var out []string
	for _, l := range c.Dentro {
		out = append(out, strings.TrimRight(l, " "))
	}
	return out
}

// ttElegidaEn es la fila de la caja que empieza con el signo de la seleccion
// (" > " o " ▸ "), sin ese comienzo y sin el relleno; "" si ninguna.
func ttElegidaEn(c ttCaja) string {
	for _, l := range c.Dentro {
		if strings.HasPrefix(l, " > ") || strings.HasPrefix(l, " ▸ ") {
			return strings.TrimRight(strings.TrimPrefix(strings.TrimPrefix(l, " > "), " ▸ "), " ")
		}
	}
	return ""
}

// ttSlugElegido es el slug de la fila elegida de [2] (su primera palabra), o
// "" si ninguna fila esta elegida.
func ttSlugElegido(t *testing.T, m *tablero.Modelo) string {
	t.Helper()
	f := strings.Fields(ttElegidaEn(ttCajaDe(t, "CA-552", m, "[2]")))
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// ttIrA mueve la seleccion de [2] (con j) hasta la tarjeta slug; el foco
// tiene que estar en [2].
func ttIrA(t *testing.T, m *tablero.Modelo, slug string) {
	t.Helper()
	for i := 0; i < 60; i++ {
		if ttSlugElegido(t, m) == slug || m.Elegida() == slug {
			return
		}
		m.Tecla("j", ttAhora)
	}
	t.Fatalf("no se llego a elegir %s con j en [2]; pantalla:\n%s", slug, ttPantalla(m))
}

// ttFocos son los nombres de las cajas con los signos del foco.
func ttFocos(m *tablero.Modelo) []string {
	var out []string
	for n, c := range ttCajas(m) {
		if c.Foco {
			out = append(out, n)
		}
	}
	return out
}

// ttNombrePanel es el nombre en pantalla de un Panel.
func ttNombrePanel(p tablero.Panel) string { return fmt.Sprintf("[%d]", int(p)) }

// ---------------------------------------------------------------- el modal

// ttModalCaja es el modal abierto tal como esta en la pantalla.
type ttModalCaja struct {
	Fila, Col, Ancho, Alto int
	Arriba, Abajo          string
	Textos                 []string // el texto de cada linea interior (Wm-6 celdas), sin el relleno de la derecha
	Marcas                 []bool   // si la linea lleva la marca del foco ("> " o "▸ ")
}

// ttAnchoModal es Wm = min(72, W-4) (doc.go, "The modals").
func ttAnchoModal(ancho int) int {
	if ancho-4 < 72 {
		return ancho - 4
	}
	return 72
}

// ttModal encuentra el modal: un borde de arriba con la esquina del foco en
// la celda (W-Wm)/2 seguida de un espacio, y la esquina de la derecha en la
// ultima celda del modal.
func ttModal(t *testing.T, ca string, m *tablero.Modelo, ancho int) ttModalCaja {
	t.Helper()
	wm := ttAnchoModal(ancho)
	col := (ancho - wm) / 2
	ls := ttLimpia(m)
	for i, l := range ls {
		c := ttCeldasDe(l)
		if len(c) < col+wm || !(c[col] == "#" || c[col] == "╔") || c[col+1] != " " || !(c[col+wm-1] == "#" || c[col+wm-1] == "╗") {
			continue
		}
		mc := ttModalCaja{Fila: i, Col: col, Ancho: wm, Arriba: ttTramo(l, col, col+wm)}
		for k := i + 1; k < len(ls); k++ {
			ck := ttCeldasDe(ls[k])
			if len(ck) < col+wm {
				break
			}
			// el borde de abajo lleva los signos de linea del foco (= o ═),
			// que una lista abierta (signos simples) no usa
			foco := func(x string) bool { return x == "=" || x == "═" }
			if (ck[col] == "#" || ck[col] == "╚") && (foco(ck[col+1]) || foco(ck[col+wm-2])) {
				mc.Abajo = ttTramo(ls[k], col, col+wm)
				mc.Alto = k - i + 1
				return mc
			}
			mark := ttTramo(ls[k], col+2, col+4)
			mc.Marcas = append(mc.Marcas, mark == "> " || mark == "▸ ")
			mc.Textos = append(mc.Textos, strings.TrimRight(ttTramo(ls[k], col+4, col+wm-2), " "))
		}
	}
	t.Fatalf("%s: la pantalla tiene un modal de %d celdas en la celda %d:\n%s", ca, wm, col, ttPantalla(m))
	return ttModalCaja{}
}

// ttEnModal dice si alguna linea del modal es exactamente s.
func (mc ttModalCaja) ttTieneLinea(s string) bool {
	for _, l := range mc.Textos {
		if l == s {
			return true
		}
	}
	return false
}

// ttLineaQueEmpieza es la primera linea del modal que empieza con s, su
// indice y si lleva la marca; "" y -1 si ninguna.
func (mc ttModalCaja) ttLineaQueEmpieza(s string) (string, int, bool) {
	for i, l := range mc.Textos {
		if strings.HasPrefix(l, s) {
			return l, i, mc.Marcas[i]
		}
	}
	return "", -1, false
}

// ttMarcada es el texto de la linea del modal con la marca del foco, o "".
func (mc ttModalCaja) ttMarcada() string {
	for i, l := range mc.Textos {
		if mc.Marcas[i] {
			return l
		}
	}
	return ""
}

// ttComando es el comando que el modal muestra bajo "Se va a ejecutar:" (o
// en su misma linea), con sus dos lineas unidas por un espacio.
func (mc ttModalCaja) ttComando() string {
	for i, l := range mc.Textos {
		if strings.HasPrefix(l, "Se va a ejecutar:") {
			if resto := strings.TrimSpace(strings.TrimPrefix(l, "Se va a ejecutar:")); resto != "" {
				out := resto
				if i+1 < len(mc.Textos) && strings.HasPrefix(mc.Textos[i+1], "  ") && strings.TrimSpace(mc.Textos[i+1]) != "" {
					out += " " + strings.TrimSpace(mc.Textos[i+1])
				}
				return out
			}
			if i+1 >= len(mc.Textos) {
				return ""
			}
			out := mc.Textos[i+1]
			if i+2 < len(mc.Textos) && strings.HasPrefix(mc.Textos[i+2], "  ") && strings.TrimSpace(mc.Textos[i+2]) != "" {
				out += " " + strings.TrimSpace(mc.Textos[i+2])
			}
			return out
		}
	}
	return ""
}

// ttCampo es la linea del modal Pedir que empieza con la etiqueta, sin la
// marca; "" si no esta.
func (mc ttModalCaja) ttCampo(etiqueta string) string {
	l, _, _ := mc.ttLineaQueEmpieza(etiqueta)
	return l
}

// ttValorYOrigen parte la linea de un selector, desde la celda 15 (donde
// empieza el valor: la etiqueta va rellena a 15), en su valor y lo que le
// sigue (el origen o el why, en la columna 15 + V). Un valor con flechas
// ("< opus >", "‹ sin tope ›") termina en su flecha de cierre; uno sin
// flechas (el provider de reanudar, un texto de otro…) termina donde empiezan
// dos espacios o la linea. Entre el valor y el origen puede haber solo dos
// espacios (V = max(26, el mas ancho + 2)).
func ttValorYOrigen(l string) (string, string) {
	c := ttCeldasDe(l)
	if len(c) <= 15 {
		return "", ""
	}
	v := strings.Join(c[15:], "")
	if strings.HasPrefix(v, "<") || strings.HasPrefix(v, "‹") {
		fin := strings.IndexAny(v, ">›")
		if fin >= 0 {
			_, n := utf8.DecodeRuneInString(v[fin:])
			return v[:fin+n], strings.TrimSpace(v[fin+n:])
		}
		return strings.TrimSpace(v), ""
	}
	if i := strings.Index(v, "  "); i >= 0 {
		return v[:i], strings.TrimSpace(v[i:])
	}
	return strings.TrimSpace(v), ""
}

// ttValor es el valor de un selector (ver ttValorYOrigen).
func (mc ttModalCaja) ttValor(etiqueta string) string {
	v, _ := ttValorYOrigen(mc.ttCampo(etiqueta))
	return v
}

// ttOrigen es lo que sigue al valor de un selector en su linea (su origen o
// su why).
func (mc ttModalCaja) ttOrigen(etiqueta string) string {
	_, o := ttValorYOrigen(mc.ttCampo(etiqueta))
	return o
}

// ---------------------------------------------------------------- dorados y forma

// ttDorado compara la pantalla entera con testdata/<nombre>, escrito a mano
// desde el contrato de doc.go y el spec.
func ttDorado(t *testing.T, ca, nombre string, m *tablero.Modelo) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", nombre))
	if err != nil {
		t.Fatalf("%s: no pude leer el dorado %s: %v", ca, nombre, err)
	}
	got := strings.Join(m.Pantalla(), "\n") + "\n"
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
			dif = append(dif, fmt.Sprintf("linea %d:\n  got  %q\n  want %q", i+1, g, w))
		}
	}
	t.Fatalf("%s: la pantalla no es el dorado %s (%d lineas, se esperaban %d):\n%s\n--- pantalla:\n%s",
		ca, nombre, len(m.Pantalla()), len(wl)-1, strings.Join(dif, "\n"), got)
}

// ttForma revisa las reglas de toda pantalla (doc.go, "The screen"):
// exactamente Alto lineas, cada una de a lo sumo Ancho celdas (con la tabla
// de anchos: CA-577), sin espacios al final y, sin color, sin ninguna
// secuencia de escape.
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
		s := ttSinSGR(l)
		if strings.TrimRight(s, " ") != s {
			t.Fatalf("%s: la linea %d no termina en espacios: %q", ca, i+1, l)
		}
		if n := tablero.Celdas(s); n > ancho {
			t.Fatalf("%s: la linea %d tiene a lo sumo Ancho=%d celdas, tiene %d: %q", ca, i+1, ancho, n, l)
		}
	}
}

// ttArchivo escribe rel bajo root (con sus carpetas) y devuelve su ruta.
func ttArchivo(t *testing.T, root, rel, contenido string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}
