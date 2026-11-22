// Tests adversariales del spec .hoom/specs/ciclo-lo-decide-hoom.md sobre el
// Studio (aclaraciones 7 y 8): POST /api/board/{slug}/launch acepta
// pedir-refutador con el pedido que armo hoom (CA-753, CA-754), niega el
// pedido editado con `el refutador arma su propio pedido: no se edita`, una
// tarjeta sin la accion es el 409 de hoy, y al tope (CA-751) no lanza al
// writer ni al refutador. CLIs falsos (lnPATH), proyectos git reales.
package servecmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

const cldMsgNoSeEdita = "el refutador arma su propio pedido: no se edita"

func cldRazon(n, k int) string {
	return fmt.Sprintf("se hicieron %d rondas de hallazgos y quedan %d que bloquean: te toca", n, k)
}

func cldPedidoRefutador(n int, slug string, ids []string) string {
	return fmt.Sprintf("Ronda %d de hallazgos de %s. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: %s. No edites codigo.",
		n, slug, strings.Join(ids, ", "))
}

// cldCartaReview: verde chico (sin review exigida) con bloq hallazgos high
// de la tarjeta y deltas rondas (una review completa y deltas registros
// delta), todo guardado. Cae en Review. Devuelve el espacio de trabajo y los
// ids de los bloqueantes en el orden de hoom finding list.
func cldCartaReview(t *testing.T, ca, root, slug string, bloq, deltas int) (string, []string) {
	t.Helper()
	tbItem(t, root, slug, "")
	wt := lnTarea(t, ca, root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-970: citado por un test.", true))
	tbEscribir(t, wt, lnTestDe(slug), "package app\n\n// CA-970\n")
	tbEscribir(t, wt, "lista.go", "package app\n\nfunc Lista() int { return 1 }\n")
	tbAprobar(t, wt, slug)
	lnCommit(t, wt, "spec y codigo")
	lnVeredicto(t, wt, slug, 0)
	lnCommit(t, wt, "veredicto")
	for i := 0; i < bloq; i++ {
		if _, err := finding.Register(wt, lnBase, finding.Draft{Severity: "high", Lens: "risk", File: "lista.go",
			Description: fmt.Sprintf("bloquea la tarjeta, numero %d", i), Author: "reviewer@test", Task: slug}); err != nil {
			t.Fatalf("%s: fixture: finding.Register: %v", ca, err)
		}
	}
	ini := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	for i := 0; deltas >= 0 && i <= deltas; i++ {
		cob := reviewcmd.CoberturaDelta
		if i == 0 {
			cob = reviewcmd.CoberturaCompleta
		}
		if _, err := reviewcmd.WriteRecord(wt, reviewcmd.Record{CreatedAt: ini.Add(time.Duration(i) * time.Minute), Task: slug,
			Spec: ".hoom/specs/" + slug + ".md", Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: reviewcmd.Lentes,
			Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, Findings: []string{}, Usage: []reviewcmd.LensUsage{},
			Cobertura: cob, Base: lnBase, Desde: strings.Repeat("a", 40), Hasta: strings.Repeat("b", 40)}); err != nil {
			t.Fatalf("%s: fixture: reviewcmd.WriteRecord: %v", ca, err)
		}
	}
	lnCommit(t, wt, "hallazgos y registros de review")
	lnColumna(t, ca, root, slug, boardcmd.ColReview)
	items, _, err := finding.List(wt, lnBase, true)
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, it := range items {
		if it.Task == slug && it.Severity == "high" {
			ids = append(ids, it.ID)
		}
	}
	return wt, ids
}

// cldSobreRefutador escribe en la raiz un sobre cerrado entregable de
// refutador de slug, de hace cuatro horas, que recibio ids.
func cldSobreRefutador(t *testing.T, root, wt, slug, id string, ids ...string) {
	t.Helper()
	ini := time.Now().UTC().Add(-4 * time.Hour).Truncate(time.Second)
	rec := envelope.Record{ID: id, Role: "refutador", Provider: "claude", Task: slug, Dir: wt,
		Spec: filepath.Join(wt, ".hoom", "specs", slug+".md"), Stage: "ok", Step: 6, Steps: 6,
		Status: envelope.StatusDeliverable, StartedAt: ini, UpdatedAt: ini.Add(5 * time.Minute),
		EndedAt: ini.Add(5 * time.Minute), Columna: boardcmd.ColReview, Hallazgos: ids}
	raw, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	tbEscribir(t, root, ".hoom/"+envelope.DirName+"/"+id+".json", string(raw)+"\n")
}

// cldError es el mensaje {"error": ...} de una respuesta que no es 202,
// cualquiera sea su codigo.
func cldError(t *testing.T, ca, que string, rec *httptest.ResponseRecorder) string {
	t.Helper()
	if rec.Code == http.StatusAccepted || rec.Code < 400 {
		t.Fatalf("%s: [%s] el launch se niega, fue %d: %s", ca, que, rec.Code, rec.Body.String())
	}
	var e map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e["error"] == "" {
		t.Fatalf("%s: [%s] el cuerpo es {\"error\": ...}: %d %q", ca, que, rec.Code, rec.Body.String())
	}
	return e["error"]
}

// CA-753, CA-754 y aclaracion 7: POST launch con pedir-refutador y el pedido
// de la accion lanza al refutador: 202, y su sobre registra los ids, el
// pedido y la columna Review.
func TestCA753_StudioLanzaAlRefutador(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	_, ids := cldCartaReview(t, "CA-753", root, "precios", 2, 1)
	c := lnCarta(t, root, "precios")
	a, ok := lnAccion(c, boardcmd.ActPedirRefutador)
	quiero := cldPedidoRefutador(2, "precios", ids)
	if !ok || a.Pedido != quiero || !reflect.DeepEqual(a.Hallazgos, ids) {
		t.Fatalf("CA-754: fixture: la tarjeta ofrece pedir-refutador con %q: %+v", quiero, a)
	}
	s := lnServidor(t, root, f)
	r := lnAceptado(t, "CA-753", lnLaunch(t, s, "precios", s.Token(), lnCuerpo(boardcmd.ActPedirRefutador, "claude", "", nil, a.Pedido)))
	if r.Role != "refutador" || r.Provider != "claude" {
		t.Fatalf("CA-753: el 202 nombra al refutador: %+v", r)
	}
	lnEsperarLanzamientos(t, "CA-753", s)
	rec := lnRegistro(t, "CA-754", root, r.EnvelopeID)
	if rec.Role != "refutador" || !reflect.DeepEqual(rec.Hallazgos, ids) || rec.Pedido != quiero || rec.Columna != boardcmd.ColReview {
		t.Fatalf("CA-754: el sobre del Studio registra %q, su pedido y la columna review: %+v", ids, rec)
	}
	if p := f.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], quiero) {
		t.Fatalf("CA-754: el refutador recibe su pedido: %q", p)
	}
}

