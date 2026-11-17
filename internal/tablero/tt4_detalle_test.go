// El detalle de la v3 (spec tablero-tu-siguiente-paso): el documento de una
// tarjeta (CA-708, re-expresa CA-556) con AHORA (CA-709), TE TOCA, SIGUE
// (CA-710) y EL CAMINO antes de las partes B a G de la v2; la vista
// Hallazgos (CA-712) y el proyecto en el detalle (CA-713). Lo esperado sale
// de doc.go ("The detail", "The detail of a card", "The detail of the
// project", "The Hallazgos view", "The step of a card"); los modelos son de
// 200 columnas (el detalle de 120, 118 por dentro) y 80 lineas, para que
// el documento entero entre sin partir lineas ni desplazarse, salvo donde
// se prueba eso.
package tablero_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ---------------------------------------------------------------- ayudas del detalle

// tv3aDoc son las lineas interiores del detalle sin el relleno, sin las
// lineas vacias del final.
func tv3aDoc(t *testing.T, ca string, m *tablero.Modelo, ancho int) []string {
	t.Helper()
	d := tv3aDetalleTexto(t, ca, m, ancho)
	for len(d) > 0 && d[len(d)-1] == "" {
		d = d[:len(d)-1]
	}
	return d
}

// tv3aIgualesDoc compara dos documentos linea por linea.
func tv3aIgualesDoc(t *testing.T, ca, que string, m *tablero.Modelo, got, want []string) {
	t.Helper()
	for i := 0; i < len(got) || i < len(want); i++ {
		var g, w string
		if i < len(got) {
			g = got[i]
		}
		if i < len(want) {
			w = want[i]
		}
		if g != w {
			t.Fatalf("%s: %s: la linea %d del detalle es\n%q\nes\n%q\n--- detalle esperado:\n%s\n--- pantalla:\n%s",
				ca, que, i+1, w, g, strings.Join(want, "\n"), tv3aLineas(m))
		}
	}
}

// tv3aDer es una linea interior de inner celdas con der pegado a la
// derecha y un espacio de margen, sin el relleno.
func tv3aDer(izq, der string, inner int) string {
	return strings.TrimRight(tv3aConDerecha(izq, der, inner), " ")
}

// tv3aMedidor son los signos de la tira del modo (hecho, falta, no aplica,
// tu decision).
func tv3aMedidor(utf bool) (hecho, falta, noAplica, decision string) {
	if utf {
		return "●", "○", "·", "✋"
	}
	return "#", "-", ".", "!"
}

// tv3aNombresPaso son los nombres de los pasos del camino, uno por columna.
var tv3aNombresPaso = []string{"Pedido", "Spec", "Tu firma", "Tests", "Código", "Review", "Tu aceptación", "Hecho"}

// tv3aCaminoDoc son las 8 lineas de EL CAMINO (con su encabezado) de una
// tarjeta de la columna col en el grupo g, con estado la palabra de su
// grupo ("" en HECHO), en un detalle de inner celdas por dentro.
func tv3aCaminoDoc(utf bool, col string, g tablero.Grupo, estado string, inner int) []string {
	i := tv3aColumna(col)
	signos := ttCeldasDe(tv3aCamino(utf, col, g))
	out := []string{tv3aDer(" EL CAMINO", fmt.Sprintf("paso %d de 8", i+1), inner)}
	for k, nombre := range tv3aNombresPaso {
		n := tv3aFijo(utf, nombre)
		if k == i {
			out = append(out, tv3aDer("  "+signos[k]+" "+tv3aRelleno(n, 15)+estado, tv3aFijo(utf, "← acá"), inner))
			continue
		}
		out = append(out, "  "+signos[k]+" "+n)
	}
	return out
}

// tv3aDocListado es el documento entero de listado (TE TOCA en Writer, rojo
// esperado, pedir-writer, sin piloto) sin el Detalle, en un detalle de
// inner celdas por dentro donde nada se parte.
func tv3aDocListado(utf bool, inner int) []string {
	f := func(s string) string { return tv3aFijo(utf, s) }
	h, fa, na, _ := tv3aMedidor(utf)
	out := []string{
		tv3aDer(" AHORA", "TE TOCA", inner),
		" " + f("Los tests están escritos y fallan, como se espera."),
		" " + f("falta el código"),
		"",
		" TE TOCA",
		" enter pedir al writer · se abre el pedido para revisarlo antes de lanzar",
		"",
		" SIGUE",
		" despues, te toca pedir la review al reviewer",
		" con el piloto seguiria sola: hoom item set listado --auto hasta-humano",
		"",
	}
	out = append(out, tv3aCaminoDoc(utf, boardcmd.ColWriter, tablero.GrupoTeToca, "te toca a vos", inner)...)
	return append(out,
		"",
		" Writer · gasto sin dato de costo",
		" rojo: los tests del test-writer fallan",
		"",
		"  "+h+" spec: aprobado",
		"  "+h+" pruebas: 4/4 con prueba",
		"  "+fa+" verde: rojo esperado",
		"  "+na+" review: todavia no",
		"",
		" leyendo la tarjeta...",
	)
}

// tv3aEnvolver parte las lineas de mas de w celdas a sus espacios (doc.go,
// "The detail": a lo sumo las palabras que entran; cada continuacion con
// los espacios del principio de la que continua). Las palabras de estos
// tests entran en una linea.
func tv3aEnvolver(ls []string, w int) []string {
	var out []string
	for _, l := range ls {
		if ttCeldas(l) <= w {
			out = append(out, l)
			continue
		}
		lead := l[:len(l)-len(strings.TrimLeft(l, " "))]
		cur := ""
		for _, p := range strings.Fields(l) {
			switch {
			case cur == "":
				cur = lead + p
			case ttCeldas(cur)+1+ttCeldas(p) <= w:
				cur += " " + p
			default:
				out = append(out, cur)
				cur = lead + p
			}
		}
		out = append(out, cur)
	}
	return out
}

// tv3aGrande es el modelo de 200x80 con el tablero b.
func tv3aGrande(utf, color bool, b boardcmd.Board) *tablero.Modelo {
	return ttModelo(tv3aCfg(200, 80, utf, color), b)
}

// tv3aAhoraAngosto es la linea AHORA de la forma angosta: " AHORA   " y la
// frase, cortada para que G entre con dos espacios antes, y G pegado a la
// derecha con un espacio de margen.
func tv3aAhoraAngosto(utf bool, frase, g string, inner int) string {
	room := inner - 9 - 2 - ttCeldas(g) - 1
	return tv3aDer(" AHORA   "+tv3aCortar(frase, room, utf), g, inner)
}

// ================================================================ CA-708: el documento de la tarjeta

// CA-708, CA-556: con 100 columnas o mas el detalle de una tarjeta es un
// documento: AHORA (el grupo a la derecha, la frase y su linea gris), TE
// TOCA (enter con la etiqueta del paso y su ayuda), SIGUE (la frase y la
// linea del piloto), EL CAMINO (paso 5 de 8, los 8 pasos y "← acá" en el de
// la tarjeta) y despues las partes B a G de la v2 (D a G, sin el Detalle,
// "leyendo la tarjeta..."), cada parte despues de una linea vacia. El borde
// de arriba es el titulo y el slug.
func TestV3A_CA708_CA556_DocumentoAncho(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		m := tv3aGrande(utf, false, tv3aTableroDorado())
		if m.Elegida() != "listado" {
			t.Fatalf("CA-708: al abrir la elegida es la primera fila, listado; es %q", m.Elegida())
		}
		de := tv3aDetalle(t, "CA-708", m, 200)
		if quiere := tv3aArriba(utf, false, 120, "Listado de proyectos", "listado"); de.Arriba != quiere {
			t.Fatalf("CA-708 (utf=%v): el borde de arriba del detalle es\n%q\nes\n%q", utf, quiere, de.Arriba)
		}
		tv3aIgualesDoc(t, "CA-708", fmt.Sprintf("listado a 200 (utf=%v)", utf), m, tv3aDoc(t, "CA-708", m, 200), tv3aDocListado(utf, 118))
	}
}

// CA-708: las lineas mas largas que el ancho de adentro se parten a sus
// espacios, cada continuacion con el margen de la que continua; las que
// tienen una parte a la derecha (AHORA, EL CAMINO, la linea de "← acá") se
// arman al ancho y no se parten.
func TestV3A_CA708_LineasLargasSeParten(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		for _, w := range []int{100, 110, 130} {
			m := ttModelo(tv3aCfg(w, 70, utf, false), tv3aTableroDorado())
			inner := (w - tv3aL(w)) - 2
			quiere := tv3aEnvolver(tv3aDocListado(utf, inner), inner)
			tv3aIgualesDoc(t, "CA-708", fmt.Sprintf("listado a %d (utf=%v)", w, utf), m, tv3aDoc(t, "CA-708", m, w), quiere)
		}
	}
}

