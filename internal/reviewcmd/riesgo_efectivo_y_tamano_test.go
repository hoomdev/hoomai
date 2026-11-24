// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// `hoom review`: la regla de lentes por el riesgo efectivo y las lineas de
// producto (CA-782), el rango y la delta (CA-783), y las rutas y el
// territorio del test-writer del merge-base, con riesgo_motivo y
// lentes_motivo en el registro, el Result y --json (CA-784). Los CLIs de IA
// son falsos (raPATH + raInstalar): ningun claude ni codex real. Escritos
// desde el spec, las aclaraciones de la ronda 0 y las firmas publicas, sin
// leer la implementacion.
package reviewcmd

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/riesgo"
)

const retySlug = "precios"

// retyItem escribe el item de slug en el arbol raiz, de tipo tipo y con el
// riesgo r ("" = sin la clave).
func retyItem(t *testing.T, root, slug, tipo, r string) {
	t.Helper()
	body := "titulo: Tarea " + slug + "\ntipo: " + tipo + "\nprioridad: media\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-11-20T10:00:00Z\n"
	if r != "" {
		body += "riesgo: " + r + "\n"
	}
	write(t, root, filepath.Join(".hoom", "items", slug+".yaml"), body)
}

// retyTarea crea la tarea slug como `hoom task start` (rama hoom/<slug>,
// worktree .hoom/worktrees/<slug>) y le commitea SOLO archivos: a diferencia
// de cbTarea, no toca app.go.
func retyTarea(t *testing.T, root, slug string, archivos map[string]string) string {
	t.Helper()
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	git(t, root, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	if len(archivos) > 0 {
		rdCommit(t, wt, "la tarea", archivos)
	}
	return wt
}

// retyProyecto arma raRepo(yaml), la tarea con su item (tipo, r) y los
// archivos, y el codex falso.
func retyProyecto(t *testing.T, yaml, tipo, r string, archivos map[string]string) (root, wt string, cx *raCLI) {
	t.Helper()
	bin := raPATH(t)
	root = raRepo(t, yaml)
	wt = retyTarea(t, root, retySlug, archivos)
	retyItem(t, root, retySlug, tipo, r)
	cx = raInstalar(t, bin, "codex", "")
	return root, wt, cx
}

func retyRevisar(t *testing.T, ca, root string, opt Options) (Result, string) {
	t.Helper()
	opt.Provider = "codex"
	if opt.Task == "" && !opt.DesdeSet && opt.Desde == "" {
		opt.Task = retySlug
	}
	return rdRevisar(t, ca, root, opt)
}

// retyLentes exige las lentes, las pasadas y el motivo exacto (Reason).
func retyLentes(t *testing.T, ca, caso string, res Result, out string, cx *raCLI, antes int, lentes []string, motivo string) {
	t.Helper()
	if res.Status != "revisado" || !reflect.DeepEqual(res.Lenses, lentes) || cx.veces()-antes != len(lentes) || res.Reason != motivo {
		t.Fatalf("%s: %s: corre %v (%d pasadas) con %q: %+v (%d pasadas)\n%s", ca, caso, lentes, len(lentes), motivo, res, cx.veces()-antes, out)
	}
}

// retyMotivos exige riesgo, riesgo_declarado (vacio = ausente) y
// riesgo_motivo en el Result, el registro, su JSON crudo y el JSON del
// Result; lentes_motivo igual a Reason; y la linea `riesgo: <e> (<m>)`.
func retyMotivos(t *testing.T, ca, dir string, res Result, out, r, declarado, motivo string) {
	t.Helper()
	if res.Riesgo != r || res.RiesgoDeclarado != declarado || res.RiesgoMotivo != motivo || res.LentesMotivo != res.Reason || res.Reason == "" {
		t.Fatalf("%s: el Result dice riesgo %q, declarado %q, motivo %q y lentes_motivo = reason: %+v", ca, r, declarado, motivo, res)
	}
	if !crLinea(out, "riesgo: "+r+" ("+motivo+")") {
		t.Fatalf("%s: la salida trae `riesgo: %s (%s)`:\n%s", ca, r, motivo, out)
	}
	if res.RecordID == "" {
		t.Fatalf("%s: la review deja registro: %+v\n%s", ca, res, out)
	}
	rec := rdRegistro(t, ca, dir, res.RecordID)
	if rec.Riesgo != r || rec.RiesgoDeclarado != declarado || rec.RiesgoMotivo != motivo || rec.LentesMotivo != res.Reason {
		t.Fatalf("%s: el registro trae riesgo, riesgo_declarado, riesgo_motivo y lentes_motivo: %+v", ca, rec)
	}
	crudo := raRegistroCrudo(t, dir, res.RecordID)
	if crudo["riesgo"] != r || crudo["riesgo_motivo"] != motivo || crudo["lentes_motivo"] != res.Reason {
		t.Fatalf("%s: el JSON del registro trae riesgo_motivo y lentes_motivo: %v", ca, crudo)
	}
	if v, ok := crudo["riesgo_declarado"]; (declarado == "" && ok) || (declarado != "" && v != declarado) {
		t.Fatalf("%s: riesgo_declarado solo cuando difiere (%q): %v", ca, declarado, crudo)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	var j map[string]any
	if err := json.Unmarshal(raw, &j); err != nil {
		t.Fatal(err)
	}
	if j["riesgo"] != r || j["riesgo_motivo"] != motivo || j["lentes_motivo"] != res.Reason {
		t.Fatalf("%s: --json trae riesgo, riesgo_motivo y lentes_motivo: %s", ca, raw)
	}
}

// ---------------------------------------------------------------- CA-782

// CA-782: las constantes de la regla, exactas.
func TestCA782_ConstantesDeLaRegla(t *testing.T) {
	const ca = "CA-782"
	for _, k := range [][2]string{
		{MotivoLenteAMano, "lente pedida a mano"},
		{MotivoSinCambios, "no hay cambios contra la base: no hay nada que revisar"},
		{MotivoSoloDocs, "el cambio es solo documentacion: no se invoca review"},
		{MotivoRiesgoAlto, "riesgo alto: las 4 lentes"},
		{MotivoRiesgoBajo, "riesgo bajo: la lente dominante"},
		{MotivoProductoGrande, "%d lineas de producto (>400): las 4 lentes"},
		{MotivoEstandar, "cambio estandar: la lente dominante"},
	} {
		if k[0] != k[1] {
			t.Fatalf("%s: la constante es %q, fue %q", ca, k[1], k[0])
		}
	}
	if UmbralLineas != 400 {
		t.Fatalf("%s: UmbralLineas es 400, fue %d", ca, UmbralLineas)
	}
	if riesgo.UmbralProducto != UmbralLineas {
		t.Fatalf("%s: UmbralLineas es riesgo.UmbralProducto: %d, %d", ca, UmbralLineas, riesgo.UmbralProducto)
	}
	if LenteDominante != "reliability" {
		t.Fatalf("%s: LenteDominante es reliability, fue %q", ca, LenteDominante)
	}
	if !reflect.DeepEqual(LentesDeUnaDelta, []string{"risk", "reliability", "resilience"}) {
		t.Fatalf("CA-783: LentesDeUnaDelta: %v", LentesDeUnaDelta)
	}
}

// CA-782: ReglaDeLentes sigue la tabla, en orden, con el motivo exacto.
func TestCA782_ReglaDeLentesTabla(t *testing.T) {
	const ca = "CA-782"
	codigo := []string{"a.go"}
	docs := []string{"README.md", "docs/a.go", "LICENSE"}
	uno := []string{LenteDominante}
	for _, k := range []struct {
		nombre             string
		files              []string
		producto           int
		efectivo, explicit string
		lentes             []string
		motivo             string
	}{
		{"--lens", codigo, 900, "alto", "risk", []string{"risk"}, MotivoLenteAMano},
		{"--lens con blancos y mayusculas", codigo, 1, "bajo", " Resilience ", []string{"resilience"}, MotivoLenteAMano},
		{"--lens sin cambios", nil, 0, "alto", "readability", []string{"readability"}, MotivoLenteAMano},
		{"sin cambios", nil, 0, "alto", "", nil, MotivoSinCambios},
		{"sin cambios vacio", []string{}, 900, "normal", "", nil, MotivoSinCambios},
		{"solo docs con alto", docs, 900, "alto", "", nil, MotivoSoloDocs},
		{"solo docs con normal grande", docs, 5000, "normal", "", nil, MotivoSoloDocs},
		{"alto chico", codigo, 1, "alto", "", Lentes, MotivoRiesgoAlto},
		{"alto sin producto", []string{"a_test.go"}, 0, "alto", "", Lentes, MotivoRiesgoAlto},
		{"bajo grande", codigo, 5000, "bajo", "", uno, MotivoRiesgoBajo},
		{"bajo 401", codigo, 401, "bajo", "", uno, MotivoRiesgoBajo},
		{"normal 401", codigo, 401, "normal", "", Lentes, "401 lineas de producto (>400): las 4 lentes"},
		{"vacio 401", codigo, 401, "", "", Lentes, "401 lineas de producto (>400): las 4 lentes"},
		{"normal 9300", codigo, 9300, "normal", "", Lentes, "9300 lineas de producto (>400): las 4 lentes"},
		{"normal 400", codigo, 400, "normal", "", uno, MotivoEstandar},
		{"normal 90", codigo, 90, "normal", "", uno, MotivoEstandar},
		{"vacio 0", []string{"a_test.go"}, 0, "", "", uno, MotivoEstandar},
	} {
		l, m, err := ReglaDeLentes(k.files, k.producto, k.efectivo, k.explicit)
		if err != nil || !reflect.DeepEqual(l, k.lentes) && !(len(l) == 0 && len(k.lentes) == 0) || m != k.motivo {
			t.Fatalf("%s: %s: ReglaDeLentes = %v %q %v; quiero %v %q", ca, k.nombre, l, m, err, k.lentes, k.motivo)
		}
	}
	// Un efectivo fuera del vocabulario es el error de riesgo.Validar,
	// antes que todo; una lente desconocida, el de siempre.
	for _, e := range []struct {
		files    []string
		explicit string
	}{{codigo, ""}, {codigo, "risk"}, {nil, ""}, {docs, ""}} {
		_, _, err := ReglaDeLentes(e.files, 1, "medio", e.explicit)
		if err == nil || err.Error() != `riesgo "medio" fuera del vocabulario (bajo, normal, alto)` {
			t.Fatalf("%s: un efectivo fuera del vocabulario es el error de riesgo.Validar primero (%v, %q): %v", ca, e.files, e.explicit, err)
		}
	}
	if _, _, err := ReglaDeLentes(codigo, 1, "alto", "profundidad"); err == nil ||
		err.Error() != `lente desconocida "profundidad" (validas: risk, reliability, resilience, readability)` {
		t.Fatalf("%s: una lente desconocida es el error de Lenses: %v", ca, err)
	}
}

type retyRegla struct {
	Files    []string
	Producto int
}

func (retyRegla) Generate(r *rand.Rand, _ int) reflect.Value {
	piezas := []string{"a.go", "README.md", "internal/auth/x.go", "docs/g.md", "a_test.go", ".hoom/specs/x.md", "LICENSE", "x/DESIGN.kt"}
	var fs []string
	for i := r.Intn(4); i > 0; i-- {
		fs = append(fs, piezas[r.Intn(len(piezas))])
	}
	return reflect.ValueOf(retyRegla{Files: fs, Producto: r.Intn(900)})
}

// CA-782 (propiedad): alto nunca da menos lentes que normal ni normal que
// bajo; las lentes son 0, la dominante o las 4 en orden; normal da las 4
// solo con mas de 400 de producto; el motivo nunca queda vacio.
func TestCA782_PropiedadMonotona(t *testing.T) {
	const ca = "CA-782"
	prop := func(e retyRegla) bool {
		var por [3][]string
		for i, ef := range []string{"bajo", "normal", "alto"} {
			l, m, err := ReglaDeLentes(e.Files, e.Producto, ef, "")
			if err != nil || m == "" {
				return false
			}
			if len(l) != 0 && !reflect.DeepEqual(l, []string{LenteDominante}) && !reflect.DeepEqual(l, Lentes) {
				return false
			}
			por[i] = l
		}
		if len(por[2]) < len(por[1]) || len(por[1]) < len(por[0]) {
			return false
		}
		if len(e.Files) > 0 && !SoloDocs(e.Files) {
			if (len(por[1]) == 4) != (e.Producto > 400) || len(por[0]) != 1 || len(por[2]) != 4 {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 1500}); err != nil {
		t.Fatalf("%s: %v", ca, err)
	}
}

// CA-782: Lenses conserva su firma con la regla nueva: el riesgo de un
// cambio sin tarjeta con las rutas por defecto (.hoom/ no es ruta de riesgo,
// una prueba si, design ya no) y producto = insertions + deletions.
func TestCA782_LensesConLaReglaNueva(t *testing.T) {
	const ca = "CA-782"
	for _, k := range []struct {
		g      gitx.Info
		lentes []string
		motivo string
	}{
		{gitx.Info{ChangedFiles: []string{".hoom/specs/auth.md", "a.go"}, Insertions: 10}, []string{LenteDominante}, MotivoEstandar},
		{gitx.Info{ChangedFiles: []string{"internal/spec/tokens_test.go"}, Insertions: 3}, Lentes, MotivoRiesgoAlto},
		{gitx.Info{ChangedFiles: []string{"src/design.go", "assign.go", "signal.go"}, Insertions: 3}, []string{LenteDominante}, MotivoEstandar},
		{gitx.Info{ChangedFiles: []string{"signature.go"}, Insertions: 3}, Lentes, MotivoRiesgoAlto},
		{gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 300, Deletions: 101}, Lentes, "401 lineas de producto (>400): las 4 lentes"},
		{gitx.Info{ChangedFiles: []string{"a.go"}, Insertions: 300, Deletions: 100}, []string{LenteDominante}, MotivoEstandar},
	} {
		l, m, err := Lenses(k.g, "")
		if err != nil || !reflect.DeepEqual(l, k.lentes) || m != k.motivo {
			t.Fatalf("%s: Lenses(%+v) = %v %q %v; quiero %v %q", ca, k.g, l, m, err, k.lentes, k.motivo)
		}
	}
}

// CA-782: una tarjeta normal con 90 lineas de producto y 3,000 de pruebas
// corre reliability con `cambio estandar: la lente dominante`.
func TestCA782_NormalCon90DeProductoY3000DePruebas(t *testing.T) {
	const ca = "CA-782"
	root, wt, cx := retyProyecto(t, "", "feature", "normal", map[string]string{
		"internal/precio.go":      rdCodigo(90, "Precio"),
		"internal/precio_test.go": rdCodigo(3000, "Prueba"),
	})
	res, out := retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "normal 90+3000", res, out, cx, 0, []string{LenteDominante}, MotivoEstandar)
	retyMotivos(t, ca, wt, res, out, "normal", "", "declarado normal")
}

// CA-782: 401 lineas de producto son las 4; 400, reliability.
func TestCA782_401DeProductoLasCuatro(t *testing.T) {
	const ca = "CA-782"
	root, wt, cx := retyProyecto(t, "", "feature", "", map[string]string{
		"internal/precio.go":      rdCodigo(401, "Precio"),
		"internal/precio_test.go": rdCodigo(50, "Prueba"),
	})
	res, out := retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "401", res, out, cx, 0, Lentes, "401 lineas de producto (>400): las 4 lentes")
	retyMotivos(t, ca, wt, res, out, "normal", "", "declarado normal")
	root, _, cx = retyProyecto(t, "", "feature", "", map[string]string{
		"internal/precio.go":      rdCodigo(400, "Precio"),
		"internal/precio_test.go": rdCodigo(900, "Prueba"),
	})
	res, out = retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "400", res, out, cx, 0, []string{LenteDominante}, MotivoEstandar)
}

