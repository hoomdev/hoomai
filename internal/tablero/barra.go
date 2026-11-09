package tablero

import "time"

// barra is the bar's message and the end notices waiting for it. Its
// protocol, in its three moves:
//
//   - dar: a message given now takes the bar for its time. An end notice it
//     takes the bar from is suspended: it goes back, whole, to the head of
//     the queue.
//   - encolarFin: an end notice waits at the end of the queue.
//   - avanzar: the end notice at the head takes the bar when the bar is
//     free or shows a message given before it; never from another end
//     notice nor from a message given after it.
//
// seq is the order in which the messages were given: none is lost, and
// none takes the bar from a newer one.
type barra struct {
	actual aviso     // the message the bar shows
	hasta  time.Time // until when it shows
	fin    bool      // actual is an end notice
	cola   []aviso   // the end notices waiting, oldest first
	seq    int       // the order of the last message given
}

// aviso is a message of the bar; seq is when it was given.
type aviso struct {
	texto  string
	estilo estilo
	seq    int
}

// mostrando says whether the bar shows a message at ahora.
func (b *barra) mostrando(ahora time.Time) bool {
	return b.actual.texto != "" && ahora.Before(b.hasta)
}

// dar puts texto in the bar from ahora for d.
func (b *barra) dar(texto string, e estilo, ahora time.Time, d time.Duration) {
	if b.fin && b.mostrando(ahora) {
		b.cola = append([]aviso{b.actual}, b.cola...)
	}
	b.seq++
	b.actual, b.hasta, b.fin = aviso{texto: texto, estilo: e, seq: b.seq}, ahora.Add(d), false
}

// encolarFin puts an end notice at the end of the queue.
func (b *barra) encolarFin(texto string, e estilo) {
	b.seq++
	b.cola = append(b.cola, aviso{texto: texto, estilo: e, seq: b.seq})
}

// avanzar puts the end notice at the head in the bar, for DuracionFin, when
// its turn came.
func (b *barra) avanzar(ahora time.Time) {
	if len(b.cola) == 0 {
		return
	}
	if b.mostrando(ahora) && (b.fin || b.actual.seq > b.cola[0].seq) {
		return
	}
	b.actual, b.hasta, b.fin = b.cola[0], ahora.Add(DuracionFin), true
	b.cola = b.cola[1:]
}
