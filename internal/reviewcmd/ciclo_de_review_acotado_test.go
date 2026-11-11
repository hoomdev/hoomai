// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// `hoom review` (CA-619, CA-620, CA-621): las lentes segun el riesgo de la
// tarjeta (LentesConRiesgo), la delta sin readability y el riesgo en el
// registro, el Result, --json y la salida. Los CLIs de IA son falsos
// (raPATH + raInstalar): ningun claude ni codex real. Escritos desde el spec
// y las firmas publicas, sin leer la implementacion.
package reviewcmd

import (
	"encoding/json"
	"math/rand"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/gitx"
)

var crTres = []string{"risk", "reliability", "resilience"}

const (
	crMotBajo = "riesgo bajo: la lente dominante"
	crMotAlto = "riesgo alto: las 4 lentes"
	crSufijo  = "; una delta no corre readability"
)

// crItem escribe el item de slug en el arbol raiz con el riesgo dado ("" =
// sin la clave).
func crItem(t *testing.T, root, slug, riesgo string) {
	t.Helper()
	body := "titulo: Tarea " + slug + "\ntipo: feature\nprioridad: media\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-20T10:00:00Z\n"
	if riesgo != "" {
		body += "riesgo: " + riesgo + "\n"
	}
	write(t, root, filepath.Join(".hoom", "items", slug+".yaml"), body)
}

// CA-619: LentesConRiesgo con bajo, normal y alto, con el motivo de la tabla.
func TestCA619_LentesConRiesgoTabla(t *testing.T) {
	grande := gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 450}
	chico := gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 10}
	riesgosa := gitx.Info{ChangedFiles: []string{"a.go", "internal/auth/login.go"}, Insertions: 10}

	l, m, err := LentesConRiesgo(grande, "", "bajo")
	if err != nil || !reflect.DeepEqual(l, []string{"reliability"}) || m != crMotBajo {
		t.Fatalf("CA-619: bajo y mas de 400 lineas: reliability, %q: %v %q %v", crMotBajo, l, m, err)
	}
	l, m, err = LentesConRiesgo(chico, "", "bajo")
	if err != nil || !reflect.DeepEqual(l, []string{"reliability"}) || m != crMotBajo {
		t.Fatalf("CA-619: bajo y chico: %v %q %v", l, m, err)
	}
	l, m, err = LentesConRiesgo(chico, "", "alto")
	if err != nil || !reflect.DeepEqual(l, Lentes) || m != crMotAlto {
		t.Fatalf("CA-619: alto y chico: las 4, %q: %v %q %v", crMotAlto, l, m, err)
	}
	l, m, err = LentesConRiesgo(grande, "", "alto")
	if err != nil || !reflect.DeepEqual(l, Lentes) || m != crMotAlto {
		t.Fatalf("CA-619: alto y grande: %v %q %v", l, m, err)
	}
	// bajo con una ruta de riesgo: las de normal (las 4 por la ruta)
	ln, mn, _ := Lenses(riesgosa, "")
	l, m, err = LentesConRiesgo(riesgosa, "", "bajo")
	if err != nil || !reflect.DeepEqual(l, ln) || m != mn || len(l) != 4 {
		t.Fatalf("CA-619: bajo que toca una ruta de riesgo corre las de normal (%v %q): %v %q %v", ln, mn, l, m, err)
	}
	// normal y "" dan exactamente Lenses
	for _, g := range []gitx.Info{grande, chico, riesgosa, {ChangedFiles: []string{"a.go"}, Insertions: 400}} {
		ln, mn, en := Lenses(g, "")
		for _, r := range []string{"normal", ""} {
			l, m, err := LentesConRiesgo(g, "", r)
			if !reflect.DeepEqual(l, ln) || m != mn || (err == nil) != (en == nil) {
				t.Fatalf("CA-619: con %q es Lenses: %v %q / %v %q", r, ln, mn, l, m)
			}
		}
	}
}

