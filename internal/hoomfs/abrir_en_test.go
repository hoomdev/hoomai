// Tests de contrato de hoomfs.AbrirDirEn y hoomfs.AbrirRegularEn para los
// hallazgos 545ddc y a081cb de la tercera ronda de review de
// .hoom/specs/evidencia-en-disco.md (CA-442: un symlink "nunca se sigue ni
// se lee"; CA-441: el piso no puede fotografiar otra cosa en lugar de la
// evidencia). Abren name dentro de un *os.Root SOLO si es la misma entrada
// (os.SameFile) que describe el Lstat que el que llama acaba de sacar: un
// os.Root sigue un symlink que queda adentro, y la entrada se puede cambiar
// entre la mirada y la apertura. Escritos desde el contrato publico (go doc),
// sin leer la implementacion. Los casos con FIFO y las carreras estan en
// abrir_en_unix_test.go.
package hoomfs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

type aeDir struct {
	r   *os.Root
	err error
}

// aeRoot abre dir como *os.Root (se cierra al final del test).
func aeRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	r, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatalf("fixture: os.OpenRoot %s: %v", dir, err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

// aeLstat es r.Lstat(name), que tiene que andar: es la mirada del que llama.
func aeLstat(t *testing.T, r *os.Root, name string) fs.FileInfo {
	t.Helper()
	fi, err := r.Lstat(name)
	if err != nil {
		t.Fatalf("fixture: Lstat %s: %v", name, err)
	}
	return fi
}

// aeAbrirDir es AbrirDirEn con reloj. Si vuelve un root, se cierra al final
// del test.
func aeAbrirDir(t *testing.T, caso string, r *os.Root, name string, antes fs.FileInfo) (*os.Root, error) {
	t.Helper()
	ch := make(chan aeDir, 1)
	go func() {
		d, err := AbrirDirEn(r, name, antes)
		ch <- aeDir{d, err}
	}()
	select {
	case a := <-ch:
		if a.r != nil {
			t.Cleanup(func() { a.r.Close() })
		}
		return a.r, a.err
	case <-time.After(arRelojPorApertura):
		t.Fatalf("CA-442: %s: AbrirDirEn(%s) no volvio en %s: siguio o abrio algo que no es el directorio mirado", caso, name, arRelojPorApertura)
	}
	return nil, nil
}

// aeAbrirRegular es AbrirRegularEn con reloj. Si vuelve un archivo, se
// cierra al final del test.
func aeAbrirRegular(t *testing.T, caso string, r *os.Root, name string, antes fs.FileInfo) (*os.File, error) {
	t.Helper()
	ch := make(chan arApertura, 1)
	go func() {
		f, err := AbrirRegularEn(r, name, antes)
		ch <- arApertura{f, err}
	}()
	select {
	case a := <-ch:
		if a.f != nil {
			t.Cleanup(func() { a.f.Close() })
		}
		return a.f, a.err
	case <-time.After(arRelojPorApertura):
		t.Fatalf("CA-442: %s: AbrirRegularEn(%s) no volvio en %s: siguio o abrio algo que no es el archivo mirado", caso, name, arRelojPorApertura)
	}
	return nil, nil
}

// aeNombres lista el directorio que d abrio.
func aeNombres(t *testing.T, d *os.Root) []string {
	t.Helper()
	es, err := fs.ReadDir(d.FS(), ".")
	if err != nil {
		t.Fatalf("CA-442: el directorio que abrio AbrirDirEn se lista: %v", err)
	}
	var out []string
	for _, e := range es {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// aeRechazadoDir exige que AbrirDirEn no haya devuelto root.
func aeRechazadoDir(t *testing.T, caso string, d *os.Root, err error) {
	t.Helper()
	if err == nil || d != nil {
		var vio []string
		if d != nil {
			vio = aeNombres(t, d)
		}
		t.Fatalf("CA-442: %s: AbrirDirEn no abre lo que no es el directorio mirado: root=%v err=%v (lista %v)", caso, d != nil, err, vio)
	}
}

// aeRechazadoRegular exige que AbrirRegularEn no haya devuelto archivo; si el
// error es *NoRegular, que su Mode no diga regular y cumpla modo (nil: no se
// mira) y que diga "no es un archivo regular". Lo devuelve.
func aeRechazadoRegular(t *testing.T, caso string, f *os.File, err error, modo func(fs.FileMode) bool) *NoRegular {
	t.Helper()
	if err == nil || f != nil {
		var leyo []byte
		if f != nil {
			leyo, _ = io.ReadAll(f)
		}
		t.Fatalf("CA-442: %s: AbrirRegularEn no abre lo que no es el archivo regular mirado: archivo=%v err=%v (leyo %q)", caso, f != nil, err, leyo)
	}
	var nr *NoRegular
	if !errors.As(err, &nr) {
		return nil
	}
	if nr.Mode.IsRegular() || (modo != nil && !modo(nr.Mode)) {
		t.Fatalf("CA-442: %s: el Mode de *NoRegular dice lo que es la entrada (no lo que hay del otro lado): %s", caso, nr.Mode)
	}
	if !strings.Contains(nr.Error(), "no es un archivo regular") {
		t.Fatalf("CA-442: %s: el error de *NoRegular dice \"no es un archivo regular\": %q", caso, nr.Error())
	}
	return nr
}

// aeEscribir escribe dir/rel (y lo que falte arriba).
func aeEscribir(t *testing.T, dir, rel string, b []byte) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// ================================================================ AbrirDirEn

// CA-442 (545ddc, a081cb): el directorio que el Lstat describe se abre, y
// es ESE: se lista con lo que tiene, se lee lo de adentro, se baja un nivel
// con su propio Lstat (tambien con unicode en el nombre), y su Stat es el
// mismo archivo que el Lstat (os.SameFile).
func TestCA442_AbrirDirEnAbreElMismoDirectorioYLoLista(t *testing.T) {
	dir := t.TempDir()
	hallazgo := []byte("{\"severity\":\"high\"}\n")
	aeEscribir(t, dir, "findings/20261001T010101_abcdef.json", hallazgo)
	aeEscribir(t, dir, "findings/sub/20261001T020202_abcdef.json", []byte("{}\n"))
	aeEscribir(t, dir, "findings/ñandú dir/x.json", []byte("x"))
	r := aeRoot(t, dir)

	antes := aeLstat(t, r, "findings")
	d, err := aeAbrirDir(t, "directorio mirado", r, "findings", antes)
	if err != nil || d == nil {
		t.Fatalf("CA-442: el directorio que describe su Lstat se abre: %v", err)
	}
	if got, want := aeNombres(t, d), []string{"20261001T010101_abcdef.json", "sub", "ñandú dir"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CA-442: el directorio abierto es el mirado: lista %v, no %v", got, want)
	}
	if fi, err := d.Stat("."); err != nil || !os.SameFile(fi, antes) {
		t.Fatalf("CA-442: el directorio abierto es el mismo archivo que su Lstat: %v", err)
	}
	if raw, err := fs.ReadFile(d.FS(), "20261001T010101_abcdef.json"); err != nil || !bytes.Equal(raw, hallazgo) {
		t.Fatalf("CA-442: lo de adentro del directorio abierto se lee: %q %v", raw, err)
	}
	for _, c := range []struct {
		sub  string
		want []string
	}{
		{"sub", []string{"20261001T020202_abcdef.json"}},
		{"ñandú dir", []string{"x.json"}},
	} {
		s, err := aeAbrirDir(t, "un nivel mas abajo", d, c.sub, aeLstat(t, d, c.sub))
		if err != nil || s == nil {
			t.Fatalf("CA-442: %s se abre desde el directorio abierto con su Lstat: %v", c.sub, err)
		}
		if got := aeNombres(t, s); !reflect.DeepEqual(got, c.want) {
			t.Fatalf("CA-442: %s abierto lista %v, no %v", c.sub, got, c.want)
		}
	}
}

// aeCambio cambia la entrada findings de dir despues del Lstat.
type aeCambio struct {
	caso    string
	cambiar func(t *testing.T, dir, afuera string)
}

// aeApartar mueve dir/name a destino (en el mismo disco).
func aeApartar(t *testing.T, dir, name, destino string) {
	t.Helper()
	if err := os.Rename(filepath.Join(dir, name), destino); err != nil {
		t.Fatalf("fixture: apartar %s: %v", name, err)
	}
}

// CA-442 (545ddc, a081cb): si entre el Lstat y la apertura la entrada se
// cambio (rename) por otra cosa, AbrirDirEn la rechaza sin devolver root:
// otro directorio con el mismo contenido, un archivo regular, un symlink que
// queda adentro del root a otro directorio, un symlink que sale del root al
// MISMO directorio que se miro (apartado afuera: seguirlo seria leer fuera
// del arbol), un symlink relativo que sale del root, y nada (la entrada ya no
// existe).
func TestCA442_AbrirDirEnRechazaUnaEntradaCambiadaDespuesDeMirarla(t *testing.T) {
	for _, c := range []aeCambio{
		{"otro directorio con el mismo contenido", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, "findings", filepath.Join(dir, "viejo"))
			aeEscribir(t, dir, "findings/20261001T010101_abcdef.json", []byte("{\"severity\":\"high\"}\n"))
		}},
		{"un archivo regular", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, "findings", filepath.Join(dir, "viejo"))
			aeEscribir(t, dir, "findings", []byte("{\"severity\":\"high\"}\n"))
		}},
		{"un symlink adentro del root a otro directorio", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, "findings", filepath.Join(dir, "viejo"))
			aeEscribir(t, dir, "otro/20261001T010101_abcdef.json", []byte("{\"severity\":\"low\"}\n"))
			arSymlink(t, "otro", filepath.Join(dir, "findings"))
		}},
		{"un symlink absoluto afuera del root al mismo directorio apartado", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, "findings", filepath.Join(afuera, "findings"))
			arSymlink(t, filepath.Join(afuera, "findings"), filepath.Join(dir, "findings"))
		}},
		{"un symlink relativo afuera del root al mismo directorio apartado", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, "findings", filepath.Join(afuera, "findings"))
			arSymlink(t, filepath.Join("..", filepath.Base(afuera), "findings"), filepath.Join(dir, "findings"))
		}},
		{"nada (la entrada ya no existe)", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, "findings", filepath.Join(afuera, "findings"))
		}},
	} {
		t.Run(c.caso, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			dir, afuera := filepath.Join(base, "repo"), filepath.Join(base, "afuera")
			aeEscribir(t, dir, "findings/20261001T010101_abcdef.json", []byte("{\"severity\":\"high\"}\n"))
			if err := os.Mkdir(afuera, 0o755); err != nil {
				t.Fatal(err)
			}
			r := aeRoot(t, dir)
			antes := aeLstat(t, r, "findings")
			c.cambiar(t, dir, afuera)
			d, err := aeAbrirDir(t, c.caso, r, "findings", antes)
			aeRechazadoDir(t, c.caso, d, err)
		})
	}
}

