//go:build !windows

// Run y los enlaces simbolicos del proyecto (CA-524, CA-522, CA-533;
// hallazgo 20261109T003335_7766ba de la review delta de la ronda 2): un
// AbrirEditor cuyo Archivo, seguido por sus enlaces, sale de la raiz del
// proyecto (un enlace del proyecto a un archivo o a un directorio de afuera)
// no abre ningun editor y la barra dice "el archivo <Archivo con
// strconv.Quote> esta fuera del proyecto"; uno que queda adentro, o que
// todavia no existe, abre el editor con raiz/<Archivo>. Escrito desde el
// contrato de Run (go doc del paquete), sin leer la implementacion. Solo
// Unix: los enlaces se crean con os.Symlink. Con la terminal, los datos, el
// lanzador y el editor falsos de tt_run_test.go; cada espera con su tiempo
// maximo.
package tablero_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttEnlaces arma en root, la raiz del proyecto, los enlaces de los casos:
// "link" a un directorio de afuera (con x.go), "flink" a un archivo de
// afuera, "dangling" a un archivo de afuera que no existe, "dentro" a
// real.go de adentro y "dsub" a sub/ de adentro (con y.go). Los de afuera son
// absolutos; los de adentro, relativos.
func ttEnlaces(t *testing.T, root string) {
	t.Helper()
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ttArchivo(t, fuera, "x.go", "package fuera\n")
	ttArchivo(t, fuera, "f.go", "package fuera\n")
	ttArchivo(t, root, "real.go", "package dentro\n")
	ttArchivo(t, root, "sub/y.go", "package sub\n")
	for enlace, destino := range map[string]string{
		"link":     fuera,
		"flink":    filepath.Join(fuera, "f.go"),
		"dangling": filepath.Join(fuera, "no-existe.go"),
		"dentro":   "real.go",
		"dsub":     "sub",
	} {
		if err := os.Symlink(destino, filepath.Join(root, enlace)); err != nil {
			t.Fatalf("no pude crear el enlace %s -> %s: %v", enlace, destino, err)
		}
	}
}

// ttMundoConEnlaces es un mundo de Run cuya raiz (Dir, sin enlaces en su
// camino) tiene los enlaces de ttEnlaces, con una sola tarjeta abierta cuyo
// detalle trae un hallazgo en archivo y el spec en archivo. La terminal tiene
// 120 columnas: la barra 1 con el aviso mas largo pasa de 80.
func ttMundoConEnlaces(t *testing.T, archivo string) *ttMundo {
	t.Helper()
	c := ttTarjetaCon(ttAccionWriter())
	w := ttNuevoMundo(t, ttTablero(c))
	w.term.cambiar(120, 24)
	dir, err := filepath.EvalSymlinks(w.e.Dir)
	if err != nil {
		t.Fatal(err)
	}
	w.e.Dir = dir
	ttEnlaces(t, dir)
	w.datos.detalleDe = func(slug string) tablero.Detalle {
		return tablero.Detalle{Slug: slug, Detalle: boardcmd.Detail{Card: c,
			Spec:     boardcmd.SpecDetail{Path: archivo, Exists: true, Markdown: "# Spec\n\nuna linea\n"},
			Findings: []finding.Item{ttHallazgo("f1", finding.StatusOpen, "high", archivo, "el hallazgo del enlace")}}}
	}
	return w
}

// ttMarcoDice dice si el ultimo cuadro, sin las SGR del color, tiene s.
func ttMarcoDice(w *ttMundo, s string) bool {
	return strings.Contains(ttSGR.ReplaceAllString(strings.Join(w.ultimoMarco(), "\n"), ""), s)
}

