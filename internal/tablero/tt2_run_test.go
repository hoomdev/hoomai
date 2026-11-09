// Run en la v2 (spec tablero-con-paneles): CA-562 las lecturas del proyecto
// y de los hallazgos (al abrir, cada IntervaloProyecto y cuando termina un
// verbo que la TUI ejecuto) y la del archivo de un hallazgo, y CA-573 los
// models de hoom.yaml (un error es exit 1 antes de tomar la pantalla; las
// entradas llegan al selector de Modelo). Con la terminal, los datos, el
// lanzador y el editor falsos de tt_run_test.go; cada espera con su tiempo
// maximo.
package tablero_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// CA-562: Run lee el proyecto y los hallazgos al abrir y despues cada
// IntervaloProyecto; [1] y [4] muestran lo leido.
func TestCA562_RunLeeElProyectoYLosHallazgos(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	w.datos.hallazgos = ttHallazgosDorados()
	w.e.IntervaloProyecto = 20 * time.Millisecond
	fin := w.correr(nil)
	ttEsperar(t, "CA-562", "[1] y [4] con lo leido", w, func() bool {
		return w.marcoCon("demo · rama main") && w.marcoCon("[4] Hallazgos 2")
	})
	ttEsperar(t, "CA-562", "cinco lecturas del proyecto con IntervaloProyecto de 20ms", w, func() bool {
		p, h := w.datos.lecturasProyecto()
		return p >= 5 && h >= 5
	})
	w.term.mandar("q")
	fin(t)
}

// CA-562: con IntervaloProyecto largo, el proyecto y los hallazgos se leen al
// abrir y otra vez cuando termina un proceso que la TUI lanzo (cada
// FinProceso).
func TestCA562_RunLeeOtraVezCuandoTerminaUnVerbo(t *testing.T) {
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(ttAccionWriter())))
	w.e.IntervaloProyecto = time.Hour
	w.lanz.fin = &tablero.FinProceso{Code: 0, Ultima: "tarjeta fijada para esta terminal: tarjeta"}
	fin := w.correr(nil)
	ttEsperar(t, "CA-562", "la lectura del principio", w, func() bool { p, h := w.datos.lecturasProyecto(); return p == 1 && h == 1 })
	ttEsperar(t, "CA-562", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	time.Sleep(50 * time.Millisecond)
	if p, h := w.datos.lecturasProyecto(); p != 1 || h != 1 {
		t.Fatalf("CA-562: con IntervaloProyecto de una hora no hay otra lectura sola: %d %d", p, h)
	}
	w.term.mandar("u")
	ttEsperar(t, "CA-562", "otra lectura tras el fin de hoom usar", w, func() bool {
		p, h := w.datos.lecturasProyecto()
		return p >= 2 && h >= 2
	})
	ttEsperar(t, "CA-563", "el Registro dice hecho", w, func() bool { return w.marcoCon("Fijar para la terminal · tarjeta · hecho") })
	w.term.mandar("q")
	fin(t)
}

// CA-562: una lectura del proyecto que falla deja lo ultimo y la linea de
// mensajes lo dice.
func TestCA562_RunLecturaDelProyectoQueFalla(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	w.e.IntervaloProyecto = 20 * time.Millisecond
	fin := w.correr(nil)
	ttEsperar(t, "CA-562", "[1] leido", w, func() bool { return w.marcoCon("demo · rama main") })
	w.datos.mu.Lock()
	w.datos.proyecto = tablero.Proyecto{Err: errors.New("git no responde")}
	w.datos.mu.Unlock()
	ttEsperar(t, "CA-562", "el error en la linea de mensajes", w, func() bool {
		return w.marcoCon("no se pudo leer el proyecto: git no responde") && w.marcoCon("demo · rama main")
	})
	w.term.mandar("q")
	fin(t)
}

