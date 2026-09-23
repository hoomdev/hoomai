// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md, pieza
// 3 (CA-367, CA-368, CA-369, CA-370): el doctor. Los problemas de una
// tarjeta los calcula Derive, que sigue siendo pura: Evidence literales, sin
// disco, sin reloj y sin git, disparados uno por uno. Los del proyecto los
// agrega Doctor sobre repos git reales en t.TempDir(), y leerlos no cambia
// ni un byte. El texto, el JSON y el parseo estricto del verbo son del
// paquete; la salida con 0 se mira con el binario real. Ningun CLI de IA.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Textos del contrato (sin acentos, como los emite el binario).
const (
	drCd               = "cd .hoom/worktrees/precios && "
	drVerify           = "hoom verify --spec .hoom/specs/precios.md"
	drWhatSpecEditado  = "el spec cambio despues de su aprobacion: revisalo y aprobalo de nuevo"
	drPlainSpecEditado = "el spec cambio despues de tu aprobacion"
	drPlainSinVer      = "hay cambios sin verificar"
	drPlainVencido     = "el codigo cambio despues del ultimo verde"
	drPlainBug         = "hay un error de hoom en esta tarjeta: no la integres"
	drAccionBug        = "reporta este bug de hoom con la salida de 'hoom board doctor --json' y no integres la tarjeta hasta entonces"
	drRefSpecWt        = ".hoom/worktrees/precios/.hoom/specs/precios.md"
	drRefSpecArbol     = ".hoom/specs/precios.md"
)

func drWhatSinVer(huella string) string {
	return "el espacio de trabajo tiene cambios que ningun veredicto certifica (huella " + huella + ")"
}

func drWhatVencido(id, vieja, actual string) string {
	return "el veredicto verde " + id + " certifica la huella " + vieja + " y el arbol tiene " + actual
}

func drWhatHuerfano(id, rol string, paso, pasos int, stage string) string {
	return fmt.Sprintf("el sobre %s del %s quedo abierto en el paso %d de %d (%s) y su proceso ya no vive", id, rol, paso, pasos, stage)
}

func drPlainHuerfano(rol string) string { return "el trabajo del " + rol + " quedo interrumpido" }

func drWhatSinSpec(dias int, fecha string) string {
	return fmt.Sprintf("el item lleva %d dias sin spec (creado el %s)", dias, fecha)
}

func drPlainSinSpec(dias int) string { return fmt.Sprintf("lleva %d dias esperando su spec", dias) }

func drWhatBug(id string) string {
	return "bug de hoom: la tarjeta esta en Tu aceptacion con el hallazgo high abierto " + id + ", y la derivacion nunca deberia permitirlo"
}

// Los del proyecto.
func drWhatHuerfanoProyecto(id, rol, stage string) string {
	return "el sobre " + id + " del " + rol + " quedo abierto en el paso " + stage + " y no es de ninguna tarjeta"
}

func drAccionBorrar(id string) string {
	return "es telemetria local: si ya no te sirve, borralo con rm .hoom/envelopes/" + id + ".json"
}

func drWhatSinVerProyecto(dir, huella string) string {
	return "el espacio de trabajo " + dir + " tiene cambios que ningun veredicto certifica (huella " + huella + ")"
}

func drAccionItem(task string) string { return `hoom item add "<titulo>" --slug ` + task }

// drProb es un problema esperado. La ref se mira aparte: el contrato la fija
// solo en el ejemplo de spec-editado; de las demas se exige que nombre su
// artefacto.
type drProb struct{ id, slug, what, plain, action string }

// drEv es la tarjeta sana de los fixtures: Tu aceptacion con espacio de
// trabajo propio, cambios en la candidata y un verde de la huella actual
// (huella-1) que la certifica.
func drEv() Evidence {
	ev := acEv()
	ev.TreeFingerprint = "huella-1"
	ev.TreeChanges = []string{"precios.go", "precios_test.go"}
	ev.Certified = true
	return ev
}

// drArbol: la misma tarjeta sin espacio de trabajo propio. La evidencia vive
// en el arbol del proyecto y no hay candidata propia.
func drArbol(ev Evidence) Evidence {
	ev = bdArbol(ev)
	ev.TreeChanges = nil
	return ev
}

// drSinVeredicto: la tarjeta sin ningun veredicto completo. Fingerprint (C1)
// queda vacio sin veredicto; TreeFingerprint es la huella del arbol.
func drSinVeredicto(ev Evidence, huella string) Evidence {
	ev.Verdict, ev.GreenVerdicts, ev.Fingerprint = nil, nil, ""
	ev.TreeFingerprint, ev.Certified = huella, false
	return ev
}

// drConVeredicto: el ultimo veredicto completo de la tarjeta es v, la huella
// actual del arbol es actual, y certified dice si algun veredicto completo
// del arbol tiene esa huella.
func drConVeredicto(ev Evidence, v *verdict.Verdict, actual string, certified bool) Evidence {
	ev.Verdict, ev.GreenVerdicts = v, nil
	if v.Verdict == "green" {
		ev.GreenVerdicts = []string{v.ID}
	}
	ev.Fingerprint, ev.TreeFingerprint, ev.Certified = actual, actual, certified
	return ev
}

func drVerde(id, huella string) *verdict.Verdict {
	return bdVeredicto(id, bdT0.Add(30*time.Minute), huella, 50, 10, bdGatesVerdes())
}

func drRojo(id, huella string) *verdict.Verdict {
	return bdVeredicto(id, bdT0.Add(40*time.Minute), huella, 50, 10,
		[]verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
}

// drSinSpec: la tarjeta sin spec (Backlog).
func drSinSpec(ev Evidence) Evidence {
	ev.SpecExists, ev.Approval, ev.Criteria, ev.Untraced, ev.LintIssues = false, "", nil, nil, nil
	return ev
}

// drWriterQuieto: la tarjeta en Writer, sin veredicto y sin cambios en su
// espacio de trabajo: ningun problema propio.
func drWriterQuieto() Evidence {
	ev := drSinVeredicto(drEv(), "huella-1")
	ev.TreeChanges = nil
	return ev
}

func drIDs(ps []Problem) []string {
	out := []string{}
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func drFmt(ps []Problem) string {
	var b strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&b, "  {id=%q slug=%q what=%q plain=%q action=%q ref=%q}\n", p.ID, p.Slug, p.What, p.Plain, p.Action, p.Ref)
	}
	return b.String()
}

func drFmtW(ps []drProb) string {
	var b strings.Builder
	for _, p := range ps {
		fmt.Fprintf(&b, "  {id=%q slug=%q what=%q plain=%q action=%q}\n", p.id, p.slug, p.what, p.plain, p.action)
	}
	return b.String()
}

// drMismos exige la lista exacta de problemas, en su orden.
func drMismos(t *testing.T, ca, nombre string, got []Problem, want []drProb) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: [%s] doctor es una lista, nunca nil", ca, nombre)
	}
	ok := len(got) == len(want)
	for i := 0; ok && i < len(want); i++ {
		g, w := got[i], want[i]
		ok = g.ID == w.id && g.Slug == w.slug && g.What == w.what && g.Plain == w.plain && g.Action == w.action
	}
	if !ok {
		t.Fatalf("%s: [%s] los problemas no son los del contrato\nquiere:\n%sfue:\n%s", ca, nombre, drFmtW(want), drFmt(got))
	}
}

// drDoctor deriva la tarjeta y devuelve sus problemas (nunca nil).
func drDoctor(t *testing.T, ca, nombre string, ev Evidence) (Card, []Problem) {
	t.Helper()
	c := Derive(ev)
	if c.Doctor == nil {
		t.Fatalf("%s: [%s] Derive trae doctor como lista, nunca nil", ca, nombre)
	}
	return c, c.Doctor
}

func drRefCon(t *testing.T, ca, nombre string, p Problem, sub string) {
	t.Helper()
	if p.Ref == "" || !strings.Contains(p.Ref, sub) {
		t.Fatalf("%s: [%s] la ref de %s nombra su artefacto (%q), fue %q", ca, nombre, p.ID, sub, p.Ref)
	}
}

