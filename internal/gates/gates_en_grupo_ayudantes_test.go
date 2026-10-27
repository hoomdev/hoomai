//go:build !windows

// Ayudantes de los tests de gates-en-grupo que corren un hoom en un
// subproceso: el ciclo de vida del proceso (arranque en un grupo propio, log,
// espera, limpieza), las senales que se le mandan, y el Test que hace de hoom
// (TestAyudanteHoomConUnGate). Antes cada ronda copiaba ese ciclo (hallazgo
// 20261027T172229_c7a3ce); aca queda uno solo, parametrizado por el Test que
// corre el subproceso (selector), su entorno y la etiqueta de sus mensajes.
package gates

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/procesos"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Variables que lee TestAyudanteHoomConUnGate.
const (
	// gegEnvAyudanteDir es la carpeta del proyecto del hoom ayudante: ahi
	// corre su gate y ahi deja resultado.json.
	gegEnvAyudanteDir = "HOOM_TEST_AYUDANTE_DIR"
	// gegEnvAyudanteGate es el comando del gate test.
	gegEnvAyudanteGate = "HOOM_TEST_AYUDANTE_GATE"
	// gegEnvAyudanteCola es la cola de salida (Options.TailSize); 4096 si
	// no esta.
	gegEnvAyudanteCola = "HOOM_TEST_AYUDANTE_COLA"
	// gegEnvAyudanteProvider, si esta, es el comando de un grupo que el
	// ayudante arranca con procesos.Arrancar (sin gracia, como el provider
	// de un run) antes de correr el gate.
	gegEnvAyudanteProvider = "HOOM_TEST_AYUDANTE_PROVIDER"
)

// TestAyudanteHoomConUnGate no es un test: es el hoom de los tests que
// necesitan uno en un subproceso. Si le piden un provider, lo arranca con
// procesos.Arrancar; despues corre el gate test con plazo de 5 minutos y, si
// Run vuelve, deja los resultados en <carpeta>/resultado.json.
func TestAyudanteHoomConUnGate(t *testing.T) {
	d, gate := os.Getenv(gegEnvAyudanteDir), os.Getenv(gegEnvAyudanteGate)
	if d == "" || gate == "" {
		t.Skip("solo corre como el hoom ayudante de los tests de gates-en-grupo")
	}
	cola := 4096
	if v := os.Getenv(gegEnvAyudanteCola); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			t.Fatal(err)
		}
		cola = n
	}
	if p := os.Getenv(gegEnvAyudanteProvider); p != "" {
		provider := exec.CommandContext(context.Background(), "sh", "-c", p)
		if err := procesos.Arrancar(provider); err != nil {
			t.Fatal(err)
		}
		go func() {
			provider.Wait()
			procesos.Liberar(provider.Process.Pid)
		}()
	}
	req := true
	m := &manifest.Manifest{Dir: d, Gates: map[string]manifest.Gate{"test": {Cmd: &gate, Required: &req}}}
	res := Run(m, gitx.Info{}, Options{Timeout: 5 * time.Minute, TailSize: cola})
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(d, "resultado.json.tmp")
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, filepath.Join(d, "resultado.json")); err != nil {
		t.Fatal(err)
	}
}

// gegAyudante describe un hoom ayudante: el Test de este binario que corre
// (selector), lo que se suma a su entorno y la etiqueta de sus mensajes.
type gegAyudante struct {
	selector string
	entorno  []string
	etiqueta string
}

// gegConUnGate es el TestAyudanteHoomConUnGate de la carpeta d con el gate
// cmd; entorno suma variables (TMPDIR, la cola, el provider).
func gegConUnGate(d, cmd, etiqueta string, entorno ...string) gegAyudante {
	return gegAyudante{
		selector: "TestAyudanteHoomConUnGate",
		entorno:  append([]string{gegEnvAyudanteDir + "=" + d, gegEnvAyudanteGate + "=" + cmd}, entorno...),
		etiqueta: etiqueta,
	}
}

// gegHoom es un hoom ayudante en marcha.
type gegHoom struct {
	cmd      *exec.Cmd
	log      string
	fin      chan struct{}
	etiqueta string
}

// gegArrancarHoom arranca el hoom ayudante a en un grupo propio, como una
// terminal: la senal "terminal" que le llega a su grupo no le llega al test.
// Al final del test lo mata con su grupo si sigue vivo, y falla si su log
// tiene una carrera de datos (que no se veria en la salida del test).
func gegArrancarHoom(t *testing.T, a gegAyudante) *gegHoom {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+a.selector+"$", "-test.count=1")
	// el ultimo valor de una variable repetida es el que vale
	cmd.Env = append(os.Environ(), a.entorno...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	log := filepath.Join(t.TempDir(), "hoom.log")
	salida, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	defer salida.Close()
	cmd.Stdout, cmd.Stderr = salida, salida
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	h := &gegHoom{cmd: cmd, log: log, fin: make(chan struct{}), etiqueta: a.etiqueta}
	go func() { cmd.Wait(); close(h.fin) }()
	t.Cleanup(func() {
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		<-h.fin
		if raw, _ := os.ReadFile(log); strings.Contains(string(raw), "DATA RACE") {
			t.Errorf("%s: el hoom ayudante tuvo una carrera de datos:\n%s", h.etiqueta, raw)
		}
	})
	return h
}

