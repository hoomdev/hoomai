// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md en
// internal/tablero (CA-730): las dos pieles dicen lo mismo. Para cada
// tarjeta de un tablero sin un piloto vivo lanzado por el tablero,
// Modelo.Grupo(slug) es su pelota.grupo, y Modelo.Caja() es Board.pelota.caja.
//
// Los fixtures de tt_fixtures_test.go son Card armadas a mano: no pasan por
// boardcmd y no traen pelota. Por eso las tarjetas de este archivo salen de
// boardcmd.Build sobre proyectos en disco (repos git en t.TempDir(), sin
// ninguna CLI de IA), y la pelota se lee del JSON (lo que emite hoom board
// --json), no de campos Go. Escritos desde los specs y las firmas publicas,
// sin leer la implementacion.
package tablero_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// Las formas de una tarjeta en disco y el grupo que les da el contrato.
const (
	spCola       = "cola"       // item sin spec: Backlog, EN COLA
	spArquitecto = "arquitecto" // spec sin la seccion riesgos: Arquitecto, TE TOCA
	spFirma      = "firma"      // spec que pasa el lint, sin aprobar: Tu aprobacion, TE TOCA
	spTests      = "tests"      // spec aprobado con un criterio sin test: Test-writer, TE TOCA
	spCorre      = "corre"      // un run suelto vivo (este proceso): CORRIENDO
	spCortada    = "cortada"    // un sobre abierto cuyo dueno murio: DETENIDA
	spRoja       = "roja"       // un sobre cerrado no entregable: DETENIDA
	spHecha      = "hecha"      // item con hecho_en: HECHO
)

var spGrupoDe = map[string]tablero.Grupo{spCola: tablero.GrupoEnCola, spArquitecto: tablero.GrupoTeToca,
	spFirma: tablero.GrupoTeToca, spTests: tablero.GrupoTeToca, spCorre: tablero.GrupoCorriendo,
	spCortada: tablero.GrupoDetenida, spRoja: tablero.GrupoDetenida, spHecha: tablero.GrupoHecho}

var spFormas = []string{spCola, spArquitecto, spFirma, spTests, spCorre, spCortada, spRoja, spHecha}

type spDisco struct {
	t      *testing.T
	root   string
	now    time.Time
	n      int
	muerto int
}

// spPIDMuerto es el PID de un proceso que ya termino.
func spPIDMuerto(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	return cmd.ProcessState.Pid()
}

