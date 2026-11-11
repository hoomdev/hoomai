# Spec: el ciclo de review termina por regla

Estado: BORRADOR — pendiente de aprobación humana.
Tarea: `hoom task start ciclo-de-review-acotado` (rama
`hoom/ciclo-de-review-acotado`).
Origen: el análisis de consumo del 2026-11-10 (datos de `hoom costo` y de
`.hoom/findings/` de hoomai) y `.hoom/intake/rfc-agente-autonomo-y-remoto.md`.
Re-expresa: CA-424, CA-427 y CA-428 (`review-por-diferencia.md`) y CA-547
(`costo-por-spec.md`), solo en lo que este spec cambia.

## Objetivo

Una tarea simple tarda casi un día y vacía la ventana de Codex. Lo medido en
hoomai el 2026-11-10 (24 tareas con review):

- 120 rondas de review, 459 runs de Codex, mediana de 5 rondas por tarea;
  119 de las 120 rondas corrieron las 4 lentes, las delta también.
- Las rondas delta, que revisan los arreglos, dieron 355 hallazgos; las
  reviews completas, que revisan el trabajo, dieron 254. El ciclo se alimenta
  de sus propios arreglos.
- La lente readability dio 210 hallazgos: 0 high, 16 medium y 194 low.
- Repitiendo las mismas tareas con el ciclo vivo solo mientras haya un high
  abierto, habrían sido 52 rondas en vez de 120.

hoom ya tiene la mitad de la regla: con `findings.block_on: high`, un medium
o un low abierto no retiene la tarjeta. Lo que falta es lo que hace que, en
la práctica, igual se arregle todo hasta cero hallazgos:

1. Lo que no bloquea no se ve al aceptar. Para no perderlo se arregla, y
   cada arreglo pide otra ronda.
2. La severidad la pone el reviewer y nadie puede corregirla hacia arriba.
3. Todas las tarjetas pagan lo mismo, sea una pantalla o un módulo de pagos.
4. Cada delta corre las 4 lentes, aunque readability nunca dio un high.

Con este spec:

- "Bloquea" tiene una sola definición, con dos subidas: por regla (un medium
  en una ruta de riesgo) y a mano (una persona).
- Lo que no bloquea queda **pendiente**: abierto, atado a la tarjeta y a la
  vista al aceptar. Un tope evita que se acumule.
- Cada tarjeta declara su **riesgo** (`bajo`, `normal`, `alto`) y de ahí
  salen las lentes y lo que bloquea.
- La delta deja de correr readability.
- `hoom costo` muestra el reloj de la firma a la aceptación.
- El contrato del reviewer dice qué es high, medium y low.

**Regla.** El ciclo de review termina cuando nada bloquea, no cuando no
queda nada que decir. Lo que queda se ve, y lo decide una persona.

## No-goals

- Arreglar los pendientes dentro de la misma tarea. Se toman después, en una
  tarjeta normal (una "limpieza"), que pasa por el flujo de siempre.
- Que el piloto corrija hallazgos solo: es el spec `autonomia-entre-firmas`.
- Cambiar cómo trabaja el reviewer: sus herramientas, la evidencia que
  recibe o correr las lentes en paralelo.
- Cambiar el umbral de 400 líneas, o que deje de contar los tests.
- Cambiar la lista de marcas de las rutas de riesgo.
- Correr a writer y arquitecto sin la configuración personal del CLI.
- Convertir clases de hallazgos repetidos en gates.
- Un tope de pendientes por módulo o por carpeta, o un tope de low.
- Bajar a mano la severidad de un hallazgo.
- Revalidar los hallazgos y las reviews anteriores a este spec.
- Mostrar el reloj de las tareas que todavía no se aceptaron.
- Cambiar el aviso de tamaño del veredicto y de `hoom report`
  (`la review exige las 4 lentes`): no leen la tarjeta.
- Que el botón Integrar del Studio devuelva texto: la línea de pendientes ya
  está en el detalle de la tarjeta, al lado del botón.

## Contratos

### El paquete `riesgo`

`internal/riesgo` no importa otros paquetes de hoom. Lo usan `item`,
`finding`, `reviewcmd` y `boardcmd`; `finding` no puede importar `reviewcmd`,
que ya importa `finding`.

- `riesgo.Vocabulario`: `bajo`, `normal`, `alto`, en ese orden.
- `riesgo.Ruta(files []string) string`: el primer archivo que contiene una
  marca de riesgo, o vacío. Las marcas son las que hoy usa la regla de
  lentes (subcadenas de la ruta en minúsculas), sin cambios; la regla de
  lentes pasa a usar esta función.
