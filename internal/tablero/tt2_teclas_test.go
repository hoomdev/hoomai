// Las teclas (spec tablero-tu-siguiente-paso CA-718, que re-expresa CA-564
// de tablero-con-paneles, que re-expresaba CA-532 de la v1; CA-705 y CA-706;
// y CA-533): Decodificar (sin cambios de la v1), la tabla Atajos por Lugar,
// la linea de teclas y la ayuda desde la misma tabla, y las dos propiedades
// del contrato: toda tecla de la ayuda tiene efecto y ninguna tecla fuera de
// la tabla lo tiene. En la v3 no hay paneles numerados: la lista, el
// detalle, la vista Hallazgos y los modales (doc.go, "The keys").
package tablero_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"testing/quick"
	"unicode"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
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

// ---------------------------------------------------------------- la tabla (CA-718)

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

// CA-718, CA-564, CA-532: AtajosDe(l) son las filas de Atajos de l, en el
// orden de la tabla; cada Lugar tiene filas; un Lugar desconocido no tiene.
// La v3 tiene 18 Lugares (atajos.go): sin los de [1] y [3] de la v2 ni su
// [0], y con Seguir solo y el Registro.
func TestCA718_CA564_CA532_AtajosDe(t *testing.T) {
	for _, l := range tablero.LugaresTodos {
		quiere := ttFilas(l)
		if len(quiere) == 0 {
			t.Fatalf("CA-718: el Lugar %q tiene filas en Atajos", l)
		}
		if got := tablero.AtajosDe(l); !reflect.DeepEqual(got, quiere) {
			t.Fatalf("CA-718: AtajosDe(%q) son sus %d filas en orden, dio %d", l, len(quiere), len(got))
		}
	}
	if got := tablero.AtajosDe("no-es-un-lugar"); len(got) != 0 {
		t.Fatalf("CA-718: un Lugar desconocido no tiene teclas: %#v", got)
	}
	if len(tablero.LugaresTodos) != 18 {
		t.Fatalf("CA-718 (guarda): los Lugares son los 18 del contrato: %d", len(tablero.LugaresTodos))
	}
}

// CA-718, CA-564 (guarda de la tabla): en ningun Lugar una tecla esta en dos
// filas: el despacho no puede elegir entre dos.
func TestCA718_CA564_SinTeclasRepetidasPorLugar(t *testing.T) {
	for _, l := range tablero.LugaresTodos {
		visto := map[tablero.Tecla]string{}
		for _, a := range ttFilas(l) {
			for _, k := range a.Teclas {
				if otra, ok := visto[k]; ok {
					t.Fatalf("CA-718: en %q la tecla %q esta en dos filas: %q y %q", l, k, otra, a.Hace)
				}
				visto[k] = a.Hace
			}
		}
	}
}

