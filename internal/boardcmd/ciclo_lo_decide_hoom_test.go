// Tests adversariales del spec .hoom/specs/ciclo-lo-decide-hoom.md sobre
// boardcmd: el tope de rondas en la tarjeta (CA-750, CA-752), en el piloto
// (CA-751), el refutador en Review (CA-753, CA-754), los bloqueantes sin
// refutar (BloqueantesSinRefutar), la frase de la pelota y el pedido del
// writer (CA-756). Escrito desde el spec, ACLARACIONES.md (1 a 7, 9 y 10) y
// el go doc de boardcmd. Las tarjetas salen de Derive sobre Evidence
// literales (bdEv, acEv, piEv); CardFor sobre disco solo para ver de que
// hoom.yaml sale el tope (aclaracion 9).
package boardcmd

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// cldRazon es la razon del contrato al tope, escrita aparte de
// RazonTopeDeRondas.
func cldRazon(n, k int) string {
	return fmt.Sprintf("se hicieron %d rondas de hallazgos y quedan %d que bloquean: te toca", n, k)
}

const cldAccionAlTope = "hoom finding list --open"

// cldRev es un registro de review de la tarjeta con cobertura cob, min
// minutos despues de bdT0, de gemini (el provider del registro mas nuevo de
// los fixtures del piloto).
func cldRev(id string, min int, cob string) reviewcmd.Record {
	return reviewcmd.Record{ID: id, CreatedAt: bdT0.Add(time.Duration(min) * time.Minute), Task: bdSlug, Spec: bdSpec,
		Provider: "gemini", Writer: "claude", Findings: []string{}, Cobertura: cob}
}

// cldRondasDesde es una review completa en el minuto desde y n deltas
// posteriores, una por minuto: n rondas.
func cldRondasDesde(desde, n int) []reviewcmd.Record {
	rs := []reviewcmd.Record{cldRev(fmt.Sprintf("r-completa-%d", desde), desde, reviewcmd.CoberturaCompleta)}
	for i := 1; i <= n; i++ {
		rs = append(rs, cldRev(fmt.Sprintf("r-delta-%d-%d", desde, i), desde+i, reviewcmd.CoberturaDelta))
	}
	return rs
}

// cldConRondas suma n rondas a ev (despues del verde de bdEv) y fija su
// tope (0 = no leido).
func cldConRondas(ev Evidence, n, tope int) Evidence {
	ev.Reviews = append(append([]reviewcmd.Record(nil), ev.Reviews...), cldRondasDesde(36, n)...)
	ev.TopeDeRondas = tope
	return ev
}

// cldRefutador es un sobre CERRADO de refutador de la tarjeta, empezado min
// minutos despues de bdT0 (antes del verde de bdEv, asi no es el rojo de la
// tarjeta), que recibio ids. En stage run cerro cortado (no-entregable).
func cldRefutador(id string, min int, stage string, ids ...string) EnvelopeState {
	ini := bdT0.Add(time.Duration(min) * time.Minute)
	status, code, paso := envelope.StatusDeliverable, 0, 5
	if stage == "run" {
		status, code, paso = envelope.StatusNotDeliverable, 1, 3
	}
	return EnvelopeState{Record: envelope.Record{ID: id, Role: "refutador", Provider: "claude", Task: bdSlug, Dir: bdWtDir,
		Spec: bdSpec, Stage: stage, Step: paso, Steps: 5, Status: status, ExitCode: code, StartedAt: ini,
		UpdatedAt: ini.Add(2 * time.Minute), EndedAt: ini.Add(2 * time.Minute), Columna: ColReview,
		Hallazgos: append([]string(nil), ids...)}}
}

// cldConSobres pone ss (el mas nuevo primero) delante de los sobres de ev.
func cldConSobres(ev Evidence, ss ...EnvelopeState) Evidence {
	ev.Envelopes = append(append([]EnvelopeState(nil), ss...), ev.Envelopes...)
	return ev
}

// cldRojo: el veredicto de la tarjeta es rojo (fallo el gate test) y no hay
// verde: la tarjeta cae en Writer.
func cldRojo(ev Evidence) Evidence {
	v := bdVeredicto("v-rojo", bdT0.Add(30*time.Minute), "huella-1", 50, 10, aeGates(map[string]string{"test": verdict.StatusFail}))
	ev.Verdict, ev.GreenVerdicts = v, nil
	return ev
}

func cldIndice(c Card, id string) int {
	for i, a := range c.Actions {
		if a.ID == id {
			return i
		}
	}
	return -1
}

// cldSinRolDeRonda exige que la tarjeta no ofrezca pedir-writer ni
// pedir-refutador (aclaracion 4: «sin acciones de rol» es sin esas dos).
func cldSinRolDeRonda(t *testing.T, ca, que string, c Card) {
	t.Helper()
	for _, id := range []string{ActPedirWriter, ActPedirRefutador} {
		if cldIndice(c, id) >= 0 {
			t.Fatalf("%s: [%s] al tope la tarjeta no ofrece %s; tiene %v", ca, que, id, acIDs(c))
		}
	}
}

// cldAlTope exige todo lo que dice la tarjeta al tope (aclaraciones 3 y 5):
// su columna sin cambiar, Rondas, Plain, Missing[0] y Next.
func cldAlTope(t *testing.T, ca, que string, c Card, col string, n, tope, k int) {
	t.Helper()
	razon := cldRazon(n, k)
	if c.Column != col {
		t.Fatalf("%s: [%s] al tope la columna no cambia: %q, fue %q (missing=%q)", ca, que, col, c.Column, c.Missing)
	}
	quiero := RondasDeHallazgos{Hechas: n, Tope: tope, AlTope: true, Razon: razon}
	if !reflect.DeepEqual(c.Rondas, quiero) {
		t.Fatalf("%s: [%s] Card.Rondas es %+v, fue %+v", ca, que, quiero, c.Rondas)
	}
	if c.Plain != razon {
		t.Fatalf("%s: [%s] Plain es la razon tal cual\n  %q\nfue\n  %q", ca, que, razon, c.Plain)
	}
	if len(c.Missing) == 0 || c.Missing[0] != razon {
		t.Fatalf("%s: [%s] la razon va primera en Missing: %q", ca, que, c.Missing)
	}
	if c.Next != cldAccionAlTope {
		t.Fatalf("%s: [%s] Next es %q, fue %q", ca, que, cldAccionAlTope, c.Next)
	}
	cldSinRolDeRonda(t, ca, que, c)
}

func cldBajoElTope(t *testing.T, ca, que string, c Card, n, tope int) {
	t.Helper()
	quiero := RondasDeHallazgos{Hechas: n, Tope: tope}
	if !reflect.DeepEqual(c.Rondas, quiero) {
		t.Fatalf("%s: [%s] bajo el tope Card.Rondas es %+v, fue %+v", ca, que, quiero, c.Rondas)
	}
	if strings.HasPrefix(c.Plain, "se hicieron ") || c.Next == cldAccionAlTope {
		t.Fatalf("%s: [%s] bajo el tope no hay razon de tope: plain %q next %q", ca, que, c.Plain, c.Next)
	}
}

// ===========================================================================
// CA-750: la tarjeta al tope
// ===========================================================================

// CA-750: RazonTopeDeRondas y AccionAlTope son los textos del contrato.
func TestCA750_TextosDelTope(t *testing.T) {
	if got := RazonTopeDeRondas(2, 3); got != "se hicieron 2 rondas de hallazgos y quedan 3 que bloquean: te toca" {
		t.Fatalf("CA-750: RazonTopeDeRondas(2, 3): %q", got)
	}
	for _, c := range [][2]int{{0, 1}, {1, 1}, {5, 12}, {17, 2}} {
		if got := RazonTopeDeRondas(c[0], c[1]); got != cldRazon(c[0], c[1]) {
			t.Fatalf("CA-750: RazonTopeDeRondas(%d, %d): %q", c[0], c[1], got)
		}
	}
	if AccionAlTope != cldAccionAlTope {
		t.Fatalf("CA-750: AccionAlTope es %q, fue %q", cldAccionAlTope, AccionAlTope)
	}
}

