// Tests de regresion de la review de 4 lentes de la cabina C3 sobre
// .hoom/specs/acciones-desde-la-tarjeta.md, por el Studio (CA-345):
//
//   - 20261014T180437_42fa10 y su duplicado 20261014T180753_f2a6d3
//     (Guardar): POST /api/board/{slug}/save llama a la misma funcion que
//     `hoom item save`, que commitea primero el espacio de trabajo y despues
//     el item en el arbol raiz. Si el segundo commit falla (un pre-commit del
//     repo que rechaza los commits con `.hoom/items/`), la respuesta es 409 y
//     su error nombra el commit del espacio de trabajo que SI quedo hecho
//     (los primeros 12 caracteres de su sha), para que la persona sepa que esa
//     parte ya esta guardada. Hoy el 409 solo trae la salida del hook.
//
// Y de la mitad 2 de esa review (el Studio):
//
//   - 20261014T203055_3362b6 (high, risk): el Studio en loopback solo atiende
//     un Host loopback con su puerto. SoloLocal(h, addr) responde 421 sin
//     llamar a h a un Host ajeno (DNS rebinding), de otro puerto o vacio; con
//     un addr que no es loopback devuelve h tal cual. El tablero y el espejo
//     de la terminal no piden token: sin esto otra pagina los leeria.
//   - 20261014T203801_78f960 (medium): el cuerpo estricto de /save, /discard
//     y /launch no acepta nada despues del objeto (`]`, `}` u otro objeto): es
//     400 y no tiene efectos. Espacios o saltos de linea despues si valen.
//   - 20261014T203912_dda1cc (low): si el sobre no tiene donde escribir
//     (.hoom/envelopes es un archivo), pedir-arquitecto desde Backlog responde
//     500 y la tarea NO queda creada.
//   - 20261014T203642_17225a (medium): un trabajo que llega a su primer
//     registro y termina enseguida responde 202, nunca 409.
//   - 20261014T204421_0cf41f (medium, resilience): si el cliente cancela un
//     launch mientras la tarea se crea (un git colgado en worktree add), el
//     handler vuelve enseguida, y cuando ese trabajo termino la tarjeta acepta
//     un lanzamiento nuevo (no queda "ya se esta lanzando").
//   - 20261014T204428_0f7f5c (medium, resilience): con un tmux colgado, Ver
//     terminal responde en a lo sumo unos segundos (el plazo es 5 s) con
//     available false y una nota, y el tmux colgado no queda vivo.
//
// Fixtures de acciones_tarjeta_test.go (atPATH, atConEspacio, atTarjeta,
// atPedir, atCuerpoPaths, atGit, atTmux, atInterrumpida), lanzar_test.go
// (lnPATH, lnServidor, lnLaunch, lnCuerpo, lnHuellas, lnSinEfectos,
// lnEsperarLanzamientos) y tablero_test.go (tbProyecto, tbEscribir, tbItem,
// tbErrorJSON). Los CLIs de IA, tmux y el git lento son falsos.
package servecmd

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// CA-345 (hallazgos 42fa10 y f2a6d3): con las rutas de unsynced y un hook que
// rechaza solo el commit del arbol raiz, el save responde 409 y el error trae
// el sha corto del commit que quedo en el espacio de trabajo.
func TestHallazgo_42fa10_GuardarDesdeLaTarjetaConElCommitDelRaizRechazado(t *testing.T) {
	atPATH(t, nil)
	dir := tbProyecto(t)
	const slug = "a-medias"
	const ca = "CA-345 (hallazgo 42fa10)"
	wt := atConEspacio(t, dir, slug)
	tbEscribir(t, wt, "app.go", "package app\n\nfunc Cambio() int { return 2 }\n")
	tbEscribir(t, wt, "nuevo.go", "package app\n\nfunc Nuevo() {}\n")
	unsynced := atOrdenadas([]string{
		".hoom/items/" + slug + ".yaml",
		".hoom/worktrees/" + slug + "/app.go",
		".hoom/worktrees/" + slug + "/nuevo.go",
	})
	if got := atTarjeta(t, dir, slug).Unsynced; !atMismas(got, unsynced) {
		t.Fatalf("%s: fixture: unsynced de la tarjeta es %v, se esperaba %v", ca, got, unsynced)
	}

	// un pre-commit que rechaza SOLO los commits que traen .hoom/items/ (git y
	// builtins de sh: el PATH no tiene nada mas)
	hooks := filepath.Join(dir, ".git", "hooks")
	gitRun(t, dir, "config", "core.hooksPath", hooks)
	tbEscribir(t, dir, ".git/hooks/pre-commit", `#!/bin/sh
for f in $(git diff --cached --name-only); do
  case "$f" in
  .hoom/items/*)
    echo 'el hook de la casa no deja commitear items' >&2
    exit 1 ;;
  esac
done
exit 0
`)
	if err := os.Chmod(filepath.Join(hooks, "pre-commit"), 0o755); err != nil {
		t.Fatal(err)
	}
	headRaiz := atGit(t, dir, "rev-parse", "HEAD")
	headWt := atGit(t, wt, "rev-parse", "HEAD")

	s := newServer(t, dir)
	rec := atPedir(t, ca, s, http.MethodPost, "/api/board/"+slug+"/save", s.Token(), atCuerpoPaths(unsynced))
	msg := tbErrorJSON(t, ca, rec, http.StatusConflict)

	if h := atGit(t, dir, "rev-parse", "HEAD"); h != headRaiz {
		t.Fatalf("%s: fixture: el hook rechaza el commit del arbol raiz (HEAD %s -> %s)", ca, headRaiz, h)
	}
	hecho := atGit(t, wt, "rev-parse", "HEAD")
	if hecho == headWt {
		t.Fatalf("%s: el espacio de trabajo se guarda primero y su commit queda hecho aunque falle el del raiz (HEAD sigue en %s); 409: %q", ca, headWt, msg)
	}
	if !strings.Contains(msg, hecho[:12]) {
		t.Fatalf("%s: el 409 dice que el espacio de trabajo ya quedo guardado y nombra ese commit (%s): %q", ca, hecho[:12], msg)
	}
}

