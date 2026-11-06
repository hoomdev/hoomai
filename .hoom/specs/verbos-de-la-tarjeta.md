# Spec: verbos de la tarjeta (la CLI fácil, capa 1)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start verbos-de-la-tarjeta`.
Depende de: nada nuevo; usa las acciones de la tarjeta de
`acciones-desde-la-tarjeta.md` (C3), ya integrada.
Habilita: `tablero-en-la-terminal.md` (capa 2), que lanza todo por los verbos
de esta spec.
Origen: el pedido de Henry del 2026-11-05, después de llevar a mano la
tarjeta `infra-api-convenciones` de oceanbuild por el Studio y por la
terminal: "en vez de escribir todo eso, que sea fácil seleccionar y solo
escribir cosas como el pedido". Ese mismo día salieron cinco tropiezos, y
esta spec arregla tres: el esfuerzo que solo se elige en la review, el tiempo
límite fijo de 30 minutos con un mensaje que culpa a los permisos cuando el
agente estaba pensando, y el test-writer que se puede pedir con el spec sin
guardar.
Re-expresa: CA-409 (review-aislada-y-modelo-elegido), porque `POST /launch`
pasa a aceptar `effort` para todas las acciones de rol.

## Objetivo

Que llevar una tarjeta desde la terminal sea escribir el pedido y nada más.
Hoy, para la tarjeta `infra-ci-cd`, hay que escribir:

```
hoom agent --role writer --task infra-ci-cd --spec .hoom/specs/infra-ci-cd.md \
  --provider claude --model opus --budget-usd 5 "Implementa el spec"
```

Casi todo eso ya lo sabe hoom: la tarjeta sale de la carpeta donde estás, la
ruta del spec es siempre `.hoom/specs/<slug>.md`, el rol sale del verbo, y el
provider, el modelo, el esfuerzo y el presupuesto pueden salir de `hoom.yaml`.
Después de esta spec, lo mismo es:

```
hoom pedir writer -m "Implementa el spec"
```

o, sin `-m`, se abre tu editor con el pedido que propone la tarjeta, como
`git commit`.

Cuatro piezas:

1. **Valores por rol en `hoom.yaml`.** `agents.<rol>` gana `provider`,
   `model`, `effort`, `budget_usd` y `timeout`, y `review:` gana `timeout`. Los
   usan `hoom agent`, los verbos nuevos y el diálogo del Studio, siempre con
   el mismo orden: lo que se pide en el comando, después `hoom.yaml`, después
   lo que la tarjeta propone hoy.
2. **Esfuerzo y tiempo límite para todos los roles.** `hoom agent` gana
   `--effort` y `--timeout`, el Studio pide esfuerzo para los cuatro roles, y
   el tiempo límite deja de ser una constante. Cuando se cumple, el mensaje
   dice si la CLI seguía trabajando o estaba muda.
3. **Los verbos de la tarjeta.** `hoom pedir <rol>`, `hoom siguiente`,
   `hoom aprobar`, `hoom guardar`, `hoom integrar`, `hoom reanudar` y
   `hoom usar`. Cada uno es una acción de la tarjeta (la misma lista que
   pinta el Studio) y lanza por la misma función que `POST /launch`.
4. **El test-writer no se pide con el spec sin guardar.** Su acción se
   deshabilita con el motivo, en vez de fallar en el paso 2 del sobre.

**Regla de oro.** Los verbos nuevos no deciden nada que la tarjeta no decida
ya. La columna, las acciones válidas, su motivo cuando no valen, el pedido
propuesto y el presupuesto propuesto siguen saliendo de `boardcmd.Derive`. Un
verbo de la tarjeta toma una acción de esa lista y la ejecuta, o dice por qué
no.

**Regla de transparencia.** Ningún valor se elige en silencio. Antes de
lanzar, el verbo imprime el comando largo equivalente y de dónde salió cada
valor. Es la línea roja de hoom: el harness no declara más de lo que
verificó, y tampoco esconde lo que eligió.

## No-goals

- **La TUI.** Es `tablero-en-la-terminal.md` (capa 2).
- **Cambiar los verbos que existen.** `hoom agent`, `hoom review`,
  `hoom spec approve`, `hoom item save`, `hoom task done` y
  `hoom task discard` conservan su sintaxis y su efecto. Solo ganan los flags
  `--effort` y `--timeout` (`hoom agent`), `timeout` en `review:` y los
  valores de `hoom.yaml`. En particular no deducen la tarjeta: `hoom review`
  sin `--task` sigue revisando el proyecto, y deducirla le cambiaría el
  sentido en silencio.
