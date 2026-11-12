// Tests de regresion de la ronda 2 de autonomia-entre-firmas, como
// consumidor externo (proyectos git reales, CLIs de IA falsos en un PATH
// minimo, nunca uno real):
//
//   - problema A, hallazgos 20261112T185129_ea20f8, 20261112T185626_e49304 y
//     20261112T190139_84d9de (CA-638, CA-652, aclaracion 18): el writer que
//     toco un test protegido y vencio no se releva aunque una persona cierre
//     el hallazgo de tests-del-test-writer;
//   - problema B, hallazgo 20261112T185130_272465 (CA-644, aclaracion 19): el
//     tope del guardado termina todo el comando, hijos incluidos (con un
//     filtro clean de git desde la aclaracion 21);
//   - problema C, hallazgo 20261112T185130_5774db (CA-656, aclaracion 20):
//     cerrar-hallazgos vuelve a mirar la tarjeta bajo sus candados.
//
// Reusa los helpers de las rondas 0 y 1 (apRonda, apr1Correr, apr1Fake...).
package pilotocmd_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agentcmd"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
)

const apr2PreHallazgo = "tests-del-test-writer: el rol writer escribio precios_test.go"

// apr2WriterVencido arma la tarjeta del test de 24e6e0: un test-writer real
// (CLI falso) protege precios_test.go, y un writer lanzado como por el piloto
// (Pilot, Columna writer, regla cadena, Prompt, Timeout 3 s) lo cambia y
// muere por su timeout. Al final el claude falso es uno que termina enseguida.
func apr2WriterVencido(t *testing.T, ca string, f *apFalsos) (root, wt, slug string) {
	t.Helper()
	root = apProyecto(t, "")
	apr1Gate(t, root, "grep -q implementado app.go") // rojo hasta que haya codigo: la tarjeta queda en Writer
	slug = "relevo"
	apItem(t, root, slug, apConAuto)
	wt = apTarea(t, root, slug)
	apEscribir(t, wt, apSpecRel(slug), apSpecTexto("- CA-1: el total redondea bien."))
	apFirmar(t, wt, slug)
	apCommit(t, wt, "spec aprobado")
	apColumna(t, ca, root, slug, boardcmd.ColTestWriter)

	apr1Fake(t, f, "claude", "printf 'package app\\n\\n// CA-1: el total redondea bien\\n' > precios_test.go\nexit 0\n")
	tw := apr1Correr(t, root, agentcmd.Options{Role: "test-writer", Provider: "claude", Task: slug, Prompt: "escribi los tests de CA-1"})
	if rec := apRegistro(t, ca, root, tw.EnvelopeID); !reflect.DeepEqual(rec.Trasplantados, []string{"precios_test.go"}) {
		t.Fatalf("%s: fixture: el test-writer protege precios_test.go: %+v %+v", ca, rec, tw)
	}
	apGit(t, wt, "add", "-A")
	apGit(t, wt, "commit", "-q", "--allow-empty", "-m", apGuardaMs+slug)
	apColumna(t, ca, root, slug, boardcmd.ColWriter)

	apr1Fake(t, f, "claude", "printf 'package app\\n\\n// CA-1: el total redondea bien, segun el writer\\n' > precios_test.go\nexec sleep 60\n")
	desde := time.Now()
	w := apr1Correr(t, root, agentcmd.Options{Role: "writer", Provider: "claude", Task: slug, Prompt: "implementa el spec",
		Timeout: 3 * time.Second, Pilot: true, Columna: boardcmd.ColWriter, Regla: boardcmd.ReglaCadena})
	if time.Since(desde) > 45*time.Second || w.Status == envelope.StatusDeliverable || w.ExitCode == 0 {
		t.Fatalf("%s: fixture: el writer muere por su timeout de 3 s sin entregar: %+v (%s)", ca, w, time.Since(desde))
	}
	if raw, _ := os.ReadFile(filepath.Join(wt, "precios_test.go")); !strings.Contains(string(raw), "segun el writer") {
		t.Fatalf("%s: fixture: el writer cambio el test protegido antes de morir: %q", ca, raw)
	}
	f.cli(t, "claude") // si el piloto lo releva, este claude termina enseguida
	return root, wt, slug
}

