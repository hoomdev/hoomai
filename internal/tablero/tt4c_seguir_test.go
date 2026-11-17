// Tests adversariales de la v3 de hoom tablero: Seguir solo (CA-715, CA-716)
// y la firma que encadena el piloto (CA-717, que re-expresa CA-574). Ningun
// test ejecuta nada: el modelo devuelve Lanzar y el test le da FinProceso con
// cada codigo y cada ultima linea (spec, "Como se prueba").
package tablero_test

import (
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/tablero"
)

const tv3cTextoSeguir = "El piloto lleva la tarjeta sola: tests, codigo, verify, review y rondas de hallazgos, " +
	"hasta tu aceptacion o hasta que algo lo frene. Nunca firma ni acepta."

// tv3cMsgSinAuto es el primer motivo de la tabla de Seguir solo.
func tv3cMsgSinAuto(slug string) string {
	return "el piloto no esta encendido en la tarjeta: hoom item set " + slug + " --auto hasta-humano"
}

func tv3cMsgColumna(col string) string {
	return "en " + ttNombreColumna(col) + " el piloto no tiene nada que hacer"
}

const (
	tv3cMsgTrabajo  = "la tarjeta ya tiene un trabajo en curso o cortado"
	tv3cMsgDetenida = "la tarjeta esta detenida: mira que fallo antes de seguir"
	tv3cMsgVivo     = "el piloto ya lleva la tarjeta"
)

// tv3cPNoAbre: P no abre nada, no ordena nada y dice msg.
func tv3cPNoAbre(t *testing.T, ca, que string, m *tablero.Modelo, msg string) {
	t.Helper()
	tv3cNoAbre(t, ca, "P "+que, m, "P", msg)
}

// tv3cPAbre: P abre Seguir solo sin ordenar nada; lo cierra con esc.
func tv3cPAbre(t *testing.T, ca, que string, m *tablero.Modelo) {
	t.Helper()
	tv3cAbre(t, ca, "P "+que, m, "P", tablero.ModalSeguir)
	ttSinOrdenes(t, ca, "esc en Seguir solo", ttTeclas(m, tablero.TeclaEsc))
	tv3cSinModal(t, ca, "esc en Seguir solo", m)
}

// ===========================================================================
// CA-715: cuando P abre y el primer motivo cuando no
// ===========================================================================

// CA-715: P abre Seguir solo en Test-writer, Writer y Review con auto, sin
// trabajo, sin rojo o con el rojo esperado.
func TestV3C_CA715_AbreEnLasColumnasDelPiloto(t *testing.T) {
	defer tv3cSinPanico(t)
	for _, col := range []string{boardcmd.ColTestWriter, boardcmd.ColWriter, boardcmd.ColReview} {
		c := tv3cSola("sola", "Sola", col)
		tv3cPAbre(t, "CA-715", "en "+col, tv3cModelo(tv3cConfig(200, 40), c))
		tv3cPAbre(t, "CA-715", "en "+col+" con el rojo esperado", tv3cModelo(tv3cConfig(200, 40), tv3cConRojo(c, true)))
		// en el detalle de la tarjeta tambien
		m := tv3cModelo(tv3cConfig(200, 40), c)
		ttTeclas(m, tablero.TeclaTab)
		tv3cPAbre(t, "CA-715", "en el detalle en "+col, m)
	}
}

// CA-715: sin auto, P no abre y dice el primer motivo, por DuracionAviso.
func TestV3C_CA715_SinAutoNoAbre(t *testing.T) {
	defer tv3cSinPanico(t)
	m := tv3cModelo(tv3cConfig(200, 40), tv3cSinAuto(tv3cSola("sola", "Sola", boardcmd.ColWriter)))
	tv3cPNoAbre(t, "CA-715", "sin auto", m, tv3cMsgSinAuto("sola"))
	m.Tic(ttAhora.Add(tablero.DuracionAviso - time.Second))
	ttDiceMensaje(t, "CA-715", "antes de DuracionAviso", m, false, tv3cMsgSinAuto("sola"))
	m.Tic(ttAhora.Add(tablero.DuracionAviso + time.Second))
	tv3cSinMensaje(t, "CA-715", "pasado DuracionAviso", m)
}

// CA-715: en Backlog, Arquitecto, Tu aprobacion, Tu aceptacion y Hecho el
// piloto no tiene nada que hacer.
func TestV3C_CA715_OtraColumnaNoAbre(t *testing.T) {
	defer tv3cSinPanico(t)
	for _, col := range []string{boardcmd.ColBacklog, boardcmd.ColArquitecto, boardcmd.ColTuAprobacion, boardcmd.ColTuAceptacion} {
		m := tv3cModelo(tv3cConfig(200, 40), tv3cSola("sola", "Sola", col))
		tv3cPNoAbre(t, "CA-715", "en "+col, m, tv3cMsgColumna(col))
	}
	m := tv3cModelo(tv3cConfig(200, 40), tv3cSola("sola", "Sola", boardcmd.ColHecho))
	ttTeclas(m, tablero.TeclaEnter, "j")
	if m.Elegida() != "sola" {
		t.Fatalf("CA-715: precondicion del spec: enter abre HECHO y j elige la tarjeta: elegida=%q", m.Elegida())
	}
	tv3cPNoAbre(t, "CA-715", "en Hecho", m, tv3cMsgColumna(boardcmd.ColHecho))
}