// spNuevoDisco: proyecto hoom en disco (como ttProyecto) sin items.
func spNuevoDisco(t *testing.T) *spDisco {
	t.Helper()
	env := gittest.EntornoSolo(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	ttGit(t, env, root, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	ttGit(t, env, root, "config", "user.email", "test@hoom.dev")
	ttGit(t, env, root, "config", "user.name", "hoom test")
	ttArchivo(t, root, "hoom.yaml", ttYAML)
	ttArchivo(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	ttArchivo(t, root, "app.go", "package app\n")
	ttGit(t, env, root, "add", "-A")
	ttGit(t, env, root, "commit", "-q", "-m", "inicial")
	return &spDisco{t: t, root: root, now: time.Now().UTC().Truncate(time.Second), muerto: spPIDMuerto(t)}
}

func (d *spDisco) id(pre string) string {
	d.n++
	return fmt.Sprintf("20261117T%02d%02d%02d_%s%04d", 10+d.n/3600%10, d.n/60%60, d.n%60, pre, d.n)
}

func spSpecTexto(conRiesgos bool) string {
	s := "# Spec: precios\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n- CA-1: algo.\n\n## Decisiones\nx\n"
	if conRiesgos {
		s += "\n## Riesgos\nx\n"
	}
	return s
}

func (d *spDisco) json(rel string, v any) {
	d.t.Helper()
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		d.t.Fatal(err)
	}
	ttArchivo(d.t, d.root, rel, string(raw)+"\n")
}

// tarjeta escribe en disco el item slug con la forma pedida.
func (d *spDisco) tarjeta(slug, forma string) {
	t := d.t
	t.Helper()
	extra := ""
	if forma == spHecha {
		extra = "hecho_en: 2026-09-23T10:00:00Z\n"
	}
	ttArchivo(t, d.root, ".hoom/items/"+slug+".yaml", "titulo: Tarjeta "+slug+"\ntipo: feature\nprioridad: media\n"+
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n"+extra)
	spec := ".hoom/specs/" + slug + ".md"
	switch forma {
	case spArquitecto:
		ttArchivo(t, d.root, spec, spSpecTexto(false))
	case spFirma:
		ttArchivo(t, d.root, spec, spSpecTexto(true))
	case spTests:
		ttArchivo(t, d.root, spec, spSpecTexto(true))
		if _, _, err := approval.Approve(d.root, filepath.Join(d.root, spec)); err != nil {
			t.Fatalf("fixture: no pude aprobar el spec de %s: %v", slug, err)
		}
	case spCorre:
		id := d.id("rn")
		d.json(".hoom/runs/"+id+".meta.json", runcmd.Meta{ID: id, Provider: "claude", Role: "writer", Task: slug, Dir: d.root,
			CreatedAt: d.now.Add(-10 * time.Minute), Status: runcmd.StatusRunning, ExitCode: -1, PID: os.Getpid()})
	case spCortada:
		run, sobre := d.id("rn"), d.id("sb")
		d.json(filepath.Join(".hoom", envelope.DirName, sobre+".json"), envelope.Record{ID: sobre, Role: "writer",
			Provider: "claude", Task: slug, Dir: d.root, RunID: run, Stage: "run", Step: 3, Steps: 5,
			Status: envelope.StatusRunning, StartedAt: d.now.Add(-30 * time.Minute), UpdatedAt: d.now.Add(-time.Minute)})
		d.json(".hoom/runs/"+run+".meta.json", runcmd.Meta{ID: run, Provider: "claude", Role: "writer", Task: slug, Dir: d.root,
			CreatedAt: d.now.Add(-30 * time.Minute), Status: runcmd.StatusRunning, ExitCode: -1, PID: d.muerto})
	case spRoja:
		sobre, fin := d.id("sb"), d.now.Add(-35*time.Minute)
		d.json(filepath.Join(".hoom", envelope.DirName, sobre+".json"), envelope.Record{ID: sobre, Role: "writer",
			Provider: "claude", Task: slug, Dir: d.root, Stage: "run", Step: 3, Steps: 5,
			Status: envelope.StatusNotDeliverable, ExitCode: 1, Note: "se corto el agente",
			StartedAt: fin.Add(-5 * time.Minute), UpdatedAt: fin, EndedAt: fin})
	}
}

// roto escribe un item invalido: va a warnings, no es tarjeta.
func (d *spDisco) roto(slug string) {
	ttArchivo(d.t, d.root, ".hoom/items/"+slug+".yaml", "titulo: Roto\ntipo: feature\nprioridad: media\n"+
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\ncolumna: hecho\n")
}

func (d *spDisco) build() boardcmd.Board {
	d.t.Helper()
	b, err := boardcmd.Build(d.root, "main", "high", d.now)
	if err != nil {
		d.t.Fatalf("Build: %v", err)
	}
	return b
}

// spGrupoDeLaPelota lee pelota.grupo del JSON de la tarjeta.
func spGrupoDeLaPelota(t *testing.T, nombre string, c boardcmd.Card) string {
	t.Helper()
	raw, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	p, ok := m["pelota"]
	if !ok {
		var claves []string
		for k := range m {
			claves = append(claves, k)
		}
		sort.Strings(claves)
		t.Fatalf("CA-730: [%s] la tarjeta %s trae \"pelota\" en su JSON (CA-722); no la trae. Claves: %v", nombre, c.Slug, claves)
	}
	var pel struct {
		Grupo *string `json:"grupo"`
	}
	if err := json.Unmarshal(p, &pel); err != nil || pel.Grupo == nil {
		t.Fatalf("CA-730: [%s] la pelota de %s trae grupo (string): %s (%v)", nombre, c.Slug, p, err)
	}
	return *pel.Grupo
}

// spCajaDeLaPelota lee Board.pelota.caja de lo que emite hoom board --json.
func spCajaDeLaPelota(t *testing.T, nombre string, b boardcmd.Board) string {
	t.Helper()
	raw, err := boardcmd.JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	p, ok := m["pelota"]
	if !ok {
		t.Fatalf("CA-730: [%s] el tablero trae \"pelota\" en su JSON (Board.pelota); no la trae", nombre)
	}
	var pel struct {
		Caja *string `json:"caja"`
	}
	if err := json.Unmarshal(p, &pel); err != nil || pel.Caja == nil {
		t.Fatalf("CA-730: [%s] Board.pelota trae caja (string): %s (%v)", nombre, p, err)
	}
	return *pel.Caja
}

// spLasDosPieles es el contrato de CA-730 sobre un tablero de Build: primero
// exige el fixture (el grupo del tablero de hoy es el de la forma), despues
// compara cada tarjeta y la caja con la pelota.
func spLasDosPieles(t *testing.T, nombre string, d *spDisco, formas map[string]string) {
	t.Helper()
	b := d.build()
	m := ttModelo(ttAncho(), b)
	var cards []boardcmd.Card
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			cards = append(cards, c)
		}
	}
	if len(cards) != len(formas) {
		t.Fatalf("fixture [%s]: el tablero tiene %d tarjetas, se esperaban %d", nombre, len(cards), len(formas))
	}
	for _, c := range cards {
		forma, ok := formas[c.Slug]
		if !ok {
			t.Fatalf("fixture [%s]: tarjeta inesperada %s", nombre, c.Slug)
		}
		if g := m.Grupo(c.Slug); g != spGrupoDe[forma] {
			t.Fatalf("fixture [%s]: %s (%s) cae en %q en el tablero, se esperaba %q (columna %s, running=%v, interrupted=%v, red=%+v)",
				nombre, c.Slug, forma, g, spGrupoDe[forma], c.Column, c.Running != nil, c.Interrupted != nil, c.Red)
		}
	}
	for _, c := range cards {
		if g, p := m.Grupo(c.Slug), spGrupoDeLaPelota(t, nombre, c); string(g) != p {
			t.Fatalf("CA-730: [%s] Modelo.Grupo(%s) = %q y su pelota.grupo = %q: son iguales", nombre, c.Slug, g, p)
		}
	}
	if caja, p := m.Caja(), spCajaDeLaPelota(t, nombre, b); caja != p {
		t.Fatalf("CA-730: [%s] Modelo.Caja() = %q y Board.pelota.caja = %q: son iguales", nombre, caja, p)
	}
}

func spConFormas(t *testing.T, formas map[string]string, conRoto bool) *spDisco {
	t.Helper()
	d := spNuevoDisco(t)
	for slug, forma := range formas {
		d.tarjeta(slug, forma)
	}
	if conRoto {
		d.roto("roto")
	}
	return d
}

// CA-730: un tablero con tarjetas en los cinco grupos (y un item roto en
// warnings): cada Modelo.Grupo es la pelota.grupo de su tarjeta y la caja es
// Board.pelota.caja.
func TestCA730_GrupoYCajaComoLaPelotaEnLosCincoGrupos(t *testing.T) {
	formas := map[string]string{"pedido-a": spCola, "pedido-b": spCola, "arq-uno": spArquitecto, "arq-dos": spArquitecto,
		"firma": spFirma, "tests": spTests, "corre": spCorre, "cortada": spCortada, "roja": spRoja, "lista": spHecha}
	spLasDosPieles(t, "cinco grupos", spConFormas(t, formas, true), formas)
}

// CA-730: sin TE TOCA la caja de las dos pieles es la misma (la primera de
// DETENIDA, por el contrato de Board.pelota.caja).
func TestCA730_CajaSinTeToca(t *testing.T) {
	formas := map[string]string{"pedido": spCola, "cortada": spCortada, "roja": spRoja, "corre": spCorre, "lista": spHecha}
	spLasDosPieles(t, "sin te toca", spConFormas(t, formas, false), formas)
}

// CA-730: sin TE TOCA ni DETENIDA las dos cajas son "".
func TestCA730_CajaVaciaSinTeTocaNiDetenida(t *testing.T) {
	formas := map[string]string{"pedido": spCola, "corre": spCorre, "lista": spHecha}
	d := spConFormas(t, formas, false)
	spLasDosPieles(t, "sin te toca ni detenida", d, formas)
	if m := ttModelo(ttAncho(), d.build()); m.Caja() != "" {
		t.Fatalf("CA-730: sin TE TOCA ni DETENIDA la caja del tablero es \"\": %q", m.Caja())
	}
}

// CA-730: un tablero vacio (con y sin un item roto): las dos cajas son "".
func TestCA730_TableroVacio(t *testing.T) {
	for _, conRoto := range []bool{false, true} {
		nombre := "vacio"
		if conRoto {
			nombre = "solo un item roto"
		}
		d := spConFormas(t, map[string]string{}, conRoto)
		spLasDosPieles(t, nombre, d, map[string]string{})
		if p := spCajaDeLaPelota(t, nombre, d.build()); p != "" {
			t.Fatalf("CA-730: [%s] un tablero vacio tiene Board.pelota.caja \"\": %q", nombre, p)
		}
	}
}

// CA-730 (propiedad, testing/quick): para cualquier mezcla de formas en
// disco, las dos pieles dan el mismo grupo para cada tarjeta y la misma caja.
func TestCA730_PropiedadLasDosPielesDicenLoMismo(t *testing.T) {
	prop := func(cuantas [8]uint8, roto bool) bool {
		formas := map[string]string{}
		for i, forma := range spFormas {
			for k := 0; k < int(cuantas[i]%3); k++ {
				formas[fmt.Sprintf("%s-%d", forma, k)] = forma
			}
		}
		spLasDosPieles(t, "propiedad", spConFormas(t, formas, roto), formas)
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 5, Rand: rand.New(rand.NewSource(730))}); err != nil {
		t.Fatalf("CA-730: %v", err)
	}
}
