// Tests de regresion del hallazgo 20260925T052348_363285 (resilience) de la
// review cruzada de los fixes de C1, sobre cockpitcmd.Open (CA-347, CA-348),
// escritos desde el contrato:
//
// Open cierra la sesion tmux que acaba de crear cuando un paso posterior
// falla (split-window) o cuando no puede registrarla en el item (hallazgo
// 20260924T200412_c1a9da, hallazgos_c1_test.go). Si ESE cierre
// (`tmux kill-session -t =<sesion>`) tambien falla, la sesion sigue viva y
// a medio armar: Open devuelve un error que conserva el error original del
// paso que fallo, NO afirma que la cerro (sin "la cerre"), y dice como
// cerrarla a mano nombrando `tmux kill-session -t =<sesion>`. Con
// kill-session sano, lo de siempre: la cierra, y el reintento la crea y la
// registra. Todo contra el tmux falso con estado h3Tmux: ningun tmux real.
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

// hc3Manual es el comando con el que una persona cierra la sesion a mano.
const hc3Manual = "tmux kill-session -t =" + h3Sesion

// hc3Tmux reutiliza el tmux falso con estado h3Tmux y ademas puede romper
// kill-session: la llamada se registra, la sesion SIGUE viva y devuelve
// hc3ErrKill. Todo lo demas (incluido el verbo en h3Tmux.fallar) lo atiende
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

// hc3SinCierreFalso exige el contrato de 363285 cuando kill-session falla:
// Open creo la sesion e intento cerrarla, la sesion sigue viva (fixture), y
// el error conserva el original, no dice que la cerro y dice como cerrarla a
// mano.
func hc3SinCierreFalso(t *testing.T, ca, caso string, f *hc3Tmux, err error, original string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: hallazgo 363285: %s: Open devuelve el error del paso que fallo:\n%s", ca, caso, f.todas())
	}
	nueva := f.indice("tmux new-session", 0)
	if nueva < 0 || f.indice(hc3Manual, nueva) < 0 {
		t.Fatalf("%s: hallazgo 363285: %s: Open creo la sesion e intento cerrarla con `%s`:\n%s", ca, caso, hc3Manual, f.todas())
	}
	if !f.viva(h3Sesion) {
		t.Fatalf("%s: fixture: %s: con kill-session roto la sesion %s sigue viva:\n%s", ca, caso, h3Sesion, f.todas())
	}
	msg := err.Error()
	if !strings.Contains(msg, original) {
		t.Errorf("%s: hallazgo 363285: %s: el error conserva el original del paso que fallo (%q): %q", ca, caso, original, msg)
	}
	bajo := strings.ToLower(msg)
	if strings.Contains(bajo, "la cerre") || strings.Contains(bajo, "la cerré") {
		t.Errorf("%s: hallazgo 363285: %s: kill-session fallo y la sesion sigue viva: el error no puede decir que la cerro: %q", ca, caso, msg)
	}
	if !strings.Contains(msg, hc3Manual) {
		t.Errorf("%s: hallazgo 363285: %s: el error dice como cerrarla a mano nombrando `%s`: %q", ca, caso, hc3Manual, msg)
	}
}

// Hallazgo 20260925T052348_363285 (resilience). CA-347: new-session funciona,
// split-window falla y el cierre de la sesion (kill-session) TAMBIEN falla.
// Open devuelve el error de split-window, no dice que la cerro, y dice como
// cerrarla a mano; nada se declara en el item (CA-348).
func TestCA347_HC3Hallazgo363285_KillSessionRotoTrasSplitWindow(t *testing.T) {
	root, _ := cbProyecto(t, true)
	crudo := cbLeerItem(t, root)
	f := &hc3Tmux{h3Tmux: &h3Tmux{fallar: "split-window"}, killRoto: true}

	_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
	if f.indice("tmux split-window", 0) < 0 {
		t.Fatalf("CA-347: fixture: Open llego a split-window:\n%s", f.todas())
	}
	hc3SinCierreFalso(t, "CA-347", "split-window roto y kill-session roto", f, err, h3ErrPaso.Error())
	if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: una sesion que no quedo abierta no se declara en el item:\n%s", ahora)
	}
}

// Hallazgo 20260925T052348_363285 (resilience). CA-348: el plan de tmux sale
// bien, registrar la sesion en el item falla (item y .hoom/items/ de solo
// lectura) y el cierre de la sesion (kill-session) TAMBIEN falla. Open
// devuelve el error de la escritura del item, no dice que la cerro, y dice
// como cerrarla a mano.
func TestCA348_HC3Hallazgo363285_KillSessionRotoTrasNoPoderRegistrar(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
	}
	root, _ := cbProyecto(t, true)
	crudo := cbLeerItem(t, root)
	hc3ItemSoloLectura(t, root)
	f := &hc3Tmux{h3Tmux: &h3Tmux{}, killRoto: true}

	_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
	hc3SinCierreFalso(t, "CA-348", "item no escribible y kill-session roto", f, err, "permission denied")
	if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: fixture: el item de solo lectura quedo igual:\n%s", ahora)
	}
}

// GUARDA (verde en la base). Hallazgo 20260925T052348_363285, CA-347 y
// CA-348: con kill-session sano, el comportamiento de c1a9da se mantiene en
// los dos caminos (a traves del mismo falso hc3Tmux): Open devuelve el error
// original, cierra la sesion con `tmux kill-session -t =<sesion>` y el
// reintento la crea (created: true) y la registra en el item.
func TestCA347_HC3Hallazgo363285_GuardaKillSessionSano(t *testing.T) {
	t.Run("split-window", func(t *testing.T) {
		root, _ := cbProyecto(t, true)
		f := &hc3Tmux{h3Tmux: &h3Tmux{fallar: "split-window"}}
		_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
		if err == nil || !strings.Contains(err.Error(), h3ErrPaso.Error()) {
			t.Fatalf("CA-347: con split-window roto Open devuelve su error: %v\n%s", err, f.todas())
		}
		h3SinHuerfana(t, "split-window roto (kill sano)", f.h3Tmux, 0)
		h3Reintento(t, "split-window roto (kill sano)", root, f.h3Tmux)
	})
	t.Run("registrar", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
		}
		root, _ := cbProyecto(t, true)
		restaurar := hc3ItemSoloLectura(t, root)
		f := &hc3Tmux{h3Tmux: &h3Tmux{}}
		_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
		if err == nil || !strings.Contains(err.Error(), "permission denied") {
			t.Fatalf("CA-348: si no puede registrar la sesion Open devuelve el error de la escritura: %v\n%s", err, f.todas())
		}
		h3SinHuerfana(t, "item no escribible (kill sano)", f.h3Tmux, 0)
		restaurar()
		h3Reintento(t, "item no escribible (kill sano)", root, f.h3Tmux)
	})
}
