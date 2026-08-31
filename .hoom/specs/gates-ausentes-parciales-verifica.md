# Spec: gate ausente real, veredictos parciales honestos y verificacion por comando

Estado: BORRADOR — pendiente de aprobación humana.
Origen: tres hallazgos reportados desde un proyecto Laravel real (API de
facturación) usando hoom como harness. Los tres son bugs/huecos de hoom, no
del proyecto verificado.

## Objetivo

Cerrar tres grietas entre lo que hoom PROMETE y lo que hoom HACE:

1. **Gate ausente roto ante `diff_cmd` de perfil.** El comentario que `hoom
   init` genera promete: "Un cmd vacio declara el gate AUSENTE". Pero si el
   perfil embebido aporta `diff_cmd` (ej. `mutation` en laravel), `verify`
   con diff activo ejecuta ese `diff_cmd` heredado igual → herramienta no
   instalada → exit 127 → gate ROJO en vez de AUSENTE amarillo. La promesa
   documentada debe cumplirse: sin `cmd`, no hay gate, punto.
2. **Fraude de veredicto por implementación.** `hoom verify --gate x`
   escribe un veredicto completo con los demás gates en `skipped`, y
   `hoom check` toma siempre el más reciente: una corrida diagnóstica de un
   solo gate se convierte en la referencia y puede dar VERDE apoyada en
   gates que nunca corrieron (o ROJO tapando un verde legítimo). Es
   exactamente lo que la regla 4b del Orquestador prohíbe por contrato;
   ahora el binario lo impide por construcción: los veredictos parciales se
   marcan y NUNCA son referencia.
3. **Criterios que no se verifican con tests.** `spec_trace` exige que cada
   CA-n aparezca en un archivo de test. Para ítems de tooling (criterios que
   se verifican con `composer`, `phpstan`, `git`, etc.) eso fuerza CAs
   circulares o a reescribir el spec. Nace el marcador "verifica: <comando>"
   (entre corchetes, en la línea del CA): el criterio se verifica ejecutando
   el comando (exit 0), con la misma filosofía de siempre — confianza en
   exit codes, no en narración.

## No-goals

- Cambiar la semántica de OMITIR un gate en `hoom.yaml`: omitir hereda el
  gate del perfil tal cual (esa es la gracia de los perfiles). Ausente se
  declara explícitamente con `cmd: ""`.
- Borrar o dejar de escribir los veredictos parciales: siguen siendo
  artefactos append-only útiles como diagnóstico; solo pierden autoridad
  como referencia de `check` / `task done`.
- Un DSL de verificación en el spec: el marcador es un comando shell por
  criterio, nada más. Sin variables, sin composición, sin retries.
- Migrar veredictos viejos en disco: se detectan por heurística
  retrocompatible, jamás se editan (append-only).

## Contratos

Manifiesto (`hoom.yaml`):

- `Gate.DiffCmd` pasa de `string` a `*string` (mismo patrón que `Cmd`):
  `diff_cmd: ""` explícito en el proyecto BORRA el `diff_cmd` del perfil;
  omitirlo lo hereda. Accessor nuevo `Gate.DiffCmdStr()`.
- Regla de ausencia en el runner: si `CmdStr()` resuelve vacío, el gate es
  AUSENTE aunque tenga `diff_cmd` (propio o heredado). `diff_cmd` es una
  optimización de `cmd`, no un gate independiente.

Veredictos:

- `Verdict.Partial bool` (`json:"partial,omitempty"`): lo setea `verify`
  cuando hubo selección `--gate`, junto a una nota visible en el veredicto.
- `verdict.LatestComplete(all)` es el ÚNICO selector de referencia: lo usan
  `hoom check`, `hoom task list/done` y el Studio (`/api/status`).
- Retrocompatibilidad: un veredicto viejo sin el campo se considera parcial
  si tiene algún gate `skipped` que no sea `spec_lint`/`spec_trace` (el
  runner solo emite `skipped` por selección `--gate`).
- `hoom check` con solo veredictos parciales: ROJO, razón `solo-parciales`,
  acción exacta "ejecuta 'hoom verify' completo (sin --gate)".

Spec (`spec_lint` / `spec_trace`):

- Marcador "verifica: <comando>" entre corchetes, en la MISMA línea que
  el token CA-n.
  `spec_trace` ejecuta cada comando declarado con `sh -c` en la raíz del
  proyecto (timeout 10 min): exit 0 = criterio trazado; distinto de 0 =
  FAIL nombrando CA, comando y exit code. Esos criterios no exigen test.
