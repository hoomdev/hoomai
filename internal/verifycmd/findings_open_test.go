// Tests adversariales del spec .hoom/specs/gate-findings-open.md
// (CA-248, CA-250..CA-257): verify integra el gate sintetico findings_open
// solo cuando hoom.yaml opta por el, fuera de las corridas --gate, en su
// lugar del veredicto y narrado como cualquier gate.
package verifycmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/ratchet"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const foGatesBaratos = "  test:\n    required: true\n    cmd: \"true\"\n" +
	"  build:\n    required: true\n    cmd: \"true\"\n"

// foProyecto arma un repo git con hoom.yaml (gates baratos + el bloque
// findings que se pase) y lo carga como lo hace el binario.
func foProyecto(t *testing.T, gatesYAML, findingsYAML string) *manifest.Manifest {
	t.Helper()
	dir := t.TempDir()
	body := "schema: hoom/v1\nproject: demo\ngates:\n" + gatesYAML + findingsYAML
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@hoom.dev"},
		{"config", "user.name", "hoom test"},
		{"add", "-A"},
		{"commit", "-m", "inicial"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatalf("fixture: hoom.yaml valido rechazado: %v\n%s", err, body)
	}
	return m
}

func foConUmbral(t *testing.T, umbral string) *manifest.Manifest {
	t.Helper()
	return foProyecto(t, foGatesBaratos, "findings:\n  block_on: "+umbral+"\n")
}

func foAlta(t *testing.T, m *manifest.Manifest, sev, lens, task string) finding.Finding {
	t.Helper()
	f, err := finding.Register(m.Dir, m.BaseBranch, finding.Draft{Severity: sev, Lens: lens, File: "hoom.yaml",
		Description: "hallazgo " + sev + " de prueba", Author: "reviewer@test", Task: task})
	if err != nil {
		t.Fatalf("fixture: Register: %v", err)
	}
	return f
}

// foSpecAprobado escribe .hoom/specs/<slug>.md con las siete secciones y un
// criterio verificado por comando, y lo aprueba. Devuelve la ruta absoluta.
func foSpecAprobado(t *testing.T, dir, slug string) string {
	t.Helper()
	p := filepath.Join(dir, ".hoom", "specs")
	if err := os.MkdirAll(p, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(p, slug+".md")
	body := "# Spec\n## Objetivo\nx\n## No-goals\nx\n## Contratos\nx\n## Casos limite\nx\n" +
		"## Criterios de aceptacion\n- CA-1: x. [verifica: true]\n## Decisiones\nx\n## Riesgos\nx\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := approval.Approve(dir, path); err != nil {
		t.Fatal(err)
	}
	return path
}

func foIndice(v *verdict.Verdict, name string) int {
	for i, g := range v.Gates {
		if g.Name == name {
			return i
		}
	}
	return -1
}

func foEventosDe(evs []live.Event, gate string) (starts, ends []live.Event) {
	for _, ev := range evs {
		if ev.Gate != gate {
			continue
		}
		switch ev.Kind {
		case live.KindGateStart:
			starts = append(starts, ev)
		case live.KindGateEnd:
			ends = append(ends, ev)
		}
	}
	return starts, ends
}

