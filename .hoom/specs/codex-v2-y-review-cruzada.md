# Spec: adapter Codex v2 y review cruzada

Estado: BORRADOR — pendiente de aprobación humana.
Trabajo en el worktree `Spec-C-adapter-Codex-v2-y-review-cruzada`
(rama `hoomdev/Spec-C-adapter-Codex-v2-y-review-cruzada`).
Origen: Spec C de la evolución "HoomAI Agent Evolution". Es el ítem que el
documento exige ANTES de cualquier orquestador: probar que la abstracción de
providers no está acoplada a Claude. La Spec B lo dejó escrito como deuda
propia — *"`hoom agent` solo funciona con Claude hasta la Spec C"* — y esta
spec la paga.

## Objetivo

Hoy el sobre determinista (`hoom agent`) exige `system_prompt` y **solo Claude
la declara**: un segundo provider ni siquiera puede encarnar un rol. Mientras
eso siga así, "la capa de providers" es Claude con otro nombre, y cualquier
orquestador construido encima heredaría ese acoplamiento sin que nadie lo note.

Esta spec hace dos cosas, y la segunda es la prueba de la primera:

1. **Adapter Codex v2.** Codex CLI 0.151.0 sabe hacer casi todo lo que hoom le
   pide: emite JSONL con `--json`, reporta el id de su hilo, reanuda por id,
   elige modelo, acepta instrucciones propias sin borrar las suyas y, además,
   impone el límite de un rol de solo lectura con un **sandbox** — enforcement
   más duro que el deny de herramientas de Claude. Nada de eso estaba
   declarado: el adapter actual anuncia `continue` y nada más.

2. **La abstracción se dobla donde tenía que doblarse.** El límite de un rol
   viaja hoy como *nombres de herramientas de Claude* (`Edit`, `Write`,
   `MultiEdit`). Codex no nombra herramientas: nombra un sandbox. Si
   tradujéramos el sandbox al dialecto de Claude, el vocabulario común sería el
   de un provider. Entonces el vocabulario sube un nivel: la `Request` pasa a
   pedir **la intención** (`ReadOnly`, `Exec`) y cada adapter la impone con lo
   que tenga — Claude con deny de herramientas, Codex con `sandbox_mode`. La
   interfaz `ToolNamer` que la Spec B agregó como puente desaparece: fue el
   andamio de un solo provider.

3. **`hoom review`: la review cruzada como propiedad verificable.** Con dos
   providers que sostienen un contrato aparece el primer uso multi-provider
   real: el writer escribió con Claude, el reviewer revisa con Codex y registra
   con `hoom finding add`. hoom no se limita a permitirlo: lo **verifica**. La
   lente sale del contrato 06 y de la evidencia (`insertions+deletions` del
   veredicto), no del criterio del modelo; el provider de la review se elige
   distinto del que escribió, y si el humano fuerza el mismo, hoom se niega y
   lo dice; y el resultado de la review son los hallazgos que hoom **observa**
   aparecer en `.hoom/findings/`, no los que el CLI dice haber registrado.

Lo que esta spec no cambia: hoom sigue siendo el árbitro. `hoom review` corre
UN rol por pasada, sin ruteo, sin máquina de estados y sin decidir nada a
partir de lo que el modelo contesta.

### Lo que se verificó a mano contra Codex CLI 0.151.0 (macOS, cuenta ChatGPT)

Todo lo de abajo se probó en esta máquina antes de escribir los contratos:

- `codex exec --json` emite JSONL. Vocabulario real observado y confirmado
  contra los símbolos del binario: `thread.started` (trae `thread_id`),
  `turn.started`, `turn.completed` (trae `usage`), `turn.failed`, `item.started`,
  `item.updated`, `item.completed` y un `error` de nivel superior. Los `item`
  tienen `type`: `agent_message`, `reasoning`, `command_execution`,
  `file_change`, `mcp_tool_call`, `web_search`, `todo_list`, `error`.
- `codex exec resume <thread_id> "<prompt>"` reanuda y **vuelve a emitir
  `thread.started` con el mismo id**. `resume --last` continúa el más reciente.
- `codex exec resume` **no tiene** `-s/--sandbox`; el modo se pasa igual en las
  dos formas con `-c sandbox_mode="read-only"|"workspace-write"`, y se comprobó
  que surte efecto: con `read-only` el CLI no pudo crear un archivo ni cuando el
  pedido lo exigía, y el turno igual cerró con exit 0.
