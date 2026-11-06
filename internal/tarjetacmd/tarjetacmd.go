// Package tarjetacmd holds the card verbs (spec verbos-de-la-tarjeta): hoom
// usar, pedir, siguiente, aprobar, integrar, guardar and reanudar. Each one
// takes ONE action of the card (boardcmd.Derive decides the list, its why and
// its proposals) and runs it with the same function as the Studio, or says
// why not. They deduce the card (operand, folder, hoom usar) and say where
// they got it from before doing anything.
package tarjetacmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/valores"
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
	dir, err := os.Getwd()
	if err != nil {
		dir = "."
	}
	return Entorno{Dir: dir, In: os.Stdin, Out: os.Stdout, Err: os.Stderr,
		Terminal: esTerminal(os.Stdin) && esTerminal(os.Stdout), Getenv: os.Getenv}
}

func esTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
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
	root, carpeta, err := proyecto(dir)
	if err != nil {
		return Deduccion{}, &Fallo{Code: 1, Msg: err.Error()}
	}
	d := Deduccion{Root: root}
	switch {
	case operando != "":
		d.Slug, d.Origen = operando, OrigenArgumento
	case carpeta != "":
		d.Slug, d.Origen = carpeta, OrigenCarpeta
	default:
		fijada, ok := leerFijada(root)
		if !ok {
			return Deduccion{}, &Fallo{Code: 2, Msg: msgSinTarjeta}
		}
		if !esTarjeta(root, fijada) {
			return Deduccion{}, &Fallo{Code: 1, Msg: fmt.Sprintf("la tarjeta fijada %s ya no existe: hoom usar <otra>", fijada)}
		}
		d.Slug, d.Origen = fijada, OrigenUsar
	}
	if !esTarjeta(root, d.Slug) {
		return Deduccion{}, &Fallo{Code: 1, Msg: fmt.Sprintf("no hay tarjeta %s: hoom item list", d.Slug)}
	}
	return d, nil
}

const msgSinTarjeta = "no se de que tarjeta hablas: pasala (hoom pedir writer <slug>), entra a su espacio de trabajo o fija una con hoom usar <slug>"

// proyecto is the project dir belongs to, and the card of the workspace dir
// is in ("" outside one). Inside <proyecto>/.hoom/worktrees/<slug>/ at any
// depth (a blind tree nested in it included) the project is the tree that
// contains the workspace, never the workspace: the outermost one. Outside,
// it is the directory that holds hoom.yaml.
func proyecto(dir string) (root, slug string, err error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", "", err
	}
	abs = filepath.Clean(abs)
	sep := string(filepath.Separator)
	marca := sep + ".hoom" + sep + "worktrees" + sep
	if i := strings.Index(abs+sep, marca); i >= 0 {
		resto := strings.TrimPrefix((abs + sep)[i+len(marca):], sep)
		if j := strings.Index(resto, sep); j > 0 {
			root = abs[:i]
			if root == "" {
				root = sep
			}
			return root, resto[:j], nil
		}
	}
	root, err = manifest.Find(abs)
	return root, "", err
}

// esTarjeta says whether slug is a card of the project: an item file.
func esTarjeta(root, slug string) bool {
	if !item.ValidSlug(slug) {
		return false
	}
	fi, err := os.Stat(filepath.Join(root, filepath.FromSlash(item.RelPath(slug))))
	return err == nil && fi.Mode().IsRegular()
}

// leerFijada is the card hoom usar fixed, if any.
func leerFijada(root string) (string, bool) {
	raw, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ArchivoFijada)))
	if err != nil {
		return "", false
	}
	s := strings.TrimSpace(string(raw))
	return s, s != ""
}