// CA-619 (antes del riesgo, como hoy): --lens a mano manda, y un cambio vacio
// o de solo documentacion no invoca review, con cualquier riesgo. Un riesgo
// fuera del vocabulario es un error.
func TestCA619_AntesDelRiesgo(t *testing.T) {
	for _, r := range []string{"bajo", "normal", "alto", ""} {
		l, _, err := LentesConRiesgo(gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 900}, "readability", r)
		if err != nil || !reflect.DeepEqual(l, []string{"readability"}) {
			t.Fatalf("CA-619: --lens a mano manda con %q: %v %v", r, l, err)
		}
		for _, g := range []gitx.Info{{}, {ChangedFiles: []string{"README.md", "docs/a.txt"}, Insertions: 900}} {
			l, m, err := LentesConRiesgo(g, "", r)
			if err != nil || len(l) != 0 || m == "" {
				t.Fatalf("CA-619: vacio o solo documentacion: 0 lentes con %q (caso limite: riesgo alto y solo documentacion): %v %q %v", r, l, m, err)
			}
		}
	}
	if _, _, err := LentesConRiesgo(gitx.Info{ChangedFiles: []string{"a.go"}}, "", "critico"); err == nil ||
		err.Error() != `riesgo "critico" fuera del vocabulario (bajo, normal, alto)` {
		t.Fatalf("CA-619: un riesgo fuera del vocabulario es el error de riesgo.Validar: %v", err)
	}
	if _, _, err := LentesConRiesgo(gitx.Info{ChangedFiles: []string{"a.go"}}, "profundidad", "alto"); err == nil {
		t.Fatal("CA-619: una lente desconocida es el error de Lenses")
	}
}

type crInfo struct {
	G gitx.Info
	R string
}

func (crInfo) Generate(r *rand.Rand, _ int) reflect.Value {
	piezas := []string{"a.go", "README.md", "internal/auth/x.go", "docs/g.md", "pagos/f.php", "x/DESIGN.kt", "LICENSE"}
	var fs []string
	for i := r.Intn(4); i > 0; i-- {
		fs = append(fs, piezas[r.Intn(len(piezas))])
	}
	rs := []string{"", "bajo", "normal", "alto"}
	return reflect.ValueOf(crInfo{G: gitx.Info{ChangedFiles: fs, Insertions: r.Intn(900), Deletions: r.Intn(50)}, R: rs[r.Intn(4)]})
}

// CA-619 (propiedad): alto nunca da menos lentes que normal y normal nunca
// menos que bajo; las lentes son siempre un prefijo-orden de Lentes; con
// bajo y una ruta de riesgo, lo mismo que normal.
func TestCA619_PropiedadOrdenDeRiesgos(t *testing.T) {
	prop := func(c crInfo) bool {
		b, _, e1 := LentesConRiesgo(c.G, "", "bajo")
		n, _, e2 := LentesConRiesgo(c.G, "", "normal")
		a, _, e3 := LentesConRiesgo(c.G, "", "alto")
		if e1 != nil || e2 != nil || e3 != nil {
			return false
		}
		if len(a) < len(n) || len(n) < len(b) {
			return false
		}
		for _, ls := range [][]string{b, n, a} {
			for _, l := range ls {
				if !crEsLente(l) {
					return false
				}
			}
		}
		hayRuta := false
		for _, f := range c.G.ChangedFiles {
			lf := strings.ToLower(f)
			for _, m := range []string{"auth", "pago", "sign"} {
				hayRuta = hayRuta || strings.Contains(lf, m)
			}
		}
		if hayRuta && !reflect.DeepEqual(b, n) {
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 1500}); err != nil {
		t.Fatalf("CA-619: %v", err)
	}
}

func crEsLente(l string) bool {
	for _, x := range Lentes {
		if x == l {
			return true
		}
	}
	return false
}

// crTareaCon arma raRepo, la tarea slug (worktree) con su item de riesgo r y
// los archivos dados commiteados en la tarea. Devuelve root, el worktree y el
// CLI falso.
func crTareaCon(t *testing.T, slug, r string, archivos map[string]string) (root, wt string, cx *raCLI) {
	t.Helper()
	bin := raPATH(t)
	root = raRepo(t, "")
	wt = cbTarea(t, root, slug)
	if len(archivos) > 0 {
		rdCommit(t, wt, "mas codigo", archivos)
	}
	crItem(t, root, slug, r)
	cx = raInstalar(t, bin, "codex", "")
	return root, wt, cx
}

