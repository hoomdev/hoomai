// Tests adversariales de la segunda ronda de review de
// .hoom/specs/evidencia-en-disco.md (enmienda 1):
//
//   - CA-442, hallazgos a0c424, b3dadb y 4219de: un DIRECTORIO creado durante
//     la corrida bajo .hoom/{findings,verdicts,approvals} es "una entrada
//     creada que no es un archivo regular": UNA manipulacion por directorio
//     creado, con "la evidencia es un archivo regular" y su ruta, para cada
//     rol, lo vea git o no (git nunca lista un directorio vacio). Take anota
//     esos directorios en Snapshot.Directorios (sin las tres raices) y no en
//     Huellas.
//   - CA-444: un directorio creado sin permiso de lectura es "la evidencia no
//     se puede leer".
//   - CA-441, hallazgo 793d8a: un symlink de evidencia que existia y cambia de
//     destino cambio: su huella es la del texto del enlace, sin seguirlo, asi
//     que dos destinos con el mismo contenido no lo esconden.
//
// Las carreras de 98faf2 (la foto decide sobre el descriptor) estan en
// evidencia_en_disco_ronda2_unix_test.go. Usa los helpers ed*/edr1* de
// evidencia_en_disco_test.go y evidencia_en_disco_ronda1_test.go.
package agentcmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/finding"
)

// edr2Mkdir crea root/rel (y lo que falte arriba).
func edr2Mkdir(t *testing.T, root, rel string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, rel), 0o755); err != nil {
		t.Fatal(err)
	}
}

// edr2Tampering cuenta las violaciones de manipulacion.
func edr2Tampering(vs []Violation) int {
	n := 0
	for _, v := range vs {
		if v.Rule == RuleTampering {
			n++
		}
	}
	return n
}

