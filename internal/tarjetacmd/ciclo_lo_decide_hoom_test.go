// Tests adversariales del spec .hoom/specs/ciclo-lo-decide-hoom.md sobre
// los verbos de la tarjeta, como consumidor externo: hoom pedir refutador
// (CA-754, CA-755), el tope en hoom pedir y hoom siguiente (CA-750, CA-751),
// el pedido del writer en Review (CA-756) y el pedido del refutador que no se
// edita por lanzar.Lanzar (aclaracion 8). Proyectos git reales, hallazgos con
// finding.Register, registros de review con reviewcmd.WriteRecord y CLIs de
// IA falsos (vdtPATH). Escrito desde el spec, ACLARACIONES.md y el go doc.
package tarjetacmd_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/lanzar"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	cldSlug = "precios"
	// textos del contrato, escritos aparte de la implementacion
	cldMsgConM       = "el refutador arma su propio pedido: no lleva -m"
	cldMsgNadaQuePed = "no hay hallazgos que bloquean sin pasar por el refutador: nada que pedirle"
	cldMsgNoSeEdita  = "el refutador arma su propio pedido: no se edita"
	cldTeTocaAlTope  = "te toca: hoom finding list --open"
)

func cldRazon(n, k int) string {
	return fmt.Sprintf("se hicieron %d rondas de hallazgos y quedan %d que bloquean: te toca", n, k)
}

func cldPedidoRefutador(n int, slug string, ids []string) string {
	return fmt.Sprintf("Ronda %d de hallazgos de %s. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: %s. No edites codigo.",
		n, slug, strings.Join(ids, ", "))
}

func cldPedidoCorreccion(ids []string) string {
	return "Corregi los hallazgos que bloquean la tarjeta: " + strings.Join(ids, ", ") + ". Solo esos: los pendientes quedan abiertos."
}

// cldIDs son los ids de los hallazgos high abiertos de slug, en el orden de
// `hoom finding list` (finding.List: del mas viejo al mas nuevo).
func cldIDs(t *testing.T, wt, slug string) []string {
	t.Helper()
	items, _, err := finding.List(wt, vdtBase, true)
	if err != nil {
		t.Fatalf("fixture: finding.List: %v", err)
	}
	var ids []string
	for _, it := range items {
		if it.Task == slug && it.Severity == "high" {
			ids = append(ids, it.ID)
		}
	}
	return ids
}

// cldBloqueantes registra n hallazgos high (bloquean con block_on high) de
// slug en wt y devuelve los ids de todos sus high, en el orden de la lista.
func cldBloqueantes(t *testing.T, wt, slug string, n int) []string {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := finding.Register(wt, vdtBase, finding.Draft{Severity: "high", Lens: "risk", File: "lista.go",
			Description: fmt.Sprintf("bloquea la tarjeta, numero %d", i), Author: "reviewer@test", Task: slug}); err != nil {
			t.Fatalf("fixture: finding.Register: %v", err)
		}
	}
	ids := cldIDs(t, wt, slug)
	if len(ids) < n {
		t.Fatalf("fixture: %d bloqueantes, la lista dio %q", n, ids)
	}
	return ids
}

// cldPendiente registra un hallazgo medium de slug (no bloquea con block_on
// high) y devuelve su id.
func cldPendiente(t *testing.T, wt, slug string) string {
	t.Helper()
	f, err := finding.Register(wt, vdtBase, finding.Draft{Severity: "medium", Lens: "risk", File: "lista.go",
		Description: "un pendiente que no bloquea", Author: "reviewer@test", Task: slug})
	if err != nil {
		t.Fatalf("fixture: finding.Register: %v", err)
	}
	return f.ID
}