// CA-367: una tarjeta sana tiene doctor [] (lista, no null en el JSON).
func TestCA367_TarjetaSanaDoctorVacio(t *testing.T) {
	for _, f := range []acFix{
		{"tu aceptacion con espacio propio y verde de la huella actual", drEv()},
		{"sin espacio propio y verde de la huella actual", drArbol(drEv())},
		{"writer sin veredicto y sin cambios", drWriterQuieto()},
		{"test-writer", func() Evidence {
			ev := drEv()
			ev.Criteria = []string{"CA-1", "CA-3"}
			ev.Untraced = []string{"CA-3"}
			return ev
		}()},
		{"tu aprobacion sin aprobar (no es spec-editado)", func() Evidence { ev := drEv(); ev.Approval = approval.StatusNotApproved; return ev }()},
	} {
		c, ps := drDoctor(t, "CA-367", f.nombre, f.ev)
		if len(ps) != 0 {
			t.Fatalf("CA-367: [%s] una tarjeta sana tiene doctor [], fue:\n%s", f.nombre, drFmt(ps))
		}
		raw, _ := json.Marshal(c)
		if !strings.Contains(string(raw), `"doctor":[]`) {
			t.Fatalf("CA-367: [%s] el JSON de la tarjeta trae \"doctor\":[] (lista, nunca null): %s", f.nombre, raw)
		}
	}
}

// CA-367: spec-editado cuando la aprobacion quedo invalidada; <cd> solo
// cuando el arbol de evidencia es el espacio de trabajo.
func TestCA367_SpecEditado(t *testing.T) {
	ev := drEv()
	ev.Approval = approval.StatusInvalidated
	c, ps := drDoctor(t, "CA-367", "spec editado con espacio propio", ev)
	bdCol(t, "CA-367", c, ColTuAprobacion)
	drMismos(t, "CA-367", "spec editado con espacio propio", ps, []drProb{
		{ProbSpecEditado, bdSlug, drWhatSpecEditado, drPlainSpecEditado, drCd + "hoom spec approve .hoom/specs/precios.md"}})
	if ps[0].Ref != drRefSpecWt {
		t.Fatalf("CA-367: la ref de spec-editado es el spec en el espacio de trabajo %q, fue %q", drRefSpecWt, ps[0].Ref)
	}

	ev = drArbol(drEv())
	ev.Approval = approval.StatusInvalidated
	_, ps = drDoctor(t, "CA-367", "spec editado sin espacio propio", ev)
	drMismos(t, "CA-367", "spec editado sin espacio propio", ps, []drProb{
		{ProbSpecEditado, bdSlug, drWhatSpecEditado, drPlainSpecEditado, "hoom spec approve .hoom/specs/precios.md"}})
	if ps[0].Ref != drRefSpecArbol {
		t.Fatalf("CA-367: sin espacio propio la ref es %q, fue %q", drRefSpecArbol, ps[0].Ref)
	}
}

// CA-367: sin-veredicto: espacio de trabajo con cambios, ninguno certificado
// y el ultimo completo no verde. No con un rojo de la huella actual ni con
// un verde viejo (que es verde-vencido).
func TestCA367_SinVeredicto(t *testing.T) {
	want := func(huella, accion string) []drProb {
		return []drProb{{ProbSinVeredicto, bdSlug, drWhatSinVer(huella), drPlainSinVer, accion}}
	}

	c, ps := drDoctor(t, "CA-367", "sin ningun veredicto", drSinVeredicto(drEv(), "huella-2"))
	bdCol(t, "CA-367", c, ColWriter)
	drMismos(t, "CA-367", "sin ningun veredicto", ps, want("huella-2", drCd+drVerify))
	drRefCon(t, "CA-367", "sin ningun veredicto", ps[0], bdWtDir)

	_, ps = drDoctor(t, "CA-367", "el ultimo completo es rojo de una huella vieja",
		drConVeredicto(drEv(), drRojo("v-rojo-viejo", "huella-1"), "huella-2", false))
	drMismos(t, "CA-367", "el ultimo completo es rojo de una huella vieja", ps, want("huella-2", drCd+drVerify))

	// el ultimo completo es rojo aunque antes hubo un verde: sigue sin certificar
	ev := drConVeredicto(drEv(), drRojo("v-rojo-viejo", "huella-1"), "huella-2", false)
	ev.GreenVerdicts = []string{"v-verde-anterior"}
	_, ps = drDoctor(t, "CA-367", "rojo despues de un verde", ev)
	drMismos(t, "CA-367", "rojo despues de un verde", ps, want("huella-2", drCd+drVerify))

	// sin spec, la accion es 'hoom verify' a secas (el item es nuevo: no es sin-spec)
	c, ps = drDoctor(t, "CA-367", "sin spec", drSinSpec(drSinVeredicto(drEv(), "huella-3")))
	bdCol(t, "CA-367", c, ColBacklog)
	drMismos(t, "CA-367", "sin spec", ps, want("huella-3", drCd+"hoom verify"))

	// negativos
	negativos := []acFix{
		{"un rojo de la huella actual: ya esta verificado y la tarjeta lo dice en rojo",
			drConVeredicto(drEv(), drRojo("v-rojo-actual", "huella-2"), "huella-2", true)},
		{"sin cambios en la candidata", drWriterQuieto()},
		{"otro veredicto completo del arbol certifica la huella actual", func() Evidence {
			ev := drSinVeredicto(drEv(), "huella-2")
			ev.Certified = true
			return ev
		}()},
		{"sin espacio de trabajo propio", drSinVeredicto(drArbol(drEv()), "")},
	}
	for _, f := range negativos {
		c, ps := drDoctor(t, "CA-367", f.nombre, f.ev)
		if len(ps) != 0 {
			t.Fatalf("CA-367: [%s] no es sin-veredicto ni ningun otro problema:\n%s", f.nombre, drFmt(ps))
		}
		if strings.HasPrefix(f.nombre, "un rojo") && c.Red == nil {
			t.Fatalf("CA-367: [%s] el fixture tiene que dejar la tarjeta en rojo", f.nombre)
		}
	}
	// un verde viejo con cambios despues es verde-vencido, no sin-veredicto
	_, ps = drDoctor(t, "CA-367", "verde viejo", drConVeredicto(drEv(), drVerde("v-verde-viejo", "huella-1"), "huella-2", false))
	if !reflect.DeepEqual(drIDs(ps), []string{ProbVerdeVencido}) {
		t.Fatalf("CA-367: un verde viejo con cambios despues es verde-vencido y no sin-veredicto: %v", drIDs(ps))
	}
}

// CA-367: verde-vencido: el ultimo completo es verde y la huella actual es
// otra.
func TestCA367_VerdeVencido(t *testing.T) {
	c, ps := drDoctor(t, "CA-367", "verde vencido con espacio propio",
		drConVeredicto(drEv(), drVerde("v-verde-viejo", "huella-1"), "huella-2", false))
	bdCol(t, "CA-367", c, ColWriter)
	drMismos(t, "CA-367", "verde vencido con espacio propio", ps, []drProb{
		{ProbVerdeVencido, bdSlug, drWhatVencido("v-verde-viejo", "huella-1", "huella-2"), drPlainVencido, drCd + drVerify}})
	drRefCon(t, "CA-367", "verde vencido con espacio propio", ps[0], "v-verde-viejo")

	_, ps = drDoctor(t, "CA-367", "verde vencido sin espacio propio",
		drConVeredicto(drArbol(drEv()), drVerde("v-verde-viejo", "huella-1"), "huella-2", false))
	drMismos(t, "CA-367", "verde vencido sin espacio propio", ps, []drProb{
		{ProbVerdeVencido, bdSlug, drWhatVencido("v-verde-viejo", "huella-1", "huella-2"), drPlainVencido, drVerify}})

	// un verde de la huella actual no esta vencido
	_, ps = drDoctor(t, "CA-367", "verde vigente", drConVeredicto(drEv(), drVerde("v-verde", "huella-2"), "huella-2", true))
	if len(ps) != 0 {
		t.Fatalf("CA-367: un verde de la huella actual no es ningun problema:\n%s", drFmt(ps))
	}
}

// drInterrumpido: una tarjeta sin otros problemas con el sobre env-caido de
// rol interrumpido en el paso 3 de 5 (run), con la sesion sesion-abc123.
func drInterrumpido(ev Evidence, rol, provider string) Evidence {
	return acInterrumpido(ev, rol, provider)
}