// CA-619/CA-621: `hoom review` corre reliability con riesgo bajo aunque el
// cambio pase de 400 lineas, con el motivo de la tabla; el registro, el
// Result y --json guardan riesgo bajo sin riesgo_declarado; la salida trae
// `riesgo: bajo`.
func TestCA619_ReviewBajoGrandeCorreReliability(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "bajo", map[string]string{"grande.go": rdCodigo(450, "Grande")})
	res, out := rdRevisar(t, "CA-619", root, Options{Provider: "codex", Task: "precios"})
	if res.Status != "revisado" || !reflect.DeepEqual(res.Lenses, []string{"reliability"}) || cx.veces() != 1 || res.Reason != crMotBajo {
		t.Fatalf("CA-619: bajo con mas de 400 lineas corre reliability con %q: %+v (%d pasadas)\n%s", crMotBajo, res, cx.veces(), out)
	}
	if res.Cobertura != CoberturaCompleta {
		t.Fatalf("CA-619: una sola lente con bajo cubre lo que la regla pide: completa: %+v", res)
	}
	crRiesgo(t, "CA-621", wt, res, out, "bajo", "")
	if !crLinea(out, "riesgo: bajo") {
		t.Fatalf("CA-621: la salida trae la linea `riesgo: bajo`:\n%s", out)
	}
}

// CA-619: con riesgo alto, las 4 aunque el cambio no pase de 400.
func TestCA619_ReviewAltoChicoCorreLasCuatro(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "alto", nil)
	res, out := rdRevisar(t, "CA-619", root, Options{Provider: "codex", Task: "precios"})
	if res.Status != "revisado" || !reflect.DeepEqual(res.Lenses, Lentes) || cx.veces() != 4 || res.Reason != crMotAlto {
		t.Fatalf("CA-619: alto y chico corre las 4 con %q: %+v\n%s", crMotAlto, res, out)
	}
	crRiesgo(t, "CA-621", wt, res, out, "alto", "")
	if !crLinea(out, "riesgo: alto") {
		t.Fatalf("CA-621: la salida trae `riesgo: alto`:\n%s", out)
	}
}

// CA-619: con riesgo normal o sin item, lo mismo que hoy (Lenses): un
// cambio chico, una lente; y riesgo normal en el registro.
func TestCA619_ReviewNormalYSinItemComoHoy(t *testing.T) {
	for _, conItem := range []bool{true, false} {
		bin := raPATH(t)
		root := raRepo(t, "")
		wt := cbTarea(t, root, "precios")
		if conItem {
			crItem(t, root, "precios", "normal")
		}
		cx := raInstalar(t, bin, "codex", "")
		res, out := rdRevisar(t, "CA-619", root, Options{Provider: "codex", Task: "precios"})
		if res.Status != "revisado" || !reflect.DeepEqual(res.Lenses, []string{LenteDominante}) || cx.veces() != 1 {
			t.Fatalf("CA-619: normal (item %v) como hoy: %+v\n%s", conItem, res, out)
		}
		if strings.HasPrefix(res.Reason, "riesgo ") {
			t.Fatalf("CA-619: con normal el motivo es el de hoy: %q", res.Reason)
		}
		crRiesgo(t, "CA-621", wt, res, out, "normal", "")
	}
}

// CA-619/CA-621 (caso limite: riesgo bajo y el cambio toca internal/auth/):
// el efectivo es normal (las 4 por la ruta), el registro guarda los dos y la
// salida dice por que.
func TestCA621_BajoQueTocaUnaRutaDeRiesgo(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "bajo", map[string]string{
		"internal/auth/token.go": "package auth\n\nfunc Valida(s string) bool { return s != \"\" }\n"})
	res, out := rdRevisar(t, "CA-619", root, Options{Provider: "codex", Task: "precios"})
	if res.Status != "revisado" || !reflect.DeepEqual(res.Lenses, Lentes) || cx.veces() != 4 {
		t.Fatalf("CA-619: una tarjeta bajo cuyo cambio toca una ruta de riesgo corre las de normal: %+v\n%s", res, out)
	}
	crRiesgo(t, "CA-621", wt, res, out, "normal", "bajo")
	if !crLinea(out, "riesgo: el cambio toca internal/auth/token.go: riesgo normal (declarado bajo)") {
		t.Fatalf("CA-621: la salida dice `riesgo: <motivo> (declarado bajo)`:\n%s", out)
	}
}

// crLinea dice si out tiene una linea que, sin espacios alrededor, es l.
func crLinea(out, l string) bool {
	for _, x := range strings.Split(out, "\n") {
		if strings.TrimSpace(x) == l {
			return true
		}
	}
	return false
}

