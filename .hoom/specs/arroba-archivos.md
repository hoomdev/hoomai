# Spec: @archivo — autocompletado de rutas en el cockpit

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start arroba-archivos`.

## Objetivo

Que escribir `@` en los textareas del Studio sugiera rutas de archivos del
proyecto mientras se tipea — como en las CLIs de IA — y que al elegir una se
inserte `@ruta/completa/archivo`. Con Claude Code la referencia es funcional
de verdad: `claude -p` interpreta `@ruta` y adjunta el archivo como contexto
del agente; con otros providers queda como referencia textual útil. El
índice de archivos no se inventa: es `git ls-files`, que ya respeta
`.gitignore` por construcción — los ignorados (worktrees, runs, cache,
node_modules) jamás se sugieren.

## No-goals

- Devolver CONTENIDO de archivos: el endpoint lista rutas, punto. Leer
  archivos es trabajo del agente con sus permisos, no del autocompletado.
- Un verbo `hoom files`: el verbo ya existe y se llama `git ls-files`; el
  endpoint es una piel sobre git, como la huella. Duplicarlo en el CLI
  seria ceremonia sin valor.
- Indexado propio del disco (walk + cache): git ES el indice; un proyecto
  sin git degrada a no sugerir, no a inventar un indexador.
- Fuzzy matching sofisticado: subcadena con ranking simple alcanza; la
  heuristica compleja es deuda que no queremos.

## Contratos

- `internal/filesearch`: logica pura y testeable.
  - `List(root)` — union de `git ls-files --cached` y
    `git ls-files --others --exclude-standard` (tracked + nuevos sin
    ignorar), dedup, orden estable. Proyecto sin git: lista vacia, sin
    error.
  - `Match(files, query, limit)` — filtro case-insensitive con ranking:
    prefijo del NOMBRE del archivo > subcadena en el nombre > subcadena en
    el directorio; empates por orden alfabetico. Query vacia: los primeros
    `limit` en orden estable.
- `GET /api/files?q=<fragmento>` — hasta 20 rutas que matchean. Solo
  lectura, sin token (misma politica que el resto de los GET). Responde
  siempre un array JSON (vacio incluido).
- UI: los textareas del cockpit (pedido, continuar) y el de review de specs
  ofrecen el dropdown al detectar un token `@...` bajo el cursor:
  navegacion con ↑/↓, seleccion con Enter o Tab, cierre con Esc; al elegir
  se reemplaza el token por `@ruta` completa. El prompt viaja al provider
  tal cual, con las referencias adentro.

## Casos límite y errores esperados

- Proyecto sin git: sin sugerencias, textarea intacto, cero errores.
- Query sin matches: array vacio, dropdown cerrado.
- Query con caracteres de regex (`.`, `(`): se trata como texto literal.
- Archivos con espacios o unicode en el nombre: se listan tal cual.
- `@` seguido de espacio o solo: dropdown cerrado (token vacio no consulta
  con cada tecla del prompt normal).
- Repos grandes: `git ls-files` lee el indice de git (no camina el disco);
  el limite de 20 mantiene la respuesta chica.

## Criterios de aceptación verificables

(Continúa la numeración: aquí CA-47..CA-52.)

- CA-47: `GET /api/files?q=` responde hasta 20 rutas del proyecto que
  matchean el fragmento, y NUNCA incluye archivos ignorados por
  `.gitignore` (el indice es `git ls-files`).
- CA-48: el ranking prioriza prefijo del nombre del archivo sobre
  subcadena en el nombre, y esta sobre subcadena en el directorio; los
  empates se resuelven en orden alfabetico estable.
- CA-49: el endpoint devuelve exclusivamente RUTAS (un array JSON de
  strings), jamas contenido de archivos; query vacia devuelve los primeros
  20 en orden estable.
- CA-50: en un proyecto sin git la lista es vacia y no hay error — la
  degradacion es silenciosa y el resto del Studio no se entera.
- CA-51: los archivos nuevos sin commitear (untracked no ignorados)
  tambien se sugieren.
- CA-52: la UI embebida contiene el autocompletado atado a los textareas
  del cockpit y del review: deteccion del token `@`, dropdown con
  navegacion por teclado e insercion de la ruta completa.

## Decisiones de diseño y alternativas descartadas

- **git como indice** — descartado un walk propio con cache: git ya
  mantiene el indice, respeta ignores y es instantaneo; menos codigo,
  cero estado nuevo.
- **Sin verbo CLI nuevo** — la regla CLI-first pide que la LOGICA viva
  fuera de la UI, y vive (paquete Go testeable + git); duplicar
  `git ls-files` como `hoom files` no agrega scriptabilidad real.
- **Endpoint sin token** — lista rutas, no contenido; mismo perfil de
  riesgo que `GET /api/tasks` (que ya expone paths de worktrees).
- **Dropdown propio en vanilla JS** — descartadas librerias de
  autocompletado: la UI es autocontenida por contrato (criterio de la v1).
- **Ranking en el binario, no en el navegador** — testeable por los gates
  y heredable por una UI nativa, como el escenario de la v4.

## Riesgos y deuda aceptada

- El posicionamiento del dropdown usa la geometria del textarea, no la
  posicion exacta del caret (aparece bajo el textarea, no bajo la
  palabra): simplicidad primero; afinarlo es cosmetica futura.
- En providers que no interpretan `@ruta` (codex, gemini, opencode) la
  referencia es solo texto: degradacion heredada del provider, no nuestra.
- `git ls-files` no ve archivos dentro de submodulos: aceptado, casi nunca
  es lo que se quiere referenciar.
