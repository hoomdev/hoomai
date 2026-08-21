# Spec: HoomAI Studio v3 — cockpit: providers, chat y ejecución headless

Estado: BORRADOR — pendiente de aprobación humana.
Depende de: `studio-v2-acciones.md` (criterios 11 a 20) integrada.
Tarea sugerida: `hoom task start studio-v3-cockpit`.

## Objetivo

Que el trabajo se pueda DESPACHAR desde el Studio: elegís el provider de IA
(la CLI que ya tenés instalada), escribís el pedido en un chat, y el Studio
lanza esa CLI en modo headless sobre el proyecto o el worktree de la tarea —
exactamente lo que hoy hacés a mano en la terminal — mostrando en vivo la
actividad de los agentes (qué rol está activo, qué herramienta usa, qué
archivo toca). hoomAI sigue siendo agnóstico de IA: el binario JAMÁS llama a
una API de modelo; ejecuta la CLI del usuario como subproceso, con la
autenticación, configuración y subagentes que esa CLI ya tiene. La terminal
sigue siendo primera clase: todo lo nuevo nace como verbo CLI (`hoom
providers`, `hoom run`) y el Studio le pone botones.

## No-goals

- Llamar APIs de modelos o gestionar credenciales de IA: eso es de cada CLI.
- Editor de código, terminal embebida (no-goal PERMANENTE del Studio).
- Que la narración toque la evidencia: los eventos de un run JAMÁS alimentan
  veredictos, gates ni checks. Narración ≠ evidencia, siempre.
- El teatro estilo Odysseus (avatares, escenas por rol): eso es la v4, sobre
  el mismo stream. Acá solo el feed cronológico básico.
- Telemetría hacia afuera: los logs de runs son locales, nunca salen.
- Reemplazar el flujo de terminal: si el Studio no corre, `claude` + los
  prompts del tutorial siguen funcionando idénticos.

## Contratos

Verbos CLI nuevos (primero; el server los reusa):

- `hoom providers [--json]` — detecta en PATH las CLIs soportadas y reporta
  disponibilidad: `[{name, installed, bin}]`. Soportadas: `claude`,
  `opencode`, `codex`, `gemini`.
- `hoom run --provider <p> [--task <slug>] [--continue <run-id>] "<prompt>"`
  — lanza la CLI headless como subproceso, cwd = proyecto o worktree de la
  tarea. Comando por provider (headless + salida estructurada si existe):
  `claude -p --output-format stream-json` · `codex exec` · `gemini -p` ·
  `opencode run`. `--continue` retoma la MISMA sesión del provider (los
  mecanismos nativos de cada CLI). Propaga el exit code del subproceso.
- Log por run en `.hoom/runs/<run-id>.jsonl`, append-only, con eventos
  normalizados a UN esquema: `{ts, kind, agent, detail}` con `kind` en
  `start|text|tool|agent|end|error`. Provider sin stream estructurado
  degrada a eventos `text` — degradación declarada, nunca error.
- `.hoom/runs/` queda EXCLUIDO de la huella del candidato y de Git
  (`.hoom/.gitignore`): telemetría local; la evidencia (veredictos) viaja,
  la narración no.

API HTTP (POST siempre con `X-Hoom-Token`; misma función que el verbo):

- `GET /api/providers` — igual a `hoom providers --json`.
- `POST /api/runs` `{provider, prompt, task?}` — crea el run, responde
  `{run_id}`. Un run por árbol a la vez (mismo worktree ocupado = 409);
  worktrees distintos corren en paralelo (un writer por tarea).
- `GET /api/runs` y `GET /api/runs/{id}?after=<n>` — estado
  (`running|done|error|canceled`, exit code) + eventos desde el índice `n`
  (polling incremental, consistente con la UI actual).
- `POST /api/runs/{id}/input` `{prompt}` — continúa la misma sesión (el
  "Spec aprobado, adelante" del tutorial, sin perder contexto).
- `POST /api/runs/{id}/cancel` — termina el subproceso.
- UI: selector de provider (solo instalados), chat por tarea, feed en vivo
  del run, botón cancelar. El feed vive visualmente SEPARADO de la zona de
  evidencia (veredictos/aprobaciones).

## Casos límite y errores esperados

- Provider no instalado: error inmediato con la instrucción de instalación;
  el selector de la UI ni lo ofrece.