// CA-715: con running o interrupted, con un rojo no esperado y con un piloto
// de este tablero vivo, P no abre.
func TestV3C_CA715_TrabajoRojoYPilotoVivo(t *testing.T) {
	defer tv3cSinPanico(t)
	c := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	tv3cPNoAbre(t, "CA-715", "con running", tv3cModelo(tv3cConfig(200, 40), tv3cConRunning(c, "env-1", "writer")), tv3cMsgTrabajo)
	tv3cPNoAbre(t, "CA-715", "con interrupted", tv3cModelo(tv3cConfig(200, 40), tv3cConInterrupted(c, "writer")), tv3cMsgTrabajo)
	tv3cPNoAbre(t, "CA-715", "con rojo no esperado", tv3cModelo(tv3cConfig(200, 40), tv3cConRojo(c, false)), tv3cMsgDetenida)

	m := tv3cModelo(tv3cConfig(200, 40), c)
	tv3cLanzaSeguir(t, "CA-715", m, "sola")
	tv3cPNoAbre(t, "CA-715", "con el piloto vivo", m, tv3cMsgVivo)
	tv3cLeer(m, c)
	tv3cPNoAbre(t, "CA-715", "con el piloto vivo tras otra lectura", m, tv3cMsgVivo)
}

// CA-715: el mensaje es el del PRIMER motivo de la tabla.
func TestV3C_CA715_ElPrimerMotivo(t *testing.T) {
	defer tv3cSinPanico(t)
	ap := tv3cSola("sola", "Sola", boardcmd.ColTuAprobacion)
	w := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	casos := []struct {
		nombre string
		c      boardcmd.Card
		msg    string
	}{
		{"sin auto, en Tu aprobacion, con running", tv3cConRunning(tv3cSinAuto(ap), "env-1", "writer"), tv3cMsgSinAuto("sola")},
		{"sin auto, en Writer, con rojo", tv3cConRojo(tv3cSinAuto(w), false), tv3cMsgSinAuto("sola")},
		{"en Tu aprobacion con running y rojo", tv3cConRojo(tv3cConRunning(ap, "env-1", "writer"), false), tv3cMsgColumna(boardcmd.ColTuAprobacion)},
		{"en Writer con running y rojo", tv3cConRojo(tv3cConRunning(w, "env-1", "writer"), false), tv3cMsgTrabajo},
		{"en Writer con interrupted y rojo", tv3cConRojo(tv3cConInterrupted(w, "writer"), false), tv3cMsgTrabajo},
	}
	for _, c := range casos {
		tv3cPNoAbre(t, "CA-715", c.nombre, tv3cModelo(tv3cConfig(200, 40), c.c), c.msg)
	}

	// el piloto vivo va despues del trabajo y del rojo
	m := tv3cModelo(tv3cConfig(200, 40), w)
	tv3cLanzaSeguir(t, "CA-715", m, "sola")
	tv3cLeer(m, tv3cConRunning(w, "env-p", "writer"))
	tv3cPNoAbre(t, "CA-715", "con el piloto vivo y su running", m, tv3cMsgTrabajo)
	tv3cLeer(m, tv3cConRojo(w, false))
	tv3cPNoAbre(t, "CA-715", "con el piloto vivo y un rojo no esperado", m, tv3cMsgDetenida)
	tv3cLeer(m, tv3cConRojo(w, true))
	tv3cPNoAbre(t, "CA-715", "con el piloto vivo y el rojo esperado", m, tv3cMsgVivo)
}

// CA-715: el piloto que se detuvo sin 0 no se ofrece de nuevo mientras la
// tarjeta siga en la misma columna sin otro trabajo; el motivo sale de su
// ultima linea sin el prefijo, de la linea entera, o del codigo.
func TestV3C_CA715_LaParadaDelPilotoSinCero(t *testing.T) {
	defer tv3cSinPanico(t)
	w := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	for _, c := range []struct {
		code           int
		ultima, motivo string
	}{
		{1, tv3cPrefijoParada + "el veredicto quedo rojo en test", "el veredicto quedo rojo en test"},
		{2, "algo raro paso", "algo raro paso"},
		{3, "", "hoom piloto salio con 3"},
	} {
		m := tv3cModelo(tv3cConfig(200, 40), w)
		l := tv3cLanzaSeguir(t, "CA-715", m, "sola")
		tv3cFin(m, l.ID, c.code, c.ultima)
		tv3cLeer(m, w)
		msg := "el piloto se detuvo: " + c.motivo
		tv3cPNoAbre(t, "CA-715", "tras la parada con "+c.ultima, m, msg)
		tv3cLeer(m, w)
		tv3cPNoAbre(t, "CA-715", "tras la parada y otra lectura sin cambios", m, msg)
		if got := ttTeclas(m, tablero.TeclaEnter); len(got) != 0 || m.Modal() == tablero.ModalSeguir {
			t.Fatalf("CA-715 y CA-704: con la parada en pie Seguir solo no es el paso: enter dio %#v, modal %q", got, m.Modal())
		}
	}

	// con 0 no hay parada que valga
	m := tv3cModelo(tv3cConfig(200, 40), w)
	l := tv3cLanzaSeguir(t, "CA-715", m, "sola")
	tv3cFin(m, l.ID, 0, tv3cPrefijoParada+"la review pide una ronda que ya se hizo")
	tv3cLeer(m, w)
	tv3cPAbre(t, "CA-715", "tras terminar con 0", m)
}