// CA-718, CA-564, CA-525, CA-532: las teclas de accion ejecutan los verbos de
// la tabla del spec, con sus acciones; enter es el paso de la tarjeta
// elegida y n el de la tarjeta de la caja (CA-705, que re-expresa CA-566),
// los dos sin verbo propio; P es Seguir solo, con hoom piloto (CA-715).
func TestCA718_CA564_CA525_VerbosDeLaTabla(t *testing.T) {
	quiere := map[tablero.Tecla][2]string{
		tablero.TeclaEnter: {"", tablero.AccionPaso},
		"n":                {"", tablero.AccionSiguiente},
		"P":                {"hoom piloto <slug>", tablero.AccionSeguir},
		"a":                {"hoom pedir arquitecto <slug>", boardcmd.ActPedirArquitecto},
		"t":                {"hoom pedir test-writer <slug>", boardcmd.ActPedirTestWriter},
		"w":                {"hoom pedir writer <slug>", boardcmd.ActPedirWriter},
		"r":                {"hoom pedir reviewer <slug>", boardcmd.ActPedirReviewer},
		"R":                {"hoom reanudar <slug>", boardcmd.ActReanudar},
		"s":                {"hoom guardar <slug>", boardcmd.ActGuardar},
		"A":                {"hoom aprobar <slug> --si", boardcmd.ActAprobar},
		"I":                {"hoom integrar <slug> --si", boardcmd.ActIntegrar},
		"u":                {"hoom usar <slug>", tablero.AccionUsar},
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
			t.Fatalf("CA-718: la tecla %q esta en la tabla de la lista de tarjetas", k)
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

// ttLineaTeclasQuiere es la linea de teclas de l segun doc.go ("The two last
// lines"): los Barra de sus filas en orden, salvo la fila de enter con
// AccionPaso, cuya pista es sig ("enter <paso>", "enter abrir" o "enter
// cerrar" en HECHO; "" sin paso: sin pista); sin las de la tarjeta
// (DeTarjeta) si no hay tarjeta; y el ajuste al ancho: "? todas las teclas"
// pasa a "? todas" y despues se quitan las de antes de la ultima, de la de
// justo antes hacia atras, nunca la primera cuando es la de enter (CA-718).
// (El corte de la ultima cuando no entra sola no lo usa ningun caso.)
func ttLineaTeclasQuiere(l tablero.Lugar, conTarjeta bool, sig string, ancho int, utf bool) string {
	var pistas []string
	primeraEnter := false
	for _, a := range ttFilas(l) {
		b := a.Barra
		if a.Accion == tablero.AccionPaso {
			b = sig // la pista de enter depende de lo elegido
		}
		if b == "" {
			continue
		}
		if !conTarjeta && a.DeTarjeta {
			continue // las filas de la tarjeta (DeTarjeta: espacio, P y las de accion)
		}
		if !utf {
			b = ttAscii(b)
		}
		if len(pistas) == 0 && len(a.Teclas) > 0 && a.Teclas[0] == tablero.TeclaEnter {
			primeraEnter = true
		}
		pistas = append(pistas, b)
	}
	mide := func(ps []string) int { return ttCeldas(" " + strings.Join(ps, " · ")) }
	if mide(pistas) > ancho && len(pistas) > 0 && pistas[len(pistas)-1] == "? todas las teclas" {
		pistas[len(pistas)-1] = "? todas"
	}
	for mide(pistas) > ancho && len(pistas) > 1 {
		if len(pistas) == 2 && primeraEnter {
			break // la de enter no se quita nunca
		}
		pistas = append(pistas[:len(pistas)-2], pistas[len(pistas)-1])
	}
	return " " + strings.Join(pistas, " · ")
}

// CA-718: la ultima linea muestra, en palabras, solo las teclas del Lugar
// actual o del modal abierto, desde la tabla: primero la pista de enter para
// lo elegido, en negrita ("enter <etiqueta del paso>", "enter abrir" o
// "enter cerrar" en HECHO, "enter ver" en la vista Hallazgos, ninguna sin
// paso), y el ajuste al ancho de doc.go, que no quita nunca la de enter (el
// dibujo del contrato a 100 columnas y el del spec a 80x24). Re-expresa
// CA-564 (y CA-532, la barra 2 de la v1).
func TestCA718_CA564_CA532_LaLineaDeTeclas(t *testing.T) {
	const paso = "enter aprobar spec" // nuc-tenancy, en Tu aprobacion: su paso es aprobar
	for _, x := range []struct {
		ancho, alto int
		utf         bool
		quiere      string
	}{
		{100, 30, true, " enter aprobar spec · n tu siguiente paso · ↑↓ elegir · tab detalle · espacio acciones · ? todas"},
		{80, 24, false, " enter aprobar spec · n tu siguiente paso · ^v elegir · tab detalle · ? todas"},
		{60, 15, true, " enter aprobar spec · n tu siguiente paso · ? todas"},
	} {
		m := ttModeloDorado(ttConfig(x.ancho, x.alto, x.utf))
		if m.Lugar() != tablero.LugarTarjetas || m.Elegida() != "nuc-tenancy" {
			t.Fatalf("CA-718 (guarda): al abrir, la lista con foco en nuc-tenancy: %q %q", m.Lugar(), m.Elegida())
		}
		if got := ttLineaTeclas(m); got != x.quiere {
			t.Fatalf("CA-718: a %d columnas, la lista en nuc-tenancy (Tu aprobacion):\n %q\nse esperaba\n %q", x.ancho, got, x.quiere)
		}
		if q := ttLineaTeclasQuiere(tablero.LugarTarjetas, true, paso, x.ancho, x.utf); q != x.quiere {
			t.Fatalf("CA-718 (guarda de ttLineaTeclasQuiere): a %d columnas da %q", x.ancho, q)
		}
	}
	// la vista Hallazgos a 60 columnas: "enter ver" primero, y no se quita
	h := ttModeloDorado(ttConfig(60, 15, true))
	h.Tecla("H", ttAhora)
	const enHallazgos = " enter ver · ↑↓ elegir · e editor · esc volver · ? todas"
	if got := ttLineaTeclas(h); h.Lugar() != tablero.LugarHallazgos || got != enHallazgos ||
		ttLineaTeclasQuiere(tablero.LugarHallazgos, false, "", 60, true) != enHallazgos {
		t.Fatalf("CA-718: a 60 columnas, la vista Hallazgos (%q):\n %q\nse esperaba\n %q", h.Lugar(), got, enHallazgos)
	}
	// con color, la pista de enter va en negrita (1) y la que le sigue sin estilo
	for _, x := range []struct {
		teclas           []tablero.Tecla
		pista, siguiente string
	}{
		{nil, paso, "n tu siguiente paso"},
		{[]tablero.Tecla{"H"}, "enter ver", "↑↓ elegir"},
	} {
		cfg := ttConfig(200, 50, true)
		cfg.Color = true
		m := ttModeloDorado(cfg)
		ttTeclas(m, x.teclas...)
		p := m.Pantalla()
		crudo := p[len(p)-1]
		if e := ttEstiloDe(crudo, x.pista); e != "\x1b[1m" {
			t.Fatalf("CA-718: con color, la pista %q va en negrita (1): %q en %q", x.pista, e, crudo)
		}
		if e := ttEstiloDe(crudo, x.siguiente); e != "" {
			t.Fatalf("CA-718: la pista %q no lleva estilo: %q en %q", x.siguiente, e, crudo)
		}
	}
	// cada lugar, a 200 columnas (todo entra)
	teclas := func(ks ...tablero.Tecla) func(*tablero.Modelo) {
		return func(m *tablero.Modelo) { ttTeclas(m, ks...) }
	}
	casos := []struct {
		nombre string
		lugar  tablero.Lugar
		ir     func(*tablero.Modelo)
		tarj   bool
		sig    string
	}{
		{"la lista en nuc-tenancy", tablero.LugarTarjetas, teclas(), true, paso},
		{"la lista en precios (sin paso)", tablero.LugarTarjetas, func(m *tablero.Modelo) { tv3bIrA(t, m, "precios") }, true, ""},
		{"la linea de HECHO cerrada", tablero.LugarTarjetas, func(m *tablero.Modelo) { tv3bIrAHecho(t, m) }, false, "enter abrir"},
		{"la linea de HECHO abierta", tablero.LugarTarjetas, func(m *tablero.Modelo) { tv3bIrAHecho(t, m); m.Tecla(tablero.TeclaEnter, ttAhora) }, false, "enter cerrar"},
		{"la vista Hallazgos", tablero.LugarHallazgos, teclas("H"), false, ""},
		{"el detalle de nuc-tenancy", tablero.LugarVerTarjeta, teclas(tablero.TeclaTab), true, paso},
		{"el detalle del proyecto", tablero.LugarVerProyecto, teclas("v"), false, ""},
		{"el detalle de un hallazgo", tablero.LugarVerHallazgo, teclas("H", tablero.TeclaTab), false, ""},
		{"el detalle sin nada (HECHO)", tablero.LugarVerNada, func(m *tablero.Modelo) { tv3bIrAHecho(t, m); m.Tecla(tablero.TeclaTab, ttAhora) }, false, ""},
		{"la busqueda", tablero.LugarBusqueda, teclas("/"), false, ""},
		{"Acciones", tablero.LugarAcciones, teclas(tablero.TeclaEspacio), true, ""},
		{"Firma", tablero.LugarFirma, teclas("A"), false, ""},
		{"la ayuda", tablero.LugarAyuda, teclas("?"), false, ""},
		{"el Registro", tablero.LugarRegistro, teclas("L"), false, ""},
	}
	for _, c := range casos {
		m := ttModeloDorado(ttConfig(200, 50, false))
		c.ir(m)
		if m.Lugar() != c.lugar {
			t.Fatalf("CA-718 (guarda): %s es el Lugar %q, se llego a %q", c.nombre, c.lugar, m.Lugar())
		}
		if c.nombre == "la linea de HECHO abierta" && !strings.HasPrefix(tv3bElegida(t, "CA-718", m), "HECHO ") {
			t.Fatalf("CA-718 (guarda): enter en la linea de HECHO la abre y la deja elegida: %q", tv3bElegida(t, "CA-718", m))
		}
		if got, q := ttLineaTeclas(m), ttLineaTeclasQuiere(c.lugar, c.tarj, c.sig, 200, false); got != q {
			t.Fatalf("CA-718: la linea de teclas de %s (%q) es\n %q\nse esperaba\n %q", c.nombre, c.lugar, got, q)
		}
	}
	// CA-718, hallazgo 403795: tambien en el detalle de una tarjeta que corre
	// (la v2: [0] de lo que corre; CA-711), sin paso, en Guardar y en Seguir solo
	v := ttModeloDorado(ttConfig(200, 50, false))
	tv3bIrA(t, v, "infra-api")
	tv3bAlDetalle(t, v)
	if v.Lugar() != tablero.LugarVerTarjeta || v.Grupo("infra-api") != tablero.GrupoCorriendo {
		t.Fatalf("CA-718 (guarda): tab en infra-api lleva al detalle de una tarjeta de CORRIENDO: %q %q", v.Lugar(), v.Grupo("infra-api"))
	}
	if got, q := ttLineaTeclas(v), ttLineaTeclasQuiere(tablero.LugarVerTarjeta, true, "", 200, false); got != q {
		t.Fatalf("CA-718 (hallazgo 403795): la linea de teclas del detalle de infra-api (corre: sin paso) es\n %q\nse esperaba\n %q", got, q)
	}
	g := ttModelo(ttConfig(200, 50, false), ttTablero(ttTarjetaCon(ttAccionGuardar(true, "", "a.go"))))
	g.Tecla("s", ttAhora)
	if g.Lugar() != tablero.LugarGuardar {
		t.Fatalf("CA-718 (guarda): s lleva a LugarGuardar: %q", g.Lugar())
	}
	if got, q := ttLineaTeclas(g), ttLineaTeclasQuiere(tablero.LugarGuardar, true, "", 200, false); got != q {
		t.Fatalf("CA-718 (hallazgo 403795): la linea de teclas de %q es\n %q\nse esperaba\n %q", tablero.LugarGuardar, got, q)
	}
	sola := ttTarjetaCon(ttAccionWriter())
	sola.Item.Auto = item.AutoHastaHumano
	s := ttModelo(ttConfig(200, 50, false), ttTablero(sola))
	s.Tecla("P", ttAhora)
	if s.Lugar() != tablero.LugarSeguir {
		t.Fatalf("CA-718 (guarda): P en una tarjeta con el piloto encendido, en Writer, lleva a LugarSeguir: %q\n%s", s.Lugar(), ttPantalla(s))
	}
	if got, q := ttLineaTeclas(s), ttLineaTeclasQuiere(tablero.LugarSeguir, true, "", 200, false); got != q {
		t.Fatalf("CA-718: la linea de teclas de %q es\n %q\nse esperaba\n %q", tablero.LugarSeguir, got, q)
	}
	// el modal Pedir: el dibujo del spec
	p := ttPedir(t, "w", ttAccionWriter())
	p.Tecla(tablero.TeclaTab, ttAhora)
	if got := ttLineaTeclas(p); got != " tab campo · <> cambiar · enter ver opciones · ctrl-e editor · esc cerrar" {
		t.Fatalf("CA-718: la linea de teclas de un selector (el dibujo del spec): %q", got)
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
			t.Fatalf("CA-718 (guarda): no llegue a %q: %q", l, m.Lugar())
		}
		if got, q := ttLineaTeclas(m), ttLineaTeclasQuiere(l, false, "", 200, false); got != q {
			t.Fatalf("CA-718: la linea de teclas de %q es\n %q\nse esperaba\n %q", l, got, q)
		}
	}
}

// ---------------------------------------------------------------- la ayuda

// tv3bTecGrupos son los titulos de los grupos de la ayuda, en orden: sus
// lineas de texto que no empiezan con un espacio (las filas empiezan con dos,
// las continuaciones con su sangria).
func tv3bTecGrupos(textos []string) []string {
	var out []string
	for _, l := range textos {
		if l != "" && !strings.HasPrefix(l, " ") {
			out = append(out, l)
		}
	}
	return out
}

// tv3bTecGrupoDe es el titulo del grupo de la ayuda donde esta la primera
// linea que empieza con prefijo, o "" si ninguna.
func tv3bTecGrupoDe(textos []string, prefijo string) string {
	grupo := ""
	for _, l := range textos {
		if l != "" && !strings.HasPrefix(l, " ") {
			grupo = l
			continue
		}
		if strings.HasPrefix(l, prefijo) {
			return grupo
		}
	}
	return ""
}

// CA-718: ? abre el modal de la ayuda ("Todas las teclas") con todas las
// teclas agrupadas en "Tarjetas", "Detalle", "Hallazgos", "Modales" y
// "Mouse", primero el grupo de donde esta el foco (despues los otros dos de
// los tres primeros, en ese orden), una linea vacia entre grupos; cada fila
// "  " + Muestra rellena a 18 + Hace (+ su verbo), una vez, en el primer
// grupo cuyos Lugares la tienen (decision de doc.go); el grupo Mouse con las
// lineas de la v3 (o, sin mouse, "apagado: HOOM_MOUSE=0"); 1, 2, 3, 4 y 0 no
// estan (CA-706); esc y ? la cierran. Re-expresa CA-564 (y CA-532).
func TestCA718_CA564_CA532_LaAyuda(t *testing.T) {
	grupos := []string{"Tarjetas", "Detalle", "Hallazgos", "Modales", "Mouse"}
	for _, mouse := range []bool{true, false} {
		cfg := ttConfig(200, 300, false)
		cfg.Mouse = mouse
		m := ttModelo(cfg, ttTableroDorado())
		ttSinOrdenes(t, "CA-718", "?", m.Tecla("?", ttAhora))
		if m.Modal() != tablero.ModalAyuda || m.Lugar() != tablero.LugarAyuda {
			t.Fatalf("CA-718: ? abre la ayuda: %q %q", m.Modal(), m.Lugar())
		}
		mc := ttModal(t, "CA-718", m, 200)
		if !strings.HasPrefix(mc.Arriba, "# Todas las teclas ") {
			t.Fatalf("CA-718: el titulo de la ayuda es 'Todas las teclas': %q", mc.Arriba)
		}
		if got := tv3bTecGrupos(mc.Textos); strings.Join(got, "|") != strings.Join(grupos, "|") {
			t.Fatalf("CA-718: abierta desde la lista, los grupos son %q: %q\n%s", grupos, got, strings.Join(mc.Textos, "\n"))
		}
		if len(mc.Textos) == 0 || mc.Textos[0] != grupos[0] {
			t.Fatalf("CA-718: la ayuda empieza con el grupo de donde esta el foco: %q", mc.Textos[:min(3, len(mc.Textos))])
		}
		for i, l := range mc.Textos {
			if i > 0 && l != "" && !strings.HasPrefix(l, " ") && mc.Textos[i-1] != "" {
				t.Fatalf("CA-718: una linea vacia entre grupos (antes de %q)", l)
			}
		}
		todo := "\n" + strings.Join(mc.Textos, "\n") + "\n"
		// R3: una linea que no entra en las Wm-6 = 66 celdas del texto se parte
		// por palabras, cada continuacion desde la columna de Hace (20); las de
		// Mouse, desde sus dos espacios. Nada se pierde.
		for _, a := range tablero.Atajos {
			base := "  " + fmt.Sprintf("%-18s", ttAscii(a.Muestra)) + a.Hace
			variantes := []string{base}
			if a.Verbo != "" {
				variantes = []string{base + "  " + a.Verbo, base + "  " + strings.ReplaceAll(a.Verbo, "<slug>", "nuc-tenancy")}
			}
			ok := false
			for _, v := range variantes {
				ok = ok || strings.Contains(todo, "\n"+strings.Join(ttPartir(v, 66, 20), "\n")+"\n")
			}
			if !ok {
				t.Fatalf("CA-718: toda fila de la tabla esta en la ayuda, entera (partida por palabras si no entra):\n%q\n%s",
					ttPartir(variantes[0], 66, 20), todo)
			}
		}
		// cada fila en el primer grupo cuyos Lugares la tienen (abierta desde
		// la lista los grupos van en el orden de la tabla del contrato)
		for _, x := range []struct{ muestra, hace, grupo string }{
			{"texto", "filtrar por slug o titulo", "Tarjetas"},               // LugarBusqueda
			{"P", "seguir solo  hoom piloto ", "Tarjetas"},                   // LugaresConTarjeta
			{"]c [c", "criterio sin prueba siguiente o anterior", "Detalle"}, // LugarVerTarjeta
			{"enter", "foco al detalle del hallazgo", "Hallazgos"},           // LugarHallazgos
			{"esc", "volver a las tarjetas", "Hallazgos"},                    // LugarHallazgos
			{"esc L", "cerrar el registro", "Modales"},                       // LugarRegistro
		} {
			linea := "  " + fmt.Sprintf("%-18s", x.muestra) + x.hace
			if g := tv3bTecGrupoDe(mc.Textos, linea); g != x.grupo {
				t.Fatalf("CA-718: la fila %q esta en el grupo %q, esta en %q:\n%s", linea, x.grupo, g, todo)
			}
		}
		// CA-706: 1, 2, 3, 4 y 0 no estan en la tabla ni en la ayuda
		for _, a := range tablero.Atajos {
			for _, k := range a.Teclas {
				if len(k) == 1 && strings.Contains("01234", string(k)) {
					t.Fatalf("CA-706: la tecla %q no esta en la tabla (fila %q)", k, a.Hace)
				}
			}
		}
		for _, d := range "01234" {
			if strings.Contains(todo, "\n  "+string(d)+" ") {
				t.Fatalf("CA-706: la ayuda no tiene la tecla %q:\n%s", string(d), todo)
			}
		}
		if mouse {
			// las lineas de Mouse se parten como las filas (doc.go, "Ayuda"):
			// por sus palabras, cada continuacion desde la columna de Hace, 20;
			// la de seleccionar texto, sin Muestra, usa en su primera linea todo
			// el ancho despues de sus "  " (ttPartir arma la primera linea por
			// palabras sobre el ancho entero, asi que su corte a 20 no la cambia)
			for _, s := range []string{"  clic              foco a la lista o al detalle; en una tarjeta, la elige",
				"  clic en la caja   elige la tarjeta de tu siguiente paso",
				"  rueda             mueve la seleccion de la lista; en el detalle y en la ayuda, desplaza",
				"  clic en un modal  enfoca un campo, elige una opcion o un boton",
				"  para seleccionar texto con el mouse, manten Option (macOS) o Shift"} {
				if q := strings.Join(ttPartir(s, 66, 20), "\n"); !strings.Contains(todo, "\n"+q+"\n") {
					t.Fatalf("CA-718, CA-575: el grupo Mouse dice %q:\n%s", q, todo)
				}
				if g := tv3bTecGrupoDe(mc.Textos, ttPartir(s, 66, 20)[0]); g != "Mouse" {
					t.Fatalf("CA-718: la linea %q esta en el grupo Mouse, esta en %q", s, g)
				}
			}
			if !strings.Contains(todo, "\n  para seleccionar texto con el mouse, manten Option (macOS) o\n"+strings.Repeat(" ", 20)+"Shift\n") {
				t.Fatalf("CA-718 (guarda de ttPartir): la linea de 68 celdas se parte antes de Shift, que sigue en la columna 20:\n%s", todo)
			}
		} else if !strings.Contains(todo, "\nMouse\n  apagado: HOOM_MOUSE=0\n") {
			t.Fatalf("CA-718, CA-575: sin mouse, el grupo Mouse dice 'apagado: HOOM_MOUSE=0':\n%s", todo)
		}
		for _, cerrar := range []tablero.Tecla{tablero.TeclaEsc, "?"} {
			ttSinOrdenes(t, "CA-718", string(cerrar), m.Tecla(cerrar, ttAhora))
			if m.Modal() != tablero.SinModal || m.Lugar() != tablero.LugarTarjetas {
				t.Fatalf("CA-718: %q cierra la ayuda y vuelve a la lista: %q %q", cerrar, m.Modal(), m.Lugar())
			}
			m.Tecla("?", ttAhora)
		}
	}
	// el primer grupo es el de donde esta el foco; despues los otros dos de
	// los tres primeros, en su orden, y Modales y Mouse
	enDetalle := []string{"Detalle", "Tarjetas", "Hallazgos", "Modales", "Mouse"}
	for _, c := range []struct {
		nombre string
		ir     func(*tablero.Modelo)
		orden  []string
	}{
		{"el detalle de una tarjeta", func(m *tablero.Modelo) { m.Tecla(tablero.TeclaTab, ttAhora) }, enDetalle},
		{"el detalle del proyecto", func(m *tablero.Modelo) { m.Tecla("v", ttAhora) }, enDetalle},
		{"el detalle sin nada (HECHO)", func(m *tablero.Modelo) { tv3bIrAHecho(t, m); m.Tecla(tablero.TeclaTab, ttAhora) }, enDetalle},
		{"el detalle de un hallazgo", func(m *tablero.Modelo) { ttTeclas(m, "H", tablero.TeclaTab) }, enDetalle},
		{"la lista de la vista Hallazgos", func(m *tablero.Modelo) { m.Tecla("H", ttAhora) },
			[]string{"Hallazgos", "Tarjetas", "Detalle", "Modales", "Mouse"}},
	} {
		m := ttModeloDorado(ttConfig(200, 300, false))
		c.ir(m)
		desde := m.Lugar()
		m.Tecla("?", ttAhora)
		if m.Lugar() != tablero.LugarAyuda {
			t.Fatalf("CA-718 (guarda): ? desde %s (%q) abre la ayuda: %q", c.nombre, desde, m.Lugar())
		}
		mc := ttModal(t, "CA-718", m, 200)
		if got := tv3bTecGrupos(mc.Textos); strings.Join(got, "|") != strings.Join(c.orden, "|") || mc.Textos[0] != c.orden[0] {
			t.Fatalf("CA-718: abierta desde %s (%q), los grupos son %q: %q", c.nombre, desde, c.orden, got)
		}
	}
}

// CA-718 (re-expresa CA-564): la ayuda que no entra ocupa B lineas, su borde
// de abajo dice "<n> de <m>" (n la ultima linea a la vista, m todas) y se
// desplaza con ↓ j ↑ k (una linea) y pgdn pgup (su alto interior), entre su
// principio y su final.
func TestCA718_CA564_LaAyudaSeDesplaza(t *testing.T) {
	m := ttModeloDorado(ttConfig(80, 24, false))
	m.Tecla("?", ttAhora)
	mc := ttModal(t, "CA-718", m, 80)
	if mc.Alto != 22 || mc.Fila != 0 {
		t.Fatalf("CA-718: la ayuda que no entra ocupa las B=22 lineas: fila %d, alto %d", mc.Fila, mc.Alto)
	}
	if !strings.Contains(mc.Abajo, "=20 de ") {
		t.Fatalf("CA-718: el borde de abajo dice '20 de <m>' (20 lineas interiores): %q", mc.Abajo)
	}
	primera := mc.Textos[0]
	m.Tecla("j", ttAhora)
	if mc := ttModal(t, "CA-718", m, 80); mc.Textos[0] == primera || !strings.Contains(mc.Abajo, "=21 de ") {
		t.Fatalf("CA-718: j baja una linea: %q %q", mc.Textos[0], mc.Abajo)
	}
	m.Tecla("k", ttAhora)
	m.Tecla("k", ttAhora)
	if mc := ttModal(t, "CA-718", m, 80); mc.Textos[0] != primera {
		t.Fatalf("CA-718: k en la primera no sube mas: %q", mc.Textos[0])
	}
	m.Tecla(tablero.TeclaPgAbajo, ttAhora)
	if mc := ttModal(t, "CA-718", m, 80); !strings.Contains(mc.Abajo, "=40 de ") {
		t.Fatalf("CA-718: pgdn baja su alto interior (20): %q", mc.Abajo)
	}
	m.Tecla(tablero.TeclaPgArriba, ttAhora)
	if mc := ttModal(t, "CA-718", m, 80); mc.Textos[0] != primera {
		t.Fatalf("CA-718: pgup vuelve: %q", mc.Textos[0])
	}
	for i := 0; i < 500; i++ {
		m.Tecla(tablero.TeclaPgAbajo, ttAhora)
	}
	mc = ttModal(t, "CA-718", m, 80)
	g := regexp.MustCompile(`([0-9]+) de ([0-9]+)`).FindStringSubmatch(mc.Abajo)
	if g == nil || g[1] != g[2] {
		t.Fatalf("CA-718: al final del todo el borde dice '<m> de <m>': %q", mc.Abajo)
	}
	m.Tecla("j", ttAhora)
	if otra := ttModal(t, "CA-718", m, 80); otra.Abajo != mc.Abajo {
		t.Fatalf("CA-718: en el final j no baja mas: %q", otra.Abajo)
	}
}

// ttPartir parte una linea de la ayuda que no entra en ancho celdas por sus
// palabras (R3): la primera conserva sus sangria celdas de comienzo (los dos
// espacios y la Muestra rellena) y tantas palabras como entren en el ancho
// entero; cada continuacion empieza con sangria espacios (la ayuda: 20, la
// columna de Hace, tambien en el grupo Mouse). Los espacios entre dos
// palabras de una misma linea quedan como estaban.
func ttPartir(texto string, ancho, sangria int) []string {
	if ttCeldas(texto) <= ancho {
		return []string{texto}
	}
	lead, rest := texto[:sangria], texto[sangria:]
	type tok struct{ sep, w string }
	var toks []tok
	sep, w := "", ""
	for _, r := range rest {
		if r == ' ' {
			if w != "" {
				toks = append(toks, tok{sep, w})
				sep, w = "", ""
			}
			sep += " "
			continue
		}
		w += string(r)
	}
	if w != "" {
		toks = append(toks, tok{sep, w})
	}
	var out []string
	linea, nueva := lead, false
	for i, k := range toks {
		cand := linea + k.sep + k.w
		if nueva {
			cand = linea + k.w
		}
		if i == 0 || ttCeldas(cand) <= ancho {
			linea, nueva = cand, false
			continue
		}
		out = append(out, linea)
		linea, nueva = strings.Repeat(" ", sangria)+k.w, false
	}
	return append(out, linea)
}

// ---------------------------------------------------------------- los estados para las propiedades

// ttEstado es un modelo en el Lugar l, en un estado donde cada tecla de la
// tabla puede tener efecto: listas con varias filas, detalles mas largos que
// su vista, una ayuda y un Registro que se desplazan. La tarjeta elegida es
// infra-api (DETENIDA por su rojo, con el detalle largo): es tambien la de la
// caja, porque TE TOCA esta vacio (doc.go, "The box TU SIGUIENTE PASO").
// Llega a cada Lugar como dice doc.go ("The keys"): tab al detalle, H a la
// vista Hallazgos, v al proyecto, la linea de HECHO y tab al detalle sin
// nada, L al Registro, P a Seguir solo (en una tarjeta con el piloto
// encendido en Writer).
func ttEstado(t *testing.T, l tablero.Lugar) *tablero.Modelo {
	t.Helper()
	c, d := ttTarjetaLarga()
	c.Actions = append(c.Actions, ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""))
	d.Detalle.Card = c
	corre := ttCorre("corre", "writer", "env-c", "run-c")
	otra := ttCorre("otra", "reviewer", "env-o", "run-o")
	tarjetas := []boardcmd.Card{c, corre, otra, ttBacklog("uno", "Uno"), ttViejo(1)}
	sola := ttTarjetaCon(ttAccionWriter()) // Seguir solo esta disponible para ella (CA-715)
	sola.Item.Auto = item.AutoHastaHumano
	if l == tablero.LugarSeguir {
		tarjetas = append(tarjetas, sola)
	}
	m := ttModelo(ttConfig(80, 24, false), ttTablero(tarjetas...))
	p := ttProyectoVerde()
	for i := 0; i < 30; i++ {
		p.Problemas = append(p.Problemas, boardcmd.Problem{ID: boardcmd.ProbSinSpec, Slug: fmt.Sprintf("p%02d", i), What: "sin spec", Action: "hoom pedir arquitecto x"})
	}
	m.LeerProyecto(p)
	var desc strings.Builder
	for i := 0; i < 30; i++ {
		fmt.Fprintf(&desc, "linea %02d de la descripcion\n", i) // distintas: desplazar cambia la pantalla
	}
	h := ttHallazgo("h1", finding.StatusOpen, "high", "a.go:3", desc.String())
	m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h, ttHallazgo("h2", finding.StatusOpen, "low", "b.go", "d")}})
	tv3bIrA(t, m, c.Slug)
	m.LeerDetalle(d)
	ir := func(ks ...tablero.Tecla) { ttTeclas(m, ks...) }
	switch l {
	case tablero.LugarTarjetas:
	case tablero.LugarHallazgos:
		ir("H")
	case tablero.LugarVerProyecto:
		ir("v")
	case tablero.LugarVerTarjeta:
		ir(tablero.TeclaTab)
	case tablero.LugarVerHallazgo:
		ir("H", tablero.TeclaTab)
	case tablero.LugarVerNada:
		tv3bIrAHecho(t, m)
		ir(tablero.TeclaTab)
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
		tarjetas[0] = c
		m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(tarjetas...)})
		ir("s")
	case tablero.LugarSeguir:
		tv3bIrA(t, m, sola.Slug)
		ir("P")
	case tablero.LugarAcciones:
		ir(tablero.TeclaEspacio)
	case tablero.LugarAyuda:
		ir("?")
	case tablero.LugarRegistro:
		// doce u (Lanzar sin modal, cada una con su asiento: CA-560, CA-714)
		// dejan 24 lineas: mas que las 20 interiores del modal a 80x24
		for i := 0; i < 12; i++ {
			ir("u")
		}
		if len(m.Registro()) < 12 {
			t.Fatalf("CA-714 (guarda): doce u dejan doce asientos en el Registro: %d", len(m.Registro()))
		}
		ir("L")
	}
	if m.Lugar() != l {
		t.Fatalf("CA-718 (guarda): no pude llegar a %q (estoy en %q)\n%s", l, m.Lugar(), ttPantalla(m))
	}
	return m
}