// cldRegistros escribe en wt una review completa de slug hace tres horas y
// deltas registros delta despues, uno por minuto: deltas rondas.
func cldRegistros(t *testing.T, wt, slug string, deltas int) {
	t.Helper()
	ini := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	for i := 0; i <= deltas; i++ {
		cob := reviewcmd.CoberturaDelta
		if i == 0 {
			cob = reviewcmd.CoberturaCompleta
		}
		if _, err := reviewcmd.WriteRecord(wt, reviewcmd.Record{CreatedAt: ini.Add(time.Duration(i) * time.Minute), Task: slug,
			Spec: vdtSpecRel(slug), Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: reviewcmd.Lentes,
			Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}, Usage: []reviewcmd.LensUsage{},
			Cobertura: cob, Base: vdtBase, Desde: strings.Repeat("a", 40), Hasta: strings.Repeat("b", 40)}); err != nil {
			t.Fatalf("fixture: reviewcmd.WriteRecord: %v", err)
		}
	}
}

// cldReview: la tarjeta de vdtTuAceptacion (verde chico, sin review
// exigida) con bloq bloqueantes y deltas rondas (con deltas < 0, sin
// registros de review), todo guardado. Cae en Review.
func cldReview(t *testing.T, ca, root, slug string, bloq, deltas int) (string, []string) {
	t.Helper()
	wt := vdtTuAceptacion(t, ca, root, slug)
	ids := cldBloqueantes(t, wt, slug, bloq)
	if deltas >= 0 {
		cldRegistros(t, wt, slug, deltas)
	}
	vdtCommit(t, wt, "hallazgos y registros de review")
	c := vdtColumna(t, ca, root, slug, boardcmd.ColReview)
	if deltas >= 0 && c.Rondas.Hechas != deltas {
		t.Fatalf("%s: fixture: la tarjeta %s cuenta %d rondas, fue %+v", ca, slug, deltas, c.Rondas)
	}
	return wt, ids
}

// cldWriterRojo: el veredicto de la tarjeta es rojo (fallo el gate test),
// con bloq bloqueantes y deltas rondas. Cae en Writer.
func cldWriterRojo(t *testing.T, ca, root, slug string, bloq, deltas int) (string, []string) {
	t.Helper()
	vdtItem(t, root, slug, "")
	wt := vdtTarea(t, root, slug)
	vdtEscribir(t, wt, vdtSpecRel(slug), vdtSpecTexto("- CA-970: citado por un test."))
	vdtEscribir(t, wt, vdtTestDe(slug), "package app\n\n// CA-970\n")
	vdtEscribir(t, wt, "lista.go", "package app\n\nfunc Lista() int { return 1 }\n")
	vdtAprobar(t, wt, slug)
	gates := vdtGatesVerdes()
	for i := range gates {
		if gates[i].Name == "test" {
			gates[i].Status = verdict.StatusFail
		}
	}
	v := &verdict.Verdict{Project: "demo", CreatedAt: time.Now().UTC().Add(-time.Minute), Spec: vdtSpecRel(slug),
		Git: gitx.Snapshot(wt, vdtBase), Gates: gates}
	v.Finalize()
	if _, err := verdict.Write(wt, v); err != nil {
		t.Fatal(err)
	}
	vdtCommit(t, wt, "rojo")
	ids := cldBloqueantes(t, wt, slug, bloq)
	cldRegistros(t, wt, slug, deltas)
	vdtCommit(t, wt, "hallazgos y registros de review")
	vdtColumna(t, ca, root, slug, boardcmd.ColWriter)
	return wt, ids
}

// cldSobreRefutador escribe un sobre CERRADO de refutador de slug, empezado
// hace `hace` (antes del veredicto: no es el rojo de la tarjeta), que recibio
// ids. En stage run cerro cortado (no-entregable).
func cldSobreRefutador(t *testing.T, root, wt, slug, id, stage string, hace time.Duration, ids ...string) {
	t.Helper()
	ini := time.Now().UTC().Add(-hace).Truncate(time.Second)
	status, code, paso := envelope.StatusDeliverable, 0, 6
	if stage == "run" {
		status, code, paso = envelope.StatusNotDeliverable, 1, 3
	}
	vdtSobre(t, root, envelope.Record{ID: id, Role: "refutador", Provider: "claude", Task: slug, Dir: wt,
		Spec: filepath.Join(wt, ".hoom", "specs", slug+".md"), Stage: stage, Step: paso, Steps: 6, Status: status,
		ExitCode: code, StartedAt: ini, UpdatedAt: ini.Add(5 * time.Minute), EndedAt: ini.Add(5 * time.Minute),
		Columna: boardcmd.ColReview, Pedido: cldPedidoRefutador(1, slug, ids), Hallazgos: append([]string(nil), ids...)})
}