- `riesgo.Efectivo(declarado string, files []string) (efectivo, motivo string)`:
  con `bajo` y `Ruta(files)` no vacía devuelve `normal` y el motivo
  `el cambio toca <ruta>: riesgo normal`. En cualquier otro caso devuelve el
  declarado (`normal` si viene vacío) y un motivo vacío. El riesgo sube solo
  y nunca baja solo.
- `files` son siempre los archivos del cambio entero contra la base: en
  `hoom review`, los del cambio entero también en una delta; en el tablero,
  los `changed_files` del veredicto.

### El riesgo de la tarjeta

- El item gana la clave opcional `riesgo`. Sin la clave vale `normal`.
  `item.Parse` rechaza otro valor con
  `riesgo %q fuera del vocabulario (bajo, normal, alto)`.
- `normal` es la ausencia de la clave: ningún verbo escribe `riesgo: normal`.
- `hoom item add` gana `--riesgo r`. Un valor fuera del vocabulario es un
  error de uso (exit 2, como `--tipo`) con el texto de `item.Parse`, y no
  crea el item.
- Verbo nuevo `hoom item set <slug> --riesgo r [--json]`. Cambia solo esa
  clave: las demás conservan sus valores (el archivo se reescribe como lo
  escribe hoom). No commitea. Imprime
  `riesgo de <slug>: <antes> -> <despues> - guardalo con: hoom item save <slug>`;
  con `--json`, `{"slug": ..., "antes": ..., "riesgo": ...}`. Si la tarjeta
  ya tenía ese riesgo, imprime `riesgo de <slug>: ya es <r>` y no escribe.
  - Sin `--riesgo` (en este spec, el único flag que cambia algo), o con un
    valor fuera del vocabulario: error de uso, exit 2.
  - Se niega con exit 1 y sin escribir:
    - dentro de una corrida (`HOOM_ROLE` no vacío):
      `el riesgo de una tarjeta lo cambia una persona, no un rol`;
    - si el item no existe: `no existe el item "<slug>"`;
    - si la tarjeta está en Hecho: `la tarjeta ya esta en Hecho`, el texto
      que ya usan el tablero y `hoom integrar`.
- El riesgo se lee del item del proyecto, el mismo que lee el tablero y que
  `hoom review` ya lee para los writers declarados. Una tarea sin item tiene
  riesgo `normal`.

### Las lentes según el riesgo

`reviewcmd.LentesConRiesgo(git gitx.Info, explicit, riesgo string) ([]string, string, error)`
aplica la regla con el riesgo efectivo. Con `normal` da exactamente lo que da
`reviewcmd.Lenses`, que no cambia.

| Riesgo efectivo | Lentes de la review completa | Motivo |
|---|---|---|
| `bajo` | `reliability` | `riesgo bajo: la lente dominante` |
| `normal` | las de hoy: ruta de riesgo o más de 400 líneas, las 4; si no, `reliability` | los de hoy |
| `alto` | las 4 | `riesgo alto: las 4 lentes` |

- Antes del riesgo, como hoy: `--lens` a mano manda, y un cambio vacío o de
  solo documentación no invoca review, con cualquier riesgo.
- **Delta.** La regla se aplica sobre el rango y sobre el cambio entero, con
  el mismo riesgo efectivo, y gana la más estricta, como hoy. Si el
  resultado son las 4, la delta corre `risk`, `reliability` y `resilience`,
  en ese orden, y su motivo termina en `; una delta no corre readability`.
- El registro de review, el `Result` y `--json` ganan `riesgo` (el efectivo)
  y `riesgo_declarado` (solo cuando difiere). La salida agrega la línea
  `riesgo: <efectivo>`, o `riesgo: <motivo> (declarado <declarado>)` cuando
  difieren.

### La review que el tablero exige

- Con riesgo efectivo `alto`, la review se exige siempre que el cambio tenga
  archivos y no sea solo documentación (la regla de hoy, exportada como
  `reviewcmd.SoloDocs(files []string) bool`). Con `bajo` y `normal` se exige
  con más de 400 líneas, como hoy.
