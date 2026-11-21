# hoomAI · calibración por riesgo: 12 cambios

- **Fecha:** 2026-11-20
- **Origen:** análisis del proyecto `oceanbuild` trabajado con hoom 0.14.0 (perfil `laravel`, Linux con DDEV), del 10 al 20 de noviembre de 2026.
- **Para:** el analista y los agentes de desarrollo de hoomAI. Pensado para entrar a `.hoom/intake/` de hoomAI.
- **Qué pide:** que hoom aplique sus controles en proporción al riesgo de cada tarjeta, sin quitar ninguno.

Este documento describe el comportamiento de hoom **visto desde un proyecto que lo usa**: sus contratos en `.hoom/agents/`, su `hoom.yaml`, su ayuda (`hoom help`) y los artefactos que deja. No se revisó el código de hoomAI. Donde dice «dónde tocar», son los puntos visibles desde fuera; el arquitecto de hoomAI confirma en el código dónde vive cada regla. Los nombres de campos nuevos son sugerencias.

## 1. Resumen

En `oceanbuild`, hoom entregó código de buena calidad en las partes críticas (tenancy, autenticación, segundo factor), pero cada tarjeta paga el mismo proceso completo sin importar su tamaño ni su riesgo. Un checkbox de 91 líneas y un cambio de un número en un YAML recorren el mismo camino que la autenticación.

El resultado medido:

- 18 runs de agente por tarjeta en promedio.
- 47 % de los tokens se gastan después de la primera review, sobre todo corrigiendo hallazgos que el gate no exigía corregir.
- Las 34 reviews usaron 3 o 4 lentes; ninguna usó 0 o 1.
- En Linux con DDEV, los 137 veredictos automáticos salieron rojos por falta de PHP en el host.

Los 12 cambios se agrupan así:

| Grupo | Cambios | Qué ataca |
| --- | --- | --- |
| Menos trabajo por tarjeta | CAL-1 a CAL-7 | Costo fijo por tarjeta, rondas de corrección, particiones |
| Verify más rápido y confiable | CAL-8 y CAL-9 | Rojos de entorno, mutación en cada corrida |
| Menos gasto por run | CAL-10 a CAL-12 | Runs repetidos, modelo único, trabajo en serie |

## 2. Evidencia

Todo sale de `.hoom/runs`, `.hoom/envelopes`, `.hoom/verdicts`, `.hoom/findings`, `.hoom/reviews`, `.hoom/items` y `git` del proyecto `oceanbuild`.

### 2.1 Volumen

| Dato | Valor |
| --- | --- |
| Tarjetas con runs | 16 (15 cerradas, 1 abierta) |
| Runs de agente | 294 (18.4 por tarjeta) |
| Runs por rol | reviewer 128, test-writer 53, arquitecto 49, writer 39, refutador 16, designer 9 |
| Runs por proveedor | Claude 166, Codex 128 |
| Tokens | 310 millones (Claude 253.6, Codex 56.6) |
| Costo equivalente API | $444 en Claude; Codex sin dato de costo |
| Horas de agente | 13.9 |
| Horas de gates | 7.3, de las cuales 5.4 son mutación (74 %) |
| Reloj total de las tarjetas | unas 42 horas, incluida una pausa de unas 7 |
| Agentes simultáneos | 2 como máximo; casi siempre 1 |

La sesión del orquestador no queda en `.hoom/runs`, así que su costo no está en estos números.

### 2.2 Tokens por fase

| Fase | Runs | Tiempo | Tokens | Costo |
| --- | --- | --- | --- | --- |
| Spec antes del código (arquitecto y designer) | 42 | 21 % | 17 % | $114 |
| Pruebas, primera pasada (test-writer) | 31 | 18 % | 15 % | $72 |
| Código, primera pasada (writer) | 19 | 22 % | 21 % | $49 |
| Reviews, todas las rondas | 128 | 9 % | 18 % | sin dato |
| Refutador | 16 | 6 % | 7 % | $24 |
| Correcciones después de la review | 58 | 23 % | 21 % | $185 |

Antes de la primera review: 53 % de los tokens. Después: 47 %.

### 2.3 Hallazgos y reviews

| Dato | Valor |
| --- | --- |
| Hallazgos | 140: 124 del reviewer, 15 del gate de scope, 1 de una persona |
| Hallazgos del reviewer por severidad | 7 high, 64 medium, 53 low |
| Hallazgos del reviewer corregidos | 81: 7 high, 35 medium, 39 low |
| Hallazgos del reviewer refutados | 20: 0 high, 11 medium, 9 low |
| Hallazgos del reviewer abiertos | 23: 18 medium, 5 low |
| Tarjetas con algún high del reviewer | 4 de 16 |
| Reviews | 34: 16 con 4 lentes, 18 con 3 lentes |
| `findings.block_on` del proyecto | `high` |

De los 81 hallazgos del reviewer que se corrigieron, 74 no bloqueaban. El refutador nunca tumbó un high del reviewer: sus 20 refutaciones sobre hallazgos del reviewer fueron de medium y low.

### 2.4 Veredictos

| Dato | Valor |
| --- | --- |
| Veredictos | 278: 77 verdes, 201 rojos |
| Emitidos por el sobre de `hoom agent` | 137, los 137 rojos |
| Motivo de esos 137 | `env: «php»: No existe el fichero o el directorio` (exit 127) |
| Sobres de roles distintos del reviewer | 172; 171 quedaron `no-entregable` |
| Gate `mutation` | 328 s por corrida completa |

