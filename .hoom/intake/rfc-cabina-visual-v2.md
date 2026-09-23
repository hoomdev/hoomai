# Cabina visual (Studio v5) — documento fuente v2

Fecha: 2026-09-22.
Tipo: documento fuente (intake). No es una spec y no tiene criterios de
aceptación: de acá salen cuatro specs (C1 a C4), y cada una escribe los suyos
con la numeración global de siempre.
Origen: `RFC_Cabina_Visual_hoomAI.md` (Equipo hoomAI / Oceans Bits,
2026-09-12) y la sección "Cabina visual (Studio v5)" del roadmap del README
(commit 0398fa4).
Qué cambia respecto del RFC v1: se conserva la visión y se corrige el
mecanismo, para que respete la filosofía de hoom.

## En una frase

La cabina es un tablero tipo Kanban dentro de `hoom serve`, donde cada columna
es un paso del método de hoom y **la tarjeta no se mueve, se gana el
movimiento**: su columna es una función de la evidencia que hay en disco y en
git, nunca un estado que alguien escribe al arrastrarla.

## Por qué corregir el RFC v1

El RFC v1 acierta en el qué: un tablero donde se ve al equipo de agentes
trabajar, con trazabilidad de la idea al merge, local-first y sincronizado por
git. Falla en el cómo, porque su motor contradice la regla que hoom ya cumple
en todo lo demás: la narración no cuenta, solo cuenta la evidencia.

- En el RFC v1 una tarjeta está en Review porque un evento `task.moved` dice
  `"to": "review"`. Ese evento lo escribe quien arrastra y no prueba nada sobre
  el código. Un tablero así puede mostrar una tarjeta en "Ready to Merge" con
  un veredicto rojo al lado.
- Los veredictos por rol (`analyst.verdict.json`, `programmer.verdict.json`)
  le devuelven al modelo el poder de juzgarse a sí mismo. En hoom el veredicto
  lo emiten gates deterministas sobre una huella.
- El log de eventos JSONL y la base SQLite duplican lo que git ya es: un log
  append-only, con autor y fecha, replicado entre computadoras.

Punto por punto:

| RFC v1 | v2 | Por qué |
|---|---|---|
| La columna es un estado que se escribe con `task.moved` | La columna es una función de la evidencia | Un estado escrito es narración. La evidencia se puede verificar |
| Diez columnas: Análisis, Arquitectura, Ready for Dev, Programación, Code Review, QA, Verification Gate, Ready to Merge, ... | Ocho: Backlog, Arquitecto, Tu aprobación, Test-writer, Writer, Review, Tu aceptación, Hecho | Ver "Las ocho columnas" |
| QA escribe los tests después del review | El test-writer escribe antes que el writer, desde el spec y en árbol ciego | Anti-circularidad: quien escribe los tests no vio la implementación (Spec D) |
| Rol Verifier y columna Verification Gate | `hoom verify`, que corre en cada transición | `verify` no es un agente: es un comando determinista |
| Veredictos por rol | Los roles dejan artefactos y el veredicto lo emiten los gates | El modelo no se juzga a sí mismo |
| `.hoomai/events/*.jsonl` y SQLite reconstruible | Proyección en memoria desde `.hoom/` y git | git ya es el log de eventos |
| `auto_run: true` por columna | Cada arrastre se confirma. El piloto automático es opt-in y llega al final | Cada corrida gasta plata y toca código: tiene que ser un acto consciente |
| Estado `conflict` cuando dos computadoras mueven la misma tarjeta | No existe | Nadie escribe columnas, así que no hay movimientos que choquen |
| Aprobación con `"actor": "hoom"` | Firma con la identidad git de la persona, atada al hash del contenido | Es lo que ya hace `hoom spec approve` |
| Columnas configurables (`columns.yaml`, `policies.yaml`) | Columnas fijas en el binario | El orden es el método. Si fuera configurable, la anti-circularidad pasaría a ser opcional |
| `.hoomai/`, `hoomai cockpit` | `.hoom/`, la cabina dentro de `hoom serve`, `hoom cockpit` sin cambios | Son los nombres del producto |
| "task" = la tarjeta | "item" = la tarjeta. "task" sigue siendo el espacio de trabajo | `hoom task` ya existe y significa worktree |
| Servidor de sync, multiusuario, WebSocket, Jira/Linear (fase 6) | Fuera | git es el transporte. No hay servidor |

