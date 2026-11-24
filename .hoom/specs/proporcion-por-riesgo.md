# Spec: el spec, las pruebas y el modelo en proporción al riesgo

Estado: BORRADOR — pendiente de aprobación humana.
Tarjeta: `proporcion-por-riesgo`. Intake: `.hoom/intake/rfc-calibracion-por-riesgo.md`, CAL-7, CAL-6 y CAL-11 con los ajustes de 10.2, y las preguntas P1, P4 y P7 (10.4 las deja a este spec). Criterios CA-791 a CA-808.

## Objetivo

Que lo que hoom pide antes de que exista el diff (el spec, las pruebas y el modelo de cada rol) se mida con el riesgo que una persona declaró en la tarjeta.

Hoy nada de eso varía:

- El arquitecto tiene una regla de prompt («se lee en 3 minutos»), y `spec_lint` no cuenta palabras. Los 49 specs de hoomAI pasan de 800 palabras, y los de las tres últimas tarjetas tienen entre 2,600 y 3,900.
- El test-writer pide pruebas hostiles y property-based en toda tarjeta. En las tarjetas 1 a 3, cada criterio tiene entre 10 y 110 líneas de prueba que lo citan.
- Todos los roles corren en el modelo por defecto del provider, porque `hoom init` no escribe `agents:` y nada varía por riesgo. `hoom agent` y el piloto no dicen con qué modelo lanzan.

Este spec hace tres cosas, todas con el riesgo **declarado** de la tarjeta (10.2):

1. `spec_lint` mide las palabras y los criterios del spec contra topes por riesgo de `hoom.yaml`. Pasar el tope es rojo y la aprobación se niega, salvo que una persona suba el tope de esa tarjeta con un motivo que queda en el item.
2. El contrato del test-writer acota las pruebas en `bajo` y `normal`, y `spec_trace` reporta las citas por criterio, con un aviso que no bloquea.
3. `agents.<rol>` acepta valores por riesgo y modelos en niveles (`fuerte`, `medio`, `barato`), que hoom traduce por provider en un solo lugar. `hoom init` los escribe, y todo lanzamiento imprime sus valores con su origen.

Protege I9 (el riesgo y los topes salen de la tarjeta y de `hoom.yaml`, nunca de un modelo), I7 (la aprobación vuelve a ser de algo que se lee), I3 (el test-writer sigue ciego en todo riesgo), I4 (la review sigue cruzada) e I1 (un modelo más barato no cambia quién decide el verde).

## No-goals

- Usar el riesgo efectivo para algo de esto: el spec, las pruebas y el modelo ocurren antes del diff (10.2). Si el diff sube el efectivo, hoom ya lo avisa y una persona sube el declarado.
- Medir el UI-spec (`<slug>-ui.md`): ningún gate lo lee, y el designer no se lanza desde la tarjeta. Este spec solo cambia su contrato.
- Lanzar al designer desde la tarjeta o el piloto.
- Contar funciones de prueba por lenguaje: hoom cuenta citas (ver Decisiones).
- Un tope de citas propio de la tarjeta.
- Valores por riesgo en `review:`: las lentes ya varían por riesgo.
- Redefinir los niveles en `models.<provider>`: un proyecto que quiere otro modelo escribe su nombre.
- El presupuesto de la tarjeta según su riesgo: `budget_usd` por riesgo ya topa cada run.
- Cambiar el `hoom.yaml` de hoomAI: la persona agrega `agents:` y `spec:` si los quiere.

## Contratos

### La medida del spec

- **Las palabras** del spec son las secuencias sin blancos del archivo entero, como las da `strings.Fields` de Go. Es lo mismo que `wc -w` sobre un texto sin caracteres de control. Cuentan los encabezados, las tablas, el código, los marcadores `verifica` y todas las secciones.
- **Los criterios** son los ítems `- CA-n: ...` de la sección de criterios de aceptación, los que `spec.Criteria` devuelve con texto. Un id citado en otra sección no cuenta.
- `spec.Medir(ruta) (Medida, error)` devuelve `Medida{Palabras, Criterios int}`. `spec.Lint` y `spec.Criteria` no cambian (CA-311).

### Los topes en `hoom.yaml`

```yaml
spec:
  topes:
    bajo:   { palabras: 400,  criterios: 4,  citas_por_criterio: 10 }
    normal: { palabras: 800,  criterios: 8,  citas_por_criterio: 10 }
    alto:   { palabras: 2000, criterios: 15, citas_por_criterio: 0 }
```

