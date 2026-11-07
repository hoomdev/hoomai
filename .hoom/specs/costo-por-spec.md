# Spec: lo que costó un spec

Estado: ENMIENDA 1 — pendiente de re-aprobación humana. La versión aprobada
(sha256 90997459) todavía no tiene código. La enmienda 1 agrega `reiniciada`
a cada ventana de un run. Sin ese dato, después no se puede distinguir un run
que cruzó un reinicio de la ventana de uno que no lo cruzó, y el movimiento
de ese run mentiría.
Depende de: nada nuevo (main f88eb1a).
Tarea: `hoom task start costo-por-spec`.

## Objetivo

Hoy nadie puede contestar "¿cuánto me costó este spec hasta quedar
aceptado?". hoom guarda el uso de cada run, pero:

- La tarjeta suma `spend`, pero Codex no informa USD. Así, la parte más cara
  de un spec (las lentes de review) figura como "sin dato de costo".
- Al usuario de un plan de 20 USD le importa cuánto de su ventana de 5 h y
  de su semana se lleva un spec. Los dos providers lo informan y hoom no lo
  guarda:
  - Claude lo manda en el stream (`rate_limit_event`, y hoom lo deja como
    texto).
  - Codex lo escribe en el archivo de sesión del thread
    (`sessions/.../rollout-*-<thread>.jsonl`, eventos `token_count`), no en
    `exec --json`.
- El sidecar de un run no dice con qué modelo ni con qué esfuerzo corrió.
- hoom descarta los tokens de razonamiento y de escritura en caché que
  informa Codex.
- El registro de una review guarda tokens por lente, pero no qué runs fueron
  suyos. Por eso no se pueden separar las rondas.

Esta spec agrega `hoom costo`. Para cada tarea muestra lo que costó hasta
quedar aceptada, por rol y por ronda de review, en las unidades que cada
provider da de verdad. Es el medidor que necesitan los cambios que vienen
(perfiles de modelo, corte del loop de review): sin él, cualquier ahorro
sería narración.

## No-goals

- **Convertir tokens a USD con tablas de precios.** hoom no inventa precios.
- **Estimar el % de la ventana con una fórmula de tokens.** Un ajuste medido
  en una cuenta no es un dato del provider.
- **Cambiar lo que fija `datos-en-vez-de-prosa.md`** (sus criterios 197 y
  198). Se agregan campos; los que ya existen no cambian de significado y la
  línea de `Usage.Summary` sigue igual.
- **Atribuir la ventana a un run con exactitud.** La ventana es de la
  cuenta, así que lo que gaste otra sesión al mismo tiempo entra en el
  movimiento. Se dice, no se corrige.
- **Mostrarlo en el Studio o en el TUI.** La capa 2 lo toma de `--json`.
- **Presupuestos, alertas o cortes por costo.**
- **Completar runs viejos.** Lo que no se guardó queda "sin dato".
- **Contar runs cuyo sidecar vive dentro del worktree de la tarea.** Se borran
  con `task done`.

## Contratos

### Lo que guarda un run

`runcmd.Meta` (`.hoom/runs/<id>.meta.json`) gana tres campos. Los tres son
opcionales y se omiten si no hay dato:

- `model` y `effort`: los que hoom le pasó al provider. Si no le pasó
  ninguno, los que el provider dice haber usado (Claude: `model` del
  `system/init`; Codex: `model` y `effort` del `turn_context` de su sesión).
- `ventanas`: una entrada por ventana informada, con el nombre `5h` o
  `semana`, y estos campos:
  - `inicio`: el % usado en el primer dato del run;
  - `fin`: el % usado en el último dato del run;
  - `reinicia_en`: la hora UTC del reinicio, tal como la da el provider en
    el último dato;
  - `reiniciada`: `true` si, dentro del run, `reinicia_en` cambió más de 60 s
    entre dos datos. Quiere decir que la ventana se reinició en medio y el
    movimiento de ese run no se puede medir. Se omite si no cambió
    (enmienda 1);
  - `plan`: si el provider lo dice.

  El % se guarda de 0 a 100, sin redondear lo que el provider dio. De dónde
  sale cada uno:
  - Claude: `rate_limit_info.unifiedWindows`. `five_hour` es `5h`,
    `seven_day` es `semana`; `utilization` viene en fracción.
  - Codex: `token_count.rate_limits`. `primary` y `secondary` se nombran por
    `window_minutes` (300 es `5h`, 10080 es `semana`; otro valor se nombra
    `<n>min`). `plan_type` es `plan`. Se lee al cerrar el run, del archivo
    de `$CODEX_HOME/sessions` (por defecto `~/.codex/sessions`) cuyo
    `session_meta.id` es el thread del run. Solo se leen `session_meta`,
    `turn_context` y `token_count`; nunca el contenido de los mensajes.

`providers.Usage` gana dos campos, sin tocar los demás:

