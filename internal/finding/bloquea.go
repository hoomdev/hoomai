package finding

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Contexto is everything besides the finding itself that decides whether an
// open finding blocks (Bloquea).
type Contexto struct {
	// BlockOn is the threshold, findings.block_on: low, medium or high. ""
	// judges with high, as the board does when the project did not declare
	// it.
	BlockOn string
	// Subidas are the VALID raises (see Subida), by finding id: an entry
	// raises the finding of its key.
	Subidas map[string]Subida
	// Riesgos is the EFFECTIVE riesgo of each task (riesgo.Vocabulario), by
	// task slug (riesgo-efectivo-y-tamano): the riesgo of its card
	// (RiesgoDeTarjeta), or, in `hoom verify`, for the verify's own task, the
	// one that verify computed (GateDelVerify). A task that is not there, an
	// empty value and a finding without task are judged as riesgo.Normal.
	Riesgos map[string]string
	// Ilegibles are the tasks whose item is there and cannot be read, with
	// the reason (item.RiesgosDe; "*" = no item of the project can be
	// listed): their riesgo is not known, so a medium of theirs blocks, and
	// the gate is ERROR for any open finding of theirs.
	Ilegibles map[string]string
	// Completas are the review records with cobertura completa, oldest
	// first; a record with any other cobertura (delta, parcial, none) is
	// never one of them.
	Completas []RevisionCompleta
	// Rutas are the project's resolved rutas (riesgo-efectivo-y-tamano):
	// case 3 of Bloquea matches a finding's file against Rutas.DeRiesgo.
	// nil = riesgo.RutasBase(), the rutas of a project without the section
	// `rutas` (no profile adds to de_riesgo).
	Rutas *riesgo.Rutas
}

// RevisionCompleta is one review record with cobertura completa as Bloquea
// reads it: its id, its task and the ids of its `findings` field.
type RevisionCompleta struct {
	ID       string
	Task     string
	Findings []string
}

// Bloquea is the one definition of whether a finding blocks: the
// findings_open gate, the card (live and historic, and with it `hoom item
// backfill`), `hoom status`, `hoom finding list` and the pendientes all ask
// it. f is judged as an OPEN finding: whoever asks never asks for a closed
// one.
//
// A severity outside low|medium|high blocks, failing closed, with the motivo
// `severidad desconocida "<sev>"` (the severity as the finding writes it,
// quoted as %q quotes it). Otherwise f blocks by the first case that holds:
//
//  1. its severity reaches c.BlockOn (Blocks; "" is high):
//     `severidad <sev>`;
//  2. c.Subidas has a raise of f.ID: `subido a mano por <autor>` (the
//     raise's author);
//  3. it is medium and its file matches the rutas de_riesgo of c
//     (riesgo.CoincideAlguno over c.Rutas.DeRiesgo, riesgo.RutasBase when
//     c.Rutas is nil; a medium without file never does):
//     `medium en ruta de riesgo: <file>`;
//  4. it is medium, the EFFECTIVE riesgo of its task in c.Riesgos is alto
//     (also when the card declares normal), and its id is in the Findings
//     of a RevisionCompleta of c.Completas whose Task is f.Task:
//     `medium de la review completa <id>: riesgo alto`, where <id> is the
//     first such record in c.Completas.
//
// The text of each motivo does not change with riesgo-efectivo-y-tamano.
//
// When none holds it does not block: (false, "").
func Bloquea(f Finding, c Contexto) (bloquea bool, motivo string) {
	sev := strings.ToLower(strings.TrimSpace(f.Severity))
	if _, ok := severityRank[sev]; !ok {
		return true, fmt.Sprintf("severidad desconocida %q", f.Severity)
	}
	umbral := c.BlockOn
	if strings.TrimSpace(umbral) == "" {
		umbral = "high"
	}
	if Blocks(sev, umbral) {
		return true, "severidad " + sev
	}
	if s, ok := c.Subidas[f.ID]; ok {
		return true, "subido a mano por " + seguro(s.Author)
	}
	if sev != "medium" {
		return false, ""
	}
	if file := strings.TrimSpace(f.File); file != "" && riesgo.CoincideAlguno(c.deRiesgo(), file) {
		return true, "medium en ruta de riesgo: " + seguro(file)
	}
	if motivo, ok := c.riesgoIlegible(f.Task); ok {
		return true, motivo
	}
	if f.Task != "" && c.Riesgos[f.Task] == riesgo.Alto {
		for _, rc := range c.Completas {
			if rc.Task != f.Task {
				continue
			}
			for _, id := range rc.Findings {
				if id == f.ID {
					return true, fmt.Sprintf("medium de la review completa %s: riesgo alto", seguro(rc.ID))
				}
			}
		}
	}
	return false, ""
}