// edr2Conjunto devuelve las claves verdaderas de m, ordenadas.
func edr2Conjunto(m map[string]bool) []string {
	var out []string
	for k, v := range m {
		if v {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// edr2SinLectura le quita a root/rel todo permiso (se lo devuelve al final
// del test) y dice si de verdad quedo ilegible: como root, o en un sistema
// de archivos que no lo respeta, no se puede medir.
func edr2SinLectura(t *testing.T, root, rel string) bool {
	t.Helper()
	if os.Geteuid() == 0 {
		return false
	}
	p := filepath.Join(root, rel)
	edr1Restaurar(t, p, 0o755)
	if err := os.Chmod(p, 0); err != nil {
		t.Fatal(err)
	}
	_, err := os.ReadDir(p)
	return err != nil
}

// ================================================================ CA-442: directorios creados

// CA-442 (hallazgos a0c424, b3dadb, 4219de): para cada forma de esconder (o
// no) lo creado y para CADA rol, el refutador incluido, un directorio creado
// durante la corrida bajo la evidencia es UNA manipulacion con "la evidencia
// es un archivo regular" y su ruta, con su hallazgo high del gate:
//
//   - vacios, con nombre valido de hallazgo (<id>.json), de resolucion de un
//     hallazgo abierto (<id>.res.json: reservaria el nombre y Resolve ya no
//     podria escribirla), de veredicto, o cualquiera (un dotfile, unicode),
//     bajo findings, verdicts y approvals;
//   - anidados (.hoom/findings/a/b): a y b son una manipulacion cada uno;
//   - uno nuevo con un archivo adentro: el directorio por no ser un archivo
//     regular, y el archivo por la forma ("solo se crean archivos con la
//     forma que escribe hoom");
//   - un hallazgo que existia y que la corrida cambia por un directorio con
//     su mismo nombre: UNA manipulacion en esa ruta (desaparecio o no es un
//     archivo regular), no dos.
//
// Controles: un subdirectorio que ya estaba antes de la corrida no es
// violacion; un archivo creado adentro de el es UNA (la de la forma), y el
// directorio ninguna. CA-444: un directorio creado con modo 000 es UNA
// manipulacion con "la evidencia no se puede leer". Y fuera de esas rutas no
// hay ninguna otra manipulacion. (Cada rol corre en paralelo, en su copia
// del repo armado para la variante.)
func TestCA442_UnDirectorioCreadoBajoLaEvidenciaEsManipulacion(t *testing.T) {
	qcLimpiarEntorno(t)
	roles := agents.Roles()
	if len(roles) < 6 {
		t.Fatalf("CA-442: fixture: la tabla trae los roles de hoom (refutador incluido), trajo %d", len(roles))
	}
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			e.armar(t, plantilla)
			abierto, _ := edHallazgo(t, plantilla, "high", "abierto: su resolucion no puede quedar reservada")
			_, reemplazado := edHallazgo(t, plantilla, "medium", "lo reemplaza un directorio")
			previos := []string{".hoom/findings/previo", ".hoom/verdicts/previo"}
			for _, d := range append(append([]string(nil), previos...), ".hoom/approvals") {
				edr2Mkdir(t, plantilla, d)
			}

			vacios := []string{
				".hoom/findings/20261001T010101_abcdef.json",
				qcResRel(abierto),
				".hoom/findings/cualquiera",
				".hoom/findings/.oculto",
				".hoom/findings/ñandú",
				".hoom/verdicts/2026-10-01T01-01-01Z_0123abcd.json",
				".hoom/verdicts/x",
				".hoom/approvals/y",
				".hoom/findings/a",
				".hoom/findings/a/b",
			}
			nuevo := ".hoom/verdicts/nuevo"
			enNuevo := nuevo + "/2026-10-01T01-01-02Z_0123abce.json"
			enPrevio := ".hoom/findings/previo/20261001T010102_abcdef.json"
			cerrado := ".hoom/findings/cerrado"

			for _, rol := range roles {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := edr1Take(t, "CA-442", caso+", foto de antes", root)

					for _, d := range vacios {
						edr2Mkdir(t, root, d)
					}
					write(t, root, enNuevo, "{\"verdict\":\"green\"}\n")
					write(t, root, enPrevio, string(edr1HallazgoJSON("20261001T010102_abcdef", "low")))
					if err := os.Remove(filepath.Join(root, reemplazado)); err != nil {
						t.Fatal(err)
					}
					edr2Mkdir(t, root, reemplazado)
					edr2Mkdir(t, root, cerrado)
					ilegible := edr2SinLectura(t, root, cerrado)
					after := edr1Take(t, "CA-442", caso+", foto con directorios", root)

					if e.oculto {
						ignoradas := edIgnorados(t, root, []string{enNuevo, enPrevio, reemplazado})
						for _, r := range []string{enNuevo, enPrevio, reemplazado} {
							if !ignoradas[r] {
								t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, r)
							}
						}
					}

					creados := append(append([]string(nil), vacios...), nuevo)
					for _, d := range append(append([]string(nil), creados...), reemplazado) {
						if before.Directorios[d] || !after.Directorios[d] {
							t.Fatalf("CA-442: %s: %s es un directorio creado en la corrida: antes %v, despues %v (Directorios despues: %v)",
								caso, d, before.Directorios[d], after.Directorios[d], edr2Conjunto(after.Directorios))
						}
						if _, ok := after.Huellas[d]; ok {
							t.Fatalf("CA-440: %s: Huellas no trae el directorio %s: %q", caso, d, after.Huellas[d])
						}
					}
					for _, p := range previos {
						if !before.Directorios[p] || !after.Directorios[p] {
							t.Fatalf("CA-442: %s: el subdirectorio previo %s esta en las dos fotos: %v / %v", caso, p, before.Directorios[p], after.Directorios[p])
						}
					}

					res := edr1Gate(t, "CA-442", caso, root, rol, before, after, PolicyFor(nil, rol))
					for _, d := range creados {
						var v Violation
						if d == qcResRel(abierto) && rol.Slug != finding.RolQueRefuta {
							// la regla de quien-cierra-un-hallazgo tambien la mira:
							// sigue siendo UNA, con cualquiera de los dos detalles
							v = edr1Una(t, "CA-442", caso+", directorio creado", res, d, []string{edr1Regular, d}, []string{qcNoCierra(rol.Slug)})
						} else {
							v = edExigir(t, "CA-442", caso+", directorio creado", res, d, edr1Regular, d)
						}
						if v.FindingID == "" {
							t.Fatalf("CA-442: %s: la manipulacion en el directorio %s registra su hallazgo high del gate", caso, d)
						}
						if rol.Slug == "writer" {
							edr1HallazgoDelGate(t, "CA-442", caso+", directorio "+d, root, v.FindingID)
						}
					}
					edExigir(t, "CA-442", caso+", archivo en un subdirectorio nuevo", res, enNuevo, edForma("verdicts"), enNuevo)
					edExigir(t, "CA-442", caso+", archivo en un subdirectorio previo", res, enPrevio, edForma("findings"), enPrevio)
					for _, p := range previos {
						edSinManipulacion(t, "CA-442", caso+", subdirectorio previo", res, p)
					}
					if e.oculto {
						edr1Una(t, "CA-442", caso+", hallazgo cambiado por un directorio", res, reemplazado,
							[]string{edAppendOnly, reemplazado}, []string{edr1Regular, reemplazado})
					} else {
						edr1Una(t, "CA-442", caso+", hallazgo cambiado por un directorio", res, reemplazado)
					}
					quiere := len(creados) + 4
					var vc Violation
					if ilegible {
						vc = edExigir(t, "CA-444", caso+", directorio creado con modo 000", res, cerrado, edr1Ilegible, cerrado)
					} else {
						// como root (o en un disco que no respeta el modo) es un
						// directorio creado mas
						vc = edExigir(t, "CA-442", caso+", directorio creado", res, cerrado, edr1Regular, cerrado)
					}
					if vc.FindingID == "" {
						t.Fatalf("CA-444: %s: la manipulacion en %s registra su hallazgo high del gate", caso, cerrado)
					}
					if n := edr2Tampering(res.Violations); n != quiere {
						t.Fatalf("CA-442: %s: una manipulacion por ruta creada o cambiada y ninguna mas (%d, hay %d): %s",
							caso, quiere, n, edLista(res.Violations))
					}
				})
			}
		})
	}
}

