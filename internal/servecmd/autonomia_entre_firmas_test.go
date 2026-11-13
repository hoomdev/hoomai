// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md sobre
// el Studio (CA-646, con CA-644 y CA-661 en lo que hace la vuelta): aprobar
// la spec desde la tarjeta (POST /api/specs/{name}/approve con {"card"}) con
// auto: hasta-humano ejecuta el piloto: guarda el worktree (la firma y el spec
// quedan commiteados en hoom/<slug>, nada en el checkout principal), lanza al
// test-writer, y la respuesta gana `piloto` con la linea de esa decision
// (ACLARACION 8: ni la de regla ni la de guardar). Sin auto no se lanza nada
// y la respuesta es la de hoy. CLIs falsos en el PATH minimo (lnPATH).
package servecmd

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
)

// aesCarta: item en la raiz (con extra) y espacio de trabajo con un spec que
// pasa lint, sin guardar y sin firma (C1: Tu aprobacion).
func aesCarta(t *testing.T, root, slug, extra string) string {
	t.Helper()
	tbItem(t, root, slug, extra)
	wt := lnTarea(t, "CA-646", root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-946: todavia nadie lo cita.", true))
	lnColumna(t, "CA-646", root, slug, boardcmd.ColTuAprobacion)
	return wt
}

// aesAprobar firma desde la tarjeta y devuelve la respuesta como mapa.
func aesAprobar(t *testing.T, s *Server, slug string) map[string]json.RawMessage {
	t.Helper()
	code, ct, body := lnAprobar(t, s, slug, s.Token(), `{"card": "`+slug+`"}`)
	if code != http.StatusOK || !strings.Contains(ct, "application/json") {
		t.Fatalf("CA-646: aprobar desde la tarjeta responde 200 JSON, fue %d %q: %s", code, ct, body)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatalf("CA-646: la respuesta es JSON: %v\n%s", err, body)
	}
	for _, k := range []string{"approval", "already"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("CA-646: la respuesta sigue trayendo %q: %s", k, body)
		}
	}
	return m
}

func aesClaves(m map[string]json.RawMessage) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// aesGuardadoDeLaFirma exige que la firma y el spec de la tarjeta esten
// commiteados en hoom/<slug> por el guardado del piloto (CA-644).
func aesGuardadoDeLaFirma(t *testing.T, ca, root, wt, slug, headPrincipal string) {
	t.Helper()
	if st := spGit(t, wt, "status", "--porcelain", "--", ".hoom/specs", ".hoom/approvals"); st != "" {
		t.Fatalf("%s: el piloto guarda la firma y el spec antes de decidir: %q", ca, st)
	}
	msgs := spGit(t, wt, "log", "--format=%s", "hoom/"+slug, "--", ".hoom/approvals")
	if !strings.Contains("\n"+msgs+"\n", "\nhoom: guardar la tarjeta "+slug+"\n") {
		t.Fatalf("%s: la firma entra en un commit `hoom: guardar la tarjeta %s` de la rama de la tarea: %q", ca, slug, msgs)
	}
	if h := spGit(t, root, "rev-parse", "HEAD"); h != headPrincipal {
		t.Fatalf("%s: el piloto no commitea en el checkout principal: HEAD %s -> %s", ca, headPrincipal, h)
	}
}

