# Spec: el test-writer en un árbol ciego

Estado: BORRADOR — pendiente de aprobación humana.
Trabajo en el worktree `Spec-D-test-writer-en-worktree-disperso`
(rama `hoomdev/Spec-D-test-writer-en-worktree-disperso`).
Origen: Spec D de la evolución "HoomAI Agent Evolution". Paga la deuda que la
Spec B declaró con todas las letras — *"el gate no ve lecturas: la
anti-circularidad del test-writer sigue siendo una regla de prompt hasta la
spec de aislamiento (worktree disperso)"* — e implementa §15 del documento por
la vía que ahí quedó sin elegir.

## Objetivo

El contrato del test-writer abre con una regla en mayúsculas: *PROHIBIDO leer
el código del Writer, el diff, o cualquier implementación del cambio.* Es la
garantía más importante del harness — un test escrito mirando la
implementación no prueba el spec, prueba lo que el código ya hace — y hoy es
lo único del rol que nadie verifica. El gate de scope que trajo la Spec B
fotografía el árbol antes y después y responde qué ESCRIBIÓ el rol. Ninguna
foto responde qué LEYÓ.

Esta spec deja de intentar verificar la lectura y elimina su posibilidad. El
sobre arma para el test-writer un **árbol ciego**: un worktree disperso
(`git sparse-checkout` en modo `--no-cone`) de UN commit que contiene
exactamente lo que el rol puede leer — el spec, los tests, los archivos con
los que el perfil reconoce el stack — y ni un archivo de implementación. El
rol no obedece la regla de oro: la habita.

Tres cosas dejan de ser narrativa:

1. **La implementación no está.** No hay que confiar en que el modelo no la
   abra: `internal/auth/reset.go` no existe en el disco donde corre. Su test
   vecino, `internal/auth/reset_test.go`, sí. Lo comprobamos a mano: git
   separa dos archivos del mismo directorio sin despeinarse.
2. **El aislamiento se verifica antes y después.** hoom congela, al abrir el
   árbol, la lista de rutas que git dejó afuera — los *testigos* — y las
   busca en el disco dos veces: si alguna aparece después del run, alguien
   deshizo la venda (`sparse-checkout disable`, o escribiendo el archivo a
   mano) y eso es una violación que corta antes de emitir veredicto.
3. **El árbol ciego es una cuarentena.** El rol escribe ahí, no en el árbol
   real. Solo se trasplantan las rutas que el gate de scope aprobó: por
   primera vez, un árbol certificable no recibe ni una escritura que no haya
   pasado por el gate. Lo que viola el scope se queda en la cuarentena, que
   queda en disco para que el humano la mire.

El árbol ciego es además **reproducible**: es una función del commit y de los
globs del rol. Cualquiera puede rearmarlo y comprobar qué vio el test-writer
cuando escribió esos tests. El commit queda registrado en el sidecar del run.

Y hay una simetría que cae sola, sin configuración nueva: **el rol lee
exactamente donde puede escribir.** Los globs de la forma `tests` que la Spec
B ya usa como política de escritura viajan TAL CUAL a `git sparse-checkout`
—verificado— así que un proyecto que declaró `agents.test-writer.write.allow`
en `hoom.yaml` para arreglar su layout arregla, con la misma línea, lo que el
rol ve.

## No-goals

- **Sandbox de proceso, de red o de comandos.** El árbol ciego cambia lo que
  el rol VE por defecto, no lo que podría hacer si decide salirse: con una
  shell a mano, `git show HEAD:internal/auth/reset.go` sigue devolviendo la
  implementación, porque el object DB es del repo y el worktree lo comparte.
  Se declara en riesgos y no se disfraza. La cerradura dura es un sandbox por
  provider: spec propia.
- **Una intención nueva en los providers del tipo "escribe archivos pero no
  ejecuta comandos"** (que en Claude sería negar `Bash` y cerraría el atajo
  del punto anterior). Es trabajo del vocabulario de providers, que la Spec C
  acaba de rehacer; entra en su propia spec y no se mezcla con el aislamiento.
- **Aislar otros roles.** El characterizer comparte la forma `tests` y su
  trabajo es justamente fijar el comportamiento del código legacy: cegarlo
  sería romperlo. El reviewer necesita el diff. Por eso el aislamiento NO se
  deduce de la forma de scope: es un campo propio del rol, y hoy lo tiene uno
  solo.
