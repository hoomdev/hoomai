# Spec: los gates según lo que cambió

Estado: BORRADOR — pendiente de aprobación humana.
Tarjeta: `gates-por-ruta`. Intake: `.hoom/intake/rfc-calibracion-por-riesgo.md`, CAL-9 y 10.2. Criterios CA-809 a CA-816.

## Objetivo

La mutación es el 74 % del tiempo de gates, y corre aunque el cambio no toque lo que muta. Cada gate podrá declarar sus rutas. En un verify de trabajo, un gate sin coincidencias queda `no_aplica` con su motivo. El cierre sigue exigiendo todos los gates (I8). Lo decide hoom desde el diff y `hoom.yaml` (I9).

## No-goals

- Rutas por defecto en los perfiles o en el `hoom.yaml` de hoomAI.
- Tocar el `diff_cmd` de laravel, que ya muta solo lo cambiado (10.1).
- Pintar `no_aplica` en el Studio, el tablero TUI, su medidor o `hoom status`.

## Contratos

### `aplica_en`

```yaml
gates:
  mutation:
    aplica_en: [app/**, src/**]
```

- Es una lista no vacía de patrones relativos a la raíz del repo, con la sintaxis de `rutas:` (CA-777).
- Un gate sin la clave aplica siempre.
- La lista del proyecto reemplaza a la del perfil.
- Los errores llevan `hoom.yaml invalido: ` delante:
  - `gates.<g>.aplica_en: tiene que ser una lista no vacia de patrones`;
  - `gates.<g>.aplica_en: patron invalido "<p>"`;
  - `gates.<g>: clave desconocida "<k>" (validas: required, cmd, diff_cmd, notes, partial, aplica_en)`, también en los perfiles embebidos.

### El verify de trabajo

Es el que corre sin `--cierre`, `--full` ni `--gate`.

**El cambio** son los archivos de `gitx.ArchivosConBorrados`: los commits desde el merge-base con `base_branch`, los cambios locales, los archivos sin seguimiento y los borrados. Las rutas no ASCII llegan tal cual, y quedan fuera las exclusiones de la huella.

**Un gate con `aplica_en`** sin ningún archivo del cambio que coincida no corre su comando. Queda con `status: "no_aplica"`, `scope: "none"` y `notes: "ningun archivo del cambio coincide con aplica_en (<p1>, <p2>)"`.

- `no_aplica` no pone rojo ni hace parcial al veredicto. Cuenta en `summary.no_aplica`, que se omite cuando vale 0.
- El reporte imprime `NO APLICA` con el motivo. El pass-rate y la tendencia lo descartan, como a `absent`.

`--cierre` corre todos los gates sin mirar `aplica_en`; en lo demás, es como sin flags.

### El cierre

Si el último veredicto completo tiene algún `no_aplica`, `taskcmd.Ready` lo rechaza con el kind `sin-cierre` y este texto:

`el ultimo veredicto de "<slug>" no es de cierre: no aplicaron <g1>, <g2>.\n  Accion: ejecuta 'hoom verify --cierre' dentro del worktree y commitea`

- Llega igual a `hoom task done`, `hoom integrar` y el Studio.
- El tablero dice `falta la verificacion de cierre del espacio de trabajo`.
- `hoom check` no cambia: acepta `no_aplica`.

## Casos límite

Los fijan CA-812 (cambio vacío, sin git, `hoom.yaml` tocado, `cmd: ""`) y CA-813.

## Criterios de aceptación

- CA-809: `manifest.Load` acepta `aplica_en`, el del proyecto reemplaza al del perfil, y rechaza los tres errores con su texto exacto, sin correr gates ni escribir veredicto; sin `aplica_en` carga igual que hoy y los cuatro perfiles embebidos cargan.
- CA-810: con solo `resources/js/app.js` cambiado y `aplica_en: [app/**]` en `static` y `mutation`, `hoom verify` los deja `no_aplica` con el motivo exacto, sin correr su comando y con `summary.no_aplica: 2`; el veredicto queda verde y no parcial, y un gate sin `aplica_en` corre. (CAL-9.1)
- CA-811: con `app/Models/User.php` cambiado, borrado en un commit, sin seguimiento o con nombre no ASCII, el gate con `aplica_en: [app/**]` corre su `diff_cmd`. (CAL-9.3)
- CA-812: todos los gates aplican con el cambio vacío, fuera de git o con `hoom.yaml` cambiado; `cmd: ""` da `absent` aunque el gate tenga `aplica_en`.
- CA-813: `--cierre`, `--full` y el gate de `--gate` corren sin mirar `aplica_en`; `--cierre` usa `diff_cmd` y `--full` no; `hoom verify --help` nombra `--cierre`.
- CA-814: `hoom task done`, `hoom integrar` y `POST /api/tasks/{slug}/done` rechazan sin cerrar, con el texto exacto y el kind `sin-cierre`, un último veredicto completo con `no_aplica`; tras un verde de `--cierre` con la misma huella, cierran; `hoom check` lo acepta. (CAL-9.2)
- CA-815: el reporte imprime `NO APLICA` con el motivo; el pass-rate y la tendencia lo descartan; el tablero muestra el texto de `sin-cierre`; un veredicto sin `no_aplica` se serializa igual que hoy.
- CA-816: el README documenta `aplica_en`, `no_aplica` y `--cierre`. [verifica: grep -q "aplica_en" README.md && grep -q "no_aplica" README.md && grep -q "verify --cierre" README.md]

## Decisiones

- **`aplica_en` y no `rutas`:** `rutas:` ya es del riesgo.
- **`--cierre` y no `--full`:** el cierre conserva el `diff_cmd` (CAL-9).
- **Ante la duda, el gate corre.** Así nadie se salta un gate editando sus rutas.
- **`check` acepta `no_aplica`:** si no, todo sobre quedaría `no-entregable`. El cierre es `task done` (I8).
- **Un estado nuevo y no `skipped`:** `skipped` hace parcial al veredicto (CA-67).

## Riesgos y deuda aceptada

- Para aceptar una tarea con `aplica_en`, antes hay que correr `hoom verify --cierre`.
