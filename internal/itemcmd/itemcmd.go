// Package itemcmd implements `hoom item add | list | show | save | set |
// backfill`: the verbs of the card as a file (backfill gives an item to the
// tasks from before the cabin that the evidence holds; set changes the
// card's riesgo). Parsing is pure and strict from day one (cliargs), so a
// request hoom does not understand is answered before the project is read,
// and it never leaves a half-written item behind.
package itemcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/procesos"
	"github.com/hoomdev/hoomai/internal/riesgo"
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
	// SubSet is `hoom item set <slug> --riesgo r [--json]`: it changes the
	// card's riesgo and nothing else (ciclo-de-review-acotado). --riesgo is
	// the only flag that changes something, so it is required: Parse answers
	// a usage error (exit 2) without it, with an empty one, or with a value
	// outside riesgo.Vocabulario (that one with item.Parse's text,
	// `riesgo "<r>" fuera del vocabulario (bajo, normal, alto)`), and for a
	// slug that is not one, as show and save do. What Execute does is Set's
	// contract.
	//
	// autonomia-entre-firmas: set also takes `--auto hasta-humano|no` and
	// `--presupuesto-usd x`, combinable with --riesgo (Request.Auto,
	// Request.PresupuestoUSD). Without any of the three it is the usage error
	// `falta --riesgo, --auto o --presupuesto-usd: es lo que set cambia`
	// (exit 2); a --presupuesto-usd that is not a number above 0 is the usage
	// error `--presupuesto-usd tiene que ser un numero mayor que 0` (exit 2).
	// What Execute does with them is SetCambios' contract.
	SubSet = "set"
)

// AutoNo is the value of `hoom item set --auto` that removes the card's
// `auto` key (the item never stores it).
const AutoNo = "no"

// UsageText is the exact usage block of `hoom item` and the single source of
// that text.
const UsageText = `Uso: hoom item add "<titulo>" [--tipo t] [--prioridad p] [--riesgo r] [--presupuesto-usd x] [--pedido "<texto>"] [--slug s] [--auto hasta-humano] [--json]
     hoom item list [--json]
     hoom item show <slug> [--json]
     hoom item save <slug> [--json]
     hoom item set <slug> [--riesgo r] [--auto hasta-humano|no] [--presupuesto-usd x] [--json]
     hoom item backfill [--dry-run] [--json]

  --tipo t             feature|bug|refactor|seguridad|docs|test (default feature)
  --prioridad p        alta|media|baja (default media)
  --riesgo r           bajo|normal|alto (default normal): cuanta review pide
                       la tarjeta (bajo: 1 lente salvo ruta de riesgo; alto:
                       las 4 siempre y sus medium de la review completa
                       bloquean)
  --presupuesto-usd x  Tope de gasto de la tarjeta en USD (> 0; opcional)
  --pedido "<texto>"   Lo que va a recibir el arquitecto (opcional)
  --slug s             Slug del item (default: derivado del titulo); es el
                       nombre del spec (.hoom/specs/<slug>.md) y de la tarea
  --auto hasta-humano  Activa el piloto automatico de la tarjeta (en el Studio
                       y con 'hoom piloto <slug>'; exige --presupuesto-usd):
                       despues de cada trabajo pide el del rol siguiente
                       hasta la proxima columna tuya o un tope
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

'hoom item set' cambia el riesgo, el piloto automatico (--auto no lo quita)
y el presupuesto de la tarjeta; no commitea (guardalo con 'hoom item save
<slug>'). Lo cambia una persona, nunca un rol.
Un titulo que empieza con '-' va despues de '--': hoom item add -- "-titulo"`

// Request is one parsed `hoom item` invocation. `hoom item add --riesgo r`
// travels in Draft.Riesgo: a value outside riesgo.Vocabulario is a usage
// error of Parse (exit 2, like --tipo) with item.Parse's text, and no item
// is created; without the flag, or with --riesgo normal, the file has no
// `riesgo` key.
type Request struct {
	Sub   string     // SubAdd | SubList | SubShow | SubSave | SubSet | SubBackfill
	Draft item.Draft // add
	Slug  string     // show, save, set
	// Riesgo is set's --riesgo, already checked against riesgo.Vocabulario.
	Riesgo string
	// Auto is set's --auto: item.AutoHastaHumano or AutoNo, "" = not given
	// (autonomia-entre-firmas).
	Auto string
	// PresupuestoUSD is set's --presupuesto-usd, already checked to be a
	// finite number above 0; nil = not given.
	PresupuestoUSD *float64
	JSON           bool
	DryRun         bool // backfill
}

// SetResult is what `hoom item set` did, identical in text and JSON
// (`{"slug": ..., "antes": ..., "riesgo": ...}`). Antes is the riesgo the
// card had (riesgo.Normal when its item had no key) and Riesgo the one it
// has now; they are equal when the card already had it.
type SetResult struct {
	Slug   string `json:"slug"`
	Antes  string `json:"antes"`
	Riesgo string `json:"riesgo"`
	// Cambio says whether the item's file was written (false when the card
	// already had that riesgo). Not part of the JSON.
	Cambio bool `json:"-"`
	// autonomia-entre-firmas: the card's auto and presupuesto_usd AFTER the
	// set (SetCambios), in the JSON only when the card has them: `"auto":
	// "hasta-humano"` and `"presupuesto_usd": <x>`. A set of --riesgo alone
	// emits exactly the JSON of before.
	Auto           string   `json:"auto,omitempty"`
	PresupuestoUSD *float64 `json:"presupuesto_usd,omitempty"`
	// AutoAntes and PresupuestoAntes are the values before the set; not part
	// of the JSON.
	AutoAntes        string   `json:"-"`
	PresupuestoAntes *float64 `json:"-"`
}