// CA-708, CA-556: entre 60 y 99 columnas AHORA, TE TOCA, SIGUE y EL CAMINO
// son una linea cada una, sin lineas vacias entre ellas y cortadas en vez
// de partidas (los rotulos a 8 celdas; AHORA sin linea gris ni barra, SIGUE
// sin la linea del piloto); despues una linea vacia y las partes B a G.
func TestV3A_CA708_CA556_FormaAngosta(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		f := func(s string) string { return tv3aFijo(utf, s) }
		h, fa, na, _ := tv3aMedidor(utf)
		for _, w := range []int{99, 80, 60} {
			m := ttModelo(tv3aCfg(w, 80, utf, false), tv3aTableroDorado())
			m.Tecla(tablero.TeclaTab, ttAhora)
			inner := w - 2
			quiere := []string{
				tv3aAhoraAngosto(utf, f("Los tests están escritos y fallan, como se espera."), "TE TOCA", inner),
				strings.TrimRight(tv3aCortar(" TE TOCA enter pedir al writer · se abre el pedido para revisarlo antes de lanzar", inner, utf), " "),
				strings.TrimRight(tv3aCortar(" SIGUE   despues, te toca pedir la review al reviewer", inner, utf), " "),
				strings.TrimRight(tv3aCortar(" CAMINO  "+tv3aCamino(utf, boardcmd.ColWriter, tablero.GrupoTeToca)+" paso 5 de 8: "+f("Código"), inner, utf), " "),
				"",
				" Writer · gasto sin dato de costo",
				" rojo: los tests del test-writer fallan",
				"",
				"  " + h + " spec: aprobado",
				"  " + h + " pruebas: 4/4 con prueba",
				"  " + fa + " verde: rojo esperado",
				"  " + na + " review: todavia no",
				"",
				" leyendo la tarjeta...",
			}
			tv3aIgualesDoc(t, "CA-708", fmt.Sprintf("forma angosta a %d (utf=%v)", w, utf), m, tv3aDoc(t, "CA-708", m, w), quiere)
		}
	}
}

// CA-708: el borde de arriba del detalle de una tarjeta lleva su titulo y,
// a la derecha, su slug, " · riesgo <r>" (riesgo que no es "" ni normal) y
// " · piloto encendido" (auto hasta-humano); lo que no entra se cae desde
// el final; si ni el slug entra, el titulo se corta a (w-2)-slug-2 celdas y,
// si eso no llega a 2, no hay parte derecha.
func TestV3A_CA708_BordeDelDetalle(t *testing.T) {
	defer tv3aSinPanico(t)
	con := func(slug, titulo, riesgoDe string, auto bool) boardcmd.Card {
		c := tv3aListado()
		c.Slug, c.Item.Slug, c.Item.Titulo, c.Item.Riesgo = slug, slug, titulo, riesgoDe
		if auto {
			c.Item.Auto = item.AutoHastaHumano
		}
		return c
	}
	largo := "un-slug-larguisimo-de-la-tarjeta-que-no-entra"
	casos := []struct {
		que      string
		c        boardcmd.Card
		ancho    int
		tit, der string
	}{
		{"todo entra", con("listado", "Listado de proyectos", riesgo.Alto, true), 120, "Listado de proyectos",
			"listado · riesgo alto · piloto encendido"},
		{"se cae el piloto", con("listado", "Listado de proyectos", riesgo.Alto, true), 100, "Listado de proyectos", "listado · riesgo alto"},
		{"se caen el piloto y el riesgo", con("listado", "Listado de proyectos con un titulo largo", riesgo.Alto, true), 100,
			"Listado de proyectos con un titulo largo", "listado"},
		{"riesgo normal no se dice", con("listado", "Listado de proyectos", riesgo.Normal, false), 120, "Listado de proyectos", "listado"},
		{"riesgo vacio no se dice", con("listado", "Listado de proyectos", "", false), 120, "Listado de proyectos", "listado"},
		{"piloto sin riesgo", con("listado", "Listado de proyectos", "", true), 120, "Listado de proyectos", "listado · piloto encendido"},
		{"slug largo: el titulo se corta", con(largo, "Listado de proyectos", "", false), 100,
			tv3aCortar("Listado de proyectos", 58-ttCeldas(largo)-2, true), largo},
		{"slug largo: el titulo se corta a 4", con(largo+"-ni-asi", "Listado de proyectos", "", false), 100,
			tv3aCortar("Listado de proyectos", 4, true), largo + "-ni-asi"},
		{"slug que no entra: sin parte derecha", con(largo+"-ni-asi-tampoco-entra", "Listado de proyectos", "", false), 100,
			"Listado de proyectos", ""},
		{"titulo vacio: el slug", con("listado", "", "", false), 120, "listado", "listado"},
	}
	for _, c := range casos {
		m := ttModelo(tv3aCfg(c.ancho, 40, true, false), ttTablero(c.c))
		w := c.ancho - tv3aL(c.ancho)
		if quiere := tv3aArriba(true, false, w, c.tit, c.der); tv3aDetalle(t, "CA-708", m, c.ancho).Arriba != quiere {
			t.Fatalf("CA-708: %s: el borde de arriba del detalle es\n%q\nes\n%q", c.que, quiere, tv3aDetalle(t, "CA-708", m, c.ancho).Arriba)
		}
	}
	// en una columna el detalle tiene todo el ancho
	m := ttModelo(tv3aCfg(60, 40, true, false), ttTablero(con("listado", "Listado de proyectos", riesgo.Alto, true)))
	if quiere := tv3aArriba(true, false, 60, "Listado de proyectos", "listado · riesgo alto"); tv3aDetalle(t, "CA-708", m, 60).Arriba != quiere {
		t.Fatalf("CA-708: a 60 columnas el borde del detalle es\n%q\nes\n%q", quiere, tv3aDetalle(t, "CA-708", m, 60).Arriba)
	}
}

// CA-708: con running y limite, AHORA tiene la barra de 20 celdas: f = 20 *
// transcurrido / limite (entero, a lo sumo 20) signos de lo corrido en cian
// y 20-f de lo que queda en gris, y "<tiempo> de <limite>"; sin limite no
// hay barra.
func TestV3A_CA708_BarraDelTiempo(t *testing.T) {
	defer tv3aSinPanico(t)
	casos := []struct {
		hace   time.Duration
		limite int
		f      int
		dice   string
	}{
		{14*time.Minute + 2*time.Second, 1800, 9, "14:02 de 30:00"},
		{40 * time.Minute, 1800, 20, "40:00 de 30:00"},
		{0, 600, 0, "0:00 de 10:00"},
		{59*time.Minute + 59*time.Second, 3600, 19, "59:59 de 1:00:00"},
	}
	for _, utf := range []bool{true, false} {
		s := tv3aSig(utf)
		for _, c := range casos {
			card := tv3aCorre(tv3aEn("corre", "Corre", boardcmd.ColWriter), "writer", c.hace, c.limite)
			m := tv3aGrande(utf, true, ttTablero(card))
			d := tv3aDoc(t, "CA-708", m, 200)
			quiere := " tiempo " + strings.Repeat(s.BarraLlena, c.f) + strings.Repeat(s.BarraVacia, 20-c.f) + " " + c.dice
			if len(d) < 4 || tv3aDe(d, 3) != quiere {
				t.Fatalf("CA-708 (utf=%v): la cuarta linea de AHORA es la barra\n%q\nes\n%q\n%s", utf, quiere, d, tv3aLineas(m))
			}
			// la barra en la pantalla: linea 7+3, celdas 80+1+8 en adelante
			tv3aSgrTramo(t, "CA-708", "lo corrido de la barra", m, 10, 89, 89+c.f, "36")
			tv3aSgrTramo(t, "CA-708", "lo que queda de la barra", m, 10, 89+c.f, 109, "90")
		}
		card := tv3aCorre(tv3aEn("corre", "Corre", boardcmd.ColWriter), "writer", time.Minute, 0)
		m := tv3aGrande(utf, false, ttTablero(card))
		for _, l := range tv3aDoc(t, "CA-708", m, 200) {
			if strings.HasPrefix(l, " tiempo ") {
				t.Fatalf("CA-708: sin limite no hay barra del tiempo: %q", l)
			}
		}
	}
}