// CA-367: sobre-huerfano, con la accion de reanudar (habilitada), la de la
// review y la de volver a lanzar; --spec salvo en los roles que escriben
// specs.
func TestCA367_SobreHuerfano(t *testing.T) {
	arquitectoEnBacklog := func() Evidence {
		ev := drSinSpec(drEv())
		ev.TreeChanges = nil
		return ev
	}
	sinSesion := func(ev Evidence) Evidence {
		ev.Runs[len(ev.Runs)-1].Meta.ProviderSessionID = ""
		return ev
	}
	revisor := func() Evidence {
		ev := acGrande(drEv())
		ev.Runs = []RunState{acGasto("run-w", "writer", "claude", nil, 5*time.Minute)}
		ev = drInterrumpido(ev, "reviewer", "codex")
		ev.Envelopes[0].Record.Step, ev.Envelopes[0].Record.Steps = 2, 4
		return ev
	}
	casos := []struct {
		nombre      string
		ev          Evidence
		rol         string
		paso, pasos int
		reanudar    bool
		accion      string
	}{
		{"writer de claude que se puede reanudar", drInterrumpido(drWriterQuieto(), "writer", "claude"), "writer", 3, 5, true,
			`hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude --resume sesion-abc123 "<pedido>"`},
		{"writer de codex que se puede reanudar", drInterrumpido(drWriterQuieto(), "writer", "codex"), "writer", 3, 5, true,
			`hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider codex --resume sesion-abc123 "<pedido>"`},
		{"arquitecto que se puede reanudar (sin --spec)", drInterrumpido(arquitectoEnBacklog(), "arquitecto", "claude"), "arquitecto", 3, 5, true,
			`hoom agent --role arquitecto --task precios --provider claude --resume sesion-abc123 "<pedido>"`},
		{"reviewer cortado: la review se vuelve a lanzar", revisor(), "reviewer", 2, 4, false,
			"hoom review --task precios --spec .hoom/specs/precios.md"},
		{"writer sin sesion: volver a lanzar", sinSesion(drInterrumpido(drWriterQuieto(), "writer", "claude")), "writer", 3, 5, false,
			`hoom agent --role writer --task precios --spec .hoom/specs/precios.md "<pedido>"`},
		{"arquitecto sin sesion: volver a lanzar sin --spec", sinSesion(drInterrumpido(arquitectoEnBacklog(), "arquitecto", "claude")), "arquitecto", 3, 5, false,
			`hoom agent --role arquitecto --task precios "<pedido>"`},
	}
	for _, cs := range casos {
		c, ps := drDoctor(t, "CA-367", cs.nombre, cs.ev)
		if c.Interrupted == nil || c.Interrupted.EnvelopeID != "env-caido" {
			t.Fatalf("CA-367: [%s] el fixture deja interrumpido a env-caido: %+v", cs.nombre, c.Interrupted)
		}
		if a := acAccion(t, "CA-367", c, ActReanudar); a.Enabled != cs.reanudar {
			t.Fatalf("CA-367: [%s] el fixture tiene reanudar enabled=%v, fue %v (%q)", cs.nombre, cs.reanudar, a.Enabled, a.Why)
		}
		drMismos(t, "CA-367", cs.nombre, ps, []drProb{{ProbSobreHuerfano, bdSlug,
			drWhatHuerfano("env-caido", cs.rol, cs.paso, cs.pasos, "run"), drPlainHuerfano(cs.rol), cs.accion}})
		drRefCon(t, "CA-367", cs.nombre, ps[0], "env-caido")
	}

	// un sobre relevado por otro posterior de la tarjeta no es huerfano (C3)
	ev := drInterrumpido(drWriterQuieto(), "writer", "claude")
	ev.Envelopes = append(ev.Envelopes, EnvelopeState{Record: envelope.Record{ID: "env-relevo", Role: "writer", Provider: "claude",
		Task: bdSlug, Dir: bdWtDir, Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
		StartedAt: bdT0.Add(20 * time.Minute), UpdatedAt: bdT0.Add(25 * time.Minute), EndedAt: bdT0.Add(25 * time.Minute)}})
	if _, ps := drDoctor(t, "CA-367", "relevado", ev); len(ps) != 0 {
		t.Fatalf("CA-367: un sobre relevado no es huerfano:\n%s", drFmt(ps))
	}
}

// CA-367: sin-spec a los 15 dias si, a los 14 no; la accion es el next de la
// tarjeta.
func TestCA367_SinSpec(t *testing.T) {
	base := func(ev Evidence) Evidence {
		ev = drSinSpec(drSinVeredicto(ev, ""))
		ev.TreeChanges = nil
		return ev
	}
	ev := base(drArbol(drEv()))
	ev.Now = ev.Item.CreadoEn.Add(15 * 24 * time.Hour)
	c, ps := drDoctor(t, "CA-367", "15 dias sin spec y sin espacio propio", ev)
	bdCol(t, "CA-367", c, ColBacklog)
	if c.Next != "hoom task start precios" {
		t.Fatalf("CA-367: el fixture tiene next 'hoom task start precios', fue %q", c.Next)
	}
	drMismos(t, "CA-367", "15 dias sin spec y sin espacio propio", ps, []drProb{
		{ProbSinSpec, bdSlug, drWhatSinSpec(15, "2026-09-22"), drPlainSinSpec(15), c.Next}})
	drRefCon(t, "CA-367", "15 dias sin spec", ps[0], bdSlug)

	ev.Now = ev.Item.CreadoEn.Add(14 * 24 * time.Hour)
	if _, ps = drDoctor(t, "CA-367", "14 dias", ev); len(ps) != 0 {
		t.Fatalf("CA-367: a los 14 dias todavia no es sin-spec (mas de 14):\n%s", drFmt(ps))
	}

	// con espacio propio: la accion es el next de la tarjeta (el arquitecto de la tarea)
	ev = base(drEv())
	ev.TreeFingerprint = "huella-wt"
	ev.Now = ev.Item.CreadoEn.Add(20 * 24 * time.Hour)
	c, ps = drDoctor(t, "CA-367", "20 dias sin spec con espacio propio", ev)
	if !strings.HasPrefix(c.Next, "hoom agent --role arquitecto --task precios") {
		t.Fatalf("CA-367: el fixture tiene next del arquitecto de la tarea, fue %q", c.Next)
	}
	drMismos(t, "CA-367", "20 dias sin spec con espacio propio", ps, []drProb{
		{ProbSinSpec, bdSlug, drWhatSinSpec(20, "2026-09-22"), drPlainSinSpec(20), c.Next}})

	// con spec no hay sin-spec aunque el item sea viejo
	ev = drEv()
	ev.Now = ev.Item.CreadoEn.Add(60 * 24 * time.Hour)
	if _, ps = drDoctor(t, "CA-367", "viejo con spec", ev); len(ps) != 0 {
		t.Fatalf("CA-367: con spec no hay sin-spec:\n%s", drFmt(ps))
	}
}

// CA-367: los problemas de una tarjeta van en el orden fijo del contrato.
func TestCA367_OrdenDeLosProblemas(t *testing.T) {
	ev := drConVeredicto(drEv(), drVerde("v-verde-viejo", "huella-1"), "huella-2", false)
	ev.Approval = approval.StatusInvalidated
	ev = drInterrumpido(ev, "writer", "claude")
	_, ps := drDoctor(t, "CA-367", "spec editado, verde vencido e interrumpido", ev)
	if want := []string{ProbSpecEditado, ProbVerdeVencido, ProbSobreHuerfano}; !reflect.DeepEqual(drIDs(ps), want) {
		t.Fatalf("CA-367: el orden es %v, fue %v", want, drIDs(ps))
	}

	ev = drSinSpec(drSinVeredicto(drEv(), "huella-2"))
	ev.Now = ev.Item.CreadoEn.Add(15 * 24 * time.Hour)
	ev = drInterrumpido(ev, "arquitecto", "claude")
	_, ps = drDoctor(t, "CA-367", "sin veredicto, interrumpido y sin spec", ev)
	if want := []string{ProbSinVeredicto, ProbSobreHuerfano, ProbSinSpec}; !reflect.DeepEqual(drIDs(ps), want) {
		t.Fatalf("CA-367: el orden es %v, fue %v", want, drIDs(ps))
	}
}