// CA-750: en Review, con dos rondas (el tope por defecto: Evidence.TopeDeRondas
// en 0 vale 2) y dos bloqueantes, la tarjeta sigue en Review sin pedir-writer
// ni pedir-refutador, con la razon en Plain y primera en Missing, y Next
// `hoom finding list --open`. Una ronda menos: bajo el tope (control).
func TestCA750_AlTopeEnReview(t *testing.T) {
	base := acConHallazgos(acEv(), "f-a", "f-b")
	c := Derive(cldConRondas(base, 1, 0))
	bdCol(t, "CA-750 (fixture)", c, ColReview)
	cldBajoElTope(t, "CA-750", "una ronda", c, 1, 2)
	if cldIndice(c, ActPedirWriter) < 0 {
		t.Fatalf("CA-750: fixture: bajo el tope la tarjeta ofrece pedir-writer: %v", acIDs(c))
	}

	c = Derive(cldConRondas(base, 2, 0))
	cldAlTope(t, "CA-750", "dos rondas, tope 0 = 2", c, ColReview, 2, 2, 2)
	c = Derive(cldConRondas(base, 4, 0))
	cldAlTope(t, "CA-750", "pasado el tope", c, ColReview, 4, 2, 2)
}

// CA-750: la razon cuenta solo los que bloquean: un pendiente (medium con
// block_on high) no suma a k, y Plain es la razon sin ningun sufijo.
func TestCA750_LaRazonCuentaSoloLosQueBloquean(t *testing.T) {
	ev := acConHallazgos(acEv(), "f-a", "f-b", "f-c")
	ev.Findings = append(ev.Findings, bdHallazgo("f-m", "medium", bdSlug))
	c := Derive(cldConRondas(ev, 2, 2))
	if c.Pendientes.Medium != 1 {
		t.Fatalf("CA-750: fixture: f-m es un pendiente: %+v", c.Pendientes)
	}
	cldAlTope(t, "CA-750", "con un pendiente", c, ColReview, 2, 2, 3)
}

// CA-750: Evidence.TopeDeRondas es el tope cuando no es 0: 1 llega con una
// ronda, 3 no llega con dos y si con tres, 5 no llega con cuatro.
func TestCA750_ElTopeDeLaEvidencia(t *testing.T) {
	base := acConHallazgos(acEv(), "f-a")
	cldAlTope(t, "CA-750", "tope 1, una ronda", Derive(cldConRondas(base, 1, 1)), ColReview, 1, 1, 1)
	cldBajoElTope(t, "CA-750", "tope 1, sin rondas", Derive(cldConRondas(base, 0, 1)), 0, 1)
	cldBajoElTope(t, "CA-750", "tope 3, dos rondas", Derive(cldConRondas(base, 2, 3)), 2, 3)
	cldAlTope(t, "CA-750", "tope 3, tres rondas", Derive(cldConRondas(base, 3, 3)), ColReview, 3, 3, 1)
	cldBajoElTope(t, "CA-750", "tope 5, cuatro rondas", Derive(cldConRondas(base, 4, 5)), 4, 5)
	// sin registro completa no hay rondas, por mas deltas que haya
	ev := base
	ev.Reviews = []reviewcmd.Record{cldRev("d1", 36, reviewcmd.CoberturaDelta), cldRev("d2", 37, reviewcmd.CoberturaDelta),
		cldRev("d3", 38, reviewcmd.CoberturaDelta)}
	cldBajoElTope(t, "CA-750", "deltas sin completa", Derive(ev), 0, 2)
}

// CA-750 y aclaracion 4: al tope, con la review exigida, pedir-reviewer sigue
// disponible (es como se sale despues de corregir a mano); pedir-writer y
// pedir-refutador no.
func TestCA750_AlTopePedirReviewerSigue(t *testing.T) {
	c := Derive(cldConRondas(acConHallazgos(acGrande(acEv()), "f-a"), 2, 2))
	cldAlTope(t, "CA-750", "review exigida", c, ColReview, 2, 2, 1)
	if a, ok := c.Action(ActPedirReviewer); !ok || !a.Enabled {
		t.Fatalf("CA-750: al tope pedir-reviewer sigue disponible: %+v (%v)", a, acIDs(c))
	}
}

// CA-750 y aclaracion 3: el tope vale en cualquier columna. En Writer (sin
// veredicto, o con el veredicto rojo) con bloqueantes, al tope la tarjeta se
// queda en Writer, sin pedir-writer, con la razon y Next del tope.
func TestCA750_AlTopeEnWriter(t *testing.T) {
	for _, f := range []acFix{
		{"writer sin veredicto", acConHallazgos(acWriter(), "f-a", "f-b")},
		{"writer con el veredicto rojo", cldRojo(acConHallazgos(acEv(), "f-a", "f-b"))},
	} {
		bajo := Derive(cldConRondas(f.ev, 1, 2))
		bdCol(t, "CA-750 (fixture "+f.nombre+")", bajo, ColWriter)
		if cldIndice(bajo, ActPedirWriter) < 0 {
			t.Fatalf("CA-750: fixture %s: bajo el tope ofrece pedir-writer: %v", f.nombre, acIDs(bajo))
		}
		cldAlTope(t, "CA-750", f.nombre, Derive(cldConRondas(f.ev, 2, 2)), ColWriter, 2, 2, 2)
	}
}

// CA-750 y aclaracion 3: al tope no hay reanudar ni relanzar de un trabajo
// cortado del writer o del refutador; los de otro rol (el reviewer) quedan
// como estan.
func TestCA750_AlTopeSinReanudarNiRelanzarDeLaRonda(t *testing.T) {
	ev := acInterrumpido(acConHallazgos(acWriter(), "f-a"), "writer", "claude")
	bajo := Derive(cldConRondas(ev, 0, 2))
	for _, id := range []string{ActReanudar, ActRelanzar} {
		if cldIndice(bajo, id) < 0 {
			t.Fatalf("CA-750: fixture: el writer cortado bajo el tope tiene %s: %v", id, acIDs(bajo))
		}
	}
	for _, rol := range []string{"writer", "refutador"} {
		base := acConHallazgos(acWriter(), "f-a")
		if rol == "refutador" {
			base = acConHallazgos(acEv(), "f-a")
		}
		c := Derive(cldConRondas(acInterrumpido(base, rol, "claude"), 2, 2))
		cldSinRolDeRonda(t, "CA-750", rol+" cortado", c)
		for _, id := range []string{ActReanudar, ActRelanzar} {
			if cldIndice(c, id) >= 0 {
				t.Fatalf("CA-750: al tope no hay %s del %s cortado: %v", id, rol, acIDs(c))
			}
		}
		if !c.Rondas.AlTope || c.Rondas.Razon != cldRazon(2, 1) {
			t.Fatalf("CA-750: el %s cortado no saca la tarjeta del tope: %+v", rol, c.Rondas)
		}
	}
	rev := acInterrumpido(acConHallazgos(acGrande(acEv()), "f-a"), "reviewer", "claude")
	bajoRev, alRev := Derive(cldConRondas(rev, 0, 2)), Derive(cldConRondas(rev, 2, 2))
	for _, id := range []string{ActReanudar, ActRelanzar} {
		if cldIndice(bajoRev, id) >= 0 && cldIndice(alRev, id) < 0 {
			t.Fatalf("CA-750: el %s de un reviewer cortado sigue al tope (aclaracion 3): bajo %v, al tope %v", id, acIDs(bajoRev), acIDs(alRev))
		}
	}
}

// CA-750 y aclaracion 5: Card.Rondas nunca va al JSON de la tarjeta.
func TestCA750_RondasNoVaAlJSON(t *testing.T) {
	c := Derive(cldConRondas(acConHallazgos(acEv(), "f-a"), 3, 2))
	if !c.Rondas.AlTope {
		t.Fatalf("CA-750: fixture: al tope: %+v", c.Rondas)
	}
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for k := range m {
		if strings.EqualFold(k, "rondas") || strings.EqualFold(k, "hechas") || strings.EqualFold(k, "al_tope") {
			t.Fatalf("CA-750: el JSON de la tarjeta no lleva %q: %s", k, raw)
		}
	}
	if !strings.Contains(string(raw), cldRazon(3, 1)) {
		t.Fatalf("CA-750: la tarjeta muestra el tope por plain/missing: %s", raw)
	}
}