// ContextoDe is the Contexto the trees hold, with threshold blockOn: the
// valid raises of arbol's .hoom/findings/, the completa review records of
// arbol's .hoom/reviews/ (the format reviewcmd.WriteRecord writes; a record
// it cannot read is left out), the rutas of proyecto's hoom.yaml
// (profiles.Clasificador; riesgo.RutasBase without one) and the riesgo of
// the card of each valid item of proyecto's .hoom/items/ (item.List):
// RiesgoDeTarjeta of the task's newest complete verdict in arbol (spec
// .hoom/specs/<slug>.md), its card and proyecto's classifier
// (riesgo-efectivo-y-tamano). The findings, the review records and the
// verdicts of a task live in its tree; its item, in the project. For one
// tree, arbol and proyecto are the same directory. Only a .hoom/findings/
// that exists and cannot be read, and a hoom.yaml of proyecto that
// profiles.ClasificadorDe cannot read (its rutas do not load, its profile
// is unknown), are errors.
func ContextoDe(arbol, proyecto, blockOn string) (Contexto, error) {
	sc, err := scan(arbol)
	if err != nil {
		return Contexto{BlockOn: blockOn}, err
	}
	cl, err := ClasificadorDelProyecto(proyecto)
	if err != nil {
		return Contexto{BlockOn: blockOn}, err
	}
	c := contextoDesde(sc, arbol, proyecto, blockOn, cl)
	for _, it := range sc.items {
		if motivo, ok := c.riesgoIlegible(it.Task); ok && it.Status == StatusOpen {
			return c, fmt.Errorf("hallazgo %s: %s", it.ID, motivo)
		}
	}
	return c, nil
}

// RiesgoDeTarjeta is the riesgo of a card (spec riesgo-efectivo-y-tamano,
// «El riesgo de una tarjeta»), with t as the card is read now and v its
// newest complete verdict (nil = none):
//
//   - a card of a task that cannot be read (t.Ilegible) is rule 1 of
//     riesgo.Clasificador.Evaluar, alto, with or without a verdict: it
//     fails closed;
//   - without a verdict, its declared riesgo, with the motivo
//     `declarado <d>` (riesgo.MotivoDeclarado);
//   - with a verdict, the higher (riesgo.Mayor) of the verdict's
//     riesgo.efectivo and t's declared riesgo now: the verdict's motivo,
//     or `declarado <d>` when the declared one is strictly higher. A verdict
//     older than riesgo-efectivo-y-tamano (no Riesgo) is measured with the
//     rules (c.Evaluar) over t and its Git.ChangedFiles.
//
// Declarado is t's (riesgo.Normal when it says nothing). So raising the
// declared riesgo shows at once, and lowering it waits for the next verify:
// the result is never lower than what the newest verify measured. The board
// (CardEvidence.Riesgo and RiesgoMotivo), ContextoDe and `hoom status` read
// a card's riesgo here.
func RiesgoDeTarjeta(v *verdict.Verdict, t riesgo.Tarjeta, c riesgo.Clasificador) riesgo.Evaluacion {
	// the declared riesgo now, as the rule reads it (an unreadable card
	// declares nothing: normal)
	ahora := c.Evaluar(t, nil)
	decl := ahora.Declarado
	if t.Tarea != "" && t.Ilegible {
		// the card cannot be read now: no declared riesgo is known, and it
		// fails closed as rule 1 does, with or without a verdict
		return ahora
	}
	if v == nil {
		return riesgo.Evaluacion{Declarado: decl, Efectivo: decl, Motivo: fmt.Sprintf(riesgo.MotivoDeclarado, decl)}
	}
	medido := c.Evaluar(t, v.Git.ChangedFiles)
	if v.Riesgo != nil {
		medido = *v.Riesgo
	}
	out := riesgo.Evaluacion{Declarado: decl, Efectivo: riesgo.Mayor(medido.Efectivo, decl), Motivo: medido.Motivo}
	// the declared one is strictly higher than what the verdict measured (or
	// the verdict says no motivo): the motivo is the declared one's
	if out.Efectivo != riesgo.Mayor(medido.Efectivo, medido.Efectivo) || strings.TrimSpace(out.Motivo) == "" {
		out.Motivo = fmt.Sprintf(riesgo.MotivoDeclarado, out.Efectivo)
	}
	return out
}

