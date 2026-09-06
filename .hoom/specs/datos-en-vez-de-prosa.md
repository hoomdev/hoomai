# Spec: datos en vez de prosa

Estado: BORRADOR — pendiente de aprobación humana.
Trabajo en el worktree `small-items` (rama `smallitems`).
Origen: los cuatro ítems chicos que quedaron anotados al cerrar las Specs A–D
—el sobre visible en `hoom status` y el Studio, un kind propio para los
mensajes `system` de Claude, costo y turnos como metadata del run, y qué hace
`Input` con `Strict` en un provider sin continuación—. Son cuatro pedidos
distintos con una sola raíz, y por eso entran en una spec y no en cuatro.

## Objetivo

hoom ya sabe casi todo lo que estos cuatro ítems piden. El problema es cómo lo
guarda: **como prosa, o como nada**.

- `hoom status` quiere decir con qué provider corre un run, y lo averigua con
  una expresión regular sobre una frase en español que hoom mismo escribió en
  el log (`^run \S+: (\S+) en `). El sidecar `<id>.meta.json` que la Spec C
  creó justamente para responder "¿quién escribió este árbol?" está ahí al
  lado, sin leerse.
- El sobre —`hoom agent`, lo que de verdad corre un rol— no deja rastro que
  nadie pueda mirar mientras corre. Su `Result` se imprime y se va. Desde otra
  terminal, `hoom status` ve un run anónimo; no sabe que es el paso 3 de 6 de
  un sobre con rol `writer` sobre un spec aprobado, ni que el árbol es ciego.
  El Studio ni siquiera ve el run: lista los de su propio proceso.
- El costo de un run existe: Claude lo reporta en su línea `result`. hoom lo
  descarta. Cuando el costo aparece, aparece como prosa dentro del texto de un
  evento (`"turn completado (17408 tokens de entrada, 5 de salida)"`), que es
  el mismo pecado que la regex del provider, con los papeles cambiados.
- Las líneas con las que la CLI habla de sí misma —hooks, compactación,
  rate limit— caen al log **como JSON crudo**, indistinguibles de lo que dijo
  el agente. En una corrida trivial medida hoy son 9 de 12 líneas.
- Y `Input` con `Strict` sobre un provider que no puede continuar hace algo
  que nadie decidió: falla con el error genérico de campo no soportado. Es un
  accidente, no una semántica.

Esta spec convierte esas cinco cosas en datos: un registro durable del sobre
que `status` y el Studio leen, un `usage` acumulado por run que sale del
stream y no de la prosa, un kind `system` para el ruido de la CLI, y una regla
decidida —con su mensaje y su test— para `Input` bajo `Strict`.

El hilo es el de siempre en hoom: **lo que se puede probar se guarda como
dato; lo que no se sabe se etiqueta; nada se adivina.**

## No-goals

- **Reanudar un run ajeno desde el Studio.** Un run que otro proceso arrancó
  se va a poder VER (sus sidecars y su jsonl están en disco) y no se va a
  poder continuar ni cancelar: continuar exigiría reconstruir las
  `StartOptions` desde el sidecar y adoptar una sesión de otro proceso. Es una
  feature con su propia spec; acá el run ajeno es de solo lectura, y la UI lo
  dice.
- **Un verbo `hoom envelopes` / `hoom agent list`.** El registro del sobre se
  lee desde `hoom status` y desde el Studio. Un verbo propio para navegar la
  historia de sobres es otra cosa.
- **Historial de costo, presupuesto por proyecto o alertas de gasto.** Se mide
  y se guarda por run. Sumar por tarea, por rol o por día —y decidir qué hacer
  cuando el número crece— es una spec de economía, no de telemetría.
- **Traducir el costo a otra moneda o estimarlo cuando el provider no lo
  reporta.** Codex no reporta costo: hoom dice "sin dato", no multiplica
  tokens por una tabla de precios que quedaría vieja.
- **Interpretar el contenido de los hooks.** El kind `system` dice "esto lo
  dijo la CLI de sí misma" y muestra lo que la línea trae. hoom no lee la
  salida de un hook para sacar conclusiones.
- **Cambiar el vocabulario de `Capabilities` ni agregar una capacidad
  `cost`.** El costo no es una capacidad que se pida: es un dato que el stream
  trae o no trae, y el `usage` ausente ya lo dice todo.
- **Tocar el formato de los veredictos ni de los hallazgos.** Nada de lo que
  esta spec agrega viaja en Git: es telemetría local, como `.hoom/runs/`.
