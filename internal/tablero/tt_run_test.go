// Run (CA-516, CA-522, CA-527, CA-528, CA-530, CA-533): el borde del
// proceso con una terminal falsa (ningun pty, ninguna terminal real), datos,
// lanzador y editor falsos. Run corre en su goroutine; cada espera tiene su
// tiempo maximo.
package tablero_test

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/tablero"
)

const ttYAML = "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

// ttRegistro es lo que paso en el borde, en orden: la terminal (crudo,
// restaurar, escribe), Out, Err y el editor.
type ttRegistro struct {
	mu      sync.Mutex
	eventos []ttSuceso
}

type ttSuceso struct {
	que  string // crudo, restaurar, escribe, out, err, abrir, editar
	dato string
}

func (r *ttRegistro) anotar(que, dato string) {
	r.mu.Lock()
	r.eventos = append(r.eventos, ttSuceso{que, dato})
	r.mu.Unlock()
}

func (r *ttRegistro) copia() []ttSuceso {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]ttSuceso(nil), r.eventos...)
}

// texto es todo lo que se escribio con que (escribe, out o err), junto.
func (r *ttRegistro) texto(que string) string {
	var b strings.Builder
	for _, e := range r.copia() {
		if e.que == que {
			b.WriteString(e.dato)
		}
	}
	return b.String()
}

// indice es la posicion del primer evento que cumple f desde i, o -1.
func (r *ttRegistro) indice(desde int, f func(ttSuceso) bool) int {
	es := r.copia()
	for i := desde; i < len(es); i++ {
		if f(es[i]) {
			return i
		}
	}
	return -1
}

// ttEscritor anota en el registro lo que se le escribe.
type ttEscritor struct {
	r   *ttRegistro
	que string
}

func (w ttEscritor) Write(p []byte) (int, error) {
	w.r.anotar(w.que, string(p))
	return len(p), nil
}

// ttTerm es la terminal falsa: las teclas llegan por un canal, lo escrito va
// al registro, Tamano se cambia desde el test.
type ttTerm struct {
	r        *ttRegistro
	teclas   chan []byte
	cerrar   sync.Once
	pend     []byte
	finErr   error // lo que da Read con el canal cerrado (nil = io.EOF)
	mu       sync.Mutex
	ancho    int
	alto     int
	escErr   error // Write falla con este error despues de escOK escrituras
	escOK    int
	escritas int
}

func ttNuevaTerm(r *ttRegistro, ancho, alto int) *ttTerm {
	return &ttTerm{r: r, teclas: make(chan []byte, 64), ancho: ancho, alto: alto}
}

func (f *ttTerm) Read(p []byte) (int, error) {
	if len(f.pend) == 0 {
		b, ok := <-f.teclas
		if !ok {
			if f.finErr != nil {
				return 0, f.finErr
			}
			return 0, io.EOF
		}
		f.pend = b
	}
	n := copy(p, f.pend)
	f.pend = f.pend[n:]
	return n, nil
}

func (f *ttTerm) Write(p []byte) (int, error) {
	f.mu.Lock()
	f.escritas++
	falla := f.escErr != nil && f.escritas > f.escOK
	f.mu.Unlock()
	if falla {
		return 0, f.escErr
	}
	f.r.anotar("escribe", string(p))
	return len(p), nil
}

func (f *ttTerm) Crudo() (func() error, error) {
	f.r.anotar("crudo", "")
	return func() error { f.r.anotar("restaurar", ""); return nil }, nil
}

func (f *ttTerm) Tamano() (int, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.ancho, f.alto, nil
}

func (f *ttTerm) cambiar(ancho, alto int) {
	f.mu.Lock()
	f.ancho, f.alto = ancho, alto
	f.mu.Unlock()
}

func (f *ttTerm) mandar(s string) { f.teclas <- []byte(s) }

func (f *ttTerm) cortar() { f.cerrar.Do(func() { close(f.teclas) }) }

// ttDatos es el tablero falso.
type ttDatos struct {
	mu        sync.Mutex
	tablero   boardcmd.Board
	err       error
	panico    bool
	lecturas  int
	detalles  []string // "<slug> <historia>"
	detalleDe func(slug string) tablero.Detalle
}

func (d *ttDatos) Tablero() (boardcmd.Board, error) {
	d.mu.Lock()
	d.lecturas++
	p := d.panico
	b, err := d.tablero, d.err
	d.mu.Unlock()
	if p {
		panic("boom en los datos")
	}
	return b, err
}

func (d *ttDatos) Fijada() string { return "" }

