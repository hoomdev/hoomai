package tablero

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

func wrLanzares(ords []Orden) []Lanzar {
	var out []Lanzar
	for _, o := range ords {
		if l, ok := o.(Lanzar); ok {
			out = append(out, l)
		}
	}
	return out
}

func wrContiene(t *testing.T, m *Modelo, s string) {
	t.Helper()
	if !strings.Contains(wrPantalla(m), s) {
		t.Fatalf("la pantalla no dice %q:\n%s", s, wrPantalla(m))
	}
}

func TestWRFocoYTab(t *testing.T) {
	m := wrModelo(100, 25, true)
	if m.Foco() != PanelTarjetas || m.Lugar() != LugarTarjetas {
		t.Fatalf("al abrir el foco esta en [2]: %v %v", m.Foco(), m.Lugar())
	}
	orden := []Panel{PanelCorriendo, PanelHallazgos, PanelPrincipal, PanelProyecto, PanelTarjetas}
	for _, p := range orden {
		wrTeclas(m, TeclaTab)
		if m.Foco() != p {
			t.Fatalf("tab lleva a %v, esta en %v", p, m.Foco())
		}
	}
	wrTeclas(m, TeclaShiftTab)
	if m.Foco() != PanelProyecto {
		t.Fatalf("shift-tab vuelve a [1]: %v", m.Foco())
	}
	wrTeclas(m, "4")
	if r, _ := m.ArchivoElegido(); m.Foco() != PanelHallazgos || r == "" {
		r, l := m.ArchivoElegido()
		t.Fatalf("4 da el foco a [4] y [0] muestra su hallazgo: %v %q %d", m.Foco(), r, l)
	}
	wrForma(t, m)
}

func TestWRSeleccionYPrincipal(t *testing.T) {
	m := wrModelo(100, 25, true)
	if m.Elegida() != "nuc-tenancy" {
		t.Fatalf("[0] muestra la primera tarjeta: %q", m.Elegida())
	}
	wrTeclas(m, "j")
	if m.Elegida() != "infra-api-convenciones" {
		t.Fatalf("j mueve la seleccion y [0] la sigue: %q", m.Elegida())
	}
	wrContiene(t, m, "[0] infra-api-convenciones · Convenciones de la API")
	wrTeclas(m, TeclaEnter)
	if m.Foco() != PanelPrincipal || m.Lugar() != LugarVerTarjeta {
		t.Fatalf("enter pasa el foco a [0]: %v %v", m.Foco(), m.Lugar())
	}
	wrTeclas(m, "h")
	if m.Foco() != PanelTarjetas {
		t.Fatalf("h vuelve a [2]: %v", m.Foco())
	}
	wrTeclas(m, "G")
	wrTeclas(m, TeclaEnter)
	wrContiene(t, m, "HECHO 3 ▾")
	wrTeclas(m, "j")
	if m.Elegida() != "viejo-1" {
		t.Fatalf("Hecho abierto muestra sus tarjetas: %q", m.Elegida())
	}
}

func TestWRBusqueda(t *testing.T) {
	m := wrModelo(100, 25, true)
	wrTeclas(m, "3", "/", "o", "b", "s")
	if m.Lugar() != LugarBusqueda || m.Foco() != PanelTarjetas {
		t.Fatalf("/ abre la busqueda en [2]: %v %v", m.Lugar(), m.Foco())
	}
	if p := m.Pantalla(); p[len(p)-2] != " /obs▌" {
		t.Fatalf("la linea de mensajes muestra la busqueda: %q", p[len(p)-2])
	}
	if m.Elegida() != "infra-observabilidad" {
		t.Fatalf("filtra y elige: %q", m.Elegida())
	}
	wrTeclas(m, TeclaEsc)
	if m.Lugar() != LugarTarjetas || m.Elegida() != "infra-observabilidad" {
		t.Fatalf("esc cierra y la tarjeta sigue elegida: %v %q", m.Lugar(), m.Elegida())
	}
}

