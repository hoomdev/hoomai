// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// approval: CalibreDe arma la medida desde la tarjeta y hoom.yaml (CA-793,
// CA-794, CA-796) y Aprobar se niega con un spec que pasa su tope sin escribir
// registro, y aprueba despues de subir el tope de la tarjeta (CA-795).
// Escritos desde el spec, las aclaraciones de la ronda 0 y las firmas
// publicas, sin leer la implementacion.
package approval_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const pprYAML = "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

func pprGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func pprEscribir(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// pprTexto es un spec que pasa Lint con exactamente palabras palabras
// (strings.Fields) y k criterios.
func pprTexto(t *testing.T, palabras, k int) string {
	t.Helper()
	var crit strings.Builder
	for i := 1; i <= k; i++ {
		fmt.Fprintf(&crit, "- CA-%d: criterio %d.\n", i, i)
	}
	body := "# Spec\n## Objetivo\nx\n## No-goals\nx\n## Contratos\n%C%\n## Casos limite\nx\n## Criterios de aceptacion\n" + crit.String() + "\n## Decisiones\nx\n## Riesgos\nx\n"
	base := len(strings.Fields(strings.Replace(body, "%C%", "x", 1)))
	if palabras < base {
		t.Fatalf("fixture: el esqueleto ya tiene %d palabras", base)
	}
	body = strings.Replace(body, "%C%", "x"+strings.Repeat(" palabra", palabras-base), 1)
	if n := len(strings.Fields(body)); n != palabras {
		t.Fatalf("fixture: %d palabras, no %d", n, palabras)
	}
	return body
}

// pprItem es el YAML de un item valido con extra.
func pprItem(extra string) string {
	return "titulo: Precios\ntipo: feature\nprioridad: media\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n" + extra
}

// pprProyecto es un repo git con hoom.yaml (yamlExtra), el spec precios con
// palabras/criterios y, si item no es "-", su tarjeta con item de extra.
func pprProyecto(t *testing.T, yamlExtra string, palabras, k int, itemExtra string) (root, specPath string, m *manifest.Manifest) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pprGit(t, root, "init", "-q", "-b", "main")
	pprGit(t, root, "config", "user.email", "test@hoom.dev")
	pprGit(t, root, "config", "user.name", "hoom test")
	pprEscribir(t, root, "hoom.yaml", pprYAML+yamlExtra)
	specPath = pprEscribir(t, root, ".hoom/specs/precios.md", pprTexto(t, palabras, k))
	if itemExtra != "-" {
		pprEscribir(t, root, ".hoom/items/precios.yaml", pprItem(itemExtra))
	}
	pprGit(t, root, "add", "-A")
	pprGit(t, root, "commit", "-q", "-m", "inicial")
	m, err = manifest.Load(root, nil)
	if err != nil {
		t.Fatalf("fixture: hoom.yaml carga: %v", err)
	}
	return root, specPath, m
}

func pprRegistros(t *testing.T, root string) int {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(root, ".hoom", "approvals"))
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(es)
}

func pprLint(t *testing.T, ca string, gates []verdict.GateResult) verdict.GateResult {
	t.Helper()
	for _, g := range gates {
		if g.Name == "spec_lint" {
			return g
		}
	}
	t.Fatalf("%s: GatesCon da spec_lint: %+v", ca, gates)
	return verdict.GateResult{}
}

func pprI(n int) *int       { return &n }
func pprS(s string) *string { return &s }