- Esos son los valores por defecto. Sin la sección, o sin un riesgo o una clave, vale el valor por defecto de esa clave.
- Cada valor es un entero mayor o igual que 0, y 0 es sin tope.
- Los topes salen del `hoom.yaml` que carga el comando, como los gates.
- Los errores van con `hoom.yaml invalido: ` delante, y `hoom verify` no corre gates ni escribe veredicto:
  - `spec: clave desconocida "<k>" (validas: topes)`;
  - `spec.topes: clave desconocida "<k>" (validas: bajo, normal, alto)`;
  - `spec.topes.<r>: clave desconocida "<k>" (validas: palabras, criterios, citas_por_criterio)`;
  - `spec.topes.<r>.<k>: <v> no es un entero mayor o igual que 0 (0 = sin tope)`;
  - `<donde> debe ser un mapa`, con `<donde>` igual a `spec`, `spec.topes` o `spec.topes.<r>`.

### La tarjeta del spec y su tope

- La tarjeta de `.hoom/specs/<slug>.md` es `.hoom/items/<slug>.yaml`, leída con `item.RiesgoDe` (primero el checkout principal). Su riesgo es el declarado, y sin clave vale `normal`.
- Un spec en otra ruta, o sin item, no tiene tarjeta: vale `normal`.
- Con la tarjeta ilegible, la medida no se hace: `spec_lint` queda en `error` con `no se puede leer la tarjeta <slug>: <error>`, y la aprobación se niega con ese texto.
- El tope de cada medida es el de la tarjeta, si lo tiene; si no, el de `spec.topes.<riesgo>`.
- `hoom item set <slug> --tope-palabras <n> --motivo "<por que>"` escribe en el item `tope_palabras`, `tope_motivo`, `tope_por` y `tope_en`. Con `--tope-criterios <n>` escribe `tope_criterios`, y los dos pueden ir juntos.
  - `tope_por` es el `user.name <user.email>` de git, y `tope_en` va en UTC, RFC 3339, como `riesgo_por` y `riesgo_en`.
  - `<n>` es un entero mayor o igual que 0, y 0 es sin tope para esa tarjeta. Otro valor sale con exit 1 y `--tope-palabras tiene que ser un entero mayor o igual que 0` (o `--tope-criterios`).
  - Sin `--motivo`, o con un motivo en blanco, sale con exit 1 y `item set: cambiar el tope del spec pide --motivo "<por que>"`. `--motivo` sin tope sale con exit 1 y `item set: --motivo va con --tope-palabras o --tope-criterios`.
  - Con `HOOM_ROLE`, sale con exit 1 como con `--riesgo` (CA-617): un rol no sube su propio tope.
  - Ningún error toca el archivo.
- `item.Parse` acepta las cinco claves. Un `tope_palabras` o `tope_criterios` que no es un entero mayor o igual que 0 da `tope_palabras tiene que ser un entero mayor o igual que 0` (o `tope_criterios`).
- El historial del tope es el del item en git, que la historia de la tarjeta ya muestra.

### `spec_lint`, la aprobación y `hoom spec status`

- **Qué se mide.** Solo un spec **sin aprobación vigente** se mide contra su tope: uno sin aprobar, o uno cambiado después de aprobarse. Un spec con aprobación vigente queda como está (P7).
- **El rojo.** Un spec que pasa su tope deja `spec_lint` en `fail`, con una línea en OutputTail por cada medida que pasa:
  - `el spec tiene <p> palabras y el tope con riesgo <r> es <t>: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set <slug> --tope-palabras <p> --motivo "<por que>"`;
  - lo mismo con `criterios` y `--tope-criterios <c>`;
  - con el tope de la tarjeta, `el tope de la tarjeta es <t>` en lugar de `el tope con riesgo <r> es <t>`;
  - sin tarjeta, la línea termina en `: reduce el alcance o sube spec.topes.<r>.<clave> en hoom.yaml`.
  El tope exacto pasa: 800 palabras con tope 800 es verde.
- **La nota en pass** es `<medida de palabras> · <medida de criterios> · riesgo <r> · <n> ids CA-n`, más el conteo de criterios con marcador `verifica` que la nota ya trae cuando hay comandos.
  - Cada medida se escribe `<p> palabras de <t>`, `<p> palabras de <t> de la tarjeta` o `<p> palabras sin tope`, y lo mismo con criterios.
  - Con aprobación vigente, la nota termina en ` · aprobado: el tope no se mide`.