// CA-715: la parada deja de valer cuando la tarjeta cambia de columna (y no
// vuelve a valer si regresa) o muestra un running con otro id; un running con
// el mismo id que tenia al detenerse no la levanta.
func TestV3C_CA715_LaParadaSeLevanta(t *testing.T) {
	defer tv3cSinPanico(t)
	w := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	rv := tv3cSola("sola", "Sola", boardcmd.ColReview)
	parada := func() (*tablero.Modelo, string) {
		m := tv3cModelo(tv3cConfig(200, 40), w)
		l := tv3cLanzaSeguir(t, "CA-715", m, "sola")
		tv3cFin(m, l.ID, 1, tv3cPrefijoParada+"tope de presupuesto")
		tv3cLeer(m, w)
		return m, "el piloto se detuvo: tope de presupuesto"
	}

	m, _ := parada()
	tv3cLeer(m, rv)
	tv3cPAbre(t, "CA-715", "tras pasar a Review", m)
	tv3cLeer(m, w)
	tv3cPAbre(t, "CA-715", "tras volver a Writer", m)

	m, msg := parada()
	tv3cLeer(m, tv3cConRunning(w, "env-otro", "writer"))
	tv3cPNoAbre(t, "CA-715", "con el otro trabajo corriendo", m, tv3cMsgTrabajo)
	tv3cLeer(m, w)
	tv3cPAbre(t, "CA-715", "despues de otro trabajo en la misma columna", m)

	// la parada se toma con el running que la tarjeta tenia entonces
	m = tv3cModelo(tv3cConfig(200, 40), w)
	l := tv3cLanzaSeguir(t, "CA-715", m, "sola")
	tv3cLeer(m, tv3cConRunning(w, "env-del-piloto", "writer"))
	tv3cFin(m, l.ID, 1, tv3cPrefijoParada+"tope de presupuesto")
	tv3cLeer(m, tv3cConRunning(w, "env-del-piloto", "writer"))
	tv3cLeer(m, w)
	tv3cPNoAbre(t, "CA-715", "el running que tenia al detenerse no es otro trabajo", m, msg)
}

// CA-715 y caso limite: sin tarjeta (la linea de HECHO) P no hace nada, y en
// el modal Acciones tampoco (decision de doc.go: Seguir solo no es una accion
// de la tarjeta).
func TestV3C_CA715_SinTarjetaYEnAccionesNoHaceNada(t *testing.T) {
	defer tv3cSinPanico(t)
	m := tv3cModelo(tv3cConfig(200, 40), ttViejo(1), ttViejo(2))
	if m.Elegida() != "" {
		t.Fatalf("CA-715: precondicion del spec: con solo HECHO cerrado no hay tarjeta elegida: %q", m.Elegida())
	}
	antes := tv3cFoto(m)
	ttSinOrdenes(t, "CA-715", "P sin tarjeta", ttTeclas(m, "P"))
	if tv3cFoto(m) != antes {
		t.Fatalf("CA-715: P sin tarjeta no hace nada:\n%s", tv3cFoto(m))
	}

	m = tv3cModelo(tv3cConfig(200, 40), tv3cSola("sola", "Sola", boardcmd.ColWriter))
	ttTeclas(m, tablero.TeclaEspacio)
	if m.Modal() != tablero.ModalAcciones {
		t.Fatalf("CA-715: precondicion del spec: espacio abre Acciones: %q", m.Modal())
	}
	antes = tv3cFoto(m)
	ttSinOrdenes(t, "CA-715", "P en Acciones", ttTeclas(m, "P"))
	if tv3cFoto(m) != antes {
		t.Fatalf("CA-715: P en Acciones no hace nada:\n%s", tv3cFoto(m))
	}
	mc := ttModal(t, "CA-715", m, 200)
	for _, l := range mc.Textos {
		if strings.Contains(l, "Seguir solo") {
			t.Fatalf("CA-715: Seguir solo no esta en Acciones: %q", mc.Textos)
		}
	}
}

// ===========================================================================
// CA-716: el modal Seguir solo
// ===========================================================================

