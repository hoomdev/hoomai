# Spec: quién cierra un hallazgo

Estado: BORRADOR — pendiente de aprobación humana.
Depende de: `base-de-la-review.md` integrada (PR #40).
Tarea: `hoom task start quien-cierra-un-hallazgo`.

## Objetivo

Un hallazgo abierto de severidad alta bloquea `verify` (`findings_open`). Un
hallazgo se cierra con `hoom finding resolve <id> --as corregido|refutado
--evidence "..."`, que escribe `.hoom/findings/<id>.res.json`. Hoy cualquier
rol puede cerrar cualquier hallazgo, y el cierre queda firmado por la
persona:

- `finding.Resolve` usa `gitx.Identity` cuando no hay `--author`, así que
  la resolución lleva el nombre de quien configuró git, no de quien la
  escribió.
- El gate de territorio trata un `.res.json` nuevo como "trabajo legítimo"
  (`agentcmd/scope.go`, `universal`): lo mismo que un `finding add`.
- Una corrida de un rol no le dice a lo que lanza qué rol es: solo pone
  `HOOM_TASK`.

Pasó en la review de `base-de-la-review`: el reviewer (Codex) cerró dos
hallazgos propios con `finding resolve --as refutado` y las resoluciones
quedaron firmadas por el humano. Eran duplicados mal escritos, pero el mismo
camino cierra un hallazgo alto que bloquea, y `verify` pasa a verde. Es la
línea roja del harness: declarar cerrado algo que nadie con autoridad
verificó, y con el nombre de otro.

Hoom no puede saber si un comando que se tipea en la terminal lo tipeó una
persona o un agente interactivo. Pero sí sabe cuándo corre dentro de una
corrida de un rol que él lanzó. Esta spec cierra ese camino: dentro de una
corrida, solo el refutador cierra hallazgos, solo como `refutado`, y queda
firmado por su rol; el gate de territorio hace valer lo mismo aunque el rol
escriba el archivo a mano.

## No-goals

- Distinguir a una persona de un agente que usa su terminal fuera de una
  corrida de hoom: para hoom es la persona. Los contratos piden `--author`.
- Revisar o reescribir las resoluciones que ya existen (incluidas las dos
  del reviewer de `base-de-la-review`): quedan como historia en Git.
- Cambiar quién cierra `corregido`: el Orquestador o una persona, fuera de
  una corrida, como hoy.
- Firmar criptográficamente las resoluciones.

## Contratos

### El entorno de una corrida

Cada corrida con rol pone en el entorno de su provider, siempre (vacío sin
valor, para que nada se herede del padre):

- `HOOM_TASK`, como hoy;
- `HOOM_ROLE`: el slug del rol (`reviewer`, `writer`, `refutador`, ...);
- `HOOM_RUN`: el id de la corrida;
- `HOOM_PROVIDER`: el provider que la corre.

### `hoom finding resolve` dentro de una corrida

Con `HOOM_ROLE` no vacío:

- Un rol que no es `refutador`: se niega sin escribir nada, exit 1:
  `hoom finding: un <rol> no cierra hallazgos: los cierra el refutador (refutado) o una persona`.
- El `refutador` con `--as corregido`: se niega, exit 1:
  `hoom finding: el refutador solo refuta: corregido lo cierra el Orquestador o una persona, con el gate verde`.
- El `refutador` con `--as refutado`: escribe la resolución con
  `author` = `refutador@<provider> (run <id>)`. Un `--author` distinto se
  niega sin escribir nada: `hoom finding: dentro de un run el autor lo pone hoom (refutador@<provider> (run <id>))`.

`Resolution` gana `role` y `run` (omitidos si están vacíos): el rol y la
corrida que la escribieron. Fuera de una corrida quedan vacíos y el resto
es como hoy (`--author` libre, o la identidad de git).

### El gate de territorio

En la corrida de cualquier rol que no sea `refutador`, un
`.hoom/findings/*.res.json` creado durante la corrida es una violación de
tipo manipulación: `un <rol> no cierra hallazgos: los cierra el refutador
(refutado) o una persona`. Vale aunque el rol escriba el archivo sin pasar
por la CLI. En `hoom review` termina `NO ENTREGABLE` por territorio, como
cualquier violación. Un `.json` de hallazgo nuevo (`finding add`) sigue
siendo trabajo legítimo.

### Los contratos de los roles

- `06-reviewer.md`: el reviewer registra hallazgos y nunca los cierra; los
  cierra el refutador (`refutado`) o el Orquestador o una persona
  (`corregido`).
- `09-refutador.md`: refuta con `--as refutado`; dentro de un run el autor
  lo pone hoom, fuera de uno lleva `--author refutador@<cli>`.
- Las copias embebidas (`internal/agents/assets/agents/`) iguales a las de
  `.hoom/agents/`.

## Casos límite y errores esperados

- `HOOM_ROLE` vacío o ausente: `finding resolve` como hoy.
- Un reviewer que escribe a mano un `.res.json` con otro autor: el gate lo
  marca igual (el archivo es nuevo en la corrida y el rol no es refutador).
- Un reviewer que borra o modifica un `.res.json` existente: ya es
  manipulación hoy (evidencia append-only).
- El refutador que escribe a mano un `.res.json` con `corregido`: el gate no
  lo marca (el refutador puede crear resoluciones); la CLI sí lo niega. Queda
  en el diff para quien commitea.
- `hoom run` sin rol: sin `HOOM_ROLE`; nada cambia.

## Criterios de aceptación

- CA-435: el provider de una corrida con rol recibe `HOOM_ROLE`, `HOOM_RUN` y `HOOM_PROVIDER` con su rol, su id y su provider (y `HOOM_TASK` como hoy); un valor heredado del padre nunca se filtra.
- CA-436: con `HOOM_ROLE` de un rol que no es `refutador`, `hoom finding resolve` se niega con `un <rol> no cierra hallazgos: los cierra el refutador (refutado) o una persona`, exit 1, y no escribe `.res.json`; el hallazgo sigue abierto.
- CA-437: con `HOOM_ROLE=refutador`, `--as refutado` escribe la resolución con `author` = `refutador@<provider> (run <id>)`, `role` y `run`; `--as corregido` y un `--author` distinto se niegan sin escribir; sin `HOOM_ROLE`, `resolve` funciona como hoy y la resolución no lleva `role` ni `run`.
- CA-438: en la corrida de un rol que no es `refutador`, un `.hoom/findings/<id>.res.json` creado durante la corrida (por la CLI o a mano) es una violación de manipulación con `un <rol> no cierra hallazgos`; en `hoom review` la review termina `NO ENTREGABLE` por territorio; un hallazgo nuevo sigue siendo legítimo, y en la corrida del refutador crear una resolución no es violación.
- CA-439: los contratos 06 y 09 dicen quién cierra, iguales en `.hoom/agents/` y en la copia embebida. [verifica: grep -q "nunca los cierra" .hoom/agents/06-reviewer.md && grep -q "nunca los cierra" internal/agents/assets/agents/06-reviewer.md && grep -q "solo como refutado" .hoom/agents/09-refutador.md && grep -q "solo como refutado" internal/agents/assets/agents/09-refutador.md]

## Decisiones

- **Solo el refutador, y solo refutado.** El contrato 09 ya dice que el
  refutador tumba hallazgos y que el `corregido` lo habilita el gate verde
  de la corrección, no un rol. Ningún otro rol tiene motivo para cerrar
  uno.
- **El autor dentro de una corrida lo pone hoom.** Es lo único que hoom sabe
  con certeza: qué rol lanzó y con qué id. Aceptar un `--author` ahí sería
  volver a dejar que un rol firme por otro.
- **El gate es la defensa, la CLI es la cortesía.** Un rol con shell puede
  escribir el archivo sin la CLI; el gate de territorio mira el árbol y lo
  marca igual. La negativa de la CLI solo evita el error honesto.
- **Fuera de una corrida, hoom no adivina.** Una sesión interactiva de un
  agente en la terminal de la persona es, para hoom, la persona. Los
  contratos piden `--author`; esta spec no promete más.

## Riesgos y deuda aceptada

- **Agentes interactivos fuera de hoom.** Un agente que corre en la terminal
  de la persona (no como rol de hoom) puede cerrar hallazgos con su nombre.
  Mitigación: los contratos piden `--author`, y la resolución queda en el
  diff de Git. Resolverlo del todo es otra conversación (firmas, o que el
  Orquestador corra siempre como rol).
- **El archivo queda en disco.** El gate marca la violación y la review es
  `NO ENTREGABLE`, pero el `.res.json` sigue en el árbol hasta que alguien
  lo borre: quien commitea lo ve en el diff y en la salida de la review.
