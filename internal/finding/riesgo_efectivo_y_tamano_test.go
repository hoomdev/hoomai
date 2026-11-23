// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// finding (CA-786, CA-787): RiesgoDeTarjeta (el mas alto entre el efectivo
// del ultimo veredicto y el declarado de ahora; sin veredicto, el declarado;
// un veredicto anterior al spec, medido con las reglas), Bloquea con el
// medium en ruta de riesgo contra rutas.de_riesgo y el medium de la review
// completa con el riesgo efectivo, ContextoDe con el riesgo de la tarjeta de
// cada tarea, y GateDelVerify con lo que midio el verify. Paquete externo:
// reviewcmd importa finding. Escritos desde el spec, las aclaraciones de la
// ronda 0 y las firmas publicas, sin leer la implementacion.
package finding_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	retyMotAuth = "el cambio toca internal/auth/x.go: riesgo alto"
	retyMotApp  = "el cambio toca a.go, fuera de las rutas de bajo riesgo: riesgo normal"
)

func retyGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func retyEscribir(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// retyRepo es un repo git con app.go y los specs de las tareas dadas, fuera
// de la corrida de un rol.
func retyRepo(t *testing.T, tareas ...string) string {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	retyGit(t, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	retyGit(t, root, "config", "user.email", "test@hoom.dev")
	retyGit(t, root, "config", "user.name", "hoom test")
	retyEscribir(t, root, "app.go", "package app\n")
	for _, s := range tareas {
		retyEscribir(t, root, ".hoom/specs/"+s+".md", "# Spec "+s+"\n")
	}
	retyGit(t, root, "add", "-A")
	retyGit(t, root, "commit", "-q", "-m", "inicial")
	return root
}

func retyItem(t *testing.T, root, slug, r string) {
	t.Helper()
	if _, err := item.Add(root, item.Draft{Titulo: "Tarea " + slug, Slug: slug, Riesgo: r}); err != nil {
		t.Fatalf("fixture: item.Add: %v", err)
	}
}

func retyAlta(t *testing.T, root, sev, file, task string) finding.Finding {
	t.Helper()
	f, err := finding.Register(root, "main", finding.Draft{Severity: sev, Lens: "risk", File: file,
		Description: "h " + sev + " en " + file, Author: "reviewer@test", Task: task})
	if err != nil {
		t.Fatalf("fixture: Register: %v", err)
	}
	return f
}

var retySeq int

// retyCompleta escribe un registro de review completa de task con esos
// hallazgos.
func retyCompleta(t *testing.T, root, task string, hallazgos []string) reviewcmd.Record {
	t.Helper()
	retySeq++
	r, err := reviewcmd.WriteRecord(root, reviewcmd.Record{CreatedAt: time.Date(2026, 11, 20, 10, retySeq%60, 0, 0, time.UTC),
		Task: task, Spec: ".hoom/specs/" + task + ".md", Fingerprint: "h", VerdictID: "v", Verdict: "green",
		Lenses: reviewcmd.Lentes, Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: hallazgos,
		Usage: []reviewcmd.LensUsage{}, Cobertura: reviewcmd.CoberturaCompleta, Base: "main",
		Desde: strings.Repeat("a", 40), Hasta: strings.Repeat("b", 40)})
	if err != nil {
		t.Fatalf("fixture: WriteRecord: %v", err)
	}
	return r
}

// retyVeredicto escribe un veredicto completo de la tarea slug, con ev de
// riesgo (nil = anterior al spec) y files como changed_files, a la hora min.
func retyVeredicto(t *testing.T, root, slug string, ev *riesgo.Evaluacion, min int, partial bool, files ...string) {
	t.Helper()
	v := &verdict.Verdict{Schema: verdict.SchemaID, CreatedAt: time.Date(2026, 11, 20, 9, min, 0, 0, time.UTC),
		Project: "demo", Spec: ".hoom/specs/" + slug + ".md",
		Git:    gitx.Info{IsRepo: true, ChangeFingerprint: "huella", Insertions: 50, Deletions: 10, ChangedFiles: files},
		Gates:  []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass, Scope: "full"}},
		Riesgo: ev, Partial: partial}
	if ev != nil {
		v.Tamano = &riesgo.Tamano{Producto: 60}
	}
	v.Finalize()
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatalf("fixture: verdict.Write: %v", err)
	}
}

