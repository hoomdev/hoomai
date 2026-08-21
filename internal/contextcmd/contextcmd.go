// Package contextcmd measures the HEALTH of the project's context with the
// same medicine the gates give the code: deterministic checks over files
// and dates, honest yellows, zero LLM. It never parses meaning — whether
// the vision is CORRECT is model work; whether it is stale, missing, or
// carrying open client questions is measurable right here. Context health
// informs, it never blocks: there is no red, and the exit code is always 0.
package contextcmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// QuestionMarker is the literal marker the analista contract mandates for
// open client questions. Literal on purpose: measuring the contract's exact
// form keeps the promise of "no LLM, no heuristics".
const QuestionMarker = "PREGUNTA PARA EL CLIENTE"

// Check status values. There is deliberately no "red".
const (
	StatusOK   = "ok"
	StatusWarn = "warn"
)

// Check is one deterministic observation about the context.
type Check struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | warn
	Detail string `json:"detail"`
	Action string `json:"action,omitempty"` // exact next step, in-band
}

// DocState describes one context artifact's existence and freshness.
type DocState struct {
	Exists    bool      `json:"exists"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

// IntakeState summarizes the client-document sources.
type IntakeState struct {
	Count      int       `json:"count"`
	NewestName string    `json:"newest_name,omitempty"`
	NewestAt   time.Time `json:"newest_at,omitempty"`
}

// Report is the context-health verdict-like summary. Status is verde or
// amarillo, never rojo: context debt is visible, not blocking.
type Report struct {
	Intake        IntakeState `json:"intake"`
	Vision        DocState    `json:"vision"`
	Backlog       DocState    `json:"backlog"`
	OpenQuestions int         `json:"open_questions"`
	Checks        []Check     `json:"checks"`
	Status        string      `json:"status"` // verde | amarillo
}

// docDate returns when a file last changed: last commit date via git, or
// mtime when the file is not committed yet (declared degradation, never a
// failure).
func docDate(root, rel string) time.Time {
	cmd := exec.Command("git", "log", "-1", "--format=%ct", "--", rel)
	cmd.Dir = root
	if out, err := cmd.Output(); err == nil {
		if secs, perr := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64); perr == nil && secs > 0 {
			return time.Unix(secs, 0).UTC()
		}
	}
	if st, err := os.Stat(filepath.Join(root, rel)); err == nil {
		return st.ModTime().UTC()
	}
	return time.Time{}
}

func docState(root, rel string) DocState {
	if _, err := os.Stat(filepath.Join(root, rel)); err != nil {
		return DocState{}
	}
	return DocState{Exists: true, UpdatedAt: docDate(root, rel)}
}

func intakeState(root string) IntakeState {
	s := IntakeState{}
	entries, err := os.ReadDir(filepath.Join(root, ".hoom", "intake"))
	if err != nil {
		return s
	}
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		s.Count++
		at := docDate(root, filepath.ToSlash(filepath.Join(".hoom", "intake", e.Name())))
		if at.After(s.NewestAt) {
			s.NewestAt = at
			s.NewestName = e.Name()
		}
	}
	return s
}

// countQuestions counts lines carrying the literal contract marker.
func countQuestions(root string, rels ...string) int {
	n := 0
	for _, rel := range rels {
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, QuestionMarker) {
				n++
			}
		}
	}
	return n
}

const (
	visionRel  = ".hoom/specs/00-vision.md"
	backlogRel = ".hoom/specs/backlog.md"
)

// Build assembles the context-health report for the project at root.
func Build(root string) Report {
	r := Report{
		Intake:  intakeState(root),
		Vision:  docState(root, visionRel),
		Backlog: docState(root, backlogRel),
	}
	r.OpenQuestions = countQuestions(root, visionRel, backlogRel)

	add := func(name, status, detail, action string) {
		r.Checks = append(r.Checks, Check{Name: name, Status: status, Detail: detail, Action: action})
	}

	nothing := r.Intake.Count == 0 && !r.Vision.Exists && !r.Backlog.Exists
	if nothing {
		add("contexto", StatusWarn,
			"sin intake, sin vision y sin backlog: el proyecto no tiene contexto del cliente",
			"arranca con la entrevista fundacional (Modo C del analista) o copia el documento del cliente a .hoom/intake/")
	} else {
		if r.Intake.Count == 0 {
			detail := "no hay documentos del cliente en .hoom/intake/"
			action := "copia el documento fuente, o deja constancia de la reconstruccion (Modo B del analista)"
			if r.Backlog.Exists {
				detail = "hay backlog pero el intake esta vacio: el backlog no tiene fuente rastreable"
			}
			add("fuentes", StatusWarn, detail, action)
		} else {
			add("fuentes", StatusOK, fmt.Sprintf("%d documento(s), el mas nuevo: %s", r.Intake.Count, r.Intake.NewestName), "")
		}

		if !r.Vision.Exists {
			status, detail, action := StatusWarn, "no existe "+visionRel, "pedile la vision al analista"
			if r.Intake.Count > 0 {
				detail = "hay intake sin destilar: no existe la vision"
				action = "pedile al analista que procese .hoom/intake/ (Modo A)"
			}
			add("vision", status, detail, action)
		} else {
			add("vision", StatusOK, "actualizada el "+r.Vision.UpdatedAt.Format("2006-01-02"), "")
		}

		if !r.Backlog.Exists {
			add("backlog", StatusWarn, "no existe "+backlogRel, "el analista lo produce junto con la vision")
		} else {
			add("backlog", StatusOK, "actualizado el "+r.Backlog.UpdatedAt.Format("2006-01-02"), "")
		}

		if r.Vision.Exists && r.Intake.Count > 0 && r.Intake.NewestAt.After(r.Vision.UpdatedAt) {
			add("frescura", StatusWarn,
				fmt.Sprintf("el intake tiene un documento (%s) mas nuevo que la ultima edicion de la vision: vision posiblemente desactualizada", r.Intake.NewestName),
				"pedile al analista que actualice la vision citando el documento nuevo")
		} else if r.Vision.Exists && r.Intake.Count > 0 {
			add("frescura", StatusOK, "la vision es posterior al ultimo documento del intake", "")
		}
	}

	if r.OpenQuestions > 0 {
		add("preguntas", StatusWarn,
			fmt.Sprintf("%d pregunta(s) abiertas para el cliente en vision/backlog", r.OpenQuestions),
			"mandaselas al cliente HOY; cada respuesta evita dias de retrabajo")
	} else if !nothing {
		add("preguntas", StatusOK, "sin preguntas abiertas", "")
	}

	r.Status = "verde"
	for _, c := range r.Checks {
		if c.Status == StatusWarn {
			r.Status = "amarillo" // nunca rojo: el contexto informa, no bloquea
		}
	}
	return r
}

// JSONBytes renders the report exactly as both the CLI and the Studio emit
// it, so the two representations cannot diverge.
func JSONBytes(root string) ([]byte, error) {
	return json.MarshalIndent(Build(root), "", "  ")
}

const (
	cReset  = "\033[0m"
	cGreen  = "\033[32m"
	cYellow = "\033[33m"
	cBold   = "\033[1m"
	cGray   = "\033[90m"
)

// Render prints the human view: one screen, same spirit as the verdict.
func Render(w io.Writer, r Report) {
	fmt.Fprintf(w, "\n%shoomAI - salud del contexto%s\n", cBold, cReset)
	fmt.Fprintln(w, strings.Repeat("-", 72))
	for _, c := range r.Checks {
		badge := cGreen + "OK  " + cReset
		if c.Status == StatusWarn {
			badge = cYellow + "AMAR" + cReset
		}
		fmt.Fprintf(w, " %s %-10s %s\n", badge, c.Name, c.Detail)
		if c.Action != "" {
			fmt.Fprintf(w, "      %sAccion: %s%s\n", cGray, c.Action, cReset)
		}
	}
	fmt.Fprintln(w, strings.Repeat("-", 72))
	color, label := cGreen, "VERDE"
	if r.Status == "amarillo" {
		color, label = cYellow, "AMARILLO"
	}
	fmt.Fprintf(w, " contexto: %s%s%s%s  (intake:%d preguntas-abiertas:%d)\n",
		cBold, color, label, cReset, r.Intake.Count, r.OpenQuestions)
	fmt.Fprintf(w, " %sel contexto informa, nunca bloquea: amarillo = deuda visible, no build roto%s\n", cGray, cReset)
	fmt.Fprintln(w)
}