// cldSobresDe son los sobres de rol de la tarjeta slug.
func cldSobresDe(t *testing.T, root, slug, rol string) []envelope.Record {
	t.Helper()
	var out []envelope.Record
	for _, r := range vdtSobres(t, root) {
		if r.Role == rol && r.Task == slug {
			out = append(out, r)
		}
	}
	return out
}

// cldNiega exige exit code, la razon en stderr, ningun efecto en disco y
// ningun CLI corrido.
func cldNiega(t *testing.T, ca, que string, f *vdtFalsos, root string, code int, msg string, args ...string) vdtSalida {
	t.Helper()
	antes := vdtFoto(t, root)
	s := vdtCorrer(t, ca, vdtOpts{dir: root}, args[0], args[1:]...)
	vdtExit(t, ca+" ("+que+")", s, code)
	if !strings.Contains(s.err, msg) {
		t.Fatalf("%s: [%s] stderr dice %q:\n%s", ca, que, msg, s)
	}
	vdtSinEfectos(t, ca, que, antes, vdtFoto(t, root))
	f.ninguna(t, ca, que)
	return s
}

// ===========================================================================
// CA-754: hoom pedir refutador lanza al refutador con su pedido
// ===========================================================================

// CA-754 y aclaracion 8: con una ronda hecha y dos bloqueantes sin refutar,
// `hoom pedir refutador precios` lanza al refutador con
// PedidoRondaRefutador(2, slug, ids), los ids en el orden de hoom finding
// list; su sobre registra hallazgos, el pedido y la columna Review. Con
// terminal y $EDITOR no abre el editor.
func TestCA754_PedirRefutadorLanzaConSuPedido(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	_, ids := cldReview(t, "CA-754", root, cldSlug, 2, 1)
	quiero := cldPedidoRefutador(2, cldSlug, ids)
	c := vdtCarta(t, root, cldSlug)
	a := vdtAccion(t, "CA-754", c, boardcmd.ActPedirRefutador)
	if c.Actions[0].ID != boardcmd.ActPedirWriter || a.Pedido != quiero || !reflect.DeepEqual(a.Hallazgos, ids) {
		t.Fatalf("CA-753/CA-754: en riesgo normal la primera accion es pedir-writer y pedir-refutador trae\n  %q y %q\nfue %v, %q y %q", quiero, ids, c.Actions[0].ID, a.Pedido, a.Hallazgos)
	}
	if boardcmd.PedidoRondaRefutador(2, cldSlug, ids) != quiero {
		t.Fatalf("CA-754: PedidoRondaRefutador es el texto del contrato: %q", boardcmd.PedidoRondaRefutador(2, cldSlug, ids))
	}

	ed, capt := vdtEditor(t, "editor", "Refuta todo, tambien los pendientes", 0)
	s := vdtCorrer(t, "CA-754", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed, "VISUAL": ed}},
		"pedir", "refutador", cldSlug)
	if vdtEditorLlamado(capt) {
		t.Fatalf("CA-754 (aclaracion 8): hoom pedir refutador no abre el editor:\n%s", s)
	}
	p := f.pedidos(t, "claude")
	if len(p) != 1 || !strings.Contains(p[0], quiero) || strings.Contains(p[0], "tambien los pendientes") {
		t.Fatalf("CA-754: el refutador recibe su pedido %q: %q\n%s", quiero, p, s)
	}
	sobres := cldSobresDe(t, root, cldSlug, "refutador")
	if len(sobres) != 1 {
		t.Fatalf("CA-754: un sobre de refutador, fueron %d:\n%s", len(sobres), s)
	}
	r := sobres[0]
	if !reflect.DeepEqual(r.Hallazgos, ids) || r.Pedido != quiero || r.Columna != boardcmd.ColReview {
		t.Fatalf("CA-754: el sobre registra hallazgos %q, el pedido y la columna review: %q %q %q", ids, r.Hallazgos, r.Pedido, r.Columna)
	}
}

