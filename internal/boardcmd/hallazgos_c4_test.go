// Tests de regresion de la review de 4 lentes de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en boardcmd: la historia de la
// tarjeta (seccion 1, CA-359..CA-362). Escritos desde el spec y la
// descripcion de cada hallazgo, sin leer la implementacion; repos git reales
// en t.TempDir() y telemetria escrita a mano, sin ningun CLI de IA. Los
// helpers compartidos (hi*, bd*) estan en historia_test.go y gather_test.go;
// los de este archivo llevan el prefijo c4.
//
//   - 20261016T195646_f47433 (risk): un sobre con un run_id que sube de
//     directorio hace que la historia lea un .jsonl fuera de .hoom/runs y
//     publique sus delegaciones. La historia solo trae los subagentes de los
//     runs de la tarjeta (.hoom/runs/<id>.jsonl).
//   - 20261016T200503_d3c75f (resilience): la historia carga el log entero de
//     cada run. Con una linea enorme que no es delegacion, las delegaciones
//     de antes y de despues salen bien, y la memoria que asigna leer la
//     historia no crece con el tamano del log.
//   - 20261016T200544_c84aa8 (resilience): los errores de git al leer el
//     contenido de un spec o de una aprobacion, y los de git rev-list, se
//     reinterpretan como datos validos. Si git falla, faltan TODAS las
//     entradas de git y notes dice "sin historial de git: <error>" (y no la
//     nota de fast-forward).
package boardcmd

