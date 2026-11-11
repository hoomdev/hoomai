# Spec: la tarjeta corre sola entre tu firma y tu aceptación

Estado: BORRADOR — pendiente de aprobación humana.
Tarea: `hoom task start autonomia-entre-firmas` (rama
`hoom/autonomia-entre-firmas`).
Origen: la Parte A de `.hoom/intake/rfc-agente-autonomo-y-remoto.md` y el
análisis de consumo del 2026-11-10.
Depende de: `ciclo-de-review-acotado.md` (su definición de qué bloquea y su
verbo `hoom item set`).
Re-expresa: CA-379 y CA-380 (`historia-doctor-y-cinta.md`), solo en lo que
este spec cambia.

## Objetivo

Henry entra dos veces: firma la spec y acepta la tarjeta. Entre esos dos
momentos el trabajo tiene que correr solo, sin que hoom empiece a creerle al
modelo. Cada decisión cae en uno de tres grupos:

| Decisión | Quién decide |
|---|---|
| **Hacer**: cómo implementar, qué test escribir, cómo arreglar | el modelo |
| **Avanzar**: qué sigue, si se reintenta, cuándo parar | hoom, con reglas escritas |
| **Aceptar**: qué se construye y qué entra al proyecto | la persona |

Hoy el piloto (`auto: hasta-humano`) no encadena nada. Entre los 140
registros de sobres que hay en disco de hoomai, oceanbuild, rapivol y
agendaues no hay uno solo lanzado por el piloto, ni uno del writer. Las
causas, leídas en el código y en la spec de la cinta:

1. **Firmar no lanza nada.** Después de firmar hay que pedir el test-writer
   a mano.
2. **El test-writer cierra en rojo por diseño.** Sus tests fallan porque
   todavía no hay implementación, y la cinta se detiene ante cualquier rojo.
3. **Nadie guarda el trabajo entre roles.** La review se niega con el árbol
   sucio.
4. **La cinta no puede pedirle la review a Codex.** Exige un provider con
   tope en USD y Codex no tiene.
5. **No reintenta ni corrige hallazgos.** Un rojo del writer o un high de la
   review la detienen.
6. **Vive solo dentro de `hoom serve`.** Desde la terminal no hay cinta.

Con este spec, una tarjeta con piloto recorre sola el camino de la firma a
Tu aceptación:

- La firma lanza al test-writer, y su rojo esperado deja pasar al writer.
- El piloto guarda el trabajo antes de cada paso.
- Un rojo del writer se reintenta en una sesión nueva, con tope.
- Los hallazgos que bloquean se corrigen por rondas: refutador, test que
  falla, arreglo, cierre por evidencia y review de la diferencia.
- Los topes dejan de depender de USD, así Codex puede revisar.
- El mismo piloto corre en el Studio y en la terminal (`hoom piloto`).
- El writer no puede tocar los tests del test-writer: con reintentos,
  aflojar el test sería la salida más barata.

**Regla.** El modelo hace; hoom decide si se avanza mirando el disco; firmar
y aceptar son de una persona.

## No-goals

- Automatizar la firma de la spec o la aceptación.
- El servicio que vive solo (launchd, systemd), la cola por ventana de
  cuota, la entrega en PR borrador y los avisos. Son specs aparte.
- El intento con **otra** CLI de la escalera del documento fuente. Aquí el
  reintento es la misma CLI en una sesión nueva.
- Los niveles numerados 0 a 4 del documento fuente: los reemplazan dos topes
  independientes.
- Cortar porque fallan "los mismos tests por nombre": hoom no ve el
  resultado test por test.
- Varias tarjetas en paralelo, o una cola entre tarjetas.
- Que el piloto abra la tarea, cree tarjetas o pida al arquitecto.
- El interruptor del piloto en el Studio y mostrar regla, intento y ronda en
  la historia de la tarjeta.
- Que la cinta siga sola después de un `hoom pedir` de la terminal: para eso
  está `hoom piloto`.
- Sesiones vencidas de las CLI, Antigravity, contenedores y el remoto.

## Contratos

### Los tests del test-writer no los toca el writer

- El registro del sobre gana `trasplantados`: las rutas que el sobre del
  test-writer trasplantó al árbol de la tarea, ordenadas y relativas a ese
  árbol. Se escribe cuando el trasplante ocurre, cierre como cierre el
  sobre.
