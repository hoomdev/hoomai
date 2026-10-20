// Tests de regresion de la septima vuelta de la review de la cabina C4 (la
// delta 3) sobre .hoom/specs/historia-doctor-y-cinta.md, en runcmd. Escritos
// desde el spec (enmienda 5, seccion "Fuente telemetria", y CA-373 del
// normalizador de Claude) y la descripcion de los hallazgos, sin leer la
// implementacion. Los helpers de este archivo llevan el prefijo c4v7; los
// compartidos (prefijo dlg) estan en delegaciones_helpers_test.go.
//
//   - 20261019T213022_b8d4a1 y 20261019T213459_733ac3: con la enmienda 4 hoom
//     recortaba al escribirlo el detail de un agent_end cuya linea pasaba de
//     1 MiB. Con un subagente de nombre largo que falla, el detail
//     "<agente> fallo: ..." quedaba en un prefijo del nombre, sin " fallo", y
//     la historia publicaba el fallo como "<agente> termino su parte".
//   - 20261019T213041_de5a5a: un hoom anterior a la enmienda 4 escribia el
//     evento entero, y sus logs pueden tener delegaciones de mas de 1 MiB. La
//     enmienda 5 deja esa delegacion fuera de contrato, en un log nuevo o
//     viejo: se le exige que no tape a sus vecinas y que nunca se lea distinta
//     de lo que fue.
//
// El contrato es el de la enmienda 5 (decision de Henry, reemplaza a la 4):
// hoom no recorta las delegaciones; escribe cada agent y agent_end tal como
// lo arma el normalizador, que ya recorta a 300 bytes la descripcion y el
// texto del fallo. La historia lee entera cada linea de delegacion de hasta
// 1 MiB, sin contar el salto, y salta una mas larga.
package runcmd

import (
	"fmt"
	"strings"
	"testing"
)

// c4v7Agente es un subagent_type de casi n bytes (sin partir un caracter)
// que empieza con prefijo, con ASCII y caracteres de 2 y 3 bytes.
func c4v7Agente(prefijo string, n int) string {
	return dlgCabeza(prefijo+strings.Repeat("falla-ñ-漢-", n/13+1), n)
}

// c4v7Fallo es una delegacion a un subagente de nombre largo que falla.
type c4v7Fallo struct {
	nombre string
	agente string
	tool   string
	// por como falla: un tool_result con is_error, o (en segundo plano) una
	// task_notification con estado failed
	notificacion bool
	// si la linea de su agent y la de su agent_end, como las arma el
	// normalizador, entran en 1 MiB
	entraAgent, entraFin bool
}

// c4v7Fallos son las delegaciones que fallan: con un agente de 200 KiB (las
// dos lineas entran), de 600 KiB (el agent entra y el agent_end, con el
// agente dos veces, no) y de 1100 KiB (no entra ninguna), por cada forma de
// fallar.
func c4v7Fallos() []c4v7Fallo {
	var fs []c4v7Fallo
	for _, n := range []int{200 << 10, 600 << 10, 1100 << 10} {
		for _, notif := range []bool{false, true} {
			como, prefijo := "tool_result con is_error", "hoom-error-"
			if notif {
				como, prefijo = "task_notification failed en segundo plano", "hoom-fondo-"
			}
			fs = append(fs, c4v7Fallo{
				nombre:       fmt.Sprintf("agente de %d KiB, %s", n>>10, como),
				agente:       c4v7Agente(prefijo, n),
				tool:         fmt.Sprintf("toolu_FALLA_%d_%v", n>>10, notif),
				notificacion: notif,
				entraAgent:   n < 1<<20,
				entraFin:     2*n < 1<<20,
			})
		}
	}
	return fs
}

// c4v7Salida es la salida del claude falso: una delegacion normal a Explore
// que termina, las delegaciones que fallan y una normal a general-purpose
// que termina.
func c4v7Salida(fs []c4v7Fallo) []byte {
	lineas := []string{
		dlgClaudeToolUse("toolu_ANTES", "Explore", false),
		dlgClaudeResultado("toolu_ANTES", false, "los precios"),
	}
	for _, f := range fs {
		if f.notificacion {
			lineas = append(lineas,
				dlgClaudeToolUse(f.tool, f.agente, true),
				dlgClaudeResultado(f.tool, false, "Async agent launched successfully"),
				dlgClaudeNotificacion(f.tool, "failed"))
			continue
		}
		lineas = append(lineas,
			dlgClaudeToolUse(f.tool, f.agente, false),
			dlgClaudeResultado(f.tool, true, "el subagente se quedo sin turnos"))
	}
	lineas = append(lineas,
		dlgClaudeToolUse("toolu_DESPUES", "general-purpose", false),
		dlgClaudeResultado("toolu_DESPUES", false, "listo"))
	return dlgClaudeSalida(lineas...)
}