Se conserva: la tarjeta como contenedor vivo de contexto, un worktree por
tarea, la trazabilidad spec → tests → código → merge, la sincronización por
git, la base local reconstruible (llevada al extremo: no hay base), y las
reglas de calidad del RFC v1, que hoom ya hace cumplir desde el binario.

## Para quién

Henry, el equipo y juniors. Un solo tablero con dos niveles, que se cambian
con un interruptor:

- **Modo normal**: sin git, sin diff, sin rutas. Vocabulario simple: "espacio
  de trabajo" en vez de worktree, "integrar" en vez de merge, "guardar" en vez
  de commit. Un junior ve la tarjeta, su medidor de evidencia, quién está
  trabajando y, si algo está rojo, por qué, en una frase.
- **Modo experto**: el diff, las rutas de cada artefacto, los ids de
  veredictos y hallazgos, abrir una sesión interactiva en el espacio de trabajo
  de la tarjeta y ver su terminal.

Por qué un tablero y no dos: el junior y el experto tienen que poder hablar de
la misma tarjeta. Los dos niveles leen la misma proyección, y el modo normal
no esconde un rojo: lo dice con otras palabras.

## La regla: la tarjeta no se mueve, se gana el movimiento

### La columna es una función

`columna(item)` = la última columna, en orden, cuya evidencia existe y sigue
valiendo.

Cada columna exige su evidencia y la de todas las anteriores. La función no
tiene memoria: la misma evidencia da la misma columna en cualquier computadora,
recién arrancado el Studio o después de un apagón. Como no hay nada guardado,
no hay nada que se pueda corromper ni reconstruir.

### Las ocho columnas

| # | Columna | Quién la gana | Evidencia que la gana | Qué corre detrás del arrastre |
|---|---|---|---|---|
| 1 | Backlog | el analista o una persona | `.hoom/items/<slug>.json` existe | `hoom item add` (verbo nuevo, C1) |
| 2 | Arquitecto | rol arquitecto | `.hoom/specs/<slug>.md` existe y pasa `spec_lint` | `hoom task start <slug>` si no hay espacio de trabajo, y `hoom agent --role arquitecto --task <slug>` con el pedido del item |
| 3 | Tu aprobación | la persona | una aprobación en `.hoom/approvals/` con el SHA-256 del spec actual (`hoom spec status` = aprobado) | `hoom spec approve .hoom/specs/<slug>.md` |
| 4 | Test-writer | rol test-writer, en árbol ciego | cada criterio del spec citado por al menos un test (`spec_trace`). Ver brecha 1 | `hoom agent --role test-writer --task <slug> --spec ...` |
| 5 | Writer | rol writer | un veredicto completo y verde con `spec: .hoom/specs/<slug>.md`, y `hoom check` verde en el espacio de trabajo | `hoom agent --role writer --task <slug> --spec ...` |
| 6 | Review | review cruzada | un registro de la review (brecha 2) y cero hallazgos de la tarea que bloqueen, según el gate `findings_open` de un veredicto con huella vigente | `hoom review --task <slug> --spec ...` |
| 7 | Tu aceptación | la persona | una aceptación firmada con su identidad git y atada a la huella del veredicto vigente (brecha 3) | verbo nuevo (C1), botón en C3 |
| 8 | Hecho | la persona, al integrar | la rama `hoom/<slug>` está integrada en la rama base | `hoom task done <slug>` y `git merge --no-ff hoom/<slug>` |

