// Regresion del hallazgo 20261125T172319_22b671 (medium), por dentro: el
// recorrido de buscarHoomYAML con un lstat inyectado. Desde afuera, un error
// de lstat que no sea ENOENT (EIO, o EACCES si el sistema de archivos se
// degrada entre la carga y CambioDe) no se puede provocar sin root. Aqui
// cada error que no es fs.ErrNotExist da dudoso, y el recorrido no sigue
// subiendo para devolver el hoom.yaml de un padre como hacia manifest.Find.
package gates

import (
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/manifest"
)

// h22Info es un fs.FileInfo con solo su modo.
type h22Info struct{ modo fs.FileMode }

func (i h22Info) Name() string       { return manifest.FileName }
func (i h22Info) Size() int64        { return 0 }
func (i h22Info) Mode() fs.FileMode  { return i.modo }
func (i h22Info) ModTime() time.Time { return time.Time{} }
func (i h22Info) IsDir() bool        { return i.modo.IsDir() }
func (i h22Info) Sys() any           { return nil }

// h22Lstat es un lstat que da, para cada ruta de entradas, su respuesta, y
// ENOENT (como os.Lstat, en un *fs.PathError) para cualquier otra.
func h22Lstat(entradas map[string]error, infos map[string]fs.FileInfo) func(string) (fs.FileInfo, error) {
	return func(p string) (fs.FileInfo, error) {
		if fi, ok := infos[p]; ok {
			return fi, nil
		}
		if err, ok := entradas[p]; ok {
			return nil, &fs.PathError{Op: "lstat", Path: p, Err: err}
		}
		return nil, &fs.PathError{Op: "lstat", Path: p, Err: syscall.ENOENT}
	}
}

func TestHallazgo22b671_CA812_UnErrorQueNoEsNoExisteEsDuda(t *testing.T) {
	const ca = "CA-812"
	dir := filepath.Join(string(filepath.Separator), "repo", "proyecto", "sub")
	yamlDe := func(d string) string { return filepath.Join(d, manifest.FileName) }
	proyecto := filepath.Dir(dir)
	raiz := filepath.Dir(proyecto)
	regular := h22Info{modo: 0o644}

	casos := []struct {
		nombre   string
		errores  map[string]error
		infos    map[string]fs.FileInfo
		proyecto string
		hay      bool
		dudoso   bool
	}{
		{"EIO en dir", map[string]error{yamlDe(dir): syscall.EIO}, map[string]fs.FileInfo{yamlDe(raiz): regular}, "", false, true},
		{"EACCES en el proyecto, con un hoom.yaml en la raiz", map[string]error{yamlDe(proyecto): syscall.EACCES}, map[string]fs.FileInfo{yamlDe(raiz): regular}, "", false, true},
		{"ENOTDIR en dir", map[string]error{yamlDe(dir): syscall.ENOTDIR}, nil, "", false, true},
		{"ELOOP en dir", map[string]error{yamlDe(dir): syscall.ELOOP}, map[string]fs.FileInfo{yamlDe(raiz): regular}, "", false, true},
		{"un enlace en el proyecto", nil, map[string]fs.FileInfo{yamlDe(proyecto): h22Info{modo: fs.ModeSymlink | 0o777}}, proyecto, true, true},
		// los controles
		{"el hoom.yaml del proyecto", nil, map[string]fs.FileInfo{yamlDe(proyecto): regular, yamlDe(raiz): regular}, proyecto, true, false},
		{"ninguno hasta la raiz", nil, nil, "", false, false},
	}
	for _, c := range casos {
		p, hay, dudoso := buscarHoomYAML(dir, h22Lstat(c.errores, c.infos))
		if p != c.proyecto || hay != c.hay || dudoso != c.dudoso {
			t.Fatalf("%s (%s): buscarHoomYAML = (%q, hay %v, dudoso %v), quiero (%q, hay %v, dudoso %v)", ca, c.nombre, p, hay, dudoso, c.proyecto, c.hay, c.dudoso)
		}
	}
}

// Con el lstat de verdad: un hoom.yaml real en el directorio del proyecto, y
// uno en el padre cuando el proyecto no tiene.
func TestHallazgo22b671_CA811_ConElLstatDeVerdad(t *testing.T) {
	const ca = "CA-811"
	raiz, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(raiz, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(raiz, manifest.FileName), []byte("schema: hoom/v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, hay, dudoso := buscarHoomYAML(sub, os.Lstat); p != raiz || !hay || dudoso {
		t.Fatalf("%s: desde sub, el hoom.yaml de la raiz: (%q, %v, %v)", ca, p, hay, dudoso)
	}
	if err := os.WriteFile(filepath.Join(sub, manifest.FileName), []byte("schema: hoom/v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p, hay, dudoso := buscarHoomYAML(sub, os.Lstat); p != sub || !hay || dudoso {
		t.Fatalf("%s: desde sub, el hoom.yaml de sub: (%q, %v, %v)", ca, p, hay, dudoso)
	}
}
