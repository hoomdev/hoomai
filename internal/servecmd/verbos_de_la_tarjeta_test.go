// Tests adversariales del spec .hoom/specs/verbos-de-la-tarjeta.md en el
// Studio: el dialogo de rol con Esfuerzo, Modelo relleno y el tiempo limite
// para las cuatro acciones (CA-496), POST /launch que pasa effort y los
// valores de hoom.yaml (CA-496, CA-492), /api/board con model, effort y
// timeout_s (CA-492), la misma funcion de lanzamiento que la terminal con el
// mismo motivo (CA-504) y el candado de archivo entre procesos (CA-505).
// Los CLIs son los falsos de lanzar_test.go, en un PATH minimo.
package servecmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

// vdtTerminal corre un verbo de la tarjeta desde root, sin terminal. Un
// panico es una falla del criterio.
func vdtTerminal(t *testing.T, ca, root string, verb string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	e := tarjetacmd.Entorno{Dir: root, In: strings.NewReader(""), Out: &out, Err: &errb,
		Getenv: func(string) string { return "" }}
	code := -1
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("%s: hoom %s %q entro en panico: %v", ca, verb, args, r)
			}
		}()
		code = tarjetacmd.Run(verb, args, e)
	}()
	return code, out.String(), errb.String()
}

// vdtMotivoStudio es el motivo de un launch rechazado (un 4xx con
// {"error": ...}).
func vdtMotivoStudio(t *testing.T, ca string, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code < 400 || rec.Code >= 500 {
		t.Fatalf("%s: el launch se rechaza con un 4xx, fue %d: %s", ca, rec.Code, rec.Body.String())
	}
	var e map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e["error"] == "" {
		t.Fatalf("%s: el cuerpo es {\"error\": ...}: %q", ca, rec.Body.String())
	}
	return e["error"]
}

// ===========================================================================
// CA-496: el dialogo y POST /launch
// ===========================================================================

// CA-496: el campo Esfuerzo es de todo dialogo de rol (no solo del
// reviewer), Modelo y Esfuerzo vienen rellenos con model y effort de la
// accion, la linea del tiempo limite sale de timeout_s y el POST lleva effort
// para toda accion de rol. El Studio no edita el tiempo limite.
func TestCA496_DialogoConEsfuerzoModeloYTiempoLimite(t *testing.T) {
	_, _, acc := atUI(t)
	if !strings.Contains(acc, `<label for="acc-effort">Esfuerzo</label>`) {
		t.Fatal(`CA-496: el dialogo tiene <label for="acc-effort">Esfuerzo</label>`)
	}
	if regexp.MustCompile("revision\\s*\\?\\s*`[^`]*acc-effort").MatchString(acc) {
		t.Fatal("CA-496: el campo Esfuerzo ya no depende de que la accion sea la revision")
	}
	if regexp.MustCompile(`effort\s*:\s*revision\b`).MatchString(acc) {
		t.Fatal("CA-496: el POST lleva effort para toda accion de rol, no solo para la revision")
	}
	for _, quiero := range []string{"a.model", "a.effort", "a.timeout_s", "tiempo limite: ", " min (el de hoom)"} {
		if !strings.Contains(acc, quiero) {
			t.Fatalf("CA-496: acciones.js contiene %q", quiero)
		}
	}
	if regexp.MustCompile(`<input[^>]*id="acc-timeout"`).MatchString(acc) {
		t.Fatal("CA-496: el Studio no edita el tiempo limite: se cambia en hoom.yaml")
	}
	if atPalabrasDeGit.MatchString("tiempo limite: 30 min (el de hoom)") {
		t.Fatal("CA-496: la linea del tiempo limite sigue la regla de las palabras")
	}
}

