// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-425, CA-426): con rango, el pedido cambia solo la primera linea, la
// linea Rango: (en lugar de Base:) y la linea de INTRODUCIDO; la salida dice
// `desde <desde12>` en su primera linea y lleva la linea rango; la politica
// review: y el contrato del reviewer siguen saliendo del merge-base con la
// base aunque la rama los cambie antes de desde. Los fixtures estan en
// review_por_diferencia_helpers_test.go.
package reviewcmd

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// La segunda mitad de la linea de INTRODUCIDO, igual en un delta y en un
// parcial.
const rdIntroducido = "INTRODUCIDO es lo que trae este rango o lo que este rango rompe de lo anterior; lo demas es pre-existente."

func rdPrimeraConRango(desde, hasta string, ev Evidencia) string {
	return fmt.Sprintf("Revisa lo que cambio en esta rama desde %s hasta %s. La evidencia completa esta abajo, congelada por hoom (sha256 %s, %d KiB): "+
		"no vuelvas a sacar el diff; lee otros archivos solo por rangos y solo si hace falta.", desde[:12], hasta[:12], ev.SHA256, raKiB(ev.Bytes))
}

func rdLineaRango(desde, hasta, cobertura string, n, ins, del int) string {
	return fmt.Sprintf("Rango: %s..%s (%s). Tamano: %d archivos, +%d/-%d lineas.", desde[:12], hasta[:12], cobertura, n, ins, del)
}

// rdFinDeLaEvidencia es el indice en p justo despues de la linea
// '=== fin de la evidencia <sha256> ===' (sin su salto de linea).
func rdFinDeLaEvidencia(t *testing.T, p string) int {
	t.Helper()
	i := strings.Index(p, "\n=== fin de la evidencia ")
	if i < 0 {
		t.Fatalf("CA-425: el pedido cierra la evidencia:\n%s", p)
	}
	j := strings.Index(p[i+1:], "\n")
	if j < 0 {
		return len(p)
	}
	return i + 1 + j
}

// rdPedidoConRango compara el pedido con rango p contra el pedido sin rango
// p0 de la misma lente en el mismo repo: cambia la primera linea, Base: pasa
// a Rango: y va la linea de INTRODUCIDO despues; lo demas (la linea de dato,
// el veredicto, Spec:, la evidencia del rango entre sus marcadores y todo lo
// que sigue a la evidencia) es igual.
func rdPedidoConRango(t *testing.T, caso, p0, p string, ev Evidencia, spec, primera, rango, intro string) {
	t.Helper()
	l0, l := raLineas(p0), raLineas(p)
	if len(l) < 8 {
		t.Fatalf("CA-425: %s: el pedido es mas corto que su contrato:\n%s", caso, p)
	}
	if l[0] != primera {
		t.Fatalf("CA-425: %s: la primera linea es\n%q\nfue\n%q", caso, primera, l[0])
	}
	if l[1] != l0[1] {
		t.Fatalf("CA-425: %s: la segunda linea (dato) no cambia: %q, fue %q", caso, l0[1], l[1])
	}
	if l[2] != rango {
		t.Fatalf("CA-425: %s: 'Base: ...' pasa a\n%q\nfue\n%q", caso, rango, l[2])
	}
	if raLinea(l, 0, "Base:") >= 0 {
		t.Fatalf("CA-425: %s: con rango no queda la linea Base::\n%s", caso, p)
	}
	if l[3] != intro {
		t.Fatalf("CA-425: %s: despues de Rango: va\n%q\nfue\n%q", caso, intro, l[3])
	}
	k0, k := raLinea(l0, 0, "=== spec "), raLinea(l, 0, "=== spec ")
	if k0 < 0 || k < 0 || !reflect.DeepEqual(l[4:k], l0[3:k0]) {
		t.Fatalf("CA-425: %s: entre INTRODUCIDO y la evidencia van las mismas lineas que sin rango (veredicto, Spec:):\nsin rango %q\ncon rango %q", caso, l0[3:max(k0, 3)], l[4:max(k, 4)])
	}
	bloque := raBloque(t, ev, spec, true)
	if !strings.HasPrefix(strings.Join(l[k:], "\n"), bloque) {
		t.Fatalf("CA-425: %s: la evidencia del rango va entera, contigua, entre sus marcadores con su sha256:\n%s", caso, p)
	}
	for _, marca := range []string{"=== diff " + ev.SHA256 + " ===", "=== fin de la evidencia " + ev.SHA256 + " ===", "=== spec " + spec + " " + ev.SHA256 + " ==="} {
		if n := raCuenta(p, marca); n != 1 {
			t.Fatalf("CA-425: %s: el marcador %q aparece una vez, aparecio %d", caso, marca, n)
		}
	}
	cola, cola0 := p[rdFinDeLaEvidencia(t, p):], p0[rdFinDeLaEvidencia(t, p0):]
	if cola != cola0 {
		t.Fatalf("CA-425: %s: despues de la evidencia (lente, registro, cierre) el pedido es el de la review sin rango:\nsin rango:\n%s\ncon rango:\n%s", caso, cola0, cola)
	}
}