- **Cambiar `next` de la tarjeta o el texto de `hoom board`.** Siguen con el
  comando largo.
- **Firmar por la persona.** `hoom siguiente` nunca aprueba ni integra: en
  Tu aprobación y Tu aceptación dice qué comando es tuyo.
- **Juntar la rama** (`git merge`) y **publicar** (`git push`).
- **Preparar el entorno del espacio de trabajo** (`vendor/`, `.env`, las
  dependencias que git no trae). Es otra spec.
- **El piloto automático desde la terminal.** La cinta sigue siendo del
  Studio.
- **Descartar, abrir sesión y ver terminal** como verbos de la tarjeta. Ya
  están `hoom task discard` y `hoom cockpit --task`.
- **Nombres en inglés o alias.** Un solo nombre por verbo.
- **Dependencias nuevas.** `go.mod` y `go.sum` no cambian.

## Contratos

### Valores por rol en `hoom.yaml`

```yaml
agents:
  arquitecto:
    provider: claude
    model: opus
    effort: high
    budget_usd: 5
    timeout: 45m
  test-writer:
    effort: high
    timeout: 45m
    write: { allow: ["tests/**"] }   # lo de hoy, sin cambios
review:
  timeout: 60m                       # nuevo; el resto de review: no cambia
```

- Claves de `agents.<rol>`: `provider`, `model`, `effort`, `budget_usd`,
  `timeout` y `write`. Cualquier otra es un error de carga:
  `agents.<rol>: clave desconocida "<k>" (validas: provider, model, effort,
  budget_usd, timeout, write)`. Hoy una clave mal escrita se ignora en
  silencio; un `efort: low` ignorado es justo el error que esta spec quiere
  evitar.
- `provider`: uno de los nombres del registro de hoom (`claude`, `codex`,
  `gemini`, `opencode`). Otro nombre es un error de carga:
  `agents.<rol>.provider: "<p>" no es un provider de hoom (claude, codex,
  gemini, opencode)`. Que esté instalado se comprueba al lanzar, no al
  cargar.
- `model` y `effort`: vocabulario del provider, sin validar, como en
  `review:`.
- `budget_usd`: número mayor o igual que 0; 0 es sin tope. Un negativo es un
  error de carga.
- `timeout`: una duración de Go (`45m`, `1h30m`) entre `1m` y `12h`. Fuera de
  rango o sin unidad es un error de carga:
  `agents.<rol>.timeout: "<v>" no es una duracion entre 1m y 12h (ej. 45m,
  1h30m)`.
- `agents.reviewer` acepta solo `write`, como hoy. Con una clave de
  lanzamiento el error es `agents.reviewer: la review se configura en review:
  (provider, model, effort, timeout)`. Un solo lugar por rol.
- `review:` gana `timeout`, con las mismas reglas.
- Los valores salen del `hoom.yaml` **del proyecto**, el mismo que lee el
  Studio, nunca del `hoom.yaml` de la rama de la tarjeta. Es la regla que
  `hoom review` ya sigue para su base: la rama no se sube su propio
  presupuesto.
- Los perfiles nunca traen estos valores: son decisiones del proyecto.

### El orden de los valores

Una sola función del manifiesto, `m.RoleDefaults(rol)`, da los valores de
`hoom.yaml` de un rol, y una sola regla los combina con lo demás. La usan
`hoom agent`, los verbos nuevos y `Derive`. Para cada valor gana el primero
que existe:

1. Lo que se pide en el comando (`--provider`, `--model`, `--effort`,
   `--budget-usd`, `--timeout`) o en el diálogo del Studio.
2. `agents.<rol>` (o `review:` para el reviewer) del `hoom.yaml` del
   proyecto.
3. Lo que propone hoy cada lugar: en la tarjeta, el primer provider que
   puede y el presupuesto que le queda; en `hoom agent`, el primer provider
   instalado que recibe el contrato.
4. Lo del provider (su modelo, su esfuerzo) y, para el tiempo límite, los 30
   minutos de hoy.

El presupuesto de `hoom.yaml` nunca pasa lo que le queda a la tarjeta: con
`budget_usd: 5` y 2 USD restantes, la propuesta es 2. En la review, como hoy,
la propuesta es por lente.

### La tarjeta propone los valores

`Action` gana tres campos, que emiten `hoom board --json`,
`hoom item show --json` y `/api/board`:

- `model` y `effort`: lo que da la regla de arriba para el rol de la
  acción, o `""`.
- `timeout_s`: el tiempo límite efectivo en segundos (1800 sin nada
  declarado).

El provider con `default: true` es el de `agents.<rol>.provider` (o
`review.provider`) cuando es una opción con `ok`. Si no lo es, el de hoy: el
primero que puede. `budget_usd` propuesto aplica el tope de arriba.

### Esfuerzo y tiempo límite para todos los roles

- `hoom agent` gana `--effort <e>` y `--timeout <d>`. `agentcmd.Options`
  gana `Effort` y `Timeout` y los pasa al run.
- `runcmd.StartOptions` gana `Timeout`. Cero es el del `Manager` (30
  minutos). El sidecar del run (`.meta.json`) gana `timeout_s`, y la regla
  que decide si un run viejo sigue vivo usa el tiempo límite **de ese run**
  más 5 minutos, no el del `Manager`. Sin `timeout_s` (un run viejo) usa el
  del `Manager`, como hoy.
- Un provider sin la capacidad `effort` lo ignora con el aviso de siempre en
  el log, y con `--strict` es un error antes de crear el run.
- `POST /launch` acepta `effort` para las cuatro acciones de rol y lo pasa al
  sobre o a la review. El error `effort solo aplica a pedir-reviewer`
  desaparece.
- El diálogo de rol del Studio tiene el campo Esfuerzo para los cuatro roles,
  relleno con `effort` de la acción, y el campo Modelo relleno con `model`.
  Debajo del presupuesto dice el tiempo límite: `tiempo limite: 45 min`, o
  `tiempo limite: 30 min (el de hoom)` sin nada declarado. El Studio no lo
  edita: se cambia en `hoom.yaml`.

### El mensaje cuando se cumple el tiempo límite

Hoy dice siempre `timeout tras 30m0s (¿la CLI espera un permiso
interactivo? preconfigura sus permisos headless)`, también cuando la CLI
narró hasta el último segundo. El mensaje pasa a depender del último evento
de la narración:

- Con un evento en los 60 segundos anteriores:
  `timeout tras <d>: la CLI seguia narrando (ultimo evento hace <s>s), no
  esperaba un permiso. Accion: subi el tiempo limite (--timeout o
  agents.<rol>.timeout en hoom.yaml) o pedi menos por run`.
- Sin eventos en esos 60 segundos:
  `timeout tras <d> sin narracion desde hace <m>: la CLI puede estar
  esperando un permiso interactivo. Accion: preconfigura sus permisos
  headless`.

En un `hoom run` sin rol, la acción del primer mensaje nombra solo
`--timeout`, que `hoom run` también gana.

### El test-writer no se pide con el spec sin guardar

El test-writer trabaja en un árbol ciego que se arma desde lo guardado del
espacio de trabajo. Si el spec no está ahí, o lo guardado es distinto de lo
que hay en disco, el sobre corta en el paso 2. Desde ahora `Derive` lo ve
antes: `pedir-test-writer` (y `relanzar` con rol test-writer) está
deshabilitada con
`el spec tiene cambios sin guardar y el test-writer trabaja con lo guardado:
guarda primero`. La frase sigue la regla de los `why`: sin palabras de git.
`hoom siguiente` en ese caso sugiere `hoom guardar`.

### La tarjeta de la que se habla

Los verbos nuevos, y solo ellos, deducen la tarjeta. El orden:

1. El operando `<slug>`, si está.
2. La carpeta: si estás dentro de `<proyecto>/.hoom/worktrees/<slug>/`, a
   cualquier profundidad, la tarjeta es `<slug>`. Un árbol ciego anidado
   (`.hoom/isolated/` de ese espacio) cuenta como el espacio que lo contiene.
3. La tarjeta fijada con `hoom usar`.
4. Si no hay ninguna, exit 2 sin efectos: `no se de que tarjeta hablas:
   pasala (hoom pedir writer <slug>), entra a su espacio de trabajo o fija una
   con hoom usar <slug>`.

Cada verbo imprime primero `tarjeta: <slug> (<origen>)`, con origen
`argumento`, `carpeta` o `hoom usar`. Un slug que no es una tarjeta es exit 1:
`no hay tarjeta <slug>: hoom item list`.

**El proyecto.** Desde adentro de un espacio de trabajo, los verbos actúan
sobre el proyecto que lo contiene (el árbol principal del repositorio), el
mismo que sirve el Studio. Nunca toman el espacio de trabajo como si fuera
un proyecto.