import (
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// c4RepoEn arma en root (que todavia no existe) el mismo proyecto que bdRepo:
// hace falta elegir el directorio para tener un proyecto vecino al lado.
func c4RepoEn(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	bdGit(t, root, "init", "-b", "main")
	bdGit(t, root, "config", "user.email", "test@hoom.dev")
	bdGit(t, root, "config", "user.name", "hoom test")
	bdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	bdEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	bdEscribir(t, root, "app.go", "package app\n")
	bdGit(t, root, "add", "-A")
	bdGit(t, root, "commit", "-m", "inicial")
}

// ---------------------------------------------------------------- f47433

// Los run_id plantados: los dos llegan desde .hoom/runs de "proyecto" al
// .jsonl de "otro", el proyecto vecino.
var c4RunIDsQueSuben = []string{
	"../../../otro/.hoom/runs/ajeno",
	"x/../../../../otro/.hoom/runs/ajeno",
}

// CA-361, CA-362 (hallazgo f47433): la historia trae los subagentes de los
// runs de la tarjeta, que viven en .hoom/runs/<id>.jsonl. Un sobre de la
// tarjeta cuyo run_id sube de directorio y apunta al .jsonl de otro proyecto
// (con delegaciones de verdad) no publica esas delegaciones: ninguna entrada
// subagente ni subagente-fin sale de ese archivo, y ninguna entrada nombra
// lo que hay en el. Los subagentes de un run legitimo de la tarjeta siguen
// saliendo (el fixture no pasa por no leer nada).
func TestHallazgo_f47433_CA362_RunIDQueSubeNoTraeSubagentesAjenos(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	c4RepoEn(t, root)
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0.Add(-24*time.Hour), "")

	// un run legitimo de la tarjeta, con una delegacion que cierra
	bdRunDisco(t, root, runcmd.Meta{ID: hiR1, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
		CreatedAt: hiMin(10), EndedAt: hiMin(30), Status: runcmd.StatusDone})
	hiEventos(t, root, hiR1,
		hiEv(10, "start", "", "", "init"),
		hiEv(12, "agent", "general-purpose", "toolu_OK", "Agent: buscar precios"),
		hiEv(14, "agent_end", "general-purpose", "toolu_OK", "general-purpose termino"),
		hiEv(29, "end", "", "", "listo"))

	// la narracion del proyecto vecino, con delegaciones de verdad
	otro := filepath.Join(base, "otro")
	hiEventos(t, otro, "ajeno",
		hiEv(40, "start", "", "", "init"),
		hiEv(41, "agent", "espia-ajeno", "toolu_AJ", "Agent: secreto del otro proyecto"),
		hiEv(42, "agent_end", "espia-ajeno", "toolu_AJ", "espia-ajeno termino"),
		hiEv(43, "agent", "otro-espia", "toolu_AJ2", "Agent: mas secretos"))
	for i, id := range c4RunIDsQueSuben {
		p := filepath.Join(root, ".hoom", "runs", id+".jsonl")
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("fixture: el run_id %q llega al .jsonl del proyecto vecino (%s): %v", id, p, err)
		}
		sobre := []string{"20260921T090000_sobre1", "20260921T090500_sobre2"}[i]
		bdSobreDisco(t, root, envelope.Record{ID: sobre, Role: "writer", Provider: "claude", Task: bdSlug, Dir: root,
			RunID: id, Stage: "ok", Step: 7, Steps: 7, Status: envelope.StatusDeliverable,
			StartedAt: hiMin(i), UpdatedAt: hiMin(i + 3), EndedAt: hiMin(i + 3)})
	}

	tl := hiTimeline(t, "CA-362", root)
	for _, e := range tl.Entries {
		todo := strings.Join([]string{e.Who, e.Plain, e.Summary, e.Artifact, e.Ref}, " | ")
		for _, ajeno := range []string{"espia-ajeno", "otro-espia", "toolu_AJ", "secreto", "otro/", ".."} {
			if strings.Contains(todo, ajeno) {
				t.Fatalf("CA-362 (hallazgo f47433): un run_id que sube de directorio no lleva la historia fuera de .hoom/runs: la entrada %s nombra %q del proyecto vecino: %+v\n%s",
					e.Kind, ajeno, e, hiListado(tl))
			}
		}
	}
	sub, fin := hiDeKind(tl, KindSubagente), hiDeKind(tl, KindSubagenteFin)
	if len(sub) != 1 || sub[0].Who != "general-purpose (claude)" || len(fin) != 1 || fin[0].Who != "general-purpose (claude)" {
		t.Fatalf("CA-362 (hallazgo f47433): los unicos subagentes son los del run de la tarjeta (1 subagente y 1 subagente-fin de general-purpose); hubo %d y %d:\n%s",
			len(sub), len(fin), hiListado(tl))
	}
	if a := ".hoom/runs/" + hiR1 + ".jsonl"; sub[0].Artifact != a || fin[0].Artifact != a {
		t.Fatalf("CA-362 (hallazgo f47433): las delegaciones salen de %s: %q, %q", a, sub[0].Artifact, fin[0].Artifact)
	}
}

// ---------------------------------------------------------------- d3c75f

const c4MiB = 1 << 20

// c4LogConLineaEnorme arma la tarjeta con un run cuyo log tiene una linea
// text de n bytes (no es delegacion) entre delegaciones: una que abre antes
// y cierra despues, una que abre y cierra despues, y una que no cierra.
func c4LogConLineaEnorme(t *testing.T, n int) string {
	t.Helper()
	root := bdRepo(t, "")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0.Add(-24*time.Hour), "")
	bdRunDisco(t, root, runcmd.Meta{ID: hiR1, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
		CreatedAt: hiMin(10), EndedAt: hiMin(30), Status: runcmd.StatusDone})
	relleno := strings.Repeat("relleno ", n/8)
	p := hiEventos(t, root, hiR1,
		hiEv(10, "start", "", "", "init"),
		hiEv(11, "agent", "Explore", "toolu_1", "Agent: antes de la linea enorme"),
		hiEv(12, "text", "", "", relleno),
		hiEv(13, "agent_end", "Explore", "toolu_1", "Explore termino"),
		hiEv(14, "agent", "Plan", "toolu_2", "Agent: despues de la linea enorme"),
		hiEv(15, "agent_end", "Plan", "toolu_2", "Plan fallo: se corto"),
		hiEv(16, "agent", "general-purpose", "toolu_3", "Agent: sin cierre"),
		hiEv(29, "end", "", "", "listo"))
	if fi, err := os.Stat(p); err != nil || fi.Size() < int64(n) {
		t.Fatalf("fixture: el log del run pesa al menos %d bytes: %v %v", n, fi, err)
	}
	return root
}

