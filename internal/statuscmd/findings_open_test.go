// Tests adversariales del spec .hoom/specs/gate-findings-open.md (CA-253,
// CA-259): hoom status cuenta con la definicion unica de "abierto", dice si
// el gate findings_open esta activo y cuantos bloquean, y sigue sin escribir
// nada.
package statuscmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
)

func foAlta(t *testing.T, dir, sev, task string) finding.Finding {
	t.Helper()
	f, err := finding.Register(dir, "main", finding.Draft{Severity: sev, Lens: "risk",
		Description: "hallazgo " + sev, Author: "reviewer@test", Task: task})
	if err != nil {
		t.Fatalf("fixture: Register: %v", err)
	}
	return f
}

func foCrudo(t *testing.T, dir, nombre, contenido string) {
	t.Helper()
	d := filepath.Join(dir, ".hoom", "findings")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, nombre), []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

func foLineaDelGate(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, "gate findings_open") {
			return l
		}
	}
	return ""
}

// CA-259: BuildFor expone el umbral y cuantos abiertos llegan a el, con
// alcance "todos" (status no tiene spec: cuenta tambien los de otra tarea).
func TestCA259_BuildForExponeUmbralYBloqueantes(t *testing.T) {
	dir := initProject(t)
	foAlta(t, dir, "high", "otra-tarea")
	foAlta(t, dir, "medium", "")
	foAlta(t, dir, "low", "")
	for umbral, quiero := range map[string]int{"high": 1, "medium": 2, "low": 3} {
		s, err := BuildFor(dir, "main", umbral)
		if err != nil {
			t.Fatal(err)
		}
		if s.Findings.BlockOn != umbral || s.Findings.Blocking != quiero {
			t.Fatalf("CA-259: con block_on %s esperaba block_on=%q blocking=%d: %+v", umbral, umbral, quiero, s.Findings)
		}
		if s.Findings.Open != 3 || s.Findings.OpenHigh != 1 {
			t.Fatalf("CA-259: open/open_high no dependen del umbral: %+v", s.Findings)
		}
	}
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Findings.BlockOn != "" {
		t.Fatalf("CA-259: Build es el gate apagado: block_on vacio: %+v", s.Findings)
	}
}

// CA-253 + CA-259: status cuenta con la definicion unica. Un high con
// evidencia en blanco o 'as' inventado sigue abierto (hoy decia "sin
// abiertos"); uno corregido con evidencia no cuenta.
func TestCA259_StatusUsaLaDefinicionUnica(t *testing.T) {
	dir := initProject(t)
	blanco := foAlta(t, dir, "high", "")
	foCrudo(t, dir, blanco.ID+".res.json", `{"finding_id": "`+blanco.ID+`", "as": "refutado", "evidence": "   "}`)
	inventado := foAlta(t, dir, "high", "")
	foCrudo(t, dir, inventado.ID+".res.json", `{"finding_id": "`+inventado.ID+`", "as": "wontfix", "evidence": "no aplica"}`)
	cerrado := foAlta(t, dir, "high", "")
	if _, err := finding.Resolve(dir, cerrado.ID, "corregido", "commit abc con test", ""); err != nil {
		t.Fatal(err)
	}

	s, err := BuildFor(dir, "main", "high")
	if err != nil {
		t.Fatal(err)
	}
	if s.Findings.Open != 2 || s.Findings.OpenHigh != 2 || s.Findings.Blocking != 2 {
		t.Fatalf("CA-259: los dos mal cerrados siguen abiertos y bloquean; el corregido no: %+v", s.Findings)
	}
	var buf bytes.Buffer
	Render(&buf, s, false)
	if strings.Contains(buf.String(), "sin abiertos") {
		t.Fatalf("CA-253: status no puede decir 'sin abiertos' con dos high mal cerrados:\n%s", buf.String())
	}
}

