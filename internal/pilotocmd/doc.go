// Package pilotocmd runs the pilot of a card (spec autonomia-entre-firmas):
// the ONE loop the Studio and `hoom piloto <slug> [--json]` share. The model
// does; hoom decides whether to go on by looking at the disk; signing the
// spec and accepting the card are a person's.
//
// Whoever runs the pilot repeats the same turn (vuelta) until a decision
// says detenerse:
//
//  1. save: at the start of every turn, when the card's item has
//     `auto: hasta-humano` and the card has its task open, it commits what
//     the task's worktree has unsaved, on its branch hoom/<slug>, with the
//     message `hoom: guardar la tarjeta <slug>` (itemcmd.SaveTarea). It
//     never commits in the main checkout (an item changed with `hoom item
//     set` stays for `hoom item save`) and never pushes; with nothing to save
//     it commits nothing. If it cannot save (a job running, the worktree off
//     its branch) it stops: `no pudo guardar el trabajo de la tarjeta: <error>`;
//  2. it builds the card's boardcmd.Estado from the disk (Armar);
//  3. it asks boardcmd.Piloto, which is pure;
//  4. it carries out the step: lanzar launches through lanzar.Lanzar, the
//     same function as the Studio's POST /launch and `hoom pedir`, with
//     Pilot and the decision's Regla, Intento, Ronda, Hallazgos,
//     Bloqueantes, Resume and Delta; cerrar-hallazgos closes each finding of
//     the decision as corregido with author `hoom piloto`
//     (finding.AutorPiloto) and its evidence (PilotDecision.Evidencias),
//     outside a run, then runs the card's verify as `hoom verify --spec`
//     does and saves the workspace (it is left with nothing to commit);
//     when the new verdict is not green it then stops with
//     `quedan hallazgos sin test en la ronda <n>: <ids>: te toca` if blocking
//     findings of the card are left that no protected test of the round
//     cites, and with
//     `despues de cerrar los hallazgos de la ronda <n>, el veredicto <id> sigue rojo en <gates>: te toca`
//     if none is (for instance, red by a finding without a task);
//     detenerse stops.
//
// The pilot's lines go to the log of the card's newest closed envelope
// (.hoom/envelopes/<id>.log) and to the output of `hoom piloto`, one per
// line:
//
//   - the three of the Studio's belt, unchanged:
//     `piloto automatico: se detiene: <why>`,
//     `piloto automatico: no pudo lanzar al <rol>: <error>` and
//     `piloto automatico: pide al <rol> con <provider> (<x> USD): sobre <id>`;
//   - with a provider without a USD cap:
//     `piloto automatico: pide al <rol> con <provider> (sin tope en USD): sobre <id>`;
//   - before every launch: `piloto automatico: regla <regla>`, followed by
//     ` intento <n>` or ` ronda <n>` when the decision has them;
//   - after a save that committed: `piloto automatico: guarda el trabajo de la tarjeta: <commit>`;
//   - for cerrar-hallazgos: `piloto automatico: cierra <n> hallazgos con su test (ronda <r>): <ids>`.
//
// In the Studio the turn runs when a job the Studio launched ends (the belt,
// as before) and when the spec is approved from the card (approveCard: the
// answer of POST /api/specs/{name}/approve with {"card": ...} gains
// `piloto`, the line of the first decision, only when the card has
// `auto: hasta-humano`); Avanzar is that skin. In the terminal, `hoom
// piloto` (Run, Correr) runs in the foreground, waits for every job it
// launches and prints one line per decision; it ends with exit 0 when it
// stopped in a human column or in Hecho, and with exit 1 on any other stop.
// While it runs it holds the lock Candado(root, slug)
// (.hoom/cache/locks/piloto-<slug>.lock); when another process holds it,
// it exits 1 with `el piloto de <slug> ya corre en otro proceso`.
//
// `hoom aprobar <slug>` and `hoom spec approve <ruta>` launch nothing: when
// the card of that spec has `auto: hasta-humano` they add the line
// AvisoDeFirma gives.
package pilotocmd
