// Archivos dorados (CA-518, CA-519, CA-521, CA-522, CA-526, CA-527, CA-529,
// CA-531, CA-533, CA-534, CA-535): la pantalla entera, comparada linea por
// linea con testdata/<nombre>.txt, escrito a mano desde el contrato de
// doc.go (sin color; en ASCII y una vez con los signos UTF-8). No hay flag
// -update: un dorado regenerado desde el codigo seria circular.
package tablero_test

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

// ttSpecMD es el spec de infra-api: CA-1 con prueba, CA-2 y CA-4 sin prueba,
// CA-3 verificado por comando, y una linea "- CA-9:" que no es criterio de
// la tarjeta (lleva dos espacios de margen).
const ttSpecMD = "# Spec: convenciones de la API\n\n## Criterios de aceptacion\n\n" +
	"- CA-1: la API usa snake_case.\n- CA-2: los errores son JSON.\n" +
	"- CA-3: la version va en la ruta. [verifica: go run ./cmd/check]\n" +
	"  - CA-9: no es criterio de la tarjeta.\n- CA-4: los ids son ulid.\n\nFin.\n"

func ttCriterios() []boardcmd.CriterionTrace {
	return []boardcmd.CriterionTrace{
		{ID: "CA-1", Text: "la API usa snake_case.", TracedBy: boardcmd.TracedTest, Files: []string{"api_test.go"}},
		{ID: "CA-2", Text: "los errores son JSON."},
		{ID: "CA-3", Text: "la version va en la ruta.", TracedBy: boardcmd.TracedComando},
		{ID: "CA-4", Text: "los ids son ulid."},
	}
}

// ttDetalleInfraAPI es el detalle de infra-api con su spec.
func ttDetalleInfraAPI(c boardcmd.Card) tablero.Detalle {
	return tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{
		Card:     c,
		Spec:     boardcmd.SpecDetail{Path: ".hoom/specs/infra-api.md", Exists: true, Markdown: ttSpecMD},
		Criteria: ttCriterios(),
	}}
}

// ttAbrirInfraAPI abre infra-api desde la lista principal y le da su detalle.
func ttAbrirInfraAPI(t *testing.T, c tablero.Config) *tablero.Modelo {
	t.Helper()
	m := ttModelo(c, ttTableroPrincipal())
	ttIrAUTF8(t, m, "infra-api")
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if v := m.Vista(); v != tablero.VistaResumen {
		t.Fatalf("CA-520: enter abre la tarjeta en Resumen, la vista es %q", v)
	}
	m.LeerDetalle(ttDetalleInfraAPI(ttInfraAPI(true)))
	return m
}

// ttIrAUTF8 es ttIrA para cualquier juego de signos: la fila seleccionada
// empieza con " > " o con " ▸ ".
func ttIrAUTF8(t *testing.T, m *tablero.Modelo, slug string) {
	t.Helper()
	for i := 0; i < 40; i++ {
		for _, l := range ttCuerpo(m) {
			if strings.HasPrefix(l, " > "+slug+" ") || strings.HasPrefix(l, " ▸ "+slug+" ") {
				return
			}
		}
		m.Tecla("j", ttAhora)
	}
	t.Fatalf("no se llego a seleccionar %s con j; pantalla:\n%s", slug, ttPantalla(m))
}

// CA-518, CA-534: la lista en ASCII sin color a 80x24. Secciones en el orden
// del contrato, cada tarjeta una sola vez, sin las secciones vacias (TU
// APROBACION y WRITER quedan vacias: sus tarjetas ya salieron arriba) salvo
// HECHO cerrada con su cantidad, la tira con ! en el primer segmento que
// falta de las que esperan tu decision, la segunda linea de la interrumpida
// y de la que corre, y los cortes con "~".
func TestCA518_CA534_ListaASCII80x24Dorado(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTableroPrincipal())
	ttForma(t, "CA-518", m, 80, 24, false)
	ttDorado(t, "CA-518", "lista-ascii-80x24.txt", m, 1)
}

// CA-518, CA-534: el mismo tablero con los signos UTF-8 (● ○ · ✋ ▸ ─ …),
// "NECESITA TU DECISIÓN" y "narró".
func TestCA518_CA534_ListaUTF8Dorado(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, true), ttTableroPrincipal())
	ttForma(t, "CA-534", m, 80, 24, false)
	ttDorado(t, "CA-534", "lista-utf8-80x24.txt", m, 1)
}

// CA-519: a 100 columnas la segunda linea de la tarjeta en curso entra
// entera: rol, paso, tiempo corrido de su tiempo limite, narro hace <n>s y
// gasto.
func TestCA519_ListaASCII100x24SegundaLineaEnteraDorado(t *testing.T) {
	m := ttModelo(ttConfig(100, 24, false), ttTableroPrincipal())
	ttForma(t, "CA-519", m, 100, 24, false)
	ttDorado(t, "CA-519", "lista-ascii-100x24.txt", m, 1)
}

