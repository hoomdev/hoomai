// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-399, hallazgo f23bb2 de resilience): el prompt grande viaja por el
// stdin del proceso, y un stdin que nadie lee no puede colgar el run. El CLI
// falso deja un hijo huerfano que HEREDA su stdin y nunca lo lee (el caso de
// un CLI que lanza un helper o un servidor MCP y termina), y el prompt pasa
// de largo el buffer de un pipe: sin nadie leyendo, escribirlo se bloquea
// para siempre. Con un timeout corto, el run tiene que asentarse (dejar de
// estar en curso) dentro del timeout mas unos segundos.
//
// Hermetico: el huerfano es un `sleep` del sistema con la salida a
// /dev/null; su pid queda fuera de cualquier arbol y el test lo mata al
// terminar, pase lo que pase.
package runcmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/providers"
)

// shHuerfano deja en segundo plano un `sleep` que hereda el stdin del CLI (se
// guarda en el fd 3 ANTES de lanzarlo: un & sin control de trabajos le
// pondria /dev/null) y nunca lo lee. Su pid queda en <d>/huerfano.pid, y el
// del CLI mismo en <d>/cli.pid (con exec, el del sleep en que se convierte).
const shHuerfano = "echo $$ > \"$d/cli.pid\"\n" +
	"exec 3<&0\n" +
	"sleep 300 <&3 >/dev/null 2>&1 3<&- &\n" +
	"echo $! > \"$d/huerfano.pid\"\n" +
	"exec 3<&-\n"

// sfMatarHuerfano mata al huerfano que dejo el CLI falso y al CLI mismo, si
// llegaron a nacer y siguen vivos: el test no deja procesos sueltos.
func sfMatarHuerfano(dir string) {
	for _, f := range []string{"huerfano.pid", "cli.pid"} {
		raw, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			continue
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(string(raw))); err == nil && pid > 1 {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

// CA-399 (f23bb2): con un prompt de 1 MiB que va por stdin, un CLI que deja
// un huerfano con su stdin sin leer y un timeout de 2 s, el run se asienta
// en menos de 2 s + 8 s: saliendo el CLI enseguida o quedandose colgado sin
// leer. En claude y en codex. El test tiene su propio reloj de 15 s: un run
// colgado es un fallo, no un test que no termina.
func TestCA399_StdinSinLeerConHuerfanoNoCuelgaElRun(t *testing.T) {
	const timeout = 2 * time.Second
	const margen = 8 * time.Second
	const reloj = 15 * time.Second
	grande := raGrande(1<<20 + 1)
	if len(grande) <= providers.StdinPromptBytes {
		t.Fatalf("CA-399: fixture: el prompt (%d bytes) va por stdin: pasa %d", len(grande), providers.StdinPromptBytes)
	}
	casos := []struct {
		nombre string
		cuerpo string // despues de dejar al huerfano; nunca lee su stdin
	}{
		{"el-cli-sale-enseguida", "exit 0\n"},
		// exec: el CLI mismo pasa a ser el sleep, asi el timeout lo mata a
		// el y no deja otro proceso suelto
		{"el-cli-se-cuelga-sin-leer", "exec sleep 300 >/dev/null 2>&1\n"},
	}
	for _, name := range []string{"claude", "codex"} {
		for _, c := range casos {
			t.Run(name+"/"+c.nombre, func(t *testing.T) {
				dir := t.TempDir()
				installFakeNamed(t, name, "d='"+dir+"'\n"+shHuerfano+c.cuerpo)
				t.Cleanup(func() { sfMatarHuerfano(dir) })

				m := NewManager(t.TempDir())
				m.Timeout = timeout
				inicio := time.Now()
				run, err := m.Start(StartOptions{Provider: name, Prompt: grande, Dir: t.TempDir()})
				if err != nil {
					t.Fatalf("CA-399: %s: Start con prompt grande: %v", name, err)
				}
				for {
					r, _, err := m.Events(run.ID, 0)
					if err != nil {
						t.Fatalf("CA-399: %s: Events: %v", name, err)
					}
					if r.Status != StatusRunning {
						if d := time.Since(inicio); d > timeout+margen {
							t.Fatalf("CA-399: %s: el run se asento en %s, mas que el timeout (%s) + %s", name, d.Round(time.Millisecond), timeout, margen)
						}
						break
					}
					if time.Since(inicio) > reloj {
						t.Fatalf("CA-399: %s: con un huerfano que hereda el stdin sin leerlo, el run sigue en curso a los %s (timeout %s): el prompt por stdin lo cuelga",
							name, reloj, timeout)
					}
					time.Sleep(20 * time.Millisecond)
				}
				if _, err := os.Stat(filepath.Join(dir, "huerfano.pid")); err != nil {
					t.Fatalf("CA-399: fixture: el CLI falso corrio y dejo a su huerfano: %v", err)
				}
			})
		}
	}
}
