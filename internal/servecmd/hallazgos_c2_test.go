// Tests de regresion de la review de 4 lentes de la cabina C2 sobre
// .hoom/specs/tablero-de-solo-lectura.md, de punta a punta por el Studio:
//
//   - 20261009T191843_bb986a (high, risk; CA-318 y CA-315): base_branch de
//     hoom.yaml termina como revision en las invocaciones de git de las
//     lecturas del Studio. Con base_branch "--output=<ruta>" ningun GET crea
//     ni trunca un archivo: o el Studio no arranca (cargar ese hoom.yaml es un
//     error que nombra base_branch), o arranca y sus lecturas dejan el disco
//     byte a byte igual. Todas las rutas hostiles viven en t.TempDir().
//   - 20261009T192554_146964 (low, reliability; CA-318, enmienda 1): HEAD se
//     atiende como GET (asi lo hace net/http con toda ruta registrada con
//     GET): 200, sin cuerpo, sin token y sin efectos. 405 es para POST, PUT,
//     PATCH y DELETE.
//
// Y de la review de esas correcciones (tarea fixes-cabina-c2), sobre
// .hoom/specs/historia-doctor-y-cinta.md:
//
//   - 20261009T204930_9fa7ac (medium, risk) y 20261009T205337_394505 (medium,
//     reliability), PRE-EXISTENTES; CA-365 y CA-360: GET
//     /api/board/{slug}/timeline no ejecuta un textconv de la configuracion
//     local de git, y los criterios que le atribuye a cada commit de la tarea
//     salen del contenido real de sus archivos de test.
package servecmd

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const c2Previo = "CONTENIDO PREVIO\n"

// c2LecturasDelTablero son los GET sin token que el refutador vio escribir
// con la base hostil (el tablero, el detalle con y sin diff, y /api/tasks),
// mas la historia de la tarjeta, que lee <base>..HEAD (CA-360).
var c2LecturasDelTablero = []string{
	"/api/board",
	"/api/board/lista",
	"/api/board/lista?diff=1",
	"/api/board/lista/timeline",
	"/api/tasks",
}

// c2ConTarea: el proyecto de C2 con la tarjeta "lista" y su espacio de
// trabajo (.hoom/worktrees/lista, rama hoom/lista) con spec aprobado, test,
// veredicto verde y un commit propio (el fixture de CA-278), para que la foto
// del arbol, el diff y la historia corran git de verdad.
func c2ConTarea(t *testing.T) (dir, wt string) {
	t.Helper()
	dir = tbProyecto(t)
	tbItem(t, dir, "lista", "")
	wt = filepath.Join(dir, ".hoom", "worktrees", "lista")
	gitRun(t, dir, "worktree", "add", "-q", "-b", "hoom/lista", wt, "main")
	tbEscribir(t, wt, ".hoom/specs/lista.md", tbSpecTexto("- CA-70: citado por un test.", true))
	tbEscribir(t, wt, "lista_test.go", "package app\n\n// CA-70\n")
	tbEscribir(t, wt, "lista.go", "package app\n\nfunc Lista() int { return 1 }\n")
	tbAprobar(t, wt, "lista")
	tbVeredicto(t, wt, "lista")
	gitRun(t, wt, "add", "-A")
	gitRun(t, wt, "commit", "-q", "-m", "lista para cerrar")
	return dir, wt
}

// c2Fuera arma el directorio de las victimas, fuera del repo: "x...HEAD" ya
// existe con contenido (si git lo abre para escribir, se trunca) y "x" no
// existe (si git lo abre, aparece). Devuelve el directorio y el base_branch
// hostil que apunta a "x".
func c2Fuera(t *testing.T) (fuera, hostil string) {
	t.Helper()
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	tbEscribir(t, fuera, "x...HEAD", c2Previo)
	return fuera, "--output=" + filepath.Join(fuera, "x")
}

// c2Reponer deja el directorio de las victimas como lo armo c2Fuera, para
// juzgar cada GET por separado.
func c2Reponer(t *testing.T, fuera string) {
	t.Helper()
	ents, err := os.ReadDir(fuera)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if err := os.RemoveAll(filepath.Join(fuera, e.Name())); err != nil {
			t.Fatal(err)
		}
	}
	tbEscribir(t, fuera, "x...HEAD", c2Previo)
}