### 2.5 Tamaños

| Dato | Valor |
| --- | --- |
| Código del producto | 3,552 líneas |
| Pruebas | 25,014 líneas (7 a 1) |
| `ObButton` | 125 líneas de código, 1,637 de pruebas |
| `ObInput` | 77 líneas de código, 2,260 de pruebas |
| `ObCheckbox` | 91 líneas de código, unas 970 de pruebas |
| Specs | 5,020 líneas en 20 archivos; entre 1,400 y 7,500 palabras cada uno |
| Spec + UI-spec de la casilla | 7,311 + 6,414 palabras |
| Particiones | `ui-componentes-base` → 6 tarjetas y 3 derivadas; `nuc-auth` → 5 tarjetas y 1 derivada |

### 2.6 Dos tarjetas triviales

| Tarjeta | Cambio real | Lo que costó |
| --- | --- | --- |
| `infra-ci-tiempo-verify` | Cambiar `timeout-minutes` de 30 a 90 | Spec de 98 líneas con 9 criterios, 9 runs, 9 veredictos, review de 4 lentes |
| `infra-ci-hoom-huella` | Corregir un SHA-256 en un archivo | Spec de 94 líneas, 6 runs, review de 4 lentes con 0 hallazgos |

## 3. Invariantes: lo que no se toca

Estos son los principios que diferencian a hoom. Ningún cambio de este documento puede debilitarlos, y cada spec que salga de aquí debe citarlos.

| ID | Invariante |
| --- | --- |
| I1 | El veredicto sale de comandos que se ejecutan, nunca de lo que diga un modelo. |
| I2 | Un verde solo vale para el árbol exacto que se verificó (huella). |
| I3 | El test-writer trabaja en un árbol ciego, sin la implementación en disco. |
| I4 | La review corre en un proveedor distinto del que escribió, sobre evidencia congelada. |
| I5 | Cada rol tiene un territorio de escritura, y el binario lo comprueba. |
| I6 | Los hallazgos son append-only y cerrar uno exige evidencia. |
| I7 | El spec lo aprueba una persona, y la aprobación queda atada al contenido. |
| I8 | El cierre de una tarjeta exige un veredicto completo, con todos los gates. |
| I9 | **Nuevo.** Ninguna decisión de proporcionalidad la toma un modelo. El riesgo, las lentes, los gates que aplican y los topes salen de la tarjeta (una persona), del diff y de `hoom.yaml`. hoom puede subir el riesgo por regla; bajarlo, solo una persona. |

I9 es la condición para que esta calibración no rompa el principio de «no creerle al modelo». Si un rol pudiera declarar que su propio cambio es de riesgo bajo, hoom estaría confiando en el modelo justo en la decisión que más importa.

## 4. Los 12 cambios

Cada cambio trae: el problema con su dato, lo que hoom hace hoy, el cambio, criterios de aceptación semilla, el invariante que protege y dónde tocar.

### CAL-1 · Carriles por riesgo

**Problema.** Todas las tarjetas recorren el mismo camino: spec, test-writer, writer, review de 3 o 4 lentes, refutador. Ver 2.6.

**Hoy.** La tarjeta ya tiene un campo de riesgo (`hoom item add --riesgo`, `hoom item set --riesgo bajo|normal|alto`) que fija una persona. En `oceanbuild` solo 5 de 29 tarjetas lo tienen, todas en `alto`. No se observó que cambie el proceso: las tarjetas con `riesgo: alto` y las que no tienen riesgo recibieron el mismo camino.

**Cambio.** El riesgo de la tarjeta decide el camino.

| Paso | `bajo` | `normal` | `alto` |
| --- | --- | --- | --- |
| Spec | El `pedido` de la tarjeta | Spec corto del arquitecto (CAL-7) | Spec completo del arquitecto |
| Aprobación humana | Al fijar `riesgo: bajo` | `hoom aprobar` | `hoom aprobar` |
| Pruebas | Las del writer o `[verifica: <comando>]` | Test-writer ciego, acotado (CAL-6) | Test-writer ciego, adversarial |
| Review | 0 lentes | 1 lente | 4 lentes |
| Refutador | No se invoca | Solo bloqueantes (CAL-5) | Solo bloqueantes (CAL-5) |
| Verify de cierre | Completo | Completo | Completo |

Reglas para decidir el riesgo, todas deterministas:

1. Lo declara una persona en la tarjeta. Sin declaración, es `normal`.
2. hoom lo recalcula en cada `verify` a partir del diff contra la base. Si el diff toca una ruta crítica, el riesgo efectivo es `alto`. Una tarjeta de tipo `seguridad` es `alto`.
3. `bajo` solo se sostiene si el diff toca únicamente rutas de bajo riesgo (documentación, textos, valores de configuración). Si toca otra cosa, sube a `normal`.
4. hoom solo sube el riesgo. Bajarlo lo hace una persona con `hoom item set --riesgo`, y queda registrado quién y cuándo.
5. Un rol que escribe el campo de riesgo comete manipulación, igual que hoy con `hoom.yaml`.
6. El veredicto registra el riesgo efectivo y el motivo (declarado, ruta crítica tocada, tipo de tarjeta).