func retyNuevo(decl, ef, mot string) *verdict.Verdict {
	return &verdict.Verdict{ID: "v1", Spec: ".hoom/specs/t.md",
		Git:    gitx.Info{IsRepo: true, ChangedFiles: []string{"internal/auth/x.go"}, Insertions: 9300},
		Riesgo: &riesgo.Evaluacion{Declarado: decl, Efectivo: ef, Motivo: mot}, Tamano: &riesgo.Tamano{Producto: 10}}
}

func retyViejo(files ...string) *verdict.Verdict {
	return &verdict.Verdict{ID: "v0", Spec: ".hoom/specs/t.md", Git: gitx.Info{IsRepo: true, ChangedFiles: files, Insertions: 50}}
}

// CA-786: RiesgoDeTarjeta en cada caso del spec y de la aclaracion 15.
func TestCA786_RiesgoDeTarjeta(t *testing.T) {
	base := riesgo.Clasificador{Rutas: riesgo.RutasBase(), Pruebas: []string{".hoom/**", "**/*_test.go"}}
	propio := riesgo.Clasificador{Rutas: riesgo.Rutas{DeRiesgo: []string{"cobros"}, DeBajoRiesgo: riesgo.RutasBase().DeBajoRiesgo}}
	generado := riesgo.Clasificador{Rutas: riesgo.RutasBase()}
	generado.Rutas.Generadas = []string{"vendor"}
	tj := func(r string) riesgo.Tarjeta { return riesgo.Tarjeta{Tarea: "t", Tipo: "feature", Riesgo: r} }
	ev := func(d, e, m string) riesgo.Evaluacion { return riesgo.Evaluacion{Declarado: d, Efectivo: e, Motivo: m} }
	for _, k := range []struct {
		nombre string
		v      *verdict.Verdict
		t      riesgo.Tarjeta
		c      riesgo.Clasificador
		want   riesgo.Evaluacion
	}{
		{"sin veredicto, alto", nil, tj("alto"), base, ev("alto", "alto", "declarado alto")},
		{"sin veredicto, sin riesgo", nil, tj(""), base, ev("normal", "normal", "declarado normal")},
		{"sin veredicto, bajo", nil, tj("bajo"), base, ev("bajo", "bajo", "declarado bajo")},
		{"sin veredicto, seguridad al pie de la letra", nil, riesgo.Tarjeta{Tarea: "t", Tipo: "seguridad"}, base, ev("normal", "normal", "declarado normal")},
		{"nuevo, sube el veredicto", retyNuevo("normal", "alto", retyMotAuth), tj(""), base, ev("normal", "alto", retyMotAuth)},
		{"nuevo, empate", retyNuevo("normal", "alto", retyMotAuth), tj("alto"), base, ev("alto", "alto", retyMotAuth)},
		{"nuevo, gana el declarado", retyNuevo("normal", "normal", "declarado normal"), tj("alto"), base, ev("alto", "alto", "declarado alto")},
		{"nuevo, bajar no se ve", retyNuevo("normal", "alto", retyMotAuth), tj("bajo"), base, ev("bajo", "alto", retyMotAuth)},
		{"nuevo, bajo subido a normal", retyNuevo("bajo", "normal", retyMotApp), tj("bajo"), base, ev("bajo", "normal", retyMotApp)},
		{"nuevo, normal sobre bajo", retyNuevo("bajo", "bajo", "declarado bajo"), tj("normal"), base, ev("normal", "normal", "declarado normal")},
		{"nuevo, sin riesgo sobre bajo", retyNuevo("bajo", "bajo", "declarado bajo"), tj(""), base, ev("normal", "normal", "declarado normal")},
		{"nuevo, seguridad", retyNuevo("normal", "alto", riesgo.MotivoSeguridad), riesgo.Tarjeta{Tarea: "t", Tipo: "seguridad"}, base, ev("normal", "alto", riesgo.MotivoSeguridad)},
		{"nuevo, el clasificador no cuenta", retyNuevo("normal", "normal", "declarado normal"), tj(""), propio, ev("normal", "normal", "declarado normal")},
		{"viejo, bajo con a.go", retyViejo("a.go"), tj("bajo"), base, ev("bajo", "normal", retyMotApp)},
		{"viejo, auth", retyViejo("internal/auth/x.go"), tj(""), base, ev("normal", "alto", retyMotAuth)},
		{"viejo, alto declarado", retyViejo("a.go"), tj("alto"), base, ev("alto", "alto", "declarado alto")},
		{"viejo, bajo con pruebas", retyViejo("internal/precio_test.go"), tj("bajo"), base, ev("bajo", "bajo", "declarado bajo")},
		{"viejo, bajo con .hoom", retyViejo(".hoom/verdicts/x.json", ".hoom/specs/auth.md"), tj("bajo"), base, ev("bajo", "bajo", "declarado bajo")},
		{"viejo, sin archivos", retyViejo(), tj("normal"), base, ev("normal", "normal", "declarado normal")},
		{"viejo, ruta propia", retyViejo("cobros/f.go"), tj(""), propio, ev("normal", "alto", "el cambio toca cobros/f.go: riesgo alto")},
		{"viejo, auth sin la marca en la ruta propia", retyViejo("internal/auth/x.go"), tj(""), propio, ev("normal", "normal", "declarado normal")},
		{"viejo, clasificador cero", retyViejo("internal/auth/x.go"), tj(""), riesgo.Clasificador{}, ev("normal", "normal", "declarado normal")},
		{"viejo, generado", retyViejo("vendor/auth/x.go"), tj(""), generado, ev("normal", "normal", "declarado normal")},
		{"viejo, seguridad", retyViejo("internal/auth/x.go"), riesgo.Tarjeta{Tarea: "t", Tipo: "seguridad"}, base, ev("normal", "alto", riesgo.MotivoSeguridad)},
		{"viejo, ilegible", retyViejo("a.go"), riesgo.Tarjeta{Tarea: "t", Ilegible: true}, base, ev("normal", "alto", riesgo.MotivoIlegible)},
		{"viejo, la primera ruta en orden de bytes", retyViejo("a/token.go", "a.go", "B/billing.go"), tj("bajo"), base, ev("bajo", "alto", "el cambio toca B/billing.go: riesgo alto")},
	} {
		if got := finding.RiesgoDeTarjeta(k.v, k.t, k.c); got != k.want {
			t.Fatalf("CA-786 (%s): RiesgoDeTarjeta es %+v, fue %+v", k.nombre, k.want, got)
		}
	}
}