// CA-754: un sobre de refutador cortado en run no cuenta como recibido; uno
// que cerro fuera de run si. El refutador recibe solo el que nadie recibio.
func TestCA754_ElCortadoEnRunNoCuentaEnDisco(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wt, ids := cldReview(t, "CA-754", root, cldSlug, 3, 0)
	cldSobreRefutador(t, root, wt, cldSlug, "20260920T100000_ref001", "run", 5*time.Hour, ids...)
	cldSobreRefutador(t, root, wt, cldSlug, "20260920T110000_ref002", "ok", 4*time.Hour, ids[0], ids[2])
	c := vdtColumna(t, "CA-754", root, cldSlug, boardcmd.ColReview)
	a := vdtAccion(t, "CA-754", c, boardcmd.ActPedirRefutador)
	sin := []string{ids[1]}
	if !reflect.DeepEqual(a.Hallazgos, sin) || a.Pedido != cldPedidoRefutador(1, cldSlug, sin) {
		t.Fatalf("CA-754: sin refutar queda %q: %q %q", sin, a.Hallazgos, a.Pedido)
	}
	vdtCorrer(t, "CA-754", vdtOpts{dir: root}, "pedir", "refutador", cldSlug)
	var nuevos []envelope.Record
	for _, r := range cldSobresDe(t, root, cldSlug, "refutador") {
		if r.ID != "20260920T100000_ref001" && r.ID != "20260920T110000_ref002" {
			nuevos = append(nuevos, r)
		}
	}
	if len(nuevos) != 1 || !reflect.DeepEqual(nuevos[0].Hallazgos, sin) {
		t.Fatalf("CA-754: el sobre nuevo registra solo %q: %+v", sin, nuevos)
	}
	if p := f.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], cldPedidoRefutador(1, cldSlug, sin)) {
		t.Fatalf("CA-754: el pedido lista solo el sin refutar: %q", p)
	}
}

// ===========================================================================
// CA-755: los errores de hoom pedir refutador
// ===========================================================================

// CA-755: con -m es el error de uso (exit 2), sin efectos, tambien cuando el
// texto es el pedido que hoom arma.
func TestCA755_ConMEsErrorDeUso(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	_, ids := cldReview(t, "CA-755", root, cldSlug, 2, 0)
	cldNiega(t, "CA-755", "pedir refutador -m", f, root, 2, cldMsgConM, "pedir", "refutador", cldSlug, "-m", "Refuta todos los abiertos")
	cldNiega(t, "CA-755", "pedir refutador -m con su propio pedido", f, root, 2, cldMsgConM,
		"pedir", "refutador", cldSlug, "-m", cldPedidoRefutador(1, cldSlug, ids))
	cldNiega(t, "CA-755", "pedir refutador -m antes del slug", f, root, 2, cldMsgConM, "pedir", "refutador", "-m", "x", cldSlug)
}

