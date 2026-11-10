# Spec: el rol ciego corre sin tu configuración personal

Estado: BORRADOR — pendiente de aprobación humana.
Tarea: `hoom task start ciego-sin-config-personal` (rama
`hoom/ciego-sin-config-personal`).
Origen: el análisis de Engram del 2026-11-10 y una prueba con claude
2.1.319 ese mismo día.
Re-expresa: CA-398 y CA-400 (`review-aislada-y-modelo-elegido.md`), solo en
lo que este spec cambia.

## Objetivo

El test-writer escribe tests "sin ver jamás la implementación". hoom lo
garantiza en el disco con el árbol ciego (un worktree disperso). Pero
`hoom agent` lanza al rol ciego con toda la configuración personal del CLI,
y por ahí entra lo que el disco esconde:

- **Los servidores MCP del usuario.** Claude carga los de `~/.claude.json`.
  Con Engram, `mem_search`, `mem_context` y `mem_get_observation` suelen
  estar pre-aprobados en `~/.claude/settings.json`, así que responden sin
  pedir permiso, y las instrucciones del servidor piden llamar a
  `mem_context` al empezar. Las memorias de un proyecto dicen qué se
  implementó, dónde y cómo.
- **La memoria automática de Claude.** `~/.claude/projects/<repo>/memory/`
  es la misma para todos los worktrees del repo, árbol ciego incluido, y su
  `MEMORY.md` entra directo al contexto, sin llamar a nada. Prueba del
  2026-11-10: en un worktree de `.hoom/isolated/` y con el aislamiento que
  hoy usa la review (`--strict-mcp-config --setting-sources project`), el
  evento de inicio traía `mcp_servers: []` pero
  `memory_paths.auto` apuntaba a esa carpeta, y el modelo citó la primera
  línea de `MEMORY.md`.

Con este spec el rol ciego corre siempre aislado de la configuración
personal del CLI, con el mismo `Isolated` que ya usa `hoom review`, y en
Claude ese aislamiento apaga también la memoria automática. Un provider que
no puede aislarse no corre un rol ciego.

**Regla.** El rol ciego no obedece la regla de oro, la habita: también
fuera del disco.

## No-goals

- Tocar la venda del disco: patrones, sparse-checkout, cuarentena,
  trasplante.
- Que hoom lea el evento de inicio del CLI para comprobar qué cargó (ver
  Decisiones y Riesgos).
- Lo que `--ignore-user-config` de Codex deja pasar (ver Riesgos).
- Revalidar los runs ciegos anteriores a este spec.
- Una memoria dentro de hoom, o cambiar cómo usan Engram los demás roles.
- `hoom run` (sin rol) y los roles que no son ciegos: siguen igual.
- Una opción en `hoom.yaml` para correr el rol ciego sin aislamiento.
- Sumar un mecanismo nuevo en `hoom roles` para claude y codex.

## Contratos

### El sobre del rol ciego

- `startOptions` pone `Isolated: true` en el `runcmd.StartOptions` de todo
  rol con `Isolated` en la tabla de roles (hoy solo el test-writer). Los
  demás roles siguen con `Isolated: false`.
- `func IsolationFor(p providers.Provider, r agents.Role) error` en
  `agentcmd`, como `ReadOnlyFor` y `NoExecFor`:
  - `nil` si `r` no es ciego o si `p.Capabilities().Isolation`;
  - si no, un error con este texto exacto (sin punto final en la segunda
    línea):

    ```
    <p> no puede correr sin tu configuracion personal (MCP, memoria, hooks): un rol ciego no corre ahi.
      Accion: usa un provider que pueda aislarse: <lista>
    ```

    `<p>` es `p.Name()` y `<lista>` son los nombres de `providers.All()`
    que declaran `Isolation`, en el orden del registro y separados por
    `, ` (hoy `claude, codex`).
- `hoom agent` pregunta `IsolationFor` al empezar el paso `aislar`, antes
  de cualquier otro chequeo del árbol ciego. Con error, el sobre cierra en
  `aislar` con exit 1 y ese texto como nota: sin crear nada en
  `.hoom/isolated/`, sin lanzar el run y sin veredicto.

### El aislamiento de Claude

- Con `Isolated`, el argv de claude lleva estos cinco argumentos seguidos y
  en este orden, entre `-p` y el prompt, una sola vez:
  `--strict-mcp-config`, `--setting-sources`, `project`, `--settings`,
  `{"autoMemoryEnabled":false}`. Hoy lleva los tres primeros.
