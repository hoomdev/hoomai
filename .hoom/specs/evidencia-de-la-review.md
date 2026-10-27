# Spec: la review ve todo lo que no es estado de hoom, y la delta no cuenta lo que trajo la base

Estado: ENMIENDA 1 — pendiente de re-aprobación humana. La implementación
de la versión aprobada (sha256 8e921c8d) mostró tres cosas, y la enmienda
las fija:

1. Con `--desde` en un árbol cuya base ya contiene `HEAD`, la regla
   descontaba el rango entero y la review terminaba `SIN REVISAR`. Es el
   caso de CA-424 de review-por-diferencia: revisar un rango ya integrado.
   Cuando la base ya contiene `HEAD`, la rama no integró nada, y el rango
   se revisa como hoy.
2. CA-487 pedía el árbol de trabajo igual después de un `--delta`, pero
   toda review escribe su registro y su sobre bajo `.hoom/`. Se precisa.
3. La suite mostró más criterios anteriores que fijan la regla vieja
   (CA-413, CA-419, CA-420, CA-421, CA-443). Se suman a la lista de
   re-expresados.

Historia: aprobada el 2026-10-27 (sha256 8e921c8d).
Depende de: `review-aislada-y-modelo-elegido.md` (PR #38),
`review-por-diferencia.md` (PR #39) y `base-de-la-review.md` (PR #40),
integradas.
Tarea: `hoom task start evidencia-de-la-review`.
Cierra: los hallazgos 8fe5a6 y 547228.

## Objetivo

Dos fallas de lo que recibe `hoom review`, las dos del mismo tipo: el registro
dice algo que la evidencia no respalda.

1. **La review no ve código que hoom ejecuta (8fe5a6).** La evidencia es
   "todo lo que está fuera de `.hoom/` más `.hoom/agents/`". El gate
   `security` ejecuta `.hoom/semgrep/gate.sh` con las reglas y el lock de
   `.hoom/semgrep/`, y eso queda afuera. La review de lock-semgrep recibió
   1352 bytes (solo comentarios de `ci.yml` y `hoom.yaml`), dos lentes
   respondieron "los cambios son únicamente comentarios", y el registro dice
   `cobertura: completa`. Con `gate.sh` en gates-dogfood pasó lo mismo. Es
   una lista de inclusión: cada directorio nuevo con código bajo `.hoom/`
   queda afuera hasta que alguien se acuerde de agregarlo.
2. **La delta le atribuye a la tarea lo que trajo la base (547228).**
   `--delta` revisa `desde..HEAD`. Si entre dos reviews la rama integra la
   base (`git merge main`), el rango trae todo lo de ese merge, y el reviewer
   lo marca como INTRODUCIDO por la tarea. En la delta 2 de endurecimiento-1
   (4676 líneas "sobre el rango"), tres hallazgos eran de archivos que
   vinieron de main (#53, #54), y hubo que abrir una tarea aparte
   (endurecimiento-2) para no mezclarlos.

La solución:

- La evidencia pasa a ser todo, menos una lista cerrada de entradas que son
  estado de hoom. El árbol sucio se mide sobre esas mismas rutas.
- La delta compara contra la punta anterior **con la base integrada** (un
  merge sintético, como "cambios desde tu última review" de GitHub). Si ese
  merge no se puede armar sin conflicto, la review es completa y lo dice.

## No-goals

- La huella (`ChangeFingerprint`) no cambia: decide qué certifica `verify`,
  no qué ve la review.
- La medida del cambio entero (`gitx.Snapshot`), el umbral de 400 líneas y
  las rutas de riesgo no cambian. `.hoom/semgrep/` como ruta de riesgo es
  otra conversación.
- `hoom agent` (el sobre) y su gate de scope no cambian.
- El Studio sigue lanzando la review completa.
- Un rebase sigue rompiendo la cadena: `--delta` se niega como hoy.
- Merges de ramas que no son la base: lo que traen es parte de la tarea.

## Contratos

### Las rutas de la evidencia

La evidencia, la medida del rango (`gitx.CambiosEntre`), la guarda de árbol
sucio y `CambioDespues` usan una sola definición: **todo el repositorio menos
el estado de hoom**. El estado de hoom es exactamente esta lista:

| Entrada | Qué es |
|---|---|
| `.hoom/approvals/*.json` | aprobaciones de specs |
| `.hoom/findings/*.json` | hallazgos y sus resoluciones (`.res.json`) |
| `.hoom/reviews/*.json` | registros de review |
| `.hoom/verdicts/*.json` | veredictos |
| `.hoom/items/*.yaml` | items del tablero |
| `.hoom/specs/*.md` | specs (el de la review va entero en el pedido) |
| `.hoom/intake/` (todo) | documentos del cliente que sube el Studio |
| `.hoom/ratchet.json` | el trinquete |
| `.hoom/.gitignore` | lo completa hoom solo (por eso ya está fuera de la huella) |
| `.hoom/cache/`, `worktrees/`, `runs/`, `isolated/`, `envelopes/` | locales (`hoomfs.Local`) |

`*.json`, `*.yaml` y `*.md` son archivos **directos** del directorio. Un
subdirectorio, u otra extensión en ese mismo lugar, no es estado: entra. Por
ejemplo, entran `.hoom/findings/x.sh`, `.hoom/specs/run.sh` y
`.hoom/findings/sub/y.json`. Todo lo demás bajo `.hoom/` también entra:
`agents/`, `semgrep/`, archivos sueltos y directorios nuevos.

### El árbol sucio

`gitx.CambioSinCommitear` mira las mismas rutas. Lo que entra a la evidencia
tiene que estar commiteado: si no, el reviewer leería en el árbol algo
distinto de lo que recibió.

- Ahora frenan la review (antes no): una edición sin commitear de
  `.hoom/agents/` o `.hoom/semgrep/`, y un archivo suelto sin rastrear en
  `.hoom/` (ej. `.hoom/notas.txt`).
- No frenan: los cambios sin commitear en el estado de hoom (el veredicto,
  los hallazgos y el registro que deja cada paso del ritual).

El error y el orden son los de hoy (CA-416).

### Lo nuevo de la rama desde un commit

La review con rango (`--delta`, `--desde`) y el tablero calculan "lo que la
rama cambió desde `D` hasta `H`" con una sola función de `gitx`, así nunca
dicen cosas distintas. Sea `I` el merge-base de la base de la review con `H`:
la base que la rama tiene integrada.

- Si `I` es `H`, la base ya contiene `HEAD`: es un rango ya integrado que
  se revisa desde la base, y la rama no integró nada. Se compara `D` con
  `H`, como hoy, y CA-424 sigue valiendo.
- Si `I` es ancestro de `D`, se compara `D` con `H`, como hoy.
- Si no, la rama integró la base después de `D`. Se arma el merge sintético
  de `D` con `I`, y si sale sin conflicto se compara **su árbol** con `H`.
  `base_integrada` = `I`. Lo que el commit de merge agregó más allá del merge
  limpio (un "evil merge") queda en la comparación, porque es de la tarea.
- **No se separa** si el merge sintético tiene conflicto, o si el git
  instalado no lo sabe armar (anterior a 2.40). Entonces la review con rango
  es la **review completa** de hoy, con el aviso de abajo, y para el tablero
  el código cambió.
- Cualquier otra falla de git es un error, sin pasadas ni registro (falla
  cerrada), y nunca se cae al diff crudo.

El merge sintético:

- no escribe nada en el repositorio: ni objetos, ni refs, ni el índice, ni
  el árbol de trabajo. El tablero es de solo lectura.
- no corre ningún programa de la configuración: ni drivers de merge ni
  filtros, como el diff ya no corre textconv (43bde9). Los `.gitattributes`
  de la rama no eligen nada.

El orden de la review con rango, extendido: guarda de árbol sucio →
`hoom.yaml` → base → `--desde`/`--delta` → **separación** → medida → lentes.

### Registro, salida y pedido

- `Record`, `Result` y `--json` ganan
  `BaseIntegrada string \`json:"base_integrada,omitempty"\``: el sha de 40 hex
  de `I` cuando se descontó la base.
- `desde`, `desde_review` y `cobertura` no cambian de significado. Una delta
  que separó sigue siendo `delta` de la review anterior, con `desde` = el
  `hasta` de esa review, así la cadena del tablero no cambia.
- La medida del rango (el tamaño, las lentes "sobre el rango", el
  `SIN REVISAR`) es la de lo nuevo, no la del rango crudo.
- Salida, después de la línea `rango`, cuando se descontó la base:

  ```
    base        integrada hasta <I12> despues de <D12>: lo que trajo no es parte del rango
  ```

- Pedido, después de la línea `Rango:`, cuando se descontó la base:

  ```
  La rama integro la base hasta <I12> despues de <D12>: el diff es contra <D12> con esa base integrada, asi que lo que trajo la base no esta en la evidencia y no es INTRODUCIDO.
  ```

- Cuando no se separa, la review es la completa de hoy: `desde` = `I`,
  `cobertura` completa, sin `desde_review` ni `base_integrada`, primera línea
  `contra <base>`. Además lleva este aviso en la salida y en `notes` (`<ruta>`
  escapada como las demás rutas de la salida):

  ```
    aviso: la rama integro la base (<I12>) despues de <D12> y ese merge no se separa (conflicto en <ruta> | git <version> no arma el merge sintetico, hace falta 2.40): la review es completa
  ```

### El tablero

`reviewcmd.CambioDespues` gana la base:

```go
func CambioDespues(dir, base, hasta, head string) (ancestro, codigo bool, err error)
```

`codigo` se calcula sobre lo nuevo desde `hasta`, y es `true` cuando no se
separa. `boardcmd` le pasa la base del proyecto. Lo demás de la regla del
tablero (CA-427, CA-428) queda igual.

### Lo que cambia de specs anteriores

Esta spec re-expresa estos criterios. Los tests que fijan la regla vieja los
re-expresa el test-writer contra esta spec, y no se borra ninguno:

- **review-aislada, CA-401.** Decía "un cambio en `.hoom/agents/` y nada más
  de `.hoom/`". Ahora: lo que está en las rutas de esta spec. Por ejemplo,
  `.hoom/notas.txt` y `.hoom/agents-viejo/` commiteados entran.
- **review-aislada, CA-416.** Decía "con cambios solo en `.hoom/` la
  evidencia se arma como siempre". Ahora: solo en el estado de hoom. Una
  edición sin commitear de `.hoom/agents/` o un `.hoom/notas-locales.txt`
  sin rastrear ahora frenan la review.
- **review-aislada, CA-419.** Las variantes en que la rama reescribe,
  borra o agrega el contrato 06 **sin commitear** ahora terminan en la
  negativa de árbol sucio, sin pasadas. El provider sigue sin recibir ese
  contrato. Las variantes commiteadas no cambian.
- **review-aislada, CA-413.** Miles de archivos sin rastrear bajo
  `.hoom/`, fuera del estado, ahora ensucian el árbol. Que no se lean
  enteros se prueba con los que están en el estado de hoom, o con la
  negativa, que lee solo la primera entrada.
- **review-por-diferencia.** `EvidenceDesde` usaba "las mismas rutas", que
  pasan a ser las de esta spec. El caso límite "un merge de `main` … revisa
  de más" lo reemplaza la delta sin la base. En CA-428, "solo de `.hoom/`"
  pasa a ser "solo del estado de hoom".
- **review-por-diferencia, CA-420 y CA-421.** En CA-420, un
  `.hoom/notas.txt` commiteado en la historia ahora va en el diff. En
  CA-421, un rango que solo toca `.hoom/notas.txt` ahora es solo
  documentación; "no hay cambios" es un rango que solo toca el estado de
  hoom.
- **evidencia-en-disco, CA-443.** El `.hoom/findings/.gitignore` que
  esconde un hallazgo no es estado: el fixture lo commitea antes de la
  review, y lo demás no cambia.

## Casos límite y errores esperados

- Una review completa sin nada bajo `.hoom/` fuera del estado: los mismos
  bytes que hoy.
- Una rama que solo cambia `.hoom/semgrep/`: la review tiene evidencia y
  lentes, y no termina `SIN REVISAR`.
- Una delta sin merges de la base en el rango: exactamente la de hoy, sin
  `base_integrada` ni líneas nuevas.
- La base integrada **antes** de la review anterior (ya está en `D`): `I` es
  ancestro de `D`, y la delta es la de hoy.
- Dos merges de la base después de la review: `I` es el último merge-base, y
  un solo merge sintético lo cubre.
- Solo un merge limpio de la base después de la review: lo nuevo está vacío.
  La delta termina `SIN REVISAR` y la tarjeta sigue revisada, siempre que
  haya un verde para el `HEAD` nuevo.
- Un conflicto que la rama resolvió al integrar la base: no se separa, y la
  review es completa. La resolución es código de la tarea y se revisa con
  todo el contexto.
- `--desde <c>` con la base integrada después de `c`: la misma regla, y la
  cobertura sale como hoy (`parcial` o `delta`).
- `--desde <c>` en un árbol cuya base ya contiene `HEAD` (un rango ya
  integrado): se revisa `c..HEAD` sin descontar nada, como dice CA-424.
- `--lens` a mano: vale; la cobertura sale de la regla de hoy.
- Una ruta conflictiva con caracteres de control: el aviso la escapa.

## Criterios de aceptación

- CA-481: la evidencia (`Evidence` y `EvidenceDesde`) trae los cambios commiteados en `.hoom/semgrep/gate.sh`, `.hoom/semgrep/paquetes.lock`, `.hoom/notas.txt`, un directorio nuevo `.hoom/otra/x.sh`, `.hoom/agents-viejo/nota.md`, `.hoom/findings/x.sh` y `.hoom/findings/sub/y.json`, y no trae los cambios commiteados en ninguna entrada de la tabla del estado de hoom: un hallazgo `.json` y su `.res.json`, un registro de review, un veredicto, un item `.yaml`, una aprobación, un spec `.md` (que sigue yendo entero), un documento de `.hoom/intake/`, `.hoom/ratchet.json` y `.hoom/.gitignore`.
- CA-482: con una modificación sin commitear de `.hoom/semgrep/gate.sh`, un `.hoom/notas.txt` sin rastrear o una edición sin commitear de `.hoom/agents/04-writer.md`, `hoom review` (con y sin `--task`) y `Evidence` devuelven el error de árbol sucio con esa ruta, sin pasadas ni registro. Con cambios sin commitear solo en entradas del estado de hoom (incluidos un documento recién subido a `.hoom/intake/` y un borrador de spec), la review corre.
- CA-483: después de una review de 4 lentes, un commit que solo toca `.hoom/semgrep/gate.sh` devuelve la tarjeta a Review con el siguiente paso `--delta`, y ese `--delta` lo revisa con lentes, no con `SIN REVISAR`. Un commit que solo toca el estado de hoom (un veredicto, hallazgos, un registro de review) no la devuelve.
- CA-484: si después de una review la rama commitea X, integra la base sin conflicto (eso trae Y) y commitea Z, `hoom review --delta` revisa X y Z sin Y. La medida y las lentes "sobre el rango" son las de X+Z. El registro, el `Result` y `--json` dicen `cobertura: delta`, `desde` = el `hasta` anterior, su `desde_review` y `base_integrada` = el merge-base de la base con `HEAD` (40 hex). La salida lleva la línea `base` y el pedido la línea de la base integrada. Un cambio hecho en el propio commit de merge, más allá del merge limpio, sí aparece. `--desde <c>` en el mismo caso también deja Y afuera.
- CA-485: si después de una review la rama solo integró la base sin conflicto, `hoom review --delta` termina `SIN REVISAR - no hay cambios desde <desde12>: no hay nada que revisar`, con exit 0 y sin registro. Con un verde para el `HEAD` nuevo, el tablero no devuelve la tarjeta a Review por ese merge.
- CA-486: si después de una review la rama integró la base resolviendo un conflicto, `hoom review --delta` hace la review completa: `desde` = el merge-base, `cobertura: completa`, sin `desde_review` ni `base_integrada` y primera línea `contra <base>`. La salida y `notes` llevan el aviso `la rama integro la base (<I12>) despues de <D12> y ese merge no se separa (conflicto en <ruta>): la review es completa`. Para el tablero, el código cambió después de la review. Un git que se anuncia anterior a 2.40 da lo mismo con el motivo `git <version> no arma el merge sintetico, hace falta 2.40`.
- CA-487: el merge sintético no deja rastro ni corre programas. Después de un `--delta` que separó y de un `hoom board` que evaluó esa tarjeta, el repositorio tiene los mismos objetos, refs e índice que antes, y el árbol de trabajo solo cambia en lo que toda review escribe bajo `.hoom/` (su registro, su sobre, su narración). Si la rama declara `* merge=drv` en su `.gitattributes` y la config del repo define `merge.drv.driver` como un programa que escribe una marca, la marca nunca aparece. Una falla de git en el merge sintético que no es un conflicto da un error, sin pasadas ni registro.
- CA-488: el README y el contrato 06 (sus dos copias iguales) dicen que la evidencia es todo menos el estado de hoom y que la delta descuenta la base integrada. [verifica: grep -q "base_integrada" README.md && grep -q "estado de hoom" README.md && grep -q "estado de hoom" internal/agents/assets/agents/06-reviewer.md && cmp -s internal/agents/assets/agents/06-reviewer.md .hoom/agents/06-reviewer.md]

## Decisiones

- **Lista cerrada de exclusiones, no de inclusiones.** Así, lo nuevo bajo
  `.hoom/` entra solo. Descartado:
  - sumar `.hoom/semgrep/` a la lista de inclusión: repite el olvido que
    causó 8fe5a6;
  - derivar de `hoom.yaml` qué ejecuta cada gate: un gate es un comando de
    shell y no hay forma confiable de leer qué archivos toca.
- **Exclusión por forma, no por directorio.** El estado son los archivos
  directos con la extensión que hoom escribe. Un `.sh` en `findings/` o un
  subdirectorio no es estado y entra. `intake/` va entero porque el Studio
  sube documentos de cualquier tipo.
- **Sin el aviso de "tocó `.hoom/` fuera de la evidencia" que propuse en el
  chat.** Con la lista cerrada, lo único que queda afuera es estado que
  escribe hoom: el aviso no tendría nada que decir.
- **El árbol sucio mide lo mismo que la evidencia.** Si no, el reviewer
  podría leer en el árbol un `gate.sh` distinto del que recibió. El costo es
  que una nota suelta en `.hoom/` frena la review; se commitea o se ignora.
- **Merge sintético en lugar de las alternativas.** Descartado:
  - negar `--delta` cuando el rango trae un merge de la base y pedir la
    review completa: es simple, pero cuesta una review entera de 4 lentes
    cada vez que se integra main, y el plan de Codex se agota;
  - comparar los dos diffs (interdiff): el reviewer recibiría un diff de
    diffs, difícil de leer y de citar;
  - avisar y seguir: el reviewer sigue atribuyéndole a la tarea lo de main,
    que es exactamente 547228.
- **Con conflicto, review completa.** La resolución de un conflicto es
  código de la tarea en el punto donde se cruza con la base: es lo que más
  contexto pide.
- **`desde` no cambia de significado.** La cadena del tablero sigue
  igual, y `base_integrada` dice qué se descontó.
- **Una sola función para la review y el tablero,** como `Encadenable`: el
  tablero nunca pide una delta que diría otra cosa.

## Riesgos y deuda aceptada

- **Código escondido en un directorio de estado** (ej. un `.json` en
  `findings/` que un script lee). Lo que lo ejecuta (`hoom.yaml`, un script,
  el código) sí está en la evidencia, y ahí se ve la referencia.
- **Integrar la base puede romper la tarea sin cambiar su diff** (una API que
  cambió en main). La delta no lo ve. Lo ven `verify` y la review completa,
  que sigue disponible. Es el mismo riesgo aceptado de "un delta ve menos
  contexto".
- **Árbol sucio más estricto.** Un archivo suelto en `.hoom/` frena la
  review. El mensaje nombra la ruta.
- **git 2.40 o más nuevo para separar.** Con un git más viejo, una delta
  después de integrar la base es completa, y lo dice.
- **Merges cruzados (criss-cross).** `git merge-base` elige uno, como hoy en
  toda la review.
