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
// Existe solo para tests, como net/http/httptest: ningun codigo de
// produccion lo importa. Es un paquete y no un _test.go porque lo usan
// agentcmd y reviewcmd, y con una copia en cada uno el apagado y la lectura
// de GIT_TRACE tenian dos fuentes de verdad que habia que mover juntas
// (hallazgo 20261007T142722_de562b). Cada paquete conserva su git(t, dir,
// ...), que con `init` llama a ApagarMantenimiento, y los tests de sus
// constructores; los del fixture mismo estan aca.
package gittest
