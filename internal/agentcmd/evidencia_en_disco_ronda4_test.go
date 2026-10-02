// Tests adversariales de la ENMIENDA 2 de .hoom/specs/evidencia-en-disco.md
// (CA-445) y de la cuarta ronda de su review, en reposo (los casos con FIFO
// estan en evidencia_en_disco_ronda4_unix_test.go):
//
//   - CA-445, hallazgo 8aed7c: una raiz de evidencia (.hoom/findings,
//     .hoom/verdicts, .hoom/approvals) que existe y no es un directorio —un
//     symlink, aunque lleve a una copia byte a byte o al MISMO directorio
//     movido, adentro de .hoom o afuera del repo; un archivo regular— deja
//     ilegible su evidencia en esa foto: Huellas[raiz] es HuellaIlegible y
//     no hay nada de lo de adentro, la evidencia de las otras raices se
//     fotografia igual, y el gate del writer y del refutador corta con "la
//     evidencia no se puede leer" y la ruta de la raiz. Vale cuando la raiz
//     ya esta asi en las DOS fotos (en reposo: el caso del hallazgo, en que
//     el piso quedaba ciego) y cuando la corrida la cambia. Y un rol que,
//     con .hoom/findings -> .hoom/aux en reposo, baja un hallazgo high a low
//     por .hoom/aux no pasa el gate, para ningun rol.
//   - CA-445, hallazgo 71d0bd: en este sistema (con /dev/fd o /proc) el
//     detalle de esa evidencia ilegible no trae el diagnostico de
//     hoomfs.ErrSinDescriptores; en uno sin ellos lo traeria.
//
// Usa los helpers ed*/edr1*/edr3* de los otros tests de la spec. Cada caso
// corre en paralelo, en su copia de la plantilla.
package agentcmd

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// edr4Forma deja la raiz de evidencia raiz de root en una forma que no es un
// directorio. final, si esta, corre al terminar el caso (con la raiz todavia
// en esa forma).
type edr4Forma struct {
	caso  string
	armar func(t *testing.T, root, raiz string)
	final func(t *testing.T, root, raiz string)
}

// edr4Apartar saca el directorio raiz de root afuera del repo (la evidencia
// de verdad deja de estar donde la foto la busca).
func edr4Apartar(t *testing.T, root, raiz string) {
	t.Helper()
	edr3Mover(t, root, raiz, filepath.Join(t.TempDir(), filepath.Base(raiz)))
}

// edr4Formas son las formas de unix y windows: symlinks (se omiten sin
// permiso para crearlos) y un archivo regular.
func edr4Formas() []edr4Forma {
	return []edr4Forma{
		{caso: "es un symlink relativo a una copia byte a byte adentro de .hoom", armar: func(t *testing.T, root, raiz string) {
			base := filepath.Base(raiz)
			copia := edCopiarArbol(t, filepath.Join(root, filepath.FromSlash(raiz)))
			edr3Mover(t, filepath.Dir(copia), filepath.Base(copia), filepath.Join(root, ".hoom", base+"-copia"))
			edr4Apartar(t, root, raiz)
			edr3Symlink(t, base+"-copia", filepath.Join(root, filepath.FromSlash(raiz)))
		}},
		{caso: "es un symlink relativo al mismo directorio movido adentro de .hoom", armar: func(t *testing.T, root, raiz string) {
			base := filepath.Base(raiz)
			edr3Mover(t, root, raiz, filepath.Join(root, ".hoom", base+"-movido"))
			edr3Symlink(t, base+"-movido", filepath.Join(root, filepath.FromSlash(raiz)))
		}},
		{caso: "es un symlink absoluto a una copia byte a byte afuera del repo", armar: func(t *testing.T, root, raiz string) {
			copia := edCopiarArbol(t, filepath.Join(root, filepath.FromSlash(raiz)))
			edr4Apartar(t, root, raiz)
			edr3Symlink(t, copia, filepath.Join(root, filepath.FromSlash(raiz)))
		}},
		{caso: "es un archivo regular", armar: func(t *testing.T, root, raiz string) {
			edr4Apartar(t, root, raiz)
			write(t, root, raiz, "no soy un directorio\n")
		}},
	}
}