- **`hoom run --input` en la CLI.** `Input` sigue siendo API del Manager y del
  Studio; esta spec fija su semántica y la prueba, no le agrega un verbo.

## Contratos

### Lo que se verificó a mano (2026-09-06, este Mac)

Contra **Claude Code 2.1.263** y **Codex CLI 0.151.0**, con corridas reales
guardadas antes de escribir una línea de código:

- Una corrida trivial de `claude -p --output-format stream-json --verbose`
  emite **12 líneas: 4 `system/hook_started`, 4 `system/hook_response`,
  1 `system/init`, 1 `assistant`, 1 `rate_limit_event` y 1 `result`**. Hoy,
  8 de esas 12 caen al log de hoom como JSON crudo dentro de un evento `text`.
- `system/hook_started` trae `hook_id`, `hook_name` (`"SessionStart:startup"`),
  `hook_event`, `uuid`, `session_id`. `system/hook_response` agrega `output`,
  `stdout`, `stderr`, `outcome` (`"success"`) y **`exit_code` como STRING
  (`"0"`, no `0`)**: el adapter no puede decodificarlo como entero.
- `rate_limit_event` es un `type` de primer nivel (no un subtype de `system`):
  trae `rate_limit_info` con `status`, `rateLimitType`, `resetsAt` y
  `unifiedWindows` (`five_hour`, `seven_day`, cada una con `utilization` y
  `resetsAt`), más `uuid` y **`session_id`** — que no debe abrir sesión.
- La línea `result` trae `num_turns`, `duration_ms`, `duration_api_ms`,
  `total_cost_usd`, `session_id`, `is_error`, `subtype`, `result` y un
  `usage` con `input_tokens`, `output_tokens`, `cache_read_input_tokens` y
  `cache_creation_input_tokens` (más `modelUsage` con el mismo `costUSD`).
- **Los valores del `result` son POR INVOCACIÓN, no acumulados por sesión.**
  Medido: la primera invocación cerró con `num_turns: 1` y
  `total_cost_usd: 0.111583`; el `--resume` de esa misma sesión cerró otra vez
  con `num_turns: 1` y `total_cost_usd: 0.1120905` —no ~0.223—, con el mismo
  `session_id`. Por lo tanto un run de varias invocaciones **suma**.
- El `--resume` reemite los 4 hooks y un `system/init` nuevo: un run con
  `Input` tiene más de un `start`. Es comportamiento previo y no cambia.
- Un turno trivial costó **0.11 USD** en este entorno (4 hooks de
  `SessionStart` y MCP inflan el contexto inicial): el dato de costo por run
  no es decorativo.
- Codex cierra con `turn.completed` y un `usage` de
  `{input_tokens, cached_input_tokens, cache_write_input_tokens,
  output_tokens, reasoning_output_tokens}` — medido `17408 / 11264 / 0 / 5 / 0`
  en una corrida trivial— y **ningún campo de costo, en ninguna línea**.

### El evento gana un kind y un usage

`providers.Event.Kind` pasa a ser `start | text | tool | agent | system | end
| error`. `system` significa exactamente una cosa: **la CLI hablando de sí
misma**, no el agente trabajando.

La regla de traducción no cambia de espíritu —nada se pierde, nada se
inventa— pero se vuelve más fina:

- `type:"system"`, `subtype:"init"` → sigue siendo el ÚNICO `start`, con su
  `session_id`.
- `type:"system"` con un subtype que hoom sabe leer (`hook_started`,
  `hook_response`) → `system` con un detalle legible que nombra el hook y, en
  la respuesta, su `outcome` y su exit.
- `type:"system"` con un subtype que hoom NO conoce → `system` con la línea
  íntegra como detalle: el kind ya dice que es ruido de la CLI, y el contenido
  no se pierde por no haberlo sabido leer.
- `type:"rate_limit_event"` → `system` con las ventanas que la línea traiga.
  No abre sesión aunque traiga `session_id`.
- Un `type` de primer nivel desconocido → sigue siendo `text` con la línea
  íntegra. hoom no sabe si es plomería o narración, y en la duda la muestra.

`Event` gana `Usage *Usage` (opcional, `omitempty`), que viaja como viaja hoy
`SessionID`: en los eventos de cierre.

