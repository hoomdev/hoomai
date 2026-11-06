# Spec: el tablero en la terminal (`hoom tablero`, capa 2)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start tablero-en-la-terminal`.
Depende de: `verbos-de-la-tarjeta.md` (capa 1) integrada. Cada acción de
esta spec ejecuta uno de sus verbos.
Origen: el pedido de Henry del 2026-11-05: "me gusta cómo se ve Neovim, la
interfaz", "que sea fácil de utilizar, que la gente se enamore más del CLI
que del Studio", "ver en qué estado está el item así como lo veo en el
tablero del Studio, tal vez de otra forma pero fácil", "que en vez de
escribir todo eso sea fácil seleccionar y solo escribir el pedido".

## Objetivo

`hoom tablero` abre en la terminal, a pantalla completa, lo mismo que el
tablero del Studio: dónde está cada tarjeta, qué le falta, qué está
corriendo y qué se puede hacer. Se maneja con el teclado, con una tecla por
acción, y para lanzar un rol solo se escribe el pedido.

No copia las ocho columnas del Studio, que en una terminal no entran. Es una
**lista con una tira de progreso por tarjeta**, con arriba lo que espera una
decisión tuya:

```
 hoom · oceanbuild                           check VERDE · 10 tarjetas
 NECESITA TU DECISIÓN
 ▸ nuc-tenancy              ● spec ✋ aprobación ○ pruebas ○ código …
 EN CURSO
   infra-api-convenciones   ● spec ● aprobación ◐ pruebas 2/27 ○ código …
                            test-writer · run 3/6 · 12:41 de 45:00 · narró hace 2s · 3.88 USD
 BACKLOG 7
   infra-ci-cd              falta el spec de la tarjeta
 HECHO 0 ▸
 ──────────────────────────────────────────────────────────────────────────
 NORMAL · tarjeta: infra-api-convenciones (hoom usar)
 j/k moverse · enter abrir · espacio acciones · n siguiente · / buscar · ? ayuda
```

**Regla de oro.** La TUI es una piel, como el Studio. La columna, las
acciones, su motivo cuando no valen, el pedido propuesto y los valores
propuestos salen de la tarjeta (`boardcmd`), y cada acción ejecuta un verbo
de la capa 1 (`hoom pedir`, `hoom siguiente`, `hoom aprobar`,
`hoom guardar`, `hoom integrar`, `hoom reanudar`, `hoom usar`). La TUI no
decide nada que la tarjeta no decida.

**La TUI enseña la CLI.** Cada formulario muestra el verbo corto que va a
ejecutar, así quien la usa aprende los comandos sin leer la ayuda.

## No-goals

- **Reemplazar al Studio.** Siguen los dos, sobre la misma tarjeta.
- **El mouse.** Todo es teclado.
- **Crear o editar items** desde la TUI. Sigue `hoom item add`.
- **Descartar cambios, abrir sesión y ver terminal.** No tienen verbo de la
  capa 1; siguen `hoom task discard` y `hoom cockpit --task`.
- **Verificar desde la TUI.** Por la misma razón que en el Studio: el sobre
  ya verifica al cerrar.
- **Un plugin de Neovim** (`hoom.nvim`). Puede venir después, leyendo el
  mismo `--json`.
- **Temas configurables.** Se usan los 16 colores de la terminal, que ya
  siguen el tema de cada persona.
- **Cambiar `hoom` sin argumentos.** Sigue imprimiendo la ayuda; la ayuda
  nombra `hoom tablero` en su primera línea.

## Contratos

### El verbo

```
hoom tablero
```

- Sin operandos ni flags, con la disciplina de `cliargs` (un flag o un
  operando es exit 2 sin efectos; `-h` sale con 0).
- Sin terminal en la entrada y la salida: exit 2, `hoom tablero necesita una
  terminal; sin terminal usa hoom board`.
- Carga el manifiesto antes de tomar la pantalla: un `hoom.yaml` inválido es
  exit 1 con el error de carga, y la terminal queda como estaba.
- Usa la pantalla alternativa de la terminal, esconde el cursor y la deja
  como estaba al salir: con `q`, con `Ctrl-C`, con un error y con un panic.

### Los datos