- **La aprobación.** `hoom spec approve`, `hoom aprobar` y la aprobación del Studio y del tablero se niegan con un spec que pasa su tope. No escriben registro, y responden con error y las líneas de `spec_lint`: exit 1 en la terminal, el mismo error que hoy en el Studio.
- **`hoom spec status <ruta>`** suma una línea después de la de hoy: `hoom spec: <medida de palabras> · <medida de criterios> (riesgo <r>)`.
  - Sin tarjeta: `(riesgo normal, sin tarjeta)`.
  - Con aprobación vigente, suma ` · aprobado: el tope no se mide`.
  - Con la tarjeta ilegible: `hoom spec: no se puede medir el tope: no se puede leer la tarjeta <slug>: <error>`.
  - Sus exit codes no cambian (CA-12).
- **El tablero.** Un spec sin aprobación vigente que pasa su tope da la columna `arquitecto`, con las líneas de `spec_lint` en `missing`, como un issue de lint.

### Las pruebas según el riesgo

- **La nota de riesgo.** hoom agrega al final del SystemPrompt, después de una línea en blanco:
  - del test-writer, después del párrafo del árbol ciego: `Riesgo declarado de la tarjeta: <r>.`;
  - del arquitecto: `Riesgo declarado de la tarjeta: <r>. Tope del spec: <t> palabras y <c> criterios.`, con `palabras sin tope` o `criterios sin tope` cuando el tope es 0.
  El riesgo es el declarado de la tarjeta de `--task`. Con la tarjeta ilegible vale `alto`, y sin tarjeta, `normal`. Los demás roles no reciben nota, y el contrato en `.hoom/agents/` no se modifica (CA-182).
- **Contrato 05.** Los puntos 2 y 3 quedan así:
  - 2: por cada caso límite que el spec enumera, un test hostil;
  - 3: solo con riesgo alto, además, un test hostil por cada caso límite que encuentre aunque el spec no lo enumere, y property-based donde el stack lo permita.
  Con riesgo `normal` o `bajo`, nada más: sin casos inventados ni property-based. Sin la nota, trabaja como con riesgo alto.
- **Las citas.** Las citas de un criterio son las líneas de los archivos que `spec_trace` escanea donde aparece su token entero. Una línea cuenta una vez por criterio.
  - `spec.TraceResult` suma `Citas map[string]int`.
  - La nota en pass suma ` · citas por criterio: <id> <n>, ...` en el orden de los ids, solo con los criterios trazados por test.
  - Un criterio sin test sigue dejando rojo `spec_trace` en cualquier riesgo.
- **El aviso.** Cuando `citas_por_criterio` del riesgo de la tarjeta es mayor que 0 y un criterio lo pasa, `hoom verify` imprime `! <k> criterios pasan el tope de <t> citas en pruebas (riesgo <r>): <id> <n>, ...`. La nota de `spec_trace` termina en ` · <k> pasan el tope de <t> citas`. El estado del veredicto y el exit code no cambian.

### Los valores por riesgo y los niveles

- **Claves.** `agents.<rol>` acepta las claves `bajo`, `normal` y `alto`. Cada una es un mapa con `model`, `effort`, `budget_usd` y `timeout`, que siguen las mismas reglas que en `agents.<rol>`. Un mapa nulo o vacío no cambia nada.
- **Errores**, con `hoom.yaml invalido: ` delante:
  - `agents.<rol>.<r>: clave desconocida "<k>" (validas: model, effort, budget_usd, timeout)`;
  - `agents.<rol>.<r> debe ser un mapa (ej. alto: { model: fuerte })`;
  - `agents.<rol>: clave desconocida "<k>" (validas: provider, model, effort, budget_usd, timeout, write, bajo, normal, alto)`;
  - `agents: rol "<x>" desconocido (validos: <los roles de agents.Roles, en su orden, separados por ", ">)`;
  - `agents.reviewer` sigue rechazando toda clave de lanzamiento, también `bajo`, `normal` y `alto`.