Las columnas 3 y 7 son las humanas: explícitas y moradas. Son los dos puntos
de control del método (aprobar el spec antes de que existan tests, aceptar el
resultado antes de integrarlo), y como columnas se ve cuándo una tarjeta está
esperando a una persona. Como un badge, esa espera quedaría escondida. Ningún
agente puede ganarlas: el piso no aflojable del sobre ya le prohíbe a un rol
tocar `.hoom/approvals/`, y la aceptación entra en ese mismo piso.

La salida de Review la impone `verify`, no el tablero. Esa decisión quedó
escrita en B2. Un proyecto sin `findings.block_on` en `hoom.yaml` no puede
ganar Review, y la tarjeta lo dice con la línea exacta que falta.

Hecho es terminal. Una tarjeta integrada se evalúa sobre lo que se integró (la
punta de su rama), no sobre la rama base de hoy: lo que pase después en main
es de otras tarjetas.

### Lo que no es columna

- **Verificación** es el gate que corre en cada transición. Todo arrastre a
  una columna de rol termina con el sobre (scope, verify, check). Todo arrastre
  a una columna humana pasa por la condición de su verbo: `spec approve` firma
  un contenido exacto, la aceptación exige check verde, `task done` exige
  verde, huella y todo commiteado. Si Verificación fuera una columna, en las
  demás habría tarjetas "avanzadas" que nadie verificó.
- **Analista** no es columna: alimenta el Backlog desde `.hoom/intake/`. Lo
  que entrega son items nuevos, no el avance de uno.
- **Orquestador** no es columna: en la cabina, el que rutea sos vos, arrastrando.
- **Scout, designer y characterizer** son ayudantes que se invocan desde
  adentro de una columna: el scout y el designer desde Arquitecto (contexto y
  UI-spec), y el characterizer antes de que el writer toque código legacy sin
  tests. Su trabajo alimenta al rol de la columna y no gana ninguna.
- **Refutador y correcciones del writer** son subestados de Review: revisando,
  con hallazgos abiertos, refutando, corrigiendo, re-verificando. Adentro de
  Review las acciones son botones de la tarjeta, no arrastres. La tarjeta no
  vuelve a Writer para corregir: se queda en Review con su subestado, y no hay
  flechas hacia atrás. El refutador conserva su contrato: dos ciclos como
  máximo y después escala a la persona.

### Arrastrar es pedirle al rol que trabaje

1. Solo se puede soltar en la columna siguiente. Las demás se atenúan y dicen
   qué evidencia les falta.
2. Soltar en una columna de rol abre una confirmación con el rol, el provider
   (uno de los CLIs instalados) y el presupuesto en USD. Nada corre sin ese sí.
3. Mientras el rol trabaja, la tarjeta sigue sólida en su columna y aparece un
   **fantasma** en la columna destino, con el paso del sobre en el que va.
4. El fantasma se materializa solo cuando existe la evidencia, calculada por
   la misma función. No alcanza con que el sobre cierre ENTREGABLE: si el
   arquitecto entregó un spec que no pasa `spec_lint`, la tarjeta no se
   materializa en Arquitecto.
5. Si el rol falla, el fantasma desaparece y la tarjeta queda donde estaba,
   con un rojo y el motivo en una frase, sacado del registro del sobre: "el
   sobre cortó en scope: el test-writer escribió fuera de tests", "veredicto
   rojo: falló el gate test", "sin entrega: el rol no dejó ningún archivo".

El fantasma es telemetría (el registro del sobre en `.hoom/envelopes/`). La
tarjeta es evidencia. En otra computadora no hay fantasma.

Soltar en una columna humana no lanza ningún rol. Abre lo que hay que firmar
(el spec renderizado con sus criterios, o la huella con su veredicto), y
firmar es el botón.

### La tarjeta retrocede sola, y dice por qué