// CA-755 y CA-753: cuando todos los bloqueantes ya pasaron por un refutador
// (un sobre cerrado fuera de run), la tarjeta no ofrece pedir-refutador, la
// primera es pedir-writer y `hoom pedir refutador` sale con exit 1 y el
// texto del contrato, sin lanzar.
func TestCA755_TodosRefutadosNadaQuePedir(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wt, ids := cldReview(t, "CA-755", root, cldSlug, 2, 0)
	cldSobreRefutador(t, root, wt, cldSlug, "20260920T100000_ref001", "ok", 4*time.Hour, ids...)
	c := vdtColumna(t, "CA-755", root, cldSlug, boardcmd.ColReview)
	for _, a := range c.Actions {
		if a.ID == boardcmd.ActPedirRefutador {
			t.Fatalf("CA-753: todos refutados: no hay pedir-refutador: %+v", c.Actions)
		}
	}
	if len(c.Actions) == 0 || c.Actions[0].ID != boardcmd.ActPedirWriter {
		t.Fatalf("CA-753: todos refutados: la primera es pedir-writer: %s", c.Actions[0].ID)
	}
	cldNiega(t, "CA-755", "pedir refutador con todo refutado", f, root, 1, "hoom: "+cldMsgNadaQuePed, "pedir", "refutador", cldSlug)
}

// CA-755: en Review sin ningun bloqueante (la review exigida) tampoco hay
// nada que pedirle.
func TestCA755_ReviewSinBloqueantes(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtReview(t, "CA-755", root, cldSlug)
	cldNiega(t, "CA-755", "pedir refutador sin bloqueantes", f, root, 1, cldMsgNadaQuePed, "pedir", "refutador", cldSlug)
}

// CA-755 (caso limite): fuera de Review, hoom pedir refutador es un rol que
// la columna no ofrece, con el mensaje de hoy; tambien en Writer con
// bloqueantes sin refutar.
func TestCA755_FueraDeReviewElMensajeDeHoy(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "CA-755", root, "escribiendo", "")
	vdtTuAceptacion(t, "CA-755", root, "lista")
	cldWriterRojo(t, "CA-755", root, "roja", 2, 0)
	for _, slug := range []string{"escribiendo", "lista", "roja"} {
		c := vdtCarta(t, root, slug)
		msg := "en " + c.ColumnName + " no se le pide al refutador: " + c.Plain + ". Siguiente: hoom siguiente"
		s := cldNiega(t, "CA-755", "pedir refutador en "+c.ColumnName+" ("+slug+")", f, root, 1, msg, "pedir", "refutador", slug)
		if strings.Contains(s.err, cldMsgNadaQuePed) {
			t.Fatalf("CA-755: fuera de Review no es el texto de nada que pedirle:\n%s", s)
		}
	}
}

// CA-755: el rol desconocido nombra los cinco roles, exit 2, sin efectos.
func TestCA755_RolDesconocidoNombraLosCinco(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	cldReview(t, "CA-755", root, cldSlug, 1, 0)
	for _, rol := range []string{"scout", "Refutador", "refutadores", "orquestador", ""} {
		want := `rol desconocido "` + rol + `" (arquitecto, test-writer, writer, refutador, reviewer); para otros roles: hoom agent`
		cldNiega(t, "CA-755", "pedir "+rol, f, root, 2, want, "pedir", rol, cldSlug)
	}
}

// ===========================================================================
// CA-750 y CA-751: el tope en los verbos
// ===========================================================================

// CA-751: al tope (dos rondas, el tope por defecto, y dos bloqueantes sin
// refutar), hoom pedir writer (con y sin -m) y hoom pedir refutador salen
// con exit 1 y la razon de CA-750, sin lanzar ni tocar nada. pedir-reviewer
// sigue en la tarjeta (aclaracion 4).
func TestCA751_AlTopePedirSeNiega(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	cldReview(t, "CA-751", root, cldSlug, 2, 2)
	c := vdtCarta(t, root, cldSlug)
	razon := cldRazon(2, 2)
	if !c.Rondas.AlTope || c.Plain != razon || c.Next != boardcmd.AccionAlTope {
		t.Fatalf("CA-750: en disco la tarjeta esta al tope: %+v plain %q next %q", c.Rondas, c.Plain, c.Next)
	}
	cldNiega(t, "CA-751", "pedir writer -m al tope", f, root, 1, razon, "pedir", "writer", cldSlug, "-m", "Corregi los hallazgos")
	cldNiega(t, "CA-751", "pedir writer al tope", f, root, 1, razon, "pedir", "writer", cldSlug)
	cldNiega(t, "CA-751", "pedir refutador al tope", f, root, 1, razon, "pedir", "refutador", cldSlug)
}