// CA-559: Run lee el Archivo de ArchivoElegido cuando cambia (el hallazgo
// elegido en [4]), y [0] muestra sus lineas.
func TestCA559_RunLeeElArchivoDelHallazgo(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	w.datos.hallazgos = ttHallazgosDorados()
	w.datos.archivoDe = func(ruta string, linea int) tablero.Archivo {
		return tablero.Archivo{Ruta: ruta, Linea: linea, Desde: linea, Lineas: []string{"la linea del hallazgo"}}
	}
	fin := w.correr(nil)
	ttEsperar(t, "CA-559", "[4] leido", w, func() bool { return w.marcoCon("[4] Hallazgos 2") })
	w.term.mandar("4")
	ttEsperar(t, "CA-559", "las lineas del archivo en [0]", w, func() bool { return w.marcoCon("> 41 la linea del hallazgo") })
	w.datos.mu.Lock()
	pedidos := append([]string(nil), w.datos.archivos...)
	w.datos.mu.Unlock()
	if len(pedidos) == 0 || pedidos[0] != "internal/api/router.go:41" {
		t.Fatalf("CA-559: Run pide el Archivo de ArchivoElegido: %v", pedidos)
	}
	w.term.mandar("j")
	ttEsperar(t, "CA-559", "el archivo del otro hallazgo", w, func() bool {
		w.datos.mu.Lock()
		defer w.datos.mu.Unlock()
		return len(w.datos.archivos) > 0 && w.datos.archivos[len(w.datos.archivos)-1] == "internal/tenant/tenant.go:7"
	})
	w.term.mandar("q")
	fin(t)
}

// CA-573, CA-516: un models: invalido en hoom.yaml (un provider que no recibe
// modelo, uno desconocido, una entrada sin name) es exit 1 con "hoom: <error
// de manifest.Load>" antes de tocar la terminal.
func TestCA573_CA516_RunModelsInvalido(t *testing.T) {
	for nombre, models := range map[string]string{
		"gemini":      "models:\n  gemini:\n    - name: x\n",
		"desconocido": "models:\n  foo:\n    - name: x\n",
		"sin name":    "models:\n  codex:\n    - description: y\n",
	} {
		w := ttNuevoMundo(t, ttTablero())
		ruta := filepath.Join(w.e.Dir, "hoom.yaml")
		if err := os.WriteFile(ruta, []byte(ttYAML+models), 0o644); err != nil {
			t.Fatal(err)
		}
		_, lerr := manifest.Load(w.e.Dir, profiles.Resolve)
		if lerr == nil {
			t.Fatalf("CA-573 (guarda): %s es un hoom.yaml invalido para manifest.Load", nombre)
		}
		code, _ := w.correr(nil)(t)
		if code != 1 {
			t.Fatalf("CA-573: con models %s Run sale con 1, salio con %d", nombre, code)
		}
		if got := strings.TrimRight(w.r.texto("err"), "\n"); got != "hoom: "+lerr.Error() {
			t.Fatalf("CA-573: con models %s Err es 'hoom: <error>':\n got %q\nwant %q", nombre, got, "hoom: "+lerr.Error())
		}
		for _, e := range w.r.copia() {
			if e.que == "crudo" || e.que == "escribe" {
				t.Fatalf("CA-573: con models %s no se toca la terminal: %v", nombre, w.r.copia())
			}
		}
	}
}

// CA-573, CA-569: las entradas de models.<provider> de hoom.yaml llegan al
// selector de Modelo del modal Pedir (Config.Modelos), con su descripcion
// como su linea.
func TestCA573_CA569_RunModelsEnElSelector(t *testing.T) {
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(ttAccionWriter())))
	extra := "models:\n  claude:\n    - name: mi-modelo\n      description: el de la casa\n"
	if err := os.WriteFile(filepath.Join(w.e.Dir, "hoom.yaml"), []byte(ttYAML+extra), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Load(w.e.Dir, profiles.Resolve); err != nil {
		t.Fatalf("CA-573 (guarda): el hoom.yaml con models es valido: %v", err)
	}
	fin := w.correr(nil)
	ttEsperar(t, "CA-573", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("w")
	ttEsperar(t, "CA-573", "el modal", w, func() bool { return w.marcoCon("Se va a ejecutar:") })
	w.term.mandar("\t\t")
	// de opus: sonnet, haiku, opusplan, fable[1m], opus[1m], sonnet[1m] y el de hoom.yaml
	w.term.mandar(strings.Repeat("\x1b[C", 7))
	ttEsperar(t, "CA-573", "el modelo de hoom.yaml elegido, con su linea", w, func() bool {
		return w.marcoCon("< mi-modelo >") && w.marcoCon("mi-modelo: el de la casa")
	})
	w.term.mandar("\x03")
	fin(t)
}
