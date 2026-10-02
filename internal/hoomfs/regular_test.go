// Tests de contrato de hoomfs.AbrirRegular para el hallazgo 98faf2 de la
// segunda ronda de review de .hoom/specs/evidencia-en-disco.md (CA-442: un
// symlink o un FIFO "nunca se sigue ni se lee"): abre para leer SOLO un
// archivo regular, decidiendo sobre el descriptor que devuelve; nunca sigue
// un symlink. Lo que no es un archivo regular vuelve como *NoRegular (con el
// Mode de lo que es) o como el error de la apertura; una ruta que no existe,
// como fs.ErrNotExist. Escritos desde el contrato, sin leer la
// implementacion. Los casos con FIFO estan en regular_unix_test.go.
package hoomfs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// arRelojPorApertura es cuanto puede tardar una apertura: mas es que siguio
// algo o se quedo esperando.
const arRelojPorApertura = 5 * time.Second

type arApertura struct {
	f   *os.File
	err error
}

// arAbrir es AbrirRegular con reloj. Si vuelve un archivo, se cierra al
// final del test.
func arAbrir(t *testing.T, caso, path string) (*os.File, error) {
	t.Helper()
	ch := make(chan arApertura, 1)
	go func() {
		f, err := AbrirRegular(path)
		ch <- arApertura{f, err}
	}()
	select {
	case a := <-ch:
		if a.f != nil {
			t.Cleanup(func() { a.f.Close() })
		}
		return a.f, a.err
	case <-time.After(arRelojPorApertura):
		t.Fatalf("CA-442: %s: AbrirRegular(%s) no volvio en %s: siguio o abrio algo que no es un archivo regular", caso, path, arRelojPorApertura)
	}
	return nil, nil
}

// arNoRegular exige que AbrirRegular haya fallado sin devolver archivo; si
// el error es *NoRegular, que nombre la ruta y que su Mode cumpla modo. Lo
// devuelve (nil si el error es el de la apertura).
func arNoRegular(t *testing.T, caso, path string, f *os.File, err error, modo func(fs.FileMode) bool) *NoRegular {
	t.Helper()
	if err == nil || f != nil {
		t.Fatalf("CA-442: %s: AbrirRegular no abre lo que no es un archivo regular: archivo=%v err=%v", caso, f != nil, err)
	}
	var nr *NoRegular
	if !errors.As(err, &nr) {
		return nil
	}
	if nr.Path != path {
		t.Fatalf("CA-442: %s: *NoRegular nombra la ruta %s: %q", caso, path, nr.Path)
	}
	if nr.Mode.IsRegular() || !modo(nr.Mode) {
		t.Fatalf("CA-442: %s: el Mode de *NoRegular dice lo que es la entrada: %s", caso, nr.Mode)
	}
	if !strings.Contains(nr.Error(), "no es un archivo regular") {
		t.Fatalf("CA-442: %s: el error de *NoRegular dice \"no es un archivo regular\": %q", caso, nr.Error())
	}
	return nr
}

// arSymlink crea en p un symlink a destino; sin permiso para crearlo
// (windows sin privilegios) el test se omite.
func arSymlink(t *testing.T, destino, p string) {
	t.Helper()
	if err := os.Symlink(destino, p); err != nil {
		t.Skipf("CA-442: no se pudo crear el symlink %s -> %s: %v", p, destino, err)
	}
}

func arEsSymlink(m fs.FileMode) bool { return m&fs.ModeSymlink != 0 }

// CA-442 (98faf2): un archivo regular se abre y se lee entero (tambien uno
// vacio y uno binario con unicode en el nombre), y el descriptor es de solo
// lectura: escribir por el falla y el archivo no cambia.
func TestCA442_AbrirRegularAbreYLeeUnArchivoRegular(t *testing.T) {
	dir := t.TempDir()
	for nombre, contenido := range map[string][]byte{
		"hallazgo.json":                  []byte("{\"severity\":\"high\"}\n"),
		"vacio.json":                     nil,
		"binario-ñandú.bin":              {0x00, 0x01, 0xff, 0xfe, 0x00, '\n'},
		"grande.json":                    bytes.Repeat([]byte("0123456789abcdef"), 1<<16+3),
		".oculto":                        []byte("x"),
		"con espacios y 'comillas'.json": []byte("{}"),
	} {
		p := filepath.Join(dir, nombre)
		if err := os.WriteFile(p, contenido, 0o644); err != nil {
			t.Fatal(err)
		}
		f, err := arAbrir(t, nombre, p)
		if err != nil || f == nil {
			t.Fatalf("CA-442: %s: un archivo regular se abre: %v", nombre, err)
		}
		fi, err := f.Stat()
		if err != nil || !fi.Mode().IsRegular() {
			t.Fatalf("CA-442: %s: el descriptor es el de un archivo regular: %v %v", nombre, fi, err)
		}
		got, err := io.ReadAll(f)
		if err != nil || !bytes.Equal(got, contenido) {
			t.Fatalf("CA-442: %s: se lee su contenido entero (%d bytes, leyo %d): %v", nombre, len(contenido), len(got), err)
		}
		if n, err := f.Write([]byte("pisado")); err == nil || n != 0 {
			t.Fatalf("CA-442: %s: el descriptor es de solo lectura: escribio %d bytes (%v)", nombre, n, err)
		}
		if raw, err := os.ReadFile(p); err != nil || !bytes.Equal(raw, contenido) {
			t.Fatalf("CA-442: %s: abrirlo no cambia el archivo: %v", nombre, err)
		}
	}
}

