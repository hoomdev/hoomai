package tablero

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

const wrYAML = "schema: hoom/v1\nproject: demo\nprofile: \"\"\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

type wrTerm struct {
	mu     sync.Mutex
	out    bytes.Buffer
	teclas chan []byte
	pend   []byte
	crudo  int
}

func (t *wrTerm) Read(p []byte) (int, error) {
	if len(t.pend) == 0 {
		b, ok := <-t.teclas
		if !ok {
			return 0, io.EOF
		}
		t.pend = b
	}
	n := copy(p, t.pend)
	t.pend = t.pend[n:]
	return n, nil
}

func (t *wrTerm) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.out.Write(p)
}

func (t *wrTerm) Crudo() (func() error, error) {
	t.mu.Lock()
	t.crudo++
	t.mu.Unlock()
	return func() error { return nil }, nil
}

func (t *wrTerm) Tamano() (int, int, error) { return 100, 25, nil }

func (t *wrTerm) texto() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.out.String()
}

type wrDatos struct {
	mu        sync.Mutex
	proyectos int
}

func (d *wrDatos) Tablero() (boardcmd.Board, error) { return wrTableroPrincipal(), nil }
func (d *wrDatos) Fijada() string                   { return "" }
func (d *wrDatos) Detalle(slug string) Detalle      { return Detalle{Slug: slug} }
func (d *wrDatos) Proyecto() Proyecto {
	d.mu.Lock()
	d.proyectos++
	d.mu.Unlock()
	return wrProyecto()
}
func (d *wrDatos) Hallazgos() Hallazgos               { return wrHallazgos() }
func (d *wrDatos) Archivo(ruta string, n int) Archivo { return Archivo{Ruta: ruta, Linea: n} }
func (d *wrDatos) cuantos() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.proyectos
}

type wrLanz struct{}

func (wrLanz) Lanzar(o Lanzar, fin func(FinProceso)) error {
	go fin(FinProceso{ID: o.ID})
	return nil
}

func wrRun(t *testing.T, yaml string, env map[string]string, teclas ...string) (int, *wrTerm, *wrDatos, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "hoom.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	term := &wrTerm{teclas: make(chan []byte, 16)}
	datos := &wrDatos{}
	var errb bytes.Buffer
	e := Entorno{Dir: dir, Out: io.Discard, Err: &errb, Getenv: func(k string) string { return env[k] }, GOOS: "linux",
		Terminal: term, Datos: datos, Lanzador: wrLanz{}, Editor: EditorDe("true", "linux"),
		Ahora: func() time.Time { return wrAhora }, Intervalo: 5 * time.Millisecond, IntervaloProyecto: 20 * time.Millisecond}
	go func() {
		for _, k := range teclas {
			time.Sleep(60 * time.Millisecond)
			term.teclas <- []byte(k)
		}
		time.Sleep(60 * time.Millisecond)
		term.teclas <- []byte("q")
	}()
	code := Run(nil, e)
	return code, term, datos, errb.String()
}

func TestWRRunMouse(t *testing.T) {
	code, term, datos, _ := wrRun(t, wrYAML, map[string]string{"NO_COLOR": "1"}, "u")
	out := term.texto()
	if code != 0 || !strings.HasPrefix(out, SecEntrar+SecMouseSi) || !strings.HasSuffix(out, SecMouseNo+SecSalir) {
		t.Fatalf("con mouse: %d %q ... %q", code, out[:min(40, len(out))], out[max(0, len(out)-40):])
	}
	// at the start, every 20 ms and after u's end
	if n := datos.cuantos(); n < 3 {
		t.Fatalf("Proyecto se lee al abrir, cada IntervaloProyecto y al terminar un verbo: %d", n)
	}
	_, term, _, _ = wrRun(t, wrYAML, map[string]string{"HOOM_MOUSE": "0"})
	if out := term.texto(); strings.Contains(out, SecMouseSi) || !strings.HasPrefix(out, SecEntrar) {
		t.Fatalf("HOOM_MOUSE=0 no pide el mouse: %q", out[:min(40, len(out))])
	}
}

func TestWRRunModelosInvalidos(t *testing.T) {
	for yaml, msg := range map[string]string{
		wrYAML + "models:\n  gemini:\n    - name: x\n":              "models.gemini: hoom no le pasa el modelo a gemini",
		wrYAML + "models:\n  nada:\n    - name: x\n":                `models: provider desconocido "nada" (validos: claude, codex)`,
		wrYAML + "models:\n  codex:\n    - description: x\n":        "models.codex[0]: falta name",
		wrYAML + "models:\n  codex:\n    - name: -x\n":              "models.codex[0]: el nombre no puede empezar con '-'",
		wrYAML + "models:\n  codex:\n    - name: a\n    - nom: b\n": `models.codex[1]: clave desconocida "nom" (validas: name, description)`,
	} {
		code, term, _, errs := wrRun(t, yaml, nil)
		if code != 1 || !strings.Contains(errs, "hoom: hoom.yaml invalido: "+msg) || term.crudo != 0 {
			t.Fatalf("%q: sale con 1 antes de tomar la terminal: %d %q %d", msg, code, errs, term.crudo)
		}
	}
}
