// Tests adversariales de la v3 de hoom tablero: el paso de una tarjeta
// (CA-704) y lo que abren enter y n (CA-705, que re-expresa CA-566). Desde el
// spec .hoom/specs/tablero-tu-siguiente-paso.md y el contrato de doc.go ("The
// step of a card", "The box TU SIGUIENTE PASO").
package tablero_test

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// tv3cAbre exige que la tecla abra el modal quiero sin ordenar nada.
func tv3cAbre(t *testing.T, ca, que string, m *tablero.Modelo, k tablero.Tecla, quiero tablero.Modal) {
	t.Helper()
	ords := ttTeclas(m, k)
	ttSinOrdenes(t, ca, que, ords)
	if m.Modal() != quiero {
		t.Fatalf("%s: %s abre el modal %q; abrio %q (mensaje %q)", ca, que, quiero, m.Modal(), ttMensaje(m))
	}
}

// tv3cNoAbre exige que la tecla no abra nada, no ordene nada y diga msg.
func tv3cNoAbre(t *testing.T, ca, que string, m *tablero.Modelo, k tablero.Tecla, msg string) {
	t.Helper()
	ords := ttTeclas(m, k)
	ttSinOrdenes(t, ca, que, ords)
	tv3cSinModal(t, ca, que, m)
	ttDiceMensaje(t, ca, que, m, false, msg)
}

// tv3cTitulo exige que el modal abierto tenga ese titulo.
func tv3cTitulo(t *testing.T, ca string, m *tablero.Modelo, ancho int, titulo string) {
	t.Helper()
	mc := ttModal(t, ca, m, ancho)
	if !strings.Contains(mc.Arriba, " "+titulo+" ") {
		t.Fatalf("%s: el modal se titula %q: %q", ca, titulo, mc.Arriba)
	}
}

// CA-704: cuando Seguir solo esta disponible (auto: hasta-humano, en
// Test-writer, Writer o Review, sin trabajo, sin rojo no esperado, sin piloto
// vivo) es el paso de la tarjeta: enter abre el modal Seguir solo, tambien
// antes que la accion de rol de la columna y con el rojo esperado del
// test-writer.
func TestV3C_CA704_SeguirSoloEsElPasoCuandoEstaDisponible(t *testing.T) {
	defer tv3cSinPanico(t)
	for _, col := range []string{boardcmd.ColTestWriter, boardcmd.ColWriter, boardcmd.ColReview} {
		for _, esperado := range []bool{false, true} {
			c := tv3cSola("sola", "Tarjeta sola", col)
			que := "enter en " + col
			if esperado {
				c = tv3cConRojo(c, true)
				que += " con el rojo esperado"
			}
			m := tv3cModelo(tv3cConfig(200, 40), c)
			tv3cAbre(t, "CA-704", que, m, tablero.TeclaEnter, tablero.ModalSeguir)
			tv3cTitulo(t, "CA-704", m, 200, "Seguir solo · Tarjeta sola")
		}
	}
}

// CA-704: sin Seguir solo, el paso es el de la v2: la accion de rol de la
// columna, aprobar en Tu aprobacion, integrar en Tu aceptacion.
func TestV3C_CA704_SinSeguirSoloElPasoEsElDeLaV2(t *testing.T) {
	defer tv3cSinPanico(t)
	casos := []struct {
		nombre string
		c      boardcmd.Card
		modal  tablero.Modal
		titulo string
	}{
		{"Writer sin auto", tv3cSinAuto(tv3cSola("w", "W", boardcmd.ColWriter)), tablero.ModalPedir, "Pedir al writer · w"},
		{"Writer con auto y rojo no esperado", tv3cConRojo(tv3cSola("w", "W", boardcmd.ColWriter), false), tablero.ModalPedir, "Pedir al writer · w"},
		{"Test-writer sin auto", tv3cSinAuto(tv3cSola("tw", "TW", boardcmd.ColTestWriter)), tablero.ModalPedir, "Pedir al test-writer · tw"},
		{"Tu aprobacion con auto", tv3cSola("ap", "AP", boardcmd.ColTuAprobacion), tablero.ModalFirma, "Aprobar spec · ap"},
		{"Tu aceptacion con auto", tv3cSola("ac", "AC", boardcmd.ColTuAceptacion), tablero.ModalFirma, "Integrar · ac"},
		{"Arquitecto con auto", tv3cSola("aq", "AQ", boardcmd.ColArquitecto), tablero.ModalPedir, "Pedir al arquitecto · aq"},
	}
	for _, c := range casos {
		m := tv3cModelo(tv3cConfig(200, 40), c.c)
		tv3cAbre(t, "CA-704", "enter: "+c.nombre, m, tablero.TeclaEnter, c.modal)
		tv3cTitulo(t, "CA-704", m, 200, c.titulo)
	}
}

