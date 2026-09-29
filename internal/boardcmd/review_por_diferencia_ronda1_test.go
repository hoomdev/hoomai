// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-427, CA-428, ENMIENDA 1), ronda 1: los defectos que encontro la
// primera review de 4 lentes, escritos desde el spec.
//
//   - Enmienda 1 (CA-428): "Si ningun registro cuenta, el tablero mira el
//     registro con el que --delta encadenaria: el mas nuevo completa o delta
//     de la tarea cuyo hasta sigue en la historia de HEAD. Si ese registro
//     cumple todo lo de arriba salvo lo ultimo" pide el delta; "si no (su
//     cadena no es toda de 4 lentes, esta rota, o su veredicto no es un
//     verde de la tarjeta), el motivo y el siguiente paso de hoy: la review
//     completa. Asi el tablero nunca pide un --delta que no lo dejaria
//     satisfecho." Se reproduce el ciclo de punta a punta con hoom review
//     de verdad (reviewcmd.Run con un codex falso) y CardFor: 450 lineas ->
//     R1 de 4 lentes; el cambio se achica -> R2 completa de una lente; el
//     cambio vuelve a crecer -> la tarjeta NO pide --delta.
//   - CA-427 (costo, "Riesgos"): el tablero se vuelve a derivar cada segundo
//     en el Studio y los registros son append-only. Derivar la tarjeta no
//     puede preguntarle a git por cada registro de la historia en cada
//     Build: el primero se detiene en el registro que cubre la tarjeta, y el
//     segundo Build del mismo arbol sin cambios no le vuelve a preguntar a
//     git por las colas.
//
// Los fixtures (rdGrande, rdTarjeta, rdNoCuenta, rdCuenta, rdPuro*) estan en
// review_por_diferencia_test.go.
package boardcmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

// ---------------------------------------------------------------- hoom review de verdad

// r1Codex deja un PATH minimo (sistema + un codex falso que se traga el
// pedido y sale con 0): ningun CLI de IA real de esta maquina corre.
func r1Codex(t *testing.T) {
	t.Helper()
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "codex"), []byte("#!/bin/sh\ncat > /dev/null\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin:/usr/sbin:/sbin")
}

// r1Esperar deja pasar un segundo: los ids de los registros tienen
// resolucion de segundos y "el mas nuevo" tiene que ser inequivoco.
func r1Esperar() { time.Sleep(1100 * time.Millisecond) }

// review corre `hoom review --task precios --spec .hoom/specs/precios.md`
// (con --delta si delta) con el codex falso, con reloj, y commitea el
// registro que deja en el worktree (la condicion de cierre pide el arbol
// limpio).
func (tj *rdTarjeta) review(t *testing.T, ca string, delta bool) (reviewcmd.Result, string) {
	t.Helper()
	type resultado struct {
		res reviewcmd.Result
		err error
		out string
	}
	ch := make(chan resultado, 1)
	go func() {
		var out bytes.Buffer
		res, err := reviewcmd.Run(tj.root, "main", reviewcmd.Options{Provider: "codex", Task: bdSlug, Spec: bdSpec, Delta: delta}, &out)
		ch <- resultado{res, err, out.String()}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("%s: hoom review (delta %v) no se rompe: %v\n%s", ca, delta, r.err, r.out)
		}
		if r.res.RecordID != "" {
			bdCommitear(t, tj.wt, "registro de review "+r.res.RecordID)
		}
		return r.res, r.out
	case <-time.After(90 * time.Second):
		t.Fatalf("%s: hoom review no volvio en 90 s", ca)
	}
	return reviewcmd.Result{}, ""
}

// r1Completa exige una review completa con registro y esas lentes.
func r1Completa(t *testing.T, ca, caso string, res reviewcmd.Result, out string, lentes []string) {
	t.Helper()
	if res.Status != "revisado" || res.RecordID == "" || res.Cobertura != reviewcmd.CoberturaCompleta || !reflect.DeepEqual(res.Lenses, lentes) {
		t.Fatalf("%s: fixture: %s: hoom review sin rango revisa, es completa y lleva %v: %+v\n%s", ca, caso, lentes, res, out)
	}
}