// CA-782: con efectivo alto, las 4: declarado, tipo seguridad, una ruta de
// riesgo en producto y una solo en una prueba.
func TestCA782_EfectivoAltoLasCuatro(t *testing.T) {
	const ca = "CA-782"
	chico := map[string]string{"internal/precio.go": rdCodigo(10, "Precio")}
	for _, k := range []struct {
		nombre, tipo, r   string
		archivos          map[string]string
		declarado, motivo string
	}{
		{"declarado alto", "feature", "alto", chico, "", "declarado alto"},
		{"tipo seguridad", "seguridad", "", chico, "normal", "tarjeta de tipo seguridad: riesgo alto"},
		{"ruta de riesgo en producto", "feature", "", map[string]string{"internal/auth/token.go": rdCodigo(10, "Token")}, "normal",
			"el cambio toca internal/auth/token.go: riesgo alto"},
		{"ruta de riesgo solo en una prueba", "feature", "bajo", map[string]string{"internal/spec/tokens_test.go": rdCodigo(10, "Prueba")}, "bajo",
			"el cambio toca internal/spec/tokens_test.go: riesgo alto"},
	} {
		root, wt, cx := retyProyecto(t, "", k.tipo, k.r, k.archivos)
		res, out := retyRevisar(t, ca, root, Options{})
		retyLentes(t, ca, k.nombre, res, out, cx, 0, Lentes, MotivoRiesgoAlto)
		retyMotivos(t, ca, wt, res, out, "alto", k.declarado, k.motivo)
	}
}

