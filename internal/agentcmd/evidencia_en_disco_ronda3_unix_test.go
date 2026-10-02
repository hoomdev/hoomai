//go:build !windows

// Carreras de los hallazgos 545ddc y a081cb de la tercera ronda de review
// de .hoom/specs/evidencia-en-disco.md (CA-442: un symlink "nunca se sigue
// ni se lee"; CA-441: el piso append-only): un proceso que la corrida dejo
// vivo cambia un DIRECTORIO de la evidencia (un subdirectorio, la raiz
// .hoom/findings, .hoom mismo) por un symlink, y lo devuelve, sin parar,
// mientras se saca la foto. La foto decide sobre el descriptor que abre en
// cada componente: nunca trae una ruta ni una huella de lo que hay del otro
// lado del symlink, nunca toma una copia limpia por la evidencia (el gate
// nunca queda OK con la evidencia bajada en el disco), y cada Take vuelve.
// Las carreras son las de internal/carreratest.
package agentcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/carreratest"
)

// edr3Carrera es cuanto dura cada carrera.
const edr3Carrera = 1500 * time.Millisecond

// edr3Fotografos es cuantas fotos se sacan a la vez durante una carrera: una
// foto pasa casi todo su tiempo en git, y cada una mas es otra chance de que
// la mirada y la apertura de un directorio caigan a los dos lados de un
// cambio.
const edr3Fotografos = 4

// edr3Fotos saca fotos de root mientras dura la carrera, edr3Fotografos a la
// vez, cada una con reloj: si una no vuelve, para la carrera y el test falla
// en vez de colgarse.
func edr3Fotos(t *testing.T, ca, caso, root string, parar func() int) []Snapshot {
	t.Helper()
	var (
		mu      sync.Mutex
		fotos   []Snapshot
		colgada atomic.Bool
		wg      sync.WaitGroup
	)
	fin := time.Now().Add(edr3Carrera)
	for f := 0; f < edr3Fotografos; f++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for time.Now().Before(fin) && !colgada.Load() {
				ch := make(chan Snapshot, 1)
				go func() { ch <- Take(root, "main") }()
				select {
				case s := <-ch:
					mu.Lock()
					fotos = append(fotos, s)
					mu.Unlock()
				case <-time.After(edr2RelojPorFoto):
					colgada.Store(true)
					return
				}
			}
		}()
	}
	wg.Wait()
	if colgada.Load() {
		parar()
		t.Fatalf("%s: %s: Take no volvio en %s mientras un directorio de la evidencia cambiaba por un symlink: siguio el symlink o abrio un FIFO",
			ca, caso, edr2RelojPorFoto)
	}
	return fotos
}

// edr3Archivos devuelve el sha256 de cada archivo regular bajo dir.
func edr3Archivos(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if fi.Mode().IsRegular() {
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			out[p] = edSHA(raw)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("fixture: recorrer %s: %v", dir, err)
	}
	return out
}

// edr3Gates corre, ya parada la carrera, el gate del writer y del refutador
// sobre (before, cada foto distinta) y exige que nunca quede OK sin
// violaciones: la evidencia en el disco esta bajada, y lo que la foto no vio
// (porque faltaba, era un symlink o cambio mientras la abria) desaparecio o
// no se puede leer. Devuelve cuantas fotos distintas hubo.
func edr3Gates(t *testing.T, ca, caso, root string, before Snapshot, fotos []Snapshot) int {
	t.Helper()
	vistas := map[string]bool{}
	for i, s := range fotos {
		clave := fmt.Sprint(s.Huellas, s.Directorios, s.Evidence)
		if vistas[clave] {
			continue
		}
		vistas[clave] = true
		for _, rol := range edr3Roles(t) {
			res := edr1Gate(t, ca, caso, root, rol, before, s, PolicyFor(nil, rol))
			if res.OK || !res.Tampering {
				t.Fatalf("%s: %s, rol %s: con la evidencia bajada en el disco, el gate sobre la foto %d de la carrera nunca queda OK: ok=%v tampering=%v %s (Huellas %v)",
					ca, caso, rol.Slug, i, res.OK, res.Tampering, edLista(res.Violations), s.Huellas)
			}
		}
	}
	return len(vistas)
}

// ================================================================ (a) un subdirectorio

// edr3Subs es cuantos subdirectorios de .hoom/findings cambia la carrera de
// (a) a la vez: cada foto los recorre todos, y cada uno es otra chance.
const edr3Subs = 6

