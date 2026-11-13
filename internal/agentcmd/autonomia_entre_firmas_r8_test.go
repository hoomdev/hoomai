// Tests de regresion de la ronda 8 de autonomia-entre-firmas sobre la
// huella de un archivo grande del material de git (aclaracion 29, que
// re-expresa la primera vineta de la 28):
//
//   - hallazgos 20261113T055721_d6de47 y 20261113T060147_f835e8 (CA-638): un
//     writer que sobrescribe en sitio un hook de mas de 1 MiB con otros bytes
//     del mismo largo y le restaura el mtime cambia su ctime, y eso cuenta
//     como cambiarlo: el sobre corta en scope como en la aclaracion 25.
//
// Reusa los helpers de las rondas anteriores (aeaProyecto, ar1Codigo,
// ar6Ruta, ar6Escribir, ar6Cortado). CLIs falsos (fakeProvider).
package agentcmd

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ar8Largo es el largo del hook grande: 2 MiB, mas que el MiB de la
// aclaracion 28.
const ar8Largo = 2 << 20

// Hallazgos 20261113T055721_d6de47 y 20261113T060147_f835e8 (CA-638,
// aclaraciones 25 y 29): en el directorio de hooks comun del repo de la
// tarea hay un hook regular de 2 MiB (pre-push) con una fecha de
// modificacion fija. Un writer de CLI falso cambia codigo, espera unos
// milisegundos (para que su escritura caiga despues de la foto previa),
// sobrescribe en sitio un byte de ese hook con otro (mismo largo, mismo
// modo, otros bytes) y le restaura la fecha de modificacion original. Su
// sobre corta en scope, no-entregable, exit 1, sin veredicto, con un
// hallazgo high abierto del gate de scope que nombra hooks/pre-push.
func TestHallazgosD6de47_F835e8_CA638_SobrescribirUnHookGrandeYRestaurarSuFechaCorta(t *testing.T) {
	const ca = "d6de47/f835e8/CA-638"
	root, wt := aeaProyecto(t, "true", "")
	hook := ar6Ruta(t, wt, "--git-path", "hooks/pre-push")
	pre := "#!/bin/sh\nexit 0\n#"
	orig := pre + strings.Repeat("a", ar8Largo-len(pre)-1) + "\n"
	ar6Escribir(t, hook, orig, 0o755)
	fecha := time.Date(2001, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := os.Chtimes(hook, fecha, fecha); err != nil {
		t.Fatal(err)
	}
	fiAntes, err := os.Stat(hook)
	if err != nil || fiAntes.Size() != ar8Largo || !fiAntes.ModTime().Equal(fecha) {
		t.Fatalf("%s: fixture: el hook grande de 2 MiB con su fecha fija: %v %v", ca, fiAntes, err)
	}

	antes := aeaVeredictos(root, wt)
	script := ar1Codigo +
		"sleep 0.2\n" +
		"h=$(git rev-parse --git-path hooks/pre-push)\n" +
		"printf 'b' | dd of=\"$h\" bs=1 seek=100 count=1 conv=notrunc 2>/dev/null\n" +
		"touch -m -d '@" + strconv.FormatInt(fecha.Unix(), 10) + "' \"$h\"\n" +
		"exit 0\n"
	res, out := aeaCorrer(t, root, "writer", script, Options{})

	fi, err := os.Stat(hook)
	if err != nil || fi.Size() != fiAntes.Size() || fi.Mode() != fiAntes.Mode() || !fi.ModTime().Equal(fecha) {
		t.Fatalf("%s: fixture: el hook queda con su largo, su modo y su fecha de antes: %v %v (antes %v)", ca, fi, err, fiAntes)
	}
	raw, err := os.ReadFile(hook)
	if err != nil || len(raw) != ar8Largo || raw[100] != 'b' || bytes.Equal(raw, []byte(orig)) {
		t.Fatalf("%s: fixture: el writer sobrescribio en sitio un byte del hook con otro: %v", ca, err)
	}
	ar6Cortado(t, ca, "sobrescribe en sitio y restaura la fecha", root, wt, res, out, antes, "hooks/pre-push")
}