// Cuando la raiz no es un directorio.
const (
	edr4EnReposo    = "en reposo, en las dos fotos"
	edr4EnLaCorrida = "la corrida la cambia, en la foto de despues"
)

// edr4RaizIlegible exige que la foto s tenga la raiz como HuellaIlegible, sin
// nada de lo de adentro (ni en Huellas, ni en Evidence, ni en Directorios) y
// sin la huella de la evidencia que vivia ahi en ninguna ruta (la copia de
// afuera es byte a byte: seguirla la traeria); y que la evidencia de las
// otras dos raices este, con el sha256 de lo que hay en el disco.
func edr4RaizIlegible(t *testing.T, ca, caso, root string, s Snapshot, raiz string, ev edr3Evidencia, huellaDeAdentro string) {
	t.Helper()
	if h, ok := s.Huellas[raiz]; !ok || h != HuellaIlegible {
		t.Fatalf("%s: %s: una raiz que no es un directorio deja ilegible su evidencia: Huellas[%s] es %q, no %q (ok=%v): %#v", ca, caso, raiz, h, HuellaIlegible, ok, s.Huellas)
	}
	for k := range edr3Claves(s) {
		if edr3Debajo(k, raiz) {
			t.Fatalf("%s: %s: la foto no recorre %s, que no es un directorio: trae %s", ca, caso, raiz, k)
		}
	}
	for k, h := range s.Huellas {
		if strings.EqualFold(h, huellaDeAdentro) {
			t.Fatalf("%s: %s: la foto no sigue %s: %s tiene la huella de la evidencia que vivia ahi", ca, caso, raiz, k)
		}
	}
	for otra, rel := range ev.PorRaiz() {
		if otra == raiz {
			continue
		}
		if want := edSHA(edLeer(t, root, rel)); !strings.EqualFold(s.Huellas[rel], want) {
			t.Fatalf("%s: %s: la evidencia de %s se fotografia igual (CA-440): Huellas[%s] es %q, no %s", ca, caso, otra, rel, s.Huellas[rel], want)
		}
	}
}

// edr4ExigirIlegibleEnLaRaiz exige que el gate corte (manipulacion, no OK)
// con una violacion EN la raiz cuyo detalle dice "la evidencia no se puede
// leer" y la ruta de la raiz, y que el detalle de cada evidencia ilegible
// diga el diagnostico de los descriptores solo si este sistema no los tiene
// (71d0bd). La devuelve.
func edr4ExigirIlegibleEnLaRaiz(t *testing.T, ca, caso string, res ScopeResult, raiz string) Violation {
	t.Helper()
	if res.OK || !res.Tampering || !res.Cuts() {
		t.Fatalf("%s: %s: con la evidencia de %s ilegible el gate corta: ok=%v tampering=%v corta=%v %s", ca, caso, raiz, res.OK, res.Tampering, res.Cuts(), edLista(res.Violations))
	}
	var la *Violation
	for i, v := range res.Violations {
		if v.Path == raiz && v.Rule == RuleTampering && strings.Contains(v.Detail, edr1Ilegible) && strings.Contains(v.Detail, raiz) {
			la = &res.Violations[i]
		}
	}
	if la == nil {
		t.Fatalf("%s: %s: el gate corta con %q y la ruta de la raiz %s: %s", ca, caso, edr1Ilegible, raiz, edLista(res.Violations))
	}
	edr4Diagnostico(t, ca, caso, res)
	return *la
}