Como la columna es una función, cuando una evidencia deja de valer la tarjeta
vuelve sola a la última columna que sigue ganada, sin que nadie la arrastre:

- Si alguien edita el spec después de aprobarlo, la aprobación se invalida y la
  tarjeta vuelve a Arquitecto: "el spec cambió después de tu aprobación".
- Si el código cambia después del veredicto, la huella se rompe y la tarjeta
  vuelve a Test-writer: "el código cambió después del último verde". Volver a
  verificar es una acción de cualquier tarjeta. No hace falta una columna.
- Un hallazgo que bloquea, registrado después del verde, no hace retroceder la
  tarjeta. Se cuenta en el próximo verify, como fijó B2, y hasta entonces la
  tarjeta lo muestra en amarillo.

Es la única flecha hacia atrás, y no la dibuja una persona: la dibuja la
evidencia.

## De dónde sale el estado

### Sin SQLite y sin log de eventos propio

El tablero es una proyección en memoria. Se calcula al arrancar `hoom serve`,
a partir de `.hoom/` y el historial de git, se recalcula cuando cambia su
evidencia y no se guarda en ningún lado. git ya es el log de eventos:
append-only, con autor y fecha, replicado entre computadoras y con su propio
merge. Un log propio sería una segunda verdad capaz de contradecir a la
primera, y una base local sería una tercera.

Las cifras de hoy no piden más. Este repo tiene 47 veredictos, 33 hallazgos
con 31 resoluciones y 14 aprobaciones, y leer todo eso al arrancar es trivial.
El item "cache SQLite regenerable en `.hoom/cache/`" del roadmap sigue
disponible para historiales grandes, como cache y nunca como fuente. La
cabina no lo necesita para nacer.

### Un archivo por item

`.hoom/items/<slug>.json`, uno por item, con el mismo patrón que
`.hoom/findings/`: dos personas que agregan items en dos computadoras nunca
chocan en git, porque nunca escriben el mismo archivo.

El archivo guarda lo que decide una persona o el analista: el slug (que es el
nombre del archivo), el título, el pedido que va a recibir el arquitecto, la
fuente (documento del intake y sección), las dependencias con otros items, el
presupuesto de la tarjeta si lo tiene, y quién lo creó y cuándo.

Nunca guarda una columna, un estado, un porcentaje ni un responsable. Si un
campo del item pudiera mover la tarjeta, arrastrar volvería a ser escribir
estado.

### El slug es la llave

La cabina no inventa relaciones. Las lee de convenciones que hoom ya tiene:

| Evidencia | Cómo se ata al item `<slug>` |
|---|---|
| Spec | `.hoom/specs/<slug>.md` |
| Aprobación | `.hoom/approvals/<slug>_<hash8>.json`, con el SHA-256 del spec |
| Espacio de trabajo | rama `hoom/<slug>` y worktree `.hoom/worktrees/<slug>` (`hoom task`) |
| Veredicto | su campo `spec` |
| Hallazgos | su campo `task` (B1: `HOOM_TASK`), contados con la tarea del spec (B2) |
| Runs y sobres | el campo `task` del sidecar y del registro del sobre |
| Integración | la rama `hoom/<slug>` alcanzada desde la rama base |

Para cada item, la cabina lee la evidencia de su espacio de trabajo si existe
(commiteada o no). Si no existe, la lee de la rama `hoom/<slug>`, y si tampoco
existe, de la rama base.

### Evidencia y telemetría

