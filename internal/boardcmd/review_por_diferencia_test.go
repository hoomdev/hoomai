// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-427, CA-428) en el tablero, con la evidencia EN DISCO (repos git
// reales, CardFor): un registro de 4 lentes cuenta como la review de la
// tarjeta solo si tiene hasta, es completa o delta, su cadena (siguiendo
// desde_review) llega a una completa con todos sus registros de la tarea y
// de 4 lentes, su veredicto es un verde de la tarjeta, y de su hasta al HEAD
// de la tarjeta solo cambio documentacion o nada en las rutas de la
// evidencia (fuera de .hoom/ mas .hoom/agents/). Si ninguno cuenta pero uno
// cumpliria todo salvo lo ultimo, la tarjeta vuelve a Review con el motivo
// y el siguiente paso del delta.
//
// Fixture: la tarea precios con su worktree, un spec aprobado con un
// criterio que se verifica con un comando, un cambio de 450 lineas de
// codigo (la review se exige por tamano) y un veredicto verde con la huella
// actual, todo commiteado: sin registro de review la tarjeta esta en Review;
// con un registro que cuenta, en Tu aceptacion.
package boardcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// rdCodigo es un archivo Go de n lineas.
func rdCodigo(n int, marca string) string {
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 2; i < n; i++ {
		fmt.Fprintf(&b, "var %s%04d = %d\n", marca, i, i)
	}
	return b.String()
}

// rdTarjeta es la tarjeta del fixture: el proyecto, el worktree de la
// tarea, el merge-base con main y el veredicto verde de la huella actual.
type rdTarjeta struct {
	root, wt, mb string
	v            *verdict.Verdict
	at           time.Duration // el reloj del fixture: cada veredicto y registro, mas tarde
}

// rdGrande arma la tarjeta: Review por tamano, sin registro de review.
func rdGrande(t *testing.T) *rdTarjeta {
	t.Helper()
	root := bdRepo(t, "findings:\n  block_on: high\n")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdEscribir(t, wt, "precios.go", rdCodigo(450, "Precio"))
	bdAprobar(t, wt, bdSlug)
	bdCommitear(t, wt, "spec y codigo")
	tj := &rdTarjeta{root: root, wt: wt, mb: bdGit(t, wt, "merge-base", "main", "HEAD"), at: time.Minute}
	tj.verde(t)
	c := tj.card(t)
	bdCol(t, "CA-427 (fixture)", c, ColReview)
	bdPrimero(t, "CA-427 (fixture)", c, rdSinRegistro(t, c))
	return tj
}

// verde escribe y commitea un veredicto verde con la huella actual.
func (tj *rdTarjeta) verde(t *testing.T) *verdict.Verdict {
	t.Helper()
	tj.at += time.Minute
	tj.v = bdVeredictoDisco(t, tj.wt, bdSpec, bdT0.Add(tj.at), false, bdGatesVerdes())
	bdCommitear(t, tj.wt, "veredicto "+tj.v.ID)
	return tj.v
}

// head es el sha del HEAD del worktree.
func (tj *rdTarjeta) head(t *testing.T) string {
	t.Helper()
	return bdGit(t, tj.wt, "rev-parse", "HEAD")
}

// registro es un registro de review de la tarjeta sobre el veredicto actual.
func (tj *rdTarjeta) registro(id, desde, hasta, cobertura, desdeReview string, lentes []string) reviewcmd.Record {
	tj.at += time.Minute
	return reviewcmd.Record{ID: id, CreatedAt: bdT0.Add(tj.at), Task: bdSlug, Spec: bdSpec,
		Fingerprint: tj.v.Git.ChangeFingerprint, VerdictID: tj.v.ID, Verdict: "green", Lenses: lentes,
		Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, WritersDeclared: []string{}, Findings: []string{},
		Usage: []reviewcmd.LensUsage{}, Isolated: true,
		Desde: desde, Hasta: hasta, Cobertura: cobertura, DesdeReview: desdeReview}
}

// escribir deja los registros en el worktree y los commitea (el worktree
// queda limpio: la condicion de cierre se cumple).
func (tj *rdTarjeta) escribir(t *testing.T, recs ...reviewcmd.Record) {
	t.Helper()
	for _, r := range recs {
		bdReviewDisco(t, tj.wt, r)
	}
	bdCommitear(t, tj.wt, "registros de review")
}

