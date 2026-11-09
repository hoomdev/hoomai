//go:build !windows

// Regresion de la ronda 1 sobre el editor real con goos "windows" (CA-522,
// CA-524, CA-527; hallazgo f28d31 de la review 20261108T231004_195bc7): cmd
// no puede citar una comilla doble, un %, un ! ni un control, y EditorDe se
// niega antes de correr nada. En Unix no hay cmd: el test pone un cmd (y un
// cmd.exe, y ComSpec) falso adelante en el PATH que anota si alguien lo
// corrio, asi "no corre nada" se puede mirar.
package tablero_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/tablero"
)

// CA-524, CA-522 (hallazgo f28d31): EditorDe(comando, "windows").Abrir con un
// argumento que lleva una comilla doble, un %, un !, un tab, un salto o otro
// control es un error que dice "cmd no puede citar", y no corre nada.
// CA-527: Editar lo mismo cuando su archivo temporal cae en un directorio con
// un % (el temporal del sistema, TMPDIR).
func TestCA524_EditorDeEnWindowsNoCitaLoQueCmdNoPuede(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marca := filepath.Join(base, "cmd-corrio")
	bin := filepath.Join(base, "bin")
	for _, n := range []string{"cmd", "cmd.exe"} {
		ttScript(t, filepath.Join(bin, n), "printf '%s\\n' \"$*\" >> '"+marca+"'\n")
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ComSpec", filepath.Join(bin, "cmd.exe"))
	corrio := func() bool {
		_, err := os.Stat(marca)
		return err == nil
	}
	e := tablero.EditorDe("vi", "windows")
	if e == nil {
		t.Fatalf("CA-524: EditorDe da un Editor")
	}
	for nombre, archivo := range map[string]string{
		"una comilla doble": `C:\proyecto\a"b.go`,
		"un %":              `C:\proyecto\100%\a.go`,
		"un !":              `C:\proyecto\hola!.go`,
		"un control":        "C:\\proyecto\\a\x01b.go",
		"un ESC":            "C:\\proyecto\\a\x1b[2Jb.go",
		"un salto":          "C:\\proyecto\\a\nb.go",
		"un tab":            "C:\\proyecto\\a\tb.go",
	} {
		err := e.Abrir(archivo, 3)
		if err == nil || !strings.Contains(err.Error(), "cmd no puede citar") {
			t.Fatalf("CA-524: Abrir con %s en windows es un error 'cmd no puede citar ...': %v", nombre, err)
		}
		if corrio() {
			t.Fatalf("CA-524: con %s no corre nada (corrio cmd con %q)", nombre, ttLeer(t, marca))
		}
	}
	tmp := filepath.Join(base, "temporal 100%")
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	got, err := e.Editar("pedido original")
	if err == nil || !strings.Contains(err.Error(), "cmd no puede citar") {
		t.Fatalf("CA-527: Editar en windows con el temporal en %q es un error 'cmd no puede citar ...': %q, %v", tmp, got, err)
	}
	if corrio() {
		t.Fatalf("CA-527: Editar que cmd no puede citar no corre nada (corrio cmd con %q)", ttLeer(t, marca))
	}
}
