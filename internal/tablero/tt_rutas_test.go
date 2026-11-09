// Los archivos fuera del proyecto en el modelo (CA-522, CA-524): enter sobre
// un hallazgo y o en la pestana Spec con una ruta que sale del proyecto en
// algun sistema (absoluta en cualquiera, con una unidad, un UNC, una raiz
// "\" o un ".." que sube con / o \) no ordenan nada y la barra dice en rojo
// "el archivo <ruta> esta fuera del proyecto". Regresiones de la ronda 1
// (review completa 20261108T231004_195bc7 de tablero-en-la-terminal) y de
// la ronda 2 (review delta: hallazgo 20261109T003952_e7f022), escritas desde
// el contrato de hoy (doc.go y go doc del paquete), sin leer la
// implementacion. Los enlaces simbolicos que salen de la raiz son de Run:
// tt_run_enlaces_unix_test.go.
package tablero_test

import (
	"fmt"
	"path"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- CA-522, CA-524: fuera del proyecto

// ttFuera es el mensaje de un archivo que sale del proyecto.
func ttFuera(ruta string) string {
	return "el archivo " + strconv.Quote(ruta) + " esta fuera del proyecto"
}

// CA-524 (hallazgo f28d31): enter sobre un hallazgo cuyo archivo sale del
// proyecto (absoluto, o con ".." que sube) no ordena nada: la barra dice en
// rojo "el archivo <ruta con strconv.Quote> esta fuera del proyecto" durante
// DuracionAviso. Uno local con ".." adentro abre el editor.
func TestCA524_HallazgoFueraDelProyecto(t *testing.T) {
	casos := []struct{ file, ruta string }{
		{"/etc/passwd", "/etc/passwd"},
		{"../../x.go", "../../x.go"},
		{"../fuera.go:12", "../fuera.go"},
		{`../x "y".go`, `../x "y".go`},
		{"../\x1b[2J.go", "../\x1b[2J.go"},
	}
	var hs []finding.Item
	for i, cs := range casos {
		hs = append(hs, ttHallazgo(fmt.Sprintf("f%d", i), finding.StatusOpen, "high", cs.file, fmt.Sprintf("hallazgo %d", i)))
	}
	hs = append(hs, ttHallazgo("local", finding.StatusOpen, "low", "dentro/../a.go", "local con puntos"))
	for _, color := range []bool{false, true} {
		cfg := ttAncho()
		cfg.Color = color
		c := ttInfraAPI(false)
		c.Evidence.Dir = "."
		m := ttAbrir(t, cfg, c, tablero.Detalle{})
		m.Tecla("4", ttAhora)
		m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Findings: hs}})
		ahora := ttAhora
		for i, cs := range casos {
			ahora = ahora.Add(time.Second)
			que := fmt.Sprintf("enter sobre el hallazgo de %q (color=%v)", cs.file, color)
			ttSinOrdenes(t, "CA-524", que, m.Tecla(tablero.TeclaEnter, ahora))
			ttBarraDice(t, "CA-524", que, m, color, true, ttFuera(cs.ruta))
			if i == 0 {
				m.Tic(ahora.Add(tablero.DuracionAviso + time.Second))
				if strings.Contains(ttBarra1(m), "fuera del proyecto") {
					t.Fatalf("CA-524: el aviso dura DuracionAviso: %q", ttBarra1(m))
				}
				ahora = ahora.Add(tablero.DuracionAviso + time.Second)
			}
			m.Tecla("j", ahora)
		}
		ords := m.Tecla(tablero.TeclaEnter, ahora)
		if len(ords) != 1 {
			t.Fatalf("CA-524: un archivo local (con un .. que no sale) abre el editor: %#v", ords)
		}
		if _, ok := ords[0].(tablero.AbrirEditor); !ok {
			t.Fatalf("CA-524: un archivo local ordena AbrirEditor: %#v", ords[0])
		}
	}
}

// CA-522 (hallazgo f28d31): o en la pestana Spec con una ruta de spec que
// sale del proyecto no ordena nada y la barra dice en rojo "el archivo
// <ruta> esta fuera del proyecto".
func TestCA522_SpecFueraDelProyecto(t *testing.T) {
	for _, ruta := range []string{"../../fuera.md", "/etc/hoom/spec.md"} {
		for _, color := range []bool{false, true} {
			cfg := ttAncho()
			cfg.Color = color
			c := ttInfraAPI(false)
			spec := boardcmd.SpecDetail{Path: ruta, Exists: true, Markdown: "# Spec\n\nuna linea\n"}
			m := ttAbrir(t, cfg, c, tablero.Detalle{Detalle: boardcmd.Detail{Spec: spec}})
			m.Tecla("2", ttAhora)
			m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec}})
			que := fmt.Sprintf("o con el spec en %q (color=%v)", ruta, color)
			ttSinOrdenes(t, "CA-522", que, m.Tecla("o", ttAhora))
			ttBarraDice(t, "CA-522", que, m, color, true, ttFuera(ruta))
		}
	}
}

// ---------------------------------------------------------------- CA-522, CA-524: fuera en otro sistema (ronda 2)

// ttAbreLocal exige que ords sea un solo AbrirEditor del archivo local ruta:
// el de la ruta tal cual o el de la misma ruta limpia (path.Clean), con la
// linea linea.
func ttAbreLocal(t *testing.T, ca, que string, ords []tablero.Orden, ruta string, linea int) {
	t.Helper()
	if len(ords) != 1 {
		t.Fatalf("%s: %s: un archivo local (con un .. que no sale) abre el editor: %#v", ca, que, ords)
	}
	a, ok := ords[0].(tablero.AbrirEditor)
	if !ok {
		t.Fatalf("%s: %s: un archivo local ordena AbrirEditor: %#v", ca, que, ords[0])
	}
	if (a.Archivo != ruta && a.Archivo != path.Clean(ruta)) || a.Linea != linea {
		t.Fatalf("%s: %s: AbrirEditor del archivo %q en la linea %d: %#v", ca, que, ruta, linea, a)
	}
}