func TestWRAccionesAbrenModal(t *testing.T) {
	m := wrModelo(100, 25, true)
	wrTeclas(m, "j")
	if ords := wrTeclas(m, "w"); len(ords) != 0 || m.Modal() != ModalPedir {
		t.Fatalf("w abre el modal de pedir sin ordenes: %v %v", ords, m.Modal())
	}
	wrTeclas(m, TeclaEsc, "n")
	wrContiene(t, m, "el writer esta trabajando: espera a que termine")
	wrTeclas(m, "k", "n")
	if m.Modal() != ModalFirma {
		t.Fatalf("n abre la firma de Tu aprobacion: %v", m.Modal())
	}
	wrTeclas(m, TeclaEsc)
	wrTeclas(m, "k", "A")
	if m.Modal() != ModalFirma {
		t.Fatalf("A abre la firma: %v", m.Modal())
	}
	if ords := wrTeclas(m, TeclaEnter); len(ords) != 0 || m.Modal() != SinModal {
		t.Fatalf("enter sobre Cancelar cierra sin ejecutar: %v", ords)
	}
	wrTeclas(m, "A", TeclaDer)
	ls := wrLanzares(wrTeclas(m, TeclaEnter))
	if len(ls) != 1 || strings.Join(ls[0].Args, " ") != "aprobar nuc-tenancy --si" {
		t.Fatalf("enter sobre Aprobar spec ejecuta: %v", ls)
	}
	if r := m.Registro(); len(r) != 1 || r[0].Comando != "hoom aprobar nuc-tenancy --si" ||
		r[0].Texto != "Aprobar spec · nuc-tenancy · lanzando…" {
		t.Fatalf("el Registro suma la entrada: %+v", r)
	}
	m.Termino(FinProceso{ID: ls[0].ID, Code: 1, Ultima: "no se pudo"}, wrAhora)
	if r := m.Registro(); r[0].Texto != "Aprobar spec · nuc-tenancy · fallo: no se pudo" {
		t.Fatalf("fallo: %+v", r)
	}
	ls = wrLanzares(wrTeclas(m, "u"))
	if len(ls) != 1 || strings.Join(ls[0].Args, " ") != "usar nuc-tenancy" || m.Modal() != SinModal {
		t.Fatalf("u ejecuta hoom usar sin modal: %v", ls)
	}
}

func TestWRPedir(t *testing.T) {
	m := wrModelo(100, 25, true)
	wrTeclas(m, "j", "w")
	if m.Lugar() != LugarPedirPedido {
		t.Fatalf("el foco empieza en el Pedido: %v", m.Lugar())
	}
	wrContiene(t, m, "Modelo         ‹ sonnet ›")
	wrContiene(t, m, "de hoom.yaml")
	// codex: the hoom.yaml model stays, with its note
	wrTeclas(m, TeclaTab, TeclaDer)
	wrContiene(t, m, "‹ codex ›")
	wrContiene(t, m, "sonnet no esta en la lista de codex: puede rechaza")
	// gemini: no model, no effort
	wrTeclas(m, TeclaDer)
	wrContiene(t, m, "hoom no le pasa el modelo a gemini")
	wrContiene(t, m, "gemini no acepta tope")
	wrTeclas(m, TeclaIzq, TeclaIzq)
	wrContiene(t, m, "‹ claude ›")
	// Tope: 10 USD
	wrTeclas(m, TeclaTab, TeclaTab, TeclaTab, TeclaDer, TeclaDer, TeclaDer)
	wrContiene(t, m, "‹ 10 USD ›")
	wrTeclas(m, TeclaTab, TeclaTab, TeclaTab)
	if m.Lugar() != LugarPedirPedido {
		t.Fatalf("tab da la vuelta al Pedido: %v", m.Lugar())
	}
	ls := wrLanzares(m.Tecla(TeclaEnter, wrAhora))
	if len(ls) != 0 {
		t.Fatalf("enter en el Pedido lleva al boton: %v", ls)
	}
	ls = wrLanzares(m.Tecla(TeclaEnter, wrAhora))
	want := "pedir writer infra-api-convenciones -m Implementa .hoom/specs/infra-api-convenciones.md hasta que hoom verify quede VERDE --budget-usd 10"
	if len(ls) != 1 || strings.Join(ls[0].Args, " ") != want || ls[0].Rol != "writer" {
		t.Fatalf("lanza con los flags cambiados: %v", ls)
	}
}

