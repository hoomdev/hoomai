// Package agents: target generators. The contracts in .hoom/agents/ are the
// single source of truth; each generator translates them into the native
// subagent format of a coding CLI. Where the tool supports it, the contract's
// discipline becomes HARD enforcement (read-only tool permissions): "the
// scout never edits" stops being a prompt rule and becomes a technical
// impossibility.
package agents

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// role carries the per-agent metadata each target format needs.
type role struct {
	file     string // embedded filename
	slug     string // native agent name (hoom-*)
	desc     string // delegation description shown to the orchestrator
	readOnly bool   // enforce no-edit permissions where the target supports it
	exec     bool   // read-only rol que ademas EJECUTA comandos (hoom finding, tests)
	primary  bool   // orchestrator: the main session in most CLIs
}

var roles = []role{
	{"00-orquestador.md", "hoom-orquestador",
		"Agente principal hoomAI: rutea el trabajo, delega a los subagentes y exige hoom verify + hoom check antes de entregar. NUNCA edita codigo.",
		true, true, true},
	{"01-arquitecto.md", "hoom-arquitecto",
		"Produce el spec de una tarea en .hoom/specs (7 secciones) a partir de la vision y el pedido. Usar ANTES de implementar cualquier cambio sustancial. Solo lectura.",
		true, false, false},
	{"02-designer.md", "hoom-designer",
		"Traduce el visual elegido a un UI-spec y protege el design system. Usar en tareas con interfaz de usuario. Solo lectura.",
		true, false, false},
	{"03-scout.md", "hoom-scout",
		"Explora el codigo y devuelve un resumen comprimido con rutas exactas y firmas. Usar cuando entender un flujo requiere leer 4 o mas archivos. Solo lectura.",
		true, false, false},
	{"04-writer.md", "hoom-writer",
		"UNICO agente que edita codigo. Implementa exactamente el scope del spec y corre hoom verify al terminar. Uno solo por tarea.",
		false, false, false},
	{"05-test-writer.md", "hoom-test-writer",
		"Escribe tests adversariales SOLO desde el spec, sin ver jamas la implementacion. Usar tras aprobar el spec, antes o en paralelo del writer.",
		false, false, false},
	{"06-reviewer.md", "hoom-reviewer",
		"Revisa un diff con la lente asignada (readability, reliability, resilience o risk); las 4 lentes si toca seguridad, dinero o supera 400 lineas. Solo lectura de codigo; registra hallazgos con hoom finding add.",
		true, true, false},
	{"07-characterizer.md", "hoom-characterizer",
		"Genera characterization tests que fijan el comportamiento ACTUAL de codigo legacy antes de refactorizar.",
		false, false, false},
	{"08-analista.md", "hoom-analista",
		"Convierte los documentos del cliente en .hoom/intake en la vision (.hoom/specs/00-vision.md) y el backlog (.hoom/specs/backlog.md).",
		false, false, false},
	{"09-refutador.md", "hoom-refutador",
		"Intenta REFUTAR los hallazgos abiertos con evidencia deterministica (correr el test, citar la linea) antes de que se corrijan; maximo 2 ciclos y escala al humano. Solo lectura de codigo; cierra con hoom finding resolve.",
		true, true, false},
}

// ValidTargets lists the supported --target values.
var ValidTargets = []string{"claude", "opencode", "codex", "gemini"}

// GenerateTargets writes native agent definitions for each requested target.
func GenerateTargets(dir string, targets []string) error {
	for _, t := range targets {
		var err error
		switch t {
		case "claude":
			err = genClaude(dir)
		case "opencode":
			err = genOpenCode(dir)
		case "codex":
			err = genCodex(dir)
		case "gemini":
			err = genGemini(dir)
		default:
			return fmt.Errorf("target desconocido %q (validos: %s, o 'all')", t, strings.Join(ValidTargets, ", "))
		}
		if err != nil {
			return fmt.Errorf("target %s: %w", t, err)
		}
	}
	return nil
}

func body(r role) (string, error) {
	raw, err := assetsFS.ReadFile("assets/agents/" + r.file)
	return string(raw), err
}

// --- Claude Code: .claude/agents/<name>.md ------------------------------
// Frontmatter: name, description, tools (comma-separated; omitted = all).
// The main session IS the orchestrator, so role 00 is not generated; its
// rules already bind via AGENTS.md.
func genClaude(dir string) error {
	out := filepath.Join(dir, ".claude", "agents")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, r := range roles {
		if r.primary {
			continue
		}
		b, err := body(r)
		if err != nil {
			return err
		}
		var f strings.Builder
		f.WriteString("---\n")
		fmt.Fprintf(&f, "name: %s\n", r.slug)
		fmt.Fprintf(&f, "description: %q\n", r.desc)
		if r.readOnly && r.exec {
			// solo lectura de CODIGO pero ejecuta comandos (hoom finding, tests)
			f.WriteString("tools: Read, Grep, Glob, Bash\n")
		} else if r.readOnly {
			f.WriteString("tools: Read, Grep, Glob\n")
		}
		f.WriteString("---\n\n")
		f.WriteString(b)
		if err := os.WriteFile(filepath.Join(out, r.slug+".md"), []byte(f.String()), 0o644); err != nil {
			return err
		}
	}
	fmt.Println("hoom: [claude] subagentes en .claude/agents/ (9; el orquestador es tu sesion principal via AGENTS.md)")
	fmt.Println("hoom: [claude] los roles de solo lectura quedan SIN herramientas de edicion: enforcement duro")
	return nil
}