// c4DelegacionesBien exige las delegaciones del log de c4LogConLineaEnorme.
func c4DelegacionesBien(t *testing.T, ca string, tl Timeline) {
	t.Helper()
	fin := func(n int) *time.Time { x := hiMin(n); return &x }
	sub := hiDeKind(tl, KindSubagente)
	want := []struct {
		at     int
		who    string
		plain  string
		termin *time.Time
	}{
		{11, "Explore (claude)", "el writer le paso trabajo a Explore", fin(13)},
		{14, "Plan (claude)", "el writer le paso trabajo a Plan", fin(15)},
		{16, "general-purpose (claude)", "el writer le paso trabajo a general-purpose", nil},
	}
	if len(sub) != len(want) {
		t.Fatalf("%s: las 3 delegaciones del log (antes y despues de la linea enorme) son entradas subagente; hubo %d:\n%s", ca, len(sub), hiListado(tl))
	}
	for i, w := range want {
		e := sub[i]
		if !e.At.Equal(hiMin(w.at)) || e.Who != w.who || e.Plain != w.plain {
			t.Fatalf("%s: la delegacion %d es la de %s del minuto %d (%q); fue %s %q %q", ca, i, w.who, w.at, w.plain,
				e.At.Format(time.RFC3339), e.Who, e.Plain)
		}
		switch {
		case w.termin == nil && e.EndedAt != nil:
			t.Fatalf("%s: la delegacion de %s no cerro: ended_at null, fue %s", ca, w.who, e.EndedAt.Format(time.RFC3339))
		case w.termin != nil && (e.EndedAt == nil || !e.EndedAt.Equal(*w.termin)):
			t.Fatalf("%s: ended_at de la delegacion de %s es el ts de su agent_end (%s), fue %v", ca, w.who, w.termin.Format(time.RFC3339), e.EndedAt)
		}
	}
	fins := hiDeKind(tl, KindSubagenteFin)
	if len(fins) != 2 || !fins[0].At.Equal(hiMin(13)) || fins[0].Plain != "Explore termino su parte" ||
		!fins[1].At.Equal(hiMin(15)) || fins[1].Plain != "Plan fallo" {
		t.Fatalf("%s: las 2 salidas de escena (Explore termino su parte, Plan fallo), despues de la linea enorme:\n%s", ca, hiListado(tl))
	}
	for _, e := range tl.Entries {
		if e.Source == SourceTelemetria && e.Kind != KindRun && e.Kind != KindSubagente && e.Kind != KindSubagenteFin {
			t.Fatalf("%s: la linea text no es una entrada de la historia: %+v", ca, e)
		}
	}
}

// CA-362 (hallazgo d3c75f): un log con una linea enorme que no es
// delegacion (un text de 4 MiB y de 32 MiB) no tapa las delegaciones de
// antes ni de despues: cada agent da su subagente con el ended_at de su
// agent_end (tambien el que cierra despues de la linea enorme), cada
// agent_end emparejado su subagente-fin, y la que no cerro queda con
// ended_at null.
func TestHallazgo_d3c75f_CA362_LineaEnormeNoTapaLasDelegaciones(t *testing.T) {
	for _, n := range []int{4 * c4MiB, 32 * c4MiB} {
		root := c4LogConLineaEnorme(t, n)
		c4DelegacionesBien(t, fmt.Sprintf("CA-362 (hallazgo d3c75f, linea de %d MiB)", n/c4MiB), hiTimeline(t, "CA-362", root))
	}
}