- **La regla** (CA-491) suma un nivel: el comando, `agents.<rol>.<riesgo>` del `hoom.yaml` del proyecto, `agents.<rol>`, la propuesta del lugar y el provider.
  - El riesgo es el declarado de la tarjeta del lanzamiento. Con la tarjeta ilegible vale `alto`, y sin tarjeta, `normal`.
  - El origen de un valor por riesgo es `hoom.yaml, riesgo <r>`.
  - El tope de lo que le queda a la tarjeta también vale para un `budget_usd` por riesgo.
  - Vale para todo lanzamiento: `hoom agent`, `pedir`, `siguiente`, `reanudar`, el tablero, el Studio y el piloto. `Action` trae los valores con el riesgo de su tarjeta.
- **Los niveles** `fuerte`, `medio` y `barato`, en minúsculas, valen donde vale un modelo: `agents.<rol>.model`, `agents.<rol>.<r>.model`, `review.model` y `--model`. La tabla vive en `internal/modelos`, que es el único lugar donde están:

  | Provider | `fuerte` | `medio` | `barato` |
  | --- | --- | --- | --- |
  | claude | `opus` | `sonnet` | `haiku` |
  | codex | `gpt-6-astra` | `gpt-6.1-sol` | `gpt-6-luna` |

  - Un nivel se traduce con el provider del lanzamiento, después de elegirlo, también el que `hoom agent` toma por estar instalado. Un provider sin modelo (gemini, opencode) no recibe ninguno, como hoy.
  - Ningún nivel llega sin traducir a un provider, al comando largo ni a una pantalla. Un nombre que no es un nivel pasa igual que hoy.
- **Cómo se muestra el modelo.**
  - En la línea `valores:`, un nivel se muestra como `modelo <m> (nivel <n>, <origen>)`. Sin modelo en el provider: `modelo del provider (nivel <n>: <p> no recibe modelo)`.
  - La cabecera de `hoom review` muestra `modelo      <m> (nivel <n>)`.
- **El origen antes de lanzar.**
  - Cada lanzamiento imprime la línea `valores:` (CA-503) una sola vez: la imprime el verbo que lanza o, sin verbo, `hoom agent`. En `hoom agent` va antes de sus pasos, y no es uno de ellos.
  - `hoom piloto` y el piloto del Studio imprimen `piloto automatico: valores: <lo mismo>` justo antes de su línea `piloto automatico: pide al ...`, que no cambia.

### `hoom init`, contratos y README

- **`hoom init`** escribe, después de los gates y con el mismo texto para los cuatro perfiles:

  ```yaml
  # Modelo por rol en niveles (fuerte, medio, barato): hoom los traduce al
  # modelo de cada provider; un nombre de modelo tambien vale. Cada rol puede
  # cambiar sus valores segun el riesgo de la tarjeta (bajo, normal, alto).
  agents:
    arquitecto: { model: fuerte }
    designer: { model: medio }
    scout: { model: barato }
    writer: { model: medio, alto: { model: fuerte } }
    test-writer: { model: medio, alto: { model: fuerte } }
    characterizer: { model: medio, alto: { model: fuerte } }
    analista: { model: medio }
    refutador: { model: medio, alto: { model: fuerte } }
  review:
    model: fuerte
  # Topes del spec segun el riesgo declarado de la tarjeta (0 = sin tope).
  spec:
    topes:
      bajo: { palabras: 400, criterios: 4, citas_por_criterio: 10 }
      normal: { palabras: 800, criterios: 8, citas_por_criterio: 10 }
      alto: { palabras: 2000, criterios: 15, citas_por_criterio: 0 }
  ```

- **Contrato 01.** La regla «se lee en 3 minutos» pasa a ser: el spec cabe en el tope de la nota «Tope del spec», que `spec_lint` mide; si no cabe, reduce el alcance o pide a Hoom que suba el tope de la tarjeta. Además, referencia el intake por ruta y sección, sin copiarlo.
- **Contrato 02.** El designer se invoca solo cuando la tarjeta tiene una decisión visual.
- **Contrato 00.** Al lanzar al test-writer o al arquitecto como subagente de su propia CLI, el orquestador agrega la nota de riesgo que hoom agrega en `hoom agent`.
- Los contratos 00, 01, 02 y 05 cambian en la plantilla y en la copia del repo.
- **El README** documenta `spec.topes`, el tope de la tarjeta, las citas, los niveles y los valores por riesgo.

## Casos límite y errores esperados

