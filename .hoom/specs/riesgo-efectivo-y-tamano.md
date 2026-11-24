# Spec: el riesgo efectivo y el tamaño de producto deciden la review

Estado: BORRADOR — pendiente de aprobación humana.
Tarjeta: `riesgo-efectivo-y-tamano`. Intake: `.hoom/intake/rfc-calibracion-por-riesgo.md`, CAL-3, CAL-1 y CAL-2 con los ajustes de 10.2, y las preguntas P3, P5 y P7 (10.4 las deja a este spec). Criterios CA-774 a CA-790.

## Objetivo

Que la review de una tarjeta se mida por lo que arriesga su código, no por cuántas pruebas trae.

Hoy el tamaño es un solo número, `insertions + deletions`, que suma todo: pruebas, specs, veredictos, `go.sum` y `vendor/`, y además lo que trajo la base si avanzó. Con más de 400 líneas, una tarjeta `normal` recibe las 4 lentes. Por eso 155 de las 156 reviews de hoomAI y las 34 del proyecto medido usaron 3 o 4 lentes. En el proyecto medido, el veredicto de `nuc-auth` contó 5,581 inserciones en 37 archivos, 15 de pruebas y 4 de `.hoom/`. El último veredicto de hoomAI marca +9300/-81.

El riesgo tampoco hace lo que promete: una ruta de riesgo solo pide las 4 lentes, y un cambio de 50 líneas en `auth/` de una tarjeta `normal` ni siquiera exige review en el tablero. La lista de rutas está fija en el código, atrapa `design` con la marca `sign`, y marca como riesgosa a una tarjeta por el nombre de su propio spec.

Este spec hace tres cosas:

1. El tamaño se reporta en tres números: producto, pruebas y estado de hoom. Los lockfiles y los generados no suman. Todo umbral usa solo el de producto.
2. El riesgo efectivo sale de reglas fijas: lo declarado en la tarjeta, su tipo y las rutas que toca el cambio. Las rutas viven en `hoom.yaml`, con valores por defecto en cada perfil. hoom solo lo sube; bajarlo lo hace una persona (I9). El veredicto y el tablero lo muestran con su motivo.
3. Las lentes y la review requerida salen del riesgo efectivo y del tamaño de producto.

Protege I9 (ninguna decisión de proporcionalidad la toma un modelo), I4 (la review corre igual, sobre la misma evidencia) e I2 (la huella no cambia).

## No-goals

- Usar el riesgo para el spec, las pruebas o el modelo (CAL-7, CAL-6 y CAL-11): es la tarjeta `proporcion-por-riesgo`, y usa el riesgo **declarado** (10.2).
- Relanzar roles cuando el efectivo sube: hoom lo señala y una persona sube el declarado (10.2).
- Un mapa de rutas a lentes para elegir la lente de `normal` (CAL-2): sigue siendo `reliability`. Se revisa con `hoom costo` después de esta tarjeta.
- Bajar las lentes de una delta a las de sus bloqueantes (CAL-2, último punto): ver Decisiones.
- Resumir las pruebas en la evidencia de la review (P3): ver Decisiones.
- Cambiar `git.insertions`, `git.deletions` ni `git.changed_files` del veredicto: siguen como hoy, para no mover la huella ni la evidencia.
- Rutas por gate y gates que no aplican (CAL-9): es la tarjeta `gates-por-ruta`.
- Que `hoom init` escriba la sección `rutas`: sin ella valen los valores por defecto.
- El chip de riesgo del Studio: ya muestra el efectivo.
- Registrar como hallazgo un `hoom item set --riesgo` que un rol intenta y hoom niega: no cambia nada, y el sobre ya marca cualquier cambio real (ver Contratos).

## Contratos

### Los archivos del cambio y su clase

