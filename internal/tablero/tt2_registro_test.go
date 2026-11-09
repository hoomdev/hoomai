// El Registro, la linea de mensajes y el fin de lo lanzado (spec
// tablero-con-paneles: CA-562 las lecturas que fallan, CA-563 el Registro;
// y de la v1, que siguen igual: CA-528 lanzar, CA-530 el aviso de fin, CA-533
// salir). Los tests de la v1 del aviso de fin (hallazgos 5f48e8, 8a3a66,
// 08c87c y 48bfb7) miran ahora la linea de mensajes (la penultima, como la
// barra 1 de la v1) y aseveran lo mismo.
package tablero_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

// ttLanzarWriter abre el modal Pedir de "tarjeta" y lanza desde el boton.
func ttLanzarWriter(t *testing.T, m *tablero.Modelo, ahora time.Time) tablero.Lanzar {
	t.Helper()
	m.Tecla("w", ahora)
	if m.Modal() != tablero.ModalPedir {
		t.Fatalf("CA-566: w abre Pedir: %q\n%s", m.Modal(), ttPantalla(m))
	}
	m.Tecla(tablero.TeclaEnter, ahora)
	return ttUnaLanzar(t, "CA-528", "enter en el boton", m.Tecla(tablero.TeclaEnter, ahora))
}

// ttCorre es una tarjeta en Writer con un running del sobre env (sin sobre si
// env es "") y del run run.
func ttCorre(slug, rol, env, run string) boardcmd.Card {
	c := ttCard(slug, "Tarjeta "+slug, boardcmd.ColWriter, "corre "+slug, ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"))
	c.Running = &boardcmd.Running{EnvelopeID: env, RunID: run, Role: rol, Provider: "claude", Stage: "run", Step: 2, Steps: 6, StartedAt: ttAhora}
	return c
}

// ttParada es la misma tarjeta sin running, con su plain de ahora.
func ttParada(slug string) boardcmd.Card {
	return ttCard(slug, "Tarjeta "+slug, boardcmd.ColWriter, "termino "+slug, ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"))
}

// ttCorriendoCon es la tarjeta de prueba con un running del sobre env.
func ttCorriendoCon(env string) boardcmd.Card {
	c := ttTarjetaCon(ttAccionWriter())
	c.Running = &boardcmd.Running{EnvelopeID: env, RunID: "run-" + env, Role: "writer", Provider: "claude", Stage: "run",
		Step: 2, Steps: 6, StartedAt: ttAhora}
	return c
}

// ---------------------------------------------------------------- CA-563: el Registro

// CA-563: cada verbo que la TUI ejecuta suma una entrada con la hora, la
// etiqueta, el slug y su estado: "lanzando…" mientras espera; "corriendo"
// cuando la tarjeta muestra running con un sobre nuevo; "hecho" cuando el
// proceso termina con 0; "fallo: <ultima linea>" en rojo si no; y una
// segunda linea con el comando que mostro "Se va a ejecutar". El fin de un
// trabajo suma "<rol> termino en <slug>: <plain>".
func TestCA563_LosEstadosDeUnaEntrada(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter())))
	t0 := ttAhora.Add(time.Minute)
	l := ttLanzarWriter(t, m, t0)
	comando := "hoom pedir writer tarjeta -m " + tarjetacmd.CitarPedido(ttPedidoWriter, false)
	r := m.Registro()
	if len(r) != 1 || r[0].Texto != "Pedir al writer · tarjeta · lanzando..." || r[0].Comando != comando || !r[0].Hora.Equal(t0) {
		t.Fatalf("CA-563: la entrada al lanzar: %#v", r)
	}
	// registrado: la tarjeta muestra running con un sobre que no tenia
	m.Leer(tablero.Lectura{Ahora: t0.Add(2 * time.Second), Tablero: ttTablero(ttCorriendoCon("env-nuevo"))})
	if r := m.Registro(); len(r) != 1 || r[0].Texto != "Pedir al writer · tarjeta · corriendo" {
		t.Fatalf("CA-563: registrado, la entrada dice 'corriendo' (en su lugar): %#v", r)
	}
	m.Termino(tablero.FinProceso{ID: l.ID, Code: 0, Ultima: "listo"}, t0.Add(time.Minute))
	if r := m.Registro(); len(r) != 1 || r[0].Texto != "Pedir al writer · tarjeta · hecho" || !r[0].Hora.Equal(t0) {
		t.Fatalf("CA-563: con codigo 0 la entrada dice 'hecho' y conserva su hora: %#v", r)
	}
	// el fin del trabajo suma su propia entrada, de una linea
	m.Leer(tablero.Lectura{Ahora: t0.Add(2 * time.Minute), Tablero: ttTablero(ttTarjetaCon(ttAccionWriter()))})
	r = m.Registro()
	if len(r) != 2 || r[1].Texto != "writer termino en tarjeta: el plain de la tarjeta" || r[1].Comando != "" || !r[1].Hora.Equal(t0.Add(2*time.Minute)) {
		t.Fatalf("CA-563: el fin de un trabajo suma '<rol> termino en <slug>: <plain>' sin comando: %#v", r)
	}
	// fallo, con y sin su ultima linea
	g := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionGuardar(true, "", "a.go"))))
	g.Tecla("s", ttAhora)
	lg := ttUnaLanzar(t, "CA-574", "enter en Guardar", g.Tecla(tablero.TeclaEnter, ttAhora))
	g.Termino(tablero.FinProceso{ID: lg.ID, Code: 1, Ultima: "no se pudo guardar: conflicto"}, ttAhora)
	if r := g.Registro(); len(r) != 1 || r[0].Texto != "Guardar · tarjeta · fallo: no se pudo guardar: conflicto" || r[0].Comando != "hoom guardar tarjeta" {
		t.Fatalf("CA-563: un proceso que falla: 'fallo: <ultima linea>': %#v", r)
	}
	g.Tecla("s", ttAhora)
	lg = ttUnaLanzar(t, "CA-574", "enter en Guardar", g.Tecla(tablero.TeclaEnter, ttAhora))
	g.Termino(tablero.FinProceso{ID: lg.ID, Code: 3}, ttAhora)
	if r := g.Registro(); len(r) != 2 || r[1].Texto != "Guardar · tarjeta · fallo: hoom guardar salio con 3" {
		t.Fatalf("CA-563: sin ultima linea: 'fallo: hoom <verbo> salio con <codigo>': %#v", r)
	}
	// con UTF-8, "lanzando…"
	u := ttModelo(ttConfig(200, 40, true), ttTablero(ttTarjetaCon(ttAccionWriter())))
	ttLanzarWriter(t, u, ttAhora)
	if r := u.Registro(); len(r) != 1 || r[0].Texto != "Pedir al writer · tarjeta · lanzando…" {
		t.Fatalf("CA-563: con UTF-8 'lanzando…': %#v", r)
	}
}

