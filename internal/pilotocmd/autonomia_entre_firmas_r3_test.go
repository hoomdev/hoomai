// Tests de regresion de la ronda 3 de autonomia-entre-firmas, como
// consumidor externo (proyectos git reales, CLIs de IA falsos en un PATH
// minimo, nunca uno real):
//
//   - hallazgos 20261112T194857_ce5f39 y 20261112T195146_5c3780 (CA-647,
//     CA-656, aclaracion 22): si al volver a mirar la tarjeta cerrar-hallazgos
//     la encuentra en Hecho o en una columna humana, el piloto se detiene con
//     exit 0 sin cerrar nada;
//   - hallazgos 20261112T194423_54fa4e y 20261112T195251_ccf2ee (CA-644):
//     fijaba la aclaracion 21 (el guardado no corre hooks); la aclaracion 23
//     lo re-expresa: un hook del guardado que no termina se corta.
//
// Tambien los helpers del filtro clean de git con el que se prueba el tope
// del guardado en los tests de 8c4407 (ronda 1) y 272465 (ronda 2).
package pilotocmd_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/pilotocmd"
)

// apr3Filtro configura en el repo de la tarea un filtro clean de git para
// app.go que corre script: guarda en la tarea su .gitattributes y deja el
// filtro en la config del repo. Despues, todo `git add` de un app.go cambiado
// corre script.
func apr3Filtro(t *testing.T, wt, script string) {
	t.Helper()
	apEscribir(t, wt, ".gitattributes", "app.go filter=hoom-colgado\n")
	apCommit(t, wt, "atributos del filtro")
	ruta := filepath.Join(t.TempDir(), "filtro.sh")
	if err := os.WriteFile(ruta, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	apGit(t, wt, "config", "filter.hoom-colgado.clean", "sh '"+ruta+"'")
}

// apr3SinFiltro quita el filtro de la config, para que los comandos git del
// test no lo corran.
func apr3SinFiltro(t *testing.T, wt string) {
	t.Helper()
	apGit(t, wt, "config", "--unset", "filter.hoom-colgado.clean")
}

// apr3Matar mata los procesos cuyos pids quedaron en el archivo pids.
func apr3Matar(pids string) {
	raw, _ := os.ReadFile(pids)
	for _, l := range strings.Fields(string(raw)) {
		if pid, err := strconv.Atoi(l); err == nil {
			if pr, err := os.FindProcess(pid); err == nil {
				_ = pr.Kill()
			}
		}
	}
}

// Hallazgos 20261112T194857_ce5f39 y 20261112T195146_5c3780 (CA-647,
// CA-656, aclaracion 22): el piloto decidio cerrar los hallazgos de la ronda
// y, justo cuando el paso toma sus candados de lanzamiento (el candado en
// proceso de lanzar.Params.Candado, la ventana del test de 5774db), la
// tarjeta pasa a Hecho (la persona marca el item como hecho en el principal,
// item.MarkDone) o a una columna humana (la persona cambia el spec firmado y
// la tarjeta vuelve a Tu aprobacion). pilotocmd.Correr se detiene como en el
// caso 3, con exit 0: no cierra ningun hallazgo, no emite veredicto y no
// commitea.
func TestHallazgosCe5f39_5c3780_CA647_CA656_SiLaTarjetaPasaAUnaColumnaHumanaSaleCon0(t *testing.T) {
	for _, c := range []struct {
		nombre, col string
		mover       func(fx apRondaFix, commit string) error
		sucio       bool // la persona deja el spec cambiado sin guardar en la tarea
	}{
		{"la persona marca el item como hecho", boardcmd.ColHecho, func(fx apRondaFix, commit string) error {
			_, err := item.MarkDone(fx.root, fx.slug, commit, time.Now().UTC())
			return err
		}, false},
		{"la persona cambia el spec firmado", boardcmd.ColTuAprobacion, func(fx apRondaFix, _ string) error {
			p := filepath.Join(fx.wt, filepath.FromSlash(apSpecRel(fx.slug)))
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			return os.WriteFile(p, []byte(strings.Replace(string(raw), "## Riesgos\nx\n", "## Riesgos\nx, y uno que agrego la persona\n", 1)), 0o644)
		}, true},
	} {
		ca := "ce5f39/5c3780/CA-647/CA-656 (" + c.nombre + ")"
		f := apPATH(t)
		fx := apRonda(t, "antes", false)
		headWt, headRoot := apHead(t, fx.wt), apHead(t, fx.root)

		var mu sync.Mutex
		tomas := 0
		var errMover error
		candado := func(slug string) (func(), bool) {
			mu.Lock()
			defer mu.Unlock()
			tomas++
			if tomas == 1 {
				errMover = c.mover(fx, headWt)
			}
			return func() {}, true
		}
		var out bytes.Buffer
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		res, err := pilotocmd.Correr(ctx, pilotocmd.Opciones{Root: fx.root, Slug: fx.slug, Lanzar: lanzar.Params{Candado: candado},
			Salida: &out, Proveedores: apNingunProveedor})
		cancel()
		mu.Lock()
		n, errM := tomas, errMover
		mu.Unlock()
		if errM != nil {
			t.Fatalf("%s: fixture: %v", ca, errM)
		}
		if n == 0 {
			t.Fatalf("%s: fixture: el paso toma el candado de lanzamiento en proceso (lanzar.Params.Candado); no lo tomo: %v %+v\n%s", ca, err, res, out.String())
		}
		if card, cerr := boardcmd.CardFor(fx.root, apBase, apBlockOn, fx.slug, time.Now().UTC()); cerr != nil || card.Column != c.col {
			t.Fatalf("%s: fixture: la tarjeta quedo en %s: %q %v", ca, c.col, card.Column, cerr)
		}
		if err != nil {
			t.Fatalf("%s: el paso se detiene (una detencion, no un error de armado): %v\n%s", ca, err, out.String())
		}
		for _, l := range res.Lineas {
			if strings.HasPrefix(l, apPrefijo+"cierra ") {
				t.Fatalf("%s: en %s el paso no cierra hallazgos: %q\n%q", ca, c.col, l, res.Lineas)
			}
		}
		if res.ExitCode != 0 || !strings.HasPrefix(apUltima(res.Lineas), apDetiene) {
			t.Fatalf("%s: en %s se detiene como en el caso 3, con exit 0: exit %d %q", ca, c.col, res.ExitCode, res.Lineas)
		}
		apAbierto(t, ca, fx, fx.f1)
		apAbierto(t, ca, fx, fx.f2)
		if nuevos := apNuevos(t, fx.wt, fx.veredictos); len(nuevos) != 0 {
			t.Fatalf("%s: sin veredicto nuevo: %d", ca, len(nuevos))
		}
		if h, r := apHead(t, fx.wt), apHead(t, fx.root); h != headWt || r != headRoot {
			t.Fatalf("%s: sin commits: HEAD de la tarea %s -> %s, del principal %s -> %s", ca, headWt, h, headRoot, r)
		}
		want := ""
		if c.sucio {
			want = "M " + apSpecRel(fx.slug)
		}
		if st := strings.TrimSpace(apGit(t, fx.wt, "status", "--porcelain")); st != want {
			t.Fatalf("%s: el espacio de trabajo queda como lo dejo la persona (%q): %q", ca, want, st)
		}
		if n := len(envelope.List(fx.root)); n != 6 {
			t.Fatalf("%s: ningun trabajo nuevo: %d sobres", ca, n)
		}
		f.ninguna(t, ca, "cerrar-hallazgos en "+c.col)
	}
}

// Hallazgos 20261112T194423_54fa4e y 20261112T195251_ccf2ee (CA-644).
//
// Re-expresado por la aclaracion 23, que reemplaza la 21: el guardado del
// piloto vuelve a correr los hooks de git, asi que lo que fijaba este test
// (que los hooks no corren) ya no vale y el nombre cambia con lo que fija.
// Ahora fija lo que la aclaracion 23 dice de un hook que no termina: con los
// cuatro hooks instalados (pre-commit, prepare-commit-msg, commit-msg y
// post-commit) colgados y cambios sin guardar en Tu aprobacion, `hoom piloto`
// corre el pre-commit, termina en un tiempo acotado (TopeGit en 2 s), sale
// con exit 1 y `no pudo guardar el trabajo de la tarjeta: <error>`, no
// commitea, suelta su candado y no queda vivo ningun proceso del grupo del
// hook: la marca que el hook escribiria en el worktree a los 4 s no aparece.
// La variante del hijo que el hook saca de su grupo con setsid se quito: la
// aclaracion 23 dice que no se fija ni en un sentido ni en el otro.
func TestHallazgos54fa4e_ccf2ee_CA644_UnHookDelGuardadoQueNoTerminaSeCorta(t *testing.T) {
	const ca = "54fa4e/ccf2ee/CA-644"
	tope := itemcmd.TopeGit
	itemcmd.TopeGit = 2 * time.Second
	t.Cleanup(func() { itemcmd.TopeGit = tope })

	f := apPATH(t)
	f.cli(t, "claude")
	root := apProyecto(t, "")
	const slug = "con-hooks"
	wt := apTuAprobacion(t, root, slug, apConAuto, true)
	apEscribir(t, wt, "app.go", "package app\n\n// sin guardar\n")
	head, rama := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug)

	aparte := t.TempDir()
	pids := filepath.Join(aparte, "pids")
	marca := filepath.Join(wt, "marca-del-hook")
	for _, h := range []string{"pre-commit", "prepare-commit-msg", "commit-msg", "post-commit"} {
		apr4Hook(t, wt, h, "#!/bin/sh\necho $$ >> '"+pids+"'\n: > '"+filepath.Join(aparte, "corrio-"+h)+"'\n"+
			"sleep 4\n: > '"+marca+"'\nexec sleep 60\n")
	}
	t.Cleanup(func() { apr3Matar(pids) })

	desde := time.Now()
	p := apr1Arrancar(root, slug)
	if !p.esperar(30 * time.Second) {
		apr3Matar(pids)
		p.esperar(2 * time.Minute)
		t.Fatalf("%s: con un hook que no termina, el piloto termina en un tiempo acotado (TopeGit 2 s); a los 30 s seguia:\n%s", ca, p.salida())
	}
	s := p.salida()
	ls := apSoloLineasDelPiloto(t, ca, s)
	pre := apDetiene + "no pudo guardar el trabajo de la tarjeta: "
	if s.code != 1 || !strings.HasPrefix(apUltima(ls), pre) || strings.TrimSpace(strings.TrimPrefix(apUltima(ls), pre)) == "" {
		t.Fatalf("%s: se detiene con `%s<error>` y exit 1 (en %s):\n%s", ca, pre, time.Since(desde), s)
	}
	for _, l := range ls {
		if apGuarda.MatchString(l) {
			t.Fatalf("%s: no dice que guardo: %q", ca, l)
		}
	}
	if !apr1Existe(filepath.Join(aparte, "corrio-pre-commit")) {
		t.Fatalf("%s: el guardado del piloto corre el hook pre-commit (aclaracion 23):\n%s", ca, s)
	}
	if h, r := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug); h != head || r != rama {
		t.Fatalf("%s: no commitea nada: HEAD %s -> %s, hoom/%s %s -> %s", ca, head, h, slug, rama, r)
	}
	soltar, err := hoomfs.Lock(pilotocmd.Candado(root, slug), 0)
	if err != nil {
		t.Fatalf("%s: al detenerse, el piloto suelta su candado: %v", ca, err)
	}
	soltar()
	// el hook escribiria la marca a los 4 s de arrancar si siguiera vivo
	for time.Since(desde) < 8*time.Second {
		if apr1Existe(marca) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if apr1Existe(marca) {
		t.Fatalf("%s: no queda vivo ningun proceso del grupo del hook: algo escribio %s en el worktree", ca, marca)
	}
	f.ninguna(t, ca, "un piloto que no pudo guardar")
}
