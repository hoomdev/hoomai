# Spec: el ciclo de review termina por regla

Estado: BORRADOR — pendiente de aprobación humana.
Tarea: `hoom task start ciclo-de-review-acotado` (rama
`hoom/ciclo-de-review-acotado`).
Origen: el análisis de consumo del 2026-11-10 (datos de `hoom costo` y de
`.hoom/findings/` de hoomai) y `.hoom/intake/rfc-agente-autonomo-y-remoto.md`.
Re-expresa: CA-424 y CA-427 (`review-por-diferencia.md`), solo en lo que este
spec cambia.

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
- Correr a writer y arquitecto sin la configuración personal del CLI.
- Convertir clases de hallazgos repetidos en gates.
- Un tope de pendientes por módulo o por carpeta.
- Bajar a mano la severidad de un hallazgo.
- Revalidar los hallazgos y las reviews anteriores a este spec.
- Mostrar el reloj de las tareas que todavía no se aceptaron.

## Contratos

### El riesgo de la tarjeta

- El item gana la clave `riesgo`: `bajo`, `normal` o `alto`. Sin la clave
  vale `normal`. Otro valor invalida el item con
  `riesgo %q fuera del vocabulario (bajo, normal, alto)`.
- `hoom item add` gana `--riesgo r`. Con `normal` o sin el flag, el archivo
  no lleva la clave.
- Verbo nuevo `hoom item set <slug> --riesgo r [--json]`. Cambia solo esa
  clave y deja el resto del item igual. No commitea. Imprime
  `riesgo de <slug>: <antes> -> <despues> - guardalo con: hoom item save <slug>`.
  Se niega con exit 1 y sin escribir:
  - dentro de una corrida (`HOOM_ROLE` no vacío):
    `el riesgo de una tarjeta lo cambia una persona, no un rol`;
  - si el item no existe: `no hay item <slug>`;
  - si la tarjeta está en Hecho: `la tarjeta <slug> ya esta en Hecho`.
- El riesgo se lee del item del proyecto, el mismo que lee el tablero. Una
  tarea sin item tiene riesgo `normal`.
- **Riesgo efectivo.**
  `reviewcmd.RiesgoEfectivo(declarado string, files []string) (riesgo, motivo string)`:
  con `bajo` y una ruta de riesgo entre los archivos cambiados devuelve
  `normal` y el motivo `el cambio toca <ruta>: riesgo normal`. En cualquier
  otro caso devuelve el declarado y un motivo vacío. El riesgo sube solo y
  nunca baja solo.
- Las rutas de riesgo son la lista que ya usa la regla de lentes, sin
  cambios. Se expone como `reviewcmd.RutaDeRiesgo(files []string) string`.

### Las lentes según el riesgo

`hoom review` aplica la regla con el riesgo efectivo de la tarjeta:

| Riesgo | Lentes de la review completa |
|---|---|
| `bajo` | `reliability` |
| `normal` | las de hoy: ruta de riesgo o más de 400 líneas, las 4; si no, `reliability` |
| `alto` | las 4 |

- Un cambio de solo documentación sigue sin invocar review, con cualquier
  riesgo. `--lens` a mano manda, como hoy.
- **Delta.** Las lentes salen como hoy (la regla sobre el rango y sobre el
  cambio entero, gana la más estricta). Si el resultado son las 4, la delta
  corre `risk`, `reliability` y `resilience`, sin `readability`.
- El registro de review gana `riesgo` (el efectivo) y `riesgo_declarado`
  (solo cuando difiere). La salida muestra el motivo junto al de las lentes.

### La review que el tablero exige

- Con riesgo `alto`, la review se exige siempre que el cambio no sea solo
  documentación. Con `bajo` y `normal` se exige con más de 400 líneas, como
  hoy.
- Un registro cuenta en la cadena si trae **las lentes que la regla pide
  para esa tarjeta, o más**: en un `completa`, las de la tabla; en un
  `delta`, esas mismas sin `readability`. El resto de la cadena no cambia
  (`hasta`, cobertura, veredicto verde, de `hasta` a HEAD nada o solo
  documentación).