// c2Cambios dice, ordenado, que cambio entre dos fotos de tbFoto. Vacio si
// el disco quedo igual.
func c2Cambios(antes, despues map[string]string) []string {
	var out []string
	for k, v := range antes {
		if w, ok := despues[k]; !ok {
			out = append(out, "borro "+k)
		} else if w != v {
			out = append(out, "trunco o reescribio "+k)
		}
	}
	for k := range despues {
		if _, ok := antes[k]; !ok {
			out = append(out, "creo "+k)
		}
	}
	sort.Strings(out)
	return out
}

// c2BaseHostil cambia el base_branch del hoom.yaml de dir por el hostil.
func c2BaseHostil(t *testing.T, dir, hostil string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	nuevo := strings.Replace(string(raw), "base_branch: main\n", "base_branch: \""+hostil+"\"\n", 1)
	if nuevo == string(raw) {
		t.Fatal("fixture: el hoom.yaml del proyecto de C2 declara base_branch: main")
	}
	tbEscribir(t, dir, "hoom.yaml", nuevo)
}

// c2LeerSinEscribir pide cada lectura y exige que ni el directorio de las
// victimas ni el proyecto cambien.
func c2LeerSinEscribir(t *testing.T, s *Server, dir, fuera, donde string, lecturas []string) {
	t.Helper()
	for _, path := range lecturas {
		antesFuera, antesRepo := tbFoto(t, fuera), tbFoto(t, dir)
		rec := tbGET(t, s, path)
		if c := c2Cambios(antesFuera, tbFoto(t, fuera)); len(c) != 0 {
			t.Errorf("bb986a CA-318: GET %s (respondio %d) es una lectura: con base_branch \"--output=<tmp>/x\" en %s no crea ni trunca ningun archivo; fuera del repo %s",
				path, rec.Code, donde, strings.Join(c, ", "))
		}
		if c := c2Cambios(antesRepo, tbFoto(t, dir)); len(c) != 0 {
			t.Errorf("bb986a CA-318: GET %s (respondio %d) deja el repo y .hoom/ byte a byte iguales, ignorados incluidos; %s",
				path, rec.Code, strings.Join(c, ", "))
		}
		c2Reponer(t, fuera)
	}
}

// Hallazgo 20261009T191843_bb986a. CA-318 (y CA-315 por el diff): con
// base_branch "--output=<tmp>/x" en el hoom.yaml del proyecto, o el Studio no
// arranca (New falla nombrando base_branch, y no escribio nada), o arranca y
// GET /api/board, GET /api/board/{slug} (con y sin ?diff=1), su historia y
// GET /api/tasks no crean <tmp>/x ni truncan <tmp>/x...HEAD. Fixture de C1:
// lista por CA-278 (tu-aceptacion, con espacio de trabajo).
func TestHallazgo_bb986a_LasLecturasDelStudioNoEscribenConBaseBranchHostil(t *testing.T) {
	dir, _ := c2ConTarea(t)
	fuera, hostil := c2Fuera(t)
	c2BaseHostil(t, dir, hostil)
	antesFuera, antesRepo := tbFoto(t, fuera), tbFoto(t, dir)

	s, err := New(dir)
	if err != nil {
		if !strings.Contains(err.Error(), "base_branch") {
			t.Fatalf("bb986a CA-318: si el Studio rechaza el hoom.yaml, el error nombra base_branch: %v", err)
		}
		if c := c2Cambios(antesFuera, tbFoto(t, fuera)); len(c) != 0 {
			t.Fatalf("bb986a CA-318: rechazar el hoom.yaml no escribe nada; fuera del repo %s", strings.Join(c, ", "))
		}
		if c := c2Cambios(antesRepo, tbFoto(t, dir)); len(c) != 0 {
			t.Fatalf("bb986a CA-318: rechazar el hoom.yaml no toca el proyecto; %s", strings.Join(c, ", "))
		}
		return
	}
	c2LeerSinEscribir(t, s, dir, fuera, "el hoom.yaml del proyecto", c2LecturasDelTablero)
}

