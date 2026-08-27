# Spec: hallazgos como artefactos + rol Refutador

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start hallazgos-y-refutador`.

## Objetivo

Cerrar los dos huecos que la comparación con Gentle dejó a la vista: (1) los
hallazgos de review hoy viven en el chat — efímeros, redescubribles,
imposibles de auditar; pasan a ser ARTEFACTOS append-only en
`.hoom/findings/`, atados a la huella del árbol al momento de encontrarlos,
con ciclo de vida explícito (abierto → corregido | refutado). (2) Nace el
rol 09 **Refutador**: antes de mandar a corregir un hallazgo, alguien cuyo
trabajo es intentar TUMBARLO con evidencia — el antídoto contra los
"hallazgos persuasivos pero falsos" — con tope de ciclos para que ningún
review entre en loop. El ciclo de review gana memoria: un hallazgo con
estado no puede redescubrirse eternamente, y el historial de qué se
encontró, qué era falso y qué se corrigió viaja en Git como el resto de la
evidencia del proceso.

## No-goals

- Que un hallazgo cambie un veredicto: los hallazgos son narración
  calificada, no evidencia de gates. `verify` y `check` no leen
  `.hoom/findings/` — el camino para que un hallazgo bloquee sigue siendo
  convertirlo en regla determinística (test, semgrep, lint).
- El doble juez multi-provider: queda en el roadmap con su condición de
  activación (cuando al Refutador se le escapen falsos positivos con
  frecuencia).
- Editar o borrar hallazgos: append-only estricto. Un error de registro se
  resuelve con una transición o un hallazgo nuevo, jamás editando la
  historia.
- Severidades automáticas o clasificación por el binario: la severidad la
  declara quien registra; el binario guarda y deriva estados, no opina.

## Contratos

Verbo nuevo `hoom finding` (CLI-first; agentes y Studio lo reusan):

- `hoom finding add --sev low|medium|high --lens <lente> --file <ruta> "<descripcion>"`
  — crea `.hoom/findings/<id>.json`, INMUTABLE: id, fecha, severidad,
  lente (readability|reliability|resilience|risk u otra), archivo
  señalado, descripción, rol autor y la huella del árbol en ese momento.
- `hoom finding resolve <id> --as corregido|refutado --evidence "<texto>"`
  — crea la transición como archivo APARTE (`<id>.res.json`: estado final,
  evidencia, autor, fecha). Sin `--evidence` el comando se niega: nadie
  cierra un hallazgo sin decir por qué. Un hallazgo ya resuelto no admite
  otra transición; reabrir = hallazgo nuevo que cita al anterior.
- `hoom finding list [--open] [--json]` — estado derivado del par
  hallazgo+transición; los hallazgos cuya huella ya no coincide con el
  árbol actual se marcan "el código cambió desde el hallazgo" (a
  re-verificar, no a asumir).
- `.hoom/findings/` viaja en Git (evidencia del proceso, como approvals)
  pero queda EXCLUIDO de la huella del candidato: registrar o resolver un
  hallazgo jamás rompe un check verde.

Contratos de agentes:

- **09-refutador.md** (nuevo): solo lectura sobre el código; toma los
  hallazgos abiertos (`hoom finding list --open --json`) y para cada uno
  busca evidencia DETERMINÍSTICA — correr el test, reproducir el caso,
  citar la línea — para refutarlo (registra `resolve --as refutado`) o
  corroborarlo (lo deja abierto anotando la evidencia a favor en su
  reporte). PROHIBIDO refutar por opinión: sin evidencia ejecutable o
  citable, el hallazgo queda abierto. Tope duro: máximo 2 ciclos
  refutación-corrección por hallazgo; si al segundo sigue en disputa, se
  escala a Hoom con ambas evidencias — el humano desempata, nunca el loop.
- **06-reviewer.md** (actualizado): los hallazgos que sobreviven su propia
  lectura se registran con `hoom finding add` — el chat no es registro. La
  corrección sigue siendo del Writer, y `resolve --as corregido` se
  registra recién con el gate verde de la corrección.

Studio:

- `GET /api/findings` — los mismos bytes que `hoom finding list --json`.
- Sección "Hallazgos" en el panel de evidencia: lista con severidad,
  estado, lente, evidencia de cierre, y el aviso de huella cambiada; punto
  de atención en el menú cuando hay abiertos high.

## Casos límite y errores esperados

- Resolver un id inexistente o ya resuelto: error con el estado actual.
- `--evidence` vacío o solo espacios: rechazado, mismo mensaje.
- Hallazgo registrado fuera de un repo git: la huella queda vacía y el
  marcado de "código cambió" no aplica (degradación declarada).
- Dos hallazgos idénticos: se permiten (append-only no deduplica); el
  Refutador los resuelve citando al duplicado.
- `.hoom/findings/` con un JSON corrupto: se omite y se reporta como
  advertencia (mismo criterio que los veredictos ilegibles), jamás tumba
  el listado.
- Proyecto sin hallazgos: `list` responde vacío sin error; la sección del
  Studio muestra el estado vacío.

## Criterios de aceptación verificables

(Continúa la numeración: aquí CA-53..CA-60.)

- CA-53: `hoom finding add` crea el registro con id, severidad, lente,
  archivo, descripción, autor y la huella del árbol al momento; ese
  archivo jamás se modifica después (las transiciones viven aparte).
- CA-54: `hoom finding resolve` exige `--as corregido|refutado` y
  `--evidence` no vacío; sin evidencia se niega y no escribe nada.
- CA-55: `hoom finding list --json` deriva el estado (abierto | corregido
  | refutado) del par hallazgo+transición, y marca los hallazgos cuya
  huella no coincide con el árbol actual.
- CA-56: un hallazgo resuelto no admite una segunda transición: el intento
  falla con el estado vigente y el archivo de transición original queda
  intacto.
- CA-57: el contrato embebido del Refutador (09) exige evidencia
  determinística para refutar, le prohíbe editar código y fija el tope de
  2 ciclos con escalamiento al humano.
- CA-58: el contrato embebido del Reviewer (06) manda registrar los
  hallazgos sobrevivientes con `hoom finding add` en vez de dejarlos en el
  chat.
- CA-59: `GET /api/findings` responde exactamente los mismos bytes que
  `hoom finding list --json`, y la UI embebida contiene la sección de
  hallazgos.
- CA-60: los hallazgos no tocan la evidencia: `verify` y `check` no leen
  `.hoom/findings/`, y registrar o resolver un hallazgo NO cambia la
  huella del candidato (un check verde sigue verde).

## Decisiones de diseño y alternativas descartadas

- **Hallazgo inmutable + transición como archivo aparte** — descartado un
  archivo con historia embebida que se reescribe: dos archivos append-only
  dan cero conflictos de merge por construcción (el patrón de approvals) y
  hacen imposible editar la historia.
- **Una sola transición terminal** — descartados los reabiertos in-place:
  reabrir es un hallazgo nuevo que cita al viejo, así el grafo de eventos
  solo crece y el "día del juicio" de cada hallazgo es auditable completo.
- **Findings viajan en Git pero fuera de la huella** — igual que los
  veredictos: el acto de registrar evidencia del proceso no puede alterar
  el candidato que certifica.
- **Evidencia obligatoria para cerrar** — es la traducción hoomAI de la
  "verificación adversarial" de Gentle: acá no se le pide evidencia al
  agente por cortesía, el binario se niega a cerrar sin ella.
- **El tope de ciclos vive en el contrato, no en el binario** — el binario
  no puede contar ciclos de conversación; el contrato sí puede mandarlos y
  el registro de transiciones los hace auditables. Frontera intacta.
- **Severidad declarada, no calculada** — clasificar es juicio; el binario
  registra y deriva, no opina.

## Riesgos y deuda aceptada

- El Refutador es un rol LLM refutando a otro LLM: mejor que nada, pero no
  determinístico. Mitigación: la evidencia obligatoria y el tope de ciclos;
  la salida de verdad sigue siendo mecanizar el hallazgo como gate.
- Disciplina de cierre: un hallazgo corregido cuyo `resolve` nadie registra
  queda abierto para siempre — visible en el Studio, molesto a propósito,
  pero exige el hábito (mismo trade-off que las preguntas del contexto).
- El marcado "el código cambió desde el hallazgo" compara huellas globales,
  no por archivo: puede avisar de más (cambió otra parte del árbol). Afinar
  a huella por archivo es mejora futura si el ruido molesta.
- Gate futuro opcional `findings_open` (rojo con hallazgos high abiertos)
  queda como deuda declarada en el roadmap: primero ver cómo se usa el
  ciclo antes de darle poder de bloqueo.