// apr2HallazgoAbierto es el hallazgo abierto high de tests-del-test-writer
// del writer, y el arbol donde esta.
func apr2HallazgoAbierto(t *testing.T, dirs ...string) (finding.Item, string, bool) {
	t.Helper()
	for _, d := range dirs {
		items, _, err := finding.List(d, apBase, true)
		if err != nil {
			continue
		}
		for _, it := range items {
			if it.Status == finding.StatusOpen && it.Severity == "high" && strings.HasPrefix(it.Description, apr2PreHallazgo) {
				return it, d, true
			}
		}
	}
	return finding.Item{}, "", false
}

// Problema A, hallazgos 20261112T185129_ea20f8, 20261112T185626_e49304 y
// 20261112T190139_84d9de (CA-638, CA-652, aclaracion 18): la variante del
// test de 24e6e0 con el hallazgo high de tests-del-test-writer de ese sobre
// cerrado por una persona (como corregido y como refutado) antes de correr
// el piloto. El piloto se detiene igual con el Why del caso 8 y exit 1: no
// lanza ningun rol, no crea sobre y no imprime la regla relevo-por-tiempo.
func TestHallazgosEa20f8E49304_84d9de_CA638_CA652_CerrarElHallazgoNoHabilitaElRelevo(t *testing.T) {
	for _, as := range []string{finding.StatusCorrected, finding.StatusRefuted} {
		ca := "ea20f8/e49304/84d9de/CA-638/CA-652 (" + as + ")"
		f := apPATH(t)
		root, wt, slug := apr2WriterVencido(t, ca, f)
		h, dir, ok := apr2HallazgoAbierto(t, wt, root)
		if !ok {
			t.Fatalf("%s: fixture: el sobre del writer dejo abierto el hallazgo high que empieza con %q", ca, apr2PreHallazgo)
		}
		if _, err := finding.Resolve(dir, h.ID, as, "lo revise a mano: el cambio del test era lo que habia que hacer",
			"hoom test <test@hoom.dev>"); err != nil {
			t.Fatalf("%s: fixture: una persona cierra %s como %s: %v", ca, h.ID, as, err)
		}
		if _, _, abierto := apr2HallazgoAbierto(t, wt, root); abierto {
			t.Fatalf("%s: fixture: el hallazgo %s quedo cerrado", ca, h.ID)
		}

		claude, sobres := f.llamadas(t, "claude"), len(envelope.List(root))
		s := apRun(t, ca, root, slug)
		ls := apSoloLineasDelPiloto(t, ca, s)
		for _, l := range ls {
			if strings.Contains(l, " pide al ") || strings.HasPrefix(l, apPrefijo+"regla ") || strings.Contains(l, boardcmd.ReglaRelevoPorTiempo) ||
				strings.Contains(l, "no pudo lanzar") {
				t.Fatalf("%s: con el hallazgo cerrado, el piloto no releva ni repite al writer que toco un test protegido: %q\n%s", ca, l, s)
			}
		}
		want := apDetiene + "el trabajo del writer no cerro entregable (" + envelope.StatusNotDeliverable + "): el piloto se detiene ante un rojo"
		if s.code != 1 || apUltima(ls) != want {
			t.Fatalf("%s: se detiene con el Why del caso 8, %q, y exit 1:\n%s", ca, want, s)
		}
		if n := f.llamadas(t, "claude"); n != claude {
			t.Fatalf("%s: ningun rol corre: claude corrio %d veces (antes %d)", ca, n, claude)
		}
		if n := len(envelope.List(root)); n != sobres {
			t.Fatalf("%s: ningun sobre nuevo: %d -> %d", ca, sobres, n)
		}
	}
}