// CA-782: con efectivo bajo, reliability aunque haya 3,000 lineas de pruebas
// o 450 de producto dentro de rutas.de_bajo_riesgo (del merge-base).
func TestCA782_BajoCorreReliability(t *testing.T) {
	const ca = "CA-782"
	root, wt, cx := retyProyecto(t, "", "feature", "bajo", map[string]string{"internal/precio_test.go": rdCodigo(3000, "Prueba")})
	res, out := retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "bajo con pruebas", res, out, cx, 0, []string{LenteDominante}, MotivoRiesgoBajo)
	retyMotivos(t, ca, wt, res, out, "bajo", "", "declarado bajo")
	root, wt, cx = retyProyecto(t, "rutas:\n  de_bajo_riesgo: [\"**/*.go\"]\n", "feature", "bajo",
		map[string]string{"grande.go": rdCodigo(450, "Grande")})
	res, out = retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "bajo con 450 de producto en de_bajo_riesgo", res, out, cx, 0, []string{LenteDominante}, MotivoRiesgoBajo)
	retyMotivos(t, ca, wt, res, out, "bajo", "", "declarado bajo")
	// bajo con un producto fuera de de_bajo_riesgo: normal, y chico.
	root, wt, cx = retyProyecto(t, "", "feature", "bajo", map[string]string{"z.go": rdCodigo(10, "Z"), "docs/a.md": "# a\n"})
	res, out = retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "bajo fuera de de_bajo_riesgo", res, out, cx, 0, []string{LenteDominante}, MotivoEstandar)
	retyMotivos(t, ca, wt, res, out, "normal", "bajo", "el cambio toca z.go, fuera de las rutas de bajo riesgo: riesgo normal")
}