// CA-751 y aclaracion 3: al tope con todos los bloqueantes ya refutados,
// pedir refutador da la razon del tope (no el de nada que pedirle), y en la
// columna Writer (veredicto rojo) pedir writer tambien se niega con ella.
func TestCA751_AlTopeEnOtrosEstados(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wt, ids := cldReview(t, "CA-751", root, cldSlug, 2, 3)
	cldSobreRefutador(t, root, wt, cldSlug, "20260920T100000_ref001", "ok", 4*time.Hour, ids...)
	cldNiega(t, "CA-751", "pedir refutador al tope, todo refutado", f, root, 1, cldRazon(3, 2), "pedir", "refutador", cldSlug)

	cldWriterRojo(t, "CA-751", root, "roja", 1, 2)
	cldNiega(t, "CA-751", "pedir writer al tope en Writer", f, root, 1, cldRazon(2, 1), "pedir", "writer", "roja", "-m", "Arregla el rojo")
	// control: una ronda menos, la tarjeta roja ofrece pedir-writer
	cldWriterRojo(t, "CA-751", root, "roja-bajo", 1, 1)
	vdtAccion(t, "CA-751", vdtCarta(t, root, "roja-bajo"), boardcmd.ActPedirWriter)
}

// CA-751 y CA-748: el tope sale de findings.max_rondas del hoom.yaml del
// proyecto, y sin la clave de autonomia.rondas_de_hallazgos si es mayor que
// 2.
func TestCA751_ElTopeDelHoomYaml(t *testing.T) {
	casos := []struct {
		nombre, yaml string
		bajo, al     int // rondas bajo el tope y al tope
	}{
		{"max_rondas 3", "  max_rondas: 3\n", 2, 3},
		{"max_rondas 1", "  max_rondas: 1\n", 0, 1},
		{"autonomia 3 sin max_rondas", "autonomia:\n  rondas_de_hallazgos: 3\n", 2, 3},
		{"max_rondas 5 y autonomia 2", "  max_rondas: 5\nautonomia:\n  rondas_de_hallazgos: 2\n", 4, 5},
	}
	for _, cs := range casos {
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, cs.yaml)
		cldReview(t, "CA-751", root, "bajo", 1, cs.bajo)
		c := vdtCarta(t, root, "bajo")
		if c.Rondas.AlTope || c.Rondas.Tope != cs.al {
			t.Fatalf("CA-751: [%s] %d rondas no llegan al tope %d: %+v", cs.nombre, cs.bajo, cs.al, c.Rondas)
		}
		vdtAccion(t, "CA-751 ("+cs.nombre+")", c, boardcmd.ActPedirRefutador)
		cldReview(t, "CA-751", root, "al", 1, cs.al)
		cldNiega(t, "CA-751", "pedir refutador al tope ("+cs.nombre+")", f, root, 1, cldRazon(cs.al, 1), "pedir", "refutador", "al")
	}
}

// CA-750: al tope, hoom siguiente no lanza nada e imprime `te toca: hoom
// finding list --open` y, en la linea siguiente, la razon; tambien con
// terminal y una respuesta afirmativa, y en la columna Writer (aclaracion 3).
func TestCA750_SiguienteAlTope(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	cldReview(t, "CA-750", root, cldSlug, 2, 2)
	cldWriterRojo(t, "CA-750", root, "roja", 1, 2)
	for _, cs := range []struct {
		slug  string
		razon string
		o     vdtOpts
	}{
		{cldSlug, cldRazon(2, 2), vdtOpts{dir: root}},
		{cldSlug, cldRazon(2, 2), vdtOpts{dir: root, terminal: true, in: "s\n"}},
		{"roja", cldRazon(2, 1), vdtOpts{dir: root}},
	} {
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "CA-750", cs.o, "siguiente", cs.slug)
		lineas := vdtLineas(s.out)
		i := -1
		for j, l := range lineas {
			if strings.TrimSpace(l) == cldTeTocaAlTope {
				i = j
				break
			}
		}
		if i < 0 || i+1 >= len(lineas) || strings.TrimSpace(lineas[i+1]) != cs.razon {
			t.Fatalf("CA-750: [%s] siguiente imprime %q y en la linea siguiente %q:\n%s", cs.slug, cldTeTocaAlTope, cs.razon, s)
		}
		vdtSinEfectos(t, "CA-750", "siguiente al tope en "+cs.slug, antes, vdtFoto(t, root))
	}
	f.ninguna(t, "CA-750", "siguiente al tope")
}

