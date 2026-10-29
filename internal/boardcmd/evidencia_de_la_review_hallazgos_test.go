// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md, ronda
// 3, en el tablero (los hallazgos de la review de la correccion, escritos
// desde el spec):
//
//   - 791394 (CA-483, CA-485): "CambioDespues gana la base": lo que cambio
//     despues de una review depende de I, el merge-base de la base con HEAD.
//     El tablero se deriva una y otra vez en el mismo proceso (el Studio):
//     si la base se mueve y cambia I, el tablero siguiente refleja la base
//     nueva, no la cola que calculo con la de antes.
//   - 4acfc8 (CA-483, ronda 4): un refs/replace plantado no esconde codigo
//     de la cola del tablero; tampoco un reemplazo en otra base de refs que
//     hoom hereda en GIT_REPLACE_REF_BASE (ronda 7).
//   - 8d0272 (CA-483, CA-485, ronda 5): un .git/info/grafts plantado no
//     cambia la base integrada que la cola descuenta; tampoco un graft en un
//     archivo que hoom hereda en GIT_GRAFT_FILE (ronda 7).
//
// Las guardas de los fixtures corren con los git de siempre (bdGit): TestMain
// quita del entorno heredado las variables de git que cambian lo que el
// fixture planta o lee (edrGitHeredado; hallazgo 43f79f). Los helpers de
// entorno que la ronda 6 habia dejado aca ya no estan, y edrGitConEntorno,
// para las guardas que leen con una variable que hoom va a heredar, esta en
// evidencia_de_la_review_test.go (hallazgo cd162c).
package boardcmd

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
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
// codigo (y su verde); un reemplazo de D plantado hace que git lea D con el
// arbol de HEAD, y el diff de D a HEAD que da git queda vacio. El tablero
// igual ve el codigo nuevo: la tarjeta vuelve a Review con el motivo y el
// siguiente paso del delta. (Sin derivar la tarjeta despues del commit y
// antes del reemplazo: el tablero guarda las colas, CA-427.) El reemplazo
// esta en refs/replace/ o en otra base de refs que hoom hereda en
// GIT_REPLACE_REF_BASE (ronda 7): TestMain quito del entorno un
// GIT_REPLACE_REF_BASE heredado (hallazgo 43f79f), y el test se lo pone a
// hoom con t.Setenv despues de plantar y guardar.
func TestHallazgo4acfc8_CA483_UnReplaceNoEscondeElCodigoDelTablero(t *testing.T) {
	for _, c := range []struct{ nombre, base string }{
		{"refs-replace", ""},
		{"GIT_REPLACE_REF_BASE-heredado", "refs/prueba-externa/"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			ca := "4acfc8/CA-483 (" + c.nombre + ")"
			tj := rdGrande(t)
			r1 := tj.completa(t, "20261028T160000_c0mp1a")
			rdCuenta(t, "4acfc8 (fixture)", "completa", tj.card(t), r1.ID)
			tj.codigo(t, "Escondido")
			head := tj.head(t)
			falso := bdGit(t, tj.wt, "commit-tree", head+"^{tree}", "-p", r1.Hasta+"^", "-m", "commit falso que reemplaza a D")
			var entorno []string
			if c.base == "" {
				bdGit(t, tj.wt, "replace", "-f", r1.Hasta, falso)
			} else {
				bdGit(t, tj.wt, "update-ref", c.base+r1.Hasta, falso)
				entorno = []string{"GIT_REPLACE_REF_BASE=" + c.base}
				if out := bdGit(t, tj.wt, "diff", "--stat", r1.Hasta, head); out == "" {
					t.Fatalf("4acfc8: fixture: sin GIT_REPLACE_REF_BASE=%s, git ve los cambios de D a HEAD", c.base)
				}
			}
			if out := edrGitConEntorno(t, tj.wt, entorno, "diff", "--stat", r1.Hasta, head); out != "" {
				t.Fatalf("4acfc8: fixture: con el reemplazo, git no ve cambios de D a HEAD: %q", out)
			}
			if c.base != "" {
				t.Setenv("GIT_REPLACE_REF_BASE", c.base)
			}
			edrPideElDelta(t, ca, "codigo escondido por un reemplazo", tj.card(t), r1.ID, r1.Hasta)
		})
	}
}

// edrMergeBaseEn es `git merge-base main HEAD` en dir con extra agregado al
// entorno (edrGitConEntorno).
func edrMergeBaseEn(t *testing.T, dir string, extra ...string) string {
	t.Helper()
	return edrGitConEntorno(t, dir, extra, "merge-base", "main", "HEAD")
}

