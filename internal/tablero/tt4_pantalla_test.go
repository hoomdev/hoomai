// La pantalla de la v3 (spec tablero-tu-siguiente-paso): la disposicion
// (CA-696, re-expresa CA-550), la cabecera (CA-697), la caja TU SIGUIENTE
// PASO (CA-698, CA-699), los grupos (CA-700, re-expresa CA-555), las filas
// (CA-701, re-expresa CA-555), el camino (CA-702), las filas de CORRIENDO y
// DETENIDA (CA-711, re-expresa CA-558), el Registro plegado (CA-714) y los
// modos sin color y ASCII (CA-720). Cada aseveracion sale del spec y de
// doc.go; las cajas se leen en la posicion que fija el contrato.
package tablero_test

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ================================================================ CA-696: la disposicion

// tv3aRevisarFranjas exige las lineas fijas de toda pantalla de 60x15 o
// mas: la cabecera en la 1, la caja en la 2 a 5, el Registro en la A-2 y la
// linea de teclas en la A.
func tv3aRevisarFranjas(t *testing.T, que string, m *tablero.Modelo, ancho, alto int, utf bool) {
	t.Helper()
	ttForma(t, "CA-696", m, ancho, alto, false)
	ls := ttLimpia(m)
	if !strings.HasPrefix(ls[0], " hoom tablero · ") {
		t.Fatalf("CA-696: %s: la linea 1 es la cabecera (\" hoom tablero · ...\"): %q\n%s", que, ls[0], tv3aLineas(m))
	}
	paso := tv3aCajaPaso(t, "CA-696", m, ancho)
	if paso.Foco || !strings.HasPrefix(paso.Arriba, tv3aSig(utf).ESI+"TU SIGUIENTE PASO") {
		t.Fatalf("CA-696: %s: las lineas 2 a 5 son la caja TU SIGUIENTE PASO, con los signos simples (nunca el foco): %q\n%s",
			que, paso.Arriba, tv3aLineas(m))
	}
	lin := tv3aSig(utf).Lin
	if !strings.HasPrefix(ls[alto-3], lin+"Registro") {
		t.Fatalf("CA-696: %s: la linea A-2 (%d) es el Registro plegado (%q...): %q\n%s", que, alto-2, lin+"Registro", ls[alto-3], tv3aLineas(m))
	}
	if !strings.HasPrefix(ls[alto-1], " enter ") {
		t.Fatalf("CA-696: %s: la linea A (%d) es la de teclas, que empieza con la pista de enter: %q", que, alto, ls[alto-1])
	}
}

// CA-696, CA-550: con 100 columnas o mas, dos zonas: la lista a la
// izquierda en W*2/5 celdas y el detalle a la derecha en el resto, las dos
// con todo el alto del cuerpo (C = A-8 lineas, de la 6 a la A-3); el foco
// empieza en la lista.
func TestV3A_CA696_CA550_DosZonas(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{false, true} {
		for _, tam := range [][2]int{{100, 30}, {120, 36}, {101, 15}, {100, 15}, {133, 21}, {150, 40}, {199, 60}} {
			w, a := tam[0], tam[1]
			que := fmt.Sprintf("%dx%d utf=%v", w, a, utf)
			m := tv3aModeloDorado(tv3aCfg(w, a, utf, false))
			tv3aRevisarFranjas(t, que, m, w, a, utf)
			l := tv3aL(w)
			li := tv3aCajaEn(t, "CA-696", m, 5, 0, l)
			de := tv3aCajaEn(t, "CA-696", m, 5, l, w-l)
			if li.Alto != tv3aC(a) || de.Alto != tv3aC(a) {
				t.Fatalf("CA-696: %s: la lista (%d celdas) y el detalle (%d) tienen las C=%d lineas del cuerpo; tienen %d y %d:\n%s",
					que, l, w-l, tv3aC(a), li.Alto, de.Alto, tv3aLineas(m))
			}
			if !li.Foco || de.Foco || m.Foco() != tablero.ZonaLista {
				t.Fatalf("CA-696: %s: al abrir el foco esta en la lista (sus signos) y no en el detalle:\n%s", que, tv3aLineas(m))
			}
			if !strings.HasPrefix(li.Arriba, tv3aSig(utf).FESI+"Tarjetas") {
				t.Fatalf("CA-696: %s: la lista se titula Tarjetas: %q", que, li.Arriba)
			}
		}
	}
}

// CA-696, CA-550: entre 60 y 99 columnas, una columna: la lista arriba con
// todo el ancho y el foco, en max(3, min(N, (C+1)/2)) lineas (N su alto
// natural: sus 11 lineas de contenido + 2), y el detalle abajo con el resto.
func TestV3A_CA696_CA550_UnaColumna(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{false, true} {
		for _, tam := range [][2]int{{60, 15}, {80, 24}, {99, 30}, {70, 40}, {99, 15}, {64, 22}, {61, 17}} {
			w, a := tam[0], tam[1]
			que := fmt.Sprintf("%dx%d utf=%v", w, a, utf)
			m := tv3aModeloDorado(tv3aCfg(w, a, utf, false))
			tv3aRevisarFranjas(t, que, m, w, a, utf)
			c := tv3aC(a)
			hl := (c + 1) / 2
			if 13 < hl {
				hl = 13
			}
			if hl < 3 {
				hl = 3
			}
			li := tv3aCajaEn(t, "CA-696", m, 5, 0, w)
			if li.Alto != hl || !li.Foco {
				t.Fatalf("CA-696: %s: la lista esta arriba, con el foco, en max(3, min(13, (C+1)/2)) = %d lineas; tiene %d (foco %v):\n%s",
					que, hl, li.Alto, li.Foco, tv3aLineas(m))
			}
			de := tv3aCajaEn(t, "CA-696", m, 5+hl, 0, w)
			if de.Alto != c-hl || de.Foco {
				t.Fatalf("CA-696: %s: el detalle esta debajo de la lista con las %d lineas que quedan y sin el foco; tiene %d (foco %v):\n%s",
					que, c-hl, de.Alto, de.Foco, tv3aLineas(m))
			}
		}
	}
}

// CA-696, CA-550: en una columna, con el foco en el detalle la lista se
// pliega a una linea ("─Tarjetas · 1 de 7────", todo el ancho con el signo
// de linea simple) y el detalle ocupa las C-1 lineas que quedan, con el
// foco; al volver el foco a la lista, se despliega.
func TestV3A_CA696_CA550_ListaPlegadaConFocoEnElDetalle(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		for _, tam := range [][2]int{{80, 24}, {60, 15}, {99, 30}} {
			w, a := tam[0], tam[1]
			que := fmt.Sprintf("%dx%d utf=%v", w, a, utf)
			m := tv3aModeloDorado(tv3aCfg(w, a, utf, false))
			m.Tecla(tablero.TeclaTab, ttAhora)
			if m.Foco() != tablero.ZonaDetalle {
				t.Fatalf("CA-696: %s: tab pasa el foco al detalle; Foco = %v", que, m.Foco())
			}
			lin := tv3aSig(utf).Lin
			titulo := lin + "Tarjetas · 1 de 7"
			quiere := titulo + strings.Repeat(lin, w-ttCeldas(titulo))
			if got := tv3aDe(ttLimpia(m), 5); got != quiere {
				t.Fatalf("CA-696: %s: con el foco en el detalle la lista es una linea en la 6:\n got %q\nwant %q\n%s", que, got, quiere, tv3aLineas(m))
			}
			de := tv3aCajaEn(t, "CA-696", m, 6, 0, w)
			if de.Alto != tv3aC(a)-1 || !de.Foco {
				t.Fatalf("CA-696: %s: el detalle ocupa las C-1 = %d lineas con el foco; tiene %d (foco %v):\n%s",
					que, tv3aC(a)-1, de.Alto, de.Foco, tv3aLineas(m))
			}
			m.Tecla(tablero.TeclaShiftTab, ttAhora)
			if tv3aListaPlegada(m) || !tv3aCajaEn(t, "CA-696", m, 5, 0, w).Foco {
				t.Fatalf("CA-696: %s: con el foco de vuelta en la lista, la lista se despliega con el foco:\n%s", que, tv3aLineas(m))
			}
		}
	}
}

// CA-696, CA-550: en dos zonas la lista nunca se pliega: con el foco en el
// detalle las dos siguen con todo el alto del cuerpo y el foco cambia de
// caja.
func TestV3A_CA696_CA550_DosZonasNoSePliega(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(100, 30, true, false))
	m.Tecla(tablero.TeclaTab, ttAhora)
	li := tv3aCajaEn(t, "CA-696", m, 5, 0, 40)
	de := tv3aCajaEn(t, "CA-696", m, 5, 40, 60)
	if li.Foco || !de.Foco || li.Alto != 22 || de.Alto != 22 {
		t.Fatalf("CA-696: con dos zonas y el foco en el detalle, las dos cajas tienen 22 lineas y el foco es del detalle:\n%s", tv3aLineas(m))
	}
}

// CA-696, CA-550: debajo de 60x15 la pantalla es una linea, "agranda la
// terminal (minimo 60x15)" cortada al ancho, y Alto-1 lineas vacias; al
// volver a un tamano valido vuelve la pantalla de antes.
func TestV3A_CA696_CA550_DebajoDe60x15(t *testing.T) {
	defer tv3aSinPanico(t)
	const msg = "agranda la terminal (minimo 60x15)"
	for _, utf := range []bool{false, true} {
		for _, tam := range [][2]int{{59, 15}, {60, 14}, {59, 40}, {200, 14}, {30, 10}, {20, 40}} {
			w, a := tam[0], tam[1]
			m := tv3aModeloDorado(tv3aCfg(w, a, utf, false))
			p := m.Pantalla()
			quiere := tv3aCortar(msg, w, utf)
			if len(p) != a || p[0] != quiere {
				t.Fatalf("CA-696: a %dx%d la pantalla es la linea %q y %d vacias: %q", w, a, quiere, a-1, p)
			}
			for i, l := range p[1:] {
				if l != "" {
					t.Fatalf("CA-696: a %dx%d la linea %d esta vacia: %q", w, a, i+2, l)
				}
			}
		}
		m := tv3aModeloDorado(tv3aCfg(80, 24, utf, false))
		antes := ttPantalla(m)
		m.Tamano(59, 15)
		if ttLinea(m, 1) != msg {
			t.Fatalf("CA-696: al achicar a 59x15 la pantalla dice que se agrande: %q", m.Pantalla())
		}
		m.Tamano(80, 24)
		if got := ttPantalla(m); got != antes {
			t.Fatalf("CA-696: al volver a 80x24 vuelve la pantalla de antes:\n%s\nantes:\n%s", got, antes)
		}
	}
}

// CA-696, CA-550: los bordes de 99 y 100 columnas: con 99 una columna (la
// lista con todo el ancho), con 100 dos zonas (40 y 60).
func TestV3A_CA696_CA550_BordeDe99y100(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(99, 30, true, false))
	if li := tv3aCajaEn(t, "CA-696", m, 5, 0, 99); li.Ancho != 99 {
		t.Fatalf("CA-696: a 99 columnas la lista tiene todo el ancho")
	}
	m = tv3aModeloDorado(tv3aCfg(100, 30, true, false))
	tv3aCajaEn(t, "CA-696", m, 5, 0, 40)
	tv3aCajaEn(t, "CA-696", m, 5, 40, 60)
}

// ================================================================ CA-697: la cabecera

// tv3aCabeceraQuiere es la linea 1 por la regla de doc.go ("The header"):
// " " + izquierda, espacios, y la derecha terminando en la celda W-2; lo que
// no entra se cae (la fijada, despues lo de hoy) y por ultimo se corta el
// check a W-4-Celdas(izq); si ni eso, no hay derecha y la izquierda se corta
// a W-2.
func tv3aCabeceraQuiere(izq string, partes []string, w int, utf bool) string {
	cl := ttCeldas(izq)
	der := ""
	for len(partes) > 0 {
		der = strings.Join(partes, "")
		if 1+cl+2+ttCeldas(der)+1 <= w {
			break
		}
		if len(partes) > 1 {
			partes = partes[:len(partes)-1]
			continue
		}
		if n := w - 4 - cl; n >= 1 {
			der = tv3aCortar(partes[0], n, utf)
		} else {
			der = ""
		}
		break
	}
	if der == "" {
		return " " + tv3aCortar(izq, w-2, utf)
	}
	return " " + izq + strings.Repeat(" ", w-1-1-cl-ttCeldas(der)) + der
}

// CA-697: a todo ancho de 60 a 140 la cabecera es "hoom tablero · demo ·
// rama main" a la izquierda y, pegado a la celda W-2, el check, lo de hoy y
// la fijada, que se caen en ese orden (la fijada primero) y por ultimo se
// corta el check; la izquierda nunca se corta antes.
func TestV3A_CA697_CabeceraEnCadaAncho(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{false, true} {
		for w := 60; w <= 140; w++ {
			m := tv3aModeloDorado(tv3aCfg(w, 20, utf, false))
			quiere := tv3aCabeceraQuiere("hoom tablero · demo · rama main",
				[]string{"check VERDE · verify hace 2h", " · hoy 3.88 USD", " · fijada: exportar-pdf"}, w, utf)
			if got := ttLinea(m, 1); got != quiere {
				t.Fatalf("CA-697: a %d columnas (utf=%v) la cabecera es\n%q\nes\n%q", w, utf, quiere, got)
			}
		}
	}
}