// r1CicloHastaQueCrece arma el ciclo de la enmienda 1 con hoom review de
// verdad: la tarjeta de 450 lineas revisada con las 4 lentes (R1, cuenta);
// el cambio se achica a 100 lineas y se revisa sin rango (R2: completa de
// una lente, la dominante); el cambio vuelve a crecer a 460 lineas, con su
// verde. Devuelve R1 y R2.
func r1CicloHastaQueCrece(t *testing.T, ca string) (tj *rdTarjeta, r1, r2 reviewcmd.Result) {
	t.Helper()
	r1Codex(t)
	tj = rdGrande(t)
	r1, out := tj.review(t, ca, false)
	r1Completa(t, ca, "450 lineas", r1, out, reviewcmd.Lentes)
	rdCuenta(t, ca+" (fixture)", "R1 de 4 lentes", tj.card(t), r1.RecordID)

	r1Esperar()
	bdEscribir(t, tj.wt, "precios.go", rdCodigo(100, "Precio"))
	bdCommitear(t, tj.wt, "el cambio se achica")
	tj.verde(t)
	r2, out = tj.review(t, ca, false)
	r1Completa(t, ca, "100 lineas", r2, out, []string{reviewcmd.LenteDominante})

	r1Esperar()
	bdEscribir(t, tj.wt, "precios.go", rdCodigo(460, "Precio"))
	bdCommitear(t, tj.wt, "el cambio vuelve a crecer")
	tj.verde(t)
	return tj, r1, r2
}

// CA-428 (enmienda 1, caso limite: "Una completa de una lente mas nueva que
// una cadena de 4 lentes (el cambio se achico, se reviso con la lente
// dominante y volvio a crecer): --delta encadenaria con ella, asi que el
// tablero pide la review completa, no el delta"). De punta a punta: con R1
// (cadena de 4) y R2 (completa de una lente, mas nueva) y codigo despues de
// las dos, la tarjeta esta en Review con el motivo y el siguiente paso de
// hoy (`hoom review --task precios --spec .hoom/specs/precios.md`, sin
// --delta), nunca `la review R1 cubre hasta ... y el codigo cambio
// despues`. La review completa que pide la devuelve a Tu aceptacion.
func TestCA428_CompletaDeUnaLenteMasNuevaPideLaReviewCompleta(t *testing.T) {
	tj, r1, _ := r1CicloHastaQueCrece(t, "CA-428")
	c := tj.card(t)
	if strings.Contains(c.Next, "--delta") {
		t.Fatalf("CA-428: --delta encadenaria con R2 (completa de una lente, mas nueva): el tablero nunca pide un --delta que no lo dejaria satisfecho: next %q, missing %q", c.Next, c.Missing)
	}
	rdNoCuenta(t, "CA-428", "completa de una lente mas nueva que la cadena de 4", c)
	bdNoTiene(t, "CA-428", c.Missing, "la review "+r1.RecordID+" cubre hasta")

	r1Esperar()
	r3, out := tj.review(t, "CA-428", false)
	r1Completa(t, "CA-428", "460 lineas", r3, out, reviewcmd.Lentes)
	rdCuenta(t, "CA-428", "despues de la review completa", tj.card(t), r3.RecordID)
}

// CA-428 (enmienda 1: "Asi el tablero nunca pide un --delta que no lo
// dejaria satisfecho"), como propiedad del ciclo: desde el estado del caso
// anterior, seguir el siguiente paso que dice la tarjeta (hoom review con o
// sin --delta, lo que diga) la saca de Review en a lo sumo 3 pasos. Con el
// tablero de antes de la enmienda la tarjeta pide --delta, --delta encadena
// con R2 (la mas nueva), el delta no cuenta y la tarjeta vuelve a pedir
// --delta: nunca sale de Review.
func TestCA428_SeguirElSiguientePasoSaleDeReview(t *testing.T) {
	tj, _, _ := r1CicloHastaQueCrece(t, "CA-428")
	var pasos []string
	for paso := 0; paso < 3; paso++ {
		c := tj.card(t)
		if c.Column == ColTuAceptacion {
			return
		}
		bdCol(t, fmt.Sprintf("CA-428 (paso %d, %q)", paso+1, pasos), c, ColReview)
		r1Esperar()
		var res reviewcmd.Result
		switch c.Next {
		case rdNextDelta:
			res, _ = tj.review(t, "CA-428", true)
		case rdNextHoy:
			res, _ = tj.review(t, "CA-428", false)
		default:
			t.Fatalf("CA-428: paso %d: el siguiente paso de una tarjeta en Review por la review es %q o %q, fue %q (missing %q)",
				paso+1, rdNextHoy, rdNextDelta, c.Next, c.Missing)
		}
		pasos = append(pasos, fmt.Sprintf("%s -> %s %s desde_review=%q lentes=%v (motivo %q)",
			c.Next, res.Status, res.Cobertura, res.DesdeReview, res.Lenses, c.Missing[0]))
	}
	c := tj.card(t)
	if c.Column != ColTuAceptacion {
		t.Fatalf("CA-428: seguir el siguiente paso de la tarjeta no la saca de Review en 3 pasos (el tablero pide un --delta que no lo deja satisfecho):\n  %s\nahora: %s, %q, %q",
			strings.Join(pasos, "\n  "), c.Column, c.Next, c.Missing)
	}
}

