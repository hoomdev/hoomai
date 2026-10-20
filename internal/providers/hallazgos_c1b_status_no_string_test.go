// Tests de regresion de los hallazgos 20261020T202026_cb821a (low,
// reliability) y 20261020T202440_1b69ad (low, resilience) de la review de los
// fixes de C1, sobre el normalizador con memoria de Claude (NewNormalizer,
// .hoom/specs/historia-doctor-y-cinta.md, CA-373): los dos son el mismo
// defecto.
//
// Una task_notification cuyo `status` esta PRESENTE pero no es un string
// (numero, objeto, lista, booleano o null) no se pudo interpretar: NO cierra
// la delegacion. Emite su system de siempre y ningun agent_end, y la
// delegacion sigue abierta: una task_notification posterior con status
// "failed" la cierra con "<agente> fallo (failed)". Hoy ese status se lee
// como "" y cierra la delegacion como exito ("<agente> termino"), y la
// notificacion real que llega despues ya no puede corregirla.
//
// Guardas (verdes en la base): un status ausente sigue como hoy (cierra con
// "<agente> termino"), y "completed" y "failed" como string siguen como hoy,
// tambien escritos con escapes JSON.
//
// Las lineas son las REALES de Claude Code 2.1.281 de correlacion_test.go,
// cambiando SOLO el valor de "status" (o quitando la clave).
package providers

import (
	"strings"
	"testing"
)

// c1bNoString son valores JSON de status que no son un string.
var c1bNoString = []struct{ nombre, raw string }{
	{"numero", `3`},
	{"numero con decimales", `3.5`},
	{"numero negativo", `-1`},
	{"objeto", `{"x":1}`},
	{"objeto vacio", `{}`},
	{"objeto con un status adentro", `{"status":"completed"}`},
	{"lista", `[]`},
	{"lista con failed", `["failed"]`},
	{"booleano true", `true`},
	{"booleano false", `false`},
	{"null", `null`},
}

// c1bNotifRaw es la task_notification real con el status reemplazado por el
// valor JSON crudo raw.
func c1bNotifRaw(t *testing.T, raw string) string {
	t.Helper()
	const viejo = `"status":"completed"`
	if strings.Count(coRealNotification, viejo) != 1 {
		t.Fatalf("fixture: la task_notification real trae un solo %s", viejo)
	}
	return strings.Replace(coRealNotification, viejo, `"status":`+raw, 1)
}

// c1bNotifSinStatus es la task_notification real sin la clave status.
func c1bNotifSinStatus(t *testing.T) string {
	t.Helper()
	const viejo = `"status":"completed",`
	if strings.Count(coRealNotification, viejo) != 1 {
		t.Fatalf("fixture: la task_notification real trae un solo %s", viejo)
	}
	return strings.Replace(coRealNotification, viejo, "", 1)
}

// c1bSoloSystem exige que la linea emita exactamente lo que emite el
// Normalize sin memoria: UN system y ningun agent_end.
func c1bSoloSystem(t *testing.T, que, linea string, evs []Event) {
	t.Helper()
	p, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	if ends := coEnds(evs); len(ends) != 0 {
		t.Fatalf("cb821a/1b69ad CA-373: %s: un status que no es string no se pudo interpretar y NO cierra la delegacion (ningun agent_end), fue %v %+v",
			que, coKinds(evs), ends)
	}
	if len(evs) != 1 || evs[0].Kind != "system" {
		t.Fatalf("cb821a/1b69ad CA-373: %s: la task_notification sigue emitiendo su system de siempre, y nada mas: %v %+v",
			que, coKinds(evs), evs)
	}
	if want := p.Normalize(linea); !coMismo(evs, want) {
		t.Fatalf("cb821a/1b69ad CA-373: %s: el system es el de siempre (el del Normalize sin memoria):\ncorrelado   %+v\nsin memoria %+v",
			que, evs, want)
	}
}

// c1bCierre exige que la linea emita su system y DESPUES un unico agent_end
// de la delegacion real, con el detail pedido.
func c1bCierre(t *testing.T, que string, evs []Event, detail string) {
	t.Helper()
	if len(evs) != 2 || evs[0].Kind != "system" || evs[1].Kind != "agent_end" {
		t.Fatalf("cb821a/1b69ad CA-373: %s: la task_notification emite su system y despues el agent_end: %v %+v",
			que, coKinds(evs), evs)
	}
	e := evs[1]
	if e.Agent != "general-purpose" || e.ToolID != coRealID || e.Detail != detail {
		t.Fatalf("cb821a/1b69ad CA-373: %s: el agent_end lleva agent general-purpose, tool_id %s y detail %q: %+v",
			que, coRealID, detail, e)
	}
}