// CA-442 (hallazgos 545ddc, a081cb, propiedad de carrera): mientras otros
// procesos cambian sin parar .hoom/findings/sub0..sub5 —directorios de
// verdad, cada uno con un archivo con nombre de hallazgo— por un symlink y
// los devuelven, CADA Take vuelve (con reloj por foto) y ninguna foto trae
// nada de lo que hay del otro lado: ni una ruta que solo existe ahi (un
// SECRETO.txt, un arbol profundo, uno ancho, un FIFO con nombre de
// hallazgo), ni una huella de un archivo de afuera, ni, bajo un sub, otra
// huella que la de su archivo (o HuellaIlegible); y la huella de un sub
// mismo, si esta, es HuellaIlegible o HuellaNoRegular, nunca un sha256. Dos
// carreras: con symlinks a directorios (afuera del repo, absoluto y
// relativo; adentro, a un hermano con el mismo nombre de hallazgo; a la raiz
// de findings; a .hoom), y con un symlink a un FIFO de findings y el FIFO
// mismo en lugar de cada sub.
func TestCA442_LaFotoNoSigueUnSubdirectorioDeLaEvidenciaQueCambiaPorUnSymlink(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, conFifo := range []bool{false, true} {
		caso := "symlinks a directorios"
		if conFifo {
			caso = "un symlink a un FIFO y el FIFO mismo"
		}
		t.Run(caso, func(t *testing.T) {
			t.Parallel()
			root := repo(t)
			qcr1Anexar(t, qcr1Exclude(t, root), ".hoom")
			const nombre = "20261001T030303_5ab5ab.json"
			var subs []string
			for i := 0; i < edr3Subs; i++ {
				sub := fmt.Sprintf(".hoom/findings/sub%d", i)
				subs = append(subs, sub)
				write(t, root, sub+"/"+nombre, string(edr1HallazgoJSON("20261001T030303_5ab5ab", "high"))+fmt.Sprintf("sub%d\n", i))
			}
			write(t, root, ".hoom/findings/otro/"+nombre, string(edr1HallazgoJSON("20261001T030303_5ab5ab", "low")))
			write(t, root, ".hoom/findings/otro/SOLO-EN-OTRO.txt", "solo en otro\n")
			edHallazgo(t, root, "high", "uno de arriba")
			tubo := filepath.Join(root, ".hoom", "findings", "tubo")
			carreratest.Fifo(t, tubo)

			afuera := t.TempDir()
			write(t, afuera, nombre, string(edr1HallazgoJSON("20261001T030303_5ab5ab", "medium"))+"afuera\n")
			write(t, afuera, "sub/SECRETO.txt", "secreto de afuera\n")
			profundo := "profundo"
			for i := 0; i < 30; i++ {
				profundo += fmt.Sprintf("/n%02d", i)
			}
			write(t, afuera, profundo+"/SECRETO-PROFUNDO.txt", "secreto profundo\n")
			for i := 0; i < 200; i++ {
				write(t, afuera, fmt.Sprintf("ancho/f%03d.txt", i), fmt.Sprintf("secreto ancho %d\n", i))
			}
			deAfuera := edr3Archivos(t, afuera)
			carreratest.Fifo(t, filepath.Join(afuera, "20261001T040404_f1f0f1.json"))

			quieta := edr1Take(t, "CA-442", caso+", foto quieta", root)
			permitidas := edr3Claves(quieta)
			adentro := map[string]string{}
			for _, sub := range subs {
				permitidas[sub] = true
				k := sub + "/" + nombre
				if !strings.EqualFold(quieta.Huellas[k], edSHA(edLeer(t, root, k))) {
					t.Fatalf("CA-442: fixture: la foto quieta trae %s: %q", k, quieta.Huellas[k])
				}
				adentro[k] = strings.ToLower(quieta.Huellas[k])
			}

			rel, err := filepath.Rel(filepath.Join(root, ".hoom", "findings"), afuera)
			if err != nil {
				t.Fatal(err)
			}
			formas := []carreratest.Forma{
				carreratest.Symlink(afuera), carreratest.Symlink(rel), carreratest.Symlink("otro"),
				carreratest.Symlink("."), carreratest.Symlink(".."),
			}
			if conFifo {
				formas = []carreratest.Forma{carreratest.Symlink("tubo"), carreratest.EnlaceDuro(tubo)}
			}
			var paradas []func() int
			for _, sub := range subs {
				paradas = append(paradas, carreratest.IntercambiarDir(t, filepath.Join(root, filepath.FromSlash(sub)), formas...))
			}
			parar := func() int {
				menos := -1
				for _, p := range paradas {
					if n := p(); menos < 0 || n < menos {
						menos = n
					}
				}
				return menos
			}
			fotos := edr3Fotos(t, "CA-442", caso, root, parar)
			vueltas := parar()
			if vueltas < 4 {
				t.Fatalf("CA-442: fixture: un intercambiador dio %d vueltas", vueltas)
			}

			estados := map[string]int{}
			for i, s := range fotos {
				for k := range edr3Claves(s) {
					if !permitidas[k] {
						t.Fatalf("CA-442: %s: la foto %d no sigue un symlink de .hoom/findings/sub*: trae %s, que no existe en el arbol", caso, i, k)
					}
				}
				for k, h := range s.Huellas {
					h = strings.ToLower(h)
					for p, x := range deAfuera {
						if h == x {
							t.Fatalf("CA-442: %s: la foto %d no lee afuera del arbol: %s tiene la huella de %s", caso, i, k, p)
						}
					}
					for _, sub := range subs {
						switch {
						case k == sub:
							if h != HuellaIlegible && !strings.HasPrefix(h, HuellaNoRegular) {
								t.Fatalf("CA-442: %s: la foto %d: la huella de %s (un directorio o un symlink) es %q o %q, no %q", caso, i, sub, HuellaIlegible, HuellaNoRegular, h)
							}
							estados["sub "+h[:min(len(h), 18)]]++
						case edr3Debajo(k, sub):
							if h != adentro[k] && h != HuellaIlegible {
								t.Fatalf("CA-442: %s: la foto %d: bajo %s solo esta la huella de su %s (o %q): %s es %q", caso, i, sub, nombre, HuellaIlegible, k, h)
							}
							estados["adentro de un sub"]++
						}
					}
				}
			}
			t.Logf("CA-442: %s: %d fotos, al menos %d vueltas por sub, estados %v", caso, len(fotos), vueltas, estados)
		})
	}
}