- Un registro cuenta en la cadena si trae **las lentes que la regla pide
  para la tarjeta, o más**: con `normal` y `alto`, las 4 en un `completa` y
  `risk`, `reliability` y `resilience` en un `delta`; con `bajo`,
  `reliability` en los dos. El resto de la cadena no cambia (`hasta`,
  cobertura, veredicto verde, de `hasta` a HEAD nada o solo documentación).
  Donde CA-428 dice "su cadena de 4 lentes", pasa a ser esta cadena.
- Un registro anterior a este spec, sin `riesgo`, cuenta con sus lentes
  igual que uno nuevo.
- Cuando falta la review, el motivo dice qué se exige:
  - riesgo `normal`: el texto de hoy;
  - riesgo `alto`: `la review exige las 4 lentes (riesgo alto) y no hay registro de review`;
  - riesgo `bajo`: `la review exige la lente reliability (%d lineas > %d, riesgo bajo) y no hay registro de review`.
- En una tarjeta de riesgo `bajo`, los textos de la tarjeta que dicen
  `4 lentes` (el paso de review, `revision de 4 lentes registrada`,
  `falta la revision de 4 lentes`, `review de 4 lentes:` de
  `hoom item show`) dicen `la lente reliability`. Con `normal` y `alto` no
  cambian.
- La línea de tiempo marca hecho el paso de review con un registro que
  cuenta en la cadena, no solo con uno de 4 lentes.

### Qué bloquea

`finding.Bloquea(f Finding, c Contexto) (bloquea bool, motivo string)` es la
única definición. `Contexto` trae el umbral (`findings.block_on`, y `high`
cuando falta, como hoy en el tablero), las subidas válidas, el riesgo
declarado del item de cada tarea y los ids del campo `findings` de los
registros de review `completa` de cada tarea.

Un hallazgo abierto bloquea por el primer caso que se cumple:

| # | Caso | Motivo |
|---|---|---|
| 1 | su severidad llega al umbral, como hoy | `severidad <sev>` |
| 2 | tiene una subida a mano válida | `subido a mano por <autor>` |
| 3 | es medium y su `file` es una ruta de riesgo | `medium en ruta de riesgo: <file>` |
| 4 | es medium, la tarjeta es de riesgo `alto` y el hallazgo figura en una review `completa` de la tarea | `medium de la review completa <id>: riesgo alto` |

- Una severidad fuera del vocabulario bloquea con
  `severidad desconocida "<sev>"`: falla cerrado, como hoy la tarjeta
  histórica; el gate sigue dando ERROR por ella antes de preguntar.
- Un hallazgo sin tarea, o de una tarea sin item, se juzga con riesgo
  `normal`.
- Usan esta función el gate `findings_open`, la tarjeta (viva e histórica, y
  con ella `hoom item backfill`) y la cuenta de `hoom status`. El detalle
  del gate y el motivo de la tarjeta nombran el motivo de cada hallazgo que
  bloquea por los casos 2, 3 o 4. El caso 1 se ve como hoy.
- El gate con `--spec` sigue contando también los hallazgos sin tarea, como
  hoy; la tarjeta cuenta solo los de su tarea.
- `hoom finding list` muestra en cada hallazgo abierto `bloquea: <motivo>` o
  `pendiente`. Su JSON, y `GET /api/findings` con él, gana `bloquea`
  (booleano) y `motivo` en cada hallazgo.

### Subir un hallazgo a mano

- Verbo nuevo `hoom finding raise <id> --reason "<por que>"`. Escribe
  `.hoom/findings/<id>.raise.json` con `finding_id`, `reason`, `author` (la
  identidad git, como `resolve`) y `raised_at`. Imprime
  `hallazgo <id> subido: ahora bloquea`. El error de subcomando desconocido
  pasa a nombrar `(add|resolve|raise|list)`.
- Se niega con exit 1 y sin escribir, con textos al estilo de `resolve`:
  - dentro de una corrida: `un <rol> no sube hallazgos: lo hace una persona`;
  - sin `--reason` o con una razón vacía:
    `falta --reason: nadie sube un hallazgo sin decir por que`;
  - si el hallazgo no existe: `hallazgo no encontrado: <id>`;
  - si ya está cerrado:
    `el hallazgo <id> ya esta resuelto como "<as>": un hallazgo cerrado no se sube`.
- Si ya estaba subido, sale con exit 0, imprime
  `hallazgo <id> ya estaba subido` y no reescribe el archivo.