- **Un verbo `hoom isolate` para uso interactivo** (abrir un árbol ciego a
  mano y trabajar ahí desde la cabina). Es un uso legítimo y probablemente la
  continuación natural; acá el aislamiento vive dentro del sobre, que es
  quien corre roles. Spec propia si el uso lo pide.
- **Bucle test-writer → writer → verify.** Sigue siendo no-goal, igual que en
  la Spec B: el sobre corre una vez.
- **Editar los 10 contratos de `.hoom/agents/`.** El texto del test-writer se
  consume tal cual. El sobre AGREGA un párrafo al system prompt en modo ciego
  (el rol necesita saber por qué faltan archivos, o gastará turnos
  buscándolos), pero no toca el archivo: el mismo contrato sigue generando el
  subagente nativo, que no corre ciego.
- **Cone mode, clones parciales o superficiales.** El modo `--no-cone` es el
  único que separa `x_test.go` de `x.go` en el mismo directorio, que es
  exactamente el caso de Go. Un clon aparte sería otra copia del repo por run.
- **Rama propia para el árbol ciego.** Se arma con `--detach`: no hay rama que
  mergear, y el resultado viaja por trasplante de archivos, no por git.
- **Mostrar cuarentenas en `hoom status` o en el Studio.** El sobre las
  nombra en su salida y en su JSON. La UI es otra spec.

## Contratos

### Lo que se verificó a mano contra git 2.50.1 (macOS)

Antes de escribir nada, sobre un repo de laboratorio con el layout de este
proyecto (`internal/auth/auth.go` + `internal/auth/auth_test.go`) y sobre este
mismo repo:

- `git worktree add --no-checkout --detach <dir> HEAD` +
  `git sparse-checkout set --no-cone <patrones>` + `git checkout` deja en
  disco `internal/auth/auth_test.go` y NO `internal/auth/auth.go`. La
  configuración vive en `.git/worktrees/<nombre>/info/sparse-checkout`: es por
  worktree y el checkout principal no se entera.
- Los globs de la forma `tests` de la Spec B funcionan TAL CUAL como patrones
  de sparse-checkout: `.hoom/**`, `tests/**`, `**/*_test.go` (matchea en la
  raíz y anidado), y los negativos `!.hoom/verdicts/**` excluyen dentro de un
  include previo si van DESPUÉS.
- Un patrón sin `/` matchea a cualquier profundidad: `go.mod` trajo
  `vendor/gopkg.in/yaml.v3/go.mod`. Los archivos base se anclan con `/`.
- En el árbol ciego, `git status --porcelain`, `git diff --name-only HEAD` y
  `git diff --name-only <base>...HEAD` salen VACÍOS: los archivos ausentes
  llevan el bit `SKIP_WORKTREE` y git no los reporta como borrados. `gitx.Touched`
  funciona ahí sin cambios.
- `git ls-files -t` marca con `S` exactamente las rutas que git dejó fuera:
  git mismo calcula el conjunto de testigos, así que hoom no reimplementa el
  matcheo de globs y no puede discrepar con él.
- **Si el agente escribe la implementación a mano, git se queda callado**:
  al refrescar el índice, git limpia el bit `SKIP_WORKTREE` del archivo que
  ahora existe (`S` → `H`) y, como el contenido coincide con HEAD,
  `git status` no dice nada. Por eso el chequeo de aislamiento es un `os.Stat`
  sobre el disco y no una consulta a git.
- `git checkout -- internal/auth/auth.go` sobre una ruta excluida falla con
  `pathspec ... did not match any file(s) known to git`: el atajo obvio para
  restaurar un archivo suelto ya está cerrado por git.
- `git sparse-checkout disable` restaura todo el árbol. Es el camino real de
  evasión y es el que el chequeo post-run caza.
- `git worktree remove` se niega con archivos sin seguimiento (`use --force`).
  La cuarentena SIEMPRE tiene archivos sin seguimiento —los tests nuevos—, así
  que cerrarla es siempre `--force`, y solo después del trasplante.
- Un árbol ciego anida bien dentro del worktree de una tarea, al HEAD de su
  rama, y el `git status` de la tarea sigue limpio si `.hoom/.gitignore`
  esconde `isolated/`.
- Costo sobre este repo (189 archivos rastreados, `vendor/` incluido): **0,11 s**
  y 114 archivos en el árbol ciego. `internal/agentcmd/` quedó con sus cuatro
  `_test.go` y sin `agentcmd.go` ni `scope.go`.
