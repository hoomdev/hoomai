// Tests de regresion de la tercera ronda de endurecimiento-2 en hoomfs.
// Escritos desde el texto del hallazgo y el contrato de la correccion, SIN
// LEER LA IMPLEMENTACION. Los helpers de este archivo llevan el prefijo e2c.
//
//   - 20261026T21*_0c9aec (high, risk): EnsureIgnored leia .hoom/.gitignore
//     anclado (por descriptores, desde root) pero lo escribia despues por
//     ruta (MkdirAll y AtomicWrite). Si entre la lectura y la escritura .hoom
//     pasaba a ser un enlace al de otro proyecto, la escritura pisaba el
//     .gitignore ajeno. Ahora la escritura sigue el mismo recorrido anclado
//     que la lectura.
//
// Como se ve la escritura por ruta sin carrera, de forma determinista: un
// root "<base>/enlace/../p", donde enlace apunta a <base>/sitio/sub. El
// recorrido anclado (el kernel) entra por el enlace y sube: lee
// <base>/sitio/p/.hoom/.gitignore. Una ruta armada con filepath.Join se
// limpia sin mirar enlaces y da <base>/p/.hoom/.gitignore: otro proyecto.
// Leer en un lugar y escribir en otro es exactamente el defecto; la carrera
// del hallazgo (cambiar .hoom en la ventana) no tiene forma determinista de
// sostenerse desde afuera (una sonda que alternaba .hoom entre un directorio
// y un enlace mientras llamaba a EnsureIgnored le acerto en 1 de 3 corridas
// de 10 s).
package hoomfs

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// e2cDosLugares arma <base>/sitio/sub, el proyecto que ve el kernel
// (<base>/sitio/p, con un .hoom/.gitignore al que le faltan las entradas) y
// el que ve la limpieza lexica (<base>/p, la victima, con su .gitignore), y
// el enlace <base>/enlace -> <base>/sitio/sub. Devuelve el root
// "<base>/enlace/../p" (sin limpiar), el .gitignore que se lee y el de la
// victima.
func e2cDosLugares(t *testing.T) (root, leido, victima string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range []string{filepath.Join(base, "sitio", "sub"), filepath.Join(base, "sitio", "p", ".hoom"), filepath.Join(base, "p", ".hoom")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	leido = filepath.Join(base, "sitio", "p", ".hoom", ".gitignore")
	victima = filepath.Join(base, "p", ".hoom", ".gitignore")
	if err := os.WriteFile(leido, []byte("solo-esto/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(victima, []byte("de-la-victima/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "sitio", "sub"), filepath.Join(base, "enlace")); err != nil {
		t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
	}
	root = filepath.Join(base, "enlace") + "/../p"
	if b, err := os.ReadFile(root + "/.hoom/.gitignore"); err != nil || string(b) != "solo-esto/\n" {
		t.Fatalf("fixture: por el root %s, el kernel ve el .gitignore de <base>/sitio/p: %q %v", root, b, err)
	}
	return root, leido, victima
}

// CA-333 (hallazgo 0c9aec): EnsureIgnored escribe donde leyo. Con un root
// cuyo camino resuelto por el kernel (<base>/sitio/p) y cuyo camino limpiado
// sin mirar enlaces (<base>/p, otro proyecto) son distintos, pidiendo todas
// las entradas o solo "envelopes": el .gitignore de <base>/p queda identico y
// no aparece nada nuevo en su .hoom; y EnsureIgnored o da error, o completo
// el .gitignore que leyo (el de <base>/sitio/p).
func TestHallazgo_0c9aec_CA333_EnsureIgnoredEscribeDondeLeyo(t *testing.T) {
	const ca = "CA-333 (hallazgo 0c9aec, EnsureIgnored escribe donde leyo)"
	for _, pide := range [][]string{nil, {"envelopes"}} {
		t.Run(strings.Join(append([]string{"pide"}, pide...), " "), func(t *testing.T) {
			root, leido, victima := e2cDosLugares(t)
			added, err := EnsureIgnored(root, pide...)
			if b, _ := os.ReadFile(victima); string(b) != "de-la-victima/\n" {
				t.Errorf("%s: EnsureIgnored(%q, %v) leyo %s y escribio el .gitignore de otro proyecto (%s): ahora %q",
					ca, root, pide, leido, victima, b)
			}
			entradas, _ := os.ReadDir(filepath.Dir(victima))
			for _, e := range entradas {
				if e.Name() != ".gitignore" {
					t.Errorf("%s: en el .hoom del otro proyecto no aparece nada nuevo: aparecio %s", ca, e.Name())
				}
			}
			if err != nil {
				t.Logf("%s: EnsureIgnored dio error: %v", ca, err)
				return
			}
			b, _ := os.ReadFile(leido)
			if !strings.Contains(string(b), "envelopes") {
				t.Errorf("%s: EnsureIgnored dio nil (agrego %v): completo el .gitignore que leyo (%s); tiene %q", ca, added, leido, b)
			}
		})
	}
}

// CA-333 (hallazgo 0c9aec, el caso estatico): con .hoom enlazado (absoluto o
// relativo) al .hoom de otro proyecto, al que le faltan entradas en su
// .gitignore (o no), EnsureIgnored da error y el .gitignore ajeno queda
// identico. (Este caso ya pasaba antes de la correccion: la lectura anclada
// rechaza el enlace; lo que faltaba era la escritura, ver el test de arriba.)
func TestHallazgo_0c9aec_CA333_EnsureIgnoredConHoomEnlazadoNoTocaElAjeno(t *testing.T) {
	const ca = "CA-333 (hallazgo 0c9aec, .hoom enlazado a otro proyecto)"
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	for _, relativo := range []bool{false, true} {
		for _, contenido := range []string{"solo-esto/\n", GitignoreBody()} {
			t.Run(map[bool]string{false: "absoluto", true: "relativo"}[relativo]+", "+map[bool]string{true: "le faltan entradas", false: "completo"}[contenido == "solo-esto/\n"], func(t *testing.T) {
				base, err := filepath.EvalSymlinks(t.TempDir())
				if err != nil {
					t.Fatal(err)
				}
				root, otro := filepath.Join(base, "proyecto"), filepath.Join(base, "otro")
				if err := os.MkdirAll(filepath.Join(otro, ".hoom"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.MkdirAll(root, 0o755); err != nil {
					t.Fatal(err)
				}
				ajeno := filepath.Join(otro, ".hoom", ".gitignore")
				if err := os.WriteFile(ajeno, []byte(contenido), 0o644); err != nil {
					t.Fatal(err)
				}
				destino := filepath.Join(otro, ".hoom")
				if relativo {
					destino = filepath.Join("..", "otro", ".hoom")
				}
				if err := os.Symlink(destino, filepath.Join(root, ".hoom")); err != nil {
					t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
				}
				_, err = EnsureIgnored(root)
				if b, _ := os.ReadFile(ajeno); string(b) != contenido {
					t.Errorf("%s: el .gitignore del otro proyecto queda identico: ahora %q", ca, b)
				}
				if err == nil {
					t.Errorf("%s: con .hoom enlazado a otro proyecto, EnsureIgnored da error, no nil", ca)
				}
			})
		}
	}
}