- `-c developer_instructions="<texto>"` es un campo de config **reconocido**
  (`--strict-config` acepta ese nombre y rechaza `base_instructions`,
  `user_instructions` y `experimental_instructions_file`, que ya no existen). El
  texto se **antepone** al primer mensaje `developer` y no borra nada: el
  contrato del reviewer (2107 bytes, con backticks, comillas y acentos) volvió
  byte a byte en `codex debug prompt-input`, y el resto del prompt de Codex
  quedó intacto.
- El valor de `-c` se parsea como TOML y solo cae a texto crudo si el parseo
  falla: un contrato que empezara con `true` o `42` da **error duro**
  (`invalid type: boolean`), y uno entre comillas pierde las comillas. Por eso
  el adapter codifica el contrato como **cadena básica TOML**; así el round-trip
  es exacto y no depende de un fallback.
- Codex corre dentro de un **git worktree** (`.git` como archivo), que es donde
  vive `hoom agent --task`. Fuera de un repo git se niega con
  `Not inside a trusted directory and --skip-git-repo-check was not specified.`
  y exit 1 por stderr, sin emitir JSONL.
- No tiene tope de turnos ni de gasto: `max_turns` y `budget` no existen.

## No-goals

- **v2 de gemini y opencode.** Esta spec agrega UN provider más, que es lo que
  la prueba necesita: dos adapters distintos sosteniendo el mismo contrato. Un
  tercero no prueba nada nuevo y sí agrega superficie.
- **Orquestador, delegación entre providers, bucle de corrección.** `hoom
  review` corre el reviewer una vez por lente, con la lista de lentes calculada
  ANTES del primer run. No hay ruteo, no hay estado, no hay una segunda pasada
  decidida por lo que dijo la primera.
- **Que `hoom review` juzgue el código.** No emite veredicto, no cambia el exit
  code por hallazgos, no bloquea nada. Los hallazgos son narración calificada
  con ciclo de vida; el que bloquea es `hoom verify`.
- **Que `hoom review` re-verifique.** No corre `verify` ni `check` ni escribe un
  veredicto nuevo: el reviewer no toca el árbol certificado, y correr los gates
  del proyecto una vez por lente sería caro y redundante.
- **Cambiar los cinco pasos de `hoom agent`.** El sobre queda exactamente como
  la Spec B lo fijó, salvo el reemplazo interno de `ToolNamer` por la capacidad.
- **`--read-only` en `hoom run`.** `hoom run` sigue siendo el primitivo SIN
  política. La intención de rol la pone el sobre.
- **Pisar la config del usuario de Codex** (`approval_policy`, perfiles,
  `AGENTS.md`, MCP). hoom corre la CLI del usuario con la config del usuario;
  lo único que fija por invocación es lo que el pedido dice.
- **Mapear los tipos de item que no pude observar en un run real** (`reasoning`,
  `todo_list`, `mcp_tool_call`, `web_search`). Degradan a `text` declarado, con
  su tipo nombrado. Se mapean cuando aparezcan, no antes.
- **Reescribir `hoom status` sobre el meta de run nuevo.** El meta se agrega y
  `hoom review` lo usa; sacar el parseo de prosa de `statuscmd` es otra spec.
- **Configurar la regla de lentes desde `hoom.yaml`.** La regla es del contrato
  06 y es deterministica; quien quiera otra lente la pide con `--lens`.

## Contratos

### Paquete `internal/providers`: la intención sube al vocabulario

```go
type Capabilities struct {
    // ...campos actuales...
    Tools    bool `json:"tools"`     // allow/deny de herramientas POR NOMBRE
    ReadOnly bool `json:"read_only"` // puede imponer un rol que NO escribe
    // ...max_turns, budget...
}
```

`Names()` la nombra `read_only` justo después de `tools` — son la misma
familia: el límite de lo que el CLI puede hacer.

```go
type Request struct {
    // ...campos actuales...
    ReadOnly bool // el rol no escribe: que el provider lo imponga con lo que tenga
    Exec     bool // ...pero SI ejecuta comandos (hoom finding, tests). Sin ReadOnly no dice nada.
}

const FieldReadOnly = "read_only" // nombre canonico en Ignored y ErrUnsupported
```

`resolve` lo trata como a los demás campos: si la capacidad está, viaja; si no,
va a `Ignored`; con `Strict`, `ErrUnsupported`. Ningún adapter cambia de forma
por esto: el que no la declara, la ignora.

`ToolNamer` **se elimina**. Su único consumidor era el sobre, y el vocabulario
de herramientas vuelve adentro del adapter de Claude, que es quien lo conoce.
`AllowTools`/`DenyTools` siguen existiendo para `hoom run --allow-tools`: ahí el
humano habla el dialecto del provider a propósito.

### Paquete `internal/providers`: adapter Claude

Capacidades: suma `read_only`. `Command` traduce la intención al mismo
vocabulario que ya escribe `hoom agents --target claude`:

| rol | allow | deny |
|---|---|---|
| solo lectura | `Read, Grep, Glob` | `Edit, Write, MultiEdit, NotebookEdit, Bash` |
| solo lectura + exec | `Read, Grep, Glob, Bash` | `Edit, Write, MultiEdit, NotebookEdit` |
| escribe | — | — |

Si el llamador además mandó listas explícitas, se conservan: la del rol se suma
a la del llamador y se deduplica preservando el primer orden.

### Paquete `internal/providers`: adapter Codex v2

```go
func (codex) Capabilities() Capabilities {
    return Capabilities{
        Structured: true, Continue: true, Resume: true, SessionID: true,
        Model: true, SystemPrompt: true, ReadOnly: true,
        // Tools, MaxTurns y Budget: false. Codex no nombra herramientas ni
        // tiene tope de turnos o de gasto.
    }
}
```

Formas de invocación (el prompt SIEMPRE último, como en Claude):

```
nuevo:     codex exec --json [flags] <prompt>
resume:    codex exec resume <id> --json [flags] <prompt>
continue:  codex exec resume --last --json [flags] <prompt>
```

Flags que arma el adapter:

| campo del Request | argv |
|---|---|
| `Model` | `-m <modelo>` |
| `SystemPrompt` | `-c developer_instructions=<contrato codificado como cadena básica TOML>` |
| `ReadOnly` sin `Exec` | `-c sandbox_mode="read-only"` |
| `ReadOnly` con `Exec` | `-c sandbox_mode="workspace-write"` |
| sin `ReadOnly` | nada: manda la config del usuario |

```go
// tomlString codifica un texto arbitrario como cadena basica TOML: comilla,
// backslash y los controles van escapados y el salto de linea viaja como \n.
// Sin esto, `-c clave=<texto>` depende del fallback a literal crudo, que
// come las comillas externas y REVIENTA si el texto parsea como bool o int.
func tomlString(s string) string
```

`Normalize` (una línea de stdout → eventos hoom):

| línea | evento |
|---|---|
| `thread.started` | `start`, `SessionID` = `thread_id` |
| `turn.completed` | `end` (detalle: tokens de `usage`) |
| `turn.failed` | `error` con `error.message` |
| `{"type":"error",...}` | `error` con `message` |
| `item.started` de `command_execution` | `tool` con el comando |
| `item.started` de `file_change` | `tool` con las rutas y su tipo de cambio |
| `item.completed` de `agent_message` | `text` |
| `item.completed` de `error` | `error` |
| `item.completed` de una acción que falló (`exit_code != 0` o `status: "failed"`) | `tool` diciendo el fallo |
| `item.completed` de una acción que salió bien | nada: ya se narró al arrancar |
| item de un tipo no mapeado | `text` que NOMBRA el tipo y su campo legible |
| `turn.started`, `item.updated` | nada |
| línea que no es JSON | `text` con la línea íntegra |

La regla de fondo es la de la Spec A: **ninguna línea se pierde**; lo que no se
entiende degrada a `text` con su contenido crudo, nunca a silencio. `Normalize`
sigue siendo sin estado — por eso las acciones se narran al arrancar y solo
vuelven a aparecer si fallan: sin memoria entre líneas no hay forma de
deduplicar un `item.completed` contra su `item.started`.

### Paquete `internal/runcmd`

```go
type StartOptions struct {
    // ...campos actuales...
    Role     string // slug del rol que encarna el run ("" = hoom run, sin rol)
    ReadOnly bool
    Exec     bool
}

// Meta es la identidad DURABLE de un run: el jsonl narra, el meta identifica.
// Vive en .hoom/runs/<id>.meta.json, es telemetria local (fuera de git, fuera
// de la huella, fuera del delta del sobre) y se escribe dos veces: al arrancar
// y al cerrar.
type Meta struct {
    ID                string    `json:"id"`
    Provider          string    `json:"provider"`
    Role              string    `json:"role,omitempty"`
    Task              string    `json:"task,omitempty"`
    Dir               string    `json:"dir"`
    CreatedAt         time.Time `json:"created_at"`
    Status            string    `json:"status"` // running | done | error | canceled
    ExitCode          int       `json:"exit_code"`
    ProviderSessionID string    `json:"provider_session_id,omitempty"`
    EndedAt           time.Time `json:"ended_at,omitempty"`
}

// Metas lista los metas del proyecto, del mas nuevo al mas viejo. Un archivo
// ilegible o de otra forma se saltea: la telemetria rota nunca rompe un
// comando.
func Metas(root string) []Meta
```

`markOrphans` y `statuscmd` ya filtran por `.jsonl`, así que el sidecar no
altera ninguna lectura existente.