// CA-442 (hallazgos a0c424, b3dadb): un directorio creado donde no habia
// evidencia (sin .hoom/approvals ni .hoom/verdicts antes de la corrida) sigue
// siendo manipulacion: .hoom/approvals/y y cada nivel de .hoom/verdicts/x/z
// son UNA manipulacion cada uno con "la evidencia es un archivo regular",
// para el writer y el refutador, lo vea git o no.
func TestCA442_UnDirectorioCreadoDondeNoHabiaEvidenciaEsManipulacion(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			for _, slug := range []string{"writer", finding.RolQueRefuta} {
				rol := edr1Rol(t, slug)
				caso := e.caso + ", rol " + slug
				root := repo(t)
				e.armar(t, root)
				before := edr1Take(t, "CA-442", caso+", foto de antes", root)
				for _, d := range []string{".hoom/approvals/y", ".hoom/verdicts/x/z"} {
					edr2Mkdir(t, root, d)
				}
				after := edr1Take(t, "CA-442", caso+", foto de despues", root)
				res := edr1Gate(t, "CA-442", caso, root, rol, before, after, PolicyFor(nil, rol))
				for _, d := range []string{".hoom/approvals/y", ".hoom/verdicts/x", ".hoom/verdicts/x/z"} {
					v := edExigir(t, "CA-442", caso+", directorio creado", res, d, edr1Regular, d)
					if v.FindingID == "" {
						t.Fatalf("CA-442: %s: la manipulacion en %s registra su hallazgo high del gate", caso, d)
					}
				}
			}
		})
	}
}

