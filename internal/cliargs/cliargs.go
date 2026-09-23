// Package cliargs is hoom's argument discipline: a request the binary does
// not understand is REJECTED, never half-executed. It knows no verb — the
// verb brings its own FlagSet and its own usage block — so the rest of the
// debt is paid one line per verb, without rethinking the mechanism.
//
// CUIDADO: Strict rejects ANY positional argument, so it cannot be applied as
// is to the verbs that take operands by contract ('init <dir>',
// 'run/agent/review "<prompt>"', 'finding add "<desc>"', 'task <slug>',
// 'spec approve <ruta>'). It is born for the verbs that have no operands in
// any syntax; copying it onto one that does would break that verb.
package cliargs

import (
	"errors"
	"flag"
	"io"
	"strconv"
	"strings"
)

// UsageError means hoom did not understand the request. It is ALWAYS exit 2
// and it never has effects: nothing on disk, no verdict, no live events.
type UsageError struct {
	Verb   string // "verify"
	Reason string // una linea: que no se entendio
	// Action is the last line of the message and it stays EMPTY for hoom's
	// verbs: their action already is the last line of Usage (the verb's usage
	// block is the single source of that text — see verifycmd.UsageText).
	// Writing it here too would print the action twice, so NewUsageError does
	// not take it; the field exists for a caller whose usage block does not
	// carry its own action.
	Action string
	Usage  string // bloque de uso exacto del verbo
}

// NewUsageError is the ONLY constructor: no verb assembles the struct by hand,
// so there is a single place that decides where each piece of the message
// lives — and the action is not one of its arguments, because it is not a
// second home for that text.
func NewUsageError(verb, reason, usage string) *UsageError {
	return &UsageError{Verb: verb, Reason: reason, Usage: usage}
}

func (e *UsageError) Error() string {
	return "hoom " + e.Verb + ": " + e.Reason + "\n\n" + e.Usage + "\n" + e.Action
}

// ExitCode is 2 and invariant: "no entendi el pedido" never gets confused
// with "entendi y salio rojo" (1). Without that separation a CI cannot tell a
// broken codebase from a badly written script.
func (e *UsageError) ExitCode() int { return 2 }

// ErrHelp is -h/--help. Asking for the usage is not an error: it goes to
// stdout and exits 0.
var ErrHelp = errors.New("uso solicitado")

// Strict parses args with ContinueOnError and the flag package's output
// silenced, and returns an *UsageError on an undefined flag, a defined flag
// without its value, or ANY positional argument.
func Strict(fs *flag.FlagSet, args []string, verb, usage string) error {
	fail := func(reason string) error {
		return NewUsageError(verb, reason, usage)
	}
	// hoom is the owner of its own message: with ExitOnError the process dies
	// INSIDE the flag package, printing a usage that hoom never wrote and a
	// syntax its documentation does not use.
	fs.Init(fs.Name(), flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	if err := escanear(fs, args, fail, nil); err != nil {
		return err
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return ErrHelp
		}
		return fail(razonResidual(err))
	}
	if fs.NArg() > 0 {
		return fail(posicional(fs.Arg(0)))
	}
	return nil
}

// FirstEmptyValue reports the name — without dashes — of the FIRST flag
// WRITTEN with an empty value ('--gate ""', '--spec='), or "" if there is
// none. It reads argv and not the FlagSet on purpose: the FlagSet only keeps
// the LAST value of a repeated flag, so '--gate "" --gate test' would hide the
// empty occurrence behind the repetition. Boolean flags carry no value and are
// never reported.
//
// It judges nothing: it reports what was written, over args that already went
// through Strict.
func FirstEmptyValue(fs *flag.FlagSet, args []string) string {
	var vacio string
	_ = escanear(fs, args, func(string) error { return nil },
		func(nombre, valor string) bool {
			if valor == "" {
				vacio = nombre
			}
			return vacio == "" // la primera vacia corta el recorrido
		})
	return vacio
}