// CA-793: CalibreDe toma el riesgo declarado de la tarjeta y el tope de
// spec.topes de ese riesgo; sin clave, normal; sin tarjeta, normal y Tarjeta
// ""; un item ilegible da Ilegible con riesgo alto.
func TestCA793_CalibreDeLaTarjeta(t *testing.T) {
	const ca = "CA-793"
	for _, c := range []struct {
		itemExtra string
		riesgo    string
		tope      spec.Tope
	}{
		{"", "normal", spec.Tope{Palabras: 800, Criterios: 8, CitasPorCriterio: 10}},
		{"riesgo: bajo\n", "bajo", spec.Tope{Palabras: 400, Criterios: 4, CitasPorCriterio: 10}},
		{"riesgo: alto\n", "alto", spec.Tope{Palabras: 2000, Criterios: 15, CitasPorCriterio: 0}},
	} {
		root, sp, m := pprProyecto(t, "", 100, 2, c.itemExtra)
		cal := approval.CalibreDe(m, root, sp)
		if cal.Tarjeta != "precios" || cal.Ilegible != "" || cal.Riesgo != c.riesgo || cal.Tope != c.tope ||
			cal.PalabrasDeLaTarjeta || cal.CriteriosDeLaTarjeta || cal.Aprobado {
			t.Fatalf("%s: la tarjeta %q da riesgo %s y tope %+v: %+v", ca, c.itemExtra, c.riesgo, c.tope, cal)
		}
		// un manifiesto nil da los mismos (los de defecto)
		if nilc := approval.CalibreDe(nil, root, sp); nilc.Tope != c.tope || nilc.Riesgo != c.riesgo {
			t.Fatalf("%s: con m nil los topes por defecto: %+v", ca, nilc)
		}
		// la ruta relativa a root vale igual
		if rel := approval.CalibreDe(m, root, ".hoom/specs/precios.md"); rel.Tarjeta != "precios" || rel.Tope != c.tope {
			t.Fatalf("%s: specPath relativo a root: %+v", ca, rel)
		}
	}
	// sin item: sin tarjeta, normal
	root, sp, m := pprProyecto(t, "", 100, 2, "-")
	if cal := approval.CalibreDe(m, root, sp); cal.Tarjeta != "" || cal.Riesgo != "normal" || cal.Tope != (spec.Tope{Palabras: 800, Criterios: 8, CitasPorCriterio: 10}) {
		t.Fatalf("%s: un spec sin item no tiene tarjeta y vale normal: %+v", ca, cal)
	}
	// un spec en otra ruta no tiene tarjeta, aunque haya un item con su nombre
	root, _, m = pprProyecto(t, "", 100, 2, "riesgo: alto\n")
	otra := pprEscribir(t, root, "docs/precios.md", pprTexto(t, 100, 2))
	if cal := approval.CalibreDe(m, root, otra); cal.Tarjeta != "" || cal.Riesgo != "normal" {
		t.Fatalf("%s: un spec fuera de .hoom/specs no tiene tarjeta: %+v", ca, cal)
	}
	// item ilegible
	root, sp, m = pprProyecto(t, "", 100, 2, "riesgo: [roto\n")
	cal := approval.CalibreDe(m, root, sp)
	if cal.Tarjeta != "precios" || cal.Ilegible == "" || cal.Riesgo != "alto" {
		t.Fatalf("%s: un item ilegible es Ilegible con riesgo alto: %+v", ca, cal)
	}
	gates, _ := spec.GatesCon(root, sp, cal)
	if g := pprLint(t, ca, gates); g.Status != verdict.StatusError || !strings.HasPrefix(g.Notes, "no se puede leer la tarjeta precios: ") {
		t.Fatalf("%s: la tarjeta ilegible da spec_lint en error: %+v", ca, g)
	}
}

// CA-793: los topes salen de spec.topes del hoom.yaml que se le da, con los
// de defecto para lo que falta.
func TestCA793_CalibreDeConSpecTopes(t *testing.T) {
	const ca = "CA-793"
	root, sp, m := pprProyecto(t, "spec:\n  topes:\n    bajo: { palabras: 100 }\n", 150, 2, "riesgo: bajo\n")
	cal := approval.CalibreDe(m, root, sp)
	if cal.Tope != (spec.Tope{Palabras: 100, Criterios: 4, CitasPorCriterio: 10}) {
		t.Fatalf("%s: spec.topes.bajo.palabras 100 y el resto por defecto: %+v", ca, cal.Tope)
	}
	gates, _ := spec.GatesCon(root, sp, cal)
	g := pprLint(t, ca, gates)
	want := `el spec tiene 150 palabras y el tope con riesgo bajo es 100: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set precios --tope-palabras 150 --motivo "<por que>"`
	if g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, want) {
		t.Fatalf("%s: spec_lint en fail con\n%s\nfue %+v", ca, want, g)
	}
}

