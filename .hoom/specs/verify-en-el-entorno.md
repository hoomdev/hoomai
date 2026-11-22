# Spec: verify corre donde vive el runtime del proyecto

Estado: BORRADOR — pendiente de aprobación humana.
Tarjeta: `verify-en-el-entorno`. Intake: `.hoom/intake/rfc-calibracion-por-riesgo.md`, CAL-8 con el ajuste de 10.2 y la respuesta P6 de 10.4. Criterios CA-760 a CA-773.
Enmienda del 2026-11-22, antes de los tests: la foto de `.hoom/local.yaml` alrededor de los gates (CA-773).
Enmienda del 2026-11-22, después del verify: las rutas relativas de los worktrees son opcionales por máquina (CA-771, ver Decisiones).
Enmienda del 2026-11-22, después de la review: en un proyecto que vive en un subdirectorio del repo, `hoom init` ignora `local.yaml` también en el principal, y el error de no ignorado dice dónde (CA-763, CA-770, ver Decisiones).

## Objetivo

Que el veredicto de `hoom verify` diga la verdad en una máquina donde el runtime del proyecto no está en el host: DDEV, Sail, Docker Compose, devcontainers o Lando.

Hoy esa máquina saca rojos que no significan nada. En el proyecto medido del intake, los 137 veredictos automáticos salieron rojos porque el host no tiene `php`, y 171 de 172 sobres quedaron `no-entregable`. Los writers lo rodearon a mano con `ddev exec … hoom verify`. Además, un comando que no existe sale con exit 127 y el gate queda en `fail`, aunque el comentario del paquete `gates`, el README y el tutorial prometen `error`. Para el piloto, un `fail` es un rojo de código y relanza al writer; un `error` es de entorno y para (CA-650). Así, cada 127 gasta un writer que no puede arreglar nada.

Este spec hace dos cosas:

1. Un gate que no pudo correr (exit 127 o 126) queda en `error` y nombra lo que faltó.
2. Un ajuste local por máquina, `.hoom/local.yaml`, fuera de git, con la plantilla de ejecución de los gates. El veredicto registra dónde corrió cada gate. El perfil solo lo sugiere: hoom nunca decide solo dónde corre un gate (I9 y P6).

Protege I1: el veredicto vuelve a salir de comandos que de verdad corrieron.

## No-goals

- Detectar el entorno y elegir el prefijo solo (P6: el veredicto cambiaría según la máquina sin que nadie lo note).
- Los comandos `verifica` de `spec_trace` (CA-69) y los del ratchet (CA-96): siguen en el host y como hoy. El caso límite «comando declarado que no existe: FAIL con exit 127» de `gates-ausentes-parciales-verifica` es de esos comandos y no cambia.
- Un prefijo por gate: hay una plantilla y una lista de gates que se quedan en el host.
- Levantar el entorno (`ddev start`, `docker compose up`). Si no está arriba, el prefijo falla y el gate lo dice.
- Un gate muerto por una señal sin timeout: sigue en `fail`.
- Correr roles o `hoom review` dentro del contenedor: solo los gates.

## Contratos

### Un gate que no pudo correr

- Exit 127 (`sh`: comando no encontrado): estado `error`, `exit_code` 127, nota `no pudo correr: comando no encontrado (exit 127): <linea>`.
- Exit 126 (encontrado pero no ejecutable): estado `error`, `exit_code` 126, nota `no pudo correr: el comando no se puede ejecutar (exit 126): <linea>`.
- `<linea>` es la última línea no vacía de la salida del gate, que es donde `sh` y `env` nombran el comando. Sin salida, la nota termina en `)`.
- Cualquier otro exit distinto de 0 sigue en `fail`, y el timeout sigue en `error`, como hoy.
- Vale con prefijo y sin él: un `ddev` que falta en el host da 127 igual que un `php` que falta.

### `.hoom/local.yaml`

- Vive en el checkout principal: la carpeta padre del directorio común de git del árbol verificado. Desde `.hoom/worktrees/<slug>/` se lee el del principal; un `.hoom/local.yaml` dentro de un worktree no se lee. Fuera de un repo git, se lee el de la raíz del proyecto.
- Claves:

  ```yaml
  ejecutar: "ddev exec -d /var/www/html/{rel} sh -c {cmd}"
  en_el_host: [semgrep]   # gates que corren como hoy
  worktrees_relativos: true   # ver «Worktrees con rutas relativas»
  ```