- **Lectura.** Todo lo que lee `.hoom/findings/` (la lista, el gate, el
  tablero) lee un `.raise.json` como subida, nunca como hallazgo. Una subida
  es válida si el archivo se lee, su `finding_id` es el del nombre, `reason`
  no está vacía y el hallazgo existe. Una inválida no sube nada y deja un
  aviso, como una resolución inválida:
  - `subida ilegible <archivo>`;
  - `subida invalida <archivo>: <motivo>; el hallazgo <id> no sube`;
  - `subida huerfana <archivo>: no existe el hallazgo <id>`.
- No hay verbo para bajar. Un hallazgo subido deja de bloquear cuando se
  cierra, con las reglas de siempre.
- **Piso.** Un rol, cualquiera y el refutador incluido, que crea, cambia o
  borra un `.raise.json` viola la regla `manipulacion` con el texto
  `un <rol> no sube hallazgos: lo hace una persona`. Es una sola violación
  por archivo, como la de un rol que escribe una resolución: el sobre corta
  en `scope` antes de verify y queda un hallazgo high.

### Los pendientes, a la vista

Un **pendiente** es un hallazgo abierto de la tarea de la tarjeta que no
bloquea. Un high nunca es pendiente: siempre llega al umbral.

- La tarjeta gana `pendientes`, siempre presente:
  `{"medium": n, "low": n, "ids": [...], "texto": "..."}`. Sale en
  `hoom board --json`, `hoom item show --json`, `GET /api/board` y
  `GET /api/board/{slug}`.
- `texto` es la línea `pendientes: 3 medium, 5 low (no bloquean)`, que
  nombra solo las severidades con cuenta mayor que 0, en orden medium, low.
  Está vacío cuando no hay pendientes.
- `RenderCard` y el detalle de la tarjeta en `hoom tablero` muestran `texto`
  cuando no está vacío. El Studio lo muestra en el detalle de la tarjeta
  como texto, sin interpretarlo como HTML.
- `hoom task done` (y con él `hoom integrar`) imprime, justo después de la
  línea de cierre,
  `pendientes: 3 medium, 5 low abiertos (hoom finding list --open)`, o
  `sin pendientes`.

### El tope de pendientes

- `findings.max_pendientes: <n>` en `hoom.yaml`, entero mayor o igual a 0.
  Sin la clave no hay tope. Un valor inválido da
  `findings: max_pendientes tiene que ser un entero mayor o igual a 0`. El
  error de clave desconocida pasa a nombrar las dos válidas:
  `findings: clave desconocida %q (validas: block_on, max_pendientes)`.
- `taskcmd.Ready` suma un chequeo al final, sobre el árbol de la tarea (sus
  hallazgos y su `hoom.yaml`: lo que el proyecto tendría al integrar). Si
  hay tope, la tarea tiene al menos un medium abierto propio y los medium
  abiertos del árbol, de cualquier tarea o sin tarea, son más que el tope,
  falla con
  `aceptar %q dejaria %d medium abiertos en el proyecto (tope %d): corrige o refuta alguno (hoom finding list --open) o sube findings.max_pendientes`.
- Una tarjeta sin medium abiertos propios pasa aunque el proyecto esté sobre
  el tope: es lo que deja aceptar una limpieza.
- Como todo `Ready`, el tablero muestra ese motivo en la tarjeta, con el
  texto llano `aceptarla dejaria demasiados medium abiertos en el proyecto`
  y la acción `hoom finding list --open`; `--force` lo salta.

### El reloj en `hoom costo`

- Cada tarea gana `firmada_en`: el `approved_at` más viejo entre los
  registros de `.hoom/approvals/` cuyo `spec` es `.hoom/specs/<slug>.md`, o
  `null` si no hay ninguno. Una enmienda no reinicia el reloj.
- Cada tarea gana `reloj_ms`: de `firmada_en` a `aceptada_en`, o `null` si
  falta alguno de los dos.
- La línea de la tarea agrega `reloj <duracion>` después de las rondas:
  horas totales y minutos truncados (`19h4m`, `50h0m`), o solo minutos con
  menos de una hora (`45m`). Sin dato no agrega nada.

### Los contratos de los roles

La plantilla que instala hoom (`internal/agents/assets/agents/`) y la copia
del repo (`.hoom/agents/`) siguen idénticas, sin tildes como el resto:

- `06-reviewer.md` gana este bloque:

  ```
  ## Severidad
  La decide lo que el cambio hace, no cuanto preocupa:
  - high: en un caso que el spec cubre o que un usuario puede provocar, el
    cambio da un resultado incorrecto, pierde o expone datos, se salta un
    control o se cuelga.
  - medium: falla real fuera del camino normal (un caso raro, un entorno
    degradado, una carrera improbable) sin perder ni exponer datos.
  - low: no cambia el comportamiento (nombres, comentarios, duplicacion,
    estilo).
  Si dudas entre dos, elige la mas alta y di por que.
  ```

- En la selección de lente de `06-reviewer.md`, después de la línea de
  seguridad y 400 líneas:

  ```
  - Riesgo de la tarjeta (lo declara una persona en el item): bajo -> 1
    lente, reliability, salvo que el cambio toque una ruta de riesgo;
    alto -> las 4 siempre.
  - Una delta (la review de los arreglos) no corre readability.
  ```

- En `00-orquestador.md`, la regla 4b dice
  `Mas de 400 lineas cambiadas = review con las 4 lentes, salvo riesgo bajo en la tarjeta (reliability); con riesgo alto, las 4 siempre.`
  en lugar de `... = review con las 4 lentes, sin excepciones.`
- La descripción del reviewer que hoom instala en los agentes nativos nombra
  el riesgo de la tarjeta.

### Documentación y pruebas

- El README documenta `--riesgo`, `hoom item set`, `hoom finding raise`, los
  pendientes y `findings.max_pendientes`, sin tildes como el resto de sus
  tablas.
- Los tests no llaman a ningún CLI real.
- Los tests que hoy fijan lo contrario de este spec pasan a esperar lo de
  este spec y citan el id re-expresado: que la delta corre las 4 lentes
  (CA-424), que la cadena exige las 4 en cada registro (CA-427, CA-428) y
  las claves exactas de una tarea de `hoom costo` (CA-547). Es el cambio que
  este spec aprueba, no un test debilitado.
- Si un test de un spec anterior da por hecho que un medium nunca bloquea
  con `block_on: high` y usa un archivo en ruta de riesgo, o que una
  severidad desconocida no bloquea la tarjeta viva, el writer no lo toca: lo
  reporta y decide Henry.

## Casos límite y errores esperados

- **Riesgo `bajo` y el cambio toca `internal/auth/`**: el efectivo es
  `normal`; el registro de review guarda los dos y la salida dice por qué.
- **Riesgo `bajo` y solo la delta no toca la ruta de riesgo**: el efectivo
  sigue `normal`, porque se mide sobre el cambio entero.
- **El riesgo cambia después de una review.** El tablero recalcula. De
  `bajo` a `alto`, un registro de una sola lente deja de cubrir y los medium
  de la review completa pasan a bloquear. De `alto` a `bajo`, lo que ya se
  revisó de más sigue contando.
- **Riesgo `alto` y un cambio de solo documentación**: no se exige review.
- **Un medium sin `file`**: el caso 3 no aplica.
- **Un medium en ruta de riesgo sin tarea**: bloquea en el gate y no está en
  ninguna tarjeta.
- **Un low subido a mano**: bloquea.
- **`block_on: medium`**: los medium ya bloquean por el caso 1 y los casos 3
  y 4 no agregan nada.
- **Sin `block_on`**: el gate `findings_open` no corre, como hoy, y el
  tablero sigue pidiendo declararlo y juzgando con `high`. Las subidas no
  cambian eso.
- **Un hallazgo que el reviewer marca high por algo cosmético**: bloquea. Se
  cierra refutándolo con evidencia, como hoy.
- **Una delta de solo documentación**: 0 lentes, como hoy.
- **`--lens readability` en una delta con código**: corre, y la cobertura es
  `parcial`, como cualquier lente a mano que no cubre la regla.
- **`max_pendientes: 0`**: ninguna tarjeta con un medium abierto propio se
  puede aceptar.
- **Una tarjeta en Hecho con pendientes**: sigue mostrándolos; cuentan para
  el tope del proyecto hasta que se cierren.
- **Un `.raise.json` de un hallazgo que no existe**: no sube nada, y
  `hoom finding list` y el gate lo avisan con `subida huerfana`.
- **Un `.raise.json` de otro hallazgo** (`finding_id` distinto del nombre):
  no sube ninguno de los dos y deja `subida invalida`.

## Criterios de aceptación

