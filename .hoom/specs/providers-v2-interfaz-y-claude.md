# Spec: capa de providers v2 — interfaz, capacidades, registro y adapter Claude

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start providers-v2`.
Origen: funde SPEC-001..005 del documento "HoomAI Agent Evolution" (interfaz
Provider, capabilities, registry, contract tests, adapter Claude y migración
de `hoom run`) en UN ítem implementable, como refactor in-place de lo que ya
existe en `internal/providers` e `internal/runcmd`.

## Objetivo

Hoy `internal/providers` es una tabla de cuatro entradas (`Spec`) con dos
capacidades implícitas (`Structured`, `CanContinue`) y un único parser
(Claude). Alcanza para `hoom run` y el Studio, pero no para lo que sigue:
`hoom agent` necesita lanzar un rol con su contrato como system prompt,
elegir modelo, acotar herramientas, turnos y gasto, y reanudar EXACTAMENTE la
sesión que abrió — no "la última de este directorio".

Esta spec convierte la tabla en una interfaz `Provider` con capacidades
declaradas y un registro, sin mover paquetes ni cambiar comportamiento
visible: `hoom run`, `hoom providers`, `hoom cockpit` y el Studio siguen
funcionando igual salvo lo que se agrega. El adapter de Claude sube a v2:
captura el `session_id` del stream, reanuda por id (`--resume`), pasa modelo,
system prompt, herramientas permitidas/prohibidas, máximo de turnos y
presupuesto. Los otros tres adapters (opencode, codex, gemini) conservan su
comportamiento actual y DECLARAN lo que no soportan. Una suite de contrato
única, consciente de capacidades, corre sobre los cuatro y sobre dos fakes
extremos. Principios intactos: degradación declarada, nunca silenciosa; y el
binario `hoom` sigue siendo el único que ejecuta procesos.

## No-goals

- `hoom agent`, roles como agent definitions, orquestador, sesiones Hoom,
  permission engine: specs siguientes (B en adelante). Acá va solo la capa
  que esas specs consumen.
- v2 de opencode, codex y gemini (salida estructurada nativa, reanudar por
  id): cada uno en su spec (codex es la Spec C). Acá implementan la interfaz
  con su comportamiento de hoy y capacidades honestas.
- Modo de permisos headless (`--permission-mode`, bypass): spec propia de
  autonomía headless. `AllowTools` ya evita el prompt de permiso para las
  herramientas listadas; el resto queda como hoy.
- Detección de autenticación ("authenticated: unknown" del documento):
  `Detect` sigue diciendo solo instalado/no instalado por PATH.
- Campos nuevos en `POST /api/runs` y en la UI del Studio: el Studio sigue
  mandando provider/prompt/task; la observabilidad de sesión llega en la
  spec de Studio.
- Filtrado del environment del subproceso: el CLI hereda el entorno como
  hoy (necesita su propia config y keychain).
- Conteo de turnos propio de hoom para providers que no lo acotan: el freno
  propio sigue siendo el timeout del run.
- Modo API directa, `hoom/v2` del manifiesto, árbol nuevo de paquetes
  (`internal/agent`, `internal/orchestration`, ...): no.

## Contratos

Paquete `internal/providers` (mismo paquete; archivos `providers.go`,
`claude.go`, `opencode.go`, `codex.go`, `gemini.go`, `contract_test.go`):

```go
type Capabilities struct {
    Structured   bool `json:"structured"`    // stdout parseable línea a línea
    Continue     bool `json:"continue"`      // continuar la ÚLTIMA sesión del directorio
    Resume       bool `json:"resume"`        // reanudar UNA sesión por id
    SessionID    bool `json:"session_id"`    // informa su id de sesión en el stream
    Model        bool `json:"model"`
    SystemPrompt bool `json:"system_prompt"` // agrega texto a su propio system prompt
    Tools        bool `json:"tools"`         // permitir/prohibir herramientas por nombre
    MaxTurns     bool `json:"max_turns"`
    Budget       bool `json:"budget"`        // tope de gasto en USD
}

type Request struct {
    Prompt       string
    ResumeID     string   // id de sesión del provider a reanudar; "" = ninguna
    Continue     bool     // continuar la última sesión del directorio (más débil que ResumeID)
    Model        string
    SystemPrompt string   // se AGREGA al system prompt del provider, nunca lo reemplaza
    AllowTools   []string // nombres/patrones en el vocabulario del provider, tal cual
    DenyTools    []string
    MaxTurns     int      // 0 = sin tope
    BudgetUSD    float64  // 0 = sin tope
    Strict       bool     // campo no soportado = error en vez de Ignored
}