- En `ejecutar`, hoom reemplaza estas marcas, cada una como una palabra de `sh` entre comillas simples:
  - `{cmd}`: el comando del gate ya armado, con `{base}`, `{files}` y `{packages}` resueltos;
  - `{dir}`: la ruta absoluta del árbol verificado;
  - `{rel}`: la ruta del árbol relativa al checkout principal: `.` en el principal y `.hoom/worktrees/<slug>` en una tarea.
- El gate corre `sh -c <ejecutar con las marcas reemplazadas>`, con el mismo directorio, timeout y gracia de hoy. Los gates de `en_el_host` corren como hoy.
- Vale para todo el que corre los gates: `hoom verify` a mano, el sobre de `hoom agent`, el piloto (`cerrar-hallazgos`) y `POST /api/verify`.
- `ejecutar:` sin valor o vacío cuenta como sin la clave (la regla de CA-747).
- Errores. Con cualquiera de ellos, `hoom verify` no corre ningún gate ni escribe veredicto, como con un `hoom.yaml` inválido:
  - `.hoom/local.yaml: clave desconocida "<k>" (validas: ejecutar, en_el_host, worktrees_relativos)`;
  - `.hoom/local.yaml: ejecutar tiene que llevar {cmd}`;
  - `.hoom/local.yaml: en_el_host nombra "<g>", que no es un gate de hoom.yaml`;
  - `.hoom/local.yaml: git no lo ignora y es de esta maquina: agrega local.yaml al .hoom/.gitignore del checkout principal (y git rm --cached .hoom/local.yaml si esta en git)`.
- `.hoom/.gitignore` canónico (`hoomfs.GitignoreBody`) suma `local.yaml`, y `hoom init` completa uno existente con esa línea.
- Cuando el proyecto vive en un subdirectorio del repo, `hoom init` completa además el `.hoom/.gitignore` del checkout principal con `local.yaml`, y si no existe lo crea solo con esa línea. Es ahí donde se lee y se valida el archivo. Un fallo al escribirlo es un error de `hoom init`, como el del ignore del proyecto.

### El veredicto

- Con `ejecutar`, el veredicto suma, en el nivel de arriba, `"entorno": {"ejecutar": "<plantilla>", "sha256": "<de .hoom/local.yaml>"}`. Cada gate suma `"entorno": "prefijo"` o `"entorno": "host"`.
- Sin `ejecutar`, ninguna de esas claves aparece y el JSON queda como hoy.
- La huella no cambia: es del árbol (I2), y `.hoom/local.yaml` está ignorado.
- `hoom verify` imprime, debajo de su primera línea, `entorno: <plantilla> (de .hoom/local.yaml)`.

### El encierro

- Con la sección `encierro` y un sobre, un gate con prefijo corre sin encierro, como uno de `gates_sin_encierro`, con la nota `sin encierro: corre con el prefijo de .hoom/local.yaml`. El contenedor es su límite. Las fotos del material de git alrededor de los gates siguen.
- Con `encierro.exigir` en `completo` o `parcial`, ese gate queda en `error` con la nota `el prefijo de .hoom/local.yaml no corre encerrado y encierro.exigir es <valor>`.
- El sobre de `hoom agent` saca el sha256 de `.hoom/local.yaml` del principal, o anota que no existe, antes y después del rol. Una diferencia es manipulación: regla `manipulacion` con la ruta `.hoom/local.yaml`, igual que un cambio a la evidencia. Un archivo ignorado no aparece en el scope, y un rol que escribe `ejecutar: "true {cmd}"` pondría verde todo.
- Todo verify (a mano, el del sobre, el del piloto y `POST /api/verify`) saca también el sha256 de `.hoom/local.yaml` del principal, o anota que no existe, antes y después de los gates. Si cambió, el veredicto suma el gate `entorno_local`, requerido y en `fail`, con la nota `manipulacion: .hoom/local.yaml cambio mientras corrian los gates`, y queda rojo, como `material_de_git`. Si no cambió, ese gate no aparece. Un gate que corre sin encierro, con prefijo o sin él, puede escribir en el principal, y la foto del sobre es de antes y después del rol, no de los gates.