// Set is `hoom item set <slug> --riesgo r`: it changes only the riesgo of
// the item slug of the project at root (item.SetRiesgo: every other key
// keeps its value, the file is rewritten the way hoom writes it) and commits
// nothing. rol is the role of the run the process runs inside (Execute
// passes $HOOM_ROLE; "" = not inside one).
//
// It refuses, writing nothing, with these errors (exit 1 in the CLI), in
// this order:
//
//   - rol not empty: `el riesgo de una tarjeta lo cambia una persona, no un rol`;
//   - no item slug: `no existe el item "<slug>"`;
//   - the card (boardcmd.CardFor with base and blockOn) in Hecho:
//     `la tarjeta ya esta en Hecho`, the text the board and `hoom integrar`
//     use.
//
// A card that already has r is a SetResult with Antes == Riesgo == r, Cambio
// false and nothing written.
//
// Execute prints, for a change,
// `riesgo de <slug>: <antes> -> <despues> - guardalo con: hoom item save <slug>`,
// and for a card that already had it `riesgo de <slug>: ya es <r>`; with
// --json, the SetResult in both cases.
func Set(root, base, blockOn, slug, r, rol string) (SetResult, error) {
	if strings.TrimSpace(rol) != "" {
		return SetResult{}, fmt.Errorf("el riesgo de una tarjeta lo cambia una persona, no un rol")
	}
	if err := riesgo.Validar(r); err != nil || r == "" {
		if err == nil {
			err = fmt.Errorf("falta el riesgo (bajo|normal|alto)")
		}
		return SetResult{}, err
	}
	// the item written is the one the riesgo is read from: the main
	// checkout's when it has it, else this tree's (findings aee796, f91dcf)
	dest, ok := item.ArbolDelRiesgo(root, slug)
	if !ok {
		return SetResult{}, fmt.Errorf("no existe el item %q", slug)
	}
	// Hecho is judged on the card of the tree Set writes in, the same item
	// (findings 822b51, c55dde): a historic card stays Hecho as before
	c, err := boardcmd.CardFor(dest, base, blockOn, slug, time.Now().UTC())
	if err != nil {
		return SetResult{}, err
	}
	if c.Column == boardcmd.ColHecho {
		return SetResult{}, fmt.Errorf("la tarjeta ya esta en Hecho")
	}
	antes, cambio, err := item.SetRiesgo(dest, slug, r)
	if errors.Is(err, os.ErrNotExist) {
		return SetResult{}, fmt.Errorf("no existe el item %q", slug)
	}
	if err != nil {
		return SetResult{}, err
	}
	return SetResult{Slug: slug, Antes: antes, Riesgo: r, Cambio: cambio}, nil
}

// SetCambios is `hoom item set <slug>` with any combination of --riesgo,
// --auto and --presupuesto-usd (autonomia-entre-firmas): Set's contract for
// all three keys, in ONE write of the same item Set writes in
// (item.ArbolDelRiesgo: the main checkout's when it has the item, else this
// tree's), committing nothing. c carries what was given (nil fields are not
// changed; Auto "" removes the key, the item side of `--auto no`).
//
// It refuses, writing nothing, with these errors (exit 1 in the CLI), in
// this order:
//
//   - rol not empty and c changes auto or presupuesto_usd:
//     `el piloto automatico y el presupuesto de una tarjeta los cambia una persona, no un rol`;
//     rol not empty and c changes only the riesgo: Set's
//     `el riesgo de una tarjeta lo cambia una persona, no un rol`;
//   - no item slug: `no existe el item "<slug>"`;
//   - the card in Hecho: `la tarjeta ya esta en Hecho`;
//   - auto hasta-humano on an item that would stay without presupuesto_usd
//     (it has none and c brings none): item.ErrAutoSinPresupuesto,
//     `--auto necesita --presupuesto-usd: el piloto automatico no corre sin tope`.
//
// The SetResult carries the riesgo before and after (as Set's), the auto
// and presupuesto_usd after, the ones before, and Cambio.
func SetCambios(root, base, blockOn, slug string, c item.Cambios, rol string) (SetResult, error) {
	if strings.TrimSpace(rol) != "" {
		if c.Auto != nil || c.PresupuestoUSD != nil {
			return SetResult{}, fmt.Errorf("el piloto automatico y el presupuesto de una tarjeta los cambia una persona, no un rol")
		}
		return SetResult{}, fmt.Errorf("el riesgo de una tarjeta lo cambia una persona, no un rol")
	}
	if c.Riesgo == nil && c.Auto == nil && c.PresupuestoUSD == nil {
		return SetResult{}, fmt.Errorf("falta --riesgo, --auto o --presupuesto-usd: es lo que set cambia")
	}
	// the item written is the one the card is read from, like Set's
	dest, ok := item.ArbolDelRiesgo(root, slug)
	if !ok {
		return SetResult{}, fmt.Errorf("no existe el item %q", slug)
	}
	card, err := boardcmd.CardFor(dest, base, blockOn, slug, time.Now().UTC())
	if err != nil {
		return SetResult{}, err
	}
	if card.Column == boardcmd.ColHecho {
		return SetResult{}, fmt.Errorf("la tarjeta ya esta en Hecho")
	}
	antes, despues, cambio, err := item.Cambiar(dest, slug, c)
	if errors.Is(err, os.ErrNotExist) {
		return SetResult{}, fmt.Errorf("no existe el item %q", slug)
	}
	if err != nil {
		return SetResult{}, err
	}
	return SetResult{Slug: slug, Antes: antes.Riesgo, Riesgo: despues.Riesgo, Cambio: cambio,
		Auto: despues.Auto, PresupuestoUSD: despues.PresupuestoUSD,
		AutoAntes: antes.Auto, PresupuestoAntes: antes.PresupuestoUSD}, nil
}

// cambiosDe is what a parsed `hoom item set` asks to change.
func cambiosDe(req Request) item.Cambios {
	var c item.Cambios
	if req.Riesgo != "" {
		r := req.Riesgo
		c.Riesgo = &r
	}
	switch req.Auto {
	case item.AutoHastaHumano:
		a := item.AutoHastaHumano
		c.Auto = &a
	case AutoNo:
		a := ""
		c.Auto = &a
	}
	if req.PresupuestoUSD != nil {
		v := *req.PresupuestoUSD
		c.PresupuestoUSD = &v
	}
	return c
}

// renderSet prints what a set of auto or presupuesto_usd did, one line per
// key it changed.
func renderSet(w io.Writer, res SetResult) {
	if !res.Cambio {
		fmt.Fprintf(w, "tarjeta %s: ya tenia esos valores\n", res.Slug)
		return
	}
	guardar := " - guardalo con: hoom item save " + res.Slug
	if res.Antes != res.Riesgo {
		fmt.Fprintf(w, "riesgo de %s: %s -> %s%s\n", res.Slug, res.Antes, res.Riesgo, guardar)
	}
	autoDe := func(a string) string {
		if a == "" {
			return AutoNo
		}
		return a
	}
	if res.AutoAntes != res.Auto {
		fmt.Fprintf(w, "auto de %s: %s -> %s%s\n", res.Slug, autoDe(res.AutoAntes), autoDe(res.Auto), guardar)
	}
	topeDe := func(v *float64) string {
		if v == nil {
			return "sin tope"
		}
		return strconv.FormatFloat(*v, 'f', -1, 64) + " USD"
	}
	if topeDe(res.PresupuestoAntes) != topeDe(res.PresupuestoUSD) {
		fmt.Fprintf(w, "presupuesto de %s: %s -> %s%s\n", res.Slug, topeDe(res.PresupuestoAntes), topeDe(res.PresupuestoUSD), guardar)
	}
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
		return Request{}, uso("item", "falta el subcomando (add|list|show|save|set|backfill)")
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
	case SubSet:
		return parseSet(rest)
	case SubBackfill:
		fs := flag.NewFlagSet("item backfill", flag.ContinueOnError)
		dryRun := fs.Bool("dry-run", false, "lista lo que crearia sin escribir nada")
		asJSON := fs.Bool("json", false, "emitir el resultado como JSON")
		if err := cliargs.Strict(fs, rest, "item backfill", UsageText); err != nil {
			return Request{}, err
		}
		return Request{Sub: SubBackfill, DryRun: *dryRun, JSON: *asJSON}, nil
	}
	return Request{}, uso("item", fmt.Sprintf("subcomando desconocido %q (add|list|show|save|set|backfill)", sub))
}