- `reasoning_output_tokens`: Codex, de `turn.completed`.
- `cache_write_input_tokens`: cuántos de `input_tokens` se escribieron en
  caché. Es un subconjunto: nunca se suma aparte. Codex lo trae de
  `turn.completed` y Claude de `cache_creation_input_tokens`.

### El registro de una review

`.hoom/reviews/<id>.json` gana `runs: [{lens, run}]`, el run de cada lente.

### `hoom costo [<slug>] [--json]`

- **Sin slug:** una línea por tarea que tenga algún run o review, de la más
  reciente a la más vieja, con:
  - el slug y el estado: `aceptada <fecha>` si el item tiene `hecho_en`;
    `cerrada sin aceptar` si no lo tiene y su worktree ya no existe; si no,
    `en curso`;
  - las rondas de review;
  - por provider, los tokens, los USD y la ventana.
- **Con slug:** el desglose.
  - Por rol: runs, tokens (`nuevos`, `cache leido`, `salida`,
    `razonamiento`), USD, ventanas y tiempo.
  - Por ronda de review: id, `completa` o `delta`, lentes, hallazgos
    high/medium/low, tokens, ventanas y tiempo.
  - El total por provider y, si la tarea está aceptada, el veredicto verde y
    el `commit_final`.
- **Siempre:** solo lee y sale con 0.
- **Slug sin runs ni reviews:** sale con 1 y dice
  `hoom costo: no hay runs ni reviews de la tarea <slug>`.

Reglas de las cifras:

- **Tokens.**
  - Por provider, nunca un total entre providers.
  - `nuevos` es Claude `input_tokens` y Codex `input_tokens − cached`;
    `cache leido` es `cached_input_tokens`.
- **USD.** Solo lo que informa el provider. Se rotula `equivalente API`, y
  un total que no cubre a todos los providers dice `<provider> no informa
  costo`.
- **Ventana.**
  - El movimiento de un run es `fin − inicio`.
  - Para el total, los runs del mismo provider que se superponen en el
    tiempo se unen en un bloque. Cada bloque cuenta desde el `inicio` de su
    primer run hasta el `fin` del último, y los bloques se suman.
  - Hay dos casos en que la ventana se reinició y ese tramo no se puede
    medir: un run con la ventana `reiniciada`, o un bloque donde
    `reinicia_en` cambia más de 60 s de un run a otro. En los dos, el total
    pasa a decir `al menos <x>`.
  - Un movimiento menor a 1 punto se muestra `<1`.
  - La línea de ventanas siempre aclara que es la ventana de la cuenta
    durante los runs.
- **Lo que falta.** Se dice: `sin dato`, con cuántos runs no lo tienen
  (`ventana: sin dato en 3 de 9 runs`).
- **Ronda sin `runs`.** Si la ronda viene de un registro viejo sin `runs`,
  los runs del reviewer de esa tarea se muestran como `reviewer (sin
  ronda)`.

`--json` tiene un esquema estable, que este spec fija:

- arriba: `tareas` (lista);
- por tarea: `slug`, `estado`, `aceptada_en`, `veredicto`, `commit_final`,
  `rondas`, `por_provider`, `por_rol` y `rondas_review`;
- cada cifra que falta es `null`, nunca 0.

## Casos límite y errores esperados

- **Run en curso:** se muestra `en curso` y no suma ventana (todavía no tiene
  `fin`).
- **Sidecar ilegible:** se cuenta como `N runs ilegibles` y el comando
  sigue.
- **Archivo de sesión de Codex ausente** (otro `CODEX_HOME`, borrado,
  ilegible): el run se cierra igual y no cambia su resultado. Sin
  `ventanas`; `model` es el que pasó hoom, si pasó uno.
- **Dos archivos con el thread en el nombre:** vale el que tiene
  `session_meta.id` igual.
- **Claude sin `rate_limit_event`** (por ejemplo, con una API key sin plan):
  sin `ventanas`.
- **`resets_at` que salta ±1 s entre eventos** (Codex lo hace): no es un
  reinicio, por eso el umbral es de 60 s.
- **El primer dato llega después de la primera llamada del run.** El
  movimiento puede quedarse corto en lo que gastó esa llamada; la ayuda lo
  dice.
- **Ronda `NO ENTREGABLE` o con lentes fallidas:** cuenta igual, porque el
  costo se gastó.
- **Tarea con runs de dos providers:** dos totales, uno por provider.

## Criterios de aceptación