- Vale para todo uso de `Isolated`: el rol ciego y cada pasada de
  `hoom review` aislada, que desde ahora tampoco ve la memoria automática.
- Codex no cambia: con `Isolated` lleva `--ignore-user-config`, como hoy.

### `hoom roles`

- Con un provider que tiene system prompt y no declara `Isolation`, las tres
  celdas del test-writer (lectura, escritura y ejecución) son `UNSUPPORTED`,
  con `Allows` `no corre el rol ciego`, el mecanismo nuevo
  `MechNoIsolation = "sin aislamiento"` y `Gap` igual a la primera línea del
  error de `IsolationFor`. Es lo que ya hace con un provider sin system
  prompt. Los demás roles, y el test-writer con claude y con codex, quedan
  como hoy.

### Documentación y pruebas

- El README, en la fila de `hoom agent`, dice que el rol ciego corre además
  sin la configuración personal del CLI (en claude: sin los servidores MCP
  del usuario, sin sus hooks, plugins ni permisos y sin la memoria
  automática; en codex: sin `config.toml`), y que un provider que no puede
  aislarse no corre un rol ciego. Esa fila se escribe sin tildes, como el
  resto de la tabla.
- El comentario de `providers.Request.Isolated` nombra la memoria
  automática.
- Los tests usan los CLIs falsos de siempre (un script en el `PATH` que
  vuelca su argv) y un provider falso para el caso sin `Isolation`; ninguno
  llama a un CLI real.
- Los tests que hoy fijan lo contrario (que `Isolated` de claude agrega solo
  tres argumentos, CA-398, y que el sobre nunca pide `Isolated`, CA-400)
  pasan a esperar lo de este spec y citan los dos ids. Es el cambio que este
  spec aprueba, no un test debilitado.

## Casos límite y errores esperados

- **`--resume` o un pedido más al mismo run**: el run recuerda sus
  `StartOptions`, así que sigue aislado. Una sesión del CLI creada antes de
  este spec trae su contexto viejo (Riesgos).
- **Un `.claude/settings.json` del proyecto con `autoMemoryEnabled: true`**:
  `--settings` le gana a la config del proyecto, y la memoria automática
  sigue apagada.
- **Un `.mcp.json` del proyecto**: `--strict-mcp-config` lo ignora, como
  ignora el del usuario.
- **Los hooks del proyecto** (`.claude/settings.json` commiteado): se
  cargan si el archivo está en el árbol ciego. Son configuración del repo,
  no personal, y se revisan en git.
- **Quien autentica Claude con `apiKeyHelper` o `env` en su settings de
  usuario, o usa un `model_provider` propio en Codex**: el rol ciego falla
  al autenticar con ese provider, como ya le pasa a la review aislada. No
  hay escape en `hoom.yaml`; correr sin la garantía es `hoom run`.
- **`review.isolated: false` en `hoom.yaml`**: la review sigue sin
  aislamiento, con la memoria automática a la vista, como hoy.
- **gemini y opencode**: ya no corren ningún rol (no tienen system prompt) y
  `pickProvider` los rechaza antes de llegar a `aislar`; este chequeo es
  para el próximo adaptador.

## Criterios de aceptación

- CA-588: `startOptions` da `Isolated: true` para el test-writer, con claude y con codex, y `Isolated: false` para orquestador, arquitecto, designer, scout, writer, reviewer, characterizer, analista y refutador; re-expresa CA-400 solo para el rol ciego, y el resto de CA-400 sigue igual.
- CA-589: con `Isolated`, el argv de claude contiene `--strict-mcp-config`, `--setting-sources`, `project`, `--settings` y `{"autoMemoryEnabled":false}`, seguidos, en ese orden, una sola vez y entre `-p` y el prompt, y `Isolated` no agrega nada más (re-expresa CA-398 en lo que agrega `Isolated`; el resto de CA-398 sigue); sin `Isolated` no aparece ninguno de los cinco.
- CA-590: con los CLIs falsos, `hoom agent --role test-writer` lanza claude con los cinco argumentos de CA-589 y codex con `--ignore-user-config`, y `hoom agent --role writer` no lanza ninguno de los dos con esos argumentos.
- CA-591: `agentcmd.IsolationFor` devuelve `nil` para todo rol sin `Isolated` con cualquier provider y para el test-writer con claude y con codex; con un provider que no declara `Isolation`, devuelve para el test-writer el error de dos líneas del contrato, con el nombre del provider y la lista `claude, codex` sacada de los providers registrados que declaran `Isolation`.
- CA-592: con un provider registrado que tiene system prompt y no declara `Isolation`, `hoom agent --role test-writer` cierra con `Stage` `aislar`, exit 1 y la nota de CA-591 en su `Result` y en el registro del sobre, sin crear nada en `.hoom/isolated/`, sin ningún run en `.hoom/runs/` y sin veredicto nuevo.
- CA-593: `rolescmd.Matrix` con un provider que tiene system prompt y no declara `Isolation` da para el test-writer las tres celdas `UNSUPPORTED`, con `Allows` `no corre el rol ciego`, mecanismos exactamente `[sin aislamiento]` y `Gap` igual a la primera línea del error de CA-591; con ese provider, las filas de los demás roles son iguales a las de un provider con las mismas capacidades más `Isolation`, y las filas de claude y codex no cambian.
- CA-594: el README dice que el rol ciego corre sin la configuración personal del CLI, sin la memoria automática, y que un provider que no puede aislarse no corre un rol ciego. [verifica: grep -q "sin la memoria automatica" README.md && grep -q "no puede aislarse no corre un rol ciego" README.md]

