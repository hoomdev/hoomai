# Spec: el tablero con paneles (`hoom tablero` v2)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start tablero-con-paneles`.
Depende de: `tablero-en-la-terminal.md` (la v1), integrada con los PR 64 y
65.
Origen: Henry probó la v1 el 2026-11-09: "me pierdo, no tiene paneles o
sidebar, los modelos no se seleccionan hay que escribirlo, hay personas que
ni saben que modelos existen, no tiene ni colores". Pidió el estilo de
lazygit sin copiarlo: paneles que muestran la información de lo que vas
eligiendo y el formulario encima de todo, como un modal. Su criterio: "que
lo sepa utilizar un junior, parecido a la filosofía de Apple, que la mayoría
lo sepa utilizar y en todo momento sepa que realmente esta haciendo".

Re-expresa, de `tablero-en-la-terminal.md`: CA-518 (la lista), CA-519 (solo
la segunda línea de una tarjeta en curso; los campos de `Running` siguen
igual), CA-520 (moverse y abrir), CA-521 (las pestañas), CA-522 (los
criterios del spec), CA-523 (el sobre), CA-524 (los hallazgos), CA-526 (el
menú de acciones), CA-527 (el formulario), CA-529 (las firmas), CA-531 (la
tarjeta fijada y el modo experto), CA-532 (la ayuda) y CA-535 (la tarjeta
que desaparece). Los otros nueve criterios de la v1 siguen igual: el verbo,
la regla de oro, las teclas de acción, lanzar, la relectura, la terminal,
el color, la dependencia y la documentación.

## Objetivo

`hoom tablero` deja de ser una lista y pasa a ser **paneles**. A la
izquierda hay cuatro paneles numerados: [1] Proyecto, [2] Tarjetas,
[3] Corriendo y [4] Hallazgos. A la derecha, el panel principal [0] muestra
el contexto vivo de lo elegido, y debajo el **Registro** lista cada comando
que la TUI ejecutó. Los formularios y las firmas salen en un **modal**
encima de todo, con selectores en lugar de texto libre y con el comando
exacto que se va a ejecutar.

A 100x25:

```
┌[1] Proyecto───────────────────┐┌[0] nuc-tenancy · Aislar los datos por tenant────────────────────┐
│ oceanbuild · rama main        ││ ¿Qué falta?  el spec espera tu aprobación                       │
│ check VERDE · verify hace 2h  ││ Siguiente paso  Aprobar spec  [A]                               │
│ hoy 3.88 USD en 4 trabajos    ││                                                                 │
│ fijada: nuc-tenancy           ││ Tu aprobación · gasto 0.42 de 5.00 USD · escribió claude        │
└───────────────────────────────┘│                                                                 │
╔[2] Tarjetas═══════════════════╗│ Spec · 14 criterios · 14 sin prueba                             │
║ NECESITA TU DECISIÓN          ║│  ✗ cada tenant ve solo sus filas                                │
║ ▸ nuc-tenancy           ●✋○○○║│  ✗ un tenant sin plan responde 402                              │
║ EN CURSO                      ║│  …                                                              │
║   infra-api-convenciones ●●○○○║│ Hallazgos · 1 abierto                                           │
║ BACKLOG 7                     ║│  low  tenant.go:7  el nombre del tenant no se recorta           │
║   infra-ci-cd            ○○○○·║│ Historia                                                        │
║   infra-observabilidad   ○○○○·║│  11-08 14:02  claude escribió el spec                           │
║ HECHO 3 ▸                     ║│                                                                 │
╚═══════════════════════1 de 10═╝│                                                                 │
┌[3] Corriendo 1────────────────┐│                                                                 │
│   infra-api-c… writer 12:41   ││                                                                 │
└───────────────────────────────┘└─────────────────────────────────────────────────────────────────┘
┌[4] Hallazgos 2────────────────┐┌Registro─────────────────────────────────────────────────────────┐
│   high infra-api… router.go:41││ 14:02 Pedir al arquitecto · nuc-tenancy · hecho                 │
│   low  nuc-tenan… tenant.go:7 ││       hoom pedir arquitecto nuc-tenancy -m "Escribe el spec…"   │
└───────────────────────────────┘└─────────────────────────────────────────────────────────────────┘

 ↑↓ elegir · enter ver · espacio acciones · n siguiente paso · A aprobar spec · ? todas las teclas
```

**Principio rector.** Un junior sabe en todo momento:

- **dónde está**: el panel con foco tiene borde doble y su número;
- **qué puede hacer**: la última línea muestra, en palabras, solo las
  teclas del panel con foco;
- **qué se va a ejecutar**: nada que lance un trabajo, firme o commitee
  corre sin un modal que diga `Se va a ejecutar: <comando>`. Después, el
  Registro dice qué corrió y cómo terminó.

**Lo que no cambia de la v1.**

- La regla de oro: la TUI es una piel y la tarjeta decide (columna,
  acciones, `why` y valores propuestos).
- Cada acción ejecuta un verbo de la capa 1 como proceso propio y
  desprendido.
- Las teclas de acción: `n`, `a`, `t`, `w`, `r`, `R`, `s`, `A`, `I` y `u`.
- La relectura del tablero cada 2 segundos.
- La terminal: pantalla alternativa, restaurada al salir.
- `NO_COLOR` y `HOOM_ASCII`.
- Una sola dependencia, `golang.org/x/term`.

## No-goals

- **Reemplazar al Studio.** Su diálogo de rol sigue con Modelo en texto
  libre. Los selectores en el Studio van en su propio spec.
- **Ejecutar con el mouse.** El mouse elige y desplaza; lanzar y firmar
  siguen pidiendo una tecla.
- **Pasar modelo o esfuerzo a gemini y opencode.** hoom hoy no se los pasa:
  sus `Capabilities` no tienen `Model` ni `Effort`. Cambiarlo toca la capa
  de providers y va en su propio spec. En la TUI, sus selectores salen en
  gris con el motivo.
- **Leer la lista de modelos de cada CLI**, por ejemplo
  `~/.codex/models_cache.json`. El catálogo es de hoom (ver Decisiones).
- **Validar modelos.** El catálogo propone, no prohíbe. `otro…` manda lo
  que escribas y, si el provider lo rechaza, el Registro muestra su error.
  Como hoy, hoom no valida el vocabulario del provider.
- **Resolver hallazgos desde la TUI.** Sigue `hoom finding resolve`, que
  pide evidencia. El panel [0] de un hallazgo muestra el comando.
- **Los runs sin tarjeta en [3]** (`hoom run` a mano, `hoom agent`). Siguen
  en `hoom status`.
- **Guardar el Registro.** Vive mientras la TUI está abierta.
- **Copiar al portapapeles, paneles configurables y temas.** Se usan los 16
  colores de la terminal, como en la v1.
- Lo que la v1 dejó fuera sigue fuera: crear o editar items, descartar,
  abrir sesión, ver la terminal y verificar.

## Contratos

### La pantalla

- **Dos columnas** con 80 columnas o más. La izquierda tiene `Ancho/3`
  celdas (división entera), con [1], [2], [3] y [4] apilados. La derecha
  tiene [0] arriba y el Registro abajo.
- **Una columna** entre 60 y 79 columnas. Arriba van los paneles de la
  izquierda, después [0] y al final el Registro, plegado.
- Debajo de 60x15 la pantalla dice solo `agranda la terminal (minimo
  60x15)`, como en la v1.
- **Alturas.** [1] ocupa 6 líneas. [3] ocupa sus filas más los bordes,
  entre 3 y 7. [4] ocupa un tercio de lo que queda, con mínimo 3, y [2] el
  resto. El Registro ocupa 6 líneas con 24 filas o más.