### Paquete nuevo `internal/reviewcmd`

```go
// Las 4 lentes del contrato 06, en orden fijo.
var Lentes = []string{"readability", "reliability", "resilience", "risk"}

// Lenses aplica la regla del contrato 06 sobre EVIDENCIA, no sobre criterio:
//   explicita           -> esa sola (invalida = error nombrando las 4)
//   solo documentacion  -> ninguna (no se lanza ningun CLI)
//   riesgo o >400 lineas-> las 4, en orden
//   resto               -> reliability, la lente del cambio estandar
func Lenses(git gitx.Info, explicit string) ([]string, string, error) // lentes, motivo

type Options struct {
    Provider     string // "" = el primero instalado que sostenga el contrato y no sea el del writer
    Lens         string
    Task         string
    Spec         string
    Model        string
    SameProvider bool // permite revisar con el mismo provider que escribio
    MaxTurns     int
    BudgetUSD    float64
}

type Pass struct {
    Lens      string                `json:"lens"`
    RunID     string                `json:"run_id,omitempty"`
    RunStatus string                `json:"run_status,omitempty"`
    SessionID string                `json:"provider_session_id,omitempty"`
    Scope     agentcmd.ScopeResult  `json:"scope"`
    Findings  []string              `json:"findings"` // ids que hoom VIO aparecer
}

type Result struct {
    Provider string   `json:"provider"`
    Writer   string   `json:"writer,omitempty"` // provider del ultimo run que escribio
    Cross    string   `json:"cross"`            // cruzada | no-cruzada | desconocida
    Reason   string   `json:"reason"`           // por que esas lentes
    Lenses   []string `json:"lenses"`
    Passes   []Pass   `json:"passes"`
    Findings []string `json:"findings"` // union de las pasadas
    Status   string   `json:"status"`   // revisado | sin-revisar | no-entregable
    ExitCode int      `json:"exit_code"`
}

func Run(root, base string, opt Options, w io.Writer) (Result, error)
```

### Los pasos de una pasada de review

`hoom review` no atraviesa los cinco pasos del sobre: usa las piezas que el
sobre ya exporta y se salta las dos que no le corresponden.

1. **lentes** — `Lenses` sobre `gitx.Snapshot`. Cero lentes cierra ahí, exit 0,
   diciendo por qué (no hay nada que revisar).
2. **cruzada** — `runcmd.Metas` da el último run de ESTE árbol cuyo rol no es de
   solo lectura: ese es el writer. El provider de la review se elige distinto;
   si el humano fuerza el mismo, hoom se niega salvo `--same-provider`.
3. **pasada por lente** — foto (`agentcmd.Take`), run del rol `reviewer` con su
   contrato como system prompt y su límite de solo lectura + exec, foto, gate de
   scope (`agentcmd.Gate`, la forma `evidencia`).
4. **hallazgos** — los ids nuevos de `.hoom/findings/` entre las dos fotos, y
   ANTES de que hoom escriba los suyos por violaciones: hoom no se cuenta a sí
   mismo como reviewer.

El pedido que recibe el reviewer es determinista y corto: base, archivos
tocados, tamaño del cambio, veredicto vigente, spec si hay, la lente asignada, y
el comando exacto de registro con `--author <rol>@<provider>` para que el
artefacto lleve su procedencia. El diff lo saca él: tiene shell.

```go
// Gate es el paso 3 del sobre, ahora compartido: dos comandos, una sola
// implementacion de "el rol escribio donde le correspondia".
func Gate(dir, base string, role agents.Role, before, after Snapshot, pol Policy) ScopeResult
```

### CLI

```
hoom review [--provider p] [--lens l] [--task slug] [--spec ruta] [--model m]
            [--same-provider] [--max-turns n] [--budget-usd x] [--json]
```

```
hoom review: reviewer (codex) sobre 12 archivos, +380/-44 lineas
  cruzada     SI - el writer corrio en claude (run 20260904T190000_ab12cd)
  lentes      reliability (cambio estandar)
  [1/1] reliability
    run       20260904T193000_77aa10 - narracion en .hoom/runs/...
    ...
    scope     3 archivos tocados, 0 fuera de scope
    hallazgos 2 nuevos: 20260904T193412_9c1a2b, 20260904T193455_04ee71
hoom review: REVISADO - 2 hallazgos nuevos (hoom finding list --open)
```

Exit: 0 si cada pasada corrió y el scope quedó limpio, con hallazgos o sin
ellos; 1 si un run falló, si el scope se violó o si la review se negó por no ser
cruzada. `--json` imprime `Result` y respeta el mismo exit.

README y el `usage` de `cmd/hoom/main.go` suman la fila de `hoom review`.