// CA-425: "con rango, el pedido cambia solo la primera linea, la linea
// Rango: y la linea de INTRODUCIDO (con la review encadenada en un delta y
// `no es parte de esta review` en un parcial); la review sin rango manda
// exactamente el pedido de hoy". En el mismo repo y con la misma lente: la
// review sin rango (el pedido de hoy, CA-403), un --desde a mano (parcial) y
// un --delta encadenado a la primera.
func TestCA425_PedidoConRangoCambiaSoloTresLineas(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root, _, a, b := rdRamaDos(t, "", spec)
	cx := raInstalar(t, bin, "codex", "")
	g := gitx.Snapshot(root, "main")
	ev0 := raEvidencia(t, "CA-425", root, spec)

	completa, out := rdRevisar(t, "CA-425", root, Options{Provider: "codex", Spec: spec})
	if completa.Status != "revisado" || cx.veces() != 1 || !reflect.DeepEqual(completa.Lenses, []string{LenteDominante}) {
		t.Fatalf("CA-425: fixture: la review sin rango corre una lente (la dominante): %+v\n%s", completa, out)
	}
	p0 := cx.pedido(t, 1)
	raRevisarPedido(t, p0, LenteDominante, "codex", g, ev0, spec, true, "No hay veredicto vigente: la review no reemplaza a 'hoom verify'.")
	if raLinea(raLineas(p0), 0, "Rango:") >= 0 || strings.Contains(p0, "Lo anterior a ") || strings.Contains(p0, "Revisa lo que cambio en esta rama desde") {
		t.Fatalf("CA-425: la review sin rango manda exactamente el pedido de hoy:\n%s", p0)
	}

	// parcial: --desde A a mano
	res, out := rdRevisar(t, "CA-425", root, Options{Provider: "codex", Spec: spec, Desde: a})
	if res.Status != "revisado" || cx.veces() != 2 || res.Cobertura != CoberturaParcial {
		t.Fatalf("CA-425/CA-423: el --desde a mano corre y es parcial: %+v\n%s", res, out)
	}
	ev := rdOraculo(t, "CA-425", root, a, spec)
	n, ins, del := rdTamano(t, root, a)
	rdPedidoConRango(t, "parcial", p0, cx.pedido(t, 2), ev, spec,
		rdPrimeraConRango(a, b, ev), rdLineaRango(a, b, CoberturaParcial, n, ins, del),
		"Lo anterior a "+a[:12]+" no es parte de esta review: "+rdIntroducido)

	// delta: encadenado a la review sin rango
	rdEsperarSegundo()
	c := rdCommit(t, root, "C", map[string]string{
		"c.go":      "package app\n\n// MARCA-DE-C\nfunc C() {}\n",
		"c_util.go": "package app\n\nfunc CUtil() {}\n",
	})
	res, out = rdRevisar(t, "CA-425", root, Options{Provider: "codex", Spec: spec, Delta: true})
	if res.Status != "revisado" || cx.veces() != 3 || res.Cobertura != CoberturaDelta {
		t.Fatalf("CA-425/CA-423: el --delta corre y es delta: %+v\n%s", res, out)
	}
	ev = rdOraculo(t, "CA-425", root, b, spec)
	n, ins, del = rdTamano(t, root, b)
	rdPedidoConRango(t, "delta", p0, cx.pedido(t, 3), ev, spec,
		rdPrimeraConRango(b, c, ev), rdLineaRango(b, c, CoberturaDelta, n, ins, del),
		"Lo anterior a "+b[:12]+" ya lo reviso la review "+completa.RecordID+": "+rdIntroducido)
}

// CA-425: con --desde igual al merge-base la review tiene rango (el flag
// esta) y es completa: la primera linea y la linea Rango: lo dicen, con el
// tamano del rango (que es el cambio entero fuera de .hoom/).
func TestCA425_PedidoConDesdeElMergeBase(t *testing.T) {
	bin := raPATH(t)
	root, mb, _, b := rdRamaDos(t, "", "")
	cx := raInstalar(t, bin, "codex", "")
	res, out := rdRevisar(t, "CA-425", root, Options{Provider: "codex", Desde: mb})
	if res.Status != "revisado" || res.Cobertura != CoberturaCompleta {
		t.Fatalf("CA-425/CA-423: --desde el merge-base corre y es completa: %+v\n%s", res, out)
	}
	ev := raEvidencia(t, "CA-425", root, "")
	n, ins, del := rdTamano(t, root, mb)
	l := raLineas(cx.pedido(t, 1))
	if l[0] != rdPrimeraConRango(mb, b, ev) || l[2] != rdLineaRango(mb, b, CoberturaCompleta, n, ins, del) {
		t.Fatalf("CA-425: con --desde el merge-base el pedido dice el rango, completa:\n%q\n%q", l[0], l[2])
	}
}

