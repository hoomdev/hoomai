// Fixtures de los tests adversariales de la v3 de hoom tablero (spec
// .hoom/specs/tablero-tu-siguiente-paso.md, CA-704..CA-719): tarjetas con el
// piloto encendido, el piloto que lanza el tablero (sin ejecutar nada: el
// modelo recibe FinProceso con cada codigo y cada ultima linea) y la
// geometria de la pantalla de la v3 escrita desde el contrato de doc.go:
//
//	linea 1          la cabecera                      (indice 0)
//	lineas 2..5      la caja TU SIGUIENTE PASO        (indices 1..4)
//	lineas 6..A-3    el cuerpo: la lista y el detalle (indices 5..A-4)
//	linea A-2        el Registro plegado              (indice A-3)
//	linea A-1        la linea de mensajes             (indice A-2)
//	linea A          la linea de teclas               (indice A-1)
//
// Con W >= 100 la lista son las celdas 0..L-1 (L = W*2/5) y el detalle las
// celdas L..W-1, las dos de C = A-8 lineas. Todos los helpers usan el reloj
// fijo ttAhora y nunca entran en panico: lo que no encuentran es Fatalf.
package tablero_test

import (
	"fmt"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// tv3cPrefijoParada es el prefijo de la ultima linea de hoom piloto cuando se
// detiene (doc.go, "Seguir solo").
const tv3cPrefijoParada = "piloto automatico: se detiene: "

// tv3cConfig es una pantalla ASCII sin color, con el mouse pedido.
func tv3cConfig(ancho, alto int) tablero.Config {
	c := ttConfig(ancho, alto, false)
	c.Mouse = true
	return c
}

// tv3cConfigColor es tv3cConfig con color.
func tv3cConfigColor(ancho, alto int) tablero.Config {
	c := tv3cConfig(ancho, alto)
	c.Color = true
	return c
}

// tv3cL es el ancho de la lista con W >= 100.
func tv3cL(ancho int) int { return ancho * 2 / 5 }

// tv3cAccionDe es la accion de rol de cada columna de un rol.
func tv3cAccionDe(col string) boardcmd.Action {
	switch col {
	case boardcmd.ColTestWriter:
		return ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "los tests")
	case boardcmd.ColReview:
		a := ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", "")
		return a
	case boardcmd.ColArquitecto, boardcmd.ColBacklog:
		return ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "el spec")
	}
	return ttAccionRol(boardcmd.ActPedirWriter, "writer", "Pedir al writer", "el codigo")
}

// tv3cSola es una tarjeta de la columna col con el piloto encendido (auto:
// hasta-humano), sin running, sin interrupted y sin rojo, con la accion de
// rol de su columna: Seguir solo esta disponible en Test-writer, Writer y
// Review.
func tv3cSola(slug, titulo, col string) boardcmd.Card {
	c := ttCard(slug, titulo, col, "plain de "+slug,
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"), ttSeg("verde", "verde", boardcmd.SegFalta, "sin verde"))
	c.Item.Auto = item.AutoHastaHumano
	switch col {
	case boardcmd.ColTuAprobacion:
		c.Actions = []boardcmd.Action{ttAccionAprobar()}
	case boardcmd.ColTuAceptacion:
		c.Actions = []boardcmd.Action{ttAccionIntegrar()}
	case boardcmd.ColHecho:
		c.Actions = []boardcmd.Action{}
	default:
		c.Actions = []boardcmd.Action{tv3cAccionDe(col)}
	}
	return c
}

// tv3cSinAuto es la tarjeta con el piloto apagado.
func tv3cSinAuto(c boardcmd.Card) boardcmd.Card {
	c.Item.Auto = ""
	return c
}

// tv3cConRunning le pone a la tarjeta un trabajo en curso del rol, con el
// sobre env, empezado hace 30 s, con limite de 45:00.
func tv3cConRunning(c boardcmd.Card, env, rol string) boardcmd.Card {
	c.Running = &boardcmd.Running{EnvelopeID: env, RunID: "run-" + env, Role: rol, Provider: "claude",
		Stage: "run", Step: 3, Steps: 6, StartedAt: ttAhora.Add(-30 * time.Second), TimeoutS: 2700,
		LastEventAt: ttT(ttAhora.Add(-2 * time.Second))}
	return c
}

// tv3cConInterrupted le pone a la tarjeta un trabajo cortado del rol.
func tv3cConInterrupted(c boardcmd.Card, rol string) boardcmd.Card {
	c.Interrupted = &boardcmd.Interrupted{EnvelopeID: "env-cortado-" + c.Slug, RunID: "run-cortado-" + c.Slug, Role: rol,
		Provider: "claude", Stage: "run", Step: 3, Steps: 5, UpdatedAt: ttAhora.Add(-time.Hour)}
	c.NeedsDecision = true
	return c
}

// tv3cConRojo le pone a la tarjeta un rojo, esperado o no.
func tv3cConRojo(c boardcmd.Card, esperado bool) boardcmd.Card {
	c.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v-" + c.Slug, Reason: "test: FAIL",
		Plain: "la verificacion de " + c.Slug + " dio rojo", Esperado: esperado}
	return c
}

