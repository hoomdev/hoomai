// Tests de contrato de hoomfs.AbrirRegularEn y hoomfs.AbrirDirEn para los
// hallazgos bd4403 y 402186 de la cuarta ronda de review de
// .hoom/specs/evidencia-en-disco.md (enmienda 2, CA-442 y CA-445: "un nombre
// que, entre mirarlo y abrirlo, pasa a ser un symlink (a otro objeto o al
// mismo movido) o cualquier otra cosa queda ilegible, nunca se sigue").
//
// El ataque: el que llama saca el Lstat de name; despues alguien mueve ESE
// objeto a otro nombre adentro del root y pone en name un symlink al objeto
// movido. Un os.Root sigue el symlink (queda adentro), y el descriptor que
// abre es el MISMO archivo o directorio (mismo dispositivo e inodo):
// os.SameFile contra el Lstat de antes no lo ve. El contrato pide ademas que
// el nombre siga siendo, despues de abrirlo, el objeto que describe el
// Lstat; si no, la apertura se rechaza sin devolver archivo ni root.
//
// Deterministas: el cambio se hace entero entre el Lstat y la apertura, y
// queda puesto (es el caso del hallazgo en que la segunda foto saldria
// identica a la primera). Escritos desde el contrato publico (go doc), sin
// leer la implementacion. El FIFO esta en abrir_en_mismo_inodo_unix_test.go.
package hoomfs

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// miAlias es un cambio de name entre el Lstat y la apertura: el objeto se
// mueve a aparte (relativo al directorio de name, adentro del root) y en
// name queda un symlink con el texto enlace. Si copia, lo que hay en aparte
// es una COPIA (otro inodo, el mismo contenido) y el original sale del root:
// es el control de que tambien se rechaza un symlink a otro objeto.
type miAlias struct {
	caso    string
	aparte  string
	enlace  func(dir string) string // texto del symlink (dir: el directorio de name)
	eslabon string                  // si no es "", name -> eslabon -> enlace (una cadena)
	copia   bool
	// abs: el texto del symlink es absoluto. Uno relativo al mismo objeto
	// es justo lo que un os.Root sigue: la fixture lo comprueba.
	abs bool
}

func miRel(texto string) func(string) string { return func(string) string { return texto } }

// miAliases son los cambios: el mismo objeto movido al lado, a un
// subdirectorio, por un camino que sube y baja, con ruta absoluta adentro
// del root y por una cadena de dos symlinks; y el control con una copia.
func miAliases(nombre string) []miAlias {
	return []miAlias{
		{caso: "el mismo objeto movido al lado, symlink relativo", aparte: "aux-" + nombre, enlace: miRel("aux-" + nombre)},
		{caso: "el mismo objeto movido al lado, symlink relativo con ./", aparte: "aux-" + nombre, enlace: miRel("./aux-" + nombre)},
		{caso: "el mismo objeto movido a un subdirectorio, symlink relativo", aparte: "aparte/aux", enlace: miRel("aparte/aux")},
		{caso: "el mismo objeto movido a un subdirectorio, symlink que sube y baja", aparte: "aparte/aux", enlace: miRel("aparte/../aparte/aux")},
		{caso: "el mismo objeto movido al lado, symlink absoluto adentro del root", aparte: "aux-" + nombre, enlace: func(dir string) string {
			return filepath.Join(dir, "aux-"+nombre)
		}, abs: true},
		{caso: "el mismo objeto movido al lado, una cadena de dos symlinks", aparte: "aux-" + nombre, enlace: miRel("aux-" + nombre), eslabon: "eslabon"},
		{caso: "control: una copia con el mismo contenido (otro inodo), symlink relativo", aparte: "copia-" + nombre, enlace: miRel("copia-" + nombre), copia: true},
	}
}

// miCopiar copia el archivo o el arbol de src en dst (dst no existe).
func miCopiar(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		q := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(q, 0o755)
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(q, raw, 0o644)
	})
	if err != nil {
		t.Fatalf("fixture: copiar %s: %v", src, err)
	}
}