- El tablero sale de la misma función que `hoom board --json`. Se vuelve a
  leer cada 2 segundos, como el sondeo del Studio. Si una lectura falla, la
  pantalla se queda con la última y la barra dice `no se pudo leer el
  tablero: <error>`.
- La TUI no tiene tabla de columnas: agrupa en el orden en que el tablero
  trae sus columnas, y su código (sin los tests) no contiene ningún id de
  columna como cadena.
- `Running` gana `timeout_s` (el tiempo límite de su run, del sidecar que
  agrega la capa 1) y `last_event_at` (la hora del último evento de su
  narración). Los emiten `hoom board --json`, `hoom item show --json` y
  `/api/board`. Es lo que la TUI necesita para decir "narró hace 2s" y "de
  45:00".

### La lista

- Secciones, en este orden: `NECESITA TU DECISIÓN` (las tarjetas con
  `needs_decision`), `EN CURSO` (las que tienen `running`) y después una por
  columna, en el orden del tablero, con su cantidad. Cada tarjeta aparece una
  sola vez: en la primera sección que le toca. Una sección vacía no se
  muestra, salvo Hecho, que aparece cerrada con su cantidad y se abre con
  `enter`.
- Cada tarjeta es una línea: el slug, y la **tira**, con un signo por
  segmento de `meter`: `●` hecho, `○` falta, `·` no aplica. Si la tarjeta
  espera tu decisión, el segmento que falta lleva `✋`. Si no entra en el
  ancho, la tira se corta con `…` y en su lugar va el `plain`.
- Una tarjeta con `running` suma una segunda línea: rol, paso
  (`<stage> <step>/<steps>`), tiempo corrido `de` su tiempo límite, `narró
  hace <n>s` y el gasto. Con `interrupted`, la segunda línea dice el motivo
  en rojo.
- `/` abre la búsqueda: filtra por slug o título, sin distinguir mayúsculas
  ni acentos. `Esc` la cierra y vuelve la lista entera.

### La tarjeta abierta

`enter` sobre una tarjeta la abre en cinco pestañas, que se eligen con `1` a
`5` o con `Tab`:

1. **Resumen**: título, columna, `plain`, lo que falta, la tira con el
   detalle de cada segmento, el gasto contra el presupuesto, quién escribió
   y quién revisó, el motivo del rojo y los problemas del doctor.
2. **Spec**: el spec de la tarjeta, de su espacio de trabajo. Cada línea de
   criterio lleva un signo en el margen: `✓` con prueba, `✗` sin prueba
   (está en `untraced`) y `⚑` verificado por comando. `]c` y `[c` saltan al
   siguiente y al anterior criterio sin prueba. `o` abre el spec en el
   editor.
3. **Sobre**: el final del log del sobre en curso, o del último, que se
   actualiza con cada lectura. Arriba, una línea con rol, paso, tiempo y
   `último evento hace <n>s`.
4. **Hallazgos**: los de la tarea, abiertos primero. `enter` abre el editor
   en el archivo y la línea del hallazgo.
5. **Historia**: la línea de tiempo de la tarjeta, la misma de
   `TimelineFor`.

`Esc`, `h` o `←` vuelven a la lista.

**Abrir en el editor.** `$VISUAL`, después `$EDITOR`, después `vi`, por el
shell como en la capa 1. Con una línea: `+<linea> <archivo>` cuando el
editor es `vi`, `vim`, `nvim`, `nano`, `emacs`, `micro` o `hx`, y
`-g <archivo>:<linea>` cuando es `code` o `cursor`. Con otro editor, solo el
archivo. La TUI suelta la pantalla mientras el editor está abierto y la
retoma al volver.

### Las teclas

Una sola tabla de teclas alimenta la ayuda y el despacho: no hay una tecla
que funcione y no esté en la ayuda, ni al revés.

