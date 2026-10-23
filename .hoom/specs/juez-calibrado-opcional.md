# Spec: juez calibrado opcional

Estado: BORRADOR — pendiente de aprobación humana. Depende de
`hallazgos-con-traza-y-cobertura`.
Tarea sugerida: `hoom task start juez-calibrado-opcional`.
Origen: análisis de TypeSafe (modelo Jev, 2026-09-23) y de un análisis
externo que Henry compartió. Jev responde solo tres tipos de pregunta
(elección, puntaje en una rúbrica, verdad de una afirmación) con
probabilidades y una confianza calibrada. No genera ni razona largo. Su
propia documentación dice que rinde menos con varios saltos de razonamiento,
con estados con mucho contenido irrelevante, y que no trata el estado como
hostil.

## Objetivo

Un juez opcional para decisiones chicas, que **solo puede hacer a hoom más
estricto o sugerir, nunca más laxo**, y que deja cada juicio registrado como
dato. Sin clave, hoom se comporta exactamente como hoy.

1. Paquete `judge` con una interfaz de tres preguntas (`Choice`, `Score`,
   `Noul`) y dos implementaciones: `typesafe` (HTTP con `net/http`, sin
   dependencias) y `none`.
2. Se activa solo si existe `HOOM_TYPESAFE_KEY` en el entorno; `hoom.yaml`
   puede apagarlo, fijar umbrales y limitar qué se envía.
3. Cuatro usos, y ninguno más:
   - **Lentes de la review**: sugiere lentes además de las que exige la regla;
     solo puede agregar.
   - **Hallazgos duplicados**: "¿estos dos describen la misma causa raíz?"
     propone fusiones que una persona confirma.
   - **Segunda opinión de severidad**: si discrepa con el reviewer, el
     hallazgo se marca `severidad discutida` para el refutador; nunca cambia
     la severidad.
   - **Motivo de un sobre que falló**: clasifica el final del run en
     terminó, bloqueado, necesita input o se rindió, como texto para la
     tarjeta que ya falló por evidencia.
4. **Registro**: cada juicio se guarda con pregunta, probabilidades,
   confianza, costo y, cuando se conoce, el desenlace (el hallazgo terminó
   corregido o refutado), para calibrar umbrales con datos propios.

## No-goals

- Jev en verify, check, ningún gate, el gate de scope, la columna de una
  tarjeta o una aprobación. Ni como "gate probabilístico".
- Preguntas que ya son determinísticas en hoom (si un CA tiene test, si un
  archivo está fuera de scope, SQL concatenado): eso es `spec_trace`, el gate
  de scope y semgrep.
- Enrutar el siguiente rol o elegir el contexto del agente.
- Un SDK de Go publicado aparte: el cliente vive en el repo; si hay demanda
  se extrae después.

## Contratos

- El estado que se le manda al juez lo arma hoom desde evidencia (hunks del
  diff, campos del hallazgo, líneas del log del run), **nunca** desde
  narración del CLI (resúmenes del agente, mensajes de commit). Un test lo
  fija: la función que arma el estado rechaza un texto marcado como
  narración.
- Una pregunta por cosa, con el estado filtrado por código; nunca "¿cumple
  los requisitos?" sobre un diff entero.
- Un `Score` se compara solo contra un umbral; su valor no se usa como
  magnitud.
- Umbrales en `hoom.yaml` (`judge.min_confidence`, por defecto 0,9); por
  debajo, el juez calla.
- `judge.enviar: rutas|hunks` limita qué sale de la máquina; por defecto
  `rutas`.
- Registro de juicios en `.hoom/cache/judge/` (telemetría local, fuera de
  git), con el desenlace anotado cuando el hallazgo se cierra.

## Casos límite y errores esperados

- Sin clave, clave inválida (401), rate limit (429/529) o red caída: el punto
  de uso sigue el camino determinístico de hoy y lo registra; nunca falla un
  comando por el juez.
- Un juicio por debajo del umbral: no se muestra ni cambia nada.
- `judge.enabled: false` con clave en el entorno: no se llama.

## Criterios de aceptación

- CA-472: sin `HOOM_TYPESAFE_KEY`, ningún comando hace una llamada de red al
  juez y su salida es idéntica a la de hoy (test con un servidor falso que
  cuenta llamadas).
- CA-473: con clave, `hoom review` agrega a las lentes de la regla las que el
  juez sugiere por encima del umbral, y nunca quita una; el registro de
  review dice cuáles vinieron del juez.
- CA-474: la fusión de duplicados se propone y no se aplica sin
  confirmación.
- CA-475: una discrepancia de severidad marca el hallazgo y no cambia su
  severidad.
- CA-476: el motivo sugerido de un sobre que falló aparece en la tarjeta
  marcado como sugerencia, y no cambia su columna ni su estado.
- CA-477: el armado del estado rechaza narración del CLI.
- CA-478: con `judge.enviar: rutas`, ningún contenido de archivo sale en el
  pedido (test con servidor falso que inspecciona el cuerpo).
- CA-479: cada juicio queda registrado con pregunta, probabilidades,
  confianza y costo, y el desenlace se agrega cuando el hallazgo se cierra.
- CA-480: errores de red, 401, 429 y 529 dejan el camino determinístico y no
  cambian el exit code.

## Decisiones

- **Determinístico manda, probabilístico frena.** Es la política que
  distingue a hoom, no usar un modelo.
- **Apagado por defecto y con la clave del usuario.** El código de cliente
  sale de la máquina a un tercero; nadie manda su código sin decidirlo.
- **Calibrar con datos propios.** Los umbrales de fábrica no vienen de código;
  hoom produce las etiquetas (corregido o refutado) para fijarlos.

## Riesgos y deuda aceptada

- **Proveedor en acceso temprano.** La interfaz es genérica: si desaparece o
  alguien prefiere un modelo local, es otra implementación.
- **Costo y latencia por comando** con clave. Se mide en el registro.
- **Nueve criterios.** Se implementa después de S1.
