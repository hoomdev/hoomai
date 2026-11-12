# Spec: la tarjeta corre sola entre tu firma y tu aceptación

Estado: BORRADOR — pendiente de aprobación humana.
Tarea: `hoom task start autonomia-entre-firmas` (rama
`hoom/autonomia-entre-firmas`).
Origen: la Parte A de `.hoom/intake/rfc-agente-autonomo-y-remoto.md`
(capacidades A1 a A6) y el análisis de consumo del 2026-11-10.
Depende de: `ciclo-de-review-acotado.md`, en main desde el PR 79 (qué
bloquea, `finding.Bloquea`, y el verbo `hoom item set`).
Re-expresa, solo en lo que la cinta hace y este spec cambia: CA-379, CA-380
y CA-381 (`historia-doctor-y-cinta.md`), CA-416
(`review-aislada-y-modelo-elegido.md`) y CA-497 (`verbos-de-la-tarjeta.md`).

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
   sucio, y el test-writer, con el spec sin guardar.
4. **La cinta no puede pedirle la review a Codex.** Exige un provider con
   tope en USD y Codex no tiene. La review no puede ser del provider que
   escribió, así que con Claude de writer la cinta se detiene siempre ahí.
5. **No reintenta ni corrige hallazgos.** Un rojo del writer o un high de la
   review la detienen.
6. **Vive solo dentro de `hoom serve`.** Desde la terminal no hay cinta.

Con este spec, una tarjeta con piloto recorre sola el camino de la firma a
Tu aceptación:

- La firma lanza al test-writer, y su rojo esperado deja pasar al writer.
- El piloto guarda el trabajo antes de cada decisión.
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
- Que una persona pida al refutador desde la tarjeta (Studio o `hoom pedir`):
  esa acción solo la usa el piloto.
- El interruptor del piloto en el Studio y mostrar regla, intento y ronda en
  la historia de la tarjeta.
- Que la cinta siga sola después de un `hoom pedir` de la terminal: para eso
  está `hoom piloto`.
- Sesiones vencidas de las CLI, Antigravity, contenedores y el remoto.

## Contratos

### Los tests del test-writer no los toca el writer

- El registro del sobre (`envelope.Record`, en `.hoom/envelopes/` de la raíz
  del proyecto) gana `trasplantados`: las rutas que el trasplante del
  test-writer aplicó al árbol de la tarea, ordenadas, con `/` y relativas a
  ese árbol. Las que el trasplante borró no entran.
- Se escribe siempre que hubo trasplante: también cuando el sobre cierra en
  `verify` o `check` con veredicto rojo, o en `scope` por `fuera-de-scope`.
  Un sobre sin trasplante (el run falló, el scope cortó o no había nada que
  trasplantar) no lleva la clave.
- Los **tests protegidos** de una tarea son la unión de `trasplantados` de
  todos sus sobres de test-writer.
- El sobre de un rol de forma `codigo` (hoy solo el writer) lee los tests
  protegidos **antes** de lanzar el run, y el gate de scope compara el
  contenido en disco de cada uno antes y después. Si cambió o ya no está
  (borrado o renombrado, commiteado o no), el run viola la regla nueva
  `tests-del-test-writer`.
- Corta como `manipulacion`: el sobre cierra en `scope`, `no-entregable`,
  exit 1, sin verify, con la nota
  `el writer toco los tests del test-writer: no se emite veredicto sobre este arbol`.
  Si el mismo run además manipuló la evidencia, la nota es la de hoy.
- Queda un hallazgo high con el formato de siempre del gate, autor
  `hoom gate de scope`:
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

- Se lee del `hoom.yaml` del proyecto (el del checkout principal), como
  `agents` y `review`. La sección es estricta; el resto del nivel superior
  sigue como hoy.
- Sin la sección, o sin una clave, los dos contadores valen 0 y
  `ventana_max` no está declarada.
- Errores, todos como `hoom.yaml invalido: ...`:
  - `autonomia: clave desconocida %q (validas: reintentos_ante_rojo, rondas_de_hallazgos, ventana_max)`;
  - `autonomia: reintentos_ante_rojo tiene que ser 0 o 1`;
  - `autonomia: rondas_de_hallazgos tiene que ser un entero de 0 a 5`;
  - `autonomia: ventana_max tiene que ser un entero de 1 a 100`;
  - `autonomia: rondas_de_hallazgos necesita findings.block_on` cuando las
    rondas son más de 0 y no hay umbral: sin él, `findings_open` no corre y
    ninguna ronda podría cerrar.