## Casos límite y errores esperados

- **Codex fuera de un repo git**: el CLI se niega por stderr con exit 1 y sin
  emitir JSONL. El run queda en `error`, la línea de stderr viaja como evento
  `text` y el mensaje del propio Codex nombra la salida
  (`--skip-git-repo-check`). hoom no la pasa por su cuenta: correr un agente
  sobre un árbol sin control de versiones es exactamente lo que hoom no avala.
- **Codex escribe `Reading additional input from stdin...` en stderr** en cada
  invocación (stdin del subproceso es `/dev/null`). Queda como un evento `text`
  con prefijo `stderr:`. Es ruido conocido, del mismo linaje que el de la Spec A.
- **Contrato que parsearía como TOML no-string** (`true`, `42`, texto entre
  comillas): la codificación como cadena básica lo vuelve irrelevante. El test
  lo fija igual, porque el fallback crudo es una trampa silenciosa.
- **`developer_instructions` en un resume**: el argv lo lleva igual. No se
  verificó que Codex lo re-aplique sobre un hilo existente; si no lo hiciera, el
  contrato ya viaja en el historial del hilo. Queda declarado en riesgos.
- **Rol de escritura en Codex**: no se manda `sandbox_mode` y vale la config del
  usuario. Si esa config es `read-only`, el rol no podrá escribir y el árbol
  quedará sin cambios: el sobre lo reporta como lo que es (scope verde, verify
  igual que antes), no como un éxito.
- **`approval_policy` distinto de `never`**: se verificó que en `exec` Codex NO
  se cuelga esperando: informa que no puede pedir permiso y cierra el turno con
  exit 0. hoom no toca esa config.
- **Provider que no puede imponer solo lectura** (gemini, opencode): aviso
  declarado y el run igual arranca, exactamente como fijó la Spec B; el gate de
  scope sigue siendo la red que no depende del CLI.
- **`hoom review` sin ningún provider instalado que sostenga el contrato**:
  error nombrando la capacidad que falta y `hoom providers`, sin crear run.
- **`hoom review` con un solo provider instalado**: no hay cruzada posible. Se
  niega nombrando el provider del writer y el escape (`--same-provider`), que
  deja la review marcada `no-cruzada` en el `Result`.
- **`hoom review` sin ningún run registrado en ese árbol** (clon fresco,
  `.hoom/runs/` borrado, código escrito a mano): `Cross: "desconocida"` y la
  review corre igual. hoom dice lo que sabe, no lo que le gustaría saber.
- **Cambio de solo documentación**: cero lentes, ningún CLI lanzado, exit 0.
  Incluye el caso de este repo: un cambio que solo toca `.hoom/specs/`.
- **Cambio vacío** (nada difiere de base): mismo camino que documentación: no
  hay nada que revisar.
- **El reviewer no registra nada**: `hallazgos 0` explícito. Una review sin
  hallazgos es información, no un error.
- **El reviewer escribe código** (el sandbox no alcanzó, o el rol corre con
  `workspace-write` por su `exec`): el gate de scope lo caza, cada violación
  deja su hallazgo `high` y la pasada cierra en rojo. Es el caso para el que el
  gate existe.
- **Un hallazgo escrito por hoom por una violación de scope**: no cuenta como
  hallazgo del reviewer. El delta se toma antes de que hoom escriba los suyos.
- **`hoom review` con las 4 lentes y la segunda pasada falla**: las pasadas
  anteriores ya dejaron sus hallazgos (append-only, no se deshacen); el
  `Result` trae las pasadas que corrieron y el exit es 1.
- **Ya hay un run activo sobre ese árbol**: `ErrBusy` intacto, antes de tocar
  nada. Un writer por tarea sigue siendo la regla, y una review no la afloja.
- **`.hoom/runs/<id>.meta.json` corrupto o de otra forma**: se saltea. La
  telemetría rota nunca rompe un comando.
- **Run que muere sin cerrar** (kill, corte de luz): el meta queda en `running`
  con exit -1. Es la verdad de lo que pasó, no un estado inventado al leerlo.

## Criterios de aceptación

- CA-148: `Capabilities.ReadOnly` existe, `Names()` la nombra `read_only`
  después de `tools` y `Summary()` la incluye; `Request.ReadOnly/Exec` viajan si
  el provider las declara, van a `Ignored` como `read_only` si no, y bajo
  `Strict` producen `ErrUnsupported` nombrando el campo. `Exec` sin `ReadOnly`
  no produce nada ni aparece en `Ignored`.