// CA-782: solo documentacion no corre lentes, ni con riesgo alto.
func TestCA782_SoloDocumentacionNinguna(t *testing.T) {
	const ca = "CA-782"
	for _, r := range []string{"alto", "normal", "bajo"} {
		root, _, cx := retyProyecto(t, "", "feature", r, map[string]string{"docs/guia.md": strings.Repeat("- linea\n", 900), "README.md": "# x\n"})
		res, err, out := rdCorrer(t, ca, root, Options{Provider: "codex", Task: retySlug})
		if err != nil || len(res.Lenses) != 0 || len(res.Passes) != 0 || cx.veces() != 0 || res.Reason != MotivoSoloDocs {
			t.Fatalf("%s: con %s, solo documentacion no corre lentes (%q): %+v %v\n%s", ca, r, MotivoSoloDocs, res, err, out)
		}
	}
}

// CA-782: --lens manda, con su motivo, aun con riesgo alto.
func TestCA782_LensAMano(t *testing.T) {
	const ca = "CA-782"
	root, wt, cx := retyProyecto(t, "", "feature", "alto", map[string]string{"internal/precio.go": rdCodigo(10, "Precio")})
	res, out := retyRevisar(t, ca, root, Options{Lens: "resilience"})
	retyLentes(t, ca, "--lens", res, out, cx, 0, []string{"resilience"}, MotivoLenteAMano)
	retyMotivos(t, ca, wt, res, out, "alto", "", "declarado alto")
}