Las rutas críticas y las de bajo riesgo se declaran en `hoom.yaml`. `oceanbuild` ya tiene una lista de rutas críticas en `tests/mutacion-critica.txt` (horas extra, costeo, monedas, sincronización, tenancy, autenticación); conviene que el carril y la mutación lean la misma lista.

**Criterios semilla.**

- CAL-1.1: una tarjeta sin `riesgo` se trata como `normal`.
- CAL-1.2: un diff que toca una ruta crítica deja el riesgo efectivo en `alto`, aunque la tarjeta diga `normal` o `bajo`.
- CAL-1.3: una tarjeta `bajo` cuyo diff toca una ruta fuera de las de bajo riesgo sube a `normal`.
- CAL-1.4: ningún comando ejecutado por un rol puede bajar el riesgo; el intento queda como hallazgo de manipulación.
- CAL-1.5: el veredicto incluye el riesgo efectivo y su motivo.
- CAL-1.6: `hoom board` y `hoom tablero` muestran el riesgo efectivo de cada tarjeta.

**Protege.** I9, I7, I8. **Dónde tocar.** Binario (cálculo del riesgo efectivo, veredicto, tablero, piloto); contratos `00-orquestador.md` y `06-reviewer.md`; plantilla de `hoom.yaml`.

### CAL-2 · Lentes según el riesgo, no según las líneas

**Problema.** Las 34 reviews usaron 3 o 4 lentes. Una corrección de un SHA pasó por 4 lentes y dio 0 hallazgos.

**Hoy.** El contrato `06-reviewer.md` dice: cambio estándar, 1 lente; seguridad, auth, pagos, DTE o más de 400 líneas cambiadas, 4 lentes. El umbral sale del veredicto (`insertions + deletions`) y cuenta todo: pruebas y specs incluidos. Ejemplo: el veredicto de `nuc-auth` reporta 5,581 inserciones en 37 archivos, de los cuales 15 son pruebas y 4 son de `.hoom/`. Con eso, toda tarjeta supera 400.

**Cambio.**

- Las lentes salen del riesgo efectivo: `bajo`, 0; `normal`, 1; `alto`, 4.
- El tamaño del diff deja de subir el número de lentes.
- En `normal`, la lente se elige por regla según las rutas tocadas (por ejemplo, shell y CI → `resilience`; por defecto → `reliability`). El mapa vive en `hoom.yaml`.
- Una persona puede pedir más lentes con `hoom review --lens`.
- La re-review después de una corrección corre sobre el delta y solo con las lentes que tuvieron hallazgos bloqueantes.

**Criterios semilla.**

- CAL-2.1: una tarjeta `normal` con un diff de 3,000 líneas recibe 1 lente.
- CAL-2.2: una tarjeta `alto` recibe las 4 lentes en el orden actual (risk, reliability, resilience, readability).
- CAL-2.3: una tarjeta `bajo` no invoca al reviewer y el registro de review lo dice con su motivo.
- CAL-2.4: el registro en `.hoom/reviews/` guarda qué regla eligió las lentes.

**Protege.** I4, I9. **Dónde tocar.** Binario (`hoom review`, selección de lentes); contratos `06-reviewer.md` y `00-orquestador.md` (punto 4b); plantilla de `hoom.yaml`.

### CAL-3 · Medir el tamaño sin pruebas ni estado de hoom

**Problema.** Las pruebas y los specs inflan el tamaño, y las tarjetas se parten «por tamaño». Cada parte vuelve a pagar spec, pruebas, review y verify. `ui-componentes-base` terminó en 6 tarjetas.

**Hoy.** El veredicto reporta un solo número (`insertions`, `deletions`) sobre todos los archivos cambiados. La evidencia de review tiene un tope (`review.max_evidence_kib`, 320 KiB); si se pasa, no corre ninguna lente. En `oceanbuild` las primeras reviews llevaron entre 10 y 260 KiB.

**Cambio.**

- El veredicto reporta el tamaño en tres números: producto, pruebas y estado de hoom (`.hoom/**`). Los archivos generados y los lockfiles quedan fuera de los tres.
- Todo umbral de tamaño (avisos de partir, presupuesto de corrección de ~120 líneas) usa solo el número de producto.
- Una tarjeta se parte por comportamiento, no por volumen de pruebas. El aviso de partir aparece solo cuando el producto supera el umbral.

**Criterios semilla.**

- CAL-3.1: un diff con 90 líneas de producto y 1,500 de pruebas reporta `producto: 90` y no dispara ningún umbral de tamaño.
- CAL-3.2: `package-lock.json` y `composer.lock` no suman a ningún número.
- CAL-3.3: `hoom status` y el tablero muestran los tres números.

**Pregunta abierta.** Qué hacer cuando la evidencia supera el tope solo por las pruebas. Ver P3.

**Protege.** I4. **Dónde tocar.** Binario (veredicto, evidencia de review); contrato `00-orquestador.md`.

### CAL-4 · Corregir solo lo que bloquea, en una sola ronda

**Problema.** 74 de los 81 hallazgos del reviewer que se corrigieron eran medium o low. Cada corrección abrió otra ronda: arquitecto, test-writer, writer, verify y review. Las correcciones son la fase más cara ($185).