// completa escribe una review completa de 4 lentes del merge-base al HEAD
// actual y la devuelve.
func (tj *rdTarjeta) completa(t *testing.T, id string) reviewcmd.Record {
	t.Helper()
	r := tj.registro(id, tj.mb, tj.head(t), reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes)
	tj.escribir(t, r)
	return r
}

// codigo commitea un cambio de codigo en el worktree y un veredicto verde
// nuevo con la huella nueva.
func (tj *rdTarjeta) codigo(t *testing.T, marca string) {
	t.Helper()
	bdEscribir(t, tj.wt, "extra_"+strings.ToLower(marca)+".go", "package app\n\n// "+marca+"\nfunc "+marca+"() {}\n")
	bdCommitear(t, tj.wt, "codigo despues de la review: "+marca)
	tj.verde(t)
}

func (tj *rdTarjeta) card(t *testing.T) Card {
	t.Helper()
	type resultado struct {
		c   Card
		err error
	}
	ch := make(chan resultado, 1)
	go func() {
		c, err := CardFor(tj.root, "main", "high", bdSlug, time.Now().UTC())
		ch <- resultado{c, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			t.Fatalf("CardFor: %v", r.err)
		}
		return r.c
	case <-time.After(30 * time.Second):
		t.Fatal("CA-427: el tablero no volvio en 30 s: seguir desde_review no termina")
	}
	return Card{}
}

// rdSinRegistro es el motivo de hoy de una review exigida sin registro que
// cuente, con las lineas que diga la tarjeta.
func rdSinRegistro(t *testing.T, c Card) string {
	t.Helper()
	for _, m := range c.Missing {
		if strings.HasPrefix(m, "la review exige las 4 lentes (") && strings.HasSuffix(m, " > 400) y no hay registro de review") {
			return m
		}
	}
	t.Fatalf("CA-427: el motivo de hoy de la review exigida (%q) esta en missing: %q",
		"la review exige las 4 lentes (<n> lineas > 400) y no hay registro de review", c.Missing)
	return ""
}

const rdNextHoy = "hoom review --task precios --spec .hoom/specs/precios.md"
const rdNextDelta = "hoom review --task precios --spec .hoom/specs/precios.md --delta"

// rdNoCuenta: la tarjeta sigue en Review con el motivo y el siguiente paso
// de hoy (ningun registro cuenta y ninguno cumpliria todo salvo lo ultimo).
func rdNoCuenta(t *testing.T, ca, caso string, c Card) {
	t.Helper()
	bdCol(t, ca+" ("+caso+")", c, ColReview)
	bdPrimero(t, ca+" ("+caso+")", c, rdSinRegistro(t, c))
	if c.Evidence.ReviewID != "" || !c.Evidence.ReviewRequired {
		t.Fatalf("%s (%s): un registro que no cuenta no es review_id: %+v", ca, caso, c.Evidence)
	}
	if c.Next != rdNextHoy {
		t.Fatalf("%s (%s): el siguiente paso es el de hoy %q, fue %q", ca, caso, rdNextHoy, c.Next)
	}
	bdNoTiene(t, ca+" ("+caso+")", c.Missing, "y el codigo cambio despues")
}

// rdCuenta: la tarjeta esta en Tu aceptacion y la review de la tarjeta es id.
func rdCuenta(t *testing.T, ca, caso string, c Card, id string) {
	t.Helper()
	bdCol(t, ca+" ("+caso+")", c, ColTuAceptacion)
	if c.Evidence.ReviewID != id || !c.Evidence.ReviewRequired {
		t.Fatalf("%s (%s): la review de la tarjeta es %s: %+v", ca, caso, id, c.Evidence)
	}
}

// ---------------------------------------------------------------- CA-427

// CA-427 (guarda del fixture, verde hoy): una review completa de 4 lentes,
// del merge-base al HEAD de la tarjeta, sobre su verde, cuenta.
func TestCA427_CompletaDeCuatroLentesHastaElHEADCuenta(t *testing.T) {
	tj := rdGrande(t)
	r := tj.completa(t, "20260922T160000_c0mp1a")
	rdCuenta(t, "CA-427", "completa hasta HEAD", tj.card(t), r.ID)
}

