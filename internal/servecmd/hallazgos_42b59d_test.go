// Regresion del hallazgo 42b59d (ronda 1 de proporcion-por-riesgo): lo que
// el dialogo mostro incluye el NIVEL del modelo. vistaDe de acciones.js manda
// la clave nivel, y si hoom.yaml pasa de model: fuerte a un nombre literal
// con el mismo modelo en el provider por defecto mientras el dialogo esta
// abierto, el POST /launch es el 409 que nombra el modelo, sin lanzar nada
// (con otro provider, el nivel y el nombre literal se traducen distinto).
package servecmd

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/modelos"
)

func TestHallazgo_42b59d_AccionesJSMandaElNivel(t *testing.T) {
	_, _, acc := atUI(t)
	cuerpo := h2Cuerpo(c2SinComentarios(acc), "vistaDe")
	if cuerpo == "" {
		t.Fatal("hallazgo 42b59d: acciones.js declara la funcion vistaDe")
	}
	if !regexp.MustCompile(`\bnivel\s*:\s*a\.nivel\b`).MatchString(cuerpo) {
		t.Fatalf("hallazgo 42b59d: vistaDe arma la clave nivel con el de la accion (nivel: a.nivel):\n%s", cuerpo)
	}
}

func TestHallazgo_42b59d_ElNivelQueCambioEsElModelo(t *testing.T) {
	const h = "hallazgo 42b59d"
	f := lnPATH(t)
	f.cli(t, "claude", false)
	f.cli(t, "codex", false)
	literal := modelos.Traducir("codex", modelos.Fuerte).Modelo
	root := raProyectoConReview(t, "agents:\n  writer:\n    provider: codex\n    model: fuerte\n")
	lnCartaWriter(t, h, root, "precios", "")
	s := lnServidor(t, root, f)
	a, ok := lnAccion(lnCarta(t, root, "precios"), boardcmd.ActPedirWriter)
	if !ok || !a.Enabled || a.Nivel != modelos.Fuerte || a.Model != literal {
		t.Fatalf("%s: fixture: pedir-writer propone %s desde el nivel fuerte: %+v", h, literal, a)
	}
	// lo que el dialogo mostro, como lo arma vistaDe
	vista := r1Vista(a)
	vista["nivel"] = a.Nivel

	// con el dialogo abierto, el writer pasa a nombrar el mismo modelo literal
	raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tbEscribir(t, root, "hoom.yaml", strings.Replace(string(raw), "model: fuerte", "model: "+literal, 1))
	lnCommit(t, root, "writer con nombre literal")
	if b, _ := lnAccion(lnCarta(t, root, "precios"), boardcmd.ActPedirWriter); b.Model != a.Model || b.Nivel != "" {
		t.Fatalf("%s: fixture: el mismo modelo para codex, ya sin nivel: %+v", h, b)
	}

	// la persona eligio claude, cuyo modelo (el del nivel) dejo intacto
	rec := lnLaunch(t, s, "precios", s.Token(), r1ConVista(lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec."), vista))
	want := "la tarjeta cambio mientras escribias el pedido (modelo): no se lanzo nada, vuelve a pedirlo"
	if rec.Code != http.StatusConflict || !strings.Contains(rec.Body.String(), want) {
		t.Fatalf("%s: el nivel que cambio es el modelo: 409 %q, fue %d %s", h, want, rec.Code, rec.Body.String())
	}
	lnEsperarLanzamientos(t, h, s)
	if n := len(f.llamadas(t, "claude")); n != 0 {
		t.Fatalf("%s: no se lanzo nada: claude corrio %d veces", h, n)
	}
}