// CA-428 (control de punta a punta, la cadena de 4 sigue siendo la
// encadenable): R1 de 4 lentes, codigo despues (el cambio sigue de mas de
// 400 lineas): la tarjeta vuelve a Review con `la review R1 cubre hasta
// <hasta12> y el codigo cambio despues` y el siguiente paso --delta; el
// --delta de hoom review encadena con R1, lleva las 4 lentes (la regla
// sobre el cambio entero) y la tarjeta pasa a Tu aceptacion.
func TestCA428_CadenaDeCuatroConCodigoDespuesPideElDeltaYElDeltaAlcanza(t *testing.T) {
	r1Codex(t)
	tj := rdGrande(t)
	r1, out := tj.review(t, "CA-428", false)
	r1Completa(t, "CA-428", "450 lineas", r1, out, reviewcmd.Lentes)
	rdCuenta(t, "CA-428 (fixture)", "R1 de 4 lentes", tj.card(t), r1.RecordID)

	r1Esperar()
	tj.codigo(t, "Despues")
	c := tj.card(t)
	bdCol(t, "CA-428", c, ColReview)
	bdPrimero(t, "CA-428", c, "la review "+r1.RecordID+" cubre hasta "+r1.Hasta[:12]+" y el codigo cambio despues")
	if c.Next != rdNextDelta {
		t.Fatalf("CA-428: el siguiente paso es %q, fue %q", rdNextDelta, c.Next)
	}
	r2, out := tj.review(t, "CA-428", true)
	if r2.Status != "revisado" || r2.Cobertura != reviewcmd.CoberturaDelta || r2.DesdeReview != r1.RecordID || !reflect.DeepEqual(r2.Lenses, reviewcmd.Lentes) {
		t.Fatalf("CA-428/CA-424: el --delta encadena con R1 y lleva las 4 lentes: %+v\n%s", r2, out)
	}
	rdCuenta(t, "CA-428", "despues del delta", tj.card(t), r2.RecordID)
}

// ---------------------------------------------------------------- Derive puro

const rdPuroH3 = "dddddddddddddddddddddddddddddddddddddddd"

