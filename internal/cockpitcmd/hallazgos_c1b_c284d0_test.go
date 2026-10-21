// Tests adversariales del hallazgo 20261021T174140_c284d0 (medium,
// reliability) de la delta 1 de fixes-cabina-c1b, sobre `hoom cockpit
// --task` (CA-85 de .hoom/specs/cockpit-tmux-zellij.md; CA-280 de
// .hoom/specs/items-y-columna-derivada.md). Escritos desde el contrato,
// nunca desde la implementacion:
//
// Con el worktree de la tarea registrado en su lugar pero fuera de su rama
// hoom/<slug> (HEAD separado, otra rama, main, o una rama que empieza como la
// suya), Open (tmux) y Run (tmux y zellij) fallan sin lanzar nada y su error
// es el de la rama: nombra hoom/<slug>, dice "rama", y no dice que la tarea
// "no existe" ni manda a `hoom task start` (la carpeta ya existe). Guarda: de
// vuelta en su rama, Open monta la sesion en el worktree.
//
// Este archivo compila en 37b6ecb. Lo que exige runcmd.ErrOtraRama (que no
// existe ahi) esta en hallazgos_c1b_c284d0_errotrarama_test.go.
//
// Reusa t28Proyecto, t28SinLanzar y t28NuevaSesionEn de
// hallazgos_c1b_28c350_test.go, el tmux falso h3Tmux y
// worktreetest.RamasAjenas.
package cockpitcmd

import (
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// c28Intento es lo que respondio una forma de abrir el cockpit.
type c28Intento struct {
	como string
	err  error
}

// c28Intentos abre el cockpit de legit por Open (tmux) y Run (tmux y
// zellij), cada uno con su tmux falso, exige que ninguno lance nada y
// devuelve sus errores.
func c28Intentos(t *testing.T, e t28Entorno) []c28Intento {
	t.Helper()
	var out []c28Intento
	f := &h3Tmux{}
	_, err := Open(e.root, "demo", Options{Provider: "claude", Task: "legit", Mux: "tmux"}, f.deps())
	t28SinLanzar(t, "Open", "legit", f)
	out = append(out, c28Intento{"Open", err})
	for _, mux := range []string{"tmux", "zellij"} {
		f := &h3Tmux{}
		err := Run(e.root, "demo", Options{Provider: "claude", Task: "legit", Mux: mux}, f.deps())
		t28SinLanzar(t, "Run ("+mux+")", "legit", f)
		out = append(out, c28Intento{"Run (" + mux + ")", err})
	}
	return out
}

// Hallazgo c284d0. CA-85 y CA-280: con legit fuera de su rama, el cockpit
// devuelve el error de la rama, no "la tarea no existe".
func TestHallazgoc284d0_CA85_CockpitConElWorktreeFueraDeSuRamaDiceLaRama(t *testing.T) {
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			e := t28Proyecto(t)
			v.Armar(t, e.legit, "legit")
			for _, in := range c28Intentos(t, e) {
				if in.err == nil {
					t.Errorf("CA-280: hallazgo c284d0 (%s): %s con el worktree fuera de su rama debe fallar", v.Nombre, in.como)
					continue
				}
				m := in.err.Error()
				if !strings.Contains(m, "hoom/legit") || !strings.Contains(m, "rama") {
					t.Errorf("CA-85: hallazgo c284d0 (%s): el error de %s es el de la rama: nombra hoom/legit y dice \"rama\": %v", v.Nombre, in.como, in.err)
				}
				if strings.Contains(m, "no existe") || strings.Contains(m, "task start") {
					t.Errorf("CA-85: hallazgo c284d0 (%s): el worktree existe: el error de %s no dice que la tarea no existe ni manda a crearla: %v", v.Nombre, in.como, in.err)
				}
			}

			worktreetest.VolverASuRama(t, e.legit, "legit")
			f := &h3Tmux{}
			if _, err := Open(e.root, "demo", Options{Provider: "claude", Task: "legit", Mux: "tmux"}, f.deps()); err != nil {
				t.Fatalf("CA-85: guarda c284d0 (%s): de vuelta en su rama Open funciona: %v\n%s", v.Nombre, err, f.todas())
			}
			if !t28NuevaSesionEn(f, e.legit) {
				t.Errorf("CA-85: guarda c284d0 (%s): la sesion vive en el worktree (%s):\n%s", v.Nombre, e.legit, f.todas())
			}
		})
	}
}