// ---------------------------------------------------------------- CA-783

// CA-783: una delta cuyo rango toca una ruta de riesgo vuelve alto el cambio
// entero, y corre risk, reliability y resilience, con el sufijo de la delta.
func TestCA783_DeltaQueTocaUnaRutaCorreRisk(t *testing.T) {
	const ca = "CA-783"
	root, wt, cx := retyProyecto(t, "", "feature", "", map[string]string{"internal/precio.go": rdCodigo(10, "Precio")})
	r1, out := retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "la completa", r1, out, cx, 0, []string{LenteDominante}, MotivoEstandar)
	rdEsperarSegundo()
	rdCommit(t, wt, "toca auth", map[string]string{"internal/auth/token.go": rdCodigo(10, "Token")})
	antes := cx.veces()
	r2, out := retyRevisar(t, ca, root, Options{Delta: true})
	if r2.Status != "revisado" || r2.Cobertura != CoberturaDelta || !reflect.DeepEqual(r2.Lenses, LentesDeUnaDelta) || cx.veces()-antes != 3 {
		t.Fatalf("%s: la delta que toca una ruta de riesgo corre %v: %+v (%d pasadas)\n%s", ca, LentesDeUnaDelta, r2, cx.veces()-antes, out)
	}
	if !strings.HasPrefix(r2.Reason, MotivoRiesgoAlto) || !strings.HasSuffix(r2.Reason, "; una delta no corre readability") {
		t.Fatalf("%s: el motivo es el de riesgo alto con el sufijo de la delta: %q", ca, r2.Reason)
	}
	retyMotivos(t, ca, wt, r2, out, "alto", "normal", "el cambio toca internal/auth/token.go: riesgo alto")
}

