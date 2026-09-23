// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-347, CA-348, CA-350) sobre cockpitcmd: Open arma la sesion de tmux del
// cockpit SIN adjuntarse (Run = Open + attach), al crear la sesion de una
// tarea con item deja al writer declarado en `sesiones`, y Capture es el
// espejo de solo lectura del pane de la IA. Todo contra grabadoras: ningun
// tmux real.
package cockpitcmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
)

// cbGrabadora es la grabadora de CA-82 con Output: registra cada paso (con
// sus argumentos exactos) y contesta las sondas de tmux.
type cbGrabadora struct {
	calls    []call     // RunCmd y QuietCmd, como recorder
	outputs  [][]string // Output: nombre y argumentos
	sesion   bool       // has-session / list-panes encuentran la sesion
	panes    string     // lo que imprime list-panes
	pantalla string     // lo que imprime capture-pane
}

func (g *cbGrabadora) deps(env map[string]string) Deps {
	rec := func(dir, name string, args ...string) error {
		g.calls = append(g.calls, call{dir, name, strings.Join(args, " ")})
		if name == "tmux" && len(args) > 0 && args[0] == "has-session" && !g.sesion {
			return fmt.Errorf("can't find session")
		}
		return nil
	}
	return Deps{
		LookPath: func(f string) (string, error) { return "/usr/bin/" + f, nil },
		RunCmd:   rec,
		QuietCmd: rec,
		Getenv:   func(k string) string { return env[k] },
		Output: func(dir, name string, args ...string) ([]byte, error) {
			g.outputs = append(g.outputs, append([]string{name}, args...))
			if name != "tmux" || len(args) == 0 {
				return nil, fmt.Errorf("sonda inesperada")
			}
			switch args[0] {
			case "has-session", "list-panes":
				if !g.sesion {
					return nil, fmt.Errorf("can't find session")
				}
				if args[0] == "list-panes" {
					return []byte(g.panes), nil
				}
				return nil, nil
			case "capture-pane":
				return []byte(g.pantalla), nil
			}
			return nil, fmt.Errorf("sonda inesperada: %v", args)
		},
		HoomBin: "/opt/fake/bin/hoom",
	}
}

// cbProhibidos son los pasos que Open y Capture nunca dan.
func cbProhibido(g *cbGrabadora, verbos ...string) string {
	for _, c := range g.calls {
		for _, v := range verbos {
			if c.name == "tmux" && strings.HasPrefix(c.args, v) {
				return c.args
			}
		}
	}
	for _, o := range g.outputs {
		for _, v := range verbos {
			if len(o) > 1 && o[1] == v {
				return strings.Join(o, " ")
			}
		}
	}
	return ""
}

// cbProyecto arma un proyecto git (identidad configurada) con el worktree de
// la tarea facturas y, si conItem, su item en el arbol raiz.
func cbProyecto(t *testing.T, conItem bool) (root, wt string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "init", "-b", "main")
	gitRun(t, root, "config", "user.email", "test@hoom.dev")
	gitRun(t, root, "config", "user.name", "hoom test")
	wt = filepath.Join(root, ".hoom", "worktrees", "facturas")
	if err := os.MkdirAll(wt, 0o755); err != nil {
		t.Fatal(err)
	}
	if conItem {
		p := item.Path(root, "facturas")
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		body := "titulo: Facturas\ntipo: feature\nprioridad: media\n" +
			"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-20T10:00:00Z\n"
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, wt
}

func cbLeerItem(t *testing.T, root string) []byte {
	t.Helper()
	raw, err := os.ReadFile(item.Path(root, "facturas"))
	if err != nil {
		t.Fatalf("CA-348: el item sigue en el arbol raiz: %v", err)
	}
	return raw
}

// cbArbol fotografia todos los archivos bajo root (fuera de .git).
func cbArbol(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			return filepath.SkipDir
		}
		if !d.IsDir() {
			raw, _ := os.ReadFile(p)
			rel, _ := filepath.Rel(root, p)
			out[rel] = string(raw)
		}
		return nil
	})
	return out
}

// CA-347: SessionName es el nombre de siempre: hoom-<proyecto>[-<tarea>].
func TestCA347_SessionName(t *testing.T) {
	for _, c := range []struct{ project, task, want string }{
		{"demo", "", "hoom-demo"},
		{"Mi Proyecto", "", "hoom-mi-proyecto"},
		{"demo", "facturas", "hoom-demo-facturas"},
		{"hoomai", "acciones-desde-la-tarjeta", "hoom-hoomai-acciones-desde-la-tarjeta"},
	} {
		if got := SessionName(c.project, c.task); got != c.want {
			t.Fatalf("CA-347: SessionName(%q, %q) = %q, no %q", c.project, c.task, got, c.want)
		}
	}
}