// parseSet is `hoom item set <slug> [--riesgo r] [--auto hasta-humano|no]
// [--presupuesto-usd x] [--json]`: at least one of the three, each checked
// against its vocabulary.
func parseSet(args []string) (Request, error) {
	fs := flag.NewFlagSet("item set", flag.ContinueOnError)
	r := fs.String("riesgo", "", "riesgo de la tarjeta: bajo|normal|alto")
	auto := fs.String("auto", "", "piloto automatico: hasta-humano|no")
	presupuesto := fs.String("presupuesto-usd", "", "tope de gasto en USD")
	asJSON := fs.Bool("json", false, "emitir el resultado como JSON")
	// an empty --presupuesto-usd is not a number above 0: set says so
	ops, err := cliargs.OperandsVacios(fs, args, "item set", UsageText, 1, "presupuesto-usd")
	if err != nil {
		return Request{}, err
	}
	if !item.ValidSlug(ops[0]) {
		return Request{}, uso("item set", fmt.Sprintf("slug invalido %q: minusculas, numeros y guiones", ops[0]))
	}
	req := Request{Sub: SubSet, Slug: ops[0], JSON: *asJSON}
	conPresupuesto := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "presupuesto-usd" {
			conPresupuesto = true
		}
	})
	req.Riesgo = strings.TrimSpace(*r)
	req.Auto = strings.TrimSpace(*auto)
	if req.Riesgo == "" && req.Auto == "" && !conPresupuesto {
		return Request{}, uso("item set", "falta --riesgo, --auto o --presupuesto-usd: es lo que set cambia")
	}
	if req.Riesgo != "" {
		if err := riesgo.Validar(req.Riesgo); err != nil {
			return Request{}, uso("item set", err.Error())
		}
	}
	if req.Auto != "" && req.Auto != item.AutoHastaHumano && req.Auto != AutoNo {
		return Request{}, uso("item set", fmt.Sprintf("--auto tiene que ser %s o %s", item.AutoHastaHumano, AutoNo))
	}
	if conPresupuesto {
		v, err := strconv.ParseFloat(strings.TrimSpace(*presupuesto), 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || !(v > 0) {
			return Request{}, uso("item set", "--presupuesto-usd tiene que ser un numero mayor que 0")
		}
		req.PresupuestoUSD = &v
	}
	return req, nil
}