```go
type Usage struct {
    CostUSD      *float64 `json:"cost_usd,omitempty"` // nil = el provider no lo reporta
    Turns        int      `json:"turns,omitempty"`
    InputTokens  int      `json:"input_tokens,omitempty"`
    CachedTokens int      `json:"cached_input_tokens,omitempty"`
    OutputTokens int      `json:"output_tokens,omitempty"`
    DurationMS   int      `json:"duration_ms,omitempty"`
}
```

`CostUSD` es puntero por una razón de identidad: **0 y "no sé" no son lo
mismo**, y Codex es exactamente el caso donde confundirlos sería mentir con un
número.

### El run acumula

`runcmd` suma los `Usage` que ve pasar (la regla se apoya en el hecho medido:
los valores son por invocación) y guarda el acumulado en dos lugares: `Run`
—que ya viaja por la API y por `hoom status`— y el sidecar
`.hoom/runs/<id>.meta.json`, que es lo único que sobrevive al proceso. Si
ningún evento reportó costo, el total queda **ausente**, no en cero.

### El sobre deja registro

Paquete nuevo `internal/envelope`, con la misma forma que ya tienen `live`,
`verdict` y `finding`: un tipo, un `Write` best-effort, un `List` que ordena
más nuevo primero y saltea lo ilegible.

```
.hoom/envelopes/<id>.json
```

El registro guarda lo que el sobre sabe en cada momento: `id`, `role`,
`provider`, `task`, `dir`, `spec`, `approval`, `run_id`, `isolated`,
`verdict_id`, `verdict`, `usage`, `stage`, `step`/`steps`, `status`
(`en curso | entregable | no-entregable`), `exit_code`, `note`, `started_at`,
`updated_at`, `ended_at`.

Se escribe en **cada transición de paso** y al cerrar. Mientras el run narra
—el paso largo— el sobre toca `updated_at` como mucho **cada 5 segundos**: un
latido, no un archivo por evento.

Es telemetría, no evidencia: vive fuera de Git (`.hoom/.gitignore`), fuera del
candidato de cambio (`gitx.excludedFromCandidate`), fuera de la huella y fuera
del delta que el gate de scope mide (`agentcmd` `noise`). Escribirlo nunca
puede romper el sobre que describe.

### `status` lee sidecars, no frases

`activeRuns` deja de sacar el provider de una frase en español. La identidad
del run —provider, rol, tarea, directorio, aislamiento, costo— sale del
sidecar; el jsonl sigue respondiendo lo único que solo él sabe: cuántos
eventos hubo, cuándo fue el último y a qué subagente delegó. Son dos cosas
distintas y se muestran distinto: **el rol del sobre** (quién es el agente) y
**el subagente delegado** (a quién llamó adentro).

Un run sin sidecar —un log viejo— se sigue listando, con el provider
desconocido. Perder un dato es mejor que inventarlo.

El `Snapshot` gana una sección `envelopes`: los sobres en curso con su paso
`n/N`, y el último cerrado con su resultado. Un sobre en curso sin actividad
hace más de 15 minutos se etiqueta **"posible huerfano"** — la misma regla,
el mismo umbral y la misma palabra que ya usa `verify`. Nunca se lo declara
muerto: hoom no mata lo que no vio morir.

### `Input` con `Strict`: la regla que faltaba

`Input` continúa la sesión de un run terminado. El provider elige el mecanismo
más fuerte que soporte: `--resume <id>` si hoom capturó la sesión, si no
`--continue`, si no una invocación nueva. Esa última rama es la que no estaba
decidida.

Queda decidido así: **con `Strict`, una invocación nueva no es continuar.**
Un `--model` que se cae es un flag perdido; una sesión que se cae es que el
modelo **empieza de cero y no recuerda nada de lo anterior**, que es
precisamente la degradación silenciosa que `Strict` existe para prohibir. Así
que `Input` con `Strict` sobre un provider sin resume ni continue se niega,
con un error de dominio que nombra al provider y la acción, y **deja el run
exactamente como estaba**: estado terminal, directorio libre, sin evento
nuevo, sin proceso lanzado.

Sin `Strict` el comportamiento se mantiene —invocación nueva— pero con **una**
línea de aviso en vez de dos, y que dice lo que importa: que el contexto
anterior no viaja.

## Casos límite

- **Run sin sidecar** (log de una versión anterior, o sidecar borrado a mano):
  se lista con lo que el jsonl prueba y el provider en `?`.
- **Sidecar ilegible o de otra forma** (JSON roto, campo `id` vacío): se
  saltea. Telemetría rota nunca rompe un comando — la regla que ya cumple
  `Metas`.