// CA-367: Hecho es terminal: una tarjeta en Hecho solo puede tener
// sobre-huerfano.
func TestCA367_HechoSoloSobreHuerfano(t *testing.T) {
	cuando := bdT0.Add(48 * time.Hour)
	ev := drConVeredicto(drEv(), drVerde("v-verde-viejo", "huella-1"), "huella-2", false)
	ev.Approval = approval.StatusInvalidated
	ev.Item.HechoEn = &cuando
	ev.Now = ev.Item.CreadoEn.Add(30 * 24 * time.Hour)
	c, ps := drDoctor(t, "CA-367", "hecho con spec editado y verde vencido", ev)
	bdCol(t, "CA-367", c, ColHecho)
	if len(ps) != 0 {
		t.Fatalf("CA-367: Hecho no tiene spec-editado ni verde-vencido:\n%s", drFmt(ps))
	}
	_, ps = drDoctor(t, "CA-367", "hecho con un sobre interrumpido", drInterrumpido(ev, "writer", "claude"))
	if !reflect.DeepEqual(drIDs(ps), []string{ProbSobreHuerfano}) {
		t.Fatalf("CA-367: en Hecho solo sobre-huerfano, fue %v", drIDs(ps))
	}

	ev = drSinSpec(drSinVeredicto(drEv(), "huella-2"))
	ev.Item.HechoEn = &cuando
	ev.Now = ev.Item.CreadoEn.Add(30 * 24 * time.Hour)
	if _, ps = drDoctor(t, "CA-367", "hecho sin spec, viejo y con cambios", ev); len(ps) != 0 {
		t.Fatalf("CA-367: Hecho no tiene sin-veredicto ni sin-spec:\n%s", drFmt(ps))
	}
}

// drFixtures son los fixtures del doctor, con nombre.
func drFixtures() []acFix {
	var out []acFix
	add := func(nombre string, ev Evidence) { out = append(out, acFix{nombre, ev}) }
	add("sana", drEv())
	add("sana sin espacio propio", drArbol(drEv()))
	ev := drEv()
	ev.Approval = approval.StatusInvalidated
	add("spec editado", ev)
	ev = drArbol(drEv())
	ev.Approval = approval.StatusInvalidated
	add("spec editado sin espacio propio", ev)
	add("sin veredicto", drSinVeredicto(drEv(), "huella-2"))
	add("rojo viejo", drConVeredicto(drEv(), drRojo("v-rojo-viejo", "huella-1"), "huella-2", false))
	add("rojo actual", drConVeredicto(drEv(), drRojo("v-rojo-actual", "huella-2"), "huella-2", true))
	add("verde vencido", drConVeredicto(drEv(), drVerde("v-verde-viejo", "huella-1"), "huella-2", false))
	add("verde vencido sin espacio propio", drConVeredicto(drArbol(drEv()), drVerde("v-verde-viejo", "huella-1"), "huella-2", false))
	add("writer interrumpido", drInterrumpido(drWriterQuieto(), "writer", "claude"))
	ev = drSinSpec(drSinVeredicto(drArbol(drEv()), ""))
	ev.Now = ev.Item.CreadoEn.Add(15 * 24 * time.Hour)
	add("sin spec viejo", ev)
	ev = drSinSpec(drSinVeredicto(drEv(), "huella-2"))
	ev.Now = ev.Item.CreadoEn.Add(15 * 24 * time.Hour)
	add("todo junto sin spec", drInterrumpido(ev, "arquitecto", "claude"))
	ev = drConVeredicto(drEv(), drVerde("v-verde-viejo", "huella-1"), "huella-2", false)
	ev.Approval = approval.StatusInvalidated
	add("todo junto con spec", drInterrumpido(ev, "writer", "claude"))
	cuando := bdT0.Add(48 * time.Hour)
	ev.Item.HechoEn = &cuando
	add("hecho con todo", drInterrumpido(ev, "writer", "claude"))
	return out
}

// drTodas son todas las Evidence de los fixtures de C1 a C4.
func drTodas() []acFix {
	var out []acFix
	for _, cs := range tbCasos() {
		out = append(out, acFix{"C2 " + cs.nombre, cs.ev})
	}
	for _, cs := range append(acColumnas(), acColumnasArbol()...) {
		out = append(out, acFix{"C3 " + cs.nombre, cs.ev})
	}
	out = append(out, acTodos()...)
	out = append(out, drFixtures()...)
	out = append(out, piEvidencias()...)
	return out
}

// CA-367: en todos los fixtures, los problemas de una tarjeta son de los
// seis del contrato (bug-derivacion nunca sale de Derive), en su orden, uno
// por id, con el slug de la tarjeta, what, action y ref no vacios y plain con
// las reglas de CA-364. <cd> aparece solo con el espacio de trabajo. En Hecho
// solo sobre-huerfano. Y los fixtures del doctor disparan los cinco.
func TestCA367_FormaDeLosProblemasEnTodosLosFixtures(t *testing.T) {
	orden := []string{ProbSpecEditado, ProbSinVeredicto, ProbVerdeVencido, ProbBugDerivacion, ProbSobreHuerfano, ProbSinSpec}
	pos := map[string]int{}
	for i, id := range orden {
		pos[id] = i
	}
	for _, f := range drTodas() {
		c, ps := drDoctor(t, "CA-367", f.nombre, f.ev)
		last := -1
		for _, p := range ps {
			i, ok := pos[p.ID]
			if !ok {
				t.Fatalf("CA-367: [%s] %q no es un problema de tarjeta del contrato", f.nombre, p.ID)
			}
			if i <= last {
				t.Fatalf("CA-367: [%s] los problemas van en el orden %v, uno por id: %v", f.nombre, orden, drIDs(ps))
			}
			last = i
			if p.Slug != c.Slug || p.What == "" || p.Action == "" || p.Ref == "" {
				t.Fatalf("CA-367: [%s] el problema lleva el slug de la tarjeta, what, action y ref: %+v", f.nombre, p)
			}
			if p.ID != ProbBugDerivacion {
				tbNormal(t, "CA-367", "["+f.nombre+"] el plain de "+p.ID, p.Plain)
			}
			if f.ev.Source != SourceWorktree && strings.HasPrefix(p.Action, "cd ") {
				t.Fatalf("CA-367: [%s] sin espacio de trabajo propio no hay <cd>: %q", f.nombre, p.Action)
			}
			if f.ev.Source == SourceWorktree && (p.ID == ProbSpecEditado || p.ID == ProbSinVeredicto || p.ID == ProbVerdeVencido) &&
				!strings.HasPrefix(p.Action, drCd) {
				t.Fatalf("CA-367: [%s] con espacio de trabajo la accion de %s empieza con %q: %q", f.nombre, p.ID, drCd, p.Action)
			}
		}
		if c.Column == ColHecho && len(ps) > 0 && !reflect.DeepEqual(drIDs(ps), []string{ProbSobreHuerfano}) {
			t.Fatalf("CA-367: [%s] en Hecho solo puede haber sobre-huerfano: %v", f.nombre, drIDs(ps))
		}
	}

	vistos := map[string]bool{}
	for _, f := range drFixtures() {
		for _, p := range Derive(f.ev).Doctor {
			vistos[p.ID] = true
		}
	}
	for _, id := range []string{ProbSpecEditado, ProbSinVeredicto, ProbVerdeVencido, ProbSobreHuerfano, ProbSinSpec} {
		if !vistos[id] {
			t.Fatalf("CA-367: los fixtures del doctor disparan %s y Derive no lo trae", id)
		}
	}
}

// CA-367: el texto de 'hoom board' y de 'hoom item show' no cambia con los
// problemas de la tarjeta.
func TestCA367_TextoDeBoardYShowNoCambia(t *testing.T) {
	ev := drConVeredicto(drEv(), drVerde("v-verde-viejo", "huella-1"), "huella-2", false)
	ev.Approval = approval.StatusInvalidated
	c := Derive(drInterrumpido(ev, "writer", "claude"))
	sin, con := c, c
	sin.Doctor = []Problem{}
	con.Doctor = []Problem{
		{ID: ProbSpecEditado, Slug: bdSlug, What: drWhatSpecEditado, Plain: drPlainSpecEditado, Action: drCd + "hoom spec approve " + bdSpec, Ref: drRefSpecWt},
		{ID: ProbVerdeVencido, Slug: bdSlug, What: drWhatVencido("v-verde-viejo", "huella-1", "huella-2"), Plain: drPlainVencido, Action: drCd + drVerify, Ref: "x"},
	}
	var a, b bytes.Buffer
	Render(&a, bdTablero(sin))
	Render(&b, bdTablero(con))
	if a.String() != b.String() {
		t.Fatalf("CA-367: el texto de 'hoom board' no cambia con doctor:\nsin:\n%s\ncon:\n%s", a.String(), b.String())
	}
	a.Reset()
	b.Reset()
	RenderCard(&a, sin)
	RenderCard(&b, con)
	if a.String() != b.String() {
		t.Fatalf("CA-367: el texto de 'hoom item show' no cambia con doctor:\nsin:\n%s\ncon:\n%s", a.String(), b.String())
	}
}

