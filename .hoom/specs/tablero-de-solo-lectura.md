# Spec: tablero de solo lectura en el Studio (cabina visual, C2)

Estado: BORRADOR — pendiente de aprobación humana.
Depende de: `items-y-columna-derivada.md` (C1) integrada.
Tarea sugerida: `hoom task start tablero-de-solo-lectura`.
Origen: `.hoom/intake/rfc-cabina-visual-v2.md` (spec 2 de 4, "C2 — Tablero
de solo lectura") y el pedido de Henry del 2026-09-23, que fija la pestaña,
la cara de la tarjeta, el detalle en tres pestañas, los dos modos, el filtro
y que no haya ninguna acción que mute.

## Objetivo

Que la cabina se vea. `hoom serve` gana una pestaña **Tablero** que pinta las
ocho columnas de C1 con sus tarjetas, y un detalle por tarjeta, sin escribir
nada: no puede romper nada porque no escribe nada. Seis piezas:

1. **Dos lecturas nuevas del Studio**: `GET /api/board` (exactamente el JSON
   de `hoom board --json`) y `GET /api/board/{slug}` (el detalle de una
   tarjeta).

2. **La tarjeta crece en el binario, no en la página.** El medidor de
   evidencia, el motivo en lenguaje normal, "necesita tu decisión" y quién
   escribió y quién revisó se calculan en `boardcmd.Derive`, que sigue siendo
   pura. La página solo pinta, `hoom board --json` dice lo mismo que el
   Studio, y los gates lo prueban en Go.

3. **La pestaña Tablero** en la UI embebida que ya existe
   (`internal/servecmd/ui/`), con el mismo estilo, el mismo sondeo y sin
   dependencias ni frameworks.

4. **El detalle en tres pestañas**: Qué (el spec y sus criterios), Quién
   (sobres, runs y el panel En vivo, que reusa la vista del Escenario) y
   Pruebas (veredicto, traza por criterio y hallazgos).

5. **Modo normal y modo experto**, con un interruptor que se recuerda. El
   normal habla sin git. El experto suma diff, rutas, huella e ids.

6. **Filtro "Necesitan tu decisión"**: solo las tarjetas de las columnas
   moradas y las interrumpidas.

## No-goals

- **Acciones (C3).** Ni arrastrar, ni lanzar un rol, ni aprobar, aceptar,
  integrar, guardar en git, reanudar o verificar desde la tarjeta. El header
  del Studio (token, botón Verificar, cajones) queda como está, y la pestaña
  Tablero no le suma ninguna acción.
- **Fantasmas en la columna destino.** Llegan con el arrastre de C3, que es lo
  que le da sentido a "columna destino". En C2 el trabajo en curso se pinta en
  la propia tarjeta, con su paso.
- **Curva del trinquete, subestados de Review** (revisando, refutando,
  corrigiendo), **timeline y replay** (C4) y **espejo del pane de tmux**. El
  pedido no los incluye. La curva sigue en el roadmap del README.
- **Un verbo CLI nuevo para el detalle** (ver Decisiones).
- **Diff sin espacio de trabajo.** Sin `.hoom/worktrees/<slug>` no hay
  `base...HEAD` de la tarea (el flujo de Orca es la brecha 6 de C1).
- **Cambiar el cockpit, los cajones o sus endpoints.** El único cambio es que
  las funciones que pintan el Escenario y el Feed reciben su contenedor, para
  que el detalle las reuse.
- **SSE o websockets.** Sondeo, como el resto del Studio.
- **Persistir el filtro.** Solo el modo se recuerda.
- **Cambiar la columna o sus condiciones.** La derivación de C1 queda igual:
  C2 solo le agrega campos a la tarjeta.

## Contratos

### La tarjeta crece: campos nuevos de `boardcmd.Card`

Todos se derivan en `Derive` (pura: sin disco, sin reloj propio, sin git).
Son aditivos: las claves de C1 no cambian, y `hoom board --json`,
`hoom item show --json` y `/api/board` los emiten igual. Ninguna lista es
`null`. El texto de `hoom board` y de `hoom item show` no cambia.

```json
{
  "...": "todas las claves de C1",
  "plain": "faltan pruebas para 1 de 12 criterios",
  "needs_decision": false,
  "meter": [
    {"id": "spec",   "label": "spec aprobado", "state": "hecho",    "detail": "aprobado para el contenido actual"},
    {"id": "CA-302", "label": "CA-302",        "state": "hecho",    "detail": "con prueba"},
    {"id": "CA-303", "label": "CA-303",        "state": "falta",    "detail": "sin prueba"},
    {"id": "build",  "label": "build",         "state": "falta",    "detail": "todavia no hay veredicto"},
    {"id": "static", "label": "static",        "state": "falta",    "detail": "todavia no hay veredicto"},
    {"id": "test",   "label": "test",          "state": "falta",    "detail": "todavia no hay veredicto"},
    {"id": "review", "label": "review",        "state": "falta",    "detail": "todavia no hay veredicto"}
  ],
  "providers": {"writer": "claude", "reviewer": "codex", "cross": "cruzada"},
  "red": {"source": "sobre", "id": "...", "reason": "(el de C1)",
          "plain": "el writer no entrego: escribio fuera de lo que le toca"}
}
```

#### `meter`: el medidor de evidencia

Segmentos en orden fijo: `spec`, uno por criterio (en el orden de
`spec.Lint`, el mismo de `Evidence.Criteria`), `build`, `static`, `test` y
`review`. `state` es `hecho`, `falta` o `no-aplica`, y `detail` es una frase
sin vocabulario de git. Un segmento se llena (`hecho`) solo con un hecho que
existe y vale hoy:

| segmento | `hecho` | `no-aplica` | `falta` (detail) |
|---|---|---|---|
| `spec` | aprobación vigente: `aprobado para el contenido actual` | nunca | sin spec: `todavia no hay spec`. Lint con issues: `el spec no esta completo`. Sin aprobación: `espera tu aprobacion`. Invalidada: `el spec cambio despues de tu aprobacion` |
| `CA-n` | el criterio no está en `untraced`: `con prueba`, o `con comando de verificacion` si lo cubre un marcador `verifica` | nunca | `sin prueba` |
| `build`, `static`, `test` | el veredicto de la tarjeta trae el gate en `pass` y su huella es la actual: `paso en el veredicto vigente` | el veredicto de la tarjeta no trae ese gate: `el proyecto no declara el gate <g>` | sin veredicto de la tarjeta: `todavia no hay veredicto`. Huella distinta: `el codigo cambio despues del veredicto`. Otro estado: `el gate <g> no paso (<estado>)` |
| `review` | hay un registro de review que cuenta (`evidence.review_id` no vacío): `revision de 4 lentes registrada` | hay veredicto de la tarjeta y no exige review: `el cambio es chico (<n> lineas): no exige la revision de 4 lentes` | sin veredicto: `todavia no hay veredicto`. Exigida y sin registro que cuente: `falta la revision de 4 lentes` |

- Los segmentos de criterios existen solo si el spec existe y pasa lint. Sin
  eso, el medidor tiene cinco segmentos.
- Un gate en `pass` de un veredicto ROJO con la huella actual llena su
  segmento: el hecho "build pasó sobre este código" vale aunque otro gate
  haya fallado.
- En la columna `hecho` no se exige la huella actual para `build`, `static`
  y `test`: una tarjeta cerrada no se reevalúa contra lo que pasó después
  (C1, "Hecho es terminal").
- `Evidence` gana `ByCommand []string`: los criterios que cubre un marcador
  `verifica` (lo junta `Gather`, que ya lee los comandos de `spec.Lint`).

#### `plain`: el motivo en una frase, para el modo normal

Nunca vacío. Sin acentos, como toda cadena del binario. Nunca contiene las
palabras `git`, `commit`, `worktree`, `merge`, `rama`, `branch`, `diff`,
`HEAD` ni `huella`, ni `.hoom/`, ni un comando `hoom `. Es la primera entrada
de `missing` dicha con el vocabulario del RFC v2:

| columna | caso | `plain` |
|---|---|---|
| backlog | — | `falta el spec de la tarjeta` |
| arquitecto | issues de lint | `el spec no esta completo (<n> problema(s) de formato)` |
| tu-aprobacion | sin aprobación | `el spec espera tu aprobacion` |
| tu-aprobacion | invalidada | `el spec cambio despues de tu aprobacion: hay que aprobarlo de nuevo` |
| test-writer | — | `faltan pruebas para <x> de <n> criterios` |
| writer | sin veredicto | `falta verificar el trabajo contra el spec` |
| writer | veredicto rojo | `la verificacion dio rojo` + `: fallo el gate <g>` o `: fallaron los gates <g1>, <g2>` (los requeridos, como el `missing` de C1) |
| writer | otra huella | `el codigo cambio despues del ultimo verde` |
| review | review exigida | `falta la revision de 4 lentes (<n> lineas)` |
| review | hallazgos | `hay <n> hallazgo(s) que bloquean` |
| review | sin `spec_approved` en pass | `hay que verificar de nuevo: el ultimo verde no incluye la aprobacion del spec` |
| review | sin gate `findings_open` | `el proyecto no declara que hallazgos bloquean` |
| review | `findings_open` no pasa | `hay que verificar de nuevo: el control de hallazgos no paso` |
| review | sin tarea | `falta el espacio de trabajo de la tarjeta` |
| review | `Ready`: `sin-tarea` | `falta el espacio de trabajo de la tarjeta` |
| review | `Ready`: `sin-guardar` | `hay cambios sin guardar en el espacio de trabajo` |
| review | `Ready`: `sin-veredicto` o `solo-parciales` | `falta verificar el espacio de trabajo completo` |
| review | `Ready`: `rojo` | `la ultima verificacion del espacio de trabajo dio rojo` |
| review | `Ready`: `huella` | `el espacio de trabajo cambio despues del ultimo verde` |
| tu-aceptacion | — | `espera tu aceptacion para integrar` |
| hecho | — | `terminada: su cierre quedo registrado` |

- En Review, con más de una condición en falta, se agrega
  ` (y <n> pendientes mas)` con la cantidad de las demás.
- Para saber qué dijo `taskcmd.Ready` sin parsear su mensaje, `Ready`
  devuelve `*taskcmd.ReadyError{Kind string}` con `Kind` en `sin-tarea`,
  `sin-guardar`, `sin-veredicto`, `solo-parciales`, `rojo` o `huella`.
  `Error()` devuelve exactamente el mensaje de hoy, así que `task done` y el
  `missing` de C1 no cambian. `Gather` llena `Evidence.ReadyKind` con
  `errors.As`.

#### `red.plain`

`red` de C1 gana `plain`, sin la nota del sobre (puede traer rutas):

- Veredicto rojo: el mismo texto del caso "writer, veredicto rojo" de arriba.
- Sobre `sin-entrega`: `el <rol> no entrego: no dejo ningun archivo`.
- Sobre `no-entregable`: `el <rol> no entrego: <glosa>`, con la glosa del
  paso donde cortó: `spec` → `el spec no tenia aprobacion vigente`, `aislar`
  → `no se pudo preparar el espacio ciego`, `run` → `el agente termino con
  error`, `scope` → `escribio fuera de lo que le toca`, `verify` → `la
  verificacion dio rojo`, `check` → `el control final no paso`, y cualquier
  otro → `corto en el paso <stage>`.

`reason` queda como lo fijó C1.

#### `needs_decision`

`true` si `waiting_human` (las dos columnas moradas) o si hay
`interrupted`, en cualquier columna. Una tarjeta en curso no cuenta. Es lo
que filtra "Necesitan tu decisión".

#### `providers`: quién escribió y quién revisó

`{"writer", "reviewer", "cross"}`, cada uno el nombre de un provider o `""`:

1. Si hay un registro de review de la tarjeta, se usa el que cuenta
   (`evidence.review_id`) o, si ninguno cuenta, el más nuevo de la tarjeta:
   `reviewer` es su `provider`, `writer` su `writer` y `cross` su `cross`.
2. Si `writer` sigue vacío, es el provider del run más nuevo de la tarjeta
   cuyo rol escribe código, y si no hay, el del sobre más nuevo con esa
   condición. "Escribe código" es una sola función nueva,
   `agents.WritesCode(role string) bool`: falso para los roles `ReadOnly` y
   los de scope `specs`, verdadero para los demás, para un rol vacío
   (`hoom run`) y para uno desconocido. `reviewcmd.writerOf` pasa a usarla,
   así que la review cruzada y el logo no pueden discrepar.
3. `reviewer` y `cross` salen solo de registros de review. Un run de review
   sin registro no es una review.

### El detalle: `boardcmd.DetailFor`

`boardcmd.DetailFor(root, base, blockOn, slug string, now time.Time, withDiff
bool) (Detail, error)` lee con el mismo `Gather`, falla igual que `CardFor`
(item inexistente o inválido) y es de solo lectura como él.

```json
{
  "card": { "...": "la misma Card de hoom item show <slug> --json" },
  "spec": {"path": ".hoom/specs/S.md", "exists": true,
           "markdown": "# Spec: ...", "approval": { "...": "approval.Record" }},
  "criteria": [{"id": "CA-302", "text": "el medidor ...", "traced_by": "test",
                "files": ["internal/boardcmd/meter_test.go"]}],
  "verdict": { "...": "el veredicto completo de la tarjeta, o null" },
  "fingerprint": "<huella actual del arbol de evidencia, o vacio sin veredicto>",
  "findings": [ { "...": "finding.Item, abiertos y resueltos" } ],
  "reviews": [ { "...": "reviewcmd.Record" } ],
  "work": [ { "...": "WorkRow" } ],
  "paths": {"item": ".hoom/items/S.yaml", "dir": ".hoom/worktrees/S",
            "spec": ".hoom/worktrees/S/.hoom/specs/S.md",
            "approval": ".hoom/worktrees/S/.hoom/approvals/S_1a2b3c4d.json",
            "verdict": ".hoom/worktrees/S/.hoom/verdicts/<id>.json",
            "reviews": [], "findings": []},
  "diff": { "...": "solo con withDiff" }
}
```

- **`spec`**: ruta y contenido del spec en el árbol de evidencia, y la
  aprobación vigente (`approval.Status`), o `null`. Sin spec: `exists: false`
  y `markdown` vacío.
- **`criteria`**: uno por id de `spec.Lint`, en su orden. `text` sale de una
  función nueva `spec.Criteria(path string) ([]Criterion, error)`: el
  enunciado del ítem de lista de la sección "criterios de aceptacion" que
  empieza con el token (`- CA-n:`), con sus líneas de continuación unidas por
  un espacio, sin el prefijo `CA-n:` y sin el marcador `verifica`. Un id que
  solo aparece citado en otra parte del spec tiene `text` vacío. `traced_by`
  es `comando` si lo cubre un marcador `verifica`, `test` si algún archivo de
  test cita el token, y `""` si ninguno. `files`: los archivos de test que lo
  citan, relativos al árbol de evidencia, ordenados. Sin spec, o con issues
  de lint, `criteria` es `[]`.
- **Índice de tokens**: `spec.IndexTokens(root string) (TokenIndex, error)`
  recorre una vez los archivos de test (el mismo filtro de hoy: nombre con
  `test` o `spec`, sin `skipDirs`, hasta 2 MiB) y guarda, por token entero,
  los archivos que lo citan. Un token no coincide dentro de otro con más
  dígitos, igual que hoy. `TokenIndex.Files(id) []string`,
  `TokenIndex.Scanned`. `spec.Tokens` pasa a ser el índice más "ids sin
  archivos", con el mismo resultado de hoy, y `Build` arma el índice una vez
  por árbol (como ya hace con veredictos y hallazgos), no una vez por
  tarjeta.
- **`verdict`**: el veredicto completo de la tarjeta (`Evidence.Verdict`),
  con sus gates y colas de salida. **`fingerprint`**: `Evidence.Fingerprint`.
- **`findings`**: todos los hallazgos del árbol de evidencia con `task` = S,
  abiertos y resueltos, con su resolución, en el orden de `finding.List`.
  Nunca los de otra tarea ni los sin tarea.
- **`reviews`**: los registros de review de la tarjeta (`Evidence.Reviews`).
- **`work`**: lo que corrió para la tarjeta, del más nuevo al más viejo:

  ```json
  {"kind": "sobre", "id": "...", "run_id": "...", "role": "writer",
   "provider": "claude", "status": "entregable", "stage": "ok", "step": 7,
   "steps": 7, "isolated": false, "started_at": "...", "ended_at": "...",
   "duration_ms": 184000, "alive": false,
   "usage": {"cost_usd": 0.8, "input_tokens": 51000, "output_tokens": 3000},
   "note": ""}
  ```

  Una fila `sobre` por sobre de la tarjeta, y una fila `run` por cada run de
  la tarjeta que ningún sobre referencia (`hoom run --task`, las pasadas de
  `hoom review`). El `usage` de un sobre es el del sidecar de su run si
  existe, el del propio sobre si no, y `null` si el sobre no tiene run. Así la
  suma de las filas es exactamente `card.spend`. `duration_ms`: cerrado,
  `ended_at − started_at`; abierto con dueño vivo, `now − started_at`; sobre
  abierto sin dueño, `updated_at − started_at`; run sin cerrar y sin dueño,
  `null`. `alive` es el que resolvió `Gather`. `status` de un run:
  `running|done|error|canceled`.
- **`paths`**: las rutas de cada artefacto, relativas a `root`, vacías cuando
  no aplica: el item del árbol actual, el árbol de evidencia (`dir`), el spec,
  el archivo de la aprobación vigente (`S_<sha8>.json`), el veredicto de la
  tarjeta, sus registros de review y sus hallazgos (`<id>.json`, y
  `<id>.res.json` si está resuelto).
- **`diff`** (solo con `withDiff`), de una función nueva
  `gitx.BranchDiff(dir, base string, maxBytes int) (Diff, error)`:

  ```json
  {"available": true, "base": "main", "head": "<sha12>",
   "files": [{"path": "internal/x.go", "insertions": 3, "deletions": 1}],
   "insertions": 3, "deletions": 1, "patch": "diff --git ...",
   "truncated": false, "note": ""}
  ```

  Con el worktree de la tarea: `git diff --no-color --no-ext-diff
  <base>...HEAD` y su `--numstat`, en el worktree. Lo sin commitear no entra
  (lo muestra `unsynced`). El parche se corta en 256 KiB en un fin de línea y
  `truncated` pasa a `true`. Sin worktree: `available: false` y `note: "sin
  espacio de trabajo de la tarea: no hay diff base...HEAD que mostrar"`. Si
  git falla (por ejemplo, no existe la rama base), `available: false` con el
  error de git en `note`.

### Endpoints

- **`GET /api/board`**: `boardcmd.JSONBytes(boardcmd.Build(m.Dir,
  m.BaseBranch, m.FindingsBlockOn(), now))`, los mismos bytes que imprime
  `hoom board --json` (salvo el salto de línea final). Un error de `Build`
  es 500 JSON.
- **`GET /api/board/{slug}`**: slug con forma inválida (`item.ValidSlug`) es
  400 JSON. Item inexistente o inválido es 404 JSON con el mensaje de
  `CardFor`. Si no, 200 con el `Detail`. `?diff=1` pide el diff.
- Los dos son lecturas: sin token, como el resto de las lecturas del Studio.
  Cualquier otro método es 405 y no tiene efectos. Ninguno crea ni modifica un
  archivo del repo ni de `.hoom/`.

### La UI

- **Pestañas.** Arriba de `<main>`: **Cockpit** (la pantalla de hoy, sin
  cambios) y **Tablero**. El header con el chip del check sigue siempre
  visible.
- **`internal/servecmd/ui/tablero.js`**, archivo nuevo embebido que
  `index.html` carga con `<script src="tablero.js">` después de su script.
  Tiene toda la lógica de la pestaña, y lee solo con `j()` (GET) las URLs
  `/api/board`, `/api/board/{slug}` (con `?diff=1` en su caso) y, para el
  panel En vivo, `/api/runs/{id}?after=n` y `/api/runs/{id}/stage`. El CSS
  va en el `<style>` de `index.html`, con las mismas variables y una nueva,
  `--human`, el morado de las columnas humanas.
- **El tablero.** Las columnas en el orden del JSON (la página no las
  reordena ni las escribe a mano), cada una con su nombre (una tabla de
  etiquetas con acentos por `id`, y el `name` del JSON si el id no está), su
  conteo y el morado cuando `human` es `true`. Scroll horizontal entre
  columnas y vertical dentro de cada una. Sin items: las ocho columnas vacías
  y `todavía no hay tarjetas: se crean con hoom item add "<título>"`. Los
  `warnings` se ven: en normal, cuántas tarjetas no se pudieron leer; en
  experto, cada aviso.
- **La cara de la tarjeta**: título, tipo y prioridad. El medidor: una celda
  por segmento, llena si `hecho`, vacía si `falta`, tenue si `no-aplica`, con
  la etiqueta y el `detail` al pasar el mouse (en normal, un criterio se
  nombra "criterio i de n"; en experto, por su id). Los subestados con ícono:
  en curso con su paso (`paso X de N: <stage>`), interrumpido
  (`interrumpido en el paso X de N`), esperando humano, rojo con su motivo en
  una frase y sin sincronizar. El gasto y el presupuesto: `0.8 de 5 USD`, o
  `sin dato de costo` más los tokens, nunca `0 USD` por falta de dato, en
  color de alerta si el gasto supera el presupuesto, y rotulado como
  telemetría local. Los dos logos: el del provider que escribió y el del que
  revisó, cada uno con la marca de su CLI, y un aviso de "misma CLI" si
  `cross` es `no-cruzada`.
- **El logo** es una marca tipográfica embebida por provider (un glifo y su
  color, en una tabla de `tablero.js`), y una genérica con la inicial para un
  provider que no está en la tabla. Sin imágenes.
- **El detalle.** Clic en una tarjeta abre un panel lateral ancho sobre el
  tablero, con tres pestañas, que se cierra con Esc o con su botón:
  - **Qué**: título, tipo, prioridad y pedido del item. Los criterios como
    lista de frases (`text`), cada una con su estado (con prueba, con
    comando, sin prueba), y "criterio citado en el spec" si `text` está
    vacío. Después el spec renderizado con el mismo `mdRender` de los specs.
  - **Quién**: una fila por entrada de `work` (rol, logo del provider,
    estado en palabras, costo o tokens, duración). Debajo, **En vivo**: si
    `card.running.run_id` existe, el Escenario y el Feed de ese run, pintados
    con las mismas funciones del cockpit (`renderStage`, `appendFeed`), que
    pasan a recibir su contenedor. Si hay un sobre en curso sin run, `paso X
    de N (<stage>): en este paso no habla ningún agente`. Si no hay nada en
    curso, `nadie está trabajando en esta tarjeta ahora`.
  - **Pruebas**: el veredicto de la tarjeta (color, fecha, gates con su
    estado y si es requerido), la traza por criterio (texto y estado) y los
    hallazgos: primero los abiertos, después los resueltos, con severidad,
    descripción, estado y la evidencia de la resolución.
- **Modo normal y modo experto.** Un interruptor `Normal | Experto` en la
  barra del Tablero, guardado en `localStorage` con la clave
  `hoom-tablero-modo` (`normal` o `experto`), leído y escrito dentro de
  `try/catch`, `normal` por defecto. Cambiarlo repinta al instante, sin
  pedir de nuevo.
  - El **normal** nunca muestra diff, rutas, ids, huellas, comandos (`next`)
    ni la lista cruda de `missing`: usa `plain` y `red.plain`, dice "N
    cambios sin guardar" en vez de las rutas, y habla de "espacio de
    trabajo", "integrar" y "guardar". Su vocabulario propio vive en una sola
    tabla de `tablero.js`, entre los comentarios `/* vocabulario normal */` y
    `/* fin vocabulario normal */`. El contenido del spec y de los criterios
    se muestra como está escrito: es el documento que la persona aprueba.
  - El **experto** suma: `missing` completo y `next`, las rutas de `unsynced`
    y de `paths`, los ids (veredicto, hallazgos, sobres, runs, registros de
    review, criterios), la huella del veredicto contra la actual, las colas
    de salida de los gates, los archivos de test por criterio, `reason` en
    vez de `red.plain`, y en Qué una sección plegable **Diff** que pide
    `?diff=1` solo mientras está abierta.
- **Filtro "Necesitan tu decisión"**: un interruptor en la barra que deja
  solo las tarjetas con `needs_decision`. Las ocho columnas siguen ahí y el
  conteo dice "x de y".
- **Sondeo.** Con la pestaña Tablero visible y la página visible
  (`document.visibilityState`), `/api/board` se pide de nuevo 1000 ms
  después de cada respuesta (`setTimeout` encadenado: dos pedidos nunca se
  pisan). El detalle abierto, igual. El panel En vivo, cada 1000 ms mientras
  el run corre, como la vista del run del cockpit. Al salir de la pestaña se
  deja de pedir. El ciclo de 5 s del Studio no cambia.
- **Idioma.** Las etiquetas de la página en español con acentos y voseo, como
  la UI de hoy. Las cadenas del binario se muestran como llegan, sin acentos.
- **Nada que mute.** Ningún elemento `draggable`, ningún manejador de
  arrastre, ningún formulario, ningún botón que haga POST y ningún uso del
  token. Los únicos botones de la pestaña cambian lo que se ve (pestañas,
  modo, filtro, abrir y cerrar el detalle, plegar el diff).

### Documentación

- README: la sección del Studio describe la pestaña Tablero, los dos
  endpoints y los dos modos. En el roadmap de la cabina, el ítem del tablero
  de solo lectura queda reducido a lo que falta: la curva del trinquete.

## Casos límite y errores esperados

- Sin items: `/api/board` trae las ocho columnas vacías y la pestaña lo dice.
- Un item inválido: la tarjeta no está, y el aviso sí (en experto, con el
  archivo y el motivo).
- Una tarjeta en Hecho con el código de `main` movido después: su medidor
  sigue lleno en `build`, `static` y `test`, porque en Hecho no se pide la
  huella actual.
- Un veredicto rojo donde `build` pasó y `test` falló, con la huella actual:
  `build` lleno, `test` vacío con `el gate test no paso (fail)`.
- Un proyecto sin gate `static`: el segmento `static` es `no-aplica`, no
  `falta`.
- Una tarjeta interrumpida en Writer: `needs_decision: true`, y el filtro la
  deja aunque su columna no sea morada.
- Una tarjeta en curso y esperando humano a la vez (un sobre corre mientras
  el spec espera aprobación): `needs_decision: true` por la columna.
- Un sobre en curso en el paso `verify`, sin run vivo: En vivo dice en qué
  paso está y que ahí no habla ningún agente.
- El run activo lo corre otro proceso (`hoom agent` en una terminal): el
  panel En vivo lo lee igual, con la lectura que ya tiene
  `/api/runs/{id}` para runs ajenos.
- `/api/runs/{id}` responde 404 (el sidecar no está en este proyecto): el
  panel lo dice y deja de pedir.
- El run termina con el detalle abierto: En vivo queda con la narración final
  y deja de pedir, como la vista del run.
- La tarjeta desaparece con el detalle abierto (el item se borró): el detalle
  recibe 404, lo dice y deja de pedir.
- `hoom serve` se cae: la pestaña conserva lo último que pintó, muestra el
  error y sigue intentando cada segundo.
- Runs de Codex sin costo y uno de Claude con 0.8 USD: la cara dice `0.8 USD`
  y los tokens de todos, y la suma de `work` coincide con `spend`.
- Una review hecha con `--same-provider`: los dos logos iguales y el aviso de
  "misma CLI".
- Sin registro de review ni runs locales (la tarjeta llegó por git desde otra
  computadora): sin logos, porque la telemetría no viaja. No se inventa un
  autor.
- `localStorage` no disponible (ventana privada): el modo arranca en normal y
  el interruptor funciona durante la sesión.
- Detalle de un slug con mayúsculas o espacios: 400. De un slug válido sin
  item: 404.
- `POST /api/board` con el token correcto: 405, sin efectos.
- El diff de un cambio de 8000 líneas: se corta en 256 KiB y lo dice.
- Un spec que cita en su texto criterios de otro spec: esos ids son criterios
  para `spec.Lint` (así es hoy), tienen segmento en el medidor y aparecen en
  Qué como "criterio citado en el spec".

## Criterios de aceptación

- CA-302: el `meter` de una `Card` trae `spec`, un segmento por criterio en el orden de `spec.Lint`, `build`, `static`, `test` y `review`, en ese orden, con `state` en `hecho|falta|no-aplica` y `detail` no vacío. Sin spec, o con issues de lint, no hay segmentos de criterios. `hoom board --json` y `hoom item show --json` conservan todas las claves de C1 y suman `plain`, `needs_decision`, `meter` y `providers`, sin listas `null`.
- CA-303: el segmento `spec` da los cinco casos del contrato (sin spec, lint con issues, sin aprobación, invalidada, aprobada) con su `detail`. Un criterio con test es `hecho` con `con prueba`, uno cubierto por un marcador `verifica` es `hecho` con `con comando de verificacion`, y uno sin nada es `falta` con `sin prueba`.
- CA-304: `build`, `static` y `test` son `hecho` solo con el gate en `pass` en el veredicto de la tarjeta y su huella actual. Con otra huella son `falta` con `el codigo cambio despues del veredicto`. Un gate `fail`, `error` o `absent` es `falta` y nombra el estado. Sin veredicto de la tarjeta, `falta`. Un veredicto sin ese gate da `no-aplica`. En un veredicto rojo con la huella actual, el gate que pasó es `hecho`. En la columna `hecho` no se exige la huella.
- CA-305: `review` es `hecho` con un registro que cuenta, `no-aplica` con un veredicto de la tarjeta de 400 líneas o menos y sin registro, y `falta` con más de 400 líneas y sin registro que cuente (un registro de una sola lente no cuenta), y también sin veredicto.
- CA-306: `plain` da el texto del contrato en cada caso de la tabla, con fixtures que recorren todas las ramas de la columna de C1. En Review con varias condiciones en falta agrega `(y N pendientes mas)`. En todos los fixtures `plain` es no vacío, no contiene las palabras `git`, `commit`, `worktree`, `merge`, `rama`, `branch`, `diff`, `HEAD` ni `huella` (enteras, sin distinguir mayúsculas) y no contiene las cadenas `.hoom/` ni `hoom `.
- CA-307: `taskcmd.Ready` devuelve `*taskcmd.ReadyError` con `Kind` `sin-tarea`, `sin-guardar`, `sin-veredicto`, `solo-parciales`, `rojo` o `huella` según el caso, y su `Error()` es byte a byte el mensaje de antes. `Gather` llena `Evidence.ReadyKind`, y `plain` en Review sale de él. Los tests de `task done` y de C1 pasan sin cambios.
- CA-308: `red.plain` de un veredicto rojo es `la verificacion dio rojo: fallo el gate test`. El de un sobre `sin-entrega` es `el <rol> no entrego: no dejo ningun archivo`, y el de un `no-entregable` usa la glosa de su paso (`scope` → `escribio fuera de lo que le toca`, y las demás del contrato). `plain` nunca incluye la nota del sobre, y `reason` no cambia.
- CA-309: `needs_decision` es `true` en `tu-aprobacion` y `tu-aceptacion`, y en cualquier columna con `interrupted`. Es `false` en las demás, también con `running`.
- CA-310: `providers` toma `reviewer`, `writer` y `cross` del registro de review que cuenta, o del más nuevo de la tarjeta si ninguno cuenta. Sin `writer` en el registro, o sin registro, `writer` es el provider del run más nuevo de la tarjeta con un rol que escribe código, y si no hay, el del sobre más nuevo. Los roles de specs (arquitecto, analista, designer) y los `ReadOnly` (reviewer, scout) nunca son `writer`. Sin datos, los tres son `""`. `agents.WritesCode` es la regla, y `reviewcmd.writerOf` la usa sin que cambien sus tests.
- CA-311: `spec.Criteria` devuelve un `Criterion` por id de `spec.Lint`, en su orden, con el enunciado de su ítem en la sección de criterios: líneas de continuación unidas, sin el prefijo del id y sin el marcador `verifica`. Un id que solo aparece citado en otro lugar tiene `text` vacío.
- CA-312: `spec.IndexTokens` guarda por token entero los archivos de test que lo citan (un token no coincide dentro de otro con más dígitos), con el mismo filtro de archivos de hoy. `spec.Tokens` da el mismo resultado que antes, y los tests existentes de `spec.Tokens` y `spec.Trace` pasan sin cambios.
- CA-313: `DetailFor` devuelve una `card` igual (igualdad profunda) a la de `CardFor` para el mismo slug. `spec` trae el markdown y la aprobación vigente. `criteria` trae `text`, `traced_by` (`test`, `comando` o vacío) y `files`. `verdict` es el veredicto completo de la tarjeta. `findings` trae los de la tarjeta abiertos y resueltos (con su resolución), y ninguno de otra tarea ni sin tarea. `paths` nombra rutas relativas a `root` que existen en disco.
- CA-314: `work` trae una fila por sobre de la tarjeta y una por run de la tarjeta que ningún sobre referencia, del más nuevo al más viejo. El `usage` sigue la regla del contrato, y la suma de `cost_usd` y de los tokens de las filas es igual a `card.spend` (costo `null` si ninguna fila trae costo). `duration_ms` cumple los cuatro casos del contrato, y `alive` coincide con lo que resolvió `Gather`.
- CA-315: sin `withDiff` el detalle no trae `diff`. Con worktree trae `base...HEAD` con archivos, inserciones, borrados, `head` y parche, sin los cambios sin commitear, y un parche de más de 256 KiB se corta con `truncated: true`. Sin worktree, `available: false` con la nota del contrato.
- CA-316: `GET /api/board` emite los mismos bytes que `boardcmd.JSONBytes(boardcmd.Build(...))` sobre el mismo árbol, que es lo que imprime `hoom board --json`. Con fixtures de C1 para backlog, tu-aprobacion, test-writer, writer, review y hecho, cada tarjeta está en su columna, `human` es `true` solo en las dos humanas y un item inválido aparece en `warnings`. Cada test cita el criterio de C1 que fija la columna de su fixture.
- CA-317: `GET /api/board/{slug}` responde 200 con el detalle, y su `card` es igual a la de `hoom item show <slug> --json`. Un slug inválido es 400 JSON, un item inexistente es 404 JSON con el mensaje de `CardFor`, y `?diff=1` agrega `diff`.
- CA-318: `POST`, `PUT`, `PATCH` y `DELETE` sobre `/api/board` y `/api/board/{slug}` responden 405, con token o sin él. Los `GET` no piden token. Pedir el tablero y el detalle (con diff) deja el repo y `.hoom/` byte a byte iguales, ignorados incluidos.
- CA-319: `index.html` tiene las pestañas Cockpit y Tablero y carga `tablero.js`, que se sirve embebido (`GET /tablero.js` es 200), y el test de UI sin assets de red pasa sobre los dos archivos. `index.html` define `--human`. `tablero.js` pinta las columnas desde `columns` y usa `human`, `needs_decision`, `meter`, `plain` y `providers`. `renderStage` y `appendFeed` reciben su contenedor, y `tablero.js` los reusa sin definir los suyos.
- CA-320: la pestaña Tablero no emite ningún POST: `tablero.js` no contiene `act(`, `POST`, `method`, `fetch(`, `XMLHttpRequest`, `sendBeacon`, `X-Hoom-Token`, `draggable`, `dragstart`, `drop` ni `<form`. Cada literal `/api/` de `tablero.js` es `/api/board`, `/api/board/` o `/api/runs/`, y un GET a cada uno no es 405. La función `j` de `index.html` llama a `fetch` sin opciones.
- CA-321: `tablero.js` guarda el modo en `localStorage` con la clave `hoom-tablero-modo` y los valores `normal` y `experto`, dentro de `try/catch`. El bloque del vocabulario normal contiene "espacio de trabajo", "integrar" y "guardar", y no contiene `git`, `commit`, `worktree`, `merge`, `rama`, `branch`, `diff`, `HEAD` ni `huella`. El filtro se llama "Necesitan tu decisión" y filtra por `needs_decision`.
- CA-322: `tablero.js` sondea con `setTimeout(..., 1000)` encadenado a cada respuesta y no usa `setInterval`. Las etiquetas "Qué", "Quién", "Pruebas", "Tu aprobación", "Tu aceptación" y "Necesitan tu decisión" están con sus acentos.
- CA-323: dogfood: hoomai tiene el item de esta spec. [verifica: go run ./cmd/hoom item show tablero-de-solo-lectura --json | grep -q '"slug": "tablero-de-solo-lectura"']
- CA-324: el README documenta la pestaña Tablero y sus endpoints. [verifica: grep -q "/api/board" README.md]

## Decisiones

- **La tarjeta crece en `Derive`, no en la página.** El medidor, `plain`,
  `needs_decision` y `providers` podrían calcularse en JavaScript con lo que
  ya trae la tarjeta, pero entonces la regla viviría en la página, sin test de
  Go y distinta de `hoom board --json`. Es la misma decisión que tomó el
  Escenario (v4) con `runcmd.Stage`: el binario calcula y la página pinta.
- **El modo normal no esconde un rojo: lo dice con otras palabras.** Por eso
  `plain` y `red.plain` son del binario, con una tabla cerrada y un test que
  recorre todas las ramas de la columna. Si C1 suma una rama sin su `plain`,
  el test falla. El normal no es un resumen optimista.
- **`/api/board` es `hoom board --json`, byte a byte**, como `/api/tasks` y
  `/api/report` desde el Studio v1: una representación y dos pieles.
- **El detalle no tiene verbo propio.** Su `card` es la de
  `hoom item show --json`, y el resto son lecturas que ya tienen verbo
  (`hoom finding list`, los veredictos, `hoom status` con los sobres) juntadas
  por slug, más el diff de git. El precedente es `/api/runs/{id}/stage`, del
  v4. Si algún día hace falta en la terminal, es un flag de `item show` sobre
  la misma función.
- **`tablero.js` en un archivo aparte.** El pedido exige un test que afirme
  que la pestaña no emite ningún POST. Con la pestaña en su propio archivo, el
  test lee exactamente la pestaña. Una región entre comentarios dentro de
  `index.html` sería un borde que nadie ve.
- **Sondeo de 1 s encadenado, solo con la pestaña visible.** Es el ritmo de la
  vista del run del cockpit, y encadenado a la respuesta nunca apila pedidos
  aunque el tablero tarde. El ciclo general de 5 s sigue para el header.
- **Índice de tokens por árbol.** Con sondeo por segundo, recorrer los
  archivos de test una vez por tarjeta crece con cada item. Una pasada por
  árbol cuesta lo mismo con una tarjeta que con veinte, y el detalle la
  necesita igual para `files`.
- **El medidor se llena solo con hechos vigentes**: un gate que pasó sobre
  otra huella no llena su segmento. La excepción es Hecho, que C1 ya declaró
  terminal.
- **`review` es `no-aplica` en un cambio chico, no `hecho`.** Llenarlo diría
  que hubo una review que no hubo.
- **Los logos salen primero del registro de review**, que viaja en git, y
  solo después de la telemetría local. El revisor sale solo de registros: un
  run de review sin registro no terminó una review.
- **`WritesCode` en `agents`.** La regla "este rol escribe código" existía
  solo dentro de `reviewcmd.writerOf`. El logo y el rechazo de la review no
  cruzada tienen que usar la misma.
- **`ReadyError` tipado en vez de parsear mensajes.** `plain` necesita saber
  qué dijo `Ready`, y un mensaje es para personas. Los mensajes no cambian.
- **Logos tipográficos, no imágenes.** La UI es autocontenida desde el v1 y no
  reproduce marcas registradas. Cada CLI tiene un glifo y un color propios en
  una tabla, y un provider nuevo tiene la marca genérica con su inicial.
  Pendiente de confirmar al aprobar.
- **El diff solo en experto, con worktree y a pedido.** Es git puro, puede
  pesar cientos de KB, y el modo normal no muestra git. Se pide solo con la
  sección abierta.
- **Fantasmas, curva del trinquete y subestados de Review quedan afuera.** El
  RFC los ponía en C2, pero el pedido no. El fantasma necesita el arrastre de
  C3 para tener una columna destino. La curva queda en el roadmap.
- **El filtro no se recuerda y el modo sí.** El modo es una preferencia de la
  persona. El filtro es una pregunta del momento.

## Riesgos y deuda aceptada

- **El costo del sondeo.** Cada segundo se recalcula todo el tablero: git
  status y huella por árbol, lint por spec y el índice de tokens. Hoy son unos
  300 ms con una tarjeta. Si `Build` tarda más de un segundo, el ritmo
  efectivo baja (el encadenado nunca apila pedidos). Si molesta, el cambio
  interno es memorizar por huella, sin tocar contratos.
- **Los tests de la UI son estáticos.** Afirman lo que está y lo que no está
  en `tablero.js` e `index.html`, como los del Studio de hoy, pero no pintan.
  Que se vea bien se comprueba a mano con `hoom serve` antes del PR.
- **Los logos del writer pueden faltar en otra computadora.** Sin registro de
  review, el writer sale de la telemetría local, que no viaja.
- **Los ids citados en la prosa de un spec cuentan como criterios**
  (`spec.Lint` es así). El medidor les da un segmento y Qué los muestra como
  "criterio citado en el spec". Cambiar esa regla es de otra spec.
- **El JSON de `hoom board` crece.** El medidor suma un segmento por criterio:
  una tarjeta con 40 criterios trae 45 segmentos.
- **El normal muestra el spec tal como está escrito**, y un spec puede hablar
  de git. El normal esconde el git de la página, no el del documento que la
  persona aprueba.
- **"Interrumpido" sigue infiriéndose por silencio antes del run** (brecha 4,
  C3). El filtro puede mostrar como interrumpido un paso verify de más de
  15 minutos.