// CA-428 (enmienda 1, Derive puro): con codigo despues de todos los
// registros, el tablero pide --delta solo si el registro con el que --delta
// encadenaria (el mas nuevo completa o delta con hasta en la historia) tiene
// su cadena de 4 lentes hasta una completa y su veredicto es un verde de la
// tarjeta; si no, el motivo y el siguiente paso de hoy. Los que --delta
// salta (un parcial, un hasta fuera de la historia, uno sin hasta) no
// cambian el registro con el que encadenaria.
func TestCA428_DerivePuroPideElDeltaSoloSiElRegistroQueEncadenariaLaDejaSatisfecha(t *testing.T) {
	cuatro, una := reviewcmd.Lentes, []string{reviewcmd.LenteDominante}
	despues := ReviewTail{Ancestro: true, Codigo: true}
	r1 := rdPuroRec("r1", 35*time.Minute, rdPuroMB, rdPuroH1, reviewcmd.CoberturaCompleta, "", cuatro)
	ajeno := rdPuroRec("r2", 40*time.Minute, rdPuroMB, rdPuroH2, reviewcmd.CoberturaCompleta, "", cuatro)
	ajeno.VerdictID = "v-ajeno"
	casos := []struct {
		nombre string
		recs   []reviewcmd.Record
		colas  map[string]ReviewTail
		delta  string // el registro que nombra el motivo del delta; "" = el motivo de hoy
	}{
		{"control-solo-la-cadena-de-4", []reviewcmd.Record{r1},
			map[string]ReviewTail{"r1": despues}, "r1"},
		{"completa-de-una-lente-mas-nueva", []reviewcmd.Record{r1,
			rdPuroRec("r2", 40*time.Minute, rdPuroMB, rdPuroH2, reviewcmd.CoberturaCompleta, "", una)},
			map[string]ReviewTail{"r1": despues, "r2": despues}, ""},
		{"completa-de-una-lente-mas-nueva-con-el-mismo-hasta", []reviewcmd.Record{r1,
			rdPuroRec("r2", 40*time.Minute, rdPuroMB, rdPuroH1, reviewcmd.CoberturaCompleta, "", una)},
			map[string]ReviewTail{"r1": despues, "r2": despues}, ""},
		{"delta-de-una-lente-mas-nuevo", []reviewcmd.Record{r1,
			rdPuroRec("r2", 40*time.Minute, rdPuroH1, rdPuroH2, reviewcmd.CoberturaDelta, "r1", una)},
			map[string]ReviewTail{"r1": despues, "r2": despues}, ""},
		{"delta-mas-nuevo-con-la-cadena-rota", []reviewcmd.Record{r1,
			rdPuroRec("r2", 40*time.Minute, rdPuroH1, rdPuroH2, reviewcmd.CoberturaDelta, "r-no-esta", cuatro)},
			map[string]ReviewTail{"r1": despues, "r2": despues}, ""},
		{"delta-mas-nuevo-que-pasa-por-una-lente", []reviewcmd.Record{r1,
			rdPuroRec("r2", 40*time.Minute, rdPuroMB, rdPuroH2, reviewcmd.CoberturaCompleta, "", una),
			rdPuroRec("r3", 45*time.Minute, rdPuroH2, rdPuroH3, reviewcmd.CoberturaDelta, "r2", cuatro)},
			map[string]ReviewTail{"r1": despues, "r2": despues, "r3": despues}, ""},
		{"completa-mas-nueva-sobre-un-veredicto-que-no-es-de-la-tarjeta", []reviewcmd.Record{r1, ajeno},
			map[string]ReviewTail{"r1": despues, "r2": despues}, ""},
		// lo que --delta salta: sigue encadenando con r1
		{"parcial-mas-nuevo", []reviewcmd.Record{r1,
			rdPuroRec("r2", 40*time.Minute, rdPuroH1, rdPuroH2, reviewcmd.CoberturaParcial, "", cuatro)},
			map[string]ReviewTail{"r1": despues, "r2": despues}, "r1"},
		{"una-lente-mas-nueva-fuera-de-la-historia", []reviewcmd.Record{r1,
			rdPuroRec("r2", 40*time.Minute, rdPuroMB, rdPuroH2, reviewcmd.CoberturaCompleta, "", una)},
			map[string]ReviewTail{"r1": despues, "r2": {Ancestro: false, Codigo: true}}, "r1"},
		{"una-lente-mas-nueva-sin-hasta", []reviewcmd.Record{r1,
			rdPuroRec("r2", 40*time.Minute, "", "", "", "", una)},
			map[string]ReviewTail{"r1": despues}, "r1"},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			ev := rdPuroGrande()
			ev.Reviews, ev.ReviewTails = c.recs, c.colas
			card := rdPuroDerive(t, "CA-428", c.nombre, ev)
			bdCol(t, "CA-428 ("+c.nombre+")", card, ColReview)
			if card.Evidence.ReviewID != "" {
				t.Fatalf("CA-428: %s: con codigo despues ningun registro es la review de la tarjeta: %+v", c.nombre, card.Evidence)
			}
			if c.delta != "" {
				bdPrimero(t, "CA-428 ("+c.nombre+")", card, "la review "+c.delta+" cubre hasta "+rdPuroH1[:12]+" y el codigo cambio despues")
				if card.Next != rdNextDelta {
					t.Fatalf("CA-428: %s: el siguiente paso es %q, fue %q", c.nombre, rdNextDelta, card.Next)
				}
				return
			}
			bdPrimero(t, "CA-428 ("+c.nombre+")", card, rdPuroSinRegistro)
			if card.Next != rdNextHoy {
				t.Fatalf("CA-428: %s: el registro con el que --delta encadenaria no deja la tarjeta satisfecha: el siguiente paso es el de hoy %q, fue %q",
					c.nombre, rdNextHoy, card.Next)
			}
			bdNoTiene(t, "CA-428 ("+c.nombre+")", card.Missing, "y el codigo cambio despues")
		})
	}
}

