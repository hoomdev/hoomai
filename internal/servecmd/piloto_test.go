// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-381 y CA-383): el piloto automatico dentro del Studio. Es una cinta
// transportadora, no un orquestador: cuando la funcion de un trabajo que
// lanzo el Studio vuelve, el Studio lee el registro del sobre, vuelve a
// derivar la tarjeta, llama a boardcmd.Piloto y, si la tarjeta tiene
// auto: hasta-humano, escribe su decision en el log del sobre que cerro y
// lanza (o no) el rol siguiente con la MISMA funcion que POST .../launch.
//
// Re-expresados por .hoom/specs/autonomia-entre-firmas.md (CA-644, CA-661):
// la vuelta del piloto primero guarda lo que el worktree de la tarea tiene
// sin guardar (y lo dice en una linea `guarda el trabajo de la tarjeta`
// cuando commiteo), y antes de cada lanzamiento escribe
// `piloto automatico: regla <regla>`. Con el spec sin guardar (CA-381,
// CA-497) o el codigo del writer sin commitear (CA-416) la cinta ya no se
// detiene: guarda y sigue.
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
	"os/exec"
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

// spEscribeCodigoSinCommitear es un writer de mentira que deja 450 lineas de
// codigo (la review de 4 lentes queda exigida) SIN commitear y sale 0.
const spEscribeCodigoSinCommitear = `i=0
{
  printf 'package app\n\n'
  while [ $i -lt 450 ]; do printf 'var cinta%d = %d\n' $i $i; i=$((i+1)); done
} > cinta.go
exit 0
`

