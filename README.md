# hoomAI

Harness de verificacion **agnostico de IA y de stack** para desarrollo asistido por agentes.

La idea central: la confianza nunca vive en el modelo. Vive en **gates deterministicos**
(tests, analisis estatico, mutation testing, tests de arquitectura) atados al scope real
derivado de Git, registrados como **veredictos inmutables** que viajan con el proyecto.
La narracion del agente no cuenta; solo cuenta la evidencia.

## Instalacion en un comando

```sh
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/hoomdev/hoomai/main/install.sh | sh
```

```powershell
# Windows (PowerShell)
irm https://raw.githubusercontent.com/hoomdev/hoomai/main/install.ps1 | iex
```

Alternativas:

```sh
go install github.com/hoomdev/hoomai/cmd/hoom@latest   # si tienes Go 1.22+
go build -o hoom ./cmd/hoom                               # desde el repo (vendor incluido, sin red)
```

## Uso en 60 segundos

```sh
cd mi-proyecto
hoom init        # detecta el stack (laravel|kmp|kmp-compose|go) y crea hoom.yaml
hoom verify      # ejecuta los gates y emite un veredicto: ROJO = exit 1
hoom agents      # instala los 8 contratos de agentes y ata AGENTS.md
hoom report      # historial y tendencia por gate
```

## Filosofia

1. **Agnostico de IA**: hoomAI nunca habla con un modelo. Cualquier agente (Claude Code,
   OpenCode, Codex CLI, Gemini CLI, Ollama) se ata al mismo contrato: `hoom verify` antes
   de entregar, veredicto rojo = no se entrega.
2. **Anti-circularidad**: la IA no puede revisar su propio codigo. Por eso el test-writer
   es adversarial (escribe desde el spec, JAMAS ve la implementacion) y el mutation
   testing mide la calidad de los tests, no solo su existencia.
3. **Degradacion declarada, nunca silenciosa**: un gate sin comando aparece en AMARILLO
   en cada veredicto. La red de seguridad incompleta se ve; no se oculta.
4. **Politica estricta**: fail o error en un gate requerido = veredicto ROJO = exit 1.
   Herramienta configurada pero rota = ERROR (fail-closed), tambien rojo.
5. **Veredictos append-only en Git**: `.hoom/verdicts/<timestamp>_<hash>.json`, un archivo
   por corrida, cero conflictos de merge por construccion. El historial viaja con el
   proyecto entre maquinas y CI. Cualquier indice (SQLite futuro) es cache regenerable.

## Comandos

| Comando | Que hace |
|---|---|
| `hoom init` | Detecta el stack, crea `hoom.yaml` con los gates del perfil materializados y `.hoom/` |
| `hoom verify` | Ejecuta los gates y emite un veredicto (rojo = exit 1) |
| `hoom verify --full` | Ignora el scoping por diff (corrida completa, ej. nocturna) |
| `hoom verify --gate test,static` | Ejecuta solo esos gates |
| `hoom report -n 10` | Historial de veredictos + tendencia de pass-rate por gate |
| `hoom report --json` | El mismo historial como JSON en stdout (para agentes y el Studio) |
| `hoom check` | Compara el arbol ACTUAL contra el ultimo veredicto: verde + huella coincidente = OK |
| `hoom check --json` | El mismo check como JSON en stdout, mismo exit code |
| `hoom serve` | HoomAI Studio: dashboard local embebido en el binario (default 127.0.0.1:4666). Lectura libre en loopback; acciones (verify, tareas, aprobar specs, intake) con el token que imprime al arrancar |
| `hoom spec approve <ruta>` | Registra la aprobacion humana del spec atada al SHA-256 de su CONTENIDO (append-only en `.hoom/approvals/`); editarlo despues la invalida |
| `hoom spec status <ruta>` | aprobado / no-aprobado / invalidado; exit 0 solo con aprobacion vigente (gateable por script) |
| `hoom providers` | Detecta que CLIs de IA hay instaladas (claude, opencode, codex, gemini) `--json` |
| `hoom run --provider <p> [--task <slug>] "<prompt>"` | Lanza TU CLI de IA en headless sobre el proyecto o el worktree de la tarea. hoom nunca llama a una API de modelo; la narracion queda en `.hoom/runs/` (local, fuera de la huella y de Git) |
| `hoom hook` | Instala el pre-push de Git que exige `hoom check` antes de integrar |
| `hoom verify --json` | Veredicto como JSON en stdout, para consumo de agentes (in-band) |
| `hoom verify --spec <ruta>` | Suma los gates `spec_lint` y `spec_trace`: cada criterio CA-n del spec debe tener un test que lo referencie |
| `hoom task start <slug>` | Tarea paralela aislada: rama `hoom/<slug>` + worktree propio + sus propios veredictos |
| `hoom task list` | Estado de las tareas activas (verde listo / drift / rojo / sin veredicto) |
| `hoom task list --json` | El mismo estado como JSON en stdout |
| `hoom task done <slug>` | Cierra la tarea SOLO con veredicto verde, huella coincidente y todo commiteado |
| `hoom agents` | Instala los 9 contratos de agentes en `.hoom/agents/` y ata `AGENTS.md` |
| `hoom agents --target all` | Genera ademas los subagentes NATIVOS de Claude Code, OpenCode, Codex y Gemini CLI |
| `hoom profiles` | Lista los perfiles embebidos |