// c4Asigna mide lo que asigna leer la historia (TotalAlloc): el minimo de
// tres lecturas, despues de un GC, para que el ruido no cuente.
func c4Asigna(t *testing.T, root string) uint64 {
	t.Helper()
	mejor := uint64(math.MaxUint64)
	for i := 0; i < 3; i++ {
		runtime.GC()
		var antes, despues runtime.MemStats
		runtime.ReadMemStats(&antes)
		_, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
		runtime.ReadMemStats(&despues)
		if err != nil {
			t.Fatalf("TimelineFor: %v", err)
		}
		if d := despues.TotalAlloc - antes.TotalAlloc; d < mejor {
			mejor = d
		}
	}
	return mejor
}

// CA-362 (hallazgo d3c75f): la historia no carga el log entero de un run.
// Con la misma tarjeta y el mismo log salvo el tamano de su linea text (4 MiB
// contra 32 MiB), lo que asigna leer la historia no crece con el log: la
// diferencia queda por debajo de 16 MiB, la mitad de la linea grande. Cargar
// el archivo entero (o la linea entera) asigna al menos los 28 MiB de
// diferencia.
func TestHallazgo_d3c75f_CA362_LaHistoriaNoCargaElLogEntero(t *testing.T) {
	chico := c4LogConLineaEnorme(t, 4*c4MiB)
	grande := c4LogConLineaEnorme(t, 32*c4MiB)
	a4 := c4Asigna(t, chico)
	a32 := c4Asigna(t, grande)
	const tope = 16 * c4MiB
	if a32 > a4 && a32-a4 >= tope {
		t.Fatalf("CA-362 (hallazgo d3c75f): leer la historia con una linea de 32 MiB asigna %.1f MiB y con una de 4 MiB %.1f MiB: crece %.1f MiB con el log (tope %d MiB); la historia no carga el log entero",
			float64(a32)/c4MiB, float64(a4)/c4MiB, float64(a32-a4)/c4MiB, tope/c4MiB)
	}
}

// ---------------------------------------------------------------- c84aa8

// c4SinBlob borra del almacen de objetos el blob que commit tiene para rel:
// el commit sigue nombrando la ruta, pero git ya no puede leer su contenido
// (un clon parcial sin el promisor, un objeto perdido).
func c4SinBlob(t *testing.T, root, commit, rel string) {
	t.Helper()
	sha := bdGit(t, root, "rev-parse", commit+":"+rel)
	obj := filepath.Join(root, ".git", "objects", sha[:2], sha[2:])
	if err := os.Remove(obj); err != nil {
		t.Fatalf("fixture: el blob de %s en %s tiene que estar suelto en %s: %v", rel, commit[:12], obj, err)
	}
	cmd := exec.Command("git", "cat-file", "-e", sha)
	cmd.Dir = root
	if cmd.Run() == nil {
		t.Fatalf("fixture: el objeto de %s:%s tiene que faltar", commit[:12], rel)
	}
}

// c4SinHistoriaGit exige la regla de git que falla: ninguna entrada de git,
// la nota "sin historial de git: <error>" con un error no vacio, y no la
// nota de fast-forward.
func c4SinHistoriaGit(t *testing.T, ca string, tl Timeline) {
	t.Helper()
	for _, e := range tl.Entries {
		if e.Source == SourceGit {
			t.Fatalf("%s: si git falla, las entradas de git faltan todas (no a medias); hay %s %s (notes %q):\n%s", ca, e.Kind, hiCorto(e.Ref), tl.Notes, hiListado(tl))
		}
	}
	hay := false
	for _, n := range tl.Notes {
		if strings.HasPrefix(n, NoteSinGitPrefijo) && strings.TrimSpace(strings.TrimPrefix(n, NoteSinGitPrefijo)) != "" {
			hay = true
		}
	}
	if !hay {
		t.Fatalf("%s: si git falla, notes dice %q seguido del error de git: %q", ca, NoteSinGitPrefijo+"<error>", tl.Notes)
	}
	if hiTieneNota(tl, NoteFastForward) {
		t.Fatalf("%s: un fallo de git no es una integracion con fast-forward: notes no dice %q: %q", ca, NoteFastForward, tl.Notes)
	}
}