func parseAdd(args []string) (Request, error) {
	fs := flag.NewFlagSet("item add", flag.ContinueOnError)
	tipo := fs.String("tipo", "", "tipo del item")
	prioridad := fs.String("prioridad", "", "prioridad del item")
	rsg := fs.String("riesgo", "", "riesgo de la tarjeta: bajo|normal|alto")
	presupuesto := fs.Float64("presupuesto-usd", 0, "tope de gasto en USD")
	pedido := fs.String("pedido", "", "pedido para el arquitecto")
	slug := fs.String("slug", "", "slug del item")
	auto := fs.String("auto", "", "piloto automatico: hasta-humano")
	asJSON := fs.Bool("json", false, "emitir el item como JSON")
	ops, err := cliargs.Operands(fs, args, "item add", UsageText, 1)
	if err != nil {
		return Request{}, err
	}
	d := item.Draft{Titulo: ops[0], Tipo: *tipo, Prioridad: *prioridad, Riesgo: *rsg, Pedido: *pedido, Slug: *slug, Auto: *auto}
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
	case SubSet:
		if req.Auto != "" || req.PresupuestoUSD != nil {
			res, err := SetCambios(root, base, blockOn, req.Slug, cambiosDe(req), os.Getenv(runcmd.EnvRole))
			if err != nil {
				return err
			}
			if req.JSON {
				return emit(stdout, res)
			}
			renderSet(stdout, res)
			return nil
		}
		res, err := Set(root, base, blockOn, req.Slug, req.Riesgo, os.Getenv(runcmd.EnvRole))
		if err != nil {
			return err
		}
		if req.JSON {
			return emit(stdout, res)
		}
		if !res.Cambio {
			fmt.Fprintf(stdout, "riesgo de %s: ya es %s\n", res.Slug, res.Riesgo)
			return nil
		}
		fmt.Fprintf(stdout, "riesgo de %s: %s -> %s - guardalo con: hoom item save %s\n", res.Slug, res.Antes, res.Riesgo, res.Slug)
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
	return uso("item", fmt.Sprintf("subcomando desconocido %q (add|list|show|save|set|backfill)", req.Sub))
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
//
// With expect nil (`hoom item save`) it saves the card from the tree `hoom
// item set` writes its item in (item.ArbolDelRiesgo): run from a task's
// worktree, when the project's main checkout has the item, it saves exactly
// as `hoom item save` run from the project does (the workspace and the
// project's item, one commit each), and the commits' Dir are relative to the
// project. With the item only in the task's tree, it saves there (finding
// 42ffc0).
//
// With a non-nil expect (the Studio) it saves exactly what the board of root
// shows, in root: the main checkout's item stays for `hoom item save`
// (findings d26397, f4d55b).
//
// Run from inside a task's worktree, for any card and with or without
// expect, it first goes through the gate of that worktree, the one the main
// checkout uses (runcmd.TaskDir): off its branch hoom/<task>, or with a
// detached HEAD, it commits nothing (findings 68644c, 13bba6, 65fbb7).
func Save(root, base, blockOn, slug string, expect []string) (SaveResult, error) {
	if p, tarea, ok := tareaDe(root, slug); ok {
		if _, err := runcmd.TaskDir(p, tarea); err != nil {
			return SaveResult{}, err
		}
	}
	if dest, ok := item.ArbolDelRiesgo(root, slug); ok && expect == nil && !mismoArbol(dest, root) {
		return Save(dest, base, blockOn, slug, expect)
	}
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
		sha, err := commitPaths(dir, res.Message, local, gitDelGuardado{}, nil)
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

// TopeGit acota cada comando git del guardado de la tarjeta (SaveTarea): un
// comando, con lo que corre (sus hooks, un filtro de git), que no termina en ese tiempo
// se mata con todo su grupo y el guardado falla con un error. Los tests lo
// achican.
var TopeGit = 5 * time.Minute

// SaveTarea is the pilot's save (autonomia-entre-firmas): it commits what the
// card's task worktree has unsaved, everything in it, in that worktree on its
// branch hoom/<slug>, with SaveMessage(slug), and nothing else. root is the
// project (the main checkout). It never commits in the main checkout (the
// item changed with `hoom item set` stays for `hoom item save`) and never
// pushes. With nothing to save it commits nothing: a SaveResult with no
// Commits. Its one commit, when there is one, is SaveResult.Commits[0], with
// Dir relative to root.
//
// It refuses, committing nothing, with Save's errors: a card without its
// task's worktree, a worktree off its branch or with a detached HEAD
// (runcmd.TaskDir's error), a job running on the card while the worktree
// has something to save (`espera a que termine <quien> que esta
// trabajando`) or an agent starting in it.
//
// It reads the card with boardcmd.PulsoDe, which runs no git that reads the
// working tree, and each git command that does (status, add, update-index,
// commit, rev-parse HEAD) runs bounded by TopeGit in a process group of its
// own (aclaraciones 17 y 19). The repository's hooks run, as in any commit
// of it (aclaracion 23): one that refuses fails the save, one that never
// ends is cut with its group. A save that fails leaves the task's branch
// where it was (deshacer): only that branch, the one HEAD named when it
// started, and only while its tip is this save's commit (aclaracion 26).
// The commit holds exactly the paths that were unsaved when it started:
// one a hook slips in or takes out, or a protected test of the task that
// changes meanwhile, fails it (revisarCommit, aclaraciones 24 y 26). base
// and blockOn are not needed for that.
func SaveTarea(root, base, blockOn, slug string) (SaveResult, error) {
	dir, err := tareaGuardable(root, slug)
	if err != nil {
		return SaveResult{}, err
	}
	// under the task tree's lock, like Save: a run that starts meanwhile
	// waits until the commit is done (finding ac2feb)
	unlock, err := TomarArbol(dir, slug)
	if err != nil {
		return SaveResult{}, err
	}
	defer unlock()
	return guardarTarea(root, slug, dir)
}

// SaveTareaBajoCandado is SaveTarea for a caller that already holds the
// task tree's lock (TomarArbol): the pilot's cerrar-hallazgos, which holds it
// from its first resolution until it saves (finding 8f4011).
func SaveTareaBajoCandado(root, base, blockOn, slug string) (SaveResult, error) {
	dir, err := tareaGuardable(root, slug)
	if err != nil {
		return SaveResult{}, err
	}
	return guardarTarea(root, slug, dir)
}

// TomarArbol takes runcmd.LockArbol of the task tree dir of the card slug,
// with Save's error when an agent is starting in it.
func TomarArbol(dir, slug string) (func(), error) {
	unlock, err := runcmd.LockArbol(dir, runcmd.EsperaArbol)
	if err != nil {
		if errors.Is(err, hoomfs.ErrLocked) {
			return nil, fmt.Errorf("un agente esta arrancando en la tarjeta %s: guardar ahora commitearia su trabajo a medias. Accion: espera a que termine", slug)
		}
		return nil, err
	}
	return unlock, nil
}

// tareaGuardable is the task tree of the card slug, when the card has its
// task's worktree.
func tareaGuardable(root, slug string) (string, error) {
	dir, err := runcmd.TaskDir(root, slug)
	if err != nil {
		return "", err
	}
	p, err := boardcmd.PulsoDe(root, slug, time.Now().UTC())
	if err != nil {
		return "", err
	}
	if p.Source != boardcmd.SourceWorktree {
		return "", fmt.Errorf("la tarjeta %s no tiene el espacio de trabajo de su tarea: hoom task start %s", slug, slug)
	}
	return dir, nil
}

// guardarTarea is SaveTarea under the task tree's lock.
func guardarTarea(root, slug, dir string) (SaveResult, error) {
	res := SaveResult{Slug: slug, Message: SaveMessage(slug), Commits: []SaveCommit{}}
	p, err := boardcmd.PulsoDe(root, slug, time.Now().UTC())
	if err != nil {
		return SaveResult{}, err
	}
	if p.Source != boardcmd.SourceWorktree {
		return res, nil
	}
	g := delGuardado()
	local, err := sinGuardar(dir, g)
	if err != nil {
		return res, err
	}
	// nothing to save is never a refusal: the pilot then sees the job
	// running in its own case
	if len(local) == 0 {
		return res, nil
	}
	if r := p.Running; r != nil {
		quien := "el agente"
		if r.Role != "" {
			quien = "el " + r.Role
		}
		return SaveResult{}, fmt.Errorf("espera a que termine %s que esta trabajando", quien)
	}
	// what the save checks its commit against (aclaraciones 24 y 26): the
	// branch HEAD names before any hook runs and where it was, and the
	// task's protected tests, on disk and in the index
	prot, err := agentcmd.TestsProtegidos(root, slug)
	if err != nil {
		return res, fmt.Errorf("no pude leer los tests protegidos de la tarea: %w", err)
	}
	s, err := empezarGuardado(dir, g, slug)
	if err != nil {
		return res, err
	}
	foto, err := fotoProtegidos(dir, g, prot)
	if err != nil {
		return res, err
	}
	// the index entry the save itself staged for each path it asks for,
	// once staged and before its commit: the commit must bring every one
	// (aclaracion 26) and keep the blob of a protected test (aclaracion
	// 24), which a hook may restage without touching the disk
	var stageados map[string]string
	sha, err := commitPaths(dir, res.Message, local, g, func() error {
		var err error
		stageados, err = entradasDe(dir, g, "", local)
		return err
	})
	if err == nil {
		err = revisarCommit(dir, g, s, sha, local, stageados, foto)
	}
	if err != nil {
		return res, deshacer(dir, g, s, err)
	}
	paths := make([]string, len(local))
	for i, l := range local {
		paths[i] = p.Dir + "/" + l
	}
	res.Commits = append(res.Commits, SaveCommit{Dir: p.Dir, SHA: sha, Paths: paths})
	return res, nil
}

// protegidosDelGuardado photographs the task's protected tests for the save:
// their disk (agentcmd.FotoProtegidos, never git) and their index entries
// (indiceDe).
type protegidosDelGuardado struct {
	rutas  []string
	disco  agentcmd.Protegidos
	indice map[string]string
}

// guardado is what a save of the card knows of the commit it is about to
// make (aclaraciones 26 y 27): the branch HEAD named when it started, before
// any hook ran (rama, a full ref), where that branch was (antes), and the
// branch's reflog as it was then (reflogDelGuardado), where git writes the
// commit the save's git commit creates.
type guardado struct {
	rama, antes string
	reflog      reflogDelGuardado
}

// reflogDelGuardado is the reflog file of the save's branch (git rev-parse
// --git-path) and how it was when the save started: its size (largo) and its
// last bytes (cola), which must still be there when the save reads what git
// appended.
type reflogDelGuardado struct {
	ruta  string
	largo int64
	cola  []byte
}

// maxColaReflog is how many of the last bytes of the branch's reflog the
// save keeps to see, later, that git only appended to it; maxReflogNuevo
// bounds what it reads of what was appended during the save (a few lines).
const (
	maxColaReflog  = 512
	maxReflogNuevo = 1 << 20
)

// empezarGuardado reads, from gits of the save that run no hook, the branch
// the save of the card slug commits on: the one HEAD names, which must be
// the task's refs/heads/hoom/<slug> (runcmd.TaskDir's ErrOtraRama
// otherwise), its tip and its reflog.
func empezarGuardado(dir string, g gitDelGuardado, slug string) (guardado, error) {
	s := guardado{rama: "refs/heads/hoom/" + slug}
	ref, err := g.correr(dir, nil, "symbolic-ref", "-q", "HEAD")
	if errors.Is(err, errTopeGit) || err != nil && ref != "" {
		return guardado{}, fmt.Errorf("no pude ver la rama de la tarea antes de guardar: %s", motivoGit(ref, err))
	}
	if ref != s.rama {
		return guardado{}, fmt.Errorf("%w: la tarea %q no esta en hoom/%s: no guardo", runcmd.ErrOtraRama, slug, slug)
	}
	s.antes, err = g.correr(dir, nil, "rev-parse", "--verify", s.rama)
	if err != nil {
		return guardado{}, fmt.Errorf("no pude ver la punta de hoom/%s antes de guardar: %s", slug, motivoGit(s.antes, err))
	}
	s.reflog.ruta, err = g.correr(dir, nil, "rev-parse", "--path-format=absolute", "--git-path", "logs/"+s.rama)
	if err != nil {
		return guardado{}, fmt.Errorf("no pude ver el reflog de hoom/%s antes de guardar: %s", slug, motivoGit(s.reflog.ruta, err))
	}
	f, fi, err := abrirReflog(s.reflog.ruta)
	if err != nil {
		return guardado{}, fmt.Errorf("no pude leer el reflog de hoom/%s antes de guardar: %w", slug, err)
	}
	if f != nil {
		defer f.Close()
		s.reflog.largo = fi.Size()
		s.reflog.cola = make([]byte, min(s.reflog.largo, maxColaReflog))
		if _, err := f.ReadAt(s.reflog.cola, s.reflog.largo-int64(len(s.reflog.cola))); err != nil {
			return guardado{}, fmt.Errorf("no pude leer el reflog de hoom/%s antes de guardar: %w", slug, err)
		}
	}
	return s, nil
}

// abrirReflog opens the reflog file at ruta when it is a regular file (never
// a FIFO, which would hang the save); nil when there is none yet (git
// creates it with the first update of the branch).
func abrirReflog(ruta string) (*os.File, fs.FileInfo, error) {
	fi, err := os.Lstat(ruta)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	if !fi.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("%s no es un archivo", ruta)
	}
	f, err := os.Open(ruta)
	if err != nil {
		return nil, nil, err
	}
	if fi, err = f.Stat(); err != nil || !fi.Mode().IsRegular() {
		f.Close()
		return nil, nil, fmt.Errorf("%s no es un archivo", ruta)
	}
	return f, fi, nil
}

// nombre is the save's branch as git shows it: hoom/<slug>.
func (s guardado) nombre() string {
	return strings.TrimPrefix(s.rama, "refs/heads/")
}

// suCommit is the commit this save's git commit created (aclaracion 27), as
// git wrote it in the branch's reflog, whatever its message (a
// prepare-commit-msg or commit-msg hook may rewrite it): the new value of
// the first entry appended during the save whose old value is antes. git
// commit moves the branch only from the tip it started on, antes, so a
// commit a hook makes on top of it, or one that replaces it (--amend), is a
// later entry. ok is false when there is none: git did not move the branch.
// It is an error when the reflog was not only appended to since the save
// started.
func (s guardado) suCommit() (sha string, ok bool, err error) {
	f, fi, err := abrirReflog(s.reflog.ruta)
	if err != nil {
		return "", false, err
	}
	if f == nil {
		if s.reflog.largo > 0 {
			return "", false, fmt.Errorf("el reflog de %s desaparecio durante el guardado", s.nombre())
		}
		return "", false, nil
	}
	defer f.Close()
	cola := make([]byte, len(s.reflog.cola))
	if fi.Size() < s.reflog.largo {
		return "", false, fmt.Errorf("el reflog de %s se acorto durante el guardado", s.nombre())
	}
	if _, err := f.ReadAt(cola, s.reflog.largo-int64(len(cola))); err != nil || !bytes.Equal(cola, s.reflog.cola) {
		return "", false, fmt.Errorf("el reflog de %s cambio durante el guardado", s.nombre())
	}
	if fi.Size()-s.reflog.largo > maxReflogNuevo {
		return "", false, fmt.Errorf("el reflog de %s crecio demasiado durante el guardado", s.nombre())
	}
	nuevo := make([]byte, fi.Size()-s.reflog.largo)
	if _, err := f.ReadAt(nuevo, s.reflog.largo); err != nil {
		return "", false, err
	}
	// <old> <new> <ident> <time> <tz>[\t<message>]\n; a line not written
	// whole (a git cut at its bound) is not an entry
	for _, l := range strings.SplitAfter(string(nuevo), "\n") {
		if !strings.HasSuffix(l, "\n") {
			break
		}
		cabecera, _, _ := strings.Cut(l, "\t")
		if c := strings.Fields(cabecera); len(c) >= 2 && c[0] == s.antes && c[1] != s.antes {
			return c[1], true, nil
		}
	}
	return "", false, nil
}

func fotoProtegidos(dir string, g gitDelGuardado, rutas []string) (protegidosDelGuardado, error) {
	idx, err := indiceDe(dir, g, rutas)
	if err != nil {
		return protegidosDelGuardado{}, err
	}
	return protegidosDelGuardado{rutas: rutas, disco: agentcmd.FotoProtegidos(dir, rutas), indice: idx}, nil
}

// indiceDe is the index entry of each path of rutas in dir (`git ls-files -s
// -v`: tag, mode, blob and stage; one per stage of a conflict), from a git
// of the save. It reads the index only: no hook, no filter.
func indiceDe(dir string, g gitDelGuardado, rutas []string) (map[string]string, error) {
	idx := map[string]string{}
	if len(rutas) == 0 {
		return idx, nil
	}
	out, err := g.listado(dir, append([]string{"--literal-pathspecs", "-c", "core.quotePath=false",
		"ls-files", "-s", "-v", "-z", "--"}, rutas...)...)
	if err != nil {
		return nil, fmt.Errorf("no pude leer los tests protegidos en el indice: %w", err)
	}
	for _, e := range strings.Split(string(out), "\x00") {
		if meta, ruta, ok := strings.Cut(e, "\t"); ok {
			idx[ruta] += meta + ";"
		}
	}
	return idx, nil
}

// revisarCommit checks that the save's commit sha is where the save put it
// and holds what the save asked for and nothing a hook slipped in
// (aclaraciones 24, 26 y 27): sha is the commit the save's git commit
// created (guardado.suCommit), with antes as its one parent, HEAD still
// names the save's branch and its tip is sha; every path it changes is one
// of pedidas, and it changes every one of them whose entry the save staged
// (stageados) is not the one in antes; and no protected test of the task
// changed during the save: on disk, nor in the index unless it is one of
// pedidas (staging those is the save's own doing). For those, the commit
// keeps the mode and blob the save staged: another one is a hook's, whatever
// the disk says. Its message is not checked: a hook may rewrite it.
func revisarCommit(dir string, g gitDelGuardado, s guardado, sha string, pedidas []string, stageados map[string]string, foto protegidosDelGuardado) error {
	suyo, ok, err := s.suCommit()
	if err != nil {
		return fmt.Errorf("no pude revisar el commit del guardado: %w", err)
	}
	if !ok || suyo != sha {
		return fmt.Errorf("el commit %s no es el que creo este guardado en %s", short(sha), s.nombre())
	}
	padres, err := g.correr(dir, nil, "rev-list", "--parents", "-n", "1", sha)
	if err != nil {
		return fmt.Errorf("no pude revisar el commit del guardado: %s", motivoGit(padres, err))
	}
	if f := strings.Fields(padres); len(f) != 2 || f[1] != s.antes {
		return fmt.Errorf("el commit del guardado %s no sale de %s", short(sha), short(s.antes))
	}
	ref, err := g.correr(dir, nil, "symbolic-ref", "-q", "HEAD")
	if errors.Is(err, errTopeGit) || err != nil && ref != "" {
		return fmt.Errorf("no pude revisar el commit del guardado: %s", motivoGit(ref, err))
	}
	if ref != s.rama {
		return fmt.Errorf("durante el guardado HEAD dejo de nombrar %s", s.nombre())
	}
	punta, err := g.correr(dir, nil, "rev-parse", "--verify", s.rama)
	if err != nil {
		return fmt.Errorf("no pude revisar el commit del guardado: %s", motivoGit(punta, err))
	}
	if punta != sha {
		return fmt.Errorf("la punta de %s es %s y no el commit del guardado %s", s.nombre(), short(punta), short(sha))
	}
	antes := s.antes
	out, err := g.listado(dir, "-c", "core.quotePath=false", "diff-tree", "-r", "--no-commit-id",
		"--name-only", "--no-renames", "-z", antes, sha)
	if err != nil {
		return fmt.Errorf("no pude revisar el commit del guardado: %w", err)
	}
	pedida := map[string]bool{}
	for _, p := range pedidas {
		pedida[p] = true
	}
	var extra []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" && !pedida[p] {
			extra = append(extra, p)
		}
	}
	if len(extra) > 0 {
		return fmt.Errorf("el commit del guardado trae rutas que no pidio: %q", extra)
	}
	enAntes, err := entradasDe(dir, g, antes, pedidas)
	if err != nil {
		return err
	}
	enCommit, err := entradasDe(dir, g, sha, pedidas)
	if err != nil {
		return err
	}
	var faltan []string
	for _, p := range pedidas {
		if stageados[p] != enAntes[p] && enCommit[p] == enAntes[p] {
			faltan = append(faltan, p)
		}
	}
	if len(faltan) > 0 {
		return fmt.Errorf("el commit del guardado no trae rutas que pidio: %q", faltan)
	}
	disco := agentcmd.FotoProtegidos(dir, foto.rutas)
	idx, err := indiceDe(dir, g, foto.rutas)
	if err != nil {
		return err
	}
	for _, p := range foto.rutas {
		if disco[p] != foto.disco[p] || !pedida[p] && idx[p] != foto.indice[p] {
			return fmt.Errorf("durante el guardado cambio el test protegido %q", p)
		}
	}
	for _, p := range enComun(foto.rutas, pedidas) {
		if enCommit[p] != stageados[p] {
			return fmt.Errorf("el commit del guardado no trae el test protegido %q que stageo el guardado", p)
		}
	}
	return nil
}

