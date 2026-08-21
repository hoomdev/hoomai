# Spec: HoomAI Studio v4 — escenario: el equipo de agentes en escena

Estado: BORRADOR — pendiente de aprobación humana.
Depende de: `studio-v3-cockpit.md` (criterios 21 a 30) integrada.
Tarea sugerida: `hoom task start studio-v4-escenario`.

## Objetivo

Darle al run una segunda vista: el **Escenario** — el equipo de agentes de
hoomAI como tarjetas en escena (orquestador, analista, arquitecto, designer,
scout, writer, test-writer, reviewer, characterizer), donde se VE quién está
actuando, qué encargo recibió y cuántas veces participó, al estilo del
teatro de agentes de Odysseus. Se alimenta del MISMO stream de eventos de la
v3: cero fuentes nuevas de datos. La lógica de atribución (qué evento
pertenece a qué rol) vive en el binario como una función pura — testeable
por los gates y heredable por cualquier UI futura — y la página solo pinta
lo que el binario computó. La atribución es HONESTA: lo que no se sabe, se
atribuye al orquestador (que es quien realmente ejecuta en headless), nunca
a un rol inventado.

## No-goals

- Fuentes de datos nuevas: el escenario deriva 100% de los eventos que ya
  registra la v3; ni un hook más, ni un formato más.
- Que la escena toque la evidencia: sigue siendo narración; veredictos,
  huellas y aprobaciones quedan intactos y visualmente separados.
- Assets externos: avatares tipográficos/emoji; ni una imagen remota
  (sigue rigiendo el criterio de UI autocontenida de la v1).
- Animaciones complejas o audio: v4 es tarjetas con estado, no un juego.
- Editor de código, terminal embebida (no-goal permanente del Studio).

## Contratos

Lógica en el binario (primero; la UI la consume):

- `runcmd.Stage(info, events) StageView` — función PURA que computa la
  escena desde los eventos de un run:
  - `StageView{status, exit_code, actors: []Actor}`.
  - `Actor{role, known, acts, last_detail, active}`.
  - Reparto fijo, siempre presente y en este orden: `orquestador`,
    `analista`, `arquitecto`, `designer`, `scout`, `writer`,
    `test-writer`, `reviewer`, `characterizer`. Roles sin actividad:
    `acts: 0` (la UI los atenúa).
  - Evento `kind:agent` con `hoom-<rol>` suma participación al rol, guarda
    el encargo (`last_detail`) y lo deja EN ESCENA (`active`) hasta la
    siguiente delegación, mientras el run corre.
  - Eventos `tool`/`text` sin agente se atribuyen al orquestador.
  - `subagent_type` desconocido (no `hoom-*` conocido): tarjeta extra con
    `known:false` al final del reparto — el escenario nunca se rompe.
  - Run terminado: nadie `active`; estado y conteos se conservan.
- `GET /api/runs/{id}/stage` — la StageView del run (misma autorización que
  el resto de lecturas; 404 si el run no existe).
- UI: en la vista del run, toggle **Feed | Escenario** — dos vistas del
  mismo stream, mismo polling. La tarjeta activa se destaca; el encargo se
  muestra como burbuja; al terminar queda el resumen de participaciones.

## Casos límite y errores esperados

- Run recién creado sin eventos: escenario válido con el reparto completo
  en cero, orquestador activo si corre.
- Provider sin stream estructurado (todo `text`): escenario válido con solo
  el orquestador actuando — degradación heredada de la v3, jamás un crash.
- Rol conocido con mayúsculas o variantes (`hoom-Test-Writer`): la
  normalización lo mapea a su tarjeta; si aún así no matchea, cae en la
  tarjeta desconocida, nunca se pierde.
- Run cancelado o con error: la escena queda congelada con el estado final
  visible.
- Run inexistente: 404 JSON, como el resto de la API.

## Criterios de aceptación verificables

(Continúa la numeración: aquí CA-31..CA-38.)

- CA-31: `GET /api/runs/{id}/stage` computa el escenario desde los MISMOS
  eventos del run que alimenta el feed (una sola fuente); run inexistente
  responde 404 JSON.
- CA-32: la StageView incluye SIEMPRE el reparto fijo completo (orquestador
  y los 8 roles) en orden estable; los roles sin actividad llevan `acts: 0`.
- CA-33: un evento `agent` con `hoom-scout` suma participación a la tarjeta
  `scout` y conserva su último encargo en `last_detail`.
- CA-34: eventos `tool` y `text` sin agente se atribuyen al orquestador y
  jamás a un rol que no actuó (atribución honesta).
- CA-35: con el run terminado (done, error o canceled) ningún actor queda
  `active` y los conteos de participación se conservan con el estado final.
- CA-36: mientras el run corre, el último rol delegado está `active` (en
  escena) junto al orquestador; una nueva delegación pasa la escena al
  nuevo rol.
- CA-37: un `subagent_type` desconocido produce una tarjeta extra con
  `known: false` al final del reparto, sin romper el escenario.
- CA-38: un run cuyos eventos son solo `text` (provider sin stream) produce
  un escenario válido donde únicamente el orquestador registra actividad.

## Decisiones de diseño y alternativas descartadas

- **Atribución computada en el binario (función pura + endpoint)** —
  descartado hacerla en JavaScript: en el binario la testean los gates
  (spec_trace incluida), la hereda una UI nativa futura, y la UI queda
  tonta como debe ser.
- **Atribución honesta al orquestador por defecto** — descartado "adivinar"
  que un tool pertenece al último subagente: en headless el que ejecuta es
  el proceso padre; inventar atribución sería narración sobre narración.
- **Reparto fijo siempre visible** — descartado mostrar solo roles activos:
  ver al equipo completo (con los que no actuaron atenuados) comunica el
  método hoomAI de un vistazo, que es el punto del teatro.
- **Avatares tipográficos/emoji** — descartadas imágenes: la UI sigue
  autocontenida y el binario no engorda.
- **Toggle Feed | Escenario** — descartado reemplazar el feed: el feed es
  la lupa (línea por línea), el escenario es el mapa (quién hace qué); se
  complementan sobre el mismo stream.

## Riesgos y deuda aceptada

- La escena hereda los límites del stream del provider: con CLIs sin
  salida estructurada el teatro es pobre (solo el orquestador) — degradación
  declarada, se enriquece sola cuando el provider mejora su stream.
- El "en escena hasta la próxima delegación" es una aproximación: el stream
  de claude no anuncia cuándo TERMINA un subagente. Deuda declarada:
  correlacionar tool_use/tool_result por id en el parser para marcar
  regresos de escena — mejora del parser de la v3, no de esta vista.
- Más polling (stage además del feed cuando la vista está abierta): costo
  local despreciable; si molestara, SSE ya quedó anotado como cambio
  interno sin tocar contratos.
