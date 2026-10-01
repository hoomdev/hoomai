// Tests adversariales de la tercera ronda de review de
// .hoom/specs/evidencia-en-disco.md, en reposo (las carreras estan en
// evidencia_en_disco_ronda3_unix_test.go):
//
//   - CA-442/CA-444, hallazgos 545ddc y a081cb: la foto nunca sigue un
//     symlink, tampoco en .hoom ni en las raices de la evidencia. Un .hoom
//     que existe y no es un directorio (un symlink a una copia byte a byte,
//     al directorio original movido, a uno vacio, colgado; un archivo
//     regular) deja .hoom/verdicts, .hoom/findings y .hoom/approvals cada uno
//     HuellaIlegible, sin nada de lo de adentro, y el gate de cualquier rol
//     corta con "la evidencia no se puede leer" en cada uno, en la foto de
//     despues o en las dos. Sin .hoom, no hay entradas y el gate pasa.
//   - CA-441, hallazgo 545ddc: una raiz de evidencia (.hoom/findings,
//     .hoom/verdicts, .hoom/approvals) cambiada por un symlink al MISMO
//     directorio movido (adentro de .hoom o afuera del repo) no se sigue: la
//     foto no trae lo de adentro y el gate corta como manipulacion.
//
// Usa los helpers ed*/edr1* de evidencia_en_disco_test.go y
// evidencia_en_disco_ronda1_test.go.
package agentcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
)

// edr3Raices son las tres raices de la evidencia.
var edr3Raices = []string{".hoom/verdicts", ".hoom/findings", ".hoom/approvals"}

// edr3Debajo dice si rel esta estrictamente debajo de dir.
func edr3Debajo(rel, dir string) bool { return strings.HasPrefix(rel, dir+"/") }

// edr3Claves es la union de las claves de las tres partes de la foto que
// nombran rutas de la evidencia.
func edr3Claves(s Snapshot) map[string]bool {
	out := map[string]bool{}
	for k := range s.Huellas {
		out[k] = true
	}
	for k := range s.Evidence {
		out[k] = true
	}
	for k := range s.Directorios {
		out[k] = true
	}
	return out
}

// edr3Symlink crea en p un symlink a destino; sin permiso para crearlo
// (windows sin privilegios) el test se omite.
func edr3Symlink(t *testing.T, destino, p string) {
	t.Helper()
	if err := os.Symlink(destino, p); err != nil {
		t.Skipf("CA-442: no se pudo crear el symlink %s -> %s: %v", p, destino, err)
	}
}

// edr3Mover mueve root/rel a destino (en el mismo disco: rename).
func edr3Mover(t *testing.T, root, rel, destino string) {
	t.Helper()
	if err := os.Rename(filepath.Join(root, filepath.FromSlash(rel)), destino); err != nil {
		t.Fatalf("fixture: mover %s: %v", rel, err)
	}
}

// edr3PlantillaConEvidencia arma un repo con un hallazgo high, un veredicto
// y una aprobacion, y con .hoom y hoom-copia escondidos de git (el piso en el
// disco es lo unico que mira). Devuelve la plantilla y las tres rutas.
func edr3PlantillaConEvidencia(t *testing.T) (string, []string) {
	t.Helper()
	root := repo(t)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom\nhoom-copia")
	_, hallazgo := edHallazgo(t, root, "high", "el retry no respeta el backoff")
	veredicto := edVeredicto(t, root, "un veredicto")
	aprobacion := edAprobacion(t, root)
	for _, rel := range []string{hallazgo, veredicto, aprobacion} {
		if !qcr1Ignorado(t, root, rel) {
			t.Fatalf("fixture: git ignora %s", rel)
		}
	}
	return root, []string{hallazgo, veredicto, aprobacion}
}

// edr3TresIlegibles exige que la foto tenga exactamente las tres raices,
// cada una HuellaIlegible, y nada de lo de adentro (ni en Huellas, ni en
// Evidence, ni en Directorios).
func edr3TresIlegibles(t *testing.T, ca, caso string, s Snapshot) {
	t.Helper()
	if len(s.Huellas) != len(edr3Raices) {
		t.Fatalf("%s: %s: con un .hoom que no es un directorio, Huellas son las tres raices, cada una %q: %#v", ca, caso, HuellaIlegible, s.Huellas)
	}
	for _, d := range edr3Raices {
		if h, ok := s.Huellas[d]; !ok || h != HuellaIlegible {
			t.Fatalf("%s: %s: con un .hoom que no es un directorio, %s es %q en Huellas: %q (%#v)", ca, caso, d, HuellaIlegible, h, s.Huellas)
		}
	}
	for k := range edr3Claves(s) {
		for _, d := range edr3Raices {
			if edr3Debajo(k, d) {
				t.Fatalf("%s: %s: la foto no sigue .hoom: trae %s, de adentro de %s", ca, caso, k, d)
			}
		}
	}
}