// CA-347: Open arma con tmux el plan de CA-82 (la CLI de la IA y el watch
// con el binario absoluto, en el proyecto) SIN attach-session ni
// switch-client, y dice que creo la sesion.
func TestCA347_OpenArmaSinAdjuntarse(t *testing.T) {
	root := t.TempDir()
	g := &cbGrabadora{}
	s, err := Open(root, "demo", Options{Provider: "claude"}, g.deps(nil))
	if err != nil {
		t.Fatalf("CA-347: Open: %v", err)
	}
	if !hasCall(g.calls, "tmux", "new-session -d -s hoom-demo -c "+root+" claude") {
		t.Fatalf("CA-347: Open crea la sesion con la CLI de la IA en el proyecto: %+v", g.calls)
	}
	if !hasCall(g.calls, "tmux", "split-window") || !hasCall(g.calls, "tmux", "'/opt/fake/bin/hoom' status --watch") {
		t.Fatalf("CA-347: Open arma el pane del watch con el binario absoluto: %+v", g.calls)
	}
	if v := cbProhibido(g, "attach-session", "switch-client", "attach"); v != "" {
		t.Fatalf("CA-347: Open no se adjunta: %q (%+v)", v, g.calls)
	}
	want := Session{Name: "hoom-demo", Created: true, Provider: "claude", Dir: ".", Attach: "tmux attach -t hoom-demo"}
	if s != want {
		t.Fatalf("CA-347: Open devuelve %+v, no %+v", s, want)
	}

	// dentro de tmux tampoco: jamas switch-client
	g = &cbGrabadora{}
	if _, err := Open(root, "demo", Options{Provider: "claude"}, g.deps(map[string]string{"TMUX": "/tmp/sock,1,0"})); err != nil {
		t.Fatalf("CA-347: %v", err)
	}
	if v := cbProhibido(g, "attach-session", "switch-client", "attach"); v != "" {
		t.Fatalf("CA-347: Open dentro de tmux tampoco se adjunta: %q", v)
	}
}

// CA-347: con --task los panes viven en el worktree (CA-85), y la sesion
// nombra su dir relativo a la raiz.
func TestCA347_OpenDeUnaTarea(t *testing.T) {
	root, wt := cbProyecto(t, false)
	g := &cbGrabadora{}
	s, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, g.deps(nil))
	if err != nil {
		t.Fatalf("CA-347: Open: %v", err)
	}
	if !hasCall(g.calls, "tmux", "new-session -d -s hoom-demo-facturas -c "+wt+" claude") {
		t.Fatalf("CA-347: los panes viven en el worktree: %+v", g.calls)
	}
	want := Session{Name: "hoom-demo-facturas", Created: true, Provider: "claude",
		Dir: ".hoom/worktrees/facturas", Attach: "tmux attach -t hoom-demo-facturas"}
	if s != want {
		t.Fatalf("CA-347: Open de la tarea devuelve %+v, no %+v", s, want)
	}

	// una tarea que no existe: el error de siempre, y ningun proceso
	g = &cbGrabadora{}
	if _, err := Open(root, "demo", Options{Provider: "claude", Task: "no-existe"}, g.deps(nil)); err == nil ||
		!strings.Contains(err.Error(), "hoom task start no-existe") {
		t.Fatalf("CA-347: una tarea inexistente es el error de CA-85: %v", err)
	}
	if hasCall(g.calls, "tmux", "new-session") {
		t.Fatalf("CA-347: sin tarea no se crea nada: %+v", g.calls)
	}
}

// CA-347: con la sesion existente Open devuelve created false y no crea nada
// (ni panes nuevos ni attach).
func TestCA347_OpenConLaSesionExistente(t *testing.T) {
	g := &cbGrabadora{sesion: true}
	s, err := Open(t.TempDir(), "demo", Options{Provider: "claude"}, g.deps(nil))
	if err != nil {
		t.Fatalf("CA-347: Open: %v", err)
	}
	if s.Created || s.Name != "hoom-demo" || s.Attach != "tmux attach -t hoom-demo" {
		t.Fatalf("CA-347: con la sesion existente, created false y el mismo nombre: %+v", s)
	}
	if v := cbProhibido(g, "new-session", "split-window", "attach-session", "switch-client", "kill-session"); v != "" {
		t.Fatalf("CA-347: con la sesion existente Open no crea ni se adjunta: %q", v)
	}
}