var retyOrden = map[string]int{"bajo": 0, "normal": 1, "alto": 2}

// CA-786 (propiedad, exhaustiva): el resultado nunca es mas bajo que el
// efectivo del veredicto (o lo que miden las reglas en uno viejo) ni que el
// declarado de ahora, es uno de los dos, y el motivo es el del veredicto
// salvo que el declarado sea estrictamente mas alto.
func TestCA786_PropiedadRiesgoDeTarjeta(t *testing.T) {
	c := riesgo.Clasificador{Rutas: riesgo.RutasBase(), Pruebas: []string{"**/*_test.go"}}
	conjuntos := [][]string{nil, {"a.go"}, {"internal/auth/x.go"}, {"docs/a.md"}, {"internal/precio_test.go"}, {"a.go", "x/billing_test.go"}}
	var veredictos []*verdict.Verdict
	for _, ef := range []string{"bajo", "normal", "alto"} {
		veredictos = append(veredictos, retyNuevo("normal", ef, "motivo del veredicto "+ef))
	}
	for _, fs := range conjuntos {
		veredictos = append(veredictos, retyViejo(fs...))
	}
	for i, v := range veredictos {
		for _, r := range []string{"", "bajo", "normal", "alto"} {
			for _, tipo := range []string{"feature", "seguridad"} {
				tj := riesgo.Tarjeta{Tarea: "t", Tipo: tipo, Riesgo: r}
				got := finding.RiesgoDeTarjeta(v, tj, c)
				decl := r
				if decl == "" {
					decl = "normal"
				}
				medido := c.Evaluar(tj, v.Git.ChangedFiles)
				if v.Riesgo != nil {
					medido = *v.Riesgo
				}
				ca := fmt.Sprintf("CA-786 (veredicto %d, declarado %q, tipo %s)", i, r, tipo)
				if got.Declarado != decl {
					t.Fatalf("%s: Declarado es el de ahora, %q: %+v", ca, decl, got)
				}
				if retyOrden[got.Efectivo] < retyOrden[medido.Efectivo] || retyOrden[got.Efectivo] < retyOrden[decl] {
					t.Fatalf("%s: nunca mas bajo que %s ni que %s: %+v", ca, medido.Efectivo, decl, got)
				}
				if got.Efectivo != medido.Efectivo && got.Efectivo != decl {
					t.Fatalf("%s: es uno de los dos: %+v", ca, got)
				}
				quiero := medido.Motivo
				if retyOrden[decl] > retyOrden[medido.Efectivo] {
					quiero = "declarado " + decl
				}
				if got.Motivo != quiero || got.Motivo == "" {
					t.Fatalf("%s: el motivo es %q: %+v", ca, quiero, got)
				}
			}
		}
	}
	// Sin veredicto, siempre el declarado, aunque sea de tipo seguridad.
	for _, r := range []string{"", "bajo", "normal", "alto"} {
		for _, tipo := range []string{"feature", "seguridad"} {
			got := finding.RiesgoDeTarjeta(nil, riesgo.Tarjeta{Tarea: "t", Tipo: tipo, Riesgo: r}, c)
			d := r
			if d == "" {
				d = "normal"
			}
			if got != (riesgo.Evaluacion{Declarado: d, Efectivo: d, Motivo: "declarado " + d}) {
				t.Fatalf("CA-786: sin veredicto vale el declarado %q (tipo %s): %+v", d, tipo, got)
			}
		}
	}
}

