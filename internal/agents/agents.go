// Package agents ships the 8 agent role contracts (orchestrator, architect,
// designer, scout, writer, adversarial test-writer, reviewer, characterizer)
// embedded in the binary, plus the AGENTS.md snippet that binds any coding
// agent (Claude Code, OpenCode, Codex, Ollama-based, ...) to the hoom
// verification contract. hoomAI never talks to a model: these files are the
// whole agent-facing interface.
package agents

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

//go:embed assets/agents/*.md
var assetsFS embed.FS

// Install writes the agent files into <dir>/.hoom/agents/ and appends the
// hoom contract to AGENTS.md (creating it if missing, skipping if already
// bound).
func Install(dir string) error {
	target := filepath.Join(dir, ".hoom", "agents")
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	entries, err := assetsFS.ReadDir("assets/agents")
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err := assetsFS.ReadFile("assets/agents/" + name)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(target, name), raw, 0o644); err != nil {
			return err
		}
		fmt.Printf("hoom: agente instalado .hoom/agents/%s\n", name)
	}
	return bindAgentsMD(dir)
}

const contractMarker = "<!-- hoomai:contract -->"

const contract = contractMarker + `
## Contrato hoomAI (verificacion obligatoria)

Este proyecto usa hoomAI. Reglas para CUALQUIER agente de codigo:

1. Antes de dar por terminado un cambio de codigo, ejecuta ` + "`hoom verify`" + ` y
   adjunta la ruta del veredicto (.hoom/verdicts/...) en tu respuesta final.
2. Veredicto ROJO = no se entrega. Corrige o reporta el bloqueo exacto.
3. Los roles y limites de cada agente estan en .hoom/agents/ y son obligatorios:
   un solo writer por tarea, test-writer NUNCA lee la implementacion,
   exploracion via scout, refactor legacy pasa por characterizer primero.
4. Nunca debilites, borres ni "ajustes" tests para hacer pasar un gate.
5. La narracion no cuenta: solo cuenta la evidencia del veredicto.
`

func bindAgentsMD(dir string) error {
	path := filepath.Join(dir, "AGENTS.md")
	raw, err := os.ReadFile(path)
	if err == nil && strings.Contains(string(raw), contractMarker) {
		fmt.Println("hoom: AGENTS.md ya contiene el contrato hoomAI")
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + contract); err != nil {
		return err
	}
	fmt.Println("hoom: contrato hoomAI agregado a AGENTS.md")
	return nil
}