- El object DB es compartido: `git show HEAD:internal/auth/auth.go` devolvió la
  implementación desde adentro del árbol ciego. Es el agujero declarado.

### Paquete `internal/agents`

```go
type Role struct {
    // ...campos actuales...
    Isolated bool `json:"isolated"` // corre en un arbol ciego: sin implementacion en disco
}
```

`test-writer` → `true`. Los otros nueve → `false`, el characterizer incluido y
a propósito. El aislamiento no se deduce de `Scope`: dos roles comparten la
forma `tests` y solo uno escribe sin mirar.

### Paquete `internal/profiles`

```go
// Markers lists the files a profile (and everything it extends) uses to
// RECOGNIZE the stack. They are exactly the files a blind role needs to know
// WHAT it is writing tests for: go.mod, composer.json, settings.gradle.kts.
func Markers(profile string) ([]string, error)
```

`go` → `go.mod`; `laravel` → `composer.json`, `artisan`; `kmp-compose` →
los suyos más los heredados de `kmp`. Perfil desconocido → el mismo error que
`Resolve`, nombrando los disponibles.

### Paquete nuevo `internal/isolate`

```go
// Tree is a blind worktree: a sparse checkout of ONE commit holding exactly
// what a role may read.
type Tree struct {
    Dir      string   // .hoom/isolated/<rol>_<id>
    Commit   string   // sha del commit desde el que se armo
    Patterns []string // los patrones que viajaron a git, en orden
    Hidden   []string // rutas rastreadas que git dejo FUERA del arbol: los testigos
}

// Patterns translates a role's write policy into sparse-checkout patterns.
// allow and deny travel verbatim (hoom's glob syntax and git's agree on the
// forms the roles use); markers are anchored with a leading slash; the
// evidence that QUOTES code never travels.
func Patterns(allow, deny, markers []string) []string

// Open builds the blind tree at parent/.hoom/isolated/<name> from parent's
// HEAD and freezes Hidden. It never touches the parent's checkout.
func Open(parent, name string, patterns []string) (*Tree, error)

// Breaches lists the witnesses that are back on disk. Empty = the isolation
// held. It stats the filesystem instead of asking git, because git goes
// quiet the moment the file exists again.
func (t *Tree) Breaches() []string

// Apply transplants paths from the blind tree into dest: it creates,
// overwrites and deletes exactly the given list, and refuses any path that
// would land outside dest.
func (t *Tree) Apply(dest string, paths []string) (applied, removed []string, err error)

// Close removes the worktree (always --force: a blind tree that did its job
// is full of untracked tests).
func (t *Tree) Close() error
```

`Patterns` produce, en este orden: los `allow` tal cual, los `markers`
anclados con `/`, `.gitignore` (sin él el árbol ciego ignora las reglas del
proyecto y ensucia el delta con artefactos de build), y al final los negativos
— los `deny` del rol, más `!.hoom/verdicts/**` y `!.hoom/findings/**`.

Esos dos últimos no son una excepción caprichosa: **un veredicto rojo cita la
implementación en la salida del gate y un hallazgo la cita en su descripción.**
La evidencia narra el código, así que no viaja al árbol ciego. Escribir ahí
adentro sí se puede: crear un hallazgo es trabajo legítimo del rol.

### Paquete `internal/runcmd`

```go
type StartOptions struct {
    // ...campos actuales...
    Dir string // directorio de trabajo explicito; "" = se resuelve desde Task
}

type Meta struct {
    // ...campos actuales...
    Isolated     bool   `json:"isolated,omitempty"`      // el run corrio en un arbol ciego
    IsolatedFrom string `json:"isolated_from,omitempty"` // commit desde el que se armo
}
```

`Dir` es lo único que `runcmd` necesita saber del aislamiento: dónde corre.
El sidecar convierte la garantía en un dato durable — *estos tests se
escribieron a ciegas, desde este commit* — en vez de una línea de terminal que
se pierde con el scroll.

### Paquete `internal/agentcmd`