- Los **tests protegidos** de una tarea son la unión de `trasplantados` de
  todos sus sobres de test-writer.
- En el gate de scope, un rol de forma `codigo` que cambia, borra o renombra
  un test protegido viola la regla nueva `tests-del-test-writer`. Corta como
  `manipulacion`: el sobre cierra en `scope`, `no-entregable`, exit 1, sin
  verify, con la nota
  `el writer toco los tests del test-writer: no se emite veredicto sobre este arbol`.
  Queda un hallazgo high con la descripción
  `tests-del-test-writer: el rol <rol> escribio <ruta> - los tests del test-writer no los cambia el writer: pide de nuevo al test-writer`.
- No es violación crear tests nuevos, cambiar un test que no está protegido,
  ni que un sobre de test-writer cambie los suyos.
- `agents.writer.write` de `hoom.yaml` no levanta la regla.
- Una tarea cuyos sobres de test-writer no traen `trasplantados` (los de
  antes de este spec) no protege nada.

### La política `autonomia`

Sección nueva y opcional de `hoom.yaml`:

```yaml
autonomia:
  reintentos_ante_rojo: 1   # 0 o 1
  rondas_de_hallazgos: 2    # de 0 a 5
  ventana_max: 80           # de 1 a 100: porcentaje de la ventana de 5 h
```

- Sin la sección, o sin una clave, los dos contadores valen 0 y
  `ventana_max` no está declarada.
- Errores, todos como `hoom.yaml invalido: ...`:
  - `autonomia: clave desconocida %q (validas: reintentos_ante_rojo, rondas_de_hallazgos, ventana_max)`;
  - `autonomia: reintentos_ante_rojo tiene que ser 0 o 1`;
  - `autonomia: rondas_de_hallazgos tiene que ser un entero de 0 a 5`;
  - `autonomia: ventana_max tiene que ser un entero de 1 a 100`.
- `hoom.yaml` ya está en el piso del gate de scope: ningún rol la cambia.
- El tiempo por trabajo ya existe: `agents.<rol>.timeout` (30 minutos por
  defecto).
- `hoom item set <slug>` gana `--auto hasta-humano|no` y
  `--presupuesto-usd x`. `--auto hasta-humano` sobre un item sin presupuesto
  se niega con el texto de hoy
  (`--auto necesita --presupuesto-usd: el piloto automatico no corre sin tope`).
  `--auto no` quita la clave.

### El piloto: una función, dos lugares

- `boardcmd.Piloto(e Estado) PilotDecision` reemplaza a `Pilot`. Es pura: no
  lee el disco ni el reloj. `Estado` trae, ya leído, lo que la decisión
  necesita: la tarjeta derivada, sus sobres cerrados del más viejo al más
  nuevo, los veredictos de esos sobres y el último de la tarjeta, sus
  registros de review, qué hallazgos que bloquean cita cada test protegido,
  la política, el último dato de la ventana de 5 h por provider y la hora.
- `PilotDecision` gana `Paso` (`lanzar`, `cerrar-hallazgos` o `detenerse`),
  `Regla`, `Intento`, `Ronda`, `Resume`, `Delta` y `Hallazgos`. `Launch` es
  verdadero solo con `lanzar`.
- **Nunca firma ni acepta.** Para ningún `Estado` la decisión es aprobar la
  spec o integrar.
- **Quien la ejecuta** hace siempre lo mismo: arma el `Estado`, llama a
  `Piloto`, ejecuta el paso y vuelve a empezar, hasta un `detenerse`.
  - **En el Studio**: al cerrar un trabajo que lanzó el Studio, como hoy, y
    además al aprobar la spec desde la tarjeta (`approveCard`). La respuesta
    de aprobar gana `piloto`, con la línea de la primera decisión.
  - **En la terminal**: verbo nuevo `hoom piloto <slug> [--json]`. Corre en
    primer plano, imprime una línea por decisión y termina con exit 0 si se
    detuvo en una columna humana o en Hecho, y con exit 1 en cualquier otra
    detención. Si otro proceso ya corre el piloto de esa tarjeta, sale con
    exit 1 y `el piloto de <slug> ya corre en otro proceso`.
- `hoom aprobar` y `hoom spec approve` no lanzan nada. Si la tarjeta de esa
  spec tiene piloto, agregan la línea
  `la tarjeta tiene piloto automatico: sigue con: hoom piloto <slug>`.