// c4SpecAprobadoYEditado: en la raiz (sin espacio de trabajo), el spec V1
// (c1), su aprobacion (c2) y el spec V2 (c3). El medidor de c1 y c2 depende
// del CONTENIDO del spec V1 y de la aprobacion en sus commits.
func c4SpecAprobadoYEditado(t *testing.T) (root, c1, c2, c3, aprob string) {
	t.Helper()
	root = bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	bdEscribir(t, root, bdSpec, hiSpecV1)
	c1 = hiCommit(t, root, "spec de precios", h(1), "")
	aprob = hiAprobar(t, root, bdSlug, hiFirmante, h(2))
	c2 = hiCommit(t, root, "aprobacion del spec", h(2), "")
	bdEscribir(t, root, bdSpec, hiSpecV2)
	c3 = hiCommit(t, root, "edicion del spec", h(3), "")

	// el fixture: con todo el contenido a mano, el medidor sale de leerlo
	tl := hiTimeline(t, "CA-363", root)
	hiEfectos(t, "CA-363", "fixture: el spec V1 sin aprobacion previa", hiUna(t, "CA-363", tl, KindSpec, c1, bdSpec), map[string]string{"spec": SegFalta})
	hiEfectos(t, "CA-363", "fixture: la aprobacion del spec V1", hiUna(t, "CA-363", tl, KindAprobacion, c2, aprob), map[string]string{"spec": SegHecho})
	hiEfectos(t, "CA-363", "fixture: la edicion sin aprobar", hiUna(t, "CA-363", tl, KindSpec, c3, bdSpec), map[string]string{"spec": SegFalta})
	for _, n := range tl.Notes {
		if strings.HasPrefix(n, NoteSinGitPrefijo) {
			t.Fatalf("fixture: con todo el contenido, git no falla: %q", tl.Notes)
		}
	}
	return root, c1, c2, c3, aprob
}

// CA-359, CA-363 (hallazgo c84aa8): si git no puede leer el contenido del
// spec o de la aprobacion en su commit (el blob falta en el almacen de
// objetos), la historia no sigue con un medidor inventado: es un fallo de
// git, asi que faltan TODAS las entradas de git y notes dice "sin historial
// de git: <error>". La tarjeta de hoy se sigue leyendo (el disco tiene todo).
func TestHallazgo_c84aa8_CA359_SinElContenidoEnSuCommitNoHayHistoriaAMedias(t *testing.T) {
	for _, caso := range []struct{ nombre, ca string }{
		{"spec", "CA-359 (hallazgo c84aa8, falta el spec V1 en su commit)"},
		{"aprobacion", "CA-359 (hallazgo c84aa8, falta la aprobacion en su commit)"},
	} {
		t.Run(caso.nombre, func(t *testing.T) {
			root, c1, c2, _, aprob := c4SpecAprobadoYEditado(t)
			if caso.nombre == "spec" {
				c4SinBlob(t, root, c1, bdSpec)
			} else {
				c4SinBlob(t, root, c2, aprob)
			}
			if _, err := CardFor(root, "main", "high", bdSlug, hiAhora); err != nil {
				t.Fatalf("fixture: la tarjeta de hoy se lee del disco aunque falte un objeto viejo: %v", err)
			}
			c4SinHistoriaGit(t, caso.ca, hiTimeline(t, "CA-359", root))
		})
	}
}