// CA-496: POST /launch con effort lo pasa al sobre en pedir-writer,
// pedir-test-writer y pedir-arquitecto.
func TestCA496_LaunchPasaEffortATodoRol(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-496", root, "a-implementar", "")
	lnCartaTestWriter(t, "CA-496", root, "faltan-tests")
	tbItem(t, root, "sin-spec", "")
	lnColumna(t, "CA-496", root, "sin-spec", boardcmd.ColBacklog)
	s := lnServidor(t, root, f)

	for _, k := range []struct{ slug, action, pedido string }{
		{"a-implementar", "pedir-writer", "Implementa el spec hasta que verify de verde."},
		{"faltan-tests", "pedir-test-writer", "Escribi los tests del criterio que falta."},
		{"sin-spec", "pedir-arquitecto", "Escribi el spec de la tarjeta."},
	} {
		rec := lnLaunch(t, s, k.slug, s.Token(), raCuerpo(k.action, "claude", "", k.pedido, "high"))
		if strings.Contains(rec.Body.String(), "effort solo aplica a pedir-reviewer") {
			t.Fatalf("CA-496: el error 'effort solo aplica a pedir-reviewer' desaparece: %s", rec.Body.String())
		}
		lnAceptado(t, "CA-496 ("+k.action+")", rec)
		lnEsperarLanzamientos(t, "CA-496", s)
	}
	ll := f.llamadas(t, "claude")
	if len(ll) != 3 {
		t.Fatalf("CA-496: tres lanzamientos, tres llamadas a claude: %d", len(ll))
	}
	for _, args := range ll {
		if v, ok := lnSigue(args, "--effort"); !ok || v != "high" {
			t.Fatalf("CA-496: el esfuerzo pedido llega al provider (--effort high): %q", args)
		}
	}
}

// CA-496, CA-492 y CA-491: sin modelo ni esfuerzo en el cuerpo, el launch
// toma los de agents.<rol> del hoom.yaml del proyecto, y su tiempo limite
// llega al sidecar del run.
func TestCA496_LaunchSinValoresTomaLosDeHoomYaml(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := raProyectoConReview(t, "agents:\n  writer:\n    model: m-yaml\n    effort: low\n    timeout: 45m\n")
	lnCartaWriter(t, "CA-496", root, "a-implementar", "")
	s := lnServidor(t, root, f)
	lnAceptado(t, "CA-496", lnLaunch(t, s, "a-implementar", s.Token(),
		raCuerpo("pedir-writer", "claude", "", "Implementa el spec hasta que verify de verde.", "")))
	lnEsperarLanzamientos(t, "CA-496", s)
	ll := f.llamadas(t, "claude")
	if len(ll) != 1 {
		t.Fatalf("CA-496: un lanzamiento: %d", len(ll))
	}
	if v, _ := lnSigue(ll[0], "--model"); v != "m-yaml" {
		t.Fatalf("CA-491: sin modelo en el dialogo, el de agents.writer.model: %q", ll[0])
	}
	if v, _ := lnSigue(ll[0], "--effort"); v != "low" {
		t.Fatalf("CA-491: sin esfuerzo en el dialogo, el de agents.writer.effort: %q", ll[0])
	}
	metas := runcmd.Metas(root)
	if len(metas) != 1 || metas[0].TimeoutS != 2700 {
		t.Fatalf("CA-494: el run del Studio lleva el tiempo limite de hoom.yaml (timeout_s 2700): %+v", metas)
	}
}

// ===========================================================================
// CA-492: /api/board
// ===========================================================================