// CA-697: sin git dice "sin git" en lugar de la rama; sin trabajos hoy,
// " · hoy sin trabajos"; sin fijada no hay " · fijada:".
func TestV3A_CA697_CabeceraSinGitYSinTrabajos(t *testing.T) {
	defer tv3aSinPanico(t)
	p := ttProyectoVerde()
	p.Git, p.Rama, p.Hoy = false, "", costcmd.Hoy{}
	m := ttModelo(tv3aCfg(120, 20, true, false), tv3aTableroDorado())
	m.LeerProyecto(p)
	quiere := tv3aCabeceraQuiere("hoom tablero · demo · sin git", []string{"check VERDE · verify hace 2h", " · hoy sin trabajos"}, 120, true)
	if got := ttLinea(m, 1); got != quiere {
		t.Fatalf("CA-697: sin git, sin trabajos hoy y sin fijada la cabecera es\n%q\nes\n%q", quiere, got)
	}
}

// CA-697: el check rojo dice "check ROJO: <motivo>" (checkcmd.Motivo) y sin
// veredicto "sin veredicto todavia".
func TestV3A_CA697_CabeceraCheckRojoYSinVeredicto(t *testing.T) {
	defer tv3aSinPanico(t)
	casos := []struct {
		que   string
		check checkcmd.Result
		dice  string
	}{
		{"rojo", checkcmd.Result{OK: false, VerdictID: "v-9", LastVerdict: "red", Reason: checkcmd.ReasonRedVerdict},
			"check ROJO: " + checkcmd.Motivo(checkcmd.Result{OK: false, VerdictID: "v-9", LastVerdict: "red", Reason: checkcmd.ReasonRedVerdict})},
		{"sin veredicto", checkcmd.Result{OK: false, Reason: checkcmd.ReasonNoVerdict}, "sin veredicto todavia"},
	}
	for _, c := range casos {
		p := ttProyectoVerde()
		p.Check = c.check
		p.Veredicto, p.VeredictoDelCheck = nil, nil
		m := ttModelo(tv3aCfg(200, 20, true, false), tv3aTableroDorado())
		m.LeerProyecto(p)
		quiere := tv3aCabeceraQuiere("hoom tablero · demo · rama main", []string{c.dice, " · hoy 3.88 USD"}, 200, true)
		if got := ttLinea(m, 1); got != quiere {
			t.Fatalf("CA-697: %s: la cabecera es\n%q\nes\n%q", c.que, quiere, got)
		}
	}
}

// CA-697: antes de la primera lectura del proyecto la derecha dice
// "leyendo el proyecto..." (y la izquierda no tiene rama), tambien antes de
// la primera lectura del tablero.
func TestV3A_CA697_CabeceraAntesDelProyecto(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{false, true} {
		m := tablero.Nuevo(tv3aCfg(90, 20, utf, false))
		quiere := tv3aCabeceraQuiere("hoom tablero · demo", []string{"leyendo el proyecto..."}, 90, utf)
		if got := ttLinea(m, 1); got != quiere {
			t.Fatalf("CA-697: antes de toda lectura la cabecera es\n%q\nes\n%q", quiere, got)
		}
		m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: tv3aTableroDorado()})
		if got := ttLinea(m, 1); got != quiere {
			t.Fatalf("CA-697: con el tablero pero sin el proyecto la cabecera es\n%q\nes\n%q", quiere, got)
		}
	}
}

// CA-697 (hostil): un nombre de proyecto largo: mientras la izquierda entra,
// se cae y se corta primero la derecha; cuando ni un check de una celda
// entra, no hay derecha y la izquierda se corta a W-2.
func TestV3A_CA697_CabeceraProyectoLargo(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, n := range []int{10, 20, 25, 30, 40, 55, 70, 90} {
		nombre := strings.Repeat("p", n)
		for _, w := range []int{60, 75, 99, 100, 130} {
			cfg := tv3aCfg(w, 20, true, false)
			cfg.Proyecto = nombre
			m := tablero.Nuevo(cfg)
			m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: tv3aTableroDorado(), Fijada: "exportar-pdf"})
			m.LeerProyecto(ttProyectoVerde())
			quiere := tv3aCabeceraQuiere("hoom tablero · "+nombre+" · rama main",
				[]string{"check VERDE · verify hace 2h", " · hoy 3.88 USD", " · fijada: exportar-pdf"}, w, true)
			if got := ttLinea(m, 1); got != quiere {
				t.Fatalf("CA-697: proyecto de %d letras a %d columnas: la cabecera es\n%q\nes\n%q", n, w, quiere, got)
			}
			ttForma(t, "CA-697", m, w, 20, false)
		}
	}
}

// CA-697: con color "hoom tablero" va en verde y negrita (1;32), VERDE en
// verde (32) y ROJO en rojo (31).
func TestV3A_CA697_CabeceraColores(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(120, 30, true, true))
	tv3aSgrDe(t, "CA-697", "hoom tablero", m, 1, "hoom tablero", 0, "1;32")
	tv3aSgrDe(t, "CA-697", "VERDE", m, 1, "VERDE", 0, "32")
	p := ttProyectoVerde()
	p.Check = checkcmd.Result{OK: false, VerdictID: "v-9", LastVerdict: "red", Reason: checkcmd.ReasonRedVerdict}
	m.LeerProyecto(p)
	tv3aSgrDe(t, "CA-697", "ROJO", m, 1, "ROJO", 0, "31")
}

// ================================================================ CA-698: la tarjeta de la caja

// tv3aPasoCaso es una tarjeta sola en el tablero y lo que la caja dice de
// ella: la frase (sin el titulo: antes y despues de «T») y la linea 2.
type tv3aPasoCaso struct {
	que            string
	card           boardcmd.Card
	antes, despues string // la frase es antes + «T» + despues (con UTF-8)
	plain          string
	izq            string // la izquierda de la linea 2 (sin el margen)
	grupo          tablero.Grupo
}

func tv3aPasoCasos() []tv3aPasoCaso {
	aprobarNo := ttAccionAprobar()
	aprobarNo.Enabled, aprobarNo.Why = false, "el spec no pasa el lint"
	writerNo := ttAccionWriter()
	writerNo.Enabled, writerNo.Why = false, "falta el espacio de trabajo"
	return []tv3aPasoCaso{
		{"aprobar", tv3aCon(tv3aEn("t-a", "Tarea A", boardcmd.ColTuAprobacion), ttAccionAprobar()),
			"Firmá el spec de ", ".", "plain de t-a", "n aprobar spec", tablero.GrupoTeToca},
		{"integrar", tv3aCon(tv3aEn("t-b", "Tarea B", boardcmd.ColTuAceptacion), ttAccionIntegrar()),
			"Aceptá ", ".", "plain de t-b", "n integrar", tablero.GrupoTeToca},
		{"pedir arquitecto", tv3aCon(tv3aEn("t-c", "Tarea C", boardcmd.ColArquitecto), tv3aPedirArq()),
			"Pedile el spec al arquitecto para ", ".", "plain de t-c", "n pedir al arquitecto", tablero.GrupoTeToca},
		{"pedir test-writer", tv3aCon(tv3aEn("t-d", "Tarea D", boardcmd.ColTestWriter), tv3aPedirTW()),
			"Pedile los tests al test-writer para ", ".", "plain de t-d", "n pedir al test-writer", tablero.GrupoTeToca},
		{"pedir writer", tv3aCon(tv3aEn("t-e", "Tarea E", boardcmd.ColWriter), ttAccionWriter()),
			"Pedile el código al writer para ", ".", "plain de t-e", "n pedir al writer", tablero.GrupoTeToca},
		{"pedir reviewer", tv3aCon(tv3aEn("t-f", "Tarea F", boardcmd.ColReview), tv3aPedirRev()),
			"Pedile la review al reviewer para ", ".", "plain de t-f", "n pedir al reviewer", tablero.GrupoTeToca},
		{"reanudar", tv3aCon(tv3aEn("t-g", "Tarea G", boardcmd.ColWriter), tv3aReanudar()),
			"Reanudá el trabajo de ", ".", "plain de t-g", "n reanudar", tablero.GrupoTeToca},
		{"guardar", tv3aCon(tv3aEn("t-h", "Tarea H", boardcmd.ColWriter), ttAccionGuardar(true, "", "x.go")),
			"Guardá el trabajo de ", ".", "plain de t-h", "n guardar", tablero.GrupoTeToca},
		{"seguir solo", tv3aConPiloto(tv3aCon(tv3aEn("t-i", "Tarea I", boardcmd.ColWriter), ttAccionWriter())),
			"Dejá que ", " siga sola.", "plain de t-i", "n seguir solo", tablero.GrupoTeToca},
		{"sin acciones", tv3aEn("t-j", "Tarea J", boardcmd.ColWriter),
			"Mirá ", ".", "plain de t-j", "en Writer no hay nada que hacer ahora: plain de t-j", tablero.GrupoTeToca},
		{"aprobar deshabilitado", tv3aCon(tv3aEn("t-k", "Tarea K", boardcmd.ColTuAprobacion), aprobarNo),
			"Mirá ", ".", "plain de t-k", "el spec no pasa el lint", tablero.GrupoTeToca},
		{"Tu aprobacion sin aprobar", tv3aCon(tv3aEn("t-l", "Tarea L", boardcmd.ColTuAprobacion), ttAccionGuardar(true, "", "x.go")),
			"Mirá ", ".", "plain de t-l", "en Tu aprobacion no hay aprobar el spec", tablero.GrupoTeToca},
		{"primera accion sin tecla", tv3aCon(tv3aEn("t-m", "Tarea M", boardcmd.ColWriter), ttRelanzar(), ttAccionWriter()),
			"Mirá ", ".", "plain de t-m", "en Writer no hay nada que hacer ahora: plain de t-m", tablero.GrupoTeToca},
		{"writer deshabilitado", tv3aCon(tv3aEn("t-n", "Tarea N", boardcmd.ColWriter), writerNo),
			"Mirá ", ".", "plain de t-n", "falta el espacio de trabajo", tablero.GrupoTeToca},
		{"detenida por un rojo", tv3aRoja(tv3aCon(tv3aEn("t-o", "Tarea O", boardcmd.ColReview), tv3aPedirRev()), "el veredicto quedo rojo", false),
			"Decidí cómo sigue ", ".", "el veredicto quedo rojo", "n pedir al reviewer", tablero.GrupoDetenida},
		{"detenida por un corte", tv3aCortada(tv3aCon(tv3aEn("t-p", "Tarea P", boardcmd.ColWriter), tv3aReanudar()), "writer"),
			"Decidí cómo sigue ", ".", "se corto el trabajo del writer", "el trabajo se corto: R reanuda o pide de nuevo", tablero.GrupoDetenida},
		{"detenida por un corte sin rol", tv3aCortada(tv3aEn("t-q", "Tarea Q", boardcmd.ColTestWriter), ""),
			"Decidí cómo sigue ", ".", "se corto el trabajo del agente", "el trabajo se corto: R reanuda o pide de nuevo", tablero.GrupoDetenida},
	}
}

// CA-698: la frase de la primera linea de la caja sale del paso de la
// tarjeta (la tabla del spec), con el titulo entre «», seguida de dos
// espacios y su plain (en DETENIDA, el de su rojo o el de su corte); la
// segunda linea es "n <etiqueta en minuscula>", o sin paso el texto de la
// regla del paso. En ASCII la frase pierde sus acentos y «» son ".
func TestV3A_CA698_FraseDeCadaPaso(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		for _, c := range tv3aPasoCasos() {
			que := fmt.Sprintf("%s (utf=%v)", c.que, utf)
			m := ttModelo(tv3aCfg(200, 30, utf, false), ttTablero(c.card))
			if m.Grupo(c.card.Slug) != c.grupo {
				t.Fatalf("CA-698: %s: la tarjeta esta en %q, esta en %q", que, c.grupo, m.Grupo(c.card.Slug))
			}
			if m.Caja() != c.card.Slug {
				t.Fatalf("CA-698: %s: la caja muestra %s; Caja() = %q", que, c.card.Slug, m.Caja())
			}
			frase := tv3aFijo(utf, c.antes) + tv3aQ(utf, c.card.Item.Titulo) + tv3aFijo(utf, c.despues)
			txt := tv3aCajaPaso(t, "CA-698", m, 200).tv3aTexto()
			if quiere := " " + frase + "  " + c.plain; txt[0] != quiere {
				t.Fatalf("CA-698: %s: la primera linea de la caja es\n%q\nes\n%q", que, quiere, txt[0])
			}
			if quiere := " " + c.izq; txt[1] != quiere {
				t.Fatalf("CA-698: %s: la segunda linea de la caja es\n%q\nes\n%q", que, quiere, txt[1])
			}
		}
	}
}

