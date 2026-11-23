// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// verifycmd: el veredicto trae riesgo{declarado, efectivo, motivo} y
// tamano{producto, pruebas, hoom} (CA-780, CA-774), la tarea sale del sobre o
// del worktree .hoom/worktrees/<slug> y no del checkout principal (CA-780),
// la tarjeta se lee del principal (CA-779), la huella no cambia (CA-780), y
// el gate findings_open juzga con el efectivo que calculo ese verify y con
// rutas.de_riesgo (CA-787). Escritos desde el spec, las aclaraciones de la
// ronda 0 y las firmas publicas, sin leer la implementacion.
//
// Sin perfiles: los gates del proyecto son "true", y las generadas van en
// rutas.generadas. verifycmd no importa reviewcmd en sus tests (reviewcmd lo
// importa): el registro de review se escribe a mano, con el formato minimo
// que ya usan los tests de finding.
package verifycmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const retySlug = "precios"

// retyTexto es un texto de n lineas, cada una con salto.
func retyTexto(n int, marca string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(marca + " " + strings.Repeat("x", i%5) + "\n")
	}
	return b.String()
}

// retyItem es un item valido de tipo tipo; riesgo "" = sin la clave.
func retyItem(tipo, riesgo string) string {
	body := "titulo: Precios\ntipo: " + tipo + "\nprioridad: media\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-11-20T10:00:00Z\n"
	if riesgo != "" {
		body += "riesgo: " + riesgo + "\n"
	}
	return body
}

// retyProyecto es un proyecto git con un gate "true", extra en su hoom.yaml,
// el item precios (de tipo tipo y con riesgo r; tipo "" = sin item)
// commiteado en main, y el worktree de la tarea como lo crea `hoom task
// start` (.hoom/worktrees/precios en la rama hoom/precios).
func retyProyecto(t *testing.T, extra, tipo, r string) (root, wt string) {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	cdwHome(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cdwGit(t, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	cdwGit(t, root, "config", "user.email", "test@hoom.dev")
	cdwGit(t, root, "config", "user.name", "hoom test")
	cdwEscribir(t, filepath.Join(root, manifest.FileName), "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n"+extra)
	cdwEscribir(t, filepath.Join(root, ".hoom", ".gitignore"), "cache/\nworktrees/\nruns/\n")
	cdwEscribir(t, filepath.Join(root, "app.go"), "package app\n")
	if tipo != "" {
		cdwEscribir(t, filepath.Join(root, ".hoom", "items", retySlug+".yaml"), retyItem(tipo, r))
	}
	cdwGit(t, root, "add", "-A")
	cdwGit(t, root, "commit", "-q", "-m", "inicial")
	wt = filepath.Join(root, ".hoom", "worktrees", retySlug)
	cdwGit(t, root, "worktree", "add", "-q", "-b", "hoom/"+retySlug, wt, "main")
	return root, wt
}

func retyCargar(t *testing.T, dir string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatalf("fixture: manifest.Load(%s): %v", dir, err)
	}
	return m
}

func retyVerify(t *testing.T, ca, dir string, opt Options) (*verdict.Verdict, string) {
	t.Helper()
	opt.Full = true
	v, path, err := Run(retyCargar(t, dir), opt)
	if err != nil {
		t.Fatalf("%s: verify corre: %v", ca, err)
	}
	return v, path
}

func retyRiesgo(t *testing.T, ca string, v *verdict.Verdict, decl, ef, mot string) {
	t.Helper()
	want := riesgo.Evaluacion{Declarado: decl, Efectivo: ef, Motivo: mot}
	if v.Riesgo == nil || *v.Riesgo != want {
		t.Fatalf("%s: el riesgo del veredicto es %+v, fue %+v", ca, want, v.Riesgo)
	}
}

// retyClaves lee el JSON del veredicto de path como un mapa de claves.
func retyClaves(t *testing.T, path string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("el veredicto es JSON: %v\n%s", err, raw)
	}
	return m
}

// ---------------------------------------------------------------- CA-780

