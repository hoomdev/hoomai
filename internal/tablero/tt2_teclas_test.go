// Las teclas (spec tablero-con-paneles CA-564, que re-expresa CA-532 de la
// v1; y CA-533): Decodificar (sin cambios de la v1), la tabla Atajos por
// Lugar, la linea de teclas y la ayuda desde la misma tabla, y las dos
// propiedades del contrato: toda tecla de la ayuda tiene efecto y ninguna
// tecla fuera de la tabla lo tiene.
package tablero_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"regexp"
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

// ---------------------------------------------------------------- Decodificar (v1, sin cambios)

// CA-532, CA-533: Decodificar convierte los bytes de la terminal cruda en
// teclas, como dice su contrato (los casos limite incluidos); un evento del
// mouse no es una tecla (CA-575).
func TestCA532_CA575_Decodificar(t *testing.T) {
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
		{"pgup y pgdn", "\x1b[5~\x1b[6~", []T{tablero.TeclaPgArriba, tablero.TeclaPgAbajo}},
		{"esc solo al final", "\x1b", []T{tablero.TeclaEsc}},
		{"esc y letra", "\x1bx", []T{tablero.TeclaEsc, "x"}},
		{"dos esc", "\x1b\x1b", []T{tablero.TeclaEsc, tablero.TeclaEsc}},
		{"esc y enter", "\x1b\r", []T{tablero.TeclaEsc, tablero.TeclaEnter}},
		{"CSI desconocida se descarta", "\x1b[15~j", []T{"j"}},
		{"pegado con corchetes", "\x1b[200~hola\x1b[201~", []T{"h", "o", "l", "a"}},
		{"otros bytes de control se descartan", "\x01\x02j\x1f", []T{"j"}},
		{"byte invalido se descarta", "a\xffb", []T{"a", "b"}},
		{"mezcla", "w\x1b[Bhola\x15\r", []T{"w", tablero.TeclaAbajo, "h", "o", "l", "a", tablero.TeclaCtrlU, tablero.TeclaEnter}},
		{"un clic SGR no es tecla", "a\x1b[<0;10;5Mb", []T{"a", "b"}},
		{"un evento X10 se descarta entero", "a\x1b[M !!b", []T{"a", "b"}},
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
	if err := quick.Check(prop, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(532))}); err != nil {
		t.Fatalf("CA-532: cada runa imprimible es su propia tecla: %v", err)
	}
}

// ---------------------------------------------------------------- la tabla (CA-564)