| Tecla | Hace | Verbo que ejecuta |
|---|---|---|
| `j` `k` `↓` `↑` | moverse | |
| `g` `G` | primera, última | |
| `enter` `l` `→` | abrir la tarjeta | |
| `esc` `h` `←` | volver | |
| `espacio` | menú de acciones de la tarjeta | |
| `n` | el siguiente paso | `hoom siguiente <slug>` |
| `a` `t` `w` `r` | pedir al arquitecto, test-writer, writer, reviewer | `hoom pedir <rol> <slug>` |
| `R` | reanudar | `hoom reanudar <slug>` |
| `s` | guardar | `hoom guardar <slug>` |
| `A` | aprobar el spec | `hoom aprobar <slug> --si` |
| `I` | integrar | `hoom integrar <slug> --si` |
| `u` | fijar la tarjeta para la terminal | `hoom usar <slug>` |
| `/` | buscar | |
| `x` | modo experto | |
| `?` | ayuda | |
| `q` `Ctrl-C` | salir | |

- **Las firmas van con mayúscula.** `A` e `I` nunca se tocan por accidente
  con una tecla de moverse.
- Una tecla cuya acción la tarjeta no tiene, o tiene deshabilitada, no
  ejecuta nada: la barra muestra su `why` (o `en <Columna> no hay <accion>`)
  durante 5 segundos.
- **El menú de acciones** (`espacio`) lista las `actions` de la tarjeta en su
  orden, cada una con su tecla y su verbo. Las deshabilitadas van en gris con
  su `why` y no se pueden elegir. Las de `expert` solo aparecen en modo
  experto.
- **Modo experto** (`x`): la barra dice `EXPERTO`, la tarjeta abierta muestra
  `next` y `missing` además de `plain`, y la tira muestra el detalle de cada
  segmento.

### El formulario para lanzar

`n` (cuando lanza un rol), `a`, `t`, `w`, `r` y `R` abren un formulario
flotante sobre la lista:

```
┌ Pedir al writer · infra-api-convenciones ──────────────────────────┐
│ Provider  ‹ claude ›      Modelo  opus       Esfuerzo  high        │
│ Presupuesto  5 USD        Tiempo límite  45m                       │
│                                                                    │
│ Pedido                                                             │
│ Implementa .hoom/specs/infra-api-convenciones.md hasta que hoom ▌  │
│                                                                    │
│ → hoom pedir writer infra-api-convenciones -m "Implementa …"       │
│ enter lanzar · tab campos · ctrl-e editor · esc cancelar           │
└────────────────────────────────────────────────────────────────────┘
```

- Los campos vienen de la acción: las opciones de provider (las que no
  pueden, en gris con su `why`) con la de `default` elegida, `model`,
  `effort`, `budget_usd`, `timeout_s` y `pedido`. El cursor empieza al final
  del pedido. `Ctrl-U` lo vacía.
- `Tab` y `Shift-Tab` pasan de campo. En Provider, `←` y `→` cambian la
  opción. `enter` lanza desde cualquier campo. `Ctrl-E` abre el pedido en el
  editor y lo trae de vuelta al cerrarlo. `Esc` cancela sin lanzar nada.
- La línea `→ ` muestra el verbo corto, con solo los flags cuyo valor
  difiere de lo propuesto. Se actualiza con cada cambio.
- El reviewer no tiene campo Pedido, como en la capa 1.
- Un pedido vacío no lanza: el campo se marca y la barra dice `el pedido
  esta vacio`.

### Lanzar

- La TUI ejecuta el verbo de la tabla como **un proceso propio,
  desprendido** (su propio grupo de procesos), con el pedido en `-m` y los
  flags que el formulario cambió. Salir de la TUI no corta lo que lanzó.
- Espera a que pase una de dos cosas: la tarjeta muestra `running` con un
  sobre nuevo, o el proceso termina. Si termina antes de su primer registro,
  la barra muestra en rojo la última línea de su error (ej. `ya se esta
  lanzando un trabajo en esta tarjeta`).
- Cuando una tarjeta que la TUI vio con `running` pasa a no tenerlo, la
  barra avisa durante 8 segundos: `<rol> termino en <slug>: <plain>`, en
  verde si el sobre cerró `entregable` y en rojo si no.
- Al salir con trabajos lanzados desde esta TUI que siguen corriendo:
  `quedan <n> trabajos corriendo: hoom status`.

### Las firmas y guardar

- `A` e `I` abren una confirmación con el mismo texto que la capa 1
  (aprobar: `Vas a aprobar el spec de "<titulo>" (<n> criterios) con tu
  identidad: <firma>. ...`) y dos botones, `Firmar` y `Cancelar`, con
  `Cancelar` elegido. Solo `enter` sobre `Firmar` ejecuta el verbo con
  `--si`.