// tv3cTextoModal es el texto del modal antes de su primera linea vacia,
// unido por espacios, y el resto de sus lineas.
func tv3cTextoModal(mc ttModalCaja) (string, []string) {
	for i, l := range mc.Textos {
		if l == "" {
			return strings.Join(mc.Textos[:i], " "), mc.Textos[i:]
		}
	}
	return strings.Join(mc.Textos, " "), nil
}

// CA-716: el modal dice que hace el piloto, "Se va a ejecutar:" y "hoom
// piloto <slug>", y los botones Seguir solo (elegido) y Cancelar; su titulo
// es "Seguir solo · <titulo>", o el slug con el titulo vacio.
func TestV3C_CA716_ElModalSeguirSolo(t *testing.T) {
	defer tv3cSinPanico(t)
	for _, ancho := range []int{200, 100, 70} {
		c := tv3cSola("modulo-usuarios", "Modulo de usuarios", boardcmd.ColWriter)
		m := tv3cModelo(tv3cConfig(ancho, 40), c)
		tv3cAbre(t, "CA-716", "P", m, "P", tablero.ModalSeguir)
		mc := ttModal(t, "CA-716", m, ancho)
		tv3cTitulo(t, "CA-716", m, ancho, "Seguir solo · Modulo de usuarios")
		texto, resto := tv3cTextoModal(mc)
		if texto != tv3cTextoSeguir {
			t.Fatalf("CA-716: [%d] el texto del modal, partido en los espacios, es\n%q\nfue\n%q", ancho, tv3cTextoSeguir, texto)
		}
		for _, l := range mc.Textos {
			if ttCeldas(l) > mc.Ancho-6 {
				t.Fatalf("CA-716: [%d] cada linea entra en Wm-6 celdas: %q", ancho, l)
			}
		}
		quiero := []string{"", "Se va a ejecutar:", "hoom piloto modulo-usuarios", ""}
		if len(resto) != len(quiero)+1 {
			t.Fatalf("CA-716: [%d] despues del texto van \"\", \"Se va a ejecutar:\", el comando, \"\" y los botones: %q", ancho, resto)
		}
		for i, q := range quiero {
			if resto[i] != q {
				t.Fatalf("CA-716: [%d] linea %d despues del texto: %q, fue %q (%q)", ancho, i, q, resto[i], resto)
			}
		}
		if got := strings.TrimSpace(resto[len(resto)-1]); got != "[ Seguir solo ]     Cancelar" {
			t.Fatalf("CA-716: [%d] los botones, Seguir solo elegido al abrir: %q", ancho, got)
		}
	}

	sinTitulo := tv3cSola("sin-titulo", "", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(200, 40), sinTitulo)
	tv3cAbre(t, "CA-716", "P con el titulo vacio", m, "P", tablero.ModalSeguir)
	tv3cTitulo(t, "CA-716", m, 200, "Seguir solo · sin-titulo")
}

// CA-716: solo enter sobre Seguir solo ordena Lanzar{Rol: "piloto", Args:
// {"piloto", <slug>}}, cierra el modal y suma la entrada del Registro;
// Cancelar, esc y las flechas no lanzan.
func TestV3C_CA716_SoloEnterSobreSeguirSoloLanza(t *testing.T) {
	defer tv3cSinPanico(t)
	c := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	for _, k := range [][]tablero.Tecla{
		{tablero.TeclaDer, tablero.TeclaEnter}, {tablero.TeclaIzq, tablero.TeclaEnter},
		{tablero.TeclaTab, tablero.TeclaEnter}, {tablero.TeclaShiftTab, tablero.TeclaEnter}, {tablero.TeclaEsc},
	} {
		m := tv3cModelo(tv3cConfig(200, 40), c)
		ttTeclas(m, "P")
		if len(k) > 1 {
			ttTeclas(m, k[0])
			mc := ttModal(t, "CA-716", m, 200)
			if got := strings.TrimSpace(mc.Textos[len(mc.Textos)-1]); got != "Seguir solo     [ Cancelar ]" {
				t.Fatalf("CA-716: %q elige Cancelar: %q", k[0], got)
			}
		}
		ords := ttTeclas(m, k[len(k)-1])
		ttSinOrdenes(t, "CA-716", "Cancelar o esc", ords)
		tv3cSinModal(t, "CA-716", "Cancelar o esc", m)
		if len(m.Registro()) != 0 {
			t.Fatalf("CA-716: Cancelar no suma entradas al Registro: %+v", m.Registro())
		}
	}

	// dos veces la flecha vuelve a Seguir solo
	m := tv3cModelo(tv3cConfig(200, 40), c)
	ttTeclas(m, "P", tablero.TeclaDer, tablero.TeclaDer)
	l := ttUnaLanzar(t, "CA-716", "enter tras volver a Seguir solo", ttTeclas(m, tablero.TeclaEnter))
	ttMismosArgs(t, "CA-716", "Seguir solo", l.Args, []string{"piloto", "sola"})

	m = tv3cModelo(tv3cConfig(200, 40), c)
	l = tv3cLanzaSeguir(t, "CA-716", m, "sola")
	if l.ID < 1 {
		t.Fatalf("CA-716: el Lanzar lleva el numero del modelo, desde 1: %+v", l)
	}
	tv3cSinModal(t, "CA-716", "enter sobre Seguir solo cierra el modal", m)
	a := tv3cAsiento(t, "CA-716", m, "Seguir solo")
	if a.Texto != "Seguir solo · sola · lanzando..." || a.Comando != "hoom piloto sola" {
		t.Fatalf("CA-716: la entrada del Registro es {Seguir solo · sola · lanzando..., hoom piloto sola}: %+v", a)
	}
	if !a.Hora.Equal(ttAhora) {
		t.Fatalf("CA-716: la hora de la entrada es la del reloj del modelo: %v", a.Hora)
	}
}