// CA-704: con un piloto de este tablero vivo no hay paso y el texto es "el
// piloto lleva la tarjeta: espera a que se detenga", tambien cuando la
// tarjeta muestra el running del rol que el piloto lanzo; al terminar el
// piloto (con 0) Seguir solo vuelve a ser el paso.
func TestV3C_CA704_ConElPilotoVivoNoHayPaso(t *testing.T) {
	defer tv3cSinPanico(t)
	c := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m := tv3cModelo(tv3cConfig(200, 40), c)
	l := tv3cLanzaSeguir(t, "CA-704", m, "sola")

	const msg = "el piloto lleva la tarjeta: espera a que se detenga"
	tv3cNoAbre(t, "CA-704", "enter con el piloto vivo", m, tablero.TeclaEnter, msg)

	tv3cLeer(m, tv3cConRunning(c, "env-del-piloto", "writer"))
	tv3cNoAbre(t, "CA-704", "enter con el piloto vivo y el writer que lanzo corriendo", m, tablero.TeclaEnter, msg)

	tv3cLeer(m, c)
	tv3cFin(m, l.ID, 0, tv3cPrefijoParada+"la tarjeta llego a tu aceptacion")
	tv3cLeer(m, c)
	tv3cAbre(t, "CA-704", "enter despues de que el piloto termino con 0", m, tablero.TeclaEnter, tablero.ModalSeguir)
}

// CA-704: las reglas de antes de Seguir solo siguen primero: en Hecho, con
// running (sin piloto) y con interrupted no hay paso, con sus textos.
func TestV3C_CA704_HechoRunningEInterrupted(t *testing.T) {
	defer tv3cSinPanico(t)
	// con running, aunque tenga auto
	run := tv3cConRunning(tv3cSola("corre", "Corre", boardcmd.ColWriter), "env-corre", "writer")
	m := tv3cModelo(tv3cConfig(200, 40), run)
	tv3cNoAbre(t, "CA-704", "enter con running", m, tablero.TeclaEnter, "el writer esta trabajando: espera a que termine")

	// con interrupted, aunque tenga auto y accion de reanudar
	cortada := ttPagosCaidos()
	cortada.Item.Auto = "hasta-humano"
	m = tv3cModelo(tv3cConfig(200, 40), cortada)
	tv3cNoAbre(t, "CA-704", "enter con interrupted", m, tablero.TeclaEnter, "el trabajo se corto: R reanuda o pide de nuevo")

	// en Hecho: abrir HECHO, bajar a la tarjeta
	m = tv3cModelo(tv3cConfig(200, 40), ttViejo(1))
	ttTeclas(m, tablero.TeclaEnter, "j")
	if m.Elegida() != "viejo-1" {
		t.Fatalf("CA-704: precondicion del spec: enter abre HECHO y j elige viejo-1; elegida=%q\n%s", m.Elegida(), ttPantalla(m))
	}
	tv3cNoAbre(t, "CA-704", "enter en Hecho", m, tablero.TeclaEnter, "la tarjeta ya esta en Hecho")
}