// ClasificadorDelProyecto is the classifier of the hoom.yaml of the project
// root proyecto (profiles.ClasificadorDe): its rutas and the test-writer's
// territory; profiles.Clasificador(nil) when there is no hoom.yaml. A
// hoom.yaml that cannot be read, or whose rutas, agents or profile do not
// load, is an error.
func ClasificadorDelProyecto(proyecto string) (riesgo.Clasificador, error) {
	raw, err := os.ReadFile(filepath.Join(proyecto, manifest.FileName))
	if errors.Is(err, os.ErrNotExist) {
		return profiles.Clasificador(nil)
	}
	if err != nil {
		return riesgo.Clasificador{}, err
	}
	return profiles.ClasificadorDe(raw)
}

// VeredictoDeTarea is the newest complete verdict of all (oldest first, as
// verdict.LoadAll gives them) bound to the spec of task slug,
// .hoom/specs/<slug>.md (an absolute spec is read relative to arbol); nil
// when there is none.
func VeredictoDeTarea(all []*verdict.Verdict, arbol, slug string) *verdict.Verdict {
	want := ".hoom/specs/" + slug + ".md"
	for i := len(all) - 1; i >= 0; i-- {
		v := all[i]
		if v == nil || v.IsPartial() {
			continue
		}
		spec := strings.TrimSpace(v.Spec)
		if spec == "" {
			continue
		}
		if filepath.IsAbs(spec) {
			rel, err := filepath.Rel(arbol, spec)
			if err != nil {
				continue
			}
			spec = rel
		}
		if filepath.ToSlash(filepath.Clean(spec)) == want {
			return v
		}
	}
	return nil
}

// deRiesgo are the patterns of case 3 of Bloquea: c.Rutas.DeRiesgo, or the
// defaults without rutas.
func (c Contexto) deRiesgo() []string {
	if c.Rutas == nil {
		return riesgo.RutasBase().DeRiesgo
	}
	return c.Rutas.DeRiesgo
}

// riesgoIlegible says whether the riesgo of task is unknown because its
// item cannot be read, and the motivo that says so.
func (c Contexto) riesgoIlegible(task string) (string, bool) {
	if task == "" {
		return "", false
	}
	why, ok := c.Ilegibles[task]
	if !ok {
		why, ok = c.Ilegibles["*"]
	}
	if !ok {
		return "", false
	}
	return fmt.Sprintf("el item de la tarea %s no se puede leer (%s): no se sabe su riesgo", seguro(task), seguro(why)), true
}

// contextoDesde completes, around an existing scan of arbol, the Contexto
// ContextoDe gives, with the classifier cl: its rutas for case 3 of
// Bloquea, and the riesgo of each card (RiesgoDeTarjeta of its newest
// complete verdict in arbol, measured with cl when it predates the spec).
func contextoDesde(sc scanned, arbol, proyecto, blockOn string, cl riesgo.Clasificador) Contexto {
	declarados, ilegibles := item.RiesgosDe(proyecto)
	riesgos := map[string]string{}
	if len(declarados) > 0 {
		vs, _, _ := verdict.LoadAllWithWarnings(arbol)
		for slug := range declarados {
			t := item.TarjetaDeRiesgo(proyecto, slug)
			riesgos[slug] = RiesgoDeTarjeta(VeredictoDeTarea(vs, arbol, slug), t, cl).Efectivo
		}
	}
	rutas := cl.Rutas
	c := Contexto{BlockOn: blockOn, Subidas: sc.subidas, Riesgos: riesgos, Ilegibles: ilegibles,
		Completas: RevisionesCompletas(arbol), Rutas: &rutas}
	if c.Subidas == nil {
		c.Subidas = map[string]Subida{}
	}
	return c
}

// Riesgos is the riesgo each item declares, by slug, and the items that
// cannot be read: Contexto.Riesgos and Contexto.Ilegibles as ContextoDe
// reads them (item.RiesgosDe).
func Riesgos(root string) (riesgos, ilegibles map[string]string) {
	return item.RiesgosDe(root)
}

