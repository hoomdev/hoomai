package itemcmd

import (
	"strings"
	"testing"
	"time"
)

// Hallazgo 20261112T195211_d49489: de la salida de un git del guardado que
// escribe mas de 64 KiB y despues falla, gitTope conserva el final, donde
// esta el diagnostico, y no solo el principio.
func TestWriterD49489GitTopeConservaElFinalDeLaSalida(t *testing.T) {
	const diagnostico = "el diagnostico del final"
	alias := "alias.ruido=!f() { i=0; while [ $i -lt 3000 ]; do " +
		"echo 'ruido ruido ruido ruido ruido ruido ruido'; i=$((i+1)); done; " +
		"echo '" + diagnostico + "' >&2; exit 1; }; f"
	out, err := gitTope(t.TempDir(), time.Minute, nil, nil, "-c", alias, "ruido")
	if err == nil {
		t.Fatalf("d49489: el git que sale con 1 es un error: %q", out)
	}
	if !strings.HasSuffix(out, diagnostico) {
		n := len(out)
		if n > 200 {
			out = "..." + out[n-200:]
		}
		t.Fatalf("d49489: la salida conserva el diagnostico del final (%d bytes): %q", n, out)
	}
	if len(out) > 64<<10 {
		t.Fatalf("d49489: la salida sigue acotada a 64 KiB: %d bytes", len(out))
	}
	if !strings.Contains(out, "ruido") {
		t.Fatalf("d49489: lo anterior al diagnostico tambien queda, hasta el limite: %q", out)
	}
}

// salidaGit sin limite guarda todo (el listado de lo que hay que guardar), y
// con limite guarda los ultimos bytes en orden.
func TestWriterSalidaGit(t *testing.T) {
	todo := &salidaGit{}
	cola := &salidaGit{limite: 10}
	for i := 0; i < 100; i++ {
		s := []byte(strings.Repeat(string(rune('a'+i%26)), 3))
		todo.Write(s)
		cola.Write(s)
	}
	if got := len(todo.bytes()); got != 300 {
		t.Fatalf("sin limite guarda todo: %d bytes", got)
	}
	want := string(todo.bytes()[290:])
	if got := string(cola.bytes()); got != want {
		t.Fatalf("con limite 10 guarda los ultimos 10 bytes: %q, quiero %q", got, want)
	}
}