// CA-442 (545ddc, a081cb): con el Lstat DEL SYMLINK (lo que el que llama
// vio), AbrirDirEn no lo sigue: un symlink que queda adentro del root a un
// directorio de verdad, uno al directorio de arriba, uno a si mismo, uno
// colgado y uno que sale del root; y un archivo regular con su propio Lstat
// tampoco se abre como directorio.
func TestCA442_AbrirDirEnNoSigueUnSymlinkConSuPropioLstat(t *testing.T) {
	base := t.TempDir()
	dir, afuera := filepath.Join(base, "repo"), filepath.Join(base, "afuera")
	aeEscribir(t, dir, "findings/sub/20261001T010101_abcdef.json", []byte("{}\n"))
	aeEscribir(t, dir, "findings/20261001T020202_abcdef.json", []byte("{}\n"))
	aeEscribir(t, afuera, "SECRETO.txt", []byte("de afuera\n"))
	for _, c := range []struct{ caso, enlace, destino string }{
		{"symlink adentro del root a un directorio", "findings/enlace", "sub"},
		{"symlink al directorio de arriba", "findings/arriba", ".."},
		{"symlink al mismo directorio", "findings/aca", "."},
		{"symlink a si mismo", "findings/bucle", "bucle"},
		{"symlink colgado", "findings/colgado", "no-existe"},
		{"symlink absoluto afuera del root", "findings/afuera", afuera},
		{"symlink relativo afuera del root", "findings/afuera-rel", filepath.Join("..", "..", "afuera")},
	} {
		arSymlink(t, c.destino, filepath.Join(dir, filepath.FromSlash(c.enlace)))
	}
	r := aeRoot(t, dir)
	f := aeLstat(t, r, "findings")
	findings, err := aeAbrirDir(t, "findings", r, "findings", f)
	if err != nil {
		t.Fatalf("CA-442: fixture: findings se abre: %v", err)
	}
	for _, n := range []string{"enlace", "arriba", "aca", "bucle", "colgado", "afuera", "afuera-rel", "20261001T020202_abcdef.json"} {
		antes := aeLstat(t, findings, n)
		d, err := aeAbrirDir(t, n, findings, n, antes)
		aeRechazadoDir(t, "findings/"+n+" con su propio Lstat ("+antes.Mode().String()+")", d, err)
	}
}

