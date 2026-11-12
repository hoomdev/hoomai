// Tests de regresion de la ronda 1 de autonomia-entre-firmas sobre la
// funcion de lanzamiento compartida (aclaracion 13): con Request.Pilot,
// Lanzar vuelve a mirar la tarjeta bajo su candado y rechaza el lanzamiento,
// sin correr ningun rol ni crear sobre, si el item ya no tiene
// `auto: hasta-humano` (hallazgo 20261112T172149_8c69af, CA-642 caso 1) o si
// la tarjeta esta en Tu aprobacion, Tu aceptacion o Hecho (hallazgo
// 20261112T172636_a3e8d2, CA-642 caso 3). Un lanzamiento de una persona sigue
// como hoy. CLI falso (r1Claude), nunca uno real.
package lanzar_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	ar1ConAuto = "presupuesto_usd: 5\nauto: hasta-humano\n"
	ar1SinAuto = "presupuesto_usd: 5\n"
)

func ar1ItemTexto(slug, extra string) string {
	return "titulo: Tarjeta " + slug + "\ntipo: feature\nprioridad: media\n" +
		"pedido: el pedido de " + slug + "\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n" + extra
}

func ar1SpecTexto(criterios string) string {
	return "# Spec\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n" + criterios + "\n\n## Decisiones\nx\n\n## Riesgos\nx\n"
}

func ar1Commit(t *testing.T, dir, msg string) {
	t.Helper()
	vdtGit(t, dir, "add", "-A")
	vdtGit(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
}

// ar1Proyecto: repo git en main con hoom.yaml (gate barato, findings con
// block_on high), el .gitignore canonico de .hoom y un commit; sin items.
func ar1Proyecto(t *testing.T) (string, *manifest.Manifest) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	vdtGit(t, root, "init", "-b", "main")
	vdtGit(t, root, "config", "user.email", "test@hoom.dev")
	vdtGit(t, root, "config", "user.name", "hoom test")
	vdtEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+
		"findings:\n  block_on: high\n")
	vdtEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	vdtEscribir(t, root, "app.go", "package app\n")
	ar1Commit(t, root, "inicial")
	m, err := manifest.Load(root, nil)
	if err != nil {
		t.Fatalf("fixture: hoom.yaml: %v", err)
	}
	return root, m
}

func ar1Item(t *testing.T, root, slug, extra string) {
	t.Helper()
	vdtEscribir(t, root, ".hoom/items/"+slug+".yaml", ar1ItemTexto(slug, extra))
	ar1Commit(t, root, "item "+slug)
}

func ar1Tarea(t *testing.T, root, slug string) string {
	t.Helper()
	if err := taskcmd.Start(root, slug, "main"); err != nil {
		t.Fatalf("fixture: hoom task start %s: %v", slug, err)
	}
	return filepath.Join(root, ".hoom", "worktrees", slug)
}

func ar1Firmar(t *testing.T, wt, slug string) {
	t.Helper()
	if _, _, err := approval.Approve(wt, filepath.Join(wt, ".hoom", "specs", slug+".md")); err != nil {
		t.Fatalf("fixture: aprobar %s: %v", slug, err)
	}
}

func ar1Carta(t *testing.T, root, slug string) boardcmd.Card {
	t.Helper()
	c, err := boardcmd.CardFor(root, "main", "high", slug, time.Now().UTC())
	if err != nil {
		t.Fatalf("fixture: CardFor %s: %v", slug, err)
	}
	return c
}

func ar1Columna(t *testing.T, ca, root, slug, col string) boardcmd.Card {
	t.Helper()
	c := ar1Carta(t, root, slug)
	if c.Column != col {
		t.Fatalf("%s: fixture: la tarjeta %s debia estar en %s y esta en %s", ca, slug, col, c.Column)
	}
	return c
}

// Las tarjetas de cada columna, todas con extra en su item.

func ar1TestWriter(t *testing.T, root, slug, extra string) {
	t.Helper()
	ar1Item(t, root, slug, extra)
	wt := ar1Tarea(t, root, slug)
	vdtEscribir(t, wt, ".hoom/specs/"+slug+".md", ar1SpecTexto("- CA-1: todavia nadie lo cita."))
	ar1Firmar(t, wt, slug)
	ar1Commit(t, wt, "spec aprobado")
	ar1Columna(t, "CA-642", root, slug, boardcmd.ColTestWriter)
}

func ar1TuAprobacion(t *testing.T, root, slug, extra string) {
	t.Helper()
	ar1Item(t, root, slug, extra)
	wt := ar1Tarea(t, root, slug)
	vdtEscribir(t, wt, ".hoom/specs/"+slug+".md", ar1SpecTexto("- CA-1: todavia nadie lo cita."))
	ar1Commit(t, wt, "spec sin aprobar")
	ar1Columna(t, "CA-642", root, slug, boardcmd.ColTuAprobacion)
}