- Un registro anterior a este spec, sin `riesgo`, cuenta con sus lentes
  igual que uno nuevo.
- Cuando falta la review, el motivo dice qué se exige:
  - riesgo `normal`: el texto de hoy;
  - riesgo `alto`: `la review exige las 4 lentes (riesgo alto) y no hay registro de review`;
  - riesgo `bajo`: `la review exige la lente reliability (%d lineas > %d, riesgo bajo) y no hay registro de review`.

### Qué bloquea

`finding.Bloquea(f Finding, c Contexto) (bloquea bool, motivo string)` es la
única definición. `Contexto` trae el umbral (`findings.block_on`), las
subidas válidas, el riesgo de la tarjeta de cada tarea y los ids que figuran
en un registro de review `completa` de esa tarea.

Un hallazgo abierto bloquea por el primer caso que se cumple:

| # | Caso | Motivo |
|---|---|---|
| 1 | su severidad llega al umbral, como hoy | `severidad <sev>` |
| 2 | tiene una subida a mano válida | `subido a mano por <autor>` |
| 3 | es medium y su `file` es una ruta de riesgo | `medium en ruta de riesgo: <file>` |
| 4 | es medium, la tarjeta es de riesgo `alto` y el hallazgo figura en una review `completa` de la tarea | `medium de la review completa <id>: riesgo alto` |

- Un hallazgo sin tarea, o de una tarea sin item, se juzga con riesgo
  `normal`.
- El gate `findings_open`, el tablero y `hoom item backfill` usan esta
  función. El detalle del gate y el motivo de la tarjeta nombran el motivo
  de cada hallazgo que bloquea por los casos 2, 3 o 4. El caso 1 se ve como
  hoy.
- `hoom finding list` muestra en cada hallazgo abierto `bloquea: <motivo>` o
  `pendiente`. Su JSON gana `bloquea` (booleano) y `motivo`.

### Subir un hallazgo a mano

- Verbo nuevo `hoom finding raise <id> --reason "<por que>"`. Escribe
  `.hoom/findings/<id>.raise.json` con `finding_id`, `reason`, `author` (la
  identidad git) y `raised_at`. Imprime
  `hallazgo <id> subido: ahora bloquea`.
- Se niega con exit 1 y sin escribir:
  - dentro de una corrida: `un rol no sube hallazgos: lo hace una persona`;
  - sin `--reason` o con una razón vacía:
    `subir un hallazgo SIN razon esta prohibido`;
  - si el hallazgo no existe: `no existe el hallazgo <id>`;
  - si ya está cerrado: `el hallazgo <id> ya esta cerrado`.
- Si ya estaba subido, sale con exit 0, dice `ya estaba subido` y no
  reescribe el archivo.
- Una subida es válida si el archivo se lee, su `finding_id` es el del
  nombre y `reason` no está vacía. Una subida inválida no sube nada.
- No hay verbo para bajar. Un hallazgo subido deja de bloquear cuando se
  cierra, con las reglas de siempre.
- **Piso.** Un rol que crea, cambia o borra un `.raise.json` viola la regla
  `manipulacion`: el sobre corta antes de verify y queda un hallazgo high
  con `un <rol> no sube hallazgos: lo hace una persona`.

### Los pendientes, a la vista

Un **pendiente** es un hallazgo abierto de la tarjeta que no bloquea.

- La tarjeta gana `pendientes`:
  `{"high": n, "medium": n, "low": n, "ids": [...]}`. Sale en
  `hoom board --json`, `hoom item show --json` y `GET /api/board`.
- `RenderCard` agrega una línea cuando hay alguno:
  `pendientes: 3 medium, 5 low (no bloquean)`. Nombra solo las severidades
  con cuenta mayor que 0, en orden high, medium, low.
- El detalle de la tarjeta en `hoom tablero` y en el Studio muestra esa
  misma línea, pintada como texto.
- `hoom task done` (y con él `hoom integrar` y el botón Integrar) agrega una
  línea después de la de cierre:
  `pendientes: 3 medium, 5 low abiertos (hoom finding list --open)`, o
  `sin pendientes`.

### El tope de pendientes