- **Los archivos del cambio** son los que da `git diff --numstat --no-renames` desde el merge-base de la base y `HEAD` contra el árbol de trabajo, más los archivos sin seguimiento que git no ignora. Entran los borrados. Commits nuevos de la base no cuentan.
- **Las líneas de un archivo** son sus inserciones más sus borrados en ese diff. Un binario da 0; un archivo sin seguimiento da sus líneas.
- **La clase de un archivo** es la primera que le toca, en este orden:
  1. `hoom`: todo lo que está bajo `.hoom/`.
  2. `generado`: coincide con `rutas.generadas`.
  3. `prueba`: está en el territorio del test-writer, el de `agents.test-writer.write.allow` o, sin esa clave, su valor por defecto (sin `.hoom/**`, que ya es `hoom`).
  4. `producto`: todo lo demás, `hoom.yaml` incluido.
- **El tamaño** son tres números: `producto`, `pruebas` y `hoom`, la suma de las líneas de cada clase. Un `generado` no suma a ninguno.

### `rutas` en `hoom.yaml`

```yaml
rutas:
  de_riesgo: ["app/Domain/Costeo", "**/*tenant*"]
  de_bajo_riesgo: ["**/*.md", "lang"]
  generadas: ["composer.lock", "vendor"]
```

- Las tres claves son listas de patrones. Una clave ausente o nula toma el valor por defecto del perfil. Una lista, vacía incluida, lo reemplaza entero.
- **Un patrón** usa la sintaxis de `agents.<rol>.write.allow`: `*`, `?` y `[...]` dentro de un segmento, y `**` para cero o más segmentos. Coincide con una ruta si coincide con la ruta o con uno de sus directorios padre, sin distinguir mayúsculas. Así, `**/*auth*` cubre `internal/oauth.go` y todo lo que está bajo `app/Http/Auth/`.
- **Errores**, con este texto exacto. Con cualquiera, el comando que lee `hoom.yaml` falla como con cualquier otro error del manifiesto:
  - `rutas: clave desconocida "<k>" (validas: de_riesgo, de_bajo_riesgo, generadas)`;
  - `rutas.<clave>: tiene que ser una lista de patrones`;
  - `rutas.<clave>: patron invalido "<p>"`.
- **Quién lee qué `hoom.yaml`.** `hoom review` lee `rutas` y el territorio del test-writer del merge-base, como el resto de su política (CA-417). Los demás leen el `hoom.yaml` del proyecto, como los gates.

**Valores por defecto.** Sin perfil valen los de la fila «base»; un perfil suma su fila a la de base.

