# Spec: la historia del proyecto en el tablero

Estado: ENMIENDA 1 — pendiente de re-aprobación humana. La versión
aprobada (sha256 51722711) está implementada. Su review mostró que deshacer
el lote ante un error obliga a backfill a borrar archivos, y un borrado por
ruta puede llevarse un item que otro proceso tocó en el medio (hallazgos
1e8b0c, 039b75, a7d0b1). Henry decidió (2026-10-23) que backfill nunca
borra: la enmienda 1 dice que "nada a medias" vale por item, que un error a
mitad del lote deja los items ya creados y los lista, y que volver a correr
backfill completa el resto. Cambia el caso límite de `.hoom/items/` no
escribible.

Historia: aprobada el 2026-10-23 (sha256 51722711).
Tarea: `hoom task start historia-en-el-tablero`.
Origen: `hoom board doctor` en main (2026-10-22) da 32 problemas, todos
`evidencia-sin-item`: 25 specs anteriores a la cabina con veredictos y sin
item, y 7 tareas de corrección sin spec (`estabilizar-ci`,
`fixes-cabina-c1`, `c1b`, `c2`, `c3`, `c4`, `review-hallazgos-con-tarea`).
El doctor tiene razón: esa evidencia no tiene tarjeta. Lo que falta es una
forma de dársela sin fabricar lo que la cabina prohíbe: `hecho_en` lo escribe
solo `hoom task done` (C1), y ningún agente se marca hecho.

## Objetivo

Que el tablero muestre la historia del proyecto como tarjetas en Hecho
cuando la evidencia lo sostiene, y que el doctor deje de reportarla, con
tres piezas:

1. **`hoom item backfill [--dry-run] [--json]`**. Para cada tarea con
   evidencia y sin item (la misma lista que hoy produce el problema
   `evidencia-sin-item`), crea `.hoom/items/<slug>.yaml` con el campo nuevo
   `historico`. Nunca escribe `hecho_en` ni `commit_final`. Con `--dry-run`
   solo lista lo que crearía y por qué. Una tarea que no califica se lista
   con su motivo y no se crea.

2. **El campo `historico`** dice qué evidencia sostiene la tarjeta:
   - con spec: `historico: veredicto <id>`, el veredicto completo y verde
     más nuevo atado a `.hoom/specs/<slug>.md`, con `spec_approved` en pass y
     su commit integrado en la base;
   - sin spec (tarea de corrección): `historico: hallazgos`, cuando ningún
     hallazgo de la tarea de severidad `block_on` o mayor está abierto.

3. **Una regla de derivación nueva para Hecho**: un item con `historico`
   cae en Hecho si su evidencia lo sigue sosteniendo al derivar, con la
   marca visible `historica` en la tarjeta, el tablero y `--json`. Si la
   evidencia dejó de sostenerlo (el veredicto ya no está, un hallazgo
   bloqueante se reabrió), la tarjeta deriva como cualquier otra y el doctor
   lo dice.

## No-goals

- Escribir `hecho_en` o `commit_final` desde algo que no sea `task done`.
- Exigir la review de 4 lentes a las tarjetas históricas: el tablero no la
  pedía cuando se integraron. La marca `historica` lo deja a la vista.
- Inferir specs o títulos con un modelo. El título sale del primer encabezado
  del spec, sin "Spec: ", o del slug.
- Agrupar las tareas de corrección bajo la tarjeta de la spec que corrigen.
  Ver Decisiones.
- Tocar items existentes.

## Contratos

- Campo nuevo del item: `historico` (string, opcional). Valores:
  `veredicto <id>` o `hallazgos`. Lo escribe solo `hoom item backfill`. Está
  en el piso del sobre como el resto de `.hoom/items/`.
- `hoom item backfill` crea un item por tarea que califica, con `titulo`,
  `tipo` (`feature` con spec, `bug` sin spec), `prioridad: media`,
  `creado_por` (identidad git de quien lo corre), `creado_en` y `historico`.
  Argumentos estrictos (`internal/cliargs`). Exit 0 aunque alguna tarea no
  califique: lo que no crea lo lista con su motivo.
- Califica con spec: existe un veredicto completo verde atado al spec, con
  `spec_approved` pass, y su `git.commit` es ancestro de la base.
- Califica sin spec: tiene al menos un hallazgo con esa tarea y ninguno
  abierto de severidad `block_on` o mayor. Sin `block_on` en `hoom.yaml`,
  ninguno abierto de severidad `high`.
