// Los archivos dorados de la v3 (spec tablero-tu-siguiente-paso, "Como se
// prueba"; CA-696, CA-720, y los de la v2 que re-expresa: CA-550, CA-576,
// CA-577): la pantalla entera a 120x36, 100x30, 80x24 y 60x15 sin color y en
// ASCII, una vez con los signos UTF-8, y la de CA-577 con 🚀 y 漢字 en un
// titulo, comparadas linea por linea con testdata/v3-*.txt. Los dorados se
// escribieron a mano desde doc.go ("The screen", "The layout of the body",
// "The boxes", "The header", "The box TU SIGUIENTE PASO", "The list", "The
// rows", "The path", "The detail of a card", "The Registro", "The two last
// lines") para el tablero de tv3aTableroDorado: no hay flag -update, un
// dorado regenerado desde el codigo seria circular. Reemplazan los de la v2
// (testdata/paneles-*.txt): es el cambio de pantalla que el spec aprueba.
package tablero_test

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// CA-696, CA-550, CA-720, CA-576: 120x36 en ASCII: dos zonas (la lista en
// 48 celdas, el detalle en 72), la cabecera entera (con la fijada), la caja
// con los tres "despues", los cinco grupos, el camino en ASCII (# ! * x -),
// el detalle de listado (rojo esperado) en su forma ancha y "<- aca".
func TestV3A_CA696_CA550_CA720_CA576_DoradoASCII120x36(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(120, 36, false, false))
	ttForma(t, "CA-696", m, 120, 36, false)
	ttDorado(t, "CA-696", "v3-ascii-120x36.txt", m)
}

// CA-696, CA-550, CA-720, CA-576: 100x30 en ASCII: el ancho minimo de las
// dos zonas (la lista en 40, el detalle en 60, 58 por dentro), la cabecera
// sin la fijada, el tercer titulo de "despues" cortado con ~ y los textos
// largos del detalle partidos a sus espacios.
func TestV3A_CA696_CA550_CA720_CA576_DoradoASCII100x30(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(100, 30, false, false))
	ttForma(t, "CA-696", m, 100, 30, false)
	ttDorado(t, "CA-696", "v3-ascii-100x30.txt", m)
}

// CA-696, CA-550, CA-720, CA-576: 80x24 en ASCII: una columna, la lista
// arriba con el foco en (C+1)/2 = 8 lineas, que se desplaza y dice "1 de 7",
// cada tarjeta en una linea con el camino en la misma columna, y el detalle
// abajo en su forma angosta (AHORA, TE TOCA, SIGUE y CAMINO en una linea
// cada uno, cortados con ~).
func TestV3A_CA696_CA550_CA720_CA576_DoradoASCII80x24(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(80, 24, false, false))
	ttForma(t, "CA-696", m, 80, 24, false)
	ttDorado(t, "CA-696", "v3-ascii-80x24.txt", m)
}

// CA-696, CA-550, CA-720, CA-576: 60x15 en ASCII: el minimo; la cabecera
// corta el check, la lista tiene 4 lineas y el detalle 3 (solo AHORA).
func TestV3A_CA696_CA550_CA720_CA576_DoradoASCII60x15(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(60, 15, false, false))
	ttForma(t, "CA-696", m, 60, 15, false)
	ttDorado(t, "CA-696", "v3-ascii-60x15.txt", m)
}

// CA-720, CA-576, CA-696: 100x30 con los signos UTF-8: las lineas dobles del
// foco, ● ◆ ◉ ✗ ○, ▸, «», el corte …, "← acá" y los acentos de las frases
// fijas (Pedile el código..., Los tests están..., Código, después:); los
// textos de las tarjetas van como vienen.
func TestV3A_CA720_CA576_CA696_DoradoUTF8_100x30(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(100, 30, true, false))
	ttForma(t, "CA-720", m, 100, 30, false)
	ttDorado(t, "CA-720", "v3-utf8-100x30.txt", m)
}

// CA-577, CA-701, CA-696: dos tarjetas con caracteres anchos (🚀, 漢字) en
// sus titulos y plains, a 80x24 con UTF-8 y sin otras lecturas que el
// tablero: la caja, las filas de la lista (el titulo ancho cortado con …
// sin partir un caracter) y el borde del detalle no se corren.
func TestV3A_CA577_CA701_DoradoAnchos(t *testing.T) {
	defer tv3aSinPanico(t)
	cohete := ttCard("cohete", "Despegue 🚀 del 漢字", boardcmd.ColTuAprobacion, "lanzar el 🚀 hacia 漢字",
		ttSeg("spec", "spec", boardcmd.SegHecho, "pasa el lint"), ttSeg("aprobacion", "aprobacion", boardcmd.SegFalta, "sin tu firma"))
	cohete.Actions = []boardcmd.Action{ttAccionAprobar()}
	kanji := ttCard("kanji", "A漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字", boardcmd.ColBacklog, "falta el spec 🚀", tv3aSegs()...)
	kanji.Actions = []boardcmd.Action{tv3aPedirArq()}
	m := tablero.Nuevo(tv3aCfg(80, 24, true, false))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(cohete, kanji)})
	ttForma(t, "CA-577", m, 80, 24, false)
	ttDorado(t, "CA-577", "v3-anchos-utf8-80x24.txt", m)
}