// CA-796: el tope de la tarjeta reemplaza al de hoom.yaml solo para ella,
// mayor o menor; su 0 es sin tope; las citas siempre son de hoom.yaml.
func TestCA796_ElTopeDeLaTarjetaSoloParaElla(t *testing.T) {
	const ca = "CA-796"
	root, sp, m := pprProyecto(t, "spec:\n  topes:\n    alto: { citas_por_criterio: 7 }\n", 100, 2,
		"riesgo: alto\ntope_palabras: 3000\ntope_criterios: 0\ntope_motivo: x\n")
	cal := approval.CalibreDe(m, root, sp)
	if cal.Tope != (spec.Tope{Palabras: 3000, Criterios: 0, CitasPorCriterio: 7}) || !cal.PalabrasDeLaTarjeta || !cal.CriteriosDeLaTarjeta {
		t.Fatalf("%s: la tarjeta pone palabras 3000 y criterios 0 (sin tope); las citas de hoom.yaml: %+v", ca, cal)
	}
	// una segunda tarjeta del mismo proyecto, sin tope propio, usa el de hoom.yaml
	pprEscribir(t, root, ".hoom/items/otra.yaml", pprItem("riesgo: alto\n"))
	otra := pprEscribir(t, root, ".hoom/specs/otra.md", pprTexto(t, 100, 2))
	if c := approval.CalibreDe(m, root, otra); c.Tope != (spec.Tope{Palabras: 2000, Criterios: 15, CitasPorCriterio: 7}) || c.PalabrasDeLaTarjeta || c.CriteriosDeLaTarjeta {
		t.Fatalf("%s: la otra tarjeta usa el tope de hoom.yaml: %+v", ca, c)
	}
	// un tope de la tarjeta menor que el de hoom.yaml tambien vale
	root, sp, m = pprProyecto(t, "", 400, 2, "tope_palabras: 300\ntope_motivo: corto\n")
	cal = approval.CalibreDe(m, root, sp)
	if cal.Tope.Palabras != 300 || !cal.PalabrasDeLaTarjeta || cal.Tope.Criterios != 8 || cal.CriteriosDeLaTarjeta {
		t.Fatalf("%s: el tope menor de la tarjeta es el de la tarjeta: %+v", ca, cal)
	}
	gates, _ := spec.GatesCon(root, sp, cal)
	want := `el spec tiene 400 palabras y el tope de la tarjeta es 300: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set precios --tope-palabras 400 --motivo "<por que>"`
	if g := pprLint(t, ca, gates); g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, want) {
		t.Fatalf("%s: 400 palabras con el tope 300 de la tarjeta:\n%s\nfue %+v", ca, want, g)
	}
	// tope_palabras 0 en la tarjeta con un spec enorme: pasa
	root, sp, m = pprProyecto(t, "", 3000, 2, "tope_palabras: 0\ntope_motivo: sin tope\n")
	gates, _ = spec.GatesCon(root, sp, approval.CalibreDe(m, root, sp))
	if g := pprLint(t, ca, gates); g.Status != verdict.StatusPass || !strings.HasPrefix(g.Notes, "3000 palabras sin tope · 2 criterios de 8 · riesgo normal") {
		t.Fatalf("%s: el 0 de la tarjeta es sin tope aunque hoom.yaml tenga uno: %+v", ca, g)
	}
}