- Derivación: `hecho_en` se sigue mirando primero. Después, un item con
  `historico` que se sostiene da `hecho` con `Historica: true`. Uno que no
  se sostiene deriva normal y el doctor agrega el problema
  `historica-sin-sosten` con la acción exacta.
- `evidencia-sin-item` deja de reportar las tareas que tienen item, como hoy.

## Casos límite y errores esperados

- Spec sin ningún veredicto verde aprobado e integrado: no se crea; motivo
  "sin veredicto verde aprobado en la base".
- Spec aprobado y después editado: el veredicto viejo trae `spec_approved`
  pass de entonces. Califica por el veredicto, que es la foto de cuando se
  integró, y la tarjeta lo muestra.
- Tarea de corrección con follow-ups abiertos de severidad menor que
  `block_on`: califica; los follow-ups siguen abiertos y visibles.
- Un item que ya existe con ese slug: no se toca ni se reescribe.
- `.hoom/items/` no escribible, o un alta que falla a mitad del lote
  (disco lleno): error. Nada a medias por item: cada item se publica entero,
  como en `item add`. Los items que esa corrida ya creó quedan, completos y
  válidos, y la salida los lista; volver a correr backfill completa el resto,
  porque salta los que ya tienen item. Backfill nunca borra un item.
- Dos personas corren backfill en dos máquinas: crean el mismo archivo con
  contenido distinto en `creado_por` y `creado_en`; es un conflicto de git
  normal y gana el que se integra primero.

## Criterios de aceptación

- CA-446: `hoom item backfill --dry-run` lista cada tarea de `evidencia-sin-item`
  con "crearía" o con el motivo por el que no, y no escribe nada.
- CA-447: `hoom item backfill` crea `.hoom/items/<slug>.yaml` con
  `historico: veredicto <id>` para una tarea con spec que califica, y no
  escribe `hecho_en` ni `commit_final`.
- CA-448: para una tarea sin spec que califica escribe `historico: hallazgos`
  y `tipo: bug`.
- CA-449: una tarea con spec cuyo único veredicto verde no está integrado en
  la base, o no tiene `spec_approved` pass, no se crea y su motivo lo dice.
- CA-450: una tarea sin spec con un hallazgo `high` abierto (con
  `block_on: high`) no se crea y su motivo nombra el hallazgo.
- CA-451: `hoom board` pone en Hecho, con la marca `historica`, el item
  histórico que se sostiene; `--json` trae `historica: true`.
- CA-452: si el veredicto de un histórico desaparece o un hallazgo
  bloqueante de su tarea se reabre, la tarjeta deja Hecho y el doctor da
  `historica-sin-sosten` con su acción.
- CA-453: un item existente con el mismo slug no se modifica, y la salida lo
  dice.
- CA-454: en hoomai, después de `hoom item backfill`, `hoom board doctor` no
  da ningún `evidencia-sin-item` de las 32 tareas de hoy que califiquen, y
  lista las que no.
- CA-455: `backfill` rechaza argumentos de más con exit 2 y cero efectos.

## Decisiones

- **Un campo nuevo y no `hecho_en`.** `hecho_en` significa "cerrado por
  `task done` con sus condiciones". Las tarjetas históricas no pasaron por
  ahí, y escribirlo sería afirmar algo que no ocurrió.
- **Hecho por evidencia, no por declaración.** `historico` no basta: dice
  dónde mirar. La tarjeta queda en Hecho mientras esa evidencia exista y se
  sostenga, igual que cualquier otra columna.
- **Las tareas de corrección son tarjetas propias.** Atarlas a la tarjeta que
  corrigen pediría saber cuál es, y `estabilizar-ci` no corrige ninguna. Si
  más adelante se quiere agrupar, es un campo `de:` en otra spec.
- **Sin review exigida.** Exigirla convertiría la historia en deuda impagable
  y el doctor seguiría rojo para siempre. La marca `historica` es la forma
  honesta de decir que no pasó por la cabina.

## Riesgos y deuda aceptada

- **Se puede correr backfill sobre una tarea nueva para saltear la cabina.**
  Mitigación: la regla exige un veredicto verde aprobado e integrado, lo
  mismo que una tarjeta necesita para Tu aceptación, y la marca `historica`
  queda a la vista para siempre. Opción a decidir: que backfill solo
  considere evidencia anterior a la fecha del primer item del proyecto.
- **Las tareas de corrección sin spec quedan en Hecho por hallazgos
  cerrados**, sin un veredicto propio atado. Su código está cubierto por el
  veredicto de main del momento, que no se ata a la tarea.
- **Diez criterios** para un verbo y una regla: se implementa en un worktree
  con su propio veredicto.