- **Pliegue.** Si los paneles no entran con su mínimo, los que no tienen el
  foco se pliegan a una línea: el borde de arriba con su título y su
  cantidad (`─[3] Corriendo 1────`). Se pliegan en este orden: [1], [4],
  [3] y el Registro. El panel con foco nunca se pliega.
- En una columna, el panel de la izquierda con foco ocupa hasta un tercio
  del cuerpo y los otros tres van plegados. Si el foco está en [0], los
  cuatro van plegados. El Registro va plegado y muestra la primera línea de
  su última entrada.
- **Bordes.**
  - El panel con foco: línea doble (`═ ║ ╔ ╗ ╚ ╝`) y, con color, verde y en
    negrita.
  - Los otros: línea simple (`─ │ ┌ ┐ └ ┘`).
  - El título va en el borde de arriba: `[n] Nombre` y, si tiene, su
    cantidad.
  - El título de [0] dice qué muestra: `<slug> · <titulo>`, `Proyecto`,
    `<slug> · <rol>` o `hallazgo <id>`.
- **Filas que no entran.** El panel se desplaza lo mínimo para que la fila
  elegida se vea, y su borde de abajo dice `<n> de <m>`: la posición de lo
  elegido y el total.
- **La selección.** La fila elegida de cada panel de la izquierda empieza
  con `▸`. En el panel con foco, con color, además es una barra de fila
  entera en video inverso. En los paneles sin foco queda solo el `▸`.
- **Las dos últimas líneas.**
  - La penúltima es la línea de mensajes: el `why`, los errores y los
    avisos, con sus tiempos y colores de la v1. En modo experto empieza
    con `EXPERTO · `. Mientras la búsqueda está abierta, muestra
    `/<texto>▌`.
  - La última muestra las teclas del panel con foco o del modal abierto.
- **Colores**, de los 16 de la terminal:
  - foco: verde y negrita;
  - tu decisión: amarillo;
  - en curso: cian;
  - hecho y los mensajes buenos: verde;
  - rojo, interrumpido, falla y los mensajes de error: rojo;
  - lo secundario y lo deshabilitado: gris;
  - la severidad de un hallazgo: high en rojo, medium en amarillo y low en
    gris.
- **Anchos.** Los caracteres de ancho doble ocupan dos celdas: East Asian
  Wide, Fullwidth y los emojis que se presentan como emoji. Las marcas
  combinantes ocupan cero. Lo decide una tabla propia de hoom. Esto
  reemplaza la regla de la v1 de "cada runa, una celda".
- **Acentos**, como en la v1:
  - los títulos y las etiquetas de la TUI llevan acentos con UTF-8 y los
    pierden en ASCII (`¿Qué falta?` es `Que falta?`);
  - los mensajes y las líneas del catálogo van sin acentos en los dos
    modos;
  - los textos de la tarjeta van como vienen.
- Todo texto de la pantalla pasa por la neutralización de controles de la
  v1.
- **Tiempos relativos** (`hace <tiempo>`): `<n>s` por debajo del minuto, y
  si no, la unidad más grande entera entre `<n>m`, `<n>h` y `<n>d`. Los
  montos van con dos decimales y ` USD`, como el gasto de la v1.

### Los paneles

**[1] Proyecto.** Cuatro líneas:

1. `<proyecto> · rama <rama>`. La rama sale de `gitx` (la misma que guarda
   el veredicto); sin repositorio, `sin git`.
2. `check VERDE · verify hace <tiempo>`, o `check ROJO: <motivo>` con el
   texto de `hoom check`, o `sin veredicto todavia`.
3. `hoy <usd> USD en <n> trabajos`, con ` · <k> sin dato` si falta el costo
   de alguno, o `hoy sin trabajos` (ver `hoom costo`).
4. `fijada: <slug>` (`hoom usar`) o `sin tarjeta fijada`.

Su [0] (`Proyecto`) muestra:

- el último veredicto: id, hace cuánto, `pass`, `fail`, `error` y `absent`,
  y su ruta en `.hoom/verdicts/`;
- la línea del check;
- los problemas de `hoom board doctor`, cada uno con su acción.

**[2] Tarjetas.** Las secciones y el orden de la v1:

- `NECESITA TU DECISIÓN`, `EN CURSO` y una sección por columna, en el orden
  del tablero.
- Cada tarjeta aparece una sola vez.
- Las secciones vacías no se muestran, salvo Hecho, que aparece cerrada con
  su cantidad y se abre o cierra con `enter`.
- Los títulos de sección van en el color de su estado.

Cada fila tiene la marca, el slug y la **tira** de la v1 alineada al borde
derecho:

- El slug se corta con `…` para dejar la tira y un espacio.
- Si al slug le quedan menos de 8 celdas, la fila va sin tira.
- Una tarjeta con `red` o con `interrupted` lleva el slug en rojo.
- El `plain` ya no va en la fila: está en [0].

Sin tarjetas: `no hay tarjetas: hoom item add "<titulo>"`. Antes de la
primera lectura: `leyendo el tablero...`.

**[3] Corriendo.**

- Una fila por tarjeta con `running`, en el orden del tablero: slug, rol y
  tiempo corrido, con ` de <limite>` si entra (formato de la v1).
- Las tarjetas con `interrupted` también aparecen, en rojo y con
  `interrumpido`, porque esperan `R`.
- Vacío: `nada corriendo`.

Su [0] (`<slug> · <rol>`):

- Primero la línea `<rol> · <paso> · <tiempo> · narró hace <n>s · <gasto>`.
  Es la segunda línea de la v1; en una interrumpida va el texto rojo de la
  v1.
- Después, la cola del log del sobre: los últimos 64 KB, con las líneas de
  la pestaña Sobre de la v1.
- La vista sigue el final mientras llegan eventos, salvo que la persona
  haya subido. `G` la vuelve al final.

**[4] Hallazgos.**

- Los hallazgos abiertos de todo el proyecto, de la misma función que
  `hoom finding list --open`. Van primero los high, después los medium y
  después los low; dentro de cada grupo, en el orden de la lista.
- Cada fila: severidad en su color, tarea (la tarjeta) y
  `<archivo>:<linea>`, con el nombre del archivo sin carpetas.
- El título lleva la cantidad y, con color, ` · <k> high` en rojo si hay.
- Vacío: `sin hallazgos abiertos`.

Su [0] (`hallazgo <id>`):

- `<severidad> · lente <lente> · tarea <tarea>`;
- la descripción entera;
- si nombra un archivo, `<archivo>:<linea>` y las líneas del archivo
  alrededor de la suya (las que entren), con la del hallazgo marcada con
  `▸`;
- al final, `para cerrarlo: hoom finding resolve <id> --as
  corregido|refutado --evidence "<por que>"`.

Las rutas siguen las reglas de la v1: relativas a la raíz del proyecto, y
una ruta que sale del proyecto no se lee ni se abre.

**[0] de una tarjeta** es un solo documento, en este orden:

1. `¿Qué falta?` y el `plain`. En modo experto, además, `siguiente:
   <next>` y una línea `missing:` por cada falta (v1).
2. `Siguiente paso`, con la etiqueta y la tecla de la acción que abre `n`
   (`Aprobar spec  [A]`). Si no hay ninguna, va el texto que diría
   `hoom siguiente` y no va tecla.
3. La columna, el gasto contra el presupuesto, quién escribió y quién
   revisó, `rojo: <plain>` y los problemas del doctor de la tarjeta, como
   el Resumen de la v1.
4. Una línea por segmento de la tira: `<signo> <etiqueta>: <detalle>`.
5. `Spec · <n> criterios · <k> sin prueba` y una línea por criterio:
   `<signo> <texto>`, con `✓` con prueba, `✗` sin prueba y `⚑` por
   comando. En modo experto, el id va antes del texto. Sin spec:
   `sin spec: <ruta>`.