## Perfiles (v1)

- **laravel** - Pest/PHPUnit, PHPStan, Pint, arch tests de Pest, Infection (mutation por
  diff con `--git-diff-base`).
- **kmp** - Gradle allTests, detekt, Konsist, Pitest (parcial declarado: solo target JVM).
- **kmp-compose** - hereda kmp + lint de Compose + metricas del Compose compiler.
- **go** - go test -race (diff por paquetes), go vet, golangci-lint, arch-go, Gremlins.

Los perfiles son defaults: `hoom init` los materializa en `hoom.yaml` y ahi los editas.
`cmd: ""` declara un gate AUSENTE explicitamente (amarillo, nunca oculto). La herencia
(`extends`) permite crear perfiles derivados (ej. un futuro `filament extends laravel`).

## Manifiesto (hoom.yaml)

```yaml
schema: hoom/v1
project: mi-proyecto
profile: laravel
base_branch: main
policy: strict
gates:
  test:
    required: true
    cmd: "vendor/bin/pest --colors=never"
  mutation:
    required: true
    cmd: "vendor/bin/infection --threads=max --no-progress --min-msi=60"
    diff_cmd: "vendor/bin/infection --threads=max --no-progress --git-diff-filter=AM --git-diff-base={base} --min-msi=60"
```

Variables de template: `{base}` (rama base), `{files}` (archivos cambiados),
`{packages}` (paquetes Go derivados de los .go cambiados). Si un `diff_cmd` expande a
vacio, cae al comando completo en lugar de ejecutar una linea rota.

## Congelamiento del candidato (anti-fraude de veredicto)

Cada veredicto congela una **huella SHA-256 del cambio exacto verificado** (commit +
contenido de cada archivo tocado) y el tamano del cambio (+ins/-del). `hoom check`
compara esa huella contra el arbol actual: si un agente verifica A y entrega B, la
huella no coincide y el check es ROJO con la accion exacta a ejecutar. `hoom hook`
lleva esto al pre-push: sin veredicto verde con huella coincidente, no hay push
(`HOOM_SKIP=1 git push` existe como escape consciente y visible, nunca silencioso).
Mas de 400 lineas cambiadas dispara deterministicamente la review con 4 lentes.
Es la idea central de RDD (Receipt Driven Development) sin la ceremonia
criptografica: el modelo de amenaza local es la deriva, no la falsificacion.

## Trazabilidad spec -> test (spec_lint / spec_trace)

Adaptacion hoomAI de la mutacion de specs de SwarmForge, en version deterministica:
el Arquitecto enumera los criterios de aceptacion como CA-1, CA-2, ... y el
test-writer referencia cada CA-n en sus tests. `hoom verify --spec <ruta>` valida
la estructura del spec (spec_lint) y que cada criterio tenga al menos un test que
lo mencione (spec_trace). Un criterio sin test = veredicto rojo con la accion
exacta a ejecutar. El spec y los tests ya no pueden divergir en silencio.

