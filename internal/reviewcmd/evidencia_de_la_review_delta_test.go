// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md
// (CA-484, CA-485, CA-486, enmienda 1) sobre la review con rango: "la delta
// compara contra la punta anterior con la base integrada (un merge
// sintetico)". Sea D el desde de la review e I el merge-base de la base con
// HEAD (la base que la rama tiene integrada): si I es HEAD (un rango ya
// integrado) o ancestro de D, el rango es el de hoy; si no, se compara el
// arbol del merge sintetico de D con I contra HEAD, y base_integrada = I. Si
// ese merge tiene conflicto, la review es la completa de hoy con un aviso.
// Los fixtures estan en evidencia_de_la_review_helpers_test.go.
package reviewcmd

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// ---------------------------------------------------------------- CA-484

// edrConBaseIntegrada arma el caso del criterio: despues de la review R1
// (hasta D), la rama commitea X, main trae Y (450 lineas en una ruta de
// riesgo y un cambio en app.go), la rama integra main sin conflicto con un
// cambio propio en el commit de merge (evil.go y una linea mas en el app.go
// que trajo Y), y commitea Z. Devuelve el repo, R1, D, I (la punta de main) y
// H (HEAD).
func edrConBaseIntegrada(t *testing.T, ca string, cx *raCLI) (root string, r1 Result, d, i, h string) {
	t.Helper()
	root, r1, d = edrConReview(t, ca, cx)
	rdCommit(t, root, "X", map[string]string{"x.go": edrCodigo("MARCA-X")})
	i = edrMainAvanza(t, root, "Y: main avanza", map[string]string{
		"internal/auth/y_main.go": rdCodigo(450, "MarcaYMain"),
		"app.go":                  edrAppY,
	})
	edrIntegrar(t, root, map[string]string{"evil.go": edrCodigo("MARCA-EVIL"), "app.go": edrAppEvil})
	h = rdCommit(t, root, "Z", map[string]string{"z.go": edrCodigo("MARCA-Z")})
	if mb := rdMB(t, root); mb != i {
		t.Fatalf("%s: fixture: el merge-base de main con HEAD es la punta de main (%s): %s", ca, i[:12], mb)
	}
	return root, r1, d, i, h
}

const (
	edrAppY    = "package app\n\n// MARCA-Y-APP\n"
	edrAppEvil = edrAppY + "// MARCA-EVIL-EN-APP\n"
)

// edrTamanoXZ es la medida de lo nuevo del caso: x.go, evil.go, la linea
// del merge en app.go y z.go.
func edrTamanoXZ() (n, ins, del int) {
	return 4, 3*edrLineasDe(edrCodigo("MARCA-X")) + 1, 0
}

// edrSoloLoNuevo exige que la evidencia del pedido sea lo que la rama hizo
// despues de D sin lo que trajo la base: las secciones de app.go, evil.go,
// x.go y z.go; en app.go solo la linea del merge; nada de y_main.go.
func edrSoloLoNuevo(t *testing.T, ca, caso, diff string) {
	t.Helper()
	if got, quiero := edrCabeceras(diff), []string{"app.go", "evil.go", "x.go", "z.go"}; !reflect.DeepEqual(got, quiero) {
		t.Fatalf("%s: %s: la evidencia trae X, Z y el cambio del commit de merge, sin lo que trajo la base: secciones %q, quiero %q\n%s",
			ca, caso, got, quiero, diff)
	}
	for _, marca := range []string{"+// MARCA-X", "+// MARCA-Z", "+// MARCA-EVIL"} {
		if !strings.Contains(diff, marca) {
			t.Fatalf("%s: %s: la evidencia trae %q:\n%s", ca, caso, marca, diff)
		}
	}
	if got := edrAgregadas(diff, "app.go"); !reflect.DeepEqual(got, []string{"+// MARCA-EVIL-EN-APP"}) {
		t.Fatalf("%s: %s: de app.go va solo lo que el commit de merge agrego mas alla del merge limpio (+// MARCA-EVIL-EN-APP): %q\n%s", ca, caso, got, diff)
	}
	if strings.Contains(diff, "MarcaYMain") || strings.Contains(diff, "y_main.go") || strings.Contains(diff, "-// MARCA-Y-APP") {
		t.Fatalf("%s: %s: lo que trajo la base (Y) no esta en la evidencia:\n%s", ca, caso, diff)
	}
}