// crRiesgo exige riesgo y riesgo_declarado (vacio = ausente) en el Result,
// el registro leido, el registro crudo y el JSON del Result.
func crRiesgo(t *testing.T, ca, dir string, res Result, out, riesgo, declarado string) {
	t.Helper()
	if res.Riesgo != riesgo || res.RiesgoDeclarado != declarado {
		t.Fatalf("%s: el Result dice riesgo %q, declarado %q: %q %q", ca, riesgo, declarado, res.Riesgo, res.RiesgoDeclarado)
	}
	if res.RecordID == "" {
		t.Fatalf("%s: la review deja registro: %+v\n%s", ca, res, out)
	}
	r := rdRegistro(t, ca, dir, res.RecordID)
	if r.Riesgo != riesgo || r.RiesgoDeclarado != declarado {
		t.Fatalf("%s: el registro dice riesgo %q, declarado %q: %+v", ca, riesgo, declarado, r)
	}
	m := raRegistroCrudo(t, dir, res.RecordID)
	if m["riesgo"] != riesgo {
		t.Fatalf("%s: el JSON del registro trae riesgo %q: %v", ca, riesgo, m)
	}
	if v, ok := m["riesgo_declarado"]; (declarado == "" && ok) || (declarado != "" && v != declarado) {
		t.Fatalf("%s: riesgo_declarado solo cuando difiere (%q): %v", ca, declarado, m)
	}
	raw, _ := json.Marshal(res)
	var j map[string]any
	_ = json.Unmarshal(raw, &j)
	if j["riesgo"] != riesgo {
		t.Fatalf("%s: --json trae riesgo %q: %s", ca, riesgo, raw)
	}
	if v, ok := j["riesgo_declarado"]; (declarado == "" && ok) || (declarado != "" && v != declarado) {
		t.Fatalf("%s: --json trae riesgo_declarado solo cuando difiere: %s", ca, raw)
	}
}

// CA-620: una delta cuya regla da las 4 (riesgo alto) corre risk,
// reliability y resilience, en ese orden, sin readability, con el motivo
// terminado en `; una delta no corre readability` y cobertura delta.
func TestCA620_DeltaDeCuatroNoCorreReadability(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "alto", nil)
	r1, out := rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios"})
	if r1.Cobertura != CoberturaCompleta || !reflect.DeepEqual(r1.Lenses, Lentes) {
		t.Fatalf("CA-620: fixture: la completa de una tarjeta alto lleva las 4 (readability sigue en la completa): %+v\n%s", r1, out)
	}
	rdEsperarSegundo()
	rdCommit(t, wt, "arreglo", map[string]string{"arreglo.go": "package app\n\nfunc Arreglo() {}\n"})
	antes := cx.veces()
	r2, out := rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios", Delta: true})
	if r2.Status != "revisado" || r2.Cobertura != CoberturaDelta || !reflect.DeepEqual(r2.Lenses, crTres) || cx.veces()-antes != 3 {
		t.Fatalf("CA-620: la delta corre %v y queda delta: %+v (%d pasadas)\n%s", crTres, r2, cx.veces()-antes, out)
	}
	if !strings.HasSuffix(r2.Reason, crSufijo) {
		t.Fatalf("CA-620: el motivo termina en %q: %q", crSufijo, r2.Reason)
	}
	var orden []string
	for _, p := range r2.Passes {
		orden = append(orden, p.Lens)
	}
	if !reflect.DeepEqual(orden, crTres) {
		t.Fatalf("CA-620: las pasadas van en el orden %v: %v", crTres, orden)
	}
	rec := rdRegistro(t, "CA-620", wt, r2.RecordID)
	if !reflect.DeepEqual(rec.Lenses, crTres) || rec.Cobertura != CoberturaDelta || rec.Riesgo != "alto" {
		t.Fatalf("CA-620/CA-621: el registro de la delta: %+v", rec)
	}
	for _, p := range r2.Passes {
		if p.Lens == "readability" {
			t.Fatalf("CA-620: ninguna pasada de readability en la delta: %+v", r2.Passes)
		}
	}
}

// CA-620: una delta cuya regla da una sola lente corre esa (un cambio chico,
// riesgo normal o bajo).
func TestCA620_DeltaDeUnaLenteCorreEsa(t *testing.T) {
	for _, r := range []string{"normal", "bajo"} {
		root, wt, cx := crTareaCon(t, "precios", r, nil)
		r1, _ := rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios"})
		if r1.Cobertura != CoberturaCompleta {
			t.Fatalf("CA-620: fixture: %+v", r1)
		}
		rdEsperarSegundo()
		rdCommit(t, wt, "arreglo", map[string]string{"arreglo.go": "package app\n\nfunc Arreglo() {}\n"})
		antes := cx.veces()
		r2, out := rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios", Delta: true})
		if r2.Cobertura != CoberturaDelta || !reflect.DeepEqual(r2.Lenses, []string{"reliability"}) || cx.veces()-antes != 1 {
			t.Fatalf("CA-620: con %s la delta chica corre reliability: %+v\n%s", r, r2, out)
		}
		if strings.HasSuffix(r2.Reason, crSufijo) {
			t.Fatalf("CA-620: el sufijo es solo para la delta que daba las 4: %q", r2.Reason)
		}
	}
}

