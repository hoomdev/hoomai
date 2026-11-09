// Ronda 1 del test-writer, en Run (con la terminal, los datos, el lanzador y
// el editor falsos de tt_run_test.go; cada espera con su tiempo maximo) y en
// los Datos reales: las lecturas del proyecto (CA-562, hallazgos 361c6a y
// 140c81) y el mouse partido entre dos lecturas (CA-575, hallazgo d57fc7).
package tablero_test

import (
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttLanzManual es un lanzador falso que guarda el fin de cada orden y lo
// entrega cuando el test lo pide (CA-562, hallazgo 361c6a).
type ttLanzManual struct {
	mu   sync.Mutex
	ords []tablero.Lanzar
	fins []func(tablero.FinProceso)
}

func (l *ttLanzManual) Lanzar(o tablero.Lanzar, fin func(tablero.FinProceso)) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ords = append(l.ords, o)
	l.fins = append(l.fins, fin)
	return nil
}

func (l *ttLanzManual) cuantas() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.ords)
}

// terminar entrega el fin de la orden i con ese codigo, desde otra goroutine
// (como un lanzador real).
func (l *ttLanzManual) terminar(i, code int) {
	l.mu.Lock()
	f, id := l.fins[i], l.ords[i].ID
	l.mu.Unlock()
	go f(tablero.FinProceso{ID: id, Code: code})
}

// CA-562, hallazgo 361c6a: IntervaloProyecto vale 10 s (e Intervalo 2 s), y
// es lo que Run usa con Entorno.IntervaloProyecto en 0: en mas de un segundo,
// con el tablero leido muchas veces, el proyecto y los hallazgos se leyeron
// una sola vez (la del principio).
func TestCA562_IntervaloProyectoPorDefecto(t *testing.T) {
	if tablero.IntervaloProyecto != 10*time.Second || tablero.Intervalo != 2*time.Second {
		t.Fatalf("CA-562 (hallazgo 361c6a): IntervaloProyecto es 10s e Intervalo 2s: %v %v", tablero.IntervaloProyecto, tablero.Intervalo)
	}
	w := ttNuevoMundo(t, ttTableroPrincipal())
	if w.e.IntervaloProyecto != 0 {
		t.Fatalf("CA-562 (guarda): el mundo de prueba deja IntervaloProyecto en 0: %v", w.e.IntervaloProyecto)
	}
	fin := w.correr(nil)
	ttEsperar(t, "CA-562", "el primer cuadro", w, func() bool { return w.marcoCon("[2] Tarjetas") })
	t0 := time.Now()
	ttEsperar(t, "CA-562", "muchas lecturas del tablero en mas de un segundo", w, func() bool {
		return w.datos.cuantas() >= 40 && time.Since(t0) > 1200*time.Millisecond
	})
	if p, h := w.datos.lecturasProyecto(); p != 1 || h != 1 {
		t.Fatalf("CA-562 (hallazgo 361c6a): con IntervaloProyecto en 0 (10s) el proyecto y los hallazgos se leen una vez en el primer segundo: %d %d", p, h)
	}
	w.term.mandar("q")
	fin(t)
}

// CA-562, hallazgo 361c6a: la relectura de [1] y [4] es al terminar un verbo
// que la TUI lanzo, no al lanzarlo: con un lanzador que entrega el
// FinProceso cuando el test lo pide, lanzar no relee; el fin si.
func TestCA562_RunReleeAlTerminarNoAlLanzar(t *testing.T) {
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(ttAccionWriter())))
	lz := &ttLanzManual{}
	w.e.Lanzador = lz
	w.e.IntervaloProyecto = time.Hour
	fin := w.correr(nil)
	ttEsperar(t, "CA-562", "la lectura del principio", w, func() bool { p, h := w.datos.lecturasProyecto(); return p == 1 && h == 1 })
	ttEsperar(t, "CA-562", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("u")
	ttEsperar(t, "CA-562", "la orden de hoom usar", w, func() bool { return lz.cuantas() == 1 })
	time.Sleep(300 * time.Millisecond)
	if p, h := w.datos.lecturasProyecto(); p != 1 || h != 1 {
		t.Fatalf("CA-562 (hallazgo 361c6a): lanzar no relee el proyecto ni los hallazgos (eso es al terminar): %d %d", p, h)
	}
	lz.terminar(0, 0)
	ttEsperar(t, "CA-562", "otra lectura tras el fin", w, func() bool {
		p, h := w.datos.lecturasProyecto()
		return p >= 2 && h >= 2
	})
	w.term.mandar("q")
	fin(t)
}

// CA-575, hallazgo d57fc7: un evento del mouse partido en dos lecturas
// separadas por mas que la espera de las secuencias (X10: "\x1b[M" y, 150 ms
// despues, " !!"; SGR: "\x1b[<64;3" y despues "1;5M") no abre ningun modal, no
// mueve el foco ni da teclas: la j que sigue elige la segunda tarjeta en [2].
func TestCA575_RunElMousePartidoNoSonTeclas(t *testing.T) {
	seg := ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado")
	for _, partes := range [][2]string{{"\x1b[M", " !!"}, {"\x1b[<64;3", "1;5M"}} {
		w := ttNuevoMundo(t, ttTablero(ttCard("uno", "Uno", boardcmd.ColWriter, "la primera", seg),
			ttCard("dos", "Dos", boardcmd.ColWriter, "la segunda", seg)))
		fin := w.correr(nil)
		ttEsperar(t, "CA-575", "[2] con uno elegida", w, func() bool { return w.filaElegida("uno") })
		w.term.mandar(partes[0])
		time.Sleep(150 * time.Millisecond)
		w.term.mandar(partes[1])
		w.term.mandar("j")
		ttEsperar(t, "CA-575 (hallazgo d57fc7)", "la j elige dos, con el foco en [2] (el evento partido no dio teclas)", w, func() bool {
			return w.filaElegida("dos")
		})
		if m := strings.Join(w.ultimoMarco(), "\n"); strings.Contains(m, "Acciones ·") || strings.Contains(m, "Todas las teclas") {
			t.Fatalf("CA-575 (hallazgo d57fc7): el evento partido %q no abre ningun modal:\n%s", partes, m)
		}
		w.term.mandar("q")
		if code, _ := fin(t); code != 0 {
			t.Fatalf("CA-575: q sale con 0: %d", code)
		}
	}
}

// CA-562, hallazgo 140c81: DatosDe.Proyecto con un veredicto ilegible no
// escribe nada en os.Stderr (la TUI tiene la terminal): el aviso llega en
// Proyecto.Avisos.
func TestCA562_DatosDeProyectoNoEscribeEnStderr(t *testing.T) {
	root, _ := ttProyecto(t)
	ttArchivo(t, root, ".hoom/verdicts/20261108T090000_bad111.json", "{esto no es un veredicto")
	d := ttDatosDe(t, root)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stderr
	os.Stderr = w
	var p tablero.Proyecto
	func() {
		defer func() { os.Stderr = orig }()
		p = d.Proyecto()
	}()
	w.Close()
	escrito, _ := io.ReadAll(r)
	r.Close()
	if len(escrito) != 0 {
		t.Fatalf("CA-562 (hallazgo 140c81): leer el proyecto no escribe en os.Stderr: %q", escrito)
	}
	if len(p.Avisos) == 0 {
		t.Fatalf("CA-562 (hallazgo 140c81): el veredicto ilegible llega en Proyecto.Avisos: %+v", p)
	}
}
