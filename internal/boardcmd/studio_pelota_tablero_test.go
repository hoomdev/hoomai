// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md en
// internal/boardcmd: la pelota del tablero (Board.pelota: orden, caja y
// cuenta, CA-728) y la de cada tarjeta en lo que emite hoom board --json
// (CA-722), con Build sobre proyectos en disco (repos git en t.TempDir(), sin
// ningun CLI de IA) y una vez con el binario real.
//
// Los valores esperados salen del contrato: el orden de la lista del tablero
// v3 (TE TOCA de la columna mas cerca de Hecho a la mas lejos, CORRIENDO,
// DETENIDA, EN COLA y HECHO, y dentro de cada uno el orden del tablero), la
// caja (la primera de TE TOCA, si no la primera de DETENIDA, si no "") y la
// cuenta con sus cinco claves siempre. El grupo de cada tarjeta se calcula con
// la tabla del contrato sobre sus campos (spGrupo), no con su pelota.
package boardcmd

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// spPelotaBoard es Board.pelota tal como la trae el JSON.
type spPelotaBoard struct {
	Orden  []string       `json:"orden"`
	Caja   string         `json:"caja"`
	Cuenta map[string]int `json:"cuenta"`
}

// spClavesCuenta son las cinco claves de cuenta.
var spClavesCuenta = []string{"te_toca", "corriendo", "detenida", "en_cola", "hecho"}

var spClaveDeGrupo = map[string]string{spTeToca: "te_toca", spCorriendo: "corriendo", spDetenida: "detenida",
	spEnCola: "en_cola", spHecho: "hecho"}

// spPelotaBoardJSON lee "pelota" del JSON crudo del tablero y exige su forma:
// orden (lista, nunca null), caja (string) y cuenta (objeto con las cinco
// claves, numeros).
func spPelotaBoardJSON(t *testing.T, ca, nombre string, raw []byte) spPelotaBoard {
	t.Helper()
	board := spObjeto(t, ca, nombre, "el tablero", raw)
	p, ok := board["pelota"]
	if !ok {
		t.Fatalf("%s: [%s] el tablero trae \"pelota\" en su JSON (Board.pelota); no la trae. Claves: %v", ca, nombre, spClaves(board))
	}
	obj := spObjeto(t, ca, nombre, "pelota del tablero", p)
	spCampos(t, ca, nombre, "pelota del tablero", obj, map[string]string{"orden": "array", "caja": "string", "cuenta": "object"})
	cuenta := spObjeto(t, ca, nombre, "pelota.cuenta", obj["cuenta"])
	quiero := map[string]string{}
	for _, k := range spClavesCuenta {
		quiero[k] = "number"
	}
	spCampos(t, ca, nombre, "pelota.cuenta", cuenta, quiero)
	var out spPelotaBoard
	if err := json.Unmarshal(p, &out); err != nil {
		t.Fatalf("%s: [%s] la pelota del tablero se decodifica: %v", ca, nombre, err)
	}
	return out
}

// spPelotaDelTablero serializa el tablero como hoom board --json (JSONBytes).
func spPelotaDelTablero(t *testing.T, ca, nombre string, b Board) (spPelotaBoard, []byte) {
	t.Helper()
	raw, err := JSONBytes(b)
	if err != nil {
		t.Fatalf("%s: [%s] JSONBytes: %v", ca, nombre, err)
	}
	return spPelotaBoardJSON(t, ca, nombre, raw), raw
}