- `findings.max_pendientes: <n>` en `hoom.yaml`, entero mayor o igual a 0.
  Sin la clave no hay tope. Un valor inválido da
  `findings: max_pendientes tiene que ser un entero mayor o igual a 0`. El
  error de clave desconocida pasa a nombrar las dos válidas:
  `findings: clave desconocida %q (validas: block_on, max_pendientes)`.
- `taskcmd.Ready` suma un chequeo al final: si hay tope, la tarea tiene al
  menos un medium abierto propio y los medium abiertos de todo el proyecto
  son más que el tope, falla con
  `aceptar %q dejaria %d medium abiertos en el proyecto (tope %d): corrige o refuta alguno (hoom finding list --open) o sube findings.max_pendientes`.
- Una tarjeta sin medium abiertos propios pasa aunque el proyecto esté sobre
  el tope: es lo que deja aceptar una limpieza.
- Como todo `Ready`, el tablero muestra ese motivo en la tarjeta y
  `--force` lo salta.

### El reloj en `hoom costo`

- Cada tarea aceptada gana `firmada_en` (el `approved_at` más viejo entre
  las aprobaciones de su spec) y `reloj_ms` (de `firmada_en` a
  `aceptada_en`). Si falta alguno de los dos datos, `reloj_ms` es `null`.
- La línea de la tarea agrega `reloj 19h4m` después de las rondas. Sin dato
  no agrega nada.

### El contrato del reviewer

La plantilla que instala hoom
(`internal/agents/assets/agents/06-reviewer.md`) y la copia del repo
(`.hoom/agents/06-reviewer.md`) ganan este bloque, sin tildes como el resto
del contrato:

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

### Documentación y pruebas

- El README documenta `--riesgo`, `hoom item set`, `hoom finding raise`, los
  pendientes y `findings.max_pendientes`, sin tildes como el resto de sus
  tablas.
- Los tests no llaman a ningún CLI real.
- Los tests que hoy fijan lo contrario de este spec (que la delta corre las
  4 lentes y que la cadena exige las 4 en cada registro) pasan a esperar lo
  de este spec y citan los dos ids re-expresados. Es el cambio que este spec
  aprueba, no un test debilitado.
- Si un test de un spec anterior da por hecho que un medium nunca bloquea
  con `block_on: high` y usa un archivo en ruta de riesgo, el writer no lo
  toca: lo reporta y decide Henry.

## Casos límite y errores esperados

- **Riesgo `bajo` y el cambio toca `internal/auth/`**: el efectivo es
  `normal`; el registro de review guarda los dos y la salida dice por qué.
- **El riesgo cambia después de una review.** El tablero recalcula. De
  `bajo` a `alto`, un registro de una sola lente deja de cubrir y los medium
  de la review completa pasan a bloquear. De `alto` a `bajo`, lo que ya se
  revisó de más sigue contando.
- **Un medium sin `file`**: el caso 3 no aplica.
- **Un low subido a mano**: bloquea.
- **`block_on: medium`**: los medium ya bloquean por el caso 1 y los casos 3
  y 4 no agregan nada.
- **Sin `block_on`**: el gate `findings_open` no corre, como hoy, y el
  tablero sigue pidiendo declararlo. Las subidas no cambian eso.
- **Un hallazgo que el reviewer marca high por algo cosmético**: bloquea. Se
  cierra refutándolo con evidencia, como hoy.
- **Una delta de solo documentación**: 0 lentes, como hoy.
- **`--lens readability` en una delta**: corre, y la cobertura es `parcial`,
  como cualquier lente a mano que no cubre la regla.
- **`max_pendientes: 0`**: ninguna tarjeta con un medium abierto propio se
  puede aceptar.
- **Una tarjeta en Hecho con pendientes**: sigue mostrándolos; cuentan para
  el tope del proyecto hasta que se cierren.
- **Un `.raise.json` de un hallazgo que no existe**: no sube nada, y
  `hoom finding list` lo avisa como hace con una resolución huérfana.

## Criterios de aceptación

