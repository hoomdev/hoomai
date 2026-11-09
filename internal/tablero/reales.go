package tablero

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/costcmd"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/statuscmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

// ---------------------------------------------------------------- the data

// datosReales reads the project at root as hoom board --json does.
type datosReales struct {
	root, base, blockOn string
}

func (d datosReales) Tablero() (boardcmd.Board, error) {
	return boardcmd.Build(d.root, d.base, d.blockOn, time.Now().UTC())
}

func (d datosReales) Fijada() string {
	raw, err := os.ReadFile(filepath.Join(d.root, filepath.FromSlash(tarjetacmd.ArchivoFijada)))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

func (d datosReales) Detalle(slug string) Detalle {
	return d.detalle(slug, true)
}

func (d datosReales) Proyecto() Proyecto {
	var p Proyecto
	p.Rama, p.Git = gitx.Rama(d.root)
	check, last, err := statuscmd.CheckYVeredicto(d.root, d.base)
	if err != nil {
		p.Err = err
		return p
	}
	p.Check, p.Veredicto = check, last
	if p.Hoy, err = costcmd.HoyEn(d.root, time.Now()); err != nil {
		p.Err = err
		return p
	}
	doc, err := boardcmd.Doctor(d.root, d.base, d.blockOn, time.Now().UTC())
	if err != nil {
		p.Err = err
		return p
	}
	p.Problemas = doc.Problems
	return p
}

func (d datosReales) Hallazgos() Hallazgos {
	items, _, err := finding.List(d.root, d.base, true)
	return Hallazgos{Items: items, Err: err}
}

// maxArchivo is how much of a finding's file Archivo reads.
const maxArchivo = 4 << 20

func (d datosReales) Archivo(ruta string, linea int) Archivo {
	a := Archivo{Ruta: ruta, Linea: linea}
	if fueraPorLaForma(ruta) {
		a.Fuera = true
		return a
	}
	rel := filepath.FromSlash(ruta)
	fuera, err := fueraPorLosEnlaces(d.root, rel)
	switch {
	case fuera:
		a.Fuera = true
		return a
	case err != nil:
		a.Err = err
		return a
	}
	r, err := os.OpenRoot(d.root)
	if err != nil {
		a.Err = err
		return a
	}
	defer r.Close()
	f, err := r.Open(rel)
	if err != nil {
		a.Err = err
		return a
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, maxArchivo))
	if err != nil {
		a.Err = err
		return a
	}
	lineas := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if n := len(lineas); n > 0 && lineas[n-1] == "" {
		lineas = lineas[:n-1]
	}
	desde, hasta := linea-ContextoArchivo, linea+ContextoArchivo
	if linea <= 0 {
		desde, hasta = 1, 2*ContextoArchivo+1
	}
	desde = max(1, desde)
	hasta = min(hasta, len(lineas))
	a.Desde = desde
	if desde <= hasta {
		a.Lineas = append([]string(nil), lineas[desde-1:hasta]...)
	}
	return a
}

func (d datosReales) detalle(slug string, historia bool) Detalle {
	now := time.Now().UTC()
	det, err := boardcmd.DetailFor(d.root, d.base, d.blockOn, slug, now, false)
	out := Detalle{Slug: slug, Detalle: det, Err: err}
	if err != nil {
		return out
	}
	if historia {
		tl, err := boardcmd.TimelineFor(d.root, d.base, d.blockOn, slug, now)
		if err != nil {
			out.Err = err
			return out
		}
		out.Historia = tl
	}
	if len(det.Work) > 0 {
		w := det.Work[0]
		run := w.RunID
		if w.Kind == boardcmd.WorkRun {
			run = w.ID
		} else {
			out.Sobre.EnvelopeID = w.ID
		}
		out.Sobre.RunID = run
		if run != "" {
			out.Sobre.Eventos, out.Sobre.Err = d.narracion(det.Card.Evidence.Dir, run)
		}
	}
	return out
}

