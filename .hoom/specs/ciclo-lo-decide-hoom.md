# Spec: la ronda de hallazgos la decide hoom, también a mano

Estado: BORRADOR — pendiente de aprobación humana.
Tarjeta: `ciclo-lo-decide-hoom`. Intake: `.hoom/intake/rfc-calibracion-por-riesgo.md`, CAL-4, CAL-5 y la parte de contrato de CAL-10 (secciones 4 y 10). Criterios CA-747 a CA-759.

## Objetivo

Después de una review, qué se corrige, qué ve el refutador y cuántas rondas hay lo decide hoom, por cualquier camino: el piloto, `hoom siguiente`, `hoom pedir` o el orquestador.

El piloto ya lo hace: solo pide corregir los bloqueantes, le pasa al refutador solo los bloqueantes que ningún refutador recibió y para en un tope de rondas. A mano no: `hoom pedir` no tiene refutador, fuera del piloto no hay tope, y los contratos 00 y 09 le piden al orquestador refutar «CADA hallazgo abierto» sin mencionar la severidad. En el proyecto medido del intake, 74 de los 81 hallazgos corregidos no bloqueaban, y las correcciones fueron la fase más cara (21 % de los tokens). Ese gasto lo decidió un modelo, que es lo que I9 del intake prohíbe.

## No-goals

- Cambiar qué bloquea: `finding.Bloquea` queda igual.
- Cambiar la secuencia de la ronda del piloto (refutador, test-writer, writer, review delta).
- Reanudar sesiones entre rondas (CAL-10: se mide aparte).
- Runs por ronda en `hoom costo` (tarjeta `costo-del-orquestador`).
- Cerrar `hoom agent`: sigue siendo el verbo de bajo nivel. El contrato dice que una ronda no se arma con él.
- Lentes, tamaño y riesgo efectivo (tarjeta `riesgo-efectivo-y-tamano`).

## Contratos

### Qué es una ronda

`Rondas(tarea)` es el número de registros de review de la tarea en `.hoom/reviews/` con cobertura `delta` y fecha posterior a su registro con cobertura `completa` más nuevo. Sin registro `completa`, es 0. Una review completa nueva lo vuelve a 0. El piloto, `hoom siguiente`, `hoom pedir` y el tablero cuentan con esta misma función. El piloto sigue numerando sus rondas con el campo `ronda` de sus sobres; el tope de abajo se suma a sus paradas de hoy.

### El tope: `findings.max_rondas`

- Clave nueva de `hoom.yaml`: `findings.max_rondas: <n>`, entero de 1 a 5. Se lee del mismo `hoom.yaml` del que el tablero lee hoy `findings.block_on`.
- Sin la clave, el tope es 2, o `autonomia.rondas_de_hallazgos` si es mayor: un `hoom.yaml` de hoy no cambia de comportamiento.
- Errores de carga:
  - valor fuera de rango o no entero: `findings: max_rondas tiene que ser un entero de 1 a 5`;
  - clave desconocida en `findings`: `findings: clave desconocida "<k>" (validas: block_on, max_pendientes, max_rondas)`;
  - `autonomia.rondas_de_hallazgos` mayor que un `findings.max_rondas` declarado: `autonomia: rondas_de_hallazgos (<a>) pasa de findings.max_rondas (<m>)`.
- **Al tope** quiere decir: `Rondas(tarea) >= tope` y la tarjeta tiene al menos un hallazgo que bloquea. Al tope:
  - la tarjeta queda en Review, sin acciones de rol, con la razón `se hicieron <n> rondas de hallazgos y quedan <k> que bloquean: te toca` y la acción `hoom finding list --open`;
  - `hoom siguiente` no lanza nada e imprime `te toca: hoom finding list --open` y, en la línea siguiente, esa razón;
  - `hoom pedir writer` y `hoom pedir refutador` salen con exit 1 y esa razón, sin lanzar;
  - el piloto para con esa razón.
- La persona sale del tope refutando o corrigiendo a mano, o subiendo `findings.max_rondas`. Un rol que edita `hoom.yaml` comete manipulación, como hoy.

### El refutador en `hoom pedir` y `hoom siguiente`