### La sugerencia del perfil

- Un perfil puede declarar `entornos`: una lista de `{nombre, si_existe, sin, ejecutar}`. `si_existe` es una ruta del proyecto, `sin` es un comando que no está en el PATH y `ejecutar` es la plantilla sugerida.
- `laravel` declara:
  - `ddev`: `si_existe: .ddev/config.yaml`, `sin: php`, `ejecutar: "ddev exec -d /var/www/html/{rel} sh -c {cmd}"`;
  - `sail`: `si_existe: vendor/bin/sail`, `sin: php`, `ejecutar: "docker compose exec -T -w /var/www/html/{rel} laravel.test sh -c {cmd}"`.
- Cuando se cumplen las dos condiciones de una entrada, se imprime `sugerencia: este proyecto parece usar <nombre> y el host no tiene <sin>; para correr los gates ahi, crea .hoom/local.yaml con:` y en la línea siguiente `  ejecutar: "<ejecutar>"`. Eso pasa:
  - al final de `hoom init`;
  - en el texto de `hoom verify`, cuando un gate quedó en `error` por exit 127 y no hay `.hoom/local.yaml`.
- Ninguno escribe `.hoom/local.yaml`. `--json` no suma nada.

### Worktrees con rutas relativas

- Con `worktrees_relativos: true` en `.hoom/local.yaml` del principal, `hoom task start` crea el worktree con `git worktree add --relative-paths`. Así el `.git` del worktree vale también dentro de un contenedor que monta el proyecto en otra ruta. `gitx.WorktreeDe` ya resuelve rutas relativas.
- Sin la clave, o con `false`, nulo o vacío, el worktree se crea como hoy, y `.git/config` no cambia.
- Con la clave y un git que no acepta `--relative-paths` (más viejo que 2.48), `hoom task start` se niega sin crear nada: `.hoom/local.yaml: worktrees_relativos pide git 2.48 o mas nuevo`.
- `--relative-paths` escribe `extensions.relativeWorktrees` en el `.git/config` de ese clon, y desde ahí un git más viejo que 2.48 rechaza el repo entero, también dentro del contenedor. Por eso es de la máquina: lo enciende quien sabe que todos sus git son 2.48 o más nuevos.
- `hoom task start` lee `.hoom/local.yaml` con las mismas reglas que `hoom verify`: con uno de los errores de arriba, se niega con ese error y no crea el worktree.

### Documentación

El README documenta `.hoom/local.yaml`, sus marcas, `en_el_host`, `worktrees_relativos` con su requisito de git 2.48 en el host y en el contenedor, la interacción con el encierro, `git worktree repair --relative-paths` para los worktrees creados antes, y que `ejecutar` no lleve secretos porque viaja en el veredicto.

## Casos límite y errores esperados

- Sin `.hoom/local.yaml`: los mismos comandos, los mismos estados y el mismo JSON que hoy (CAL-8.2), salvo el 127 y el 126.
- Con prefijo, `php` falta dentro del contenedor: el 127 viene de adentro y `ddev exec` lo propaga; el gate queda en `error` con la línea del contenedor.
- El entorno está apagado: el prefijo sale con su propio código, distinto de 127, y el gate queda en `fail` con su salida. Es lo que pasó, y la salida lo dice.
- Una comilla simple en el comando del gate: `{cmd}` la escapa (`'\''`), y `sh` dentro del contenedor recibe el comando intacto.
- `en_el_host` sin `ejecutar` no cambia nada.
- Un worktree con ruta absoluta (creado antes, o sin `worktrees_relativos`): git dentro del contenedor no lo encuentra y el gate que use git falla. El README dice cómo repararlo.

## Lo que este spec re-expresa

Ningún criterio firmado. Ningún CA fija hoy que el exit 127 de un gate sea `fail`; lo dicen solo un comentario de `internal/gates/runner_test.go` y el caso límite de los comandos `verifica`, que no cambia. CA-650 queda como está: ahora también lo dispara un 127.