// ttFilas son las filas de Atajos que valen en l, en el orden de la tabla
// (calculadas aca, no con AtajosDe).
func ttFilas(l tablero.Lugar) []tablero.Atajo {
	var out []tablero.Atajo
	for _, a := range tablero.Atajos {
		for _, x := range a.Lugares {
			if x == l {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// CA-564, CA-532: AtajosDe(l) son las filas de Atajos de l, en el orden de la
// tabla; cada Lugar tiene filas; un Lugar desconocido no tiene.
func TestCA564_CA532_AtajosDe(t *testing.T) {
	for _, l := range tablero.LugaresTodos {
		quiere := ttFilas(l)
		if len(quiere) == 0 {
			t.Fatalf("CA-564: el Lugar %q tiene filas en Atajos", l)
		}
		if got := tablero.AtajosDe(l); !reflect.DeepEqual(got, quiere) {
			t.Fatalf("CA-564: AtajosDe(%q) son sus %d filas en orden, dio %d", l, len(quiere), len(got))
		}
	}
	if got := tablero.AtajosDe("no-es-un-lugar"); len(got) != 0 {
		t.Fatalf("CA-564: un Lugar desconocido no tiene teclas: %#v", got)
	}
	if len(tablero.LugaresTodos) != 19 {
		t.Fatalf("CA-564 (guarda): los Lugares son los 19 del contrato: %d", len(tablero.LugaresTodos))
	}
}

// CA-564 (guarda de la tabla): en ningun Lugar una tecla esta en dos filas:
// el despacho no puede elegir entre dos.
func TestCA564_SinTeclasRepetidasPorLugar(t *testing.T) {
	for _, l := range tablero.LugaresTodos {
		visto := map[tablero.Tecla]string{}
		for _, a := range ttFilas(l) {
			for _, k := range a.Teclas {
				if otra, ok := visto[k]; ok {
					t.Fatalf("CA-564: en %q la tecla %q esta en dos filas: %q y %q", l, k, otra, a.Hace)
				}
				visto[k] = a.Hace
			}
		}
	}
}

// CA-564, CA-525, CA-532: las teclas de accion ejecutan los verbos de la tabla
// del spec, con sus acciones; n es la accion del siguiente paso, sin verbo
// propio (en la v1 ejecutaba hoom siguiente: CA-566 lo re-expresa).
func TestCA564_CA525_VerbosDeLaTabla(t *testing.T) {
	quiere := map[tablero.Tecla][2]string{
		"n": {"", tablero.AccionSiguiente},
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
		for i, a := range ttFilas(tablero.LugarTarjetas) {
			for _, x := range a.Teclas {
				if x == k {
					f := ttFilas(tablero.LugarTarjetas)[i]
					fila = &f
				}
			}
		}
		if fila == nil {
			t.Fatalf("CA-564: la tecla %q esta en la tabla de [2]", k)
		}
		if fila.Verbo != q[0] || fila.Accion != q[1] {
			t.Fatalf("CA-525: la tecla %q ejecuta %q (accion %q), la tabla dice %q (%q)", k, q[0], q[1], fila.Verbo, fila.Accion)
		}
	}
}

// ---------------------------------------------------------------- la linea de teclas

// ttAscii es una pista o una Muestra como la escribe la pantalla en ASCII.
func ttAscii(s string) string {
	return strings.NewReplacer("↑", "^", "↓", "v", "←", "<", "→", ">").Replace(s)
}

// ttLineaTeclasQuiere es la linea de teclas de l segun doc.go: los Barra de
// sus filas en orden, con la pista del siguiente paso despues de "n
// siguiente paso" (sig, "" sin ella), sin las de la tarjeta si no hay
// tarjeta, y el ajuste al ancho.
func ttLineaTeclasQuiere(l tablero.Lugar, conTarjeta bool, sig string, ancho int, utf bool) string {
	var pistas []string
	for _, a := range ttFilas(l) {
		if a.Barra == "" {
			continue
		}
		if !conTarjeta && (a.Accion != "" || (len(a.Teclas) == 1 && a.Teclas[0] == tablero.TeclaEspacio)) {
			continue
		}
		b := a.Barra
		if !utf {
			b = ttAscii(b)
		}
		pistas = append(pistas, b)
		if a.Accion == tablero.AccionSiguiente && sig != "" {
			pistas = append(pistas, sig)
		}
	}
	mide := func(ps []string) int { return ttCeldas(" " + strings.Join(ps, " · ")) }
	if mide(pistas) > ancho && len(pistas) > 0 && pistas[len(pistas)-1] == "? todas las teclas" {
		pistas[len(pistas)-1] = "? todas"
	}
	for mide(pistas) > ancho && len(pistas) > 1 {
		pistas = append(pistas[:len(pistas)-2], pistas[len(pistas)-1])
	}
	return " " + strings.Join(pistas, " · ")
}

// CA-564: la ultima linea muestra, en palabras, solo las teclas del lugar
// con foco o del modal abierto, desde la tabla; con el ajuste al ancho de
// doc.go (los dos dibujos del spec: a 100 columnas entera, a 60 con "?
// todas"). Re-expresa CA-532 (la barra 2 de la v1).
func TestCA564_CA532_LaLineaDeTeclas(t *testing.T) {
	// los dos dibujos del contrato
	m := ttModeloDorado(ttConfig(100, 30, true))
	if got := ttLineaTeclas(m); got != " ↑↓ elegir · enter ver · espacio acciones · n siguiente paso · A aprobar spec · ? todas las teclas" {
		t.Fatalf("CA-564: a 100 columnas, [2] en Tu aprobacion: %q", got)
	}
	m = ttModeloDorado(ttConfig(60, 15, true))
	if got := ttLineaTeclas(m); got != " ↑↓ elegir · enter ver · espacio acciones · ? todas" {
		t.Fatalf("CA-564: a 60 columnas: %q", got)
	}
	m = ttModeloDorado(ttConfig(80, 24, false))
	if got := ttLineaTeclas(m); got != " ^v elegir · enter ver · espacio acciones · n siguiente paso · ? todas" {
		t.Fatalf("CA-564: a 80 columnas en ASCII: %q", got)
	}
	// cada lugar, a 200 columnas (todo entra)
	casos := []struct {
		lugar  tablero.Lugar
		teclas []tablero.Tecla
		tarj   bool
		sig    string
	}{
		{tablero.LugarTarjetas, nil, true, "A aprobar spec"},
		{tablero.LugarTarjetas, []tablero.Tecla{"G"}, false, ""},
		{tablero.LugarProyecto, []tablero.Tecla{"1"}, false, ""},
		{tablero.LugarCorriendo, []tablero.Tecla{"3"}, true, ""},
		{tablero.LugarHallazgos, []tablero.Tecla{"4"}, false, ""},
		{tablero.LugarVerTarjeta, []tablero.Tecla{tablero.TeclaEnter}, true, "A aprobar spec"},
		{tablero.LugarVerProyecto, []tablero.Tecla{"1", tablero.TeclaEnter}, false, ""},
		{tablero.LugarVerHallazgo, []tablero.Tecla{"4", tablero.TeclaEnter}, false, ""},
		{tablero.LugarVerNada, []tablero.Tecla{"G", "0"}, false, ""},
		{tablero.LugarBusqueda, []tablero.Tecla{"/"}, false, ""},
		{tablero.LugarAcciones, []tablero.Tecla{tablero.TeclaEspacio}, true, "A aprobar spec"},
		{tablero.LugarFirma, []tablero.Tecla{"A"}, false, ""},
		{tablero.LugarAyuda, []tablero.Tecla{"?"}, false, ""},
	}
	for _, c := range casos {
		m := ttModeloDorado(ttConfig(200, 50, false))
		ttTeclas(m, c.teclas...)
		if m.Lugar() != c.lugar {
			t.Fatalf("CA-564 (guarda): %v lleva a %q, llevo a %q", c.teclas, c.lugar, m.Lugar())
		}
		if got, q := ttLineaTeclas(m), ttLineaTeclasQuiere(c.lugar, c.tarj, c.sig, 200, false); got != q {
			t.Fatalf("CA-564: la linea de teclas de %q es\n %q\nse esperaba\n %q", c.lugar, got, q)
		}
	}
	// el modal Pedir: el dibujo del spec
	p := ttPedir(t, "w", ttAccionWriter())
	p.Tecla(tablero.TeclaTab, ttAhora)
	if got := ttLineaTeclas(p); got != " tab campo · <> cambiar · enter ver opciones · ctrl-e editor · esc cerrar" {
		t.Fatalf("CA-564: la linea de teclas de un selector (el dibujo del spec): %q", got)
	}
	for _, l := range []tablero.Lugar{tablero.LugarPedirPedido, tablero.LugarPedirBotones, tablero.LugarPedirLista, tablero.LugarPedirTexto} {
		m := ttPedir(t, "w", ttAccionWriter())
		switch l {
		case tablero.LugarPedirBotones:
			m.Tecla(tablero.TeclaEnter, ttAhora)
		case tablero.LugarPedirLista:
			ttTeclas(m, tablero.TeclaTab, tablero.TeclaEnter)
		case tablero.LugarPedirTexto:
			ttTeclas(m, tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaEnter)
			for i := 0; i < 20; i++ {
				m.Tecla(tablero.TeclaAbajo, ttAhora)
			}
			m.Tecla(tablero.TeclaEnter, ttAhora)
		}
		if m.Lugar() != l {
			t.Fatalf("CA-564 (guarda): no llegue a %q: %q", l, m.Lugar())
		}
		if got, q := ttLineaTeclas(m), ttLineaTeclasQuiere(l, false, "", 200, false); got != q {
			t.Fatalf("CA-564: la linea de teclas de %q es\n %q\nse esperaba\n %q", l, got, q)
		}
	}
}

// ---------------------------------------------------------------- la ayuda

// CA-564: ? abre el modal de la ayuda ("Todas las teclas") con todas las
// teclas agrupadas: primero el grupo del lugar con foco, despues "En todas
// partes", los otros paneles, "Modales" y "Mouse"; cada fila "  " + Muestra
// rellena a 18 + Hace (+ su verbo); el grupo Mouse dice como seleccionar
// texto (o, sin mouse, "apagado: HOOM_MOUSE=0"); esc y ? la cierran.
// Re-expresa CA-532.
func TestCA564_CA532_LaAyuda(t *testing.T) {
	grupos := []string{"[2] Tarjetas", "En todas partes", "[1] Proyecto", "[3] Corriendo", "[4] Hallazgos", "[0] el proyecto",
		"[0] una tarjeta", "[0] lo que corre", "[0] un hallazgo", "Modales", "Mouse"}
	for _, mouse := range []bool{true, false} {
		cfg := ttConfig(200, 300, false)
		cfg.Mouse = mouse
		m := ttModelo(cfg, ttTableroDorado())
		ttSinOrdenes(t, "CA-564", "?", m.Tecla("?", ttAhora))
		if m.Modal() != tablero.ModalAyuda || m.Lugar() != tablero.LugarAyuda {
			t.Fatalf("CA-564: ? abre la ayuda: %q %q", m.Modal(), m.Lugar())
		}
		mc := ttModal(t, "CA-564", m, 200)
		if !strings.HasPrefix(mc.Arriba, "# Todas las teclas ") {
			t.Fatalf("CA-564: el titulo de la ayuda es 'Todas las teclas': %q", mc.Arriba)
		}
		pos := -1
		for _, g := range grupos {
			i := -1
			for k, l := range mc.Textos {
				if l == g && k > pos {
					i = k
					break
				}
			}
			if i < 0 {
				t.Fatalf("CA-564: la ayuda tiene el grupo %q despues del anterior:\n%s", g, strings.Join(mc.Textos, "\n"))
			}
			if i > 0 && mc.Textos[i-1] != "" {
				t.Fatalf("CA-564: una linea vacia entre grupos (antes de %q)", g)
			}
			pos = i
		}
		todo := "\n" + strings.Join(mc.Textos, "\n") + "\n"
		for _, a := range tablero.Atajos {
			// las lineas del modal tienen Wm-6 = 66 celdas: una fila con su
			// verbo puede cortarse; su comienzo esta
			base := "  " + fmt.Sprintf("%-18s", ttAscii(a.Muestra)) + a.Hace
			if len(base) > 60 {
				base = base[:60]
			}
			if !strings.Contains(todo, "\n"+base) {
				t.Fatalf("CA-564: toda fila de la tabla esta en la ayuda: %q (+ %q):\n%s", base, a.Verbo, todo)
			}
		}
		if mouse {
			for _, s := range []string{"  clic              foco al panel; en una fila, la elige",
				"  rueda             mueve la seleccion; en [0] y en la ayuda",
				"  clic en un modal  enfoca un campo, elige una opcion o un boton",
				"  para seleccionar texto con el mouse, manten Option (macOS)"} {
				if !strings.Contains(todo, "\n"+s) {
					t.Fatalf("CA-564, CA-575: el grupo Mouse dice %q:\n%s", s, todo)
				}
			}
		} else if !strings.Contains(todo, "\nMouse\n  apagado: HOOM_MOUSE=0\n") {
			t.Fatalf("CA-564, CA-575: sin mouse, el grupo Mouse dice 'apagado: HOOM_MOUSE=0':\n%s", todo)
		}
		for _, cerrar := range []tablero.Tecla{tablero.TeclaEsc, "?"} {
			ttSinOrdenes(t, "CA-564", string(cerrar), m.Tecla(cerrar, ttAhora))
			if m.Modal() != tablero.SinModal || m.Lugar() != tablero.LugarTarjetas {
				t.Fatalf("CA-564: %q cierra la ayuda: %q", cerrar, m.Modal())
			}
			m.Tecla("?", ttAhora)
		}
	}
	// el primer grupo es el del lugar de donde se abrio
	for _, c := range []struct {
		teclas []tablero.Tecla
		grupo  string
	}{{[]tablero.Tecla{"3"}, "[3] Corriendo"}, {[]tablero.Tecla{"4", tablero.TeclaEnter}, "[0] un hallazgo"},
		{[]tablero.Tecla{"G", "0"}, "[0]"}, {[]tablero.Tecla{"1"}, "[1] Proyecto"}} {
		m := ttModeloDorado(ttConfig(200, 300, false))
		ttTeclas(m, c.teclas...)
		m.Tecla("?", ttAhora)
		mc := ttModal(t, "CA-564", m, 200)
		if len(mc.Textos) == 0 || mc.Textos[0] != c.grupo {
			t.Fatalf("CA-564: abierta desde %v, el primer grupo es %q: %q", c.teclas, c.grupo, mc.Textos[:min(3, len(mc.Textos))])
		}
	}
}

// CA-564: la ayuda que no entra ocupa B lineas, su borde de abajo dice "<n>
// de <m>" (n la ultima linea a la vista, m todas) y se desplaza con ↓ j ↑ k
// (una linea) y pgdn pgup (su alto interior), entre su principio y su final.
func TestCA564_LaAyudaSeDesplaza(t *testing.T) {
	m := ttModeloDorado(ttConfig(80, 24, false))
	m.Tecla("?", ttAhora)
	mc := ttModal(t, "CA-564", m, 80)
	if mc.Alto != 22 || mc.Fila != 0 {
		t.Fatalf("CA-564: la ayuda que no entra ocupa las B=22 lineas: fila %d, alto %d", mc.Fila, mc.Alto)
	}
	if !strings.Contains(mc.Abajo, "=20 de ") {
		t.Fatalf("CA-564: el borde de abajo dice '20 de <m>' (20 lineas interiores): %q", mc.Abajo)
	}
	primera := mc.Textos[0]
	m.Tecla("j", ttAhora)
	if mc := ttModal(t, "CA-564", m, 80); mc.Textos[0] == primera || !strings.Contains(mc.Abajo, "=21 de ") {
		t.Fatalf("CA-564: j baja una linea: %q %q", mc.Textos[0], mc.Abajo)
	}
	m.Tecla("k", ttAhora)
	m.Tecla("k", ttAhora)
	if mc := ttModal(t, "CA-564", m, 80); mc.Textos[0] != primera {
		t.Fatalf("CA-564: k en la primera no sube mas: %q", mc.Textos[0])
	}
	m.Tecla(tablero.TeclaPgAbajo, ttAhora)
	if mc := ttModal(t, "CA-564", m, 80); !strings.Contains(mc.Abajo, "=40 de ") {
		t.Fatalf("CA-564: pgdn baja su alto interior (20): %q", mc.Abajo)
	}
	m.Tecla(tablero.TeclaPgArriba, ttAhora)
	if mc := ttModal(t, "CA-564", m, 80); mc.Textos[0] != primera {
		t.Fatalf("CA-564: pgup vuelve: %q", mc.Textos[0])
	}
	for i := 0; i < 500; i++ {
		m.Tecla(tablero.TeclaPgAbajo, ttAhora)
	}
	mc = ttModal(t, "CA-564", m, 80)
	g := regexp.MustCompile(`([0-9]+) de ([0-9]+)`).FindStringSubmatch(mc.Abajo)
	if g == nil || g[1] != g[2] {
		t.Fatalf("CA-564: al final del todo el borde dice '<m> de <m>': %q", mc.Abajo)
	}
	m.Tecla("j", ttAhora)
	if otra := ttModal(t, "CA-564", m, 80); otra.Abajo != mc.Abajo {
		t.Fatalf("CA-564: en el final j no baja mas: %q", otra.Abajo)
	}
}

// ---------------------------------------------------------------- los estados para las propiedades

// ttEstado es un modelo en el Lugar l, en un estado donde cada tecla de la
// tabla puede tener efecto: paneles con varias filas, [0] mas largos que su
// vista, una ayuda que se desplaza.
func ttEstado(t *testing.T, l tablero.Lugar) *tablero.Modelo {
	t.Helper()
	c, d := ttTarjetaLarga()
	c.Actions = append(c.Actions, ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""))
	d.Detalle.Card = c
	corre := ttCorre("corre", "writer", "env-c", "run-c")
	otra := ttCorre("otra", "reviewer", "env-o", "run-o")
	m := ttModelo(ttConfig(80, 24, false), ttTablero(c, corre, otra, ttBacklog("uno", "Uno"), ttViejo(1)))
	p := ttProyectoVerde()
	for i := 0; i < 30; i++ {
		p.Problemas = append(p.Problemas, boardcmd.Problem{ID: boardcmd.ProbSinSpec, Slug: fmt.Sprintf("p%02d", i), What: "sin spec", Action: "hoom pedir arquitecto x"})
	}
	m.LeerProyecto(p)
	h := ttHallazgo("h1", finding.StatusOpen, "high", "a.go:3", strings.Repeat("linea de la descripcion\n", 30))
	m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h, ttHallazgo("h2", finding.StatusOpen, "low", "b.go", "d")}})
	ttIrA(t, m, c.Slug)
	m.LeerDetalle(d)
	ir := func(ks ...tablero.Tecla) { ttTeclas(m, ks...) }
	switch l {
	case tablero.LugarTarjetas:
	case tablero.LugarProyecto:
		ir("1")
	case tablero.LugarCorriendo:
		ir("3")
	case tablero.LugarHallazgos:
		ir("4")
	case tablero.LugarVerProyecto:
		ir("1", tablero.TeclaEnter)
	case tablero.LugarVerTarjeta:
		ir(tablero.TeclaEnter)
	case tablero.LugarVerCorriendo:
		ir("3")
		m.LeerDetalle(tablero.Detalle{Slug: "corre", Sobre: tablero.Sobre{EnvelopeID: "env-c", Eventos: ttEventos(40)}})
		ir(tablero.TeclaEnter)
	case tablero.LugarVerHallazgo:
		ir("4", tablero.TeclaEnter)
	case tablero.LugarVerNada:
		ir("G", "0")
	case tablero.LugarBusqueda:
		ir("/")
	case tablero.LugarPedirPedido:
		ir("w")
	case tablero.LugarPedirSelector:
		ir("w", tablero.TeclaTab, tablero.TeclaTab)
	case tablero.LugarPedirLista:
		ir("w", tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaEnter)
	case tablero.LugarPedirTexto:
		ir("w", tablero.TeclaTab, tablero.TeclaTab, tablero.TeclaEnter)
		for i := 0; i < 20; i++ {
			m.Tecla(tablero.TeclaAbajo, ttAhora)
		}
		ir(tablero.TeclaEnter)
	case tablero.LugarPedirBotones:
		ir("w", tablero.TeclaEnter)
	case tablero.LugarFirma:
		ir("A")
	case tablero.LugarGuardar:
		for i := range c.Actions {
			if c.Actions[i].ID == boardcmd.ActGuardar {
				c.Actions[i] = ttAccionGuardar(true, "", "a.go")
			}
		}
		m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(c, corre, otra, ttBacklog("uno", "Uno"), ttViejo(1))})
		ir("s")
	case tablero.LugarAcciones:
		ir(tablero.TeclaEspacio)
	case tablero.LugarAyuda:
		ir("?")
	}
	if m.Lugar() != l {
		t.Fatalf("CA-564 (guarda): no pude llegar a %q (estoy en %q)\n%s", l, m.Lugar(), ttPantalla(m))
	}
	return m
}