| Perfil | `de_riesgo` | `de_bajo_riesgo` | `generadas` |
|---|---|---|---|
| base | `**/*<m>*` por cada marca: auth, login, password, passwd, secret, token, cred, crypt, signin, signature, pago, payment, billing, invoice, dte, permission, permiso, sandbox, sudo | `**/*.md`, `**/*.txt`, `**/*.rst`, `**/*.adoc`, `**/docs`, `**/LICENSE`, `**/NOTICE` | ninguna |
| go | — | — | `go.sum`, `vendor` |
| laravel | — | `lang`, `resources/lang` | `composer.lock`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`, `vendor`, `node_modules`, `public/build` |
| kmp, kmp-compose | — | — | `kotlin-js-store/yarn.lock`, `**/gradle.lockfile` |

Las marcas son las de `riesgo.Marcas` de hoy, salvo `sign`, que pasa a `signin` y `signature`: `sign` atrapaba `design`, `assign` y `signal`.

### El riesgo efectivo

hoom lo calcula con estas reglas, en orden; gana la primera que se cumple:

| # | Condición | Efectivo | Motivo |
|---|---|---|---|
| 1 | Hay tarea y su tarjeta no se puede leer | `alto` | `no se pudo leer la tarjeta: riesgo alto` |
| 2 | La tarjeta es de tipo `seguridad` | `alto` | `tarjeta de tipo seguridad: riesgo alto` |
| 3 | Declara `alto` | `alto` | `declarado alto` |
| 4 | Un archivo `producto` o `prueba` coincide con `de_riesgo` | `alto` | `el cambio toca <ruta>: riesgo alto` |
| 5 | Declara `bajo` y un archivo `producto` no coincide con `de_bajo_riesgo` | `normal` | `el cambio toca <ruta>, fuera de las rutas de bajo riesgo: riesgo normal` |
| 6 | Declara `bajo` | `bajo` | `declarado bajo` |
| 7 | Sin tarea | `normal` | `sin tarjeta: riesgo normal` |
| 8 | Lo demás | `normal` | `declarado normal` |

- `<ruta>` es el primer archivo que cumple la condición, en orden de bytes.
- Un `hoom` o un `generado` nunca sube el riesgo.
- El efectivo nunca es menor que el declarado.

### El veredicto

- **La tarea** del veredicto es la del sobre de `hoom agent`, o la del worktree `.hoom/worktrees/<slug>` donde corre `hoom verify`. En cualquier otro árbol no hay tarea.
- **La tarjeta** se lee de `.hoom/items/<slug>.yaml` del checkout principal, como en el tablero.
- **El JSON** suma dos claves arriba:
  - `riesgo`, con `declarado` (`normal` si la tarjeta no lo dice o no hay tarea), `efectivo` y `motivo`;
  - `tamano`, con `producto`, `pruebas` y `hoom`.
  Fuera de un repo git no va `tamano`, y el riesgo se calcula sin archivos. Ninguna de las dos entra en la huella (I2).
- **El texto** de `hoom verify` imprime, en la cabecera:
  - `riesgo: <efectivo> (<motivo>)`;
  - `tamano: producto <p> · pruebas <t> · hoom <h>`;
  - si el efectivo es mayor que el declarado: `! el riesgo efectivo (<efectivo>) es mayor que el declarado (<declarado>): hoom item set <slug> --riesgo <efectivo> lo sube`;
  - si el efectivo es `normal` y `producto` pasa de 400: `! mas de 400 lineas de producto: la review exige las 4 lentes`.
- El aviso fijo de hoy, `! cambio grande (>400 lineas): la review exige las 4 lentes`, desaparece.

### Las lentes de `hoom review`

| Caso, en orden | Lentes | Motivo de las lentes |
|---|---|---|
| `--lens` | las pedidas | `lente pedida a mano` (como hoy) |
| Sin cambios, o solo documentación (`SoloDocs`) | 0 | como hoy |
| Efectivo `alto` | las 4 | `riesgo alto: las 4 lentes` |
| Efectivo `bajo` | `reliability` | `riesgo bajo: la lente dominante` |
| Efectivo `normal` y `producto` > 400 | las 4 | `<p> lineas de producto (>400): las 4 lentes` |
| Efectivo `normal` | `reliability` | `cambio estandar: la lente dominante` |

- Con un rango (`--desde` o `--delta`) gana, como hoy, la regla más estricta entre el rango y el cambio entero (CA-424). El tamaño del rango es su `producto`, con las mismas clases.
- Una delta con las 4 sigue sin `readability`. Si su rango toca una ruta de riesgo, el cambio entero es `alto`, así que la delta corre `risk`.
- **El registro** de `.hoom/reviews/`, el `Result` y `--json` suman `riesgo_motivo` y `lentes_motivo` a `riesgo` y `riesgo_declarado`. La línea `riesgo:` de la salida es `riesgo: <efectivo> (<motivo>)`.

### El tablero, `hoom status` y los hallazgos

- **El riesgo de una tarjeta** es el más alto entre el `riesgo.efectivo` de su último veredicto y su declarado de ahora. El motivo es el del veredicto, o `declarado <d>` si gana el declarado. Sin veredicto, es el declarado.
- **La review requerida:**
  - con efectivo `alto`, siempre, salvo un cambio vacío o de solo documentación;
  - con `bajo` y `normal`, cuando `producto` pasa de 400 y el cambio no es solo documentación.
  - El segmento `no-aplica` dice `el cambio es chico (<p> lineas de producto): no exige la revision de <lentes>`.
- **Dónde se ve:**
  - `hoom board`, `hoom item show` y el detalle de `hoom tablero` muestran ` · riesgo <efectivo>` cuando no es `normal`. Hoy el tablero muestra el declarado.
  - El JSON de `hoom board`, `hoom item show` y `/api/board` trae `riesgo` (el efectivo) y `riesgo_motivo`.
  - El detalle de `hoom tablero` y `hoom status` muestran la línea `tamano:` del último veredicto de la tarjeta, y `hoom status` también su línea `riesgo:`.
- **`finding.Bloquea`:**
  - el medium «en ruta de riesgo» se mide contra `rutas.de_riesgo`;
  - el medium de la review completa bloquea con el riesgo **efectivo** `alto`. En `hoom verify`, el de la tarea es el que calcula ese verify; en los demás comandos y para las demás tareas, el de la tarjeta.
  - El texto de cada motivo no cambia.

### La tarjeta

- `hoom item set <slug> --riesgo r` escribe además `riesgo_por` (el `user.name <user.email>` de git, como `creado_por`) y `riesgo_en` (UTC, RFC 3339). Las escribe también con `--riesgo normal`, que deja el archivo sin `riesgo`.
- `item.Parse` acepta las dos claves.
- Un rol no cambia el riesgo, como hoy:
  - `hoom item set` con `HOOM_ROLE` sale con exit 1 (CA-617);
  - el sobre marca `manipulacion` si cambia `.hoom/items/` o `hoom.yaml`, también la sección `rutas`.

### Contratos de los roles y README

- **00:** el punto 4b dice que el veredicto trae el riesgo efectivo y las líneas de producto, y que más de 400 líneas de producto son las 4 lentes, salvo riesgo bajo; con riesgo alto, las 4 siempre. El presupuesto de corrección es de ~120 líneas de producto.
- **06:** las lentes salen del riesgo efectivo y de las líneas de producto, no se estiman a ojo, y una ruta de riesgo sube la tarjeta a alto.
- Los dos contratos cambian en la plantilla y en la copia del repo, igual que la descripción del reviewer en `internal/agents/targets.go`.
- El README documenta `rutas`, el tamaño en tres números y el riesgo efectivo.

## Casos límite y errores esperados

- **Veredicto anterior a este spec** (sin `riesgo` ni `tamano`): el tablero calcula el riesgo con las reglas de arriba sobre `git.changed_files`, y toma `insertions + deletions` como `producto`. Esa base es más dura que la de ahora, nunca más blanda.
- **Lo que no baja solo:** una persona que baja el declarado ve el efectivo viejo hasta el siguiente verify, porque el del veredicto todavía es más alto. Subir el declarado se ve enseguida.
- **Tarjetas ya abiertas (P7):** las reglas valen desde el siguiente verify o review. Los registros viejos conservan sus lentes. Una cadena que la regla nueva cubre sigue valiendo. Una tarjeta que ahora sale `alto` vuelve a Review si su cadena no trae las 4.
- **Ruta de riesgo solo en una prueba:** sube a `alto`. Por ejemplo, `internal/spec/tokens_test.go` de hoomAI, por `token`.
- **`bajo` que solo toca pruebas o `.hoom/`:** se sostiene, porque la regla 5 mira solo `producto`.
- **Un `generado` editado a mano** (por ejemplo, un archivo dentro de `vendor`): no suma al tamaño ni sube el riesgo. Lo decide la lista del proyecto.
- **`rutas.de_riesgo: []`:** ninguna ruta sube el riesgo. Es explícito, y el veredicto sigue diciendo el motivo.

## Lo que este spec re-expresa

Cada test de estos CA se re-expresa con la regla nueva; nada más cambia.

- **CA-160** (codex-v2) y **CA-619** (ciclo-de-review-acotado): las 4 lentes por tamaño se miden en líneas de producto y solo en `normal`. Una ruta de riesgo da `alto`; en `bajo` ya no da las de `normal`.
- **CA-424** (review-por-diferencia): el rango y el cambio entero se miden en líneas de producto.
- **CA-274** (items-y-columna-derivada), **CA-305** (tablero-de-solo-lectura) y **CA-622** (ciclo-de-review-acotado): los 400 de la review requerida son líneas de producto, y solo documentación nunca la exige.
- **CA-617** (ciclo-de-review-acotado): `hoom item set --riesgo` también escribe `riesgo_por` y `riesgo_en`.
- **CA-618** (ciclo-de-review-acotado): `bajo` con ruta de riesgo da `alto`, no `normal`, y el motivo nunca queda vacío. Las marcas cambian `sign` por `signin` y `signature`, y salen de `rutas.de_riesgo`.
- **CA-621** (ciclo-de-review-acotado): el registro suma `riesgo_motivo` y `lentes_motivo`, y la línea `riesgo:` lleva el motivo siempre.
- **CA-623** y **CA-624** (ciclo-de-review-acotado): la ruta de riesgo sale de `rutas.de_riesgo`, y el riesgo de la regla del medium de la review completa es el efectivo.
- **CA-708** (tablero-tu-siguiente-paso): el borde ` · riesgo <r>` del detalle usa el efectivo.

## Criterios de aceptación

- CA-774: un cambio con 90 líneas de producto y 1,500 de pruebas tiene `tamano.producto` 90 y `tamano.pruebas` 1500; `composer.lock` y `package-lock.json` con el perfil `laravel`, y `go.sum` y `vendor/` con el perfil `go`, no suman a ningún número; todo lo de `.hoom/` suma a `hoom`, también un `_test.go` bajo `.hoom/`; con `agents.test-writer.write.allow`, `prueba` es ese territorio.
- CA-775: el tamaño se mide desde el merge-base: un commit nuevo de la base no suma; un archivo borrado suma sus líneas y cuenta para el riesgo; uno sin seguimiento suma sus líneas; un binario suma 0.
- CA-776: una lista en `rutas.<clave>` reemplaza el valor por defecto, `[]` lo deja vacío, y la clave nula o ausente usa el del perfil; los tres errores de `rutas` salen con su texto exacto, y `hoom verify` no corre gates ni escribe veredicto.
- CA-777: un patrón coincide con la ruta o con un directorio padre, sin distinguir mayúsculas: `**/*auth*` cubre `internal/oauth.go` y `app/Http/Auth/Login.php`, y `vendor` cubre `vendor/a/b.go`.
- CA-778: sin la sección `rutas`, `design.md`, `assign.go` y `signal.go` no son rutas de riesgo, y `signature.go`, `SignIn.php` y `internal/auth/x.go` sí; `de_bajo_riesgo` y `generadas` son las de la tabla para cada perfil; un archivo `hoom` o `generado` con una marca no sube el riesgo.
- CA-779: el riesgo efectivo sigue la tabla de 8 reglas con su motivo exacto: tarjeta ilegible y tipo `seguridad` dan `alto`; una ruta de riesgo da `alto` con declarado `bajo` o `normal`; `bajo` con un `producto` fuera de `de_bajo_riesgo` da `normal`; `bajo` con solo documentación, pruebas o `.hoom/` sigue `bajo`; sin tarea da `normal`; nunca sale menor que el declarado.
- CA-780: el veredicto trae `riesgo{declarado, efectivo, motivo}` y `tamano{producto, pruebas, hoom}`; la tarea sale del sobre o del worktree `.hoom/worktrees/<slug>`, y en el checkout principal no hay tarea; la huella del mismo árbol es la misma que sin estas claves.
- CA-781: `hoom verify` imprime las líneas `riesgo:` y `tamano:`, el aviso con `hoom item set <slug> --riesgo <efectivo>` cuando el efectivo supera al declarado, y el de 400 líneas de producto solo con efectivo `normal`; el texto `cambio grande (>400 lineas)` ya no sale.
- CA-782: `hoom review` corre `reliability` en una tarjeta `normal` con 90 líneas de producto y 3,000 de pruebas, las 4 con 401 de producto, las 4 con efectivo `alto` (declarado, tipo `seguridad` o ruta de riesgo), `reliability` con `bajo`, ninguna con solo documentación, y las de `--lens` cuando se pasa, cada una con el motivo de la tabla. Re-expresa CA-160 y CA-619.
- CA-783: con un rango, gana la más estricta entre la regla del rango y la del cambio entero, las dos en líneas de producto; una delta cuyo rango toca una ruta de riesgo corre `risk`, `reliability` y `resilience`. Re-expresa CA-424.
- CA-784: `hoom review` lee `rutas` y el territorio del test-writer del `hoom.yaml` del merge-base, y no de la rama; el registro, el `Result` y `--json` traen `riesgo_motivo` y `lentes_motivo`, y la línea `riesgo:` lleva el motivo. Re-expresa CA-621.
- CA-785: el tablero exige review con efectivo `alto` salvo un cambio vacío o de solo documentación, y con `bajo` o `normal` solo con más de 400 líneas de producto y un cambio que no es solo documentación; `no-aplica` trae el texto con las líneas de producto; un veredicto sin `tamano` usa `insertions + deletions`. Re-expresa CA-274, CA-305 y CA-622.
- CA-786: el riesgo de la tarjeta es el más alto entre el del último veredicto y el declarado; `hoom board`, `hoom item show` y el detalle de `hoom tablero` muestran ` · riesgo <efectivo>` cuando no es `normal`; el JSON trae `riesgo` y `riesgo_motivo`; el detalle de `hoom tablero` y `hoom status` muestran la línea `tamano:`, y `hoom status` también `riesgo:`. Re-expresa CA-708.
- CA-787: `finding.Bloquea` mide el medium en ruta de riesgo contra `rutas.de_riesgo`, y bloquea el medium de una review completa cuando el riesgo efectivo de su tarea es `alto`, también con el declarado `normal`. Re-expresa CA-623 y CA-624.
- CA-788: `hoom item set <slug> --riesgo r` escribe `riesgo_por` y `riesgo_en`, también con `normal`; `item.Parse` las acepta; con `HOOM_ROLE` sigue saliendo con exit 1 sin tocar el archivo. Re-expresa CA-617.
- CA-789: los contratos 00 y 06 hablan de líneas de producto y del riesgo efectivo, en la plantilla y en la copia del repo, y 00 sigue diciendo «salvo riesgo bajo». [verifica: grep -q "lineas de producto" internal/agents/assets/agents/00-orquestador.md && grep -q "lineas de producto" .hoom/agents/00-orquestador.md && grep -q "lineas de producto" internal/agents/assets/agents/06-reviewer.md && grep -q "lineas de producto" .hoom/agents/06-reviewer.md && grep -q "riesgo efectivo" internal/agents/assets/agents/06-reviewer.md && grep -q "riesgo efectivo" .hoom/agents/06-reviewer.md && grep -q "salvo riesgo bajo" .hoom/agents/00-orquestador.md]
- CA-790: el README documenta `rutas`, el tamaño y el riesgo efectivo. [verifica: grep -q "de_bajo_riesgo" README.md && grep -q "riesgo efectivo" README.md && grep -q "lineas de producto" README.md]

## Decisiones

- **El riesgo sube por regla y baja por una persona** (I9 y CAL-1.4). Ninguna regla da un efectivo menor que el declarado. La única forma de bajarlo es `hoom item set`, que un rol no puede correr, y ahora queda registrado quién y cuándo.
- **Una ruta de riesgo da `alto`** (CAL-1.2), con todo lo que trae `alto`: review siempre requerida, las 4 lentes y los medium de la review completa bloqueando. Hoy solo pide las 4 lentes, y un cambio chico en `auth/` no exige review. Descartado: que la ruta solo pida las 4 lentes y el efectivo quede en `normal`. Hoom lo eligió el 2026-11-23.
- **La delta sigue con `risk`, `reliability` y `resilience`**, como hoy. En `verify-en-el-entorno`, la segunda delta corregía dos high de `risk` y encontró dos high nuevos de `resilience` (814254 y 7e38c4). Descartado: que corra solo las lentes de sus bloqueantes, más `risk` si toca una ruta de riesgo, porque habría corrido solo `risk`. Hoom lo eligió el 2026-11-23.
- **`bajo` es una lista permitida que falla cerrado** (10.2): basta un archivo de producto fuera de la lista para que suba. Las pruebas y `.hoom/` no lo suben: una tarjeta `bajo` que agrega una prueba no arriesga producción.
- **Las pruebas cuentan para la ruta de riesgo.** Debilitar una prueba de `auth` es tan riesgoso como tocar el código. Fallar hacia más control es lo que pide I9.
- **`.hoom/` nunca es ruta de riesgo.** Así, una tarjeta como `design-system` deja de ser riesgosa por el nombre de su spec.
- **«Prueba» es el territorio del test-writer.** Una sola definición: lo que el test-writer puede escribir es lo que cuenta como prueba. Hoy hay tres (`spec.isTestFile`, `spec.IsTestPath` y ese territorio), y las otras dos atrapan `latest.go` por `test`.
- **El tamaño sigue contando, pero solo el de producto** (10.2, sobre CAL-2.1): 400 líneas de producto en `normal` siguen pidiendo las 4 lentes.
- **P5: las rutas de bajo riesgo por defecto son la documentación**, la misma que `SoloDocs`, más los textos de traducción en `laravel`. Los valores de configuración no entran: un número en un YAML puede tumbar producción, y la persona los agrega si quiere.
- **P3: las pruebas entran completas a la evidencia**, como hoy. El reviewer también revisa las pruebas, y un resumen le esconde justo lo que mira. Si la evidencia pasa el tope, la tarjeta se parte por comportamiento o la persona sube `review.max_evidence_kib`.
- **P7: vale para todas las tarjetas desde su siguiente verify o review.** Los veredictos y los registros viejos no se reescriben.
- **El efectivo del tablero es el más alto entre el del veredicto y el declarado de ahora.** Así una subida del declarado se ve sin esperar un verify, y una bajada espera al siguiente: nunca hay un efectivo menor que el que el último verify midió.
- **La review lee las rutas del merge-base** (CA-417): una rama no afloja su propia review. Una persona que cambia `rutas` en su rama lo ve en la review después de fusionar.

## Riesgos y deuda aceptada

- **Falsos positivos de las marcas.** `auth` atrapa `author` y `AUTHORS`, `token` atrapa un tokenizador, y `cred` atrapa `credito`. Con `alto`, cada uno cuesta 4 lentes y medium que bloquean. La salida es `rutas.de_riesgo` en el `hoom.yaml` del proyecto. Una tarjeta `alto` de más es más barata que una ruta de riesgo sin revisar.
- **`**/*.txt` es de bajo riesgo**, como en `SoloDocs`, aunque un `requirements.txt` o una lista de mutación críticas también lo son. La persona lo saca de `de_bajo_riesgo` si le importa.
- **Una lista `generadas` mal puesta esconde código:** lo que coincide no suma ni sube el riesgo. La lista es de la persona, y un rol que la cambia comete manipulación.
- **Dos fuentes de `rutas`:** la review usa la del merge-base y el veredicto la del proyecto. En una rama que cambia `rutas`, los dos pueden decir riesgos distintos hasta que se fusiona.

## Preguntas para la firma

Ninguna. Las dos que había (la ruta de riesgo y las lentes de la delta) están contestadas en Decisiones. Contestarlas no es la firma.