- `hoom.yaml` ya está en el piso del gate de scope: ningún rol la cambia.
- El tiempo por trabajo ya existe: `agents.<rol>.timeout` (30 minutos por
  defecto) y, para el reviewer, `review.timeout`.

### La tarjeta: `hoom item set` y de dónde se lee

- `hoom item set <slug>` gana `--auto hasta-humano|no` y
  `--presupuesto-usd x`, que se combinan con `--riesgo`. Escribe en el mismo
  item que `--riesgo` (el del checkout principal, si lo tiene) y no
  commitea.
- Sin ninguno de los tres, error de uso (exit 2):
  `falta --riesgo, --auto o --presupuesto-usd: es lo que set cambia`.
- `--auto hasta-humano` sobre un item que queda sin presupuesto (no lo tiene
  y no viene `--presupuesto-usd`) se niega con exit 1 y el texto de
  `item add`: `--auto necesita --presupuesto-usd: el piloto automatico no corre sin tope`.
- `--auto no` quita la clave.
- `--presupuesto-usd` rechaza con exit 2 lo que no sea un número mayor que 0:
  `--presupuesto-usd tiene que ser un numero mayor que 0`.
- Con `$HOOM_ROLE` en el entorno, `--auto` y `--presupuesto-usd` se niegan:
  `el piloto automatico y el presupuesto de una tarjeta los cambia una persona, no un rol`.
- `auto` y `presupuesto_usd` se leen como el riesgo: del item del checkout
  principal si lo tiene, y si no, del árbol de la tarea.

### El piloto: una función, dos lugares

- `boardcmd.Piloto(e Estado) PilotDecision` reemplaza a
  `Pilot(before, after Card, closed envelope.Record)`. Es pura: no lee el
  disco ni el reloj.
- `Estado` trae, ya leído:
  - la tarjeta derivada;
  - sus sobres cerrados, del más viejo al más nuevo, con sus registros;
  - el veredicto de cada sobre y el último de la tarjeta, con el estado de
    cada gate;
  - sus registros de review;
  - los hallazgos que bloquean y qué test protegido cita a cada uno;
  - la política;
  - el último dato de la ventana de 5 h por provider, sacado de los
    sidecars de los runs de la tarjeta;
  - la hora.
- `PilotDecision` gana `Paso` (`lanzar`, `cerrar-hallazgos` o `detenerse`),
  `Regla`, `Intento`, `Ronda`, `Resume`, `Delta` y `Hallazgos`. `Launch` es
  verdadero solo con `lanzar`.
- **Nunca firma ni acepta.** Para ningún `Estado` la decisión es aprobar la
  spec o integrar.
- **Quien la ejecuta** repite siempre la misma vuelta hasta un `detenerse`:
  1. guarda (ver abajo);
  2. arma el `Estado`;
  3. llama a `Piloto`;
  4. ejecuta el paso.
- **En el Studio** la vuelta corre al cerrar un trabajo que lanzó el Studio,
  como hoy, y además al aprobar la spec desde la tarjeta (`approveCard`). La
  respuesta de aprobar gana `piloto`, con la línea de la primera decisión.
- **En la terminal**, verbo nuevo `hoom piloto <slug> [--json]`:
  - corre en primer plano e imprime una línea por decisión;
  - termina con exit 0 si se detuvo en una columna humana o en Hecho, y con
    exit 1 en cualquier otra detención;
  - toma el candado `.hoom/cache/locks/piloto-<slug>.lock` mientras corre;
    si ya lo tiene otro proceso, sale con exit 1 y
    `el piloto de <slug> ya corre en otro proceso`.
- `hoom aprobar <slug>` y `hoom spec approve <ruta>` no lanzan nada. Si la
  tarjeta de esa spec (la que tiene esa ruta de spec) tiene piloto, agregan
  la línea `la tarjeta tiene piloto automatico: sigue con: hoom piloto <slug>`.
