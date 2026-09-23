// Package itemcmd implements `hoom item add | list | show | save`: the verbs
// of the card as a file. Parsing is pure and strict from day one (cliargs), so a
// request hoom does not understand is answered before the project is read,
// and it never leaves a half-written item behind.
package itemcmd

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
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
	SubSave = "save"
)

// UsageText is the exact usage block of `hoom item` and the single source of
// that text.
const UsageText = `Uso: hoom item add "<titulo>" [--tipo t] [--prioridad p] [--presupuesto-usd x] [--pedido "<texto>"] [--slug s] [--auto hasta-humano] [--json]
     hoom item list [--json]
     hoom item show <slug> [--json]
     hoom item save <slug> [--json]

  --tipo t             feature|bug|refactor|seguridad|docs|test (default feature)
  --prioridad p        alta|media|baja (default media)
  --presupuesto-usd x  Tope de gasto de la tarjeta en USD (> 0; opcional)
  --pedido "<texto>"   Lo que va a recibir el arquitecto (opcional)
  --slug s             Slug del item (default: derivado del titulo); es el
                       nombre del spec (.hoom/specs/<slug>.md) y de la tarea
  --auto hasta-humano  Activa el piloto automatico de la tarjeta en el Studio
                       (exige --presupuesto-usd): despues de un trabajo que
                       cierra bien, pide el del rol siguiente hasta la proxima
                       columna tuya, un rojo o el fin del presupuesto
  --json               Emite el resultado como JSON en stdout

'hoom item save' commitea lo que la tarjeta tiene sin guardar (en su espacio
de trabajo, todo; en el proyecto, su evidencia y su item), un commit por
arbol, con el mensaje fijo "hoom: guardar la tarjeta <slug>".

El item es un archivo en .hoom/items/<slug>.yaml que viaja en git. Nunca
guarda una columna: la columna sale de la evidencia ('hoom board').
Un titulo que empieza con '-' va despues de '--': hoom item add -- "-titulo"`

// Request is one parsed `hoom item` invocation.
type Request struct {
	Sub   string     // SubAdd | SubList | SubShow | SubSave
	Draft item.Draft // add
	Slug  string     // show, save
	JSON  bool
}