## Criterios de aceptación

- CA-760: un gate requerido cuyo comando sale con 127 queda `error`, con `exit_code` 127 y la nota `no pudo correr: comando no encontrado (exit 127): <ultima linea no vacia de la salida>`; el veredicto es rojo y el resumen cuenta un `error`.
- CA-761: un gate que sale con 126 queda `error` con la nota `no pudo correr: el comando no se puede ejecutar (exit 126): <linea>`; un exit 1, 2 o 3 sigue en `fail` con su código, y el timeout sigue en `error`.
- CA-762: `.hoom/local.yaml` se lee del checkout principal también desde `.hoom/worktrees/<slug>/`; uno dentro del worktree no se lee; `ejecutar:` vacío o nulo cuenta como sin la clave.
- CA-763: los cuatro errores de `.hoom/local.yaml` salen con su texto exacto y `hoom verify` no corre gates ni escribe veredicto.
- CA-764: con `ejecutar`, cada gate fuera de `en_el_host` corre `sh -c` con la plantilla y `{cmd}`, `{dir}` y `{rel}` reemplazados como palabras entre comillas simples; `{rel}` es `.` en el principal y `.hoom/worktrees/<slug>` en una tarea; los de `en_el_host` corren como hoy.
- CA-765: el prefijo vale igual en `hoom verify`, en el sobre de `hoom agent`, en el `cerrar-hallazgos` del piloto y en `POST /api/verify`.
- CA-766: con `ejecutar`, el veredicto trae `entorno.ejecutar`, `entorno.sha256` y el `entorno` de cada gate, y el texto imprime la línea `entorno:`; sin `ejecutar`, el JSON no trae ninguna clave `entorno` y los comandos y estados son los de hoy.
- CA-767: con `encierro` y un sobre, un gate con prefijo corre sin encierro y con la nota `sin encierro: corre con el prefijo de .hoom/local.yaml`; con `exigir` en `completo` o `parcial`, queda `error` con la nota de exigir.
- CA-768: un rol que cambia, crea o borra `.hoom/local.yaml` del principal deja el sobre con la regla `manipulacion` y la ruta `.hoom/local.yaml`.
- CA-769: con `.ddev/config.yaml`, perfil `laravel` y sin `php` en el PATH, `hoom init` imprime la sugerencia de `ddev`, y `hoom verify` la imprime debajo de un `error` por 127 cuando no hay `.hoom/local.yaml`; con `vendor/bin/sail`, la de `sail`; con `php` en el PATH, ninguna; nadie escribe `.hoom/local.yaml`.
- CA-770: `hoomfs.GitignoreBody` incluye `local.yaml` y `hoom init` lo agrega a un `.hoom/.gitignore` que no lo tiene; con el proyecto en un subdirectorio del repo, `hoom init` deja `local.yaml` también en el `.hoom/.gitignore` del checkout principal (lo crea si no existe, con esa sola línea), y después un `.hoom/local.yaml` del principal no da `ErrNoIgnorado`; el `.hoom/.gitignore` de este repo lo tiene. [verifica: grep -qx 'local.yaml' .hoom/.gitignore]
- CA-771: con `worktrees_relativos: true` y git 2.48 o más nuevo, `hoom task start` deja en el worktree un `.git` con `gitdir` relativo, y `hoom task list`, `hoom verify` y `hoom task done` lo reconocen como hoy; sin la clave, el `.git` del worktree es absoluto y `.git/config` no gana `extensions.relativeWorktrees`; con la clave y un git más viejo, `hoom task start` se niega con `.hoom/local.yaml: worktrees_relativos pide git 2.48 o mas nuevo` y no crea el worktree.
- CA-772: el README documenta el ajuste local y el encierro. [verifica: grep -q "\.hoom/local.yaml" README.md && grep -q "en_el_host" README.md && grep -q "relative-paths" README.md && grep -q "worktrees_relativos" README.md]
- CA-773: un gate que crea, cambia o borra `.hoom/local.yaml` del principal mientras corre deja el veredicto rojo, con el gate `entorno_local` requerido en `fail` y la nota `manipulacion: .hoom/local.yaml cambio mientras corrian los gates`; un verify que no lo toca no trae ese gate.