// ================================================================ AbrirRegularEn

// CA-442 (545ddc, a081cb): el archivo regular que el Lstat describe se abre
// y se lee entero (vacio, binario con unicode en el nombre, de mas de 1 MiB,
// un dotfile), desde el root y desde un directorio abierto con AbrirDirEn; el
// descriptor es el mismo archivo que el Lstat (os.SameFile) y es de solo
// lectura: escribir por el falla y el archivo no cambia.
func TestCA442_AbrirRegularEnAbreYLeeElMismoArchivo(t *testing.T) {
	dir := t.TempDir()
	archivos := map[string][]byte{
		"hallazgo.json":     []byte("{\"severity\":\"high\"}\n"),
		"vacio.json":        nil,
		"binario-ñandú.bin": {0x00, 0x01, 0xff, 0xfe, 0x00, '\n'},
		"grande.json":       bytes.Repeat([]byte("0123456789abcdef"), 1<<16+3),
		".oculto":           []byte("x"),
	}
	for n, b := range archivos {
		aeEscribir(t, dir, n, b)
		aeEscribir(t, dir, "findings/"+n, b)
	}
	r := aeRoot(t, dir)
	d, err := aeAbrirDir(t, "findings", r, "findings", aeLstat(t, r, "findings"))
	if err != nil {
		t.Fatalf("CA-442: fixture: findings se abre: %v", err)
	}
	for n, contenido := range archivos {
		for _, c := range []struct {
			caso string
			en   *os.Root
			p    string
		}{
			{"desde el root", r, filepath.Join(dir, n)},
			{"desde el directorio abierto", d, filepath.Join(dir, "findings", n)},
		} {
			caso := n + ", " + c.caso
			antes := aeLstat(t, c.en, n)
			f, err := aeAbrirRegular(t, caso, c.en, n, antes)
			if err != nil || f == nil {
				t.Fatalf("CA-442: %s: el archivo regular que describe su Lstat se abre: %v", caso, err)
			}
			fi, err := f.Stat()
			if err != nil || !fi.Mode().IsRegular() || !os.SameFile(fi, antes) {
				t.Fatalf("CA-442: %s: el descriptor es el del archivo regular mirado: %v %v", caso, fi, err)
			}
			got, err := io.ReadAll(f)
			if err != nil || !bytes.Equal(got, contenido) {
				t.Fatalf("CA-442: %s: se lee su contenido entero (%d bytes, leyo %d): %v", caso, len(contenido), len(got), err)
			}
			if k, err := f.Write([]byte("pisado")); err == nil || k != 0 {
				t.Fatalf("CA-442: %s: el descriptor es de solo lectura: escribio %d bytes (%v)", caso, k, err)
			}
			if raw, err := os.ReadFile(c.p); err != nil || !bytes.Equal(raw, contenido) {
				t.Fatalf("CA-442: %s: abrirlo no cambia el archivo: %v", caso, err)
			}
		}
	}
}