- CA-615: `item.Parse` acepta `riesgo: bajo`, `normal` y `alto`, da `normal` cuando falta la clave, y rechaza cualquier otro valor con `riesgo "x" fuera del vocabulario (bajo, normal, alto)`.
- CA-616: `hoom item add --riesgo alto` escribe `riesgo: alto` en el item; con `--riesgo normal` o sin el flag el archivo no lleva la clave; un valor fuera del vocabulario sale con exit 2 y el texto de `item.Parse`, sin crear el item.
- CA-617: `hoom item set <slug> --riesgo r` cambia solo esa clave, deja las demás con sus valores, no commitea e imprime la línea del contrato (con `--json`, `slug`, `antes` y `riesgo`); con `--riesgo normal` el archivo queda sin la clave; con el riesgo que ya tenía imprime `ya es` y no escribe; con `HOOM_ROLE` no vacío, con un slug sin item o con la tarjeta en Hecho sale con exit 1, el texto del contrato y sin tocar el archivo.
- CA-618: `riesgo.Ruta` devuelve el primer archivo que contiene una marca de riesgo, o vacío; `riesgo.Efectivo` devuelve `normal` con el motivo `el cambio toca <ruta>: riesgo normal` para `bajo` con una ruta de riesgo, y el declarado con motivo vacío para `bajo` sin ruta de riesgo, para `normal` y para `alto`, con o sin ruta de riesgo; con el declarado vacío devuelve `normal`.
- CA-619: `reviewcmd.LentesConRiesgo` y `hoom review` corren `reliability` con riesgo `bajo` aunque el cambio pase de 400 líneas, las 4 lentes con riesgo `alto` aunque no pase, cada uno con el motivo de la tabla, y con riesgo `normal` o sin item lo mismo que `reviewcmd.Lenses`; una tarjeta `bajo` cuyo cambio toca una ruta de riesgo corre las de `normal`.
- CA-620: una delta cuya regla da las 4 lentes corre `risk`, `reliability` y `resilience`, en ese orden y sin `readability`, con el motivo terminado en `; una delta no corre readability` y cobertura `delta`; una cuya regla da una sola lente corre esa; re-expresa CA-424 solo en que la delta ya no corre `readability`.
- CA-621: el registro de review, el `Result` y `--json` guardan `riesgo` con el efectivo y `riesgo_declarado` solo cuando difiere, y la salida de `hoom review` trae la línea `riesgo:` del contrato.
- CA-622: el tablero exige review siempre con riesgo `alto` (salvo un cambio vacío o de solo documentación) y con más de 400 líneas con `bajo` y `normal`; una cadena cubre si cada registro trae las lentes que la regla pide para la tarjeta o más, y el motivo de una review que falta es el del contrato para cada riesgo; con riesgo `bajo` los textos de la tarjeta dicen `la lente reliability` donde hoy dicen `4 lentes`; re-expresa CA-427 y CA-428 solo en las lentes que cada registro necesita.
- CA-623: `finding.Bloquea` da verdadero con `severidad <sev>` cuando la severidad llega al umbral, con `medium en ruta de riesgo: <file>` para un medium con `block_on: high` cuyo `file` es una ruta de riesgo, y con `severidad desconocida "<sev>"` para una severidad fuera del vocabulario; da falso para un medium fuera de ruta de riesgo, para un medium sin `file` y para un low.
- CA-624: con una tarjeta de riesgo `alto`, un medium que figura en un registro `completa` de la tarea bloquea con `medium de la review completa <id>: riesgo alto`, y uno que figura solo en un registro `delta` no bloquea; con riesgo `normal` o `bajo` ninguno de los dos bloquea.
- CA-625: `hoom finding raise <id> --reason "..."` escribe `.hoom/findings/<id>.raise.json` con `finding_id`, `reason`, `author` y `raised_at`; dentro de una corrida, sin razón, con un id que no existe o con un hallazgo cerrado sale con exit 1, el texto del contrato y sin escribir; sobre un hallazgo ya subido sale con exit 0, imprime `ya estaba subido` y no reescribe el archivo.
- CA-626: un hallazgo low con una subida válida bloquea con `subido a mano por <autor>`; una subida ilegible, con otro `finding_id`, sin razón o de un hallazgo que no existe no sube nada, deja su aviso y no rompe la lectura de los hallazgos (el gate no da ERROR por ella); un run de cualquier rol que crea, cambia o borra un `.raise.json` cierra su sobre cortado en `scope`, sin veredicto y con un hallazgo high de la regla `manipulacion` con el texto del contrato.
- CA-627: para un mismo conjunto de hallazgos de una tarea, subidas, riesgo y registros de review, el gate `findings_open` con el spec de la tarea falla por esos hallazgos si y solo si la tarjeta tiene hallazgos que bloquean, y los dos nombran los mismos ids; el detalle del gate y el motivo de la tarjeta traen el motivo de cada hallazgo que bloquea por una subida, por ruta de riesgo o por riesgo alto; `hoom status` cuenta como que bloquean los mismos.
- CA-628: `hoom finding list` muestra `bloquea: <motivo>` o `pendiente` en cada hallazgo abierto, y su JSON trae `bloquea` y `motivo` en cada uno, con `bloquea` falso y `motivo` vacío en los cerrados; los avisos de subidas inválidas salen en la lista y en sus `warnings`.
- CA-629: la tarjeta trae `pendientes` con la cuenta de medium y low, los ids de sus hallazgos abiertos que no bloquean y el `texto`, en `hoom board --json`, `hoom item show --json`, `GET /api/board` y `GET /api/board/{slug}`; `texto` es `pendientes: 3 medium, 5 low (no bloquean)` con solo las severidades presentes, y vacío cuando no hay ninguno; `RenderCard` muestra la línea solo cuando no está vacía.
- CA-630: el detalle de la tarjeta de `hoom tablero` y el del Studio muestran la línea de pendientes cuando la tarjeta tiene alguno, como texto, y no la muestran cuando no tiene.
- CA-631: `hoom task done` imprime, justo después de la línea de cierre, `pendientes: <cuentas> abiertos (hoom finding list --open)` cuando la tarea tiene pendientes y `sin pendientes` cuando no.
- CA-632: `hoom.yaml` acepta `findings.max_pendientes` con un entero mayor o igual a 0, rechaza un negativo o un valor que no es entero con el texto del contrato, y una clave desconocida en `findings` nombra `block_on, max_pendientes`.
- CA-633: con `max_pendientes: n` en el árbol de la tarea, `taskcmd.Ready` falla con el texto del contrato cuando la tarea tiene un medium abierto propio y el árbol tiene más de n medium abiertos; pasa cuando la tarea no tiene medium abiertos propios aunque el árbol esté sobre el tope, cuando no pasa de n, y cuando la clave falta; el tablero muestra el texto llano y la acción del contrato.
- CA-634: `hoom costo --json` trae `firmada_en` y `reloj_ms` en cada tarea, con `firmada_en` igual a la aprobación más vieja de su spec y `reloj_ms` igual a la diferencia entre esa aprobación y `aceptada_en`, y `null` cuando falta la aprobación o la aceptación; la línea de texto agrega `reloj <duracion>` en el formato del contrato solo cuando hay dato; re-expresa CA-547 solo en que cada tarea suma esas dos claves.
- CA-635: los contratos de los roles definen las tres severidades, sacan readability de la delta y nombran el riesgo de la tarjeta, en la plantilla y en la copia del repo. [verifica: grep -q "medium: falla real fuera del camino normal" internal/agents/assets/agents/06-reviewer.md && grep -q "medium: falla real fuera del camino normal" .hoom/agents/06-reviewer.md && grep -q "no corre readability" internal/agents/assets/agents/06-reviewer.md && grep -q "no corre readability" .hoom/agents/06-reviewer.md && grep -q "salvo riesgo bajo" internal/agents/assets/agents/00-orquestador.md && grep -q "salvo riesgo bajo" .hoom/agents/00-orquestador.md]
- CA-636: el README documenta el riesgo de la tarjeta, la subida a mano, los pendientes y su tope. [verifica: grep -q "hoom item set" README.md && grep -q "hoom finding raise" README.md && grep -q "max_pendientes" README.md && grep -q "pendientes" README.md]