- **Guardar.** Antes de cada lanzamiento, si el espacio de trabajo de la
  tarjeta tiene cambios sin commitear, quien ejecuta los commitea con el
  mensaje de siempre (`hoom: guardar la tarjeta <slug>`). Solo en la rama de
  la tarea: nunca en el árbol del proyecto y nunca hace push.

### La tabla del piloto

Decide el primer caso que se cumple. Los casos que dicen "como hoy"
conservan su `Why` exacto.

| # | Caso | Decisión |
|---|---|---|
| 1 | `auto` no es `hasta-humano` | se detiene, como hoy |
| 2 | la tarjeta tiene un trabajo en curso o interrumpido | se detiene, como hoy |
| 3 | la tarjeta está en una columna humana o en Hecho | se detiene, como hoy |
| 4 | la tarjeta no tiene tarea abierta | se detiene: `el piloto corre en el espacio de trabajo de la tarjeta: abre la tarea con hoom task start <slug>` |
| 5 | el último sobre cerró con `corte: tiempo` | ver "Corte por tiempo" |
| 6 | `reintentos_ante_rojo` es 1 y el último sobre es del writer, fuera de ronda, cerrado en `verify` o `check` con veredicto rojo | ver "Reintento ante un rojo" |
| 7 | `rondas_de_hallazgos` es mayor que 0 y, o bien el último sobre es del reviewer y hay hallazgos que bloquean, o bien el último sobre lleva `ronda` | ver "La ronda de hallazgos" |
| 8 | el último sobre no cerró `entregable` y no es un rojo esperado | se detiene, como el caso 2 de hoy |
| 9 | la columna de la tarjeta no está después de la del rol del último sobre | se detiene, como el caso 6 de hoy |
| 10 | la acción principal no es la de la tabla de columnas, su rol es el del último sobre, o está deshabilitada | se detiene, como los casos 7, 8 y 9 de hoy |
| 11 | el item no tiene `presupuesto_usd` | se detiene, como hoy |
| 12 | ningún provider de la acción está `ok` y tiene tope | se detiene, como hoy |
| 13 | la ventana de 5 h del provider elegido llegó a `ventana_max` | se detiene (ver "Los topes") |
| — | ninguno | lanza la acción de la tabla de columnas, regla `cadena` |

- Todo lanzamiento, salga del caso que salga, pasa además por los casos 11,
  12 y 13.
- Sin sobres cerrados (una spec escrita a mano y recién firmada), los casos
  5 a 10 no aplican.
- En la regla `cadena`, el provider y el presupuesto se eligen como hoy. La
  review se pide con `Delta` cuando la tarjeta ya tiene una review que se
  puede encadenar.
- **Rojo esperado del test-writer**: su sobre cerró en `verify` o `check`
  con veredicto rojo, sin violación de scope, y en ese veredicto
  `spec_lint`, `spec_trace` y `spec_approved` están en pass. Para los casos
  8 a 10 cuenta como entregable, así que sigue el writer.

### Reintento ante un rojo

Aplica en el caso 6. `n` es la cantidad de sobres del writer fuera de ronda
que tiene la tarjeta. Decide el primero que se cumple:

| Condición | Decisión |
|---|---|
| un gate del veredicto dio ERROR | se detiene: `entorno: el gate <gate> no pudo correr: no se reintenta con modelos` |
| `n` es 2 o más | se detiene: `rojo final: <n> intentos del writer y el veredicto <id> sigue rojo en <gates>` |
| ninguna | lanza al writer: mismo provider, sesión nueva (sin `Resume`), `Intento: 2`, regla `reintento` |

El pedido lo arma hoom:

```
Intento 2 de <slug>. El veredicto <id> quedo ROJO en: <gates>. La salida de cada gate esta en .hoom/verdicts/<id>.json. Corrige sobre el arbol actual hasta que verify quede verde. No toques los tests del test-writer.
```

Un writer que cierra cortado en `scope` no entra en este caso: tocar los
tests no se reintenta.

### Corte por tiempo

- Cuando un run muere por su `timeout`, su sidecar y el registro de su sobre
  ganan `corte: "tiempo"`. El cierre del sobre no cambia.