// CA-698: la caja muestra la primera tarjeta de TE TOCA en el orden de la
// lista (de Tu aceptacion a Arquitecto), aunque el tablero las traiga en
// otro orden; "despues:" lista las que siguen en ese orden, las de TE TOCA y
// despues las de DETENIDA, nunca las de CORRIENDO ni EN COLA.
func TestV3A_CA698_CajaPrimeraDeTeToca(t *testing.T) {
	defer tv3aSinPanico(t)
	b := ttTablero(
		tv3aCon(tv3aEn("a1", "A1", boardcmd.ColArquitecto), tv3aPedirArq()),
		tv3aCon(tv3aEn("b1", "B1", boardcmd.ColTuAprobacion), ttAccionAprobar()),
		tv3aCon(tv3aEn("c1", "C1", boardcmd.ColWriter), ttAccionWriter()),
		tv3aCon(tv3aEn("c2", "C2", boardcmd.ColWriter), ttAccionWriter()),
		tv3aCon(tv3aEn("e1", "E1", boardcmd.ColReview), tv3aPedirRev()),
		tv3aCon(tv3aEn("d1", "D1", boardcmd.ColTuAceptacion), ttAccionIntegrar()),
		tv3aCorre(tv3aEn("r1", "R1", boardcmd.ColWriter), "writer", time.Minute, 0),
		tv3aRoja(tv3aEn("x1", "X1", boardcmd.ColReview), "rojo de x1", false),
		tv3aCortada(tv3aEn("x2", "X2", boardcmd.ColTestWriter), "test-writer"),
		tv3aEn("q1", "Q1", boardcmd.ColBacklog),
	)
	m := ttModelo(tv3aCfg(200, 40, true, false), b)
	if m.Caja() != "d1" {
		t.Fatalf("CA-698: la caja muestra la primera de TE TOCA en el orden de la lista (d1, en Tu aceptacion); Caja() = %q", m.Caja())
	}
	txt := tv3aCajaPaso(t, "CA-698", m, 200).tv3aTexto()
	if !strings.HasPrefix(txt[0], " Aceptá «D1».  plain de d1") {
		t.Fatalf("CA-698: la primera linea de la caja es la de d1: %q", txt[0])
	}
	der := "después: «E1» · «C1» · «C2» · «B1» · «A1» · «X2» · «X1»"
	if quiere := tv3aConDerecha(" n integrar", der, 198); txt[1] != strings.TrimRight(quiere, " ") {
		t.Fatalf("CA-698: la segunda linea de la caja es\n%q\nes\n%q", strings.TrimRight(quiere, " "), txt[1])
	}
}

// CA-698: con TE TOCA vacia la caja muestra la primera de DETENIDA, en el
// orden del tablero, y "despues:" las otras de DETENIDA.
func TestV3A_CA698_CajaDetenidaSinTeToca(t *testing.T) {
	defer tv3aSinPanico(t)
	b := ttTablero(tv3aModulo(), tv3aPagos(), tv3aNotificaciones(), tv3aImportar(), ttViejo(1))
	m := ttModelo(tv3aCfg(200, 40, true, false), b)
	if m.Caja() != "pagos" {
		t.Fatalf("CA-698: sin TE TOCA la caja muestra la primera de DETENIDA (pagos); Caja() = %q", m.Caja())
	}
	txt := tv3aCajaPaso(t, "CA-698", m, 200).tv3aTexto()
	if quiere := " Decidí cómo sigue «Pagos con tarjeta».  se corto el trabajo del writer"; txt[0] != quiere {
		t.Fatalf("CA-698: la primera linea es\n%q\nes\n%q", quiere, txt[0])
	}
	quiere := strings.TrimRight(tv3aConDerecha(" el trabajo se corto: R reanuda o pide de nuevo", "después: «Notificaciones por correo»", 198), " ")
	if txt[1] != quiere {
		t.Fatalf("CA-698: la segunda linea es\n%q\nes\n%q", quiere, txt[1])
	}
}

// tv3aSegundaQuiere es la segunda linea de la caja por la regla de doc.go
// (decision del espacio): room = (w-2) - 1 - Celdas(izq) - 2 - 1; los
// titulos entran de a uno; uno que no entra entero se corta (con sus
// comillas) al lugar que deja, y no entran mas. ambiguo es true cuando ese
// lugar (comillas incluidas) es de 2 o 3 celdas: doc.go dice "when that is
// at least 2 cells" sin decir si cuenta las comillas, y ahi las dos lecturas
// difieren; los tests no miran esos anchos (duda).
func tv3aSegundaQuiere(utf bool, w int, izq string, titulos []string) (linea string, ambiguo bool) {
	room := (w - 2) - 1 - ttCeldas(izq) - 2 - 1
	der := tv3aFijo(utf, "después: ")
	n := 0
	for i, tit := range titulos {
		sep := ""
		if i > 0 {
			sep = " · "
		}
		if ttCeldas(der)+ttCeldas(sep)+ttCeldas(tv3aQ(utf, tit)) <= room {
			der += sep + tv3aQ(utf, tit)
			n++
			continue
		}
		queda := room - ttCeldas(der) - ttCeldas(sep)
		if queda >= 2 && queda <= 3 {
			ambiguo = true
		}
		if queda-2 >= 2 {
			der += sep + tv3aQ(utf, tv3aCortar(tit, queda-2, utf))
			n++
		}
		break
	}
	if n == 0 {
		return " " + izq, ambiguo
	}
	return strings.TrimRight(tv3aConDerecha(" "+izq, der, w-2), " "), ambiguo
}

// CA-698: a todo ancho de 60 a 140, "despues:" lleva los titulos que entran
// y corta con …/~ el que no entra entero; la izquierda ("n pedir al
// writer") no se corta.
func TestV3A_CA698_DespuesLoQueEntra(t *testing.T) {
	defer tv3aSinPanico(t)
	tits := []string{"Exportar reporte a PDF", "Pagos con tarjeta", "Notificaciones por correo"}
	for _, utf := range []bool{false, true} {
		for w := 60; w <= 140; w++ {
			m := tv3aModeloDorado(tv3aCfg(w, 20, utf, false))
			txt := tv3aCajaPaso(t, "CA-698", m, w).tv3aTexto()
			quiere, ambiguo := tv3aSegundaQuiere(utf, w, "n pedir al writer", tits)
			if !ambiguo && txt[1] != quiere {
				t.Fatalf("CA-698: a %d columnas (utf=%v) la segunda linea de la caja es\n%q\nes\n%q", w, utf, quiere, txt[1])
			}
			frase := tv3aFijo(utf, "Pedile el código al writer para ") + tv3aQ(utf, "Listado de proyectos") + "."
			if quiere := strings.TrimRight(tv3aCortar(" "+frase+"  los tests fallan: falta el codigo", w-2, utf), " "); txt[0] != quiere {
				t.Fatalf("CA-698: a %d columnas (utf=%v) la primera linea de la caja, cortada a w-2, es\n%q\nes\n%q", w, utf, quiere, txt[0])
			}
		}
	}
}

// CA-698: sin otras tarjetas en TE TOCA ni DETENIDA no hay "despues:".
func TestV3A_CA698_SinOtrasNoHayDespues(t *testing.T) {
	defer tv3aSinPanico(t)
	m := ttModelo(tv3aCfg(120, 30, true, false), ttTablero(tv3aExportar(), tv3aModulo(), tv3aImportar(), ttViejo(1)))
	txt := tv3aCajaPaso(t, "CA-698", m, 120).tv3aTexto()
	if txt[1] != " n aprobar spec" {
		t.Fatalf("CA-698: con una sola tarjeta en TE TOCA y DETENIDA la segunda linea es \" n aprobar spec\", es %q", txt[1])
	}
}

// CA-698: con color la frase va en negrita (1), y el plain y "despues:" en
// gris (90).
func TestV3A_CA698_CajaColores(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(140, 30, true, true))
	tv3aSgrDe(t, "CA-698", "la frase", m, 3, "Pedile el código al writer para «Listado de proyectos».", 0, "1")
	tv3aSgrDe(t, "CA-698", "el plain", m, 3, "los tests fallan: falta el codigo", 0, "90")
	tv3aSgrDe(t, "CA-698", "despues", m, 4, "después: «Exportar reporte a PDF» · «Pagos con tarjeta» · «Notificaciones por correo»", 0, "90")
}

// CA-698 (decision de doc.go): la caja cuenta y elige entre todas las
// tarjetas del tablero: el filtro de la busqueda no la cambia.
func TestV3A_CA698_CajaIgnoraLaBusqueda(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(120, 30, true, false))
	m.Tecla("/", ttAhora)
	ttEscribir(m, "importar")
	if m.Caja() != "listado" {
		t.Fatalf("CA-698: con la busqueda filtrando, la caja sigue en listado; Caja() = %q", m.Caja())
	}
	paso := tv3aCajaPaso(t, "CA-698", m, 120)
	if paso.Arriba != tv3aArriba(true, false, 120, "TU SIGUIENTE PASO", "te toca 2 · corriendo 1 · detenida 2") {
		t.Fatalf("CA-698: con la busqueda los numeros de la caja son los de todo el tablero: %q", paso.Arriba)
	}
}

// CA-698 (caso limite): si la tarjeta de la caja desaparece en una lectura,
// la caja pasa a la siguiente con la regla de siempre.
func TestV3A_CA698_CajaCuandoDesapareceSuTarjeta(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(120, 30, true, false))
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(tv3aImportar(), tv3aExportar(), tv3aModulo(), tv3aPagos(),
		tv3aNotificaciones())})
	if m.Caja() != "exportar-pdf" || !strings.HasPrefix(tv3aDe(tv3aCajaPaso(t, "CA-698", m, 120).tv3aTexto(), 0), " Firmá el spec de «Exportar reporte a PDF».") {
		t.Fatalf("CA-698: sin listado la caja pasa a exportar-pdf; Caja() = %q\n%s", m.Caja(), tv3aLineas(m))
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: ttTablero(tv3aImportar(), tv3aModulo(), tv3aPagos(), tv3aNotificaciones())})
	if m.Caja() != "pagos" || !strings.HasPrefix(tv3aDe(tv3aCajaPaso(t, "CA-698", m, 120).tv3aTexto(), 0), " Decidí cómo sigue «Pagos con tarjeta».") {
		t.Fatalf("CA-698: sin TE TOCA la caja pasa a la primera de DETENIDA; Caja() = %q\n%s", m.Caja(), tv3aLineas(m))
	}
}

// CA-698, CA-701 (caso limite): un titulo vacio: la caja y la fila usan el
// slug en su lugar, tambien en "despues:".
func TestV3A_CA698_CA701_TituloVacioUsaElSlug(t *testing.T) {
	defer tv3aSinPanico(t)
	a := tv3aCon(tv3aEn("sin-titulo", "", boardcmd.ColTuAceptacion), ttAccionIntegrar())
	b := tv3aCon(tv3aEn("otra-sin-titulo", "", boardcmd.ColWriter), ttAccionWriter())
	for _, w := range []int{120, 80} {
		m := ttModelo(tv3aCfg(w, 30, true, false), ttTablero(a, b))
		txt := tv3aCajaPaso(t, "CA-698", m, w).tv3aTexto()
		if !strings.HasPrefix(txt[0], " Aceptá «sin-titulo».") || !strings.HasSuffix(txt[1], "después: «otra-sin-titulo»") {
			t.Fatalf("CA-698: a %d columnas, sin titulo la caja usa el slug:\n%s", w, tv3aLineas(m))
		}
		li := tv3aLista(t, "CA-701", m, w).tv3aTexto()
		otra := 2 // en una columna cada tarjeta es una linea; con dos zonas, dos
		if w >= 100 {
			otra = 3
		}
		if !strings.HasPrefix(tv3aDe(li, 1), " ▸ sin-titulo ") || !strings.HasPrefix(tv3aDe(li, otra), "   otra-sin-titulo ") {
			t.Fatalf("CA-701: a %d columnas, sin titulo la fila usa el slug:\n%s", w, tv3aLineas(m))
		}
	}
}

// CA-698 (hostil): los controles de un titulo (ESC, un bidi) no llegan a la
// terminal: son "?"; un titulo que no entra corta la linea con …; ninguna
// linea pasa del ancho.
func TestV3A_CA698_TituloHostilEnLaCaja(t *testing.T) {
	defer tv3aSinPanico(t)
	c := tv3aCon(tv3aEn("hostil", "Malo\x1b[31m\u202eX "+strings.Repeat("larguisimo ", 20), boardcmd.ColTuAprobacion), ttAccionAprobar())
	m := ttModelo(tv3aCfg(80, 24, true, false), ttTablero(c))
	ttForma(t, "CA-698", m, 80, 24, false)
	txt := tv3aCajaPaso(t, "CA-698", m, 80).tv3aTexto()
	if !strings.HasPrefix(txt[0], " Firmá el spec de «Malo?[31m?X larguisimo") || !strings.HasSuffix(txt[0], "…") || ttCeldas(txt[0]) != 78 {
		t.Fatalf("CA-698: un titulo con controles y largo: los controles son ? y la linea se corta a 78 celdas con …: %q", txt[0])
	}
}