## Tareas paralelas aisladas (hoom task)

Adaptacion del worktree-por-rol de SwarmForge a nuestra unidad de trabajo: una
tarea = una rama `hoom/<slug>` = un worktree bajo `.hoom/worktrees/` (ignorado
por Git) = UN writer = su propio historial de veredictos. Varias tareas corren en
paralelo con aislamiento duro de filesystem, y `hoom task done` solo cierra con
veredicto verde, huella coincidente y todo commiteado; la rama queda lista para
`git merge --no-ff hoom/<slug>`.

La huella (v2) es de CONTENIDO puro: commitear exactamente lo verificado preserva
la huella; cambiar un byte la rompe. Verificar, commitear e integrar sin re-correr
gates es legitimo por construccion.

## Gate de seguridad (Semgrep)

Todos los perfiles incluyen un gate `security` (ausente por defecto; se activa
instalando semgrep y llenando el cmd). Recomendado: reglas `p/default` +
`p/trailofbits`, con `--baseline-commit` para scoping por diff. Las reglas
propias del proyecto viven versionadas en `.hoom/semgrep/`: cuando una review
encuentra un patron peligroso, se convierte en regla (las skills de Trail of
Bits para Claude Code hacen exactamente eso en la capa del agente) y el gate lo
bloquea para siempre. En proyectos con auth/pagos/facturacion: `required: true`.

## Los 8 agentes (.hoom/agents/)

00 Orquestador (padre, nunca escribe codigo) - 01 Arquitecto (produce el spec que ata la
cadena) - 02 Designer (dueño del design system; el design NO entra al verify) - 03 Scout
(exploracion solo-lectura, Ollama-compatible) - 04 Writer (UNICO que edita, uno por
tarea) - 05 Test-writer adversarial (PROHIBIDO ver la implementacion) - 06 Reviewer
(4 lentes deterministicas segun riesgo) - 07 Characterizer (fija el comportamiento
actual de codigo legacy antes de refactorizar).

`verify` NO es un agente: es un comando deterministico.

## Subagentes nativos (multi-CLI)

Los contratos de `.hoom/agents/` son la unica fuente de verdad. Con
`hoom agents --target claude,opencode,codex,gemini` (o `all`) se generan los
subagentes en el formato nativo de cada herramienta:

| Target | Genera | Enforcement duro |
|---|---|---|
| `claude` | `.claude/agents/*.md` | roles de solo lectura sin herramientas de edicion |
| `opencode` | `.opencode/agents/*.md` | orquestador PRIMARY con `edit: deny`; subagentes con permisos por rol |
| `codex` | `.codex/agents/*.toml` | roles de solo lectura con `sandbox_mode = "read-only"` |
| `gemini` | `.gemini/agents/*.md` | tools restringidas a lectura por rol |

Donde la CLI lo soporta, la disciplina del contrato deja de ser texto y se
vuelve imposibilidad tecnica: el scout no puede editar aunque quiera. En
Claude Code, Codex y Gemini el orquestador es tu sesion principal (atada via
AGENTS.md); en OpenCode es un agente primary seleccionable con Tab. Nota:
Antigravity CLI aun no carga subagentes; el target `gemini` sirve a Gemini CLI
(licencias Code Assist) y se agregara `antigravity` cuando Google documente su
formato.

## Publicar un release (mantenedores)

```sh
git tag v0.2.0 && git push origin v0.2.0
```

El workflow de GitHub Actions (goreleaser) compila los binarios para
linux/darwin/windows (amd64+arm64), genera `checksums.txt` y publica el release.
`install.sh` siempre apunta al release mas reciente.

## Roadmap

- Fase 2: `hoom characterize` (characterization tests asistidos sobre el blast radius).
- Cache SQLite regenerable en `.hoom/cache/` para historiales grandes.
- `hoom onboard` (bootstrap de codebase-memory-mcp + Engram en un proyecto).
- Perfil `filament extends laravel`. Homebrew tap / Scoop / AUR.

 
 # Tutorial hoomAI Explicativo
### De la entrevista con el cliente a tu primera funcionalidad verificada