// Problema B, hallazgo 20261112T185130_272465 (CA-644, aclaracion 19): con
// algo que no termina en un comando git del guardado, cambios sin guardar y
// itemcmd.TopeGit en 1 s, el guardado de la tarjeta (itemcmd.SaveTarea)
// devuelve un error sin commitear, y para entonces no queda vivo ningun
// proceso del comando: un rato despues nada escribio en el worktree (el
// archivo marca que el proceso, o su hijo, escribiria a los 3 s si siguiera
// vivo) y no hay index.lock. Tres variantes: uno que solo duerme, uno que
// ignora la interrupcion y uno que deja un hijo en segundo plano dentro de su
// grupo.
//
// Re-expresado por la aclaracion 21 (el guardado del piloto no corre hooks):
// lo que no termina ya no es un hook pre-commit sino un filtro clean de git
// configurado en el repo de la tarea para app.go (apr3Filtro), con las
// mismas tres variantes. Las exigencias son las mismas.
func TestHallazgo272465_CA644_ElTopeDelGuardadoTerminaTodoElHook(t *testing.T) {
	const ca = "272465/CA-644"
	tope := itemcmd.TopeGit
	itemcmd.TopeGit = time.Second
	t.Cleanup(func() { itemcmd.TopeGit = tope })

	for _, c := range []struct {
		nombre string
		cuerpo func(marca, pids string) string
	}{
		{"un filtro que solo duerme", func(marca, pids string) string {
			return "sleep 3\n: > '" + marca + "'\n"
		}},
		{"un filtro que ignora la interrupcion", func(marca, pids string) string {
			return "trap '' HUP INT TERM QUIT\nsleep 3\n: > '" + marca + "'\n"
		}},
		{"un filtro que deja un hijo en segundo plano", func(marca, pids string) string {
			return "( sleep 3; : > '" + marca + "' ) &\necho $! >> '" + pids + "'\nexec sleep 30\n"
		}},
	} {
		f := apPATH(t)
		root := apProyecto(t, "")
		const slug = "hook-colgado"
		wt := apTuAprobacion(t, root, slug, apConAuto, true)
		lock := apGit(t, wt, "rev-parse", "--git-path", "index.lock")
		if !filepath.IsAbs(lock) {
			lock = filepath.Join(wt, lock)
		}
		aparte := t.TempDir()
		pids, arranco := filepath.Join(aparte, "pids"), filepath.Join(aparte, "arranco")
		marca := filepath.Join(wt, "marca-del-hook")
		// aclaracion 21: antes, un hook pre-commit con este mismo cuerpo
		apr3Filtro(t, wt, "#!/bin/sh\necho $$ >> '"+pids+"'\n: > '"+arranco+"'\n"+c.cuerpo(marca, pids))
		apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
		head := apHead(t, wt)
		t.Cleanup(func() {
			raw, _ := os.ReadFile(pids)
			for _, l := range strings.Fields(string(raw)) {
				if pid, err := strconv.Atoi(l); err == nil {
					if pr, err := os.FindProcess(pid); err == nil {
						_ = pr.Kill()
					}
				}
			}
		})

		desde := time.Now()
		type guardado struct {
			res itemcmd.SaveResult
			err error
		}
		listo := make(chan guardado, 1)
		go func() {
			res, err := itemcmd.SaveTarea(root, apBase, apBlockOn, slug)
			listo <- guardado{res, err}
		}()
		var g guardado
		select {
		case g = <-listo:
		case <-time.After(30 * time.Second):
			t.Fatalf("%s: %s: con TopeGit en 1 s el guardado termina en un tiempo acotado; a los 30 s seguia", ca, c.nombre)
		}
		if g.err == nil {
			t.Fatalf("%s: %s: el guardado vencido por TopeGit devuelve un error: %+v", ca, c.nombre, g.res)
		}
		if !apr1Existe(arranco) {
			t.Fatalf("%s: %s: fixture: el guardado corrio el filtro clean de app.go (%v)", ca, c.nombre, g.err)
		}
		if h := apHead(t, wt); h != head {
			t.Fatalf("%s: %s: el guardado que fallo no commitea: HEAD %s -> %s", ca, c.nombre, head, h)
		}
		// el filtro (o su hijo) escribiria la marca a los 3 s de arrancar
		for time.Since(desde) < 7*time.Second {
			if apr1Existe(marca) {
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		if apr1Existe(marca) {
			t.Fatalf("%s: %s: despues del error no queda vivo ningun proceso del filtro: algo escribio %s en el worktree (error: %v)", ca, c.nombre, marca, g.err)
		}
		if apr1Existe(lock) {
			t.Fatalf("%s: %s: despues del error no queda %s (error: %v)", ca, c.nombre, lock, g.err)
		}
		f.ninguna(t, ca, "el guardado de la tarjeta")
	}
}

// Problema C, hallazgo 20261112T185130_5774db (CA-656, aclaracion 20): el
// piloto decidio cerrar los hallazgos de la ronda; justo cuando el paso toma
// sus candados de lanzamiento (el candado en proceso de lanzar.Params.Candado,
// que lanzar.Reservar toma antes que el de archivo), la persona quita el
// piloto automatico del item (lo que deja `hoom item set --auto no` en el
// principal, sin guardar). El paso se detiene: no cierra ningun hallazgo
// (siguen abiertos), no emite veredicto nuevo y no crea commits.
func TestHallazgo5774db_CA656_CerrarHallazgosVuelveAMirarLaTarjeta(t *testing.T) {
	const ca = "5774db/CA-656"
	f := apPATH(t)
	fx := apRonda(t, "antes", false)
	headWt, headRoot := apHead(t, fx.wt), apHead(t, fx.root)
	item := filepath.Join(fx.root, ".hoom", "items", fx.slug+".yaml")

	var mu sync.Mutex
	var tomas []string
	var errApagar error
	candado := func(slug string) (func(), bool) {
		mu.Lock()
		defer mu.Unlock()
		tomas = append(tomas, slug)
		if len(tomas) == 1 {
			errApagar = os.WriteFile(item, []byte(apItemTexto(fx.slug, apSinAuto)), 0o644)
		}
		return func() {}, true
	}
	var out bytes.Buffer
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	res, err := pilotocmd.Correr(ctx, pilotocmd.Opciones{Root: fx.root, Slug: fx.slug, Lanzar: lanzar.Params{Candado: candado},
		Salida: &out, Proveedores: apNingunProveedor})
	mu.Lock()
	n, errA := len(tomas), errApagar
	mu.Unlock()
	if errA != nil {
		t.Fatalf("%s: fixture: la persona quita el auto del item: %v", ca, errA)
	}
	if n == 0 {
		t.Fatalf("%s: fixture: el paso toma el candado de lanzamiento en proceso (lanzar.Params.Candado); no lo tomo: %v %+v\n%s", ca, err, res, out.String())
	}
	if err != nil {
		t.Fatalf("%s: el paso se detiene (una detencion, no un error de armado): %v\n%s", ca, err, out.String())
	}
	cierre := apPrefijo + "cierra "
	for _, l := range res.Lineas {
		if strings.HasPrefix(l, cierre) {
			t.Fatalf("%s: sin auto, el paso no cierra hallazgos: %q\n%q", ca, l, res.Lineas)
		}
	}
	if !strings.HasPrefix(apUltima(res.Lineas), apDetiene) {
		t.Fatalf("%s: el piloto se detiene: %q", ca, res.Lineas)
	}
	apAbierto(t, ca, fx, fx.f1)
	apAbierto(t, ca, fx, fx.f2)
	if nuevos := apNuevos(t, fx.wt, fx.veredictos); len(nuevos) != 0 {
		t.Fatalf("%s: sin veredicto nuevo: %d", ca, len(nuevos))
	}
	if h, r := apHead(t, fx.wt), apHead(t, fx.root); h != headWt || r != headRoot {
		t.Fatalf("%s: sin commits: HEAD de la tarea %s -> %s, del principal %s -> %s", ca, headWt, h, headRoot, r)
	}
	if st := apGit(t, fx.wt, "status", "--porcelain"); st != "" {
		t.Fatalf("%s: el espacio de trabajo queda como estaba: %q", ca, st)
	}
	if n := len(envelope.List(fx.root)); n != 6 {
		t.Fatalf("%s: ningun trabajo nuevo: %d sobres", ca, n)
	}
	f.ninguna(t, ca, "cerrar-hallazgos sin auto")
}
