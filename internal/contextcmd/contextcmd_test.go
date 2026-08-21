// Tests adversariales del spec .hoom/specs/contexto-salud.md
// (CA-40..CA-44, CA-46): la salud del contexto se mide con archivos y
// fechas, con amarillos honestos, y jamas bloquea.
package contextcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// newProject arma un proyecto SIN git a proposito: docDate cae a mtime,
// que controlamos con Chtimes — determinismo total en el test.
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{".hoom/intake", ".hoom/specs"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func writeAt(t *testing.T, root, rel, content string, at time.Time) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}

func check(t *testing.T, r Report, name string) Check {
	t.Helper()
	for _, c := range r.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("no existe el check %q en %+v", name, r.Checks)
	return Check{}
}

// CA-40: el JSON trae fuentes (cantidad y mas nuevo), estado de vision y
// backlog, y la cuenta de preguntas abiertas.
func TestCA40_ReporteCompleto(t *testing.T) {
	root := newProject(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	writeAt(t, root, ".hoom/intake/srs-v1.md", "# SRS\n", base)
	writeAt(t, root, ".hoom/intake/srs-v2.md", "# SRS v2\n", base.Add(48*time.Hour))
	writeAt(t, root, ".hoom/specs/00-vision.md",
		"# Vision\n\nPREGUNTA PARA EL CLIENTE: precios netos o brutos?\n", base.Add(72*time.Hour))
	writeAt(t, root, ".hoom/specs/backlog.md",
		"# Backlog\n\nPREGUNTA PARA EL CLIENTE: hay descuentos por volumen?\n", base.Add(72*time.Hour))

	raw, err := JSONBytes(root)
	if err != nil {
		t.Fatal(err)
	}
	var r Report
	if err := json.Unmarshal(raw, &r); err != nil {
		t.Fatal(err)
	}
	if r.Intake.Count != 2 || r.Intake.NewestName != "srs-v2.md" {
		t.Fatalf("CA-40: fuentes mal medidas: %+v", r.Intake)
	}
	if !r.Vision.Exists || !r.Backlog.Exists {
		t.Fatalf("CA-40: vision/backlog deben reportar existencia: %+v %+v", r.Vision, r.Backlog)
	}
	if r.OpenQuestions != 2 {
		t.Fatalf("CA-40: esperaba 2 preguntas abiertas, hay %d", r.OpenQuestions)
	}
	if r.Status != "amarillo" {
		t.Fatalf("CA-40: con preguntas abiertas el estado global es amarillo, es %q", r.Status)
	}
}

// CA-41: intake mas nuevo que la vision => amarillo de posible
// desactualizacion; vision posterior al intake => frescura ok.
func TestCA41_FrescuraDeLaVision(t *testing.T) {
	root := newProject(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	writeAt(t, root, ".hoom/specs/00-vision.md", "# Vision\n", base)
	writeAt(t, root, ".hoom/specs/backlog.md", "# Backlog\n", base)
	writeAt(t, root, ".hoom/intake/cambio-alcance.md", "# Cambio\n", base.Add(24*time.Hour))

	r := Build(root)
	fresh := check(t, r, "frescura")
	if fresh.Status != StatusWarn || !strings.Contains(fresh.Detail, "desactualizada") {
		t.Fatalf("CA-41: intake mas nuevo debe dar amarillo de desactualizacion: %+v", fresh)
	}
	if r.Status != "amarillo" {
		t.Fatalf("CA-41: el estado global debe ser amarillo, es %q", r.Status)
	}

	// la vision se actualiza despues del documento => frescura ok
	writeAt(t, root, ".hoom/specs/00-vision.md", "# Vision v2\n", base.Add(48*time.Hour))
	if fresh = check(t, Build(root), "frescura"); fresh.Status != StatusOK {
		t.Fatalf("CA-41: vision posterior al intake debe ser ok: %+v", fresh)
	}
}

// CA-42: backlog sin intake => amarillo (sin fuente); intake sin vision =>
// amarillo (sin destilar).
func TestCA42_HuellasDeOrigen(t *testing.T) {
	root := newProject(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	writeAt(t, root, ".hoom/specs/backlog.md", "# Backlog\n", base)

	r := Build(root)
	fuentes := check(t, r, "fuentes")
	if fuentes.Status != StatusWarn || !strings.Contains(fuentes.Detail, "no tiene fuente") {
		t.Fatalf("CA-42: backlog sin intake debe avisar que no tiene fuente: %+v", fuentes)
	}

	root2 := newProject(t)
	writeAt(t, root2, ".hoom/intake/srs.md", "# SRS\n", base)
	vision := check(t, Build(root2), "vision")
	if vision.Status != StatusWarn || !strings.Contains(vision.Detail, "sin destilar") {
		t.Fatalf("CA-42: intake sin vision debe avisar 'sin destilar': %+v", vision)
	}
}

// CA-43: proyecto sin contexto en absoluto => amarillo con la accion
// exacta: la entrevista fundacional (Modo C).
func TestCA43_SinContextoProponeEntrevista(t *testing.T) {
	r := Build(newProject(t))
	if r.Status != "amarillo" {
		t.Fatalf("CA-43: sin contexto el estado es amarillo, es %q", r.Status)
	}
	ctx := check(t, r, "contexto")
	if ctx.Status != StatusWarn || !strings.Contains(ctx.Action, "entrevista fundacional") || !strings.Contains(ctx.Action, "Modo C") {
		t.Fatalf("CA-43: la accion debe proponer la entrevista fundacional (Modo C): %+v", ctx)
	}
}

// CA-44: el contexto informa pero JAMAS bloquea: el estado solo puede ser
// verde o amarillo, en cualquier combinacion de entradas.
func TestCA44_NuncaRojoNuncaBloquea(t *testing.T) {
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	escenarios := []func(root string){
		func(root string) {}, // vacio total
		func(root string) { writeAt(t, root, ".hoom/specs/backlog.md", "x\n", base) },
		func(root string) {
			writeAt(t, root, ".hoom/intake/a.md", "x\n", base.Add(time.Hour))
			writeAt(t, root, ".hoom/specs/00-vision.md", "PREGUNTA PARA EL CLIENTE: ?\n", base)
		},
	}
	for i, setup := range escenarios {
		root := newProject(t)
		setup(root)
		r := Build(root)
		if r.Status != "verde" && r.Status != "amarillo" {
			t.Fatalf("CA-44: escenario %d produjo estado prohibido %q (no existe el rojo de contexto)", i, r.Status)
		}
	}
}

// CA-46: las preguntas se cuentan por el marcador LITERAL del contrato;
// variantes no cuentan; sin archivos la cuenta es 0 sin error.
func TestCA46_MarcadorLiteral(t *testing.T) {
	root := newProject(t)
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	writeAt(t, root, ".hoom/specs/00-vision.md",
		"PREGUNTA PARA EL CLIENTE: una\n"+
			"pregunta para el cliente: minusculas NO cuentan\n"+
			"- PREGUNTA PARA EL CLIENTE: dos, con vinieta\n", base)

	if n := Build(root).OpenQuestions; n != 2 {
		t.Fatalf("CA-46: esperaba 2 preguntas (la variante en minusculas no cuenta), hay %d", n)
	}
	if n := Build(newProject(t)).OpenQuestions; n != 0 {
		t.Fatalf("CA-46: sin archivos la cuenta es 0, es %d", n)
	}
}