// CA-708: en modo experto (x) AHORA suma " siguiente: <next>" y una linea
// " missing: <m>" por cada missing, despues de la linea gris; sin el modo
// experto no estan, y en la forma angosta tampoco.
func TestV3A_CA708_LineasDelModoExperto(t *testing.T) {
	defer tv3aSinPanico(t)
	c := tv3aListado()
	c.Next, c.Missing = "pedir-writer", []string{"verde", "review"}
	m := tv3aGrande(true, false, ttTablero(c))
	if d := tv3aDoc(t, "CA-708", m, 200); tv3aDe(d, 3) != "" {
		t.Fatalf("CA-708: sin modo experto AHORA no tiene siguiente ni missing: %q", d[:5])
	}
	m.Tecla("x", ttAhora)
	d := tv3aDoc(t, "CA-708", m, 200)
	quiere := []string{" siguiente: pedir-writer", " missing: verde", " missing: review", "", " TE TOCA"}
	for i, q := range quiere {
		if tv3aDe(d, 3+i) != q {
			t.Fatalf("CA-708: en modo experto la linea %d del detalle es %q, es %q:\n%s", 4+i, q, tv3aDe(d, 3+i), strings.Join(d, "\n"))
		}
	}
	m = ttModelo(tv3aCfg(99, 80, true, false), ttTablero(c))
	m.Tecla("x", ttAhora)
	m.Tecla(tablero.TeclaTab, ttAhora)
	for _, l := range tv3aDoc(t, "CA-708", m, 99) {
		if strings.Contains(l, "siguiente:") || strings.Contains(l, "missing:") {
			t.Fatalf("CA-708: la forma angosta no tiene las lineas del modo experto: %q", l)
		}
	}
}

// tv3aAyudaCaso es una tarjeta sola y la linea de TE TOCA de su detalle.
type tv3aAyudaCaso struct {
	que   string
	c     boardcmd.Card
	linea string
	paso  bool
}

func tv3aAyudaCasos() []tv3aAyudaCaso {
	return []tv3aAyudaCaso{
		{"aprobar", tv3aCon(tv3aEn("x", "X", boardcmd.ColTuAprobacion), ttAccionAprobar()), " enter aprobar spec · hoom no firma por vos", true},
		{"integrar", tv3aCon(tv3aEn("x", "X", boardcmd.ColTuAceptacion), ttAccionIntegrar()), " enter integrar · hoom no acepta por vos", true},
		{"pedir arquitecto", tv3aCon(tv3aEn("x", "X", boardcmd.ColArquitecto), tv3aPedirArq()),
			" enter pedir al arquitecto · se abre el pedido para revisarlo antes de lanzar", true},
		{"pedir test-writer", tv3aCon(tv3aEn("x", "X", boardcmd.ColTestWriter), tv3aPedirTW()),
			" enter pedir al test-writer · se abre el pedido para revisarlo antes de lanzar", true},
		{"pedir writer", tv3aCon(tv3aEn("x", "X", boardcmd.ColWriter), ttAccionWriter()),
			" enter pedir al writer · se abre el pedido para revisarlo antes de lanzar", true},
		{"pedir reviewer", tv3aCon(tv3aEn("x", "X", boardcmd.ColReview), tv3aPedirRev()),
			" enter pedir al reviewer · se abre el pedido para revisarlo antes de lanzar", true},
		{"reanudar", tv3aCon(tv3aEn("x", "X", boardcmd.ColWriter), tv3aReanudar()), " enter reanudar · retoma la sesion que se corto", true},
		{"guardar", tv3aCon(tv3aEn("x", "X", boardcmd.ColWriter), ttAccionGuardar(true, "", "x.go")),
			" enter guardar · guarda el trabajo de la tarjeta en su rama", true},
		{"seguir solo", tv3aConPiloto(tv3aCon(tv3aEn("x", "X", boardcmd.ColReview), tv3aPedirRev())),
			" enter seguir solo · el piloto la lleva hasta tu aceptacion o hasta que algo lo frene", true},
		{"corriendo", tv3aCorre(tv3aCon(tv3aEn("x", "X", boardcmd.ColWriter), ttAccionWriter()), "writer", time.Minute, 0),
			" nada: el writer esta trabajando: espera a que termine", false},
		{"cortada", tv3aCortada(tv3aCon(tv3aEn("x", "X", boardcmd.ColWriter), tv3aReanudar()), "writer"),
			" nada: el trabajo se corto: R reanuda o pide de nuevo", false},
		{"sin acciones", tv3aEn("x", "X", boardcmd.ColWriter), " nada: en Writer no hay nada que hacer ahora: plain de x", false},
		{"aprobar que no tiene", tv3aEn("x", "X", boardcmd.ColTuAprobacion), " nada: en Tu aprobacion no hay aprobar el spec", false},
		{"integrar que no tiene", tv3aCon(tv3aEn("x", "X", boardcmd.ColTuAceptacion), ttAccionGuardar(true, "", "x.go")),
			" nada: en Tu aceptacion no hay integrar", false},
		{"deshabilitada", tv3aCon(tv3aEn("x", "X", boardcmd.ColWriter), ttAccionGuardar(false, "no hay cambios para guardar")),
			" nada: no hay cambios para guardar", false},
		{"primera sin tecla", tv3aCon(tv3aEn("x", "X", boardcmd.ColWriter), ttRelanzar(), ttAccionWriter()),
			" nada: en Writer no hay nada que hacer ahora: plain de x", false},
	}
}

// CA-708: TE TOCA dice "enter <etiqueta en minuscula> · <ayuda>" con la
// ayuda del paso, o "nada: <texto de la regla del paso>" sin paso; en la
// forma angosta, en la misma linea que su rotulo.
func TestV3A_CA708_TeTocaDelDetalle(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, c := range tv3aAyudaCasos() {
		m := tv3aGrande(true, false, ttTablero(c.c))
		d := tv3aDoc(t, "CA-708", m, 200)
		i := tv3aIndice(d, " TE TOCA")
		if i < 0 || i+1 >= len(d) || tv3aDe(d, i+1) != c.linea {
			t.Fatalf("CA-708: %s: despues de \" TE TOCA\" el detalle dice\n%q\n%s", c.que, c.linea, strings.Join(d, "\n"))
		}
		m = ttModelo(tv3aCfg(99, 80, true, false), ttTablero(c.c))
		m.Tecla(tablero.TeclaTab, ttAhora)
		if quiere := " TE TOCA" + c.linea; tv3aDe(tv3aDoc(t, "CA-708", m, 99), 1) != quiere {
			t.Fatalf("CA-708: %s: en la forma angosta la segunda linea del detalle es\n%q\nes\n%q", c.que, quiere, tv3aDe(tv3aDoc(t, "CA-708", m, 99), 1))
		}
	}
}

// CA-708: con color, los rotulos AHORA, SIGUE y EL CAMINO van en negrita
// gris (1;90); TE TOCA en negrita amarilla (1;33) con paso y en negrita gris
// sin paso; el grupo de AHORA con el estilo de su linea de grupo (7;33); la
// frase de AHORA en negrita y su linea gris en gris; la ayuda de TE TOCA y
// la linea del piloto en gris; el rojo de la parte B en rojo.
func TestV3A_CA708_ColoresDelDetalle(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aGrande(true, true, tv3aTableroDorado())
	d := tv3aDoc(t, "CA-708", m, 200)
	n := func(s string) int { // la linea de la pantalla de la linea del detalle que es s
		i := tv3aIndice(d, s)
		if i < 0 {
			t.Fatalf("CA-708: el detalle tiene la linea %q:\n%s", s, strings.Join(d, "\n"))
		}
		return 7 + i
	}
	tv3aSgrDe(t, "CA-708", "AHORA", m, 7, "AHORA", 81, "1;90")
	tv3aSgrDe(t, "CA-708", "el grupo de AHORA", m, 7, "TE TOCA", 90, "7;33")
	tv3aSgrDe(t, "CA-708", "la frase de AHORA", m, 8, "Los tests están escritos y fallan, como se espera.", 81, "1")
	tv3aSgrDe(t, "CA-708", "la linea gris de AHORA", m, 9, "falta el código", 81, "90")
	tv3aSgrDe(t, "CA-708", "TE TOCA con paso", m, n(" TE TOCA"), "TE TOCA", 81, "1;33")
	tv3aSgrDe(t, "CA-708", "la ayuda", m, n(" enter pedir al writer · se abre el pedido para revisarlo antes de lanzar"),
		"se abre el pedido para revisarlo antes de lanzar", 81, "90")
	tv3aSgrDe(t, "CA-708", "SIGUE", m, n(" SIGUE"), "SIGUE", 81, "1;90")
	tv3aSgrDe(t, "CA-708", "la linea del piloto", m, n(" con el piloto seguiria sola: hoom item set listado --auto hasta-humano"),
		"con el piloto seguiria sola: hoom item set listado --auto hasta-humano", 81, "90")
	k := n(tv3aDer(" EL CAMINO", "paso 5 de 8", 118))
	tv3aSgrDe(t, "CA-708", "EL CAMINO", m, k, "EL CAMINO", 81, "1;90")
	tv3aSgrDe(t, "CA-708", "el rojo de B", m, n(" rojo: los tests del test-writer fallan"), "rojo: los tests del test-writer fallan", 81, "31")
	// sin paso: TE TOCA en gris
	m = tv3aGrande(true, true, ttTablero(tv3aModulo()))
	d = tv3aDoc(t, "CA-708", m, 200)
	tv3aSgrDe(t, "CA-708", "TE TOCA sin paso", m, n(" TE TOCA"), "TE TOCA", 81, "1;90")
	tv3aSgrDe(t, "CA-708", "el grupo de AHORA de CORRIENDO", m, 7, "CORRIENDO", 90, "7;36")
}