- `hoom pedir` gana el rol `refutador`. Sus roles quedan: arquitecto, test-writer, writer, refutador, reviewer. El error de rol desconocido nombra esos cinco.
- **Bloqueantes sin refutar**: los hallazgos que bloquean y que no están en el campo `hallazgos` de ningún sobre de refutador de la tarjeta cuyo `stage` no es `run`. Es la regla de hoy del piloto (`piloto.go`), que pasa a una sola función que usan los dos.
- En Review, bajo el tope:
  - con bloqueantes sin refutar, la primera acción es `pedir-refutador`;
  - con bloqueantes y ninguno sin refutar, es `pedir-writer`, como hoy;
  - sin bloqueantes, no hay ninguna de las dos, como hoy.
- `hoom pedir refutador <slug>` lanza al refutador con el pedido `PedidoRondaRefutador(Rondas+1, slug, ids)`, donde `ids` son los bloqueantes sin refutar en el orden de `hoom finding list`. Su sobre registra `hallazgos` con esos ids, igual que en el piloto.
- El refutador arma su propio pedido: con `-m`, `hoom pedir` sale con el error de uso `el refutador arma su propio pedido: no lleva -m`. Así el orquestador no puede meterle pendientes.
- Sin bloqueantes sin refutar, sale con exit 1 y `no hay hallazgos que bloquean sin pasar por el refutador: nada que pedirle`, sin lanzar.
- El pedido del writer en Review pasa a: `Corregi los hallazgos que bloquean la tarjeta: <ids>. Solo esos: los pendientes quedan abiertos.`

### Contratos de los roles

Cambian `00-orquestador.md`, `04-writer.md` y `09-refutador.md`, idénticos en `internal/agents/assets/agents/` y en `.hoom/agents/`:

- **00, punto nuevo 4c.** Después de una review, el siguiente paso lo dice `hoom siguiente` o el piloto; el orquestador no lo elige. Solo se corrigen los hallazgos que bloquean (`bloquea:` en `hoom finding list --open`); los `pendiente` no se corrigen ni se refutan en la tarjeta. Una ronda es el refutador con `hoom pedir refutador`, si hay bloqueantes sin refutar, y UN writer con todos los bloqueantes; después, verify y la review delta. El arquitecto se relanza solo si la corrección cambia un criterio de aceptación, y eso pide firma nueva. Al tope (`findings.max_rondas`), se escala a Hoom.
- **09.** Recibe los hallazgos de su pedido, que son los que bloquean. Desaparece «Para CADA hallazgo abierto». Se quedan el tope de 2 ciclos y la exigencia de evidencia.
- **04.** En una corrección, corrige solo los hallazgos del pedido. Un pendiente que vea de pasada no se toca: queda abierto.
- La descripción del rol refutador en `agents.Roles` dice «los hallazgos que bloquean» en lugar de «los hallazgos abiertos».

### Documentación

El README documenta `findings.max_rondas` y `hoom pedir refutador`.

## Casos límite y errores esperados

- Una delta sin correcciones, por ejemplo después de integrar la base, cuenta como ronda: la definición es por registros.
- Un bloqueante nuevo que trae la delta es un bloqueante sin refutar: el refutador lo recibe en la ronda siguiente, si no se llegó al tope.
- Al tope sin bloqueantes, no pasa nada: la tarjeta sigue su camino de hoy (Tu aceptación cuando no falta nada más).
- Un sobre de refutador cortado (`stage` `run`) no cuenta como recibido: sus hallazgos siguen sin refutar.
- `hoom pedir refutador` fuera de Review se comporta como cualquier rol no disponible en la columna, con el mensaje de hoy.
- Un proyecto con `autonomia.rondas_de_hallazgos: 3` y sin `findings.max_rondas` tiene tope 3.

## Criterios de aceptación