// CA-442/CA-440 (hallazgos a0c424, b3dadb): Take anota en Directorios
// EXACTAMENTE los directorios bajo .hoom/{findings,verdicts,approvals} en el
// disco (rutas relativas con /), anidados incluidos y sin las tres raices,
// los vea git o no; y Huellas no los trae (sigue siendo solo de entradas que
// no son directorios). Un symlink a un directorio es una entrada que no es un
// archivo regular: va a Huellas con HuellaNoRegular, no a Directorios, y la
// foto no entra por el. Lo de al lado de la evidencia no es evidencia.
func TestCA442_TakeAnotaLosDirectoriosDeLaEvidenciaYNoLosHuellea(t *testing.T) {
	qcLimpiarEntorno(t)
	root := repo(t)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/verdicts/")
	dirs := []string{
		".hoom/findings/a",
		".hoom/findings/a/b",
		".hoom/findings/a/b/c",
		".hoom/findings/20261001T010101_abcdef.json",
		".hoom/findings/20261001T010101_abcdef.res.json",
		".hoom/findings/.oculto",
		".hoom/findings/ñandú",
		".hoom/verdicts/x",
		".hoom/verdicts/x/y",
		".hoom/verdicts/2026-10-01T01-01-01Z_0123abcd.json",
		".hoom/approvals/y",
	}
	for _, d := range dirs {
		edr2Mkdir(t, root, d)
	}
	archivos := []string{
		".hoom/findings/a/b/notas.txt",
		".hoom/verdicts/x/y/2026-10-01T01-01-02Z_0123abce.json",
		".hoom/approvals/y/demo_0123abcd.json",
		".hoom/findings/20261001T010102_abcdef.json",
	}
	for _, a := range archivos {
		write(t, root, a, "{\"en\":\""+a+"\"}\n")
	}
	// al lado, no adentro
	for _, d := range []string{".hoom/specs/sub", ".hoom/runs/sub", ".hoom/findings-borrador/sub", "internal/.hoom/findings/sub",
		".hoom/isolated/q/.hoom/findings/sub"} {
		edr2Mkdir(t, root, d)
	}
	// un symlink a un directorio dentro y a uno fuera de la evidencia
	afuera := t.TempDir()
	edr2Mkdir(t, afuera, "sub/subsub")
	write(t, afuera, "sub/20261001T010103_abcdef.json", "{}\n")
	enlaces := map[string]string{
		".hoom/findings/enlace":        "a",
		".hoom/verdicts/enlace-afuera": afuera,
	}
	for rel, destino := range enlaces {
		edr1Symlink(t, root, rel, destino)
	}

	s := edr1Take(t, "CA-442", "directorios de la evidencia", root)
	if s.Directorios == nil {
		t.Fatalf("CA-442: Take llena Directorios siempre: es nil")
	}
	quiere := map[string]bool{}
	for _, d := range dirs {
		quiere[d] = true
	}
	var falta, sobra []string
	for _, d := range dirs {
		if !s.Directorios[d] {
			falta = append(falta, d)
		}
	}
	for d, ok := range s.Directorios {
		if !ok || !quiere[d] {
			sobra = append(sobra, d)
		}
	}
	sort.Strings(falta)
	sort.Strings(sobra)
	if len(falta) != 0 || len(sobra) != 0 {
		t.Fatalf("CA-442: Directorios son exactamente los directorios bajo la evidencia (sin las raices):\nfaltan: %v\nsobran: %v", falta, sobra)
	}
	for _, d := range dirs {
		if _, ok := s.Huellas[d]; ok {
			t.Fatalf("CA-440: Huellas no trae directorios: %s = %q", d, s.Huellas[d])
		}
	}
	for _, r := range []string{".hoom/findings", ".hoom/verdicts", ".hoom/approvals"} {
		if _, ok := s.Huellas[r]; ok {
			t.Fatalf("CA-440: Huellas no trae la raiz %s", r)
		}
	}
	for _, a := range archivos {
		if got := strings.ToLower(s.Huellas[a]); got != edSHA([]byte("{\"en\":\""+a+"\"}\n")) {
			t.Fatalf("CA-440: el archivo %s, dentro de un subdirectorio, tiene la huella de su contenido: %q", a, s.Huellas[a])
		}
	}
	for rel := range enlaces {
		if s.Directorios[rel] {
			t.Fatalf("CA-442: un symlink a un directorio no es un directorio de la evidencia: %s", rel)
		}
		if !strings.HasPrefix(s.Huellas[rel], HuellaNoRegular) {
			t.Fatalf("CA-442: un symlink a un directorio va a Huellas con %q: %s = %q", HuellaNoRegular, rel, s.Huellas[rel])
		}
		for k := range s.Huellas {
			if strings.HasPrefix(k, rel+"/") {
				t.Fatalf("CA-442: la foto no entra por el symlink %s: %s", rel, k)
			}
		}
		for k := range s.Directorios {
			if strings.HasPrefix(k, rel+"/") {
				t.Fatalf("CA-442: la foto no entra por el symlink %s: %s", rel, k)
			}
		}
	}
}

