# Spec: lo que corre código del writer corre encerrado

Estado: BORRADOR — pendiente de aprobación humana. Falta la firma de la
persona; nadie más la pone. Revisado contra el código de main (2cdb3c0) el
2026-11-13. Ese mismo día la persona contestó las ocho preguntas de la
versión anterior; sus respuestas están en el contrato y en Decisiones. Esas
respuestas no son la firma.
Tarea: `hoom task start contencion-del-writer` (rama
`hoom/contencion-del-writer`).
Origen: el No-goal que la persona agregó el 2026-11-13 a
`autonomia-entre-firmas.md` ("Contener a un writer que se propone burlar los
controles dejando código que corra después de su run") y dos de sus riesgos
("El guardado del piloto corre los hooks del repo", "El piloto decide sobre
telemetría local").
Depende de: `autonomia-entre-firmas.md`, en main desde los PR 82, 83 y 84:
el guardado del piloto (`itemcmd.SaveTarea`), el gate de material de git
(`agentcmd.RuleMaterialDeGit`, aclaración 25), los tests protegidos y
`boardcmd.ParadaAntesDeGuardar`.
Re-expresa: ningún criterio. El encierro se activa solo en un proyecto cuyo
`hoom.yaml` trae la sección `encierro` (lo decidió la persona el
2026-11-13), y ningún test firmado usa un proyecto con esa sección, que es
nueva. Eso incluye los de `autonomia-entre-firmas`, los del test-writer
ciego (`test-writer-en-arbol-ciego`, `ciego-sin-config-personal`), los de la
review (`review-aislada-y-modelo-elegido`, `evidencia-de-la-review`,
`hallazgos-y-refutador`) y los del sobre (`hoom-agent-sobre-determinista`).
`material-de-git` sigue siendo del writer, como lo firmó la aclaración 25.
Las reglas nuevas, que valen sin la sección, no cambian ningún resultado de
esos tests, leídos el 2026-11-13: sus CLIs falsos no tocan `refs/replace/`,
ni objetos que ya existían, ni la config de los agentes. El único test que
crea una ref de reemplazo (`internal/itemcmd/reemplazos_writer_test.go`) la
crea antes del guardado, y `git-ajeno` compara lo que cambia mientras corre.

## Objetivo

`autonomia-entre-firmas` contiene al writer que se pasa de celo: mira lo que
cambió el run y lo que commitea el guardado. No contiene al writer que se
propone burlar los controles, el que deja algo para que corra cuando hoom ya
no mira. Hoy ese writer tiene cuatro salidas, leídas en el código de main
(2cdb3c0):

1. **Procesos que sobreviven.** Cada cosa corre en su propio grupo de
   procesos: el run del provider con `procesos.Arrancar`, y los gates de
   verify y cada git del guardado con `procesos.ArrancarConGracia`. Al
   terminar, `procesos.Liberar` mata lo que quede en el grupo. El guardado
   además corta cada git a los `itemcmd.TopeGit`: SIGTERM al grupo y, después
   de `graciaGit`, SIGKILL. Pero un proceso que hace `setsid` sale del grupo y
   sigue vivo. También sigue vivo lo que el writer le pide a un daemon que ya
   corre: `systemd-run --user`, un servidor de tmux, docker.
2. **Escrituras fuera del árbol.** El gate de scope fotografía solo el árbol
   donde corre el rol (`agentcmd.Take(runDir, base)`). El de material de git
   fotografía cuatro piezas: la carpeta de hooks efectiva, el `config` común,
   el `config.worktree` y el `info/attributes` común. El run hereda el entorno
   y el `$HOME` de la persona (`append(os.Environ(), HOOM_TASK, HOOM_ROLE,
   HOOM_RUN, HOOM_PROVIDER)` en `runcmd`). Los gates heredan el entorno tal
   cual, y el git del guardado le suma lo suyo (`entornoGit`). Así, el writer
   puede escribir sin que lo vea ningún gate:
   - en `$HOME`: `~/.gitconfig`, `~/.bashrc`, `~/.local/bin/hoom`,
     `~/go/bin/golangci-lint`, `~/.config/go/env`, `~/.claude/settings.json`;
   - en el checkout principal: `hoom.yaml`, de donde el piloto lee la política
     `autonomia`; `.hoom/items/<slug>.yaml`, con su `presupuesto_usd`;
     `.hoom/envelopes/`;
   - en el worktree de otra tarea.
3. **El verify del sobre corre el código del writer después del gate.** En
   `agentcmd.Run` el gate de scope corre antes que `verifycmd.Run`, y los
   gates de verify corren los tests y el build del writer. Lo que ese código
   escriba fuera del árbol, o en el material de git, ya no lo mira nadie. Lo
   mismo pasa con el verify del paso `cerrar-hallazgos` del piloto, que corre
   en el proceso del piloto, sin sobre y sin una foto previa.
4. **Los hooks del guardado corren lo que el árbol les da.** Con husky,
   `core.hooksPath` es `.husky/_`, y cada hook de ahí llama a
   `.husky/<hook>`, que está en el árbol y que el writer puede escribir. El
   gate solo mira las entradas de la carpeta de hooks efectiva. Pasa lo mismo
   con `.pre-commit-config.yaml`, `lefthook.yml` o un `scripts/lint.sh` que el
   hook llame.

No se puede hacer una lista completa de lo que corre después: un hook llama a
un script, el script a `npm`, `npm` a `package.json`. Por eso este spec
encierra en vez de enumerar, y mantiene el gate para lo que sí se puede
fotografiar.

Con este spec, en un proyecto que pide el encierro, **lo que corre código
del writer bajo un sobre, la review o el piloto corre encerrado**:

- el run de cada rol que ejecuta (writer, test-writer, reviewer,
  characterizer y refutador), porque todos corren los tests del writer;
- los gates del verify que juzga ese código;
- el git del guardado.

Encerrado quiere decir que escribe solo en su tarea, no ve ni señala procesos
de afuera, no lee los secretos conocidos de la persona, y nada suyo sigue
vivo cuando termina. Donde el sistema no deja encerrar, el sobre lo dice y la
persona decide si el rol corre igual. El gate de material de git se amplía
para todos los proyectos, con encierro o sin él.

**Regla.** Lo que se puede encerrar se encierra; lo que no, se fotografía; y
lo que tampoco se puede fotografiar se declara.

## No-goals

- **Ocultar lecturas**, con una excepción. Lo encerrado sigue leyendo todo
  el disco de la persona, salvo los secretos conocidos: `~/.ssh`,
  `~/.gnupg`, `~/.config/gh`, `~/.aws` y `~/.docker/config.json`, que el
  encierro tapa (punto 2). La persona aceptó esa excepción el 2026-11-13.
  Las credenciales de las CLI, el resto de `$HOME` y lo que se lea sin
  encierro siguen a la vista (Riesgos).
- **La red.** Lo encerrado sigue con red porque la CLI la necesita. Llevarse un
  secreto o bajar algo queda fuera.
- **Lo que la persona corre a mano**: `hoom verify`, `POST /api/verify` del
  Studio, `hoom task done`, `hoom item save`, un `git commit` en el worktree,
  o abrir el árbol en un editor. La persona corre ese código sabiendo que es
  del writer.
- **Los modelos que escriben sin sobre**: `hoom run --task` y
  `POST /api/runs` con su `/input` del Studio siguen como hoy.
- **Inyectar instrucciones en archivos del árbol** (comentarios, `AGENTS.md`,
  `CLAUDE.md`) dirigidas a la review o a otro rol. No es código que corre.
- **El encierro en macOS y las escrituras en Windows.** Se evalúan en
  Decisiones y quedan para su propia spec (macOS lo decidió la persona el
  2026-11-13).
- **Los roles que no ejecutan** (`Exec: false`: analista, arquitecto,
  designer, scout y los demás) siguen como hoy: no corren código del writer.
- **Root, otra cuenta de usuario, contenedores o máquinas virtuales** para
  los roles que ejecutan.
- **Un tope de memoria o de procesos** (cgroups, `pids.max`).
- **Revalidar los sobres anteriores a este spec.**
- **Mostrar el nivel del encierro en la tarjeta o en el Studio.**
- **Endurecer la máquina** (sysctl, AppArmor, SELinux).

## Contratos

### Qué corre encerrado

Solo en un proyecto cuyo `hoom.yaml` trae la sección `encierro`, y solo para
un rol con tarea (`--task`). La persona decidió el 2026-11-13 que el encierro
se activa con la sección y no solo, y que los roles que ejecutan también se
encierran, en este spec. **Rol que ejecuta** es uno con `Exec: true` en
`internal/agents/targets.go`: writer, test-writer, reviewer, characterizer y
refutador. El orquestador también tiene `Exec: true`; ver "Preguntas para la
firma".

| Qué | Hoy | Con este spec |
|---|---|---|
| el run del writer o del characterizer, con tarea, lo lance `hoom agent`, `hoom tablero`, el Studio, `hoom pedir` o el piloto | su grupo de procesos | encierro; escribe en el árbol de la tarea |
| el run del test-writer, con tarea | su grupo, en su árbol ciego (`.hoom/isolated/<nombre>`, dentro del árbol de la tarea) | encierro; escribe solo en su árbol ciego, y el resto del árbol de la tarea queda de solo lectura |
| el run del refutador, con tarea | su grupo de procesos | encierro; escribe en el árbol de la tarea, donde `hoom finding resolve` deja su resolución |
| cada pasada del reviewer de `hoom review --task` (`reviewcmd.Run`), la lance la persona, el tablero o el piloto | su grupo de procesos | encierro; escribe en el árbol de la tarea, donde `hoom finding add` deja sus hallazgos |
| los gates del verify que corre el sobre de un rol que ejecuta, cuando lo corre | su grupo, con gracia | encierro, con la vista de ese rol y una caché propia del verify |
| los gates del verify del paso `cerrar-hallazgos` | su grupo, con gracia | encierro, con la vista del writer y una caché propia del verify |
| cada git del guardado del piloto: `SaveTarea` y `SaveTareaBajoCandado`, que pasan por `guardarTarea` y `delGuardado()`, con sus hooks y filtros | su grupo, con `TopeGit` | encierro, con `TopeGit`, la vista del writer y una caché propia del guardado |
| un rol que ejecuta sin `--task`, que corre en el checkout principal | — | como hoy, con nivel `ninguno` y motivo `sin tarea` |
| `hoom verify` de la persona, `POST /api/verify`, `hoom item save`, `hoom run` | — | como hoy |

Los gates del verify se encierran con espacios de usuario anidados
permitidos: lo decidió la persona el 2026-11-13 (punto 1).

Sin la sección `encierro`, nada de esto cambia. Solo aplica lo de "El gate
amplía" y las reglas nuevas.

### 1. Ningún proceso de lo encerrado sobrevive a lo que lo lanzó

**Por qué.** Un proceso vivo después del run hace lo que quiera cuando ya
pasó el gate: reescribe un test protegido después de la foto, espera al
guardado para tocar el índice, o se queda escuchando. El grupo de procesos de
hoy se puede dejar con un `setsid`, y un daemon que ya existe no es hijo de
nadie que hoom conozca.

En Linux, el encierro es un espacio de procesos propio:

- Cada cosa encerrada corre en su propio espacio de nombres de PID, de usuario
  y de montajes, armado con bubblewrap (`bwrap`). Adentro no se ven ni se
  pueden señalar los procesos de afuera: ni hoom, ni el Studio, ni la terminal
  de la persona.
- **Termina con su proceso principal.** Cuando el CLI, el `sh` del gate o el
  git termina, el encierro termina, y el kernel mata todo lo que quedó
  adentro, con `setsid` o con doble fork. Vale igual para un relevo, un
  reintento o una ronda.
- **La gracia de hoy se conserva.** bwrap no reenvía señales. Cuando vence un
  tope o hoom cancela:
  1. hoom manda SIGTERM al grupo del proceso principal adentro, usando el pid
     de afuera que bwrap informa (`--info-fd`);
  2. espera la gracia de hoy (`graciaGit` en el guardado, `graciaGate` en un
     gate, la del run para el provider);
  3. recién entonces termina el encierro.
  Así, git suelta su `index.lock` igual que hoy. Este punto no se probó
  todavía con bwrap 0.13.1: es lo primero que se prueba al implementar.
- **Muere con hoom** (`--die-with-parent`). Si muere el proceso de hoom que
  lo lanzó, aunque sea con SIGKILL, el encierro muere con él. Esto es nuevo:
  hoy, un SIGKILL a hoom deja vivos los grupos, porque el manejador solo
  atiende SIGINT, SIGTERM y SIGHUP.
- **Sin terminal.** Corre en una sesión nueva, sin terminal de control: no
  puede abrir `/dev/tty` ni escribirle a la terminal de la persona.
- **Sin privilegios nuevos.** `no_new_privs` hace que `crontab`, `at`, `sudo`,
  `su` y cualquier binario setuid no ganen nada.
- **Espacios de usuario anidados**: se permiten (lo decidió la persona el
  2026-11-13). El gate `test`
  de hoomai, y el sandbox propio de cada CLI, crean el suyo adentro. Un
  montaje que cruza a un espacio de usuario anidado queda bloqueado por el
  kernel (`MNT_LOCKED`), así que adentro no se puede volver escribible lo
  que era de solo lectura.
- **Sin los sockets que lanzan procesos.** El encierro tapa, con un directorio
  vacío y solo si existen:
  - `/run/user/<uid>`: el bus de systemd del usuario, la sesión gráfica, los
    agentes de ssh y gpg;
  - `/run/docker.sock` y `/run/podman/`;
  - en `/tmp`, `tmux-<uid>/`, `.X11-unix/`, `.ICE-unix/` y los `ssh-*` de la
    persona.
  Así quedan afuera `systemd-run --user`, tmux, docker y X11 por ruta.
- **Sin sockets abstractos de afuera.** Con Landlock ABI 6 o más nuevo (Linux
  6.12), un paso de hoom dentro del encierro, antes del CLI, le prohíbe
  conectarse a un socket abstracto de fuera de su dominio. Esa es la vía de
  X11 a la sesión gráfica de la persona. El paso es el mismo binario de hoom
  (`os.Executable`), que adentro se ve porque la raíz está montada. En los
  tests hace ese papel el binario del test, con un `TestMain`, como hace
  `cockpitcmd` con su `HoomBin`. `x/sys`, en vendor, ya trae las llamadas de
  Landlock.
- El tope de tiempo no cambia: `agents.<rol>.timeout`, `TopeGit` y el tope de
  cada gate.

En Windows, el Job Object de `procesos.Adoptar` no deja salir a un hijo y lo
mata al cerrarse: los procesos ya quedan contenidos, salvo lo declarado en
Riesgos. Si Windows no deja crear el job (un proceso que ya está en un job que
no anida), hoy `Adoptar` sigue sin avisar y cae a `taskkill /T`. Con este spec,
ese run queda en nivel `ninguno` con el motivo. En macOS no hay forma de
contener procesos con este spec (ver Decisiones).

### 2. Cada rol que ejecuta escribe solo en su tarea

**Por qué.** Todo lo que está fuera del árbol y corre después escapa a los
gates: la config global de git, el PATH de la persona, la config de las CLI,
los archivos de inicio del shell, la política del piloto en el checkout
principal. La lista no se puede cerrar. En cambio, lo que un rol que ejecuta
necesita escribir es corto. Los roles que no son el writer corren su código
igual: el test-writer y el characterizer corren los tests, la review y el
refutador también, para buscar o refutar un hallazgo. Algo escondido en un
`_test.go` corre en cada uno de ellos.

**La vista del encierro**, en Linux:

- **Lectura**: todo el sistema de archivos como lo ve la persona, de solo
  lectura, `/tmp` incluido, con un `/proc` y un `/dev` propios. Así las CLI,
  los repos y los scripts que viven en `/tmp`, como los de los tests en
  `t.TempDir()`, se siguen viendo.
- **Los secretos conocidos se tapan**, solo si existen: `~/.ssh`, `~/.gnupg`,
  `~/.config/gh` y `~/.aws` con un directorio vacío, y
  `~/.docker/config.json` con un archivo vacío, los dos de solo lectura. Es
  la excepción al primer No-goal que aceptó la persona el 2026-11-13. Las
  credenciales de las CLI (`~/.claude`, `~/.codex`) no se tapan: el CLI las
  necesita para correr.
- **Escritura**, solo en:
  1. **su árbol de run**: el árbol de la tarea (`.hoom/worktrees/<slug>`)
     para el writer, el characterizer, el reviewer y el refutador; su árbol
     ciego (`.hoom/isolated/<nombre>`, dentro del árbol de la tarea) para el
     test-writer, que no escribe en el resto del árbol de la tarea. Quedan de
     solo lectura el material de git y la config de los agentes que están
     adentro (punto 3) y el archivo `.git` del árbol;
  2. el directorio común de git, menos el material de git y los directorios de
     administración de los otros worktrees. El writer sigue pudiendo commitear
     en `hoom/<slug>` (ver "Git ajeno");
  3. **su caché**: `.hoom/cache/encierro/<slug>/<quien>/` del checkout
     principal, donde `<quien>` es el rol del run, `verify` para los gates del
     verify y `guardado` para el git del guardado. hoom la crea si no existe.
     Cada cosa encerrada escribe solo en la suya y no ve escribibles las de
     las otras: así, la caché que llena el writer (los resultados de
     `go test`, los módulos bajados) no la usa el verify que lo juzga, ni el
     test-writer, ni la review;
  4. lo que la CLI del provider necesita escribir, que declara su adaptador
     (ver "El estado del provider");
  5. lo que el proyecto agrega en `encierro.escribibles`.
- **Lo que no existe no se monta de solo lectura.** Una pieza del material o
  de la config de agentes que no existe al empezar (`shallow`,
  `config.worktree`, `.mcp.json`, un archivo de lefthook) se puede crear:
  crearla se detecta (punto 3), no se bloquea.
- **El entorno** es el de hoy, menos `SSH_AUTH_SOCK`, `GPG_AGENT_INFO`,
  `DBUS_SESSION_BUS_ADDRESS`, `DISPLAY`, `WAYLAND_DISPLAY` y
  `XDG_RUNTIME_DIR`, que apuntarían a lo tapado. Le agrega:
  - las variables de caché, todas dentro de su caché
    (`.hoom/cache/encierro/<slug>/<quien>/`):
    `TMPDIR` y `XDG_CACHE_HOME` siempre, más las que declara el perfil del
    proyecto. En el perfil go son `GOCACHE`, `GOMODCACHE` y `GOTMPDIR`. Cada
    perfil declara las suyas; con gradle, `GRADLE_USER_HOME` también tapa los
    scripts de `~/.gradle/init.d/`;
  - la config de git del entorno `gc.auto=0`, `maintenance.auto=false` y
    `core.fsmonitor=false` (por `GIT_CONFIG_COUNT`), como ya hace
    `delGuardado()`. Así, un `git commit` del writer no deja una tarea de
    mantenimiento corriendo en segundo plano: git 2.47 la lanza con
    `--detach`, y moriría a medias al terminar el encierro.

**Qué se bloquea, qué se detecta y qué no se puede.** "Sin encierro" es
macOS, Windows en lo que toca a escrituras, Linux sin bwrap, un rol sin
tarea, o un proyecto sin la sección `encierro`.

| Destino | Ejemplos | Linux, encierro completo | Sin encierro |
|---|---|---|---|
| `$HOME` | `~/.gitconfig`, `~/.bashrc`, `~/.profile`, `~/.local/bin/hoom`, `~/go/bin/golangci-lint`, `~/.config/go/env`, `~/.config/systemd/user/`, `~/.ssh/config` | se bloquea | se detecta solo la config de git que lee el árbol (punto 3); el resto no se ve |
| la config de las CLI | `hooks` en `~/.claude/settings.json`, `mcpServers` en `~/.claude.json`, `~/.codex/config.toml` | se bloquea, salvo lo que el adaptador declara escribible; lo que hace correr código se detecta | se detecta (`config-de-agentes`) |
| el checkout principal | `hoom.yaml`, `.hoom/items/<slug>.yaml`, `.hoom/envelopes/`, `.hoom/approvals/` | se bloquea | no se ve, como hoy |
| otros worktrees | `.hoom/worktrees/<otra>/` | se bloquea | no se ve |
| el árbol de la tarea, para el test-writer | la implementación, fuera de su árbol ciego | se bloquea | no se ve |
| la caché de otro rol o del verify | `.hoom/cache/encierro/<slug>/verify/` desde el run del writer | se bloquea | no hay caché propia: todos usan la de la persona |
| los secretos conocidos (lectura) | `~/.ssh`, `~/.gnupg`, `~/.config/gh`, `~/.aws`, `~/.docker/config.json` | se tapan: se ven vacíos | se leen |
| el material de git que existe | la carpeta de hooks, `config`, `info/` (`attributes`, `exclude`), `objects/info/alternates`, `objects/info/grafts`, el `.git` del árbol | se bloquea | se detecta |
| el material de git que no existe | `shallow`, `config.worktree` | se detecta | se detecta |
| el resto del directorio común | `refs/replace/`, objetos que ya existían | se detecta (`git-ajeno`) | se detecta |
| otras refs | mover `main` u otra rama | no se ve (Riesgos) | no se ve |
| `/tmp` | sockets de tmux, X11 y ssh | se bloquea | no se ve |
| daemons y setuid | `systemd-run --user`, tmux, docker, `crontab`, `at` | se bloquea | no se ve |
| un socket del sistema que el encierro no tapa | un servicio que hace algo a pedido | no se puede (Riesgos) | no se puede |
| la red | subir algo a un servidor | no se puede (No-goals) | no se puede |

**Git ajeno.** El directorio común de git queda escribible porque el writer
commitea y los hooks del guardado escriben refs y objetos: lint-staged, por
ejemplo, usa `git stash`. Comparar todas las refs daría falsos positivos,
porque mientras corre el writer también las mueven, con razón, otras tareas,
`hoom task start`, `isolate.Open`, la persona, un `git fetch` del writer y el
mantenimiento de git. Por eso la regla nueva `git-ajeno` mira solo lo que
nadie cambia con razón. Lo decidió la persona el 2026-11-13; que el writer
mueva otra rama queda en Riesgos.

- **`refs/replace/`**: una ref creada, movida o borrada ahí, suelta o
  empaquetada. Una ref de reemplazo cambia lo que git muestra de un objeto en
  todas las ramas;
- **un objeto reescrito**: un objeto suelto, un pack o su índice que ya
  existía y sigue existiendo con otro contenido. hoom revisa solo los
  archivos con fecha de cambio posterior al arranque. Borrar un objeto
  (`gc`, `prune`) o crear uno nuevo no cuenta, porque el nombre de un objeto
  y el de un pack salen de su contenido.
- hoom compara después del run de cada rol que ejecuta, alrededor de cada
  pasada de la review y alrededor de cada guardado.
- En el sobre, `git-ajeno` corta como `material-de-git`: cierra en `scope`,
  `no-entregable`, exit 1, sin verify, con la nota
  `el <rol> cambio git fuera de la rama de su tarea: no se emite veredicto sobre este arbol`
  y un hallazgo high del gate de scope con el detalle
  `un rol solo cambia la rama de su tarea: las refs de reemplazo y los objetos que ya existian no los toca`.
- En la review, la pasada que la comete hace fallar `hoom review` con exit 1,
  sin escribir registro, como el árbol sucio de hoy, con el error
  `la pasada <n> del reviewer cambio git fuera de la rama de su tarea: <ruta>: no registro la review`.
  Los hallazgos que esa pasada haya dejado quedan en el árbol: la persona
  decide qué hacer con ellos.
- En el guardado, falla y deshace su commit como hoy, con el error
  `el guardado cambio git fuera de hoom/<slug>: <ruta>: no guardo`. hoom no
  restaura nada: eso lo decide la persona.
- `git-ajeno` aplica con encierro o sin él, y en cualquier proyecto, a todo
  rol que ejecuta.

### 3. Lo que queda para correr después: el gate amplía, no se reemplaza

**Por qué.** El encierro bloquea, pero no existe en todas partes ni en todos
los kernels, ni puede bloquear lo que todavía no existe. Además, un bloqueo es
silencioso: el writer recibe un error del sistema y nadie se entera de que lo
intentó. El gate funciona en cualquier sistema y deja un hallazgo con la ruta.
Por eso se quedan los dos: el encierro donde se pide y se puede, y el gate
siempre. Con encierro, el gate casi nunca va a ver un cambio del material que
existe, porque el encierro lo bloquea antes; su trabajo pasa a ser lo que no
existía y los sistemas sin encierro. El gate gana más material, más momentos,
un orden y una regla nueva.

**Más material.** `FotoMaterialGit` suma, a las cuatro piezas de hoy:

- **la config de git que lee el árbol fuera del repo**: cada archivo que
  `git config --list --show-origin` nombra como origen en el árbol de la
  tarea (`~/.gitconfig`, `$XDG_CONFIG_HOME/git/config` y lo que alcanza un
  `include.path` o un `includeIf`), el archivo de `core.attributesFile` y
  `$XDG_CONFIG_HOME/git/attributes`. `/etc/gitconfig` no entra: la persona
  tampoco lo escribe sin privilegios;
- del directorio común: `info/exclude`, `objects/info/alternates`,
  `objects/info/grafts` y `shallow`. No entra el resto de `objects/info/`: el
  mantenimiento de git escribe ahí `commit-graphs` después de cada commit;
- el archivo `.git` del árbol, que dice dónde está su repo;
- **los gestores de hooks del árbol**:
  - si la carpeta de hooks efectiva está dentro del árbol, las entradas que
    están directamente en su carpeta padre (husky llama desde `.husky/_/` a
    `.husky/<hook>`);
  - siempre, `.pre-commit-config.yaml`, `lefthook.yml`, `lefthook.yaml`,
    `.lefthook.yml`, `.lefthook.yaml` y `lefthook-local.yml` de la raíz del
    árbol, y la carpeta `.lefthook/`.
- Las piezas nuevas usan las mismas huellas y el mismo tope de foto que las de
  hoy (aclaraciones 28 y 29), y cuentan como `material-de-git`.

`info/exclude` entra porque `Take` lista lo no rastreado con
`--exclude-standard`. Un `zz_test.go` excluido ahí no lo vería el gate ni lo
commitearía el guardado, pero verify lo correría.

**El orden después del run.** Después del run, el sobre de un rol que
ejecuta compara la config de agentes, y en el del writer también el material,
**antes** de correr cualquier otro git suyo sobre el árbol (`Take`,
`gitx.TouchedErr`). Si cambiaron, corta sin correr `git status`. Así, un
fsmonitor o un filtro que dejó el código del writer no corre dentro de hoom
antes de que el gate lo vea. Además, todo git de hoom sobre el árbol de un rol
que ejecuta (`Take`, el `gitx.Snapshot` de verify, check, la evidencia de la
review) lleva `-c core.fsmonitor=false`.

**Más momentos.** Hoy el material se fotografía antes del run y se compara
después. Se suman tres comparaciones:

- **Dentro del verify del sobre y del de `cerrar-hallazgos`.**
  - `verifycmd.Run` gana una opción que solo usan esos dos: el verify que
    corre el sobre de un rol que ejecuta, y el de `cerrar-hallazgos`. Con
    ella, los gates corren encerrados, con la vista del rol del sobre (la del
    writer en `cerrar-hallazgos`) y la caché del verify, y el material se
    fotografía antes y después de los gates. `hoom verify` y
    `POST /api/verify` no la usan.
  - Si el material cambió, el veredicto suma el gate `material_de_git` en
    `fail`, con las rutas en su salida: queda rojo, y ningún lector de
    veredictos (`verdict.LatestComplete`, `hoom task done`, check, review) lo
    toma como verde.
  - En el sobre del writer, además, queda el hallazgo high de siempre de
    `material-de-git`, con la ruta, y el sobre cierra en `verify`. Así, el
    piloto se detiene antes del próximo guardado, como hoy
    (`ParadaAntesDeGuardar`). En el sobre de otro rol, el veredicto rojo lo
    cierra como lo cierra hoy un veredicto rojo.
  - En `cerrar-hallazgos`, el piloto se detiene con su texto de veredicto
    rojo de siempre, que nombra `material_de_git`.
  - Sin la sección `encierro`, la opción igual fotografía el material, pero
    los gates corren como hoy.
- **Alrededor del guardado.**
  - `guardarTarea` fotografía el material antes de su primer git y después
    del último. Si cambió, falla, deshace su commit y devuelve
    `un hook del guardado cambio el material de git: <ruta>: no guardo`.
  - Antes de empezar, el guardado se niega si el último veredicto del árbol
    tiene `material_de_git` en `fail`:
    `el ultimo veredicto de la tarea encontro el material de git cambiado: no guardo`.
- Los gates del verify del sobre cuyo nombre está en
  `encierro.gates_sin_encierro` corren como hoy, pero la foto de antes y
  después los incluye.

**`material-de-git` sigue siendo del writer.** La aclaración 25 la firmó para
un rol de forma `codigo`, y este spec no la re-expresa. Para los otros roles
que ejecutan, el encierro deja de solo lectura el material que existe, y lo
que creen en el árbol fuera de su alcance lo corta su gate de scope, como hoy.
Lo que queda sin ver está en Riesgos.

**La config de los agentes.** Regla nueva, `config-de-agentes`, para todo rol
que ejecuta, con encierro o sin él. En el sobre corta como `material-de-git`,
con la nota
`el <rol> cambio la config de los agentes: no se emite veredicto sobre este arbol`
y el detalle
`la config de los agentes hace correr codigo en la proxima sesion: ningun rol que ejecuta la cambia`.
En la review, hoom compara alrededor de cada pasada, y la pasada que la
comete hace fallar `hoom review` como `git-ajeno`, con el error
`la pasada <n> del reviewer cambio la config de los agentes: <ruta>: no registro la review`.
Cubre:

- **en el árbol**: `.claude/`, `.codex/`, `.mcp.json` y lo que declare cada
  adaptador. Un hook en `.claude/settings.json` correría en la próxima sesión
  de Claude en ese árbol: el reintento, el relevo, o el test-writer ciego si
  el archivo llega a su árbol;
- **fuera del árbol**:
  - claude: las claves `hooks` de `~/.claude/settings.json` y de
    `~/.claude/settings.local.json`, y `mcpServers` de `~/.claude.json`,
    global y de cada proyecto. Son JSON y se leen con la biblioteca estándar.
    Que claude cambie otras claves de esos archivos, como sus contadores o su
    historial, no es violación;
  - codex: el archivo `~/.codex/config.toml` entero. hoom no tiene un parser
    de TOML (go.mod trae `x/sys`, `x/term` y `yaml.v3`), y codex no debería
    escribir ese archivo en un run: lo confirma la sonda de "El estado del
    provider". Si lo escribe, hace falta un parser de TOML en vendor, que es
    una dependencia nueva y vuelve a la persona.
- `agents.<rol>.write` de `hoom.yaml` no levanta ninguna de las dos reglas.

**El piloto no releva ni guarda tras una regla nueva.** `git-ajeno` y
`config-de-agentes` cuentan como `material-de-git` para el piloto. Un sobre
cortado por una de ellas no se releva aunque traiga `corte: tiempo`, no se
reintenta, y detiene al piloto antes del próximo guardado
(`ParadaAntesDeGuardar`, caso 8). Hoy el relevo solo mira los tests protegidos
(`piloto.go`), y la parada solo el material de git (`cortesDelGate`).

**El guardado encerrado.** Cada git de `guardarTarea` corre encerrado, con la
vista del writer, con `TopeGit` y con la gracia de `graciaGit`. Un hook, o el
script que llama, puede hacer en el árbol y en git lo mismo que el writer en
su run, y el guardado lo revisa como hoy (`revisarCommit`, aclaraciones 24 a
30), más el material y `git-ajeno`. El git del guardado lleva además
`-c commit.gpgsign=false`: adentro no hay agente de gpg ni de ssh para firmar.
La persona decidió el 2026-11-13 que el guardado corra sin firma, en vez de
fallar en un repo que firma: la persona firma al integrar.

### El nivel del encierro

- Cada sobre de un rol que ejecuta y cada review, en un proyecto con la
  sección `encierro`, calcula un **nivel**, con su **mecanismo** y lo que
  **falta** (`procesos`, `escrituras`, `ipc`):

  | Sistema | Nivel | Mecanismo | Falta |
  |---|---|---|---|
  | Linux con bwrap, espacios de usuario sin privilegios y Landlock ABI 6 o más | `completo` | `bwrap` | — |
  | Linux con bwrap y espacios de usuario, sin Landlock ABI 6 | `parcial` | `bwrap` | `ipc` |
  | Linux sin bwrap, o con bwrap que no puede crear el espacio de usuario | `ninguno` | — | `procesos`, `escrituras`, `ipc` |
  | Windows, con el job creado | `parcial` | `job` | `escrituras`, `ipc` |
  | Windows, sin job | `ninguno` | — | `procesos`, `escrituras`, `ipc` |
  | macOS | `ninguno` | — | `procesos`, `escrituras`, `ipc` |
  | un rol que ejecuta sin `--task`, en cualquier sistema | `ninguno` | — | `procesos`, `escrituras`, `ipc` |
  | un provider que no declara su estado | `ninguno` | — | `procesos`, `escrituras`, `ipc` |

- En Linux, hoom lo averigua una vez por proceso con una sonda: corre bwrap con
  la vista mínima y un comando que termina, y pregunta a Landlock su ABI. Si
  la sonda falla, el motivo queda escrito: `bwrap no esta`, la salida de bwrap
  (por ejemplo, el rechazo de AppArmor en Ubuntu 24.04), `landlock ABI <n>`,
  `sin tarea` o `<provider> no declara su estado`. En Windows el nivel se sabe
  al arrancar el run, cuando `Adoptar` crea o no el job.
- **El registro del sobre**, el registro de la review (`.hoom/reviews/`) y el
  sidecar de cada run ganan `encierro: {nivel, mecanismo, falta, motivo}`.
  Con `completo`, `falta` es la lista vacía y `motivo` no va. En un proyecto
  sin la sección, o en el sobre de un rol que no ejecuta, la clave no va.
- Con nivel menor que `completo`, el log del sobre, o la salida de la review,
  suma
  `aviso: encierro <nivel> (falta: <lista>): lo que el <rol> deje fuera de su arbol no se ve`.

### La política `encierro`

Sección nueva y opcional de `hoom.yaml`, leída del checkout principal como
`autonomia`. Su presencia activa el encierro: sin ella, nada se encierra
(lo decidió la persona el 2026-11-13):

```yaml
encierro:
  exigir: completo          # completo, parcial o no (no por defecto)
  escribibles:              # rutas fuera del arbol que el encierro deja escribir
    - ~/.semgrep
  gates_sin_encierro:       # gates del verify que corren como hoy
    - e2e
```

- **`exigir`**, por defecto `no`: lo decidió la persona el 2026-11-13, para
  que un `hoom.yaml` compartido con quien trabaja en una Mac no deje de correr
  ahí. Si el nivel es menor:
  - el sobre de un rol que ejecuta se niega **dentro del paso `run`**, antes
    de lanzar, con exit 1, sin run ni veredicto, y la nota
    `el encierro de este sistema es <nivel> (falta: <lista>) y hoom.yaml exige <exigir>: el <rol> no corre`.
    Los pasos siguen siendo los de siempre (cinco, seis con `aislar` para el
    test-writer): no se agrega uno, así que no cambia el criterio de los
    pasos de `hoom-agent-sobre-determinista` ni el texto del tablero
    (`stageGloss`). Para el piloto es un cierre no entregable, su caso 8;
  - la review se niega antes de su primera pasada, con exit 1, sin registro,
    con la misma nota;
  - el guardado se niega con
    `el encierro de este sistema es <nivel> y hoom.yaml exige <exigir>: no guardo`,
    y el piloto se detiene con su texto de "no pudo guardar";
  - el verify de `cerrar-hallazgos` no corre, y el piloto se detiene con el
    mismo motivo.
  Con `no`, el valor por defecto, todo corre con el nivel que haya y el aviso
  queda en el log.
- **`escribibles`**: cada ruta es absoluta o empieza con `~/`.
- **`gates_sin_encierro`**: nombres de gates. Esos gates corren como hoy,
  también en el verify del sobre. Es la salida para un gate que necesita
  docker, y debilita el encierro: el README lo dice.
- **Al leer `hoom.yaml`** (`LoadRoles`, como `autonomia`), sin correr git,
  hay estos errores, todos con el prefijo `hoom.yaml invalido: `:
  - `encierro: clave desconocida %q (validas: exigir, escribibles, gates_sin_encierro)`;
  - `encierro: exigir tiene que ser completo, parcial o no`;
  - `encierro: escribibles: %q no es una ruta absoluta ni empieza con ~/`, para
    una ruta relativa o vacía. En YAML, `- ~` es nulo y cae acá;
  - `encierro: escribibles: %q es todo el disco o todo el directorio personal`,
    para `/` y `~/`.
- **Al empezar el sobre**, con el árbol y el repo a mano, el sobre se niega en
  el paso `run`, sin lanzar, con:
  - `encierro: escribibles: <ruta> contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible`;
  - `encierro: gates_sin_encierro: el gate <g> no esta en los gates de este arbol`.
    Se valida contra los gates ya mezclados con el perfil, del `hoom.yaml` del
    árbol, que es el que corre verify.
- Las validaciones de "Al empezar el sobre" valen igual al empezar la review,
  que se niega antes de su primera pasada, con exit 1 y sin registro.
- Con encierro, el `hoom.yaml` del checkout principal es de solo lectura para
  todo lo encerrado. Sin encierro, el writer puede cambiar esta sección como puede
  cambiar `autonomia` (Riesgos). El piso del gate de scope cubre el
  `hoom.yaml` del árbol, no el del checkout principal.

### El estado del provider

- Interfaz nueva y opcional, `providers.ConEstado`, con `Estado()`. No se suma
  a `Provider`, para no romper los proveedores falsos de los tests:
  - `Escribe`: las rutas fuera del árbol que su CLI escribe (sesiones, logs,
    credenciales que se refrescan);
  - `EnArbol`: su config dentro del árbol;
  - `Corre`: cómo leer, de cada archivo de config, solo lo que hace correr
    código.
- La implementan claude y codex. gemini y opencode no corren roles
  (`pickProvider` exige system prompt), así que no la necesitan.
- El encierro deja escribibles solo las rutas de `Escribe`. `EnArbol` y
  `Corre` alimentan `config-de-agentes`, con encierro o sin él.
- Las listas de claude y codex las fija una sonda con la versión real de cada
  CLI, que corre una vez antes de implementar y queda escrita en el comentario
  de cada adaptador. Por ejemplo, claude escribe `~/.claude.json` en cada
  arranque, y la sonda dice si el encierro puede dejarlo de solo lectura.
- `~/.codex/sessions/`, de donde hoom lee las ventanas de 5 h que usa el piloto
  (`providers/medicion.go`), va a estar seguramente en `Escribe` de codex
  (Riesgos).

### Lo que queda escrito

- El registro del sobre y el de la review ganan `encierro`.
- Las reglas nuevas `git-ajeno` y `config-de-agentes` registran su hallazgo con
  el formato de siempre del gate de scope, y tienen sus funciones
  `DeGitAjeno` y `DeConfigDeAgentes`, como `DeMaterialDeGit`.
- `material-de-git` conserva su nombre, su nota y su detalle.
- El veredicto puede traer el gate `material_de_git`.
- Líneas nuevas en el log del sobre, dentro del paso `run`:
  - `encierro <nivel> con <mecanismo>`, o `sin encierro: <motivo>`;
  - el aviso de nivel menor que `completo`.

### Documentación y pruebas

- El README documenta el encierro (qué corre encerrado y qué no), los niveles,
  la sección `encierro` y que `gates_sin_encierro` lo debilita. Sin tildes,
  como el resto de sus tablas.
- Los tests usan los CLIs falsos de siempre, en `t.TempDir()`, que con `/tmp`
  de solo lectura se siguen viendo. También usan un provider falso que
  implementa `ConEstado`. Ninguno llama a un CLI real.
- Los secretos se prueban con un `$HOME` del test que tiene `~/.ssh`,
  `~/.gnupg`, `~/.config/gh`, `~/.aws` y `~/.docker/config.json` con
  contenido: el CLI falso los ve vacíos y lee el resto de ese `$HOME`.
- Los tests del encierro son solo de Linux (etiqueta de compilación), porque el
  mantenedor también corre los tests en una Mac.
- **El CI** (`.github/workflows/ci.yml`, `ubuntu-latest`) hoy no instala
  bubblewrap. Este spec le suma instalarlo y habilitar los espacios de usuario
  sin privilegios (en Ubuntu 24.04,
  `sysctl kernel.apparmor_restrict_unprivileged_userns=0`).
  - Un test del encierro que no encuentra bwrap falla: no se salta.
  - Si el kernel del runner es anterior a 6.12, el test del socket abstracto
    comprueba que el nivel es `parcial` con falta `ipc`, en vez de conectar.
    Lo que no puede probar queda dicho en el test.
- **Lo que un runner sin terminal ni sesión no tiene**, el test lo prueba de
  otra forma:
  - que el proceso no tiene terminal de control: `tty_nr` en 0 en
    `/proc/self/stat`, y es líder de su sesión;
  - que la vista tapa `/run/user/<uid>` cuando existe: el test crea un
    directorio que hace de `/run/user/<uid>` (la ruta es inyectable en los
    tests) y comprueba que adentro se ve vacío.
- La sonda de nivel se puede reemplazar en los tests, para probar `parcial` y
  `ninguno` en una máquina que tiene `completo`.

## Casos límite y errores esperados

- **El writer corre `hoom verify` adentro**, como pide su contrato: su
  veredicto queda en el árbol de la tarea, como hoy (el manifest es el del
  árbol). Ese verify no tiene la opción de encierro: sus gates corren dentro
  del encierro del run. Si ese hoom necesita escribir fuera del árbol, falla
  con el error del sistema. El veredicto que cuenta es el que corre el sobre.
- **El writer commitea** en `hoom/<slug>`: funciona. Crear una ref de
  reemplazo o reescribir un objeto es `git-ajeno`. Mover `main` no se ve
  (Riesgos).
- **El writer necesita una dependencia nueva**: la baja a su caché. La
  primera vez, cada tarea arranca con las cachés vacías, y el verify, el
  test-writer y la review la vuelven a bajar a la suya: tarda más.
- **El test-writer intenta escribir en la implementación** (el árbol de la
  tarea fuera de su árbol ciego): falla dentro del encierro. Hoy no lo ve el
  gate de scope, que fotografía solo el árbol ciego.
- **El reviewer registra un hallazgo** con `hoom finding add`: ese hoom corre
  dentro del encierro de la pasada y escribe `.hoom/findings/` del árbol de
  la tarea, que es su árbol de run. Funciona igual que hoy. El refutador, con
  `hoom finding resolve`, igual.
- **Una review sin `--task`**: revisa el checkout principal, sin encierro y
  con el motivo `sin tarea`. Con `exigir`, no corre.
- **Un gate o un hook que usa las credenciales de `gh`, de ssh o de la nube**
  (bajar un módulo privado, `aws s3 cp`) falla encerrado, porque esos
  secretos se ven vacíos. Va en `gates_sin_encierro`, o la persona guarda con
  `hoom item save`.
- **Un gate que escribe en `$HOME`** (semgrep escribe `~/.semgrep/`) falla
  encerrado. La persona agrega la ruta a `escribibles` o el gate a
  `gates_sin_encierro`.
- **Un gate con docker** falla encerrado porque no ve el socket. Va en
  `gates_sin_encierro`, con lo que eso debilita.
- **Los hooks y servidores MCP personales de claude** corren dentro del
  encierro del run, y lo que escriben en `$HOME` fuera de `Escribe` falla.
- **Un hook del guardado que firma, empuja o habla con un agente** (gpg, ssh,
  `git push` por ssh) falla encerrado, el guardado falla y el piloto se detiene
  con `no pudo guardar el trabajo de la tarjeta: <error>`. La persona guarda
  con `hoom item save`, que no corre encerrado.
- **Un repo con `commit.gpgsign=true`**: los guardados del piloto quedan sin
  firma. La persona firma al integrar (lo decidió el 2026-11-13).
- **lint-staged o pre-commit en el guardado**: escriben `refs/stash` y la
  restauran. No es `git-ajeno`, que no mira esa ref.
- **Una tarea que necesita cambiar `.husky/`, `.claude/` o
  `.pre-commit-config.yaml`**: el writer no puede. Lo cambia la persona, como
  el spec y `hoom.yaml`.
- **Un `include.path` que apunta adentro del árbol**: ese archivo pasa a ser
  material de git y el writer no lo cambia.
- **El writer mata al CLI desde adentro**: el encierro termina con el CLI y el
  sobre cierra como un run que terminó mal. No puede matar a hoom: no lo ve.
- **El writer agota el tope dejando un proceso con `setsid`**: cuando vence el
  tope, el grupo principal recibe SIGTERM, pasa la gracia, y el encierro
  termina con todo lo que tiene adentro.
- **La persona cambia sus hooks mientras corre un guardado**: el guardado ve el
  material cambiado y falla. La persona lo vuelve a pedir.
- **Dos sobres de writer de tareas distintas a la vez**: cada uno tiene su
  encierro y su caché, y cada uno mueve su rama. `git-ajeno` no mira las
  ramas, así que no se cruzan.
- **Un rol que ejecuta sin `--task`**: corre en el checkout principal, sin
  encierro y con el motivo `sin tarea`. Con `exigir`, no corre.
- **Linux con bwrap pero sin espacios de usuario** (Ubuntu 24.04 sin el perfil
  de AppArmor): nivel `ninguno`, con la salida de bwrap como motivo. Con
  `exigir: no`, el rol corre como hoy, con el aviso.
- **El verify del sobre sobre hoomai mismo**: el gate `test`
  (`go test -race ./...`) corre los tests del encierro adentro del encierro
  del verify, y esos tests crean su propio espacio de usuario anidado. Por eso
  hacen falta los espacios anidados, que la persona aceptó el 2026-11-13.
- **Un proyecto sin la sección `encierro`**: todo corre como hoy, salvo el gate
  ampliado, `git-ajeno` y `config-de-agentes`.
- **Un sobre anterior a este spec**: no lleva `encierro`, y nada lo revalida.

## Criterios de aceptación

- CA-674: en un proyecto con la sección `encierro`, el registro del sobre de un rol que ejecuta (writer, test-writer, reviewer, characterizer o refutador), el registro de la review y el sidecar de cada run llevan `encierro` con `nivel` (`completo`, `parcial` o `ninguno`), `mecanismo`, `falta` (vacía con `completo`) y `motivo` (ausente con `completo`); sin la sección, o en el sobre de un rol que no ejecuta, no llevan la clave; con nivel menor que `completo`, el log del sobre o la salida de la review trae la línea de aviso del contrato con el rol.
- CA-675: el `hoom.yaml` del proyecto acepta la sección `encierro` con `exigir`, `escribibles` y `gates_sin_encierro`; sin la sección no hay encierro, y sin `exigir` vale `no`; `LoadRoles` rechaza con el texto del contrato, sin correr git, una clave desconocida, un `exigir` fuera de la lista, una ruta escribible relativa, vacía o nula, y `/` o `~/`; al empezar el sobre, una ruta escribible que contiene o está dentro del checkout principal o del directorio común de git, o un gate de `gates_sin_encierro` que no está en los gates del árbol, cierra el sobre en `run` con exit 1, sin run.
- CA-676: con un nivel menor que `exigir`, el sobre de un rol que ejecuta cierra en `run` con exit 1 y la nota `el encierro de este sistema es <nivel> (falta: <lista>) y hoom.yaml exige <exigir>: el <rol> no corre`, sin ningún run en `.hoom/runs/`, sin veredicto nuevo y con los pasos de siempre; la review se niega antes de su primera pasada con la misma nota, exit 1 y sin registro; el guardado se niega con su texto del contrato y el verify de `cerrar-hallazgos` no corre; con `exigir: no`, todo corre con el nivel que haya.
- CA-677: el nivel sale de la tabla del contrato para cada combinación de sistema y resultado de la sonda (bwrap ausente, espacio de usuario rechazado, Landlock por debajo de ABI 6, todo disponible), con su mecanismo, lo que falta y el motivo; un rol que ejecuta sin `--task` y un provider que no implementa `ConEstado` dan `ninguno` con su motivo; en macOS es `ninguno`, y en Windows `parcial` con mecanismo `job` solo si el job se creó.
- CA-678: en Linux con encierro, un CLI falso que vive en `t.TempDir()` corre, y lo que intenta escribir en `$HOME`, en `hoom.yaml` y en `.hoom/items/<slug>.yaml` del checkout principal, en el worktree de otra tarea o en `/tmp` fuera de su árbol no deja nada en el disco; lo que escribe en el árbol de la tarea, en `.hoom/cache/encierro/<slug>/`, en una ruta de `Escribe` de su provider y en una de `escribibles` queda.
- CA-679: en Linux con encierro, un CLI falso de cualquier rol que ejecuta que deja un proceso con `setsid` y otro con doble fork, y termina, no deja ninguno vivo cuando cierra el sobre o la pasada de la review; tampoco cuando el run muere por su timeout, ni cuando hoom cancela el run; al vencer un tope, el grupo principal recibe SIGTERM y tiene la gracia de hoy antes de que termine el encierro.
- CA-680: en Linux con encierro, el CLI falso de cualquier rol que ejecuta no ve el pid de hoom ni puede señalarlo, no tiene terminal de control (`tty_nr` 0 y es líder de su sesión), no recibe `SSH_AUTH_SOCK`, `GPG_AGENT_INFO`, `DBUS_SESSION_BUS_ADDRESS`, `DISPLAY`, `WAYLAND_DISPLAY` ni `XDG_RUNTIME_DIR`, ve vacío el directorio que hace de `/run/user/<uid>` cuando existe, recibe `gc.auto=0`, `maintenance.auto=false` y `core.fsmonitor=false` en la config de git del entorno, y con Landlock ABI 6 o más no se conecta a un socket abstracto que escucha fuera del encierro; con un ABI menor, el nivel es `parcial` con falta `ipc`.
- CA-681: en Linux con encierro, escribir en la carpeta de hooks efectiva, en el `config` común, en `info/attributes`, en `info/exclude`, en `objects/info/alternates` o en el archivo `.git` del árbol falla dentro del encierro, cuando existen; un `git commit` del writer en `hoom/<slug>` funciona; y crear una pieza del material que no existía es la violación `material-de-git`.
- CA-682: todo lo encerrado recibe `TMPDIR` y `XDG_CACHE_HOME` dentro de su caché, `.hoom/cache/encierro/<slug>/<quien>/` (el rol del run, `verify` o `guardado`), que hoom crea si no existe, y las variables de caché de su perfil (`GOCACHE`, `GOMODCACHE` y `GOTMPDIR` en el perfil go) dentro del mismo directorio; lo que el run del writer intenta escribir en la caché `verify`, en la de otro rol o en la `guardado` no deja nada en el disco.
- CA-683: con la opción nueva de `verifycmd.Run`, que usan el sobre de un rol que ejecuta y `cerrar-hallazgos`, los gates corren encerrados con la vista del rol del sobre (la del writer en `cerrar-hallazgos`) y la caché `verify`: un gate falso que escribe en `$HOME` no deja nada afuera, y uno que deja un proceso con `setsid` no lo deja vivo; un gate de `gates_sin_encierro` corre como hoy; `hoom verify` de la persona y `POST /api/verify` no usan la opción y corren como hoy.
- CA-684: cada git de `guardarTarea` corre encerrado, con `TopeGit`, la gracia de `graciaGit` y `commit.gpgsign=false`: un hook que escribe en `$HOME` o deja un proceso con `setsid` no deja nada afuera ni vivo; un hook que pasa el tope recibe SIGTERM, tiene su gracia, termina con todo lo que lanzó, no queda `index.lock`, y el guardado falla como hoy; `hoom item save` no corre encerrado.
- CA-685: después del run de cada rol que ejecuta, alrededor de cada pasada de la review y alrededor de cada guardado, una ref creada, movida o borrada bajo `refs/replace/` (suelta o empaquetada), o un objeto suelto, un pack o su índice que ya existía y sigue existiendo con otro contenido, es la violación `git-ajeno`: el sobre cierra en `scope`, `no-entregable`, exit 1, sin verify, con la nota (que nombra el rol) y un hallazgo high con el detalle del contrato; la review falla con exit 1, sin registro y con su error del contrato; el guardado falla, deshace su commit y devuelve `el guardado cambio git fuera de hoom/<slug>: <ruta>: no guardo`; vale igual sin encierro; mover otra rama, `refs/stash`, crear objetos nuevos o borrarlos (`gc`) no es violación.
- CA-686: `FotoMaterialGit` incluye cada archivo que `git config --list --show-origin` nombra en el árbol fuera de `/etc`, `core.attributesFile`, `$XDG_CONFIG_HOME/git/attributes`, `info/exclude`, `objects/info/alternates`, `objects/info/grafts`, `shallow`, el archivo `.git` del árbol, las entradas directas de la carpeta padre de la carpeta de hooks efectiva cuando está dentro del árbol, y los archivos de pre-commit y lefthook del contrato, y no incluye `objects/info/commit-graphs`; un writer que cambia, crea o borra cualquiera de ellos, sin encierro, comete la violación `material-de-git` con esa ruta.
- CA-687: con la opción nueva de `verifycmd.Run`, un gate que cambia el material de git deja el veredicto rojo con el gate `material_de_git` en `fail` y las rutas en su salida; el sobre del writer cierra en `verify`, `no-entregable`, exit 1, con un hallazgo high `material-de-git: el rol writer escribio <ruta> - ...`; `verdict.LatestComplete` no lo da como verde; en `cerrar-hallazgos` el piloto se detiene con su texto de veredicto rojo, que nombra `material_de_git`.
- CA-688: `guardarTarea` fotografía el material de git antes de su primer git y después del último; un hook que lo cambia hace fallar el guardado, que deshace su commit y devuelve `un hook del guardado cambio el material de git: <ruta>: no guardo`; el guardado se niega antes de empezar si el último veredicto del árbol tiene `material_de_git` en `fail`; un hook que no cambia el material guarda como hoy.
- CA-689: para todo rol que ejecuta, con encierro o sin él, cambiar `.claude/`, `.codex/`, `.mcp.json` o la config en el árbol que declara su adaptador, las claves `hooks` de los settings de claude, `mcpServers` global o de un proyecto en `~/.claude.json`, o el archivo `~/.codex/config.toml`, es la violación `config-de-agentes`, que corta el sobre con su nota (que nombra el rol) y su detalle, y hace fallar la review con su error del contrato, exit 1 y sin registro; que claude cambie otras claves de sus archivos JSON no lo es; `agents.<rol>.write` no la levanta.
- CA-690: con un provider falso que implementa `ConEstado`, el encierro deja escribir fuera del árbol y de la caché de la tarea solo las rutas de `Escribe`, y `config-de-agentes` compara lo que declaran `EnArbol` y `Corre`; los proveedores falsos que no la implementan siguen compilando y corriendo, con nivel `ninguno`.
- CA-691: el README documenta el encierro, sus niveles, la sección `encierro` y que `gates_sin_encierro` lo debilita. [verifica: grep -q "encierro:" README.md && grep -q "gates_sin_encierro" README.md && grep -q "nivel del encierro" README.md]
- CA-692: el piloto trata un sobre cortado por `git-ajeno` o por `config-de-agentes` como uno cortado por `material-de-git`: no lo releva aunque traiga `corte: tiempo`, no lo reintenta, y se detiene antes del próximo guardado con el `Why` del caso 8.
- CA-693: después del run de un rol que ejecuta, el sobre compara la config de agentes, y en el del writer también el material de git, antes de correr `Take` o cualquier otro git sobre el árbol, y si cambiaron corta sin correr `git status`; todo git de hoom sobre el árbol de un rol que ejecuta lleva `core.fsmonitor=false`: un `core.fsmonitor` que el writer dejó en la config global no corre dentro de hoom.
- CA-694: en Linux con encierro, con un `$HOME` del test que tiene `~/.ssh`, `~/.gnupg`, `~/.config/gh`, `~/.aws` y `~/.docker/config.json` con contenido, el CLI falso de cualquier rol que ejecuta, un gate encerrado y un hook del guardado ven esos directorios vacíos y ese archivo vacío, no pueden escribirlos, y leen el resto de ese `$HOME`; sin encierro, se leen como hoy.
- CA-695: en Linux con encierro, el CLI falso del test-writer escribe en su árbol ciego, y lo que intenta escribir en el árbol de la tarea fuera de él no deja nada en el disco; el del characterizer, el del refutador y el de cada pasada del reviewer escriben en el árbol de la tarea; un `hoom finding add` corrido desde la pasada del reviewer y un `hoom finding resolve` corrido desde el refutador dejan su archivo en `.hoom/findings/` del árbol de la tarea, como hoy.

## Decisiones

- **Lo que decidió la persona el 2026-11-13.** Contestó las ocho preguntas
  de la versión anterior con "Como recomienda". Esas respuestas no son la
  firma. Quedan así en el contrato:
  1. **El encierro se activa solo con la sección `encierro` de `hoom.yaml`**,
     no solo cuando el sistema puede. Así ningún criterio ni test firmado
     cambia, y hoomai se suma a la sección en un cambio aparte, cuando el
     encierro haya corrido con las CLI reales.
  2. **Los gates del verify del sobre y del de `cerrar-hallazgos` se
     encierran**, con espacios de usuario anidados permitidos. Sin encerrarlos,
     el código del writer corre libre en su propio verify (la salida 3 del
     Objetivo). Sin espacios anidados se rompen el gate `test` de hoomai y el
     sandbox propio de cada CLI. Los montajes que cruzan quedan bloqueados.
  3. **Los roles que ejecutan también se encierran, en este spec**: el
     test-writer, el reviewer, el characterizer y el refutador, con su árbol
     de run como lo escribible. Los criterios que decían "writer" dicen "rol
     que ejecuta" donde corresponde: CA-674, CA-676, CA-677, CA-679, CA-680,
     CA-682, CA-683, CA-685, CA-689 y CA-693, más CA-695, que es nuevo.
  4. **`git-ajeno` mira solo `refs/replace/` y los objetos reescritos.**
     Mirar todas las refs daría falsos positivos. Que el writer mueva otra
     rama va a Riesgos.
  5. **`exigir` por defecto en `no`**, para que un `hoom.yaml` compartido con
     quien trabaja en una Mac no deje de correr ahí.
  6. **macOS va en su propia spec**, con un runner de macOS.
  7. **El guardado corre sin firma** (`commit.gpgsign=false`), en vez de
     fallar en un repo que firma. La persona firma al integrar.
  8. **Los secretos conocidos se tapan** (`~/.ssh`, `~/.gnupg`,
     `~/.config/gh`, `~/.aws`, `~/.docker/config.json`), como excepción
     explícita al primer No-goal. CA-694 es nuevo.
- **Las reglas nuevas valen para todo rol que ejecuta; `material-de-git`
  sigue siendo del writer.** `git-ajeno` y `config-de-agentes` son de este
  spec, así que alcanzan a los roles que la respuesta 3 encierra. Extender
  `material-de-git` cambiaría el alcance de la aclaración 25, que está
  firmada, y la persona pidió no re-expresar ningún criterio. Con encierro, el
  material que existe ya es de solo lectura para todos. Ver la pregunta de
  abajo.
- **Una caché por cada cosa encerrada.** Una caché es código que corre
  después (ver "Cachés por tarea"). Si el verify usara la caché que llena el
  run del writer, el writer podría dejar ahí el resultado de un `go test` que
  pasa, o un módulo cambiado, y su propio verify lo tomaría. Por eso cada rol,
  el verify y el guardado tienen la suya, y ninguno escribe la de otro. Las
  cachés se llenan una vez por tarea y por cada uno, y eso es más lento.
- **Encerrar en vez de enumerar.** Un hook llama a un script, el script a una
  herramienta, la herramienta lee una config del árbol. Ninguna foto cubre esa
  cadena. El encierro no necesita saber qué va a correr: le alcanza con saber
  dónde puede escribir y cuánto vive.
- **El gate amplía, no se reemplaza** (lo que pedía decidir el No-goal). El
  encierro falta en macOS, en parte de Windows, en los Linux sin espacios de
  usuario y en los proyectos que no lo piden. No bloquea lo que no existe, y
  bloquea en silencio. El gate funciona en todos los sistemas, deja un
  hallazgo con la ruta y es lo único que ve el material que el encierro deja
  escribible (la config de las CLI y el directorio común de git).
- **Cómo se contienen los procesos en Linux.** Evaluación:

  | Mecanismo | `setsid` y doble fork | si el writer mata al supervisor | daemon que ya corre | requisitos |
  |---|---|---|---|---|
  | grupo de procesos (hoy) | escapa | escapa | escapa | ninguno |
  | subreaper (`PR_SET_CHILD_SUBREAPER`) en un guardián por run | contiene: los huérfanos van al guardián, que los mata al final | escapa: con el mismo uid puede matar al guardián, y los huérfanos se van a `systemd --user` | escapa | Linux 3.4 |
  | cgroup v2 por run y `cgroup.kill` | contiene, y mata de una vez aunque esté haciendo fork | contiene: el cgroup sigue aunque muera el guardián | escapa | cgroup v2 delegado (systemd del usuario), Linux 5.14; con el mismo uid se puede mudar a otro cgroup del subárbol delegado si no hay espacio de cgroup |
  | espacio de PID (con espacio de usuario) | contiene: al terminar el principal, el kernel mata todo | contiene: no ve ni señala a nadie de afuera | contiene si además el espacio de montajes tapa los sockets | espacios de usuario sin privilegios |

  Solo el espacio de PID contiene a un writer que ataca al supervisor. El
  subreaper y el cgroup se pueden saltear con el mismo uid. El cgroup sí
  serviría para un tope de memoria y de procesos (`pids.max`, contra una bomba
  de fork), pero no hace falta para que nada sobreviva y queda fuera.
- **bubblewrap y no espacios de nombres hechos a mano.** bwrap arma el espacio
  de usuario, de PID y de montajes, `no_new_privs`, `/proc`, `/dev` y la
  sesión nueva. Lo usan Flatpak y el sandbox de Claude Code en Linux, y está
  empaquetado en todas las distribuciones. Hacerlo en Go es posible
  (`SysProcAttr` con `Cloneflags`), pero suma a hoom un runtime de
  contenedores para mantener. Lo único que hoom hace por su cuenta es la
  gracia del SIGTERM y Landlock, que son llamadas al sistema que `x/sys` ya
  trae. En esta máquina, el 2026-11-13: bwrap 0.13.1, espacios de usuario
  habilitados, Landlock activo, cgroup v2 y `dev.tty.legacy_tiocsti=0`.
- **`/tmp` de solo lectura con los sockets tapados, no privado.** Un `/tmp`
  privado escondería las CLI, los repos y los scripts de filtro que viven ahí,
  como todos los de los tests en `t.TempDir()`. Las escrituras temporales van
  a `TMPDIR`, en la caché de la tarea. Lo que cuesta es tener que tapar los
  sockets por nombre.
- **Un writer sin tarea no se encierra.** Su árbol es el checkout principal,
  con `hoom.yaml`, `.hoom/items/` y `.hoom/envelopes/` adentro, y no tiene una
  rama `hoom/<slug>` propia. Encerrarlo ahí no protegería lo que el encierro
  promete. Con `exigir`, no corre.
- **La negativa de `exigir` va dentro del paso `run`.** Un paso nuevo, como
  `aislar`, cambiaría el criterio firmado de los cinco pasos y el texto del
  tablero, por una negativa que no necesita su propio paso.
- **El veredicto con el material cambiado lleva un gate rojo.** Escribir el
  veredicto y marcarlo "no cuenta" en el registro del sobre obligaría a cada
  lector de veredictos (`LatestComplete`, `task done`, check, review) a
  conocer esa marca. Un gate rojo ya lo entienden todos.
- **macOS queda fuera de esta spec.** Lo que hay es `sandbox-exec` (Seatbelt),
  que usan Codex y Claude Code. Contiene escrituras, señales
  (`signal (target same-sandbox)`) y servicios de mach, y se hereda sin
  vuelta atrás. Lo que no hay es forma de contener procesos: macOS no tiene
  subreaper, ni cgroups, ni espacio de PID, y `NOTE_TRACK` de kqueue no
  existe ahí. Un proceso que hace `setsid` sigue vivo, aunque encerrado en las
  mismas escrituras. `sandbox-exec` está deprecado y el CI no tiene runner de
  macOS. La persona decidió el 2026-11-13 que macOS vaya en su propia spec.
- **Windows: los procesos ya, las escrituras no.** El Job Object con
  `KILL_ON_JOB_CLOSE` y sin breakaway ya contiene a los hijos. Para las
  escrituras habría que correr con integridad baja o en AppContainer, y
  marcar el árbol y las cachés con esa integridad. El CI tampoco tiene runner
  de Windows. Queda para su spec.
- **Cachés por tarea, frías la primera vez.** Una caché de build es código que
  corre después: Go reutiliza lo que encuentra bajo su clave, y Gradle corre
  los scripts de `init.d`. Compartir la caché de la persona dejaría que el
  writer la envenene para los builds de ella. Compartirla de solo lectura
  rompe agregar una dependencia.
- **El estado del provider se declara, no se copia.** Una copia de `~/.claude`
  o `~/.codex` por run (`CLAUDE_CONFIG_DIR`, `CODEX_HOME`) obligaría a copiar
  de vuelta las credenciales refrescadas, que es otro camino para escribir en
  el `$HOME` de la persona. Además, el relevo por tiempo necesita la sesión en
  el directorio real para `--resume`. La interfaz es opcional para no tocar
  los proveedores falsos de los tests.
- **El directorio común de git escribible.** Dejar de solo lectura las refs
  ajenas no se puede con montajes: git crea sus `.lock` en el mismo directorio
  que la ref. Los hooks reales escriben `refs/stash`, y el writer commitea.
- **El writer sigue commiteando.** Sacarle el commit cambiaría su contrato
  (`.hoom/agents/04-writer.md`) y no cerraría nada que `git-ajeno` no cierre.
- **El guardado no firma.** Darle al encierro el agente de gpg o de ssh dejaría
  al writer firmar lo que quiera con la llave de la persona. Los commits del
  guardado viven en la rama de la tarea, y la persona firma al integrar.

### Preguntas para la firma

Las ocho preguntas de la versión anterior las contestó la persona el
2026-11-13 (ver arriba). Al pasar esas respuestas al contrato salieron dos
nuevas:

1. **¿Se encierra también el orquestador cuando lo lanza un sobre?** Tiene
   `Exec: true` y `Primary` (`internal/agents/targets.go`), pero no está en
   la lista de la respuesta 3. Casi siempre es la sesión principal de la
   persona, o el proceso de `hoom run`, que siguen fuera (No-goals). Pero
   nada en `agentcmd` impide `hoom agent --role orquestador --task <slug>`, y
   ese run corre con tarea y puede correr los tests del writer.
   - **Recomendación:** sí, con la misma regla: todo rol que ejecuta lanzado
     por un sobre con tarea. No cuesta nada más y la regla queda sin
     excepciones. Mientras la persona no lo diga, el orquestador sigue como
     hoy.
2. **¿`material-de-git` alcanza también a los otros roles que ejecutan?** Hoy,
   y en este spec, es solo del writer (aclaración 25). Con encierro, el
   material que existe ya es de solo lectura para todos. Lo que el
   test-writer, el characterizer o el refutador creen en el árbol fuera de su
   alcance lo corta su gate de scope. Quedan sin ver dos cosas:
   - que el reviewer cree en el árbol una pieza del material que no existía,
     porque la review no tiene gate de scope;
   - que cualquiera de ellos cree `shallow` o un `config.worktree` en el
     directorio común. Un `config.worktree` solo se lee si el `config` común
     lo habilita, y ese archivo es de solo lectura.
   - **Recomendación:** no, en este spec. Extenderla cambia el alcance de un
     criterio firmado, y lo que queda sin ver es chico. Queda en Riesgos.

## Riesgos y deuda aceptada

- **Lo encerrado puede llevarse otros secretos.** Con encierro, los secretos
  conocidos se ven vacíos, pero siguen a la vista las credenciales de las CLI
  (`~/.claude`, `~/.codex`), los `.env` y el resto de `$HOME`, y hay red. Sin
  encierro se lee todo, `~/.ssh` incluido.
- **Mover otra rama no se ve** (lo decidió la persona el 2026-11-13). Un
  writer que mueve `main` en el
  directorio común no deja hallazgo; la persona lo ve en el historial de esa
  rama.
- **Los sockets del sistema que el encierro no tapa** siguen visibles: el bus
  del sistema, lo que haya en `/run` fuera de lo tapado y los sockets de `/tmp`
  con otros nombres. Un servicio que hace algo a pedido por esa vía no se ve.
- **Sin Landlock ABI 6** (Linux anterior a 6.12, como quizá el runner del CI),
  el writer puede hablarle a un socket abstracto de afuera, como el de X11 en
  una sesión X: nivel `parcial`, falta `ipc`.
- **La gracia con bwrap no está probada.** Si `--info-fd` no da lo que hace
  falta para mandar SIGTERM adentro, git muere con SIGKILL y deja su
  `index.lock`.
- **Lo que la persona corre después** (verify a mano, un commit en el
  worktree, abrir el árbol en un editor con tareas al abrir) corre código del
  writer sin encierro. Es el riesgo de siempre de correr código ajeno.
- **Los scripts del árbol que llama un hook** corren encerrados cuando los
  corre el guardado, y sin encierro cuando la persona commitea a mano.
- **Los roles que ejecutan sin encierro** (un proyecto sin la sección, o un
  sistema que no deja encerrar) corren los tests del writer como hoy.
- **Lo que `material-de-git` no ve de los otros roles.** Con encierro, el
  reviewer puede crear en el árbol de la tarea una pieza del material que no
  existía, y cualquiera de los otros roles `shallow` o un `config.worktree` en
  el directorio común (pregunta 2 de arriba).
- **El orquestador lanzado por un sobre** corre sin encierro mientras la
  persona no conteste la pregunta 1 de arriba.
- **La caché del verify la escriben los gates.** El código del writer corre en
  los gates del verify, así que puede dejar algo en la caché `verify` para el
  verify siguiente. La caché propia separa el run del writer de su verify, no
  un verify del siguiente.
- **Windows**: entre que arranca el proceso y `procesos.Adoptar` lo mete en su
  job, un hijo creado en ese instante queda afuera. Tampoco se contiene lo que
  lanza otro servicio a pedido: el programador de tareas, WMI o un servidor
  COM.
- **macOS, Linux sin espacios de usuario y los proyectos sin la sección**
  quedan como hoy, más el gate ampliado y las reglas nuevas.
- **Las listas de `Estado()` dependen de la versión de cada CLI.** Una versión
  que escribe en otro lugar falla dentro del encierro, o abre una vía nueva si
  el adaptador la declara escribible sin mirar.
- **La telemetría de codex queda escribible.** Si `~/.codex/sessions/` está en
  `Escribe`, el writer puede forjar los datos de la ventana de 5 h que lee el
  piloto, y hacer que siga lanzando cuando debería esperar. El piloto igual no
  firma ni acepta.
- **Si codex escribe `~/.codex/config.toml` en un run**, `config-de-agentes`
  corta cada sobre de writer con codex, hasta que haya un parser de TOML.
- **bwrap es una dependencia nueva** fuera del binario de hoom. Sin él, el
  nivel es `ninguno`, y con `exigir` el writer no corre.
- **La primera build de cada tarea es más lenta** porque las cachés empiezan
  vacías.
- **El gate de objetos mira la fecha de cambio.** Un sistema de archivos sin
  ctime confiable lo debilita.
- **La telemetría local del checkout principal** (`.hoom/envelopes/`) queda
  protegida solo con encierro. Sin él sigue el riesgo que declaró
  `autonomia-entre-firmas`.
