# Spec: acciones desde la tarjeta (cabina visual, C3)

Estado: BORRADOR — pendiente de aprobación humana.
Depende de: `tablero-de-solo-lectura.md` (C2) integrada (PR #23).
Tarea sugerida: `hoom task start acciones-desde-la-tarjeta`.
Origen: `.hoom/intake/rfc-cabina-visual-v2.md` (spec 3 de 4, "C3 — Acciones
desde la tarjeta") y el pedido de Henry del 2026-09-23, que fija las siete
acciones, la regla de oro, el diálogo de confirmación, el fantasma, el writer
declarado y el espejo de solo lectura de la terminal.
Re-expresa: CA-319 (C2), por el archivo nuevo de la UI.

## Objetivo

Que la cabina actúe sin dejar de ser una proyección de la evidencia. La
tarjeta gana botones y se puede arrastrar, y cada acción llama a la misma
función que su verbo de la CLI, con el modelo de POST con token que el Studio
ya tiene.

**Regla de oro.** No existe un endpoint para mover una tarjeta ni para
escribir su columna. La columna sigue siendo la función de la evidencia de C1:
una acción produce evidencia (un spec, tests, un veredicto, una firma, un
cierre) y la columna la recalcula `boardcmd.Derive`. Un test afirma que
ninguna ruta del Studio recibe una columna como entrada.

Siete piezas:

1. **Pedir al rol** (arquitecto, test-writer, writer, reviewer): lanza el
   sobre (`agentcmd`, lo mismo que `hoom agent --role <rol> --task <slug>`) o
   la review (`reviewcmd`, lo mismo que `hoom review --task <slug> --spec
   ...`) después de un diálogo que confirma rol, provider, modelo,
   presupuesto y pedido. Mientras corre, un **fantasma** de la tarjeta aparece
   en la columna destino con el paso del sobre. Arrastrar es un atajo al mismo
   diálogo.
2. **Aprobar spec** en Tu aprobación: el `POST /api/specs/{name}/approve` que
   ya existe, resuelto en el espacio de trabajo de la tarjeta.
3. **Integrar** en Tu aceptación: el `POST /api/tasks/{slug}/done` que ya
   existe, que además escribe `hecho_en` y `commit_final` en el item (C1).
4. **Reanudar** y **Volver a lanzar** una tarjeta interrumpida, siempre con un
   sobre nuevo. En modo experto, **Descartar cambios** del run interrumpido.
   El registro del sobre gana el PID de su dueño (brecha 4 del RFC), así que
   "interrumpido" pasa a ser un hecho.
5. **Guardar en git**: commitea lo que la tarjeta tiene sin guardar, con un
   mensaje fijo.
6. En modo experto, **Abrir sesión** (el CLI interactivo en el espacio de
   trabajo de la tarjeta, con el tmux de `hoom cockpit`), que deja en el item
   un **writer declarado** para `hoom review`, y **Ver terminal**, el espejo de
   solo lectura de ese pane.
7. **Costo**: la tarjeta compara gasto con presupuesto, y el servidor se niega
   a lanzar cuando lo que queda no alcanza.

La tarjeta sabe qué acciones valen: la lista la calcula `Derive` y la emiten
`hoom board --json`, `hoom item show --json` y `/api/board`. La página solo
muestra lo que el binario le da, y cada endpoint vuelve a derivar la tarjeta
antes de actuar.

## No-goals

- **Mover una tarjeta o escribir su columna.** No hay endpoint, verbo ni
  campo del item que lo haga (la regla de oro).
- **Escribir en la terminal desde el navegador** (`tmux send-keys`). El token
  del Studio pasaría a dar un shell: es otro modelo de riesgo y le toca a otra
  spec. En C3 el espejo es de solo lectura.
- **Verificar de nuevo desde la tarjeta.** El RFC lo ponía en C3 y el pedido
  no. El sobre del writer ya verifica al cerrar, y `POST /api/verify` verifica
  el árbol del proyecto, no el espacio de trabajo de la tarjeta.
- **Publicar (`git push`) e integrar con `git merge`.** Guardar en git
  commitea y nada más. Integrar es `hoom task done`, que deja la rama lista y
  no la junta.
- **Cancelar un rol en curso desde la tarjeta.**
- **Revisar con el mismo provider que escribió** (`--same-provider`). Desde la
  cabina la review es cruzada o no se lanza. La terminal conserva el flag.
- **Abrir la sesión con zellij desde el Studio.** zellij necesita una
  terminal para arrancar. `hoom cockpit --mux zellij` sigue igual.
- **Pedirle trabajo a una tarjeta sin espacio de trabajo propio de hoom**
  fuera de Backlog. Atar los espacios de trabajo de otras herramientas (Orca)
  es la brecha 6 de C1.
- **Descartar la cuarentena de un test-writer ciego** (`.hoom/isolated/`) y
  **descartar un item**. Son preguntas abiertas del RFC.
- **El piloto automático, el timeline, el replay y el doctor** (C4 y
  después) y **la curva del trinquete**.
- **Dependencias nuevas.** `go.mod` y `go.sum` no cambian, y la UI sigue sin
  frameworks ni assets de red.

## Contratos

### La regla de oro

- Ningún patrón de ruta del Studio contiene `column`, `columna`, `move` ni
  `mover`. `Handler` registra sus rutas con un solo ayudante, y
  `servecmd.Routes()` devuelve la lista de patrones registrados, en orden, para
  que el test la recorra.
- Ningún struct del paquete `servecmd` tiene una etiqueta json `column`,
  `columna`, `to`, `from`, `destino` ni `destination`. El test lo afirma
  leyendo el código del paquete con `go/parser`.
- Los endpoints nuevos decodifican su cuerpo en modo estricto
  (`DisallowUnknownFields`): una clave desconocida es 400 y no tiene efectos.
- Un POST a cualquier ruta del Studio con `{"column": "hecho", "columna":
  "hecho"}` y el token deja la columna de la tarjeta igual.
- El item sigue sin aceptar una clave de columna o de estado (C1).

### Lo que la tarjeta suma

Todo se deriva en `Derive`, que sigue siendo pura. Las claves de C1 y C2 no
cambian, las nuevas son aditivas y ninguna lista es `null`. El texto de
`hoom board` y de `hoom item show` no cambia.

```json
{
  "...": "todas las claves de C1 y C2",
  "actions": [
    {"id": "pedir-writer", "role": "writer", "label": "Pedir al writer",
     "expert": false, "enabled": true, "why": "",
     "pedido": "Implementa .hoom/specs/S.md ...", "budget_usd": 3.2,
     "providers": [
       {"name": "claude", "ok": true, "why": "", "budget": true, "min_budget_usd": 0.5, "default": true},
       {"name": "codex", "ok": true, "why": "", "budget": false, "min_budget_usd": 0, "default": false}],
     "paths": [], "resume_id": "", "signer": ""}
  ],
  "drops": [{"column": "review", "action": "pedir-writer", "why": ""},
            {"column": "backlog", "action": "", "why": "la tarjeta no vuelve atras: su columna la da la evidencia"}],
  "ghost": {"column": "review", "role": "writer", "provider": "claude",
            "envelope_id": "...", "run_id": "...", "stage": "run", "step": 3, "steps": 5},
  "spend": {"...": "C1", "remaining_usd": 3.2},
  "providers": {"...": "C2", "declared": ["claude"]}
}
```

`Evidence` gana `Providers []providers.Info` (lo que detecta
`providers.Detect()`, una vez por `Build`) y `Signer string` (la identidad git
del árbol de evidencia, `gitx.Identity`, leída una vez por árbol).

#### `actions`: qué se puede hacer con la tarjeta

Visibles según la columna, en este orden (la primera de la columna es la
**principal**, la que dispara el arrastre):

| columna | acciones de la columna |
|---|---|
| `backlog` | `pedir-arquitecto` |
| `arquitecto` | `pedir-arquitecto` |
| `tu-aprobacion` | `aprobar`, `pedir-arquitecto` |
| `test-writer` | `pedir-test-writer` |
| `writer` | `pedir-writer` |
| `review` | `pedir-reviewer` si la review de 4 lentes es exigida y no hay registro que cuente; `pedir-writer` si hay hallazgos que bloquean |
| `tu-aceptacion` | `integrar` |
| `hecho` | ninguna |

Después, en cualquier columna:

- `reanudar` y `relanzar`, si hay `interrupted`. Su rol es el del sobre
  interrumpido.
- `descartar` (experto), si hay `interrupted`, la tarjeta tiene espacio de
  trabajo propio y ahí hay cambios sin guardar fuera de `.hoom/`.
- `guardar`, si `unsynced` no está vacío.
- `sesion` y `terminal` (experto), si la tarjeta tiene espacio de trabajo
  propio.

Las etiquetas (`label`, sin acentos): `Pedir al arquitecto`, `Pedir al
test-writer`, `Pedir al writer`, `Pedir al reviewer`, `Aprobar spec`,
`Integrar`, `Reanudar`, `Volver a lanzar`, `Descartar cambios`, `Guardar`,
`Abrir sesion`, `Ver terminal`. `expert` es `true` solo en `descartar`,
`sesion` y `terminal`.

- **`paths`**: en `guardar`, `unsynced`. En `descartar`, las rutas de
  `unsynced` que están dentro del espacio de trabajo y fuera de su `.hoom/`.
  En las demás, `[]`.
- **`signer`**: solo en `aprobar`, la identidad git con la que se va a
  firmar.
- **`resume_id`**: solo en `reanudar`, el id de sesión del provider guardado
  en el sidecar del run del sobre interrumpido.

#### `enabled` y `why`

`why` es `""` cuando `enabled` es `true`. Si no, es una frase con las mismas
reglas que `plain` de C2: nunca vacía, sin acentos y sin las palabras `git`,
`commit`, `worktree`, `merge`, `rama`, `branch`, `diff`, `HEAD` ni `huella`,
ni `.hoom/` ni `hoom `. Se evalúa en este orden, y gana el primer caso:

| acciones | caso | `why` |
|---|---|---|
| todas menos `sesion` y `terminal` | hay `running` | `espera a que termine el <rol> que esta trabajando` (sin rol: `el agente`) |
| de rol (`pedir-*`, `reanudar`, `relanzar`) | sin espacio de trabajo propio y la columna no es `backlog` | `la tarjeta no tiene su propio espacio de trabajo: desde el tablero solo se le pide trabajo a una tarjeta que lo tiene` |
| de rol | el item tiene presupuesto y `remaining_usd <= 0` | `se agoto el presupuesto de la tarjeta: se gastaron <g> de <p> USD` |
| de rol | ningún provider con `ok` | `ningun proveedor instalado puede tomar este trabajo` |
| `reanudar` | el sobre murió antes del paso run (sin `run_id`) | `el trabajo se corto antes de que el agente empezara: solo se puede volver a lanzar` |
| `reanudar` | el sidecar del run no tiene id de sesión | `el agente no dejo una sesion que retomar: solo se puede volver a lanzar` |
| `reanudar` | el sobre era ciego (`isolated`) | `el <rol> trabaja a ciegas y su espacio ciego ya no existe: solo se puede volver a lanzar` |
| `reanudar` | el rol es `reviewer` | `una revision cortada se vuelve a lanzar entera` |
| `reanudar` | el provider no tiene la capacidad `resume` | `<provider> no puede retomar una sesion: solo se puede volver a lanzar` |

Los montos se escriben como en `providers.Usage` (sin ceros de cola).

#### `providers` de una acción de rol, y el presupuesto

- Una opción por provider **instalado**, en el orden del registro. `budget`
  es su capacidad `budget` y `min_budget_usd` su mínimo (ver "El mínimo del
  provider").
- `ok` es `false`, con su `why`, cuando: no tiene la capacidad
  `system_prompt` (`<p> no puede recibir el contrato del rol`); el item tiene
  presupuesto y lo que queda para un run (`remaining_usd`, o `remaining_usd /
  4` en la review) es menor que `min_budget_usd` (`quedan <r> USD y <p>
  necesita al menos <m> USD por run`, con `<r>` lo que queda para un run); en
  `pedir-reviewer` y en `relanzar` de
  un reviewer, es un writer de la tarjeta (`providers.writer` o uno de
  `providers.declared`: `<p> escribio el codigo: la revision tiene que ser de
  otro proveedor`).
- En `reanudar` la única opción es el provider del sobre interrumpido: la
  sesión es suya.
- `default` es `true` en la primera opción `ok`. En `relanzar`, en el
  provider del sobre interrumpido si está `ok`.
- **`spend.remaining_usd`** = `presupuesto_usd` del item − `spend.cost_usd`
  (un costo `null` cuenta 0). Puede ser negativo. `null` sin presupuesto.
- **`budget_usd`** de la acción, el valor que propone el diálogo: sin
  presupuesto en el item, `null` (sin tope). Con presupuesto, `remaining_usd`,
  y en la review `remaining_usd / 4`, porque su presupuesto es por lente y una
  review exigida tiene cuatro.

#### `pedido` por defecto

La propuesta que el diálogo muestra y la persona puede editar:

- `pedir-arquitecto`: `Escribi el spec .hoom/specs/<slug>.md de la tarjeta
  "<titulo>".` y, si el item tiene `pedido`, un salto de línea y `Pedido:
  <pedido>`.
- `pedir-test-writer`: `Escribi los tests que citan los criterios de
  .hoom/specs/<slug>.md que todavia no tienen prueba: <CA-x, CA-y>.`
- `pedir-writer`: `Implementa .hoom/specs/<slug>.md hasta que hoom verify
  --spec .hoom/specs/<slug>.md de verde.` En Review, con hallazgos que
  bloquean: `Corregi los hallazgos que bloquean la tarjeta: <id1>, <id2>.`
- `pedir-reviewer`: `""`. La review arma su propio pedido.
- `reanudar`: `El trabajo anterior se corto en el paso <X> de <N> (<stage>).
  Revisa el arbol como quedo y termina lo que se te pidio.`
- `relanzar`: el pedido de `pedir-<rol>` para los cuatro roles, y `""` para
  otro rol.

#### `drops`: soltar en otra columna

Una entrada por cada columna que no es la de la tarjeta, en el orden fijo:

- La **siguiente** columna: si la tarjeta tiene acción principal, `action` es
  su id y `why` es `""`. Soltar ahí abre el diálogo de esa acción (y si está
  deshabilitada, muestra su `why`). Sin acción principal, `action` es `""` y
  `why` es `plain`.
- Una columna **anterior**: `la tarjeta no vuelve atras: su columna la da la
  evidencia`.
- Una columna **más allá** de la siguiente: `primero tiene que llegar a
  <Siguiente>: <plain>`, con el `name` de la columna siguiente.

#### `ghost`: el fantasma en la columna destino

Cada columna de rol tiene sus **roles de estación**: `backlog` y `arquitecto`
{arquitecto}, `test-writer` {test-writer}, `writer` {writer} y `review`
{reviewer, writer}. Las demás no tienen.

- Si hay `running` y su rol es un rol de estación de la columna de la tarjeta,
  `ghost` es `{column: <la siguiente>, role, provider, envelope_id, run_id,
  stage, step, steps}`, copiados de `running`. Si no, es `null`.
- La tarjeta sigue sólida en su columna, con su "en curso" de C2.
- Cuando el sobre cierra, `running` deja de existir y el fantasma con él. La
  columna la vuelve a calcular la evidencia: si el rol no la produjo, la
  tarjeta sigue donde estaba y dice por qué con `red.plain` (el sobre falló) o
  con `plain` (lo que todavía falta, por ejemplo un spec que no pasa lint).

#### Interrumpido: el PID del dueño y el relevo

- `envelope.Record` gana `pid` (el PID del proceso dueño del sobre). Lo
  escriben `agentcmd` y `reviewcmd` desde el primer registro. Un sobre que
  lanza el Studio tiene el PID de `hoom serve`.
- `Gather` resuelve la vida de un sobre abierto así: con `pid` mayor que 0,
  vive si y solo si ese proceso vive (`runcmd.Alive`), sin mirar el latido.
  Sin `pid` (registros viejos), la regla de C1 no cambia.
- **Relevo.** Un sobre abierto sin dueño vivo deja de ser `interrupted` si la
  tarjeta tiene otro sobre que empezó después de su último registro
  (`started_at` posterior a su `updated_at`). Reanudar, volver a lanzar o
  pedir de nuevo relevan al sobre muerto. hoom nunca lo cierra ni lo reescribe:
  su registro queda como está.

### El sobre y la review

- `agentcmd.Options` gana `EnvelopeID string` (vacío = uno nuevo) y `Started
  func()`, que se llama una vez, cuando el primer registro ya está en disco.
  Un error antes de ese registro (rol desconocido, pedido vacío, provider sin
  `system_prompt`) llega sin que `Started` se llame.
- `hoom review` deja **registro de sobre**, como `hoom agent`: rol
  `reviewer`, su provider, `task`, `dir`, `spec` y `pid`. `steps` es la
  cantidad de lentes, y cada pasada es el paso `run` con `step` = su número y
  `run_id` = su run. Cierra `entregable` (revisado, `stage: "ok"`) o
  `no-entregable` con `stage` y nota (`run`: `el run del reviewer fallo`;
  `scope`: `el reviewer escribio fuera de su territorio`). Lo que se decide
  antes de la primera pasada (sin lentes, review no cruzada, sin provider) no
  escribe registro. `usage` queda vacío: el gasto de cada pasada ya está en su
  sidecar.
- `reviewcmd.Options` gana `EnvelopeID` y `Started`, con el mismo contrato.

### El mínimo del provider

- `providers.Info` gana `min_budget_usd`: lo que declara el adapter con una
  interfaz opcional `BudgetFloor` (`MinBudgetUSD() float64`), y 0 si no la
  implementa. `hoom providers --json` y `/api/providers` lo emiten.
- **Claude: 0.5 USD.** Con `--max-budget-usd` Claude corta después del turno
  que pasa el tope, y en el dogfood un turno de un rol con su contrato cuesta
  del orden de 0.1 USD. Con menos de 0.5 USD el run se corta antes de
  entregar algo y el gasto se pierde. Pendiente de confirmar al aprobar.
- Un provider sin la capacidad `budget` (Codex, gemini, opencode) declara 0:
  no acepta tope, así que el mínimo no aplica. Con él, lo único que frena es
  un presupuesto agotado.

### Endpoints

| método y ruta | cuerpo | acción | respuestas |
|---|---|---|---|
| `POST /api/board/{slug}/launch` | `{action, provider, model, budget_usd, pedido}` | `pedir-*`, `reanudar`, `relanzar` | 202, 400, 401, 404, 409 |
| `POST /api/specs/{name}/approve` | opcional `{card}` | `aprobar` | 200, 400, 401, 404, 409 |
| `POST /api/tasks/{slug}/done` | — | `integrar` | 200, 401, 409 (sin cambios de hoy) |
| `POST /api/board/{slug}/save` | `{paths}` | `guardar` | 200, 400, 401, 404, 409 |
| `POST /api/board/{slug}/discard` | `{paths}` | `descartar` | 200, 400, 401, 404, 409 |
| `POST /api/board/{slug}/session` | `{provider}` | `sesion` | 200, 400, 401, 404, 409 |
| `GET /api/board/{slug}/terminal` | — | `terminal` | 200, 400, 404 |

Todo POST exige el token (sin él, 401 y ningún efecto). Un slug con forma
inválida es 400, y un item inexistente o inválido es 404 con el mensaje de
`CardFor`. Los errores son JSON `{"error": ...}`, como hoy. `launch`,
`approve` con `card`, `discard` y `session` **vuelven a derivar la tarjeta**
antes de actuar: si su acción ya no está en `actions`, es 409 con `la tarjeta
ya no esta donde la viste: ahora esta en <Columna> (<plain>)`, y si está
deshabilitada, 409 con su `why`. `save` y `done` dejan las condiciones a su
función (la de `hoom item save` y la de `hoom task done`).

#### `POST /api/board/{slug}/launch`

Cuerpo estricto: `action` (`pedir-arquitecto`, `pedir-test-writer`,
`pedir-writer`, `pedir-reviewer`, `reanudar` o `relanzar`), `provider`,
`model` (opcional), `budget_usd` (número o `null`) y `pedido`. Se valida en
este orden:

1. Token (401). Slug y cuerpo: clave desconocida, tipo inválido o `action`
   desconocida (400).
2. Item (404) y tarjeta: la acción está y está habilitada (409).
3. `provider`: una de las opciones de la acción (400 si no está) y `ok` (409
   con su `why`).
4. `pedido`: obligatorio y no de relleno (`agentcmd.IsPlaceholder`) en toda
   acción menos `pedir-reviewer` y el `relanzar` de un reviewer, que no lo
   aceptan (400).
5. `budget_usd`. Con un provider sin tope: tiene que ser `null` (400: `<p> no
   acepta tope de presupuesto: deja el presupuesto vacio`). Con tope: `null`
   toma el `budget_usd` de la acción (sin presupuesto en el item, sin tope);
   un número tiene que ser mayor que 0 (400), al menos el mínimo del provider
   (409) y, si el item tiene presupuesto, no más que lo que queda (409), o su
   cuádruple no más que lo que queda en la review.
6. Un solo lanzamiento por tarjeta a la vez en el Studio (409: `ya se esta
   lanzando un trabajo en esta tarjeta`), y el árbol de la tarjeta sin run
   activo de ningún proceso (`runcmd.Manager.Busy`, 409 que nombra el run).
7. Sin espacio de trabajo propio y en Backlog: `taskcmd.Start` (el mismo
   `hoom task start <slug>`). Si falla, 409 con su mensaje.
8. Lanzamiento, en una goroutine del Studio:
   - `pedir-arquitecto`, `pedir-test-writer`, `pedir-writer`, y `reanudar` y
     `relanzar` de esos roles o de otro que no sea reviewer: `agentcmd.Run`
     con `Role`, `Provider`, `Model`, `BudgetUSD`, `Prompt` = `pedido`,
     `Task` = el slug y `Spec` = `.hoom/specs/<slug>.md` salvo para los roles
     que escriben specs. `reanudar` suma `ResumeID` = `resume_id`.
   - `pedir-reviewer` y `relanzar` de un reviewer: `reviewcmd.Run` con `Task`,
     `Spec`, `Provider`, `Model` y `BudgetUSD`, sin `SameProvider`.

   La salida de texto va a `.hoom/envelopes/<envelope_id>.log`. El endpoint
   responde **202** `{"envelope_id", "role", "provider", "task_started"}`
   cuando `Started` se llamó. Si la función vuelve antes (con error, o una
   review que se decidió sin pasadas), responde 409 con el error o con la
   nota del resultado, y no queda registro.

#### Aprobar

`POST /api/specs/{name}/approve` acepta un cuerpo opcional y estricto
`{"card": "<slug>"}`. Sin cuerpo hace lo de hoy. Con `card`:

- `name` tiene que ser el slug (400).
- La tarjeta tiene que tener `aprobar` habilitada (409 como arriba).
- El spec se resuelve en el árbol de evidencia de la tarjeta (su espacio de
  trabajo si existe) y se llama `approval.Approve(<arbol>, <spec>)`: lo mismo
  que `hoom spec approve .hoom/specs/<slug>.md` corrido en ese árbol. La
  aprobación queda en su `.hoom/approvals/`, y `approved_by` es
  `gitx.Identity(<arbol>)`: la identidad git de quien corre `hoom serve`,
  nunca `hoom` ni un agente. El cuerpo no acepta un aprobador.
- La respuesta es la de hoy: `{"approval", "already"}`.

#### Integrar

`POST /api/tasks/{slug}/done` no cambia: es `taskcmd.Done` sin `--force`,
su rechazo viaja tal cual (409) y, si cierra, escribe `hecho_en` y
`commit_final` en el item del árbol raíz (C1). La tarjeta solo ofrece
`integrar` en Tu aceptación. El endpoint no le suma condiciones al verbo.

#### Guardar en git: `hoom item save <slug> [--json]`

`itemcmd.Save(root, base, blockOn, slug string, expect []string)
(SaveResult, error)` commitea exactamente `unsynced` de la tarjeta:

- Un commit por árbol, con el mensaje fijo `hoom: guardar la tarjeta <slug>`
  y la identidad git configurada (sin `--author`). En el espacio de trabajo
  de la tarjeta van sus rutas, y en el árbol raíz, el item. Se hace `git add
  -A -- <rutas>` y `git commit -m <mensaje> -- <rutas>`, así que lo que otro
  dejó en el índice del árbol raíz no entra ni se pierde. Los hooks del repo
  corren.
- Con `running` se niega (`espera a que termine el <rol> que esta
  trabajando`). Sin nada que guardar no commitea y lo dice.
- `expect` no `nil` tiene que ser igual (como conjunto) a `unsynced`. Si no,
  se niega: `los cambios de la tarjeta cambiaron desde que los viste:
  revisalos de nuevo`. El Studio siempre lo pasa y la CLI nunca.

```json
{"slug": "S", "message": "hoom: guardar la tarjeta S",
 "commits": [{"dir": ".hoom/worktrees/S", "sha": "<40>", "paths": ["..."]},
             {"dir": ".", "sha": "<40>", "paths": [".hoom/items/S.yaml"]}]}
```

`POST /api/board/{slug}/save` con `{"paths": [...]}` (obligatorio) llama a
la misma función: 200 con ese JSON; 409 con `running` o con otras rutas; 409
con la salida de git si el commit falla. El texto de la CLI:

```
hoom item save: tarjeta S guardada
  .hoom/worktrees/S: <sha12> (3 archivos)
  .: <sha12> (1 archivo)
```

Sin nada que guardar: `hoom item save: la tarjeta S no tiene nada sin
guardar`, exit 0.

#### Descartar: `hoom task discard <slug> [--yes] [--json]`

`taskcmd.Discard(root, slug string, expect []string, yes bool)
(DiscardResult, error)` sobre el espacio de trabajo de la tarea:

- Las rutas son las sin guardar del espacio de trabajo **fuera de `.hoom/`**.
  hoom nunca descarta evidencia: veredictos, hallazgos, aprobaciones y
  registros son de solo agregar.
- Una ruta que existe en `HEAD` vuelve a como está ahí (`git restore
  --source=HEAD --staged --worktree`). Una que `HEAD` no tiene se borra.
- Se niega si la tarea no existe o si su árbol tiene un run activo
  (`Busy`).
- Sin `yes` no toca nada: la CLI lista las rutas y termina con exit 1 y
  `Accion: repeti con --yes para descartarlos (no se puede deshacer)`. El
  Studio pasa `yes` y `expect`, que tiene que ser igual al conjunto actual
  (si no, la misma negativa que Guardar).
- `{"restored": [...], "removed": [...]}`, rutas relativas a la raíz.

`POST /api/board/{slug}/discard` con `{"paths": [...]}` exige además la
acción `descartar` habilitada en la tarjeta.

#### Abrir sesión: `cockpitcmd.Open`

- `cockpitcmd.Open(root, project string, opt Options, deps Deps) (Session,
  error)` arma la sesión de tmux del cockpit (el mismo plan de CA-82 y CA-85:
  el CLI de la IA a la izquierda, `hoom status --watch` a la derecha, las dos
  en el espacio de trabajo) **sin adjuntarse**, y dice si la creó o ya
  existía. `Run` pasa a ser `Open` más el `attach` o `switch-client` de hoy
  (tmux). zellij no pasa por `Open`.
- `cockpitcmd.SessionName(project, task string) string` es el nombre de
  siempre: `hoom-<proyecto>[-<tarea>]`.
- Cuando `Open` **crea** la sesión de una tarea que tiene item en el árbol
  raíz, agrega una entrada a `sesiones` del item (`item.AddSession`). Si la
  sesión ya existía, no registra nada. Lo hace igual desde la CLI (`hoom
  cockpit --task <slug>` con tmux) y desde el Studio.
- `POST /api/board/{slug}/session` con `{"provider": "<p>"}` (obligatorio:
  desde el Studio no se autodetecta) exige `sesion` en la tarjeta y responde
  `{"session", "created", "provider", "dir", "attach": "tmux attach -t
  <session>"}`. Sin tmux o sin el provider instalado: 409 con el mensaje de
  `cockpitcmd`.

El item gana la clave `sesiones`, la única que se agrega:

```yaml
sesiones:
  - provider: claude
    abierta_por: Henry Orellana <henry@...>
    abierta_en: 2026-09-23T18:00:00Z
```

Cada entrada exige `provider` y `abierta_en`. `abierta_por` es
`gitx.Identity(root)`. Un item sin `sesiones` sigue siendo válido y se
escribe igual que hoy. El piso del sobre ya prohíbe que un rol toque
`.hoom/items/`, así que un agente no puede declararse writer.

#### Ver terminal: `GET /api/board/{slug}/terminal`

`cockpitcmd.Capture(root, project, slug string, deps Deps) (Terminal, error)`:

```json
{"available": true, "session": "hoom-hoomai-S", "pane": "%3",
 "text": "<la pantalla, con sus secuencias de color>", "note": ""}
```

- El pane es el primero que lista `tmux list-panes -t =<session> -F
  '#{pane_id}'` (el del CLI de la IA, que el cockpit crea primero), y `text`
  es `tmux capture-pane -p -e -t <pane>`, cortado en 256 KiB.
- Sin tmux, o sin la sesión: `available: false`, `text: ""` y la nota (`tmux
  no esta instalado` o `no hay una sesion abierta para esta tarjeta`).
- Es una lectura: sin token, como las demás, y no crea ni modifica nada. Otro
  método es 405.

### El writer declarado en `hoom review`

- `reviewcmd.Run` lee las `sesiones` del item de la tarea en el árbol raíz.
  `Result` y el registro de review ganan `writers_declared`: los providers
  distintos de esas sesiones, ordenados, `[]` sin sesiones.
- El writer **observado** sigue saliendo del sidecar (`writerOf`), y el
  declarado nunca lo reemplaza. `cross`:
  - `no-cruzada` si el reviewer es el writer observado **o** uno declarado.
    Sin `--same-provider`, la review se niega como hoy.
  - `cruzada` si hay writer observado y el reviewer no es ninguno de los dos.
  - **`cruzada-declarada`** si no hay writer observado, hay declarados y el
    reviewer no es ninguno. Nunca `cruzada`: nadie lo vio escribir.
  - `desconocida` sin ninguno de los dos.
- Sin `--provider`, la review elige el primer provider que no sea ni el
  observado ni un declarado.
- La tarjeta: `providers.declared` es la lista de providers de `sesiones`,
  distintos y en orden de aparición. `writer`, `reviewer` y `cross` siguen la
  regla de C2, y `cross` puede traer `cruzada-declarada` desde el registro.

### La UI

- **`internal/servecmd/ui/acciones.js`**, archivo nuevo embebido que
  `index.html` carga con `<script src="acciones.js"></script>` después de
  `tablero.js`. Todo POST de la cabina está ahí y pasa por `act(` (el token de
  siempre). `tablero.js` sigue siendo el pintor sin POST de C2 (CA-320 no
  cambia) y le avisa a `acciones.js` cada vez que pinta, con
  `document.dispatchEvent(new Event("tablero:pintado"))`.
- **La tarjeta.** `tablero.js` pinta cada tarjeta dentro de un contenedor
  `.tcard-wrap` con `data-slug`, con el botón de la tarjeta y un lugar vacío
  `.tacc`. `acciones.js` pone en `.tacc` un botón por acción de `actions` (las
  de `expert` solo en modo experto), deshabilitado con su `why` como título si
  `enabled` es `false`. Un clic en una acción no abre el detalle.
- **El fantasma.** `tablero.js` pinta, en la columna de `ghost.column`, una
  tarjeta tenue con el título, la marca del provider, el rol y `paso X de N:
  <stage>`. No es arrastrable ni tiene acciones.
- **Arrastrar.** `acciones.js` hace arrastrable cada `.tcard-wrap`. Al
  soltar en una columna busca su entrada en `drops`: con `action`, abre el
  diálogo de esa acción (o muestra su `why` si está deshabilitada); sin
  `action`, muestra el `why` en la columna. Soltar en la propia columna no
  hace nada.
- **El diálogo de rol** (`pedir-*`, `reanudar`, `relanzar`): rol (fijo),
  provider (las opciones de la acción; las que no están `ok`, deshabilitadas
  con su `why`), modelo (texto, opcional), presupuesto en USD (con el
  `budget_usd` propuesto) y pedido (con el `pedido` propuesto; no aparece en
  la review). En `reanudar` el provider está fijo y el diálogo nombra la
  sesión que se retoma, y si la tarjeta tiene cambios sin guardar avisa que
  el trabajo nuevo empieza desde el árbol como quedó. Con un provider sin
  `budget`, el presupuesto se deshabilita y el diálogo dice `<Nombre> no
  acepta un tope de presupuesto ni reporta costo: este trabajo no tiene tope
  en USD y su gasto no se descuenta del presupuesto de la tarjeta.` Confirmar
  hace el POST. Un error se muestra en el diálogo, tal como lo manda el
  servidor, y el diálogo queda abierto.
- **Aprobar**: `Vas a aprobar el spec de "<título>" (<n> criterios) con tu
  identidad: <signer>. Si el spec cambia después, la aprobación deja de
  valer.`, un enlace al detalle (pestaña Qué) y Confirmar
  (`act("/api/specs/<slug>/approve", {card: <slug>})`).
- **Integrar**: en normal, `Se cierra el trabajo de "<título>": hoom
  comprueba una última vez que todo esté verde y guardado, y la tarjeta pasa
  a Hecho. Juntarlo con el proyecto lo hacés vos después.` En experto suma
  `git merge --no-ff hoom/<slug>`.
- **Guardar**: en normal, `Guardar <n> cambios de esta tarjeta`. En experto,
  las rutas y el mensaje del commit. Si hay `interrupted`, avisa que lo que el
  trabajo cortado dejó a medias también se guarda.
- **Descartar** (experto): las rutas, `no se puede deshacer` y Confirmar.
- **Abrir sesión** (experto): el provider (los instalados) y, después, el
  comando para adjuntarse y un botón que abre Ver terminal.
- **Ver terminal** (experto): abre el detalle en Quién, con una sección
  **Terminal** que pide `/api/board/{slug}/terminal` 1000 ms después de cada
  respuesta mientras está abierta (sondeo de C2), y pinta `text` convirtiendo
  los colores básicos, el brillante y la negrita de las secuencias SGR en
  `<span>` y descartando las demás secuencias. Es solo de lectura: la sección
  no tiene campo de texto ni manda teclas.
- **La página no decide.** `acciones.js` no tiene tabla de columnas ni de
  reglas: muestra lo que traen `actions`, `drops` y `ghost`. No contiene
  ningún id de columna como cadena (`"backlog"`, `"arquitecto"`,
  `"tu-aprobacion"`, `"test-writer"`, `"writer"`, `"review"`,
  `"tu-aceptacion"`, `"hecho"`).
- **Vocabulario.** Las etiquetas de las acciones, con acentos, en una tabla
  por id (`Pedir al arquitecto`, `Aprobar spec`, `Integrar`, `Reanudar`,
  `Volver a lanzar`, `Descartar cambios`, `Guardar` en normal y `Guardar en
  git` en experto, `Abrir sesión`, `Ver terminal`). Los textos del modo normal
  viven entre `/* vocabulario normal */` y `/* fin vocabulario normal */`,
  sin las palabras de git de C2.
- CSS en el `<style>` de `index.html`, con las variables de siempre.

### Documentación

- README: la sección del Studio describe las acciones de la tarjeta, sus
  endpoints, el fantasma, el writer declarado y `hoom item save` y `hoom task
  discard`. El roadmap de la cabina pierde el ítem "Acciones desde la
  tarjeta". La tabla de verbos suma los dos verbos nuevos.

## Casos límite y errores esperados

- Una tarjeta en Backlog sin espacio de trabajo: Pedir al arquitecto crea la
  tarea y lanza el sobre ahí. Si el sobre falla antes de su primer registro,
  la tarea queda creada y el 409 lo dice.
- Una tarjeta en Test-writer sin espacio de trabajo propio (su evidencia vive
  en el proyecto): `pedir-test-writer` aparece deshabilitada con su `why`.
- Dos pestañas del navegador piden al writer a la vez: una recibe 202 y la
  otra 409.
- Un `hoom agent` de una terminal trabaja en el mismo árbol: la tarjeta está
  en curso y sus acciones de rol, deshabilitadas. Si el run es de otra tarea
  en el mismo árbol, el POST responde 409 nombrando el run.
- `hoom serve` se cae con un sobre en curso: su registro queda abierto con el
  PID de un proceso muerto, y la tarjeta se ve interrumpida en el próximo
  sondeo, sin esperar 15 minutos.
- Reanudar un writer cuyo sidecar tiene `provider_session_id`: sobre nuevo con
  `--resume` de esa sesión. El registro viejo queda byte a byte igual y deja
  de contar como interrumpido.
- Reanudar un test-writer ciego, un reviewer o un sobre muerto antes del run:
  deshabilitado con su motivo. Volver a lanzar sí se puede.
- Presupuesto de 5 USD con 4.8 gastados y Claude: Claude no es `ok` (quedan
  0.2 y necesita 0.5). Codex sí, con el aviso de que no hay tope.
- Presupuesto gastado de más (6 de 5 USD): `remaining_usd` es -1 y todas las
  acciones de rol dicen que se agotó.
- Un item sin presupuesto y Claude: el diálogo propone sin tope. Si la persona
  pone 0.2, es 409 por el mínimo.
- Codex con `budget_usd: 2`: 400, y no corre nada.
- Una review cuyo único provider distinto del writer no está instalado: la
  acción está deshabilitada (`ningun proveedor instalado puede tomar este
  trabajo`), y cada opción dice por qué no.
- Soltar una tarjeta de Writer en Hecho: `primero tiene que llegar a Review:
  <plain>`. Soltarla en Arquitecto: `la tarjeta no vuelve atras: su columna la
  da la evidencia`.
- Aprobar con `{"card": "otra"}` en `/api/specs/S/approve`: 400. Aprobar una
  tarjeta que está en Arquitecto: 409 con el motivo.
- Integrar una tarjeta en Writer con el POST de done: 409 con el mensaje de
  `task done`, y la tarjeta y el item quedan igual.
- Guardar cuando alguien agregó un veredicto después de abrir el diálogo: 409,
  nada commiteado.
- Guardar con un archivo propio de la persona en el índice del árbol raíz: el
  commit lleva solo el item, y el archivo sigue en el índice.
- Un hook de pre-commit que falla: 409 con su salida, y las rutas quedan en el
  índice.
- Descartar con un veredicto sin commitear en el espacio de trabajo: el
  veredicto no se toca.
- Abrir sesión dos veces: la segunda responde `created: false` y el item
  tiene una sola entrada.
- Ver terminal sin sesión o sin tmux: `available: false` con la nota. La
  sección lo dice y sigue sondeando mientras está abierta.
- Un item escrito por esta versión con `sesiones`, leído por un `hoom` viejo:
  inválido por clave desconocida (ver Riesgos).
- `POST /api/board/S/launch` con `{"action": "pedir-writer", "column":
  "review"}`: 400 por clave desconocida.

## Criterios de aceptación

- CA-325: `actions` de una `Card` sigue la tabla de columnas del contrato, en su orden y con la principal primero, con fixtures para las ocho columnas (en Review, con y sin review exigida y con y sin hallazgos que bloquean). `reanudar` y `relanzar` aparecen solo con `interrupted`, `descartar` solo con `interrupted`, espacio de trabajo propio y rutas descartables, `guardar` solo con `unsynced`, y `sesion` y `terminal` solo con espacio de trabajo propio. `expert` es `true` solo en `descartar`, `sesion` y `terminal`. `hoom board --json`, `hoom item show --json` y `/api/board` traen `actions`, `drops` y `ghost`, sin listas `null`, y el texto de `hoom board` y `hoom item show` no cambia.
- CA-326: `enabled` y `why` siguen la tabla del contrato y su orden: con `running` toda acción menos `sesion` y `terminal` está deshabilitada con `espera a que termine el <rol> que esta trabajando`, y sin espacio de trabajo propio fuera de Backlog las de rol lo están con su frase. En todos los fixtures un `why` no vacío no contiene `git`, `commit`, `worktree`, `merge`, `rama`, `branch`, `diff`, `HEAD` ni `huella` (enteras, sin distinguir mayúsculas), ni `.hoom/` ni `hoom `.
- CA-327: `spend.remaining_usd` es el presupuesto menos el costo reportado (puede ser negativo) y `null` sin presupuesto. Con `remaining_usd <= 0` las acciones de rol dicen `se agoto el presupuesto de la tarjeta: se gastaron <g> de <p> USD`. Cada opción de provider trae `budget`, `min_budget_usd` y `ok`/`why`: sin `system_prompt`, por debajo del mínimo (`quedan <r> USD y <p> necesita al menos <m> USD por run`) y, en la review, siendo un writer de la tarjeta. `budget_usd` propuesto es `null` sin presupuesto, `remaining_usd` con él y `remaining_usd / 4` en la review.
- CA-328: `providers.Info` trae `min_budget_usd`: 0.5 para claude y 0 para codex, gemini y opencode, y `hoom providers --json` lo emite. Un provider que no implementa `BudgetFloor` declara 0.
- CA-329: el `pedido` propuesto de cada acción es el texto del contrato: el del arquitecto con y sin `pedido` en el item, el del test-writer con los criterios sin prueba, el del writer y el de Review con los hallazgos que bloquean, `""` en la review, el de reanudar con su paso, y el de volver a lanzar igual al de pedir para los cuatro roles.
- CA-330: `reanudar` trae `resume_id` del sidecar del run del sobre interrumpido y una sola opción de provider, la de ese sobre. Está deshabilitada con la frase del contrato sin `run_id`, sin id de sesión en el sidecar, con un sobre ciego, con el rol reviewer y con un provider sin la capacidad `resume`. `relanzar` está disponible en esos cinco casos.
- CA-331: `drops` trae una entrada por cada columna que no es la de la tarjeta, en el orden fijo. La siguiente lleva la acción principal (en Tu aprobación, `aprobar`; en Tu aceptación, `integrar`), o `plain` si no hay acción principal. Una anterior dice `la tarjeta no vuelve atras: su columna la da la evidencia`, y una más allá, `primero tiene que llegar a <Siguiente>: <plain>`. Una tarjeta en Hecho solo tiene columnas anteriores.
- CA-332: `ghost` existe solo si hay `running` con un rol de estación de la columna de la tarjeta, y está en la columna siguiente con el rol, el provider, los ids y el `stage`, `step` y `steps` de `running`. Un scout en curso, o un arquitecto que corrige el spec en Tu aprobación, no tiene fantasma. Cuando el sobre cierra `no-entregable` el fantasma desaparece y `red.plain` da el motivo. Cuando cierra `entregable` sin la evidencia (un spec que no pasa lint), desaparece y `plain` dice lo que falta.
- CA-333: el registro del sobre trae `pid`, que escriben `hoom agent` y `hoom review` con el PID de su proceso. `Gather` da por vivo un sobre abierto con `pid` solo si ese proceso vive, con o sin latido fresco, y aplica la regla de C1 a un registro sin `pid`. Un sobre abierto sin dueño vivo deja de ser `interrupted` cuando otro sobre de la tarjeta empezó después de su último registro, y su archivo no cambia. Los tests de CA-282 y CA-283 pasan sin cambios.
- CA-334: `hoom review` escribe un registro de sobre con rol `reviewer`, `steps` igual a la cantidad de lentes, el paso `run` de cada pasada con su `run_id`, y cierre `entregable` con `stage: "ok"` o `no-entregable` con `stage` y la nota del contrato. Una review que se decide antes de la primera pasada no escribe registro. El gasto de la tarjeta no cuenta dos veces una pasada.
- CA-335: `agentcmd.Run` y `reviewcmd.Run` usan `EnvelopeID` cuando viene y llaman a `Started` una sola vez, con el primer registro ya en disco. Con un error antes de ese registro, `Started` no se llama y no queda registro.
- CA-336: `POST /api/board/{slug}/launch` con `pedir-arquitecto` sobre una tarjeta en Backlog sin espacio de trabajo corre `hoom task start` y lanza el sobre con el rol, el provider, el modelo, el presupuesto, el pedido y `task` = el slug, sin `--spec`. Responde 202 con `envelope_id` y `task_started: true` y el registro existe. Mientras el sobre corre, la tarjeta tiene `ghost` en la columna Arquitecto, y la salida del sobre queda en `.hoom/envelopes/<id>.log`.
- CA-337: `pedir-test-writer` y `pedir-writer` lanzan el sobre con `--spec .hoom/specs/<slug>.md` en el espacio de trabajo de la tarjeta. `pedir-reviewer` corre `hoom review` con `task` y `spec`, sin `--same-provider`, y rechaza un pedido (400). Una review que se niega antes de la primera pasada (no cruzada) responde 409 con su nota.
- CA-338: el launch sin token o con un token equivocado es 401 y no crea tarea, registro ni run. Una acción que no está en la tarjeta (columna equivocada) es 409 con `la tarjeta ya no esta donde la viste: ahora esta en <Columna> (<plain>)`, una deshabilitada es 409 con su `why`, una clave desconocida o una `action` desconocida es 400, un pedido de relleno es 400 y un provider que no es opción de la acción es 400. En ninguno de estos casos queda tarea, registro, run ni log nuevo en disco.
- CA-339: con el presupuesto agotado el launch es 409 con la frase de CA-327. Con Claude, un `budget_usd` por debajo de 0.5 es 409, uno mayor que lo que queda es 409 y `null` lanza con lo que queda. Con Codex, un `budget_usd` numérico es 400 y `null` lanza sin tope. Sin presupuesto en el item, `null` lanza sin tope.
- CA-340: con la tarjeta en curso el launch es 409. Con un run activo de otro proceso en el árbol de la tarjeta, 409 nombrando el run. Dos launches simultáneos sobre la misma tarjeta dan un 202 y un 409.
- CA-341: `reanudar` lanza un sobre nuevo con `ResumeID` = el `resume_id` de la acción y el rol y el provider del interrumpido, y `relanzar` uno sin `ResumeID`. En los dos, el registro del sobre interrumpido queda byte a byte igual y la tarjeta deja de estar interrumpida.
- CA-342: `POST /api/specs/{name}/approve` con `{"card": "<slug>"}` escribe la aprobación en el árbol de evidencia de la tarjeta (su espacio de trabajo), con `approved_by` igual a `gitx.Identity` de ese árbol, y la tarjeta sale de Tu aprobación. Con `name` distinto del slug es 400, con la tarjeta fuera de Tu aprobación es 409, con una clave `approved_by` es 400 y sin token es 401, sin aprobación escrita en ninguno de los cuatro. Sin cuerpo, el endpoint hace lo de hoy y sus tests pasan sin cambios.
- CA-343: una tarjeta en Tu aceptación, integrada con `POST /api/tasks/{slug}/done`, pasa a Hecho con `hecho_en` y `commit_final` en el item. Una en Writer recibe 409 con el mensaje de `task done`, y su item y su espacio de trabajo quedan igual. `integrar` aparece solo en Tu aceptación.
- CA-344: `hoom item save <slug>` hace un commit por árbol con `hoom: guardar la tarjeta <slug>`: en el espacio de trabajo, sus rutas sin guardar; en el árbol raíz, solo el item. Un archivo ajeno que estaba en el índice del árbol raíz sigue ahí sin commitear. Después, `unsynced` está vacío. Con la tarjeta en curso se niega sin commitear. Sin nada que guardar dice `la tarjeta <slug> no tiene nada sin guardar` y sale con 0. `--json` emite el `SaveResult` del contrato.
- CA-345: `POST /api/board/{slug}/save` con las rutas de `unsynced` responde 200 con el `SaveResult`. Con otras rutas es 409 con `los cambios de la tarjeta cambiaron desde que los viste: revisalos de nuevo`, sin cuerpo o sin `paths` es 400 y sin token es 401, sin commit en los tres casos.
- CA-346: `hoom task discard <slug>` sin `--yes` lista las rutas y sale con 1 sin tocar nada. Con `--yes` vuelve a `HEAD` las rutas que `HEAD` tiene y borra las que no, y nunca toca nada bajo `.hoom/` (un veredicto sin commitear sigue ahí). Con un run activo en el árbol se niega. `POST /api/board/{slug}/discard` exige la acción `descartar` habilitada y las mismas rutas (409 si no), y sin token es 401.
- CA-347: `cockpitcmd.Open` arma con tmux el plan de CA-82 y CA-85 sin `attach-session` ni `switch-client`, y devuelve `created: true`. Con la sesión existente devuelve `created: false` y no crea nada. `Run` es `Open` más el attach, y los tests de CA-81 a CA-88 pasan sin cambios.
- CA-348: cuando `Open` crea la sesión de una tarea con item, el item del árbol raíz gana una entrada en `sesiones` con `provider`, `abierta_por` = `gitx.Identity` y `abierta_en`. Con la sesión existente, o sin item, no escribe nada. `POST /api/board/{slug}/session` responde con `session`, `created`, `provider`, `dir` y `attach`. Sin espacio de trabajo propio es 409, sin tmux o sin el provider instalado es 409 con el mensaje de `cockpitcmd`, sin `provider` es 400 y sin token es 401.
- CA-349: `item.Parse` acepta `sesiones` con `provider` y `abierta_en` en cada entrada, rechaza una entrada sin ellos y sigue rechazando claves desconocidas. Un item sin `sesiones` se lee y se escribe byte a byte como antes. `item.AddSession` agrega al final sin tocar las demás claves.
- CA-350: `GET /api/board/{slug}/terminal` responde el `text` de `tmux capture-pane -p -e` del primer pane de la sesión de la tarjeta, con sus secuencias, cortado en 256 KiB. Sin sesión o sin tmux responde 200 con `available: false` y la nota del contrato. No pide token, `POST` es 405 y la lectura deja el repo y `.hoom/` byte a byte iguales.
- CA-351: `hoom review` trae `writers_declared` en su resultado y en su registro, desde las `sesiones` del item. Con el reviewer igual a un writer declarado es `no-cruzada` y se niega sin `--same-provider`, aunque el observado sea otro. Sin writer observado y con declarados distintos del reviewer es `cruzada-declarada`, nunca `cruzada`. Sin `--provider` elige uno que no sea ni el observado ni un declarado. `providers.declared` de la tarjeta lista los providers de `sesiones`. Los tests de review que ya existen pasan sin cambios.
- CA-352: la regla de oro. `servecmd.Routes()` lista cada ruta que registra `Handler`, y ninguna contiene `column`, `columna`, `move` ni `mover`. Ningún struct del paquete `servecmd` tiene una etiqueta json `column`, `columna`, `to`, `from`, `destino` ni `destination`. Un POST con token a cada ruta POST, con `{"column": "hecho", "columna": "hecho"}`, deja la columna de una tarjeta de fixture igual, y los endpoints nuevos responden 400 a esa clave.
- CA-319: (re-expresado, de C2) `index.html` tiene las pestañas Cockpit y Tablero y carga `tablero.js` y después `acciones.js`, que se sirven embebidos (`GET /tablero.js` y `GET /acciones.js` son 200). La UI embebida son exactamente `index.html`, `tablero.js` y `acciones.js`, y el test de UI sin assets de red pasa sobre los tres. `index.html` define `--human`, y `tablero.js` pinta las columnas desde `columns`, usa `human`, `needs_decision`, `meter`, `plain` y `providers`, y reusa `renderStage` y `appendFeed`, que reciben su contenedor.
- CA-353: `tablero.js` sigue cumpliendo CA-320 sin cambios en su test, pinta el fantasma desde `ghost`, cada tarjeta dentro de `.tcard-wrap` con `data-slug` y `.tacc`, emite `tablero:pintado` después de pintar, y pide `/api/board/{slug}/terminal` con `setTimeout(..., 1000)` encadenado mientras la sección Terminal está abierta. La sección Terminal no tiene `<input`, `<textarea` ni `send-keys`.
- CA-354: `acciones.js` hace cada POST con `act(` a `/launch`, `/approve`, `/done`, `/save`, `/discard` y `/session`, usa `actions`, `enabled`, `why`, `drops`, `draggable`, `dragstart` y `drop`, y no contiene `setInterval`, `fetch(` ni ningún id de columna como cadena. El diálogo de rol tiene provider, modelo, presupuesto y pedido, y contiene `no tiene tope`. Las acciones con `expert` solo se muestran en modo experto.
- CA-355: `acciones.js` tiene el bloque `/* vocabulario normal */` ... `/* fin vocabulario normal */`, que contiene "Guardar" e "Integrar" y no contiene `git`, `commit`, `worktree`, `merge`, `rama`, `branch`, `diff`, `HEAD` ni `huella`. Las etiquetas "Abrir sesión", "Aprobar spec", "Volver a lanzar" y "Guardar en git" están con sus acentos, y "Guardar en git" solo fuera del bloque normal.
- CA-356: sin dependencias nuevas: `go.mod` y `go.sum` no cambian respecto de la rama base. [verifica: git diff --quiet main -- go.mod go.sum]
- CA-357: dogfood: hoomai tiene el item de esta spec. [verifica: go run ./cmd/hoom item show acciones-desde-la-tarjeta --json | grep -q '"slug": "acciones-desde-la-tarjeta"']
- CA-358: el README documenta las acciones de la tarjeta y sus endpoints. [verifica: grep -q "/api/board/{slug}/launch" README.md]

## Decisiones

- **La tarjeta sabe qué acciones valen, y la página no.** `actions`, `drops`
  y `ghost` salen de `Derive` por la misma razón que el medidor en C2: la
  regla vive en Go, con tests, e igual en `hoom item show --json` que en el
  Studio. Cada POST vuelve a derivar la tarjeta, así que un botón viejo en una
  pestaña que no se refrescó no puede actuar sobre una tarjeta que ya se
  movió.
- **La columna es la estación donde está el trabajo pendiente (C1), así que
  "pedir" es hacer el trabajo de la columna actual**, y la columna siguiente
  es adonde se suelta y donde aparece el fantasma. Arrastrar de Test-writer a
  Writer es pedirle al test-writer. De Backlog a Arquitecto es pedirle al
  arquitecto: si su spec pasa lint, la tarjeta salta a Tu aprobación, porque
  la columna la da la evidencia y no el arrastre.
- **Una sola acción principal por columna.** El arrastre necesita un destino
  sin ambigüedad. Las demás son botones: pedirle cambios al arquitecto en Tu
  aprobación (no gana ninguna columna) y, en Review, la segunda de las dos
  cuando la primera no aplica.
- **Nunca el writer antes que los tests.** En Test-writer no se ofrece
  `pedir-writer`: la anti-circularidad del método (Spec D) no se saltea desde
  un botón.
- **Pedir al reviewer corre `hoom review`, no `hoom agent --role reviewer`.**
  El pedido decía "lanza el sobre" para los cuatro roles, pero el sobre de un
  reviewer no deja el registro de review de 4 lentes que exige la columna
  (C1). Para que la review tenga fantasma, interrumpido y motivo como los
  demás, `hoom review` pasa a dejar un registro de sobre. Pendiente de
  confirmar al aprobar.
- **Aprobar e Integrar son los endpoints de hoy.** `approve` solo gana `card`
  para resolver el spec en el espacio de trabajo de la tarjeta, que es donde
  vive su evidencia (el endpoint de hoy aprueba el del árbol raíz), y se
  niega fuera de Tu aprobación. `done` no cambia: sus condiciones son las del
  verbo.
- **Guardar commitea `unsynced` entero.** El pedido nombra la evidencia del
  item (item, spec, aprobación, veredictos, hallazgos) y dice que el ícono
  "sin sincronizar" se apaga al guardar. En el árbol compartido `unsynced` es
  exactamente esa evidencia (C1), pero en el espacio de trabajo de la tarea
  también es el código, porque ese árbol es de la tarjeta sola y `task done`
  lo exige limpio. Guardar solo la evidencia dejaría el ícono prendido.
  Pendiente de confirmar: si preferís que el código lo commitee siempre la
  persona desde la terminal, el cambio es filtrar `paths` a `.hoom/`.
- **Guardar y Descartar tienen verbo** (`hoom item save`, `hoom task
  discard`): el Studio es un control remoto y lo nuevo nace como verbo. Los
  dos piden las rutas que la persona vio y se niegan si cambiaron. Descartar
  nunca toca `.hoom/`: la evidencia es de solo agregar.
- **Reanudar y volver a lanzar abren un sobre nuevo.** El registro viejo dice
  la verdad sobre lo que pasó y hoom no cierra lo que no vio cerrar. Para que
  la tarjeta no quede interrumpida para siempre, un sobre posterior de la
  tarjeta lo **releva**: la persona ya decidió.
- **Reanudar mide el árbol como quedó.** El sobre nuevo toma su foto
  "antes" al empezar, así que lo que el run muerto dejó escrito es su punto
  de partida. Por eso el diálogo lo avisa y el modo experto ofrece
  Descartar.
- **Un rol ciego y la review no se reanudan.** La sesión del test-writer
  vivía en un árbol ciego que ya no existe (Claude busca sus sesiones por
  directorio), y el registro de review exige todas sus lentes en una sola
  corrida.
- **El PID del dueño va en el registro del sobre** (brecha 4). Con él,
  "interrumpido" es un hecho apenas muere el proceso, y un `verify` de más de
  15 minutos deja de verse interrumpido (riesgo aceptado en C2).
- **El sobre corre dentro de `hoom serve`**, en una goroutine, con la misma
  función que la CLI. Si el Studio se cae, el sobre muere con él y la tarjeta
  lo muestra interrumpido. Un proceso aparte sería otro camino para lanzar
  roles, y el RFC lo prohíbe.
- **El mínimo de Claude es 0.5 USD**, declarado en su adapter. Pendiente de
  confirmar.
- **La review tiene presupuesto por lente**, porque `hoom review` le pasa
  `--budget-usd` a cada pasada. El diálogo propone lo que queda dividido 4 y
  el servidor exige que cuatro veces el valor quepa en lo que queda.
- **Desde la cabina no hay `--same-provider`.** La review que no sería cruzada
  no se ofrece: el provider del writer aparece como opción no `ok`.
- **El writer declarado nunca asciende a "cruzada".** Una sesión interactiva
  no deja sidecar, así que nadie vio qué escribió. Declararla puede volver una
  review `no-cruzada` pero nunca `cruzada`: sin writer observado, lo más que
  llega es `cruzada-declarada`. Esto absorbe el ítem "hoom review y el writer
  declarado" del backlog.
- **`hoom cockpit --task` también declara el writer.** Si solo lo hiciera el
  Studio, la misma sesión contaría distinto según por dónde se abrió.
- **Ver terminal es una lectura sin token**, como la narración de los runs:
  el Studio escucha en loopback y otra página no puede leer sus respuestas.
  Escribir en el pane queda afuera (no-goals).
- **Un archivo `acciones.js` aparte.** `tablero.js` sigue siendo el pintor
  sin POST de C2 y su test (CA-320) no cambia. Lo único que se re-expresa es
  la lista de archivos de la UI (CA-319).
- **Pedir solo con espacio de trabajo propio, salvo en Backlog.** Crear la
  tarea desde otra columna la armaría desde la rama base, sin el spec ni la
  aprobación que viven en el proyecto, y la tarjeta retrocedería a Backlog.

## Riesgos y deuda aceptada

- **El flujo de Orca no puede pedir desde el tablero** después de Backlog: sus
  tarjetas no tienen espacio de trabajo de hoom (brecha 6). hoomai mismo
  trabaja así, así que su propio tablero va a mostrar esas acciones
  deshabilitadas hasta que exista la brecha 6.
- **Lo que el run muerto dejó escrito no lo mide el gate de scope** del sobre
  que reanuda. Queda a la vista en `unsynced` y en el diálogo, y lo cubren
  `verify` y `check` al cerrar, pero una escritura fuera de territorio de un
  run muerto no se registra como hallazgo.
- **El PID puede reciclarse.** Después de un reinicio, otro proceso con el
  mismo PID haría ver vivo un sobre muerto. Es improbable y se corrige solo
  cuando el proceso termina.
- **El presupuesto solo cuenta lo que se reportó en esta computadora.** El
  gasto de Codex no tiene USD y el de otras computadoras no viaja (C2). El
  diálogo lo dice con Codex.
- **Un `hoom` viejo no lee un item con `sesiones`**: lo marca inválido por
  clave desconocida. Es el costo del parse estricto de C1.
- **Guardar corre los hooks del repo.** Un pre-commit lento hace lento el
  botón, y uno que falla lo rechaza con su salida.
- **El sobre corre dentro del Studio**, así que cerrar `hoom serve` corta un
  trabajo que puede costar dinero. La tarjeta lo muestra interrumpido y se
  puede reanudar si el agente dejó sesión.
- **La cuarentena de un test-writer interrumpido queda en disco**
  (`.hoom/isolated/`). Volver a lanzar arma otra. Descartarla es otra spec.
- **La salida del sobre en `.hoom/envelopes/<id>.log` crece sin límite.** Es
  telemetría local, fuera de git, como los `.jsonl` de los runs.
- **Los tests de la UI son estáticos**, como en C2. Que el arrastre y los
  diálogos se vean y funcionen se comprueba a mano con `hoom serve` antes del
  PR.
- **El espejo de la terminal no es tiempo real**: se refresca cada segundo,
  y pinta solo los colores básicos.
