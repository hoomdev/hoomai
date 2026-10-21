// Tests de regresion del hallazgo 20260925T052348_363285 (resilience) de la
// review cruzada de los fixes de C1, sobre cockpitcmd.Open (CA-347, CA-348),
// escritos desde el contrato:
//
// Con el contrato de C1, Open cerraba la sesion tmux que acababa de crear
// cuando un paso posterior fallaba (split-window) o cuando no podia
// registrarla en el item (hallazgo 20260924T200412_c1a9da), y 363285 pedia
// que, si ESE cierre (`kill-session -t =<sesion>`) tambien fallaba, el error
// no dijera que la cerro y dijera como cerrarla a mano.
//
// La ENMIENDA 6 de .hoom/specs/acciones-desde-la-tarjeta.md (decision de
// Henry, hallazgo bde7e4) saco ese cierre: Open NO cierra la sesion (no corre
// ningun kill-session), devuelve el error, que dice como cerrarla a mano con
// `tmux kill-session -t '=<sesion>'` (destino entre comillas: enmienda 5,
// hallazgo db8541; que corra tal cual en sh y en zsh lo exige
// cierreEjecutable, en hallazgos_db8541_f7efdc_test.go), y la sesion cuenta
// como existente: el reintento responde created false y no registra nada. Un
// kill-session roto ya no tiene nada que romper: los tests de 363285 lo dejan
// como trampa (si Open lo corriera, quedaria anotado) y exigen la enmienda 6
// en ese mismo escenario. Las piezas de la enmienda 6 (h6NoLaCerro,
// h6ReintentoLaVeExistente, h6CerrarAMano) estan en
// hallazgos_c1b_bde7e4_test.go. Todo contra el tmux falso con estado h3Tmux:
// ningun tmux real.
package cockpitcmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/item"
)

var hc3ErrKill = errors.New("tmux falso: kill-session fallo a proposito (hc3)")

// hc3Tmux reutiliza el tmux falso con estado h3Tmux y ademas puede romper
// kill-session: la llamada se registra, la sesion SIGUE viva y devuelve
// hc3ErrKill (con la enmienda 6, una trampa: Open no debe correrlo). Todo lo demas (incluido el verbo en h3Tmux.fallar) lo atiende
// h3Tmux por los tres canales.
type hc3Tmux struct {
	*h3Tmux
	killRoto bool
}

func (f *hc3Tmux) atender(name string, args []string) ([]byte, error) {
	if f.killRoto && name == "tmux" && len(args) > 0 && args[0] == "kill-session" {
		f.mu.Lock()
		f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
		f.mu.Unlock()
		return nil, hc3ErrKill
	}
	return f.h3Tmux.atender(name, args)
}

func (f *hc3Tmux) deps() Deps {
	d := f.h3Tmux.deps()
	run := func(dir, name string, args ...string) error {
		_, err := f.atender(name, args)
		return err
	}
	d.RunCmd, d.QuietCmd = run, run
	d.Output = func(dir, name string, args ...string) ([]byte, error) {
		return f.atender(name, args)
	}
	return d
}

// hc3ItemSoloLectura deja el item de facturas y .hoom/items/ de solo lectura
// (registrar la sesion falla) y devuelve con que restaurarlos.
func hc3ItemSoloLectura(t *testing.T, root string) func() {
	t.Helper()
	p := item.Path(root, "facturas")
	dir := filepath.Dir(p)
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	restaurar := func() { os.Chmod(dir, 0o755); os.Chmod(p, 0o644) }
	t.Cleanup(restaurar)
	return restaurar
}

// Hallazgo 20260925T052348_363285 (resilience), con la ENMIENDA 6. CA-347:
// new-session funciona, split-window falla y kill-session esta roto. Open
// devuelve el error de split-window y ni intenta cerrar la sesion (ningun
// kill-session ni kill-pane): sigue viva, a medias; el error no dice que la
// cerro y dice como cerrarla a mano; nada se declara en el item (CA-348), y
// el reintento con tmux sano la ve existente (created false) y no registra
// nada.
//
// Reemplaza a TestCA347_HC3Hallazgo363285_KillSessionRotoTrasSplitWindow, que
// exigia el error de un kill-session fallido: con la enmienda 6 (decision de
// Henry, hallazgo bde7e4) Open no corre kill-session, asi que un
// kill-session roto perdio su razon de ser. No es debilitar: ahora se exige
// no tocar una sesion que Open no puede probar suya, en el mismo escenario.
func TestCA347_HC3Hallazgo363285_Enmienda6_ConKillSessionRotoTrasSplitWindowNiLoIntenta(t *testing.T) {
	const ca = "CA-347: 363285 (enmienda 6): split-window roto y kill-session roto"
	root, _ := cbProyecto(t, true)
	crudo := cbLeerItem(t, root)
	f := &hc3Tmux{h3Tmux: &h3Tmux{fallar: "split-window"}, killRoto: true}

	_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
	if f.indice("tmux new-session", 0) < 0 || f.indice("tmux split-window", 0) < 0 {
		t.Fatalf("%s: fixture: Open creo la sesion y llego a split-window:\n%s", ca, f.todas())
	}
	h6NoLaCerro(t, ca, f.h3Tmux, err, h3ErrPaso.Error())
	if !f.viva(h3Sesion) {
		t.Errorf("%s: la sesion %s sigue viva, a medias: Open no la toca:\n%s", ca, h3Sesion, f.todas())
	}
	if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: una sesion que no quedo armada no se declara en el item:\n%s", ahora)
	}
	h6ReintentoLaVeExistente(t, ca, root, f.h3Tmux, crudo)
}