- Un spec vacío mide 0 palabras y 0 criterios; `spec_lint` ya falla por sus secciones.
- Un spec en el tope exacto pasa, y uno más falla, en palabras y en criterios.
- Un criterio con marcador `verifica` cuenta como criterio para el tope, pero no tiene citas.
- `tope_palabras: 0` en la tarjeta es sin tope, aunque `hoom.yaml` tenga uno. Un tope de la tarjeta menor que el de `hoom.yaml` también vale: es el de la tarjeta.
- Aprobar un spec que pasa su tope no escribe nada. Subir el tope de la tarjeta y aprobar otra vez sí lo aprueba.
- Un spec aprobado que pasa su tope sigue verde. Si cambia después de aprobarse, se mide otra vez.
- Una línea que cita dos criterios cuenta una cita para cada uno, y un token dentro de otro con más dígitos no cuenta.
- `model: Fuerte` no es un nivel: pasa como nombre, igual que hoy.
- `alto: { provider: codex }` falla con la clave desconocida: el provider no varía por riesgo.
- Un rol mal escrito, como `writr` o `test_writer`, falla al cargar `hoom.yaml`.
- Con la tarjeta ilegible, el modelo y la nota usan `alto`, y la medida del spec da `error`.

## Lo que este spec re-expresa

Cada test de estos CA se re-expresa con la regla nueva; nada más cambia.

- **CA-182** (test-writer-en-arbol-ciego): el SystemPrompt del run ciego es el contrato, el párrafo del árbol ciego y la nota de riesgo. Los pasos siguen siendo seis y cinco.
- **CA-270** y **CA-271** (items-y-columna-derivada): un spec sin aprobación vigente que pasa su tope da `arquitecto`, no `tu-aprobacion`.
- **CA-490** (verbos-de-la-tarjeta): `agents.<rol>` acepta las claves por riesgo, su lista de claves válidas las suma, y un rol desconocido falla.
- **CA-491** y **CA-492** (verbos-de-la-tarjeta): la regla suma el nivel del riesgo, y `Action` trae los valores con el riesgo de su tarjeta y el modelo traducido.
- **CA-647** (autonomia-entre-firmas): sigue habiendo una línea por decisión, más la línea `valores:` antes de cada lanzamiento.

## Criterios de aceptación