// CA-794: un spec con aprobacion vigente que pasa su tope sigue en pass con
// la nota; si cambia despues de aprobarse, se mide y falla.
func TestCA794_AprobadoNoSeMideYEditadoSi(t *testing.T) {
	const ca = "CA-794"
	root, sp, m := pprProyecto(t, "", 1200, 3, "")
	if _, _, err := approval.Approve(root, sp); err != nil {
		t.Fatal(err)
	}
	cal := approval.CalibreDe(m, root, sp)
	if !cal.Aprobado {
		t.Fatalf("%s: con la aprobacion vigente Aprobado es true: %+v", ca, cal)
	}
	gates, _ := spec.GatesCon(root, sp, cal)
	g := pprLint(t, ca, gates)
	if g.Status != verdict.StatusPass || !strings.HasSuffix(g.Notes, " · aprobado: el tope no se mide") ||
		!strings.HasPrefix(g.Notes, "1200 palabras de 800 · 3 criterios de 8 · riesgo normal · 3 ids CA-n") {
		t.Fatalf("%s: aprobado queda en pass con la nota: %+v", ca, g)
	}
	// Aprobar un spec ya aprobado no lo mide: already, sin error
	if _, already, err := approval.Aprobar(m, root, sp); err != nil || !already {
		t.Fatalf("%s: un spec aprobado no se mide: %v %v", ca, already, err)
	}
	// cambia despues de aprobarse
	b, _ := os.ReadFile(sp)
	if err := os.WriteFile(sp, append(b, []byte("una enmienda\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	cal = approval.CalibreDe(m, root, sp)
	if cal.Aprobado {
		t.Fatalf("%s: un spec cambiado no tiene aprobacion vigente: %+v", ca, cal)
	}
	gates, _ = spec.GatesCon(root, sp, cal)
	if g := pprLint(t, ca, gates); g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, "el spec tiene 1202 palabras y el tope con riesgo normal es 800") {
		t.Fatalf("%s: editado se mide otra vez y falla: %+v", ca, g)
	}
}

// CA-795: Aprobar se niega con un spec que pasa su tope, con las lineas de
// spec_lint unidas por "\n", sin escribir registro; con la tarjeta ilegible,
// con su texto; despues de subir el tope de la tarjeta, aprueba.
func TestCA795_AprobarSeNiegaYDespuesDeSubirElTopeAprueba(t *testing.T) {
	const ca = "CA-795"
	root, sp, m := pprProyecto(t, "", 900, 9, "")
	_, _, err := approval.Aprobar(m, root, sp)
	want := `el spec tiene 900 palabras y el tope con riesgo normal es 800: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set precios --tope-palabras 900 --motivo "<por que>"` + "\n" +
		`el spec tiene 9 criterios y el tope con riesgo normal es 8: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set precios --tope-criterios 9 --motivo "<por que>"`
	if err == nil || err.Error() != want {
		t.Fatalf("%s: Aprobar se niega con las lineas:\n%s\nfue\n%v", ca, want, err)
	}
	if n := pprRegistros(t, root); n != 0 {
		t.Fatalf("%s: la negativa no escribe registro: %d", ca, n)
	}
	if st, _, _ := approval.Status(root, sp); st != approval.StatusNotApproved {
		t.Fatalf("%s: el spec sigue sin aprobar: %s", ca, st)
	}
	// subir solo las palabras: sigue negado por los criterios
	if _, _, _, err := item.Cambiar(root, "precios", item.Cambios{TopePalabras: pprI(900), TopeMotivo: pprS("el intake es largo")}); err != nil {
		t.Fatal(err)
	}
	_, _, err = approval.Aprobar(m, root, sp)
	if err == nil || strings.Contains(err.Error(), "palabras") || !strings.Contains(err.Error(), "9 criterios") || pprRegistros(t, root) != 0 {
		t.Fatalf("%s: con las palabras subidas queda la linea de criterios: %v", ca, err)
	}
	if _, _, _, err := item.Cambiar(root, "precios", item.Cambios{TopeCriterios: pprI(9), TopeMotivo: pprS("nueve")}); err != nil {
		t.Fatal(err)
	}
	rec, already, err := approval.Aprobar(m, root, sp)
	if err != nil || already || rec.SHA256 == "" || pprRegistros(t, root) != 1 {
		t.Fatalf("%s: despues de subir el tope de la tarjeta aprueba: %+v %v %v", ca, rec, already, err)
	}
	if st, _, _ := approval.Status(root, sp); st != approval.StatusApproved {
		t.Fatalf("%s: queda aprobado: %s", ca, st)
	}
	// dentro del tope aprueba de una, y otra vez es already
	root, sp, m = pprProyecto(t, "", 800, 8, "")
	if _, already, err := approval.Aprobar(m, root, sp); err != nil || already || pprRegistros(t, root) != 1 {
		t.Fatalf("%s: en el tope exacto aprueba: %v %v", ca, already, err)
	}
	if _, already, err := approval.Aprobar(m, root, sp); err != nil || !already || pprRegistros(t, root) != 1 {
		t.Fatalf("%s: aprobar otra vez es already: %v %v", ca, already, err)
	}
	// tarjeta ilegible
	root, sp, m = pprProyecto(t, "", 100, 2, "tope_palabras: -1\n")
	_, _, err = approval.Aprobar(m, root, sp)
	if err == nil || !strings.HasPrefix(err.Error(), "no se puede leer la tarjeta precios: ") || pprRegistros(t, root) != 0 {
		t.Fatalf("%s: con la tarjeta ilegible se niega con su texto, sin registro: %v", ca, err)
	}
	// sin tarjeta: las lineas nombran spec.topes
	root, sp, m = pprProyecto(t, "", 801, 2, "-")
	_, _, err = approval.Aprobar(m, root, sp)
	if err == nil || err.Error() != "el spec tiene 801 palabras y el tope con riesgo normal es 800: reduce el alcance o sube spec.topes.normal.palabras en hoom.yaml" || pprRegistros(t, root) != 0 {
		t.Fatalf("%s: sin tarjeta se niega nombrando spec.topes: %v", ca, err)
	}
}

// CA-795: un spec aprobado y luego editado por encima del tope: Aprobar se
// niega y el registro viejo queda como estaba (uno).
func TestCA795_EnmiendaPorEncimaDelTopeSeNiega(t *testing.T) {
	const ca = "CA-795"
	root, sp, m := pprProyecto(t, "", 700, 3, "")
	if _, _, err := approval.Aprobar(m, root, sp); err != nil {
		t.Fatalf("fixture: dentro del tope aprueba: %v", err)
	}
	if err := os.WriteFile(sp, []byte(pprTexto(t, 850, 3)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := approval.Aprobar(m, root, sp)
	if err == nil || !strings.Contains(err.Error(), "el spec tiene 850 palabras y el tope con riesgo normal es 800") || pprRegistros(t, root) != 1 {
		t.Fatalf("%s: la enmienda por encima del tope se niega sin registro nuevo: %v", ca, err)
	}
	if st, _, _ := approval.Status(root, sp); st != approval.StatusInvalidated {
		t.Fatalf("%s: queda invalidado: %s", ca, st)
	}
}