func TestWRPedidoVacioYOtro(t *testing.T) {
	m := wrModelo(100, 25, true)
	wrTeclas(m, "j", "w", TeclaCtrlU, TeclaEnter)
	if ords := wrTeclas(m, TeclaEnter); len(ords) != 0 || m.Lugar() != LugarPedirPedido {
		t.Fatalf("un pedido vacio no lanza: %v %v", ords, m.Lugar())
	}
	wrContiene(t, m, "el pedido esta vacio")
	wrTeclas(m, "x", TeclaTab, TeclaTab, TeclaEnter)
	if m.Lugar() != LugarPedirLista {
		t.Fatalf("enter abre la lista: %v", m.Lugar())
	}
	wrTeclas(m, "G")
	for i := 0; i < 20; i++ {
		wrTeclas(m, TeclaAbajo)
	}
	wrTeclas(m, TeclaEnter)
	if m.Lugar() != LugarPedirTexto {
		t.Fatalf("otro… convierte el campo en texto: %v", m.Lugar())
	}
	wrTeclas(m, "g", "p", "t", "-", "7", TeclaEnter)
	wrContiene(t, m, "‹ gpt-7 ›")
	wrContiene(t, m, "--model gpt-7")
}

func TestWRMouse(t *testing.T) {
	es := DecodificarEntradas([]byte("a\x1b[<0;5;9M\x1b[<0;5;9m\x1b[<65;3;4M\x1b[5~\x1b[Mabc"))
	if len(es) != 4 || es[0].Tecla != "a" || es[1].Raton == nil || *es[1].Raton != (Raton{RatonClic, 4, 8}) ||
		*es[2].Raton != (Raton{RatonRuedaAbajo, 2, 3}) || es[3].Tecla != TeclaPgArriba {
		t.Fatalf("decodifica: %+v", es)
	}
	m := wrModelo(100, 25, true)
	// a click on [4]'s second row
	c := m.caja(PanelHallazgos)
	if ords := m.Raton(Raton{RatonClic, 5, c.r.y + 2}, wrAhora); len(ords) != 0 {
		t.Fatalf("un clic no da ordenes: %v", ords)
	}
	if h, _ := m.hallazgoElegido(); m.Foco() != PanelHallazgos || h.ID != "f-low" {
		t.Fatalf("el clic da el foco y elige la fila: %v %q", m.Foco(), h.ID)
	}
	wrTeclas(m, "2", "j", "w")
	g, _ := m.geometria()
	for i, l := range g.lineas {
		if l.botones {
			m.Raton(Raton{RatonClic, g.r.x + 4 + 40, g.r.y + 1 + i}, wrAhora)
		}
	}
	if m.pedir.boton != 1 {
		t.Fatalf("el clic elige Cancelar: %d", m.pedir.boton)
	}
	wrContiene(t, m, "enter para cancelar")
}

func TestWRAnchos(t *testing.T) {
	for r, w := range map[rune]int{'a': 1, '漢': 2, '🚀': 2, '✋': 2, '\u0301': 0, '●': 1, '═': 1} {
		if AnchoDe(r) != w {
			t.Fatalf("AnchoDe(%q) = %d, es %d", r, AnchoDe(r), w)
		}
	}
	m := Nuevo(Config{Proyecto: "p", Ancho: 100, Alto: 25, UTF8: true})
	c := wrCard("emoji", "漢字 y 🚀 en el titulo", boardcmd.ColBacklog, "plain 漢字")
	m.Leer(Lectura{Ahora: wrAhora, Tablero: wrTablero(c)})
	wrForma(t, m)
	for _, l := range m.Pantalla()[:23] {
		if Celdas(l) != 100 {
			t.Fatalf("una linea de paneles ocupa 100 celdas: %d %q", Celdas(l), l)
		}
	}
}

func TestWRUnaColumnaYPequena(t *testing.T) {
	for _, tam := range [][2]int{{60, 15}, {79, 20}, {80, 24}, {120, 32}, {80, 15}} {
		for _, k := range []Tecla{"1", "2", "3", "4", "0"} {
			m := wrModelo(tam[0], tam[1], false)
			wrTeclas(m, k)
			wrForma(t, m)
		}
	}
	m := wrModelo(59, 15, true)
	if p := m.Pantalla(); p[0] != "agranda la terminal (minimo 60x15)" || p[1] != "" {
		t.Fatalf("debajo de 60x15: %q", p)
	}
}

func TestWRColor(t *testing.T) {
	m := Nuevo(Config{Proyecto: "p", Ancho: 100, Alto: 25, UTF8: true, Color: true})
	m.Leer(Lectura{Ahora: wrAhora, Tablero: wrTableroPrincipal()})
	p := strings.Join(m.Pantalla(), "\n")
	for _, s := range []string{"\x1b[1;32m╔[2] Tarjetas", "\x1b[7m ▸ nuc-tenancy", "\x1b[33mNECESITA", "\x1b[36mEN CURSO"} {
		if !strings.Contains(p, s) {
			t.Fatalf("con color esta %q:\n%q", s, p)
		}
	}
}
