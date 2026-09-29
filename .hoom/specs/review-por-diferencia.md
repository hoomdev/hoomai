# Spec: la review de lo que cambió desde la última review

Estado: ENMIENDA 1 — pendiente de re-aprobación humana. La versión
aprobada (sha256 9fe30ea5) está implementada. Su primera review encontró un
ciclo entre `--delta` y el tablero: el tablero pedía `--delta` por la última
cadena de 4 lentes, pero `--delta` encadenaba con un registro más nuevo que
el tablero no cuenta, y la tarjeta nunca salía de Review. La enmienda 1 hace
que el tablero pida `--delta` solo cuando el registro con el que `--delta`
encadenaría lo dejaría satisfecho, y cambia CA-428.

Historia: aprobada el 2026-09-29 (sha256 9fe30ea5).
Depende de: `review-aislada-y-modelo-elegido.md` integrada (PR #38).
Tarea: `hoom task start review-por-diferencia`.

## Objetivo

Tres problemas, uno solo de fondo: la review no sabe qué commit revisó.

1. **Cada ronda de corrección vuelve a mandar la rama entera.** Las seis
   reviews de `review-aislada-y-modelo-elegido` crecieron de 178 a 406 KiB de
   evidencia; la séptima ya no entraba (464 KiB, tope 448, y la sexta llegó a
   213k de los 258k tokens de contexto de Codex). Lo que cambió desde la
   sexta eran 72 KiB, y quedaron sin review de Codex: el PR #38 lo declara.
   Con una review del rango, esa ronda cuesta una fracción y la rama nunca se
   vuelve irrevisable por haber pasado por muchas rondas.
2. **El tablero cuenta como revisado código que nadie revisó.** `reviewOf`
   acepta cualquier registro de 4 lentes de la tarea cuyo veredicto fue
   verde, sin comparar nada con el árbol: después de la review, un commit
   nuevo y un verde nuevo dejan la tarjeta como revisada. Es exactamente lo
   que hoom existe para impedir: el harness declara más de lo que verificó.
3. **No hay forma de revisar un rango.** Los fixes de C1 (`67f842f..765193b`)
   quedaron sin review porque `hoom review` no tiene base propia.

La solución: el registro de la review guarda de qué commit a qué commit
revisó (`desde`, `hasta`), `hoom review --desde <commit>` y `--delta`
revisan un rango, y el tablero solo da por revisado el código que una cadena
de reviews cubre hasta el `HEAD` de la tarjeta.

## No-goals

- El Studio no cambia: `pedir-reviewer` y la cinta siguen lanzando la review
  completa. Que usen el delta es la spec siguiente.
- Revisar un rango que no termina en `HEAD` (`--hasta`): el reviewer lee el
  árbol, y el árbol es `HEAD`.
- Partir sola una primera review demasiado grande en tramos. Se puede hacer
  a mano con `--desde` (queda `parcial`), pero hoom no lo decide.
- Pasarle al reviewer los hallazgos ya refutados.
- Cambiar la medida con la que la review sin rango decide las lentes
  (`gitx.Snapshot`), ni el umbral de 400 líneas.
- Que la historia de la tarjeta distinga una review completa de un delta.
- Migrar los registros viejos: no tienen `hasta` y no se puede reconstruir
  (la huella es de contenido, no de posición en git).

## Contratos

### `hoom review`

```
hoom review [--task t] [--spec s] [--desde <commit> | --delta] [--lens l] ...
```

- `--desde <commit>`: revisa de `<commit>` a `HEAD`. `<commit>` se resuelve
  con git a un commit; un valor que empieza con `-` se rechaza sin llamar a
  git. Tiene que ser ancestro de `HEAD`.
- `--delta`: `--desde` el `hasta` del registro más nuevo de la tarea que
  sirve para encadenar (ver Cobertura) y cuyo `hasta` sigue siendo ancestro
  de `HEAD`. Si hay varios con ese `hasta`, el más nuevo.
- `--desde` y `--delta` juntos: error de uso
  `hoom review: --desde y --delta no van juntos`, exit 2, sin efectos.
- La tarea de la review es la de hoy (`--task`, o el slug del spec sin
  `--task`) y los registros se buscan en el mismo árbol donde se escriben.

Errores, todos antes de lanzar una pasada y sin escribir registro:

- `--desde <c>: no es un commit de este repositorio`
- `--desde <c>: no es un ancestro de HEAD (la review revisa de <c> a HEAD)`
- `--delta: la tarea <t> no tiene una review completa o delta que llegue a este HEAD: corre hoom review sin --delta`

**Orden** (extiende el de la enmienda 5): guarda de árbol sucio →
`hoom.yaml` → base (merge-base y un `git diff` que git pueda armar) →
`--desde` o `--delta` → medida → lentes. Un árbol sucio con un `--desde`
inválido da la negativa de árbol sucio.

### Evidencia

```go
// EvidenceDesde es Evidence con el comienzo explícito: el diff va de desde
// a HEAD. desde tiene que ser ancestro de HEAD.
func EvidenceDesde(dir, desde, spec string, maxBytes int) (Evidencia, error)
```

- El diff es el `git diff --find-renames` de `desde` a `HEAD`, con las
  mismas rutas (fuera de `.hoom/` más `.hoom/agents/`), el mismo tope, los
  mismos marcadores, el spec de `HEAD`, la misma guarda de árbol sucio y el
  mismo fallo cerrado de hoy.
- `Evidence(dir, base, spec, max)` es `EvidenceDesde` con el merge-base de
  la base: los mismos bytes que hoy.
- La política `review:` y el contrato del reviewer siguen saliendo del
  merge-base con la base (CA-417, CA-419), nunca de `desde`: un rango no
  elige su propio reviewer.

### Lentes de una review con rango

- Si de `desde` a `HEAD` no cambió nada en las rutas de la evidencia: 0
  lentes, `SIN REVISAR - no hay cambios desde <desde12>: no hay nada que
  revisar`, exit 0, sin registro.
- Si todo lo que cambió es documentación (la misma `soloDocs` de hoy): 0
  lentes, `SIN REVISAR - lo que cambio desde <desde12> es solo documentacion:
  no se invoca review`, exit 0, sin registro.
- Si no, la regla de hoy aplicada dos veces, al rango (sus archivos y sus
  líneas en las rutas de la evidencia) y al cambio entero (la medida de
  hoy), y gana la más estricta: ninguna < la dominante < las 4. El motivo es
  el de la que ganó, con ` (sobre el rango)` o ` (sobre el cambio entero)`.
  Así un cambio grande no se revisa de a pedacitos con una sola lente.
- `--lens` a mano manda, como hoy.

### Cobertura y registro

`Record` gana:

```go
Desde       string `json:"desde"`                  // sha de 40 hex: donde empieza la evidencia
Hasta       string `json:"hasta"`                  // sha de 40 hex: el HEAD revisado
Cobertura   string `json:"cobertura"`              // completa | delta | parcial
DesdeReview string `json:"desde_review,omitempty"` // el registro que este delta continua
```

- `completa`: `desde` es el merge-base con la base (sin `--desde`, o con un
  `--desde` igual al merge-base).
- `delta`: `desde` es el `hasta` de un registro de la misma tarea que sirve
  para encadenar; `desde_review` lo nombra.
- `parcial`: cualquier otro `desde` (un commit elegido a mano, una base que
  ya contiene el cambio).
- Sirve para encadenar un registro `completa` o `delta` con `hasta`.
- Una `--lens` a mano que no cubre las lentes que la regla pide deja la
  review en `parcial`, aunque el rango sea completo o delta.
- `Result` y `--json` ganan los mismos cuatro campos. Los registros
  anteriores a esta spec no los tienen.

### Lo que imprime

Solo con `--desde` o `--delta`:

- La primera línea dice `desde <desde12>` en lugar de `contra <base>`, con
  el tamaño del rango.
- Después, `  rango       <desde12>..<hasta12> - <cobertura>`, con
  ` de la review <id>` en un delta.

La review sin rango imprime exactamente lo de hoy.

### El pedido

Con rango cambian tres cosas; el resto (marcadores, spec, lente, cierre) es
igual. La primera línea:

```
Revisa lo que cambio en esta rama desde <desde12> hasta <hasta12>. La evidencia completa esta abajo, congelada por hoom (sha256 <hex>, <N> KiB): no vuelvas a sacar el diff; lee otros archivos solo por rangos y solo si hace falta.
```

`Base: ...` pasa a `Rango: <desde12>..<hasta12> (<cobertura>). Tamano: <n>
archivos, +<i>/-<d> lineas.`, con el tamaño del rango, y después va:

```
Lo anterior a <desde12> ya lo reviso la review <id>: INTRODUCIDO es lo que trae este rango o lo que este rango rompe de lo anterior; lo demas es pre-existente.
```

En un `parcial`, la primera mitad es `Lo anterior a <desde12> no es parte
de esta review:`. La review sin rango manda exactamente el pedido de hoy.

### Tablero

Un registro de 4 lentes cuenta como la review de la tarjeta solo si:

- tiene `hasta` y es `completa` o `delta`;
- su cadena existe: siguiendo `desde_review` se llega a un `completa`, y
  todos los registros del camino son de la misma tarea y de 4 lentes;
- su veredicto es un verde de la tarjeta (como hoy);
- su `hasta` es ancestro del `HEAD` de la tarjeta y, de `hasta` a `HEAD`,
  en las rutas de la evidencia no cambió nada o solo cambió documentación.

Un registro sin `hasta` (anterior a esta spec) o `parcial` nunca cuenta.

Si ningún registro cuenta, el tablero mira el registro con el que
`--delta` encadenaría: el más nuevo `completa` o `delta` de la tarea cuyo
`hasta` sigue en la historia de `HEAD`. Si ese registro cumple todo lo de
arriba salvo lo último (el código siguió después de la review):

- motivo `la review <id> cubre hasta <hasta12> y el codigo cambio despues`;
- en palabras simples, `falta revisar lo nuevo desde la ultima review`;
- siguiente paso `hoom review --task <s> --spec <spec> --delta`.

Si no (su cadena no es toda de 4 lentes, está rota, o su veredicto no es un
verde de la tarjeta), el motivo y el siguiente paso de hoy: la review
completa. Así el tablero nunca pide un `--delta` que no lo dejaría
satisfecho.

## Casos límite y errores esperados

- Una review sin rango: sus bytes, su salida y su pedido son los de hoy; el
  registro gana `desde` = merge-base, `hasta` = `HEAD` y `cobertura:
  completa`.
- `--delta` sin ningún registro de la tarea, o solo con registros viejos (sin
  `hasta`), `parcial`, o cuyo `hasta` ya no es ancestro de `HEAD` (un
  rebase): el error de `--delta`.
- `--delta` con `hasta` = `HEAD`: `SIN REVISAR - no hay cambios desde ...`.
- Un merge de `main` en la rama después de la review: el delta incluye lo
  que trajo el merge. Revisa de más, nunca de menos. Si pasa el tope, la
  negativa de hoy.
- `--desde` en un árbol cuya base ya contiene el cambio (un commit ya
  integrado, revisado desde un worktree separado): el cambio entero está
  vacío, las lentes salen del rango, y la review queda `parcial`.
- Un commit que solo toca documentación después de la review: la tarjeta
  sigue revisada.
- Un commit que solo toca `.hoom/` (veredictos, hallazgos, registros) fuera
  de `.hoom/agents/`: no cambia la evidencia; la tarjeta sigue revisada.
- Una cadena rota (falta el registro que nombra `desde_review`): no cuenta.
- Una cadena que empezó con una sola lente (el cambio era chico) y después
  creció: no tiene las 4 lentes en todo el camino y no cuenta; hace falta la
  review completa.
- Una `completa` de una lente más nueva que una cadena de 4 lentes (el
  cambio se achicó, se revisó con la lente dominante y volvió a crecer):
  `--delta` encadenaría con ella, así que el tablero pide la review
  completa, no el delta.
- `--desde` y `--delta` con `--lens`: vale; la cobertura sale de la regla de
  arriba.

## Criterios de aceptación

- CA-420: `hoom review --desde <c>` arma la evidencia con el `git diff --find-renames` de `<c>` a `HEAD` (mismas rutas, tope y marcadores): un cambio commiteado antes de `<c>` no aparece y uno posterior sí; `EvidenceDesde(dir, mb, spec, max)` con el merge-base devuelve los mismos bytes que `Evidence(dir, base, spec, max)`.
- CA-421: `--desde` que no es un commit, que no es ancestro de `HEAD` o que empieza con `-` da su error sin pasadas ni registro, después de la guarda de árbol sucio; un rango sin cambios en las rutas de la evidencia, o solo de documentación, termina `SIN REVISAR` con exit 0 y sin registro.
- CA-422: `--delta` revisa desde el `hasta` del registro más nuevo de la tarea `completa` o `delta` cuyo `hasta` es ancestro de `HEAD`; sin uno así (ningún registro, solo registros sin `hasta`, `parcial`, o con `hasta` fuera de la historia) da el error de `--delta` sin pasadas; `--desde` con `--delta` es un error de uso con exit 2.
- CA-423: el registro, el `Result` y `--json` llevan `desde`, `hasta` (40 hex), `cobertura` y `desde_review`: sin rango `completa` con `desde` = merge-base y `hasta` = `HEAD`; `--delta` o `--desde` igual a un `hasta` encadenable da `delta` con `desde_review`; `--desde` igual al merge-base da `completa`; otro `--desde` da `parcial`; una `--lens` a mano que no cubre las lentes de la regla da `parcial`.
- CA-424: las lentes de una review con rango son las más estrictas entre la regla sobre el rango y sobre el cambio entero: un delta de menos de 400 líneas sin rutas de riesgo en una rama de más de 400 lleva las 4; un rango de más de 400 líneas con el cambio entero vacío lleva las 4; `--lens` a mano manda.
- CA-425: con rango, el pedido cambia solo la primera línea, la línea `Rango:` y la línea de INTRODUCIDO (con la review encadenada en un delta y `no es parte de esta review` en un parcial); la review sin rango manda exactamente el pedido de hoy.
- CA-426: con rango, la salida dice `desde <desde12>` en la primera línea y lleva la línea `rango`; la política `review:` y el contrato del reviewer salen del merge-base con la base aunque la rama los cambie en un commit anterior a `desde`.
- CA-427: el tablero cuenta un registro de 4 lentes solo si tiene `hasta`, es `completa` o `delta`, su cadena llega a un `completa` con todos sus registros de la tarea y de 4 lentes, y de su `hasta` al `HEAD` de la tarjeta solo cambió documentación o nada en las rutas de la evidencia; un registro sin `hasta`, uno `parcial` o una cadena rota no cuentan.
- CA-428: con código commiteado después de la review, si el registro con el que `--delta` encadenaría tiene su cadena de 4 lentes, la tarjeta vuelve a Review con `la review <id> cubre hasta <hasta12> y el codigo cambio despues` y el siguiente paso `hoom review --task <s> --spec <spec> --delta`, y después de ese delta pasa a Tu aceptación si lo demás se cumple; si ese registro no la tiene (una `completa` de una lente más nueva que la cadena de 4), el motivo y el siguiente paso de hoy (la review completa); un commit solo de documentación o solo de `.hoom/` no la hace volver.
- CA-429: el README y la ayuda de `hoom review` documentan `--desde` y `--delta`. [verifica: grep -q -- "--delta" README.md && grep -q -- "--desde" README.md && grep -q "cobertura" README.md]

## Decisiones

- **El ancla es el commit, no la huella.** `ChangeFingerprint` es un hash
  de contenido: no dice de qué commit a qué commit se revisó ni permite
  sacar un diff. Desde ahora cada registro lo dice (`desde`, `hasta`).
- **La cadena la valida quien la lee.** El tablero sigue `desde_review`
  hasta un `completa` en vez de confiar en un campo derivado: si falta un
  eslabón, no cuenta.
- **Las lentes más estrictas de las dos.** Si el delta decidiera solo sobre
  sí mismo, una rama de 2000 líneas hecha en deltas de 300 recibiría una sola
  lente. Con la regla sobre el cambio entero, cada delta de una rama grande
  lleva las 4; es más barato igual porque la evidencia es chica.
- **La documentación al final no desarma la review.** La regla ya dice que
  un cambio solo de documentación no se revisa; exigir un delta para un
  README obligaría a un `SIN REVISAR` que no deja registro y la tarjeta
  quedaría trabada.
- **`parcial` existe y no cuenta.** Hace falta para revisar a mano lo que no
  tiene cadena: los fixes de C1, un tramo de una rama grande o la primera
  ronda sin review de la review aislada. El registro dice qué se revisó; el
  tablero no lo cuenta como la review de la tarjeta.
- **Los registros viejos dejan de contar.** No tienen `hasta` y nada prueba
  que cubran el código actual. Una tarjeta que hoy figura como revisada con
  un registro viejo pasa a pedir la review, que es la verdad.
- **La política sale del merge-base, no de `desde`.** `desde` es un commit
  del candidato: leer de ahí la sección `review:` dejaría que el cambio
  eligiera su reviewer, lo que CA-417 prohíbe.
- **Primer uso: la ronda sin review de la review aislada.** Una tarea cuya
  rama apunta a `994a5cc` (la punta de esa spec) y
  `hoom review --task <t> --desde 9c08d85 --spec .hoom/specs/review-aislada-y-modelo-elegido.md`:
  72 KiB de diff más 31 de spec, `parcial` porque el registro de la sexta
  review no tiene `hasta`. Sus registros y hallazgos se commitean en esa
  rama.

## Riesgos y deuda aceptada

- **Un delta ve menos contexto.** Un defecto que nace de la interacción del
  rango con código anterior es más difícil de ver. Mitigación: el reviewer
  puede leer el árbol por rangos, el pedido le dice que lo que el rango
  rompe de lo anterior es INTRODUCIDO, y la review completa sigue
  disponible.
- **Un merge de `main` agranda el delta.** Revisa de más; si pasa el tope,
  hace falta la review completa o partir el cambio.
- **Un rebase rompe la cadena.** `--delta` se niega y hace falta la review
  completa.
- **El tablero confía en el registro.** Como hoy: lo escribe solo
  `hoom review` y un rol que lo toca viola su territorio, pero una persona
  con acceso al repo puede editarlo.
- **Dos medidas.** El rango se mide con las rutas de la evidencia; el
  cambio entero, con la medida de hoy (incluye `.hoom/specs/`). Unificarlas
  es otra spec.