- **Guardar.** Al empezar cada vuelta, si `auto` está en `hasta-humano` y la
  tarjeta tiene su tarea abierta:
  - commitea lo que no está guardado en el worktree de la tarea, en su rama
    `hoom/<slug>`, con el mensaje de siempre
    (`hoom: guardar la tarjeta <slug>`);
  - no commitea en el checkout principal (el item cambiado con
    `hoom item set` queda para `hoom item save`) y nunca hace push;
  - sin cambios no commitea nada;
  - si no puede guardar (un trabajo corriendo, el worktree fuera de su rama),
    se detiene: `no pudo guardar el trabajo de la tarjeta: <error>`.
- **Lanzar.** El piloto lanza por `lanzar`, la misma función que usan el
  Studio y `hoom pedir`.
  - `lanzar.Request` gana `Resume` (el id de sesión del provider) y `Delta`.
    Solo los usa el piloto; el cuerpo de `POST /launch` no cambia.
  - El piloto puede lanzar al refutador. La tarjeta no le ofrece esa acción
    a una persona.
- **La tabla del piloto pierde la columna Arquitecto.** Desde sus columnas
  lanza solo test-writer, writer y reviewer, y al refutador solo dentro de
  una ronda.

### La tabla del piloto

Decide el primer caso que se cumple. Los casos que dicen "como hoy"
conservan su `Why` exacto.

| # | Caso | Decisión |
|---|---|---|
| 1 | `auto` no es `hasta-humano` | se detiene, como el caso 1 de hoy |
| 2 | la tarjeta tiene un trabajo en curso o interrumpido | se detiene, como el caso 4 de hoy |
| 3 | la tarjeta está en una columna humana o en Hecho | se detiene, como el caso 5 de hoy |
| 4 | la tarjeta no tiene tarea abierta | se detiene: `el piloto corre en el espacio de trabajo de la tarjeta: abre la tarea con hoom task start <slug>` |
| 5 | el último sobre cerró con `corte: tiempo` | ver "Corte por tiempo" |
| 6 | `reintentos_ante_rojo` es 1 y el último sobre es del writer, fuera de ronda, cerrado en `verify` o `check` con veredicto rojo | ver "Reintento ante un rojo" |
| 7 | `rondas_de_hallazgos` es mayor que 0 y, o bien el último sobre es del reviewer y hay hallazgos que bloquean, o bien el último sobre lleva `ronda` | ver "La ronda de hallazgos" |
| 8 | el último sobre no cerró `entregable` | se detiene, como el caso 2 de hoy |
| 9 | la tarjeta está en rojo | se detiene, como el caso 3 de hoy |
| 10 | la columna de la tarjeta no está después de la `columna` del último sobre | se detiene, como el caso 6 de hoy |
| 11 | la acción principal no es la de la tabla de columnas, su rol es el del último sobre, o está deshabilitada | se detiene, como los casos 7, 8 y 9 de hoy |
| 12 | el item no tiene `presupuesto_usd` | se detiene, como el caso 10 de hoy |
| 13 | ningún provider de la acción está `ok` y tiene tope | se detiene, como el caso 11 de hoy |
| 14 | la ventana de 5 h del provider elegido llegó a `ventana_max` | se detiene (ver "Los topes") |
| — | ninguno | lanza la acción de la tabla de columnas, regla `cadena` |

- Todo lanzamiento, salga del caso que salga, pasa además por los casos 12,
  13 y 14.
- Sin sobres cerrados (una spec escrita a mano y recién firmada), los casos
  5 a 8 y 10 no aplican, y el 11 no compara con el rol del último sobre. Un
  último sobre sin `columna` (los de antes de este spec, o un `hoom agent`
  directo) no aplica al caso 10.
- **Rojo esperado del test-writer**: su sobre cerró en `verify` o `check`
  con veredicto rojo, sin violación de scope, y en ese veredicto
  `spec_lint`, `spec_trace` y `spec_approved` están en `pass`. Para los
  casos 8 a 11 cuenta como entregable y no deja la tarjeta en rojo, así que
  sigue el writer.
- **El sobre que termina una ronda** (ver "La ronda de hallazgos") cuenta
  igual que un rojo esperado para los casos 8 a 11.