- CA-149: Claude traduce `ReadOnly`: sin `Exec` → allow `Read,Grep,Glob` y deny
  con `Edit,Write,MultiEdit,NotebookEdit,Bash`; con `Exec` → `Bash` en allow y
  fuera del deny; sin `ReadOnly` no aparece ninguna bandera de herramientas; y
  las listas explícitas del llamador se conservan, sumadas y deduplicadas. El
  test que cita CA-131 sigue existiendo y asserta este camino; `ToolNamer` ya no
  existe en `internal/providers`.
- CA-150: el sobre resuelve el límite por CAPACIDAD, no por type assertion: un
  provider que declara `read_only` recibe `ReadOnly` (y `Exec` según el rol);
  uno que no la declara produce el aviso y el run igual arranca; un rol de
  escritura nunca pide límite.
- CA-151: capacidades de codex: `structured`, `continue`, `resume`,
  `session_id`, `model`, `system_prompt` y `read_only` en verdadero; `tools`,
  `max_turns` y `budget` en falso, de modo que pedirlos los deja en `Ignored`
  en orden canónico y bajo `Strict` es `ErrUnsupported`.
- CA-152: forma base de codex: `exec` con `--json`, el prompt SIEMPRE como
  último argumento, y un prompt vacío o que empieza con `-` rechazado por la
  regla común antes de armar nada.
- CA-153: system prompt de codex: el argv trae
  `-c developer_instructions=<cadena básica TOML>` y decodificar ese valor
  devuelve el contrato **byte a byte**, incluidos comillas, backslashes, saltos
  de línea, tabs y acentos; un contrato cuyo texto crudo parsearía como bool o
  número (`true`, `42`) también sobrevive como texto.
- CA-154: sesión en codex: `ResumeID` → `exec resume <id>` con el id
  inmediatamente después de `resume` y el prompt último; `Continue` →
  `exec resume --last`; id y continue juntos → gana el id, sin duplicar
  subcomandos.
- CA-155: límite del rol en codex: solo lectura sin exec →
  `-c sandbox_mode="read-only"`; con exec → `"workspace-write"`; la misma
  bandera aparece en la forma base y en la de resume (que no tiene `-s`); un rol
  de escritura no manda `sandbox_mode`.
- CA-156: `Normalize` de codex: `thread.started` → `start` con `SessionID`;
  `turn.completed` → `end`; `turn.failed` y el `error` de nivel superior →
  `error`; `item.completed` de `agent_message` → `text`; `item.started` de
  `command_execution` y de `file_change` → `tool` con el comando y con las
  rutas; `item.completed` de `error` → `error`; `item.completed` de una acción
  con `exit_code` distinto de cero → `tool` diciendo el fallo, y con exit cero →
  ningún evento; un item de tipo no mapeado → un `text` que nombra el tipo;
  `turn.started` e `item.updated` → ningún evento; una línea que no es JSON → un
  único `text` con la línea íntegra.
- CA-157: `hoom providers` muestra las capacidades nuevas de codex. [verifica: go run ./cmd/hoom providers 2>&1 | grep -A1 '^  codex' | grep -q system_prompt] [verifica: go run ./cmd/hoom providers 2>&1 | grep -A1 '^  codex' | grep -q read_only]
- CA-158: cada run deja `.hoom/runs/<id>.meta.json` con id, provider, role,
  task, dir y created_at al arrancar, completado al cerrar con status,
  exit_code, ended_at y provider_session_id; un run que no cierra deja el meta
  en `running` con exit -1; el meta sigue fuera de git, fuera de la huella y
  fuera del delta del sobre.
- CA-159: `runcmd.Metas` devuelve los metas del más nuevo al más viejo,
  ignorando archivos ilegibles, de otra forma o ajenos; sin directorio devuelve
  vacío sin error.
- CA-160: lentes deterministas: `--lens` explícito manda y una lente inválida es
  error listando las 4; un cambio de solo documentación (incluido uno que solo
  toca `.hoom/specs/`) da cero lentes, no lanza ningún CLI y sale 0 diciendo por
  qué; rutas de riesgo o `insertions+deletions` mayor a 400 dan las 4 lentes en
  orden fijo; el resto da `reliability`. La lista se calcula antes del primer
  run y no cambia durante la review.
- CA-161: cruzada: sin `--provider` se elige el primero instalado que sostenga
  el contrato y que NO sea el provider del último run que escribió en ese árbol;
  con `--provider` igual al del writer, `hoom review` se niega con exit 1
  nombrando el run del writer y `--same-provider`, sin lanzar ningún CLI; con
  `--same-provider` corre y queda `Cross: "no-cruzada"`; sin meta previo queda
  `Cross: "desconocida"` y la review corre igual.