- En el caso 5, `k` es la cantidad de sobres de ese rol con `corte: tiempo`
  en la tarjeta:
  - con `k` igual a 1, el piloto **releva** una vez, con la regla
    `relevo-por-tiempo`. Si el rol no es ciego ni reviewer, el provider
    sabe reanudar y el run guardó su id de sesión, lanza con `Resume` y el
    pedido
    `Te cortaste por tiempo. Continua donde quedaste y termina el pedido original.`
    Si no, repite el lanzamiento que se cortó: mismo rol, provider, regla,
    intento, ronda y pedido;
  - con `k` igual a 2 o más se detiene:
    `el <rol> se corto por tiempo dos veces: sube agents.<rol>.timeout o parte la tarea`.

### La ronda de hallazgos

Aplica en el caso 7. "Bloqueantes" son los hallazgos abiertos de la tarjeta
que bloquean, con la definición de `ciclo-de-review-acotado`. Cada ronda
tiene hasta cuatro lanzamientos y un cierre, y todos sus sobres llevan
`ronda: <n>`.

**Empezar una ronda** (el último sobre es del reviewer y hay bloqueantes).
`n` es 1 más la mayor `ronda` de los sobres de la tarjeta; `k`, los
bloqueantes de ahora; `k_antes`, los que había al empezar la ronda `n-1`.

| Condición | Decisión |
|---|---|
| `n` es mayor que `rondas_de_hallazgos` | se detiene: `se hicieron <n-1> rondas de hallazgos y quedan <k> que bloquean: te toca` |
| `n` es 2 o más y `k` no es menor que `k_antes` | se detiene: `los hallazgos que bloquean no bajan (<k_antes> -> <k>): te toca` |
| algún bloqueante no pasó todavía por un refutador de la tarjeta | lanza al refutador con esos ids, regla `ronda-refutador` |
| ninguna | sigue con el test-writer de la ronda |

**Los pasos siguientes**, según el último sobre de la ronda:

| Último sobre | Condición | Decisión |
|---|---|---|
| refutador | no quedan bloqueantes | la ronda termina: sigue la tabla desde el caso 8 |
| refutador | quedan bloqueantes | lanza al test-writer, regla `ronda-test-writer` |
| test-writer | es un rojo esperado | lanza al writer, regla `ronda-writer` |
| test-writer | su veredicto es verde | se detiene: `los tests de la ronda <n> no fallan: los hallazgos no se reproducen: te toca` |
| writer | el último veredicto de la tarjeta es verde | lanza al reviewer con `Delta`, regla `ronda-review` |
| writer | el veredicto es rojo solo por `findings_open` y algún bloqueante está citado por un test protegido de esta ronda | paso `cerrar-hallazgos` con esos ids, regla `ronda-cierre` |
| writer | el veredicto es rojo solo por `findings_open` y ningún bloqueante está citado | se detiene: `quedan hallazgos sin test en la ronda <n>: <ids>: te toca` |
| writer | el veredicto es rojo por otro gate | se detiene: `el arreglo de la ronda <n> quedo rojo en <gates>: te toca` |
| reviewer | hay bloqueantes | empieza otra ronda |
| reviewer | no hay bloqueantes | la ronda termina: sigue la tabla desde el caso 8 |

Cualquier otro cierre dentro de una ronda se detiene con el caso 8.

**El paso `cerrar-hallazgos`** lo ejecuta hoom, sin modelo:

1. Cierra como `corregido` cada hallazgo de la decisión, con autor
   `hoom piloto` y la evidencia
   `ronda <n>: el test <ruta> cita el hallazgo; veredicto <id> rojo antes del arreglo; despues, veredicto <id> con todos los demas gates en pass`.
2. Corre verify de la tarjeta, como `hoom verify --spec`.
3. Guarda el espacio de trabajo.

Un test **cita** un hallazgo cuando el texto del archivo contiene su id.

**Quién corre cada paso** sale del disco, nunca se elige otra CLI:

- refutador y reviewer: el provider del registro de review más nuevo de la
  tarjeta;
- test-writer y writer: el provider del sobre más nuevo de ese rol en la
  tarjeta.

Si falta ese sobre o ese registro, se detiene:
`la tarjeta no tiene un sobre de <rol>: el piloto no elige provider por su cuenta`.

**Los pedidos de la ronda** los arma hoom:

```
Ronda <n> de hallazgos de <slug>. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: <ids>. No edites codigo.
```

```
Ronda <n> de hallazgos de <slug>. Por cada hallazgo escribe un test que falle hoy y cita su id en el test.
<id> [<severidad>] <file>: <descripcion>
```