// tv3cModelo es el modelo de la v3 tras la primera lectura de un tablero con
// esas tarjetas.
func tv3cModelo(c tablero.Config, cards ...boardcmd.Card) *tablero.Modelo {
	return ttModelo(c, ttTablero(cards...))
}

// tv3cLeer lee de nuevo el tablero con esas tarjetas.
func tv3cLeer(m *tablero.Modelo, cards ...boardcmd.Card) []tablero.Orden {
	return m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(cards...)})
}

// tv3cFin termina el proceso id con su codigo y su ultima linea.
func tv3cFin(m *tablero.Modelo, id, code int, ultima string) []tablero.Orden {
	return m.Termino(tablero.FinProceso{ID: id, Code: code, Ultima: ultima}, ttAhora)
}

// tv3cFoto es todo lo que se ve del modelo: donde estan las teclas, el modal,
// el foco, lo elegido, la vista, la caja, el Registro y la pantalla.
func tv3cFoto(m *tablero.Modelo) string {
	var reg []string
	for _, a := range m.Registro() {
		reg = append(reg, a.Texto+" | "+a.Comando)
	}
	ruta, linea := m.ArchivoElegido()
	return fmt.Sprintf("lugar=%s modal=%s foco=%d elegida=%q vista=%s caja=%q corriendo=%d archivo=%s:%d\nregistro=%q\n%s",
		m.Lugar(), m.Modal(), m.Foco(), m.Elegida(), m.Vista(), m.Caja(), m.Corriendo(), ruta, linea, reg, ttPantalla(m))
}

// tv3cAscii escribe las flechas de una Muestra o una Barra como el modo ASCII.
func tv3cAscii(s string) string {
	return strings.NewReplacer("↑", "^", "↓", "v", "←", "<", "→", ">").Replace(s)
}

// tv3cLanzaSeguir abre Seguir solo con P sobre la tarjeta elegida y lo
// confirma con enter: exige el Lanzar del piloto y lo devuelve.
func tv3cLanzaSeguir(t *testing.T, ca string, m *tablero.Modelo, slug string) tablero.Lanzar {
	t.Helper()
	if ords := ttTeclas(m, "P"); len(ords) != 0 {
		t.Fatalf("%s: P abre el modal y no ordena nada, dio %#v", ca, ords)
	}
	if m.Modal() != tablero.ModalSeguir {
		t.Fatalf("%s: P sobre %s abre el modal Seguir solo; modal=%q, mensaje=%q", ca, slug, m.Modal(), ttMensaje(m))
	}
	l := ttUnaLanzar(t, ca, "enter sobre Seguir solo", ttTeclas(m, tablero.TeclaEnter))
	if l.Rol != "piloto" || l.Slug != slug {
		t.Fatalf("%s: el Lanzar de Seguir solo es {Rol: piloto, Slug: %s}, fue %+v", ca, slug, l)
	}
	ttMismosArgs(t, ca, "Seguir solo", l.Args, []string{"piloto", slug})
	return l
}

// tv3cAsiento es la entrada mas nueva del Registro cuyo Texto empieza con
// prefijo.
func tv3cAsiento(t *testing.T, ca string, m *tablero.Modelo, prefijo string) tablero.Asiento {
	t.Helper()
	reg := m.Registro()
	for i := len(reg) - 1; i >= 0; i-- {
		if strings.HasPrefix(reg[i].Texto, prefijo) {
			return reg[i]
		}
	}
	t.Fatalf("%s: el Registro tiene una entrada que empieza con %q: %+v", ca, prefijo, reg)
	return tablero.Asiento{}
}