func (d *ttDatos) Detalle(slug string, historia bool) tablero.Detalle {
	d.mu.Lock()
	d.detalles = append(d.detalles, fmt.Sprintf("%s %v", slug, historia))
	f := d.detalleDe
	d.mu.Unlock()
	if f != nil {
		return f(slug)
	}
	return tablero.Detalle{Slug: slug}
}

func (d *ttDatos) cuantas() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.lecturas
}

func (d *ttDatos) pedidos() []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]string(nil), d.detalles...)
}

// ttLanz es el lanzador falso.
type ttLanz struct {
	mu      sync.Mutex
	ordenes []tablero.Lanzar
	err     error
	fin     *tablero.FinProceso // si no es nil, se llama a fin enseguida, desde otra goroutine
}

func (l *ttLanz) Lanzar(o tablero.Lanzar, fin func(tablero.FinProceso)) error {
	l.mu.Lock()
	l.ordenes = append(l.ordenes, o)
	err, f := l.err, l.fin
	l.mu.Unlock()
	if err != nil {
		return err
	}
	if f != nil {
		x := *f
		x.ID = o.ID
		go fin(x)
	}
	return nil
}

func (l *ttLanz) lanzadas() []tablero.Lanzar {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]tablero.Lanzar(nil), l.ordenes...)
}

// ttEd es el editor falso: anota cuando se abre.
type ttEd struct {
	r      *ttRegistro
	editar string
	err    error
}

func (e *ttEd) Abrir(archivo string, linea int) error {
	e.r.anotar("abrir", fmt.Sprintf("%s:%d", archivo, linea))
	return e.err
}

func (e *ttEd) Editar(texto string) (string, error) {
	e.r.anotar("editar", texto)
	return e.editar, e.err
}

// ttMundo es todo lo de un Run.
type ttMundo struct {
	r     *ttRegistro
	term  *ttTerm
	datos *ttDatos
	lanz  *ttLanz
	ed    *ttEd
	e     tablero.Entorno
	listo atomic.Bool // Run ya termino
}

