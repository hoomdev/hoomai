// Tests del analizador compartido (hallazgo 1c0773): la matriz de frases que
// DiceQueLaCerro reconoce y rechaza. Con la ENMIENDA 6 de
// .hoom/specs/acciones-desde-la-tarjeta.md (hallazgo bde7e4), Abrir sesion no
// cierra la sesion que creo cuando despues falla el split-window o el registro
// en el item (CA-347, CA-348), y su error no puede decir que la cerro: lo
// exigen los incumplimientosEnmienda6 de internal/cockpitcmd e
// internal/servecmd con DiceQueLaCerro, y cada uno prueba su verificador en
// TestCA347_CA348_HallazgoBde7e4_ElVerificadorDeLaEnmienda6. Sin esto, esos
// tests podrian pasar o fallar por el analizador y no por el mensaje.
//
// Cada frase corre con los dos pares (error original, sesion) de los
// consumidores: el de cockpitcmd ("split-window: fallo", h3Sesion) y el de
// servecmd ("exit status 1", cockpitcmd.SessionName("demo", "fix")).
package cierretmuxtest_test

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/cierretmuxtest"
)

// vcPares son los (error original, sesion) con los que corre cada frase.
var vcPares = []struct{ orig, sesion string }{
	{"split-window: fallo", "hoom-demo-facturas"},
	{"exit status 1", "hoom-demo-fix"},
}

// vcArmar reemplaza en frase {o} por orig, {s} por sesion y {cmd} por el
// comando de cierre con el destino entre comillas.
func vcArmar(frase, orig, sesion string) string {
	return strings.NewReplacer(
		"{o}", orig,
		"{s}", sesion,
		"{cmd}", "tmux kill-session -t '="+sesion+"'",
	).Replace(frase)
}

// CA-347, CA-348: DiceQueLaCerro reconoce que la apertura cerro la sesion
// (con o sin tildes) y no lo ve en una negacion, en un "no pude cerrar" ni en
// los errores que la enmienda 6 permite: el que dice como cerrarla a mano,
// como el de un new-session fallido, y el que niega haberla cerrado.
func TestCA347_CA348_DiceQueLaCerro(t *testing.T) {
	for _, c := range []struct {
		frase string
		cerro bool
	}{
		{"{o}; cerré la sesión {s} para que el próximo intento la arme entera", true},
		{"{o}; cerre la sesion {s} para que el proximo intento la arme entera", true},
		{"{o}; la cerre: reintenta", true},
		{"{o}; no la cerré: {cmd}", false},
		{"{o}; no pude cerrar la sesion {s}", false},
		{"{o}; si la sesion {s} quedo a medias (sin el pane de vigilancia), cerrala y reintenta: {cmd}", false},
		{"{o}; no cerre la sesion {s}: si sigue abierta, cerrala a mano con {cmd} antes de reintentar", false},
		{"{o}; no cerré la sesión {s}: si quedó abierta, cerrala con {cmd}", false},
		{"{o}; la sesion {s} no se cerro: cerrala a mano con {cmd}", false},
	} {
		for _, p := range vcPares {
			msg := vcArmar(c.frase, p.orig, p.sesion)
			if got := cierretmuxtest.DiceQueLaCerro(msg); got != c.cerro {
				t.Errorf("DiceQueLaCerro(%q) = %v, quiere %v", msg, got, c.cerro)
			}
		}
	}
}