// ---------------------------------------------------------------- CA-427: el costo de las colas

// r1GitEspia pone al frente del PATH un git que deja cada invocacion (sus
// argumentos separados por NUL) en un archivo propio de dir y despues es el
// git de verdad.
func r1GitEspia(t *testing.T) (dir string) {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("sin git en el PATH")
	}
	real, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	bin, dir := t.TempDir(), t.TempDir()
	s := "#!/bin/sh\nf=$(mktemp '" + dir + "/inv.XXXXXXXX') || exit 96\nprintf '%s\\000' \"$@\" > \"$f\"\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return dir
}

// r1Invocaciones lee y borra las invocaciones de git que anoto el espia.
func r1Invocaciones(t *testing.T, dir string) [][]string {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var invs [][]string
	for _, e := range ents {
		p := filepath.Join(dir, e.Name())
		raw, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		args := strings.Split(string(raw), "\x00")
		if len(args) > 0 && args[len(args)-1] == "" {
			args = args[:len(args)-1]
		}
		invs = append(invs, args)
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
	}
	return invs
}

// r1Colas son las invocaciones de `git merge-base` o `git diff` (las que
// arman una cola: merge-base --is-ancestor y diff --numstat) que nombran
// alguno de los hasta.
func r1Colas(invs [][]string, hastas []string) []string {
	var colas []string
	for _, args := range invs {
		sub := false
		for _, a := range args {
			if a == "merge-base" || a == "diff" {
				sub = true
				break
			}
		}
		if !sub {
			continue
		}
		for _, a := range args {
			nombra := false
			for _, h := range hastas {
				if strings.Contains(a, h) {
					nombra = true
					break
				}
			}
			if nombra {
				colas = append(colas, strings.Join(args, " "))
				break
			}
		}
	}
	return colas
}

// r1Primeras son las primeras 8 invocaciones de la lista (para el mensaje).
func r1Primeras(colas []string) []string {
	if len(colas) > 8 {
		return append(colas[:8:8], fmt.Sprintf("... y %d mas", len(colas)-8))
	}
	return colas
}

// r1Historia arma la tarjeta con n registros de 4 lentes, todos de la tarea
// y sobre verdes de la tarjeta, cada uno despues de un commit de codigo: una
// cadena completa -> delta -> ... con una completa nueva cada 10 (las
// completas viejas y sus deltas quedan superados). El mas nuevo (un delta)
// cubre el HEAD de la tarjeta: despues de el solo se commiteo .hoom/.
// Devuelve la tarjeta y los registros, del mas viejo al mas nuevo.
func r1Historia(t *testing.T, n int) (*rdTarjeta, []reviewcmd.Record) {
	t.Helper()
	tj := rdGrande(t)
	recs := []reviewcmd.Record{tj.completa(t, "20260922T160001_000001")}
	for i := 2; i <= n; i++ {
		tj.codigo(t, fmt.Sprintf("Ronda%03d", i))
		id := fmt.Sprintf("20260922T16%02d%02d_%06x", i/60, i%60, i)
		prev := recs[len(recs)-1]
		var r reviewcmd.Record
		if i%10 == 5 {
			r = tj.registro(id, tj.mb, tj.head(t), reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes)
		} else {
			r = tj.registro(id, prev.Hasta, tj.head(t), reviewcmd.CoberturaDelta, prev.ID, reviewcmd.Lentes)
		}
		tj.escribir(t, r)
		recs = append(recs, r)
	}
	if recs[n-1].Cobertura != reviewcmd.CoberturaDelta {
		t.Fatal("CA-427: fixture: el registro mas nuevo es un delta")
	}
	return tj, recs
}

