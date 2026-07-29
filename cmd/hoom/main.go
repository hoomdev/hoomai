// hoom is the hoomAI CLI: an AI- and stack-agnostic verification harness.
// Trust never lives in the agent; it lives in deterministic gates bound to
// Git-derived scope, recorded as append-only verdicts that travel with the
// project.
package main

import (
	"flag"
	"fmt"
	"os"
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

var version = "0.1.0"

const usage = `hoomAI %s - harness de verificacion agnostico de IA y de stack

Uso: hoom <comando> [flags]

Comandos:
  init        Detecta el stack y crea hoom.yaml + .hoom/ en el proyecto
  verify      Ejecuta los gates del manifiesto y emite un veredicto
  report      Muestra historial y tendencia de veredictos
  agents      Instala los 8 contratos de agentes en .hoom/agents/ y AGENTS.md
  profiles    Lista los perfiles de stack embebidos
  version     Muestra la version

Flags de init:
  --profile <nombre>   Fuerza un perfil (laravel|kmp|kmp-compose|go)
  --name <proyecto>    Nombre del proyecto (default: nombre del directorio)

Flags de verify:
  --full               Ignora el scoping por diff (corrida completa, ej. nocturna)
  --gate a,b           Ejecuta solo esos gates (el resto queda 'skipped')
  --spec <ruta>        Asocia el veredicto a un spec (.hoom/specs/...)

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
	report.Render(os.Stdout, v, path)
	if v.Verdict == "red" {
		os.Exit(1) // strict policy: red verdict = failing exit code
	}
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
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	return agents.Install(m.Dir)
}
