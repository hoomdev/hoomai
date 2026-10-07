//go:build unix

// Tests de regresion de la review cruzada de la cabina (C1, 2026-09-24),
// hallazgo 20260924T200412_151300 sobre `hoom serve`: el launch respondia
// 202 "el sobre esta corriendo" apoyado en un Started que se llamaba aunque
// el primer registro del sobre NO estuviera en disco (CA-335). Sin ese
// registro la tarjeta no tiene fantasma ni sobre que mostrar: el launch
// responde 409 JSON, nunca 202, y no arranca ningun run.
//
// El caso se arma parando al Studio en un FIFO (h3Alto), y un FIFO es de
// unix: en windows este archivo no se compila, asi que ahi estos tests no
// existen (ni fallan ni se saltean a la vista) y el resto del paquete
// compila igual. El CI corre en ubuntu.
package servecmd

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/envelope"
)

// h3IDSobre es la forma de un id de sobre (la de un run): 20260924T200412_151300.
var h3IDSobre = regexp.MustCompile(`\d{8}T\d{6}_[0-9a-f]{6}`)

// h3Tope es lo que el alto espera por cada cosa que no depende de el: que
// alguien abra el contrato del rol para leerlo y que el que lo leyo lo suelte.
const h3Tope = 30 * time.Second

// h3Pausa es cada cuanto el alto se fija si ya hay alguien leyendo el
// contrato. No es una ventana que ganar: el que lee se queda parado hasta que
// el alto le escribe, asi que mirar mas tarde solo lo hace esperar mas.
const h3Pausa = 500 * time.Microsecond

// Por que el alto no armo el caso. Son fallas del FIXTURE (el launch ya no
// pasa por donde el alto lo espera), no respuestas del Studio que el test mida.
var (
	errH3NadieLeyo = errors.New("nadie abrio el contrato del rol para leerlo")
	errH3SinID     = errors.New("cuando se leyo el contrato del rol no habia ningun id de sobre nuevo en .hoom/envelopes/")
	errH3VariosIDs = errors.New("cuando se leyo el contrato del rol habia mas de un id de sobre nuevo en .hoom/envelopes/")
	errH3YaEscrito = errors.New("cuando se leyo el contrato del rol el primer registro del sobre ya estaba escrito")
	errH3NoPlanto  = errors.New("no se pudo plantar la trampa donde va el primer registro del sobre")
	errH3NoSolto   = errors.New("el que leyo el contrato del rol no lo solto")
	errH3NoTermino = errors.New("el alto no termino")
)

// h3Modo es lo que el alto hace con el Studio parado en el contrato del rol.
type h3Modo int

const (
	h3SinTrampa h3Modo = iota // mira que id de sobre hay y deja seguir (el control)
	h3ConTrampa               // ademas planta la trampa donde va el primer registro
)

// h3Visto es lo que el alto vio e hizo con el Studio parado.
type h3Visto struct {
	id     string // el id de sobre nuevo en .hoom/envelopes/: el del launch parado
	trampa string // el directorio no vacio plantado en <id>.json ("" si no se planto)
	err    error  // por que no se armo el caso; nil si se armo
}