// CA-427: "un registro sin hasta (anterior a esta spec) ... nunca cuenta",
// aunque tenga 4 lentes sobre el verde actual de la tarjeta: el registro
// viejo tal como lo escribia hoom (sin las claves) y uno con las claves
// vacias. El motivo y el siguiente paso son los de hoy.
func TestCA427_RegistroSinHastaNoCuenta(t *testing.T) {
	tj := rdGrande(t)
	viejo := `{
  "id": "20260922T160000_v1ej0a",
  "created_at": "2026-09-22T16:00:00Z",
  "task": "precios",
  "spec": ".hoom/specs/precios.md",
  "fingerprint": "` + tj.v.Git.ChangeFingerprint + `",
  "verdict_id": "` + tj.v.ID + `",
  "verdict": "green",
  "lenses": ["risk", "reliability", "resilience", "readability"],
  "provider": "codex",
  "writer": "claude",
  "cross": "cruzada",
  "writers_declared": [],
  "findings": []
}
`
	bdEscribir(t, tj.wt, filepath.Join(".hoom", reviewcmd.RecordsDir, "20260922T160000_v1ej0a.json"), viejo)
	bdCommitear(t, tj.wt, "registro viejo")
	rdNoCuenta(t, "CA-427", "registro viejo sin las claves", tj.card(t))

	tj = rdGrande(t)
	tj.escribir(t, tj.registro("20260922T160000_vac10a", "", "", "", "", reviewcmd.Lentes))
	rdNoCuenta(t, "CA-427", "registro con desde, hasta y cobertura vacios", tj.card(t))
}

// CA-427: "uno parcial ... nunca cuenta": un parcial de 4 lentes hasta el
// HEAD de la tarjeta, desde un commit a mano o desde el merge-base (una
// --lens a mano lo deja parcial aunque el rango sea completo). Y un
// registro con hasta y una cobertura que no es completa ni delta, tampoco.
func TestCA427_ParcialNoCuenta(t *testing.T) {
	casos := []struct {
		nombre    string
		desde     func(tj *rdTarjeta, t *testing.T) string
		cobertura string
	}{
		{"parcial-desde-un-commit-a-mano", func(tj *rdTarjeta, t *testing.T) string { return bdGit(t, tj.wt, "rev-parse", "HEAD~1") }, reviewcmd.CoberturaParcial},
		{"parcial-desde-el-merge-base", func(tj *rdTarjeta, t *testing.T) string { return tj.mb }, reviewcmd.CoberturaParcial},
		{"cobertura-desconocida", func(tj *rdTarjeta, t *testing.T) string { return tj.mb }, "total"},
		{"cobertura-vacia", func(tj *rdTarjeta, t *testing.T) string { return tj.mb }, ""},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tj := rdGrande(t)
			tj.escribir(t, tj.registro("20260922T160000_p4rc1a", c.desde(tj, t), tj.head(t), c.cobertura, "", reviewcmd.Lentes))
			rdNoCuenta(t, "CA-427", c.nombre, tj.card(t))
		})
	}
}

