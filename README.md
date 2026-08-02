# hoomAI

Harness de verificacion **agnostico de IA y de stack** para desarrollo asistido por agentes.

La idea central: la confianza nunca vive en el modelo. Vive en **gates deterministicos**
(tests, analisis estatico, mutation testing, tests de arquitectura) atados al scope real
derivado de Git, registrados como **veredictos inmutables** que viajan con el proyecto.
La narracion del agente no cuenta; solo cuenta la evidencia.

## Instalacion en un comando

```sh
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/hoomdev/hoomai/main/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/hoomdev/hoomai/main/install.ps1 | iex
```

Alternativas:

```sh
go install github.com/hoomdev/hoomai/cmd/hoom@latest   # si tienes Go 1.22+
go build -o hoom ./cmd/hoom                               # desde el repo (vendor incluido, sin red)
```

## Uso en 60 segundos

```sh
cd mi-proyecto
hoom init        # detecta el stack (laravel|kmp|kmp-compose|go) y crea hoom.yaml
hoom verify      # ejecuta los gates y emite un veredicto: ROJO = exit 1
hoom agents      # instala los 8 contratos de agentes y ata AGENTS.md
hoom report      # historial y tendencia por gate
```

## Filosofia

1. **Agnostico de IA**: hoomAI nunca habla con un modelo. Cualquier agente (Claude Code,
   OpenCode, Codex CLI, Gemini CLI, Ollama) se ata al mismo contrato: `hoom verify` antes
   de entregar, veredicto rojo = no se entrega.
2. **Anti-circularidad**: la IA no puede revisar su propio codigo. Por eso el test-writer
   es adversarial (escribe desde el spec, JAMAS ve la implementacion) y el mutation
   testing mide la calidad de los tests, no solo su existencia.
3. **Degradacion declarada, nunca silenciosa**: un gate sin comando aparece en AMARILLO
   en cada veredicto. La red de seguridad incompleta se ve; no se oculta.
4. **Politica estricta**: fail o error en un gate requerido = veredicto ROJO = exit 1.
   Herramienta configurada pero rota = ERROR (fail-closed), tambien rojo.
5. **Veredictos append-only en Git**: `.hoom/verdicts/<timestamp>_<hash>.json`, un archivo
   por corrida, cero conflictos de merge por construccion. El historial viaja con el
   proyecto entre maquinas y CI. Cualquier indice (SQLite futuro) es cache regenerable.

## Comandos

| Comando | Que hace |
|---|---|
| `hoom init` | Detecta el stack, crea `hoom.yaml` con los gates del perfil materializados y `.hoom/` |
| `hoom verify` | Ejecuta los gates y emite un veredicto (rojo = exit 1) |
| `hoom verify --full` | Ignora el scoping por diff (corrida completa, ej. nocturna) |
| `hoom verify --gate test,static` | Ejecuta solo esos gates |
| `hoom report -n 10` | Historial de veredictos + tendencia de pass-rate por gate |
| `hoom check` | Compara el arbol ACTUAL contra el ultimo veredicto: verde + huella coincidente = OK |
| `hoom hook` | Instala el pre-push de Git que exige `hoom check` antes de integrar |
| `hoom verify --json` | Veredicto como JSON en stdout, para consumo de agentes (in-band) |
| `hoom agents` | Instala los 9 contratos de agentes en `.hoom/agents/` y ata `AGENTS.md` |
| `hoom agents --target all` | Genera ademas los subagentes NATIVOS de Claude Code, OpenCode, Codex y Gemini CLI |
| `hoom profiles` | Lista los perfiles embebidos |

## Perfiles (v1)

- **laravel** - Pest/PHPUnit, PHPStan, Pint, arch tests de Pest, Infection (mutation por
  diff con `--git-diff-base`).
- **kmp** - Gradle allTests, detekt, Konsist, Pitest (parcial declarado: solo target JVM).
- **kmp-compose** - hereda kmp + lint de Compose + metricas del Compose compiler.
- **go** - go test -race (diff por paquetes), go vet, golangci-lint, arch-go, Gremlins.

Los perfiles son defaults: `hoom init` los materializa en `hoom.yaml` y ahi los editas.
`cmd: ""` declara un gate AUSENTE explicitamente (amarillo, nunca oculto). La herencia
(`extends`) permite crear perfiles derivados (ej. un futuro `filament extends laravel`).

## Manifiesto (hoom.yaml)