6. `Hallazgos` y los de su tarea, una línea cada uno: los abiertos primero
   y los cerrados en gris.
7. `Historia` y la línea de tiempo de la tarjeta, en el orden de la v1.
8. `Narración` y la cola del sobre en curso o del último (64 KB), o
   `sin sobres todavia`.

### El Registro

- Una entrada por cada verbo que la TUI ejecuta. Se ven las últimas que
  entran, la más nueva abajo. Cada entrada ocupa dos líneas:
  - `<HH:MM> <etiqueta> · <slug> · <estado>`;
  - seis espacios y el comando, con el mismo texto que mostró
    `Se va a ejecutar`.
- La etiqueta es la de la acción en la tarjeta (`Pedir al writer`,
  `Aprobar spec`, `Guardar`). `u` se registra como `Fijar para la
  terminal`.
- Los estados posibles:
  - `lanzando…`: mientras espera, como en la v1;
  - `corriendo`: la tarjeta muestra `running` con un sobre nuevo;
  - `hecho`: el proceso terminó con 0;
  - `fallo: <ultima linea de su error>`, en rojo.
- El fin de un trabajo (el aviso de la v1) suma su propia entrada:
  `<HH:MM> <rol> termino en <slug>: <plain>`, en verde o en rojo con la
  regla de la v1. También va 8 segundos a la línea de mensajes.
- Abrir el editor suma la entrada `<HH:MM> Abrir en el editor ·
  <archivo>[:<linea>]`, con el comando del editor en la segunda línea.
- Plegado, el Registro muestra la primera línea de su última entrada.

### Las teclas

Una sola tabla alimenta la línea de teclas, la ayuda y el despacho. "Con
tarjeta" quiere decir: en [2] sobre una tarjeta, en [3] (la tarjeta de la
fila) o en [0] mostrando una tarjeta.

| Tecla | Dónde | Hace | Verbo que ejecuta |
|---|---|---|---|
| `1` `2` `3` `4` `0` | siempre | foco a ese panel | |
| `tab` `shift-tab` | siempre | panel siguiente o anterior, en el orden 1, 2, 3, 4, 0 | |
| `↑` `↓` `k` `j` | un panel | elegir (en [0], desplazar) | |
| `g` `G` | un panel | primera o última | |
| `pgup` `pgdn` | [0] | desplazar una página | |
| `enter` `→` `l` | [1] a [4] | ver en grande: foco a [0]; en Hecho, abrir o cerrar | |
| `esc` `←` `h` | [0] | volver al panel de donde vino | |
| `espacio` | con tarjeta | acciones de la tarjeta | |
| `n` | con tarjeta | el siguiente paso | el de su acción |
| `a` `t` `w` `r` | con tarjeta | pedir al arquitecto, test-writer, writer o reviewer | `hoom pedir <rol> <slug>` |
| `R` | con tarjeta | reanudar | `hoom reanudar <slug>` |
| `s` | con tarjeta | guardar | `hoom guardar <slug>` |
| `A` | con tarjeta | aprobar el spec | `hoom aprobar <slug> --si` |
| `I` | con tarjeta | integrar | `hoom integrar <slug> --si` |
| `u` | con tarjeta | fijar la tarjeta para la terminal | `hoom usar <slug>` |
| `e` | [0] de una tarjeta, [4] y su [0] | abrir en el editor | |
| `]c` `[c` | [0] de una tarjeta | criterio sin prueba siguiente o anterior | |
| `/` | siempre | buscar tarjetas | |
| `x` | siempre | modo experto | |
| `?` | siempre | todas las teclas | |
| `q` `ctrl-c` | siempre | salir | |

Con un modal abierto valen solo las teclas del modal, que están en la misma
tabla:

| Tecla | Dónde | Hace |
|---|---|---|
| `tab` `shift-tab` | modal | campo o botón siguiente o anterior |
| `←` `→` | selector | opción anterior o siguiente, saltando las grises |
| `←` `→` | botones | botón anterior o siguiente |
| `←` `→` `backspace` `ctrl-u` | campo de texto | mover el cursor, borrar, vaciar |
| `enter` `espacio` | selector | abrir la lista de opciones |
| `↑` `↓` `enter` | lista abierta | moverse, elegir |
| `enter` | Pedido | ir al botón de la acción |
| `enter` | botón | ejecutar la acción o cancelar |
| `ctrl-e` | Pedido | abrir el pedido en el editor |
| `↑` `↓` `k` `j` | Acciones | moverse por las acciones que se pueden elegir |
| `enter`, la tecla de la acción | Acciones | abrir el modal de esa acción, o ejecutar `u` |
| `↑` `↓` `k` `j` `pgup` `pgdn` | Ayuda | desplazar |
| `?` | Ayuda | cerrar |
| `esc` | modal | cerrar sin ejecutar (con una lista abierta, cerrar la lista) |

- `/` pone el foco en [2] y filtra mientras se escribe, con las reglas de la
  v1: por slug o título, sin mayúsculas ni acentos. `enter` y `esc` cierran
  la búsqueda y sueltan el filtro, y la tarjeta elegida sigue elegida.
- `e` en [0] de una tarjeta abre el spec en la línea del primer criterio a
  la vista (o en la línea 1). En [4] o en su [0], abre el archivo del
  hallazgo en su línea. El editor y sus flags de línea son los de la v1,
  que suelta la pantalla mientras está abierto.
- `]c` y `[c` llevan la vista al siguiente o al anterior criterio con `✗`.
  Si no hay, la línea de mensajes dice `no hay mas criterios sin prueba`.
- Una tecla de acción que la tarjeta no tiene, o tiene deshabilitada, hace
  lo de la v1: no ejecuta nada y la línea de mensajes muestra su `why`.

### Nada a ciegas

- Toda tecla que ejecuta un verbo abre primero su modal, salvo `u`.
  `hoom usar` corre enseguida porque solo fija la tarjeta para la
  terminal: no lanza, no firma y no commitea. Igual va al Registro.
- `n` abre el modal de su acción, sea cual sea su tipo: pedir, firma o
  guardar. Sin siguiente paso, la línea de mensajes dice lo que diría
  `hoom siguiente`. En la v1, `n` solo abría el formulario de un rol.

### Los modales

**Lo común a todos.**

- Un cuadro centrado encima de todo, con el borde del foco. Con color, lo
  de atrás se dibuja en gris mientras está abierto.
- Las teclas van solo al modal. La última línea muestra las del modal.
- `esc` cierra sin ejecutar nada. Con una lista de opciones abierta, cierra
  solo la lista.
- Los modales que ejecutan un verbo (pedir, firma y guardar) llevan abajo
  dos botones: el de la acción y `Cancelar`. El elegido va entre corchetes
  (`[ Pedir al writer ]`). `←`, `→` y `tab` pasan de botón, y `enter` sobre
  un botón lo ejecuta o cancela. Acciones y Ayuda no tienen botones.
- Si el modal no entra en la pantalla, sigue la regla de la v1 para las
  cajas, con dos cosas que se van primero: la línea de ayuda del selector y
  las palabras de origen. `Se va a ejecutar` y los botones nunca se van.
  Una firma que no entra entera nunca se firma (v1).

**Pedir** (`a`, `t`, `w`, `r`, `R`, y `n` cuando el siguiente paso lanza un
rol):

