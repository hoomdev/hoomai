// Package itemcmd implements `hoom item add | list | show | save | backfill`:
// the verbs of the card as a file (backfill gives an item to the tasks from
// before the cabin that the evidence holds). Parsing is pure and strict from day one (cliargs), so a
// request hoom does not understand is answered before the project is read,
// and it never leaves a half-written item behind.
package itemcmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// Subcommands.
const (
	SubAdd  = "add"
	SubList = "list"
	SubShow = "show"
	SubSave = "save"
	// SubBackfill creates the items of the tasks that predate the cabin
	// (historia-en-el-tablero).
	SubBackfill = "backfill"
)

// UsageText is the exact usage block of `hoom item` and the single source of
// that text.
const UsageText = `Uso: hoom item add "<titulo>" [--tipo t] [--prioridad p] [--presupuesto-usd x] [--pedido "<texto>"] [--slug s] [--auto hasta-humano] [--json]
     hoom item list [--json]
     hoom item show <slug> [--json]
     hoom item save <slug> [--json]
     hoom item backfill [--dry-run] [--json]

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

'hoom item backfill' crea el item de cada tarea con evidencia y sin item que
califica: con spec, si tiene un veredicto completo verde con spec_approved en
pass e integrado en la base; sin spec, si ningun hallazgo bloqueante suyo esta
abierto. El item lleva 'historico' (que evidencia lo sostiene) y nunca
hecho_en. Lo que no califica sale con su motivo; --dry-run no escribe nada.

El item es un archivo en .hoom/items/<slug>.yaml que viaja en git. Nunca
guarda una columna: la columna sale de la evidencia ('hoom board').
Un titulo que empieza con '-' va despues de '--': hoom item add -- "-titulo"`

// Request is one parsed `hoom item` invocation.
type Request struct {
	Sub    string     // SubAdd | SubList | SubShow | SubSave | SubBackfill
	Draft  item.Draft // add
	Slug   string     // show, save
	JSON   bool
	DryRun bool // backfill
}

// BackfillResult is what `hoom item backfill` did, or would do with
// --dry-run.
type BackfillResult struct {
	DryRun      bool                 `json:"dry_run"`
	Creados     []boardcmd.Candidata `json:"creados"`      // created (or that it would create)
	NoCalifican []boardcmd.Candidata `json:"no_califican"` // with their Motivo
	ConItem     []string             `json:"con_item"`     // tasks that already have an item: untouched
	// Error is why the run stopped halfway, with Creados the items it did
	// create, which stay; "" when it did not stop.
	Error string `json:"error,omitempty"`
}

// Backfill creates an item with historico for every task with evidence and
// no item that qualifies, and lists the rest with why. dryRun writes
// nothing. It never removes an item: on an error halfway (a full disk) the
// items this run created stay, each one whole and valid, and res lists them;
// running it again completes the rest, since it skips the tasks with an item
// (enmienda 1: a rollback by path could take somebody else's item, findings
// 1e8b0c, 039b75, a7d0b1).
func Backfill(root, base, blockOn string, dryRun bool, now time.Time) (BackfillResult, error) {
	res := BackfillResult{DryRun: dryRun, Creados: []boardcmd.Candidata{}, NoCalifican: []boardcmd.Candidata{}, ConItem: []string{}}
	cands, err := boardcmd.Candidatas(root, base, blockOn, now)
	if err != nil {
		return res, err
	}
	for _, c := range cands {
		switch {
		case c.ConItem:
			res.ConItem = append(res.ConItem, c.Slug)
		case c.Historico == "":
			res.NoCalifican = append(res.NoCalifican, c)
		case dryRun:
			res.Creados = append(res.Creados, c)
		default:
			_, err := item.Add(root, item.Draft{Titulo: c.Titulo, Tipo: c.Tipo, Prioridad: "media",
				Slug: c.Slug, Historico: c.Historico})
			switch {
			case errors.Is(err, item.ErrExists):
				// it appeared since the list: it is somebody's, and stays
				// as it is
				res.ConItem = append(res.ConItem, c.Slug)
			case err != nil:
				err = fmt.Errorf("no se pudo crear el item de %s: %w; %s", c.Slug, err, quedan(len(res.Creados)))
				res.Error = err.Error()
				return res, err
			default:
				res.Creados = append(res.Creados, c)
			}
		}
	}
	return res, nil
}