- CA-791: las palabras del spec son las de `strings.Fields` sobre el archivo entero, con encabezados, tablas, código y Decisiones incluidos; los criterios son los ítems `- CA-n:` de la sección de criterios, y un id citado o re-expresado en otra sección no cuenta.
- CA-792: `manifest.Load` acepta `spec.topes` con los valores por defecto de la tabla para cada clave que falta, acepta 0 como sin tope y rechaza los cinco errores con su texto exacto; con uno de ellos, `hoom verify` no corre gates ni escribe veredicto.
- CA-793: un spec sin aprobar de una tarjeta `normal` con 801 palabras deja `spec_lint` en `fail` con la línea exacta, el tope, el conteo y el comando para subirlo; uno con 9 criterios también; 800 palabras y 8 criterios pasan; `bajo` y `alto` usan sus topes; un spec sin tarjeta usa `normal` y nombra `spec.topes`; una tarjeta ilegible da `error`.
- CA-794: un spec con aprobación vigente que pasa su tope deja `spec_lint` en `pass` con la nota ` · aprobado: el tope no se mide`; si cambia después de aprobarse, se mide y falla.
- CA-795: `hoom spec approve` y `hoom aprobar` salen con exit 1 sin escribir registro con un spec que pasa su tope, y la aprobación del Studio responde con error y tampoco escribe; después de subir el tope de la tarjeta, aprueban.
- CA-796: `hoom item set --tope-palabras` y `--tope-criterios` con `--motivo` escriben las claves de la tarjeta con `tope_por` y `tope_en`; sin motivo, con motivo en blanco, con un valor negativo o con `HOOM_ROLE`, salen con exit 1 sin tocar el archivo; `item.Parse` acepta las cinco claves; el tope de la tarjeta reemplaza al de `hoom.yaml` solo para ella.
- CA-797: `hoom spec status` imprime la línea de las medidas con sus variantes (de la tarjeta, sin tope, sin tarjeta, aprobado y tarjeta ilegible), y sus exit codes siguen los de CA-12.
- CA-798: el tablero da `arquitecto` con las líneas de `spec_lint` en `missing` para un spec sin aprobación vigente que pasa su tope, y la columna de hoy para uno aprobado. Re-expresa CA-270 y CA-271.
- CA-799: el SystemPrompt del test-writer termina en `Riesgo declarado de la tarjeta: <r>.` después del párrafo ciego, y el del arquitecto en la nota con su tope; la tarjeta ilegible da `alto` y sin tarjeta da `normal`; el writer no recibe nota y el contrato en disco no cambia. Re-expresa CA-182.
- CA-800: los contratos 05, 01, 02 y 00 dicen lo de este spec, en la plantilla y en la copia del repo. [verifica: grep -q "Solo con riesgo alto" internal/agents/assets/agents/05-test-writer.md && grep -q "Solo con riesgo alto" .hoom/agents/05-test-writer.md && grep -q "Tope del spec" internal/agents/assets/agents/01-arquitecto.md && grep -q "Tope del spec" .hoom/agents/01-arquitecto.md && grep -q "decision visual" internal/agents/assets/agents/02-designer.md && grep -q "decision visual" .hoom/agents/02-designer.md && grep -q "Riesgo declarado de la tarjeta" internal/agents/assets/agents/00-orquestador.md && grep -q "Riesgo declarado de la tarjeta" .hoom/agents/00-orquestador.md]
- CA-801: `spec_trace` cuenta las citas de cada criterio trazado por test, una por línea, y su nota en pass trae ` · citas por criterio: ` en el orden de los ids; un criterio sin test sigue dejando rojo `spec_trace` con riesgo `bajo`, `normal` y `alto`.
- CA-802: con una tarjeta `normal` y un criterio con 11 citas, `hoom verify --spec` imprime el aviso exacto y la nota termina con el tope pasado, con el mismo estado y exit code que sin el aviso; con `alto` y los valores por defecto no hay aviso.
- CA-803: `manifest.Load` acepta `agents.<rol>.bajo|normal|alto` con `model`, `effort`, `budget_usd` y `timeout`, y rechaza con su texto exacto una clave desconocida en esos mapas, uno que no es un mapa y un rol desconocido; un `hoom.yaml` sin estas claves carga igual que hoy. Re-expresa CA-490.
- CA-804: un writer en una tarjeta `alto`, con `agents.writer.alto.model`, lanza con ese modelo y origen `hoom.yaml, riesgo alto`, y en una tarjeta `normal` lanza con `agents.writer.model`; el comando gana a los dos; una tarjeta ilegible usa `alto`; el presupuesto por riesgo no pasa lo que le queda a la tarjeta. Re-expresa CA-491 y CA-492.
- CA-805: `medio` llega a claude como `sonnet` y a codex como `gpt-6.1-sol`, con la tabla completa de niveles, también desde `--model` y `review.model`; a gemini no le llega ningún modelo; `Action.model`, el comando largo y las pantallas traen el modelo traducido; un nombre que no es nivel pasa sin cambios.
- CA-806: `hoom agent` sin verbo imprime la línea `valores:` una vez, antes de sus pasos y con `nivel <n>` cuando el modelo vino de un nivel; `hoom pedir` la imprime una sola vez; `hoom piloto` imprime `piloto automatico: valores: ` justo antes de su línea `pide al`, que no cambia. Re-expresa CA-647.
- CA-807: `hoom init` escribe las secciones `agents`, `review` y `spec` de este spec, iguales para los cuatro perfiles, y el `hoom.yaml` que escribe carga con `manifest.Load`.
- CA-808: el README documenta los topes del spec, el tope de la tarjeta, las citas, los niveles y los valores por riesgo. [verifica: grep -q "spec.topes" README.md && grep -q "tope-palabras" README.md && grep -q "citas_por_criterio" README.md && grep -q "fuerte, medio" README.md && grep -q "riesgo alto" README.md]

## Decisiones