// ttPedirEditor arranca Run, abre la tarjeta y pide el editor desde la
// pestana de desde ("hallazgo": enter en Hallazgos; "spec": o en Spec).
// Devuelve el fin de Run, desde donde mirar el registro y la linea con la que
// se abriria.
func ttPedirEditor(t *testing.T, ca string, w *ttMundo, desde, archivo string) (func(*testing.T) (int, any), int, int) {
	t.Helper()
	fin := w.correr(nil)
	ttEsperar(t, ca, "la tarjeta", w, func() bool { return ttMarcoDice(w, "tarjeta") })
	w.term.mandar("\r")
	ttEsperar(t, ca, "la tarjeta abierta", w, func() bool { return ttMarcoDice(w, "[1 Resumen]") })
	if desde == "spec" {
		w.term.mandar("2")
		ttEsperar(t, ca, "la pestana Spec", w, func() bool { return ttMarcoDice(w, "una linea") })
		marca := len(w.r.copia())
		w.term.mandar("o")
		return fin, marca, 1
	}
	w.term.mandar("4")
	ttEsperar(t, ca, "la pestana Hallazgos", w, func() bool { return ttMarcoDice(w, "high "+archivo) })
	marca := len(w.r.copia())
	w.term.mandar("\r")
	return fin, marca, 0
}

// ttNoAbrio exige que no haya ningun Abrir del editor desde marca.
func ttNoAbrio(t *testing.T, ca, que string, w *ttMundo, marca int) {
	t.Helper()
	if i := w.r.indice(marca, func(e ttSuceso) bool { return e.que == "abrir" }); i >= 0 {
		t.Fatalf("%s: %s no abre ningun editor, se llamo a Abrir con %q", ca, que, w.r.copia()[i].dato)
	}
}

// ttAbrioCon espera el Abrir del editor desde marca y exige que sea con
// quiere; despues espera que la terminal se tome de nuevo.
func ttAbrioCon(t *testing.T, ca, que string, w *ttMundo, marca int, quiere string) {
	t.Helper()
	ttEsperar(t, ca, que+": el editor", w, func() bool {
		return w.r.indice(marca, func(e ttSuceso) bool { return e.que == "abrir" }) >= 0
	})
	abrir := w.r.indice(marca, func(e ttSuceso) bool { return e.que == "abrir" })
	if got := w.r.copia()[abrir].dato; got != quiere {
		t.Fatalf("%s: %s: Abrir con raiz/<Archivo> y la linea: %q, se esperaba %q", ca, que, got, quiere)
	}
	if w.marcoCon("fuera del proyecto") {
		t.Fatalf("%s: %s: un archivo de adentro no dice que esta fuera del proyecto", ca, que)
	}
	ttEsperar(t, ca, que+": la terminal tomada de nuevo", w, func() bool {
		cr := w.r.indice(abrir, func(e ttSuceso) bool { return e.que == "crudo" })
		en := w.r.indice(abrir, func(e ttSuceso) bool { return e.que == "escribe" && strings.Contains(e.dato, tablero.SecEntrar) })
		return cr > abrir && en > cr
	})
}

// ttSalirConQ manda q y exige que Run salga con 0.
func ttSalirConQ(t *testing.T, ca, que string, w *ttMundo, fin func(*testing.T) (int, any)) {
	t.Helper()
	w.term.mandar("q")
	if code, pan := fin(t); code != 0 || pan != nil {
		t.Fatalf("%s: %s: q sale con 0, salio con %d (panic %v)", ca, que, code, pan)
	}
}