```
╔ Pedir al writer · infra-api-convenciones ════════════════════════════╗
║   Pedido                                                             ║
║   ┌────────────────────────────────────────────────────────────────┐ ║
║   │ Implementa .hoom/specs/infra-api-convenciones.md hasta que     │ ║
║   │ hoom verify quede VERDE                                        │ ║
║   └────────────────────────────────────────────────────────────────┘ ║
║   Provider       ‹ claude ›                de la tarjeta             ║
║   Modelo         ‹ sonnet ›                de hoom.yaml              ║
║ ▸ Esfuerzo       ‹ high ›                  elegido                   ║
║   Tope           ‹ 5 USD ›                 de la tarjeta             ║
║   Tiempo límite  ‹ 45m ›                   de hoom.yaml              ║
║                  high: más profundidad para problemas complejos      ║
║                                                                      ║
║   Se va a ejecutar:                                                  ║
║   hoom pedir writer infra-api-convenciones -m "Implementa .hoom/s…"  ║
║     --effort high                                                    ║
║                                                                      ║
║                    [ Pedir al writer ]     Cancelar                  ║
╚══════════════════════════════════════════════════════════════════════╝
 tab campo · ←→ cambiar · enter ver opciones · ctrl-e editor · esc cerrar
```

- **Los campos**, en orden: Pedido, Provider, Modelo, Esfuerzo, Tope,
  Tiempo límite y los botones. `tab` y `shift-tab` pasan de uno a otro.
- **El Pedido** es un cuadro de 4 líneas que se edita como en la v1
  (`←`, `→`, borrar, `ctrl-u` lo vacía, `ctrl-e` lo abre en el editor).
  `enter` en el Pedido no lanza: lleva el foco al botón de la acción.
- **Un selector** muestra `‹ <opcion> ›` y, en gris, de dónde salió el
  valor: `de hoom.yaml`, `de la tarjeta` o `del provider` (de `origen`),
  o `elegido` si la persona lo cambió.
  - `←` y `→` cambian la opción ahí mismo y saltan las grises.
  - `enter` o `espacio` abren la lista de opciones encima del modal, una
    línea por opción con su nombre y su línea. Las grises muestran su
    `why` y no se eligen. `↑` y `↓` se mueven, `enter` elige y `esc` cierra
    sin cambiar.
  - Debajo de los selectores, una línea gris dice la línea de la opción
    elegida en el selector con foco, o su `why`.
- **`otro…`** convierte el campo en texto. `enter` lo confirma; `esc`, o un
  texto vacío, vuelve a la opción de antes. La TUI no valida lo escrito.
- **`Se va a ejecutar:`** lleva el comando con las reglas de la línea `→`
  de la v1: solo los flags cuyo valor difiere de lo propuesto, citados como
  en la capa 1. Ocupa hasta dos líneas. Si no entra, el argumento de `-m`
  se acorta a su comienzo y `…`, porque el pedido entero está en su cuadro.
- **Los botones** son `[ <etiqueta de la acción> ]`, elegido al abrir, y
  `Cancelar`.
- Con el pedido vacío, `enter` sobre el botón no lanza: el Pedido se marca
  y la línea de mensajes dice `el pedido esta vacio` (v1).
- Siguen las reglas de la v1 por verbo:
  - el reviewer no tiene Pedido, y su Tiempo límite va en gris con
    `de review.timeout`;
  - en reanudar, el Provider es fijo y su línea dice `la sesion es de
    <provider>`.

**Las opciones de cada selector.**

- **Provider**: los `providers` de la acción, en su orden. Los que no
  pueden van en gris con su `why`. Al abrir está elegido el `default`.
- **Modelo**, en este orden:
  1. `por defecto del provider`, con la línea `el que elige <provider>: su
     configuracion o tu cuenta`;
  2. los modelos del catálogo del provider elegido;
  3. los `models.<provider>` de `hoom.yaml`, con su descripción. Uno con el
     nombre de un modelo del catálogo le cambia la línea en vez de
     repetirlo;
  4. `otro…`.

  Al abrir está elegido el `model` de la acción. Si no está en ninguna
  lista, se suma antes de `otro…` con la línea `de hoom.yaml`.
- **Esfuerzo**: `por defecto del provider`, los esfuerzos del catálogo del
  provider y `otro…`.
- **Un provider que no recibe modelo** (sin `Model` en sus `Capabilities`,
  hoy gemini y opencode): Modelo muestra `‹ del provider ›` en gris con
  `hoom no le pasa el modelo a <provider>` y no abre lista. Esfuerzo hace
  lo mismo con `el esfuerzo`.
- **Lo que fija `hoom.yaml`.** La capa 1 aplica `agents.<rol>.model` a
  cualquier provider (`valores.pick`) y no tiene flag para volver al del
  provider. El modal lo muestra tal cual:
  - si el `origen.model` de la acción es `hoom.yaml`,
    `por defecto del provider` va en gris con `hoom.yaml fija <modelo> para
    el <rol>`;
  - al cambiar de provider, ese modelo sigue elegido. Si no está en la
    lista del provider nuevo, la línea gris dice `<modelo> no esta en la
    lista de <provider>: puede rechazarlo`;
  - sin valor de `hoom.yaml`, cambiar de provider vuelve Modelo y Esfuerzo
    a `por defecto del provider`, y volver al provider por defecto
    devuelve lo propuesto;
  - el esfuerzo sigue las mismas tres reglas.
- **Tope**: si el provider elegido no acepta tope (`budget` en falso), va
  `‹ sin tope ›` en gris con `<provider> no acepta tope`. Si lo acepta, las
  opciones son:
  - primero lo propuesto: el `budget_usd` de la acción, o `sin tope` si no
    trae;
  - después 1, 2, 5, 10 y 20 USD y `otro…`, sin repetir lo propuesto;
  - `sin tope` aparece solo cuando es lo propuesto: la capa 1 rechaza
    `--budget-usd 0` y no tiene otra forma de pedirlo;
  - las que quedan por debajo de `min_budget_usd` van en gris con
    `<provider> pide al menos <min> USD`.
- **Tiempo límite**: primero lo propuesto (`timeout_s`), después 15m, 30m,
  45m, 1h y 2h, y `otro…`, sin repetir.

**Firma** (`A`, `I`, y `n` cuando el siguiente paso es una firma):

- el texto de la firma de la capa 1 (`tarjetacmd.TextoFirma`);
- `Se va a ejecutar: hoom aprobar <slug> --si`, o `hoom integrar`;
- los botones `[ Cancelar ]`, elegido al abrir, y el de la acción
  (`Aprobar spec`, `Integrar`).

Solo `enter` sobre el botón de la acción ejecuta. La regla de la v1 que ata
la confirmación a lo que muestra sigue igual.

**Guardar** (`s`, y `n` cuando el siguiente paso es guardar):

- `Se guarda en git:` y las rutas (v1);
- `Se va a ejecutar: hoom guardar <slug>`;
- los botones `[ Guardar ]`, elegido al abrir, y `Cancelar`.

**Acciones** (`espacio`). El contenido es el del menú de la v1:

- una línea por acción, con su tecla, su etiqueta y su verbo;
- las deshabilitadas, en gris con su `why`;
- las que no tienen verbo, en gris con `sin verbo en la terminal`;
- las de experto, solo en modo experto.

`enter`, o la tecla de una acción, abre el modal de esa acción, o ejecuta
`u`.

**Ayuda** (`?`). Todas las teclas, agrupadas en este orden: el panel con
foco, "En todas partes", los otros paneles, "Modales" y "Mouse". El grupo
"Mouse" dice `para seleccionar texto con el mouse, manten Option (macOS) o
Shift`. La ayuda se desplaza, y `esc` o `?` la cierran.

### El mouse

- Salvo con `HOOM_MOUSE=0`, al entrar la TUI escribe `\x1b[?1000h\x1b[?1006h`
  (clics y rueda, en formato SGR). Los apaga con `\x1b[?1006l\x1b[?1000l`
  al salir con `q`, con `ctrl-c`, con un error y con un panic, y antes de
  darle la terminal al editor. Los vuelve a pedir al recuperarla.