// CA-442/CA-444 (hallazgos 545ddc, a081cb): un .hoom que existe y no es un
// directorio no se sigue ni se lee. Para cada forma, en la foto de despues
// (el rol lo cambio) o en las dos (ya estaba asi), Take deja .hoom/verdicts,
// .hoom/findings y .hoom/approvals cada uno HuellaIlegible, sin nada de lo
// de adentro aunque del otro lado este la MISMA evidencia byte a byte; y el
// gate del writer y del refutador corta con "la evidencia no se puede leer"
// en cada una de las tres raices. Las formas: un symlink absoluto a una
// copia byte a byte afuera del repo, uno relativo al directorio original
// movido adentro del repo, uno a un directorio vacio, uno colgado, y un
// archivo regular. (Cada caso corre en paralelo, en su copia del repo.)
func TestCA442_UnPuntoHoomQueNoEsUnDirectorioDejaLaEvidenciaIlegible(t *testing.T) {
	qcLimpiarEntorno(t)
	plantilla, _ := edr3PlantillaConEvidencia(t)
	formas := []struct {
		caso  string
		armar func(t *testing.T, root string) // deja .hoom en esa forma
	}{
		{"symlink absoluto a una copia byte a byte afuera", func(t *testing.T, root string) {
			copia := edCopiarArbol(t, filepath.Join(root, ".hoom"))
			edr3Mover(t, root, ".hoom", filepath.Join(t.TempDir(), "hoom-real"))
			edr3Symlink(t, copia, filepath.Join(root, ".hoom"))
		}},
		{"symlink relativo al directorio original movido adentro del repo", func(t *testing.T, root string) {
			edr3Mover(t, root, ".hoom", filepath.Join(root, "hoom-copia"))
			edr3Symlink(t, "hoom-copia", filepath.Join(root, ".hoom"))
		}},
		{"symlink a un directorio vacio", func(t *testing.T, root string) {
			edr3Mover(t, root, ".hoom", filepath.Join(t.TempDir(), "hoom-real"))
			edr3Symlink(t, t.TempDir(), filepath.Join(root, ".hoom"))
		}},
		{"symlink colgado", func(t *testing.T, root string) {
			edr3Mover(t, root, ".hoom", filepath.Join(t.TempDir(), "hoom-real"))
			edr3Symlink(t, filepath.Join(t.TempDir(), "no-existe"), filepath.Join(root, ".hoom"))
		}},
		{"archivo regular", func(t *testing.T, root string) {
			edr3Mover(t, root, ".hoom", filepath.Join(t.TempDir(), "hoom-real"))
			write(t, root, ".hoom", "no soy un directorio\n")
		}},
	}
	for _, f := range formas {
		for _, cuando := range []string{"despues", "las dos"} {
			caso := ".hoom " + f.caso + ", en la foto de " + cuando
			t.Run(caso, func(t *testing.T) {
				t.Parallel()
				root := edCopiarArbol(t, plantilla)
				if cuando == "las dos" {
					f.armar(t, root)
				}
				before := edr1Take(t, "CA-442", caso+", foto de antes", root)
				if cuando == "las dos" {
					edr3TresIlegibles(t, "CA-444", caso+", foto de antes", before)
				} else {
					f.armar(t, root)
				}
				after := edr1Take(t, "CA-442", caso+", foto de despues", root)
				edr3TresIlegibles(t, "CA-442", caso+", foto de despues", after)
				for _, slug := range []string{"writer", finding.RolQueRefuta} {
					rol := edr1Rol(t, slug)
					res := edr1Gate(t, "CA-444", caso+", rol "+slug, root, rol, before, after, PolicyFor(nil, rol))
					for _, d := range edr3Raices {
						edr1ExigirIlegible(t, "CA-444", caso+", rol "+slug, res, d)
					}
				}
			})
		}
	}
}