// c4IntegradaNoFF: la tarjeta integrada con merge --no-ff y sin rama, como
// en TestCA360_IntegradaSinFastForwardYSinRama. Devuelve sus dos commits.
func c4IntegradaNoFF(t *testing.T) (root, w1, w2 string) {
	t.Helper()
	root = bdRepo(t, "")
	d0 := time.Date(2026, 9, 21, 9, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, "precios.go", "package app\n")
	w1 = hiCommit(t, wt, "codigo de precios", h(1), hiAgente)
	bdEscribir(t, wt, "precios_test.go", "package app\n")
	w2 = hiCommit(t, wt, "prueba de precios", h(2), "")
	bdEscribir(t, root, "main.go", "package app // en main\n")
	hiCommit(t, root, "otro trabajo en main", h(3), "")
	bdGit(t, root, "worktree", "remove", "--force", wt)
	if ok, err := item.MarkDone(root, bdSlug, w2, h(4)); err != nil || !ok {
		t.Fatalf("fixture: MarkDone: %v %v", ok, err)
	}
	hiCommit(t, root, "cierre de precios", h(4), "")
	hiGitEnv(t, root, hiFechas(h(5)), "merge", "--no-ff", "-q", "-m", "integrar precios", "hoom/"+bdSlug)
	bdGit(t, root, "branch", "-D", "hoom/"+bdSlug)
	hiNoExisteRama(t, root)
	return root, w1, w2
}

// c4GitRoto pone al frente del PATH un git que falla (exit 128, con un
// mensaje en stderr) en cualquier invocacion que lleve flag, y que es el git
// de verdad en todas las demas. Devuelve el archivo que marca que fallo.
func c4GitRoto(t *testing.T, flag string) (marca string) {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("sin git en el PATH")
	}
	real, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marca = filepath.Join(t.TempDir(), "disparo")
	s := "#!/bin/sh\nfor a in \"$@\"; do\n  if [ \"$a\" = '" + flag + "' ]; then\n" +
		"    : > '" + marca + "'\n" +
		"    echo 'fatal: git roto por la prueba en " + flag + "' >&2\n    exit 128\n  fi\ndone\n" +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return marca
}

// CA-360 (hallazgo c84aa8): para separar los commits de una tarjeta
// integrada, la historia busca M con git rev-list (--first-parent
// --ancestry-path) y mira si M es un merge. Si git rev-list falla (en
// --ancestry-path o en --parents), eso es un fallo de git, no una
// integracion con fast-forward: notes dice "sin historial de git: <error>",
// NO dice la nota de fast-forward, y no hay entradas de git. Sin la falla,
// la misma tarjeta separa sus dos commits (el fixture).
func TestHallazgo_c84aa8_CA360_RevListQueFallaNoEsFastForward(t *testing.T) {
	root, w1, w2 := c4IntegradaNoFF(t)
	ok := hiTimeline(t, "CA-360", root)
	hiSoloCommits(t, "CA-360 (fixture, sin falla)", ok, []string{w1, w2}, nil)
	for _, n := range ok.Notes {
		if n == NoteFastForward || strings.HasPrefix(n, NoteSinGitPrefijo) {
			t.Fatalf("fixture: sin la falla, la tarjeta integrada con --no-ff no tiene notas de git: %q", ok.Notes)
		}
	}

	for _, flag := range []string{"--ancestry-path", "--parents"} {
		t.Run(strings.TrimPrefix(flag, "--"), func(t *testing.T) {
			marca := c4GitRoto(t, flag)
			tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
			if err != nil {
				t.Fatalf("CA-360 (hallazgo c84aa8): TimelineFor solo falla como CardFor; con git roto en %s la historia se lee igual: %v", flag, err)
			}
			if _, err := os.Stat(marca); err != nil {
				t.Skipf("la historia ya no corre git con %s: esta falla no se puede inyectar asi", flag)
			}
			hiBienFormada(t, "CA-360", tl)
			c4SinHistoriaGit(t, "CA-360 (hallazgo c84aa8, git rev-list falla en "+flag+")", tl)
		})
	}
}