**Hoy.** El gate `findings_open` funciona bien: con `block_on: high`, ningún veredicto salió rojo por un medium o un low. Pero la regla no llega al ciclo de trabajo. El contrato `09-refutador.md` dice que todo hallazgo corroborado lo corrige el writer, y `00-orquestador.md` no menciona la severidad.

**Cambio.**

- Después de una review, solo van a corrección los hallazgos que bloquean: los de severidad igual o mayor a `findings.block_on` y los que una persona subió con `hoom finding raise`.
- Los demás quedan abiertos como `pendiente` (el estado ya existe en `hoom finding list`) y no generan trabajo en la tarjeta.
- Una ronda: una corrección que atiende todos los bloqueantes, y luego una re-review sobre el delta que solo comprueba esos hallazgos.
- Si la re-review trae un bloqueante nuevo, hay una segunda y última ronda. Después se escala a una persona. Un hallazgo nuevo que no bloquea queda `pendiente`.
- Al cerrar, la tarjeta lista sus pendientes. El tablero los muestra.
- `hoom siguiente` y `hoom piloto` no proponen corregir cuando no hay bloqueantes.

**Criterios semilla.**

- CAL-4.1: con `block_on: high` y solo hallazgos medium y low abiertos, la acción siguiente de la tarjeta es cerrar, no corregir.
- CAL-4.2: un hallazgo medium subido con `hoom finding raise` entra a la corrección.
- CAL-4.3: después de dos rondas de corrección con bloqueantes abiertos, la tarjeta pide decisión humana y no lanza otro rol.
- CAL-4.4: los hallazgos `pendiente` siguen en `.hoom/findings/` sin resolución y aparecen en el cierre.

**Protege.** I6. **Dónde tocar.** Contratos `00-orquestador.md` y `09-refutador.md`; binario (`siguiente`, `piloto`, tablero).

### CAL-5 · Refutador solo para los bloqueantes

**Problema.** El refutador gastó 16 runs y el 7 % de los tokens. Sus 20 refutaciones sobre hallazgos del reviewer fueron de medium y low, que no bloqueaban. No tumbó ningún high del reviewer.

**Hoy.** `09-refutador.md` dice «para CADA hallazgo abierto».

**Cambio.** El refutador recibe solo los hallazgos que bloquean. Si no hay ninguno, no se invoca.

**Criterios semilla.**

- CAL-5.1: el pedido al refutador lista únicamente hallazgos bloqueantes.
- CAL-5.2: sin bloqueantes abiertos, `hoom siguiente` no propone al refutador.

**Protege.** I6. **Dónde tocar.** Contrato `09-refutador.md`; binario (armado del pedido, `siguiente`, `piloto`).

### CAL-6 · Pruebas acotadas en riesgo normal

**Problema.** Las pruebas son 7 veces el código. Antes de la primera review, el test-writer escribió más del doble de texto que el writer.

**Hoy.** `05-test-writer.md` pide al menos una prueba por criterio, una prueba hostil por caso límite (vacíos, nulos, límites, unicode, montos raros, fechas borde, concurrencia) y property-based donde se pueda. No tiene tope.

**Cambio.**

- Riesgo `alto`: como hoy.
- Riesgo `normal`: una prueba por criterio del spec y una por cada caso límite que el spec enumera. Sin casos inventados y sin property-based.
- El gate `spec_trace` reporta cuántas pruebas hay por criterio.
- Aviso amarillo, que no bloquea, cuando una tarjeta `normal` supera un tope de pruebas por criterio.

**Criterios semilla.**

- CAL-6.1: el contrato del test-writer que recibe el rol cambia según el riesgo efectivo.
- CAL-6.2: un criterio sin prueba sigue dejando rojo `spec_trace`, en cualquier riesgo.
- CAL-6.3: `spec_trace` muestra el número de pruebas por criterio.

**Protege.** I3. El test-writer sigue en árbol ciego en `normal` y en `alto`. **Dónde tocar.** Contrato `05-test-writer.md`; binario (`spec_trace`, selección de contrato por riesgo).

### CAL-7 · Specs cortos, comprobados por `spec_lint`

**Problema.** El arquitecto es el rol más caro ($155 en 49 runs). Hubo 29 aprobaciones de spec, 6 en una sola tarjeta. Un spec de 7,000 palabras no se aprueba leyéndolo.

**Hoy.** `01-arquitecto.md` dice «el spec debe poder leerse en 3 minutos o menos». Es una regla de prompt. `spec_lint` comprueba la estructura y cuenta criterios, no la longitud.

**Cambio.**

- `spec_lint` mide palabras y número de criterios contra topes por riesgo, configurables en `hoom.yaml`.
- Punto de partida propuesto: `normal`, 800 palabras y 8 criterios; `alto`, 2,000 palabras y 15 criterios; UI-spec, 800 palabras.
- Pasar el tope deja rojo `spec_lint`, con el mensaje: reducir el alcance, o que una persona suba el tope de esa tarjeta con un motivo que quede registrado.
- El arquitecto referencia el intake por ruta y sección en vez de copiarlo.
- El designer solo se invoca cuando la tarjeta tiene una decisión visual.

**Criterios semilla.**