// CA-484: "si despues de una review la rama commitea X, integra la base sin
// conflicto (eso trae Y) y commitea Z, `hoom review --delta` revisa X y Z sin
// Y". La medida y las lentes sobre el rango son las de X+Z (Y son 450 lineas
// en internal/auth: con Y serian las 4 lentes; sin Y, la dominante). El
// registro, el Result y su JSON (el de --json) dicen cobertura delta, desde =
// el hasta anterior, su desde_review y base_integrada = I (40 hex). La salida
// lleva la linea base despues de la linea rango, y el pedido la linea de la
// base integrada despues de Rango:. Lo que el commit de merge agrego mas alla
// del merge limpio si aparece.
func TestCA484_DeltaRevisaXYZSinLoQueTrajoLaBase(t *testing.T) {
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")
	root, r1, d, i, h := edrConBaseIntegrada(t, "CA-484", cx)
	antes := cx.veces()

	res, out := edrRevisar(t, "CA-484", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
	if res.Status != "revisado" || res.RecordID == "" {
		t.Fatalf("CA-484: --delta revisa: %+v\n%s", res, out)
	}
	if res.Cobertura != CoberturaDelta || res.Desde != d || res.Hasta != h || res.DesdeReview != r1.RecordID {
		t.Fatalf("CA-484: el Result dice cobertura delta, desde %s (el hasta anterior), hasta %s y desde_review %s: %+v",
			d[:12], h[:12], r1.RecordID, res)
	}
	if !reflect.DeepEqual(res.Lenses, []string{LenteDominante}) || len(res.Passes) != 1 || cx.veces() != antes+1 {
		t.Fatalf("CA-484: las lentes sobre el rango son las de X+Z (la dominante); con lo que trajo la base serian las 4: %v (%q)\n%s",
			res.Lenses, res.Reason, out)
	}
	if m := edrJSON(t, res); m["base_integrada"] != i {
		t.Fatalf("CA-484: el Result (--json) dice base_integrada %s (40 hex): %v", i, m["base_integrada"])
	}
	crudo := raRegistroCrudo(t, root, res.RecordID)
	if crudo["base_integrada"] != i || crudo["cobertura"] != CoberturaDelta || crudo["desde"] != d || crudo["desde_review"] != r1.RecordID {
		t.Fatalf("CA-484: el registro dice base_integrada %s, delta, desde %s, desde_review %s: %v", i[:12], d[:12], r1.RecordID, crudo)
	}

	n, ins, del := edrTamanoXZ()
	lineas := raLineas(out)
	if primera := fmt.Sprintf("hoom review: %d archivos cambiados, +%d/-%d lineas desde %s", n, ins, del, d[:12]); lineas[0] != primera {
		t.Fatalf("CA-484: la medida del rango es la de X+Z: la primera linea es\n%q\nfue\n%q", primera, lineas[0])
	}
	rango := rdLineaDeRango(t, "CA-484", out)
	if quiero := "  rango       " + d[:12] + ".." + h[:12] + " - delta de la review " + r1.RecordID; rango != quiero {
		t.Fatalf("CA-484: la linea rango es la de hoy %q, fue %q", quiero, rango)
	}
	k := raLinea(lineas, 0, "  rango ")
	if k+1 >= len(lineas) || lineas[k+1] != edrLineaBase(i, d) {
		t.Fatalf("CA-484: despues de la linea rango va\n%q\n:\n%s", edrLineaBase(i, d), out)
	}

	p := cx.pedido(t, antes+1)
	pl := raLineas(p)
	if !regexp.MustCompile(`^Revisa lo que cambio en esta rama desde ` + d[:12] + ` hasta ` + h[:12] + `\. `).MatchString(pl[0]) {
		t.Fatalf("CA-484: la primera linea del pedido es la de un rango de %s a %s: %q", d[:12], h[:12], pl[0])
	}
	if pl[2] != rdLineaRango(d, h, CoberturaDelta, n, ins, del) {
		t.Fatalf("CA-484: la linea Rango: del pedido mide X+Z:\n%q\nfue\n%q", rdLineaRango(d, h, CoberturaDelta, n, ins, del), pl[2])
	}
	if pl[3] != edrLineaPedido(i, d) {
		t.Fatalf("CA-484: despues de Rango: va la linea de la base integrada\n%q\nfue\n%q", edrLineaPedido(i, d), pl[3])
	}
	if raCuenta(p, "Lo anterior a "+d[:12]+" ya lo reviso la review "+r1.RecordID+": "+rdIntroducido) != 1 {
		t.Fatalf("CA-484: el pedido conserva la linea de INTRODUCIDO del delta:\n%s", p)
	}
	diff, sha := edrDiffDelPedido(t, "CA-484", p)
	if sha != res.EvidenceSHA256 {
		t.Fatalf("CA-484: los marcadores llevan el sha256 de la evidencia del Result: %s vs %s", sha, res.EvidenceSHA256)
	}
	edrSoloLoNuevo(t, "CA-484", "--delta", diff)

	// --desde D en el mismo caso: la misma evidencia, sin Y
	rdEsperarSegundo()
	antes = cx.veces()
	res2, out2 := edrRevisar(t, "CA-484", root, Options{Provider: "codex", Spec: edrSpec, Desde: d})
	if res2.Status != "revisado" || cx.veces() != antes+1 {
		t.Fatalf("CA-484: --desde %s revisa: %+v\n%s", d[:12], res2, out2)
	}
	if res2.Cobertura != CoberturaParcial && res2.Cobertura != CoberturaDelta {
		t.Fatalf("CA-484: con --desde la cobertura sale como hoy (parcial o delta): %q", res2.Cobertura)
	}
	if m := edrJSON(t, res2); m["base_integrada"] != i {
		t.Fatalf("CA-484: --desde %s con la base integrada despues: base_integrada %s: %v", d[:12], i[:12], m["base_integrada"])
	}
	l2 := raLineas(out2)
	if k := raLinea(l2, 0, "  rango "); k < 0 || k+1 >= len(l2) || l2[k+1] != edrLineaBase(i, d) {
		t.Fatalf("CA-484: --desde: despues de la linea rango va la linea base:\n%s", out2)
	}
	p2 := cx.pedido(t, antes+1)
	if raLineas(p2)[3] != edrLineaPedido(i, d) {
		t.Fatalf("CA-484: --desde: despues de Rango: va la linea de la base integrada:\n%s", p2)
	}
	diff2, _ := edrDiffDelPedido(t, "CA-484", p2)
	edrSoloLoNuevo(t, "CA-484", "--desde", diff2)
	if diff2 != diff {
		t.Fatalf("CA-484: --desde D y --delta desde D revisan lo mismo:\n--- delta\n%s\n--- desde\n%s", diff, diff2)
	}
}

// CA-484 (caso limite: "dos merges de la base despues de la review: I es el
// ultimo merge-base, y un solo merge sintetico lo cubre"). X, merge (Y1), Z,
// merge (Y2), W: --delta revisa X, Z y W, sin Y1 ni Y2, y base_integrada es la
// punta de main.
func TestCA484_DosMergesDeLaBaseDespuesDeLaReview(t *testing.T) {
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")
	root, r1, d := edrConReview(t, "CA-484", cx)
	rdCommit(t, root, "X", map[string]string{"x.go": edrCodigo("MARCA-X")})
	edrMainAvanza(t, root, "Y1", map[string]string{"y1.go": edrCodigo("MARCA-Y1")})
	edrIntegrar(t, root, nil)
	rdCommit(t, root, "Z", map[string]string{"z.go": edrCodigo("MARCA-Z")})
	i := edrMainAvanza(t, root, "Y2", map[string]string{"y2.go": edrCodigo("MARCA-Y2"), "y1.go": edrCodigo("MARCA-Y1-BIS")})
	edrIntegrar(t, root, nil)
	h := rdCommit(t, root, "W", map[string]string{"w.go": edrCodigo("MARCA-W")})
	antes := cx.veces()

	res, out := edrRevisar(t, "CA-484", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
	if res.Status != "revisado" || res.Cobertura != CoberturaDelta || res.Desde != d || res.Hasta != h || res.DesdeReview != r1.RecordID {
		t.Fatalf("CA-484: --delta con dos merges de la base es un delta de %s a %s: %+v\n%s", d[:12], h[:12], res, out)
	}
	if m := edrJSON(t, res); m["base_integrada"] != i {
		t.Fatalf("CA-484: base_integrada es el ultimo merge-base (%s): %v", i[:12], m["base_integrada"])
	}
	diff, _ := edrDiffDelPedido(t, "CA-484", cx.pedido(t, antes+1))
	if got := edrCabeceras(diff); !reflect.DeepEqual(got, []string{"w.go", "x.go", "z.go"}) || strings.Contains(diff, "MARCA-Y") {
		t.Fatalf("CA-484: con dos merges de la base la evidencia es X, Z y W, sin Y1 ni Y2: %q\n%s", got, diff)
	}
	lineas := raLineas(out)
	if k := raLinea(lineas, 0, "  rango "); k < 0 || k+1 >= len(lineas) || lineas[k+1] != edrLineaBase(i, d) {
		t.Fatalf("CA-484: la linea base nombra el ultimo merge-base:\n%s", out)
	}
}

// CA-484 (controles, casos limite): la delta de hoy, sin base_integrada ni
// lineas nuevas, cuando no hay nada que descontar: sin merges de la base en
// el rango; con la base integrada ANTES de la review anterior (I es
// ancestro de D); con un merge de una rama que no es la base (lo que trae es
// parte de la tarea); y con --desde en un arbol cuya base ya contiene HEAD
// (un rango ya integrado: enmienda 1, CA-424). La evidencia es la de hoy:
// la de Evidence contra una rama en D.
func TestCA484_SinNadaQueDescontarLaDeltaEsLaDeHoy(t *testing.T) {
	casos := []struct {
		nombre string
		// armar deja la rama lista despues de la review R1 (hasta D) y
		// devuelve las opciones de la review con rango y la base de Run.
		armar  func(t *testing.T, root, d string) (Options, string)
		marcas []string // lo que la evidencia trae
	}{
		{"sin-merges-de-la-base", func(t *testing.T, root, d string) (Options, string) {
			rdCommit(t, root, "X", map[string]string{"x.go": edrCodigo("MARCA-X")})
			edrMainAvanza(t, root, "main avanza sin que la rama lo integre", map[string]string{"y.go": edrCodigo("MARCA-Y-SIN-INTEGRAR")})
			return Options{Provider: "codex", Spec: edrSpec, Delta: true}, "main"
		}, []string{"+// MARCA-X"}},
		{"merge-de-una-rama-que-no-es-la-base", func(t *testing.T, root, d string) (Options, string) {
			git(t, root, "checkout", "-q", "-b", "otra", d)
			rdCommit(t, root, "O", map[string]string{"o.go": edrCodigo("MARCA-O")})
			git(t, root, "checkout", "-q", "feature")
			rdCommit(t, root, "X", map[string]string{"x.go": edrCodigo("MARCA-X")})
			git(t, root, "merge", "-q", "--no-ff", "--no-edit", "otra")
			return Options{Provider: "codex", Spec: edrSpec, Delta: true}, "main"
		}, []string{"+// MARCA-X", "+// MARCA-O"}},
		{"desde-en-un-rango-ya-integrado", func(t *testing.T, root, d string) (Options, string) {
			rdCommit(t, root, "X", map[string]string{"x.go": edrCodigo("MARCA-X")})
			git(t, root, "branch", "-q", "integrada", "HEAD")
			return Options{Provider: "codex", Spec: edrSpec, Desde: d}, "integrada"
		}, []string{"+// MARCA-X"}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, _, d := edrConReview(t, "CA-484", cx)
			opt, base := c.armar(t, root, d)
			ev := rdOraculo(t, "CA-484", root, d, edrSpec)
			antes := cx.veces()
			edrLimpio(t, "CA-484", root)
			res, err, out := raRunBaseConReloj(t, "CA-484", 60*time.Second, root, base, opt)
			if err != nil || res.Status != "revisado" || cx.veces() <= antes {
				t.Fatalf("CA-484: %s: la review con rango revisa: %+v %v\n%s", c.nombre, res, err, out)
			}
			edrSinClave(t, "CA-484", c.nombre+": el Result (--json)", edrJSON(t, res), "base_integrada")
			edrSinClave(t, "CA-484", c.nombre+": el registro", raRegistroCrudo(t, root, res.RecordID), "base_integrada")
			if raLinea(raLineas(out), 0, "  base ") >= 0 || strings.Contains(out, "no se separa") {
				t.Fatalf("CA-484: %s: sin nada que descontar no hay linea base ni aviso:\n%s", c.nombre, out)
			}
			p := cx.pedido(t, antes+1)
			if strings.Contains(p, "La rama integro la base") {
				t.Fatalf("CA-484: %s: sin nada que descontar el pedido no lleva la linea de la base integrada:\n%s", c.nombre, p)
			}
			if !strings.Contains(p, raBloque(t, ev, edrSpec, true)) {
				t.Fatalf("CA-484: %s: la evidencia es la de hoy, de %s a HEAD:\n%s", c.nombre, d[:12], p)
			}
			for _, m := range c.marcas {
				if !strings.Contains(string(ev.Diff), m) {
					t.Fatalf("CA-484: %s: fixture: la evidencia de hoy trae %q", c.nombre, m)
				}
			}
		})
	}

	// la base integrada antes de la review anterior: I es ancestro de D
	t.Run("base-integrada-antes-de-la-review", func(t *testing.T) {
		bin := raPATH(t)
		cx := raInstalar(t, bin, "codex", "")
		root := rdDeltaRepo(t)
		edrMainAvanza(t, root, "Y0", map[string]string{"y0.go": edrCodigo("MARCA-Y0")})
		edrIntegrar(t, root, nil)
		r1, out := edrRevisar(t, "CA-484", root, Options{Provider: "codex", Spec: edrSpec})
		if r1.Status != "revisado" || r1.Cobertura != CoberturaCompleta {
			t.Fatalf("CA-484: fixture: la review completa corre: %+v\n%s", r1, out)
		}
		d := rdSha(t, root, "HEAD")
		rdEsperarSegundo()
		rdCommit(t, root, "X", map[string]string{"x.go": edrCodigo("MARCA-X")})
		ev := rdOraculo(t, "CA-484", root, d, edrSpec)
		antes := cx.veces()
		res, out := edrRevisar(t, "CA-484", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
		if res.Status != "revisado" || res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID {
			t.Fatalf("CA-484: la delta de hoy: %+v\n%s", res, out)
		}
		edrSinClave(t, "CA-484", "base integrada antes: el Result (--json)", edrJSON(t, res), "base_integrada")
		if raLinea(raLineas(out), 0, "  base ") >= 0 || !strings.Contains(cx.pedido(t, antes+1), raBloque(t, ev, edrSpec, true)) ||
			strings.Contains(cx.pedido(t, antes+1), "La rama integro la base") {
			t.Fatalf("CA-484: con la base integrada antes de la review anterior la delta es la de hoy:\n%s\n%s", out, cx.pedido(t, antes+1))
		}
	})
}

// CA-484, de punta a punta: `hoom review --delta --json` en el caso del
// criterio trae base_integrada (40 hex) junto a desde, cobertura y
// desde_review.
func TestCA484_E2EJSONConBaseIntegrada(t *testing.T) {
	hoom := hbHoomReal(t)
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")
	root, r1, d, i, h := edrConBaseIntegrada(t, "CA-484", cx)
	edrLimpio(t, "CA-484", root)
	code, out, errOut := raHoom(t, hoom, root, "review", "--provider", "codex", "--spec", edrSpec, "--delta", "--json")
	m := map[string]any{}
	if err := json.Unmarshal([]byte(out), &m); err != nil || code != 0 {
		t.Fatalf("CA-484: --json: exit 0 y JSON en stdout (exit %d, %v):\n%s\n%s", code, err, out, errOut)
	}
	if m["base_integrada"] != i || m["desde"] != d || m["hasta"] != h || m["cobertura"] != CoberturaDelta || m["desde_review"] != r1.RecordID {
		t.Fatalf("CA-484: --json --delta: base_integrada %s, desde %s, hasta %s, delta, desde_review %s: %v", i[:12], d[:12], h[:12], r1.RecordID, m)
	}
}

// ---------------------------------------------------------------- CA-485

// CA-485: "si despues de una review la rama solo integro la base sin
// conflicto, `hoom review --delta` termina `SIN REVISAR - no hay cambios
// desde <desde12>: no hay nada que revisar`, con exit 0 y sin registro". Con
// lo que trajo la base siendo codigo en una ruta de riesgo, documentacion
// (no es "solo documentacion": lo nuevo esta vacio) o dos merges seguidos.
func TestCA485_SoloIntegrarLaBaseEsSinRevisar(t *testing.T) {
	casos := []struct {
		nombre string
		traer  func(t *testing.T, root string)
	}{
		{"codigo-en-una-ruta-de-riesgo", func(t *testing.T, root string) {
			edrMainAvanza(t, root, "Y", map[string]string{"internal/auth/y.go": rdCodigo(450, "MarcaY")})
			edrIntegrar(t, root, nil)
		}},
		{"documentacion", func(t *testing.T, root string) {
			edrMainAvanza(t, root, "Y docs", map[string]string{"README.md": "# demo\n\nprosa de main\n", "docs/guia.md": "# guia\n"})
			edrIntegrar(t, root, nil)
		}},
		{"dos-merges", func(t *testing.T, root string) {
			edrMainAvanza(t, root, "Y1", map[string]string{"y1.go": edrCodigo("MARCA-Y1")})
			edrIntegrar(t, root, nil)
			edrMainAvanza(t, root, "Y2", map[string]string{"y2.go": edrCodigo("MARCA-Y2")})
			edrIntegrar(t, root, nil)
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, _, d := edrConReview(t, "CA-485", cx)
			c.traer(t, root)
			edrLimpio(t, "CA-485", root)
			antes, registros := cx.veces(), rdRegistrosEn(root)
			res, err, out := rdCorrer(t, "CA-485", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
			rdSinRevisar(t, "CA-485", c.nombre, root, cx, antes, registros, res, err, out, rdSinCambios(d[:12]))
		})
	}
}

// ---------------------------------------------------------------- CA-486

// edrConConflicto arma el caso: la rama cambia app.go (lo que la review R1
// cubre: D lo trae), main cambia despues la misma linea (Y), la rama integra
// main resolviendo el conflicto en app.go, y commitea Z. El merge sintetico
// de D con I choca en app.go como choco el merge de la rama. Devuelve el
// repo, R1, D, I y H.
func edrConConflicto(t *testing.T, ca string, cx *raCLI) (root string, r1 Result, d, i, h string) {
	t.Helper()
	root, r1, d = edrConReviewTras(t, ca, cx, map[string]string{"app.go": "package app\n\nfunc Rama() {} // DE-LA-RAMA\n"})
	i = edrMainAvanza(t, root, "Y", map[string]string{"app.go": "package app\n\nfunc Main() {} // DE-MAIN\n", "y.go": edrCodigo("MARCA-Y")})
	edrIntegrarConConflicto(t, root, map[string]string{"app.go": "package app\n\nfunc Rama() {} // DE-LA-RAMA\nfunc Main() {} // DE-MAIN\n// MARCA-RESOLUCION\n"})
	h = rdCommit(t, root, "Z", map[string]string{"z.go": edrCodigo("MARCA-Z")})
	if mb := rdMB(t, root); mb != i {
		t.Fatalf("%s: fixture: el merge-base de main con HEAD es la punta de main: %s", ca, mb)
	}
	return root, r1, d, i, h
}

// edrCompleta exige que la review con rango que no se separo sea la
// completa de hoy, con el aviso: desde = I, cobertura completa, sin
// desde_review ni base_integrada (Result, JSON y registro), la primera linea
// `contra main` con la medida del cambio entero, la evidencia y las lentes de
// la review sin rango, el pedido sin rango, y el aviso en la salida y en las
// notas del Result y del registro.
func edrCompleta(t *testing.T, ca, caso, root string, cx *raCLI, antes int, res Result, out, d, i, h, motivo string) {
	t.Helper()
	g := gitx.Snapshot(root, "main")
	ev := raEvidenciaCruda(t, ca, root, edrSpec)
	if res.Status != "revisado" || res.RecordID == "" {
		t.Fatalf("%s: %s: la review completa revisa: %+v\n%s", ca, caso, res, out)
	}
	if res.Cobertura != CoberturaCompleta || res.Desde != i || res.Hasta != h || res.DesdeReview != "" {
		t.Fatalf("%s: %s: no se separa: la review es completa, desde %s (el merge-base), hasta %s, sin desde_review: %+v", ca, caso, i[:12], h[:12], res)
	}
	if lentes, _, err := Lenses(g, ""); err != nil || !reflect.DeepEqual(res.Lenses, lentes) {
		t.Fatalf("%s: %s: las lentes son las de la review completa (%v, %v): %v", ca, caso, lentes, err, res.Lenses)
	}
	m := edrJSON(t, res)
	edrSinClave(t, ca, caso+": el Result (--json)", m, "base_integrada")
	edrSinClave(t, ca, caso+": el Result (--json)", m, "desde_review")
	crudo := raRegistroCrudo(t, root, res.RecordID)
	edrSinClave(t, ca, caso+": el registro", crudo, "base_integrada")
	edrSinClave(t, ca, caso+": el registro", crudo, "desde_review")
	if crudo["cobertura"] != CoberturaCompleta || crudo["desde"] != i {
		t.Fatalf("%s: %s: el registro dice completa desde %s: %v", ca, caso, i[:12], crudo)
	}
	lineas := raLineas(out)
	if primera := fmt.Sprintf("hoom review: %d archivos cambiados, +%d/-%d lineas contra main", len(g.ChangedFiles), g.Insertions, g.Deletions); lineas[0] != primera {
		t.Fatalf("%s: %s: la primera linea es la de la review completa\n%q\nfue\n%q", ca, caso, primera, lineas[0])
	}
	if raLinea(lineas, 0, "  base ") >= 0 {
		t.Fatalf("%s: %s: sin base descontada no hay linea base:\n%s", ca, caso, out)
	}
	aviso := edrAviso(i, d, motivo)
	if raCuenta(out, "  aviso: "+aviso) != 1 {
		t.Fatalf("%s: %s: la salida lleva la linea\n%q\n:\n%s", ca, caso, "  aviso: "+aviso, out)
	}
	if _, ok := hbNota(res.Notes, aviso); !ok {
		t.Fatalf("%s: %s: las notas del Result llevan el aviso %q: %q", ca, caso, aviso, res.Notes)
	}
	if _, ok := hbNota(rdRegistro(t, ca, root, res.RecordID).Notes, aviso); !ok {
		t.Fatalf("%s: %s: las notas del registro llevan el aviso %q", ca, caso, aviso)
	}
	p := cx.pedido(t, antes+1)
	pl := raLineas(p)
	if !strings.HasPrefix(pl[0], "Revisa el cambio de esta rama. ") || !strings.HasPrefix(pl[2], "Base: main. ") ||
		raLinea(pl, 0, "Rango:") >= 0 || strings.Contains(p, "La rama integro la base") {
		t.Fatalf("%s: %s: el pedido es el de la review completa (sin Rango: ni base integrada):\n%s", ca, caso, p)
	}
	if !strings.Contains(p, raBloque(t, ev, edrSpec, true)) || res.EvidenceSHA256 != ev.SHA256 {
		t.Fatalf("%s: %s: la evidencia es la del cambio entero (Evidence contra main):\n%s", ca, caso, p)
	}
}

// CA-486: "si despues de una review la rama integro la base resolviendo un
// conflicto, `hoom review --delta` hace la review completa": desde = el
// merge-base, cobertura completa, sin desde_review ni base_integrada, primera
// linea `contra main`, y el aviso `la rama integro la base (<I12>) despues
// de <D12> y ese merge no se separa (conflicto en app.go): la review es
// completa` en la salida y en notes. Con --desde D, lo mismo.
func TestCA486_ConflictoAlIntegrarLaBaseEsLaReviewCompleta(t *testing.T) {
	for _, opt := range []Options{
		{Provider: "codex", Spec: edrSpec, Delta: true},
		{Provider: "codex", Spec: edrSpec, Desde: "D"},
	} {
		caso := "--delta"
		if opt.Desde != "" {
			caso = "--desde"
		}
		t.Run(caso, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, _, d, i, h := edrConConflicto(t, "CA-486", cx)
			if opt.Desde != "" {
				opt.Desde = d
			}
			antes := cx.veces()
			res, out := edrRevisar(t, "CA-486", root, opt)
			edrCompleta(t, "CA-486", caso, root, cx, antes, res, out, d, i, h, edrMotivoConflicto("app.go"))
		})
	}
}

// CA-486 (caso limite: "una ruta conflictiva con caracteres de control: el
// aviso la escapa"). El conflicto es en un archivo cuyo nombre trae un salto
// de linea, una linea falsa de la cabecera y un escape de terminal: el aviso
// es una sola linea, sin caracteres de control, con la ruta visible
// (escapada); ninguna linea de la salida empieza con la linea falsa, y las
// notas no llevan controles.
func TestCA486_LaRutaDelConflictoVaEscapadaEnElAviso(t *testing.T) {
	const falsa = "  lentes      ninguna - FALSO"
	ruta := "conflicto\n" + falsa + "\x1b.txt"
	bin := raPATH(t)
	raInstalar(t, bin, "codex", "")
	root := raRepo(t, "")
	write(t, root, ruta, "linea base\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "la base trae el archivo de nombre hostil")
	rdCommit(t, root, "A", map[string]string{edrSpec: "# Spec x\n\n- CA-1: algo.\n", "antes.go": edrCodigo("MARCA-DE-ANTES"),
		ruta: "linea de la rama\n"})
	r1, out := edrRevisar(t, "CA-486", root, Options{Provider: "codex", Spec: edrSpec})
	if r1.Status != "revisado" {
		t.Fatalf("CA-486: fixture: la primera review revisa: %+v\n%s", r1, out)
	}
	d := rdSha(t, root, "HEAD")
	rdEsperarSegundo()
	i := edrMainAvanza(t, root, "Y", map[string]string{ruta: "linea de main\n"})
	edrIntegrarConConflicto(t, root, map[string]string{ruta: "linea de la rama\nlinea de main\n"})
	rdCommit(t, root, "Z", map[string]string{"z.go": edrCodigo("MARCA-Z")})

	res, out := edrRevisar(t, "CA-486", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
	if res.Status != "revisado" || res.Cobertura != CoberturaCompleta || res.Desde != i {
		t.Fatalf("CA-486: con conflicto la review es completa desde %s: %+v\n%s", i[:12], res, out)
	}
	pre := "la rama integro la base (" + i[:12] + ") despues de " + d[:12] + " y ese merge no se separa (conflicto en "
	const pos = "): la review es completa"
	aviso, ok := hbAviso(out, pre)
	if !ok {
		t.Fatalf("CA-486: la salida lleva el aviso %q... en una sola linea:\n%s", pre, out)
	}
	aviso = strings.TrimRight(aviso, " ")
	if !strings.HasSuffix(aviso, pos) {
		t.Fatalf("CA-486: el aviso es una sola linea que cierra con %q: %q\n%s", pos, aviso, out)
	}
	mostrada := aviso[strings.Index(aviso, pre)+len(pre) : len(aviso)-len(pos)]
	if !bdSinControles(aviso) || mostrada == ruta || !strings.Contains(mostrada, "FALSO") || !strings.Contains(mostrada, "conflicto") {
		t.Fatalf("CA-486: la ruta va escapada (visible, sin controles): %q", mostrada)
	}
	for _, l := range raLineas(out) {
		if strings.HasPrefix(l, falsa) {
			t.Fatalf("CA-486: la ruta del conflicto no mete lineas en la salida: %q\n%s", l, out)
		}
	}
	if strings.ContainsAny(out, "\x1b\r") {
		t.Fatalf("CA-486: la salida no lleva ESC ni retorno de carro de la ruta: %q", out)
	}
	for _, notas := range [][]string{res.Notes, rdRegistro(t, "CA-486", root, res.RecordID).Notes} {
		if _, ok := hbNota(notas, pre); !ok {
			t.Fatalf("CA-486: las notas llevan el aviso: %q", notas)
		}
		for _, n := range notas {
			if !bdSinControles(n) {
				t.Fatalf("CA-486: ninguna nota lleva caracteres de control: %q", n)
			}
		}
	}
}

// CA-484 (caso limite: "--lens a mano: vale; la cobertura sale de la regla de
// hoy"). En el caso del criterio, la regla sobre lo nuevo (X+Z) es la lente
// dominante: --delta --lens reliability la cubre (delta, con base_integrada
// y la evidencia sin Y); --delta --lens risk no (parcial, con base_integrada
// igual).
func TestCA484_LensAManoConLaBaseIntegrada(t *testing.T) {
	for _, c := range []struct{ lente, cobertura string }{
		{LenteDominante, CoberturaDelta},
		{"risk", CoberturaParcial},
	} {
		t.Run(c.lente, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, _, d, i, _ := edrConBaseIntegrada(t, "CA-484", cx)
			antes := cx.veces()
			res, out := edrRevisar(t, "CA-484", root, Options{Provider: "codex", Spec: edrSpec, Delta: true, Lens: c.lente})
			if res.Status != "revisado" || res.Cobertura != c.cobertura || res.Desde != d || cx.veces() != antes+1 {
				t.Fatalf("CA-484: --delta --lens %s: la cobertura sale de la regla de hoy sobre lo nuevo (%s): %+v\n%s", c.lente, c.cobertura, res, out)
			}
			if m := edrJSON(t, res); m["base_integrada"] != i {
				t.Fatalf("CA-484: --delta --lens %s: base_integrada %s: %v", c.lente, i[:12], m["base_integrada"])
			}
			diff, _ := edrDiffDelPedido(t, "CA-484", cx.pedido(t, antes+1))
			edrSoloLoNuevo(t, "CA-484", "--lens "+c.lente, diff)
		})
	}
}
