// Package gittest arma, SOLO para tests, repos git en los que git no lanza
// nada por su cuenta (hallazgo 20261006T204527_30abfc). Despues de cada
// commit (y de merge, rebase, fetch, am) git lanza `git maintenance run
// --auto` —desde git 2.47 suelto en segundo plano, con --detach; antes de
// git 2.29, `git gc --auto`—, que crea y borra .git/objects/maintenance.lock
// mientras el test sigue: un proceso de mas por commit, y un archivo que
// aparece y desaparece debajo del que copie o fotografie el repo. En el CI,
// una de cada ocho corridas.
//
// Aca vive lo que un repo de prueba lleva para que eso no pase
// (SinMantenimiento, que ApagarMantenimiento anexa a su .git/config), lo que
// se le exige al que lo va a copiar (ExigirMantenimientoApagado), y con que
// se prueba que anda: git sin mas configuracion que la suya (EntornoSolo,
// RepoCrudo), lo que git lanza despues de un commit (CommitConTraza) y las
// formas de escribir el apagado (ConfigsDeMantenimiento).
//
// Tambien vive aca el git aislado de un binario de tests entero (AislarGit):
// lo que sus TestMain le sacan y le fijan al entorno antes de m.Run para que
// ni las variables de git de quien corre los tests ni su config cambien lo
// que el fixture planta o lee (hallazgos 20261029T172636_8f2253 y
// 20261029T172811_c4957f).
//
// Existe solo para tests, como net/http/httptest: ningun codigo de
// produccion lo importa. Es un paquete y no un _test.go porque lo usan
// varios paquetes (agentcmd, reviewcmd, boardcmd, gitx), y con una copia en
// cada uno el apagado, la lectura de GIT_TRACE y el aislamiento tenian
// varias fuentes de verdad que habia que mover juntas (hallazgos
// 20261007T142722_de562b y 20261029T172811_c4957f). Solo importa la
// biblioteca estandar, asi que tambien lo puede usar gitx. Cada paquete
// conserva su git(t, dir, ...), que con `init` llama a ApagarMantenimiento, y
// los tests de sus constructores; los del fixture mismo estan aca.
package gittest