// CA-531: en modo experto la barra dice EXPERTO y la tira muestra el detalle
// de cada segmento ("<signo> <label>: <detalle>"), sin plain.
func TestCA531_ListaExpertaDorado(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTablero(ttInfraAPI(true), ttBacklog("docs-guia", "Guia"), ttPrecios()))
	ttSinOrdenes(t, "CA-531", "x", ttTeclas(m, "x"))
	ttForma(t, "CA-531", m, 80, 24, false)
	ttDorado(t, "CA-531", "lista-experta-ascii-80x24.txt", m, 1)
	// x otra vez vuelve al modo normal
	m.Tecla("x", ttAhora)
	if b := ttBarra1(m); !strings.HasPrefix(b, " NORMAL · ") {
		t.Fatalf("CA-531: x alterna el modo experto: la segunda x vuelve a NORMAL, barra 1 = %q", b)
	}
}

// CA-521: la tarjeta abierta en Resumen: las cinco pestanas con la activa
// entre corchetes, columna, plain, lo que falta, la tira con el detalle de
// cada segmento, el gasto contra el presupuesto, quien escribio y reviso, el
// motivo del rojo y los problemas del doctor.
func TestCA521_ResumenDorado(t *testing.T) {
	m := ttAbrirInfraAPI(t, ttConfig(80, 24, false))
	ttForma(t, "CA-521", m, 80, 24, false)
	ttDorado(t, "CA-521", "resumen-ascii-80x24.txt", m, 1)
}

// CA-522, CA-534: la pestana Spec con los signos UTF-8 en el margen: ✓ con
// prueba, ✗ sin prueba (untraced), ⚑ por comando, y dos espacios en toda otra
// linea (tambien en "- CA-9:", que no es criterio de la tarjeta).
func TestCA522_CA534_SpecUTF8Dorado(t *testing.T) {
	m := ttAbrirInfraAPI(t, ttConfig(80, 24, true))
	m.Tecla("2", ttAhora)
	if v := m.Vista(); v != tablero.VistaSpec {
		t.Fatalf("CA-521: 2 elige la pestana Spec, la vista es %q", v)
	}
	m.LeerDetalle(ttDetalleInfraAPI(ttInfraAPI(true)))
	ttForma(t, "CA-522", m, 80, 24, false)
	ttDorado(t, "CA-522", "spec-utf8-80x24.txt", m, 1)
}

// CA-527: el formulario de pedir-writer sobre la lista: provider por
// defecto, modelo, esfuerzo, presupuesto, tiempo limite (valores.Duracion),
// el foco en Pedido con el cursor al final y sus ultimas 4 lineas, la linea
// -> con el verbo corto (sin flags: nada cambio) cortada a la caja.
func TestCA527_FormularioWriterDorado(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTablero(ttInfraAPI(false)))
	ttSinOrdenes(t, "CA-527", "w (abre el formulario)", ttTeclas(m, "w"))
	if v := m.Vista(); v != tablero.VistaFormulario {
		t.Fatalf("CA-525: w con pedir-writer habilitada abre el formulario, la vista es %q", v)
	}
	ttForma(t, "CA-527", m, 80, 24, false)
	ttDorado(t, "CA-527", "formulario-writer-ascii-80x24.txt", m, 1)
}

// CA-529: la confirmacion de A con el texto de la firma de la capa 1
// (tarjetacmd.TextoFirma) envuelto en las palabras y Cancelar elegido.
func TestCA529_ConfirmacionAprobarDorado(t *testing.T) {
	c := ttNucTenancy()
	a, _ := c.Action(boardcmd.ActAprobar)
	// guarda: el dorado se escribio con este texto de la capa 1
	quiere := `Vas a aprobar el spec de "Tenancy del nucleo" (5 criterios) con tu identidad: Henry Orellana <henry@hoom.dev>. Si el spec cambia despues, la aprobacion deja de valer.`
	if got := tarjetacmd.TextoFirma(c, a); got != quiere {
		t.Fatalf("CA-529 (guarda): TextoFirma cambio y el dorado ya no es el de la capa 1:\n got %q\nwant %q", got, quiere)
	}
	m := ttModelo(ttConfig(80, 24, false), ttTablero(c))
	ttSinOrdenes(t, "CA-529", "A (abre la confirmacion)", ttTeclas(m, "A"))
	if v := m.Vista(); v != tablero.VistaConfirmacion {
		t.Fatalf("CA-529: A abre la confirmacion, la vista es %q", v)
	}
	ttForma(t, "CA-529", m, 80, 24, false)
	ttDorado(t, "CA-529", "confirmacion-aprobar-ascii-80x24.txt", m, 1)
}

