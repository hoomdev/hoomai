package boardcmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// meterGates are the gate segments of the meter, in order.
var meterGates = []string{"build", "static", "test"}

// meterOf is the evidence meter: a segment fills only with a fact that exists
// and holds today. In Hecho the fingerprint is not asked for: a closed card is
// not re-evaluated against what happened after.
func meterOf(ev Evidence, col string, m CardEvidence) []Segment {
	seg := func(id, label, state, detail string) Segment {
		return Segment{ID: id, Label: label, State: state, Detail: detail}
	}
	out := []Segment{}

	lintOK := ev.SpecExists && len(ev.LintIssues) == 0
	switch {
	case !ev.SpecExists:
		out = append(out, seg("spec", "spec aprobado", SegFalta, "todavia no hay spec"))
	case !lintOK:
		out = append(out, seg("spec", "spec aprobado", SegFalta, "el spec no esta completo"))
	case ev.Approval == approval.StatusApproved:
		out = append(out, seg("spec", "spec aprobado", SegHecho, "aprobado para el contenido actual"))
	case ev.Approval == approval.StatusInvalidated:
		out = append(out, seg("spec", "spec aprobado", SegFalta, "el spec cambio despues de tu aprobacion"))
	default:
		out = append(out, seg("spec", "spec aprobado", SegFalta, "espera tu aprobacion"))
	}

	if lintOK {
		untraced, byCmd := set(ev.Untraced), set(ev.ByCommand)
		for _, id := range ev.Criteria {
			switch {
			case untraced[id]:
				out = append(out, seg(id, id, SegFalta, "sin prueba"))
			case byCmd[id]:
				out = append(out, seg(id, id, SegHecho, "con comando de verificacion"))
			default:
				out = append(out, seg(id, id, SegHecho, "con prueba"))
			}
		}
	}

	v := ev.Verdict
	for _, g := range meterGates {
		st := ""
		if v != nil {
			st = gateStatus(v, g)
		}
		switch {
		case v == nil:
			out = append(out, seg(g, g, SegFalta, "todavia no hay veredicto"))
		case st == "":
			out = append(out, seg(g, g, SegNoAplica, "el proyecto no declara el gate "+g))
		case st != verdict.StatusPass:
			out = append(out, seg(g, g, SegFalta, fmt.Sprintf("el gate %s no paso (%s)", g, st)))
		case col != ColHecho && !m.FingerprintMatch:
			out = append(out, seg(g, g, SegFalta, "el codigo cambio despues del veredicto"))
		default:
			out = append(out, seg(g, g, SegHecho, "paso en el veredicto vigente"))
		}
	}

	lines, required := reviewRequired(v)
	switch {
	case m.ReviewID != "":
		out = append(out, seg("review", "review", SegHecho, "revision de 4 lentes registrada"))
	case v == nil:
		out = append(out, seg("review", "review", SegFalta, "todavia no hay veredicto"))
	case !required:
		out = append(out, seg("review", "review", SegNoAplica,
			fmt.Sprintf("el cambio es chico (%d lineas): no exige la revision de 4 lentes", lines)))
	default:
		out = append(out, seg("review", "review", SegFalta, "falta la revision de 4 lentes"))
	}
	return out
}

// plainVerdict says a red verdict in the normal mode's words.
func plainVerdict(v *verdict.Verdict) string {
	failed := failedGates(v)
	switch len(failed) {
	case 0:
		return "la verificacion dio rojo"
	case 1:
		return "la verificacion dio rojo: fallo el gate " + failed[0]
	default:
		return "la verificacion dio rojo: fallaron los gates " + strings.Join(failed, ", ")
	}
}

// plainReady says what taskcmd.Ready refused, by its kind: a message written
// for people is never parsed.
func plainReady(kind string) string {
	switch kind {
	case taskcmd.ReadySinTarea:
		return "falta el espacio de trabajo de la tarjeta"
	case taskcmd.ReadySinGuardar:
		return "hay cambios sin guardar en el espacio de trabajo"
	case taskcmd.ReadySinVeredicto, taskcmd.ReadySoloParciales:
		return "falta verificar el espacio de trabajo completo"
	case taskcmd.ReadyRojo:
		return "la ultima verificacion del espacio de trabajo dio rojo"
	case taskcmd.ReadyHuella:
		return "el espacio de trabajo cambio despues del ultimo verde"
	}
	return "el espacio de trabajo no esta listo para cerrar"
}

// stageGloss says where an envelope cut, without its note (it may carry
// paths).
var stageGloss = map[string]string{
	"spec":   "el spec no tenia aprobacion vigente",
	"aislar": "no se pudo preparar el espacio ciego",
	"run":    "el agente termino con error",
	"scope":  "escribio fuera de lo que le toca",
	"verify": "la verificacion dio rojo",
	"check":  "el control final no paso",
}

func plainEnvelope(rec envelope.Record) string {
	role := rec.Role
	if role == "" {
		role = "rol"
	}
	if rec.Status == envelope.StatusNoDelivery {
		return "el " + role + " no entrego: no dejo ningun archivo"
	}
	gloss, ok := stageGloss[rec.Stage]
	if !ok {
		gloss = "corto en el paso " + rec.Stage
	}
	return "el " + role + " no entrego: " + gloss
}

// crewOf answers who wrote and who reviewed: first the review record, which
// travels in Git; then, for the writer only, the local telemetry. A review
// run without a record did not finish a review, so it names no reviewer.
func crewOf(ev Evidence, reviewID string) Providers {
	var p Providers
	var rec *reviewcmd.Record
	for i := range ev.Reviews {
		r := ev.Reviews[i]
		if r.Task != ev.Item.Slug {
			continue
		}
		if r.ID == reviewID {
			rec = &r
			break
		}
		if rec == nil || r.CreatedAt.After(rec.CreatedAt) {
			rec = &r
		}
	}
	if rec != nil {
		p.Reviewer, p.Writer, p.Cross = rec.Provider, rec.Writer, rec.Cross
	}
	if p.Writer != "" {
		return p
	}
	var at time.Time
	for _, r := range ev.Runs {
		if r.Meta.Provider != "" && agents.WritesCode(r.Meta.Role) && (p.Writer == "" || r.Meta.CreatedAt.After(at)) {
			p.Writer, at = r.Meta.Provider, r.Meta.CreatedAt
		}
	}
	if p.Writer != "" {
		return p
	}
	for _, e := range ev.Envelopes {
		rec := e.Record
		if rec.Provider != "" && agents.WritesCode(rec.Role) && (p.Writer == "" || rec.StartedAt.After(at)) {
			p.Writer, at = rec.Provider, rec.StartedAt
		}
	}
	return p
}

// count renders "1 problema" / "3 problemas".
func count(n int, uno, varios string) string {
	return fmt.Sprintf("%d %s", n, plural(n, uno, varios))
}

func set(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}
