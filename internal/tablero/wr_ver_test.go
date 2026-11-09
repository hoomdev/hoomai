package tablero

import (
	"os"
	"testing"
)

// TestWRVer dumps screens to look at them (WR_VER=1).
func TestWRVer(t *testing.T) {
	if os.Getenv("WR_VER") == "" {
		t.Skip("WR_VER=1 para ver las pantallas")
	}
	for _, tam := range [][2]int{{100, 25}, {120, 32}, {80, 24}, {60, 15}} {
		m := wrModelo(tam[0], tam[1], true)
		t.Logf("%dx%d\n%s", tam[0], tam[1], wrPantalla(m))
	}
	m := wrModelo(100, 25, true)
	wrTeclas(m, "j")
	wrTeclas(m, "w")
	t.Logf("modal pedir\n%s", wrPantalla(m))
	wrTeclas(m, TeclaTab, TeclaTab, TeclaEnter)
	t.Logf("lista de modelos\n%s", wrPantalla(m))
	m = wrModelo(60, 15, false)
	wrTeclas(m, "j", "w")
	t.Logf("modal pedir 60x15 ascii\n%s", wrPantalla(m))
	m = wrModelo(100, 25, true)
	wrTeclas(m, "?")
	t.Logf("ayuda\n%s", wrPantalla(m))
	m = wrModelo(100, 25, true)
	wrTeclas(m, "4", TeclaEnter)
	t.Logf("hallazgo\n%s", wrPantalla(m))
	m = wrModelo(100, 25, true)
	wrTeclas(m, "1", TeclaEnter)
	t.Logf("proyecto\n%s", wrPantalla(m))
	m = wrModelo(100, 25, true)
	wrTeclas(m, "A")
	t.Logf("firma\n%s", wrPantalla(m))
	m = wrModelo(100, 25, true)
	wrTeclas(m, " ")
	t.Logf("acciones\n%s", wrPantalla(m))
}