// CA-780 y CA-774: en el worktree de la tarea, el veredicto trae la tarea,
// el riesgo con la tarjeta del principal y el tamano en tres numeros (90 de
// producto, 1500 de pruebas, el generado en ninguno, .hoom/ en hoom); el JSON
// trae riesgo y tamano con sus claves y no la tarea; la huella y git.* son los
// del arbol, como sin estas claves.
func TestCA780_ElVeredictoDeLaTareaTraeRiesgoYTamano(t *testing.T) {
	const ca = "CA-780"
	_, wt := retyProyecto(t, "rutas:\n  generadas: [\"vendor\"]\n", "feature", "bajo")
	cdwEscribir(t, filepath.Join(wt, "internal", "precio.go"), retyTexto(60, "func"))
	cdwGit(t, wt, "add", "-A")
	cdwGit(t, wt, "commit", "-q", "-m", "parte del producto")
	cdwEscribir(t, filepath.Join(wt, "internal", "precio_extra.go"), retyTexto(30, "var"))
	cdwEscribir(t, filepath.Join(wt, "internal", "precio_test.go"), retyTexto(1500, "assert"))
	cdwEscribir(t, filepath.Join(wt, "vendor", "lib", "auth.go"), retyTexto(500, "lib"))
	cdwEscribir(t, filepath.Join(wt, ".hoom", "specs", "precios.md"), retyTexto(30, "# spec"))
	antes := gitx.Snapshot(wt, "main")
	v, path := retyVerify(t, ca, wt, Options{})
	if v.Tarea != retySlug {
		t.Fatalf("%s: la tarea del worktree .hoom/worktrees/%s es %q, fue %q", ca, retySlug, retySlug, v.Tarea)
	}
	retyRiesgo(t, ca, v, "bajo", "normal", "el cambio toca internal/precio.go, fuera de las rutas de bajo riesgo: riesgo normal")
	if v.Tamano == nil || v.Tamano.Producto != 90 || v.Tamano.Pruebas != 1500 || v.Tamano.Hoom < 30 {
		t.Fatalf("%s: el tamano es producto 90, pruebas 1500 y hoom al menos 30: %+v", ca, v.Tamano)
	}
	if v.Git.ChangeFingerprint != antes.ChangeFingerprint || v.Git.Insertions != antes.Insertions || v.Git.Deletions != antes.Deletions {
		t.Fatalf("%s: la huella y git.* son las del arbol: %+v vs %+v", ca, v.Git, antes)
	}
	claves := retyClaves(t, path)
	var r map[string]string
	if err := json.Unmarshal(claves["riesgo"], &r); err != nil || len(r) != 3 || r["declarado"] != "bajo" || r["efectivo"] != "normal" ||
		r["motivo"] != "el cambio toca internal/precio.go, fuera de las rutas de bajo riesgo: riesgo normal" {
		t.Fatalf("%s: el JSON trae riesgo{declarado, efectivo, motivo}: %s %v", ca, claves["riesgo"], err)
	}
	var tam map[string]int
	if err := json.Unmarshal(claves["tamano"], &tam); err != nil || len(tam) != 3 || tam["producto"] != 90 || tam["pruebas"] != 1500 {
		t.Fatalf("%s: el JSON trae tamano{producto, pruebas, hoom}: %s %v", ca, claves["tamano"], err)
	}
	if _, ok := tam["hoom"]; !ok {
		t.Fatalf("%s: el JSON de tamano trae hoom: %s", ca, claves["tamano"])
	}
	for k := range claves {
		if strings.EqualFold(k, "tarea") {
			t.Fatalf("%s: la tarea no va en el JSON del veredicto: %q", ca, k)
		}
	}
	all, err := verdict.LoadAll(wt)
	if err != nil || len(all) == 0 {
		t.Fatalf("%s: LoadAll: %v", ca, err)
	}
	leido := all[len(all)-1]
	if leido.Tarea != "" || leido.Riesgo == nil || *leido.Riesgo != *v.Riesgo || leido.Tamano == nil || *leido.Tamano != *v.Tamano {
		t.Fatalf("%s: el veredicto leido trae riesgo y tamano, y la tarea vacia: %+v", ca, leido)
	}
}

