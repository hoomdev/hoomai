//go:build !windows

// Los casos con FIFO y las carreras de hoomfs.AbrirDirEn y
// hoomfs.AbrirRegularEn (hallazgos 545ddc y a081cb de la tercera ronda de
// review de .hoom/specs/evidencia-en-disco.md, CA-442): un FIFO no cuelga la
// apertura, ni mirado ni puesto en lugar de lo que se miro, un symlink a un
// FIFO no se sigue, y mientras otro proceso cambia la entrada sin parar lo
// que se abre es siempre lo mismo que describe el Lstat. El FIFO y las
// carreras son los de internal/carreratest.
package hoomfs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// aeCarrera es cuanto dura cada carrera.
const aeCarrera = 1500 * time.Millisecond

// CA-442 (545ddc): un FIFO (que nadie escribe) con su propio Lstat no
// cuelga AbrirRegularEn: vuelve enseguida con *NoRegular cuyo Mode dice
// ModeNamedPipe (es la misma entrada, y no es un archivo regular), y no
// queda abierto para leer. AbrirDirEn tampoco se cuelga ni lo abre: lo
// mirado no es un directorio.
func TestCA442_AbrirRegularEnYAbrirDirEnNoSeCuelganEnUnFIFO(t *testing.T) {
	for _, comoDir := range []bool{false, true} {
		caso := "AbrirRegularEn en un FIFO"
		if comoDir {
			caso = "AbrirDirEn en un FIFO"
		}
		t.Run(caso, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			const nombre = "20261001T020202_f1f0f1.json"
			fifo := filepath.Join(dir, nombre)
			carreratest.Fifo(t, fifo)
			r := aeRoot(t, dir)
			antes := aeLstat(t, r, nombre)
			if comoDir {
				d, err := aeAbrirDir(t, caso, r, nombre, antes)
				aeRechazadoDir(t, caso, d, err)
			} else {
				f, err := aeAbrirRegular(t, caso, r, nombre, antes)
				if nr := aeRechazadoRegular(t, caso, f, err, arEsFifo); nr == nil {
					t.Fatalf("CA-442: un FIFO con su propio Lstat es la misma entrada y no es un archivo regular: vuelve como *NoRegular con ModeNamedPipe: %v", err)
				}
			}
			arNadieLee(t, caso, fifo)
		})
	}
}

// CA-442 (545ddc): un symlink a un FIFO, adentro del root (relativo) o
// afuera (absoluto), con el Lstat del symlink, no se sigue: AbrirRegularEn y
// AbrirDirEn vuelven enseguida con error (si es *NoRegular, su Mode dice
// symlink y no ModeNamedPipe), y el FIFO no queda abierto para leer.
func TestCA442_AbrirRegularEnYAbrirDirEnNoSiguenUnSymlinkAUnFIFO(t *testing.T) {
	for _, c := range []struct {
		caso   string
		afuera bool
	}{
		{"symlink adentro del root a un FIFO", false},
		{"symlink absoluto afuera del root a un FIFO", true},
	} {
		for _, comoDir := range []bool{false, true} {
			caso := c.caso + ", AbrirRegularEn"
			if comoDir {
				caso = c.caso + ", AbrirDirEn"
			}
			t.Run(caso, func(t *testing.T) {
				t.Parallel()
				dir := t.TempDir()
				fifo, destino := filepath.Join(dir, "tubo"), "tubo"
				if c.afuera {
					fifo = filepath.Join(t.TempDir(), "tubo")
					destino = fifo
				}
				carreratest.Fifo(t, fifo)
				const enlace = "20261001T010101_5e1f01.json"
				arSymlink(t, destino, filepath.Join(dir, enlace))
				r := aeRoot(t, dir)
				antes := aeLstat(t, r, enlace)
				if comoDir {
					d, err := aeAbrirDir(t, caso, r, enlace, antes)
					aeRechazadoDir(t, caso, d, err)
				} else {
					f, err := aeAbrirRegular(t, caso, r, enlace, antes)
					aeRechazadoRegular(t, caso, f, err, func(m fs.FileMode) bool { return arEsSymlink(m) && !arEsFifo(m) })
				}
				arNadieLee(t, caso, fifo)
			})
		}
	}
}