// miCambiar hace el cambio a de dir/nombre (nombre en r), despues del Lstat
// antes, y exige que la fixture mida lo que dice: name es un symlink, y
// seguirlo lleva al MISMO objeto que antes (os.SameFile) o, en el control, a
// otro. Con un symlink relativo al mismo objeto, ademas, el os.Root solo lo
// sigue y abre ESE objeto: os.SameFile sobre el descriptor no ve el cambio
// (es el hallazgo bd4403), y lo unico que lo ve es mirar el nombre.
func miCambiar(t *testing.T, r *os.Root, dir, nombre string, a miAlias, antes os.FileInfo) {
	t.Helper()
	p, aparte := filepath.Join(dir, nombre), filepath.Join(dir, filepath.FromSlash(a.aparte))
	if err := os.MkdirAll(filepath.Dir(aparte), 0o755); err != nil {
		t.Fatal(err)
	}
	if a.copia {
		miCopiar(t, p, aparte)
		aeApartar(t, dir, nombre, filepath.Join(t.TempDir(), "original"))
	} else if err := os.Rename(p, aparte); err != nil {
		t.Fatalf("fixture: mover %s a %s: %v", nombre, a.aparte, err)
	}
	texto := a.enlace(dir)
	if a.eslabon != "" {
		arSymlink(t, texto, filepath.Join(dir, a.eslabon))
		texto = a.eslabon
	}
	arSymlink(t, texto, p)
	if fi, err := os.Lstat(p); err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("CA-445: fixture: %s: %s es un symlink: %v %v", a.caso, nombre, fi, err)
	}
	fi, err := os.Stat(p)
	if err != nil {
		t.Fatalf("CA-445: fixture: %s: el symlink %s lleva a algo: %v", a.caso, nombre, err)
	}
	if mismo := os.SameFile(fi, antes); mismo == a.copia {
		t.Fatalf("CA-445: fixture: %s: seguir %s lleva al mismo objeto que su Lstat = %v (esperaba %v)", a.caso, nombre, mismo, !a.copia)
	}
	if a.copia || a.abs {
		return
	}
	f, err := r.Open(nombre)
	if err != nil {
		t.Fatalf("CA-445: fixture: %s: el os.Root sigue el symlink relativo %s al mismo objeto: %v", a.caso, nombre, err)
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !os.SameFile(fi, antes) {
		t.Fatalf("CA-445: fixture: %s: lo que abre el os.Root siguiendo %s es el mismo objeto que su Lstat (os.SameFile): %v", a.caso, nombre, err)
	}
}

// CA-445/CA-442 (hallazgos bd4403, 402186): un archivo regular mirado
// (Lstat) y despues movido adentro del root, con un symlink en su nombre al
// MISMO archivo movido (relativo, con ./, a un subdirectorio, subiendo y
// bajando, absoluto adentro del root, por una cadena de dos symlinks): el
// descriptor seria el mismo archivo (os.SameFile), pero el nombre ya no es
// el archivo regular mirado, y AbrirRegularEn lo rechaza sin devolver
// archivo, desde el root y desde un directorio abierto con AbrirDirEn.
// Control: un symlink a una copia (otro inodo) tambien se rechaza, y el
// archivo sin tocar se abre y se lee entero.
func TestCA445_AbrirRegularEnRechazaUnSymlinkAlMismoArchivoMovido(t *testing.T) {
	const nombre = "20261001T010101_abcdef.json"
	alto := []byte("{\"severity\":\"high\"}\n")
	for _, a := range miAliases(nombre) {
		for _, desde := range []string{"el root", "un directorio abierto con AbrirDirEn"} {
			caso := a.caso + ", desde " + desde
			t.Run(caso, func(t *testing.T) {
				t.Parallel()
				base := t.TempDir()
				arriba := "."
				if desde != "el root" {
					arriba = "findings"
				}
				aeEscribir(t, base, filepath.Join(arriba, nombre), alto)
				r, dir := aeRoot(t, base), base
				if desde != "el root" {
					d, err := aeAbrirDir(t, caso, r, arriba, aeLstat(t, r, arriba))
					if err != nil || d == nil {
						t.Fatalf("CA-445: fixture: %s se abre: %v", arriba, err)
					}
					r, dir = d, filepath.Join(base, arriba)
				}

				antes := aeLstat(t, r, nombre)
				if !antes.Mode().IsRegular() {
					t.Fatalf("CA-445: fixture: %s es un archivo regular: %s", nombre, antes.Mode())
				}
				miCambiar(t, r, dir, nombre, a, antes)
				f, err := aeAbrirRegular(t, caso, r, nombre, antes)
				aeRechazadoRegular(t, "CA-445: "+caso, f, err, nil)
			})
		}
	}

	t.Run("control: sin tocar se abre y es el mismo archivo", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		aeEscribir(t, dir, nombre, alto)
		r := aeRoot(t, dir)
		antes := aeLstat(t, r, nombre)
		f, err := aeAbrirRegular(t, "sin tocar", r, nombre, antes)
		if err != nil || f == nil {
			t.Fatalf("CA-445: el archivo regular que nadie toco se abre: %v", err)
		}
		fi, err := f.Stat()
		if err != nil || !os.SameFile(fi, antes) {
			t.Fatalf("CA-445: el descriptor es el del archivo mirado: %v", err)
		}
		if got, err := io.ReadAll(f); err != nil || !bytes.Equal(got, alto) {
			t.Fatalf("CA-445: se lee entero: %q %v", got, err)
		}
	})
}

