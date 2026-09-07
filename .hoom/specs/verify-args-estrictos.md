# Spec: `hoom verify` estricto con sus argumentos

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start verify-args-estrictos`.
Origen: un reviewer escribió `hoom verify show <id>` por curiosidad, esperando
leer un veredicto. hoom ignoró los dos argumentos posicionales, corrió una
verificación REAL y escribió un veredicto rojo en `.hoom/verdicts/`. Verificado
en un proyecto de prueba: `hoom verify show abc123` sale **0**, imprime VERDE,
deja un artefacto de veredicto en disco y trunca
`.hoom/cache/verify-live.jsonl` (con lo que también pisa la narración de una
corrida legítima en curso). El pedido era una pregunta; hoom respondió con
evidencia falsificada.

## Objetivo

Que `hoom verify` **no ejecute nada que no le pidieron**. Un argumento que hoom
no entiende deja de ser un argumento ignorado y pasa a ser un rechazo: exit 2,
el uso exacto por stderr, cero efectos en disco — ningún veredicto, ningún
evento vivo, ningún trinquete tocado.

La raíz no es un flag mal parseado: es que hoy `verify` acepta en silencio lo
que no comprende y devuelve un veredicto igual. Un veredicto es la unidad de
evidencia de hoom; producir uno a partir de un pedido que hoom no entendió es
la misma clase de fraude que el binario ya combate cuando marca PARCIAL una
corrida `--gate`. Esta spec cierra tres puertas del mismo tipo:

1. **Posicionales ignorados.** `flag.Parse` corta en el primer no-flag y
   `cmdVerify` nunca mira `fs.Args()`: `verify show <id>`, `verify .`,
   `verify --full ayer` corren la verificación completa.
2. **Mensaje ajeno.** Un flag desconocido ya sale 2, pero imprime el uso por
   defecto del paquete `flag` (`Usage of verify:`, con `-full` de un guion),
   que no es el de hoom y enseña una sintaxis que la documentación no usa.
3. **Valores que no significan nada.** `--gate noexiste` deja todos los gates
   en `skipped` y escribe un veredicto VERDE parcial; `--gate ","` no
   selecciona nada y corre TODO como veredicto completo de referencia. La
   misma grieta existe en el Studio, que pasa `body.Gates` directo a
   `verifycmd.Run`.

## No-goals

- Darle subcomandos a `verify`. No nace `verify show`: nace su rechazo. Leer
  veredictos ya es `hoom report`; el estado ya es `hoom status`.
- Adivinar la intención. Descartado enrutar `verify show <id>` hacia `report`:
  interpretar de más es lo que produjo el veredicto falso.
- Cambiar el dialecto de flags de Go: `-full` y `--full` siguen siendo lo
  mismo, `--gate=a,b` y `--gate a,b` también. Solo se rechaza lo **no
  definido**, no lo escrito distinto.
- Endurecer los otros verbos en esta spec. `check`, `status`, `report`,
  `serve`, `cockpit`, `providers` y compañía siguen ignorando posicionales;
  el mecanismo nace reutilizable para saldar esa deuda verbo por verbo.
- Tocar los verbos que SÍ usan posicionales por contrato (`init <dir>`,
  `run/agent/review "<prompt>"`, `finding add "<desc>"`, `task <slug>`,
  `spec approve <ruta>`).
- Validar que la ruta de `--spec` exista al parsear: un spec inexistente ya
  cae en los gates de spec y produce un veredicto ROJO honesto.

## Contratos

**Paquete nuevo `internal/cliargs`** — la disciplina de argumentos, sin
conocimiento de ningún verbo:

```go
// UsageError: hoom no entendió el pedido. Siempre exit 2, nunca efectos.
type UsageError struct {
    Verb   string // "verify"
    Reason string // una línea: qué no se entendió
    Action string // una línea: qué hacer
    Usage  string // bloque de uso exacto del verbo
}
func (e *UsageError) Error() string  // "hoom <verb>: <reason>\n\n<usage>\n<action>"
func (e *UsageError) ExitCode() int  // 2, invariante