```
Ronda <n> de hallazgos de <slug>. Estos tests fallan y cada uno cita su hallazgo: <rutas>. Corrige el codigo hasta que pasen. No toques esos tests.
```

Ningún pedido del piloto lleva texto de la narración de un run: solo ids,
rutas, nombres de gate y el texto de los hallazgos que están en disco.

### Los topes

- **USD**: sigue igual para los providers que lo aplican. Un run sin costo
  suma 0.
- **Caso 12.** Un provider "tiene tope" si aplica un tope en USD o si
  `autonomia.ventana_max` está declarada. En el segundo caso se lanza sin
  presupuesto en USD.
- **Caso 13.** Para el provider elegido se mira el último dato de su ventana
  de 5 h entre los runs de la tarjeta cuyo reinicio todavía no pasó. Si
  llegó a `ventana_max`, se detiene:
  `la ventana de 5h de <provider> va en <x>% (tope <y>%): el piloto espera; reinicia <hora UTC>`.
  Sin dato, lanza.
- **Siempre termina.** Los lanzamientos del piloto en una tarjeta no pasan
  de `3 + reintentos_ante_rojo + 4*rondas_de_hallazgos + 4`: la cadena, el
  reintento, cuatro por ronda y un relevo por rol.

### Lo que queda escrito

- El registro del sobre gana, además de `trasplantados` y `corte`: `regla`,
  `intento`, `ronda`, `hallazgos` (los ids que recibió ese paso) y
  `bloqueantes` (cuántos había, en el primer sobre de cada ronda). Todo
  lanzamiento del piloto lleva `piloto: true` y su `regla`.
- Líneas del piloto, en el log del último sobre cerrado de la tarjeta y en
  la salida de `hoom piloto`:
  - las tres de hoy, sin cambios;
  - con un provider sin tope en USD:
    `piloto automatico: pide al <rol> con <provider> (sin tope en USD): sobre <id>`;
  - antes de cada lanzamiento: `piloto automatico: regla <regla>`, seguida
    de ` intento <n>` o ` ronda <n>` cuando aplica;
  - `piloto automatico: guarda el trabajo de la tarjeta: <commit>`;
  - `piloto automatico: cierra <n> hallazgos con su test (ronda <r>): <ids>`.

### Documentación y pruebas

- El README documenta `hoom piloto`, la sección `autonomia` y que el writer
  no toca los tests del test-writer, sin tildes como el resto de sus tablas.
- Los tests usan los CLIs falsos de siempre; ninguno llama a un CLI real.
- Los tests que hoy fijan lo contrario (los once casos de la cinta y que una
  persona encadena como mucho tres trabajos) pasan a esperar lo de este spec
  y citan los dos ids re-expresados. Es el cambio que este spec aprueba, no
  un test debilitado.

## Casos límite y errores esperados

- **Sin sección `autonomia`**: el piloto no reintenta ni hace rondas, y no
  le pide nada a un provider sin tope en USD. Sí guarda antes de lanzar,
  deja pasar el rojo esperado del test-writer y arranca con la firma.
- **El writer corrige un test protegido porque "estaba mal"**: su sobre
  cierra cortado y el piloto se detiene en el caso 8. Decide la persona:
  pedir de nuevo al test-writer o corregir la spec.
- **El test-writer deja un test con un error de lint**: el writer no puede
  tocarlo, queda en rojo y, tras el reintento, el piloto se detiene con
  `rojo final`.
- **El refutador refuta todos los bloqueantes**: no corre test-writer ni
  writer; la tarjeta se vuelve a derivar y sigue la tabla.
- **Un hallazgo que el test-writer no logra reproducir junto a otros que
  sí**: el veredicto es rojo, el writer arregla, y hoom cierra solo los que
  tienen un test que los cita. El otro queda abierto y el piloto se detiene
  con `quedan hallazgos sin test`.
- **Un bloqueante subido a mano en medio de una ronda**: cuenta desde la
  siguiente decisión.
- **La persona pide un trabajo a mano en medio de la cadena**: sus sobres
  cuentan para `n`, para `k` y para la ronda igual que los del piloto.
- **La persona apaga `auto` mientras corre un trabajo**: al cerrar, el caso
  1 detiene el piloto.