func retyF(id, sev, file, task string) finding.Finding {
	return finding.Finding{ID: id, Severity: sev, Lens: "risk", File: file, Description: "d", Author: "codex", Task: task}
}

func retyBloquea(t *testing.T, ca string, f finding.Finding, c finding.Contexto, bloquea bool, motivo string) {
	t.Helper()
	b, m := finding.Bloquea(f, c)
	if b != bloquea || m != motivo {
		t.Fatalf("%s: Bloquea(%s %q) es (%v, %q), fue (%v, %q)", ca, f.Severity, f.File, bloquea, motivo, b, m)
	}
}

// CA-787: el caso 3 de Bloquea mide contra Contexto.Rutas.DeRiesgo (nil =
// RutasBase), sin mayusculas y con el directorio padre, al pie de la letra
// tambien para .hoom/ y generados (aclaracion 20); `sign` ya no es marca.
func TestCA787_BloqueaMideLaRutaContraRutasDeRiesgo(t *testing.T) {
	const ca = "CA-787"
	base := riesgo.RutasBase()
	sinRutas := finding.Contexto{BlockOn: "high"}
	for _, k := range []struct {
		file    string
		bloquea bool
	}{
		{"internal/auth/x.go", true},
		{"Internal/AUTH/X.go", true},
		{"internal/signin/x.go", true},
		{"x/signature.go", true},
		{"internal/spec/tokens_test.go", true},
		{".hoom/specs/auth.md", true},
		{"vendor/auth/x.go", true},
		{"app/DESIGN.kt", false},
		{"internal/sign/x.go", false},
		{"cobros/f.go", false},
		{"app.go", false},
	} {
		m := ""
		if k.bloquea {
			m = "medium en ruta de riesgo: " + k.file
		}
		retyBloquea(t, ca+" (sin rutas)", retyF("m", "medium", k.file, "t"), sinRutas, k.bloquea, m)
		retyBloquea(t, ca+" (RutasBase)", retyF("m", "medium", k.file, "t"), finding.Contexto{BlockOn: "high", Rutas: &base}, k.bloquea, m)
		// un low nunca por la ruta
		retyBloquea(t, ca+" (low)", retyF("l", "low", k.file, "t"), sinRutas, false, "")
	}
	propias := riesgo.Rutas{DeRiesgo: []string{"cobros", "app/Domain/**/Costeo*"}}
	conPropias := finding.Contexto{BlockOn: "high", Rutas: &propias}
	for _, k := range []struct {
		file    string
		bloquea bool
	}{
		{"cobros/f.go", true},
		{"COBROS/f.go", true},
		{"app/Domain/Ventas/CosteoLote.php", true},
		{"app/Domain/CosteoLote.php", true},
		{"internal/auth/x.go", false},
		{"x/cobros.go", false},
	} {
		m := ""
		if k.bloquea {
			m = "medium en ruta de riesgo: " + k.file
		}
		retyBloquea(t, ca+" (propias)", retyF("m", "medium", k.file, "t"), conPropias, k.bloquea, m)
	}
	vacias := riesgo.Rutas{DeRiesgo: []string{}}
	retyBloquea(t, ca+" (de_riesgo vacia)", retyF("m", "medium", "internal/auth/x.go", "t"), finding.Contexto{BlockOn: "high", Rutas: &vacias}, false, "")
	// un patron invalido no coincide con nada
	malas := riesgo.Rutas{DeRiesgo: []string{"[auth", ""}}
	retyBloquea(t, ca+" (patron invalido)", retyF("m", "medium", "[auth/x.go", "t"), finding.Contexto{BlockOn: "high", Rutas: &malas}, false, "")
	// sin file nunca por la ruta
	retyBloquea(t, ca+" (sin file)", retyF("m", "medium", "", "t"), sinRutas, false, "")
	// el high sigue siendo el caso 1
	retyBloquea(t, ca+" (high)", retyF("h", "high", "internal/auth/x.go", "t"), sinRutas, true, "severidad high")
}