// spOraculoTablero calcula orden, caja y cuenta desde las columnas del
// tablero y la tabla de grupos del contrato.
func spOraculoTablero(b Board) spPelotaBoard {
	var teToca []string
	for i := len(b.Columns) - 1; i >= 0; i-- {
		for _, c := range b.Columns[i].Cards {
			if spGrupo(c) == spTeToca {
				teToca = append(teToca, c.Slug)
			}
		}
	}
	otros := map[string][]string{}
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if g := spGrupo(c); g != spTeToca {
				otros[g] = append(otros[g], c.Slug)
			}
		}
	}
	out := spPelotaBoard{Orden: []string{}, Cuenta: map[string]int{}}
	out.Orden = append(out.Orden, teToca...)
	for _, g := range []string{spCorriendo, spDetenida, spEnCola, spHecho} {
		out.Orden = append(out.Orden, otros[g]...)
	}
	switch {
	case len(teToca) > 0:
		out.Caja = teToca[0]
	case len(otros[spDetenida]) > 0:
		out.Caja = otros[spDetenida][0]
	}
	for _, k := range spClavesCuenta {
		out.Cuenta[k] = 0
	}
	out.Cuenta["te_toca"] = len(teToca)
	for g, xs := range otros {
		out.Cuenta[spClaveDeGrupo[g]] = len(xs)
	}
	return out
}

func spSlugsDe(b Board) []string {
	var out []string
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			out = append(out, c.Slug)
		}
	}
	return out
}

// spMiraTablero compara Board.pelota con el oraculo y sus propiedades:
// orden es una permutacion de los slugs de las tarjetas y cuenta suma la
// cantidad de tarjetas.
func spMiraTablero(t *testing.T, ca, nombre string, b Board) spPelotaBoard {
	t.Helper()
	got, _ := spPelotaDelTablero(t, ca, nombre, b)
	quiero := spOraculoTablero(b)
	slugs := spSlugsDe(b)
	a, c := append([]string{}, got.Orden...), append([]string{}, slugs...)
	sort.Strings(a)
	sort.Strings(c)
	if len(a) != len(c) || (len(a) > 0 && !reflect.DeepEqual(a, c)) {
		t.Fatalf("%s: [%s] pelota.orden es una permutacion de los slugs de las tarjetas %v, fue %v", ca, nombre, slugs, got.Orden)
	}
	if !reflect.DeepEqual(got.Orden, quiero.Orden) {
		t.Fatalf("%s: [%s] pelota.orden es el de la lista del tablero v3\n quiero %v\n fue    %v", ca, nombre, quiero.Orden, got.Orden)
	}
	if got.Caja != quiero.Caja {
		t.Fatalf("%s: [%s] pelota.caja es %q (la primera de TE TOCA, si no la de DETENIDA), fue %q (orden %v)", ca, nombre, quiero.Caja, got.Caja, got.Orden)
	}
	suma := 0
	for _, k := range spClavesCuenta {
		suma += got.Cuenta[k]
	}
	if suma != len(slugs) {
		t.Fatalf("%s: [%s] pelota.cuenta suma la cantidad de tarjetas (%d), sumo %d: %v", ca, nombre, len(slugs), suma, got.Cuenta)
	}
	if !reflect.DeepEqual(got.Cuenta, quiero.Cuenta) {
		t.Fatalf("%s: [%s] pelota.cuenta es %v, fue %v", ca, nombre, quiero.Cuenta, got.Cuenta)
	}
	return got
}

// ---------------------------------------------------------------------------
// Proyectos en disco.
// ---------------------------------------------------------------------------

// Las formas de una tarjeta en disco.
const (
	spDCola       = "cola"       // item sin spec: Backlog
	spDArquitecto = "arquitecto" // spec sin la seccion riesgos: Arquitecto
	spDFirma      = "firma"      // spec que pasa el lint, sin aprobar: Tu aprobacion
	spDTests      = "tests"      // spec aprobado con un criterio sin test: Test-writer
	spDCorre      = "corre"      // item con un run suelto vivo: CORRIENDO
	spDCortada    = "cortada"    // item con un sobre abierto cuyo dueno murio: DETENIDA
	spDRoja       = "roja"       // item con un sobre cerrado no entregable: DETENIDA
	spDHecha      = "hecha"      // item con hecho_en: Hecho
)

// spGrupoDeForma es el grupo que el contrato da a cada forma.
var spGrupoDeForma = map[string]string{spDCola: spEnCola, spDArquitecto: spTeToca, spDFirma: spTeToca, spDTests: spTeToca,
	spDCorre: spCorriendo, spDCortada: spDetenida, spDRoja: spDetenida, spDHecha: spHecho}