- **La tarjeta escrita fuera del sobre** (el writer trabajó en una sesión
  interactiva): no hay sobre de writer, así que la ronda se detiene con
  `la tarjeta no tiene un sobre de writer`.
- **El relevo de un rol ciego**: no se puede reanudar su sesión, así que se
  vuelve a lanzar desde cero.
- **Dos pilotos sobre la misma tarjeta** (Studio y terminal): el segundo ve
  el trabajo en curso (caso 2) o el candado de `hoom piloto`.
- **`hoom piloto` sobre una tarjeta sin `auto`**: imprime el `Why` del caso
  1 y sale con exit 1.
- **El piloto se cae a mitad de un trabajo**: el sobre queda interrumpido y
  el caso 2 lo detiene. Retomarlo es de una persona hasta que exista el
  servicio.

## Criterios de aceptación

- CA-637: el sobre del test-writer guarda en su registro `trasplantados`, con las rutas que trasplantó ordenadas y relativas al árbol de la tarea, también cuando cierra `no-entregable` por veredicto rojo; un sobre que no trasplantó nada no lleva la clave.
- CA-638: un run del writer que cambia, borra o renombra un test protegido cierra su sobre en `scope`, `no-entregable`, exit 1, sin veredicto nuevo, con la nota del contrato y un hallazgo high cuya descripción empieza con `tests-del-test-writer: el rol writer escribio <ruta>`.
- CA-639: no es violación que el writer cree un test nuevo o cambie uno que no está protegido, ni que un sobre de test-writer cambie un test protegido; un `agents.writer.write` de `hoom.yaml` que permite esa ruta no levanta la regla; una tarea sin `trasplantados` en sus sobres no protege nada.
- CA-640: `hoom.yaml` acepta la sección `autonomia` con sus tres claves, da 0, 0 y `ventana_max` sin declarar cuando falta la sección o una clave, y rechaza una clave desconocida y cada valor fuera de rango con el texto del contrato.
- CA-641: `hoom item set <slug> --auto hasta-humano` escribe `auto` en un item con presupuesto y se niega con el texto del contrato en uno sin presupuesto; `--auto no` quita la clave; `--presupuesto-usd x` escribe el tope y rechaza un valor que no es mayor que 0.
- CA-642: `boardcmd.Piloto` da la misma decisión para el mismo `Estado` sin leer el disco ni el reloj; nunca decide aprobar ni integrar; y en los casos 1, 2, 3, 8, 9, 10, 11 y 12 se detiene con el `Why` exacto de hoy, y en el caso 4 con el del contrato; re-expresa CA-379 en los casos que este spec agrega o reordena.
- CA-643: después de un sobre del test-writer que es un rojo esperado, `Piloto` lanza al writer con la regla `cadena`; si en ese veredicto `spec_trace` no está en pass, o el sobre tuvo una violación de scope, se detiene con el `Why` del caso 8.
- CA-644: antes de un lanzamiento, quien ejecuta el piloto commitea los cambios sin guardar del espacio de trabajo de la tarjeta con el mensaje `hoom: guardar la tarjeta <slug>`, no crea ningún commit en el árbol del proyecto ni hace push, y no commitea nada cuando no hay cambios.
- CA-645: con la regla `cadena`, la review se pide con `Delta` cuando la tarjeta tiene una review que se puede encadenar y sin `Delta` cuando no la tiene.
- CA-646: aprobar la spec desde la tarjeta del Studio (`approveCard`) con `auto: hasta-humano` ejecuta el piloto, que lanza al test-writer, y la respuesta trae `piloto` con la línea de esa decisión; sin `auto` no lanza nada y la respuesta es la de hoy.
- CA-647: `hoom piloto <slug>` ejecuta las decisiones de `Piloto` en primer plano hasta un `detenerse`, imprime una línea por decisión, sale con exit 0 si se detuvo en una columna humana o en Hecho y con exit 1 en cualquier otra detención; con otro piloto corriendo sobre la misma tarjeta sale con exit 1 y el texto del contrato.
- CA-648: `hoom aprobar` y `hoom spec approve` no lanzan ningún trabajo, e imprimen `la tarjeta tiene piloto automatico: sigue con: hoom piloto <slug>` solo cuando la tarjeta de esa spec tiene `auto: hasta-humano`.
- CA-649: con `reintentos_ante_rojo: 1`, después del primer sobre del writer cerrado en `verify` con veredicto rojo y sin ningún gate en ERROR, `Piloto` lanza al writer con el mismo provider, sin `Resume`, `Intento` 2, la regla `reintento` y un pedido que nombra el id del veredicto y los gates en rojo.
- CA-650: en el caso 6, `Piloto` se detiene con `entorno: el gate <gate> no pudo correr: no se reintenta con modelos` cuando un gate dio ERROR, y con `rojo final: ...` cuando la tarjeta ya tiene dos sobres de writer fuera de ronda; con `reintentos_ante_rojo: 0`, o con un writer cortado en `scope`, se detiene con el `Why` del caso 8.
- CA-651: un run que muere por su `timeout` deja `corte: "tiempo"` en su sidecar y en el registro de su sobre; un run que termina de otra forma no lleva la clave.
- CA-652: después del primer sobre de un rol con `corte: tiempo`, `Piloto` lanza con `Resume` y el pedido de relevo cuando el rol no es ciego ni reviewer, el provider sabe reanudar y hay id de sesión, y si no repite el mismo lanzamiento; después del segundo se detiene con `el <rol> se corto por tiempo dos veces: ...`.
- CA-653: al empezar una ronda, `Piloto` se detiene con el texto del contrato cuando `n` pasa de `rondas_de_hallazgos` y cuando los bloqueantes no bajaron respecto de la ronda anterior; si no, lanza al refutador con los ids que todavía no pasaron por un refutador y `Ronda` `n`, o al test-writer cuando todos ya pasaron.
- CA-654: después del refutador de una ronda, `Piloto` lanza al test-writer si quedan bloqueantes y sigue la tabla desde el caso 8 si no quedan; después del test-writer de la ronda, lanza al writer si es un rojo esperado y se detiene con `los tests de la ronda <n> no fallan: ...` si su veredicto es verde.
- CA-655: después del writer de una ronda, `Piloto` decide `cerrar-hallazgos` con los bloqueantes que cita algún test protegido de esa ronda cuando el veredicto es rojo solo por `findings_open`; se detiene con `quedan hallazgos sin test en la ronda <n>: <ids>: te toca` cuando ninguno está citado, y con `el arreglo de la ronda <n> quedo rojo en <gates>: te toca` cuando falla otro gate.
- CA-656: al ejecutar `cerrar-hallazgos`, cada hallazgo de la decisión queda cerrado como `corregido` con autor `hoom piloto` y una evidencia que nombra la ronda, el test que lo cita y los dos veredictos; los bloqueantes que ningún test cita siguen abiertos; después hay un veredicto nuevo de la tarjeta y el espacio de trabajo queda sin cambios por commitear.
- CA-657: después del writer de una ronda con el último veredicto de la tarjeta en verde, `Piloto` lanza al reviewer con `Delta` y la regla `ronda-review`; después de ese reviewer empieza otra ronda si hay bloqueantes y sigue la tabla si no; el provider de cada paso de la ronda es el que el contrato saca del disco, y sin ese sobre o registro se detiene con `la tarjeta no tiene un sobre de <rol>: ...`.
- CA-658: los pedidos de reintento, de relevo y de ronda son los textos del contrato con sus valores, y para cualquier `Estado` no contienen ningún texto que no venga de la plantilla, de un id, de una ruta, de un nombre de gate o de un hallazgo en disco.
- CA-659: con `ventana_max` declarada, `Piloto` lanza la review con un provider sin tope en USD y sin presupuesto en USD, y se detiene con `la ventana de 5h de <provider> va en <x>% (tope <y>%): ...` cuando el último dato vigente de esa ventana llegó al tope; sin dato lanza; sin `ventana_max`, ese provider no cuenta y se detiene con el `Why` del caso 12.
- CA-660: para cualquier política válida y cualquier secuencia de cierres, los lanzamientos del piloto en una tarjeta no pasan de `3 + reintentos_ante_rojo + 4*rondas_de_hallazgos + 4`; re-expresa CA-380 en la cota, que deja de ser tres.
- CA-661: todo sobre que lanza el piloto lleva `piloto: true` y su `regla`, y además `intento`, `ronda`, `hallazgos` y `bloqueantes` cuando el contrato los pide; las líneas del piloto son las del contrato, con las tres de hoy sin cambios.
- CA-662: el README documenta el piloto de la terminal, la política y los tests protegidos. [verifica: grep -q "hoom piloto" README.md && grep -q "autonomia:" README.md && grep -q "tests del test-writer" README.md]