- En la regla `cadena`, el provider y el presupuesto se eligen como hoy: el
  provider del sobre cerrado si está `ok` y tiene tope; si no, el primero
  `ok` con tope. La review descarta, como hoy, al provider que escribió el
  código. Un provider sin tope en USD se lanza sin presupuesto en USD.
- La review se pide con `Delta` cuando la tarjeta tiene una review que se
  puede encadenar: la misma condición con la que su siguiente paso propone
  `hoom review ... --delta`.

### Reintento ante un rojo

Aplica en el caso 6. `n` es la cantidad de sobres del writer fuera de ronda
que tiene la tarjeta. Decide el primero que se cumple:

| Condición | Decisión |
|---|---|
| un gate del veredicto quedó en `error` | se detiene: `entorno: el gate <gate> no pudo correr: no se reintenta con modelos` |
| `n` es 2 o más | se detiene: `rojo final: <n> intentos del writer y el veredicto <id> sigue rojo en <gates>` |
| ninguna | lanza al writer: mismo provider, sesión nueva (sin `Resume`), `Intento: 2`, regla `reintento` |

El pedido lo arma hoom:

```
Intento 2 de <slug>. El veredicto <id> quedo ROJO en: <gates>. La salida de cada gate esta en .hoom/verdicts/<id>.json. Corrige sobre el arbol actual hasta que verify quede verde. No toques los tests del test-writer.
```

Un writer que cierra cortado en `scope` no entra en este caso: tocar los
tests no se reintenta.

### Corte por tiempo

- Cuando un run muere por su timeout, su sidecar y el registro de su sobre
  ganan `corte: "tiempo"`. El cierre del sobre no cambia (`run`,
  `no-entregable`).
- En el caso 5, `k` es la cantidad de sobres de ese rol con `corte: tiempo`
  en la tarjeta:
  - con `k` igual a 1, el piloto **releva** una vez, con la regla
    `relevo-por-tiempo`:
    - si el rol no es ciego ni reviewer, el provider declara `Resume` y el
      sidecar del run guardó su id de sesión, lanza con `Resume` y el pedido
      `Te cortaste por tiempo. Continua donde quedaste y termina el pedido original.`;
    - si no, repite el lanzamiento que se cortó: mismo rol, provider, regla,
      intento, ronda y hallazgos, con el `pedido` de su registro (al
      reviewer, las mismas opciones de review). Sin `pedido` en el registro
      se detiene, como el caso 2 de hoy;
  - con `k` igual a 2 o más se detiene:
    `el <rol> se corto por tiempo dos veces: sube <clave> o parte la tarea`,
    donde `<clave>` es `agents.<rol>.timeout`, o `review.timeout` para el
    reviewer.

### La ronda de hallazgos

Aplica en el caso 7. "Bloqueantes" son los hallazgos abiertos de la tarea
de la tarjeta que bloquean según `finding.Bloquea`. Cada ronda tiene hasta
cuatro lanzamientos y un cierre, y todos sus sobres llevan `ronda: <n>`.

Un bloqueante **ya pasó por un refutador** cuando su id está en
`hallazgos` de un sobre de refutador de la tarjeta que no cortó en `run`.

**Empezar una ronda** (el último sobre es del reviewer y hay bloqueantes).
`n` es 1 más la mayor `ronda` de los sobres de la tarjeta; `k`, los
bloqueantes de ahora; `k_antes`, los que había al empezar la ronda `n-1`.

| Condición | Decisión |
|---|---|
| `n` es mayor que `rondas_de_hallazgos` | se detiene: `se hicieron <n-1> rondas de hallazgos y quedan <k> que bloquean: te toca` |
| `n` es 2 o más y `k` no es menor que `k_antes` | se detiene: `los hallazgos que bloquean no bajan (<k_antes> -> <k>): te toca` |
| algún bloqueante no pasó todavía por un refutador | lanza al refutador con esos ids, regla `ronda-refutador` |
| ninguna | sigue con el test-writer de la ronda |

**Los pasos siguientes**, según el último sobre de la ronda. "Rojo solo por
`findings_open`" quiere decir que ese gate está en `fail` y todos los demás
gates requeridos en `pass`.