// drQuitarDir deja las rutas de TreeChanges relativas al arbol de evidencia
// (el contrato no fija si llevan el prefijo del espacio de trabajo).
func drQuitarDir(dir string, in []string) []string {
	out := []string{}
	for _, p := range in {
		out = append(out, strings.TrimPrefix(p, dir+"/"))
	}
	sort.Strings(out)
	return out
}

// CA-367: Gather llena TreeFingerprint (con espacio de trabajo o veredicto),
// TreeChanges (solo con espacio de trabajo) y Certified (algun veredicto
// COMPLETO del arbol tiene esa huella). Fingerprint de C1 y el fingerprint
// del detalle siguen vacios sin veredicto (CA-313).
func TestCA367_EvidenceDelArbolEnDisco(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC) // el item es de ayer: no es sin-spec
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, "precios.go", "package app\n")
	snap := gitx.Snapshot(wt, "main")
	if snap.ChangeFingerprint == "" || len(snap.ChangedFiles) == 0 {
		t.Fatalf("CA-367: el fixture tiene cambios en la candidata: %+v", snap)
	}

	ev := Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.Fingerprint != "" {
		t.Fatalf("CA-367: Fingerprint (C1) sigue vacio sin veredicto, fue %q", ev.Fingerprint)
	}
	if ev.TreeFingerprint != snap.ChangeFingerprint {
		t.Fatalf("CA-367: TreeFingerprint es la huella actual del espacio de trabajo %q, fue %q", snap.ChangeFingerprint, ev.TreeFingerprint)
	}
	if got := drQuitarDir(ev.Dir, ev.TreeChanges); !reflect.DeepEqual(got, snap.ChangedFiles) {
		t.Fatalf("CA-367: TreeChanges son los archivos de la candidata %v, fueron %v", snap.ChangedFiles, ev.TreeChanges)
	}
	if ev.Certified {
		t.Fatal("CA-367: sin ningun veredicto nada esta certificado")
	}
	c := Derive(ev)
	drMismos(t, "CA-367", "espacio de trabajo con cambios sin veredicto", c.Doctor, []drProb{
		{ProbSinVeredicto, bdSlug, drWhatSinVer(snap.ChangeFingerprint), drPlainSinVer, drCd + "hoom verify"}})
	d, err := DetailFor(root, "main", "high", bdSlug, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Fingerprint != "" {
		t.Fatalf("CA-367: el fingerprint del detalle sigue vacio sin veredicto (CA-313), fue %q", d.Fingerprint)
	}

	// un veredicto PARCIAL de la huella actual no certifica
	bdVeredictoDisco(t, wt, bdSpec, bdT0, true, []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass}})
	ev = Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.Certified || ev.Fingerprint != "" || ev.TreeFingerprint != snap.ChangeFingerprint {
		t.Fatalf("CA-367: un parcial no certifica ni llena Fingerprint: certified=%v fingerprint=%q tree=%q", ev.Certified, ev.Fingerprint, ev.TreeFingerprint)
	}

	// un veredicto COMPLETO del arbol (rojo, de otra spec) con la huella actual certifica
	bdVeredictoDisco(t, wt, "", bdT0.Add(time.Minute), false, []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}})
	ev = Gather(root, "main", "high", bdItem(bdSlug), now)
	if !ev.Certified || ev.TreeFingerprint != snap.ChangeFingerprint {
		t.Fatalf("CA-367: un veredicto completo del arbol con la huella actual certifica: certified=%v tree=%q", ev.Certified, ev.TreeFingerprint)
	}
	if ev.Fingerprint != "" {
		t.Fatalf("CA-367: la tarjeta sigue sin veredicto propio: Fingerprint vacio, fue %q", ev.Fingerprint)
	}
	if ps := Derive(ev).Doctor; len(ps) != 0 {
		t.Fatalf("CA-367: certificada, no es sin-veredicto:\n%s", drFmt(ps))
	}

	// el arbol cambia: la huella nueva ya no esta certificada
	bdEscribir(t, wt, "otro.go", "package app\n")
	ev = Gather(root, "main", "high", bdItem(bdSlug), now)
	if ev.TreeFingerprint == snap.ChangeFingerprint || ev.TreeFingerprint != gitx.Snapshot(wt, "main").ChangeFingerprint || ev.Certified {
		t.Fatalf("CA-367: con otro cambio la huella es otra y no esta certificada: tree=%q certified=%v", ev.TreeFingerprint, ev.Certified)
	}

	// sin espacio de trabajo y sin veredicto: ni huella del arbol ni candidata
	bdItemArchivo(t, root, "arbol", "Arbol", "")
	ev = Gather(root, "main", "high", bdItem("arbol"), now)
	if ev.TreeFingerprint != "" || len(ev.TreeChanges) != 0 || ev.Certified {
		t.Fatalf("CA-367: sin espacio de trabajo ni veredicto no hay huella del arbol: %q %v %v", ev.TreeFingerprint, ev.TreeChanges, ev.Certified)
	}
	// sin espacio de trabajo con veredicto: la huella del arbol, sin candidata propia
	bdVeredictoDisco(t, root, ".hoom/specs/arbol.md", bdT0, false, bdGatesVerdes())
	ev = Gather(root, "main", "high", bdItem("arbol"), now)
	if ev.TreeFingerprint == "" || ev.TreeFingerprint != ev.Fingerprint || len(ev.TreeChanges) != 0 || !ev.Certified {
		t.Fatalf("CA-367: con veredicto y sin espacio de trabajo: tree=%q fingerprint=%q changes=%v certified=%v",
			ev.TreeFingerprint, ev.Fingerprint, ev.TreeChanges, ev.Certified)
	}
}

// CA-368: bug-derivacion, cuando la funcion del doctor recibe una tarjeta en
// Tu aceptacion con un hallazgo high abierto de su tarea.
func TestCA368_BugDerivacion(t *testing.T) {
	ev := drEv()
	c := Derive(ev)
	bdCol(t, "CA-368", c, ColTuAceptacion)
	ev.Findings = []finding.Item{bdHallazgo("f-alto", "high", bdSlug)}
	c.Evidence.BlockingFindings = []string{"f-alto"} // la tarjeta como la dejaria una derivacion rota
	ps := DoctorOf(ev, c)
	drMismos(t, "CA-368", "tu aceptacion con un high abierto", ps, []drProb{
		{ProbBugDerivacion, bdSlug, drWhatBug("f-alto"), drPlainBug, drAccionBug}})
	drRefCon(t, "CA-368", "tu aceptacion con un high abierto", ps[0], "f-alto")

	// su lugar en el orden: antes de sobre-huerfano
	ev = drInterrumpido(drEv(), "writer", "claude")
	c = Derive(ev)
	bdCol(t, "CA-368", c, ColTuAceptacion)
	ev.Findings = []finding.Item{bdHallazgo("f-alto", "high", bdSlug)}
	c.Evidence.BlockingFindings = []string{"f-alto"}
	if got := drIDs(DoctorOf(ev, c)); !reflect.DeepEqual(got, []string{ProbBugDerivacion, ProbSobreHuerfano}) {
		t.Fatalf("CA-368: bug-derivacion va antes de sobre-huerfano: %v", got)
	}

	// negativos: un medium, uno resuelto, uno de otra tarea, y otras columnas
	cerrado := bdHallazgo("f-cerrado", "high", bdSlug)
	cerrado.Status = finding.StatusCorrected
	for _, cs := range []struct {
		nombre string
		f      finding.Item
	}{
		{"un hallazgo medium", bdHallazgo("f-medio", "medium", bdSlug)},
		{"un high ya corregido", cerrado},
		{"un high de otra tarea", bdHallazgo("f-ajeno", "high", "otra")},
	} {
		ev := drEv()
		c := Derive(ev)
		ev.Findings = []finding.Item{cs.f}
		if got := DoctorOf(ev, c); got == nil || len(got) != 0 {
			t.Fatalf("CA-368: [%s] no es bug-derivacion: %v", cs.nombre, drIDs(got))
		}
	}
	// en Review con el high abierto la derivacion esta bien: no es un bug
	ev = acConHallazgos(drEv(), "f-alto")
	c = Derive(ev)
	bdCol(t, "CA-368", c, ColReview)
	if got := DoctorOf(ev, c); len(got) != 0 {
		t.Fatalf("CA-368: en Review con el high no hay bug-derivacion: %v", drIDs(got))
	}
	// en Hecho tampoco
	cuando := bdT0.Add(48 * time.Hour)
	ev = drEv()
	ev.Item.HechoEn = &cuando
	c = Derive(ev)
	ev.Findings = []finding.Item{bdHallazgo("f-alto", "high", bdSlug)}
	if got := DoctorOf(ev, c); len(got) != 0 {
		t.Fatalf("CA-368: en Hecho no hay bug-derivacion: %v", drIDs(got))
	}
}

