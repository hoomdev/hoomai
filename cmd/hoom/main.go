// hoom is the hoomAI CLI: an AI- and stack-agnostic verification harness.
// Trust never lives in the agent; it lives in deterministic gates bound to
// Git-derived scope, recorded as append-only verdicts that travel with the
// project.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/checkcmd"
	"github.com/hoomdev/hoomai/internal/cockpitcmd"
	"github.com/hoomdev/hoomai/internal/contextcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/initcmd"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/profiles"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/ratchet"
	"github.com/hoomdev/hoomai/internal/report"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/servecmd"
	"github.com/hoomdev/hoomai/internal/statuscmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
	"github.com/hoomdev/hoomai/internal/verifycmd"
)

var version = "0.9.0"

const usage = `hoomAI %s - harness de verificacion agnostico de IA y de stack

Uso: hoom <comando> [flags]

Comandos:
  init        Detecta el stack y crea hoom.yaml + .hoom/ en el proyecto
  verify      Ejecuta los gates del manifiesto y emite un veredicto
  report      Muestra historial y tendencia de veredictos
  check       Compara el arbol actual contra el ultimo veredicto (huella + verde)
  status      La ventana del arbitro: check, veredicto, verify en curso, runs
              con su rol, tareas y hallazgos [--json | --watch]
  cockpit     Arma el puesto completo sobre tmux/zellij: tu CLI de IA en un
              pane real + status --watch al lado [--provider p] [--task slug]
  ratchet     El trinquete: calidad que solo puede subir. init crea la linea
              base (.hoom/ratchet.json); 'verify --full' la compara y aprieta;
              lower <metrica> --to <v> --reason "..." la afloja CON registro
  serve       HoomAI Studio: dashboard local embebido en el binario (lectura + acciones con token)
  task        Tareas paralelas aisladas en worktrees: start <slug> | list | done <slug>
  spec        Aprobacion humana atada al contenido: approve <ruta> | status <ruta>
  context     Salud del contexto: intake, vision/backlog, preguntas abiertas,
              staleness. Amarillos honestos; informa, nunca bloquea [--json]
  finding     Hallazgos de review como artefactos append-only (.hoom/findings/):
              add --sev low|medium|high [--lens l] [--file f] "<descripcion>"
              resolve <id> --as corregido|refutado --evidence "<por que>"
              list [--open] [--json]   (cerrar SIN evidencia esta prohibido)
  providers   Detecta las CLIs de IA instaladas y las capacidades que declara
              cada una (claude|opencode|codex|gemini) [--json]
  run         Lanza tu CLI de IA en headless: --provider <p> [--task <slug>] "<prompt>"
              hoom NUNCA llama a una API de IA: ejecuta TU CLI como subproceso.
              Narracion en .hoom/runs/ (local, fuera de la huella y de Git).
              Opciones de sesion, modelo, system prompt, tools y topes abajo.
  hook        Instala el pre-push de Git que exige 'hoom check' antes de integrar
  agents      Instala los 10 contratos en .hoom/agents/ y AGENTS.md
              --target claude,opencode,codex,gemini|all genera ademas los
              subagentes NATIVOS de cada CLI desde los mismos contratos
  profiles    Lista los perfiles de stack embebidos
  version     Muestra la version

Flags de init:
  --profile <nombre>   Fuerza un perfil (laravel|kmp|kmp-compose|go)
  --name <proyecto>    Nombre del proyecto (default: nombre del directorio)

Flags de verify:
  --full               Ignora el scoping por diff (corrida completa, ej. nocturna)
  --gate a,b           Ejecuta solo esos gates (el resto queda 'skipped'); el
                       veredicto queda PARCIAL: diagnostico, nunca referencia
                       de 'hoom check' ni de 'hoom task done'
  --spec <ruta>        Asocia el veredicto a un spec y ejecuta los gates
                       spec_lint y spec_trace (trazabilidad CA-n -> tests)
  --json               Emite el veredicto como JSON en stdout (para agentes)

Flags de report:
  -n <cantidad>        Cantidad de veredictos recientes a mostrar (default 10)
  --json               Emite el historial como JSON en stdout

Flags de check:
  --json               Emite el resultado como JSON en stdout (mismo exit code)

Flags de status:
  --json               Emite el snapshot como JSON en stdout
  --watch              Refresca en vivo (requiere TTY; sin TTY imprime una vez)

Flags de cockpit:
  --provider <p>       CLI de IA a lanzar (claude|opencode|codex|gemini);
                       omitido: se usa la unica instalada, jamas se adivina
  --task <slug>        Monta el cockpit dentro del worktree de esa tarea
  --mux tmux|zellij    Fuerza el multiplexor (default: tmux, luego zellij)

Flags de run (lo que el provider no soporta se ignora CON aviso en el log;
              --strict lo vuelve error antes de crear el run):
  --resume <id>        Reanuda esa sesion del provider (la imprime un run anterior)
  --model <m>          Modelo, en el vocabulario del provider (ej: sonnet)
  --system-prompt <t>  Texto que se AGREGA al system prompt del provider;
                       @ruta lee un archivo (ej: @.hoom/agents/04-writer.md)
  --allow-tools a,b    Herramientas permitidas (vocabulario del provider)
  --deny-tools a,b     Herramientas prohibidas
  --max-turns n        Tope de turnos del agente (0 = sin tope)
  --budget-usd x       Tope de gasto en USD (0 = sin tope)
  --strict             Campo no soportado = error, no aviso

Flags de serve:
  --addr host:puerto   Direccion de escucha (default 127.0.0.1:4666, solo loopback)

'hoom task list --json' emite el estado de las tareas como JSON en stdout.

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
	case "status":
		err = cmdStatus(args)
	case "cockpit":
		err = cmdCockpit(args)
	case "ratchet":
		err = cmdRatchet(args)
	case "serve":
		err = cmdServe(args)
	case "task":
		err = cmdTask(args)
	case "spec":
		err = cmdSpec(args)
	case "context":
		err = cmdContext(args)
	case "finding":
		err = cmdFinding(args)
	case "providers":
		err = cmdProviders(args)
	case "run":
		err = cmdRun(args)
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
	specPath := fs.String("spec", "", "ruta del spec asociado")
	asJSON := fs.Bool("json", false, "emitir veredicto como JSON en stdout")
	_ = fs.Parse(args)

	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	opt := verifycmd.Options{Full: *full, Spec: *specPath}
	if *gateList != "" {
		opt.Gates = strings.Split(*gateList, ",")
	}
	fmt.Printf("hoom: verificando %s (perfil %s, %d gates)...\n", m.Project, m.Profile, len(m.Gates))
	v, path, err := verifycmd.Run(m, opt)
	if err != nil {
		return err
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
	fs := flag.NewFlagSet("check", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emitir el resultado del check como JSON en stdout")
	_ = fs.Parse(args)
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	res, err := checkcmd.Run(m.Dir, m.BaseBranch)
	if err != nil {
		return err
	}
	if *asJSON {
		raw, _ := json.MarshalIndent(res, "", "  ")
		fmt.Println(string(raw))
		if !res.OK {
			os.Exit(1) // CA-3: mismo exit code que el modo texto
		}
		return nil
	}
	switch res.Reason {
	case checkcmd.ReasonNoVerdict:
		fmt.Fprintln(os.Stderr, "hoom check: ROJO - no existe ningun veredicto. Accion: ejecuta 'hoom verify'.")
	case checkcmd.ReasonOnlyPartial:
		fmt.Fprintln(os.Stderr, "hoom check: ROJO - solo hay veredictos PARCIALES (--gate), que no son referencia. Accion: ejecuta 'hoom verify' completo.")
	case checkcmd.ReasonRedVerdict:
		fmt.Fprintf(os.Stderr, "hoom check: ROJO - el ultimo veredicto (%s) es rojo. Accion: corrige y ejecuta 'hoom verify'.\n", res.VerdictID)
	case checkcmd.ReasonDrift:
		fmt.Fprintf(os.Stderr, "hoom check: ROJO - el arbol cambio despues del ultimo veredicto verde (%s).\n", res.VerdictID)
		fmt.Fprintf(os.Stderr, "  huella del veredicto: %s\n  huella actual:        %s\n", res.FingerprintVerdict, res.FingerprintNow)
		fmt.Fprintln(os.Stderr, "  Accion: ejecuta 'hoom verify' de nuevo sobre el estado actual.")
	default:
		fmt.Printf("hoom check: VERDE - el arbol actual coincide con el veredicto %s (huella %s)\n", res.VerdictID, res.FingerprintNow)
	}
	if !res.OK {
		os.Exit(1)
	}
	return nil
}

// cmdStatus renders the arbiter's window: read-only, never blocks, exit 0.
// The live discipline stays with check/verify; status only shows.
func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "emitir el snapshot como JSON en stdout")
	watch := fs.Bool("watch", false, "refrescar en vivo (requiere TTY)")
	_ = fs.Parse(args)
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	tty := false
	if fi, err := os.Stdout.Stat(); err == nil {
		tty = fi.Mode()&os.ModeCharDevice != 0
	}
	return statuscmd.Run(m.Dir, m.BaseBranch, os.Stdout, statuscmd.Options{
		JSON: *asJSON, Watch: *watch, TTY: tty,
	})
}

// cmdRatchet manages the quality baseline: scaffold it, or loosen one
// metric with a mandatory, recorded reason. Tightening has no verb on
// purpose: only a measurement during verify --full moves a baseline up.
func cmdRatchet(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: hoom ratchet init | lower <metrica> --to <valor> --reason \"<por que>\"")
	}
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "init":
		p, err := ratchet.Init(m.Dir)
		if err != nil {
			return err
		}
		fmt.Printf("hoom ratchet: creado %s (viaja en Git)\n", p)
		fmt.Println("  declara metricas como comandos cuya ULTIMA linea de salida sea un numero:")
		fmt.Println(`    "metrics": {`)
		fmt.Println(`      "cobertura": {"cmd": "go test -cover ./... | ...", "direction": "up", "tolerance": 0.5}`)
		fmt.Println(`    }`)
		fmt.Println("  'hoom verify --full' congela la base en la primera corrida y desde ahi: solo sube.")
		return nil
	case "lower":
		if len(rest) < 1 || strings.HasPrefix(rest[0], "-") {
			return fmt.Errorf("uso: hoom ratchet lower <metrica> --to <valor> --reason \"<por que>\"")
		}
		name := rest[0]
		fs := flag.NewFlagSet("ratchet lower", flag.ExitOnError)
		to := fs.Float64("to", math.NaN(), "nueva base (peor que la actual)")
		reason := fs.String("reason", "", "razon del aflojamiento (obligatoria)")
		_ = fs.Parse(rest[1:])
		if math.IsNaN(*to) {
			return fmt.Errorf("falta --to <valor>")
		}
		ch, err := ratchet.Lower(m.Dir, name, *to, *reason)
		if err != nil {
			return err
		}
		fmt.Printf("hoom ratchet: %s aflojada %v -> %v\n  razon: %s\n  registro: history de .hoom/ratchet.json (queda en el diff de Git)\n",
			ch.Metric, *ch.From, ch.To, ch.Reason)
		return nil
	default:
		return fmt.Errorf("subcomando desconocido %q (init|lower)", sub)
	}
}

// cmdCockpit assembles the tmux/zellij layout: AI CLI + status --watch.
func cmdCockpit(args []string) error {
	fs := flag.NewFlagSet("cockpit", flag.ExitOnError)
	provider := fs.String("provider", "", "CLI de IA a lanzar (claude|opencode|codex|gemini)")
	task := fs.String("task", "", "slug de tarea: monta el cockpit en su worktree")
	mux := fs.String("mux", "", "multiplexor a usar (tmux|zellij)")
	_ = fs.Parse(args)
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	return cockpitcmd.Run(m.Dir, m.Project, cockpitcmd.Options{
		Provider: *provider, Task: *task, Mux: *mux,
	}, cockpitcmd.DefaultDeps())
}

// cmdContext reports context health. Always exit 0: context debt is
// visible, never blocking (there is no red here by design).
func cmdContext(args []string) error {
	asJSON := false
	for _, a := range args {
		if a == "--json" || a == "-json" {
			asJSON = true
		}
	}
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	if asJSON {
		raw, err := contextcmd.JSONBytes(m.Dir)
		if err != nil {
			return err
		}
		fmt.Println(string(raw))
		return nil
	}
	contextcmd.Render(os.Stdout, contextcmd.Build(m.Dir))
	return nil
}

func cmdProviders(args []string) error {
	asJSON := false
	for _, a := range args {
		if a == "--json" || a == "-json" {
			asJSON = true
		}
	}
	if asJSON {
		raw, err := providers.JSONBytes()
		if err != nil {
			return err
		}
		fmt.Println(string(raw))
		return nil
	}
	providers.RenderText(os.Stdout, providers.Detect())
	return nil
}

// cmdRun launches the user's own AI CLI headless over the project or a task
// worktree. hoom never talks to a model API — it executes the CLI the user
// already has, exactly like typing it in a terminal.
func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	provider := fs.String("provider", "", "provider de IA (claude|opencode|codex|gemini)")
	task := fs.String("task", "", "slug de la tarea: corre en su worktree aislado")
	resume := fs.String("resume", "", "id de sesion del provider a reanudar (lo imprime un run anterior)")
	model := fs.String("model", "", "modelo, en el vocabulario del provider (ej: sonnet)")
	sysPrompt := fs.String("system-prompt", "", "texto que se AGREGA al system prompt del provider; @ruta lee un archivo")
	allow := fs.String("allow-tools", "", "herramientas permitidas, separadas por coma (vocabulario del provider)")
	deny := fs.String("deny-tools", "", "herramientas prohibidas, separadas por coma")
	maxTurns := fs.Int("max-turns", 0, "tope de turnos del agente (0 = sin tope)")
	budget := fs.Float64("budget-usd", 0, "tope de gasto en USD (0 = sin tope)")
	strict := fs.Bool("strict", false, "un campo que el provider no soporta es error, no aviso")
	_ = fs.Parse(args)
	if *provider == "" {
		return fmt.Errorf("falta --provider (mira 'hoom providers')")
	}
	prompt := strings.TrimSpace(strings.Join(fs.Args(), " "))
	if prompt == "" {
		return fmt.Errorf("falta el prompt: hoom run --provider %s \"<pedido>\"", *provider)
	}
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	sp, err := runcmd.ResolveSystemPrompt(*sysPrompt)
	if err != nil {
		return err
	}
	mgr := runcmd.NewManager(m.Dir)
	info, err := mgr.Start(runcmd.StartOptions{
		Provider: *provider, Prompt: prompt, Task: *task, ResumeID: *resume,
		Model: *model, SystemPrompt: sp,
		AllowTools: splitList(*allow), DenyTools: splitList(*deny),
		MaxTurns: *maxTurns, BudgetUSD: *budget, Strict: *strict,
	})
	if err != nil {
		return err
	}
	fmt.Printf("hoom run: %s (%s) - narracion en .hoom/runs/%s.jsonl (local, fuera de la huella)\n", info.ID, *provider, info.ID)
	// stream the narration to the terminal as it happens
	seen := 0
	for {
		st, evs, err := mgr.Events(info.ID, seen)
		if err != nil {
			return err
		}
		for _, ev := range evs {
			agent := ""
			if ev.Agent != "" {
				agent = "[" + ev.Agent + "] "
			}
			fmt.Printf("  %-5s %s%s\n", ev.Kind, agent, ev.Detail)
		}
		seen += len(evs)
		if st.Status != runcmd.StatusRunning {
			if st.ProviderSessionID != "" {
				// la sesion del provider es el handle para retomar EXACTAMENTE este hilo
				fmt.Printf("hoom run: sesion del provider %s\n  reanudar: hoom run --provider %s --resume %s \"<prompt>\"\n",
					st.ProviderSessionID, *provider, st.ProviderSessionID)
			}
			if st.Status != runcmd.StatusDone {
				fmt.Printf("hoom run: %s (exit %d)\n", st.Status, st.ExitCode)
			}
			if st.ExitCode > 0 {
				os.Exit(st.ExitCode) // CA-22: el exit del provider se propaga
			}
			if st.Status != runcmd.StatusDone {
				os.Exit(1)
			}
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
}

// splitList parses a comma-separated flag value; empty entries are dropped.
func splitList(raw string) []string {
	var out []string
	for _, s := range strings.Split(raw, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// cmdFinding manages review findings as append-only artifacts: immutable
// record + single terminal resolution with mandatory evidence.
func cmdFinding(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: hoom finding add|resolve|list (mira 'hoom help')")
	}
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "add":
		fs := flag.NewFlagSet("finding add", flag.ExitOnError)
		sev := fs.String("sev", "", "severidad: low|medium|high")
		lens := fs.String("lens", "", "lente: readability|reliability|resilience|risk|otra")
		file := fs.String("file", "", "archivo senalado")
		author := fs.String("author", "", "rol o autor (default: git config)")
		_ = fs.Parse(rest)
		desc := strings.TrimSpace(strings.Join(fs.Args(), " "))
		f, err := finding.Add(m.Dir, m.BaseBranch, *sev, *lens, *file, desc, *author)
		if err != nil {
			return err
		}
		fmt.Printf("hoom finding: registrado %s (%s/%s)\n  descripcion: %s\n  huella: %s\n  el registro es INMUTABLE; se cierra con 'hoom finding resolve %s --as corregido|refutado --evidence \"...\"'\n",
			f.ID, f.Severity, f.Lens, f.Description, f.Fingerprint, f.ID)
		return nil
	case "resolve":
		// el id va primero y flag.Parse corta en el primer posicional:
		// separarlo a mano antes de parsear los flags
		if len(rest) < 1 || strings.HasPrefix(rest[0], "-") {
			return fmt.Errorf("uso: hoom finding resolve <id> --as corregido|refutado --evidence \"...\"")
		}
		id := rest[0]
		fs := flag.NewFlagSet("finding resolve", flag.ExitOnError)
		as := fs.String("as", "", "corregido|refutado")
		evidence := fs.String("evidence", "", "evidencia del cierre (obligatoria)")
		author := fs.String("author", "", "rol o autor (default: git config)")
		_ = fs.Parse(rest[1:])
		r, err := finding.Resolve(m.Dir, id, *as, *evidence, *author)
		if err != nil {
			return err
		}
		fmt.Printf("hoom finding: %s -> %s\n  evidencia: %s\n", r.FindingID, strings.ToUpper(r.As), r.Evidence)
		return nil
	case "list":
		openOnly, asJSON := false, false
		for _, a := range rest {
			switch a {
			case "--open", "-open":
				openOnly = true
			case "--json", "-json":
				asJSON = true
			}
		}
		if asJSON {
			raw, err := finding.JSONBytes(m.Dir, m.BaseBranch, openOnly)
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			return nil
		}
		items, warnings, err := finding.List(m.Dir, m.BaseBranch, openOnly)
		if err != nil {
			return err
		}
		for _, w := range warnings {
			fmt.Fprintln(os.Stderr, "hoom:", w)
		}
		if len(items) == 0 {
			fmt.Println("hoom: sin hallazgos registrados (el reviewer los crea con 'hoom finding add')")
			return nil
		}
		fmt.Println("hoom: hallazgos")
		for _, it := range items {
			changed := ""
			if it.CodeChanged {
				changed = "  [el codigo cambio desde el hallazgo]"
			}
			fmt.Printf("  %-22s %-9s %-7s %s%s\n", it.ID, strings.ToUpper(it.Status), it.Severity, it.Description, changed)
			if it.Resolution != nil {
				fmt.Printf("      evidencia: %s\n", it.Resolution.Evidence)
			}
		}
		return nil
	default:
		return fmt.Errorf("subcomando desconocido %q (add|resolve|list)", sub)
	}
}

// cmdSpec records and queries human approvals bound to the spec's content
// hash: approve A, edit to B, and the approval is invalid by construction.
func cmdSpec(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("uso: hoom spec approve <ruta> | hoom spec status <ruta>")
	}
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	sub, path := args[0], args[1]
	switch sub {
	case "approve":
		rec, already, err := approval.Approve(m.Dir, path)
		if err != nil {
			return err
		}
		if already {
			fmt.Printf("hoom spec: %s ya estaba aprobado con este contenido exacto (sha256 %s, por %s) - sin cambios\n",
				rec.Spec, rec.SHA256[:8], rec.ApprovedBy)
			return nil
		}
		fmt.Printf("hoom spec: APROBADO %s\n  sha256:   %s\n  aprobado: %s (%s)\n  registro: .hoom/approvals/ (append-only, viaja en Git)\n",
			rec.Spec, rec.SHA256, rec.ApprovedBy, rec.ApprovedAt.Format("2006-01-02 15:04 UTC"))
		fmt.Println("  editar el spec ahora INVALIDA esta aprobacion (hash de contenido)")
		return nil
	case "status":
		state, rec, err := approval.Status(m.Dir, path)
		if err != nil {
			return err
		}
		fmt.Println("hoom spec:", approval.Describe(state, rec))
		if state != approval.StatusApproved {
			os.Exit(1)
		}
		return nil
	default:
		return fmt.Errorf("subcomando desconocido %q (approve|status)", sub)
	}
}

func cmdTask(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("uso: hoom task start <slug> | hoom task list | hoom task done <slug> [--force]")
	}
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "start":
		if len(rest) < 1 {
			return fmt.Errorf("uso: hoom task start <slug>")
		}
		return taskcmd.Start(m.Dir, rest[0], m.BaseBranch)
	case "list":
		for _, a := range rest {
			if a == "--json" || a == "-json" {
				raw, err := taskcmd.JSONBytes(m.Dir, m.BaseBranch)
				if err != nil {
					return err
				}
				fmt.Println(string(raw))
				return nil
			}
		}
		return taskcmd.List(m.Dir, m.BaseBranch)
	case "done":
		if len(rest) < 1 {
			return fmt.Errorf("uso: hoom task done <slug> [--force]")
		}
		force := false
		for _, a := range rest[1:] {
			if a == "--force" {
				force = true
			}
		}
		return taskcmd.Done(m.Dir, rest[0], m.BaseBranch, force)
	default:
		return fmt.Errorf("subcomando desconocido %q (start|list|done)", sub)
	}
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
	asJSON := fs.Bool("json", false, "emitir el historial como JSON en stdout")
	_ = fs.Parse(args)
	m, err := manifest.Load(".", profiles.Resolve)
	if err != nil {
		return err
	}
	all, err := verdict.LoadAll(m.Dir)
	if err != nil {
		return err
	}
	if *asJSON {
		raw, err := report.JSONBytes(all, *n)
		if err != nil {
			return err
		}
		fmt.Println(string(raw))
		return nil
	}
	report.History(os.Stdout, all, *n)
	return nil
}

// cmdServe starts the HoomAI Studio: the read-only dashboard embedded in
// this same binary. Loopback by default; exposing is a conscious --addr.
func cmdServe(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", servecmd.DefaultAddr, "direccion de escucha (host:puerto)")
	_ = fs.Parse(args)
	return servecmd.Run(".", *addr)
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