// CA-441/CA-442 (545ddc, a081cb): si entre el Lstat y la apertura la
// entrada se cambio (rename) por otra cosa, AbrirRegularEn la rechaza sin
// devolver archivo, aunque lo nuevo tenga el MISMO contenido (la foto le
// pondria a la evidencia la huella de otro archivo): otro archivo regular, un
// directorio, un symlink que queda adentro del root a otro archivo, un
// symlink que sale del root al MISMO archivo que se miro (apartado afuera), y
// nada.
func TestCA442_AbrirRegularEnRechazaUnaEntradaCambiadaDespuesDeMirarla(t *testing.T) {
	const nombre = "20261001T010101_abcdef.json"
	alto := []byte("{\"severity\":\"high\"}\n")
	for _, c := range []aeCambio{
		{"otro archivo regular con el mismo contenido", func(t *testing.T, dir, afuera string) {
			aeEscribir(t, afuera, "nuevo.json", alto)
			if err := os.Rename(filepath.Join(afuera, "nuevo.json"), filepath.Join(dir, nombre)); err != nil {
				t.Fatal(err)
			}
		}},
		{"un directorio", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, nombre, filepath.Join(afuera, nombre))
			aeEscribir(t, dir, nombre+"/adentro.json", alto)
		}},
		{"un symlink adentro del root a otro archivo con el mismo contenido", func(t *testing.T, dir, afuera string) {
			aeEscribir(t, dir, "copia.json", alto)
			aeApartar(t, dir, nombre, filepath.Join(afuera, nombre))
			arSymlink(t, "copia.json", filepath.Join(dir, nombre))
		}},
		{"un symlink absoluto afuera del root al mismo archivo apartado", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, nombre, filepath.Join(afuera, nombre))
			arSymlink(t, filepath.Join(afuera, nombre), filepath.Join(dir, nombre))
		}},
		{"nada (la entrada ya no existe)", func(t *testing.T, dir, afuera string) {
			aeApartar(t, dir, nombre, filepath.Join(afuera, nombre))
		}},
	} {
		t.Run(c.caso, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			dir, afuera := filepath.Join(base, "repo"), filepath.Join(base, "afuera")
			aeEscribir(t, dir, nombre, alto)
			if err := os.Mkdir(afuera, 0o755); err != nil {
				t.Fatal(err)
			}
			r := aeRoot(t, dir)
			antes := aeLstat(t, r, nombre)
			c.cambiar(t, dir, afuera)
			f, err := aeAbrirRegular(t, c.caso, r, nombre, antes)
			aeRechazadoRegular(t, c.caso, f, err, nil)
		})
	}
}