// CA-368: ninguna tarjeta derivada por Derive en los fixtures de C1 a C4
// trae bug-derivacion.
func TestCA368_DeriveNuncaTraeBugDerivacion(t *testing.T) {
	for _, f := range drTodas() {
		for _, p := range Derive(f.ev).Doctor {
			if p.ID == ProbBugDerivacion {
				t.Fatalf("CA-368: [%s] Derive nunca produce bug-derivacion: %+v", f.nombre, p)
			}
		}
	}
}

// drSobre escribe un registro de sobre en root, tal cual.
func drSobre(t *testing.T, root, id, task, rol, stage, status string, pid int, started, updated time.Time) string {
	t.Helper()
	rec := envelope.Record{ID: id, Role: rol, Provider: "claude", Task: task, Dir: root, Stage: stage,
		Step: 3, Steps: 5, Status: status, PID: pid, StartedAt: started, UpdatedAt: updated}
	if status != envelope.StatusRunning {
		rec.EndedAt = updated
	}
	return bdSobreDisco(t, root, rec)
}

func drHallazgoDisco(t *testing.T, root, task, desc string) {
	t.Helper()
	if _, err := finding.Register(root, "main", finding.Draft{Severity: "medium", Lens: "risk", Description: desc,
		Author: "reviewer", Task: task}); err != nil {
		t.Fatal(err)
	}
}

// CA-369: Doctor junta los problemas de las tarjetas en el orden del tablero
// y despues los del proyecto: sobres huerfanos sin tarjeta (no los relevados
// ni los vivos ni los cerrados), espacios de trabajo de 'hoom task' sin item
// con cambios sin veredicto, y tareas con evidencia y sin item (uno invalido
// cuenta como item). Con los textos del contrato, y sin cambiar un byte.
func TestCA369_DoctorDelProyecto(t *testing.T) {
	root := bdRepo(t, "findings:\n  block_on: high\n")
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	m := func(min int) time.Time { return now.Add(time.Duration(-min) * time.Minute) }
	muerto := bdPIDMuerto(t)
	yo := os.Getpid()
	rojo := []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusFail}}

	// tarjeta precios (Backlog): espacio propio con cambios sin veredicto y un
	// sobre suyo interrumpido que nadie relevo
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wtPrecios := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wtPrecios, "precios.go", "package app\n")
	fpPrecios := gitx.Snapshot(wtPrecios, "main").ChangeFingerprint
	drSobre(t, root, "20260923T080000_card01", bdSlug, "writer", "run", envelope.StatusRunning, muerto, m(60), m(55))

	// tarjeta aprob (Tu aprobacion, en el arbol del proyecto): spec editado
	// despues de aprobarlo
	bdItemArchivo(t, root, "aprob", "Aprob", "")
	bdEscribir(t, root, ".hoom/specs/aprob.md", bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdAprobar(t, root, "aprob")
	bdEscribir(t, root, ".hoom/specs/aprob.md", bdSpecTexto("- CA-1: algo distinto. [verifica: true]", true))

	// sobres sin tarjeta
	drSobre(t, root, "20260923T090000_sinta1", "", "writer", "run", envelope.StatusRunning, muerto, m(30), m(10))
	drSobre(t, root, "20260923T090000_fant01", "fantasma", "test-writer", "verify", envelope.StatusRunning, muerto, m(40), m(35))
	// relevado por un sobre posterior de su tarea: no es huerfano
	drSobre(t, root, "20260923T090000_relev1", "fantasma2", "writer", "run", envelope.StatusRunning, muerto, m(60), m(50))
	drSobre(t, root, "20260923T090000_relev2", "fantasma2", "writer", "ok", envelope.StatusDeliverable, muerto, m(45), m(44))
	// vivo y cerrado: no son huerfanos (empezaron antes del ultimo registro de sinta1)
	drSobre(t, root, "20260923T070000_vivo01", "", "writer", "run", envelope.StatusRunning, yo, m(100), m(1))
	drSobre(t, root, "20260923T070000_cerr01", "", "writer", "ok", envelope.StatusDeliverable, muerto, m(120), m(110))

	// espacios de trabajo de 'hoom task' sin item
	wtSuelto := bdWorktree(t, root, "suelto")
	bdEscribir(t, wtSuelto, "suelto.go", "package app\n")
	bdEscribir(t, wtSuelto, ".hoom/specs/suelto.md", bdSpecTexto("- CA-1: algo.", true))
	fpSuelto := gitx.Snapshot(wtSuelto, "main").ChangeFingerprint
	wtSuelto2 := bdWorktree(t, root, "suelto2")
	bdEscribir(t, wtSuelto2, "suelto2.go", "package app\n")
	fpSuelto2 := gitx.Snapshot(wtSuelto2, "main").ChangeFingerprint
	bdWorktree(t, root, "limpio") // sin cambios: nada
	wtCertif := bdWorktree(t, root, "certif")
	bdEscribir(t, wtCertif, "certif.go", "package app\n")
	bdVeredictoDisco(t, wtCertif, ".hoom/specs/certif.md", bdT0, false, rojo) // certifica su huella (rojo)

	// evidencia de tareas sin item
	bdVeredictoDisco(t, root, ".hoom/specs/viejo.md", bdT0, false, bdGatesVerdes())
	bdVeredictoDisco(t, root, ".hoom/specs/viejo.md", bdT0.Add(time.Minute), false, rojo)
	bdVeredictoDisco(t, root, ".hoom/specs/mixto.md", bdT0.Add(2*time.Minute), false, bdGatesVerdes())
	drHallazgoDisco(t, root, "mixto", "hallazgo de mixto")
	for i := 1; i <= 3; i++ {
		drHallazgoDisco(t, root, "solohall", fmt.Sprintf("hallazgo %d de solohall", i))
	}
	// un item invalido cuenta como item
	bdEscribir(t, root, ".hoom/items/rota.yaml", bdItemYAML("Rota", "columna: writer\n"))
	bdVeredictoDisco(t, root, ".hoom/specs/rota.md", bdT0.Add(3*time.Minute), false, bdGatesVerdes())
	// evidencia de una tarea con item, y evidencia sin tarea: nada
	drHallazgoDisco(t, root, bdSlug, "hallazgo de precios en la raiz")
	drHallazgoDisco(t, root, "", "hallazgo sin tarea")
	bdVeredictoDisco(t, root, "", bdT0.Add(4*time.Minute), false, bdGatesVerdes())

	antes := bdFoto(t, root)
	r, err := Doctor(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-369: Doctor sobre un proyecto valido: %v", err)
	}
	bdMismaFoto(t, "CA-369", antes, bdFoto(t, root))

	want := []drProb{
		// las tarjetas, en el orden del tablero: precios (Backlog) y aprob (Tu aprobacion)
		{ProbSinVeredicto, bdSlug, drWhatSinVer(fpPrecios), drPlainSinVer, drCd + "hoom verify"},
		{ProbSobreHuerfano, bdSlug, drWhatHuerfano("20260923T080000_card01", "writer", 3, 5, "run"), drPlainHuerfano("writer"),
			`hoom agent --role writer --task precios --spec .hoom/specs/precios.md "<pedido>"`},
		{ProbSpecEditado, "aprob", drWhatSpecEditado, drPlainSpecEditado, "hoom spec approve .hoom/specs/aprob.md"},
		// los del proyecto: sobres por ref
		{ProbSobreHuerfano, "fantasma", drWhatHuerfanoProyecto("20260923T090000_fant01", "test-writer", "verify"), "",
			drAccionBorrar("20260923T090000_fant01")},
		{ProbSobreHuerfano, "", drWhatHuerfanoProyecto("20260923T090000_sinta1", "writer", "run"), "",
			drAccionBorrar("20260923T090000_sinta1")},
		// espacios de trabajo por ref
		{ProbSinVeredicto, "suelto", drWhatSinVerProyecto(".hoom/worktrees/suelto", fpSuelto), "",
			"cd .hoom/worktrees/suelto && hoom verify --spec .hoom/specs/suelto.md"},
		{ProbSinVeredicto, "suelto2", drWhatSinVerProyecto(".hoom/worktrees/suelto2", fpSuelto2), "",
			"cd .hoom/worktrees/suelto2 && hoom verify"},
		// tareas por nombre
		{ProbEvidenciaSinItem, "certif", "hay 1 veredicto de la tarea certif y ningun item la representa", "", drAccionItem("certif")},
		{ProbEvidenciaSinItem, "mixto", "hay 1 veredicto y 1 hallazgo de la tarea mixto y ningun item la representa", "", drAccionItem("mixto")},
		{ProbEvidenciaSinItem, "solohall", "hay 3 hallazgos de la tarea solohall y ningun item la representa", "", drAccionItem("solohall")},
		{ProbEvidenciaSinItem, "viejo", "hay 2 veredictos de la tarea viejo y ningun item la representa", "", drAccionItem("viejo")},
	}
	drMismos(t, "CA-369", "proyecto completo", r.Problems, want)
	if r.Problems[2].Ref != ".hoom/specs/aprob.md" {
		t.Fatalf("CA-369: la ref de spec-editado sin espacio propio es el spec, fue %q", r.Problems[2].Ref)
	}
	drRefCon(t, "CA-369", "sobre sin tarjeta", r.Problems[3], "20260923T090000_fant01")
	drRefCon(t, "CA-369", "sobre sin tarea", r.Problems[4], "20260923T090000_sinta1")
	drRefCon(t, "CA-369", "espacio sin item", r.Problems[5], ".hoom/worktrees/suelto")
	drRefCon(t, "CA-369", "espacio sin item ni spec", r.Problems[6], ".hoom/worktrees/suelto2")

	// Doctor dice lo mismo que las tarjetas del tablero
	b, err := Build(root, "main", "high", now)
	if err != nil {
		t.Fatal(err)
	}
	if got := drIDs(bdCard(t, b, bdSlug).Doctor); !reflect.DeepEqual(got, []string{ProbSinVeredicto, ProbSobreHuerfano}) {
		t.Fatalf("CA-369: la tarjeta del tablero trae sus problemas: %v", got)
	}
}

