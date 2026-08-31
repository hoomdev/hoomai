# Spec: eventos vivos del harness + hoom status --watch (fase 1 de visibilidad)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start eventos-vivos-y-status-watch`.

## Objetivo

Hoy el harness trabaja pero no se siente: `verify` es una caja negra hasta el
reporte final, y quien opera desde su CLI de IA (claude, opencode, codex,
gemini) no percibe al árbitro. Fase 1 de la visibilidad, 100% agnóstica de
IA: (1) hoom emite sus propios **eventos vivos** a `.hoom/cache/` mientras
verifica — es el binario de hoom quien los escribe, lo invoque quien lo
invoque, así que funcionan con cualquier IA presente o futura; (2) nace
`hoom status`, la foto completa del proyecto en un comando, y
`hoom status --watch`, el panel vivo para una segunda terminal: check, gates
corriendo con su tiempo, runs activos con su rol, tareas y hallazgos.

Los eventos cubren únicamente lo que aún no tiene artefacto (la corrida en
curso). Todo lo demás — veredictos, hallazgos, tareas, aprobaciones — ya
vive en disco y el status lo lee directo: los artefactos siguen siendo la
única historia; los eventos son efímeros por diseño.

## No-goals

- El launcher de cockpit (tmux/zellij) y los adaptadores por provider
  (statusline, hooks): fases 2 y 3, sobre esta base.
- Instrumentar sesiones interactivas de las IAs: sin datos del provider no
  se muestra nada; jamás se simula actividad de roles.
- Frameworks TUI o dependencias nuevas: ANSI plano, cero deps fuera de yaml.
- Historial de eventos: la historia son los veredictos; el archivo de
  eventos se trunca en cada corrida y no viaja en Git.
- Dirigir agentes desde el panel: hoom muestra, nunca orquesta. La
  orquestación vive en la CLI de IA del usuario.
- Cambiar CUALQUIER comportamiento existente: esto es estrictamente aditivo.

## Contratos

Eventos (`internal/gates` + `internal/verifycmd`):

- Archivo `.hoom/cache/verify-live.jsonl` (cache/ ya está gitignoreado y
  fuera de la huella). Se TRUNCA al iniciar cada verify; un evento JSON por
  línea: `{schema:"hoom.live/v1", ts, kind, ...}` con kinds
  `verify_start` (gates, spec), `gate_start` (gate, scope),
  `gate_end` (gate, status, duration_ms, exit_code),
  `verify_end` (verdict, id, partial).
- `gates.Options` gana un emisor opcional (callback); `verifycmd.Run` lo
  conecta al archivo. Emisor nil = comportamiento idéntico al actual.
- Escritura best-effort: un fallo de escritura de eventos JAMÁS altera la
  ejecución de gates, el veredicto ni el exit code.

Verbo nuevo `hoom status` (`internal/statuscmd`):

- `hoom status`: snapshot one-shot legible — estado del check (mismo
  cerebro: `checkcmd.Run`), último veredicto (con su marca parcial), verify
  en curso si lo hay (leyendo los eventos), runs activos de `.hoom/runs/`
  con provider y último rol, tareas activas y conteo de hallazgos abiertos.
- `hoom status --json`: el mismo snapshot como JSON en stdout (mismo dato,
  otra piel), componiendo los JSON que ya existen.
- `hoom status --watch`: en TTY refresca el snapshot en el lugar (ANSI,
  intervalo ~1s, misma paleta de report.go); sin TTY imprime el snapshot
  una vez, avisa y sale 0 — sin escapes ANSI en logs de CI.
- `status` es SOLO LECTURA: no escribe ni modifica nada bajo `.hoom/`.

## Casos límite y errores esperados

- Proyecto virgen (sin veredictos, runs ni tareas): cada sección muestra su
  estado vacío con la acción exacta (ej. "ejecuta 'hoom verify'").