// Parse is the syntactic AND vocabulary stage, pure: no disk, no
// environment. It returns a *cliargs.UsageError for anything hoom does not
// understand and cliargs.ErrHelp for -h/--help.
func Parse(args []string) (Request, error) {
	if len(args) == 0 {
		return Request{}, uso("item", "falta el subcomando (add|list|show|save)")
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
	case SubSave:
		fs := flag.NewFlagSet("item save", flag.ContinueOnError)
		asJSON := fs.Bool("json", false, "emitir lo que se guardo como JSON")
		ops, err := cliargs.Operands(fs, rest, "item save", UsageText, 1)
		if err != nil {
			return Request{}, err
		}
		if !item.ValidSlug(ops[0]) {
			return Request{}, uso("item save", fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", ops[0]))
		}
		return Request{Sub: SubSave, Slug: ops[0], JSON: *asJSON}, nil
	}
	return Request{}, uso("item", fmt.Sprintf("subcomando desconocido %q (add|list|show|save)", sub))
}

func parseAdd(args []string) (Request, error) {
	fs := flag.NewFlagSet("item add", flag.ContinueOnError)
	tipo := fs.String("tipo", "", "tipo del item")
	prioridad := fs.String("prioridad", "", "prioridad del item")
	presupuesto := fs.Float64("presupuesto-usd", 0, "tope de gasto en USD")
	pedido := fs.String("pedido", "", "pedido para el arquitecto")
	slug := fs.String("slug", "", "slug del item")
	auto := fs.String("auto", "", "piloto automatico: hasta-humano")
	asJSON := fs.Bool("json", false, "emitir el item como JSON")
	ops, err := cliargs.Operands(fs, args, "item add", UsageText, 1)
	if err != nil {
		return Request{}, err
	}
	d := item.Draft{Titulo: ops[0], Tipo: *tipo, Prioridad: *prioridad, Pedido: *pedido, Slug: *slug, Auto: *auto}
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
	if strings.TrimSpace(d.Auto) != "" && d.PresupuestoUSD == nil {
		return Request{}, uso("item add", "--auto necesita --presupuesto-usd: el piloto automatico no corre sin tope")
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
	case SubSave:
		res, err := Save(root, base, blockOn, req.Slug, nil)
		if err != nil {
			return err
		}
		if req.JSON {
			return emit(stdout, res)
		}
		if len(res.Commits) == 0 {
			fmt.Fprintf(stdout, "hoom item save: la tarjeta %s no tiene nada sin guardar\n", req.Slug)
			return nil
		}
		fmt.Fprintf(stdout, "hoom item save: tarjeta %s guardada\n", req.Slug)
		for _, cm := range res.Commits {
			n := fmt.Sprintf("%d archivos", len(cm.Paths))
			if len(cm.Paths) == 1 {
				n = "1 archivo"
			}
			fmt.Fprintf(stdout, "  %s: %s (%s)\n", cm.Dir, short(cm.SHA), n)
		}
		return nil
	}
	return uso("item", fmt.Sprintf("subcomando desconocido %q (add|list|show|save)", req.Sub))
}

func emit(w io.Writer, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(w, string(raw))
	return nil
}

// SaveCommit is one commit `hoom item save` made.
type SaveCommit struct {
	Dir   string   `json:"dir"` // relative to root ("." = the project)
	SHA   string   `json:"sha"`
	Paths []string `json:"paths"` // relative to root
}

// SaveResult is what `hoom item save` did, identical in text and JSON.
type SaveResult struct {
	Slug    string       `json:"slug"`
	Message string       `json:"message"`
	Commits []SaveCommit `json:"commits"`
}

// SaveMessage is the fixed commit message of `hoom item save`.
func SaveMessage(slug string) string { return "hoom: guardar la tarjeta " + slug }

// ErrChanged is what Save and the Studio answer when the paths a person saw
// are not the ones there are now.
const ErrChanged = "los cambios de la tarjeta cambiaron desde que los viste: revisalos de nuevo"

// Save commits exactly the card's unsynced paths, one commit per tree. A
// non-nil expect must equal them as a set.
func Save(root, base, blockOn, slug string, expect []string) (SaveResult, error) {
	c, err := boardcmd.CardFor(root, base, blockOn, slug, time.Now().UTC())
	if err != nil {
		return SaveResult{}, err
	}
	res := SaveResult{Slug: slug, Message: SaveMessage(slug), Commits: []SaveCommit{}}
	if r := c.Running; r != nil {
		quien := "el agente"
		if r.Role != "" {
			quien = "el " + r.Role
		}
		return SaveResult{}, fmt.Errorf("espera a que termine %s que esta trabajando", quien)
	}
	paths := append([]string{}, c.Unsynced...)
	if expect != nil && !sameSet(expect, paths) {
		return SaveResult{}, fmt.Errorf("%s", ErrChanged)
	}
	if len(paths) == 0 {
		return res, nil
	}
	// One commit per tree: the card's workspace first, then the project
	// (where its item lives).
	var inTask, inRoot []string
	wt := ""
	if c.Evidence.Source == boardcmd.SourceWorktree {
		wt = c.Evidence.Dir
	}
	for _, p := range paths {
		if wt != "" && strings.HasPrefix(p, wt+"/") {
			inTask = append(inTask, p)
		} else {
			inRoot = append(inRoot, p)
		}
	}
	for _, tr := range []struct {
		rel   string
		paths []string
	}{{wt, inTask}, {".", inRoot}} {
		if len(tr.paths) == 0 {
			continue
		}
		dir, local := root, tr.paths
		if tr.rel != "." {
			dir = filepath.Join(root, filepath.FromSlash(tr.rel))
			local = make([]string, len(tr.paths))
			for i, p := range tr.paths {
				local[i] = strings.TrimPrefix(p, tr.rel+"/")
			}
		}
		sha, err := commitPaths(dir, res.Message, local)
		if err != nil {
			return res, err
		}
		res.Commits = append(res.Commits, SaveCommit{Dir: tr.rel, SHA: sha, Paths: tr.paths})
	}
	return res, nil
}

// commitPaths stages exactly paths and commits only them: whatever else sits
// in the index stays there, uncommitted. The repository's hooks run.
func commitPaths(dir, msg string, paths []string) (string, error) {
	if out, err := git(dir, append([]string{"add", "-A", "--"}, paths...)...); err != nil {
		return "", fmt.Errorf("git add fallo: %s", out)
	}
	if out, err := git(dir, append([]string{"commit", "-q", "-m", msg, "--"}, paths...)...); err != nil {
		return "", fmt.Errorf("git commit fallo: %s", out)
	}
	sha, err := git(dir, "rev-parse", "HEAD")
	if err != nil {
		return "", fmt.Errorf("git rev-parse fallo: %s", sha)
	}
	return sha, nil
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func sameSet(a, b []string) bool {
	x, y := append([]string{}, a...), append([]string{}, b...)
	sort.Strings(x)
	sort.Strings(y)
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

func short(sha string) string {
	if len(sha) > 12 {
		return sha[:12]
	}
	return sha
}