// ErrHelp: -h/--help. No es error: uso por stdout y exit 0.
var ErrHelp = errors.New("uso solicitado")

// Strict parsea con ContinueOnError y la salida del paquete flag silenciada,
// y devuelve *UsageError ante flag no definido, flag sin valor, o CUALQUIER
// argumento posicional (fs.NArg() > 0).
func Strict(fs *flag.FlagSet, args []string, verb, usage string) error
```

**`internal/verifycmd`**:

```go
// UsageText es el bloque de uso EXACTO, fuente única del texto.
const UsageText = `...`

// ParseArgs es puro: no lee disco, no toca el entorno, no tiene efectos.
func ParseArgs(args []string) (Options, error) // *cliargs.UsageError o ErrHelp

// Run valida Options contra el manifiesto ANTES de abrir el escritor de
// eventos vivos y antes de correr un solo gate; devuelve *cliargs.UsageError.
func Run(m *manifest.Manifest, opt Options) (*verdict.Verdict, string, error)
```

Reglas de `ParseArgs` (etapa sintáctica, sin entorno):

- `fs.NArg() > 0` → `UsageError` nombrando el **primer** token no reconocido.
- Flag no definido o flag definido sin valor → `UsageError` propio de hoom.
- Un flag escrito con valor vacío (`--gate ""`, `--spec ""`) → `UsageError`:
  escribir un flag y no darle valor es un typo, no el default.
- `--gate` se normaliza con trim y descarte de vacíos; si la selección
  resuelve a CERO nombres (`","`, `" , "`) → `UsageError`. Con al menos un
  nombre válido, comas sobrantes y duplicados se aceptan (selección
  inequívoca).

Reglas de `Run` (etapa semántica, ya con manifiesto):

- Todo nombre de `Options.Gates` debe existir en `m.Gates`. Uno desconocido →
  `UsageError` que nombra el desconocido y **lista los gates de este
  proyecto**, antes de `live.NewWriter` y sin escribir veredicto.
- Los gates sintéticos (`spec_lint`, `spec_trace`, `spec_approved`, `ratchet`)
  no son seleccionables por `--gate`: los gobiernan `--spec` y `--full`.
- Vale para las dos puertas: el CLI y el `POST /api/verify` del Studio entran
  por la misma función, así que la validación no puede vivir en `main`.

**`cmd/hoom/main.go`**:

- `cmdVerify` llama `ParseArgs` **antes** de `manifest.Load`: al pedido mal
  escrito se le contesta sin leer el proyecto.
- `main` mapea `*cliargs.UsageError` (con `errors.As`) a stderr + `os.Exit(2)`;
  todo otro error sigue en exit 1.
- El bloque `Flags de verify:` del `usage` global ES `verifycmd.UsageText`
  (misma constante, concatenada): `hoom help` y `hoom verify --help` no pueden
  divergir.

**`internal/servecmd`**: `POST /api/verify` mapea `*cliargs.UsageError` a HTTP
400 con la razón en el cuerpo; no escribe veredicto ni eventos.

**Texto de uso exacto** (en Go, sin acentos, como el resto del binario):

```
Uso: hoom verify [--full] [--gate a,b] [--spec <ruta>] [--json]

  --full          Ignora el scoping por diff (corrida completa, ej. nocturna)
  --gate a,b      Ejecuta solo esos gates (el resto queda 'skipped'); el
                  veredicto queda PARCIAL: diagnostico, nunca referencia
                  de 'hoom check' ni de 'hoom task done'
  --spec <ruta>   Asocia el veredicto a un spec y suma los gates spec_lint,
                  spec_trace y spec_approved
  --json          Emite el veredicto como JSON en stdout (para agentes)