// CA-524 (hallazgo 20261109T003952_e7f022): un archivo que sale del
// proyecto en algun sistema sale tambien en este (Unix): con una unidad
// ("C:/fuera/x.go", tambien con su ":12", y "C:x.go"), un UNC
// (`\\servidor\recurso\x.go`), una raiz "\" (`\x.go`) o un ".." que sube con
// "\" como separador (`..\..\x.go`, `sub\..\..\x.go`). enter sobre ese
// hallazgo no ordena nada y la barra dice en rojo "el archivo <ruta con
// strconv.Quote> esta fuera del proyecto". Uno local con un ".." que no sale
// ("sub/../x.go", y con "\": `sub\..\x.go`) abre el editor.
func TestCA524_HallazgoFueraDelProyectoEnOtroSistema(t *testing.T) {
	casos := []struct{ file, ruta string }{
		{"C:/fuera/x.go", "C:/fuera/x.go"},
		{"C:/fuera/x.go:12", "C:/fuera/x.go"},
		{"C:x.go", "C:x.go"},
		{`\\servidor\recurso\x.go`, `\\servidor\recurso\x.go`},
		{`\x.go`, `\x.go`},
		{`..\..\x.go`, `..\..\x.go`},
		{`sub\..\..\x.go`, `sub\..\..\x.go`},
	}
	var hs []finding.Item
	for i, cs := range casos {
		hs = append(hs, ttHallazgo(fmt.Sprintf("f%d", i), finding.StatusOpen, "high", cs.file, fmt.Sprintf("hallazgo %d", i)))
	}
	hs = append(hs, ttHallazgo("local", finding.StatusOpen, "low", "sub/../x.go", "local con puntos"),
		ttHallazgo("local-barra", finding.StatusOpen, "low", `sub\..\x.go`, "local con puntos y barras"))
	for _, color := range []bool{false, true} {
		cfg := ttAncho()
		cfg.Color = color
		c := ttInfraAPI(false)
		c.Evidence.Dir = "."
		m := ttAbrir(t, cfg, c, tablero.Detalle{})
		m.Tecla("4", ttAhora)
		m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Findings: hs}})
		ahora := ttAhora
		for _, cs := range casos {
			ahora = ahora.Add(time.Second)
			que := fmt.Sprintf("enter sobre el hallazgo de %q (color=%v)", cs.file, color)
			ttSinOrdenes(t, "CA-524", que, m.Tecla(tablero.TeclaEnter, ahora))
			ttBarraDice(t, "CA-524", que, m, color, true, ttFuera(cs.ruta))
			m.Tecla("j", ahora)
		}
		ttAbreLocal(t, "CA-524", fmt.Sprintf("sub/../x.go (color=%v)", color), m.Tecla(tablero.TeclaEnter, ahora), "sub/../x.go", 0)
		m.Tecla("j", ahora)
		ords := m.Tecla(tablero.TeclaEnter, ahora)
		if len(ords) != 1 {
			t.Fatalf(`CA-524: un archivo local con "\" y un .. que no sale (sub\..\x.go) abre el editor: %#v (barra %q)`, ords, ttBarra1(m))
		}
		if _, ok := ords[0].(tablero.AbrirEditor); !ok {
			t.Fatalf(`CA-524: sub\..\x.go ordena AbrirEditor: %#v`, ords[0])
		}
	}
}

// CA-522 (hallazgo 20261109T003952_e7f022): lo mismo con o en la pestana
// Spec: un spec en "C:/fuera/spec.md", "C:spec.md", un UNC, con raiz "\" o
// con un "..\" que sube no ordena nada y la barra dice en rojo "el archivo
// <ruta> esta fuera del proyecto". Uno local con un ".." que no sale
// ("sub/../spec.md") abre el editor en la linea 1.
func TestCA522_SpecFueraDelProyectoEnOtroSistema(t *testing.T) {
	abrir := func(cfg tablero.Config, ruta string) *tablero.Modelo {
		t.Helper()
		c := ttInfraAPI(false)
		spec := boardcmd.SpecDetail{Path: ruta, Exists: true, Markdown: "# Spec\n\nuna linea\n"}
		m := ttAbrir(t, cfg, c, tablero.Detalle{Detalle: boardcmd.Detail{Spec: spec}})
		m.Tecla("2", ttAhora)
		m.LeerDetalle(tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{Card: c, Spec: spec}})
		if m.Vista() != tablero.VistaSpec {
			t.Fatalf("CA-521: 2 es la pestana Spec, la vista es %q", m.Vista())
		}
		return m
	}
	for _, ruta := range []string{"C:/fuera/spec.md", "C:spec.md", `\\servidor\recurso\spec.md`, `\spec.md`, `..\..\spec.md`} {
		for _, color := range []bool{false, true} {
			cfg := ttAncho()
			cfg.Color = color
			m := abrir(cfg, ruta)
			que := fmt.Sprintf("o con el spec en %q (color=%v)", ruta, color)
			ttSinOrdenes(t, "CA-522", que, m.Tecla("o", ttAhora))
			ttBarraDice(t, "CA-522", que, m, color, true, ttFuera(ruta))
		}
	}
	m := abrir(ttAncho(), "sub/../spec.md")
	ttAbreLocal(t, "CA-522", "o con el spec en sub/../spec.md", m.Tecla("o", ttAhora), "sub/../spec.md", 1)
}
