// hoom is the hoomAI CLI: an AI- and stack-agnostic verification harness.
// Trust never lives in the agent; it lives in deterministic gates bound to
// Git-derived scope, recorded as append-only verdicts that travel with the
// project.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/gates"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/initcmd"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/report"
	"github.com/hoomdev/hoomai/internal/verdict"
)

var version = "0.3.0"

const usage = `hoomAI %s - harness de verificacion agnostico de IA y de stack

Uso: hoom <comando> [flags]

Comandos:
  init        Detecta el stack y crea hoom.yaml + .hoom/ en el proyecto
  verify      Ejecuta los gates del manifiesto y emite un veredicto
  report      Muestra historial y tendencia de veredictos
  check       Compara el arbol actual contra el ultimo veredicto (huella + verde)
  hook        Instala el pre-push de Git que exige 'hoom check' antes de integrar
  agents      Instala los 9 contratos en .hoom/agents/ y AGENTS.md
              --target claude,opencode,codex,gemini|all genera ademas los
              subagentes NATIVOS de cada CLI desde los mismos contratos
  profiles    Lista los perfiles de stack embebidos
  version     Muestra la version

Flags de init:
  --profile <nombre>   Fuerza un perfil (laravel|kmp|kmp-compose|go)
  --name <proyecto>    Nombre del proyecto (default: nombre del directorio)

Flags de verify:
  --full               Ignora el scoping por diff (corrida completa, ej. nocturna)
  --gate a,b           Ejecuta solo esos gates (el resto queda 'skipped')
  --spec <ruta>        Asocia el veredicto a un spec (.hoom/specs/...)
  --json               Emite el veredicto como JSON en stdout (para agentes)

Flags de report:
  -n <cantidad>        Cantidad de veredictos recientes a mostrar (default 10)

Filosofia: veredicto ROJO = exit code 1. La narracion del agente no cuenta;
solo cuenta la evidencia. Gates ausentes se declaran en amarillo, jamas se ocultan.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	cmd, args := os.Args[1], os.Args[2:]
	var err error
	switch cmd {
	case "init":
		err = cmdInit(args)
	case "verify":
		err = cmdVerify(args)
	case "report":
		err = cmdReport(args)
	case "check":
		err = cmdCheck(args)
	case "hook":
		err = cmdHook(args)
	case "agents":
		err = cmdAgents(args)
	case "profiles":
		for _, line := range profiles.Describe() {
			fmt.Println(line)
		}
	case "version":
		fmt.Println("hoom", version)
	case "help", "-h", "--help":
		fmt.Printf(usage, version)
	default:
		fmt.Fprintf(os.Stderr, "hoom: comando desconocido %q\n\n", cmd)
		fmt.Printf(usage, version)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "hoom:", err)
		os.Exit(1)
	}
}

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	profileName := fs.String("profile", "", "perfil a usar (omite deteccion)")
	name := fs.String("name", "", "nombre del proyecto")
	_ = fs.Parse(args)
	dir := "."
	if fs.NArg() > 0 {
		dir = fs.Arg(0)
	}
	return initcmd.Run(dir, *name, *profileName)
}

func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	full := fs.Bool("full", false, "ignorar scoping por diff")
	gateList := fs.String("gate", "", "gates a ejecutar, separados por coma")
	spec := fs.String("spec", "", "ruta del spec asociado")
	asJSON := fs.Bool("json", false, "emitir veredicto como JSON en stdout")
	_ = fs.Parse(args)

	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	git := gitx.Snapshot(m.Dir, m.BaseBranch)

	opt := gates.Options{Full: *full}
	if *gateList != "" {
		opt.Only = map[string]bool{}
		for _, g := range strings.Split(*gateList, ",") {
			opt.Only[strings.TrimSpace(g)] = true
		}
	}

	fmt.Printf("hoom: verificando %s (perfil %s, %d gates)...\n", m.Project, m.Profile, len(m.Gates))
	results := gates.Run(m, git, opt)

	v := &verdict.Verdict{
		Project: m.Project,
		Profile: m.Profile,
		Policy:  m.Policy,
		Git:     git,
		Gates:   results,
		Spec:    *spec,
	}
	v.Finalize()
	path, err := verdict.Write(m.Dir, v)
	if err != nil {
		return fmt.Errorf("no se pudo escribir el veredicto: %w", err)
	}
	if *asJSON {
		// In-band, machine-readable output for agents: no ANSI, pure JSON.
		raw, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(raw))
	} else {
		report.Render(os.Stdout, v, path)
	}
	if v.Verdict == "red" {
		os.Exit(1) // strict policy: red verdict = failing exit code
	}
	return nil
}

// cmdCheck is hoomAI's answer to RDD receipts without the ceremony: it
// deterministically proves that the CURRENT working tree is exactly what the
// latest verdict verified (fingerprint match) and that said verdict is green.
// Failure messages are in-band: they state the exact command to run next.
func cmdCheck(args []string) error {
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	git := gitx.Snapshot(m.Dir, m.BaseBranch)
	all, err := verdict.LoadAll(m.Dir)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		fmt.Fprintln(os.Stderr, "hoom check: ROJO - no existe ningun veredicto. Accion: ejecuta 'hoom verify'.")
		os.Exit(1)
	}
	last := all[len(all)-1]
	if last.Verdict != "green" {
		fmt.Fprintf(os.Stderr, "hoom check: ROJO - el ultimo veredicto (%s) es rojo. Accion: corrige y ejecuta 'hoom verify'.\n", last.ID)
		os.Exit(1)
	}
	if last.Git.ChangeFingerprint != git.ChangeFingerprint {
		fmt.Fprintf(os.Stderr, "hoom check: ROJO - el arbol cambio despues del ultimo veredicto verde (%s).\n", last.ID)
		fmt.Fprintf(os.Stderr, "  huella del veredicto: %s\n  huella actual:        %s\n", last.Git.ChangeFingerprint, git.ChangeFingerprint)
		fmt.Fprintln(os.Stderr, "  Accion: ejecuta 'hoom verify' de nuevo sobre el estado actual.")
		os.Exit(1)
	}
	fmt.Printf("hoom check: VERDE - el arbol actual coincide con el veredicto %s (huella %s)\n", last.ID, git.ChangeFingerprint)
	return nil
}

const prePushHook = `#!/bin/sh
# hoomAI pre-push gate - sin veredicto verde con huella coincidente, no hay push.
# Escape consciente y visible: HOOM_SKIP=1 git push
[ -n "$HOOM_SKIP" ] && { echo "hoom: gate saltado por HOOM_SKIP=1" >&2; exit 0; }
exec hoom check
`

// cmdHook installs the Git pre-push hook that enforces `hoom check` before
// any integration: RDD's gate, evidence-bound, no cryptography.
func cmdHook(args []string) error {
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	hooksDir := filepath.Join(m.Dir, ".git", "hooks")
	if _, err := os.Stat(filepath.Join(m.Dir, ".git")); err != nil {
		return fmt.Errorf("no es un repositorio git: %s", m.Dir)
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(hooksDir, "pre-push")
	if raw, err := os.ReadFile(target); err == nil && !strings.Contains(string(raw), "hoomAI pre-push gate") {
		return fmt.Errorf("ya existe un pre-push ajeno en %s; integralo a mano agregando 'hoom check'", target)
	}
	if err := os.WriteFile(target, []byte(prePushHook), 0o755); err != nil {
		return err
	}
	fmt.Printf("hoom: pre-push instalado en %s\n", target)
	fmt.Println("hoom: cada 'git push' exigira veredicto verde con huella coincidente (HOOM_SKIP=1 para saltarlo, queda a la vista)")
	return nil
}

func cmdReport(args []string) error {
	fs := flag.NewFlagSet("report", flag.ExitOnError)
	n := fs.Int("n", 10, "cantidad de veredictos recientes")
	_ = fs.Parse(args)
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	all, err := verdict.LoadAll(m.Dir)
	if err != nil {
		return err
	}
	report.History(os.Stdout, all, *n)
	return nil
}

func cmdAgents(args []string) error {
	fs := flag.NewFlagSet("agents", flag.ExitOnError)
	target := fs.String("target", "", "genera subagentes nativos: claude,opencode,codex,gemini o all")
	_ = fs.Parse(args)
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	if err := agents.Install(m.Dir); err != nil {
		return err
	}
	if *target == "" {
		return nil
	}
	var list []string
	if *target == "all" {
		list = agents.ValidTargets
	} else {
		for _, t := range strings.Split(*target, ",") {
			list = append(list, strings.TrimSpace(t))
		}
	}
	return agents.GenerateTargets(m.Dir, list)
}
