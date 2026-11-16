# Spec: el tablero dice tu siguiente paso (`hoom tablero` v3)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start tablero-tu-siguiente-paso`.
Depende de: `tablero-con-paneles.md` (la v2, integrada con los PR 69 y 71),
`ciclo-de-review-acotado.md` y `autonomia-entre-firmas.md` (integradas con
los PR 79, 80 y 82 a 84).
Origen: Henry, 2026-11-16, después de usar la v2: "padece de lo mismo del
Studio, me pierdo, no sé en qué está, qué debo hacer a continuación"; "lo
empiezo utilizando pero termino utilizando Claude Code Desktop porque no sé
qué hace falta hacer". Pidió llevar al tablero la idea del Studio nuevo: el
diseño "Tablero de terminal" del lienzo "hoom Studio nuevo"
(https://claude.ai/artifact/5ua6wr92SHiA7w2mmjvbxh), seis pantallas.
Después eligió hacer primero el tablero, en una versión recortada: la
pantalla principal con lo que la tarjeta ya da.

Re-expresa, de `tablero-con-paneles.md`: CA-550 (la disposición), CA-551
(el foco), CA-552 (la selección), CA-553 (ver lo elegido), CA-555 (la
lista), CA-556 (el detalle de una tarjeta), CA-558 (lo que corre), CA-564
(la línea de teclas), CA-566 (las teclas de acción y `n`), CA-574 (solo la
firma con el piloto encendido) y CA-575 (solo dónde hace efecto un clic).
Los otros 19 criterios de la v2 siguen igual, con los nombres de sus
paneles traducidos como dice "Los nombres de la v2".

## Objetivo

Que quien abre `hoom tablero` sepa, sin leer nada más, dos cosas:

1. **Qué tiene que hacer ahora**: una frase y una tecla, siempre en el
   mismo lugar, arriba de todo.
2. **En qué está cada tarjeta**: si le toca a la persona, si hoom está
   trabajando, si algo la frenó o si todavía no empezó. Con el título de la
   tarjeta y su estado en palabras, no con un slug y una tira de signos.

La v2 ya tiene lo necesario: el `plain` de cada tarjeta, el siguiente paso
que abre `n`, lo que corre, el rojo, los pendientes y el piloto. Pero lo
reparte en seis zonas del mismo peso: cuatro paneles numerados, el detalle
y el Registro. "¿Qué falta?" y "Siguiente paso" son dos líneas chicas
dentro del detalle. El que entra no sabe por dónde empezar.

Este spec ordena la pantalla alrededor de una pregunta: **quién tiene la
pelota**. Arriba, la caja "TU SIGUIENTE PASO". A la izquierda, una sola
lista agrupada por quién tiene la pelota. A la derecha, el detalle de la
tarjeta elegida, siempre en el mismo orden: AHORA, TE TOCA, SIGUE y EL
CAMINO. Además, el tablero puede dejar que una tarjeta con piloto siga
sola (`hoom piloto`), así no hace falta salir a otra terminal.

El principio de la v2 sigue: nada a ciegas. Toda tecla que ejecuta algo
abre primero un modal con el comando exacto, y el mouse nunca ejecuta. Y la
regla de hoom también: la pantalla muestra lo que hoom comprobó, no lo que
un modelo cuenta.

## No-goals

- **No cambia el Studio.** El diseño del Studio nuevo es otra tarea.
- **No cambian los modales de la v2**: Pedir, Acciones, Guardar y Ayuda
  quedan como están, con su contenido. La firma solo suma lo del piloto
  (ver "Seguir solo"). Las pantallas "Leer y firmar" y "Ver y aceptar" del
  lienzo quedan para un spec propio.
- **No hay pantalla de pedir algo nuevo** (`hoom item add` desde el
  tablero) ni pantalla de primer uso. Un proyecto sin tarjetas sigue
  diciendo `no hay tarjetas: hoom item add "<titulo>"`.
- **No se enciende ni se apaga el piloto desde el tablero.** El detalle
  dice el comando (`hoom item set <slug> --auto hasta-humano`), pero no lo
  ejecuta.
- **No se muestra la ventana de 5 h de codex ni de claude.** El lienzo la
  dibuja en la cabecera, pero el tablero no la lee hoy.
- **No cambia ningún verbo de la capa 1** ni `hoom piloto`. El único cambio
  fuera del tablero es un campo nuevo en el rojo de la tarjeta (ver "El
  rojo esperado").
- **No cambian el catálogo de modelos, la tabla de anchos, la búsqueda, el
  modo experto, los datos que se leen ni su frecuencia.**
- **No hay datos nuevos inventados.** El camino no muestra horas por paso
  ni cantidad de criterios, porque la tarjeta no los da en la lista.

## Contratos

### La pantalla

De arriba hacia abajo, con `A` líneas y `W` columnas (60x15 o más):

| Líneas | Qué es |
|---|---|
| 1 | la cabecera |
| 2 a 5 | la caja TU SIGUIENTE PASO |
| 6 a A-3 | el cuerpo: la lista y el detalle |
| A-2 | el Registro, plegado en una línea |
| A-1 | la línea de mensajes, como en la v2 |
| A | la línea de teclas |

- **Con 100 columnas o más hay dos zonas.** La lista va a la izquierda en
  `W*2/5` celdas (división entera) y el detalle a la derecha en el resto,
  las dos con todo el alto del cuerpo.
- **Entre 60 y 99 columnas hay una sola columna**: la lista arriba y el
  detalle abajo, los dos de todo el ancho. Con el foco en la lista, la
  lista ocupa lo que necesita, hasta la mitad del cuerpo redondeada hacia
  arriba y al menos 3 líneas, y el detalle el resto. Con el foco en el
  detalle, la lista se pliega a una línea (`─Tarjetas · 1 de 6────`) y el
  detalle ocupa el resto.
- **Debajo de 60x15** sigue la regla de la v2: una línea que dice
  `agranda la terminal (minimo 60x15)`.
- La lista y el detalle son cajas con borde, como los paneles de la v2. La
  que tiene el foco lleva el borde del foco. La caja TU SIGUIENTE PASO nunca
  tiene el foco.

Boceto a 100x30, sin color (los dorados fijan cada celda):

```
 hoom tablero · oceanbuild · rama main                  check VERDE · verify hace 2h · hoy 3.88 USD
┌TU SIGUIENTE PASO────────────────────────────────────────────te toca 2 · corriendo 1 · detenida 1─┐
│ Firmá el spec de «Exportar reporte a PDF».  te toca aprobar el spec                              │
│ n aprobar spec                     después: «Listado de proyectos» · «Notificaciones por correo» │
└──────────────────────────────────────────────────────────────────────────────────────────────────┘
╔Tarjetas══════════════════════════════╗┌Exportar reporte a PDF───────────────exportar-reporte-pdf─┐
║ TE TOCA 2                            ║│ AHORA                                            TE TOCA │
║ ▸ Exportar reporte a PDF     ●●◆○○○○○║│ El spec está listo y espera tu firma.                    │
║   te toca aprobar el spec            ║│ te toca aprobar el spec                                  │
║   Listado de proyectos       ●●●●●●◆○║│                                                          │
║   lista para tu aceptacion           ║│ TE TOCA                                                  │
║ CORRIENDO 1                          ║│ enter aprobar spec · hoom no firma por vos               │
║   Módulo de usuarios         ●●●●◉○○○║│                                                          │
║   writer · 14:02 de 30:00            ║│ SIGUE                                                    │
║ DETENIDA 1                           ║│ al firmar, el piloto pide los tests y sigue solo hasta   │
║   Notificaciones por correo  ●●●●✗○○○║│ tu aceptación                                            │
║   ✗ el veredicto quedo rojo en test  ║│                                                          │
║ EN COLA 2                            ║│ EL CAMINO                                    paso 3 de 8 │
║   Importar desde Excel       ○○○○○○○○║│  ● Pedido                                                │
║   falta el spec de la tarjeta        ║│  ● Spec                                                  │
║   Recordatorios de vencimie… ○○○○○○○○║│  ◆ Tu firma         te toca a vos                  ← acá │
║   falta el spec de la tarjeta        ║│  ○ Tests                                                 │
║ HECHO 4 ▸                            ║│  ○ Código                                                │
║                                      ║│  ○ Review                                                │
║                                      ║│  ○ Tu aceptación                                         │
║                                      ║│  ○ Hecho                                                 │
║                                      ║│                                                          │
╚══════════════════════════════════════╝└──────────────────────────────────────────────────────────┘
─Registro · 09:58 Pedir al arquitecto · exportar-reporte-pdf · hecho────────────────────────────────

 enter aprobar spec · n tu siguiente paso · ↑↓ elegir · tab detalle · espacio acciones · ? todas
```

Los `plain` del boceto son de ejemplo: en la pantalla van los de cada
tarjeta.

### La cabecera

Una línea con lo que la v2 mostraba en [1]:

- a la izquierda, `hoom tablero · <proyecto> · rama <rama>` (o
  `· sin git`), con `hoom tablero` en verde y negrita;
- a la derecha, la línea del check de la v2 (`check VERDE · verify hace
  <t>`, `check ROJO: <motivo>` o `sin veredicto todavia`), después
  ` · hoy <usd> USD` (o ` · hoy sin trabajos`) y después
  ` · fijada: <slug>` cuando hay tarjeta fijada.

Cuando no entra, se cae primero lo fijado, después lo de hoy, y por último
se corta el check. La izquierda nunca se corta antes que la derecha
entera. Antes de la primera lectura, la derecha dice
`leyendo el proyecto...`.

### La caja TU SIGUIENTE PASO

Cuatro líneas de todo el ancho: el borde de arriba, dos líneas y el borde
de abajo.

- **El borde de arriba** dice `TU SIGUIENTE PASO` a la izquierda y
  `te toca <a> · corriendo <b> · detenida <c>` a la derecha, siempre con
  los tres números (ver "Los grupos").
- **El color del borde.** Con color, el borde y su título van en amarillo
  cuando TE TOCA o DETENIDA tienen alguna tarjeta, y en gris cuando no.

**Qué tarjeta muestra.** La primera de TE TOCA, en el orden de la lista. Si
TE TOCA está vacía, la primera de DETENIDA. Si las dos están vacías, la
caja no muestra una tarjeta (ver abajo).

**La primera línea** es una frase en negrita y, después de dos espacios, en
gris, el `plain` de la tarjeta (en DETENIDA, el de su rojo o el del corte).
La frase sale del paso de la tarjeta (ver "El paso de una tarjeta"):

| El paso | La frase |
|---|---|
| Aprobar spec | `Firmá el spec de «<titulo>».` |
| Integrar | `Aceptá «<titulo>».` |
| Pedir al arquitecto | `Pedile el spec al arquitecto para «<titulo>».` |
| Pedir al test-writer | `Pedile los tests al test-writer para «<titulo>».` |
| Pedir al writer | `Pedile el código al writer para «<titulo>».` |
| Pedir al reviewer | `Pedile la review al reviewer para «<titulo>».` |
| Reanudar | `Reanudá el trabajo de «<titulo>».` |
| Guardar | `Guardá el trabajo de «<titulo>».` |
| Seguir solo | `Dejá que «<titulo>» siga sola.` |
| sin paso, en TE TOCA | `Mirá «<titulo>».` |
| una tarjeta de DETENIDA | `Decidí cómo sigue «<titulo>».` |

En DETENIDA la frase es siempre la última fila, tenga paso o no.

**La segunda línea**, a la izquierda: `n` y la etiqueta del paso en
minúscula (`n aprobar spec`, `n seguir solo`). Sin paso, el texto que da la
regla del paso. A la derecha, en gris: `después: «<titulo>» · «<titulo>»`
con las tarjetas que siguen en el mismo orden (las de TE TOCA y después las
de DETENIDA), tantas como entren, cada título cortado con `…` si hace
falta. Sin otras, no hay `después:`.

**Sin tarjeta en TE TOCA ni en DETENIDA:**

- con alguna en CORRIENDO, la primera línea es
  `Nada te espera. hoom trabaja en <n> tarjeta(s).` y la segunda
  `corriendo: «<titulo>» (<rol>, <tiempo>)`, con ` y <k> más` cuando hay
  más. `n` dice en la línea de mensajes `nada te espera: hoom esta trabajando`.
- sin ninguna corriendo y con alguna en EN COLA, la primera línea es
  `Nada en curso. Podés empezar «<titulo>».`, con la primera de EN COLA, y
  la segunda es `n` y la etiqueta de su paso (`n pedir al arquitecto`).
  `n` abre ese paso.
- sin ninguna de las tres, la primera línea es `No hay nada pendiente.` y la
  segunda `para sumar trabajo: hoom item add "<titulo>"`.

```
┌TU SIGUIENTE PASO────────────────────────────te toca 0 · corriendo 2 · detenida 0─┐
│ Nada te espera. hoom trabaja en 2 tarjetas.                                      │
│ corriendo: «Módulo de usuarios» (writer, 14:02) y 1 más                          │
└──────────────────────────────────────────────────────────────────────────────────┘
```

Antes de la primera lectura del tablero, la caja dice
`leyendo el tablero...`. La frase y el texto fijo de la caja llevan acentos
con UTF-8 y los pierden en ASCII, como los títulos de la v2; `«` y `»` son
`"` en ASCII.

### Los grupos

Cada tarjeta cae en un solo grupo: el primero de esta tabla que se cumple.

| Orden | Grupo | Cuándo |
|---|---|---|
| 1 | HECHO | la columna es Hecho |
| 2 | CORRIENDO | tiene `running`, o un piloto que lanzó este tablero sigue vivo para ella |
| 3 | DETENIDA | tiene `interrupted`, o tiene `red` y el rojo no es esperado |
| 4 | EN COLA | la columna es Backlog |
| 5 | TE TOCA | cualquier otra |

TE TOCA incluye a las tarjetas cuyo paso está deshabilitado o que no tienen
paso: la pelota igual la tiene la persona, y el detalle dice por qué no hay
nada que ejecutar.

**El orden de la lista**: TE TOCA, CORRIENDO, DETENIDA, EN COLA y HECHO.

- **En TE TOCA las tarjetas van de la más cerca de Hecho a la más lejos**,
  por columna: Tu aceptación, Review, Writer, Test-writer, Tu aprobación y
  Arquitecto. Primero se termina lo empezado.
- **Dentro de una misma columna**, y en los otros grupos, van en el orden
  del tablero.

**La línea de un grupo** es ` <GRUPO> <n>`. Con color, el nombre va en
video inverso del color del grupo: TE TOCA amarillo (`7;33`), CORRIENDO
cian (`7;36`) y DETENIDA rojo (`7;31`). EN COLA y HECHO van en gris (`90`).

- No hay grupos vacíos, salvo HECHO, que siempre está y empieza cerrado,
  como en la v2: ` HECHO <n> ▸` cerrado y ` HECHO <n> ▾` abierto.
- La línea de HECHO se puede elegir; `enter` la abre y la cierra. Las otras
  líneas de grupo no se eligen.

### Las filas

**La primera línea de una tarjeta** es la marca de la v2 (`▸` o un
espacio), el **título** de la tarjeta y el camino (8 signos) pegado al
borde derecho. El título se corta con `…` para dejar el camino. Cuando al
título le quedan menos de 8 celdas, la fila va sin camino, como en la v2.
El slug no está en la lista: está en el detalle y la búsqueda lo sigue
encontrando.

**Con 100 columnas o más, cada tarjeta tiene una segunda línea**, con tres
espacios de margen, con su estado en el color de su grupo:

| Grupo | La segunda línea |
|---|---|
| TE TOCA | el `plain` |
| CORRIENDO | `<rol> · <tiempo>[ de <limite>]`, con los tiempos de la v2; con un piloto de este tablero vivo, empieza con `el piloto la lleva · `, y sin trabajo en curso es `el piloto la lleva · decidiendo` |
| DETENIDA | `✗ ` y el `plain` de su rojo, o `✗ se corto el trabajo del <rol>` |
| EN COLA | el `plain`, en gris |
| HECHO | el `plain`, en gris |

**Entre 60 y 99 columnas, una tarjeta es una sola línea**: la marca, el
título, el camino y, después de un espacio, la segunda línea de arriba. El
título se corta o se rellena para que el camino quede en la misma columna
en todas las filas.

La selección es la de la v2: `▸` en la primera línea y, en la zona con
foco y con color, la barra en video inverso sobre la primera línea.

### El camino

Un signo por columna del tablero, en su orden, 8 en total:

| Dónde | Signo | ASCII | Color |
|---|---|---|---|
| columnas antes de la de la tarjeta | `●` | `#` | verde (`32`) |
| la de la tarjeta, en TE TOCA | `◆` | `!` | amarillo (`33`) |
| la de la tarjeta, en CORRIENDO | `◉` | `*` | cian (`36`) |
| la de la tarjeta, en DETENIDA | `✗` | `x` | rojo (`31`) |
| la de la tarjeta, en EN COLA | `○` | `-` | gris (`90`) |
| columnas después de la de la tarjeta | `○` | `-` | gris (`90`) |

En HECHO los 8 son `●`.

El detalle nombra los 8 pasos del camino con palabras de la persona, una por
columna:

| Columna | Paso del camino |
|---|---|
| Backlog | Pedido |
| Arquitecto | Spec |
| Tu aprobacion | Tu firma |
| Test-writer | Tests |
| Writer | Código |
| Review | Review |
| Tu aceptacion | Tu aceptación |
| Hecho | Hecho |

### El rojo esperado

Después del test-writer, la tarjeta queda en rojo a propósito: sus tests
fallan porque todavía no hay código. Hoy `red` no dice si ese rojo es el
esperado, y la lista lo pondría en DETENIDA como si algo se hubiera roto.

- **`boardcmd.Red` gana `Esperado bool` (`"esperado"` en el JSON)**. Es
  `true` cuando el sobre más nuevo de la tarjeta es el rojo esperado del
  test-writer con la misma regla que usa el piloto (`rojoEsperado` de
  `internal/boardcmd/piloto.go`): una sola definición para los dos.
- `hoom board --json` y el Studio lo reciben, pero el Studio no cambia.
- Una tarjeta con `red.esperado` no cae en DETENIDA. Su AHORA lo dice (ver
  "El detalle").

### El paso de una tarjeta

Es lo que abren `enter` sobre la tarjeta y `n` sobre la tarjeta de la caja.
Es la regla de la v2 (la que abría `n`), con una regla nueva antes de la 4:

1. en Hecho: ninguno, `la tarjeta ya esta en Hecho`;
2. con `running`, o con un piloto de este tablero vivo: ninguno,
   `el <rol> esta trabajando: espera a que termine` (o
   `el piloto lleva la tarjeta: espera a que se detenga`);
3. con `interrupted`: ninguno, `el trabajo se corto: R reanuda o pide de nuevo`;
4. **nueva**: cuando Seguir solo está disponible (ver "Seguir solo"):
   Seguir solo;
5. a 8: las reglas 4 a 7 de la v2 (aprobar en Tu aprobación, integrar en
   Tu aceptación, la primera acción de rol o guardar, o ninguno con su
   texto).

El texto de un paso que no hay, o que está deshabilitado, es el de la v2.

### El detalle

**El borde de arriba** dice el título de lo que muestra a la izquierda y,
a la derecha, para una tarjeta, `<slug>`, ` · riesgo <r>` cuando su riesgo
no es normal, y ` · piloto encendido` cuando su item tiene
`auto: hasta-humano`. Lo que no entra se cae desde el final; el título se
corta con `…` cuando ni el slug entra. Para un hallazgo, `hallazgo <id>`;
para el proyecto, `Proyecto`.

**El detalle de una tarjeta** es un documento, sus partes separadas por una
línea vacía:

**AHORA.** La línea `AHORA` (en gris y negrita) con, pegado a la derecha,
el nombre del grupo en su color (`TE TOCA`, `CORRIENDO`, `DETENIDA`,
`EN COLA` o `HECHO`). Después, una frase en negrita y una línea en gris,
según el primer caso que se cumple:

| Caso | La frase | La línea en gris |
|---|---|---|
| con `running` | según el rol (abajo) | `<provider> · empezo <HH:MM> · <paso>`, con el paso de la v2 |
| un piloto de este tablero vivo, sin `running` | `El piloto está decidiendo el siguiente paso.` | `hoom piloto <slug>` |
| con `interrupted` | `El trabajo del <rol> se cortó en el paso <step> de <steps>.` | `ya no corre: R lo reanuda` |
| un piloto de este tablero que se detuvo, y la tarjeta sigue en la misma columna | `El piloto se detuvo.` | el motivo de su última línea |
| con `red` no esperado | el `plain` del rojo | el `reason` del rojo en modo experto; nada en modo normal |
| con `red` esperado | `Los tests están escritos y fallan, como se espera.` | `falta el código` |
| en Tu aprobación | `El spec está listo y espera tu firma.` | el `plain` |
| en Tu aceptación | `Está lista para que la pruebes y la aceptes.` | el `plain` |
| en Backlog | `Todavía no empezó.` | el `plain` |
| en Hecho | `Terminada.` | el `plain` |
| cualquier otro | el `plain` | nada |

La frase según el rol: arquitecto, `El arquitecto está escribiendo el
spec.`; test-writer, `El test-writer está escribiendo los tests.`; writer,
`El writer está escribiendo el código.`; reviewer, `El reviewer está
revisando el cambio.`; cualquier otro, `El <rol> está trabajando.`.

Con `running` y un límite de tiempo, una tercera línea:
`tiempo <barra> <tiempo> de <limite>`. La barra tiene 20 celdas: las del
tiempo corrido en cian, con `█` (`#` en ASCII), y el resto en gris, con `░`
(`-` en ASCII).

En modo experto, después de esas líneas van las de la v2: `siguiente:
<next>` y una `missing: <...>` por cada una.

**TE TOCA.** La línea `TE TOCA`, en amarillo cuando la tarjeta tiene paso.
Después, `enter <etiqueta en minúscula>` y, en gris, ` · ` y la ayuda de
ese paso:

| Paso | La ayuda |
|---|---|
| Aprobar spec | `hoom no firma por vos` |
| Integrar | `hoom no acepta por vos` |
| Pedir a un rol | `se abre el pedido para revisarlo antes de lanzar` |
| Reanudar | `retoma la sesion que se corto` |
| Guardar | `guarda el trabajo de la tarjeta en su rama` |
| Seguir solo | `el piloto la lleva hasta tu aceptacion o hasta que algo lo frene` |

Sin paso, `nada: ` y el texto de la regla del paso.

**SIGUE.** La línea `SIGUE` y una frase de esta tabla, según la columna de
la tarjeta y si su item tiene `auto: hasta-humano`:

| Columna | Con piloto | Sin piloto |
|---|---|---|
| Backlog | `el arquitecto escribe el spec; despues te toca firmarlo` | lo mismo |
| Arquitecto | `te toca firmar el spec` | lo mismo |
| Tu aprobacion | `al firmar, el piloto pide los tests y sigue solo hasta tu aceptación` | `al firmar, te toca pedir los tests al test-writer` |
| Test-writer | `el piloto pide el código al writer y sigue hasta tu aceptación` | `despues, te toca pedir el código al writer` |
| Writer | `el piloto pide la review; si algo bloquea, hace una ronda de hallazgos` | `despues, te toca pedir la review al reviewer` |
| Review | `si nada bloquea, te toca aceptar; si algo bloquea, el piloto hace una ronda de hallazgos` | `si nada bloquea, te toca aceptar` |
| Tu aceptacion | `al aceptar, hoom integra y la tarjeta pasa a Hecho` | lo mismo |
| Hecho | `nada: la tarjeta esta terminada` | lo mismo |

Sin piloto, en Tu aprobación, Test-writer, Writer y Review, una segunda
línea en gris: `con el piloto seguiria sola: hoom item set <slug> --auto hasta-humano`.

**EL CAMINO.** La línea `EL CAMINO` con `paso <i> de 8` pegado a la
derecha, donde `i` es la posición de la columna de la tarjeta. Después,
una línea por paso del camino: dos espacios, su signo, un espacio y su
nombre. La línea de la columna de la tarjeta suma, en el color de su
grupo, `te toca a vos`, `<rol>, ahora`, `se detuvo` o `sin empezar` (nada
en HECHO), y `← acá` (`<- aca` en ASCII) pegado a la derecha.

**El resto** son las partes B a G de la v2, en su orden y con su
contenido: la columna con el gasto, quién escribió y revisó, el rojo, el
doctor, la línea de pendientes, la tira, los criterios del spec, los
hallazgos de su tarea, la historia y la narración.

**Entre 60 y 99 columnas**, AHORA, TE TOCA, SIGUE y EL CAMINO son una línea
cada uno, sin línea vacía entre ellos:

```
│ AHORA   El spec está listo y espera tu firma.                        TE TOCA │
│ TE TOCA enter aprobar spec · hoom no firma por vos                           │
│ SIGUE   al firmar, el piloto pide los tests y sigue solo hasta tu…           │
│ CAMINO  ●●◆○○○○○ paso 3 de 8: Tu firma                                       │
```

En esta forma, AHORA lleva solo la frase, y cada línea se corta con `…`
en vez de partirse.

**Desplazar y seguir el final.** El detalle se desplaza como el [0] de la
v2. Con `running`, el detalle de esa tarjeta hace lo que hacía el [0] de
[3]: abre mostrando el final de la narración y lo sigue mientras llegan
eventos, salvo que la persona haya subido; `G` vuelve al final.

### Las vistas Hallazgos y Proyecto

- **`H` abre la vista Hallazgos.** La lista pasa a mostrar los hallazgos
  abiertos del proyecto, con las filas, el orden y los textos del [4] de
  la v2. El título de la caja es `Hallazgos <n>`, con ` · <k> high` en rojo
  cuando hay. El detalle muestra el hallazgo elegido como el [0] de [4].
  `esc` o `H` vuelven a las tarjetas, con la tarjeta que estaba elegida.
- **`v` muestra el proyecto en el detalle**: el [0] de [1] de la v2
  (veredicto, check, avisos y doctor), con el foco en el detalle. `esc`
  vuelve a la tarjeta elegida.
- **`/` desde la vista Hallazgos** vuelve a las tarjetas y abre la
  búsqueda.

### El Registro

- **Las entradas son las del criterio 563 de la v2**, sin cambios: la
  hora, la etiqueta, el slug y el estado, más el comando.
- **En pantalla es una línea**, la A-2: la regla de la v2 para el Registro
  plegado, `─Registro · <primera línea de la última entrada>───`, o
  `─Registro───` vacío.
- **`L` abre el modal Registro** con todas las entradas, la más nueva
  abajo, cada una con su comando. Se desplaza como la Ayuda, abre mostrando
  el final, y `esc` o `L` lo cierran. No ejecuta nada.

### Seguir solo

Una acción nueva del tablero, que no está en las acciones de la tarjeta:

| Tecla | Etiqueta | Verbo |
|---|---|---|
| `P` | Seguir solo | `hoom piloto <slug>` |

**Está disponible** cuando se cumple todo esto:

- el item de la tarjeta tiene `auto: hasta-humano`;
- su columna es Test-writer, Writer o Review;
- no tiene `running` ni `interrupted`;
- no tiene un rojo que no sea esperado;
- no tiene un piloto de este tablero vivo;
- y no tiene un piloto de este tablero que se detuvo sin 0 mientras siga
  en la misma columna y no haya corrido otro trabajo en ella.

Cuando no está disponible, `P` no abre nada y la línea de mensajes dice el
primer motivo, sin acentos:

| Motivo | Mensaje |
|---|---|
| sin `auto` | `el piloto no esta encendido en la tarjeta: hoom item set <slug> --auto hasta-humano` |
| otra columna | `en <column_name> el piloto no tiene nada que hacer` |
| con `running` o `interrupted` | `la tarjeta ya tiene un trabajo en curso o cortado` |
| rojo no esperado | `la tarjeta esta detenida: mira que fallo antes de seguir` |
| piloto vivo | `el piloto ya lleva la tarjeta` |
| se detuvo y nada cambió | `el piloto se detuvo: <motivo>` |

**El modal.** `P`, `n` y `enter` abren el modal Seguir solo, con la regla
común de los modales de la v2:

```
╔Seguir solo · Módulo de usuarios══════════════════════════════════╗
║ El piloto lleva la tarjeta sola: tests, codigo, verify, review   ║
║ y rondas de hallazgos, hasta tu aceptacion o hasta que algo      ║
║ lo frene. Nunca firma ni acepta.                                 ║
║                                                                  ║
║ Se va a ejecutar:                                                ║
║ hoom piloto modulo-usuarios                                      ║
║                                                                  ║
║                    [ Seguir solo ]   Cancelar                    ║
╚══════════════════════════════════════════════════════════════════╝
```

`Seguir solo` está elegido al abrir. Solo `enter` sobre él ejecuta.

**Lanzar.** `enter` sobre `Seguir solo` ordena `Lanzar{Args: {"piloto",
<slug>}}`, un proceso propio y desprendido, como los verbos de la v2. El
piloto sigue aunque se cierre el tablero.

**En el Registro**, la entrada tiene la etiqueta `Seguir solo` y el
comando `hoom piloto <slug>`. Su estado:

- `lanzando…` hasta que la tarjeta muestra un `running` con un sobre nuevo;
- `corriendo` después;
- al terminar, si su última línea empieza con
  `piloto automatico: se detiene: `, el estado es `se detuvo: <el resto>`,
  en verde con código 0 y en rojo con otro código;
- si no, la regla de la v2: `hecho` con 0 y `fallo: <ultima>` en rojo con
  otro código.

**Mientras el proceso vive**, la tarjeta está en CORRIENDO (ver "Los
grupos").

**La firma con el piloto encendido.** Cuando la tarjeta de `A` tiene
`auto: hasta-humano`, el modal de firma de la v2 suma, después de
`Se va a ejecutar: hoom aprobar <slug> --si`, la línea
`y despues: hoom piloto <slug>`. Si `hoom aprobar` termina con 0, el
tablero lanza `hoom piloto <slug>` como arriba, con su propia entrada en
el Registro. Si termina con otro código, no lo lanza.

**Lo que el tablero no hace.** No mira el candado del piloto
(`.hoom/cache/locks/piloto-<slug>.lock`). `hoom piloto` lo toma sin
esperar, así que mirarlo podría hacer fallar a un piloto que arranca en ese
instante. Solo sabe de los pilotos que lanzó él mismo (ver "Riesgos").

### Las teclas

Una sola tabla sigue alimentando la línea de teclas, la ayuda y el
despacho. Cambian estas filas; las demás de la v2 siguen igual (las
acciones `a`, `t`, `w`, `r`, `R`, `s`, `A`, `I` y `u`, `espacio`, `e`,
`]c` `[c`, `/`, `x`, `?`, `q` y `ctrl-c`, y todas las de los modales):

| Tecla | Dónde | Hace | Verbo |
|---|---|---|---|
| `tab` `shift-tab` | lista o detalle | pasa el foco a la otra zona | |
| `→` `l` | lista | foco al detalle | |
| `esc` `←` `h` | detalle | foco a la lista | |
| `↑` `↓` `k` `j` | lista, vista Hallazgos | elegir | |
| `↑` `↓` `k` `j` `pgup` `pgdn` | detalle | desplazar | |
| `g` `G` | cualquiera | primera o última; en el detalle, principio o final | |
| `enter` | una tarjeta de la lista | abre el modal de su paso | el de su paso |
| `enter` | la línea de HECHO | la abre o la cierra | |
| `enter` | vista Hallazgos | foco al detalle | |
| `n` | lista, detalle, vista Hallazgos | abre el modal del paso de la tarjeta de la caja | el de ese paso |
| `P` | con tarjeta | seguir solo | `hoom piloto <slug>` |
| `H` | lista, detalle, vista Hallazgos | abrir o cerrar la vista Hallazgos | |
| `v` | lista, detalle | el proyecto en el detalle | |
| `L` | lista, detalle, vista Hallazgos | el modal Registro | |
| `esc` `L` | modal Registro | cerrarlo | |

- **Se van `1`, `2`, `3`, `4` y `0`.** No hay paneles numerados, así que no
  están en la ayuda ni hacen nada.
- **`enter` ya no "ve en grande": hace el paso.** Abre el modal del paso,
  nunca ejecuta: la confirmación sigue siendo el botón del modal. Sin paso,
  la línea de mensajes dice el texto de la regla del paso y nada se abre.
  Ver la tarjeta entera es `tab` o `→`.
- **`n` es la tarjeta de la caja**, elegida o no. `enter` es la tarjeta
  elegida. Cuando son la misma, hacen lo mismo.

**La línea de teclas** empieza siempre por lo que haría `enter` sobre lo
elegido, en negrita: `enter <etiqueta del paso en minúscula>` sobre una
tarjeta con paso, `enter abrir` o `enter cerrar` sobre HECHO, `enter ver`
en la vista Hallazgos. Sobre una tarjeta sin paso no hay pista de `enter`.
Siguen, unidas por ` · `:

- en la lista: `n tu siguiente paso`, `↑↓ elegir`, `tab detalle`,
  `espacio acciones`, `H hallazgos`, `v proyecto` y `? todas las teclas`;
- en el detalle: `n tu siguiente paso`, `↑↓ desplazar`, `esc volver`,
  `espacio acciones` y `? todas las teclas`;
- en la vista Hallazgos: `↑↓ elegir`, `e editor`, `esc volver` y
  `? todas las teclas`.

Sin tarjeta, las pistas de la tarjeta (`espacio`) se quitan. Cuando la
línea no entra, se aplica la regla de la v2: `? todas las teclas` pasa a
`? todas` y se quitan las pistas de antes de la última, de atrás para
adelante. La primera, la de `enter`, no se quita nunca.

**La ayuda** agrupa las teclas en `Tarjetas`, `Detalle`, `Hallazgos`,
`Modales` y `Mouse`, primero el grupo de donde está el foco.

### El mouse

- **Lo que pide y suelta el tablero no cambia**, ni `HOOM_MOUSE=0`.
- **En la lista**, un clic elige la tarjeta bajo el clic (en cualquiera de
  sus dos líneas) y le da el foco a la lista. Un clic en una línea de grupo
  solo le da el foco.
- **En el detalle**, un clic le da el foco.
- **En la caja**, un clic elige en la lista la tarjeta de la caja.
- **La rueda** mueve la selección de la lista o desplaza el detalle, según
  dónde está el puntero.
- **En un modal**, lo de la v2.
- **Ningún evento del mouse ejecuta ni abre un modal**, tampoco sobre la
  línea del Registro.

### Los nombres de la v2

Los criterios de la v2 que siguen igual nombran sus paneles. Con este spec
se leen así:

| En la v2 | Ahora |
|---|---|
| [2] | la lista de tarjetas |
| [0] | el detalle |
| [0] de una tarjeta de [2] | el detalle de la tarjeta elegida |
| [1] | la cabecera |
| [0] de [1] | el detalle con `v` |
| [4] y su [0] | la vista Hallazgos |
| [3] | el grupo CORRIENDO |
| el foco en [0] | el foco en el detalle |
| "vuelve a [2]" | "vuelve a la lista" |

### Cómo se prueba

- **El modelo sigue siendo puro**, como en la v2.
- **Archivos dorados** a 120x36, 100x30, 80x24 y 60x15, sin color y en
  ASCII, y una vez con signos UTF-8. Con color, una prueba por elemento mira
  sus secuencias SGR: el borde de la caja, cada grupo, cada signo del camino,
  la barra del tiempo y la primera pista de la línea de teclas.
- **Los archivos dorados de la v2 se reemplazan.** Es el cambio de pantalla
  que este spec aprueba, no un test debilitado.
- **Los tests de la v2 que citan un criterio re-expresado** se adaptan al
  comportamiento nuevo y citan los dos ids. Los tests de los criterios que
  siguen igual solo cambian dónde miran la pantalla, con la tabla de "Los
  nombres de la v2", nunca lo que aseveran.
- **`red.esperado`** se prueba en `internal/boardcmd` con los mismos sobres
  que prueban el rojo esperado del piloto.
- **Seguir solo** pasa por la interfaz de lanzar de la v2, sin ejecutar
  nada: el modelo recibe `FinProceso` con cada código y cada última línea.
- **Ningún test necesita una terminal real.**

### La dependencia

Sin cambios: `golang.org/x/term` y nada más.

### Documentación

La sección "El tablero en la terminal" del README se reescribe con:

- la caja TU SIGUIENTE PASO y los grupos por quién tiene la pelota;
- el detalle en orden AHORA, TE TOCA, SIGUE y EL CAMINO;
- `enter`, `n`, `P` (Seguir solo, que sigue aunque cierres el tablero),
  `H`, `v` y `L`.

La fila de `hoom tablero` en la tabla de comandos deja de decir "en
paneles".

## Casos límite y errores esperados

- **Terminal de 80x24**: una columna, la lista arriba con el foco.

  ```
   hoom tablero · oceanbuild · rama main             check VERDE · verify hace 2h
  ┌TU SIGUIENTE PASO────────────────────────te toca 2 · corriendo 1 · detenida 1─┐
  │ Firmá el spec de «Exportar reporte a PDF».  te toca aprobar…                 │
  │ n aprobar spec               después: «Listado de proyectos» · «Notificaci…» │
  └──────────────────────────────────────────────────────────────────────────────┘
  ╔Tarjetas══════════════════════════════════════════════════════════════════════╗
  ║ TE TOCA 2                                                                    ║
  ║ ▸ Exportar reporte a PDF         ●●◆○○○○○ te toca aprobar el spec            ║
  ║   Listado de proyectos           ●●●●●●◆○ lista para tu aceptacion           ║
  ║ CORRIENDO 1                                                                  ║
  ║   Módulo de usuarios             ●●●●◉○○○ writer · 14:02 de 30:00            ║
  ║ DETENIDA 1                                                                   ║
  ║   Notificaciones por correo      ●●●●✗○○○ ✗ el veredicto quedo rojo en test  ║
  ╚══════════════════════════════════════════════════════════════════════1 de 6═╝
  ┌Exportar reporte a PDF───────────────────────────────────exportar-reporte-pdf─┐
  │ AHORA   El spec está listo y espera tu firma.                        TE TOCA │
  │ TE TOCA enter aprobar spec · hoom no firma por vos                           │
  │ SIGUE   al firmar, el piloto pide los tests y sigue solo hasta tu…           │
  │ CAMINO  ●●◆○○○○○ paso 3 de 8: Tu firma                                       │
  │                                                                              │
  │ Tu aprobacion · gasto 0.42 USD                                               │
  └──────────────────────────────────────────────────────────────────────────────┘
  ─Registro · 09:58 Pedir al arquitecto · exportar-reporte-pdf · hecho────────────

   enter aprobar spec · n tu siguiente paso · ↑↓ elegir · tab detalle · ? todas
  ```

- **Todo corre solo**: la caja dice `Nada te espera.`, con el borde en
  gris, y `n` no abre nada.
- **Nada empezado y nada en curso**: la caja ofrece la primera de EN COLA
  (`Nada en curso. Podés empezar «<titulo>».`) y `n` abre su paso.
- **Sin tarjetas**: la caja dice `No hay nada pendiente.` y la lista
  `no hay tarjetas: hoom item add "<titulo>"`, como en la v2.
- **El rojo esperado del test-writer**: la tarjeta está en TE TOCA (sin
  piloto, con `enter pedir al writer`) o en CORRIENDO (con su piloto), nunca
  en DETENIDA. Su AHORA dice `Los tests están escritos y fallan, como se
  espera.`
- **Dos pilotos para la misma tarjeta.** Uno que lanzó otro proceso (el
  Studio, otra terminal) no lo ve este tablero. Entre trabajo y trabajo, la
  tarjeta puede quedar un momento en TE TOCA con Seguir solo. Si la persona
  lo confirma, el segundo `hoom piloto` sale con
  `el piloto de <slug> ya corre en otro proceso` y el Registro lo muestra
  en rojo. No lanza nada.
- **El piloto se detiene con 1** (un rojo final, un tope): la tarjeta queda
  en DETENIDA o en TE TOCA según su evidencia. Su AHORA dice
  `El piloto se detuvo.` con el motivo, y Seguir solo no se ofrece de nuevo
  hasta que la tarjeta cambie de columna o corra otro trabajo en ella.
- **`hoom aprobar` falla con el piloto encendido**: el Registro muestra el
  fallo de la firma y no hay entrada de Seguir solo.
- **Se cierra el tablero con un piloto corriendo**: el piloto sigue. Al
  abrir de nuevo, la tarjeta se ve en CORRIENDO mientras haya un trabajo en
  curso, y entre trabajo y trabajo pasa lo de "Dos pilotos".
- **La tarjeta de la caja desaparece en una lectura**: la caja pasa a la
  siguiente con la regla de siempre. Si la elegida desaparece, vale el
  criterio 578 de la v2.
- **Un título con caracteres anchos o marcas combinantes**: se mide y se
  corta con la tabla de anchos de la v2.
- **Un título vacío**: la fila y la caja usan el slug en su lugar.
- **La vista Hallazgos sin hallazgos**: dice `sin hallazgos abiertos`, como
  la v2.
- **Con `NO_COLOR`**: la caja, los grupos y el camino se distinguen por sus
  palabras y signos; el borde de la caja es la línea simple.
- **Con `HOOM_ASCII=1`**: los signos del camino son `#`, `!`, `*`, `x` y
  `-`; `←` es `<-`, la barra del tiempo usa `#` y `-`, y `«»` son `"`.

## Criterios de aceptación

- CA-696: con 100 columnas o más la pantalla tiene, de arriba hacia abajo, la cabecera, la caja TU SIGUIENTE PASO de 4 líneas, el cuerpo con la lista a la izquierda en `W*2/5` celdas y el detalle a la derecha, el Registro en una línea, la línea de mensajes y la línea de teclas; entre 60 y 99 columnas el cuerpo es una columna con la lista arriba (hasta la mitad del cuerpo, al menos 3 líneas) y el detalle abajo, y con el foco en el detalle la lista se pliega a una línea; debajo de 60x15 vale la regla de la v2 (archivos dorados a 120x36, 100x30, 80x24 y 60x15). Re-expresa CA-550.
- CA-697: la cabecera es una línea con `hoom tablero · <proyecto> · rama <rama>` (o `sin git`) a la izquierda y, a la derecha, la línea del check de la v2, el gasto de hoy y la tarjeta fijada; lo que no entra se cae en el orden fijada, hoy y por último se corta el check, y antes de la primera lectura dice `leyendo el proyecto...`.
- CA-698: la caja muestra la primera tarjeta de TE TOCA en el orden de la lista, o la primera de DETENIDA si TE TOCA está vacía; su primera línea es la frase de la tabla de este spec para el paso de esa tarjeta, con el título entre `«»`, y su `plain` en gris; su segunda línea es `n` con la etiqueta del paso en minúscula (o el texto de la regla del paso) y, a la derecha, `después:` con los títulos de las siguientes de TE TOCA y DETENIDA que entran.
- CA-699: el borde de arriba de la caja dice `TU SIGUIENTE PASO` y `te toca <a> · corriendo <b> · detenida <c>` con los tres números; con color va en amarillo cuando TE TOCA o DETENIDA tienen alguna tarjeta y en gris cuando no; sin ninguna de las dos, la caja dice `Nada te espera. hoom trabaja en <n> tarjeta(s).` con una tarjeta en CORRIENDO, `Nada en curso. Podés empezar «<titulo>».` con la primera de EN COLA, o `No hay nada pendiente.`, con la segunda línea de este spec para cada caso.
- CA-700: cada tarjeta cae en un solo grupo, el primero que se cumple de HECHO (columna Hecho), CORRIENDO (`running` o un piloto de este tablero vivo), DETENIDA (`interrupted`, o `red` con `esperado` en `false`), EN COLA (columna Backlog) y TE TOCA (las demás); la lista los muestra en el orden TE TOCA, CORRIENDO, DETENIDA, EN COLA y HECHO, sin grupos vacíos salvo HECHO, que empieza cerrado; TE TOCA va de Tu aceptación a Arquitecto por columna y, dentro de una columna y en los otros grupos, en el orden del tablero; con color, el nombre de TE TOCA, CORRIENDO y DETENIDA va en video inverso de amarillo, cian y rojo, y EN COLA y HECHO en gris. Re-expresa CA-555.
- CA-701: la primera línea de una tarjeta lleva la marca, el título (nunca el slug) cortado con `…` y el camino de 8 signos pegado al borde derecho, y va sin camino cuando al título le quedan menos de 8 celdas; con 100 columnas o más hay una segunda línea con el estado de la tabla de este spec en el color del grupo, y entre 60 y 99 columnas el estado va en la misma línea después del camino, con el camino en la misma columna en todas las filas; la selección es `▸` y, con foco y color, la barra inversa sobre la primera línea. Re-expresa CA-555.
- CA-702: el camino tiene un signo por columna del tablero en su orden: `●` en verde antes de la columna de la tarjeta, `◆` amarillo, `◉` cian, `✗` rojo u `○` gris en la suya según su grupo (TE TOCA, CORRIENDO, DETENIDA o EN COLA), `○` gris después, y 8 `●` en HECHO; en ASCII los signos son `#`, `!`, `*`, `x` y `-`.
- CA-703: `boardcmd.Red` tiene `Esperado` (`"esperado"` en el JSON de `hoom board --json`), en `true` solo cuando el sobre más nuevo de la tarjeta es el rojo esperado del test-writer según la misma función que usa el piloto, y en `false` en cualquier otro rojo.
- CA-704: el paso de una tarjeta es la regla del siguiente paso de la v2 con una regla nueva antes de aprobar e integrar: cuando Seguir solo está disponible, el paso es Seguir solo; con un piloto de este tablero vivo no hay paso y el texto es `el piloto lleva la tarjeta: espera a que se detenga`.
- CA-705: `enter` sobre una tarjeta de la lista abre el modal de su paso, sea de pedir, de firma, de guardar o de Seguir solo, y nunca lanza; sin paso, la línea de mensajes dice el texto de la regla del paso y nada se abre; `n` hace lo mismo con la tarjeta de la caja, esté elegida o no, y sin tarjeta en la caja dice el texto de este spec para ese caso; toda tecla de acción salvo `u` produce como primera orden un modal y nunca un lanzamiento. Re-expresa CA-566.
- CA-706: siempre hay una zona con foco, la lista o el detalle, con el borde del foco de la v2, y al abrir es la lista; `tab` y `shift-tab` pasan el foco a la otra zona, `→` y `l` lo pasan de la lista al detalle y `esc`, `←` y `h` del detalle a la lista; `1`, `2`, `3`, `4` y `0` no hacen nada y no están en la ayuda. Re-expresa CA-551 y CA-553.
- CA-707: en la lista y en la vista Hallazgos `↑`, `↓`, `k` y `j` mueven la selección y `g` y `G` van a la primera y a la última, saltando las líneas de grupo salvo la de HECHO; en el detalle esas teclas y `pgup` y `pgdn` lo desplazan; la lista se desplaza para que la tarjeta elegida se vea entera y su borde de abajo dice `<n> de <m>` cuando no entra; el detalle cambia con cada movimiento de la selección, sin `enter`. Re-expresa CA-552 y CA-553.
- CA-708: el detalle de una tarjeta muestra, en este orden, AHORA (con el nombre del grupo pegado a la derecha, la frase y la línea gris del primer caso de la tabla de este spec, y la barra de 20 celdas del tiempo con `running` y límite), TE TOCA (`enter` con la etiqueta del paso y su ayuda, o `nada: ` con el texto de la regla), SIGUE (la frase de la tabla por columna y piloto), EL CAMINO (los 8 pasos con su signo, `paso <i> de 8` y `← acá` en el de la tarjeta) y después las partes B a G de la v2; entre 60 y 99 columnas las cuatro primeras son una línea cada una. Re-expresa CA-556.
- CA-709: con `running` el AHORA dice la frase de su rol (`El writer está escribiendo el código.` y las demás de este spec) y `<provider> · empezo <HH:MM> · <paso>`; con `interrupted`, `El trabajo del <rol> se cortó en el paso <step> de <steps>.`; con `red` esperado, `Los tests están escritos y fallan, como se espera.`; con `red` no esperado, el `plain` del rojo; en Tu aprobación, Tu aceptación, Backlog y Hecho, las frases de la tabla de este spec.
- CA-710: la frase de SIGUE sale de la columna de la tarjeta y de si su item tiene `auto: hasta-humano`, con la tabla de este spec; sin piloto, en Tu aprobación, Test-writer, Writer y Review, suma la línea `con el piloto seguiria sola: hoom item set <slug> --auto hasta-humano`.
- CA-711: el grupo CORRIENDO tiene las tarjetas con `running` o con un piloto de este tablero vivo, cada una con `<rol> · <tiempo>[ de <limite>]` (con `el piloto la lleva · ` delante cuando corresponde, o `el piloto la lleva · decidiendo` sin trabajo), y las tarjetas con `interrupted` van a DETENIDA en rojo; el detalle de una tarjeta con `running` abre mostrando el final de su narración y lo sigue mientras llegan eventos, salvo que la persona haya subido, y `G` vuelve al final. Re-expresa CA-558.
- CA-712: `H` cambia la lista por los hallazgos abiertos del proyecto, con las filas, el orden y el detalle del [4] de la v2, con el título `Hallazgos <n>` y ` · <k> high` en rojo cuando hay; `esc` o `H` vuelven a las tarjetas con la que estaba elegida, y `/` desde ahí vuelve a las tarjetas y abre la búsqueda.
- CA-713: `v` muestra en el detalle el [0] de [1] de la v2 (veredicto, check, avisos y doctor) con el título `Proyecto` y el foco en el detalle, y `esc` vuelve a la tarjeta elegida.
- CA-714: el Registro ocupa una línea con la primera línea de su última entrada, como el Registro plegado de la v2; `L` abre el modal Registro con todas las entradas y sus comandos, la más nueva abajo, que abre mostrando el final, se desplaza como la Ayuda, se cierra con `esc` o `L` y no ejecuta nada.
- CA-715: `P` abre el modal Seguir solo solo cuando la tarjeta tiene `auto: hasta-humano`, está en Test-writer, Writer o Review, no tiene `running`, `interrupted`, un rojo no esperado ni un piloto de este tablero vivo, y su último piloto de este tablero no se detuvo sin 0 en la misma columna sin otro trabajo después; si no, la línea de mensajes dice el primer motivo de la tabla de este spec y nada se abre.
- CA-716: el modal Seguir solo dice qué hace el piloto, `Se va a ejecutar:` y `hoom piloto <slug>`, con `Seguir solo` elegido al abrir y `Cancelar`; solo `enter` sobre `Seguir solo` ordena `Lanzar{Args: {"piloto", <slug>}}`, desprendido; su entrada del Registro tiene la etiqueta `Seguir solo` y pasa por `lanzando…`, `corriendo` y, al terminar, `se detuvo: <motivo>` (verde con 0, rojo con otro código) cuando su última línea empieza con `piloto automatico: se detiene: `, o la regla de la v2 si no; mientras su proceso vive, la tarjeta está en CORRIENDO.
- CA-717: con `auto: hasta-humano`, el modal de firma de `A` suma `y despues: hoom piloto <slug>` debajo de `Se va a ejecutar: hoom aprobar <slug> --si`; si `hoom aprobar` termina con 0 el tablero lanza `hoom piloto <slug>` con su propia entrada del Registro, y si termina con otro código no lo lanza; sin `auto`, el modal de firma es el de la v2. Re-expresa CA-574.
- CA-718: la línea de teclas empieza con la pista de `enter` para lo elegido, en negrita (`enter <etiqueta del paso>`, `enter abrir` o `enter cerrar` en HECHO, `enter ver` en la vista Hallazgos, ninguna sin paso), sigue con las pistas de este spec para la lista, el detalle o la vista Hallazgos, y al no entrar se acorta con la regla de la v2 sin quitar nunca la de `enter`; `?` abre la ayuda con los grupos `Tarjetas`, `Detalle`, `Hallazgos`, `Modales` y `Mouse`, primero el de donde está el foco, y la línea, la ayuda y el despacho salen de una sola tabla. Re-expresa CA-564.
- CA-719: un clic en la lista elige la tarjeta bajo el clic, en cualquiera de sus dos líneas, y le da el foco a la lista; un clic en el detalle le da el foco; un clic en la caja elige en la lista la tarjeta de la caja; la rueda mueve la selección de la lista o desplaza el detalle según dónde está el puntero; ningún evento del mouse ejecuta ni abre un modal; lo que se pide y se apaga al salir sigue igual. Re-expresa CA-575.
- CA-720: con `NO_COLOR` la caja, los grupos y el camino no tienen secuencias de color y se distinguen por sus palabras y signos; con `HOOM_ASCII=1` o sin un locale UTF-8, el camino usa `#`, `!`, `*`, `x` y `-`, `←` es `<-`, la barra del tiempo usa `#` y `-`, `«` y `»` son `"` y las frases de la caja pierden sus acentos (archivos dorados de los dos modos).
- CA-721: el README describe la caja TU SIGUIENTE PASO, los grupos, el orden del detalle y las teclas `enter`, `n`, `P`, `H`, `v` y `L`, y dice que Seguir solo sigue aunque se cierre el tablero. [verifica: grep -q "TU SIGUIENTE PASO" README.md && grep -q "Seguir solo" README.md && grep -q "hoom piloto" README.md]

## Decisiones

Todas son mías y Henry no las revisó todavía. Las que más cambian el uso
van primero.

- **Tres zonas en vez de seis.** La caja, la lista y el detalle. Proyecto
  pasa a la cabecera y a `v`, Hallazgos a `H`, Corriendo se funde en la
  lista y el Registro queda en una línea con `L` para verlo entero. Es lo
  que el lienzo propone y lo que contesta "no sé qué debo hacer".
- **`enter` hace el paso y `tab` ve el detalle.** En la v2, `enter` veía en
  grande. El cambio es fuerte para quien ya usa la v2, pero un junior
  aprieta `enter` sobre lo que quiere hacer. Es seguro porque `enter` solo
  abre el modal: ejecuta el botón del modal, como siempre.
- **`n` es la caja, no la tarjeta elegida.** Así `n` es siempre "lo que me
  toca", desde donde esté. `enter` cubre la tarjeta elegida.
- **La pelota manda el orden.** TE TOCA va primero y, dentro, de Tu
  aceptación hacia atrás: terminar lo empezado antes de empezar lo nuevo.
  DETENIDA va después de CORRIENDO porque la caja ya la ofrece cuando no
  hay nada en TE TOCA.
- **El rojo esperado va en `boardcmd` y no en el tablero.** Es la regla del
  piloto. Si cada piel la copiara, la lista y el piloto podrían discrepar.
- **Seguir solo desde el tablero, y la firma encadena el piloto.** Sin eso,
  una tarjeta con piloto obliga a salir a otra terminal a correr `hoom
  piloto`, que es justo lo que hoy lleva a Claude Code Desktop. El Studio ya
  hace lo mismo al aprobar desde la tarjeta.
- **El tablero no mira el candado del piloto.** `hoom piloto` lo toma sin
  esperar (`hoomfs.Lock` con 0), así que mirarlo podría hacer fallar a un
  piloto que arranca justo entonces. Se prefiere un segundo piloto que sale
  solo a un primero que no arranca.
- **`◉` para CORRIENDO, no `▶`.** `▶` (U+25B6) tiene presentación de emoji
  en algunas terminales y ocuparía dos celdas.
- **El camino con palabras de la persona** (Pedido, Spec, Tu firma, Tests,
  Código, Review, Tu aceptación, Hecho), como el Studio nuevo. Los nombres
  de columna siguen en los mensajes de la v2 (`en <column_name> no hay...`).
- **El orden de las cuatro primeras partes del detalle es fijo** (AHORA, TE
  TOCA, SIGUE, EL CAMINO), en las dos formas, para que la mirada aprenda
  dónde está cada cosa.
- **Las frases fijas** (la caja, AHORA, SIGUE) están en el spec para que la
  review pueda comparar la pantalla con lo que se aprobó. Van en voseo, como
  pide la línea hoomdev, y los mensajes siguen sin acentos, como en la v2.

## Riesgos y deuda aceptada

- **`enter` cambia de significado.** Quien venga de la v2 va a abrir un
  modal donde esperaba ver la tarjeta. El modal se cierra con `esc` y no
  ejecuta nada; la ayuda y la línea de teclas lo dicen.
- **El tablero solo conoce sus pilotos.** Un piloto lanzado por el Studio o
  por otra terminal no se ve entre trabajo y trabajo, y al reabrir el
  tablero tampoco se ven los suyos. El peor caso es un segundo `hoom piloto`
  que sale con `ya corre en otro proceso`. Arreglarlo pide que `hoom piloto`
  espere un poco el candado: es otro spec.
- **El motivo de parada del piloto vive en la memoria del tablero.** Al
  cerrarlo, queda en el log del sobre más nuevo (la narración del detalle),
  pero el AHORA ya no lo dice.
- **Las frases de SIGUE copian la tabla del piloto en palabras.** Si
  `autonomia-entre-firmas` cambia lo que hace el piloto después de una
  columna, esta tabla queda vieja. Es texto de este spec y se cambia con él.
- **Dos líneas por tarjeta entran la mitad.** Con muchas tarjetas, la lista
  se desplaza más que en la v2. Lo compensa que HECHO empieza cerrado.
- **La lista más ancha (2/5) achica el detalle.** A 100 columnas el detalle
  tiene 58 celdas por dentro, y las partes largas de la v2 (criterios,
  narración) se parten en más líneas.
- **Los signos `◆`, `◉`, `●` y `○` son de ancho ambiguo.** En terminales con
  ancho ambiguo doble, el camino se puede correr. Ya pasaba con `●` y `○` en
  la tira de la v2; `HOOM_ASCII=1` lo evita.
- **Más estado en el modelo**: los pilotos vivos, sus paradas y qué entrada
  del Registro es de cuál. Lo contienen el modelo puro y los tests con
  `FinProceso`.
