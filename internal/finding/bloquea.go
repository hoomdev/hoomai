package finding

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/riesgo"
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
	// Riesgos is the riesgo the item of each task declares
	// (riesgo.Vocabulario), by task slug. A task that is not there, an empty
	// value and a finding without task are judged as riesgo.Normal.
	Riesgos map[string]string
	// Completas are the review records with cobertura completa, oldest
	// first; a record with any other cobertura (delta, parcial, none) is
	// never one of them.
	Completas []RevisionCompleta
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
//  3. it is medium and its file is a risk path (riesgo.Ruta of
//     []string{f.File} not empty; a medium without file never is):
//     `medium en ruta de riesgo: <file>`;
//  4. it is medium, the riesgo of its task in c.Riesgos is alto, and its id
//     is in the Findings of a RevisionCompleta of c.Completas whose Task is
//     f.Task: `medium de la review completa <id>: riesgo alto`, where <id>
//     is the first such record in c.Completas.
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
	if file := strings.TrimSpace(f.File); file != "" && riesgo.Ruta([]string{file}) != "" {
		return true, "medium en ruta de riesgo: " + seguro(file)
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
// it cannot read is left out) and the riesgo of each valid item of
// proyecto's .hoom/items/ (item.List). The findings and the review records
// of a task live in its tree; its item, in the project. For one tree, arbol
// and proyecto are the same directory. Only a .hoom/findings/ that exists
// and cannot be read is an error.
func ContextoDe(arbol, proyecto, blockOn string) (Contexto, error) {
	sc, err := scan(arbol)
	if err != nil {
		return Contexto{BlockOn: blockOn}, err
	}
	return contextoDesde(sc, arbol, proyecto, blockOn), nil
}

// contextoDesde completes, around an existing scan of arbol, the Contexto
// ContextoDe gives.
func contextoDesde(sc scanned, arbol, proyecto, blockOn string) Contexto {
	c := Contexto{BlockOn: blockOn, Subidas: sc.subidas, Riesgos: Riesgos(proyecto),
		Completas: RevisionesCompletas(arbol)}
	if c.Subidas == nil {
		c.Subidas = map[string]Subida{}
	}
	return c
}

// Riesgos is the riesgo each valid item of root's .hoom/items/ declares, by
// slug: Contexto.Riesgos as ContextoDe reads it. No items is an empty map.
func Riesgos(root string) map[string]string {
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