// CA-369: en un proyecto sin items, solo los problemas del proyecto, o
// ninguno.
func TestCA369_DoctorSinItems(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	r, err := Doctor(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-369: Doctor sin items: %v", err)
	}
	if len(r.Problems) != 0 {
		t.Fatalf("CA-369: sin items ni evidencia no hay problemas:\n%s", drFmt(r.Problems))
	}
	drSobre(t, root, "20260923T090000_solo01", "", "scout", "run", envelope.StatusRunning, bdPIDMuerto(t),
		now.Add(-time.Hour), now.Add(-50*time.Minute))
	antes := bdFoto(t, root)
	if r, err = Doctor(root, "main", "high", now); err != nil {
		t.Fatal(err)
	}
	bdMismaFoto(t, "CA-369", antes, bdFoto(t, root))
	drMismos(t, "CA-369", "solo un sobre huerfano", r.Problems, []drProb{{ProbSobreHuerfano, "",
		drWhatHuerfanoProyecto("20260923T090000_solo01", "scout", "run"), "", drAccionBorrar("20260923T090000_solo01")}})
}

// drEjemplo es el reporte del ejemplo del contrato.
func drEjemplo() DoctorReport {
	return DoctorReport{Problems: []Problem{
		{ID: ProbVerdeVencido, Slug: "items-y-columna-derivada",
			What:   "el veredicto verde 2026-09-23T06-10-06Z_ab41dc52 certifica la huella 1a2b3c4d5e6f7a8b y el arbol tiene 9f8e7d6c5b4a3a2b",
			Plain:  drPlainVencido,
			Action: "hoom verify --spec .hoom/specs/items-y-columna-derivada.md",
			Ref:    ".hoom/verdicts/2026-09-23T06-10-06Z_ab41dc52.json"},
		{ID: ProbEvidenciaSinItem, Slug: "verify-args-estrictos",
			What:   "hay 2 veredictos de la tarea verify-args-estrictos y ningun item la representa",
			Action: `hoom item add "<titulo>" --slug verify-args-estrictos`},
	}}
}

// CA-370: el texto de 'hoom board doctor': la cabecera con el conteo, y por
// problema su id, el slug (o '-'), what y 'Accion: <action>'.
func TestCA370_RenderDoctor(t *testing.T) {
	var buf bytes.Buffer
	RenderDoctor(&buf, drEjemplo())
	want := "hoom board doctor: 2 problemas\n" +
		"  verde-vencido  items-y-columna-derivada\n" +
		"    el veredicto verde 2026-09-23T06-10-06Z_ab41dc52 certifica la huella 1a2b3c4d5e6f7a8b y el arbol tiene 9f8e7d6c5b4a3a2b\n" +
		"    Accion: hoom verify --spec .hoom/specs/items-y-columna-derivada.md\n" +
		"  evidencia-sin-item  verify-args-estrictos\n" +
		"    hay 2 veredictos de la tarea verify-args-estrictos y ningun item la representa\n" +
		"    Accion: hoom item add \"<titulo>\" --slug verify-args-estrictos\n"
	if buf.String() != want {
		t.Fatalf("CA-370: el texto del contrato\nquiere:\n%s\nfue:\n%s", want, buf.String())
	}

	buf.Reset()
	RenderDoctor(&buf, DoctorReport{Problems: []Problem{{ID: ProbSobreHuerfano, Slug: "",
		What:   drWhatHuerfanoProyecto("20260923T090000_sinta1", "writer", "run"),
		Action: drAccionBorrar("20260923T090000_sinta1"), Ref: ".hoom/envelopes/20260923T090000_sinta1.json"}}})
	want = "hoom board doctor: 1 problema\n" +
		"  sobre-huerfano  -\n" +
		"    " + drWhatHuerfanoProyecto("20260923T090000_sinta1", "writer", "run") + "\n" +
		"    Accion: " + drAccionBorrar("20260923T090000_sinta1") + "\n"
	if buf.String() != want {
		t.Fatalf("CA-370: con un problema, '1 problema', y '-' sin slug\nquiere:\n%s\nfue:\n%s", want, buf.String())
	}

	for _, r := range []DoctorReport{{}, {Problems: []Problem{}}} {
		buf.Reset()
		RenderDoctor(&buf, r)
		if buf.String() != "hoom board doctor: sin problemas de coherencia\n" {
			t.Fatalf("CA-370: sin problemas el texto es una linea, fue %q", buf.String())
		}
	}
}