// CA-563, caso limite: un trabajo que termina antes de su primer registro:
// la entrada dice "fallo: <linea>" (y la linea de mensajes lo dice en rojo:
// CA-528).
func TestCA563_CA528_TerminaAntesDelRegistro(t *testing.T) {
	cfg := ttAncho()
	cfg.Color = true
	m := ttModelo(cfg, ttTablero(ttTarjetaCon(ttAccionWriter())))
	l := ttLanzarWriter(t, m, ttAhora)
	if m.Corriendo() != 1 {
		t.Fatalf("CA-528: lanzado, Corriendo es 1, es %d", m.Corriendo())
	}
	m.Termino(tablero.FinProceso{ID: l.ID, Code: 1, Ultima: "ya se esta lanzando un trabajo en esta tarjeta"}, ttAhora.Add(time.Second))
	if m.Corriendo() != 0 {
		t.Fatalf("CA-528: terminado, Corriendo es 0, es %d", m.Corriendo())
	}
	ttDiceMensaje(t, "CA-528", "termino antes del registro", m, true, "ya se esta lanzando un trabajo en esta tarjeta")
	if r := m.Registro(); len(r) != 1 || r[0].Texto != "Pedir al writer · tarjeta · fallo: ya se esta lanzando un trabajo en esta tarjeta" {
		t.Fatalf("CA-563: la entrada dice 'fallo: <linea>': %#v", r)
	}
	var reg string
	for _, x := range m.Pantalla() {
		if strings.Contains(ttSinSGR(x), "fallo: ya se esta") {
			reg = x
		}
	}
	if !ttConColor(ttRojo, reg, "fallo: ya se esta") {
		t.Fatalf("CA-563: 'fallo: <...>' va en rojo en el Registro: %q", reg)
	}
	m.Tic(ttAhora.Add(time.Second + tablero.DuracionAviso + time.Second))
	if strings.Contains(ttMensaje(m), "ya se esta lanzando") {
		t.Fatalf("CA-528: el aviso dura DuracionAviso: %q", ttMensaje(m))
	}
}

// CA-563: el panel del Registro muestra las entradas mas nuevas que entran
// enteras, la mas vieja arriba y la mas nueva abajo, desde su primera linea
// interior: " <HH:MM> <texto>" y siete espacios y el comando; vacio, "todavia
// no se ejecuto nada"; la hora en la zona del reloj del modelo.
func TestCA563_ElPanelDelRegistro(t *testing.T) {
	m := ttModelo(ttConfig(120, 32, false), ttTablero(ttTarjetaCon(ttAccionWriter()), ttBacklog("otra", "Otra")))
	reg := func() []string { return ttCajaDe(t, "CA-563", m, "Registro").ttTexto() }
	if got := reg(); got[0] != " todavia no se ejecuto nada" || ttCajaDe(t, "CA-563", m, "Registro").Titulo != "Registro" {
		t.Fatalf("CA-563: vacio, el Registro dice ' todavia no se ejecuto nada': %q", got)
	}
	zona := time.FixedZone("UTC-6", -6*3600)
	ttIrA(t, m, "tarjeta")
	m.Tecla("u", ttAhora.In(zona))                     // 06:00
	m.Tecla("u", ttAhora.Add(5*time.Minute).In(zona))  // 06:05
	m.Tecla("u", ttAhora.Add(10*time.Minute).In(zona)) // 06:10
	got := reg()
	quiere := []string{
		" 06:05 Fijar para la terminal · tarjeta · lanzando...",
		"       hoom usar tarjeta",
		" 06:10 Fijar para la terminal · tarjeta · lanzando...",
		"       hoom usar tarjeta",
	}
	if strings.Join(got, "\n") != strings.Join(quiere, "\n") {
		t.Fatalf("CA-563: a 120x32 (4 lineas interiores) las dos entradas mas nuevas, la vieja arriba, con la hora del reloj del modelo:\n got %q\nwant %q", got, quiere)
	}
	// una entrada de una linea (el fin de un trabajo) tambien cuenta entera
	m = ttModelo(ttConfig(120, 32, false), ttTablero(ttCorre("corre", "writer", "env", "run")))
	m.Tecla("u", ttAhora)
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Minute), Tablero: ttTablero(ttParada("corre"))})
	got = ttCajaDe(t, "CA-563", m, "Registro").ttTexto()
	quiere = []string{" 12:00 Fijar para la terminal · corre · lanzando...", "       hoom usar corre", " 12:01 writer termino en corre: termino corre", ""}
	if strings.Join(got, "\n") != strings.Join(quiere, "\n") {
		t.Fatalf("CA-563: la entrada del fin es una linea:\n got %q\nwant %q", got, quiere)
	}
}

