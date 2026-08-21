# Spec: HoomAI Studio v2 — acciones, aprobación de specs e intake

Estado: BORRADOR — pendiente de aprobación humana.
Depende de: `studio-v1-dashboard.md` (criterios 1 a 10) integrada.
Tarea sugerida: `hoom task start studio-v2-acciones`.

## Objetivo

Convertir el Studio de espejo en **control remoto del harness**: botones que
mapean 1:1 a verbos del CLI (verificar, crear tarea, cerrar tarea), subida de
documentos al intake, y el flujo donde la UI supera al terminal — la
**aprobación humana del spec**: leerlo renderizado con sus CA-n, comentarlo y
apretar "Aprobar". Regla de diseño inviolable: si un botón necesita lógica que
el CLI no tiene, primero se agrega el verbo al CLI y después se le pone botón.
Por eso esta tarea incluye el verbo nuevo `hoom spec approve`/`hoom spec
status`, que registra la aprobación atada al hash del contenido — la misma
filosofía de la huella: aprobar A y editar a B invalida la aprobación.

## No-goals

- Editor de código, terminal embebida, gestión de archivos (no-goal PERMANENTE
  del Studio).
- Ejecutar agentes o hablar con modelos desde el Studio: el Studio opera el
  harness; los agentes se orquestan desde la CLI de IA de cada uno.
- `git merge` / operaciones de Git desde la UI: `task done` deja la rama
  lista, integrarla sigue siendo un acto humano en la terminal.
- Autenticación multiusuario/SSO: un token de sesión de operador único.
- Edición del spec desde la UI: se comenta y se aprueba/rechaza; corregirlo es
  trabajo del arquitecto en su archivo.

## Contratos

Verbos CLI nuevos (primero; el server los reusa):

- `hoom spec approve <ruta>` — registra la aprobación como archivo append-only
  `.hoom/approvals/<slug>_<hash8>.json`: `{spec, sha256, approved_by (git
  user), approved_at}`. Re-aprobar el mismo contenido es no-op informado.
- `hoom spec status <ruta>` — imprime `aprobado|no-aprobado|invalidado`
  (invalidado = existe aprobación pero el SHA-256 actual del spec no
  coincide). Exit 0 solo con aprobación vigente, 1 en los otros dos casos,
  para poder gatearlo por script o por contrato de agente.

API HTTP de acciones (todo POST exige header `X-Hoom-Token`; `hoom serve`
imprime el token una sola vez al arrancar). Cada endpoint invoca la MISMA
función interna que su verbo CLI:

- `POST /api/verify` `{full?, gates?, spec?}` — corre verify y devuelve el
  veredicto JSON; el veredicto se persiste en `.hoom/verdicts/` igual que una
  corrida CLI.
- `POST /api/tasks` `{slug}` — `hoom task start`.
- `POST /api/tasks/{slug}/done` — `hoom task done`; los rechazos del CLI se
  devuelven con su mensaje verbatim.
- `POST /api/intake` (multipart) — guarda el documento bajo `.hoom/intake/`
  (basename saneado + fecha), único directorio donde este endpoint escribe.
- `GET /api/specs` y `GET /api/specs/{name}` — markdown crudo + la lista de
  CA-n extraída con la MISMA regex de `spec_lint` + estado de aprobación.
- `POST /api/specs/{name}/approve` — mismo efecto que `hoom spec approve`.
- `POST /api/specs/{name}/review` `{comments}` — persiste los comentarios en
  `.hoom/specs/<name>.review.md` (versionado en Git; el arquitecto lo lee).

## Casos límite y errores esperados

- Dos verify a la vez sobre el mismo árbol: lock in-process; el segundo POST
  recibe 409 con referencia a la corrida en curso.
- Spec aprobado y editado después: el estado pasa a `invalidado` en el CLI y
  en la UI; aprobar de nuevo genera un registro nuevo (append-only, historial
  completo de aprobaciones).
- `task done` con worktree sucio, veredicto rojo o drift: el endpoint responde
  error con el mensaje exacto del CLI y la tarea NO se cierra.
- Upload con nombre malicioso (`../../x`): se reduce a basename; nada escapa
  de `.hoom/intake/`.