// edr4Diagnostico (hallazgo 71d0bd): con /dev/fd o /proc
// (hoomfs.DescriptoresDisponibles nil) el detalle de una evidencia ilegible
// no trae el diagnostico de hoomfs.ErrSinDescriptores; sin ellos, dice que
// hoom necesita "/dev/fd o /proc".
func edr4Diagnostico(t *testing.T, ca, caso string, res ScopeResult) {
	t.Helper()
	hay := hoomfs.DescriptoresDisponibles() == nil
	for _, v := range res.Violations {
		if !strings.Contains(v.Detail, edr1Ilegible) {
			continue
		}
		diagnostico := strings.Contains(v.Detail, "/dev/fd") || strings.Contains(v.Detail, "/proc") || strings.Contains(v.Detail, hoomfs.ErrSinDescriptores.Error())
		if hay && diagnostico {
			t.Fatalf("CA-445: %s: %s: este sistema tiene /dev/fd o /proc: el detalle de la evidencia ilegible no trae el diagnostico de los descriptores: %q", ca, caso, v.Detail)
		}
		if !hay && !strings.Contains(v.Detail, "/dev/fd o /proc") {
			t.Fatalf("CA-445: %s: %s: sin /dev/fd ni /proc el detalle dice que hoom necesita \"/dev/fd o /proc\": %q", ca, caso, v.Detail)
		}
	}
}

// edr4RaizQueNoEsUnDirectorio corre, para cada forma, cada una de las tres
// raices y cada momento (en reposo en las dos fotos, o puesta por la
// corrida), las dos fotos y el gate del writer y del refutador sobre una
// copia de la plantilla.
func edr4RaizQueNoEsUnDirectorio(t *testing.T, formas []edr4Forma) {
	t.Helper()
	plantilla, ev := edr3PlantillaConEvidencia(t)
	for _, f := range formas {
		for _, raiz := range edr3Raices {
			for _, cuando := range []string{edr4EnReposo, edr4EnLaCorrida} {
				caso := raiz + " " + f.caso + ", " + cuando
				t.Run(caso, func(t *testing.T) {
					t.Parallel()
					root := edCopiarArbol(t, plantilla)
					adentro := ev.PorRaiz()[raiz]
					huella := edSHA(edLeer(t, root, adentro))
					reposo := cuando == edr4EnReposo
					if reposo {
						f.armar(t, root, raiz)
					}
					before := edr1Take(t, "CA-445", caso+", foto de antes", root)
					if reposo {
						edr4RaizIlegible(t, "CA-445", caso+", foto de antes", root, before, raiz, ev, huella)
					} else {
						if !strings.EqualFold(before.Huellas[adentro], huella) {
							t.Fatalf("CA-445: fixture: %s: la foto de antes trae %s: %q", caso, adentro, before.Huellas[adentro])
						}
						f.armar(t, root, raiz)
					}
					after := edr1Take(t, "CA-445", caso+", foto de despues", root)
					edr4RaizIlegible(t, "CA-445", caso+", foto de despues", root, after, raiz, ev, huella)
					for _, rol := range edr3Roles(t) {
						cr := caso + ", rol " + rol.Slug
						res := edr1Gate(t, "CA-445", cr, root, rol, before, after, PolicyFor(nil, rol))
						edr4ExigirIlegibleEnLaRaiz(t, "CA-445", cr, res, raiz)
						if reposo && len(res.Violations) != 1 {
							t.Fatalf("CA-445: %s: en reposo la corrida no hizo nada mas: la unica violacion es la de %s: %s", cr, raiz, edLista(res.Violations))
						}
					}
					if f.final != nil {
						f.final(t, root, raiz)
					}
				})
			}
		}
	}
}

// CA-445 (hallazgo 8aed7c): para .hoom/findings, .hoom/verdicts y
// .hoom/approvals, una raiz que es un symlink relativo a una copia byte a
// byte adentro de .hoom, un symlink relativo al MISMO directorio movido
// adentro de .hoom, un symlink absoluto a una copia afuera del repo, o un
// archivo regular —ya asi en las dos fotos, o puesta asi por la corrida—
// deja su evidencia ilegible en esa foto (HuellaIlegible, nada de adentro,
// las otras raices intactas), y el gate del writer y del refutador corta con
// "la evidencia no se puede leer" y la ruta de la raiz, sin el diagnostico
// de los descriptores en este sistema (71d0bd). En reposo es la unica
// violacion.
func TestCA445_UnaRaizDeEvidenciaQueNoEsUnDirectorioDejaSuEvidenciaIlegible(t *testing.T) {
	qcLimpiarEntorno(t)
	edr4RaizQueNoEsUnDirectorio(t, edr4Formas())
}