// CA-708: EL CAMINO dice "paso <i> de 8" (i, el lugar de la columna) y una
// linea por paso con su signo; la de la tarjeta rellena el nombre a 15
// celdas y suma la palabra de su grupo ("te toca a vos", "<rol>, ahora",
// "se detuvo", "sin empezar", nada en HECHO) y "← acá" terminando en la celda
// w-3, en el color de su grupo; los signos en los colores del camino.
func TestV3A_CA708_ElCaminoPorGrupo(t *testing.T) {
	defer tv3aSinPanico(t)
	casos := []struct {
		que    string
		c      boardcmd.Card
		g      tablero.Grupo
		estado string
		color  string
	}{
		{"TE TOCA", tv3aCon(tv3aEn("x", "X", boardcmd.ColTestWriter), tv3aPedirTW()), tablero.GrupoTeToca, "te toca a vos", "33"},
		{"CORRIENDO", tv3aCorre(tv3aEn("x", "X", boardcmd.ColTestWriter), "test-writer", time.Minute, 0), tablero.GrupoCorriendo,
			"test-writer, ahora", "36"},
		{"CORRIENDO sin rol", tv3aCorre(tv3aEn("x", "X", boardcmd.ColArquitecto), "", time.Minute, 0), tablero.GrupoCorriendo,
			"agente, ahora", "36"},
		{"DETENIDA", tv3aRoja(tv3aEn("x", "X", boardcmd.ColReview), "rojo", false), tablero.GrupoDetenida, "se detuvo", "31"},
		{"EN COLA", tv3aEn("x", "X", boardcmd.ColBacklog), tablero.GrupoEnCola, "sin empezar", "90"},
		{"HECHO", tv3aEn("x", "X", boardcmd.ColHecho), tablero.GrupoHecho, "", "90"},
	}
	for _, utf := range []bool{true, false} {
		for _, c := range casos {
			que := fmt.Sprintf("%s (utf=%v)", c.que, utf)
			m := tv3aGrande(utf, true, ttTablero(c.c))
			if c.g == tablero.GrupoHecho {
				tv3aAbrirHecho(t, m)
				m.Tecla("j", ttAhora)
			}
			if m.Elegida() != "x" {
				t.Fatalf("CA-708: %s: el detalle muestra la tarjeta", que)
			}
			d := tv3aDoc(t, "CA-708", m, 200)
			quiere := tv3aCaminoDoc(utf, c.c.Column, c.g, c.estado, 118)
			i := tv3aIndice(d, quiere[0])
			if i < 0 || i+9 > len(d) {
				t.Fatalf("CA-708: %s: el detalle tiene %q:\n%s", que, quiere[0], strings.Join(d, "\n"))
			}
			tv3aIgualesDoc(t, "CA-708", que+": EL CAMINO", m, d[i:i+9], quiere)
			k := tv3aColumna(c.c.Column)
			fila := 7 + i + 1 + k // la linea de la pantalla del paso de la tarjeta
			aca := tv3aFijo(utf, "← acá")
			tv3aSgrDe(t, "CA-708", que+": ← acá", m, fila, aca, 81, c.color)
			if x := tv3aPosEn(m, fila, aca, 81); x+ttCeldas(aca) != 80+118 {
				t.Fatalf("CA-708: %s: \"← acá\" termina en la celda w-3 del detalle (la %d de la pantalla); termina en la %d", que, 80+117, x+ttCeldas(aca)-1)
			}
			if c.estado != "" {
				tv3aSgrDe(t, "CA-708", que+": la palabra del grupo", m, fila, c.estado, 81, c.color)
			}
			colores := map[tablero.Grupo]string{tablero.GrupoTeToca: "33", tablero.GrupoCorriendo: "36", tablero.GrupoDetenida: "31",
				tablero.GrupoEnCola: "90"}
			for p := 0; p < 8; p++ {
				sgr := "90"
				switch {
				case c.g == tablero.GrupoHecho || p < k:
					sgr = "32"
				case p == k:
					sgr = colores[c.g]
				}
				tv3aSgrTramo(t, "CA-708", fmt.Sprintf("%s: el signo del paso %d", que, p+1), m, 7+i+1+p, 83, 84, sgr)
			}
		}
	}
}

// CA-708: despues de EL CAMINO van las partes B a G de la v2, en su orden y
// cada una despues de una linea vacia: B (columna y gasto), C (la tira, con
// ✋ en la primera que falta de una tarjeta que espera tu decision), D (el
// Spec y sus criterios), E (los Hallazgos de su tarea), F (Historia) y G
// (Narración).
func TestV3A_CA708_PartesBaG(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		f := func(s string) string { return tv3aFijo(utf, s) }
		h, fa, na, dec := tv3aMedidor(utf)
		con, sin := "✓", "✗"
		if !utf {
			con, sin = "+", "x"
		}
		m := tv3aGrande(utf, false, ttTablero(ttNucConUntraced()))
		m.LeerDetalle(ttDetalleNuc())
		d := tv3aDoc(t, "CA-708", m, 200)
		i := tv3aIndice(d, tv3aDer(" EL CAMINO", "paso 3 de 8", 118))
		if i < 0 || i+9 >= len(d) {
			t.Fatalf("CA-708 (utf=%v): el detalle de nuc-tenancy tiene EL CAMINO, paso 3 de 8:\n%s", utf, strings.Join(d, "\n"))
		}
		resto := d[i+9:]
		quiere := []string{
			"",
			" Tu aprobacion · gasto sin dato de costo",
			"",
			"  " + h + " spec: pasa el lint",
			"  " + dec + " aprobacion: sin tu firma",
			"  " + fa + " pruebas: 1/2",
			"  " + na + " codigo: todavia no",
			"",
			" Spec · 2 criterios · 1 sin prueba",
			"  " + con + " cada tenant ve solo sus filas",
			"  " + sin + " un tenant sin plan responde 402",
			"",
			" Hallazgos · 1 abierto",
		}
		tv3aIgualesDoc(t, "CA-708", fmt.Sprintf("nuc-tenancy B a E (utf=%v)", utf), m, resto[:len(quiere)], quiere)
		e := resto[len(quiere)]
		if !strings.HasPrefix(e, "  low ") || !strings.Contains(e, "tenant.go:7") || !strings.HasSuffix(e, "el nombre del tenant no se recorta") {
			t.Fatalf("CA-708 (utf=%v): la fila del hallazgo de E es \"  low <donde> <descripcion>\": %q", utf, e)
		}
		cola := []string{"", " Historia", "  sin historia todavia", "", " " + f("Narración"), "  sin sobres todavia"}
		tv3aIgualesDoc(t, "CA-708", fmt.Sprintf("nuc-tenancy F y G (utf=%v)", utf), m, resto[len(quiere)+1:], cola)
	}
}

