// Tests de regresion de la tercera ronda de endurecimiento-2 en gitx, sobre
// el gate de scope de .hoom/specs/hoom-agent-sobre-determinista.md (CA-133)
// y .hoom/specs/evidencia-en-disco.md (CA-442). Escritos desde el texto de
// los hallazgos y el contrato de la correccion, SIN LEER LA IMPLEMENTACION.
// Reusan git y write (touched_test.go), e2Repo, e2Roturas, e2GitFallaEn y
// e2ConReloj (hallazgos_endurecimiento2_test.go) y e2bAlias
// (hallazgos_endurecimiento2b_test.go); los helpers de este archivo llevan el
// prefijo e2c.
//
//   - 20261026T21*_7adfcb (high; correccion incompleta de los alias): para
//     decidir "fuera de un repo", el camino se limpiaba con filepath.Abs
//     antes de resolver enlaces. Con dir = <afuera>/alias/.., donde alias
//     apunta a repo/sub, git entra por el enlace y despues sube: trabaja en
//     repo; pero la limpieza colapsaba alias/.. a <afuera>, fuera de todo
//     repo. Con el repo roto, el fallo de git se tomaba por "fuera de un
//     repo" y volvia una lista vacia (o ninguna huella) sin error. Ahora es
//     un error; con alias apuntando fuera de todo repo, sigue vacio sin
//     error.
//   - 20261026T21*_0738fe (high): la decision "fuera de un repo" resolvia el
//     camino otra vez DESPUES de correr git; un enlace cambiado en el medio
//     la desligaba del directorio donde corrio git. El caso que la fuerza
//     esta en hallazgos_endurecimiento2c_unix_test.go (un git que cambia el
//     enlace, en un proceso hijo). Lo observable del diseno (por un alias,
//     la lista y la huella son las del directorio real) lo cubre la guarda
//     de 90c86c en hallazgos_endurecimiento2b_test.go.
package gitx

import (
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// e2cPuntoPunto es alias seguido de rel ("/.." o "/../sub") SIN limpiar: un
// filepath.Join colapsaria el alias.
func e2cPuntoPunto(alias, rel string) string { return alias + rel }

// CA-133, CA-442 (hallazgo 7adfcb): dir es <afuera>/alias/.. (o
// <afuera>/alias/../sub), donde alias, puesto fuera de todo repo, apunta
// (absoluto o relativo) a repo/sub de un repo roto (HEAD corrupto, config
// corrupto, .git de un worktree cuyo gitdir ya no existe, .git vacio). git
// entra por el enlace y sube: trabaja en el repo roto y falla. TouchedErr
// devuelve error, no una lista vacia; HuellaConTope, error y ninguna huella.
// Y con el repo sano, por <afuera>/alias/.. TouchedErr y HuellaConTope dan,
// sin error, lo mismo que por el repo mismo (la misma limpieza del camino
// hacia que los archivos se buscaran en <afuera> y salieran como borrados).
// Guarda: git de verdad falla por ese camino (si no, Skip).
func TestHallazgo_7adfcb_CA133_UnAliasSeguidoDePuntoPuntoAUnRepoRotoEsUnError(t *testing.T) {
	const ca = "CA-133, CA-442 (hallazgo 7adfcb, alias/.. a un repo roto)"
	t.Run("repo sano", func(t *testing.T) {
		sano := e2Repo(t)
		dir := e2cPuntoPunto(e2bAlias(t, filepath.Join(sano, "sub"), false), "/..")
		porAlias, err := TouchedErr(dir, "main")
		directo, err2 := TouchedErr(sano, "main")
		if err != nil || err2 != nil || len(porAlias) == 0 || !reflect.DeepEqual(porAlias, directo) {
			t.Errorf("%s: con el repo sano, TouchedErr por %s da sin error lo mismo que por el repo: %v %v; por el repo %v %v", ca, dir, porAlias, err, directo, err2)
		}
		ha, err := HuellaConTope(dir, 30*time.Second)
		hd, err2 := HuellaConTope(sano, 30*time.Second)
		if err != nil || err2 != nil || ha == "" || ha != hd {
			t.Errorf("%s: con el repo sano, HuellaConTope por %s da sin error la huella del repo: %q %v; por el repo %q %v", ca, dir, ha, err, hd, err2)
		}
	})

	for _, r := range e2Roturas {
		for _, cs := range []struct {
			nombre, rel string
			relativo    bool
		}{
			{"alias absoluto a sub/, seguido de /..", "/..", false},
			{"alias relativo a sub/, seguido de /..", "/..", true},
			{"alias absoluto a sub/, seguido de /../sub", "/../sub", false},
		} {
			t.Run(r.nombre+", "+cs.nombre, func(t *testing.T) {
				repo := e2Repo(t)
				r.romper(t, repo)
				dir := e2cPuntoPunto(e2bAlias(t, filepath.Join(repo, "sub"), cs.relativo), cs.rel)
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

// CA-133 (hallazgo 7adfcb, guarda del contrato de siempre): con alias
// apuntando a un directorio fuera de todo repo, <afuera>/alias/.. (y
// <afuera>/alias/../sub) sigue sin nada que listar: TouchedErr da una lista
// vacia sin error, y HuellaConTope ninguna huella sin error. (Con
// <afuera>/alias/../sub, la limpieza del camino daba <afuera>/sub, que no
// existe, y el resultado era un error: la misma causa, del lado seguro.)
func TestHallazgo_7adfcb_CA133_UnAliasSeguidoDePuntoPuntoFueraDeUnRepoSigueVacio(t *testing.T) {
	const ca = "CA-133 (hallazgo 7adfcb, alias/.. fuera de un repo)"
	destino := t.TempDir()
	write(t, destino, "sub/archivo.txt", "fuera de un repo\n")
	for _, cs := range []struct {
		nombre, rel string
		relativo    bool
	}{
		{"alias absoluto, seguido de /..", "/..", false},
		{"alias relativo, seguido de /..", "/..", true},
		{"alias absoluto, seguido de /../sub", "/../sub", false},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			dir := e2cPuntoPunto(e2bAlias(t, filepath.Join(destino, "sub"), cs.relativo), cs.rel)
			c := exec.Command("git", "rev-parse", "--git-dir")
			c.Dir = dir
			if _, err := c.CombinedOutput(); err == nil {
				t.Skipf("%s: fixture: el directorio temporal de este sistema esta dentro de un repo git", ca)
			}
			var m map[string]string
			var err error
			e2ConReloj(t, ca, "TouchedErr", func() { m, err = TouchedErr(dir, "main") })
			if err != nil || len(m) != 0 {
				t.Errorf("%s: por %s, fuera de un repo, TouchedErr da una lista vacia sin error: %v %v", ca, dir, m, err)
			}
			var h string
			e2ConReloj(t, ca, "HuellaConTope", func() { h, err = HuellaConTope(dir, 30*time.Second) })
			if err != nil || h != "" {
				t.Errorf("%s: por %s, fuera de un repo, HuellaConTope no da huella ni error: %q %v", ca, dir, h, err)
			}
		})
	}
}