// Hallazgo 20260925T052348_363285 (resilience), con la ENMIENDA 6. CA-348: el
// plan de tmux sale bien, registrar la sesion en el item falla (item y
// .hoom/items/ de solo lectura) y kill-session esta roto. Open devuelve el
// error de la escritura del item y ni intenta cerrar la sesion (sigue viva);
// el error no dice que la cerro y dice como cerrarla a mano. El reintento,
// con el item escribible, la ve existente y no registra nada.
//
// Reemplaza a TestCA348_HC3Hallazgo363285_KillSessionRotoTrasNoPoderRegistrar:
// el contrato cambio por la enmienda 6 (decision de Henry, hallazgo bde7e4) y
// un kill-session roto perdio su razon de ser. No es debilitar: ahora se
// exige no tocar una sesion que Open no puede probar suya.
func TestCA348_HC3Hallazgo363285_Enmienda6_ConKillSessionRotoTrasNoPoderRegistrarNiLoIntenta(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
	}
	const ca = "CA-348: 363285 (enmienda 6): item no escribible y kill-session roto"
	root, _ := cbProyecto(t, true)
	crudo := cbLeerItem(t, root)
	restaurar := hc3ItemSoloLectura(t, root)
	f := &hc3Tmux{h3Tmux: &h3Tmux{}, killRoto: true}

	_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
	if f.indice("tmux new-session", 0) < 0 {
		t.Fatalf("%s: fixture: Open creo la sesion:\n%s", ca, f.todas())
	}
	h6NoLaCerro(t, ca, f.h3Tmux, err, "permission denied")
	if !f.viva(h3Sesion) {
		t.Errorf("%s: la sesion %s sigue viva: Open no la toca:\n%s", ca, h3Sesion, f.todas())
	}
	if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: fixture: el item de solo lectura quedo igual:\n%s", ahora)
	}
	restaurar()
	h6ReintentoLaVeExistente(t, ca, root, f.h3Tmux, crudo)
}

// GUARDA, con la ENMIENDA 6. Hallazgo 20260925T052348_363285, CA-347 y
// CA-348: con kill-session SANO (que cerraria la sesion si Open lo corriera)
// Open tampoco la cierra, en los dos caminos (a traves del mismo falso
// hc3Tmux): devuelve el error original, no corre kill-session ni kill-pane,
// la sesion sigue viva y el error dice como cerrarla a mano; el reintento la
// ve existente (created false) y no registra nada, y despues de cerrarla a
// mano con ese comando el siguiente intento la crea (created: true) y la
// registra.
//
// Reemplaza a TestCA347_HC3Hallazgo363285_GuardaKillSessionSano, que exigia
// cerrarla (el comportamiento de c1a9da): el contrato cambio por la enmienda
// 6 (decision de Henry, hallazgo bde7e4). No es debilitar: ahora se exige no
// tocar una sesion que Open no puede probar suya.
func TestCA347_CA348_HC3Hallazgo363285_Enmienda6_ConKillSessionSanoTampocoLaCierra(t *testing.T) {
	t.Run("split-window", func(t *testing.T) {
		const ca = "CA-347: 363285 (enmienda 6): split-window roto (kill sano)"
		root, _ := cbProyecto(t, true)
		crudo := cbLeerItem(t, root)
		f := &hc3Tmux{h3Tmux: &h3Tmux{fallar: "split-window"}}
		_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
		h6NoLaCerro(t, ca, f.h3Tmux, err, h3ErrPaso.Error())
		if !f.viva(h3Sesion) {
			t.Errorf("%s: la sesion %s sigue viva: Open no la toca:\n%s", ca, h3Sesion, f.todas())
		}
		h6ReintentoLaVeExistente(t, ca, root, f.h3Tmux, crudo)
		h6CerrarAMano(t, ca, f.h3Tmux)
		h3Reintento(t, "363285 (enmienda 6): split-window roto (kill sano), cerrada a mano", root, f.h3Tmux)
	})
	t.Run("registrar", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
		}
		const ca = "CA-348: 363285 (enmienda 6): item no escribible (kill sano)"
		root, _ := cbProyecto(t, true)
		crudo := cbLeerItem(t, root)
		restaurar := hc3ItemSoloLectura(t, root)
		f := &hc3Tmux{h3Tmux: &h3Tmux{}}
		_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
		h6NoLaCerro(t, ca, f.h3Tmux, err, "permission denied")
		if !f.viva(h3Sesion) {
			t.Errorf("%s: la sesion %s sigue viva: Open no la toca:\n%s", ca, h3Sesion, f.todas())
		}
		restaurar()
		h6ReintentoLaVeExistente(t, ca, root, f.h3Tmux, crudo)
		h6CerrarAMano(t, ca, f.h3Tmux)
		h3Reintento(t, "363285 (enmienda 6): item no escribible (kill sano), cerrada a mano", root, f.h3Tmux)
	})
}
