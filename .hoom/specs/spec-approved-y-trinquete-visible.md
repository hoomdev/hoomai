# Spec: gate spec_approved + trinquete visible en status (coherencia)

Estado: BORRADOR — pendiente de aprobación humana.
Ítem chico de coherencia: dos deudas donde el binario todavía no cumple lo
que el sistema promete.

## Objetivo

(1) **El binario hace cumplir la aprobación humana.** Hoy `hoom verify
--spec` traza los criterios pero NO exige que el spec esté aprobado: la
aprobación la exige el contrato del orquestador, no el código. Es la misma
familia de bug ya corregida dos veces (el contrato lo prohíbe, la
implementación lo permite). Nace el gate `spec_approved`, requerido: spec
sin aprobación vigente = veredicto ROJO con la acción exacta. La
infraestructura ya existe (aprobación por hash de contenido); esto solo la
conecta al veredicto.

(2) **El trinquete se ve.** El gate y la línea base existen, pero ninguna
vista los muestra: `hoom status` (texto, `--json` y por herencia `--watch`)
gana la sección trinquete — cada métrica con su base congelada, dirección y
último movimiento. La recompensa visible del sistema: mirar la curva subir.

## No-goals

- Trinquete en el Studio (hoom serve): fase siguiente; esta es la vista CLI.
- Que `status` ejecute los comandos de métricas: mostrar la base es leer el
  archivo; medir es trabajo de `verify --full`. `status` sigue siendo
  inofensivo byte a byte.
- Exigir aprobación en verifies SIN `--spec`: atar un spec a una corrida
  sigue siendo opt-in; una vez atado, se exige completo (traza Y
  aprobación).
- Aprobar desde el gate o auto-aprobar: la aprobación es humana y explícita
  (`hoom spec approve`); el gate solo la verifica.

## Contratos

Gate `spec_approved` (sintético, required, scope spec, solo con `--spec`):

- Aprobación vigente (hash del contenido actual coincide con un registro):
  PASS con quién, cuándo y sha en la nota.
- Sin aprobación registrada: FAIL con la acción exacta
  (`hoom spec approve <ruta>`).
- Aprobación invalidada (el spec se editó después de aprobarse): FAIL que
  lo dice explícitamente y pide revisar y re-aprobar.
- Error de lectura: ERROR fail-closed.
- Emite eventos vivos como cualquier gate y cuenta en el total declarado
  de la corrida.

Sección trinquete en `hoom status` (`internal/statuscmd`):

- Snapshot gana `ratchet`: por métrica, nombre, base congelada (o "sin
  congelar"), dirección, tolerancia y último movimiento del history (kind,
  fecha, from → to).
- Sin `.hoom/ratchet.json` o sin métricas: la sección lo dice con la
  acción (`hoom ratchet init`).
- Archivo ilegible: la sección lo rotula honesto ("ilegible: ...") sin
  romper el snapshot — status informa, nunca bloquea.
- `--json` expone el mismo dato; texto y JSON salen del mismo Build.

## Casos límite y errores esperados

- Spec aprobado y luego editado (aunque sea un espacio): FAIL de
  `spec_approved` por construcción (hash), con mensaje que distingue
  "editado tras aprobar" de "nunca aprobado".
- Re-aprobar el spec editado: el gate vuelve a PASS sin tocar nada más.
- Verify sin `--spec`: el gate no existe; cero cambios.
- Métrica declarada pero aún sin base congelada: la sección la muestra
  como "sin congelar (corre hoom verify --full)".
- History vacío con base presente (archivo escrito a mano): se muestra la
  base sin último movimiento, sin inventar historia.

## Criterios de aceptación

- CA-99: con `--spec` y aprobación vigente, el gate `spec_approved` es
  PASS y su nota trae quién aprobó, cuándo y el sha corto.
- CA-100: con `--spec` y sin aprobación registrada, `spec_approved` es
  FAIL nombrando la acción exacta (`hoom spec approve`) y el veredicto es
  ROJO.
- CA-101: un spec editado después de aprobado da FAIL explicando que la
  aprobación quedó invalidada por el hash; re-aprobar lo devuelve a PASS.
- CA-102: sin `--spec` el gate no existe en el veredicto.
- CA-103: `spec_approved` emite gate_start/gate_end en los eventos vivos y
  suma en el total de gates declarado en verify_start.
- CA-104: `hoom status` muestra la sección trinquete con cada métrica: base
  congelada, dirección y último movimiento (kind y from → to); sin línea
  base declara la acción (`hoom ratchet init`).
- CA-105: la sección trinquete jamás ejecuta comandos de métricas ni
  modifica nada: `.hoom` queda byte a byte idéntico tras el status.
- CA-106: `hoom status --json` expone el mismo dato del trinquete
  (métricas con base, dirección y último movimiento).
- CA-107: una línea base ilegible se rotula "ilegible" en la sección sin
  romper el snapshot ni el resto de las secciones.

## Decisiones

- `spec_approved` como gate separado de spec_lint/spec_trace y no como
  issue de lint: la aprobación es un estado EXTERNO al contenido (vive en
  `.hoom/approvals/`), y su fallo pide una acción humana distinta a
  corregir el spec.
- El gate se construye en verifycmd con `approval.Status` (el paquete spec
  queda puro: analiza contenido, no estado del proyecto).
- La vista del trinquete lee SOLO el archivo (base + history): medir desde
  status duplicaría trabajo caro y violaría su contrato de solo lectura.
- Orden en el veredicto: spec_lint, spec_trace, spec_approved — primero el
  contenido, después la traza, después el estado humano.

## Riesgos y deuda aceptada

- Flujos existentes que corrían `verify --spec` sin aprobar pasarán a
  ROJO: es el comportamiento CORRECTO (el contrato siempre lo exigió),
  pero rompe hábitos; el mensaje del gate trae la acción exacta.
- El "último movimiento" sale del history interno; un archivo editado a
  mano sin history muestra menos contexto — aceptado, el diff de Git sigue
  siendo la auditoría completa.
- Studio sin la sección todavía: deuda declarada en no-goals y roadmap.