| Dato | Dónde vive | Viaja por git | Mueve la tarjeta |
|---|---|---|---|
| Item | `.hoom/items/` | sí | la crea |
| Spec y aprobación | `.hoom/specs/`, `.hoom/approvals/` | sí | sí |
| Tests que citan los criterios | el código del proyecto | sí | sí |
| Veredictos | `.hoom/verdicts/` | sí | sí |
| Hallazgos y resoluciones | `.hoom/findings/` | sí | sí (Review) |
| Registro de review y aceptación | nuevos (C1) | sí | sí |
| Integración | git | sí | sí (Hecho) |
| Espacio de trabajo | `.hoom/worktrees/<slug>` | la rama sí, el directorio no | no: lo muestra el medidor |
| Sobres | `.hoom/envelopes/` | no | no: pintan el fantasma y el "interrumpido" |
| Runs: narración, sesión, PID, gasto | `.hoom/runs/` | no | no: alimentan el en vivo, el reanudar y el gasto |
| Trinquete | `.hoom/ratchet.json` | sí | no: su curva va en el tablero (C2) |

La telemetría (runs y sobres) queda local, como fijó la Spec C para el
sidecar del run y antes el Studio v3 para la narración. La evidencia viaja y
la narración no.

### Conflictos

El RFC v1 necesitaba un estado `conflict` porque dos computadoras podían
escribir dos movimientos distintos. En v2 nadie escribe movimientos. Sí puede
pasar que dos computadoras produzcan evidencia sobre el mismo item: eso es un
conflicto de git común (código en la rama de la tarea), se resuelve en git, y
después la columna se recalcula. La evidencia de hoom es un archivo por
artefacto con nombre único, así que no choca. El único choque posible es uno
real: dos personas que crean el mismo slug.

## Roles, gates y veredictos

- No existen veredictos por rol. El veredicto lo emiten los gates sobre una
  huella, con `hoom verify`. Los roles dejan artefactos: el arquitecto un
  spec, el test-writer tests que citan los criterios, el writer código que
  pone verde el veredicto, el reviewer hallazgos y el refutador resoluciones
  con evidencia.
- No existe un rol Verifier. Es `hoom verify`, un comando.
- Cada rol corre bajo el sobre determinista de `hoom agent` (y la review bajo
  `hoom review`), exactamente como desde la CLI: el mismo contrato, el mismo
  límite de escritura, el mismo gate de scope y el mismo árbol ciego para el
  test-writer. La cabina no tiene un camino propio para lanzar roles.

## Solo CLIs instalados

La cabina no llama a ninguna API de IA ni guarda credenciales. Lanza los CLIs
que `hoom providers` encuentra en el PATH, y cada persona entra a su CLI con su
cuenta, su configuración y sus subagentes. hoom nunca habla con un modelo, y
la cabina tampoco.

El presupuesto usa lo que cada CLI soporta. Claude Code acepta
`--max-budget-usd` y reporta el costo. Codex (0.151.0, la versión verificada)
no tiene tope ni reporta costo: con Codex, la confirmación lo avisa y la
tarjeta muestra tokens, nunca un 0 en USD.

## Sin auto_run

Cada arrastre es consciente y confirma rol, provider y presupuesto en USD. Un
tablero que lanza agentes con solo entrar a una columna gasta plata y toca
código sin que nadie lo haya decidido.

El **piloto automático por tarjeta** existe, pero es opt-in y llega al final,
después de C4. Es una cinta tonta: sigue el orden fijo de la tabla, no tiene
juicio, no reintenta, y se detiene ante cualquier rojo y en las dos columnas
humanas. Usa el rol, el provider y el presupuesto que una persona confirmó al
activarlo, y no gasta más que el presupuesto de la tarjeta. Llega al final
porque una cinta sin juicio solo es segura cuando cada columna ya se gana por
evidencia, y eso recién está probado cuando C1 a C4 están en uso.

## Las columnas humanas firman con identidad git

Una aprobación humana nunca tiene actor hoom. El aprobador es la identidad git
de la persona, atada al hash del contenido, como ya hace `hoom spec approve`.
La aceptación tiene la misma forma, atada a la huella. Si se edita lo
aprobado, la firma deja de valer.

