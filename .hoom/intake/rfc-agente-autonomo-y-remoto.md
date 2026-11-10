# hoom como agente autónomo y remoto — documento fuente v1

Fecha: 2026-11-10.
Tipo: documento fuente (intake). No es una spec y no tiene criterios de
aceptación. De acá salen varias specs, más o menos una por capacidad de §3.4
y por pieza de §4, y cada una escribe sus criterios con la numeración global
de siempre (la última usada es CA-587, de `pegado-como-texto`).
Origen: la conversación de diseño entre Henry y Claude del 2026-11-09 y el
2026-11-10, y el mockup "hoom en el bolsillo"
(https://claude.ai/artifact/PkosUqJ7qyBJvSPkjtETVr), hecho con la línea
gráfica hoomdev (https://claude.ai/artifact/88GUDoT11haapCo9uQh4M3).
Antecedentes: `rfc-cabina-visual-v2.md` (en este mismo directorio) y el RFC
"HoomAI Remote" del 2026-10-14 (fuera del repo). La §6 lista lo que este
documento cambia de ellos.
Para quién: el analista, que destila la visión y el backlog; el arquitecto,
que escribe cada spec; y el equipo que lo implementa.

## 0. En una frase

hoom deja de ser un harness que alguien maneja paso a paso y pasa a ser un
agente que **trabaja solo entre los dos momentos de Henry** (firmar la spec y
aceptar la tarjeta) **sin empezar a creerle al modelo**, y que se maneja
desde el celular a través de una **central propia**, como Claude Remote
Control.

## 1. Reglas innegociables

Las frases de Henry van textuales.

1. **Los dos momentos humanos no cambian.** Henry: "mis dos momentos no
   cambian, pero de ahí toda la autonomía sí tiene que funcionar". Los dos
   momentos son firmar la spec (columna Tu aprobación) y aceptar la tarjeta
   (columna Tu aceptación, acción Integrar). Ninguna regla de autonomía los
   automatiza (§3.1).
2. **hoom no le cree al modelo.** Henry: "El objetivo del harness es que no
   confío en […] la IA, en el modelo". AGENTS.md, regla 5: "La narracion no
   cuenta: solo cuenta la evidencia del veredicto".
3. **Solo por las CLI, con la suscripción.** Henry: "yo no quiero que se
   conecte por API a los proveedores de IA. Eso posiblemente sea en un
   futuro para dar la opción". hoom ejecuta Claude Code, Codex, Antigravity
   CLI y las demás como subproceso, con el login de cada una.
4. **Es para uso propio y es open source.** Henry: "yo solo necesito para
   mí. Y como esto es software libre […] otra persona lo va a hacer de esa
   misma manera pero para él". Cada persona instala su agente y su central,
   y usa su suscripción. Que todo comparta la misma ventana de 5 horas o la
   semanal no es un problema.
5. **El entorno de cada repo vive fuera del agente.** Henry aceptó que el
   entorno "esté afuera" del agente: vive en el repo, y hoom lo verifica sin
   administrarlo (A12).
6. **Instalar y configurar tiene que ser fácil.** Henry: "no quiero que
   nuestro agente sea complicado de instalar o de configurar", con un paso a
   paso que reconozca las CLI (A14).
7. **Nunca cambiar de CLI en silencio.** Solo se pasa a otra CLI por una
   regla escrita (la escalera de A3), y queda registrado en la evidencia. Si
   una CLI se queda sin sesión, se ofrece otra; no se cambia sola (A10).
8. **hoom nunca declara más de lo que verificó.** Lo que el agente hace solo
   queda marcado como hecho por una regla, nunca como hecho por una persona.
9. **CLI-first.** Toda funcionalidad nueva nace como verbo de la CLI, y cada
   botón del Studio, de la app y de la central ejecuta la misma función que
   ese verbo (README, sección "HoomAI Studio").

## 2. Punto de partida: lo que ya existe

| Pieza | Qué hace hoy | Dónde |
|---|---|---|
| Sobre (`hoom agent`) | Pasos deterministas spec → run → scope → verify → check. Con el test-writer ciego son seis | `internal/agentcmd` |
| Roles | Diez roles con contrato y forma de escritura: evidencia, tests, código o specs | `internal/agents/targets.go` |
| Gate de scope | Compara lo tocado antes y después del run. Un piso de solo agregar protege veredictos, hallazgos, aprobaciones, `hoom.yaml` y el trinquete | `internal/agentcmd/scope.go` |
| Test-writer ciego | Escribe los tests en un árbol sin la implementación | `internal/isolate` |
| Veredicto | Lo emiten gates deterministas sobre una huella del código. Un gate que ni siquiera arranca da ERROR, que es distinto de FAIL | `hoom verify`, `internal/gates` |
| Review cruzada | El reviewer nunca es la CLI que escribió. Los hallazgos se observan en disco | `hoom review` |
| Aprobación | Firma con la identidad git, atada al sha256 de la spec | `hoom spec approve`, `internal/approval` |
| Tablero | La columna de cada tarjeta se deriva de la evidencia. Son ocho columnas | `hoom item`, `hoom board`, `.hoom/items/` |
| Piloto `auto: hasta-humano` | Una cinta: encadena test-writer → writer → reviewer sin juicio y sin reintentos. Se detiene ante cualquier rojo, en las columnas humanas y cuando se acaba el presupuesto | spec `historia-doctor-y-cinta`, `boardcmd.PilotTable` |
| Studio (`hoom serve`) | API y UI embebida. Las acciones piden token. El tablero se refresca cada segundo mientras está abierto | `internal/servecmd` |
| Providers | claude, codex, gemini y opencode | `internal/providers` |

Lo que falta, en una línea: la cinta no reintenta, no sigue sola después de
la firma, se muere con `hoom serve`, no entrega un PR, no avisa y no se
puede manejar desde fuera de la máquina.

## 3. Parte A — Autonomía

### 3.1 Qué autonomía sí y cuál no

- **Autonomía de tiempo: sí.** El agente trabaja sin que la persona esté:
  de noche, mientras está en otra cosa, sin la compu delante.
- **Autonomía de juicio: nunca.** El modelo no decide qué se construye ni si
  quedó bien.

Cada decisión cae en uno de tres grupos:

| Decisión | Ejemplos | Quién decide |
|---|---|---|
| **Hacer** | cómo implementar, qué tests escribir, cómo arreglar un hallazgo | el modelo |
| **Avanzar** | qué sigue, si se reintenta, cuándo parar, cuánto se gasta | hoom, con reglas fijas |
| **Aceptar** | qué se construye (la spec) y qué entra al proyecto | la persona |

"¿Quedó bien?" nunca lo contesta el modelo: lo contesta el veredicto.

hoom pierde su objetivo el día que el modelo decide algo de "avanzar" o de
"aceptar". Por ejemplo, si la tarjeta avanza porque el modelo dijo "listo",
si el modelo elige saltarse al test-writer, si reintenta cuantas veces
quiere, o si a la persona le llega el resumen del modelo en vez del
veredicto.

**Sobre automatizar las firmas.** Queda fuera (§7). Son los únicos dos
controles que ninguna máquina puede hacer. Firmar la spec responde "¿esto es
lo que yo quería?", y todo lo que viene después sale de la spec: si el
arquitecto entendió mal, todo sale verde y todo está mal. Aceptar responde
"¿está bien de verdad?", y cubre lo que la spec no dice. Si algún día se
habilitara para cierto tipo de tarjeta, tendría que valer solo para ese
tipo, quedar escrito en la evidencia ("aprobada por regla automática, no
por <persona>") y poder deshacerse. La recomendación es no hacerlo y, en
cambio, abaratar los dos momentos: firmar desde el celu leyendo la spec
completa, y que al final llegue el PR con el veredicto listo para aceptar.

### 3.2 Principios de diseño

- **P1. El modelo nunca reporta: hoom observa.** Lo que el modelo dice
  ("terminé", "corrí los tests") no mueve nada. Cada paso se decide con
  algo que hoom mira por su cuenta: el veredicto en disco, el diff de git,
  los códigos de salida y los hallazgos que aparecen en `.hoom/findings/`.
- **P2. Las reglas las escribe la persona, antes, y el modelo no las
  toca.** La política de autonomía (§3.3) vive donde el gate de scope ya
  corta si un rol la modifica.
- **P3. Topes duros que no dependen del modelo:** presupuesto, intentos,
  tiempo por trabajo y corte cuando no hay avance (A4).
- **P4. Nadie se corrige a sí mismo.** Quien escribe el examen no lo rinde:
  el test-writer ciego escribe los tests y el writer no puede tocarlos
  (A1). Revisa otra CLI. La última palabra la tienen los gates, no un
  tercer modelo.
- **P5. Lo irreversible queda fuera del alcance del agente.** Solo propone:
  una rama y un PR borrador. No hace merge a main, no despliega y no ve
  credenciales. Que no pueda hacer push a main lo garantiza GitHub con la
  protección de la rama, no hoom confiando en sí mismo (A9). Se puede
  equivocar barato, nunca caro.
- **P6. Todo queda en disco.** Sobres, logs, veredictos, hallazgos e
  historia. Una noche de trabajo se puede reconstruir entera.
- **P7. La autonomía se gana con evidencia.** Se sube por niveles (§3.3), y
  la persona pasa al siguiente cuando el anterior lleva varias corridas
  dando buenos resultados. Es la misma idea del trinquete de hoom.

### 3.3 Niveles de autonomía y política

El nivel se elige por tarjeta, como hoy `auto: hasta-humano`, y nunca puede
pasar el máximo del proyecto.

| Nivel | Qué hace solo | Se detiene en |
|---|---|---|
| 0 · manual | nada: cada paso lo lanza la persona | — |
| 1 · cinta (lo de hoy) | encadena test-writer → writer → reviewer si todo sale verde | cualquier rojo, columnas humanas, presupuesto |
| 2 · reintenta | lo anterior, más el reintento ante un rojo con la escalera de A3 | tope de intentos, falta de avance, presupuesto, tiempo |
| 3 · corrige hallazgos | lo anterior, más el ciclo de hallazgos de la review (A5) | igual que el nivel 2 |
| 4 · de noche | lo anterior, más correr desde la cola (A8) y entregar el PR borrador (A9) | Tu aceptación |

Los nombres de los valores los fija la spec. En todos los niveles, la
cadena empieza después de la firma de la spec (A2) y termina en Tu
aceptación.

La política del proyecto vive en `hoom.yaml`. Este ejemplo es solo
ilustrativo; la forma exacta la decide la spec:

```yaml
autonomia:
  nivel_maximo: 2          # ninguna tarjeta puede pedir más que esto
  reintentos_ante_rojo: 2
  corte_sin_avance: true   # mismos tests fallando = parar
  minutos_por_trabajo: 45
  entrega: pr-borrador     # nunca main
```

- `hoom.yaml` ya está protegido por el piso del gate de scope: ningún rol
  puede cambiarlo.
- El nivel de una tarjeta se cambia con un verbo de la CLI (CLI-first), y
  con el mismo interruptor en el Studio y en la app. El roadmap del README
  ya pide "activar y apagar el piloto automático desde la tarjeta".
- El registro de cada sobre dice con qué nivel y por qué regla corrió.

### 3.4 Capacidades a construir

Cada una dice qué hay hoy, qué se propone, de qué depende y cómo se puede
probar. Las ideas de prueba no son criterios de aceptación: esos los
escribe la spec.

#### A1. El writer no toca los tests del test-writer

- **Hoy:** el writer tiene la forma `codigo`, que permite `**` y solo niega
  `.hoom/specs/**` (`internal/agentcmd/scope.go`, `defaultAllow` y
  `defaultDeny`). Puede editar o borrar los tests que escribió el
  test-writer ciego, y hoy lo frenan solo la regla escrita de AGENTS.md
  ("nunca debilites tests") y la review.
- **Por qué importa:** con reintentos automáticos, aflojar el test es la
  salida más barata que tiene un modelo frente a un rojo.
- **Propuesta:** en una tarea donde corrió el test-writer, los archivos que
  él dejó quedan negados para el writer. Si el writer toca uno, el gate
  corta por manipulación, igual que con `hoom.yaml`, y deja un hallazgo
  high.
- **A decidir en la spec:** los helpers y fixtures de test compartidos, y
  los tests de otras tareas que el cambio rompe a propósito. En ese caso el
  writer no los toca: se reporta, y decide la persona o vuelve el
  test-writer.
- **Depende de:** nada. Es requisito de A3 y A5.
- **Prueba:** un writer que modifica un test del test-writer cierra
  cortado, con un hallazgo high, y sin llegar a verify.

#### A2. La firma hace seguir la tarjeta

- **Hoy:** "Aprobar spec" (`approveCard` en
  `internal/servecmd/acciones.go`) firma, pero no llama al piloto: la cinta
  solo arranca cuando cierra un sobre. Después de firmar, la persona tiene
  que tocar "Pedir test-writer".
- **Propuesta:** si la tarjeta tiene nivel ≥ 1 y presupuesto, la firma
  dispara al test-writer. Esto no automatiza el momento humano: hace que el
  trabajo siga después de él. Tiene que valer igual si la firma viene de la
  terminal (`hoom spec approve`), y el roadmap del README ya pide que la
  cinta siga también después de un `hoom agent` de la terminal.
- **Depende de:** A7, para que la firma desde la terminal encuentre un
  proceso vivo que siga la cinta.
- **Prueba:** firmar en una tarjeta con piloto lanza el sobre del
  test-writer y deja una línea en el log del piloto. En una tarjeta sin
  piloto no lanza nada.

#### A3. Reintento ante un rojo: sesión nueva y escalera

- **Hoy:** la cinta no reintenta (reglas 6 y 8 de la tabla de la spec
  `historia-doctor-y-cinta`).
- **Por qué:** un modelo atascado se ancla a su propio diagnóstico, relee
  sus errores como si fueran hechos, y con el contexto largo atiende peor
  al error real. Henry lo vio en la práctica: "inicio una nueva sesión con
  el modelo ya fresco […] o un modelo de otra IA, y ese lo resuelve
  rapidito".
- **Propuesta (nivel ≥ 2).** Cuando un sobre del writer cierra en rojo
  porque verify dio FAIL:

  | Intento | Quién | Con qué |
  |---|---|---|
  | 1 | el writer | la spec |
  | 2 | la misma CLI, en una sesión nueva | la spec, la salida de los gates que fallaron y el diff actual. Trabaja sobre ese diff |
  | 3 | otra CLI que todavía no escribió en la tarjeta, en una sesión nueva | la spec y la lista de tests que fallaron. Arranca desde el commit base de la tarea; el diff fallido queda guardado como evidencia y no se le pasa |
  | — | se para y avisa | "rojo final": intentos, CLIs y tests que siguen fallando |

- **Reglas:**
  - Un reintento es siempre una sesión nueva, nunca `--resume` de la que
    falló.
  - Retomar la misma sesión queda solo para un trabajo INTERRUMPIDO (corte
    por caída o por sesión vencida, A7 y A10), no para un rojo.
  - Cada intento es un sobre propio, con su registro: piloto, número de
    intento y motivo. La historia los muestra.
  - Los números salen de la política (§3.3).
- **Depende de:** A1 y A6.
- **Prueba:** con un provider de prueba que falla una vez y después pasa,
  el segundo sobre corre en una sesión nueva con la salida del gate en el
  pedido, y la tarjeta avanza. Si falla las tres veces, la tarjeta se
  detiene con el aviso y la evidencia.

#### A4. Corte cuando no hay avance

- **Señales que mide hoom, nunca el modelo:**
  1. Fallan los mismos tests, por nombre, que en el intento anterior.
  2. El diff del intento tiene la misma huella que uno anterior.
  3. Se acabó el presupuesto de la tarjeta.
  4. Se acabó el tiempo por trabajo.
  5. Un gate dio ERROR (`verdict.StatusError`): no llegó ni a correr. Es un
     fallo de entorno, no se reintenta con modelos, y se avisa "entorno".
- **Hueco conocido:** un entorno roto que hace fallar los tests (por
  ejemplo, la base de datos no levantó) hoy se ve como FAIL, no como ERROR.
  Distinguirlo es parte de A12, con un gate de entorno que corre antes que
  los demás.
- **Regla de sospecha:** si dos CLI distintas fallan el mismo test, se para
  con "sospecha: el test o la spec" y decide la persona. Puede relanzar al
  test-writer, corregir la spec (que vuelve a firmarse) o tomarlo ella.
  Ningún modelo decide que el test está mal.
- **Precedente:** la práctica que ya se aplica a los ciclos de review. Si
  los hallazgos no bajan, se simplifica y se escala a la persona; no se
  sigue parcheando.

#### A5. Arreglo de los hallazgos de la review

- **Hoy:** `hoom review` registra los hallazgos y arreglarlos es manual. Ya
  existen la review por diferencia (spec `review-por-diferencia`) y las
  reglas de quién cierra un hallazgo (spec `quien-cierra-un-hallazgo`).
- **Propuesta (nivel ≥ 3):**
  1. El writer, en una sesión nueva, recibe los hallazgos abiertos.
  2. Corre verify.
  3. Corre una review por diferencia con un reviewer que no escribió.
  4. Se repite mientras los hallazgos bajen, hasta el tope de rondas.

  Un hallazgo que el writer refuta pasa por el refutador (otra CLI) y se
  cierra según las reglas que ya existen.
- **Corte:** si los hallazgos no bajan de una ronda a la siguiente, si se
  llega al tope de rondas o si se acaba el presupuesto.
- **Restricción:** el reviewer no puede ser ninguna CLI que haya escrito en
  la tarjeta. Si escribieron dos (por la escalera de A3), hace falta una
  tercera o la persona (§8, pregunta 3).
- **Depende de:** A1, A3 y A4.

#### A6. Política de autonomía y niveles

Es la §3.3 convertida en spec: el bloque de `hoom.yaml`, el nivel por
tarjeta, el verbo para cambiarlo, el interruptor en el Studio y en la app, y
el registro del nivel en cada sobre.
**Depende de:** nada. Va antes de A3.

#### A7. Un servicio que vive solo y retoma

- **Hoy:** la cinta vive dentro de `hoom serve`. Si se cae, el sobre en
  curso queda interrumpido y nadie lo relanza (lo dice la spec
  `historia-doctor-y-cinta`). `markOrphans` ya cierra los runs que
  murieron.
- **Propuesta:** hoom corre como servicio del sistema (launchd en macOS,
  systemd en Linux), arranca solo con la máquina y es el mismo proceso que
  usa `hoom remote` (§4.6).
- **Al volver de una caída:**
  - Lista los sobres INTERRUMPIDO.
  - Si el nivel de la tarjeta lo permite, si se conoce el id de la sesión y
    si la CLI sabe continuarla, la retoma.
  - Si no se cumple algo de lo anterior, avisa.
- **En la Mac:** la mantiene despierta mientras haya trabajo (por ejemplo,
  con `caffeinate -i`).
- **Además:** `hoom doctor` muestra si el servicio corre.
- **Depende de:** nada.

#### A8. Cola de trabajo y ventanas de cuota

- **Por qué:** el dolor original es que la ventana de 5 horas y la semanal
  vencen sin usarse los días en que la compu no se prende.
- **Propuesta:**
  - Las tarjetas firmadas con nivel 4 entran a una cola.
  - El servicio las lanza cuando la CLI tiene ventana: un trabajo a la vez
    por CLI (o lo que diga la política), primero por la `prioridad` de la
    tarjeta y después por orden de llegada.
  - Cada tarjeta trabaja en su worktree, así que tarjetas de distintas CLI
    pueden correr en paralelo.
- **Dato disponible:**
  - Claude emite `rate_limit_event`, con `rate_limit_info.unifiedWindows`
    (`five_hour` y `seven_day`); se verificó en septiembre con Claude Code
    2.1.263, y hay que confirmarlo de nuevo.
  - De Codex y Antigravity no hay un dato conocido.
- **Depende de:** A7 y A9.

#### A9. Entrega: rama y PR borrador

- **Hoy:** "Integrar" comprueba "una última vez que todo esté verde y
  guardado, y la tarjeta pasa a Hecho. Juntarlo con el proyecto lo hacés
  vos después" (`internal/servecmd/ui/acciones.js`). No hay push ni PR.
- **Propuesta:** un verbo de entrega (el nombre lo fija la spec) que hace
  push de la rama de la tarea y abre un PR borrador. La descripción del PR
  lleva el veredicto: su ruta y el resumen de los gates.
  - La credencial de la máquina no tiene permiso sobre main. Lo impone la
    protección de rama de GitHub.
  - El merge lo hace siempre la persona.
- **A decidir:** si aceptar desde el celu es Integrar más el merge del PR
  hecho por la persona en GitHub, o si Integrar hace el merge (§8,
  pregunta 4).
- **Depende de:** nada. Es requisito de A8.

#### A10. Sesiones de las CLI: vencimiento y re-login

- **Principio:** hoom nunca lee, guarda ni renueva credenciales. Cada CLI
  maneja su propio login.
- **Saber antes, sin gastar tokens.** Se chequea antes de cada run y cada
  cierto tiempo:
  - `claude auth status --json` devuelve `loggedIn` y `loginExpiresAt`, así
    que se puede avisar con días de anticipación.
  - `codex login status` solo dice si hay sesión.
  - Antigravity CLI (`agy`) no tiene comando de estado. Se puede probar con
    `agy models`, que no está verificado, o enterarse con el primer error.
  - El estado se ve en `hoom doctor`, en el tablero y en la app.
- **Si vence a mitad de un trabajo:**
  - El adaptador reconoce el error de "no autenticado".
  - El sobre queda **INTERRUMPIDO · sin sesión**, no ROJO: el código ni
    llegó a verificarse.
  - El worktree queda intacto y se guarda el id de la sesión.
  - El piloto pausa solo las tarjetas de esa CLI y no hay reintentos en
    loop.
  - Después del re-login, el trabajo se retoma en la misma sesión.
- **Re-login desde el celu:** hoom corre el login de la propia CLI en la
  máquina y solo le pasa a la persona la URL y el código.
  - `codex login --device-auth`
  - Antigravity muestra una URL y un código por SSH, según las guías; no
    está verificado.
  - Para Claude en una máquina sin navegador: `claude setup-token`, una
    sola vez. Según reportes es un token largo, de cerca de un año y solo
    para inferencia; hay reportes de errores 401 con `-p`, así que hay que
    verificarlo.
- **Nunca cambiar de CLI en silencio:** se ofrece "relanzar con otra CLI"
  como botón.
- **Plan B, solo documentado:** copiar el archivo de sesión desde otra
  máquina. Vale tanto como una contraseña, y no aplica a Claude en la Mac,
  que guarda la sesión en el llavero.
- **Depende de:** A11, para cubrir Antigravity.

#### A11. Adaptador de Antigravity CLI (`agy`)

- **Hoy:**
  - El provider `gemini` de hoom llama al binario `gemini`. Según guías de
    terceros, Gemini CLI dejó de funcionar para los planes gratis, Pro y
    Ultra el 2026-06-18, y su reemplazo para esas suscripciones es
    Antigravity CLI.
  - `internal/agents/targets.go` ya anota que Antigravity CLI todavía no
    carga subagentes.
- **Lo que trae `agy` 1.4.5 (verificado en su ayuda):** `-p`,
  `--output-format text|json|stream-json`, `--input-format stream-json`,
  `--model`, `--effort low..max`, `--mode accept-edits|auto|plan`,
  `--conversation <id>`, `-c`, `--dangerously-skip-permissions`,
  `--sandbox`, `--print-timeout` y `--json-schema`.
- **Propuesta:** un adaptador nuevo con las mismas intenciones que los
  demás (solo lectura, ejecución, desatendido), captura de la sesión para
  retomar, y medición de uso si la CLI la reporta.
- **Depende de:** nada.

#### A12. Entorno por repo y contenedor (para el droplet)

- **El contrato es un devcontainer en el repo.** Si el repo no tiene uno,
  el agente lo escribe como un cambio más, con su spec y su veredicto.
- **La CLI de IA corre dentro del contenedor.** Si no, los comandos del
  agente correrían en la máquina.
- **Un gate de entorno corre antes que los demás:** comprueba que el
  contenedor levanta y que los gates pueden correr. Así un entorno roto se
  ve como entorno y no como un test que falla (A4).
- **El contenedor es además el sandbox de proceso que la Spec D declaró
  como faltante.** Dentro del árbol ciego, `git show HEAD:<ruta>` todavía
  devuelve la implementación. Un árbol ciego dentro del contenedor y sin el
  historial de git cierra ese agujero.
- **Perfiles:** hoy existen `go`, `laravel`, `kmp` y `kmp-compose`. Python
  y Node llegan después.
- **Depende de:** la prueba en el droplet (§5, fase 4).

#### A13. Crear tarjetas sin la terminal, y pedidos por voz

- **Hoy:** las tarjetas solo se crean con
  `hoom item add "<título>" --pedido "…" --auto hasta-humano --presupuesto-usd <n>`.
  El Studio no tiene una ruta para crearlas.
- **Propuesta:** una ruta de la API que ejecute ese mismo verbo
  (CLI-first), con un tope de presupuesto obligatorio, igual que hoy con
  `--auto`.
- **Voz (fase posterior):**
  - La app graba y la máquina transcribe en local, con whisper.cpp.
  - La persona lee el texto antes de mandarlo, porque la transcripción
    puede fallar.
  - hoom nunca llama a una API de modelo.
- **Depende de:** nada. La voz depende de la app (§4.9).

#### A14. Instalación paso a paso

- **Propuesta:** un asistente `hoom setup` que hace, en orden:
  1. Detecta las CLI instaladas y el estado de su sesión (A10).
  2. Guía el login de cada una.
  3. Elige el perfil del repo.
  4. Instala el servicio (A7).
  5. Opcionalmente, vincula la máquina con la central (§4.5).

  Al terminar corre `hoom doctor` y muestra el estado completo.
- **Depende de:** A7, A10 y §4.5.

### 3.5 Qué atrapa cada error típico del modelo

| El modelo… | Lo atrapa | ¿Existe hoy? |
|---|---|---|
| dice que corrió los tests y no lo hizo | hoom corre los gates él mismo | sí |
| dice que terminó y no terminó | el veredicto y `spec_trace` (cada CA citado) | sí |
| inventa una función o una API que no existe | no compila o el test falla: ROJO | sí |
| toca cosas fuera de lo pedido | gate de scope | sí |
| toca la config, las aprobaciones o los veredictos | piso de solo agregar del gate de scope | sí |
| afloja un test para pasar | gate de scope sobre los tests del test-writer | **no (A1)** |
| se queda en un loop | tope de intentos y corte sin avance | **no (A3, A4)** |
| gasta de más | presupuesto por tarjeta | sí |
| hace algo destructivo | contenedor y credencial sin permiso sobre main | **no (A9, A12)** |
| se queda sin sesión a mitad de un trabajo | INTERRUMPIDO · sin sesión, y re-login | **no (A10)** |

### 3.6 El límite honesto

Nada de esto atrapa una spec mal hecha. Si la spec pide lo incorrecto, el
agente construye lo incorrecto, bien hecho y en verde, porque los tests
solo comprueban lo que la spec dice. Por eso la firma y la aceptación no se
automatizan. El objetivo no es que el modelo nunca se equivoque. Es que
todo lo que le llega a la persona esté respaldado por evidencia, y que cada
error sea barato y se pueda deshacer.

## 4. Parte B — hoom remoto

### 4.1 Objetivo

Manejar hoom desde el celular como se maneja Claude Code con Remote
Control: ver el tablero, lanzar, responder, cancelar, firmar la spec,
aceptar y pedir trabajo nuevo, en varias máquinas (primero la Mac prendida,
después un droplet) y en varios proyectos. Henry va a montar la central en
uno de sus servidores en la nube, bajo un subdominio propio
(`app.<dominio>`), con una página web que tiene la interfaz del mockup.

### 4.2 Cómo lo hace Claude (referencia)

Verificado en la ayuda de Claude Code 2.1.319 y en fuentes de terceros:

- `claude remote-control` es un servidor persistente que **corre en la
  máquina**. Según su ayuda, "Claude and its commands run on this
  computer; claude.ai and the app only show the session". Si la máquina se
  duerme o el comando se detiene, la sesión se detiene.
- **La máquina abre una conexión de salida** hacia Anthropic y registra la
  sesión con la cuenta. No abre puertos, así que funciona detrás de
  cualquier router.
- **El celular entra al mismo servidor con la misma cuenta.** Por eso el
  servidor sabe qué sesiones hay y en qué máquina: por defecto, el nombre
  sale del hostname.
- **El servidor reenvía los mensajes en los dos sentidos.** Funciona como
  un punto de encuentro (relay).
- Antigravity CLI tiene lo mismo: `agy remote-control start|status|stop`,
  con `--name` para el nombre de la máquina.

### 4.3 Tres caminos y el elegido

| Camino | Qué es | Uso |
|---|---|---|
| Red privada (Tailscale) + `hoom serve -addr` | el celu abre el Studio de la máquina por una VPN privada | **fase 0**: prueba sin código (Henry, 2026-11-11). Queda como alternativa permanente para quien no quiera montar una central |
| **Central propia** en `app.<dominio>` | hoom en cada máquina sale hacia un servidor de la persona, y el celu entra al mismo | **destino** |
| Remote Control de cada CLI | se controla la sesión de claude o de agy desde su propia app | **descartado** como remoto de hoom: se habla directo con el modelo y se saltan el sobre, el veredicto y el tablero |

**Decisión de diseño:** la app habla siempre con la **API del Studio**. El
camino (directo por Tailscale o por la central) es intercambiable, así que
la app, los avisos y la autonomía se construyen una sola vez.

### 4.4 Arquitectura

```
  Celular (navegador o app instalada)        app.<dominio>
        │  HTTPS · login con passkey
        ▼
 ┌──────────────────────────────── Central ─────────────────────────────────┐
 │ cuenta · dispositivos · máquinas · emparejamiento · relay · avisos ·      │
 │ auditoría · sirve la app                                                  │
 └───────────────────────────────────────────────────────────────────────────┘
        ▲  conexión de salida persistente (la abre siempre la máquina)
        │
 Mac de Henry ── servicio `hoom remote` ── API del Studio ── hoomai, oceanbuild…
 Droplet ─────── servicio `hoom remote` ── API del Studio ── …
```

La máquina es siempre la que llama, porque la central no puede llamarla:
está detrás del router. La verdad está siempre en la máquina (git y
`.hoom/`). La central solo pasa mensajes.

### 4.5 Comandos nuevos de hoom

| Comando | Qué hace |
|---|---|
| `hoom remote url <URL>` | Guarda la URL de la central para esta máquina. Comprueba que sea HTTPS y que sea una central de hoom con una versión de protocolo compatible. Sin argumento, muestra la URL actual. Henry: "posiblemente en un futuro quisiéramos cambiar esa URL porque tal vez me mudo de subdominio" |
| `hoom remote link [--name "Mac de Henry"]` | Empareja la máquina: pide a la central un código de un solo uso, lo muestra (también como QR) y espera a que la persona lo confirme desde la app con su sesión iniciada. Guarda la credencial de la máquina |
| `hoom remote on` | Dentro de un repo: agrega ese proyecto a los que expone la máquina, se asegura de que el servicio corra y conecta |
| `hoom remote off [--all]` | Saca el proyecto actual. Con `--all`, desconecta la máquina entera |
| `hoom remote status` | Muestra la URL, si la máquina está vinculada y conectada, el último latido, los proyectos expuestos y la versión |
| `hoom remote unlink` | Borra la credencial local y la revoca en la central |

- **En el Studio:** un botón "Remote" (encendido o apagado) que ejecuta el
  mismo verbo (CLI-first).
- **En `hoom doctor`:** el estado remoto.
- **Dónde se guarda:** en la configuración de usuario de la máquina
  (`os.UserConfigDir()`: `~/Library/Application Support/hoom/` en macOS y
  `~/.config/hoom/` en Linux), no en el repo. Son datos de la máquina, no
  del proyecto. Además, `hoom.yaml` está versionado: la credencial se
  filtraría.
- **La credencial de la máquina:** en el llavero del sistema si lo hay, si
  no en un archivo con permisos 0600.
- **Cambiar de subdominio sin romper los passkeys:** los passkeys quedan
  atados al dominio. Conviene registrarlos sobre el dominio raíz (`rpId` =
  `<dominio>`), así se puede pasar de `app.<dominio>` a otro subdominio sin
  perderlos. Cambiar de dominio raíz sí obliga a registrar los passkeys de
  nuevo y a volver a vincular las máquinas.
- **Los nombres de los verbos** los fija la spec (§8, pregunta 12).

### 4.6 El servicio en la máquina

- **Un solo proceso y una sola conexión por máquina.** Es el mismo servicio
  de A7.
- **Varios proyectos.** Hoy `hoom serve` atiende un solo proyecto. El
  servicio atiende la API del Studio de cada proyecto expuesto, y la app
  elige el proyecto (pantalla Proyectos).
- **Sin lógica paralela.** Cada pedido que llega de la central se ejecuta
  con los mismos handlers de `hoom serve`. Cada botón sigue siendo su
  verbo.
- **Lista fija de rutas, en el binario.** La central no puede cambiarla.
  - Pasan las lecturas: tablero, tarjeta, timeline, terminal de solo
    lectura, runs, sobres, veredictos, specs, hallazgos, estado, reporte y
    archivos.
  - Pasan las acciones de las tarjetas: lanzar, responder, cancelar,
    aprobar, comentar la spec, integrar, crear tarjeta (A13) y cambiar el
    nivel (A6).
  - **No pasa `POST /api/runs`.** Es la corrida libre del Cockpit, fuera
    del sobre, y por la central solo pasa trabajo dentro del sobre.
  - Subir documentos al intake queda para decidir.
- **Auditoría propia.** La máquina guarda su propio registro, de solo
  agregar, de cada pedido remoto: dispositivo, ruta, hora y resultado. No
  depende del registro de la central.
- **Eventos.** La máquina avisa a la central cuando cambia el tablero,
  termina un sobre, sale un veredicto, una tarjeta llega a una columna
  humana, una CLI se queda sin sesión o se acaba un presupuesto. Los deriva
  hoom, no el modelo.
- **Conexión.** Se reconecta con espera creciente y manda un latido. Si la
  máquina no está, la app muestra "sin conexión", como `hoomdev-web` en el
  mockup.

### 4.7 La central

**Hace:**

1. **Cuenta de una persona.** Es de un solo usuario por diseño: cada uno
   tiene la suya. El login es con passkey (WebAuthn), con códigos de
   recuperación y sin contraseña. La primera cuenta se reclama con un
   código que la central imprime en su consola al instalarse, para que
   nadie más pueda adelantarse.
2. **Dispositivos.** Cada celular o navegador con sesión es un dispositivo
   que se puede revocar.
3. **Máquinas.** Por cada una guarda el id, el nombre, el hash de su
   credencial, la versión de hoom, los proyectos expuestos, las CLI con el
   estado de su sesión, el último latido y si está conectada.
4. **Emparejamiento.** Un código corto de un solo uso, que vence en minutos
   y se confirma desde la app con la sesión iniciada.
5. **Relay.** Mantiene abierta la conexión de cada máquina, reenvía los
   pedidos de la app y las respuestas (separados por id), y transmite a la
   app los eventos en vivo.
6. **Avisos.** Web Push al celular, con un texto corto y sin código (§4.12).
7. **Auditoría.** Un registro de solo agregar de cada pedido que pasó:
   dispositivo, máquina, proyecto, ruta, hora y resultado.
8. **Sirve la app.** Los archivos estáticos van embebidos en su binario.

**No hace:**

- No guarda código, specs ni diffs. Pasan por la central y no se quedan.
- No ejecuta nada ni habla con modelos.
- No firma por la persona. Las firmas las verifica la máquina (§4.11).
- No decide nada del flujo. Deciden la máquina y sus reglas.
- No es un servidor de sincronización. git sigue siendo el transporte
  (§6).

### 4.8 Protocolo entre la máquina y la central (v1)

Tiene versión (`hoom-remote/1`). Si las versiones no son compatibles, el
error lo dice con claridad y no se intenta adivinar.

| Mensaje | Sentido | Contenido |
|---|---|---|
| `hello` | máquina → central | id, versión de hoom, versión del protocolo, proyectos, CLI y estado de su sesión |
| `ping` / `pong` | ambos | latido (por ejemplo, cada 20 s) |
| `request` | central → máquina | id, dispositivo, proyecto, método, ruta, cuerpo y, si es una firma humana, la firma del dispositivo |
| `response` | máquina → central | id, estado HTTP y cuerpo |
| `event` | máquina → central | tipo, proyecto, tarjeta y resumen corto |
| `bye` | ambos | motivo |

**Transporte (a decidir, §8 pregunta 7).** hoom hoy depende solo de
`golang.org/x/sys`, `golang.org/x/term` y `gopkg.in/yaml.v3` (`go.mod`).
Hay dos opciones:
- **WebSocket:** necesita una dependencia nueva.
- **HTTP de larga espera con respuestas en streaming:** se hace solo con la
  biblioteca estándar, y es parecido a lo que describen las fuentes sobre
  Remote Control ("polls for work").

### 4.9 La interfaz web (`app.<dominio>`)

Sale del mockup "hoom en el bolsillo". Los datos del mockup son de ejemplo.

| Pantalla | Para qué | Datos | Acciones |
|---|---|---|---|
| Proyectos | elegir máquina y proyecto | central: máquinas y proyectos | elegir |
| Inicio | ver todo de un vistazo | `/api/status`, `/api/verdicts`, `/api/board` | — |
| Tablero | la pila de tarjetas | `/api/board` | abrir una tarjeta |
| Sobre | los pasos del trabajo en curso | `/api/envelopes`, `/api/runs/{id}/stage` | cancelar |
| Decisión | responder corto la pregunta de un rol | `/api/runs/{id}` | `POST /api/runs/{id}/input` |
| Firma | leer la spec completa y firmarla | `/api/specs`, `/api/board/{slug}` | aprobar (con firma del dispositivo) o comentar (`/review`) |
| Rojo / Avisos | qué falló y con qué evidencia | `/api/verdicts`, `/api/board/{slug}/timeline` | dejar para después o relanzar |
| Pedir | crear una tarjeta, escrita o por voz | — | crear tarjeta (A13) |
| Aceptar (nueva) | ver el veredicto final y aceptar | `/api/board/{slug}`, `/api/verdicts` | integrar (con firma del dispositivo) |
| Máquinas (nueva) | vincular, ver cuáles están conectadas, revocar | central | vincular, revocar |

**Reglas de la app:**

- **Solo se firma en una pantalla que muestra la spec completa** (celular o
  compu). El aparato chico de 480×480 del mockup no firma ("acá no se
  firma").
- **Todo dato que llega de la máquina se pinta como texto.** El Studio ya
  tuvo un hallazgo de XSS por un estado que venía de afuera.
- **Sin recursos de red de terceros.** Como el Studio de hoy ("cero assets
  de red"), todo va dentro del build.
- **Es una PWA instalable** (manifest y service worker), para poder recibir
  avisos y abrirse como una app.
- **El transporte es intercambiable:** la misma app habla directo con
  `hoom serve` (Tailscale) o a través de la central.
- **La línea gráfica es hoomdev** (los tokens del design system).

### 4.10 Tecnología

**La central (recomendación): Go, en este mismo repo.**

- Comparte con el agente los tipos del protocolo.
- Es un binario sin runtime.
- Es fácil de instalar para "cada uno el suyo".
- Puede ser un subcomando (`hoom central`) o un binario aparte (§8,
  pregunta 6).

| Pieza | Recomendación | Por qué |
|---|---|---|
| Lenguaje | Go | el mismo de hoom; se comparte el código del protocolo |
| Datos | archivos JSON con escritura atómica (ya existe `hoomfs.AtomicWrite`) | es una persona con pocos registros: no hace falta base de datos, y SQLite agregaría una dependencia |
| HTTPS | Caddy adelante, con certificados automáticos | lo más simple y probado |
| Arranque | systemd | es lo estándar en el droplet |
| Login | passkeys (WebAuthn). El navegador entrega la clave pública en SPKI (`getPublicKey()`) y la verificación ES256 se hace con la biblioteca estándar | sin contraseñas y sin dependencias; hay que confirmarlo en la spec |
| Avisos | Web Push con VAPID. El cifrado (ECDH P-256, HKDF y AES-GCM) está en la biblioteca estándar de Go 1.25 | sin un servicio de terceros |

**Alternativa considerada: Laravel con Reverb.** El equipo ya conoce
Laravel, pero sería un segundo stack, con el runtime de PHP en el servidor
y el protocolo duplicado en dos lenguajes. Además le haría más difícil a
otra persona instalar su propia central.

**La app (pregunta para Henry, §8 pregunta 5):**

| Opción | A favor | En contra |
|---|---|---|
| Vue 3 + Vite + TypeScript | la app tiene muchas pantallas, estado (máquinas, proyectos, eventos en vivo, voz) y transiciones; Vue es familiar en equipos que usan Laravel | trae una cadena de build de Node al repo, que hoy es Go puro |
| JavaScript sin framework, como el Studio actual | sin build ni dependencias, y consistente con el Studio | se vuelve difícil de mantener a medida que crece |

Recomendación: Vue 3 + Vite + TypeScript, en una carpeta propia con su
build. El resultado se embebe en la central (`go:embed`) y, si se quiere,
también en `hoom serve`.

### 4.11 Seguridad: tampoco le creemos a la central

La central puede apretar botones en la máquina, y esos botones ponen a
trabajar código con las CLI de la persona. Por eso se diseña como si la
central pudiera fallar.

| Amenaza | Defensa |
|---|---|
| Alguien entra a la cuenta de la central | Passkey sin contraseña y sesiones por dispositivo que se pueden revocar. La central solo puede usar la lista fija de rutas (§4.6). Las dos firmas humanas piden la firma del dispositivo, que verifica la máquina |
| La central queda comprometida | No puede ejecutar comandos libres, no puede firmar y no tiene código guardado. La máquina lleva su propia auditoría, y `hoom remote off` corta la conexión |
| Roban el celular | El passkey pide huella o PIN. El dispositivo se revoca desde otra sesión |
| Se filtra la credencial de una máquina | Está en el llavero o en un archivo 0600 y se puede revocar. Solo sirve para conectarse como esa máquina, no para dar órdenes |
| Se repite un pedido viejo | Ids únicos con vencimiento. Las firmas humanas van atadas a un desafío nuevo y al sha256 |
| Se firma una spec distinta de la que se leyó | El celular manda el sha256 de la spec que mostró, y la máquina rechaza la firma si la spec ya no es esa. Hoy `approveCard` no recibe un sha: hay que agregarlo |
| Trabajo libre a través de la central | La corrida libre (`POST /api/runs`) no pasa |
| XSS en la app | Todo se pinta como texto y hay una CSP estricta |
| Gasto descontrolado | Los topes los aplica la máquina, no la central |
| Datos sensibles en los avisos | Solo texto corto, sin código ni rutas internas |

La central va en un servidor aparte, no junto a otros sistemas: si se
compromete otro sistema de ese servidor, no debe quedar abierto un camino
hacia la Mac.

### 4.12 Avisos

Cada aviso lleva un enlace directo a la tarjeta:

| Evento | Texto de ejemplo |
|---|---|
| spec lista para firmar | "Tarjeta 1: la spec está lista para firmar" |
| tarjeta lista para aceptar | "Tarjeta 1: verde, lista para aceptar" |
| rojo final (se agotó la escalera) | "Tarjeta 1: sigue en rojo después de 3 intentos" |
| corte por falta de avance o por sospecha del test | "Tarjeta 1: dos CLI fallan el mismo test, te toca decidir" |
| CLI sin sesión, o sesión por vencer | "claude sin sesión: entrá desde el celu" / "claude vence en 2 días" |
| presupuesto agotado | "Tarjeta 1: se acabó el tope de 5 USD" |
| entorno roto | "oceanbuild: el entorno no levanta" |
| máquina desconectada (lo detecta la central) | "Mac de Henry sin conexión desde hace 10 min" |

En la fase de Tailscale, sin central, los avisos pueden salir por ntfy, una
app gratuita que también se puede hospedar uno mismo.

### 4.13 Despliegue de la central (los pasos de Henry)

1. Crear un droplet chico, aparte de los de otros sistemas.
2. Apuntar el DNS de `app.<dominio>` a ese droplet.
3. Instalar hoom con el mismo instalador y correr el comando de
   inicialización de la central, con el dominio. Esto genera la
   configuración, las claves de los avisos y el código para reclamar la
   cuenta.
4. Poner Caddy adelante y registrar el servicio en systemd.
5. Abrir `app.<dominio>`, reclamar la cuenta con el código y crear el
   passkey.
6. En la Mac: `hoom remote url https://app.<dominio>`, después
   `hoom remote link` y, en cada repo, `hoom remote on`.

## 5. Plan por fases

| Fase | Qué | Valida | Depende de |
|---|---|---|---|
| 0 | **Prueba sin código** (Henry, 2026-11-11): el Studio desde el celular por Tailscale (`hoom serve -addr <IP de Tailscale>:4666`). Las molestias que salgan se guardan como un documento fechado en `.hoom/intake/` | que manejar hoom desde el celular sirve | — |
| 1 | **Autonomía núcleo, en la Mac:** A1 → A6 → A2 → A3 y A4 → A5 | los niveles 2 y 3 | A1 primero |
| 2 | **Vivir solo y entregar:** A7, A9 y A8 | el nivel 4 en la Mac | fase 1 |
| 3a | **La app nueva** sobre la API del Studio, probada por Tailscale, más A13 (crear tarjetas) y los eventos en vivo | la interfaz del mockup con datos reales | fase 0 |
| 3b | **La central y `hoom remote`:** cuenta, emparejamiento, relay, avisos, firma del dispositivo y sha en la aprobación | el celular sin Tailscale | 3a, A7 |
| 4 | **El droplet:** A10, A11, A12 y A14, más una prueba en un droplet real que mida los logins sin navegador, cuánto duran de verdad las sesiones y qué mensaje da cada CLI al vencer (forzándolo con logout) | el agente en Linux, sin la Mac | fases 2 y 3b |
| 5 | **Voz** (A13) y los perfiles de Python y Node | — | 3a, A12 |

- **La prueba en el droplet de la fase 4 no necesita código.** Puede
  hacerse antes, en paralelo, y le quita riesgo a esa fase.
- **Cada capacidad pasa por el flujo de siempre:** el arquitecto escribe la
  spec, Henry la firma, el test-writer ciego escribe los tests, el writer
  implementa, verify y la review cruzada. Así, este documento se construye
  con el mismo método que propone.

## 6. Qué cambia respecto de documentos anteriores

- **`rfc-cabina-visual-v2.md`, tabla "Punto por punto".** Decía: "Servidor
  de sync, multiusuario, WebSocket, Jira/Linear (fase 6) → Fuera. git es el
  transporte. No hay servidor". **Ahora hay un servidor, la central, pero
  no sincroniza nada:** no guarda estado del proyecto, git sigue siendo el
  transporte y la verdad sigue estando en la máquina. Siguen fuera el
  multiusuario y Jira/Linear.
- **Spec `historia-doctor-y-cinta`.** La cinta es "sin juicio y sin
  reintentos". **A3 agrega reintentos por regla.** Lo de "sin juicio" no
  cambia: el reintento lo decide una regla escrita por la persona, no el
  modelo.
- **RFC "HoomAI Remote" (2026-10-14).**
  - Dejaba los gestores de entorno y de dependencias fuera de hoom. **Se
    mantiene:** el entorno vive en el repo y hoom lo verifica (A12).
  - El canal Telegram + n8n **se reemplaza** por la app y la central.
- **README, sección "HoomAI Studio":** "Es un control remoto, jamas un
  segundo cerebro". **Se mantiene** para la app y para la central.

## 7. No-goals

- Automatizar la firma de la spec o la aceptación (§3.1).
- Llamar a APIs de modelos. Las API keys pueden ser una opción futura,
  fuera de este documento.
- Varios usuarios o equipos en una central, o venderla como servicio.
- Que la central guarde código o sincronice proyectos.
- Que el agente haga merge a main o despliegue.
- Gestores de entorno o de paquetes propios de hoom.
- Manejar hoom a través del Remote Control de las CLI.
- Una shell remota o comandos libres a través de la central.

## 8. Preguntas abiertas (PREGUNTAS PARA HENRY)

Cada una trae la recomendación de Claude, que no es una decisión.

1. **Valores por defecto de la política:** cuántos reintentos, cuántos
   minutos por trabajo y qué nivel máximo. Recomendación: 2 reintentos, 45
   minutos y nivel máximo 2 hasta que la fase 1 tenga evidencia.
2. **¿Desde dónde arranca el intento con otra CLI?** Recomendación: la
   misma CLI sigue sobre el diff y la otra CLI arranca del commit base (A3).
3. **¿Quién revisa cuando escribieron dos CLI?** Recomendación: una tercera
   CLI si está disponible; si no, la persona.
4. **Aceptar a distancia:** ¿Integrar sigue sin hacer el merge, como hoy,
   y la persona mergea el PR en GitHub, o Integrar hace el merge?
   Recomendación: Integrar no mergea, y el merge lo hace la persona en
   GitHub.
5. **Tecnología de la app:** ¿Vue 3 + Vite + TypeScript, o sin framework
   como el Studio? Recomendación: Vue 3 (§4.10).
6. **La central:** ¿subcomando del mismo binario (`hoom central`) o binario
   aparte? Recomendación: el mismo binario, para instalar una sola cosa.
7. **Transporte:** ¿WebSocket, que trae una dependencia nueva, o HTTP de
   larga espera con la biblioteca estándar? Recomendación: la biblioteca
   estándar, para mantener la política de dependencias mínimas.
8. **Firma del dispositivo en las dos firmas humanas:** ¿desde la v1 o
   después? Recomendación: desde la v1, porque es lo que impide que la
   central firme por la persona.
9. **Avisos:** ¿Web Push propio desde la central, o ntfy? Recomendación:
   Web Push en la central, y ntfy solo en la fase de Tailscale.
10. **Cola:** ¿cuántos trabajos a la vez por CLI, y por prioridad o por
    orden de llegada? Recomendación: uno por CLI, primero por prioridad y
    después por llegada.
11. **¿Dónde corre la central?** Recomendación: en un droplet aparte, no en
    uno de los servidores de otros sistemas.
12. **Nombres de los verbos** (`hoom remote url|link|on|off|status|unlink`,
    el verbo de entrega y el del nivel de la tarjeta).
13. **¿La app reemplaza al Studio o conviven?** Recomendación: que
    convivan por ahora. El Studio es la vista de la compu y la app es la
    del celular.

## Anexo A — Hechos verificados (2026-11-10, en la Mac de Henry)

- **Claude Code 2.1.319:**
  - `claude auth status --json` devuelve `loggedIn`, `authMethod`,
    `loginExpiresAt`, `subscriptionType` y `planTier`.
  - Existe `claude setup-token` ("long-lived authentication token
    (requires Claude subscription)").
  - `claude remote-control` es un servidor persistente que corre en la
    máquina. El prefijo del nombre de sesión es, por defecto, el hostname.
- **Codex 0.160.0:** existen `codex login --device-auth`,
  `--with-api-key`, `--with-access-token` y `codex login status`.
- **Antigravity CLI (`agy`) 1.4.5:**
  - No tiene subcomando de login ni de status.
  - Tiene las opciones listadas en A11.
  - `agy remote-control start|status|stop`, con `--name` y `--session`.
- **Gemini CLI 0.46.0:** está instalado, y el provider `gemini` de hoom
  llama a ese binario.
- **hoom:**
  - Providers: claude, codex, gemini y opencode. Perfiles: go, laravel, kmp
    y kmp-compose.
  - `hoom serve -addr` escucha por defecto en `127.0.0.1:4666`. Con una
    dirección que no es loopback avisa que el token viaja sin cifrar. En
    loopback, `SoloLocal` rechaza con 421 cualquier Host que no sea local.
  - El token de acciones se genera al azar en cada arranque. El navegador
    lo guarda en `localStorage`, pero si `hoom serve` se reinicia hay que
    volver a pegarlo.
  - El tablero del Studio se refresca cada segundo mientras está abierto.
    No hay avisos ni push.
  - `approveCard` firma con la identidad git del árbol de la tarjeta, no
    llama al piloto y no recibe un sha.
  - Existe `POST /api/specs/{name}/review`, para comentar la spec.
  - El writer tiene la forma `codigo`: puede todo salvo `.hoom/specs/**`.
  - Las reglas de la cinta están en la spec `historia-doctor-y-cinta`:
    "una cinta transportadora, no un orquestador".
  - No hay una ruta para crear tarjetas.
  - Un gate que no pudo arrancar da `StatusError`.
  - `go.mod` depende solo de `x/sys`, `x/term` y `yaml.v3`.

## Anexo B — Por verificar

- El login sin navegador de cada CLI en Linux, cuánto dura de verdad la
  sesión y qué mensaje exacto da cada una al vencer.
- Si `loginExpiresAt` de Claude se renueva solo con el uso.
- Si `claude setup-token` funciona con `-p` en una suscripción Max (hay
  reportes de errores 401).
- El login de `agy` por SSH, y si `agy models` consulta al servidor.
- Las opciones de `codex exec resume` en la versión actual.
- Si `tailscale serve` choca con `SoloLocal`.
- Web Push en una PWA de iOS: tiene que estar agregada a la pantalla de
  inicio (iOS 16.4 o posterior).
- Si hoy queda registrado qué rutas trasplantó el sobre del test-writer,
  algo que A1 necesita.
- Si `rate_limit_event` de Claude sigue trayendo `unifiedWindows`.

## Anexo C — Glosario

- **Sobre:** el ciclo determinista que hoom pone alrededor de un run de una
  CLI (spec → run → scope → verify → check).
- **Gate:** una comprobación determinista (tests, análisis, compilación).
  Da PASS, FAIL o ERROR.
- **Veredicto:** el resultado de los gates sobre una huella del código. Rojo
  significa que no se entrega.
- **Gate de scope:** compara lo que tocó un rol con lo que su contrato le
  permite.
- **Piso:** lo que ningún rol puede tocar nunca (veredictos, hallazgos,
  aprobaciones, `hoom.yaml`, trinquete).
- **Árbol ciego:** el worktree del test-writer, sin la implementación.
- **Tarjeta (item):** una unidad de trabajo del tablero. Su columna se
  deriva de la evidencia.
- **Cinta / piloto:** el encadenamiento automático de roles de una tarjeta.
- **Central:** el servidor propio que conecta el celular con las máquinas.
- **Relay:** un punto de encuentro que reenvía mensajes entre dos partes que
  no pueden llamarse directamente.
- **Emparejar (link):** vincular una máquina con la cuenta de la central
  mediante un código de un solo uso.
- **Passkey:** una credencial sin contraseña, atada al dominio y desbloqueada
  con huella o PIN.
- **PWA:** una página web que se instala como app y puede recibir avisos.

## Fuentes externas

- Claude Code Remote Control:
  https://dev.to/chwu1946/deep-dive-how-claude-code-remote-control-actually-works-50p6 ·
  https://recursivedev.github.io/ReCursor/research/claude-remote-control-2026-03-17 ·
  https://www.helpnetsecurity.com/?p=358918 ·
  https://www.penligent.ai/hackinglabs/hi/claude-code-remote-control-security-risks-when-your-local-session-becomes-a-remote-interface/
- Antigravity CLI:
  https://github.com/google-antigravity/antigravity-cli/issues/78 ·
  https://pasqualepillitteri.it/en/news/3422/antigravity-cli-agy-install-migrate-gemini-cli ·
  https://computingforgeeks.com/install-antigravity-cli-linux-macos-windows/
- `claude setup-token`:
  https://gist.github.com/coenjacobs/d37adc34149d8c30034cd1f20a89cce9 ·
  https://claudeissues.com/issue/65320-setup-token-oauth-token-returns-401-invalid-bearer-token-with-claude-p-on-max-su
- Autenticación de Codex:
  https://codex.danielvaughan.com/2026/04/01/codex-cli-authentication-flows-credential-management/ ·
  https://blog.laozhang.ai/en/posts/codex-headless-login