// CA-427: "su cadena existe: siguiendo desde_review se llega a un completa,
// y todos los registros del camino son de la misma tarea y de 4 lentes";
// "una cadena rota no cuenta". Un delta de 4 lentes hasta el HEAD de la
// tarjeta, sobre su verde, cuya cadena: nombra un registro que no existe; se
// nombra a si mismo; es un ciclo de dos deltas; pasa por un parcial; llega a
// una completa de otra tarea; llega a una completa de una sola lente (la
// cadena empezo con el cambio chico); o no nombra a nadie. Nunca cuenta, y
// el tablero vuelve (no se cuelga siguiendo un ciclo).
func TestCA427_CadenaRotaNoCuenta(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T, tj *rdTarjeta) []reviewcmd.Record
	}{
		{"desde-review-que-no-existe", func(t *testing.T, tj *rdTarjeta) []reviewcmd.Record {
			return []reviewcmd.Record{tj.registro("20260922T170000_d3lta1", tj.mb, tj.head(t), reviewcmd.CoberturaDelta, "20260922T150000_n0h4y1", reviewcmd.Lentes)}
		}},
		{"se-nombra-a-si-mismo", func(t *testing.T, tj *rdTarjeta) []reviewcmd.Record {
			return []reviewcmd.Record{tj.registro("20260922T170000_d3lta1", tj.mb, tj.head(t), reviewcmd.CoberturaDelta, "20260922T170000_d3lta1", reviewcmd.Lentes)}
		}},
		{"ciclo-de-dos-deltas", func(t *testing.T, tj *rdTarjeta) []reviewcmd.Record {
			return []reviewcmd.Record{
				tj.registro("20260922T170000_d3lta1", tj.mb, tj.head(t), reviewcmd.CoberturaDelta, "20260922T170100_d3lta2", reviewcmd.Lentes),
				tj.registro("20260922T170100_d3lta2", tj.mb, tj.head(t), reviewcmd.CoberturaDelta, "20260922T170000_d3lta1", reviewcmd.Lentes),
			}
		}},
		{"pasa-por-un-parcial", func(t *testing.T, tj *rdTarjeta) []reviewcmd.Record {
			return []reviewcmd.Record{
				tj.registro("20260922T165000_p4rc1a", tj.mb, tj.head(t), reviewcmd.CoberturaParcial, "", reviewcmd.Lentes),
				tj.registro("20260922T170000_d3lta1", tj.head(t), tj.head(t), reviewcmd.CoberturaDelta, "20260922T165000_p4rc1a", reviewcmd.Lentes),
			}
		}},
		{"llega-a-una-completa-de-otra-tarea", func(t *testing.T, tj *rdTarjeta) []reviewcmd.Record {
			otra := tj.registro("20260922T165000_0tr4a1", tj.mb, tj.head(t), reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes)
			otra.Task, otra.Spec = "otra", ".hoom/specs/otra.md"
			return []reviewcmd.Record{otra,
				tj.registro("20260922T170000_d3lta1", tj.head(t), tj.head(t), reviewcmd.CoberturaDelta, otra.ID, reviewcmd.Lentes)}
		}},
		{"llega-a-una-completa-de-una-lente", func(t *testing.T, tj *rdTarjeta) []reviewcmd.Record {
			return []reviewcmd.Record{
				tj.registro("20260922T165000_un4l3n", tj.mb, tj.head(t), reviewcmd.CoberturaCompleta, "", []string{reviewcmd.LenteDominante}),
				tj.registro("20260922T170000_d3lta1", tj.head(t), tj.head(t), reviewcmd.CoberturaDelta, "20260922T165000_un4l3n", reviewcmd.Lentes),
			}
		}},
		{"delta-sin-desde-review", func(t *testing.T, tj *rdTarjeta) []reviewcmd.Record {
			return []reviewcmd.Record{tj.registro("20260922T170000_d3lta1", tj.mb, tj.head(t), reviewcmd.CoberturaDelta, "", reviewcmd.Lentes)}
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tj := rdGrande(t)
			tj.escribir(t, c.armar(t, tj)...)
			rdNoCuenta(t, "CA-427", c.nombre, tj.card(t))
		})
	}
}

// CA-427: "su veredicto es un verde de la tarjeta (como hoy)": una
// completa de 4 lentes hasta el HEAD pero sobre un veredicto que no es un
// verde de la tarjeta no cuenta. Guarda: verde hoy.
func TestCA427_VeredictoQueNoEsUnVerdeDeLaTarjetaNoCuenta(t *testing.T) {
	tj := rdGrande(t)
	r := tj.registro("20260922T160000_r0j0a1", tj.mb, tj.head(t), reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes)
	r.VerdictID = "2026-09-22T15-00-00Z_n0esv3rd"
	tj.escribir(t, r)
	rdNoCuenta(t, "CA-427", "veredicto que no es de la tarjeta", tj.card(t))
}