// CA-704: sin paso, el texto es el de la v2: sin acciones, sin una accion con
// tecla, la accion que no esta y la deshabilitada. El texto dura
// DuracionAviso.
func TestV3C_CA704_TextosSinPaso(t *testing.T) {
	defer tv3cSinPanico(t)
	writer := ttNombreColumna(boardcmd.ColWriter)
	desh := ttAccionWriter()
	desh.Enabled, desh.Why = false, "un why inventado para la accion"
	aprobarDesh := ttAccionAprobar()
	aprobarDesh.Enabled, aprobarDesh.Why = false, "otro why inventado"
	enAprob := ttCard("ap", "AP", boardcmd.ColTuAprobacion, "plain ap")
	enAprob.Actions = []boardcmd.Action{ttAccionGuardar(true, "", "a.go")}
	enAprobDesh := ttCard("ap", "AP", boardcmd.ColTuAprobacion, "plain ap")
	enAprobDesh.Actions = []boardcmd.Action{aprobarDesh}
	casos := []struct {
		nombre string
		c      boardcmd.Card
		msg    string
	}{
		{"sin acciones", ttTarjetaCon(), "en " + writer + " no hay nada que hacer ahora: el plain de la tarjeta"},
		{"solo una accion sin tecla", ttTarjetaCon(ttRelanzar()), "en " + writer + " no hay nada que hacer ahora: el plain de la tarjeta"},
		{"la primera accion deshabilitada", ttTarjetaCon(desh), "un why inventado para la accion"},
		{"Tu aprobacion sin aprobar", enAprob, "en " + ttNombreColumna(boardcmd.ColTuAprobacion) + " no hay " + ttHaceDe(t, "A")},
		{"aprobar deshabilitada", enAprobDesh, "otro why inventado"},
		{"backlog con el arquitecto deshabilitado", ttBacklog("bk", "Backlog"), ttWhySinEspac},
	}
	for _, c := range casos {
		m := tv3cModelo(tv3cConfig(200, 40), c.c)
		tv3cNoAbre(t, "CA-704", "enter: "+c.nombre, m, tablero.TeclaEnter, c.msg)
		m.Tic(ttAhora.Add(tablero.DuracionAviso + 1e9))
		tv3cSinMensaje(t, "CA-704", "el texto sin paso pasado DuracionAviso", m)
	}
}

// CA-705 y CA-566: enter sobre una tarjeta de la lista, o en su detalle, abre
// el modal de su paso, sea Pedir, Firma, Guardar o Seguir solo, y nunca
// lanza.
func TestV3C_CA705_CA566_EnterAbreElModalDelPaso(t *testing.T) {
	defer tv3cSinPanico(t)
	casos := []struct {
		nombre string
		c      boardcmd.Card
		modal  tablero.Modal
	}{
		{"pedir", ttTarjetaCon(ttAccionWriter()), tablero.ModalPedir},
		{"firma", ttNucTenancy(), tablero.ModalFirma},
		{"guardar", ttTarjetaCon(ttAccionGuardar(true, "", "a.go", "b.go")), tablero.ModalGuardar},
		{"seguir solo", tv3cSola("sola", "Sola", boardcmd.ColReview), tablero.ModalSeguir},
	}
	for _, c := range casos {
		m := tv3cModelo(tv3cConfig(200, 40), c.c)
		tv3cAbre(t, "CA-705", "enter en la lista ("+c.nombre+")", m, tablero.TeclaEnter, c.modal)

		m = tv3cModelo(tv3cConfig(200, 40), c.c)
		ttTeclas(m, tablero.TeclaTab)
		if m.Lugar() != tablero.LugarVerTarjeta {
			t.Fatalf("CA-705: precondicion del spec: tab pasa al detalle de la tarjeta; lugar=%q", m.Lugar())
		}
		tv3cAbre(t, "CA-705", "enter en el detalle ("+c.nombre+")", m, tablero.TeclaEnter, c.modal)
	}
}