// CA-783: el rango se mide en lineas de producto: un rango de 900 lineas de
// pruebas y un .go chico es un cambio estandar, y uno de mas de 400 de
// producto, las 4.
func TestCA783_ElRangoSeMideEnProducto(t *testing.T) {
	const ca = "CA-783"
	bin := raPATH(t)
	root := raRepo(t, "")
	desde := rdCommit(t, root, "codigo chico", map[string]string{"uno.go": "package app\n\nfunc Uno() {}\n"})
	rdCommit(t, root, "pruebas grandes", map[string]string{"grande_test.go": rdCodigo(900, "Prueba"), "dos.go": "package app\n\nfunc Dos() {}\n"})
	cx := raInstalar(t, bin, "codex", "")
	res, out := rdRevisar(t, ca, root, Options{Provider: "codex", Desde: desde})
	if !reflect.DeepEqual(res.Lenses, []string{LenteDominante}) || cx.veces() != 1 || !strings.HasPrefix(res.Reason, MotivoEstandar) {
		t.Fatalf("%s: 900 lineas de pruebas en el rango no son producto: %+v\n%s", ca, res, out)
	}
	if res.LentesMotivo != res.Reason {
		t.Fatalf("%s: lentes_motivo es Reason, con los sufijos del rango: %q %q", ca, res.LentesMotivo, res.Reason)
	}
	root = raRepo(t, "")
	desde = rdCommit(t, root, "codigo chico", map[string]string{"uno.go": "package app\n\nfunc Uno() {}\n"})
	rdCommit(t, root, "producto grande", map[string]string{"grande.go": rdCodigo(401, "Grande")})
	antes := cx.veces()
	res, out = rdRevisar(t, ca, root, Options{Provider: "codex", Desde: desde})
	if !reflect.DeepEqual(res.Lenses, Lentes) || cx.veces()-antes != 4 || !strings.Contains(res.Reason, " lineas de producto (>400): las 4 lentes") {
		t.Fatalf("%s: un rango de 401 lineas de producto lleva las 4: %+v\n%s", ca, res, out)
	}
}