// CA-347: sin tmux Open es un error y no lanza nada.
func TestCA347_OpenSinTmux(t *testing.T) {
	g := &cbGrabadora{}
	deps := g.deps(nil)
	deps.LookPath = func(f string) (string, error) {
		if f == "tmux" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + f, nil
	}
	if _, err := Open(t.TempDir(), "demo", Options{Provider: "claude"}, deps); err == nil {
		t.Fatal("CA-347: sin tmux Open es un error")
	}
	if len(g.calls) != 0 {
		t.Fatalf("CA-347: sin tmux no se lanza ningun proceso: %+v", g.calls)
	}
}

// CA-347: Run es Open mas el attach: los mismos pasos, en el mismo orden, y
// despues attach-session (o switch-client dentro de tmux).
func TestCA347_RunEsOpenMasAttach(t *testing.T) {
	for _, c := range []struct {
		env    map[string]string
		attach string
	}{
		{nil, "attach-session -t hoom-demo"},
		{map[string]string{"TMUX": "/tmp/sock,1,0"}, "switch-client -t hoom-demo"},
	} {
		root := t.TempDir()
		gOpen := &cbGrabadora{}
		if _, err := Open(root, "demo", Options{Provider: "claude"}, gOpen.deps(c.env)); err != nil {
			t.Fatalf("CA-347: Open: %v", err)
		}
		gRun := &cbGrabadora{}
		if err := Run(root, "demo", Options{Provider: "claude"}, gRun.deps(c.env)); err != nil {
			t.Fatalf("CA-347: Run: %v", err)
		}
		if len(gRun.calls) != len(gOpen.calls)+1 {
			t.Fatalf("CA-347: Run da los pasos de Open y uno mas:\nOpen %+v\nRun  %+v", gOpen.calls, gRun.calls)
		}
		if !reflect.DeepEqual(gRun.calls[:len(gOpen.calls)], gOpen.calls) {
			t.Fatalf("CA-347: Run empieza con los mismos pasos que Open:\nOpen %+v\nRun  %+v", gOpen.calls, gRun.calls)
		}
		if last := gRun.calls[len(gRun.calls)-1]; last.name != "tmux" || last.args != c.attach {
			t.Fatalf("CA-347: el ultimo paso de Run es %q: %+v", c.attach, last)
		}
	}
}

// CA-348: cuando Open CREA la sesion de una tarea con item, el item del
// arbol raiz gana una entrada en sesiones: provider, abierta_por (la
// identidad git del proyecto) y abierta_en. Las demas claves no cambian.
func TestCA348_OpenRegistraLaSesionEnElItem(t *testing.T) {
	root, _ := cbProyecto(t, true)
	antes, err := item.Load(root, "facturas")
	if err != nil {
		t.Fatalf("CA-348: fixture: el item es valido: %v", err)
	}
	g := &cbGrabadora{}
	desde := time.Now().Add(-2 * time.Second)
	s, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, g.deps(nil))
	hasta := time.Now().Add(2 * time.Second)
	if err != nil || !s.Created {
		t.Fatalf("CA-348: Open crea la sesion: %+v %v", s, err)
	}
	it, err := item.Load(root, "facturas")
	if err != nil {
		t.Fatalf("CA-348: el item con sesiones sigue siendo valido: %v\n%s", err, cbLeerItem(t, root))
	}
	if len(it.Sesiones) != 1 {
		t.Fatalf("CA-348: el item gana UNA sesion: %+v", it.Sesiones)
	}
	ses := it.Sesiones[0]
	if ses.Provider != "claude" {
		t.Fatalf("CA-348: la sesion nombra su provider: %+v", ses)
	}
	if want := gitx.Identity(root); ses.AbiertaPor != want || want == "desconocido" {
		t.Fatalf("CA-348: abierta_por es gitx.Identity del proyecto (%q): %+v", want, ses)
	}
	if ses.AbiertaEn.IsZero() || ses.AbiertaEn.Before(desde) || ses.AbiertaEn.After(hasta) {
		t.Fatalf("CA-348: abierta_en es cuando se abrio: %v", ses.AbiertaEn)
	}
	if it.Titulo != antes.Titulo || it.Tipo != antes.Tipo || it.Prioridad != antes.Prioridad ||
		it.CreadoPor != antes.CreadoPor || !it.CreadoEn.Equal(antes.CreadoEn) {
		t.Fatalf("CA-348: las demas claves del item no cambian: %+v vs %+v", it, antes)
	}

	// la segunda vez la sesion ya existe: created false y el item queda igual
	crudo := cbLeerItem(t, root)
	g = &cbGrabadora{sesion: true}
	s, err = Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, g.deps(nil))
	if err != nil || s.Created {
		t.Fatalf("CA-348: con la sesion abierta, created false: %+v %v", s, err)
	}
	if ahora := cbLeerItem(t, root); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: con la sesion existente no se registra nada:\nantes:\n%s\ndespues:\n%s", crudo, ahora)
	}
}