// Run executes one card verb and returns its exit code: 0 done, 1 the card
// (or the job) said no, 2 the request was not understood. A verb of the
// envelope or the review returns what `hoom agent` or `hoom review` would.
func Run(verb string, args []string, e Entorno) int {
	if e.Out == nil {
		e.Out = io.Discard
	}
	if e.Err == nil {
		e.Err = io.Discard
	}
	if e.In == nil {
		e.In = strings.NewReader("")
	}
	if e.Getenv == nil {
		e.Getenv = os.Getenv
	}
	if e.Dir == "" {
		e.Dir = "."
	}
	v := &verbo{nombre: verb, e: e}
	switch verb {
	case "usar":
		return v.usar(args)
	case "pedir":
		return v.pedir(args)
	case "siguiente":
		return v.siguiente(args)
	case "aprobar", "integrar":
		return v.firmar(args)
	case "guardar":
		return v.guardar(args)
	case "reanudar":
		return v.reanudar(args)
	}
	fmt.Fprintf(e.Err, "hoom: %q no es un verbo de la tarjeta (%s)\n", verb, strings.Join(Verbos, ", "))
	return 2
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

// verbo is one run of a card verb.
type verbo struct {
	nombre string
	e      Entorno
	info   io.Writer // where the tarjeta:, valores: and → lines go (Err with --json)
}

// falla prints an exit-1 message and returns 1.
func (v *verbo) falla(msg string) int {
	fmt.Fprintf(v.e.Err, "hoom: %s\n", msg)
	return 1
}

// uso prints an exit-2 message with the verb's usage block and returns 2.
func (v *verbo) uso(razon string) int {
	fmt.Fprintln(v.e.Err, cliargs.NewUsageError(v.nombre, razon, Uso(v.nombre)).Error())
	return 2
}

// fallo prints a *Fallo (or any error, as exit 1).
func (v *verbo) fallo(err error) int {
	var f *Fallo
	if errors.As(err, &f) {
		if f.Code == 2 {
			return v.uso(f.Msg)
		}
		return v.falla(f.Msg)
	}
	return v.falla(err.Error())
}

// parsear applies cliargs' discipline: a flag that does not exist, a flag
// without its value or an operand too many is exit 2 with the usage block;
// -h prints it and exits 0. ok=false means the verb ends with code.
func (v *verbo) parsear(fs *flag.FlagSet, args []string, min, max int, vacios ...string) (ops []string, code int, ok bool) {
	ops, err := cliargs.OperandsRango(fs, args, v.nombre, Uso(v.nombre), min, max, vacios...)
	if errors.Is(err, cliargs.ErrHelp) {
		fmt.Fprintln(v.e.Out, Uso(v.nombre))
		return nil, 0, false
	}
	var ue *cliargs.UsageError
	if errors.As(err, &ue) {
		fmt.Fprintln(v.e.Err, ue.Error())
		return nil, 2, false
	}
	if err != nil {
		return nil, v.uso(err.Error()), false
	}
	return ops, 0, true
}

// tarjeta deduces the card and says where it came from, before anything.
func (v *verbo) tarjeta(operando string) (Deduccion, int, bool) {
	d, err := Deducir(v.e.Dir, operando)
	if err != nil {
		return d, v.fallo(err), false
	}
	fmt.Fprintf(v.info, "tarjeta: %s (%s)\n", d.Slug, d.Origen)
	return d, 0, true
}

// carta loads the project's hoom.yaml and derives the card: a hoom.yaml
// that does not load stops every verb that reads it.
func (v *verbo) carta(d Deduccion) (*manifest.Manifest, boardcmd.Card, int, bool) {
	m, err := manifest.Load(d.Root, profiles.Resolve)
	if err != nil {
		return nil, boardcmd.Card{}, v.falla(err.Error()), false
	}
	c, err := boardcmd.CardFor(d.Root, m.BaseBranch, m.FindingsBlockOn(), d.Slug, time.Now().UTC())
	if err != nil {
		return nil, boardcmd.Card{}, v.falla(err.Error()), false
	}
	return m, c, 0, true
}

// ---------------------------------------------------------------- usar

func (v *verbo) usar(args []string) int {
	v.info = v.e.Out
	fs := flag.NewFlagSet("usar", flag.ContinueOnError)
	ninguna := fs.Bool("ninguna", false, "")
	ops, code, ok := v.parsear(fs, args, 0, 1)
	if !ok {
		return code
	}
	if *ninguna && len(ops) > 0 {
		return v.uso("--ninguna no lleva tarjeta")
	}
	root, _, err := proyecto(v.e.Dir)
	if err != nil {
		return v.falla(err.Error())
	}
	path := filepath.Join(root, filepath.FromSlash(ArchivoFijada))
	switch {
	case *ninguna:
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return v.falla(err.Error())
		}
		fmt.Fprintln(v.e.Out, "sin tarjeta fijada")
	case len(ops) == 1:
		slug := ops[0]
		if !esTarjeta(root, slug) {
			return v.falla(fmt.Sprintf("no hay tarjeta %s: hoom item list", slug))
		}
		// local state: .hoom/cache stays out of git and of the fingerprint
		hoomfs.EnsureIgnored(root)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return v.falla(err.Error())
		}
		if err := os.WriteFile(path, []byte(slug+"\n"), 0o644); err != nil {
			return v.falla(err.Error())
		}
		fmt.Fprintf(v.e.Out, "tarjeta fijada: %s\n", slug)
	default:
		slug, ok := leerFijada(root)
		if !ok {
			fmt.Fprintln(v.e.Out, "sin tarjeta fijada")
			return 0
		}
		if !esTarjeta(root, slug) {
			return v.falla(fmt.Sprintf("la tarjeta fijada %s ya no existe: hoom usar <otra>", slug))
		}
		fmt.Fprintf(v.e.Out, "tarjeta: %s\n", slug)
	}
	return 0
}