- **Sobre que corta en el paso 1** (spec sin aprobación vigente): el registro
  igual existe, con `stage: "spec"`, `status: "no-entregable"` y la nota. Un
  sobre que no llegó a correr nada es exactamente el que hay que poder mirar.
- **Sobre muerto a mitad** (Ctrl-C, terminal cerrada): el registro queda "en
  curso" para siempre. `status` lo etiqueta "posible huerfano" pasados 15
  minutos sin actividad, y no lo reescribe: quien no vio el final no lo cuenta.
- **Dos sobres a la vez** sobre tareas distintas: dos registros, los dos
  visibles. `status` los ordena por `started_at`, más nuevo primero.
- **Provider que no reporta costo** (Codex): `usage` con tokens y sin
  `cost_usd`; las superficies dicen "sin dato de costo".
- **Provider sin stream estructurado** (gemini, opencode): sin `usage` en
  absoluto; el run cierra sin costo y sin turnos, y eso se muestra como
  ausencia, no como ceros.
- **Run con varias invocaciones**: `Input` suma sobre lo que ya había. Un
  `Input` que falla no suma nada.
- **`exit_code` de un hook como string** (`"0"`): el adapter lo lee como texto
  y lo muestra; no falla el parseo por esperar un número.
- **Línea `system` sin `subtype`**: cae en la rama del subtype desconocido,
  con la línea íntegra.
- **`result` con `is_error: true`** (por ejemplo `error_max_budget_usd`): el
  costo YA se gastó, así que el `usage` se cuenta igual. Un run que falló caro
  tiene que poder decir cuánto costó fallar.
- **Run ajeno abierto en el Studio**: se ven sus eventos leídos del jsonl y no
  se ofrece ni cancelar ni continuar.
- **`.hoom/envelopes/` inexistente**: `List` devuelve vacío, no error. Un
  proyecto que nunca corrió un sobre no tiene por qué tener el directorio.

## Criterios de aceptación

- CA-192: `Event.Kind` admite `system` y el vocabulario queda documentado
  (`start | text | tool | agent | system | end | error`). En el adapter de
  Claude, `system/init` sigue siendo el ÚNICO `start` con su `session_id`, y
  cualquier otro subtype produce UN evento de kind `system` —ya no `text`—.
  Re-expresa **CA-112**, que fijaba la degradación a `text`: su test conserva
  el token y cambia la aserción, como ya hizo la Spec C con un criterio
  de la Spec B.
- CA-193: detalles legibles en vez de JSON crudo: `hook_started` nombra el
  hook; `hook_response` nombra el hook, su `outcome` y su exit leído como
  STRING (`"0"`, verificado) sin romper el parseo; un subtype desconocido —o
  ausente— produce igual un `system` con la línea íntegra como detalle.
- CA-194: `rate_limit_event` deja de ser JSON crudo: kind `system` con las
  ventanas que la línea traiga (`five_hour`, `seven_day` y su utilización),
  sin inventar las que falten, y NO abre sesión aunque traiga `session_id`.
- CA-195: el escenario (`runcmd.Stage`) no cuenta los `system` como actos del
  orquestador —hoy 8 líneas de hooks le inflan la cuenta—; `markOrphans` y
  `activeRuns` siguen tratando como terminal solo `end` y `error`, así que un
  run cuyo último evento es `system` sigue activo.
- CA-196: superficies del ruido: el feed del Studio distingue los `system`
  (clase propia, atenuados) y ofrece ocultarlos sin recargar; la mirror del
  sobre y de `hoom run` los imprimen alineados y legibles, sin JSON crudo.
- CA-197: `providers.Usage` y `Event.Usage`: el adapter de Claude los llena
  desde `result` —en `success` y también cuando `is_error`— con costo, turnos,
  tokens de entrada, de caché leído y de salida, y duración; `CostUSD` es
  puntero y queda `nil` cuando el provider no reporta costo, jamás 0.
- CA-198: el adapter de Codex llena `Usage` desde `turn.completed` con
  `input_tokens`, `cached_input_tokens`, `output_tokens` y 1 turno, y sin
  costo; el `Detail` del evento de cierre deja de llevar los números en prosa.
- CA-199: `runcmd` acumula por run: dos invocaciones (Start + Input) suman
  costo, turnos y tokens —la regla se apoya en el hecho medido de que Claude
  reporta por invocación—; si ningún evento reportó costo, el total queda
  ausente y no en cero.