## Decisiones

- **Ajuste por máquina, explícito.** La respuesta P6 del intake: la detección automática haría que el mismo árbol diera veredictos distintos según la máquina, en silencio. El perfil sugiere; la persona escribe el archivo.
- **Solo 126 y 127 pasan a `error`.** `sh` los reserva para «no encontrado» y «no ejecutable». Cualquier otro código es de la herramienta, y hoom no lo interpreta. Descartado: leer la salida en busca de «not found», porque depende del idioma (el intake trae `No existe el fichero o el directorio`).
- **En el checkout principal, no en `~/.config`.** Es del proyecto y de la máquina, junto a los otros directorios locales, y los worktrees lo comparten sin copiarlo.
- **Una plantilla con `{cmd}`, no un prefijo que se antepone.** DDEV, Compose, devcontainers y Lando reciben el comando de formas distintas; con `{cmd}` entran todos.
- **La plantilla no entra en la huella.** La huella es del árbol (I2). El veredicto dice dónde corrió, y `hoom check` y `task done` no cambian.
- **Con encierro, el gate con prefijo corre fuera del bwrap.** El contenedor necesita el socket de docker, y abrírselo al bwrap equivale a darle root al código del writer. `exigir` lo niega, porque quien exige encierro no lo pierde por un archivo local. Descartados: abrir el socket, y rechazar `.hoom/local.yaml` siempre que haya encierro, porque una máquina con DDEV y encierro no podría usar el prefijo. Hoom lo eligió el 2026-11-22.
- **La foto de `.hoom/local.yaml` en el sobre y alrededor de los gates.** Un archivo ignorado no aparece en el scope, y esa plantilla decide qué corre. La del sobre ve al rol; la de los gates ve al código del writer que corre en un test, y vale en todo verify, porque también un `hoom verify` a mano corre ese código.
- **Rutas relativas en los worktrees, opcionales por máquina**, para CAL-8.4: sin ellas, git dentro del contenedor no encuentra el repo de una tarea. Son opcionales porque `--relative-paths` escribe una extensión en `.git/config` que un git más viejo que 2.48 no acepta, y la imagen del contenedor puede traer uno más viejo. `.git/config` es de cada clon, igual que `.hoom/local.yaml`. Descartados: encenderlas siempre (lo firmado al principio; el writer encontró la extensión), y sacarlas de esta tarjeta. Hoom lo eligió el 2026-11-22.
- **En un subdirectorio, `hoom init` ignora `local.yaml` también en el principal.** El archivo se lee y se valida en el principal, y un ignore en el `.hoom/.gitignore` del proyecto no lo cubre. Se crea solo con `local.yaml` porque el principal puede no ser un proyecto de hoom. Descartado: dejarlo a mano, con el error como guía. Hoom lo eligió el 2026-11-22, por los hallazgos 470368 y 75bffc de la review.

## Riesgos y deuda aceptada

- Una plantilla que traga el código de salida, como `true {cmd}`, pone verde todo. La escribe la persona, el veredicto la registra con su sha256 y ningún rol la puede cambiar sin que el sobre lo marque. El README lo advierte.
- La plantilla viaja entera en el veredicto, que va en git, y en la salida de `hoom verify`: una credencial escrita en `ejecutar` se filtra. El README advierte que no se pongan secretos ahí. Hoom aceptó el riesgo el 2026-11-22, por el hallazgo 61fc24 de la review.
- El `hoom verify` que un rol corre por su cuenta, dentro de su encierro, no llega al socket de docker y sus gates con prefijo fallan. El que cuenta es el del sobre.
- Los worktrees con ruta absoluta la conservan hasta que alguien corre `git worktree repair --relative-paths`, que también escribe la extensión en `.git/config`. Sin `worktrees_relativos`, un gate con prefijo que use git dentro del contenedor falla en una tarea; el README lo dice.
- El exit 2 que `.hoom/semgrep/gate.sh` usa para «no pudo correr» sigue siendo `fail`. Cambiarlo es del gate, no de hoom.

## Preguntas para la firma

Ninguna. La que había (el gate con prefijo y el encierro) está contestada en Decisiones. Contestarla no es la firma.
