# Spec: lo pegado es texto en el tablero (`hoom tablero`)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start pegado-como-texto`.
Depende de: `tablero-con-paneles.md` (la v2) integrada; hoy está en la rama
`hoom/tablero-con-paneles`.
Origen: el hallazgo `20261109T184531_6ebce5` (medium, PRE-EXISTENTE) de la
review de la v2. Ya pasaba en la v1 (`tablero-en-la-terminal.md`).

## Objetivo

La TUI no le pide a la terminal que marque lo pegado, así que un pegado
llega como teclas:

- pegar `linea1\rlinea2\r` en el Pedido lanza `hoom pedir -m "linea1"`: el
  primer `\r` lleva el foco al botón y el segundo lo ejecuta;
- pegar `A`, un tab y un enter sobre una tarjeta en Tu aprobación firma con
  `hoom aprobar --si`.

Eso rompe "nada a ciegas". Con este spec la TUI pide el **modo de pegado**
de la terminal (bracketed paste, `\x1b[?2004h`). Lo que llega entre
`ESC [ 200 ~` y `ESC [ 201 ~` es un **pegado**: texto, nunca teclas. Solo
lo toma un campo de texto.

**Regla.** Un pegado nunca ejecuta un verbo, nunca abre ni cierra un modal,
nunca mueve el foco ni un botón y nunca sale de la TUI.

## No-goals

- Adivinar un pegado sin marcas, o exigir una pausa antes de que `enter`
  ejecute (ver Decisiones).
- Copiar al portapapeles, OSC 52, pegar imágenes o archivos.
- El Studio (el navegador ya pega texto) y el editor externo.
- Cambiar alguna tecla de la v2.

## Contratos

### El modo

- `SecPegadoSi = "\x1b[?2004h"` y `SecPegadoNo = "\x1b[?2004l"`. Se pide
  siempre, sin variable para apagarlo.
- Tomar la terminal es una escritura de `SecEntrar`, `SecMouseSi` (salvo
  `HOOM_MOUSE=0`) y `SecPegadoSi`. Devolverla es una escritura en el orden
  inverso: `SecPegadoNo`, `SecMouseNo` (salvo `HOOM_MOUSE=0`) y `SecSalir`.
- Se devuelve en los caminos de la v2: `q`, `ctrl-c`, un error al leer o
  escribir la terminal (fin de archivo incluido), un panic, SIGTERM, SIGHUP
  y SIGINT (`os.Interrupt` en Windows) y la falla de la escritura que la
  toma.
- Alrededor del editor (`AbrirEditor`, `EditarPedido`), como el mouse: la
  devuelve y la retoma con esas mismas dos escrituras.

### El pegado

- `Entrada` suma `Pegado string`; exactamente uno de `Tecla`, `Raton` y
  `Pegado` está puesto.
- `DecodificarEntradas` da como **una** `Entrada` los bytes entre
  `ESC [ 200 ~` y el primer `ESC [ 201 ~`, tal cual. Nada de adentro es
  tecla ni evento del mouse: ni `\r`, `\n`, tab, `0x03` (ctrl-c), `ESC`,
  una secuencia del mouse ni otro `ESC [ 200 ~`.
- Un pegado vacío no da entrada; un `ESC [ 201 ~` suelto no da nada;
  `Decodificar` (solo teclas) no da pegados.
- Una lectura puede cortar un pegado o sus marcas en cualquier byte. Un
  pegado abierto, y una marca de inicio cortada desde `ESC [ 2`, esperan el
  resto sin límite de tiempo, como `ESC [ <` del mouse: nunca se vuelven
  teclas por tiempo. `ESC` y `ESC [` solos siguen con la espera corta de la
  v2, porque también son `esc` y alt-`[`.
- `func (m *Modelo) Pegar(texto string, ahora time.Time)`, **sin órdenes**.
  Run le entrega cada pegado.

### Qué hace un pegado en cada lugar

| Lugar | Hace |
|---|---|
| el Pedido (`LugarPedirPedido`) | inserta en el cursor; los finales de línea quedan como `\n` y los tabs quedan |
| la búsqueda (`LugarBusqueda`) y `otro…` (`LugarPedirTexto`) | inserta en el cursor; cada final de línea y cada tab es un espacio |
| cualquier otro | nada; la línea de mensajes dice `lo pegado se ignoro: no hay un campo de texto con el foco` (`DuracionAviso`) |

- En un campo, pegar deja el campo, el cursor y la pantalla **igual que
  escribir lo pegado tecla por tecla**, salvo los finales de línea
  (`\r\n`, `\r`, `\n`) y los tabs de la tabla.