// CA-442 (545ddc, a081cb): un FIFO puesto (rename) en lugar del archivo
// regular o del directorio que se miro, o un symlink a un FIFO puesto en
// lugar del directorio, no cuelga la apertura: AbrirRegularEn y AbrirDirEn
// vuelven enseguida con error, sin archivo ni root, y el FIFO no queda
// abierto para leer.
func TestCA442_UnFIFOPuestoEnLugarDeLoMiradoNoCuelgaLaApertura(t *testing.T) {
	for _, c := range []struct {
		caso    string
		dir     bool // lo mirado es un directorio
		symlink bool // lo que se pone es un symlink al FIFO (y no el FIFO)
	}{
		{"un FIFO en lugar del archivo regular", false, false},
		{"un symlink a un FIFO en lugar del archivo regular", false, true},
		{"un FIFO en lugar del directorio", true, false},
		{"un symlink a un FIFO en lugar del directorio", true, true},
	} {
		t.Run(c.caso, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			const nombre = "findings"
			fifo := filepath.Join(dir, "tubo")
			carreratest.Fifo(t, fifo)
			if c.dir {
				aeEscribir(t, dir, nombre+"/20261001T010101_abcdef.json", []byte("{}\n"))
			} else {
				aeEscribir(t, dir, nombre, []byte("{\"severity\":\"high\"}\n"))
			}
			r := aeRoot(t, dir)
			antes := aeLstat(t, r, nombre)
			aeApartar(t, dir, nombre, filepath.Join(t.TempDir(), nombre))
			if c.symlink {
				arSymlink(t, "tubo", filepath.Join(dir, nombre))
			} else if err := os.Link(fifo, filepath.Join(dir, nombre)); err != nil {
				t.Fatalf("fixture: enlace duro al FIFO: %v", err)
			}
			if c.dir {
				d, err := aeAbrirDir(t, c.caso, r, nombre, antes)
				aeRechazadoDir(t, c.caso, d, err)
			} else {
				f, err := aeAbrirRegular(t, c.caso, r, nombre, antes)
				aeRechazadoRegular(t, c.caso, f, err, nil)
			}
			arNadieLee(t, c.caso, fifo)
		})
	}
}

// CA-441/CA-442 (545ddc, a081cb, propiedad de carrera): mientras otro
// proceso cambia una ruta con rename atomico y sin parar entre un archivo
// regular, un symlink a un FIFO y el FIFO mismo, CADA Lstat seguido de
// AbrirRegularEn vuelve (con reloj por apertura); cuando devuelve un
// archivo, es el MISMO que describe su Lstat (os.SameFile), es regular y se
// lee el contenido regular entero; cuando lo que se miro no era un archivo
// regular, nunca devuelve archivo. Al final nadie quedo leyendo el FIFO.
func TestCA442_AbrirRegularEnDecideSobreElDescriptorAunqueLaEntradaCambie(t *testing.T) {
	dir := t.TempDir()
	const nombre = "20261001T060606_ca2e2a.json"
	regular := []byte("{\"severity\":\"high\",\"relleno\":\"" + string(bytes.Repeat([]byte("x"), 4096)) + "\"}\n")
	aeEscribir(t, dir, nombre, regular)
	fifo := filepath.Join(t.TempDir(), "fifo")
	carreratest.Fifo(t, fifo)
	r := aeRoot(t, dir)
	detener := carreratest.RegularYFifo(t, filepath.Join(dir, nombre), fifo, regular)

	vistas := map[string]int{}
	fin := time.Now().Add(aeCarrera)
	for time.Now().Before(fin) {
		antes, err := r.Lstat(nombre)
		if err != nil {
			detener()
			t.Fatalf("CA-442: fixture: con rename atomico %s siempre existe: %v", nombre, err)
		}
		ch := make(chan arApertura, 1)
		go func() {
			f, err := AbrirRegularEn(r, nombre, antes)
			ch <- arApertura{f, err}
		}()
		var a arApertura
		select {
		case a = <-ch:
		case <-time.After(arRelojPorApertura):
			detener()
			t.Fatalf("CA-442: AbrirRegularEn no volvio en %s mientras la entrada cambiaba entre un archivo regular y un FIFO", arRelojPorApertura)
		}
		if a.err != nil {
			if a.f != nil {
				a.f.Close()
				detener()
				t.Fatalf("CA-442: con error AbrirRegularEn no devuelve archivo: %v", a.err)
			}
			var nr *NoRegular
			if errors.As(a.err, &nr) && nr.Mode.IsRegular() {
				detener()
				t.Fatalf("CA-442: un *NoRegular nunca dice regular: %s", nr.Mode)
			}
			vistas["rechazada, mirada "+antes.Mode().Type().String()]++
			continue
		}
		fi, err := a.f.Stat()
		got, rerr := io.ReadAll(a.f)
		a.f.Close()
		if !antes.Mode().IsRegular() {
			detener()
			t.Fatalf("CA-442: lo mirado era %s y AbrirRegularEn devolvio un archivo", antes.Mode().Type())
		}
		if err != nil || !fi.Mode().IsRegular() || !os.SameFile(fi, antes) {
			detener()
			t.Fatalf("CA-442: lo que AbrirRegularEn devuelve es el archivo regular que describe su Lstat (os.SameFile): %v %v", fi, err)
		}
		if rerr != nil || !bytes.Equal(got, regular) {
			detener()
			t.Fatalf("CA-442: lo que AbrirRegularEn devuelve se lee entero y es el archivo regular (%d bytes, leyo %d): %v", len(regular), len(got), rerr)
		}
		vistas["regular"]++
	}
	cambios := detener()
	if cambios < 4 {
		t.Fatalf("CA-442: fixture: el intercambiador cambio la entrada %d veces", cambios)
	}
	arNadieLee(t, "despues de la carrera", fifo)
	t.Logf("CA-442: %d cambios, aperturas vistas %v", cambios, vistas)
}

