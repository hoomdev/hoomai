# Spec: la review toma la base del proyecto, no de la rama

Estado: ENMIENDA 1 — pendiente de re-aprobación humana. La versión
aprobada (sha256 9cf40ac5) está implementada. Su review mostró que con
`--base` y sin `--task` el territorio del reviewer seguía saliendo del
`hoom.yaml` del árbol revisado, aunque la spec recomienda `--base` para
revisar una rama en la que no confiás. La enmienda 1 saca el territorio del
mismo lugar que la política `review:` y el contrato: el merge-base con la
base. Cambia CA-431.

Historia: aprobada el 2026-09-29 (sha256 9cf40ac5).
Depende de: `review-por-diferencia.md` integrada (PR #39).
Tarea: `hoom task start base-de-la-review`.

## Objetivo

CA-417 y CA-419 prometen que un cambio no elige su propia review: la
política `review:` (aislamiento, tope, `same_provider`, modelo) y el contrato
del reviewer salen del merge-base con la base, nunca de la rama. Pero la base
misma sale de la rama. `hoom review --task` recibe la base del `hoom.yaml`
del proyecto y `prepararRevision` la reemplaza por el `base_branch` del
`hoom.yaml` del worktree de la tarea.

El refutador de `review-por-diferencia` lo reprodujo sin ninguna carrera:

1. La rama commitea W con `base_branch: w`, una política laxa y su propio
   contrato 06.
2. La ref `w` apunta a W.
3. `hoom review --task` termina `REVISADO` completa de 4 lentes, con
   `isolated: false`, tope 4096, el contrato de la rama y `desde` = W.

La rama eligió su base, y con ella su política, su contrato y hasta dónde
empieza lo que se revisa. Es la misma clase de agujero que CA-417 cerró,
un nivel más arriba.

Además, la política de escritura del reviewer (`agents.reviewer.write` de
`hoom.yaml`), que decide qué puede tocar sin violar su territorio, también
se lee del worktree de la tarea.

La solución: la review toma la base y la política del reviewer del proyecto
(el árbol donde corre hoom), o de un `--base` explícito, nunca del árbol que
revisa.

## No-goals

- `hoom agent` y `hoom verify` siguen tomando la base del árbol donde
  corren. El alcance de sus gates es otra conversación; esta spec es de la
  review.
- Sin `--task` y sin `--base`, la base es el `base_branch` del `hoom.yaml`
  del árbol donde corre hoom, que es el del proyecto por construcción.
  Revisar en el propio checkout una rama en la que no confiás se hace con
  una tarea (`--task`) o con `--base`: la base la elegís vos, y todo lo que
  sale de ella (política, contrato, territorio) sale del merge-base, no de
  la rama.
- Validar que el `base_branch` del proyecto sea "bueno": lo elige quien
  corre hoom.

## Contratos

### De dónde sale la base

En orden:

1. `hoom review --base <ref>`: la elige quien corre la review. `<ref>` se
   resuelve con git a un commit; un valor que empieza con `-` se rechaza sin
   llamar a git.
2. El `base_branch` del `hoom.yaml` del proyecto: el que la CLI lee del árbol
   donde corre y el que el Studio tiene cargado, que es lo que ya le pasan a
   `reviewcmd.Run(root, base, ...)`.

`prepararRevision` deja de reemplazar esa base por el `base_branch` del
worktree de la tarea. Todo lo que sale de la base sale de esta:

- el merge-base;
- la evidencia y el rango (`--desde`, `--delta`);
- la cobertura;
- la política `review:` y el contrato del reviewer (CA-417, CA-419);
- las lentes.

Si el `hoom.yaml` del worktree de la tarea declara otra base, la review
sigue con la del proyecto y lo dice, en la salida y en `notes` del registro:

```
  aviso: el hoom.yaml de la rama dice base_branch <x>: la review usa <y>, la del proyecto
```

`--base` inválido: `--base <ref>: no es un commit de este repositorio`, sin
pasadas ni registro, en el orden de la enmienda 5 de la review aislada
(después de la guarda de árbol sucio, antes de medir).

### La política de escritura del reviewer

El gate de territorio de cada pasada usa `agents.reviewer.write` del
`hoom.yaml` del merge-base con la base (la del proyecto o `--base`), como la
política `review:` (CA-417) y el contrato (CA-419): nunca del árbol revisado,
con o sin `--task`. Una rama que agranda lo que el reviewer puede escribir
no agranda el gate de su propia review. Un merge-base sin `hoom.yaml` o sin
esa sección deja el territorio por defecto del rol.

### Registro y salida

- `Record`, `Result` y `--json` ganan `base`: el nombre con el que se
  resolvió la base (el de `--base` o el `base_branch` del proyecto).
- `Options` gana `Base` (`--base`).
- La ayuda de `hoom review` y el README nombran `--base` y la regla.

## Casos límite y errores esperados

- Una rama que no toca `base_branch`: la review es exactamente la de hoy.
- Una tarea cuyo `hoom.yaml` no declara `base_branch` (vale `main` por
  defecto) en un proyecto con `develop`: la review usa `develop` y avisa.
- `--base` con `--task`, con `--desde` o con `--delta`: vale; `--delta`
  encadena como siempre, porque lo que cambia es de qué merge-base sale la
  política, no los registros.
- `--base` que no existe, o un clon sin el merge-base: el error de CA-414.
- El Studio (`pedir-reviewer`, la cinta): usa la base del proyecto que el
  servidor tiene cargado; nada cambia en su API.
- Sin `--task` y sin `--base`: la base es la del `hoom.yaml` del árbol donde
  corre hoom, como hoy (es el proyecto).

## Criterios de aceptación

- CA-430: con `--task`, una rama que commitea `base_branch: w` (con `w` en su propia historia, una política `review:` laxa y su propio contrato 06) se revisa contra la base del proyecto: el `desde` es el merge-base de esa base con `HEAD`, la política y el contrato son los de ese merge-base (aislada, su tope, su contrato), y la salida y `notes` del registro llevan `el hoom.yaml de la rama dice base_branch w: la review usa <base>, la del proyecto`.
- CA-431: el gate de territorio del reviewer usa `agents.reviewer.write` del `hoom.yaml` del merge-base con la base: una rama que lo agranda en su `hoom.yaml` no evita que un reviewer que escribe fuera del territorio por defecto termine en violación de territorio, con `--task`, y también sin `--task` con `--base`; lo que el proyecto declara en la base sí vale.
- CA-432: `--base <ref>` manda sobre el `base_branch` del proyecto y de la rama para la evidencia, el rango, la política, el contrato y las lentes; un `--base` que no es un commit o que empieza con `-` da `--base <ref>: no es un commit de este repositorio` sin pasadas ni registro; el registro, el `Result` y `--json` llevan `base`.
- CA-433: `pedir-reviewer` del Studio revisa una tarea cuya rama cambia `base_branch` contra la base del proyecto del servidor, con el mismo aviso en el registro.
- CA-434: el README y la ayuda de `hoom review` documentan `--base`. [verifica: grep -q -- "--base <ref>" README.md]

## Decisiones

- **La base es del proyecto.** El árbol donde corre hoom es el del proyecto:
  el checkout principal en la CLI y el árbol que sirve el Studio. Ahí se
  creó la tarea (`hoom task start` ya usa esa base) y ese `hoom.yaml` es la
  voz de quien opera. El worktree de la tarea es el candidato.
- **No se puede sacar la base de la base.** Leer `base_branch` del merge-base
  es circular: el merge-base depende de la base. Tampoco sirve comparar la
  rama con el `hoom.yaml` de su base: en el ataque, W declara `w` y se
  valida a sí misma. La confianza tiene que venir de afuera del candidato:
  el proyecto o `--base`.
- **Avisar en vez de negarse** cuando la rama declara otra base: cambiar
  `base_branch` puede ser legítimo (el proyecto cambia de rama principal) y
  se revisa como cualquier otro cambio del `hoom.yaml`, que está en la
  evidencia. Lo que no puede es decidir su propia review.
- **La política de escritura del reviewer va con la base.** Es la misma
  clase: una rama no afloja el gate de su propio reviewer. Sale del
  merge-base, como la política `review:` y el contrato, y no del `hoom.yaml`
  del checkout del proyecto: así vale igual con `--task`, con `--base` y en
  el Studio, y un cambio sin commitear en el checkout no la mueve.

## Riesgos y deuda aceptada

- **El checkout del proyecto puede estar en otra rama.** Quien opera elige
  su checkout; `--base` lo resuelve explícitamente.
- **Sin `--task` y sin `--base`, la base sale del `hoom.yaml` del árbol.**
  Es el del proyecto porque es el checkout donde corre hoom. Si ese checkout
  es la rama de otro, hay que usar una tarea o `--base`. Queda documentado.
- **`hoom agent` y `verify` siguen con la base de su árbol.** Fuera de esta
  spec.
