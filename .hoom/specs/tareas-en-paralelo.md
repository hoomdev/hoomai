# Spec: tareas en paralelo sin choques

Estado: BORRADOR — pendiente de aprobación humana.
Tarjeta: `tareas-en-paralelo`. Intake: `.hoom/intake/rfc-calibracion-por-riesgo.md`, CAL-12 y 10.2. Criterios CA-817 a CA-824.

## Objetivo

Que varias tarjetas avancen a la vez sin pisarse: hoom corre el setup del proyecto y le pasa a cada gate su tarea. La base por tarea es del proyecto (10.2).

## No-goals

- Limpiar recursos en `task done`, setup en el árbol ciego, o que el piloto elija o espere tarjetas.
- Dependencias en `hoom tablero` o el Studio.

## Contratos

### El setup

```yaml
worktree:
  setup: composer install --no-interaction --no-progress
```

- El perfil lo declara (solo laravel, el de arriba) y `hoom.yaml` lo reemplaza; `setup: ""` lo apaga.
- `timeout` sigue las reglas de `agents.<rol>.timeout`; por defecto, `30m`.
- Otra clave da `hoom.yaml invalido: worktree: clave desconocida "<k>" (validas: setup, timeout)`.
- Tras `git worktree add`, start corre `sh -c <setup>` en el worktree con `HOOM_TASK=<slug>`, con el prefijo `ejecutar` de `.hoom/local.yaml` si lo hay, sin encierro (CA-767).
- **Falla** con exit distinto de 0, sin tiempo, o si `git status --porcelain` del worktree no queda vacío (I2). Start quita el worktree y la rama, sale con 1 e imprime:

`el setup del espacio de trabajo fallo (<motivo>):\n<ultimas 20 lineas>\n  Accion: corrige el setup o el .gitignore y vuelve a correr 'hoom task start <slug>'`

con `<motivo>` `exit <n>`, `tiempo agotado` o `dejo archivos sin ignorar: <a>, <b>`.

### `HOOM_TASK` en los gates

- Cada gate recibe `HOOM_TASK` con la tarea del árbol (`verdict.Tarea`), y ninguna en el checkout principal, aunque venga heredada.
- `ejecutar` acepta `{task}`, reemplazada como `{rel}` (`''` sin tarea). Las sugerencias ddev y sail de laravel pasan `HOOM_TASK={task}`.

### Dependencias

- El item acepta `depende_de: [<slug>, ...]`, que escriben `hoom item add` y `hoom item set <s>` con `--depende-de a,b` (`""` lo quita).
- Add y set rechazan, sin escribir: `depende_de: no existe el item "<x>"`, `depende_de: un item no depende de si mismo` y `depende_de: ciclo <a> -> <b> -> <a>`.
- Una dependencia está **pendiente** si su item no tiene `hecho_en` o ya no existe.
- `hoom board` muestra `espera a: <a>, <b>` y `--json` da `espera_a`, omitido vacío.
- Start con una pendiente imprime `aviso: "<s>" depende de <a>, que no esta hecha` y abre la tarea.

### El tope

- `.hoom/local.yaml` acepta `tareas_simultaneas: <n>`, entero mayor que 0; si no, `.hoom/local.yaml: tareas_simultaneas tiene que ser un entero mayor que 0`.
- Cuenta los worktrees de `.hoom/worktrees/`, aun los que esperan a una persona.
- Llegado al tope, start se niega con `hay <n> tareas abiertas y .hoom/local.yaml permite <n> a la vez.\n  Accion: cierra una con 'hoom task done <slug>' o sube tareas_simultaneas`.
- Cuenta y crea bajo un lock. Sin la clave, no hay tope.

## Casos límite

- `POST /api/tasks` y `lanzar` usan `taskcmd.Start`.
- P7: vale desde el próximo start; lo ya abierto no cambia.

## Criterios de aceptación

- CA-817: `manifest.Load` toma `worktree` del perfil o de `hoom.yaml`, con `setup: ""` apagado, y rechaza la clave desconocida y el `timeout` inválido con su texto.
- CA-818: start corre el setup en el worktree con `HOOM_TASK=<slug>` y el prefijo; después, un gate `lint` que usa lo instalado pasa sin pasos manuales. (CAL-12.2)
- CA-819: con el setup en exit 1, sin tiempo o dejando un archivo sin ignorar, start sale con 1 y el texto exacto, sin worktree ni rama; `POST /api/tasks` da 400.
- CA-820: dos verifies a la vez en los worktrees `a` y `b` dan a sus gates `HOOM_TASK` `a` y `b`, también por `{task}`; en el principal, ninguno. (CAL-12.1)
- CA-821: add y set escriben y quitan `depende_de` y rechazan los tres errores sin escribir; set se niega con `$HOOM_ROLE`; un item sin la clave queda igual.
- CA-822: `hoom board` y `--json` marcan la dependencia pendiente y no la hecha; start imprime el aviso y abre la tarea. (CAL-12.3)
- CA-823: con `tareas_simultaneas: 2` y dos worktrees, start se niega con el texto exacto; con uno, de dos starts simultáneos abre uno; sin la clave, no hay tope.
- CA-824: el README documenta `worktree`, `HOOM_TASK`, `{task}`, `depende_de` y `tareas_simultaneas`. [verifica: grep -q "{task}" README.md && grep -q "depende_de" README.md && grep -q "tareas_simultaneas" README.md]

## Decisiones

- **De la persona:** el setup que falla deshace todo; una dependencia pendiente avisa y sigue (I9); el tope es por máquina, como el prefijo (P6).
- **`HOOM_TASK` del gate es la del árbol:** en un worktree coincide con la de los hallazgos (CA-549).

## Riesgos y deuda aceptada

- El setup instala lo de la base y, si es largo, retrasa el 202 de `lanzar`.
- Un binario viejo omite un item con `depende_de` (CA-266).
- «Hecha» es `hecho_en`, no integrada en la base.