// CA-248: sin findings.block_on (bloque ausente, findings: {} o block_on
// null) verify no emite findings_open ni en el veredicto ni en los eventos,
// y el total de verify_start no lo cuenta. Aunque haya un high abierto.
func TestCA248_SinBlockOnNoHayFindingsOpen(t *testing.T) {
	for nombre, yml := range map[string]string{
		"ausente":        "",
		"findings: {}":   "findings: {}\n",
		"block_on null":  "findings:\n  block_on:\n",
		"block_on: null": "findings:\n  block_on: null\n",
	} {
		m := foProyecto(t, foGatesBaratos, yml)
		foAlta(t, m, "high", "risk", "")
		spec := foSpecAprobado(t, m.Dir, "tarea-a")
		for _, opt := range []Options{{}, {Full: true}, {Spec: spec}} {
			v, _, err := Run(m, opt)
			if err != nil {
				t.Fatal(err)
			}
			if gateNamed(v, "findings_open") != nil {
				t.Fatalf("CA-248 (%s, %+v): sin block_on no existe findings_open: %+v", nombre, opt, v.Gates)
			}
			if v.Verdict != "green" {
				t.Fatalf("CA-248 (%s, %+v): el high abierto no toca el veredicto con el gate apagado: %+v", nombre, opt, v.Summary)
			}
			evs := readLiveEvents(t, m.Dir)
			quiero := len(m.Gates)
			if opt.Spec != "" {
				quiero += 3
			}
			if evs[0].Kind != live.KindVerifyStart || evs[0].Gates != quiero {
				t.Fatalf("CA-248 (%s, %+v): verify_start declara %d gates, no %d: %+v", nombre, opt, quiero, evs[0].Gates, evs[0])
			}
			if s, e := foEventosDe(evs, "findings_open"); len(s)+len(e) != 0 {
				t.Fatalf("CA-248 (%s, %+v): no hay eventos de findings_open: %+v %+v", nombre, opt, s, e)
			}
		}
	}

	// Control: el mismo arreglo con block_on high SI emite el gate; sin esto
	// los asertos de arriba pasarian con un gate que nunca corre.
	m := foConUmbral(t, "high")
	foAlta(t, m, "high", "risk", "")
	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gateNamed(v, "findings_open") == nil || v.Verdict != "red" {
		t.Fatalf("CA-248 (control): con block_on high el gate corre y el high pone ROJO: %v", foNombres(v))
	}
}

// CA-250: con block_on high y un high abierto, findings_open es FAIL
// required y el veredicto es ROJO, en diff y en --full; el artefacto lo trae.
func TestCA250_VerifyRojoConHighAbierto(t *testing.T) {
	m := foConUmbral(t, "high")
	f := foAlta(t, m, "high", "risk", "")
	for _, opt := range []Options{{}, {Full: true}} {
		v, path, err := Run(m, opt)
		if err != nil {
			t.Fatal(err)
		}
		g := gateNamed(v, "findings_open")
		if g == nil {
			t.Fatalf("CA-250 (%+v): con block_on el gate findings_open corre: %+v", opt, v.Gates)
		}
		if g.Status != verdict.StatusFail || !g.Required {
			t.Fatalf("CA-250 (%+v): esperaba FAIL required: %+v", opt, g)
		}
		if v.Verdict != "red" {
			t.Fatalf("CA-250 (%+v): el veredicto debe ser ROJO: %+v", opt, v.Summary)
		}
		if !strings.Contains(g.Notes, f.ID) || !strings.Contains(g.Notes, "[risk]") {
			t.Fatalf("CA-250 (%+v): las notas traen el id y la lente: %q", opt, g.Notes)
		}
		if !strings.Contains(g.OutputTail, "hoom finding resolve "+f.ID) {
			t.Fatalf("CA-250 (%+v): output_tail trae la accion exacta: %q", opt, g.OutputTail)
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), `"findings_open"`) || !strings.Contains(string(raw), f.ID) {
			t.Fatalf("CA-250 (%+v): el artefacto del veredicto lleva el gate y el id", opt)
		}
	}
}

// CA-251: con block_on high, un medium y un low abiertos: PASS, notas que
// empiezan con '0 abiertos' y los informan como que no bloquean. VERDE.
func TestCA251_VerifyVerdeConMenores(t *testing.T) {
	m := foConUmbral(t, "high")
	foAlta(t, m, "medium", "risk", "")
	foAlta(t, m, "low", "readability", "")
	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	g := gateNamed(v, "findings_open")
	if g == nil || g.Status != verdict.StatusPass {
		t.Fatalf("CA-251: esperaba findings_open PASS: %+v", g)
	}
	if !strings.HasPrefix(g.Notes, "0 abiertos") {
		t.Fatalf("CA-251: las notas empiezan con '0 abiertos': %q", g.Notes)
	}
	for _, s := range []string{"no bloquean", "1 medium", "1 low"} {
		if !strings.Contains(g.Notes, s) {
			t.Fatalf("CA-251: las notas mencionan %q: %q", s, g.Notes)
		}
	}
	if v.Verdict != "green" {
		t.Fatalf("CA-251: el veredicto es VERDE: %+v", v.Summary)
	}
}