// RevisionesCompletas are the review records of root's .hoom/reviews/ with
// cobertura completa, oldest first: Contexto.Completas as ContextoDe reads
// it. It reads the same records reviewcmd.Records trusts (a regular file
// whose id is its name and has the shape of a record id), and only the
// fields Bloquea needs; one it cannot read is left out.
func RevisionesCompletas(root string) []RevisionCompleta {
	type registro struct {
		ID        string    `json:"id"`
		CreatedAt time.Time `json:"created_at"`
		Task      string    `json:"task"`
		Findings  []string  `json:"findings"`
		Cobertura string    `json:"cobertura"`
	}
	var regs []registro
	_ = hoomfs.RecorrerDirectorio(root, []string{".hoom", "reviews"},
		func(n string) bool { return strings.HasSuffix(n, ".json") }, maxArchivo,
		func(e hoomfs.Entrada) {
			if e.Err != nil {
				return
			}
			var r registro
			if json.Unmarshal(e.Contenido, &r) != nil || r.ID != strings.TrimSuffix(e.Nombre, ".json") || !idDeRegistro(r.ID) {
				return
			}
			if r.Cobertura == "completa" {
				regs = append(regs, r)
			}
		})
	sort.SliceStable(regs, func(i, j int) bool { return regs[i].CreatedAt.Before(regs[j].CreatedAt) })
	out := []RevisionCompleta{}
	for _, r := range regs {
		out = append(out, RevisionCompleta{ID: r.ID, Task: r.Task, Findings: append([]string{}, r.Findings...)})
	}
	return out
}

// idDeRegistro is the shape reviewcmd gives a record id: 20060102T150405_
// plus 6 lowercase letters or digits (reviewcmd's validRecordID; a test of
// reviewcmd checks that the two readings agree).
func idDeRegistro(id string) bool {
	if len(id) != len("20060102T150405_")+6 || id[8] != 'T' || id[15] != '_' {
		return false
	}
	for i, c := range id {
		switch {
		case i == 8 || i == 15:
		case i < 15:
			if c < '0' || c > '9' {
				return false
			}
		default:
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'z') {
				return false
			}
		}
	}
	return true
}

// Juzgar fills Bloquea and Motivo of every item of items with Bloquea under
// c: an open one gets Bloquea(it.Finding, c), a closed one false and "". It
// returns items, judged in place.
func Juzgar(items []Item, c Contexto) []Item {
	for i := range items {
		items[i].Bloquea, items[i].Motivo = false, ""
		if items[i].Status == StatusOpen {
			items[i].Bloquea, items[i].Motivo = Bloquea(items[i].Finding, c)
		}
	}
	return items
}

// ListConUmbral is List with every item judged (Juzgar) under
// ContextoDe(root, root, blockOn): what `hoom finding list` shows, with
// `bloquea: <motivo>` or `pendiente` on each open finding. The warnings are
// List's, the ones about raises that raise nothing included.
func ListConUmbral(root, base, blockOn string, openOnly bool) ([]Item, []string, error) {
	items, warnings, err := List(root, base, openOnly)
	if err != nil {
		return items, warnings, err
	}
	c, err := ContextoDe(root, root, blockOn)
	if err != nil {
		return items, warnings, err
	}
	return Juzgar(items, c), warnings, nil
}

// JSONBytesConUmbral renders what `hoom finding list --json` and GET
// /api/findings emit: ListView of ListConUmbral, so every finding carries
// bloquea and motivo (false and "" on a closed one), and the warnings
// (never null) carry the ones about raises.
func JSONBytesConUmbral(root, base, blockOn string, openOnly bool) ([]byte, error) {
	items, warnings, err := ListConUmbral(root, base, blockOn, openOnly)
	if err != nil {
		return nil, err
	}
	if warnings == nil {
		warnings = []string{}
	}
	return json.MarshalIndent(ListView{Warnings: warnings, Findings: items}, "", "  ")
}

// CuentaPendientes is the count of the pendientes as their lines say it:
// "3 medium, 5 low", naming only the severities with a count above 0,
// medium first; "" when both are 0. The card's texto (boardcmd) and the line of
// `hoom task done` are built on it.
func CuentaPendientes(medium, low int) string {
	var partes []string
	if medium > 0 {
		partes = append(partes, fmt.Sprintf("%d medium", medium))
	}
	if low > 0 {
		partes = append(partes, fmt.Sprintf("%d low", low))
	}
	return strings.Join(partes, ", ")
}

// seguro is a value written by a person or a model (a raise's author, a
// finding's file, a record id) as it goes into a motivo, which the gate, the
// list and the card print on a terminal: as it is when it is plain text,
// quoted with its control and direction characters (ESC, U+202E) escaped
// when it is not. The board's textoSeguro, for the motivos (finding 04b283).
func seguro(v string) string {
	if strings.IndexFunc(v, func(r rune) bool {
		return unicode.IsControl(r) || unicode.Is(unicode.Bidi_Control, r)
	}) >= 0 {
		return strconv.Quote(v)
	}
	return v
}