// CA-646: con auto, aprobar desde la tarjeta ejecuta el piloto: guarda, lanza
// al test-writer con la regla cadena (claude, lo que queda: 5 USD) y la
// respuesta trae `piloto` con la linea de pedir de esa decision.
func TestCA646_AprobarConAutoLanzaAlTestWriter(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	const slug = "firma-y-sigue"
	wt := aesCarta(t, root, slug, spPresupuesto+spAuto)
	headPrincipal := spGit(t, root, "rev-parse", "HEAD")
	s := lnServidor(t, root, f)

	m := aesAprobar(t, s, slug)
	var piloto string
	if err := json.Unmarshal(m["piloto"], &piloto); err != nil {
		t.Fatalf("CA-646: con auto la respuesta trae \"piloto\" con una linea: %v (%s)", err, m["piloto"])
	}
	mm := spPide.FindStringSubmatch(piloto)
	if mm == nil {
		t.Fatalf("CA-646: \"piloto\" es la linea de la decision (pide al ...), no la de regla ni la de guardar: %q", piloto)
	}
	id := mm[4]
	spRevisarPide(t, "CA-646", piloto, "test-writer", "claude", 5, id)

	rec := spCerrado(t, "CA-646", root, id)
	if rec.Role != "test-writer" || rec.Task != slug || rec.Provider != "claude" {
		t.Fatalf("CA-646: el piloto lanzo al test-writer de la tarjeta: %+v", rec)
	}
	spPiloto(t, "CA-646", root, rec)
	crudo := spCrudo(t, root, id)
	if strings.TrimSpace(string(crudo["regla"])) != `"`+boardcmd.ReglaCadena+`"` ||
		strings.TrimSpace(string(crudo["columna"])) != `"`+boardcmd.ColTestWriter+`"` {
		t.Fatalf("CA-646/CA-661: el sobre lleva la regla cadena y la columna al lanzar (test-writer): %s %s", crudo["regla"], crudo["columna"])
	}
	lnEsperarLanzamientos(t, "CA-646", s)
	aesGuardadoDeLaFirma(t, "CA-644", root, wt, slug, headPrincipal)
	if n := len(f.llamadas(t, "claude")); n != 1 {
		t.Fatalf("CA-646: el piloto lanza un test-writer y nada mas: claude corrio %d veces", n)
	}
	for _, otro := range spOtros(root, id) {
		t.Fatalf("CA-646: aprobar no lanza otro trabajo que el test-writer: %+v", otro)
	}
}

// CA-646: con auto y sin ningun provider con tope, aprobar firma igual (200
// con la respuesta de hoy), y `piloto` es la linea de la detencion; no se
// lanza nada, pero la vuelta guardo antes de decidir (CA-644).
func TestCA646_AprobarConAutoQueSeDetieneLoDice(t *testing.T) {
	f := lnPATH(t) // ningun CLI de IA instalado
	root := tbProyecto(t)
	const slug = "firma-sin-cli"
	wt := aesCarta(t, root, slug, spPresupuesto+spAuto)
	headPrincipal := spGit(t, root, "rev-parse", "HEAD")
	s := lnServidor(t, root, f)

	m := aesAprobar(t, s, slug)
	var piloto string
	if err := json.Unmarshal(m["piloto"], &piloto); err != nil || !strings.HasPrefix(piloto, spPrefijo+"se detiene: ") {
		t.Fatalf("CA-646: sin provider la respuesta trae \"piloto\" con la linea de la detencion: %q %v", piloto, err)
	}
	lnEsperarLanzamientos(t, "CA-646", s)
	if recs := envelope.List(root); len(recs) != 0 {
		t.Fatalf("CA-646: el piloto que se detiene no lanza nada: %+v", recs)
	}
	if ap := lnAprobaciones(t, wt); len(ap) != 1 {
		t.Fatalf("CA-646: la firma queda en el espacio de trabajo: %v", ap)
	}
	aesGuardadoDeLaFirma(t, "CA-644", root, wt, slug, headPrincipal)
}

// CA-646: sin auto, aprobar desde la tarjeta no lanza nada, no guarda nada, y
// la respuesta es exactamente la de hoy ({approval, already}).
func TestCA646_AprobarSinAutoEsLoDeHoy(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	const slug = "firma-a-mano"
	wt := aesCarta(t, root, slug, spPresupuesto)
	headTarea := spGit(t, wt, "rev-parse", "HEAD")
	s := lnServidor(t, root, f)

	m := aesAprobar(t, s, slug)
	if ks := aesClaves(m); strings.Join(ks, ",") != "already,approval" {
		t.Fatalf("CA-646: sin auto la respuesta es la de hoy, {approval, already}: %v", ks)
	}
	if spEsperar(time.Second, func() bool { return len(envelope.List(root)) > 0 }) {
		t.Fatalf("CA-646: sin auto aprobar no lanza nada: %+v", envelope.List(root))
	}
	lnEsperarLanzamientos(t, "CA-646", s)
	if n := len(f.llamadas(t, "claude")); n != 0 {
		t.Fatalf("CA-646: sin auto no corre ningun CLI: claude corrio %d veces", n)
	}
	if h := spGit(t, wt, "rev-parse", "HEAD"); h != headTarea {
		t.Fatalf("CA-644: sin auto no hay vuelta del piloto ni guardado: HEAD de la tarea %s -> %s", headTarea, h)
	}
	if ap := lnAprobaciones(t, wt); len(ap) != 1 {
		t.Fatalf("CA-646: la firma queda en el espacio de trabajo, como hoy: %v", ap)
	}
}