```go
// RuleIsolation: el arbol ciego dejo de serlo. Corta como la manipulacion.
const RuleIsolation = "aislamiento"

// Blind is the evidence of an isolated run.
type Blind struct {
    Restored []string // testigos que volvieron al disco del arbol ciego
    Leaked   []string // rutas que cambiaron en el arbol REAL mientras el rol corria confinado
}

// Gate gains its last argument: blind = nil cuando el rol no corre ciego.
func Gate(dir, base string, role agents.Role, before, after Snapshot, pol Policy, blind *Blind) ScopeResult

type Isolation struct {
    Dir      string   `json:"dir"`
    Commit   string   `json:"commit"`
    Patterns []string `json:"patterns"`
    Hidden   int      `json:"hidden"`   // archivos rastreados fuera del arbol
    Applied  []string `json:"applied"`  // rutas trasplantadas al arbol real
    Removed  []string `json:"removed"`  // rutas que el trasplante borro
    Kept     bool     `json:"kept"`     // la cuarentena quedo en disco
}

type Result struct {
    // ...campos actuales...
    Isolation *Isolation `json:"isolation,omitempty"`
}
```

`reviewcmd` es el único otro llamador de `Gate` y pasa `nil`: el reviewer lee
el diff por definición y jamás corre ciego.

### Los pasos del sobre: cinco, o seis con un rol ciego

1. **spec** — igual que hoy. Con un rol ciego suma una condición: el spec debe
   estar en el commit desde el que se armará el árbol y con el mismo
   contenido. Un spec sin commitear no existe para el rol, y prefiero negarme
   a que el rol trabaje sobre un spec que no vio.
2. **aislar** — solo si `role.Isolated`. `PolicyFor` + `profiles.Markers` →
   `isolate.Patterns` → `isolate.Open`. Se toma además la foto del árbol REAL
   (`Take` sobre el directorio de trabajo) para poder detectar fugas.
3. **run** — igual que hoy, con `Dir` = el árbol ciego y el párrafo agregado
   al system prompt.
4. **scope** — `Take` antes/después sobre el árbol ciego, `Breaches()`, foto
   del árbol real de nuevo, y `Gate(..., &Blind{...})`. Si no hay violaciones
   de aislamiento, se trasplantan las rutas SIN violación.
5. **verify** — en el directorio de trabajo, sobre el árbol ya trasplantado.
6. **check** — igual que hoy.

Cortes: `manipulacion` y `aislamiento` cortan en el paso de scope, sin
trasplantar y sin veredicto. `fuera-de-scope` no corta: verify y check corren
igual, pero **las rutas violadoras no viajan**; se quedan en la cuarentena.

La cuarentena se cierra (`Close`) cuando no quedó nada adentro que el humano
necesite ver: sin violaciones. Con violaciones queda en disco, su ruta se
imprime y también el comando exacto para descartarla.

### El párrafo que el sobre agrega al contrato

En modo ciego, y solo ahí, el `SystemPrompt` es el contrato del rol más:

```
Este arbol es CIEGO: hoom lo armo desde el commit <sha> con git sparse-checkout
y dejo fuera <n> archivos rastreados, entre ellos toda la implementacion. Los
tests que escribas salen del spec, no del codigo, y por eso el codigo no esta.
No falta nada ni el repo esta roto. Buscar la implementacion por otra via
(historia de git, rutas absolutas fuera de este arbol) viola el contrato del
rol y hoom lo trata como tal.
```

No reemplaza al aislamiento: le dice al rol lo que ya es cierto. Un modelo que
no sabe por qué faltan archivos gasta turnos buscándolos o concluye que el
repo está roto.

### Reglas de aislamiento

Se evalúan junto con las universales de la Spec B, con la misma consecuencia:

| Regla | Violación |
|---|---|
| un testigo volvió al disco del árbol ciego | `aislamiento`: el rol devolvió al árbol un archivo que el aislamiento había quitado |
| el árbol REAL cambió durante el run ciego | `aislamiento`: el rol escribió fuera de la cuarentena |

La lista de testigos se congela al abrir el árbol: ensanchar los patrones
después no la achica.

### CLI

Ningún flag nuevo. `hoom agent --role test-writer` aísla porque el rol lo
declara, y no hay `--no-isolate`: un flag para apagar la garantía dentro del
comando que la impone la vuelve decorativa. Aflojar existe y se llama
`hoom run`, que no tiene política y lo dice.

Salida humana con un rol ciego:

```
hoom agent: rol test-writer (claude) en .hoom/worktrees/auth-reset
  [1/6] spec    .hoom/specs/auth-reset.md APROBADO (vigente)
  [2/6] aislar  arbol ciego .hoom/isolated/test-writer_20260905T191200_ab12cd
                desde 9f6aec2 - 75 archivos rastreados quedaron fuera
  [3/6] run     20260905T191201_c93f11 - narracion en .hoom/runs/...
  [4/6] scope   3 archivos tocados, 0 fuera de scope, aislamiento intacto
                trasplantados: internal/auth/reset_test.go, tests/e2e_reset_test.go
  [5/6] verify  ROJO (veredicto 2026-09-05T19-15-00Z_9f3a1c2b)
  [6/6] check   ROJO - el arbol cambio despues del ultimo veredicto verde
hoom agent: NO ENTREGABLE (verify) - veredicto rojo
```

