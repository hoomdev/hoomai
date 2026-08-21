# Spec: salud del contexto — modos del Analista y hoom context

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start contexto-salud`.

## Objetivo

Cerrar la asimetría de enforcement del harness: el lado del código tiene
recibos determinísticos (veredictos, huellas, spec_trace) y el lado del
CONTEXTO era pura convención en prompts. Dos piezas: (1) el contrato del
Analista incorpora sus TRES modos de arranque — documento, reconstrucción
desde código, entrevista fundacional — que hoy solo existían en
conversaciones; (2) `hoom context`, el semáforo determinístico de la salud
del contexto: fuentes de intake, estado de visión y backlog, preguntas
abiertas contadas y staleness por fechas — medible con archivos y fechas,
sin LLM, con amarillos honestos igual que los gates.

## No-goals

- Que el binario ENTIENDA el contexto: nada de parsear semántica de specs
  ni validar que la visión sea "correcta" — eso es trabajo de modelo y la
  frontera queda donde está.
- Bloquear: `hoom context` informa, jamás corta un flujo. No hay rojo de
  contexto; hay amarillo visible. Verify y check no cambian.
- Ejecutar al Analista desde el binario: los modos son contrato del agente;
  hoom solo mide lo que los archivos ya dicen.

## Contratos

Contrato del Analista (`internal/agents/assets/agents/08-analista.md`,
instalado por `hoom agents`): nueva sección **"Modos de arranque"**, en
orden de preferencia y con detección obligatoria del estado de intake:

- **Modo A — documento**: hay documentos en `.hoom/intake/` → el flujo
  actual del contrato.
- **Modo B — reconstrucción desde código**: intake vacío pero el proyecto
  ya tiene código → contexto del Scout (solo lectura), visión marcada
  "RECONSTRUIDA DESDE CODIGO", cada afirmación de negocio es un supuesto
  listado como PREGUNTA PARA EL CLIENTE. Nunca se presenta como palabra
  del cliente.
- **Modo C — entrevista fundacional**: intake vacío y sin código relevante
  → propone la entrevista de 6 preguntas (qué es y para quién; módulos;
  roles; reglas innegociables textuales; qué NO quiere; prioridades). Las
  respuestas se guardan como documento fechado en `.hoom/intake/` y recién
  entonces se destila (Modo A). Sin respuestas no hay visión.

Verbo nuevo `hoom context [--json]` (CLI-first; el Studio lo reusa):

- Fuentes: cantidad de documentos en `.hoom/intake/` y fecha del más nuevo
  (fecha de último commit vía git; fallback a mtime si no está commiteado).
- Visión (`.hoom/specs/00-vision.md`) y backlog (`.hoom/specs/backlog.md`):
  existencia y última actualización.
- Preguntas abiertas: líneas con el marcador literal del contrato
  ("PREGUNTA PARA EL CLIENTE") en visión y backlog.
- Checks con estado ok/amarillo y acción exacta: intake más nuevo que la
  visión (posible desactualización), backlog sin fuente en intake, intake
  sin destilar (sin visión), preguntas abiertas pendientes, y proyecto sin
  contexto en absoluto (acción: entrevista fundacional, Modo C).
- Estado global: `verde` o `amarillo`. Exit code SIEMPRE 0.
- `GET /api/context` en el Studio: los mismos bytes que `--json`, y una
  sección "Contexto" en el panel lateral de evidencia.

## Casos límite y errores esperados

- Proyecto recién iniciado (todo vacío): amarillo con la acción de arranque
  (Modo C), nunca un error.
- Archivos de contexto sin commitear todavía: la fecha cae a mtime — la
  medición degrada, no se rompe.
- Visión sin backlog o backlog sin visión: amarillo específico por pieza.
- Marcador de pregunta con espaciado o mayúsculas variantes: se cuenta la
  forma literal del contrato; variantes no cuentan (regla simple y
  documentada antes que heurística).
- Directorio intake con subdirectorios o archivos ocultos: solo cuentan
  archivos regulares visibles.

## Criterios de aceptación verificables

(Continúa la numeración: aquí CA-39..CA-46.)

- CA-39: el contrato embebido del Analista contiene los TRES modos de
  arranque: documento, reconstrucción desde código (con la marca
  "RECONSTRUIDA DESDE CODIGO") y entrevista fundacional con sus 6
  preguntas.
- CA-40: `hoom context --json` emite las fuentes de intake (cantidad y
  fecha del más nuevo), el estado de visión y backlog (existencia y última
  actualización) y la cuenta de preguntas abiertas.
- CA-41: un documento de intake más nuevo que la última actualización de la
  visión produce el check amarillo de posible desactualización.
- CA-42: backlog existente con intake vacío produce check amarillo (backlog
  sin fuente); intake con documentos y sin visión produce check amarillo
  (intake sin destilar).
- CA-43: proyecto sin intake, sin visión y sin backlog → estado global
  amarillo con la acción exacta: la entrevista fundacional (Modo C del
  Analista).
- CA-44: la salud del contexto informa pero JAMÁS bloquea: el estado global
  solo puede ser verde o amarillo (nunca rojo) y el comando termina con
  exit 0 en todos los casos.
- CA-45: `GET /api/context` responde exactamente los mismos bytes que
  `hoom context --json` (paridad CLI–Studio, un solo cerebro).
- CA-46: las preguntas abiertas se cuentan por el marcador literal
  "PREGUNTA PARA EL CLIENTE" en visión y backlog; sin archivos la cuenta es
  0 y no hay error.

## Decisiones de diseño y alternativas descartadas

- **Fechas por git con fallback a mtime** — descartado solo-mtime (no
  sobrevive clones) y solo-git (rompe con archivos aún no commiteados): la
  medición honesta degrada en vez de mentir o fallar.
- **Marcador literal del contrato para contar preguntas** — descartada una
  heurística de variantes: el contrato ya fija la forma exacta; medir esa
  forma mantiene la promesa "sin LLM, sin semántica".
- **Amarillo, nunca rojo** — el contexto desactualizado es deuda visible,
  no un build roto: bloquear acá empujaría a la gente a saltarse el
  comando. La misma razón por la que los gates ausentes son amarillos.
- **Los modos viven en el contrato del agente, no en el binario** — el
  binario mide archivos; decidir cómo arrancar una visión es juicio de
  modelo. Cada cosa de su lado de la frontera.

## Riesgos y deuda aceptada

- El conteo de preguntas es textual: una pregunta respondida pero no
  borrada sigue contando — correcto a propósito (si sigue en la visión,
  sigue abierta para el harness), pero exige la disciplina de limpiarlas.
- La fecha por mtime en archivos sin commitear varía entre máquinas: se
  acepta como degradación temporal hasta el primer commit.
- La sección "Contexto" del Studio es lectura pura en esta versión; acciones
  (marcar pregunta respondida, disparar al analista) quedan para cuando el
  cockpit tenga plantillas de prompts.
