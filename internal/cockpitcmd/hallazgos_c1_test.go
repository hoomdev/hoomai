// Tests de regresion de la review cruzada de la cabina (C1, 2026-09-24),
// hallazgo 20260924T200412_c1a9da sobre cockpitcmd.Open: si new-session
// funciona y algo posterior falla (un paso del plan de tmux, o registrar la
// sesion en el item), Open dejaba viva una sesion a medio armar. El reintento
// la encontraba "existente" (created: false, CA-347) y ya no la registraba
// en el item (CA-348): el writer declarado se perdia para siempre. Ahora
// Open cierra la sesion que acaba de crear (`tmux kill-session -t =<sesion>`)
// y devuelve el error, y el reintento la crea y la registra. Todo contra un
// tmux falso con estado: ningun tmux real.
package cockpitcmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hoomdev/hoomai/internal/item"
)

const h3Sesion = "hoom-demo-facturas"

// h3Tmux es un tmux falso CON ESTADO: new-session crea la sesion,
// kill-session la cierra y has-session/list-panes la encuentran solo si
// esta viva. El verbo en fallar devuelve un error (despues de registrarse
// la llamada). Atiende igual por RunCmd, QuietCmd y Output.
type h3Tmux struct {
	mu       sync.Mutex
	vivas    map[string]bool
	fallar   string
	llamadas []string // "tmux <args>", en orden, de los tres canales
}

var h3ErrPaso = errors.New("tmux falso: el paso fallo a proposito (h3)")

func h3Destino(args []string, flag string) string {
	for i, a := range args {
		if a == flag && i+1 < len(args) {
			d := strings.TrimPrefix(args[i+1], "=")
			if j := strings.IndexAny(d, ":."); j >= 0 {
				d = d[:j]
			}
			return d
		}
	}
	return ""
}

func (f *h3Tmux) atender(name string, args []string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.llamadas = append(f.llamadas, name+" "+strings.Join(args, " "))
	if name != "tmux" || len(args) == 0 {
		return nil, nil
	}
	if f.vivas == nil {
		f.vivas = map[string]bool{}
	}
	if args[0] == f.fallar {
		return nil, h3ErrPaso
	}
	switch args[0] {
	case "has-session":
		if !f.vivas[h3Destino(args, "-t")] {
			return nil, errors.New("can't find session")
		}
	case "list-panes":
		if !f.vivas[h3Destino(args, "-t")] {
			return nil, errors.New("can't find session")
		}
		return []byte("%1\n%2\n"), nil
	case "new-session":
		f.vivas[h3Destino(args, "-s")] = true
	case "kill-session":
		delete(f.vivas, h3Destino(args, "-t"))
	}
	return nil, nil
}

func (f *h3Tmux) deps() Deps {
	run := func(dir, name string, args ...string) error {
		_, err := f.atender(name, args)
		return err
	}
	return Deps{
		LookPath: func(p string) (string, error) { return "/usr/bin/" + p, nil },
		RunCmd:   run,
		QuietCmd: run,
		Getenv:   func(string) string { return "" },
		Output: func(dir, name string, args ...string) ([]byte, error) {
			return f.atender(name, args)
		},
		HoomBin: "/opt/fake/bin/hoom",
	}
}

// indice de la primera llamada que empieza con prefijo desde la posicion desde.
func (f *h3Tmux) indice(prefijo string, desde int) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := desde; i < len(f.llamadas); i++ {
		if strings.HasPrefix(f.llamadas[i], prefijo) {
			return i
		}
	}
	return -1
}

func (f *h3Tmux) viva(s string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.vivas[s]
}

func (f *h3Tmux) todas() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.llamadas, "\n")
}

// h3SinHuerfana exige que un Open fallido no deje la sesion viva: si la
// creo (new-session), la cerro despues con kill-session -t =<sesion>.
func h3SinHuerfana(t *testing.T, caso string, f *h3Tmux, desde int) {
	t.Helper()
	if f.viva(h3Sesion) {
		t.Fatalf("CA-347: hallazgo c1a9da: %s: Open fallo y dejo viva la sesion %s a medio armar:\n%s", caso, h3Sesion, f.todas())
	}
	if nueva := f.indice("tmux new-session", desde); nueva >= 0 {
		if kill := f.indice("tmux kill-session -t ="+h3Sesion, nueva); kill < 0 {
			t.Fatalf("CA-347: hallazgo c1a9da: %s: Open cierra la sesion que acaba de crear con `tmux kill-session -t =%s`:\n%s", caso, h3Sesion, f.todas())
		}
	}
}

