// Package carreratest arma, SOLO para tests, las carreras contra la foto de
// la evidencia (.hoom/specs/evidencia-en-disco.md, CA-441/CA-442): un FIFO
// que nadie escribe, y un proceso que la corrida dejo vivo cambiando una
// entrada sin parar, con rename atomico, entre un archivo regular, un
// symlink, el FIFO mismo o un directorio.
//
// Existe solo para tests, como net/http/httptest: ningun codigo de
// produccion lo importa. Es un paquete y no un _test.go porque la misma
// carrera se usa en hoomfs, gitx y agentcmd, y tres copias de una fixture de
// goroutines, atomicos y cleanup terminan desincronizadas (hallazgo b08c53).
//
// Las carreras son de unix (mkfifo, un rename sobre una entrada que otro
// esta mirando): sus archivos van con //go:build !windows, y en windows el
// paquete queda vacio.
package carreratest