// senal le manda sig a hoom, solo a su PID o (terminal) a su grupo entero.
func (h *gegHoom) senal(t *testing.T, sig syscall.Signal, terminal bool) {
	t.Helper()
	destino := h.cmd.Process.Pid
	if terminal {
		destino = -destino
	}
	if err := syscall.Kill(destino, sig); err != nil && !(err == syscall.ESRCH && h.termino(0)) {
		t.Fatal(err)
	}
}

// termino dice si hoom termino antes de que pase plazo.
func (h *gegHoom) termino(plazo time.Duration) bool {
	select {
	case <-h.fin:
		return true
	case <-time.After(plazo):
		return false
	}
}

// porLaSenal dice si hoom (ya terminado) termino como terminaria siempre con
// sig: por la senal, o con 128+sig como un shell.
func (h *gegHoom) porLaSenal(sig syscall.Signal) bool {
	ws, _ := h.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return (ws.Signaled() && ws.Signal() == sig) || (ws.Exited() && ws.ExitStatus() == 128+int(sig))
}

// salida es el log del hoom ayudante, para los mensajes.
func (h *gegHoom) salida() string {
	raw, _ := os.ReadFile(h.log)
	return string(raw)
}

// resultado lee el resultado.json que dejo el hoom ayudante de la carpeta d
// (ya terminado) y devuelve su gate test.
func (h *gegHoom) resultado(t *testing.T, d string) verdict.GateResult {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(d, "resultado.json"))
	if err != nil {
		t.Fatalf("fixture: el hoom ayudante no dejo sus resultados: %v\n%s", err, h.salida())
	}
	var res []verdict.GateResult
	if err := json.Unmarshal(raw, &res); err != nil {
		t.Fatalf("fixture: resultados del hoom ayudante: %v\n%s", err, raw)
	}
	for _, r := range res {
		if r.Name == "test" {
			return r
		}
	}
	t.Fatalf("fixture: el hoom ayudante no devolvio el gate test: %s", raw)
	return verdict.GateResult{}
}

// memoriaPico es la memoria residente maxima del hoom ayudante (ya
// terminado), en bytes.
func (h *gegHoom) memoriaPico() int64 {
	ru, ok := h.cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	if runtime.GOOS == "darwin" {
		return int64(ru.Maxrss) // bytes en macOS
	}
	return int64(ru.Maxrss) * 1024 // KiB en Linux
}

// gegSenalesAHoom son las senales con las que se termina un hoom: el Ctrl-C
// (al PID de hoom o a su grupo, como la terminal) y SIGTERM.
var gegSenalesAHoom = []struct {
	nombre   string
	senal    syscall.Signal
	terminal bool
}{
	{"SIGINT", syscall.SIGINT, false},
	{"SIGINT terminal", syscall.SIGINT, true},
	{"SIGTERM", syscall.SIGTERM, false},
	{"SIGTERM terminal", syscall.SIGTERM, true},
}

// gegLanzarConPlazo es gegLanzar con un plazo propio y una cola de 16 KiB.
func gegLanzarConPlazo(t *testing.T, gates map[string]string, plazo time.Duration) *gegCorrida {
	t.Helper()
	req := true
	m := &manifest.Manifest{Dir: t.TempDir(), Gates: map[string]manifest.Gate{}}
	for nombre, cmd := range gates {
		m.Gates[nombre] = manifest.Gate{Cmd: strPtr(cmd), Required: &req}
	}
	c := &gegCorrida{inicio: time.Now(), fin: make(chan struct{})}
	go func() {
		c.res = Run(m, gitx.Info{}, Options{Timeout: plazo, TailSize: 16384})
		c.dur = time.Since(c.inicio)
		close(c.fin)
	}()
	return c
}

// gegScript escribe un script de sh en d/nombre y devuelve su ruta.
func gegScript(t *testing.T, d, nombre, cuerpo string) string {
	t.Helper()
	ruta := filepath.Join(d, nombre)
	if err := os.WriteFile(ruta, []byte(cuerpo), 0o644); err != nil {
		t.Fatal(err)
	}
	return ruta
}

// gegArchivos lista lo que hay en dir (recursivo) y cuales de esos archivos
// contienen marca.
func gegArchivos(t *testing.T, dir, marca string) (todos, conMarca []string) {
	t.Helper()
	err := filepath.WalkDir(dir, func(ruta string, e fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ruta == dir {
			return nil
		}
		todos = append(todos, ruta)
		if e.Type().IsRegular() {
			if raw, err := os.ReadFile(ruta); err == nil && strings.Contains(string(raw), marca) {
				conMarca = append(conMarca, ruta)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fixture: no se pudo recorrer %s: %v", dir, err)
	}
	return todos, conMarca
}