// CA-445/CA-441 (hallazgo 8aed7c, el caso del refutador de la ronda 4): con
// .hoom/findings -> .hoom/aux en reposo (aux es una copia byte a byte, o el
// mismo directorio movido), el rol baja un hallazgo high a low por
// .hoom/aux, y por el symlink hoom lee el hallazgo bajado. Las dos fotos
// tienen .hoom/findings ilegible y nada de adentro, y el gate de CADA rol (el
// refutador incluido) corta con "la evidencia no se puede leer" en
// .hoom/findings: el piso no queda ciego. Control: con .hoom/findings un
// directorio y sin cambios, el gate de cada rol pasa.
func TestCA445_UnRolQueBajaUnHallazgoPorElDestinoDeUnaRaizSymlinkNoPasaElGate(t *testing.T) {
	qcLimpiarEntorno(t)
	plantilla, ev := edr3PlantillaConEvidencia(t)
	roles := agents.Roles()
	if len(roles) < 6 {
		t.Fatalf("CA-445: fixture: la tabla trae los roles de hoom (refutador incluido), trajo %d", len(roles))
	}
	const raiz = ".hoom/findings"
	t.Run("control: .hoom/findings es un directorio y nada cambia", func(t *testing.T) {
		t.Parallel()
		root := edCopiarArbol(t, plantilla)
		before := edr1Take(t, "CA-445", "control, antes", root)
		after := edr1Take(t, "CA-445", "control, despues", root)
		for _, rol := range roles {
			if res := edr1Gate(t, "CA-445", "control, rol "+rol.Slug, root, rol, before, after, PolicyFor(nil, rol)); !res.OK || res.Tampering {
				t.Fatalf("CA-445: fixture: sin cambios el gate del rol %s pasa: %s", rol.Slug, edLista(res.Violations))
			}
		}
	})
	for _, copia := range []bool{true, false} {
		caso := raiz + " -> .hoom/aux, el mismo directorio movido"
		if copia {
			caso = raiz + " -> .hoom/aux, una copia byte a byte"
		}
		t.Run(caso, func(t *testing.T) {
			t.Parallel()
			root := edCopiarArbol(t, plantilla)
			alto := edLeer(t, root, ev.Hallazgo)
			aux := filepath.Join(root, ".hoom", "aux")
			if copia {
				c := edCopiarArbol(t, filepath.Join(root, ".hoom", "findings"))
				edr3Mover(t, filepath.Dir(c), filepath.Base(c), aux)
				edr4Apartar(t, root, raiz)
			} else {
				edr3Mover(t, root, raiz, aux)
			}
			edr3Symlink(t, "aux", filepath.Join(root, ".hoom", "findings"))

			before := edr1Take(t, "CA-445", caso+", foto de antes", root)
			edr4RaizIlegible(t, "CA-445", caso+", foto de antes", root, before, raiz, ev, edSHA(alto))
			bajo := edBajarSeveridad(t, alto)
			write(t, root, ".hoom/aux/"+filepath.Base(ev.Hallazgo), string(bajo))
			if got := edLeer(t, root, ev.Hallazgo); string(got) != string(bajo) {
				t.Fatalf("CA-445: fixture: %s: por el symlink, %s es el hallazgo bajado", caso, ev.Hallazgo)
			}
			after := edr1Take(t, "CA-445", caso+", foto de despues", root)
			edr4RaizIlegible(t, "CA-445", caso+", foto de despues", root, after, raiz, ev, edSHA(alto))
			for k, h := range after.Huellas {
				if strings.EqualFold(h, edSHA(bajo)) {
					t.Fatalf("CA-445: %s: la foto no sigue %s: %s tiene la huella del hallazgo bajado", caso, raiz, k)
				}
			}
			for _, rol := range roles {
				cr := caso + ", rol " + rol.Slug
				res := edr1Gate(t, "CA-445", cr, root, rol, before, after, PolicyFor(nil, rol))
				edr4ExigirIlegibleEnLaRaiz(t, "CA-445", cr, res, raiz)
			}
		})
	}
}