// CA-442/CA-440: sin directorios bajo la evidencia, Directorios es un mapa
// vacio y no nil: sin .hoom, con las tres raices vacias, con evidencia sin
// subdirectorios, y fuera de git.
func TestCA442_SinSubdirectoriosDirectoriosEsUnMapaVacio(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, c := range []struct {
		caso  string
		armar func(t *testing.T) string
	}{
		{"sin .hoom", func(t *testing.T) string { return repo(t) }},
		{"las tres raices vacias", func(t *testing.T) string {
			root := repo(t)
			for _, d := range []string{".hoom/findings", ".hoom/verdicts", ".hoom/approvals", ".hoom/specs/sub"} {
				edr2Mkdir(t, root, d)
			}
			return root
		}},
		{"evidencia sin subdirectorios", func(t *testing.T) string {
			root := repo(t)
			edHallazgo(t, root, "high", "uno")
			edVeredicto(t, root, "uno")
			return root
		}},
		{"fuera de git", func(t *testing.T) string {
			root := t.TempDir()
			write(t, root, ".hoom/findings/20260101T000000_abcdef.json", "{}\n")
			edr2Mkdir(t, root, ".hoom/approvals")
			return root
		}},
	} {
		t.Run(c.caso, func(t *testing.T) {
			s := Take(c.armar(t), "main")
			if s.Directorios == nil || len(s.Directorios) != 0 {
				t.Fatalf("CA-442: %s: sin subdirectorios Directorios es un mapa vacio (no nil): %#v", c.caso, s.Directorios)
			}
		})
	}
}

// CA-442 de punta a punta (hallazgos 4219de, b3dadb): 'hoom agent --role
// writer' implementa y ademas crea directorios vacios con el nombre de la
// resolucion de un hallazgo abierto (que reservaria esa ruta: Resolve ya no
// podria escribirla), de un veredicto valido y uno cualquiera bajo
// .hoom/approvals/, lo vea git o no. git no lista directorios vacios, y aun
// asi el sobre corta en scope, no-entregable, exit 1, sin veredicto: UNA
// manipulacion por directorio, con su hallazgo high del gate en el disco.
func TestCA442_ElWriterQueCreaDirectoriosVaciosBajoLaEvidenciaCortaEnScope(t *testing.T) {
	for _, oculto := range []bool{false, true} {
		nombre := "git lo ve"
		if oculto {
			nombre = "git lo ignora"
		}
		t.Run(nombre, func(t *testing.T) {
			root := edRepo(t, false, false)
			if oculto {
				qcr1Anexar(t, qcr1Exclude(t, root), edReglas)
			}
			id, _ := edHallazgo(t, root, "high", "abierto: su resolucion no puede quedar reservada")
			dirs := []string{qcResRel(id), ".hoom/verdicts/2026-10-01T01-01-01Z_0123abcd.json", ".hoom/approvals/y"}
			script := edImplementar
			for _, d := range dirs {
				script += "mkdir -p " + qcComillas(d) + "\n"
			}
			fakeProvider(t, "claude", script+"exit 0\n")
			res := edCorrer(t, "CA-442", nombre, root, Options{Role: "writer", Prompt: "implementa"})
			for _, d := range dirs {
				if fi, err := os.Stat(filepath.Join(root, d)); err != nil || !fi.IsDir() {
					t.Fatalf("CA-442: fixture: el writer falso creo el directorio %s: %v", d, err)
				}
			}
			if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
				t.Errorf("CA-442: %s: directorios creados bajo la evidencia no dejan verde al sobre: status=%s verdict=%s exit=%d stage=%s scope=%s",
					nombre, res.Status, res.Verdict, res.ExitCode, res.Stage, edLista(res.Scope.Violations))
			}
			for _, d := range dirs {
				var v Violation
				if d == qcResRel(id) {
					v = edr1Una(t, "CA-442", nombre+", directorio creado", res.Scope, d, []string{edr1Regular, d}, []string{qcNoCierra("writer")})
				} else {
					v = edExigir(t, "CA-442", nombre+", directorio creado", res.Scope, d, edr1Regular, d)
				}
				edr1HallazgoDelGate(t, "CA-442", nombre+", "+d, root, v.FindingID)
			}
			if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status != "no-entregable" {
				t.Fatalf("CA-442: %s: la manipulacion corta el sobre en scope, no-entregable, exit 1, sin veredicto: %+v", nombre, res)
			}
		})
	}
}

