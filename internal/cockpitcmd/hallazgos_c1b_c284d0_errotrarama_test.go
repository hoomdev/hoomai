// Tests adversariales del hallazgo 20261021T174140_c284d0 (medium,
// reliability) de la delta 1 de fixes-cabina-c1b, sobre `hoom cockpit
// --task` (CA-85 de .hoom/specs/cockpit-tmux-zellij.md; CA-280 de
// .hoom/specs/items-y-columna-derivada.md) y runcmd.ErrOtraRama. Escritos
// desde el contrato, nunca desde la implementacion:
//
// Con el worktree de la tarea registrado en su lugar pero fuera de su rama
// hoom/<slug>, el error de Open (tmux) y de Run (tmux y zellij) envuelve
// runcmd.ErrOtraRama (errors.Is): el cockpit no lo reemplaza por "la tarea
// no existe".
//
// OJO: runcmd.ErrOtraRama no existe en 37b6ecb: este archivo no compila ahi
// (falla en la base). Usa c28Intentos de hallazgos_c1b_c284d0_test.go.
package cockpitcmd

import (
	"errors"
	"testing"

	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/worktreetest"
)

// Hallazgo c284d0. CA-85 y CA-280: con legit fuera de su rama, el error del
// cockpit envuelve runcmd.ErrOtraRama.
func TestHallazgoc284d0_CA85_CockpitFueraDeSuRamaEsErrOtraRama(t *testing.T) {
	for _, v := range worktreetest.RamasAjenas {
		v := v
		t.Run(v.Nombre, func(t *testing.T) {
			e := t28Proyecto(t)
			v.Armar(t, e.legit, "legit")
			for _, in := range c28Intentos(t, e) {
				if !errors.Is(in.err, runcmd.ErrOtraRama) {
					t.Errorf("CA-85: hallazgo c284d0 (%s): el error de %s envuelve runcmd.ErrOtraRama: %v", v.Nombre, in.como, in.err)
				}
			}
		})
	}
}