## Decisiones

- **Solo bloquea lo que llega al umbral, y el umbral ya es high.** No se
  cambia `block_on`: se hace visible lo que deja pasar. Con high y medium
  bloqueando, la repetición sobre los datos da 94 rondas en vez de 52.
- **Los pendientes no se arreglan dentro de la tarea.** Cada arreglo es
  código nuevo que pide otra ronda, y los arreglos dieron más hallazgos que
  el trabajo original. Si no se arreglan, no entra código sin revisar.
- **La limpieza es una tarjeta normal, sin mecanismo propio.** Su spec
  nombra los hallazgos que toma; el flujo de siempre los revalida, los
  corrige y los cierra.
- **El tope va al aceptar, cuenta el árbol entero y solo los medium.**
  Aceptar es el momento en que un pendiente se vuelve deuda. "Módulo" no
  tiene una definición que hoom pueda medir. Un low no cambia el
  comportamiento: acumularlos no es un defecto que entra.
- **El tope lee el árbol de la tarea, no el checkout principal.** Es lo que
  se integra; los hallazgos de otras tareas en curso viven en sus worktrees.
- **El riesgo lo declara una persona en el item.** El item ya está en el
  piso del gate de scope: un rol no lo toca. Que el modelo elija cuánta
  revisión merece su propio trabajo es justo lo que hoom no hace.