// CA-708 (correccion del contrato): la linea de pendientes de la tarjeta
// (Card.Pendientes.Texto) va en la parte B, despues de las lineas "
// doctor: ..." y antes de la tira (C), solo cuando no es ""; no va en TE
// TOCA ni como linea propia de la forma angosta.
func TestV3A_CA708_PendientesEnLaParteB(t *testing.T) {
	defer tv3aSinPanico(t)
	c := tv3aCon(ttCard("pend", "Con pendientes", boardcmd.ColTuAceptacion, "te toca aceptar",
		ttSeg("spec", "spec", boardcmd.SegHecho, "aprobado"), ttSeg("review", "review", boardcmd.SegHecho, "sin bloqueos")), ttAccionIntegrar())
	c.Doctor = []boardcmd.Problem{{ID: "p1", Slug: "pend", What: "spec cambiado", Plain: "el spec cambio despues de tu firma",
		Action: "hoom aprobar pend --si"}}
	c.Pendientes = boardcmd.Pendientes{Medium: 1, Low: 2, IDs: []string{"a1", "b2", "c3"}, Texto: "pendientes: 1 medium, 2 low (no bloquean)"}
	h, _, _, _ := tv3aMedidor(true)
	b := []string{
		"",
		" Tu aceptacion · gasto sin dato de costo",
		" doctor: el spec cambio despues de tu firma: hoom aprobar pend --si",
		" pendientes: 1 medium, 2 low (no bloquean)",
		"",
		"  " + h + " spec: aprobado",
		"  " + h + " review: sin bloqueos",
	}
	m := tv3aGrande(true, false, ttTablero(c))
	d := tv3aDoc(t, "CA-708", m, 200)
	i := tv3aIndice(d, tv3aDer(" EL CAMINO", "paso 7 de 8", 118))
	if i < 0 || i+9+len(b) > len(d) {
		t.Fatalf("CA-708: el detalle tiene EL CAMINO y despues B y C:\n%s", strings.Join(d, "\n"))
	}
	tv3aIgualesDoc(t, "CA-708", "pendientes en B", m, d[i+9:i+9+len(b)], b)
	for _, l := range d[:i] {
		if strings.Contains(l, "pendientes") {
			t.Fatalf("CA-708: la linea de pendientes no va en AHORA, TE TOCA ni SIGUE: %q", l)
		}
	}
	m = ttModelo(tv3aCfg(99, 80, true, false), ttTablero(c))
	m.Tecla(tablero.TeclaTab, ttAhora)
	d = tv3aDoc(t, "CA-708", m, 99)
	if len(d) < 4+len(b) {
		t.Fatalf("CA-708: la forma angosta tiene sus cuatro lineas y despues B:\n%s", strings.Join(d, "\n"))
	}
	tv3aIgualesDoc(t, "CA-708", "pendientes en B de la forma angosta", m, d[4:4+len(b)], b)
	// sin pendientes no hay linea
	c.Pendientes = boardcmd.Pendientes{IDs: []string{}}
	m = tv3aGrande(true, false, ttTablero(c))
	for _, l := range tv3aDoc(t, "CA-708", m, 200) {
		if strings.Contains(l, "pendientes") {
			t.Fatalf("CA-708: con Pendientes.Texto \"\" no hay linea de pendientes: %q", l)
		}
	}
}

// CA-708: antes del Detalle las partes D a G son una sola, "leyendo la
// tarjeta..."; con Detalle.Err, " no se pudo leer la tarjeta: <Err>".
func TestV3A_CA708_DetalleLeyendoYError(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aGrande(true, false, ttTablero(ttNucConUntraced()))
	d := tv3aDoc(t, "CA-708", m, 200)
	if d[len(d)-1] != " leyendo la tarjeta..." || d[len(d)-2] != "" {
		t.Fatalf("CA-708: sin el Detalle el documento termina en \" leyendo la tarjeta...\":\n%s", strings.Join(d, "\n"))
	}
	m.LeerDetalle(tablero.Detalle{Slug: "nuc-tenancy", Err: fmt.Errorf("permiso denegado")})
	d = tv3aDoc(t, "CA-708", m, 200)
	if d[len(d)-1] != " no se pudo leer la tarjeta: permiso denegado" {
		t.Fatalf("CA-708: con Detalle.Err el documento termina en \" no se pudo leer la tarjeta: <Err>\":\n%s", strings.Join(d, "\n"))
	}
}

// ================================================================ CA-709: AHORA

// tv3aAhoraCaso es una tarjeta sola, el grupo que AHORA pone a la derecha,
// su frase y su linea gris ("" sin ella).
type tv3aAhoraCaso struct {
	que         string
	c           boardcmd.Card
	g           tablero.Grupo
	frase, gris string
}

func tv3aAhoraCasos() []tv3aAhoraCaso {
	corre := func(col, rol string) boardcmd.Card {
		return tv3aCorre(tv3aEn("x", "X", col), rol, time.Minute, 0)
	}
	sinPasos := corre(boardcmd.ColWriter, "writer")
	sinPasos.Running.Steps = 0
	sinProv := corre(boardcmd.ColWriter, "writer")
	sinProv.Running.Provider = ""
	sinNada := corre(boardcmd.ColWriter, "writer")
	sinNada.Running.Provider, sinNada.Running.Stage, sinNada.Running.Steps = "", "", 0
	const gris = "claude · empezo 11:59 · run 3/6"
	return []tv3aAhoraCaso{
		{"writer", corre(boardcmd.ColWriter, "writer"), tablero.GrupoCorriendo, "El writer está escribiendo el código.", gris},
		{"arquitecto", corre(boardcmd.ColArquitecto, "arquitecto"), tablero.GrupoCorriendo, "El arquitecto está escribiendo el spec.", gris},
		{"test-writer", corre(boardcmd.ColTestWriter, "test-writer"), tablero.GrupoCorriendo, "El test-writer está escribiendo los tests.", gris},
		{"reviewer", corre(boardcmd.ColReview, "reviewer"), tablero.GrupoCorriendo, "El reviewer está revisando el cambio.", gris},
		{"otro rol", corre(boardcmd.ColReview, "refutador"), tablero.GrupoCorriendo, "El refutador está trabajando.", gris},
		{"sin rol", corre(boardcmd.ColWriter, ""), tablero.GrupoCorriendo, "El agente está trabajando.", gris},
		{"sin pasos", sinPasos, tablero.GrupoCorriendo, "El writer está escribiendo el código.", "claude · empezo 11:59 · run"},
		{"sin provider", sinProv, tablero.GrupoCorriendo, "El writer está escribiendo el código.", "empezo 11:59 · run 3/6"},
		{"sin provider ni paso", sinNada, tablero.GrupoCorriendo, "El writer está escribiendo el código.", "empezo 11:59"},
		{"corriendo y cortada", tv3aCortada(corre(boardcmd.ColWriter, "writer"), "writer"), tablero.GrupoCorriendo,
			"El writer está escribiendo el código.", gris},
		{"cortada", tv3aCortada(tv3aEn("x", "X", boardcmd.ColWriter), "writer"), tablero.GrupoDetenida,
			"El trabajo del writer se cortó en el paso 3 de 5.", "ya no corre: R lo reanuda"},
		{"cortada sin rol", tv3aCortada(tv3aEn("x", "X", boardcmd.ColWriter), ""), tablero.GrupoDetenida,
			"El trabajo del agente se cortó en el paso 3 de 5.", "ya no corre: R lo reanuda"},
		{"cortada y roja", tv3aRoja(tv3aCortada(tv3aEn("x", "X", boardcmd.ColWriter), "writer"), "rojo de x", false), tablero.GrupoDetenida,
			"El trabajo del writer se cortó en el paso 3 de 5.", "ya no corre: R lo reanuda"},
		{"rojo no esperado", tv3aRoja(tv3aEn("x", "X", boardcmd.ColReview), "el veredicto quedo rojo en test", false), tablero.GrupoDetenida,
			"el veredicto quedo rojo en test", ""},
		{"rojo no esperado en Tu aprobacion", tv3aRoja(tv3aEn("x", "X", boardcmd.ColTuAprobacion), "el spec no pasa", false),
			tablero.GrupoDetenida, "el spec no pasa", ""},
		{"rojo esperado", tv3aRoja(tv3aEn("x", "X", boardcmd.ColWriter), "fallan", true), tablero.GrupoTeToca,
			"Los tests están escritos y fallan, como se espera.", "falta el código"},
		{"rojo esperado en Backlog", tv3aRoja(tv3aEn("x", "X", boardcmd.ColBacklog), "fallan", true), tablero.GrupoEnCola,
			"Los tests están escritos y fallan, como se espera.", "falta el código"},
		{"Tu aprobacion", tv3aEn("x", "X", boardcmd.ColTuAprobacion), tablero.GrupoTeToca, "El spec está listo y espera tu firma.", "plain de x"},
		{"Tu aceptacion", tv3aEn("x", "X", boardcmd.ColTuAceptacion), tablero.GrupoTeToca, "Está lista para que la pruebes y la aceptes.",
			"plain de x"},
		{"Backlog", tv3aEn("x", "X", boardcmd.ColBacklog), tablero.GrupoEnCola, "Todavía no empezó.", "plain de x"},
		{"Hecho", tv3aEn("x", "X", boardcmd.ColHecho), tablero.GrupoHecho, "Terminada.", "plain de x"},
		{"Writer sin nada", tv3aEn("x", "X", boardcmd.ColWriter), tablero.GrupoTeToca, "plain de x", ""},
		{"Arquitecto sin nada", tv3aEn("x", "X", boardcmd.ColArquitecto), tablero.GrupoTeToca, "plain de x", ""},
	}
}

