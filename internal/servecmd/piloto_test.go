// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-381 y CA-383): el piloto automatico dentro del Studio. Es una cinta
// transportadora, no un orquestador: cuando la funcion de un trabajo que
// lanzo el Studio vuelve, el Studio lee el registro del sobre, vuelve a
// derivar la tarjeta, llama a boardcmd.Pilot y, si la tarjeta tiene
// auto: hasta-humano, escribe su decision en el log del sobre que cerro y
// lanza (o no) el rol siguiente con la MISMA funcion que POST .../launch.
//
// Los CLIs de IA son falsos, en el PATH minimo de lanzar_test.go (lnPATH):
// anotan sus argumentos y hacen lo justo para que el sobre cierre como el
// caso necesita. Ningun claude, codex, gemini ni opencode real entra.
//
// Nota: el criterio nombra el encadenamiento test-writer -> writer, que con
// la derivacion real no se puede armar en el Studio: un test-writer
// entregable deja un veredicto verde de la huella actual, y con eso la
// tarjeta nunca queda en Writer. Los encadenamientos reales que cubren lo
// mismo son writer (codex) -> reviewer (claude, el primer provider con tope)
// y arquitecto -> test-writer (el spec que escribe ya tenia aprobacion).
package servecmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
)

const (
	spAuto        = "auto: hasta-humano\n"
	spPresupuesto = "presupuesto_usd: 5\n"
	spPrefijo     = "piloto automatico: "
)

// spEscribeCodigo es el writer de mentira: deja 450 lineas de codigo (la
// review de 4 lentes queda exigida) y sale 0.
const spEscribeCodigo = `i=0
{
  printf 'package app\n\n'
  while [ $i -lt 450 ]; do printf 'var cinta%d = %d\n' $i $i; i=$((i+1)); done
} > cinta.go
exit 0
`