```yaml
schema: hoom/v1
project: mi-proyecto
profile: laravel
base_branch: main
policy: strict
gates:
  test:
    required: true
    cmd: "vendor/bin/pest --colors=never"
  mutation:
    required: true
    cmd: "vendor/bin/infection --threads=max --no-progress --min-msi=60"
    diff_cmd: "vendor/bin/infection --threads=max --no-progress --git-diff-filter=AM --git-diff-base={base} --min-msi=60"
```

Variables de template: `{base}` (rama base), `{files}` (archivos cambiados),
`{packages}` (paquetes Go derivados de los .go cambiados). Si un `diff_cmd` expande a
vacio, cae al comando completo en lugar de ejecutar una linea rota.

## Congelamiento del candidato (anti-fraude de veredicto)

Cada veredicto congela una **huella SHA-256 del cambio exacto verificado** (commit +
contenido de cada archivo tocado) y el tamano del cambio (+ins/-del). `hoom check`
compara esa huella contra el arbol actual: si un agente verifica A y entrega B, la
huella no coincide y el check es ROJO con la accion exacta a ejecutar. `hoom hook`
lleva esto al pre-push: sin veredicto verde con huella coincidente, no hay push
(`HOOM_SKIP=1 git push` existe como escape consciente y visible, nunca silencioso).
Mas de 400 lineas cambiadas dispara deterministicamente la review con 4 lentes.
Es la idea central de RDD (Receipt Driven Development) sin la ceremonia
criptografica: el modelo de amenaza local es la deriva, no la falsificacion.

## Gate de seguridad (Semgrep)

Todos los perfiles incluyen un gate `security` (ausente por defecto; se activa
instalando semgrep y llenando el cmd). Recomendado: reglas `p/default` +
`p/trailofbits`, con `--baseline-commit` para scoping por diff. Las reglas
propias del proyecto viven versionadas en `.hoom/semgrep/`: cuando una review
encuentra un patron peligroso, se convierte en regla (las skills de Trail of
Bits para Claude Code hacen exactamente eso en la capa del agente) y el gate lo
bloquea para siempre. En proyectos con auth/pagos/facturacion: `required: true`.

## Los 8 agentes (.hoom/agents/)

00 Orquestador (padre, nunca escribe codigo) - 01 Arquitecto (produce el spec que ata la
cadena) - 02 Designer (dueño del design system; el design NO entra al verify) - 03 Scout
(exploracion solo-lectura, Ollama-compatible) - 04 Writer (UNICO que edita, uno por
tarea) - 05 Test-writer adversarial (PROHIBIDO ver la implementacion) - 06 Reviewer
(4 lentes deterministicas segun riesgo) - 07 Characterizer (fija el comportamiento
actual de codigo legacy antes de refactorizar).

`verify` NO es un agente: es un comando deterministico.

## Subagentes nativos (multi-CLI)

Los contratos de `.hoom/agents/` son la unica fuente de verdad. Con
`hoom agents --target claude,opencode,codex,gemini` (o `all`) se generan los
subagentes en el formato nativo de cada herramienta:

| Target | Genera | Enforcement duro |
|---|---|---|
| `claude` | `.claude/agents/*.md` | roles de solo lectura sin herramientas de edicion |
| `opencode` | `.opencode/agents/*.md` | orquestador PRIMARY con `edit: deny`; subagentes con permisos por rol |
| `codex` | `.codex/agents/*.toml` | roles de solo lectura con `sandbox_mode = "read-only"` |
| `gemini` | `.gemini/agents/*.md` | tools restringidas a lectura por rol |

Donde la CLI lo soporta, la disciplina del contrato deja de ser texto y se
vuelve imposibilidad tecnica: el scout no puede editar aunque quiera. En
Claude Code, Codex y Gemini el orquestador es tu sesion principal (atada via
AGENTS.md); en OpenCode es un agente primary seleccionable con Tab. Nota:
Antigravity CLI aun no carga subagentes; el target `gemini` sirve a Gemini CLI
(licencias Code Assist) y se agregara `antigravity` cuando Google documente su
formato.

## Publicar un release (mantenedores)

```sh
git tag v0.2.0 && git push origin v0.2.0
```

El workflow de GitHub Actions (goreleaser) compila los binarios para
linux/darwin/windows (amd64+arm64), genera `checksums.txt` y publica el release.
`install.sh` siempre apunta al release mas reciente.

## Roadmap

- Fase 2: `hoom characterize` (characterization tests asistidos sobre el blast radius).
- Cache SQLite regenerable en `.hoom/cache/` para historiales grandes.
- `hoom onboard` (bootstrap de codebase-memory-mcp + Engram en un proyecto).
- Perfil `filament extends laravel`. Homebrew tap / Scoop / AUR.

## Licencia

MIT
