package verifycmd

import (
	"flag"
	"fmt"
	"strconv"
	"strings"

	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/manifest"
)

// UsageText is the EXACT usage block of the verb and the single source of
// that text: 'hoom help' concatenates this same constant, so the global usage
// and 'hoom verify --help' cannot diverge.
const UsageText = `Uso: hoom verify [--full] [--gate a,b] [--spec <ruta>] [--json]

  --full          Ignora el scoping por diff (corrida completa, ej. nocturna)
  --gate a,b      Ejecuta solo esos gates (el resto queda 'skipped'); el
                  veredicto queda PARCIAL: diagnostico, nunca referencia
                  de 'hoom check' ni de 'hoom task done'
  --spec <ruta>   Asocia el veredicto a un spec y suma los gates spec_lint,
                  spec_trace y spec_approved
  --json          Emite el veredicto como JSON en stdout (para agentes)

'hoom verify' no tiene subcomandos ni argumentos posicionales.
Accion: para leer veredictos ya emitidos usa 'hoom report'; para el estado
del proyecto, 'hoom status'.`

// ParseArgs is the SYNTACTIC stage and it is pure: it reads no disk, touches
// no environment and has no effects. That is why 'hoom verify show x'
// answers the same in a project without hoom.yaml — the argument error wins
// over the manifest error because it is decided before the project is read.
//
// It returns a *cliargs.UsageError for anything hoom does not understand, and
// cliargs.ErrHelp for -h/--help (which is not an error: stdout and exit 0).
func ParseArgs(args []string) (Options, error) {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	full := fs.Bool("full", false, "ignorar scoping por diff")
	gateList := fs.String("gate", "", "gates a ejecutar, separados por coma")
	specPath := fs.String("spec", "", "ruta del spec asociado")
	asJSON := fs.Bool("json", false, "emitir veredicto como JSON en stdout")
	if err := cliargs.Strict(fs, args, "verify", UsageText); err != nil {
		return Options{}, err
	}

	// Escribir un flag y no darle valor es un typo, no el default: quien
	// escribe --spec "" pedia un spec, no una corrida sin spec.
	var vacio string
	fs.Visit(func(f *flag.Flag) {
		if vacio == "" && f.Value.String() == "" {
			vacio = f.Name
		}
	})
	if vacio != "" {
		return Options{}, usoInvalido("--" + vacio + " vacio: escribir un flag y no darle valor es un typo")
	}

	opt := Options{Full: *full, Spec: *specPath, JSON: *asJSON}
	if *gateList != "" {
		// Comas sobrantes y duplicados son inequivocos y se aceptan; cero
		// nombres es ambiguo (¿ninguno o todos?) y hoy resolvia al peor de los
		// dos: TODOS, y como veredicto completo de referencia.
		for _, g := range strings.Split(*gateList, ",") {
			if g = strings.TrimSpace(g); g != "" {
				opt.Gates = append(opt.Gates, g)
			}
		}
		if len(opt.Gates) == 0 {
			return Options{}, usoInvalido("--gate vacio: no selecciona ningun gate")
		}
	}
	return opt, nil
}

// validarGates is the SEMANTIC stage: a gate name only means something
// against THIS project's manifest. It lives here and not in main because the
// verb and the Studio's POST /api/verify enter through the same function —
// validating only in the CLI would leave the bug to the Studio, which is
// literally the second brain this package forbids itself.
//
// The synthetic gates (spec_lint, spec_trace, spec_approved, ratchet) are not
// in m.Gates: --spec and --full govern them, so they are rejected here too.
func validarGates(m *manifest.Manifest, seleccion []string) error {
	for _, g := range seleccion {
		if _, ok := m.Gates[g]; ok {
			continue
		}
		return usoInvalido(fmt.Sprintf("gate desconocido %s; este proyecto declara: %s",
			strconv.Quote(g), strings.Join(m.SortedGateNames(), ", ")))
	}
	return nil
}

func usoInvalido(razon string) *cliargs.UsageError {
	return &cliargs.UsageError{Verb: "verify", Reason: razon, Usage: UsageText}
}