// CA-780: en el checkout principal no hay tarea: el riesgo es el de sin
// tarjeta, o el de una ruta de riesgo.
func TestCA780_EnElPrincipalNoHayTarea(t *testing.T) {
	const ca = "CA-780"
	root, _ := retyProyecto(t, "", "feature", "bajo")
	v, _ := retyVerify(t, ca, root, Options{})
	if v.Tarea != "" {
		t.Fatalf("%s: el checkout principal no tiene tarea: %q", ca, v.Tarea)
	}
	retyRiesgo(t, ca, v, "normal", "normal", "sin tarjeta: riesgo normal")
	if v.Tamano == nil || v.Tamano.Producto != 0 || v.Tamano.Pruebas != 0 {
		t.Fatalf("%s: sin cambios, producto y pruebas son 0: %+v", ca, v.Tamano)
	}
	root, _ = retyProyecto(t, "", "feature", "bajo")
	cdwEscribir(t, filepath.Join(root, "internal", "auth", "x.go"), retyTexto(10, "auth"))
	v, _ = retyVerify(t, ca, root, Options{})
	retyRiesgo(t, ca, v, "normal", "alto", "el cambio toca internal/auth/x.go: riesgo alto")
	if v.Tamano == nil || v.Tamano.Producto != 10 {
		t.Fatalf("%s: el producto del principal es 10: %+v", ca, v.Tamano)
	}
}

// CA-780: con Sobre la tarea es Sobre.Task, aunque el arbol no sea el
// worktree; una tarea sin item es `declarado normal` (aclaracion 9).
func TestCA780_LaTareaDelSobre(t *testing.T) {
	const ca = "CA-780"
	root, _ := retyProyecto(t, "", "feature", "bajo")
	cdwEscribir(t, filepath.Join(root, "docs", "a.md"), retyTexto(5, "doc"))
	v, _ := retyVerify(t, ca, root, Options{Sobre: &Sobre{Root: root, Task: retySlug, Rol: "writer"}})
	if v.Tarea != retySlug {
		t.Fatalf("%s: con Sobre la tarea es Sobre.Task: %q", ca, v.Tarea)
	}
	retyRiesgo(t, ca, v, "bajo", "bajo", "declarado bajo")
	root, wt := retyProyecto(t, "", "feature", "bajo")
	v, _ = retyVerify(t, ca, wt, Options{Sobre: &Sobre{Root: root, Task: "otra", Rol: "writer"}})
	if v.Tarea != "otra" {
		t.Fatalf("%s: Sobre.Task gana sobre el worktree: %q", ca, v.Tarea)
	}
	retyRiesgo(t, ca, v, "normal", "normal", "declarado normal")
}