### `hoom usar`

```
hoom usar <slug>      fija la tarjeta
hoom usar             muestra la fijada: "tarjeta: <slug>" o "sin tarjeta fijada"
hoom usar --ninguna   la quita
```

Se guarda en `.hoom/cache/tarjeta`, una línea con el slug. Es estado local:
no viaja en git y no entra en la huella. Fijar un slug que no es una tarjeta
es exit 1 y no cambia nada. Si la tarjeta fijada dejó de existir, los verbos
dicen `la tarjeta fijada <slug> ya no existe: hoom usar <otra>` (exit 1).

### `hoom pedir`

```
hoom pedir <rol> [<slug>] [-m "<pedido>"] [--provider p] [--model m]
           [--effort e] [--budget-usd x] [--timeout d] [--json]
```

- `<rol>`: `arquitecto`, `test-writer`, `writer` o `reviewer`, la acción
  `pedir-<rol>` de la tarjeta. Otro rol es exit 2: `rol desconocido "<r>"
  (arquitecto, test-writer, writer, reviewer); para otros roles: hoom
  agent`.
- Si la tarjeta no tiene esa acción: exit 1, `en <Columna> no se le pide al
  <rol>: <plain>. Siguiente: hoom siguiente`. Si la tiene deshabilitada:
  exit 1 con su `why`.
- **El pedido.** Con `-m`, ese texto. Sin `-m` y con terminal, se abre el
  editor (`$VISUAL`, después `$EDITOR`, después `vi`, o `notepad` en
  Windows) sobre un archivo temporal con el pedido que propone la acción y,
  debajo, líneas de comentario que empiezan con `#`: la tarjeta, el rol, los
  valores con su origen y `Las lineas que empiezan con # se ignoran. Si lo
  dejas vacio no se lanza nada.` Al cerrar se quitan las líneas con `#`. Un
  pedido vacío es exit 1, `pedido vacio: no se lanzo nada`. Si el editor sale
  con error: exit 1, `el editor salio con error: no se lanzo nada`. Sin `-m`
  y sin terminal, se usa el pedido propuesto tal cual.
- El reviewer arma su propio pedido: `hoom pedir reviewer -m ...` es exit 2,
  `la revision arma su propio pedido: no lleva -m`, y sin `-m` no se abre el
  editor.
- **Antes de lanzar** imprime, en este orden:
  - `valores: provider <p> (<origen>) · modelo <m> (<origen>) · esfuerzo
    <e> (<origen>) · presupuesto <x> (<origen>) · tiempo limite <d>
    (<origen>)`, con origen `comando`, `hoom.yaml`, `tarjeta` o `provider`,
    y `del provider` en lugar del valor cuando no hay ninguno.
  - `→ ` y el comando largo equivalente: `hoom agent --role <rol> --task
    <slug> --spec .hoom/specs/<slug>.md --provider ... "<pedido>"`, o
    `hoom review --task <slug> --spec ... --provider ...` para el reviewer.
    El pedido va recortado a 60 caracteres con `…`.
- **Lanzar** es la misma función que `POST /launch` (ver abajo), con la
  narración del sobre en la salida estándar, como `hoom agent`. El exit code
  y `--json` son los de `hoom agent` (o los de `hoom review` para el
  reviewer).

### La misma función para el Studio y la terminal

La lógica de `servecmd.start` (el candado de la tarjeta, la acción y sus
checks en su orden, el presupuesto, el run activo en el árbol, el log del
sobre, la tarea que se crea en Backlog) pasa a un paquete que usan los dos.
El handler queda como una piel: decodifica, llama, responde. La función
escribe siempre la narración en el log del sobre
(`.hoom/envelopes/<id>.log`), lance quien lance, así que cualquier piel puede
leer un trabajo en curso. Las diferencias son solo de piel:

- El Studio vuelve en el primer registro del sobre y el trabajo sigue solo;
  la terminal espera al final y además copia la narración a la salida
  estándar.
- La cinta (el piloto automático) corre solo después de un trabajo que lanzó
  el Studio.

**El candado de la tarjeta es de archivo.** Hoy es un mapa en memoria del
proceso del Studio, así que un `hoom pedir` en otra terminal no lo ve. Pasa
a ser un candado en `.hoom/cache/locks/` por tarjeta, que toman los dos.
El segundo que llega recibe el mismo mensaje que hoy:
`ya se esta lanzando un trabajo en esta tarjeta`.