Este tutorial asume que nunca usaste hoomAI. Al terminarlo vas a tener: el
harness instalado, tu CLI de IA configurada con un equipo de agentes
especializados, un proyecto con todo el contexto del cliente adentro, y tu
primera funcionalidad construida por agentes con un veredicto verde que lo
demuestra.

**Qué necesitás antes de empezar:**
- Git instalado y una cuenta de GitHub (o similar).
- UNA CLI de IA para programar: Claude Code, OpenCode, Codex CLI o Gemini CLI
  (cualquiera sirve; hoomAI habla con las cuatro).
- Las herramientas de tu stack (PHP/Composer, Go, o Gradle según el proyecto).

**La idea en una frase:** hoomAI no confía en lo que la IA *dice* que hizo;
confía en gates determinísticos (tests, análisis, compilación) que emiten un
**veredicto** con una **huella** del código exacto verificado. Verde = se
entrega. Rojo = no se entrega. Sin excepciones.

---

## Paso 1 — Instalar hoom (1 minuto)

```sh
# Linux / macOS
curl -fsSL https://raw.githubusercontent.com/hoomdev/hoomai/main/install.sh | sh

# Windows (PowerShell)
irm https://raw.githubusercontent.com/hoomdev/hoomai/main/install.ps1 | iex
```

Verificá que quedó:

```sh
hoom version     # debe responder: hoom 0.5.0 (o superior)
hoom profiles    # lista los stacks soportados: laravel, kmp, kmp-compose, go
```

---

## Paso 2 — La entrevista con el cliente → el documento fuente

Antes de escribir código, escribí lo que el cliente quiere. En la entrevista
cubrí como mínimo estas seis cosas (son las que el harness va a necesitar
después):

1. **Qué es el sistema y para quién** — en las palabras del cliente.
2. **Los módulos o partes** — "necesito una tienda, un panel de admin, reportes..."
3. **Los roles** — quiénes lo usan y qué puede hacer cada uno.
4. **Las reglas de negocio innegociables** — "los precios cambian por región",
   "el contador cierra caja todos los días". Estas son ORO: anotalas textuales.
5. **Lo que NO quiere** (o no todavía) — igual de importante que lo que sí.
6. **Prioridades** — si mañana solo pudiera tener una parte, ¿cuál?

Volcá todo en un documento. Puede ser un SRS formal, un Google Doc, o notas de
la reunión — el formato no importa, la fidelidad sí: **escribí lo que el
cliente dijo, no lo que vos ya estás diseñando en tu cabeza**. Guardalo como
markdown si podés (`srs-mi-proyecto.md`); si es Word/PDF, convertilo:

```sh
pandoc -t markdown srs.docx -o srs-mi-proyecto.md
```

---

## Paso 3 — Crear el proyecto

Creá el esqueleto con la herramienta normal de tu stack y dejalo bajo Git:

```sh
# Ejemplo Laravel:
laravel new mi-proyecto && cd mi-proyecto

# Ejemplo Go:
mkdir mi-proyecto && cd mi-proyecto && go mod init github.com/mi-usuario/mi-proyecto

git init -b main
git add -A
git commit -m "esqueleto inicial"
```

---

## Paso 4 — Inicializar el harness

```sh
hoom init
```

Esto detecta tu stack y crea:
- **`hoom.yaml`** — el manifiesto: qué gates (verificaciones) exige tu proyecto.
- **`.hoom/intake/`** — acá van los documentos del cliente.
- **`.hoom/specs/`** — acá van a vivir la visión y los specs por tarea.
- **`.hoom/verdicts/`** — acá quedan los veredictos (viajan en Git).

Ahora abrí `hoom.yaml` y hacelo honesto. La regla de oro:

> **Lo que está configurado debe poder pasar. Lo que todavía no adoptás se
> declara vacío (`cmd: ""`) y aparece en AMARILLO — visible, nunca oculto.
> NUNCA dejes el comando de una herramienta que no tenés instalada: eso es
> veredicto ROJO garantizado.**

Instalá las herramientas de los gates que sí vas a exigir. Ejemplo Laravel:

```sh
composer require --dev pestphp/pest phpstan/phpstan laravel/pint infection/infection
```

---

## Paso 5 — Instalar tu equipo de agentes

