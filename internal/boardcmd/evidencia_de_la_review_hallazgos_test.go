// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md, ronda
// 3, en el tablero (los hallazgos de la review de la correccion, escritos
// desde el spec):
//
//   - 791394 (CA-483, CA-485): "CambioDespues gana la base": lo que cambio
//     despues de una review depende de I, el merge-base de la base con HEAD.
//     El tablero se deriva una y otra vez en el mismo proceso (el Studio):
//     si la base se mueve y cambia I, el tablero siguiente refleja la base
//     nueva, no la cola que calculo con la de antes.
package boardcmd

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

// edrBuildCard es la tarjeta de la tarea en un Build del proceso del test.
func edrBuildCard(t *testing.T, ca string, tj *rdTarjeta) Card {
	t.Helper()
	b, err := Build(tj.root, "main", "high", time.Now().UTC())
	if err != nil {
		t.Fatalf("%s: Build: %v", ca, err)
	}
	return bdCard(t, b, bdSlug)
}

// edrCardDelBinario es la tarjeta de la tarea en `hoom board --json`.
func edrCardDelBinario(t *testing.T, ca string, r bxSalida) Card {
	t.Helper()
	if r.exit != 0 {
		t.Fatalf("%s: hoom board --json sale con 0: %d\n%s\n%s", ca, r.exit, r.stdout, r.stderr)
	}
	var b bxTablero
	if err := json.Unmarshal([]byte(r.stdout), &b); err != nil {
		t.Fatalf("%s: hoom board --json es JSON: %v\n%s", ca, err, r.stdout)
	}
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if c.Slug == bdSlug {
				return c
			}
		}
	}
	t.Fatalf("%s: el tablero tiene la tarjeta %s:\n%s", ca, bdSlug, r.stdout)
	return Card{}
}

// edrMismaTarjeta exige que dos tarjetas digan lo mismo: columna, motivos,
// siguiente paso y la review que cuenta.
func edrMismaTarjeta(t *testing.T, ca, caso string, got, want Card) {
	t.Helper()
	if got.Column != want.Column || !reflect.DeepEqual(got.Missing, want.Missing) || got.Next != want.Next ||
		got.Evidence.ReviewID != want.Evidence.ReviewID {
		t.Fatalf("%s: %s: la tarjeta del tablero en el proceso es la de un tablero nuevo:\n  en el proceso: %s %q next=%q review=%q\n  tablero nuevo: %s %q next=%q review=%q",
			ca, caso, got.Column, got.Missing, got.Next, got.Evidence.ReviewID, want.Column, want.Missing, want.Next, want.Evidence.ReviewID)
	}
}

// Hallazgo 791394 (CA-483, CA-485): la tarea tiene su review R1 (hasta D) y
// despues integra la rama lib (un commit L de codigo que main todavia no
// tiene). Con main en su commit de antes, L es codigo de la tarea despues de
// R1; con main movida a L (main integro lib), L es lo que trajo la base: el
// merge-base con HEAD pasa de M0 a L, y solo integrarla no cuenta (CA-485).
// El verde mas nuevo es el de main en L. En el mismo proceso: un Build con
// main en M0 (calcula y guarda la cola de R1), main avanza a L, otro Build:
// su tarjeta es la de un tablero recien armado (`hoom board --json`, otro
// proceso), que dice Tu aceptacion con R1; nunca la de la cola guardada con
// la base de antes. Guardas: las dos respuestas difieren de verdad (el
// tablero nuevo con main en M0 no es el de main en L).
func TestHallazgo791394_CA483_CA485_ElTableroVeLaBaseNuevaEnElMismoProceso(t *testing.T) {
	const ca = "791394/CA-485"
	bin := bxBinario(t)
	tj := rdGrande(t)
	m0 := bdGit(t, tj.root, "rev-parse", "main")
	r1 := tj.completa(t, "20261028T160000_c0mp1a")

	bdGit(t, tj.root, "checkout", "-q", "-b", "lib")
	bdEscribir(t, tj.root, "lib.go", "package app\n\n// MARCA-LIB\nfunc Lib() {}\n")
	bdGit(t, tj.root, "add", "lib.go")
	bdGit(t, tj.root, "commit", "-q", "-m", "L: codigo de lib")
	bdGit(t, tj.root, "checkout", "-q", "main")
	bdGit(t, tj.wt, "merge", "-q", "--no-ff", "--no-edit", "lib")
	// el verde mas nuevo es el de main en L
	bdGit(t, tj.root, "merge", "-q", "--ff-only", "lib")
	tj.verde(t)
	bdGit(t, tj.root, "reset", "-q", "--hard", m0)

	// main en M0: el tablero en el proceso (guarda la cola) y uno nuevo
	enM0 := edrBuildCard(t, ca, tj)
	nuevoEnM0 := edrCardDelBinario(t, ca, bxCorrer(t, bin, tj.root, "board", "--json"))
	edrMismaTarjeta(t, ca, "main en M0 (fixture)", enM0, nuevoEnM0)

	// main avanza a L
	bdGit(t, tj.root, "merge", "-q", "--ff-only", "lib")
	nuevoEnL := edrCardDelBinario(t, ca, bxCorrer(t, bin, tj.root, "board", "--json"))
	rdCuenta(t, ca+" (fixture)", "tablero nuevo con main en L", nuevoEnL, r1.ID)
	if nuevoEnM0.Column == nuevoEnL.Column && reflect.DeepEqual(nuevoEnM0.Missing, nuevoEnL.Missing) {
		t.Fatalf("%s: fixture: con main en M0 y en L el tablero dice cosas distintas: %s %q", ca, nuevoEnL.Column, nuevoEnL.Missing)
	}
	edrMismaTarjeta(t, ca, "main en L, segundo Build del proceso", edrBuildCard(t, ca, tj), nuevoEnL)
}

// Hallazgo 4acfc8 (CA-483, ronda 4): la cola del tablero sale de los objetos
// reales. Despues de la review de la tarjeta (hasta D) la tarea commitea
// codigo (y su verde); un refs/replace/<D> plantado hace que git lea D con el
// arbol de HEAD, y el diff de D a HEAD que da git queda vacio. El tablero
// igual ve el codigo nuevo: la tarjeta vuelve a Review con el motivo y el
// siguiente paso del delta. (Sin derivar la tarjeta despues del commit y
// antes del reemplazo: el tablero guarda las colas, CA-427.)
func TestHallazgo4acfc8_CA483_UnReplaceNoEscondeElCodigoDelTablero(t *testing.T) {
	tj := rdGrande(t)
	r1 := tj.completa(t, "20261028T160000_c0mp1a")
	rdCuenta(t, "4acfc8 (fixture)", "completa", tj.card(t), r1.ID)
	tj.codigo(t, "Escondido")
	head := tj.head(t)
	falso := bdGit(t, tj.wt, "commit-tree", head+"^{tree}", "-p", r1.Hasta+"^", "-m", "commit falso que reemplaza a D")
	bdGit(t, tj.wt, "replace", "-f", r1.Hasta, falso)
	if out := bdGit(t, tj.wt, "diff", "--stat", r1.Hasta, head); out != "" {
		t.Fatalf("4acfc8: fixture: con el reemplazo, git no ve cambios de D a HEAD: %q", out)
	}
	edrPideElDelta(t, "4acfc8/CA-483", "codigo escondido por refs/replace", tj.card(t), r1.ID, r1.Hasta)
}