// ttPrepara deja m donde la tecla k tiene que tener efecto: lejos del
// principio para subir, con texto para borrar, con el cursor atras para ir
// adelante.
func ttPrepara(m *tablero.Modelo, l tablero.Lugar, k tablero.Tecla) {
	texto := l == tablero.LugarPedirTexto || l == tablero.LugarBusqueda
	switch {
	case l == tablero.LugarVerCorriendo && (k == "j" || k == tablero.TeclaAbajo || k == "G" || k == tablero.TeclaPgAbajo):
		m.Tecla(tablero.TeclaPgArriba, ttAhora)
	case (k == "k" || k == tablero.TeclaArriba || k == "g" || k == tablero.TeclaPgArriba) && !texto && l != tablero.LugarPedirSelector &&
		l != tablero.LugarPedirLista && l != tablero.LugarPedirPedido:
		m.Tecla(tablero.TeclaPgAbajo, ttAhora)
		m.Tecla("j", ttAhora)
	case l == tablero.LugarBusqueda && k == tablero.TeclaArriba:
		m.Tecla(tablero.TeclaAbajo, ttAhora)
	case texto && (k == tablero.TeclaBorrar || k == tablero.TeclaIzq || k == tablero.TeclaCtrlU):
		ttEscribir(m, "ab")
	case texto && k == tablero.TeclaDer:
		ttEscribir(m, "ab")
		m.Tecla(tablero.TeclaIzq, ttAhora)
	case l == tablero.LugarPedirPedido && k == tablero.TeclaDer:
		m.Tecla(tablero.TeclaIzq, ttAhora)
	}
}