// CA-259: el JSON lleva findings.block_on y findings.blocking; con el gate
// apagado block_on va vacio (la clave existe).
func TestCA259_JSONLlevaUmbralYBloqueantes(t *testing.T) {
	dir := initProject(t)
	foAlta(t, dir, "high", "")
	leer := func(blockOn string) map[string]any {
		s, err := BuildFor(dir, "main", blockOn)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := JSONBytes(s)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Findings map[string]any `json:"findings"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m.Findings
	}
	on := leer("high")
	if on["block_on"] != "high" || on["blocking"] != float64(1) {
		t.Fatalf("CA-259: findings.block_on=high y findings.blocking=1: %v", on)
	}
	off := leer("")
	if v, ok := off["block_on"]; !ok || v != "" {
		t.Fatalf("CA-259: sin umbral la clave block_on existe y va vacia: %v", off)
	}
	if _, ok := off["blocking"]; !ok {
		t.Fatalf("CA-259: la clave blocking existe siempre: %v", off)
	}
}

// CA-259: con el gate activo el texto agrega 'gate findings_open: N bloquean
// (block_on: X)', en rojo si N > 0, y aparece aunque no haya abiertos; con el
// gate apagado el texto no lo menciona.
func TestCA259_TextoDelGate(t *testing.T) {
	dir := initProject(t)
	s, err := BuildFor(dir, "main", "high")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	Render(&buf, s, false)
	if !strings.Contains(buf.String(), "gate findings_open: 0 bloquean (block_on: high)") {
		t.Fatalf("CA-259: con el gate activo y sin abiertos la linea aparece con 0:\n%s", buf.String())
	}
	buf.Reset()
	Render(&buf, s, true)
	if l := foLineaDelGate(buf.String()); l == "" || strings.Contains(l, cRed) {
		t.Fatalf("CA-259: con 0 bloqueantes la linea del gate no va en rojo: %q", l)
	}

	foAlta(t, dir, "high", "")
	foAlta(t, dir, "low", "")
	s, err = BuildFor(dir, "main", "high")
	if err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	Render(&buf, s, false)
	if !strings.Contains(buf.String(), "gate findings_open: 1 bloquean (block_on: high)") {
		t.Fatalf("CA-259: la linea dice cuantos bloquean y el umbral:\n%s", buf.String())
	}
	if strings.Contains(buf.String(), "\033") {
		t.Fatalf("CA-259: sin color no hay ANSI:\n%q", buf.String())
	}
	buf.Reset()
	Render(&buf, s, true)
	if l := foLineaDelGate(buf.String()); !strings.Contains(l, cRed) {
		t.Fatalf("CA-259: con bloqueantes la parte del gate va en rojo: %q", l)
	}

	s, err = Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	Render(&buf, s, false)
	if strings.Contains(buf.String(), "findings_open") {
		t.Fatalf("CA-259: con el gate apagado el texto no menciona findings_open:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "hallazgos:") {
		t.Fatalf("CA-259: la linea de hallazgos sigue como hoy:\n%s", buf.String())
	}
}

// CA-259: Run usa Options.BlockOn en todas sus pieles: --json, texto y
// --watch sin TTY.
func TestCA259_RunUsaElUmbralDeLasOpciones(t *testing.T) {
	dir := initProject(t)
	foAlta(t, dir, "medium", "")

	var buf bytes.Buffer
	if err := Run(dir, "main", &buf, Options{JSON: true, BlockOn: "medium"}); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Findings FindingsSummary `json:"findings"`
	}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("CA-259: --json es JSON: %v\n%s", err, buf.String())
	}
	if m.Findings.BlockOn != "medium" || m.Findings.Blocking != 1 {
		t.Fatalf("CA-259: Run --json con BlockOn medium: %+v", m.Findings)
	}

	for _, opt := range []Options{{BlockOn: "medium"}, {Watch: true, TTY: false, BlockOn: "medium"}} {
		buf.Reset()
		if err := Run(dir, "main", &buf, opt); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(buf.String(), "gate findings_open: 1 bloquean (block_on: medium)") {
			t.Fatalf("CA-259 (%+v): el texto muestra el gate con el umbral de las opciones:\n%s", opt, buf.String())
		}
	}

	buf.Reset()
	if err := Run(dir, "main", &buf, Options{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "findings_open") {
		t.Fatalf("CA-259: sin BlockOn el texto no menciona el gate:\n%s", buf.String())
	}
}

// CA-259 (borde): status informa y no bloquea: un hallazgo ilegible o con
// severidad desconocida no lo rompe, y mirar sigue siendo solo lectura.
func TestCA259_StatusRotulaSinRomperYNoEscribe(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	foAlta(t, dir, "high", "")
	foCrudo(t, dir, "roto.json", "{roto")
	foCrudo(t, dir, "critical.json", `{"id": "critical", "severity": "critical", "description": "a mano"}`)
	hoomDir := filepath.Join(dir, ".hoom")
	antes := hashTree(t, hoomDir)

	s, err := BuildFor(dir, "main", "high")
	if err != nil {
		t.Fatalf("CA-259: status rotula sin romper ante hallazgos ilegibles: %v", err)
	}
	if s.Findings.BlockOn != "high" || s.Findings.Blocking < 1 {
		t.Fatalf("CA-259: el high legible sigue contando: %+v", s.Findings)
	}
	var buf bytes.Buffer
	Render(&buf, s, true)
	if err := Run(dir, "main", &buf, Options{JSON: true, BlockOn: "high"}); err != nil {
		t.Fatal(err)
	}
	if despues := hashTree(t, hoomDir); despues != antes {
		t.Fatalf("CA-259: status con el gate activo modifico .hoom (hash %s -> %s)", antes, despues)
	}
}