// entradasDe is the mode and blob of each path of rutas in dir: in the index
// (arbol "") or in the tree of the commit arbol, from a git of the save; a
// path that has none is not in the map. It reads the index or the objects
// only: no hook, no filter, nothing from the disk.
func entradasDe(dir string, g gitDelGuardado, arbol string, rutas []string) (map[string]string, error) {
	out := map[string]string{}
	if len(rutas) == 0 {
		return out, nil
	}
	args := []string{"--literal-pathspecs", "-c", "core.quotePath=false", "ls-files", "-s", "-z", "--"}
	if arbol != "" {
		args = []string{"--literal-pathspecs", "-c", "core.quotePath=false", "ls-tree", "-r", "-z", arbol, "--"}
	}
	raw, err := g.listado(dir, append(args, rutas...)...)
	if err != nil {
		return nil, fmt.Errorf("no pude leer las entradas de lo que guarda: %w", err)
	}
	for _, e := range strings.Split(string(raw), "\x00") {
		meta, ruta, ok := strings.Cut(e, "\t")
		f := strings.Fields(meta)
		if !ok || len(f) != 3 {
			continue
		}
		// ls-files -s: <mode> <blob> <stage>; ls-tree: <mode> <type> <blob>
		blob := f[1]
		if arbol != "" {
			blob = f[2]
		}
		out[ruta] += f[0] + " " + blob + ";"
	}
	return out, nil
}