### `hoom siguiente`

```
hoom siguiente [<slug>] [-m "<pedido>"] [--provider p] [--model m]
               [--effort e] [--budget-usd x] [--timeout d] [--json]
```

Hace la acción principal de la tarjeta (la primera de `actions`), con estas
reglas:

| La tarjeta | `hoom siguiente` |
|---|---|
| Acción principal de rol, habilitada | Lo mismo que `hoom pedir <rol>` |
| En Tu aprobación | No firma. Imprime `te toca: hoom aprobar` y el `plain`. Exit 0 |
| En Tu aceptación | No firma. Imprime `te toca: hoom integrar` y el `plain`. Exit 0 |
| Con un rol trabajando | `el <rol> esta trabajando (<stage>, paso <n> de <m>): espera a que termine`. Exit 1 |
| Interrumpida | No elige. Imprime `hoom reanudar` y `hoom pedir <rol>` (volver a lanzar). Exit 0 |
| Acción principal deshabilitada | Imprime su `why` y, si `guardar` está habilitada, `hoom guardar`. Exit 1 |
| En Hecho | `la tarjeta ya esta en Hecho`. Exit 0 |

Los flags de lanzamiento cuando la acción principal no lanza un rol son
exit 2: `<flag> solo sirve cuando hoom siguiente lanza un rol; ahora te toca
hoom <verbo>`.

### `hoom aprobar` y `hoom integrar`

```
hoom aprobar [<slug>] [--si]
hoom integrar [<slug>] [--si]
```

Son las acciones `aprobar` e `integrar` de la tarjeta: aprobar resuelve el
spec en el espacio de trabajo de la tarjeta, como el Studio, e integrar es
`hoom task done`. Son firmas, así que piden confirmación:

- Con terminal, imprimen el mismo texto que el diálogo del Studio (aprobar:
  `Vas a aprobar el spec de "<titulo>" (<n> criterios) con tu identidad:
  <firma>. Si el spec cambia despues, la aprobacion deja de valer.`) y
  preguntan `¿Seguir? [s/N] `. Solo `s` o `si` siguen; cualquier otra
  respuesta es exit 1, `no se firmo nada`.
- `--si` saltea la pregunta.
- Sin terminal y sin `--si`: exit 2 sin efectos, `sin terminal, <verbo>
  necesita --si`.
- Al terminar: `aprobado: <spec> (sha256 <8>)`, o `listo: la tarjeta pasa a
  Hecho` y `para juntarla: git merge --no-ff hoom/<slug>`.

### `hoom guardar`

`hoom guardar [<slug>]` es la acción `guardar`: commitea las rutas sin
guardar de la tarjeta (sus `paths`) con la misma función que
`hoom item save` y el Studio, y el mismo mensaje. Imprime las rutas y los
commits. Sin nada sin guardar: `no hay nada sin guardar`, exit 0.

### `hoom reanudar`

`hoom reanudar [<slug>] [-m "<pedido>"] [--model m] [--effort e]
[--budget-usd x] [--timeout d] [--json]` es la acción `reanudar`: retoma la
sesión del sobre interrumpido, con su provider (no se elige otro). Sin
`-m`, el pedido propuesto (`El trabajo anterior se corto en el paso ...`)
pasa por el editor como en `hoom pedir`. Si la acción está deshabilitada, su
`why`.

### Argumentos

Todos los verbos nuevos siguen la disciplina de `cliargs`: un flag que no
existe, un flag sin valor o un operando de más es exit 2 sin efectos, con el
bloque de uso del verbo. `-h` imprime el uso y sale con 0.

### Documentación

- `hoom help` lista los siete verbos en un bloque `La tarjeta`, con una línea
  cada uno, y dice el orden de los valores y cómo se deduce la tarjeta.
- El README suma una sección "Llevar una tarjeta desde la terminal" con el
  ciclo completo usando solo los verbos nuevos, y documenta `agents.<rol>` y
  `review.timeout`.

## Casos límite y errores esperados

- Un espacio de trabajo de una tarea sin item (como una tarea de
  borradores): `no hay tarjeta <slug>: hoom item list`.
- Dos terminales con distinta tarjeta fijada: gana el último `hoom usar`, y
  la línea `tarjeta: <slug> (hoom usar)` lo muestra antes de hacer nada.
- `$EDITOR` con argumentos (`code --wait`): se ejecuta por el shell, como
  hace git, con la ruta del archivo entre comillas.