// --- OpenCode: .opencode/agents/<name>.md --------------------------------
// Frontmatter: description, mode (primary|subagent), permission map.
// The orchestrator becomes a PRIMARY agent that cannot edit: the contract's
// core rule, enforced by the tool itself.
func genOpenCode(dir string) error {
	out := filepath.Join(dir, ".opencode", "agents")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, r := range roles {
		b, err := body(r)
		if err != nil {
			return err
		}
		mode := "subagent"
		if r.primary {
			mode = "primary"
		}
		var f strings.Builder
		f.WriteString("---\n")
		fmt.Fprintf(&f, "description: %q\n", r.desc)
		fmt.Fprintf(&f, "mode: %s\n", mode)
		f.WriteString("permission:\n")
		if r.readOnly {
			f.WriteString("  edit: deny\n")
			if r.primary || r.exec {
				f.WriteString("  bash: allow\n") // corre hoom verify/check/finding y tests
			} else {
				f.WriteString("  bash: deny\n")
			}
		} else {
			f.WriteString("  edit: allow\n")
			f.WriteString("  bash: allow\n")
		}
		f.WriteString("---\n\n")
		f.WriteString(b)
		if err := os.WriteFile(filepath.Join(out, r.slug+".md"), []byte(f.String()), 0o644); err != nil {
			return err
		}
	}
	fmt.Println("hoom: [opencode] agentes en .opencode/agents/ (10; hoom-orquestador es PRIMARY con edit:deny)")
	fmt.Println("hoom: [opencode] seleccionalo con Tab o invoca subagentes con @hoom-<rol>")
	return nil
}

// --- Codex CLI: .codex/agents/<name>.toml --------------------------------
// One TOML per agent: name, description, developer_instructions; read-only
// roles override the sandbox. The root session is the orchestrator.
func genCodex(dir string) error {
	out := filepath.Join(dir, ".codex", "agents")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, r := range roles {
		if r.primary {
			continue
		}
		b, err := body(r)
		if err != nil {
			return err
		}
		var f strings.Builder
		fmt.Fprintf(&f, "name = %q\n", r.slug)
		fmt.Fprintf(&f, "description = %q\n", r.desc)
		if r.readOnly && !r.exec {
			f.WriteString("sandbox_mode = \"read-only\"\n")
		} else if r.readOnly && r.exec {
			// necesita ejecutar hoom finding (escribe .hoom/findings/); la
			// disciplina de no editar codigo queda en el contrato
			f.WriteString("sandbox_mode = \"workspace-write\"\n")
		}
		f.WriteString("developer_instructions = \"\"\"\n")
		f.WriteString(strings.ReplaceAll(b, `"""`, `'''`))
		f.WriteString("\"\"\"\n")
		if err := os.WriteFile(filepath.Join(out, r.slug+".toml"), []byte(f.String()), 0o644); err != nil {
			return err
		}
	}
	fmt.Println("hoom: [codex] subagentes en .codex/agents/ (9; el orquestador es tu sesion raiz via AGENTS.md)")
	fmt.Println("hoom: [codex] recuerda habilitar multi-agente: [features] multi_agent = true en ~/.codex/config.toml")
	return nil
}

// --- Gemini CLI: .gemini/agents/<name>.md --------------------------------
// Frontmatter: name, description, tools (YAML list), model: inherit.
// NOTE: Antigravity CLI does not load these yet; this target serves Gemini
// CLI (enterprise/Code Assist). An antigravity target will be added when
// Google documents its format.
func genGemini(dir string) error {
	out := filepath.Join(dir, ".gemini", "agents")
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	for _, r := range roles {
		if r.primary {
			continue
		}
		b, err := body(r)
		if err != nil {
			return err
		}
		var f strings.Builder
		f.WriteString("---\n")
		fmt.Fprintf(&f, "name: %s\n", r.slug)
		fmt.Fprintf(&f, "description: %q\n", r.desc)
		f.WriteString("model: inherit\n")
		f.WriteString("tools:\n")
		if r.readOnly {
			for _, t := range []string{"read_file", "read_many_files", "glob", "search_file_content"} {
				fmt.Fprintf(&f, "  - %s\n", t)
			}
			if r.exec {
				f.WriteString("  - run_shell_command\n")
			}
		} else {
			f.WriteString("  - \"*\"\n")
		}
		f.WriteString("---\n\n")
		f.WriteString(b)
		if err := os.WriteFile(filepath.Join(out, r.slug+".md"), []byte(f.String()), 0o644); err != nil {
			return err
		}
	}
	fmt.Println("hoom: [gemini] subagentes en .gemini/agents/ (9; el orquestador es tu sesion principal)")
	fmt.Println("hoom: [gemini] NOTA: Antigravity CLI aun no carga subagentes; este target sirve a Gemini CLI (Code Assist)")
	return nil
}