La firma toma la identidad git de la computadora donde corre `hoom serve`. Por
eso el Studio sigue escuchando en loopback por defecto: firma quien está
sentado frente a esa computadora. Exponerlo con `--addr` sigue siendo una
decisión consciente, porque firmar desde otra máquina con la identidad de esta
es justo el fraude que la firma existe para impedir.

## En vivo

- **Sobres headless**: la tarjeta muestra la narración normalizada del
  stream, la misma que ya pintan el Escenario y el Feed del Studio v4,
  filtrada a su run. No hay ninguna fuente de datos nueva.
- **Sesiones interactivas**: el pane de tmux se espeja en el navegador, al
  principio solo de lectura. La sesión es la que abre
  `hoom cockpit --task <slug>` (`hoom-<proyecto>-<slug>`). hoom sigue sin
  emular terminales: la emulación la hace tmux, y el Studio muestra lo que tmux
  ya pintó.

Esto revisa un no-goal permanente del Studio v1 a v4 ("terminal embebida"). La
razón: un junior tiene que poder ver qué hace el agente sin aprender tmux, y el
experto ya tiene la terminal. Es solo de lectura al principio porque escribir
en el pane desde el navegador sí sería una terminal embebida, con otro modelo
de riesgo: el token del Studio pasaría a dar un shell. Esa decisión le toca a
otra spec.

## Apagón y cambio de computadora

**Apagón.** Al volver, la columna se recalcula: es una función y no hay nada
que recuperar. Lo que sí queda a medias es un sobre interrumpido:

- Queda etiquetado como huérfano. hoom nunca cierra lo que no vio cerrar: el
  registro del sobre sigue "en curso" y sin latido (Spec E), y el sidecar del
  run que lanzó tiene el PID del proceso dueño (PR 9), que ya no vive.
- La tarjeta muestra "interrumpido en el paso X de N" con dos botones.
  **Reanudar** lanza un sobre NUEVO con `--resume` y el id de sesión del
  provider, y el registro viejo queda como está, porque dice la verdad sobre lo
  que pasó. **Volver a lanzar** empieza de cero. Si el sobre murió antes del
  paso run, no hay sesión que reanudar y solo queda volver a lanzar.
- Solo se puede reanudar en la computadora donde corrió el sobre, porque la
  sesión del provider vive en esa máquina.
- Un test-writer interrumpido deja su cuarentena en `.hoom/isolated/`, y el
  sobre se niega a abrir otra encima. La tarjeta lo avisa y ofrece
  descartarla: nada de la cuarentena llegó al árbol real, porque solo se
  trasplanta lo que aprobó el gate.

**Cambio de computadora.** Viaja lo commiteado y nada más. Una tarjeta con
evidencia sin commitear o sin publicar muestra un ícono de "sin sincronizar",
y cada tarjeta tiene un botón de guardar en git: hace commit de lo pendiente de
esa tarjeta en su espacio de trabajo, y push de su rama si hay remoto. En la
otra computadora, después de un pull, la tarjeta aparece en la columna que le
da su evidencia commiteada, sin fantasma y sin el gasto de los runs de la
primera, que es telemetría.

## Diferenciadores visuales

- **Medidor de evidencia en vez de barra de progreso.** Una barra de progreso
  es una predicción ("80%"), o sea narración. El medidor enciende un segmento
  por cada evidencia que existe y vale: spec, aprobación, tests con criterios
  (x de y), veredicto con huella, hallazgos abiertos, espacio de trabajo,
  integrado. Nunca muestra algo que no se pueda abrir y verificar.
- **Presupuesto y gasto por tarjeta, en USD.** El gasto es la suma del `Usage`
  de los runs con la tarea de la tarjeta (Spec E), y el presupuesto es el del
  item. Sin dato no es 0: Codex muestra tokens. El gasto es telemetría local, y
  la tarjeta lo rotula así.
- **Dos logos.** Quién escribió y quién revisó, cada uno con el logo de su CLI.
  Tienen que ser distintos: `hoom review` ya se niega a revisar con el mismo
  provider que escribió, salvo con `--same-provider`, y en ese caso la tarjeta
  lo muestra.