// quedan says what an error halfway leaves of a run that created n items.
func quedan(n int) string {
	if n == 0 {
		return "no se creo ningun item"
	}
	return fmt.Sprintf("los %d items que esta corrida ya creo quedan; correr backfill de nuevo completa el resto", n)
}

// Parse is the syntactic AND vocabulary stage, pure: no disk, no
// environment. It returns a *cliargs.UsageError for anything hoom does not
// understand and cliargs.ErrHelp for -h/--help.
func Parse(args []string) (Request, error) {
	if len(args) == 0 {
		return Request{}, uso("item", "falta el subcomando (add|list|show|save|backfill)")
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
	case SubBackfill:
		fs := flag.NewFlagSet("item backfill", flag.ContinueOnError)
		dryRun := fs.Bool("dry-run", false, "lista lo que crearia sin escribir nada")
		asJSON := fs.Bool("json", false, "emitir el resultado como JSON")
		if err := cliargs.Strict(fs, rest, "item backfill", UsageText); err != nil {
			return Request{}, err
		}
		return Request{Sub: SubBackfill, DryRun: *dryRun, JSON: *asJSON}, nil
	}
	return Request{}, uso("item", fmt.Sprintf("subcomando desconocido %q (add|list|show|save|backfill)", sub))
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
	case SubBackfill:
		res, err := Backfill(root, base, blockOn, req.DryRun, now)
		if err != nil && len(res.Creados) == 0 {
			return err
		}
		// stopped halfway: what it did create stays, and it is said
		if req.JSON {
			if eerr := emit(stdout, res); eerr != nil {
				return eerr
			}
		} else {
			renderBackfill(stdout, res)
		}
		return err
	}
	return uso("item", fmt.Sprintf("subcomando desconocido %q (add|list|show|save|backfill)", req.Sub))
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
	// something under the card's workspace name that is not its worktree (a
	// symlink, a plain folder, another clone) is not "no workspace": saving
	// would commit the card without the work it claims to have, so it
	// stops, like every task command at the same gate (finding 28c350)
	if _, err := os.Lstat(filepath.Join(root, ".hoom", "worktrees", slug)); !errors.Is(err, os.ErrNotExist) && item.ValidSlug(slug) {
		if _, err := runcmd.TaskDir(root, slug); err != nil {
			return SaveResult{}, err
		}
	}
	c, err := boardcmd.CardFor(root, base, blockOn, slug, time.Now().UTC())
	if err != nil {
		return SaveResult{}, err
	}
	// from the check of the card to the last commit, under the lock of each
	// tree it commits in: a run that starts meanwhile waits until it is over
	// (finding ac2feb). The card is derived again under them.
	unlock, err := lockArboles(root, c)
	if err != nil {
		return SaveResult{}, err
	}
	defer unlock()
	tomados := arbolesDe(root, c)
	if c, err = boardcmd.CardFor(root, base, blockOn, slug, time.Now().UTC()); err != nil {
		return SaveResult{}, err
	}
	// the trees committed are the ones locked: a workspace that appeared
	// between the two derivations has no lock taken (finding ebae2a)
	if !sameSet(tomados, arbolesDe(root, c)) {
		return SaveResult{}, fmt.Errorf("%s", ErrChanged)
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
			if len(res.Commits) > 0 {
				// the workspace is already saved: say so, with its commit
				// (finding 42fa10)
				var hechos []string
				for _, c := range res.Commits {
					hechos = append(hechos, fmt.Sprintf("%s en %s", short(c.SHA), c.Dir))
				}
				err = fmt.Errorf("%v (ya quedo guardado %s; lo que falta se guarda de nuevo desde la tarjeta)", err, strings.Join(hechos, ", "))
			}
			return res, err
		}
		res.Commits = append(res.Commits, SaveCommit{Dir: tr.rel, SHA: sha, Paths: tr.paths})
	}
	return res, nil
}