## Decisiones

- **Dos topes independientes en vez de niveles 0 a 4.** El reintento ante un
  rojo es barato y lo decide un gate. La ronda de hallazgos es la cara y la
  única que nace de la opinión de un modelo. Poder encender una sin la otra
  vale más que una escalera.
- **La política es del proyecto; la tarjeta solo enciende o apaga.** Un
  nivel por tarjeta suma una combinación más que probar y nadie lo pidió
  todavía.
- **Reintento con la misma CLI en sesión nueva, y nada más.** Un modelo
  atascado relee sus errores como hechos; una sesión nueva recibe solo el
  veredicto. La otra CLI obliga a decidir quién revisa cuando escribieron
  dos y desde qué commit arranca: va en su propio spec.
- **Refutar antes de arreglar.** 24 de 139 high terminaron refutados; pasar
  primero por el refutador evita escribir código para un hallazgo falso.
- **El refutador es la CLI que revisó.** Que el acusador retire su propio
  hallazgo, con evidencia, es creíble. Que lo cierre la CLI que escribió el
  código no lo sería.
- **Un hallazgo se cierra con un test que lo cita, no con la palabra de un
  rol.** El test-writer ciego lo escribe, falla, el writer arregla y hoom
  cierra citando los dos veredictos. Es lo que deja cerrar sin creerle a
  nadie, y es el mismo ritual que hoy se hace a mano.