Un solo comando instala los contratos Y los convierte en subagentes nativos de
tu CLI:

```sh
hoom agents --target claude      # si usás Claude Code
hoom agents --target opencode    # si usás OpenCode
hoom agents --target codex       # si usás Codex CLI
hoom agents --target gemini      # si usás Gemini CLI (Code Assist)
hoom agents --target all         # los cuatro a la vez (equipos mixtos)
```

Esto hace tres cosas:
1. Instala los **9 contratos** en `.hoom/agents/` (la fuente de verdad).
2. Ata el contrato de verificación a `AGENTS.md`.
3. Genera los subagentes en el formato nativo de tu CLI — y donde la
   herramienta lo permite, las reglas se vuelven **imposibilidad técnica**:
   el scout no puede editar aunque el modelo quiera (sin herramientas de
   edición en Claude/Gemini, `sandbox read-only` en Codex, `edit: deny` en
   OpenCode).

Los 9 roles, en una línea cada uno:

| # | Agente | Qué hace |
|---|--------|----------|
| 00 | Orquestador | El único que habla con vos; delega, nunca programa |
| 01 | Arquitecto | Escribe el spec de cada tarea, con criterios CA-1, CA-2... |
| 02 | Designer | Cuida el design system; no dibuja |
| 03 | Scout | Explora el código, solo lectura |
| 04 | Writer | El ÚNICO que edita código |
| 05 | Test-writer | Escribe tests desde el spec, referenciando cada CA-n |
| 06 | Reviewer | Revisa con 4 lentes según el riesgo |
| 07 | Characterizer | Fija el comportamiento de código legacy antes de tocarlo |
| 08 | Analista | Convierte tu documento de entrevista en visión + backlog |

**¿Quién es el orquestador en tu CLI?** En Claude Code, Codex y Gemini es tu
sesión principal (ya atada por AGENTS.md). En OpenCode es el agente
`hoom-orquestador`: apretá **Tab** hasta seleccionarlo — es un agente primary
que tiene la edición bloqueada, así que SOLO puede delegar. Detalle para
Codex: habilitá multi-agente una vez con `[features] multi_agent = true` en
`~/.codex/config.toml`.

Commiteá todo: los subagentes viajan en el repo y tu equipo los hereda gratis.

```sh
git add -A && git commit -m "hoomAI: harness + equipo de agentes"
```

---

## Paso 6 — Meterle el contexto completo del sistema (el corazón del tutorial)

Acá es donde entra tu documento de la entrevista. **El documento del cliente NO
es el spec** — es demasiado grande para verificarlo de una vez. El harness lo
destila en cascada: documento → visión → backlog → un spec por tarea.

**6.1** Copiá el documento a la carpeta de intake:

```sh
cp ~/Documentos/srs-mi-proyecto.md .hoom/intake/
git add -A && git commit -m "intake: SRS del cliente v1"
```

**6.2** Abrí tu CLI en el proyecto y pedíselo al analista. Gracias a los
subagentes nativos, alcanza con:

```
Usá el subagente hoom-analista: el documento del cliente está en
.hoom/intake/srs-mi-proyecto.md. Producí la visión y el backlog.
```

(En OpenCode podés invocarlo directo: `@hoom-analista procesá
.hoom/intake/srs-mi-proyecto.md`.)

El analista tiene reglas duras en su contrato: prohibido inventar
requerimientos, toda regla de negocio cita su sección del documento, y los
vacíos se marcan como **PREGUNTA PARA EL CLIENTE**.

**6.3** Revisá lo que produjo (esto es trabajo TUYO, 10 minutos bien gastados):
- `00-vision.md`: ¿las reglas innegociables son las que el cliente dijo?
  ¿Cada una cita la sección del documento? ¿Los no-goals están?
- `backlog.md`: ¿el orden respeta las prioridades del cliente? ¿Cada item es
  del tamaño de una tarea (no "hacer toda la tienda" sino "catálogo",
  "precios por región", "checkout" por separado)?
- La sección **PREGUNTAS PARA EL CLIENTE** es tu lista de pendientes reales:
  mandáselas al cliente HOY. Cada respuesta evita días de retrabajo.