// CA-787: el caso 4 bloquea el medium de una completa de su tarea con el
// riesgo efectivo alto de Contexto.Riesgos; el caso 3 va antes.
func TestCA787_BloqueaElMediumDeLaCompletaConElEfectivo(t *testing.T) {
	const ca = "CA-787"
	completas := []finding.RevisionCompleta{
		{ID: "c0", Task: "u", Findings: []string{"m1"}},
		{ID: "c1", Task: "t", Findings: []string{"m1", "m2"}},
		{ID: "c2", Task: "t", Findings: []string{"m1"}},
	}
	con := func(r string) finding.Contexto {
		return finding.Contexto{BlockOn: "high", Riesgos: map[string]string{"t": r, "u": "normal"}, Completas: completas}
	}
	retyBloquea(t, ca+" (alto)", retyF("m1", "medium", "app.go", "t"), con("alto"), true, "medium de la review completa c1: riesgo alto")
	retyBloquea(t, ca+" (alto, otra completa)", retyF("m2", "medium", "app.go", "t"), con("alto"), true, "medium de la review completa c1: riesgo alto")
	for _, r := range []string{"normal", "bajo", ""} {
		retyBloquea(t, ca+" ("+r+")", retyF("m1", "medium", "app.go", "t"), con(r), false, "")
	}
	// fuera de una completa de su tarea, no
	retyBloquea(t, ca+" (no esta)", retyF("m3", "medium", "app.go", "t"), con("alto"), false, "")
	retyBloquea(t, ca+" (de otra tarea)", retyF("m1", "medium", "app.go", "u"), finding.Contexto{BlockOn: "high",
		Riesgos: map[string]string{"t": "alto", "u": "normal"}, Completas: completas}, false, "")
	// una tarea que no esta en Riesgos es normal
	retyBloquea(t, ca+" (sin riesgo)", retyF("m1", "medium", "app.go", "t"), finding.Contexto{BlockOn: "high", Completas: completas}, false, "")
	// el caso 3 va antes que el 4
	retyBloquea(t, ca+" (ruta y completa)", retyF("m1", "medium", "internal/auth/x.go", "t"), con("alto"), true, "medium en ruta de riesgo: internal/auth/x.go")
	// un low de la completa no
	retyBloquea(t, ca+" (low)", retyF("m1", "low", "app.go", "t"), con("alto"), false, "")
}