// CA-563: plegado (una columna), el Registro muestra en su titulo la primera
// linea de su entrada mas nueva, sin su margen; vacio, "Registro".
func TestCA563_ElRegistroPlegado(t *testing.T) {
	m := ttModelo(ttConfig(70, 24, false), ttTablero(ttTarjetaCon(ttAccionWriter())))
	if c := ttCajaDe(t, "CA-563", m, "Registro"); !c.Plegado || c.Titulo != "Registro" {
		t.Fatalf("CA-563: plegado y vacio, el titulo es 'Registro': %+v", c)
	}
	m.Tecla("u", ttAhora)
	m.Tecla("u", ttAhora.Add(2*time.Minute))
	c := ttCajaDe(t, "CA-563", m, "Registro")
	if !c.Plegado || c.Titulo != "Registro · 12:02 Fijar para la terminal · tarjeta · lanzando..." {
		t.Fatalf("CA-563: plegado, el titulo dice la primera linea de la entrada mas nueva: %q", c.Titulo)
	}
	if !strings.HasPrefix(c.Arriba, "-Registro · 12:02 ") || ttCeldas(c.Arriba) != 70 {
		t.Fatalf("CA-563: la linea plegada mide su ancho: %q", c.Arriba)
	}
}

// CA-563: el comando del Registro es el que mostro "Se va a ejecutar", en una
// linea y cortado al panel; ningun control de un texto llega al Registro.
func TestCA563_ElComandoCortadoAlPanel(t *testing.T) {
	a := ttAccionWriter()
	a.Pedido = "un pedido largo con\ttab y \x1b[2J control " + strings.Repeat("mas texto ", 10)
	m := ttModelo(ttConfig(80, 24, false), ttTablero(ttTarjetaCon(a)))
	ttLanzarWriter(t, m, ttAhora)
	c := ttCajaDe(t, "CA-563", m, "Registro")
	if len(c.Dentro) < 2 {
		t.Fatalf("CA-563: el Registro tiene la entrada: %+v", c)
	}
	linea := c.Dentro[1]
	if ttCeldas(linea) != 52 || !strings.HasPrefix(linea, "       hoom pedir writer tarjeta -m ") || strings.Contains(linea, "\x1b") {
		t.Fatalf("CA-563: la segunda linea es el comando cortado a las 52 celdas, sin controles: %q", linea)
	}
}

// ---------------------------------------------------------------- CA-562: las lecturas que fallan