// h3Alto para al Studio en un punto exacto del launch de pedir-writer, sin
// tocar produccion. El launch (1) crea <id>.log en .hoom/envelopes/, (2) lee
// entero, bloqueando, el contrato del rol <espacio de trabajo>/.hoom/agents/
// 04-writer.md y (3) escribe el primer registro <id>.json. El contrato es un
// FIFO sin datos: el que lo lee se queda parado hasta que le escriban. Con el
// Studio parado ahi el id ya esta en disco y el registro todavia no: el alto
// mira el id, planta (o no) la trampa, deja un archivo comun con el contrato
// de verdad donde estaba el FIFO y le escribe ese mismo contrato al que
// esperaba. Un solo intento y ninguna ventana que ganar: los hallazgos
// 20261005T182842_7964af y 20261005T183301_d5e760 eran la carrera de la
// trampa anterior, que miraba el directorio esperando ganarle al registro.
//
// El alto NO abre el FIFO bloqueando para enterarse de que llego el lector.
// Tiene la punta de escritura abierta desde antes de poner el FIFO en la ruta
// del contrato, y se entera probando: abrir para escribir sin esperar da
// ENXIO mientras nadie lo tenga abierto para leer (h3Tocar). Asi ninguna
// apertura duerme en el nucleo: ni la del alto ni la del Studio, que al abrir
// ya encuentra un escritor. Una apertura dormida no es un punto de encuentro
// confiable en macOS: si al hilo que duerme en ella le llega una senal cuando
// el otro acaba de abrir, el que lee recibe fin de archivo con CERO bytes (un
// contrato vacio) y el que escribe sigue dormido, como si no hubiera venido
// nadie (la apertura se deshace y vuelve a empezar, y por un momento el FIFO
// no tiene escritor). Medido el 2026-10-06 en el test del alto, 16 procesos a
// la vez: abriendo bloqueando, 2 contratos vacios en 80.000 corridas y 132 en
// 8.000 mandandole SIGURG al proceso; sosteniendo la punta de escritura y
// probando, ninguno en 40.000 ni en 8.000 con las mismas senales.
//
// Atiende UNA lectura: el contrato se escribe una vez, y despues la ruta es
// un archivo comun y nadie mas llega al FIFO por ella.
type h3Alto struct {
	contrato string // lo que lee el Studio: <espacio de trabajo>/.hoom/agents/04-writer.md
	fifo     string // el mismo FIFO por otro nombre (enlace duro), fuera del arbol
	regular  string // el contrato de verdad en un archivo comun, listo para ocupar contrato
	texto    []byte // el contrato de verdad: el embebido del rol
	envs     string
	previos  map[string]bool // los ids de sobre que ya estaban en envs
	modo     h3Modo
	tope     time.Duration

	escritura int           // la punta de escritura del FIFO, abierta desde que se arma
	basta     chan struct{} // cerrar: no se espera mas a un lector
	fin       chan struct{} // atender termino; visto ya no cambia
	visto     h3Visto       // lo escribe atender antes de cerrar fin
	cierre    sync.Once
	informe   h3Visto // lo que devuelve cerrar
}

