package tablero

import (
	"strings"
	"testing"
)

// Pruebas propias del writer que miran estado interno del modelo (la
// geometria de los paneles y del modal); el comportamiento por la API
// publica lo prueban los tt_* del test-writer.

func wrContiene(t *testing.T, m *Modelo, s string) {
	t.Helper()
	if !strings.Contains(wrPantalla(m), s) {
		t.Fatalf("la pantalla no dice %q:\n%s", s, wrPantalla(m))
	}
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