// Hallazgos 20261020T202026_cb821a y 20261020T202440_1b69ad. CA-373: una
// delegacion en segundo plano (solo cierra con su task_notification) no
// cierra con una task_notification cuyo status no es un string; la
// task_notification "failed" posterior la cierra una vez con
// "general-purpose fallo (failed)".
func TestCA373_HallazgoCb821a_StatusNoStringNoCierraEnSegundoPlano(t *testing.T) {
	for _, c := range c1bNoString {
		t.Run(c.nombre, func(t *testing.T) {
			n := coNorm(t)
			evUso := n(hcToolUseRealSegundoPlano(t))
			if len(evUso) != 1 || evUso[0].Kind != "agent" || evUso[0].ToolID != coRealID {
				t.Fatalf("CA-373: fixture: la delegacion real en segundo plano es un agent con su tool_id: %+v", evUso)
			}
			evStart := n(coRealTaskStarted)
			evRes := n(coRealToolResult) // en segundo plano el tool_result no cierra
			linea := c1bNotifRaw(t, c.raw)
			evRaro := n(linea)
			c1bSoloSystem(t, "segundo plano, status "+c.raw, linea, evRaro)
			evFin := n(hcNotifReal(t, "failed"))
			c1bCierre(t, "segundo plano, status "+c.raw+" y despues failed", evFin, "general-purpose fallo (failed)")
			if total := hcContarEnds(evUso, evStart, evRes, evRaro, evFin); total != 1 {
				t.Fatalf("CA-373: status %s: un solo agent_end por delegacion, hubo %d", c.raw, total)
			}
		})
	}
}

// Hallazgos 20261020T202026_cb821a y 20261020T202440_1b69ad. CA-373: una
// delegacion normal (cierra con el primero de su tool_result o su
// task_notification) tampoco cierra con un status que no es un string: la
// task_notification "failed" posterior la cierra con "fallo (failed)", y el
// tool_result real que llega despues no emite otro agent_end.
func TestCA373_HallazgoCb821a_StatusNoStringNoCierraLaNormal(t *testing.T) {
	for _, c := range c1bNoString {
		t.Run(c.nombre, func(t *testing.T) {
			n := coNorm(t)
			evUso := n(coRealToolUse)
			evStart := n(coRealTaskStarted)
			linea := c1bNotifRaw(t, c.raw)
			evRaro := n(linea)
			c1bSoloSystem(t, "delegacion normal, status "+c.raw, linea, evRaro)
			evFin := n(hcNotifReal(t, "failed"))
			c1bCierre(t, "delegacion normal, status "+c.raw+" y despues failed", evFin, "general-purpose fallo (failed)")
			evRes := n(coRealToolResult)
			if total := hcContarEnds(evUso, evStart, evRaro, evFin, evRes); total != 1 {
				t.Fatalf("CA-373: status %s: un solo agent_end por delegacion, hubo %d", c.raw, total)
			}
		})
	}
}

// GUARDA (verde en la base). Hallazgos cb821a y 1b69ad. CA-373: una
// task_notification SIN la clave status sigue como hoy: cierra la delegacion
// con "general-purpose termino", y la "failed" que llega despues ya no emite
// otro agent_end.
func TestCA373_HallazgoCb821a_StatusAusenteSigueComoHoy(t *testing.T) {
	for _, segundoPlano := range []bool{true, false} {
		n := coNorm(t)
		uso := coRealToolUse
		if segundoPlano {
			uso = hcToolUseRealSegundoPlano(t)
		}
		evUso := n(uso)
		evFin := n(c1bNotifSinStatus(t))
		c1bCierre(t, "status ausente", evFin, "general-purpose termino")
		evOtra := n(hcNotifReal(t, "failed"))
		if total := hcContarEnds(evUso, evFin, evOtra); total != 1 {
			t.Fatalf("CA-373: status ausente (segundo plano %v): un solo agent_end por delegacion, hubo %d", segundoPlano, total)
		}
	}
}

// GUARDA (verde en la base). Hallazgos cb821a y 1b69ad. CA-373: "completed" y
// "failed" como string siguen como hoy, tambien escritos con escapes JSON
// (siguen siendo un string).
func TestCA373_HallazgoCb821a_StatusStringSigueComoHoy(t *testing.T) {
	casos := []struct{ raw, detail string }{
		{`"completed"`, "general-purpose termino"},
		{`"completed"`, "general-purpose termino"},
		{`"failed"`, "general-purpose fallo (failed)"},
		{`"failed"`, "general-purpose fallo (failed)"},
	}
	for _, c := range casos {
		for _, segundoPlano := range []bool{true, false} {
			n := coNorm(t)
			uso := coRealToolUse
			if segundoPlano {
				uso = hcToolUseRealSegundoPlano(t)
			}
			evUso := n(uso)
			evFin := n(c1bNotifRaw(t, c.raw))
			c1bCierre(t, "status "+c.raw, evFin, c.detail)
			evRes := n(coRealToolResult)
			if total := hcContarEnds(evUso, evFin, evRes); total != 1 {
				t.Fatalf("CA-373: status %s (segundo plano %v): un solo agent_end por delegacion, hubo %d", c.raw, segundoPlano, total)
			}
		}
	}
}