// h3PararEnElContrato arma el alto en el contrato del writer del espacio de
// trabajo wt, mirando los sobres de envs. Lo cierra el test (cerrar dice lo
// que vio) y, si no, el final del test.
func h3PararEnElContrato(t *testing.T, ca, wt, envs string, modo h3Modo, tope time.Duration) *h3Alto {
	t.Helper()
	rol, err := agents.Lookup("writer")
	if err != nil {
		t.Fatalf("%s: fixture: el rol writer: %v", ca, err)
	}
	texto, err := agents.Embedded(rol)
	if err != nil || texto == "" {
		t.Fatalf("%s: fixture: el contrato embebido del writer: %q, %v", ca, texto, err)
	}
	previos, err := h3IDsEn(envs)
	if err != nil {
		t.Fatalf("%s: fixture: los sobres que ya estaban: %v", ca, err)
	}
	aparte := t.TempDir() // en el mismo disco que wt: el enlace duro y el rename lo necesitan
	a := &h3Alto{
		contrato: filepath.Join(wt, ".hoom", "agents", rol.File),
		fifo:     filepath.Join(aparte, "contrato.fifo"),
		regular:  filepath.Join(aparte, "contrato.md"),
		texto:    []byte(texto),
		envs:     envs,
		previos:  previos,
		modo:     modo,
		tope:     tope,
		basta:    make(chan struct{}),
		fin:      make(chan struct{}),
	}
	armar := func() error {
		if err := os.MkdirAll(filepath.Dir(a.contrato), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(a.regular, a.texto, 0o644); err != nil {
			return err
		}
		if err := syscall.Mkfifo(a.fifo, 0o600); err != nil {
			return fmt.Errorf("mkfifo %s: %w", a.fifo, err)
		}
		// La punta de escritura se abre sin esperar, y eso pide un lector:
		// una punta de lectura pasajera, que se cierra enseguida.
		lectura, err := h3AbrirSinEsperar(a.fifo, syscall.O_RDONLY)
		if err != nil {
			return fmt.Errorf("abrir el FIFO para leer: %w", err)
		}
		a.escritura, err = h3AbrirSinEsperar(a.fifo, syscall.O_WRONLY)
		syscall.Close(lectura)
		if err != nil {
			return fmt.Errorf("abrir el FIFO para escribir: %w", err)
		}
		// Recien ahora, con su escritor, el FIFO aparece en la ruta del
		// contrato. El enlace duro falla si el espacio de trabajo ya trae su
		// contrato: el alto no pisa uno que no puso.
		if err := os.Link(a.fifo, a.contrato); err != nil {
			syscall.Close(a.escritura)
			return err
		}
		return nil
	}
	if err := armar(); err != nil {
		t.Fatalf("%s: fixture: armar el alto en %s: %v", ca, a.contrato, err)
	}
	go a.atender()
	t.Cleanup(func() { a.cerrar() })
	return a
}

// atender es la goroutine del alto: espera al que lee el contrato, mira (y
// planta) con el parado, y lo deja seguir con el contrato de verdad.
func (a *h3Alto) atender() {
	defer close(a.fin)
	v := &a.visto

	hayLector := false
	if v.err = a.esperarLector(); v.err == nil {
		// El Studio esta parado en el contrato: el id ya esta, el registro no.
		hayLector = true
		v.id, v.err = h3IDNuevo(a.envs, a.previos)
		if v.err == nil && a.modo == h3ConTrampa {
			v.trampa, v.err = h3Plantar(a.envs, v.id)
		}
	}

	// Lo que sigue se hace SIEMPRE, tambien si el caso no se armo: nadie se
	// queda parado en el FIFO. Primero el archivo comun (el que lea despues
	// no espera a nadie) y despues el contrato para el que estaba esperando.
	if err := a.ocupar(); err != nil && v.err == nil {
		v.err = err
	}
	if !hayLector {
		// Se dejo de esperar. Si alguien abrio el FIFO justo antes de que la
		// ruta cambiara, tambien recibe su contrato.
		hayLector, _ = h3Tocar(a.fifo)
	}
	if hayLector {
		if err := h3Escribir(a.escritura, a.texto, a.tope); err != nil && v.err == nil {
			v.err = fmt.Errorf("escribirle el contrato del rol al que lo leia: %w", err)
		}
	}
	syscall.Close(a.escritura) // el ultimo escritor: fin de archivo para el que leia
	if err := h3TocarHastaQueLoSuelten(a.fifo, a.tope); err != nil && v.err == nil {
		v.err = err
	}
}

// esperarLector vuelve con nil cuando alguien tiene el contrato abierto para
// leerlo: como el FIFO no tiene datos y si un escritor (el alto), ese lector
// esta parado o por pararse en su lectura. Vuelve con errH3NadieLeyo si
// cerrar dijo basta o si paso el tope sin que llegara nadie.
func (a *h3Alto) esperarLector() (err error) {
	pausa := time.NewTicker(h3Pausa)
	defer pausa.Stop()
	limite := time.Now().Add(a.tope)
	for {
		hayLector, err := h3Tocar(a.fifo)
		if err != nil {
			return err
		}
		if hayLector {
			return nil
		}
		select {
		case <-a.basta:
			return errH3NadieLeyo
		case <-pausa.C:
		}
		if time.Now().After(limite) {
			return fmt.Errorf("%w en %s", errH3NadieLeyo, a.tope)
		}
	}
}

// ocupar pone, con un rename, el archivo comun con el contrato de verdad
// donde estaba el FIFO: la ruta no falta nunca y el FIFO sigue alcanzable por
// su otro nombre. Si el rename falla saca el FIFO de la ruta: sin copia en el
// espacio de trabajo el contrato que vale es el embebido, que es el mismo texto.
func (a *h3Alto) ocupar() error {
	err := os.Rename(a.regular, a.contrato)
	if err != nil {
		os.Remove(a.contrato)
		return fmt.Errorf("dejar el contrato del rol como archivo comun: %w", err)
	}
	return nil
}

// cerrar termina el alto y dice lo que vio. Se puede llamar las veces que
// haga falta (tambien corre al final del test): la primera hace el trabajo y
// todas devuelven lo mismo. Si nadie leyo el contrato no espera mas: el
// contrato queda como archivo comun y nada queda bloqueado en el FIFO.
func (a *h3Alto) cerrar() h3Visto {
	a.cierre.Do(func() {
		close(a.basta)
		select {
		case <-a.fin:
			a.informe = a.visto
			os.Remove(a.fifo)
		case <-time.After(2 * a.tope): // uno para escribir y otro para que lo suelten
			a.informe = h3Visto{err: fmt.Errorf("%w en %s", errH3NoTermino, 2*a.tope)}
		}
	})
	return a.informe
}

// h3IDsEn son los ids de sobre que tienen algo en dir (.hoom/envelopes/): el
// log <id>.log, el registro <id>.json o un temporal de ese registro.
func h3IDsEn(dir string) (ids map[string]bool, err error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	ids = map[string]bool{}
	for _, e := range entries {
		if m := h3IDSobre.FindString(e.Name()); m != "" {
			ids[m] = true
		}
	}
	return ids, nil
}

// h3IDNuevo MIRA: devuelve el id de sobre que tiene algo en dir y no estaba
// en previos. Con el Studio parado en el contrato es el del launch en curso,
// asi que tiene que haber uno solo. No escribe nada (hallazgo
// 20261005T183447_6df35b: mirar y plantar eran una sola funcion con un bool).
func h3IDNuevo(dir string, previos map[string]bool) (id string, err error) {
	ahora, err := h3IDsEn(dir)
	if err != nil {
		return "", fmt.Errorf("mirar los sobres: %w", err)
	}
	var nuevos []string
	for m := range ahora {
		if !previos[m] {
			nuevos = append(nuevos, m)
		}
	}
	sort.Strings(nuevos)
	switch len(nuevos) {
	case 0:
		return "", errH3SinID
	case 1:
		return nuevos[0], nil
	}
	return "", fmt.Errorf("%w: %v", errH3VariosIDs, nuevos)
}

// h3Plantar PLANTA la trampa: un DIRECTORIO no vacio donde va el primer
// registro del sobre id (dir/<id>.json), que asi ya no se puede escribir.
// Devuelve la ruta de la trampa. Si ahi ya hay algo no lo toca: el registro
// se escribio antes de leer el contrato y el caso no se arma.
func h3Plantar(dir, id string) (trampa string, err error) {
	p := filepath.Join(dir, id+".json")
	if _, err := os.Lstat(p); err == nil {
		return "", fmt.Errorf("%w: %s", errH3YaEscrito, p)
	}
	if err := os.Mkdir(p, 0o755); err != nil {
		return "", fmt.Errorf("%w: %w", errH3NoPlanto, err)
	}
	if err := os.WriteFile(filepath.Join(p, "ocupado"), []byte("x\n"), 0o644); err != nil {
		return "", fmt.Errorf("%w: %w", errH3NoPlanto, err)
	}
	return p, nil
}

// h3AbrirSinEsperar abre una punta del FIFO (modo es O_RDONLY u O_WRONLY)
// sin bloquear. El descriptor es crudo y con O_CLOEXEC: no pasa por el poller
// del runtime y no lo hereda un proceso hijo (un git o un CLI falso con el
// FIFO abierto contaria como lector o como escritor).
func h3AbrirSinEsperar(fifo string, modo int) (fd int, err error) {
	for {
		fd, err = syscall.Open(fifo, modo|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != syscall.EINTR {
			return fd, err
		}
	}
}

// h3Tocar toca el FIFO: lo abre para escribir sin esperar y lo cierra sin
// escribir nada. Dice si alguien lo tiene abierto para LEER: sin lector esa
// apertura da ENXIO. Al que esta leyendo no le cambia nada mientras el alto
// tenga su punta de escritura; cuando ya no queda escritor, el cierre del
// toque le vuelve a avisar el fin de archivo (h3TocarHastaQueLoSuelten).
func h3Tocar(fifo string) (hayLector bool, err error) {
	fd, err := h3AbrirSinEsperar(fifo, syscall.O_WRONLY)
	if err == syscall.ENXIO {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("tocar el FIFO del contrato del rol: %w", err)
	}
	syscall.Close(fd)
	return true, nil
}

// h3Escribir escribe b entero en la punta de escritura fd, que no bloquea:
// si el FIFO se llena espera a que el lector lo vacie, hasta tope.
func h3Escribir(fd int, b []byte, tope time.Duration) (err error) {
	limite := time.Now().Add(tope)
	for len(b) > 0 {
		n, err := syscall.Write(fd, b)
		switch {
		case err == nil:
			b = b[n:]
		case err == syscall.EINTR:
		case err == syscall.EAGAIN && time.Now().Before(limite):
			time.Sleep(time.Millisecond)
		default:
			return err
		}
	}
	return nil
}

// h3TocarHastaQueLoSuelten corre cuando el contrato ya se escribio ENTERO y
// la punta de escritura se cerro: toca el FIFO cada milisegundo hasta que
// nadie lo tiene abierto para leer, con tope como limite.
//
// Hace falta en macOS, donde el que lee un FIFO bloqueando puede PERDER el
// fin de archivo del ultimo cierre: medido el 2026-10-05, un lector que lee
// hasta el final y un escritor que escribe 9 bytes y cierra se colgaron 7
// veces en 200.000 vueltas, siempre con los 9 bytes ya leidos y el lector
// bloqueado para siempre en la lectura siguiente; y el 2026-10-06, en el test
// del alto con 16 procesos a la vez, 6 lecturas colgadas en 40.000 corridas
// sin este toque y ninguna en 40.000 con el. Otro escritor que abre y cierra
// sin escribir vuelve a avisar el fin de archivo y suelta al lector con todos
// sus datos. No le recorta nada: el toque no escribe, y con datos pendientes
// en el FIFO la lectura devuelve datos, no fin de archivo. Donde el fin de
// archivo no se pierde el toque sobra y no molesta: da ENXIO en cuanto el
// lector cerro.
func h3TocarHastaQueLoSuelten(fifo string, tope time.Duration) (err error) {
	limite := time.Now().Add(tope)
	for {
		hayLector, err := h3Tocar(fifo)
		if err != nil || !hayLector {
			return err
		}
		if time.Now().After(limite) {
			return fmt.Errorf("%w en %s", errH3NoSolto, tope)
		}
		time.Sleep(time.Millisecond)
	}
}

// h3Launch hace el launch con el alto armado y devuelve su respuesta. El
// launch corre en otra goroutine: si se cuelga (parado en el FIFO o en otra
// cosa) el test falla con un mensaje del fixture en vez de quedarse hasta el
// timeout de go test. El limite es tres topes: uno para que el launch llegue
// al contrato, otro para que lo suelte y otro para responder.
func h3Launch(t *testing.T, ca string, alto *h3Alto, launch func() *httptest.ResponseRecorder) *httptest.ResponseRecorder {
	t.Helper()
	vuelta := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		var rec *httptest.ResponseRecorder
		defer func() { vuelta <- rec }() // nil si launch corto sin responder
		rec = launch()
	}()
	select {
	case rec := <-vuelta:
		if rec == nil {
			t.Fatalf("%s: fixture: el launch corto sin dar una respuesta", ca)
		}
		return rec
	case <-time.After(3 * alto.tope):
		t.Fatalf("%s: fixture: el launch no volvio en %s (el alto: %v)", ca, 3*alto.tope, alto.cerrar().err)
		return nil
	}
}

// h3Metas lista los sidecars .hoom/runs/*.meta.json de cada dir.
func h3Metas(t *testing.T, dirs ...string) []string {
	t.Helper()
	var out []string
	for _, d := range dirs {
		m, err := filepath.Glob(filepath.Join(d, ".hoom", "runs", "*.meta.json"))
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, m...)
	}
	return out
}

