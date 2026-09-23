// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-310): quien escribio y quien reviso. Los logos salen primero del
// registro de review (que viaja en git) y solo despues de la telemetria
// local. El revisor sale SOLO de registros: un run de review sin registro no
// termino una review. Los autores de specs y los roles de solo lectura nunca
// son el writer.
package boardcmd

import (
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// tbRegistro es un registro de review de la tarjeta; con cuatro lentes sobre
// un verde de la tarjeta cuenta, con una sola no.
func tbRegistro(id string, en time.Duration, cuenta bool, provider, writer, cross string) reviewcmd.Record {
	lentes := []string{"reliability"}
	if cuenta {
		lentes = reviewcmd.Lentes
	}
	return reviewcmd.Record{ID: id, CreatedAt: bdT0.Add(en), Task: bdSlug, Spec: bdSpec, Fingerprint: "huella-1",
		VerdictID: "2026-09-22T15-30-00Z_aaaa1111", Verdict: "green", Lenses: lentes,
		Provider: provider, Writer: writer, Cross: cross, Findings: []string{}}
}

func tbRun(id, rol, provider string, en time.Duration) RunState {
	return RunState{Meta: runcmd.Meta{ID: id, Provider: provider, Role: rol, Task: bdSlug,
		Status: runcmd.StatusDone, CreatedAt: bdT0.Add(en), EndedAt: bdT0.Add(en + time.Minute)}}
}

func tbSobre(id, rol, provider string, en time.Duration) EnvelopeState {
	return EnvelopeState{Record: envelope.Record{ID: id, Role: rol, Provider: provider, Task: bdSlug,
		Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
		StartedAt: bdT0.Add(en), UpdatedAt: bdT0.Add(en + time.Minute), EndedAt: bdT0.Add(en + time.Minute)}}
}

func tbProv(t *testing.T, ca string, ev Evidence, writer, reviewer, cross string) {
	t.Helper()
	p := Derive(ev).Providers
	if p.Writer != writer || p.Reviewer != reviewer || p.Cross != cross {
		t.Fatalf("%s: providers debe ser {writer %q, reviewer %q, cross %q}, fue %+v", ca, writer, reviewer, cross, p)
	}
}

// CA-310: con un registro de review que cuenta, los tres salen de el, aunque
// haya uno mas nuevo que no cuenta y runs que digan otra cosa.
func TestCA310_ProvidersDelRegistroQueCuenta(t *testing.T) {
	ev := bdEv()
	ev.Reviews = []reviewcmd.Record{
		tbRegistro("r-nuevo-una-lente", 40*time.Minute, false, "gemini", "opencode", reviewcmd.CrossNo),
		tbRegistro("r-cuenta", 35*time.Minute, true, "codex", "claude", reviewcmd.CrossYes),
	}
	ev.Runs = []RunState{tbRun("run-w", "writer", "opencode", 20*time.Minute)}
	c := Derive(ev)
	if c.Evidence.ReviewID != "r-cuenta" {
		t.Fatalf("CA-310: el fixture tiene un registro que cuenta: %+v", c.Evidence)
	}
	tbProv(t, "CA-310", ev, "claude", "codex", reviewcmd.CrossYes)
}

// CA-310: si ninguno cuenta, manda el mas NUEVO de la tarjeta (por fecha, no
// por su posicion en la lista).
func TestCA310_ProvidersDelRegistroMasNuevo(t *testing.T) {
	ev := bdEv()
	ev.Reviews = []reviewcmd.Record{
		tbRegistro("r-medio", 30*time.Minute, false, "codex", "claude", reviewcmd.CrossYes),
		tbRegistro("r-nuevo", 50*time.Minute, false, "claude", "claude", reviewcmd.CrossNo),
		tbRegistro("r-viejo", 10*time.Minute, false, "gemini", "codex", reviewcmd.CrossYes),
	}
	tbProv(t, "CA-310", ev, "claude", "claude", reviewcmd.CrossNo)

	// una review con --same-provider: los dos logos iguales y cross no-cruzada
	ev = bdEv()
	ev.Reviews = []reviewcmd.Record{tbRegistro("r-mismo", 35*time.Minute, true, "claude", "claude", reviewcmd.CrossNo)}
	tbProv(t, "CA-310", ev, "claude", "claude", reviewcmd.CrossNo)
}

// CA-310: sin writer en el registro, el writer es el provider del run mas
// nuevo de la tarjeta con un rol que escribe codigo; reviewer y cross siguen
// saliendo del registro.
func TestCA310_WriterDeLosRunsSiElRegistroNoLoDice(t *testing.T) {
	ev := bdEv()
	ev.Reviews = []reviewcmd.Record{tbRegistro("r-sin-writer", 35*time.Minute, true, "codex", "", reviewcmd.CrossUnknown)}
	ev.Runs = []RunState{
		tbRun("run-review", "reviewer", "codex", 34*time.Minute),
		tbRun("run-arq", "arquitecto", "gemini", 30*time.Minute),
		tbRun("run-writer", "writer", "claude", 20*time.Minute),
		tbRun("run-tw", "test-writer", "opencode", 10*time.Minute),
	}
	tbProv(t, "CA-310", ev, "claude", "codex", reviewcmd.CrossUnknown)
}

// CA-310: sin registro, writer sale de los runs (rol que escribe, el mas
// nuevo) y reviewer y cross quedan vacios aunque haya un run de reviewer.
func TestCA310_SinRegistroNoHayRevisor(t *testing.T) {
	ev := bdEv()
	ev.Runs = []RunState{
		tbRun("run-review", "reviewer", "codex", 40*time.Minute),
		tbRun("run-refutador", "refutador", "codex", 38*time.Minute),
		tbRun("run-tw", "test-writer", "opencode", 25*time.Minute),
		tbRun("run-writer", "writer", "claude", 20*time.Minute),
	}
	tbProv(t, "CA-310", ev, "opencode", "", "")

	// un run sin rol (hoom run --task) escribe
	ev = bdEv()
	ev.Runs = []RunState{
		tbRun("run-scout", "scout", "codex", 40*time.Minute),
		tbRun("run-suelto", "", "gemini", 30*time.Minute),
		tbRun("run-writer", "writer", "claude", 20*time.Minute),
	}
	tbProv(t, "CA-310", ev, "gemini", "", "")

	// el characterizer escribe
	ev = bdEv()
	ev.Runs = []RunState{tbRun("run-char", "characterizer", "codex", 40*time.Minute)}
	tbProv(t, "CA-310", ev, "codex", "", "")
}

// CA-310: sin runs que escriban, el writer es el provider del sobre mas
// nuevo con un rol que escribe codigo.
func TestCA310_WriterDeLosSobres(t *testing.T) {
	ev := bdEv()
	ev.Runs = []RunState{tbRun("run-review", "reviewer", "codex", 40*time.Minute)}
	ev.Envelopes = []EnvelopeState{
		tbSobre("env-analista", "analista", "gemini", 30*time.Minute),
		tbSobre("env-writer", "writer", "claude", 20*time.Minute),
		tbSobre("env-tw", "test-writer", "opencode", 10*time.Minute),
	}
	tbProv(t, "CA-310", ev, "claude", "", "")

	// los runs que escriben ganan sobre los sobres
	ev.Runs = append(ev.Runs, tbRun("run-tw", "test-writer", "opencode", 5*time.Minute))
	tbProv(t, "CA-310", ev, "opencode", "", "")
}

// CA-310: los autores de specs y los roles de solo lectura nunca son el
// writer; sin datos, los tres son "".
func TestCA310_NuncaWriterSinDatos(t *testing.T) {
	ev := bdEv()
	var runs []RunState
	var sobres []EnvelopeState
	for i, rol := range []string{"arquitecto", "analista", "designer", "reviewer", "scout", "refutador", "orquestador"} {
		en := time.Duration(60-i) * time.Minute
		runs = append(runs, tbRun("run-"+rol, rol, "claude", en))
		sobres = append(sobres, tbSobre("env-"+rol, rol, "codex", en))
	}
	ev.Runs, ev.Envelopes = runs, sobres
	tbProv(t, "CA-310", ev, "", "", "")

	// sin registro ni telemetria (la tarjeta llego por git): no se inventa un autor
	tbProv(t, "CA-310", bdEv(), "", "", "")
}