// CA-786/CA-787 y aclaracion 22: ContextoDe trae, para cada tarea, el riesgo
// de su tarjeta (RiesgoDeTarjeta del ultimo veredicto completo y el item),
// y las rutas del hoom.yaml del proyecto.
func TestCA787_ContextoDeConElRiesgoDeLaTarjeta(t *testing.T) {
	const ca = "CA-787"
	root := retyRepo(t, "u", "w", "x", "y", "z")
	for slug, r := range map[string]string{"u": "normal", "w": "normal", "x": "alto", "y": "", "z": "bajo"} {
		retyItem(t, root, slug, r)
	}
	// u: un veredicto viejo alto y uno nuevo normal: vale el ultimo
	retyVeredicto(t, root, "u", &riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: retyMotAuth}, 1, false, "internal/auth/x.go")
	retyVeredicto(t, root, "u", &riesgo.Evaluacion{Declarado: "normal", Efectivo: "normal", Motivo: "declarado normal"}, 2, false, "a.go")
	// w: un veredicto anterior al spec que toca auth
	retyVeredicto(t, root, "w", nil, 3, false, "internal/auth/y.go")
	// y: un veredicto normal, y uno parcial despues que no cuenta
	retyVeredicto(t, root, "y", &riesgo.Evaluacion{Declarado: "normal", Efectivo: "normal", Motivo: "declarado normal"}, 4, false, "a.go")
	retyVeredicto(t, root, "y", &riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: retyMotAuth}, 5, true, "internal/auth/x.go")
	// z: bajo que el veredicto subio a alto
	retyVeredicto(t, root, "z", &riesgo.Evaluacion{Declarado: "bajo", Efectivo: "alto", Motivo: retyMotAuth}, 6, false, "internal/auth/x.go")
	c, err := finding.ContextoDe(root, root, "high")
	if err != nil {
		t.Fatalf("%s: ContextoDe: %v", ca, err)
	}
	if want := map[string]string{"u": "normal", "w": "alto", "x": "alto", "y": "normal", "z": "alto"}; !reflect.DeepEqual(c.Riesgos, want) {
		t.Fatalf("%s: Riesgos es el de cada tarjeta %v, fue %v", ca, want, c.Riesgos)
	}
	if c.Rutas != nil && !reflect.DeepEqual(c.Rutas.DeRiesgo, riesgo.RutasBase().DeRiesgo) {
		t.Fatalf("%s: sin hoom.yaml las rutas son las base: %+v", ca, c.Rutas)
	}
	// con rutas propias: la de riesgo cambia, y el viejo de w ya no sube
	retyEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+
		"rutas:\n  de_riesgo: [cobros]\n")
	c, err = finding.ContextoDe(root, root, "high")
	if err != nil {
		t.Fatalf("%s: ContextoDe con rutas: %v", ca, err)
	}
	if c.Rutas == nil || !reflect.DeepEqual(c.Rutas.DeRiesgo, []string{"cobros"}) {
		t.Fatalf("%s: las rutas son las del hoom.yaml: %+v", ca, c.Rutas)
	}
	if c.Riesgos["w"] != "normal" || c.Riesgos["z"] != "alto" || c.Riesgos["x"] != "alto" {
		t.Fatalf("%s: el viejo se mide con el clasificador del proyecto; los nuevos traen el suyo: %v", ca, c.Riesgos)
	}
	// unas rutas que no cargan son un error
	retyEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+
		"rutas:\n  de_riesgo: cobros\n")
	if _, err := finding.ContextoDe(root, root, "high"); err == nil {
		t.Fatalf("%s: un hoom.yaml cuyas rutas no cargan es un error de ContextoDe", ca)
	}
}