// h3Reintento: con tmux sano, Open crea la sesion de nuevo y la registra en
// el item: UNA entrada en sesiones, la de este provider.
func h3Reintento(t *testing.T, caso, root string, f *h3Tmux) {
	t.Helper()
	f.fallar = ""
	desde := len(f.llamadas)
	s, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
	if err != nil {
		t.Fatalf("CA-347: %s: el reintento con tmux sano abre la sesion: %v\n%s", caso, err, f.todas())
	}
	if !s.Created || s.Name != h3Sesion {
		t.Fatalf("CA-347: hallazgo c1a9da: %s: el reintento CREA la sesion de nuevo (created: true), no la encuentra a medio armar: %+v\n%s", caso, s, f.todas())
	}
	if f.indice("tmux new-session", desde) < 0 || !f.viva(h3Sesion) {
		t.Fatalf("CA-347: %s: el reintento corrio new-session y la sesion quedo viva:\n%s", caso, f.todas())
	}
	it, err := item.Load(root, "facturas")
	if err != nil {
		t.Fatalf("CA-348: %s: el item sigue siendo valido: %v\n%s", caso, err, cbLeerItem(t, root))
	}
	if len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
		t.Fatalf("CA-348: hallazgo c1a9da: %s: el reintento registra la sesion en el item (una entrada, claude): %+v", caso, it.Sesiones)
	}
}

// Hallazgo 20260924T200412_c1a9da (CA-347, CA-348): new-session funciona y
// split-window falla. Open devuelve el error, cierra la sesion que acaba de
// crear (`tmux kill-session -t =<sesion>`) y no declara nada en el item; el
// reintento con tmux sano crea la sesion (created: true) y la registra.
func TestHallazgo_c1a9da_OpenCierraLaSesionSiFallaUnPasoDelPlan(t *testing.T) {
	root, _ := cbProyecto(t, true)
	crudo := cbLeerItem(t, root)
	f := &h3Tmux{fallar: "split-window"}

	_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
	if err == nil {
		t.Fatalf("CA-347: con split-window roto Open devuelve el error:\n%s", f.todas())
	}
	if !strings.Contains(err.Error(), h3ErrPaso.Error()) {
		t.Fatalf("CA-347: Open devuelve el error del paso que fallo: %v", err)
	}
	if f.indice("tmux new-session", 0) < 0 || f.indice("tmux split-window", 0) < 0 {
		t.Fatalf("CA-347: fixture: new-session funciono y despues fallo split-window:\n%s", f.todas())
	}
	h3SinHuerfana(t, "split-window roto", f, 0)
	if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: una sesion que no quedo abierta no se declara en el item:\n%s", ahora)
	}
	h3Reintento(t, "split-window roto", root, f)
}

// Hallazgo 20260924T200412_c1a9da (CA-347, CA-348): el plan de tmux sale
// bien pero registrar la sesion en el item falla (item y .hoom/items/ de solo
// lectura). Open devuelve error y cierra la sesion, asi el reintento (con el
// item escribible) la crea y la registra.
func TestHallazgo_c1a9da_OpenCierraLaSesionSiNoPuedeRegistrarla(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("CA-348: corriendo como root los permisos no frenan la escritura del item")
	}
	root, _ := cbProyecto(t, true)
	p := item.Path(root, "facturas")
	dir := filepath.Dir(p)
	crudo := cbLeerItem(t, root)
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	restaurar := func() { os.Chmod(dir, 0o755); os.Chmod(p, 0o644) }
	t.Cleanup(restaurar)
	f := &h3Tmux{}

	_, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
	if err == nil {
		t.Fatalf("CA-348: hallazgo c1a9da: si no puede registrar la sesion en el item, Open devuelve error:\n%s", f.todas())
	}
	h3SinHuerfana(t, "item no escribible", f, 0)
	if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: fixture: el item de solo lectura quedo igual:\n%s", ahora)
	}

	restaurar()
	h3Reintento(t, "item no escribible", root, f)
}

// Hallazgo 20260924T200412_c1a9da (CA-347, CA-348), GUARDA: con la sesion
// ya existente Open no escribe nada en el item y no la cierra: created
// false, sin new-session ni kill-session. Vale tambien con el item de solo
// lectura: sin nada que registrar no hay nada que falle.
func TestHallazgo_c1a9da_ConLaSesionExistenteNoEscribeNiCierra(t *testing.T) {
	for _, soloLectura := range []bool{false, true} {
		if soloLectura && os.Geteuid() == 0 {
			continue
		}
		root, _ := cbProyecto(t, true)
		p := item.Path(root, "facturas")
		dir := filepath.Dir(p)
		crudo := cbLeerItem(t, root)
		if soloLectura {
			os.Chmod(p, 0o444)
			os.Chmod(dir, 0o555)
			t.Cleanup(func() { os.Chmod(dir, 0o755); os.Chmod(p, 0o644) })
		}
		f := &h3Tmux{vivas: map[string]bool{h3Sesion: true}}

		s, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, f.deps())
		if err != nil {
			t.Fatalf("CA-348: solo lectura=%v: con la sesion existente Open no falla: %v\n%s", soloLectura, err, f.todas())
		}
		if s.Created || s.Name != h3Sesion {
			t.Fatalf("CA-347: solo lectura=%v: con la sesion existente, created false: %+v", soloLectura, s)
		}
		if f.indice("tmux new-session", 0) >= 0 || f.indice("tmux kill-session", 0) >= 0 || !f.viva(h3Sesion) {
			t.Fatalf("CA-347: solo lectura=%v: con la sesion existente Open ni crea ni cierra:\n%s", soloLectura, f.todas())
		}
		if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
			t.Fatalf("CA-348: solo lectura=%v: con la sesion existente Open no escribe nada en el item:\n%s", soloLectura, ahora)
		}
	}
}