- CA-615: `item.Parse` acepta `riesgo: bajo`, `normal` y `alto`, da `normal` cuando falta la clave, y rechaza cualquier otro valor con `riesgo "x" fuera del vocabulario (bajo, normal, alto)`.
- CA-616: `hoom item add --riesgo alto` escribe `riesgo: alto` en el item; con `--riesgo normal` o sin el flag el archivo no lleva la clave; un valor fuera del vocabulario sale con exit 1 sin crear el item.
- CA-617: `hoom item set <slug> --riesgo r` cambia solo esa clave, deja las demás iguales, no commitea e imprime la línea del contrato; con `HOOM_ROLE` no vacío, con un slug sin item o con la tarjeta en Hecho sale con exit 1, el texto del contrato y sin tocar el archivo.
- CA-618: `reviewcmd.RiesgoEfectivo` devuelve `normal` con el motivo `el cambio toca <ruta>: riesgo normal` para `bajo` con una ruta de riesgo, y el declarado con motivo vacío para `bajo` sin ruta de riesgo, para `normal` y para `alto`, con o sin ruta de riesgo.
- CA-619: una review completa corre `reliability` con riesgo `bajo` aunque el cambio pase de 400 líneas, las 4 lentes con riesgo `alto` aunque no pase, y con riesgo `normal` o sin item las mismas lentes que hoy; una tarjeta `bajo` cuyo cambio toca una ruta de riesgo corre las de `normal`.
- CA-620: una delta cuya regla da las 4 lentes corre `risk`, `reliability` y `resilience`, en ese orden y sin `readability`, y su cobertura es `delta`; una cuya regla da una sola lente corre esa; re-expresa CA-424 solo en que la delta ya no corre `readability`.
- CA-621: el registro de review guarda `riesgo` con el efectivo, guarda `riesgo_declarado` solo cuando difiere, y la salida de `hoom review` muestra el motivo del riesgo efectivo.
- CA-622: el tablero exige review siempre con riesgo `alto` y con más de 400 líneas con `bajo` y `normal`; una cadena cubre si cada registro trae las lentes que la regla pide para la tarjeta o más (un `completa` las de la tabla, un `delta` esas sin `readability`), y el motivo de una review que falta es el del contrato para cada riesgo; re-expresa CA-427 solo en las lentes que cada registro necesita.
- CA-623: `finding.Bloquea` da verdadero con `severidad <sev>` cuando la severidad llega al umbral, y con `medium en ruta de riesgo: <file>` para un medium con `block_on: high` cuyo `file` es una ruta de riesgo; da falso para un medium fuera de ruta de riesgo, para un medium sin `file` y para un low.
- CA-624: con una tarjeta de riesgo `alto`, un medium que figura en un registro `completa` de la tarea bloquea con `medium de la review completa <id>: riesgo alto`, y uno que figura solo en un registro `delta` no bloquea; con riesgo `normal` o `bajo` ninguno de los dos bloquea.
- CA-625: `hoom finding raise <id> --reason "..."` escribe `.hoom/findings/<id>.raise.json` con `finding_id`, `reason`, `author` y `raised_at`; dentro de una corrida, sin razón, con un id que no existe o con un hallazgo cerrado sale con exit 1, el texto del contrato y sin escribir; sobre un hallazgo ya subido sale con exit 0 sin reescribir el archivo.
- CA-626: un hallazgo low con una subida válida bloquea con `subido a mano por <autor>`, y con una subida ilegible, con otro `finding_id` o sin razón no bloquea; un run de cualquier rol que crea, cambia o borra un `.raise.json` cierra su sobre cortado en `scope`, sin veredicto y con un hallazgo high de la regla `manipulacion`.
- CA-627: para un mismo conjunto de hallazgos, subidas, riesgo y registros de review, el gate `findings_open` falla si y solo si la tarjeta del tablero tiene hallazgos que bloquean, y los dos nombran los mismos ids; el detalle del gate y el motivo de la tarjeta traen el motivo de cada hallazgo que bloquea por una subida, por ruta de riesgo o por riesgo alto.
- CA-628: `hoom finding list` muestra `bloquea: <motivo>` o `pendiente` en cada hallazgo abierto, y su JSON trae `bloquea` y `motivo` en cada uno, con `bloquea` falso y `motivo` vacío en los cerrados.
- CA-629: la tarjeta trae `pendientes` con la cuenta por severidad y los ids de sus hallazgos abiertos que no bloquean, en `hoom board --json`, `hoom item show --json` y `GET /api/board`; `RenderCard` agrega `pendientes: 3 medium, 5 low (no bloquean)` con solo las severidades presentes, y no agrega la línea cuando no hay ninguno.
- CA-630: el detalle de la tarjeta de `hoom tablero` y el del Studio muestran la línea de pendientes de `RenderCard` cuando la tarjeta tiene alguno, y no la muestran cuando no tiene.
- CA-631: `hoom task done` imprime, después de la línea de cierre, `pendientes: <cuentas> abiertos (hoom finding list --open)` cuando la tarea tiene pendientes y `sin pendientes` cuando no.
- CA-632: `hoom.yaml` acepta `findings.max_pendientes` con un entero mayor o igual a 0, rechaza un negativo o un valor que no es entero con el texto del contrato, y una clave desconocida en `findings` nombra `block_on, max_pendientes`.
- CA-633: con `max_pendientes: n`, `taskcmd.Ready` falla con el texto del contrato cuando la tarea tiene un medium abierto propio y el proyecto tiene más de n medium abiertos; pasa cuando la tarea no tiene medium abiertos propios aunque el proyecto esté sobre el tope, cuando el proyecto no pasa de n, y cuando la clave falta.
- CA-634: `hoom costo --json` trae `firmada_en` y `reloj_ms` en cada tarea aceptada que tiene una aprobación de su spec, con `reloj_ms` igual a la diferencia entre la aprobación más vieja y `aceptada_en`, y `reloj_ms` nulo cuando falta la aprobación o la aceptación; la línea de texto agrega `reloj <duracion>` solo cuando hay dato.
- CA-635: el contrato del reviewer define las tres severidades, en la plantilla y en la copia del repo. [verifica: grep -q "medium: falla real fuera del camino normal" internal/agents/assets/agents/06-reviewer.md && grep -q "medium: falla real fuera del camino normal" .hoom/agents/06-reviewer.md]
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
- **El tope va al aceptar y cuenta el proyecto entero.** Aceptar es el
  momento en que un pendiente se vuelve deuda. "Módulo" no tiene una
  definición que hoom pueda medir.
