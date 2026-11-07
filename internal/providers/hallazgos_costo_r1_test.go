// Tests del writer para los hallazgos de la review 1 de costo-por-spec
// (registro 20261107T163525_52e14a) en la medicion de los providers.
package providers

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// Hallazgos acae9a y 0b28bb — CA-540/CA-541: un porcentaje fuera de rango
// no es una ventana: esa muestra queda sin dato.
func TestHallazgo_acae9a_0b28bb_CA540_UtilizationFueraDeRangoNoEsVentana(t *testing.T) {
	for _, u := range []float64{-0.1, 1.5, 200} {
		m := cpsMedidor(t, "claude").MedirLinea(cpsRateLimit(u, 1794078000, u, 1794517200))
		if !cpsSinVentanas(m) {
			t.Fatalf("utilization %v no da ventana: %+v", u, m)
		}
	}
	m := cpsMedidor(t, "claude").MedirLinea(cpsRateLimit(1, 1794078000, 0, 1794517200))
	if v, ok := cpsVentana(m.Muestras[0], "5h"); !ok || !cpsCasi(v.Usado, 100) {
		t.Fatalf("los bordes 0 y 1 valen: %+v", m)
	}
}

func TestHallazgo_acae9a_0b28bb_CA541_UsedPercentFueraDeRangoNoEsVentana(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t0 := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	cpsEscribirSesion(t, home, t0, cpsThread,
		cpsSessionMeta(t0, cpsThread),
		cpsTokenCount(t0.Add(time.Second), &cpsRL{used: 250, minutes: 300, resets: 1794053319}, &cpsRL{used: -3, minutes: 10080, resets: 1794549884}, "plus"),
		cpsTokenCount(t0.Add(2*time.Second), &cpsRL{used: 40, minutes: 300, resets: 1794053319}, nil, "plus"))
	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, t0)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Muestras) != 1 || len(m.Muestras[0]) != 1 || !cpsCasi(m.Muestras[0][0].Usado, 40) {
		t.Fatalf("solo la muestra en rango cuenta: %+v", m.Muestras)
	}
}

// Hallazgo e81263 — CA-541: la lectura de la sesion tiene topes; pasarlos
// es "sin dato" (error y Medicion vacia), nunca una espera sin fin.
func TestHallazgo_e81263_CA541_LaLecturaDeLaSesionTieneTopes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CODEX_HOME", home)
	t0 := time.Date(2026, 11, 7, 10, 0, 0, 0, time.UTC)
	lineas := []string{cpsSessionMeta(t0, cpsThread)}
	for i := 0; i < 200; i++ {
		lineas = append(lineas, cpsTokenCount(t0.Add(time.Duration(i)*time.Second), &cpsRL{used: 10, minutes: 300, resets: 1794053319}, nil, "plus"))
	}
	cpsEscribirSesion(t, home, t0, cpsThread, lineas...)

	viejo := limitesSesion
	t.Cleanup(func() { limitesSesion = viejo })

	limitesSesion.bytes = 4 << 10
	m, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, t0)
	if !errors.Is(err, errTopeSesion) || !cpsSinVentanas(m) {
		t.Fatalf("pasar el tope de bytes da sin dato: %v %+v", err, m)
	}

	limitesSesion = viejo
	limitesSesion.plazo = -time.Second
	if _, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, t0); !errors.Is(err, errTopeSesion) {
		t.Fatalf("pasar el plazo da sin dato: %v", err)
	}

	limitesSesion = viejo
	limitesSesion.archivos = 1
	cpsEscribirSesion(t, home, t0.Add(time.Hour), cpsThread, cpsSessionMeta(t0, cpsThread))
	if _, err := cpsMedidor(t, "codex").MedirSesion(cpsThread, t0); !errors.Is(err, errTopeSesion) {
		t.Fatalf("mas archivos que el tope da sin dato: %v", err)
	}
	if !strings.Contains(errTopeSesion.Error(), "sin dato") {
		t.Fatalf("el error lo dice: %v", errTopeSesion)
	}
}

// Hallazgo 1f72e7 — CA-544: la contabilidad de tokens nuevos vive con los
// providers y es exhaustiva: uno que hoom no conoce no recibe la de otro.
func TestHallazgo_1f72e7_CA544_EntradaNueva(t *testing.T) {
	u := Usage{InputTokens: 1000, CachedTokens: 300}
	for _, c := range []struct {
		prov string
		n    int
		ok   bool
	}{{"claude", 1000, true}, {"codex", 700, true}, {"gemini", 0, false}, {"opencode", 0, false}, {"", 0, false}} {
		if n, ok := EntradaNueva(c.prov, u); n != c.n || ok != c.ok {
			t.Fatalf("EntradaNueva(%q) = %d %v, quiero %d %v", c.prov, n, ok, c.n, c.ok)
		}
	}
	if n, ok := EntradaNueva("codex", Usage{InputTokens: 10, CachedTokens: 30}); n != 0 || !ok {
		t.Fatalf("nunca negativo: %d %v", n, ok)
	}
}
