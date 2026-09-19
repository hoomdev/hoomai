# Spec: gate opcional findings_open

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start gate-findings-open`.
Origen: roadmap del README ("Gate opcional `findings_open`: rojo con hallazgos
high abiertos — primero ver cómo se usa el ciclo antes de darle poder de
bloqueo"). El ciclo ya se usó en serio: 64 hallazgos en `.hoom/findings/`,
la review cruzada de las Specs D+E y la de verify estricto. Hoy `hoom status`
muestra "hallazgos: sin abiertos" o "N abierto(s), M high", pero nada bloquea.
Es prerrequisito de la cabina visual: la condición de salida de la columna
Review es "cero hallazgos graves abiertos", y eso lo tiene que imponer
`verify`. El tablero no lo calcula por su cuenta.

## Objetivo

Que un proyecto pueda decidir, en `hoom.yaml`, que un hallazgo grave abierto
ponga ROJO el veredicto. Cuatro piezas:

1. **Gate sintético `findings_open`**, de la misma familia que `spec_lint`,
   `spec_trace`, `spec_approved` y `ratchet`: no lo declara `gates:`, no se
   puede seleccionar con `--gate`, y su nombre queda reservado en
   `internal/verifycmd/args.go`. FAIL si hay hallazgos abiertos de la
   severidad que bloquea, con la lista de ids y lente en las notas. PASS con
   "0 abiertos". Los de severidad menor se informan en la nota y no bloquean.

2. **Opt-in desde `hoom.yaml`**:

   ```yaml
   findings:
     block_on: high
   ```

   Si `findings.block_on` no está, el gate no corre y verify queda igual que
   hoy. Si corre, es `required`: no existe un `findings_open` informativo,
   porque para informar ya está `hoom status`.

3. **Una sola definición de "abierto"**, leída de `internal/finding` y
   corregida ahí, para que el gate, `hoom finding list`, `hoom status` y el
   Studio cuenten lo mismo. CORREGIDO y REFUTADO no cuentan. Una resolución
   sin evidencia, con un estado inventado o ilegible no cierra nada: el
   hallazgo sigue abierto. Si el hallazgo tiene tarea (campo `task`, Spec B1),
   `hoom verify --spec` cuenta los de la tarea de ese spec. Sin `--spec`
   cuenta todos.

4. **Se ve igual que los otros sintéticos**: eventos vivos (`hoom status`,
   `--watch`), el detalle del veredicto en el Studio y la tendencia de
   `hoom report`. `hoom status` dice además si el gate está activo y cuántos
   bloquean. El Studio deja de afirmar que los hallazgos "jamás tocan un
   veredicto".

## No-goals

- Que `hoom check` o `hoom task done` lean hallazgos. Siguen comparando la
  huella con el último veredicto completo. Un hallazgo registrado después de
  un verde se ve en el próximo `verify` (ver Riesgos).
- Juzgar la evidencia de una resolución. El gate cuenta estados. Que una
  refutación sea buena lo decide el ciclo del Refutador y la review humana.
  La resolución queda en Git con autor y fecha.
- Un flag `--task` en `verify`. La tarea sale del spec (ver Decisiones).
  `verify-args-estrictos.md` acaba de cerrar los argumentos del verbo.
- Umbral por lente, por archivo o por antigüedad. `block_on` es una
  severidad y nada más.
- Que el tablero de la cabina (Studio v5) exista. Esta spec le da el dato:
  el gate en el veredicto.
- Cambiar `hoom review`. Sigue sin correr verify ni check, así que sus
  hallazgos no le cambian el exit code.
- Rechazar en `manifest.Load` un gate de proyecto que use un nombre
  reservado. Hoy no se hace con ninguno de los cuatro y esta spec no abre
  ese frente: solo impide seleccionarlo con `--gate`.

## Contratos

### `hoom.yaml`: bloque `findings`

- Campo nuevo del manifiesto, solo a nivel proyecto (los perfiles no lo
  traen): `findings: { block_on: <severidad> }`.
- `block_on` acepta `low`, `medium` o `high`. Se normaliza igual que
  `hoom finding add` normaliza la severidad: minúsculas y sin espacios, así
  que `HIGH` = `high`.
- `block_on` es un umbral, "esa severidad o mayor":
  - `high` bloquea solo high.
  - `medium` bloquea medium y high.
  - `low` bloquea todo.
- `findings` ausente, `findings: {}` o `block_on:` sin valor (null en YAML):
  el gate no corre.
- `block_on` con cualquier otro valor, incluido `""`, o una clave
  desconocida dentro de `findings` (por ejemplo `blockon`): `hoom.yaml`
  inválido. El error nombra la clave o el valor y los valores válidos, y se
  produce en `manifest.Load`, así que ningún verbo corre. Un typo que apague
  en silencio un gate que bloquea es el bug que este contrato impide.

### "Abierto": la definición única (`internal/finding`)

Un hallazgo está **abierto** salvo que tenga una resolución **válida**. Una
resolución es válida si, y solo si:

- el archivo `.res.json` se lee como JSON y trae `finding_id` no vacío;
- `as`, normalizado (minúsculas y sin espacios), es `corregido` o
  `refutado`;
- `evidence` no está vacía ni es solo espacios.

Una resolución que no cumple eso se ignora y deja un aviso: "resolucion
invalida <archivo>: <motivo>; el hallazgo <id> sigue abierto". Una ilegible
ya se ignoraba con aviso y sigue igual. `finding.List` aplica esta regla, así
que `hoom finding list`, `/api/findings`, `hoom status` y el gate ven el
mismo estado. Hoy `List` copia `as` tal cual: una resolución con `as` vacío o
inventado saca al hallazgo de `--open` y `status` dice "sin abiertos".
Ese bug se corrige acá.

Que el código haya cambiado desde el hallazgo (`code_changed`) no lo cierra.
Cerrar exige resolución.

`hoom finding resolve <id>` sobre un hallazgo que ya tiene un `.res.json`
inválido se niega sin sobreescribir: "la resolucion existente de <id> es
invalida (<motivo>): el hallazgo sigue ABIERTO. Accion: repara o borra a mano
<ruta> (el cambio queda en el diff de Git) y vuelve a resolver". Borrar
evidencia append-only es un acto humano y visible, no un camino del binario.

### Gate `findings_open`

Sintético y `required`. Corre en `hoom verify` y en `POST /api/verify` (es la
misma función, `verifycmd.Run`) cuando:

- el manifiesto declara `findings.block_on`, **y**
- la corrida no es `--gate`. Una corrida `--gate` es un diagnóstico de los
  gates elegidos, igual que con el trinquete. Corre tanto en corridas por
  diff como en `--full`.

**Alcance.** El gate lee `.hoom/findings/` del árbol donde corre verify
(`m.Dir`).

- Sin `--spec`, cuentan todos los hallazgos, sea cual sea su tarea.
  Alcance: "todos los hallazgos". Scope del gate: `full`.
- Con `--spec <ruta>`, la tarea del spec es el nombre del archivo sin `.md`
  (`.hoom/specs/gate-findings-open.md` → `gate-findings-open`). Cuentan los
  hallazgos con `task` igual a esa tarea y los que no tienen tarea. Los de
  otra tarea no cuentan y la nota dice cuántos quedaron afuera. Alcance:
  "tarea <slug> + sin tarea". Scope del gate: `spec`.

**Resultado:**

- **PASS**: cero abiertos que bloquean. `notes` empieza con `0 abiertos` y
  sigue con el umbral, el alcance y, si hay, cuántos abiertos no bloquean
  por severidad, por ejemplo `no bloquean: 1 medium, 2 low`.
- **FAIL**: uno o más abiertos que bloquean. `notes` trae la cuenta, el
  umbral, cada id con su lente entre corchetes (`[sin lente]` si está vacía),
  el alcance y los que no bloquean. `output_tail` trae una línea por hallazgo
  que bloquea (id, severidad, lente, archivo y descripción recortada) y la
  acción exacta: `hoom finding resolve <id> --as corregido --evidence
  "..."`, o `--as refutado` con la evidencia que lo tumba, y después volver a
  correr `hoom verify`.
- **ERROR** (fail-closed, el veredicto queda ROJO): un archivo de hallazgo
  ilegible, o un hallazgo con severidad fuera de low|medium|high. No se
  puede saber si bloquea, de qué tarea es ni qué severidad tiene, así que no
  se asume que no. `notes` nombra cada archivo y la acción: repararlo a mano,
  porque es evidencia versionada.
- Los avisos de resoluciones ignoradas van en `output_tail` en cualquier
  resultado.

**Posición en el veredicto:** después de `spec_lint`, `spec_trace` y
`spec_approved`, y antes de los gates del proyecto. Primero el estado humano
y de review, que es instantáneo; después el árbol; al final el trinquete.

**Eventos vivos:** `gate_start` y `gate_end` con su scope, como cualquier
gate, y suma uno al total declarado en `verify_start`.

**Nombre reservado:** `findings_open` entra en `gateSintetico`. `--gate
findings_open` es error de uso (exit 2) aunque `hoom.yaml` declare un gate
con ese nombre, y no aparece en la lista de seleccionables.

### `hoom status`

- `Snapshot.findings` gana dos campos:
  - `block_on`: el umbral del manifiesto; vacío si el gate está apagado.
  - `blocking`: cuántos abiertos llegan al umbral, con alcance "todos",
    porque status no tiene spec.
- `open` y `open_high` se calculan con la definición única.
- Texto con el gate activo: la línea de hallazgos agrega `gate findings_open:
  N bloquean (block_on: high)`, en rojo si N > 0, y aparece aunque no haya
  abiertos. Con el gate apagado, la línea queda como hoy.
- El progreso en vivo del gate sale de los eventos, como el de los demás
  gates, sin código propio.

### Studio

- El detalle del veredicto (`/api/verdicts/{id}`) y la tendencia
  (`/api/report`) muestran `findings_open` con sus notas y su `output_tail`
  con el mismo render que usan todos los gates.
- `GET /api/status` gana `findings_block_on`.
- La nota del cajón de hallazgos deja de decir "jamás tocan un veredicto".
  Con `findings_block_on` vacío dice que los hallazgos no bloquean. Con valor,
  dice que los abiertos de esa severidad o mayor ponen ROJO a verify (gate
  `findings_open`).
- El Studio no recalcula quién bloquea: esa respuesta está en el gate del
  veredicto.

### Documentación

- El doc del paquete `finding` y el comentario del campo `Task` dejan de
  decir que verify nunca los lee.
- README: manifiesto (`findings.block_on`), la tabla de comandos (`finding`)
  y el roadmap, que pierde este ítem.
- `hoom help` y `UsageText` de verify mencionan el gate. El bloque de uso
  sigue siendo una sola constante.

## Casos límite y errores esperados

- `block_on: high` con cero hallazgos en el repo (o sin `.hoom/findings/`):
  PASS, `0 abiertos`.
- Un high con `.res.json` de `evidence: "   "`: sigue abierto, el gate da
  FAIL y el aviso sale en `output_tail`. `hoom finding list --open` lo lista
  y `hoom status` lo cuenta.
- Un high con `.res.json` de `as: "wontfix"`: igual que sin evidencia.
- Un high con `.res.json` ilegible: sigue abierto, como ya pasa hoy con
  resoluciones rotas.
- Un `.json` de hallazgo roto: ERROR con el nombre del archivo, aunque exista
  una resolución que parezca cerrarlo. Sin leer el hallazgo no se sabe qué
  cierra.
- Un hallazgo con `"severity": "critical"` escrito a mano: ERROR, no se
  degrada a high.
- `--spec` con un nombre de archivo que no es un slug válido (mayúsculas,
  espacios): ningún hallazgo puede tener esa tarea, porque `Register` valida
  la forma del slug. Cuentan solo los que no tienen tarea y la nota muestra
  el alcance real.
- `--spec` de la tarea A con un high abierto de la tarea B: PASS, y la nota
  dice `1 de otras tareas no cuentan`. El mismo verify sin `--spec` es FAIL.
- `hoom verify --gate test` con `block_on` configurado: el veredicto
  (PARCIAL) no trae `findings_open` y el total vivo no lo cuenta.
- El sobre (`hoom agent`) de un rol reviewer que registra un high con el
  gate activo cierra NO ENTREGABLE en el paso verify. Es correcto: el árbol
  no es entregable. `hoom review` no corre verify y no cambia.
- Un rol bajo el sobre que edita `hoom.yaml` para apagar el gate: ya es
  manipulación por el piso universal de la Spec B, que corta antes de
  verify. Nada nuevo que hacer.
- Hallazgo registrado desde el checkout principal con `--task X`: el
  worktree de X no lo ve hasta integrar, porque el gate lee el árbol donde
  corre verify. Queda declarado en Riesgos.

## Criterios de aceptación

- CA-248: sin `findings.block_on` en `hoom.yaml` (bloque ausente, `findings:
  {}` o `block_on:` null), `hoom verify` no emite `findings_open` ni en el
  veredicto ni en los eventos vivos, y el total de `verify_start` no lo
  cuenta.
- CA-249: `block_on` fuera de low|medium|high (incluido `""`) o una clave
  desconocida dentro de `findings` hace fallar `manifest.Load` con un error
  que nombra el valor o la clave y los valores válidos. `hoom verify` no
  escribe veredicto. `block_on: HIGH` se acepta como `high`.
- CA-250: con `block_on: high` y un hallazgo high abierto, `findings_open`
  es FAIL, `required: true`, y el veredicto es ROJO. `notes` contiene el id y
  la lente del hallazgo, y `output_tail` contiene `hoom finding resolve <id>`.
- CA-251: con `block_on: high`, cero high abiertos y un medium y un low
  abiertos, `findings_open` es PASS, `notes` empieza con `0 abiertos` y
  menciona `1 medium` y `1 low` como que no bloquean, y el veredicto es VERDE.
- CA-252: el umbral incluye las severidades mayores: con `block_on: medium`
  un medium abierto da FAIL y un low solo no. Con `block_on: low` un low
  abierto da FAIL.
- CA-253: un high con resolución `corregido` o `refutado` con evidencia no
  cuenta. Un high cuya resolución tiene evidencia vacía o en blanco, o `as`
  fuera de corregido|refutado, sí cuenta (FAIL). En ese caso `finding.List`
  lo devuelve con estado `abierto`, lo incluye con `openOnly` y emite un
  aviso que nombra el archivo.
- CA-254: un archivo de hallazgo ilegible o con severidad fuera de
  low|medium|high da `findings_open` ERROR, nombrando el archivo, y el
  veredicto ROJO.
- CA-255: con `--spec .hoom/specs/<slug>.md` cuentan los hallazgos con
  `task` = `<slug>` y los que no tienen tarea. Un high abierto de otra tarea
  no bloquea y la nota dice cuántos quedaron fuera. El scope del gate es
  `spec`. Sin `--spec` el mismo high bloquea y el scope es `full`.
- CA-256: una corrida `--gate` no incluye `findings_open` aunque `block_on`
  esté configurado. `--gate findings_open` es `*cliargs.UsageError` (exit 2)
  aunque `hoom.yaml` declare un gate con ese nombre, y el mensaje no lo ofrece
  entre los seleccionables.
- CA-257: con el gate activo, verify emite `gate_start` y `gate_end` de
  `findings_open` con su scope y el total de `verify_start` lo cuenta. En el
  veredicto va después de los gates de spec y antes del primer gate del
  proyecto.
- CA-258: `hoom finding resolve` sobre un hallazgo con un `.res.json`
  inválido se niega con un error que dice que sigue abierto y nombra el
  archivo, y el `.res.json` queda byte a byte igual.
- CA-259: `hoom status --json` expone `findings.block_on` y
  `findings.blocking` con la definición única, y el texto muestra `gate
  findings_open` con cuántos bloquean. Sin `block_on`, `block_on` va vacío y
  el texto no menciona el gate.
- CA-260: en el Studio, `/api/verdicts/{id}` devuelve `findings_open` con sus
  notas, `/api/status` expone `findings_block_on`, y la UI embebida ya no
  contiene "jamás tocan un veredicto".
- CA-261: E2E con el binario real en un repo temporal con `block_on: high`
  y un hallazgo high de la tarea del spec: `hoom verify --spec` sale con exit
  1 y veredicto ROJO. Después de `hoom finding resolve <id> --as refutado
  --evidence "..."`, el mismo verify sale con exit 0 y veredicto VERDE.
- CA-262: el `hoom.yaml` de hoomai declara `findings.block_on: high` (dogfood). [verifica: go run ./cmd/hoom status --json | grep -q '"block_on": "high"']

## Decisiones

- **El gate vive en `internal/finding`** (`finding.Gate`), como
  `ratchet.Gate` y `spec.Gates` viven con el dato que juzgan. La definición
  de "abierto" y el gate que la aplica no pueden divergir si están en el
  mismo paquete. verifycmd solo decide cuándo corre y dónde va.
- **Arreglar `List` en vez de filtrar en el gate.** Si el gate tuviera su
  propia regla, `hoom status` diría "sin abiertos" mientras verify da ROJO:
  dos cerebros. La regla se corrige en la fuente y todos los que leen
  hallazgos la heredan.
- **Opt-in por proyecto, no por perfil.** Bloquear por hallazgos es una
  decisión de proceso del equipo, no del stack.
- **`required` fijo.** El roadmap pedía darle poder de bloqueo. Un
  `findings_open` no requerido repetiría lo que ya dice `hoom status`.
- **Umbral, no lista.** `block_on: medium` lee naturalmente como "medium o
  peor". Una lista (`[high, low]` sin medium) no tiene un caso de uso
  sensato.
- **La tarea sale del nombre del spec**, no de `HOOM_TASK` ni de un flag. Es
  determinista, queda escrita en el veredicto (el campo `spec`) y coincide
  con el flujo real (`.hoom/specs/<slug>.md` + `hoom task start <slug>`,
  como en B1). Una variable de entorno haría depender el veredicto de estado
  invisible.
- **Con `--spec`, los hallazgos sin tarea cuentan.** "Si el hallazgo tiene
  campo task, solo los de esa tarea": el filtro excluye los que se pueden
  probar ajenos, es decir, los que tienen otra tarea. Un hallazgo sin tarea
  puede ser de cualquiera, y el flujo más común hoy (review desde una sesión
  de Orca, sin `hoom task` ni `HOOM_TASK`) registra hallazgos sin tarea. Si
  no contaran, el gate sería decorativo justo en el `verify --spec` con el
  que se cierra cada tarea. El costo es que un high sin dueño bloquea todos
  los `verify --spec` de ese árbol, y esa presión para triarlo es buscada.
  Hoy hoomai tiene 0 high abiertos.
- **No corre con `--gate`**, igual que el trinquete. Los gates que corren por
  configuración no entran en un diagnóstico de gates elegidos. Los de spec sí
  corren con `--gate --spec` porque el flag los pide explícitamente.
- **Ilegible = ERROR, severidad desconocida = ERROR.** Un gate con poder de
  bloqueo falla cerrado, igual que el trinquete con una base corrupta.
  `hoom status` y `finding list` siguen rotulando sin romper, porque
  informan y no bloquean.
- **`resolve` no sobreescribe una resolución inválida.** Sería reescribir
  evidencia append-only desde el binario. El arreglo es humano, a mano y
  visible en el diff.
- **Dogfood (CA-262):** hoomai se activa el gate a sí mismo con
  `block_on: high`. Si no lo querés en este repo todavía, borralo al revisar
  y se va el CA con él.

## Riesgos y deuda aceptada

- **`check` no ve hallazgos posteriores al veredicto.** Los hallazgos están
  fuera de la huella a propósito, así que registrar uno no cambia el
  candidato. Un high registrado después de un verde deja `hoom check` y
  `hoom task done` en verde hasta el próximo `verify`. El gate certifica el
  estado de los hallazgos en el momento del veredicto. Seguimiento
  candidato: que `check` compare las resoluciones y altas posteriores al
  veredicto de referencia cuando el gate está activo. Queda fuera de esta
  spec para no convertir `check` en un segundo cerebro sin diseñarlo.
- **El gate cuenta estados, no juzga evidencia.** `--as refutado --evidence
  "no aplica"` lo pone verde. La defensa es el Refutador, la review humana y
  que la resolución queda en Git con autor.
- **Los hallazgos viven en el árbol donde se registran.** Uno registrado
  desde el checkout principal con `--task X` no está en el worktree de X
  hasta integrar. Dentro de `hoom agent --task X` el run corre en el
  worktree de X, así que ahí coinciden.
- **Tarea = nombre del spec.** Si alguien corre `hoom task start foo` con
  `.hoom/specs/bar.md`, los hallazgos de `foo` no cuentan en `verify --spec
  .hoom/specs/bar.md` (sí los que no tienen tarea). La nota del gate muestra
  el alcance real para que el desajuste se vea.
- **Un reviewer bajo el sobre cierra NO ENTREGABLE si registra un high.** Es
  semánticamente correcto, pero el exit 1 puede leerse como "falló el
  reviewer". La nota del sobre ya cita el gate rojo.
- **El punto del Studio** (`find-dot`) sigue contando high en JS, sin mirar
  `block_on`. Ahora lee el estado de la definición única, pero es la vista
  de hoy y no la de la cabina, que debe leer el gate del veredicto.
