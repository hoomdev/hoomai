// Package tarjetacmd holds the card verbs (spec verbos-de-la-tarjeta): hoom
// usar, pedir, siguiente, aprobar, integrar, guardar and reanudar. Each one
// takes ONE action of the card (boardcmd.Derive decides the list, its why and
// its proposals) and runs it with the same function as the Studio, or says
// why not. They deduce the card (operand, folder, hoom usar) and say where
// they got it from before doing anything.
package tarjetacmd

import (
	"io"
	"os"
)

// Verbos are the card verbs, in the order `hoom help` lists them.
var Verbos = []string{"usar", "pedir", "siguiente", "aprobar", "integrar", "guardar", "reanudar"}

// Where a verb got the card from, as the line `tarjeta: <slug> (<origen>)`
// says it.
const (
	OrigenArgumento = "argumento"
	OrigenCarpeta   = "carpeta"
	OrigenUsar      = "hoom usar"
)

// ArchivoFijada is where `hoom usar` keeps the fixed card, relative to the
// project: one line with the slug. Local state: outside git and the
// fingerprint.
const ArchivoFijada = ".hoom/cache/tarjeta"

// Entorno is the process boundary of a verb: where it was called from, its
// standard streams, whether it runs on a terminal and the environment it
// reads ($VISUAL, $EDITOR). Tests replace it whole.
type Entorno struct {
	Dir      string    // the working directory
	In       io.Reader // answers to the confirmation of a signature
	Out      io.Writer // what the verb says
	Err      io.Writer // errors (and, with --json, the lines that are not the JSON)
	Terminal bool      // stdin and stdout are a terminal: the editor and the questions are allowed
	Getenv   func(string) string
}

// Real is the Entorno of the hoom process.
func Real() Entorno {
	panic("sin implementar")
}

// Deduccion is the card a verb talks about and the project it acts on.
type Deduccion struct {
	Root   string // the project: the main tree of the repository, never a workspace
	Slug   string
	Origen string // OrigenArgumento | OrigenCarpeta | OrigenUsar
}

// Fallo is a verb that stops with an exit code and a message.
type Fallo struct {
	Code int
	Msg  string
}

func (f *Fallo) Error() string { return f.Msg }

// Deducir finds the card in the contract's order: the operand (if not
// empty), the folder (inside <proyecto>/.hoom/worktrees/<slug>/ at any
// depth, a blind tree nested in it included), the card fixed with hoom usar.
// Without any it is a *Fallo with Code 2; a slug that is not a card, or a
// fixed card that no longer exists, a *Fallo with Code 1.
func Deducir(dir, operando string) (Deduccion, error) {
	panic("sin implementar")
}

// Run executes one card verb and returns its exit code: 0 done, 1 the card
// (or the job) said no, 2 the request was not understood. A verb of the
// envelope or the review returns what `hoom agent` or `hoom review` would.
func Run(verb string, args []string, e Entorno) int {
	panic("sin implementar")
}

// Uso is the usage block of a verb ("" for one that is not a card verb).
func Uso(verb string) string {
	return usos[verb]
}

var usos = map[string]string{
	"usar": `Uso: hoom usar [<slug> | --ninguna]

  <slug>       fija la tarjeta de la que hablan los verbos de la tarjeta
  (sin nada)   muestra la tarjeta fijada
  --ninguna    quita la tarjeta fijada

Se guarda en .hoom/cache/tarjeta: es estado local, no viaja en git.`,

	"pedir": `Uso: hoom pedir <rol> [<slug>] [-m "<pedido>"] [--provider p] [--model m]
                  [--effort e] [--budget-usd x] [--timeout d] [--json]

  <rol>            arquitecto, test-writer, writer o reviewer
  <slug>           la tarjeta; sin el: la de la carpeta o la de hoom usar
  -m <pedido>      lo que el rol tiene que hacer; sin -m se abre tu editor
                   con el pedido que propone la tarjeta (el reviewer arma el suyo)
  --provider p     CLI de IA; vacio = agents.<rol>.provider o el de la tarjeta
  --model m        modelo; vacio = agents.<rol>.model o el del provider
  --effort e       esfuerzo; vacio = agents.<rol>.effort o el del provider
  --budget-usd x   tope en USD; vacio = agents.<rol>.budget_usd o lo que queda
  --timeout d      tiempo limite, entre 1m y 12h (ej. 45m, 1h30m)
  --json           el resultado de hoom agent (o hoom review) como JSON`,

	"siguiente": `Uso: hoom siguiente [<slug>] [-m "<pedido>"] [--provider p] [--model m]
                      [--effort e] [--budget-usd x] [--timeout d] [--json]

Hace la accion principal de la tarjeta. Si es pedirle trabajo a un rol, es
hoom pedir <rol> (con los mismos flags); nunca aprueba ni integra: en Tu
aprobacion y Tu aceptacion dice el comando que es tuyo.`,

	"aprobar": `Uso: hoom aprobar [<slug>] [--si]

  --si   no pregunta (sin terminal es obligatorio)

Aprueba el spec de la tarjeta con tu identidad, en su espacio de trabajo.`,

	"integrar": `Uso: hoom integrar [<slug>] [--si]

  --si   no pregunta (sin terminal es obligatorio)

Cierra la tarea de la tarjeta (hoom task done): la tarjeta pasa a Hecho.`,

	"guardar": `Uso: hoom guardar [<slug>]

Guarda en git lo que la tarjeta tiene sin guardar (hoom item save).`,

	"reanudar": `Uso: hoom reanudar [<slug>] [-m "<pedido>"] [--model m] [--effort e]
                     [--budget-usd x] [--timeout d] [--json]

Retoma la sesion del trabajo cortado de la tarjeta, con su provider. Sin -m
se abre tu editor con el pedido que propone la tarjeta.`,
}

var _ = os.Getenv
