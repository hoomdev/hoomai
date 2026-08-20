# Spec: HoomAI Studio v1 — dashboard de lectura

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start studio-v1-dashboard`.

## Objetivo

Darle al harness una piel visual de SOLO LECTURA: `hoom serve` levanta, desde el
mismo binario, un dashboard local que muestra veredictos, tareas paralelas y
tendencia por gate. El Studio es un espejo del harness, no un segundo cerebro:
cada dato que pinta sale de un verbo que el CLI ya expone. Como hoy solo
`verify` habla JSON, la v1 incluye primero los verbos JSON que faltan
(CLI-first): lo que la UI necesita se agrega al CLI — donde queda scriptable,
testeable y disponible para agentes — y recién después se pinta.

## No-goals

- Editor de código, terminal embebida, gestión de archivos. **No-goal
  PERMANENTE del Studio** (v1, v2 y siempre): el código se escribe en el
  editor/CLI de IA de cada uno; el Studio es donde el humano gobierna.
- Acciones de escritura (disparar verify, crear/cerrar tareas, aprobar specs,
  subir intake): eso es la v2 (`studio-v2-acciones.md`).
- Autenticación multiusuario, HTTPS, despliegue remoto: v1 es loopback local.
- Base de datos: la fuente de verdad sigue siendo `.hoom/verdicts/` en Git;
  cualquier índice (SQLite) es cache regenerable futura, fuera de esta tarea.
- Hablar con un modelo de IA: hoomAI sigue siendo agnóstico; el server jamás
  invoca un LLM.

## Contratos

Verbos CLI nuevos o extendidos (primero; el server los reusa):

- `hoom task list --json` — array JSON en stdout: `[{slug, branch, state,
  verdict_id, dirty}]`, con `state` ∈ `green|drift|red|no-verdict`. Sin
  tareas: `[]`.
- `hoom report --json [-n N]` — `{runs: [...], gates: {nombre: {pass_rate,
  last_status}}}`, la misma información de la vista texto.
- `hoom check --json` — `{ok, verdict_id, fingerprint_match, action}` con el
  mismo exit code que el modo texto.
- `hoom serve [--addr host:puerto]` — default `127.0.0.1:4666`; sirve la UI
  embebida (go:embed, cero assets de red) y la API de lectura.

API HTTP (regla 1:1 — cada endpoint mapea a una lectura que el CLI ya tiene y
reusa la MISMA función interna y los mismos structs de serialización):

- `GET /` — la UI embebida.
- `GET /api/status` — proyecto, perfil, política, resultado de check.
- `GET /api/verdicts?n=50` — veredictos del más nuevo al más viejo.
- `GET /api/verdicts/{id}` — veredicto completo, incluido `output_tail` por
  gate (el tail del test clickeable del tablero). Inexistente: 404 JSON.
- `GET /api/tasks` — igual a `hoom task list --json`.
- `GET /api/report?n=10` — igual a `hoom report --json`.

## Casos límite y errores esperados

- Proyecto sin veredictos: estado vacío con la acción sugerida (`hoom verify`),
  nunca un error.
- Veredicto ilegible en `.hoom/verdicts/`: se omite y se cuenta como
  advertencia en la respuesta (mismo criterio que `LoadAll`), sin tumbar nada.
- Directorio sin `.hoom/`: `hoom serve` sale con exit 1 y la acción exacta
  (`hoom init`).
- Puerto ocupado: error claro con la sugerencia `--addr`.
- `--addr` no-loopback: warning explícito de exposición sin autenticación.
- Historial grande: paginación por `?n=` (default 50), nunca cargar todo a la UI.

## Criterios de aceptación verificables

(Numeración continua con la v2: esta v1 usa los criterios 1 a 10; la v2
continúa del 11 al 20, para que `spec_trace` nunca cruce criterios entre
specs. Nota: este spec solo menciona SUS tokens CA-n — la regex del gate
captura cualquier token, así que citar los de otro spec contaminaría la traza.)

- CA-1: `hoom task list --json` emite en stdout un array JSON con slug, rama y
  estado (`green|drift|red|no-verdict`) por tarea; sin tareas emite `[]`.
- CA-2: `hoom report --json -n N` emite en stdout el historial y el pass-rate
  por gate como JSON, con la misma información que la vista texto.
- CA-3: `hoom check --json` emite el resultado del check como JSON y conserva
  el mismo exit code que el modo texto (0 coincide, 1 no).
- CA-4: `hoom serve` escucha por defecto SOLO en 127.0.0.1 y sirve la UI
  embebida en `/` desde el propio binario.
- CA-5: `GET /api/verdicts` devuelve los veredictos ordenados del más nuevo al
  más viejo y respeta `?n=` (default 50).
- CA-6: `GET /api/verdicts/{id}` devuelve el veredicto completo incluido el
  `output_tail` de cada gate; un id inexistente responde 404 con cuerpo JSON.
- CA-7: `GET /api/tasks` y `GET /api/report` producen exactamente la misma
  estructura JSON que `hoom task list --json` y `hoom report --json`.
- CA-8: un archivo ilegible en `.hoom/verdicts/` no tumba el server ni vacía
  el listado: se omite y se reporta en un campo de advertencias.
- CA-9: `hoom serve` fuera de un proyecto hoom termina con exit 1 y un mensaje
  que indica la acción exacta (`hoom init`).
- CA-10: con la red deshabilitada, el dashboard carga y muestra datos: la UI
  no referencia ningún asset externo (fuentes, JS, CSS de CDN).

## Decisiones de diseño y alternativas descartadas

- **Mismo binario con go:embed** — descartada una app separada (Electron,
  repo aparte): un solo artefacto instalable, cero fricción, la UI viaja con
  el CLI.
- **CLI-first estricto** — descartado implementar lógica de listado en el
  server: si un endpoint necesita algo que el CLI no tiene, primero nace el
  verbo CLI y el server lo reusa. La UI nunca se convierte en segundo cerebro.
- **Polling simple desde la UI** — descartados websockets/SSE en v1: los
  veredictos cambian a ritmo humano; recargar `/api/*` alcanza.
- **Loopback por default** — descartado escuchar en 0.0.0.0: exponer es una
  decisión consciente vía `--addr`, nunca el default.
- **Numeración CA disjunta entre v1 y v2** — porque `spec_trace` busca los
  tokens CA-n globalmente en los tests; compartir CA-1 entre dos specs haría
  que un test de la v1 "trace" un criterio de la v2.

## Riesgos y deuda aceptada

- Exponer `--addr` a la red queda sin autenticación en v1 (solo lectura):
  riesgo documentado en el warning; el token de acciones llega con la v2.
- Escanear todos los veredictos por request no escala a historiales enormes:
  deuda aceptada; el cache SQLite regenerable ya está en el roadmap.
- La UI embebida engorda el binario: aceptado mientras se mantenga sin
  frameworks pesados (HTML/CSS/JS plano embebido).
