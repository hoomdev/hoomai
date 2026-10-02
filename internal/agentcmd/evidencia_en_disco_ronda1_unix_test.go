//go:build !windows

// Los casos con FIFO de la ENMIENDA 1 de .hoom/specs/evidencia-en-disco.md
// (CA-442: un symlink o un FIFO creado con un nombre valido es manipulacion
// "la evidencia es un archivo regular", y nunca se sigue ni se lee). Estaban
// en evidencia_en_disco_ronda1_test.go; viven aca porque mkfifo es de unix y
// los tests de agentcmd tambien compilan en windows. El FIFO es el de
// internal/carreratest (hallazgo b08c53: una sola copia de la fixture).
package agentcmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/carreratest"
	"github.com/hoomdev/hoomai/internal/finding"
)

// CA-442 ("nunca se sigue ni se lee"): un symlink con nombre valido de
// hallazgo o de veredicto que lleva a un FIFO fuera de .hoom (que nadie
// escribe). Para cada forma de esconder (o no) lo creado: ni la foto ni el
// gate se cuelgan siguiendolo, y para el writer y el refutador es UNA
// manipulacion con "la evidencia es un archivo regular" y su ruta.
func TestCA442_UnSymlinkAUnFIFOConNombreValidoNoCuelgaLaFoto(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			root := repo(t)
			e.armar(t, root)
			fifoAfuera := filepath.Join(t.TempDir(), "fifo")
			carreratest.Fifo(t, fifoAfuera)
			if err := os.MkdirAll(filepath.Join(root, ".hoom", "verdicts"), 0o755); err != nil {
				t.Fatal(err)
			}
			before := edr1Take(t, "CA-442", e.caso+", foto de antes", root)
			enlaces := []string{".hoom/findings/20261001T010102_5e1f01.json", ".hoom/verdicts/2026-10-01T01-01-02Z_5e1f0001.json"}
			for _, l := range enlaces {
				edr1Symlink(t, root, l, fifoAfuera)
				if e.oculto && !qcr1Ignorado(t, root, l) {
					t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, l)
				}
			}
			after := edr1Take(t, "CA-442", e.caso+", foto con symlinks a un FIFO", root)
			for _, slug := range []string{"writer", finding.RolQueRefuta} {
				rol := edr1Rol(t, slug)
				caso := e.caso + ", rol " + slug
				r := edr1Gate(t, "CA-442", caso, root, rol, before, after, PolicyFor(nil, rol))
				for _, l := range enlaces {
					edExigir(t, "CA-442", caso+", symlink a un FIFO", r, l, edr1Regular, l)
				}
			}
		})
	}
}

// CA-442: para cada forma de esconder (o no) lo creado y para CADA rol, el
// refutador incluido: un FIFO creado con un nombre valido de hallazgo, de
// veredicto o de resolucion es UNA manipulacion con "la evidencia es un
// archivo regular" y su ruta. Nunca se lee: ni la foto ni el gate se cuelgan
// en el FIFO (que nadie escribe). (Cada rol corre en paralelo, en su copia
// del repo armado para la variante.)
func TestCA442_UnFIFOConNombreValidoEsManipulacionYNoCuelgaLaFoto(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			e.armar(t, plantilla)
			id, _ := edHallazgo(t, plantilla, "high", "a refutar")
			if err := os.MkdirAll(filepath.Join(plantilla, ".hoom", "verdicts"), 0o755); err != nil {
				t.Fatal(err)
			}
			fifos := []string{".hoom/findings/20261001T020202_f1f0f1.json", ".hoom/verdicts/2026-10-01T02-02-02Z_f1f0f1f0.json"}
			res := qcResRel(id)
			for _, rol := range agents.Roles() {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := edr1Take(t, "CA-442", caso+", foto de antes", root)
					for _, f := range []string{fifos[0], fifos[1], res} {
						carreratest.Fifo(t, filepath.Join(root, f))
						if e.oculto && !qcr1Ignorado(t, root, f) {
							t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, f)
						}
					}
					after := edr1Take(t, "CA-442", caso+", foto con FIFOs", root)

					r := edr1Gate(t, "CA-442", caso, root, rol, before, after, PolicyFor(nil, rol))
					for _, f := range fifos {
						v := edExigir(t, "CA-442", caso+", FIFO", r, f, edr1Regular, f)
						if v.FindingID == "" {
							t.Fatalf("CA-442: %s: la manipulacion en %s registra su hallazgo high del gate", caso, f)
						}
					}
					if rol.Slug == finding.RolQueRefuta {
						edExigir(t, "CA-442", caso+", resolucion FIFO", r, res, edr1Regular, res)
					} else {
						edr1Una(t, "CA-442", caso+", resolucion FIFO", r, res, []string{edr1Regular, res}, []string{qcNoCierra(rol.Slug)})
					}
				})
			}
		})
	}
}
