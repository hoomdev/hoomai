// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// `hoom review` (CA-619, CA-620, CA-621): las lentes segun el riesgo de la
// tarjeta (LentesConRiesgo), la delta sin readability y el riesgo en el
// registro, el Result, --json y la salida. Los CLIs de IA son falsos
// (raPATH + raInstalar): ningun claude ni codex real. Escritos desde el spec
// y las firmas publicas, sin leer la implementacion. Re-expresados por
// riesgo-efectivo-y-tamano (CA-619, CA-621): LentesConRiesgo desaparece y la
// regla es ReglaDeLentes con el riesgo efectivo (Evaluar) y las lineas de
// producto; una ruta de riesgo da alto, un bajo que toca producto fuera de
// las rutas de bajo riesgo da normal, y el registro suma riesgo_motivo y
// lentes_motivo.
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
	"github.com/hoomdev/hoomai/internal/riesgo"
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

// crRegla es la regla de lentes de riesgo-efectivo-y-tamano para un cambio
// conocido por su gitx.Info y una tarjeta que declara r: ReglaDeLentes con el
// efectivo de Evaluar (rutas base) y producto = insertions + deletions. Es
// lo que antes daba LentesConRiesgo(g, explicit, r).
func crRegla(g gitx.Info, explicit, r string) ([]string, string, error) {
	e := riesgo.Clasificador{Rutas: riesgo.RutasBase()}.Evaluar(riesgo.Tarjeta{Tarea: "t", Riesgo: r}, g.ChangedFiles)
	return ReglaDeLentes(g.ChangedFiles, g.Insertions+g.Deletions, e.Efectivo, explicit)
}