// spCLI instala un CLI de IA falso que anota sus argumentos como los de
// lnFakes (f.llamadas los lee) y despues corre cuerpo; si espera, no sigue
// hasta que el test lo suelta (con un tope de 30 s).
func spCLI(t *testing.T, f *lnFakes, name, cuerpo string, espera bool) {
	t.Helper()
	tmp := filepath.Join(f.argv, ".tmp-"+name)
	final := filepath.Join(f.argv, name)
	s := "#!/bin/sh\n" +
		"printf '%s\\000' \"$@\" > '" + tmp + ".'$$\n" +
		"mv '" + tmp + ".'$$ '" + final + ".'$$\n"
	if espera {
		s += "i=0\nwhile [ ! -f '" + f.gate + "' ] && [ $i -lt 600 ]; do sleep 0.05; i=$((i+1)); done\n"
	}
	s += cuerpo
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	if err := os.WriteFile(filepath.Join(f.bin, name), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
}

// spEscribeSpec es el arquitecto de mentira: cuando el pedido trae la marca
// SPEC-DE-LA-CINTA, deja en su arbol el spec byte a byte igual a fuente.
func spEscribeSpec(fuente, slug string) string {
	return "case \"$*\" in\n*SPEC-DE-LA-CINTA*) mkdir -p .hoom/specs && cp '" + fuente + "' '.hoom/specs/" + slug + ".md' ;;\nesac\nexit 0\n"
}

// spEsperar sondea cond con pausas cortas hasta el plazo: nunca un sleep
// largo fijo.
func spEsperar(plazo time.Duration, cond func() bool) bool {
	deadline := time.Now().Add(plazo)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return cond()
}

// spTarjeta deriva la tarjeta con el codigo de C1; un item con
// auto: hasta-humano es valido (CA-378).
func spTarjeta(t *testing.T, ca, root, slug string) boardcmd.Card {
	t.Helper()
	c, err := boardcmd.CardFor(root, lnBase, tbBlockOn, slug, time.Now().UTC())
	if err != nil {
		t.Fatalf("%s: la tarjeta %s se deriva (un item con auto: hasta-humano es valido, CA-378): %v", ca, slug, err)
	}
	return c
}

func spColumna(t *testing.T, ca, root, slug, col string) boardcmd.Card {
	t.Helper()
	c := spTarjeta(t, ca, root, slug)
	if c.Column != col {
		t.Fatalf("%s: fixture: la tarjeta %s debia estar en %s y esta en %s (%s)", ca, slug, col, c.Column, c.Plain)
	}
	return c
}

// spCartaWriter: item en la raiz (con extra), espacio de trabajo con el
// spec aprobado y su criterio citado por un test, sin veredicto (C1: Writer).
func spCartaWriter(t *testing.T, ca, root, slug, extra string) string {
	t.Helper()
	tbItem(t, root, slug, extra)
	wt := lnTarea(t, ca, root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-921: citado por un test.", true))
	tbEscribir(t, wt, lnTestDe(slug), "package app\n\n// CA-921\n")
	tbAprobar(t, wt, slug)
	lnCommit(t, wt, "spec aprobado y sus tests")
	spColumna(t, ca, root, slug, boardcmd.ColWriter)
	return wt
}

// spCartaReviewLimpia: verde de la huella actual, mas de 400 lineas y sin
// registro de review (C1: Review, con las 4 lentes exigidas), todo guardado.
// El espacio de trabajo ignora .hoom/reviews/: el registro que deja la review
// no ensucia el arbol, y la tarjeta puede quedar lista para aceptar despues
// del trabajo (sin eso ningun trabajo lanzado la deja en Tu aceptacion).
func spCartaReviewLimpia(t *testing.T, ca, root, slug, extra string) string {
	t.Helper()
	tbItem(t, root, slug, extra)
	wt := lnTarea(t, ca, root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-923: citado por un test.", true))
	tbEscribir(t, wt, lnTestDe(slug), "package app\n\n// CA-923\n")
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 0; i < 450; i++ {
		b.WriteString("var limpia" + strconv.Itoa(i) + " = " + strconv.Itoa(i) + "\n")
	}
	tbEscribir(t, wt, "limpia.go", b.String())
	tbEscribir(t, wt, ".hoom/.gitignore", hoomfs.GitignoreBody()+"reviews/\n")
	tbAprobar(t, wt, slug)
	lnCommit(t, wt, "cambio grande")
	lnVeredicto(t, wt, slug, 0)
	lnCommit(t, wt, "veredicto")
	if c := spColumna(t, ca, root, slug, boardcmd.ColReview); !c.Evidence.ReviewRequired {
		t.Fatalf("%s: fixture: la review de 4 lentes tiene que ser exigida: %+v", ca, c.Evidence)
	}
	return wt
}

// spCartaBacklogAprobada: una tarjeta en Backlog (sin spec ni espacio de
// trabajo) cuyo spec, el que va a escribir el arquitecto, ya tiene su
// aprobacion guardada en la base. Devuelve el archivo con ese spec.
func spCartaBacklogAprobada(t *testing.T, ca, root, slug, extra string) string {
	t.Helper()
	tbItem(t, root, slug, extra)
	texto := tbSpecTexto("- CA-925: todavia nadie lo cita.", true)
	spec := filepath.Join(root, ".hoom", "specs", slug+".md")
	tbEscribir(t, root, ".hoom/specs/"+slug+".md", texto)
	tbAprobar(t, root, slug)
	if err := os.Remove(spec); err != nil {
		t.Fatal(err)
	}
	gitRun(t, root, "add", ".hoom/approvals")
	gitRun(t, root, "commit", "-q", "-m", "la aprobacion del spec que va a escribir el arquitecto")
	fuente := filepath.Join(t.TempDir(), slug+".md")
	if err := os.WriteFile(fuente, []byte(texto), 0o644); err != nil {
		t.Fatal(err)
	}
	spColumna(t, ca, root, slug, boardcmd.ColBacklog)
	return fuente
}

// spLog es la salida del sobre, .hoom/envelopes/<id>.log del proyecto.
func spLog(root, id string) string {
	raw, _ := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, id+".log"))
	return string(raw)
}

// spLineas son las lineas del piloto en el log del sobre.
func spLineas(root, id string) []string {
	var out []string
	for _, l := range strings.Split(spLog(root, id), "\n") {
		if strings.Contains(l, "piloto automatico") {
			out = append(out, strings.TrimSpace(l))
		}
	}
	return out
}

// spEsperarLinea espera la primera linea del piloto en el log del sobre id.
func spEsperarLinea(t *testing.T, ca, root, id, que string) string {
	t.Helper()
	if !spEsperar(20*time.Second, func() bool { return len(spLineas(root, id)) > 0 }) {
		t.Fatalf("%s: %s: el log del sobre %s no gano ninguna linea %q\nlog:\n%s", ca, que, id, spPrefijo, spLog(root, id))
	}
	return spLineas(root, id)[0]
}

// spUnaLinea exige exactamente una linea del piloto en el log del sobre.
func spUnaLinea(t *testing.T, ca, root, id string) string {
	t.Helper()
	l := spLineas(root, id)
	if len(l) != 1 {
		t.Fatalf("%s: el log del sobre %s tiene una sola linea del piloto: %q", ca, id, l)
	}
	return l[0]
}

// spCerrado espera a que el sobre id cierre y devuelve su registro.
func spCerrado(t *testing.T, ca, root, id string) envelope.Record {
	t.Helper()
	var rec envelope.Record
	leer := func() bool {
		raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, id+".json"))
		if err != nil || json.Unmarshal(raw, &rec) != nil {
			return false
		}
		return rec.Done()
	}
	if !spEsperar(30*time.Second, leer) {
		t.Fatalf("%s: el sobre %s no cerro en 30 s: %+v\nlog:\n%s", ca, id, rec, spLog(root, id))
	}
	return rec
}