// CA-750 (propiedad): para n rondas, tope t de 1 a 5 y k bloqueantes de 0
// a 3, AlTope es n >= t y k > 0; Razon es la del contrato solo al tope.
func TestCA750_PropiedadAlTope(t *testing.T) {
	prop := func(rn, rt, rk uint8) bool {
		n, tope, k := int(rn%7), int(rt%5)+1, int(rk%4)
		ev := acEv()
		for i := 0; i < k; i++ {
			ev = acConHallazgos(ev, fmt.Sprintf("f-%d", i))
		}
		c := Derive(cldConRondas(ev, n, tope))
		al := n >= tope && k > 0
		if c.Rondas.Hechas != n || c.Rondas.Tope != tope || c.Rondas.AlTope != al {
			return false
		}
		if al {
			return c.Rondas.Razon == cldRazon(n, k) && c.Plain == c.Rondas.Razon && c.Next == cldAccionAlTope &&
				cldIndice(c, ActPedirWriter) < 0 && cldIndice(c, ActPedirRefutador) < 0
		}
		return c.Rondas.Razon == ""
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 120}); err != nil {
		t.Fatalf("CA-750: %v", err)
	}
}

// ===========================================================================
// CA-752: Rondas >= tope sin bloqueantes: como hoy
// ===========================================================================

func cldMismaTarjeta(t *testing.T, ca, que string, got, quiero Card) {
	t.Helper()
	if got.Column != quiero.Column || !reflect.DeepEqual(acIDs(got), acIDs(quiero)) || got.Plain != quiero.Plain ||
		!reflect.DeepEqual(got.Missing, quiero.Missing) || got.Next != quiero.Next {
		t.Fatalf("%s: [%s] sin bloqueantes la tarjeta es la de hoy:\n quiero col=%s acciones=%v plain=%q missing=%q next=%q\n fue    col=%s acciones=%v plain=%q missing=%q next=%q",
			ca, que, quiero.Column, acIDs(quiero), quiero.Plain, quiero.Missing, quiero.Next,
			got.Column, acIDs(got), got.Plain, got.Missing, got.Next)
	}
}

// CA-752 (caso limite): al tope sin bloqueantes no pasa nada: Tu aceptacion
// sigue en Tu aceptacion con integrar, y un pendiente tampoco es un
// bloqueante.
func TestCA752_SinBloqueantesSigueSuCamino(t *testing.T) {
	c := Derive(cldConRondas(acEv(), 3, 2))
	bdCol(t, "CA-752", c, ColTuAceptacion)
	if len(c.Actions) == 0 || c.Actions[0].ID != ActIntegrar {
		t.Fatalf("CA-752: al tope sin bloqueantes la primera accion es integrar: %v", acIDs(c))
	}
	if c.Rondas.AlTope || c.Rondas.Razon != "" || c.Rondas.Hechas != 3 || c.Rondas.Tope != 2 {
		t.Fatalf("CA-752: sin bloqueantes no esta al tope: %+v", c.Rondas)
	}
	ev := acEv()
	ev.Findings = []finding.Item{bdHallazgo("f-m", "medium", bdSlug), bdHallazgo("f-l", "low", bdSlug)}
	c = Derive(cldConRondas(ev, 5, 1))
	bdCol(t, "CA-752", c, ColTuAceptacion)
	if c.Rondas.AlTope {
		t.Fatalf("CA-752: los pendientes no ponen la tarjeta al tope: %+v", c.Rondas)
	}
}

// CA-752 (propiedad): en cualquier columna sin bloqueantes, la tarjeta con
// n rondas y tope t <= n es la misma (columna, acciones, plain, missing,
// next) que con un tope que no alcanza.
func TestCA752_PropiedadSinBloqueantesComoHoy(t *testing.T) {
	cuando := bdT0.Add(48 * time.Hour)
	hecho := acEv()
	hecho.Item.HechoEn = &cuando
	pendiente := acEv()
	pendiente.Findings = []finding.Item{bdHallazgo("f-m", "medium", bdSlug)}
	fixtures := []acFix{
		{"tu-aceptacion", acEv()}, {"review exigida", acGrande(acEv())}, {"writer", acWriter()},
		{"test-writer", acTestWriter()}, {"hecho", hecho}, {"con un pendiente", pendiente},
		{"writer cortado", acInterrumpido(acWriter(), "writer", "claude")},
		{"sin espacio propio", bdArbol(acEv())},
	}
	prop := func(rf, rn, rt uint8) bool {
		f := fixtures[int(rf)%len(fixtures)]
		n := int(rn % 6)
		tope := int(rt%5) + 1
		if tope > n {
			n = tope
		}
		got := Derive(cldConRondas(f.ev, n, tope))
		quiero := Derive(cldConRondas(f.ev, n, n+1))
		if got.Rondas.AlTope || got.Rondas.Razon != "" {
			t.Errorf("CA-752: [%s] sin bloqueantes no esta al tope: %+v", f.nombre, got.Rondas)
			return false
		}
		cldMismaTarjeta(t, "CA-752", fmt.Sprintf("%s, %d rondas, tope %d", f.nombre, n, tope), got, quiero)
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 80}); err != nil {
		t.Fatalf("CA-752: %v", err)
	}
}

// ===========================================================================
// CA-753: el refutador en Review, bajo el tope
// ===========================================================================

// CA-753 y aclaracion 1: sin review exigida, con bloqueantes sin refutar, la
// primera accion es pedir-refutador; pedir-writer sigue disponible despues
// (aclaracion 2). Sus Hallazgos son los bloqueantes en el orden de la
// tarjeta.
func TestCA753_PrimeraAccionPedirRefutador(t *testing.T) {
	ev := acConHallazgos(acEv(), "f-a", "f-b", "f-c")
	// En riesgo normal: pedir-writer va primero y pedir-refutador sigue disponible a mano.
	cn := Derive(ev)
	bdCol(t, "CA-753 (normal)", cn, ColReview)
	if len(cn.Actions) < 2 || cn.Actions[0].ID != ActPedirWriter || cn.Actions[1].ID != ActPedirRefutador {
		t.Fatalf("CA-753: en normal la primera accion es pedir-writer y luego pedir-refutador: %v", acIDs(cn))
	}
	// En riesgo alto: pedir-refutador es la primera accion.
	ev.Item.Riesgo = "alto"
	r := acRegistroQueCuenta()
	r.VerdictID = ev.Verdict.ID
	ev.Reviews = []reviewcmd.Record{r}
	c := Derive(bdColas(ev))
	bdCol(t, "CA-753", c, ColReview)
	if len(c.Actions) == 0 || c.Actions[0].ID != ActPedirRefutador {
		t.Fatalf("CA-753: en alto la primera accion es pedir-refutador: %v", acIDs(c))
	}
	a := c.Actions[0]
	if a.Role != "refutador" || !a.Enabled || !IsRoleAction(a.ID) {
		t.Fatalf("CA-753: pedir-refutador lanza al refutador y esta habilitada: %+v", a)
	}
	if !reflect.DeepEqual(a.Hallazgos, []string{"f-a", "f-b", "f-c"}) {
		t.Fatalf("CA-753: Hallazgos son los bloqueantes sin refutar en orden: %q", a.Hallazgos)
	}
	if !reflect.DeepEqual(c.Evidence.BlockingFindings, []string{"f-a", "f-b", "f-c"}) {
		t.Fatalf("CA-753: fixture: blocking_findings: %q", c.Evidence.BlockingFindings)
	}
	iw := cldIndice(c, ActPedirWriter)
	if iw <= 0 {
		t.Fatalf("CA-753 (aclaracion 2): pedir-writer sigue disponible despues de pedir-refutador: %v", acIDs(c))
	}
	if w, _ := c.Action(ActPedirWriter); !w.Enabled {
		t.Fatalf("CA-753 (aclaracion 2): pedir-writer no se deshabilita por los bloqueantes sin refutar: %+v", w)
	}
	if got, ok := c.Action(ActPedirRefutador); !ok || !reflect.DeepEqual(got, a) {
		t.Fatalf("CA-753: Card.Action(pedir-refutador) es la misma accion: %+v %v", got, ok)
	}
}