// CA-716: el estado de la entrada pasa por lanzando..., corriendo (cuando la
// tarjeta muestra un running con un sobre nuevo) y, al terminar, "se detuvo:
// <resto>" (verde con 0, rojo con otro) o la regla de la v2; su ultima linea
// va a la linea de mensajes.
func TestV3C_CA716_ElEstadoDeLaEntrada(t *testing.T) {
	defer tv3cSinPanico(t)
	c := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	// colorReg es el color del estado en el Registro plegado ("" = el
	// contrato no lo fija: hecho).
	casos := []struct {
		code                    int
		ultima, final, colorReg string
		verde                   bool
	}{
		{0, tv3cPrefijoParada + "lista para tu aceptacion", "se detuvo: lista para tu aceptacion", "verde", true},
		{1, tv3cPrefijoParada + "el veredicto quedo rojo", "se detuvo: el veredicto quedo rojo", "rojo", false},
		{0, "todo bien", "hecho", "", true},
		{1, "explota", "fallo: explota", "rojo", false},
		{2, "", "fallo: hoom piloto salio con 2", "rojo", false},
	}
	for _, k := range casos {
		m := tv3cModelo(tv3cConfigColor(200, 40), c)
		l := tv3cLanzaSeguir(t, "CA-716", m, "sola")

		tv3cLeer(m, c)
		if a := tv3cAsiento(t, "CA-716", m, "Seguir solo"); a.Texto != "Seguir solo · sola · lanzando..." {
			t.Fatalf("CA-716: sin running nuevo sigue lanzando...: %q", a.Texto)
		}
		tv3cLeer(m, tv3cConRunning(c, "env-del-piloto", "writer"))
		if a := tv3cAsiento(t, "CA-716", m, "Seguir solo"); a.Texto != "Seguir solo · sola · corriendo" {
			t.Fatalf("CA-716: con el running de un sobre nuevo dice corriendo: %q", a.Texto)
		}
		// termina mientras la tarjeta todavia muestra el running: ninguna
		// entrada de fin de trabajo se suma antes que la del piloto
		ttSinOrdenes(t, "CA-716", "el fin del piloto", tv3cFin(m, l.ID, k.code, k.ultima))
		a := tv3cAsiento(t, "CA-716", m, "Seguir solo")
		if a.Texto != "Seguir solo · sola · "+k.final || a.Comando != "hoom piloto sola" {
			t.Fatalf("CA-716: [%d %q] al terminar la entrada dice %q: %+v", k.code, k.ultima, k.final, a)
		}
		if n := len(m.Registro()); n != 1 || m.Registro()[0].Texto != a.Texto {
			t.Fatalf("CA-716: la entrada cambia en su lugar: %+v", m.Registro())
		}
		raw := ttLinea(m, 38)
		if !strings.Contains(ttSinSGR(raw), "Seguir solo · sola · "+k.final) {
			t.Fatalf("CA-716: el Registro plegado muestra la entrada mas nueva: %q", ttSinSGR(raw))
		}
		if k.colorReg != "" {
			re := ttRojo
			if k.colorReg == "verde" {
				re = ttVerde
			}
			if !ttConColor(re, raw, k.final) {
				t.Fatalf("CA-716: [%d %q] el Registro plegado dice %q en %s: %q", k.code, k.ultima, k.final, k.colorReg, raw)
			}
		}
		if k.ultima != "" {
			ttDiceMensaje(t, "CA-716", "el fin del piloto", m, !k.verde, k.ultima)
			if k.verde && !ttConColor(ttVerde, ttMensajeCrudo(m), k.ultima) {
				t.Fatalf("CA-716: con 0 la ultima linea va en verde: %q", ttMensajeCrudo(m))
			}
		}
	}
}