// CA-753 y CA-754: bajo el tope, en riesgo alto con bloqueantes sin refutar,
// hoom siguiente lanza al refutador (la primera accion) con su pedido.
func TestCA753_SiguienteBajoElTopeLanzaAlRefutador(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wt, ids := cldReview(t, "CA-753", root, cldSlug, 2, -1)
	vdtItem(t, root, cldSlug, "riesgo: alto\n")
	all, err := verdict.LoadAll(wt)
	if err != nil {
		t.Fatalf("CA-753: fixture: verdict.LoadAll: %v", err)
	}
	v := verdict.LatestComplete(all)
	if v == nil {
		t.Fatalf("CA-753: fixture: sin veredicto completo")
	}
	base, err := gitx.MergeBase(wt, vdtBase)
	if err != nil {
		t.Fatalf("CA-753: fixture: gitx.MergeBase: %v", err)
	}
	head, err := gitx.Head(wt)
	if err != nil {
		t.Fatalf("CA-753: fixture: gitx.Head: %v", err)
	}
	ini := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	for i, cob := range []string{reviewcmd.CoberturaCompleta, reviewcmd.CoberturaDelta} {
		if _, err := reviewcmd.WriteRecord(wt, reviewcmd.Record{
			CreatedAt: ini.Add(time.Duration(i) * time.Minute), Task: cldSlug,
			Spec: vdtSpecRel(cldSlug), Fingerprint: v.Git.ChangeFingerprint, VerdictID: v.ID, Verdict: "green",
			Lenses: reviewcmd.Lentes, Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes,
			Findings: []string{}, Usage: []reviewcmd.LensUsage{},
			Cobertura: cob, Base: vdtBase, Desde: base, Hasta: head,
		}); err != nil {
			t.Fatalf("CA-753: fixture: WriteRecord: %v", err)
		}
	}
	s := vdtCorrer(t, "CA-753", vdtOpts{dir: root}, "siguiente", cldSlug)
	if strings.Contains(s.out, cldTeTocaAlTope) {
		t.Fatalf("CA-753: bajo el tope no hay tope:\n%s", s)
	}
	p := f.pedidos(t, "claude")
	if len(p) != 1 || !strings.Contains(p[0], cldPedidoRefutador(2, cldSlug, ids)) {
		t.Fatalf("CA-753: siguiente lanza al refutador con su pedido: %q\n%s", p, s)
	}
	if r := cldSobresDe(t, root, cldSlug, "refutador"); len(r) != 1 || !reflect.DeepEqual(r[0].Hallazgos, ids) {
		t.Fatalf("CA-754: el sobre del refutador registra %q: %+v", ids, r)
	}
}

// ===========================================================================
// CA-756: el pedido del writer en Review
// ===========================================================================