// ---------------------------------------------------------------------------
// 3362b6: el Studio en loopback solo atiende un Host loopback

// h3Eco es un handler que cuenta los pedidos que le llegan y responde 418.
type h3Eco struct {
	mu sync.Mutex
	n  int
}

func (h *h3Eco) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	h.mu.Lock()
	h.n++
	h.mu.Unlock()
	w.WriteHeader(http.StatusTeapot)
}

func (h *h3Eco) llamadas() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.n
}

// h3ConHost arma un GET sin token con ese Host.
func h3ConHost(ruta, host string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, ruta, nil)
	req.Host = host
	return req
}

// Hallazgo 3362b6 (CA-350: el espejo de la terminal es una lectura sin token
// porque "otra pagina no puede leer sus respuestas"): con un addr loopback,
// SoloLocal deja pasar a h los Host loopback con el puerto del addr y
// responde 421 (sin llamar a h) a un Host ajeno, de otro puerto o vacio. Con
// un addr que no es loopback devuelve h tal cual.
func TestHallazgo_3362b6_SoloLocalAtiendeSoloHostLoopback(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:4666", "localhost:4666"} {
		for _, host := range []string{"127.0.0.1:4666", "localhost:4666", "[::1]:4666"} {
			h := &h3Eco{}
			rec := httptest.NewRecorder()
			SoloLocal(h, addr).ServeHTTP(rec, h3ConHost("/api/board", host))
			if h.llamadas() != 1 || rec.Code != http.StatusTeapot {
				t.Fatalf("3362b6: con addr %s, un pedido con Host %q llega al Studio: llamadas %d, codigo %d", addr, host, h.llamadas(), rec.Code)
			}
		}
		for _, host := range []string{"rebind.atacante.example:4666", "127.0.0.1:9999", ""} {
			h := &h3Eco{}
			rec := httptest.NewRecorder()
			SoloLocal(h, addr).ServeHTTP(rec, h3ConHost("/api/board", host))
			if rec.Code != http.StatusMisdirectedRequest || h.llamadas() != 0 {
				t.Fatalf("3362b6: con addr %s, un pedido con Host %q es 421 y no llega al Studio: codigo %d, llamadas %d", addr, host, rec.Code, h.llamadas())
			}
		}
	}
	for _, addr := range []string{":4666", "0.0.0.0:4666", "192.168.1.5:4666"} {
		h := &h3Eco{}
		got := SoloLocal(h, addr)
		if got != http.Handler(h) {
			t.Fatalf("3362b6: con addr %s (no loopback) SoloLocal devuelve el handler tal cual: %T", addr, got)
		}
		rec := httptest.NewRecorder()
		got.ServeHTTP(rec, h3ConHost("/api/board", "rebind.atacante.example:4666"))
		if h.llamadas() != 1 {
			t.Fatalf("3362b6: con addr %s (no loopback) todo Host llega al Studio: codigo %d", addr, rec.Code)
		}
	}
}