- CA-747: `hoom.yaml` acepta `findings.max_rondas` con un entero de 1 a 5; rechaza 0, 6 y un valor no entero con `findings: max_rondas tiene que ser un entero de 1 a 5`; una clave desconocida en `findings` nombra `block_on, max_pendientes, max_rondas`.
- CA-748: sin `findings.max_rondas`, el tope es 2, o `autonomia.rondas_de_hallazgos` si es mayor; un `autonomia.rondas_de_hallazgos` mayor que un `findings.max_rondas` declarado es el error `autonomia: rondas_de_hallazgos (<a>) pasa de findings.max_rondas (<m>)`.
- CA-749: `Rondas` de una tarea cuenta sus registros de review `delta` posteriores a su registro `completa` más nuevo; da 0 sin registro `completa` y vuelve a 0 con una review completa nueva.
- CA-750: al tope, la tarjeta está en Review sin acciones de rol, con la razón `se hicieron <n> rondas de hallazgos y quedan <k> que bloquean: te toca` y la acción `hoom finding list --open`; `hoom siguiente` no lanza e imprime `te toca: hoom finding list --open` y la razón.
- CA-751: al tope, `hoom pedir writer` y `hoom pedir refutador` salen con exit 1 y la razón de CA-750 sin lanzar, y el piloto para con esa razón.
- CA-752: con `Rondas >= tope` y sin bloqueantes, la columna y las acciones de la tarjeta son las de hoy.
- CA-753: en Review y bajo el tope, la primera acción es `pedir-refutador` cuando hay bloqueantes sin refutar y `pedir-writer` cuando todos ya pasaron por un refutador; sin bloqueantes no aparece ninguna de las dos.
- CA-754: `hoom pedir refutador <slug>` lanza al refutador con `PedidoRondaRefutador(Rondas+1, slug, ids)`, con los bloqueantes sin refutar en el orden de `hoom finding list`, y su sobre registra esos ids en `hallazgos`; un sobre de refutador en `stage` `run` no cuenta como recibido.
- CA-755: `hoom pedir refutador` sin bloqueantes sin refutar sale con exit 1 y `no hay hallazgos que bloquean sin pasar por el refutador: nada que pedirle` sin lanzar; con `-m` da el error de uso `el refutador arma su propio pedido: no lleva -m`; el error de rol desconocido nombra `arquitecto, test-writer, writer, refutador, reviewer`.
- CA-756: el pedido del writer en Review con bloqueantes es `Corregi los hallazgos que bloquean la tarjeta: <ids>. Solo esos: los pendientes quedan abiertos.`
- CA-757: los contratos 00, 04 y 09 son idénticos en las dos copias y dicen la regla. [verifica: sh -c 'for f in 00-orquestador 04-writer 09-refutador; do cmp -s internal/agents/assets/agents/$f.md .hoom/agents/$f.md || exit 1; done; ! grep -q "Para CADA hallazgo abierto" internal/agents/assets/agents/09-refutador.md && grep -q "hoom pedir refutador" internal/agents/assets/agents/00-orquestador.md && grep -q "findings.max_rondas" internal/agents/assets/agents/00-orquestador.md && grep -q "pendiente" internal/agents/assets/agents/04-writer.md']
- CA-758: la descripción del rol refutador en `agents.Roles` dice «los hallazgos que bloquean» y no «los hallazgos abiertos».
- CA-759: el README documenta el tope y el refutador a mano. [verifica: grep -q "findings.max_rondas" README.md && grep -q "hoom pedir refutador" README.md]

## Decisiones

- **La ronda se cuenta por reviews delta, no por sobres.** Los sobres lanzados a mano no traen `ronda`, y la review delta es lo que cierra una ronda por cualquier camino. Descartado: contar sobres de writer, porque un writer cortado por tiempo contaría dos veces.
- **El tope vive en `findings`, no en `autonomia`.** `autonomia` es lo que el piloto hace solo; el tope es de la tarjeta, por cualquier camino. El 2 por defecto es la «segunda y última ronda» de CAL-4; Hoom lo confirmó el 2026-11-21.
- **El refutador arma su pedido**, como la review. Si aceptara `-m`, el orquestador podría volver a meterle pendientes.
- **A mano, la ronda es refutador y después writer.** El piloto suma el test-writer de regresión. Igualarlos sumaría un run por ronda; Hoom eligió el 2026-11-21 dejar la ronda a mano sin ese run.
- **`hoom agent` no se toca.** Lo usan el piloto y `lanzar`: cerrarlo rompería esos caminos.

## Riesgos y deuda aceptada

- Una delta sin correcciones consume una ronda. Se sale subiendo el tope.
- El orquestador todavía puede lanzar roles con `hoom agent` por fuera de `pedir`: eso lo frena el contrato, no el binario. Cerrarlo sería otra tarjeta.
- Los proyectos con `autonomia.rondas_de_hallazgos` mayor que 2 suben el tope con él, para no cambiar su comportamiento.

## Preguntas para la firma

Ninguna. Las dos que había (el test-writer de regresión en la ronda a mano y el tope por defecto) están contestadas en Decisiones. Contestarlas no es la firma.