// CA-492: /api/board emite model, effort y timeout_s en toda accion, con
// los valores de hoom.yaml en la de su rol.
func TestCA492_ApiBoardEmiteLosValores(t *testing.T) {
	root := raProyectoConReview(t, "agents:\n  writer:\n    model: m-yaml\n    effort: low\n    timeout: 45m\n")
	lnCartaWriter(t, "CA-492", root, "a-implementar", "")
	s := newServer(t, root)
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-492: GET /api/board es 200: %d %s", rec.Code, rec.Body.String())
	}
	var b struct {
		Columns []struct {
			Cards []map[string]any `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("CA-492: /api/board es JSON: %v", err)
	}
	vista := false
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			acts, _ := c["actions"].([]any)
			for _, x := range acts {
				a, _ := x.(map[string]any)
				for _, k := range []string{"model", "effort", "timeout_s"} {
					if _, ok := a[k]; !ok {
						t.Fatalf("CA-492: la accion %v de %v no trae %q en /api/board", a["id"], c["slug"], k)
					}
				}
				if c["slug"] == "a-implementar" && a["id"] == boardcmd.ActPedirWriter {
					vista = true
					if a["model"] != "m-yaml" || a["effort"] != "low" || a["timeout_s"] != float64(2700) {
						t.Fatalf("CA-492: pedir-writer trae los valores de agents.writer: %v", a)
					}
				}
			}
		}
	}
	if !vista {
		t.Fatal("CA-492: fixture: la tarjeta a-implementar tiene pedir-writer en /api/board")
	}
}

// ===========================================================================
// CA-504: la misma funcion para el Studio y la terminal
// ===========================================================================

// CA-504: para los mismos estados de la tarjeta, POST /launch y hoom pedir
// dan el mismo motivo, sin efectos ni llamadas.
func TestCA504_MismoMotivoEnLasDosPieles(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	f.cli(t, "opencode", false)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-504", root, "agotada", "presupuesto_usd: 1\n")
	wtA := filepath.Join(root, ".hoom", "worktrees", "agotada")
	lnGasto(t, "CA-504", root, "agotada", wtA, 2)
	lnCartaWriter(t, "CA-504", root, "sin-provider", "")
	lnCartaWriter(t, "CA-504", root, "poca-plata", "presupuesto_usd: 5\n")
	wtP := filepath.Join(root, ".hoom", "worktrees", "poca-plata")
	c := 3.0
	lnSidecar(t, root, runcmd.Meta{ID: "20260923T060000_gasto3", Provider: "claude", Role: "writer", Task: "poca-plata", Dir: wtP,
		CreatedAt: time.Now().UTC().Add(-time.Hour), Status: runcmd.StatusDone, EndedAt: time.Now().UTC().Add(-50 * time.Minute),
		Usage: &providers.Usage{CostUSD: &c, Turns: 3}})
	wtO := lnCartaWriter(t, "CA-504", root, "ocupada", "")
	lnSidecar(t, root, runcmd.Meta{ID: "20260923T070000_activo", Provider: "claude", Dir: wtO,
		CreatedAt: time.Now().UTC().Add(-time.Minute), Status: runcmd.StatusRunning, ExitCode: -1, PID: os.Getpid()})
	lnCartaWriter(t, "CA-504", root, "relleno", "")
	s := lnServidor(t, root, f)

	casos := []struct {
		nombre, slug, provider, pedido string
		budget                         any
		flags                          []string
		contiene                       string
	}{
		{"deshabilitada (presupuesto agotado)", "agotada", "claude", "Implementa el spec.", nil, nil, "se agoto el presupuesto"},
		{"provider que no puede", "sin-provider", "opencode", "Implementa el spec.", nil, []string{"--provider", "opencode"}, "opencode"},
		{"presupuesto que no alcanza", "poca-plata", "claude", "Implementa el spec.", 10.0, []string{"--budget-usd", "10"}, "presupuesto"},
		{"run activo", "ocupada", "claude", "Implementa el spec.", nil, nil, "ya hay un run activo (20260923T070000_activo) en el espacio de trabajo de la tarjeta: espera a que termine"},
		{"pedido de relleno", "relleno", "claude", "<pedido>", nil, nil, `el pedido "<pedido>" no dice nada: escribi lo que el rol tiene que hacer`},
	}
	for _, k := range casos {
		antes := lnHuellas(t, root)
		studio := vdtMotivoStudio(t, "CA-504 ("+k.nombre+")", lnLaunch(t, s, k.slug, s.Token(),
			lnCuerpo("pedir-writer", k.provider, "", k.budget, k.pedido)))
		args := append([]string{"writer", k.slug, "-m", k.pedido}, k.flags...)
		code, _, errT := vdtTerminal(t, "CA-504", root, "pedir", args...)
		if code != 1 {
			t.Fatalf("CA-504 (%s): hoom pedir sale con 1, fue %d: %s", k.nombre, code, errT)
		}
		if !strings.Contains(studio, k.contiene) {
			t.Fatalf("CA-504 (%s): el motivo dice %q: %q", k.nombre, k.contiene, studio)
		}
		if !strings.Contains(errT, "hoom: "+studio) {
			t.Fatalf("CA-504 (%s): la terminal da el mismo motivo que el Studio %q:\n%s", k.nombre, studio, errT)
		}
		lnEsperarLanzamientos(t, "CA-504", s)
		lnSinEfectos(t, "CA-504", k.nombre, antes, lnHuellas(t, root), false)
	}
	lnSinLlamadas(t, "CA-504", "un lanzamiento rechazado", f)
}

// CA-504: sin la accion, cada piel dice su texto del contrato (el Studio,
// que la tarjeta ya no esta donde se la vio; la terminal, que en esa columna
// no se le pide al rol), sin efectos.
func TestCA504_SinLaAccion(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-504", root, "en-writer", "")
	c := lnCarta(t, root, "en-writer")
	s := lnServidor(t, root, f)
	antes := lnHuellas(t, root)
	rec := lnLaunch(t, s, "en-writer", s.Token(), lnCuerpo("pedir-test-writer", "claude", "", nil, "Escribi los tests."))
	if got := vdtMotivoStudio(t, "CA-504", rec); got != lnYaNoEsta(c) {
		t.Fatalf("CA-504: el Studio sin la accion dice %q: %q", lnYaNoEsta(c), got)
	}
	code, _, errT := vdtTerminal(t, "CA-504", root, "pedir", "test-writer", "en-writer", "-m", "Escribi los tests.")
	want := "hoom: en " + c.ColumnName + " no se le pide al test-writer: " + c.Plain + ". Siguiente: hoom siguiente"
	if code != 1 || !strings.Contains(errT, want) {
		t.Fatalf("CA-504: la terminal sin la accion sale con 1 y %q: %d\n%s", want, code, errT)
	}
	lnSinEfectos(t, "CA-504", "sin la accion", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-504", "sin la accion", f)
}

// CA-504: en Backlog las dos pieles crean la tarea, y el lanzamiento del
// Studio escribe el log del sobre.
func TestCA504_BacklogCreaLaTareaEnLasDosPieles(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	tbItem(t, root, "desde-studio", "")
	tbItem(t, root, "desde-terminal", "")
	lnColumna(t, "CA-504", root, "desde-studio", boardcmd.ColBacklog)
	lnColumna(t, "CA-504", root, "desde-terminal", boardcmd.ColBacklog)
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-504", lnLaunch(t, s, "desde-studio", s.Token(),
		lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec de la tarjeta.")))
	lnEsperarLanzamientos(t, "CA-504", s)
	if !r.TaskStarted {
		t.Fatalf("CA-504: el Studio en Backlog crea la tarea: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "envelopes", r.EnvelopeID+".log")); err != nil {
		t.Fatalf("CA-504: el lanzamiento del Studio escribe el log del sobre: %v", err)
	}

	vdtTerminal(t, "CA-504", root, "pedir", "arquitecto", "desde-terminal", "-m", "Escribi el spec de la tarjeta.")
	for _, slug := range []string{"desde-studio", "desde-terminal"} {
		if fi, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", slug)); err != nil || !fi.IsDir() {
			t.Fatalf("CA-504: en Backlog se crea la tarea de %s: %v", slug, err)
		}
	}
	if n := len(f.llamadas(t, "claude")); n != 2 {
		t.Fatalf("CA-504: un arquitecto por piel: %d", n)
	}
}

// ===========================================================================
// CA-505: el candado de la tarjeta es de archivo
// ===========================================================================

// TestVdtAyudanteCandado es el proceso ayudante de CA-505: con
// VDT_CANDADO_LOCK toma ese candado de archivo, avisa en VDT_CANDADO_LISTO y
// lo suelta cuando aparece VDT_CANDADO_SOLTAR. Sin esas variables no hace
// nada.
func TestVdtAyudanteCandado(t *testing.T) {
	lock := os.Getenv("VDT_CANDADO_LOCK")
	if lock == "" {
		return
	}
	soltar, err := hoomfs.Lock(lock, 0)
	if err != nil {
		t.Fatalf("ayudante: no pude tomar el candado: %v", err)
	}
	defer soltar()
	if err := os.WriteFile(os.Getenv("VDT_CANDADO_LISTO"), []byte("ya\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(60 * time.Second)
	for time.Now().Before(limite) {
		if _, err := os.Stat(os.Getenv("VDT_CANDADO_SOLTAR")); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// vdtOtroProceso toma el candado de la tarjeta desde otro proceso y
// devuelve con que soltarlo.
func vdtOtroProceso(t *testing.T, root, slug string) func() {
	t.Helper()
	dir := t.TempDir()
	listo, soltar := filepath.Join(dir, "listo"), filepath.Join(dir, "soltar")
	cmd := exec.Command(os.Args[0], "-test.run=^TestVdtAyudanteCandado$", "-test.count=1")
	cmd.Env = append(os.Environ(), "VDT_CANDADO_LOCK="+lanzar.Candado(root, slug),
		"VDT_CANDADO_LISTO="+listo, "VDT_CANDADO_SOLTAR="+soltar)
	var salida bytes.Buffer
	cmd.Stdout, cmd.Stderr = &salida, &salida
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	limite := time.Now().Add(30 * time.Second)
	for {
		if _, err := os.Stat(listo); err == nil {
			break
		}
		if time.Now().After(limite) {
			cmd.Process.Kill()
			t.Fatalf("CA-505: fixture: el otro proceso no tomo el candado:\n%s", salida.String())
		}
		time.Sleep(20 * time.Millisecond)
	}
	hecho := false
	suelta := func() {
		if hecho {
			return
		}
		hecho = true
		os.WriteFile(soltar, []byte("ya\n"), 0o644)
		cmd.Wait()
	}
	t.Cleanup(suelta)
	return suelta
}

// CA-505: con el candado de archivo de la tarjeta tomado en este proceso,
// POST /launch es 409 con MsgLanzando, sin efectos.
func TestCA505_CandadoDeArchivoEnElStudio(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-505", root, "a-implementar", "")
	s := lnServidor(t, root, f)
	soltar, err := hoomfs.Lock(lanzar.Candado(root, "a-implementar"), 0)
	if err != nil {
		t.Fatal(err)
	}
	antes := lnHuellas(t, root)
	msg := tbErrorJSON(t, "CA-505", lnLaunch(t, s, "a-implementar", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec.")), http.StatusConflict)
	soltar()
	if msg != lanzar.MsgLanzando {
		t.Fatalf("CA-505: el Studio responde %q: %q", lanzar.MsgLanzando, msg)
	}
	lnEsperarLanzamientos(t, "CA-505", s)
	lnSinEfectos(t, "CA-505", "el candado tomado", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-505", "el candado tomado", f)
}

// CA-505: con otro proceso lanzando sobre la tarjeta (su candado de
// archivo tomado), el Studio y hoom pedir reciben MsgLanzando; soltado, el
// Studio lanza.
func TestCA505_OtroProcesoTieneElCandado(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-505", root, "a-implementar", "")
	s := lnServidor(t, root, f)
	suelta := vdtOtroProceso(t, root, "a-implementar")

	antes := lnHuellas(t, root)
	msg := tbErrorJSON(t, "CA-505", lnLaunch(t, s, "a-implementar", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec.")), http.StatusConflict)
	if msg != lanzar.MsgLanzando {
		t.Fatalf("CA-505: con otro proceso lanzando el Studio responde %q: %q", lanzar.MsgLanzando, msg)
	}
	code, _, errT := vdtTerminal(t, "CA-505", root, "pedir", "writer", "a-implementar", "-m", "Implementa el spec.")
	if code != 1 || !strings.Contains(errT, "hoom: "+lanzar.MsgLanzando) {
		t.Fatalf("CA-505: con otro proceso lanzando hoom pedir sale con 1 y %q: %d\n%s", lanzar.MsgLanzando, code, errT)
	}
	lnSinEfectos(t, "CA-505", "otro proceso lanzando", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-505", "otro proceso lanzando", f)

	suelta()
	lnAceptado(t, "CA-505", lnLaunch(t, s, "a-implementar", s.Token(),
		lnCuerpo("pedir-writer", "claude", "", nil, "Implementa el spec.")))
	lnEsperarLanzamientos(t, "CA-505", s)
}