// Hallazgo 3362b6 (CA-350) de punta a punta: el Studio real envuelto en
// SoloLocal con DefaultAddr. Con Host ajeno y sin token, GET del espejo de la
// terminal y de /api/status son 421, y tmux no se entera. Guarda: con Host
// 127.0.0.1:4666 los dos responden 200.
func TestHallazgo_3362b6_ElStudioConHostAjenoEs421(t *testing.T) {
	marcas := t.TempDir()
	log, marca := filepath.Join(marcas, "tmux.log"), filepath.Join(marcas, "sesion")
	atPATH(t, map[string]string{"tmux": atTmux(log, marca, atCapturaColor)})
	dir := tbProyecto(t)
	const slug = "espejo"
	atConEspacio(t, dir, slug)
	if err := os.WriteFile(marca, nil, 0o644); err != nil { // la sesion existe
		t.Fatal(err)
	}
	h := SoloLocal(newServer(t, dir).Handler(), DefaultAddr)
	rutas := []string{"/api/board/" + slug + "/terminal", "/api/status"}

	for _, ruta := range rutas {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, h3ConHost(ruta, "rebind.atacante.example:4666"))
		if rec.Code != http.StatusMisdirectedRequest {
			t.Fatalf("3362b6: GET %s con Host ajeno y sin token es 421, fue %d: %s", ruta, rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "rojo") {
			t.Fatalf("3362b6: el 421 no trae la pantalla de la terminal: %s", rec.Body.String())
		}
	}
	if l := atConVerbo(atLineas(t, log), "capture-pane"); len(l) > 0 {
		t.Fatalf("3362b6: con Host ajeno no se captura la terminal: %q", l)
	}

	for _, ruta := range rutas {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, h3ConHost(ruta, "127.0.0.1:4666"))
		if rec.Code != http.StatusOK {
			t.Fatalf("3362b6: guarda: GET %s con Host 127.0.0.1:4666 responde 200, fue %d: %s", ruta, rec.Code, rec.Body.String())
		}
	}
}

// ---------------------------------------------------------------------------
// 78f960: nada despues del objeto del cuerpo

