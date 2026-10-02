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
//     ErrSinDescriptores dice que hoom los necesita. Un sistema SIN ninguno
//     de los dos no se puede simular desde un test sin una costura (el
//     paquete no expone donde los busca): aca se mide el sistema que hay.
//
// El FIFO es el de internal/carreratest.
package hoomfs

import (
	"errors"
	"os"
	"path/filepath"
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

// CA-445 (hallazgo 71d0bd): en un sistema con /dev/fd o /proc/self/fd
// (darwin, linux con procfs) DescriptoresDisponibles es nil; sin ninguno de
// los dos es ErrSinDescriptores. Mira una vez: preguntar de nuevo dice lo
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
	_, errDev := os.Stat("/dev/fd")
	_, errProc := os.Stat("/proc/self/fd")
	hay := errDev == nil || errProc == nil
	err := DescriptoresDisponibles()
	switch {
	case hay && err != nil:
		t.Fatalf("CA-445: este sistema tiene /dev/fd (%v) o /proc/self/fd (%v): DescriptoresDisponibles es nil, no %v", errDev, errProc, err)
	case !hay && !errors.Is(err, ErrSinDescriptores):
		t.Fatalf("CA-445: este sistema no tiene /dev/fd ni /proc/self/fd: DescriptoresDisponibles es ErrSinDescriptores, no %v", err)
	}
	for i := 0; i < 3; i++ {
		if otra := DescriptoresDisponibles(); !errors.Is(otra, err) && otra != err {
			t.Fatalf("CA-445: DescriptoresDisponibles mira una vez: la vez %d dijo %v y antes %v", i+2, otra, err)
		}
	}
	t.Logf("CA-445: /dev/fd: %v, /proc/self/fd: %v, DescriptoresDisponibles: %v", errDev == nil, errProc == nil, err)
}
