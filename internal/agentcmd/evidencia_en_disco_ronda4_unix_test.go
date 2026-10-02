//go:build !windows

// Los casos con FIFO de la ENMIENDA 2 de .hoom/specs/evidencia-en-disco.md
// (CA-445, hallazgo 8aed7c): una raiz de evidencia (.hoom, .hoom/findings,
// .hoom/verdicts, .hoom/approvals) que es un FIFO que nadie escribe —ya asi
// en las dos fotos, o puesta asi por la corrida— no cuelga Take ni el gate,
// no queda abierta para leer, deja ilegible su evidencia en esa foto, y el
// gate del writer y del refutador corta con "la evidencia no se puede leer"
// y la ruta de la raiz.
//
// Por que no hay un test de Take para el alias al mismo inodo de los
// hallazgos bd4403/402186 (un nombre movido adentro del root y cambiado por
// un symlink al objeto movido, ENTRE la mirada y la apertura): el contenido
// es el mismo (es el mismo inodo), asi que una foto que siguio el symlink da
// la misma huella que una que abrio el nombre antes del cambio, y una que lo
// vio despues de abrirlo es un proceso que escribe despues de la foto
// (no-goal); desde afuera de Take no hay como poner el cambio justo entre su
// Lstat y su apertura, ni como distinguir en el resultado si la foto siguio
// el symlink. Lo determinista es el contrato de hoomfs.AbrirRegularEn y
// hoomfs.AbrirDirEn (abrir_en_mismo_inodo_test.go), que es como recorre la
// foto; y en reposo, el mismo directorio movido y enlazado en lugar de una
// raiz (evidencia_en_disco_ronda4_test.go).
//
// El FIFO es el de internal/carreratest.
package agentcmd

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// edr4NadieLee exige que nadie haya dejado el FIFO p abierto para leer:
// abrirlo para escribir sin bloquear falla con ENXIO.
func edr4NadieLee(t *testing.T, ca, caso, p string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err == nil {
		f.Close()
		t.Fatalf("%s: %s: la foto o el gate dejaron abierto para leer el FIFO %s", ca, caso, p)
	}
	if !errors.Is(err, syscall.ENXIO) {
		t.Fatalf("%s: %s: fixture: %s sigue siendo un FIFO que nadie lee: %v", ca, caso, p, err)
	}
}

// edr4FormaFifo deja la raiz como un FIFO que nadie escribe (la evidencia de
// verdad, afuera del repo), y al final exige que nadie lo este leyendo.
func edr4FormaFifo() edr4Forma {
	return edr4Forma{
		caso: "es un FIFO que nadie escribe",
		armar: func(t *testing.T, root, raiz string) {
			edr4Apartar(t, root, raiz)
			carreratest.Fifo(t, filepath.Join(root, filepath.FromSlash(raiz)))
		},
		final: func(t *testing.T, root, raiz string) {
			edr4NadieLee(t, "CA-445", raiz+" es un FIFO", filepath.Join(root, filepath.FromSlash(raiz)))
		},
	}
}

// CA-445 (hallazgo 8aed7c): .hoom/findings, .hoom/verdicts o
// .hoom/approvals es un FIFO que nadie escribe, en reposo en las dos fotos o
// puesto por la corrida: Take y el gate vuelven (con reloj), la foto deja
// esa raiz HuellaIlegible sin nada de adentro y las otras intactas, el gate
// del writer y del refutador corta con "la evidencia no se puede leer" y la
// ruta de la raiz (en reposo, la unica violacion), y nadie queda leyendo el
// FIFO.
func TestCA445_UnaRaizDeEvidenciaQueEsUnFIFONoCuelgaLaFotoYCorta(t *testing.T) {
	qcLimpiarEntorno(t)
	edr4RaizQueNoEsUnDirectorio(t, []edr4Forma{edr4FormaFifo()})
}

// CA-445 (hallazgo 8aed7c): .hoom es un FIFO que nadie escribe, en reposo en
// las dos fotos o puesto por la corrida: Take y el gate vuelven (con reloj),
// la foto deja .hoom/verdicts, .hoom/findings y .hoom/approvals cada uno
// HuellaIlegible sin nada de adentro, el gate del writer y del refutador
// corta con "la evidencia no se puede leer" en cada una de las tres (en
// reposo, las unicas violaciones), y nadie queda leyendo el FIFO.
func TestCA445_UnPuntoHoomQueEsUnFIFONoCuelgaLaFotoYCorta(t *testing.T) {
	qcLimpiarEntorno(t)
	plantilla, _ := edr3PlantillaConEvidencia(t)
	f := edr4FormaFifo()
	for _, cuando := range []string{edr4EnReposo, edr4EnLaCorrida} {
		caso := ".hoom " + f.caso + ", " + cuando
		t.Run(caso, func(t *testing.T) {
			t.Parallel()
			root := edCopiarArbol(t, plantilla)
			reposo := cuando == edr4EnReposo
			if reposo {
				f.armar(t, root, ".hoom")
			}
			before := edr1Take(t, "CA-445", caso+", foto de antes", root)
			if reposo {
				edr3TresIlegibles(t, "CA-445", caso+", foto de antes", before)
			} else {
				f.armar(t, root, ".hoom")
			}
			after := edr1Take(t, "CA-445", caso+", foto de despues", root)
			edr3TresIlegibles(t, "CA-445", caso+", foto de despues", after)
			for _, rol := range edr3Roles(t) {
				cr := caso + ", rol " + rol.Slug
				res := edr1Gate(t, "CA-445", cr, root, rol, before, after, PolicyFor(nil, rol))
				for _, d := range edr3Raices {
					edr4ExigirIlegibleEnLaRaiz(t, "CA-445", cr, res, d)
				}
				if reposo && len(res.Violations) != len(edr3Raices) {
					t.Fatalf("CA-445: %s: en reposo la corrida no hizo nada mas: las unicas violaciones son las de las tres raices: %s", cr, edLista(res.Violations))
				}
			}
			f.final(t, root, ".hoom")
		})
	}
}