**6.4** Cuando estés conforme:

```sh
git add -A && git commit -m "vision y backlog aprobados"
```

Desde ahora, todo agente que trabaje en el proyecto lee esa visión. El
contexto del cliente vive EN el repositorio, versionado, no en tu memoria ni
en un chat perdido.

---

## Paso 7 — El primer veredicto verde (tu baseline)

```sh
hoom verify
```

La primera vez probablemente salga algo en rojo o amarillo. Ajustá `hoom.yaml`
(comandos reales de tu proyecto, gates no listos en `cmd: ""`) y repetí hasta
ver:

```
veredicto: VERDE
```

Ese es tu punto de partida: desde acá, todo cambio se compara contra algo que
funcionaba. Commiteá:

```sh
git add -A && git commit -m "baseline hoomAI verde"
```

---

## Paso 8 — Poner el candado

```sh
hoom hook
```

Instala un gate en Git: **sin veredicto verde cuya huella coincida con tu
código actual, `git push` se bloquea**. Esto impide el fraude clásico de la
IA: verificar una versión y entregar otra "con un arreglito más". (Escape
consciente si algún día lo necesitás: `HOOM_SKIP=1 git push` — queda a la
vista, nunca es silencioso.)

Dato tranquilizador: la huella es de **contenido puro** — commitear
exactamente lo que verificaste NO la rompe. Solo cambiar el código la rompe.

---

## Paso 9 — Tu primera tarea real (el flujo completo)

Con los subagentes instalados, tu prompt es una línea. Tomá el **primer item
del backlog** y decile a tu sesión principal (el orquestador):

**Prompt 1 — pedir el spec (y frenar):**

```
Implementá el item "<nombre-del-item>" del backlog. Spec primero
(hoom-arquitecto, con contexto de hoom-scout) y esperá mi aprobación
antes de tocar código.
```

El orquestador delega solo: el scout mapea, el arquitecto escribe
`.hoom/specs/<nombre-del-item>.md` — con los criterios de aceptación
**enumerados como CA-1, CA-2, ...** — y la sesión se detiene.

**Tu momento de control (≤3 minutos):** leé el spec. ¿Los criterios CA-n son
los del cliente? ¿Los no-goals excluyen lo que no va? Corregí AHÍ — es cien
veces más barato que corregir código.

**Prompt 2 — autorizar la implementación:**

```
Spec aprobado. Adelante: tests adversariales primero (hoom-test-writer, solo
desde el spec, referenciando cada CA-n en el nombre o comentario del test),
después hoom-writer, y cerrá con:
hoom verify --spec .hoom/specs/<nombre-del-item>.md
y hoom check. Entregame la ruta del veredicto.
```

El `--spec` agrega dos gates nuevos al veredicto: **spec_lint** (el spec tiene
sus 7 secciones y criterios con ID) y **spec_trace** (cada CA-n tiene al menos
un test que lo referencia). Un criterio sin test = veredicto ROJO con la
acción exacta. **Es la garantía de que la IA no "olvidó" ningún criterio del
cliente: lo verifica el binario, no la palabra del agente.**

**Tu cierre:** el agente te entrega la ruta del veredicto. Verificá vos mismo:

```sh
hoom check      # VERDE = el código actual ES el verificado
hoom report -n 3
git push        # el hook vuelve a exigir el check; si pasa, se integra
```

Repetí el Paso 9 con cada item del backlog. Eso es todo el sistema.

> **¿Tu CLI no tiene subagentes configurados?** Todo funciona igual con el
> prompt universal: "Actuá como el Orquestador según
> .hoom/agents/00-orquestador.md y respetá AGENTS.md. Tarea: ... Usá los roles
> Scout (03) y Arquitecto (01), producí el spec y esperá mi aprobación."

---

## Extra — ¿Dos tareas a la vez? (hoom task)

Cuando dos items del backlog son **independientes**, no hace falta esperar:

```sh
hoom task start precios-por-region    # crea rama hoom/precios-por-region + worktree aislado
hoom task start catalogo              # otra tarea, otro worktree, en paralelo
hoom task list                        # estado honesto de cada una
```