// CA-252: el umbral del manifiesto incluye las severidades mayores, y se
// lee normalizado (block_on: HIGH = high).
func TestCA252_UmbralDesdeElManifiesto(t *testing.T) {
	casos := []struct {
		umbral  string
		sevs    []string
		veredic string
	}{
		{"medium", []string{"medium"}, "red"},
		{"medium", []string{"high"}, "red"},
		{"medium", []string{"low"}, "green"},
		{"low", []string{"low"}, "red"},
		{"HIGH", []string{"high"}, "red"},
		{"HIGH", []string{"medium"}, "green"},
	}
	for _, c := range casos {
		m := foConUmbral(t, c.umbral)
		for _, s := range c.sevs {
			foAlta(t, m, s, "risk", "")
		}
		v, _, err := Run(m, Options{})
		if err != nil {
			t.Fatal(err)
		}
		g := gateNamed(v, "findings_open")
		if g == nil {
			t.Fatalf("CA-252: block_on %s activa el gate: %+v", c.umbral, v.Gates)
		}
		if v.Verdict != c.veredic {
			t.Fatalf("CA-252: block_on %s con %v debia ser %s, fue %s (%s)", c.umbral, c.sevs, c.veredic, v.Verdict, g.Notes)
		}
		if !strings.Contains(g.Notes, "block_on: "+strings.ToLower(c.umbral)) {
			t.Fatalf("CA-252: las notas muestran el umbral normalizado: %q", g.Notes)
		}
	}
}

// CA-253: una resolucion valida cierra (VERDE); una con evidencia en blanco
// no cierra (ROJO) y el aviso que nombra el archivo sale en output_tail.
func TestCA253_VerifyVeLaDefinicionUnica(t *testing.T) {
	m := foConUmbral(t, "high")
	f := foAlta(t, m, "high", "risk", "")
	if _, err := finding.Resolve(m.Dir, f.ID, "corregido", "commit abc123 con el test que lo cubre", ""); err != nil {
		t.Fatal(err)
	}
	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if g := gateNamed(v, "findings_open"); g == nil || g.Status != verdict.StatusPass || v.Verdict != "green" {
		t.Fatalf("CA-253: un high corregido con evidencia no cuenta: %+v", g)
	}

	m = foConUmbral(t, "high")
	f = foAlta(t, m, "high", "risk", "")
	archivo := f.ID + ".res.json"
	res := `{"finding_id": "` + f.ID + `", "as": "refutado", "evidence": "   "}`
	if err := os.WriteFile(filepath.Join(m.Dir, ".hoom", "findings", archivo), []byte(res), 0o644); err != nil {
		t.Fatal(err)
	}
	v, _, err = Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	g := gateNamed(v, "findings_open")
	if g == nil || g.Status != verdict.StatusFail || v.Verdict != "red" {
		t.Fatalf("CA-253: evidencia en blanco no cierra: el high cuenta y el veredicto es ROJO: %+v", g)
	}
	if !strings.Contains(g.OutputTail, archivo) || !strings.Contains(g.OutputTail, "resolucion invalida") {
		t.Fatalf("CA-253: el aviso de la resolucion invalida va en output_tail: %q", g.OutputTail)
	}
}