// CA-780: fuera de un repo git no hay tamano y el riesgo se calcula sin
// archivos; con una base que no existe tampoco hay tamano (aclaracion 3).
func TestCA780_SinRepoOSinBaseNoHayTamano(t *testing.T) {
	const ca = "CA-780"
	cdwHome(t)
	m := loadProject(t)
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(m.Dir))
	v, path, err := Run(m, Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	if v.Tamano != nil || v.Tarea != "" {
		t.Fatalf("%s: fuera de un repo no hay tamano ni tarea: %+v %q", ca, v.Tamano, v.Tarea)
	}
	retyRiesgo(t, ca, v, "normal", "normal", "sin tarjeta: riesgo normal")
	claves := retyClaves(t, path)
	if _, ok := claves["tamano"]; ok {
		t.Fatalf("%s: fuera de un repo no va la clave tamano", ca)
	}
	if _, ok := claves["riesgo"]; !ok {
		t.Fatalf("%s: la clave riesgo va siempre", ca)
	}
	root, _ := retyProyecto(t, "", "feature", "")
	cdwEscribir(t, filepath.Join(root, manifest.FileName), "schema: hoom/v1\nproject: demo\nbase_branch: no-existe\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	v, _ = retyVerify(t, ca, root, Options{})
	if v.Tamano != nil {
		t.Fatalf("%s: si falla el merge-base no hay tamano: %+v", ca, v.Tamano)
	}
	if v.Riesgo == nil || v.Riesgo.Motivo == "" {
		t.Fatalf("%s: sin tamano igual hay riesgo con motivo: %+v", ca, v.Riesgo)
	}
}

// ---------------------------------------------------------------- CA-779

// CA-779 por verify: una tarjeta de tipo seguridad y una ilegible del
// principal dan alto; una que solo esta en la rama se lee del arbol
// (aclaracion 10).
func TestCA779_LaTarjetaDelVerify(t *testing.T) {
	const ca = "CA-779"
	_, wt := retyProyecto(t, "", "seguridad", "")
	v, _ := retyVerify(t, ca, wt, Options{})
	retyRiesgo(t, ca, v, "normal", "alto", "tarjeta de tipo seguridad: riesgo alto")
	root, wt := retyProyecto(t, "", "feature", "bajo")
	cdwEscribir(t, filepath.Join(root, ".hoom", "items", retySlug+".yaml"), "titulo: [sin cerrar\n")
	v, _ = retyVerify(t, ca, wt, Options{})
	retyRiesgo(t, ca, v, "normal", "alto", "no se pudo leer la tarjeta: riesgo alto")
	_, wt = retyProyecto(t, "", "", "")
	cdwEscribir(t, filepath.Join(wt, ".hoom", "items", retySlug+".yaml"), retyItem("feature", "alto"))
	v, _ = retyVerify(t, ca, wt, Options{})
	retyRiesgo(t, ca, v, "alto", "alto", "declarado alto")
	// El item del principal gana sobre el de la rama.
	root, wt = retyProyecto(t, "", "feature", "bajo")
	cdwEscribir(t, filepath.Join(wt, ".hoom", "items", retySlug+".yaml"), retyItem("feature", "alto"))
	cdwEscribir(t, filepath.Join(wt, "docs", "a.md"), "doc\n")
	v, _ = retyVerify(t, ca, wt, Options{})
	retyRiesgo(t, ca, v, "bajo", "bajo", "declarado bajo")
}

// ---------------------------------------------------------------- CA-774

// CA-774 por verify: con agents.test-writer.write.allow, `prueba` es ese
// territorio: un _test.go fuera de el es producto.
func TestCA774_VerifyConElTerritorioDelTestWriter(t *testing.T) {
	const ca = "CA-774"
	_, wt := retyProyecto(t, "agents:\n  test-writer:\n    write:\n      allow: [\"pruebas/**\"]\n", "feature", "")
	cdwEscribir(t, filepath.Join(wt, "pruebas", "precio.go"), retyTexto(1500, "p"))
	cdwEscribir(t, filepath.Join(wt, "internal", "precio_test.go"), retyTexto(90, "t"))
	cdwEscribir(t, filepath.Join(wt, ".hoom", "x_test.go"), retyTexto(7, "h"))
	v, _ := retyVerify(t, ca, wt, Options{})
	if v.Tamano == nil || v.Tamano.Producto != 90 || v.Tamano.Pruebas != 1500 || v.Tamano.Hoom < 7 {
		t.Fatalf("%s: con allow [pruebas/**] el tamano es producto 90, pruebas 1500: %+v", ca, v.Tamano)
	}
}

// ---------------------------------------------------------------- CA-787

// retyRegistro escribe a mano un registro de review completa de task con
// los hallazgos dados, en dir/.hoom/reviews/.
func retyRegistro(t *testing.T, dir, id, task string, hallazgos []string) {
	t.Helper()
	ids, err := json.Marshal(hallazgos)
	if err != nil {
		t.Fatal(err)
	}
	rec := `{"id":"` + id + `","created_at":"` + time.Now().UTC().Format(time.RFC3339) + `","task":"` + task +
		`","findings":` + string(ids) + `,"cobertura":"completa","lenses":["risk","reliability","resilience","readability"]}`
	cdwEscribir(t, filepath.Join(dir, ".hoom", "reviews", id+".json"), rec)
}

func retyGateHallazgos(t *testing.T, ca string, v *verdict.Verdict) *verdict.GateResult {
	t.Helper()
	for i := range v.Gates {
		if v.Gates[i].Name == finding.GateName {
			return &v.Gates[i]
		}
	}
	t.Fatalf("%s: el veredicto trae el gate %s: %+v", ca, finding.GateName, v.Gates)
	return nil
}

// CA-787: en verify, el medium de la review completa de la tarea bloquea con
// el efectivo que calcula ese verify (alto por una ruta de riesgo), aunque la
// tarjeta declare normal; sin la ruta, no bloquea.
func TestCA787_VerifyBloqueaConElEfectivoQueCalcula(t *testing.T) {
	const ca = "CA-787"
	const id = "20261120T100000_abcdef"
	for _, conRuta := range []bool{true, false} {
		_, wt := retyProyecto(t, "findings:\n  block_on: high\n", "feature", "")
		if conRuta {
			cdwEscribir(t, filepath.Join(wt, "internal", "auth", "x.go"), retyTexto(10, "auth"))
		} else {
			cdwEscribir(t, filepath.Join(wt, "internal", "precio.go"), retyTexto(10, "precio"))
		}
		f, err := finding.Register(wt, "main", finding.Draft{Severity: "medium", Lens: "risk", File: "app.go",
			Description: "medium de la completa", Author: "codex", Task: retySlug})
		if err != nil {
			t.Fatalf("fixture: Register: %v", err)
		}
		retyRegistro(t, wt, id, retySlug, []string{f.ID})
		v, _ := retyVerify(t, ca, wt, Options{})
		g := retyGateHallazgos(t, ca, v)
		motivo := "medium de la review completa " + id + ": riesgo alto"
		if conRuta {
			retyRiesgo(t, ca, v, "normal", "alto", "el cambio toca internal/auth/x.go: riesgo alto")
			if g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, motivo) || v.Verdict != "red" {
				t.Fatalf("%s: con efectivo alto el medium de la completa bloquea con %q: %+v (%s)", ca, motivo, g, v.Verdict)
			}
		} else {
			retyRiesgo(t, ca, v, "normal", "normal", "declarado normal")
			if g.Status != verdict.StatusPass || strings.Contains(g.OutputTail, motivo) {
				t.Fatalf("%s: con efectivo normal el medium de la completa no bloquea: %+v", ca, g)
			}
		}
	}
}

