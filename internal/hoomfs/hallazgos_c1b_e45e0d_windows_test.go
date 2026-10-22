//go:build windows

// Tests de hoomfs.PublicarNuevo en Windows, en un volumen SIN hard links
// (exFAT, FAT), con que se publican los items
// (.hoom/specs/items-y-columna-derivada.md, CA-263 y CA-265) y los registros
// de review, que son append-only (CA-293). Escritos desde la spec y el
// hallazgo, nunca desde la implementacion. Cubren el hallazgo
// 20261022T145136_e45e0d (medium, reliability) de la delta 3 de
// fixes-cabina-c1b.
//
// El contrato (enmienda 2): PublicarNuevo publica con un enlace (hard link)
// bajo el candado. Si el enlace falla porque el volumen no tiene hard links,
// devuelve un error que envuelve hoomfs.ErrSinEnlaces (y que lo dice) y no
// escribe nada. En Unix eso es ENOTSUP, EOPNOTSUPP o EPERM
// (hallazgos_c1b_cbbc8c_434aec_test.go). En Windows, CreateHardLinkW sobre
// exFAT o FAT falla con ERROR_INVALID_FUNCTION (syscall.Errno(1)), y algunos
// volumenes con ERROR_NOT_SUPPORTED (syscall.Errno(50)); syscall.EPERM en
// Windows es un valor inventado por Go que CreateHardLinkW nunca devuelve, asi
// que no sirve para reconocer el caso. Con cualquier otro error de enlace
// (ERROR_ACCESS_DENIED, ERROR_WRITE_PROTECT, ERROR_DISK_FULL) PublicarNuevo
// devuelve ese error tal cual: no es ErrSinEnlaces ni os.ErrExist.
//
// El volumen sin hard links se simula reemplazando la variable de paquete
// enlazar (os.Link) por un enlace que falla como os.Link en Windows: un
// *os.LinkError con el syscall.Errno de CreateHardLinkW. El CI corre solo en
// Linux: este archivo se compila con GOOS=windows pero se ejecuta solo en
// Windows.
package hoomfs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"testing/quick"
)

// Los errores de CreateHardLinkW (winerror.h).
const (
	wnErrorInvalidFunction = syscall.Errno(1)   // ERROR_INVALID_FUNCTION: exFAT, FAT
	wnErrorAccessDenied    = syscall.Errno(5)   // ERROR_ACCESS_DENIED
	wnErrorWriteProtect    = syscall.Errno(19)  // ERROR_WRITE_PROTECT
	wnErrorNotSupported    = syscall.Errno(50)  // ERROR_NOT_SUPPORTED
	wnErrorDiskFull        = syscall.Errno(112) // ERROR_DISK_FULL
)

// wnNoSoportado son los errores con que falla CreateHardLinkW en un volumen
// sin hard links: los que llevan a ErrSinEnlaces en Windows.
var wnNoSoportado = []struct {
	nombre string
	errno  syscall.Errno
}{
	{"ERROR_INVALID_FUNCTION", wnErrorInvalidFunction},
	{"ERROR_NOT_SUPPORTED", wnErrorNotSupported},
}

// wnSinHardLinks reemplaza enlazar por un enlace que falla como os.Link en
// Windows (un *os.LinkError con errno) y no crea nada, hasta que se llama a
// restaurar o termina el test. Devuelve el contador de llamadas. Ningun test
// de este archivo es paralelo: nadie mas publica mientras enlazar esta
// reemplazado.
func wnSinHardLinks(t *testing.T, errno syscall.Errno) (llamadas *atomic.Int64, restaurar func()) {
	t.Helper()
	llamadas = new(atomic.Int64)
	previo := enlazar
	enlazar = func(viejo, nuevo string) error {
		llamadas.Add(1)
		return &os.LinkError{Op: "link", Old: viejo, New: nuevo, Err: errno}
	}
	var una sync.Once
	restaurar = func() { una.Do(func() { enlazar = previo }) }
	t.Cleanup(restaurar)
	return llamadas, restaurar
}

// wnSeLlamo exige que PublicarNuevo haya intentado el enlace (si no, el
// camino sin hard links no se ejercito y el test no prueba nada).
func wnSeLlamo(t *testing.T, llamadas *atomic.Int64) {
	t.Helper()
	if llamadas.Load() == 0 {
		t.Fatalf("fixture: PublicarNuevo nunca llamo a enlazar: el volumen sin hard links no se simulo")
	}
}

// wnRaiz arma un proyecto vacio: el directorio donde se publica
// (.hoom/items) y el candado del contrato (LockPath(root, "items")).
func wnRaiz(t *testing.T) (dir, candado string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir = filepath.Join(root, ".hoom", "items")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	return dir, LockPath(root, "items")
}