Cada tarea vive en `.hoom/worktrees/<slug>`: su propio directorio, su propia
rama, su writer y sus propios veredictos — dos agentes trabajando a la vez sin
pisarse. Entrás al worktree (`cd .hoom/worktrees/precios-por-region`), corrés
ahí el Paso 9 completo, commiteás TODO (código + veredictos), y cerrás desde
el proyecto principal:

```sh
hoom task done precios-por-region     # solo cierra con verde + huella + todo commiteado
git merge --no-ff hoom/precios-por-region
```

Si intentás cerrar con cambios sin commitear, veredicto rojo o drift, `done`
se niega y te dice exactamente qué hacer. Empezá con una tarea a la vez; el
paralelo es para cuando ya le agarraste la mano.

---

## Paso 10 — La rutina de todos los días

| Momento | Comando / acción |
|---|---|
| Nueva tarea | Prompt 1 → aprobás spec (CA-n) → Prompt 2 → veredicto con spec_trace |
| Dos tareas independientes | `hoom task start <slug>` por cada una; cierre con `hoom task done` |
| Antes de integrar | `hoom check` (el hook lo exige solo) |
| Ver cómo viene el proyecto | `hoom report -n 10` — tendencia por gate |
| Corrida completa nocturna | `hoom verify --full` (cron en un servidor) |
| El cliente cambió algo | Documento nuevo a `.hoom/intake/` con fecha → hoom-analista actualiza visión y backlog marcando las contradicciones |
| Cambiaste de CLI (o sumás una) | `hoom agents --target <la-nueva>` — mismos contratos, otro formato |
| Deuda visible | Los AUSENTES en amarillo de cada veredicto son tu backlog de gates |

---

## Errores típicos de novato (y su solución)

**"Un gate dice ERROR y el veredicto es ROJO"** → Configuraste el comando de
una herramienta que no está instalada. Instalala, o declarála ausente con
`cmd: ""` hasta que la adoptes. Roto ≠ ausente: roto es rojo, ausente es
amarillo visible.

**"`hoom check` dice ROJO pero verify me había dado VERDE"** → Editaste el
código DESPUÉS del verify (aunque sea un comentario). Es el sistema
funcionando: la huella ya no coincide. Corré `hoom verify` de nuevo y listo.
(Commitear lo verificado NO rompe la huella — solo cambiar el contenido.)

**"spec_trace salió ROJO"** → Hay criterios CA-n del spec sin ningún test que
los mencione. El veredicto te lista cuáles. Pedile al test-writer que cubra
esos criterios — para eso existe el gate: ningún requerimiento del cliente se
queda sin test en silencio.

**"`hoom task done` se niega a cerrar mi tarea"** → Lee el mensaje: o tenés
cambios sin commitear dentro del worktree (incluidos los veredictos), o el
último veredicto es rojo, o editaste después del verify (drift). Las tres
tienen la acción exacta impresa.

**"Mi CLI no ve los subagentes"** → Reiniciá la sesión de la CLI (los agentes
se cargan al arrancar). En OpenCode: Tab para llegar a `hoom-orquestador` o
`@hoom-<rol>` para invocar uno. En Codex: verificá `[features]
multi_agent = true` en `~/.codex/config.toml`. En Gemini CLI: al arrancar te
pide "Acknowledge and Enable" para los agentes nuevos.

**"El agente dice que los tests pasan"** → No importa lo que diga. Importa el
veredicto. Pedile la ruta de `.hoom/verdicts/...json` y corré `hoom check` vos.

**"El spec que produjo el Arquitecto no es lo que el cliente pidió"** → El
Analista destiló mal la visión o el documento fuente estaba flojo. Volvé al
Paso 6.3: la visión es el contrato; arreglala ahí y regenerá el spec.

**"Quiero saltarme el spec, es una tarea chiquita"** → Para tareas triviales
está permitido: el Orquestador puede rutear directo al Writer. Pero si toca
reglas de negocio, dinero o permisos, spec siempre.

**"Mi proyecto ya existe y tiene código viejo sin tests"** → Antes de dejar
que un agente refactorice, pedile characterization tests al rol
`hoom-characterizer`: primero se fija en verde lo que el código HACE hoy,
después se toca.


## Licencia

MIT