type spDisco struct {
	t    *testing.T
	root string
	now  time.Time
	n    int
	muer int
}

func spNuevoDisco(t *testing.T) *spDisco {
	t.Helper()
	return &spDisco{t: t, root: bdRepo(t, ""), now: time.Now().UTC().Truncate(time.Second), muer: bdPIDMuerto(t)}
}

func (d *spDisco) id(pre string) string {
	d.n++
	return fmt.Sprintf("20260922T%02d%02d%02d_%s%04d", 10+d.n/3600%10, d.n/60%60, d.n%60, pre, d.n)
}

// tarjeta escribe en disco el item slug con la forma pedida.
func (d *spDisco) tarjeta(slug, titulo, forma string) {
	t := d.t
	t.Helper()
	extra := ""
	if forma == spDHecha {
		extra = "hecho_en: 2026-09-23T10:00:00Z\n"
	}
	bdItemArchivo(t, d.root, slug, titulo, extra)
	spec := ".hoom/specs/" + slug + ".md"
	switch forma {
	case spDArquitecto:
		bdEscribir(t, d.root, spec, bdSpecTexto("- CA-1: algo.", false))
	case spDFirma:
		bdEscribir(t, d.root, spec, bdSpecTexto("- CA-1: algo.", true))
	case spDTests:
		bdEscribir(t, d.root, spec, bdSpecTexto("- CA-1: algo.", true))
		bdAprobar(t, d.root, slug)
	case spDCorre:
		bdRunDisco(t, d.root, runcmd.Meta{ID: d.id("rn"), Provider: "claude", Role: "writer", Task: slug, Dir: d.root,
			CreatedAt: d.now.Add(-10 * time.Minute), Status: runcmd.StatusRunning, ExitCode: -1, PID: os.Getpid()})
	case spDCortada:
		run := d.id("rn")
		bdSobreDisco(t, d.root, envelope.Record{ID: d.id("sb"), Role: "writer", Provider: "claude", Task: slug, Dir: d.root,
			RunID: run, Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning,
			StartedAt: d.now.Add(-30 * time.Minute), UpdatedAt: d.now.Add(-time.Minute)})
		bdRunDisco(t, d.root, runcmd.Meta{ID: run, Provider: "claude", Role: "writer", Task: slug, Dir: d.root,
			CreatedAt: d.now.Add(-30 * time.Minute), Status: runcmd.StatusRunning, ExitCode: -1, PID: d.muer})
	case spDRoja:
		fin := d.now.Add(-35 * time.Minute)
		bdSobreDisco(t, d.root, envelope.Record{ID: d.id("sb"), Role: "writer", Provider: "claude", Task: slug, Dir: d.root,
			Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusNotDeliverable, ExitCode: 1, Note: "se corto el agente",
			StartedAt: fin.Add(-5 * time.Minute), UpdatedAt: fin, EndedAt: fin})
	}
}

// roto escribe un item invalido: va a warnings, no es tarjeta.
func (d *spDisco) roto(slug string) {
	bdEscribir(d.t, d.root, ".hoom/items/"+slug+".yaml", bdItemYAML("Roto", "columna: hecho\n"))
}

func (d *spDisco) build() Board {
	d.t.Helper()
	b, err := Build(d.root, "main", "high", d.now)
	if err != nil {
		d.t.Fatalf("Build: %v", err)
	}
	return b
}

// spExigeGrupos exige que cada slug caiga en el grupo de su forma (lo que el
// fixture quiere probar), con la tabla del contrato sobre sus campos.
func spExigeGrupos(t *testing.T, b Board, formas map[string]string) {
	t.Helper()
	for slug, forma := range formas {
		c := bdCard(t, b, slug)
		if g := spGrupo(c); g != spGrupoDeForma[forma] {
			t.Fatalf("fixture: %s (%s) cae en %s, se esperaba %s (columna %s, running=%v, interrupted=%v, red=%+v, missing=%q)",
				slug, forma, g, spGrupoDeForma[forma], c.Column, c.Running != nil, c.Interrupted != nil, c.Red, c.Missing)
		}
	}
}

