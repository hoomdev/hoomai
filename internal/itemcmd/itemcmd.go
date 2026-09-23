// Package itemcmd implements `hoom item add | list | show`: the verbs of the
// card as a file. Parsing is pure and strict from day one (cliargs), so a
// request hoom does not understand is answered before the project is read,
// and it never leaves a half-written item behind.
package itemcmd

import (
	"io"

	"github.com/hoomdev/hoomai/internal/item"
)

// Subcommands.
const (
	SubAdd  = "add"
	SubList = "list"
	SubShow = "show"
)

// UsageText is the exact usage block of `hoom item` and the single source of
// that text.
const UsageText = `Uso: hoom item add "<titulo>" [--tipo t] [--prioridad p] [--presupuesto-usd x] [--pedido "<texto>"] [--slug s] [--json]
     hoom item list [--json]
     hoom item show <slug> [--json]

  --tipo t             feature|bug|refactor|seguridad|docs|test (default feature)
  --prioridad p        alta|media|baja (default media)
  --presupuesto-usd x  Tope de gasto de la tarjeta en USD (> 0; opcional)
  --pedido "<texto>"   Lo que va a recibir el arquitecto (opcional)
  --slug s             Slug del item (default: derivado del titulo); es el
                       nombre del spec (.hoom/specs/<slug>.md) y de la tarea
  --json               Emite el resultado como JSON en stdout

El item es un archivo en .hoom/items/<slug>.yaml que viaja en git. Nunca
guarda una columna: la columna sale de la evidencia ('hoom board').
Un titulo que empieza con '-' va despues de '--': hoom item add -- "-titulo"`

// Request is one parsed `hoom item` invocation.
type Request struct {
	Sub   string     // SubAdd | SubList | SubShow
	Draft item.Draft // add
	Slug  string     // show
	JSON  bool
}

// Parse is the syntactic AND vocabulary stage, pure: no disk, no
// environment. It returns a *cliargs.UsageError for anything hoom does not
// understand and cliargs.ErrHelp for -h/--help.
func Parse(args []string) (Request, error) {
	return Request{}, nil
}

// Execute runs a parsed request against the project at root. stdout carries
// the result (text or JSON); stderr the warnings about invalid items.
func Execute(root, base, blockOn string, req Request, stdout, stderr io.Writer) error {
	return nil
}