// enComun are the paths of a that are also in b, in a's order.
func enComun(a, b []string) []string {
	en := map[string]bool{}
	for _, p := range b {
		en[p] = true
	}
	var out []string
	for _, p := range a {
		if en[p] {
			out = append(out, p)
		}
	}
	return out
}

// deshacer leaves the task's branch where it was when the save s failed
// with err (aclaraciones 24, 26 y 27). It moves s.rama, the branch HEAD
// named when the save started, never HEAD nor another branch a hook made
// HEAD name, and only when its tip is the commit this save's git commit
// created (guardado.suCommit), whatever its message: with a commit on top of
// it, one that replaced it (even with its parent and message) or any other
// tip, it stays where it is and the error says so. The move is an
// update-ref of the ref itself conditioned on that tip, bounded and in its
// group like every git of the save (the reference-transaction hook runs
// there too). The error is err, and says whether the branch went back or
// why it did not.
func deshacer(dir string, g gitDelGuardado, s guardado, err error) error {
	punta, rerr := g.correr(dir, nil, "rev-parse", "--verify", s.rama)
	if rerr != nil {
		return fmt.Errorf("%w; y no pude ver donde quedo %s: %s", err, s.nombre(), motivoGit(punta, rerr))
	}
	if punta == s.antes {
		return err
	}
	suyo, ok, serr := s.suCommit()
	if serr != nil {
		return fmt.Errorf("%w; y no pude ver que commit creo este guardado: %v: no muevo %s", err, serr, s.nombre())
	}
	if !ok || punta != suyo {
		return fmt.Errorf("%w; la punta de %s (%s) no es el commit que creo este guardado: no la muevo", err, s.nombre(), short(punta))
	}
	if out, uerr := g.correr(dir, nil, "update-ref", "--no-deref", "-m", "hoom: deshace un guardado que fallo", s.rama, s.antes, punta); uerr != nil {
		return fmt.Errorf("%w; y no pude devolver %s de %s a %s: %s", err, s.nombre(), short(punta), short(s.antes), motivoGit(out, uerr))
	}
	return fmt.Errorf("%w; %s vuelve a %s", err, s.nombre(), short(s.antes))
}