- Un clic en un panel le da el foco; si cae en una fila, además la elige.
  Un clic en un panel plegado le da el foco, y entonces se despliega.
- La rueda mueve la selección en los paneles de la izquierda y desplaza 3
  líneas en [0] y en la ayuda.
- Con un modal abierto:
  - un clic fuera del modal no hace nada;
  - un clic en un campo le da el foco;
  - un clic en una opción de una lista abierta la elige y cierra la lista;
  - un clic en un botón lo elige y no lo ejecuta: la línea de mensajes dice
    `enter para <accion>` (ej. `enter para pedir al writer`).
- Ningún clic ni la rueda ejecuta un verbo ni abre un modal.
- Una terminal que no manda eventos SGR no tiene mouse; la TUI funciona
  igual con el teclado.

### Los datos

- **El tablero**: como en la v1, la misma función que `hoom board --json`,
  cada 2 segundos.
- **El proyecto y los hallazgos** ([1], [4] y el [0] de cada uno):
  - el check y el último veredicto, de la función de `hoom status`;
  - el gasto de hoy, de la de `hoom costo`;
  - los hallazgos abiertos, de la de `hoom finding list --open`;
  - los problemas, de la de `hoom board doctor`;
  - la rama, de `gitx`.

  Se leen al abrir, cada 10 segundos y cuando termina un verbo que la TUI
  ejecutó. Si una lectura falla, la pantalla se queda con la última y la
  línea de mensajes dice `no se pudo leer el proyecto: <error>` o `no se
  pudieron leer los hallazgos: <error>`.
- **El detalle de una tarjeta** (spec, historia, hallazgos de su tarea y
  cola del sobre): la misma lectura de la v1, cuando cambia la selección y
  en cada relectura.
- **Las líneas del archivo de un hallazgo**: se leen por el `os.Root` del
  proyecto cuando cambia la selección, con las reglas de rutas de la v1.

### `hoom costo`: el gasto de hoy

- `hoom costo` sin slug suma una primera línea: `hoy: <usd> USD en <n>
  trabajos`, con `, <k> sin dato de costo` si falta el costo de alguno, o
  `hoy: sin trabajos`.
- `hoom costo --json` suma el objeto
  `"hoy": {"runs": <n>, "costo_usd": <x>, "sin_costo": <k>}`.
- Cuenta los runs que empezaron hoy en la hora local de la computadora.
  `costo_usd` suma los runs con costo, con la misma regla que el resto de
  `hoom costo` (USD equivalente API), y es 0 si no hay ninguno.
- `hoom costo <slug>` no cambia.

### El catálogo de modelos

El catálogo vive en un solo lugar de hoom (`internal/modelos`) y lo lee la
TUI. Tiene una entrada por cada provider al que hoom le pasa el modelo, hoy
claude y codex. Cada entrada tiene sus modelos y sus esfuerzos, con su
línea y en el orden de las tablas, y la versión de la CLI contra la que se
verificó. Las líneas van sin acentos, iguales en UTF-8 y en ASCII.
Contenido inicial, verificado el 2026-11-09:

**claude** (Claude Code 2.1.319). Los alias salen del binario y de
`claude --help`. Las líneas traducen la guía de la propia CLI: "Fable for
the hardest problems, Opus for complex work, Sonnet for most tasks, Haiku
for quick questions", y "Opus in plan mode, else Sonnet" para `opusplan`.

| Modelo | Línea |
|---|---|
| `fable` | para los problemas mas dificiles |
| `opus` | para trabajo complejo |
| `sonnet` | para la mayoria de las tareas |
| `haiku` | para preguntas rapidas |
| `opusplan` | opus mientras planea, sonnet despues |
| `fable[1m]` | fable con contexto de 1M, para sesiones largas |
| `opus[1m]` | opus con contexto de 1M, para sesiones largas |
| `sonnet[1m]` | sonnet con contexto de 1M, para sesiones largas |

**codex** (codex-cli 0.160.0). Son los modelos de
`~/.codex/models_cache.json` con `visibility: list`, en su orden. Las
líneas traducen su `description`.

| Modelo | Línea |
|---|---|
| `gpt-6.1-sol` | el de todos los dias para programar, el mas nuevo |
| `gpt-6-astra` | el mas capaz, para el trabajo mas exigente |
| `gpt-6.1-terra` | Terra 6.1 de codex |
| `gpt-6-sol` | el de todos los dias, de la generacion anterior |
| `gpt-6-luna` | rapido y barato, para tareas faciles |
| `gpt-5.6-sol` | el de todos los dias, de una generacion mas vieja |
| `gpt-5.6-terra` | equilibrado, para trabajo directo, mas viejo |
| `gpt-5.6-luna` | rapido y eficiente, mas viejo |

**Esfuerzos.** claude (`--effort`): `low`, `medium`, `high`, `xhigh` y
`max`. codex (`model_reasoning_effort`): los mismos y `ultra`. Sus líneas:

| Esfuerzo | Línea |
|---|---|
| `low` | rapido, razona poco |
| `medium` | equilibra velocidad y profundidad |
| `high` | mas profundidad para problemas complejos |
| `xhigh` | profundidad extra |
| `max` | la maxima profundidad, el mas lento |
| `ultra` | el maximo y delega tareas solo; no todos los modelos lo aceptan |

**`models` en `hoom.yaml`.** Amplía el catálogo:

```yaml
models:
  codex:
    - name: gpt-7-sol
      description: el nuevo de todos los dias
```

- Solo acepta los providers a los que hoom les pasa el modelo.
- Cada entrada lleva `name`, obligatorio y que no empiece con `-`, y
  `description`, opcional.
- Un error es un `hoom.yaml` inválido: `hoom tablero` sale con 1 antes de
  tomar la pantalla, como cualquier verbo. Los textos:
  - `models: provider desconocido "<p>" (validos: claude, codex)`;
  - `models.<p>: hoom no le pasa el modelo a <p>`, para gemini y opencode;
  - `models.<p>[<i>]: falta name`;
  - `models.<p>[<i>]: el nombre no puede empezar con '-'`;
  - `models.<p>[<i>]: clave desconocida "<k>" (validas: name,
    description)`.

### La dependencia

Sin cambios: `golang.org/x/term` (que trae `golang.org/x/sys`) y nada más.
Los paneles, los modales, el mouse y la tabla de anchos se escriben en
hoom.

### Cómo se prueba

- El modelo sigue siendo puro, como en la v1. Recibe teclas, eventos del
  mouse (nuevo), lecturas, el reloj y el tamaño, y devuelve una pantalla y
  órdenes.
- **Archivos dorados** a 120x32, 80x24 y 60x15, sin color y en ASCII, y una
  vez con signos UTF-8. Con color, una prueba por elemento mira sus
  secuencias SGR: el foco, la selección y cada estado.
- **Los archivos dorados de la v1 se reemplazan.** Es el cambio de pantalla
  que este spec aprueba, no un test debilitado.
- Los tests de la v1 que citan un criterio re-expresado se adaptan al
  comportamiento nuevo y citan los dos ids, como hizo verbos-de-la-tarjeta
  con el criterio 409. Los tests de los criterios que siguen igual solo
  cambian dónde miran la pantalla (la línea de mensajes, los dorados),
  nunca lo que aseveran.
- Lanzar pasa por la interfaz de la v1, sin ejecutar nada.
- Un test compara el catálogo de `internal/modelos` con las tablas de este
  spec.
- Ningún test necesita una terminal real.

### Documentación

La sección "El tablero en la terminal" del README se reescribe con:

- la pantalla con paneles y el principio rector;
- las teclas por panel y el mouse (`HOOM_MOUSE`);
- el catálogo y `models` en `hoom.yaml`.

La sección de `hoom costo` suma la línea `hoy`.

