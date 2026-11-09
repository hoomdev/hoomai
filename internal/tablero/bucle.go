package tablero

import (
	"fmt"
	"io"
	"path/filepath"
	"strconv"
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
	proyecto  *Proyecto
	hallazgos *Hallazgos
	archivo   *Archivo
	fin       *FinProceso
	panico    any
	hayPanico bool
	// what kind of read ended
	tipo int
}

// The kinds of reads, each at most one at a time.
const (
	leeTablero = iota
	leeDetalle
	leeProyecto
	leeArchivo
	tiposLectura
)

// bucle is one run of the TUI.
type bucle struct {
	e     Entorno
	root  string
	cfg   Config
	mouse bool
	m     *Modelo

	restaurar func() error
	tomada    bool
	ultimo    string // the last frame written

	eventos chan evento
	listo   chan struct{}

	enVuelo   [tiposLectura]bool
	pendiente [tiposLectura]bool // read again when the one in flight ends
	leida     string             // the Elegida of the last Detalle asked
	leidoArch string             // the ArchivoElegido of the last Archivo asked
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

// tomar puts the terminal in raw mode, enters the alternate screen and asks
// for the mouse, in one write.
func (b *bucle) tomar() error {
	restaurar, err := b.e.Terminal.Crudo()
	if err != nil {
		return err
	}
	b.restaurar, b.tomada = restaurar, true
	b.ultimo = ""
	sec := SecEntrar
	if b.mouse {
		sec += SecMouseSi
	}
	if _, err = io.WriteString(b.e.Terminal, sec); err != nil {
		// the terminal back as it was before returning: no caller is left
		// holding a raw terminal it does not know it took
		_ = b.soltar()
		return err
	}
	return nil
}

// soltar gives the mouse back, leaves the alternate screen, in one write,
// and puts the terminal back.
func (b *bucle) soltar() error {
	if !b.tomada {
		return nil
	}
	b.tomada = false
	sec := SecSalir
	if b.mouse {
		sec = SecMouseNo + SecSalir
	}
	_, errW := io.WriteString(b.e.Terminal, sec)
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

// leer starts a read of the kind tipo in another goroutine, or, when one of
// that kind is in flight, leaves it pending.
func (b *bucle) leer(tipo int) {
	if b.enVuelo[tipo] {
		b.pendiente[tipo] = true
		return
	}
	var f func() evento
	switch tipo {
	case leeTablero:
		f = func() evento {
			tab, err := b.e.Datos.Tablero()
			return evento{lectura: &Lectura{Tablero: tab, Err: err, Fijada: b.e.Datos.Fijada()}}
		}
	case leeDetalle:
		slug := b.m.Elegida()
		b.leida = slug
		if slug == "" {
			return
		}
		f = func() evento {
			d := b.e.Datos.Detalle(slug)
			if d.Slug == "" {
				d.Slug = slug
			}
			return evento{detalle: &d}
		}
	case leeProyecto:
		f = func() evento {
			p := b.e.Datos.Proyecto()
			h := b.e.Datos.Hallazgos()
			return evento{proyecto: &p, hallazgos: &h}
		}
	case leeArchivo:
		ruta, linea := b.m.ArchivoElegido()
		b.leidoArch = ruta + "\x00" + strconv.Itoa(linea)
		if ruta == "" {
			return
		}
		f = func() evento {
			a := b.e.Datos.Archivo(ruta, linea)
			a.Ruta, a.Linea = ruta, linea
			return evento{archivo: &a}
		}
	}
	b.enVuelo[tipo] = true
	go func() {
		defer b.reenviar()
		ev := f()
		ev.tipo = tipo
		b.enviar(ev)
	}()
}

func (b *bucle) bucle() (int, error) {
	b.eventos = make(chan evento, 16)
	b.listo = make(chan struct{})
	defer close(b.listo)

	tic := time.NewTicker(cadaTic)
	defer tic.Stop()
	// the next read of the board (and of the project) is its interval after
	// the last one ended: a read that takes longer than its interval is not
	// read without rest
	var proxima, proximaProyecto <-chan time.Time
	winch, parar := avisosDeTamano()
	defer parar()
	senales, pararSenales := avisosDeSalida()
	defer pararSenales()

	go b.leerTeclas()
	if err := b.dibujar(); err != nil {
		return 1, err
	}
	b.leer(leeTablero)
	b.leer(leeProyecto)

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
				fin, code, err = b.entradas(pendiente)
				pendiente = nil
			case ev.fin != nil:
				b.m.Termino(*ev.fin, b.e.Ahora())
				b.leer(leeProyecto)
			default:
				b.enVuelo[ev.tipo] = false
				switch {
				case ev.lectura != nil:
					l := *ev.lectura
					l.Ahora = b.e.Ahora()
					fin, code, err = b.ordenes(b.m.Leer(l))
					proxima = time.After(b.e.Intervalo)
					b.leer(leeDetalle)
				case ev.detalle != nil:
					b.m.LeerDetalle(*ev.detalle)
				case ev.proyecto != nil:
					b.m.LeerProyecto(*ev.proyecto)
					b.m.LeerHallazgos(*ev.hallazgos)
					proximaProyecto = time.After(b.e.IntervaloProyecto)
				case ev.archivo != nil:
					b.m.LeerArchivo(*ev.archivo)
				}
				if b.pendiente[ev.tipo] {
					b.pendiente[ev.tipo] = false
					b.leer(ev.tipo)
				}
			}
		case <-espera:
			espera = nil
			// a mouse event cut by the read waits for its tail: it never
			// turns into keys
			resto := []byte(nil)
			if i := prefijoRaton(pendiente); i >= 0 {
				resto = append(resto, pendiente[i:]...)
				pendiente = pendiente[:i]
			}
			fin, code, err = b.entradas(pendiente)
			pendiente = resto
		case <-tic.C:
			b.m.Tic(b.e.Ahora())
			b.mirarTamano()
		case <-winch:
			b.mirarTamano()
		case s := <-senales:
			// as Salir: correr gives the terminal back (the mouse off, the
			// normal screen) before Run returns
			return codigoDeSenal(s), nil
		case <-proxima:
			proxima = nil
			b.leer(leeTablero)
		case <-proximaProyecto:
			proximaProyecto = nil
			b.leer(leeProyecto)
		}
		if fin || err != nil {
			return code, err
		}
		// what [0] shows changed: read it now
		if b.m.Elegida() != b.leida {
			b.leer(leeDetalle)
		}
		if ruta, linea := b.m.ArchivoElegido(); ruta+"\x00"+strconv.Itoa(linea) != b.leidoArch {
			b.leer(leeArchivo)
		}
		if err := b.dibujar(); err != nil {
			return 1, err
		}
	}
}