// CA-443 con CA-442 (hallazgos a0c424, b3dadb): 'hoom agent --role
// test-writer' corre ciego en .hoom/isolated, escribe su test y crea con una
// ruta relativa directorios vacios bajo la evidencia del arbol REAL (uno con
// nombre valido de hallazgo y otro bajo .hoom/approvals/, que git ignora).
// El sobre no termina verde, y cada directorio es UNA manipulacion en el
// arbol real con "la evidencia es un archivo regular" y su hallazgo del gate.
func TestCA443_ElRolCiegoQueCreaDirectoriosEnLaEvidenciaDelArbolRealEsManipulacion(t *testing.T) {
	root := edRepo(t, false, true)
	qcr1Anexar(t, qcr1Exclude(t, root), edReglas)
	dirs := []string{".hoom/findings/20261001T070707_d1d1d1.json", ".hoom/approvals/y"}
	script := ""
	for _, d := range dirs {
		script += "mkdir -p " + qcComillas(edRealDesdeLaCuarentena(d)) + "\n"
	}
	donde := filepath.Join(t.TempDir(), "pwd.txt")
	fakeProvider(t, "claude", qcr2Donde(donde)+qcr2TestLegitimo+script+"exit 0\n")
	res := edCorrer(t, "CA-443", "directorios en el arbol real", root, Options{Role: "test-writer", Prompt: "escribi los tests"})
	qcr2EnLaCuarentena(t, "directorios en el arbol real", donde)
	if res.Isolation == nil {
		t.Fatalf("CA-443: fixture: el test-writer corrio ciego: %+v", res)
	}
	for _, d := range dirs {
		if fi, err := os.Stat(filepath.Join(root, d)); err != nil || !fi.IsDir() {
			t.Fatalf("CA-443: fixture: el test-writer falso creo el directorio %s en el arbol real: %v", d, err)
		}
	}
	if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
		t.Errorf("CA-443: directorios creados en la evidencia del arbol real no dejan verde al sobre ciego: status=%s verdict=%s exit=%d stage=%s scope=%s",
			res.Status, res.Verdict, res.ExitCode, res.Stage, edLista(res.Scope.Violations))
	}
	for _, d := range dirs {
		v := edExigir(t, "CA-443", "directorio en el arbol real", res.Scope, d, edr1Regular, d)
		edr1HallazgoDelGate(t, "CA-443", "directorio en el arbol real "+d, root, v.FindingID)
	}
}

// ================================================================ CA-441: el symlink que cambia de destino

// edr2Escondites son los escondites de CA-441 mas las dos formas de que git
// SI vea la evidencia previa: sin commitear, o commiteada.
func edr2Escondites() []edEscondite {
	return append(edEscondites(),
		edEscondite{caso: "sin commitear, git la ve"},
		edEscondite{caso: "commiteada, git la ve", sellar: func(t *testing.T, root string, rutas []string) {
			git(t, root, append([]string{"add", "-f", "--"}, rutas...)...)
			git(t, root, "commit", "-q", "-m", "evidencia commiteada")
		}},
	)
}