// spCrudo son las claves del registro del sobre tal como esta en disco.
func spCrudo(t *testing.T, root, id string) map[string]json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, id+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// spOtros son los registros de sobre del proyecto que no son los nombrados.
func spOtros(root string, ids ...string) []envelope.Record {
	fuera := map[string]bool{}
	for _, id := range ids {
		fuera[id] = true
	}
	var out []envelope.Record
	for _, rec := range envelope.List(root) {
		if !fuera[rec.ID] {
			out = append(out, rec)
		}
	}
	return out
}

// spEsperarOtro espera el sobre que la cinta lanza despues de los nombrados.
func spEsperarOtro(t *testing.T, ca, root, que string, ids ...string) envelope.Record {
	t.Helper()
	var otro envelope.Record
	if !spEsperar(20*time.Second, func() bool {
		if l := spOtros(root, ids...); len(l) > 0 {
			otro = l[0]
			return true
		}
		return false
	}) {
		t.Fatalf("%s: %s: la cinta lanza el sobre siguiente sin otro POST, y no hubo ninguno\nlog de %s:\n%s", ca, que, ids[len(ids)-1], spLog(root, ids[len(ids)-1]))
	}
	return otro
}

// spPide parsea "piloto automatico: pide al <rol> con <provider> (<n> USD):
// sobre <id>".
var spPide = regexp.MustCompile(`^piloto automatico: pide al (\S+) con (\S+) \(([0-9]+(?:\.[0-9]+)?) USD\): sobre (\S+)$`)

func spRevisarPide(t *testing.T, ca, linea, rol, provider string, presupuesto float64, id string) {
	t.Helper()
	m := spPide.FindStringSubmatch(linea)
	if m == nil {
		t.Fatalf("%s: la linea del piloto es %q, fue %q", ca,
			"piloto automatico: pide al "+rol+" con "+provider+" (<presupuesto> USD): sobre <id>", linea)
	}
	n, _ := strconv.ParseFloat(m[3], 64)
	if m[1] != rol || m[2] != provider || n != presupuesto || m[4] != id {
		t.Fatalf("%s: la cinta pide al %s con %s (%v USD): sobre %s; la linea dice %q", ca, rol, provider, presupuesto, id, linea)
	}
}

// spPiloto exige que el sobre lo haya lanzado la cinta: "piloto": true.
func spPiloto(t *testing.T, ca, root string, rec envelope.Record) {
	t.Helper()
	if !rec.Pilot || strings.TrimSpace(string(spCrudo(t, root, rec.ID)["piloto"])) != "true" {
		t.Fatalf("%s: el registro del sobre que lanzo la cinta trae \"piloto\": true: %+v", ca, rec)
	}
}

// spPersona exige que el sobre que pidio una persona no traiga la clave.
func spPersona(t *testing.T, ca, root, id string) {
	t.Helper()
	if _, ok := spCrudo(t, root, id)["piloto"]; ok {
		t.Fatalf("%s: el registro del sobre que pidio una persona no trae la clave piloto: %s", ca, spCrudo(t, root, id)["piloto"])
	}
}