| Último sobre | Condición | Decisión |
|---|---|---|
| refutador | no quedan bloqueantes | la ronda termina: sigue la tabla |
| refutador | quedan bloqueantes | lanza al test-writer, regla `ronda-test-writer` |
| test-writer | es un rojo esperado | lanza al writer, regla `ronda-writer` |
| test-writer | su veredicto es verde | se detiene: `los tests de la ronda <n> no fallan: los hallazgos no se reproducen: te toca` |
| writer | el último veredicto de la tarjeta es verde | lanza al reviewer con `Delta`, regla `ronda-review` |
| writer | el veredicto es rojo solo por `findings_open` y algún bloqueante está citado por un test protegido de esta ronda | paso `cerrar-hallazgos` con esos ids, regla `ronda-cierre` |
| writer | el veredicto es rojo solo por `findings_open` y ningún bloqueante está citado | se detiene: `quedan hallazgos sin test en la ronda <n>: <ids>: te toca` |
| writer | el veredicto es rojo por otro gate | se detiene: `el arreglo de la ronda <n> quedo rojo en <gates>: te toca` |
| reviewer | hay bloqueantes | empieza otra ronda |
| reviewer | no hay bloqueantes | la ronda termina: sigue la tabla |

Cualquier otro cierre dentro de una ronda se detiene, como el caso 2 de hoy.

**El paso `cerrar-hallazgos`** lo ejecuta hoom, sin modelo y fuera de una
corrida. La regla de hoy, que dentro de una corrida solo el refutador
cierra, no cambia.

1. Cierra como `corregido` cada hallazgo de la decisión, con autor
   `hoom piloto` y la evidencia
   `ronda <n>: el test <ruta> cita el hallazgo; veredicto <id> rojo antes del arreglo; despues, veredicto <id> con todos los demas gates en pass`.
2. Corre verify de la tarjeta, como `hoom verify --spec`.
3. Si el veredicto nuevo no queda verde, se detiene:
   `despues de cerrar los hallazgos de la ronda <n>, el veredicto <id> sigue rojo en <gates>: te toca`.
4. Guarda el espacio de trabajo.

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
rutas, nombres de gate, el texto de los hallazgos que están en disco y, al
repetir un lanzamiento, el `pedido` guardado en su registro.

### Los topes

- **USD**: sigue igual para los providers que lo aplican. Un run sin costo
  suma 0.
- **Caso 13.** Un provider "tiene tope" si declara tope en USD (hoy solo
  claude) o si `autonomia.ventana_max` está declarada. En el segundo caso
  se lanza sin presupuesto en USD.
- **Caso 14.** Para el provider elegido se mira el último dato `5h` de los
  sidecars de los runs de la tarjeta (`ventanas`) cuyo reinicio todavía no
  pasó. Si llegó a `ventana_max`, se detiene:
  `la ventana de 5h de <provider> va en <x>% (tope <y>%): el piloto espera; reinicia <hora UTC>`.
  Sin dato, lanza.
- **Siempre termina.** Los lanzamientos del piloto en una tarjeta no pasan
  de `3 + reintentos_ante_rojo + 4*rondas_de_hallazgos + 4`: la cadena, el
  reintento, cuatro por ronda y un relevo por cada rol que el piloto lanza
  (test-writer, writer, reviewer y refutador).

### Lo que queda escrito

- El registro del sobre gana:
  - `trasplantados` y `corte`, ya descritos;
  - `columna` (la de la tarjeta al lanzar) y `pedido` (el texto que recibió
    el rol), en todo sobre lanzado por `lanzar`;
  - `regla`, `intento`, `ronda`, `hallazgos` (los ids que recibió ese paso)
    y `bloqueantes` (cuántos había, en el primer sobre de cada ronda), en
    los que lanza el piloto.
- Todo lanzamiento del piloto lleva `piloto: true` y su `regla`.
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
- Los tests que hoy fijan lo contrario pasan a esperar lo de este spec y
  citan los ids re-expresados. Es el cambio que este spec aprueba, no un
  test debilitado:
  - la tabla del piloto con la columna Arquitecto, los once casos de la
    cinta y que una persona encadena como mucho tres trabajos (CA-379,
    CA-380);
  - que la cinta se detiene ante el rojo del test-writer (CA-381);
  - que se detiene con el spec sin guardar (CA-381, CA-497) o con el código
    del writer sin commitear (CA-416). Ahora guarda y sigue.

## Casos límite y errores esperados