// entradas gives the keys and the mouse events to the model and carries
// out its orders.
func (b *bucle) entradas(buf []byte) (bool, int, error) {
	for _, e := range DecodificarEntradas(buf) {
		var ords []Orden
		if e.Raton != nil {
			ords = b.m.Raton(*e.Raton, b.e.Ahora())
		} else {
			ords = b.m.Tecla(e.Tecla, b.e.Ahora())
		}
		if fin, code, err := b.ordenes(ords); fin || err != nil {
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
				b.leer(leeProyecto)
			}
		case AbrirEditor:
			// the model looked at the path's form; through a symlink of the
			// project it may still leave it
			if fuera, err := fueraPorLosEnlaces(b.root, filepath.FromSlash(o.Archivo)); fuera || err != nil {
				msg := "el archivo " + strconv.Quote(o.Archivo) + " esta fuera del proyecto"
				if !fuera {
					msg = "no se pudo abrir el editor: " + err.Error()
				}
				b.m.avisar(msg, rojo, DuracionAviso)
				continue
			}
			archivo := filepath.Join(b.root, filepath.FromSlash(o.Archivo))
			var errEd error
			if err := b.conEditor(func() error {
				errEd = b.e.Editor.Abrir(archivo, o.Linea)
				return nil
			}); err != nil {
				return true, 1, err
			}
			if errEd != nil {
				// the clock moved while the editor had the terminal
				b.m.Tic(b.e.Ahora())
				b.m.avisar("no se pudo abrir el editor: "+strings.TrimPrefix(errEd.Error(), "hoom: "), rojo, DuracionAviso)
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

// sinSGR is a line of Pantalla without its SGR sequences.
func sinSGR(l string) string {
	var b strings.Builder
	for i := 0; i < len(l); i++ {
		if l[i] == 0x1b && i+1 < len(l) && l[i+1] == '[' {
			j := i + 2
			for j < len(l) && l[j] != 'm' {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(l[i])
	}
	return b.String()
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
		// after the last column EL would clear that very cell (the cursor
		// stays on it): a line that fills the width has nothing to clear
		if Celdas(sinSGR(l)) < b.m.c.Ancho {
			sb.WriteString("\x1b[K")
		}
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