// CA-709: la frase y la linea gris de AHORA son las del primer caso que se
// cumple de la tabla del spec (running por su rol, interrupted, rojo no
// esperado, rojo esperado, Tu aprobacion, Tu aceptacion, Backlog, Hecho, el
// plain); a su derecha, el nombre del grupo. En ASCII las frases pierden
// sus acentos.
func TestV3A_CA709_FraseDeAhora(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		for _, c := range tv3aAhoraCasos() {
			que := fmt.Sprintf("%s (utf=%v)", c.que, utf)
			m := tv3aGrande(utf, false, ttTablero(c.c))
			if c.g == tablero.GrupoHecho {
				tv3aAbrirHecho(t, m)
				m.Tecla("j", ttAhora)
			}
			d := tv3aDoc(t, "CA-709", m, 200)
			if len(d) < 4 {
				t.Fatalf("CA-709: %s: el detalle tiene AHORA:\n%s", que, tv3aLineas(m))
			}
			if quiere := tv3aDer(" AHORA", string(c.g), 118); tv3aDe(d, 0) != quiere {
				t.Fatalf("CA-709: %s: la primera linea es\n%q\nes\n%q", que, quiere, tv3aDe(d, 0))
			}
			if quiere := " " + tv3aFijo(utf, c.frase); tv3aDe(d, 1) != quiere {
				t.Fatalf("CA-709: %s: la frase de AHORA es\n%q\nes\n%q", que, quiere, tv3aDe(d, 1))
			}
			quiere := ""
			if c.gris != "" {
				quiere = " " + tv3aFijo(utf, c.gris)
			}
			if tv3aDe(d, 2) != quiere {
				t.Fatalf("CA-709: %s: la linea gris de AHORA es\n%q\nes\n%q", que, quiere, tv3aDe(d, 2))
			}
			if c.gris != "" && tv3aDe(d, 3) != "" {
				t.Fatalf("CA-709: %s: sin limite ni modo experto, AHORA termina en su linea gris: %q", que, tv3aDe(d, 3))
			}
			// la forma angosta: solo la frase, con el grupo a la derecha
			m = ttModelo(tv3aCfg(99, 80, utf, false), ttTablero(c.c))
			if c.g == tablero.GrupoHecho {
				tv3aAbrirHecho(t, m)
				m.Tecla("j", ttAhora)
			}
			m.Tecla(tablero.TeclaTab, ttAhora)
			if quiere := tv3aAhoraAngosto(utf, tv3aFijo(utf, c.frase), string(c.g), 97); tv3aDe(tv3aDoc(t, "CA-709", m, 99), 0) != quiere {
				t.Fatalf("CA-709: %s: en la forma angosta AHORA es\n%q\nes\n%q", que, quiere, tv3aDe(tv3aDoc(t, "CA-709", m, 99), 0))
			}
		}
	}
}

// CA-709: en modo experto la linea gris de un rojo no esperado es su
// red.reason (y la parte B dice el red.reason en " rojo: ").
func TestV3A_CA709_RojoEnModoExperto(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aGrande(true, false, ttTablero(tv3aNotificaciones()))
	m.Tecla("x", ttAhora)
	d := tv3aDoc(t, "CA-709", m, 200)
	if tv3aDe(d, 1) != " el veredicto quedo rojo en test" || tv3aDe(d, 2) != " test: FAIL" {
		t.Fatalf("CA-709: en modo experto AHORA dice el plain del rojo y su reason: %q", d[:3])
	}
	if tv3aIndice(d, " rojo: test: FAIL") < 0 {
		t.Fatalf("CA-709: en modo experto la parte B dice \" rojo: <red.reason>\":\n%s", strings.Join(d, "\n"))
	}
}

// CA-709 (hostil): una frase que viene de la tarjeta (el plain de un rojo)
// con controles, larga y con caracteres anchos: los controles son "?" y se
// parte a sus espacios sin pasar el ancho; en la forma angosta se corta
// para que el grupo entre.
func TestV3A_CA709_FraseHostil(t *testing.T) {
	defer tv3aSinPanico(t)
	plain := "rojo\x1b[2J en 漢字 " + strings.Repeat("palabra ", 30) + "🚀"
	c := tv3aRoja(tv3aEn("x", "X", boardcmd.ColReview), plain, false)
	m := ttModelo(tv3aCfg(110, 60, true, false), ttTablero(c))
	ttForma(t, "CA-709", m, 110, 60, false)
	d := tv3aDoc(t, "CA-709", m, 110)
	if !strings.HasPrefix(tv3aDe(d, 1), " rojo?[2J en 漢字 palabra") {
		t.Fatalf("CA-709: el ESC de la frase es \"?\": %q", tv3aDe(d, 1))
	}
	for _, l := range d {
		if ttCeldas(l) > 64 {
			t.Fatalf("CA-709: ninguna linea del detalle pasa sus 64 celdas de adentro: %q", l)
		}
	}
	m = ttModelo(tv3aCfg(70, 60, true, false), ttTablero(c))
	m.Tecla(tablero.TeclaTab, ttAhora)
	ttForma(t, "CA-709", m, 70, 60, false)
	if quiere := tv3aAhoraAngosto(true, "rojo?[2J en 漢字 "+strings.Repeat("palabra ", 30)+"🚀", "DETENIDA", 68); tv3aDe(tv3aDoc(t, "CA-709", m, 70), 0) != quiere {
		t.Fatalf("CA-709: en la forma angosta la frase se corta para que DETENIDA entre:\n%q\nes\n%q", quiere, tv3aDe(tv3aDoc(t, "CA-709", m, 70), 0))
	}
}

// ================================================================ CA-710: SIGUE

// CA-710: la frase de SIGUE sale de la columna de la tarjeta y de si su
// item tiene auto hasta-humano (la tabla del spec); sin el piloto, en Tu
// aprobacion, Test-writer, Writer y Review le sigue la linea gris "con el
// piloto seguiria sola: hoom item set <slug> --auto hasta-humano"; en la
// forma angosta SIGUE es una linea, sin la linea gris.
func TestV3A_CA710_FraseDeSigue(t *testing.T) {
	defer tv3aSinPanico(t)
	tabla := []struct {
		col         string
		on, off     string
		lineaPiloto bool
	}{
		{boardcmd.ColBacklog, "el arquitecto escribe el spec; despues te toca firmarlo", "el arquitecto escribe el spec; despues te toca firmarlo", false},
		{boardcmd.ColArquitecto, "te toca firmar el spec", "te toca firmar el spec", false},
		{boardcmd.ColTuAprobacion, "al firmar, el piloto pide los tests y sigue solo hasta tu aceptación",
			"al firmar, te toca pedir los tests al test-writer", true},
		{boardcmd.ColTestWriter, "el piloto pide el código al writer y sigue hasta tu aceptación", "despues, te toca pedir el código al writer", true},
		{boardcmd.ColWriter, "el piloto pide la review; si algo bloquea, hace una ronda de hallazgos",
			"despues, te toca pedir la review al reviewer", true},
		{boardcmd.ColReview, "si nada bloquea, te toca aceptar; si algo bloquea, el piloto hace una ronda de hallazgos",
			"si nada bloquea, te toca aceptar", true},
		{boardcmd.ColTuAceptacion, "al aceptar, hoom integra y la tarjeta pasa a Hecho", "al aceptar, hoom integra y la tarjeta pasa a Hecho", false},
		{boardcmd.ColHecho, "nada: la tarjeta esta terminada", "nada: la tarjeta esta terminada", false},
	}
	for _, utf := range []bool{true, false} {
		for _, f := range tabla {
			for _, auto := range []bool{false, true} {
				que := fmt.Sprintf("%s con piloto %v (utf=%v)", f.col, auto, utf)
				c := tv3aEn("sig-x", "X", f.col)
				frase := f.off
				if auto {
					c = tv3aConPiloto(c)
					frase = f.on
				}
				m := tv3aGrande(utf, false, ttTablero(c))
				if f.col == boardcmd.ColHecho {
					tv3aAbrirHecho(t, m)
					m.Tecla("j", ttAhora)
				}
				d := tv3aDoc(t, "CA-710", m, 200)
				i := tv3aIndice(d, " SIGUE")
				if i < 0 || i+2 >= len(d) {
					t.Fatalf("CA-710: %s: el detalle tiene SIGUE:\n%s", que, strings.Join(d, "\n"))
				}
				if quiere := " " + tv3aFijo(utf, frase); tv3aDe(d, i+1) != quiere {
					t.Fatalf("CA-710: %s: la frase de SIGUE es\n%q\nes\n%q", que, quiere, tv3aDe(d, i+1))
				}
				gris := ""
				if f.lineaPiloto && !auto {
					gris = " con el piloto seguiria sola: hoom item set sig-x --auto hasta-humano"
				}
				if tv3aDe(d, i+2) != gris {
					t.Fatalf("CA-710: %s: despues de la frase de SIGUE va\n%q\nva\n%q", que, gris, tv3aDe(d, i+2))
				}
				// la forma angosta
				m = ttModelo(tv3aCfg(99, 80, utf, false), ttTablero(c))
				if f.col == boardcmd.ColHecho {
					tv3aAbrirHecho(t, m)
					m.Tecla("j", ttAhora)
				}
				m.Tecla(tablero.TeclaTab, ttAhora)
				da := tv3aDoc(t, "CA-710", m, 99)
				if quiere := strings.TrimRight(tv3aCortar(" SIGUE   "+tv3aFijo(utf, frase), 97, utf), " "); tv3aDe(da, 2) != quiere {
					t.Fatalf("CA-710: %s: en la forma angosta SIGUE es\n%q\nes\n%q", que, quiere, tv3aDe(da, 2))
				}
				for _, l := range da {
					if strings.Contains(l, "con el piloto seguiria sola") {
						t.Fatalf("CA-710: %s: la forma angosta no tiene la linea del piloto", que)
					}
				}
			}
		}
	}
}