// CA-370: --json emite {"problems": [...]}, lista nunca null, con las claves
// de cada problema.
func TestCA370_DoctorJSONBytes(t *testing.T) {
	for _, r := range []DoctorReport{{}, {Problems: []Problem{}}} {
		raw, err := DoctorJSONBytes(r)
		if err != nil {
			t.Fatalf("CA-370: DoctorJSONBytes sin problemas: %v", err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("CA-370: es JSON: %v\n%s", err, raw)
		}
		l, ok := m["problems"].([]any)
		if !ok || len(l) != 0 || len(m) != 1 || strings.Contains(string(raw), "null") {
			t.Fatalf("CA-370: sin problemas es {\"problems\": []}, nunca null: %s", raw)
		}
	}
	r := drEjemplo()
	raw, err := DoctorJSONBytes(r)
	if err != nil {
		t.Fatal(err)
	}
	var js struct {
		Problems []map[string]any `json:"problems"`
	}
	if err := json.Unmarshal(raw, &js); err != nil || len(js.Problems) != 2 {
		t.Fatalf("CA-370: {\"problems\": [...]} con los dos: %v\n%s", err, raw)
	}
	for i, p := range js.Problems {
		claves := []string{}
		for k := range p {
			claves = append(claves, k)
		}
		sort.Strings(claves)
		if !reflect.DeepEqual(claves, []string{"action", "id", "plain", "ref", "slug", "what"}) {
			t.Fatalf("CA-370: cada problema trae id, slug, what, plain, action y ref (plain \"\" incluido): %v", claves)
		}
		w := r.Problems[i]
		if p["id"] != w.ID || p["slug"] != w.Slug || p["what"] != w.What || p["plain"] != w.Plain || p["action"] != w.Action || p["ref"] != w.Ref {
			t.Fatalf("CA-370: el problema %d viaja tal cual: %v", i, p)
		}
	}
}

// CA-370: ParseDoctorArgs es estricto; ParseArgs de 'hoom board' no cambia
// (CA-288): 'hoom board doctor' no es un argumento de board.
func TestCA370_ParseDoctorArgs(t *testing.T) {
	opt, err := ParseDoctorArgs(nil)
	if err != nil || opt.JSON {
		t.Fatalf("CA-370: 'hoom board doctor' sin argumentos es valido: %+v %v", opt, err)
	}
	opt, err = ParseDoctorArgs([]string{"--json"})
	if err != nil || !opt.JSON {
		t.Fatalf("CA-370: --json: %+v %v", opt, err)
	}
	for _, args := range [][]string{{"x"}, {"--bogus"}, {"--json", "x"}, {"--", "x"}, {"doctor"}, {"--fix"}} {
		_, err := ParseDoctorArgs(args)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || ue.ExitCode() != 2 {
			t.Fatalf("CA-370: 'hoom board doctor %s' es *cliargs.UsageError (exit 2), fue %T %v", strings.Join(args, " "), err, err)
		}
		for _, l := range strings.Split(DoctorUsageText, "\n") {
			if l = strings.TrimRight(l, " "); strings.TrimSpace(l) != "" && !strings.Contains(ue.Error(), l) {
				t.Fatalf("CA-370: el rechazo lleva el bloque de uso del doctor; falta %q en:\n%s", l, ue.Error())
			}
		}
	}
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		if _, err := ParseDoctorArgs(args); !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-370: %v es ErrHelp, fue %v", args, err)
		}
	}
	// ParseArgs no cambia: 'doctor' sigue siendo un posicional que rechaza
	_, err = ParseArgs([]string{"doctor"})
	var ue *cliargs.UsageError
	if !errors.As(err, &ue) || !strings.Contains(ue.Error(), "Uso: hoom board [--json]") {
		t.Fatalf("CA-288: ParseArgs no cambia: 'doctor' es un error de uso de board, fue %T %v", err, err)
	}
}

// drDoctorJSON parsea la salida de 'hoom board doctor --json'.
func drDoctorJSON(t *testing.T, ca, out string) []Problem {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("%s: 'hoom board doctor --json' es JSON: %v\n%s", ca, err, out)
	}
	if string(m["problems"]) == "" || string(m["problems"]) == "null" {
		t.Fatalf("%s: la salida es {\"problems\": [...]}, nunca null:\n%s", ca, out)
	}
	var ps []Problem
	if err := json.Unmarshal(m["problems"], &ps); err != nil {
		t.Fatalf("%s: problems es una lista de problemas: %v\n%s", ca, err, out)
	}
	return ps
}

// CA-370: con el binario, 'hoom board doctor' y '--json' salen con 0 con y
// sin problemas, imprimen el texto del contrato y no tocan nada; un pedido
// que no entiende es exit 2; 'hoom board x' sigue siendo un error de uso. Y
// los problemas de la tarjeta viajan en 'hoom board --json' y 'hoom item
// show --json', sin cambiar el texto de 'hoom board'.
func TestCA370_E2EBoardDoctor(t *testing.T) {
	bin := bxBinario(t)
	root := bdRepo(t, "findings:\n  block_on: high\n")

	r := bxCorrer(t, bin, root, "board", "doctor")
	bxOK(t, "CA-370", r, "board", "doctor")
	if r.stdout != "hoom board doctor: sin problemas de coherencia\n" {
		t.Fatalf("CA-370: sin problemas, una linea; fue %q", r.stdout)
	}
	r = bxCorrer(t, bin, root, "board", "doctor", "--json")
	bxOK(t, "CA-370", r, "board", "doctor", "--json")
	if ps := drDoctorJSON(t, "CA-370", r.stdout); len(ps) != 0 {
		t.Fatalf("CA-370: sin problemas, problems []: %+v", ps)
	}

	bdVeredictoDisco(t, root, ".hoom/specs/verify-args-estrictos.md", bdT0, false, bdGatesVerdes())
	bdVeredictoDisco(t, root, ".hoom/specs/verify-args-estrictos.md", bdT0.Add(time.Minute), false, bdGatesVerdes())
	antes := bdFoto(t, root)
	r = bxCorrer(t, bin, root, "board", "doctor")
	bxOK(t, "CA-370", r, "board", "doctor")
	want := "hoom board doctor: 1 problema\n" +
		"  evidencia-sin-item  verify-args-estrictos\n" +
		"    hay 2 veredictos de la tarea verify-args-estrictos y ningun item la representa\n" +
		"    Accion: hoom item add \"<titulo>\" --slug verify-args-estrictos\n"
	if r.stdout != want {
		t.Fatalf("CA-370: el texto del contrato, con exit 0 aunque haya problemas\nquiere:\n%s\nfue:\n%s", want, r.stdout)
	}
	r = bxCorrer(t, bin, root, "board", "doctor", "--json")
	bxOK(t, "CA-370", r, "board", "doctor", "--json")
	ps := drDoctorJSON(t, "CA-370", r.stdout)
	if len(ps) != 1 || ps[0].ID != ProbEvidenciaSinItem || ps[0].Slug != "verify-args-estrictos" ||
		ps[0].Action != drAccionItem("verify-args-estrictos") || ps[0].Plain != "" {
		t.Fatalf("CA-370: --json trae el problema del proyecto: %+v", ps)
	}
	bdMismaFoto(t, "CA-370", antes, bdFoto(t, root))

	// uso
	r = bxCorrer(t, bin, root, "board", "doctor", "x")
	if r.exit != 2 || !strings.Contains(r.stdout+r.stderr, "Uso: hoom board doctor [--json]") {
		t.Fatalf("CA-370: 'hoom board doctor x' es un error de uso (exit 2) con su bloque: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	r = bxCorrer(t, bin, root, "board", "doctor", "-h")
	if r.exit != 0 || !strings.Contains(r.stdout, "Uso: hoom board doctor [--json]") {
		t.Fatalf("CA-370: 'hoom board doctor -h' es el uso con exit 0: %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	if r = bxCorrer(t, bin, root, "board", "x"); r.exit != 2 {
		t.Fatalf("CA-288: 'hoom board x' sigue siendo un error de uso (exit 2), fue %d", r.exit)
	}

	// la tarjeta lleva sus problemas en el JSON, no en el texto
	root2 := bdRepo(t, "findings:\n  block_on: high\n")
	bdEscribir(t, root2, ".hoom/items/precios.yaml", "titulo: Precios\ntipo: feature\nprioridad: media\n"+
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: "+time.Now().UTC().Format(time.RFC3339)+"\n")
	wt := bdWorktree(t, root2, bdSlug)
	bdEscribir(t, wt, "precios.go", "package app\n")
	c := bxCard(t, bin, root2, bdSlug)
	if got := drIDs(c.Doctor); !reflect.DeepEqual(got, []string{ProbSinVeredicto}) {
		t.Fatalf("CA-367: 'hoom board --json' trae doctor en la tarjeta: %v", got)
	}
	r = bxCorrer(t, bin, root2, "item", "show", bdSlug, "--json")
	bxOK(t, "CA-367", r, "item", "show", bdSlug, "--json")
	var show struct {
		Card Card `json:"card"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &show); err != nil || !reflect.DeepEqual(drIDs(show.Card.Doctor), []string{ProbSinVeredicto}) {
		t.Fatalf("CA-367: 'hoom item show --json' trae doctor en la tarjeta: %v %v", err, drIDs(show.Card.Doctor))
	}
	for _, args := range [][]string{{"board"}, {"item", "show", bdSlug}} {
		r = bxCorrer(t, bin, root2, args...)
		bxOK(t, "CA-367", r, args...)
		if strings.Contains(r.stdout, "ningun veredicto certifica") || strings.Contains(r.stdout, ProbSinVeredicto) ||
			strings.Contains(r.stdout, drPlainSinVer) {
			t.Fatalf("CA-367: el texto de 'hoom %s' no cambia: no lista los problemas del doctor:\n%s", strings.Join(args, " "), r.stdout)
		}
	}
	r = bxCorrer(t, bin, root2, "board", "doctor")
	bxOK(t, "CA-370", r, "board", "doctor")
	if !strings.HasPrefix(r.stdout, "hoom board doctor: 1 problema\n  sin-veredicto  precios\n") {
		t.Fatalf("CA-370: el doctor lista el problema de la tarjeta:\n%s", r.stdout)
	}
}