// c4v7Referencia es lo que da el normalizador de Claude para la salida, con
// la forma que exige el fixture: la delegacion a Explore, el agent y el
// agent_end de cada fallo (este con la marca "<agente> fallo") y la
// delegacion a general-purpose; y el tamano de cada linea es el que dice
// cada fallo.
func c4v7Referencia(t *testing.T, salida []byte, fs []c4v7Fallo) []Event {
	t.Helper()
	ref := dlgNormalizarClaude(t, salida)
	if len(ref) != 4+2*len(fs) || ref[0].ToolID != "toolu_ANTES" || ref[len(ref)-1].ToolID != "toolu_DESPUES" {
		t.Fatalf("fixture: el normalizador de Claude da agent y agent_end de cada delegacion, en orden:\n%s", dlgListado(ref))
	}
	for i, f := range fs {
		a, fin := ref[2+2*i], ref[3+2*i]
		if a.Kind != "agent" || a.Agent != f.agente || a.ToolID != f.tool ||
			fin.Kind != "agent_end" || fin.Agent != f.agente || fin.ToolID != f.tool || !strings.HasPrefix(fin.Detail, f.agente+" fallo") {
			t.Fatalf("fixture: %s: el normalizador da su agent y un agent_end \"<agente> fallo...\" (CA-373), con el agente entero:\n  %s\n  %s",
				f.nombre, dlgResumen(a), dlgResumen(fin))
		}
		for _, x := range []struct {
			e     Event
			entra bool
		}{{a, f.entraAgent}, {fin, f.entraFin}} {
			// el ts del run mide como mucho 35 bytes, como el de la referencia
			menor, mayor := dlgLargoJSON(t, x.e)
			if (x.entra && mayor+64 > dlgMiB) || (!x.entra && menor-64 <= dlgMiB) {
				t.Fatalf("fixture: %s: la linea de su %s entra en 1 MiB = %v, lejos del borde: mide %d/%d", f.nombre, x.e.Kind, x.entra, menor, mayor)
			}
		}
	}
	return ref
}

// CA-362 (hallazgos b8d4a1 y 733ac3; enmienda 5), con el claude de CA-374:
// un run de Claude delega en subagentes de nombre largo que FALLAN (con un
// tool_result con is_error y, en segundo plano, con una task_notification
// failed), con agentes de 200 KiB, 600 KiB y 1100 KiB, entre dos delegaciones
// normales que terminan. hoom escribe el agent_end de cada fallo tal como lo
// arma el normalizador: el detail es exactamente "<agente> fallo: ..." o
// "<agente> fallo (failed)", con el agente entero, mida lo que mida la linea
// (con 600 KiB pasa de 1 MiB; la enmienda 4 lo recortaba y borraba la marca).
// Lo que el run tiene en memoria es lo escrito. ReadDelegations trae entero
// cada agent_end de un fallo cuya linea entra en 1 MiB, con su marca, y nunca
// uno sin ella: el de una linea mas larga no sale. Las delegaciones vecinas
// salen enteras.
func TestHallazgo_b8d4a1_733ac3_CA362_ElFalloDeUnSubagenteDeNombreLargoSeEscribeTalCual(t *testing.T) {
	fs := c4v7Fallos()
	salida := c4v7Salida(fs)
	ref := c4v7Referencia(t, salida, fs)
	var casos []dlgCaso
	for i, e := range ref {
		casos = append(casos, dlgCaso{fmt.Sprintf("delegacion %d (%s %s)", i, e.Kind, e.ToolID), e})
	}

	root, id, mem := dlgCorrerSalida(t, "claude", salida)
	ca := "CA-362 (hallazgos b8d4a1 y 733ac3, enmienda 5)"
	dlgTSDelDisco(t, ca, root, id, casos)
	dlgExigirTalCual(t, ca, root, id, mem, casos)

	// lo que la historia lee de cada fallo, segun el contrato (no segun lo
	// que haya en el disco): el agent_end de una linea que entra sale con su
	// marca; el de una que no entra no sale; nunca sale uno sin la marca
	ds, err := ReadDelegations(root, id)
	if err != nil {
		t.Fatalf("%s: ReadDelegations lee el run: %v", ca, err)
	}
	for _, f := range fs {
		cc := ca + ", " + f.nombre
		var fins, agents []Event
		for _, d := range ds {
			switch {
			case d.ToolID == f.tool && d.Kind == "agent_end":
				fins = append(fins, d)
			case d.ToolID == f.tool && d.Kind == "agent":
				agents = append(agents, d)
			}
		}
		for _, d := range fins {
			if !strings.HasPrefix(d.Detail, f.agente+" fallo") || d.Agent != f.agente {
				t.Errorf("%s: la historia nunca lee un fallo como otra cosa: ReadDelegations dio el agent_end %s, sin la marca \"<agente> fallo\" o sin su agente entero",
					cc, dlgResumen(d))
			}
		}
		switch {
		case f.entraFin && len(fins) != 1:
			t.Errorf("%s: la linea de su agent_end entra en 1 MiB: ReadDelegations la trae una vez, entera; trajo %d:\n%s", cc, len(fins), dlgListado(fins))
		case !f.entraFin && len(fins) != 0:
			t.Errorf("%s: la linea de su agent_end pasa de 1 MiB: la historia la salta; ReadDelegations trajo %d:\n%s", cc, len(fins), dlgListado(fins))
		}
		if f.entraAgent && (len(agents) != 1 || agents[0].Agent != f.agente) {
			t.Errorf("%s: la linea de su agent entra en 1 MiB: ReadDelegations la trae una vez, con su agente entero; trajo %d:\n%s",
				cc, len(agents), dlgListado(agents))
		}
	}
	var vecinas []string
	for _, d := range ds {
		if d.ToolID == "toolu_ANTES" || d.ToolID == "toolu_DESPUES" {
			vecinas = append(vecinas, d.Kind+" "+d.ToolID)
		}
	}
	if got, want := strings.Join(vecinas, ", "), "agent toolu_ANTES, agent_end toolu_ANTES, agent toolu_DESPUES, agent_end toolu_DESPUES"; got != want {
		t.Errorf("%s: las delegaciones vecinas de los fallos salen todas, en orden: %s, quiere %s", ca, got, want)
	}
}
