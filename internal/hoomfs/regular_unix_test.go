//go:build !windows

// Los casos con FIFO de hoomfs.AbrirRegular (hallazgo 98faf2 de la segunda
// ronda de review de .hoom/specs/evidencia-en-disco.md, CA-442): un FIFO no
// bloquea la apertura, un symlink a un FIFO no se sigue, y la decision se
// toma sobre el descriptor aunque otro proceso cambie la entrada sin parar.
// El FIFO y la carrera son los de internal/carreratest (hallazgo b08c53: una
// sola copia de la fixture para hoomfs, gitx y agentcmd).
package hoomfs

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// arNadieLee exige que ningun descriptor de lectura quede abierto en el
// FIFO: abrirlo para escribir sin bloquear falla (ENXIO) si nadie lo lee.
func arNadieLee(t *testing.T, caso, fifo string) {
	t.Helper()
	f, err := os.OpenFile(fifo, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err == nil {
		f.Close()
		t.Fatalf("CA-442: %s: AbrirRegular dejo abierto para leer el FIFO %s (lo siguio o no lo cerro)", caso, fifo)
	}
}

func arEsFifo(m fs.FileMode) bool { return m&fs.ModeNamedPipe != 0 }

// CA-442 (98faf2): un FIFO con nombre de evidencia, que nadie escribe, no
// cuelga la apertura: vuelve enseguida con *NoRegular cuyo Mode dice
// ModeNamedPipe, y no queda abierto.
func TestCA442_AbrirRegularNoSeCuelgaEnUnFIFO(t *testing.T) {
	p := filepath.Join(t.TempDir(), "20261001T020202_f1f0f1.json")
	carreratest.Fifo(t, p)
	f, err := arAbrir(t, "FIFO", p)
	nr := arNoRegular(t, "FIFO", p, f, err, arEsFifo)
	if nr == nil {
		t.Fatalf("CA-442: un FIFO vuelve como *NoRegular con ModeNamedPipe (abrirlo sin seguir ni bloquear no falla): %v", err)
	}
	arNadieLee(t, "FIFO", p)
}

// CA-442 (98faf2): un symlink a un FIFO de afuera (absoluto o relativo) no
// se sigue: vuelve enseguida con un error que no es el del FIFO (si es
// *NoRegular, su Mode dice symlink y no ModeNamedPipe), y el FIFO no queda
// abierto para leer.
func TestCA442_AbrirRegularNoSigueUnSymlinkAUnFIFO(t *testing.T) {
	dir := t.TempDir()
	afuera := t.TempDir()
	fifo := filepath.Join(afuera, "fifo")
	carreratest.Fifo(t, fifo)
	for i, c := range []struct{ caso, destino string }{
		{"symlink absoluto a un FIFO", fifo},
		{"symlink relativo a un FIFO", filepath.Join("..", filepath.Base(afuera), "fifo")},
	} {
		p := filepath.Join(dir, fmt.Sprintf("20261001T01010%d_5e1f01.json", i))
		arSymlink(t, c.destino, p)
		f, err := arAbrir(t, c.caso, p)
		arNoRegular(t, c.caso, p, f, err, func(m fs.FileMode) bool { return arEsSymlink(m) && !arEsFifo(m) })
		arNadieLee(t, c.caso, fifo)
	}
}

// CA-442 (98faf2, propiedad de carrera): mientras otro proceso cambia una
// ruta con rename atomico y sin parar entre un archivo regular, un symlink a
// un FIFO y el FIFO mismo (un enlace duro), CADA AbrirRegular vuelve (con
// reloj por apertura: si una se cuelga, el test falla en vez de colgarse);
// cuando devuelve un archivo, su descriptor es el de un archivo regular y se
// lee el contenido regular entero, nunca el FIFO ni el destino del symlink;
// cuando falla, no es fs.ErrNotExist (la ruta siempre existe) y un
// *NoRegular nunca dice regular. Al final nadie quedo leyendo el FIFO.
func TestCA442_AbrirRegularDecideSobreElDescriptorAunqueLaEntradaCambie(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "20261001T060606_ca2e2a.json")
	regular := []byte("{\"severity\":\"high\",\"relleno\":\"" + string(bytes.Repeat([]byte("x"), 4096)) + "\"}\n")
	if err := os.WriteFile(p, regular, 0o644); err != nil {
		t.Fatal(err)
	}
	fifo := filepath.Join(t.TempDir(), "fifo")
	carreratest.Fifo(t, fifo)

	detener := carreratest.RegularYFifo(t, p, fifo, regular)

	vistas := map[string]int{}
	fin := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(fin) {
		ch := make(chan arApertura, 1)
		go func() {
			f, err := AbrirRegular(p)
			ch <- arApertura{f, err}
		}()
		var a arApertura
		select {
		case a = <-ch:
		case <-time.After(arRelojPorApertura):
			detener()
			t.Fatalf("CA-442: AbrirRegular no volvio en %s mientras la entrada cambiaba entre un archivo regular y un FIFO", arRelojPorApertura)
		}
		if a.err != nil {
			if a.f != nil {
				a.f.Close()
				detener()
				t.Fatalf("CA-442: con error AbrirRegular no devuelve archivo: %v", a.err)
			}
			if errors.Is(a.err, fs.ErrNotExist) {
				detener()
				t.Fatalf("CA-442: la ruta existe siempre (rename atomico): no es fs.ErrNotExist: %v", a.err)
			}
			var nr *NoRegular
			if errors.As(a.err, &nr) {
				if nr.Mode.IsRegular() {
					detener()
					t.Fatalf("CA-442: un *NoRegular nunca dice regular: %s", nr.Mode)
				}
				vistas["no regular "+nr.Mode.Type().String()]++
			} else {
				vistas["error de la apertura"]++
			}
			continue
		}
		fi, err := a.f.Stat()
		if err != nil || !fi.Mode().IsRegular() {
			a.f.Close()
			detener()
			t.Fatalf("CA-442: lo que AbrirRegular devuelve es un archivo regular en su descriptor: %v %v", fi, err)
		}
		got, err := io.ReadAll(a.f)
		a.f.Close()
		if err != nil || !bytes.Equal(got, regular) {
			detener()
			t.Fatalf("CA-442: lo que AbrirRegular devuelve se lee entero y es el archivo regular (%d bytes, leyo %d): %v", len(regular), len(got), err)
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