// CA-716: mientras el proceso del piloto vive la tarjeta esta en CORRIENDO,
// con o sin running; el fin de otro proceso no lo termina.
func TestV3C_CA716_EnCorriendoMientrasViveElPiloto(t *testing.T) {
	defer tv3cSinPanico(t)
	c := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(200, 40), c)
	if m.Grupo("sola") != tablero.GrupoTeToca {
		t.Fatalf("CA-716: precondicion del spec: antes del piloto la tarjeta esta en TE TOCA: %q", m.Grupo("sola"))
	}
	l := tv3cLanzaSeguir(t, "CA-716", m, "sola")
	if m.Grupo("sola") != tablero.GrupoCorriendo || m.Corriendo() != 1 {
		t.Fatalf("CA-716: desde el Lanzar la tarjeta esta en CORRIENDO: grupo=%q corriendo=%d", m.Grupo("sola"), m.Corriendo())
	}
	tv3cLeer(m, c)
	if m.Grupo("sola") != tablero.GrupoCorriendo {
		t.Fatalf("CA-716: sin running, con el piloto vivo, sigue en CORRIENDO: %q", m.Grupo("sola"))
	}
	ttSinOrdenes(t, "CA-716", "el fin de un proceso que no existe", tv3cFin(m, l.ID+100, 0, ""))
	if m.Grupo("sola") != tablero.GrupoCorriendo || m.Corriendo() != 1 {
		t.Fatalf("CA-716: el fin de otro id no termina el piloto: grupo=%q corriendo=%d", m.Grupo("sola"), m.Corriendo())
	}
	tv3cFin(m, l.ID, 1, tv3cPrefijoParada+"algo lo freno")
	if m.Grupo("sola") != tablero.GrupoTeToca || m.Corriendo() != 0 {
		t.Fatalf("CA-716: con su FinProceso sale de CORRIENDO: grupo=%q corriendo=%d", m.Grupo("sola"), m.Corriendo())
	}
}

// CA-716: el modal esta atado a lo que muestra: en cada lectura Seguir solo
// tiene que seguir disponible, y si no lo esta se cierra y la linea de
// mensajes dice en rojo el motivo; una lectura sin cambios no lo cierra.
func TestV3C_CA716_AtadoALaDisponibilidad(t *testing.T) {
	defer tv3cSinPanico(t)
	c := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	casos := []struct {
		nombre string
		nueva  boardcmd.Card
		msg    string
	}{
		{"con running", tv3cConRunning(c, "env-x", "writer"), tv3cMsgTrabajo},
		{"sin auto", tv3cSinAuto(c), tv3cMsgSinAuto("sola")},
		{"en Tu aceptacion", tv3cSola("sola", "Sola", boardcmd.ColTuAceptacion), tv3cMsgColumna(boardcmd.ColTuAceptacion)},
		{"con rojo no esperado", tv3cConRojo(c, false), tv3cMsgDetenida},
		{"con interrupted", tv3cConInterrupted(c, "writer"), tv3cMsgTrabajo},
	}
	for _, k := range casos {
		m := tv3cModelo(tv3cConfigColor(200, 40), c)
		ttTeclas(m, "P")
		if m.Modal() != tablero.ModalSeguir {
			t.Fatalf("CA-716: precondicion del spec: P abre Seguir solo: %q", m.Modal())
		}
		ttSinOrdenes(t, "CA-716", "la lectura que lo cierra", tv3cLeer(m, k.nueva))
		tv3cSinModal(t, "CA-716", "la lectura "+k.nombre, m)
		ttDiceMensaje(t, "CA-716", "la lectura "+k.nombre, m, true, k.msg)
	}
	for _, k := range []struct {
		nombre string
		nueva  boardcmd.Card
	}{{"sin cambios", c}, {"con el rojo esperado", tv3cConRojo(c, true)}, {"en Review", tv3cSola("sola", "Sola", boardcmd.ColReview)}} {
		m := tv3cModelo(tv3cConfig(200, 40), c)
		ttTeclas(m, "P")
		tv3cLeer(m, k.nueva)
		if m.Modal() != tablero.ModalSeguir {
			t.Fatalf("CA-716: una lectura %s deja Seguir solo disponible: el modal sigue abierto, es %q", k.nombre, m.Modal())
		}
	}
	// la tarjeta que desaparece (CA-578)
	m := tv3cModelo(tv3cConfig(200, 40), c, ttNucTenancy())
	if m.Elegida() != "sola" {
		t.Fatalf("CA-716: precondicion del spec: la primera de TE TOCA es la de Writer: elegida=%q", m.Elegida())
	}
	ttTeclas(m, "P")
	tv3cLeer(m, ttNucTenancy())
	tv3cSinModal(t, "CA-716", "la tarjeta que desaparece", m)
	ttDiceMensaje(t, "CA-716", "la tarjeta que desaparece", m, false, "la tarjeta sola ya no existe")
}

// ===========================================================================
// CA-717: la firma encadena el piloto
// ===========================================================================

// tv3cNucAuto es nuc-tenancy con el piloto encendido.
func tv3cNucAuto() boardcmd.Card {
	c := ttNucTenancy()
	c.Item.Auto = item.AutoHastaHumano
	return c
}

// tv3cLineasFirma son las lineas del modal desde "Se va a ejecutar:".
func tv3cLineasFirma(t *testing.T, ca string, m *tablero.Modelo, ancho int) []string {
	t.Helper()
	mc := ttModal(t, ca, m, ancho)
	for i, l := range mc.Textos {
		if strings.HasPrefix(l, "Se va a ejecutar:") {
			return mc.Textos[i:]
		}
	}
	t.Fatalf("%s: el modal de firma dice \"Se va a ejecutar:\": %q", ca, mc.Textos)
	return nil
}