- CAL-7.1: un spec `normal` de 801 palabras deja rojo `spec_lint` y el mensaje dice el tope y el conteo.
- CAL-7.2: subir el tope de una tarjeta exige una persona y un motivo, y queda en el historial de la tarjeta.
- CAL-7.3: `hoom spec status` muestra palabras y criterios contra el tope.

**Protege.** I7. La aprobación vuelve a ser real. **Dónde tocar.** Binario (`spec_lint`, `spec status`); contratos `01-arquitecto.md` y `02-designer.md`; plantilla de `hoom.yaml`.

### CAL-8 · Verify dentro del entorno del proyecto

**Problema.** En Linux con DDEV, el host no tiene PHP. El sobre de `hoom agent` cierra con scope → verify → check en el host, y los 137 veredictos automáticos salieron rojos. 171 de 172 sobres de roles distintos del reviewer quedaron `no-entregable`. El veredicto real hay que sacarlo aparte.

**Hoy.**

- Los gates PHP fallan con exit 127 (`env: «php»: No existe el fichero o el directorio`) y quedan como `fail`.
- Otros 13 veredictos fallaron con `vendor/bin/pint: No existe`.
- Los writers lo rodean por su cuenta: 22 de 39 runs de writer corrieron `ddev exec -d /var/www/html/.hoom/worktrees/<slug> hoom verify`.
- En macOS y Windows con Herd, PHP está en el host y el problema no existe.

**Cambio.**

- hoom ejecuta los gates en el entorno donde vive el runtime del proyecto. Dos vías, a decidir (P6):
  - Detección por perfil: si existe `.ddev/` y el host no tiene el runtime, cada gate corre dentro del contenedor, en la ruta que corresponde al árbol (también para worktrees).
  - Ajuste local por máquina, fuera de git, con el prefijo de ejecución.
- `hoom.yaml` queda idéntico para Herd, DDEV y CI.
- Un comando que no existe (exit 127) deja el gate en `error` de entorno, no en `fail`. «No pude correr» y «corrió y falló» son cosas distintas.
- El veredicto registra en qué entorno corrió cada gate.

**Criterios semilla.**

- CAL-8.1: en una máquina con `ddev` y sin `php`, el verify del sobre de `hoom agent` da el mismo resultado que `ddev hoom verify` en el mismo árbol.
- CAL-8.2: en una máquina con `php` en el host, nada cambia.
- CAL-8.3: un gate cuyo comando no existe queda `error` con un mensaje que nombra el comando faltante.
- CAL-8.4: el verify funciona igual dentro de `.hoom/worktrees/<slug>/`.

**Protege.** I1. Hoy el rojo automático no significa nada en esta máquina, y eso enseña a ignorar los rojos. **Dónde tocar.** Binario (`hoom verify`, `hoom agent`); perfil `laravel`.

### CAL-9 · Gates según lo que cambió, y verify completo al cierre

**Problema.** La mutación tarda 328 segundos por corrida y es el 74 % del tiempo de gates. Corre completa también en tarjetas que solo tocan Vue. Los gates PHP corren en tarjetas sin PHP.

**Hoy.** `hoom verify` ya acota por diff con `diff_cmd`, y `--full` lo ignora. En `oceanbuild`, el `diff_cmd` de `mutation` es el mismo comando que `cmd`, así que nunca acota. `--gate` deja un veredicto parcial que no sirve para entregar.

**Cambio.**

- Cada gate puede declarar en `hoom.yaml` las rutas de las que depende.
- En un verify de trabajo, un gate cuyas rutas no aparecen en el diff contra la base queda `no aplica`, con su motivo. Lo decide hoom desde el diff.
- `hoom task done` e `integrar` exigen un veredicto completo sobre la huella final, como hoy.
- El perfil `laravel` trae un `diff_cmd` de mutación que muta solo los archivos cambiados.

**Criterios semilla.**

- CAL-9.1: en una tarjeta que solo toca `resources/js/**`, el verify de trabajo deja `no aplica` los gates PHP y lo explica.
- CAL-9.2: `hoom task done` rechaza un veredicto con gates en `no aplica` y pide el completo.
- CAL-9.3: tocar un archivo de una ruta crítica hace que la mutación de esa ruta corra en el verify de trabajo.

**Protege.** I8, I9. **Dónde tocar.** Binario (`hoom verify`, `task done`); plantilla de `hoom.yaml` del perfil `laravel`.

### CAL-10 · Menos runs por tarjeta

**Problema.** Cada run arranca de cero y vuelve a leer el contexto: entre 0.85 y 3.5 millones de tokens por run. Después de la primera review hubo 58 runs de corrección: 20 de writer, 22 de test-writer y 16 de arquitecto.

**Hoy.** Cada corrección es un run nuevo. `hoom run --resume` y `hoom reanudar` existen.

**Cambio.**

- Una ronda de corrección es un solo run de writer con todos los bloqueantes.
- El arquitecto solo se relanza si la corrección cambia un criterio de aceptación.
- Cuando la corrección es del mismo rol en la misma tarjeta, se reanuda su sesión en vez de abrir una nueva. Dos excepciones: el reviewer siempre corre con contexto fresco, y el test-writer sigue en árbol ciego.

**Criterios semilla.**

- CAL-10.1: una ronda con 5 bloqueantes lanza un run de writer, no cinco.
- CAL-10.2: `hoom costo <slug>` muestra runs por ronda de corrección.
- CAL-10.3: el reviewer nunca reanuda una sesión anterior.

