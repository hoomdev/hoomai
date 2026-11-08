// Las teclas (CA-532, CA-533): Decodificar, la tabla Atajos y AtajosDe, la
// ayuda y la barra 2 desde la misma tabla, y las dos propiedades del
// contrato: toda tecla de la ayuda tiene efecto y ninguna tecla fuera de la
// tabla lo tiene.
package tablero_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- Decodificar

// CA-532, CA-533: Decodificar convierte los bytes de la terminal cruda en
// teclas, como dice su contrato (los casos limite incluidos).
func TestCA532_Decodificar(t *testing.T) {
	type T = tablero.Tecla
	casos := []struct {
		nombre string
		in     string
		quiere []T
	}{
		{"letra", "j", []T{"j"}},
		{"mayuscula", "G", []T{"G"}},
		{"espacio", " ", []T{tablero.TeclaEspacio}},
		{"varias", "jk/", []T{"j", "k", "/"}},
		{"runa de varios bytes", "ñé✓", []T{"ñ", "é", "✓"}},
		{"ctrl-c", "\x03", []T{tablero.TeclaCtrlC}},
		{"ctrl-e", "\x05", []T{tablero.TeclaCtrlE}},
		{"ctrl-u", "\x15", []T{tablero.TeclaCtrlU}},
		{"tab", "\x09", []T{tablero.TeclaTab}},
		{"enter cr", "\r", []T{tablero.TeclaEnter}},
		{"enter lf", "\n", []T{tablero.TeclaEnter}},
		{"backspace del", "\x7f", []T{tablero.TeclaBorrar}},
		{"backspace bs", "\x08", []T{tablero.TeclaBorrar}},
		{"flechas CSI", "\x1b[A\x1b[B\x1b[C\x1b[D", []T{tablero.TeclaArriba, tablero.TeclaAbajo, tablero.TeclaDer, tablero.TeclaIzq}},
		{"flechas SS3", "\x1bOA\x1bOB\x1bOC\x1bOD", []T{tablero.TeclaArriba, tablero.TeclaAbajo, tablero.TeclaDer, tablero.TeclaIzq}},
		{"shift-tab", "\x1b[Z", []T{tablero.TeclaShiftTab}},
		{"esc solo al final", "\x1b", []T{tablero.TeclaEsc}},
		{"esc y letra", "\x1bx", []T{tablero.TeclaEsc, "x"}},
		{"dos esc", "\x1b\x1b", []T{tablero.TeclaEsc, tablero.TeclaEsc}},
		{"esc y enter", "\x1b\r", []T{tablero.TeclaEsc, tablero.TeclaEnter}},
		{"CSI desconocida se descarta", "\x1b[15~j", []T{"j"}},
		{"pegado con corchetes", "\x1b[200~hola\x1b[201~", []T{"h", "o", "l", "a"}},
		{"otros bytes de control se descartan", "\x01\x02j\x1f", []T{"j"}},
		{"byte invalido se descarta", "a\xffb", []T{"a", "b"}},
		{"mezcla", "w\x1b[Bhola\x15\r", []T{"w", tablero.TeclaAbajo, "h", "o", "l", "a", tablero.TeclaCtrlU, tablero.TeclaEnter}},
		{"vacio", "", nil},
	}
	for _, c := range casos {
		got := tablero.Decodificar([]byte(c.in))
		if len(got) != len(c.quiere) || (len(got) > 0 && !reflect.DeepEqual(got, c.quiere)) {
			t.Errorf("CA-532: Decodificar(%q) (%s) = %q, se esperaba %q", c.in, c.nombre, got, c.quiere)
		}
	}
}

