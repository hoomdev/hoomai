# Spec: la review aislada, con la evidencia completa y el modelo elegido

Estado: BORRADOR — pendiente de aprobación humana.
Tarea: `hoom task start review-aislada-y-modelo-elegido` (ya creada).
Origen: análisis de cupo de Henry del 2026-09-25 (Codex Plus, Claude Max) y
la lectura de gentle-ai (commit 520ed86). Re-expresa CA-109 (prompts
grandes), CA-160 (orden de las lentes), CA-337 (prefijo del pedido) y CA-354
(campos del diálogo). Contradice, solo para el reviewer y con justificación,
el no-goal de `codex-v2-y-review-cruzada.md` "pisar la config del usuario".

## Objetivo

Bajar el cupo que gasta `hoom review` **sin quitarle al reviewer nada de lo
que usa para revisar**, y darle al usuario control visible y registrado de
con qué provider, modelo y esfuerzo se revisa, incluido quien paga un solo
provider.

Medido en los logs de Codex del 2026-09-24 (plan Plus, 4 lentes): PR #32
gastó 28% de la ventana de 5 h y 5% de la semana; PR #33, 53% y 8%; C1, 92%
y 15%. Cada llamada del reviewer arranca con ~20k tokens fijos, que incluyen
los 4 servidores MCP de la config personal de Codex, y corre con el modelo y
el esfuerzo de esa config (`gpt-5.6-sol`, `xhigh`), que hoom no elige ni
registra. Cada lente además saca el diff por su cuenta y decide qué leer.

Seis piezas:

1. **Reviewer aislado**: su sesión no carga la config personal del provider
   (MCP, hooks, plugins, perfil). El resto de los roles no cambia.
2. **Evidencia armada una vez**: hoom congela el diff del candidato y el
   spec y se los da enteros a cada lente, antes de la línea de la lente.
3. **Negarse en vez de truncar**: si la evidencia pasa el tope, no corre
   ninguna lente.
4. **Risk primero**: risk, reliability, resilience, readability.
5. **Modelo y esfuerzo elegidos**: `--effort`, sección `review:` de
   `hoom.yaml` y campo en el Studio; se muestran antes de correr, quedan en
   el registro y se ven en la tarjeta, igual que si la review fue cruzada.
6. **Gasto por lente**: el consumo que informa el provider queda en cada
   pasada, en el resumen y en el registro, para medir el ahorro.

## No-goals

- No cambia cuándo corren 4 lentes (400 líneas, rutas de riesgo, solo
  documentación). `umbral-review-produccion` sigue archivada.