// CA-755 y aclaracion 8: un pedido de refutador que no es el que armo hoom se niega
// con `el refutador arma su propio pedido: no se edita`, sin efectos ni
// llamadas.
func TestCA755_StudioNiegaElPedidoEditado(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	_, ids := cldCartaReview(t, "CA-755", root, "precios", 2, 0)
	s := lnServidor(t, root, f)
	propio := cldPedidoRefutador(1, "precios", ids)
	antes := lnHuellas(t, root)
	for _, editado := range []string{
		propio + " Refuta tambien los pendientes.",
		"Refuta todos los hallazgos abiertos de la tarjeta.",
		cldPedidoRefutador(1, "precios", ids[:1]),
		cldPedidoRefutador(3, "precios", ids),
	} {
		rec := lnLaunch(t, s, "precios", s.Token(), lnCuerpo(boardcmd.ActPedirRefutador, "claude", "", nil, editado))
		if msg := cldError(t, "CA-755", "pedido editado", rec); msg != cldMsgNoSeEdita {
			t.Fatalf("CA-755 (aclaracion 8): el pedido editado %q se niega con\n  %q\nfue\n  %q", editado, cldMsgNoSeEdita, msg)
		}
		lnEsperarLanzamientos(t, "CA-755", s)
		lnSinEfectos(t, "CA-755", "un refutador con el pedido editado", antes, lnHuellas(t, root), false)
	}
	lnSinLlamadas(t, "CA-755", "un refutador con el pedido editado", f)
}

