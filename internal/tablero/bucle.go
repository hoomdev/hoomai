package tablero

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"
)

// How often Run looks at the terminal's size and moves the clock, and how
// long it waits for the rest of an escape sequence.
const (
	cadaTic         = 250 * time.Millisecond
	esperaSecuencia = 30 * time.Millisecond
)

// pausable is a terminal that can stop reading while the editor has it
// (the real one): the reader goroutine must not take the editor's keys.
type pausable interface {
	Pausar()
	Reanudar()
}

// evento is what the goroutines of Run tell the loop.
type evento struct {
	teclas    []byte
	errLeer   error
	lectura   *Lectura
	detalle   *Detalle
	fin       *FinProceso
	panico    any
	hayPanico bool
}

// bucle is one run of the TUI.
type bucle struct {
	e    Entorno
	root string
	cfg  Config
	m    *Modelo

	restaurar func() error
	tomada    bool
	ultimo    string // the last frame written

	eventos chan evento
	listo   chan struct{}

	enVuelo      bool   // a read of the data is running
	abiertaLeida string // the Abierta of the last read asked
}

// correr takes the terminal, runs the loop and gives the terminal back,
// whatever happens.
func (b *bucle) correr() int {
	t := b.e.Terminal
	w, h, err := t.Tamano()
	if err != nil {
		fmt.Fprintln(b.e.Err, "hoom:", err)
		return 1
	}
	b.cfg.Ancho, b.cfg.Alto = w, h
	b.m = Nuevo(b.cfg)
	if err := b.tomar(); err != nil {
		fmt.Fprintln(b.e.Err, "hoom:", err)
		return 1
	}
	defer func() {
		if r := recover(); r != nil {
			b.soltar()
			panic(r)
		}
	}()
	code, errFinal := b.bucle()
	if err := b.soltar(); err != nil && errFinal == nil {
		errFinal, code = err, 1
	}
	if errFinal != nil {
		fmt.Fprintln(b.e.Err, "hoom:", errFinal)
	}
	if msg := MensajeSalida(b.m.Corriendo()); msg != "" {
		fmt.Fprintln(b.e.Out, msg)
	}
	return code
}

// tomar puts the terminal in raw mode and enters the alternate screen.
func (b *bucle) tomar() error {
	restaurar, err := b.e.Terminal.Crudo()
	if err != nil {
		return err
	}
	b.restaurar, b.tomada = restaurar, true
	b.ultimo = ""
	_, err = io.WriteString(b.e.Terminal, SecEntrar)
	return err
}

// soltar leaves the alternate screen and puts the terminal back.
func (b *bucle) soltar() error {
	if !b.tomada {
		return nil
	}
	b.tomada = false
	_, errW := io.WriteString(b.e.Terminal, SecSalir)
	var errR error
	if b.restaurar != nil {
		errR = b.restaurar()
	}
	if errW != nil {
		return errW
	}
	return errR
}

// enviar hands an event to the loop, unless the loop ended.
func (b *bucle) enviar(ev evento) {
	select {
	case b.eventos <- ev:
	case <-b.listo:
	}
}

// reenviar carries a panic of a goroutine to the loop's goroutine.
func (b *bucle) reenviar() {
	if r := recover(); r != nil {
		b.enviar(evento{panico: r, hayPanico: true})
	}
}

func (b *bucle) leerTeclas() {
	defer b.reenviar()
	buf := make([]byte, 512)
	for {
		n, err := b.e.Terminal.Read(buf)
		if n > 0 {
			b.enviar(evento{teclas: append([]byte(nil), buf[:n]...)})
		}
		if err != nil {
			b.enviar(evento{errLeer: err})
			return
		}
		select {
		case <-b.listo:
			return
		default:
		}
	}
}

// leer reads the data in another goroutine: the board (when conTablero)
// and the open card.
func (b *bucle) leer(conTablero bool) {
	b.enVuelo = true
	slug, historia := b.m.Abierta()
	b.abiertaLeida = clave(slug, historia)
	go func() {
		defer b.reenviar()
		var ev evento
		if conTablero {
			tab, err := b.e.Datos.Tablero()
			ev.lectura = &Lectura{Tablero: tab, Err: err, Fijada: b.e.Datos.Fijada()}
		}
		if slug != "" {
			d := b.e.Datos.Detalle(slug, historia)
			if d.Slug == "" {
				d.Slug = slug
			}
			ev.detalle = &d
		}
		b.enviar(ev)
	}()
}

func clave(slug string, historia bool) string {
	if historia {
		return slug + "\x00h"
	}
	return slug
}