Y con el aislamiento roto:

```
  [4/6] scope   ROJO - 4 archivos tocados, 1 violacion (rol test-writer, scope tests):
                  internal/auth/reset.go (aislamiento): el rol devolvio al arbol un
                  archivo que el aislamiento habia quitado [20260905T191500_a1b2c3]
                cuarentena conservada en .hoom/isolated/test-writer_20260905T191200_ab12cd
                (no se trasplanto nada; para descartarla:
                 git worktree remove --force .hoom/isolated/test-writer_20260905T191200_ab12cd)
hoom agent: NO ENTREGABLE (scope) - el aislamiento se rompio: no se emite veredicto
```

`.hoom/.gitignore` suma `isolated/`; `hoomOwn` y `excludedFromCandidate` suman
`.hoom/isolated/`. El README y el `usage` de `cmd/hoom/main.go` dicen, en la
fila de `hoom agent`, que el test-writer corre en un árbol sin implementación.

## Casos límite y errores esperados

- **Los tests nuevos no compilan en el árbol real**, porque la implementación
  todavía no existe: `verify` cierra ROJO y el sobre `no-entregable`. Es
  correcto y es el contrato del rol ("los tests expresan la INTENCIÓN: si la
  implementación difiere del spec, el test debe fallar"). La salida lo nombra
  como lo que es —el trabajo del test-writer terminó, el del writer empieza—
  en vez de dejarlo como un fracaso sin explicación.
- **El proyecto no es git**: un rol ciego no puede correr. Error antes de
  todo, nombrando `hoom run` como la vía sin garantía. No hay degradación
  silenciosa: un test-writer sin venda no es un test-writer.
- **El repo no tiene commits**: mismo corte, con `git commit` como acción.
- **El spec no está commiteado, o difiere del commit**: corte en `aislar`,
  antes de crear nada, con la acción exacta (`git add <ruta> && git commit`).
- **Ya hay un run activo en el directorio de trabajo**: `ErrBusy` con su id,
  antes de armar el árbol ciego. Un writer y un test-writer sobre la misma
  tarea al mismo tiempo harían que el trasplante pise ediciones en curso; un
  run por tarea sigue siendo la regla.
- **Los patrones no matchean ningún archivo** (layout exótico, `allow`
  declarado que no existe): `Open` falla nombrando los patrones y remitiendo a
  `agents.test-writer.write.allow`. Un árbol ciego vacío no es aislamiento, es
  un rol sin insumos.
- **La cuarentena ya existe** (un run anterior que quedó en rojo con ese
  mismo nombre): error nombrando la ruta y el comando para descartarla.
- **El rol borra un test existente**: el trasplante lo borra también en el
  árbol real, y la línea lo dice con su verbo. Ningún gate determinista
  distingue "borré un test obsoleto" de "debilité la suite" — eso lo sigue
  midiendo el gate de mutación, igual que con el writer.
- **El rol no escribe nada** (no entendió el spec, o el spec no pedía tests):
  delta vacío, scope verde, trasplante vacío, verify corre igual. El sobre no
  maquilla: la línea dice "0 archivos trasplantados".
- **El rol escribe un archivo de implementación NUEVO** (`internal/nuevo/x.go`,
  que no era testigo porque no existía en el commit): es `fuera-de-scope`, no
  viaja, y la cuarentena queda para inspección. El árbol certificable nunca lo
  ve.
- **`sparse-checkout disable` a mitad del run**: todos los testigos vuelven,
  `Breaches()` los nombra (se reportan los primeros y el total, no 75
  renglones), corta y no hay veredicto.
- **La implementación reescrita a mano con el contenido idéntico al de HEAD**:
  git no dice nada y `gitx.Touched` tampoco. `Breaches()` la caza igual porque
  mira el disco. Es el caso que justifica que el chequeo no se apoye en git.
- **`Apply` con una ruta que escapa del destino** (`../`, absoluta, un symlink
  que apunta afuera): se niega y el trasplante entero falla. Copiar fuera del
  árbol de trabajo sería exactamente el agujero que la cuarentena viene a
  cerrar.
- **`Close` falla** (disco, permisos, un proceso con el directorio abierto):
  el sobre lo avisa y sigue; el resultado no depende de haber podido barrer.
- **Un stack donde los tests viven DENTRO del archivo de implementación**
  (Rust con `#[cfg(test)] mod tests`, doctests): el árbol ciego no puede
  separarlos, así que el rol no ve ningún test existente. Puede escribir
  igual —los suyos, en archivos nuevos— pero el aislamiento le cuesta el
  contexto de la suite previa. Se declara; hoy no hay perfil de Rust.
- **`vendor/` y otros directorios de dependencias**: entran o no según los
  globs del rol, y con los defaults casi no entran. No es implementación del
  proyecto y su ausencia no molesta a un rol que no compila.

## Criterios de aceptación

- CA-170: `agents.Role` gana `Isolated`: `test-writer` lo tiene en `true` y
  los otros nueve en `false` —characterizer incluido, pese a compartir la
  forma `tests`—; `Roles()` conserva orden, flags y scopes, y `Lookup` sigue
  resolviendo slug y nombre nativo.
- CA-171: `profiles.Markers`: `go` devuelve `go.mod`; `laravel` devuelve
  `composer.json` y `artisan`; `kmp-compose` devuelve los suyos MÁS los
  heredados de `kmp` por `extends`, sin repetidos; un perfil desconocido
  devuelve error nombrando los disponibles.
- CA-172: `isolate.Patterns`: los `allow` viajan tal cual y en orden; los
  `markers` viajan anclados con `/` inicial (un `go.mod` sin anclar matchearía
  `vendor/.../go.mod`); `.gitignore` siempre viaja; los negativos van al final
  e incluyen los `deny` del rol prefijados con `!` más
  `!.hoom/verdicts/**` y `!.hoom/findings/**`; la salida es determinista.
- CA-173: `isolate.Open` sobre un repo con `internal/x/x.go`, `internal/x/x_test.go`,
  `.hoom/specs/s.md` y `.hoom/verdicts/v.json` deja en disco el `_test.go` y el
  spec, y NO deja el `.go` ni el veredicto; `Tree.Commit` es el sha de HEAD del
  padre; `Tree.Hidden` lista las rutas que git marcó con `S` y no está vacía;
  el checkout del padre queda intacto y su `git status` limpio.
- CA-174: errores de `Open`, cada uno con su acción exacta y sin dejar
  directorio a medias: fuera de un repo git; en un repo sin commits; con un
  destino que ya existe; y con patrones que no matchean ningún archivo.
- CA-175: `Breaches()` devuelve vacío recién abierto el árbol; nombra la ruta
  después de escribir a mano un testigo con contenido IDÉNTICO al de HEAD
  (el caso que `git status` no reporta); nombra las rutas después de un
  `git sparse-checkout disable`; y ensanchar los patrones después de `Open` no
  achica el conjunto de testigos congelado.
- CA-176: `Tree.Apply` crea archivos nuevos (con los directorios intermedios),
  sobrescribe los existentes y borra los que ya no están en el árbol ciego,
  exactamente sobre la lista recibida y sobre nada más; devuelve `applied` y
  `removed` ordenados; y una ruta que caiga fuera del destino (`../x`,
  absoluta) hace fallar el trasplante sin escribir nada.
- CA-177: `Tree.Close` elimina el worktree aunque tenga archivos sin
  seguimiento, y después `git worktree list` ya no lo lista.
- CA-178: `runcmd`: `StartOptions.Dir` manda sobre `Task` al resolver el
  directorio del run; el `Meta` del run guarda ese directorio y, cuando el run
  fue ciego, `Isolated` en `true` y `IsolatedFrom` con el commit.
- CA-179: el sobre se niega a correr un rol ciego fuera de un repo git y en un
  repo sin commits: exit 1, sin run, con `hoom run` nombrado como la vía sin
  garantía.
- CA-180: con `--spec` y un rol ciego, un spec que no existe en HEAD o cuyo
  contenido difiere del commiteado corta en `aislar` con exit 1, sin crear el
  árbol ciego y sin run, con `git add` + `git commit` en el mensaje.
- CA-181: un run activo en el directorio de trabajo corta con `ErrBusy` y su
  id antes de armar el árbol ciego.
- CA-182: el sobre imprime seis pasos con un rol ciego y cinco con los demás;
  la línea `aislar` nombra la ruta de la cuarentena, el commit y la cantidad
  de archivos que quedaron fuera; el `SystemPrompt` del run ciego es el
  contrato del rol MÁS el párrafo del árbol ciego, con el commit y esa misma
  cantidad; el contrato en `.hoom/agents/` no se modifica.
- CA-183: un testigo presente después del run produce una violación
  `aislamiento` que corta en scope: `Stage: "scope"`, exit 1, sin verify, sin
  check, sin trasplante; la cuarentena queda en disco (`Kept: true`) con su
  ruta y el comando para descartarla en la salida; y la violación deja un
  hallazgo `high` con lente `risk` cuyo id vuelve en `Violation.FindingID`.
- CA-184: un cambio en el árbol REAL durante un run ciego produce una
  violación `aislamiento` con su propio `Detail` y el mismo corte; las rutas
  que hoom escribe mientras corre (`.hoom/runs/`, `.hoom/cache/`,
  `.hoom/worktrees/`, `.hoom/isolated/`) no cuentan como fuga.
- CA-185: con una violación `fuera-de-scope` y ninguna de aislamiento, viajan
  solo las rutas SIN violación, la ruta violadora se queda en la cuarentena,
  la cuarentena se conserva, verify y check corren igual y el sobre cierra
  `no-entregable`.
- CA-186: camino limpio con un provider falso que escribe un test: todas las
  rutas del delta viajan al directorio de trabajo (altas, sobrescrituras y
  bajas), la cuarentena se cierra (`Kept: false` y el worktree deja de
  existir), verify se ejecuta en el directorio de trabajo y ve los tests
  trasplantados, y `Stage` llega a `"ok"` cuando verify y check dan verde.
- CA-187: `--json` incluye el bloque `isolation` con `dir`, `commit`,
  `patterns`, `hidden`, `applied`, `removed` y `kept`, y respeta el mismo exit
  que el modo texto; con un rol no aislado el bloque no aparece.
- CA-188: el árbol ciego no ensucia lo que hoom certifica: `.hoom/.gitignore`
  contiene `isolated/`, `.hoom/isolated/**` queda fuera del candidato de
  cambio y de la huella, y abrir una cuarentena no cambia el resultado de
  `hoom check` ni el delta que el gate de scope mide en el árbol real.
- CA-189: compatibilidad. [verifica: go test ./...]
  Los roles no aislados conservan sus cinco pasos y su salida; `hoom review`
  sigue verde pasando `nil` como evidencia de aislamiento; `hoom run`,
  `hoom task`, el Studio y el status no cambian.
- CA-190: `hoom agent -h` y el `usage` documentan el árbol ciego. [verifica: go run ./cmd/hoom agent -h 2>&1 | grep -q -- -role] [verifica: go run ./cmd/hoom help 2>&1 | grep -qi ciego]
- CA-191: E2E opcional: con `HOOM_E2E=1` y `claude` real en PATH,
  `hoom agent --role test-writer --max-turns 2 --budget-usd 1` sobre una copia
  de este repo corre en un árbol ciego, el run cierra con exit 0, `Breaches()`
  queda vacío y todo lo que el rol escribió aparece trasplantado en el árbol
  real; sin la variable el test se omite y jamás es requisito de `go test`.

## Decisiones

- **Se elimina la posibilidad en vez de verificar el hecho.** Un gate post-run
  no puede ver lecturas, y ningún gate futuro va a poder: la lectura no deja
  huella en el árbol. La única forma honesta de convertir la regla de oro en
  algo técnico era que el archivo no esté. Todo lo demás de esta spec existe
  para probar que efectivamente no estuvo.
- **Worktree disperso y no un clon aparte.** Comparte el object DB, cuesta
  0,11 s en este repo, vive donde el humano lo puede mirar y se cierra con un
  comando. Un clon por run sería una copia del repo por cada tanda de tests.
- **`--no-cone` y no cone mode.** Cone mode solo entiende directorios; en Go
  el test y su implementación son vecinos en el mismo directorio, que es
  justo el caso que hay que separar.
- **Los globs del rol viajan tal cual a git.** Se verificó que las formas que
  los roles usan significan lo mismo en los dos matchers. Traducir habría
  metido una capa donde el escritor y el lector podían discrepar, y una
  discrepancia ahí es un agujero: un archivo legible que no era escribible, o
  al revés.
- **El rol lee donde puede escribir.** No hay una segunda configuración que
  mantener en sincronía. Un proyecto que ya declaró su layout en
  `agents.test-writer.write.allow` no declara nada más.
- **La evidencia que cita código no viaja.** Veredictos y hallazgos son las
  dos rutas de `.hoom/` que contienen implementación textual. Dejarlas entrar
  habría hecho de la anti-circularidad un chiste.
- **El aislamiento es un campo del rol, no de la forma de scope.** El
  characterizer comparte `tests` y necesita leer legacy. Deducirlo de la forma
  habría roto un rol para arreglar otro.
- **El chequeo mira el disco, no git.** Verificado: git se calla cuando el
  archivo vuelve con el contenido de HEAD. Un chequeo apoyado en `git status`
  habría dado verde justo en el caso que importa.
- **Los testigos se congelan al abrir.** Si se recalcularan después del run,
  bastaría con ensanchar los patrones para que la lista de testigos se
  achicara y la violación desapareciera. Una prueba que el sujeto puede
  reescribir no es una prueba.
- **La cuarentena solo entrega lo que el gate aprobó.** Es la consecuencia
  buena de correr en otro árbol: por primera vez, un árbol certificable no
  recibe escrituras que no pasaron por el gate. En el sobre normal el gate
  llega después del hecho; acá llega antes.
- **El árbol ciego se arma desde un commit, no desde el árbol de trabajo.**
  Así es reproducible: cualquiera puede rearmarlo con el sha y los patrones y
  ver exactamente lo que el rol vio. Copiar archivos sin commitear habría
  hecho del contenido una decisión de hoom en vez de un hecho de git.
- **Sin `--no-isolate`.** Misma razón que la Spec B dio para no poner
  `--allow-tools` en el sobre: un flag para aflojar la política dentro del
  comando que la impone la vuelve decorativa.
- **hoom le explica al rol por qué falta el código.** El párrafo agregado no
  reemplaza nada: describe lo que ya es cierto. La alternativa —un modelo que
  no sabe por qué el repo parece roto— gasta turnos o inventa.

## Riesgos y deuda aceptada

- **El object DB es compartido y `git show` sigue funcionando.** Verificado
  desde adentro del árbol ciego. El aislamiento cambia el camino por defecto
  —abrir un archivo— por uno que exige un acto deliberado, y no lo cierra.
  Cerrarlo pide un sandbox de proceso, o al menos negar la ejecución de
  comandos a un rol que no ejecuta; las dos cosas son spec propia y están
  declaradas como no-goal acá para no venderlas como resueltas.
- **Un rol con shell puede salir del directorio.** `cd`, rutas absolutas, el
  árbol real a dos niveles de distancia. La regla de fuga caza las
  ESCRITURAS que eso produzca en el árbol real; las lecturas no. Misma deuda
  que el punto anterior y el mismo dueño.
- **El delta no atribuye autoría, y ahora en dos árboles.** Si algo externo
  toca el árbol real mientras corre un run ciego, se reporta como fuga. Un run
  activo por directorio y el uso en worktree lo hacen improbable, y el sobre
  se niega a arrancar con un run activo en el árbol de trabajo.
- **El trasplante es de hoom y no pasa por git.** Copia archivos: no hay
  merge, no hay resolución de conflictos, y si el árbol real cambió desde que
  se abrió la cuarentena, la copia gana. Es la contracara de no inventar
  orquestación, y por eso el sobre exige que el árbol de trabajo esté quieto.
- **Stacks donde el test vive dentro del archivo de implementación** quedan
  sin aislamiento útil. Hoy ningún perfil embebido es de ésos; el día que haya
  uno, el rol ciego verá menos contexto del que le sirve y habrá que decidir
  entre menos aislamiento o menos contexto.
- **Un árbol ciego más por run.** Es barato en tiempo pero no es gratis en
  disco ni en inodos, y las cuarentenas con violaciones se conservan a
  propósito. El sobre imprime el comando para barrerlas; nadie las barre solo.
- **Verde imposible en el paso verify durante el TDD normal.** Tests escritos
  antes de la implementación no compilan, así que el sobre cierra
  `no-entregable` casi siempre en su uso previsto. Es honesto y es el
  contrato del rol, pero significa que el exit code del test-writer casi nunca
  es 0: quien automatice alrededor tiene que leer el `Stage`, no solo el exit.
- **22 criterios.** Igual que las specs A, B y C, y por la misma razón: hay un
  mecanismo nuevo (el árbol ciego), su prueba (los testigos), su cierre (el
  trasplante) y su integración con el sobre. Se implementa en un worktree con
  su propio veredicto.
