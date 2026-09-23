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
)

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