// ================================================================ (b) y (c) una copia limpia

// edr3CopiaLimpia es una carrera de (b) o (c): que directorio se cambia por
// el symlink y a donde lleva.
type edr3CopiaLimpia struct {
	caso string
	// cambiado es el directorio que la carrera cambia por el symlink.
	cambiado string
	// copia arma la copia limpia (byte a byte, con la evidencia de antes y un
	// hallazgo que solo esta ahi) y devuelve el texto del symlink.
	copia func(t *testing.T, root string) string
}

// edr3CopiaDe copia root/rel byte a byte a un directorio temporal nuevo, le
// agrega extra (relativo a la copia) y lo devuelve.
func edr3CopiaDe(t *testing.T, root, rel, extra string, contenido []byte) string {
	t.Helper()
	copia := edCopiarArbol(t, filepath.Join(root, filepath.FromSlash(rel)))
	write(t, copia, extra, string(contenido))
	return copia
}

// CA-441/CA-442 (hallazgo 545ddc, propiedad de carrera): el rol baja en el
// disco un hallazgo high a low (git no lo ve) y, sin parar, cambia por un
// symlink a una COPIA LIMPIA —con el hallazgo todavia high y uno que solo
// esta en la copia— la raiz .hoom/findings (b) o .hoom entero (c), y la
// devuelve. Para cada foto de la carrera (cada Take vuelve, con reloj): la
// huella del hallazgo, si esta, es la del low o HuellaIlegible, nunca la del
// high; nunca aparece el hallazgo que solo esta en la copia, ni su huella; y,
// parada la carrera, el gate del writer y del refutador sobre (la foto de
// antes, esa foto) nunca queda OK sin violaciones: o la foto vio la
// evidencia bajada, o no la pudo ver, y las dos cosas son manipulacion. La
// copia limpia esta afuera del repo (symlink absoluto y relativo) o adentro
// (donde un os.Root solo, sin mirar que sea el mismo directorio, lo
// seguiria). Control: el gate sobre (antes, antes) pasa.
func TestCA441_LaFotoNoTomaUnaCopiaLimpiaPorLaEvidenciaCuandoSuDirectorioCambiaPorUnSymlink(t *testing.T) {
	qcLimpiarEntorno(t)
	const extra = "20261001T050505_c1ea00.json"
	extraJSON := edr1HallazgoJSON("20261001T050505_c1ea00", "low")
	var carreras []edr3CopiaLimpia
	for _, abs := range []bool{true, false} {
		como := "afuera del repo, symlink relativo"
		if abs {
			como = "afuera del repo, symlink absoluto"
		}
		carreras = append(carreras,
			edr3CopiaLimpia{"(b) .hoom/findings por una copia limpia " + como, ".hoom/findings", func(t *testing.T, root string) string {
				copia := edr3CopiaDe(t, root, ".hoom/findings", extra, extraJSON)
				if abs {
					return copia
				}
				r, err := filepath.Rel(filepath.Join(root, ".hoom"), copia)
				if err != nil {
					t.Fatal(err)
				}
				return r
			}},
			edr3CopiaLimpia{"(c) .hoom por una copia limpia " + como, ".hoom", func(t *testing.T, root string) string {
				copia := edr3CopiaDe(t, root, ".hoom", "findings/"+extra, extraJSON)
				if abs {
					return copia
				}
				r, err := filepath.Rel(root, copia)
				if err != nil {
					t.Fatal(err)
				}
				return r
			}},
		)
	}
	carreras = append(carreras,
		edr3CopiaLimpia{"(b) .hoom/findings por una copia limpia adentro de .hoom", ".hoom/findings", func(t *testing.T, root string) string {
			copia := edr3CopiaDe(t, root, ".hoom/findings", extra, extraJSON)
			edr3Mover(t, filepath.Dir(copia), filepath.Base(copia), filepath.Join(root, ".hoom", "limpio"))
			return "limpio"
		}},
		edr3CopiaLimpia{"(c) .hoom por una copia limpia adentro del repo", ".hoom", func(t *testing.T, root string) string {
			copia := edr3CopiaDe(t, root, ".hoom", "findings/"+extra, extraJSON)
			edr3Mover(t, filepath.Dir(copia), filepath.Base(copia), filepath.Join(root, "hoom-copia"))
			return "hoom-copia"
		}},
	)
	for _, c := range carreras {
		t.Run(c.caso, func(t *testing.T) {
			t.Parallel()
			root := repo(t)
			qcr1Anexar(t, qcr1Exclude(t, root), ".hoom\nhoom-copia")
			_, rel := edHallazgo(t, root, "high", "el retry no respeta el backoff")
			edVeredicto(t, root, "un veredicto")
			alto := edLeer(t, root, rel)
			bajo := edBajarSeveridad(t, alto)
			if !qcr1Ignorado(t, root, rel) {
				t.Fatalf("CA-441: fixture: git ignora %s", rel)
			}
			before := edr1Take(t, "CA-441", c.caso+", foto de antes", root)
			if !strings.EqualFold(before.Huellas[rel], edSHA(alto)) {
				t.Fatalf("CA-441: fixture: la foto de antes trae %s high: %q", rel, before.Huellas[rel])
			}
			for _, rol := range edr3Roles(t) {
				if res := edr1Gate(t, "CA-441", c.caso+", control", root, rol, before, before, PolicyFor(nil, rol)); !res.OK || res.Tampering {
					t.Fatalf("CA-441: fixture: sin cambios el gate del rol %s pasa: %s", rol.Slug, edLista(res.Violations))
				}
			}

			enlace := c.copia(t, root)
			write(t, root, rel, string(bajo))
			parar := carreratest.IntercambiarDir(t, filepath.Join(root, filepath.FromSlash(c.cambiado)), carreratest.Symlink(enlace))
			fotos := edr3Fotos(t, "CA-441", c.caso, root, parar)
			vueltas := parar()
			if vueltas < 4 {
				t.Fatalf("CA-441: fixture: el intercambiador dio %d vueltas", vueltas)
			}
			if got := edLeer(t, root, rel); string(got) != string(bajo) {
				t.Fatalf("CA-441: fixture: parada la carrera, %s es el hallazgo bajado", rel)
			}

			prohibidas := map[string]string{edSHA(alto): "el hallazgo high de la copia limpia", edSHA(extraJSON): "el hallazgo que solo esta en la copia limpia"}
			estados := map[string]int{}
			for i, s := range fotos {
				if _, ok := s.Huellas[".hoom/findings/"+extra]; ok || s.Evidence[".hoom/findings/"+extra] {
					t.Fatalf("CA-442: %s: la foto %d no sigue el symlink de %s: trae %s, que solo esta en la copia limpia", c.caso, i, c.cambiado, extra)
				}
				for k, h := range s.Huellas {
					if que, ok := prohibidas[strings.ToLower(h)]; ok {
						t.Fatalf("CA-441: %s: la foto %d tomo la copia limpia por la evidencia: %s tiene la huella de %s", c.caso, i, k, que)
					}
				}
				h, ok := s.Huellas[rel]
				switch {
				case !ok:
					estados["el hallazgo no esta"]++
				case strings.EqualFold(h, edSHA(bajo)):
					estados["el hallazgo bajado"]++
				case h == HuellaIlegible:
					estados["el hallazgo ilegible"]++
				default:
					t.Fatalf("CA-441: %s: la foto %d: la huella de %s es la del hallazgo bajado o %q: %q", c.caso, i, rel, HuellaIlegible, h)
				}
			}
			distintas := edr3Gates(t, "CA-441", c.caso, root, before, fotos)
			t.Logf("CA-441: %s: %d fotos (%d distintas), %d vueltas, estados %v", c.caso, len(fotos), distintas, vueltas, estados)
		})
	}
}