// CA-442 (545ddc, a081cb, propiedad de carrera): mientras otro proceso
// cambia sin parar el directorio sub por un symlink (a otro directorio
// adentro del root, a un FIFO adentro del root, al root mismo, y afuera del
// root con ruta absoluta y relativa), por el FIFO mismo y por un archivo
// regular, y lo devuelve, CADA Lstat seguido de AbrirDirEn vuelve (con reloj
// por apertura); cuando devuelve un root, lo mirado era un directorio, lo
// abierto es ESE (os.SameFile) y lista lo de sub, nunca lo de otro, lo del
// root o lo de afuera; con el Lstat de lo que no es un directorio, nunca
// devuelve root. Al final nadie quedo leyendo el FIFO. Son dos carreras: una
// sin FIFO y otra con el symlink al FIFO y el FIFO mismo.
func TestCA442_AbrirDirEnDecideSobreElDescriptorAunqueLaEntradaCambie(t *testing.T) {
	for _, conFifo := range []bool{false, true} {
		caso := "symlinks a directorios y un archivo regular"
		if conFifo {
			caso = "un symlink a un FIFO y el FIFO mismo"
		}
		t.Run(caso, func(t *testing.T) {
			t.Parallel()
			base := t.TempDir()
			dir, afuera := filepath.Join(base, "repo"), filepath.Join(base, "afuera")
			aeEscribir(t, dir, "sub/solo-en-sub.json", []byte("{}\n"))
			aeEscribir(t, dir, "otro/solo-en-otro.json", []byte("{}\n"))
			aeEscribir(t, afuera, "SECRETO.txt", []byte("de afuera\n"))
			tubo := filepath.Join(dir, "tubo")
			carreratest.Fifo(t, tubo)
			r := aeRoot(t, dir)
			formas := []carreratest.Forma{carreratest.Symlink("otro"), carreratest.Symlink("."),
				carreratest.Symlink(afuera), carreratest.Symlink(filepath.Join("..", "afuera")),
				carreratest.Regular([]byte("{}\n"))}
			if conFifo {
				formas = []carreratest.Forma{carreratest.Symlink("tubo"), carreratest.EnlaceDuro(tubo)}
			}
			detener := carreratest.IntercambiarDir(t, filepath.Join(dir, "sub"), formas...)

			vistas := map[string]int{}
			fin := time.Now().Add(aeCarrera)
			for time.Now().Before(fin) {
				antes, err := r.Lstat("sub")
				if err != nil {
					vistas["falta"]++
					continue
				}
				ch := make(chan aeDir, 1)
				go func() {
					d, err := AbrirDirEn(r, "sub", antes)
					ch <- aeDir{d, err}
				}()
				var a aeDir
				select {
				case a = <-ch:
				case <-time.After(arRelojPorApertura):
					detener()
					t.Fatalf("CA-442: %s: AbrirDirEn no volvio en %s mientras sub cambiaba: abrio algo que no es el directorio mirado", caso, arRelojPorApertura)
				}
				if a.err != nil {
					if a.r != nil {
						a.r.Close()
						detener()
						t.Fatalf("CA-442: con error AbrirDirEn no devuelve root: %v", a.err)
					}
					vistas["rechazada, mirada "+antes.Mode().Type().String()]++
					continue
				}
				fi, serr := a.r.Stat(".")
				nombres := aeNombres(t, a.r)
				a.r.Close()
				if !antes.IsDir() {
					detener()
					t.Fatalf("CA-442: lo mirado era %s y AbrirDirEn devolvio un root que lista %v", antes.Mode().Type(), nombres)
				}
				if serr != nil || !os.SameFile(fi, antes) || !reflect.DeepEqual(nombres, []string{"solo-en-sub.json"}) {
					detener()
					t.Fatalf("CA-442: lo que AbrirDirEn devuelve es el directorio sub que describe su Lstat (os.SameFile): lista %v (%v)", nombres, serr)
				}
				vistas["directorio"]++
			}
			vueltas := detener()
			if vueltas < 4 {
				t.Fatalf("CA-442: fixture: el intercambiador dio %d vueltas", vueltas)
			}
			if fi, err := os.Lstat(filepath.Join(dir, "sub")); err != nil || !fi.IsDir() {
				t.Fatalf("CA-442: fixture: parado, sub es otra vez el directorio: %v", err)
			}
			arNadieLee(t, "despues de la carrera", tubo)
			t.Logf("CA-442: %s: %d vueltas, aperturas vistas %v", caso, vueltas, vistas)
		})
	}
}