// Guarda del hallazgo 20261009T191843_bb986a. CA-318: lo mismo si el
// base_branch hostil no esta en el hoom.yaml del proyecto sino en el que
// commitea la rama de la tarea (lo escribe un agente). El del proyecto es
// valido, asi que el Studio arranca, y el tablero, el detalle con diff y la
// historia siguen sin escribir.
func TestHallazgo_bb986a_GuardaBaseBranchHostilEnElHoomYamlDeLaTarea(t *testing.T) {
	dir, wt := c2ConTarea(t)
	fuera, hostil := c2Fuera(t)
	c2BaseHostil(t, wt, hostil)
	gitRun(t, wt, "add", "-A")
	gitRun(t, wt, "commit", "-q", "-m", "la rama cambia base_branch")

	s := newServer(t, dir)
	c2LeerSinEscribir(t, s, dir, fuera, "el hoom.yaml de la rama de la tarea",
		[]string{"/api/board", "/api/board/lista?diff=1", "/api/board/lista/timeline"})
}

// Hallazgo 20261009T192554_146964 (enmienda 1: se acepta HEAD). CA-318: HEAD
// /api/board y HEAD /api/board/{slug} se atienden como GET: responden 200,
// con el Content-Type del GET, sin cuerpo y sin pedir token, y dejan el repo
// y .hoom/ byte a byte iguales, ignorados incluidos. Los cuatro metodos de
// escritura siguen dando 405, con token o sin el. Va por un servidor HTTP de
// verdad (en loopback): el cuerpo de un HEAD lo quita net/http al responder,
// no el manejador. Este test pasa desde antes de la enmienda: esta para que
// nadie "arregle" HEAD con un 405.
func TestHallazgo_146964_HEADSeAtiendeComoGETSinCuerpoYSinEfectos(t *testing.T) {
	dir, _ := c2ConTarea(t)
	s := newServer(t, dir)
	srv := httptest.NewServer(s.Handler())
	defer srv.Close()
	antes := tbFoto(t, dir)

	pedir := func(metodo, path string) (int, string, []byte) {
		t.Helper()
		req, err := http.NewRequest(metodo, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatalf("146964 CA-318: %s %s: %v", metodo, path, err)
		}
		defer resp.Body.Close()
		cuerpo, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		return resp.StatusCode, resp.Header.Get("Content-Type"), cuerpo
	}
	for _, path := range []string{"/api/board", "/api/board/lista"} {
		codigo, tipo, cuerpo := pedir(http.MethodHead, path)
		if codigo != http.StatusOK {
			t.Fatalf("146964 CA-318: HEAD %s se atiende como GET y sin token: responde 200, respondio %d", path, codigo)
		}
		if len(cuerpo) != 0 {
			t.Fatalf("146964 CA-318: HEAD %s responde sin cuerpo, trajo %d bytes", path, len(cuerpo))
		}
		codigoGET, tipoGET, cuerpoGET := pedir(http.MethodGet, path)
		if codigoGET != http.StatusOK || len(cuerpoGET) == 0 {
			t.Fatalf("146964 CA-318: fixture: GET %s responde 200 con cuerpo: %d, %d bytes", path, codigoGET, len(cuerpoGET))
		}
		if tipo != tipoGET || !strings.Contains(tipo, "application/json") {
			t.Fatalf("146964 CA-318: HEAD %s responde como GET, con su Content-Type (%q): fue %q", path, tipoGET, tipo)
		}
	}
	if c := c2Cambios(antes, tbFoto(t, dir)); len(c) != 0 {
		t.Fatalf("146964 CA-318: HEAD es una lectura: deja el repo y .hoom/ byte a byte iguales, ignorados incluidos; %s", strings.Join(c, ", "))
	}

	// 405 sigue siendo para los cuatro metodos de escritura
	for _, path := range []string{"/api/board", "/api/board/lista"} {
		for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			for _, token := range []string{"", s.Token()} {
				req := httptest.NewRequest(metodo, path, strings.NewReader(`{"slug":"lista","column":"hecho"}`))
				req.Header.Set("Content-Type", "application/json")
				if token != "" {
					req.Header.Set(TokenHeader, token)
				}
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusMethodNotAllowed {
					t.Fatalf("146964 CA-318: %s %s (token=%v) sigue siendo 405, fue %d: %s", metodo, path, token != "", rec.Code, rec.Body.String())
				}
			}
		}
	}
	if c := c2Cambios(antes, tbFoto(t, dir)); len(c) != 0 {
		t.Fatalf("146964 CA-318: un 405 no tiene efectos; %s", strings.Join(c, ", "))
	}
}

