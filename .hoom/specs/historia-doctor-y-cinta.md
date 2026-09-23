# Spec: historia, doctor y cinta de la tarjeta (cabina visual, C4)

Estado: BORRADOR — pendiente de aprobación humana.
Depende de: `acciones-desde-la-tarjeta.md` (C3) integrada (PR #25).
Tarea sugerida: `hoom task start historia-doctor-y-cinta`.
Origen: `.hoom/intake/rfc-cabina-visual-v2.md` (spec 4 de 4, "C4 — Timeline,
replay y doctor", y "Sin auto_run") y el pedido de Henry del 2026-09-23, que
fija las seis piezas: la historia de la tarjeta, el replay, `hoom board
doctor` con sus insignias, la correlación tool_use/tool_result, la curva del
trinquete y el piloto automático opt-in.
Absorbe del roadmap del README (sección "Cabina visual (Studio v5)"): "Curva
del trinquete en el tablero" y "Timeline, replay y doctor", con su
correlación tool_use/tool_result.
Re-afirma, sin cambiar sus tests: CA-288 (C1), CA-313, CA-320 y CA-322 (C2),
CA-354 y CA-355 (C3), y CA-32 y CA-38 del escenario (Studio v4).

## Objetivo

Que la tarjeta cuente cómo llegó a donde está, que hoom diga en voz alta
dónde la evidencia no cierra, y que una persona pueda dejar que una tarjeta
avance sola entre dos firmas suyas, sin que hoom decida nada. Seis piezas:

1. **La historia de la tarjeta.** Una lista de entradas ordenada en el
   tiempo, derivada de dos fuentes rotuladas como tales: el **historial de
   git** de los archivos de la tarjeta (item, spec, aprobaciones, veredictos,
   hallazgos y sus resoluciones, registros de review) y los commits de su
   tarea, y la **telemetría local** cuando existe (sobres, runs y las
   delegaciones a subagentes de esos runs). Cada entrada dice cuándo, quién
   (identidad git, o rol y provider), qué artefacto y cuánto costó. La lee
   `GET /api/board/{slug}/timeline` y la pinta la pestaña **Historia** del
   detalle.
2. **El replay.** La historia reproducida como secuencia en la pestaña
   Historia, con el medidor de evidencia llenándose a medida que pasan las
   entradas y la tarjeta de hoy al final. Sirve para mostrarle a un cliente
   cómo se construyó algo.
3. **`hoom board doctor [--json]`**: lista cada lugar donde la evidencia no
   es coherente, con su acción exacta, y la tarjeta muestra los suyos como
   **insignias** en el tablero.
4. **La correlación tool_use/tool_result** en el parser de Claude: cada
   delegación a un subagente se empareja con su resultado, y el escenario y
   la historia marcan cuándo el subagente sale de escena.
5. **El trinquete en el Studio**: `GET /api/ratchet` con la línea base y sus
   movimientos, y la curva de cada métrica en el tablero de control (pestaña
   Cockpit). La fuente es la sección del trinquete de `hoom status`.
6. **El piloto automático por tarjeta**, opt-in con `auto: hasta-humano` en
   el item. **Es una cinta transportadora, no un orquestador**: después de un
   sobre exitoso que lanzó el Studio, pide el trabajo del rol siguiente según
   el orden fijo de la tabla de columnas de C1, sin juicio y sin reintentos,
   y se detiene ante cualquier rojo, en las columnas moradas y cuando el
   presupuesto del item se agota.

Todo se deriva en el binario, como en C1 a C3: la historia, el doctor y la
decisión de la cinta son funciones de Go con tests, y la página solo pinta.
Nada de esto escribe una columna, y la regla de oro de C3 no cambia.

## No-goals

- **Guardar la historia.** No hay log de eventos propio ni base: la historia
  se calcula cada vez que se pide, desde git y desde la telemetría, como el
  tablero.
- **Reconstruir el medidor del pasado con exactitud.** El replay enciende los
  segmentos cuando llega su evidencia y no recalcula la huella de cada commit
  viejo. Lo que vale hoy lo dice la tarjeta, que es el último cuadro del
  replay (ver Decisiones).
- **Grabar un video.** El replay es una secuencia animada en la página, no un
  archivo.
- **Un verbo de CLI para la historia.** Como el detalle de C2
  (`boardcmd.DetailFor`), la historia es una función de Go que lee el Studio.
- **Que el doctor arregle algo.** Diagnostica y da la acción; no escribe,
  no cierra sobres, no aprueba y no verifica.
- **Un endpoint para el doctor.** El Studio muestra las insignias de cada
  tarjeta, que viajan en la tarjeta. Los problemas que no son de ninguna
  tarjeta se ven con `hoom board doctor`.
- **Atribuirle al subagente los actos que hizo adentro de su delegación**
  (las líneas con `parent_tool_use_id`). El escenario sigue contándoselos al
  orquestador; solo cambia cuándo el subagente está en escena.
- **Correlación para Codex, gemini y opencode.** Solo Claude delega en
  subagentes con un id que se pueda emparejar.
- **Medir el trinquete.** `GET /api/ratchet` lee el archivo, como `hoom
  status`; medir sigue siendo de `verify --full`.
- **Que la cinta juzgue.** No elige rol fuera de la tabla, no reintenta, no
  relanza tras un rojo, no corrige hallazgos por su cuenta, no firma y no
  integra.
- **La cinta en la terminal.** `hoom agent` y `hoom review` no encadenan
  nada: la cinta corre en `hoom serve`, después de un trabajo que lanzó el
  Studio.
- **Activar o apagar el piloto desde el Studio.** Se activa en el item (a
  mano o con `hoom item add --auto`), que es un archivo que firma una persona
  en git.
- **Dependencias nuevas.** `go.mod` y `go.sum` no cambian, y la UI sigue sin
  frameworks ni assets de red.

## Contratos

### 1. La historia de la tarjeta

`boardcmd.TimelineFor(root, base, blockOn, slug string, now time.Time)
(Timeline, error)` falla como `CardFor` (item inexistente o inválido) y, como
`Gather`, no crea ni modifica nada: los únicos comandos que corre son
lecturas de git.

```json
{
  "slug": "S",
  "card": { "...": "la tarjeta de hoy, la misma de /api/board" },
  "meter": [{"id": "spec", "label": "spec aprobado"}, {"id": "CA-n", "label": "CA-n"}],
  "entries": [
    {"at": "2026-09-23T18:00:00Z", "ended_at": null,
     "source": "git", "kind": "aprobacion",
     "who": "Henry Orellana <henry@...>", "role": "", "provider": "",
     "artifact": ".hoom/worktrees/S/.hoom/approvals/S_d0160342.json",
     "ref": "<sha de 40>", "summary": "...", "plain": "se aprobo el spec",
     "cost_usd": null, "tokens": 0, "piloto": false,
     "meter": [{"id": "spec", "state": "hecho"}]}
  ],
  "telemetry": {"sobres": 2, "runs": 3},
  "notes": []
}
```

- **`card`** es la tarjeta derivada ahora (`Derive`): el último cuadro del
  replay. **`meter`** es el esqueleto del medidor para el replay: los `id` y
  `label` de `card.meter`, en su orden.
- **`entries`** va en orden ascendente por `at`. A igual `at`, primero `git`
  y después `telemetria`, y dentro de cada fuente en el orden de `kind` de la
  tabla de abajo, y después por `ref`. Ninguna lista es `null`.
- **`artifact`** es relativo a la raíz del proyecto (con el prefijo
  `.hoom/worktrees/<slug>/` cuando se leyó en el espacio de trabajo), o `""`
  en un commit de código.
- **`cost_usd`** es `null` cuando nadie reportó costo (toda entrada de git,
  toda delegación y los runs sin costo). **`tokens`** es entrada más salida,
  0 sin dato.
- **`piloto`** es `true` solo en un sobre que lanzó la cinta (pieza 6).
- **`telemetry`** cuenta los sobres y runs de la tarjeta que hay en esta
  computadora.
- **`plain`** sigue las reglas de `plain` de C2: nunca vacía, sin acentos y
  sin `git`, `commit`, `worktree`, `merge`, `rama`, `branch`, `diff`, `HEAD`
  ni `huella` (palabras enteras, sin distinguir mayúsculas), ni `.hoom/` ni
  `hoom `. **`summary`** es la versión del modo experto (ids, sha de 12,
  rutas, notas).

#### Fuente `git`

Se lee con `git log --no-renames` (fecha de autor, `%aI`; autor `%an <%ae>`)
sobre los archivos de la tarjeta. El item se lee en el árbol raíz y lo demás
en el árbol de evidencia de la tarjeta (su espacio de trabajo si existe, como
`Gather`). Una entrada por cada par (commit, archivo), con la letra de
`--name-status` (`A`, `M` o `D`):

| `kind` | archivos | `plain` |
|---|---|---|
| `item` | `.hoom/items/<slug>.yaml` | A: `se creo la tarjeta`; M: `cambio la tarjeta`; D: `se borro la tarjeta` |
| `spec` | `.hoom/specs/<slug>.md` | A: `se escribio el spec`; M: `cambio el spec`; D: `se borro el spec` |
| `aprobacion` | `.hoom/approvals/<slug>_*.json` | `se aprobo el spec` |
| `commit` | los commits de la tarea (abajo) | `se guardaron cambios en <n> archivos` (`1 archivo`) |
| `veredicto` | `.hoom/verdicts/<id>.json` de cada veredicto de la tarjeta (su `spec`, como `Gather`), completo o parcial | completo verde: `la verificacion dio verde`; completo rojo: el `plain` de C2 (`la verificacion dio rojo: fallo el gate <g>`); parcial: `una verificacion parcial dio verde` o `... dio rojo` |
| `hallazgo` | `.hoom/findings/<id>.json` de los hallazgos con `task` = slug | `la revision encontro un problema (<severidad>)` |
| `resolucion` | `.hoom/findings/<id>.res.json` de esos hallazgos | `se resolvio un problema de la revision (<corregido\|refutado>)` |
| `review` | `.hoom/reviews/<id>.json` de los registros con `task` = slug | con las 4 lentes: `quedo registrada la revision de 4 lentes`; si no: `quedo registrada una revision` |

`who` es el autor del commit, `ref` el sha completo, y `summary` nombra el
sha de 12, la letra y el asunto del commit (en la aprobación, además, quién
firmó según el archivo de hoy, si todavía existe).

**Los commits de la tarea** (`kind: commit`, leídos con `--no-merges`:
un merge no es una entrada): uno por commit, con `who` su autor,
`ref` su sha, `summary` su asunto y la cantidad de archivos. El rango:

1. Si el item tiene `commit_final`: si no está integrado en la rama base,
   `<base>..<commit_final>`. Si está integrado, `M^1..<commit_final>`, donde
   `M` es el primer commit de la cadena de primeros padres de la rama base
   que contiene `commit_final` (`git rev-list --first-parent
   --ancestry-path <commit_final>..<base>`, el más viejo). Si `M` no es un
   merge (se integró con fast-forward), no hay commits de la tarea y
   `notes` dice `no se pueden separar los commits de la tarea: se integro sin
   commit de merge`.
2. Si no, con espacio de trabajo: `<base>..HEAD` en el espacio de trabajo.
3. Si no, con la rama `hoom/<slug>`: `<base>..hoom/<slug>` en la raíz.
4. Si no, ninguno, y `notes` dice `la tarjeta no tiene espacio de trabajo
   propio: la historia muestra solo su evidencia`.

Si git falla, las entradas de git faltan y `notes` dice `sin historial de
git: <error>`.

#### Fuente `telemetria`

Los sobres y runs de la tarjeta, leídos donde los lee `Gather` (la raíz y el
espacio de trabajo), con la misma regla de gasto que el detalle de C2
(`workOf`): cada run se cuenta una vez.

| `kind` | de dónde | `at` / `ended_at` | `who` | `plain` |
|---|---|---|---|---|
| `sobre` | cada registro de sobre | `started_at` / `ended_at` (null si no cerró) | `<rol> (<provider>)` | entregable: `el <rol> entrego su trabajo`; no-entregable y sin-entrega: el `plain` de C2 del sobre; en curso con dueño vivo: `el <rol> esta trabajando`; en curso sin dueño: `el trabajo del <rol> quedo interrumpido` |
| `run` | cada run que ningún sobre referencia | `created_at` / `ended_at` | `<rol> (<provider>)`, o el provider solo | `trabajo el <rol o provider>`; con estado `error`: `el trabajo del <rol o provider> fallo` |
| `subagente` | cada evento `agent` de los runs de la tarjeta (`.hoom/runs/<id>.jsonl`) | su `ts` / el `ts` de su `agent_end` (null si no llegó) | `<agente> (<provider>)` | `el <rol del run> le paso trabajo a <agente>` |
| `subagente-fin` | cada evento `agent_end` emparejado | su `ts` / null | `<agente> (<provider>)` | `<agente> termino su parte`; con error: `<agente> fallo` |

`artifact` es la ruta del registro del sobre o del `.jsonl` del run. El
costo de un sobre es el `usage` del sidecar de su run (o el suyo si el run no
tiene sidecar); las delegaciones no tienen costo propio: el provider reporta
por invocación, no por subagente. `role` y `provider` van en sus campos, y
`piloto` copia el del registro del sobre.

Sin sobres ni runs de la tarjeta en esta computadora, no hay entradas de
telemetría y `notes` dice `sin telemetria en esta computadora: el trabajo de
los agentes solo se ve donde corrio`.

#### El medidor del replay: `meter` de cada entrada

Cada entrada trae los segmentos que enciende o apaga, como `[{id, state}]`
(`hecho`, `falta` o `no-aplica`), y `[]` si no toca el medidor. El replay
empieza con todos los segmentos del esqueleto en `falta` y aplica los
efectos en orden. Un `id` que el esqueleto no tiene se ignora.

- **`spec`**: una entrada `spec` con A o M lo pone en `hecho` si el
  contenido del spec en ese commit tiene una aprobación de una entrada
  anterior o del mismo commit (su SHA-256 es el `sha256` de esa aprobación),
  y en `falta` si no. Una entrada `aprobacion` lo pone en `hecho` si el spec
  del último commit anterior o igual tiene ese SHA-256. Un spec borrado lo
  pone en `falta`.
- **Criterios**: un commit de la tarea enciende (`hecho`) cada criterio del
  spec de hoy cuyo token `CA-n` aparece, por primera vez en la historia, en
  una línea agregada de un archivo de test (la misma regla de archivo de test
  que `spec_trace`). Un veredicto completo cuyo gate `spec_trace` pasó
  enciende todos los criterios. El parche de los commits de la tarea se lee
  hasta 8 MiB; si se corta, `notes` dice `la historia de los tests se corto
  en 8 MiB: los criterios se encienden con el veredicto`.
- **Gates** (`build`, `static`, `test`): un veredicto **completo** pone cada
  uno en `hecho` si pasó, en `falta` si falló o dio error, y en `no-aplica`
  si el veredicto no lo trae. Un veredicto parcial no toca el medidor.
- **`review`**: un veredicto completo la pone en `no-aplica` si el cambio no
  exige las 4 lentes (la regla de tamaño de C1), y un registro de review con
  las 4 lentes, en `hecho`.
- Los hallazgos, las resoluciones, el item y la telemetría no tocan el
  medidor.

#### `GET /api/board/{slug}/timeline`

- Responde el `Timeline` en JSON. Un slug con forma inválida es 400 y un item
  inexistente o inválido es 404 con el mensaje de `CardFor`.
- Es una lectura: sin token, como las demás del tablero; otro método es 405;
  deja el repo y `.hoom/` byte a byte iguales, ignorados incluidos.

### 2. La pestaña Historia y el replay

- El detalle de la tarjeta gana una cuarta pestaña, **Historia**, después de
  Pruebas (`data-pane="historia"`, panel `tb-p-historia`). La pinta
  `tablero.js`, que sigue sin POST (CA-320 no cambia).
- Al abrirla pide `/api/board/<slug>/timeline` una vez; un botón
  **Actualizar** la vuelve a pedir. No sondea: la historia sale de git y se
  pide cuando alguien la mira.
- **La lista**: una fila por entrada con la hora, quién (en modo normal, el
  nombre sin el correo), el texto (`plain` en modo normal, `summary` en
  experto), el costo si lo hay (`sin dato` cuando es `null` en un sobre o un
  run) y el rótulo de su fuente: en modo normal `guardado` (git) y `esta
  computadora` (telemetría); en experto `git` y `telemetría local`. Una
  entrada con `piloto` lleva la marca `piloto automático`. Las `notes` van
  arriba de la lista.
- **El replay**: botones **Reproducir** / **Pausa**, **Reiniciar** y la
  velocidad (`1x`, `2x`, `4x`). Cada paso resalta una entrada, aplica sus
  efectos al medidor del replay (pintado desde `meter`, con los mismos
  estados y colores que el medidor de la tarjeta) y espera 800 ms divididos
  por la velocidad con `setTimeout` (nunca `setInterval`). Después de la
  última entrada muestra **Así está hoy**: el medidor de `card.meter` y su
  `plain`.
- Sin entradas, la pestaña dice `Todavía no hay historia para esta tarjeta.`
  y no ofrece Reproducir.

### 3. El doctor

#### Los problemas

```json
{"id": "spec-editado", "slug": "S", "what": "...", "plain": "...",
 "action": "cd .hoom/worktrees/S && hoom spec approve .hoom/specs/S.md",
 "ref": ".hoom/worktrees/S/.hoom/specs/S.md"}
```

- **De una tarjeta**: los calcula `Derive`, que sigue siendo pura, y viajan en
  la tarjeta como `doctor` (lista, nunca `null`), así que salen en `hoom board
  --json`, `hoom item show --json`, `/api/board` y el detalle. El texto de
  `hoom board` y de `hoom item show` no cambia. `plain` sigue las reglas de
  C2.
- **Del proyecto**: los que no son de ninguna tarjeta. Solo los lista `hoom
  board doctor`, con `plain: ""` y `slug` = la tarea, o `""`.
- `<cd>` es `cd <dir> && ` cuando el árbol de evidencia de la tarjeta no es
  la raíz (`<dir>` = `.hoom/worktrees/<slug>`), y vacío si lo es. `<spec>` es
  `.hoom/specs/<slug>.md`.

Los de una tarjeta, en este orden. Una tarjeta en Hecho solo puede tener
`sobre-huerfano`: Hecho es terminal (C1).

| `id` | cuándo | `what` | `plain` | `action` |
|---|---|---|---|---|
| `spec-editado` | la aprobación del spec quedó invalidada (`approval.StatusInvalidated`) | `el spec cambio despues de su aprobacion: revisalo y aprobalo de nuevo` | `el spec cambio despues de tu aprobacion` | `<cd>hoom spec approve <spec>` |
| `sin-veredicto` | la tarjeta tiene espacio de trabajo con cambios (la candidata tiene archivos), ningún veredicto completo de ese árbol certifica su huella actual, y el último veredicto completo de la tarjeta no es verde | `el espacio de trabajo tiene cambios que ningun veredicto certifica (huella <huella>)` | `hay cambios sin verificar` | `<cd>hoom verify --spec <spec>` (sin spec: `<cd>hoom verify`) |
| `verde-vencido` | el último veredicto completo de la tarjeta es verde y la huella actual de su árbol es otra | `el veredicto verde <id> certifica la huella <vieja> y el arbol tiene <actual>` | `el codigo cambio despues del ultimo verde` | `<cd>hoom verify --spec <spec>` |
| `bug-derivacion` | la tarjeta está en Tu aceptación y tiene un hallazgo abierto de severidad `high` con su `task` | `bug de hoom: la tarjeta esta en Tu aceptacion con el hallazgo high abierto <id>, y la derivacion nunca deberia permitirlo` | `hay un error de hoom en esta tarjeta: no la integres` | `reporta este bug de hoom con la salida de 'hoom board doctor --json' y no integres la tarjeta hasta entonces` |
| `sobre-huerfano` | la tarjeta tiene `interrupted` | `el sobre <id> del <rol> quedo abierto en el paso <x> de <n> (<stage>) y su proceso ya no vive` | `el trabajo del <rol> quedo interrumpido` | con `reanudar` habilitada: `hoom agent --role <rol> --task <slug>[ --spec <spec>] --provider <p> --resume <resume_id> "<pedido>"`; si el rol es `reviewer`: `hoom review --task <slug> --spec <spec>`; si no: `hoom agent --role <rol> --task <slug>[ --spec <spec>] "<pedido>"` |
| `sin-spec` | no hay spec y el item tiene más de 14 días (`now - creado_en`) | `el item lleva <d> dias sin spec (creado el <AAAA-MM-DD>)` | `lleva <d> dias esperando su spec` | el `next` de la tarjeta |

`--spec <spec>` va en las acciones de `hoom agent` salvo para los roles que
escriben specs, como en C3. `"<pedido>"` es el marcador que el sobre rechaza
a propósito, como en `next`.

`bug-derivacion` no debería aparecer nunca: la columna Review retiene toda
tarjeta con un hallazgo de la tarea que bloquea (C1), y `high` bloquea con
cualquier umbral. Si aparece, es un bug de hoom y se reporta como tal.

Los del proyecto, después de los de todas las tarjetas:

| `id` | cuándo | `what` | `action` |
|---|---|---|---|
| `sobre-huerfano` | un registro de sobre abierto cuyo dueño no vive (la regla de `Gather`), sin `task` o con una `task` que no tiene item, y que ningún sobre posterior de la misma `task` relevó (C3) | `el sobre <id> del <rol> quedo abierto en el paso <stage> y no es de ninguna tarjeta` | `es telemetria local: si ya no te sirve, borralo con rm <ruta del registro>` |
| `sin-veredicto` | un espacio de trabajo de `hoom task` sin item, con cambios y sin ningún veredicto completo de ese árbol que certifique su huella actual | `el espacio de trabajo <dir> tiene cambios que ningun veredicto certifica (huella <huella>)` | `cd <dir> && hoom verify --spec .hoom/specs/<slug>.md` si ese spec existe ahí; si no, `cd <dir> && hoom verify` |
| `evidencia-sin-item` | una tarea con veredictos (su `spec` = `.hoom/specs/<t>.md`) o hallazgos (`task` = `<t>`), en la raíz o en un espacio de trabajo, y sin archivo de item `<t>` | `hay <n> veredictos y <m> hallazgos de la tarea <t> y ningun item la representa` (`1 veredicto`, `1 hallazgo`; se omite la parte en 0) | `hoom item add "<titulo>" --slug <t>` |

Uno por sobre, por espacio de trabajo y por tarea, ordenados por su `ref` o
su tarea. Un archivo de item inválido cuenta como item: el tablero ya lo
avisa en `warnings`.

`Evidence` gana `TreeFingerprint` (la huella actual del árbol de evidencia
cuando la tarjeta tiene espacio de trabajo o veredicto), `TreeChanges` (los
archivos de la candidata de ese árbol, solo con espacio de trabajo) y
`Certified` (algún veredicto completo del árbol tiene esa huella). El campo
`Fingerprint` de C1 y el `fingerprint` del detalle no cambian: siguen vacíos
sin veredicto.

#### `hoom board doctor [--json]`

- `boardcmd.Doctor(root, base, blockOn string, now time.Time) (DoctorReport,
  error)` arma el tablero con `Build`, junta los `doctor` de sus tarjetas en
  el orden del tablero y agrega los del proyecto. Solo lee.
- `--json`: `{"problems": [...]}`, lista nunca `null`.
- El texto:

```
hoom board doctor: 2 problemas
  verde-vencido  items-y-columna-derivada
    el veredicto verde 2026-09-23T06-10-06Z_ab41dc52 certifica la huella 1a2b3c4d5e6f7a8b y el arbol tiene 9f8e7d6c5b4a3a2b
    Accion: hoom verify --spec .hoom/specs/items-y-columna-derivada.md
  evidencia-sin-item  verify-args-estrictos
    hay 2 veredictos de la tarea verify-args-estrictos y ningun item la representa
    Accion: hoom item add "<titulo>" --slug verify-args-estrictos
```

  Con un solo problema, `1 problema`. Sin problemas: `hoom board doctor: sin
  problemas de coherencia`. Un problema del proyecto sin tarea muestra `-`
  en lugar del slug.
- Sale con 0 siempre que entendió el pedido: el doctor informa, como `hoom
  board`; los que bloquean son `verify` y `check`.
- `boardcmd.ParseDoctorArgs` es estricto (`*cliargs.UsageError` con su bloque
  de uso, `-h` es `ErrHelp`). `main` despacha `hoom board doctor ...` a él
  antes de `ParseArgs`, que no cambia: `hoom board x` sigue siendo un error de
  uso (CA-288). `hoom help` suma `board doctor`.

#### Las insignias en el tablero

- `tablero.js` pinta en cada tarjeta con `doctor` no vacío una insignia
  `.tdoc` con la cantidad. Su título es la lista de `plain` en modo normal, y
  de `what` más `Accion: <action>` en modo experto.
- La pestaña Qué del detalle lista los problemas de la tarjeta: `plain` en
  modo normal, y `what` con su acción en experto.

### 4. La correlación tool_use/tool_result (Claude)

Verificado contra Claude Code 2.1.281 (`claude -p --output-format
stream-json --verbose`): la delegación es un bloque `tool_use` con `id`,
`name` = `Agent` (las versiones hasta 2.1.263 lo llamaban `Task`) e `input`
con `subagent_type` y `run_in_background`; su resultado llega en un mensaje
`user` con un bloque `tool_result` que lleva `tool_use_id` (e `is_error:
true` si falló), y además Claude escribe líneas `system` `task_started`,
`task_updated` y `task_notification` (esta con `tool_use_id` y `status`).

- **El nombre.** El parser reconoce la delegación con `name` `Agent` o
  `Task`. Hoy solo reconoce `Task`, así que con la versión actual de Claude
  las delegaciones no se ven como subagentes: es un bug que esta spec
  corrige.
- `providers.Event` gana `ToolID` (`tool_id`, omitido si vacío). El evento
  `agent` lo lleva cuando el `tool_use` trae `id`.
- **Kind nuevo `agent_end`**: un subagente delegado salió de escena. Lleva
  `agent` (el mismo `subagent_type`), `tool_id` y `detail`: `<agente>
  termino`, o `<agente> fallo: <texto recortado>` si el resultado trae
  `is_error: true` o si `task_notification` trae un `status` distinto de
  `completed` (sin texto: `<agente> fallo (<status>)`).
- **El emparejamiento necesita memoria del run.** `providers` gana la
  interfaz opcional `Correlating { NewNormalizer() func(line string) []Event
  }`. Claude la implementa: su normalizador recuerda los `id` de sus
  delegaciones y, para cada una, emite **un solo** `agent_end`:
  - delegación normal: con el primero que llegue de su `tool_result` o de su
    `task_notification` con estado terminal;
  - delegación en segundo plano (`run_in_background: true`): solo con su
    `task_notification`, porque su `tool_result` llega enseguida y solo dice
    que arrancó.
  Un `tool_result` o una `task_notification` de un id que no es una
  delegación no emite nada nuevo. La línea `system` de `task_notification`
  sigue emitiendo su evento `system` de siempre, y el `agent_end` va
  después.
- `runcmd` pide un normalizador por run cuando el provider implementa
  `Correlating`, y usa `Normalize` si no. El `Normalize` sin memoria de
  Claude sigue igual salvo por `tool_id` y el nombre `Agent`: nunca emite
  `agent_end`.
- **El escenario** (`runcmd.Stage`): `Actor` gana `open`, las delegaciones
  suyas con `tool_id` que todavía no tienen su `agent_end`. Con el run en
  curso, el orquestador está activo y un subagente está activo si y solo si
  `open > 0`. Si ningún evento `agent` del run trae `tool_id` (logs viejos, u
  otro provider), rige la regla de siempre: el último delegado está activo
  mientras el run corre. `agent_end` no suma actos ni cambia `last_detail`, y
  uno sin su `agent` se ignora. Con el run terminado nadie está activo, como
  hoy.
- El Feed muestra los `agent_end` como cualquier evento, con su clase
  `k-agent_end`.

### 5. El trinquete en el Studio

- `statuscmd.RatchetMetric` gana `history`: los movimientos de esa métrica en
  el `history` de `.hoom/ratchet.json` (`ts`, `metric`, `from`, `to`, `kind`,
  `reason`), del más viejo al más nuevo, lista nunca `null`. `hoom status
  --json` lo trae; el texto de `hoom status` no cambia.
- `statuscmd.Ratchet(root string) RatchetView` (la función que hoy es
  `ratchetView`) es la única fuente: la usan `hoom status` y `GET
  /api/ratchet`.
- `GET /api/ratchet` responde ese `RatchetView`: los mismos datos que la
  clave `ratchet` de `hoom status --json`. Sin línea base, `declared: false`
  y `metrics: []`; con un archivo ilegible, `declared: true` y `error`. Sin
  token, otro método es 405, y no modifica nada.
- **La curva**: la pestaña Cockpit gana una sección **Trinquete** que
  `index.html` pinta desde `/api/ratchet` (con `j`, en el refresco de
  siempre): por métrica, su nombre, su dirección, la base vigente y un SVG en
  línea con la curva en escalón de sus movimientos (eje x el tiempo, eje y
  `to`), marcando en otro color los `loosened`. Una métrica declarada sin
  congelar dice `sin congelar`. Sin línea base, la sección dice `Sin línea
  base: 'hoom ratchet init' la crea y 'hoom verify --full' la congela.` Sin
  assets de red.

### 6. El piloto automático: una cinta transportadora, no un orquestador

#### El item

- El item gana la clave opcional `auto`, con un solo valor: `hasta-humano`.
  Otro valor es inválido (`auto "<v>" fuera del vocabulario (hasta-humano)`).
  Un item sin `auto` se lee y se escribe byte a byte como antes.
- `hoom item add ... --auto hasta-humano` la escribe, y exige
  `--presupuesto-usd` (error de uso: `--auto necesita --presupuesto-usd: el
  piloto automatico no corre sin tope`).
- El piso del sobre ya prohíbe que un rol toque `.hoom/items/`: un agente no
  puede activarse la cinta.

#### La decisión: `boardcmd.Pilot`

`Pilot(before, after Card, closed envelope.Record) PilotDecision` es pura.
`before` es la tarjeta que el Studio derivó para lanzar el trabajo, `closed`
el registro del sobre (o de la review) cuando la función del trabajo volvió,
y `after` la tarjeta derivada de nuevo en ese momento.

```go
type PilotDecision struct {
    Launch    bool
    Action    string  // la acción de la tabla: pedir-*
    Role      string
    Provider  string
    BudgetUSD float64
    Pedido    string
    Why       string  // por qué se detiene; "" cuando lanza
}
```

**La tabla** (`boardcmd.PilotTable`), el orden fijo de C1 para las columnas
de rol a las que una tarjeta puede avanzar: `arquitecto` → `pedir-arquitecto`,
`test-writer` → `pedir-test-writer`, `writer` → `pedir-writer`, `review` →
`pedir-reviewer`. Nada más.

Se detiene en el primer caso que se cumple, con este `Why`:

| # | caso | `Why` |
|---|---|---|
| 1 | `after.item.auto` no es `hasta-humano` | `el piloto automatico no esta activado en la tarjeta` |
| 2 | `closed.status` no es `entregable` | `el trabajo del <rol> no cerro entregable (<status>): el piloto se detiene ante un rojo` (sin status: `sin cerrar`) |
| 3 | `after.red` no es `null` | `la tarjeta esta en rojo: <red.plain>` |
| 4 | `after` tiene `running` o `interrupted` | `la tarjeta tiene otro trabajo en curso o interrumpido` |
| 5 | la columna de `after` es humana o Hecho | `la tarjeta llego a <Nombre>: le toca a una persona` (Hecho: `la tarjeta llego a Hecho: no queda trabajo`) |
| 6 | la columna de `after` no está después de la de `before` | `la tarjeta no avanzo (sigue en <Nombre>): el piloto no reintenta` |
| 7 | la acción principal de `after` no es la de la tabla para su columna (o no hay) | `lo que sigue en <Nombre> no esta en la tabla del piloto: le toca a una persona` |
| 8 | el rol de esa acción es el de `closed` | `seria volver a pedirle al <rol>: el piloto no reintenta` |
| 9 | la acción está deshabilitada | su `why` (incluye el presupuesto agotado de C3) |
| 10 | el item no tiene `presupuesto_usd` | `la tarjeta no tiene presupuesto: el piloto automatico solo corre con un tope` |
| 11 | ninguna opción de provider de la acción está `ok` y tiene `budget` | `ningun proveedor con tope de presupuesto puede tomar este trabajo` |

Si no se detiene, lanza la acción de la tabla con:

- **provider**: el de `closed` si es una opción `ok` con `budget`; si no, la
  primera opción `ok` con `budget`, en el orden del registro. En la review,
  la opción del writer ya no es `ok` (C3), así que nunca revisa quien
  escribió.
- **presupuesto**: el `budget_usd` que propone la acción (lo que queda, o lo
  que queda dividido 4 en la review).
- **pedido**: el `pedido` que propone la acción.

Como cada lanzamiento exige que la columna avance y que cambie el rol, y la
cinta se detiene en las columnas humanas, la cinta nunca pide al arquitecto
(solo se llega a Arquitecto desde Backlog, con el arquitecto) y una persona
que pide un trabajo encadena como mucho tres más: test-writer, writer y
reviewer, en ese orden. La cinta siempre termina.

#### Dónde corre: `hoom serve`

- Cuando la función de un trabajo que lanzó el Studio vuelve (el `launch` de
  C3, o un lanzamiento de la cinta), el Studio lee el registro del sobre,
  vuelve a derivar la tarjeta y llama a `Pilot`. Si `after.item.auto` no es
  `hasta-humano`, no hace nada más.
- Si no, agrega una línea al log del sobre que cerró
  (`.hoom/envelopes/<id>.log`): `piloto automatico: se detiene: <Why>`, o
  `piloto automatico: pide al <rol> con <provider> (<presupuesto> USD): sobre
  <id nuevo>`, o `piloto automatico: no pudo lanzar al <rol>: <error>`.
- Lanza con la **misma función** que el endpoint `launch`, con todas sus
  validaciones (candado por tarjeta, tarjeta derivada de nuevo, acción
  presente y habilitada, provider `ok`, presupuesto, árbol sin run activo), y
  sin token porque no entra por HTTP. El trabajo lanzado vuelve a pasar por
  la cinta cuando termina.
- `agentcmd.Options` y `reviewcmd.Options` ganan `Pilot bool`, y el registro
  del sobre gana `piloto` (omitido si es `false`): un sobre de la cinta lo
  dice, y la historia lo muestra.
- Si `hoom serve` se cae, la cinta se corta con él: el sobre en curso queda
  interrumpido (C3) y nadie lo relanza.

#### La tarjeta en el Studio

- `tablero.js` pinta el chip `piloto automático` en una tarjeta cuyo item
  tiene `auto: hasta-humano`.
- El diálogo de rol de `acciones.js`, con esa tarjeta, avisa: `Piloto
  automático: si este trabajo termina bien, la tarjeta sigue sola hasta la
  próxima columna tuya o hasta que se agote su presupuesto.`

### Documentación

- README: la sección del Studio describe la pestaña Historia y el replay,
  `GET /api/board/{slug}/timeline`, las insignias del doctor, `GET
  /api/ratchet` y la curva, y el piloto automático con la frase "una cinta
  transportadora, no un orquestador". La tabla de verbos suma `hoom board
  doctor`. El roadmap de la cabina pierde "Curva del trinquete en el tablero"
  y "Timeline, replay y doctor".

## Casos límite y errores esperados

- Una tarjeta recién creada, sin commit: la historia no tiene entradas de git
  y el replay no se ofrece. La tarjeta de hoy igual está en `card`.
- Una tarjeta de otra computadora después de un pull: la historia trae su
  evidencia de git y la nota de que no hay telemetría.
- Un spec aprobado, editado y aprobado de nuevo: el replay enciende `spec`,
  lo apaga con la edición y lo vuelve a encender con la segunda aprobación.
- Spec y aprobación en el mismo commit: `spec` termina encendido.
- Un veredicto rojo y después uno verde: los gates se apagan y se vuelven a
  encender.
- Tests y código en un solo commit (el flujo de C3): todos los criterios se
  encienden juntos en ese commit.
- Una tarjeta integrada con `git merge --no-ff`: sus commits salen de
  `M^1..commit_final` aunque la rama `hoom/<slug>` ya no exista. Integrada
  con fast-forward: la nota y ninguna entrada `commit`.
- Un run viejo sin `tool_id`: sus delegaciones aparecen como `subagente` sin
  `ended_at` y sin `subagente-fin`, y el escenario usa la regla de siempre.
- Una delegación en segundo plano: su `tool_result` inmediato no la cierra;
  la cierra su `task_notification`. Un run que termina con una delegación
  abierta la deja con `open` 1 y nadie activo.
- Dos delegaciones al mismo subagente en paralelo: `open` 2, y el subagente
  sigue en escena hasta que cierren las dos.
- Un `tool_result` de `Read` o `Bash`: no emite nada nuevo.
- `hoom board doctor` en un proyecto sin items: solo los problemas del
  proyecto, o `sin problemas de coherencia`.
- hoomai mismo: sus tarjetas de C1 y C2 dan `verde-vencido` (flujo de Orca,
  brecha 6) y las tareas de specs anteriores a la cabina dan
  `evidencia-sin-item`. Es la verdad de hoy, no un falso positivo.
- Un espacio de trabajo con un veredicto rojo de la huella actual: no es
  `sin-veredicto` (hay veredicto; la tarjeta ya lo dice en rojo).
- Un espacio de trabajo con un verde viejo y cambios después: es
  `verde-vencido`, no `sin-veredicto`.
- Un sobre interrumpido y relevado por otro (C3): no es huérfano.
- Un item con `auto` y sin presupuesto: se lee bien; la cinta se detiene en el
  caso 10 y lo escribe en el log.
- Writer con Claude termina verde y la tarjeta pasa a Review con la review
  exigida: la cinta pide al reviewer con otro provider con tope, con lo que
  queda dividido 4. Si el único otro provider es Codex (sin tope), se detiene
  en el caso 11.
- El reviewer abre un hallazgo high: la tarjeta sigue en Review y la cinta se
  detiene (caso 6). Corregir es de una persona.
- El arquitecto entrega un spec que no pasa lint: la tarjeta pasa de Backlog a
  Arquitecto, y la cinta se detiene en el caso 8.
- El test-writer cierra no-entregable: la cinta se detiene en el caso 2, y la
  tarjeta muestra su rojo.
- La persona apaga `auto` en el item mientras corre el writer: cuando el
  writer termina, la cinta se detiene en el caso 1.
- Dos pestañas: la cinta y una persona lanzan a la vez sobre la misma
  tarjeta: una gana el candado y la otra recibe el 409 de C3 (la cinta lo
  escribe en el log).
- `GET /api/board/S/timeline` con `S` inválido: 400; con un item que no
  existe: 404. `POST`: 405.
- `/api/ratchet` en hoomai (sin `.hoom/ratchet.json`): `declared: false`, y la
  sección dice que no hay línea base.

## Criterios de aceptación

- CA-359: `TimelineFor` trae una entrada `git` por cada par (commit, archivo) de la tarjeta, con `kind` de la tabla (`item`, `spec`, `aprobacion`, `veredicto`, `hallazgo`, `resolucion`, `review`), su `plain` según la letra A/M/D, `who` = el autor del commit, `ref` = el sha completo, `artifact` relativo a la raíz (con el prefijo del espacio de trabajo cuando corresponde) y `cost_usd` `null`. El item se lee en la raíz y lo demás en el árbol de evidencia, y un archivo ajeno a la tarjeta no aparece. Las entradas van en orden ascendente por `at`, con el desempate del contrato, y ninguna lista es `null`.
- CA-360: los commits de la tarea son `kind: commit`, sin merges, con su autor, su sha, su asunto y la cantidad de archivos, en el rango del contrato: `<base>..HEAD` con espacio de trabajo, `<base>..hoom/<slug>` sin él, `M^1..<commit_final>` en una tarjeta integrada con `--no-ff` (aunque la rama ya no exista), y ninguno con la nota del contrato si se integró con fast-forward o si la tarjeta no tiene espacio de trabajo propio.
- CA-361: la telemetría de la tarjeta da una entrada `sobre` por registro (con `ended_at` `null` si no cerró, el `plain` de su estado, `who` = `<rol> (<provider>)` y `piloto` del registro), una `run` por cada run que ningún sobre referencia, y el costo de cada run una sola vez, con `cost_usd` `null` sin dato. Sin sobres ni runs, no hay entradas de telemetría y `notes` trae `sin telemetria en esta computadora: el trabajo de los agentes solo se ve donde corrio`, y `telemetry` cuenta sobres y runs.
- CA-362: cada evento `agent` de los runs de la tarjeta da una entrada `subagente` y cada `agent_end` emparejado por `tool_id` una `subagente-fin`, con `ended_at` de la delegación = el `ts` de su `agent_end`, o `null` si no llegó (también en un run viejo sin `tool_id`). Las delegaciones tienen `cost_usd` `null`.
- CA-363: `meter` es el esqueleto de `card.meter` y cada entrada trae sus efectos: `spec` se enciende con una aprobación del contenido del spec de ese punto y se apaga con una edición sin aprobación (y vuelve con la segunda aprobación); un commit de la tarea enciende los criterios cuyo token aparece por primera vez en una línea agregada de un archivo de test; un veredicto completo pone `build`, `static` y `test` según sus gates, enciende todos los criterios si pasó `spec_trace` y pone `review` en `no-aplica` si el cambio es chico; un registro de review con las 4 lentes pone `review` en `hecho`; un veredicto parcial, un hallazgo, una resolución, el item y la telemetría traen `[]`. `card` es la tarjeta de `CardFor`.
- CA-364: en todos los fixtures de la historia, cada `plain` es no vacío y no contiene `git`, `commit`, `worktree`, `merge`, `rama`, `branch`, `diff`, `HEAD` ni `huella` (palabras enteras, sin distinguir mayúsculas), ni `.hoom/` ni `hoom `.
- CA-365: `GET /api/board/{slug}/timeline` responde 200 con el JSON de `TimelineFor`, 400 con un slug inválido y 404 con un item que no existe (mensaje de `CardFor`). No pide token, `POST` es 405, y pedirlo deja el repo y `.hoom/` byte a byte iguales, ignorados incluidos.
- CA-366: `index.html` tiene la pestaña de detalle "Historia" (`data-pane="historia"`) y el panel `tb-p-historia`. `tablero.js` pide `/api/board/` + slug + `/timeline`, usa `entries`, `meter`, `source`, `plain`, `summary`, `piloto`, `notes` y `card`, rotula las fuentes con "guardado", "esta computadora", "git" y "telemetría local", tiene "Reproducir", "Pausa", "Reiniciar", "Así está hoy" y "Todavía no hay historia para esta tarjeta.", avanza el replay con `setTimeout` y sigue cumpliendo CA-320 y CA-322 sin cambios en sus tests.
- CA-367: `Derive` trae `doctor` en cada tarjeta, nunca `null`, con los seis problemas de tarjeta del contrato en su orden, cada uno con su `id`, `what`, `plain`, `action` y `ref` exactos, en fixtures que los disparan uno por uno: `spec-editado`, `sin-veredicto` (y no con un rojo de la huella actual ni con un verde viejo), `verde-vencido`, `sobre-huerfano` (con la acción de reanudar, la de la review y la de volver a lanzar), `sin-spec` (a los 15 días sí, a los 14 no). `<cd>` aparece solo cuando el árbol de evidencia es el espacio de trabajo. Una tarjeta sana tiene `doctor: []`, y una en Hecho solo puede tener `sobre-huerfano`. Los `plain` cumplen las reglas de CA-364. El texto de `hoom board` y de `hoom item show` no cambia, y el `fingerprint` del detalle sigue vacío sin veredicto (CA-313 pasa sin cambios).
- CA-368: `bug-derivacion` se reporta con su `what`, su `plain` y su acción cuando la función del doctor recibe una tarjeta en Tu aceptación con un hallazgo `high` abierto de su tarea, y ninguna tarjeta derivada por `Derive` en los fixtures de C1 a C4 lo trae.
- CA-369: `Doctor` agrega, después de los de las tarjetas, los problemas del proyecto: un `sobre-huerfano` por cada sobre abierto sin dueño vivo, sin tarea o con una tarea sin item, que ningún sobre posterior de su tarea relevó; un `sin-veredicto` por cada espacio de trabajo sin item con cambios y sin veredicto de su huella; y un `evidencia-sin-item` por tarea con veredictos o hallazgos y sin archivo de item (uno inválido cuenta como item), con los textos y acciones del contrato. `Doctor` deja el repo y `.hoom/` byte a byte iguales.
- CA-370: `hoom board doctor` imprime el texto del contrato (cabecera con `1 problema` o `<n> problemas`, o `sin problemas de coherencia`; por problema su `id`, el slug o `-`, `what` y `Accion: <action>`), `--json` emite `{"problems": [...]}` y los dos salen con 0. `ParseDoctorArgs` rechaza un argumento o flag desconocido con `*cliargs.UsageError` que lleva su bloque de uso y da `ErrHelp` con `-h`, y los tests de CA-288 pasan sin cambios.
- CA-371: `tablero.js` pinta la insignia `.tdoc` con la cantidad de `doctor` en cada tarjeta que tiene problemas, con los `plain` como título en modo normal y `what` y `Accion` en experto, y la pestaña Qué lista los problemas de la tarjeta.
- CA-372: el `Normalize` de Claude reconoce la delegación con `name` `Agent` y con `Task`, y su evento `agent` lleva `tool_id` cuando el `tool_use` trae `id`. Nunca emite `agent_end`. Los tests de providers que ya existen pasan sin cambios.
- CA-373: el normalizador de `Correlating` de Claude emite un solo `agent_end` por delegación, con `agent`, `tool_id` y `detail` `<agente> termino`, o `<agente> fallo: ...` con `is_error: true` o `<agente> fallo (<status>)` con una `task_notification` no `completed`: una delegación normal cierra con el primero de su `tool_result` o su `task_notification`, y una en segundo plano solo con su `task_notification`. Un `tool_result` de otra herramienta o de un id desconocido no emite nada nuevo, y la `task_notification` sigue emitiendo su `system`. Un fixture con las líneas reales de Claude Code 2.1.281 (tool_use `Agent`, `task_started`, `task_notification`, `tool_result`) da `agent` y después un único `agent_end` con el mismo `tool_id`.
- CA-374: `runcmd` usa un normalizador de `NewNormalizer` por run cuando el provider implementa `Correlating`, y `Normalize` si no: el `.jsonl` de un run de Claude con una delegación trae su `agent_end`.
- CA-375: `runcmd.Stage` da a cada actor `open` = sus delegaciones con `tool_id` sin `agent_end`; con el run en curso un subagente está activo si y solo si `open > 0` y el orquestador siempre; sin ningún `tool_id` en el run rige la regla del último delegado; `agent_end` no suma actos ni cambia `last_detail`, y uno sin su `agent` se ignora. Con el run terminado nadie está activo. Los tests de CA-32 a CA-38 pasan sin cambios.
- CA-376: `statuscmd.Ratchet` trae en cada métrica `history` con sus movimientos del más viejo al más nuevo (lista nunca `null`), `hoom status --json` lo emite y el texto de `hoom status` no cambia. `GET /api/ratchet` responde el mismo `RatchetView`: con línea base, sus métricas y movimientos; sin ella, `declared: false` y `metrics: []`; ilegible, `declared: true` y `error`. No pide token, `POST` es 405 y no modifica nada.
- CA-377: `index.html` tiene la sección "Trinquete" en la pestaña Cockpit, pide `/api/ratchet` con `j(`, pinta la curva con `<svg` desde `history`, distingue `loosened`, dice "sin congelar" y "Sin línea base", y el test de UI sin assets de red sigue pasando.
- CA-378: `item.Parse` acepta `auto: hasta-humano`, rechaza otro valor con el mensaje del contrato y sigue rechazando claves desconocidas. Un item sin `auto` se lee y se escribe byte a byte igual. `hoom item add --auto hasta-humano --presupuesto-usd 5` lo escribe, y `--auto` sin `--presupuesto-usd` es un error de uso con el mensaje del contrato.
- CA-379: `Pilot` se detiene en cada uno de los once casos del contrato con su `Why` exacto, con un fixture por caso, y lanza en los encadenamientos de la tabla: un test-writer entregable cuya tarjeta pasa a Writer lanza al writer, un writer entregable cuya tarjeta pasa a Review con la review exigida lanza al reviewer, y un arquitecto entregable cuya tarjeta salta a Test-writer (su spec ya tenía aprobación vigente) lanza al test-writer; siempre con el provider de `closed` si es `ok` y tiene tope, o la primera opción `ok` con tope, y con el `budget_usd` y el `pedido` que propone la acción. En la review nunca elige al writer.
- CA-380: la cinta es una cinta transportadora, no un orquestador. Sobre todas las combinaciones de fixtures (las ocho columnas de `before` y de `after`, los cuatro estados de `closed`, con y sin `red`, `running` o `interrupted`, con y sin presupuesto), cada vez que `Pilot` lanza: la acción es la de `PilotTable` para la columna de `after` (nunca un rol fuera de la tabla), `closed` cerró `entregable`, `after` no tiene `red`, la columna avanzó, no es humana ni Hecho y el rol no es el de `closed` (nunca relanza tras un rojo ni reintenta). `Pilot` nunca lanza `pedir-arquitecto`, y encadenado sobre cualquier secuencia de fixtures lanza como mucho tres veces seguidas.
- CA-381: en el Studio, con un provider de prueba: un `pedir-test-writer` que termina entregable sobre una tarjeta con `auto: hasta-humano` y presupuesto lanza el sobre del writer sin nuevo POST, con `piloto: true` en su registro, y el log del primer sobre trae `piloto automatico: pide al writer con <provider> (<presupuesto> USD): sobre <id>`. Un trabajo que cierra no-entregable, una tarjeta sin `auto` y una que llega a Tu aceptación no lanzan nada; los dos primeros casos con `auto` escriben `piloto automatico: se detiene: <Why>` y sin `auto` el log no gana ninguna línea. Un lanzamiento de la cinta que choca con el candado de la tarjeta escribe `piloto automatico: no pudo lanzar al <rol>: ...`.
- CA-382: `agentcmd.Options` y `reviewcmd.Options` tienen `Pilot`, y con `Pilot` el registro del sobre trae `"piloto": true`; sin él, el registro no trae la clave.
- CA-383: `tablero.js` pinta el chip "piloto automático" en una tarjeta cuyo `item.auto` es `hasta-humano`, y `acciones.js` tiene el aviso "Piloto automático: si este trabajo termina bien" en el diálogo de rol y sigue cumpliendo CA-354 y CA-355 sin cambios en sus tests.
- CA-384: sin dependencias nuevas: `go.mod` y `go.sum` no cambian respecto de la rama base. [verifica: git diff --quiet main -- go.mod go.sum]
- CA-385: dogfood: hoomai tiene el item de esta spec. [verifica: go run ./cmd/hoom item show historia-doctor-y-cinta --json | grep -q '"slug": "historia-doctor-y-cinta"']
- CA-386: el README documenta la historia, el doctor y la cinta. [verifica: grep -q "/api/board/{slug}/timeline" README.md && grep -q "hoom board doctor" README.md && grep -q "una cinta transportadora, no un orquestador" README.md]
- CA-387: `hoom help` lista `board doctor`. [verifica: go run ./cmd/hoom help | grep -q "board doctor"]

## Decisiones

- **La historia no se guarda: se calcula.** Como el tablero, sale de git y de
  la telemetría cada vez que alguien abre la pestaña. Un log propio sería una
  segunda verdad (RFC). Por eso la pestaña no sondea: pedir git cada segundo
  no aporta nada.
- **Una entrada por archivo y por commit**, no por commit: el replay enciende
  el medidor artefacto por artefacto, y "qué artefacto" es un dato de cada
  entrada.
- **Fecha de autor, no de commit.** Un rebase o un cherry-pick no mueven
  cuándo se hizo el trabajo.
- **La telemetría se rotula y se cuenta, no se esconde.** En modo normal la
  fuente se lee "guardado" o "esta computadora": el junior entiende por qué
  en otra máquina faltan los agentes.
- **El replay enciende cuando llega la evidencia y no recalcula el pasado.**
  Recalcular la huella de cada commit viejo exigiría rearmar el árbol de cada
  punto. El replay dice cuándo apareció cada evidencia; lo que vale hoy lo
  dice la tarjeta, que es su último cuadro ("Así está hoy"). Las dos
  excepciones son baratas y exactas: la aprobación se compara con el SHA-256
  del spec de ese commit, y un veredicto rojo apaga sus gates.
- **Los criterios se encienden con el primer test que los cita**, leyendo
  las líneas agregadas de los commits de la tarea con la misma regla de
  archivo de test que `spec_trace`. Si el parche es enorme, se corta en 8 MiB
  y el veredicto los enciende igual.
- **Los commits de una tarjeta integrada** salen del merge que la integró
  (`M^1..commit_final`), porque después del merge `base..hoom/<slug>` está
  vacío y la rama puede no existir. Con fast-forward no hay forma honesta de
  separarlos, y la historia lo dice.
- **Sin verbo para la historia**, como el detalle de C2: es una lectura del
  Studio sobre una función de Go con tests. Pendiente de confirmar: si la
  querés en la terminal, el verbo sería `hoom item history <slug> [--json]`
  sobre la misma función.
- **Los problemas de una tarjeta viajan en la tarjeta** (`doctor`, derivado
  en `Derive`), así las insignias no piden un endpoint aparte y `hoom item
  show --json` dice lo mismo que el tablero. Los del proyecto solo los
  lista el verbo.
- **`sin-veredicto` y `verde-vencido` no se pisan.** Un verde viejo con
  cambios después es `verde-vencido`; `sin-veredicto` es un espacio de
  trabajo con cambios que ningún veredicto vio, cuando el último no es verde.
  Un rojo de la huella actual no es ninguno de los dos: ya está verificado, y
  la tarjeta lo dice en rojo.
- **Un sobre relevado no es huérfano** (C3): la persona ya decidió.
- **14 días sin spec**, fijo en el binario, como las 400 líneas de la
  review. Pendiente de confirmar el número.
- **El doctor sale con 0.** Informa, como `hoom board` y `hoom context`; los
  que bloquean son `verify` y `check`. Con problemas no rompe un script que
  lo corre para mirar.
- **Un sobre huérfano sin tarjeta se puede borrar.** Es telemetría local
  fuera de git: hoom no lo cierra (no vio cerrarlo), pero la persona puede
  descartarlo. Es la única acción del doctor que borra, y la corre la
  persona.
- **`bug-derivacion` se chequea aunque no pueda pasar.** Es una defensa en
  profundidad: si la derivación de C1 se rompe, el doctor lo dice antes de
  que alguien integre.
- **Claude llama `Agent` a la delegación desde alguna versión posterior a
  2.1.263** (verificado en 2.1.281). Reconocer los dos nombres arregla que
  hoy el escenario no vea ningún subagente.
- **El emparejamiento vive en el adapter, con memoria por run**
  (`Correlating`), porque el pedido lo pone en el parser de Claude y porque un
  `tool_result` suelto no dice si su id era una delegación. Emitir un evento
  por cada `tool_result` duplicaría el Feed.
- **La delegación en segundo plano cierra con `task_notification`**: su
  `tool_result` llega enseguida y solo dice que arrancó.
- **La curva del trinquete dibuja los movimientos de la línea base**, que
  son lo que el archivo prueba. Las mediciones que no movieron la base no se
  guardan en ningún lado, y hoom no las inventa.
- **La cinta corre solo en `hoom serve`**, después de un trabajo que lanzó
  el Studio: el Studio es dueño de la goroutine, del candado de la tarjeta y
  del log del sobre. Desde la terminal, la cinta es la persona. Pendiente de
  confirmar.
- **La cinta exige presupuesto en el item y un provider con tope.** "Se
  detiene cuando el presupuesto se agota" no significa nada sin presupuesto,
  y un provider sin tope (Codex) no reporta costo, así que su gasto nunca
  agotaría nada. Con esto, la cinta nunca gasta sin un tope que la corte.
  Pendiente de confirmar.
- **La cinta sigue el provider que eligió la persona** (el del trabajo que
  cerró) mientras pueda, y si no, el primero con tope. Es una regla fija,
  no una elección.
- **La cinta exige que la columna avance y que cambie el rol.** Es lo que la
  hace una cinta: si el trabajo no ganó su columna, lo que sigue es de una
  persona. También garantiza que termina.
- **Corregir hallazgos no es de la cinta.** En Review la tabla dice reviewer;
  si hay hallazgos que bloquean, la acción principal es `pedir-writer` (C3),
  que no está en la tabla, y la cinta se detiene.
- **Los lanzamientos de la cinta dejan `piloto: true` en el registro del
  sobre**: la historia y el log dicen quién decidió cada trabajo, una persona
  o la cinta.
- **El aviso del diálogo** hace consciente el encadenamiento en el momento en
  que la persona confirma el primer trabajo.

## Riesgos y deuda aceptada

- **La historia de un proyecto grande es lenta.** `git log` sobre los
  archivos de la tarjeta y el parche de sus commits de tarea se leen cada vez
  que se abre la pestaña (con el tope de 8 MiB). La cache regenerable del
  roadmap sigue disponible si hace falta.
- **El replay no es la verdad del pasado.** Un veredicto verde que después
  quedó vencido por un commit sigue encendido en el replay hasta que otro
  veredicto lo cambie. El último cuadro lo corrige.
- **Las entradas de git dependen de los commits.** Evidencia sin guardar no
  está en la historia hasta que se guarda: la tarjeta de hoy la muestra en
  `unsynced`.
- **La historia de un spec renombrado o movido se corta** (`--no-renames`):
  el slug es la llave y no se sigue un archivo por su contenido.
- **El formato del stream de Claude cambia sin aviso.** El nombre de la
  delegación ya cambió una vez. Los fixtures con las líneas reales de 2.1.281
  lo fijan, y una versión que cambie la forma vuelve a la regla de siempre en
  el escenario sin romper nada.
- **La cinta corre dentro del Studio.** Si se cae, se corta, y el sobre queda
  interrumpido (C3).
- **La cinta confía en el presupuesto reportado en esta computadora** (C2 y
  C3): el gasto de otra computadora no descuenta.
- **hoomai va a tener muchos `evidencia-sin-item`**: las specs anteriores a la
  cabina no tienen items. Es la verdad; se apaga creando sus items o no.
- **Los tests de la UI son estáticos**, como en C2 y C3. Que el replay, las
  insignias y la curva se vean bien se comprueba a mano con `hoom serve`
  antes del PR.