// CA-562: una lectura del proyecto o de los hallazgos que falla deja lo
// ultimo en pantalla y la linea de mensajes dice "no se pudo leer el
// proyecto: <error>" o "no se pudieron leer los hallazgos: <error>" hasta
// que una lectura funcione; los errores de lectura van debajo de cualquier
// mensaje con tiempo: el del tablero primero, despues el del proyecto,
// despues el de los hallazgos.
func TestCA562_LecturasQueFallan(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	antes1 := ttCajaDe(t, "CA-562", m, "[1]").ttTexto()
	antes4 := ttCajaDe(t, "CA-562", m, "[4]").ttTexto()
	m.LeerProyecto(tablero.Proyecto{Err: errors.New("git no responde")})
	ttDiceMensaje(t, "CA-562", "el proyecto falla", m, false, "no se pudo leer el proyecto: git no responde")
	if got := ttCajaDe(t, "CA-562", m, "[1]").ttTexto(); strings.Join(got, "|") != strings.Join(antes1, "|") {
		t.Fatalf("CA-562: [1] se queda con la ultima lectura: %q", got)
	}
	m.LeerHallazgos(tablero.Hallazgos{Err: errors.New("hallazgo ilegible")})
	ttDiceMensaje(t, "CA-562", "los dos fallan: primero el proyecto", m, false, "no se pudo leer el proyecto: git no responde")
	if got := ttCajaDe(t, "CA-562", m, "[4]").ttTexto(); strings.Join(got, "|") != strings.Join(antes4, "|") {
		t.Fatalf("CA-562: [4] se queda con la ultima lectura: %q", got)
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Err: errors.New("disco roto")})
	ttDiceMensaje(t, "CA-562", "el tablero falla: primero el tablero", m, false, "no se pudo leer el tablero: disco roto")
	// un mensaje con tiempo va encima de los errores de lectura
	m.Tecla("w", ttAhora) // nuc-tenancy no tiene writer
	ttDiceMensaje(t, "CA-562", "un aviso encima", m, false, "en Tu aprobacion no hay pedir al writer")
	m.Tic(ttAhora.Add(tablero.DuracionAviso + time.Second))
	ttDiceMensaje(t, "CA-562", "pasado el aviso vuelve el error", m, false, "no se pudo leer el tablero: disco roto")
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Minute), Tablero: ttTableroDorado()})
	ttDiceMensaje(t, "CA-562", "el tablero lee: el del proyecto", m, false, "no se pudo leer el proyecto: git no responde")
	m.LeerProyecto(ttProyectoVerde())
	ttDiceMensaje(t, "CA-562", "el proyecto lee: el de los hallazgos", m, false, "no se pudieron leer los hallazgos: hallazgo ilegible")
	m.LeerHallazgos(ttHallazgosDorados())
	if got := ttMensaje(m); got != "" {
		t.Fatalf("CA-562: con todas las lecturas bien no hay mensaje: %q", got)
	}
	// antes de la primera lectura buena del proyecto tambien lo dice
	n := ttModelo(ttConfig(120, 32, false), ttTableroPrincipal())
	n.LeerProyecto(tablero.Proyecto{Err: errors.New("sin permiso")})
	ttDiceMensaje(t, "CA-562", "la primera lectura falla", n, false, "no se pudo leer el proyecto: sin permiso")
}

// CA-530, CA-562: una lectura del tablero que falla deja la ultima pantalla
// de [2] y la linea de mensajes dice "no se pudo leer el tablero: <error>"
// hasta que una lectura funcione.
func TestCA530_CA562_LecturaDelTableroQueFalla(t *testing.T) {
	m := ttModelo(ttConfig(120, 24, false), ttTableroPrincipal())
	antes := ttCajaDe(t, "CA-530", m, "[2]").ttTexto()
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Err: errors.New("disco roto")})
	ttDiceMensaje(t, "CA-530", "falla", m, false, "no se pudo leer el tablero: disco roto")
	if got := ttCajaDe(t, "CA-530", m, "[2]").ttTexto(); strings.Join(got, "|") != strings.Join(antes, "|") {
		t.Fatalf("CA-530: la pantalla se queda con el ultimo tablero: %q", got)
	}
	m.Tic(ttAhora.Add(time.Minute))
	ttDiceMensaje(t, "CA-530", "un minuto despues", m, false, "no se pudo leer el tablero: disco roto")
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Minute + 2*time.Second), Tablero: ttTableroPrincipal()})
	if strings.Contains(ttMensaje(m), "no se pudo leer") {
		t.Fatalf("CA-530: una lectura que funciona quita el error: %q", ttMensaje(m))
	}
	n := tablero.Nuevo(ttConfig(120, 24, false))
	n.Leer(tablero.Lectura{Ahora: ttAhora, Err: errors.New("sin permiso")})
	ttDiceMensaje(t, "CA-530", "una primera lectura que falla", n, false, "no se pudo leer el tablero: sin permiso")
}

// ---------------------------------------------------------------- CA-530: el aviso de fin (de la v1)