- CA-162: la pasada corre el rol `reviewer` con su contrato como system prompt,
  `ReadOnly` con `Exec`, y el gate de scope de la forma `evidencia`: una
  escritura fuera de `.hoom/**` es violación con su hallazgo `high` y la pasada
  cierra en rojo. El pedido nombra la lente asignada y el comando exacto de
  registro con `--author <rol>@<provider>`.
- CA-163: el resultado del reviewer son los hallazgos que hoom OBSERVA aparecer
  en `.hoom/findings/` entre las dos fotos, no lo que el CLI narra; los
  hallazgos que hoom escribe por violaciones de scope NO se cuentan como
  hallazgos del reviewer; cero hallazgos se reporta explícitamente.
- CA-164: `hoom review` no ejecuta `verify` ni `check` y no escribe ningún
  veredicto nuevo: la cantidad de archivos en `.hoom/verdicts/` es la misma
  antes y después de una review completa.
- CA-165: exit codes de `hoom review`: 0 si cada pasada corrió y el scope quedó
  limpio (haya o no hallazgos); 1 si un run falló, si hubo violación de scope o
  si se negó por no ser cruzada; con 4 lentes y una pasada fallida, el `Result`
  trae las pasadas que sí corrieron y el exit es 1. `--json` imprime el `Result`
  completo (provider, writer, cross, reason, lenses, passes con scope y
  findings, findings, status, exit_code) y respeta el mismo exit.
- CA-166: `hoom review -h` lista sus flags. [verifica: go run ./cmd/hoom review -h 2>&1 | grep -q -- -lens] [verifica: go run ./cmd/hoom review -h 2>&1 | grep -q -- -same-provider] [verifica: go run ./cmd/hoom review -h 2>&1 | grep -q -- -json]
- CA-167: compatibilidad. [verifica: go test ./...]
  `hoom run`, `hoom agent`, `hoom providers`, `hoom status`, el cockpit y el
  Studio siguen verdes; la única aserción que cambia es la del test que cita
  CA-131, que pasa al camino de la capacidad.
- CA-168: E2E opcional del adapter: con `HOOM_E2E=1` y `codex` real en PATH, el
  argv que devuelve `Command` corre con exit 0 sobre un repo git temporal y su
  stream da un `start` con `SessionID` no vacío y un `end`; y `codex debug
  prompt-input` con el mismo `-c developer_instructions` muestra el contrato al
  principio del primer mensaje `developer` sin gastar una llamada al modelo. Sin
  la variable el test se omite y jamás es requisito de `go test`.
- CA-169: E2E opcional de la review: con `HOOM_E2E=1`, `codex` en PATH y sobre
  una COPIA de este repo con un cambio plantado, `hoom review --provider codex
  --lens risk` cierra con exit 0, sin violaciones de scope, sin veredicto nuevo,
  y reporta la lista —posiblemente vacía— de hallazgos nuevos observados.

## Decisiones

- **La abstracción se dobla acá, y esa es la prueba.** Tocar el vocabulario que
  la Spec A fijó no es un accidente: es lo que pasa la primera vez que un
  segundo provider entra en serio. La alternativa era enseñarle a Codex a hablar
  en nombres de herramientas de Claude, y entonces "la capa de providers" habría
  seguido siendo Claude disfrazado. El límite de un rol es una intención
  (`ReadOnly`, `Exec`); el mecanismo es del adapter.
- **`ToolNamer` se elimina en vez de convivir.** Dos mecanismos para la misma
  pregunta divergen. La Spec B la agregó explícitamente como puente para no
  tocar la interfaz recién fijada; el puente cumplió y se levanta.
- **El contrato viaja como cadena básica TOML.** `-c clave=valor` parsea el
  valor como TOML y solo cae a crudo si falla: confiar en ese fallback es
  confiar en que ningún contrato empiece nunca por algo que parezca un bool.
  Codificar es determinista y el test lo fija con el round-trip.
- **`sandbox_mode` por `-c` en las dos formas.** `codex exec resume` no tiene
  `-s`, y un solo camino se verifica una vez. Se comprobó que surte efecto
  también en la forma base.
- **Solo se mapea lo que se vio correr.** Los tipos de item que no aparecieron
  en un run real degradan a `text` nombrando su tipo. Inventar el nombre de un
  campo que no vi sería exactamente la clase de adivinanza que hoom existe para
  no hacer.
- **La narración de acciones va al arrancar.** `Normalize` es sin estado por
  contrato, así que no puede deduplicar `item.completed` contra su
  `item.started`. Narrar al arrancar es lo que sirve en vivo; el cierre solo
  vuelve a hablar si falló, que es cuando hay noticia.
