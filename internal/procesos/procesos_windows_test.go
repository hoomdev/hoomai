//go:build windows

// Test de regresion del hallazgo 20261015T163243_d24626 (high) de la review
// de las correcciones de C3 (.hoom/specs/acciones-desde-la-tarjeta.md), para
// Windows. Escrito desde el contrato (go doc de Adoptar, MatarGrupo y Soltar
// con GOOS=windows), sin leer la implementacion: cancelar un run en Windows
// tiene que terminar el arbol de procesos entero, no solo el del provider.
// Adoptar mete al proceso recien lanzado en un Job Object y lo que lance de
// ahi en mas queda adentro; MatarGrupo lo mata entero.
//
// Corre solo con un runner de Windows. El hijo y el nieto son este mismo
// binario de tests corriendo TestAyudanteProcesosWindows.
package procesos

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

// peRol dice al binario de tests vuelto a ejecutar que papel hace.
const peRol = "HOOM_TEST_PROCESOS_ROL"

// TestAyudanteProcesosWindows no es un test: es el hijo (espera que el padre
// lo adopte, lanza al nieto y dice "nieto <pid>") o el nieto (duerme) de
// TestHallazgo_d24626_MatarGrupoMataAlNieto.
func TestAyudanteProcesosWindows(t *testing.T) {
	switch os.Getenv(peRol) {
	case "hijo":
		bufio.NewReader(os.Stdin).ReadString('\n') // el padre ya lo adopto
		nieto := exec.Command(os.Args[0], "-test.run=^TestAyudanteProcesosWindows$", "-test.count=1")
		nieto.Env = append(os.Environ(), peRol+"=nieto")
		if err := nieto.Start(); err != nil {
			fmt.Printf("error %v\n", err)
			return
		}
		fmt.Printf("nieto %d\n", nieto.Process.Pid)
		time.Sleep(5 * time.Minute)
	case "nieto":
		time.Sleep(5 * time.Minute)
	default:
		t.Skip("solo corre como proceso ayudante de TestHallazgo_d24626_MatarGrupoMataAlNieto")
	}
}

// peTermina espera hasta plazo a que el proceso pid termine. En Windows un
// handle con permiso de esperar sirve para cualquier proceso, no solo para
// los hijos.
func peTermina(pid int, plazo time.Duration) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return true // ya no existe
	}
	fin := make(chan struct{})
	go func() {
		p.Wait()
		close(fin)
	}()
	select {
	case <-fin:
		return true
	case <-time.After(plazo):
		return false
	}
}

// Hallazgo 20261015T163243_d24626 (high): un proceso adoptado (Adoptar,
// apenas lanzado) que despues lanza un nieto; MatarGrupo sobre el hijo
// termina a los dos, y Soltar lo cierra despues.
func TestHallazgo_d24626_MatarGrupoMataAlNieto(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestAyudanteProcesosWindows$", "-test.count=1")
	cmd.Env = append(os.Environ(), peRol+"=hijo")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	Adoptar(cmd)
	fin := make(chan struct{})
	lineas := make(chan string, 4)
	go func() {
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			select {
			case lineas <- s.Text():
			default:
			}
		}
		io.Copy(io.Discard, stdout)
		cmd.Wait()
		close(fin)
	}()
	nietoPID := 0
	t.Cleanup(func() {
		cmd.Process.Kill()
		if nietoPID > 0 {
			if p, err := os.FindProcess(nietoPID); err == nil {
				p.Kill()
			}
		}
		select {
		case <-fin:
		case <-time.After(20 * time.Second):
		}
	})

	if _, err := io.WriteString(stdin, "sigue\n"); err != nil {
		t.Fatal(err)
	}
	select {
	case l := <-lineas:
		n, err := strconv.Atoi(strings.TrimPrefix(l, "nieto "))
		if !strings.HasPrefix(l, "nieto ") || err != nil || n <= 0 {
			t.Fatalf("hallazgo d24626: fixture: el hijo lanza al nieto y dice su PID: %q", l)
		}
		nietoPID = n
	case <-fin:
		t.Fatal("hallazgo d24626: fixture: el hijo termino sin lanzar al nieto")
	case <-time.After(30 * time.Second):
		t.Fatal("hallazgo d24626: fixture: el hijo no lanzo al nieto en 30 s")
	}
	if peTermina(nietoPID, 500*time.Millisecond) {
		t.Fatalf("hallazgo d24626: fixture: el nieto %d esta vivo antes de matar el grupo", nietoPID)
	}

	if err := MatarGrupo(cmd.Process.Pid); err != nil {
		t.Fatalf("hallazgo d24626: MatarGrupo(%d): %v", cmd.Process.Pid, err)
	}
	select {
	case <-fin:
	case <-time.After(20 * time.Second):
		t.Fatal("hallazgo d24626: MatarGrupo termina al hijo")
	}
	if !peTermina(nietoPID, 20*time.Second) {
		t.Fatalf("hallazgo d24626: MatarGrupo termina el arbol entero: el nieto %d, que lanzo el hijo despues de Adoptar, sigue vivo", nietoPID)
	}
	Soltar(cmd.Process.Pid)
}