// Hallazgo 8d0272 (CA-483, CA-485, ronda 5): la cola del tablero descuenta
// la base que la rama integro de verdad. Despues de la review de la tarjeta
// (hasta D) la tarea commitea X (codigo nuevo), integra main en I (un merge
// limpio, H), y main sigue con J, que trae el mismo archivo de X. Un
// .git/info/grafts declara H con padres X y J: para git, el merge-base de
// main con HEAD pasa de I a J (las guardas lo comprueban; sin el graft es
// I), y con J descontada el codigo de X desapareceria. Sin refs/replace:
// falla solo por el graft (y el aviso de desuso de git va apagado, como lo
// apagaria quien lo planta). Con un verde para el HEAD (escrito con el graft
// ya plantado), la tarjeta no queda revisada: vuelve a Review con el motivo y
// el siguiente paso del delta. El graft esta en .git/info/grafts o en un
// archivo de afuera que hoom hereda en GIT_GRAFT_FILE (ronda 7): TestMain
// quito del entorno un GIT_GRAFT_FILE heredado (edrGitHeredado; hallazgos
// ff30ce y 43f79f), y el test se lo pone a hoom con t.Setenv despues de
// plantar y guardar. Con un git que ya no lee grafts el test se salta
// (hallazgo a21de0).
func TestHallazgo8d0272_CA485_UnGraftNoDejaLaTarjetaRevisada(t *testing.T) {
	for _, externo := range []bool{false, true} {
		nombre := "info-grafts"
		if externo {
			nombre = "GIT_GRAFT_FILE-heredado"
		}
		t.Run(nombre, func(t *testing.T) { edr8d0272Tablero(t, "8d0272/CA-485 ("+nombre+")", externo) })
	}
}

// edr8d0272Tablero es el caso de TestHallazgo8d0272_CA485 con el graft en
// .git/info/grafts o, si externo, en un archivo que hoom hereda en
// GIT_GRAFT_FILE.
func edr8d0272Tablero(t *testing.T, ca string, externo bool) {
	tj := rdGrande(t)
	r1 := tj.completa(t, "20261028T160000_c0mp1a")
	rdCuenta(t, ca+" (fixture)", "completa", tj.card(t), r1.ID)
	const x = "package app\n\n// MARCA-X-TAREA\nfunc XTarea() {}\n"
	bdEscribir(t, tj.wt, "x_tarea.go", x)
	bdCommitear(t, tj.wt, "X: codigo de la tarea")
	xSha := tj.head(t)
	i := edrMainAvanza(t, tj, "I", map[string]string{"y.go": "package app\n\nfunc Y() {}\n"})
	edrIntegrar(t, tj)
	h := tj.head(t)
	j := edrMainAvanza(t, tj, "J: main trae el mismo archivo de X", map[string]string{"x_tarea.go": x})
	if mb := edrMergeBaseEn(t, tj.wt); mb != i {
		t.Fatalf("%s: fixture: sin el graft, el merge-base de main con HEAD es I: %s", ca, mb)
	}
	// quien planta el graft tambien apaga el aviso de git ("info/grafts esta
	// en desuso", por stderr en cada comando): asi lo unico que cambia es la
	// historia que git cree, no la salida de los comandos
	bdGit(t, tj.wt, "config", "advice.graftFileDeprecated", "false")
	graft := h + " " + xSha + " " + j + "\n"
	var hostil []string
	if externo {
		archivo := bdEscribir(t, t.TempDir(), "grafts", graft)
		hostil = []string{"GIT_GRAFT_FILE=" + archivo}
	} else {
		comun := bdGit(t, tj.wt, "rev-parse", "--path-format=absolute", "--git-common-dir")
		bdEscribir(t, comun, filepath.Join("info", "grafts"), graft)
	}
	switch mb := edrMergeBaseEn(t, tj.wt, hostil...); mb {
	case j:
	case i:
		// hallazgo a21de0: git 3.0 (o uno compilado con
		// WITH_BREAKING_CHANGES) ya no lee info/grafts
		t.Skip("8d0272: el git instalado no lee grafts (el graft no cambia el merge-base): ese vector no existe con este git")
	default:
		t.Fatalf("%s: fixture: con el graft, git ve a J como merge-base de main con HEAD: %s", ca, mb)
	}
	if mb := edrMergeBaseEn(t, tj.wt, "GIT_GRAFT_FILE=/dev/null"); mb != i {
		t.Fatalf("%s: fixture: sin leer el graft, el merge-base sigue siendo I: %s", ca, mb)
	}
	for _, kv := range hostil {
		k, v, _ := strings.Cut(kv, "=")
		t.Setenv(k, v)
	}
	tj.verde(t)
	edrPideElDelta(t, ca, "codigo de la tarea que el graft le atribuye a la base", tj.card(t), r1.ID, r1.Hasta)
}