// CA-532: propiedad: un texto de runas imprimibles se decodifica en una tecla
// por runa, cada una su propio texto.
func TestCA532_DecodificarPropiedadImprimibles(t *testing.T) {
	prop := func(rs []rune) bool {
		var b strings.Builder
		var quiere []tablero.Tecla
		for _, r := range rs {
			if !unicode.IsPrint(r) || r == utf8.RuneError || !utf8.ValidRune(r) {
				continue
			}
			b.WriteRune(r)
			quiere = append(quiere, tablero.Tecla(string(r)))
		}
		got := tablero.Decodificar([]byte(b.String()))
		return len(got) == len(quiere) && (len(got) == 0 || reflect.DeepEqual(got, quiere))
	}
	cfg := &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(532))}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("CA-532: cada runa imprimible es su propia tecla: %v", err)
	}
	// y no es vacuo
	if got := tablero.Decodificar([]byte("abc")); len(got) != 3 {
		t.Fatalf("CA-532: Decodificar(\"abc\") son tres teclas: %q", got)
	}
}

// ---------------------------------------------------------------- la tabla

var ttTodasLasVistas = []tablero.Vista{tablero.VistaLista, tablero.VistaBusqueda, tablero.VistaResumen, tablero.VistaSpec,
	tablero.VistaSobre, tablero.VistaHallazgos, tablero.VistaHistoria, tablero.VistaMenu, tablero.VistaFormulario,
	tablero.VistaConfirmacion, tablero.VistaAyuda}