// Hallazgo 78f960: un cuerpo con algo despues del objeto (`]`, `}` u otro
// objeto) a /save, /discard o /launch es 400 y no tiene efectos, aunque el
// objeto sea uno valido que, solo, haria el commit, el descarte o el
// lanzamiento. Guarda: espacios o saltos de linea despues del objeto valen.
func TestHallazgo_78f960_CuerpoConAlgoDespuesDelObjetoEs400(t *testing.T) {
	const ca = "78f960"
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)

	// guardar: cambios sin guardar en el espacio de trabajo
	wtG := atConEspacio(t, root, "guardable")
	tbEscribir(t, wtG, "app.go", "package app\n\nfunc Cambio() int { return 2 }\n")
	guardar := atTarjeta(t, root, "guardable").Unsynced
	if len(guardar) != 2 {
		t.Fatalf("%s: fixture: guardable tiene el item y app.go sin guardar: %v", ca, guardar)
	}
	// descartar: una tarjeta interrumpida con app.go a medias
	wtD := atConEspacio(t, root, "cortada")
	atInterrumpida(t, root, "cortada")
	tbEscribir(t, wtD, "app.go", "package app\n\n// a medias\n")
	descartar := []string{".hoom/worktrees/cortada/app.go"}
	if a, ok := lnAccion(atTarjeta(t, root, "cortada"), "descartar"); !ok || !a.Enabled {
		t.Fatalf("%s: fixture: cortada tiene descartar habilitada: %+v", ca, a)
	}
	// lanzar: una tarjeta en Backlog
	tbItem(t, root, "nueva", "")
	lnColumna(t, ca, root, "nueva", boardcmd.ColBacklog)
	lanzar, err := json.Marshal(lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec."))
	if err != nil {
		t.Fatal(err)
	}

	s := lnServidor(t, root, f)
	objetos := map[string]string{
		"/api/board/guardable/save":  atCuerpoPaths(guardar),
		"/api/board/cortada/discard": atCuerpoPaths(descartar),
		"/api/board/nueva/launch":    string(lanzar),
	}
	antesGit := atEstadoGit(t, root, wtG, wtD)
	antes := lnHuellas(t, root)

	for ruta, objeto := range objetos {
		cuerpos := []string{`{"paths":[]}]`, `{"paths":["a"]}}`, `{"paths":["a"]}]{"column":"hecho"}`}
		for _, cola := range []string{"]", "}", `]{"column":"hecho"}`, `{"column":"hecho"}`} {
			cuerpos = append(cuerpos, objeto+cola)
		}
		for _, cuerpo := range cuerpos {
			caso := fmt.Sprintf("%s (CA-345, CA-346, CA-338): POST %s con %q", ca, ruta, cuerpo)
			tbErrorJSON(t, caso, atPedir(t, caso, s, http.MethodPost, ruta, s.Token(), cuerpo), http.StatusBadRequest)
		}
	}
	lnEsperarLanzamientos(t, ca, s)
	atMismoGit(t, ca+": un cuerpo con algo despues del objeto", antesGit, root, wtG, wtD)
	lnSinEfectos(t, ca, "un cuerpo con algo despues del objeto", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, ca, "un cuerpo con algo despues del objeto", f)

	// guarda: espacios y saltos de linea despues del objeto valen
	for ruta, quiero := range map[string]int{
		"/api/board/guardable/save":  http.StatusOK,
		"/api/board/cortada/discard": http.StatusOK,
		"/api/board/nueva/launch":    http.StatusAccepted,
	} {
		rec := atPedir(t, ca, s, http.MethodPost, ruta, s.Token(), objetos[ruta]+" \n\t\n")
		if rec.Code != quiero {
			t.Fatalf("%s: guarda: POST %s con espacios y un salto de linea despues del objeto responde %d, fue %d: %s", ca, ruta, quiero, rec.Code, rec.Body.String())
		}
	}
	lnEsperarLanzamientos(t, ca, s)
}

// ---------------------------------------------------------------------------
// dda1cc: sin donde escribir el sobre no se crea la tarea