// CA-530, CA-563 (hallazgo 5f48e8 de la v1): una tarjeta vista con running
// del sobre E, leida sin running y con interrupted de E, avisa en rojo; lo
// mismo un run sin sobre que queda interrupted; la que solo dejo de correr,
// sin rojo ni interrupted, en verde; tambien la que muestra interrupted de
// otro sobre. El aviso va a la linea de mensajes y al Registro.
func TestCA530_CA563_FinInterrumpidoEsRojo(t *testing.T) {
	interr := func(slug, env, run, rol string) boardcmd.Card {
		c := ttParada(slug)
		c.Plain, c.NeedsDecision = "el trabajo se corto", true
		c.Interrupted = &boardcmd.Interrupted{EnvelopeID: env, RunID: run, Role: rol, Provider: "claude", Stage: "run", Step: 2, Steps: 6, UpdatedAt: ttAhora}
		return c
	}
	casos := []struct {
		nombre string
		antes  boardcmd.Card
		despue boardcmd.Card
		msg    string
		rojo   bool
	}{
		{"interrumpida del sobre", ttCorre("tarjeta", "writer", "env-e", "run-e"), interr("tarjeta", "env-e", "run-e", "writer"),
			"writer termino en tarjeta: el trabajo se corto", true},
		{"interrumpida del run sin sobre", ttCorre("tarjeta", "reviewer", "", "run-r"), interr("tarjeta", "", "run-r", "reviewer"),
			"reviewer termino en tarjeta: el trabajo se corto", true},
		{"solo paro", ttCorre("tarjeta", "writer", "env-e", "run-e"), ttParada("tarjeta"),
			"writer termino en tarjeta: termino tarjeta", false},
		{"interrumpida de otro sobre", ttCorre("tarjeta", "writer", "env-e", "run-e"), interr("tarjeta", "env-otro", "run-otro", "writer"),
			"writer termino en tarjeta: el trabajo se corto", false},
	}
	for _, cs := range casos {
		cfg := ttAncho()
		cfg.Color = true
		m := ttModelo(cfg, ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		b := ttMensajeCrudo(m)
		color := ttVerde
		if cs.rojo {
			color = ttRojo
		}
		if !strings.Contains(b, cs.msg) || !ttConColor(color, b, cs.msg) {
			t.Fatalf("CA-530: %s: la linea de mensajes dice %q en %s: %q", cs.nombre, cs.msg, map[bool]string{true: "rojo", false: "verde"}[cs.rojo], b)
		}
		var reg string
		for _, x := range m.Pantalla()[:len(m.Pantalla())-2] {
			if strings.Contains(ttSinSGR(x), cs.msg) {
				reg = x
			}
		}
		if reg == "" || !ttConColor(color, reg, cs.msg) {
			t.Fatalf("CA-563: %s: el Registro dice el fin en el mismo color: %q", cs.nombre, reg)
		}
		m = ttModelo(ttAncho(), ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		ttDiceMensaje(t, "CA-530", cs.nombre, m, false, cs.msg)
	}
}

// CA-530 (hallazgo 8a3a66 de la v1): dos tarjetas que dejan de correr en la
// misma lectura avisan una tras otra, en el orden del tablero, cada una
// durante DuracionFin; despues, ninguna.
func TestCA530_DosFinesEnUnaLectura(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttCorre("zeta", "test-writer", "env-z", "run-z"), ttCorre("alfa", "writer", "env-a", "run-a")))
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttParada("zeta"), ttParada("alfa"))})
	primero, segundo := "test-writer termino en zeta: termino zeta", "writer termino en alfa: termino alfa"
	ttDiceMensaje(t, "CA-530", "al leer", m, false, primero)
	m.Tic(t0.Add(tablero.DuracionFin - time.Second))
	ttDiceMensaje(t, "CA-530", "antes de DuracionFin", m, false, primero)
	m.Tic(t0.Add(tablero.DuracionFin + 500*time.Millisecond))
	ttDiceMensaje(t, "CA-530", "pasado el primero", m, false, segundo)
	m.Tic(t0.Add(2*tablero.DuracionFin - time.Second))
	ttDiceMensaje(t, "CA-530", "el segundo dura DuracionFin", m, false, segundo)
	m.Tic(t0.Add(2*tablero.DuracionFin + time.Second))
	if strings.Contains(ttMensaje(m), "termino en") {
		t.Fatalf("CA-530: pasados los dos, ningun fin: %q", ttMensaje(m))
	}
	if r := m.Registro(); len(r) != 2 || r[0].Texto != primero || r[1].Texto != segundo {
		t.Fatalf("CA-563: los dos fines van al Registro, en el orden del tablero: %#v", r)
	}
}

// CA-530, CA-528 (hallazgo 8a3a66 de la v1): el aviso de fin le gana la linea
// de mensajes enseguida a un mensaje de otra clase ("lanzando hoom ...", un
// why).
func TestCA530_ElAvisoDeFinTomaLaLinea(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter()), ttCorre("dos", "reviewer", "env-dos", "run-dos")))
	ttIrA(t, m, "tarjeta")
	ttLanzarWriter(t, m, ttAhora)
	ttDiceMensaje(t, "CA-528", "lanzado", m, false, "lanzando hoom pedir writer tarjeta")
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Second), Tablero: ttTablero(ttTarjetaCon(ttAccionWriter()), ttParada("dos"))})
	ttDiceMensaje(t, "CA-530", "tras lanzar", m, false, "reviewer termino en dos: termino dos")
	m = ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar)), ttCorre("dos", "reviewer", "env-dos", "run-dos")))
	ttIrA(t, m, "tarjeta")
	m.Tecla("s", ttAhora)
	ttDiceMensaje(t, "CA-525", "el why", m, false, ttWhyGuardar)
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(time.Second), Tablero: ttTablero(ttTarjetaCon(ttAccionGuardar(false, ttWhyGuardar)), ttParada("dos"))})
	ttDiceMensaje(t, "CA-530", "tras un why", m, false, "reviewer termino en dos: termino dos")
}