- **`cerrar-hallazgos` lo firma `hoom piloto`.** Lo que el agente hace solo
  queda marcado como hecho por una regla, nunca por una persona.
- **Sin reintentos dentro de una ronda.** La ronda ya es larga; un rojo en
  el arreglo vuelve a la persona.
- **El rojo esperado se define por los gates del spec.** Antes de la
  implementación fallan el build, el análisis y los tests, según el stack;
  lo único que tiene que estar bien es que cada criterio tenga su test.
- **El tope sin USD es la ventana, mirada entre trabajos.** Con suscripción
  el USD es un equivalente, y Codex no lo informa. La ventana se conoce al
  cerrar cada run, no durante.
- **El piloto guarda, pero solo en la rama de la tarea.** Sin commit, la
  review se niega y el árbol ciego no ve el trabajo. El árbol del proyecto
  y el push siguen siendo de la persona.
- **El piloto exige la tarea abierta.** Sin espacio de trabajo propio,
  guardar sería commitear en la rama que esté tomada en el proyecto.
- **`hoom piloto` en primer plano, sin servicio.** Toda función nace como
  verbo. El servicio que lo mantenga vivo llamará a esta misma función.
- **Aprobar desde la terminal avisa y no lanza.** `hoom spec approve` es un
  paso corto y síncrono; arrancar un trabajo de una hora desde ahí
  sorprendería.

## Riesgos y deuda aceptada

- **La cadena completa bajo el sobre nunca corrió de punta a punta.** En los
  registros que hay en disco hay 0 sobres de writer y 0 lanzados por el
  piloto. Este spec va a ser su primer uso real.
- **hoom no ve el resultado test por test.** Un test que cita un hallazgo y
  falla por otra cosa alcanza para cerrarlo. Es la misma confianza que hoy
  tiene la traza de criterios.
- **El test-writer ciego puede no lograr reproducir un hallazgo** que
  describe la implementación. Ese hallazgo vuelve a la persona.
- **Cada ronda suma un refutador y un test-writer.** No está medido si eso
  baja los hallazgos que nacen de los arreglos; `hoom costo` lo va a decir.
- **Un error del test-writer deja al writer sin salida**, porque ya no puede
  tocar ese test. Cuesta un reintento antes de volver a la persona.
- **La ventana se mira entre trabajos.** Un solo trabajo largo puede pasarse
  del tope, y el primer trabajo de un provider en la tarjeta corre sin dato.
- **El piloto muere con el proceso que lo corre** (`hoom serve` o la
  terminal). El servicio y la cola quedan para su spec.
- **Fuera de una corrida hoom no distingue a una persona de un agente.** Un
  agente con shell podría correr `hoom piloto`, que igual no firma ni
  acepta.
- **Sin aviso al terminar.** Hay que mirar el tablero para saber que la
  tarjeta llegó a Tu aceptación o que se detuvo.