- **Sin sección `autonomia`**: el piloto no reintenta ni hace rondas, y no
  le pide nada a un provider sin tope en USD. Sí guarda antes de decidir,
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
- **Un hallazgo sin tarea que bloquea**: `findings_open` también lo cuenta.
  `cerrar-hallazgos` no lo toca, el veredicto nuevo sigue rojo y el piloto
  se detiene.
- **Un bloqueante subido a mano en medio de una ronda**: cuenta desde la
  siguiente decisión.
- **La persona pide un trabajo a mano en medio de la cadena**: sus sobres
  cuentan para `n`, para `k` y para la ronda igual que los del piloto.
- **La persona apaga `auto` mientras corre un trabajo**: al cerrar, el caso
  1 detiene el piloto.
- **El item cambiado con `hoom item set` sin guardar**: el piloto lo lee
  del disco y no lo commitea.
- **La tarjeta escrita fuera del sobre** (el writer trabajó en una sesión
  interactiva): no hay sobre de writer, así que la ronda se detiene con
  `la tarjeta no tiene un sobre de writer`.
- **El relevo de un rol ciego**: no se puede reanudar su sesión, así que se
  vuelve a lanzar desde cero con su `pedido`.
- **Dos pilotos sobre la misma tarjeta** (Studio y terminal): el segundo ve
  el trabajo en curso (caso 2) o el candado de `hoom piloto`.
- **`hoom piloto` sobre una tarjeta sin `auto`**: imprime el `Why` del caso
  1 y sale con exit 1.
- **El piloto se cae a mitad de un trabajo**: el sobre queda interrumpido y
  el caso 2 lo detiene. Retomarlo es de una persona hasta que exista el
  servicio.

## Criterios de aceptación