- **El riesgo lo declara una persona en el item.** El item ya está en el
  piso del gate de scope: un rol no lo toca. Que el modelo elija cuánta
  revisión merece su propio trabajo es justo lo que hoom no hace.
- **`normal` es exactamente lo de hoy.** Las tarjetas que ya existen no
  cambian de exigencia; solo cambian las que declaran `bajo` o `alto`.
- **`bajo` da el carril de una lente sin tocar el umbral.** El spec
  `umbral-review-produccion` buscaba lo mismo cambiando qué cuenta como
  línea; declararlo es más simple y queda firmado.
- **`alto` bloquea los medium de la review completa y no los de las delta.**
  Arregla lo que la primera mirada encontró en el trabajo, sin perseguir los
  medium que aparecen en los arreglos.
- **La subida por ruta usa la lista que ya existe.** Es gruesa a propósito.
  En hoomai no habría cambiado nada: de 443 medium, ninguno cayó en una ruta
  de riesgo.
- **La subida a mano no tiene bajada.** Bajar una severidad sin evidencia es
  cerrar un hallazgo por opinión; para eso ya existe refutar.
- **Readability sale de las delta, no de la review completa.** 0 high en
  210 hallazgos. Sigue mirando el trabajo original una vez.
- **Writer y arquitecto siguen con la configuración personal.** El contrato
  del arquitecto usa el grafo y la memoria del usuario, y no está medido
  cuánto pesan hoy.
- **Verbos en inglés dentro de `finding` e `item`**, como `add`, `resolve`
  y `save`; los flags de `item` siguen en español.

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
- **Un rol con shell podría escribir el item del proyecto desde fuera de su
  árbol.** El gate de scope mide el árbol del run; es un límite que ya
  existe para toda la evidencia del proyecto.
- **`hoom task done` sigue sin mirar la columna ni los hallazgos**; solo
  `hoom integrar` exige la acción de la tarjeta. Este spec no lo cambia.