// ttApretar aprieta k ("]c" es "]" y despues "c"; TeclaTexto es una letra).
func ttApretar(m *tablero.Modelo, k tablero.Tecla) []tablero.Orden {
	switch k {
	case "]c", "[c":
		return ttTeclas(m, tablero.Tecla(k[:1]), "c")
	case tablero.TeclaTexto:
		return m.Tecla("z", ttAhora)
	}
	return m.Tecla(k, ttAhora)
}

// ttFoto es todo lo que una tecla puede cambiar.
func ttFoto(m *tablero.Modelo) string {
	return fmt.Sprintf("%s|%s|%d|%s|%d\n%s", m.Lugar(), m.Modal(), int(m.Foco()), m.Elegida(), len(m.Registro()), ttPantalla(m))
}

// CA-564, propiedad: toda tecla de la ayuda tiene efecto. En cada Lugar, cada
// tecla de cada una de sus filas (desde un estado donde puede tenerlo)
// cambia algo: la pantalla, el Lugar, el modal, el foco, el Registro, o da
// una orden. Re-expresa CA-532. Quedan afuera la tecla del panel que ya
// tiene el foco y las de desplazar en [0] sin nada que mostrar (pregunta
// abierta del informe).
func TestCA564_CA532_TodaTeclaDeLaAyudaTieneEfecto(t *testing.T) {
	probadas := 0
	for _, l := range tablero.LugaresTodos {
		for _, a := range tablero.AtajosDe(l) {
			for _, k := range a.Teclas {
				if l == tablero.LugarVerNada && a.Barra != "esc volver" && len(a.Lugares) == len(tablero.LugaresVer) {
					continue
				}
				m := ttEstado(t, l)
				if string(k) == fmt.Sprint(int(m.Foco())) && len(a.Lugares) == len(tablero.LugaresPaneles) {
					continue
				}
				ttPrepara(m, l, k)
				antes := ttFoto(m)
				os := ttApretar(m, k)
				if len(os) == 0 && ttFoto(m) == antes {
					t.Fatalf("CA-564: en %q la tecla %q (%s) esta en la ayuda y no tuvo efecto:\n%s", l, k, a.Hace, antes)
				}
				probadas++
			}
		}
	}
	if probadas < 150 {
		t.Fatalf("CA-564: se probaron %d teclas, se esperaban todas las de la tabla", probadas)
	}
}