// CA-717 y CA-574: con auto, el modal de firma de A (y el de enter y n, cuyo
// paso es aprobar) suma "y despues: hoom piloto <slug>" debajo del comando,
// con Cancelar elegido al abrir.
func TestV3C_CA717_CA574_LaFirmaConAutoSumaElPiloto(t *testing.T) {
	defer tv3cSinPanico(t)
	for _, k := range []tablero.Tecla{"A", tablero.TeclaEnter, "n"} {
		m := tv3cModelo(tv3cConfig(200, 40), tv3cNucAuto())
		tv3cAbre(t, "CA-717", string(k)+" con auto", m, k, tablero.ModalFirma)
		ls := tv3cLineasFirma(t, "CA-717", m, 200)
		quiero := []string{"Se va a ejecutar:", "hoom aprobar nuc-tenancy --si", "y despues: hoom piloto nuc-tenancy", ""}
		if len(ls) != len(quiero)+1 {
			t.Fatalf("CA-717: [%s] desde \"Se va a ejecutar:\" van %q y los botones: %q", k, quiero, ls)
		}
		for i, q := range quiero {
			if ls[i] != q {
				t.Fatalf("CA-717: [%s] linea %d: %q, fue %q (%q)", k, i, q, ls[i], ls)
			}
		}
		if got := strings.TrimSpace(ls[len(ls)-1]); got != "[ Cancelar ]     Aprobar spec" {
			t.Fatalf("CA-717: [%s] Cancelar elegido al abrir: %q", k, got)
		}
	}
}

// CA-717 y CA-574: sin auto el modal de firma es el de la v2, y el de
// integrar no suma el piloto aunque tenga auto.
func TestV3C_CA717_CA574_SinAutoEsLaFirmaDeLaV2(t *testing.T) {
	defer tv3cSinPanico(t)
	for _, c := range []struct {
		nombre, comando string
		card            boardcmd.Card
		tecla           tablero.Tecla
	}{
		{"aprobar sin auto", "hoom aprobar nuc-tenancy --si", ttNucTenancy(), "A"},
		{"integrar con auto", "hoom integrar acep --si", tv3cSola("acep", "Acep", boardcmd.ColTuAceptacion), "I"},
	} {
		m := tv3cModelo(tv3cConfig(200, 40), c.card)
		tv3cAbre(t, "CA-717", c.nombre, m, c.tecla, tablero.ModalFirma)
		ls := tv3cLineasFirma(t, "CA-717", m, 200)
		if len(ls) != 4 || ls[1] != c.comando || ls[2] != "" {
			t.Fatalf("CA-717: [%s] el modal de la v2: \"Se va a ejecutar:\", %q, \"\" y los botones: %q", c.nombre, c.comando, ls)
		}
		for _, l := range ttModal(t, "CA-717", m, 200).Textos {
			if strings.Contains(l, "hoom piloto") {
				t.Fatalf("CA-717: [%s] sin la linea del piloto: %q", c.nombre, l)
			}
		}
	}
}

// tv3cAprueba abre la firma de nuc-tenancy, elige el boton de la accion y lo
// confirma: devuelve el Lanzar de hoom aprobar.
func tv3cAprueba(t *testing.T, m *tablero.Modelo) tablero.Lanzar {
	t.Helper()
	ttTeclas(m, "A", tablero.TeclaDer)
	l := ttUnaLanzar(t, "CA-717", "enter sobre Aprobar spec", ttTeclas(m, tablero.TeclaEnter))
	ttMismosArgs(t, "CA-717", "aprobar", l.Args, []string{"aprobar", "nuc-tenancy", "--si"})
	if l.Rol != "" {
		t.Fatalf("CA-717: hoom aprobar no lanza un rol: %+v", l)
	}
	return l
}