// CA-442 (545ddc, a081cb): con el Lstat DEL SYMLINK, AbrirRegularEn no lo
// sigue: a un archivo regular adentro del root (con el mismo contenido que
// una evidencia), a un directorio, colgado, a si mismo y afuera del root.
// Falla sin devolver archivo y, si el error es *NoRegular, su Mode dice
// symlink. Un directorio con su propio Lstat tampoco se abre (*NoRegular con
// ModeDir, o el error de la apertura).
func TestCA442_AbrirRegularEnNoSigueUnSymlinkConSuPropioLstat(t *testing.T) {
	base := t.TempDir()
	dir, afuera := filepath.Join(base, "repo"), filepath.Join(base, "afuera")
	aeEscribir(t, dir, "20261001T010101_abcdef.json", []byte("{\"severity\":\"high\"}\n"))
	aeEscribir(t, dir, "sub/x.json", []byte("{}\n"))
	aeEscribir(t, afuera, "destino.json", []byte("{\"severity\":\"low\"}\n"))
	enlaces := map[string]string{
		"enlace-regular.json":  "20261001T010101_abcdef.json",
		"enlace-dir.json":      "sub",
		"enlace-colgado.json":  "no-existe.json",
		"bucle.json":           "bucle.json",
		"enlace-afuera.json":   filepath.Join(afuera, "destino.json"),
		"enlace-afuera-r.json": filepath.Join("..", "afuera", "destino.json"),
	}
	for n, destino := range enlaces {
		arSymlink(t, destino, filepath.Join(dir, n))
	}
	r := aeRoot(t, dir)
	for n := range enlaces {
		antes := aeLstat(t, r, n)
		f, err := aeAbrirRegular(t, n, r, n, antes)
		aeRechazadoRegular(t, n+" con su propio Lstat", f, err, arEsSymlink)
	}
	f, err := aeAbrirRegular(t, "directorio", r, "sub", aeLstat(t, r, "sub"))
	aeRechazadoRegular(t, "un directorio con su propio Lstat", f, err, fs.FileMode.IsDir)
}

// CA-444 (545ddc): un archivo regular sin permiso de lectura no se abre con
// AbrirRegularEn (es el error de la apertura, no un archivo vacio).
func TestCA444_AbrirRegularEnNoAbreUnArchivoSinPermisoDeLectura(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("como root chmod 000 no quita la lectura")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "cerrado.json")
	aeEscribir(t, dir, "cerrado.json", []byte("{}\n"))
	t.Cleanup(func() { _ = os.Chmod(p, 0o644) })
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.ReadFile(p); err == nil {
		t.Skip("el sistema de archivos deja leer un archivo con modo 000")
	}
	r := aeRoot(t, dir)
	f, err := aeAbrirRegular(t, "modo 000", r, "cerrado.json", aeLstat(t, r, "cerrado.json"))
	if err == nil || f != nil {
		t.Fatalf("CA-444: un archivo sin permiso de lectura no se abre: archivo=%v err=%v", f != nil, err)
	}
}
