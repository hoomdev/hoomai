package tablero

import (
	"strings"
	"testing"
)

// Pruebas propias del writer que miran estado interno del modelo (la
// geometria de las zonas y del modal); el comportamiento por la API
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
	// la vista Hallazgos: un clic en su segunda fila
	wrTeclas(m, "H", TeclaTab)
	l := m.disponer().lista
	if ords := m.Raton(Raton{RatonClic, 5, l.y + 2}, wrAhora); len(ords) != 0 {
		t.Fatalf("un clic no da ordenes: %v", ords)
	}
	if h, _ := m.hallazgoElegido(); m.Foco() != ZonaLista || h.ID != "f-low" {
		t.Fatalf("el clic da el foco y elige la fila: %v %q", m.Foco(), h.ID)
	}
	// de vuelta a las tarjetas: el writer que corre (CORRIENDO, segunda fila)
	wrTeclas(m, "H", "j", "w")
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