// CA-620 (caso limite: riesgo bajo y solo la delta no toca la ruta de
// riesgo): el efectivo sigue normal porque se mide sobre el cambio entero;
// la delta corre las 3 (el cambio entero toca la ruta: las 4 de normal) y el
// registro guarda riesgo normal y declarado bajo.
func TestCA620_DeltaDeBajoConLaRutaEnElCambioEntero(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "bajo", map[string]string{
		"internal/auth/token.go": "package auth\n\nfunc Valida(s string) bool { return s != \"\" }\n"})
	r1, out := rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios"})
	if !reflect.DeepEqual(r1.Lenses, Lentes) {
		t.Fatalf("CA-620: fixture: %+v\n%s", r1, out)
	}
	rdEsperarSegundo()
	rdCommit(t, wt, "arreglo fuera de la ruta", map[string]string{"arreglo.go": "package app\n\nfunc Arreglo() {}\n"})
	antes := cx.veces()
	r2, out := rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios", Delta: true})
	if r2.Cobertura != CoberturaDelta || !reflect.DeepEqual(r2.Lenses, crTres) || cx.veces()-antes != 3 || !strings.HasSuffix(r2.Reason, crSufijo) {
		t.Fatalf("CA-620: la delta de una tarjeta bajo cuyo cambio entero toca la ruta corre las 3: %+v\n%s", r2, out)
	}
	crRiesgo(t, "CA-621", wt, r2, out, "normal", "bajo")
}

// CA-620 (caso limite: `--lens readability` en una delta con codigo): corre,
// y la cobertura es parcial.
func TestCA620_LensReadabilityEnUnaDeltaEsParcial(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "alto", nil)
	rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios"})
	rdEsperarSegundo()
	rdCommit(t, wt, "arreglo", map[string]string{"arreglo.go": "package app\n\nfunc Arreglo() {}\n"})
	antes := cx.veces()
	r2, out := rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios", Delta: true, Lens: "readability"})
	if r2.Status != "revisado" || !reflect.DeepEqual(r2.Lenses, []string{"readability"}) || cx.veces()-antes != 1 || r2.Cobertura != CoberturaParcial {
		t.Fatalf("CA-620: --lens readability en una delta corre y queda parcial: %+v\n%s", r2, out)
	}
}

// CA-620 (caso limite: una delta de solo documentacion): 0 lentes, como hoy.
func TestCA620_DeltaSoloDocumentacion(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "alto", nil)
	rdRevisar(t, "CA-620", root, Options{Provider: "codex", Task: "precios"})
	rdEsperarSegundo()
	rdCommit(t, wt, "docs", map[string]string{"docs/guia.md": "# guia\n"})
	antes := cx.veces()
	res, err, out := rdCorrer(t, "CA-620", root, Options{Provider: "codex", Task: "precios", Delta: true})
	if err != nil || cx.veces() != antes || len(res.Lenses) != 0 || len(res.Passes) != 0 {
		t.Fatalf("CA-620: una delta de solo documentacion no corre lentes: %+v %v\n%s", res, err, out)
	}
}

// CA-619: una tarea sin item tiene riesgo normal; el item de otra tarea no
// cuenta.
func TestCA619_ReviewSinItemDeLaTarea(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	wt := cbTarea(t, root, "precios")
	crItem(t, root, "otra", "alto") // el item de OTRA tarea no cuenta
	cx := raInstalar(t, bin, "codex", "")
	res, out := rdRevisar(t, "CA-619", root, Options{Provider: "codex", Task: "precios"})
	if !reflect.DeepEqual(res.Lenses, []string{LenteDominante}) || cx.veces() != 1 {
		t.Fatalf("CA-619: una tarea sin item tiene riesgo normal: %+v\n%s", res, out)
	}
	crRiesgo(t, "CA-621", wt, res, out, "normal", "")
}