// CA-440/CA-442 (control de 545ddc): sin .hoom no hay entradas: Huellas es
// un mapa vacio (no nil), Directorios no tiene nada, y el gate del writer y
// del refutador sobre dos fotos sin .hoom pasa sin violaciones. (La regla de
// arriba es para un .hoom que existe y no es un directorio, no para uno que
// falta.)
func TestCA442_SinPuntoHoomNoHayEntradasYElGatePasa(t *testing.T) {
	qcLimpiarEntorno(t)
	root := repo(t)
	if _, err := os.Lstat(filepath.Join(root, ".hoom")); !os.IsNotExist(err) {
		t.Fatalf("fixture: el repo no tiene .hoom: %v", err)
	}
	before := edr1Take(t, "CA-440", "sin .hoom, antes", root)
	after := edr1Take(t, "CA-440", "sin .hoom, despues", root)
	for _, s := range []Snapshot{before, after} {
		if s.Huellas == nil || len(s.Huellas) != 0 || len(s.Directorios) != 0 || len(s.Evidence) != 0 {
			t.Fatalf("CA-440: sin .hoom no hay entradas (Huellas vacio, no nil): Huellas=%#v Directorios=%v Evidence=%v", s.Huellas, s.Directorios, s.Evidence)
		}
	}
	for _, slug := range []string{"writer", finding.RolQueRefuta} {
		rol := edr1Rol(t, slug)
		res := edr1Gate(t, "CA-442", "sin .hoom, rol "+slug, root, rol, before, after, PolicyFor(nil, rol))
		if !res.OK || res.Tampering || len(res.Violations) != 0 {
			t.Fatalf("CA-442: sin .hoom en las dos fotos el gate del rol %s pasa: %s", slug, edLista(res.Violations))
		}
	}
}

// CA-441/CA-442 (hallazgo 545ddc): una raiz de evidencia cambiada por un
// symlink al MISMO directorio movido —relativo, adentro de .hoom, o
// absoluto, afuera del repo—, con la misma evidencia byte a byte del otro
// lado, no se sigue: la foto de despues no trae nada de adentro de esa raiz
// ni ninguna de sus huellas, y para el writer y el refutador la evidencia
// que estaba ahi ya no esta: el gate corta como manipulacion (no queda OK).
// (Cada caso corre en paralelo, en su copia del repo.)
func TestCA441_UnaRaizDeEvidenciaCambiadaPorUnSymlinkNoSeSigue(t *testing.T) {
	qcLimpiarEntorno(t)
	plantilla, rutas := edr3PlantillaConEvidencia(t)
	for _, raiz := range edr3Raices {
		adentro := map[string]string{".hoom/verdicts": rutas[1], ".hoom/findings": rutas[0], ".hoom/approvals": rutas[2]}[raiz]
		for _, donde := range []string{"adentro de .hoom, relativo", "afuera del repo, absoluto"} {
			caso := raiz + " cambiada por un symlink al mismo directorio movido " + donde
			t.Run(caso, func(t *testing.T) {
				t.Parallel()
				root := edCopiarArbol(t, plantilla)
				huella := edSHA(edLeer(t, root, adentro))
				before := edr1Take(t, "CA-441", caso+", foto de antes", root)
				if !strings.EqualFold(before.Huellas[adentro], huella) {
					t.Fatalf("CA-441: fixture: la foto de antes trae %s: %q", adentro, before.Huellas[adentro])
				}
				base := filepath.Base(raiz)
				if donde == "adentro de .hoom, relativo" {
					edr3Mover(t, root, raiz, filepath.Join(root, ".hoom", base+"-movido"))
					edr3Symlink(t, base+"-movido", filepath.Join(root, filepath.FromSlash(raiz)))
				} else {
					movido := filepath.Join(t.TempDir(), base)
					edr3Mover(t, root, raiz, movido)
					edr3Symlink(t, movido, filepath.Join(root, filepath.FromSlash(raiz)))
				}
				after := edr1Take(t, "CA-441", caso+", foto de despues", root)
				for k := range edr3Claves(after) {
					if edr3Debajo(k, raiz) {
						t.Fatalf("CA-442: %s: la foto no sigue el symlink %s: trae %s", caso, raiz, k)
					}
				}
				for k, h := range after.Huellas {
					if strings.EqualFold(h, huella) {
						t.Fatalf("CA-442: %s: la foto no sigue el symlink %s: %s tiene la huella de %s", caso, raiz, k, adentro)
					}
				}
				for _, slug := range []string{"writer", finding.RolQueRefuta} {
					rol := edr1Rol(t, slug)
					res := edr1Gate(t, "CA-441", caso+", rol "+slug, root, rol, before, after, PolicyFor(nil, rol))
					if res.OK || !res.Tampering {
						t.Fatalf("CA-441: %s, rol %s: la evidencia que estaba en %s ya no esta (su raiz es un symlink): el gate corta como manipulacion: ok=%v tampering=%v %s",
							caso, slug, raiz, res.OK, res.Tampering, edLista(res.Violations))
					}
				}
			})
		}
	}
}

// edr3Roles es el writer y el refutador.
func edr3Roles(t *testing.T) []agents.Role {
	t.Helper()
	return []agents.Role{edr1Rol(t, "writer"), edr1Rol(t, finding.RolQueRefuta)}
}