// arbolesDe are the trees saving card c commits in: its workspace (when it
// has one) and the project, in that order.
func arbolesDe(root string, c boardcmd.Card) []string {
	dirs := []string{}
	if c.Evidence.Source == boardcmd.SourceWorktree {
		dirs = append(dirs, filepath.Join(root, filepath.FromSlash(c.Evidence.Dir)))
	}
	return append(dirs, root)
}

// lockArboles takes the lock of each tree of arbolesDe, in order, and returns
// what releases them.
func lockArboles(root string, c boardcmd.Card) (func(), error) {
	dirs := arbolesDe(root, c)
	var unlocks []func()
	release := func() {
		for i := len(unlocks) - 1; i >= 0; i-- {
			unlocks[i]()
		}
	}
	for _, d := range dirs {
		u, err := runcmd.LockArbol(d, runcmd.EsperaArbol)
		if err != nil {
			release()
			if errors.Is(err, hoomfs.ErrLocked) {
				return nil, fmt.Errorf("un agente esta arrancando en la tarjeta %s: guardar ahora commitearia su trabajo a medias. Accion: espera a que termine", c.Slug)
			}
			return nil, err
		}
		unlocks = append(unlocks, u)
	}
	return release, nil
}

// commitPaths stages exactly paths and commits only them: whatever else sits
// in the index stays there, uncommitted. The repository's hooks run.
func commitPaths(dir, msg string, paths []string) (string, error) {
	// what is on disk is added; what is not (a deletion, also one a git rm
	// or a git mv already staged) leaves the index: git add stops at a name
	// that is in neither (finding 186524). The paths are names, never
	// patterns (finding a26f9d).
	var presentes, ausentes []string
	for _, p := range paths {
		if _, err := os.Lstat(filepath.Join(dir, filepath.FromSlash(p))); os.IsNotExist(err) {
			ausentes = append(ausentes, p)
		} else {
			presentes = append(presentes, p)
		}
	}
	if len(presentes) > 0 {
		if out, err := git(dir, append([]string{"--literal-pathspecs", "add", "-A", "--"}, presentes...)...); err != nil {
			return "", fmt.Errorf("git add fallo: %s", out)
		}
	}
	if len(ausentes) > 0 {
		cmd := exec.Command("git", "update-index", "--remove", "-z", "--stdin")
		cmd.Dir = dir
		cmd.Stdin = strings.NewReader(strings.Join(ausentes, "\x00") + "\x00")
		if out, err := cmd.CombinedOutput(); err != nil {
			return "", fmt.Errorf("git update-index fallo: %s", strings.TrimSpace(string(out)))
		}
	}
	if out, err := git(dir, append([]string{"--literal-pathspecs", "commit", "-q", "-m", msg, "--"}, paths...)...); err != nil {
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

// renderBackfill prints what backfill did: one line per task, created first
// ("crearia" with --dry-run), then the ones that do not qualify with their
// motivo, then the ones that already have an item.
func renderBackfill(w io.Writer, res BackfillResult) {
	verbo, resumen := "creado", "creado(s)"
	if res.DryRun {
		verbo, resumen = "crearia", "crearia(n)"
	}
	fmt.Fprintf(w, "hoom item backfill: %d %s, %d no califican, %d ya tienen item\n",
		len(res.Creados), resumen, len(res.NoCalifican), len(res.ConItem))
	for _, c := range res.Creados {
		fmt.Fprintf(w, "  %-9s %-36s historico: %s\n", verbo, c.Slug, c.Historico)
	}
	for _, c := range res.NoCalifican {
		fmt.Fprintf(w, "  %-9s %-36s %s\n", "no crea", c.Slug, c.Motivo)
	}
	for _, s := range res.ConItem {
		fmt.Fprintf(w, "  %-9s %-36s ya tiene item: no se toca\n", "intacto", s)
	}
}