## Decisiones

- **El `Isolated` que ya existe, no un mecanismo nuevo.** La review lo usa
  desde `review-aislada-y-modelo-elegido`; "aislado" tiene un solo
  significado en hoom y un solo lugar donde cambiarlo.
- **Contra CA-400 y el no-goal de `codex-v2` solo para el rol ciego.** Ese
  no-goal ("Pisar la config del usuario de Codex") y CA-400 dejan que la CLI
  se comporte como el usuario la conoce. Para el rol ciego esa configuración
  es justo la fuga. Los demás roles siguen igual.
- **La memoria automática va dentro del `Isolated` de claude, también para
  la review.** Es configuración personal, como el MCP. La review aislada
  promete evidencia congelada, y una memoria que dice qué hallazgos ya se
  descartaron la contamina.
- **`--settings` en el argv y no `CLAUDE_CODE_DISABLE_AUTO_MEMORY=1`.** Lo
  que el rol puede hacer queda escrito en el argv, como sus herramientas.
  Las dos vías se probaron el 2026-11-10 con claude 2.1.319: las dos dejan
  `memory_paths` vacío y el modelo ya no ve `MEMORY.md`.
- **Ni `--bare` ni `--safe-mode`.** `--bare` no lee OAuth ni el llavero, y
  rompe el uso con suscripción. `--safe-mode` es para diagnosticar una
  config rota, apaga también el CLAUDE.md del proyecto y su alcance puede
  cambiar entre versiones.
- **Negarse en vez de avisar.** Un aviso dejaría correr un test-writer con
  la memoria a la vista, y su registro diría igual que escribió a ciegas.
  Falla segura, como el resto del árbol ciego.
- **Sin escape en `hoom.yaml`.** Un rol ciego que no es ciego no es el
  rol; correr sin la garantía ya tiene nombre: `hoom run`.
- **Sin leer el evento de inicio en este spec.** Comprobar `mcp_servers` y
  `memory_paths` del run sería la prueba más fuerte, pero es distinta en
  cada CLI y necesita su propio camino en el run. Queda como la mitigación
  del primer riesgo.

## Riesgos y deuda aceptada

- **Una versión nueva del CLI con otra vía de contexto personal** que estos
  argumentos no cubren reabre el agujero sin que hoom lo note. Lo cerraría
  leer el evento de inicio del run y cortar si no viene vacío.
- **Codex**: `--ignore-user-config` salta `config.toml` (MCP, perfil,
  modelo). No está verificado si salta el `~/.codex/AGENTS.md` global,
  `hooks.json` o una memoria propia de Codex. Hoy ese `AGENTS.md` solo trae
  instrucciones de codebase-memory-mcp, cuyo servidor no se carga, y
  `~/.codex/memories` está vacío.
- **El CLAUDE.md del usuario** (`~/.claude/CLAUDE.md`): se espera que
  `--setting-sources project` lo deje fuera; no está verificado.
- **El rol ciego pierde los hooks del usuario** (Orca, supacode): la cabina
  externa deja de ver sus eventos, como ya pasa con la review aislada.
- **Runs y sesiones viejos**: los sidecars de antes dicen `isolated: true`
  aunque corrieron con la config personal, y no se reescriben. Un
  `--resume` de una de esas sesiones trae su contexto viejo.
- **Lo que otros roles guardan en Engram** sigue siendo narración: este
  spec solo cierra la lectura del rol ciego.