// Aclaracion 7: en una tarjeta que no ofrece pedir-refutador (Writer), el
// launch es el 409 de hoy, sin efectos; en Review con todo refutado tampoco
// lanza.
func TestCA753_StudioSinLaAccion(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	lnCartaWriter(t, "CA-753", root, "en-writer", "")
	wt, ids := cldCartaReview(t, "CA-753", root, "refutada", 1, 0)
	cldSobreRefutador(t, root, wt, "refutada", "20260920T100000_ref001", ids...)
	enWriter := lnColumna(t, "CA-753", root, "en-writer", boardcmd.ColWriter)
	if _, ok := lnAccion(lnCarta(t, root, "refutada"), boardcmd.ActPedirRefutador); ok {
		t.Fatalf("CA-753: fixture: con todo refutado la tarjeta no ofrece pedir-refutador")
	}
	s := lnServidor(t, root, f)
	antes := lnHuellas(t, root)

	pedido := cldPedidoRefutador(1, "en-writer", []string{"f1"})
	msg := tbErrorJSON(t, "CA-753", lnLaunch(t, s, "en-writer", s.Token(), lnCuerpo(boardcmd.ActPedirRefutador, "claude", "", nil, pedido)),
		http.StatusConflict)
	if msg != lnYaNoEsta(enWriter) {
		t.Fatalf("CA-753: pedir-refutador en Writer es el 409 de hoy\n  %q\nfue\n  %q", lnYaNoEsta(enWriter), msg)
	}
	cldError(t, "CA-753", "pedir-refutador con todo refutado", lnLaunch(t, s, "refutada", s.Token(),
		lnCuerpo(boardcmd.ActPedirRefutador, "claude", "", nil, cldPedidoRefutador(1, "refutada", ids))))
	lnEsperarLanzamientos(t, "CA-753", s)
	lnSinEfectos(t, "CA-753", "pedir-refutador sin la accion", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-753", "pedir-refutador sin la accion", f)
}

// CA-751: al tope, el Studio no lanza al writer ni al refutador; lo que dice
// lleva la razon del tope.
func TestCA751_StudioAlTope(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	_, ids := cldCartaReview(t, "CA-751", root, "precios", 2, 2)
	c := lnCarta(t, root, "precios")
	razon := cldRazon(2, 2)
	if !c.Rondas.AlTope || c.Plain != razon {
		t.Fatalf("CA-750: fixture: la tarjeta esta al tope: %+v %q", c.Rondas, c.Plain)
	}
	s := lnServidor(t, root, f)
	antes := lnHuellas(t, root)
	for _, k := range []struct{ action, pedido string }{
		{boardcmd.ActPedirWriter, "Corregi los hallazgos que bloquean la tarjeta: " + strings.Join(ids, ", ") + ". Solo esos: los pendientes quedan abiertos."},
		{boardcmd.ActPedirRefutador, cldPedidoRefutador(3, "precios", ids)},
	} {
		msg := cldError(t, "CA-751", k.action+" al tope", lnLaunch(t, s, "precios", s.Token(), lnCuerpo(k.action, "claude", "", nil, k.pedido)))
		if !strings.Contains(msg, razon) {
			t.Fatalf("CA-751: %s al tope dice la razon %q: %q", k.action, razon, msg)
		}
		lnEsperarLanzamientos(t, "CA-751", s)
	}
	lnSinEfectos(t, "CA-751", "lanzar al tope", antes, lnHuellas(t, root), false)
	lnSinLlamadas(t, "CA-751", "lanzar al tope", f)
}