// spMezclado es el proyecto con una tarjeta de cada forma (dos de algunas) y
// un item roto.
func spMezclado(t *testing.T) (*spDisco, map[string]string) {
	t.Helper()
	d := spNuevoDisco(t)
	formas := map[string]string{
		"pedido-a": spDCola, "pedido-b": spDCola,
		"arq-uno": spDArquitecto, "arq-dos": spDArquitecto,
		"firma": spDFirma, "tests": spDTests,
		"corre": spDCorre, "cortada": spDCortada, "roja": spDRoja,
		"lista": spDHecha, "lista-dos": spDHecha,
	}
	for slug, forma := range formas {
		d.tarjeta(slug, "Tarjeta "+slug, forma)
	}
	d.roto("roto")
	return d, formas
}

// ===========================================================================
// CA-728
// ===========================================================================

// CA-728: con tarjetas en todos los grupos, Board.pelota trae orden (todas,
// en el orden del tablero v3: TE TOCA de Test-writer a Arquitecto, despues
// CORRIENDO, DETENIDA, EN COLA y HECHO), caja (la primera de TE TOCA) y cuenta
// con sus cinco claves; el item roto (warnings) no esta en orden.
func TestCA728_PelotaDelTableroMezclado(t *testing.T) {
	d, formas := spMezclado(t)
	b := d.build()
	spExigeGrupos(t, b, formas)
	if len(b.Warnings) == 0 {
		t.Fatal("fixture: el item roto va a warnings")
	}
	got := spMiraTablero(t, "CA-728", "mezclado", b)
	if got.Caja != "tests" {
		t.Fatalf("CA-728: la caja es la primera de TE TOCA, la de la columna mas cerca de Hecho (tests, en Test-writer): %q (orden %v)", got.Caja, got.Orden)
	}
	if len(got.Orden) < 2 || got.Orden[0] != "tests" || got.Orden[1] != "firma" {
		t.Fatalf("CA-728: TE TOCA va de la columna mas cerca de Hecho a la mas lejos: Test-writer, Tu aprobacion, Arquitecto: %v", got.Orden)
	}
	quiero := map[string]int{"te_toca": 4, "corriendo": 1, "detenida": 2, "en_cola": 2, "hecho": 2}
	if !reflect.DeepEqual(got.Cuenta, quiero) {
		t.Fatalf("CA-728: cuenta es %v, fue %v", quiero, got.Cuenta)
	}
	for _, s := range got.Orden {
		if s == "roto" {
			t.Fatalf("CA-728: un item invalido (en warnings) no entra en orden: %v", got.Orden)
		}
	}
	// el orden de los grupos: CORRIENDO, DETENIDA, EN COLA y HECHO, despues de TE TOCA
	pos := map[string]int{}
	for i, s := range got.Orden {
		pos[s] = i
	}
	for _, par := range [][2]string{{"arq-uno", "corre"}, {"arq-dos", "corre"}, {"corre", "cortada"}, {"corre", "roja"},
		{"cortada", "pedido-a"}, {"roja", "pedido-b"}, {"pedido-a", "lista"}, {"pedido-b", "lista-dos"}} {
		if pos[par[0]] > pos[par[1]] {
			t.Fatalf("CA-728: %s va antes que %s en orden: %v", par[0], par[1], got.Orden)
		}
	}
}

// CA-728: sin TE TOCA, la caja es la primera de DETENIDA en el orden.
func TestCA728_CajaEsLaPrimeraDetenidaSinTeToca(t *testing.T) {
	d := spNuevoDisco(t)
	formas := map[string]string{"pedido": spDCola, "cortada": spDCortada, "roja": spDRoja, "corre": spDCorre, "lista": spDHecha}
	for slug, forma := range formas {
		d.tarjeta(slug, "Tarjeta "+slug, forma)
	}
	b := d.build()
	spExigeGrupos(t, b, formas)
	got := spMiraTablero(t, "CA-728", "sin te toca", b)
	if got.Caja != "cortada" && got.Caja != "roja" {
		t.Fatalf("CA-728: sin TE TOCA la caja es la primera de DETENIDA: %q (orden %v)", got.Caja, got.Orden)
	}
	if got.Cuenta["te_toca"] != 0 || got.Cuenta["detenida"] != 2 {
		t.Fatalf("CA-728: cuenta sin TE TOCA y con dos DETENIDA: %v", got.Cuenta)
	}
}