## Casos límite y errores esperados

- **Terminal entre 60 y 79 columnas**: una columna con los paneles
  plegados.
  ```
  ─[1] Proyecto · check VERDE─────────────────────────────────
  ╔[2] Tarjetas══════════════════════════════════════════════╗
  ║ NECESITA TU DECISIÓN                                     ║
  ║ ▸ nuc-tenancy                                      ●✋○○○║
  ║ EN CURSO                                                 ║
  ╚══════════════════════════════════════════════════1 de 10═╝
  ─[3] Corriendo 1────────────────────────────────────────────
  ─[4] Hallazgos 2────────────────────────────────────────────
  ┌[0] nuc-tenancy · Aislar los datos por tenant─────────────┐
  │ ¿Qué falta?  el spec espera tu aprobación                │
  │ Siguiente paso  Aprobar spec  [A]                        │
  └──────────────────────────────────────────────────────────┘
  ─Registro · 14:02 Pedir al arquitecto · nuc-tenancy · hecho─

   ↑↓ elegir · enter ver · espacio acciones · ? todas
  ```
- **Una lista de opciones más larga que su modal**: se desplaza y su borde
  dice `<n> de <m>`.
- **La terminal cambia de tamaño con un modal abierto**: el modal se vuelve
  a centrar y sigue la regla de lo que no entra.
- **El modelo de `hoom.yaml` no está en ninguna lista**: aparece elegido
  antes de `otro…`, con `de hoom.yaml`.
- **Un hallazgo sin archivo**: su [0] va sin líneas y `e` dice `el
  hallazgo no nombra un archivo` (v1).
- **El archivo de un hallazgo no se puede leer**: su [0] dice `no se pudo
  leer <archivo>: <error>`.
- **Una ruta fuera del proyecto**: el mensaje de la v1, sin leer ni abrir
  nada.
- **Sin git o sin veredicto**: [1] dice `sin git` o `sin veredicto
  todavia`.
- **Un trabajo que termina antes de su primer registro**: la línea de
  mensajes en rojo (v1) y la entrada del Registro con `fallo: <linea>`.
- **Un pedido con saltos de línea**: en `Se va a ejecutar` y en el Registro
  los saltos se ven como espacios (regla de controles de la v1). El
  argumento del verbo los conserva.
- **Más filas que alto en [3] o [4]**: se desplazan con `<n> de <m>`.
- **La tarjeta elegida desaparece** en una lectura:
  - la selección pasa a la siguiente y [0] la muestra;
  - si el foco estaba en [0] mostrándola, vuelve a [2] y la línea de
    mensajes dice `la tarjeta <slug> ya no existe`.
- **Un provider rechaza un modelo** (por ejemplo, Opus con un plan que no
  lo incluye): el verbo falla y el Registro muestra su error en rojo.

## Criterios de aceptación