- Varios marcadores para el mismo CA: todos deben pasar.
- Un marcador en una línea sin CA-n es issue de `spec_lint` (marcador
  huérfano: no se sabe qué criterio verifica).

## Casos límite y errores esperados

- `cmd: ""` + perfil con `diff_cmd` + diff activo (el caso reportado):
  AUSENTE amarillo, sin ejecutar nada.
- `cmd: ""` + `--full`: AUSENTE (ya funcionaba; no debe romperse).
- Gate del perfil con `cmd` válido y `diff_cmd` válido, sin override:
  comportamiento intacto (diff cuando hay diff, full con `--full`).
- Veredicto parcial más nuevo que un completo verde con la misma huella:
  check VERDE (el parcial no pisa al completo, en ninguna dirección).
- Veredicto parcial verde más nuevo que un completo rojo: check ROJO.
- Worktree de tarea con solo veredictos parciales: `task done` se niega
  con acción exacta; `task list` muestra SIN-VEREDICTO(-completo).
- Comando declarado que no existe: FAIL con exit 127 visible.
- Spec sin ningún marcador: `spec_trace` se comporta exactamente
  como antes.

## Criterios de aceptacion

- CA-61: un gate con `cmd: ""` en el proyecto se reporta `absent` aunque el
  perfil aporte `diff_cmd` y exista diff activo; el comando heredado jamás
  se ejecuta.
- CA-62: `diff_cmd: ""` explícito en el proyecto borra el `diff_cmd` del
  perfil (campo puntero); omitir la clave lo hereda intacto.
- CA-63: un veredicto emitido con `--gate` lleva `partial: true` y una nota
  que lo declara diagnóstico; `report` lo rotula PARCIAL.
- CA-64: `hoom check` usa como referencia el veredicto completo más
  reciente: un parcial posterior no lo pisa ni para verde ni para rojo.
- CA-65: con únicamente veredictos parciales, `hoom check` es ROJO con
  razón `solo-parciales` y acción "hoom verify" completo.
- CA-66: el estado de tareas (`task list`/`task done`) aplica la misma
  regla de referencia (último veredicto completo del worktree).
- CA-67: un veredicto viejo sin campo `partial` pero con un gate `skipped`
  ajeno a `spec_lint`/`spec_trace` se trata como parcial (heurística
  retrocompatible).
- CA-68: un criterio que declara el marcador de comando en su línea queda
  trazado cuando el comando sale 0, sin exigir test que lo mencione.
- CA-69: si el comando declarado falla, `spec_trace` es FAIL nombrando el
  CA, el comando y el exit code.
- CA-70: un marcador de comando en una línea sin CA-n produce issue de
  `spec_lint`.

## Decisiones

- `diff_cmd` puntero en vez de un flag aparte "ausente": simetría exacta
  con `cmd`, cero vocabulario nuevo en el manifiesto.
- La ausencia se decide por `cmd` vacío (y no "cmd vacío Y diff_cmd
  vacío"): `diff_cmd` sin `cmd` no define capacidad completa — un gate que
  solo sabe correr sobre diff no puede sostener `--full` y sería una
  degradación silenciosa.
- Los parciales se marcan y se ignoran como referencia, en vez de no
  escribirse: append-only manda; la evidencia diagnóstica también es
  evidencia. Alternativa descartada: que `Finalize` nunca dé verde con
  skips — mentiría en rojo, y un parcial rojo tampoco debe ser referencia.
- El marcador EJECUTA el comando en vez de solo eximir del test: eximir
  crearía criterios que nadie verifica; ejecutar mantiene la regla de oro
  (exit codes, no narración). El spec ya es contenido aprobado por hash
  (`hoom spec approve`), mismo dominio de confianza que `hoom.yaml`.
- Marcador en la misma línea del CA: asociación determinística por línea,
  sin heurísticas de párrafo que inviten ambigüedad.

## Riesgos

- Cambio de tipo de `DiffCmd` rompe compilación de código externo que use
  el struct (no hay consumidores conocidos fuera del repo).
- La heurística CA-67 marcaría como parcial un veredicto viejo con un gate
  `skipped` de origen distinto a `--gate`; no existe tal origen en el
  código histórico del runner, riesgo aceptado.
- Comandos declarados largos pueden alargar `verify`; tope de 10 min
  por comando, deuda aceptada hasta que exista configuración por spec.