// ---------------------------------------------------------------------------

// CA-381: una persona pide al writer (codex, sin tope) sobre una tarjeta con
// auto: hasta-humano y 5 USD de presupuesto. El writer cierra entregable y
// la tarjeta pasa a Review con la review exigida: sin otro POST, la cinta
// lanza al reviewer con el primer provider ok con tope (claude: codex
// escribio y no tiene tope) y lo que queda dividido 4 (1.25 USD), su registro
// trae "piloto": true, y el log del writer gana
// "piloto automatico: pide al reviewer con claude (1.25 USD): sobre <id>".
// Cuando la review termina, la cinta vuelve a mirar y se detiene (lo escribe
// en el log de la review); nada mas se lanza.
func TestCA381_LaCintaPideAlReviewerSinOtroPOST(t *testing.T) {
	f := lnPATH(t)
	spCLI(t, f, "codex", spEscribeCodigo, false)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	const slug = "en-cinta"
	spCartaWriter(t, "CA-381", root, slug, spPresupuesto+spAuto)
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-381", lnLaunch(t, s, slug, s.Token(),
		lnCuerpo("pedir-writer", "codex", "", nil, "Implementa el spec hasta que verify de verde.")))
	primero := spCerrado(t, "CA-381", root, r.EnvelopeID)
	if primero.Status != envelope.StatusDeliverable {
		t.Fatalf("CA-381: fixture: el writer de codex cierra entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	spPersona(t, "CA-381", root, r.EnvelopeID)

	segundo := spEsperarOtro(t, "CA-381", root, "writer entregable y la tarjeta en Review con la review exigida", r.EnvelopeID)
	segundo = spCerrado(t, "CA-381", root, segundo.ID)
	if segundo.Role != "reviewer" || segundo.Provider != "claude" || segundo.Task != slug {
		t.Fatalf("CA-381: la cinta pide al reviewer de la tabla con claude (codex escribio y no tiene tope): %+v", segundo)
	}
	spPiloto(t, "CA-381", root, segundo)

	spEsperarLinea(t, "CA-381", root, r.EnvelopeID, "la cinta lanzo al reviewer")
	spRevisarPide(t, "CA-381", spUnaLinea(t, "CA-381", root, r.EnvelopeID), "reviewer", "claude", 1.25, segundo.ID)

	// la review corre con el tope que propone la accion (lo que queda / 4)
	ll := f.llamadas(t, "claude")
	if len(ll) == 0 {
		t.Fatal("CA-381: la review que lanzo la cinta corrio claude")
	}
	for _, args := range ll {
		if v, ok := lnSigue(args, "--max-budget-usd"); !ok || v != "1.25" {
			t.Fatalf("CA-381: la review que lanzo la cinta corre con --max-budget-usd 1.25: %q", args)
		}
	}
	if n := len(f.llamadas(t, "codex")); n != 1 {
		t.Fatalf("CA-381: la cinta no reintenta al writer: codex corrio %d veces", n)
	}

	// la review tambien pasa por la cinta cuando termina: se detiene
	l := spEsperarLinea(t, "CA-381", root, segundo.ID, "la review que lanzo la cinta termino")
	lnEsperarLanzamientos(t, "CA-381", s)
	if !strings.HasPrefix(l, spPrefijo+"se detiene: ") || spUnaLinea(t, "CA-381", root, segundo.ID) != l {
		t.Fatalf("CA-381: despues de la review la cinta se detiene y lo escribe una vez: %q", spLineas(root, segundo.ID))
	}
	if otros := spOtros(root, r.EnvelopeID, segundo.ID); len(otros) != 0 {
		t.Fatalf("CA-381: la cinta no lanza nada despues de la review: %+v", otros)
	}
}

// CA-381: la cinta sobre otro tramo de la tabla. Una persona pide al
// arquitecto desde Backlog; el spec que entrega ya tenia aprobacion, asi que
// la tarjeta salta a Test-writer, y la cinta pide al test-writer con el
// provider del arquitecto (claude, ok y con tope) y lo que queda (5 USD). El
// test-writer ciego no llega a correr (su spec no esta en el commit) y
// cierra no-entregable: la cinta se detiene ante ese rojo y lo escribe en su
// log, con el Why del contrato.
func TestCA381_LaCintaPideAlTestWriterYSeDetieneAnteSuRojo(t *testing.T) {
	f := lnPATH(t)
	root := tbProyecto(t)
	const slug = "desde-backlog"
	fuente := spCartaBacklogAprobada(t, "CA-381", root, slug, spPresupuesto+spAuto)
	spCLI(t, f, "claude", spEscribeSpec(fuente, slug), false)
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-381", lnLaunch(t, s, slug, s.Token(),
		lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec de la tarjeta (SPEC-DE-LA-CINTA).")))
	primero := spCerrado(t, "CA-381", root, r.EnvelopeID)
	if primero.Status != envelope.StatusDeliverable {
		t.Fatalf("CA-381: fixture: el arquitecto que entrega el spec cierra entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	spPersona(t, "CA-381", root, r.EnvelopeID)

	segundo := spEsperarOtro(t, "CA-381", root, "arquitecto entregable y la tarjeta en Test-writer", r.EnvelopeID)
	segundo = spCerrado(t, "CA-381", root, segundo.ID)
	if segundo.Role != "test-writer" || segundo.Provider != "claude" || segundo.Task != slug {
		t.Fatalf("CA-381: la cinta pide al test-writer de la tabla con el provider del arquitecto: %+v", segundo)
	}
	spPiloto(t, "CA-381", root, segundo)
	spEsperarLinea(t, "CA-381", root, r.EnvelopeID, "la cinta lanzo al test-writer")
	spRevisarPide(t, "CA-381", spUnaLinea(t, "CA-381", root, r.EnvelopeID), "test-writer", "claude", 5, segundo.ID)

	if segundo.Status != envelope.StatusNotDeliverable {
		t.Fatalf("CA-381: fixture: el test-writer ciego sin su spec en el commit cierra no-entregable: %+v\n%s", segundo, spLog(root, segundo.ID))
	}
	const quiere = spPrefijo + "se detiene: el trabajo del test-writer no cerro entregable (no-entregable): el piloto se detiene ante un rojo"
	l := spEsperarLinea(t, "CA-381", root, segundo.ID, "el test-writer que lanzo la cinta cerro no-entregable")
	lnEsperarLanzamientos(t, "CA-381", s)
	if l != quiere || spUnaLinea(t, "CA-381", root, segundo.ID) != quiere {
		t.Fatalf("CA-381: ante un rojo la cinta escribe\n  %q\nfue\n  %q", quiere, spLineas(root, segundo.ID))
	}
	if otros := spOtros(root, r.EnvelopeID, segundo.ID); len(otros) != 0 {
		t.Fatalf("CA-381: la cinta no relanza tras un rojo: %+v", otros)
	}
	if n := len(f.llamadas(t, "claude")); n != 1 {
		t.Fatalf("CA-381: solo corrio el arquitecto (el test-writer no llego a correr): claude corrio %d veces", n)
	}
}

// CA-381: un trabajo que cierra no-entregable, con auto, no lanza nada, y
// el log de su sobre gana exactamente
// "piloto automatico: se detiene: el trabajo del writer no cerro entregable
// (no-entregable): el piloto se detiene ante un rojo".
func TestCA381_UnCierreNoEntregableDetieneLaCinta(t *testing.T) {
	f := lnPATH(t)
	spCLI(t, f, "codex", "exit 1\n", false)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	const slug = "con-rojo"
	spCartaWriter(t, "CA-381", root, slug, spPresupuesto+spAuto)
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-381", lnLaunch(t, s, slug, s.Token(),
		lnCuerpo("pedir-writer", "codex", "", nil, "Implementa el spec hasta que verify de verde.")))
	primero := spCerrado(t, "CA-381", root, r.EnvelopeID)
	if primero.Status != envelope.StatusNotDeliverable {
		t.Fatalf("CA-381: fixture: el writer cuyo run sale 1 cierra no-entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	const quiere = spPrefijo + "se detiene: el trabajo del writer no cerro entregable (no-entregable): el piloto se detiene ante un rojo"
	l := spEsperarLinea(t, "CA-381", root, r.EnvelopeID, "el writer cerro no-entregable")
	lnEsperarLanzamientos(t, "CA-381", s)
	if l != quiere || spUnaLinea(t, "CA-381", root, r.EnvelopeID) != quiere {
		t.Fatalf("CA-381: ante un rojo la cinta escribe\n  %q\nfue\n  %q", quiere, spLineas(root, r.EnvelopeID))
	}
	if otros := spOtros(root, r.EnvelopeID); len(otros) != 0 {
		t.Fatalf("CA-381: un cierre no-entregable no lanza nada: %+v", otros)
	}
	if n := len(f.llamadas(t, "claude")); n != 0 {
		t.Fatalf("CA-381: un cierre no-entregable no lanza nada: claude corrio %d veces", n)
	}
}

// CA-381: una tarjeta que llega a Tu aceptacion no lanza nada. La persona
// pide la review de 4 lentes (claude) sobre una tarjeta con auto; la review
// entrega y la tarjeta queda lista para aceptar: el log de la review gana
// exactamente "piloto automatico: se detiene: la tarjeta llego a <Nombre>:
// le toca a una persona".
func TestCA381_LaTarjetaQueLlegaATuAceptacionDetieneLaCinta(t *testing.T) {
	f := lnPATH(t)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	const slug = "casi-lista"
	spCartaReviewLimpia(t, "CA-381", root, slug, spPresupuesto+spAuto)
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-381", lnLaunch(t, s, slug, s.Token(), lnCuerpo("pedir-reviewer", "claude", "", nil, "")))
	primero := spCerrado(t, "CA-381", root, r.EnvelopeID)
	if primero.Status != envelope.StatusDeliverable {
		t.Fatalf("CA-381: fixture: la review de 4 lentes cierra entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	lnEsperarLanzamientos(t, "CA-381", s)
	c := spColumna(t, "CA-381", root, slug, boardcmd.ColTuAceptacion)
	quiere := spPrefijo + "se detiene: la tarjeta llego a " + c.ColumnName + ": le toca a una persona"
	l := spEsperarLinea(t, "CA-381", root, r.EnvelopeID, "la tarjeta llego a Tu aceptacion")
	if l != quiere || spUnaLinea(t, "CA-381", root, r.EnvelopeID) != quiere {
		t.Fatalf("CA-381: en una columna humana la cinta escribe\n  %q\nfue\n  %q", quiere, spLineas(root, r.EnvelopeID))
	}
	if otros := spOtros(root, r.EnvelopeID); len(otros) != 0 {
		t.Fatalf("CA-381: una tarjeta en Tu aceptacion no lanza nada: %+v", otros)
	}
	if n := len(f.llamadas(t, "claude")); n != 4 {
		t.Fatalf("CA-381: solo corrieron las 4 lentes de la review: claude corrio %d veces", n)
	}
}

// CA-381: sin auto, el mismo writer entregable que con auto haria lanzar al
// reviewer no lanza nada, y el log de su sobre no gana ninguna linea del
// piloto.
func TestCA381_SinAutoLaCintaNoHaceNada(t *testing.T) {
	f := lnPATH(t)
	spCLI(t, f, "codex", spEscribeCodigo, false)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	const slug = "a-mano"
	spCartaWriter(t, "CA-381", root, slug, spPresupuesto)
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-381", lnLaunch(t, s, slug, s.Token(),
		lnCuerpo("pedir-writer", "codex", "", nil, "Implementa el spec hasta que verify de verde.")))
	primero := spCerrado(t, "CA-381", root, r.EnvelopeID)
	if primero.Status != envelope.StatusDeliverable {
		t.Fatalf("CA-381: fixture: el writer de codex cierra entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	lnEsperarLanzamientos(t, "CA-381", s)

	// con auto, la cinta lanzaria: la tarjeta esta en Review y su accion
	// principal, pedir-reviewer, esta habilitada con claude
	c := spColumna(t, "CA-381", root, slug, boardcmd.ColReview)
	if len(c.Actions) == 0 || c.Actions[0].ID != boardcmd.ActPedirReviewer || !c.Actions[0].Enabled {
		t.Fatalf("CA-381: fixture: la accion principal de la tarjeta es pedir-reviewer, habilitada: %+v", c.Actions)
	}

	// un segundo de gracia: nada llega tarde
	if spEsperar(time.Second, func() bool { return len(spOtros(root, r.EnvelopeID)) > 0 || len(spLineas(root, r.EnvelopeID)) > 0 }) {
		t.Fatalf("CA-381: sin auto la cinta no hace nada: sobres %+v, lineas %q", spOtros(root, r.EnvelopeID), spLineas(root, r.EnvelopeID))
	}
	if n := len(f.llamadas(t, "claude")); n != 0 {
		t.Fatalf("CA-381: sin auto no se pide al reviewer: claude corrio %d veces", n)
	}
}

// CA-381: un lanzamiento de la cinta que choca con el candado de la tarjeta
// (otra pestaña esta lanzando en ese momento) no lanza nada y escribe
// "piloto automatico: no pudo lanzar al reviewer: <error>". El candado es el
// del endpoint launch: la cinta lanza con la misma funcion y sus mismas
// validaciones.
func TestCA381_LaCintaQueChocaConElCandadoLoEscribe(t *testing.T) {
	f := lnPATH(t)
	spCLI(t, f, "codex", spEscribeCodigo, true)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	const slug = "disputada"
	spCartaWriter(t, "CA-381", root, slug, spPresupuesto+spAuto)
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-381", lnLaunch(t, s, slug, s.Token(),
		lnCuerpo("pedir-writer", "codex", "", nil, "Implementa el spec hasta que verify de verde.")))

	// otra pestaña toma el candado de la tarjeta mientras el writer trabaja
	s.launchMu.Lock()
	if s.launching[slug] {
		s.launchMu.Unlock()
		t.Fatal("CA-381: fixture: el candado del POST se suelta con el 202")
	}
	s.launching[slug] = true
	s.launchMu.Unlock()
	soltar := func() {
		s.launchMu.Lock()
		delete(s.launching, slug)
		s.launchMu.Unlock()
	}
	t.Cleanup(soltar)

	f.soltar()
	primero := spCerrado(t, "CA-381", root, r.EnvelopeID)
	if primero.Status != envelope.StatusDeliverable {
		t.Fatalf("CA-381: fixture: el writer de codex cierra entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	l := spEsperarLinea(t, "CA-381", root, r.EnvelopeID, "la cinta choco con el candado de la tarjeta")
	soltar()
	lnEsperarLanzamientos(t, "CA-381", s)
	const pre = spPrefijo + "no pudo lanzar al reviewer: "
	if !strings.HasPrefix(l, pre) || strings.TrimSpace(strings.TrimPrefix(l, pre)) == "" || spUnaLinea(t, "CA-381", root, r.EnvelopeID) != l {
		t.Fatalf("CA-381: con el candado tomado la cinta escribe %q con el error, una vez: %q", pre+"<error>", spLineas(root, r.EnvelopeID))
	}
	if otros := spOtros(root, r.EnvelopeID); len(otros) != 0 {
		t.Fatalf("CA-381: con el candado tomado la cinta no lanza nada: %+v", otros)
	}
	if n := len(f.llamadas(t, "claude")); n != 0 {
		t.Fatalf("CA-381: con el candado tomado no corre el reviewer: claude corrio %d veces", n)
	}
}

// CA-383: tablero.js pinta el chip "piloto automático" en la tarjeta cuyo
// item.auto es hasta-humano (y sigue sin POST, CA-320); acciones.js avisa en
// el dialogo de rol "Piloto automático: si este trabajo termina bien, ..." y
// sigue cumpliendo CA-354 y CA-355.
func TestCA383_ChipYAvisoDelPilotoAutomatico(t *testing.T) {
	html, js, acc := atUI(t)
	for _, quiero := range []string{"piloto automático", "hasta-humano"} {
		if !strings.Contains(js, quiero) {
			t.Fatalf("CA-383: tablero.js pinta el chip del piloto: contiene %q", quiero)
		}
	}
	if !regexp.MustCompile(`\.auto\b`).MatchString(js) {
		t.Fatal("CA-383: el chip sale de item.auto")
	}
	if !strings.Contains(acc, "Piloto automático: si este trabajo termina bien") {
		t.Fatal(`CA-383: el dialogo de rol de acciones.js avisa "Piloto automático: si este trabajo termina bien, ..."`)
	}
	tbRevisarNoMuta(t, html, js)
	TestCA354_AccionesJSActuaConActYNoDecide(t)
	TestCA355_VocabularioYEtiquetasDeLasAcciones(t)
}