- CA-550: con 80 columnas o más la pantalla tiene dos columnas: [1], [2], [3] y [4] apilados a la izquierda en `Ancho/3` celdas, y [0] y el Registro a la derecha; entre 60 y 79 columnas hay una sola columna con los paneles de la izquierda arriba, [0] y el Registro plegado; los paneles que no entran con su mínimo se pliegan a su título con la cantidad en el orden [1], [4], [3], Registro, nunca el que tiene el foco (archivos dorados a 120x32, 80x24 y 60x15).
- CA-551: siempre hay un panel con foco y su borde es de línea doble (`=` y `#` en ASCII) y, con color, verde y en negrita; `1`, `2`, `3`, `4` y `0` llevan el foco a ese panel, `tab` y `shift-tab` lo mueven en el orden 1, 2, 3, 4, 0, y al abrir el foco está en [2]. Re-expresa CA-520.
- CA-552: en cada panel `↑`, `↓`, `k` y `j` mueven la selección (en [0], desplazan) y `g` y `G` van a la primera y a la última; la fila elegida de cada panel de la izquierda empieza con `▸`, y en el panel con foco es además, con color, una barra de fila entera en video inverso; el panel se desplaza para que la fila elegida se vea y su borde de abajo dice `<n> de <m>` cuando las filas no entran. Re-expresa CA-520.
- CA-553: [0] muestra lo elegido en el último panel de la izquierda que tuvo el foco y cambia con cada movimiento de la selección, sin `enter`; `enter`, `→` y `l` pasan el foco a [0], `esc`, `←` y `h` lo devuelven al panel de donde vino, y `enter` sobre la línea de Hecho la abre y la cierra. Re-expresa CA-520 y CA-521.
- CA-554: `/` pone el foco en [2], la línea de mensajes muestra `/<texto>▌` y [2] se filtra mientras se escribe por slug o título sin distinguir mayúsculas ni acentos; `enter` y `esc` cierran la búsqueda, sueltan el filtro y dejan elegida la tarjeta que estaba elegida. Re-expresa CA-520.
- CA-555: [2] muestra las secciones de la v1 en su orden, cada tarjeta una sola vez, sin secciones vacías salvo Hecho cerrada con su cantidad, con los títulos en el color de su estado; cada fila tiene la marca, el slug y la tira alineada al borde derecho, el slug se corta con `…` para dejar la tira y la fila va sin tira cuando al slug le quedan menos de 8 celdas; una tarjeta con `red` o `interrupted` lleva el slug en rojo (archivos dorados). Re-expresa CA-518.
- CA-556: [0] de una tarjeta muestra, en este orden, `¿Qué falta?` con el `plain`, `Siguiente paso` con la etiqueta y la tecla de la acción que abre `n` (o el texto de `hoom siguiente` sin tecla), la columna con el gasto, quién escribió y revisó, el rojo y el doctor, una línea por segmento de la tira, los criterios del spec con `✓`, `✗` o `⚑`, los hallazgos de su tarea con los abiertos primero, la historia y la narración del sobre en curso o del último (64 KB); `x` alterna el modo experto, la línea de mensajes empieza con `EXPERTO · ` y [0] suma `next`, `missing` y el id de cada criterio. Re-expresa CA-521, CA-523 y CA-531.
- CA-557: con el foco en [0] de una tarjeta, `↑`, `↓`, `k`, `j`, `pgup`, `pgdn`, `g` y `G` la desplazan; `]c` y `[c` llevan la vista al siguiente y al anterior criterio con `✗` (o la línea de mensajes dice `no hay mas criterios sin prueba`), y `e` ordena abrir el spec en el editor en la línea del primer criterio a la vista, o en la línea 1. Re-expresa CA-522.
- CA-558: [3] tiene una fila por tarjeta con `running` (slug, rol y tiempo corrido, con su límite si entra) y las tarjetas con `interrupted` en rojo, o `nada corriendo`; su [0] muestra la línea `<rol> · <paso> · <tiempo> · narró hace <n>s · <gasto>` y los últimos 64 KB del log del sobre, que siguen el final mientras llegan eventos salvo que la persona haya subido, y `G` vuelve al final. Re-expresa CA-519 y CA-523.
- CA-559: [4] lista los hallazgos abiertos de todo el proyecto, de la función de `hoom finding list --open`, con los high primero, después los medium y después los low, cada fila con severidad, tarea y `<archivo>:<linea>`, o `sin hallazgos abiertos`; su [0] muestra severidad, lente, tarea, la descripción entera, las líneas del archivo alrededor de la del hallazgo marcada con `▸` (o `no se pudo leer <archivo>: <error>`) y `para cerrarlo: hoom finding resolve <id> ...`; `e` ordena abrir el editor en esa línea con las reglas de la v1, sin leer ni abrir una ruta fuera del proyecto. Re-expresa CA-524.
- CA-560: [1] muestra el proyecto y la rama (o `sin git`), el check (`check VERDE · verify hace <tiempo>`, `check ROJO: <motivo>` o `sin veredicto todavia`), el gasto de hoy y la tarjeta fijada (o `sin tarjeta fijada`); `u` ejecuta `hoom usar <slug>` sin modal y [1] muestra la tarjeta fijada tras la siguiente lectura; su [0] muestra el último veredicto (id, hace cuánto, sus cuentas y su ruta), la línea del check y los problemas de `hoom board doctor` con su acción. Re-expresa CA-531.
- CA-561: `hoom costo` sin slug imprime primero `hoy: <usd> USD en <n> trabajos` (con `, <k> sin dato de costo` cuando falta alguno) o `hoy: sin trabajos`, y `hoom costo --json` tiene `hoy` con `runs`, `costo_usd` y `sin_costo`, contando los runs que empezaron hoy en la hora local; `hoom costo <slug>` no cambia.
- CA-562: los datos de [1] y [4] se leen al abrir, cada 10 segundos y cuando termina un verbo que la TUI ejecutó; una lectura que falla deja lo último en pantalla y la línea de mensajes dice `no se pudo leer el proyecto: <error>` o `no se pudieron leer los hallazgos: <error>`.
- CA-563: cada verbo que la TUI ejecuta suma al Registro una entrada con la hora, la etiqueta de la acción, el slug y su estado (`lanzando…`, `corriendo`, `hecho` o en rojo `fallo: <ultima linea de su error>`) y una segunda línea con el mismo comando que mostró `Se va a ejecutar`; el fin de un trabajo suma `<rol> termino en <slug>: <plain>` en verde o en rojo con la regla de la v1; abrir el editor suma su entrada con el comando del editor; plegado, el Registro muestra la primera línea de la última entrada.
- CA-564: la última línea muestra, en palabras, solo las teclas del panel con foco o del modal abierto; `?` abre un modal con todas las teclas agrupadas, primero el grupo del panel con foco y con los grupos "Modales" y "Mouse"; la línea de teclas, la ayuda y el despacho salen de una sola tabla: toda tecla con efecto está en la ayuda y toda tecla de la ayuda tiene efecto. Re-expresa CA-532.
- CA-565: los modales se dibujan centrados encima de todo con el borde del foco y, con color, lo de atrás en gris; mientras hay uno abierto las teclas van solo a él; `esc` lo cierra sin ejecutar nada (con una lista abierta, cierra solo la lista); el botón elegido va entre corchetes, `←`, `→` y `tab` pasan de botón y `enter` sobre un botón lo ejecuta o cancela.
- CA-566: toda tecla de acción salvo `u` produce como primera orden un modal y nunca un lanzamiento; `n` abre el modal de su acción sea de pedir, de firma o de guardar, y sin siguiente paso la línea de mensajes dice lo que diría `hoom siguiente`.
- CA-567: `espacio` abre el modal de acciones con una línea por acción de la tarjeta en su orden (tecla, etiqueta y verbo), las deshabilitadas en gris con su `why`, las sin verbo en gris con `sin verbo en la terminal` y las de experto solo en modo experto; `enter` o la tecla de una acción abre el modal de esa acción, o ejecuta `u`. Re-expresa CA-526.
- CA-568: el modal de pedir tiene Pedido, Provider, Modelo, Esfuerzo, Tope, Tiempo límite y los botones, en ese orden de `tab`; el Pedido se edita como en la v1 y `enter` en él lleva el foco al botón; en un selector `←` y `→` cambian la opción saltando las grises y `enter` o `espacio` abren la lista con la línea de cada opción; cada selector muestra su origen (`de hoom.yaml`, `de la tarjeta`, `del provider` o `elegido`) y la línea gris de abajo la línea de la opción del selector con foco; `otro…` acepta texto y un texto vacío vuelve a la opción de antes; `Se va a ejecutar:` muestra el comando con solo los flags cambiados en hasta dos líneas, acortando `-m` si no entra; los botones son la etiqueta de la acción, elegida, y `Cancelar`; un pedido vacío no lanza; el reviewer no tiene Pedido y reanudar no cambia el provider. Re-expresa CA-527.
- CA-569: Modelo ofrece `por defecto del provider`, los modelos del catálogo del provider elegido, los `models.<provider>` de `hoom.yaml` (sin repetir un nombre del catálogo) y `otro…`, con el `model` de la acción elegido al abrir (sumado antes de `otro…` cuando no está); Esfuerzo ofrece `por defecto del provider`, los esfuerzos del catálogo y `otro…`; con un provider sin `Model` en sus `Capabilities`, Modelo y Esfuerzo van en gris con `hoom no le pasa el modelo a <provider>` y `hoom no le pasa el esfuerzo a <provider>`.
- CA-570: si el `origen.model` de la acción es `hoom.yaml`, `por defecto del provider` va en gris con `hoom.yaml fija <modelo> para el <rol>`, cambiar de provider deja ese modelo elegido y, si no está en la lista del provider nuevo, la línea gris dice `<modelo> no esta en la lista de <provider>: puede rechazarlo`; sin valor de `hoom.yaml`, cambiar de provider vuelve Modelo y Esfuerzo a `por defecto del provider` y volver al provider por defecto devuelve lo propuesto; el esfuerzo sigue las mismas reglas con `origen.effort`.
- CA-571: Tope ofrece primero lo propuesto (el `budget_usd` de la acción, o `sin tope` si no trae), después 1, 2, 5, 10 y 20 USD y `otro…` sin repetir, nunca `sin tope` cuando lo propuesto es un tope, con las opciones por debajo de `min_budget_usd` en gris con `<provider> pide al menos <min> USD`, y con un provider sin tope va `‹ sin tope ›` en gris con `<provider> no acepta tope`; Tiempo límite ofrece primero lo propuesto, después 15m, 30m, 45m, 1h y 2h, y `otro…` sin repetir, y el del reviewer va en gris con `de review.timeout`.
- CA-572: el catálogo de `internal/modelos` es exactamente el de las tablas de este spec: los modelos de claude y de codex, los esfuerzos de cada uno, sus líneas, su orden y la versión de la CLI contra la que se verificó cada provider.
- CA-573: `hoom.yaml` acepta `models.<provider>` con entradas `name` y `description` para claude y codex, y esas entradas aparecen en el selector de Modelo; un provider desconocido, gemini u opencode, una entrada sin `name`, un nombre que empieza con `-` o una clave desconocida son un error de carga con el texto de este spec, y `hoom tablero` sale con 1 antes de tomar la pantalla.
- CA-574: `A` e `I` abren el modal de firma con el texto de la capa 1, `Se va a ejecutar: hoom aprobar <slug> --si` (o `hoom integrar`) y los botones `Cancelar`, elegido al abrir, y la etiqueta de la acción, y solo `enter` sobre la etiqueta ejecuta el verbo; `s` abre el modal de guardar con las rutas, `Se va a ejecutar: hoom guardar <slug>` y los botones `Guardar`, elegido, y `Cancelar`; la regla de la v1 que ata la confirmación a lo que muestra sigue igual. Re-expresa CA-529.
- CA-575: salvo con `HOOM_MOUSE=0`, la TUI pide el mouse con `\x1b[?1000h\x1b[?1006h` y lo apaga al salir con `q`, con `ctrl-c`, con un error y con un panic, y mientras el editor tiene la terminal; un clic en un panel le da el foco y elige la fila bajo el clic, la rueda mueve la selección o desplaza [0], y en un modal un clic enfoca un campo, elige una opción de una lista abierta o elige un botón sin ejecutarlo (la línea de mensajes dice `enter para <accion>`); ningún evento del mouse produce la orden de lanzar ni abre un modal.
- CA-576: con `NO_COLOR` la pantalla no tiene secuencias de color, el foco se distingue por la línea doble y la selección por `▸`; con `HOOM_ASCII=1` o sin un locale UTF-8, los bordes, el `▸`, las flechas de los selectores y el corte usan los signos ASCII de la v1 y el borde del foco usa `=` y `#` (archivos dorados de los dos modos).
- CA-577: un carácter de ancho doble (East Asian Wide, Fullwidth o un emoji que se presenta como emoji) ocupa dos celdas y una marca combinante ninguna, según una tabla propia de hoom; una tarjeta con `漢字` o `🚀` en su título no corre el borde derecho de [0] (archivo dorado).
- CA-578: si la tarjeta elegida desaparece en una lectura, la selección pasa a la siguiente y [0] la muestra; si el foco estaba en [0] mostrándola, vuelve a [2] y la línea de mensajes dice `la tarjeta <slug> ya no existe`; un proyecto sin items muestra `no hay tarjetas: hoom item add "<titulo>"` en [2]. Re-expresa CA-535.
- CA-579: el README describe el tablero con paneles, el modal que dice qué se va a ejecutar y `HOOM_MOUSE`. [verifica: grep -q "Se va a ejecutar" README.md && grep -q "HOOM_MOUSE" README.md && grep -q "panel con foco" README.md]