// CA-348: sin item (o sin tarea) Open no escribe ningun item.
func TestCA348_SinItemNoEscribeNada(t *testing.T) {
	root, _ := cbProyecto(t, false)
	antes := cbArbol(t, root)
	g := &cbGrabadora{}
	if s, err := Open(root, "demo", Options{Provider: "claude", Task: "facturas"}, g.deps(nil)); err != nil || !s.Created {
		t.Fatalf("CA-348: Open crea la sesion de una tarea sin item: %+v %v", s, err)
	}
	if _, err := os.Stat(item.Path(root, "facturas")); err == nil {
		t.Fatal("CA-348: sin item, Open no crea uno")
	}
	if despues := cbArbol(t, root); !reflect.DeepEqual(antes, despues) {
		t.Fatalf("CA-348: sin item, Open no escribe nada en el proyecto:\nantes %v\ndespues %v", antes, despues)
	}

	// la sesion del proyecto (sin tarea) tampoco toca un item
	root2, _ := cbProyecto(t, true)
	crudo := cbLeerItem(t, root2)
	g = &cbGrabadora{}
	if _, err := Open(root2, "demo", Options{Provider: "claude"}, g.deps(nil)); err != nil {
		t.Fatalf("CA-348: %v", err)
	}
	if ahora := cbLeerItem(t, root2); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: la sesion del proyecto no registra nada en el item de una tarea:\n%s", ahora)
	}
}

// CA-348: `hoom cockpit --task` con tmux (Run) tambien declara el writer, y
// con zellij (que no pasa por Open) no.
func TestCA348_RunConTmuxRegistraYConZellijNo(t *testing.T) {
	root, _ := cbProyecto(t, true)
	g := &cbGrabadora{}
	if err := Run(root, "demo", Options{Provider: "claude", Task: "facturas"}, g.deps(nil)); err != nil {
		t.Fatalf("CA-348: Run: %v", err)
	}
	it, err := item.Load(root, "facturas")
	if err != nil || len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "claude" {
		t.Fatalf("CA-348: hoom cockpit --task con tmux registra la sesion como el Studio: %+v %v", it.Sesiones, err)
	}

	root2, _ := cbProyecto(t, true)
	crudo := cbLeerItem(t, root2)
	g = &cbGrabadora{}
	if err := Run(root2, "demo", Options{Provider: "claude", Task: "facturas", Mux: "zellij"}, g.deps(nil)); err != nil {
		t.Fatalf("CA-348: Run con zellij: %v", err)
	}
	if ahora := cbLeerItem(t, root2); !bytes.Equal(ahora, crudo) {
		t.Fatalf("CA-348: zellij no pasa por Open y no registra nada:\n%s", ahora)
	}
}

// CA-350: Capture lee el primer pane que lista tmux (el de la IA) y devuelve
// lo que tmux ya pinto, con sus secuencias de color, sin crear nada ni
// mandar teclas.
func TestCA350_CaptureLeeElPrimerPane(t *testing.T) {
	root, _ := cbProyecto(t, true)
	antes := cbArbol(t, root)
	pantalla := "\x1b[1;32mverde\x1b[0m y \x1b[31mrojo\x1b[0m\n$ hoom verify\n"
	g := &cbGrabadora{sesion: true, panes: "%3\n%7\n", pantalla: pantalla}

	term, err := Capture(root, "demo", "facturas", g.deps(nil))
	if err != nil {
		t.Fatalf("CA-350: Capture: %v", err)
	}
	want := Terminal{Available: true, Session: "hoom-demo-facturas", Pane: "%3", Text: pantalla, Note: ""}
	if term != want {
		t.Fatalf("CA-350: Capture devuelve %+v, no %+v", term, want)
	}
	lista, captura := -1, -1
	for i, o := range g.outputs {
		switch {
		case reflect.DeepEqual(o, []string{"tmux", "list-panes", "-t", "=hoom-demo-facturas", "-F", "#{pane_id}"}):
			lista = i
		case reflect.DeepEqual(o, []string{"tmux", "capture-pane", "-p", "-e", "-t", "%3"}):
			captura = i
		}
	}
	if lista < 0 || captura < 0 || lista > captura {
		t.Fatalf("CA-350: primero `tmux list-panes -t =<sesion> -F #{pane_id}` y despues `tmux capture-pane -p -e -t <pane>`: %v", g.outputs)
	}
	if v := cbProhibido(g, "send-keys", "new-session", "split-window", "kill-session", "attach-session", "switch-client"); v != "" {
		t.Fatalf("CA-350: Capture es una lectura: nunca %q", v)
	}
	for _, c := range g.calls {
		if c.name == "tmux" && !strings.HasPrefix(c.args, "has-session") {
			t.Fatalf("CA-350: Capture solo sondea: %+v", g.calls)
		}
	}
	if despues := cbArbol(t, root); !reflect.DeepEqual(antes, despues) {
		t.Fatalf("CA-350: Capture no escribe nada:\nantes %v\ndespues %v", antes, despues)
	}
}