// CA-753 y aclaracion 1: con la review exigida y bloqueantes, el orden es
// pedir-reviewer, pedir-refutador, pedir-writer.
func TestCA753_OrdenConLaReviewExigida(t *testing.T) {
	ev := acConHallazgos(acGrande(acEv()), "f-a")
	ids := acIDs(Derive(ev))
	if len(ids) < 3 || ids[0] != ActPedirReviewer || ids[1] != ActPedirWriter || ids[2] != ActPedirRefutador {
		t.Fatalf("CA-753: en normal con la review exigida el orden es pedir-reviewer, pedir-writer, pedir-refutador: %v", ids)
	}
	ev.Item.Riesgo = "alto"
	c := Derive(ev)
	bdCol(t, "CA-753", c, ColReview)
	ids = acIDs(c)
	if len(ids) < 3 || ids[0] != ActPedirReviewer || ids[1] != ActPedirRefutador || ids[2] != ActPedirWriter {
		t.Fatalf("CA-753: en alto con la review exigida el orden es pedir-reviewer, pedir-refutador, pedir-writer: %v", ids)
	}
}

// CA-753: cuando todos los bloqueantes ya pasaron por un refutador (un sobre
// cerrado fuera de run), no hay pedir-refutador y la primera es pedir-writer,
// como hoy; con la review exigida, pedir-reviewer y despues pedir-writer.
func TestCA753_TodosRefutadosLaPrimeraEsPedirWriter(t *testing.T) {
	ev := cldConSobres(acConHallazgos(acEv(), "f-a", "f-b"), cldRefutador("env-ref-1", 10, "ok", "f-a", "f-b"))
	c := Derive(ev)
	bdCol(t, "CA-753", c, ColReview)
	acNoTiene(t, "CA-753", c, ActPedirRefutador)
	if len(c.Actions) == 0 || c.Actions[0].ID != ActPedirWriter {
		t.Fatalf("CA-753: todos refutados: la primera es pedir-writer: %v", acIDs(c))
	}
	// repartidos en dos sobres, uno en verify (no es run: cuenta)
	ev = cldConSobres(acConHallazgos(acEv(), "f-a", "f-b"), cldRefutador("env-ref-2", 20, "verify", "f-b"),
		cldRefutador("env-ref-1", 10, "ok", "f-a", "f-x"))
	c = Derive(ev)
	acNoTiene(t, "CA-753", c, ActPedirRefutador)
	if len(c.Actions) == 0 || c.Actions[0].ID != ActPedirWriter {
		t.Fatalf("CA-753: refutados en dos sobres: la primera es pedir-writer: %v", acIDs(c))
	}
	ev = cldConSobres(acConHallazgos(acGrande(acEv()), "f-a"), cldRefutador("env-ref-1", 10, "ok", "f-a"))
	ids := acIDs(Derive(ev))
	if len(ids) < 2 || ids[0] != ActPedirReviewer || ids[1] != ActPedirWriter {
		t.Fatalf("CA-753: review exigida y todos refutados: pedir-reviewer, pedir-writer: %v", ids)
	}
}

// CA-753 (caso limite): un bloqueante nuevo que trae la delta es un
// bloqueante sin refutar: pedir-refutador vuelve, solo con el.
func TestCA753_UnBloqueanteNuevoVuelveAlRefutador(t *testing.T) {
	ev := cldConSobres(acConHallazgos(acEv(), "f-a", "f-nuevo", "f-b"), cldRefutador("env-ref-1", 10, "ok", "f-a", "f-b"))
	ev.Item.Riesgo = "alto"
	r := acRegistroQueCuenta()
	r.VerdictID = ev.Verdict.ID
	ev.Reviews = []reviewcmd.Record{r}
	c := Derive(bdColas(cldConRondas(ev, 1, 2)))
	a := acAccion(t, "CA-753", c, ActPedirRefutador)
	if c.Actions[0].ID != ActPedirRefutador || !reflect.DeepEqual(a.Hallazgos, []string{"f-nuevo"}) {
		t.Fatalf("CA-753: el bloqueante nuevo va solo al refutador, primero: %v %q", acIDs(c), a.Hallazgos)
	}
}

// CA-753 (caso limite) y CA-754: un sobre de refutador cortado en run no
// cuenta como recibido; tampoco los sobres de otros roles con hallazgos.
func TestCA753_CortadoEnRunYOtrosRolesNoCuentan(t *testing.T) {
	escritor := cldRefutador("env-w", 12, "ok", "f-a")
	escritor.Record.Role = "writer"
	revisor := cldRefutador("env-r", 14, "ok", "f-b")
	revisor.Record.Role = "reviewer"
	ev := cldConSobres(acConHallazgos(acEv(), "f-a", "f-b"), revisor, escritor, cldRefutador("env-cortado", 5, "run", "f-a", "f-b"))
	ev.Item.Riesgo = "alto"
	r := acRegistroQueCuenta()
	r.VerdictID = ev.Verdict.ID
	ev.Reviews = []reviewcmd.Record{r}
	c := Derive(bdColas(ev))
	a := acAccion(t, "CA-753", c, ActPedirRefutador)
	if c.Actions[0].ID != ActPedirRefutador || !reflect.DeepEqual(a.Hallazgos, []string{"f-a", "f-b"}) {
		t.Fatalf("CA-753: el cortado en run y los otros roles no cuentan: %v %q", acIDs(c), a.Hallazgos)
	}
}

// CA-753 y aclaracion 6: Derive mira solo los sobres CERRADOS: un refutador
// abierto (cortado sin cerrar, en verify) todavia no recibio nada.
func TestCA753_UnSobreAbiertoNoCuenta(t *testing.T) {
	abierto := EnvelopeState{Record: envelope.Record{ID: "env-ref-abierto", Role: "refutador", Provider: "claude", Task: bdSlug,
		Dir: bdWtDir, Spec: bdSpec, Stage: "verify", Step: 4, Steps: 5, Status: envelope.StatusRunning,
		StartedAt: bdT0.Add(10 * time.Minute), UpdatedAt: bdT0.Add(12 * time.Minute), Columna: ColReview,
		Hallazgos: []string{"f-a"}}}
	c := Derive(cldConSobres(acConHallazgos(acEv(), "f-a", "f-b"), abierto))
	a := acAccion(t, "CA-753", c, ActPedirRefutador)
	if !reflect.DeepEqual(a.Hallazgos, []string{"f-a", "f-b"}) {
		t.Fatalf("CA-753: un sobre de refutador sin cerrar no cuenta como recibido: %q", a.Hallazgos)
	}
}

// CA-753: sin bloqueantes, en Review no aparece ninguna de las dos.
func TestCA753_SinBloqueantesNingunaDeLasDos(t *testing.T) {
	ev := acGrande(acEv())
	ev.Findings = []finding.Item{bdHallazgo("f-m", "medium", bdSlug)}
	c := Derive(ev)
	bdCol(t, "CA-753", c, ColReview)
	acNoTiene(t, "CA-753", c, ActPedirRefutador)
	acNoTiene(t, "CA-753", c, ActPedirWriter)
}

// CA-753 (caso limite): fuera de Review la tarjeta no ofrece pedir-refutador
// (Writer con bloqueantes, Tu aceptacion, Test-writer, Backlog, Hecho).
func TestCA753_FueraDeReviewNoHayPedirRefutador(t *testing.T) {
	cuando := bdT0.Add(48 * time.Hour)
	hecho := acConHallazgos(acEv(), "f-a")
	hecho.Item.HechoEn = &cuando
	sinSpec := acEv()
	sinSpec.SpecExists = false
	for _, f := range []acFix{
		{"writer con bloqueantes", acConHallazgos(acWriter(), "f-a")},
		{"writer rojo con bloqueantes", cldRojo(acConHallazgos(acEv(), "f-a"))},
		{"tu-aceptacion", acEv()}, {"test-writer", acTestWriter()}, {"backlog", sinSpec}, {"hecho", hecho},
	} {
		c := Derive(f.ev)
		if c.Column == ColReview {
			t.Fatalf("CA-753: fixture %s: fuera de Review, cayo en %s", f.nombre, c.Column)
		}
		acNoTiene(t, "CA-753 ("+f.nombre+")", c, ActPedirRefutador)
	}
}

