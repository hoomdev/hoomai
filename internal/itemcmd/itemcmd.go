// Package itemcmd implements `hoom item add | list | show`: the verbs of the
// card as a file. Parsing is pure and strict from day one (cliargs), so a
// request hoom does not understand is answered before the project is read,
// and it never leaves a half-written item behind.
package itemcmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
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
	if len(args) == 0 {
		return Request{}, uso("item", "falta el subcomando (add|list|show)")
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "-h", "--help", "help":
		return Request{}, cliargs.ErrHelp
	case SubAdd:
		return parseAdd(rest)
	case SubList:
		fs := flag.NewFlagSet("item list", flag.ContinueOnError)
		asJSON := fs.Bool("json", false, "emitir los items como JSON")
		if err := cliargs.Strict(fs, rest, "item list", UsageText); err != nil {
			return Request{}, err
		}
		return Request{Sub: SubList, JSON: *asJSON}, nil
	case SubShow:
		fs := flag.NewFlagSet("item show", flag.ContinueOnError)
		asJSON := fs.Bool("json", false, "emitir el item y su tarjeta como JSON")
		ops, err := cliargs.Operands(fs, rest, "item show", UsageText, 1)
		if err != nil {
			return Request{}, err
		}
		if !item.ValidSlug(ops[0]) {
			return Request{}, uso("item show", fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", ops[0]))
		}
		return Request{Sub: SubShow, Slug: ops[0], JSON: *asJSON}, nil
	}
	return Request{}, uso("item", fmt.Sprintf("subcomando desconocido %q (add|list|show)", sub))
}

func parseAdd(args []string) (Request, error) {
	fs := flag.NewFlagSet("item add", flag.ContinueOnError)
	tipo := fs.String("tipo", "", "tipo del item")
	prioridad := fs.String("prioridad", "", "prioridad del item")
	presupuesto := fs.Float64("presupuesto-usd", 0, "tope de gasto en USD")
	pedido := fs.String("pedido", "", "pedido para el arquitecto")
	slug := fs.String("slug", "", "slug del item")
	asJSON := fs.Bool("json", false, "emitir el item como JSON")
	ops, err := cliargs.Operands(fs, args, "item add", UsageText, 1)
	if err != nil {
		return Request{}, err
	}
	d := item.Draft{Titulo: ops[0], Tipo: *tipo, Prioridad: *prioridad, Pedido: *pedido, Slug: *slug}
	// Only a WRITTEN budget is one: its absence means "no cap", never 0.
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "presupuesto-usd" {
			v := *presupuesto
			d.PresupuestoUSD = &v
		}
	})
	if d.PresupuestoUSD != nil && (math.IsNaN(*d.PresupuestoUSD) || math.IsInf(*d.PresupuestoUSD, 0)) {
		return Request{}, uso("item add", "--presupuesto-usd tiene que ser un numero mayor que 0")
	}
	d, err = item.Validate(d)
	if err != nil {
		return Request{}, uso("item add", err.Error())
	}
	return Request{Sub: SubAdd, Draft: d, JSON: *asJSON}, nil
}

func uso(verb, reason string) error { return cliargs.NewUsageError(verb, reason, UsageText) }

// Execute runs a parsed request against the project at root. stdout carries
// the result (text or JSON); stderr the warnings about invalid items.
func Execute(root, base, blockOn string, req Request, stdout, stderr io.Writer) error {
	now := time.Now().UTC()
	switch req.Sub {
	case SubAdd:
		it, err := item.Add(root, req.Draft)
		if err != nil {
			return err
		}
		if req.JSON {
			return emit(stdout, it)
		}
		fmt.Fprintf(stdout, "hoom item: creado %s\n", item.RelPath(it.Slug))
		fmt.Fprintf(stdout, "  titulo:  %s (%s, prioridad %s)\n", it.Titulo, it.Tipo, it.Prioridad)
		if c, err := boardcmd.CardFor(root, base, blockOn, it.Slug, now); err == nil {
			motivo := ""
			if len(c.Missing) > 0 {
				motivo = " - " + c.Missing[0]
			}
			fmt.Fprintf(stdout, "  columna: %s%s\n", c.ColumnName, motivo)
			if c.Next != "" {
				fmt.Fprintf(stdout, "  siguiente: %s\n", c.Next)
			}
		}
		fmt.Fprintln(stdout, "  commitealo: viaja en git como los hallazgos")
		return nil
	case SubList:
		items, warnings, err := item.List(root)
		if err != nil {
			return err
		}
		if req.JSON {
			return emit(stdout, struct {
				Warnings []string    `json:"warnings"`
				Items    []item.Item `json:"items"`
			}{Warnings: warnings, Items: items})
		}
		for _, w := range warnings {
			fmt.Fprintln(stderr, "hoom:", w)
		}
		if len(items) == 0 {
			fmt.Fprintln(stdout, `hoom item: sin items (crea uno con 'hoom item add "<titulo>"')`)
			return nil
		}
		fmt.Fprintln(stdout, "hoom item: items")
		for _, it := range items {
			fmt.Fprintf(stdout, "  %-28s %-10s %-6s %s\n", it.Slug, it.Tipo, it.Prioridad, it.Titulo)
		}
		return nil
	case SubShow:
		c, err := boardcmd.CardFor(root, base, blockOn, req.Slug, now)
		if err != nil {
			return err
		}
		if req.JSON {
			return emit(stdout, struct {
				Item item.Item     `json:"item"`
				Card boardcmd.Card `json:"card"`
			}{Item: c.Item, Card: c})
		}
		boardcmd.RenderCard(stdout, c)
		return nil
	}
	return uso("item", fmt.Sprintf("subcomando desconocido %q (add|list|show)", req.Sub))
}

func emit(w io.Writer, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(raw))
	return nil
}
