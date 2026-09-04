// Tests adversariales del spec .hoom/specs/studio-v3-cockpit.md
// (CA-21, CA-24): deteccion honesta por PATH y normalizacion de eventos a
// un esquema unico con degradacion declarada.
package providers

import (
	"os"
	"path/filepath"
	"testing"
)

func fakeBin(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

// CA-21: las cuatro CLIs soportadas se reportan con installed segun el PATH
// REAL: presente = true, ausente = false.
func TestCA21_DeteccionPorPATH(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "claude")) // SOLO claude existe en este PATH
	got := map[string]bool{}
	for _, p := range Detect() {
		got[p.Name] = p.Installed
	}
	for _, name := range []string{"claude", "opencode", "codex", "gemini"} {
		if _, ok := got[name]; !ok {
			t.Fatalf("CA-21: falta el provider soportado %q", name)
		}
	}
	if !got["claude"] {
		t.Fatal("CA-21: claude esta en PATH y debe reportarse instalado")
	}
	if got["opencode"] || got["codex"] || got["gemini"] {
		t.Fatalf("CA-21: providers ausentes reportados como instalados: %v", got)
	}
}

// CA-24: el stream-json de claude se normaliza a {ts, kind, agent, detail}:
// texto, herramienta, delegacion a subagente; y cualquier linea no
// reconocida degrada a kind text sin perderse.
func TestCA24_NormalizacionClaude(t *testing.T) {
	spec, err := Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}

	evs := spec.Normalize(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"internal/gates/runner.go"}}]}}`)
	if len(evs) != 1 || evs[0].Kind != "tool" || evs[0].Detail != "Read: internal/gates/runner.go" {
		t.Fatalf("CA-24: tool_use mal normalizado: %+v", evs)
	}

	evs = spec.Normalize(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Task","input":{"subagent_type":"hoom-scout","prompt":"mapea el codigo"}}]}}`)
	if len(evs) != 1 || evs[0].Kind != "agent" || evs[0].Agent != "hoom-scout" {
		t.Fatalf("CA-24: la delegacion a subagente debe ser kind agent con el rol: %+v", evs)
	}

	evs = spec.Normalize("esto no es json y NO debe perderse")
	if len(evs) != 1 || evs[0].Kind != "text" || evs[0].Detail != "esto no es json y NO debe perderse" {
		t.Fatalf("CA-24: linea no reconocida debe degradar a text: %+v", evs)
	}
}

// CA-24: un provider sin salida estructurada emite todo como text — la
// degradacion es visible en el propio spec (Structured=false), nunca error.
func TestCA24_ProviderSinStreamDegradaAText(t *testing.T) {
	spec, err := Lookup("gemini")
	if err != nil {
		t.Fatal(err)
	}
	if spec.Capabilities().Structured {
		t.Fatal("CA-24: gemini no tiene stream estructurado declarado")
	}
	evs := spec.Normalize(`{"type":"assistant"}`)
	if len(evs) != 1 || evs[0].Kind != "text" {
		t.Fatalf("CA-24: sin parser, hasta el json va como text: %+v", evs)
	}
}