- **El meta de run existe porque la review necesita saber quién escribió.** Hoy,
  cuando el proceso termina, un run queda anónimo: `hoom status` reconstruye el
  provider parseando una frase en español. Un sidecar de telemetría local
  responde la pregunta sin adivinar prosa, y no toca ni la huella ni la
  evidencia.
- **`hoom review` no re-verifica.** El reviewer no cambia el árbol certificado:
  volver a correr los gates una vez por lente sería caro, redundante y llenaría
  `.hoom/verdicts/` de veredictos idénticos. Verify sigue siendo del writer.
- **Los hallazgos no cambian el exit code.** Un hallazgo es narración
  calificada, con ciclo de vida y un Refutador que puede tumbarlo; el que
  bloquea es el gate. Una review que encuentra tres cosas es una review exitosa.
- **La review se niega a no ser cruzada, pero deja el escape a la vista.** Que
  el mismo modelo que escribió sea el que revisa es la circularidad que hoom
  existe para romper. Quien tenga un solo CLI instalado igual puede revisar: lo
  pide con `--same-provider` y el `Result` lo dice.
- **La lente sale de la evidencia.** El contrato 06 ya declara la regla y ya
  dice de dónde sale el umbral (`insertions+deletions` del veredicto). Estaba
  escrita en un prompt; acá se ejecuta. La lente dominante del cambio estándar
  se fija en `reliability` en vez de adivinarse del contenido: quien sepa que su
  cambio es de otra naturaleza la elige con `--lens`.
- **Los hallazgos del reviewer los cuenta hoom, no el CLI.** Es la misma regla
  que el gate de scope: el árbol antes y el árbol después. Que el reviewer diga
  "registré tres hallazgos" no es evidencia de nada.
- **`hoom review` compone las piezas del sobre; no las duplica.** El gate de
  scope pasa a ser una función compartida. Dos implementaciones de "el rol
  escribió donde le correspondía" serían dos reglas distintas al tercer cambio.

## Riesgos y deuda aceptada

- **El rol de solo lectura CON exec corre en `workspace-write`.** En Codex, para
  que el reviewer pueda ejecutar `hoom finding add` hay que darle escritura en
  el workspace: el enforcement duro lo pierde justo el rol que registra
  hallazgos. Es la misma deuda que ya tienen los `.codex/agents/*.toml` que
  genera `hoom agents --target codex`, y el gate de scope post-run es quien la
  cubre. Un sandbox que permita escribir solo `.hoom/findings/` es otra spec.
- **No verifiqué que Codex re-aplique `developer_instructions` en un resume.**
  El argv lo lleva igual; si Codex lo ignorara sobre un hilo existente, el
  contrato ya está en el historial. Se declara para no venderlo como probado.
- **El adapter depende de un vocabulario JSONL sin contrato público.** Codex
  puede renombrar sus eventos en cualquier versión. La mitigación es la de
  siempre: lo desconocido degrada a `text` con la línea íntegra, así que un
  cambio de esquema empeora la narración pero no rompe el run ni pierde nada.
  Verificado contra 0.151.0 y nada más.
- **Codex no tiene tope de turnos ni de gasto.** `--max-turns` y `--budget-usd`
  simplemente no existen: bajo `Strict` un pedido con esos campos se rechaza, y
  el sobre siempre es `Strict`. Es decir: `hoom agent --budget-usd` con Codex se
  niega. Prefiero la negativa explícita a un tope que nadie impone.
- **`.hoom/runs/` es local y borrable.** La respuesta a "¿quién escribió esto?"
  vive fuera de git a propósito (es telemetría, no evidencia), así que un clon
  fresco no sabe. Por eso `desconocida` es un estado de primera clase de la
  review y no un error.
- **La regla de lentes es gruesa.** "Rutas de riesgo" es una lista de subcadenas
  y el umbral es una constante. Se equivocará por exceso en algunos repos. Es
  ruido honesto y corregible con `--lens`; la alternativa —que el modelo elija
  su propia lente— es justamente lo que el contrato 06 prohíbe.
- **Una review de 4 lentes son 4 sesiones.** El costo lo paga el humano que la
  pide, secuencialmente y sin paralelismo. No hay adaptación entre pasadas: la
  lista se fija antes de la primera. Si esto tienta a convertirse en un
  planificador, la respuesta es no.
- **22 criterios propios, más uno prestado.** CA-148 a CA-169 son de esta
  spec; CA-131 es de la Spec B y aparece a propósito: al cambiarle el mecanismo
  al límite de herramientas, su test tiene que seguir vivo y citando su token, y
  la trazabilidad de esta spec lo exige. El tamaño es el mismo que el de la Spec
  A y la Spec B, y por la misma razón: el adapter no se puede probar sin un uso,
  y el uso no existe sin el adapter. Se implementa en un worktree con su propio
  veredicto.