// ---------------------------------------------------------------- lanzar

// roles are the roles hoom pedir asks for: the four of the card.
var roles = []string{"arquitecto", "test-writer", "writer", "reviewer"}

// flagsLanzamiento are the launch flags, in the order the usage gives them.
var flagsLanzamiento = []string{"m", "provider", "model", "effort", "budget-usd", "timeout", "json"}

// lanzamiento is what the launch flags asked.
type lanzamiento struct {
	pedido   string
	provider string
	model    string
	effort   string
	budget   float64
	timeoutV string
	json     bool
	escritos map[string]bool // the flags written
	timeout  time.Duration
}

func (l *lanzamiento) registrar(fs *flag.FlagSet, conProvider bool) {
	fs.StringVar(&l.pedido, "m", "", "")
	if conProvider {
		fs.StringVar(&l.provider, "provider", "", "")
	}
	fs.StringVar(&l.model, "model", "", "")
	fs.StringVar(&l.effort, "effort", "", "")
	fs.Float64Var(&l.budget, "budget-usd", 0, "")
	fs.StringVar(&l.timeoutV, "timeout", "", "")
	fs.BoolVar(&l.json, "json", false, "")
}

// leer records which flags were written and checks the timeout: one that
// is not between 1m and 12h is exit 2, like a flag without its value.
func (l *lanzamiento) leer(v *verbo, fs *flag.FlagSet) (int, bool) {
	l.escritos = map[string]bool{}
	fs.Visit(func(f *flag.Flag) { l.escritos[f.Name] = true })
	if l.escritos["timeout"] {
		d, err := manifest.ParseTimeout("--timeout", l.timeoutV)
		if err != nil {
			return v.uso(err.Error()), false
		}
		l.timeout = d
	}
	v.info = v.e.Out
	if l.json {
		v.info = v.e.Err // --json: stdout carries the JSON alone
	}
	return 0, true
}

// request is the launch the flags ask for.
func (l *lanzamiento) request(action string) lanzar.Request {
	r := lanzar.Request{Action: action, Provider: strings.TrimSpace(l.provider), Model: strings.TrimSpace(l.model),
		Effort: strings.TrimSpace(l.effort), Timeout: l.timeout}
	if l.escritos["budget-usd"] {
		b := l.budget
		r.BudgetUSD = &b
	}
	return r
}