**Protege.** I4. **Dónde tocar.** Binario (`piloto`, `siguiente`, `pedir`); contrato `00-orquestador.md`.

### CAL-11 · Modelo por rol y por riesgo

**Problema.** Los 166 runs de Claude corrieron en Opus, sin distinguir rol ni riesgo. Una tarjeta promedio consume 12 % de la ventana de 5 horas y 1.8 % de la cuota semanal de la cuenta donde se midió.

**Hoy.** `hoom.yaml` ya acepta `agents.<rol>.model`, `effort`, `budget_usd` y `timeout`. `oceanbuild` no los define, así que todo cae en el modelo por defecto del proveedor. La tarjeta ya acepta `--presupuesto-usd`.

**Cambio.**

- `hoom init` deja en `hoom.yaml` un modelo por rol.
- `agents.<rol>` acepta valores por riesgo.
- Sugerencia: arquitecto y roles de tarjetas `alto`, el modelo más fuerte; writer y test-writer en `normal` y `bajo`, un modelo medio; scout, uno barato.
- Presupuesto por defecto de la tarjeta según su riesgo.

**Criterios semilla.**

- CAL-11.1: con `agents.writer` configurado por riesgo, un run de writer en una tarjeta `normal` usa el modelo de `normal` y lo imprime con su origen antes de lanzar.
- CAL-11.2: un `hoom.yaml` recién creado por `hoom init` trae modelo por rol.
- CAL-11.3: el reviewer sigue en un proveedor distinto del que escribió.

**Protege.** I4. Como el veredicto no depende de creerle al writer (I1), un writer más barato no baja la garantía: los gates deciden. **Dónde tocar.** Binario (resolución de valores de lanzamiento, `hoom init`); plantillas de `hoom.yaml`.

### CAL-12 · Tarjetas en paralelo sin choques

**Problema.** De unas 42 horas de reloj, los agentes trabajaron 13.9 y el verify unas 6. El resto es espera entre pasos. Casi siempre corrió un solo agente.

**Hoy.**

- Cada tarea ya vive en su worktree (`hoom task start`).
- Las tarjetas comparten contenedor y base de pruebas (`oceanbuild_testing`). El writer de `nuc-auth` revisó si había otros procesos de `pest` corriendo mientras `infra-semgrep` trabajaba en paralelo.
- 13 veredictos fallaron con `vendor/bin/pint: No existe`, lo que apunta a worktrees sin dependencias instaladas.

**Cambio.**

- `hoom task start` deja el worktree listo para correr: dependencias instaladas y recursos propios. El perfil define cómo se derivan del slug (por ejemplo, una base de pruebas por tarea).
- El tablero y el piloto señalan qué tarjetas no dependen entre sí y se pueden lanzar a la vez.
- Tope configurable de tarjetas simultáneas.

**Criterios semilla.**

- CAL-12.1: dos tarjetas que corren su suite a la vez no comparten base de pruebas.
- CAL-12.2: un worktree recién creado pasa el gate `lint` sin pasos manuales.
- CAL-12.3: el tablero marca las tarjetas sin dependencias pendientes entre sí.

**Protege.** I2, I5. Baja el tiempo de reloj, no los tokens. **Dónde tocar.** Binario (`task start`, tablero, piloto); perfil `laravel`.

## 5. Orden sugerido

| Fase | Cambios | Por qué primero |
| --- | --- | --- |
| 1 | CAL-8 | Es un defecto: el veredicto automático no sirve en DDEV. Sin esto no se puede medir bien el resto. |
| 2 | CAL-4, CAL-5, CAL-10 | Atacan el 47 % de los tokens que se gasta después de la primera review. Casi todo es contrato y lógica de `siguiente` y `piloto`. |
| 3 | CAL-3, CAL-1, CAL-2 | El riesgo efectivo es la base de las lentes, las pruebas y los specs por riesgo. |
| 4 | CAL-6, CAL-7, CAL-11 | Dependen del riesgo efectivo de CAL-1. |
| 5 | CAL-9, CAL-12 | Bajan tiempo de reloj. Son los que más tocan los perfiles. |

Dependencias: CAL-2, CAL-6, CAL-7 y CAL-11 dependen de CAL-1. CAL-5 depende de CAL-4. CAL-12 se apoya en CAL-8.

Sin tocar el binario, un proyecto ya puede hacer hoy la mitad de CAL-11: definir `agents.<rol>.model` en su `hoom.yaml`.

## 6. Cómo medir el resultado

Los mismos números de la sección 2, con las mismas fuentes y con `hoom costo`. Las metas son propuestas para ajustar con la primera medición.

| Métrica | Hoy | Meta propuesta |
| --- | --- | --- |
| Runs por tarjeta | 18.4 | 6 o menos en `normal`; 12 o menos en `alto` |
| Tokens después de la primera review | 47 % | 25 % o menos |
| Reviews con 3 o 4 lentes | 34 de 34 | Solo tarjetas `alto` |
| Hallazgos corregidos que no bloqueaban | 74 de 81 | 0, salvo los subidos con `raise` |
| Veredictos automáticos rojos por entorno | 137 de 137 | 0 |
| Corridas completas de mutación por tarjeta | Una por verify | Una al cierre |
| Pruebas por línea de código en `normal` | 7 a 1 | 3 a 1 o menos |
| Palabras por spec | 1,400 a 7,500 | Dentro del tope de su riesgo |
| Tarjetas por ítem de backlog | Hasta 6 | 1, salvo partición por comportamiento |