// r1Build corre Build con reloj y devuelve la tarjeta de la tarea.
func r1Build(t *testing.T, ca string, tj *rdTarjeta) Card {
	t.Helper()
	type resultado struct {
		b   Board
		err error
	}
	ch := make(chan resultado, 1)
	go func() {
		b, err := Build(tj.root, "main", "high", time.Now().UTC())
		ch <- resultado{b, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("%s: Build: %v", ca, r.err)
		}
		return bdCard(t, r.b, bdSlug)
	case <-time.After(60 * time.Second):
		t.Fatalf("%s: Build no volvio en 60 s", ca)
	}
	return Card{}
}

// r1TopeColas es cuantas invocaciones de git que nombran un hasta VIEJO
// (superado por el registro que cubre la tarjeta) puede correr un Build:
// una constante chica, nunca proporcional a la historia.
const r1TopeColas = 3

const r1Registros = 30

// CA-427 (costo): con 30 registros de review de la tarea (29 superados por
// el mas nuevo, que cubre la tarjeta), derivar el tablero no recorre la
// historia: el Build calcula colas solo hasta el registro que cubre la
// tarjeta, y corre a lo sumo r1TopeColas invocaciones de git merge-base /
// git diff que nombren el hasta de un registro superado.
func TestCA427_BuildNoRecorreLaHistoriaDeReviews(t *testing.T) {
	tj, recs := r1Historia(t, r1Registros)
	nuevo := recs[len(recs)-1]
	var viejos []string
	for _, r := range recs[:len(recs)-1] {
		viejos = append(viejos, r.Hasta)
	}
	espia := r1GitEspia(t)
	c := r1Build(t, "CA-427", tj)
	rdCuenta(t, "CA-427 (fixture)", "el delta mas nuevo cubre la tarjeta", c, nuevo.ID)
	if colas := r1Colas(r1Invocaciones(t, espia), viejos); len(colas) > r1TopeColas {
		t.Fatalf("CA-427: un Build no le pregunta a git por cada registro de la historia: con %d registros corrio %d invocaciones de merge-base/diff sobre hasta superados (tope %d):\n  %s",
			r1Registros, len(colas), r1TopeColas, strings.Join(r1Primeras(colas), "\n  "))
	}
}

// CA-427 (costo): el segundo Build del mismo arbol sin cambios (lo que el
// Studio hace cada segundo) no le vuelve a preguntar a git por las colas de
// review: ninguna invocacion de git merge-base / git diff nombra el hasta de
// ningun registro. Guarda: la tarjeta es la misma; y cuando el HEAD cambia
// (codigo despues del registro que la cubria), el Build siguiente lo ve: la
// tarjeta vuelve a Review con el motivo del delta (el cache no queda viejo).
func TestCA427_SegundoBuildDelMismoArbolNoRecalculaColas(t *testing.T) {
	tj, recs := r1Historia(t, r1Registros)
	nuevo := recs[len(recs)-1]
	var hastas []string
	for _, r := range recs {
		hastas = append(hastas, r.Hasta)
	}
	espia := r1GitEspia(t)
	c1 := r1Build(t, "CA-427", tj)
	rdCuenta(t, "CA-427 (fixture)", "primer Build", c1, nuevo.ID)
	r1Invocaciones(t, espia)

	c2 := r1Build(t, "CA-427", tj)
	colas := r1Colas(r1Invocaciones(t, espia), hastas)
	rdCuenta(t, "CA-427", "segundo Build", c2, nuevo.ID)
	if len(colas) != 0 {
		// Errorf: la guarda de abajo (el cache no queda viejo) corre igual
		t.Errorf("CA-427: el segundo Build del mismo arbol no vuelve a calcular colas de review (cache por arbol, hasta y HEAD): corrio %d invocaciones:\n  %s",
			len(colas), strings.Join(r1Primeras(colas), "\n  "))
	}

	tj.codigo(t, "DespuesDelCache")
	r1Invocaciones(t, espia)
	c3 := r1Build(t, "CA-427", tj)
	bdCol(t, "CA-427/CA-428 (HEAD nuevo)", c3, ColReview)
	bdPrimero(t, "CA-427/CA-428 (HEAD nuevo)", c3, "la review "+nuevo.ID+" cubre hasta "+nuevo.Hasta[:12]+" y el codigo cambio despues")
	if c3.Next != rdNextDelta {
		t.Fatalf("CA-427/CA-428: con codigo despues del registro el siguiente paso es %q, fue %q", rdNextDelta, c3.Next)
	}
}