- La CLI espera un permiso interactivo en headless y se queda muda: timeout
  configurable del run (default 30 min, como los gates) + cancel manual; el
  error explica que hay que preconfigurar los permisos del proyecto en esa
  CLI.
- Stream estructurado no parseable (la CLI cambió su formato): el run
  degrada a eventos `text` crudos y lo dice; el log nunca se pierde.
- `hoom serve` muere con runs vivos: los subprocesos reciben terminate; al
  arrancar, los runs `running` huérfanos pasan a `error` con nota.
- Prompt vacío → 400. Task inexistente → 404. Run inexistente → 404.
- Dos runs sobre el mismo worktree → 409 con el run_id en curso.

## Criterios de aceptación verificables

(Continúa la numeración: aquí CA-21..CA-30.)

- CA-21: `hoom providers --json` emite las cuatro CLIs soportadas con
  `installed` verdadero/falso según el PATH real.
- CA-22: `hoom run --provider <p>` ejecuta el comando headless documentado
  para `p` como subproceso, con cwd en el proyecto o en el worktree de
  `--task`, y propaga su exit code.
- CA-23: cada run persiste `.hoom/runs/<id>.jsonl` y ese directorio queda
  excluido de la huella: correr un run NO rompe un `hoom check` verde.
- CA-24: los eventos quedan normalizados al esquema único
  `{ts, kind, agent, detail}`; un provider sin salida estructurada produce
  eventos `kind:"text"` y el run lo declara (degradación visible).
- CA-25: `POST /api/runs` con token crea el run y `GET /api/runs/{id}?after=n`
  entrega estado + eventos incrementales, con los que la UI pinta el feed en
  vivo.
- CA-26: dos runs simultáneos sobre el MISMO árbol: el segundo recibe 409
  con el run en curso; sobre worktrees distintos, ambos corren.
- CA-27: `POST /api/runs/{id}/input` continúa la misma sesión del provider
  usando su mecanismo nativo de continuación (mismo contexto, mismo run).
- CA-28: `POST /api/runs/{id}/cancel` termina el subproceso y el run queda
  `canceled` conservando su log completo.
- CA-29: `hoom verify` y `hoom check` no leen `.hoom/runs/`: ningún evento
  de run participa en un veredicto ni en un check (narración ≠ evidencia,
  verificable en el código y en los tests).
- CA-30: `POST` de runs, input o cancel sin token válido → 401 y CERO
  efectos: ningún subproceso lanzado, ningún archivo escrito.

## Decisiones de diseño y alternativas descartadas

- **Subproceso de la CLI del usuario** — descartado llamar APIs de modelos
  desde hoom: rompería el agnosticismo, metería credenciales y duplicaría lo
  que las CLIs ya hacen bien (auth, subagentes, permisos, resume).
- **Logs de runs locales e ignorados por Git** — descartado versionarlos
  como los veredictos: la narración no es evidencia; versionarla invitaría a
  tratarla como tal. Lo que viaja es lo que firma.
- **Esquema de eventos normalizado con parser por provider** — descartado
  mostrar el stream crudo de cada CLI: la UI y la v4 se escriben UNA vez
  contra un esquema; los parsers son defensivos y degradan a `text`.
- **Polling incremental `?after=n`** — descartado SSE/websockets por ahora:
  mismo mecanismo simple del resto del Studio; si el feed pide más fluidez,
  SSE es un cambio interno sin tocar el contrato.
- **Un run por worktree** — espejo del "un writer por tarea" del harness:
  el aislamiento ya existe, el cockpit lo respeta en vez de inventar colas.

## Riesgos y deuda aceptada

- Quien tenga el token puede poner una IA a editar código en tu máquina:
  el token pasa a ser frontera crítica. Mitigación: loopback por default,
  advertencia agresiva en `--addr` no-loopback, y el harness como red final
  (sin veredicto verde no se integra). Deuda aceptada: no hay auth de
  usuarios (operador único local).
- Los permisos headless dependen de la configuración de cada CLI en el
  proyecto (allowlists propias): fuera del control de hoom. Deuda declarada:
  guía por provider en el README; `hoom agents` podrá instalar defaults
  seguros en una iteración futura.
- Los formatos de stream de las CLIs cambian con sus versiones: parsers
  defensivos con degradación a texto; el riesgo es perder detalle del feed,
  nunca el run ni el log.
- Un run largo consume tokens de la cuenta del usuario sin supervisión:
  mitigado con el feed en vivo, cancel y timeout; el costo es del operador,
  como en la terminal.