type Invocation struct {
    Bin     string
    Args    []string
    Ignored []string // campos que el provider no pudo honrar (nombres canónicos)
}

type Provider interface {
    Name() string
    Bin() string
    Capabilities() Capabilities
    Command(req Request) (Invocation, error) // traduce; JAMÁS ejecuta
    Normalize(line string) []Event           // una línea de stdout -> eventos
}

type ErrUnsupported struct{ Provider string; Fields []string }

type Registry struct{ /* orden de inserción */ }
func NewRegistry() *Registry
func (r *Registry) Register(p Provider) error         // nombre vacío o duplicado = error
func (r *Registry) Lookup(name string) (Provider, error)
func (r *Registry) All() []Provider                   // orden de registro
func (r *Registry) Detect() []Info                    // PATH real, mismo orden
var Default = NewRegistry()                           // claude, opencode, codex, gemini (en init)
func Lookup(name string) (Provider, error)            // delegan a Default
func All() []Provider
func Detect() []Info
func JSONBytes() ([]byte, error)
func RenderText(w io.Writer, infos []Info)            // lo que imprime `hoom providers`

type Info struct {
    Name         string       `json:"name"`
    Installed    bool         `json:"installed"`
    Bin          string       `json:"bin,omitempty"`
    Capabilities Capabilities `json:"capabilities"`
}

