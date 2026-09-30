// Tests adversariales del spec .hoom/specs/quien-cierra-un-hallazgo.md
// (CA-437) sobre la API de Go: Resolution gana role y run, omitidos si estan
// vacios; fuera de una corrida la resolucion es la de hoy, sin esas claves; y
// una resolucion escrita dentro de una corrida se lee con su rol y su
// corrida. La negativa dentro de una corrida es de la CLI
// (cmd/hoom/quien_cierra_test.go).
package finding

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// qcFueraDeCorrida deja el proceso sin HOOM_ROLE/HOOM_RUN/HOOM_PROVIDER (los
// restaura al final): la suite puede correr dentro de la corrida de un rol.
func qcFueraDeCorrida(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER"} {
		t.Setenv(k, "")
		if err := os.Unsetenv(k); err != nil {
			t.Fatal(err)
		}
	}
}

// CA-437: role y run van al JSON con esas claves, y se omiten vacios.
func TestCA437_ResolutionRoleYRunOmitidosSiVacios(t *testing.T) {
	cuando := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	raw, err := json.Marshal(Resolution{FindingID: "f1", As: StatusRefuted, Evidence: "TestX", Author: "Henry", ResolvedAt: cuando})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["role"]; ok {
		t.Fatalf("CA-437: sin rol la resolucion no lleva la clave role: %s", raw)
	}
	if _, ok := m["run"]; ok {
		t.Fatalf("CA-437: sin corrida la resolucion no lleva la clave run: %s", raw)
	}

	raw, err = json.Marshal(Resolution{FindingID: "f1", As: StatusRefuted, Evidence: "TestX",
		Author: "refutador@codex (run 20260930T120000_ab12cd)", ResolvedAt: cuando,
		Role: "refutador", Run: "20260930T120000_ab12cd"})
	if err != nil {
		t.Fatal(err)
	}
	m = nil
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["role"] != "refutador" || m["run"] != "20260930T120000_ab12cd" {
		t.Fatalf("CA-437: la resolucion de una corrida lleva role y run: %s", raw)
	}
}

// CA-437 (guarda): fuera de una corrida, Resolve es el de hoy: el autor que
// se le da, o la identidad de git, y el .res.json sin role ni run.
func TestCA437_ResolveFueraDeUnaCorridaEsElDeHoy(t *testing.T) {
	qcFueraDeCorrida(t)
	root := initRepo(t)

	a, err := Add(root, "main", "high", "risk", "app.go", "el retry no respeta el backoff", "reviewer@codex")
	if err != nil {
		t.Fatal(err)
	}
	res, err := Resolve(root, a.ID, StatusRefuted, "TestRetry lo refuta", "Henry Orellana")
	if err != nil {
		t.Fatalf("CA-437: fuera de una corrida Resolve funciona como hoy: %v", err)
	}
	if res.Author != "Henry Orellana" || res.Role != "" || res.Run != "" {
		t.Fatalf("CA-437: fuera de una corrida el autor es el dado y no hay rol ni corrida: %+v", res)
	}

	b, err := Add(root, "main", "medium", "risk", "app.go", "otro", "reviewer@codex")
	if err != nil {
		t.Fatal(err)
	}
	res, err = Resolve(root, b.ID, StatusCorrected, "commit abc123 con el gate verde", "")
	if err != nil {
		t.Fatalf("CA-437: fuera de una corrida corregido funciona como hoy: %v", err)
	}
	if res.Role != "" || res.Run != "" || res.Author == "" {
		t.Fatalf("CA-437: sin autor sale la identidad de git, sin rol ni corrida: %+v", res)
	}

	for _, id := range []string{a.ID, b.ID} {
		raw, err := os.ReadFile(filepath.Join(root, ".hoom", "findings", id+".res.json"))
		if err != nil {
			t.Fatalf("CA-437: falta la resolucion de %s: %v", id, err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("CA-437: la resolucion es JSON: %v", err)
		}
		if _, ok := m["role"]; ok {
			t.Fatalf("CA-437: fuera de una corrida la resolucion no lleva role: %s", raw)
		}
		if _, ok := m["run"]; ok {
			t.Fatalf("CA-437: fuera de una corrida la resolucion no lleva run: %s", raw)
		}
	}
}

// CA-437: una resolucion escrita dentro de la corrida del refutador (con
// role y run) cierra el hallazgo y List la devuelve con su rol y su corrida.
func TestCA437_ListLeeElRolYLaCorridaDeLaResolucion(t *testing.T) {
	qcFueraDeCorrida(t)
	root := initRepo(t)
	f, err := Add(root, "main", "high", "risk", "app.go", "el retry no respeta el backoff", "reviewer@codex")
	if err != nil {
		t.Fatal(err)
	}
	cuerpo := `{"finding_id":"` + f.ID + `","as":"refutado","evidence":"TestRetry lo refuta",` +
		`"author":"refutador@codex (run 20260930T120000_ab12cd)","resolved_at":"2026-09-30T12:00:00Z",` +
		`"role":"refutador","run":"20260930T120000_ab12cd"}` + "\n"
	if err := os.WriteFile(filepath.Join(root, ".hoom", "findings", f.ID+".res.json"), []byte(cuerpo), 0o644); err != nil {
		t.Fatal(err)
	}
	items, warns, err := List(root, "main", false)
	if err != nil || len(warns) != 0 {
		t.Fatalf("CA-437: List sin avisos: %v %v", err, warns)
	}
	for _, it := range items {
		if it.ID != f.ID {
			continue
		}
		if it.Status != StatusRefuted || it.Resolution == nil {
			t.Fatalf("CA-437: la resolucion del refutador cierra el hallazgo: %+v", it)
		}
		if it.Resolution.Role != "refutador" || it.Resolution.Run != "20260930T120000_ab12cd" ||
			it.Resolution.Author != "refutador@codex (run 20260930T120000_ab12cd)" {
			t.Fatalf("CA-437: List devuelve el rol, la corrida y la firma: %+v", it.Resolution)
		}
		return
	}
	t.Fatalf("CA-437: List devuelve el hallazgo %s", f.ID)
}
