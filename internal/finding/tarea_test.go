// Tests adversariales del spec .hoom/specs/arquitecto-bajo-el-sobre.md
// (CA-242): los hallazgos saben de que tarea son. La tarea es una etiqueta de
// pertenencia validada por FORMA (la de 'hoom task start'), no por
// existencia; una tarea invalida no escribe nada.
package finding

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// abArtefacto lee el JSON crudo del hallazgo tal como quedo en disco.
func abArtefacto(t *testing.T, root, id string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "findings", id+".json"))
	if err != nil {
		t.Fatalf("CA-242: falta el artefacto del hallazgo %s: %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-242: el artefacto debe ser JSON legible: %v", err)
	}
	return m
}

func abCuantos(t *testing.T, root string) int {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".hoom", "findings"))
	if err != nil {
		return 0
	}
	return len(entries)
}

// CA-242: Register con Task la escribe en el artefacto y la devuelven List y
// JSONBytes; Add no escribe la clave.
func TestCA242_RegisterAtaLaTarea(t *testing.T) {
	root := initRepo(t)
	f, err := Register(root, "main", Draft{
		Severity: "high", Lens: "risk", File: "app.go",
		Description: "el sobre certifico la nada", Author: "reviewer@claude", Task: "cabina-visual",
	})
	if err != nil {
		t.Fatalf("CA-242: una tarea con forma valida se acepta: %v", err)
	}
	if f.Task != "cabina-visual" || f.ID == "" || f.Severity != "high" || f.Lens != "risk" ||
		f.File != "app.go" || f.Author != "reviewer@claude" || f.Fingerprint == "" {
		t.Fatalf("CA-242: Register sella el hallazgo como Add, mas su tarea: %+v", f)
	}
	art := abArtefacto(t, root, f.ID)
	if art["task"] != "cabina-visual" {
		t.Fatalf("CA-242: el artefacto debe llevar \"task\": \"cabina-visual\": %v", art)
	}

	// Add conserva su firma y no escribe la clave
	g, err := Add(root, "main", "low", "readability", "", "sin tarea", "")
	if err != nil {
		t.Fatal(err)
	}
	if g.Task != "" {
		t.Fatalf("CA-242: Add es Register sin Task: %+v", g)
	}
	if _, ok := abArtefacto(t, root, g.ID)["task"]; ok {
		t.Fatalf("CA-242: Add no escribe la clave task: %v", abArtefacto(t, root, g.ID))
	}

	// Register con Task vacia tampoco la escribe: vacio = sin tarea
	h, err := Register(root, "main", Draft{Severity: "medium", Lens: "reliability", Description: "vacia"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := abArtefacto(t, root, h.ID)["task"]; ok || h.Task != "" {
		t.Fatalf("CA-242: una tarea vacia no escribe la clave: %+v", h)
	}

	// List la devuelve
	items, warnings, err := List(root, "main", false)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("CA-242: %v %v", err, warnings)
	}
	porID := map[string]Item{}
	for _, it := range items {
		porID[it.ID] = it
	}
	if porID[f.ID].Task != "cabina-visual" || porID[g.ID].Task != "" || porID[h.ID].Task != "" {
		t.Fatalf("CA-242: List devuelve la tarea de cada hallazgo: %+v", porID)
	}

	// JSONBytes la devuelve, y solo en el que la tiene
	raw, err := JSONBytes(root, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	var view struct {
		Findings []map[string]any `json:"findings"`
	}
	if err := json.Unmarshal(raw, &view); err != nil {
		t.Fatalf("CA-242: el JSON del listado debe ser legible: %v", err)
	}
	vistos := 0
	for _, it := range view.Findings {
		switch it["id"] {
		case f.ID:
			vistos++
			if it["task"] != "cabina-visual" {
				t.Fatalf("CA-242: JSONBytes lleva la tarea: %v", it)
			}
		case g.ID, h.ID:
			vistos++
			if _, ok := it["task"]; ok {
				t.Fatalf("CA-242: sin tarea no hay clave task en el JSON: %v", it)
			}
		}
	}
	if vistos != 3 {
		t.Fatalf("CA-242: el listado JSON trae los tres hallazgos: %s", raw)
	}

	// abierto sigue siendo abierto: la tarea no toca el ciclo de vida
	if porID[f.ID].Status != StatusOpen {
		t.Fatalf("CA-242: la tarea no cambia el estado: %+v", porID[f.ID])
	}
}

// CA-242: una tarea con forma invalida es error y no escribe nada.
func TestCA242_TareaInvalidaNoEscribeNada(t *testing.T) {
	root := initRepo(t)
	if _, err := Add(root, "main", "low", "readability", "", "previo", ""); err != nil {
		t.Fatal(err)
	}
	antes := abCuantos(t, root)
	for _, tarea := range []string{
		"../x", "Mayus", "con espacio", // los del criterio
		"-x", "x/y", "tarea_1", "ñandu", "a.b", "../../etc", "TAREA", "tarea!", "x\x00y",
	} {
		_, err := Register(root, "main", Draft{
			Severity: "high", Lens: "risk", Description: "tarea invalida", Task: tarea,
		})
		if err == nil {
			t.Fatalf("CA-242: la tarea %q no tiene forma de slug y debe ser error", tarea)
		}
		if n := abCuantos(t, root); n != antes {
			t.Fatalf("CA-242: la tarea %q fue rechazada pero escribio algo: %d -> %d archivos", tarea, antes, n)
		}
	}
	// nada escapo de .hoom/findings/ por la ruta
	if _, err := os.Stat(filepath.Join(root, "x.json")); err == nil {
		t.Fatal("CA-242: una tarea con ../ no puede escribir fuera")
	}
}

// CA-242: propiedad sobre la forma. Para cadenas al azar del alfabeto que
// importa, Register acepta exactamente las que tienen la forma de slug de
// 'hoom task start' (^[a-z0-9][a-z0-9-]*$), y solo esas escriben.
func TestCA242_PropiedadFormaDeSlug(t *testing.T) {
	root := initRepo(t)
	slug := regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	alfabeto := []rune("abz09-_A./ñ")
	rng := rand.New(rand.NewSource(242))
	for i := 0; i < 60; i++ {
		n := 1 + rng.Intn(6)
		var b strings.Builder
		for j := 0; j < n; j++ {
			b.WriteRune(alfabeto[rng.Intn(len(alfabeto))])
		}
		tarea := b.String()
		antes := abCuantos(t, root)
		f, err := Register(root, "main", Draft{Severity: "low", Lens: "readability", Description: "p", Task: tarea})
		valida := slug.MatchString(tarea)
		if valida != (err == nil) {
			t.Fatalf("CA-242: tarea %q: valida=%v pero err=%v", tarea, valida, err)
		}
		if valida {
			if f.Task != tarea || abArtefacto(t, root, f.ID)["task"] != tarea {
				t.Fatalf("CA-242: la tarea %q debe quedar tal cual: %+v", tarea, f)
			}
			continue
		}
		if abCuantos(t, root) != antes {
			t.Fatalf("CA-242: la tarea invalida %q escribio algo", tarea)
		}
	}
}
