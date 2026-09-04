// Tests adversariales del spec .hoom/specs/codex-v2-y-review-cruzada.md
// (CA-158, CA-159): el meta de un run. El jsonl narra; el meta identifica.
package runcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func leerMeta(t *testing.T, root, id string) Meta {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "runs", id+".meta.json"))
	if err != nil {
		t.Fatalf("CA-158: falta el meta del run %s: %v", id, err)
	}
	var m Meta
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-158: el meta debe ser JSON legible: %v", err)
	}
	return m
}

// CA-158: el meta se escribe al arrancar y se completa al cerrar; un run que
// no cierra deja el meta en running; y sigue siendo telemetria local.
func TestCA158_MetaDelRun(t *testing.T) {
	installFake(t, `printf '{"type":"system","subtype":"init","session_id":"sess-9"}\n'`+"\nexit 0\n")
	root := t.TempDir()
	m := NewManager(root)

	run, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola", Role: "reviewer"})
	if err != nil {
		t.Fatal(err)
	}
	inicial := leerMeta(t, root, run.ID)
	if inicial.ID != run.ID || inicial.Provider != "claude" || inicial.Role != "reviewer" {
		t.Fatalf("CA-158: el meta identifica el run desde que arranca: %+v", inicial)
	}
	if inicial.Dir != root || inicial.CreatedAt.IsZero() {
		t.Fatalf("CA-158: el meta dice donde y cuando: %+v", inicial)
	}

	final := waitRun(t, m, run.ID)
	cerrado := leerMeta(t, root, run.ID)
	if cerrado.Status != StatusDone || cerrado.ExitCode != 0 {
		t.Fatalf("CA-158: al cerrar, el meta trae el estado y el exit: %+v", cerrado)
	}
	if cerrado.ProviderSessionID != final.ProviderSessionID || cerrado.ProviderSessionID == "" {
		t.Fatalf("CA-158: el meta se queda con la sesion del provider: %+v", cerrado)
	}
	if cerrado.EndedAt.IsZero() || cerrado.EndedAt.Before(cerrado.CreatedAt) {
		t.Fatalf("CA-158: y con cuando termino: %+v", cerrado)
	}

	// un run que todavia no cerro: el meta dice running con exit -1, que es la
	// verdad de lo que esta pasando y no un estado inventado al leerlo
	installFake(t, "sleep 30\n")
	m2 := NewManager(t.TempDir())
	corriendo, err := m2.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	enCurso := leerMeta(t, m2.root, corriendo.ID)
	if enCurso.Status != StatusRunning || enCurso.ExitCode != -1 {
		t.Fatalf("CA-158: mientras corre, el meta dice running: %+v", enCurso)
	}
	// cancelar solo PIDE el cierre: el cierre lo escribe la goroutine del run.
	// Hay que esperarlo, o el meta final se escribe sobre el TempDir que el
	// test ya esta borrando. Y el pedido se reintenta: en un run recien
	// lanzado el cancel puede no estar armado todavia.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if fin, err := m2.Get(corriendo.ID); err != nil || fin.Status != StatusRunning {
			break
		}
		m2.Cancel(corriendo.ID)
		time.Sleep(20 * time.Millisecond)
	}
	if fin, err := m2.Get(corriendo.ID); err != nil || fin.Status == StatusRunning {
		t.Fatalf("CA-158: el run cancelado tiene que cerrar: %+v (%v)", fin, err)
	}

	// telemetria local: vive junto a la narracion, que ya esta fuera de Git
	if _, err := os.Stat(filepath.Join(root, ".hoom", "runs", run.ID+".jsonl")); err != nil {
		t.Fatalf("CA-158: el meta acompana al jsonl, no lo reemplaza: %v", err)
	}
}

// CA-159: Metas lista del mas nuevo al mas viejo y se saltea lo que no puede
// leer: la telemetria rota nunca rompe un comando.
func TestCA159_MetasOrdenYBasura(t *testing.T) {
	root := t.TempDir()
	if got := Metas(root); len(got) != 0 {
		t.Fatalf("CA-159: sin directorio, lista vacia sin error: %v", got)
	}

	dir := filepath.Join(root, ".hoom", "runs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	escribir := func(id string, meta Meta) {
		raw, _ := json.MarshalIndent(meta, "", "  ")
		if err := os.WriteFile(filepath.Join(dir, id+".meta.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base := time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
	escribir("viejo", Meta{ID: "viejo", Provider: "claude", CreatedAt: base})
	escribir("nuevo", Meta{ID: "nuevo", Provider: "codex", CreatedAt: base.Add(time.Hour)})
	escribir("medio", Meta{ID: "medio", Provider: "claude", CreatedAt: base.Add(30 * time.Minute)})

	// basura que no debe romper nada: JSON invalido, otra forma, otro sufijo
	os.WriteFile(filepath.Join(dir, "roto.meta.json"), []byte("{esto no es json"), 0o644)
	os.WriteFile(filepath.Join(dir, "ajeno.meta.json"), []byte(`{"cosa":1}`), 0o644)
	os.WriteFile(filepath.Join(dir, "narracion.jsonl"), []byte("{}\n"), 0o644)

	got := Metas(root)
	if len(got) != 3 {
		t.Fatalf("CA-159: se saltea lo ilegible, lo de otra forma y lo que no es meta: %+v", got)
	}
	if got[0].ID != "nuevo" || got[1].ID != "medio" || got[2].ID != "viejo" {
		t.Fatalf("CA-159: del mas nuevo al mas viejo: %+v", got)
	}
}