// wnEntradas lista TODO lo que hay en dir, ordenado.
func wnEntradas(t *testing.T, dir string) []string {
	t.Helper()
	es, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, e := range es {
		out = append(out, e.Name())
	}
	sort.Strings(out)
	return out
}

// wnSolo exige que en dir haya exactamente esos nombres: ningun temporal ni
// parcial.
func wnSolo(t *testing.T, ca, dir string, nombres ...string) {
	t.Helper()
	want := append([]string{}, nombres...)
	sort.Strings(want)
	if got := wnEntradas(t, dir); strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("%s: en %s debian quedar solo %q, sin temporales ni parciales; quedan %q", ca, dir, want, got)
	}
}

// wnIgual exige que path tenga exactamente want.
func wnIgual(t *testing.T, ca, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%s: %s se lee: %v", ca, path, err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s: %s debia tener los %d bytes esperados, entero; tiene %d bytes distintos", ca, path, len(want), len(got))
	}
}

// wnNoEsta exige que el nombre final no exista (Lstat: ni archivo ni nada).
func wnNoEsta(t *testing.T, ca, final string) {
	t.Helper()
	if info, err := os.Lstat(final); !errors.Is(err, os.ErrNotExist) {
		sz := int64(-1)
		if err == nil {
			sz = info.Size()
		}
		t.Fatalf("%s: el nombre final %s debia no existir; Lstat = %v (%d bytes)", ca, final, err, sz)
	}
}

// wnEsSinEnlaces explica por que err no es el error de un volumen sin hard
// links sobre un nombre libre, o "" si lo es: no nil, envuelve
// ErrSinEnlaces, lo dice en su texto, y no es os.ErrExist (el nombre estaba
// libre).
func wnEsSinEnlaces(err error) string {
	switch {
	case err == nil:
		return "devolvio nil (publico)"
	case !errors.Is(err, ErrSinEnlaces):
		return fmt.Sprintf("no envuelve hoomfs.ErrSinEnlaces, devolvio el error crudo: %v", err)
	case errors.Is(err, os.ErrExist):
		return fmt.Sprintf("es os.ErrExist con el nombre libre: %v", err)
	case !strings.Contains(err.Error(), ErrSinEnlaces.Error()):
		return fmt.Sprintf("no dice que .hoom necesita hard links (%q): %v", ErrSinEnlaces.Error(), err)
	}
	return ""
}

// wnDatos son n bytes deterministas que dependen de la semilla, con NUL y
// saltos de linea adentro.
func wnDatos(n, semilla int) []byte {
	b := make([]byte, n)
	x := uint32(2166136261) ^ uint32(semilla)*16777619
	for i := range b {
		x ^= uint32(i)
		x *= 16777619
		b[i] = byte(x>>24) ^ byte(semilla)
	}
	return b
}

// Hallazgo 20261022T145136_e45e0d. CA-263 y CA-293: en Windows, sin hard
// links (CreateHardLinkW falla con ERROR_INVALID_FUNCTION, el de exFAT y FAT,
// o con ERROR_NOT_SUPPORTED), PublicarNuevo sobre un nombre libre devuelve un
// error que envuelve ErrSinEnlaces (no el *os.LinkError crudo) y no escribe
// nada: ni el nombre final, ni temporales, para datos vacios, de un byte,
// unicode, binarios y grandes. Intentarlo otra vez da lo mismo y no acumula
// nada. Guarda: el candado quedo suelto y el nombre libre, asi que con hard
// links (NTFS), despues, se publica entero.
func TestCA263_HallazgoE45e0d_WindowsSinHardLinksEsErrSinEnlacesYNoEscribeNada(t *testing.T) {
	casos := []struct {
		nombre string
		datos  []byte
	}{
		{"vacio", []byte{}},
		{"un byte NUL", []byte{0}},
		{"texto con unicode", []byte("titulo: Precios por región ñandú 🦙\r\npedido: |\r\n  x\r\n")},
		{"binario de 64 KiB", wnDatos(64<<10, 21)},
		{"grande de 8 MiB", wnDatos(8<<20, 22)},
	}
	for _, e := range wnNoSoportado {
		for _, c := range casos {
			t.Run(e.nombre+"/"+c.nombre, func(t *testing.T) {
				llamadas, restaurar := wnSinHardLinks(t, e.errno)
				dir, candado := wnRaiz(t)
				final := filepath.Join(dir, "precios.yaml")
				for intento := 1; intento <= 3; intento++ {
					err := PublicarNuevo(final, c.datos, 0o644, candado)
					wnSeLlamo(t, llamadas)
					if por := wnEsSinEnlaces(err); por != "" {
						t.Fatalf("CA-263: hallazgo e45e0d: en Windows sin hard links (CreateHardLinkW da %s = syscall.Errno(%d)), intento %d, PublicarNuevo sobre un nombre libre falla con ErrSinEnlaces: %s",
							e.nombre, uint(e.errno), intento, por)
					}
					wnNoEsta(t, fmt.Sprintf("CA-263: hallazgo e45e0d: en Windows sin hard links (%s), intento %d, no se escribe nada", e.nombre, intento), final)
					if got := wnEntradas(t, dir); len(got) != 0 {
						t.Fatalf("CA-293: hallazgo e45e0d: en Windows sin hard links (%s), intento %d, no quedan temporales ni parciales en %s: %q",
							e.nombre, intento, dir, got)
					}
				}

				// guarda: el candado quedo suelto y el nombre libre
				restaurar()
				if err := PublicarNuevo(final, c.datos, 0o644, candado); err != nil {
					t.Fatalf("CA-263: guarda: tras fallar sin hard links (%s) el candado se suelta y el nombre queda libre: con hard links se publica: %v", e.nombre, err)
				}
				wnIgual(t, "CA-263: guarda: con hard links", final, c.datos)
				wnSolo(t, "CA-263: guarda: con hard links", dir, "precios.yaml")
			})
		}
	}
}