// CA-717 y CA-574: si hoom aprobar termina con 0 el tablero lanza hoom piloto
// con su propia entrada del Registro; con otro codigo no lo lanza.
func TestV3C_CA717_CA574_AprobarConCeroLanzaElPiloto(t *testing.T) {
	defer tv3cSinPanico(t)
	m := tv3cModelo(tv3cConfig(200, 40), tv3cNucAuto())
	ap := tv3cAprueba(t, m)
	if a := tv3cAsiento(t, "CA-717", m, "Aprobar spec"); a.Texto != "Aprobar spec · nuc-tenancy · lanzando..." {
		t.Fatalf("CA-717: la entrada de la firma: %+v", a)
	}
	ords := tv3cFin(m, ap.ID, 0, "spec aprobado")
	p := ttUnaLanzar(t, "CA-717", "el fin con 0 de hoom aprobar con el piloto", ords)
	if p.Rol != "piloto" || p.Slug != "nuc-tenancy" || p.ID == ap.ID || p.ID < 1 {
		t.Fatalf("CA-717: el Lanzar del piloto es {Rol: piloto, Slug: nuc-tenancy} con su propio numero: %+v (aprobar %d)", p, ap.ID)
	}
	ttMismosArgs(t, "CA-717", "el piloto", p.Args, []string{"piloto", "nuc-tenancy"})
	if a := tv3cAsiento(t, "CA-717", m, "Aprobar spec"); a.Texto != "Aprobar spec · nuc-tenancy · hecho" {
		t.Fatalf("CA-717: la firma termino: %+v", a)
	}
	s := tv3cAsiento(t, "CA-717", m, "Seguir solo")
	if s.Texto != "Seguir solo · nuc-tenancy · lanzando..." || s.Comando != "hoom piloto nuc-tenancy" {
		t.Fatalf("CA-717: el piloto tiene su propia entrada: %+v", s)
	}
	if m.Grupo("nuc-tenancy") != tablero.GrupoCorriendo {
		t.Fatalf("CA-717: con su piloto vivo la tarjeta esta en CORRIENDO: %q", m.Grupo("nuc-tenancy"))
	}

	for _, code := range []int{1, 2, -1} {
		m := tv3cModelo(tv3cConfig(200, 40), tv3cNucAuto())
		ap := tv3cAprueba(t, m)
		ttSinOrdenes(t, "CA-717", "el fin sin 0 de hoom aprobar", tv3cFin(m, ap.ID, code, "no se pudo firmar"))
		if n := tv3cCuentaAsientos(m, "Seguir solo"); n != 0 {
			t.Fatalf("CA-717: sin 0 no hay entrada de Seguir solo: %+v", m.Registro())
		}
		if a := tv3cAsiento(t, "CA-717", m, "Aprobar spec"); a.Texto != "Aprobar spec · nuc-tenancy · fallo: no se pudo firmar" {
			t.Fatalf("CA-717: el Registro muestra el fallo de la firma: %+v", a)
		}
	}
}

// CA-717 y CA-574: sin auto, o con integrar, el fin con 0 no lanza nada; y el
// piloto se encadena por lo que dijo el modal al firmar (doc.go, Termino),
// aunque una lectura de despues ya no traiga auto.
func TestV3C_CA717_CA574_QuienEncadenaElPiloto(t *testing.T) {
	defer tv3cSinPanico(t)
	m := tv3cModelo(tv3cConfig(200, 40), ttNucTenancy())
	ap := tv3cAprueba(t, m)
	ttSinOrdenes(t, "CA-717", "el fin con 0 de aprobar sin auto", tv3cFin(m, ap.ID, 0, ""))

	m = tv3cModelo(tv3cConfig(200, 40), tv3cSola("acep", "Acep", boardcmd.ColTuAceptacion))
	ttTeclas(m, "I", tablero.TeclaDer)
	in := ttUnaLanzar(t, "CA-717", "enter sobre Integrar", ttTeclas(m, tablero.TeclaEnter))
	ttSinOrdenes(t, "CA-717", "el fin con 0 de integrar con auto", tv3cFin(m, in.ID, 0, ""))

	m = tv3cModelo(tv3cConfig(200, 40), tv3cNucAuto())
	ap = tv3cAprueba(t, m)
	tv3cLeer(m, ttNucTenancy())
	p := ttUnaLanzar(t, "CA-717", "el fin con 0 de un aprobar cuyo modal dijo el piloto", tv3cFin(m, ap.ID, 0, ""))
	ttMismosArgs(t, "CA-717", "el piloto", p.Args, []string{"piloto", "nuc-tenancy"})
}

// CA-717: el modal de firma esta atado tambien al auto de la tarjeta: si
// cambia (en cualquier sentido) se cierra con "cambio lo que ibas a confirmar
// en <slug>: vuelve a abrirlo" en rojo, sin firmar.
func TestV3C_CA717_CambiarElAutoCierraLaFirma(t *testing.T) {
	defer tv3cSinPanico(t)
	for _, c := range []struct {
		nombre      string
		antes, dsps boardcmd.Card
	}{
		{"se apaga el piloto", tv3cNucAuto(), ttNucTenancy()},
		{"se enciende el piloto", ttNucTenancy(), tv3cNucAuto()},
	} {
		m := tv3cModelo(tv3cConfigColor(200, 40), c.antes)
		ttTeclas(m, "A")
		if m.Modal() != tablero.ModalFirma {
			t.Fatalf("CA-717: precondicion del spec: A abre la firma: %q", m.Modal())
		}
		ttSinOrdenes(t, "CA-717", "la lectura", tv3cLeer(m, c.dsps))
		tv3cSinModal(t, "CA-717", c.nombre, m)
		ttDiceMensaje(t, "CA-717", c.nombre, m, true, "cambio lo que ibas a confirmar en nuc-tenancy: vuelve a abrirlo")
	}
	m := tv3cModelo(tv3cConfig(200, 40), tv3cNucAuto())
	ttTeclas(m, "A")
	tv3cLeer(m, tv3cNucAuto())
	if m.Modal() != tablero.ModalFirma {
		t.Fatalf("CA-717: una lectura sin cambios no cierra la firma: %q", m.Modal())
	}
}