Forma de comprobarlo: aplicar los cambios a 2 o 3 tarjetas reales de un proyecto y comparar contra esta tabla.

## 7. Preguntas abiertas para Hoom

- **P1.** En riesgo `bajo`, ¿las pruebas las escribe el writer, o solo se aceptan criterios con `[verifica: <comando>]`? La segunda opción es más estricta con I3.
- **P2.** ¿`findings.block_on` debe poder variar por riesgo? Por ejemplo, `medium` en tarjetas `alto`. La alternativa es dejar `high` en todo y usar `hoom finding raise` caso por caso.
- **P3.** Cuando la evidencia de review supera el tope solo por las pruebas, ¿las pruebas entran resumidas (nombre y criterio) o la tarjeta se parte?
- **P4.** Topes iniciales de palabras y criterios por riesgo (CAL-7).
- **P5.** ¿Qué rutas son de bajo riesgo por defecto en cada perfil?
- **P6.** CAL-8: ¿detección automática por perfil, ajuste local por máquina, o ambas?
- **P7.** ¿Los cambios aplican a tarjetas ya abiertas o solo a las nuevas?
- **P8.** CAL-10: ¿se acepta reanudar la sesión del writer entre rondas, o cada ronda debe arrancar con contexto fresco?

## 8. Observaciones fuera de los 12 cambios

- **Territorio contra el objeto de la tarjeta.** En `infra-ci-cd`, `infra-semgrep` e `infra-semgrep-excluir-hoom`, el objeto de la tarjeta era editar `hoom.yaml`, y el gate de scope registró hallazgos high de manipulación contra el writer por tocarlo. Hace falta una forma de que una persona autorice ese archivo para una tarjeta concreta.
- **Estado de hoom en git.** `oceanbuild` tiene 47,548 líneas de artefactos de hoom versionadas, y 206 de 290 commits son de contabilidad. Un veredicto rojo viejo que citaba un literal disparó el gate `security` y costó una tarjeta entera (`infra-semgrep-excluir-hoom`).
- **Costo del orquestador.** No queda registrado en `.hoom/runs`. Sin ese dato, `hoom costo` subestima el gasto real de una tarjeta.

## 9. Fuera de alcance

- El orden de trabajo del producto en `oceanbuild` (pantallas completas antes que el kit de UI, uso de shadcn-vue). Son decisiones del proyecto, no de hoom.
- Cambiar la lista de gates o sus umbrales (mutación a 60 y 90, PHPStan al máximo).

## 10. Revisión contra el código de hoomAI (2026-11-21)

Hecha por el orquestador de hoomAI sobre `main` (a114966), leyendo el código. Corrige lo que las secciones 1 a 9 dicen de hoom visto desde fuera y fija cómo se reparte el trabajo en tarjetas. Donde esta sección choca con las anteriores, manda esta.

### 10.1 Lo que ya existe

- **Riesgo por tarjeta** (`internal/riesgo`, desde v0.14.0, spec `ciclo-de-review-acotado`). `bajo` da 1 lente (reliability) salvo que el cambio toque una ruta de riesgo; `alto` da las 4. En `alto`, los medium de la review completa bloquean, y en cualquier riesgo un medium en ruta de riesgo bloquea (`finding.Bloquea`). En `normal` sigue la regla vieja: ruta de riesgo o más de 400 líneas (`reviewcmd.UmbralLineas`, que cuenta pruebas y specs) dan las 4 lentes. Por eso ninguna tarjeta `normal` recibió 1 lente. hoomAI tiene el mismo patrón: 155 de sus 156 reviews usaron 3 o 4 lentes.
- **Las reviews de 3 lentes son deltas**: una delta no corre readability.
- **CAL-4 y CAL-5 ya están en el binario.** `hoom siguiente` solo propone corregir cuando hay bloqueantes. El piloto le pasa al refutador solo los bloqueantes que ningún refutador recibió, y para en `autonomia.rondas_de_hallazgos` o cuando los bloqueantes no bajan. Lo que falta es que el camino a mano (el orquestador lanzando roles) siga la misma regla: los contratos 00 y 09 no la dicen (09: «para CADA hallazgo abierto»), `hoom pedir` no tiene refutador y fuera del piloto no hay tope de rondas.
- **«Pendiente» no es un estado del hallazgo**: es la vista «abierto y no bloquea» (`hoom finding list`, `findings.max_pendientes`). Los estados son abierto, corregido y refutado.
- **Gates.** El estado `error` existe, pero el exit 127 cae en `fail` (`internal/gates/runner.go`), contra lo que promete el comentario del paquete. No hay prefijo de ejecución (ddev, docker), y el encierro tapa `/run/docker.sock`. El veredicto no registra el entorno.
- **`diff_cmd`** acepta `{base}`, `{files}` y `{packages}`; el perfil `laravel` ya acota la mutación con Infection. No hay rutas por gate ni estado «no aplica», y una clave desconocida en un gate se ignora sin avisar.
- **Valores de lanzamiento.** `agents.<rol>.model|effort|budget_usd|timeout` funciona. El origen se imprime en `pedir`, `siguiente` y `reanudar`, no en el piloto. `hoom init` no escribe `agents:`. Nada varía por riesgo.
- **`hoom reanudar`** ya prohíbe reanudar al reviewer y a los roles ciegos.
- **`hoom task start`** solo hace `git worktree add`. No hay dependencias entre ítems ni tope de tareas simultáneas.
- **El costo del orquestador** no queda en `.hoom/runs`, y `hoom costo` cuenta como «rondas» los registros de review, no las rondas del piloto.