// CA-710: en una columna angosta la frase de SIGUE se corta a w-2 con …/~.
func TestV3A_CA710_SigueCortadaEnLaFormaAngosta(t *testing.T) {
	defer tv3aSinPanico(t)
	c := tv3aConPiloto(tv3aEn("x", "X", boardcmd.ColReview))
	for _, utf := range []bool{true, false} {
		m := ttModelo(tv3aCfg(64, 60, utf, false), ttTablero(c))
		m.Tecla(tablero.TeclaTab, ttAhora)
		frase := " SIGUE   si nada bloquea, te toca aceptar; si algo bloquea, el piloto hace una ronda de hallazgos"
		if quiere := tv3aCortar(frase, 62, utf); tv3aDe(tv3aDoc(t, "CA-710", m, 64), 2) != quiere {
			t.Fatalf("CA-710 (utf=%v): SIGUE cortada es\n%q\nes\n%q", utf, quiere, tv3aDe(tv3aDoc(t, "CA-710", m, 64), 2))
		}
	}
}

// ================================================================ CA-712: la vista Hallazgos

// tv3aFilaHallazgo es una fila de la vista Hallazgos en una lista de w
// celdas (doc.go): " " + M + " " + severidad a 6 + " " + tarea a room/2 +
// " " + donde cortado a room - room/2 - 1, room = (w-2) - 10.
func tv3aFilaHallazgo(utf bool, w int, elegida bool, sev, tarea, donde string) string {
	room := (w - 2) - 10
	m := " "
	if elegida {
		m = tv3aSig(utf).Sel
	}
	return strings.TrimRight(" "+m+" "+tv3aRelleno(sev, 6)+" "+tv3aRelleno(tv3aCortar(tarea, room/2, utf), room/2)+" "+
		tv3aCortar(donde, room-room/2-1, utf), " ")
}

// tv3aModeloHallazgos es el modelo dorado de 200x40 con los hallazgos.
func tv3aModeloHallazgos(utf, color bool) *tablero.Modelo {
	m := tv3aModeloDorado(tv3aCfg(200, 40, utf, color))
	m.LeerHallazgos(ttHallazgosDorados())
	return m
}

// CA-712, CA-559: H cambia la lista por los hallazgos abiertos del proyecto
// (titulo "Hallazgos <n>"), el high primero, cada fila con severidad, tarea y
// archivo:linea, y el detalle muestra el elegido ("hallazgo <id>").
func TestV3A_CA712_CA559_VistaHallazgos(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		m := tv3aModeloHallazgos(utf, false)
		m.Tecla("H", ttAhora)
		if m.Vista() != tablero.VistaHallazgos || m.Lugar() != tablero.LugarHallazgos || m.Foco() != tablero.ZonaLista {
			t.Fatalf("CA-712 (utf=%v): H abre la vista Hallazgos con el foco en su lista; Vista %q, Lugar %q, Foco %v",
				utf, m.Vista(), m.Lugar(), m.Foco())
		}
		li := tv3aLista(t, "CA-712", m, 200)
		if !strings.HasPrefix(li.Arriba, tv3aSig(utf).FESI+"Hallazgos 2") || !li.Foco {
			t.Fatalf("CA-712 (utf=%v): la lista se titula \"Hallazgos 2\" y tiene el foco: %q", utf, li.Arriba)
		}
		txt := li.tv3aTexto()
		if quiere := tv3aFilaHallazgo(utf, 80, true, "high", "infra-api", "router.go:41"); tv3aDe(txt, 0) != quiere {
			t.Fatalf("CA-712 (utf=%v): la primera fila es el high, elegido:\n%q\nes\n%q", utf, quiere, tv3aDe(txt, 0))
		}
		if quiere := tv3aFilaHallazgo(utf, 80, false, "low", "nuc-tenancy", "tenant.go:7"); tv3aDe(txt, 1) != quiere {
			t.Fatalf("CA-712 (utf=%v): la segunda fila es el low:\n%q\nes\n%q", utf, quiere, tv3aDe(txt, 1))
		}
		de := tv3aDetalle(t, "CA-712", m, 200)
		if !strings.HasPrefix(de.Arriba, tv3aSig(utf).ESI+"hallazgo h-high") {
			t.Fatalf("CA-712 (utf=%v): el detalle muestra el hallazgo elegido, \"hallazgo h-high\": %q", utf, de.Arriba)
		}
		d := de.tv3aTexto()
		if tv3aDe(d, 0) != " high · lente risk · tarea infra-api" || tv3aDe(d, 1) != " el router acepta rutas sin version" {
			t.Fatalf("CA-712 (utf=%v): el detalle del hallazgo empieza con severidad, lente y tarea, y la descripcion: %q", utf, d[:2])
		}
		m.Tecla("j", ttAhora)
		if !strings.HasPrefix(tv3aDetalle(t, "CA-712", m, 200).Arriba, tv3aSig(utf).ESI+"hallazgo h-low") {
			t.Fatalf("CA-712 (utf=%v): j elige el low y el detalle lo sigue", utf)
		}
	}
}

// CA-712: con color, el titulo de la vista lleva " · <k> high" en rojo (31
// solo, dentro del estilo del borde con el foco, 1;32), y la severidad de
// cada fila va en su color (high 31, low 90).
func TestV3A_CA712_TituloConLosHighEnRojo(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloHallazgos(true, true)
	m.Tecla("H", ttAhora)
	tv3aSgrDe(t, "CA-712", "el titulo", m, 6, "Hallazgos 2", 1, "1;32")
	tv3aSgrDe(t, "CA-712", "los high del titulo", m, 6, " · 1 high", 12, "31")
	// la fila elegida lleva la barra de la seleccion: se mira la otra
	tv3aSgrDe(t, "CA-712", "la severidad low", m, 8, "low", 3, "90")
	m.Tecla("j", ttAhora)
	tv3aSgrDe(t, "CA-712", "la severidad high", m, 7, "high", 3, "31")
}

// CA-712, CA-559: el orden de la vista es high, medium, low y despues otra
// severidad, cada grupo en el orden de la lectura.
func TestV3A_CA712_CA559_OrdenPorSeveridad(t *testing.T) {
	defer tv3aSinPanico(t)
	h := func(id, sev, tarea string) finding.Item {
		x := ttHallazgo(id, finding.StatusOpen, sev, "a/"+id+".go:1", "desc "+id)
		x.Task = tarea
		return x
	}
	m := tv3aModeloDorado(tv3aCfg(200, 40, true, false))
	m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{h("a", "low", "t-a"), h("b", "medium", "t-b"), h("c", "info", "t-c"),
		h("d", "high", "t-d"), h("e", "medium", "t-e"), h("f", "high", "t-f")}})
	m.Tecla("H", ttAhora)
	txt := tv3aLista(t, "CA-712", m, 200).tv3aTexto()
	quiere := []struct{ sev, tarea, donde string }{{"high", "t-d", "d.go:1"}, {"high", "t-f", "f.go:1"}, {"medium", "t-b", "b.go:1"},
		{"medium", "t-e", "e.go:1"}, {"low", "t-a", "a.go:1"}, {"info", "t-c", "c.go:1"}}
	for i, q := range quiere {
		if f := tv3aFilaHallazgo(true, 80, i == 0, q.sev, q.tarea, q.donde); tv3aDe(txt, i) != f {
			t.Fatalf("CA-712: la fila %d es\n%q\nes\n%q\n%s", i+1, f, tv3aDe(txt, i), strings.Join(txt, "\n"))
		}
	}
	if !strings.HasPrefix(tv3aLista(t, "CA-712", m, 200).Arriba, "╔Hallazgos 6") {
		t.Fatalf("CA-712: el titulo cuenta los 6 abiertos")
	}
}

