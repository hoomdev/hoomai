# Spec: el trinquete — calidad que solo puede subir (gate ratchet)

Estado: BORRADOR — pendiente de aprobación humana.
Tarea sugerida: `hoom task start trinquete-ratchet`.

## Objetivo

Los gates actuales comparan contra umbrales FIJOS: responden "¿este cambio
supera la vara?" pero no ven la erosión lenta (el MSI que baja de 82 a 61 a
lo largo de meses, todo verde porque la vara está en 60) ni atrincheran las
mejoras (la cobertura sube a 85% y nada impide que vuelva a 62% en verde).
Con agentes generando volumen, ese es EL riesgo: mil cambios individualmente
verdes que en conjunto degradan.

Nace el gate `ratchet`: una línea base de métricas medibles congelada en
`.hoom/ratchet.json` (viaja en Git), que solo puede moverse hacia MEJOR.
Empeorar más allá de la tolerancia = veredicto ROJO con la cifra exacta.
Mejorar = la base se aprieta sola y el equipo hereda el piso nuevo.
Aflojar existe, pero es un comando explícito con razón obligatoria que deja
registro — escape consciente y visible, jamás silencioso. La frase "el
código que sale del harness es escalable y bueno" deja de ser una foto por
cambio y pasa a ser una curva monótona certificada por el binario.

## No-goals

- Extractores por herramienta: hoom no sabe qué es Infection ni coverage.
  Cada métrica declara un COMANDO cuya última línea de salida es un número
  — el mismo principio agnóstico del manifiesto (capacidad → comando).
- Métricas nativas de hallazgos: el contrato vigente dice que `verify` no
  lee `.hoom/findings/`; el trinquete lo respeta. Quien quiera esa métrica
  la declara como comando sobre el JSON de findings, bajo su
  responsabilidad.
- Correr en verifies normales o con selección de gates: el hábitat del
  trinquete es la corrida COMPLETA (`--full`, típicamente nocturna). Las
  corridas parciales jamás tocan la base.
- Mostrar el trinquete en `hoom status` y el Studio: fase siguiente,
  cuando haya bases reales en uso.
- Umbrales por porcentaje relativo, ventanas móviles o estadística: v1 es
  base absoluta + tolerancia absoluta. Simple y auditable.

## Contratos

Archivo `.hoom/ratchet.json` (schema `hoom.ratchet/v1`, commiteado):

- `metrics`: mapa nombre → { `cmd` (shell, su última línea no vacía debe
  ser un número), `direction` ("up" = más es mejor, "down" = menos es
  mejor), `tolerance` (absoluta, default 0), `value` (la base congelada;
  ausente = aún sin congelar), `updated_at` }.
- `history`: registro append de cada movimiento — { ts, metric, from, to,
  kind: frozen | tightened | loosened, reason }.
- El archivo queda EXCLUIDO de la huella del candidato (misma clase que
  los veredictos: estado del harness, no código bajo verificación) — así
  un apriete de base durante verify no rompe el check verde.

Verbos nuevos:

- `hoom ratchet init` — crea el esqueleto (sin pisar uno existente) y
  explica cómo declarar métricas.
- `hoom ratchet lower <metrica> --to <valor> --reason "<por qué>"` — baja
  la base dejando registro `loosened`; sin `--reason` se niega; un valor
  que en realidad mejora la base también se niega (eso es apretar, y
  apretar es trabajo del verify).

Gate `ratchet` (sintético, required, solo en `verify --full` sin
`--gate` y con métricas declaradas):

- Mide cada métrica con `sh -c` (timeout 10 min por comando) en la raíz.
- Métrica sin base: se congela con el valor medido (kind `frozen`, nota
  visible) y pasa.
- Regresión más allá de la tolerancia: FAIL nombrando métrica, valor
  actual, base y delta.
- Mejora más allá de la tolerancia: pasa y APRIETA la base (kind
  `tightened`).
- Movimiento dentro de ±tolerancia: pasa sin tocar la base (anti-ruido).
- Comando roto (exit != 0, timeout o salida no numérica): ERROR
  fail-closed, la base no se toca.
- El gate emite eventos vivos como cualquier otro y aparece en el
  veredicto con su evidencia por métrica.

## Casos límite y errores esperados