// ================================================================ CA-699: el borde y la caja sin tarjeta

// CA-699: el borde de arriba dice TU SIGUIENTE PASO a la izquierda y "te
// toca <a> · corriendo <b> · detenida <c>" a la derecha, siempre con los tres
// numeros.
func TestV3A_CA699_BordeDeLaCaja(t *testing.T) {
	defer tv3aSinPanico(t)
	casos := []struct {
		b    boardcmd.Board
		dice string
	}{
		{tv3aTableroDorado(), "te toca 2 · corriendo 1 · detenida 2"},
		{ttTablero(tv3aModulo(), ttViejo(1)), "te toca 0 · corriendo 1 · detenida 0"},
		{ttTablero(), "te toca 0 · corriendo 0 · detenida 0"},
		{ttTablero(tv3aPagos()), "te toca 0 · corriendo 0 · detenida 1"},
	}
	for _, utf := range []bool{true, false} {
		for _, w := range []int{60, 80, 100, 160} {
			for _, c := range casos {
				m := ttModelo(tv3aCfg(w, 20, utf, false), c.b)
				p := tv3aCajaPaso(t, "CA-699", m, w)
				if quiere := tv3aArriba(utf, false, w, "TU SIGUIENTE PASO", c.dice); p.Arriba != quiere {
					t.Fatalf("CA-699: a %d (utf=%v) el borde de arriba es\n%q\nes\n%q", w, utf, quiere, p.Arriba)
				}
				if quiere := tv3aAbajoDe(utf, false, w, ""); p.Abajo != quiere {
					t.Fatalf("CA-699: a %d (utf=%v) el borde de abajo es\n%q\nes\n%q", w, utf, quiere, p.Abajo)
				}
			}
		}
	}
}

// tv3aBordeDeColor exige que todo el borde de la caja (las lineas 2 y 5
// enteras y los lados de las 3 y 4) tenga los codigos sgr.
func tv3aBordeDeColor(t *testing.T, que string, m *tablero.Modelo, w int, sgr string) {
	t.Helper()
	tv3aSgrTramo(t, "CA-699", que+": el borde de arriba", m, 2, 0, w, sgr)
	tv3aSgrTramo(t, "CA-699", que+": el borde de abajo", m, 5, 0, w, sgr)
	for _, n := range []int{3, 4} {
		tv3aSgrTramo(t, "CA-699", que+": el lado izquierdo", m, n, 0, 1, sgr)
		tv3aSgrTramo(t, "CA-699", que+": el lado derecho", m, n, w-1, w, sgr)
	}
}

// CA-699: con color, el borde de la caja y su titulo (esquinas, lineas,
// lados, titulo y numeros) van en amarillo (33) cuando TE TOCA o DETENIDA
// tienen alguna tarjeta, y en gris (90) cuando no (tambien antes de la
// primera lectura).
func TestV3A_CA699_BordeAmarilloOGris(t *testing.T) {
	defer tv3aSinPanico(t)
	casos := []struct {
		que string
		b   *boardcmd.Board
		sgr string
	}{
		{"con TE TOCA", ptrB(tv3aTableroDorado()), "33"},
		{"solo DETENIDA", ptrB(ttTablero(tv3aPagos(), tv3aModulo())), "33"},
		{"solo CORRIENDO", ptrB(ttTablero(tv3aModulo(), ttViejo(1))), "90"},
		{"solo EN COLA", ptrB(ttTablero(tv3aImportar())), "90"},
		{"sin tarjetas", ptrB(ttTablero()), "90"},
		{"solo HECHO", ptrB(ttTablero(ttViejo(1), ttViejo(2))), "90"},
		{"antes de la primera lectura", nil, "90"},
	}
	for _, c := range casos {
		m := tablero.Nuevo(tv3aCfg(100, 24, true, true))
		if c.b != nil {
			m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: *c.b})
		}
		tv3aBordeDeColor(t, c.que, m, 100, c.sgr)
	}
}

func ptrB(b boardcmd.Board) *boardcmd.Board { return &b }

// CA-699 (todo corre solo): sin tarjetas en TE TOCA ni DETENIDA y con alguna
// en CORRIENDO la caja dice "Nada te espera. hoom trabaja en <n>
// tarjeta(s)." y "corriendo: «T» (<rol>, <tiempo>)" de la primera, con " y
// <k> más"; no muestra tarjeta (Caja es "").
func TestV3A_CA699_NadaTeEspera(t *testing.T) {
	defer tv3aSinPanico(t)
	otra := tv3aCorre(tv3aEn("otra", "Otra que corre", boardcmd.ColReview), "reviewer", time.Minute+5*time.Second, 0)
	for _, utf := range []bool{true, false} {
		m := ttModelo(tv3aCfg(120, 30, utf, false), ttTablero(tv3aModulo(), otra, ttViejo(1)))
		txt := tv3aCajaPaso(t, "CA-699", m, 120).tv3aTexto()
		if txt[0] != " Nada te espera. hoom trabaja en 2 tarjetas." {
			t.Fatalf("CA-699 (utf=%v): con dos corriendo la primera linea es \" Nada te espera. hoom trabaja en 2 tarjetas.\", es %q", utf, txt[0])
		}
		if quiere := " corriendo: " + tv3aQ(utf, "Módulo de usuarios") + " (writer, 14:02)" + tv3aFijo(utf, " y 1 más"); txt[1] != quiere {
			t.Fatalf("CA-699 (utf=%v): la segunda linea es\n%q\nes\n%q", utf, quiere, txt[1])
		}
		if m.Caja() != "" {
			t.Fatalf("CA-699: sin TE TOCA ni DETENIDA la caja no muestra tarjeta; Caja() = %q", m.Caja())
		}
		m = ttModelo(tv3aCfg(120, 30, utf, false), ttTablero(tv3aModulo()))
		txt = tv3aCajaPaso(t, "CA-699", m, 120).tv3aTexto()
		if txt[0] != " Nada te espera. hoom trabaja en 1 tarjeta." || txt[1] != " corriendo: "+tv3aQ(utf, "Módulo de usuarios")+" (writer, 14:02)" {
			t.Fatalf("CA-699 (utf=%v): con una corriendo la caja dice \"1 tarjeta\" y no \" y 0 más\":\n%s", utf, tv3aLineas(m))
		}
	}
}

// CA-699 (nada empezado y nada en curso): sin TE TOCA, DETENIDA ni
// CORRIENDO y con alguna en EN COLA, la caja ofrece la primera de EN COLA:
// "Nada en curso. Podés empezar «T»." y "n <su paso>"; Caja es "".
func TestV3A_CA699_NadaEnCurso(t *testing.T) {
	defer tv3aSinPanico(t)
	otra := tv3aCon(tv3aEn("otra-cola", "Otra en cola", boardcmd.ColBacklog), tv3aPedirArq())
	for _, utf := range []bool{true, false} {
		m := ttModelo(tv3aCfg(120, 30, utf, false), ttTablero(tv3aImportar(), otra, ttViejo(1)))
		txt := tv3aCajaPaso(t, "CA-699", m, 120).tv3aTexto()
		if quiere := " " + tv3aFijo(utf, "Nada en curso. Podés empezar ") + tv3aQ(utf, "Importar desde Excel") + "."; txt[0] != quiere {
			t.Fatalf("CA-699 (utf=%v): la primera linea es\n%q\nes\n%q", utf, quiere, txt[0])
		}
		if txt[1] != " n pedir al arquitecto" {
			t.Fatalf("CA-699 (utf=%v): la segunda linea es \" n pedir al arquitecto\", es %q", utf, txt[1])
		}
		if m.Caja() != "" {
			t.Fatalf("CA-699: cuando la caja ofrece la primera de EN COLA, Caja() es \"\"; es %q", m.Caja())
		}
		if got := tv3aCajaPaso(t, "CA-699", m, 120).Arriba; got != tv3aArriba(utf, false, 120, "TU SIGUIENTE PASO", "te toca 0 · corriendo 0 · detenida 0") {
			t.Fatalf("CA-699: los tres numeros son 0: %q", got)
		}
	}
}

// CA-699, CA-578 (sin tarjetas): la caja dice "No hay nada pendiente." y
// `para sumar trabajo: hoom item add "<titulo>"`, y la lista `no hay
// tarjetas: hoom item add "<titulo>"`, como en la v2, partido en sus
// palabras cuando no entra en una linea (la caja vacia, doc.go); con solo
// tarjetas en HECHO, la caja dice lo mismo.
func TestV3A_CA699_NoHayNadaPendiente(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, b := range []boardcmd.Board{ttTablero(), ttTablero(ttViejo(1), ttViejo(2))} {
		m := ttModelo(tv3aCfg(100, 24, true, false), b)
		txt := tv3aCajaPaso(t, "CA-699", m, 100).tv3aTexto()
		if txt[0] != " No hay nada pendiente." || txt[1] != ` para sumar trabajo: hoom item add "<titulo>"` {
			t.Fatalf("CA-699: sin nada pendiente la caja dice \"No hay nada pendiente.\" y como sumar trabajo:\n%s", tv3aLineas(m))
		}
		if m.Caja() != "" {
			t.Fatalf("CA-699: Caja() es \"\", es %q", m.Caja())
		}
	}
	// a 120 la lista tiene 46 celdas por dentro y el texto (42) entra en una
	// linea; a 100 tiene 38 y se parte en sus palabras, la continuacion con
	// su margen (las dos lineas entran en el alto interior, 14)
	casos := []struct {
		w      int
		lineas []string
	}{
		{120, []string{` no hay tarjetas: hoom item add "<titulo>"`, ""}},
		{100, []string{" no hay tarjetas: hoom item add", ` "<titulo>"`, ""}},
	}
	for _, c := range casos {
		m := ttModelo(tv3aCfg(c.w, 24, true, false), ttTablero())
		li := tv3aLista(t, "CA-699", m, c.w).tv3aTexto()
		for i, q := range c.lineas {
			if tv3aDe(li, i) != q {
				t.Fatalf("CA-699, CA-578: a %d sin tarjetas la lista dice ` no hay tarjetas: hoom item add \"<titulo>\"`, partido si no entra; la linea %d es %q, deberia ser %q: %q",
					c.w, i, tv3aDe(li, i), q, li)
			}
		}
	}
}

// CA-699: antes de la primera lectura del tablero la caja dice "leyendo el
// tablero..." en su primera linea y nada en la segunda, y su borde no tiene
// numeros.
func TestV3A_CA699_AntesDeLaPrimeraLectura(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		m := tablero.Nuevo(tv3aCfg(90, 20, utf, false))
		p := tv3aCajaPaso(t, "CA-699", m, 90)
		if p.Arriba != tv3aArriba(utf, false, 90, "TU SIGUIENTE PASO", "") {
			t.Fatalf("CA-699: antes de leer el borde no tiene numeros: %q", p.Arriba)
		}
		if txt := p.tv3aTexto(); txt[0] != " leyendo el tablero..." || txt[1] != "" {
			t.Fatalf("CA-699: antes de leer la caja dice \" leyendo el tablero...\" y nada mas: %q", txt)
		}
		if m.Caja() != "" {
			t.Fatalf("CA-699: antes de leer Caja() es \"\"")
		}
	}
}

// CA-699: con color, la frase de la caja sin tarjeta va en negrita.
func TestV3A_CA699_FraseEnNegrita(t *testing.T) {
	defer tv3aSinPanico(t)
	m := ttModelo(tv3aCfg(100, 24, true, true), ttTablero(tv3aModulo()))
	tv3aSgrDe(t, "CA-699", "Nada te espera", m, 3, "Nada te espera. hoom trabaja en 1 tarjeta.", 0, "1")
	m = ttModelo(tv3aCfg(100, 24, true, true), ttTablero())
	tv3aSgrDe(t, "CA-699", "No hay nada pendiente", m, 3, "No hay nada pendiente.", 0, "1")
}

// ================================================================ CA-700: los grupos

