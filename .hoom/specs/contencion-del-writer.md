# Spec: lo que corre código del writer corre encerrado

Estado: BORRADOR — pendiente de aprobación humana. Falta la firma de la
persona; nadie más la pone.
Tarea: `hoom task start contencion-del-writer` (rama
`hoom/contencion-del-writer`).
Origen: el No-goal que la persona agregó el 2026-11-13 a
`autonomia-entre-firmas.md` ("Contener a un writer que se propone burlar los
controles dejando código que corra después de su run") y dos de sus riesgos
("El guardado del piloto corre los hooks del repo", "El piloto decide sobre
telemetría local").
Depende de: `autonomia-entre-firmas.md`, en main desde los PR 82, 83 y 84:
el guardado del piloto (`itemcmd.SaveTarea`), el gate de material de git
(`agentcmd.RuleMaterialDeGit`, aclaración 25) y los tests protegidos.
Re-expresa: ningún criterio. Este spec amplía. Si al implementarlo aparece un
test que fija lo contrario, se trae a la persona antes de tocarlo.

## Objetivo

`autonomia-entre-firmas` contiene al writer que se pasa de celo: mira lo que
cambió el run y lo que commitea el guardado. No contiene al writer que se
propone burlar los controles, el que deja algo para que corra cuando hoom ya
no mira. Hoy ese writer tiene cuatro salidas, leídas en el código de main
(2cdb3c0):

1. **Procesos que sobreviven.** El run del provider, cada gate de verify y
   cada git del guardado corren en su propio grupo de procesos
   (`procesos.Arrancar`), y al terminar `procesos.Liberar` mata lo que quede
   en el grupo. El guardado además corta cada git a los `itemcmd.TopeGit` y
   mata su grupo. Pero un proceso que hace `setsid` sale del grupo y sigue
   vivo. También sigue vivo lo que el writer le pide a un daemon que ya
   corre: `systemd-run --user`, un servidor de tmux, docker.
2. **Escrituras fuera del árbol.** El gate de scope fotografía solo el árbol
   donde corre el rol (`agentcmd.Take(runDir)`). El de material de git
   fotografía cuatro piezas: la carpeta de hooks efectiva, el `config` común,
   el `config.worktree` y el `info/attributes` común. El run hereda el
   entorno y el `$HOME` de la persona (`cmd.Env = os.Environ()` en
   `runcmd`), así que el writer puede escribir sin que lo vea ningún gate:
   - en `$HOME`: `~/.gitconfig`, `~/.bashrc`, `~/.local/bin/hoom`,
     `~/go/bin/golangci-lint`, `~/.config/go/env`, `~/.claude/settings.json`;
   - en el checkout principal: `hoom.yaml`, de donde el piloto lee la
     política `autonomia`; `.hoom/items/<slug>.yaml`, con su
     `presupuesto_usd`; `.hoom/envelopes/`;
   - en el worktree de otra tarea.
3. **El verify del sobre corre el código del writer después del gate.** En
   `agentcmd.Run` el gate de scope corre antes que `verifycmd.Run`, y los
   gates de verify corren los tests y el build del writer. Lo que ese código
   escriba fuera del árbol, o en el material de git, ya no lo mira nadie. Lo
   mismo pasa con el verify del paso `cerrar-hallazgos` del piloto.
   Hallazgo de este borrador; el No-goal no lo nombraba.
4. **Los hooks del guardado corren lo que el árbol les da.** Con husky,
   `core.hooksPath` es `.husky/_`, y cada hook de ahí llama a
   `.husky/<hook>`, que está en el árbol y que el writer puede escribir. El
   gate solo mira las entradas de la carpeta de hooks efectiva. Pasa lo mismo
   con `.pre-commit-config.yaml`, `lefthook.yml` o un `scripts/lint.sh` que
   el hook llame. Hallazgo de este borrador.

No se puede hacer una lista completa de lo que corre después: un hook llama a
un script, el script a `npm`, `npm` a `package.json`. Por eso este spec
encierra en vez de enumerar, y mantiene el gate para lo que sí se puede
fotografiar.

Con este spec, **todo lo que corre código del writer bajo un sobre o el
piloto corre encerrado**: su run, los gates del verify que lo juzga y el git
del guardado. Encerrado quiere decir que escribe solo en su tarea, no ve ni
señala procesos de afuera, y nada suyo sigue vivo cuando termina. Donde el
sistema no deja encerrar, el sobre lo dice y la persona decide si el writer
corre igual.

**Regla.** Lo que se puede encerrar se encierra; lo que no, se fotografía; y
lo que tampoco se puede fotografiar se declara.

## No-goals

- **Ocultar lecturas.** El writer sigue leyendo todo el disco de la persona,
  `~/.ssh` incluido (ver Riesgos y la pregunta 4).
- **La red.** El writer sigue con red porque la CLI la necesita. Llevarse un
  secreto o bajar algo queda fuera.
- **Encerrar a los demás roles** (test-writer, reviewer, refutador,
  arquitecto). Siguen con sus controles de hoy. El mecanismo sirve para
  ellos, en su propia spec.
- **Lo que la persona corre a mano**: `hoom verify`, `hoom task done`,
  `hoom item save`, un `git commit` en el worktree, o abrir el árbol en un
  editor. La persona corre ese código sabiendo que es del writer.
- **Inyectar instrucciones en archivos del árbol** (comentarios, `AGENTS.md`,
  `CLAUDE.md`) dirigidas a la review o a otro rol. No es código que corre.
- **El encierro en macOS y las escrituras en Windows.** Se evalúan en
  Decisiones y quedan para su propia spec (pregunta 2).
- **Root, otra cuenta de usuario, contenedores o máquinas virtuales** para el
  writer.
- **Revalidar los sobres anteriores a este spec.**
- **Mostrar el nivel del encierro en la tarjeta o en el Studio.**
- **Endurecer la máquina** (sysctl, AppArmor, SELinux).

## Contratos

### Qué corre encerrado

| Qué | Hoy | Con este spec |
|---|---|---|
| el run de un rol de forma `codigo` (hoy solo el writer), lo lance `hoom agent`, el Studio, `hoom pedir` o el piloto | su grupo de procesos | encierro |
| los gates del verify que corre ese mismo sobre | su grupo de procesos | encierro, con la vista del writer |
| los gates del verify del paso `cerrar-hallazgos` | su grupo de procesos | encierro, con la vista del writer |
| cada git del guardado del piloto (`SaveTarea`), con sus hooks y filtros | su grupo, con `TopeGit` | encierro, con `TopeGit` |
| `hoom verify` de la persona, `hoom item save`, los demás roles | — | como hoy |

### 1. Ningún proceso del writer sobrevive a lo que lo lanzó

**Por qué.** Un proceso vivo después del run hace lo que quiera cuando ya
pasó el gate: reescribe un test protegido después de la foto, espera al
guardado para tocar el índice, o se queda escuchando. El grupo de procesos de
hoy se puede dejar con un `setsid`, y un daemon que ya existe no es hijo de
nadie que hoom conozca.

En Linux, el encierro es un espacio de procesos propio:

- Cada cosa encerrada corre en su propio espacio de nombres de PID, de
  usuario y de montajes, armado con bubblewrap (`bwrap`). Adentro no se ven
  ni se pueden señalar los procesos de afuera: ni hoom, ni el Studio, ni la
  terminal de la persona.
- **Termina con su proceso principal.** Cuando el CLI, el `sh` del gate o el
  git termina (bien, mal, por su tope o porque hoom lo cancela), el encierro
  termina, y el kernel mata todo lo que quedó adentro, con `setsid` o con
  doble fork. Vale igual para un relevo, un reintento o una ronda.
- **Muere con hoom.** Si muere el proceso de hoom que lo lanzó, el encierro
  muere con él.
- **Sin terminal.** Corre en una sesión nueva, sin terminal de control: no
  puede abrir `/dev/tty` ni escribirle a la terminal de la persona.
- **Sin privilegios nuevos.** `no_new_privs` hace que `crontab`, `at`, `sudo`,
  `su` y cualquier binario setuid no ganen nada. No puede crear espacios de
  nombres de usuario anidados.
- **Sin los sockets que lanzan procesos.** El encierro tapa
  `/run/user/<uid>` (el bus de systemd del usuario, la sesión gráfica, los
  agentes de ssh y gpg), `/run/docker.sock`, `/run/podman/` y `/tmp`, que es
  privado y se descarta al terminar. Así quedan afuera
  `systemd-run --user`, tmux, docker y X11 por ruta.
- **Sin sockets abstractos de afuera.** Con Landlock ABI 6 o más nuevo
  (Linux 6.12), un paso de hoom dentro del encierro, antes del CLI, le
  prohíbe conectarse a un socket abstracto de fuera de su dominio. Esa es la
  vía de X11 a la sesión gráfica de la persona.
- El tope de tiempo no cambia: `agents.<rol>.timeout`, `review.timeout`,
  `TopeGit` y el tope de cada gate. Cuando vence, hoom cancela el proceso
  principal y el encierro termina con todo lo que tiene adentro.

En Windows, el Job Object que ya usa `procesos.Adoptar` no deja salir a un
hijo y lo mata al cerrarse: los procesos ya quedan contenidos, salvo lo
declarado en Riesgos. En macOS no hay forma de contenerlos con este spec (ver
Decisiones).

### 2. El writer escribe solo en su tarea

**Por qué.** Todo lo que está fuera del árbol y corre después escapa a los
gates: la config global de git, el PATH de la persona, la config de las CLI,
los archivos de inicio del shell, la política del piloto en el checkout
principal. La lista no se puede cerrar. En cambio, lo que el writer
necesita escribir es corto.

**La vista del encierro**, en Linux:

- **Lectura**: todo el sistema de archivos como lo ve la persona, de solo
  lectura, con un `/proc` y un `/dev` propios y un `/tmp` privado.
- **Escritura**, solo en:
  1. el árbol de la tarea, menos el material de git y la config de los
     agentes que están adentro (ver el punto 3), que quedan de solo lectura;
  2. el directorio común de git, menos el material de git, `objects/info/`,
     `shallow` y los directorios de administración de los otros worktrees.
     El writer sigue pudiendo commitear en `hoom/<slug>` (ver "Git ajeno");
  3. `.hoom/cache/encierro/<slug>/` del checkout principal: las cachés y los
     temporales de la tarea, que hoom crea si no existen;
  4. lo que la CLI del provider necesita escribir, que declara su adaptador
     (ver "El estado del provider");
  5. lo que el proyecto agrega en `encierro.escribibles`.
- **El entorno** es el de hoy, menos `SSH_AUTH_SOCK`, `GPG_AGENT_INFO`,
  `DBUS_SESSION_BUS_ADDRESS`, `DISPLAY`, `WAYLAND_DISPLAY` y
  `XDG_RUNTIME_DIR`, que apuntarían a lo tapado. Le agrega las variables de
  caché, todas dentro de `.hoom/cache/encierro/<slug>/`: `TMPDIR` y
  `XDG_CACHE_HOME` siempre, más las que declara el perfil del proyecto. En
  el perfil go son `GOCACHE`, `GOMODCACHE` y `GOTMPDIR`. Cada perfil declara
  las suyas; con gradle, `GRADLE_USER_HOME` también tapa los scripts de
  `~/.gradle/init.d/`.

**Qué se bloquea, qué se detecta y qué no se puede.** "Sin encierro" es
macOS, Windows en lo que toca a escrituras, o Linux sin bwrap.

| Destino | Ejemplos | Linux, encierro completo | Sin encierro |
|---|---|---|---|
| `$HOME` | `~/.gitconfig`, `~/.bashrc`, `~/.profile`, `~/.local/bin/hoom`, `~/go/bin/golangci-lint`, `~/.config/go/env`, `~/.config/systemd/user/`, `~/.ssh/config` | se bloquea | se detecta solo la config de git que lee el árbol (punto 3); el resto no se ve |
| la config de las CLI | `hooks` en `~/.claude/settings.json`, `mcpServers` en `~/.claude.json`, `notify` en `~/.codex/config.toml` | se bloquea, salvo lo que el adaptador declara escribible; lo que hace correr código se detecta | se detecta (`config-de-agentes`) |
| el checkout principal | `hoom.yaml`, `.hoom/items/<slug>.yaml`, `.hoom/envelopes/`, `.hoom/approvals/` | se bloquea | no se ve, como hoy |
| otros worktrees | `.hoom/worktrees/<otra>/` | se bloquea | no se ve |
| el material de git | la carpeta de hooks, `config`, `config.worktree`, `info/attributes`, `objects/info/`, `shallow` | se bloquea y se detecta | se detecta |
| el resto del directorio común | refs ajenas, `refs/replace/`, objetos que ya existían | se detecta (`git-ajeno`) | se detecta |
| `/tmp` | sockets de tmux y de X11 | privado, se descarta | no se ve |
| daemons y setuid | `systemd-run --user`, tmux, docker, `crontab`, `at` | se bloquea | no se ve |
| un socket del sistema que el encierro no tapa | un servicio que hace algo a pedido | no se puede (Riesgos) | no se puede |
| la red | subir algo a un servidor | no se puede (No-goals) | no se puede |

**Git ajeno.** El directorio común de git queda escribible porque el writer
commitea y los hooks del guardado escriben refs y objetos: lint-staged, por
ejemplo, usa `git stash`. A cambio, hoom compara después de cada encierro del
writer y de cada guardado:

- **las refs**: todas, sueltas y empaquetadas, `refs/replace/` incluida, y el
  `HEAD` de cada worktree. Una ref que no es `refs/heads/hoom/<slug>` y que
  cambió es la violación nueva `git-ajeno`, con la ruta `refs/...` o
  `worktrees/<nombre>/HEAD`;
- **los objetos**: un objeto suelto o un pack que ya existía y cambió de
  contenido (hoom revisa solo los archivos con fecha de cambio posterior al
  arranque) también es `git-ajeno`, con la ruta `objects/...`.
- En el sobre, `git-ajeno` corta como `material-de-git`: cierra en `scope`,
  `no-entregable`, exit 1, sin verify, con la nota
  `el writer cambio git fuera de la rama de su tarea: no se emite veredicto sobre este arbol`,
  y un hallazgo high del gate de scope con el detalle
  `el writer solo cambia la rama de su tarea: las demas refs y los objetos que ya existian no los toca`.
- En el guardado, falla y deshace su commit como hoy, con el error
  `el guardado cambio git fuera de hoom/<slug>: <ruta>: no guardo`. hoom no
  restaura la ref ajena: eso lo decide la persona.
- `git-ajeno` aplica en todos los niveles del encierro, también sin él.

### 3. Lo que queda para correr después: el gate amplía, no se reemplaza

**Por qué.** El encierro bloquea, pero no existe en todas partes ni en todos
los kernels. Además, un bloqueo es silencioso: el writer recibe un error del
sistema y nadie se entera de que lo intentó. El gate funciona en cualquier
sistema y deja un hallazgo con la ruta. Por eso se quedan los dos: el
encierro donde se puede y el gate siempre. El gate gana más material, más
momentos y una regla nueva.

**Más material.** `FotoMaterialGit` suma, a las cuatro piezas de hoy:

- **la config de git que lee el árbol fuera del repo**: cada archivo que
  `git config --list --show-origin` nombra como origen en el árbol de la
  tarea (`~/.gitconfig`, `$XDG_CONFIG_HOME/git/config` y lo que alcanza un
  `include.path` o un `includeIf`), el archivo de `core.attributesFile` y
  `$XDG_CONFIG_HOME/git/attributes`. `/etc/gitconfig` no entra: la persona
  tampoco lo escribe sin privilegios;
- `objects/info/` (alternates, grafts) y `shallow` del directorio común;
- **los gestores de hooks del árbol**:
  - si la carpeta de hooks efectiva está dentro del árbol, las entradas que
    están directamente en su carpeta padre (husky llama desde `.husky/_/` a
    `.husky/<hook>`);
  - siempre, `.pre-commit-config.yaml`, `lefthook.yml`, `lefthook.yaml`,
    `.lefthook.yml`, `.lefthook.yaml` y `lefthook-local.yml` de la raíz del
    árbol, y la carpeta `.lefthook/`.
- Las piezas nuevas usan las mismas huellas y el mismo tope de foto que las
  de hoy (aclaraciones 28 y 29), y cuentan como `material-de-git`.

**Más momentos.** Hoy el material se fotografía antes del run y se compara
después. Se suman dos comparaciones:

- **Después del verify del sobre.** El sobre del writer lo vuelve a
  fotografiar al terminar su paso `verify` y lo compara con la foto de antes
  del run. Si cambió, el sobre cierra en `verify`, `no-entregable`, exit 1,
  con la nota de material de git y el hallazgo high de siempre (con la ruta
  que cambió). El veredicto que escribió verify queda en disco y el registro
  del sobre lo nombra, pero no cuenta como entrega. Vale igual para el
  verify de `cerrar-hallazgos`, que se detiene con
  `el verify de la ronda <n> cambio el material de git: <ruta>: te toca`.
- **Alrededor del guardado.** `SaveTarea` fotografía el material antes de su
  primer git y después del último. Si cambió, falla, deshace su commit y
  devuelve `un hook del guardado cambio el material de git: <ruta>: no guardo`.

**La config de los agentes.** Regla nueva, `config-de-agentes`, para un rol
de forma `codigo`. Corta como `material-de-git`, con la nota
`el writer cambio la config de los agentes: no se emite veredicto sobre este arbol`
y el detalle
`la config de los agentes hace correr codigo en la proxima sesion: el writer no la cambia`.
Cubre:

- **en el árbol**: `.claude/`, `.codex/`, `.mcp.json` y lo que declare cada
  adaptador. Un hook en `.claude/settings.json` correría en la próxima
  sesión de Claude en ese árbol: el reintento, el relevo, o el test-writer
  ciego si el archivo llega a su árbol;
- **fuera del árbol**: solo las claves que hacen correr código, que el
  adaptador sabe leer:
  - claude: `hooks` de `~/.claude/settings.json` y
    `~/.claude/settings.local.json`, y `mcpServers` de `~/.claude.json`,
    global y de cada proyecto;
  - codex: `notify`, `mcp_servers` y `hooks` de `~/.codex/config.toml`.
- Que la CLI cambie otras claves de esos archivos, como sus contadores o su
  historial, no es violación.
- `agents.<rol>.write` de `hoom.yaml` no levanta ninguna de las dos reglas.

**El encierro alrededor del guardado.** Cada git de `SaveTarea` corre
encerrado con la vista del writer y con `TopeGit`. Un hook, o el script que
llama, puede hacer en el árbol y en git lo mismo que el writer en su run, y
el guardado lo revisa como hoy (`revisarCommit`, aclaraciones 24 a 30), más
el material y `git-ajeno`. El git del guardado lleva además
`-c commit.gpgsign=false`: adentro no hay agente de gpg ni de ssh para
firmar (ver la pregunta 3).

### El nivel del encierro

- Cada sobre de un rol de forma `codigo` calcula un **nivel**, con su
  **mecanismo** y lo que **falta** (`procesos`, `escrituras`, `ipc`):

  | Sistema | Nivel | Mecanismo | Falta |
  |---|---|---|---|
  | Linux con bwrap, espacios de usuario sin privilegios y Landlock ABI 6 o más | `completo` | `bwrap` | — |
  | Linux con bwrap y espacios de usuario, sin Landlock ABI 6 | `parcial` | `bwrap` | `ipc` |
  | Linux sin bwrap, o con bwrap que no puede crear el espacio de usuario | `ninguno` | — | `procesos`, `escrituras`, `ipc` |
  | Windows | `parcial` | `job` | `escrituras`, `ipc` |
  | macOS | `ninguno` | — | `procesos`, `escrituras`, `ipc` |

- En Linux, hoom lo averigua una vez por proceso con una sonda: corre bwrap
  con la vista mínima y un comando que termina, y pregunta a Landlock su ABI.
  Si la sonda falla, el motivo queda en el registro: `bwrap no esta`, la
  salida de bwrap (por ejemplo, el rechazo de AppArmor en Ubuntu 24.04) o
  `landlock ABI <n>`.
- **El registro del sobre** y el sidecar del run ganan
  `encierro: {nivel, mecanismo, falta, motivo}`. Los sobres de otras formas
  no lo llevan.
- Con nivel menor que `completo`, el log del sobre suma
  `aviso: encierro <nivel> (falta: <lista>): lo que el writer deje fuera de su arbol no se ve`.

### La política `encierro`

Sección nueva y opcional de `hoom.yaml`, leída del checkout principal como
`autonomia`:

```yaml
encierro:
  exigir: completo          # completo, parcial o no (no por defecto)
  escribibles:              # rutas fuera del arbol que el encierro deja escribir
    - ~/.semgrep
  gates_sin_encierro:       # gates del verify que corren como hoy
    - e2e
```

- **`exigir`.** Si el nivel del sistema es menor, el sobre del writer se
  niega en un paso nuevo, `encerrar`, antes de lanzar el run (como `aislar`
  para el rol ciego). Cierra con exit 1, sin run ni veredicto, con la nota
  `el encierro de este sistema es <nivel> (falta: <lista>) y hoom.yaml exige <exigir>: el writer no corre`.
  Para el piloto es un cierre no entregable, su caso 8. Con `no` (el valor
  por defecto) el writer corre con el nivel que haya, y el aviso queda en el
  log.
- **`escribibles`**: rutas absolutas o que empiezan con `~/`. Valen para todo
  lo encerrado.
- **`gates_sin_encierro`**: nombres de gates de `gates:`. Esos gates corren
  como hoy, sin encierro, también en el verify del sobre. Es la salida para
  un gate que necesita docker, y debilita el encierro: el README lo dice.
- **Errores**, todos con el prefijo `hoom.yaml invalido: `:
  - `encierro: clave desconocida %q (validas: exigir, escribibles, gates_sin_encierro)`;
  - `encierro: exigir tiene que ser completo, parcial o no`;
  - `encierro: <ruta> no puede ser escribible: <motivo>`, para `~`, `/`, una
    ruta relativa, o una que contiene o está dentro del checkout principal o
    del directorio común de git;
  - `encierro: el gate <g> no esta en gates`.
- `hoom.yaml` ya está en el piso del gate de scope, y con encierro el del
  checkout principal es de solo lectura: el writer no cambia esta sección.

### El estado del provider

- Cada adaptador declara `Estado()`:
  - `Escribe`: las rutas fuera del árbol que su CLI escribe (sesiones, logs,
    credenciales que se refrescan);
  - `EnArbol`: su config dentro del árbol;
  - `Corre`: cómo leer, de cada archivo de config, solo lo que hace correr
    código.
- El encierro deja escribibles solo las rutas de `Escribe`. `EnArbol` y
  `Corre` alimentan `config-de-agentes`, con encierro o sin él.
- Las listas de claude y codex las fija una sonda con la versión real de cada
  CLI, que corre una vez antes de implementar y queda escrita en el comentario
  de cada adaptador. Por ejemplo, claude escribe `~/.claude.json` en cada
  arranque, y la sonda dice si el encierro puede dejarlo de solo lectura.

### Lo que queda escrito

- El registro del sobre gana `encierro`. El de un sobre cortado después de
  verify nombra el veredicto que no cuenta.
- Las reglas nuevas `git-ajeno` y `config-de-agentes` registran su hallazgo
  con el formato de siempre del gate de scope. `material-de-git` conserva su
  nombre, su nota y su detalle, así que el piloto sigue leyéndolo con
  `agentcmd.DeMaterialDeGit`.
- Líneas nuevas en el log del sobre:
  - `encerrar  encierro <nivel> con <mecanismo>`, o
    `encerrar  sin encierro: <motivo>`, al empezar el run;
  - el aviso de nivel menor que `completo`.

### Documentación y pruebas

- El README documenta el encierro (qué corre encerrado y qué no), los
  niveles, la sección `encierro` y que `gates_sin_encierro` lo debilita.
  Sin tildes, como el resto de sus tablas.
- Los tests usan los CLIs falsos de siempre (un script en el `PATH`) y un
  provider falso que declara su `Estado`. Ninguno llama a un CLI real.
- El CI de Linux instala bubblewrap y habilita los espacios de usuario sin
  privilegios. En Ubuntu 24.04 eso es
  `sysctl kernel.apparmor_restrict_unprivileged_userns=0`. Un test del
  encierro que no encuentra bwrap falla: no se salta.
- La sonda de nivel se puede reemplazar en los tests, para probar `parcial` y
  `ninguno` en una máquina que tiene `completo`.

## Casos límite y errores esperados

- **El writer corre `hoom verify` adentro**, como pide su contrato: su
  veredicto queda en el árbol de la tarea, como hoy. Si ese hoom necesita
  escribir fuera del árbol, falla con el error del sistema. El veredicto que
  cuenta es el que corre el sobre.
- **El writer commitea** en `hoom/<slug>`: funciona. Commitear en otra rama, o
  mover `main`, es `git-ajeno`.
- **El writer necesita una dependencia nueva**: la baja a la caché de la
  tarea. La primera vez cada tarea arranca con la caché vacía y tarda más.
- **Un gate que escribe en `$HOME`** (semgrep escribe `~/.semgrep/`) falla
  encerrado. La persona agrega la ruta a `escribibles` o el gate a
  `gates_sin_encierro`.
- **Un gate con docker** falla encerrado porque no ve el socket. Va en
  `gates_sin_encierro`, con lo que eso debilita.
- **Un hook del guardado que firma, empuja o habla con un agente** (gpg, ssh,
  `git push` por ssh) falla encerrado, el guardado falla y el piloto se
  detiene con `no pudo guardar el trabajo de la tarjeta: <error>`. La
  persona guarda con `hoom item save`, que no corre encerrado.
- **Un repo con `commit.gpgsign=true`**: los guardados del piloto quedan sin
  firma. La persona firma al integrar.
- **lint-staged o pre-commit en el guardado**: escriben `refs/stash` y la
  restauran. Como la ref queda igual, no es `git-ajeno`. Si queda distinta,
  sí.
- **Una tarea que necesita cambiar `.husky/`, `.claude/` o
  `.pre-commit-config.yaml`**: el writer no puede. Lo cambia la persona, como
  el spec y `hoom.yaml`.
- **Un `include.path` que apunta adentro del árbol**: ese archivo pasa a ser
  material de git y el writer no lo cambia.
- **El writer mata al CLI desde adentro**: el encierro termina con el CLI y el
  sobre cierra como un run que terminó mal. No puede matar a hoom: no lo ve.
- **El writer agota el tope dejando un proceso con `setsid`**: cuando vence el
  tope, el encierro termina con todo lo que tiene adentro.
- **La persona cambia sus hooks mientras corre un guardado**: el guardado ve
  el material cambiado y falla. La persona lo vuelve a pedir.
- **Dos sobres de writer de tareas distintas a la vez**: cada uno tiene su
  encierro y su caché. El directorio común de git es el mismo, y cada uno
  solo puede mover su rama sin caer en `git-ajeno`.
- **Linux con bwrap pero sin espacios de usuario** (Ubuntu 24.04 sin el
  perfil de AppArmor): nivel `ninguno`, con la salida de bwrap como motivo.
  Con `exigir: no`, el writer corre como hoy, con el aviso.
- **Un sobre anterior a este spec**: no lleva `encierro`, y nada lo
  revalida.

## Criterios de aceptación

- CA-674: el registro de un sobre de forma `codigo` y el sidecar de su run llevan `encierro` con `nivel` (`completo`, `parcial` o `ninguno`), `mecanismo`, `falta` y `motivo`; los sobres de otras formas no llevan la clave; con nivel menor que `completo`, el log del sobre trae la línea de aviso del contrato.
- CA-675: el `hoom.yaml` del proyecto acepta la sección `encierro` con `exigir`, `escribibles` y `gates_sin_encierro`; sin la sección, `exigir` vale `no`; rechaza con el texto del contrato una clave desconocida, un `exigir` fuera de la lista, una ruta escribible que es `~`, `/`, relativa o que contiene o está dentro del checkout principal o del directorio común de git, y un gate que no está en `gates`.
- CA-676: con un nivel menor que `exigir`, el sobre del writer cierra en `encerrar` con exit 1 y la nota `el encierro de este sistema es <nivel> (falta: <lista>) y hoom.yaml exige <exigir>: el writer no corre`, sin ningún run en `.hoom/runs/` y sin veredicto nuevo; con `exigir: no` corre con el nivel que haya.
- CA-677: el nivel sale de la tabla del contrato para cada combinación de sistema y resultado de la sonda (bwrap ausente, espacio de usuario rechazado, Landlock por debajo de ABI 6, todo disponible), con su mecanismo, lo que falta y el motivo; en macOS es `ninguno` y en Windows `parcial` con mecanismo `job` y falta `escrituras` e `ipc`.
- CA-678: en Linux con encierro, un CLI falso que escribe en `$HOME`, en `hoom.yaml` y en `.hoom/items/<slug>.yaml` del checkout principal, en el worktree de otra tarea y en `/tmp` no deja nada de eso en el disco de afuera; lo que escribe en el árbol de la tarea, en `.hoom/cache/encierro/<slug>/`, en una ruta de `Escribe` de su provider y en una de `escribibles` queda.
- CA-679: en Linux con encierro, un CLI falso que deja un proceso con `setsid` y otro con doble fork, y termina, no deja ninguno vivo cuando cierra el sobre; tampoco cuando el run muere por su timeout, ni cuando hoom cancela el run.
- CA-680: en Linux con encierro, el CLI falso no ve el pid de hoom ni puede señalarlo, no puede abrir `/dev/tty`, no recibe `SSH_AUTH_SOCK`, `GPG_AGENT_INFO`, `DBUS_SESSION_BUS_ADDRESS`, `DISPLAY`, `WAYLAND_DISPLAY` ni `XDG_RUNTIME_DIR`, ve vacío `/run/user/<uid>`, no ve `/run/docker.sock`, y con Landlock ABI 6 o más no se conecta a un socket abstracto que escucha fuera del encierro.
- CA-681: en Linux con encierro, escribir en la carpeta de hooks efectiva, en el `config` común, en el `config.worktree` de su worktree, en `info/attributes`, en `objects/info/` o en `shallow` falla dentro del encierro, y un `git commit` del writer en `hoom/<slug>` funciona.
- CA-682: todo lo encerrado recibe `TMPDIR` y `XDG_CACHE_HOME` dentro de `.hoom/cache/encierro/<slug>/`, que hoom crea si no existe, y las variables de caché de su perfil (`GOCACHE`, `GOMODCACHE` y `GOTMPDIR` en el perfil go) dentro del mismo directorio.
- CA-683: los gates del verify que corre el sobre del writer, y los del verify de `cerrar-hallazgos`, corren encerrados con la vista del writer: un gate falso que escribe en `$HOME` o en la carpeta de hooks no deja nada afuera, y uno que deja un proceso con `setsid` no lo deja vivo; un gate de `gates_sin_encierro` corre como hoy; `hoom verify` de la persona no corre encerrado.
- CA-684: cada git de `SaveTarea` corre encerrado, con `TopeGit` y con `commit.gpgsign=false`: un hook que escribe en `$HOME` o deja un proceso con `setsid` no deja nada afuera ni vivo; un hook que pasa el tope termina con todo lo que lanzó y el guardado falla como hoy; `hoom item save` no corre encerrado.
- CA-685: después del run del writer, y después de cada guardado, una ref distinta de `refs/heads/hoom/<slug>` que cambió (suelta o empaquetada, `refs/replace/` incluida, o el `HEAD` de otro worktree), o un objeto suelto o un pack que ya existía y cambió de contenido, es la violación `git-ajeno`: el sobre cierra en `scope`, `no-entregable`, exit 1, sin verify, con la nota y un hallazgo high con el detalle del contrato; el guardado falla, deshace su commit y devuelve `el guardado cambio git fuera de hoom/<slug>: <ruta>: no guardo`; vale igual sin encierro; un `refs/stash` que queda igual no es violación.
- CA-686: `FotoMaterialGit` incluye cada archivo que `git config --list --show-origin` nombra en el árbol fuera de `/etc`, `core.attributesFile`, `$XDG_CONFIG_HOME/git/attributes`, `objects/info/`, `shallow`, las entradas directas de la carpeta padre de la carpeta de hooks efectiva cuando está dentro del árbol, y los archivos de pre-commit y lefthook del contrato; un writer que cambia, crea o borra cualquiera de ellos, sin encierro, comete la violación `material-de-git` con esa ruta.
- CA-687: el sobre del writer vuelve a fotografiar el material de git después de su verify; si un gate lo cambió, el sobre cierra en `verify`, `no-entregable`, exit 1, con la nota de material de git y un hallazgo high `material-de-git: el rol writer escribio <ruta> - ...`, y su registro nombra el veredicto que escribió verify sin tomarlo como entrega; en `cerrar-hallazgos` el piloto se detiene con `el verify de la ronda <n> cambio el material de git: <ruta>: te toca`.
- CA-688: `SaveTarea` fotografía el material de git antes de su primer git y después del último; un hook que lo cambia hace fallar el guardado, que deshace su commit y devuelve `un hook del guardado cambio el material de git: <ruta>: no guardo`; un hook que no lo cambia guarda como hoy.
- CA-689: para un rol de forma `codigo`, cambiar `.claude/`, `.codex/`, `.mcp.json` o la config en el árbol que declara su adaptador, o las claves que hacen correr código fuera del árbol (`hooks` de los settings de claude, `mcpServers` global o de un proyecto en `~/.claude.json`, `notify`, `mcp_servers` y `hooks` de `~/.codex/config.toml`), es la violación `config-de-agentes`, que corta con su nota y su detalle; que la CLI cambie otras claves de esos archivos no lo es; `agents.writer.write` no la levanta.
- CA-690: con un provider falso que declara `Escribe`, `EnArbol` y `Corre`, el encierro deja escribir solo las rutas de `Escribe` fuera del árbol y de la caché de la tarea, y `config-de-agentes` compara lo que declaran `EnArbol` y `Corre`.
- CA-691: el README documenta el encierro, sus niveles, la sección `encierro` y que `gates_sin_encierro` lo debilita. [verifica: grep -q "encierro:" README.md && grep -q "gates_sin_encierro" README.md && grep -q "nivel del encierro" README.md]

## Decisiones

- **Encerrar en vez de enumerar.** Un hook llama a un script, el script a una
  herramienta, la herramienta lee una config del árbol. Ninguna foto cubre
  esa cadena. El encierro no necesita saber qué va a correr: le alcanza con
  saber dónde puede escribir y cuánto vive.
- **El gate amplía, no se reemplaza** (lo que pedía decidir el No-goal). El
  encierro falta en macOS, en parte de Windows y en los Linux sin espacios
  de usuario. Además bloquea en silencio. El gate funciona en todos los
  sistemas, deja un hallazgo con la ruta y es lo único que ve el material
  que el encierro deja escribible (la config de las CLI y el directorio
  común de git).
- **Cómo se contienen los procesos en Linux.** Evaluación:

  | Mecanismo | `setsid` y doble fork | si el writer mata al supervisor | daemon que ya corre | requisitos |
  |---|---|---|---|---|
  | grupo de procesos (hoy) | escapa | escapa | escapa | ninguno |
  | subreaper (`PR_SET_CHILD_SUBREAPER`) en un guardián por run | contiene: los huérfanos van al guardián, que los mata al final | escapa: con el mismo uid puede matar al guardián, y los huérfanos se van a `systemd --user` | escapa | Linux 3.4 |
  | cgroup v2 por run y `cgroup.kill` | contiene, y mata de una vez aunque esté haciendo fork | contiene: el cgroup sigue aunque muera el guardián | escapa | cgroup v2 delegado (systemd del usuario), Linux 5.14; con el mismo uid se puede mudar a otro cgroup del subárbol delegado si no hay espacio de cgroup |
  | espacio de PID (con espacio de usuario) | contiene: al terminar el principal, el kernel mata todo | contiene: no ve ni señala a nadie de afuera | contiene si además el espacio de montajes tapa los sockets | espacios de usuario sin privilegios |

  Solo el espacio de PID contiene a un writer que ataca al supervisor. El
  subreaper y el cgroup se pueden saltear con el mismo uid. El cgroup sí
  serviría para un tope de memoria y de procesos (`pids.max`, contra una
  bomba de fork), pero no hace falta para que nada sobreviva y queda fuera.
- **bubblewrap y no espacios de nombres hechos a mano.** bwrap arma el
  espacio de usuario, de PID y de montajes, `no_new_privs`, `/proc`, `/dev`
  y la sesión nueva. Lo usan Flatpak y el sandbox de Claude Code en Linux, y
  está empaquetado en todas las distribuciones. Hacerlo en Go es posible
  (`SysProcAttr` con `Cloneflags`), pero suma a hoom un runtime de
  contenedores para mantener. Lo único que hoom hace por su cuenta es
  Landlock, que son dos llamadas al sistema y no requieren cgo. En esta
  máquina, el 2026-11-13: bwrap 0.13.1, espacios de usuario habilitados,
  Landlock activo, cgroup v2 y `dev.tty.legacy_tiocsti=0`.
- **macOS queda fuera de esta spec.** Lo que hay es `sandbox-exec` (Seatbelt),
  que usan Codex y Claude Code. Contiene escrituras, señales
  (`signal (target same-sandbox)`) y servicios de mach, y se hereda sin
  vuelta atrás. Lo que no hay es forma de contener procesos: macOS no tiene
  subreaper, ni cgroups, ni espacio de PID, y `NOTE_TRACK` de kqueue no
  existe ahí. Un proceso que hace `setsid` sigue vivo, aunque encerrado en
  las mismas escrituras. `sandbox-exec` está deprecado y el CI no tiene
  runner de macOS. Ver la pregunta 2.
- **Windows: los procesos ya, las escrituras no.** El Job Object con
  `KILL_ON_JOB_CLOSE` y sin breakaway ya contiene a los hijos. Para las
  escrituras habría que correr con integridad baja o en AppContainer, y
  marcar el árbol y las cachés con esa integridad. El CI tampoco tiene runner
  de Windows. Queda para su spec.
- **Cachés por tarea, frías la primera vez.** Una caché de build es código
  que corre después: Go reutiliza lo que encuentra bajo su clave, y Gradle
  corre los scripts de `init.d`. Compartir la caché de la persona dejaría que
  el writer la envenene para los builds de ella. Compartirla de solo lectura
  rompe agregar una dependencia.
- **El estado del provider se declara, no se copia.** Una copia de
  `~/.claude` o `~/.codex` por run (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`)
  obligaría a copiar de vuelta las credenciales refrescadas, que es otro
  camino para escribir en el `$HOME` de la persona. Además, el relevo por
  tiempo necesita la sesión en el directorio real para `--resume`.
- **El directorio común de git escribible, y comparado después.** Dejar de
  solo lectura las refs ajenas no se puede con montajes: git crea sus
  `.lock` en el mismo directorio que la ref. Los hooks reales escriben
  `refs/stash`, y el writer commitea. Comparar refs y objetos después cuesta
  poco y funciona también sin encierro.
- **El writer sigue commiteando.** Sacarle el commit cambiaría su contrato
  (`.hoom/agents/04-writer.md`) y no cerraría nada que `git-ajeno` no cierre.
- **El guardado no firma.** Darle al encierro el agente de gpg o de ssh
  dejaría al writer firmar lo que quiera con la llave de la persona. Los
  commits del guardado viven en la rama de la tarea, y la persona firma al
  integrar.
- **`exigir: no` por defecto, por ahora.** Encerrar a las CLI reales todavía
  no se probó de punta a punta. Si el valor por defecto fuera `completo`, el
  piloto dejaría de andar en macOS y en Windows el día que se fusione. El
  aviso queda en cada sobre. Ver la pregunta 1.
- **Solo el writer y lo que corre su código.** Es lo que pidió el No-goal.
  Encerrar al test-writer con el mismo mecanismo también lo haría ciego de
  verdad, porque no vería el árbol real. Pero eso es lectura, y va en su
  propia spec.

### Preguntas para la firma

1. **¿`exigir` por defecto en `no` o en `completo`?** Recomendación: `no` en
   este spec, y pasarlo a `completo` cuando haya un uso real con las CLI
   reales encerradas.
2. **¿macOS en esta spec (Seatbelt sin CI) o en la suya, con un runner de
   macOS?** Recomendación: la suya.
3. **¿El guardado sin firma (`commit.gpgsign=false`), o que falle en un repo
   que firma?** Recomendación: sin firma.
4. **¿Tapar también los secretos conocidos** (`~/.ssh`, `~/.gnupg`,
   `~/.config/gh`, `~/.aws`, `~/.docker/config.json`) con el mismo
   encierro? Cuesta casi nada, pero es lectura y contradice el primer
   No-goal. Recomendación: en esta spec, como excepción explícita a ese
   No-goal, si la persona lo acepta.

## Riesgos y deuda aceptada

- **El writer puede llevarse secretos.** Lee `~/.ssh`, las credenciales de
  las CLI y el token de `gh`, y tiene red. El encierro no protege secretos
  (pregunta 4).
- **Los sockets del sistema que el encierro no tapa** siguen visibles: el bus
  del sistema y lo que haya en `/run` fuera de lo tapado. Un servicio que
  hace algo a pedido por esa vía no se ve.
- **Sin Landlock ABI 6** (Linux anterior a 6.12), el writer puede hablarle a
  un socket abstracto de afuera, como el de X11 en una sesión X: nivel
  `parcial`, falta `ipc`.
- **Lo que la persona corre después** (verify a mano, un commit en el
  worktree, abrir el árbol en un editor con tareas al abrir) corre código
  del writer sin encierro. Es el riesgo de siempre de correr código ajeno.
- **Los scripts del árbol que llama un hook** corren encerrados cuando los
  corre el guardado, y sin encierro cuando la persona commitea a mano.
- **Windows**: entre que arranca el proceso y `procesos.Adoptar` lo mete en
  su job, un hijo creado en ese instante queda afuera. Tampoco se contiene lo
  que lanza otro servicio a pedido: el programador de tareas, WMI o un
  servidor COM.
- **macOS y Linux sin espacios de usuario** quedan como hoy, más el gate
  ampliado.
- **Las listas de `Estado()` dependen de la versión de cada CLI.** Una
  versión que escribe en otro lugar falla dentro del encierro, o abre una vía
  nueva si el adaptador la declara escribible sin mirar.
- **bwrap es una dependencia nueva** fuera del binario de hoom. Sin él, el
  nivel es `ninguno`, y con `exigir` el writer no corre.
- **La primera build de cada tarea es más lenta** porque las cachés empiezan
  vacías.
- **El gate de objetos mira la fecha de cambio.** Un sistema de archivos sin
  ctime confiable lo debilita.
- **La telemetría local del checkout principal** (`.hoom/envelopes/`) queda
  protegida solo con encierro. Sin él sigue el riesgo que declaró
  `autonomia-entre-firmas`.