// CA-441 (hallazgo 793d8a): para cada forma de esconder (o no) la evidencia
// que ya existia y para CADA rol, el refutador incluido: un symlink con
// nombre de hallazgo o de veredicto que existia antes de la corrida y que la
// corrida apunta a OTRO destino cambio, aunque los dos destinos tengan el
// mismo contenido byte a byte (y aunque el texto nuevo lleve al mismo
// archivo): UNA manipulacion "la evidencia es append-only: <ruta> cambio
// durante el run" cuando git no lo ve (si lo ve, la del piso de git), con su
// hallazgo high del gate. Control: el que se borra y se vuelve a crear con el
// MISMO texto no cambio; el que no se toca, tampoco. (Cada rol corre en
// paralelo, en su copia de la evidencia armada para el escondite.)
func TestCA441_UnSymlinkDeEvidenciaQueExistiaYCambiaDeDestinoEsManipulacion(t *testing.T) {
	qcLimpiarEntorno(t)
	roles := agents.Roles()
	for _, e := range edr2Escondites() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			if e.antes != nil {
				e.antes(t, plantilla)
			}
			afuera := t.TempDir()
			mismo := string(edr1HallazgoJSON("20261001T050505_5e1f05", "high"))
			a, b := filepath.Join(afuera, "a.json"), filepath.Join(afuera, "b.json")
			write(t, afuera, "a.json", mismo)
			write(t, afuera, "b.json", mismo)

			hallazgo := ".hoom/findings/20261001T050505_5e1f05.json"
			veredicto := ".hoom/verdicts/2026-10-01T05-05-05Z_5e1f0005.json"
			relativo := ".hoom/findings/20261001T050506_5e1f06.json" // -> su vecino; despues -> ./su vecino
			igual := ".hoom/findings/20261001T050507_5e1f07.json"    // se vuelve a crear con el mismo texto
			quieto := ".hoom/findings/20261001T050508_5e1f08.json"   // no se toca
			edr1Symlink(t, plantilla, hallazgo, a)
			edr1Symlink(t, plantilla, veredicto, a)
			edr1Symlink(t, plantilla, relativo, "20261001T050505_5e1f05.json")
			edr1Symlink(t, plantilla, igual, a)
			edr1Symlink(t, plantilla, quieto, a)
			if e.sellar != nil {
				e.sellar(t, plantilla, []string{hallazgo, igual, quieto, relativo, veredicto})
			}

			cambian := map[string]string{hallazgo: b, veredicto: b, relativo: "./20261001T050505_5e1f05.json"}
			for _, rol := range roles {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := edr1Take(t, "CA-441", caso+", foto de antes", root)
					for rel, destino := range cambian {
						if err := os.Remove(filepath.Join(root, rel)); err != nil {
							t.Fatal(err)
						}
						edr1Symlink(t, root, rel, destino)
					}
					if err := os.Remove(filepath.Join(root, igual)); err != nil {
						t.Fatal(err)
					}
					edr1Symlink(t, root, igual, a)
					after := edr1Take(t, "CA-441", caso+", foto de despues", root)

					// la huella de un symlink: HuellaNoRegular, su tipo, ":" y el
					// sha256 del texto del enlace
					textos := map[string][2]string{hallazgo: {a, b}, veredicto: {a, b}, igual: {a, a}, quieto: {a, a},
						relativo: {"20261001T050505_5e1f05.json", "./20261001T050505_5e1f05.json"}}
					for rel, tx := range textos {
						for i, s := range []Snapshot{before, after} {
							h := s.Huellas[rel]
							if !strings.HasPrefix(h, HuellaNoRegular) || !strings.HasSuffix(strings.ToLower(h), ":"+edSHA([]byte(tx[i]))) {
								t.Fatalf("CA-441: %s: la huella del symlink %s -> %s es %q + su tipo + \":\" + el sha256 del texto: %q",
									caso, rel, tx[i], HuellaNoRegular, h)
							}
						}
					}
					for rel := range cambian {
						if before.Huellas[rel] == "" || before.Huellas[rel] == after.Huellas[rel] {
							t.Fatalf("CA-441: %s: la huella del symlink %s es la de su texto: cambia con el destino (%q -> %q)",
								caso, rel, before.Huellas[rel], after.Huellas[rel])
						}
					}
					for _, rel := range []string{igual, quieto} {
						if before.Huellas[rel] == "" || before.Huellas[rel] != after.Huellas[rel] {
							t.Fatalf("CA-441: %s: el symlink %s con el mismo texto tiene la misma huella (%q -> %q)",
								caso, rel, before.Huellas[rel], after.Huellas[rel])
						}
					}

					res := edr1Gate(t, "CA-441", caso, root, rol, before, after, PolicyFor(nil, rol))
					for rel := range cambian {
						var v Violation
						if edOculta(t, root, rel, e) {
							v = edExigir(t, "CA-441", caso+", symlink que cambia de destino", res, rel, edAppendOnly, rel, edCambio)
						} else {
							// git lo ve: el piso de hoy lo marca y no se duplica
							v = edExigir(t, "CA-441", caso+", symlink que cambia de destino", res, rel)
						}
						if v.FindingID == "" {
							t.Fatalf("CA-441: %s: la manipulacion en %s registra su hallazgo high del gate", caso, rel)
						}
						if rol.Slug == "writer" {
							edr1HallazgoDelGate(t, "CA-441", caso+", "+rel, root, v.FindingID)
						}
					}
					for _, rel := range []string{igual, quieto} {
						edSinManipulacion(t, "CA-441", caso+", symlink con el mismo texto", res, rel)
					}
					if n := edr2Tampering(res.Violations); n != len(cambian) {
						t.Fatalf("CA-441: %s: una manipulacion por symlink que cambio y ninguna mas (%d, hay %d): %s",
							caso, len(cambian), n, edLista(res.Violations))
					}
				})
			}
		})
	}
}