// CA-787 y aclaracion 22: el gate de los otros comandos (Gate) juzga el
// medium de la completa con el riesgo de la tarjeta: el efectivo de su
// ultimo veredicto, aunque el item declare normal.
func TestCA787_GateConElRiesgoDeLaTarjeta(t *testing.T) {
	const ca = "CA-787"
	root := retyRepo(t, "u")
	retyItem(t, root, "u", "normal")
	m := retyAlta(t, root, "medium", "app.go", "u")
	rc := retyCompleta(t, root, "u", []string{m.ID})
	if g := finding.Gate(root, "main", "high", ".hoom/specs/u.md"); g.Status != verdict.StatusPass {
		t.Fatalf("%s: sin veredicto, normal no bloquea: %s %q\n%s", ca, g.Status, g.Notes, g.OutputTail)
	}
	retyVeredicto(t, root, "u", &riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: retyMotAuth}, 1, false, "internal/auth/x.go")
	g := finding.Gate(root, "main", "high", ".hoom/specs/u.md")
	if g.Status != verdict.StatusFail || !strings.Contains(g.Notes, m.ID) ||
		!strings.Contains(g.OutputTail, "medium de la review completa "+rc.ID+": riesgo alto") {
		t.Fatalf("%s: con el efectivo alto del veredicto, el medium de la completa bloquea: %s %q\n%s", ca, g.Status, g.Notes, g.OutputTail)
	}
}

// CA-787: GateDelVerify juzga la tarea del verify con el efectivo que ese
// verify calculo, en lugar del de su tarjeta, y el caso 3 con las rutas de
// su clasificador; las demas tareas, con el de su tarjeta.
func TestCA787_GateDelVerifyConLoQueMidio(t *testing.T) {
	const ca = "CA-787"
	base := riesgo.Clasificador{Rutas: riesgo.RutasBase()}
	root := retyRepo(t, "t", "u")
	retyItem(t, root, "t", "normal")
	retyItem(t, root, "u", "alto")
	m := retyAlta(t, root, "medium", "app.go", "t")
	rc := retyCompleta(t, root, "t", []string{m.ID})
	spec := ".hoom/specs/t.md"
	g := finding.GateDelVerify(root, "main", "high", spec, finding.DelVerify{Clasificador: base, Tarea: "t", Efectivo: "alto"})
	if g.Status != verdict.StatusFail || !strings.Contains(g.Notes, m.ID) ||
		!strings.Contains(g.OutputTail, "medium de la review completa "+rc.ID+": riesgo alto") {
		t.Fatalf("%s: con el efectivo alto del verify, el medium de la completa bloquea aunque la tarjeta declare normal: %s %q\n%s", ca, g.Status, g.Notes, g.OutputTail)
	}
	for _, d := range []finding.DelVerify{
		{Clasificador: base, Tarea: "t", Efectivo: "normal"},
		{Clasificador: base, Tarea: "otra", Efectivo: "alto"},
		{Clasificador: base},
	} {
		if g := finding.GateDelVerify(root, "main", "high", spec, d); g.Status != verdict.StatusPass {
			t.Fatalf("%s: %+v: la tarjeta t declara normal y no hay veredicto: pasa: %s %q\n%s", ca, d, g.Status, g.Notes, g.OutputTail)
		}
	}
	if g := finding.Gate(root, "main", "high", spec); g.Status != verdict.StatusPass {
		t.Fatalf("%s: Gate, con la tarjeta normal, pasa: %s %q", ca, g.Status, g.Notes)
	}
	// un veredicto viejo de t que la subio a alto: el verify de ahora manda
	retyVeredicto(t, root, "t", &riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: retyMotAuth}, 1, false, "internal/auth/x.go")
	if g := finding.GateDelVerify(root, "main", "high", spec, finding.DelVerify{Clasificador: base, Tarea: "t", Efectivo: "normal"}); g.Status != verdict.StatusPass {
		t.Fatalf("%s: para su tarea vale el efectivo de este verify, no el de la tarjeta: %s %q\n%s", ca, g.Status, g.Notes, g.OutputTail)
	}
	// la otra tarea, de alto declarado, con su tarjeta: su medium de la completa bloquea en el gate entero
	mu := retyAlta(t, root, "medium", "app.go", "u")
	ru := retyCompleta(t, root, "u", []string{mu.ID})
	g = finding.GateDelVerify(root, "main", "high", "", finding.DelVerify{Clasificador: base, Tarea: "t", Efectivo: "normal"})
	if g.Status != verdict.StatusFail || !strings.Contains(g.Notes, mu.ID) || strings.Contains(g.Notes, m.ID) ||
		!strings.Contains(g.OutputTail, "medium de la review completa "+ru.ID+": riesgo alto") {
		t.Fatalf("%s: la otra tarea se juzga con su tarjeta (alto): %s %q\n%s", ca, g.Status, g.Notes, g.OutputTail)
	}
}