// h3Estudio: el proyecto con su .hoom/envelopes/, el CLI falso de claude (que
// termina solo) y el Studio.
func h3Estudio(t *testing.T) (f *lnFakes, root, envs string, s *Server) {
	t.Helper()
	f = lnPATH(t)
	f.cli(t, "claude", false)
	root = tbProyecto(t)
	envs = filepath.Join(root, ".hoom", envelope.DirName)
	if err := os.MkdirAll(envs, 0o755); err != nil {
		t.Fatal(err)
	}
	return f, root, envs, lnServidor(t, root, f)
}

// Hallazgo 20260924T200412_151300 (CA-335, CA-338): POST
// /api/board/{slug}/launch de pedir-writer cuando el primer registro del
// sobre no se puede escribir (un directorio en .hoom/envelopes/<id>.json)
// responde 409 JSON, nunca 202; el CLI de la IA no se invoca y no aparece
// ningun .hoom/runs/*.meta.json. La trampa se planta con el Studio parado en
// el contrato del rol (h3Alto): un solo intento, sin carrera.
func TestHallazgo_151300_LaunchSinPrimerRegistroEs409(t *testing.T) {
	const ca = "CA-335"
	f, root, envs, s := h3Estudio(t)

	const slug = "sin-registro"
	wt := lnCartaWriter(t, ca, root, slug, "")
	llamadasAntes := len(f.llamadas(t, "claude"))
	metasAntes := h3Metas(t, root, wt)

	alto := h3PararEnElContrato(t, ca, wt, envs, h3ConTrampa, h3Tope)
	rec := h3Launch(t, ca, alto, func() *httptest.ResponseRecorder {
		return lnLaunch(t, s, slug, s.Token(), lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec hasta que verify de verde."))
	})
	visto := alto.cerrar()
	if visto.err != nil {
		t.Fatalf("%s: fixture: la trampa no se planto: %v (respuesta %d: %s)", ca, visto.err, rec.Code, rec.Body.String())
	}
	id := visto.id

	if rec.Code == http.StatusAccepted {
		t.Fatalf("%s: hallazgo 151300: sin primer registro del sobre %s el launch nunca responde 202: %s", ca, id, rec.Body.String())
	}
	tbErrorJSON(t, ca, rec, http.StatusConflict)
	lnEsperarLanzamientos(t, ca, s)
	if n := len(f.llamadas(t, "claude")); n != llamadasAntes {
		t.Fatalf("%s: hallazgo 151300: sin primer registro no arranca ningun run: claude se invoco %d veces", ca, n-llamadasAntes)
	}
	if m := h3Metas(t, root, wt); len(m) != len(metasAntes) {
		t.Fatalf("%s: hallazgo 151300: sin primer registro no aparece ningun .hoom/runs/*.meta.json: antes %v, despues %v", ca, metasAntes, m)
	}
	for _, r := range envelope.List(root) {
		if r.ID == id {
			t.Fatalf("%s: no queda registro del sobre %s: %+v", ca, id, r)
		}
	}
}

// Control del hallazgo 151300 (CA-335): el MISMO alto en el contrato del
// rol, sin plantar nada, y el launch responde 202 con el id de sobre que el
// alto vio con el Studio parado, y su primer registro esta en disco. El 409
// del test de arriba sale entonces del registro que no se pudo escribir, no
// de haber parado al Studio en un FIFO, y el id que el alto mira es el del
// sobre de ese launch.
func TestHallazgo_151300_ControlElMismoAltoSinTrampaEs202(t *testing.T) {
	const ca = "CA-335"
	_, root, envs, s := h3Estudio(t)

	const slug = "con-registro"
	wt := lnCartaWriter(t, ca, root, slug, "")

	alto := h3PararEnElContrato(t, ca, wt, envs, h3SinTrampa, h3Tope)
	rec := h3Launch(t, ca, alto, func() *httptest.ResponseRecorder {
		return lnLaunch(t, s, slug, s.Token(), lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec hasta que verify de verde."))
	})
	visto := alto.cerrar()
	if visto.err != nil {
		t.Fatalf("%s: fixture: el alto no paro al launch: %v (respuesta %d: %s)", ca, visto.err, rec.Code, rec.Body.String())
	}

	r := lnAceptado(t, ca, rec)
	if r.EnvelopeID != visto.id {
		t.Fatalf("%s: control: el 202 nombra el sobre que estaba en .hoom/envelopes/ con el Studio parado en el contrato (%s), nombro %s", ca, visto.id, r.EnvelopeID)
	}
	// el registro ya esta en disco cuando llega el 202
	if reg := lnRegistro(t, ca, root, visto.id); reg.ID != visto.id {
		t.Fatalf("%s: control: el primer registro del sobre %s es el suyo: %+v", ca, visto.id, reg)
	}
	lnEsperarLanzamientos(t, ca, s)
}

// El alto del hallazgo 151300 (CA-335) probado a solas, sin Studio: el que
// lee es agents.Contract, que busca el contrato del rol en el espacio de
// trabajo. En todos los casos el que lee recibe el contrato embebido entero y
// nada queda bloqueado; lo que cambia es lo que el alto informa.
func TestHallazgo_151300_ElAltoEnElContratoDelRol(t *testing.T) {
	const ca = "CA-335"
	const viejo, nuevo, otro = "20260924T200412_151300", "20261005T183301_d5e760", "20261005T183447_6df35b"
	rol, err := agents.Lookup("writer")
	if err != nil {
		t.Fatal(err)
	}
	embebido, err := agents.Embedded(rol)
	if err != nil {
		t.Fatal(err)
	}

	// escena: un espacio de trabajo vacio y unos sobres con el registro de
	// uno viejo, que el alto tiene que ignorar.
	escena := func(t *testing.T) (wt, envs string) {
		t.Helper()
		wt, envs = t.TempDir(), t.TempDir()
		tbEscribir(t, envs, viejo+".json", "{}\n")
		tbEscribir(t, envs, viejo+".log", "")
		return wt, envs
	}
	// leer lee el contrato como el envelope (entero, bloqueando) y exige el
	// embebido completo. Con tope: una lectura colgada es una falla, no un cuelgue.
	leer := func(t *testing.T, que, wt string) {
		t.Helper()
		type leido struct {
			texto string
			err   error
		}
		ch := make(chan leido, 1)
		go func() {
			texto, err := agents.Contract(wt, rol)
			ch <- leido{texto, err}
		}()
		select {
		case l := <-ch:
			if l.err != nil || l.texto != embebido {
				t.Fatalf("%s: %s: el que lee recibe el contrato embebido entero (%d bytes): recibio %d bytes, err %v", ca, que, len(embebido), len(l.texto), l.err)
			}
		case <-time.After(h3Tope):
			t.Fatalf("%s: %s: la lectura del contrato del rol no volvio en %s", ca, que, h3Tope)
		}
	}
	// despues: el alto cerrado deja el contrato como archivo comun (el que
	// lea despues no espera a nadie), su FIFO borrado y la goroutine terminada;
	// cerrar otra vez devuelve lo mismo.
	despues := func(t *testing.T, a *h3Alto, wt string, visto h3Visto) {
		t.Helper()
		select {
		case <-a.fin:
		default:
			t.Fatalf("%s: despues de cerrar la goroutine del alto termino", ca)
		}
		if st, err := os.Lstat(a.contrato); err != nil || !st.Mode().IsRegular() {
			t.Fatalf("%s: despues de cerrar el contrato es un archivo comun: %v, %v", ca, st, err)
		}
		if _, err := os.Lstat(a.fifo); !os.IsNotExist(err) {
			t.Fatalf("%s: despues de cerrar el FIFO del alto no esta: %v", ca, err)
		}
		leer(t, "despues de cerrar", wt)
		if otra := a.cerrar(); otra != visto {
			t.Fatalf("%s: cerrar otra vez devuelve lo mismo: %+v y %+v", ca, visto, otra)
		}
	}

	t.Run("un lector con id nuevo: lo para, planta y le da el contrato", func(t *testing.T) {
		wt, envs := escena(t)
		a := h3PararEnElContrato(t, ca, wt, envs, h3ConTrampa, h3Tope)
		tbEscribir(t, envs, nuevo+".log", "")
		leer(t, "con el alto armado", wt)
		visto := a.cerrar()
		if visto.err != nil || visto.id != nuevo || visto.trampa != filepath.Join(envs, nuevo+".json") {
			t.Fatalf("%s: el alto vio el id nuevo %s y planto en su registro: %+v", ca, nuevo, visto)
		}
		if adentro, err := os.ReadDir(visto.trampa); err != nil || len(adentro) == 0 {
			t.Fatalf("%s: la trampa es un directorio no vacio: %v, %v", ca, adentro, err)
		}
		if st, err := os.Lstat(filepath.Join(envs, viejo+".json")); err != nil || !st.Mode().IsRegular() {
			t.Fatalf("%s: el registro del sobre viejo no se toca: %v, %v", ca, st, err)
		}
		despues(t, a, wt, visto)
	})

	t.Run("sin trampa: lo para, mira el id y no planta", func(t *testing.T) {
		wt, envs := escena(t)
		a := h3PararEnElContrato(t, ca, wt, envs, h3SinTrampa, h3Tope)
		tbEscribir(t, envs, nuevo+".log", "")
		leer(t, "con el alto armado", wt)
		visto := a.cerrar()
		if visto.err != nil || visto.id != nuevo || visto.trampa != "" {
			t.Fatalf("%s: el alto vio el id nuevo %s y no planto: %+v", ca, nuevo, visto)
		}
		if _, err := os.Lstat(filepath.Join(envs, nuevo+".json")); !os.IsNotExist(err) {
			t.Fatalf("%s: sin trampa no hay nada donde va el registro: %v", ca, err)
		}
		despues(t, a, wt, visto)
	})

	t.Run("un lector sin id nuevo", func(t *testing.T) {
		wt, envs := escena(t)
		a := h3PararEnElContrato(t, ca, wt, envs, h3ConTrampa, h3Tope)
		leer(t, "con el alto armado", wt)
		visto := a.cerrar()
		if !errors.Is(visto.err, errH3SinID) || visto.id != "" || visto.trampa != "" {
			t.Fatalf("%s: sin id nuevo el alto lo dice y no planta: %+v", ca, visto)
		}
		despues(t, a, wt, visto)
	})

	t.Run("un lector con dos ids nuevos", func(t *testing.T) {
		wt, envs := escena(t)
		a := h3PararEnElContrato(t, ca, wt, envs, h3ConTrampa, h3Tope)
		tbEscribir(t, envs, nuevo+".log", "")
		tbEscribir(t, envs, otro+".log", "")
		leer(t, "con el alto armado", wt)
		visto := a.cerrar()
		if !errors.Is(visto.err, errH3VariosIDs) || visto.id != "" || visto.trampa != "" {
			t.Fatalf("%s: con dos ids nuevos el alto no elige: %+v", ca, visto)
		}
		despues(t, a, wt, visto)
	})

	t.Run("el registro ya estaba escrito", func(t *testing.T) {
		wt, envs := escena(t)
		a := h3PararEnElContrato(t, ca, wt, envs, h3ConTrampa, h3Tope)
		tbEscribir(t, envs, nuevo+".log", "")
		tbEscribir(t, envs, nuevo+".json", "{}\n")
		leer(t, "con el alto armado", wt)
		visto := a.cerrar()
		if !errors.Is(visto.err, errH3YaEscrito) || visto.id != nuevo || visto.trampa != "" {
			t.Fatalf("%s: con el registro ya escrito el alto lo dice y no planta: %+v", ca, visto)
		}
		if raw, err := os.ReadFile(filepath.Join(envs, nuevo+".json")); err != nil || string(raw) != "{}\n" {
			t.Fatalf("%s: el registro ya escrito no se toca: %q, %v", ca, raw, err)
		}
		despues(t, a, wt, visto)
	})

	t.Run("nadie lee y se cierra", func(t *testing.T) {
		wt, envs := escena(t)
		a := h3PararEnElContrato(t, ca, wt, envs, h3ConTrampa, h3Tope)
		tbEscribir(t, envs, nuevo+".log", "")
		empezo := time.Now()
		visto := a.cerrar()
		if tardo := time.Since(empezo); tardo > h3Tope/2 {
			t.Fatalf("%s: cerrar sin lector no espera al tope: tardo %s", ca, tardo)
		}
		if !errors.Is(visto.err, errH3NadieLeyo) || visto.id != "" || visto.trampa != "" {
			t.Fatalf("%s: sin lector el alto lo dice y no planta: %+v", ca, visto)
		}
		if _, err := os.Lstat(filepath.Join(envs, nuevo+".json")); !os.IsNotExist(err) {
			t.Fatalf("%s: sin lector no hay nada donde va el registro: %v", ca, err)
		}
		despues(t, a, wt, visto)
	})

	t.Run("nadie lee ni cierra: desiste al tope", func(t *testing.T) {
		wt, envs := escena(t)
		const tope = 50 * time.Millisecond
		a := h3PararEnElContrato(t, ca, wt, envs, h3ConTrampa, tope)
		select {
		case <-a.fin:
		case <-time.After(h3Tope):
			t.Fatalf("%s: con un tope de %s y sin lector el alto no desistio solo en %s", ca, tope, h3Tope)
		}
		leer(t, "despues del tope", wt)
		visto := a.cerrar()
		if !errors.Is(visto.err, errH3NadieLeyo) || visto.id != "" || visto.trampa != "" {
			t.Fatalf("%s: al tope el alto informa que nadie leyo: %+v", ca, visto)
		}
		despues(t, a, wt, visto)
	})

	// El caso hostil: senales al proceso mientras el que lee y el alto se
	// encuentran, cien veces seguidas. Con una apertura dormida de por medio
	// esto entrega contratos vacios (ver h3Alto); con la punta de escritura
	// sostenida, ninguno. SIGURG es para Go un pedido de preempcion: al resto
	// del proceso no le cambia nada.
	t.Run("con senales al proceso ninguna lectura sale vacia ni se cuelga", func(t *testing.T) {
		basta, paro := make(chan struct{}), make(chan struct{})
		go func() {
			defer close(paro)
			for pid := os.Getpid(); ; time.Sleep(200 * time.Microsecond) {
				select {
				case <-basta:
					return
				default:
					syscall.Kill(pid, syscall.SIGURG)
				}
			}
		}()
		defer func() { close(basta); <-paro }()
		for vuelta := 0; vuelta < 100; vuelta++ {
			wt, envs := escena(t)
			a := h3PararEnElContrato(t, ca, wt, envs, h3ConTrampa, h3Tope)
			tbEscribir(t, envs, nuevo+".log", "")
			leer(t, fmt.Sprintf("con senales, vuelta %d", vuelta), wt)
			if visto := a.cerrar(); visto.err != nil || visto.id != nuevo || visto.trampa == "" {
				t.Fatalf("%s: con senales, vuelta %d: el alto vio el id nuevo %s y planto: %+v", ca, vuelta, nuevo, visto)
			}
		}
	})
}