func (b *bucle) bucle() (int, error) {
	b.eventos = make(chan evento, 16)
	b.listo = make(chan struct{})
	defer close(b.listo)

	tic := time.NewTicker(cadaTic)
	defer tic.Stop()
	// the next read of the board is Intervalo after the last one ended: a
	// board that takes longer than Intervalo to read is not read without
	// rest
	var proxima <-chan time.Time
	tableroPendiente := false
	winch, parar := avisosDeTamano()
	defer parar()

	go b.leerTeclas()
	if err := b.dibujar(); err != nil {
		return 1, err
	}
	b.leer(true)

	var pendiente []byte
	var espera <-chan time.Time
	for {
		var fin bool
		var code int
		var err error
		select {
		case ev := <-b.eventos:
			switch {
			case ev.hayPanico:
				panic(ev.panico)
			case ev.errLeer != nil:
				return 1, ev.errLeer
			case ev.teclas != nil:
				pendiente = append(pendiente, ev.teclas...)
				if incompleta(pendiente) {
					espera = time.After(esperaSecuencia)
					continue
				}
				espera = nil
				fin, code, err = b.teclas(pendiente)
				pendiente = nil
			case ev.fin != nil:
				b.m.Termino(*ev.fin, b.e.Ahora())
			default:
				b.enVuelo = false
				if ev.lectura != nil {
					l := *ev.lectura
					l.Ahora = b.e.Ahora()
					fin, code, err = b.ordenes(b.m.Leer(l))
					proxima = time.After(b.e.Intervalo)
				}
				if ev.detalle != nil {
					b.m.LeerDetalle(*ev.detalle)
				}
				if tableroPendiente {
					tableroPendiente = false
					b.leer(true)
				}
			}
		case <-espera:
			espera = nil
			fin, code, err = b.teclas(pendiente)
			pendiente = nil
		case <-tic.C:
			b.m.Tic(b.e.Ahora())
			b.mirarTamano()
		case <-winch:
			b.mirarTamano()
		case <-proxima:
			proxima = nil
			if b.enVuelo {
				tableroPendiente = true // after the card's read that is running
			} else {
				b.leer(true)
			}
		}
		if fin || err != nil {
			return code, err
		}
		// a card opened, or its Historia: read it now
		if slug, historia := b.m.Abierta(); slug != "" && !b.enVuelo && clave(slug, historia) != b.abiertaLeida {
			b.leer(false)
		}
		if err := b.dibujar(); err != nil {
			return 1, err
		}
	}
}

// teclas gives the keys to the model and carries out its orders.
func (b *bucle) teclas(buf []byte) (bool, int, error) {
	for _, t := range Decodificar(buf) {
		if fin, code, err := b.ordenes(b.m.Tecla(t, b.e.Ahora())); fin || err != nil {
			return fin, code, err
		}
	}
	return false, 0, nil
}

// ordenes carries out the model's orders; Salir ends the loop.
func (b *bucle) ordenes(ords []Orden) (bool, int, error) {
	for _, o := range ords {
		switch o := o.(type) {
		case Salir:
			return true, 0, nil
		case Lanzar:
			err := b.e.Lanzador.Lanzar(o, func(f FinProceso) { b.enviar(evento{fin: &f}) })
			if err != nil {
				b.m.Termino(FinProceso{ID: o.ID, Code: -1, Ultima: strings.TrimPrefix(err.Error(), "hoom: ")}, b.e.Ahora())
			}
		case AbrirEditor:
			archivo := filepath.Join(b.root, filepath.FromSlash(o.Archivo))
			if err := b.conEditor(func() error { return b.e.Editor.Abrir(archivo, o.Linea) }); err != nil {
				return true, 1, err
			}
		case EditarPedido:
			var texto string
			var errEd error
			if err := b.conEditor(func() error {
				texto, errEd = b.e.Editor.Editar(o.Texto)
				return nil
			}); err != nil {
				return true, 1, err
			}
			b.m.PedidoEditado(texto, errEd)
		}
	}
	return false, 0, nil
}

// conEditor gives the terminal to the editor while f runs and takes it
// again after. Only an error of the terminal is returned.
func (b *bucle) conEditor(f func() error) error {
	p, _ := b.e.Terminal.(pausable)
	if p != nil {
		p.Pausar()
	}
	if err := b.soltar(); err != nil {
		return err
	}
	_ = f()
	if err := b.tomar(); err != nil {
		return err
	}
	if p != nil {
		p.Reanudar()
	}
	b.mirarTamano()
	return nil
}

// mirarTamano asks the terminal's size and tells the model when it
// changed.
func (b *bucle) mirarTamano() {
	w, h, err := b.e.Terminal.Tamano()
	if err != nil || (w == b.m.c.Ancho && h == b.m.c.Alto) {
		return
	}
	b.m.Tamano(w, h)
}

// dibujar writes the screen when it changed.
func (b *bucle) dibujar() error {
	lineas := b.m.Pantalla()
	var sb strings.Builder
	sb.WriteString("\x1b[H")
	for i, l := range lineas {
		if i > 0 {
			sb.WriteString("\r\n")
		}
		sb.WriteString(l)
		sb.WriteString("\x1b[K")
	}
	frame := sb.String()
	if frame == b.ultimo {
		return nil
	}
	if _, err := io.WriteString(b.e.Terminal, frame); err != nil {
		return err
	}
	b.ultimo = frame
	return nil
}