- CA-637: el sobre del test-writer guarda en su registro `trasplantados`, con las rutas que su trasplante aplicó (no las que borró), ordenadas y relativas al árbol de la tarea, también cuando cierra rojo en `verify` o `check` o en `scope` por `fuera-de-scope`; un sobre sin trasplante no lleva la clave.
- CA-638: un run del writer que cambia, borra o renombra un test protegido, lo commitee o no, cierra su sobre en `scope`, `no-entregable`, exit 1, sin veredicto nuevo, con la nota del contrato y un hallazgo high cuya descripción empieza con `tests-del-test-writer: el rol writer escribio <ruta>`.
- CA-639: no es violación que el writer cree un test nuevo o cambie uno que no está protegido, ni que un sobre de test-writer cambie un test protegido; un `agents.writer.write` de `hoom.yaml` que permite esa ruta no levanta la regla; una tarea sin `trasplantados` en sus sobres no protege nada.
- CA-640: el `hoom.yaml` del proyecto acepta la sección `autonomia` con sus tres claves, da 0, 0 y `ventana_max` sin declarar cuando falta la sección o una clave, y rechaza con el texto del contrato una clave desconocida, cada valor fuera de rango y `rondas_de_hallazgos` mayor que 0 sin `findings.block_on`.
- CA-641: `hoom item set <slug> --auto hasta-humano` escribe `auto` en un item con presupuesto y se niega con exit 1 y el texto del contrato en uno sin presupuesto; `--auto no` quita la clave; `--presupuesto-usd x` escribe el tope y rechaza con exit 2 un valor que no es mayor que 0; con `$HOOM_ROLE` se niega; y la tarjeta lee `auto` y `presupuesto_usd` del item del checkout principal cuando lo tiene.
- CA-642: `boardcmd.Piloto` da la misma decisión para el mismo `Estado` sin leer el disco ni el reloj; nunca decide aprobar ni integrar, ni lanzar al arquitecto; en los casos 1, 2, 3 y 8 a 13 se detiene con el `Why` exacto de hoy, y en el caso 4 con el del contrato; re-expresa CA-379 en los casos que este spec agrega o reordena.
- CA-643: después de un sobre del test-writer que es un rojo esperado, `Piloto` lanza al writer con la regla `cadena`; si en ese veredicto `spec_trace` no está en `pass`, o el sobre tuvo una violación de scope, se detiene con el `Why` del caso 8.
- CA-644: en cada vuelta con `auto` y la tarea abierta, antes de decidir, quien ejecuta el piloto commitea lo no guardado del worktree de la tarea en `hoom/<slug>` con el mensaje `hoom: guardar la tarjeta <slug>`, no crea ningún commit en el checkout principal ni hace push, no commitea nada sin cambios, y con el spec o el código del writer sin guardar guarda y sigue; re-expresa CA-381, CA-416 y CA-497 en lo que hace la cinta.
- CA-645: con la regla `cadena`, la review se pide con `Delta` cuando la tarjeta tiene una review que se puede encadenar y sin `Delta` cuando no la tiene.
- CA-646: aprobar la spec desde la tarjeta del Studio (`approveCard`) con `auto: hasta-humano` ejecuta el piloto, que lanza al test-writer, y la respuesta trae `piloto` con la línea de esa decisión; sin `auto` no lanza nada y la respuesta es la de hoy.
- CA-647: `hoom piloto <slug>` ejecuta las decisiones de `Piloto` en primer plano hasta un `detenerse`, imprime una línea por decisión, sale con exit 0 si se detuvo en una columna humana o en Hecho y con exit 1 en cualquier otra detención; con otro piloto corriendo sobre la misma tarjeta sale con exit 1 y el texto del contrato.
- CA-648: `hoom aprobar <slug>` y `hoom spec approve <ruta>` no lanzan ningún trabajo, e imprimen `la tarjeta tiene piloto automatico: sigue con: hoom piloto <slug>` solo cuando la tarjeta de esa spec tiene `auto: hasta-humano`.
- CA-649: con `reintentos_ante_rojo: 1`, después del primer sobre del writer cerrado en `verify` con veredicto rojo y sin ningún gate en `error`, `Piloto` lanza al writer con el mismo provider, sin `Resume`, `Intento` 2, la regla `reintento` y un pedido que nombra el id del veredicto y los gates en rojo.
- CA-650: en el caso 6, `Piloto` se detiene con `entorno: el gate <gate> no pudo correr: no se reintenta con modelos` cuando un gate quedó en `error`, y con `rojo final: ...` cuando la tarjeta ya tiene dos sobres de writer fuera de ronda; con `reintentos_ante_rojo: 0`, o con un writer cortado en `scope`, se detiene con el `Why` del caso 8.
- CA-651: un run que muere por su timeout deja `corte: "tiempo"` en su sidecar y en el registro de su sobre, con el cierre de siempre; un run que termina de otra forma no lleva la clave.
- CA-652: después del primer sobre de un rol con `corte: tiempo`, `Piloto` lanza con `Resume` y el pedido de relevo cuando el rol no es ciego ni reviewer, el provider declara `Resume` y hay id de sesión; si no, repite el mismo lanzamiento con el `pedido` del registro, y sin `pedido` se detiene con el `Why` del caso 8; después del segundo se detiene con `el <rol> se corto por tiempo dos veces: sube <clave> ...`, con `review.timeout` para el reviewer.
- CA-653: al empezar una ronda, `Piloto` se detiene con el texto del contrato cuando `n` pasa de `rondas_de_hallazgos` y cuando los bloqueantes no bajaron respecto de la ronda anterior; si no, lanza al refutador con los ids que todavía no nombra ningún sobre de refutador de la tarjeta y `Ronda` `n`, o al test-writer cuando todos ya pasaron.
- CA-654: después del refutador de una ronda, `Piloto` lanza al test-writer si quedan bloqueantes y sigue la tabla si no quedan; después del test-writer de la ronda, lanza al writer si es un rojo esperado y se detiene con `los tests de la ronda <n> no fallan: ...` si su veredicto es verde.
- CA-655: después del writer de una ronda, `Piloto` decide `cerrar-hallazgos` con los bloqueantes que cita algún test protegido de esa ronda cuando el veredicto es rojo solo por `findings_open`; se detiene con `quedan hallazgos sin test en la ronda <n>: <ids>: te toca` cuando ninguno está citado, y con `el arreglo de la ronda <n> quedo rojo en <gates>: te toca` cuando falla otro gate.
- CA-656: al ejecutar `cerrar-hallazgos`, cada hallazgo de la decisión queda cerrado como `corregido` con autor `hoom piloto` y una evidencia que nombra la ronda, el test que lo cita y los dos veredictos; los bloqueantes que ningún test cita siguen abiertos; después hay un veredicto nuevo de la tarjeta, el piloto se detiene con el texto del contrato si no es verde, y el espacio de trabajo queda sin cambios por commitear.
- CA-657: después del writer de una ronda con el último veredicto de la tarjeta en verde, `Piloto` lanza al reviewer con `Delta` y la regla `ronda-review`; después de ese reviewer empieza otra ronda si hay bloqueantes y sigue la tabla si no; el provider de cada paso de la ronda es el que el contrato saca del disco, y sin ese sobre o registro se detiene con `la tarjeta no tiene un sobre de <rol>: ...`.
- CA-658: los pedidos de reintento, de relevo y de ronda son los textos del contrato con sus valores, y para cualquier `Estado` no contienen ningún texto que no venga de la plantilla, de un id, de una ruta, de un nombre de gate, de un hallazgo en disco o del `pedido` del sobre que se repite.
- CA-659: con `ventana_max` declarada, `Piloto` lanza la review con un provider sin tope en USD y sin presupuesto en USD, y se detiene con `la ventana de 5h de <provider> va en <x>% (tope <y>%): ...` cuando el último dato vigente de esa ventana llegó al tope; sin dato lanza; sin `ventana_max`, ese provider no cuenta y se detiene con el `Why` del caso 13.
- CA-660: para cualquier política válida y cualquier secuencia de cierres, los lanzamientos del piloto en una tarjeta no pasan de `3 + reintentos_ante_rojo + 4*rondas_de_hallazgos + 4`; re-expresa CA-380 en la cota, que deja de ser tres.
- CA-661: todo sobre lanzado por `lanzar` lleva `columna` y `pedido`; todo sobre que lanza el piloto lleva `piloto: true` y su `regla`, y además `intento`, `ronda`, `hallazgos` y `bloqueantes` cuando el contrato los pide; las líneas del piloto son las del contrato, con las tres de hoy sin cambios.
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
- **El piloto guarda antes de decidir, y solo en la rama de la tarea.** Si
  guardara después de decidir, el spec sin guardar ya habría deshabilitado
  al test-writer. Sin commit, la review se niega y el árbol ciego no ve el
  trabajo. El checkout principal, su item y el push siguen siendo de la
  persona.
