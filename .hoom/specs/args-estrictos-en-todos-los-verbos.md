# Spec: argumentos estrictos en todos los verbos

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start args-estrictos-en-todos-los-verbos`.
Origen: deuda declarada de `.hoom/specs/verify-args-estrictos.md`
(CA-217..CA-227): el paquete `internal/cliargs` quedó aplicado a `verify`, y
"los otros verbos siguen flojos: `hoom check show <id>` sigue corriendo un
check". Los verbos nuevos de la cabina (`item`, `board`, `roles`) ya nacieron
estrictos. Quedan los anteriores, que parsean con
`flag.NewFlagSet(..., flag.ExitOnError)` y descartan el resto de `args`.

## Objetivo

Que ningún verbo de hoom haga algo distinto de lo que se le pidió por un
argumento que no entiende. Mismo contrato que `verify`: un posicional o un
flag desconocido sale con exit 2, un mensaje de uso que nombra el token
inválido entre comillas (`strconv.Quote`), y cero efectos.

## No-goals

- Cambiar lo que hace cada verbo con argumentos válidos.
- Aplicar `Strict` a los posicionales de texto libre: el prompt de `run` y
  `agent`, el pedido de `review` y la descripción de `finding add`. Para esos
  solo se rechazan los flags desconocidos.
- Unificar la forma de los verbos (un `init` que toma un directorio y un
  `verify` que no toma nada siguen distintos).

## Contratos

Inventario de hoy y regla de cada uno. "Ninguno" significa que todo
posicional es un error.

| Verbo | Posicionales válidos | Flags | Hoy |
|---|---|---|---|
| `init` | un directorio opcional | los actuales | descarta extras |
| `report` | ninguno | los actuales | descarta extras |
| `check` | ninguno | los actuales | `check show <id>` corre un check |
| `status` | ninguno | los actuales | descarta extras |
| `cockpit` | ninguno | los actuales | descarta extras |
| `ratchet init` / `ratchet lower` | los de cada subverbo | los actuales | revisar |
| `serve` | ninguno | los actuales | descarta extras |
| `task start|list|done|discard` | el slug donde corresponde | los actuales | revisar extras |
| `spec approve|status` | una ruta | ninguno | descarta extras |
| `context` | ninguno | los actuales | revisar |
| `finding add` | la descripción (texto libre) | los actuales | solo flags |
| `finding resolve|list` | el id donde corresponde | los actuales | revisar |
| `providers`, `agents`, `profiles`, `version` | ninguno | los actuales | revisar |
| `run`, `agent` | el prompt (texto libre) | los actuales | solo flags |
| `review` | el pedido (texto libre) | los actuales | solo flags |
| `hook` | los de su contrato | ninguno | revisar |

La tabla definitiva se completa al implementar, leyendo cada `cmd*` de
`cmd/hoom/main.go`; lo que esta spec fija es la regla, no la lista.

- Exit 2 y mensaje `uso: hoom <verbo> ...` con el token inválido citado.
- `-h` y `--help` se resuelven después de validar el resto: `hoom check
  --help show` sale con 2.
- Un flag repetido con un valor vacío se rechaza en cada ocurrencia, no solo
  en la última (la lección de CA-221).
- Cero efectos: ni `.hoom/cache`, ni veredictos, ni aprobaciones, ni items.

## Casos límite y errores esperados

- `hoom check show <id>`: exit 2, no corre ningún check.
- `hoom status --watch extra`: exit 2.
- `hoom run --bogus "prompt"`: exit 2; `hoom run "texto con --palabras"` sigue
  andando, porque el prompt va después de los flags.
- `hoom spec approve <ruta> extra`: exit 2 sin aprobar nada.
- `hoom task done <slug> extra`: exit 2 sin cerrar nada.

## Criterios de aceptación

- CA-456: cada verbo de la tabla sin posicionales libres rechaza un
  posicional de más con exit 2, nombra el token entre comillas y no escribe
  nada en `.hoom/`. Un test por verbo a nivel binario, como
  `cmd/hoom/*_test.go` de verify estricto.
- CA-457: cada verbo rechaza un flag desconocido con exit 2 y cero efectos,
  incluidos `run`, `agent`, `review` y `finding add`.
- CA-458: `--help` seguido de un argumento inválido sale con 2; `--help` solo
  sale con 0.
- CA-459: `hoom check show <id>` sale con 2 y no escribe en `.hoom/cache`.
- CA-460: un flag repetido con una ocurrencia vacía se rechaza aunque la
  última no lo sea.
- CA-461: con argumentos válidos, la salida de cada verbo es la de hoy (los
  tests existentes de cada verbo siguen verdes sin tocarse).
- CA-462: el README cubre todos los verbos en la sección de argumentos
  estrictos.

## Decisiones

- **Un solo mecanismo.** Todos pasan por `internal/cliargs`, como `verify`.
  No se copia la validación verbo por verbo.
- **Texto libre solo después de los flags.** Es como ya funcionan `run` y
  `agent`: el primer posicional termina los flags.
- **Exit 2 y no 1.** Es el código de uso que fijó verify estricto.

## Riesgos y deuda aceptada

- **Scripts que hoy pasan basura y siguen andando pasan a fallar.** Es el
  objetivo, y va anunciado en el release.
- **Los tests a nivel binario son lentos**: compilan el binario una vez por
  paquete en `TestMain`.
- **Siete criterios para muchos verbos**: el criterio es por regla y el test
  es por verbo.
