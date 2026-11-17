# Spec: el Studio dice tu siguiente paso (Studio nuevo: Ahora y Tarjeta)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start studio-tu-siguiente-paso`.
Depende de: `tablero-tu-siguiente-paso.md` (el tablero v3, integrado con el
PR 93), `acciones-desde-la-tarjeta.md`, `historia-doctor-y-cinta.md` y
`studio-v1-dashboard.md` (CA-10: sin assets de red).
Origen: Henry, 2026-11-11: quiere un Studio "más dinámico, que lo use el
90 %", donde siempre se vea en qué está hoom y cuál es el siguiente paso; de
ahí salió el lienzo "hoom Studio nuevo"
(https://claude.ai/artifact/5ua6wr92SHiA7w2mmjvbxh). El 2026-11-16 eligió
hacer primero el tablero de terminal y después el Studio. El 2026-11-17,
con el tablero v3 integrado, eligió el alcance de este spec:

- solo las pantallas "Ahora — escritorio" y "Tarjeta — detalle" del lienzo;
- Ahora pasa a ser la portada, y las vistas de hoy siguen como están;
- la línea gráfica hoomdev del lienzo (paleta, fuentes y microcopy).

Re-expresa CA-319 en su versión de `acciones-desde-la-tarjeta.md` (las
pestañas y la lista exacta de archivos de la UI embebida). Los demás
criterios del Studio siguen igual.

## Objetivo

Que quien abre el Studio sepa, sin buscar, lo mismo que ya sabe quien abre
`hoom tablero`:

1. **Qué tiene que hacer ahora**: una frase y un botón, arriba de todo.
2. **En qué está cada tarjeta**: si le toca a la persona, si hoom está
   trabajando, si algo la frenó o si todavía no empezó, con el título de la
   tarjeta y su estado en palabras.

Hoy la portada del Studio es el Cockpit (despachar un pedido, sobres, runs y
el trinquete), y el Tablero es una pestaña con ocho columnas del mismo peso.
Henry: "me pierdo, no sé en qué está, qué debo hacer a continuación". El
tablero v3 resolvió eso en la terminal con una regla: **quién tiene la
pelota**. Este spec lleva la misma regla al Studio con dos pantallas:

- **Ahora**, la portada: la banda TU SIGUIENTE PASO y las tarjetas
  agrupadas en Te toca, En curso, Detenidas, En cola y Hecho.
- **Tarjeta**, el detalle de una: el camino de 8 pasos, AHORA, SIGUE, la
  historia y la evidencia.

Para que el Studio y el tablero no puedan decir cosas distintas, la regla
deja de vivir en `internal/tablero` y pasa a `boardcmd`: el JSON de la
tarjeta la trae ya resuelta (`pelota`), el tablero la usa y el Studio solo
la pinta. El principio del Studio sigue: lo que el CLI no tiene, el Studio
no lo inventa, y la pantalla muestra lo que hoom comprobó, no lo que un
modelo cuenta.

## No-goals

- **No hay pantallas de celular ni del aparato** (las cinco del lienzo:
  Cel-Ahora, Cel-Firmar, Cel-Aceptar y las dos de 480×480). Quedan para
  otro spec.
- **No se rediseñan las vistas de hoy.** Cockpit, Tablero, Trinquete,
  Escenario, Feed y los cajones (Veredictos, Contexto, Hallazgos, Specs,
  Tareas e Intake) quedan con su contenido y su disposición. Solo cambian
  sus colores y sus fuentes (ver "La línea gráfica").
- **No hay pantallas de Evidencia ni de Costo.** El lienzo las nombra en la
  navegación pero no las diseña.
- **No hay acciones nuevas ni rutas nuevas.** Todo botón de Ahora y de
  Tarjeta abre el mismo diálogo de acciones que ya usa el Tablero
  (`abrirAccion` de `acciones.js`). No hay "Cancelar este trabajo", "Apagar
  el piloto", "Seguir solo" ni "Comentar la spec": para el piloto la
  Tarjeta dice el comando (`hoom item set <slug> --auto no`), pero no lo
  ejecuta.
- **No se muestra lo que hoom no sirve hoy:** la máquina conectada ("Mac de
  Henry — conectada"), las ventanas de 5 h de codex y de claude, los topes
  de autonomía (`ventana_max`, `reintentos_ante_rojo`,
  `rondas_de_hallazgos`), la ronda de hallazgos en curso, la regla del
  piloto que lanzó un trabajo y el tiempo "de la firma a la aceptación".
- **No hay tema claro.** La línea gráfica declara dos temas; el Studio usa
  el oscuro.
- **No cambia ningún verbo del CLI** ni el texto de `hoom board`, `hoom
  item show` ni `hoom tablero`. El JSON de la tarjeta y del tablero gana
  campos; ninguno cambia ni desaparece.
- **El Studio no conoce los pilotos lanzados por un `hoom tablero`.** Una
  tarjeta que un piloto lleva entre dos trabajos aparece donde la pone su
  estado en disco (ver "Riesgos").

## Contratos

### La regla, en un solo lugar: `boardcmd`

Un archivo nuevo, `internal/boardcmd/pelota.go`, tiene la regla de quién
tiene la pelota que hoy está en `internal/tablero` (`grupos.go`,
`detalle.go` y `pantalla.go`). `Derive` la calcula para cada tarjeta y
`Build` para el tablero.

**`Card` gana `Pelota Pelota` (`"pelota"` en el JSON)**, siempre presente,
nunca `null`:

```json
"pelota": {
  "grupo": "TE TOCA",
  "paso": {"accion": "aprobar", "etiqueta": "Aprobar spec", "texto": ""},
  "frase": "Firmá el spec de «Exportar reporte a PDF».",
  "ahora": {"frase": "El spec está listo y espera tu firma.", "gris": "<plain>"},
  "sigue": {"frase": "al firmar, te toca pedir los tests al test-writer", "gris": "con el piloto seguiria sola: hoom item set exportar-reporte-pdf --auto hasta-humano"},
  "camino": [
    {"nombre": "Pedido", "estado": "hecho", "texto": ""},
    {"nombre": "Spec", "estado": "hecho", "texto": ""},
    {"nombre": "Tu firma", "estado": "actual", "texto": "te toca a vos"},
    {"nombre": "Tests", "estado": "falta", "texto": ""}
  ]
}
```

(El camino tiene siempre 8 entradas; el ejemplo muestra 4.)

**`grupo`** es el de la tabla del tablero v3 ("Los grupos"), sin la
cláusula del piloto lanzado por un tablero, que `boardcmd` no puede saber:

| Orden | Grupo | Cuándo |
|---|---|---|
| 1 | `HECHO` | la columna es Hecho |
| 2 | `CORRIENDO` | tiene `running` |
| 3 | `DETENIDA` | tiene `interrupted`, o tiene `red` y `red.esperado` es `false` |
| 4 | `EN COLA` | la columna es Backlog |
| 5 | `TE TOCA` | cualquier otra |

**`paso`** es la regla del paso del tablero v3 ("El paso de una tarjeta")
sin la regla del piloto vivo y sin la regla 4 (Seguir solo), que son del
tablero:

1. en Hecho: ninguno, `la tarjeta ya esta en Hecho`;
2. con `running`: ninguno, `el <rol> esta trabajando: espera a que termine`;
3. con `interrupted`: ninguno, `el trabajo se corto: reanudalo o pedilo de nuevo`;
4. en Tu aprobación, `aprobar`; en Tu aceptación, `integrar`;
5. si no, la primera acción de la tarjeta cuando es de rol
   (`boardcmd.IsRoleAction`) o `guardar`;
6. si no hay ninguna de esas: ninguno,
   `en <column_name> no hay nada que hacer ahora: <plain>`.

Con una acción elegida: si la tarjeta no la tiene, ninguno, con
`en <column_name> no hay <etiqueta en minúscula>`; si la tiene deshabilitada,
ninguno, con su `why`. `accion` es el id de la acción solo cuando el paso
existe y está habilitado; entonces `etiqueta` es su `label` y `texto` es
`""`. Sin paso, `accion` y `etiqueta` son `""` y `texto` dice por qué.

**`frase`** es la frase de la caja del tablero v3, por grupo y paso:

| Cuándo | `frase` |
|---|---|
| `DETENIDA` | `Decidí cómo sigue «<titulo>».` |
| `TE TOCA` con `aprobar` | `Firmá el spec de «<titulo>».` |
| `TE TOCA` con `integrar` | `Aceptá «<titulo>».` |
| `TE TOCA` con `pedir-arquitecto` | `Pedile el spec al arquitecto para «<titulo>».` |
| `TE TOCA` con `pedir-test-writer` | `Pedile los tests al test-writer para «<titulo>».` |
| `TE TOCA` con `pedir-writer` | `Pedile el código al writer para «<titulo>».` |
| `TE TOCA` con `pedir-reviewer` | `Pedile la review al reviewer para «<titulo>».` |
| `TE TOCA` con `reanudar` | `Reanudá el trabajo de «<titulo>».` |
| `TE TOCA` con `guardar` | `Guardá el trabajo de «<titulo>».` |
| `TE TOCA` con otro paso o sin paso | `Mirá «<titulo>».` |
| `CORRIENDO`, `EN COLA` o `HECHO` | `""` |

`<titulo>` es el `titulo` del item en una línea, o el slug si está vacío.

**`ahora`** y **`sigue`** son la frase y la línea gris de AHORA y de SIGUE
del tablero v3 ("El detalle"), con estas diferencias:

- sin los casos del piloto lanzado por un tablero ni de su parada;
- con `interrupted`, `gris` es `ya no corre`;
- con un rojo no esperado, `gris` es `red.reason`; el tablero lo muestra
  solo en modo experto.

`gris` es `""` cuando no hay línea gris.

**`camino`** son 8 entradas, una por columna de `boardcmd.Columns`, en su
orden:

- `nombre`: Pedido, Spec, Tu firma, Tests, Código, Review, Tu aceptación y
  Hecho (los del tablero v3).
- `estado`: `hecho` antes de la columna de la tarjeta, `actual` en la suya
  y `falta` después. En `HECHO` las 8 son `hecho`.
- `texto`: solo en la entrada `actual`. Vale `te toca a vos` (TE TOCA),
  `<rol>, ahora` (CORRIENDO), `se detuvo` (DETENIDA) o `sin empezar` (EN
  COLA). Las demás entradas van con `""`.

**Ningún texto de `pelota` nombra una tecla, un botón ni un verbo de la
terminal.** Cada piel agrega cómo se actúa.

**`Board` gana `Pelota PelotaBoard` (`"pelota"` en el JSON)**:

```json
"pelota": {
  "orden": ["listado-proyectos", "exportar-reporte-pdf", "modulo-usuarios"],
  "caja": "listado-proyectos",
  "cuenta": {"te_toca": 2, "corriendo": 1, "detenida": 0, "en_cola": 0, "hecho": 0}
}
```

- `orden` son los slugs de todas las tarjetas en el orden de la lista del
  tablero v3: TE TOCA (de la columna más cerca de Hecho a la más lejos),
  CORRIENDO, DETENIDA, EN COLA y HECHO, y dentro de cada uno el orden del
  tablero.
- `caja` es la primera de TE TOCA en ese orden, si no hay la primera de
  DETENIDA, y si tampoco, `""`.
- `cuenta` tiene las cinco claves siempre, aunque valgan 0.

`boardcmd` exporta el orden como función (`OrdenPelota(cards, grupo)`), con
el grupo como parámetro, para que el tablero ordene con el suyo.

**El tablero usa la regla de `boardcmd`.** `internal/tablero` deja de
tener su propia tabla de grupos, su regla del paso, sus frases de la caja,
de AHORA, de SIGUE y del camino, y su orden de la lista: los toma de
`boardcmd`. Encima agrega lo que solo él sabe:

- el piloto que lanzó y sigue vivo (pasa la tarjeta a CORRIENDO);
- Seguir solo (la regla 4) y la parada del piloto;
- sus textos que nombran teclas (`R reanuda o pide de nuevo`, `ya no corre:
  R lo reanuda`);
- el modo experto y el modo ASCII.

Lo que dibuja `hoom tablero` no cambia: sus tests pasan sin tocarlos.

### La línea gráfica

**Los tokens** son los de la línea gráfica hoomdev
(`project/ds/hoomdev/tokens.json` del lienzo), como variables CSS en el
`:root` de `index.html`:

| Variable | Valor | Uso |
|---|---|---|
| `--hd-carbon` | `#0d1117` | fondo de página; texto sobre verde |
| `--hd-carbon-900` | `#090c10` | hundido (cajas de comando) |
| `--hd-grafito` | `#21262d` | tarjetas y paneles |
| `--hd-grafito-700` | `#2d333b` | botón secundario |
| `--hd-borde` | `#30363d` | todo borde |
| `--hd-blanco` | `#ffffff` | títulos |
| `--hd-gris-claro` | `#c9d1d9` | texto |
| `--hd-gris` | `#8b949e` | atenuado |
| `--hd-verde` | `#3ed18b` | marca, botón primario, trabajo abierto, VERDE |
| `--hd-verde-gate` | `#0ca30c` | PASS |
| `--hd-amarillo` | `#fab219` | te toca |
| `--hd-naranja` | `#ec835a` | error |
| `--hd-rojo` | `#d03b3b` | detenida, FAIL |
| `--hd-azul` | `#3987e5` | corriendo, foco |

**Las variables de hoy toman los valores hoomdev**, así las vistas viejas
cambian de color sin cambiar su CSS:

| Variable de hoy | Valor de hoy | Pasa a |
|---|---|---|
| `--surface` | `#1a1a19` | `#0d1117` |
| `--panel` | `#232322` | `#21262d` |
| `--panel-2` | `#2b2b29` | `#2d333b` |
| `--border` | `#3a3a37` | `#30363d` |
| `--text-2` | `#c3c2b7` | `#c9d1d9` |
| `--text-3` | `#8a897f` | `#8b949e` |

`--text`, `--good`, `--warning`, `--serious`, `--critical`, `--accent` y
`--human` quedan como están.

**Las fuentes** van embebidas en el binario y se sirven desde el mismo
origen; nunca desde la red:

| Archivo en `internal/servecmd/ui/fuentes/` | sha256 |
|---|---|
| `SpaceGrotesk-400.ttf` | `f40060430b03d78a3c848fd7a03afe43a69b929a390189df5f6dcac8b0566106` |
| `SpaceGrotesk-500.ttf` | `86a4ead05e0ccc556313f2953d80118a4e457c653dd953decef57ea651fa7d83` |
| `SpaceGrotesk-700.ttf` | `6a8d89aa96030d44419744bf6be8d86a1493989390a39c4ce2baa54ac7100233` |
| `JetBrainsMono-400.ttf` | `0c300074685498dea5579a65920e6441b31b76778c82d5f2e3eac2d4b7c4a8ab` |
| `JetBrainsMono-500.ttf` | `eb2184a7a02804a58a9e909d8e63353b14ad1f8f2df9969045053a4f0cc8fc5d` |

- **De dónde salen.** Son los archivos de la línea gráfica hoomdev. Este
  spec los trae en `.hoom/intake/studio-tu-siguiente-paso/fuentes/`, y la
  tarea los mueve a `internal/servecmd/ui/fuentes/` con `git mv`, sin
  convertirlos.
- **La licencia.** Las dos familias son SIL Open Font License 1.1, que
  permite empotrarlas si cada copia lleva el aviso de copyright y la
  licencia, "como archivo de texto aparte o en los campos de metadatos" de
  la fuente.
  - **En el binario**, cada `.ttf` ya trae en su tabla `name` su copyright
    (id 0) y el aviso de la licencia (ids 13 y 14).
  - **En el repo**, el texto entero va en
    `internal/servecmd/licencias/OFL-SpaceGrotesk.txt` y
    `internal/servecmd/licencias/OFL-JetBrainsMono.txt`. Cada archivo lleva
    el texto de la OFL 1.1 y, arriba, la línea de copyright de su fuente:
    - `Copyright 2020 The Space Grotesk Project Authors (https://github.com/floriankarsten/space-grotesk)`
    - `Copyright 2020 The JetBrains Mono Project Authors (https://github.com/JetBrains/JetBrainsMono)`
  - **Por qué fuera de `ui/`.** El test de CA-10 recorre todo lo embebido en
    `ui/` y prohíbe `http:` y `https:`, y el texto de la OFL los trae. Los
    `.ttf` no los traen en sus bytes: sus URLs van en UTF-16, y lo comprobé
    al traerlos.
- **Cómo se cargan.** `index.html` las declara con `@font-face` y
  `src: url("fuentes/<archivo>")`, `font-display: swap`.
- **Las familias.** El núcleo es `"Space Grotesk", system-ui, -apple-system,
  "Segoe UI", sans-serif`. `--mono` es `"JetBrains Mono", ui-monospace,
  "Cascadia Code", "Source Code Pro", Menlo, Consolas, monospace`.
- **Dónde se usan.** El `body` usa el núcleo y todo valor de máquina usa
  `--mono`: slugs, comandos, ids, tiempos y gates.

**El resto de la línea**, tomado del lienzo:

- **Bordes y sombras.** Los bordes son de 1 px. El borde izquierdo de 3 px
  es un estado (un trabajo abierto), nunca un adorno. Las sombras van solo
  en lo que flota (diálogo, toast).
- **Radios.** 4 px badge, 6 px botón, 8 px fila, 10 px tarjeta y panel,
  999 px chip.
- **Botones.**
  - Primario: fondo y borde `--hd-verde`, texto `--hd-carbon`, peso 700.
  - Secundario: fondo `--hd-grafito-700`, borde `--hd-borde`, texto
    `--hd-blanco`.
  - Los dos miden 44 px de alto como mínimo.
- **Badges.** 11 px, peso 700, con espaciado de letra de `.04em`, en
  MAYÚSCULA:
  - te toca: fondo `--hd-amarillo`, texto `--hd-carbon`;
  - CORRIENDO: fondo `--hd-azul`, texto `--hd-carbon`;
  - DETENIDA: fondo `--hd-rojo`, texto `--hd-blanco`.
- **Iconos.** No hay: los controles son palabras.
- **Microcopy.** Los títulos de bloque van en MAYÚSCULA (AHORA, SIGUE). Los
  subtítulos y el texto de la página van en minúscula y en voseo. Las
  frases que vienen de `pelota` se muestran tal cual.

### La portada y la navegación

**Las pestañas** de `index.html` pasan a ser tres, en este orden: **Ahora**
(`data-tab="ahora"`), **Tablero** (`data-tab="tablero"`) y **Cockpit**
(`data-tab="cockpit"`). Ahora es la que está activa al cargar. Debajo de
Tablero y de Cockpit sigue lo de hoy: run abierto, Escenario, Feed,
Trinquete y el detalle del Tablero.

**El hash dice la pantalla**, así el botón Atrás del navegador funciona:

| Hash | Pantalla |
|---|---|
| vacío o `#ahora` | Ahora |
| `#tablero` | la pestaña Tablero de hoy |
| `#cockpit` | la pestaña Cockpit de hoy |
| `#tarjeta/<slug>` | Tarjeta de `<slug>` |
| cualquier otro | Ahora |

- Cambiar de pestaña escribe el hash, y un `hashchange` cambia de pestaña.
- `mostrarPestana` sigue siendo la única que muestra y oculta las
  pestañas; suma Ahora y Tarjeta.
- Salir de Ahora o de Tarjeta corta su sondeo, como hoy hace el Tablero.
- Un slug que no es válido para `item.ValidSlug` lleva a Ahora sin pedir
  nada.

**La cabecera** sigue con lo suyo (título, proyecto, chip de `check`,
cajones, token y Verificar), con la línea gráfica nueva. No suma nada del
lienzo que hoom no sirva (ver No-goals).

**El pie** de Ahora y de Tarjeta dice:
`acá ves lo que hoom comprobó, no lo que el modelo cuenta`.

### La pantalla Ahora

Vive en `ui/ahora.js` y pide solo `GET /api/board`. Sondea con
`setTimeout(…, 1000)` encadenado a cada respuesta, solo mientras Ahora está
a la vista y la página visible, como el Tablero. Antes de la primera
respuesta dice `leyendo el tablero...`, y si la lectura falla,
`no se pudo leer el tablero: <error>`, y reintenta en el próximo ciclo.

El contenido va centrado, a 1120 px como máximo, con 24 px a los lados.
Los bloques, en este orden:

**1. La banda TU SIGUIENTE PASO.** Es un panel `--hd-grafito`, con borde
`--hd-amarillo` cuando `cuenta.te_toca` o `cuenta.detenida` son mayores que
0, y `--hd-borde` cuando no.

- **Arriba**: la etiqueta `TU SIGUIENTE PASO` y, a la derecha,
  `te toca <a> · corriendo <b> · detenida <c>`, siempre con los tres
  números.
- **Con `caja`**, la tarjeta de la caja:
  - su `frase`, a 25 px;
  - su `plain`, en `--hd-gris` (en DETENIDA, el `plain` del rojo o
    `se cortó el trabajo del <rol>`);
  - el botón primario de su paso, con la `etiqueta` del paso, cuando hay
    paso, o su `texto` cuando no;
  - el secundario `Ver la tarjeta`;
  - `después: «<titulo>» · «<titulo>»`, con las tarjetas que siguen en
    TE TOCA y después en DETENIDA, hasta 3 títulos.
- **Sin `caja`**, los textos del tablero v3:
  - con alguna CORRIENDO: `Nada te espera. hoom trabaja en <n> tarjeta(s).`
    y `corriendo: «<titulo>» (<rol>, <tiempo>)`, con ` y <k> más`;
  - sin ninguna corriendo y con alguna EN COLA:
    `Nada en curso. Podés empezar «<titulo>».`, con la primera de EN COLA y
    el botón de su paso;
  - sin ninguna: `No hay nada pendiente.` y
    `para sumar trabajo: hoom item add "<titulo>"` en `--mono`.

**2. Las secciones**, una por grupo con tarjetas, en el orden de
`pelota.orden`. Una sección sin tarjetas no se muestra. Hecho muestra sus
5 primeras en `pelota.orden` y, si tiene más, `y <k> más en el Tablero`
(el tablero no las ordena por fecha, así que no son "las últimas").

| Grupo | Título | Subtítulo |
|---|---|---|
| TE TOCA | `Te toca — <n> decisión(es) tuya(s)` | `hoom no firma ni acepta por vos` |
| CORRIENDO | `En curso — lo que hoom hace ahora` | `<n> tarjeta(s) trabajando` |
| DETENIDA | `Detenidas — hoom paró y dice por qué` | `<n> tarjeta(s)` |
| EN COLA | `En cola — todavía no empezó` | `<n> tarjeta(s)` |
| HECHO | `Hecho — lo que ya aceptaste` | `<n> tarjeta(s)` |

**Una tarjeta de Te toca** es un panel `--hd-grafito` de radio 10 px:

- un badge amarillo con el nombre del paso del camino de su columna en
  MAYÚSCULA (`TU FIRMA`, `TU ACEPTACIÓN`, `SPEC`, `TESTS`, `CÓDIGO`,
  `REVIEW`) y su slug en `--mono`;
- su título, a 18 px;
- su `plain`;
- con `pendientes.texto` no vacío, ese texto;
- con `evidence.verdict` en `green`, `VERDE` en `--hd-verde`, peso 700,
  antes del `plain`;
- el botón primario de su paso, o su `texto` en `--hd-gris`;
- el secundario `Ver la tarjeta`.

**Una fila de En curso** es un panel con borde izquierdo de 3 px
`--hd-verde`, en tres columnas que se apilan en una pantalla angosta:

- **Primera columna.** El título, como enlace a `#tarjeta/<slug>`, el chip
  `riesgo <riesgo>` cuando la evidencia trae riesgo y el chip
  `piloto encendido` cuando el item tiene `auto: hasta-humano`. Debajo, la
  barra del camino: 8 segmentos de 6 px, `--hd-verde` para `hecho`,
  `--hd-azul` para `actual` y `--hd-borde` para `falta`. Debajo de la
  barra, `paso <i> de 8 — <nombre en minúscula>`.
- **AHORA.** La frase de `ahora`, el badge `CORRIENDO` y
  `<provider> · hace <tiempo>[ · tope <limite>]` en `--mono`, con
  `running.started_at` y `running.timeout_s`.
- **SIGUE.** La frase de `sigue` y su `gris`.

**Una tarjeta de Detenidas** lleva:

- el badge `DETENIDA`, su título y su slug;
- su `ahora.frase`;
- una caja hundida (`--hd-carbon-900`, `--mono`) con `ahora.gris` cuando no
  está vacío;
- un botón secundario por cada acción habilitada de la tarjeta, en su orden
  y hasta 3, con su `label`;
- `Ver la tarjeta`.

**Una fila de En cola o de Hecho** es una línea: el título como enlace, su
slug en `--mono` y su `plain` en `--hd-gris`. En Hecho, además,
`aceptada <fecha>` con `item.hecho_en` cuando existe. En cola muestra el
botón de su paso solo en la banda (ver arriba).

**Todo botón de paso o de acción** llama a `abrirAccion(card, action)` de
`acciones.js` con la tarjeta y la acción de `/api/board`: el mismo diálogo
y la misma confirmación que el Tablero. `ahora.js` nunca hace un POST.

### La pantalla Tarjeta

También vive en `ui/ahora.js`. Pide solo `GET /api/board/<slug>` y
`GET /api/board/<slug>/timeline`, con el mismo sondeo de 1 s encadenado.

- Una respuesta 404 muestra `no existe la tarjeta «<slug>»` y el enlace
  `← Ahora`.
- Cualquier otro error muestra `no se pudo leer la tarjeta: <error>` y
  reintenta.

De arriba abajo:

1. **El enlace `← Ahora`** (a `#ahora`).
2. **El título y los chips.** El título de la tarjeta, a 31 px, y su slug
   en `--mono`. Después, `riesgo <riesgo>` cuando hay, `piloto encendido`
   (en `--hd-verde`) con `auto: hasta-humano`, y el badge de su grupo
   (te toca, CORRIENDO o DETENIDA; nada en EN COLA y en HECHO).
3. **`Camino — de tu firma a tu aceptación`.** Las 8 entradas de
   `pelota.camino`:
   - en una grilla de 8 columnas con scroll horizontal debajo de 760 px;
   - cada entrada con su barra de 6 px (`hecho` verde, `actual` azul en
     CORRIENDO, amarillo en TE TOCA y rojo en DETENIDA, y `falta` borde);
   - su `nombre`, en peso 700 y con `aria-current="step"` en la `actual`;
   - su estado en palabras: `listo` para `hecho`, el `texto` para `actual`
     y `falta` para `falta`.
4. **Dos columnas** que se apilan en una pantalla angosta.

**La columna izquierda:**

- **`AHORA — QUÉ ESTÁ HACIENDO`.** Un panel con borde izquierdo de 3 px
  `--hd-verde` cuando la tarjeta tiene `running`. Lleva la frase de
  `ahora` a 25 px y su `gris`. Con `running`:
  - el badge `CORRIENDO` y
    `<provider> · empezó <HH:MM> · hace <tiempo>` en `--mono`;
  - con `timeout_s`, la fila `tiempo de este trabajo` con
    `<min> de <limite> min` y una barra de 6 px `--hd-azul` con la
    proporción, que no pasa de 100 %;
  - `sobre <envelope_id>` en `--mono`.
- **`SIGUE — QUÉ PASA DESPUÉS`.** La frase de `sigue` y su `gris` en
  `--mono`, en `--hd-gris`.
- **`Historia — paso a paso`.** Una fila por entrada de la historia, de la
  más nueva a la más vieja y hasta 20: la hora (`HH:MM` si es de hoy,
  `DD/MM HH:MM` si no) en `--mono`, el `plain` y, en `--mono`, el
  `provider` y la `ref` cuando existen. Sin entradas:
  `Todavía no hay historia para esta tarjeta.`

**La columna derecha:**

- **`Topes — hasta dónde llega solo`.** Solo cuando `spend.budget_usd` es
  mayor que 0: la fila `presupuesto` con
  `<cost_usd> de <budget_usd> USD`, a dos decimales, y su barra
  (`--hd-verde` hasta el 80 % y `--hd-amarillo` desde ahí). Con
  `cost_usd` en `null` (ningún run reportó costo), la fila dice
  `sin dato de costo · tope <budget_usd> USD` y no hay barra. Sin
  presupuesto, este bloque no aparece.
- **`Evidencia — lo último comprobado`.** Con veredicto:
  - `veredicto <id corto> · <VERDE|ROJO>`, con el `verdict` del detalle
    (`green` es VERDE y `red` es ROJO);
  - una fila por gate de `verdict.gates`, con su `name` y su `status` en
    MAYÚSCULA y en `--mono`:
    - `PASS` en `--hd-verde-gate`;
    - `FAIL` y `ERROR` en `--hd-rojo`; con `red.esperado`, un `fail` se
      dice `FAIL esperado`, en `--hd-amarillo`;
    - los demás en `--hd-gris`;
  - `hallazgos que bloquean: <n>`, con la cantidad de ids de
    `evidence.blocking_findings`;
  - `pendientes.texto` tal cual (ya empieza con `pendientes:`), o
    `sin pendientes` cuando está vacío.

  Sin veredicto: `todavía no hay veredicto`.
- **Los botones.** El primario del paso (o su `texto`) y un secundario por
  cada otra acción habilitada de la tarjeta, en su orden. Todos van por
  `abrirAccion`.
- **El piloto.** Con `auto: hasta-humano`, en `--mono`:
  `para apagar el piloto: hoom item set <slug> --auto no`. Sin el piloto,
  en Tu aprobación, Test-writer, Writer o Review, el `sigue.gris` ya dice
  cómo encenderlo.

### Archivos de la UI

La UI embebida son exactamente estos archivos:

- `ui/index.html`, `ui/tablero.js`, `ui/acciones.js` y `ui/ahora.js`;
- los 5 `.ttf` de `ui/fuentes/`.

`index.html` carga `tablero.js`, `acciones.js` y `ahora.js`, en ese orden y
después de su propio script. El CSS nuevo va en el `<style>` de
`index.html`. Ningún archivo embebido, ni los `.ttf`, contiene `http:`,
`https:`, `//cdn` ni `@import`: `TestCA10_UISinRed` sigue pasando sin
tocarlo.

El único test de hoy que cambia es la lista exacta de archivos de
`TestCA319_SinAssetsDeRed` (`tablero_ui_test.go`), porque CA-734 la
re-expresa. Ningún otro test de hoy se toca.

### Cómo se prueba

- **La regla.** Tests de `boardcmd` con fixtures de tarjetas, uno por fila
  de cada tabla de "La regla, en un solo lugar": grupos, pasos, frases,
  `ahora`, `sigue`, camino, `orden`, `caja` y `cuenta`.
- **El JSON.** `/api/board` y `/api/board/{slug}` traen `pelota` (con
  `httptest`, como CA-316 y CA-317). `hoom board --json` y `/api/board`
  siguen dando los mismos bytes.
- **El tablero.** Los tests de hoy de `internal/tablero` pasan sin
  modificar, borrar ni renombrar ningún `_test.go`. Un test nuevo, en un
  archivo nuevo de `internal/tablero`, comprueba que
  para cada tarjeta de los fixtures sin piloto vivo del tablero,
  `Modelo.Grupo(slug)` es igual a `card.Pelota.Grupo`.
- **La UI.** Tests estáticos, como los de hoy:
  - strings y regex sobre `index.html` y `ahora.js`;
  - `ahora.js` no contiene `act(`, `POST`, `method`, `fetch(`,
    `XMLHttpRequest`, `sendBeacon` ni `X-Hoom-Token`;
  - sus literales `/api/` son solo `/api/board` y `/api/board/`;
  - no usa `setInterval`;
  - **no decide grupos**: no contiene los ids de columna (`tu-aprobacion`,
    `tu-aceptacion`, `test-writer`, `backlog`, `hecho`) ni lee `red.esperado`
    ni `running` para elegir sección. El grupo sale de `pelota.grupo`.
- **Las fuentes.** El sha256 de cada `.ttf` embebido es el de la tabla, y
  `GET /fuentes/<archivo>` responde 200.

### Documentación

El README, en la sección del Studio, describe:

- la portada Ahora, con la banda TU SIGUIENTE PASO y los grupos;
- la pantalla Tarjeta y sus hashes;
- el campo `pelota` de `hoom board --json`;
- que las fuentes van embebidas, con su licencia.

## Casos límite y errores esperados

- **Un tablero sin tarjetas.** La banda dice `No hay nada pendiente.` y no
  hay secciones. `pelota.orden` es `[]` y `cuenta` tiene sus cinco ceros.
- **Un item inválido** (en `warnings`) no es tarjeta: no está en `orden` ni
  en ninguna sección. Los `warnings` se muestran como hoy en el Tablero; en
  Ahora, como una línea en `--hd-naranja` arriba de la banda:
  `<n> item(s) no se pudieron leer: mirá el Tablero`.
- **Una tarjeta con rojo esperado** está en TE TOCA (si no corre nada).
  Su AHORA dice `Los tests están escritos y fallan, como se espera.` y su
  gate `test` dice `FAIL esperado`.
- **Una tarjeta sin título** usa su slug en la frase, en la sección y en el
  detalle.
- **Un título largo** se corta con `…` en la banda (`después:`) y en las
  filas de una línea. En las tarjetas se parte en varias líneas.
- **Un paso deshabilitado** no tiene botón: se muestra su `texto` en
  `--hd-gris`, igual que un paso que no hay.
- **Sin token de acciones**, el diálogo hace lo de hoy: el POST responde
  401 y el toast lo dice. Ahora y Tarjeta no piden el token por su cuenta.
- **El hash `#tarjeta/` de un slug que existía y ya no** (se borró el
  item) da 404: `no existe la tarjeta «<slug>»`.
- **Un `running` sin `timeout_s`** no tiene fila de tiempo ni ` · tope`.
- **Un `running` con `started_at` en el futuro** (reloj corrido) dice
  `hace 0 min`.
- **Una tarjeta que un piloto de `hoom tablero` lleva entre dos trabajos**
  aparece en el Studio donde la pone su estado en disco (TE TOCA o
  DETENIDA), mientras el tablero que la lanzó la muestra en CORRIENDO. Ver
  "Riesgos".
- **Sin fuentes** (un navegador que bloquea fuentes locales): la página
  cae en las familias de respaldo y se sigue leyendo.
- **Una página muy angosta** (menos de 720 px): las columnas de En curso y
  de Tarjeta se apilan; el camino de Tarjeta hace scroll horizontal.

## Criterios de aceptación

- CA-722: `boardcmd.Card` tiene `pelota` en `hoom board --json`, en `/api/board`, en la `card` de `/api/board/{slug}` y en `hoom item show <slug> --json`, nunca `null`, con `grupo`, `paso` (`accion`, `etiqueta`, `texto`), `frase`, `ahora` (`frase`, `gris`), `sigue` (`frase`, `gris`) y `camino`. `/api/board` sigue dando los mismos bytes que `hoom board --json`.
- CA-723: `pelota.grupo` sigue la tabla de grupos del contrato, con un fixture por fila: Hecho, `running`, `interrupted`, rojo no esperado, Backlog y una tarjeta de TE TOCA. Una tarjeta con `red.esperado` y sin `running` está en `TE TOCA`, no en `DETENIDA`.
- CA-724: `pelota.paso` sigue las 6 reglas del contrato con sus textos exactos, nunca es Seguir solo, `accion` es el id de una acción habilitada de la tarjeta o `""`, y sin paso `texto` dice por qué: la acción que falta o el `why` de la deshabilitada.
- CA-725: `pelota.frase` es la de la tabla de frases del contrato para cada grupo y cada paso, con el slug cuando el título está vacío, y es `""` en CORRIENDO, EN COLA y HECHO.
- CA-726: `pelota.ahora` y `pelota.sigue` dan la frase y la línea gris de AHORA y de SIGUE del tablero v3 para cada caso de su tabla, sin los del piloto del tablero, con `ya no corre` para `interrupted` y `red.reason` para un rojo no esperado. Ningún texto de `pelota` contiene el nombre de una tecla del tablero (`R `, `enter`, ` n `, `P `).
- CA-727: `pelota.camino` tiene siempre 8 entradas con los nombres del contrato en orden, `estado` `hecho`, `actual` o `falta` según la columna de la tarjeta (las 8 `hecho` en Hecho), y `texto` solo en la `actual`, con el texto de su grupo.
- CA-728: `Board.pelota` trae `orden` (todas las tarjetas, en el orden del tablero v3), `caja` (la primera de TE TOCA, si no la primera de DETENIDA, si no `""`) y `cuenta` con sus cinco claves siempre presentes; un tablero vacío da `orden: []`, `caja: ""` y cinco ceros. Los `warnings` no entran en `orden`.
- CA-729: `internal/tablero` toma de `boardcmd` el grupo, el paso, las frases de la caja, de AHORA, de SIGUE y del camino, y el orden de la lista (`OrdenPelota`), y solo agrega el piloto que lanzó, Seguir solo, su parada, sus textos con teclas, el modo experto y el ASCII. Sus tests de hoy pasan sin tocarlos: ninguno se modifica, se borra ni se renombra (sí se suman archivos nuevos, como el de CA-730). [verifica: git diff --quiet --no-renames --diff-filter=MD main -- 'internal/tablero/*_test.go']
- CA-730: para cada tarjeta de los fixtures del tablero sin un piloto vivo lanzado por él, `Modelo.Grupo(slug)` es igual a su `pelota.grupo`, y `Modelo.Caja()` es igual a `Board.pelota.caja`.
- CA-731: `index.html` define en `:root` las 14 variables `--hd-*` con los valores de la tabla, y `--surface`, `--panel`, `--panel-2`, `--border`, `--text-2` y `--text-3` con sus valores nuevos; `--text`, `--good`, `--warning`, `--serious`, `--critical`, `--accent` y `--human` no cambian. El `body` usa la familia del núcleo y `--mono` empieza con `"JetBrains Mono"`.
- CA-732: los 5 `.ttf` de `internal/servecmd/ui/fuentes/` tienen exactamente los sha256 de la tabla, `GET /fuentes/<archivo>` responde 200 para cada uno, `index.html` los declara con `@font-face` y `url("fuentes/<archivo>")`, y `.hoom/intake/studio-tu-siguiente-paso/fuentes/` ya no tiene los `.ttf`.
- CA-733: `internal/servecmd/licencias/OFL-SpaceGrotesk.txt` y `internal/servecmd/licencias/OFL-JetBrainsMono.txt` contienen la línea de copyright de su familia, tal como está en el contrato, y el texto de la SIL Open Font License, Version 1.1 (`SIL OPEN FONT LICENSE Version 1.1` y sus cinco condiciones).
- CA-734: (re-expresa CA-319) `index.html` tiene las pestañas Ahora, Tablero y Cockpit, en ese orden, con Ahora activa al cargar, y carga `tablero.js`, `acciones.js` y `ahora.js`, en ese orden, después de su script; los tres se sirven embebidos (200). La UI embebida son exactamente `index.html`, `tablero.js`, `acciones.js`, `ahora.js` y los 5 `.ttf` de `fuentes/`, y ningún archivo embebido contiene `http:`, `https:`, `//cdn` ni `@import`. `index.html` define `--human`, y `tablero.js` sigue cumpliendo lo demás de CA-319.
- CA-735: el hash elige la pantalla según la tabla del contrato (vacío y `#ahora` Ahora, `#tablero`, `#cockpit`, `#tarjeta/<slug>`, cualquier otro Ahora); cambiar de pestaña escribe el hash, `hashchange` cambia de pestaña y `mostrarPestana` es la única que muestra y oculta las pestañas.
- CA-736: `ahora.js` no contiene `act(`, `POST`, `method`, `fetch(`, `XMLHttpRequest`, `sendBeacon`, `X-Hoom-Token` ni `setInterval`; sus literales `/api/` son solo `/api/board` y `/api/board/` (más `/timeline`), y sondea con `setTimeout(…, 1000)` encadenado, solo con la página visible y su pantalla a la vista.
- CA-737: `ahora.js` no decide grupos: no contiene los ids de columna `tu-aprobacion`, `tu-aceptacion`, `test-writer`, `backlog` ni `hecho`, no lee `esperado`, y elige la sección de cada tarjeta por `pelota.grupo` y su orden por `pelota.orden`.
- CA-738: la banda TU SIGUIENTE PASO lleva `TU SIGUIENTE PASO`, `te toca`, `corriendo` y `detenida` con sus números, la `frase` de la tarjeta de `caja`, `Ver la tarjeta` y `después:`, y sin `caja` los tres textos del contrato: `Nada te espera. hoom trabaja en`, `Nada en curso. Podés empezar` y `No hay nada pendiente.`.
- CA-739: Ahora tiene los cinco títulos de sección del contrato (`Te toca —`, `En curso — lo que hoom hace ahora`, `Detenidas — hoom paró y dice por qué`, `En cola — todavía no empezó` y `Hecho — lo que ya aceptaste`) y `hoom no firma ni acepta por vos`, y el pie `acá ves lo que hoom comprobó, no lo que el modelo cuenta`.
- CA-740: cada botón de paso o de acción de Ahora y de Tarjeta llama a `abrirAccion` con la tarjeta y la acción del JSON, y `ahora.js` no define su propio diálogo ni su propia confirmación.
- CA-741: Tarjeta tiene `← Ahora`, `Camino — de tu firma a tu aceptación` con `listo`, `falta` y `aria-current="step"`, `AHORA — QUÉ ESTÁ HACIENDO`, `SIGUE — QUÉ PASA DESPUÉS`, `Historia — paso a paso`, `Topes — hasta dónde llega solo`, `Evidencia — lo último comprobado`, `FAIL esperado`, `hallazgos que bloquean:`, `para apagar el piloto: hoom item set` y `Todavía no hay historia para esta tarjeta.`.
- CA-742: Tarjeta muestra `no existe la tarjeta «` ante un 404 de `/api/board/{slug}` y `no se pudo leer la tarjeta:` ante otro error; Ahora muestra `leyendo el tablero...` antes de la primera respuesta y `no se pudo leer el tablero:` ante un error.
- CA-743: la vista Tablero de hoy sigue igual: los tests de CA-10, CA-320, CA-321, CA-322, CA-353 y CA-366 pasan, en `tablero_ui_test.go` solo cambia la lista de archivos de `TestCA319_SinAssetsDeRed`, y `tablero.js` no cambia salvo lo que exige `mostrarPestana` para sumar Ahora y Tarjeta. [verifica: git diff --quiet main -- internal/servecmd/servecmd_test.go internal/servecmd/acciones_ui_test.go internal/servecmd/historia_ui_test.go]
- CA-744: sin dependencias nuevas: `go.mod` y `go.sum` no cambian. [verifica: git diff --quiet main -- go.mod go.sum]
- CA-745: dogfood: hoomai tiene el item de esta spec. [verifica: go run ./cmd/hoom item show studio-tu-siguiente-paso --json | grep -q '"slug": "studio-tu-siguiente-paso"']
- CA-746: el README documenta la portada Ahora, la banda TU SIGUIENTE PASO, la pantalla Tarjeta y el campo `pelota`. [verifica: grep -q "TU SIGUIENTE PASO" README.md && grep -q "#tarjeta/" README.md && grep -q '"pelota"' README.md]

## Decisiones

Todas son propuestas del agente; Henry eligió solo el alcance, la portada y
la línea gráfica.

1. **La regla se muda a `boardcmd`** (CA-729). Alternativa: que
   `ahora.js` la copie. Se descartó porque el Studio y el tablero podrían
   decir cosas distintas sobre la misma tarjeta, y porque el Studio no
   tiene lógica que el CLI no tenga. Es lo más caro del spec: toca el
   tablero recién integrado. Lo protegen sus tests sin cambios y CA-730.
2. **También se mudan las frases**, no solo los grupos. Así `ahora.js` es
   un pintor sin lógica (CA-737) y la frase de la banda es la misma que la
   de la caja de la terminal.
3. **Los textos compartidos no nombran teclas**: el tablero conserva los
   suyos con `R`, y el Studio pone botones.
4. **Seguir solo queda fuera del Studio.** El Studio no tiene ruta para
   `hoom piloto`, y sumarla es una acción nueva (fuera del alcance). En el
   Studio, una tarjeta donde la terminal diría Seguir solo muestra el paso
   siguiente de la regla (por ejemplo, pedir el código al writer).
5. **La banda TU SIGUIENTE PASO se suma aunque el lienzo no la tenga.** El
   lienzo es anterior al tablero v3; la banda contesta "qué hago ahora" en
   el mismo lugar en las dos pieles.
6. **Se suma la sección En cola**, que el lienzo no tiene, para que ninguna
   tarjeta quede fuera de Ahora.
7. **Hecho muestra 5 tarjetas** y manda el resto al Tablero. Son las 5
   primeras del orden del tablero, no las más recientes: ordenar por fecha
   cambiaría el orden de HECHO en la terminal.
8. **Las vistas viejas toman la paleta nueva** cambiando el valor de sus
   variables, sin tocar su CSS. Alternativa: dejarlas con la paleta de hoy
   hasta rediseñarlas, con dos looks en la misma página.
9. **Las fuentes van embebidas** (unos 495 KB más en el binario) en vez de
   usar solo las del sistema, que no se parecen al lienzo. Se traen en el
   PR del spec (en intake) para que la tarea no dependa de la red y el
   sha256 sea verificable.
10. **TTF y no WOFF2.** Convertirlas pide una herramienta que el repo no
    tiene; los navegadores cargan TTF.
11. **Sin tema claro** en esta versión.
12. **El estado del camino en palabras** es `listo`, el `texto` del grupo y
    `falta`, en vez de las del lienzo ("firmaste 09:10", "te va a tocar"),
    que piden datos que la tarjeta no trae.
13. **Los badges de Te toca dicen el paso del camino** (`TU FIRMA`,
    `TU ACEPTACIÓN`, `TESTS`…), que es lo que el lienzo hace con las dos
    columnas humanas.

## Riesgos y deuda aceptada

- **Tocar el tablero v3.** La mudanza de la regla puede romper algo que
  sus tests no miran. Mitigación: sus `_test.go` quedan intactos (CA-729)
  y CA-730 compara las dos pieles.
- **El piloto de un tablero no se ve en el Studio.** Entre dos trabajos, el
  Studio pone la tarjeta en TE TOCA o DETENIDA mientras el tablero que la
  lanzó la muestra en CORRIENDO. Mostrarlo pide leer el candado del piloto,
  que `hoom piloto` toma con espera 0 (ver la decisión del tablero v3). Si
  molesta, un spec propio puede publicar el estado del piloto en disco.
- **La UI se prueba sin navegador.** Los tests son estáticos: ven que los
  textos y las llamadas estén, no que la pantalla se vea como el lienzo. La
  aceptación de Henry, mirándola, es la prueba visual.
- **El binario crece unos 495 KB** por las fuentes.
- **Dos looks durante un tiempo.** Las vistas viejas toman la paleta, pero
  no la disposición ni el microcopy del lienzo.
- **La review va a ser grande.** El diff toca `boardcmd`, `tablero` y
  `servecmd/ui`, y probablemente pase el tope de evidencia (576 KiB): se
  parte en reviews apiladas, como el tablero v3, sin subir el tope.
- **Las licencias OFL** las escribe la tarea con el texto oficial. CA-733
  mira que estén la línea de copyright y la licencia, no cada palabra.
- **El aviso de la licencia viaja en las fuentes, no al lado.** El binario
  lleva el copyright y el aviso de la OFL dentro de cada `.ttf`, que la OFL
  acepta. El texto entero queda en el repo, fuera de `ui/`, por CA-10. Si
  algún día se quiere servir el texto, pide cambiar CA-10.