- Lo que el teclado descarta se descarta: los otros controles C0, DEL, C1,
  los controles bidi y los bytes que no son UTF-8.
- Sigue la v2: la caja del Pedido muestra los saltos como saltos,
  `Se va a ejecutar` y el Registro como espacios, y `-m` los lleva. La
  búsqueda filtra y sigue abierta; `otro…` sigue en texto hasta un `enter`
  tecleado.
- **Tope**: un pegado de más de 64 KiB (`TopePegado`, en bytes) no entra en
  ningún campo, y la línea de mensajes dice en rojo `lo pegado no se uso:
  pasa de 64 KB`. Con la búsqueda abierta, ese aviso sigue la regla de la
  v2: se ve al cerrarla, si no venció.

### Una terminal sin el modo

Lo ignora y manda lo pegado sin marcas: llega como teclas, como hoy, y la
TUI no lo adivina. Quedan las defensas de la v2 (`enter` en el Pedido no
lanza, la firma abre en `Cancelar`, `Se va a ejecutar` antes de todo), pero
`A`, un tab y un enter pegados todavía firman (Riesgos).

### Documentación y pruebas

- El README, en "El tablero en la terminal", suma "Lo pegado es texto":
  solo entra en el campo con el foco, fuera se ignora con aviso, y sin
  bracketed paste llega como teclas.
- El decodificador se prueba con bytes; el modelo, con `Pegar` en cada
  `Lugar` y comparando contra escribir tecla por tecla; Run, con la
  terminal de los tests de la v2. Ningún test necesita una terminal real.
- Los tests de la v2 que comparan los bytes exactos al tomar o devolver la
  terminal (CA-533, CA-575) esperan también las secuencias nuevas y citan
  los dos ids. Es el cambio que este spec aprueba, no un test debilitado.

## Casos límite y errores esperados

- **Modal de pedir con el foco en un selector o un botón**: se ignora con
  el aviso; el foco no salta al Pedido. El reviewer, sin Pedido, solo toma
  lo pegado en un `otro…`.
- **Una línea copiada entera**, con su salto al final: en el Pedido queda
  el `\n` y en la búsqueda un espacio. Ninguno confirma.
- **Un portapapeles con `ESC [ 201 ~` adentro**: el pegado termina ahí y lo
  que sigue llega como teclas (Riesgos).
- **Un pegado que nunca termina**: lo que se teclea después va al pegado y
  `q` no sale; las señales y cerrar la terminal sí.
- **Un editor que pide el modo por su cuenta** (vim, nvim): lo maneja él;
  al volver, la TUI lo pide de nuevo.
- **SIGKILL**: el modo queda pedido, como el mouse; `reset` lo apaga.
- **Windows**: depende de la consola; sin soporte, es una terminal sin el
  modo.

## Criterios de aceptación

- CA-580: Run toma la terminal con una sola escritura de `SecEntrar`, `SecMouseSi` (salvo `HOOM_MOUSE=0`) y `SecPegadoSi = "\x1b[?2004h"`, en ese orden, y la devuelve con una sola escritura de `SecPegadoNo = "\x1b[?2004l"`, `SecMouseNo` (salvo `HOOM_MOUSE=0`) y `SecSalir` al salir con `q`, con `ctrl-c`, con un error al leer o escribir la terminal (fin de archivo incluido), con un panic, con SIGTERM, SIGHUP o SIGINT y cuando falla la escritura que la toma; alrededor de `AbrirEditor` y `EditarPedido` la devuelve y la retoma con esas mismas dos escrituras. Amplía CA-533 y CA-575.
- CA-581: `DecodificarEntradas` da lo que hay entre `ESC [ 200 ~` y el primer `ESC [ 201 ~` como una sola `Entrada` con `Pegado` igual a esos bytes, sin ninguna tecla ni evento del mouse por los `\r`, `\n`, tabs, `0x03`, `ESC`, secuencias del mouse u otro `ESC [ 200 ~` que traiga adentro; un pegado vacío no da entrada, un `ESC [ 201 ~` suelto no da nada y `Decodificar` no da pegados.
- CA-582: con la terminal de los tests, cortar los bytes de un pegado (sus marcas incluidas) en cualquier byte y en cualquier cantidad de lecturas da el mismo único pegado; un pegado abierto, o una marca de inicio cortada desde `ESC [ 2`, espera el resto sin límite de tiempo sin entregar nada como teclas, y `ESC` y `ESC [` solos siguen con la espera corta de la v2.
- CA-583: con el foco en el Pedido, en la búsqueda o en `otro…`, `Pegar` deja el campo, el cursor y la pantalla igual que escribir el texto tecla por tecla, con `\r\n`, `\r` y `\n` como `\n` y los tabs tal cual en el Pedido, y como un espacio cada uno en la búsqueda y en `otro…`; se descartan los demás controles C0, DEL, C1, los controles bidi y los bytes no UTF-8; el `Lanzar` que sigue lleva en `-m` el pedido con sus `\n`, y la búsqueda sigue abierta y `otro…` sin confirmarse.
- CA-584: en cualquier otro `Lugar` (cada panel, [0], Firma, Guardar, Acciones, Ayuda, un selector, una lista abierta y los botones de pedir), `Pegar` no cambia nada salvo la línea de mensajes, que dice `lo pegado se ignoro: no hay un campo de texto con el foco` durante `DuracionAviso`; un pegado vacío no cambia nada.
- CA-585: un pegado de más de 64 KiB no entra en ningún campo, el campo queda como estaba y la línea de mensajes dice en rojo `lo pegado no se uso: pasa de 64 KB` (con la búsqueda abierta, al cerrarla, si el aviso no venció).
- CA-586: `Modelo.Pegar` no devuelve órdenes, y después de un pegado en cualquier `Lugar` siguen iguales el `Lugar`, el foco, el modal, la lista abierta, el botón elegido, la tarjeta elegida (salvo en la búsqueda, donde el filtro la mueve como al escribir) y el Registro; con la terminal de los tests, pegar `linea1\rlinea2\r` en el modal de pedir y pegar `A`, un tab y un enter sobre una tarjeta en Tu aprobación no ejecutan ningún verbo (los dos casos del hallazgo `20261109T184531_6ebce5`).
- CA-587: el README explica que lo pegado es texto y qué pasa sin bracketed paste. [verifica: grep -q "Lo pegado es texto" README.md && grep -q "bracketed paste" README.md]