// CA-753: pedir-refutador lanza un rol: IsRoleAction lo dice, como de
// pedir-writer, reanudar y relanzar, y no de integrar ni guardar.
func TestCA753_IsRoleAction(t *testing.T) {
	for _, id := range []string{ActPedirRefutador, ActPedirWriter, ActPedirReviewer, ActPedirTestWriter, ActPedirArquitecto, ActReanudar, ActRelanzar} {
		if !IsRoleAction(id) {
			t.Fatalf("CA-753: %s lanza un rol", id)
		}
	}
	for _, id := range []string{ActIntegrar, ActAprobar, ActGuardar, ActDescartar, ActSesion, ActTerminal, "", "pedir-orquestador"} {
		if IsRoleAction(id) {
			t.Fatalf("CA-753: %q no lanza un rol", id)
		}
	}
}

// CA-753 y aclaracion 7: la pelota de una tarjeta cuyo paso es
// pedir-refutador dice `Pedile al refutador que revise los hallazgos de
// «<titulo>».`, por FraseDe y en la pelota de la tarjeta (su paso es su
// primera accion, que lanza un rol: AccionDelPaso).
func TestCA753_FraseDeLaPelota(t *testing.T) {
	ev := acConHallazgos(acEv(), "f-a")
	ev.Item.Riesgo = "alto"
	r := acRegistroQueCuenta()
	r.VerdictID = ev.Verdict.ID
	ev.Reviews = []reviewcmd.Record{r}
	c := Derive(bdColas(ev))
	quiero := "Pedile al refutador que revise los hallazgos de «Precios por region»."
	if got := FraseDe(&c, GrupoTeToca, ActPedirRefutador, Piel{}); got != quiero {
		t.Fatalf("CA-753: FraseDe con pedir-refutador: %q, fue %q", quiero, got)
	}
	if got := AccionDelPaso(&c); got != ActPedirRefutador {
		t.Fatalf("CA-753: el paso de la tarjeta es pedir-refutador, fue %q", got)
	}
	if p := PelotaDe(&c, Piel{}); p.Grupo != GrupoTeToca || p.Paso.Accion != ActPedirRefutador || p.Frase != quiero {
		t.Fatalf("CA-753: la pelota de la tarjeta: grupo %q paso %q frase %q", p.Grupo, p.Paso.Accion, p.Frase)
	}
	cita := Piel{Citar: func(s string) string { return "\"" + s + "\"" }}
	if got := FraseDe(&c, GrupoTeToca, ActPedirRefutador, cita); got != "Pedile al refutador que revise los hallazgos de \"Precios por region\"." {
		t.Fatalf("CA-753: la frase cita el titulo con la piel: %q", got)
	}
}