- No pasa a un reviewer sin herramientas ni de una sola llamada (otra spec,
  con la prueba contra los hallazgos conocidos de PR #32 y PR #33).
- No reanuda una review cortada por cupo ni junta lentes de corridas
  distintas (otra spec).
- No estima el costo antes de correr.
- No aísla ni cambia el esfuerzo de writer, test-writer ni los demás roles;
  tampoco de `hoom agent --role reviewer`: solo `hoom review`.
- No bloquea reviews no cruzadas: las muestra.
- No valida nombres de modelo ni niveles de esfuerzo: son vocabulario de
  cada provider y cambian entre versiones; el CLI rechaza lo que no conoce.
- No agrega tokens de razonamiento a `Usage` ni lee el modelo que informa el
  provider: se registra el que se pidió.

## Contratos

### `hoom.yaml`

```yaml
review:
  provider: codex          # opcional
  model: gpt-5.6-sol       # opcional, vocabulario del provider
  effort: high             # opcional, vocabulario del provider
  same_provider: false     # opcional; true = --same-provider
  isolated: true           # opcional; false solo por compatibilidad
  max_evidence_kib: 320    # opcional; tope de la evidencia
```

- `manifest.Manifest` gana `Review *ReviewPolicy` (`yaml:"review,omitempty"`).
  Estricta como `findings`: `review: clave desconocida "<k>" (validas:
  provider, model, effort, same_provider, isolated, max_evidence_kib)`;
  `max_evidence_kib` ≤ 0: `review: max_evidence_kib debe ser mayor que 0`.
- Precedencia, resuelta dentro de `reviewcmd.Run` (la CLI y el Studio la
  comparten): opción explícita > `hoom.yaml` > vacío. Modelo o esfuerzo
  vacío = el que el provider use por defecto; hoom no lo conoce y registra
  `""`. `isolated` ausente = `true`. `max_evidence_kib` ausente = 320.
  `same_provider: true` equivale a `--same-provider`: permite, no fuerza.

### Providers

- `Request` gana `Effort string` e `Isolated bool`; `Capabilities` gana
  `Effort` e `Isolation` (nombres `effort` e `isolation`, al final de
  `Names()`): `true` en codex y claude, `false` en el resto (gemini sigue sin
  capacidades). Pedirlos a un provider sin la capacidad es `ErrUnsupported`
  (la review corre `Strict`).
- Codex: `Isolated` agrega `--ignore-user-config`; `Effort` x agrega
  `-c model_reasoning_effort="x"` (codificado como el resto de `-c`). Ambos
  después de `--json`.
- Claude: `Isolated` agrega `--strict-mcp-config --setting-sources project`;
  `Effort` x agrega `--effort x`. Después de `-p`, antes del prompt.
- Sin esos campos el argv es idéntico al de hoy (CA-113, CA-152, CA-155).
- **Prompt grande por stdin** (codex y claude, cualquier rol): con un prompt
  de más de 16 KiB, `Invocation.Stdin` lleva el prompt y el argv no lo
  lleva; Codex termina en `-` y Claude no tiene prompt posicional. `runcmd`
  lo escribe en el stdin del proceso. Con 16 KiB o menos, todo como hoy.
- `runcmd.StartOptions` gana `Effort` e `Isolated` y los pasa al `Request`.

### Evidencia

```go
type Evidencia struct {
    Diff   []byte // parche del candidato
    Spec   []byte // texto del spec; vacío sin --spec
    Bytes  int    // len(Diff) + len(Spec)
    SHA256 string // hex de Diff seguido de Spec
}
func Evidence(dir, base string, git gitx.Info, spec string) (Evidencia, error)
```

- El diff es el candidato del veredicto: cada archivo de `git.ChangedFiles`
  fuera de `.hoom/`, del merge-base de la base contra el árbol de trabajo
  (commiteado o no), como parche unificado de git; un no rastreado va como
  parche de archivo nuevo; un binario, con la línea que git escribe para un
  binario, sin contenido. Se arma una sola vez, antes de la primera lente.
- `Bytes` mayor que el tope → no corre ninguna lente, no hay registro ni
  hallazgos, exit 1: `hoom review: NO ENTREGABLE - la evidencia (<N> KiB)
  pasa el tope (<M> KiB): hoom no la corta; parti el cambio o subi
  review.max_evidence_kib si el modelo del reviewer la aguanta`. Igual al
  tope, corre.

### El pedido

Idéntico para las 4 lentes hasta `=== fin de la evidencia ===`; lo único
que cambia por lente va después (así el provider puede reusar la caché):

```
Revisa el cambio de esta rama. La evidencia completa esta abajo, congelada por hoom (sha256 <hex>, <N> KiB): no vuelvas a sacar el diff; lee otros archivos solo por rangos y solo si hace falta.
Base: <base>. Tamano: <n> archivos, +<i>/-<d> lineas.
<linea del veredicto, como hoy>
Spec: <ruta>                      (solo con spec)
=== spec <ruta> ===               (solo con spec)
<texto del spec>
=== diff ===
<parche>
=== fin de la evidencia ===
Revisalo con la lente <lente>. Solo esa lente.
<las lineas de hoy desde "Registra cada hallazgo" hasta "No edites codigo...">
```

Desaparecen "El diff lo sacas vos: git diff ..." y la lista `Archivos:`.

### Lo que imprime y registra

- Antes de la primera lente, después de la línea `reviewer`:
  ```
    modelo      <m> | por defecto del provider (no elegido)
    esfuerzo    <e> | por defecto del provider (no elegido)
    aislado     si - sin la config personal del provider | no - review.isolated: false en hoom.yaml
    evidencia   <N> KiB (diff <a> + spec <b>), tope <M> KiB - sha256 <12 hex>
  ```
  (KiB redondeado hacia arriba.)
- Orden de las lentes: `risk, reliability, resilience, readability`.
- Tras cada pasada: `    gasto     entrada <i> - cache <c> - salida <o> - turnos <t>`,
  o `    gasto     el provider no informo consumo`. Al final, antes de
  `registro`: `  gasto       <n> lentes: entrada <I> - cache <C> - salida <O>`.
  Los números son los de `providers.Usage`, con la semántica de cada
  provider (CA-197, CA-198).
- `Pass` gana `usage` (omitido sin datos). `Result` gana `model`, `effort`,
  `isolated`, `evidence_bytes`, `evidence_sha256`.
- `Record` gana `model`, `effort` (`""` = no elegido), `isolated`,
  `evidence_bytes`, `evidence_sha256` y `usage`: una entrada
  `{lens, input_tokens, cached_input_tokens, output_tokens, turns}` por
  pasada con datos. El sobre sigue sin gasto (CA-334) y el tablero no suma
  el del registro.
- `--effort <e>` en `hoom review` y en su ayuda. El mensaje que rechaza una
  review no cruzada nombra también `review.same_provider: true`.

### Tablero y Studio

- `boardcmd.Providers` gana `reviewer_model` y `reviewer_effort`, del mismo
  registro del que sale `reviewer` (`""` = sin registrar).
- La tarjeta muestra junto al reviewer `<modelo> · <esfuerzo>`, con
  `sin registrar` por cada vacío. `no-cruzada` sigue con `misma CLI`,
  `cruzada-declarada` igual que hoy, y `desconocida` gana la marca
  `cruzada sin confirmar`.
- El diálogo de `pedir-reviewer` gana el campo esfuerzo. `POST /launch`
  acepta `effort`; con otra acción y `effort` no vacío responde 400
  `effort solo aplica a pedir-reviewer`.

## Casos límite y errores esperados

- 0 lentes (solo documentación): no se arma evidencia; `SIN REVISAR` como hoy.
- `--lens x`: la misma evidencia, una pasada.
- Sin `--spec`: evidencia sin bloque de spec.
- `--effort` con un provider sin `Effort`: `ErrUnsupported`, sin pasadas.
- `review.provider` sin `system_prompt`: el error de hoy.
- `same_provider: true` con otro provider instalado: revisa el otro (cruzada).
- Un modelo que no aguanta la evidencia aunque esté bajo el tope: el run
  falla y la review es `NO ENTREGABLE`, como cualquier run fallido.
- Registros viejos sin `model`/`effort`: la tarjeta dice `sin registrar`.
- Con los tamaños de hoy: C2 (diff 223 KiB + spec 38 KiB) entra con el tope
  por defecto; C1, C3 y C4 no, y hay que partirlos o subir el tope.

## Criterios de aceptación

- CA-394: `hoom.yaml` con `review:` parsea `provider`, `model`, `effort`, `same_provider`, `isolated` y `max_evidence_kib`; una clave desconocida y `max_evidence_kib: 0` fallan con los textos del contrato; sin la sección, `isolated` es `true` y el tope 320 KiB.
- CA-395: `reviewcmd.Run` resuelve provider, modelo, esfuerzo y `same_provider` con opción explícita > `hoom.yaml` > vacío, y el Studio (sin opciones) toma los de `hoom.yaml`; `same_provider: true` deja revisar con el mismo provider y el registro dice `no-cruzada`.
- CA-396: `Capabilities.Names()` termina en `effort,isolation` para codex y claude; gemini no tiene capacidades y opencode no gana ninguna; pedir `Effort` o `Isolated` con `Strict` a un provider sin la capacidad da `ErrUnsupported` con esos campos.
- CA-397: el argv de codex con `Isolated` y `Effort` "high" empieza con `exec --json`, contiene `--ignore-user-config` y `-c model_reasoning_effort="high"`; sin esos campos es idéntico al de hoy.
- CA-398: el argv de claude con `Isolated` y `Effort` "high" contiene `--strict-mcp-config`, `--setting-sources project` y `--effort high` entre `-p` y el prompt; sin esos campos cumple CA-113 tal cual.
- CA-399: con un prompt de más de 16 KiB, codex termina su argv en `-` y claude no lleva prompt posicional; `Invocation.Stdin` es el prompt entero y el proceso lo recibe por stdin; con 16 KiB o menos, CA-109 se cumple igual.
- CA-400: cada pasada de `hoom review` corre con `Isolated` (salvo `isolated: false`) y con el esfuerzo resuelto; `hoom agent` y los roles de escritura nunca llevan `Isolated` (CA-155).
- CA-401: `Evidence` arma el diff del candidato (rastreado commiteado o no, no rastreado como archivo nuevo, binario sin contenido, nada de `.hoom/`) más el spec, con `Bytes` y `SHA256`; las 4 pasadas reciben exactamente los mismos bytes de evidencia.
- CA-402: con la evidencia sobre el tope, `hoom review` no lanza ninguna pasada, no escribe registro ni hallazgos, sale con 1 e imprime el texto de NO ENTREGABLE del contrato; igual al tope, corre; el tope de `hoom.yaml` lo cambia.
- CA-403: el pedido empieza con `Revisa el cambio de esta rama.`, es idéntico entre lentes hasta `=== fin de la evidencia ===` inclusive, sigue con `Revisalo con la lente <lente>. Solo esa lente.` y conserva la línea de `hoom finding add` de CA-162; no contiene `El diff lo sacas vos` (re-expresa el prefijo de CA-337).
- CA-404: `Lentes` es `risk, reliability, resilience, readability`, en ese orden de ejecución y en `Record.Lenses` (re-expresa el orden de CA-160).
- CA-405: la salida muestra las líneas `modelo`, `esfuerzo`, `aislado` y `evidencia` del contrato, con `por defecto del provider (no elegido)` cuando no hay valor.
- CA-406: cada pasada imprime su `gasto` con los números de `providers.Usage` o `el provider no informo consumo`, el resumen imprime el total de las lentes, `Result.passes[].usage` los trae y el sobre de la review sigue sin gasto (CA-334).
- CA-407: el registro de review trae `model`, `effort`, `isolated`, `evidence_bytes`, `evidence_sha256` y `usage` por lente; un registro viejo sin esas claves se sigue leyendo.
- CA-408: la tarjeta trae `providers.reviewer_model` y `providers.reviewer_effort` del registro de la review; `tablero.js` muestra `sin registrar` para un vacío y `cruzada sin confirmar` para `desconocida`, y conserva `misma CLI`.
- CA-409: el diálogo de `pedir-reviewer` tiene provider, modelo, esfuerzo, presupuesto y pedido (re-expresa CA-354); `POST /launch` pasa `effort` a la review y responde 400 con `effort solo aplica a pedir-reviewer` para otra acción.
- CA-410: el contrato 06, la ayuda y el README dicen el aislamiento, el orden, la evidencia y `review:`. [verifica: grep -q "risk, reliability, resilience, readability" internal/agents/assets/agents/06-reviewer.md && cmp -s internal/agents/assets/agents/06-reviewer.md .hoom/agents/06-reviewer.md && grep -q "max_evidence_kib" README.md && grep -q -- "--effort" README.md]
- CA-411: el `hoom.yaml` de hoomai declara el reviewer de hoy de forma explícita. [verifica: grep -q "^review:" hoom.yaml && grep -q "model: gpt-5.6-sol" hoom.yaml && grep -q "effort: xhigh" hoom.yaml]

## Decisiones

- **Aislar solo al reviewer, contra el no-goal de `codex-v2`.** Ese no-goal
  protegía la config del usuario para todos los roles. Para el reviewer hoy
  cuesta ~20k tokens por llamada en herramientas que no usa para revisar, y
  lo único que la config aportaba (modelo y esfuerzo) pasa a elegirse en
  `hoom.yaml`, visible y registrado. Los roles de escritura siguen igual
  (CA-155). `isolated: false` queda para quien use un `model_provider`
  propio en la config de Codex.
- **La evidencia va en el pedido, no en un archivo.** Así cada lente tiene
  el candidato entero en su contexto y ninguna puede saltarse archivos y
  quedar registrada como una de las 4 lentes. Va antes de la línea de la lente
  para que las 4 sesiones compartan el prefijo.
- **Tope por defecto de 320 KiB.** Son ~80-100k tokens, alrededor de un
  tercio de la ventana de Codex (258k), y dejan lugar para que el reviewer
  lea y razone. Negarse en vez de cortar sigue a gentle-ai: una evidencia
  cortada deja dar un resultado limpio sobre una vista parcial.
- **Risk primero, reliability segunda.** Los dos hallazgos high de C1 los
  dio risk; si el cupo corta, la lente que más rinde ya corrió. Reliability
  sigue en la posición 2.
- **Modelo y esfuerzo sin validar y registrados como se pidieron.** La lista
  válida es de cada provider y cambia con sus versiones; hoom no la copia.
- **El `hoom.yaml` de hoomai declara `gpt-5.6-sol` con `xhigh`**, lo mismo
  que hoy: la primera medición aísla el efecto de esta spec. Bajar el
  esfuerzo o cambiar de modelo es otra decisión, con la prueba contra
  PR #32 y PR #33.
- **Stdin por encima de 16 KiB y no siempre**: el argv tiene límite (128 KiB
  por argumento en Linux, 32 KiB la línea entera en Windows); los prompts
  chicos siguen por argv y nada más cambia.

## Riesgos y deuda aceptada

- **El ahorro no está garantizado.** La evidencia viaja en cada llamada; en
  un diff grande una lente puede costar lo mismo que hoy o más, porque ahora
  lleva el cambio entero. Lo dirá el `gasto` por lente en C2 y en cambios del
  tamaño recomendado (1-2k líneas).
- El `gasto` usa la semántica de cada provider (CA-197/198): se compara
  dentro del mismo provider, no entre providers. El razonamiento no aparece;
  el efecto del esfuerzo se ve en los logs del provider.
- Que el provider reuse la caché entre lentes depende de él.
- El `AGENTS.md` global de Codex puede seguir cargando: `--ignore-user-config`
  solo saltea `config.toml`.
- Sin MCP, el reviewer pierde `codebase-memory-mcp`; conserva su shell.
- Un modelo de contexto chico (200k) con una evidencia cerca del tope queda
  justo: se baja `max_evidence_kib`.
- C1, C3 y C4 no entran enteros con el tope por defecto.