func (v *verbo) pedir(args []string) int {
	fs := flag.NewFlagSet("pedir", flag.ContinueOnError)
	var l lanzamiento
	l.registrar(fs, true)
	ops, code, ok := v.parsear(fs, args, 1, 2, "m")
	if !ok {
		return code
	}
	if code, ok := l.leer(v, fs); !ok {
		return code
	}
	rol := ops[0]
	if !contiene(roles, rol) {
		return v.uso(fmt.Sprintf("rol desconocido %q (%s); para otros roles: hoom agent", rol, strings.Join(roles, ", ")))
	}
	if rol == "reviewer" && l.escritos["m"] {
		return v.uso("la revision arma su propio pedido: no lleva -m")
	}
	operando := ""
	if len(ops) == 2 {
		operando = ops[1]
	}
	d, code, ok := v.tarjeta(operando)
	if !ok {
		return code
	}
	m, c, code, ok := v.carta(d)
	if !ok {
		return code
	}
	a, ok := c.Action("pedir-" + rol)
	if !ok {
		return v.falla(fmt.Sprintf("en %s no se le pide al %s: %s. Siguiente: hoom siguiente", c.ColumnName, rol, c.Plain))
	}
	return v.lanzarRol(d, m, c, a, &l)
}

func (v *verbo) reanudar(args []string) int {
	fs := flag.NewFlagSet("reanudar", flag.ContinueOnError)
	var l lanzamiento
	l.registrar(fs, false) // the session is its provider's
	ops, code, ok := v.parsear(fs, args, 0, 1, "m")
	if !ok {
		return code
	}
	if code, ok := l.leer(v, fs); !ok {
		return code
	}
	d, code, ok := v.tarjeta(primero(ops))
	if !ok {
		return code
	}
	m, c, code, ok := v.carta(d)
	if !ok {
		return code
	}
	a, ok := c.Action(boardcmd.ActReanudar)
	if !ok {
		return v.falla(fmt.Sprintf("en %s no hay nada que reanudar: %s", c.ColumnName, c.Plain))
	}
	return v.lanzarRol(d, m, c, a, &l)
}

// lanzarRol runs a role action of the card with the same function as the
// Studio: it checks the action, asks for the pedido (-m, the editor or the
// proposal), prints the values and the long command, launches and waits,
// with the narration on stdout. Its exit code and --json are hoom agent's
// (or hoom review's for the reviewer).
func (v *verbo) lanzarRol(d Deduccion, m *manifest.Manifest, c boardcmd.Card, a boardcmd.Action, l *lanzamiento) int {
	if !a.Enabled {
		return v.falla(a.Why)
	}
	req := l.request(a.ID)
	// the values before the pedido: the editor shows them, and a provider
	// or budget that cannot launch does not cost the person a pedido
	if err := lanzar.Comprobar(c, a, req); err != nil {
		return v.falla(err.Error())
	}
	vals := lanzar.Valores(c, a, req)
	linea := lineaValores(vals, a, req)
	review := a.Role == "reviewer"
	if !review {
		pedido, code, ok := v.elPedido(d, a, l, linea)
		if !ok {
			return code
		}
		req.Pedido = pedido
	}
	fmt.Fprintln(v.info, linea)
	fmt.Fprintln(v.info, comandoLargo(d.Slug, a, vals, req.Pedido))
	p := lanzar.Params{Root: d.Root, Manifest: m}
	if !l.json {
		p.Eco = v.e.Out
	}
	t, err := lanzar.Lanzar(context.Background(), p, d.Slug, req)
	if err != nil && (t == nil || t.Wait().Review == nil && t.Wait().Agent == nil) {
		return v.falla(err.Error())
	}
	fin := t.Wait()
	if fin.Err != nil {
		return v.falla(fin.Err.Error())
	}
	var res any
	exit := 0
	switch {
	case fin.Review != nil:
		res, exit = fin.Review, fin.Review.ExitCode
	case fin.Agent != nil:
		res, exit = fin.Agent, fin.Agent.ExitCode
	case fin.Nota != "":
		return v.falla(fin.Nota)
	}
	if l.json && res != nil {
		raw, _ := json.MarshalIndent(res, "", "  ")
		fmt.Fprintln(v.e.Out, string(raw))
	}
	return exit
}

