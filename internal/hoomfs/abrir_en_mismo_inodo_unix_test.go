//go:build !windows

// Los casos de unix de la enmienda 2 de .hoom/specs/evidencia-en-disco.md
// (CA-445) para hoomfs:
//
//   - hallazgos bd4403 y 402186: un FIFO mirado (Lstat) y despues movido
//     adentro del root, con un symlink en su nombre al MISMO FIFO, no cuelga
//     AbrirRegularEn ni AbrirDirEn y no se sigue: vuelven enseguida con
//     error y nadie queda leyendo el FIFO.
//   - hallazgo 71d0bd: DescriptoresDisponibles dice si el sistema deja
//     reabrir un directorio por su descriptor (/dev/fd o /proc/self/fd), y
//     ErrSinDescriptores dice que hoom los necesita. Aca se mide el sistema
//     que hay, decidiendo si "hay" reabriendo DE VERDAD un directorio por su
//     descriptor (hallazgos 669a42, c32559: que /dev/fd exista no es que
//     sirva). Un sistema sin ninguna que sirva se simula con la costura
//     fdDirs en descriptores_unix_test.go.
//
// El FIFO es el de internal/carreratest.
package hoomfs

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// CA-445/CA-442 (hallazgos bd4403, 402186): un FIFO que nadie escribe,
// mirado con su Lstat y despues movido al lado adentro del root, con un
// symlink relativo en su nombre al MISMO FIFO movido: AbrirRegularEn y
// AbrirDirEn vuelven enseguida (con reloj) con error, sin archivo ni root,
// y el FIFO no queda abierto para leer.
func TestCA445_UnSymlinkAlMismoFIFOMovidoNoCuelgaNiSeSigue(t *testing.T) {
	for _, comoDir := range []bool{false, true} {
		caso := "AbrirRegularEn"
		if comoDir {
			caso = "AbrirDirEn"
		}
		t.Run(caso, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			const nombre = "20261001T020202_f1f0f1.json"
			carreratest.Fifo(t, filepath.Join(dir, nombre))
			r := aeRoot(t, dir)
			antes := aeLstat(t, r, nombre)
			if antes.Mode()&os.ModeNamedPipe == 0 {
				t.Fatalf("CA-445: fixture: %s es un FIFO: %s", nombre, antes.Mode())
			}
			movido := filepath.Join(dir, "aux-tubo")
			if err := os.Rename(filepath.Join(dir, nombre), movido); err != nil {
				t.Fatalf("fixture: mover el FIFO: %v", err)
			}
			carreratest.Soltar(t, movido)
			arSymlink(t, "aux-tubo", filepath.Join(dir, nombre))
			if fi, err := os.Stat(filepath.Join(dir, nombre)); err != nil || !os.SameFile(fi, antes) {
				t.Fatalf("CA-445: fixture: el symlink lleva al mismo FIFO que su Lstat: %v", err)
			}
			if comoDir {
				d, err := aeAbrirDir(t, caso, r, nombre, antes)
				aeRechazadoDir(t, "CA-445: symlink al mismo FIFO movido", d, err)
			} else {
				f, err := aeAbrirRegular(t, caso, r, nombre, antes)
				aeRechazadoRegular(t, "CA-445: symlink al mismo FIFO movido", f, err, nil)
			}
			arNadieLee(t, "CA-445: "+caso+", symlink al mismo FIFO movido", movido)
		})
	}
}

// CA-445 (hallazgo 71d0bd; hallazgos 669a42, c32559): en un sistema que
// reabre DE VERDAD un directorio abierto por su descriptor por /dev/fd o
// /proc/self/fd (darwin, linux con procfs) DescriptoresDisponibles es nil;
// si ninguna de las dos sirve (no existe, o existe y no deja: sandbox-exec
// negando /dev/fd/N) es ErrSinDescriptores. Que exista no decide: decide
// reabrir. Las carpetas por defecto son esas dos, en ese orden; sondear (la
// sonda sin cache) dice lo mismo. Mira una vez: preguntar de nuevo dice lo
// mismo. Y el texto de ErrSinDescriptores dice que hoom necesita "/dev/fd o
// /proc" (es el diagnostico que el detalle de la evidencia ilegible lleva en
// un sistema asi).
func TestCA445_DescriptoresDisponiblesEnEsteSistemaYElDiagnostico(t *testing.T) {
	if ErrSinDescriptores == nil {
		t.Fatal("CA-445: ErrSinDescriptores existe")
	}
	if msg := ErrSinDescriptores.Error(); !strings.Contains(msg, "/dev/fd o /proc") || !strings.Contains(msg, "necesita") {
		t.Fatalf("CA-445: ErrSinDescriptores dice que hoom necesita \"/dev/fd o /proc\": %q", msg)
	}
	if !reflect.DeepEqual(fdDirs, fdPorDefecto) {
		t.Fatalf("CA-445: las carpetas de descriptores por defecto son %q, en ese orden, no %q", fdPorDefecto, fdDirs)
	}
	_, errDev := os.Stat("/dev/fd")
	_, errProc := os.Stat("/proc/self/fd")
	anda, probado := fdQueAnda(t)
	hay := anda != ""
	err := DescriptoresDisponibles()
	switch {
	case hay && err != nil:
		t.Fatalf("CA-445: este sistema reabre un directorio por su descriptor por %s (%v): DescriptoresDisponibles es nil, no %v", anda, probado, err)
	case !hay && !errors.Is(err, ErrSinDescriptores):
		t.Fatalf("CA-445: este sistema no reabre un directorio por su descriptor ni por /dev/fd ni por /proc/self/fd (%v; existen: /dev/fd %v, /proc/self/fd %v): DescriptoresDisponibles es ErrSinDescriptores, no %v", probado, errDev == nil, errProc == nil, err)
	case !hay && (!strings.Contains(err.Error(), "/proc/self/fd") || !strings.Contains(err.Error(), "; ") || strings.ContainsAny(err.Error(), "\n\r")):
		t.Fatalf("CA-445: sin /dev/fd ni /proc que sirvan, ErrSinDescriptores trae la causa de cada carpeta (/dev/fd y /proc/self/fd) separadas por \"; \", en una linea: %q", err.Error())
	}
	s := sondear()
	if (s == nil) != hay || (!hay && !errors.Is(s, ErrSinDescriptores)) {
		t.Fatalf("CA-445: sondear dice lo mismo que reabrir de verdad (%s, %v): es %v", anda, probado, s)
	}
	for i := 0; i < 3; i++ {
		if otra := DescriptoresDisponibles(); !errors.Is(otra, err) && otra != err {
			t.Fatalf("CA-445: DescriptoresDisponibles mira una vez: la vez %d dijo %v y antes %v", i+2, otra, err)
		}
	}
	t.Logf("CA-445: existen /dev/fd: %v, /proc/self/fd: %v; reabre por: %q (%v); DescriptoresDisponibles: %v", errDev == nil, errProc == nil, anda, probado, err)
}