- `hoom pedir writer` con `-m ""`: es un pedido vacío, exit 1 sin lanzar.
- Un pedido que la acción considera de relleno (`<pedido>`): el mismo 400 de
  hoy se vuelve exit 1 con el mismo texto.
- El Studio y una terminal piden a la vez sobre la misma tarjeta: uno lanza y
  el otro recibe `ya se esta lanzando un trabajo en esta tarjeta`.
- `budget_usd` en `hoom.yaml` con un provider que no acepta tope (codex): no
  se pasa, y la línea de valores dice `presupuesto sin tope (codex no acepta
  tope)`. Con `--budget-usd` explícito y ese provider, el error de hoy.
- `--timeout 0`, `--timeout 30` o `--timeout 13h`: exit 2, `--timeout: "<v>"
  no es una duracion entre 1m y 12h (ej. 45m, 1h30m)`.
- Un run que venció con `timeout: 2h`: otra instancia de hoom lo sigue dando
  por vivo mientras su proceso viva y su última narración tenga menos de 2 h
  5 min, no 35 min.
- `hoom siguiente` sobre una tarjeta en Test-writer con el spec sin guardar:
  imprime el `why` y `hoom guardar`, exit 1.
- `hoom aprobar` sobre una tarjeta que no está en Tu aprobación: exit 1, `en
  <Columna> no hay nada que aprobar: <plain>`.
- `Ctrl-C` durante `hoom pedir`: lo mismo que en `hoom agent`, se mata el
  grupo de procesos del agente y el sobre queda cerrado como cancelado.
- `hoom.yaml` con `agents.test-writer.timeout: 45` (sin unidad): error de
  carga, y ningún verbo que lee el manifiesto arranca.

## Criterios de aceptación