- **Replay de la tarjeta como video.** La historia de cómo se ganó cada
  columna, reproducida en el tiempo. Así el método se aprende mirándolo. La
  evidencia sale de git y viaja. La narración de cada run sale de
  `.hoom/runs/` y solo existe en la computadora donde corrió: cuando falta, se
  rotula.

## Lo que la cabina no cambia

- `hoom cockpit` sigue siendo el lanzador de tmux. La cabina vive dentro de
  `hoom serve` y continúa el Studio v1 a v4, que ya tiene el Escenario, el
  panel de evidencia, la aprobación de specs y el token. Una aplicación aparte
  sería un segundo cerebro.
- El Studio sigue siendo un control remoto y nunca un segundo cerebro: cada
  botón llama a la misma función que su verbo CLI, y todo lo nuevo nace primero
  como verbo. La columna la calcula una función pura dentro del binario (como
  `runcmd.Stage` para el Escenario), y la página solo pinta.
- Directorio `.hoom/`, binario `hoom`, UI embebida sin assets de red, token
  para toda acción, loopback por defecto y el chip del check siempre visible.
- Si el Studio no corre, la terminal funciona igual que siempre.

## No-goals del Studio que esta cabina revisa

- "Terminal embebida" (v1 a v4, permanente): pasa a permitir el espejo de solo
  lectura de un pane de tmux. Escribir en el pane sigue afuera.
- "`git merge` y operaciones de git desde la UI" (v2): integrar pasa a ser el
  arrastre a una columna humana, y guardar en git un botón de la tarjeta.
  Sigue siendo un acto humano: lo hace la persona que arrastra, y antes corre
  `hoom task done`, que exige verde, huella y todo commiteado.
- "Ejecutar agentes desde el Studio" (v2): ya lo había revisado el v3 con
  `hoom run`. La cabina usa el sobre (`hoom agent`), que agrega scope, verify y
  check.

Siguen afuera: editor de código, gestión de archivos, hablar con un modelo,
multiusuario y servidor.

## Brechas del harness que expone la cabina

La tabla de columnas pide evidencia que hoy no existe o que el sobre trata
mal. Cada brecha la cierra una spec:

1. **El test-writer deja tests rojos por diseño, y su sobre cierra rojo.** El
   sobre corre verify al final y, con veredicto rojo, cierra NO-ENTREGABLE.
   Pero el test-writer escribe antes que el writer, así que sus tests fallan.
   En el dogfood de B1 y B2 se resolvió a mano: el writer commiteó primero un
   esqueleto de firmas sin comportamiento, y el test-writer escribió contra él
   tests que compilan y fallan. C1 define qué evidencia gana la columna
   Test-writer y dónde vive el esqueleto, y el sobre del test-writer tiene que
   aceptar ese rojo esperado como su entrega.
2. **Una review limpia no deja rastro.** `hoom review` observa los hallazgos
   que aparecen. Si no aparece ninguno, en disco no queda nada que diga que
   hubo review, con qué lente, ni quién escribió y quién revisó (eso vive en
   sidecars locales). C1 agrega un registro de review commiteable: huella
   revisada, lentes, provider del writer y del reviewer, e ids de los
   hallazgos. Es un artefacto de rol y no un veredicto: dice "revisé esto", no
   "esto está bien". También es la fuente de los dos logos.
3. **La aceptación no existe.** C1 crea el registro (append-only, con
   identidad git, atado a la huella del veredicto vigente e invalidado si el
   árbol cambia) con su verbo, y lo suma al piso no aflojable del sobre. C3 le
   pone el botón.
4. **El registro del sobre no tiene PID propio.** Llega al PID a través de su
   `run_id`, y antes del paso run solo tiene el latido. C3 le agrega el PID,
   para que "interrumpido" sea un hecho y no una inferencia por silencio.