// CA-254: un hallazgo ilegible o con severidad desconocida es ERROR nombrando
// el archivo, y el veredicto queda ROJO (fail-closed).
func TestCA254_VerifyHallazgoRotoEsRojo(t *testing.T) {
	for nombre, contenido := range map[string]string{
		"roto.json":     "{roto",
		"critical.json": `{"id": "critical", "severity": "critical", "lens": "risk", "description": "a mano"}`,
	} {
		m := foConUmbral(t, "high")
		d := filepath.Join(m.Dir, ".hoom", "findings")
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(d, nombre), []byte(contenido), 0o644); err != nil {
			t.Fatal(err)
		}
		v, _, err := Run(m, Options{})
		if err != nil {
			t.Fatal(err)
		}
		g := gateNamed(v, "findings_open")
		if g == nil || g.Status != verdict.StatusError {
			t.Fatalf("CA-254 (%s): esperaba findings_open ERROR: %+v", nombre, g)
		}
		if !strings.Contains(g.Notes, nombre) {
			t.Fatalf("CA-254 (%s): las notas nombran el archivo: %q", nombre, g.Notes)
		}
		if v.Verdict != "red" {
			t.Fatalf("CA-254 (%s): ERROR en un gate required es ROJO: %+v", nombre, v.Summary)
		}
	}
}