// elPedido is what the role has to do: -m, else the editor on a terminal,
// else the action's proposal as it is.
func (v *verbo) elPedido(d Deduccion, a boardcmd.Action, l *lanzamiento, linea string) (string, int, bool) {
	var pedido string
	switch {
	case l.escritos["m"]:
		pedido = l.pedido
	case v.e.Terminal:
		texto, err := v.editar(plantilla(d, a, linea))
		if err != nil {
			return "", v.falla("el editor salio con error: no se lanzo nada"), false
		}
		pedido = sinComentarios(texto)
	default:
		pedido = a.Pedido
	}
	if strings.TrimSpace(pedido) == "" {
		return "", v.falla("pedido vacio: no se lanzo nada"), false
	}
	return strings.TrimSpace(pedido), 0, true
}

// plantilla is the file the editor opens: the proposed pedido and, under
// it, the comment lines.
func plantilla(d Deduccion, a boardcmd.Action, linea string) string {
	var b strings.Builder
	b.WriteString(a.Pedido)
	b.WriteString("\n\n")
	fmt.Fprintf(&b, "# tarjeta: %s (%s)\n", d.Slug, d.Origen)
	fmt.Fprintf(&b, "# rol: %s\n", a.Role)
	fmt.Fprintf(&b, "# %s\n", linea)
	b.WriteString("# Las lineas que empiezan con # se ignoran. Si lo dejas vacio no se lanza nada.\n")
	return b.String()
}