- **`normal` es exactamente lo de hoy.** Las tarjetas que ya existen no
  cambian de exigencia; solo cambian las que declaran `bajo` o `alto`.
- **`bajo` da el carril de una lente sin tocar el umbral.** Cambiar qué
  cuenta como línea afectaría a todas las tarjetas; declararlo afecta solo a
  la que lo pide, y queda firmado.
- **El riesgo efectivo se mide sobre el cambio entero.** Una delta que no
  toca la ruta de riesgo no devuelve a `bajo` un cambio que sí la toca, y
  la review y el tablero ven los mismos archivos.
- **`alto` bloquea los medium de la review completa y no los de las delta.**
  Arregla lo que la primera mirada encontró en el trabajo, sin perseguir los
  medium que aparecen en los arreglos.
- **La subida por ruta usa la lista que ya existe**, movida a un paquete sin
  dependencias para que `finding` la use sin importar `reviewcmd`. Es gruesa
  a propósito. En hoomai no habría cambiado nada: de 443 medium, ninguno
  cayó en una ruta de riesgo.
- **Una severidad desconocida bloquea.** Una sola definición tiene que
  elegir, y la tarjeta histórica y el gate ya fallan cerrado.
- **La subida a mano no tiene bajada.** Bajar una severidad sin evidencia es
  cerrar un hallazgo por opinión; para eso ya existe refutar.
- **Una subida inválida avisa y no rompe la lectura.** Un `.raise.json`
  mal escrito por una persona no tiene que dejar el gate en ERROR; el
  hallazgo se juzga por su severidad y el aviso queda a la vista.
- **Readability sale de las delta, no de la review completa.** 0 high en
  210 hallazgos. Sigue mirando el trabajo original una vez.
- **Writer y arquitecto siguen con la configuración personal.** El contrato
  del arquitecto usa el grafo y la memoria del usuario, y no está medido
  cuánto pesan hoy.
- **Verbos en inglés dentro de `finding` e `item`**, como `add`, `resolve`
  y `save`; los flags de `item` siguen en español y los de `finding` en
  inglés (`--reason`, como `--evidence`).

## Riesgos y deuda aceptada

- **La severidad sigue siendo la opinión de un modelo.** Un high mal
  etiquetado como medium no bloquea. Lo acotan las dos subidas, el bloque de
  severidad del contrato y que los pendientes se leen al aceptar.
- **Un pendiente es un defecto conocido que entra al proyecto.** Queda
  expuesto hasta que alguien lo tome. Sin `max_pendientes` nada obliga a
  hacerlo.
- **Arreglar después cuesta más** si mientras tanto se construyó encima.
- **Una delta ya no ve problemas de legibilidad que meta un arreglo.**
  Aparecen recién en la próxima review completa de ese código.
- **Las rutas de riesgo son subcadenas** (`sign` también atrapa `design`):
  pueden subir de más. No está medido en proyectos Laravel o KMP.
- **Los números son de hoomai** y la cifra de 52 rondas es una repetición
  sobre lo que pasó, no un experimento.
- **Un hoom anterior a este spec lee un `.raise.json` como un hallazgo
  roto**: su `findings_open` da ERROR y su tablero retiene las tarjetas.
  Subir hallazgos pide que todos los que trabajan en el proyecto, el CI
  incluido, usen el binario nuevo.
- **El veredicto y `hoom report` siguen diciendo que más de 400 líneas
  exigen las 4 lentes**, también en una tarjeta de riesgo `bajo`. La
  exigencia real es la del tablero.
- **Un rol con shell podría escribir el item del proyecto desde fuera de su
  árbol.** El gate de scope mide el árbol del run; es un límite que ya
  existe para toda la evidencia del proyecto.
- **`hoom task done` sigue sin mirar la columna ni los hallazgos**; solo
  `hoom integrar` exige la acción de la tarjeta. Este spec no lo cambia.