## Decisiones

- **El motor sigue a mano, sobre `golang.org/x/term`** (Henry, 2026-11-09).
  Las tres opciones se midieron en un módulo de prueba:
  - Bubble Tea v1.3.10 con Lip Gloss v1.1.0 y Bubbles v1.0.0: 25 módulos
    en `vendor/` (23 nuevos para hoom), unas 60 mil líneas de Go sin
    contar `x/sys`. La v2 actual (`charm.land/bubbletea/v2` 2.1.1) pide Go
    1.26, y el CI usa 1.25.
  - tcell v2.13.10: 7 módulos (5 nuevos), unas 35 mil líneas.
  - A mano: cero módulos nuevos.

  La v1 ya compone cajas encima de la lista, lee el teclado en macOS, Linux
  y Windows, y tiene el modelo puro con dorados (5.126 líneas de código y
  7.354 de tests). Lo que falta es dibujo. Además, el vendor de cualquier
  librería vuelve a pasar el tope de evidencia de la review (448 KiB) y
  habría que aislarlo en su propio commit, como en la v1. Lazygit usa gocui
  sobre tcell, no Bubble Tea: lo que Henry quiere de lazygit sale del
  diseño, no del motor.
- **El catálogo es de hoom y `hoom.yaml` lo amplía** (Henry, 2026-11-09).
  - Las entradas se verificaron contra cada CLI; el harness no ofrece lo
    que no verificó.
  - Se descartó leer `~/.codex/models_cache.json`, aunque da los modelos
    exactos de cada cuenta. Es un archivo interno de codex, sin contrato,
    y si cambia de formato el selector volvería en silencio al catálogo.
  - Los nombres de codex envejecen con cada versión. Lo cubren
    `models.codex` y `otro…`.
- **Un spec nuevo que re-expresa, no una enmienda** (Henry, 2026-11-09).
  Una enmienda es chica para un rediseño y borraría la v1 como referencia.
  Un spec completo repetiría nueve criterios que no cambian y pasaría de
  los tres minutos de lectura.
- **El mouse sirve para moverse, nunca para ejecutar** (Henry,
  2026-11-09). Quien viene de interfaces gráficas va a hacer clic. Un clic
  de más nunca lanza un trabajo ni firma.
- **[0] sigue a la selección sin `enter`**, como lazygit. El junior ve el
  contexto mientras se mueve; `enter` solo sirve para leer en grande y
  desplazar.
- **Un documento por tarjeta en vez de cinco pestañas.** Arriba y en
  palabras va lo que falta y el siguiente paso; el resto queda debajo, a
  un desplazamiento. Las pestañas de la v1 escondían lo importante detrás
  de una tecla.
- **`e` abre en el editor en todos lados y `enter` siempre es "ver en
  grande".** En la v1, `o` abría el spec y `enter` abría un hallazgo: dos
  teclas para lo mismo y un `enter` con dos significados.
- **`enter` en el Pedido no lanza; lleva al botón.** En la v1 lanzaba desde
  cualquier campo. Con selectores donde `enter` abre una lista, lanzar
  solo desde el botón le da a `enter` un único significado peligroso, y
  quien lanza siempre ve el botón elegido y el comando.
- **Los botones dicen el verbo de la acción** (su etiqueta en la tarjeta),
  no `Firmar`. En las firmas, `Cancelar` sigue elegido al abrir (v1); en
  pedir y guardar va elegida la acción, porque abrir el modal ya fue la
  decisión.
- **El foco con línea doble y la selección con `▸`, además del color.** Se
  ven con `NO_COLOR` y para quien no distingue colores. La barra de
  selección va en video inverso porque sigue el tema de la terminal.
- **Las teclas de la v1 se conservan.** Quien aprendió la v1 no reaprende,
  y cada tecla de acción sigue siendo la del verbo que enseña.
- **[1] y [4] se leen cada 10 segundos, no cada 2.** El check calcula la
  huella del árbol, que pesa en proyectos grandes. El tablero sigue cada 2
  segundos.
- **El gasto de hoy sale de `hoom costo`, no de la TUI.** La regla de
  primero la CLI: la TUI no suma gastos por su cuenta, y quien no usa la
  TUI ve lo mismo en `hoom costo`.
- **`u` corre sin modal.** Solo fija la tarjeta para la terminal: no lanza,
  no firma y no commitea. Un modal ahí sería ruido.
- **Una tabla de anchos propia** en lugar de `go-runewidth` o `uniseg`,
  para no sumar módulos. Con bordes, un emoji que corre el borde se nota
  mucho más que en la lista de la v1.
- **Las tarjetas interrumpidas también van en [3].** Esperan `R`, y [3] es
  donde se mira lo que corre o dejó de correr.

## Riesgos y deuda aceptada

- **Más código propio.** Los paneles, el pliegue, los modales, los
  selectores, el mouse y la tabla de anchos suman unas 2 a 3 mil líneas y
  sus tests. Lo contienen el modelo puro y los dorados.
- **El mouse a mano.** Solo se lee el formato SGR (1006); una terminal sin
  él no tiene mouse. Con el mouse activo, seleccionar texto pide mantener
  Option o Shift (la ayuda lo dice) o arrancar con `HOOM_MOUSE=0`.
- **Los anchos.** Las terminales no coinciden en algunos casos: emojis con
  selector de variación y caracteres de ancho ambiguo. En esos casos una
  línea se puede correr una celda.
- **El catálogo envejece.**
  - codex cambia los nombres con cada versión.
  - Algunos modelos dependen del plan: Opus no viene con el plan Pro de
    Claude, y el contexto de 1M depende de la cuenta. El provider rechaza
    y el Registro muestra el error.
  - Actualizar el catálogo es actualizar sus tablas en un spec.
- **La capa 1 aplica el modelo y el esfuerzo de `hoom.yaml` a cualquier
  provider.** Lo hace `valores.pick`, y no hay flag para volver al del
  provider. El modal lo dice, pero no lo arregla. Arreglarlo es un cambio
  de la capa 1, en su propio spec.
- **Sin tope no se puede pedir cuando la tarjeta propone uno.** La capa 1
  rechaza `--budget-usd 0`. El selector no lo ofrece; habilitarlo también
  es un cambio de la capa 1.
- **A 80x24, [2] tiene 26 columnas.** Los slugs largos se cortan; el
  título de [0] los muestra enteros.
- **Leer el proyecto cada 10 segundos** calcula la huella del árbol en
  proyectos grandes.
- **Dos pieles que mantener.** El diálogo del Studio sigue con Modelo en
  texto libre hasta su propio spec.
