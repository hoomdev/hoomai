package finding

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
	return Contexto{}, nil
}

// Juzgar fills Bloquea and Motivo of every item of items with Bloquea under
// c: an open one gets Bloquea(it.Finding, c), a closed one false and "". It
// returns items, judged in place.
func Juzgar(items []Item, c Contexto) []Item {
	return items
}

// ListConUmbral is List with every item judged (Juzgar) under
// ContextoDe(root, root, blockOn): what `hoom finding list` shows, with
// `bloquea: <motivo>` or `pendiente` on each open finding. The warnings are
// List's, the ones about raises that raise nothing included.
func ListConUmbral(root, base, blockOn string, openOnly bool) ([]Item, []string, error) {
	return nil, nil, nil
}

// JSONBytesConUmbral renders what `hoom finding list --json` and GET
// /api/findings emit: ListView of ListConUmbral, so every finding carries
// bloquea and motivo (false and "" on a closed one), and the warnings
// (never null) carry the ones about raises.
func JSONBytesConUmbral(root, base, blockOn string, openOnly bool) ([]byte, error) {
	return nil, nil
}