// CA-442 (98faf2): un symlink nunca se sigue. A un archivo regular (con el
// mismo contenido que una evidencia), a uno fuera del directorio con ruta
// relativa, a un directorio, y colgado: AbrirRegular falla sin devolver
// archivo, y si el error es *NoRegular su Mode dice symlink (no lo que hay
// del otro lado). Nunca devuelve el contenido del destino.
func TestCA442_AbrirRegularNoSigueUnSymlink(t *testing.T) {
	dir := t.TempDir()
	afuera := t.TempDir()
	destino := filepath.Join(afuera, "destino.json")
	if err := os.WriteFile(destino, []byte("{\"severity\":\"low\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "vecino.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ caso, destino string }{
		{"symlink absoluto a un archivo regular de afuera", destino},
		{"symlink relativo a un vecino regular", "vecino.json"},
		{"symlink relativo que sale del directorio", filepath.Join("..", filepath.Base(afuera), "destino.json")},
		{"symlink a un directorio", "sub"},
		{"symlink colgado", filepath.Join(afuera, "no-existe.json")},
		{"symlink a si mismo", "bucle.json"},
	} {
		p := filepath.Join(dir, "enlace-"+strings.ReplaceAll(c.caso, " ", "-")+".json")
		if c.caso == "symlink a si mismo" {
			p = filepath.Join(dir, "bucle.json")
		}
		arSymlink(t, c.destino, p)
		f, err := arAbrir(t, c.caso, p)
		arNoRegular(t, c.caso, p, f, err, arEsSymlink)
	}
}

// CA-442 (98faf2): un directorio no se abre como evidencia (*NoRegular con
// ModeDir, o el error de la apertura); una ruta que no existe es
// fs.ErrNotExist, sin archivo.
func TestCA442_AbrirRegularNoAbreUnDirectorioYUnaRutaQueNoExisteEsErrNotExist(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "20261001T010101_abcdef.json")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := arAbrir(t, "directorio", sub)
	arNoRegular(t, "directorio", sub, f, err, fs.FileMode.IsDir)

	for _, p := range []string{filepath.Join(dir, "no-existe.json"), filepath.Join(dir, "no", "existe.json")} {
		f, err := arAbrir(t, "no existe", p)
		if f != nil || !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("CA-442: una ruta que no existe es fs.ErrNotExist sin archivo: %s: archivo=%v err=%v", p, f != nil, err)
		}
	}
}

// CA-442/CA-444 (98faf2): un archivo regular sin permiso de lectura no se
// abre (es el error de la apertura, no un archivo vacio).
func TestCA442_AbrirRegularNoAbreUnArchivoSinPermisoDeLectura(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root chmod 000 no quita la lectura")
	}
	p := filepath.Join(t.TempDir(), "cerrado.json")
	if err := os.WriteFile(p, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(p); err == nil {
		t.Skip("el sistema de archivos deja leer un archivo con modo 000")
	}
	f, err := arAbrir(t, "modo 000", p)
	if err == nil || f != nil {
		t.Fatalf("CA-444: un archivo sin permiso de lectura no se abre: archivo=%v err=%v", f != nil, err)
	}
}

// CA-442 (98faf2): el error de *NoRegular dice "no es un archivo regular"
// para cualquier tipo.
func TestCA442_NoRegularDiceQueNoEsUnArchivoRegular(t *testing.T) {
	for _, m := range []fs.FileMode{fs.ModeSymlink, fs.ModeNamedPipe, fs.ModeDir, fs.ModeSocket, fs.ModeDevice | fs.ModeCharDevice} {
		e := &NoRegular{Path: ".hoom/findings/20261001T010101_abcdef.json", Mode: m}
		if !strings.Contains(e.Error(), "no es un archivo regular") {
			t.Fatalf("CA-442: *NoRegular (%s) dice \"no es un archivo regular\": %q", m, e.Error())
		}
	}
}
