// Stage computes the theater view of a run: the hoomAI agent team as a cast
// of actors with honest attribution. It is a PURE function over the run's
// existing event stream — no new data sources, testable by the gates, and
// inheritable by any future UI. What cannot be attributed goes to the
// orquestador (the process that actually executes in headless mode), never
// to an invented role.
package runcmd

import "strings"

// FixedCast is the always-visible roster, in stable stage order.
var FixedCast = []string{
	"orquestador", "analista", "arquitecto", "designer", "scout",
	"writer", "test-writer", "reviewer", "characterizer",
}

// Actor is one card on the stage.
type Actor struct {
	Role       string `json:"role"`
	Known      bool   `json:"known"`
	Acts       int    `json:"acts"`
	LastDetail string `json:"last_detail,omitempty"`
	Active     bool   `json:"active"`
	// Open counts the delegations to this actor (agent events with a tool
	// id) that have not received their agent_end yet.
	Open int `json:"open"`
}

// StageView is the computed scene for one run.
type StageView struct {
	Status   string  `json:"status"`
	ExitCode int     `json:"exit_code"`
	Actors   []Actor `json:"actors"`
}

// normalizeRole maps a subagent identifier (e.g. "hoom-Test-Writer") to a
// fixed-cast role name, or "" if it is not one of ours.
func normalizeRole(agent string) string {
	name := strings.ToLower(strings.TrimSpace(agent))
	name = strings.TrimPrefix(name, "hoom-")
	for _, role := range FixedCast {
		if name == role {
			return role
		}
	}
	return ""
}

// Stage derives the scene from a run's state and events.
func Stage(info Run, events []Event) StageView {
	idx := map[string]int{}
	actors := make([]Actor, 0, len(FixedCast))
	for i, role := range FixedCast {
		actors = append(actors, Actor{Role: role, Known: true})
		idx[role] = i
	}

	lastDelegated := -1
	paired := false           // some delegation of the run carries a tool id
	owner := map[string]int{} // open tool id -> actor index
	for _, ev := range events {
		switch ev.Kind {
		case "agent":
			role := normalizeRole(ev.Agent)
			i, ok := idx[role]
			if role == "" || !ok {
				// reparto desconocido: tarjeta extra al final, nunca se pierde
				key := strings.ToLower(strings.TrimSpace(ev.Agent))
				if j, seen := idx[key]; seen {
					i = j
				} else {
					actors = append(actors, Actor{Role: key, Known: false})
					i = len(actors) - 1
					idx[key] = i
				}
			}
			actors[i].Acts++
			actors[i].LastDetail = ev.Detail
			lastDelegated = i
			if ev.ToolID != "" {
				paired = true
				if _, dup := owner[ev.ToolID]; !dup {
					owner[ev.ToolID] = i
					actors[i].Open++
				}
			}
		case "agent_end":
			// el subagente sale de escena: su delegacion recibio su
			// resultado. No es un acto, y uno sin su agent no cierra nada.
			if i, ok := owner[ev.ToolID]; ok && ev.ToolID != "" {
				delete(owner, ev.ToolID)
				actors[i].Open--
			}
		case "tool", "text":
			// atribucion honesta: sin agente explicito, actua el orquestador.
			// Un evento `system` NO entra aca: es la CLI hablando de si misma
			// —ocho lineas de hooks por invocacion— y contarlo le atribuiria
			// al orquestador un trabajo que nadie hizo.
			actors[0].Acts++
			actors[0].LastDetail = ev.Detail
		}
	}

	if info.Status == StatusRunning {
		actors[0].Active = true
		switch {
		case paired:
			// con ids, en escena esta quien tiene una delegacion abierta
			for i := range actors {
				if actors[i].Open > 0 {
					actors[i].Active = true
				}
			}
		case lastDelegated >= 0:
			// logs sin ids (u otro provider): la regla de siempre
			actors[lastDelegated].Active = true
		}
	}
	return StageView{Status: info.Status, ExitCode: info.ExitCode, Actors: actors}
}