// CA-753 y CA-754: en el JSON, pedir-refutador lleva sus hallazgos y las
// otras acciones no llevan la clave.
func TestCA753_HallazgosEnElJSON(t *testing.T) {
	c := Derive(acConHallazgos(acEv(), "f-a", "f-b"))
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Actions []map[string]json.RawMessage `json:"actions"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	vistas := 0
	for _, a := range m.Actions {
		var id string
		json.Unmarshal(a["id"], &id)
		h, tiene := a["hallazgos"]
		switch {
		case id == ActPedirRefutador:
			vistas++
			var ids []string
			if !tiene || json.Unmarshal(h, &ids) != nil || !reflect.DeepEqual(ids, []string{"f-a", "f-b"}) {
				t.Fatalf("CA-754: pedir-refutador lleva hallazgos [f-a, f-b] en el JSON: %s", raw)
			}
		case tiene:
			t.Fatalf("CA-754: la accion %s no lleva hallazgos en el JSON: %s", id, raw)
		}
	}
	if vistas != 1 {
		t.Fatalf("CA-753: el JSON trae pedir-refutador una vez: %s", raw)
	}
}

// ===========================================================================
// BloqueantesSinRefutar: la regla unica (CA-753, CA-754)
// ===========================================================================

func cldRec(rol, stage string, ids ...string) envelope.Record {
	return envelope.Record{ID: "e-" + rol + "-" + stage, Role: rol, Stage: stage, Task: bdSlug, Hallazgos: ids}
}

// CA-754: los bloqueantes, en su orden, que no estan en los hallazgos de
// ningun sobre de refutador fuera de run; nil cuando no queda ninguno.
func TestCA754_BloqueantesSinRefutar(t *testing.T) {
	casos := []struct {
		nombre string
		bloq   []string
		sobres []envelope.Record
		quiero []string
	}{
		{"sin sobres", []string{"f1", "f2", "f3"}, nil, []string{"f1", "f2", "f3"}},
		{"uno refutado", []string{"f1", "f2", "f3"}, []envelope.Record{cldRec("refutador", "ok", "f2")}, []string{"f1", "f3"}},
		{"cortado en run", []string{"f1", "f2"}, []envelope.Record{cldRec("refutador", "run", "f1", "f2")}, []string{"f1", "f2"}},
		{"otros roles", []string{"f1", "f2"}, []envelope.Record{cldRec("writer", "ok", "f1"), cldRec("reviewer", "ok", "f2"),
			cldRec("test-writer", "verify", "f1", "f2")}, []string{"f1", "f2"}},
		{"el orden de los bloqueantes", []string{"f3", "f1", "f2"}, []envelope.Record{cldRec("refutador", "scope", "f1")}, []string{"f3", "f2"}},
		{"ids que ya no bloquean", []string{"f1"}, []envelope.Record{cldRec("refutador", "ok", "f9", "f8")}, []string{"f1"}},
		{"en verify y en check cuentan", []string{"f1", "f2", "f3"}, []envelope.Record{cldRec("refutador", "verify", "f1"),
			cldRec("refutador", "check", "f3")}, []string{"f2"}},
	}
	for _, c := range casos {
		if got := BloqueantesSinRefutar(c.bloq, c.sobres); !reflect.DeepEqual(got, c.quiero) {
			t.Fatalf("CA-754: [%s] BloqueantesSinRefutar(%q) es %q, fue %q", c.nombre, c.bloq, c.quiero, got)
		}
	}
	if got := BloqueantesSinRefutar([]string{"f1", "f2"}, []envelope.Record{cldRec("refutador", "ok", "f2", "f1")}); got != nil {
		t.Fatalf("CA-754: todos refutados es nil, fue %#v", got)
	}
	if got := BloqueantesSinRefutar(nil, []envelope.Record{cldRec("refutador", "ok", "f1")}); got != nil {
		t.Fatalf("CA-754: sin bloqueantes es nil, fue %#v", got)
	}
	bloq := []string{"f1", "f2"}
	BloqueantesSinRefutar(bloq, []envelope.Record{cldRec("refutador", "ok", "f1")})
	if !reflect.DeepEqual(bloq, []string{"f1", "f2"}) {
		t.Fatalf("CA-754: BloqueantesSinRefutar no toca su entrada: %q", bloq)
	}
}

// CA-754 (propiedad): la regla contra una referencia escrita aparte, con
// roles, stages y hallazgos al azar.
func TestCA754_PropiedadBloqueantesSinRefutar(t *testing.T) {
	pool := []string{"f1", "f2", "f3", "f4", "f5", "f6"}
	roles := []string{"refutador", "writer", "reviewer", "test-writer"}
	stages := []string{"run", "ok", "verify", "scope", "check", "aislar", ""}
	prop := func(semilla int64) bool {
		r := rand.New(rand.NewSource(semilla))
		var bloq []string
		for _, id := range r.Perm(len(pool))[:r.Intn(len(pool)+1)] {
			bloq = append(bloq, pool[id])
		}
		var sobres []envelope.Record
		for i := r.Intn(5); i > 0; i-- {
			var hs []string
			for _, id := range r.Perm(len(pool))[:r.Intn(3)] {
				hs = append(hs, pool[id])
			}
			sobres = append(sobres, cldRec(roles[r.Intn(len(roles))], stages[r.Intn(len(stages))], hs...))
		}
		recibido := map[string]bool{}
		for _, s := range sobres {
			if s.Role == "refutador" && s.Stage != "run" {
				for _, h := range s.Hallazgos {
					recibido[h] = true
				}
			}
		}
		var quiero []string
		for _, b := range bloq {
			if !recibido[b] {
				quiero = append(quiero, b)
			}
		}
		got := BloqueantesSinRefutar(bloq, sobres)
		if len(quiero) == 0 {
			return got == nil
		}
		return reflect.DeepEqual(got, quiero)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 400}); err != nil {
		t.Fatalf("CA-754: %v", err)
	}
}

// ===========================================================================
// CA-754: el pedido del refutador
// ===========================================================================

// CA-754: PedidoRondaRefutador es el texto del contrato.
func TestCA754_PedidoRondaRefutadorLiteral(t *testing.T) {
	quiero := "Ronda 3 de hallazgos de precios. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: f-x, f-y, f-z. No edites codigo."
	if got := PedidoRondaRefutador(3, "precios", []string{"f-x", "f-y", "f-z"}); got != quiero {
		t.Fatalf("CA-754: PedidoRondaRefutador:\n  %q\nfue\n  %q", quiero, got)
	}
}

// CA-754: el Pedido de pedir-refutador es PedidoRondaRefutador(Hechas+1,
// slug, ids): ronda 1 sin rondas, ronda 2 tras una delta, con los ids sin
// refutar.
func TestCA754_ElPedidoDeLaAccion(t *testing.T) {
	ev := acConHallazgos(acEv(), "f-a", "f-b")
	a := acAccion(t, "CA-754", Derive(ev), ActPedirRefutador)
	if a.Pedido != PedidoRondaRefutador(1, bdSlug, []string{"f-a", "f-b"}) {
		t.Fatalf("CA-754: sin rondas el pedido es la ronda 1: %q", a.Pedido)
	}
	c := Derive(cldConRondas(cldConSobres(acConHallazgos(acEv(), "f-a", "f-b", "f-c"), cldRefutador("env-ref", 10, "ok", "f-b")), 1, 3))
	a = acAccion(t, "CA-754", c, ActPedirRefutador)
	quiero := "Ronda 2 de hallazgos de precios. Revisa estos hallazgos y cierra como refutado, con evidencia deterministica, los que no se sostienen: f-a, f-c. No edites codigo."
	if c.Rondas.Hechas != 1 || a.Pedido != quiero || a.Pedido != PedidoRondaRefutador(c.Rondas.Hechas+1, bdSlug, a.Hallazgos) {
		t.Fatalf("CA-754: tras una delta el pedido es la ronda 2 con los sin refutar:\n  %q\nfue\n  %q (rondas %+v)", quiero, a.Pedido, c.Rondas)
	}
	if !reflect.DeepEqual(a.Hallazgos, []string{"f-a", "f-c"}) {
		t.Fatalf("CA-754: Hallazgos son los que lista el pedido: %q", a.Hallazgos)
	}
	for _, otra := range c.Actions {
		if otra.ID != ActPedirRefutador && otra.Hallazgos != nil {
			t.Fatalf("CA-754: solo pedir-refutador lleva Hallazgos: %s %q", otra.ID, otra.Hallazgos)
		}
	}
}

// ===========================================================================
// CA-756: el pedido del writer en Review
// ===========================================================================

// CA-756: PedidoCorreccion es el texto del contrato, con los ids unidos con
// ", ".
func TestCA756_PedidoCorreccionLiteral(t *testing.T) {
	quiero := "Corregi los hallazgos que bloquean la tarjeta: f-a, f-b. Solo esos: los pendientes quedan abiertos."
	if got := PedidoCorreccion([]string{"f-a", "f-b"}); got != quiero {
		t.Fatalf("CA-756: PedidoCorreccion:\n  %q\nfue\n  %q", quiero, got)
	}
	if got := PedidoCorreccion([]string{"f-z"}); got != "Corregi los hallazgos que bloquean la tarjeta: f-z. Solo esos: los pendientes quedan abiertos." {
		t.Fatalf("CA-756: PedidoCorreccion de uno: %q", got)
	}
}

// CA-756: el pedido de pedir-writer en Review con bloqueantes lleva TODOS
// los que bloquean (refutador o no), nunca un pendiente, en el orden de la
// tarjeta; con la review exigida, tambien.
func TestCA756_ElPedidoDelWriterEnReview(t *testing.T) {
	ev := acConHallazgos(acEv(), "f-a", "f-b")
	ev.Findings = append(ev.Findings, bdHallazgo("f-m", "medium", bdSlug))
	for _, f := range []acFix{
		{"sin refutar", ev},
		{"uno refutado", cldConSobres(ev, cldRefutador("env-ref", 10, "ok", "f-a"))},
		{"todos refutados", cldConSobres(ev, cldRefutador("env-ref", 10, "ok", "f-a", "f-b"))},
		{"review exigida", acGrande(ev)},
		{"bajo el tope, con una ronda", cldConRondas(ev, 1, 2)},
	} {
		c := Derive(f.ev)
		bdCol(t, "CA-756 (fixture "+f.nombre+")", c, ColReview)
		w := acAccion(t, "CA-756", c, ActPedirWriter)
		if w.Pedido != PedidoCorreccion([]string{"f-a", "f-b"}) {
			t.Fatalf("CA-756: [%s] el pedido del writer en Review:\n  %q\nfue\n  %q", f.nombre, PedidoCorreccion([]string{"f-a", "f-b"}), w.Pedido)
		}
		if strings.Contains(w.Pedido, "f-m") {
			t.Fatalf("CA-756: [%s] el pendiente no va en el pedido del writer: %q", f.nombre, w.Pedido)
		}
	}
}

// ===========================================================================
// CA-751, CA-752, CA-754: el piloto y el tope
// ===========================================================================

// cldEvRonda es la tarjeta de una ronda del piloto: en Review por sus
// bloqueantes (o, sin ellos, con la review exigida).
func cldEvRonda(bloq []finding.Item) Evidence {
	ev := piEv("tu-aceptacion", piConTope, piNada)
	ev.Item.Riesgo = "alto"
	if len(bloq) == 0 {
		ev = piEv("review-exigida", piConTope, piNada)
		ev.Item.Riesgo = "alto"
	}
	ev.Findings = append([]finding.Item(nil), bloq...)
	return ev
}

// cldEstado: lo que el piloto ve con n rondas (una completa y n deltas,
// despues de los dos registros de aeRonda; el mas nuevo de gemini). topeEv
// es el tope que leyo el tablero (Evidence.TopeDeRondas) y topeE el del
// Estado.
func cldEstado(ev Evidence, pol manifest.AutonomiaPolicy, n, topeEv, topeE int, xs ...aeS) Estado {
	ev.Reviews = append([]reviewcmd.Record{aeRev("r-viejo", "codex", 35), aeRev("r-nuevo", "gemini", 50)}, cldRondasDesde(51, n)...)
	ev.TopeDeRondas = topeEv
	e := aeEstado(ev, pol, aeSobres(xs...))
	e.TopeDeRondas = topeE
	return e
}

var cldPol2 = manifest.AutonomiaPolicy{RondasDeHallazgos: 2}

// cldLanzaRefutador exige que el piloto lance al refutador de una ronda
// con esos hallazgos. No fija Ronda ni Pedido: con deltas registradas el spec
// no dice si la ronda del piloto es la suya o Rondas+1.
func cldLanzaRefutador(t *testing.T, ca, nombre string, d PilotDecision, hallazgos []string) {
	t.Helper()
	if !d.Launch || d.Paso != PasoLanzar || d.Why != "" || d.Action != ActPedirRefutador || d.Role != "refutador" ||
		d.Regla != ReglaRondaRefutador || d.Caso != 7 || !reflect.DeepEqual(d.Hallazgos, hallazgos) {
		t.Fatalf("%s: [%s] el piloto lanza al refutador de la ronda con %q, fue %+v", ca, nombre, hallazgos, d)
	}
}

// cldTrasWriter es la tarjeta en Writer tras los sobres de writer dados (el
// veredicto de la tarjeta es el del ultimo), con bloq de bloqueantes y n
// rondas; los dos topes en 2.
func cldTrasWriter(pol manifest.AutonomiaPolicy, bloq []finding.Item, n int, writers ...aeS) Estado {
	ev := piEv("writer", piConTope, piNada)
	ev.Findings = append([]finding.Item(nil), bloq...)
	xs := []aeS{{rol: "test-writer", prov: "claude", col: ColTestWriter, stage: "verify", status: aeNo, v: aeRojoTest("v-tw")}}
	xs = append(xs, writers...)
	if v := writers[len(writers)-1].v; v != nil {
		ev.Verdict = v
	}
	ev.Reviews = cldRondasDesde(51, n)
	ev.TopeDeRondas = 2
	e := aeEstado(ev, pol, aeSobres(xs...))
	e.TopeDeRondas = 2
	return e
}

// CA-751: al tope el piloto para con la razon de CA-750 y el caso 7, donde
// sin el tope empezaria la ronda con el refutador (control).
func TestCA751_ElPilotoParaAlTope(t *testing.T) {
	bloq := []finding.Item{aeF1, aeF2}
	e := cldEstado(cldEvRonda(bloq), cldPol2, 1, 2, 2, aeBase()...)
	aeCol(t, "CA-751", e, ColReview)
	cldLanzaRefutador(t, "CA-751", "una ronda: bajo el tope", Piloto(e), []string{"f1", "f2"})

	e = cldEstado(cldEvRonda(bloq), cldPol2, 2, 2, 2, aeBase()...)
	aeDetiene(t, "CA-751", "dos rondas: al tope", Piloto(e), cldRazon(2, 2), 7)
	e = cldEstado(cldEvRonda([]finding.Item{aeF3}), cldPol2, 3, 2, 2, aeBase()...)
	aeDetiene(t, "CA-751", "pasado el tope, un bloqueante", Piloto(e), cldRazon(3, 1), 7)
	if Piloto(e).Why != RazonTopeDeRondas(3, 1) {
		t.Fatalf("CA-751: el Why es RazonTopeDeRondas")
	}
}

// CA-751: el tope para tambien sin rondas en la politica y a mitad de una
// ronda del piloto (el sobre mas nuevo lleva `ronda`).
func TestCA751_ElTopeValeFueraYDentroDeLaRonda(t *testing.T) {
	bloq := []finding.Item{aeF1, aeF2}
	e := cldEstado(cldEvRonda(bloq), aePol0, 2, 2, 2, aeBase()...)
	aeDetiene(t, "CA-751", "sin rondas en la politica", Piloto(e), cldRazon(2, 2), 7)
	enRonda := aeMas(aeBase(), aeRef1([]string{"f1", "f2"}, 2))
	e = cldEstado(cldEvRonda(bloq), cldPol2, 1, 2, 2, enRonda...)
	if d := Piloto(e); strings.HasPrefix(d.Why, "se hicieron ") {
		t.Fatalf("CA-751: fixture: bajo el tope la ronda sigue: %+v", d)
	}
	e = cldEstado(cldEvRonda(bloq), cldPol2, 2, 2, 2, enRonda...)
	aeDetiene(t, "CA-751", "a mitad de la ronda", Piloto(e), cldRazon(2, 2), 7)
}

// CA-751 y aclaraciones 3 y 10: los casos 1 a 4 y ParadaAntesDeGuardar
// deciden antes que el tope; el tope decide antes que el caso 5 (relevo), el
// 6 (reintento) y el 7 (la ronda), tambien con la tarjeta en Writer.
func TestCA751_DondeParaElTope(t *testing.T) {
	bloq := []finding.Item{aeF1, aeF2}
	pol := manifest.AutonomiaPolicy{ReintentosAnteRojo: 1, RondasDeHallazgos: 2}
	reintento := aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "verify", status: aeNo, v: aeRojoTest("v-w-rojo")}

	ev := cldEvRonda(bloq)
	ev.Item.Auto = ""
	aeDetiene(t, "CA-751", "caso 1: sin auto", Piloto(cldEstado(ev, pol, 2, 2, 2, aeBase()...)), piWhyNoActivado, 1)

	ev = piEv("tu-aceptacion", piConTope, piEnCurso)
	ev.Findings = append([]finding.Item(nil), bloq...)
	aeDetiene(t, "CA-751", "caso 2: otro trabajo", Piloto(cldEstado(ev, pol, 2, 2, 2, aeBase()...)), piWhyOtroTrabajo, 2)

	ev = piEv("hecho", piConTope, piNada)
	ev.Findings = append([]finding.Item(nil), bloq...)
	e := cldEstado(ev, pol, 2, 2, 2, aeBase()...)
	aeCol(t, "CA-751", e, ColHecho)
	aeDetiene(t, "CA-751", "caso 3: Hecho", Piloto(e), piWhyHecho, 3)

	e = cldEstado(bdArbol(cldEvRonda(bloq)), pol, 2, 2, 2, aeBase()...)
	aeDetiene(t, "CA-751", "caso 4: sin tarea", Piloto(e), piWhySinTarea, 4)

	e = cldEstado(cldEvRonda(bloq), pol, 2, 2, 2, aeMas(aeBase(), reintento)...)
	e.Sobres[len(e.Sobres)-1].TocoMaterialGit = true
	parada, para := ParadaAntesDeGuardar(e.Sobres)
	if !para {
		t.Fatalf("CA-751: fixture: el writer que toco el material para antes de guardar")
	}
	aeDetiene(t, "CA-751", "ParadaAntesDeGuardar", Piloto(e), parada.Why, parada.Caso)

	// caso 5: el writer cortado por tiempo, con sesion (se releva sin el tope)
	cortado := aeCortado("writer", "claude", "ses-w-1")
	e = cldTrasWriter(pol, nil, 2, cortado)
	if d := Piloto(e); !d.Launch || d.Regla != ReglaRelevoPorTiempo {
		t.Fatalf("CA-751: fixture: sin bloqueantes (CA-752) el writer cortado se releva: %+v", d)
	}
	e = cldTrasWriter(pol, bloq, 2, cortado)
	aeCol(t, "CA-751", e, ColWriter)
	aeDetiene(t, "CA-751", "le gana al caso 5 (relevo)", Piloto(e), cldRazon(2, 2), 7)

	// caso 6: el primer rojo del writer (se reintenta sin el tope)
	rojoW := aeS{rol: "writer", prov: "claude", col: ColWriter, stage: "verify", status: aeNo,
		v: aeVer("v-w-rojo", aeGatesWriter(map[string]string{"test": verdict.StatusFail}))}
	e = cldTrasWriter(pol, nil, 2, rojoW)
	if d := Piloto(e); !d.Launch || d.Regla != ReglaReintento {
		t.Fatalf("CA-751: fixture: sin bloqueantes (CA-752) el rojo del writer se reintenta: %+v", d)
	}
	e = cldTrasWriter(pol, bloq, 2, rojoW)
	aeCol(t, "CA-751", e, ColWriter)
	aeDetiene(t, "CA-751", "le gana al caso 6 (reintento)", Piloto(e), cldRazon(2, 2), 7)
}

// CA-751 y aclaracion 9: Estado.TopeDeRondas en 0 vale max(2,
// Politica.RondasDeHallazgos); uno declarado manda.
func TestCA751_ElTopeDelEstado(t *testing.T) {
	bloq := []finding.Item{aeF1, aeF2}
	e := cldEstado(cldEvRonda(bloq), cldPol2, 2, 0, 0, aeBase()...)
	aeDetiene(t, "CA-751", "tope 0 y politica 2: 2", Piloto(e), cldRazon(2, 2), 7)
	e = cldEstado(cldEvRonda(bloq), aePol0, 2, 0, 0, aeBase()...)
	aeDetiene(t, "CA-751", "tope 0 y politica 0: 2", Piloto(e), cldRazon(2, 2), 7)

	pol3 := manifest.AutonomiaPolicy{RondasDeHallazgos: 3}
	e = cldEstado(cldEvRonda(bloq), pol3, 2, 3, 0, aeBase()...)
	cldLanzaRefutador(t, "CA-751", "tope 0 y politica 3: dos rondas no llegan", Piloto(e), []string{"f1", "f2"})
	e = cldEstado(cldEvRonda(bloq), pol3, 3, 3, 0, aeBase()...)
	aeDetiene(t, "CA-751", "tope 0 y politica 3: tres rondas llegan", Piloto(e), cldRazon(3, 2), 7)

	e = cldEstado(cldEvRonda(bloq), cldPol2, 2, 4, 4, aeBase()...)
	cldLanzaRefutador(t, "CA-751", "tope 4: dos rondas no paran", Piloto(e), []string{"f1", "f2"})
	e = cldEstado(cldEvRonda(bloq), cldPol2, 1, 1, 1, aeBase()...)
	aeDetiene(t, "CA-751", "tope 1, una ronda", Piloto(e), cldRazon(1, 2), 7)
}

// CA-752: el piloto, sin bloqueantes, decide lo mismo con cualquier numero
// de rondas: el tope no lo para.
func TestCA752_ElPilotoSinBloqueantes(t *testing.T) {
	quiero := Piloto(cldEstado(cldEvRonda(nil), cldPol2, 0, 2, 2, aeBase()...))
	for _, n := range []int{2, 3, 5} {
		d := Piloto(cldEstado(cldEvRonda(nil), cldPol2, n, 2, 2, aeBase()...))
		if strings.HasPrefix(d.Why, "se hicieron ") || !reflect.DeepEqual(d, quiero) {
			t.Fatalf("CA-752: sin bloqueantes %d rondas no cambian la decision:\n quiero %+v\n fue    %+v", n, quiero, d)
		}
	}
}

// CA-754: el refutador de la ronda del piloto recibe solo los bloqueantes
// que ningun refutador de la tarjeta recibio, tambien uno lanzado a mano
// (sin regla ni ronda); uno cortado en run no cuenta.
func TestCA754_ElPilotoCuentaElRefutadorAMano(t *testing.T) {
	base := aeBase()
	aMano := aeS{rol: "refutador", prov: "claude", col: ColReview, hallazgos: []string{"f1"},
		pedido: aeStr(PedidoRondaRefutador(1, bdSlug, []string{"f1"}))}
	con := append(append(append([]aeS{}, base[:3]...), aMano), base[3])
	e := aeRonda(cldPol2, []finding.Item{aeF1, aeF2}, con...)
	aeLanza(t, "CA-754", "refutador a mano entregable", Piloto(e), aeQuiero{accion: ActPedirRefutador, rol: "refutador", prov: "gemini",
		regla: ReglaRondaRefutador, caso: 7, ronda: 1, hallazgos: []string{"f2"}, bloq: 2,
		pedido: PedidoRondaRefutador(1, bdSlug, []string{"f2"})})

	cortado := aMano
	cortado.stage, cortado.status = "run", aeNo
	con = append(append(append([]aeS{}, base[:3]...), cortado), base[3])
	e = aeRonda(cldPol2, []finding.Item{aeF1, aeF2}, con...)
	aeLanza(t, "CA-754", "refutador a mano cortado en run", Piloto(e), aeQuiero{accion: ActPedirRefutador, rol: "refutador", prov: "gemini",
		regla: ReglaRondaRefutador, caso: 7, ronda: 1, hallazgos: []string{"f1", "f2"}, bloq: 2, pedido: aePedidoRefutadorLiteral})
}

// ===========================================================================
// Aclaracion 9: el tope sale del hoom.yaml de la raiz (CardFor sobre disco)
// ===========================================================================

const cldBaseYAML = "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"

// cldDisco arma en disco una tarjeta en Review por dos bloqueantes, con una
// review completa y n deltas, y la deriva con CardFor. wtYAML, si no es "",
// es el findings/autonomia del hoom.yaml de la rama de la tarea. Devuelve
// tambien el worktree de la tarea.
func cldDisco(t *testing.T, rootYAML, wtYAML string, n int) (Card, string) {
	t.Helper()
	root := bdRepo(t, rootYAML)
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdCommitear(t, root, "item")
	wt := bdWorktree(t, root, bdSlug)
	if wtYAML != "" {
		bdEscribir(t, wt, "hoom.yaml", cldBaseYAML+wtYAML)
	}
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdEscribir(t, wt, "precios.go", "package app\n\nfunc Precio() int { return 1 }\n")
	bdAprobar(t, wt, bdSlug)
	bdCommitear(t, wt, "spec y codigo")
	bdVeredictoDisco(t, wt, bdSpec, time.Now().UTC().Add(-2*time.Hour), false, bdGatesVerdes())
	bdCommitear(t, wt, "veredicto")
	for i := 0; i < 2; i++ {
		if _, err := finding.Register(wt, "main", finding.Draft{Severity: "high", Lens: "risk", File: "precios.go",
			Description: fmt.Sprintf("bloquea la tarjeta %d", i), Author: "reviewer@test", Task: bdSlug}); err != nil {
			t.Fatal(err)
		}
	}
	ini := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	for i := 0; i <= n; i++ {
		cob := reviewcmd.CoberturaDelta
		if i == 0 {
			cob = reviewcmd.CoberturaCompleta
		}
		cuando := ini.Add(time.Duration(i) * time.Minute)
		bdReviewDisco(t, wt, reviewcmd.Record{ID: fmt.Sprintf("%s_%06x", cuando.Format("20060102T150405"), i+1), CreatedAt: cuando,
			Task: bdSlug, Spec: bdSpec, Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: reviewcmd.Lentes,
			Provider: "codex", Writer: "claude", Cross: reviewcmd.CrossYes, WritersDeclared: []string{}, Findings: []string{},
			Usage: []reviewcmd.LensUsage{}, Cobertura: cob, Desde: bdDesde, Hasta: bdHasta})
	}
	bdCommitear(t, wt, "hallazgos y registros de review")
	c, err := CardFor(root, "main", "high", bdSlug, time.Now().UTC())
	if err != nil {
		t.Fatalf("fixture: CardFor: %v", err)
	}
	if len(c.Evidence.BlockingFindings) != 2 {
		t.Fatalf("fixture: dos bloqueantes: %+v", c.Evidence)
	}
	return c, wt
}

// CA-750 y aclaracion 9: el tablero lee max_rondas y autonomia del hoom.yaml
// de la raiz (el mismo de block_on), no el de la rama de la tarea.
func TestCA750_ElTopeSaleDelHoomYamlDeLaRaiz(t *testing.T) {
	c, _ := cldDisco(t, "findings:\n  block_on: high\n  max_rondas: 1\n", "", 1)
	cldAlTope(t, "CA-750", "max_rondas 1, una ronda", c, ColReview, 1, 1, 2)

	c, _ = cldDisco(t, "findings:\n  block_on: high\n", "", 1)
	cldBajoElTope(t, "CA-750", "sin max_rondas, una ronda", c, 1, 2)
	if len(c.Actions) < 2 || c.Actions[0].ID != ActPedirWriter || c.Actions[1].ID != ActPedirRefutador {
		t.Fatalf("CA-753: en disco (normal), bajo el tope, la primera accion es pedir-writer y luego pedir-refutador: %v", acIDs(c))
	}

	c, _ = cldDisco(t, "findings:\n  block_on: high\nautonomia:\n  rondas_de_hallazgos: 3\n", "", 2)
	cldBajoElTope(t, "CA-750", "autonomia 3, dos rondas", c, 2, 3)
	c, _ = cldDisco(t, "findings:\n  block_on: high\nautonomia:\n  rondas_de_hallazgos: 3\n", "", 3)
	cldAlTope(t, "CA-750", "autonomia 3, tres rondas", c, ColReview, 3, 3, 2)

	c, _ = cldDisco(t, "findings:\n  block_on: high\n  max_rondas: 1\n", "findings:\n  block_on: high\n  max_rondas: 5\n", 1)
	cldAlTope(t, "CA-750", "la rama dice 5, la raiz 1", c, ColReview, 1, 1, 2)
	c, _ = cldDisco(t, "findings:\n  block_on: high\n  max_rondas: 5\n", "findings:\n  block_on: high\n  max_rondas: 1\n", 1)
	cldBajoElTope(t, "CA-750", "la rama dice 1, la raiz 5", c, 1, 5)
}

// CA-753 y CA-754: los ids de pedir-refutador en disco salen en el orden de
// `hoom finding list` (finding.List, del mas viejo al mas nuevo).
func TestCA753_EnDiscoElOrdenDeFindingList(t *testing.T) {
	c, wt := cldDisco(t, "findings:\n  block_on: high\n", "", 0)
	a := acAccion(t, "CA-753", c, ActPedirRefutador)
	items, _, err := finding.List(wt, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	var quiero []string
	for _, it := range items {
		if it.Task == bdSlug && it.Severity == "high" {
			quiero = append(quiero, it.ID)
		}
	}
	if len(quiero) != 2 || !reflect.DeepEqual(a.Hallazgos, quiero) {
		t.Fatalf("CA-753: Hallazgos en el orden de hoom finding list: %q, fue %q", quiero, a.Hallazgos)
	}
	if a.Pedido != PedidoRondaRefutador(1, bdSlug, quiero) {
		t.Fatalf("CA-754: el pedido en disco: %q", a.Pedido)
	}
}