- CA-490: `manifest.Load` acepta en `agents.<rol>` las claves `provider`, `model`, `effort`, `budget_usd`, `timeout` y `write`, y en `review:` la clave `timeout`. Rechaza con el texto del contrato: una clave desconocida, un provider fuera del registro, un `budget_usd` negativo, un `timeout` sin unidad o fuera de `1m`..`12h`, y una clave de lanzamiento en `agents.reviewer`. Un `hoom.yaml` sin esas claves carga igual que hoy.
- CA-491: la regla de los valores aplica el orden del contrato (comando, `hoom.yaml` del proyecto, propuesta del lugar, provider) para cada valor, toma el `hoom.yaml` del proyecto aunque la rama de la tarjeta tenga otro, y nunca propone un presupuesto mayor que lo que le queda a la tarjeta (por lente en la review).
- CA-492: `Action` trae `model`, `effort` y `timeout_s` según esa regla, el provider `default` es el de `hoom.yaml` cuando es una opción con `ok` y el de hoy cuando no, y `hoom board --json`, `hoom item show --json` y `/api/board` los emiten.
- CA-493: `hoom agent` acepta `--effort` y `--timeout` y usa los valores de `agents.<rol>` cuando no se pasan; el provider recibe el esfuerzo, y un provider sin la capacidad `effort` lo ignora con aviso (con `--strict`, error antes de crear el run).
- CA-494: un run con `Timeout` corta a ese tiempo y no al del `Manager`, su sidecar trae `timeout_s`, y la regla de run vivo usa el tiempo límite del run más 5 minutos (y el del `Manager` para un sidecar sin `timeout_s`).
- CA-495: al vencer el tiempo límite, el detalle del run es el mensaje "seguia narrando" con un evento en los 60 segundos anteriores y el mensaje "sin narracion" sin él, con los textos del contrato; `hoom run` acepta `--timeout`.
- CA-496: el diálogo de rol del Studio tiene el campo Esfuerzo para las cuatro acciones de rol, relleno con `effort` de la acción, el Modelo relleno con `model` y la línea del tiempo límite; `POST /launch` pasa `effort` al sobre o a la review en las cuatro, y ya no responde `effort solo aplica a pedir-reviewer`. Re-expresa CA-409.
- CA-497: `pedir-test-writer`, y `relanzar` con rol test-writer, están deshabilitadas con el `why` del contrato cuando el spec de la tarjeta no está en lo guardado de su espacio de trabajo o lo guardado difiere del disco, y habilitadas cuando coinciden; el `why` no contiene ninguna palabra de git.
- CA-498: los verbos nuevos deducen la tarjeta en el orden del contrato (operando, carpeta, `hoom usar`), imprimen `tarjeta: <slug> (<origen>)` antes de hacer nada, y sin tarjeta salen con 2 y el texto del contrato sin efectos; un slug que no es tarjeta es exit 1.
- CA-499: desde una carpeta dentro de `.hoom/worktrees/<slug>/` (incluido un árbol ciego anidado) la tarjeta es `<slug>` y el proyecto es el árbol principal, no el espacio de trabajo.
- CA-500: `hoom usar <slug>` escribe `.hoom/cache/tarjeta`, `hoom usar` la muestra o dice `sin tarjeta fijada`, `hoom usar --ninguna` la quita, un slug inexistente es exit 1 sin cambiar nada, y una tarjeta fijada que dejó de existir da el error del contrato. El archivo no entra en la huella.
- CA-501: `hoom pedir` acepta solo los cuatro roles (exit 2 con el texto del contrato para otro), sale con 1 y el texto del contrato cuando la columna no tiene la acción, y con 1 y el `why` cuando está deshabilitada.
- CA-502: el pedido de `hoom pedir` sale de `-m`; sin `-m` y con terminal pasa por el editor con la plantilla del contrato y se quitan las líneas con `#`; un pedido vacío o un editor que falla son exit 1 sin lanzar; sin terminal se usa el pedido propuesto; `hoom pedir reviewer -m` es exit 2.
- CA-503: antes de lanzar, `hoom pedir` imprime la línea `valores:` con el origen de cada valor y la línea `→ ` con el comando largo equivalente, con el pedido recortado a 60 caracteres.
- CA-504: `hoom pedir` y `POST /launch` lanzan por la misma función: para los mismos estados de la tarjeta (sin la acción, deshabilitada, provider que no puede, presupuesto que no alcanza, run activo, pedido de relleno, Backlog sin espacio de trabajo) dan el mismo motivo, y en Backlog los dos crean la tarea. Los dos escriben el log del sobre, y `hoom pedir` además copia la narración a la salida estándar. La cinta corre solo después de un lanzamiento del Studio.
- CA-505: el candado de lanzamiento de una tarjeta es de archivo: con el Studio lanzando, un `hoom pedir` sobre la misma tarjeta desde otro proceso recibe `ya se esta lanzando un trabajo en esta tarjeta`, y al revés.
- CA-506: el exit code y la salida `--json` de `hoom pedir` son los de `hoom agent` para los roles del sobre y los de `hoom review` para el reviewer.
- CA-507: `hoom siguiente` sigue la tabla del contrato en sus siete casos, nunca aprueba ni integra, y con flags de lanzamiento cuando la acción principal no lanza un rol sale con 2 y el texto del contrato.
- CA-508: `hoom aprobar` aprueba el spec en el espacio de trabajo de la tarjeta; con terminal pregunta `¿Seguir? [s/N] ` y solo sigue con `s` o `si`; `--si` saltea la pregunta; sin terminal y sin `--si` es exit 2 sin efectos; fuera de Tu aprobación es exit 1 con el texto del contrato.
- CA-509: `hoom integrar` hace lo mismo que `hoom task done` con la misma confirmación que `hoom aprobar`, e imprime `para juntarla: git merge --no-ff hoom/<slug>`.
- CA-510: `hoom guardar` commitea las rutas sin guardar de la tarjeta con la misma función y el mismo mensaje que `hoom item save`, y sin nada que guardar dice `no hay nada sin guardar` con exit 0.
- CA-511: `hoom reanudar` retoma la sesión del sobre interrumpido con su provider, pasa el pedido por el editor sin `-m`, y con la acción deshabilitada sale con 1 y su `why`.
- CA-512: los siete verbos nuevos rechazan un flag desconocido, un flag sin valor y un operando de más con exit 2, sin efectos y con su bloque de uso, y `-h` sale con 0.
- CA-513: los verbos que ya existían conservan su sintaxis y su efecto: `hoom review` sin `--task` sigue revisando el proyecto aunque haya una tarjeta fijada, `hoom agent` sin `--spec` no exige spec, y `next` y el texto de `hoom board` no cambian.
- CA-514: `hoom help` tiene el bloque `La tarjeta` con los siete verbos y el README la sección del ciclo con los verbos nuevos y las claves nuevas de `hoom.yaml`. [verifica: go run ./cmd/hoom help | grep -q "La tarjeta" && grep -q "Llevar una tarjeta desde la terminal" README.md && grep -q "budget_usd" README.md]
- CA-515: sin dependencias nuevas: `go.mod` y `go.sum` no cambian respecto de la rama base. [verifica: git diff --quiet main -- go.mod go.sum]