// CA-787: el caso 3 de GateDelVerify usa las rutas del clasificador del
// verify, no RutasBase ni el hoom.yaml del arbol.
func TestCA787_GateDelVerifyConLasRutasDelVerify(t *testing.T) {
	const ca = "CA-787"
	root := retyRepo(t, "t")
	retyItem(t, root, "t", "normal")
	mc := retyAlta(t, root, "medium", "cobros/f.go", "t")
	ma := retyAlta(t, root, "medium", "internal/auth/x.go", "t")
	spec := ".hoom/specs/t.md"
	propio := riesgo.Clasificador{Rutas: riesgo.Rutas{DeRiesgo: []string{"cobros"}}}
	g := finding.GateDelVerify(root, "main", "high", spec, finding.DelVerify{Clasificador: propio, Tarea: "t", Efectivo: "normal"})
	if g.Status != verdict.StatusFail || !strings.Contains(g.Notes, mc.ID) || strings.Contains(g.Notes, ma.ID) ||
		!strings.Contains(g.OutputTail, "medium en ruta de riesgo: cobros/f.go") {
		t.Fatalf("%s: con de_riesgo [cobros], bloquea solo el de cobros: %s %q\n%s", ca, g.Status, g.Notes, g.OutputTail)
	}
	g = finding.GateDelVerify(root, "main", "high", spec, finding.DelVerify{Clasificador: riesgo.Clasificador{Rutas: riesgo.RutasBase()}, Tarea: "t", Efectivo: "normal"})
	if g.Status != verdict.StatusFail || !strings.Contains(g.Notes, ma.ID) || strings.Contains(g.Notes, mc.ID) ||
		!strings.Contains(g.OutputTail, "medium en ruta de riesgo: internal/auth/x.go") {
		t.Fatalf("%s: con las rutas base, bloquea solo el de auth: %s %q\n%s", ca, g.Status, g.Notes, g.OutputTail)
	}
	vacio := riesgo.Clasificador{Rutas: riesgo.Rutas{DeRiesgo: []string{}}}
	if g := finding.GateDelVerify(root, "main", "high", spec, finding.DelVerify{Clasificador: vacio, Tarea: "t", Efectivo: "normal"}); g.Status != verdict.StatusPass {
		t.Fatalf("%s: con de_riesgo vacia, ningun medium bloquea por la ruta: %s %q\n%s", ca, g.Status, g.Notes, g.OutputTail)
	}
}