// ttPrepara deja m donde la tecla k tiene que tener efecto: lejos del
// principio para subir, lejos del final del Registro (que abre en su final)
// para bajar, con texto para borrar, con el cursor atras para ir adelante.
func ttPrepara(m *tablero.Modelo, l tablero.Lugar, k tablero.Tecla) {
	texto := l == tablero.LugarPedirTexto || l == tablero.LugarBusqueda
	switch {
	case l == tablero.LugarRegistro && (k == "j" || k == tablero.TeclaAbajo || k == tablero.TeclaPgAbajo):
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

// ttFoto es todo lo que una tecla puede cambiar (la vista de la lista
// incluida: H).
func ttFoto(m *tablero.Modelo) string {
	return fmt.Sprintf("%s|%s|%d|%s|%s|%d\n%s", m.Lugar(), m.Modal(), int(m.Foco()), m.Vista(), m.Elegida(), len(m.Registro()), ttPantalla(m))
}

// CA-718, propiedad: toda tecla de la ayuda tiene efecto. En cada Lugar, cada
// tecla de cada una de sus filas (desde un estado donde puede tenerlo)
// cambia algo: la pantalla, el Lugar, el modal, el foco, la vista, el
// Registro, o da una orden. Re-expresa CA-564 (y CA-532). Queda afuera v con
// el proyecto ya en el detalle y el foco ahi (LugarVerProyecto), como en la
// v2 la tecla del panel que ya tenia el foco. (En el detalle sin nada que
// mostrar las teclas de desplazar no estan en sus filas: LugaresVerAlgo.)
func TestCA718_CA564_CA532_TodaTeclaDeLaAyudaTieneEfecto(t *testing.T) {
	probadas := 0
	for _, l := range tablero.LugaresTodos {
		for _, a := range tablero.AtajosDe(l) {
			for _, k := range a.Teclas {
				if l == tablero.LugarVerProyecto && k == "v" {
					continue
				}
				m := ttEstado(t, l)
				ttPrepara(m, l, k)
				antes := ttFoto(m)
				os := ttApretar(m, k)
				if len(os) == 0 && ttFoto(m) == antes {
					t.Fatalf("CA-718: en %q la tecla %q (%s) esta en la ayuda y no tuvo efecto:\n%s", l, k, a.Hace, antes)
				}
				probadas++
			}
		}
	}
	if probadas < 150 {
		t.Fatalf("CA-718: se probaron %d teclas, se esperaban todas las de la tabla", probadas)
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

// ttRunasV1 son las runas de la propiedad negativa de la v1: todas las letras,
// los digitos (en la v3 1, 2, 3, 4 y 0 no hacen nada en ningun Lugar:
// CA-706), "[" y "]" (el primer golpe de ]c y [c, que solo tiene el detalle
// de una tarjeta) y la puntuacion (CA-564, hallazgo d95bb0).
const ttRunasV1 = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!#$%&()*+,-.:;<=>@[]^_{|}~ñé"

// CA-718, CA-706, propiedad negativa: toda tecla con efecto esta en la
// ayuda. En cada Lugar, una tecla que no esta en ninguna de sus filas no
// cambia nada ni da ordenes: las especiales todas, las runas de la v1 y
// runas imprimibles al azar; 1, 2, 3, 4 y 0 en los Lugares de los paneles
// (la v2 movia el foco con ellas: CA-706). Re-expresa CA-564 (y CA-532).
// (CA-564, hallazgo d95bb0: con la lista de la v1, "u" en Firma, Guardar o
// Ayuda, "]" o "[" en el detalle del proyecto o de un hallazgo, y los
// digitos dentro de un modal se prueban.)
func TestCA718_CA706_CA564_CA532_NingunaTeclaFueraDeLaTablaTieneEfecto(t *testing.T) {
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
				t.Fatalf("CA-718: %v", err)
			}
			probadas++
		}
		for _, r := range ttRunasV1 { // CA-564, hallazgo d95bb0
			k := tablero.Tecla(string(r))
			if ttEnLaTabla(l, k) {
				continue
			}
			if err := sinEfecto(l, k); err != nil {
				t.Fatalf("CA-718: %v", err)
			}
			probadas++
		}
	}
	if probadas < 400 {
		t.Fatalf("CA-718: se probaron solo %d teclas fuera de la tabla", probadas)
	}
	// CA-706: 1, 2, 3, 4 y 0 no estan en la tabla de ningun Lugar de los
	// paneles y no hacen nada ahi
	for _, l := range tablero.LugaresPaneles {
		for _, k := range []tablero.Tecla{"1", "2", "3", "4", "0"} {
			if ttEnLaTabla(l, k) {
				t.Fatalf("CA-706: %q no esta en la tabla de %q", k, l)
			}
			if err := sinEfecto(l, k); err != nil {
				t.Fatalf("CA-706: %v", err)
			}
		}
	}
	// CA-564, hallazgo d95bb0: "]" y "[" tampoco desplazan el detalle del
	// proyecto ni el de un hallazgo desde el medio de su documento (donde un
	// desplazamiento se veria para los dos lados).
	for _, l := range []tablero.Lugar{tablero.LugarVerProyecto, tablero.LugarVerHallazgo} {
		for _, k := range []tablero.Tecla{"]", "["} {
			if ttEnLaTabla(l, k) {
				t.Fatalf("CA-718: %q no tiene ]c ni [c (son del detalle de una tarjeta)", l)
			}
			m := ttEstado(t, l)
			ttTeclas(m, "g", "j", "j", "j")
			antes := ttFoto(m)
			if os := ttTeclas(m, k, k); len(os) != 0 || ttFoto(m) != antes {
				t.Fatalf("CA-718: en %q %q no esta en la tabla y desplazo el detalle (ordenes %#v):\n%s", l, k, os, ttPantalla(m))
			}
		}
	}
	// CA-564, hallazgo d95bb0: x con un modal abierto no cambia el modo
	// experto (que el modal tapa): cerrado el modal, la pantalla es la misma
	// que sin la x.
	for _, l := range tablero.LugaresTodos {
		if ttEnLaTabla(l, "x") {
			continue
		}
		cerrado := func(conX bool) string {
			m := ttEstado(t, l)
			if m.Modal() == tablero.SinModal {
				return ""
			}
			if conX {
				m.Tecla("x", ttAhora)
			}
			for i := 0; i < 5 && m.Modal() != tablero.SinModal; i++ {
				m.Tecla(tablero.TeclaEsc, ttAhora)
			}
			return ttPantalla(m)
		}
		if sin, con := cerrado(false), cerrado(true); sin != con {
			t.Fatalf("CA-718: en %q x no esta en la tabla y cambio el modo experto; sin x:\n%s\ncon x:\n%s", l, sin, con)
		}
	}
	for _, l := range []tablero.Lugar{tablero.LugarTarjetas, tablero.LugarHallazgos, tablero.LugarVerTarjeta, tablero.LugarAcciones,
		tablero.LugarAyuda, tablero.LugarFirma, tablero.LugarSeguir, tablero.LugarRegistro} {
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
			t.Fatalf("CA-718: en %q una runa fuera de la tabla tuvo efecto: %v", l, err)
		}
	}
}

