package agentcmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
)

// Pruebas propias del writer (autonomia-entre-firmas): la foto de los tests
// protegidos, su lista desde los sobres y la regla del gate.

func TestWriterTestsProtegidosUnionDeTestWriter(t *testing.T) {
	root := t.TempDir()
	recs := []envelope.Record{
		{ID: "a1", Role: "test-writer", Task: "t", Trasplantados: []string{"b_test.go", "a_test.go"}},
		{ID: "a2", Role: "test-writer", Task: "t", Trasplantados: []string{"a_test.go", "c_test.go"}},
		{ID: "a3", Role: "test-writer", Task: "otra", Trasplantados: []string{"x_test.go"}},
		{ID: "a4", Role: "writer", Task: "t", Trasplantados: []string{"w_test.go"}},
		{ID: "a5", Role: "test-writer", Task: "t"},
	}
	for _, r := range recs {
		if err := envelope.Write(root, r); err != nil {
			t.Fatal(err)
		}
	}
	got, err := TestsProtegidos(root, "t")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a_test.go", "b_test.go", "c_test.go"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("TestsProtegidos = %v, quiero %v", got, want)
	}
	if got, _ := TestsProtegidos(root, ""); len(got) != 0 {
		t.Fatalf("sin tarea no protege nada: %v", got)
	}
}

func TestWriterFotoProtegidos(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "pkg", "a_test.go"), []byte("package pkg\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	foto := FotoProtegidos(dir, []string{"pkg/a_test.go", "pkg/no_test.go"})
	if foto["pkg/no_test.go"] != ProtegidoAusente {
		t.Fatalf("un protegido que no esta es %q, quiero %q", foto["pkg/no_test.go"], ProtegidoAusente)
	}
	if h := foto["pkg/a_test.go"]; len(h) != 64 {
		t.Fatalf("la huella de un archivo regular es su sha256, fue %q", h)
	}
	if len(FotoProtegidos(dir, nil)) != 0 {
		t.Fatal("sin rutas la foto esta vacia")
	}
}

func TestWriterGateTestsDelTestWriter(t *testing.T) {
	qcLimpiarEntorno(t)
	root := repo(t)
	write(t, root, "app_test.go", "package app\n")
	write(t, root, "otro_test.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "tests del test-writer")
	writer, err := agents.Lookup("writer")
	if err != nil {
		t.Fatal(err)
	}
	pol := PolicyFor(nil, writer)
	pol.Protegidos = FotoProtegidos(root, []string{"app_test.go", "otro_test.go"})
	before := Take(root, "main")
	write(t, root, "app_test.go", "package app\n// aflojado\n")
	if err := os.Remove(filepath.Join(root, "otro_test.go")); err != nil {
		t.Fatal(err)
	}
	write(t, root, "nuevo_test.go", "package app\n")
	res := Gate(root, "main", "t", writer, before, Take(root, "main"), pol, nil)
	if !res.Cuts() || res.OK || res.Tampering {
		t.Fatalf("la regla corta sin ser manipulacion de la evidencia: %+v", res)
	}
	items, _, err := finding.List(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	porID := map[string]finding.Finding{}
	for _, it := range items {
		porID[it.ID] = it.Finding
	}
	var reglas []string
	for _, v := range res.Violations {
		reglas = append(reglas, v.Path+"="+v.Rule)
		if v.Rule != RuleTestsDelTestWriter {
			continue
		}
		f, ok := porID[v.FindingID]
		if !ok {
			t.Fatalf("la violacion %s no tiene hallazgo", v.Path)
		}
		want := "tests-del-test-writer: el rol writer escribio " + v.Path + " - " + DetalleTestsDelTestWriter
		if f.Description != want || f.Severity != "high" || f.Author != "hoom gate de scope" {
			t.Fatalf("hallazgo %+v, quiero %q", f, want)
		}
	}
	if got := strings.Join(reglas, " "); got != "app_test.go=tests-del-test-writer otro_test.go=tests-del-test-writer" {
		t.Fatalf("violaciones: %s", got)
	}

	tw, err := agents.Lookup("test-writer")
	if err != nil {
		t.Fatal(err)
	}
	before = Take(root, "main")
	write(t, root, "app_test.go", "package app\n// otra vez\n")
	twPol := PolicyFor(nil, tw)
	twPol.Protegidos = pol.Protegidos
	if res := Gate(root, "main", "t", tw, before, Take(root, "main"), twPol, nil); res.Cuts() {
		t.Fatalf("un test-writer no se protege de si mismo: %+v", res)
	}
}

func TestWriterDeTestsDelTestWriter(t *testing.T) {
	v := Violation{Rule: RuleTestsDelTestWriter, Path: "a_test.go", Detail: DetalleTestsDelTestWriter}
	f := finding.Finding{Author: AutorGateDeScope, Severity: "high", Description: DescripcionViolacion(v, "writer")}
	if !DeTestsDelTestWriter(f, "writer") {
		t.Fatalf("el hallazgo del gate: %q", f.Description)
	}
	otro := f
	otro.Description = DescripcionViolacion(Violation{Rule: RuleOutOfScope, Path: "a_test.go", Detail: "x"}, "writer")
	persona := f
	persona.Author = "alguien"
	for _, c := range []struct {
		f   finding.Finding
		rol string
	}{{f, "reviewer"}, {otro, "writer"}, {persona, "writer"}} {
		if DeTestsDelTestWriter(c.f, c.rol) {
			t.Fatalf("no es el de tests-del-test-writer de %s: %+v", c.rol, c.f)
		}
	}
}