// ar1TuAceptacion: verde chico con la huella actual, todo guardado, sin
// hallazgos.
func ar1TuAceptacion(t *testing.T, root, slug, extra string) {
	t.Helper()
	ar1Item(t, root, slug, extra)
	wt := ar1Tarea(t, root, slug)
	vdtEscribir(t, wt, ".hoom/specs/"+slug+".md", ar1SpecTexto("- CA-1: citado por un test."))
	vdtEscribir(t, wt, strings.ReplaceAll(slug, "-", "_")+"_test.go", "package app\n\n// CA-1\n")
	vdtEscribir(t, wt, "lista.go", "package app\n\nfunc Lista() int { return 1 }\n")
	ar1Firmar(t, wt, slug)
	var gates []verdict.GateResult
	for _, n := range []string{"spec_lint", "spec_trace", "spec_approved", "findings_open", "test"} {
		gates = append(gates, verdict.GateResult{Name: n, Required: true, Status: verdict.StatusPass})
	}
	v := &verdict.Verdict{Project: "demo", CreatedAt: time.Now().UTC().Add(-time.Minute),
		Spec: ".hoom/specs/" + slug + ".md", Git: gitx.Snapshot(wt, "main"), Gates: gates}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	ar1Commit(t, wt, "lista para cerrar")
	ar1Columna(t, "CA-642", root, slug, boardcmd.ColTuAceptacion)
}

func ar1Hecho(t *testing.T, root, slug, extra string) {
	t.Helper()
	ar1Item(t, root, slug, extra+"hecho_en: 2026-09-23T10:00:00Z\ncommit_final: 3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c\n")
	ar1Columna(t, "CA-642", root, slug, boardcmd.ColHecho)
}

// --- lanzar ---

func ar1Lanzar(t *testing.T, root string, m *manifest.Manifest, slug string, req lanzar.Request) (*lanzar.Trabajo, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	return lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, slug, req)
}

// ar1Rechazado exige un rechazo sin efectos: ningun sobre nuevo, ningun
// CLI corrido.
func ar1Rechazado(t *testing.T, ca, que, root, marcas string, sobres, llamadas int, tr *lanzar.Trabajo, err error) {
	t.Helper()
	if tr != nil {
		tr.Wait()
	}
	if err == nil {
		t.Fatalf("%s: %s se rechaza; lanzo %+v", ca, que, tr)
	}
	if n := len(envelope.List(root)); n != sobres {
		t.Fatalf("%s: %s no crea sobre: %d -> %d (%+v)", ca, que, sobres, n, envelope.List(root))
	}
	if n := r1Llamadas(t, marcas); n != llamadas {
		t.Fatalf("%s: %s no corre ningun rol: claude corrio %d veces (antes %d)", ca, que, n, llamadas)
	}
}

// ar1Lanzado exige que el lanzamiento arranque y termine, y devuelve su
// registro.
func ar1Lanzado(t *testing.T, ca, que, root string, tr *lanzar.Trabajo, err error) envelope.Record {
	t.Helper()
	if err != nil || tr == nil || tr.EnvelopeID == "" {
		t.Fatalf("%s: %s lanza como hoy: %v %+v", ca, que, err, tr)
	}
	tr.Wait()
	for _, r := range envelope.List(root) {
		if r.ID == tr.EnvelopeID {
			return r
		}
	}
	t.Fatalf("%s: %s: el sobre %s esta en disco", ca, que, tr.EnvelopeID)
	return envelope.Record{}
}