// CA-705: n abre el paso de la tarjeta de la caja, este elegida o no, desde
// la lista, el detalle, la vista Hallazgos y el detalle del proyecto. La caja
// es la primera de TE TOCA en el orden de la lista (Writer antes que Tu
// aprobacion).
func TestV3C_CA705_NAbreElPasoDeLaTarjetaDeLaCaja(t *testing.T) {
	defer tv3cSinPanico(t)
	tablero1 := func() *tablero.Modelo {
		m := tv3cModelo(tv3cConfig(200, 40), ttNucTenancy(), ttPagosCaidos())
		m.LeerHallazgos(ttHallazgosDorados())
		m.LeerProyecto(ttProyectoVerde())
		return m
	}
	m := tablero1()
	if m.Caja() != "nuc-tenancy" {
		t.Fatalf("CA-705: precondicion del spec: la caja es nuc-tenancy (TE TOCA), es %q", m.Caja())
	}
	ttTeclas(m, "j")
	if m.Elegida() != "pagos-caidos" {
		t.Fatalf("CA-705: precondicion del spec: j elige pagos-caidos (DETENIDA); elegida=%q", m.Elegida())
	}
	tv3cAbre(t, "CA-705", "n con otra tarjeta elegida", m, "n", tablero.ModalFirma)
	tv3cTitulo(t, "CA-705", m, 200, "Aprobar spec · nuc-tenancy")

	for _, prep := range []struct {
		nombre string
		teclas []tablero.Tecla
	}{
		{"desde el detalle de otra tarjeta", []tablero.Tecla{"j", tablero.TeclaTab}},
		{"desde la vista Hallazgos", []tablero.Tecla{"H"}},
		{"desde el detalle de un hallazgo", []tablero.Tecla{"H", tablero.TeclaEnter}},
		{"desde el detalle del proyecto", []tablero.Tecla{"v"}},
	} {
		m := tablero1()
		ttTeclas(m, prep.teclas...)
		tv3cSinModal(t, "CA-705", "precondicion del spec: "+prep.nombre, m)
		tv3cAbre(t, "CA-705", "n "+prep.nombre, m, "n", tablero.ModalFirma)
		tv3cTitulo(t, "CA-705", m, 200, "Aprobar spec · nuc-tenancy")
	}

	// Writer va antes que Tu aprobacion en TE TOCA: la caja es la de Writer
	m = tv3cModelo(tv3cConfig(200, 40), ttNucTenancy(), ttTarjetaCon(ttAccionWriter()))
	if m.Caja() != "tarjeta" {
		t.Fatalf("CA-705: la caja es la primera de TE TOCA en el orden de la lista (Writer antes que Tu aprobacion): %q", m.Caja())
	}
	tv3cAbre(t, "CA-705", "n con la caja en Writer", m, "n", tablero.ModalPedir)
	tv3cTitulo(t, "CA-705", m, 200, "Pedir al writer · tarjeta")

	// la caja en DETENIDA: su paso no hay, n dice su texto
	m = tv3cModelo(tv3cConfig(200, 40), ttPagosCaidos())
	if m.Caja() != "pagos-caidos" {
		t.Fatalf("CA-705: sin TE TOCA la caja es la primera de DETENIDA: %q", m.Caja())
	}
	tv3cNoAbre(t, "CA-705", "n con la caja cortada", m, "n", "el trabajo se corto: R reanuda o pide de nuevo")
}