- `s` abre una confirmación con las rutas sin guardar y ejecuta
  `hoom guardar <slug>` con `enter`.

### La pantalla

- **Tamaño mínimo** 60 columnas por 15 filas. Más chica, la pantalla dice
  solo `agranda la terminal (minimo 60x15)`. Se redibuja al cambiar el
  tamaño.
- **Colores**: los 16 de la terminal, para que siga su tema. Con `NO_COLOR`
  definida, ninguno.
- **Signos**: los de este contrato con un locale UTF-8. Si `LC_ALL`,
  `LC_CTYPE` y `LANG` no dicen UTF-8, o con `HOOM_ASCII=1`, la tira y los
  márgenes usan ASCII: `#` hecho, `-` falta, `.` no aplica, `!` tu decisión,
  `+` con prueba, `x` sin prueba, `v` por comando.
- **Proyecto vacío**: `no hay tarjetas: hoom item add "<titulo>"`.
- **La barra de abajo** tiene dos líneas: el modo, la tarjeta fijada con
  `hoom usar` (o `sin tarjeta fijada`) y el último mensaje; y las teclas de
  la pantalla actual.

### La dependencia

La TUI necesita poner la terminal en modo crudo y saber su tamaño, en macOS,
Linux y Windows. Para eso suma **una** dependencia: `golang.org/x/term` (que
trae `golang.org/x/sys`). El dibujo, el teclado y el diseño se escriben en
hoom con secuencias ANSI. Pendiente de confirmar al aprobar (ver Decisiones).

### Cómo se prueba

- El modelo de la TUI es puro: recibe teclas y lecturas del tablero y
  devuelve una pantalla y órdenes. Los tests manejan secuencias de teclas
  sobre tableros de ejemplo y comparan la pantalla con archivos dorados en
  `testdata/` (sin color y en ASCII, y una vez con signos UTF-8).
- Lanzar pasa por una interfaz: los tests comprueban los argumentos exactos
  del verbo que se ejecutaría, sin ejecutarlo.
- Ningún test necesita una terminal real.

### Documentación

- `hoom help` nombra `hoom tablero` en su primera línea y en la lista de
  verbos.
- El README suma la sección "El tablero en la terminal" con la pantalla, las
  teclas y la regla de que cada acción es un verbo.

## Casos límite y errores esperados

- `hoom tablero > archivo` o dentro de un pipe: exit 2 con el texto del
  contrato, sin tocar la terminal.
- La tarjeta elegida desaparece en una lectura (se borró su item): la
  selección pasa a la siguiente, y si la tarjeta estaba abierta se vuelve a
  la lista con `la tarjeta <slug> ya no existe`.
- Un rol lanzado desde otra terminal o desde el Studio aparece igual, porque
  `running` sale de la evidencia; el aviso de fin aparece también.
- Dos TUI abiertas lanzan sobre la misma tarjeta: una lanza y la otra
  muestra `ya se esta lanzando un trabajo en esta tarjeta`, del candado de
  archivo de la capa 1.
- La tecla `t` en una tarjeta con el spec sin guardar: la barra dice el
  `why` de la capa 1 y no se abre el formulario.
- El editor falla al abrir el pedido (`Ctrl-E`): el pedido queda como
  estaba y la barra dice `el editor salio con error`.
- Un spec con miles de líneas: la pestaña Spec se desplaza; `]c` busca en
  todo el archivo.
- Un log del sobre de varios MB: la pestaña Sobre lee solo los últimos 64
  KB.
- Títulos con emojis o caracteres de ancho doble: pueden desalinear la
  línea (ver Riesgos).
- Terminal sin pantalla alternativa: la TUI dibuja igual y al salir limpia la
  pantalla.

## Criterios de aceptación