// CA-524, CA-522, CA-533 (hallazgo 20261109T003335_7766ba): enter sobre un
// hallazgo, y o con el spec, cuyo archivo es "link/x.go" (por un enlace a un
// directorio de afuera), "link/nuevo.go" (por ese enlace, un archivo que no
// existe), "flink" (un enlace a un archivo de afuera) o "dangling" (un enlace
// a un archivo de afuera que no existe): el Abrir del Editor falso no se
// llama y la barra dice `el archivo "<archivo>" esta fuera del proyecto`; la
// TUI sigue (q sale con 0). "dentro" (un enlace a un archivo de adentro),
// "dsub/y.go" (por un enlace a un directorio de adentro) y "nuevo.go" (que
// todavia no existe) abren el editor con raiz/<archivo>.
func TestCA524_RunEnlaceQueSaleDelProyecto(t *testing.T) {
	casos := []struct {
		archivo string
		fuera   bool
	}{
		{"link/x.go", true},
		{"link/nuevo.go", true},
		{"flink", true},
		{"dangling", true},
		{"dentro", false},
		{"dsub/y.go", false},
		{"nuevo.go", false},
	}
	for _, desde := range []string{"hallazgo", "spec"} {
		for _, cs := range casos {
			que := desde + " en " + cs.archivo
			t.Run(que, func(t *testing.T) {
				w := ttMundoConEnlaces(t, cs.archivo)
				fin, marca, linea := ttPedirEditor(t, "CA-524", w, desde, cs.archivo)
				if cs.fuera {
					msg := ttFuera(cs.archivo)
					ttEsperar(t, "CA-524", que+": "+msg+" en la barra", w, func() bool { return ttMarcoDice(w, msg) })
					ttNoAbrio(t, "CA-524", que, w, marca)
					ttSalirConQ(t, "CA-533", que, w, fin)
					ttNoAbrio(t, "CA-524", que, w, marca)
					return
				}
				ttAbrioCon(t, "CA-524", que, w, marca, fmt.Sprintf("%s:%d", filepath.Join(w.e.Dir, cs.archivo), linea))
				ttSalirConQ(t, "CA-533", que, w, fin)
			})
		}
	}
}

// CA-524 (hallazgo 20261109T003335_7766ba): con color, el aviso del enlace
// que sale del proyecto va en rojo.
func TestCA524_RunEnlaceQueSaleDelProyectoEnRojo(t *testing.T) {
	w := ttMundoConEnlaces(t, "link/x.go")
	env := map[string]string{"HOOM_ASCII": "1", "LANG": "C"} // sin NO_COLOR
	w.e.Getenv = func(k string) string { return env[k] }
	msg := ttFuera("link/x.go")
	fin, marca, _ := ttPedirEditor(t, "CA-524", w, "hallazgo", "link/x.go")
	var barra string
	ttEsperar(t, "CA-524", "el aviso en la barra", w, func() bool {
		for _, l := range w.ultimoMarco() {
			if strings.Contains(l, msg) {
				barra = l
				return true
			}
		}
		return false
	})
	if !ttConColor(ttRojo, barra, msg) {
		t.Fatalf("CA-524: el aviso del enlace que sale del proyecto va en rojo: %q", barra)
	}
	ttNoAbrio(t, "CA-524", "con color", w, marca)
	ttSalirConQ(t, "CA-533", "con color", w, fin)
}

// CA-524 (hallazgo 20261109T003335_7766ba): la raiz del proyecto puede estar
// detras de un enlace (Dir es un enlace al directorio del proyecto, como
// /tmp en macOS): lo de adentro sigue adentro ("dentro" y "dsub/y.go" abren
// el editor con Dir/<archivo>) y lo que sale sigue afuera ("link/x.go" no
// abre nada).
func TestCA524_RunRaizDetrasDeUnEnlace(t *testing.T) {
	for _, cs := range []struct {
		archivo string
		fuera   bool
	}{{"dentro", false}, {"dsub/y.go", false}, {"link/x.go", true}} {
		t.Run(cs.archivo, func(t *testing.T) {
			w := ttMundoConEnlaces(t, cs.archivo)
			otro, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(otro, "alias")
			if err := os.Symlink(w.e.Dir, alias); err != nil {
				t.Fatal(err)
			}
			w.e.Dir = alias
			que := "con Dir detras de un enlace, " + cs.archivo
			fin, marca, _ := ttPedirEditor(t, "CA-524", w, "hallazgo", cs.archivo)
			if cs.fuera {
				msg := ttFuera(cs.archivo)
				ttEsperar(t, "CA-524", que+": "+msg+" en la barra", w, func() bool { return ttMarcoDice(w, msg) })
				ttNoAbrio(t, "CA-524", que, w, marca)
				ttSalirConQ(t, "CA-533", que, w, fin)
				return
			}
			ttAbrioCon(t, "CA-524", que, w, marca, filepath.Join(alias, cs.archivo)+":0")
			ttSalirConQ(t, "CA-533", que, w, fin)
		})
	}
}