// narracion is the last MaxSobre bytes of the run's narration: in the
// project's .hoom/runs, or else in the card's evidence tree. A narration
// that is in neither (a run of another machine) is nothing to show, not an
// error; any other failure is, the evidence tree's first.
func (d datosReales) narracion(dir, run string) ([]runcmd.Event, error) {
	ev, err := runcmd.UltimosEventos(d.root, run, MaxSobre)
	if err == nil {
		return ev, nil
	}
	if dir != "" && dir != "." {
		ev2, err2 := runcmd.UltimosEventos(filepath.Join(d.root, filepath.FromSlash(dir)), run, MaxSobre)
		if err2 == nil {
			return ev2, nil
		}
		if !errors.Is(err2, fs.ErrNotExist) {
			return nil, err2
		}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return nil, err
}

// ---------------------------------------------------------------- the editor

// editorReal runs the person's editor through the shell, like the capa-1
// verbs.
type editorReal struct {
	comando, goos string
}

func (e editorReal) Abrir(archivo string, linea int) error {
	return e.correr(ArgumentosEditor(e.comando, archivo, linea))
}

func (e editorReal) Editar(texto string) (string, error) {
	f, err := os.CreateTemp("", "hoom-pedido-*.txt")
	if err != nil {
		return "", err
	}
	path := f.Name()
	defer os.Remove(path)
	if _, err := f.WriteString(texto); err != nil {
		f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := e.correr([]string{path}); err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	return string(raw), err
}

func (e editorReal) correr(args []string) error {
	var cmd *exec.Cmd
	if e.goos == "windows" {
		// each argument between quotes, and none that cmd reads inside
		// them: a quote ends the quoting, % and ! expand variables. The
		// arguments come from the evidence (a finding's file).
		linea := e.comando
		for _, a := range args {
			if strings.ContainsAny(a, `"%!`) || strings.IndexFunc(a, unicode.IsControl) >= 0 {
				return fmt.Errorf("cmd no puede citar %q: tiene comillas, porcentaje, ! o un control", a)
			}
			linea += ` "` + a + `"`
		}
		cmd = lineaCmd(linea)
	} else {
		cmd = exec.Command("sh", append([]string{"-c", e.comando + ` "$@"`, e.comando}, args...)...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- el editor es el de la persona ($VISUAL/$EDITOR), como en git y en la capa 1
	}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// ---------------------------------------------------------------- the launches

// lanzadorReal runs hoom itself, detached.
type lanzadorReal struct {
	hoom, dir, logs string
}

func (l *lanzadorReal) Lanzar(o Lanzar, fin func(FinProceso)) error {
	if err := os.MkdirAll(l.logs, 0o755); err != nil {
		return err
	}
	base := filepath.Join(l.logs, fmt.Sprintf("%s-%d-%d", time.Now().UTC().Format("20060102T150405"), os.Getpid(), o.ID))
	salida, err := nuevoLog(base + ".out")
	if err != nil {
		return err
	}
	errores, err := nuevoLog(base + ".err")
	if err != nil {
		salida.Close()
		return err
	}
	cmd := exec.Command(l.hoom, o.Args...) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command -- es el binario de hoom mismo con los argumentos de un verbo de la tarjeta
	cmd.Dir = l.dir
	cmd.Stdin = nil // the null device: nothing to ask the person
	cmd.Stdout, cmd.Stderr = salida, errores
	desprender(cmd)
	if err := cmd.Start(); err != nil {
		salida.Close()
		errores.Close()
		return err
	}
	go func() {
		err := cmd.Wait()
		salida.Close()
		errores.Close()
		code := 0
		if err != nil {
			code = -1
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				code = ee.ExitCode()
			}
		}
		ultima := ultimaLinea(base + ".err")
		if ultima == "" {
			ultima = ultimaLinea(base + ".out")
		}
		if ultima == "" && code == -1 && err != nil {
			ultima = err.Error()
		}
		fin(FinProceso{ID: o.ID, Code: code, Ultima: strings.TrimPrefix(ultima, "hoom: ")})
	}()
	return nil
}

// nuevoLog creates a log file that did not exist.
func nuevoLog(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
}

// ultimaLinea is the last line of the file that is not empty, from its
// last 8 KB.
func ultimaLinea(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	const cola = 8 << 10
	fi, err := f.Stat()
	if err != nil {
		return ""
	}
	desde := fi.Size() - cola
	if desde < 0 {
		desde = 0
	}
	raw, err := io.ReadAll(io.NewSectionReader(f, desde, fi.Size()-desde))
	if err != nil {
		return ""
	}
	lineas := bytes.Split(raw, []byte("\n"))
	for i := len(lineas) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(string(lineas[i])); l != "" {
			return l
		}
	}
	return ""
}