// CA-700, CA-555: cada tarjeta cae en un solo grupo, el primero que se
// cumple: HECHO (columna Hecho), CORRIENDO (running), DETENIDA (interrupted,
// o red con esperado en false), EN COLA (Backlog), TE TOCA (las demas).
func TestV3A_CA700_CA555_GrupoDeCadaTarjeta(t *testing.T) {
	defer tv3aSinPanico(t)
	casos := []struct {
		que  string
		c    boardcmd.Card
		quie tablero.Grupo
	}{
		{"Hecho con running", tv3aCorre(tv3aEn("x", "X", boardcmd.ColHecho), "writer", time.Minute, 0), tablero.GrupoHecho},
		{"Hecho con rojo", tv3aRoja(tv3aEn("x", "X", boardcmd.ColHecho), "r", false), tablero.GrupoHecho},
		{"Hecho con corte", tv3aCortada(tv3aEn("x", "X", boardcmd.ColHecho), "writer"), tablero.GrupoHecho},
		{"running y rojo", tv3aRoja(tv3aCorre(tv3aEn("x", "X", boardcmd.ColWriter), "writer", time.Minute, 0), "r", false), tablero.GrupoCorriendo},
		{"running y corte", tv3aCortada(tv3aCorre(tv3aEn("x", "X", boardcmd.ColWriter), "writer", time.Minute, 0), "writer"), tablero.GrupoCorriendo},
		{"running en Backlog", tv3aCorre(tv3aEn("x", "X", boardcmd.ColBacklog), "arquitecto", time.Minute, 0), tablero.GrupoCorriendo},
		{"running en Tu aprobacion", tv3aCorre(tv3aEn("x", "X", boardcmd.ColTuAprobacion), "writer", time.Minute, 0), tablero.GrupoCorriendo},
		{"corte", tv3aCortada(tv3aEn("x", "X", boardcmd.ColWriter), "writer"), tablero.GrupoDetenida},
		{"corte en Backlog", tv3aCortada(tv3aEn("x", "X", boardcmd.ColBacklog), "arquitecto"), tablero.GrupoDetenida},
		{"rojo no esperado", tv3aRoja(tv3aEn("x", "X", boardcmd.ColReview), "r", false), tablero.GrupoDetenida},
		{"rojo no esperado en Backlog", tv3aRoja(tv3aEn("x", "X", boardcmd.ColBacklog), "r", false), tablero.GrupoDetenida},
		{"rojo no esperado en Tu aceptacion", tv3aRoja(tv3aEn("x", "X", boardcmd.ColTuAceptacion), "r", false), tablero.GrupoDetenida},
		{"rojo esperado en Writer", tv3aRoja(tv3aEn("x", "X", boardcmd.ColWriter), "r", true), tablero.GrupoTeToca},
		{"rojo esperado en Backlog", tv3aRoja(tv3aEn("x", "X", boardcmd.ColBacklog), "r", true), tablero.GrupoEnCola},
		{"Backlog", tv3aEn("x", "X", boardcmd.ColBacklog), tablero.GrupoEnCola},
		{"Arquitecto", tv3aEn("x", "X", boardcmd.ColArquitecto), tablero.GrupoTeToca},
		{"Tu aprobacion", tv3aEn("x", "X", boardcmd.ColTuAprobacion), tablero.GrupoTeToca},
		{"Test-writer", tv3aEn("x", "X", boardcmd.ColTestWriter), tablero.GrupoTeToca},
		{"Writer sin acciones", tv3aEn("x", "X", boardcmd.ColWriter), tablero.GrupoTeToca},
		{"Review", tv3aEn("x", "X", boardcmd.ColReview), tablero.GrupoTeToca},
		{"Tu aceptacion", tv3aEn("x", "X", boardcmd.ColTuAceptacion), tablero.GrupoTeToca},
		{"Hecho", tv3aEn("x", "X", boardcmd.ColHecho), tablero.GrupoHecho},
	}
	for _, c := range casos {
		m := ttModelo(tv3aCfg(120, 30, true, false), ttTablero(c.c))
		if got := m.Grupo("x"); got != c.quie {
			t.Fatalf("CA-700: %s: el grupo es %q, es %q", c.que, c.quie, got)
		}
	}
	m := ttModelo(tv3aCfg(120, 30, true, false), tv3aTableroDorado())
	if m.Grupo("no-existe") != "" {
		t.Fatalf("CA-700: una tarjeta que no esta en el tablero no tiene grupo")
	}
	if tablero.Nuevo(tv3aCfg(120, 30, true, false)).Grupo("listado") != "" {
		t.Fatalf("CA-700: antes de la primera lectura ninguna tarjeta tiene grupo")
	}
}

// tv3aTableroRevuelto: tarjetas en todas las columnas, en un orden del
// tablero que no es el de la lista.
func tv3aTableroRevuelto() boardcmd.Board {
	return ttTablero(
		tv3aCon(tv3aEn("a-arq", "Arq uno", boardcmd.ColArquitecto), tv3aPedirArq()),
		tv3aCon(tv3aEn("b-apr", "Apr uno", boardcmd.ColTuAprobacion), ttAccionAprobar()),
		tv3aEn("c-tw", "TW uno", boardcmd.ColTestWriter),
		tv3aEn("d-w1", "Wri uno", boardcmd.ColWriter),
		tv3aRoja(tv3aEn("d-w2", "Wri dos", boardcmd.ColWriter), "esperado", true),
		tv3aEn("e-rev", "Rev uno", boardcmd.ColReview),
		tv3aEn("f-acp", "Acp uno", boardcmd.ColTuAceptacion),
		tv3aCorre(tv3aEn("g-run", "Corre uno", boardcmd.ColTestWriter), "test-writer", time.Minute, 0),
		tv3aCorre(tv3aEn("h-run", "Corre dos", boardcmd.ColArquitecto), "arquitecto", time.Minute, 0),
		tv3aCortada(tv3aEn("i-cut", "Cortada", boardcmd.ColReview), "reviewer"),
		tv3aRoja(tv3aEn("j-red", "Roja", boardcmd.ColWriter), "rojo", false),
		tv3aEn("k-bl1", "Cola uno", boardcmd.ColBacklog),
		tv3aEn("k-bl2", "Cola dos", boardcmd.ColBacklog),
		tv3aEn("z-h1", "Hecha uno", boardcmd.ColHecho),
		tv3aEn("z-h2", "Hecha dos", boardcmd.ColHecho),
	)
}

// tv3aOrdenRevuelto es la lista esperada de tv3aTableroRevuelto: las lineas
// de grupo tal cual y las tarjetas por su titulo.
var tv3aOrdenRevuelto = []string{
	"#TE TOCA 7", "Acp uno", "Rev uno", "Wri uno", "Wri dos", "TW uno", "Apr uno", "Arq uno",
	"#CORRIENDO 2", "Corre dos", "Corre uno",
	"#DETENIDA 2", "Roja", "Cortada",
	"#EN COLA 2", "Cola uno", "Cola dos",
	"#HECHO 2 ▸",
}

// tv3aRevisarOrden exige que el contenido de la lista (sin las lineas de
// estado de las filas anchas) sea el de quiere: "#..." una linea de grupo
// exacta, lo demas una fila cuyo titulo empieza en la celda 3.
func tv3aRevisarOrden(t *testing.T, que string, li []string, quiere []string, ancha bool) {
	t.Helper()
	var filas []string
	for i := 0; i < len(li); i++ {
		l := li[i]
		if l == "" {
			continue
		}
		filas = append(filas, l)
		if ancha && (strings.HasPrefix(l, "   ") || strings.HasPrefix(l, " ▸ ") || strings.HasPrefix(l, " > ")) {
			i++ // la segunda linea de la tarjeta
		}
	}
	if len(filas) != len(quiere) {
		t.Fatalf("CA-700: %s: la lista tiene %d lineas de grupo y de tarjeta (%q), tiene %d:\n%s",
			que, len(quiere), quiere, len(filas), strings.Join(li, "\n"))
	}
	for i, q := range quiere {
		if strings.HasPrefix(q, "#") {
			if filas[i] != " "+q[1:] {
				t.Fatalf("CA-700: %s: la linea %d de la lista es la del grupo %q, es %q:\n%s", que, i+1, " "+q[1:], filas[i], strings.Join(li, "\n"))
			}
			continue
		}
		if c := ttCeldasDe(filas[i]); len(c) < 3 || !strings.HasPrefix(strings.Join(c[3:], ""), q+" ") {
			t.Fatalf("CA-700: %s: la linea %d de la lista es la fila de %q, es %q:\n%s", que, i+1, q, filas[i], strings.Join(li, "\n"))
		}
	}
}

// CA-700, CA-555: la lista va en el orden TE TOCA, CORRIENDO, DETENIDA, EN
// COLA y HECHO; en TE TOCA por columna de Tu aceptacion a Arquitecto y,
// dentro de una columna (Writer: Wri uno, Wri dos) y en los otros grupos, en
// el orden del tablero; HECHO empieza cerrado. En una columna y en dos.
func TestV3A_CA700_CA555_OrdenDeLaLista(t *testing.T) {
	defer tv3aSinPanico(t)
	m := ttModelo(tv3aCfg(99, 60, true, false), tv3aTableroRevuelto())
	tv3aRevisarOrden(t, "una columna", tv3aLista(t, "CA-700", m, 99).tv3aTexto(), tv3aOrdenRevuelto, false)
	m = ttModelo(tv3aCfg(140, 60, true, false), tv3aTableroRevuelto())
	tv3aRevisarOrden(t, "dos zonas", tv3aLista(t, "CA-700", m, 140).tv3aTexto(), tv3aOrdenRevuelto, true)
}

// CA-700, CA-555: no hay grupos vacios, salvo HECHO, que siempre esta.
func TestV3A_CA700_CA555_SinGruposVacios(t *testing.T) {
	defer tv3aSinPanico(t)
	m := ttModelo(tv3aCfg(99, 40, true, false), ttTablero(tv3aImportar()))
	li := tv3aLista(t, "CA-700", m, 99).tv3aTexto()
	tv3aRevisarOrden(t, "solo EN COLA", li, []string{"#EN COLA 1", "Importar desde Excel", "#HECHO 0 ▸"}, false)
	for _, g := range []string{"TE TOCA", "CORRIENDO", "DETENIDA"} {
		for _, l := range li {
			if strings.Contains(l, g) {
				t.Fatalf("CA-700: un grupo vacio (%s) no se muestra:\n%s", g, strings.Join(li, "\n"))
			}
		}
	}
}

// tv3aGrupoDe es el grupo de una tarjeta por la regla del spec, calculada
// aca (sin pilotos).
func tv3aGrupoDe(c boardcmd.Card) tablero.Grupo {
	switch {
	case c.Column == boardcmd.ColHecho:
		return tablero.GrupoHecho
	case c.Running != nil:
		return tablero.GrupoCorriendo
	case c.Interrupted != nil || c.Red != nil && !c.Red.Esperado:
		return tablero.GrupoDetenida
	case c.Column == boardcmd.ColBacklog:
		return tablero.GrupoEnCola
	}
	return tablero.GrupoTeToca
}

// CA-700, CA-555 (propiedad): en tableros al azar cada tarjeta cae en el
// grupo de la regla, la lista la muestra una sola vez bajo la linea de su
// grupo (salvo HECHO, cerrado), los numeros de cada linea de grupo y los de
// la caja cuentan sus tarjetas y el orden de los grupos es siempre el mismo.
// La seleccion esta en la primera fila de tarjeta, o en la linea de HECHO
// cuando no hay ninguna.
func TestV3A_CA700_CA555_PropiedadUnSoloGrupo(t *testing.T) {
	defer tv3aSinPanico(t)
	orden := []tablero.Grupo{tablero.GrupoTeToca, tablero.GrupoCorriendo, tablero.GrupoDetenida, tablero.GrupoEnCola, tablero.GrupoHecho}
	f := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		n := r.Intn(22)
		var cards []boardcmd.Card
		cuenta := map[tablero.Grupo]int{}
		for i := 0; i < n; i++ {
			col := boardcmd.Columns[r.Intn(8)].ID
			c := tv3aEn(fmt.Sprintf("t%02d", i), fmt.Sprintf("Tarjeta %02d", i), col)
			if r.Intn(4) == 0 {
				c = tv3aCorre(c, "writer", time.Minute, 0)
			}
			if r.Intn(4) == 0 {
				c = tv3aCortada(c, "writer")
			}
			if r.Intn(3) == 0 {
				c = tv3aRoja(c, "rojo", r.Intn(2) == 0)
			}
			cards = append(cards, c)
			cuenta[tv3aGrupoDe(c)]++
		}
		m := ttModelo(tv3aCfg(99, 120, true, false), ttTablero(cards...))
		for _, c := range cards {
			if got := m.Grupo(c.Slug); got != tv3aGrupoDe(c) {
				t.Logf("CA-700: semilla %d: %s (columna %s, running %v, corte %v, rojo %+v) esta en %q, deberia estar en %q",
					semilla, c.Slug, c.Column, c.Running != nil, c.Interrupted != nil, c.Red, got, tv3aGrupoDe(c))
				return false
			}
		}
		var quiere []string
		for _, g := range orden {
			if g == tablero.GrupoHecho {
				// la seleccion empieza en la primera fila de tarjeta; sin
				// ninguna (todas en HECHO, cerrado) esta en la linea de HECHO,
				// que elegida es " ▸ HECHO n ▸" (doc.go, la lista)
				if cuenta[g] == n {
					quiere = append(quiere, fmt.Sprintf("#▸ HECHO %d ▸", cuenta[g]))
				} else {
					quiere = append(quiere, fmt.Sprintf("#HECHO %d ▸", cuenta[g]))
				}
				continue
			}
			if cuenta[g] == 0 {
				continue
			}
			quiere = append(quiere, fmt.Sprintf("#%s %d", g, cuenta[g]))
			// el orden dentro del grupo: TE TOCA por columna de atras para
			// adelante, los demas en el orden del tablero
			var gs []boardcmd.Card
			if g == tablero.GrupoTeToca {
				for k := 7; k >= 0; k-- {
					for _, b := range ttTablero(cards...).Columns[k].Cards {
						if tv3aGrupoDe(b) == g {
							gs = append(gs, b)
						}
					}
				}
			} else {
				for _, col := range ttTablero(cards...).Columns {
					for _, b := range col.Cards {
						if tv3aGrupoDe(b) == g {
							gs = append(gs, b)
						}
					}
				}
			}
			for _, b := range gs {
				quiere = append(quiere, b.Item.Titulo)
			}
		}
		li := tv3aLista(t, "CA-700", m, 99).tv3aTexto()
		var filas []string
		for _, l := range li {
			if l != "" {
				filas = append(filas, l)
			}
		}
		if n == 0 {
			return true // sin tarjetas la lista dice "no hay tarjetas" (CA-578)
		}
		if len(filas) != len(quiere) {
			t.Logf("CA-700: semilla %d: la lista es %q, deberia ser %q", semilla, filas, quiere)
			return false
		}
		for i, q := range quiere {
			if strings.HasPrefix(q, "#") {
				if filas[i] != " "+q[1:] {
					t.Logf("CA-700: semilla %d: linea %d %q, deberia ser %q", semilla, i+1, filas[i], " "+q[1:])
					return false
				}
			} else if c := ttCeldasDe(filas[i]); len(c) < 3 || !strings.HasPrefix(strings.Join(c[3:], ""), q+" ") {
				t.Logf("CA-700: semilla %d: linea %d %q, deberia ser la fila de %q", semilla, i+1, filas[i], q)
				return false
			}
		}
		arriba := tv3aCajaPaso(t, "CA-700", m, 99).Arriba
		dice := fmt.Sprintf("te toca %d · corriendo %d · detenida %d", cuenta[tablero.GrupoTeToca], cuenta[tablero.GrupoCorriendo],
			cuenta[tablero.GrupoDetenida])
		if arriba != tv3aArriba(true, false, 99, "TU SIGUIENTE PASO", dice) {
			t.Logf("CA-700: semilla %d: la caja cuenta %q: %q", semilla, dice, arriba)
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300, Rand: rand.New(rand.NewSource(700))}); err != nil {
		t.Fatalf("CA-700: %v", err)
	}
}

