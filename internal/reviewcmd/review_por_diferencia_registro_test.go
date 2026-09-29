// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-423, CA-424): lo que el registro, el Result y --json dicen de lo que
// se reviso (desde, hasta, cobertura, desde_review), y las lentes de una
// review con rango. Los fixtures estan en
// review_por_diferencia_helpers_test.go.
package reviewcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- CA-423

// rdCobertura exige que el Result, el registro leido con Records y el
// registro crudo digan lo mismo: desde, hasta (40 hex), cobertura y
// desde_review ("" = ausente del JSON o vacio).
func rdCobertura(t *testing.T, ca, caso, dir string, res Result, desde, hasta, cobertura, desdeReview string) {
	t.Helper()
	if !rdHex40(desde) || !rdHex40(hasta) {
		t.Fatalf("%s: %s: fixture: desde y hasta son shas de 40 hex: %q %q", ca, caso, desde, hasta)
	}
	if res.Desde != desde || res.Hasta != hasta || res.Cobertura != cobertura || res.DesdeReview != desdeReview {
		t.Fatalf("%s: %s: el Result dice desde %s, hasta %s, %s, desde_review %q: desde %q, hasta %q, cobertura %q, desde_review %q",
			ca, caso, desde[:12], hasta[:12], cobertura, desdeReview, res.Desde, res.Hasta, res.Cobertura, res.DesdeReview)
	}
	if res.RecordID == "" {
		t.Fatalf("%s: %s: la review deja registro: %+v", ca, caso, res)
	}
	r := rdRegistro(t, ca, dir, res.RecordID)
	if r.Desde != desde || r.Hasta != hasta || r.Cobertura != cobertura || r.DesdeReview != desdeReview {
		t.Fatalf("%s: %s: el registro dice lo mismo que el Result: %+v", ca, caso, r)
	}
	m := raRegistroCrudo(t, dir, res.RecordID)
	if m["desde"] != desde || m["hasta"] != hasta || m["cobertura"] != cobertura {
		t.Fatalf("%s: %s: el JSON del registro trae desde, hasta y cobertura: %v", ca, caso, m)
	}
	if v, ok := m["desde_review"]; desdeReview == "" && ok && v != "" {
		t.Fatalf("%s: %s: sin encadenar, el registro no nombra desde_review: %v", ca, caso, v)
	} else if desdeReview != "" && v != desdeReview {
		t.Fatalf("%s: %s: el JSON del registro trae desde_review %s: %v", ca, caso, desdeReview, v)
	}
	raw, _ := json.Marshal(res)
	var j map[string]any
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	if j["desde"] != desde || j["hasta"] != hasta || j["cobertura"] != cobertura {
		t.Fatalf("%s: %s: el JSON del Result (--json) trae desde, hasta y cobertura: %s", ca, caso, raw)
	}
	if desdeReview != "" && j["desde_review"] != desdeReview {
		t.Fatalf("%s: %s: el JSON del Result trae desde_review: %s", ca, caso, raw)
	}
}