// Hallazgo dda1cc (CA-336): con .hoom/envelopes existiendo como ARCHIVO, el
// sobre del arquitecto no tiene donde escribir su salida ni su registro.
// pedir-arquitecto desde Backlog responde 500, y la tarea NO queda creada: ni
// .hoom/worktrees/<slug> ni la rama hoom/<slug>, y ningun CLI corre.
func TestHallazgo_dda1cc_SinDondeEscribirElSobreNoCreaLaTarea(t *testing.T) {
	const ca = "dda1cc (CA-336)"
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	tbItem(t, root, "nueva", "")
	lnColumna(t, ca, root, "nueva", boardcmd.ColBacklog)
	tbEscribir(t, root, ".hoom/envelopes", "esto es un archivo, no una carpeta\n")
	s := lnServidor(t, root, f)

	rec := lnLaunch(t, s, "nueva", s.Token(), lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec."))
	tbErrorJSON(t, ca, rec, http.StatusInternalServerError)
	lnEsperarLanzamientos(t, ca, s)

	if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "nueva")); !os.IsNotExist(err) {
		t.Fatalf("%s: sin donde escribir el sobre, la tarea no se crea: existe .hoom/worktrees/nueva (%v)", ca, err)
	}
	if err := exec.Command("git", "-C", root, "rev-parse", "--verify", "--quiet", "refs/heads/hoom/nueva").Run(); err == nil {
		t.Fatalf("%s: sin donde escribir el sobre, la tarea no se crea: existe la rama hoom/nueva", ca)
	}
	lnSinLlamadas(t, ca, "un launch sin donde escribir el sobre", f)
}

// ---------------------------------------------------------------------------
// 17225a: un trabajo que termina enseguida es 202

// Hallazgo 17225a (CA-336 y CA-335): el endpoint responde 202 cuando Started
// se llamo, aunque el trabajo termine enseguida. Con un CLI falso que sale al
// instante, 10 tarjetas en Backlog (con su espacio de trabajo) piden al
// arquitecto a la vez, 10 rondas: los 100 launches son 202, cada 202 nombra un
// sobre distinto con registro, y no queda ningun registro sin su 202.
//
// La carrera (el trabajo llega a Started Y termina antes de que el handler
// mire) depende del planificador: este test la busca con presion, no la
// fuerza.
func TestHallazgo_17225a_UnTrabajoQueTerminaEnseguidaEs202(t *testing.T) {
	const ca = "17225a (CA-336)"
	const tarjetas, rondas = 10, 10
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	var slugs []string
	for i := 0; i < tarjetas; i++ {
		slug := "rapida-" + strconv.Itoa(i)
		tbItem(t, root, slug, "")
		lnTarea(t, ca, root, slug)
		lnColumna(t, ca, root, slug, boardcmd.ColBacklog)
		slugs = append(slugs, slug)
	}
	s := lnServidor(t, root, f)
	raw, err := json.Marshal(lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec."))
	if err != nil {
		t.Fatal(err)
	}

	ids := map[string]bool{}
	for r := 0; r < rondas; r++ {
		recs := make([]*httptest.ResponseRecorder, len(slugs))
		var wg sync.WaitGroup
		for i, slug := range slugs {
			wg.Add(1)
			go func(i int, slug string) {
				defer wg.Done()
				req := httptest.NewRequest(http.MethodPost, "/api/board/"+slug+"/launch", strings.NewReader(string(raw)))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set(TokenHeader, s.Token())
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, req)
				recs[i] = rec
			}(i, slug)
		}
		wg.Wait()
		lnEsperarLanzamientos(t, ca, s)
		for i, rec := range recs {
			lanzado := lnAceptado(t, fmt.Sprintf("%s: ronda %d, %s: un trabajo que llega a su primer registro y termina enseguida", ca, r, slugs[i]), rec)
			if ids[lanzado.EnvelopeID] {
				t.Fatalf("%s: cada 202 nombra un sobre distinto: %s repetido", ca, lanzado.EnvelopeID)
			}
			ids[lanzado.EnvelopeID] = true
			lnRegistro(t, ca, root, lanzado.EnvelopeID)
		}
	}
	if regs := lnRegistros(t, root); len(regs) != len(ids) {
		t.Fatalf("%s: un registro por cada 202 y ninguno mas: %d registros, %d 202", ca, len(regs), len(ids))
	}
}