// ttFinVuelveEntero mueve el reloj con Tic, de a 250 ms desde desde, hasta que
// la linea de mensajes dice el aviso de fin msg (a lo sumo hasta tope), y
// exige que lo diga sin cortes durante un DuracionFin entero desde que
// volvio y que pasado ese DuracionFin ya no lo diga.
func ttFinVuelveEntero(t *testing.T, ca, que string, m *tablero.Modelo, desde, tope time.Time, msg string) {
	t.Helper()
	const paso = 250 * time.Millisecond
	dice := func() bool { return ttMensaje(m) == msg }
	ahora := desde
	for !dice() {
		ahora = ahora.Add(paso)
		if ahora.After(tope) {
			t.Fatalf("%s: %s: el aviso de fin %q vuelve a lo sumo a los %v, la linea dice %q", ca, que, msg, tope.Sub(desde)+paso, ttMensaje(m))
		}
		m.Tic(ahora)
	}
	volvio := ahora
	for ahora.Before(volvio.Add(tablero.DuracionFin - 2*paso)) {
		ahora = ahora.Add(paso)
		m.Tic(ahora)
		if !dice() {
			t.Fatalf("%s: %s: el aviso de fin vuelve entero: %v despues ya no lo dice: %q", ca, que, ahora.Sub(volvio), ttMensaje(m))
		}
	}
	m.Tic(volvio.Add(tablero.DuracionFin + 2*paso))
	if strings.Contains(ttMensaje(m), msg) {
		t.Fatalf("%s: %s: pasado DuracionFin desde que volvio, ya no lo dice: %q", ca, que, ttMensaje(m))
	}
}

// CA-530, CA-578 (hallazgo 08c87c de la v1): ningun aviso de fin se pierde. En
// una lectura donde una tarjeta deja de correr y la tarjeta que [0] mostraba
// con el foco ya no esta, el foco vuelve a [2] y la linea dice primero "la
// tarjeta <slug> ya no existe" (su DuracionAviso) y despues el aviso de fin,
// entero.
func TestCA530_CA578_ElFinVuelveDespuesDeYaNoExiste(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttBacklog("abierta", "La tarjeta abierta"), ttCorre("corre", "writer", "env-c", "run-c")))
	ttIrA(t, m, "abierta")
	m.Tecla(tablero.TeclaEnter, ttAhora)
	if m.Foco() != tablero.PanelPrincipal || m.Elegida() != "abierta" {
		t.Fatalf("CA-553: enter pasa el foco a [0] con la tarjeta: %d %q", int(m.Foco()), m.Elegida())
	}
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttParada("corre"))})
	if m.Foco() != tablero.PanelTarjetas {
		t.Fatalf("CA-578: la tarjeta de [0] ya no existe: el foco vuelve a [2]: %d", int(m.Foco()))
	}
	const ya = "la tarjeta abierta ya no existe"
	ttDiceMensaje(t, "CA-530", "primero lo de esta lectura", m, false, ya)
	m.Tic(t0.Add(tablero.DuracionAviso - time.Second))
	ttDiceMensaje(t, "CA-530", "ya no existe dura su DuracionAviso", m, false, ya)
	ttFinVuelveEntero(t, "CA-530", "despues de ya no existe", m, t0.Add(tablero.DuracionAviso-time.Second),
		t0.Add(tablero.DuracionAviso+time.Second), "writer termino en corre: termino corre")
}

// CA-530, CA-574 (hallazgo 08c87c de la v1): lo mismo con una firma que esa
// lectura cierra: primero "cambio lo que ibas a confirmar en <slug>: vuelve
// a abrirlo" y despues el aviso de fin, entero. Nada se firma.
func TestCA530_CA574_ElFinVuelveDespuesDeLaFirmaCerrada(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttNucTenancy(), ttCorre("corre", "reviewer", "env-c", "run-c")))
	ttIrA(t, m, "nuc-tenancy")
	ttSinOrdenes(t, "CA-574", "A", m.Tecla("A", ttAhora))
	m.Tecla(tablero.TeclaDer, ttAhora)
	otra := ttNucTenancy()
	otra.Item.Titulo = "Tenancy del nucleo, otra"
	t0 := ttAhora.Add(2 * time.Second)
	if l := ttLanzares(m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(otra, ttParada("corre"))})); len(l) != 0 {
		t.Fatalf("CA-574: la lectura que cambia la firma no firma: %#v", l)
	}
	if m.Modal() != tablero.SinModal {
		t.Fatalf("CA-574: la firma se cierra: %q", m.Modal())
	}
	cambio := ttCambio("nuc-tenancy")
	ttDiceMensaje(t, "CA-530", "primero la firma cerrada", m, false, cambio)
	m.Tic(t0.Add(tablero.DuracionAviso - time.Second))
	ttDiceMensaje(t, "CA-530", "dura su DuracionAviso", m, false, cambio)
	ttFinVuelveEntero(t, "CA-530", "despues de la firma cerrada", m, t0.Add(tablero.DuracionAviso-time.Second),
		t0.Add(tablero.DuracionAviso+time.Second), "reviewer termino en corre: termino corre")
	if l := ttLanzares(m.Tecla(tablero.TeclaEnter, t0.Add(30*time.Second))); len(l) != 0 {
		t.Fatalf("CA-574: nada se firma: %#v", l)
	}
}

