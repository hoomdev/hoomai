# Spec: `hoom agent` — el sobre determinista de un rol

Estado: BORRADOR — pendiente de aprobación humana.
Trabajo en el worktree `Spec_B_hoom-agent` (rama `hoomdev/Spec_B_hoom-agent`).
Origen: Spec B de la evolución "HoomAI Agent Evolution". Funde el MVP de §40
con `hoom agent` (§11.2), roles como agent definitions (§12-§13), permission
engine (§14), anti-circularidad (§15) y verification lifecycle (§27) en UN
ítem implementable sobre lo que la Spec A dejó listo.

## Objetivo

Hoy `hoom run` lanza un CLI de IA en headless y narra lo que pasa. Nadie
verifica DESPUÉS qué tocó ese CLI ni si el rol que decía encarnar respetó sus
límites: el contrato del rol vive en un prompt, y un prompt es narrativa.

Esta spec agrega `hoom agent`: un SOBRE determinista alrededor de UNA sesión
headless. El sobre resuelve el rol (contrato como system prompt + herramientas
+ scope de escritura), elige el provider con sus capacidades declaradas, corre
el CLI, y al cerrar exige evidencia en este orden: **scope → verify → check**.
El scope es el aporte nuevo: hoom compara el árbol antes y después del run y
responde una pregunta que ningún prompt puede responder — *¿el rol escribió
solo donde le correspondía, y dejó la evidencia intacta?*

Dos invariantes se vuelven técnicas, no textuales:

1. **Cada rol escribe solo en su territorio.** El test-writer no toca la
   implementación; los roles de solo lectura no dejan cambios de código; el
   writer no reescribe el spec que el humano aprobó.