// ttEnLaTabla dice si k esta en alguna fila de l (una runa imprimible esta en
// la fila de TeclaTexto si l la tiene; "]" y "[" son el primer golpe de "]c"
// y "[c").
func ttEnLaTabla(l tablero.Lugar, k tablero.Tecla) bool {
	r, n := utf8.DecodeRuneInString(string(k))
	esTexto := n == len(k) && unicode.IsPrint(r)
	for _, a := range ttFilas(l) {
		for _, x := range a.Teclas {
			dosGolpes := (x == "]c" || x == "[c") && string(x[0]) == string(k)
			if x == k || (x == tablero.TeclaTexto && esTexto) || dosGolpes {
				return true
			}
		}
	}
	return false
}

var ttTeclasEspeciales = []tablero.Tecla{tablero.TeclaEnter, tablero.TeclaEsc, tablero.TeclaTab, tablero.TeclaShiftTab,
	tablero.TeclaArriba, tablero.TeclaAbajo, tablero.TeclaIzq, tablero.TeclaDer, tablero.TeclaBorrar, tablero.TeclaCtrlC,
	tablero.TeclaCtrlU, tablero.TeclaCtrlE, tablero.TeclaPgArriba, tablero.TeclaPgAbajo, tablero.TeclaEspacio}

// CA-564, propiedad negativa: toda tecla con efecto esta en la ayuda. En cada
// Lugar, una tecla que no esta en ninguna de sus filas no cambia nada ni da
// ordenes: las especiales todas, y runas imprimibles al azar. Re-expresa
// CA-532.
func TestCA564_CA532_NingunaTeclaFueraDeLaTablaTieneEfecto(t *testing.T) {
	sinEfecto := func(l tablero.Lugar, k tablero.Tecla) error {
		m := ttEstado(t, l)
		antes := ttFoto(m)
		os := m.Tecla(k, ttAhora)
		if len(os) != 0 || ttFoto(m) != antes {
			return fmt.Errorf("en %q la tecla %q no esta en la tabla y tuvo efecto (ordenes %#v):\n%s", l, k, os, ttPantalla(m))
		}
		return nil
	}
	probadas := 0
	for _, l := range tablero.LugaresTodos {
		for _, k := range ttTeclasEspeciales {
			if ttEnLaTabla(l, k) {
				continue
			}
			if err := sinEfecto(l, k); err != nil {
				t.Fatalf("CA-564: %v", err)
			}
			probadas++
		}
		for _, r := range "bcdfimoprvyzBCDEFHJKLMNOPQSTUVWXYZ56789!#$%&()*+,-.:;<=>@^_{|}~ñé" {
			k := tablero.Tecla(string(r))
			if ttEnLaTabla(l, k) {
				continue
			}
			if err := sinEfecto(l, k); err != nil {
				t.Fatalf("CA-564: %v", err)
			}
			probadas++
		}
	}
	if probadas < 200 {
		t.Fatalf("CA-564: se probaron solo %d teclas fuera de la tabla", probadas)
	}
	for _, l := range []tablero.Lugar{tablero.LugarTarjetas, tablero.LugarVerTarjeta, tablero.LugarAcciones, tablero.LugarAyuda, tablero.LugarFirma} {
		prop := func(r rune) bool {
			r = r % 0x3000
			if !unicode.IsPrint(r) || r == ' ' {
				return true
			}
			k := tablero.Tecla(string(r))
			if ttEnLaTabla(l, k) {
				return true
			}
			return sinEfecto(l, k) == nil
		}
		if err := quick.Check(prop, &quick.Config{MaxCount: 40, Rand: rand.New(rand.NewSource(int64(len(l))))}); err != nil {
			t.Fatalf("CA-564: en %q una runa fuera de la tabla tuvo efecto: %v", l, err)
		}
	}
}