// CA-756: con dos bloqueantes (uno ya paso por el refutador, el otro no) y
// un pendiente, hoom pedir writer sin -m ni terminal lanza al writer con
// `Corregi los hallazgos que bloquean la tarjeta: <ids>. Solo esos: los
// pendientes quedan abiertos.`: todos los que bloquean, ningun pendiente.
func TestCA756_PedirWriterEnReview(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	wt := vdtTuAceptacion(t, "CA-756", root, cldSlug)
	ids := cldBloqueantes(t, wt, cldSlug, 2)
	pend := cldPendiente(t, wt, cldSlug)
	vdtCommit(t, wt, "hallazgos")
	cldSobreRefutador(t, root, wt, cldSlug, "20260920T100000_ref001", "ok", 4*time.Hour, ids[0])
	quiero := cldPedidoCorreccion(ids)
	c := vdtColumna(t, "CA-756", root, cldSlug, boardcmd.ColReview)
	if w := vdtAccion(t, "CA-756", c, boardcmd.ActPedirWriter); w.Pedido != quiero || boardcmd.PedidoCorreccion(ids) != quiero {
		t.Fatalf("CA-756: el pedido propuesto del writer:\n  %q\nfue\n  %q", quiero, w.Pedido)
	}
	s := vdtCorrer(t, "CA-756", vdtOpts{dir: root}, "pedir", "writer", cldSlug)
	p := f.pedidos(t, "claude")
	if len(p) != 1 || !strings.Contains(p[0], quiero) || strings.Contains(p[0], pend) {
		t.Fatalf("CA-756: el writer recibe %q y nunca el pendiente %s: %q\n%s", quiero, pend, p, s)
	}
}

// ===========================================================================
// Aclaracion 8: el pedido del refutador no se edita (lanzar.Lanzar)
// ===========================================================================

// CA-755 y aclaracion 8: lanzar.Lanzar niega un pedido de refutador que no
// es el que armo hoom con `el refutador arma su propio pedido: no se edita`,
// sin efectos; con el pedido de hoom lanza y el sobre registra los ids.
func TestCA755_LanzarNiegaElPedidoEditado(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	_, ids := cldReview(t, "CA-755", root, cldSlug, 2, 0)
	m, err := manifest.Load(root, nil)
	if err != nil {
		t.Fatalf("fixture: hoom.yaml: %v", err)
	}
	propio := cldPedidoRefutador(1, cldSlug, ids)
	for _, editado := range []string{
		propio + " De paso refuta tambien los pendientes.",
		"Refuta todos los hallazgos abiertos.",
		cldPedidoRefutador(1, cldSlug, ids[:1]),
		cldPedidoRefutador(2, cldSlug, ids),
		strings.Replace(propio, "No edites codigo.", "Edita lo que haga falta.", 1),
	} {
		antes := vdtFoto(t, root)
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, cldSlug,
			lanzar.Request{Action: boardcmd.ActPedirRefutador, Provider: "claude", Pedido: editado})
		if tr != nil {
			tr.Wait()
		}
		cancel()
		var r *lanzar.Rechazo
		if !errors.As(err, &r) || r.Motivo != cldMsgNoSeEdita {
			t.Fatalf("CA-755 (aclaracion 8): el pedido editado %q se niega con %q, fue %v", editado, cldMsgNoSeEdita, err)
		}
		vdtSinEfectos(t, "CA-755", "lanzar un refutador con el pedido editado", antes, vdtFoto(t, root))
	}
	f.ninguna(t, "CA-755", "un refutador con el pedido editado")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	tr, err := lanzar.Lanzar(ctx, lanzar.Params{Root: root, Manifest: m}, cldSlug,
		lanzar.Request{Action: boardcmd.ActPedirRefutador, Provider: "claude", Pedido: propio})
	if err != nil || tr == nil {
		t.Fatalf("CA-755: con el pedido que armo hoom lanza: %v %+v", err, tr)
	}
	tr.Wait()
	if tr.Role != "refutador" {
		t.Fatalf("CA-755: lanza al refutador: %+v", tr)
	}
	r := cldSobresDe(t, root, cldSlug, "refutador")
	if len(r) != 1 || !reflect.DeepEqual(r[0].Hallazgos, ids) || r[0].Pedido != propio {
		t.Fatalf("CA-755: el sobre registra %q y su pedido: %+v", ids, r)
	}
}