2. **La evidencia es append-only.** Un agente puede AGREGAR un veredicto o un
   hallazgo; jamás modificar ni borrar uno existente, jamás firmar una
   aprobación humana, jamás cambiar `hoom.yaml` ni aflojar el trinquete. Esto
   es AG-2 del documento ("ningún provider adapter puede alterar verdicts
   oficiales") convertido en un gate.

`hoom agent` NO orquesta: corre un rol, una vez, y contesta entregable o no
entregable con exit code. La orquestación sigue donde ya está — en la sesión
principal del CLI, con los subagentes nativos que `hoom agents --target` ya
genera desde los mismos contratos. hoom sigue siendo el árbitro, no la cabina.

## No-goals

- Bucle de corrección automático (run → rojo → re-prompt → run). El sobre
  corre UNA vez e imprime el comando exacto para reanudar. El bucle es del
  humano o del CLI, no del árbitro. Spec futura si se justifica.
- Orquestador propio, máquina de estados, `hoom swarm`, delegación entre
  providers: no.
- Sesiones Hoom persistentes (`.hoom/sessions/`). El sobre reusa `.hoom/runs/`
  para la narración y `.hoom/verdicts/` + `.hoom/findings/` para la evidencia
  durable. No se inventa un artefacto nuevo.
- Enforcement de LECTURA (anti-circularidad dura del test-writer). Un gate
  post-run ve escrituras, nunca lecturas. Bloquear la lectura de la
  implementación exige worktree disperso o sandbox: spec propia.
- v2 de opencode, codex y gemini. `hoom agent` exige `system_prompt` y hoy
  solo Claude la declara: el sobre lo dice con la capacidad que falta y el
  comando para verlo, en vez de degradar en silencio. Codex entra con la
  Spec C.
- Permission engine general del documento (allow/deny de comandos, red,
  variables de entorno). Acá van solo scope de escritura + herramientas.
- Cambios en el Studio, en `POST /api/runs` y en `hoom status`: el sobre no
  aparece todavía en la UI. Spec de Studio.
- `--allow-tools` / `--deny-tools` en `hoom agent`: las herramientas las fija
  el rol. Quien necesite otra cosa baja a `hoom run`, que no tiene política —
  aflojar se hace a la vista, no con un flag del sobre.
- Roles nuevos o edición de los 10 contratos existentes: se consumen tal cual.

## Contratos

### Paquete `internal/agents` (fuente única de la verdad de los roles)

La tabla `roles` que ya alimenta a `hoom agents --target` gana el scope y se
exporta. Un rol no se define en dos lugares.

```go
// Formas de scope de escritura de un rol.
const (
    ScopeEvidencia = "evidencia" // solo .hoom/**: specs, hallazgos, veredictos
    ScopeTests     = "tests"     // tests + .hoom/**; nunca implementación
    ScopeCodigo    = "codigo"    // todo salvo .hoom/specs/** (el spec es del arquitecto)
)

type Role struct {
    Slug     string `json:"slug"`   // writer
    Native   string `json:"native"` // hoom-writer
    File     string `json:"file"`   // 04-writer.md
    Desc     string `json:"desc"`
    ReadOnly bool   `json:"read_only"`
    Exec     bool   `json:"exec"`
    Primary  bool   `json:"primary"`
    Scope    string `json:"scope"`
}

func Roles() []Role                                  // orden de la tabla (00..09)
func Lookup(name string) (Role, error)               // acepta "writer" y "hoom-writer"
func Contract(dir string, r Role) (string, error)    // .hoom/agents/<File> si existe; si no, el embebido
```

Scope por rol: `orquestador`, `arquitecto`, `designer`, `scout`, `reviewer`,
`analista` y `refutador` → `evidencia`; `test-writer` y `characterizer` →
`tests`; `writer` → `codigo`.

### Paquete `internal/gitx`

```go
// Touched lista TODO lo que difiere de base: commits de la rama, árbol de
// trabajo y sin seguimiento, SIN las exclusiones del candidato — el sobre
// necesita ver justamente .hoom/verdicts/, .hoom/findings/ y ratchet.json.
// Valor = hash del contenido (git hash-object); "-" = la ruta ya no existe.
func Touched(dir, base string) map[string]string
```

`Snapshot`, `ChangedFiles` y la huella no cambian: sus exclusiones existen
para que registrar un veredicto no altere la huella que certifica, y eso sigue
igual.

### Paquete `internal/ratchet`

```go
// Loosened nombra las métricas que EMPEORARON o desaparecieron entre dos
// estados del trinquete. Apretar, congelar por primera vez y agregar métricas
// devuelven vacío: el trinquete solo puede subir.
func Loosened(before, after *File) []string
```

### Paquete `internal/manifest`

```go
type Manifest struct {
    // ...campos actuales...
    Agents map[string]AgentPolicy `yaml:"agents,omitempty"`
}

type AgentPolicy struct {
    Write WriteScope `yaml:"write,omitempty"`
}

type WriteScope struct {
    Allow []string `yaml:"allow,omitempty"`
    Deny  []string `yaml:"deny,omitempty"`
}
```

Clave del mapa = slug del rol (`test-writer`). Un `allow` declarado REEMPLAZA
los globs por defecto de la forma; `deny` se SUMA. Las reglas universales de
la sección siguiente no son aflojables desde el manifiesto: son el piso.

### Paquete nuevo `internal/agentcmd`

```go
type Options struct {
    Role      string // slug o nombre nativo; obligatorio
    Provider  string // "" = autoselección
    Task      string // slug de tarea: corre en su worktree
    Spec      string // ruta del spec: gate previo de aprobación + verify --spec
    Prompt    string
    Model     string
    ResumeID  string
    MaxTurns  int
    BudgetUSD float64
}

// Snapshot es la foto del árbol que el sobre toma antes y después del run.
type Snapshot struct {
    Touched  map[string]string // gitx.Touched
    Evidence map[string]bool   // rutas existentes bajo .hoom/{verdicts,findings,approvals}
    Manifest string            // hash de hoom.yaml ("" si no se pudo leer)
    Ratchet  *ratchet.File     // nil si no hay trinquete
}
func Take(root, base string) Snapshot

type Violation struct {
    Path      string `json:"path"`
    Rule      string `json:"rule"`   // manipulacion | fuera-de-scope
    Detail    string `json:"detail"` // por qué, en una línea
    FindingID string `json:"finding_id,omitempty"`
}

type ScopeResult struct {
    Touched    []string    `json:"touched"`    // rutas que el run cambió, ordenadas
    Violations []Violation `json:"violations"`
    Tampering  bool        `json:"tampering"`  // hay al menos una violación de regla universal
    OK         bool        `json:"ok"`
}
func CheckScope(before, after Snapshot, pol Policy) ScopeResult

type Policy struct{ Allow, Deny []string } // globs ya resueltos: forma del rol + manifiesto
func PolicyFor(m *manifest.Manifest, r agents.Role) Policy

type Result struct {
    Role      string        `json:"role"`
    Provider  string        `json:"provider"`
    Dir       string        `json:"dir"`
    Spec      string        `json:"spec,omitempty"`
    Approval  string        `json:"approval,omitempty"` // aprobado | invalidado | no-aprobado
    RunID     string        `json:"run_id,omitempty"`
    RunStatus string        `json:"run_status,omitempty"`
    SessionID string        `json:"provider_session_id,omitempty"`
    Scope     ScopeResult   `json:"scope"`
    VerdictID string        `json:"verdict_id,omitempty"`
    Verdict   string        `json:"verdict,omitempty"` // green | red
    Check     *checkcmd.Result `json:"check,omitempty"`
    Stage     string        `json:"stage"`   // spec | run | scope | verify | check | ok
    Status    string        `json:"status"`  // entregable | no-entregable
    ExitCode  int           `json:"exit_code"`
}

func Run(root, base string, opt Options, w io.Writer) (Result, error)
```

### Los cinco pasos del sobre

1. **spec** — solo si `--spec`. Con un rol que NO es de solo lectura, el spec
   debe estar `aprobado` (hash vigente): `invalidado` o `no-aprobado` cortan
   ANTES de lanzar el CLI, con la acción exacta (`hoom spec approve <ruta>`).
   Los roles de solo lectura no la exigen — el arquitecto escribe el spec que
   todavía no está aprobado.
2. **run** — `runcmd.Manager.Start` con `SystemPrompt` = contrato del rol,
   `Strict: true`, herramientas del rol, y `Model`/`MaxTurns`/`BudgetUSD`/
   `ResumeID` de los flags. La narración se imprime igual que en `hoom run`.
3. **scope** — `Take` antes, `Take` después, `CheckScope`.
4. **verify** — `verifycmd.Run` con `--spec` si se pasó.
5. **check** — `checkcmd.Run`.

Cortes: un `spec` no aprobado corta en 1. Un run con exit distinto de cero
corta en 2 (no hay árbol confiable que medir). Una violación de tipo
`manipulacion` corta en 3: un árbol donde el rol tocó la definición de la
exigencia o la evidencia no merece un veredicto nuevo. Una violación
`fuera-de-scope` NO corta: verify y check corren igual y el sobre cierra
`no-entregable` con el cuadro completo, que es lo que el humano necesita para
decidir.

### Elección y uso del provider

- `--provider <p>` explícito manda. Sin él: el primer provider del registro
  que esté instalado Y declare `system_prompt`. Ninguno → error nombrando la
  capacidad que falta y `hoom providers`.
- El sobre invoca SIEMPRE con `Strict: true`. Un rol sin su contrato como
  system prompt no es un rol: mejor negarse que fingir.
- Herramientas: interfaz opcional en `internal/providers`, resuelta con type
  assertion para no tocar la interfaz `Provider` que la Spec A acaba de fijar.

```go
// ToolNamer lo implementan los providers que nombran sus herramientas.
type ToolNamer interface {
    ReadOnlyTools(exec bool) (allow, deny []string)
}
```

Claude lo implementa con el MISMO vocabulario que ya genera
`.claude/agents/*.md`: `Read, Grep, Glob` (más `Bash` si el rol ejecuta) como
allow, y `Edit, Write, MultiEdit, NotebookEdit` (más `Bash` si no ejecuta)
como deny. Un rol de escritura no restringe herramientas. Un provider que no
implementa `ToolNamer` produce un aviso declarado — el gate de scope sigue
siendo la red que no depende del CLI.

### Reglas universales (piso, no aflojable)

Se evalúan sobre el delta del run, antes que la política del rol:

| Regla | Violación |
|---|---|
| `.hoom/verdicts/**` | modificar o borrar un archivo que existía antes del run (crear = `hoom verify`, permitido) |
| `.hoom/findings/**` | modificar o borrar un archivo que existía antes del run (crear = `hoom finding add`, permitido) |
| `.hoom/approvals/**` | cualquier cambio, crear incluido: la aprobación es del humano |
| `hoom.yaml` | cualquier cambio: es la definición de la exigencia |
| `.hoom/ratchet.json` | que alguna métrica quede peor o desaparezca (`ratchet.Loosened`); apretar es correcto |

Todas producen `Rule: "manipulacion"`.

### Globs y política del rol

Matcher propio (el proyecto no tiene dependencias fuera de yaml):

```go
// matchGlob soporta ** (cero o más segmentos), * (dentro de un segmento) y ?.
func matchGlob(pattern, path string) bool
```

Rutas siempre relativas al directorio de trabajo y con `/`. Defaults por forma:

- `evidencia` → allow `.hoom/**`.
- `tests` → allow `.hoom/**` más `tests/**`, `test/**`, `spec/**`,
  `src/test/**`, `**/*_test.go`, `**/*Test.php`, `**/*Test.kt`,
  `**/*Test.java`, `**/*Spec.kt`, `**/*.test.ts`, `**/*.test.js`.
- `codigo` → allow `**`, deny `.hoom/specs/**`.

Deny gana sobre allow. Una ruta fuera del allow produce `Rule:
"fuera-de-scope"` con un Detail que nombra la forma y, para `tests`, la acción
exacta: declarar `agents.test-writer.write.allow` en `hoom.yaml`.

### Hallazgos

Cada violación se registra con `finding.Add` (severidad `high`, lente `risk`,
`file` = la ruta) en el directorio donde corrió el run, y su id vuelve en
`Violation.FindingID`. La violación deja de ser un mensaje de terminal y pasa
a ser un artefacto append-only que aparece en `hoom status` y exige
resolución con evidencia.

### CLI

```
hoom agent --role <rol> [--provider p] [--task slug] [--spec ruta]
           [--model m] [--resume id] [--max-turns n] [--budget-usd x]
           [--json] "<pedido>"
```

Salida humana (un renglón por paso, la narración del run entre medio):

```
hoom agent: rol writer (claude) en .hoom/worktrees/auth-reset
  [1/5] spec    .hoom/specs/auth-reset.md APROBADO (vigente)
  [2/5] run     20260904T190000_ab12cd - narracion en .hoom/runs/...
  ...
  [3/5] scope   7 archivos tocados, 0 fuera de scope
  [4/5] verify  VERDE (veredicto 2026-09-04T19-12-00Z_9f3a1c2b)
  [5/5] check   VERDE (huella 4d2f9a10bb31c7e5)
hoom agent: ENTREGABLE
```

Exit: el del provider si el run falló; 1 si cortó por spec o falló scope,
verify o check; 0 solo con los cinco pasos verdes. `--json` imprime `Result`
y respeta el mismo exit. `hoom run` no cambia en nada.

README y el `usage` de `cmd/hoom/main.go` suman la fila de `hoom agent`.

## Casos límite y errores esperados

- **El agente corre `hoom verify` en su turno**: aparece un veredicto NUEVO en
  el delta. Es creación, no modificación: permitido. Si además corrió
  `verify --full` y el trinquete se apretó, `Loosened` devuelve vacío:
  permitido. Solo aflojar es violación.
- **El agente modifica un veredicto viejo** que no estaba en el diff contra
  base: al tocarlo pasa a estar en `Touched` y NO está en `Evidence` con
  contenido igual — por eso el snapshot previo lista las rutas existentes de
  los tres directorios de evidencia y no se confía solo en el diff.
- **Archivo tocado y devuelto a su contenido original**: mismo hash antes y
  después, no aparece en el delta. Correcto: el árbol es lo que importa, no la
  historia de ediciones.
- **Archivo ya sucio antes del run y modificado por el run**: hash distinto,
  aparece en el delta. El sobre mide el árbol, no la autoría.
- **Rol de solo lectura que igual escribió** (el CLI ignoró `--disallowedTools`
  o el provider no las soporta): el gate lo caza. Es exactamente el caso para
  el que existe.
- **Provider sin `system_prompt`** (opencode, codex, gemini hoy): `Strict`
  devuelve `ErrUnsupported` y no se crea run ni log. El mensaje nombra la
  capacidad y remite a `hoom providers`.
- **Provider sin `tools`**: aviso declarado, el run igual corre, el scope se
  verifica igual.
- **`--task` de una tarea inexistente**: el error actual de `runcmd`
  (`la tarea %q no existe`), antes de tocar nada.
- **Ya hay un run activo en ese árbol**: `ErrBusy` intacto; un writer por
  tarea sigue siendo la regla.
- **`--spec` con ruta inexistente**: error que la nombra, antes del run.
- **Rol desconocido**: error listando los 10 slugs.
- **`.hoom/agents/` no instalado**: se usa el contrato embebido y se avisa que
  `hoom agents` lo dejaría editable. Un contrato presente pero VACÍO es error:
  un rol sin contrato no es un rol.
- **El repo no es git**: `Touched` devuelve vacío, el delta es vacío y el
  scope pasa; verify y check ya se comportan como hoy en ese caso.
- **El run deja el árbol sin cambios** (un scout): delta vacío, scope verde,
  verify corre igual — y `check` puede quedar rojo por deriva previa. El sobre
  no maquilla: reporta lo que hay.
- **Violación de manipulación**: no corre verify. La salida dice qué rutas
  revertir y que el veredicto se emite después, no antes.
- **`agents.<rol>.write.allow` vacío en `hoom.yaml`** (`allow: []`): cuenta
  como no declarado y valen los defaults; para prohibir todo se usa `deny`.
- **Un rol `tests` en un proyecto con layout raro**: sus escrituras legítimas
  se marcan fuera de scope. El Detail trae el comando para declarar el layout
  en `hoom.yaml`; es ruido honesto, no un falso verde.
- **`finding.Add` falla** (disco, permisos): la violación igual se imprime y
  el exit sigue siendo 1. La evidencia en pantalla no depende del artefacto.

## Criterios de aceptación

- CA-127: `agents.Roles()` devuelve los 10 roles en orden con su `Scope`
  (`evidencia`, `tests`, `codigo`) y sus flags; `agents.Lookup` resuelve tanto
  `writer` como `hoom-writer`, es insensible a mayúsculas y falla ante un
  nombre desconocido listando los slugs válidos.
- CA-128: `agents.Contract`: con `.hoom/agents/04-writer.md` presente devuelve
  su contenido; sin el archivo devuelve el contrato embebido; con el archivo
  vacío o solo espacios devuelve error.
- CA-129: elección de provider: `--provider` explícito manda; sin él se elige
  el primer provider instalado que declare `system_prompt`; si ninguno lo
  declara, error que nombra la capacidad faltante y remite a `hoom providers`,
  sin crear run.
- CA-130: el sobre arma la `StartOptions` con `Strict: true`, el contrato como
  `SystemPrompt` y las herramientas del rol; un provider sin `system_prompt`
  produce `ErrUnsupported` y no deja run ni archivo de log.
- CA-131: `ToolNamer` en Claude: rol de solo lectura sin exec → allow
  `Read,Grep,Glob` y deny con `Edit,Write,MultiEdit,NotebookEdit,Bash`; rol de
  solo lectura con exec → `Bash` permitido y fuera del deny; rol de escritura
  → sin herramientas declaradas. Un provider sin `ToolNamer` produce aviso y
  el run igual arranca.
- CA-132: gate previo de aprobación: con `--spec` y un rol que escribe, un
  spec `no-aprobado` o `invalidado` corta en el paso `spec` con exit 1, sin
  run, y el mensaje trae `hoom spec approve <ruta>`; aprobado vigente sigue de
  largo; un rol de solo lectura no exige aprobación.
- CA-133: `gitx.Touched` incluye rutas que `Snapshot` excluye del candidato
  (`.hoom/verdicts/`, `.hoom/findings/`, `.hoom/ratchet.json`), cubre
  modificados, sin seguimiento y borrados (valor `-`), y devuelve un mapa
  vacío fuera de un repo git.
- CA-134: delta del run: un archivo creado, uno modificado y uno borrado
  aparecen en `Touched`; un archivo modificado y devuelto a su contenido
  original NO aparece; un archivo ya sucio antes y modificado por el run SÍ
  aparece.
- CA-135: `matchGlob`: `**` cruza cero o más segmentos (`.hoom/**` matchea
  `.hoom/a.json` y `.hoom/x/y/z.json` pero no `hoom.yaml`), `*` no cruza `/`,
  y `**/*_test.go` matchea tanto en la raíz como anidado.
- CA-136: scope por forma: `evidencia` marca fuera-de-scope cualquier ruta que
  no cuelgue de `.hoom/`; `tests` acepta `tests/x_test.go` y marca
  `internal/x.go`; `codigo` acepta `internal/x.go` y marca
  `.hoom/specs/y.md`. Cada violación trae `Path`, `Rule` y `Detail` no vacío.
- CA-137: reglas universales append-only: modificar o borrar un veredicto o un
  hallazgo existente es `manipulacion`; crear uno nuevo no lo es; cualquier
  cambio bajo `.hoom/approvals/` —crear incluido— es `manipulacion`; cualquier
  cambio en `hoom.yaml` es `manipulacion`.
- CA-138: `ratchet.Loosened`: una métrica que empeora respecto de su
  dirección, una que desaparece y una que cambia de dirección se nombran;
  apretar, congelar por primera vez y agregar una métrica nueva devuelven
  vacío. Un trinquete aflojado en el delta produce `manipulacion`.
- CA-139: `PolicyFor`: `agents.<rol>.write.allow` en `hoom.yaml` REEMPLAZA los
  globs por defecto de la forma y `deny` se suma; un `allow` vacío deja los
  defaults; y ninguna combinación del manifiesto evita una violación de
  `manipulacion` (el piso no se afloja).
- CA-140: cada violación crea un hallazgo `high` con lente `risk` y la ruta en
  `file`, cuyo id vuelve en `Violation.FindingID` y aparece en la salida; si
  `finding.Add` falla, la violación se reporta igual y el exit sigue en 1.
- CA-141: cortes: una violación `manipulacion` deja `Stage: "scope"` y NO
  ejecuta verify ni check (no se escribe veredicto nuevo); una violación solo
  `fuera-de-scope` ejecuta verify y check igual y cierra `no-entregable`.
- CA-142: camino verde completo con un provider falso que no toca el árbol:
  `Stage: "ok"`, `Status: "entregable"`, `ExitCode: 0`, `VerdictID` no vacío y
  `Check.OK` verdadero; verify se invocó con el `--spec` recibido.
- CA-143: exit codes: run con exit N mayor que cero → el sobre sale N y no
  corre scope ni verify; scope, verify o check en rojo → exit 1; los cinco
  pasos verdes → exit 0.
- CA-144: `--json` imprime el `Result` completo (role, provider, dir, spec,
  approval, run_id, scope con touched y violations, verdict_id, verdict,
  check, stage, status, exit_code) y respeta el mismo exit que el modo texto.
- CA-145: `hoom agent -h` lista los flags del sobre. [verifica: go run ./cmd/hoom agent -h 2>&1 | grep -q -- -role] [verifica: go run ./cmd/hoom agent -h 2>&1 | grep -q -- -budget-usd] [verifica: go run ./cmd/hoom agent -h 2>&1 | grep -q -- -json]
- CA-146: compatibilidad. [verifica: go test ./...]
  `hoom run`, `hoom providers`, el Studio, el cockpit y el status siguen
  verdes sin tocar sus aserciones.
- CA-147: E2E opcional: con `HOOM_E2E=1` y `claude` real en PATH, `hoom agent
  --role scout --max-turns 1 --budget-usd 1` sobre este repo cierra el run con
  exit 0, captura sesión y no deja ninguna violación de scope; sin la variable
  el test se omite y jamás es requisito de `go test`.

## Decisiones

- **Sobre, no orquestador.** Una pasada, cinco pasos, un exit code. La línea
  acordada (hoom es el árbitro, nunca la cabina) se mantiene: la coordinación
  entre roles la hace la sesión principal del CLI con los subagentes nativos
  que ya se generan desde estos mismos contratos.
- **El gate de scope es post-run y determinista.** El documento (§15) lista
  cinco formas de imponer límites; el sobre usa dos: la nativa del provider
  (best-effort, declarada) y la verificación posterior (determinista, igual
  para todos los providers). La segunda es la que decide.
- **La evidencia es append-only, y eso es el piso.** Crear un veredicto o un
  hallazgo es trabajo legítimo; reescribirlos, borrarlos, firmar una
  aprobación o cambiar `hoom.yaml` es manipulación. Ninguna configuración del
  proyecto puede aflojar esa regla — si se pudiera, no sería un piso.
- **El trinquete se compara semánticamente, no por hash.** `verify --full`
  mueve el archivo legítimamente; lo que importa es la dirección. Un hash
  simple daría un falso positivo cada vez que el agente hace lo correcto.
- **Violación = hallazgo.** Reusa el artefacto append-only que ya existe, con
  su resolución obligatoria con evidencia y su lugar en `hoom status`. Un
  mensaje de terminal se pierde; un hallazgo no.
- **Manipulación corta, fuera-de-scope no.** Emitir un veredicto sobre un
  árbol donde alguien tocó la definición de la exigencia sería certificar la
  trampa. Un writer que tocó el spec, en cambio, produce información útil: se
  reporta todo y se cierra en rojo.
- **Los roles viven en `internal/agents`, no en un paquete nuevo.** La tabla
  que genera los subagentes nativos y la que arma la invocación headless son
  la misma; dos tablas divergirían.
- **`ToolNamer` como interfaz opcional.** La interfaz `Provider` de la Spec A
  no se toca; el vocabulario de herramientas queda en el adapter, que es quien
  lo conoce.
- **`Strict` siempre.** En `hoom run` la degradación se avisa; en `hoom agent`
  se niega: sin el contrato como system prompt no hay rol que verificar.
- **Sin `--allow-tools` en el sobre.** Un flag para aflojar la política dentro
  del comando que la impone la vuelve decorativa. Aflojar existe: se llama
  `hoom run`.
- **Sin artefacto de sesión nuevo.** `.hoom/runs/` narra, `.hoom/verdicts/` y
  `.hoom/findings/` prueban. Un tercer formato sería narrativa con extensión
  `.json`.

## Riesgos y deuda aceptada

- **El gate no ve lecturas.** La anti-circularidad del test-writer sigue
  siendo una regla de prompt hasta la spec de aislamiento (worktree disperso).
  Se declara acá para no venderla como resuelta.
- **Globs de test por defecto multi-stack.** Cubren los layouts habituales de
  los cuatro perfiles, pero un proyecto con otro layout verá ruido hasta
  declarar `agents.test-writer.write.allow`. Ruido honesto sobre falso verde.
- **El writer puede tocar tests.** Su contrato lo permite (agregar los
  propios) y ningún gate determinista distingue "agregar" de "debilitar": eso
  lo sigue midiendo el gate de mutación. No se prohíbe lo que no se puede
  discriminar.
- **`hoom agent` solo funciona con Claude hasta la Spec C.** Es consecuencia
  de exigir `system_prompt`; se prefiere un sobre honesto y angosto a uno
  ancho que finge.
- **Matcher de globs propio.** Menos poderoso que `doublestar` y con su propio
  riesgo de bugs; a cambio, cero dependencias nuevas. Los casos que importan
  quedan fijados en CA-135.
- **El delta no atribuye autoría.** Si algo externo modifica el árbol mientras
  corre el run, se le imputa al rol. Un run activo por directorio y el uso en
  worktree lo hacen improbable; el sobre mide el árbol, que es lo verificable.
- **21 criterios.** Comparable a la Spec A y por la misma razón: el sobre
  funde el MVP del documento con cuatro secciones de política. Se implementa
  en un worktree con su propio veredicto.