// motivoGit is why a git of the save failed: its bound, or what it said.
func motivoGit(out string, err error) string {
	if errors.Is(err, errTopeGit) || out == "" {
		return err.Error()
	}
	return out
}

// sinGuardar is what the task tree dir has unsaved, sorted, as the card's
// Unsynced lists it (gitx.Rutas of git status: every uncommitted path,
// untracked directories expanded, and the origin of a rename), from a git
// status of the save's own: bounded and in its group, like its other git
// commands. It takes no optional lock (--no-optional-locks):
// reading what there is to save writes nothing.
func sinGuardar(dir string, g gitDelGuardado) ([]string, error) {
	out, err := g.listado(dir, "--no-optional-locks", "-c", "core.quotePath=false",
		"status", "--porcelain", "--untracked-files=all", "-z")
	if err != nil {
		return nil, err
	}
	paths := gitx.Rutas(gitx.LeerCambios(out))
	sort.Strings(paths)
	return paths, nil
}

// tareaDe is the task whose worktree root is (<principal>/.hoom/worktrees/
// <tarea>) and its main checkout, whatever card is being saved; ok is false
// when root is not a task's worktree.
func tareaDe(root, slug string) (principal, tarea string, ok bool) {
	p, ok := gitx.CheckoutPrincipal(root)
	if !ok || mismoArbol(p, root) {
		return "", "", false
	}
	nombres := []string{slug, filepath.Base(root)}
	if r, err := filepath.EvalSymlinks(root); err == nil {
		nombres = append(nombres, filepath.Base(r))
	}
	for _, x := range nombres {
		if item.ValidSlug(x) && mismoArbol(root, filepath.Join(p, ".hoom", "worktrees", x)) {
			return p, x, true
		}
	}
	return "", "", false
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

// gitDelGuardado is how commitPaths runs its git commands: hoom item save's
// (the zero value: unbounded) or SaveTarea's (delGuardado). In both the
// repository's hooks run.
type gitDelGuardado struct {
	tope     time.Duration // > 0 bounds each command (gitTope); 0 = unbounded
	opciones []string      // git options before the subcommand
	env      []string      // added to the environment; nil = inherited as is
}

// delGuardado is the git of SaveTarea: each command bounded by TopeGit in
// its group, the repository's hooks and filters included (aclaraciones 19 y
// 23). It runs no fsmonitor nor any automatic gc or maintenance, which
// check nothing: a gc that detaches would leave a process of the command
// alive once it returned (aclaracion 19). It reads the repository's own
// objects (gitx.SinReemplazos): a refs/replace or a grafts file must not
// make a staged change look already saved (finding 1d9e67). It logs every
// update of the branch in its reflog (core.logAllRefUpdates), where the save
// reads the commit its git commit created (guardado.suCommit, aclaracion
// 27).
func delGuardado() gitDelGuardado {
	return gitDelGuardado{tope: TopeGit, env: gitx.SinReemplazos(), opciones: []string{
		"-c", "core.fsmonitor=false",
		"-c", "gc.auto=0",
		"-c", "maintenance.auto=false",
		"-c", "core.logAllRefUpdates=true",
	}}
}

func (g gitDelGuardado) con(args []string) []string {
	return append(append([]string{}, g.opciones...), args...)
}

// correr is gitTope with g's bound and options.
func (g gitDelGuardado) correr(dir string, stdin io.Reader, args ...string) (string, error) {
	return gitTope(dir, g.tope, g.env, stdin, g.con(args)...)
}

// listado is listadoGit with g's bound and options.
func (g gitDelGuardado) listado(dir string, args ...string) ([]byte, error) {
	return listadoGit(dir, g.tope, g.env, g.con(args)...)
}

// commitPaths stages exactly paths and commits only them: whatever else sits
// in the index stays there, uncommitted. The repository's hooks run.
// antesDelCommit (nil = none) runs once the paths are staged and before git
// commit: a failure there stops the save before the commit.
func commitPaths(dir, msg string, paths []string, g gitDelGuardado, antesDelCommit func() error) (string, error) {
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
	fallo := func(que, out string, err error) error {
		if errors.Is(err, errTopeGit) {
			return err
		}
		return fmt.Errorf("git %s fallo: %s", que, out)
	}
	if len(presentes) > 0 {
		if out, err := g.correr(dir, nil, append([]string{"--literal-pathspecs", "add", "-A", "--"}, presentes...)...); err != nil {
			return "", fallo("add", out, err)
		}
	}
	if len(ausentes) > 0 {
		in := strings.NewReader(strings.Join(ausentes, "\x00") + "\x00")
		if out, err := g.correr(dir, in, "update-index", "--remove", "-z", "--stdin"); err != nil {
			return "", fallo("update-index", out, err)
		}
	}
	if antesDelCommit != nil {
		if err := antesDelCommit(); err != nil {
			return "", err
		}
	}
	commit := append([]string{"--literal-pathspecs", "commit", "-q", "-m", msg, "--"}, paths...)
	if out, err := g.correr(dir, nil, commit...); err != nil {
		return "", fallo("commit", out, err)
	}
	sha, err := g.correr(dir, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", fallo("rev-parse", sha, err)
	}
	return sha, nil
}

// errTopeGit marks a git command of the save that did not end within its
// bound (TopeGit).
var errTopeGit = errors.New("no termino a tiempo")

// graciaGit is how long the group of a save's git command that passed
// TopeGit has between its SIGTERM (git drops its index.lock) and the SIGKILL
// of whatever is left: a hook or filter that ignores the signal, or its
// children.
const graciaGit = 500 * time.Millisecond

// finDeLecturaGit bounds the reading of a save's git command output once its
// group is dead: something that left the group and still holds the output
// does not keep the save waiting.
const finDeLecturaGit = time.Second

// colaGit is how much of a save's git command output gitTope keeps: its
// last 64 KiB.
const colaGit = 64 << 10

// gitTope runs git in dir with stdin (nil = none), its combined output
// trimmed. tope 0 runs it as always (hoom item save). With tope > 0 it runs
// as correrConTope says, and of a long output it keeps the end, the last
// colaGit bytes: where git, or what it ran, says why it failed (finding
// d49489).
func gitTope(dir string, tope time.Duration, env []string, stdin io.Reader, args ...string) (string, error) {
	if tope <= 0 {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = entornoGit(env)
		cmd.Stdin = stdin
		out, err := cmd.CombinedOutput()
		return strings.TrimSpace(string(out)), err
	}
	salida := &salidaGit{limite: colaGit}
	err := correrConTope(dir, tope, env, stdin, salida, salida, args...)
	return strings.TrimSpace(string(salida.bytes())), err
}

// listadoGit is git's stdout, whole and untrimmed: a listing the save reads
// (what there is to save), run like gitTope. When git fails the error says
// which command and carries the end of its stderr.
func listadoGit(dir string, tope time.Duration, env []string, args ...string) ([]byte, error) {
	out, stderr := &salidaGit{}, &salidaGit{limite: colaGit}
	var err error
	if tope <= 0 {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = entornoGit(env)
		cmd.Stdout, cmd.Stderr = out, stderr
		err = cmd.Run()
	} else {
		err = correrConTope(dir, tope, env, nil, out, stderr, args...)
	}
	if errors.Is(err, errTopeGit) {
		return nil, err
	}
	if err != nil {
		msg := strings.TrimSpace(string(stderr.bytes()))
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("%s fallo: %s", subcomandoGit(args), msg)
	}
	return out.bytes(), nil
}

// correrConTope runs git in dir in a process group of its own
// (procesos.ArrancarConGracia), stdout to stdout and stderr to stderr through
// pipes of our own: Wait returns when git ends even if something it ran
// still holds them. When stdout and stderr are the same writer (gitTope)
// both go through ONE pipe, so the output keeps the order the kernel gave
// it: with two pipes read by two goroutines the order of what both wrote
// would depend on the scheduler (finding 7b1645). Once tope passes, the whole group, whatever git ran and
// its children included, gets SIGTERM and, graciaGit later, SIGKILL, and
// nothing of the command is alive when it returns (procesos.Liberar,
// aclaracion 19). The error then wraps errTopeGit and says which command and
// its bound.
func correrConTope(dir string, tope time.Duration, env []string, stdin io.Reader, stdout, stderr io.Writer, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), tope)
	defer cancel()
	var lecturas, escrituras []*os.File
	var leidos []chan struct{}
	defer func() {
		for _, f := range lecturas {
			f.Close()
		}
	}()
	salidas := []io.Writer{stdout, stderr}
	if stdout == stderr {
		salidas = salidas[:1]
	}
	for _, w := range salidas {
		pr, pw, err := os.Pipe()
		if err != nil {
			for _, f := range escrituras {
				f.Close()
			}
			return err
		}
		lecturas, escrituras = append(lecturas, pr), append(escrituras, pw)
		leido := make(chan struct{})
		leidos = append(leidos, leido)
		go func() {
			defer close(leido)
			_, _ = io.Copy(w, pr)
		}()
	}
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = entornoGit(env)
	cmd.Stdin = stdin
	cmd.Stdout, cmd.Stderr = escrituras[0], escrituras[len(escrituras)-1]
	cmd.WaitDelay = graciaGit + time.Second
	err := procesos.ArrancarConGracia(cmd, graciaGit)
	for _, f := range escrituras {
		f.Close() // git keeps its copies: EOF comes when the group's are closed
	}
	if err != nil {
		return err // the deferred Close ends the reading
	}
	err = cmd.Wait()
	vencio := errors.Is(ctx.Err(), context.DeadlineExceeded)
	if vencio {
		procesos.EsperarGrupo(cmd.Process.Pid, time.Now().Add(graciaGit))
	}
	procesos.Liberar(cmd.Process.Pid)
	leyeron := func(hasta <-chan time.Time) bool {
		for _, l := range leidos {
			select {
			case <-l:
			case <-hasta:
				return false
			}
		}
		return true
	}
	if !leyeron(time.After(finDeLecturaGit)) {
		// someone outside the group holds the output: stop reading. On
		// Windows a pending read may outlive its Close, so this wait is
		// bounded too
		for _, f := range lecturas {
			f.Close()
		}
		leyeron(time.After(finDeLecturaGit))
	}
	if vencio {
		return fmt.Errorf("%s %w: se corto a los %s, con lo que corria", subcomandoGit(args), errTopeGit, tope)
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		// git ended well; its stdin took longer to drop
		err = nil
	}
	return err
}