- Upload que no es markdown: se guarda igual y la UI sugiere la conversión
  (`pandoc`), como en el tutorial.
- POST sin token o con token inválido: 401 y cero efectos secundarios.
- Aprobar un spec que no pasa `spec_lint`: se permite pero la UI lo advierte
  (la aprobación es humana; el gate lo atrapará en el verify).

## Criterios de aceptación verificables

(Continúa la numeración de la v1: aquí CA-11..CA-20.)

- CA-11: `hoom spec approve <ruta>` escribe en `.hoom/approvals/` un registro
  con el SHA-256 del contenido del spec y el autor de Git; re-aprobar el mismo
  contenido no duplica el registro y lo informa.
- CA-12: `hoom spec status <ruta>` sale con exit 0 solo cuando existe una
  aprobación cuyo hash coincide con el contenido actual; `no-aprobado` e
  `invalidado` salen con exit 1 y se distinguen en el mensaje.
- CA-13: `POST /api/verify` produce un veredicto con el mismo esquema que
  `hoom verify --json` y lo persiste en `.hoom/verdicts/` como cualquier
  corrida CLI.
- CA-14: un `POST /api/tasks/{slug}/done` que el CLI rechazaría (rojo, drift o
  cambios sin commitear) responde error con el mismo mensaje del CLI y la
  tarea queda abierta.
- CA-15: con un verify en curso, un segundo `POST /api/verify` recibe 409;
  nunca corren dos verify concurrentes sobre el mismo árbol.
- CA-16: `POST /api/intake` guarda el archivo únicamente bajo `.hoom/intake/`
  con basename saneado: un nombre con `../` no escribe fuera del directorio.
- CA-17: `GET /api/specs/{name}` devuelve el markdown y la lista de CA-n
  extraída con la misma regex que usa `spec_lint` (una sola definición de
  criterio en el código).
- CA-18: aprobar desde la UI produce el mismo registro que `hoom spec
  approve`; si el spec se edita después, tanto `hoom spec status` como
  `GET /api/specs/{name}` lo reportan `invalidado`.
- CA-19: todo POST sin `X-Hoom-Token` válido responde 401 sin ejecutar nada;
  el token se genera por sesión y se imprime una sola vez al arrancar serve.
- CA-20: `POST /api/specs/{name}/review` persiste los comentarios en
  `.hoom/specs/<name>.review.md`, versionable en Git.

## Decisiones de diseño y alternativas descartadas

- **Aprobación atada al hash del contenido** — descartado un flag booleano en
  el front-matter del spec: sería editable sin invalidarse. El hash reusa la
  filosofía de la huella: lo aprobado es un contenido exacto, no un nombre.
- **Registros de aprobación append-only en `.hoom/approvals/`** — descartado
  mutar el spec o un estado central: cero conflictos de merge por
  construcción, historial de aprobaciones completo, viaja en Git.
- **Server invoca funciones internas, no shell-out al binario** — mismo
  proceso, mismos structs, sin dependencia del PATH; la regla 1:1 se sostiene
  porque endpoint y verbo comparten la misma función.
- **Token de sesión simple** — descartada auth de usuarios: el Studio es una
  herramienta de operador único local; el token evita que cualquier página web
  local dispare acciones (CSRF), no pretende ser SSO.
- **Comentarios como sidecar `.review.md` versionado** — descartado un sistema
  de comentarios con base de datos: el arquitecto lee markdown del repo, igual
  que todo lo demás en el harness.

## Riesgos y deuda aceptada

- La aprobación todavía no es exigida por el binario: un writer indisciplinado
  podría arrancar sin `spec approve`. Hoy lo exige el contrato de agentes
  (orquestador espera aprobación); la deuda declarada es un futuro gate
  `spec_approved` en `hoom verify --spec` que ponga en rojo la falta de
  aprobación vigente.
- Con `--addr` expuesto a la red, el token viaja en claro (HTTP): deuda
  aceptada y documentada; el uso previsto es loopback o túnel SSH.
- Un verify largo deja el lock tomado: mitigado con el 409 informativo y el
  polling del estado; sin cola de trabajos (descartada por simplicidad).