// Hallazgo 20261112T172149_8c69af (CA-642 caso 1, aclaracion 13): el piloto
// decidio con auto encendido, la persona lo apaga (`hoom item set --auto no`,
// sin guardar, en el principal) y recien entonces llega el lanzamiento del
// piloto: se rechaza sin correr ningun rol ni crear sobre. La persona, en el
// mismo estado, lanza como hoy (no depende de auto); con auto encendido, el
// mismo pedido del piloto lanza.
func TestHallazgo8c69af_CA642_ElPilotoNoLanzaSiSeApagoElAuto(t *testing.T) {
	marcas := r1Claude(t)
	root, m := ar1Proyecto(t)
	const slug = "apagada"
	ar1TestWriter(t, root, slug, ar1ConAuto)
	vdtEscribir(t, root, ".hoom/items/"+slug+".yaml", ar1ItemTexto(slug, ar1SinAuto))
	if c := ar1Carta(t, root, slug); c.Item.Auto != "" || c.Column != boardcmd.ColTestWriter {
		t.Fatalf("fixture: la tarjeta en Test-writer, con el auto apagado en el principal: %q %q", c.Item.Auto, c.Column)
	}
	piloto := lanzar.Request{Action: boardcmd.ActPedirTestWriter, Provider: "claude", Pedido: "escribi los tests de " + slug,
		Pilot: true, Regla: boardcmd.ReglaCadena}
	tr, err := ar1Lanzar(t, root, m, slug, piloto)
	ar1Rechazado(t, "8c69af/CA-642", "el lanzamiento del piloto con el auto apagado", root, marcas, 0, 0, tr, err)

	persona := piloto
	persona.Pilot, persona.Regla = false, ""
	tr, err = ar1Lanzar(t, root, m, slug, persona)
	if rec := ar1Lanzado(t, "8c69af/CA-642", "el pedido de una persona con el auto apagado", root, tr, err); rec.Pilot {
		t.Fatalf("8c69af/CA-642: el sobre de una persona no es del piloto: %+v", rec)
	}

	root2, m2 := ar1Proyecto(t)
	ar1TestWriter(t, root2, slug, ar1ConAuto)
	tr, err = ar1Lanzar(t, root2, m2, slug, piloto)
	if rec := ar1Lanzado(t, "8c69af/CA-642", "el lanzamiento del piloto con el auto encendido", root2, tr, err); !rec.Pilot {
		t.Fatalf("8c69af/CA-642: con auto encendido el piloto lanza y su sobre lo dice: %+v", rec)
	}
}

// ar1AccionLanzable es una accion de rol habilitada de la tarjeta (primero
// las que ve una persona, despues las del piloto) cuyos valores con claude
// puede lanzar hoy (lanzar.Comprobar, sin Pilot).
func ar1AccionLanzable(c boardcmd.Card) (boardcmd.Action, bool) {
	for _, xs := range [][]boardcmd.Action{c.Actions, c.PilotActions} {
		for _, a := range xs {
			if !a.Enabled || a.Role == "" || !strings.HasPrefix(a.ID, "pedir-") {
				continue
			}
			if lanzar.Comprobar(c, a, lanzar.Request{Action: a.ID, Provider: "claude", Pedido: a.Pedido}) == nil {
				return a, true
			}
		}
	}
	return boardcmd.Action{}, false
}

// Hallazgo 20261112T172636_a3e8d2 (CA-642 caso 3, aclaracion 13): en Tu
// aprobacion, Tu aceptacion o Hecho, un lanzamiento del piloto se rechaza sin
// correr ningun rol ni crear sobre, aunque la accion que pide exista y este
// habilitada para esa tarjeta. El mismo pedido de una persona en una columna
// de modelo sigue funcionando como hoy.
func TestHallazgoa3e8d2_CA642_ElPilotoNoLanzaEnUnaColumnaHumana(t *testing.T) {
	marcas := r1Claude(t)
	probadas := 0
	for _, c := range []struct {
		col   string
		armar func(t *testing.T, root, slug, extra string)
	}{
		{boardcmd.ColTuAprobacion, ar1TuAprobacion},
		{boardcmd.ColTuAceptacion, ar1TuAceptacion},
		{boardcmd.ColHecho, ar1Hecho},
	} {
		root, m := ar1Proyecto(t)
		slug := "en-" + c.col
		c.armar(t, root, slug, ar1ConAuto)
		carta := ar1Columna(t, "a3e8d2/CA-642", root, slug, c.col)
		id, pedido := boardcmd.ActPedirWriter, "implementa "+slug
		if a, ok := ar1AccionLanzable(carta); ok {
			id, pedido = a.ID, a.Pedido
			probadas++
		}
		antes := r1Llamadas(t, marcas)
		tr, err := ar1Lanzar(t, root, m, slug, lanzar.Request{Action: id, Provider: "claude", Pedido: pedido,
			Pilot: true, Regla: boardcmd.ReglaCadena})
		ar1Rechazado(t, "a3e8d2/CA-642", "el lanzamiento del piloto ("+id+") en "+c.col, root, marcas, 0, antes, tr, err)
	}
	if probadas == 0 {
		t.Fatal("a3e8d2/CA-642: fixture: ninguna columna humana tiene una accion de rol habilitada que se pueda lanzar: el test no abre su ventana")
	}

	root, m := ar1Proyecto(t)
	const modelo = "en-una-columna-de-modelo"
	ar1TestWriter(t, root, modelo, ar1ConAuto)
	tr, err := ar1Lanzar(t, root, m, modelo, lanzar.Request{Action: boardcmd.ActPedirTestWriter, Provider: "claude",
		Pedido: "escribi los tests de " + modelo})
	ar1Lanzado(t, "a3e8d2/CA-642", "el pedido de una persona en Test-writer", root, tr, err)
}