// CA-728: sin TE TOCA ni DETENIDA, la caja es "" (aunque haya EN COLA,
// CORRIENDO y HECHO), y cuenta trae los ceros.
func TestCA728_CajaVaciaSinTeTocaNiDetenida(t *testing.T) {
	d := spNuevoDisco(t)
	formas := map[string]string{"pedido": spDCola, "corre": spDCorre, "lista": spDHecha}
	for slug, forma := range formas {
		d.tarjeta(slug, "Tarjeta "+slug, forma)
	}
	b := d.build()
	spExigeGrupos(t, b, formas)
	got := spMiraTablero(t, "CA-728", "sin te toca ni detenida", b)
	if got.Caja != "" {
		t.Fatalf("CA-728: sin TE TOCA ni DETENIDA la caja es \"\": %q", got.Caja)
	}
	quiero := map[string]int{"te_toca": 0, "corriendo": 1, "detenida": 0, "en_cola": 1, "hecho": 1}
	if !reflect.DeepEqual(got.Cuenta, quiero) {
		t.Fatalf("CA-728: cuenta con sus cinco claves, ceros incluidos: %v, fue %v", quiero, got.Cuenta)
	}
}

// CA-728: un tablero vacio da orden [] (no null), caja "" y cinco ceros; un
// tablero con solo un item roto, lo mismo.
func TestCA728_TableroVacio(t *testing.T) {
	for _, conRoto := range []bool{false, true} {
		d := spNuevoDisco(t)
		nombre := "vacio"
		if conRoto {
			d.roto("roto")
			nombre = "solo un item roto"
		}
		b := d.build()
		if len(spSlugsDe(b)) != 0 {
			t.Fatalf("fixture [%s]: sin tarjetas: %v", nombre, spSlugsDe(b))
		}
		got, raw := spPelotaDelTablero(t, "CA-728", nombre, b)
		var crudo struct {
			Pelota map[string]json.RawMessage `json:"pelota"`
		}
		if err := json.Unmarshal(raw, &crudo); err != nil {
			t.Fatal(err)
		}
		if spTipo(crudo.Pelota["orden"]) != "array" || len(got.Orden) != 0 {
			t.Fatalf("CA-728: [%s] orden es [] (no null): %s", nombre, crudo.Pelota["orden"])
		}
		if got.Caja != "" {
			t.Fatalf("CA-728: [%s] caja es \"\": %q", nombre, got.Caja)
		}
		quiero := map[string]int{"te_toca": 0, "corriendo": 0, "detenida": 0, "en_cola": 0, "hecho": 0}
		if !reflect.DeepEqual(got.Cuenta, quiero) {
			t.Fatalf("CA-728: [%s] cuenta tiene sus cinco ceros: %v", nombre, got.Cuenta)
		}
	}
}

// CA-728: dentro de un grupo, y de una columna de TE TOCA, va el orden del
// tablero: dos tarjetas de Arquitecto y dos de Backlog salen en el orden de su
// columna.
func TestCA728_DentroDeUnGrupoElOrdenDelTablero(t *testing.T) {
	d := spNuevoDisco(t)
	formas := map[string]string{"b-arq": spDArquitecto, "a-arq": spDArquitecto, "c-arq": spDArquitecto,
		"z-cola": spDCola, "m-cola": spDCola}
	for slug, forma := range formas {
		d.tarjeta(slug, "Tarjeta "+slug, forma)
	}
	b := d.build()
	spExigeGrupos(t, b, formas)
	got := spMiraTablero(t, "CA-728", "orden del tablero", b)
	var arq, cola []string
	for _, c := range b.Columns[1].Cards {
		arq = append(arq, c.Slug)
	}
	for _, c := range b.Columns[0].Cards {
		cola = append(cola, c.Slug)
	}
	if !reflect.DeepEqual(got.Orden, append(arq, cola...)) {
		t.Fatalf("CA-728: TE TOCA (Arquitecto, en su orden) y despues EN COLA (Backlog, en su orden): %v, fue %v", append(arq, cola...), got.Orden)
	}
}