// CA-427 (caso hostil): un hasta que no es un sha de 40 hex no ancla nada:
// "HEAD" (se moveria con la tarjeta y cubriria cualquier codigo), un nombre
// de rama, y un valor que git tomaria como opcion (--output=<ruta>, que
// escribiria un archivo). Nunca cuenta, y leer el tablero no crea nada.
func TestCA427_HastaQueNoEsUnShaNoCuenta(t *testing.T) {
	pwned := filepath.Join(t.TempDir(), "pwned-ca427")
	for _, c := range []struct{ nombre, hasta string }{
		{"HEAD", "HEAD"}, {"nombre-de-rama", "hoom/precios"}, {"opcion-de-git", "--output=" + pwned},
	} {
		hasta := c.hasta
		t.Run(c.nombre, func(t *testing.T) {
			tj := rdGrande(t)
			tj.escribir(t, tj.registro("20260922T160000_h4st4a", tj.mb, hasta, reviewcmd.CoberturaCompleta, "", reviewcmd.Lentes))
			c := tj.card(t)
			if _, err := os.Stat(pwned); err == nil {
				t.Fatalf("CA-427: leer el tablero con hasta %q no escribe nada: se creo %s", hasta, pwned)
			}
			bdCol(t, "CA-427 (hasta "+hasta+")", c, ColReview)
			if c.Evidence.ReviewID != "" {
				t.Fatalf("CA-427: un hasta %q no cuenta: %+v", hasta, c.Evidence)
			}
		})
	}
}

// CA-427: una cadena sana de tres eslabones (completa -> delta -> delta),
// todos de la tarea y de 4 lentes, cuyo ultimo hasta es el HEAD de la
// tarjeta: cuenta, y la review de la tarjeta es el ultimo eslabon.
func TestCA427_CadenaSanaCuenta(t *testing.T) {
	tj := rdGrande(t)
	r1 := tj.completa(t, "20260922T160000_c0mp1a")
	tj.codigo(t, "Segundo")
	h1 := r1.Hasta
	h2 := tj.head(t)
	r2 := tj.registro("20260922T170000_d3lta1", h1, h2, reviewcmd.CoberturaDelta, r1.ID, reviewcmd.Lentes)
	tj.escribir(t, r2)
	tj.codigo(t, "Tercero")
	r3 := tj.registro("20260922T180000_d3lta2", h2, tj.head(t), reviewcmd.CoberturaDelta, r2.ID, reviewcmd.Lentes)
	tj.escribir(t, r3)
	rdCuenta(t, "CA-427", "completa -> delta -> delta", tj.card(t), r3.ID)
}

// ---------------------------------------------------------------- CA-428

// CA-428: "con codigo commiteado despues de la review, la tarjeta vuelve a
// Review con `la review <id> cubre hasta <hasta12> y el codigo cambio
// despues` y el siguiente paso `hoom review --task <s> --spec <spec>
// --delta`" (en palabras simples, `falta revisar lo nuevo desde la ultima
// review`), aunque haya un verde nuevo con la huella actual y el registro
// sea sobre un verde anterior de la tarjeta. "Despues del delta de 4 lentes
// pasa a Tu aceptacion si lo demas se cumple."
func TestCA428_CodigoDespuesDeLaReviewVuelveAReviewHastaElDelta(t *testing.T) {
	tj := rdGrande(t)
	r1 := tj.completa(t, "20260922T160000_c0mp1a")
	rdCuenta(t, "CA-428 (fixture)", "completa", tj.card(t), r1.ID)

	tj.codigo(t, "Despues")
	c := tj.card(t)
	motivo := "la review " + r1.ID + " cubre hasta " + r1.Hasta[:12] + " y el codigo cambio despues"
	bdCol(t, "CA-428", c, ColReview)
	bdPrimero(t, "CA-428", c, motivo)
	if c.Next != rdNextDelta {
		t.Fatalf("CA-428: el siguiente paso es %q, fue %q", rdNextDelta, c.Next)
	}
	if c.Plain != "falta revisar lo nuevo desde la ultima review" {
		t.Fatalf("CA-428: en palabras simples, %q; fue %q", "falta revisar lo nuevo desde la ultima review", c.Plain)
	}
	if c.Evidence.ReviewID != "" || !c.Evidence.ReviewRequired {
		t.Fatalf("CA-428: la review de antes ya no es la review de la tarjeta: %+v", c.Evidence)
	}
	bdNoTiene(t, "CA-428", c.Missing, "no hay registro de review")

	r2 := tj.registro("20260922T170000_d3lta1", r1.Hasta, tj.head(t), reviewcmd.CoberturaDelta, r1.ID, reviewcmd.Lentes)
	tj.escribir(t, r2)
	rdCuenta(t, "CA-428", "despues del delta de 4 lentes", tj.card(t), r2.ID)
}