// CA-526: el menu de acciones: las actions en su orden con su tecla y su
// verbo, la deshabilitada con su why, la que no tiene verbo de la capa 1 con
// "sin verbo en la terminal" y la experta escondida. El titulo de la pantalla
// del menu no lo fija el contrato: el dorado trae las lineas 2..A.
func TestCA526_MenuDorado(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTablero(ttInfraAPI(false)))
	ttSinOrdenes(t, "CA-526", "espacio", ttTeclas(m, tablero.TeclaEspacio))
	if v := m.Vista(); v != tablero.VistaMenu {
		t.Fatalf("CA-526: espacio abre el menu, la vista es %q", v)
	}
	ttForma(t, "CA-526", m, 80, 24, false)
	ttDorado(t, "CA-526", "menu-ascii-80x24-sin-titulo.txt", m, 2)
}

// CA-533: por debajo de 60x15 la pantalla es solo "agranda la terminal
// (minimo 60x15)" y Alto-1 lineas vacias; a 60x15 ya es el marco de siempre.
func TestCA533_PantallaChicaDorado(t *testing.T) {
	m := ttModelo(ttConfig(59, 15, false), ttTableroPrincipal())
	ttDorado(t, "CA-533", "chica-59x15.txt", m, 1)
	m = ttModelo(ttConfig(50, 10, true), ttTableroPrincipal())
	ttDorado(t, "CA-533", "chica-50x10.txt", m, 1)
	m = ttModelo(ttConfig(60, 14, false), ttTableroPrincipal())
	if got := ttLinea(m, 1); got != "agranda la terminal (minimo 60x15)" || len(m.Pantalla()) != 14 {
		t.Fatalf("CA-533: a 60x14 tambien es la pantalla chica (14 lineas): %q", m.Pantalla())
	}
	for i, l := range m.Pantalla()[1:] {
		if l != "" {
			t.Fatalf("CA-533: la pantalla chica tiene solo la primera linea; la %d es %q", i+2, l)
		}
	}
	m = ttModelo(ttConfig(60, 15, false), ttTableroPrincipal())
	ttForma(t, "CA-533", m, 60, 15, false)
	if got := ttLinea(m, 1); got != " hoom · demo · 9 tarjetas" {
		t.Fatalf("CA-533: a 60x15 la pantalla tiene su marco (titulo), linea 1 = %q", got)
	}
	if got := ttLinea(m, 13); got != strings.Repeat("-", 60) {
		t.Fatalf("CA-533: a 60x15 la linea A-2 es el separador de 60 '-', es %q", got)
	}
}

// CA-533: el modelo se redibuja con el tamano nuevo (Tamano), en los dos
// sentidos.
func TestCA533_TamanoRedibuja(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTableroPrincipal())
	m.Tamano(50, 10)
	ttDorado(t, "CA-533", "chica-50x10.txt", m, 1)
	m.Tamano(100, 24)
	ttDorado(t, "CA-533", "lista-ascii-100x24.txt", m, 1)
	m.Tamano(80, 24)
	ttDorado(t, "CA-533", "lista-ascii-80x24.txt", m, 1)
}

// CA-535: un proyecto sin items: el titulo con 0 tarjetas y, en la primera
// linea del cuerpo, ` no hay tarjetas: hoom item add "<titulo>"`. El
// contrato no dice si la linea de Hecho (siempre visible) la sigue: se
// acepta solo el mensaje o el mensaje y " HECHO 0 >" (ver el informe).
func TestCA535_ProyectoVacio(t *testing.T) {
	m := ttModelo(ttConfig(80, 24, false), ttTablero())
	ttForma(t, "CA-535", m, 80, 24, false)
	p := m.Pantalla()
	if p[0] != " hoom · demo · 0 tarjetas" {
		t.Fatalf("CA-535: el titulo del proyecto vacio es %q", p[0])
	}
	if p[1] != ` no hay tarjetas: hoom item add "<titulo>"` {
		t.Fatalf("CA-535: la primera linea del cuerpo es el mensaje del contrato, es %q", p[1])
	}
	resto := p[2:21]
	if resto[0] == " HECHO 0 >" {
		resto = resto[1:]
	}
	for i, l := range resto {
		if l != "" {
			t.Fatalf("CA-535: el proyecto vacio no muestra nada mas en el cuerpo; linea %q (%d)", l, i)
		}
	}
	if p[21] != strings.Repeat("-", 80) || p[22] != " NORMAL · sin tarjeta fijada" ||
		p[23] != " j/k moverse · enter abrir · espacio acciones · n siguiente · / buscar · ? ayuda" {
		t.Fatalf("CA-535: el marco del proyecto vacio es el de la lista: %q", p[21:])
	}
}

// Antes de la primera lectura, el cuerpo dice " leyendo el tablero...".
func TestCA518_AntesDeLaPrimeraLectura(t *testing.T) {
	m := tablero.Nuevo(ttConfig(80, 24, false))
	if len(m.Pantalla()) != 24 || ttLinea(m, 2) != " leyendo el tablero..." {
		t.Fatalf("CA-518: antes de leer el tablero el cuerpo dice ' leyendo el tablero...': %q", m.Pantalla())
	}
}