// tv3cCuentaAsientos cuenta las entradas del Registro que empiezan con prefijo.
func tv3cCuentaAsientos(m *tablero.Modelo, prefijo string) int {
	n := 0
	for _, a := range m.Registro() {
		if strings.HasPrefix(a.Texto, prefijo) {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- geometria

// tv3cAlto es el alto de la pantalla del modelo.
func tv3cAlto(m *tablero.Modelo) int { return len(m.Pantalla()) }

// tv3cInteriorLista son las lineas interiores de la lista con W >= 100 (las
// celdas 1..L-2 de los indices 6..A-5), sin el relleno de la derecha.
func tv3cInteriorLista(m *tablero.Modelo, ancho int) []string {
	ls := ttLimpia(m)
	l := tv3cL(ancho)
	var out []string
	for y := 6; y <= len(ls)-5; y++ {
		out = append(out, strings.TrimRight(ttTramo(ls[y], 1, l-1), " "))
	}
	return out
}

// tv3cInteriorDetalle son las lineas interiores del detalle con W >= 100 (las
// celdas L+1..W-2 de los indices 6..A-5), sin el relleno de la derecha.
func tv3cInteriorDetalle(m *tablero.Modelo, ancho int) []string {
	ls := ttLimpia(m)
	l := tv3cL(ancho)
	var out []string
	for y := 6; y <= len(ls)-5; y++ {
		out = append(out, strings.TrimRight(ttTramo(ls[y], l+1, ancho-1), " "))
	}
	return out
}

// tv3cBordeLista son los bordes de arriba y de abajo de la lista (W >= 100).
func tv3cBordeLista(m *tablero.Modelo, ancho int) (arriba, abajo string) {
	ls := ttLimpia(m)
	l := tv3cL(ancho)
	return ttTramo(ls[5], 0, l), ttTramo(ls[len(ls)-4], 0, l)
}

// tv3cBordeDetalle son los bordes de arriba y de abajo del detalle (W >= 100).
func tv3cBordeDetalle(m *tablero.Modelo, ancho int) (arriba, abajo string) {
	ls := ttLimpia(m)
	l := tv3cL(ancho)
	return ttTramo(ls[5], l, ancho), ttTramo(ls[len(ls)-4], l, ancho)
}

// tv3cFilaY es el indice de pantalla de la primera linea interior de la lista
// (W >= 100) que contiene texto.
func tv3cFilaY(t *testing.T, ca string, m *tablero.Modelo, ancho int, texto string) int {
	t.Helper()
	ls := ttLimpia(m)
	l := tv3cL(ancho)
	for y := 6; y <= len(ls)-5; y++ {
		if strings.Contains(ttTramo(ls[y], 1, l-1), texto) {
			return y
		}
	}
	t.Fatalf("%s: la lista tiene una linea con %q:\n%s", ca, texto, ttPantalla(m))
	return -1
}

// tv3cSegunda es la segunda linea de la fila de la tarjeta de titulo
// (W >= 100): la linea interior de la lista que sigue a la de su titulo.
func tv3cSegunda(t *testing.T, ca string, m *tablero.Modelo, ancho int, titulo string) string {
	t.Helper()
	y := tv3cFilaY(t, ca, m, ancho, titulo)
	return strings.TrimRight(ttTramo(ttLimpia(m)[y+1], 1, tv3cL(ancho)-1), " ")
}

// tv3cSegundaCruda es esa segunda linea con sus SGR (la linea de pantalla
// entera, para mirar el color de un texto).
func tv3cSegundaCruda(t *testing.T, ca string, m *tablero.Modelo, ancho int, titulo string) string {
	t.Helper()
	y := tv3cFilaY(t, ca, m, ancho, titulo)
	return m.Pantalla()[y+1]
}

// tv3cDetalleTiene dice si alguna linea interior del detalle contiene s.
func tv3cDetalleTiene(m *tablero.Modelo, ancho int, s string) bool {
	for _, l := range tv3cInteriorDetalle(m, ancho) {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// tv3cRegistroLinea es la linea A-2 (el Registro plegado), sin SGR.
func tv3cRegistroLinea(m *tablero.Modelo) string {
	ls := ttLimpia(m)
	return ls[len(ls)-3]
}

// tv3cEventos son n eventos de narracion, el ultimo hace 1 s.
func tv3cEventos(n int) []runcmd.Event {
	var out []runcmd.Event
	for i := 0; i < n; i++ {
		out = append(out, ttEvento(-n+i, "tool", fmt.Sprintf("evento-%03d", i)))
	}
	return out
}

// tv3cDetalleCon es el Detalle de la tarjeta c con su narracion.
func tv3cDetalleCon(c boardcmd.Card, eventos []runcmd.Event) tablero.Detalle {
	env, run := "env-"+c.Slug, "run-"+c.Slug
	if c.Running != nil {
		env, run = c.Running.EnvelopeID, c.Running.RunID
	}
	return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c},
		Sobre: tablero.Sobre{EnvelopeID: env, RunID: run, Eventos: eventos}}
}

// tv3cMuchas son n tarjetas en Writer (TE TOCA), con titulos "Tarjeta 01"..
// y planes "plain 01"..: para que la lista no entre.
func tv3cMuchas(n int) []boardcmd.Card {
	var out []boardcmd.Card
	for i := 1; i <= n; i++ {
		c := ttCard(fmt.Sprintf("t%02d", i), fmt.Sprintf("Tarjeta %02d", i), boardcmd.ColWriter, fmt.Sprintf("plain %02d", i),
			ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"))
		c.Actions = []boardcmd.Action{ttAccionWriter()}
		out = append(out, c)
	}
	return out
}

// tv3cSinMensaje exige que la linea de mensajes este vacia.
func tv3cSinMensaje(t *testing.T, ca, que string, m *tablero.Modelo) {
	t.Helper()
	if got := strings.TrimPrefix(ttMensaje(m), "EXPERTO"); strings.TrimSpace(got) != "" {
		t.Fatalf("%s: %s: la linea de mensajes queda vacia, dice %q", ca, que, ttMensaje(m))
	}
}

// tv3cSinModal exige que no haya modal abierto.
func tv3cSinModal(t *testing.T, ca, que string, m *tablero.Modelo) {
	t.Helper()
	if m.Modal() != tablero.SinModal {
		t.Fatalf("%s: %s no abre ningun modal; abrio %q", ca, que, m.Modal())
	}
}

// tv3cSinPanico convierte un panico del modelo en un fallo del test que lo
// tuvo, para que los demas tests sigan corriendo (un panico es un fallo, no
// un permiso para no mirar el resto).
func tv3cSinPanico(t *testing.T) {
	t.Helper()
	if r := recover(); r != nil {
		t.Fatalf("el modelo entro en panico: %v\n%s", r, debug.Stack())
	}
}

// ---------------------------------------------------------------- estados por Lugar

// tv3cHallazgos son tres hallazgos abiertos (high, medium y low en el orden de
// la vista); el high tiene una descripcion de 60 lineas, para que su detalle
// se desplace.
func tv3cHallazgos() tablero.Hallazgos {
	var desc []string
	for i := 0; i < 60; i++ {
		desc = append(desc, fmt.Sprintf("linea %02d de la descripcion", i))
	}
	high := ttHallazgo("h-high", finding.StatusOpen, "high", "internal/api/router.go:41", strings.Join(desc, "\n"))
	med := ttHallazgo("h-med", finding.StatusOpen, "medium", "internal/api/medio.go:3", "un hallazgo medium")
	low := ttHallazgoNuc()
	return tablero.Hallazgos{Items: []finding.Item{low, med, high}}
}

// tv3cBase es el modelo de los dorados con documentos largos: el proyecto con
// 60 problemas, los tres hallazgos de tv3cHallazgos y nuc-tenancy (la
// elegida) con 100 eventos de narracion. Sin running, la narracion solo se
// muestra con filas de trabajo (doc.go, parte G: sin ellas es "  sin sobres
// todavia"), asi que su detalle lleva la fila del sobre env-nuc.
func tv3cBase(c tablero.Config) *tablero.Modelo {
	m := ttModelo(c, ttTableroDorado())
	p := ttProyectoVerde()
	for i := 0; i < 60; i++ {
		p.Problemas = append(p.Problemas, boardcmd.Problem{ID: boardcmd.ProbSpecEditado, Slug: "nuc-tenancy",
			What: fmt.Sprintf("problema %02d", i), Plain: fmt.Sprintf("problema %02d", i), Action: "hoom aprobar nuc-tenancy"})
	}
	m.LeerProyecto(p)
	m.LeerHallazgos(tv3cHallazgos())
	d := ttDetalleNuc()
	ms := int64(61000)
	d.Detalle.Work = []boardcmd.WorkRow{{Kind: boardcmd.WorkSobre, ID: "env-nuc", RunID: "run-nuc", Role: "arquitecto",
		Provider: "claude", Status: "entregable", Stage: "run", Step: 6, Steps: 6, StartedAt: ttAhora.Add(-time.Hour), DurationMS: &ms}}
	d.Sobre = tablero.Sobre{EnvelopeID: "env-nuc", RunID: "run-nuc", Eventos: tv3cEventos(100)}
	m.LeerDetalle(d)
	return m
}

// tv3cLugaresProbados son los Lugares que tv3cEstado sabe armar (todos salvo
// LugarPedirTexto, que necesita un selector con otro…).
var tv3cLugaresProbados = []tablero.Lugar{tablero.LugarTarjetas, tablero.LugarHallazgos,
	tablero.LugarVerProyecto, tablero.LugarVerTarjeta, tablero.LugarVerHallazgo, tablero.LugarVerNada, tablero.LugarBusqueda,
	tablero.LugarPedirSelector, tablero.LugarPedirPedido, tablero.LugarPedirLista, tablero.LugarPedirBotones,
	tablero.LugarFirma, tablero.LugarGuardar, tablero.LugarSeguir, tablero.LugarAcciones, tablero.LugarAyuda, tablero.LugarRegistro}

// tv3cEstado arma un modelo con las teclas en el Lugar l, a 100x30.
func tv3cEstado(t *testing.T, ca string, l tablero.Lugar) *tablero.Modelo {
	t.Helper()
	c := tv3cConfig(100, 30)
	var m *tablero.Modelo
	pedir := func(tabs int) {
		m = tv3cModelo(c, ttTarjetaCon(ttAccionWriter()))
		ttTeclas(m, "w")
		for i := 0; i < tabs; i++ {
			ttTeclas(m, tablero.TeclaTab)
		}
	}
	switch l {
	case tablero.LugarTarjetas:
		m = tv3cBase(c)
	case tablero.LugarHallazgos:
		m = tv3cBase(c)
		ttTeclas(m, "H")
	case tablero.LugarVerProyecto:
		m = tv3cBase(c)
		ttTeclas(m, "v")
	case tablero.LugarVerTarjeta:
		m = tv3cBase(c)
		ttTeclas(m, tablero.TeclaTab)
	case tablero.LugarVerHallazgo:
		m = tv3cBase(c)
		ttTeclas(m, "H", tablero.TeclaEnter)
	case tablero.LugarVerNada:
		m = tv3cBase(c)
		ttTeclas(m, "G", tablero.TeclaTab)
	case tablero.LugarBusqueda:
		m = tv3cBase(c)
		ttTeclas(m, "/")
	case tablero.LugarPedirPedido:
		pedir(0)
	case tablero.LugarPedirSelector:
		pedir(1)
	case tablero.LugarPedirLista:
		pedir(1)
		ttTeclas(m, tablero.TeclaEnter)
	case tablero.LugarPedirBotones:
		pedir(0)
		for i := 0; i < 10 && m.Lugar() != tablero.LugarPedirBotones; i++ {
			ttTeclas(m, tablero.TeclaTab)
		}
	case tablero.LugarFirma:
		m = tv3cBase(c)
		ttTeclas(m, "A")
	case tablero.LugarGuardar:
		m = tv3cModelo(c, ttTarjetaCon(ttAccionGuardar(true, "", "a.go", "b.go")))
		ttTeclas(m, "s")
	case tablero.LugarSeguir:
		m = tv3cModelo(c, tv3cSola("sola", "Sola", boardcmd.ColWriter))
		ttTeclas(m, "P")
	case tablero.LugarAcciones:
		m = tv3cModelo(c, ttTarjetaCon(ttAccionWriter(), ttAccionAprobar(), ttAccionIntegrar(), ttAccionGuardar(true, "", "a.go")))
		ttTeclas(m, tablero.TeclaEspacio)
	case tablero.LugarAyuda:
		m = tv3cBase(c)
		ttTeclas(m, "?")
	case tablero.LugarRegistro:
		// 20 entradas (40 lineas, mas de las que entran): cada u termina
		// antes de la siguiente, para que ninguna espere a la anterior
		m = tv3cBase(c)
		for i := 0; i < 20; i++ {
			for _, l := range ttLanzares(ttTeclas(m, "u")) {
				tv3cFin(m, l.ID, 0, "")
			}
		}
		ttTeclas(m, "L")
	default:
		t.Fatalf("%s: tv3cEstado no sabe armar el Lugar %q", ca, l)
	}
	if m.Lugar() != l {
		t.Fatalf("%s: precondicion del spec: con las teclas del spec el estado %q tiene las teclas en %q; estan en %q (modal %q, foco %d):\n%s", ca, l, l, m.Lugar(), m.Modal(), m.Foco(), ttPantalla(m))
	}
	return m
}

// tv3cApretar aprieta una tecla de la tabla: "]c" son dos teclas y
// TeclaTexto es una letra cualquiera.
func tv3cApretar(m *tablero.Modelo, k tablero.Tecla) []tablero.Orden {
	switch k {
	case "]c", "[c":
		return append(m.Tecla(tablero.Tecla(string(k[0])), ttAhora), m.Tecla("c", ttAhora)...)
	case tablero.TeclaTexto:
		return m.Tecla("z", ttAhora)
	}
	return m.Tecla(k, ttAhora)
}