// CA-428: "un commit solo de documentacion o solo de .hoom/ no la hace
// volver" (caso limite: "Un commit que solo toca documentacion despues de la
// review: la tarjeta sigue revisada"; "Un commit que solo toca .hoom/
// (veredictos, hallazgos, registros) fuera de .hoom/agents/: no cambia la
// evidencia; la tarjeta sigue revisada"). Guardas: verdes hoy.
func TestCA428_DocumentacionOHoomDespuesDeLaReviewNoLaHaceVolver(t *testing.T) {
	t.Run("documentacion", func(t *testing.T) {
		tj := rdGrande(t)
		r1 := tj.completa(t, "20260922T160000_c0mp1a")
		bdEscribir(t, tj.wt, "README.md", "# precios\n\nprosa nueva despues de la review\n")
		bdEscribir(t, tj.wt, "docs/guia.md", "# guia\n")
		bdCommitear(t, tj.wt, "solo documentacion")
		tj.verde(t)
		rdCuenta(t, "CA-428", "solo documentacion despues", tj.card(t), r1.ID)
	})
	t.Run("hoom", func(t *testing.T) {
		tj := rdGrande(t)
		r1 := tj.completa(t, "20260922T160000_c0mp1a")
		if _, err := finding.Register(tj.wt, "main", finding.Draft{Severity: "low", Lens: "risk",
			Description: "nota sin tarea", Author: "reviewer"}); err != nil {
			t.Fatal(err)
		}
		bdCommitear(t, tj.wt, "un hallazgo")
		tj.verde(t)
		rdCuenta(t, "CA-428", "solo .hoom/ despues", tj.card(t), r1.ID)
	})
}

// CA-428: documentacion Y codigo despues de la review: el codigo la hace
// volver (con el motivo del delta), aunque el ultimo commit sea solo de
// documentacion.
func TestCA428_CodigoYDocumentacionDespuesVuelveAReview(t *testing.T) {
	tj := rdGrande(t)
	r1 := tj.completa(t, "20260922T160000_c0mp1a")
	tj.codigo(t, "Colado")
	bdEscribir(t, tj.wt, "README.md", "# precios\n\nprosa\n")
	bdCommitear(t, tj.wt, "documentacion al final")
	tj.verde(t)
	c := tj.card(t)
	bdCol(t, "CA-428", c, ColReview)
	bdPrimero(t, "CA-428", c, "la review "+r1.ID+" cubre hasta "+r1.Hasta[:12]+" y el codigo cambio despues")
	if c.Next != rdNextDelta {
		t.Fatalf("CA-428: el siguiente paso es %q, fue %q", rdNextDelta, c.Next)
	}
}

// CA-428: el motivo del delta es solo para un registro que cumpliria todo
// salvo lo ultimo: con codigo despues de un parcial, de un registro viejo
// sin hasta o de una cadena rota, el motivo y el siguiente paso son los de
// hoy.
func TestCA428_SinUnRegistroQueCumplaTodoSalvoLoUltimoElMotivoEsElDeHoy(t *testing.T) {
	casos := []struct {
		nombre string
		rec    func(t *testing.T, tj *rdTarjeta) reviewcmd.Record
	}{
		{"parcial", func(t *testing.T, tj *rdTarjeta) reviewcmd.Record {
			return tj.registro("20260922T160000_p4rc1a", tj.mb, tj.head(t), reviewcmd.CoberturaParcial, "", reviewcmd.Lentes)
		}},
		{"sin-hasta", func(t *testing.T, tj *rdTarjeta) reviewcmd.Record {
			return tj.registro("20260922T160000_v1ej0a", "", "", "", "", reviewcmd.Lentes)
		}},
		{"cadena-rota", func(t *testing.T, tj *rdTarjeta) reviewcmd.Record {
			return tj.registro("20260922T160000_d3lta1", tj.mb, tj.head(t), reviewcmd.CoberturaDelta, "20260922T150000_n0h4y1", reviewcmd.Lentes)
		}},
		{"una-lente", func(t *testing.T, tj *rdTarjeta) reviewcmd.Record {
			return tj.registro("20260922T160000_un4l3n", tj.mb, tj.head(t), reviewcmd.CoberturaCompleta, "", []string{reviewcmd.LenteDominante})
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			tj := rdGrande(t)
			tj.escribir(t, c.rec(t, tj))
			tj.codigo(t, "Despues")
			rdNoCuenta(t, "CA-428", c.nombre+" y codigo despues", tj.card(t))
		})
	}
}