// ---------------------------------------------------------------- CA-784

// retyYAMLEnLaRama commitea en wt el hoom.yaml que da cambiar(el de wt) y
// los archivos.
func retyYAMLEnLaRama(t *testing.T, wt string, cambiar func(string) string, archivos map[string]string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(wt, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	todo := map[string]string{"hoom.yaml": cambiar(string(raw))}
	for k, v := range archivos {
		todo[k] = v
	}
	rdCommit(t, wt, "la rama cambia hoom.yaml", todo)
}

// CA-784: `hoom review` lee rutas del hoom.yaml del merge-base y no de la
// rama: una rama que pone `de_riesgo: []` no afloja su propia review, y una
// que la quita no la endurece.
func TestCA784_LasRutasDelMergeBase(t *testing.T) {
	const ca = "CA-784"
	auth := map[string]string{"internal/auth/token.go": rdCodigo(10, "Token")}
	root, wt, cx := retyProyecto(t, "", "feature", "", nil)
	retyYAMLEnLaRama(t, wt, func(s string) string { return s + "rutas:\n  de_riesgo: []\n" }, auth)
	res, out := retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "la rama vacia de_riesgo", res, out, cx, 0, Lentes, MotivoRiesgoAlto)
	retyMotivos(t, ca, wt, res, out, "alto", "normal", "el cambio toca internal/auth/token.go: riesgo alto")

	root, wt, cx = retyProyecto(t, "rutas:\n  de_riesgo: []\n", "feature", "", nil)
	retyYAMLEnLaRama(t, wt, func(s string) string { return strings.Replace(s, "rutas:\n  de_riesgo: []\n", "", 1) }, auth)
	res, out = retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "el merge-base vacia de_riesgo", res, out, cx, 0, []string{LenteDominante}, MotivoEstandar)
	retyMotivos(t, ca, wt, res, out, "normal", "", "declarado normal")
}

// CA-784: el territorio del test-writer tambien sale del merge-base: lo que
// la rama declara como pruebas no le baja el producto, y lo que el merge-base
// declara si.
func TestCA784_ElTerritorioDelMergeBase(t *testing.T) {
	const ca = "CA-784"
	const allow = "agents:\n  test-writer:\n    write:\n      allow: [\"pruebas/**\"]\n"
	grande := map[string]string{"pruebas/grande.go": rdCodigo(450, "Grande")}
	root, wt, cx := retyProyecto(t, allow, "feature", "", nil)
	retyYAMLEnLaRama(t, wt, func(s string) string { return strings.Replace(s, "pruebas/**", "nada/**", 1) }, grande)
	res, out := retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "el merge-base dice pruebas/**", res, out, cx, 0, []string{LenteDominante}, MotivoEstandar)

	root, wt, cx = retyProyecto(t, "", "feature", "", nil)
	retyYAMLEnLaRama(t, wt, func(s string) string { return s + allow }, grande)
	res, out = retyRevisar(t, ca, root, Options{})
	retyLentes(t, ca, "solo la rama dice pruebas/**", res, out, cx, 0, Lentes, fmt.Sprintf(MotivoProductoGrande, 454))
}