## Decisiones

- **Verbos en castellano, con las palabras de los botones del Studio.**
  `pedir`, `aprobar`, `guardar`, `integrar`, `reanudar`: la misma palabra en
  el botón y en la terminal, que es lo que hace que la TUI enseñe la CLI. La
  alternativa era verbos cortos en inglés (`hoom write`, `hoom approve`),
  más parecidos a `verify` o `review`, pero con un vocabulario distinto del
  que la persona ya ve en el Studio. Los mensajes de hoom ya son en
  castellano. Pendiente de confirmar al aprobar.
- **Verbos nuevos en lugar de deducir en los viejos.** Deducir la tarjeta en
  `hoom review` cambiaría lo que hace sin `--task`, y deducir el spec en
  `hoom agent` le sumaría la exigencia de aprobación. Los verbos viejos
  quedan para scripts y para lo que los nuevos no cubren.
- **`-m` y el editor, como `git commit`.** Es el gesto que todo dev ya
  conoce: con `-m` no se abre nada, sin `-m` el editor trae la propuesta y
  un archivo vacío cancela.
- **La tarjeta fijada es estado local y siempre visible.** `hoom usar` es
  cómodo pero es estado escondido (el problema conocido del contexto de
  `kubectl`). Por eso la carpeta gana sobre `hoom usar`, y cada verbo dice de
  dónde sacó la tarjeta antes de hacer nada.
- **`hoom siguiente` no firma.** Aprobar e integrar son el humano en el
  método. El verbo que avanza solo se detiene en las dos columnas humanas y
  dice el comando.
- **Confirmación en las firmas, no en los lanzamientos.** Lanzar ya tiene su
  confirmación: el editor o el `-m`. Aprobar e integrar son de un solo gesto,
  y una tarjeta deducida mal firmaría la equivocada. Sin terminal se exige
  `--si`, así un script lo dice explícitamente.
- **Los valores salen del `hoom.yaml` del proyecto, no de la rama.** Si el
  `hoom.yaml` de la rama mandara, un agente podría subirse su propio tiempo
  límite o presupuesto en su rama. Es la misma regla que la base de la
  review.
- **`next` de la tarjeta no cambia.** Lo fijan diez criterios de C1, y el
  comando largo sigue siendo correcto. La forma corta aparece donde se lanza:
  en `hoom siguiente`, en la línea `→ ` y en la TUI.
- **El tiempo límite se edita en `hoom.yaml` y en el comando, no en el
  Studio.** El Studio lo muestra. Agregar un campo más al diálogo para un
  valor que cambia poco no paga; en `hoom.yaml` queda para todo el equipo.
- **El candado pasa a archivo.** Con dos pieles que lanzan desde dos
  procesos, el candado en memoria ya no alcanza. `hoomfs` ya tiene los
  candados de `.hoom/cache/locks/`.
- **Entre 1 minuto y 12 horas.** Menos de un minuto no deja arrancar a
  ninguna CLI; más de 12 horas es un run olvidado. Los dos extremos son
  errores con la acción, no silencios.

## Riesgos y deuda aceptada

- **Siete verbos más en el nivel de arriba.** La ayuda crece. Se acepta
  porque cada verbo es una acción que la persona ya conoce del Studio, y
  `hoom help` los agrupa en un bloque.
- **La tarjeta fijada puede apuntar a otra cosa que la que tienes en la
  cabeza.** Lo mitiga la línea `tarjeta: ... (hoom usar)` y que las firmas
  preguntan.
- **Tiempos límite más largos gastan más.** Un `timeout: 2h` sin
  presupuesto deja correr a un agente dos horas. El presupuesto sigue siendo
  el tope de gasto donde el provider lo acepta; con codex no hay tope y la
  línea de valores lo dice.
- **Mover `servecmd.start` a un paquete compartido toca el Studio.** Los
  tests de C3 tienen que pasar sin cambios, salvo CA-409 que esta spec
  re-expresa.
- **El editor por defecto varía por sistema.** `vi` en Unix y `notepad` en
  Windows. Quien no los quiera declara `$EDITOR`, como con git.
- **El mensaje del tiempo límite mira solo el último evento.** Una CLI que
  narra un latido sin hacer nada parece "trabajando". Es mejor que hoy, que
  siempre culpa a los permisos, y no se puede saber más sin leer el
  vocabulario de cada provider.