// CA-423: "sin rango completa con desde = merge-base y hasta = HEAD". Una
// review sin --desde ni --delta, con main avanzado despues de abrir la rama
// (el merge-base no es la punta de main): desde es el merge-base, hasta el
// HEAD revisado, completa, sin desde_review. Con el registro, el Result y
// su JSON.
func TestCA423_SinRangoEsCompletaDelMergeBaseAHEAD(t *testing.T) {
	bin := raPATH(t)
	root, mb, _, b := rdRamaDos(t, "", ".hoom/specs/x.md")
	git(t, root, "checkout", "-q", "main")
	write(t, root, "main.go", "package app\n\nvar Main = 1\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "main avanza")
	git(t, root, "checkout", "-q", "feature")
	if rdSha(t, root, "main") == mb || rdMB(t, root) != mb {
		t.Fatal("CA-423: fixture: main avanzo y el merge-base sigue siendo el de la rama")
	}
	raInstalar(t, bin, "codex", "")
	res, out := rdRevisar(t, "CA-423", root, Options{Provider: "codex", Spec: ".hoom/specs/x.md"})
	if res.Status != "revisado" {
		t.Fatalf("CA-423: fixture: %+v\n%s", res, out)
	}
	rdCobertura(t, "CA-423", "sin rango", root, res, mb, b, CoberturaCompleta, "")
}

// CA-423: "--desde igual al merge-base da completa": por sha completo, por
// sha abreviado y por nombre de rama (main apunta al merge-base); desde queda
// el sha de 40 hex. "otro --desde da parcial", sin desde_review, tambien
// abreviado.
func TestCA423_DesdeDaCompletaOParcial(t *testing.T) {
	bin := raPATH(t)
	root, mb, a, b := rdRamaDos(t, "", "")
	raInstalar(t, bin, "codex", "")
	for _, c := range []string{mb, mb[:12], "main"} {
		res, out := rdRevisar(t, "CA-423", root, Options{Provider: "codex", Desde: c})
		if res.Status != "revisado" {
			t.Fatalf("CA-423: --desde %s revisa: %+v\n%s", c, res, out)
		}
		rdCobertura(t, "CA-423", "desde el merge-base ("+c+")", root, res, mb, b, CoberturaCompleta, "")
	}
	for _, c := range []string{a, a[:12]} {
		res, out := rdRevisar(t, "CA-423", root, Options{Provider: "codex", Desde: c})
		if res.Status != "revisado" {
			t.Fatalf("CA-423: --desde %s revisa: %+v\n%s", c, res, out)
		}
		rdCobertura(t, "CA-423", "desde un commit a mano ("+c+")", root, res, a, b, CoberturaParcial, "")
	}
}

// CA-423: "--delta o --desde igual a un hasta encadenable da delta con
// desde_review": despues de una completa en B y un commit C, --delta y
// --desde B (sha completo y abreviado) dan delta, desde B, hasta C,
// desde_review = el id de la completa.
func TestCA423_DeltaYDesdeIgualAUnHastaEncadenable(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root, _, _, b := rdRamaDos(t, "", spec)
	raInstalar(t, bin, "codex", "")
	completa, _ := rdRevisar(t, "CA-423", root, Options{Provider: "codex", Spec: spec})
	if completa.Cobertura != CoberturaCompleta {
		t.Fatalf("CA-423: la review sin rango es completa: %+v", completa)
	}
	rdEsperarSegundo()
	c := rdCommit(t, root, "C", map[string]string{"c.go": "package app\n\nfunc C() {}\n"})

	res, _ := rdRevisar(t, "CA-423", root, Options{Provider: "codex", Spec: spec, Delta: true})
	rdCobertura(t, "CA-423", "--delta", root, res, b, c, CoberturaDelta, completa.RecordID)
	for _, d := range []string{b, b[:12]} {
		res, _ = rdRevisar(t, "CA-423", root, Options{Provider: "codex", Spec: spec, Desde: d})
		rdCobertura(t, "CA-423", "--desde el hasta de la completa ("+d+")", root, res, b, c, CoberturaDelta, completa.RecordID)
	}
}

// CA-423: "una --lens a mano que no cubre las lentes de la regla da
// parcial". Un cambio en una ruta de riesgo (la regla pide las 4) revisado
// con --lens risk: parcial aunque el rango sea el completo. Un cambio chico
// (la regla pide la dominante) con --lens de la dominante: la cubre, sigue
// completa; con otra lente: parcial.
func TestCA423_LensAManoQueNoCubreLaReglaEsParcial(t *testing.T) {
	bin := raPATH(t)
	raInstalar(t, bin, "codex", "")

	root := raRepo(t, "")
	raCuatro(t, root)
	mb, head := rdMB(t, root), rdSha(t, root, "HEAD")
	res, _ := rdRevisar(t, "CA-423", root, Options{Provider: "codex", Lens: "risk"})
	rdCobertura(t, "CA-423", "4 lentes por regla, --lens risk", root, res, mb, head, CoberturaParcial, "")
	res, _ = rdRevisar(t, "CA-423", root, Options{Provider: "codex", Lens: "risk", Desde: mb})
	rdCobertura(t, "CA-423", "4 lentes por regla, --lens risk, --desde el merge-base", root, res, mb, head, CoberturaParcial, "")

	root = raRepo(t, "")
	raCambio(t, root)
	mb, head = rdMB(t, root), rdSha(t, root, "HEAD")
	res, _ = rdRevisar(t, "CA-423", root, Options{Provider: "codex", Lens: LenteDominante})
	rdCobertura(t, "CA-423", "la regla pide la dominante y --lens la cubre", root, res, mb, head, CoberturaCompleta, "")
	res, _ = rdRevisar(t, "CA-423", root, Options{Provider: "codex", Lens: "readability"})
	rdCobertura(t, "CA-423", "la regla pide la dominante y --lens pide otra", root, res, mb, head, CoberturaParcial, "")
}

// CA-423 (caso hostil): un delta cuya --lens a mano no cubre la regla queda
// parcial; y un parcial no sirve para encadenar: el --delta siguiente
// arranca en la completa de antes.
func TestCA423_DeltaConLensQueNoCubreEsParcialYNoEncadena(t *testing.T) {
	bin := raPATH(t)
	const spec = ".hoom/specs/x.md"
	root, _, _, b := rdRamaDos(t, "", spec)
	raInstalar(t, bin, "codex", "")
	completa, _ := rdRevisar(t, "CA-423", root, Options{Provider: "codex", Spec: spec})
	rdEsperarSegundo()
	rdCommit(t, root, "C", map[string]string{"c.go": "package app\n\nfunc C() {}\n"})
	res, out := rdRevisar(t, "CA-423", root, Options{Provider: "codex", Spec: spec, Delta: true, Lens: "readability"})
	if res.Cobertura != CoberturaParcial || res.Desde != b {
		t.Fatalf("CA-423: un --delta con una --lens que no cubre la regla es parcial (desde %s): %+v\n%s", b[:12], res, out)
	}
	rdEsperarSegundo()
	d := rdCommit(t, root, "D", map[string]string{"d.go": "package app\n\nfunc D() {}\n"})
	res, _ = rdRevisar(t, "CA-423", root, Options{Provider: "codex", Spec: spec, Delta: true})
	rdCobertura(t, "CA-423", "delta despues de un parcial", root, res, b, d, CoberturaDelta, completa.RecordID)
}

// ---------------------------------------------------------------- CA-424

// rdRamaGrandeYChica: la rama commitea primero un cambio grande (450 lineas,
// sin rutas de riesgo: la regla sobre el cambio entero pide las 4) y
// despues uno chico (menos de 400 lineas, sin rutas de riesgo: sobre el
// rango, la dominante). Devuelve el commit grande.
func rdRamaGrandeYChica(t *testing.T, root string) (grande string) {
	t.Helper()
	grande = rdCommit(t, root, "grande", map[string]string{"grande.go": rdCodigo(450, "Grande")})
	rdCommit(t, root, "chico", map[string]string{"chico.go": "package app\n\nfunc Chico() {}\n"})
	return grande
}

// CA-424: "un delta de menos de 400 lineas sin rutas de riesgo en una rama
// de mas de 400 lleva las 4": con --desde el commit grande y con --delta
// despues de una review completa, las 4 lentes, en orden, y el motivo es el
// de la regla sobre el cambio entero.
func TestCA424_DeltaChicoEnUnaRamaGrandeLlevaLasCuatro(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	grande := rdRamaGrandeYChica(t, root)
	cx := raInstalar(t, bin, "codex", "")
	res, out := rdRevisar(t, "CA-424", root, Options{Provider: "codex", Desde: grande})
	if !reflect.DeepEqual(res.Lenses, Lentes) || len(res.Passes) != 4 || cx.veces() != 4 {
		t.Fatalf("CA-424: el rango chico de una rama grande lleva las 4 lentes: %+v\n%s", res, out)
	}
	if !strings.Contains(res.Reason, " (sobre el cambio entero)") {
		t.Fatalf("CA-424: el motivo es el de la regla que gano, con ' (sobre el cambio entero)': %q", res.Reason)
	}
	if !strings.Contains(out, " (sobre el cambio entero)") {
		t.Fatalf("CA-424: la salida dice el motivo con ' (sobre el cambio entero)':\n%s", out)
	}

	// con --delta: completa primero (las 4), un commit chico, el delta
	const spec = ".hoom/specs/x.md"
	root = raRepo(t, "")
	rdRamaGrandeYChica(t, root)
	rdCommit(t, root, "spec", map[string]string{spec: "# x\n"})
	rdRevisar(t, "CA-424", root, Options{Provider: "codex", Spec: spec})
	rdEsperarSegundo()
	rdCommit(t, root, "otro chico", map[string]string{"otro.go": "package app\n\nfunc Otro() {}\n"})
	res, out = rdRevisar(t, "CA-424", root, Options{Provider: "codex", Spec: spec, Delta: true})
	if !reflect.DeepEqual(res.Lenses, Lentes) || res.Cobertura != CoberturaDelta {
		t.Fatalf("CA-424: un --delta chico de una rama grande lleva las 4 y queda delta: %+v\n%s", res, out)
	}
}

// CA-424: "un rango de mas de 400 lineas con el cambio entero vacio lleva
// las 4" (caso limite: "--desde en un arbol cuya base ya contiene el
// cambio: el cambio entero esta vacio, las lentes salen del rango, y la
// review queda parcial"). La base es una rama que ya contiene HEAD. Y con
// el cambio entero vacio, un rango chico lleva la dominante (la regla sobre
// el rango) y uno que toca una ruta de riesgo, las 4; siempre parcial y con
// el motivo ' (sobre el rango)'.
func TestCA424_RangoConElCambioEnteroVacio(t *testing.T) {
	casos := []struct {
		nombre  string
		rango   map[string]string
		lentes  []string
		pasadas int
	}{
		{"rango-grande", map[string]string{"grande.go": rdCodigo(450, "Grande")}, Lentes, 4},
		{"rango-chico", map[string]string{"chico.go": "package app\n\nfunc Chico() {}\n"}, []string{LenteDominante}, 1},
		{"rango-con-ruta-de-riesgo", map[string]string{"internal/auth/token.go": "package auth\n\nfunc Valida(s string) bool { return s != \"\" }\n"}, Lentes, 4},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			// la base del proyecto es la de base_branch de hoom.yaml (la que
			// usan Run y la CLI): integrada, que despues va a contener HEAD
			raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			write(t, root, "hoom.yaml", strings.Replace(string(raw), "base_branch: main\n", "base_branch: integrada\n", 1))
			git(t, root, "commit", "-q", "-am", "la base del proyecto es integrada")
			desde := rdSha(t, root, "main")
			head := rdCommit(t, root, "el rango", c.rango)
			git(t, root, "branch", "integrada", "HEAD")
			if rdGit(t, root, "merge-base", "integrada", "HEAD") != head || !strings.Contains(rdGit(t, root, "show", "HEAD:hoom.yaml"), "base_branch: integrada") {
				t.Fatal("CA-424: fixture: la base del proyecto (integrada) ya contiene HEAD: el cambio entero esta vacio")
			}
			cx := raInstalar(t, bin, "codex", "")
			raLimpio(t, "CA-424", root)
			res, err, out := raRunBaseConReloj(t, "CA-424", 60*time.Second, root, "integrada", Options{Provider: "codex", Desde: desde})
			if err != nil || res.Status != "revisado" {
				t.Fatalf("CA-424: %s: la review del rango corre aunque el cambio entero este vacio: %+v %v\n%s", c.nombre, res, err, out)
			}
			if !reflect.DeepEqual(res.Lenses, c.lentes) || cx.veces() != c.pasadas {
				t.Fatalf("CA-424: %s: las lentes salen del rango: %v, fueron %v (%d pasadas)\n%s", c.nombre, c.lentes, res.Lenses, cx.veces(), out)
			}
			if !strings.Contains(res.Reason, " (sobre el rango)") {
				t.Fatalf("CA-424: %s: el motivo es el de la regla sobre el rango: %q", c.nombre, res.Reason)
			}
			if res.Cobertura != CoberturaParcial || res.Desde != desde || res.Hasta != head || res.DesdeReview != "" {
				t.Fatalf("CA-424: %s: con la base que ya contiene el cambio la review queda parcial, de %s a %s: %+v", c.nombre, desde[:12], head[:12], res)
			}
		})
	}
}