- CA-516: `hoom tablero` sin terminal sale con 2 y el texto del contrato sin tocar la terminal; con un flag o un operando sale con 2 sin efectos; con `-h` sale con 0; con un `hoom.yaml` inválido sale con 1 y el error de carga antes de tomar la pantalla.
- CA-517: el paquete de la TUI agrupa las secciones de columna en el orden del tablero, su código sin los tests no contiene `"backlog"`, `"tu-aprobacion"`, `"tu-aceptacion"` ni `"hecho"`, y las acciones, sus `why`, los pedidos y los valores que muestra son los de la tarjeta.
- CA-518: la lista muestra las secciones del contrato en su orden, cada tarjeta una sola vez, sin secciones vacías salvo Hecho cerrada con su cantidad, y la tira con un signo por segmento de `meter` y `✋` en lo que espera tu decisión (archivos dorados).
- CA-519: `Running` trae `timeout_s` y `last_event_at` en `hoom board --json`, `hoom item show --json` y `/api/board`, y la segunda línea de una tarjeta en curso muestra rol, paso, tiempo corrido de su tiempo límite, `narró hace <n>s` y gasto.
- CA-520: `j`, `k`, las flechas, `g` y `G` mueven la selección; `enter`, `l` y `→` abren la tarjeta; `esc`, `h` y `←` vuelven; `/` filtra por slug o título sin distinguir mayúsculas ni acentos y `Esc` lo cierra.
- CA-521: la tarjeta abierta tiene las cinco pestañas del contrato, que se eligen con `1` a `5` y con `Tab`, y Resumen muestra `plain`, lo que falta, el gasto, quién escribió y revisó, el motivo del rojo y los problemas del doctor.
- CA-522: la pestaña Spec marca cada criterio con `✓`, `✗` o `⚑` según `untraced` y los criterios verificados por comando, `]c` y `[c` saltan entre los criterios sin prueba, y `o` abre el spec en el editor.
- CA-523: la pestaña Sobre muestra los últimos 64 KB del log del sobre en curso o del último, se actualiza con cada lectura y dice `último evento hace <n>s`.
- CA-524: la pestaña Hallazgos lista los de la tarea con los abiertos primero, y `enter` abre el editor con `+<linea> <archivo>` o `-g <archivo>:<linea>` según el editor, como dice el contrato.
- CA-525: cada tecla de acción de la tabla ejecuta su verbo cuando la tarjeta tiene la acción habilitada; si no la tiene o está deshabilitada, no ejecuta nada y la barra muestra el `why` o el texto del contrato.
- CA-526: `espacio` lista las `actions` de la tarjeta en su orden con su tecla y su verbo, las deshabilitadas en gris con su `why` y sin poder elegirse, y las de `expert` solo en modo experto.
- CA-527: el formulario trae las opciones de provider con la de `default` elegida y las que no pueden con su `why`, y `model`, `effort`, `budget_usd`, `timeout_s` y `pedido` de la acción; `Tab`, `Shift-Tab`, `←`, `→`, `Ctrl-U`, `Ctrl-E` y `Esc` hacen lo del contrato; la línea `→ ` muestra el verbo corto con solo los flags cambiados; un pedido vacío no lanza; el reviewer no tiene Pedido.
- CA-528: lanzar ejecuta el verbo con los argumentos exactos (`-m` con el pedido y solo los flags cambiados) como proceso desprendido; un proceso que termina antes de su primer registro muestra en rojo la última línea de su error; salir de la TUI no corta lo lanzado y dice cuántos trabajos quedan corriendo.
- CA-529: `A` e `I` muestran el texto de la firma con `Cancelar` elegido y ejecutan `hoom aprobar` o `hoom integrar` con `--si` solo con `enter` sobre `Firmar`; `s` muestra las rutas sin guardar y ejecuta `hoom guardar` con `enter`.
- CA-530: el tablero se vuelve a leer cada 2 segundos; una lectura que falla deja la última pantalla con `no se pudo leer el tablero: <error>`; una tarjeta que deja de tener `running` produce el aviso de 8 segundos en verde o rojo según cerró su sobre.
- CA-531: `u` ejecuta `hoom usar <slug>` y la barra muestra la tarjeta fijada; `x` alterna el modo experto, que muestra `next`, `missing` y el detalle de la tira.
- CA-532: la ayuda (`?`) lista las teclas de la pantalla actual desde la misma tabla que usa el despacho: toda tecla con efecto está en la ayuda y toda tecla de la ayuda tiene efecto.
- CA-533: la TUI usa la pantalla alternativa y deja la terminal como estaba al salir con `q`, con `Ctrl-C`, con un error y con un panic; redibuja al cambiar el tamaño; por debajo de 60x15 muestra solo `agranda la terminal (minimo 60x15)`.
- CA-534: con `NO_COLOR` la pantalla no tiene secuencias de color; sin un locale UTF-8 o con `HOOM_ASCII=1` usa los signos ASCII del contrato (archivos dorados de los dos modos).
- CA-535: un proyecto sin items muestra `no hay tarjetas: hoom item add "<titulo>"`, y una tarjeta que desaparece en una lectura mueve la selección o vuelve a la lista con el mensaje del contrato.
- CA-536: los `require` de `go.mod` son exactamente `gopkg.in/yaml.v3`, `golang.org/x/term` y `golang.org/x/sys`.
- CA-537: `hoom help` nombra `hoom tablero` y el README tiene la sección "El tablero en la terminal". [verifica: go run ./cmd/hoom help | grep -q "hoom tablero" && grep -q "El tablero en la terminal" README.md]

