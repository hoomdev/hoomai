// Tests de regresion de la segunda ronda de endurecimiento-2 en gitx, sobre
// el gate de scope de .hoom/specs/hoom-agent-sobre-determinista.md (CA-133)
// y .hoom/specs/evidencia-en-disco.md (CA-442). Escritos desde el texto de
// los hallazgos y el contrato de la correccion, SIN LEER LA IMPLEMENTACION.
// Reusan git y write (touched_test.go), y e2Repo, e2Roturas, e2GitFallaEn y
// e2ConReloj (hallazgos_endurecimiento2_test.go); los helpers de este
// archivo llevan el prefijo e2b.
//
//   - 20261026T20*_a5cd1a, _90c86c y _6f858b (high; correccion incompleta de
//     9ad39d): para decidir "fuera de un repo", TouchedErr y HuellaConTope
//     buscaban .git por los padres del NOMBRE de dir, sin resolver enlaces.
//     Si dir es un enlace simbolico (un alias) a un subdirectorio de un repo
//     roto, git trabaja en el destino real y falla, pero los padres del alias
//     no tienen .git: el fallo se tomaba por "fuera de un repo" y volvia una
//     lista vacia (o ninguna huella) sin error. Ahora es un error; un alias a
//     un directorio fuera de cualquier repo sigue dando vacio sin error.
package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

// e2bAlias crea, en un directorio fuera de cualquier repo, un enlace
// simbolico (absoluto o relativo) a destino, y lo devuelve. Saltea donde no
// hay enlaces simbolicos.
func e2bAlias(t *testing.T, destino string, relativo bool) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	afuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(destino)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(afuera, "alias")
	apunta := real
	if relativo {
		if apunta, err = filepath.Rel(afuera, real); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(apunta, alias); err != nil {
		t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
	}
	return alias
}

// CA-133, CA-442 (hallazgos a5cd1a, 90c86c y 6f858b): dir es un enlace
// simbolico (absoluto o relativo), puesto fuera de cualquier repo, al
// subdirectorio sub/ de un repo roto (HEAD corrupto, config corrupto, .git de
// un worktree cuyo gitdir ya no existe, .git vacio). git, que trabaja en el
// destino real, falla: TouchedErr devuelve error, no una lista vacia que el
// gate de scope toma por "nada cambio"; y HuellaConTope devuelve error y
// ninguna huella. Lo mismo con el alias a la raiz del repo (ahi el .git se ve
// a traves del alias: no depende de resolverlo, es la guarda de que el caso
// comun sigue). Guardas: con el repo sano, por el alias TouchedErr y
// HuellaConTope dan, sin error, lo mismo que por el subdirectorio mismo; y git
// de verdad falla por el alias (si no, Skip).
func TestHallazgo_a5cd1a_90c86c_6f858b_CA133_UnAliasAUnRepoRotoEsUnError(t *testing.T) {
	const ca = "CA-133, CA-442 (hallazgos a5cd1a, 90c86c y 6f858b, alias a un repo roto)"
	sano := e2Repo(t)
	alias := e2bAlias(t, filepath.Join(sano, "sub"), false)
	porAlias, err := TouchedErr(alias, "main")
	directo, err2 := TouchedErr(filepath.Join(sano, "sub"), "main")
	if err != nil || err2 != nil || len(porAlias) == 0 || !reflect.DeepEqual(porAlias, directo) {
		t.Fatalf("%s: guarda: con el repo sano, TouchedErr por un alias a su subdirectorio da sin error lo mismo que por el subdirectorio: %v %v; %v %v", ca, porAlias, err, directo, err2)
	}
	ha, err := HuellaConTope(alias, 30*time.Second)
	hd, err2 := HuellaConTope(filepath.Join(sano, "sub"), 30*time.Second)
	if err != nil || err2 != nil || ha == "" || ha != hd {
		t.Fatalf("%s: guarda: con el repo sano, HuellaConTope por un alias a su subdirectorio da sin error la misma huella que por el subdirectorio: %q %v; %q %v", ca, ha, err, hd, err2)
	}

	for _, r := range e2Roturas {
		for _, cs := range []struct {
			nombre, sub string
			relativo    bool
		}{
			{"alias absoluto a sub/", "sub", false},
			{"alias relativo a sub/", "sub", true},
			{"alias absoluto a la raiz", "", false},
		} {
			t.Run(r.nombre+", "+cs.nombre, func(t *testing.T) {
				repo := e2Repo(t)
				r.romper(t, repo)
				dir := e2bAlias(t, filepath.Join(repo, cs.sub), cs.relativo)
				e2GitFallaEn(t, dir)

				var m map[string]string
				var err error
				e2ConReloj(t, ca, "TouchedErr", func() { m, err = TouchedErr(dir, "main") })
				if err == nil {
					t.Errorf("%s: con %s, pedido por un %s, git falla: TouchedErr devuelve error, no la lista %v sin error (el gate de scope la toma por \"nada cambio\")",
						ca, r.nombre, cs.nombre, m)
				}
				var h string
				e2ConReloj(t, ca, "HuellaConTope", func() { h, err = HuellaConTope(dir, 30*time.Second) })
				if err == nil || h != "" {
					t.Errorf("%s: con %s, pedido por un %s, git falla: HuellaConTope devuelve error y ninguna huella, no %q con error %v",
						ca, r.nombre, cs.nombre, h, err)
				}
			})
		}
	}
}

// CA-133 (hallazgos a5cd1a, 90c86c y 6f858b, guarda del contrato de siempre):
// un alias (absoluto o relativo) a un directorio fuera de cualquier repo
// sigue sin nada que listar: TouchedErr da una lista vacia sin error, y
// HuellaConTope ninguna huella sin error.
func TestHallazgo_a5cd1a_90c86c_6f858b_CA133_UnAliasFueraDeUnRepoSigueVacioSinError(t *testing.T) {
	const ca = "CA-133 (hallazgos a5cd1a, 90c86c y 6f858b, alias fuera de un repo)"
	destino := t.TempDir()
	write(t, destino, "sub/archivo.txt", "fuera de un repo\n")
	for _, cs := range []struct {
		nombre, sub string
		relativo    bool
	}{
		{"alias absoluto", "", false},
		{"alias relativo", "", true},
		{"alias absoluto a un subdirectorio", "sub", false},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			dir := e2bAlias(t, filepath.Join(destino, cs.sub), cs.relativo)
			c := exec.Command("git", "rev-parse", "--git-dir")
			c.Dir = dir
			if _, err := c.CombinedOutput(); err == nil {
				t.Skipf("%s: fixture: el directorio temporal de este sistema esta dentro de un repo git", ca)
			}
			var m map[string]string
			var err error
			e2ConReloj(t, ca, "TouchedErr", func() { m, err = TouchedErr(dir, "main") })
			if err != nil || len(m) != 0 {
				t.Errorf("%s: por un %s a un directorio fuera de un repo, TouchedErr da una lista vacia sin error: %v %v", ca, cs.nombre, m, err)
			}
			var h string
			e2ConReloj(t, ca, "HuellaConTope", func() { h, err = HuellaConTope(dir, 30*time.Second) })
			if err != nil || h != "" {
				t.Errorf("%s: por un %s a un directorio fuera de un repo, HuellaConTope no da huella ni error: %q %v", ca, cs.nombre, h, err)
			}
		})
	}
}