// CA-424: "--lens a mano manda": un rango chico en una rama grande con
// --lens risk corre una sola pasada (risk), y la review queda parcial (no
// cubre las 4 que pide la regla).
func TestCA424_LensAManoMandaEnUnRango(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	grande := rdRamaGrandeYChica(t, root)
	cx := raInstalar(t, bin, "codex", "")
	res, out := rdRevisar(t, "CA-424", root, Options{Provider: "codex", Desde: grande, Lens: "risk"})
	if !reflect.DeepEqual(res.Lenses, []string{"risk"}) || len(res.Passes) != 1 || cx.veces() != 1 {
		t.Fatalf("CA-424: --lens a mano manda: una pasada de risk: %+v\n%s", res, out)
	}
	if res.Cobertura != CoberturaParcial {
		t.Fatalf("CA-424/CA-423: una --lens que no cubre las 4 de la regla deja la review parcial: %+v", res)
	}
}

// CA-424 (guarda de la medida): el rango se mide con las rutas de la
// evidencia. Un rango que solo toca .hoom/specs/ con 900 lineas y un .go
// chico: sobre el rango es un cambio chico (la dominante); el cambio entero
// (con la medida de hoy, que incluye .hoom/specs/) pasa de 400 y pide las 4.
// Gana la mas estricta: las 4, con el motivo sobre el cambio entero.
func TestCA424_LaMasEstrictaEntreRangoYCambioEntero(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	desde := rdCommit(t, root, "codigo chico", map[string]string{"uno.go": "package app\n\nfunc Uno() {}\n"})
	rdCommit(t, root, "spec grande y otro chico", map[string]string{
		".hoom/specs/x.md": strings.Repeat("- una linea del spec\n", 900),
		"dos.go":           "package app\n\nfunc Dos() {}\n",
	})
	cx := raInstalar(t, bin, "codex", "")
	res, out := rdRevisar(t, "CA-424", root, Options{Provider: "codex", Desde: desde})
	if !reflect.DeepEqual(res.Lenses, Lentes) || cx.veces() != 4 || !strings.Contains(res.Reason, " (sobre el cambio entero)") {
		t.Fatalf("CA-424: gana la regla mas estricta (el cambio entero, con .hoom/specs/, pasa de 400): %+v\n%s", res, out)
	}
}