5. **El analista escribe `backlog.md`, no items.** Su scope es
   `.hoom/specs/**` (B1). C1 decide cómo pasa a escribir en `.hoom/items/`, y
   qué pasa con `.hoom/specs/backlog.md` y con `hoom context`, que hoy lo
   cuenta.
6. **Los espacios de trabajo de otras herramientas no se ven.** La llave es la
   rama `hoom/<slug>` de `hoom task`, así que un worktree creado por Orca (rama
   `hoomdev/...`) no queda atado a ningún item. C1 decide si el item puede
   declarar su rama.

## Plan

Prerrequisitos, ya integrados en main:

- **B1** `arquitecto-bajo-el-sobre` (PR #17): el arquitecto escribe su spec
  bajo el sobre, que es justo lo que lanza la columna Arquitecto. Además, el
  sobre deja de certificar la nada (`sin-entrega`) y los hallazgos saben de
  qué tarea son.
- **B2** `gate-findings-open` (PR #18): la salida de Review la impone
  `verify`, con una sola definición de hallazgo abierto.

Las cuatro specs, en orden. Cada una sirve sola antes de que exista la
siguiente:

- **C1 — Items y columna derivada.** `.hoom/items/`, los verbos
  `hoom item add` y `hoom item list [--json]` (columna, medidor y qué falta
  para la siguiente), la función de columna como función pura en el binario, y
  las brechas 1, 2, 3, 5 y 6. Sin UI: la columna se puede consultar desde la
  terminal antes de que exista el tablero.
- **C2 — Tablero de solo lectura.** La vista del tablero en `hoom serve`: las
  ocho columnas, las tarjetas con su medidor, los fantasmas de los sobres en
  curso, los rojos con su motivo, los subestados de Review, los dos logos, el
  gasto, la curva del trinquete y los modos normal y experto. No escribe nada,
  así que no puede romper nada.
- **C3 — Acciones desde la tarjeta.** Arrastrar con confirmación (rol,
  provider y presupuesto), las firmas de las columnas humanas, verificar de
  nuevo, integrar, guardar en git, reanudar y volver a lanzar, abrir la sesión
  interactiva del espacio de trabajo y el espejo de solo lectura de su pane.
  También la brecha 4. Todo pasa por los mismos verbos de la CLI.
- **C4 — Timeline, replay y doctor.** La historia de la tarjeta desde git y los
  `.jsonl`, con la correlación tool_use/tool_result que marca cuándo un
  subagente entra y sale de escena. El replay como video. Y un doctor que
  explica por qué un veredicto es rojo o por qué una tarjeta no avanza.

Después de C4, y solo si se pide: el piloto automático por tarjeta.

## Preguntas abiertas

Las cierra la spec que corresponda. Acá queda la recomendación:

- **¿Cómo se descarta un item?** Es una decisión humana, no evidencia.
  Recomendación: un registro firmado con motivo, igual que una aprobación, que
  saca la tarjeta del tablero sin borrar su historia.
- **¿El presupuesto es de la tarjeta o del arrastre?** Recomendación: el item
  declara un tope opcional, y cada arrastre propone lo que queda y la persona
  lo confirma o lo cambia.
- **¿El gasto tendría que viajar?** Hoy es telemetría local, así que cada
  computadora ve solo el gasto de sus propios runs. Recomendación: rotularlo en
  C2 y decidir después, con uso real, si lo commitea el registro de review o un
  artefacto de costo.
- **Guardar en git: ¿commit y push en un solo botón?** Recomendación: un botón
  que hace lo que falte, y un ícono que distingue "sin guardar" de "sin
  publicar".
- **El esqueleto de firmas (brecha 1): ¿quién lo escribe?** Recomendación: el
  writer, invocado como ayudante desde la columna Test-writer y limitado a
  firmas. El arquitecto no puede, porque solo escribe `.hoom/specs/**` (B1).