// CA-530 (hallazgo 08c87c de la v1): un mensaje que una tecla da mientras se
// ve el aviso de fin toma la linea su DuracionAviso y despues el aviso de fin
// vuelve entero.
func TestCA530_ElFinVuelveDespuesDelMensajeDeUnaTecla(t *testing.T) {
	m := ttModelo(ttAncho(), ttTablero(ttCorre("corre", "writer", "env-c", "run-c")))
	t0 := ttAhora.Add(2 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t0, Tablero: ttTablero(ttParada("corre"))})
	const fin = "writer termino en corre: termino corre"
	ttDiceMensaje(t, "CA-530", "al leer", m, false, fin)
	t1 := t0.Add(2 * time.Second)
	m.Tic(t1)
	if m.Elegida() != "corre" {
		t.Fatalf("CA-552: la seleccion sigue en su tarjeta: %q", m.Elegida())
	}
	msg := "en " + ttNombreColumna(boardcmd.ColWriter) + " no hay " + ttHaceDe(t, "a")
	ttSinOrdenes(t, "CA-525", "a sin la accion", m.Tecla("a", t1))
	ttDiceMensaje(t, "CA-530", "la tecla toma la linea", m, false, msg)
	m.Tic(t1.Add(tablero.DuracionAviso - time.Second))
	ttDiceMensaje(t, "CA-530", "su DuracionAviso", m, false, msg)
	ttFinVuelveEntero(t, "CA-530", "despues del mensaje de la tecla", m, t1.Add(tablero.DuracionAviso-time.Second),
		t1.Add(tablero.DuracionAviso+time.Second), fin)
}

// CA-530 (hallazgo 48bfb7 de la v1): un trabajo sin sobre (un run de hoom run
// a mano) que deja de correr no cerro ningun sobre entregable: su aviso va en
// rojo, tambien con el rojo de un veredicto o un interrupted de otro run;
// con sobre, sin rojo, es verde.
func TestCA530_ElFinDeUnRunSinSobreEsRojo(t *testing.T) {
	conRojo := func() boardcmd.Card {
		c := ttParada("tarjeta")
		c.Red = &boardcmd.Red{Source: boardcmd.RedVerdict, ID: "v-9", Reason: "test: FAIL", Plain: "la verificacion dio rojo"}
		return c
	}
	conOtro := func() boardcmd.Card {
		c := ttParada("tarjeta")
		c.Interrupted = &boardcmd.Interrupted{RunID: "run-otro", Role: "writer", Provider: "claude", Stage: "run", Step: 2, Steps: 6, UpdatedAt: ttAhora}
		return c
	}
	casos := []struct {
		nombre string
		antes  boardcmd.Card
		despue boardcmd.Card
		verde  bool
	}{
		{"run sin sobre que paro", ttCorre("tarjeta", "writer", "", "run-r"), ttParada("tarjeta"), false},
		{"run sin sobre con el rojo de un veredicto", ttCorre("tarjeta", "writer", "", "run-r"), conRojo(), false},
		{"run sin sobre con un interrupted de otro run", ttCorre("tarjeta", "writer", "", "run-r"), conOtro(), false},
		{"con sobre (guarda: verde)", ttCorre("tarjeta", "writer", "env-e", "run-e"), ttParada("tarjeta"), true},
	}
	const msg = "writer termino en tarjeta: termino tarjeta"
	for _, cs := range casos {
		cfg := ttAncho()
		cfg.Color = true
		m := ttModelo(cfg, ttTablero(cs.antes))
		m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(cs.despue)})
		b := ttMensajeCrudo(m)
		verde, rojo := ttConColor(ttVerde, b, msg), ttConColor(ttRojo, b, msg)
		if !strings.Contains(ttSinSGR(b), msg) || (cs.verde && (!verde || rojo)) || (!cs.verde && (!rojo || verde)) {
			t.Fatalf("CA-530: %s: el aviso va en %s: %q", cs.nombre, map[bool]string{true: "verde", false: "rojo"}[cs.verde], b)
		}
	}
}

// CA-528, CA-530: lanzado y registrado, el fin del proceso deja la linea al
// aviso de Leer: "<rol> termino en <slug>: <plain>" durante DuracionFin, en
// rojo si el rojo de la tarjeta es el de ese sobre.
func TestCA528_CA530_RegistradoYTermino(t *testing.T) {
	cfg := ttAncho()
	cfg.Color = true
	m := ttModelo(cfg, ttTablero(ttCorriendoCon("env-viejo")))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(ttTarjetaCon(ttAccionWriter()))})
	m.Tic(ttAhora.Add(time.Minute))
	l := ttLanzarWriter(t, m, ttAhora.Add(time.Minute))
	t1 := ttAhora.Add(time.Minute + 2*time.Second)
	m.Leer(tablero.Lectura{Ahora: t1, Tablero: ttTablero(ttCorriendoCon("env-nuevo"))})
	m.Termino(tablero.FinProceso{ID: l.ID, Code: 1, Ultima: "algo fallo en el writer"}, t1.Add(time.Second))
	if strings.Contains(ttMensaje(m), "algo fallo en el writer") {
		t.Fatalf("CA-528: registrado, el fin del proceso deja la linea al aviso de Leer: %q", ttMensaje(m))
	}
	fin := ttTarjetaCon(ttAccionWriter())
	fin.Plain = "la verificacion dio rojo"
	fin.Red = &boardcmd.Red{Source: boardcmd.RedEnvelope, ID: "env-nuevo", Plain: "la verificacion dio rojo"}
	t2 := t1.Add(10 * time.Second)
	m.Leer(tablero.Lectura{Ahora: t2, Tablero: ttTablero(fin)})
	b := ttMensajeCrudo(m)
	if !strings.Contains(ttSinSGR(b), "writer termino en tarjeta: la verificacion dio rojo") || !ttConColor(ttRojo, b, "writer termino en tarjeta") {
		t.Fatalf("CA-530: el aviso de fin, en rojo porque el rojo es de su sobre: %q", b)
	}
	m.Tic(t2.Add(tablero.DuracionFin - time.Second))
	if !strings.Contains(ttMensaje(m), "writer termino en tarjeta") {
		t.Fatalf("CA-530: el aviso dura DuracionFin: %q", ttMensaje(m))
	}
	m.Tic(t2.Add(tablero.DuracionFin + time.Second))
	if strings.Contains(ttMensaje(m), "termino en") {
		t.Fatalf("CA-530: pasado DuracionFin el aviso se va: %q", ttMensaje(m))
	}
}