- CA-200: el acumulado es durable: `<id>.meta.json` guarda `usage`, `Run.Usage`
  viaja por `Get`/`List`/`Events`, y el JSON omite lo que no se midió.
- CA-201: superficies del costo: `hoom run` cierra con una línea de costo,
  turnos y tokens que dice "sin dato de costo" cuando corresponde, y
  `hoom agent` la imprime en el paso `run` e incluye `usage` en su `--json`.
- CA-202: `internal/envelope`: `Write` es best-effort (un directorio no
  escribible no rompe nada), `List` devuelve más nuevo primero, saltea
  archivos ilegibles, de otra forma o sin `id`, y devuelve vacío —no error—
  cuando el directorio no existe.
- CA-203: el registro es telemetría, no evidencia: `.hoom/.gitignore` contiene
  `envelopes/` (y `hoom init` lo escribe junto con `isolated/`, que hoy le
  falta), `.hoom/envelopes/**` queda fuera del candidato de cambio, de la
  huella y del delta del gate de scope, y escribir sobres no cambia el
  resultado de `hoom check`.
- CA-204: el sobre registra cada transición —`spec`, `aislar`, `run`, `scope`,
  `verify`, `check` y el cierre— con rol, provider, tarea, directorio, spec,
  aprobación, `run_id`, aislamiento, veredicto, `usage`, `stage`, paso `n/N`,
  estado, exit y nota; el registro existe también cuando el sobre corta en el
  paso 1 sin haber lanzado ningún run.
- CA-205: latido: mientras el run narra, `updated_at` avanza como mucho una
  vez cada 5 segundos, sin escribir un archivo por evento.
- CA-206: `hoom status` muestra los sobres: los que están en curso con rol,
  provider, paso `n/N` y hace cuánto se movieron, y el último cerrado con su
  resultado; uno en curso sin actividad hace más de 15 minutos se etiqueta
  "posible huerfano" —misma regla, umbral y palabra que `verify`— y nunca se
  lo declara muerto.
- CA-207: `status` deja de adivinar: el provider y el rol salen del sidecar
  del run y la regex sobre la frase en español desaparece; un run sin sidecar
  se lista igual con provider desconocido; el rol del sobre y el subagente
  delegado se muestran como dos datos distintos.
- CA-208: `hoom status --json` incluye `envelopes` y el `usage` de cada run
  activo, y el texto dice exactamente lo mismo que el JSON.
- CA-209: Studio: `GET /api/envelopes` devuelve los sobres; `GET /api/runs`
  mezcla los runs en memoria con los sidecars del disco —un run de otro
  proceso aparece, con su rol y su costo—; los eventos de un run ajeno se leen
  del jsonl; y sobre un run ajeno la API no acepta `input` ni `cancel`.
- CA-210: la UI embebida pinta los sobres con su paso y su estado, y el meta
  del run abierto muestra rol y costo; el Studio sigue sin referenciar ningún
  asset externo.
- CA-211: con `Strict`, `Input` sobre un provider que no puede continuar —ni
  resume por id ni continue— se niega con un error de dominio que nombra al
  provider y la acción, y deja el run intacto: estado terminal, directorio
  libre, sin evento nuevo en el log y sin proceso lanzado.
- CA-212: sin `Strict`, el comportamiento se mantiene —invocación nueva— con
  UNA sola línea de aviso, que dice que el contexto anterior no viaja.
- CA-213: positivos bajo `Strict`: con sesión capturada y provider con resume,
  la invocación lleva `--resume <id>` y no hay campos ignorados; un provider
  con `continue` y sin `resume` continúa con `--continue`, porque continuar la
  última sesión del directorio es continuación real.
- CA-214: el Studio traduce la negativa de `Input` a un 400 con el mensaje del
  dominio, y el run sigue consultable después del rechazo.
- CA-215: compatibilidad y ayuda. [verifica: go test ./...] [verifica: go run ./cmd/hoom help 2>&1 | grep -qi sobre]
  Los cinco/seis pasos del sobre, `hoom review`, `hoom task`, los veredictos y
  la huella no cambian; `hoom status -h` y la ayuda documentan lo nuevo.
- CA-216: E2E opcional: con `HOOM_E2E=1` y `claude` real en PATH, un run corto
  deja en su sidecar un costo mayor que cero y al menos un turno, y su jsonl
  no contiene ninguna línea JSON cruda de `"type":"system"`; sin la variable
  el test se omite y jamás es requisito de `go test`.

## Decisiones