// CA-700, CA-555: con color, el nombre de TE TOCA va en video inverso
// amarillo (7;33), el de CORRIENDO en cian (7;36), el de DETENIDA en rojo
// (7;31), y EN COLA y HECHO en gris (90); el numero y el signo de HECHO no
// tienen estilo.
func TestV3A_CA700_CA555_ColoresDeLosGrupos(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(120, 36, true, true))
	casos := []struct{ nombre, resto, sgr string }{
		{"TE TOCA", " 2", "7;33"}, {"CORRIENDO", " 1", "7;36"}, {"DETENIDA", " 2", "7;31"}, {"EN COLA", " 1", "90"}, {"HECHO", " 2 ▸", "90"},
	}
	for _, c := range casos {
		n := 0
		for k := 6; k <= 33; k++ {
			if tv3aPosEn(m, k, " "+c.nombre+c.resto, 1) == 1 {
				n = k
				break
			}
		}
		if n == 0 {
			t.Fatalf("CA-700: la lista tiene la linea \" %s%s\":\n%s", c.nombre, c.resto, tv3aLineas(m))
		}
		tv3aSgrTramo(t, "CA-700", "el nombre de "+c.nombre, m, n, 2, 2+ttCeldas(c.nombre), c.sgr)
		tv3aSgrTramo(t, "CA-700", "el numero de "+c.nombre, m, n, 2+ttCeldas(c.nombre), 2+ttCeldas(c.nombre+c.resto), "")
	}
}

// CA-700, CA-555: HECHO empieza cerrado (" HECHO 2 ▸"); su linea se elige
// (" ▸ HECHO 2 ▸") y enter la abre (" ▸ HECHO 2 ▾", con sus tarjetas
// debajo) y la cierra.
func TestV3A_CA700_CA555_HechoCerradoYAbierto(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		s := tv3aSig(utf)
		m := tv3aModeloDorado(tv3aCfg(99, 40, utf, false))
		li := tv3aLista(t, "CA-700", m, 99).tv3aTexto()
		if tv3aIndice(li, " HECHO 2 "+s.HechoCerrado) < 0 || ttTiene(m, "Viejo 1") {
			t.Fatalf("CA-700 (utf=%v): HECHO empieza cerrado, sin sus tarjetas:\n%s", utf, tv3aLineas(m))
		}
		m.Tecla("G", ttAhora)
		li = tv3aLista(t, "CA-700", m, 99).tv3aTexto()
		if tv3aIndice(li, " "+s.Sel+" HECHO 2 "+s.HechoCerrado) < 0 || m.Elegida() != "" {
			t.Fatalf("CA-700 (utf=%v): G elige la linea de HECHO (\" %s HECHO 2 %s\") y el detalle no muestra tarjeta:\n%s",
				utf, s.Sel, s.HechoCerrado, tv3aLineas(m))
		}
		m.Tecla(tablero.TeclaEnter, ttAhora)
		li = tv3aLista(t, "CA-700", m, 99).tv3aTexto()
		i := tv3aIndice(li, " "+s.Sel+" HECHO 2 "+s.HechoAbierto)
		if i < 0 || i+2 >= len(li) || !strings.HasPrefix(tv3aDe(li, i+1), "   Viejo 1 ") || !strings.HasPrefix(tv3aDe(li, i+2), "   Viejo 2 ") {
			t.Fatalf("CA-700 (utf=%v): enter abre HECHO con sus tarjetas debajo:\n%s", utf, tv3aLineas(m))
		}
		if m.Modal() != tablero.SinModal {
			t.Fatalf("CA-700: enter sobre HECHO no abre un modal")
		}
		m.Tecla(tablero.TeclaEnter, ttAhora)
		if tv3aIndice(tv3aLista(t, "CA-700", m, 99).tv3aTexto(), " "+s.Sel+" HECHO 2 "+s.HechoCerrado) < 0 || ttTiene(m, "Viejo 1") {
			t.Fatalf("CA-700 (utf=%v): enter otra vez cierra HECHO:\n%s", utf, tv3aLineas(m))
		}
	}
}

// CA-700: una tarjeta con un piloto que lanzo este tablero (Seguir solo)
// esta en CORRIENDO mientras su proceso vive, desde el Lanzar hasta su
// FinProceso, aunque no tenga running.
func TestV3A_CA700_PilotoVivoEsCorriendo(t *testing.T) {
	defer tv3aSinPanico(t)
	c := tv3aConPiloto(tv3aCon(tv3aEn("auto-w", "Con piloto", boardcmd.ColWriter), ttAccionWriter()))
	m := ttModelo(tv3aCfg(120, 30, true, false), ttTablero(c))
	if m.Grupo("auto-w") != tablero.GrupoTeToca {
		t.Fatalf("CA-700: sin piloto vivo la tarjeta esta en TE TOCA; esta en %q", m.Grupo("auto-w"))
	}
	ttSinOrdenes(t, "CA-700", "P", m.Tecla("P", ttAhora))
	if m.Modal() != tablero.ModalSeguir {
		t.Fatalf("CA-700: P abre el modal Seguir solo; Modal = %q\n%s", m.Modal(), tv3aLineas(m))
	}
	l := ttUnaLanzar(t, "CA-700", "enter sobre Seguir solo", m.Tecla(tablero.TeclaEnter, ttAhora))
	ttMismosArgs(t, "CA-700", "el piloto", l.Args, []string{"piloto", "auto-w"})
	if m.Grupo("auto-w") != tablero.GrupoCorriendo {
		t.Fatalf("CA-700: con su piloto vivo la tarjeta esta en CORRIENDO; esta en %q", m.Grupo("auto-w"))
	}
	m.Leer(tablero.Lectura{Ahora: ttAhora.Add(2 * time.Second), Tablero: ttTablero(c)})
	if m.Grupo("auto-w") != tablero.GrupoCorriendo || tv3aDebeLinea(t, "CA-700", m, " CORRIENDO 1", 0) == 0 {
		t.Fatalf("CA-700: entre trabajo y trabajo, con el piloto vivo, sigue en CORRIENDO:\n%s", tv3aLineas(m))
	}
	m.Termino(tablero.FinProceso{ID: l.ID, Code: 0, Ultima: "piloto automatico: se detiene: listo para tu aceptacion"}, ttAhora.Add(3*time.Second))
	if m.Grupo("auto-w") == tablero.GrupoCorriendo {
		t.Fatalf("CA-700: con su FinProceso la tarjeta deja CORRIENDO")
	}
}

// ================================================================ CA-701: las filas

// tv3aEstadoDe es la segunda linea (el estado) de una tarjeta del tablero
// dorado, en el modo.
func tv3aFilasDorado(utf bool) []struct {
	tit, col string
	g        tablero.Grupo
	est      string
} {
	s := tv3aSig(utf)
	return []struct {
		tit, col string
		g        tablero.Grupo
		est      string
	}{
		{"Listado de proyectos", boardcmd.ColWriter, tablero.GrupoTeToca, "los tests fallan: falta el codigo"},
		{"Exportar reporte a PDF", boardcmd.ColTuAprobacion, tablero.GrupoTeToca, "te toca aprobar el spec"},
		{"Módulo de usuarios", boardcmd.ColWriter, tablero.GrupoCorriendo, "writer · 14:02 de 30:00"},
		{"Pagos con tarjeta", boardcmd.ColWriter, tablero.GrupoDetenida, s.MarcaDet + " se corto el trabajo del writer"},
		{"Notificaciones por correo", boardcmd.ColReview, tablero.GrupoDetenida, s.MarcaDet + " el veredicto quedo rojo en test"},
		{"Importar desde Excel", boardcmd.ColBacklog, tablero.GrupoEnCola, "falta el spec de la tarjeta"},
	}
}

// CA-701, CA-555: con 100 columnas o mas la primera linea de una tarjeta es
// la marca, el titulo (nunca el slug) cortado o rellenado a room-9 celdas y
// el camino de 8 signos pegado al borde derecho; la segunda, tres espacios y
// su estado. Ningun slug esta en la lista.
func TestV3A_CA701_CA555_FilaAncha(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{false, true} {
		for _, w := range []int{100, 120, 160} {
			m := tv3aModeloDorado(tv3aCfg(w, 40, utf, false))
			li := tv3aLista(t, "CA-701", m, w)
			txt := li.Dentro
			tw := (li.Ancho - 2) - 3 - 9
			k := 1
			for i, f := range tv3aFilasDorado(utf) {
				for strings.HasPrefix(tv3aDe(txt, k), " "+string(f.g)+" ") || tv3aDe(txt, k) == "" {
					k++
				}
				marca := " "
				if i == 0 {
					marca = tv3aSig(utf).Sel
				}
				quiere := " " + marca + " " + tv3aRelleno(tv3aCortar(f.tit, tw, utf), tw) + " " + tv3aCamino(utf, f.col, f.g)
				if tv3aDe(txt, k) != quiere {
					t.Fatalf("CA-701: a %d (utf=%v) la primera linea de %q es\n%q\nes\n%q", w, utf, f.tit, quiere, tv3aDe(txt, k))
				}
				if quiere := tv3aRelleno("   "+f.est, li.Ancho-2); tv3aDe(txt, k+1) != quiere {
					t.Fatalf("CA-701: a %d (utf=%v) la segunda linea de %q es\n%q\nes\n%q", w, utf, f.tit, quiere, tv3aDe(txt, k+1))
				}
				k += 2
			}
			for _, l := range txt {
				for _, slug := range []string{"listado", "exportar-pdf", "modulo-usuarios", "pagos", "notificaciones", "importar-excel"} {
					if strings.Contains(l, slug) {
						t.Fatalf("CA-701: el slug %s no esta en la lista: %q", slug, l)
					}
				}
			}
		}
	}
}

// CA-701, CA-577 (hostil): un titulo largo se corta con …/~ para dejar el
// camino; uno de caracteres anchos sale entero o no sale (la celda que deja
// es un espacio) y una marca combinante se queda con su base: el camino
// sigue pegado al borde y el lado no se corre.
func TestV3A_CA701_CA577_TitulosLargosYAnchos(t *testing.T) {
	defer tv3aSinPanico(t)
	titulos := []string{
		"Un titulo larguisimo que no entra en la fila de la lista",
		"漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字",
		"A漢字漢字漢字漢字漢字漢字漢字漢字漢字漢字",
		"Cafe\u0301 con leche y medialunas de manteca",
		"🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀🚀",
	}
	for _, utf := range []bool{true, false} {
		for _, w := range []int{100, 113, 140} {
			for _, tit := range titulos {
				c := tv3aCon(tv3aEn("larga", tit, boardcmd.ColTestWriter), tv3aPedirTW())
				m := ttModelo(tv3aCfg(w, 24, utf, false), ttTablero(c))
				ttForma(t, "CA-701", m, w, 24, false)
				li := tv3aLista(t, "CA-701", m, w)
				tw := (li.Ancho - 2) - 3 - 9
				quiere := " " + tv3aSig(utf).Sel + " " + tv3aRelleno(tv3aCortar(tit, tw, utf), tw) + " " + tv3aCamino(utf, boardcmd.ColTestWriter, tablero.GrupoTeToca)
				if tv3aDe(li.Dentro, 1) != quiere {
					t.Fatalf("CA-701: a %d (utf=%v) la fila de %q es\n%q\nes\n%q", w, utf, tit, quiere, tv3aDe(li.Dentro, 1))
				}
			}
		}
	}
}