// CA-255: con --spec cuentan los de la tarea del spec y los sin tarea; un high
// de otra tarea no bloquea, la nota dice cuantos quedaron fuera y el scope es
// spec. Sin --spec el mismo high bloquea con scope full.
func TestCA255_VerifySpecAcotaPorTarea(t *testing.T) {
	m := foConUmbral(t, "high")
	spec := foSpecAprobado(t, m.Dir, "tarea-a")
	ajeno := foAlta(t, m, "high", "risk", "tarea-b")

	v, _, err := Run(m, Options{Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	g := gateNamed(v, "findings_open")
	if g == nil || g.Status != verdict.StatusPass || g.Scope != "spec" {
		t.Fatalf("CA-255: con --spec de tarea-a el high de tarea-b no bloquea (PASS, scope spec): %+v", g)
	}
	if !strings.Contains(g.Notes, "1 de otras tareas no cuentan") || !strings.Contains(g.Notes, "tarea tarea-a + sin tarea") {
		t.Fatalf("CA-255: la nota muestra el alcance y cuantos quedaron fuera: %q", g.Notes)
	}
	if v.Verdict != "green" {
		t.Fatalf("CA-255: veredicto VERDE con --spec: %+v %+v", v.Summary, v.Gates)
	}

	v, _, err = Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	g = gateNamed(v, "findings_open")
	if g == nil || g.Status != verdict.StatusFail || g.Scope != "full" || !strings.Contains(g.Notes, ajeno.ID) {
		t.Fatalf("CA-255: sin --spec el mismo high bloquea con scope full: %+v", g)
	}
	if v.Verdict != "red" {
		t.Fatalf("CA-255: sin --spec el veredicto es ROJO: %+v", v.Summary)
	}

	// un sin tarea si cuenta con --spec
	sin := foAlta(t, m, "high", "risk", "")
	v, _, err = Run(m, Options{Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	if g := gateNamed(v, "findings_open"); g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.Notes, sin.ID) {
		t.Fatalf("CA-255: con --spec el high sin tarea cuenta: %+v", g)
	}
}

// CA-256: una corrida --gate no incluye findings_open aunque block_on este
// configurado, ni en el veredicto ni en los eventos ni en el total vivo; y
// con --spec los gates de spec si corren pero findings_open no.
func TestCA256_GateNoCorreFindingsOpen(t *testing.T) {
	m := foConUmbral(t, "high")
	foAlta(t, m, "high", "risk", "")
	spec := foSpecAprobado(t, m.Dir, "tarea-a")
	for _, opt := range []Options{
		{Gates: []string{"test"}},
		{Gates: []string{"test"}, Full: true},
		{Gates: []string{"test", "build"}},
		{Gates: []string{"test"}, Spec: spec},
	} {
		v, _, err := Run(m, opt)
		if err != nil {
			t.Fatal(err)
		}
		if !v.Partial {
			t.Fatalf("CA-256 (%+v): una corrida --gate es PARCIAL: %+v", opt, v)
		}
		if gateNamed(v, "findings_open") != nil {
			t.Fatalf("CA-256 (%+v): --gate no incluye findings_open: %+v", opt, v.Gates)
		}
		evs := readLiveEvents(t, m.Dir)
		quiero := len(m.Gates)
		if opt.Spec != "" {
			quiero += 3
			if gateNamed(v, "spec_approved") == nil {
				t.Fatalf("CA-256 (%+v): los gates de spec si corren con --gate --spec: %+v", opt, v.Gates)
			}
		}
		if evs[0].Gates != quiero {
			t.Fatalf("CA-256 (%+v): el total vivo no cuenta findings_open: %d, esperaba %d", opt, evs[0].Gates, quiero)
		}
		if s, e := foEventosDe(evs, "findings_open"); len(s)+len(e) != 0 {
			t.Fatalf("CA-256 (%+v): sin eventos de findings_open en --gate: %+v %+v", opt, s, e)
		}
	}

	// Control: sin --gate el mismo arbol SI corre findings_open (y sale ROJO);
	// sin esto los asertos de arriba pasarian con un gate que nunca corre.
	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if gateNamed(v, "findings_open") == nil || v.Verdict != "red" {
		t.Fatalf("CA-256 (control): la corrida completa si trae findings_open: %v", foNombres(v))
	}
}

// CA-256: --gate findings_open es *cliargs.UsageError (exit 2) aunque
// hoom.yaml declare un gate con ese nombre; el mensaje no lo ofrece entre los
// seleccionables, y el rechazo no deja veredicto ni toca la narracion.
func TestCA256_FindingsOpenEsNombreReservado(t *testing.T) {
	declarado := foGatesBaratos + "  findings_open:\n    required: true\n    cmd: \"true\"\n"
	for nombre, yml := range map[string]struct{ gates, findings string }{
		"declarado y con block_on": {declarado, "findings:\n  block_on: high\n"},
		"declarado sin block_on":   {declarado, ""},
		"sin declarar":             {foGatesBaratos, "findings:\n  block_on: high\n"},
	} {
		m := foProyecto(t, yml.gates, yml.findings)
		centinela := sembrarNarracion(t, m.Dir)
		antes := huellaDir(t, m.Dir)
		for _, sel := range [][]string{{"findings_open"}, {"test", "findings_open"}} {
			v, path, err := Run(m, Options{Gates: sel})
			ue := comoUso(t, "CA-256 ("+nombre+")", err)
			if v != nil || path != "" {
				t.Fatalf("CA-256 (%s, %v): el rechazo no produce veredicto: v=%+v path=%q", nombre, sel, v, path)
			}
			linea := primeraLinea(ue.Error())
			if !strings.Contains(linea, `"findings_open"`) {
				t.Fatalf("CA-256 (%s, %v): el mensaje nombra el gate pedido: %q", nombre, sel, linea)
			}
			_, lista, _ := strings.Cut(linea, "este proyecto declara:")
			if strings.Contains(lista, "findings_open") {
				t.Fatalf("CA-256 (%s, %v): findings_open no se ofrece como seleccionable: %q", nombre, sel, linea)
			}
			if !strings.Contains(lista, "test") {
				t.Fatalf("CA-256 (%s, %v): el mensaje sigue listando los gates del proyecto: %q", nombre, sel, linea)
			}
		}
		if got := narracion(t, m.Dir); string(got) != string(centinela) {
			t.Fatalf("CA-256 (%s): la narracion fue tocada por un rechazo", nombre)
		}
		exigeMismoArbol(t, "CA-256 ("+nombre+")", antes, huellaDir(t, m.Dir))
	}
}

// CA-257: con el gate activo verify emite gate_start y gate_end de
// findings_open (scope spec con --spec) y el total de verify_start lo
// cuenta; en el veredicto va despues de los gates de spec y antes del primer
// gate del proyecto.
func TestCA257_EventosYPosicionConSpec(t *testing.T) {
	m := foConUmbral(t, "high")
	spec := foSpecAprobado(t, m.Dir, "tarea-a")
	foAlta(t, m, "medium", "risk", "")
	v, _, err := Run(m, Options{Spec: spec})
	if err != nil {
		t.Fatal(err)
	}
	evs := readLiveEvents(t, m.Dir)
	if evs[0].Kind != live.KindVerifyStart || evs[0].Gates != len(m.Gates)+3+1 {
		t.Fatalf("CA-257: verify_start declara gates+3+1 (spec + findings_open): %+v", evs[0])
	}
	starts, ends := foEventosDe(evs, "findings_open")
	if len(starts) != 1 || len(ends) != 1 {
		t.Fatalf("CA-257: un gate_start y un gate_end de findings_open: starts=%+v ends=%+v", starts, ends)
	}
	if starts[0].Scope != "spec" {
		t.Fatalf("CA-257: el gate_start lleva scope spec con --spec: %+v", starts[0])
	}
	g := gateNamed(v, "findings_open")
	if g == nil || ends[0].Status != g.Status || g.Scope != "spec" {
		t.Fatalf("CA-257: el gate_end narra el mismo status que el veredicto: evento=%+v gate=%+v", ends[0], g)
	}

	fo := foIndice(v, "findings_open")
	for _, s := range []string{"spec_lint", "spec_trace", "spec_approved"} {
		if i := foIndice(v, s); i < 0 || i > fo {
			t.Fatalf("CA-257: findings_open va despues de %s: %v", s, gatesEjecutados(v))
		}
	}
	for n := range m.Gates {
		if i := foIndice(v, n); i < fo {
			t.Fatalf("CA-257: findings_open va antes del gate del proyecto %s: %v", n, foNombres(v))
		}
	}
}

// CA-257: sin --spec findings_open es el primer gate, scope full, y con
// --full y trinquete va antes de los del proyecto y el trinquete al final.
func TestCA257_PosicionSinSpecYConTrinquete(t *testing.T) {
	m := foConUmbral(t, "high")
	v, _, err := Run(m, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Gates) == 0 || v.Gates[0].Name != "findings_open" || v.Gates[0].Scope != "full" {
		t.Fatalf("CA-257: sin --spec findings_open es el primer gate con scope full: %v", foNombres(v))
	}
	evs := readLiveEvents(t, m.Dir)
	if evs[0].Gates != len(m.Gates)+1 {
		t.Fatalf("CA-257: verify_start cuenta findings_open: %d, esperaba %d", evs[0].Gates, len(m.Gates)+1)
	}
	if starts, _ := foEventosDe(evs, "findings_open"); len(starts) != 1 || starts[0].Scope != "full" {
		t.Fatalf("CA-257: gate_start de findings_open con scope full: %+v", starts)
	}

	base := 70.0
	writeRatchet(t, m.Dir, "echo 80", &base)
	v, _, err = Run(m, Options{Full: true})
	if err != nil {
		t.Fatal(err)
	}
	fo, rt := foIndice(v, "findings_open"), foIndice(v, "ratchet")
	if fo != 0 || rt != len(v.Gates)-1 {
		t.Fatalf("CA-257: orden findings_open, gates del proyecto, trinquete: %v", foNombres(v))
	}
	if evs := readLiveEvents(t, m.Dir); evs[0].Gates != len(m.Gates)+2 {
		t.Fatalf("CA-257: --full con trinquete declara gates+findings_open+ratchet: %+v", evs[0])
	}
	if f, _ := ratchet.Load(m.Dir); f == nil || *f.Metrics["cobertura"].Value != 80 {
		t.Fatalf("CA-257: el trinquete sigue funcionando con el gate activo")
	}
}

func foNombres(v *verdict.Verdict) []string {
	var out []string
	for _, g := range v.Gates {
		out = append(out, g.Name)
	}
	return out
}
