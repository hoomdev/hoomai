package taskcmd

import "fmt"

// Kinds of ReadyError: which of the close conditions failed.
const (
	ReadySinTarea      = "sin-tarea"      // the task worktree does not exist
	ReadySinGuardar    = "sin-guardar"    // uncommitted changes in the worktree
	ReadySinVeredicto  = "sin-veredicto"  // no verdict at all
	ReadySoloParciales = "solo-parciales" // only partial (--gate) verdicts
	ReadyRojo          = "rojo"           // the latest complete verdict is red
	ReadyHuella        = "huella"         // the tree changed after the green
	// ReadyTopePendientes: accepting would leave more open mediums in the
	// project than findings.max_pendientes (ciclo-de-review-acotado).
	ReadyTopePendientes = "tope-pendientes"
	// ReadySinCierre (gates-por-ruta): the latest complete verdict has a
	// gate in verdict.StatusNoAplica, so it is not a verify de cierre. The
	// message is FormatoSinCierre.
	ReadySinCierre = "sin-cierre"
)

// FormatoSinCierre (gates-por-ruta) is the message of ReadySinCierre,
// exactly: %q is the task's slug and %s the gates in verdict.StatusNoAplica
// of the latest complete verdict, in the verdict's order, joined by ", ".
// With the slug precios and the gates static and mutation:
//
//	el ultimo veredicto de "precios" no es de cierre: no aplicaron static, mutation.
//	  Accion: ejecuta 'hoom verify --cierre' dentro del worktree y commitea
const FormatoSinCierre = "el ultimo veredicto de %q no es de cierre: no aplicaron %s.\n  Accion: ejecuta 'hoom verify --cierre' dentro del worktree y commitea"

// ReadyError is what Ready returns: the same message as always, plus the
// kind, so a reader never parses a message written for people.
type ReadyError struct {
	Kind string
	Msg  string
}

func (e *ReadyError) Error() string { return e.Msg }

func notReady(kind, format string, args ...any) *ReadyError {
	return &ReadyError{Kind: kind, Msg: fmt.Sprintf(format, args...)}
}