// CA-445/CA-442 (hallazgos bd4403, 402186): un directorio mirado (Lstat) y
// despues movido adentro del root, con un symlink en su nombre al MISMO
// directorio movido (las mismas formas que para un archivo): el root que
// abriria es el mismo directorio (os.SameFile), pero el nombre ya no es el
// directorio mirado, y AbrirDirEn lo rechaza sin devolver root, desde el
// root y un nivel mas abajo (desde un directorio abierto con AbrirDirEn,
// como recorre la foto). Control: un symlink a una copia (otro inodo)
// tambien se rechaza, y el directorio sin tocar se abre y lista lo suyo.
func TestCA445_AbrirDirEnRechazaUnSymlinkAlMismoDirectorioMovido(t *testing.T) {
	const nombre = "findings"
	for _, a := range miAliases(nombre) {
		for _, desde := range []string{"el root", "un directorio abierto con AbrirDirEn"} {
			caso := a.caso + ", desde " + desde
			t.Run(caso, func(t *testing.T) {
				t.Parallel()
				base := t.TempDir()
				arriba := "."
				if desde != "el root" {
					arriba = "hoom"
				}
				aeEscribir(t, base, filepath.Join(arriba, nombre, "20261001T010101_abcdef.json"), []byte("{\"severity\":\"high\"}\n"))
				r, dir := aeRoot(t, base), base
				if desde != "el root" {
					d, err := aeAbrirDir(t, caso, r, arriba, aeLstat(t, r, arriba))
					if err != nil || d == nil {
						t.Fatalf("CA-445: fixture: %s se abre: %v", arriba, err)
					}
					r, dir = d, filepath.Join(base, arriba)
				}

				antes := aeLstat(t, r, nombre)
				if !antes.IsDir() {
					t.Fatalf("CA-445: fixture: %s es un directorio: %s", nombre, antes.Mode())
				}
				miCambiar(t, r, dir, nombre, a, antes)
				d, err := aeAbrirDir(t, caso, r, nombre, antes)
				aeRechazadoDir(t, "CA-445: "+caso, d, err)
			})
		}
	}

	t.Run("control: sin tocar se abre y lista lo suyo", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		aeEscribir(t, dir, nombre+"/20261001T010101_abcdef.json", []byte("{}\n"))
		r := aeRoot(t, dir)
		antes := aeLstat(t, r, nombre)
		d, err := aeAbrirDir(t, "sin tocar", r, nombre, antes)
		if err != nil || d == nil {
			t.Fatalf("CA-445: el directorio que nadie toco se abre: %v", err)
		}
		if got := aeNombres(t, d); !reflect.DeepEqual(got, []string{"20261001T010101_abcdef.json"}) {
			t.Fatalf("CA-445: el directorio abierto es el mirado: lista %v", got)
		}
	})
}
