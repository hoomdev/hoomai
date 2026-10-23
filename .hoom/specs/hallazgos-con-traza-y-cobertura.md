# Spec: hallazgos con traza, por validar y cobertura de la review

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start hallazgos-con-traza-y-cobertura`.
Origen: análisis del skill de auditoría de seguridad de Cloudflare
(`cloudflare/security-audit-skill`, MIT, 2026-09-23). Su modelo de datos y
sus reglas coinciden con la filosofía de hoom (verificador fresco que
refuta, presupuesto, corrida incompleta con razón exacta) y les falta
exactamente a los hallazgos de hoom tres cosas: un estado para lo que no se
puede probar, una traza verificable y saber qué parte del cambio no miró
nadie. No se copian sus guías ni su orquestación de seis fases.

## Objetivo

1. **Estado `por-validar`.** Un reviewer que encuentra algo sospechoso y no
   puede probar el hecho decisivo lo registra sin severidad, con el
   bloqueador exacto y un plan para validarlo. Hoy está obligado a inventar
   una severidad. `findings_open` no cuenta un `por-validar` (no tiene
   severidad) y `hoom finding list`, `status` y el tablero lo muestran
   aparte.
2. **Traza verificable.** Un hallazgo puede llevar `traza`: tramos
   `entrada`, `propagacion` y `efecto` con archivo y línea. `hoom finding lint`
   comprueba que cada archivo existe y que cada línea existe en la huella del
   árbol del hallazgo. Un hallazgo con traza que no resuelve es inválido.
3. **Cobertura de la review.** El registro de review (`.hoom/reviews/`) lista
   las rutas del diff que el reviewer declaró revisadas, y hoom marca la
   review como parcial si quedan rutas del diff sin revisar. El tablero no
   cuenta una review parcial como la review de la tarjeta.
4. **Anclas de severidad** en los contratos 06 (reviewer) y 09 (refutador):
   la severidad nunca supera el daño demostrado, con una tabla de anclas para
   crítico, alto, medio y bajo. Es texto de contrato, sin código.
5. **No repetir lo refutado.** El pedido del reviewer lleva la lista de
   hallazgos refutados de la tarea cuyo archivo no cambió desde el refutado,
   con la instrucción de no volver a registrarlos sin evidencia nueva.

## No-goals

- Copiar las guías de clases de ataque de Cloudflare. Quien quiera la
  auditoría completa instala el skill en su CLI.
- Una orquestación de cazadores y críticos en paralelo.
- Cambiar los hallazgos existentes: todos los campos nuevos son opcionales y
  los 500+ hallazgos de hoy siguen válidos.

## Contratos

- `hoom finding add --estado por-validar --bloqueador "<texto>" --plan
  "<texto>"`: sin `--sev`; `--sev` junto con `por-validar` es exit 2.
- `hoom finding add --traza entrada:<archivo>:<linea>:<texto> --traza ...`:
  repetible, en orden; el primer tramo es `entrada` y el último `efecto`.
- Un `por-validar` se cierra como hoy (corregido o refutado) o se
  **confirma**: `hoom finding confirm <id> --sev <s> --evidencia "<texto>"`,
  que escribe un hallazgo nuevo con severidad que referencia al anterior y
  cierra el `por-validar`. Nada se edita.
- `hoom finding lint [<id>...]` exit 1 si una traza no resuelve.
- Registro de review: campo `revisadas` (rutas) y `parcial` (bool, lo
  calcula hoom con el diff).

## Casos límite y errores esperados

- Una traza con una línea que existía cuando se registró y ya no: el lint
  mira la huella del hallazgo, no HEAD.
- Un reviewer que no declara rutas revisadas: la review es parcial.
- Un `por-validar` sin bloqueador o sin plan: exit 2.

## Criterios de aceptación

- CA-463: `finding add --estado por-validar` exige bloqueador y plan, rechaza
  `--sev`, y el hallazgo no cuenta en `findings_open`.
- CA-464: `finding list`, `status` y el tablero muestran los `por-validar`
  aparte de los abiertos con severidad.
- CA-465: `finding confirm` escribe un hallazgo nuevo con severidad que
  referencia al `por-validar` y lo cierra; los dos archivos son inmutables.
- CA-466: `finding add --traza` guarda los tramos en orden y rechaza una traza
  que no empieza en `entrada` o no termina en `efecto`.
- CA-467: `finding lint` da exit 1 si un archivo o una línea de una traza no
  existe en la huella del hallazgo, y 0 si todas resuelven.
- CA-468: el registro de review trae `revisadas` y `parcial`; una ruta del
  diff fuera de `revisadas` deja `parcial: true`.
- CA-469: el tablero no cuenta una review parcial como la review de la
  tarjeta y lo dice en la tarjeta.
- CA-470: el pedido del reviewer lista los refutados de la tarea cuyo
  archivo no cambió, y no lista los de archivos que cambiaron.
- CA-471: los contratos 06 y 09 traen la tabla de anclas de severidad y la
  regla "nunca más que el daño demostrado".

## Decisiones

- **Sin severidad para lo que no se probó**, como `needs_validation` de
  Cloudflare. Asignarla es lo que hoy produce falsas alarmas.
- **Confirmar es un hallazgo nuevo**, no una edición: los hallazgos son
  append-only.
- **La traza se verifica contra la huella del hallazgo**, porque el código
  sigue cambiando después.

## Riesgos y deuda aceptada

- **El reviewer puede declarar rutas que no leyó.** La cobertura mide lo que
  declara, no lo que leyó; lo que sí agrega es que lo que no declara queda a
  la vista.
- **Nueve criterios** y cambios en `finding`, `reviewcmd` y `boardcmd`.