1. **Una spec, cuatro ítems.** Comparten raíz (prosa donde debería haber
   dato), archivos (`providers`, `runcmd`, `statuscmd`, `servecmd`) y
   superficie (status y Studio). Cuatro specs serían cuatro veces el ritual
   para tocar los mismos cuatro archivos.
2. **Paquete `internal/envelope` y no un campo más en el sidecar del run.**
   El sobre tiene pasos ANTES de que exista el run (`spec`, `aislar`) y
   DESPUÉS de que termina (`scope`, `verify`, `check`), y puede cortar sin
   haber lanzado ninguno. Colgar su estado del run sería mentir sobre quién
   contiene a quién.
3. **`.hoom/envelopes/` y no `.hoom/runs/<id>.envelope.json`.** Mismo motivo:
   el id del sobre no es el id del run, y un sobre puede no tener run.
4. **`CostUSD` como puntero.** Es la diferencia entre "gastó 0" y "no sé lo
   que gastó". Codex hace que esa diferencia sea el caso normal, no el borde.
5. **Sumar y no reemplazar.** Se midió: Claude reporta por invocación
   (0.111583 y después 0.1120905 en la misma sesión, no 0.223). Si mañana un
   provider reportara acumulado, sumar rompería — queda escrito en riesgos.
6. **El kind `system` en vez de un flag `noise` en el evento.** Un kind es lo
   que el resto del sistema ya sabe leer: el escenario lo ignora, el feed lo
   atenúa y el JSON no cambia de forma. Un flag nuevo obligaría a tocar cada
   consumidor para que lo respete.
7. **El subtype desconocido conserva la línea íntegra.** El kind ya clasifica;
   el detalle no tiene por qué empeorar. Solo se resume lo que hoom sabe leer.
8. **Un `type` de primer nivel desconocido sigue siendo `text`.** Solo baja a
   `system` lo que hoom RECONOCE como plomería. En la duda, visible.
9. **`Input` con `Strict` se niega.** Perder la sesión no es perder un flag:
   es empezar de cero. Es la degradación que `Strict` prohíbe, y hasta hoy
   pasaba con el mensaje genérico de campo no soportado.
10. **El run ajeno es de solo lectura en el Studio.** Ver un run que otro
    proceso corre es telemetría; adoptarlo es control, y el control de un
    proceso ajeno pide su propia spec.
11. **"Posible huerfano" con el umbral de `verify` (15 minutos).** Una palabra
    y un número que ya existen en el harness. Inventar un segundo umbral para
    la misma pregunta sería otra cosa que después hay que explicar.
12. **`status` pierde el fallback de la regex.** Sostener la regex "por los
    logs viejos" sería sostener el bug que esta spec vino a matar. Un run sin
    sidecar dice `?`, que es la verdad.

## Riesgos

- **Un provider futuro que reporte costo acumulado por sesión.** Sumar lo
  contaría dos veces. Mitigación: la suma vive en un solo lugar (`runcmd`), la
  decisión está escrita acá, y cada adapter es dueño de traducir su stream a
  `Usage` por invocación.
- **Los campos del `result` de Claude pueden cambiar de nombre.** Ya pasó con
  otras claves entre versiones. Mitigación: los campos ausentes dan `usage`
  vacío, que se muestra como ausencia y no como ceros; ningún parseo falla
  por un campo que no está.
- **El registro del sobre puede quedar "en curso" para siempre** si el proceso
  muere. Mitigación: se etiqueta, no se corrige. hoom no cierra lo que no vio
  cerrar (la misma regla que `verify` y sus huérfanos).
- **Archivos que se acumulan** en `.hoom/envelopes/`, uno por sobre. Es el
  mismo crecimiento que `.hoom/runs/`, en un orden de magnitud menor, y está
  gitignorado. Si molesta, la poda es una spec de mantenimiento.
- **Bajar el ruido a `system` puede esconder algo que importaba.** Un
  `compact_boundary` dice que el contexto se compactó, y eso explica
  comportamientos raros. Mitigación: el evento sigue en el log íntegro y la UI
  lo muestra por defecto (atenuado); ocultarlo es una decisión del que mira.
- **El costo es un dato sensible**: queda en telemetría local, fuera de Git y
  fuera de los veredictos, como el resto de `.hoom/runs/`.
- **Ensanchar el `Snapshot`** de `status` toca un JSON que otros ya consumen.
  Mitigación: solo se AGREGAN campos; ninguno cambia de nombre ni de forma.