// CA-787: el medium «en ruta de riesgo» se mide contra rutas.de_riesgo del
// hoom.yaml: con una lista propia, su patron bloquea y una marca por defecto
// ya no.
func TestCA787_VerifyMideElMediumContraRutasDeRiesgo(t *testing.T) {
	const ca = "CA-787"
	_, wt := retyProyecto(t, "findings:\n  block_on: high\nrutas:\n  de_riesgo: [\"app/Domain/Costeo\"]\n", "feature", "")
	for _, file := range []string{"app/Domain/Costeo/Lote.php", "internal/auth/x.go"} {
		if _, err := finding.Register(wt, "main", finding.Draft{Severity: "medium", Lens: "risk", File: file,
			Description: "medium en " + file, Author: "codex"}); err != nil {
			t.Fatalf("fixture: Register: %v", err)
		}
	}
	v, _ := retyVerify(t, ca, wt, Options{})
	g := retyGateHallazgos(t, ca, v)
	if g.Status != verdict.StatusFail || !strings.Contains(g.OutputTail, "medium en ruta de riesgo: app/Domain/Costeo/Lote.php") {
		t.Fatalf("%s: el medium en app/Domain/Costeo bloquea por la ruta propia: %+v", ca, g)
	}
	if strings.Contains(g.OutputTail, "medium en ruta de riesgo: internal/auth/x.go") {
		t.Fatalf("%s: con de_riesgo propia, internal/auth ya no es ruta de riesgo: %q", ca, g.OutputTail)
	}
}