// ttFilas son las filas de Atajos que valen en v, en el orden de la tabla
// (calculadas aca, no con AtajosDe).
func ttFilas(v tablero.Vista) []tablero.Atajo {
	var out []tablero.Atajo
	for _, a := range tablero.Atajos {
		for _, x := range a.Vistas {
			if x == v {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// CA-532: AtajosDe(v) son las filas de Atajos de v, en el orden de la tabla.
func TestCA532_AtajosDe(t *testing.T) {
	for _, v := range ttTodasLasVistas {
		quiere := ttFilas(v)
		if len(quiere) == 0 {
			t.Fatalf("CA-532: la vista %q tiene filas en Atajos", v)
		}
		if got := tablero.AtajosDe(v); !reflect.DeepEqual(got, quiere) {
			t.Fatalf("CA-532: AtajosDe(%q) son sus %d filas de Atajos en orden, dio %d:\n%#v", v, len(quiere), len(got), got)
		}
	}
	if got := tablero.AtajosDe("no-es-una-vista"); len(got) != 0 {
		t.Fatalf("CA-532: una vista desconocida no tiene teclas: %#v", got)
	}
}

// CA-532 (guarda de la tabla): en ninguna vista una tecla esta en dos
// filas: el despacho no puede elegir entre dos.
func TestCA532_SinTeclasRepetidasPorVista(t *testing.T) {
	for _, v := range ttTodasLasVistas {
		vista := map[tablero.Tecla]string{}
		for _, a := range ttFilas(v) {
			for _, k := range a.Teclas {
				if otra, ok := vista[k]; ok {
					t.Fatalf("CA-532: en %q la tecla %q esta en dos filas: %q y %q", v, k, otra, a.Hace)
				}
				vista[k] = a.Hace
			}
		}
	}
}

// CA-525, CA-532 (guarda de la tabla): las teclas de accion ejecutan los
// verbos de la tabla del spec.
func TestCA525_CA532_VerbosDeLaTabla(t *testing.T) {
	quiere := map[tablero.Tecla][2]string{
		"n": {"hoom siguiente <slug>", tablero.AccionSiguiente},
		"a": {"hoom pedir arquitecto <slug>", boardcmd.ActPedirArquitecto},
		"t": {"hoom pedir test-writer <slug>", boardcmd.ActPedirTestWriter},
		"w": {"hoom pedir writer <slug>", boardcmd.ActPedirWriter},
		"r": {"hoom pedir reviewer <slug>", boardcmd.ActPedirReviewer},
		"R": {"hoom reanudar <slug>", boardcmd.ActReanudar},
		"s": {"hoom guardar <slug>", boardcmd.ActGuardar},
		"A": {"hoom aprobar <slug> --si", boardcmd.ActAprobar},
		"I": {"hoom integrar <slug> --si", boardcmd.ActIntegrar},
		"u": {"hoom usar <slug>", tablero.AccionUsar},
	}
	for k, q := range quiere {
		var fila *tablero.Atajo
		for i, a := range ttFilas(tablero.VistaLista) {
			for _, x := range a.Teclas {
				if x == k {
					fila = &ttFilas(tablero.VistaLista)[i]
				}
			}
		}
		if fila == nil {
			t.Fatalf("CA-532: la tecla %q esta en la tabla de la lista", k)
		}
		if fila.Verbo != q[0] || fila.Accion != q[1] {
			t.Fatalf("CA-525: la tecla %q ejecuta %q (accion %q), la tabla dice %q (%q)", k, q[0], q[1], fila.Verbo, fila.Accion)
		}
	}
}

// ---------------------------------------------------------------- un estado rico

// ttRica es una tarjeta con todo: las ocho acciones habilitadas, treinta
// segmentos (un Resumen mas largo que la pantalla), y su detalle con un spec
// de miles de lineas, cuarenta hallazgos con archivo, sesenta eventos y
// sesenta entradas de historia.
func ttRica() (boardcmd.Card, tablero.Detalle) {
	var segs []boardcmd.Segment
	for i := 0; i < 30; i++ {
		segs = append(segs, ttSeg(fmt.Sprintf("s%02d", i), fmt.Sprintf("segmento %02d", i), boardcmd.SegHecho, "hecho"))
	}
	c := ttCard("tarjeta", "Tarjeta rica", boardcmd.ColWriter, "el plain de la rica", segs...)
	c.Evidence.Untraced = []string{"CA-2", "CA-5"}
	c.Evidence.Criteria = 6
	c.Actions = []boardcmd.Action{
		ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec"),
		ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests"),
		ttAccionWriter(),
		ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""),
		ttPagosCaidos().Actions[0],
		ttAccionGuardar(true, "", "internal/a.go"),
		ttAccionAprobar(),
		ttAccionIntegrar(),
	}
	md, cr := ttSpecLargo()
	var hs []finding.Item
	for i := 0; i < 40; i++ {
		hs = append(hs, ttHallazgo(fmt.Sprintf("f%02d", i), finding.StatusOpen, "low", fmt.Sprintf("x%02d.go:%d", i, i+1), fmt.Sprintf("hallazgo %02d", i)))
	}
	var evs []runcmd.Event
	var tl []boardcmd.TimelineEntry
	for i := 0; i < 60; i++ {
		evs = append(evs, ttEvento(-600+i*5, "text", fmt.Sprintf("evento %02d", i)))
		tl = append(tl, boardcmd.TimelineEntry{At: ttAhora.Add(time.Duration(i-60) * time.Hour), Kind: boardcmd.KindItem,
			Who: "Henry Orellana", Plain: fmt.Sprintf("entrada %02d", i), Summary: fmt.Sprintf("resumen %02d", i)})
	}
	ms := int64(125000)
	d := tablero.Detalle{Slug: c.Slug, Detalle: boardcmd.Detail{
		Card:     c,
		Spec:     boardcmd.SpecDetail{Path: ".hoom/specs/tarjeta.md", Exists: true, Markdown: md},
		Criteria: cr,
		Findings: hs,
		Work: []boardcmd.WorkRow{{Kind: boardcmd.WorkSobre, ID: "env-rica", RunID: "run-rica", Role: "writer", Provider: "claude",
			Status: "ok", Stage: "run", Step: 6, Steps: 6, StartedAt: ttAhora.Add(-time.Hour), DurationMS: &ms}},
	}, Historia: boardcmd.Timeline{Slug: c.Slug, Entries: tl}, Sobre: tablero.Sobre{EnvelopeID: "env-rica", RunID: "run-rica", Eventos: evs}}
	return c, d
}

// ttEstado es el modelo en la vista v con la tarjeta rica (en medio de otras
// dos y con una en Hecho), en UTF-8 a 120x24 (ttEstadoCon: con cfg).
func ttEstado(t *testing.T, v tablero.Vista) *tablero.Modelo {
	t.Helper()
	return ttEstadoCon(t, v, ttConfig(120, 24, true))
}

func ttEstadoCon(t *testing.T, v tablero.Vista, cfg tablero.Config) *tablero.Modelo {
	t.Helper()
	c, d := ttRica()
	uno := ttBacklog("uno", "Uno")
	uno.Column, uno.ColumnName = boardcmd.ColWriter, "Writer"
	tres := ttBacklog("tres", "Tres")
	tres.Column, tres.ColumnName = boardcmd.ColWriter, "Writer"
	m := ttModelo(cfg, ttTablero(uno, c, tres, ttViejo(1)))
	ir := func(teclas ...tablero.Tecla) {
		for _, k := range teclas {
			m.Tecla(k, ttAhora)
		}
	}
	switch v {
	case tablero.VistaLista:
	case tablero.VistaBusqueda:
		ir("/")
	case tablero.VistaMenu:
		ir("j", tablero.TeclaEspacio)
	case tablero.VistaFormulario:
		ir("j", "w")
	case tablero.VistaConfirmacion:
		ir("j", "A")
	case tablero.VistaAyuda:
		ir("?")
	default:
		ir("j", tablero.TeclaEnter)
		m.LeerDetalle(d)
		for i, p := range tablero.Pestanas {
			if p == v {
				ir(tablero.Tecla(string(rune('1' + i))))
			}
		}
		m.LeerDetalle(d)
	}
	if m.Vista() != v {
		t.Fatalf("no pude llegar a la vista %q (estoy en %q):\n%s", v, m.Vista(), ttPantalla(m))
	}
	return m
}

// ttPrepara deja el estado donde la tecla k de v tiene algo que hacer: arriba
// para bajar, abajo para subir, el cursor lejos del borde que mueve.
func ttPrepara(m *tablero.Modelo, v tablero.Vista, k tablero.Tecla) {
	ir := func(teclas ...tablero.Tecla) {
		for _, x := range teclas {
			m.Tecla(x, ttAhora)
		}
	}
	esPestana := false
	for _, p := range tablero.Pestanas {
		esPestana = esPestana || p == v
	}
	switch {
	case v == tablero.VistaLista || v == tablero.VistaMenu:
		if k == "k" || k == tablero.TeclaArriba || k == "g" {
			ir("j")
		}
	case v == tablero.VistaBusqueda:
		switch k {
		case tablero.TeclaBorrar:
			ir("t")
		case tablero.TeclaArriba:
			ir(tablero.TeclaAbajo)
		}
	case esPestana:
		switch k {
		case "j", tablero.TeclaAbajo, "G", "]c":
			ir("g")
		case "k", tablero.TeclaArriba, "g", "[c":
			ir("G")
		case tablero.TeclaEnter:
			ir("g")
		}
	case v == tablero.VistaFormulario:
		if k == tablero.TeclaDer {
			ir(tablero.TeclaIzq)
		}
	case v == tablero.VistaConfirmacion:
		if k == tablero.TeclaIzq {
			ir(tablero.TeclaDer)
		}
	}
}

// ttApretar aprieta k ("]c" es "]" y despues "c"; TeclaTexto es una letra).
func ttApretar(m *tablero.Modelo, k tablero.Tecla) []tablero.Orden {
	switch k {
	case "]c", "[c":
		return ttEscribir(m, string(k))
	case tablero.TeclaTexto:
		return m.Tecla("z", ttAhora)
	}
	return m.Tecla(k, ttAhora)
}

// CA-532, propiedad positiva: toda tecla de la ayuda tiene efecto. En cada
// vista, cada tecla de cada fila de AtajosDe, en un estado donde tiene algo
// que hacer, cambia la pantalla, la vista o da una orden. (El numero de la
// propia pestana se salta: elegir la pestana que ya esta elegida no tiene
// efecto que ver.)
func TestCA532_TodaTeclaDeLaAyudaTieneEfecto(t *testing.T) {
	probadas := 0
	for _, v := range ttTodasLasVistas {
		filas := tablero.AtajosDe(v)
		if len(filas) == 0 {
			t.Fatalf("CA-532: AtajosDe(%q) no esta vacio", v)
		}
		for _, a := range filas {
			for _, k := range a.Teclas {
				if propia := ttNumeroDePestana(v); propia != "" && k == propia {
					continue
				}
				m := ttEstado(t, v)
				ttPrepara(m, v, k)
				antes, vista := ttPantalla(m), m.Vista()
				os := ttApretar(m, k)
				if len(os) == 0 && ttPantalla(m) == antes && m.Vista() == vista {
					t.Fatalf("CA-532: en %q la tecla %q (%s) esta en la ayuda y no tuvo efecto:\n%s", v, k, a.Hace, antes)
				}
				probadas++
			}
		}
	}
	if probadas < 100 {
		t.Fatalf("CA-532: se probaron %d teclas, se esperaban todas las de la tabla", probadas)
	}
}

func ttNumeroDePestana(v tablero.Vista) tablero.Tecla {
	for i, p := range tablero.Pestanas {
		if p == v {
			return tablero.Tecla(string(rune('1' + i)))
		}
	}
	return ""
}

// ttTeclasEspeciales son las teclas que no son texto.
var ttTeclasEspeciales = []tablero.Tecla{tablero.TeclaEnter, tablero.TeclaEsc, tablero.TeclaTab, tablero.TeclaShiftTab,
	tablero.TeclaArriba, tablero.TeclaAbajo, tablero.TeclaIzq, tablero.TeclaDer, tablero.TeclaBorrar, tablero.TeclaCtrlC,
	tablero.TeclaCtrlU, tablero.TeclaCtrlE, tablero.TeclaEspacio}

// ttEnLaTabla dice si k esta en alguna fila de v (una runa imprimible esta
// en la fila de TeclaTexto, si v la tiene; "]" y "[" son el primer golpe de
// "]c" y "[c").
func ttEnLaTabla(v tablero.Vista, k tablero.Tecla) bool {
	r, n := utf8.DecodeRuneInString(string(k))
	esTexto := n == len(k) && unicode.IsPrint(r)
	for _, a := range ttFilas(v) {
		for _, x := range a.Teclas {
			dosGolpes := (x == "]c" || x == "[c") && string(x[0]) == string(k)
			if x == k || (x == tablero.TeclaTexto && esTexto) || dosGolpes {
				return true
			}
		}
	}
	return false
}

// CA-532, propiedad negativa: toda tecla con efecto esta en la ayuda. En
// cada vista, una tecla que no esta en ninguna fila de la vista no cambia
// nada: ni la pantalla, ni la vista, ni da ordenes. Las teclas especiales
// todas, y runas imprimibles al azar.
func TestCA532_NingunaTeclaFueraDeLaTablaTieneEfecto(t *testing.T) {
	sinEfecto := func(v tablero.Vista, k tablero.Tecla) error {
		m := ttEstado(t, v)
		antes, vista := ttPantalla(m), m.Vista()
		os := m.Tecla(k, ttAhora)
		if len(os) != 0 || ttPantalla(m) != antes || m.Vista() != vista {
			return fmt.Errorf("en %q la tecla %q no esta en la tabla y tuvo efecto (ordenes %#v, vista %q):\n%s", v, k, os, m.Vista(), ttPantalla(m))
		}
		return nil
	}
	probadas := 0
	for _, v := range ttTodasLasVistas {
		for _, k := range ttTeclasEspeciales {
			if ttEnLaTabla(v, k) {
				continue
			}
			if err := sinEfecto(v, k); err != nil {
				t.Fatalf("CA-532: %v", err)
			}
			probadas++
		}
		for _, r := range "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&()*+,-.:;<=>@[]^_{|}~ñé" {
			k := tablero.Tecla(string(r))
			if ttEnLaTabla(v, k) {
				continue
			}
			if err := sinEfecto(v, k); err != nil {
				t.Fatalf("CA-532: %v", err)
			}
			probadas++
		}
	}
	if probadas < 60 {
		t.Fatalf("CA-532: se probaron solo %d teclas fuera de la tabla", probadas)
	}
	// runas al azar en la lista y en una pestana
	for _, v := range []tablero.Vista{tablero.VistaLista, tablero.VistaSpec, tablero.VistaMenu, tablero.VistaAyuda} {
		prop := func(r rune) bool {
			r = r % 0x3000
			if !unicode.IsPrint(r) || r == ' ' {
				return true
			}
			k := tablero.Tecla(string(r))
			if ttEnLaTabla(v, k) {
				return true
			}
			return sinEfecto(v, k) == nil
		}
		cfg := &quick.Config{MaxCount: 60, Rand: rand.New(rand.NewSource(int64(len(v))))}
		if err := quick.Check(prop, cfg); err != nil {
			t.Fatalf("CA-532: en %q una runa fuera de la tabla tuvo efecto: %v", v, err)
		}
	}
}

// ---------------------------------------------------------------- la ayuda y la barra 2

func ttPad(s string, n int) string {
	if k := utf8.RuneCountInString(s); k < n {
		return s + strings.Repeat(" ", n-k)
	}
	return s
}

// CA-532: ? abre la ayuda de la vista de donde se abrio: " Teclas de
// <vista>" y una linea por fila de AtajosDe (Muestra a 14 celdas, Hace, y
// el Verbo cuando tiene); esc y ? la cierran y vuelven a esa vista.
func TestCA532_LaAyudaListaLaTablaDeLaVista(t *testing.T) {
	desde := []tablero.Vista{tablero.VistaLista, tablero.VistaResumen, tablero.VistaSpec, tablero.VistaSobre,
		tablero.VistaHallazgos, tablero.VistaHistoria, tablero.VistaMenu}
	for _, v := range desde {
		for _, cerrar := range []tablero.Tecla{tablero.TeclaEsc, "?"} {
			m := ttEstadoCon(t, v, ttConfig(120, 40, true))
			ttSinOrdenes(t, "CA-532", "?", m.Tecla("?", ttAhora))
			if m.Vista() != tablero.VistaAyuda {
				t.Fatalf("CA-532: ? desde %q abre la ayuda, la vista es %q", v, m.Vista())
			}
			cuerpo := ttCuerpo(m)
			if len(cuerpo) == 0 || cuerpo[0] != " Teclas de "+string(v) {
				t.Fatalf("CA-532: la ayuda empieza con ' Teclas de %s':\n%s", v, ttPantalla(m))
			}
			filas := ttFilas(v)
			if len(cuerpo) < len(filas)+1 {
				t.Fatalf("CA-532: la ayuda de %q tiene una linea por fila (%d):\n%s", v, len(filas), ttPantalla(m))
			}
			for i, a := range filas {
				base := " " + ttPad(a.Muestra, 14) + a.Hace
				got := cuerpo[i+1]
				ok := got == base
				if a.Verbo != "" {
					ok = got == base+"  "+a.Verbo || got == base+"  "+strings.ReplaceAll(a.Verbo, "<slug>", "tarjeta")
				}
				if !ok {
					t.Fatalf("CA-532: la linea %d de la ayuda de %q es %q, se esperaba %q (+ su verbo %q)", i+1, v, got, base, a.Verbo)
				}
			}
			if b := ttBarra2(m); b != " esc cerrar" {
				t.Fatalf("CA-532: la barra 2 de la ayuda es ' esc cerrar': %q", b)
			}
			ttSinOrdenes(t, "CA-532", string(cerrar)+" en la ayuda", m.Tecla(cerrar, ttAhora))
			if m.Vista() != v {
				t.Fatalf("CA-532: %q cierra la ayuda y vuelve a %q, la vista es %q", cerrar, v, m.Vista())
			}
		}
	}
}

// CA-532: la barra 2 son los Barra de las filas de la vista, en el orden de
// la tabla, unidos por " · ".
func TestCA532_LaBarra2SaleDeLaTabla(t *testing.T) {
	for _, v := range ttTodasLasVistas {
		m := ttEstado(t, v)
		var pistas []string
		for _, a := range ttFilas(v) {
			if a.Barra != "" {
				pistas = append(pistas, a.Barra)
			}
		}
		if quiere := " " + strings.Join(pistas, " · "); ttBarra2(m) != quiere {
			t.Fatalf("CA-532: la barra 2 de %q es\n %q\nse esperaba\n %q", v, ttBarra2(m), quiere)
		}
	}
}