// CA-619: la regla con bajo, normal y alto, con el motivo de la tabla.
func TestCA619_LentesConRiesgoTabla(t *testing.T) {
	grande := gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 450}
	chico := gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 10}
	riesgosa := gitx.Info{ChangedFiles: []string{"a.go", "internal/auth/login.go"}, Insertions: 10}

	// CAL-2: normal corre 1 sola lente dominante (reliability) sin importar las 400 lineas.
	l, m, err := crRegla(grande, "", "bajo")
	if err != nil || !reflect.DeepEqual(l, []string{"reliability"}) || m != MotivoEstandar {
		t.Fatalf("CA-619: bajo que toca a.go y mas de 400 lineas: normal, 1 lente: %v %q %v", l, m, err)
	}
	l, m, err = crRegla(chico, "", "bajo")
	if err != nil || !reflect.DeepEqual(l, []string{"reliability"}) || m != MotivoEstandar {
		t.Fatalf("CA-619: bajo que toca a.go y chico: normal, %q (ya no %q): %v %q %v", MotivoEstandar, crMotBajo, l, m, err)
	}
	l, m, err = crRegla(chico, "", "alto")
	if err != nil || !reflect.DeepEqual(l, Lentes) || m != crMotAlto {
		t.Fatalf("CA-619: alto y chico: las 4, %q: %v %q %v", crMotAlto, l, m, err)
	}
	l, m, err = crRegla(grande, "", "alto")
	if err != nil || !reflect.DeepEqual(l, Lentes) || m != crMotAlto {
		t.Fatalf("CA-619: alto y grande: %v %q %v", l, m, err)
	}
	// bajo con una ruta de riesgo: alto, las 4 (como Lenses, que ve la ruta)
	ln, mn, _ := Lenses(riesgosa, "")
	l, m, err = crRegla(riesgosa, "", "bajo")
	if err != nil || !reflect.DeepEqual(l, ln) || m != mn || len(l) != 4 || m != crMotAlto {
		t.Fatalf("CA-619: bajo que toca una ruta de riesgo es alto, las 4 (%v %q): %v %q %v", ln, mn, l, m, err)
	}
	// normal y "" dan exactamente Lenses
	for _, g := range []gitx.Info{grande, chico, riesgosa, {ChangedFiles: []string{"a.go"}, Insertions: 400}} {
		ln, mn, en := Lenses(g, "")
		for _, r := range []string{"normal", ""} {
			l, m, err := crRegla(g, "", r)
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
		l, _, err := crRegla(gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 900}, "readability", r)
		if err != nil || !reflect.DeepEqual(l, []string{"readability"}) {
			t.Fatalf("CA-619: --lens a mano manda con %q: %v %v", r, l, err)
		}
		for _, g := range []gitx.Info{{}, {ChangedFiles: []string{"README.md", "docs/a.txt"}, Insertions: 900}} {
			l, m, err := crRegla(g, "", r)
			if err != nil || len(l) != 0 || m == "" {
				t.Fatalf("CA-619: vacio o solo documentacion: 0 lentes con %q (caso limite: riesgo alto y solo documentacion): %v %q %v", r, l, m, err)
			}
		}
	}
	// riesgo-efectivo-y-tamano: un efectivo fuera del vocabulario es el error
	// de riesgo.Validar (un declarado fuera del vocabulario ya es alto)
	if _, _, err := ReglaDeLentes([]string{"a.go"}, 0, "critico", ""); err == nil ||
		err.Error() != `riesgo "critico" fuera del vocabulario (bajo, normal, alto)` {
		t.Fatalf("CA-619: un riesgo fuera del vocabulario es el error de riesgo.Validar: %v", err)
	}
	if _, _, err := crRegla(gitx.Info{ChangedFiles: []string{"a.go"}}, "profundidad", "alto"); err == nil {
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
// bajo y una ruta de riesgo, lo mismo que alto.
func TestCA619_PropiedadOrdenDeRiesgos(t *testing.T) {
	deRiesgo := riesgo.RutasBase().DeRiesgo
	prop := func(c crInfo) bool {
		b, _, e1 := crRegla(c.G, "", "bajo")
		n, _, e2 := crRegla(c.G, "", "normal")
		a, _, e3 := crRegla(c.G, "", "alto")
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
			hayRuta = hayRuta || riesgo.CoincideAlguno(deRiesgo, f)
		}
		if hayRuta && !reflect.DeepEqual(b, a) {
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

// CA-619/CA-621: con riesgo bajo, un cambio que toca producto fuera de las
// rutas de bajo riesgo (app.go de la tarea y grande.go) es normal efectivo
// (riesgo-efectivo-y-tamano): pasa de 400 lineas de producto (452) y corre
// las 4, con el motivo de la tabla; el registro, el Result y --json guardan
// riesgo normal y declarado bajo; la salida trae `riesgo: normal (<motivo>)`.
func TestCA619_ReviewBajoGrandeCorreReliability(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "bajo", map[string]string{"grande.go": rdCodigo(450, "Grande")})
	res, out := rdRevisar(t, "CA-619", root, Options{Provider: "codex", Task: "precios"})
	if res.Status != "revisado" || !reflect.DeepEqual(res.Lenses, []string{LenteDominante}) || cx.veces() != 1 || res.Reason != MotivoEstandar {
		t.Fatalf("CA-619: bajo que sube a normal con 452 lineas de producto corre 1 lente con %q: %+v (%d pasadas)\n%s", MotivoEstandar, res, cx.veces(), out)
	}
	if res.Cobertura != CoberturaCompleta {
		t.Fatalf("CA-619: las lentes que la regla pide cubren: completa: %+v", res)
	}
	crRiesgo(t, "CA-621", wt, res, out, "normal", "bajo")
	if !crLinea(out, "riesgo: normal (el cambio toca app.go, fuera de las rutas de bajo riesgo: riesgo normal)") {
		t.Fatalf("CA-621: la salida trae la linea `riesgo: normal (<motivo>)`:\n%s", out)
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
	if !crLinea(out, "riesgo: alto (declarado alto)") {
		t.Fatalf("CA-621: la salida trae `riesgo: alto (declarado alto)`:\n%s", out)
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
// el efectivo es alto (riesgo-efectivo-y-tamano: las 4 por la ruta), el
// registro guarda los dos y la salida dice por que.
func TestCA621_BajoQueTocaUnaRutaDeRiesgo(t *testing.T) {
	root, wt, cx := crTareaCon(t, "precios", "bajo", map[string]string{
		"internal/auth/token.go": "package auth\n\nfunc Valida(s string) bool { return s != \"\" }\n"})
	res, out := rdRevisar(t, "CA-619", root, Options{Provider: "codex", Task: "precios"})
	if res.Status != "revisado" || !reflect.DeepEqual(res.Lenses, Lentes) || cx.veces() != 4 {
		t.Fatalf("CA-619: una tarjeta bajo cuyo cambio toca una ruta de riesgo corre las 4 de alto: %+v\n%s", res, out)
	}
	crRiesgo(t, "CA-621", wt, res, out, "alto", "bajo")
	if !crLinea(out, "riesgo: alto (el cambio toca internal/auth/token.go: riesgo alto)") {
		t.Fatalf("CA-621: la salida dice `riesgo: alto (<motivo>)`:\n%s", out)
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
// el registro leido, el registro crudo y el JSON del Result; y, con
// riesgo-efectivo-y-tamano, riesgo_motivo (nunca vacio) y lentes_motivo (el
// Reason) en los cuatro, y la linea `riesgo: <riesgo> (<motivo>)` en out.
func crRiesgo(t *testing.T, ca, dir string, res Result, out, riesgo, declarado string) {
	t.Helper()
	crMotivos(t, ca, dir, res, out, riesgo)
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

// crMotivos: riesgo_motivo y lentes_motivo en el Result, el registro leido,
// el crudo y --json, y la linea `riesgo:` con el motivo (CA-621 re-expresado).
func crMotivos(t *testing.T, ca, dir string, res Result, out, riesgo string) {
	t.Helper()
	if res.RiesgoMotivo == "" || res.LentesMotivo != res.Reason {
		t.Fatalf("%s: el Result trae riesgo_motivo y lentes_motivo (= Reason): %q %q %q", ca, res.RiesgoMotivo, res.LentesMotivo, res.Reason)
	}
	if res.RecordID == "" {
		return
	}
	r := rdRegistro(t, ca, dir, res.RecordID)
	if r.RiesgoMotivo != res.RiesgoMotivo || r.LentesMotivo != res.Reason {
		t.Fatalf("%s: el registro trae riesgo_motivo %q y lentes_motivo %q: %+v", ca, res.RiesgoMotivo, res.Reason, r)
	}
	m := raRegistroCrudo(t, dir, res.RecordID)
	if m["riesgo_motivo"] != res.RiesgoMotivo || m["lentes_motivo"] != res.Reason {
		t.Fatalf("%s: el JSON del registro trae riesgo_motivo y lentes_motivo: %v", ca, m)
	}
	raw, _ := json.Marshal(res)
	var j map[string]any
	_ = json.Unmarshal(raw, &j)
	if j["riesgo_motivo"] != res.RiesgoMotivo || j["lentes_motivo"] != res.Reason {
		t.Fatalf("%s: --json trae riesgo_motivo y lentes_motivo: %s", ca, raw)
	}
	if l := "riesgo: " + riesgo + " (" + res.RiesgoMotivo + ")"; !crLinea(out, l) {
		t.Fatalf("%s: la salida trae %q:\n%s", ca, l, out)
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
// riesgo): el efectivo sigue alto porque se mide sobre el cambio entero
// (riesgo-efectivo-y-tamano, aclaracion 19); la delta corre las 3 (el cambio
// entero toca la ruta: las 4 de alto) y el registro guarda riesgo alto y
// declarado bajo.
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
	crRiesgo(t, "CA-621", wt, r2, out, "alto", "bajo")
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