// ---------------------------------------------------------------- lo que corre es lo que se muestra (v1)

// CA-564, CA-566, CA-525, CA-532 (hallazgo e8c2ef de la v1, guarda): para
// cada fila de Atajos con Accion y Verbo, lo que corre es lo que se muestra:
// los Args del Lanzar son las palabras de su Verbo sin "hoom" y con <slug>
// reemplazado (mas -m <pedido> cuando el modal tiene Pedido), "Se va a
// ejecutar" empieza con ese verbo, y el modal de acciones lo muestra.
func TestCA564_CA566_LoQueCorreEsLoQueSeMuestra(t *testing.T) {
	acciones := map[string]boardcmd.Action{
		boardcmd.ActPedirArquitecto: ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec"),
		boardcmd.ActPedirTestWriter: ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests"),
		boardcmd.ActPedirWriter:     ttAccionWriter(),
		boardcmd.ActPedirReviewer:   ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""),
		boardcmd.ActReanudar:        ttPagosCaidos().Actions[0],
		boardcmd.ActGuardar:         ttAccionGuardar(true, "", "internal/a.go"),
		boardcmd.ActAprobar:         ttAccionAprobar(),
		boardcmd.ActIntegrar:        ttAccionIntegrar(),
	}
	vistas := 0
	for _, fila := range tablero.Atajos {
		if fila.Accion == "" || fila.Verbo == "" {
			continue
		}
		vistas++
		k := fila.Teclas[0]
		ca := fmt.Sprintf("CA-564 (%q, %s)", k, fila.Accion)
		verbo := strings.ReplaceAll(fila.Verbo, "<slug>", "tarjeta")
		pal := strings.Fields(verbo)
		quiere := append([]string{}, pal[1:]...)
		acc, con := acciones[fila.Accion]
		card := ttTarjetaCon()
		if con {
			card = ttTarjetaCon(acc)
			m := ttModelo(ttAncho(), ttTablero(card))
			m.Tecla(tablero.TeclaEspacio, ttAhora)
			if !ttModal(t, ca, m, 200).ttTieneLinea(string(k) + " " + acc.Label + "  " + verbo) {
				t.Fatalf("%s: Acciones muestra la accion con su tecla y el verbo %q:\n%s", ca, verbo, ttPantalla(m))
			}
		} else if fila.Accion != tablero.AccionUsar {
			t.Fatalf("%s: la guarda no sabe armar la accion %q", ca, fila.Accion)
		}
		m := ttModelo(ttAncho(), ttTablero(card))
		ords := m.Tecla(k, ttAhora)
		switch m.Modal() {
		case tablero.ModalPedir:
			ttSinOrdenes(t, ca, "abrir Pedir", ords)
			mc := ttModal(t, ca, m, 200)
			if !strings.HasPrefix(mc.ttComando(), verbo) {
				t.Fatalf("%s: 'Se va a ejecutar' empieza con el verbo de la tabla %q: %q", ca, verbo, mc.ttComando())
			}
			if tiene := mc.ttCampo("Pedido") != ""; tiene != (acc.Role != "reviewer") {
				t.Fatalf("%s: el modal tiene Pedido salvo el del reviewer (tiene: %v)", ca, tiene)
			}
			if acc.Role != "reviewer" {
				quiere = append(quiere, "-m", strings.TrimSpace(acc.Pedido))
			}
			for m.Lugar() != tablero.LugarPedirBotones {
				m.Tecla(tablero.TeclaTab, ttAhora)
			}
			ords = m.Tecla(tablero.TeclaEnter, ttAhora)
		case tablero.ModalFirma:
			ttSinOrdenes(t, ca, "abrir Firma", ords)
			if got := ttModal(t, ca, m, 200).ttComando(); got != verbo {
				t.Fatalf("%s: 'Se va a ejecutar' es el verbo %q: %q", ca, verbo, got)
			}
			m.Tecla(tablero.TeclaDer, ttAhora)
			ords = m.Tecla(tablero.TeclaEnter, ttAhora)
		case tablero.ModalGuardar:
			ttSinOrdenes(t, ca, "abrir Guardar", ords)
			ords = m.Tecla(tablero.TeclaEnter, ttAhora)
		}
		l := ttUnaLanzar(t, ca, "la accion", ords)
		ttMismosArgs(t, ca, "lo que corre es lo que se muestra", l.Args, quiere)
		if l.Slug != "tarjeta" || l.Rol != acc.Role {
			t.Fatalf("%s: el Lanzar es de la tarjeta y del rol de la accion (%q): %#v", ca, acc.Role, l)
		}
		if r := m.Registro(); len(r) == 0 || !strings.HasPrefix(r[len(r)-1].Comando, verbo) {
			t.Fatalf("%s: el Registro lleva el mismo comando: %#v", ca, r)
		}
	}
	if vistas != 9 {
		t.Fatalf("CA-564: la tabla tiene las nueve teclas de accion con verbo del spec, tiene %d", vistas)
	}
	_ = time.Second
	_ = runcmd.StatusDone
}