// Hallazgos 20261009T204930_9fa7ac y 20261009T205337_394505 (PRE-EXISTENTES).
// historia-doctor-y-cinta CA-365 (GET /api/board/{slug}/timeline es una
// lectura: 200, sin token, y deja el repo y .hoom/ byte a byte iguales) sobre
// los commits de la tarea de CA-360: con un driver diff.<x>.textconv en la
// config local y un .gitattributes de la rama que se lo aplica a los archivos
// de test, pedir la historia no corre el comando del driver ninguna vez (deja
// una marca fuera del proyecto si corre), y el commit que agrego lista_test.go
// enciende CA-70, que es lo que el archivo cita de verdad (el driver imprime
// otro texto, sin ese token).
func TestHallazgo_9fa7ac_GETTimelineNoEjecutaElTextconvDeLaConfigLocal(t *testing.T) {
	dir, wt := c2ConTarea(t)
	tbEscribir(t, wt, ".gitattributes", "*_test.go diff=c2drv\n")
	gitRun(t, wt, "add", "-A")
	gitRun(t, wt, "commit", "-q", "-m", "atributos de diff")

	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marca := filepath.Join(fuera, "el-driver-corrio")
	driver := filepath.Join(fuera, "driver.sh")
	if err := os.WriteFile(driver, []byte("#!/bin/sh\necho corrio >> '"+marca+"'\necho '// CA-99 segun el driver de diff'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "config", "diff.c2drv.textconv", driver)
	// el fixture no es ciego: el git log -p de siempre SI corre el driver
	gitRun(t, wt, "log", "-p", "--no-ext-diff", "main..HEAD", "--", "lista_test.go")
	if err := os.Remove(marca); err != nil {
		t.Fatalf("fixture: en esta maquina git log -p corre el textconv de la config local y deja su marca: %v", err)
	}

	s := newServer(t, dir)
	antes := tbFoto(t, dir)
	rec := tbGET(t, s, "/api/board/lista/timeline")
	if rec.Code != http.StatusOK {
		t.Fatalf("9fa7ac CA-365: GET /api/board/lista/timeline responde 200 sin token, respondio %d: %s", rec.Code, rec.Body.String())
	}
	if raw, err := os.ReadFile(marca); err == nil {
		t.Errorf("9fa7ac CA-365: pedir la historia es una lectura: no ejecuta el textconv de la config local; corrio %d veces",
			strings.Count(string(raw), "corrio"))
	}
	if c := c2Cambios(antes, tbFoto(t, dir)); len(c) != 0 {
		t.Fatalf("9fa7ac CA-365: pedir la historia deja el repo y .hoom/ byte a byte iguales, ignorados incluidos; %s", strings.Join(c, ", "))
	}

	var tl struct {
		Entries []struct {
			Kind  string `json:"kind"`
			Ref   string `json:"ref"`
			Meter []struct {
				ID    string `json:"id"`
				State string `json:"state"`
			} `json:"meter"`
		} `json:"entries"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tl); err != nil {
		t.Fatalf("9fa7ac CA-365: la historia es JSON: %v\n%s", err, rec.Body.String())
	}
	commits, encienden := 0, 0
	for _, e := range tl.Entries {
		if e.Kind != "commit" {
			continue
		}
		commits++
		for _, m := range e.Meter {
			if m.ID == "CA-70" && m.State == "hecho" {
				encienden++
			}
			if m.ID != "CA-70" {
				t.Fatalf("394505 CA-360: un commit de la tarea solo enciende criterios que sus archivos de test citan de verdad (CA-70): el commit %.12s toco %s", e.Ref, m.ID)
			}
		}
	}
	if commits != 2 {
		t.Fatalf("394505 CA-360: fixture: la tarea tiene dos commits (el trabajo y los atributos de diff), la historia trae %d", commits)
	}
	if encienden != 1 {
		t.Fatalf("394505 CA-360: el commit que agrego lista_test.go enciende CA-70 (lo que el archivo cita de verdad, no lo que imprime el driver): lo encendieron %d commits", encienden)
	}
}