// tv3aT es el ancho del titulo de una fila angosta (decision de doc.go):
// max(8, min(L, (w-2-12)/2)).
func tv3aT(w, l int) int {
	t := (w - 2 - 12) / 2
	if l < t {
		t = l
	}
	if t < 8 {
		t = 8
	}
	return t
}

// CA-701, CA-555: entre 60 y 99 columnas una tarjeta es una linea: la marca,
// el titulo cortado o rellenado a t celdas, el camino (en la misma columna
// en todas las filas) y, despues de un espacio, su estado; la linea se corta
// a w-2.
func TestV3A_CA701_CA555_FilaAngosta(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{false, true} {
		for w := 60; w <= 99; w++ {
			m := tv3aModeloDorado(tv3aCfg(w, 40, utf, false))
			li := tv3aLista(t, "CA-701", m, w)
			tt := tv3aT(w, 25)
			k := 1
			for i, f := range tv3aFilasDorado(utf) {
				for strings.HasPrefix(tv3aDe(li.Dentro, k), " "+string(f.g)+" ") {
					k++
				}
				marca := " "
				if i == 0 {
					marca = tv3aSig(utf).Sel
				}
				quiere := tv3aRelleno(tv3aCortar(" "+marca+" "+tv3aRelleno(tv3aCortar(f.tit, tt, utf), tt)+" "+tv3aCamino(utf, f.col, f.g)+" "+f.est,
					w-2, utf), w-2)
				if tv3aDe(li.Dentro, k) != quiere {
					t.Fatalf("CA-701: a %d (utf=%v) la fila de %q es\n%q\nes\n%q", w, utf, f.tit, quiere, tv3aDe(li.Dentro, k))
				}
				if got := ttTramo(tv3aDe(li.Dentro, k), 4+tt, 12+tt); got != tv3aCamino(utf, f.col, f.g) {
					t.Fatalf("CA-701: a %d el camino de %q esta en las celdas %d a %d de la fila: %q", w, f.tit, 4+tt, 12+tt, got)
				}
				k++
			}
		}
	}
}

// CA-701: en una columna el ancho del titulo es el del titulo mas ancho de
// las filas que se ven (con un minimo de 8): con titulos cortos el camino
// queda en la celda 12; al abrir HECHO, su titulo largo cuenta. TE TOCA va
// de Tu aceptacion a Arquitecto (CA-700): la fila 1 es BB (Writer) y la 2 es
// A (Arquitecto), cada una con su camino.
func TestV3A_CA701_AnchoDelTituloAngosto(t *testing.T) {
	defer tv3aSinPanico(t)
	b := ttTablero(tv3aEn("a", "A", boardcmd.ColArquitecto), tv3aEn("bb", "BB", boardcmd.ColWriter),
		tv3aEn("h", "Una tarjeta hecha de titulo largo", boardcmd.ColHecho))
	filas := []struct {
		linea         int
		titulo, signo string
	}{{1, "BB", "●●●●◆○○○"}, {2, "A", "●◆○○○○○○"}}
	m := ttModelo(tv3aCfg(80, 40, true, false), b)
	li := tv3aLista(t, "CA-701", m, 80).Dentro
	for _, f := range filas {
		if got := ttTramo(tv3aDe(li, f.linea), 3, 12); got != fmt.Sprintf("%-9s", f.titulo) {
			t.Fatalf("CA-701, CA-700: la fila %d es la de %q (TE TOCA de Tu aceptacion a Arquitecto): %q", f.linea, f.titulo, tv3aDe(li, f.linea))
		}
		if got := ttTramo(tv3aDe(li, f.linea), 12, 20); got != f.signo {
			t.Fatalf("CA-701: con titulos de 1 y 2 celdas, t = 8 y el camino de %q (%s) empieza en la celda 12 de su fila: %q",
				f.titulo, f.signo, tv3aDe(li, f.linea))
		}
	}
	tv3aAbrirHecho(t, m)
	li = tv3aLista(t, "CA-701", m, 80).Dentro
	tt := tv3aT(80, 33)
	for _, f := range filas {
		if got := ttTramo(tv3aDe(li, f.linea), 4+tt, 12+tt); got != f.signo {
			t.Fatalf("CA-701: con HECHO abierto su titulo de 33 celdas cuenta: t = %d y el camino de %q (%s) empieza en la celda %d: %q",
				tt, f.titulo, f.signo, 4+tt, tv3aDe(li, f.linea))
		}
	}
}

// CA-701, CA-555: la seleccion es ▸ en la primera linea; en la zona con el
// foco y con color, esa linea (solo la primera) es una barra en video
// inverso sobre las w-2 celdas, con los colores de sus piezas (7;32, 7;33,
// 7;90); sin el foco no hay barra pero sigue ▸.
func TestV3A_CA701_CA555_SeleccionYBarra(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(120, 36, true, true))
	// la fila de listado es la linea 8 (la 6 es el borde, la 7 " TE TOCA 2")
	if tv3aPosEn(m, 8, "▸ Listado de proyectos", 2) != 2 {
		t.Fatalf("CA-701: la fila elegida es la de listado, con ▸ en la celda 2:\n%s", tv3aLineas(m))
	}
	for x := 1; x < 47; x++ {
		if sgr := tv3aSgrEn(m, 8, x); !tv3aCon7(sgr) {
			t.Fatalf("CA-701: la barra de la fila elegida cubre las 46 celdas interiores en video inverso; la celda %d va con %q: %q",
				x, sgr, ttLinea(m, 8))
		}
	}
	tv3aSgrTramo(t, "CA-701", "el camino antes, en la barra", m, 8, 39, 43, "7;32")
	tv3aSgrTramo(t, "CA-701", "el signo de TE TOCA, en la barra", m, 8, 43, 44, "7;33")
	tv3aSgrTramo(t, "CA-701", "el camino despues, en la barra", m, 8, 44, 47, "7;90")
	tv3aSgrTramo(t, "CA-701", "la segunda linea no lleva barra", m, 9, 4, 4+ttCeldas("los tests fallan: falta el codigo"), "33")
	tv3aSgrTramo(t, "CA-701", "otra fila no lleva barra", m, 10, 1, 39, "")
	m.Tecla(tablero.TeclaTab, ttAhora)
	if tv3aPosEn(m, 8, "▸ Listado de proyectos", 2) != 2 {
		t.Fatalf("CA-701: sin el foco la fila elegida sigue con ▸:\n%s", tv3aLineas(m))
	}
	tv3aSgrTramo(t, "CA-701", "sin el foco no hay barra", m, 8, 1, 39, "")
	tv3aSgrTramo(t, "CA-701", "sin el foco el camino va en sus colores", m, 8, 39, 43, "32")
}

// CA-701: en una columna, con el foco y color, la barra cubre tambien el
// relleno que sigue al estado, hasta w-2.
func TestV3A_CA701_BarraAngostaConRelleno(t *testing.T) {
	defer tv3aSinPanico(t)
	m := tv3aModeloDorado(tv3aCfg(80, 40, true, true))
	for x := 1; x < 79; x++ {
		if sgr := tv3aSgrEn(m, 8, x); !tv3aCon7(sgr) {
			t.Fatalf("CA-701: la barra angosta cubre las 78 celdas (el relleno incluido); la celda %d va con %q: %q", x, sgr, ttLinea(m, 8))
		}
	}
}

// CA-701, CA-555: con color, el estado de cada tarjeta va en el color de su
// grupo: TE TOCA 33, CORRIENDO 36, DETENIDA 31, EN COLA 90, HECHO 90; en una
// columna, el estado que sigue al camino tambien.
func TestV3A_CA701_CA555_EstadoEnElColorDelGrupo(t *testing.T) {
	defer tv3aSinPanico(t)
	colores := map[tablero.Grupo]string{tablero.GrupoTeToca: "33", tablero.GrupoCorriendo: "36", tablero.GrupoDetenida: "31",
		tablero.GrupoEnCola: "90", tablero.GrupoHecho: "90"}
	for _, w := range []int{120, 80} {
		m := tv3aModeloDorado(tv3aCfg(w, 40, true, true))
		m.Tecla(tablero.TeclaTab, ttAhora) // sin barra en la fila elegida
		if w < 100 {
			m.Tecla(tablero.TeclaShiftTab, ttAhora)
			m.Tecla("G", ttAhora) // la seleccion en HECHO: ninguna tarjeta con barra
		}
		for _, f := range tv3aFilasDorado(true) {
			n := 0
			for k := 6; k < 38; k++ {
				if tv3aPosEn(m, k, f.est, 3) >= 0 {
					n = k
					break
				}
			}
			if n == 0 {
				t.Fatalf("CA-701: a %d la lista dice %q:\n%s", w, f.est, tv3aLineas(m))
			}
			tv3aSgrDe(t, "CA-701", fmt.Sprintf("a %d el estado de %s", w, f.tit), m, n, f.est, 3, colores[f.g])
		}
	}
	m := tv3aModeloDorado(tv3aCfg(120, 40, true, true))
	tv3aAbrirHecho(t, m)
	n := tv3aDebeLinea(t, "CA-701", m, "terminada: su cierre quedo registrado", 3)
	tv3aSgrDe(t, "CA-701", "el estado de HECHO", m, n, "terminada: su cierre quedo registrado", 3, "90")
}

// ================================================================ CA-702: el camino

// CA-702: el camino tiene un signo por columna: ● antes de la de la
// tarjeta, ◆ ◉ ✗ u ○ en la suya segun su grupo, ○ despues; 8 ● en HECHO; en
// ASCII # ! * x y -. Con color: 32 antes, 33 / 36 / 31 / 90 en la suya, 90
// despues, y 32 los 8 de HECHO.
func TestV3A_CA702_SignosDelCamino(t *testing.T) {
	defer tv3aSinPanico(t)
	type caso struct {
		c boardcmd.Card
		g tablero.Grupo
	}
	var casos []caso
	for _, k := range boardcmd.Columns {
		switch k.ID {
		case boardcmd.ColBacklog:
			casos = append(casos, caso{tv3aEn("x", "X", k.ID), tablero.GrupoEnCola})
		case boardcmd.ColHecho:
			casos = append(casos, caso{tv3aEn("x", "X", k.ID), tablero.GrupoHecho})
		default:
			casos = append(casos, caso{tv3aEn("x", "X", k.ID), tablero.GrupoTeToca})
		}
		if k.ID != boardcmd.ColHecho {
			casos = append(casos, caso{tv3aCorre(tv3aEn("x", "X", k.ID), "writer", time.Minute, 0), tablero.GrupoCorriendo})
			casos = append(casos, caso{tv3aRoja(tv3aEn("x", "X", k.ID), "rojo", false), tablero.GrupoDetenida})
			casos = append(casos, caso{tv3aCortada(tv3aEn("x", "X", k.ID), "writer"), tablero.GrupoDetenida})
		}
	}
	colores := map[tablero.Grupo]string{tablero.GrupoTeToca: "33", tablero.GrupoCorriendo: "36", tablero.GrupoDetenida: "31",
		tablero.GrupoEnCola: "90", tablero.GrupoHecho: "32"}
	for _, utf := range []bool{true, false} {
		for _, c := range casos {
			que := fmt.Sprintf("%s en %s (utf=%v)", c.g, c.c.Column, utf)
			m := ttModelo(tv3aCfg(120, 30, utf, true), ttTablero(c.c))
			if c.g == tablero.GrupoHecho {
				tv3aAbrirHecho(t, m)
				m.Tecla("j", ttAhora)
			}
			m.Tecla(tablero.TeclaTab, ttAhora) // el foco al detalle: la fila sin barra
			// la fila de la tarjeta es la linea 8; el camino, sus celdas 39 a 46
			got := ttTramo(tv3aDe(ttLimpia(m), 7), 39, 47)
			if quiere := tv3aCamino(utf, c.c.Column, c.g); got != quiere {
				t.Fatalf("CA-702: %s: el camino es %q, es %q:\n%s", que, quiere, got, tv3aLineas(m))
			}
			i := tv3aColumna(c.c.Column)
			for x := 0; x < 8; x++ {
				quiere := "90"
				switch {
				case c.g == tablero.GrupoHecho || x < i:
					quiere = "32"
				case x == i:
					quiere = colores[c.g]
				}
				if sgr := tv3aSgrEn(m, 8, 39+x); sgr != quiere {
					t.Fatalf("CA-702: %s: el signo %d del camino va con %q, va con %q: %q", que, x+1, quiere, sgr, ttLinea(m, 8))
				}
			}
		}
	}
}

// ================================================================ CA-711: las filas de CORRIENDO y DETENIDA