type Event struct { // igual que hoy + session_id
    TS        time.Time `json:"ts"`
    Kind      string    `json:"kind"` // start | text | tool | agent | end | error
    Agent     string    `json:"agent,omitempty"`
    Detail    string    `json:"detail,omitempty"`
    SessionID string    `json:"session_id,omitempty"` // solo en start/end/error, cuando el provider lo informa
}
```

Nombres canónicos de `Ignored` y de `ErrUnsupported.Fields`: `resume`,
`continue`, `model`, `system_prompt`, `tools` (cubre allow y deny),
`max_turns`, `budget`. Mensaje de `ErrUnsupported`:
`el provider %q no soporta: %s (mira 'hoom providers')`.

Reglas comunes de `Command` (la suite de contrato se las exige a todos):

- Prompt vacío o que empieza con `-`: error (evita que el CLI lo lea como
  flag). Lo mismo para un `ResumeID` que empieza con `-`.
- `MaxTurns` o `BudgetUSD` negativos: error.
- Entradas vacías o solo espacios en `AllowTools`/`DenyTools` se
  descartan; si no queda ninguna, el campo cuenta como no enviado.
- El prompt es SIEMPRE el último argumento y aparece exactamente una vez;
  ningún argumento es la cadena vacía.
- Campo enviado con capability falsa: se omite y su nombre canónico va a
  `Ignored` (una vez, aunque allow y deny vengan juntos). Con `Strict`, la
  misma request devuelve `ErrUnsupported` con TODOS los campos no
  soportados y sin Invocation.
- Precedencia de sesión: `ResumeID` con capability `resume` gana; si no,
  `Continue` con capability `continue`; si ninguna aplica, invocación nueva
  y lo que no se honró va a `Ignored`.

Adapter Claude v2 (binario `claude`; verificado contra Claude Code 2.1.259
el 2026-09-04):

- Capacidades: las nueve verdaderas.
- Comando base sin opciones, idéntico al actual:
  `claude -p --output-format stream-json --verbose <prompt>`.
- `AllowTools` → `--allowedTools <a,b,c>`; `DenyTools` →
  `--disallowedTools <a,b,c>`: UN argumento separado por comas, ubicados
  ANTES de `-p`. Son opciones variádicas de commander: consumen posicionales
  hasta la siguiente opción; puestas al final se tragarían el prompt.
- `ResumeID` → `--resume <id>` (sin `--continue`); `Continue` sin
  `ResumeID` → `--continue`.
- `Model` → `--model <m>` tal cual (alias o nombre completo; hoom no valida
  nombres de modelo).
- `SystemPrompt` → `--append-system-prompt <texto>`; nunca
  `--system-prompt`, que reemplazaría el prompt del CLI y con él su
  comportamiento normal (CLAUDE.md, subagentes nativos).
- `MaxTurns` → `--max-turns <n>`. En 2.1.259 la opción está registrada y
  valida "must be a number", pero ya NO figura en `--help`: se declara
  soportada con ese riesgo anotado.
- `BudgetUSD` → `--max-budget-usd <n>` formateado con
  `strconv.FormatFloat(x, 'f', -1, 64)`: decimal, jamás notación científica.
- `Normalize`: `type: system` + `subtype: init` → evento `start` con
  `SessionID` si la línea trae `session_id`; `type: result` → si
  `subtype == "success"` e `is_error` no es true, evento `end` con el texto
  de `result` (o el subtype si viene vacío) y `SessionID`; en cualquier otro
  caso evento `error` con Detail `<subtype>: <texto>` y `SessionID`. Texto,
  tool_use y delegación (`Task` → kind `agent`) siguen igual.

Adapters opencode, codex y gemini: comandos idénticos a los actuales
(`opencode run [--continue] <prompt>`; `codex exec <prompt>` /
`codex exec resume --last <prompt>`; `gemini -p <prompt>`). Capacidades:
`continue` en opencode y codex; ninguna en gemini. `Normalize` = un evento
`text` por línea.

Paquete `internal/runcmd`:

```go
type StartOptions struct {
    Provider, Prompt, Task        string
    ResumeID, Model, SystemPrompt string
    AllowTools, DenyTools         []string
    MaxTurns                      int
    BudgetUSD                     float64
    Strict                        bool
}
func (m *Manager) Start(opts StartOptions) (Run, error)
func ResolveSystemPrompt(arg string) (string, error) // "@ruta" lee el archivo; el resto es literal
```

- `Run` gana `ProviderSessionID string json:"provider_session_id,omitempty"`:
  el último `SessionID` no vacío visto en los eventos del run.
- Cada campo en `Ignored` produce un evento `text` en el log:
  `aviso: <provider> no soporta <campo>; se ignora`. Con `Strict`, `Start`
  devuelve el `ErrUnsupported` del adapter y no crea run ni log.
- El run recuerda sus `StartOptions`. `Input(id, prompt)` arma la request
  con el nuevo prompt, `ResumeID = ProviderSessionID`, `Continue = true` y
  las mismas opciones originales (modelo, system prompt, tools, topes); el
  adapter elige la vía más fuerte que soporte. El aviso actual para
  providers sin continuación se conserva.
- El evento `start` del run mantiene su formato
  (`run <id>: <provider> en ...`): `hoom status` lo parsea.
- Un run activo por directorio, timeout, cancelación y huérfanos: sin
  cambios.

CLI `hoom run` (`cmd/hoom/main.go`, misma semántica de exit que hoy):

- Flags nuevos: `--resume <id>`, `--model <m>`,
  `--system-prompt <texto|@ruta>`, `--allow-tools a,b`, `--deny-tools a,b`,
  `--max-turns n`, `--budget-usd x`, `--strict`. `--continue` (que solo
  devolvía un error) desaparece.
- Al terminar, si el run capturó sesión, imprime una línea con el id y el
  comando para reanudarla (`hoom run --provider <p> --resume <id> "..."`).

CLI `hoom providers`:

- `--json`: cada provider suma `capabilities` (nueve booleanos); `name`,
  `installed`, `bin` y el orden no cambian.
- Texto: mismo encabezado de hoy y, bajo cada provider, una línea
  `capacidades: <lista de las soportadas>`; sin ninguna,
  `capacidades: texto plano (sin sesion ni stream)`. Sale de
  `providers.RenderText`, que main solo invoca.

Callers que cambian con la firma y nada más: `cockpitcmd` resuelve el
binario con `Provider.Bin()`; `servecmd` construye `StartOptions` con los
tres campos que ya recibe; `statuscmd` no cambia. README: filas de
`hoom run` y `hoom providers` actualizadas con los flags y las capacidades.

## Casos límite y errores esperados

- `--allowedTools` al final del comando: bug clásico, el prompt pasa a ser
  una "herramienta permitida" y el CLI espera stdin hasta el timeout. Por
  eso el contrato fija la posición ANTES de `-p` y la suite exige que el
  prompt sea el último argumento.
- Init sin `session_id` (CLI vieja, fake, provider sin la capacidad): el
  `start` sale sin `SessionID`, `ProviderSessionID` queda vacío e `Input`
  cae a `--continue`: exactamente el comportamiento de hoy (la
  continuación del Studio).
- `session_id` distinto en init y en result (no debería ocurrir): gana el
  último.
- `Input` sobre un provider con `resume` pero sin sesión capturada:
  `--continue`, la sesión-del-directorio de siempre; el log no inventa ids.
- `ResumeID` en un run nuevo (`hoom run --resume`): primera invocación con
  `--resume <id>`; si el provider no lo soporta, aviso + Ignored, y con
  `--strict` se niega antes de crear el run.
- Prompt de varios KB (un contrato de rol entero como system prompt): va en
  argv; el límite del sistema (cientos de KB por argumento) no se toca. Un
  prompt mayor es un error del CLI, no de hoom.
- `--system-prompt @ruta` con ruta inexistente: error que nombra la ruta;
  `@` solo (sin ruta) es literal.
- `--allow-tools ",,"`: todas vacías, cuenta como no enviado: sin flag y
  sin aviso.
- `--budget-usd 0.1` → `--max-budget-usd 0.1`; `--budget-usd 0.0000001` se
  formatea decimal, nunca `1e-07`.
- Result con `is_error: true` y subtype `success` (defensivo): `error`.
- Líneas de más de 4 MB: el scanner del run ya las corta (límite vigente);
  fuera de alcance, no empeora.
- Registro duplicado desde el `init` del paquete: `Register` devuelve error
  y el `init` entra en pánico, porque es un bug del binario y no un estado
  del proyecto.

## Criterios de aceptación

- CA-108: `Registry`: `Register` rechaza nombre vacío y nombre duplicado con
  error; `All` y `Detect` devuelven los providers en orden de registro;
  `Lookup` de un nombre desconocido falla listando los soportados. `Default`
  trae claude, opencode, codex y gemini en ese orden y `Detect` reporta
  `installed` según el PATH real.
- CA-109: suite de contrato que corre sobre todos los providers de `Default`
  y sobre dos fakes (todas las capacidades / ninguna): prompt vacío o que
  empieza con `-` → error; prompt válido → `Bin` del provider, prompt como
  último argumento exactamente una vez, `Ignored` vacío y ningún argumento
  vacío.
- CA-110: por cada campo opcional del `Request` enviado solo: capability
  verdadera → los args difieren de la invocación plana y el campo no está
  en `Ignored`; capability falsa → el campo aparece en `Ignored` con su
  nombre canónico y los args son los de la invocación plana. `AllowTools` y
  `DenyTools` juntos sin capability producen `tools` una sola vez.
- CA-111: con `Strict`, un campo no soportado devuelve `ErrUnsupported`
  nombrando el provider y TODOS los campos no soportados, sin Invocation;
  sin `Strict`, la misma request produce Invocation con esos campos en
  `Ignored`.
- CA-112: `Normalize` en todos los providers: línea vacía o solo espacios →
  ningún evento; línea no reconocida → exactamente un evento `text` con la
  línea íntegra; provider sin `structured` → hasta un JSON válido es
  `text`; una línea de 1 MB no entra en pánico y produce al menos un
  evento.
- CA-113: adapter Claude sin opciones → args exactamente
  `-p --output-format stream-json --verbose <prompt>`, y sus capacidades
  declaradas son las nueve verdaderas.
- CA-114: Claude con `ResumeID` → `--resume <id>` y sin `--continue`; con
  `Continue` sin `ResumeID` → `--continue`; con ambos → solo `--resume`.
- CA-115: Claude con `Model` → `--model <m>`; con `SystemPrompt` →
  `--append-system-prompt <texto>` y nunca `--system-prompt`; con
  `MaxTurns` → `--max-turns <n>`; con `BudgetUSD` → `--max-budget-usd <n>`
  decimal sin notación científica; turnos o presupuesto negativos → error.
- CA-116: Claude con `AllowTools`/`DenyTools` → `--allowedTools` /
  `--disallowedTools` con UN argumento separado por comas, entradas vacías
  descartadas, ambos ubicados antes de `-p`, y el prompt sigue último.
- CA-117: `Normalize` de Claude: `system/init` con `session_id` → `start`
  con `SessionID`; `result` `success` → `end` con el texto y `SessionID`;
  `result` con `is_error` o subtype de error → evento `error` con
  `<subtype>: <texto>`; init sin `session_id` → `start` sin `SessionID`.
- CA-118: `Manager.Start(StartOptions)` traduce las opciones a la
  invocación: con un fake que imprime argv aparecen `--model`,
  `--append-system-prompt`, `--allowedTools`, `--max-turns` y
  `--max-budget-usd` con los valores dados.
- CA-119: sesión capturada: un fake que emite `system/init` con
  `session_id` deja `Run.ProviderSessionID` con ese id (también en su
  JSON); `Input` invoca con `--resume <id>` y sin `--continue`, y re-aplica
  las opciones originales (el mismo `--append-system-prompt`).
- CA-120: sin sesión capturada, `Input` invoca con `--continue` (la
  continuación actual del Studio, intacta); un provider sin `continue` ni `resume` invoca de nuevo con el
  aviso actual en el log; `StartOptions.ResumeID` en un run nuevo produce
  `--resume <id>` en la primera invocación.
- CA-121: campos ignorados → un evento `text`
  `aviso: <provider> no soporta <campo>; se ignora` por campo en el log del
  run; con `Strict` → `Start` devuelve `ErrUnsupported` y no existe run ni
  archivo de log.
- CA-122: `ResolveSystemPrompt`: `@ruta` devuelve el contenido del archivo;
  ruta inexistente → error que la nombra; texto sin `@` inicial se devuelve
  literal; `@` solo es literal.
- CA-123: `hoom run -h` lista los flags nuevos. [verifica: go run ./cmd/hoom run -h 2>&1 | grep -q -- -budget-usd] [verifica: go run ./cmd/hoom run -h 2>&1 | grep -q -- -strict] [verifica: go run ./cmd/hoom run -h 2>&1 | grep -q -- -system-prompt]
- CA-124: `hoom providers --json` suma `capabilities` con los nueve
  booleanos por provider y conserva `name`, `installed`, `bin` y el orden
  claude, opencode, codex, gemini; `RenderText` imprime por provider su
  estado y la línea `capacidades:` con las soportadas (o `texto plano` si
  no hay ninguna).
- CA-125: compatibilidad. [verifica: go test ./internal/providers ./internal/runcmd ./internal/servecmd ./internal/cockpitcmd ./internal/statuscmd]
  Los tests de las specs anteriores sobre providers, runs, Studio, cockpit
  y status siguen verdes sin tocar sus aserciones (solo la firma de
  `Start`).
- CA-126: E2E opcional: con `HOOM_E2E=1` y `claude` real en PATH, un test
  corre Claude con `--max-turns 1` y presupuesto 0.05 USD y exige
  `SessionID` no vacío en el `start` y en el cierre; sin la variable el
  test se omite (skip) y jamás es requisito de `go test`.

## Decisiones

- Adapters puros (`Command` traduce, `Normalize` parsea) y `runcmd` como
  ÚNICO ejecutor: desvío deliberado de la interfaz Start/Send/Resume/Stop
  del documento. Un solo lugar para timeout, cancelación, huérfanos y
  un-run-por-directorio; los adapters se prueban con tablas, sin procesos.
- Mismo paquete `internal/providers`: el árbol de siete paquetes del
  documento no encaja con un core de ~5k líneas ni con la convención plana
  del repo.
- Registro con orden de inserción, no alfabético: la salida de
  `hoom providers` y `/api/providers` no cambia de orden. Tipo `Registry`
  además de `Default` para que los tests no compartan estado global.
- `Ignored` + `Strict` como modelo de degradación: lo visible por defecto,
  lo estricto para quien necesita garantías (los roles de la Spec B).
- `--append-system-prompt` por argv y no la variante `-file`: `Command`
  queda puro, sin ciclo de vida de archivos temporales; un contrato de rol
  pesa KB y cabe en argv.
- `--max-turns` declarado soportado aunque el help de 2.1.259 no lo
  documente: el binario lo registra y lo valida; el E2E opcional lo vigila.
  `--max-budget-usd` (documentado) es el segundo tope, complementario.
- `SessionID` solo en `start`, `end` y `error`: el log no repite el id en
  cada línea; el run guarda el último.
- `Input` re-aplica las opciones originales: el propio CLI dice que
  `--append-system-prompt` aplica "fresh each launch"; sin re-aplicarlo, la
  reanudación perdería el contrato del rol.
- Otros adapters sin cambios de comportamiento: la abstracción se prueba
  multi-provider con capacidades honestas, no con features nuevas.
- El prompt sigue en argv (no stdin): no rompe el test actual del
  subproceso ni el eco de argv de los fakes; stdin queda como opción futura para prompts enormes.

## Riesgos y deuda aceptada

- Deriva de flags del CLI (`--max-turns` ya salió del help): adapter
  aislado + E2E opcional. Mitigación pendiente: `hoom providers` mostrando
  la versión detectada (spec futura).
- Suposición sobre opciones variádicas de commander: si cambian, el orden
  elegido (antes de `-p`) sigue siendo correcto aunque dejen de serlo.
- System prompt visible en `ps` mientras corre: uso local, aceptado; la
  variante `-file` queda anotada como alternativa.
- Flags desconocidos: en 2.1.259 `--version` los ignora; en un run real el
  CLI podría fallar o ignorar en silencio. El E2E es la única red.
- 19 criterios en un ítem, más que los anteriores: justificado por fundir
  cinco specs; se implementa en un único worktree con su propio veredicto.