// sinComentarios drops the lines that start with #.
func sinComentarios(texto string) string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(texto, "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(line, "#") {
			out = append(out, line)
		}
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// editar opens the person's editor on a temporary file with texto and
// returns what it holds when the editor ends: $VISUAL, then $EDITOR, then vi
// (notepad on Windows). The editor runs through the shell, as git does, so
// an editor with arguments (code --wait) works.
func (v *verbo) editar(texto string) (string, error) {
	f, err := os.CreateTemp("", "hoom-pedido-*.txt")
	if err != nil {
		return "", err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.WriteString(texto); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	editor := strings.TrimSpace(v.e.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(v.e.Getenv("EDITOR"))
	}
	var cmd *exec.Cmd
	switch {
	case editor == "" && runtime.GOOS == "windows":
		cmd = exec.Command("notepad", path)
	case editor == "":
		cmd = exec.Command("vi", path)
	case runtime.GOOS == "windows":
		cmd = exec.Command("cmd", "/C", editor+` "`+path+`"`) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- el editor es el de la persona ($VISUAL/$EDITOR), como en git
	default:
		cmd = exec.Command("sh", "-c", editor+` "$@"`, editor, path) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- el editor es el de la persona ($VISUAL/$EDITOR), como en git
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = v.e.In, v.e.Out, v.e.Err
	if err := cmd.Run(); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	return string(raw), err
}

// lineaValores is the line of the values and where each came from.
func lineaValores(e valores.Elegidos, a boardcmd.Action, req lanzar.Request) string {
	valor := func(nombre, v, origen string) string {
		if v == "" {
			v, origen = "del provider", valores.OrigenProvider
		}
		return fmt.Sprintf("%s %s (%s)", nombre, v, origen)
	}
	presupuesto := valor("presupuesto", "", e.Origen.BudgetUSD)
	o, ok := opcionDe(a, e.Provider)
	switch {
	case ok && !o.Budget && req.BudgetUSD == nil:
		presupuesto = fmt.Sprintf("presupuesto sin tope (%s no acepta tope)", e.Provider)
	case e.BudgetUSD != nil:
		presupuesto = fmt.Sprintf("presupuesto %s (%s)", lanzar.USD(*e.BudgetUSD), e.Origen.BudgetUSD)
	case e.Origen.BudgetUSD == valores.OrigenHoomYAML:
		presupuesto = "presupuesto sin tope (hoom.yaml)" // budget_usd: 0
	}
	return "valores: " + strings.Join([]string{
		valor("provider", e.Provider, e.Origen.Provider),
		valor("modelo", e.Model, e.Origen.Model),
		valor("esfuerzo", e.Effort, e.Origen.Effort),
		presupuesto,
		fmt.Sprintf("tiempo limite %s (%s)", valores.Duracion(e.Timeout), e.Origen.Timeout),
	}, " · ")
}

// comandoLargo is the equivalent command of a launch, with the pedido cut
// to 60 characters.
func comandoLargo(slug string, a boardcmd.Action, e valores.Elegidos, pedido string) string {
	spec := ".hoom/specs/" + slug + ".md"
	var parts []string
	if a.Role == "reviewer" {
		parts = []string{"hoom review", "--task", slug, "--spec", spec, "--provider", e.Provider}
	} else {
		parts = []string{"hoom agent", "--role", a.Role, "--task", slug}
		if !boardcmd.WritesSpecs(a.Role) {
			parts = append(parts, "--spec", spec)
		}
		parts = append(parts, "--provider", e.Provider)
	}
	if e.Model != "" {
		parts = append(parts, "--model", e.Model)
	}
	if e.Effort != "" {
		parts = append(parts, "--effort", e.Effort)
	}
	if o, ok := opcionDe(a, e.Provider); e.BudgetUSD != nil && (!ok || o.Budget) {
		// a provider that takes no cap gets none: hoom agent would refuse it
		parts = append(parts, "--budget-usd", lanzar.USD(*e.BudgetUSD))
	}
	if a.Role != "reviewer" {
		// hoom review has no --timeout: review.timeout is its place
		if e.Timeout != valores.DefaultTimeout {
			parts = append(parts, "--timeout", valores.Duracion(e.Timeout))
		}
		if a.ID == boardcmd.ActReanudar && a.ResumeID != "" {
			parts = append(parts, "--resume", a.ResumeID)
		}
		parts = append(parts, `"`+recortar(pedido)+`"`)
	}
	return "→ " + strings.Join(parts, " ")
}

// recortar is the pedido on one line, cut to 60 characters with ….
func recortar(p string) string {
	p = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(strings.TrimSpace(p))
	if utf8.RuneCountInString(p) <= 60 {
		return p
	}
	return string([]rune(p)[:60]) + "…"
}

func opcionDe(a boardcmd.Action, name string) (boardcmd.ProviderOption, bool) {
	for _, o := range a.Providers {
		if o.Name == name {
			return o, true
		}
	}
	return boardcmd.ProviderOption{}, false
}

// ---------------------------------------------------------------- siguiente

func (v *verbo) siguiente(args []string) int {
	fs := flag.NewFlagSet("siguiente", flag.ContinueOnError)
	var l lanzamiento
	l.registrar(fs, true)
	ops, code, ok := v.parsear(fs, args, 0, 1, "m")
	if !ok {
		return code
	}
	if code, ok := l.leer(v, fs); !ok {
		return code
	}
	d, code, ok := v.tarjeta(primero(ops))
	if !ok {
		return code
	}
	m, c, code, ok := v.carta(d)
	if !ok {
		return code
	}
	// what the card asks now, when it is not to launch a role: the launch
	// flags have nothing to do then
	noLanza := func(verbo string, imprimir func()) int {
		if f := flagEscrito(args); f != "" {
			ahora := "ahora te toca hoom " + verbo
			if verbo == "" {
				ahora = "la tarjeta ya esta en Hecho"
			}
			return v.uso(fmt.Sprintf("%s solo sirve cuando hoom siguiente lanza un rol; %s", f, ahora))
		}
		imprimir()
		return 0
	}
	out := v.e.Out
	switch {
	case c.Column == boardcmd.ColHecho:
		return noLanza("", func() { fmt.Fprintln(out, "la tarjeta ya esta en Hecho") })
	case c.Running != nil:
		r := c.Running
		return v.falla(fmt.Sprintf("%s esta trabajando (%s, paso %d de %d): espera a que termine", quien(r.Role), r.Stage, r.Step, r.Steps))
	case c.Interrupted != nil:
		i := c.Interrupted
		return noLanza("reanudar", func() {
			fmt.Fprintf(out, "el trabajo %s se corto en el paso %d de %d (%s). Elegi:\n", delRol(i.Role), i.Step, i.Steps, i.Stage)
			if a, ok := c.Action(boardcmd.ActReanudar); ok && !a.Enabled {
				fmt.Fprintf(out, "  hoom reanudar          no se puede: %s\n", a.Why)
			} else {
				fmt.Fprintln(out, "  hoom reanudar          retoma su sesion")
			}
			fmt.Fprintf(out, "  hoom pedir %-11s lo vuelve a lanzar desde como quedo\n", i.Role)
		})
	case c.Column == boardcmd.ColTuAprobacion:
		return noLanza("aprobar", func() { fmt.Fprintf(out, "te toca: hoom aprobar\n%s\n", c.Plain) })
	case c.Column == boardcmd.ColTuAceptacion:
		return noLanza("integrar", func() { fmt.Fprintf(out, "te toca: hoom integrar\n%s\n", c.Plain) })
	}
	if len(c.Actions) == 0 {
		return v.falla(fmt.Sprintf("en %s no hay nada que hacer ahora: %s", c.ColumnName, c.Plain))
	}
	a := c.Actions[0]
	if !boardcmd.IsRoleAction(a.ID) {
		verbo := ""
		if a.ID == boardcmd.ActGuardar {
			verbo = "guardar"
		}
		if verbo == "" {
			return v.falla(fmt.Sprintf("en %s no hay nada que hacer ahora: %s", c.ColumnName, c.Plain))
		}
		return noLanza(verbo, func() { fmt.Fprintf(out, "te toca: hoom %s\n%s\n", verbo, c.Plain) })
	}
	if !a.Enabled {
		msg := a.Why
		if g, ok := c.Action(boardcmd.ActGuardar); ok && g.Enabled {
			msg += "\n  siguiente: hoom guardar"
		}
		return v.falla(msg)
	}
	if a.Role == "reviewer" && l.escritos["m"] {
		return v.uso("la revision arma su propio pedido: no lleva -m")
	}
	return v.lanzarRol(d, m, c, a, &l)
}

// flagEscrito is the first launch flag of args, as it was written (--model,
// -m), or "".
func flagEscrito(args []string) string {
	for i := 0; i < len(args); i++ {
		s := args[i]
		if s == "--" {
			return ""
		}
		if len(s) < 2 || s[0] != '-' {
			continue
		}
		nombre := strings.TrimLeft(s, "-")
		escrito, conValor := s, false
		if j := strings.IndexByte(nombre, '='); j >= 0 {
			nombre, conValor = nombre[:j], true
			escrito = s[:strings.IndexByte(s, '=')]
		}
		if contiene(flagsLanzamiento, nombre) {
			return escrito
		}
		if !conValor && nombre != "json" {
			i++ // the value of a flag is not a flag
		}
	}
	return ""
}

func quien(role string) string {
	if strings.TrimSpace(role) == "" {
		return "el agente"
	}
	return "el " + role
}

func delRol(role string) string {
	if strings.TrimSpace(role) == "" {
		return "del agente"
	}
	return "del " + role
}

// ---------------------------------------------------------------- firmas

// firmar is hoom aprobar and hoom integrar: the card's signatures. They ask
// for a confirmation, --si skips it, and without a terminal --si is a must.
func (v *verbo) firmar(args []string) int {
	v.info = v.e.Out
	fs := flag.NewFlagSet(v.nombre, flag.ContinueOnError)
	si := fs.Bool("si", false, "")
	ops, code, ok := v.parsear(fs, args, 0, 1)
	if !ok {
		return code
	}
	d, code, ok := v.tarjeta(primero(ops))
	if !ok {
		return code
	}
	m, c, code, ok := v.carta(d)
	if !ok {
		return code
	}
	id, texto := boardcmd.ActAprobar, ""
	if v.nombre == "integrar" {
		id = boardcmd.ActIntegrar
	}
	a, ok := c.Action(id)
	if !ok {
		return v.falla(fmt.Sprintf("en %s no hay nada que %s: %s", c.ColumnName, v.nombre, c.Plain))
	}
	if !a.Enabled {
		return v.falla(a.Why)
	}
	if id == boardcmd.ActAprobar {
		n := c.Evidence.Criteria
		criterios := fmt.Sprintf("%d criterios", n)
		if n == 1 {
			criterios = "1 criterio"
		}
		texto = fmt.Sprintf("Vas a aprobar el spec de %q (%s) con tu identidad: %s. Si el spec cambia despues, la aprobacion deja de valer.",
			c.Item.Titulo, criterios, a.Signer)
	} else {
		texto = fmt.Sprintf("Se cierra el trabajo de %q: se comprueba una ultima vez que todo este verde y guardado, y la tarjeta pasa a Hecho. Juntarlo con el proyecto lo haces vos despues.",
			c.Item.Titulo)
	}
	if !*si {
		if !v.e.Terminal {
			return v.uso(fmt.Sprintf("sin terminal, %s necesita --si", v.nombre))
		}
		fmt.Fprintln(v.e.Out, texto)
		fmt.Fprint(v.e.Out, "¿Seguir? [s/N] ")
		resp, _ := bufio.NewReader(v.e.In).ReadString('\n')
		if r := strings.ToLower(strings.TrimSpace(resp)); r != "s" && r != "si" {
			return v.falla("no se firmo nada")
		}
	}
	if id == boardcmd.ActAprobar {
		dir := d.Root
		if c.Evidence.Source == boardcmd.SourceWorktree {
			dir = filepath.Join(d.Root, filepath.FromSlash(c.Evidence.Dir))
		}
		spec := ".hoom/specs/" + d.Slug + ".md"
		rec, _, err := approval.Approve(dir, filepath.Join(dir, filepath.FromSlash(spec)))
		if err != nil {
			return v.falla(err.Error())
		}
		fmt.Fprintf(v.e.Out, "aprobado: %s (sha256 %s)\n", spec, corto(rec.SHA256, 8))
		return 0
	}
	if err := taskcmd.Done(d.Root, d.Slug, m.BaseBranch, false); err != nil {
		return v.falla(err.Error())
	}
	fmt.Fprintln(v.e.Out, "listo: la tarjeta pasa a Hecho")
	fmt.Fprintf(v.e.Out, "para juntarla: git merge --no-ff hoom/%s\n", d.Slug)
	return 0
}

// ---------------------------------------------------------------- guardar

func (v *verbo) guardar(args []string) int {
	v.info = v.e.Out
	fs := flag.NewFlagSet("guardar", flag.ContinueOnError)
	ops, code, ok := v.parsear(fs, args, 0, 1)
	if !ok {
		return code
	}
	d, code, ok := v.tarjeta(primero(ops))
	if !ok {
		return code
	}
	m, c, code, ok := v.carta(d)
	if !ok {
		return code
	}
	if len(c.Unsynced) == 0 {
		fmt.Fprintln(v.e.Out, "no hay nada sin guardar")
		return 0
	}
	if a, ok := c.Action(boardcmd.ActGuardar); ok && !a.Enabled {
		return v.falla(a.Why)
	}
	res, err := itemcmd.Save(d.Root, m.BaseBranch, m.FindingsBlockOn(), d.Slug, nil)
	if err != nil {
		return v.falla(err.Error())
	}
	if len(res.Commits) == 0 {
		fmt.Fprintln(v.e.Out, "no hay nada sin guardar")
		return 0
	}
	for _, cm := range res.Commits {
		fmt.Fprintf(v.e.Out, "guardado en %s: %s (%s)\n", cm.Dir, corto(cm.SHA, 12), res.Message)
		for _, p := range cm.Paths {
			fmt.Fprintf(v.e.Out, "  %s\n", p)
		}
	}
	return 0
}

// ---------------------------------------------------------------- helpers

func primero(ops []string) string {
	if len(ops) > 0 {
		return ops[0]
	}
	return ""
}

func contiene(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func corto(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}
