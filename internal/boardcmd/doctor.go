package boardcmd

import (
	"errors"
	"io"
	"time"
)

// Problem ids, in the order the doctor checks a card.
const (
	ProbSpecEditado      = "spec-editado"
	ProbSinVeredicto     = "sin-veredicto"
	ProbVerdeVencido     = "verde-vencido"
	ProbBugDerivacion    = "bug-derivacion"
	ProbSobreHuerfano    = "sobre-huerfano"
	ProbSinSpec          = "sin-spec"
	ProbEvidenciaSinItem = "evidencia-sin-item"
)

// DoctorSinSpecDias: an item older than this without a spec is a problem.
const DoctorSinSpecDias = 14

// Problem is one place where the evidence does not add up, with the exact
// action that fixes it.
type Problem struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`   // the card, or the task; "" when none
	What   string `json:"what"`   // expert words
	Plain  string `json:"plain"`  // normal words; "" for a project problem
	Action string `json:"action"` // the exact command (or instruction)
	Ref    string `json:"ref"`    // the artifact it talks about
}

// DoctorReport is what `hoom board doctor` prints.
type DoctorReport struct {
	Problems []Problem `json:"problems"`
}

// DoctorOptions mirror the doctor verb's flags.
type DoctorOptions struct {
	JSON bool
}

// DoctorUsageText is the exact usage block of `hoom board doctor`.
const DoctorUsageText = `Uso: hoom board doctor [--json]

  --json   Emite los problemas como JSON en stdout

Lista cada lugar donde la evidencia de las tarjetas no es coherente, con su
accion exacta. Solo lee: no arregla nada. Sale con 0 siempre que entendio el
pedido; los que bloquean son 'hoom verify' y 'hoom check'.`

// DoctorOf is the card's own problems, in the fixed order. Pure: Derive calls
// it with the evidence and the card it just derived.
func DoctorOf(ev Evidence, c Card) []Problem {
	return []Problem{}
}

// Doctor builds the board and lists the problems of every card, in board
// order, followed by the project's own. Read-only.
func Doctor(root, base, blockOn string, now time.Time) (DoctorReport, error) {
	return DoctorReport{}, errors.New("sin implementar")
}

// ParseDoctorArgs is pure and strict, like ParseArgs.
func ParseDoctorArgs(args []string) (DoctorOptions, error) {
	return DoctorOptions{}, errors.New("sin implementar")
}

// RenderDoctor prints the report as text.
func RenderDoctor(w io.Writer, r DoctorReport) {}

// DoctorJSONBytes is the report as `hoom board doctor --json` prints it.
func DoctorJSONBytes(r DoctorReport) ([]byte, error) {
	return nil, errors.New("sin implementar")
}