// CA-350: el texto se corta en TerminalMaxBytes (256 KiB).
func TestCA350_CaptureCortaEn256KiB(t *testing.T) {
	if TerminalMaxBytes != 256*1024 {
		t.Fatalf("CA-350: el tope del espejo es 256 KiB: %d", TerminalMaxBytes)
	}
	root, _ := cbProyecto(t, false)
	linea := strings.Repeat("x", 99) + "\n"
	grande := strings.Repeat(linea, (300*1024)/len(linea))
	g := &cbGrabadora{sesion: true, panes: "%3\n", pantalla: grande}
	term, err := Capture(root, "demo", "facturas", g.deps(nil))
	if err != nil {
		t.Fatalf("CA-350: Capture: %v", err)
	}
	if !term.Available || len(term.Text) > TerminalMaxBytes || len(term.Text) < TerminalMaxBytes-len(linea) {
		t.Fatalf("CA-350: el texto se corta en %d bytes: disponible %v, %d bytes", TerminalMaxBytes, term.Available, len(term.Text))
	}
	if !strings.Contains(grande, term.Text) {
		t.Fatal("CA-350: el texto cortado es un pedazo de la pantalla, sin inventar nada")
	}
}

// CA-350: sin tmux, available false con la nota `tmux no esta instalado`;
// sin la sesion, `no hay una sesion abierta para esta tarjeta`. Ninguno
// escribe nada ni es un error.
func TestCA350_CaptureSinTmuxOSinSesion(t *testing.T) {
	root, _ := cbProyecto(t, true)
	antes := cbArbol(t, root)

	g := &cbGrabadora{sesion: true, panes: "%3\n", pantalla: "no deberia leerse"}
	deps := g.deps(nil)
	deps.LookPath = func(f string) (string, error) {
		if f == "tmux" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + f, nil
	}
	term, err := Capture(root, "demo", "facturas", deps)
	if err != nil {
		t.Fatalf("CA-350: sin tmux Capture no es un error (el endpoint responde 200): %v", err)
	}
	if term.Available || term.Text != "" || term.Note != "tmux no esta instalado" {
		t.Fatalf("CA-350: sin tmux, available false, text vacio y la nota \"tmux no esta instalado\": %+v", term)
	}
	if len(g.outputs) != 0 || len(g.calls) != 0 {
		t.Fatalf("CA-350: sin tmux no se corre nada: %v %+v", g.outputs, g.calls)
	}

	g = &cbGrabadora{sesion: false}
	term, err = Capture(root, "demo", "facturas", g.deps(nil))
	if err != nil {
		t.Fatalf("CA-350: sin sesion Capture no es un error (el endpoint responde 200): %v", err)
	}
	if term.Available || term.Text != "" || term.Note != "no hay una sesion abierta para esta tarjeta" {
		t.Fatalf("CA-350: sin sesion, available false, text vacio y la nota \"no hay una sesion abierta para esta tarjeta\": %+v", term)
	}
	for _, o := range g.outputs {
		if len(o) > 1 && o[1] == "capture-pane" {
			t.Fatalf("CA-350: sin sesion no hay pane que capturar: %v", g.outputs)
		}
	}
	if v := cbProhibido(g, "new-session", "split-window", "send-keys"); v != "" {
		t.Fatalf("CA-350: sin sesion Capture no la crea: %q", v)
	}
	if despues := cbArbol(t, root); !reflect.DeepEqual(antes, despues) {
		t.Fatalf("CA-350: Capture no escribe nada:\nantes %v\ndespues %v", antes, despues)
	}
}