func ttNuevoMundo(t *testing.T, b boardcmd.Board) *ttMundo {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(ttYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := manifest.Load(dir, profiles.Resolve); err != nil {
		t.Fatalf("el hoom.yaml de prueba es valido: %v", err)
	}
	r := &ttRegistro{}
	w := &ttMundo{r: r, term: ttNuevaTerm(r, 80, 24), datos: &ttDatos{tablero: b}, lanz: &ttLanz{}, ed: &ttEd{r: r}}
	env := map[string]string{"NO_COLOR": "1", "HOOM_ASCII": "1", "LANG": "C"}
	w.e = tablero.Entorno{
		Dir: dir, Out: ttEscritor{r, "out"}, Err: ttEscritor{r, "err"},
		Getenv: func(k string) string { return env[k] }, GOOS: "linux",
		Terminal: w.term, Datos: w.datos, Lanzador: w.lanz, Editor: w.ed,
		Ahora: func() time.Time { return ttAhora }, Intervalo: 10 * time.Millisecond,
	}
	t.Cleanup(w.term.cortar)
	return w
}

// correr lanza Run en su goroutine; fin espera su codigo (o su panic).
func (w *ttMundo) correr(args []string) (fin func(t *testing.T) (int, any)) {
	type res struct {
		code int
		pan  any
	}
	ch := make(chan res, 1)
	go func() {
		var code int
		defer func() {
			if p := recover(); p != nil {
				ch <- res{-99, p}
			}
		}()
		defer w.listo.Store(true)
		code = tablero.Run(args, w.e)
		ch <- res{code, nil}
	}()
	return func(t *testing.T) (int, any) {
		t.Helper()
		select {
		case x := <-ch:
			return x.code, x.pan
		case <-time.After(10 * time.Second):
			w.term.cortar()
			t.Fatalf("Run no termino en 10s; escribio:\n%q", w.r.texto("escribe"))
			return 0, nil
		}
	}
}

// ttEsperar espera hasta 5s a que cond se cumpla.
func ttEsperar(t *testing.T, ca, que string, w *ttMundo, cond func() bool) {
	t.Helper()
	limite := time.Now().Add(5 * time.Second)
	for !cond() {
		if w.listo.Load() && !cond() {
			t.Fatalf("%s: Run termino antes de que pasara: %s; la terminal recibio:\n%q", ca, que, ttCola(w.r.texto("escribe"), 3000))
		}
		if time.Now().After(limite) {
			t.Fatalf("%s: no paso en 5s: %s; la terminal recibio:\n%q", ca, que, ttCola(w.r.texto("escribe"), 3000))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func ttCola(s string, n int) string {
	if len(s) > n {
		return "..." + s[len(s)-n:]
	}
	return s
}

// ttMarcos son los cuadros que Run escribio: lo que sigue a cada "\x1b[H"
// (fuera de SecSalir), cada uno como sus lineas sin el "\x1b[K" final.
func ttMarcos(salida string) [][]string {
	salida = strings.ReplaceAll(salida, tablero.SecSalir, "\x00")
	salida = strings.ReplaceAll(salida, tablero.SecEntrar, "\x00")
	var out [][]string
	for _, tramo := range strings.Split(salida, "\x00") {
		partes := strings.Split(tramo, "\x1b[H")
		for _, p := range partes[1:] {
			var ls []string
			for _, l := range strings.Split(p, "\r\n") {
				ls = append(ls, strings.TrimSuffix(l, "\x1b[K"))
			}
			out = append(out, ls)
		}
	}
	return out
}

// ultimoMarco es el ultimo cuadro completo (Alto lineas que terminan en
// "\x1b[K").
func (w *ttMundo) ultimoMarco() []string {
	ms := ttMarcos(w.r.texto("escribe"))
	for i := len(ms) - 1; i >= 0; i-- {
		if len(ms[i]) >= 15 {
			return ms[i]
		}
	}
	return nil
}

func (w *ttMundo) marcoCon(s string) bool {
	return strings.Contains(strings.Join(w.ultimoMarco(), "\n"), s)
}

// filaElegida dice si el ultimo cuadro (sin las SGR del color) tiene, al
// principio de una linea, la fila de la tarjeta slug elegida en la lista:
// " > " + slug + " " (el ▸ en ASCII, y HOOM_ASCII=1 en todos los mundos de
// Run; el slug relleno a 24 celdas, o entero y seguido del " " de la tira).
//
// Toda espera que deja pasar una tecla sobre el tablero (enter que abre la
// tarjeta, w que abre su formulario) espera esto, y no un texto cualquiera:
// antes de la primera lectura Run ya dibujo un cuadro (doc.go, "The list": el
// cuerpo dice " leyendo el tablero...", el titulo "0 tarjetas" y la barra
// "sin tarjeta fijada"), y el contrato no dice que una tecla mandada antes de
// la primera lectura la espere. Un texto como "tarjeta" ya esta en ese primer
// cuadro: la tecla saldria sobre una lista vacia y no haria nada. La fila
// elegida solo la muestra el tablero leido, con la seleccion ya en la tarjeta
// ("At the start it is on the first card line"), que es sobre lo que actuan
// enter y las teclas de accion. Es mas estricta que marcoCon(slug): un cuadro
// con esa fila tiene el slug.
func (w *ttMundo) filaElegida(slug string) bool {
	for _, l := range w.ultimoMarco() {
		if strings.HasPrefix(ttSGR.ReplaceAllString(l, ""), " > "+slug+" ") {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- CA-516

// CA-516: un flag o un operando: exit 2 con el bloque de uso en Err, sin
// tocar la terminal; -h y --help: el uso en Out y exit 0.
func TestCA516_RunArgs(t *testing.T) {
	for _, args := range [][]string{{"--json"}, {"infra-api"}, {"-x", "y"}} {
		w := ttNuevoMundo(t, ttTablero())
		code, _ := w.correr(args)(t)
		if code != 2 {
			t.Fatalf("CA-516: Run(%q) sale con 2, salio con %d", args, code)
		}
		if !strings.Contains(w.r.texto("err"), tablero.UsageText) {
			t.Fatalf("CA-516: Run(%q) escribe el bloque de uso en Err: %q", args, w.r.texto("err"))
		}
		for _, e := range w.r.copia() {
			if e.que == "crudo" || e.que == "escribe" || e.que == "out" {
				t.Fatalf("CA-516: Run(%q) no toca la terminal ni escribe en Out: %v", args, w.r.copia())
			}
		}
	}
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		w := ttNuevoMundo(t, ttTablero())
		code, _ := w.correr(args)(t)
		if code != 0 || !strings.Contains(w.r.texto("out"), tablero.UsageText) {
			t.Fatalf("CA-516: Run(%q) sale con 0 con el uso en Out: %d, %q", args, code, w.r.texto("out"))
		}
		for _, e := range w.r.copia() {
			if e.que == "crudo" || e.que == "escribe" {
				t.Fatalf("CA-516: Run(%q) no toca la terminal", args)
			}
		}
	}
}

// CA-516, caso limite: sin terminal (hoom tablero > archivo, un pipe): exit
// 2 con MsgSinTerminal en Err y nada mas en ninguna parte; los args se miran
// antes, y la terminal antes que el hoom.yaml.
func TestCA516_RunSinTerminal(t *testing.T) {
	w := ttNuevoMundo(t, ttTablero())
	w.e.Terminal = nil
	code, _ := w.correr(nil)(t)
	if code != 2 || !strings.Contains(w.r.texto("err"), tablero.MsgSinTerminal) || w.r.texto("out") != "" {
		t.Fatalf("CA-516: sin terminal: exit 2 y %q en Err, Out vacio: %d, err %q, out %q", tablero.MsgSinTerminal, code, w.r.texto("err"), w.r.texto("out"))
	}
	if w.datos.cuantas() != 0 {
		t.Fatalf("CA-516: sin terminal no se lee el tablero")
	}
	// un operando sin terminal: es el error de uso
	w = ttNuevoMundo(t, ttTablero())
	w.e.Terminal = nil
	code, _ = w.correr([]string{"x"})(t)
	if code != 2 || !strings.Contains(w.r.texto("err"), tablero.UsageText) || strings.Contains(w.r.texto("err"), tablero.MsgSinTerminal) {
		t.Fatalf("CA-516: los args se miran antes que la terminal: %d, %q", code, w.r.texto("err"))
	}
	// sin terminal y con un hoom.yaml invalido: es el de la terminal
	w = ttNuevoMundo(t, ttTablero())
	w.e.Terminal = nil
	if err := os.WriteFile(filepath.Join(w.e.Dir, "hoom.yaml"), []byte("schema: hoom/v2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, _ = w.correr(nil)(t)
	if code != 2 || !strings.Contains(w.r.texto("err"), tablero.MsgSinTerminal) {
		t.Fatalf("CA-516: la terminal se mira antes que el hoom.yaml: %d, %q", code, w.r.texto("err"))
	}
}

// CA-516: un hoom.yaml invalido (o ninguno): exit 1 con "hoom: <error de
// manifest.Load>" en Err, antes de tocar la terminal.
func TestCA516_RunHoomYAMLInvalido(t *testing.T) {
	for nombre, yaml := range map[string]string{"schema v2": "schema: hoom/v2\nproject: demo\n", "yaml roto": "schema: [\n", "sin hoom.yaml": ""} {
		w := ttNuevoMundo(t, ttTablero())
		ruta := filepath.Join(w.e.Dir, "hoom.yaml")
		if yaml == "" {
			if err := os.Remove(ruta); err != nil {
				t.Fatal(err)
			}
		} else if err := os.WriteFile(ruta, []byte(yaml), 0o644); err != nil {
			t.Fatal(err)
		}
		_, lerr := manifest.Load(w.e.Dir, profiles.Resolve)
		if lerr == nil {
			t.Fatalf("el hoom.yaml %q es invalido para manifest.Load", nombre)
		}
		code, _ := w.correr(nil)(t)
		if code != 1 {
			t.Fatalf("CA-516: con %s Run sale con 1, salio con %d", nombre, code)
		}
		if got := strings.TrimRight(w.r.texto("err"), "\n"); got != "hoom: "+lerr.Error() {
			t.Fatalf("CA-516: con %s Err es 'hoom: <error>':\n got %q\nwant %q", nombre, got, "hoom: "+lerr.Error())
		}
		for _, e := range w.r.copia() {
			if e.que == "crudo" || e.que == "escribe" {
				t.Fatalf("CA-516: con %s no se toca la terminal: %v", nombre, w.r.copia())
			}
		}
	}
}

// ---------------------------------------------------------------- CA-533

// ttRevisarDevuelta revisa que la terminal se tomo y se devolvio: crudo
// antes de escribir, SecEntrar al principio y una sola vez, SecSalir al
// final y restaurar despues de la ultima escritura.
func ttRevisarDevuelta(t *testing.T, ca, que string, w *ttMundo) {
	t.Helper()
	es := w.r.copia()
	primeraEsc, ultimaEsc, crudo, rest := -1, -1, -1, -1
	for i, e := range es {
		switch e.que {
		case "escribe":
			if primeraEsc < 0 {
				primeraEsc = i
			}
			ultimaEsc = i
		case "crudo":
			if crudo < 0 {
				crudo = i
			}
		case "restaurar":
			rest = i
		}
	}
	if crudo < 0 || primeraEsc < 0 || crudo > primeraEsc {
		t.Fatalf("%s (%s): Crudo antes de escribir en la terminal: %v", ca, que, es)
	}
	salida := w.r.texto("escribe")
	if !strings.HasPrefix(salida, tablero.SecEntrar) || strings.Count(salida, tablero.SecEntrar) != 1 {
		t.Fatalf("%s (%s): SecEntrar una vez, al principio: %q", ca, que, ttCola(salida, 200))
	}
	if !strings.HasSuffix(salida, tablero.SecSalir) || strings.Count(salida, tablero.SecSalir) != 1 {
		t.Fatalf("%s (%s): SecSalir una vez, al final: %q", ca, que, ttCola(salida, 200))
	}
	if rest < ultimaEsc {
		t.Fatalf("%s (%s): restaurar despues de la ultima escritura: %v", ca, que, es)
	}
}

// CA-533: q y ctrl-c salen con 0 y devuelven la terminal como estaba.
func TestCA533_SalirConQYCtrlC(t *testing.T) {
	for _, tecla := range []string{"q", "\x03"} {
		w := ttNuevoMundo(t, ttTableroPrincipal())
		fin := w.correr(nil)
		ttEsperar(t, "CA-533", "el primer cuadro", w, func() bool { return w.ultimoMarco() != nil })
		w.term.mandar(tecla)
		code, pan := fin(t)
		if code != 0 || pan != nil {
			t.Fatalf("CA-533: %q sale con 0: %d (panic %v)", tecla, code, pan)
		}
		ttRevisarDevuelta(t, "CA-533", fmt.Sprintf("%q", tecla), w)
		if w.r.texto("out") != "" || w.r.texto("err") != "" {
			t.Fatalf("CA-533: sin trabajos corriendo no hay mensaje de salida: out %q err %q", w.r.texto("out"), w.r.texto("err"))
		}
	}
}

// CA-533: cada cuadro es "\x1b[H" y las lineas de Pantalla, cada una con
// "\x1b[K", unidas por "\r\n": el ultimo es la pantalla del modelo con la
// misma configuracion (proyecto del hoom.yaml, tamano de la terminal, sin
// color con NO_COLOR, ASCII sin locale UTF-8).
func TestCA533_ElCuadroEsLaPantallaDelModelo(t *testing.T) {
	b := ttTableroPrincipal()
	w := ttNuevoMundo(t, b)
	fin := w.correr(nil)
	m := tablero.Nuevo(tablero.Config{Proyecto: "demo", Ancho: 80, Alto: 24})
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: b})
	quiere := m.Pantalla()
	ttEsperar(t, "CA-533", "un cuadro con el tablero", w, func() bool {
		return strings.Join(w.ultimoMarco(), "\n") == strings.Join(quiere, "\n")
	})
	salida := w.r.texto("escribe")
	cuadro := "\x1b[H" + strings.Join(quiere, "\x1b[K\r\n") + "\x1b[K"
	if !strings.Contains(salida, cuadro) {
		t.Fatalf("CA-533: el cuadro va entero, con \\x1b[K en cada linea y \\r\\n entre lineas")
	}
	w.term.mandar("q")
	fin(t)
}

// CA-533: redibuja al cambiar el tamano (se pregunta al menos cada 250 ms):
// por debajo de 60x15, "agranda la terminal (minimo 60x15)"; de vuelta, el
// tablero.
func TestCA533_RedibujaAlCambiarElTamano(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	fin := w.correr(nil)
	ttEsperar(t, "CA-533", "el primer cuadro", w, func() bool { return w.marcoCon("hoom · demo") })
	w.term.cambiar(50, 10)
	limite := time.Now().Add(2 * time.Second)
	for !strings.Contains(w.r.texto("escribe"), "\x1b[Hagranda la terminal (minimo 60x15)\x1b[K") {
		if time.Now().After(limite) {
			t.Fatalf("CA-533: a 50x10 se redibuja 'agranda la terminal (minimo 60x15)' en menos de 2s: %q", ttCola(w.r.texto("escribe"), 500))
		}
		time.Sleep(5 * time.Millisecond)
	}
	antes := len(w.r.texto("escribe"))
	w.term.cambiar(100, 30)
	ttEsperar(t, "CA-533", "el tablero a 100x30", w, func() bool {
		s := w.r.texto("escribe")
		ms := ttMarcos(s[antes:])
		return len(ms) > 0 && len(ms[len(ms)-1]) == 30 && strings.HasPrefix(ms[len(ms)-1][0], " hoom · demo")
	})
	w.term.mandar("q")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-533: q sale con 0, salio con %d", code)
	}
}

// CA-533: un error al leer la terminal (y su fin de archivo) sale con 1,
// devuelve la terminal y despues escribe "hoom: <error>" en Err.
func TestCA533_ErrorDeLaTerminal(t *testing.T) {
	for nombre, ferr := range map[string]error{"error": errors.New("se corto la terminal"), "eof": nil} {
		w := ttNuevoMundo(t, ttTableroPrincipal())
		w.term.finErr = ferr
		fin := w.correr(nil)
		ttEsperar(t, "CA-533", "el primer cuadro", w, func() bool { return w.ultimoMarco() != nil })
		w.term.cortar()
		code, pan := fin(t)
		if code != 1 || pan != nil {
			t.Fatalf("CA-533: con %s Run sale con 1: %d (%v)", nombre, code, pan)
		}
		ttRevisarDevuelta(t, "CA-533", nombre, w)
		quiere := "hoom: EOF"
		if ferr != nil {
			quiere = "hoom: " + ferr.Error()
		}
		if got := strings.TrimRight(w.r.texto("err"), "\n"); got != quiere {
			t.Fatalf("CA-533: con %s Err es %q, es %q", nombre, quiere, got)
		}
		rest := w.r.indice(0, func(e ttSuceso) bool { return e.que == "restaurar" })
		errI := w.r.indice(0, func(e ttSuceso) bool { return e.que == "err" })
		if errI < rest {
			t.Fatalf("CA-533: el error se escribe despues de devolver la terminal: %v", w.r.copia())
		}
	}
}

// CA-533: un error al escribir en la terminal tambien sale con 1 y la
// devuelve (restaurar).
func TestCA533_ErrorAlEscribir(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	w.term.escErr, w.term.escOK = errors.New("terminal cerrada"), 1
	fin := w.correr(nil)
	ttEsperar(t, "CA-533", "la primera escritura", w, func() bool {
		return w.r.indice(0, func(e ttSuceso) bool { return e.que == "escribe" }) >= 0
	})
	time.Sleep(50 * time.Millisecond)
	w.term.mandar("j") // fuerza otro cuadro, que ya no se puede escribir
	code, pan := fin(t)
	if code != 1 || pan != nil {
		t.Fatalf("CA-533: un error al escribir sale con 1: %d (%v)", code, pan)
	}
	if w.r.indice(0, func(e ttSuceso) bool { return e.que == "restaurar" }) < 0 {
		t.Fatalf("CA-533: con un error al escribir la terminal se devuelve: %v", w.r.copia())
	}
	if got := w.r.texto("err"); !strings.HasPrefix(got, "hoom: ") || !strings.Contains(got, "terminal cerrada") {
		t.Fatalf("CA-533: Err dice 'hoom: <error>': %q", got)
	}
}

// CA-533: un panic en lo que Run llama (los datos) devuelve la terminal y
// el panic sigue en la goroutine de Run.
func TestCA533_PanicDevuelveLaTerminal(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	w.datos.panico = true
	_, pan := w.correr(nil)(t)
	if pan == nil || !strings.Contains(fmt.Sprint(pan), "boom en los datos") {
		t.Fatalf("CA-533: el panic sigue en la goroutine de Run: %v", pan)
	}
	salida := w.r.texto("escribe")
	if !strings.HasPrefix(salida, tablero.SecEntrar) || !strings.HasSuffix(salida, tablero.SecSalir) {
		t.Fatalf("CA-533: con un panic la terminal se toma y se devuelve: %q", ttCola(salida, 200))
	}
	es := w.r.copia()
	if len(es) == 0 || es[len(es)-1].que != "restaurar" {
		t.Fatalf("CA-533: con un panic lo ultimo es restaurar la terminal: %v", es)
	}
}

// ---------------------------------------------------------------- CA-530

// CA-530: el tablero se vuelve a leer cada Intervalo, y el detalle de la
// tarjeta abierta con cada lectura (la Historia solo con su pestana).
func TestCA530_RunReleeElTablero(t *testing.T) {
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(ttAccionWriter())))
	fin := w.correr(nil)
	ttEsperar(t, "CA-530", "cinco lecturas con Intervalo 10ms", w, func() bool { return w.datos.cuantas() >= 5 })
	if len(w.datos.pedidos()) != 0 {
		t.Fatalf("CA-530: sin tarjeta abierta no se lee ningun detalle: %v", w.datos.pedidos())
	}
	ttEsperar(t, "CA-530", "la tarjeta elegida en la lista", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-530", "el detalle de la tarjeta abierta", w, func() bool {
		ps := w.datos.pedidos()
		return len(ps) >= 2 && ps[len(ps)-1] == "tarjeta false"
	})
	w.term.mandar("5")
	ttEsperar(t, "CA-530", "el detalle con la Historia", w, func() bool {
		ps := w.datos.pedidos()
		return len(ps) > 0 && ps[len(ps)-1] == "tarjeta true"
	})
	w.term.mandar("q")
	fin(t)
}

// CA-530: una lectura que falla deja la ultima pantalla con "no se pudo leer
// el tablero: <error>".
func TestCA530_RunLecturaQueFalla(t *testing.T) {
	w := ttNuevoMundo(t, ttTableroPrincipal())
	fin := w.correr(nil)
	ttEsperar(t, "CA-530", "el tablero", w, func() bool { return w.marcoCon("9 tarjetas") })
	w.datos.mu.Lock()
	w.datos.err = errors.New("disco roto")
	w.datos.mu.Unlock()
	ttEsperar(t, "CA-530", "el error de lectura en la barra", w, func() bool {
		return w.marcoCon("no se pudo leer el tablero: disco roto") && w.marcoCon("9 tarjetas")
	})
	w.term.mandar("q")
	fin(t)
}

// ---------------------------------------------------------------- CA-528

// CA-528: lanzar desde la TUI le pasa al Lanzador la orden exacta; al salir,
// MensajeSalida dice cuantos trabajos quedan corriendo (despues de devolver
// la terminal).
func TestCA528_RunLanzaYAlSalirDiceCuantosQuedan(t *testing.T) {
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(ttAccionWriter())))
	fin := w.correr(nil)
	ttEsperar(t, "CA-528", "la tarjeta en la lista", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("w")
	ttEsperar(t, "CA-528", "el formulario", w, func() bool { return w.marcoCon("-> hoom pedir writer tarjeta") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-528", "la orden al lanzador", w, func() bool { return len(w.lanz.lanzadas()) == 1 })
	l := w.lanz.lanzadas()[0]
	ttMismosArgs(t, "CA-528", "Run", l.Args, []string{"pedir", "writer", "tarjeta", "-m", strings.TrimSpace(ttPedidoWriter)})
	if l.ID != 1 || l.Rol != "writer" || l.Slug != "tarjeta" {
		t.Fatalf("CA-528: la orden lleva su ID, rol y tarjeta: %#v", l)
	}
	ttEsperar(t, "CA-528", "lanzando en la barra", w, func() bool { return w.marcoCon("lanzando hoom pedir writer tarjeta") })
	w.term.mandar("q")
	code, _ := fin(t)
	if code != 0 {
		t.Fatalf("CA-528: q sale con 0, salio con %d", code)
	}
	if got := strings.TrimRight(w.r.texto("out"), "\n"); got != "queda 1 trabajo corriendo: hoom status" {
		t.Fatalf("CA-528: al salir dice cuantos quedan: %q", got)
	}
	rest := w.r.indice(0, func(e ttSuceso) bool { return e.que == "restaurar" })
	outI := w.r.indice(0, func(e ttSuceso) bool { return e.que == "out" })
	if outI < rest {
		t.Fatalf("CA-528: el mensaje de salida va despues de devolver la terminal: %v", w.r.copia())
	}
}

// CA-528, caso limite: el proceso termina antes de su primer registro (el
// candado de la capa 1): la barra lo dice y ya no cuenta al salir.
func TestCA528_RunTerminaAntesDelRegistro(t *testing.T) {
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(ttAccionWriter())))
	w.lanz.fin = &tablero.FinProceso{Code: 1, Ultima: "ya se esta lanzando un trabajo en esta tarjeta"}
	fin := w.correr(nil)
	ttEsperar(t, "CA-528", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("w")
	ttEsperar(t, "CA-528", "el formulario", w, func() bool { return w.marcoCon("-> hoom pedir writer tarjeta") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-528", "el error del candado en la barra", w, func() bool {
		return w.marcoCon("ya se esta lanzando un trabajo en esta tarjeta")
	})
	w.term.mandar("q")
	fin(t)
	if got := w.r.texto("out"); got != "" {
		t.Fatalf("CA-528: lo que termino no cuenta al salir: %q", got)
	}
}

// CA-528: un Lanzador que no puede arrancar el proceso: la barra dice su
// error y nada queda corriendo.
func TestCA528_RunNoPudoArrancar(t *testing.T) {
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(ttAccionWriter())))
	w.lanz.err = errors.New("exec: hoom no encontrado")
	fin := w.correr(nil)
	ttEsperar(t, "CA-528", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("w")
	ttEsperar(t, "CA-528", "el formulario", w, func() bool { return w.marcoCon("-> hoom pedir writer tarjeta") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-528", "el error del arranque en la barra", w, func() bool { return w.marcoCon("exec: hoom no encontrado") })
	w.term.mandar("q")
	fin(t)
	if got := w.r.texto("out"); got != "" {
		t.Fatalf("CA-528: lo que no arranco no cuenta al salir: %q", got)
	}
}

// ---------------------------------------------------------------- el editor

// CA-522: o abre el editor con la ruta absoluta (Dir + la del spec) y la
// linea; mientras esta abierto la terminal se devuelve (SecSalir,
// restaurar) y despues se toma de nuevo (Crudo, SecEntrar).
func TestCA522_RunAbreElEditorYDevuelveLaTerminal(t *testing.T) {
	c := ttTarjetaCon(ttAccionWriter())
	w := ttNuevoMundo(t, ttTablero(c))
	w.datos.detalleDe = func(slug string) tablero.Detalle {
		return tablero.Detalle{Slug: slug, Detalle: boardcmd.Detail{Card: c,
			Spec: boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: "# Spec\n\nuna linea\n"}}}
	}
	fin := w.correr(nil)
	ttEsperar(t, "CA-522", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-522", "la tarjeta abierta", w, func() bool { return w.marcoCon("[1 Resumen]") })
	w.term.mandar("2")
	ttEsperar(t, "CA-522", "la pestana Spec", w, func() bool { return w.marcoCon("una linea") })
	desde := len(w.r.copia())
	w.term.mandar("o")
	ttEsperar(t, "CA-522", "el editor", w, func() bool {
		return w.r.indice(desde, func(e ttSuceso) bool { return e.que == "abrir" }) >= 0
	})
	abrir := w.r.indice(desde, func(e ttSuceso) bool { return e.que == "abrir" })
	if got, quiere := w.r.copia()[abrir].dato, filepath.Join(w.e.Dir, ".hoom/specs/tarjeta.md")+":1"; got != quiere {
		t.Fatalf("CA-522: o abre el spec con su ruta absoluta y la linea: %q, se esperaba %q", got, quiere)
	}
	salir := w.r.indice(desde, func(e ttSuceso) bool { return e.que == "escribe" && strings.Contains(e.dato, tablero.SecSalir) })
	rest := w.r.indice(desde, func(e ttSuceso) bool { return e.que == "restaurar" })
	if salir < 0 || rest < 0 || !(salir < rest && rest < abrir) {
		t.Fatalf("CA-522: antes del editor, SecSalir y restaurar: %v", w.r.copia()[desde:])
	}
	ttEsperar(t, "CA-522", "la terminal tomada de nuevo", w, func() bool {
		cr := w.r.indice(abrir, func(e ttSuceso) bool { return e.que == "crudo" })
		en := w.r.indice(abrir, func(e ttSuceso) bool { return e.que == "escribe" && strings.Contains(e.dato, tablero.SecEntrar) })
		return cr > abrir && en > cr
	})
	w.term.mandar("q")
	if code, _ := fin(t); code != 0 {
		t.Fatalf("CA-522: q sale con 0, salio con %d", code)
	}
}

// CA-527: ctrl-e (0x05) en el formulario abre el pedido en el editor (con la
// terminal devuelta) y lo que da el editor es el pedido que se lanza.
func TestCA527_RunCtrlEEditaElPedido(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "pedido original"
	w := ttNuevoMundo(t, ttTablero(ttTarjetaCon(a)))
	w.ed.editar = "pedido del editor\n"
	fin := w.correr(nil)
	ttEsperar(t, "CA-527", "la tarjeta", w, func() bool { return w.filaElegida("tarjeta") })
	w.term.mandar("w")
	ttEsperar(t, "CA-527", "el formulario", w, func() bool { return w.marcoCon("-> hoom pedir writer tarjeta") })
	desde := len(w.r.copia())
	w.term.mandar("\x05")
	ttEsperar(t, "CA-527", "el editor del pedido", w, func() bool {
		return w.r.indice(desde, func(e ttSuceso) bool { return e.que == "editar" }) >= 0
	})
	ed := w.r.indice(desde, func(e ttSuceso) bool { return e.que == "editar" })
	if got := w.r.copia()[ed].dato; got != "pedido original" {
		t.Fatalf("CA-527: el editor recibe el pedido: %q", got)
	}
	if rest := w.r.indice(desde, func(e ttSuceso) bool { return e.que == "restaurar" }); rest < 0 || rest > ed {
		t.Fatalf("CA-527: mientras el editor esta abierto la terminal esta devuelta: %v", w.r.copia()[desde:])
	}
	ttEsperar(t, "CA-527", "el pedido editado en el formulario", w, func() bool { return w.marcoCon("pedido del editor") })
	w.term.mandar("\r")
	ttEsperar(t, "CA-527", "la orden", w, func() bool { return len(w.lanz.lanzadas()) == 1 })
	ttMismosArgs(t, "CA-527", "el pedido del editor", w.lanz.lanzadas()[0].Args, []string{"pedir", "writer", "tarjeta", "-m", "pedido del editor"})
	w.term.mandar("q")
	fin(t)
}
