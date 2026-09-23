// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-333) sobre la evidencia EN DISCO: el registro del sobre trae el PID de
// su dueno, y Gather da por vivo un sobre abierto con PID solo si ese proceso
// vive, con o sin latido fresco. Un sobre muerto deja de ser interrumpido
// cuando otro sobre de la tarjeta empezo despues de su ultimo registro, y
// hoom nunca lo reescribe. Repos git reales en t.TempDir(), sin CLIs de IA.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// CA-333: el registro del sobre lleva pid en JSON, y la lectura lo conserva.
func TestCA333_RegistroTraePID(t *testing.T) {
	rec := envelope.Record{ID: "20260923T100000_aaaaaa", Role: "writer", Provider: "claude", Task: bdSlug,
		Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, PID: 4242, StartedAt: bdT0, UpdatedAt: bdT0}
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"pid":4242`) {
		t.Fatalf("CA-333: el registro del sobre trae la clave pid: %s", raw)
	}
	root := bdRepo(t, "")
	bdSobreDisco(t, root, rec)
	ev := Gather(root, "main", "high", bdItem(bdSlug), time.Now().UTC())
	if len(ev.Envelopes) != 1 || ev.Envelopes[0].Record.PID != 4242 {
		t.Fatalf("CA-333: Gather lee el pid del registro: %+v", ev.Envelopes)
	}
}

// CA-333: con pid mayor que 0, el sobre vive si y solo si ese proceso vive,
// sin mirar el latido; sin pid, la regla de C1 no cambia.
func TestCA333_PIDDelDuenoDecideLaVida(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	yo := os.Getpid()
	muerto := bdPIDMuerto(t)
	viejo := now.Add(-live.OrphanAfter - time.Hour)
	fresco := now.Add(-1 * time.Minute)
	sobre := func(id, task, runID, stage string, step, pid int, updated time.Time) string {
		return bdSobreDisco(t, root, envelope.Record{ID: id, Role: "writer", Provider: "claude", Task: task, Dir: root,
			RunID: runID, Stage: stage, Step: step, Steps: 5, Status: envelope.StatusRunning, PID: pid,
			StartedAt: now.Add(-3 * time.Hour), UpdatedAt: updated})
	}
	run := func(id, task string, pid int) string {
		return bdRunDisco(t, root, runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: task, Dir: root,
			CreatedAt: now.Add(-3 * time.Hour), Status: runcmd.StatusRunning, ExitCode: -1, PID: pid})
	}
	var registros []string
	// dueno vivo, latido de hace mas de una hora: en curso
	registros = append(registros, sobre("20260923T100000_vivo01", "vivo-latido-viejo", "", "spec", 1, yo, viejo))
	// dueno muerto, latido fresco: interrumpido sin esperar 15 minutos
	registros = append(registros, sobre("20260923T100000_muer01", "muerto-latido-fresco", "", "spec", 1, muerto, fresco))
	// dueno muerto en el paso run, con un sidecar sin pid y el latido fresco
	// (la regla de C1 lo daria vivo): manda el pid del sobre
	registros = append(registros, sobre("20260923T100000_muer02", "muerto-en-run", "20260923T100000_run002", "run", 3, muerto, fresco))
	registros = append(registros, run("20260923T100000_run002", "muerto-en-run", 0))
	// dueno vivo aunque el sidecar de su run diga un pid muerto
	registros = append(registros, sobre("20260923T100000_vivo02", "vivo-sidecar-muerto", "20260923T100000_run003", "run", 3, yo, viejo))
	registros = append(registros, run("20260923T100000_run003", "vivo-sidecar-muerto", muerto))
	// registros viejos, sin pid: la regla de C1
	registros = append(registros, sobre("20260923T100000_sinp01", "sin-pid-fresco", "", "spec", 1, 0, fresco))
	registros = append(registros, sobre("20260923T100000_sinp02", "sin-pid-viejo", "", "spec", 1, 0, viejo))
	antes := map[string][]byte{}
	for _, p := range registros {
		antes[p], _ = os.ReadFile(p)
	}

	card := func(slug string) Card {
		t.Helper()
		return Derive(Gather(root, "main", "high", bdItem(slug), now))
	}
	enCurso := func(slug, envID string) {
		t.Helper()
		c := card(slug)
		if c.Running == nil || c.Running.EnvelopeID != envID || c.Interrupted != nil {
			t.Fatalf("CA-333: [%s] el sobre %s esta en curso (su dueno vive): running=%+v interrupted=%+v", slug, envID, c.Running, c.Interrupted)
		}
	}
	interrumpido := func(slug, envID string) {
		t.Helper()
		c := card(slug)
		if c.Running != nil || c.Interrupted == nil || c.Interrupted.EnvelopeID != envID {
			t.Fatalf("CA-333: [%s] el sobre %s esta interrumpido (su dueno murio): running=%+v interrupted=%+v", slug, envID, c.Running, c.Interrupted)
		}
		if !c.NeedsDecision {
			t.Fatalf("CA-333: [%s] una tarjeta interrumpida necesita una decision: %+v", slug, c)
		}
	}
	enCurso("vivo-latido-viejo", "20260923T100000_vivo01")
	interrumpido("muerto-latido-fresco", "20260923T100000_muer01")
	interrumpido("muerto-en-run", "20260923T100000_muer02")
	enCurso("vivo-sidecar-muerto", "20260923T100000_vivo02")
	enCurso("sin-pid-fresco", "20260923T100000_sinp01")
	interrumpido("sin-pid-viejo", "20260923T100000_sinp02")

	for _, p := range registros {
		if ahora, _ := os.ReadFile(p); !bytes.Equal(ahora, antes[p]) {
			t.Fatalf("CA-333: leer el tablero no toca los registros: %s quedo distinto\nantes: %s\nahora: %s", p, antes[p], ahora)
		}
	}
}

// CA-333: un sobre abierto sin dueno vivo deja de ser interrumpido cuando
// otro sobre de la tarjeta empezo despues de su ultimo registro (relevo). Su
// archivo no cambia.
func TestCA333_RelevoDeUnSobreMuerto(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	yo := os.Getpid()
	muerto := bdPIDMuerto(t)
	abierto := func(id, task, rol string, pid int, started, updated time.Time) string {
		return bdSobreDisco(t, root, envelope.Record{ID: id, Role: rol, Provider: "claude", Task: task, Dir: root,
			RunID: "run-" + id, Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, PID: pid,
			StartedAt: started, UpdatedAt: updated})
	}
	cerrado := func(id, task, rol string, started time.Time) string {
		fin := started.Add(5 * time.Minute)
		return bdSobreDisco(t, root, envelope.Record{ID: id, Role: rol, Provider: "codex", Task: task, Dir: root,
			Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable, PID: muerto,
			StartedAt: started, UpdatedAt: fin, EndedAt: fin})
	}
	m := func(min int) time.Time { return now.Add(time.Duration(-min) * time.Minute) }
	var registros []string
	// relevado por un sobre posterior que ya cerro (volver a lanzar)
	registros = append(registros, abierto("20260923T100000_rel001", "relevo-cerrado", "writer", muerto, m(60), m(50)))
	registros = append(registros, cerrado("20260923T100000_rel002", "relevo-cerrado", "writer", m(40)))
	// relevado por un sobre posterior que sigue en curso (reanudar)
	registros = append(registros, abierto("20260923T100000_rel003", "relevo-en-curso", "writer", muerto, m(60), m(50)))
	registros = append(registros, abierto("20260923T100000_rel004", "relevo-en-curso", "writer", yo, m(40), m(1)))
	// relevado por un sobre de OTRO rol (pedir de nuevo, otra cosa)
	registros = append(registros, abierto("20260923T100000_rel005", "relevo-otro-rol", "test-writer", muerto, m(60), m(50)))
	registros = append(registros, cerrado("20260923T100000_rel006", "relevo-otro-rol", "arquitecto", m(45)))
	// un registro viejo sin pid (latido vencido) tambien se releva
	registros = append(registros, abierto("20260923T100000_rel007", "relevo-sin-pid", "writer", 0, m(180), m(120)))
	registros = append(registros, cerrado("20260923T100000_rel008", "relevo-sin-pid", "writer", m(60)))
	// sin relevo: el otro sobre empezo ANTES del ultimo registro del muerto
	registros = append(registros, abierto("20260923T100000_sin001", "sin-relevo", "writer", muerto, m(60), m(20)))
	registros = append(registros, cerrado("20260923T100000_sin002", "sin-relevo", "writer", m(40)))
	// sin relevo: el sobre posterior es de otra tarjeta
	registros = append(registros, abierto("20260923T100000_sin003", "sin-relevo-ajeno", "writer", muerto, m(60), m(50)))
	registros = append(registros, cerrado("20260923T100000_sin004", "otra-tarjeta", "writer", m(40)))
	// sin relevo: despues solo hubo un run suelto, no un sobre
	registros = append(registros, abierto("20260923T100000_sin005", "sin-relevo-run", "writer", muerto, m(60), m(50)))
	registros = append(registros, bdRunDisco(t, root, runcmd.Meta{ID: "20260923T100000_suel01", Provider: "codex", Role: "writer",
		Task: "sin-relevo-run", Dir: root, CreatedAt: m(40), Status: runcmd.StatusDone, EndedAt: m(35)}))
	// encadenado: el segundo muerto releva al primero, y queda interrumpido el segundo
	registros = append(registros, abierto("20260923T100000_enc001", "encadenado", "writer", muerto, m(60), m(50)))
	registros = append(registros, abierto("20260923T100000_enc002", "encadenado", "writer", muerto, m(40), m(30)))
	antes := map[string][]byte{}
	for _, p := range registros {
		antes[p], _ = os.ReadFile(p)
	}

	card := func(slug string) Card {
		t.Helper()
		return Derive(Gather(root, "main", "high", bdItem(slug), now))
	}
	relevado := func(slug string) Card {
		t.Helper()
		c := card(slug)
		if c.Interrupted != nil {
			t.Fatalf("CA-333: [%s] un sobre posterior de la tarjeta releva al muerto: interrupted=%+v", slug, c.Interrupted)
		}
		for _, a := range c.Actions {
			if a.ID == ActReanudar || a.ID == ActRelanzar {
				t.Fatalf("CA-333: [%s] relevado, no se ofrece %s: %v", slug, a.ID, acIDs(c))
			}
		}
		return c
	}
	interrumpido := func(slug, envID string) {
		t.Helper()
		c := card(slug)
		if c.Interrupted == nil || c.Interrupted.EnvelopeID != envID {
			t.Fatalf("CA-333: [%s] el sobre %s sigue interrumpido: interrupted=%+v running=%+v", slug, envID, c.Interrupted, c.Running)
		}
	}
	if c := relevado("relevo-cerrado"); c.Running != nil {
		t.Fatalf("CA-333: relevado por un sobre cerrado, nada esta en curso: %+v", c.Running)
	}
	if c := relevado("relevo-en-curso"); c.Running == nil || c.Running.EnvelopeID != "20260923T100000_rel004" {
		t.Fatalf("CA-333: relevado por el sobre que reanuda, que esta en curso: %+v", c.Running)
	}
	relevado("relevo-otro-rol")
	relevado("relevo-sin-pid")
	interrumpido("sin-relevo", "20260923T100000_sin001")
	interrumpido("sin-relevo-ajeno", "20260923T100000_sin003")
	interrumpido("sin-relevo-run", "20260923T100000_sin005")
	interrumpido("encadenado", "20260923T100000_enc002")

	// y leyo de verdad: los registros relevados siguen en la evidencia, tal cual
	ev := Gather(root, "main", "high", bdItem("relevo-cerrado"), now)
	if len(ev.Envelopes) != 2 {
		t.Fatalf("CA-333: el sobre relevado sigue en la evidencia (hoom no lo cierra ni lo borra): %+v", ev.Envelopes)
	}
	for _, e := range ev.Envelopes {
		if e.Record.ID == "20260923T100000_rel001" && (e.Record.Done() || e.Record.Status != envelope.StatusRunning) {
			t.Fatalf("CA-333: el registro del sobre relevado queda como estaba (en curso): %+v", e.Record)
		}
	}
	for _, p := range registros {
		if ahora, _ := os.ReadFile(p); !bytes.Equal(ahora, antes[p]) {
			t.Fatalf("CA-333: hoom nunca cierra ni reescribe el sobre relevado: %s quedo distinto\nantes: %s\nahora: %s", p, antes[p], ahora)
		}
	}
}