- **El riesgo es el declarado** (10.2). CAL-6.1 dice «efectivo», pero el test-writer corre antes del diff. Si el efectivo sube, hoom ya avisa en verify y una persona sube el declarado; hoom no relanza a nadie.
- **P7: el tope solo mide un spec sin aprobación vigente, y la aprobación se niega si lo pasa** (pendiente de la firma). Así, ningún spec aprobado se vuelve rojo, y los 49 de hoy siguen como están. Una enmienda se mide otra vez. Descartado: medir todo spec de una tarjeta abierta, porque obliga a subir el tope de cada tarjeta en curso sin que eso ahorre nada.
- **P4: los topes del intake** (pendiente de la firma): `normal` 800 palabras y 8 criterios, y `alto` 2,000 y 15. Para `bajo`, que el intake no fija, la mitad de `normal`. 800 palabras es lo que se lee en 3 minutos, que ya era la regla del contrato 01.
- **P1: en `bajo`, las pruebas siguen siendo del test-writer, como en `normal`** (pendiente de la firma). Así se sostiene I3 sin un camino nuevo. Descartado: aceptar solo criterios con marcador `verifica`, porque deja sin prueba lo que un comando no cubre.
- **Una palabra es todo el archivo.** La regla es simple y se reproduce con `wc -w`. Además, nadie esquiva el tope pasando texto a un bloque de código. Las re-expresiones también cuentan, porque son parte de lo que la persona firma.
- **Un criterio es un ítem de la sección de criterios.** Un id citado no es una conducta nueva, y contarlo castigaría justo a los specs que re-expresan.
- **El tope de la tarjeta vive en el item**, con quién, cuándo y por qué, como `riesgo_por` y `riesgo_en`. El historial es el de git. Descartado: un registro append-only aparte, porque es más mecanismo para lo mismo.
- **hoom cuenta citas, no pruebas.** Contar funciones de prueba exige un parser por lenguaje (Go, PHP y Kotlin), y las citas son iguales en todos. Las citas son aproximadas, y por eso solo avisan.
- **El aviso no es un gate.** El veredicto no tiene un estado amarillo, y un gate no requerido en `fail` se lee como una falla.
- **La nota de riesgo se agrega al lanzar, no con un contrato por riesgo.** Cada rol sigue teniendo un solo contrato, y las copias de los proyectos siguen valiendo. Sin la nota, el test-writer trabaja como hoy (alto): los subagentes nativos y los flujos viejos no bajan sus pruebas sin que nadie lo decida.
- **`fuerte` es `opus`, no `fable`.** `opus` es lo que corre hoy; quien quiera `fable` escribe el nombre. Los niveles de claude son alias de su CLI, que no envejecen. Los de codex sí, y viven junto al catálogo de `Verificado`.
- **`hoom init` pone al writer y al test-writer en `medio`, y en `fuerte` con `alto`**, como sugiere el intake. Un writer más barato no baja la garantía: los gates deciden el verde (I1), y `hoom costo` mide si suben las rondas.
- **Un rol desconocido falla.** `hoom init` ahora escribe nombres de rol, y un `test_writer` mal escrito dejaría de aplicar su modelo sin que nadie lo note.
- **El provider no varía por riesgo.** Así la review sigue eligiendo un provider distinto del writer con la misma regla de hoy (I4).

## Riesgos y deuda aceptada

- **Los specs de hoomAI no caben en 800 palabras.** Con estos topes, casi toda tarjeta de hoomAI va a pedir subir su tope o partirse. Es lo que busca CAL-7, pero cuesta una decisión por tarjeta.
- **Las citas cuentan de más.** Un test que cita su criterio en el nombre y en un comentario da 2, y `isTestFile` toma cualquier ruta con `test` o `spec`, también archivos que no son pruebas.
- **Que el test-writer escriba menos pruebas en `normal` es una regla de prompt.** hoom solo la mide con el aviso. El riesgo sí lo decide una persona (I9).
- **Un writer en `medio` puede pedir más rondas.** Se mide con `hoom costo`, y la persona lo sube en `hoom.yaml`.
- **Los nombres de codex envejecen.** La tabla se actualiza junto con el catálogo.
- **El UI-spec no tiene tope**, porque ningún gate lo mide.

## Preguntas para la firma

1. **P7.** ¿El tope mide también los specs ya aprobados? La recomendación es que no: solo mide un spec sin aprobación vigente, y la aprobación se niega si lo pasa. La alternativa es medir todo spec de una tarjeta abierta.
2. **P4.** ¿Qué topes por defecto? La recomendación son los del intake: `bajo` 400 palabras y 4 criterios, `normal` 800 y 8, y `alto` 2,000 y 15. La alternativa más holgada es `bajo` 800 y 6, `normal` 1,500 y 12, y `alto` 3,000 y 20.
3. **P1.** En `bajo`, ¿quién escribe las pruebas? La recomendación es el test-writer, igual que en `normal`. La alternativa es aceptar solo criterios con marcador `verifica`.