// entornoGit is the environment of a git of the save: the inherited one
// plus env, or nil (inherited as is) without env.
func entornoGit(env []string) []string {
	if len(env) == 0 {
		return nil
	}
	return append(os.Environ(), env...)
}

// subcomandoGit is "git <subcommand>" of args, past git's own options and
// the value of each -c and -C.
func subcomandoGit(args []string) string {
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "-c" || a == "-C":
			i++
		case !strings.HasPrefix(a, "-"):
			return "git " + a
		}
	}
	return "git"
}

// salidaGit keeps what a git command of the save writes, safe for the
// readers of its two pipes: everything with limite 0, otherwise its last
// limite bytes. To copy less it lets its buffer grow up to 2*limite before
// dropping the oldest part, like the gate runner's colaAcotada.
type salidaGit struct {
	mu     sync.Mutex
	limite int
	buf    []byte
}

func (s *salidaGit) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	if s.limite > 0 && len(s.buf) > 2*s.limite {
		s.buf = append(s.buf[:0], s.buf[len(s.buf)-s.limite:]...)
	}
	return len(p), nil
}

// bytes is a copy of what it kept: the last limite bytes, or everything.
func (s *salidaGit) bytes() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	b := s.buf
	if s.limite > 0 && len(b) > s.limite {
		b = b[len(b)-s.limite:]
	}
	return append([]byte(nil), b...)
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

// mismoArbol says whether a and b name the same tree, symlinks resolved.
func mismoArbol(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}