- Sin `.hoom/ratchet.json` o con `metrics` vacío: el gate no existe; nada
  cambia en ningún verify.
- Archivo presente pero corrupto o con `direction` inválida: gate ERROR
  (configurado pero roto = rojo), base intacta.
- `verify --full --gate x`: corrida parcial; el trinquete no corre ni
  toca la base.
- Primera corrida de un proyecto ya degradado: la base se congela en la
  realidad de HOY, sin vergüenza; la única regla es "de acá, para arriba".
- Mejora y regresión simultáneas en métricas distintas: FAIL (una
  regresión alcanza), pero las mejoras igual aprietan — lo ganado no se
  pierde por lo perdido.
- No se puede persistir el apriete (disco): ERROR honesto; la base vieja
  sigue vigente y el próximo verify re-aprieta.
- `ratchet lower` de una métrica inexistente o sin base congelada: error
  con la acción exacta.

## Criterios de aceptación

- CA-89: `hoom ratchet init` crea `.hoom/ratchet.json` con el schema y
  `metrics` vacío, y se niega a pisar un archivo existente.
- CA-90: una métrica declarada sin base se mide y congela en el primer
  `verify --full`: value queda escrito, history registra `frozen` y el
  gate pasa con la nota visible.
- CA-91: una regresión más allá de la tolerancia produce gate FAIL (y
  veredicto rojo) nombrando métrica, valor actual, base y delta exactos.
- CA-92: una mejora más allá de la tolerancia pasa y aprieta la base: el
  nuevo valor queda persistido con un registro `tightened`.
- CA-93: movimientos dentro de ±tolerancia pasan SIN tocar la base, en
  ambos sentidos (anti-ruido).
- CA-94: `direction: down` invierte la regla: subir es regresión (FAIL) y
  bajar aprieta la base.
- CA-95: el gate solo existe en corridas `--full` sin `--gate` y con
  métricas declaradas: un verify normal no lo incluye ni altera la base.
- CA-96: comando roto (exit != 0 o salida final no numérica) = gate ERROR
  fail-closed con el detalle, y la base no se modifica.
- CA-97: `hoom ratchet lower` sin `--reason` se niega; con razón, baja la
  base registrando `loosened` con la razón; un valor que mejora la base se
  rechaza.
- CA-98: `.hoom/ratchet.json` está fuera de la huella: un `verify --full`
  que aprieta la base deja el check VERDE (huella intacta).

## Decisiones

- Métrica = comando declarado, no extractor embebido: mismo contrato
  filosófico que hoom.yaml — el core agnóstico ejecuta y compara números;
  el proyecto sabe de dónde salen.
- La base vive en un archivo commiteado y NO en los veredictos: el diff de
  Git muestra cada apriete y cada aflojada en el PR; la historia del
  archivo es su auditoría, reforzada por `history` interno.
- Congelar en el primer `--full` en vez de exigir un init con medición:
  arranque sin fricción; `init` solo da el esqueleto.
- Solo `--full`: los comandos de métricas suelen ser caros (mutation
  completa) y una medición sobre corrida acotada no es comparable con la
  base. El trinquete vive en la nocturna.
- Excluir el archivo de la huella: el modelo de amenaza de la huella es la
  DERIVA del código verificado, no el estado del harness; incluirlo haría
  que cada apriete rompa el check inmediatamente después de verificar —
  castigar la mejora es lo contrario del propósito.
- Aflojar por comando aparte y no editando el JSON a mano: la edición
  manual queda a la vista en el diff igual, pero el comando exige la razón
  y la registra — el camino fácil es el camino auditado.

## Riesgos y deuda aceptada

- Métricas ruidosas con tolerancia mal calibrada pueden apretar sobre
  ruido y fallar en la vuelta al valor real; la tolerancia por métrica es
  la mitigación v1 y la experiencia dirá los defaults por perfil.
- Un comando de métrica lento alarga la nocturna (10 min de tope por
  métrica); aceptado: es el mismo costo que un gate más.
- La edición manual del JSON puede saltarse `lower` (queda en el diff de
  Git, pero sin razón estructurada); endurecerlo (hash, firma) sería
  ceremonia sin amenaza real en el modelo local-first.
- Sin visualización en status/Studio todavía: el gate y el archivo son la
  única vista v1; roadmap.