// Hallazgo 20261022T145136_e45e0d. CA-265: en Windows sin hard links
// (ERROR_INVALID_FUNCTION o ERROR_NOT_SUPPORTED), si el nombre final ya esta
// tomado (un archivo de otro escritor, uno vacio, un directorio),
// PublicarNuevo falla con ErrSinEnlaces u os.ErrExist (nunca con el
// *os.LinkError crudo) y no lo toca ni deja temporales.
func TestCA265_HallazgoE45e0d_WindowsSinHardLinksNombreOcupadoQuedaIntacto(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T, final string)
		sigue  func(t *testing.T, final string)
	}{
		{"archivo de otro escritor", func(t *testing.T, final string) {
			if err := os.WriteFile(final, []byte("titulo: del otro escritor\r\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final string) {
			wnIgual(t, "CA-265: hallazgo e45e0d: el archivo del otro escritor", final, []byte("titulo: del otro escritor\r\n"))
		}},
		{"archivo vacio de otro escritor", func(t *testing.T, final string) {
			if err := os.WriteFile(final, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final string) {
			wnIgual(t, "CA-265: hallazgo e45e0d: el archivo vacio del otro escritor sigue vacio", final, []byte{})
		}},
		{"directorio con un archivo", func(t *testing.T, final string) {
			if err := os.MkdirAll(final, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(final, "adentro"), []byte("intacto"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, func(t *testing.T, final string) {
			if es, err := os.ReadDir(final); err != nil || len(es) != 1 {
				t.Fatalf("CA-265: hallazgo e45e0d: el directorio sigue con lo suyo y nada mas: %v %v", es, err)
			}
			wnIgual(t, "CA-265: hallazgo e45e0d: lo de adentro del directorio", filepath.Join(final, "adentro"), []byte("intacto"))
		}},
	}
	for _, e := range wnNoSoportado {
		for _, c := range casos {
			t.Run(e.nombre+"/"+c.nombre, func(t *testing.T) {
				llamadas, _ := wnSinHardLinks(t, e.errno)
				dir, candado := wnRaiz(t)
				final := filepath.Join(dir, "precios.yaml")
				c.armar(t, final)
				antes, err := os.Lstat(final)
				if err != nil {
					t.Fatal(err)
				}
				err = PublicarNuevo(final, wnDatos(4096, 23), 0o644, candado)
				t.Logf("enlazar llamado %d veces; error: %v", llamadas.Load(), err)
				if err == nil {
					t.Fatalf("CA-265: hallazgo e45e0d: en Windows sin hard links (%s), con el nombre tomado por un %s PublicarNuevo falla, devolvio nil", e.nombre, c.nombre)
				}
				if !errors.Is(err, ErrSinEnlaces) && !errors.Is(err, os.ErrExist) {
					t.Fatalf("CA-265: hallazgo e45e0d: en Windows sin hard links (%s), con el nombre tomado por un %s el error es ErrSinEnlaces u os.ErrExist, no el error crudo del enlace; fue %v", e.nombre, c.nombre, err)
				}
				despues, err := os.Lstat(final)
				if err != nil || !os.SameFile(antes, despues) || despues.Mode() != antes.Mode() || despues.Size() != antes.Size() {
					t.Fatalf("CA-265: hallazgo e45e0d: en Windows sin hard links, el %s que ocupaba el nombre no se reemplaza, ni se borra, ni cambia: %v %v", c.nombre, despues, err)
				}
				c.sigue(t, final)
				wnSolo(t, "CA-265: hallazgo e45e0d: en Windows sin hard links, el intento fallido no deja temporales", dir, "precios.yaml")
			})
		}
	}
}

// GUARDA. Hallazgo 20261022T145136_e45e0d. CA-265 y CA-293: en Windows solo
// ERROR_INVALID_FUNCTION y ERROR_NOT_SUPPORTED son "no soportado". Si
// CreateHardLinkW falla con cualquier otro error (ERROR_ACCESS_DENIED,
// ERROR_WRITE_PROTECT, ERROR_DISK_FULL), PublicarNuevo devuelve ese error tal
// cual (errors.Is), que no es ErrSinEnlaces ni os.ErrExist, no crea el nombre
// final por otro camino y no deja temporales.
func TestCA265_HallazgoE45e0d_GuardaWindowsOtroErrorDeEnlaceSeDevuelve(t *testing.T) {
	for _, e := range []struct {
		nombre string
		errno  syscall.Errno
	}{
		{"ERROR_ACCESS_DENIED", wnErrorAccessDenied},
		{"ERROR_WRITE_PROTECT", wnErrorWriteProtect},
		{"ERROR_DISK_FULL", wnErrorDiskFull},
	} {
		t.Run(e.nombre, func(t *testing.T) {
			llamadas, _ := wnSinHardLinks(t, e.errno)
			dir, candado := wnRaiz(t)
			final := filepath.Join(dir, "precios.yaml")
			err := PublicarNuevo(final, wnDatos(4096, 24), 0o644, candado)
			wnSeLlamo(t, llamadas)
			if err == nil {
				t.Fatalf("CA-265: hallazgo e45e0d: CreateHardLinkW fallo con %s (no es \"no soportado\"): PublicarNuevo devuelve ese error y no publica por otro camino, devolvio nil", e.nombre)
			}
			if !errors.Is(err, e.errno) {
				t.Fatalf("CA-265: hallazgo e45e0d: CreateHardLinkW fallo con %s = syscall.Errno(%d): PublicarNuevo devuelve ese error tal cual (errors.Is), devolvio %v", e.nombre, uint(e.errno), err)
			}
			if errors.Is(err, ErrSinEnlaces) {
				t.Fatalf("CA-265: hallazgo e45e0d: CreateHardLinkW fallo con %s, que no es \"no soportado\": el error no es ErrSinEnlaces (no dice que faltan hard links): %v", e.nombre, err)
			}
			if errors.Is(err, os.ErrExist) {
				t.Fatalf("CA-265: hallazgo e45e0d: CreateHardLinkW fallo con %s y el nombre estaba libre: el error no es os.ErrExist: %v", e.nombre, err)
			}
			wnNoEsta(t, fmt.Sprintf("CA-293: hallazgo e45e0d: CreateHardLinkW fallo con %s: PublicarNuevo no publica por otro camino", e.nombre), final)
			if got := wnEntradas(t, dir); len(got) != 0 {
				t.Fatalf("CA-293: hallazgo e45e0d: CreateHardLinkW fallo con %s y en %s quedaron temporales: %q", e.nombre, dir, got)
			}
		})
	}
}

// Hallazgo 20261022T145136_e45e0d. CA-263 y CA-293, como propiedad: en
// Windows sin hard links, para cualquier contenido, cualquier nombre libre y
// cualquiera de los dos errores "no soportado" de CreateHardLinkW,
// PublicarNuevo falla con ErrSinEnlaces y en el directorio no queda nada.
func TestCA263_HallazgoE45e0d_WindowsSinHardLinksPropiedad(t *testing.T) {
	var fallas []string
	k := 0
	prop := func(datos []byte, cual uint8, sufijo uint16) bool {
		k++
		e := wnNoSoportado[int(cual)%len(wnNoSoportado)]
		llamadas, restaurar := wnSinHardLinks(t, e.errno)
		defer restaurar()
		dir, candado := wnRaiz(t)
		nombre := fmt.Sprintf("item-%05d.yaml", sufijo)
		final := filepath.Join(dir, nombre)
		err := PublicarNuevo(final, datos, 0o644, candado)
		if llamadas.Load() == 0 {
			fallas = append(fallas, fmt.Sprintf("caso %d (%s): PublicarNuevo nunca llamo a enlazar", k, e.nombre))
			return false
		}
		if por := wnEsSinEnlaces(err); por != "" {
			fallas = append(fallas, fmt.Sprintf("caso %d (%s, %d bytes, %s): %s", k, e.nombre, len(datos), nombre, por))
			return false
		}
		if got := wnEntradas(t, dir); len(got) != 0 {
			fallas = append(fallas, fmt.Sprintf("caso %d (%s): en %s quedo %q", k, e.nombre, dir, got))
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 60}); err != nil {
		t.Fatalf("CA-263: hallazgo e45e0d: en Windows sin hard links PublicarNuevo falla con ErrSinEnlaces y no escribe nada: %q (%v)", fallas, err)
	}
}