// CA-712: sin hallazgos la vista dice " sin hallazgos abiertos" (titulo
// "Hallazgos"); antes de la primera lectura, " leyendo los hallazgos...".
func TestV3A_CA712_SinHallazgosYLeyendo(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(200, 40, true, false))
	m.Tecla("H", ttAhora)
	if txt := tv3aLista(t, "CA-712", m, 200).tv3aTexto(); tv3aDe(txt, 0) != " leyendo los hallazgos..." {
		t.Fatalf("CA-712: antes de leer los hallazgos la vista dice \" leyendo los hallazgos...\": %q", txt)
	}
	m.LeerHallazgos(tablero.Hallazgos{Items: []finding.Item{}})
	li := tv3aLista(t, "CA-712", m, 200)
	if txt := li.tv3aTexto(); tv3aDe(txt, 0) != " sin hallazgos abiertos" {
		t.Fatalf("CA-712: sin hallazgos la vista dice \" sin hallazgos abiertos\": %q", txt)
	}
	if quiere := tv3aArriba(true, true, 80, "Hallazgos", ""); li.Arriba != quiere {
		t.Fatalf("CA-712: sin hallazgos el titulo es \"Hallazgos\":\n%q\nes\n%q", quiere, li.Arriba)
	}
}

// CA-712: esc en la lista de la vista y H en cualquier lugar de ella
// vuelven a las tarjetas con la que estaba elegida y el foco en la lista; /
// vuelve a las tarjetas y abre la busqueda.
func TestV3A_CA712_VolverALasTarjetas(t *testing.T) {
	defer tv3aSinPanico(t)
	volver := []struct {
		que    string
		teclas []tablero.Tecla
	}{
		{"esc en la lista", []tablero.Tecla{tablero.TeclaEsc}},
		{"H en la lista", []tablero.Tecla{"H"}},
		{"H en el detalle de un hallazgo", []tablero.Tecla{tablero.TeclaTab, "H"}},
	}
	for _, v := range volver {
		m := tv3aModeloHallazgos(true, false)
		tv3aElegir(t, m, "exportar-pdf")
		m.Tecla("H", ttAhora)
		m.Tecla("j", ttAhora)
		ttTeclas(m, v.teclas...)
		if m.Vista() != tablero.VistaTarjetas || m.Elegida() != "exportar-pdf" || m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarTarjetas {
			t.Fatalf("CA-712: %s vuelve a las tarjetas con exportar-pdf elegida y el foco en la lista; Vista %q, Elegida %q, Foco %v, Lugar %q",
				v.que, m.Vista(), m.Elegida(), m.Foco(), m.Lugar())
		}
		if !strings.HasPrefix(tv3aDetalle(t, "CA-712", m, 200).Arriba, "┌Exportar reporte a PDF") {
			t.Fatalf("CA-712: %s: el detalle vuelve a mostrar la tarjeta elegida:\n%s", v.que, tv3aLineas(m))
		}
		if !strings.HasPrefix(tv3aLista(t, "CA-712", m, 200).Arriba, "╔Tarjetas") {
			t.Fatalf("CA-712: %s: la lista vuelve a ser la de las tarjetas", v.que)
		}
	}
	m := tv3aModeloHallazgos(true, false)
	m.Tecla("H", ttAhora)
	m.Tecla("/", ttAhora)
	if m.Vista() != tablero.VistaTarjetas || m.Lugar() != tablero.LugarBusqueda {
		t.Fatalf("CA-712: / en la vista Hallazgos vuelve a las tarjetas y abre la busqueda; Vista %q, Lugar %q", m.Vista(), m.Lugar())
	}
}

// ================================================================ CA-713: el proyecto en el detalle

// tv3aDocProyecto es el documento del proyecto verde de los fixtures.
func tv3aDocProyecto(fijada string) []string {
	return []string{
		" demo · rama main",
		" check VERDE · verify hace 2h",
		" hoy 3.88 USD en 4 trabajos",
		fijada,
		"",
		" Veredicto",
		"  20261108T100000_abc123 · hace 2h · VERDE",
		"  pass 7 · fail 0 · error 0 · absent 1",
		"  .hoom/verdicts/20261108T100000_abc123.json",
		"",
		" Check",
		"  check VERDE · verify hace 2h",
		"",
		" Doctor",
		"  sin problemas",
	}
}

// CA-713: v muestra el proyecto en el detalle, con el titulo "Proyecto" y
// el foco en el detalle: las cuatro lineas del [1] de la v2 y despues el
// Veredicto, el Check y el Doctor; esc y tab vuelven el foco a la lista y el
// detalle vuelve a mostrar la tarjeta elegida.
func TestV3A_CA713_ProyectoEnElDetalle(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, salir := range []tablero.Tecla{tablero.TeclaEsc, tablero.TeclaTab} {
		m := tv3aModeloDorado(tv3aCfg(200, 40, true, false))
		tv3aElegir(t, m, "notificaciones")
		m.Tecla("v", ttAhora)
		if m.Lugar() != tablero.LugarVerProyecto || m.Foco() != tablero.ZonaDetalle {
			t.Fatalf("CA-713: v pone el foco en el detalle con el proyecto; Lugar %q, Foco %v", m.Lugar(), m.Foco())
		}
		de := tv3aDetalle(t, "CA-713", m, 200)
		if quiere := tv3aArriba(true, true, 120, "Proyecto", ""); de.Arriba != quiere {
			t.Fatalf("CA-713: el detalle se titula Proyecto, con el foco:\n%q\nes\n%q", quiere, de.Arriba)
		}
		tv3aIgualesDoc(t, "CA-713", "el proyecto", m, tv3aDoc(t, "CA-713", m, 200), tv3aDocProyecto(" fijada: exportar-pdf"))
		m.Tecla(salir, ttAhora)
		if m.Foco() != tablero.ZonaLista || m.Lugar() != tablero.LugarTarjetas || m.Elegida() != "notificaciones" {
			t.Fatalf("CA-713: %s vuelve el foco a la lista con notificaciones elegida; Foco %v, Lugar %q, Elegida %q",
				salir, m.Foco(), m.Lugar(), m.Elegida())
		}
		if !strings.HasPrefix(tv3aDetalle(t, "CA-713", m, 200).Arriba, "┌Notificaciones por correo") {
			t.Fatalf("CA-713: tras %s el detalle vuelve a la tarjeta elegida:\n%s", salir, tv3aLineas(m))
		}
	}
}

// CA-713: con avisos el documento suma " Avisos" y uno por linea antes del
// Doctor; sin fijada dice " sin tarjeta fijada"; antes del primer Proyecto,
// " leyendo el proyecto...".
func TestV3A_CA713_AvisosSinFijadaYLeyendo(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tablero.Nuevo(tv3aCfg(200, 40, true, false))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: tv3aTableroDorado()})
	m.Tecla("v", ttAhora)
	if d := tv3aDoc(t, "CA-713", m, 200); len(d) != 1 || tv3aDe(d, 0) != " leyendo el proyecto..." {
		t.Fatalf("CA-713: antes del primer Proyecto el detalle dice \" leyendo el proyecto...\": %q", d)
	}
	p := ttProyectoVerde()
	p.Avisos = []string{"no se pudo leer .hoom/verdicts/roto.json"}
	m.LeerProyecto(p)
	quiere := tv3aDocProyecto(" sin tarjeta fijada")
	quiere = append(quiere[:13:13], " Avisos", "  no se pudo leer .hoom/verdicts/roto.json", "", " Doctor", "  sin problemas")
	tv3aIgualesDoc(t, "CA-713", "el proyecto con avisos", m, tv3aDoc(t, "CA-713", m, 200), quiere)
}

// CA-713: en una columna, v pliega la lista y el detalle del proyecto ocupa
// las C-1 lineas, con el foco.
func TestV3A_CA713_ProyectoEnUnaColumna(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(80, 40, true, false))
	m.Tecla("v", ttAhora)
	if !tv3aListaPlegada(m) {
		t.Fatalf("CA-713: con el foco en el detalle del proyecto la lista se pliega:\n%s", tv3aLineas(m))
	}
	de := tv3aCajaEn(t, "CA-713", m, 6, 0, 80)
	if de.Alto != 31 || !de.Foco || !strings.HasPrefix(de.Arriba, "╔Proyecto═") {
		t.Fatalf("CA-713: el detalle del proyecto ocupa C-1 = 31 lineas con el foco y el titulo Proyecto:\n%s", tv3aLineas(m))
	}
	tv3aIgualesDoc(t, "CA-713", "el proyecto en una columna", m, tv3aDoc(t, "CA-713", m, 80), tv3aDocProyecto(" fijada: exportar-pdf"))
}