- Verify huérfano (proceso muerto a mitad de corrida): eventos sin
  `verify_end` y sin actividad reciente se rotulan "posible huérfano",
  nunca "corriendo" a secas. El próximo verify trunca y sana.
- Dos verify simultáneos en la misma raíz: el último en arrancar trunca y
  gana el archivo; degradación documentada, no error.
- `.hoom/cache/` no escribible: verify completa igual, mismo veredicto;
  el watch simplemente no ve la corrida en vivo.
- Run activo cuyo provider no emite delegación a roles: se muestra el run
  con "sin delegación visible", sin inventar rol.
- Ejecutado dentro del worktree de una tarea: el status refleja ese
  worktree (manifest.Find ya resuelve la raíz correcta).

## Criterios de aceptación

- CA-71: `hoom verify` emite a `.hoom/cache/verify-live.jsonl` los eventos
  verify_start, gate_start, gate_end (con status y duración) y verify_end
  (con veredicto y marca parcial), truncando el archivo al inicio de cada
  corrida.
- CA-72: con `.hoom/cache/` no escribible, verify termina con el MISMO
  veredicto y exit code que sin eventos: la emisión es best-effort.
- CA-73: emitir eventos no altera la huella del cambio: el
  ChangeFingerprint es idéntico antes y después de una corrida con
  eventos, y un check verde no se rompe por ellos.
- CA-74: `hoom status` muestra en un solo snapshot: estado del check,
  último veredicto con su marca parcial, verify en curso (si lo hay), runs
  activos con provider y último rol, tareas activas y hallazgos abiertos.
- CA-75: `hoom status --json` emite el mismo snapshot como JSON válido en
  stdout, incluyendo las mismas secciones del modo texto.
- CA-76: `hoom status --watch` sin TTY imprime el snapshot UNA vez sin
  escapes ANSI y sale con código 0, avisando que el modo vivo requiere
  terminal.
- CA-77: con un verify en curso, el status muestra cada gate con su estado
  vivo (corriendo/pass/fail/absent) y el tiempo transcurrido, derivado
  exclusivamente de los eventos.
- CA-78: una corrida con eventos sin verify_end y sin actividad reciente se
  rotula como posible huérfano en el status.
- CA-79: un run activo con eventos de delegación muestra su último rol; un
  run sin esos eventos muestra "sin delegación visible" — nunca un rol
  inventado.
- CA-80: `hoom status` es solo lectura: tras ejecutarlo, el contenido bajo
  `.hoom/` queda byte a byte idéntico.

## Decisiones

- Eventos solo para lo efímero (la corrida en curso); el resto se lee de
  los artefactos existentes: una sola fuente de verdad por dato, cero
  duplicación de estado.
- Un archivo truncado por corrida en vez de log rotativo: la historia ya
  la guardan los veredictos; así no hay crecimiento sin límite ni política
  de rotación que mantener.
- Snapshot + redibujo simple en el watch (no TUI incremental): trivial de
  mantener, imposible de corromper, y suficiente a ~1s de refresco.
- El rol visible sale de los runs de `hoom run` (el parser de providers ya
  normaliza la delegación); la visibilidad en sesiones interactivas llega
  en fase 3 con los adaptadores por provider — aquí sería simulación.
- Verbo `status` separado de `check`: check es un gate con exit code para
  scripts; status es una vista para humanos y jamás bloquea.

## Riesgos y deuda aceptada

- Formato de eventos v1 con campo `schema`: si crece, se versiona; los
  consumidores externos aún no existen, deuda barata hoy.
- El watch sondea archivos (~1s): costo trivial en proyectos reales; se
  acepta polling antes que meter fsnotify como dependencia.
- Detección de huérfanos por inactividad (umbral fijo): un gate legítimo
  extremadamente silencioso podría rotularse "posible huérfano"; el rótulo
  es honesto ("posible"), riesgo aceptado.
- ANSI en Windows: funciona en Windows Terminal moderno; consolas legacy
  quedan con el modo one-shot.