## Decisiones

- **Una lista con tira, no ocho columnas.** En 80 columnas de terminal, ocho
  columnas dejan 10 caracteres por tarjeta. La tira dice lo mismo que el
  medidor del Studio en una línea, y las secciones de arriba ponen primero
  lo que espera a la persona.
- **Cada acción es un verbo de la capa 1, ejecutado como proceso propio.**
  La TUI no importa la función de lanzar: ejecuta el comando que mostró en
  la línea `→ `. Así lo que enseña es exactamente lo que corre, y salir de la
  TUI no corta nada. La alternativa (llamar a la función en el mismo
  proceso, como el Studio) obligaba a cortar los trabajos al salir o a
  esperar.
- **Las firmas con mayúscula y con `Cancelar` elegido.** Es la única
  defensa contra una tecla de más en una interfaz de una tecla por acción.
- **`golang.org/x/term` y nada más, en lugar de Bubble Tea.** hoom tiene una
  sola dependencia (`yaml.v3`), y su postura es la de un árbitro: cada
  módulo de más es superficie de cadena de suministro que un usuario tiene
  que creerle. `x/term` es del proyecto Go y resuelve la parte que depende
  del sistema operativo (modo crudo y tamaño, también en Windows). Bubble
  Tea con Lip Gloss daría componentes y un teclado más pulido con menos
  código propio, a cambio de una decena de módulos. Pendiente de confirmar
  al aprobar: si Henry elige Bubble Tea, cambian esta sección, CA-536 y el
  riesgo del teclado; el resto del contrato no.
- **Los 16 colores de la terminal, sin temas propios.** Así la TUI se ve bien
  con tokyonight, catppuccin o el tema de cada persona, sin configurar nada.
- **Teclas de Vim, con flechas también.** `j`/`k`, `g`/`G`, `h`/`l` para
  quien viene de Vim; flechas, `enter` y `esc` para todos. El público de
  hoom es todo dev, no solo quien usa Vim.
- **`s` guarda, no `g`.** `g`/`G` son ir arriba y abajo en Vim, y `s` es
  guardar en casi cualquier programa.
- **Cada 2 segundos, como el Studio.** Menos carga en proyectos grandes que
  vigilar el disco, y ya probado en la cabina.

## Riesgos y deuda aceptada

- **Un teclado escrito a mano.** Leer secuencias de escape (flechas,
  `Shift-Tab`, `Ctrl-` y teclas) en todas las terminales tiene casos raros.
  Se cubre lo de la tabla de teclas; una terminal que mande otra cosa puede
  necesitar arreglos.
- **El ancho de los caracteres.** Sin una tabla Unicode de anchos, un título
  con emojis o caracteres de ancho doble puede desalinear su línea.
- **Windows se compila, pero el CI no maneja la TUI ahí.** Los tests del
  modelo corren en todos lados; el modo crudo en Windows se prueba a mano.
- **Leer el tablero cada 2 segundos** en un proyecto con cientos de tarjetas
  puede pesar. Es lo mismo que hace el Studio hoy.
- **Dos pieles que mantener** (Studio y TUI). Lo acota la regla de oro:
  ninguna decide, las dos leen la misma tarjeta.