// ---------------------------------------------------------------- CA-426

// rdLineaDeRango devuelve la unica linea de la salida que empieza con
// '  rango ' (y exige que vaya despues de la primera y antes de la primera
// lente), o falla.
func rdLineaDeRango(t *testing.T, caso, out string) string {
	t.Helper()
	lineas := raLineas(out)
	i := raLinea(lineas, 0, "  rango ")
	if i < 0 {
		t.Fatalf("CA-426: %s: con rango la salida lleva la linea rango:\n%s", caso, out)
	}
	if j := raLinea(lineas, i+1, "  rango "); j >= 0 {
		t.Fatalf("CA-426: %s: una sola linea rango:\n%s", caso, out)
	}
	if p := raLinea(lineas, 0, "  [1/"); i == 0 || (p >= 0 && i > p) {
		t.Fatalf("CA-426: %s: la linea rango va despues de la primera y antes de las lentes:\n%s", caso, out)
	}
	return lineas[i]
}

// CA-426: "con rango, la salida dice `desde <desde12>` en la primera linea
// y lleva la linea rango": la primera linea es la de hoy con `desde
// <desde12>` en lugar de `contra <base>` y el tamano del rango; despues,
// `  rango       <desde12>..<hasta12> - <cobertura>` (+ ` de la review <id>`
// en un delta). La review sin rango imprime lo de hoy: `contra main` y sin
// linea rango.
func TestCA426_SalidaConRango(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root, mb, a, b := rdRamaDos(t, "", spec)
	raInstalar(t, bin, "codex", "")
	g := gitx.Snapshot(root, "main")

	completa, out := rdRevisar(t, "CA-426", root, Options{Provider: "codex", Spec: spec})
	hoy := fmt.Sprintf("hoom review: %d archivos cambiados, +%d/-%d lineas contra main", len(g.ChangedFiles), g.Insertions, g.Deletions)
	if raLineas(out)[0] != hoy || raLinea(raLineas(out), 0, "  rango ") >= 0 {
		t.Fatalf("CA-426: la review sin rango imprime lo de hoy (%q, sin linea rango):\n%s", hoy, out)
	}

	casos := []struct {
		nombre, desde, cobertura string
		opt                      Options
	}{
		{"parcial", a, CoberturaParcial, Options{Provider: "codex", Spec: spec, Desde: a}},
		{"completa-por-desde", mb, CoberturaCompleta, Options{Provider: "codex", Spec: spec, Desde: mb}},
	}
	encadenable := completa.RecordID // la completa mas nueva con hasta B
	for _, c := range casos {
		rdEsperarSegundo()
		res, out := rdRevisar(t, "CA-426", root, c.opt)
		if res.Status != "revisado" || res.Cobertura != c.cobertura {
			t.Fatalf("CA-426/CA-423: %s: la review corre con su cobertura: %+v\n%s", c.nombre, res, out)
		}
		if res.Cobertura == CoberturaCompleta {
			encadenable = res.RecordID
		}
		n, ins, del := rdTamano(t, root, c.desde)
		primera := fmt.Sprintf("hoom review: %d archivos cambiados, +%d/-%d lineas desde %s", n, ins, del, c.desde[:12])
		if raLineas(out)[0] != primera || strings.Contains(raLineas(out)[0], "contra ") {
			t.Fatalf("CA-426: %s: la primera linea es\n%q\nfue\n%q", c.nombre, primera, raLineas(out)[0])
		}
		if l := rdLineaDeRango(t, c.nombre, out); l != "  rango       "+c.desde[:12]+".."+b[:12]+" - "+c.cobertura {
			t.Fatalf("CA-426: %s: la linea rango es %q, fue %q", c.nombre, "  rango       "+c.desde[:12]+".."+b[:12]+" - "+c.cobertura, l)
		}
	}

	rdEsperarSegundo()
	c := rdCommit(t, root, "C", map[string]string{"c.go": "package app\n\nfunc C() {}\n", "c_util.go": "package app\n\nfunc CUtil() {}\n"})
	res, out := rdRevisar(t, "CA-426", root, Options{Provider: "codex", Spec: spec, Delta: true})
	if res.Cobertura != CoberturaDelta {
		t.Fatalf("CA-426/CA-423: el --delta es delta: %+v\n%s", res, out)
	}
	n, ins, del := rdTamano(t, root, b)
	primera := fmt.Sprintf("hoom review: %d archivos cambiados, +%d/-%d lineas desde %s", n, ins, del, b[:12])
	if raLineas(out)[0] != primera {
		t.Fatalf("CA-426: delta: la primera linea es\n%q\nfue\n%q", primera, raLineas(out)[0])
	}
	if res.DesdeReview != encadenable {
		t.Fatalf("CA-422: el delta encadena con la completa mas nueva (%s, la de --desde el merge-base): %+v", encadenable, res)
	}
	quiero := "  rango       " + b[:12] + ".." + c[:12] + " - delta de la review " + encadenable
	if l := rdLineaDeRango(t, "delta", out); l != quiero {
		t.Fatalf("CA-426: delta: la linea rango es %q, fue %q", quiero, l)
	}
}