// spEscribeCodigo es el writer de mentira de la cinta: deja esas 450 lineas
// y las COMMITEA en su rama. Enmienda 4 de review-aislada-y-modelo-elegido:
// la review revisa solo lo commiteado (CA-416), asi que un writer que no
// commitea detiene la cinta (TestCA416_LaCintaSeDetieneSiElWriterNoCommitea);
// commitear desde la cinta es otra spec.
const spEscribeCodigo = `i=0
{
  printf 'package app\n\n'
  while [ $i -lt 450 ]; do printf 'var cinta%d = %d\n' $i $i; i=$((i+1)); done
} > cinta.go
git add -A && git commit -qm cinta || exit 1
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

// spGuarda es la linea del piloto despues de un guardado que commiteo
// (autonomia-entre-firmas, CA-644): `guarda el trabajo de la tarjeta: <commit>`.
var spGuarda = regexp.MustCompile(`^piloto automatico: guarda el trabajo de la tarjeta: ([0-9a-f]{7,40})$`)

// spDecision espera, en el log del sobre id, la linea de la decision del
// piloto (pide al, se detiene, no pudo lanzar al) y devuelve las lineas del
// piloto de esa vuelta sin la de guardar, que si esta va primero y una sola
// vez (CA-644); el commit que nombra es el segundo valor ("" sin ella).
func spDecision(t *testing.T, ca, root, id, que string) ([]string, string) {
	t.Helper()
	esDecision := func(l string) bool {
		for _, p := range []string{"pide al ", "se detiene: ", "no pudo lanzar al "} {
			if strings.HasPrefix(l, spPrefijo+p) {
				return true
			}
		}
		return false
	}
	if !spEsperar(20*time.Second, func() bool {
		for _, l := range spLineas(root, id) {
			if esDecision(l) {
				return true
			}
		}
		return false
	}) {
		t.Fatalf("%s: %s: el log del sobre %s no gano la linea de una decision del piloto: %q\nlog:\n%s", ca, que, id, spLineas(root, id), spLog(root, id))
	}
	ls, commit := spLineas(root, id), ""
	if len(ls) > 0 {
		if m := spGuarda.FindStringSubmatch(ls[0]); m != nil {
			ls, commit = ls[1:], m[1]
		}
	}
	for _, l := range ls {
		if strings.Contains(l, "guarda el trabajo de la tarjeta") {
			t.Fatalf("%s: %s: la linea de guardar va antes de decidir, una vez y con su commit: %q", ca, que, spLineas(root, id))
		}
	}
	return ls, commit
}

// spLanzo exige que la vuelta del sobre id haya lanzado con la regla regla:
// `regla <regla>` y despues la linea de pedir (CA-661), y devuelve esta.
func spLanzo(t *testing.T, ca, root, id, que, regla string) string {
	t.Helper()
	ls, _ := spDecision(t, ca, root, id, que)
	if len(ls) != 2 || ls[0] != spPrefijo+"regla "+regla {
		t.Fatalf("%s: %s: antes de lanzar el piloto escribe %q y despues la linea de pedir: %q", ca, que, spPrefijo+"regla "+regla, spLineas(root, id))
	}
	return ls[1]
}

// spSeDetuvo exige que la vuelta del sobre id se haya detenido, con una sola
// linea de decision, y la devuelve.
func spSeDetuvo(t *testing.T, ca, root, id, que string) string {
	t.Helper()
	ls, _ := spDecision(t, ca, root, id, que)
	if len(ls) != 1 || !strings.HasPrefix(ls[0], spPrefijo+"se detiene: ") {
		t.Fatalf("%s: %s: el piloto se detiene y lo escribe una vez: %q", ca, que, spLineas(root, id))
	}
	return ls[0]
}

// spGit corre git en dir y devuelve su salida.
func spGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// spGuardado exige que commit sea el guardado del piloto (CA-644): en la rama
// hoom/<slug> del worktree, con el mensaje fijo, y que traiga ruta.
func spGuardado(t *testing.T, ca, wt, slug, commit, ruta string) {
	t.Helper()
	if commit == "" {
		t.Fatalf("%s: el piloto guarda lo que habia sin guardar y lo dice: `guarda el trabajo de la tarjeta: <commit>`", ca)
	}
	full := spGit(t, wt, "rev-parse", commit+"^{commit}")
	if !strings.HasPrefix(full, commit) {
		t.Fatalf("%s: el commit de la linea %s es %s", ca, commit, full)
	}
	if msg := spGit(t, wt, "log", "-1", "--format=%s", full); msg != "hoom: guardar la tarjeta "+slug {
		t.Fatalf("%s: el guardado lleva el mensaje %q, lleva %q", ca, "hoom: guardar la tarjeta "+slug, msg)
	}
	if !strings.Contains("\n"+spGit(t, wt, "branch", "--contains", full, "--format=%(refname:short)")+"\n", "\nhoom/"+slug+"\n") {
		t.Fatalf("%s: el guardado %s esta en la rama hoom/%s", ca, full, slug)
	}
	if archivos := spGit(t, wt, "show", "--name-only", "--format=", full); !strings.Contains("\n"+archivos+"\n", "\n"+ruta+"\n") {
		t.Fatalf("%s: el guardado %s trae %s: %q", ca, full, ruta, archivos)
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
//
// Re-expresado por autonomia-entre-firmas (CA-644, CA-661): la linea de
// pedir va despues de `piloto automatico: regla cadena`, y cada vuelta
// puede abrir con la de guardar (el veredicto del writer, la review).
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

	spRevisarPide(t, "CA-381", spLanzo(t, "CA-381", root, r.EnvelopeID, "la cinta lanzo al reviewer", boardcmd.ReglaCadena),
		"reviewer", "claude", 1.25, segundo.ID)

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
	spSeDetuvo(t, "CA-381", root, segundo.ID, "la review que lanzo la cinta termino")
	lnEsperarLanzamientos(t, "CA-381", s)
	if otros := spOtros(root, r.EnvelopeID, segundo.ID); len(otros) != 0 {
		t.Fatalf("CA-381: la cinta no lanza nada despues de la review: %+v", otros)
	}
}

// CA-416 (caso limite del spec review-aislada-y-modelo-elegido, enmienda 4),
// re-expresado por autonomia-entre-firmas (CA-644): "La cinta del Studio que
// lanza la review sobre lo que dejo el writer sin commitear" ya no se
// detiene: guarda y sigue. El mismo writer entregable que en
// TestCA381_LaCintaPideAlReviewerSinOtroPOST deja cinta.go SIN commitear; al
// cerrar, la vuelta del piloto commitea lo no guardado del worktree en
// hoom/<slug> con `hoom: guardar la tarjeta <slug>` (nada en el checkout
// principal), lo dice en el log del writer, y pide al reviewer con la regla
// cadena: la review corre sobre lo commiteado, sin la negativa del arbol
// sucio.
func TestCA416_CA644_LaCintaGuardaLoDelWriterYPideLaReview(t *testing.T) {
	f := lnPATH(t)
	spCLI(t, f, "codex", spEscribeCodigoSinCommitear, false)
	f.cli(t, "claude", false)
	root := tbProyecto(t)
	const slug = "sin-commitear"
	wt := spCartaWriter(t, "CA-416", root, slug, spPresupuesto+spAuto)
	headPrincipal := spGit(t, root, "rev-parse", "HEAD")
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-416", lnLaunch(t, s, slug, s.Token(),
		lnCuerpo("pedir-writer", "codex", "", nil, "Implementa el spec hasta que verify de verde.")))
	primero := spCerrado(t, "CA-416", root, r.EnvelopeID)
	if primero.Status != envelope.StatusDeliverable {
		t.Fatalf("CA-416: fixture: el writer de codex cierra entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	spPersona(t, "CA-416", root, r.EnvelopeID)

	segundo := spEsperarOtro(t, "CA-416", root, "el piloto guardo lo del writer y pide la review", r.EnvelopeID)
	ls, commit := spDecision(t, "CA-416", root, r.EnvelopeID, "el writer dejo cinta.go sin commitear")
	spGuardado(t, "CA-644", wt, slug, commit, "cinta.go")
	if len(ls) != 2 || ls[0] != spPrefijo+"regla "+boardcmd.ReglaCadena {
		t.Fatalf("CA-416: despues de guardar, regla cadena y la linea de pedir: %q", spLineas(root, r.EnvelopeID))
	}
	segundo = spCerrado(t, "CA-416", root, segundo.ID)
	spRevisarPide(t, "CA-416", ls[1], "reviewer", "claude", 1.25, segundo.ID)
	if segundo.Role != "reviewer" || segundo.Provider != "claude" || segundo.Task != slug {
		t.Fatalf("CA-416: la cinta pide al reviewer de la tabla con claude: %+v", segundo)
	}
	spPiloto(t, "CA-416", root, segundo)
	lnEsperarLanzamientos(t, "CA-416", s)

	const sucio = "la review revisa solo lo commiteado y hay cambios sin commitear (cinta.go)"
	for _, id := range []string{r.EnvelopeID, segundo.ID} {
		if strings.Contains(spLog(root, id), sucio) {
			t.Fatalf("CA-416: con lo del writer guardado la review no se niega por el arbol sucio:\n%s", spLog(root, id))
		}
	}
	if n := len(f.llamadas(t, "claude")); n == 0 {
		t.Fatal("CA-416: la review que pidio la cinta corre sus pasadas con claude")
	}
	if n := len(f.llamadas(t, "codex")); n != 1 {
		t.Fatalf("CA-416: la cinta no reintenta al writer: codex corrio %d veces", n)
	}
	if h := spGit(t, root, "rev-parse", "HEAD"); h != headPrincipal {
		t.Fatalf("CA-644: el piloto no commitea en el checkout principal: HEAD %s -> %s", headPrincipal, h)
	}
	if b := spGit(t, root, "rev-parse", "--abbrev-ref", "HEAD"); b != "main" {
		t.Fatalf("CA-644: el checkout principal sigue en main: %s", b)
	}
}

// spMarcaArquitecto es la marca del pedido del arquitecto de mentira: sus
// CLIs falsos la usan para saber que les toca escribir el spec.
const spMarcaArquitecto = "SPEC-DE-LA-CINTA"

// spEscribeYGuardaSpec es el arquitecto de mentira que GUARDA lo que entrega:
// con la marca SPEC-DE-LA-CINTA deja en su arbol el spec byte a byte igual a
// fuente y lo commitea en su espacio de trabajo (como spEscribeCodigo commitea
// lo del writer). Cualquier otro pedido es el del test-writer que lanza la
// cinta: sale 1 sin tocar nada.
func spEscribeYGuardaSpec(fuente, slug string) string {
	spec := ".hoom/specs/" + slug + ".md"
	return "case \"$*\" in\n*" + spMarcaArquitecto + "*) mkdir -p .hoom/specs && cp '" + fuente + "' '" + spec + "' && " +
		"git add '" + spec + "' && git commit -qm spec || exit 1 ;;\n*) exit 1 ;;\nesac\nexit 0\n"
}

// CA-381, el tramo arquitecto -> test-writer tal como lo re-expresa CA-497
// (verbos-de-la-tarjeta): con el spec del arquitecto GUARDADO, la cinta pide
// al test-writer como hoy. Una persona pide al arquitecto desde Backlog; el
// spec que entrega ya tenia aprobacion y el arquitecto lo commitea en su
// espacio de trabajo, asi que la tarjeta salta a Test-writer con el spec en
// lo guardado, y sin otro POST la cinta pide al test-writer con el provider
// del arquitecto (claude, ok y con tope) y lo que queda (5 USD), con
// "piloto": true en su registro.
//
// Supuesto sobre como cierra el sobre del test-writer: su CLI falso sale 1, y
// un run con exit distinto de cero corta el sobre, que cierra no-entregable
// (CA-143 de hoom-agent-sobre-determinista; el mismo supuesto que el writer
// de TestCA381_UnCierreNoEntregableDetieneLaCinta). Los specs no dicen como
// cierra un test-writer que sale 0 sin escribir ningun test, y este test no
// lo supone. Ante ese rojo la cinta se detiene, lo escribe en el log del
// test-writer con el Why del contrato (caso 2) y no relanza: claude corrio
// una vez como arquitecto y una como test-writer (con el spec en el commit,
// el arbol ciego se arma y su CLI llega a correr), y nada mas.
//
// Re-expresado por autonomia-entre-firmas (CA-642, CA-644, CA-661): el corte
// es el caso 8 de la tabla nueva, con el Why exacto de hoy; la linea de pedir
// va despues de `regla cadena` y cada vuelta puede abrir con la de guardar.
// Que la cinta deje pasar el rojo ESPERADO del test-writer (un veredicto
// rojo de un sobre entregable) lo fijan los tests de boardcmd (CA-643).
func TestCA381_LaCintaPideAlTestWriterYSeDetieneAnteSuRojo(t *testing.T) {
	f := lnPATH(t)
	root := tbProyecto(t)
	const slug = "desde-backlog"
	fuente := spCartaBacklogAprobada(t, "CA-381", root, slug, spPresupuesto+spAuto)
	spCLI(t, f, "claude", spEscribeYGuardaSpec(fuente, slug), false)
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-381", lnLaunch(t, s, slug, s.Token(),
		lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec de la tarjeta ("+spMarcaArquitecto+").")))
	primero := spCerrado(t, "CA-381", root, r.EnvelopeID)
	if primero.Status != envelope.StatusDeliverable {
		t.Fatalf("CA-381: fixture: el arquitecto que entrega y guarda el spec cierra entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	spPersona(t, "CA-381", root, r.EnvelopeID)

	segundo := spEsperarOtro(t, "CA-381", root, "arquitecto entregable, spec guardado y la tarjeta en Test-writer", r.EnvelopeID)
	segundo = spCerrado(t, "CA-381", root, segundo.ID)
	if segundo.Role != "test-writer" || segundo.Provider != "claude" || segundo.Task != slug {
		t.Fatalf("CA-381: la cinta pide al test-writer de la tabla con el provider del arquitecto: %+v", segundo)
	}
	spPiloto(t, "CA-381", root, segundo)
	spRevisarPide(t, "CA-381", spLanzo(t, "CA-381", root, r.EnvelopeID, "la cinta lanzo al test-writer", boardcmd.ReglaCadena),
		"test-writer", "claude", 5, segundo.ID)

	if segundo.Status != envelope.StatusNotDeliverable {
		t.Fatalf("CA-381: fixture: el test-writer cuyo run sale 1 cierra no-entregable: %+v\n%s", segundo, spLog(root, segundo.ID))
	}
	const quiere = spPrefijo + "se detiene: el trabajo del test-writer no cerro entregable (no-entregable): el piloto se detiene ante un rojo"
	l := spSeDetuvo(t, "CA-381", root, segundo.ID, "el test-writer que lanzo la cinta cerro no-entregable")
	lnEsperarLanzamientos(t, "CA-381", s)
	if l != quiere {
		t.Fatalf("CA-381: ante un rojo la cinta escribe\n  %q\nfue\n  %q", quiere, spLineas(root, segundo.ID))
	}
	if otros := spOtros(root, r.EnvelopeID, segundo.ID); len(otros) != 0 {
		t.Fatalf("CA-381: la cinta no relanza tras un rojo: %+v", otros)
	}

	// claude corrio una vez como arquitecto y una como el test-writer que
	// lanzo la cinta, con el tope que propone la accion (lo que queda, 5 USD)
	var arquitecto, testWriter [][]string
	for _, args := range f.llamadas(t, "claude") {
		if strings.Contains(strings.Join(args, " "), spMarcaArquitecto) {
			arquitecto = append(arquitecto, args)
		} else {
			testWriter = append(testWriter, args)
		}
	}
	if len(arquitecto) != 1 || len(testWriter) != 1 {
		t.Fatalf("CA-381: con el spec guardado claude corre una vez como arquitecto y una como test-writer (la cinta no relanza): arquitecto %d, test-writer %d", len(arquitecto), len(testWriter))
	}
	if v, ok := lnSigue(testWriter[0], "--max-budget-usd"); !ok || v != "5" {
		t.Fatalf("CA-381: el test-writer que lanzo la cinta corre con lo que queda (--max-budget-usd 5): %q", testWriter[0])
	}
}

// CA-381 y CA-497 (verbos-de-la-tarjeta re-expresa el tramo arquitecto ->
// test-writer de CA-381), re-expresados por autonomia-entre-firmas (CA-644):
// el mismo arquitecto desde Backlog deja el spec en su espacio de trabajo SIN
// guardarlo. Antes la cinta se detenia ("el spec tiene cambios sin
// guardar..."); ahora la vuelta del piloto guarda primero: commitea el spec en
// hoom/<slug> con `hoom: guardar la tarjeta <slug>` (nada en el checkout
// principal), la tarjeta queda en Test-writer con pedir-test-writer
// habilitada, y el piloto pide al test-writer con la regla cadena, el provider
// del arquitecto y lo que queda (5 USD). El log del arquitecto gana, en orden,
// la linea de guardar, `regla cadena` y la de pedir.
func TestCA381_CA497_CA644_LaCintaGuardaElSpecYPideAlTestWriter(t *testing.T) {
	f := lnPATH(t)
	root := tbProyecto(t)
	const slug = "sin-guardar"
	fuente := spCartaBacklogAprobada(t, "CA-497", root, slug, spPresupuesto+spAuto)
	spCLI(t, f, "claude", spEscribeSpec(fuente, slug), false)
	headPrincipal := spGit(t, root, "rev-parse", "HEAD")
	s := lnServidor(t, root, f)

	r := lnAceptado(t, "CA-497", lnLaunch(t, s, slug, s.Token(),
		lnCuerpo("pedir-arquitecto", "claude", "", nil, "Escribi el spec de la tarjeta ("+spMarcaArquitecto+").")))
	primero := spCerrado(t, "CA-497", root, r.EnvelopeID)
	if primero.Status != envelope.StatusDeliverable {
		t.Fatalf("CA-497: fixture: el arquitecto que entrega el spec sin guardarlo cierra entregable: %+v\n%s", primero, spLog(root, r.EnvelopeID))
	}
	spPersona(t, "CA-497", root, r.EnvelopeID)

	segundo := spEsperarOtro(t, "CA-497", root, "el piloto guardo el spec y pide al test-writer", r.EnvelopeID)
	ls, commit := spDecision(t, "CA-497", root, r.EnvelopeID, "el arquitecto dejo el spec sin guardar")
	wt := filepath.Join(root, ".hoom", "worktrees", slug)
	spGuardado(t, "CA-644", wt, slug, commit, ".hoom/specs/"+slug+".md")
	if len(ls) != 2 || ls[0] != spPrefijo+"regla "+boardcmd.ReglaCadena {
		t.Fatalf("CA-497: despues de guardar, regla cadena y la linea de pedir: %q", spLineas(root, r.EnvelopeID))
	}
	segundo = spCerrado(t, "CA-497", root, segundo.ID)
	spRevisarPide(t, "CA-497", ls[1], "test-writer", "claude", 5, segundo.ID)
	if segundo.Role != "test-writer" || segundo.Provider != "claude" || segundo.Task != slug {
		t.Fatalf("CA-497: con el spec guardado la cinta pide al test-writer con el provider del arquitecto: %+v", segundo)
	}
	spPiloto(t, "CA-497", root, segundo)
	if crudo := spCrudo(t, root, segundo.ID); strings.TrimSpace(string(crudo["regla"])) != `"`+boardcmd.ReglaCadena+`"` ||
		strings.TrimSpace(string(crudo["columna"])) != `"`+boardcmd.ColTestWriter+`"` {
		t.Fatalf("CA-661: el sobre que lanzo el piloto lleva su regla y la columna al lanzar: %s %s", crudo["regla"], crudo["columna"])
	}
	lnEsperarLanzamientos(t, "CA-497", s)

	// el spec guardado ya no es "cambios sin guardar"
	const why = "el spec tiene cambios sin guardar y el test-writer trabaja con lo guardado: guarda primero"
	if strings.Contains(spLog(root, r.EnvelopeID), why) {
		t.Fatalf("CA-497: con el spec guardado por el piloto la cinta no se detiene por el spec sin guardar:\n%s", spLog(root, r.EnvelopeID))
	}
	if st := spGit(t, wt, "status", "--porcelain", "--", ".hoom/specs/"+slug+".md"); st != "" {
		t.Fatalf("CA-644: el spec queda guardado en la rama de la tarea: %q", st)
	}
	if n := len(f.llamadas(t, "claude")); n != 2 {
		t.Fatalf("CA-497: claude corrio como arquitecto y como el test-writer que pidio la cinta: %d veces", n)
	}
	if h := spGit(t, root, "rev-parse", "HEAD"); h != headPrincipal {
		t.Fatalf("CA-644: el piloto no commitea en el checkout principal: HEAD %s -> %s", headPrincipal, h)
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
//
// Re-expresado por autonomia-entre-firmas (CA-644, CA-661): la vuelta puede
// abrir con la linea de guardar y escribir `regla cadena` antes de intentar
// el lanzamiento; la linea de la decision sigue siendo una y es la de hoy.
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
	ls, _ := spDecision(t, "CA-381", root, r.EnvelopeID, "la cinta choco con el candado de la tarjeta")
	soltar()
	lnEsperarLanzamientos(t, "CA-381", s)
	const pre = spPrefijo + "no pudo lanzar al reviewer: "
	if len(ls) == 2 && ls[0] == spPrefijo+"regla "+boardcmd.ReglaCadena {
		ls = ls[1:]
	}
	if len(ls) != 1 || !strings.HasPrefix(ls[0], pre) || strings.TrimSpace(strings.TrimPrefix(ls[0], pre)) == "" {
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
