# Spec: la evidencia se mide en el disco, no en lo que git lista

Estado: BORRADOR — pendiente de aprobación humana.
Depende de: `quien-cierra-un-hallazgo.md` integrada (PR #41).
Tarea: `hoom task start evidencia-en-disco`.

## Objetivo

La evidencia de hoom vive en `.hoom/findings/` (hallazgos y resoluciones),
`.hoom/verdicts/` (veredictos) y `.hoom/approvals/` (aprobaciones humanas).
El gate de territorio la protege con un piso append-only: un rol puede
agregar un hallazgo o un veredicto, pero no tocar uno que ya existía, y
nunca escribir una aprobación. Ese piso mira solo lo que git lista
(`Snapshot.Touched`, de `gitx.Touched`). `Snapshot.Evidence` sí recorre el
disco, pero solo anota qué archivos existen, no su contenido.

Eso deja dos agujeros, anteriores a `quien-cierra-un-hallazgo` y encontrados
por su refutador y su test-writer:

1. **Un archivo cualquiera bajo `.hoom/findings/` pasa como legítimo.**
   Cualquier creación ahí es "trabajo legítimo (finding add)". Un rol (hasta
   el reviewer) puede crear `.hoom/findings/.gitignore` con `*.json` y
   esconderle a git los hallazgos, incluidos los que registra el propio gate.
2. **Una evidencia que git ignora se puede editar sin que el gate la vea.**
   Un rol que hace que git ignore los hallazgos (`.gitignore` de la raíz, un
   `.gitignore` en `.hoom/findings/` o `.git/info/exclude`) puede bajar un
   hallazgo de `high` a `low` para que deje de bloquear `findings_open`, o
   borrarlo, o editar un veredicto. La ruta no aparece en `Touched` y el piso
   nunca la mira.

`quien-cierra-un-hallazgo` ya compara en disco las resoluciones creadas.
Esta spec extiende la misma idea a toda la evidencia: el piso se mide en el
disco, por forma y por contenido, diga lo que diga git.

## No-goals

- Validar el contenido de un hallazgo o un veredicto nuevo (que el JSON sea
  coherente): lo hacen `finding.List` y `verdict` al leerlo.
- Cambiar quién puede crear qué: un rol sigue pudiendo agregar hallazgos y
  veredictos; las resoluciones siguen la regla de `quien-cierra-un-hallazgo`.
- Proteger la evidencia de una persona con acceso al repo fuera de una
  corrida: eso queda en Git.
- Los procesos hijos que escriben después de la foto (otra tarea).

## Contratos

### La foto

`agentcmd.Snapshot` gana `Huellas map[string]string`: para cada archivo bajo
`.hoom/{verdicts,findings,approvals}` en el disco (también los que git
ignora), su ruta relativa y el sha256 de su contenido. `Take` lo llena
siempre (un mapa vacío si no hay evidencia). Una foto sin `Huellas` (nil,
armada a mano en un test) no aplica las reglas de abajo.

### El piso en el disco

En la corrida de cualquier rol (el refutador incluido), comparando las dos
fotos de cada árbol que la corrida pudo escribir (el suyo y, en una corrida
ciega, el real):

- **Una evidencia que existía y cambió o desapareció** es manipulación:
  `la evidencia es append-only: <ruta> cambio durante el run` o
  `... desaparecio durante el run`. Vale aunque git la ignore.
- **Un archivo creado** es legítimo solo con la forma que escribe hoom:
  - bajo `.hoom/findings/`: `<id>.json` (un hallazgo) o `<id>.res.json` (una
    resolución), con `<id>` = `AAAAMMDDTHHMMSS_` + 6 hex;
  - bajo `.hoom/verdicts/`: `<id>.json` con `<id>` =
    `AAAA-MM-DDTHH-MM-SSZ_` + 8 hex.

  Cualquier otro (un `.gitignore`, un dotfile, otro nombre, un archivo en un
  subdirectorio) es manipulación:
  `bajo .hoom/<dir> solo se crean archivos con la forma que escribe hoom: <ruta>`.
- **Bajo `.hoom/approvals/`, toda creación** es manipulación (como hoy cuando
  git la lista), aunque git la ignore.

Una ruta que el piso de hoy ya marcó como manipulación no se duplica. La
regla de las resoluciones de `quien-cierra-un-hallazgo` sigue igual (y ve
igual que esta el árbol real de una corrida ciega).

## Casos límite y errores esperados

- `hoom finding add` dentro de una corrida: crea `<id>.json` con la forma
  correcta; legítimo, como hoy.
- `hoom verify` dentro de una corrida (un writer que verifica): crea un
  veredicto con la forma correcta; legítimo.
- Un hallazgo que el gate registra por una violación: se escribe después de
  la segunda foto; no es parte de la corrida.
- Un rol que borra un hallazgo que git ignora: manipulación (desapareció).
- Un rol que reescribe un hallazgo con el mismo contenido byte a byte: no es
  cambio.
- Un `.gitignore` que el rol pone en `.hoom/findings/`: manipulación aunque
  todavía no esconda nada.
- Corrida ciega: un cambio a la evidencia del árbol real (fuera de la
  cuarentena) es manipulación además de la fuga.

## Criterios de aceptación

- CA-440: `Take` fotografía en `Huellas` el sha256 de cada archivo bajo `.hoom/{verdicts,findings,approvals}` en el disco, incluidos los que git ignora (por el `.gitignore` de la raíz, uno en `.hoom/findings/` o `.git/info/exclude`); sin evidencia, `Huellas` es un mapa vacío.
- CA-441: en la corrida de cualquier rol, un hallazgo o un veredicto que existía y cambió (por ejemplo, la severidad de un hallazgo `high` bajada a `low`) o desapareció es una violación de manipulación aunque git lo ignore, con su hallazgo `high` del gate; `findings_open` sigue bloqueando en la corrida siguiente; reescribirlo con el mismo contenido no es violación.
- CA-442: en la corrida de cualquier rol, un archivo creado bajo `.hoom/findings/` o `.hoom/verdicts/` sin la forma que escribe hoom (un `.gitignore`, un dotfile, otro nombre, un subdirectorio) es manipulación con `bajo .hoom/<dir> solo se crean archivos con la forma que escribe hoom`, aunque git lo ignore; un hallazgo de `hoom finding add` y un veredicto de `hoom verify` siguen siendo legítimos; bajo `.hoom/approvals/` toda creación es manipulación aunque git la ignore.
- CA-443: en una corrida ciega, las mismas reglas valen para el árbol real; y en `hoom review`, un reviewer que crea `.hoom/findings/.gitignore` o edita un hallazgo ignorado termina `NO ENTREGABLE` por territorio.

## Decisiones

- **El contenido, no la fecha.** Un hash del contenido dice si la evidencia
  cambió sin depender de mtimes ni de git; un archivo reescrito igual no
  es un cambio.
- **La forma, no una lista de permitidos por rol.** hoom sabe qué nombres
  escribe (`newID` de `finding`, el id del veredicto); todo lo demás bajo
  esos directorios no lo escribió hoom.
- **Para todos los roles, el refutador incluido.** El refutador puede crear
  resoluciones, pero nadie dentro de una corrida edita o borra evidencia.
- **Fotos sin `Huellas` no aplican la regla.** Así los tests que arman fotos
  a mano siguen describiendo lo que describían; `Take` siempre la llena.

## Riesgos y deuda aceptada

- **Costo de la foto.** Cada corrida hashea toda la evidencia dos veces
  (hoy, cientos de archivos chicos). Es lineal en la evidencia; si crece
  mucho, se puede cachear por mtime y tamaño (otra spec).
- **Procesos hijos que escriben después de la segunda foto.** No los ve
  nadie; es la tarea de los grupos de procesos.
- **El hallazgo del gate puede no persistirse** (error de `Register`
  descartado): la otra tarea.
