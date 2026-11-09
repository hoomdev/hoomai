// Los archivos dorados de la v2 (spec tablero-con-paneles, "Como se
// prueba"; CA-550, CA-555, CA-576, CA-577): la pantalla entera a 120x32,
// 80x24 y 60x15 sin color y en ASCII, una vez con los signos UTF-8, y la de
// CA-577 con 🚀 y 漢字 en un titulo, comparada linea por linea con
// testdata/<nombre>.txt. Los dorados se escribieron a mano, panel por panel,
// desde doc.go ("The layout", "The panels", "[2] Tarjetas", "[0] of a
// card", "[3] Corriendo", "[4] Hallazgos", "[1] Proyecto", "The two last
// lines"); reemplazan los de la v1 (el cambio de pantalla que el spec
// aprueba). No hay flag -update: un dorado regenerado desde el codigo seria
// circular.
package tablero_test

import (
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// CA-550, CA-555, CA-576: 120x32 en ASCII: dos columnas, sin pliegues,
// [0] con el documento entero de nuc-tenancy. (CA-556, hallazgo 0ee5e4: los
// fixtures ya no se contradicen: la tira dice "pruebas: 1/2" como los dos
// criterios del spec, uno sin prueba, y la parte Hallazgos de [0] lista el
// hallazgo low de nuc-tenancy que [4] muestra.)
func TestCA550_CA555_CA576_DoradoASCII120x32(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, false))
	ttForma(t, "CA-550", m, 120, 32, false)
	ttDorado(t, "CA-550", "paneles-ascii-120x32.txt", m)
}

// CA-550, CA-555, CA-576: 80x24 en ASCII: dos columnas; [2] se desplaza y
// dice "1 de 7"; los textos que no entran se cortan con ~.
func TestCA550_CA555_CA576_DoradoASCII80x24(t *testing.T) {
	m := ttModeloDorado(ttConfig(80, 24, false))
	ttForma(t, "CA-550", m, 80, 24, false)
	ttDorado(t, "CA-550", "paneles-ascii-80x24.txt", m)
}

// CA-550, CA-576: 60x15 en ASCII: una columna, [1], [3], [4] y el Registro
// plegados, [2] con el foco en A/3 = 5 lineas.
func TestCA550_CA576_DoradoASCII60x15(t *testing.T) {
	m := ttModeloDorado(ttConfig(60, 15, false))
	ttForma(t, "CA-550", m, 60, 15, false)
	ttDorado(t, "CA-550", "paneles-ascii-60x15.txt", m)
}

// CA-550, CA-555, CA-576: 80x24 con los signos UTF-8 (las lineas dobles del
// foco, ▸, ✋ de dos celdas, ✓ ✗, el corte …, los acentos de la TUI).
func TestCA550_CA555_CA576_DoradoUTF8_80x24(t *testing.T) {
	m := ttModeloDorado(ttConfig(80, 24, true))
	ttForma(t, "CA-576", m, 80, 24, false)
	ttDorado(t, "CA-576", "paneles-utf8-80x24.txt", m)
}

// CA-577: una tarjeta con 🚀 y 漢字 en su titulo y su plain: el borde
// derecho de [0] no se corre (80x24 con UTF-8, sin otras lecturas que el
// tablero).
func TestCA577_DoradoAnchos(t *testing.T) {
	c := ttCard("cohete", "Despegue 🚀 del 漢字", boardcmd.ColBacklog, "lanzar el 🚀 hacia 漢字",
		ttSeg("spec", "spec", boardcmd.SegFalta, "no hay spec"), ttSeg("aprobacion", "aprobacion", boardcmd.SegFalta, "sin aprobar"))
	c.Actions = []boardcmd.Action{ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "Escribi el spec")}
	m := tablero.Nuevo(ttConfig(80, 24, true))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(c)})
	ttForma(t, "CA-577", m, 80, 24, false)
	ttDorado(t, "CA-577", "anchos-utf8-80x24.txt", m)
}