// ---------------------------------------------------------------------------
// 0cf41f: cancelar el pedido no deja la tarjeta tomada

// h3GitLento instala un git falso al frente del PATH que en `worktree add`
// anota que empezo, tarda 5 s, corre el git de verdad y anota que termino;
// todo lo demas va al git de verdad.
func h3GitLento(t *testing.T, bin, empezo, termino string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(real, bin) {
		t.Fatalf("fixture: el git de verdad no es el falso: %s", real)
	}
	script := strings.NewReplacer("@REAL@", real, "@EMPEZO@", empezo, "@TERMINO@", termino).Replace(`#!/bin/sh
prev=""
for a in "$@"; do
  if [ "$prev" = "worktree" ] && [ "$a" = "add" ]; then
    : > '@EMPEZO@'
    /bin/sleep 5
    '@REAL@' "$@"
    rc=$?
    : > '@TERMINO@'
    exit $rc
  fi
  prev="$a"
done
exec '@REAL@' "$@"
`)
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Hallazgo 0cf41f (CA-336 y CA-340): pedir-arquitecto desde Backlog con un git
// que tarda 5 s en `worktree add`. El cliente cancela el pedido a los 0,3 s
// (con worktree add ya en curso): el handler vuelve en menos de 1 s desde la
// cancelacion, no cuando git termina. Y cuando ese trabajo termino, un
// lanzamiento nuevo sobre la tarjeta no es "ya se esta lanzando".
func TestHallazgo_0cf41f_CancelarElLaunchNoDejaLaTarjetaTomada(t *testing.T) {
	const ca = "0cf41f (CA-340)"
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	tbItem(t, root, "lenta", "")
	lnColumna(t, ca, root, "lenta", boardcmd.ColBacklog)
	marcas := t.TempDir()
	empezo, termino := filepath.Join(marcas, "empezo"), filepath.Join(marcas, "termino")
	h3GitLento(t, f.bin, empezo, termino)
	s := lnServidor(t, root, f)

	raw, err := json.Marshal(lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec."))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/board/lenta/launch", strings.NewReader(string(raw))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(TokenHeader, s.Token())
	rec := httptest.NewRecorder()
	inicio := time.Now()
	volvio := make(chan time.Time, 1)
	go func() {
		s.Handler().ServeHTTP(rec, req)
		volvio <- time.Now()
	}()

	// cancela a los 0,3 s, con worktree add ya en curso
	if !lnEsperar(func() bool { _, err := os.Stat(empezo); return err == nil }) {
		t.Fatalf("%s: fixture: el launch desde Backlog corre git worktree add", ca)
	}
	if espera := 300*time.Millisecond - time.Since(inicio); espera > 0 {
		time.Sleep(espera)
	}
	cancelado := time.Now()
	cancel()
	select {
	case fin := <-volvio:
		if d := fin.Sub(cancelado); d >= time.Second {
			t.Errorf("%s: cancelado el pedido, el handler vuelve en menos de 1 s, volvio a los %v (codigo %d)", ca, d.Round(time.Millisecond), rec.Code)
		}
	case <-time.After(15 * time.Second):
		t.Fatalf("%s: cancelado el pedido, el handler no volvio en 15 s", ca)
	}

	// cuando ese trabajo termino, la tarjeta acepta un lanzamiento nuevo
	if !lnEsperar(func() bool { _, err := os.Stat(termino); return err == nil }) {
		t.Fatalf("%s: fixture: el git lento termina", ca)
	}
	lnEsperarLanzamientos(t, ca, s)
	otra := lnLaunch(t, s, "lenta", s.Token(), lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec."))
	if otra.Code == http.StatusConflict && strings.Contains(otra.Body.String(), "ya se esta lanzando") {
		t.Fatalf("%s: terminado el trabajo del pedido cancelado, la tarjeta no queda tomada: %s", ca, otra.Body.String())
	}
	if otra.Code != http.StatusAccepted {
		t.Logf("%s: el lanzamiento nuevo respondio %d: %s", ca, otra.Code, otra.Body.String())
	}
	lnEsperarLanzamientos(t, ca, s)
}

// ---------------------------------------------------------------------------
// 0f7f5c: Ver terminal con tmux colgado

// h3VivoPID dice si pid sigue vivo (un proceso terminado y esperado no esta).
func h3VivoPID(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}

// Hallazgo 0f7f5c (CA-350): con un tmux que se cuelga (solo en capture-pane,
// con la sesion y su pane respondiendo; o en cualquier verbo), GET
// /api/board/<slug>/terminal responde en menos de 8 s (el plazo es 5 s) con
// 200, available false y una nota, y el tmux colgado no queda vivo.
func TestHallazgo_0f7f5c_VerTerminalConTmuxColgadoNoSeCuelga(t *testing.T) {
	for _, c := range []struct {
		caso    string
		colgado func(log, marca, pids string) string
	}{
		{"solo capture-pane colgado", func(log, marca, pids string) string {
			return atTmux(log, marca, "printf '%s\\n' \"$$\" >> '"+pids+"'\nexec /bin/sleep 20")
		}},
		{"todo tmux colgado", func(log, marca, pids string) string {
			return "printf '%s\\n' \"$*\" >> '" + log + "'\nprintf '%s\\n' \"$$\" >> '" + pids + "'\nexec /bin/sleep 20\n"
		}},
	} {
		t.Run(c.caso, func(t *testing.T) {
			ca := "0f7f5c (CA-350): " + c.caso
			marcas := t.TempDir()
			log, marca, pids := filepath.Join(marcas, "tmux.log"), filepath.Join(marcas, "sesion"), filepath.Join(marcas, "pids")
			atPATH(t, map[string]string{"tmux": c.colgado(log, marca, pids)})
			dir := tbProyecto(t)
			const slug = "colgada"
			atConEspacio(t, dir, slug)
			if err := os.WriteFile(marca, nil, 0o644); err != nil { // la sesion existe
				t.Fatal(err)
			}
			s := newServer(t, dir)

			type respuesta struct {
				rec *httptest.ResponseRecorder
				d   time.Duration
			}
			llego := make(chan respuesta, 1)
			go func() {
				inicio := time.Now()
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/board/"+slug+"/terminal", nil))
				llego <- respuesta{rec, time.Since(inicio)}
			}()
			var r respuesta
			select {
			case r = <-llego:
			case <-time.After(8 * time.Second):
				t.Fatalf("%s: Ver terminal se quedo colgado mas de 8 s (el plazo es 5 s)", ca)
			}
			var term atTerminal
			atOK(t, ca, r.rec, &term)
			if term.Available || term.Text != "" || strings.TrimSpace(term.Note) == "" {
				t.Fatalf("%s: con tmux colgado, available false, sin text y una nota: %+v (a los %v)", ca, term, r.d.Round(time.Millisecond))
			}

			// el tmux colgado no queda vivo
			raw, _ := os.ReadFile(pids)
			for _, l := range strings.Fields(string(raw)) {
				pid, err := strconv.Atoi(l)
				if err != nil {
					t.Fatalf("fixture: pid %q", l)
				}
				if !h3EsperarHasta(3*time.Second, func() bool { return !h3VivoPID(pid) }) {
					t.Fatalf("%s: el tmux colgado (pid %d) sigue vivo despues de la respuesta", ca, pid)
				}
			}
			if len(strings.Fields(string(raw))) == 0 {
				t.Fatalf("%s: fixture: el tmux falso se colgo al menos una vez", ca)
			}
		})
	}
}

// h3EsperarHasta sondea cond hasta d.
func h3EsperarHasta(d time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return cond()
}