// CA-530, caso limite de la v1: un rol lanzado desde otra terminal aparece
// igual (en [3]) y su fin avisa, en verde cuando el rojo no es el de su sobre.
func TestCA530_FinDeUnRolLanzadoDesdeOtraTerminal(t *testing.T) {
	cfg := ttConfig(120, 24, false)
	cfg.Color = true
	m := ttModelo(cfg, ttTablero(ttCorriendoCon("env-ajeno")))
	if e := ttElegidaEn(ttCajaDe(t, "CA-558", m, "[3]")); !strings.HasPrefix(e, "tarjeta writer ") {
		t.Fatalf("CA-558: el running ajeno se ve en [3]: %q", e)
	}
	fin := ttTarjetaCon(ttAccionWriter())
	fin.Plain = "espera tu aceptacion"
	fin.Red = &boardcmd.Red{Source: boardcmd.RedEnvelope, ID: "env-otro-mas-viejo", Plain: "x"}
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(fin)})
	if b := ttMensajeCrudo(m); !ttConColor(ttVerde, b, "writer termino en tarjeta: espera tu aceptacion") {
		t.Fatalf("CA-530: el fin de un rol ajeno avisa en verde: %q", b)
	}
}

// CA-528: MensajeSalida dice cuantos trabajos de rol quedan corriendo; Corriendo
// cuenta los lanzados que no terminaron (un ID desconocido no descuenta); los
// ID van de 1 en 1.
func TestCA528_MensajeSalidaYCorriendo(t *testing.T) {
	if got := tablero.MensajeSalida(0); got != "" {
		t.Fatalf("CA-528: sin trabajos no hay mensaje: %q", got)
	}
	if got := tablero.MensajeSalida(1); got != "queda 1 trabajo corriendo: hoom status" {
		t.Fatalf("CA-528: con uno: %q", got)
	}
	for n, q := range map[int]string{2: "quedan 2 trabajos corriendo: hoom status", 17: "quedan 17 trabajos corriendo: hoom status"} {
		if got := tablero.MensajeSalida(n); got != q {
			t.Fatalf("CA-528: con %d: %q", n, got)
		}
	}
	m := ttModelo(ttAncho(), ttTablero(ttTarjetaCon(ttAccionWriter())))
	var ids []int
	for i := 0; i < 3; i++ {
		ids = append(ids, ttLanzarWriter(t, m, ttAhora).ID)
	}
	if ids[0] != 1 || ids[1] != 2 || ids[2] != 3 || m.Corriendo() != 3 {
		t.Fatalf("CA-528: los ID van de 1 en 1 y corren tres: %v, %d", ids, m.Corriendo())
	}
	m.Termino(tablero.FinProceso{ID: 99, Code: 0}, ttAhora)
	m.Termino(tablero.FinProceso{ID: 2, Code: 0, Ultima: "ok"}, ttAhora)
	if m.Corriendo() != 2 || tablero.MensajeSalida(m.Corriendo()) != "quedan 2 trabajos corriendo: hoom status" {
		t.Fatalf("CA-528: termino uno (y uno desconocido no cuenta): %d", m.Corriendo())
	}
}

// CA-533: q y ctrl-c ordenan Salir desde los paneles y [0]; ctrl-c tambien
// desde la busqueda, la lista abierta y cada modal (q ahi no sale: en un
// texto es una q).
func TestCA533_QYCtrlCOrdenanSalir(t *testing.T) {
	salir := func(os []tablero.Orden) bool { return len(os) == 1 && os[0] == tablero.Orden(tablero.Salir{}) }
	for _, k := range []tablero.Tecla{"q", tablero.TeclaCtrlC} {
		for _, antes := range []tablero.Tecla{"2", "1", "3", "4", "0"} {
			m := ttModeloDorado(ttConfig(120, 32, false))
			m.Tecla(antes, ttAhora)
			if os := m.Tecla(k, ttAhora); !salir(os) {
				t.Fatalf("CA-533: %q con el foco en [%s] ordena Salir: %#v", k, antes, os)
			}
		}
	}
	m := ttPedir(t, "w", ttAccionWriter())
	if os := ttEscribir(m, "q"); len(os) != 0 {
		t.Fatalf("CA-533: q en el Pedido es texto: %#v", os)
	}
	if os := m.Tecla(tablero.TeclaCtrlC, ttAhora); !salir(os) {
		t.Fatalf("CA-533: ctrl-c en el modal ordena Salir: %#v", os)
	}
}