## Decisiones

- **El modo de la terminal, no una heurística.** DECSET 2004 viene de
  xterm y lo adoptó la mayoría de las terminales de hoy. Lo marcado es un
  pegado sin adivinar.
- **Siempre pedido.** El mouse tiene `HOOM_MOUSE=0` porque cuesta (para
  seleccionar hay que mantener Shift); el modo de pegado no le quita nada a
  nadie.
- **Después del mouse al entrar, antes al salir**: es el orden inverso, y
  `SecEntrar`+`SecMouseSi` y `SecMouseNo`+`SecSalir` siguen juntos para los
  tests de la v2.
- **`Pegar` sin órdenes.** Que un pegado no lance, no firme y no salga lo
  garantiza su firma, no una regla que se pueda olvidar.
- **El foco no salta al Pedido y fuera de un campo se avisa.** La persona
  elige dónde pega y sabe qué pasó con lo que pegó (principio rector).
- **El Pedido conserva sus líneas** (`ctrl-e` ya las permite); `\r` se
  vuelve `\n` porque muchas terminales mandan así los saltos pegados. La
  búsqueda y `otro…` son de una línea: un `\n` invisible rompería el filtro
  o el modelo.
- **Lo que el teclado descarta, se descarta**: un campo nunca tiene algo
  que la persona no pudo escribir, salvo los saltos y tabs del Pedido.
- **El tope es todo o nada.** Cortar sería meter a ciegas un texto distinto
  del copiado. 64 KiB queda bajo el límite de un argumento en Linux
  (128 KiB); si el sistema igual rechaza `-m`, el Registro lo muestra.
- **Sin el modo, no se adivina.** Se descartaron:
  - tomar como pegado una lectura con varias teclas y un enter: falla
    cuando la lectura corta el pegado, y por ssh confunde teclas rápidas;
  - exigir una pausa mínima antes de que el `enter` ejecute: cerraría
    también el portapapeles armado, pero cambia el `enter` de toda la v2 y
    obliga a mover el reloj en decenas de tests. Si Henry la quiere, va en
    su propio spec;
  - preguntarle a la terminal (DECRQM): solo informa, y una terminal sin el
    modo puede no contestar.

## Riesgos y deuda aceptada

- **Sin el modo, el hallazgo sigue abierto en esa terminal**: `A`, tab y
  enter pegados firman. El README lo dice.
- **Una red que corta la marca de inicio** justo después de `ESC` o de
  `ESC [` por más que la espera corta la vuelve teclas, como ya pasa con
  las flechas en la v2.
- **Un portapapeles armado** con `ESC [ 201 ~` adentro corta el pegado y lo
  que sigue son teclas. Algunas terminales filtran esa marca; la TUI no lo
  puede distinguir. Lo cerraría la pausa descartada.
- **Un pegado que nunca termina, SIGKILL y Windows**: ver Casos límite. Lo
  primero solo pasa con una terminal rota; en Windows el CI no maneja la
  TUI (v1).
