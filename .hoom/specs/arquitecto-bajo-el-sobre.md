# Spec: el arquitecto bajo el sobre y el sobre que certifica la nada

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start arquitecto-bajo-el-sobre`.
Origen: dogfood del 2026-09-07. `hoom agent --role arquitecto` no pudo escribir
su spec: en `internal/agents/targets.go` el rol es `ReadOnly: true` con scope
`evidencia`, así que Claude le negó `Write`. Aun así el sobre cerró
ENTREGABLE con 0 archivos tocados, corrió verify sobre un árbol sin cambios y
escribió un veredicto verde. Costó 2.29 USD para nada. Es prerrequisito de la
cabina visual: la columna Arquitecto del tablero lanza exactamente este rol.

## Objetivo

Cuatro arreglos del sobre (`hoom agent`), del mismo tamaño y la misma raíz: el
sobre dejaba pasar como trabajo algo que no lo era.

1. **Los autores de specs escriben, y solo specs.** Revisados los tres roles
   cuyo contrato produce documentos en `.hoom/specs/`:
   - `arquitecto`: el bug del dogfood. Es de solo lectura y su contrato le
     pide escribir `.hoom/specs/<cambio>.md`.
   - `designer`: el mismo problema. Solo lectura, y su contrato le pide
     escribir el UI-spec `.hoom/specs/<cambio>-ui.md`.
   - `analista`: otro problema. Ya escribe, pero con scope `evidencia`
     (`.hoom/**`: podría reescribir los contratos de `.hoom/agents/` o los
     documentos fuente de `.hoom/intake/`, que su contrato prohíbe tocar) y con
     shell completo, aunque su contrato dice que no lee ni escribe código.

   Los tres pasan a ser roles que escriben (`ReadOnly: false`) SIN ejecutar
   comandos (`Exec: false`) y con una forma de scope nueva, `specs`, que
   permite solo `.hoom/specs/**`. Para que `Exec: false` diga algo en un rol
   que escribe, `Exec` pasa a significar lo mismo en toda la tabla: *el rol
   ejecuta comandos*. Un rol que escribe sin `Exec` recibe del provider
   escritura sin shell.

2. **Un rol que escribe y no entrega no produce veredicto.** Si el run de un
   rol con escritura termina bien sin haber cambiado nada más que la evidencia
   que hoom mismo genera, el sobre cierra con un estado propio, `sin-entrega`,
   exit 1, sin correr verify ni check, y lo deja en el registro del sobre y en
   `hoom status`. Los roles de solo lectura (scout, reviewer, refutador,
   orquestador) siguen como hoy: su entrega son hallazgos, no archivos.

3. **Un pedido vacío o de relleno no gasta un token.** `...`, `…`, espacios o
   el `<pedido>` que el propio sobre imprime en su pista de reanudación cortan
   antes de lanzar el run.

4. **Los hallazgos saben de qué tarea son.** Con `--task <slug>`, lo que el rol
   registre con `hoom finding add` queda atado a ese slug (campo opcional
   `task` en el hallazgo), propagado por la variable de entorno `HOOM_TASK`
   que hoom le pone al proceso del run, o por el flag `--task`. La cabina
   necesita saber qué hallazgos pertenecen a qué tarjeta.

Los contratos de las Specs B (`hoom-agent-sobre-determinista.md`) y D
(`test-writer-en-arbol-ciego.md`) se mantienen. Cinco criterios previos cambian
de mecanismo y se re-expresan con su token (ver Decisiones).

## No-goals

- Juzgar la CALIDAD de lo que entrega un autor de specs. El sobre certifica
  que entregó algo en su territorio y que el árbol sigue verde; el spec se
  juzga con `spec_lint` cuando el writer corre `verify --spec`, y con la
  aprobación humana.
- Quitarle el shell al test-writer. Con `NoExec` ahora sería posible, y
  cerraría bajo Claude el agujero que la Spec D declaró (`git show
  HEAD:<impl>` desde el árbol ciego), pero cambia qué puede hacer ese rol:
  decisión propia, spec propia. Acá conserva su shell.
- Apagar el shell de Codex (`features.shell_tool`). No se pudo verificar sin
  pagar una corrida real: Codex no declara la capacidad y avisa.
- `--no-exec` en `hoom run` o en `POST /api/runs`. La intención nace para el
  sobre; quien corre sin rol elige herramientas a mano.
- Detectar pedidos de relleno en `hoom run` o en el Studio.
- Un exit code distinto para `sin-entrega`. El dato que distingue es `status`
  (texto, JSON y registro).
- Validar que la tarea de un hallazgo exista, filtrar hallazgos por tarea en
  `hoom finding list` o en el Studio, o mostrarla en la lista de texto. La
  pertenencia viaja en el JSON, que es lo que la cabina lee.
- Cambiar verify, check o la huella. Los hallazgos siguen fuera de la huella
  y el campo `task` no la toca.

## Contratos

### `internal/agents`: la tabla

```go
const ScopeSpecs = "specs" // solo .hoom/specs/**: el territorio de quien escribe specs

type Role struct {
    // ...campos actuales...
    // Exec: el rol EJECUTA comandos (tests, hoom finding, hoom verify). En un
    // rol de solo lectura abre el shell sobre el set de lectura; en un rol que
    // escribe, sin Exec el provider le da escritura sin shell.
    Exec bool `json:"exec"`
}
```

| rol | ReadOnly | Exec | Scope |
|---|---|---|---|
| orquestador | sí | sí | evidencia |
| arquitecto | **no** | no | **specs** |
| designer | **no** | no | **specs** |
| scout | sí | no | evidencia |
| writer | no | **sí** | codigo |
| test-writer | no | **sí** | tests (ciego) |
| reviewer | sí | sí | evidencia |
| characterizer | no | **sí** | tests |
| analista | no | no | **specs** |
| refutador | sí | sí | evidencia |

En negrita lo que cambia. writer, test-writer y characterizer ya tenían shell;
ahora la tabla lo dice. `Desc` de arquitecto y designer deja de decir "Solo
lectura". Los contratos embebidos `01-arquitecto.md`, `02-designer.md` y
`08-analista.md` dicen que escriben SOLO en `.hoom/specs/` y que no ejecutan
comandos (el analista conserva sus tres modos de arranque).

`PolicyFor` con forma `specs`: allow `.hoom/specs/**`, sin deny. El manifiesto
lo re-apunta igual que a las otras formas (`agents.<rol>.write.allow`), y el
piso universal no se afloja.

### `internal/providers`: la intención `NoExec`

```go
type Capabilities struct {
    // ...
    NoExec bool `json:"no_exec"` // puede quitarle el shell a un rol que ESCRIBE
}

type Request struct {
    // ...
    // NoExec: el rol escribe pero NO ejecuta comandos. Bajo ReadOnly no dice
    // nada: ahi la ausencia de Exec ya lo dice.
    NoExec bool
}

const FieldNoExec = "no_exec"
```

`Names()` la nombra justo después de `read_only`. Claude la declara; Codex,
OpenCode y Gemini no. Sin la capacidad se ignora con su nombre canónico y bajo
`Strict` es `ErrUnsupported`, igual que `read_only`.

Claude con `NoExec` (sin `ReadOnly`): `--disallowedTools` incluye `Bash`; con
`Unattended`, `--allowedTools` es `Read,Grep,Glob,Edit,Write,MultiEdit,
NotebookEdit` (el set de escritura de hoy menos `Bash`). `Request.Exec` no
cambia de significado: sigue siendo el modificador de `ReadOnly`.

### `internal/agentcmd`: el sobre

```go
// NoExecFor resuelve el "sin shell" de un rol que escribe contra lo que el
// provider DECLARA, igual que ReadOnlyFor con la solo lectura: quien no
// puede imponerlo avisa, el run corre igual y el gate de scope es la red.
func NoExecFor(p providers.Provider, role agents.Role) (noExec, warn bool)

// IsPlaceholder: vacio tras quitar todo espacio en blanco, solo '.' y '…',
// o exactamente "<pedido>" sin distinguir mayusculas.
func IsPlaceholder(pedido string) bool

// Delivered: Touched menos lo que generan los comandos de hoom
// (.hoom/verdicts/**, .hoom/findings/**, .hoom/ratchet.json). Metodo, no
// campo: el JSON de ScopeResult que fijo la Spec B no cambia.
func (s ScopeResult) Delivered() []string

// Gate gana la tarea: los hallazgos de sus violaciones llevan Task.
func Gate(dir, base, task string, role agents.Role, before, after Snapshot, pol Policy, blind *Blind) ScopeResult

// Result.Status: entregable | no-entregable | sin-entrega
```

`startOptions` suma `NoExec` (vía `NoExecFor`) a lo que ya arma; `ReadOnlyFor`
no cambia. Aviso cuando el provider no puede: `aviso: <p> no puede quitarle el
shell a un rol que escribe; su escritura se verifica solo despues del run`.

Validaciones de entrada, antes de elegir provider, crear registro o run (como
hoy el pedido vacío): pedido de relleno → `el pedido %q no dice nada: escribi
lo que el rol tiene que hacer`; `--spec` con un rol de forma `specs` → `el rol
<r> escribe specs: --spec es el spec desde el que un rol trabaja y contra el
que se verifica, y un autor no se verifica contra el suyo. Nombra el spec en
el pedido`.

El paso scope, en orden:

1. `Gate` → si `Cuts()` (manipulación o aislamiento), corta como hoy.
2. Rol ciego: el trasplante, como hoy (así viajan los hallazgos que el rol
   haya creado en la cuarentena).
3. **Nuevo:** rol que escribe (`!ReadOnly`) y `Delivered()` vacío →
   `Stage: "scope"`, `Status: "sin-entrega"`, exit 1, nota `el rol <r>
   escribe y el run no dejo ningun archivo: no hay arbol nuevo que
   certificar`. No corren verify ni check.
4. verify y check, como hoy.

```
hoom agent: rol arquitecto (claude) en .hoom/worktrees/cabina-visual
  [1/5] spec    sin --spec: no se exige aprobacion ni trazabilidad
  [2/5] run     20260918T120000_ab12cd - narracion en .hoom/runs/...
  [3/5] scope   0 archivos tocados, 0 fuera de scope
hoom agent: SIN ENTREGA - el rol arquitecto escribe y el run no dejo ningun archivo: no hay arbol nuevo que certificar
```

### `internal/envelope`, `hoom status` y el Studio

`envelope.StatusNoDelivery = "sin-entrega"`: un estado terminal más
(`Done()` verdadero). `hoom status` lo pinta `SIN ENTREGA` (amarillo) con la
nota, no `NO ENTREGABLE`; `status --json` lleva `"status": "sin-entrega"`. El
Studio pinta el mismo badge.

### `internal/reviewcmd`

`writerOf` ("¿quién escribió este árbol?") saltea, además de los roles de solo
lectura, los de forma `specs`: un autor de specs no escribió el código que se
revisa, y contarlo falsearía la cruzada.

### `internal/finding` y `internal/runcmd`: la tarea del hallazgo

```go
type Finding struct {
    // ...campos actuales...
    Task string `json:"task,omitempty"` // slug de la tarea; vacio = sin tarea
}

type Draft struct{ Severity, Lens, File, Description, Author, Task string }

// Register valida la tarea con la forma de slug de 'hoom task start'
// (^[a-z0-9][a-z0-9-]*$): una tarea invalida es error y no escribe nada.
func Register(root, base string, d Draft) (Finding, error)
// Add conserva su firma: es Register sin Task.
```

```go
const EnvTask = "HOOM_TASK"

// runcmd pone HOOM_TASK=<StartOptions.Task> en el entorno del proceso del
// provider, SIEMPRE: vacio sin tarea, para no heredar una ajena.
// CurrentTask: el flag manda; si esta vacio, HOOM_TASK; si no, "".
func CurrentTask(flag string) string
```

CLI: `hoom finding add [--task slug] ...`; sin flag toma `HOOM_TASK`. Así, un
rol que corre `hoom finding add` dentro de un run con tarea la hereda sin que
su contrato cambie, y el gate de scope ata la tarea a los hallazgos que crea.
Vale para toda corrida con tarea: el sobre, `hoom review` y `hoom run`.

## Casos límite y errores esperados

- **El writer solo corrió `hoom verify`**: el veredicto que creó queda (es
  append-only y es lo que hizo), pero no es entrega → `sin-entrega`. Igual si
  solo registró hallazgos o apretó el trinquete.
- **Rol que escribe y solo tocó rutas fuera de su scope** (no evidencia): hay
  cambios, así que no es `sin-entrega`; verify y check corren y cierra
  no-entregable en scope, como fijó la Spec B.
- **Autor de specs que solo registró un hallazgo** (posible bajo Codex): el
  hallazgo es fuera-de-scope para la forma `specs` y además no es entrega →
  `sin-entrega`, con la violación y su hallazgo del gate igual registrados.
- **Manipulación o aislamiento roto sin nada entregado**: corta primero la
  manipulación; `sin-entrega` nunca tapa un corte.
- **Test-writer ciego que no escribió tests**: `sin-entrega` tras el
  trasplante; la cuarentena se cierra porque no quedó nada adentro.
- **Árbol ya sucio antes del run y el run no agrega nada**: `sin-entrega`. El
  sobre certifica lo que hizo ESTE run; para certificar el árbol existe
  `hoom verify`.
- **Edición devuelta a su contenido original**: delta vacío → `sin-entrega`.
- **Analista en Modo C** (sin intake y sin código): pide la entrevista y no
  escribe → `sin-entrega`. Correcto: sin respuestas no hay visión.
- **El arquitecto edita un spec ya aprobado**: la aprobación queda invalidada
  por hash, como siempre. El sobre no la protege ni la renueva.
- **Arquitecto bajo Codex**: aviso de que no puede quitarle el shell; el
  sandbox `workspace-write` lo deja ejecutar; lo que escriba fuera de
  `.hoom/specs/**` lo caza el gate.
- **`hoom.yaml` con `agents.arquitecto.write.allow: ["**"]`**: el territorio
  se amplía (mecanismo de la Spec B); el shell sigue negado, porque sale de
  `Exec` en la tabla y no del scope.
- **Pedidos que NO son relleno**: `revisa ...`, `?`, `.gitignore` → pasan.
- **`HOOM_TASK` exportado en el shell del humano**: `hoom finding add` lo
  toma; es una elección explícita. Dentro de un run manda el que pone hoom.
- **`--task` explícito distinto de `HOOM_TASK`**: gana el flag. `task` es una
  etiqueta de pertenencia, no evidencia: verify y check no la leen.
- **Binario `hoom` viejo en el PATH del rol**: ignora `HOOM_TASK` y registra
  el hallazgo sin tarea; no rompe. Por eso la vía por defecto es la variable
  y no el flag, que un binario viejo rechazaría.

## Criterios de aceptación

- CA-228: tabla de roles: arquitecto, designer y analista quedan con
  `ReadOnly` falso, `Exec` falso y `Scope` `specs`; writer, test-writer y
  characterizer con `Exec` verdadero; orquestador, scout, reviewer y
  refutador sin cambios; `ScopeSpecs == "specs"`.
- CA-229: los contratos embebidos de arquitecto, designer y analista nombran
  `.hoom/specs/` como el único lugar donde escriben y dicen que no ejecutan
  comandos; ni ellos ni la `Desc` de arquitecto y designer dicen "Solo
  lectura".
- CA-230: `PolicyFor` con forma `specs` permite exactamente `.hoom/specs/**`:
  `CheckScope` acepta `.hoom/specs/x.md` y `.hoom/specs/sub/y-ui.md` y marca
  fuera-de-scope `internal/x.go`, `.hoom/agents/01-arquitecto.md`,
  `.hoom/intake/doc.md` y un hallazgo nuevo en `.hoom/findings/`; un `allow`
  declarado en `hoom.yaml` lo reemplaza.
- CA-231: `Capabilities.NoExec` se nombra `no_exec` después de `read_only`;
  Claude la declara y Codex, OpenCode y Gemini no. Claude con `Unattended` y
  `NoExec` pone en `--allowedTools` `Edit` y `Write` pero no `Bash`, pone
  `Bash` en `--disallowedTools` y deja el prompt último; `NoExec` con
  `ReadOnly` produce el mismo argv que `ReadOnly` solo; un provider sin la
  capacidad la ignora como `no_exec` y bajo `Strict` devuelve
  `ErrUnsupported` que la nombra.
- CA-232: `NoExecFor`: un rol que escribe sin `Exec` bajo Claude → `true`
  sin aviso; bajo Codex → `false` con aviso, el sobre imprime el aviso y el run
  arranca; el writer y los roles de solo lectura → `false` sin aviso. El sobre
  del arquitecto bajo Claude arma `NoExec` y su argv final permite `Write` y
  prohíbe `Bash`; el del writer conserva `Bash`.
- CA-233: `hoom agents --target claude,opencode,gemini`: los subagentes de
  arquitecto, designer y analista quedan con escritura y sin shell (Claude
  `tools: Read, Grep, Glob, Edit, Write, MultiEdit`; OpenCode `edit: allow` y
  `bash: deny`; Gemini con `write_file` y `replace` y sin `run_shell_command`
  ni `"*"`); los de writer, test-writer y characterizer salen como hoy.
- CA-234: `--spec` con un rol de forma `specs` devuelve error antes de todo:
  el provider nunca se invoca y no quedan run ni registro de sobre; el mensaje
  nombra el rol y pide nombrar el spec en el pedido.
- CA-235: `hoom review` no cuenta a un autor de specs como el writer: con un
  run de writer (codex) y uno posterior de arquitecto (claude) en el mismo
  directorio, el writer que la review reconoce es el de codex.
- CA-236: `Delivered()` devuelve `Touched` sin `.hoom/verdicts/**`,
  `.hoom/findings/**` ni `.hoom/ratchet.json`, en el mismo orden; el JSON de
  `ScopeResult` conserva exactamente sus claves (touched, violations,
  tampering, ok).
- CA-237: sin entrega: un writer cuyo run sale 0 sin cambiar el árbol cierra
  `Stage` scope, `Status` `sin-entrega`, exit 1, sin `VerdictID`, sin `Check`,
  sin archivos nuevos en `.hoom/verdicts/`, y la salida dice `SIN ENTREGA` con
  la nota; lo mismo si el run solo creó un veredicto o solo un hallazgo.
- CA-238: sin entrega en el árbol ciego: un test-writer que no escribió tests
  cierra `sin-entrega` con la cuarentena cerrada (`Kept` falso); un hallazgo
  que el rol creó adentro llega al árbol real.
- CA-239: lo que no cambia: un scout que no toca nada cierra entregable con
  veredicto como hoy; un writer que solo tocó una ruta prohibida que no es
  evidencia corre verify y check y cierra no-entregable en scope; una
  manipulación sin nada entregado corta como manipulación, no como
  `sin-entrega`.
- CA-240: registro y vistas: el registro del sobre cierra con `Status`
  `sin-entrega`, `Stage` scope, exit 1, nota no vacía, `EndedAt` y `Done()`
  verdadero; `hoom status` lo pinta `SIN ENTREGA` con la nota y no
  `NO ENTREGABLE`; su JSON lleva `sin-entrega`; `--json` del sobre lleva
  `status` `sin-entrega` y el mismo exit; el Studio tiene su badge.
- CA-241: `IsPlaceholder` es verdadero para `""`, `"   "`, `"..."`, `"…"`,
  `". . ."`, `" \n...\t"`, `"<pedido>"` y `"<PEDIDO>"`, y falso para
  `"revisa ..."`, `"?"` y `".gitignore"`; `Run` con un pedido de relleno
  devuelve error sin invocar al provider y sin dejar run ni registro.
- CA-242: `finding.Register` con `Task` escribe `"task": "<slug>"` en el
  artefacto y `List`/`JSONBytes` lo devuelven; `Add` no escribe la clave
  `task`; una tarea con forma inválida (`../x`, `Mayus`, `con espacio`) es
  error y no escribe nada.
- CA-243: el proceso del provider recibe `HOOM_TASK` igual a la tarea del
  run, y vacío en un run sin tarea aunque el proceso padre la tuviera;
  `CurrentTask`: el flag gana, sin flag manda `HOOM_TASK`, sin ambos `""`.
- CA-244: con `--task`, el hallazgo que el gate de scope crea por una
  violación lleva esa tarea, en el sobre y en `hoom review`; sin `--task` no
  lleva ninguna.
- CA-245: `hoom finding add` declara `--task`. [verifica: go run ./cmd/hoom finding add -h 2>&1 | grep -q -- -task]
- CA-246: E2E opcional: con `HOOM_E2E=1` y `claude` real en PATH, `hoom agent
  --role arquitecto --max-turns 3 --budget-usd 1` sobre una copia de este
  repo, con un pedido que manda escribir un spec trivial, cierra entregable y
  `Delivered()` contiene ese spec; sin la variable el test se omite.
- CA-247: compatibilidad: todo lo demás sigue verde. [verifica: go test ./...]

## Decisiones

- **Forma `specs` en vez de reusar `evidencia`.** `evidencia` es `.hoom/**`:
  contratos de rol, intake, hallazgos. El territorio de un autor es el spec;
  que sea exactamente eso lo prueba el gate.
- **`Exec` significa lo mismo en toda la tabla.** Hoy `Exec: false` en un rol
  que escribe no dice nada: el writer lo tiene falso y usa shell. Poner
  "arquitecto: Exec false" sin cambiar eso sería una tabla que miente.
  Alternativa descartada: deducir el "sin shell" de la forma `specs`; ata dos
  preguntas distintas (dónde escribe, qué ejecuta) y deja `Exec` decorativo.
- **`NoExec` es una intención nueva y aditiva en el Request**, no un cambio de
  `Request.Exec`. CA-148 fija que `Exec` sin `ReadOnly` no dice nada, y
  `hoom run --unattended` da lectura, escritura y shell sin rol: redefinir
  `Exec` rompería ambos. El valor cero conserva el comportamiento de hoy.
- **`NoExec` se resuelve por capacidad, como la solo lectura.** Quien no puede
  imponerlo avisa y el run corre: el gate de scope no depende de ningún CLI.
- **El analista se angosta a `specs`.** Sus dos salidas viven ahí. Guardar la
  entrevista en `.hoom/intake/` es un acto del humano; un proyecto que lo
  quiera del rol lo declara en `agents.analista.write.allow`.
- **`--spec` se rechaza para los autores, no se reinterpreta.** Para el resto
  significa "gate de aprobación + verify contra el spec", y un spec que se
  está escribiendo no está aprobado ni trazado: verify saldría rojo por
  construcción. La Spec B decía que el arquitecto no exige aprobación porque
  era de solo lectura; el motivo real (escribe el spec que todavía no está
  aprobado) queda como regla. CA-132 no cambia: su rol de solo lectura es el
  scout.
- **`sin-entrega` es un estado, no una violación.** No crea hallazgo: nadie
  rompió una regla, simplemente no hay nada que certificar. Exit 1: el 2 ya
  lo usa el parser de flags y el exit del provider puede ser cualquiera; lo
  que distingue es `status`.
- **Entrega = cambios que no son evidencia generada por hoom.** Correr
  `hoom verify` no es entregar; es exactamente cómo se fabricaba el veredicto
  de la nada. Las rutas fuera de scope SÍ cuentan como cambio: la Spec B fijó
  que fuera-de-scope es información y verify corre igual (CA-141).
- **`sin-entrega` va después del corte y del trasplante.** La manipulación
  siempre gana, y un hallazgo que el test-writer ciego haya creado no se
  pierde con la cuarentena.
- **Pedido de relleno = validación de entrada, sin registro**, como el pedido
  vacío o un rol desconocido. `<pedido>` entra en la regla porque el propio
  sobre lo imprime en la pista de reanudación.
- **La tarea viaja por entorno, con flag que manda.** El entorno no cambia
  ningún contrato de rol y sobrevive a un `hoom` viejo; se pone en `runcmd`,
  un solo lugar para el sobre, la review y `hoom run`, y siempre (vacío sin
  tarea) para no heredar una ajena.
- **La tarea se valida por forma, no por existencia.** Desde el worktree de
  una tarea el registro de tareas vive en otro directorio, y un hallazgo
  sobrevive al `hoom task done` que borra el worktree.
- **`Register` + `Draft` en vez de un octavo string en `Add`.** `Add` queda
  con su firma y CA-53 a CA-60 no se tocan.
- **Re-expresados con su token** (precedente: CA-131 en la Spec C):
  - CA-127 (Spec B): arquitecto, designer y analista pasan de `evidencia` a
    `specs` en la tabla de scopes que asegura.
  - CA-143 (Spec B): los casos "veredicto rojo" y "todo verde" usaban un
    writer que no tocaba nada; ahora el writer falso entrega un archivo. Lo
    que asegura, el exit por paso, no cambia.
  - CA-170 (Spec D): la guarda de flags del test-writer pasa de `Exec` falso a
    verdadero: conserva el shell que siempre tuvo, ahora nombrado.
  - CA-182 (Spec D): el test-writer falso, además de volcar sus argumentos,
    escribe un test; sin entrega no habría pasos 5 y 6 que mostrar.
  - CA-204 (Spec E): el "camino completo" usa un writer que entrega un
    archivo.
  CA-131 no cambia: `ReadOnlyFor` sigue devolviendo lo mismo para el writer.

## Riesgos y deuda aceptada

- **Codex no le quita el shell a un rol que escribe.** Avisa, y el gate de
  scope cubre las escrituras; lo que el rol ejecute sin escribir no lo ve
  nadie. `features.shell_tool=false` existe en Codex 0.151.0 pero no se
  verificó.
- **Copias locales de los contratos.** Un proyecto que ya corrió `hoom agents`
  tiene `.hoom/agents/01-arquitecto.md` que dice "Solo lectura", y `Contract`
  prefiere la copia: el modelo leería el límite viejo aunque pueda escribir.
  Acción: `hoom agents` de nuevo (reinstala los contratos, pisando ediciones
  locales, como hoy).
- **El veredicto del autor corre todos los gates por un markdown.** Honesto
  pero lento; no juzga el spec (ver no-goals).
- **Una entrega trivial pasa.** El sobre mide que algo se entregó en el
  territorio, no que sirva.
- **`HOOM_TASK` bajo Codex depende de su `shell_environment_policy`.** Por
  defecto hereda el entorno salvo nombres con KEY/SECRET/TOKEN; no se
  verificó con una corrida real. Si se pierde, el hallazgo queda sin tarea;
  el flag sigue disponible.
- **Binario instalado viejo.** Los hallazgos salen sin tarea hasta reinstalar
  `hoom` tras el merge.
- **20 criterios y cinco re-expresiones.** Cuatro arreglos chicos que tocan el
  mismo sobre; separarlos haría cuatro specs que re-expresan los mismos tests.