// CA-426: "la politica review: y el contrato del reviewer salen del
// merge-base con la base aunque la rama los cambie en un commit anterior a
// desde". La base trae su contrato 06 y ninguna seccion review:; el primer
// commit de la rama se da `review: {same_provider: true, isolated: false}` y
// reescribe el contrato 06; despues, una review completa con codex (el
// writer fue claude) y otro commit de codigo. Con --delta y con --desde ese
// primer commit: codex recibe el contrato de la BASE y corre aislado; con
// claude (el que escribio) la review se niega por no cruzada, sin pasadas ni
// registro.
func TestCA426_PoliticaYContratoDelMergeBaseConRango(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root := raRepo(t, "")
	write(t, root, raContrato, raContratoBase)
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "la base trae su contrato 06")
	git(t, root, "checkout", "-q", "-b", "feature")
	c1 := rdCommit(t, root, "la rama se elige su review y reescribe su reviewer", map[string]string{
		"hoom.yaml": raYAML(raAflojada),
		raContrato:  raContratoRama,
		spec:        "# Spec x\n",
		"rama.go":   "package app\n\nfunc Rama() {}\n",
	})
	metaDeRun(t, root, "20260926T090000_wr1ter", "claude", "writer", time.Now().Add(-time.Minute))
	cx := raInstalar(t, bin, "codex", "")
	cl := raInstalar(t, bin, "claude", "")

	completa, out := rdRevisar(t, "CA-426", root, Options{Provider: "codex", Spec: spec})
	if completa.Status != "revisado" || completa.Cross != CrossYes || completa.Cobertura != CoberturaCompleta {
		t.Fatalf("CA-426/CA-423: la review sin rango con codex es cruzada y completa: %+v\n%s", completa, out)
	}
	rdEsperarSegundo()
	rdCommit(t, root, "codigo despues", map[string]string{"despues.go": "package app\n\nfunc Despues() {}\n"})

	for _, opt := range []Options{
		{Provider: "codex", Spec: spec, Delta: true},
		{Provider: "codex", Spec: spec, Desde: c1},
	} {
		antes := cx.veces()
		res, out := rdRevisar(t, "CA-426", root, opt)
		if res.Status != "revisado" || cx.veces() <= antes {
			t.Fatalf("CA-426: fixture: la review con rango corre con codex (%+v): %+v\n%s", opt, res, out)
		}
		for n := antes + 1; n <= cx.veces(); n++ {
			args := cx.argv(t, n)
			sistema := raSistema(t, "codex", args)
			if !raMismoTexto(sistema, raContratoBase) || strings.Contains(sistema, "CONTRATO-DE-LA-RAMA-CA419") {
				t.Fatalf("CA-426: con rango (%+v) el contrato del reviewer es el del merge-base, nunca el de la rama: %q", opt, sistema)
			}
			if !raTiene(args, "--ignore-user-config") || !res.Isolated {
				t.Fatalf("CA-426: con rango (%+v) isolated: false de la rama no vale: codex corre aislado: %v", opt, args)
			}
		}
	}

	for _, opt := range []Options{
		{Provider: "claude", Spec: spec, Delta: true},
		{Provider: "claude", Spec: spec, Desde: c1},
	} {
		registros := rdRegistrosEn(root)
		res, out := rdRevisar(t, "CA-426", root, opt)
		if res.Status != "no-entregable" || res.ExitCode != 1 || res.Cross != CrossNo || len(res.Passes) != 0 || cl.veces() != 0 {
			t.Fatalf("CA-426: con rango (%+v) same_provider: true de la rama no deja revisar con el provider que escribio: %+v\n%s", opt, res, out)
		}
		if !strings.Contains(out, "--same-provider") {
			t.Fatalf("CA-426: la negativa es la de siempre y nombra --same-provider:\n%s", out)
		}
		if rdRegistrosEn(root) != registros {
			t.Fatalf("CA-426: la negativa no escribe registro (%+v)", opt)
		}
	}
}
