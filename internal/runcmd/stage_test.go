// Tests adversariales del spec .hoom/specs/studio-v4-escenario.md
// (CA-32..CA-38): el escenario se computa con atribucion HONESTA desde el
// stream existente — reparto fijo, orquestador por defecto, desconocidos
// sin romper nada.
package runcmd

import (
	"testing"
	"time"
)

func ev(kind, agent, detail string) Event {
	return Event{TS: time.Now().UTC(), Kind: kind, Agent: agent, Detail: detail}
}

func actorByRole(t *testing.T, sv StageView, role string) Actor {
	t.Helper()
	for _, a := range sv.Actors {
		if a.Role == role {
			return a
		}
	}
	t.Fatalf("no existe el actor %q en %+v", role, sv.Actors)
	return Actor{}
}

// CA-32: el reparto fijo completo esta SIEMPRE, en orden estable, con
// acts 0 para quien no actuo.
func TestCA32_RepartoFijoCompleto(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, nil)
	if len(sv.Actors) != len(FixedCast) {
		t.Fatalf("CA-32: esperaba %d actores, hay %d", len(FixedCast), len(sv.Actors))
	}
	for i, role := range FixedCast {
		if sv.Actors[i].Role != role {
			t.Fatalf("CA-32: orden inestable: posicion %d es %q, esperaba %q", i, sv.Actors[i].Role, role)
		}
		if sv.Actors[i].Acts != 0 {
			t.Fatalf("CA-32: %q sin actividad debe llevar acts 0", role)
		}
		if !sv.Actors[i].Known {
			t.Fatalf("CA-32: el reparto fijo es conocido: %q", role)
		}
	}
}

// CA-33: una delegacion a hoom-scout suma participacion a la tarjeta scout
// y conserva el encargo.
func TestCA33_DelegacionSumaAlRol(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, []Event{
		ev("agent", "hoom-scout", "Task: mapea el modulo de precios"),
		ev("agent", "hoom-scout", "Task: revisa las migraciones"),
	})
	scout := actorByRole(t, sv, "scout")
	if scout.Acts != 2 {
		t.Fatalf("CA-33: scout debia tener 2 participaciones, tiene %d", scout.Acts)
	}
	if scout.LastDetail != "Task: revisa las migraciones" {
		t.Fatalf("CA-33: el ultimo encargo debe conservarse: %q", scout.LastDetail)
	}
}

// CA-34: tool y text sin agente van al orquestador; ningun otro rol
// registra actividad que no tuvo.
func TestCA34_AtribucionHonesta(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, []Event{
		ev("tool", "", "Read: app/Models/Precio.php"),
		ev("text", "", "analizando el modelo"),
		ev("start", "", "init"), // start/end/error no atribuyen actividad
	})
	if orq := actorByRole(t, sv, "orquestador"); orq.Acts != 2 {
		t.Fatalf("CA-34: el orquestador debia registrar 2 actos, tiene %d", orq.Acts)
	}
	for _, role := range FixedCast[1:] {
		if a := actorByRole(t, sv, role); a.Acts != 0 {
			t.Fatalf("CA-34: %q registra actividad que no tuvo (%d)", role, a.Acts)
		}
	}
}

// CA-35: run terminado => nadie queda en escena y los conteos sobreviven
// con el estado final.
func TestCA35_TerminadoCongelaLaEscena(t *testing.T) {
	events := []Event{
		ev("agent", "hoom-writer", "Task: implementa el spec"),
		ev("tool", "", "Edit: app/Services/Precios.php"),
	}
	for _, status := range []string{StatusDone, StatusError, StatusCanceled} {
		sv := Stage(Run{Status: status, ExitCode: 0}, events)
		if sv.Status != status {
			t.Fatalf("CA-35: el estado final debe conservarse: %q", sv.Status)
		}
		for _, a := range sv.Actors {
			if a.Active {
				t.Fatalf("CA-35: con run %s nadie queda active: %+v", status, a)
			}
		}
		if actorByRole(t, sv, "writer").Acts != 1 || actorByRole(t, sv, "orquestador").Acts != 1 {
			t.Fatalf("CA-35: los conteos deben sobrevivir al cierre")
		}
	}
}

// CA-36: mientras corre, el ultimo rol delegado esta en escena junto al
// orquestador; una nueva delegacion pasa la escena.
func TestCA36_EscenaSigueALaUltimaDelegacion(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, []Event{
		ev("agent", "hoom-scout", "Task: mapea"),
		ev("agent", "hoom-arquitecto", "Task: escribi el spec"),
	})
	if !actorByRole(t, sv, "orquestador").Active {
		t.Fatal("CA-36: el orquestador narra mientras el run corre")
	}
	if actorByRole(t, sv, "scout").Active {
		t.Fatal("CA-36: la escena debe haber pasado del scout al arquitecto")
	}
	if !actorByRole(t, sv, "arquitecto").Active {
		t.Fatal("CA-36: el ultimo delegado debe estar en escena")
	}
}

// CA-37: un subagente desconocido gana una tarjeta extra known:false al
// final, sin romper el reparto; la normalizacion tolera mayusculas.
func TestCA37_DesconocidosSinRomper(t *testing.T) {
	sv := Stage(Run{Status: StatusRunning}, []Event{
		ev("agent", "hoom-Test-Writer", "Task: tests desde el spec"), // variante de mayusculas
		ev("agent", "custom-refactorer", "Task: algo raro"),
		ev("agent", "custom-refactorer", "Task: mas raro"),
	})
	if tw := actorByRole(t, sv, "test-writer"); tw.Acts != 1 {
		t.Fatalf("CA-37: la variante con mayusculas debe mapear a test-writer: %+v", tw)
	}
	extra := sv.Actors[len(sv.Actors)-1]
	if extra.Role != "custom-refactorer" || extra.Known || extra.Acts != 2 {
		t.Fatalf("CA-37: el desconocido va al final con known:false y sus actos: %+v", extra)
	}
	if len(sv.Actors) != len(FixedCast)+1 {
		t.Fatalf("CA-37: una sola tarjeta extra por desconocido, hay %d", len(sv.Actors))
	}
}

// CA-38: un run de provider sin stream (solo text) produce un escenario
// valido donde solo el orquestador registra actividad.
func TestCA38_SoloTextoEsSoloOrquestador(t *testing.T) {
	sv := Stage(Run{Status: StatusDone}, []Event{
		ev("text", "", "salida cruda linea 1"),
		ev("text", "", "salida cruda linea 2"),
		ev("text", "", "salida cruda linea 3"),
	})
	if orq := actorByRole(t, sv, "orquestador"); orq.Acts != 3 {
		t.Fatalf("CA-38: todo el texto va al orquestador: %d", orq.Acts)
	}
	for _, a := range sv.Actors[1:] {
		if a.Acts != 0 {
			t.Fatalf("CA-38: %q no puede registrar actividad en un run degradado", a.Role)
		}
	}
}