'hoom verify' no tiene subcomandos ni argumentos posicionales.
Accion: para leer veredictos ya emitidos usa 'hoom report'; para el estado
del proyecto, 'hoom status'.
```

Primera línea del rechazo, según el caso:

```
hoom verify: argumento posicional no reconocido: "show"
hoom verify: flag desconocido: --bogus
hoom verify: --gate necesita un valor
hoom verify: --gate vacio: no selecciona ningun gate
hoom verify: gate desconocido "noexiste"; este proyecto declara: static, test
```

## Casos límite y errores esperados

- `hoom verify` sin argumentos: intacto.
- `hoom verify --` (terminador solo): NArg 0 → corrida normal.
- `hoom verify -- show`: exit 2. `--` no habilita operandos, porque `verify`
  no tiene operandos.
- `hoom verify show --full` y `hoom verify --full show`: ambos exit 2; el
  mensaje nombra el primer token no reconocido (`show` en los dos).
- `hoom verify .` (calco de `hoom init .`): exit 2. La asimetría entre verbos
  es real y el mensaje la resuelve en una línea.
- `-full`, `--full`, `--gate=test`: siguen siendo válidos.
- `--gate test,test` y `--gate "test,"`: válidos (selección inequívoca).
- `--gate spec_lint` o `--gate ratchet`: exit 2 con la lista de gates del
  proyecto. Cambio de conducta consciente: hoy no seleccionan nada y dejan
  todo en `skipped`.
- `--gate noexiste` desde el Studio: HTTP 400, sin veredicto ni eventos.
- Proyecto sin `hoom.yaml` + `hoom verify show x`: gana el error de
  ARGUMENTO, no el de manifiesto.
- `--spec` con ruta inexistente: sin cambios (gates de spec → veredicto ROJO).
- Veredicto rojo legítimo: sigue saliendo 1. El 2 no lo pisa nunca.
- `hoom verify -h` / `--help`: uso exacto por stdout, exit 0, cero artefactos.

## Criterios de aceptación

- CA-217: `hoom verify show <id>` sale 2 y no deja rastro: ningún archivo
  nuevo en `.hoom/verdicts/` y `.hoom/cache/verify-live.jsonl` sin crear ni
  truncar (si existía de una corrida previa, queda byte por byte igual).
- CA-218: ese rechazo imprime por stderr el token ofensor (`"show"`) y el
  bloque `verifycmd.UsageText` completo, incluida la línea que declara que
  `verify` no tiene subcomandos.
- CA-219: un flag no definido (`--bogus` y `-bogus`) sale 2 con el bloque de
  uso de hoom; la cadena `Usage of verify:` del paquete `flag` no aparece
  jamás en la salida.
- CA-220: un flag definido sin valor (`hoom verify --gate` al final de la
  línea) sale 2 con el mensaje de hoom, no con el del paquete `flag`.
- CA-221: un flag escrito con valor vacío o que resuelve a nada (`--gate ""`,
  `--gate ","`, `--spec ""`) sale 2; en particular `--gate ","` nunca vuelve a
  producir una corrida completa de referencia.
- CA-222: `--gate <nombre-inexistente>` es rechazado nombrando el nombre
  desconocido y listando los gates que declara el proyecto, sin escribir
  veredicto ni emitir eventos vivos.
- CA-223: ese mismo rechazo nace en `verifycmd.Run`, así que
  `POST /api/verify` del Studio con un gate desconocido responde 400 con la
  razón y no escribe veredicto: una sola validación para las dos puertas.
- CA-224: `hoom verify --help` y `hoom verify -h` imprimen el uso exacto por
  stdout, salen 0 y no dejan artefactos.
- CA-225: regresión de lo válido — `verify`, `--full`, `--gate <valido>`,
  `--gate=<valido>`, `-full`, `--spec <ruta>`, `--json` y `--` sin operandos
  producen exactamente el mismo veredicto y el mismo exit code que antes.
- CA-226: el bloque `Flags de verify:` del uso global de `hoom` y el que imprime `hoom verify --help` son la MISMA constante: no existe un segundo texto que pueda desincronizarse. [verifica: sh -c "go run ./cmd/hoom verify --help | grep -q 'no tiene subcomandos' && go run ./cmd/hoom help | grep -q 'no tiene subcomandos'"]
- CA-227: disciplina de exit codes — argumento no entendido = 2 y cero
  artefactos; veredicto rojo con argumentos válidos = 1 y veredicto escrito.
  Los dos caminos se distinguen en el mismo test.

## Decisiones de diseño y alternativas descartadas

- **Exit 2 = "no entendí el pedido"; exit 1 = "entendí y salió rojo".** La
  convención ya existe en el binario (comando desconocido sale 2); esta spec
  la baja del comando al argumento. Sin esa separación, un CI no puede
  distinguir "el código está roto" de "el script está mal escrito".
- **`ContinueOnError` con la salida de `flag` silenciada, en vez de
  `ExitOnError`.** Con `ExitOnError` el proceso muere dentro del paquete
  `flag`: hoom no es dueño del mensaje y el parseo es intesteable. Es la razón
  técnica de que el mensaje de hoy sea ajeno.
- **Dos etapas, sintáctica y semántica.** Lo que se puede juzgar sin leer el
  disco se juzga sin leer el disco: así `verify show x` responde lo mismo en
  un proyecto sin `hoom.yaml`. Los nombres de gate necesitan el manifiesto y
  por eso se validan donde el manifiesto ya está.
- **La validación de gates vive en `verifycmd.Run`, no en `main`.** El paquete
  existe para que el verbo y el Studio ejecuten la MISMA función; validar solo
  en el CLI le dejaría el bug al Studio. Ese es literalmente el segundo
  cerebro que el paquete se prohíbe.
- **Rechazar en vez de interpretar.** Alternativa descartada: alias de
  cortesía (`verify show` → `report show`). Un harness que adivina intención
  es un harness que puede adivinar mal en silencio; hoom prefiere no entender
  a entender mal.
- **`--` no habilita operandos.** Alternativa descartada: aceptar posicionales
  después del terminador. `verify` no tiene operandos en ninguna sintaxis.
- **La selección vacía de `--gate` se rechaza, la selección redundante no.**
  Cero gates es ambiguo (¿ninguno o todos?) y hoy resuelve al peor de los dos
  (todos, y como veredicto de referencia). Comas de más son inequívocas.
- **`-h/--help` sale 0.** Pedir ayuda no es un error; reservar el 2 para el
  error real mantiene el código de salida informativo.
- **Una sola constante de texto, concatenada en el uso global.** Dos textos
  que se juran iguales se separan; una constante compartida no puede.
- **`cliargs` nace genérico aunque hoy lo use un solo verbo.** El resto de la
  deuda se salda copiando una línea por verbo, no repensando el mecanismo.

## Riesgos y deuda aceptada

- **Rompe pipelines que hoy pasan basura y siguen en verde**: `hoom verify .`,
  `hoom verify $DIR` copiado de `hoom init`, o un argumento sobrante de un
  script viejo pasan de exit 0 a exit 2. Es exactamente el objetivo, pero es
  un cambio de conducta observable: va anunciado en el README.
- **`--gate spec_lint` deja de ser un no-op silencioso** y pasa a ser un
  error. Nadie debería depender de eso; si alguien lo hace, su corrida hoy no
  verifica nada.
- **La asimetría entre verbos queda más visible**: `init` toma un directorio
  posicional y `verify` no toma nada. Se mitiga con el mensaje, no con el
  diseño; unificar es trabajo de la deuda declarada.
- **Los otros verbos siguen flojos** hasta que se les aplique `cliargs`:
  `hoom check show <id>` sigue corriendo un check. Deuda aceptada y nombrada.
- **`Strict` no se le puede aplicar tal cual a `run`, `agent`, `review` ni
  `finding add`**, que usan los posicionales como prompt o descripción. El
  riesgo es que alguien lo copie sin mirar; se documenta en el doc del
  paquete, que es donde se va a leer.
- **Validar dentro de `Run` acopla `Options` al manifiesto**: un caller que
  arme `Options` a mano (tests incluidos) con un gate inventado empieza a
  fallar. Se acepta: es la garantía que se busca, y falla ruidosamente.
- **El uso vive en Go sin acentos** mientras el spec y el README los usan.
  Divergencia estética conocida, coherente con el resto del binario.