// CA-728 (propiedad, testing/quick): para cualquier mezcla de formas en
// disco, orden es una permutacion de las tarjetas en el orden del contrato,
// la caja es la del contrato y cuenta suma la cantidad de tarjetas.
func TestCA728_PropiedadOrdenCajaYCuenta(t *testing.T) {
	formas := []string{spDCola, spDArquitecto, spDFirma, spDTests, spDCorre, spDCortada, spDRoja, spDHecha}
	prop := func(cuantas [8]uint8, roto bool) bool {
		d := spNuevoDisco(t)
		for i, forma := range formas {
			for k := 0; k < int(cuantas[i]%3); k++ {
				d.tarjeta(fmt.Sprintf("%s-%d", forma, k), fmt.Sprintf("Tarjeta %s %d", forma, k), forma)
			}
		}
		if roto {
			d.roto("roto")
		}
		spMiraTablero(t, "CA-728", "propiedad", d.build())
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 6, Rand: rand.New(rand.NewSource(728))}); err != nil {
		t.Fatalf("CA-728: %v", err)
	}
}

// ===========================================================================
// CA-722 en lo que emite hoom board --json.
// ===========================================================================

// CA-722: en JSONBytes (lo que emite hoom board --json), cada tarjeta de cada
// columna trae su pelota, nunca null, con todos sus campos, y es la del
// contrato.
func TestCA722_BoardJSONTraePelotaEnCadaTarjeta(t *testing.T) {
	d, formas := spMezclado(t)
	b := d.build()
	spExigeGrupos(t, b, formas)
	raw, err := JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"pelota": null`) || strings.Contains(string(raw), `"pelota":null`) {
		t.Fatalf("CA-722: ninguna pelota es null en hoom board --json:\n%s", raw)
	}
	var tablero struct {
		Columns []struct {
			Cards []json.RawMessage `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(raw, &tablero); err != nil {
		t.Fatal(err)
	}
	vistas := 0
	for _, col := range tablero.Columns {
		for _, rc := range col.Cards {
			var c Card
			if err := json.Unmarshal(rc, &c); err != nil {
				t.Fatal(err)
			}
			p := spPelotaJSON(t, "CA-722", "hoom board --json "+c.Slug, rc)
			spMiraTodo(t, "hoom board --json "+c.Slug, bdCard(t, b, c.Slug), p)
			vistas++
		}
	}
	if vistas != len(formas) {
		t.Fatalf("fixture: el tablero tiene %d tarjetas, tuvo %d", len(formas), vistas)
	}
}

// CA-722, CA-728: el binario real: hoom board --json trae la pelota del
// tablero y la de cada tarjeta.
func TestCA722_CA728_E2EHoomBoardJSONTraePelota(t *testing.T) {
	bin := bxBinario(t)
	d, _ := spMezclado(t)
	r := bxCorrer(t, bin, d.root, "board", "--json")
	bxOK(t, "CA-722", r, "board", "--json")
	pb := spPelotaBoardJSON(t, "CA-728", "hoom board --json", []byte(r.stdout))
	if len(pb.Orden) != 11 {
		t.Fatalf("CA-728: hoom board --json trae en orden las 11 tarjetas: %v", pb.Orden)
	}
	var tablero struct {
		Columns []struct {
			Cards []json.RawMessage `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &tablero); err != nil {
		t.Fatalf("CA-722: hoom board --json es JSON: %v", err)
	}
	vistas := 0
	for _, col := range tablero.Columns {
		for _, rc := range col.Cards {
			spPelotaJSON(t, "CA-722", "hoom board --json (binario)", rc)
			vistas++
		}
	}
	if vistas != 11 {
		t.Fatalf("fixture: hoom board --json trae las 11 tarjetas, trajo %d", vistas)
	}
}