// CA-711, CA-558: una tarjeta con running esta en CORRIENDO y su estado es
// "<rol> · <tiempo>[ de <limite>]" con los tiempos de la v1 (m:ss, h:mm:ss),
// en cian; el rol vacio es "agente"; el tiempo se mueve con el reloj.
func TestV3A_CA711_CA558_FilaDeLoQueCorre(t *testing.T) {
	defer tv3aSinPanico(t)
	casos := []struct {
		rol    string
		hace   time.Duration
		limite int
		dice   string
	}{
		{"writer", 14*time.Minute + 2*time.Second, 1800, "writer · 14:02 de 30:00"},
		{"writer", 14*time.Minute + 2*time.Second, 0, "writer · 14:02"},
		{"reviewer", time.Hour + 2*time.Minute + 3*time.Second, 7200, "reviewer · 1:02:03 de 2:00:00"},
		{"", 5 * time.Second, 0, "agente · 0:05"},
		{"test-writer", 59 * time.Second, 3600, "test-writer · 0:59 de 1:00:00"},
	}
	for _, c := range casos {
		card := tv3aCorre(tv3aEn("corre", "Corre", boardcmd.ColTestWriter), c.rol, c.hace, c.limite)
		for _, w := range []int{120, 80} {
			m := ttModelo(tv3aCfg(w, 30, true, true), ttTablero(card))
			if m.Grupo("corre") != tablero.GrupoCorriendo {
				t.Fatalf("CA-711: con running la tarjeta esta en CORRIENDO")
			}
			m.Tecla(tablero.TeclaTab, ttAhora)
			if w < 100 {
				m.Tecla(tablero.TeclaShiftTab, ttAhora)
				m.Tecla("G", ttAhora)
			}
			n := tv3aDebeLinea(t, "CA-711", m, c.dice, 3)
			if n < 6 {
				t.Fatalf("CA-711: el estado %q esta en la lista (linea 6 o mas), esta en la %d", c.dice, n)
			}
			tv3aSgrDe(t, "CA-711", "el estado de CORRIENDO", m, n, c.dice, 3, "36")
		}
	}
	card := tv3aModulo()
	m := ttModelo(tv3aCfg(120, 30, true, false), ttTablero(card))
	m.Tic(ttAhora.Add(10 * time.Second))
	if tv3aLineaCon(m, "writer · 14:12 de 30:00", 3) == 0 {
		t.Fatalf("CA-711: el tiempo de la fila se mueve con el reloj (Tic): \"writer · 14:12 de 30:00\":\n%s", tv3aLineas(m))
	}
}

// CA-711, CA-558: una tarjeta con interrupted va a DETENIDA y su estado es
// "✗ se corto el trabajo del <rol>" ("x " en ASCII; "agente" sin rol), en
// rojo; el corte gana sobre su rojo.
func TestV3A_CA711_CA558_CortadaVaADetenidaEnRojo(t *testing.T) {
	defer tv3aSinPanico(t)
	casos := []struct {
		c    boardcmd.Card
		dice string
	}{
		{tv3aCortada(tv3aEn("x", "Cortada", boardcmd.ColWriter), "writer"), "se corto el trabajo del writer"},
		{tv3aCortada(tv3aEn("x", "Cortada", boardcmd.ColReview), ""), "se corto el trabajo del agente"},
		{tv3aRoja(tv3aCortada(tv3aEn("x", "Cortada", boardcmd.ColWriter), "writer"), "un rojo", false), "se corto el trabajo del writer"},
	}
	for _, utf := range []bool{true, false} {
		for _, c := range casos {
			for _, w := range []int{120, 80} {
				m := ttModelo(tv3aCfg(w, 30, utf, true), ttTablero(c.c))
				if m.Grupo("x") != tablero.GrupoDetenida {
					t.Fatalf("CA-711: con interrupted la tarjeta esta en DETENIDA; esta en %q", m.Grupo("x"))
				}
				m.Tecla(tablero.TeclaTab, ttAhora)
				if w < 100 {
					m.Tecla(tablero.TeclaShiftTab, ttAhora)
					m.Tecla("G", ttAhora)
				}
				dice := tv3aSig(utf).MarcaDet + " " + c.dice
				n := tv3aDebeLinea(t, "CA-711", m, dice, 3)
				if n < 6 {
					t.Fatalf("CA-711: %q esta en la lista", dice)
				}
				tv3aSgrDe(t, "CA-711", "el estado de DETENIDA", m, n, dice, 3, "31")
			}
		}
	}
}

// ================================================================ CA-714: el Registro plegado

// CA-714: el Registro ocupa la linea A-2: "─Registro" y signos de linea
// hasta W cuando esta vacio.
func TestV3A_CA714_RegistroPlegadoVacio(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		for _, tam := range [][2]int{{60, 15}, {80, 24}, {100, 30}, {137, 41}} {
			m := tv3aModeloDorado(tv3aCfg(tam[0], tam[1], utf, false))
			lin := tv3aSig(utf).Lin
			if got, quiere := tv3aDe(ttLimpia(m), tam[1]-3), lin+"Registro"+strings.Repeat(lin, tam[0]-9); got != quiere {
				t.Fatalf("CA-714: a %dx%d el Registro vacio es\n%q\nes\n%q", tam[0], tam[1], quiere, got)
			}
		}
	}
}

// CA-714: con entradas, la linea A-2 es "─Registro · " y la primera linea de
// la entrada mas nueva, con signos de linea hasta W; un fallo va en rojo.
func TestV3A_CA714_RegistroPlegadoMuestraLaUltima(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, utf := range []bool{true, false} {
		m := tv3aModeloDorado(tv3aCfg(100, 30, utf, true))
		lin := tv3aSig(utf).Lin
		reg := func() string { return tv3aDe(ttLimpia(m), 27) }
		quiere := func(s string) string {
			x := lin + "Registro · " + s
			return x + strings.Repeat(lin, 100-ttCeldas(x))
		}
		u1 := ttUnaLanzar(t, "CA-714", "u", m.Tecla("u", ttAhora))
		if q := quiere("12:00 Fijar para la terminal · listado · " + tv3aFijo(utf, "lanzando…")); reg() != q {
			t.Fatalf("CA-714 (utf=%v): tras u el Registro plegado es\n%q\nes\n%q", utf, q, reg())
		}
		m.Tecla("j", ttAhora)
		u2 := ttUnaLanzar(t, "CA-714", "u", m.Tecla("u", ttAhora))
		if q := quiere("12:00 Fijar para la terminal · exportar-pdf · " + tv3aFijo(utf, "lanzando…")); reg() != q {
			t.Fatalf("CA-714 (utf=%v): el Registro plegado muestra la entrada mas nueva\n%q\nes\n%q", utf, q, reg())
		}
		m.Termino(tablero.FinProceso{ID: u1.ID, Code: 0}, ttAhora)
		if q := quiere("12:00 Fijar para la terminal · exportar-pdf · " + tv3aFijo(utf, "lanzando…")); reg() != q {
			t.Fatalf("CA-714 (utf=%v): el fin de una entrada vieja no cambia la linea\n%q\nes\n%q", utf, q, reg())
		}
		m.Termino(tablero.FinProceso{ID: u2.ID, Code: 1, Ultima: "no se pudo fijar"}, ttAhora)
		if q := quiere("12:00 Fijar para la terminal · exportar-pdf · fallo: no se pudo fijar"); reg() != q {
			t.Fatalf("CA-714 (utf=%v): tras el fallo el Registro plegado es\n%q\nes\n%q", utf, q, reg())
		}
		tv3aSgrDe(t, "CA-714", "el fallo", m, 28, "fallo: no se pudo fijar", 0, "31")
	}
}

// ================================================================ CA-720: sin color y en ASCII

// tv3aPantallasVarias son los estados de la pantalla que mira CA-720: el
// principio, una tarjeta que corre (la barra), una detenida, HECHO abierto
// con una tarjeta elegida, la vista Hallazgos, el proyecto, y la lista
// plegada en una columna.
func tv3aPantallasVarias(t *testing.T, cfg tablero.Config, b boardcmd.Board) map[string]*tablero.Modelo {
	t.Helper()
	nuevo := func() *tablero.Modelo {
		m := tablero.Nuevo(cfg)
		m.Leer(tablero.Lectura{Ahora: ttAhora, Tablero: b, Fijada: "exportar-pdf"})
		m.LeerProyecto(ttProyectoVerde())
		m.LeerHallazgos(ttHallazgosDorados())
		return m
	}
	out := map[string]*tablero.Modelo{}
	out["principio"] = nuevo()
	m := nuevo()
	tv3aElegir(t, m, "modulo-usuarios")
	out["corriendo"] = m
	m = nuevo()
	tv3aElegir(t, m, "notificaciones")
	out["detenida"] = m
	m = nuevo()
	tv3aAbrirHecho(t, m)
	m.Tecla("j", ttAhora)
	out["hecho"] = m
	m = nuevo()
	m.Tecla("H", ttAhora)
	out["hallazgos"] = m
	m = nuevo()
	m.Tecla("v", ttAhora)
	out["proyecto"] = m
	m = nuevo()
	m.Tecla(tablero.TeclaTab, ttAhora)
	out["detalle con foco"] = m
	return out
}

// CA-720: con NO_COLOR (Config.Color false) ninguna linea lleva una
// secuencia de escape: la caja, los grupos y el camino se distinguen por sus
// palabras y signos, y el borde de la caja es la linea simple.
func TestV3A_CA720_SinColorNingunaSecuencia(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, tam := range [][2]int{{120, 36}, {80, 24}} {
		cfg := tv3aCfg(tam[0], tam[1], true, false)
		for que, m := range tv3aPantallasVarias(t, cfg, tv3aTableroDorado()) {
			ttForma(t, "CA-720: "+que, m, tam[0], tam[1], false)
			if p := tv3aCajaPaso(t, "CA-720", m, tam[0]); !strings.HasPrefix(p.Arriba, "┌TU SIGUIENTE PASO") || !strings.HasSuffix(p.Arriba, "─┐") {
				t.Fatalf("CA-720: %s: sin color el borde de la caja es la linea simple: %q", que, p.Arriba)
			}
		}
	}
	m := tv3aModeloDorado(tv3aCfg(120, 36, true, false))
	for _, s := range []string{"TE TOCA 2", "CORRIENDO 1", "DETENIDA 2", "EN COLA 1", "HECHO 2 ▸", "●●●●◆○○○", "●●●●◉○○○", "●●●●✗○○○", "○○○○○○○○",
		"✗ se corto el trabajo del writer"} {
		if !ttTiene(m, s) {
			t.Fatalf("CA-720: sin color la pantalla se lee por sus palabras y signos: dice %q:\n%s", s, tv3aLineas(m))
		}
	}
}

// tv3aTableroASCII es el tablero dorado con todos los textos de sus
// tarjetas en ASCII (el titulo de modulo-usuarios sin su ó).
func tv3aTableroASCII() boardcmd.Board {
	mod := tv3aModulo()
	mod.Item.Titulo = "Modulo de usuarios"
	return ttTablero(tv3aImportar(), tv3aExportar(), tv3aListado(), mod, tv3aPagos(), tv3aNotificaciones(), ttViejo(1), ttViejo(2))
}

// CA-720: con HOOM_ASCII=1 o sin un locale UTF-8 (Config.UTF8 false), con
// tarjetas y proyecto en ASCII, la pantalla no tiene ninguna runa que no sea
// ASCII salvo el separador "·": el camino usa # ! * x -, la flecha es <-, la
// barra del tiempo # y -, «» son " y las frases fijas pierden sus acentos.
func TestV3A_CA720_ASCIISoloASCII(t *testing.T) {
	defer tv3aSinPanico(t)
	for _, tam := range [][2]int{{120, 36}, {100, 30}, {80, 24}, {60, 15}} {
		cfg := tv3aCfg(tam[0], tam[1], false, false)
		for que, m := range tv3aPantallasVarias(t, cfg, tv3aTableroASCII()) {
			for i, l := range m.Pantalla() {
				for _, r := range l {
					if r > 127 && r != '·' {
						t.Fatalf("CA-720: %dx%d %s: en ASCII la linea %d no lleva %q: %q\n%s", tam[0], tam[1], que, i+1, r, l, tv3aLineas(m))
					}
				}
			}
		}
	}
	m := tv3aModeloDorado(tv3aCfg(120, 36, false, false))
	tv3aElegir(t, m, "modulo-usuarios")
	if !ttTiene(m, " tiempo #########----------- 14:02 de 30:00") {
		t.Fatalf("CA-720: en ASCII la barra del tiempo usa # y -:\n%s", tv3aLineas(m))
	}
	m = tv3aModeloDorado(tv3aCfg(120, 36, false, false))
	if !ttTiene(m, "te toca a vos") || !ttTiene(m, "<- aca") || !ttTiene(m, "Pedile el codigo al writer para \"Listado de proyectos\".") ||
		!ttTiene(m, "despues: \"Exportar reporte a PDF\"") || !ttTiene(m, " Los tests estan escritos y fallan, como se espera.") {
		t.Fatalf("CA-720: en ASCII: <- aca, las comillas \" y las frases sin acentos:\n%s", tv3aLineas(m))
	}
}