// CA-705: sin tarjeta en la caja, n dice el texto de su caso: todo corre
// (tambien un piloto de este tablero), nada pendiente; con solo EN COLA abre
// el paso de la primera de EN COLA.
func TestV3C_CA705_NSinTarjetaEnLaCaja(t *testing.T) {
	defer tv3cSinPanico(t)
	m := tv3cModelo(tv3cConfig(200, 40), ttInfraAPI(true))
	if m.Caja() != "" {
		t.Fatalf("CA-705: con todo en CORRIENDO la caja no tiene tarjeta: %q", m.Caja())
	}
	tv3cNoAbre(t, "CA-705", "n con todo corriendo", m, "n", "nada te espera: hoom esta trabajando")

	sola := tv3cSola("sola", "Sola", boardcmd.ColWriter)
	m = tv3cModelo(tv3cConfig(200, 40), sola)
	tv3cLanzaSeguir(t, "CA-705", m, "sola")
	if m.Caja() != "" || m.Grupo("sola") != tablero.GrupoCorriendo {
		t.Fatalf("CA-705: con su piloto vivo la tarjeta esta en CORRIENDO y la caja vacia: caja=%q grupo=%q", m.Caja(), m.Grupo("sola"))
	}
	tv3cNoAbre(t, "CA-705", "n con el piloto corriendo", m, "n", "nada te espera: hoom esta trabajando")

	lista := ttBacklog("lista", "Lista para empezar")
	lista.Actions = []boardcmd.Action{ttAccionRol(boardcmd.ActPedirArquitecto, "arquitecto", "Pedir al arquitecto", "el spec")}
	m = tv3cModelo(tv3cConfig(200, 40), lista, ttBacklog("otra", "Otra"))
	if m.Caja() != "" {
		t.Fatalf("CA-705: con solo EN COLA Caja es \"\": %q", m.Caja())
	}
	ttTeclas(m, "j")
	tv3cAbre(t, "CA-705", "n con solo EN COLA", m, "n", tablero.ModalPedir)
	tv3cTitulo(t, "CA-705", m, 200, "Pedir al arquitecto · lista")

	m = tv3cModelo(tv3cConfig(200, 40), ttBacklog("bk", "Backlog"))
	tv3cNoAbre(t, "CA-705", "n con la primera de EN COLA deshabilitada", m, "n", ttWhySinEspac)

	for _, b := range []struct {
		nombre string
		cards  []boardcmd.Card
	}{{"solo Hecho", []boardcmd.Card{ttViejo(1), ttViejo(2)}}, {"sin tarjetas", nil}} {
		m = tv3cModelo(tv3cConfig(200, 40), b.cards...)
		tv3cNoAbre(t, "CA-705", "n "+b.nombre, m, "n", "no hay nada pendiente")
	}
}

// CA-705: ninguna tecla de accion salvo u lanza: a, t, w, r, R, s, A, I, n, P
// y enter, en la lista y en el detalle, sobre tarjetas de todas las formas,
// abren un modal o dicen un texto; u lanza hoom usar al momento.
func TestV3C_CA705_NingunaTeclaDeAccionLanza(t *testing.T) {
	defer tv3cSinPanico(t)
	cards := []boardcmd.Card{
		ttInfraAPI(false), ttInfraAPI(true), ttNucTenancy(), ttPagosCaidos(), ttBacklog("bk", "Backlog"), ttPrecios(),
		ttTarjetaCon(ttAccionGuardar(true, "", "a.go")), tv3cSola("sola", "Sola", boardcmd.ColWriter),
		tv3cConRojo(tv3cSola("roja", "Roja", boardcmd.ColTestWriter), true),
		tv3cSola("acep", "Acep", boardcmd.ColTuAceptacion),
	}
	teclas := []tablero.Tecla{"a", "t", "w", "r", "R", "s", "A", "I", "n", "P", tablero.TeclaEnter}
	modales := map[tablero.Modal]bool{tablero.SinModal: true, tablero.ModalPedir: true, tablero.ModalFirma: true,
		tablero.ModalGuardar: true, tablero.ModalSeguir: true}
	for _, c := range cards {
		for _, enDetalle := range []bool{false, true} {
			for _, k := range teclas {
				m := tv3cModelo(tv3cConfig(200, 40), c)
				if enDetalle {
					ttTeclas(m, tablero.TeclaTab)
				}
				ords := ttTeclas(m, k)
				if len(ttLanzares(ords)) != 0 {
					t.Fatalf("CA-705: %q sobre %s (detalle=%v) nunca lanza, dio %#v", k, c.Slug, enDetalle, ords)
				}
				if !modales[m.Modal()] {
					t.Fatalf("CA-705: %q sobre %s abre un modal de accion o nada, abrio %q", k, c.Slug, m.Modal())
				}
			}
		}
		m := tv3cModelo(tv3cConfig(200, 40), c)
		l := ttUnaLanzar(t, "CA-705", "u sobre "+c.Slug, ttTeclas(m, "u"))
		ttMismosArgs(t, "CA-705", "u", l.Args, []string{"usar", c.Slug})
		tv3cSinModal(t, "CA-705", "u", m)
	}
}
