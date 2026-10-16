// Tests del verificador compartido (hallazgo 1c0773): la matriz COMUN de
// frases que aceptan y rechazan IncumplimientosDelCierreFallido,
// DiceQueLaCerro y DiceQueNoSabe, el contrato de los hallazgos 9c1816, 57bd29
// y 03481c (decision de Henry) y el de C1 sobre el error de Abrir sesion de
// .hoom/specs/acciones-desde-la-tarjeta.md (CA-347, CA-348). Sin esto, los
// tests de internal/cockpitcmd e internal/servecmd que usan el verificador
// podrian pasar o fallar por el verificador y no por el mensaje.
//
// Lo propio de cada consumidor lo prueba su test del verificador
// (TestCA347_Hallazgo9c1816_ElVerificadorDelMensaje): en cockpitcmd, que el
// error conserve el del paso que fallo y que el comando se busque tal cual
// (strings.Contains); en servecmd, que se busque con el nombre exacto de la
// sesion (contieneElAviso). Aca el comando se busca con strings.Contains.
//
// Cada frase corre con los dos pares (error original, sesion) que usaban las
// matrices de los consumidores: el de cockpitcmd ("split-window: fallo",
// h3Sesion) y el de servecmd ("exit status 1", cockpitcmd.SessionName("demo",
// "fix")), asi cada caso que tenian sigue corriendo tal cual.
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

// Guarda del verificador (9c1816, CA-347, CA-348):
// IncumplimientosDelCierreFallido no devuelve nada para los mensajes que
// cumplen el contrato nuevo aunque lo digan con otras palabras (un
// condicional, una disyuncion, con o sin tildes), y devuelve incumplimientos
// para los del contrato de tres estados, los que dicen que la cerro y los que
// no muestran el comando con comillas.
func TestCA347_CA348_Hallazgo9c1816_LaMatrizDelVerificador(t *testing.T) {
	for _, c := range []struct {
		frase, porque string
		vale          bool
	}{
		{"{o}; no pude cerrar la sesion {s} (exit status 1) y no pude saber si sigue abierta: si sigue, cerrala y reintenta: {cmd}", "dice que no pudo saber y muestra el comando", true},
		{"{o}; no pude cerrar la sesión {s} y no sé si quedó abierta: {cmd}", "\"no se si\" con tildes", true},
		{"{o}; no pude saber si la sesion {s} sigue abierta o ya no existe; si sigue, {cmd}", "\"ya no existe\" como segundo miembro de una disyuncion condicional", true},
		{"{o}; no pude saber si la sesión {s} sigue abierta o ya no existe; si sigue, {cmd}", "la misma disyuncion con tilde en sesion", true},
		{"{o}; no pude cerrarla: puede que siga abierta ({cmd})", "\"puede que siga\", con el comando entre parentesis", true},
		{"{o}; la sesion {s} ya no existe: reintenta", "afirma que ya no existe y no muestra el comando (estado A)", false},
		{"{o}; la sesion {s} quedo abierta y no pude cerrarla (exit status 1): cerrala y reintenta: {cmd}", "afirma que quedo abierta (estado B)", false},
		{"{o}; la sesión {s} quedó abierta, no sé si podrás cerrarla: {cmd}", "afirma que quedo abierta aunque despues diga \"no se si\"", false},
		{"{o}; no pude saber si sigue abierta: cerrala con tmux kill-session -t ={s}", "el comando sin comillas", false},
		{"{o}; no pude saber si sigue abierta: tmux kill-session -t ={s}", "el comando sin comillas, solo", false},
		{"{o}; no pude cerrar la sesion {s}: {cmd}", "no dice que no pudo saber si sigue abierta", false},
		{"{o}; cerre la sesion {s}, pero no pude saber si sigue abierta: {cmd}", "dice que la cerro y kill-session fallo", false},
	} {
		for _, p := range vcPares {
			msg := vcArmar(c.frase, p.orig, p.sesion)
			if mal := cierretmuxtest.IncumplimientosDelCierreFallido(msg, p.sesion, strings.Contains); (len(mal) == 0) != c.vale {
				t.Errorf("9c1816 (%s): IncumplimientosDelCierreFallido(%q) = %q, quiere que valga = %v", c.porque, msg, mal, c.vale)
			}
		}
	}
}