// ---------------------------------------------------------------- lo que corre es lo que se muestra (v1)

// CA-718, CA-705, CA-564, CA-566, CA-525, CA-532 (hallazgo e8c2ef de la v1,
// guarda): para cada fila de Atajos con Accion y Verbo, lo que corre es lo
// que se muestra: los Args del Lanzar son las palabras de su Verbo sin
// "hoom" y con <slug> reemplazado (mas -m <pedido> cuando el modal tiene
// Pedido), "Se va a ejecutar" empieza con ese verbo, y el modal de acciones
// lo muestra. P (Seguir solo, CA-715 y CA-716) no es una accion de la
// tarjeta: Acciones no la lista, y su modal lanza hoom piloto <slug> con el
// rol piloto.
// Re-expresado por CA-753 y CA-755 (ciclo-lo-decide-hoom): la tabla suma la
// fila f (pedir al refutador, hoom pedir refutador <slug>): son once teclas
// de accion con verbo. El modal del refutador no lleva Pedido y su Lanzar no
// lleva -m aunque la accion traiga el pedido que armo hoom.
func TestCA718_CA705_CA564_CA566_LoQueCorreEsLoQueSeMuestra(t *testing.T) {
	// el refutador trae el pedido que armo hoom (CA-754), que no se edita
	refutador := ttAccionRol(boardcmd.ActPedirRefutador, "refutador", "Pedir al refutador",
		boardcmd.PedidoRondaRefutador(1, "tarjeta", []string{"f1", "f2"}))
	refutador.Hallazgos = []string{"f1", "f2"}
	acciones := map[string]boardcmd.Action{
		boardcmd.ActPedirArquitecto: ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec"),
		boardcmd.ActPedirTestWriter: ttAccionRol(boardcmd.ActPedirTestWriter, "test-writer", "Pedir al test-writer", "Escribi los tests"),
		boardcmd.ActPedirWriter:     ttAccionWriter(),
		boardcmd.ActPedirReviewer:   ttAccionRol(boardcmd.ActPedirReviewer, "reviewer", "Pedir al reviewer", ""),
		boardcmd.ActPedirRefutador:  refutador,
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
		ca := fmt.Sprintf("CA-718 (%q, %s)", k, fila.Accion)
		verbo := strings.ReplaceAll(fila.Verbo, "<slug>", "tarjeta")
		pal := strings.Fields(verbo)
		quiere := append([]string{}, pal[1:]...)
		acc, con := acciones[fila.Accion]
		rol := acc.Role
		card := ttTarjetaCon()
		switch {
		case fila.Accion == tablero.AccionSeguir:
			rol = "piloto"
			card = ttTarjetaCon(ttAccionWriter())
			card.Item.Auto = item.AutoHastaHumano // en Writer: Seguir solo esta disponible
			m := ttModelo(ttAncho(), ttTablero(card))
			m.Tecla(tablero.TeclaEspacio, ttAhora)
			if l, _, _ := ttModal(t, ca, m, 200).ttLineaQueEmpieza(string(k) + " "); l != "" {
				t.Fatalf("CA-715: Seguir solo no es una accion de la tarjeta y Acciones no la lista: %q", l)
			}
		case con:
			card = ttTarjetaCon(acc)
			m := ttModelo(ttAncho(), ttTablero(card))
			m.Tecla(tablero.TeclaEspacio, ttAhora)
			if !ttModal(t, ca, m, 200).ttTieneLinea(string(k) + " " + acc.Label + "  " + verbo) {
				t.Fatalf("%s: Acciones muestra la accion con su tecla y el verbo %q:\n%s", ca, verbo, ttPantalla(m))
			}
		case fila.Accion != tablero.AccionUsar:
			t.Fatalf("%s: la guarda no sabe armar la accion %q", ca, fila.Accion)
		}
		m := ttModelo(ttAncho(), ttTablero(card))
		ords := m.Tecla(k, ttAhora)
		if fila.Accion == tablero.AccionSeguir && m.Modal() != tablero.ModalSeguir {
			t.Fatalf("CA-716: P en una tarjeta con Seguir solo disponible abre su modal: %q\n%s", m.Modal(), ttPantalla(m))
		}
		switch m.Modal() {
		case tablero.ModalPedir:
			ttSinOrdenes(t, ca, "abrir Pedir", ords)
			mc := ttModal(t, ca, m, 200)
			if !strings.HasPrefix(mc.ttComando(), verbo) {
				t.Fatalf("%s: 'Se va a ejecutar' empieza con el verbo de la tabla %q: %q", ca, verbo, mc.ttComando())
			}
			sinPedido := acc.Role == "reviewer" || acc.Role == "refutador"
			if tiene := mc.ttCampo("Pedido") != ""; tiene == sinPedido {
				t.Fatalf("%s: el modal tiene Pedido salvo el del reviewer y el del refutador (tiene: %v)", ca, tiene)
			}
			if !sinPedido {
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
		case tablero.ModalSeguir:
			ttSinOrdenes(t, ca, "abrir Seguir solo", ords)
			if got := ttModal(t, ca, m, 200).ttComando(); got != verbo {
				t.Fatalf("CA-716: 'Se va a ejecutar' es el verbo %q: %q", verbo, got)
			}
			ords = m.Tecla(tablero.TeclaEnter, ttAhora) // Seguir solo esta elegido al abrir
		}
		l := ttUnaLanzar(t, ca, "la accion", ords)
		ttMismosArgs(t, ca, "lo que corre es lo que se muestra", l.Args, quiere)
		if l.Slug != "tarjeta" || l.Rol != rol {
			t.Fatalf("%s: el Lanzar es de la tarjeta y del rol de la accion (%q): %#v", ca, rol, l)
		}
		if r := m.Registro(); len(r) == 0 || !strings.HasPrefix(r[len(r)-1].Comando, verbo) {
			t.Fatalf("%s: el Registro lleva el mismo comando: %#v", ca, r)
		}
	}
	if vistas != 11 {
		t.Fatalf("CA-718, CA-753: la tabla tiene las once teclas de accion con verbo (P, las nueve de la v2 y f), tiene %d", vistas)
	}
}