- **El piloto exige la tarea abierta.** Sin espacio de trabajo propio,
  guardar sería commitear en la rama que esté tomada en el proyecto.
- **El piloto no pide al arquitecto.** Entre la firma y la aceptación no hay
  trabajo de arquitecto: una spec que vuelve a esa columna la arregla una
  persona. Sacarlo de la tabla deja la cota en cuatro roles.
- **Los tests protegidos se leen antes del run y se miden en el disco.** Así
  un run no puede desproteger sus propios tests, y un renombre commiteado,
  que `git diff` lista solo con el nombre nuevo, igual se ve.
- **`auto` y `presupuesto_usd` se leen como el riesgo.** `hoom item set`
  escribe en el item del checkout principal; si la tarjeta los leyera del
  árbol de la tarea, el piloto no vería lo que la persona acaba de cambiar.
- **El relevo repite con el pedido guardado.** La función es pura y el
  pedido no estaba en el disco: el registro pasa a guardarlo.
- **Las rondas exigen `findings.block_on`.** Sin umbral no corre
  `findings_open` y ningún veredicto puede quedar "rojo solo por
  `findings_open`".
- **`hoom piloto` en primer plano, sin servicio.** Toda función nace como
  verbo. El servicio que lo mantenga vivo llamará a esta misma función.
- **Aprobar desde la terminal avisa y no lanza.** `hoom spec approve` es un
  paso corto y síncrono; arrancar un trabajo de una hora desde ahí
  sorprendería.

## Riesgos y deuda aceptada

- **La cadena completa bajo el sobre nunca corrió de punta a punta.** En los
  registros que hay en disco hay 0 sobres de writer y 0 lanzados por el
  piloto. Este spec va a ser su primer uso real.
- **El piloto decide sobre telemetría local.** Los registros de los sobres
  viven en `.hoom/envelopes/`, fuera de Git. Un run no puede desproteger sus
  propios tests, pero un rol con shell podría editar esos registros para
  confundir a un run posterior o al piloto. El piloto igual no firma ni
  acepta.
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