- CA-538: el sidecar de un run guarda `model` y `effort` cuando hoom se los pasó al provider; sin ellos guarda los que informó el provider (Claude `system/init.model`, Codex `turn_context.model`/`effort`); sin ninguno, los omite.
- CA-539: `Usage` gana `reasoning_output_tokens` (Codex, de `turn.completed`) y `cache_write_input_tokens` (Codex `cache_write_input_tokens`, Claude `cache_creation_input_tokens`), sin cambiar `input_tokens`, `cached_input_tokens`, `output_tokens` ni el texto de `Summary`.
- CA-540: un run de Claude con `rate_limit_event` guarda en `ventanas` las ventanas `5h` y `semana` con `inicio` del primer evento, `fin` del último y `reinicia_en` del último, con `utilization` pasada a %, y `reiniciada: true` cuando `reinicia_en` cambió más de 60 s entre dos eventos del run (un salto de 1 s no la marca); sin eventos, el sidecar no tiene `ventanas`.
- CA-541: al cerrar un run de Codex, hoom lee el archivo de `$CODEX_HOME/sessions` (por defecto `~/.codex/sessions`) cuyo `session_meta.id` es el thread y guarda `ventanas` (`primary`/`secondary` nombradas por `window_minutes`, con `plan` y con `reiniciada` según la misma regla) y, si hoom no los pasó, `model` y `effort` del `turn_context`; con el archivo ausente o ilegible el run termina con el mismo resultado y sin `ventanas`.
- CA-542: el registro de una review guarda `runs` con el run de cada lente.
- CA-543: `hoom costo` sin slug lista una línea por tarea con runs o reviews, de la más reciente a la más vieja, con estado `aceptada <fecha>` (item con `hecho_en`), `cerrada sin aceptar` (sin `hecho_en` y sin worktree) o `en curso`, rondas de review y, por provider, tokens, USD y ventana.
- CA-544: `hoom costo <slug>` desglosa por rol y por ronda de review (id, completa o delta, lentes, hallazgos por severidad); muestra los tokens por provider (`nuevos`, `cache leido`, `salida`, `razonamiento`) y nunca suma tokens de dos providers; los USD salen rotulados `equivalente API` y un total que no cubre a un provider dice `<provider> no informa costo`.
- CA-545: el total de ventana de una tarea une los runs superpuestos del mismo provider en bloques y suma los bloques; un run con la ventana `reiniciada`, o un `reinicia_en` que cambia más de 60 s entre runs de un bloque, marca el tramo como reiniciado y el total dice `al menos`; un salto de 1 s no es reinicio; un movimiento menor a 1 punto se muestra `<1`.
- CA-546: lo que falta se dice: `sin dato` con el conteo de runs que no lo tienen, los sidecars ilegibles se cuentan sin abortar, una ronda de un registro sin `runs` muestra sus runs como `reviewer (sin ronda)`, y un slug sin runs ni reviews sale con 1 y `hoom costo: no hay runs ni reviews de la tarea <slug>`.
- CA-547: `hoom costo --json` emite `tareas` con `slug`, `estado`, `aceptada_en`, `veredicto`, `commit_final`, `rondas`, `por_provider`, `por_rol` y `rondas_review`, y cada cifra que falta es `null`, nunca 0.
- CA-548: `hoom help` lista `costo` con una línea que dice que el movimiento de ventana es el de la cuenta durante los runs.

## Decisiones

- **Por provider, nunca un total cruzado.** Los tokens de dos providers no
  son la misma unidad ni salen del mismo cupo.
- **La ventana es la cifra principal.** Es lo que agota un plan de 20 USD.
  Los USD de Claude se muestran rotulados como `equivalente API`, porque con
  suscripción no se cobran.
- **Leer el archivo de sesión de Codex.** Es el único lugar donde Codex dice
  su ventana. Si el formato cambia, el run queda `sin dato`; nunca falla.
- **Bloques unidos en vez de sumar runs.** Sumar runs superpuestos contaría
  dos veces el mismo movimiento de la cuenta.
- **El reinicio se marca en el run, mientras pasa (enmienda 1).** El run es
  el único que ve todos sus datos. Después solo queda `inicio`, `fin` y un
  `reinicia_en`, y con eso un reinicio en medio se confunde con un
  movimiento real.
- **Aceptada = `hecho_en`.** Es el único hecho que hoom escribe con el gate
  verde.
- **Un verbo nuevo.** `report` es de veredictos y `status` es lo que está
  vivo; mezclarlos con costo los haría menos claros.
- **Alternativas descartadas.**
  - Precios por token: inventaría.
  - Una fórmula de cupo: un ajuste medido en una cuenta no es un dato.
  - Atribuir por run restando las otras sesiones de la cuenta: hoom no las
    ve.

## Riesgos y deuda aceptada

- **Formatos internos.** El archivo de sesión de Codex y el
  `rate_limit_event` de Claude no son APIs públicas. Si cambian, el run
  queda `sin dato`. Los tests fijan las versiones verificadas: Codex 0.160.0
  y Claude Code 2.1.319.
- **La cuenta, no el run.** Si la persona usa el mismo provider mientras
  corre la tarea, el movimiento lo incluye. Se dice en cada salida.
- **El primer dato llega tarde.** El movimiento puede quedarse corto en la
  primera llamada de cada run.
- **Granularidad.** Codex informa % enteros, así que un run corto puede
  mostrar `<1` aunque haya gastado.
- **Runs perdidos.** Los runs con sidecar dentro del worktree de la tarea se
  borran con `task done` y no se cuentan.
