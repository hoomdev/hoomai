# Spec: hoom cockpit — el layout del arbitro sobre tmux/zellij (fase 2)

Estado: BORRADOR — pendiente de aprobación humana.
Continúa la fase 1 (eventos vivos + status --watch), que es su motor.

## Objetivo

Un solo comando que arma el puesto de trabajo completo: `hoom cockpit` abre
la CLI de IA del usuario en un pane real de terminal y `hoom status --watch`
al costado — la sensación de cabina, sin construir una cabina. hoom NO
emula terminales ni reinventa multiplexores: detecta tmux o zellij en el
PATH (el mismo patrón de detección que los providers) y compone la sesión;
la emulación la hace el multiplexor, que es perfecto, y por eso CUALQUIER
CLI de IA corre intacta adentro. Integra con las tareas: `--task <slug>`
monta el cockpit dentro del worktree aislado de la tarea, que es como el
paralelismo de sesiones debe usarse (una sesión por worktree, jamás dos
writers sobre el mismo árbol).

## No-goals

- Emulador de terminal o TUI propio para hospedar la CLI de IA: decidido y
  cerrado — una IA renderizada a medias se siente rota; tmux/zellij ya lo
  hacen perfecto.
- Orquestar o dirigir a la IA desde el cockpit: hoom lanza y muestra; la
  orquestación del trabajo vive en la CLI (los contratos de roles ya la
  gobiernan). hoom sigue siendo el árbitro, no la cabina.
- Gestionar el ciclo de vida del multiplexor (matar sesiones, reconfigurar
  al vuelo): el usuario opera tmux/zellij con sus propios comandos.
- Soporte Windows nativo: tmux/zellij no existen ahí; el comando degrada
  con la acción exacta (watch manual en una segunda terminal).
- Providers desconocidos: el cockpit lanza las CLIs del registro existente
  (claude, opencode, codex, gemini); otras herramientas se abren a mano y
  el watch igual funciona al lado.

## Contratos

Verbo nuevo `hoom cockpit` (`internal/cockpitcmd`):

- `hoom cockpit [--provider <p>] [--task <slug>] [--mux tmux|zellij]`.
- Detección de multiplexor por PATH: tmux primero, zellij después; `--mux`
  fuerza uno. Sin ninguno instalado: error con la acción exacta (instalar
  tmux o zellij, o abrir una segunda terminal con el watch) y NADA se
  lanza a medias.
- Provider: `--provider` valida contra el registro (`providers.Lookup` +
  binario en PATH, mismo mensaje de error que `hoom run`). Sin
  `--provider`: si hay EXACTAMENTE una CLI instalada se usa esa; con cero
  o varias, error que lista lo detectado y pide el flag — nunca se
  adivina.
- Layout tmux: sesión con dos panes sobre el directorio de trabajo — la
  CLI de IA en el principal (~70%) y el binario hoom EN EJECUCIÓN (ruta
  absoluta de os.Executable, inmune al PATH de la sesión) corriendo
  `status --watch` en el lateral. Foco inicial en el pane de la IA.
- Sesión nombrada estable: `hoom-<proyecto>` (slug), `hoom-<proyecto>-<slug>`
  con `--task`. Si la sesión ya existe se RE-ADJUNTA (idempotente, nunca
  duplica); si el usuario ya está dentro de tmux, se cambia de cliente en
  vez de anidar sesiones.
- `--task <slug>`: ambos panes con cwd en `.hoom/worktrees/<slug>`; tarea
  inexistente = error con la acción ("hoom task start <slug>").
- Layout zellij: se genera el KDL equivalente en `.hoom/cache/` (unico
  lugar donde cockpit escribe) y se lanza la sesión; si ya existe, se
  adjunta.
- Ejecución en foreground con los streams de la terminal heredados: al
  salir del multiplexor, hoom cockpit termina.

## Casos límite y errores esperados

- Sin tmux ni zellij: error accionable, exit 1, cero procesos lanzados.
- Cero CLIs de IA instaladas: error apuntando a `hoom providers`.
- Varias CLIs instaladas sin `--provider`: error listándolas.
- Sesión ya existente con el mismo nombre: re-attach, no una segunda
  sesión ni panes duplicados.
- Usuario ya dentro de tmux: switch-client, jamás sesión anidada.
- `--task` de una tarea que no existe: error con la acción exacta.
- `--mux zellij` sin zellij instalado: error honesto (no cae a tmux en
  silencio: un flag explícito no se desobedece).

## Criterios de aceptación

- CA-81: sin tmux ni zellij en PATH, `hoom cockpit` falla con la acción
  exacta (instalar uno, o watch manual) sin ejecutar ningún proceso.
- CA-82: con tmux, la sesión nueva se compone con dos panes sobre el
  directorio del proyecto: el binario del provider en el principal y
  `status --watch` en el lateral, y se adjunta al final.
- CA-83: sin `--provider` y con exactamente una CLI instalada se usa esa;
  con cero o con varias, error que lista lo detectado — jamás se adivina.
- CA-84: el nombre de sesión es estable por proyecto; si la sesión existe
  se re-adjunta sin crear panes nuevos, y dentro de tmux se usa
  switch-client en vez de attach.
- CA-85: `--task <slug>` monta ambos panes con cwd en el worktree de la
  tarea y el nombre de sesión incluye el slug; tarea inexistente = error
  que nombra `hoom task start`.
- CA-86: con zellij (o `--mux zellij`), el layout KDL se genera en
  `.hoom/cache/` con los dos panes equivalentes y se lanza la sesión.
- CA-87: cockpit no escribe NADA fuera de `.hoom/cache/` y no altera la
  huella del cambio.
- CA-88: el pane del watch invoca la ruta absoluta del binario hoom en
  ejecución, no un `hoom` resuelto por el PATH de la sesión.

## Decisiones

- tmux antes que zellij en la autodetección: es el más ubicuo; `--mux`
  existe para quien prefiera el otro. Un `--mux` explícito que no se puede
  cumplir es error, no fallback silencioso.
- Subproceso en foreground con streams heredados en vez de exec(2):
  portable, y deja a hoom limpiar y reportar si el multiplexor falla al
  arrancar.
- Construcción del plan (argv por paso) separada de la ejecución: los
  tests verifican el plan determinísticamente y los binarios falsos en
  PATH verifican la ejecución — sin tmux real en CI.
- La CLI de IA se lanza INTERACTIVA (binario pelado del registro), no en
  modo headless: el cockpit es el puesto del humano; el headless ya lo
  cubre `hoom run`.
- Un pane de watch por sesión y no por tarea global: el watch muestra el
  estado del cwd donde corre, que con `--task` es el worktree — la vista
  correcta para esa sesión.

## Riesgos y deuda aceptada

- Sintaxis de split porcentual de tmux (`-l 30%`) requiere tmux moderno
  (3.1+, 2020); versiones más viejas fallan con el error de tmux a la
  vista. Deuda aceptada antes que detectar versiones.
- El CLI de zellij cambia entre versiones más que tmux; si la invocación
  falla, el error de zellij se muestra tal cual (honesto) y el usuario
  conserva el watch manual. Se revisará contra la versión estable al
  empaquetar.
- Nombres de sesión con proyectos homónimos en carpetas distintas
  colisionan: re-attach a la sesión del otro proyecto; mitigable a futuro
  con hash del path en el nombre.