// El contrato de retorno de IncumplimientosDelCierreFallido (hallazgo bbadcb):
// no es un predicado, devuelve la lista de incumplimientos; vacia si el
// mensaje cumple todo, y con uno por regla rota, en el orden de su doc (no
// dice que la cerro, no afirma un hecho, dice que no pudo saber, muestra el
// comando con comillas), si las rompe todas.
func TestCA347_CA348_Hallazgo9c1816_LosIncumplimientosVanUnoPorRegla(t *testing.T) {
	for _, p := range vcPares {
		bien := vcArmar("{o}; no pude cerrar la sesion {s} (exit status 1) y no pude saber si sigue abierta: si sigue, cerrala y reintenta: {cmd}", p.orig, p.sesion)
		if mal := cierretmuxtest.IncumplimientosDelCierreFallido(bien, p.sesion, strings.Contains); len(mal) != 0 {
			t.Errorf("bbadcb: el mensaje que cumple todo no tiene incumplimientos: IncumplimientosDelCierreFallido(%q) = %q", bien, mal)
		}
		todo := vcArmar("{o}; la cerre; la sesion {s} ya no existe", p.orig, p.sesion)
		mal := cierretmuxtest.IncumplimientosDelCierreFallido(todo, p.sesion, strings.Contains)
		claves := []string{"la cerro", "ya no existe", "no se pudo saber", "tmux kill-session -t '=" + p.sesion + "'"}
		if len(mal) != len(claves) {
			t.Fatalf("bbadcb: el mensaje que rompe las cuatro reglas tiene un incumplimiento por regla: IncumplimientosDelCierreFallido(%q) = %q", todo, mal)
		}
		for i, clave := range claves {
			if !strings.Contains(mal[i], clave) {
				t.Errorf("bbadcb: el incumplimiento %d habla de %q (en el orden de la doc): %q", i, clave, mal)
			}
		}
	}
}

// Guarda del contrato de C1 (CA-347, CA-348): DiceQueLaCerro reconoce que la
// apertura cerro la sesion (con o sin tildes) y no una negacion ni un "no
// pude cerrar".
func TestCA347_CA348_DiceQueLaCerro(t *testing.T) {
	for _, c := range []struct {
		frase string
		cerro bool
	}{
		{"{o}; cerré la sesión {s} para que el próximo intento la arme entera", true},
		{"{o}; la cerre: reintenta", true},
		{"{o}; no la cerré: {cmd}", false},
		{"{o}; no pude cerrar la sesion {s}", false},
	} {
		for _, p := range vcPares {
			msg := vcArmar(c.frase, p.orig, p.sesion)
			if got := cierretmuxtest.DiceQueLaCerro(msg); got != c.cerro {
				t.Errorf("C1: DiceQueLaCerro(%q) = %v, quiere %v", msg, got, c.cerro)
			}
		}
	}
}

// DiceQueNoSabe (CA-347, CA-348) reconoce, con o sin tildes, que el error
// dice que no se pudo saber si la sesion sigue abierta, y no lo ve en el
// error de C1 que dice que la cerro ni en uno que afirma que ya no existe.
func TestCA347_CA348_DiceQueNoSabe(t *testing.T) {
	for _, c := range []struct {
		frase  string
		noSabe bool
	}{
		{"{o}; no pude cerrar la sesión {s} y no sé si quedó abierta: {cmd}", true},
		{"{o}; no pude cerrar la sesion {s} y no se si quedo abierta: {cmd}", true},
		{"{o}; cerré la sesión {s} para que el próximo intento la arme entera", false},
		{"{o}; la sesion {s} ya no existe: reintenta", false},
	} {
		for _, p := range vcPares {
			msg := vcArmar(c.frase, p.orig, p.sesion)
			if got := cierretmuxtest.DiceQueNoSabe(msg); got != c.noSabe {
				t.Errorf("DiceQueNoSabe(%q) = %v, quiere %v", msg, got, c.noSabe)
			}
		}
	}
}