// escanear replays the flag package's parsing rules to judge the request
// BEFORE the flag package judges it. That is the whole trick: the verdict on
// the arguments is hoom's, in hoom's words, and it is testable because no one
// calls os.Exit inside a library.
//
// visita, when it is not nil, receives every NON-boolean flag with its value
// AS WRITTEN — one call per occurrence, so a repetition cannot mask an earlier
// one — and stops the walk by returning false.
func escanear(fs *flag.FlagSet, args []string, fail func(string) error, visita func(nombre, valor string) bool) error {
	ayuda := false
	for i := 0; i < len(args); i++ {
		s := args[i]
		if len(s) < 2 || s[0] != '-' {
			return fail(posicional(s))
		}
		guiones := 1
		if s[1] == '-' {
			guiones = 2
			if len(s) == 2 {
				// '--' no habilita operandos: el verbo no tiene operandos.
				if i+1 < len(args) {
					return fail(posicional(args[i+1]))
				}
				break // '--' al final: ya no queda nada que juzgar
			}
		}
		nombre, valor := s[guiones:], ""
		if nombre[0] == '-' || nombre[0] == '=' {
			return fail("sintaxis de flag invalida: " + strconv.Quote(s))
		}
		conValor := false
		if j := strings.IndexByte(nombre, '='); j >= 0 {
			nombre, valor, conValor = nombre[:j], nombre[j+1:], true
		}
		f := fs.Lookup(nombre)
		if f == nil {
			if nombre == "h" || nombre == "help" {
				// La ayuda se resuelve al final, no aca: '--help show' y
				// '--help --bogus' siguen siendo pedidos rotos, y un pedido
				// roto no puede salir 0 por haber pedido ayuda primero.
				ayuda = true
				continue
			}
			return fail("flag desconocido: " + citar("--"+nombre))
		}
		if esBool(f) {
			continue // un flag booleano jamas consume el argumento siguiente
		}
		if !conValor {
			if i+1 >= len(args) {
				return fail("--" + nombre + " necesita un valor")
			}
			i++ // el valor de un flag no es un posicional
			valor = args[i]
		}
		if visita != nil && !visita(nombre, valor) {
			return nil
		}
	}
	if ayuda {
		return ErrHelp
	}
	return nil
}

func posicional(tok string) string {
	return "argumento posicional no reconocido: " + strconv.Quote(tok)
}

// citar quotes a token that comes from argv ONLY when quoting changes it: a
// newline in a flag name forges a second line on stderr, and an escape
// sequence repaints a log. A clean name travels as written because the message
// has to read like what was typed ('flag desconocido: --bogus').
func citar(tok string) string {
	q := strconv.Quote(tok)
	if q == `"`+tok+`"` {
		return tok
	}
	return q
}

func esBool(f *flag.Flag) bool {
	b, ok := f.Value.(interface{ IsBoolFlag() bool })
	return ok && b.IsBoolFlag()
}

// razonResidual translates whatever the flag package still rejects after the
// scan — a value that does not convert — into hoom's language. The foreign
// text never leaks: only the flag name travels, the wording is hoom's.
func razonResidual(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, "for flag -"); i >= 0 {
		return "valor invalido para --" + hastaDosPuntos(msg[i+len("for flag -"):])
	}
	if rest, ok := strings.CutPrefix(msg, "invalid boolean flag "); ok {
		return "valor invalido para --" + hastaDosPuntos(rest)
	}
	return "no pude interpretar los argumentos"
}

func hastaDosPuntos(s string) string {
	if i := strings.IndexByte(s, ':'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimLeft(s, "-")
}

// Operands parses a verb that takes exactly n operands (`item add "<titulo>"`,
// `item show <slug>`): flags may go before or after them, and after `--`
// everything is an operand. It returns the operands, or a *UsageError on an
// undefined flag, a flag without its value, a flag WRITTEN with an empty
// value, an operand too many or too few, or an empty operand; -h/--help is
// ErrHelp.
func Operands(fs *flag.FlagSet, args []string, verb, usage string, n int) ([]string, error) {
	return nil, nil
}
