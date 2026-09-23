# Spec: items y columna derivada (cabina visual, C1)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start items-y-columna-derivada`.
Origen: `.hoom/intake/rfc-cabina-visual-v2.md` (spec 1 de 4, "C1 — Items y
columna derivada") y el pedido de Henry del 2026-09-22, que fija la tabla de
columnas, el formato del item y absorbe dos ítems del roadmap: "roles que
dejan rastro" y la matriz de enforcement por provider. Sin una línea de
frontend: la columna se puede consultar desde la terminal antes de que exista
el tablero (C2).

## Objetivo

Que una tarjeta de la cabina sea un archivo que escribe una persona y que su
columna sea una función de la evidencia que ya existe en disco y en git. Nadie
escribe columnas. Seis piezas:

1. **Entidad item.** Un archivo por item en `.hoom/items/<slug>.yaml`,
   versionado en git con el mismo patrón que `.hoom/findings/`: dos personas
   que agregan items en dos computadoras nunca escriben el mismo archivo. El
   slug es la única identidad de la tarjeta en toda su vida: es el nombre del
   item, del spec (`.hoom/specs/<slug>.md`) y de la tarea
   (`hoom task start <slug>`).

2. **Verbos `hoom item add | list | show`**, con `--json` y con argumentos
   estrictos (`internal/cliargs`) desde el primer día.

3. **`hoom board [--json]`**: la columna de cada item, calculada por una
   función pura (`boardcmd.Derive`) a partir de la evidencia que junta
   `boardcmd.Gather`. La misma evidencia da la misma columna en cualquier
   computadora, y no se guarda nada.

4. **Subestados ortogonales a la columna**, también derivados: en curso,
   interrumpido, esperando humano, rojo con motivo, sin sincronizar y gasto
   acumulado en USD.

5. **Roles que dejan rastro.** La derivación ES la regla "el rol trabajó si
   existe su artefacto". Para que la regla también se lea en la tabla de
   roles, cada descripción de `internal/agents/targets.go` nombra el artefacto
   que deja. La review limpia, que hoy no deja nada en disco, pasa a dejar un
   registro commiteable (`.hoom/reviews/<id>.json`): sin él, la columna Review
   no tendría salida.

6. **Matriz de enforcement.** `hoom roles [--role r] [--provider p] [--json]`
   imprime qué puede leer, escribir y ejecutar cada rol con cada provider, con
   qué mecanismo y en qué categoría cae: ENFORCED, BEST-EFFORT, POST-VERIFIED
   o UNSUPPORTED. Se deriva de `agents.Roles`, `agentcmd.PolicyFor` y las
   `Capabilities` de cada provider, con las mismas funciones que usa el sobre
   para decidir (`ReadOnlyFor`, `NoExecFor`).

## No-goals

- **Frontend.** Ni la vista del tablero en `hoom serve` (C2) ni endpoints
  nuevos del Studio. C2 lee `hoom board --json`.
- **Acciones desde la tarjeta** (C3): arrastrar, firmar, guardar en git,
  reanudar. En C1 cada columna dice qué comando la mueve, y el comando lo
  corre una persona.
- **Brecha 1 del RFC** (el sobre del test-writer cierra rojo con los tests
  rojos esperados, y el esqueleto de firmas). La columna Test-writer se gana
  con `spec_trace` por token, sin mirar el veredicto, así que la derivación no
  la necesita. El sobre sigue cerrando rojo y la tarjeta lo muestra como
  "rojo con motivo" (ver Riesgos).
- **Brecha 3 del RFC** (registro de aceptación firmado). El pedido la
  reemplaza: la columna Tu aceptación son las condiciones de
  `hoom task done`, y el cierre lo registra `task done` en el item
  (`hecho_en`, `commit_final`).
- **Brecha 4** (PID propio del sobre, C3), **brecha 5** (el analista
  escribiendo items; sigue escribiendo `backlog.md`) y **brecha 6** (worktrees
  de otras herramientas atados a un item por su rama). Un item no declara
  rama.
- **Campos del item que el pedido no nombra**: fuente del intake,
  dependencias entre items, responsable, descarte. Un item con una clave
  desconocida se rechaza (ver Contratos), así que agregarlos después es un
  cambio de esquema consciente.
- **Ejecutar los marcadores `verifica` desde el tablero.** El tablero solo lee. Los
  comandos declarados corren en `hoom verify`.
- **Hacer cumplir el presupuesto.** `presupuesto_usd` se guarda y se muestra
  junto al gasto. Proponerlo como tope de cada arrastre es de C3.
- **Leer la rama `hoom/<slug>` cuando no hay worktree** (lo proponía el RFC).
  El pedido fija la regla: worktree de la tarea si existe, si no el árbol
  actual.
- Cambiar `hoom check`, `hoom status`, `hoom verify` o el Studio, salvo lo
  que se nombra abajo (huella, piso del sobre, registro de review).

## Contratos

### El item: `.hoom/items/<slug>.yaml`

```yaml
titulo: Precios por region
tipo: feature                  # feature|bug|refactor|seguridad|docs|test
prioridad: media               # alta|media|baja
pedido: |                      # texto libre para el arquitecto (opcional)
  Que el catalogo muestre precios por region...
creado_por: Henry Orellana <henry@example.com>
creado_en: 2026-09-22T15:04:05Z
presupuesto_usd: 5             # opcional, > 0
hecho_en: 2026-09-25T10:00:00Z # lo escribe `hoom task done`, nunca un agente
commit_final: 3f2a...          # sha completo de la punta de hoom/<slug> al cerrar
```

- El slug es el nombre del archivo y no es un campo. Tiene la forma que
  acepta `hoom task start`: `^[a-z0-9][a-z0-9-]*$`, hasta 64 caracteres.
- Obligatorios: `titulo` (no vacío), `tipo`, `prioridad`, `creado_por`,
  `creado_en`. Opcionales: `pedido`, `presupuesto_usd`, `hecho_en`,
  `commit_final`.
- El item nunca guarda una columna, un estado, un porcentaje ni un
  responsable. La lectura es estricta: una clave fuera de la lista de arriba
  (por ejemplo `columna:`) hace inválido al item, y el aviso dice que la
  columna sale de la evidencia.
- `creado_por` es la identidad git del proyecto con la misma regla que ya usan
  `hoom spec approve` y `hoom finding add`. Esa regla pasa a vivir en un solo
  lugar: `gitx.Identity(dir string) string` ("Nombre <email>", o lo que haya,
  o `desconocido`). `approval` y `finding` borran su copia y la usan.
- Paquete nuevo `internal/item`: `Item` (con `Slug` fuera del YAML y dentro
  del JSON), `Draft`, `Tipos`, `Prioridades`, `Slugify(titulo) string`,
  `Add(root string, d Draft) (Item, error)`, `Load(root, slug string) (Item,
  error)`, `List(root string) ([]Item, []string, error)` (items válidos +
  avisos) y `MarkDone(root, slug, commit string, at time.Time) (bool, error)`.
  Todas las escrituras son atómicas (`hoomfs.AtomicWrite`).
- `Slugify`: minúsculas, sin acentos (á→a, ñ→n), cada tramo que no sea
  `[a-z0-9]` pasa a un solo `-`, sin `-` al principio ni al final, recortado a
  64 caracteres sin `-` final. "Precios por región (v2)!" →
  `precios-por-region-v2`.
- JSON de un item (lo emiten `item add|list|show --json` y `board --json`):
  las claves del YAML más `slug`. `presupuesto_usd`, `hecho_en` y
  `commit_final` se omiten si no están.
- `.hoom/items/` es evidencia que viaja en git, pero queda FUERA del
  candidato del cambio (`gitx.excludedFromCandidate`), igual que
  `.hoom/findings/`: crear o editar un item nunca cambia la huella ni pone
  rojo `hoom check`, y `hoom task done` puede registrar el cierre sin romper
  el veredicto que lo habilitó.

### Verbos `hoom item`

```
Uso: hoom item add "<titulo>" [--tipo t] [--prioridad p] [--presupuesto-usd x] [--pedido "<texto>"] [--slug s] [--json]
     hoom item list [--json]
     hoom item show <slug> [--json]
```

- Parseo estricto con una función nueva de `cliargs`:
  `cliargs.Operands(fs *flag.FlagSet, args []string, verb, usage string, n
  int) ([]string, error)`. Exige exactamente `n` operandos, acepta flags antes
  o después de ellos, y después de `--` todo es operando (así un título que
  empieza con `-` se escribe `hoom item add -- "-x"`). Devuelve
  `*UsageError` (exit 2) ante: flag desconocido, flag sin valor, flag escrito
  con valor vacío (`--tipo ""`, la regla de typo de verify), un operando de
  más o de menos, o un operando vacío. `-h`/`--help` es `ErrHelp`: el uso va a
  stdout con exit 0. `cliargs.Strict` no cambia.
- `item add`: valida `--tipo` (default `feature`), `--prioridad` (default
  `media`), `--presupuesto-usd` (número > 0) y el slug (el de `--slug` o
  `Slugify(titulo)`). Un valor fuera de su vocabulario, un presupuesto ≤ 0 o
  no numérico, o un título del que no sale ningún slug es `*UsageError`, y no
  se crea nada. Si `.hoom/items/<slug>.yaml` ya existe es un error (exit 1)
  que nombra el archivo y sugiere `--slug`, y el item existente queda byte a
  byte igual. Si existe `.hoom/specs/<slug>.md` no es error: adoptar un spec
  que ya existe es un caso normal.
  Texto: `hoom item: creado .hoom/items/<slug>.yaml`, el título, la columna
  que le toca y la línea `commitealo: viaja en git como los hallazgos`.
  `--json`: el item.
- `item list`: los items válidos del árbol actual, ordenados por `creado_en` y
  después por slug, uno por línea (slug, tipo, prioridad, título). Los
  inválidos se omiten con un aviso por stderr que nombra el archivo y el
  motivo. `--json`: `{"warnings": [...], "items": [...]}`, con `warnings`
  nunca `null`.
- `item show <slug>`: el item y su tarjeta (la misma de `hoom board`). Un slug
  con forma inválida es `*UsageError`. Un slug válido sin archivo es error
  (exit 1). `--json`: `{"item": {...}, "card": {...}}`.
- `hoom item` sin subcomando, o con uno desconocido, es `*UsageError` con el
  bloque de uso de arriba.
- `item list` y `item show` salen con exit 0 aunque haya avisos. Informan y no
  bloquean.

### La evidencia de una tarjeta

`boardcmd.Gather(root, base, blockOn string, it item.Item, now time.Time)
Evidence` junta, para el slug `S`, lo que sigue. `boardcmd.Build(root, base,
blockOn string, now time.Time) (Board, error)` lo hace para todos los items
del árbol actual (es lo que emiten `hoom board` y `hoom item add`), y
`boardcmd.CardFor(root, base, blockOn, slug string, now time.Time) (Card,
error)` para uno (`hoom item show`). Los tres terminan en `Derive`.

- **Árbol de evidencia `E`**: el worktree de la tarea
  (`runcmd.TaskDir(root, S)`) si existe, y si no el árbol actual (`root`).
  `Evidence` dice cuál (`source`: `worktree` | `arbol`) y la ruta.
- **Spec**: `E/.hoom/specs/S.md`. Si existe: `spec.Lint`, y la cobertura por
  token de sus criterios. `spec.Trace` se parte en dos: la búsqueda de tokens
  en los archivos de test (función nueva `spec.Tokens(root string, ids
  []string) (missing []string, scanned int, err error)`, la misma que usa
  `Trace`) y la ejecución de los marcadores `verifica`. El tablero usa solo la
  primera. Un criterio con marcador `verifica` cuenta como cubierto por comando y no
  se ejecuta.
- **Aprobación**: `approval.Status(E, E/.hoom/specs/S.md)`.
- **Veredicto de la tarjeta**: el más nuevo de `verdict.LoadAll(E)` que no es
  parcial y cuyo campo `spec`, limpio y relativo a `E`, es
  `.hoom/specs/S.md`. También el último veredicto completo de `E`
  (`verdict.LatestComplete`) y la huella actual
  (`gitx.Snapshot(E, base).ChangeFingerprint`).
- **Registros de review** de `E/.hoom/reviews/` con `task` = `S`.
- **Hallazgos abiertos** de `finding.List(E, base, true)` con `task` = `S`. El
  umbral es `blockOn` del `hoom.yaml` del proyecto, o `high` si no hay.
- **Condiciones de cierre**: `taskcmd.Ready(root, S, base) error`, función
  nueva que es exactamente el chequeo que hoy hace `taskcmd.Done` sin
  `--force` (la tarea existe, árbol limpio, hay veredicto completo, el último
  completo es verde y la huella coincide), con los mismos mensajes. `Done` la
  llama, así que las dos no pueden divergir.
- **Sin commitear**: con `E` = worktree, todo lo que lista
  `git status --porcelain --untracked-files=all` en `E` (la misma condición de
  `task done`). Con `E` = árbol actual, solo las rutas de la tarjeta:
  `.hoom/items/S.yaml`, `.hoom/specs/S.md`, `.hoom/approvals/S_*.json`, los
  veredictos de la tarjeta, los hallazgos y resoluciones con `task` = `S` y
  sus registros de review. Además, siempre, `root/.hoom/items/S.yaml`.
- **Telemetría** (local, no viaja): `envelope.List(root)` y
  `runcmd.Metas(root)`, más los de `E` si es otro directorio, sin repetir ids,
  filtrados por `task` = `S`. La vida del dueño se resuelve acá y no en
  `Derive`, que es pura: `runcmd.Alive(pid int) bool` (el `alive` de hoy,
  exportado) y el umbral `live.OrphanAfter`.

`Gather`, `Build` y `CardFor` son de solo lectura: no crean ni modifican nada
bajo `.hoom/` ni en el árbol, y no ejecutan ningún comando que no sea `git` de
lectura. `Build` hace una sola lectura por directorio de lo que comparten
varias tarjetas del mismo árbol (veredictos, hallazgos, status, huella).

### La columna: `boardcmd.Derive(ev Evidence) Card`

Función pura: sin disco, sin reloj propio (`ev.Now`), sin git. La tarjeta
queda en la columna del PRIMER requisito que no se cumple. Cada columna es la
estación donde está el trabajo pendiente:

| # | id | Nombre | La tarjeta está acá cuando |
|---|---|---|---|
| 8 | `hecho` | Hecho | el item tiene `hecho_en` (se mira primero; es terminal) |
| 1 | `backlog` | Backlog | no existe `E/.hoom/specs/S.md` |
| 2 | `arquitecto` | Arquitecto | el spec existe y `spec.Lint` da issues o no se puede leer |
| 3 | `tu-aprobacion` | Tu aprobacion | lint pasa y no hay aprobación vigente para el hash actual |
| 4 | `test-writer` | Test-writer | aprobado y algún CA sin test ni marcador `verifica` |
| 5 | `writer` | Writer | trazado y no hay veredicto de la tarjeta verde con la huella actual |
| 6 | `review` | Review | hay veredicto verde con la huella actual y falla alguna condición de aceptación |
| 7 | `tu-aceptacion` | Tu aceptacion | se cumplen todas las condiciones de aceptación y falta `hecho_en` |

**Condiciones de aceptación** (todas, sobre el veredicto verde de la
tarjeta):

1. **Review no pendiente.** La review se exige cuando el veredicto de la
   tarjeta tiene `insertions + deletions` > `reviewcmd.UmbralLineas` (400), que
   es cuando el veredicto ya dice "la review exige las 4 lentes" (contrato
   06). Se cumple con un registro de review de la tarjeta cuyas `lenses`
   incluyen las 4 de `reviewcmd.Lentes` y cuyo `verdict_id` es un veredicto
   completo, verde y de la tarjeta. Un cambio de 400 líneas o menos no exige
   review.
2. **Sin hallazgos que bloqueen**: ninguno abierto con `task` = `S` y
   severidad que bloquea según el umbral (`finding.Blocks`).
3. **`spec_approved` = pass** en el veredicto de la tarjeta.
4. **`findings_open` = pass** en el veredicto de la tarjeta. Si el gate no
   está (el proyecto no declara `findings.block_on`), la tarjeta no sale de
   Review y la falta dice la línea exacta: `findings: { block_on: high }` en
   `hoom.yaml`.
5. **`taskcmd.Ready(root, S, base)` sin error.** Sin worktree de la tarea la
   falta es "cerrar exige la tarea: hoom task start S" (el cierre lo registra
   `hoom task done`, que necesita la tarea).

**Motivos y siguiente paso.** `Card.Missing` lista, en orden, lo que falta
para salir de la columna. La primera entrada es el motivo principal. En la
columna 6 van todas las condiciones que fallan, en el orden de arriba.
`Card.Next` es el comando que mueve la tarjeta:

- backlog: `hoom task start S` si no hay worktree, si no
  `hoom agent --role arquitecto --task S "<pedido>"`.
- arquitecto: el mismo `hoom agent --role arquitecto ...`, y los issues de
  lint en `Missing`.
- tu-aprobacion: `hoom spec approve .hoom/specs/S.md`. Si la aprobación quedó
  invalidada, el motivo es "el spec cambio despues de tu aprobacion".
- test-writer: `hoom agent --role test-writer [--task S] --spec
  .hoom/specs/S.md "<pedido>"`, y los CA sin test en `Missing`.
- writer: `hoom agent --role writer [--task S] --spec .hoom/specs/S.md
  "<pedido>"`. Con un verde de otra huella el motivo es "el codigo cambio
  despues del ultimo verde". Con el veredicto de la tarjeta rojo, los gates
  que fallaron.
- review: el comando de la primera condición que falla
  (`hoom review [--task S] --spec .hoom/specs/S.md`,
  `hoom finding resolve <id> ...`, la línea del `hoom.yaml`, o la acción que
  trae el error de `taskcmd.Ready`).
- tu-aceptacion: `hoom task done S`.
- hecho: vacío.

`--task S` aparece en los comandos solo cuando `E` es el worktree.

### Subestados (en la misma `Card`, todos derivados)

- **`running`** (en curso): un sobre de la tarjeta sin cerrar cuyo dueño vive.
  El dueño vive si el sidecar de su `run_id` está `running` con `PID` vivo. Si
  el sobre no tiene run, o su run ya cerró (pasos scope, verify, check), vive
  si su `updated_at` tiene menos de `live.OrphanAfter`. Un run sin sobre
  (`hoom run --task`, las pasadas de `hoom review`) con sidecar `running` y
  `PID` vivo también cuenta, con stage `run`. Trae `envelope_id`, `run_id`,
  `role`, `provider`, `stage`, `step`, `steps`, `started_at`. Si hay varios,
  va el más nuevo.
- **`interrupted`**: un sobre de la tarjeta sin cerrar cuyo dueño no vive (run
  `running` con `PID` muerto, o latido más viejo que `live.OrphanAfter`). Trae
  el paso donde quedó (`stage`, `step`, `steps`) y `updated_at`. hoom no lo
  cierra: lo rotula.
- **`waiting_human`**: `true` exactamente en `tu-aprobacion` y
  `tu-aceptacion`.
- **`red`**: se compara el veredicto de la tarjeta (`created_at`) con su
  último sobre cerrado (`ended_at`) y gana el más nuevo. Si es un veredicto
  rojo: `{"source": "veredicto", "id", "reason": "veredicto rojo: fallo el
  gate test"}` (fallaron los gates requeridos, por nombre). Si es un sobre
  `no-entregable` o `sin-entrega`: `{"source": "sobre", "id", "reason": "el
  sobre de <rol> corto en <stage>: <note>"}`. Si el más nuevo es verde o
  entregable, no hay rojo.
- **`unsynced`**: las rutas sin commitear de la evidencia (ver arriba),
  ordenadas. Vacío = sincronizada.
- **`spend`**: la suma de los runs de la tarjeta (sidecars con `task` = `S`),
  más el `usage` de cada sobre de la tarjeta cuyo run no tiene sidecar (así
  nada se cuenta dos veces). `cost_usd` suma los costos reportados y es
  `null` si ningún run reportó costo, nunca 0 por falta de dato. También
  `runs`, `runs_without_cost`, `input_tokens`, `output_tokens` y
  `budget_usd` (el `presupuesto_usd` del item). Es telemetría local y la
  tarjeta lo dice.

### `Card` (JSON)

```json
{
  "slug": "precios-por-region",
  "item": { "...": "las claves del item" },
  "column": "review",
  "column_name": "Review",
  "missing": ["la review exige las 4 lentes (612 lineas > 400) y no hay registro de review"],
  "next": "hoom review --task precios-por-region --spec .hoom/specs/precios-por-region.md",
  "evidence": {
    "source": "worktree", "dir": ".hoom/worktrees/precios-por-region",
    "spec": ".hoom/specs/precios-por-region.md", "spec_exists": true,
    "lint_issues": [], "criteria": 12, "traced": 12, "untraced": [],
    "approval": "aprobado",
    "verdict_id": "2026-...", "verdict": "green", "fingerprint_match": true,
    "review_required": true, "review_id": "",
    "blocking_findings": []
  },
  "running": null, "interrupted": null, "waiting_human": false,
  "red": null, "unsynced": [],
  "spend": { "cost_usd": 1.2, "runs": 3, "runs_without_cost": 1,
             "input_tokens": 51000, "output_tokens": 3000, "budget_usd": 5 }
}
```

Las listas nunca son `null`. Las rutas son relativas a `root`.

### `hoom board [--json]`

```
Uso: hoom board [--json]
```

- Parseo con `cliargs.Strict` + `cliargs.FirstEmptyValue`: sin operandos.
- Texto: `hoom board: N tarjetas (columna derivada de la evidencia; nada
  guardado)`, y después las 8 columnas en el orden 1..8, cada una con su
  nombre y cuántas tarjetas tiene, también con 0. Las dos humanas llevan
  `esperando humano`. Por tarjeta: slug, título, la primera línea de
  `missing`, `siguiente: <next>` y una línea por subestado presente (`en
  curso`, `interrumpido en el paso X de N`, `rojo: <reason>`, `sin
  sincronizar: N archivos`, `gasto: ...`). Sin items: `hoom board: sin items
  (crea uno con 'hoom item add "<titulo>"')`.
- `--json`: `{"columns": [{"id", "name", "human", "cards": [Card...]}],
  "warnings": [...]}` con las 8 columnas siempre, en orden, y los avisos de
  items inválidos.
- Exit 0 siempre que el pedido se entendió: el tablero informa, no bloquea.

### `hoom task done`: registrar el cierre

- Después de `taskcmd.Ready` y antes de `git worktree remove`: si existe
  `root/.hoom/items/S.yaml` (el árbol donde corre `task done`), `item.MarkDone`
  escribe `hecho_en` (UTC, segundos) y `commit_final` (`git rev-parse
  hoom/S`, sha completo) y conserva los demás campos. Si el remove falla, el
  item vuelve a su contenido original.
- Imprime `hoom: item S hecho (commit_final <sha12>) - commitea
  .hoom/items/S.yaml`.
- Sin item: cierra igual que hoy y dice `hoom: sin item .hoom/items/S.yaml en
  este arbol: no hay tarjeta que cerrar`.
- Un item ilegible o inválido: `task done` falla antes de tocar el worktree,
  con el motivo.
- Un item que ya tiene `hecho_en`: no se reescribe, y la salida lo dice.
- `--force` no escribe `hecho_en`: sin las condiciones, la tarjeta no se gana
  Hecho. La salida lo dice.

### Registro de review: `.hoom/reviews/<id>.json`

- `hoom review` que termina `revisado` (todas sus lentes corrieron y el scope
  dio OK) escribe, en el árbol revisado (`dir`), un registro append-only:

  ```json
  {"id": "20260922T150405_ab12cd", "created_at": "...",
   "task": "S", "spec": ".hoom/specs/S.md",
   "fingerprint": "<huella del arbol revisado>",
   "verdict_id": "<ultimo veredicto completo al empezar>", "verdict": "green",
   "lenses": ["readability", "reliability", "resilience", "risk"],
   "provider": "codex", "writer": "claude", "cross": "cruzada",
   "findings": ["<ids que hoom vio aparecer>"]}
  ```

- `task` es `--task`, y si no hay, la tarea del `--spec`
  (`finding.TaskOfSpec`). Sin ninguno de los dos queda vacío.
- `sin-revisar` y `no-entregable` no escriben registro. `reviewcmd.Result`
  gana `record_id` (se omite si no hay registro).
- Es un artefacto de rol, no un veredicto: dice "revisé esto", no "esto está
  bien". Viaja en git y queda fuera del candidato
  (`gitx.excludedFromCandidate`), igual que los hallazgos.
- Tipo y funciones en `reviewcmd`: `Record`, `WriteRecord(dir string, r
  Record) (Record, error)`, `Records(dir string) ([]Record, []string)`.

### El piso del sobre: items y reviews

- `agentcmd.universal` suma dos reglas, las dos `manipulacion` (cortan antes
  de verify) y no aflojables desde `hoom.yaml`:
  - cualquier cambio bajo `.hoom/items/`: "el item lo escribe una persona
    (hoom item add) y su cierre hoom task done; un rol no mueve su tarjeta ni
    afloja su presupuesto";
  - cualquier cambio bajo `.hoom/reviews/`: "el registro de review lo escribe
    hoom review, no un rol".

### Roles que dejan rastro: descripciones

Cada `Role.Desc` de `internal/agents/targets.go` termina con una oración
`Deja: <artefacto>.`, y las que mueven una columna nombran exactamente lo que
lee la derivación:

| rol | Deja |
|---|---|
| orquestador | nada propio: lo que cuenta lo dejan los roles que delega |
| arquitecto | `.hoom/specs/<slug>.md` que pasa spec_lint |
| designer | el UI-spec en `.hoom/specs/` |
| scout | nada en disco: su entrega es el resumen que devuelve |
| writer | el codigo que pone verde `hoom verify --spec` (el veredicto en `.hoom/verdicts/`) |
| test-writer | tests que citan cada CA-n del spec (spec_trace) |
| reviewer | hallazgos en `.hoom/findings/` y el registro de review en `.hoom/reviews/` |
| characterizer | characterization tests que fijan el comportamiento actual |
| analista | `.hoom/specs/00-vision.md` y `.hoom/specs/backlog.md` |
| refutador | resoluciones con evidencia (`.hoom/findings/<id>.res.json`) |

Lo que ya fijó CA-229 se mantiene: ninguna Desc dice "Solo lectura" en los
autores de specs, y ninguna queda vacía.

### Matriz de enforcement: `hoom roles`

```
Uso: hoom roles [--role r] [--provider p] [--json]
```

- Paquete nuevo `internal/rolescmd`: `Matrix(m *manifest.Manifest, roles
  []agents.Role, provs []providers.Provider) []Row`.
- `Row`: `role`, `provider` y tres celdas, `read`, `write` y `exec`. Cada
  celda tiene `allows` (qué puede hacer el rol en esa dimensión), `mechanisms`
  (lista, nunca vacía), `category` y `gap` (qué queda sin cubrir: vacío en
  ENFORCED y obligatorio en las otras tres).
- Categorías:
  - `ENFORCED`: el provider o el sistema de archivos impide cualquier cosa
    fuera de lo que dice `allows`. Una dimensión donde el rol no tiene límite
    (`todo el arbol`, `comandos`) es ENFORCED con el mecanismo `sin
    restriccion`: lo que dice la tabla es exactamente lo que tiene.
  - `POST-VERIFIED`: nada lo impide durante el run, y el gate de scope lo ve
    después y lo convierte en hallazgo high (fuera de scope) o en corte
    (manipulación, aislamiento).
  - `BEST-EFFORT`: lo pide el contrato. Nada lo impide y ningún gate lo puede
    ver.
  - `UNSUPPORTED`: el provider no puede llevar el rol, porque `hoom agent` y
    `hoom review` exigen `system_prompt` para darle el contrato. Las tres
    celdas.
- Reglas (P = provider con `system_prompt`, r = rol). El mecanismo de un
  provider que impone `ReadOnly` es `deny de tools` si declara `Tools` (niega
  herramientas por nombre, Bash incluido cuando el rol no ejecuta) y
  `sandbox` si no (el sandbox read-only no le quita el shell al rol, y con
  `Exec` pasa a workspace-write):
  - **read**: `r.Isolated` → `allows` "el spec y los tests, sin la
    implementacion en disco", mecanismos `arbol ciego` + `gate post-run
    (aislamiento)`, BEST-EFFORT, `gap` "el object DB de git es compartido:
    'git show HEAD:<ruta>' lee la implementacion" cuando el rol ejecuta
    comandos (ENFORCED si no ejecuta y P le quita el shell). Cualquier otro
    rol: `todo el arbol`, ENFORCED, `sin restriccion`.
  - **write**: `allows` son los globs de `PolicyFor(m, r)` (allow, y "menos"
    los deny), más "nunca el piso: aprobaciones, hoom.yaml, evidencia
    existente, items, reviews".
    - r ReadOnly sin Exec, P impone ReadOnly (`ReadOnlyFor`): `allows`
      "nada", mecanismos [el de P, `gate post-run`], ENFORCED.
    - r ReadOnly con Exec, P impone ReadOnly: con `deny de tools`, mecanismos
      [`deny de tools (sin Edit/Write; el shell puede escribir)`, `gate
      post-run`]; con `sandbox`, [`gate post-run`]. POST-VERIFIED.
    - P no impone ReadOnly, o r escribe: [`gate post-run`], y `arbol ciego
      (cuarentena)` primero si `r.Isolated`. POST-VERIFIED.
  - **exec**: r con Exec → `comandos`, ENFORCED, `sin restriccion`.
    r sin Exec: si es ReadOnly y P lo impone con `deny de tools`, o si
    escribe y `NoExecFor` da `noExec`, `allows` "ningun comando" con
    `deny de tools`, ENFORCED. Si no (sandbox read-only, o `NoExecFor` avisa),
    `allows` "ningun comando (lo pide el contrato)" con `contrato`,
    BEST-EFFORT.
- Tabla que sale hoy (los 10 roles × claude, codex; opencode y gemini son
  UNSUPPORTED en todo):

  | rol | read | write claude | write codex | exec claude | exec codex |
  |---|---|---|---|---|---|
  | orquestador, reviewer, refutador | ENFORCED | POST-VERIFIED | POST-VERIFIED | ENFORCED | ENFORCED |
  | scout | ENFORCED | ENFORCED | ENFORCED | ENFORCED | BEST-EFFORT |
  | arquitecto, designer, analista | ENFORCED | POST-VERIFIED | POST-VERIFIED | ENFORCED | BEST-EFFORT |
  | writer, characterizer | ENFORCED | POST-VERIFIED | POST-VERIFIED | ENFORCED | ENFORCED |
  | test-writer | BEST-EFFORT | POST-VERIFIED | POST-VERIFIED | ENFORCED | ENFORCED |

- `--role` y `--provider` filtran. Un valor que no existe es `*UsageError`
  (exit 2) con la lista de válidos. Se parsea con `cliargs.Strict` +
  `FirstEmptyValue`.
- Texto: un bloque por rol · provider con tres líneas (`lee`, `escribe`,
  `ejecuta`: allows, categoría, mecanismos), y al final una leyenda de una
  línea por categoría. `--json`: el arreglo de `Row`, en el orden de
  `agents.Roles()` y después `providers.All()`.
- La política sale del `hoom.yaml` del árbol actual (`manifest.Load`), así
  que un `agents.<rol>.write` del proyecto se ve en la matriz.

### Documentación

- `hoom help` lista `item`, `board` y `roles` en Comandos.
- README: los tres verbos en la tabla de comandos, el formato del item, la
  tabla de columnas y el piso nuevo. El roadmap pierde "Items y columna
  derivada".

## Casos límite y errores esperados

- Un item hecho sin spec en el árbol (se cerró la tarea y el spec vive en la
  rama sin integrar): Hecho. `hecho_en` gana sobre cualquier evidencia.
- Existe `.hoom/worktrees/S` pero el spec está solo en el árbol actual (la
  rama se creó desde `main` y el spec no se integró): Backlog, y
  `evidence.dir` muestra el worktree para que se vea dónde miró.
- El worktree tiene una copia distinta del item: manda la del árbol actual.
  Items creados dentro de un worktree aparecen en el tablero de ese worktree,
  no en el del árbol principal.
- Spec con un marcador `verifica` cuyo comando falla: el tablero lo cuenta como
  cubierto (no lo ejecuta). El rojo aparece en el próximo `verify --spec`, y
  entonces la tarjeta queda en Writer con el rojo.
- Un veredicto con `spec: ./.hoom/specs/S.md` o con la ruta absoluta:
  cuenta, porque se compara limpio y relativo a `E`. Un veredicto parcial
  (`--gate`) nunca cuenta, en ninguna dirección.
- El último veredicto completo del worktree no es de la tarjeta (alguien
  corrió `hoom verify` sin `--spec`): si es verde con la misma huella,
  `taskcmd.Ready` pasa. Si es rojo, la tarjeta queda en Review con el mensaje
  de `task done`.
- Un veredicto verde de la tarjeta de antes de que existiera `spec_approved`:
  no gana Tu aceptación (la condición 3 pide el gate presente y en pass).
- Un hallazgo high abierto de otra tarea, o sin tarea: no mantiene la tarjeta
  en Review. El `findings_open` del próximo verify sí lo cuenta (B2).
- Un hallazgo que bloquea registrado después del verde: la tarjeta queda en
  Review por la condición 2. No retrocede a Writer.
- La review se hizo sobre un veredicto rojo o de otro spec: no cuenta. Una
  review con `--lens` de una sola lente: no cuenta para un cambio > 400
  líneas.
- El código cambia después de la review y vuelve a verde: el registro sigue
  valiendo (las correcciones de la review son lo que la review pidió), y
  `evidence` lo muestra con `review_id`.
- Un sobre abierto con `run_id` cuyo sidecar desapareció: se juzga por el
  latido.
- Un sidecar con `PID: 0` (versiones viejas): se juzga por el latido del sobre
  si hay sobre. Un run suelto sin PID no cuenta como en curso.
- Un run de Codex (sin costo) y uno de Claude (0.8 USD) en la misma tarjeta:
  `cost_usd` 0.8, `runs` 2, `runs_without_cost` 1, con los tokens de los dos.
- Un item con `presupuesto_usd: 0`, `-1` o `"cinco"`: inválido (aviso), igual
  que `tipo: epica` o `prioridad: urgente`.
- Un archivo `.hoom/items/Foo Bar.yaml` o `.hoom/items/x.yml`: se ignora con
  un aviso (el nombre no es un slug o la extensión no es `.yaml`).
- `hoom item add "¡¡¡"`: `*UsageError`, "del titulo no sale ningun slug; usa
  --slug".
- `hoom item add "A" "B"`, `hoom item show`, `hoom board x`,
  `hoom roles --role`: `*UsageError`, exit 2, sin efectos.
- `hoom task done S --force` con item: el worktree se va y el item queda
  igual.
- Un rol bajo el sobre que corre `hoom item add` o edita el `pedido` de su
  item: manipulación, el sobre corta antes de verify. Lo mismo si corre
  `hoom review` desde adentro de su sobre.
- Proyecto sin `findings.block_on`: ninguna tarjeta llega a Tu aceptación, y
  todas las que están en Review lo dicen. hoomai ya lo declara (B2).

## Criterios de aceptación

- CA-263: `hoom item add "Precios por región (v2)!"` crea
  `.hoom/items/precios-por-region-v2.yaml` con `titulo`, `tipo: feature`,
  `prioridad: media`, `creado_por` igual a la identidad que registra
  `hoom spec approve` en el mismo repo y `creado_en` en UTC, sin
  `presupuesto_usd`, `hecho_en` ni `commit_final`. `--slug otro` fija el slug.
  `Slugify` cumple los ejemplos del contrato (acentos, tramos, 64
  caracteres).
- CA-264: `--tipo`, `--prioridad`, `--presupuesto-usd` y `--pedido` quedan en
  el archivo. Un tipo o prioridad fuera del vocabulario, un presupuesto ≤ 0 o
  no numérico, un título vacío o sin slug, un flag desconocido, un flag vacío
  (`--tipo ""`) o un operando de más son `*cliargs.UsageError` (exit 2 con el
  binario) y `.hoom/items/` queda igual.
- CA-265: `item add` con un slug que ya existe sale con exit 1, nombra el
  archivo, y el item existente queda byte a byte igual. Con
  `.hoom/specs/<slug>.md` ya existente el item se crea.
- CA-266: `item list` ordena por `creado_en` y slug. `--json` emite
  `{"warnings": [...], "items": [...]}` (nunca `null`) con las claves del
  YAML más `slug`. Un YAML roto, una clave desconocida (`columna:`), un tipo
  inválido o un nombre de archivo que no es slug se omiten con un aviso que
  nombra el archivo, y el comando sale 0.
- CA-267: `item show <slug> --json` emite `{"item", "card"}` con la misma
  tarjeta que `hoom board --json` para ese slug. Slug inexistente: exit 1.
  `hoom item` sin subcomando o con uno desconocido: `*UsageError`.
- CA-268: `cliargs.Operands` acepta flags antes y después del operando,
  operandos después de `--` aunque empiecen con `-`, rechaza con
  `*UsageError` los operandos de más o de menos, el operando vacío, el flag
  desconocido, el flag sin valor y el flag con valor vacío, y devuelve
  `ErrHelp` con `-h`. Los tests de `cliargs.Strict` (CA-219..CA-221) siguen
  pasando sin cambios.
- CA-269: Backlog: sin `.hoom/specs/S.md` en `E`, la columna es `backlog`,
  `missing` nombra la ruta que falta y `next` es `hoom task start S` sin
  worktree, o el `hoom agent --role arquitecto --task S` con worktree.
- CA-270: Arquitecto: un spec sin la sección "riesgos" da `arquitecto` con el
  issue de lint en `missing`. Un spec vacío también.
- CA-271: Tu aprobación: lint OK sin aprobación da `tu-aprobacion`,
  `waiting_human: true` y `next` `hoom spec approve .hoom/specs/S.md`. Con una
  aprobación de un contenido anterior, la columna es la misma y `missing`
  dice "el spec cambio despues de tu aprobacion".
- CA-272: Test-writer: aprobado con un CA sin test da `test-writer` y
  `missing` nombra ese CA. `evidence` trae `criteria`, `traced` y `untraced`.
  Un CA con marcador `verifica` cuenta como cubierto y el tablero no ejecuta
  el comando (un marcador `verifica` con el comando `touch marca` no crea `marca`).
- CA-273: Writer: todos los CA trazados y ningún veredicto de la tarjeta da
  `writer`. También: un veredicto de la tarjeta rojo (con los gates que
  fallaron en `missing`), uno verde con otra huella ("el codigo cambio
  despues del ultimo verde"), uno verde de OTRO spec y uno verde parcial.
- CA-274: Review por tamaño: veredicto verde de la tarjeta con más de 400
  líneas y sin registro da `review` con "la review exige las 4 lentes". Un
  registro de la tarjeta con las 4 lentes y `verdict_id` verde de la tarjeta
  cumple la condición. No la cumplen un registro de una sola lente, uno cuyo
  `verdict_id` es rojo o de otro spec, ni uno de otra tarea. Con 400 líneas o
  menos no se exige review.
- CA-275: Review por hallazgos: un hallazgo abierto con `task` = S y
  severidad que bloquea deja la tarjeta en `review` aunque el cambio sea
  chico. No la dejan uno resuelto, uno de otra tarea, uno sin tarea ni uno
  por debajo del umbral. Con `block_on: medium` un medium bloquea, y sin
  `block_on` el umbral es `high`.
- CA-276: Review por gates: un veredicto verde de la tarjeta sin
  `findings_open` deja la tarjeta en `review` y `missing` contiene
  `findings: { block_on: high }`. Uno sin `spec_approved` en pass tampoco
  gana Tu aceptación.
- CA-277: Review por cierre: sin worktree de la tarea, `missing` contiene
  "cerrar exige la tarea". Con el worktree sucio, contiene el mensaje de
  `taskcmd.Ready` sobre cambios sin commitear. Con el último veredicto
  completo del worktree rojo, el de veredicto rojo. `taskcmd.Done` usa
  `taskcmd.Ready` y sus mensajes no cambian.
- CA-278: Tu aceptación: con todas las condiciones cumplidas la columna es
  `tu-aceptacion`, `waiting_human: true` y `next` es `hoom task done S`.
- CA-279: Hecho: un item con `hecho_en` da `hecho` aunque no haya spec, ni
  veredicto, ni worktree, y `next` va vacío.
- CA-280: la evidencia se lee del worktree `.hoom/worktrees/S` si existe (un
  spec aprobado solo en el árbol actual con un worktree sin spec da
  `backlog`, con `evidence.source: "worktree"`), y del árbol actual si no
  (`evidence.source: "arbol"`).
- CA-281: `Derive` es pura: la misma `Evidence` da la misma `Card` (igualdad
  profunda, llamada dos veces). `hoom board` y `hoom item show` no crean ni
  modifican ningún archivo del repo ni de `.hoom/` (foto del árbol antes y
  después, ignorados incluidos).
- CA-282: En curso: un sobre sin cerrar de la tarjeta cuyo run tiene sidecar
  `running` con el PID de un proceso vivo da `running` con `envelope_id`,
  `role`, `provider`, `stage`, `step` y `steps`. Un run suelto con `task` = S,
  `running` y PID vivo da `running` con stage `run`. Un sobre en el paso
  verify (run cerrado) con latido fresco también está en curso.
- CA-283: Interrumpido: un sobre sin cerrar cuyo run está `running` con un PID
  muerto, o sin run y con `updated_at` más viejo que `live.OrphanAfter`, da
  `interrupted` con su `stage`/`step`/`steps` y `running: null`. El registro
  del sobre queda byte a byte igual.
- CA-284: `waiting_human` es `true` en `tu-aprobacion` y `tu-aceptacion`, y
  `false` en las otras seis columnas.
- CA-285: Rojo: si lo más nuevo de la tarjeta es un veredicto rojo, `red` es
  `{"source": "veredicto"}` con el id y los gates que fallaron. Si es un sobre
  `no-entregable` o `sin-entrega`, es `{"source": "sobre"}` con "el sobre de
  <rol> corto en <stage>" y la nota. Un veredicto verde posterior al sobre
  fallido deja `red: null`.
- CA-286: Sin sincronizar: con `E` = worktree, un archivo sin commitear del
  worktree aparece en `unsynced`. Con `E` = árbol actual, aparecen el item,
  el spec, una aprobación, un veredicto, un hallazgo y un registro de review
  de la tarjeta sin commitear, y un archivo ajeno sucio no. Un item sin
  commitear en el árbol actual aparece aunque `E` sea el worktree.
- CA-287: Gasto: dos runs de la tarjeta, uno con `cost_usd` 0.8 y otro sin
  costo, dan `cost_usd` 0.8, `runs` 2, `runs_without_cost` 1 y la suma de sus
  tokens. Solo runs sin costo dan `cost_usd: null`. Un sobre cuyo run no tiene
  sidecar suma su propio `usage`, y uno cuyo run tiene sidecar no se cuenta
  dos veces. Un run de otra tarea no suma. `budget_usd` es el
  `presupuesto_usd` del item.
- CA-288: `hoom board --json` emite las 8 columnas siempre, en el orden
  `backlog, arquitecto, tu-aprobacion, test-writer, writer, review,
  tu-aceptacion, hecho`, con `human` solo en las dos humanas, y los avisos de
  items inválidos en `warnings`. El texto muestra las 8 con su conteo, las
  humanas con `esperando humano`, y por tarjeta el motivo y `siguiente:`.
  Exit 0. `hoom board x` y `hoom board --bogus` son `*UsageError`.
- CA-289: `hoom task done S` con las condiciones cumplidas escribe en
  `.hoom/items/S.yaml` del árbol donde corre `hecho_en` (UTC) y
  `commit_final` (el sha completo de `hoom/S`), conserva los demás campos,
  imprime que hay que commitearlo, y después `hoom board` da `hecho`.
- CA-290: `task done` sin item cierra igual y lo dice. Con `--force` el item
  queda byte a byte igual. Con un item inválido falla antes de quitar el
  worktree (el worktree sigue ahí). Un item que ya tiene `hecho_en` no se
  reescribe.
- CA-291: `.hoom/items/` y `.hoom/reviews/` quedan fuera de la huella: crear
  un item o un registro de review (sin commitear o commiteado) no cambia
  `ChangeFingerprint`, y `hoom check` sigue verde.
- CA-292: Bajo el sobre, crear, editar o borrar un archivo de
  `.hoom/items/` o de `.hoom/reviews/` es violación `manipulacion` para
  cualquier rol, aunque un `agents.<rol>.write.allow` de `hoom.yaml` cubra la
  ruta, y el sobre corta antes de verify.
- CA-293: `hoom review` que termina `revisado` escribe
  `.hoom/reviews/<id>.json` en el árbol revisado con los campos del contrato,
  `task` sale de `--task` o del `--spec`, y el `Result` trae `record_id`.
  `sin-revisar` y `no-entregable` no escriben nada en `.hoom/reviews/`.
- CA-294: cada `Desc` de la tabla de roles contiene `Deja:`. La de arquitecto
  nombra `.hoom/specs/<slug>.md`, la de test-writer `CA-n`, la de writer
  `hoom verify --spec`, la de reviewer `.hoom/reviews/` y la de refutador
  `.res.json`. CA-229 sigue pasando.
- CA-295: `rolescmd.Matrix` sobre todos los roles de `agents.Roles()` y todos
  los providers de `providers.All()` da una fila por combinación (10 × 4), y
  cada celda tiene una de las cuatro categorías, al menos un mecanismo y un
  `gap` no vacío salvo en ENFORCED. El test falla si una combinación queda sin
  clasificar.
- CA-296: la matriz da la tabla del contrato para claude y codex, con
  opencode y gemini UNSUPPORTED en las tres celdas. La lectura del
  test-writer es BEST-EFFORT con `git show` en `gap`.
- CA-297: el mecanismo que declara la matriz coincide con el argv real del
  adapter: donde dice `deny de tools`, `Command` con `ReadOnly` trae
  `--disallowedTools` (y `Bash` en la lista si el rol no ejecuta). Donde dice
  `sandbox`, trae `sandbox_mode="read-only"`, o `"workspace-write"` si el rol
  ejecuta.
- CA-298: un `agents.writer.write.allow` en `hoom.yaml` aparece en `allows`
  de la celda write del writer. `--role`/`--provider` filtran las filas, y un
  valor desconocido es `*UsageError` con los válidos. El texto termina con la
  leyenda de las cuatro categorías.
- CA-299: E2E con el binario real en un repo temporal (gates del proyecto
  reemplazados por `true`, `findings.block_on: high`): `item add` → `backlog`,
  `task start` → spec sin sección → `arquitecto`, spec completo →
  `tu-aprobacion`, `spec approve` → `test-writer`, test que cita los CA →
  `writer`, `verify --spec` verde y commit → `tu-aceptacion`, `task done` →
  `hecho` con `hecho_en` y `commit_final` en el item. Cada paso se lee de
  `hoom board --json`.
- CA-300: `hoom help` lista `item`, `board` y `roles`. [verifica: go run ./cmd/hoom help | grep -q "board"]
- CA-301: dogfood: hoomai tiene el item de esta spec. [verifica: go run ./cmd/hoom item show items-y-columna-derivada --json | grep -q '"slug": "items-y-columna-derivada"']

## Decisiones

- **La columna es la estación del trabajo pendiente**, como la fija la tabla
  del pedido, y no "la última columna ganada" del RFC. Las dos lecturas
  coinciden en el orden. Cambia qué se muestra: una tarjeta en Tu aprobación
  está esperando que la firmes, no la firmaste. Por eso `waiting_human` es
  exactamente las dos columnas moradas.
- **Hecho se mira primero.** `hecho_en` es terminal. Una tarjeta integrada no
  se reevalúa contra lo que pase después en `main`: eso es de otras tarjetas.
- **Tu aceptación = las condiciones de `hoom task done`, más las dos del
  pedido** (`spec_approved` y `findings_open` en pass). El chequeo de
  `task done` se extrae a `taskcmd.Ready` en vez de copiarse: el tablero no
  puede decir "listo para cerrar" de algo que `task done` rechaza. La
  consecuencia es que sin `hoom task` la tarjeta no pasa de Review. Es
  honesto: `hecho_en` solo lo escribe `task done`, y `task done` necesita la
  tarea. Atar worktrees de otras herramientas es la brecha 6.
- **Review pide un registro nuevo** (`.hoom/reviews/`). El pedido define
  Review como "review pendiente", y sin un rastro de la review limpia la
  columna no tendría salida: hoy una review sin hallazgos no deja nada en
  disco (brecha 2 del RFC). Los runs de `hoom review` son telemetría local y
  no pueden mover una tarjeta: en otra computadora retrocedería.
- **La review se exige por tamaño**, como dice el pedido: cuando el veredicto
  ya imprime "la review exige las 4 lentes" (> 400 líneas). Las rutas de
  riesgo y la lente única del cambio estándar siguen siendo del contrato 06
  y de `hoom review`, pero el tablero no las exige en C1. Queda anotado en
  Riesgos.
- **La review vale si revisó un verde de la tarjeta**, no si revisó la huella
  exacta de hoy. Pedir la huella exacta obligaría a otra pasada de 4 lentes
  después de cada corrección. Las correcciones las cubren los hallazgos
  (condición 2) y el veredicto nuevo (Writer). Pedir un verde de la tarjeta
  evita que cuente una review hecha antes de que existiera el código.
- **Hallazgos de la tarjeta = `task` = S, con el umbral de `block_on`.** El
  pedido dice "high", que es el `block_on` de hoomai. Con el umbral del
  proyecto el tablero y `findings_open` no pueden discrepar sobre qué
  bloquea. Los hallazgos sin tarea los sigue contando verify, no la tarjeta.
- **El tablero no ejecuta los marcadores `verifica`.** Leer el tablero no puede costar
  diez minutos ni tener efectos. Por eso `spec.Trace` se parte en dos
  (tokens | comandos) y el tablero usa solo la primera mitad, la misma que usa
  verify.
- **`.hoom/items/` y `.hoom/reviews/` fuera de la huella.** Son registros
  sobre el trabajo, no el trabajo. Si estuvieran adentro, cada
  `hoom item add` en `main` pondría rojo `hoom check`, y el `hecho_en` que
  escribe `task done` rompería la huella del veredicto que lo habilitó. El
  gate de scope los sigue viendo (`gitx.Touched` no excluye nada).
- **Items y reviews en el piso del sobre.** El presupuesto de la tarjeta es un
  tope, y el que corre no afloja su propio tope (la misma razón que el
  trinquete). Si un rol pudiera escribir su item, podría marcarse hecho.
- **`task done` escribe el item del árbol donde corre** y lo deja sin
  commitear. Registrar el cierre dentro de la rama obligaría a hoom a
  commitear por la persona. Así la tarjeta pasa a Hecho en el acto, y el
  "sin sincronizar" dice lo que falta guardar.
- **`--force` no gana Hecho.** Forzar el cierre es sacar el worktree sin las
  condiciones, y marcarlo hecho sería un Hecho sin evidencia.
- **Claves en español en el item, en inglés en lo derivado.** El item usa las
  claves que fijó el pedido y su JSON repite esas mismas claves: un esquema,
  no dos. La tarjeta, la matriz y el tablero siguen la convención del resto
  del `--json` de hoom (`status`, `verdict_id`, `envelopes`), con valores en
  español.
- **Prioridad `alta|media|baja`, default `media`; tipo default `feature`.**
  El pedido no fija los valores de prioridad.
- **`--slug`** además de los flags pedidos: sin él no se puede adoptar como
  item un spec que ya existe con otro nombre, ni resolver dos títulos que dan
  el mismo slug.
- **`cliargs.Operands` en vez de relajar `Strict`.** `Strict` sigue siendo el
  de los verbos sin operandos (verify, board, roles). El verbo con operandos
  declara cuántos, y el flag vacío es typo en los dos casos.
- **Matriz: una dimensión sin límite es ENFORCED con `sin restriccion`.** El
  pedido quiere las cuatro categorías y ninguna celda sin clasificar. Una
  quinta categoría ("libre") haría que el test aceptara celdas que no dicen
  nada. `sin restriccion` dice exactamente lo que hay.
- **El mecanismo de ReadOnly se deduce de `Tools`**: quien nombra
  herramientas las niega por nombre, y quien no, usa su sandbox. Hoy es cierto
  para los dos providers que imponen ReadOnly, y CA-297 lo ata al argv real
  del adapter para que no se desvíe en silencio.
- **`gitx.Identity` y `runcmd.Alive` se exportan** en vez de copiarse. La
  identidad git estaba duplicada en `approval` y `finding`, y el item sería
  la tercera copia.

## Riesgos y deuda aceptada

- **El sobre del test-writer sigue cerrando rojo** (brecha 1). La tarjeta pasa
  a Writer por `spec_trace`, pero `red` muestra el sobre fallido hasta el
  próximo verde. Es verdad, pero puede leerse como "algo salió mal". La
  brecha la cierra otra spec.
- **Riesgo por ruta no se exige.** Un cambio chico que toca `auth` pasa a Tu
  aceptación sin review: el contrato 06 pide las 4 lentes y el tablero de C1
  no. Se sube a exigencia cuando el pedido lo diga. `Lenses` ya lo calcula.
- **Sin `hoom task` no hay Tu aceptación.** El flujo de Orca (rama
  `hoomdev/...`, sin worktree de hoom) deja las tarjetas en Review. Es la
  brecha 6.
- **El gasto es solo el de esta computadora.** Los runs y los sobres no
  viajan. La tarjeta lo rotula como telemetría local.
- **"Interrumpido" antes del run se infiere por silencio.** Sin PID del sobre
  (brecha 4, C3), un paso verify de más de 15 minutos se ve como
  interrumpido. Es el mismo umbral que ya usa `hoom status`.
- **Costo de `hoom board`.** Hace una foto de git por árbol distinto (no por
  tarjeta) y lee los veredictos y hallazgos de cada árbol. Con decenas de
  items en el árbol principal es una sola foto. Con muchos worktrees crece
  lineal.
- **Items con claves desconocidas se omiten.** Un item escrito por una
  versión más nueva de hoom con un campo nuevo desaparece del tablero de una
  versión vieja, con aviso. Es el precio de que un item no pueda guardar una
  columna.
- **La huella cambia para árboles con `.hoom/items/` o `.hoom/reviews/`.** Un
  veredicto emitido por un binario viejo sobre un árbol que ya tenía uno de
  esos directorios no coincide con el binario nuevo (lo mismo que pasó con
  `.hoom/.gitignore`). Hoy ningún veredicto de hoomai tiene esos
  directorios. La regla sigue siendo cerrar las tareas con el mismo binario
  que verificó.
- **La review de un cambio que después crece** (de 300 a 600 líneas) no
  queda exigida hasta que el nuevo veredicto verde lo diga, y entonces pide un
  registro de 4 lentes aunque ya exista uno de una lente.