### 10.2 Ajustes a los cambios

- **Riesgo declarado contra efectivo.** El spec (CAL-7), el test-writer (CAL-6) y el modelo (CAL-11) ocurren antes de que exista el diff, así que usan el riesgo **declarado**. Si después el diff sube el efectivo, hoom lo señala y una persona sube el declarado; hoom no relanza roles por eso.
- **CAL-1.** `bajo` solo se sostiene si todo el diff está en rutas de bajo riesgo (lista permitida: falla cerrado). Las rutas de riesgo y las de bajo riesgo pasan a `hoom.yaml`, con valores por defecto en cada perfil; la lista fija de hoy (`riesgo.Marcas`) queda como valor por defecto, revisando su `sign`, que también atrapa `design`.
- **CAL-2.** El tamaño sigue contando, pero solo el de producto (CAL-3). `bajo` recibe 1 lente; 0 solo para documentación, como hoy. Una delta que toca una ruta de riesgo siempre corre `risk`.
- **CAL-6.** En `alto`, como hoy. En `normal` se mantiene una prueba hostil por cada caso límite que el spec enumera; esa lista la aprueba una persona (I7).
- **CAL-8.** Genérico, no solo DDEV: también Sail, Docker Compose, devcontainers y Lando. Prefijo de ejecución local por máquina, fuera de git, explícito y registrado en el veredicto; el perfil solo lo sugiere. Se diseña junto con el encierro.
- **CAL-9.** Además, `hoom.yaml` rechaza las claves desconocidas de un gate.
- **CAL-10.** Reanudar entre rondas queda como opción a medir, no como regla: reanudar re-envía todo el contexto anterior. Los runs por ronda en `hoom costo` pasan a la tarjeta de costo.
- **CAL-11.** `hoom init` no escribe nombres de modelo, porque envejecen en meses: niveles (fuerte, medio, barato) traducidos por proveedor en un solo lugar.
- **CAL-12.** Genérico: un `setup` de worktree en el perfil, una variable `HOOM_TASK` para que el proyecto derive sus recursos, dependencias entre ítems y un tope de tarjetas simultáneas. La base de datos por tarea es del proyecto, no de hoom.

### 10.3 Cambios nuevos

- **CAL-13 · `hoom.yaml` autorizado por tarjeta** (de la sección 8). Una persona autoriza ese archivo para una tarjeta concreta, y queda registrado; sin eso, tocarlo sigue siendo manipulación.
- **CAL-14 · Costo del orquestador y runs por ronda** (sección 8 y CAL-10.2). La sesión del orquestador entra en `hoom costo`, y `hoom costo` muestra los runs de cada ronda.
- El volumen del estado de hoom en git queda como observación, sin tarjeta por ahora.

### 10.4 Respuestas a las preguntas abiertas

- **P2.** Ya está resuelta en el código: en `alto`, los medium de la review completa bloquean.
- **P6.** Ajuste local por máquina, explícito; el perfil solo lo sugiere. La detección automática haría que el veredicto cambie según la máquina sin que nadie lo note.
- **P8.** Reanudar al writer es una opción, no la regla. El reviewer y los roles ciegos nunca reanudan.
- **P1, P3, P4, P5 y P7** se deciden en el spec de la tarjeta que los usa.

### 10.5 Tarjetas

| Orden | Tarjeta | Cambios |
| --- | --- | --- |
| 1 | `ciclo-lo-decide-hoom` | CAL-4, CAL-5 y la parte de contrato de CAL-10 |
| 2 | `verify-en-el-entorno` | CAL-8 |
| 3 | `riesgo-efectivo-y-tamano` | CAL-3, CAL-1 y CAL-2 |
| 4 | `proporcion-por-riesgo` | CAL-7, CAL-6 y CAL-11 |
| 5 | `gates-por-ruta` | CAL-9 |
| 6 | `tareas-en-paralelo` | CAL-12 |
| 7 | `hoom-yaml-por-tarjeta` | CAL-13 |
| 8 | `costo-del-orquestador` | CAL-14 |

Ocho de los doce cambios dependen del riesgo efectivo: la tarjeta 3 se diseña con la regla de declarado contra efectivo de 10.2.

### 10.6 Estimación

Unos 40 % menos tokens por tarjeta: correcciones de 21 % a ~6 %, reviews de 18 % a ~8 %, refutador de 7 % a ~2 %, spec de 17 % a ~11 % y pruebas de 15 % a ~10 %; el código de primera pasada casi no cambia. Entre 40 y 55 % menos tiempo de agentes y gates. En las tarjetas `alto` el ahorro es menor, del 20 al 30 %, a propósito. Es una estimación sobre el reparto por fase de la sección 2.2, no una medición: se mide con `hoom costo` después de la tarjeta 1.
