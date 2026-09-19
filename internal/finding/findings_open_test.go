// Tests adversariales del spec .hoom/specs/gate-findings-open.md
// (CA-250..CA-255, CA-258): una sola definicion de "abierto" en
// internal/finding, el umbral de block_on, el alcance por tarea del spec y
// el gate findings_open que falla cerrado ante lo que no puede leer.
package finding

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/verdict"
)

var foSeveridades = []string{"low", "medium", "high"}

func foRango(s string) int {
	for i, v := range foSeveridades {
		if v == s {
			return i
		}
	}
	return -1
}

func foDir(root string) string { return filepath.Join(root, ".hoom", "findings") }

// foCrudo escribe a mano un archivo en .hoom/findings/: la forma de fabricar
// lo que el binario nunca escribiria.
func foCrudo(t *testing.T, root, nombre, contenido string) string {
	t.Helper()
	if err := os.MkdirAll(foDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(foDir(root), nombre)
	if err := os.WriteFile(p, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func foRes(t *testing.T, root, id string, campos map[string]any) string {
	t.Helper()
	raw, err := json.MarshalIndent(campos, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return foCrudo(t, root, id+".res.json", string(raw))
}

func foAlta(t *testing.T, root, sev, lens, task, desc string) Finding {
	t.Helper()
	f, err := Register(root, "main", Draft{Severity: sev, Lens: lens, File: "app.go",
		Description: desc, Author: "reviewer@test", Task: task})
	if err != nil {
		t.Fatalf("fixture: Register %s/%s: %v", sev, task, err)
	}
	return f
}

// foSpec crea .hoom/specs/<nombre> y devuelve su ruta relativa al root.
func foSpec(t *testing.T, root, nombre string) string {
	t.Helper()
	rel := filepath.Join(".hoom", "specs", nombre)
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "specs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rel), []byte("# Spec\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return rel
}

func foExigeGate(t *testing.T, ca string, g verdict.GateResult, estado, scope string) {
	t.Helper()
	if g.Name != "findings_open" {
		t.Fatalf("%s: el gate se llama findings_open, fue %q", ca, g.Name)
	}
	if !g.Required {
		t.Fatalf("%s: findings_open es required siempre: %+v", ca, g)
	}
	if g.Status != estado {
		t.Fatalf("%s: esperaba status %q, fue %q\nnotes: %s\noutput_tail: %s", ca, estado, g.Status, g.Notes, g.OutputTail)
	}
	if scope != "" && g.Scope != scope {
		t.Fatalf("%s: esperaba scope %q, fue %q", ca, scope, g.Scope)
	}
}

// ---------------------------------------------------------------------------
// Umbral y tarea del spec
// ---------------------------------------------------------------------------

// CA-252: block_on es un umbral "esa severidad o mayor": high bloquea solo
// high, medium bloquea medium y high, low bloquea todo.
func TestCA252_BlocksEsUnUmbral(t *testing.T) {
	for _, b := range foSeveridades {
		for _, s := range foSeveridades {
			quiero := foRango(s) >= foRango(b)
			if got := Blocks(s, b); got != quiero {
				t.Fatalf("CA-252: Blocks(%q, block_on %q) = %v, esperaba %v", s, b, got, quiero)
			}
		}
	}
}

type foPar struct{ Sev, Umbral int }

func (foPar) Generate(rnd *rand.Rand, size int) reflect.Value {
	return reflect.ValueOf(foPar{rnd.Intn(3), rnd.Intn(3)})
}

// CA-252 (propiedad): Blocks es reflexivo (una severidad bloquea bajo su
// propio umbral), monotono en la severidad y antitono en el umbral.
func TestCA252_PropiedadMonotonia(t *testing.T) {
	prop := func(p, q foPar) bool {
		s, b := foSeveridades[p.Sev], foSeveridades[p.Umbral]
		if !Blocks(s, s) || !Blocks("high", b) {
			return false
		}
		if Blocks(s, b) {
			// una severidad mayor o igual tambien bloquea
			if mayor := foSeveridades[q.Sev]; foRango(mayor) >= foRango(s) && !Blocks(mayor, b) {
				return false
			}
			// un umbral menor o igual tambien la bloquea
			if menor := foSeveridades[q.Umbral]; foRango(menor) <= foRango(b) && !Blocks(s, menor) {
				return false
			}
		}
		return Blocks(s, b) == (p.Sev >= p.Umbral)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("CA-252: Blocks no es un umbral monotono: %v", err)
	}
}

// CA-255: la tarea de un spec es el nombre del archivo sin .md, venga la ruta
// relativa, absoluta o con el nombre que no es un slug.
func TestCA255_TaskOfSpecEsElNombreDelArchivo(t *testing.T) {
	casos := map[string]string{
		".hoom/specs/gate-findings-open.md":             "gate-findings-open",
		"./.hoom/specs/gate-findings-open.md":           "gate-findings-open",
		"/tmp/repo/.hoom/specs/cabina-visual.md":        "cabina-visual",
		"item.md":                                       "item",
		".hoom/specs/b1.md":                             "b1",
		filepath.Join("a", "b", "c", "tarea-a.md"):      "tarea-a",
		".hoom/specs/Mi Spec.md":                        "Mi Spec",
		"../otro-arbol/.hoom/specs/arquitecto-sobre.md": "arquitecto-sobre",
	}
	for ruta, quiero := range casos {
		if got := TaskOfSpec(ruta); got != quiero {
			t.Fatalf("CA-255: TaskOfSpec(%q) = %q, esperaba %q", ruta, got, quiero)
		}
	}
}

type foRutaSpec struct {
	Dir, Slug string
}

func (foRutaSpec) Generate(rnd *rand.Rand, size int) reflect.Value {
	letras := "abcdefghijklmnopqrstuvwxyz0123456789"
	slug := string(letras[rnd.Intn(26)])
	for i, n := 0, rnd.Intn(20); i < n; i++ {
		if rnd.Intn(6) == 0 {
			slug += "-"
		} else {
			slug += string(letras[rnd.Intn(len(letras))])
		}
	}
	segs := []string{".hoom", "specs", "x.md", "otro dir", "ñandú", "a.b", "..", "."}
	var partes []string
	if rnd.Intn(2) == 0 {
		partes = append(partes, "/")
	}
	for i, n := 0, rnd.Intn(4); i < n; i++ {
		partes = append(partes, segs[rnd.Intn(len(segs))])
	}
	return reflect.ValueOf(foRutaSpec{Dir: filepath.Join(partes...), Slug: slug})
}

// CA-255 (propiedad): para cualquier directorio, la tarea es el slug del
// archivo; jamas arrastra el directorio ni la extension.
func TestCA255_PropiedadTaskOfSpec(t *testing.T) {
	prop := func(r foRutaSpec) bool {
		ruta := filepath.Join(r.Dir, r.Slug+".md")
		got := TaskOfSpec(ruta)
		return got == r.Slug && !strings.Contains(got, "/") && !strings.HasSuffix(got, ".md")
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("CA-255: TaskOfSpec no devuelve el nombre del archivo sin .md: %v", err)
	}
}

// ---------------------------------------------------------------------------
// El gate
// ---------------------------------------------------------------------------

// CA-251 (borde): sin .hoom/findings/ el gate es PASS con "0 abiertos",
// alcance de todos los hallazgos, y sin la parte "no bloquean".
func TestCA251_SinHallazgosEsPass(t *testing.T) {
	root := initRepo(t)
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-251", g, verdict.StatusPass, "full")
	if !strings.HasPrefix(g.Notes, "0 abiertos") {
		t.Fatalf("CA-251: las notas de un PASS empiezan con '0 abiertos': %q", g.Notes)
	}
	if !strings.Contains(g.Notes, "block_on: high") || !strings.Contains(g.Notes, "todos los hallazgos") {
		t.Fatalf("CA-251: las notas llevan el umbral y el alcance: %q", g.Notes)
	}
	if strings.Contains(g.Notes, "no bloquean") {
		t.Fatalf("CA-251: sin abiertos menores no hay parte 'no bloquean': %q", g.Notes)
	}
	// directorio existente pero vacio: mismo resultado
	if err := os.MkdirAll(foDir(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if g := Gate(root, "main", "high", ""); g.Status != verdict.StatusPass || !strings.HasPrefix(g.Notes, "0 abiertos") {
		t.Fatalf("CA-251: .hoom/findings vacio es PASS con 0 abiertos: %+v", g)
	}
}

// CA-250: un high abierto con block_on high es FAIL required; las notas
// llevan el id con su lente entre corchetes y el output_tail la accion exacta.
func TestCA250_HighAbiertoEsFail(t *testing.T) {
	root := initRepo(t)
	f := foAlta(t, root, "high", "risk", "", "el retry ignora el backoff exponencial")
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-250", g, verdict.StatusFail, "full")
	if !strings.HasPrefix(g.Notes, "1 abiertos que bloquean") {
		t.Fatalf("CA-250: las notas empiezan con la cuenta de los que bloquean: %q", g.Notes)
	}
	if !strings.Contains(g.Notes, "block_on: high") || !strings.Contains(g.Notes, "todos los hallazgos") {
		t.Fatalf("CA-250: las notas llevan umbral y alcance: %q", g.Notes)
	}
	if !strings.Contains(g.Notes, f.ID+" [risk]") {
		t.Fatalf("CA-250: las notas deben traer '%s [risk]': %q", f.ID, g.Notes)
	}
	if !strings.Contains(g.OutputTail, "hoom finding resolve "+f.ID) {
		t.Fatalf("CA-250: el output_tail debe traer 'hoom finding resolve %s': %q", f.ID, g.OutputTail)
	}
	if !strings.Contains(g.OutputTail, "hoom verify") {
		t.Fatalf("CA-250: la accion termina en volver a correr 'hoom verify': %q", g.OutputTail)
	}
	// una linea por hallazgo que bloquea: id, severidad, lente, archivo y descripcion
	completa := false
	for _, l := range strings.Split(g.OutputTail, "\n") {
		if !strings.Contains(l, f.ID) {
			continue
		}
		todo := true
		for _, dato := range []string{"high", "risk", "app.go", "el retry ignora"} {
			todo = todo && strings.Contains(l, dato)
		}
		completa = completa || todo
	}
	if !completa {
		t.Fatalf("CA-250: falta una linea con id, severidad, lente, archivo y descripcion de %s:\n%s", f.ID, g.OutputTail)
	}
}

// CA-250 (borde): lente vacia se rotula [sin lente]; varios que bloquean se
// cuentan y se listan todos, en notas y en output_tail.
func TestCA250_VariosYSinLente(t *testing.T) {
	root := initRepo(t)
	a := foAlta(t, root, "high", "", "", "sin lente declarada")
	b := foAlta(t, root, "high", "reliability", "", "otro high")
	foAlta(t, root, "low", "readability", "", "no bloquea")
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-250", g, verdict.StatusFail, "full")
	if !strings.HasPrefix(g.Notes, "2 abiertos que bloquean") {
		t.Fatalf("CA-250: dos high abiertos son '2 abiertos que bloquean': %q", g.Notes)
	}
	if !strings.Contains(g.Notes, a.ID+" [sin lente]") || !strings.Contains(g.Notes, b.ID+" [reliability]") {
		t.Fatalf("CA-250: cada id con su lente ([sin lente] si esta vacia): %q", g.Notes)
	}
	if !strings.Contains(g.Notes, "1 low") {
		t.Fatalf("CA-250: el FAIL tambien informa los que no bloquean: %q", g.Notes)
	}
	for _, id := range []string{a.ID, b.ID} {
		if !strings.Contains(g.OutputTail, id) {
			t.Fatalf("CA-250: output_tail debe listar %s:\n%s", id, g.OutputTail)
		}
	}
}

// CA-251: con block_on high, un medium y un low abiertos no bloquean: PASS,
// notas que empiezan con "0 abiertos" y los informan por severidad.
func TestCA251_MenoresNoBloquean(t *testing.T) {
	root := initRepo(t)
	foAlta(t, root, "medium", "risk", "", "medio")
	foAlta(t, root, "low", "readability", "", "bajo")
	cerrado := foAlta(t, root, "high", "risk", "", "ya refutado")
	if _, err := Resolve(root, cerrado.ID, "refutado", "el test TestX lo tumba", ""); err != nil {
		t.Fatal(err)
	}
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-251", g, verdict.StatusPass, "full")
	if !strings.HasPrefix(g.Notes, "0 abiertos") {
		t.Fatalf("CA-251: el PASS empieza con '0 abiertos': %q", g.Notes)
	}
	for _, s := range []string{"no bloquean", "1 medium", "1 low"} {
		if !strings.Contains(g.Notes, s) {
			t.Fatalf("CA-251: las notas deben mencionar %q: %q", s, g.Notes)
		}
	}
	if strings.Contains(g.Notes, cerrado.ID) {
		t.Fatalf("CA-251: un high refutado no aparece en las notas: %q", g.Notes)
	}
}

// CA-252: el umbral incluye las mayores. medium bloquea medium y high, no un
// low solo; low bloquea un low.
func TestCA252_UmbralEnElGate(t *testing.T) {
	casos := []struct {
		umbral string
		sevs   []string
		quiero string
	}{
		{"medium", []string{"medium"}, verdict.StatusFail},
		{"medium", []string{"high"}, verdict.StatusFail},
		{"medium", []string{"low"}, verdict.StatusPass},
		{"medium", []string{"low", "low"}, verdict.StatusPass},
		{"low", []string{"low"}, verdict.StatusFail},
		{"high", []string{"medium", "low"}, verdict.StatusPass},
		{"high", []string{"high"}, verdict.StatusFail},
	}
	for _, c := range casos {
		root := initRepo(t)
		for _, s := range c.sevs {
			foAlta(t, root, s, "risk", "", "hallazgo "+s)
		}
		g := Gate(root, "main", c.umbral, "")
		if g.Status != c.quiero {
			t.Fatalf("CA-252: block_on %s con %v debia ser %s, fue %s (%s)", c.umbral, c.sevs, c.quiero, g.Status, g.Notes)
		}
		if !strings.Contains(g.Notes, "block_on: "+c.umbral) {
			t.Fatalf("CA-252: las notas nombran el umbral %q: %q", c.umbral, g.Notes)
		}
	}
}

// CA-253: corregido o refutado con evidencia cierran; la forma de 'as' se
// normaliza (minusculas y sin espacios) igual que en resolve.
func TestCA253_ResolucionValidaCierra(t *testing.T) {
	root := initRepo(t)
	a := foAlta(t, root, "high", "risk", "", "corregido por el binario")
	if _, err := Resolve(root, a.ID, "corregido", "commit abc123 con el test que lo cubre", ""); err != nil {
		t.Fatal(err)
	}
	b := foAlta(t, root, "high", "risk", "", "refutado a mano con mayusculas")
	foRes(t, root, b.ID, map[string]any{"finding_id": b.ID, "as": " Refutado ", "evidence": "no reproduce con TestY",
		"author": "humano", "resolved_at": "2026-09-18T00:00:00Z"})

	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-253", g, verdict.StatusPass, "full")

	items, warnings, err := List(root, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range warnings {
		if strings.Contains(w, "resolucion invalida") {
			t.Fatalf("CA-253: una resolucion valida no deja aviso: %v", warnings)
		}
	}
	estados := map[string]string{}
	for _, it := range items {
		estados[it.ID] = it.Status
	}
	if estados[a.ID] != StatusCorrected || estados[b.ID] != StatusRefuted {
		t.Fatalf("CA-253: los estados derivados son corregido/refutado normalizados: %v", estados)
	}
	open, _, _ := List(root, "main", true)
	if len(open) != 0 {
		t.Fatalf("CA-253: ninguno queda abierto: %+v", open)
	}
}

// CA-253: una resolucion sin evidencia, en blanco, con 'as' inventado o
// vacio, o sin finding_id NO cierra: List la ignora con aviso que nombra el
// archivo, el hallazgo sigue abierto (tambien con openOnly) y el gate es FAIL
// con el aviso en output_tail.
func TestCA253_ResolucionInvalidaNoCierra(t *testing.T) {
	casos := map[string]func(id string) map[string]any{
		"evidencia vacia": func(id string) map[string]any {
			return map[string]any{"finding_id": id, "as": "refutado", "evidence": ""}
		},
		"evidencia en blanco": func(id string) map[string]any {
			return map[string]any{"finding_id": id, "as": "corregido", "evidence": "   \t\n "}
		},
		"sin evidencia": func(id string) map[string]any {
			return map[string]any{"finding_id": id, "as": "refutado"}
		},
		"as wontfix": func(id string) map[string]any {
			return map[string]any{"finding_id": id, "as": "wontfix", "evidence": "no lo vamos a arreglar"}
		},
		"as vacio": func(id string) map[string]any {
			return map[string]any{"finding_id": id, "as": "", "evidence": "evidencia real"}
		},
		"as abierto": func(id string) map[string]any {
			return map[string]any{"finding_id": id, "as": "abierto", "evidence": "evidencia real"}
		},
	}
	for nombre, cuerpo := range casos {
		root := initRepo(t)
		f := foAlta(t, root, "high", "risk", "", "hallazgo con resolucion "+nombre)
		archivo := f.ID + ".res.json"
		foRes(t, root, f.ID, cuerpo(f.ID))

		items, warnings, err := List(root, "main", false)
		if err != nil {
			t.Fatalf("CA-253 (%s): List no se rompe por una resolucion invalida: %v", nombre, err)
		}
		if len(items) != 1 || items[0].Status != StatusOpen {
			t.Fatalf("CA-253 (%s): el hallazgo sigue ABIERTO: %+v", nombre, items)
		}
		if items[0].Resolution != nil {
			t.Fatalf("CA-253 (%s): una resolucion invalida se ignora, no se adjunta: %+v", nombre, items[0].Resolution)
		}
		aviso := strings.Join(warnings, "\n")
		if !strings.Contains(aviso, archivo) || !strings.Contains(aviso, "resolucion invalida") ||
			!strings.Contains(aviso, "sigue abierto") {
			t.Fatalf("CA-253 (%s): el aviso debe decir 'resolucion invalida %s: ...; el hallazgo sigue abierto': %v",
				nombre, archivo, warnings)
		}
		open, _, _ := List(root, "main", true)
		if len(open) != 1 || open[0].ID != f.ID {
			t.Fatalf("CA-253 (%s): openOnly lo incluye: %+v", nombre, open)
		}

		g := Gate(root, "main", "high", "")
		foExigeGate(t, "CA-253 ("+nombre+")", g, verdict.StatusFail, "full")
		if !strings.Contains(g.Notes, f.ID) {
			t.Fatalf("CA-253 (%s): el high sigue bloqueando: %q", nombre, g.Notes)
		}
		if !strings.Contains(g.OutputTail, "resolucion invalida") || !strings.Contains(g.OutputTail, archivo) {
			t.Fatalf("CA-253 (%s): el aviso va en output_tail:\n%s", nombre, g.OutputTail)
		}
	}
}

// CA-253 (borde): /api/findings y 'finding list --json' salen de JSONBytes:
// el hallazgo con resolucion invalida figura abierto y el aviso viaja.
func TestCA253_JSONBytesVeLoMismo(t *testing.T) {
	root := initRepo(t)
	f := foAlta(t, root, "high", "risk", "", "resolucion sin evidencia")
	foRes(t, root, f.ID, map[string]any{"finding_id": f.ID, "as": "refutado", "evidence": "   "})
	raw, err := JSONBytes(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	var lv ListView
	if err := json.Unmarshal(raw, &lv); err != nil {
		t.Fatal(err)
	}
	if len(lv.Findings) != 1 || lv.Findings[0].ID != f.ID || lv.Findings[0].Status != StatusOpen {
		t.Fatalf("CA-253: --open --json debe listarlo abierto: %s", raw)
	}
	if !strings.Contains(strings.Join(lv.Warnings, "\n"), f.ID+".res.json") {
		t.Fatalf("CA-253: el aviso nombra el archivo: %s", raw)
	}
}

// CA-253 (borde): una resolucion ilegible sigue como hoy: el hallazgo abierto
// y el aviso de ilegible, tambien en el output_tail del gate.
func TestCA253_ResolucionIlegibleNoCierra(t *testing.T) {
	root := initRepo(t)
	f := foAlta(t, root, "high", "risk", "", "resolucion rota")
	foCrudo(t, root, f.ID+".res.json", `{"finding_id": "`+f.ID+`", "as": "refutado", "evidence": "x"`) // sin cerrar
	open, warnings, err := List(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	if len(open) != 1 || open[0].ID != f.ID {
		t.Fatalf("CA-253: con la resolucion ilegible el hallazgo sigue abierto: %+v", open)
	}
	if !strings.Contains(strings.Join(warnings, "\n"), f.ID+".res.json") {
		t.Fatalf("CA-253: el aviso nombra el archivo ilegible: %v", warnings)
	}
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-253", g, verdict.StatusFail, "full")
	if !strings.Contains(g.OutputTail, f.ID+".res.json") {
		t.Fatalf("CA-253: el aviso de la resolucion ilegible va en output_tail:\n%s", g.OutputTail)
	}
}

// CA-253 (borde): los avisos van en output_tail en cualquier resultado, PASS
// incluido: un low con resolucion invalida no bloquea bajo high pero se avisa.
func TestCA253_AvisoTambienEnPass(t *testing.T) {
	root := initRepo(t)
	f := foAlta(t, root, "low", "readability", "", "low mal cerrado")
	foRes(t, root, f.ID, map[string]any{"finding_id": f.ID, "as": "wontfix", "evidence": "no"})
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-253", g, verdict.StatusPass, "full")
	if !strings.Contains(g.Notes, "1 low") {
		t.Fatalf("CA-253: el low sigue abierto y se informa como que no bloquea: %q", g.Notes)
	}
	if !strings.Contains(g.OutputTail, f.ID+".res.json") || !strings.Contains(g.OutputTail, "resolucion invalida") {
		t.Fatalf("CA-253: el aviso va en output_tail aun en PASS:\n%s", g.OutputTail)
	}
}

// CA-253 (borde): que el codigo haya cambiado desde el hallazgo no lo cierra.
func TestCA253_CodigoCambiadoNoCierra(t *testing.T) {
	root := initRepo(t)
	f := foAlta(t, root, "high", "risk", "", "sigue abierto aunque cambie el arbol")
	if err := os.WriteFile(filepath.Join(root, "app.go"), []byte("package app // arreglado?\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-253", g, verdict.StatusFail, "full")
	if !strings.Contains(g.Notes, f.ID) {
		t.Fatalf("CA-253: code_changed no cierra; cerrar exige resolucion: %q", g.Notes)
	}
}

// CA-254: un archivo de hallazgo ilegible es ERROR nombrando el archivo, aun
// si hay una resolucion que parece cerrarlo: sin leer el hallazgo no se sabe
// que cierra.
func TestCA254_HallazgoIlegibleEsError(t *testing.T) {
	root := initRepo(t)
	foAlta(t, root, "low", "readability", "", "uno valido que no bloquea")
	foCrudo(t, root, "20260918T000000_abcdef.json", "{roto")
	foRes(t, root, "20260918T000000_abcdef", map[string]any{"finding_id": "20260918T000000_abcdef",
		"as": "refutado", "evidence": "parece cerrarlo"})
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-254", g, verdict.StatusError, "full")
	if !strings.Contains(g.Notes, "20260918T000000_abcdef.json") {
		t.Fatalf("CA-254: las notas nombran el archivo ilegible: %q", g.Notes)
	}
}

// CA-254: severidad fuera de low|medium|high (escrita a mano) es ERROR: no se
// degrada a high ni se ignora. Tambien sin severidad o sin id.
func TestCA254_SeveridadDesconocidaEsError(t *testing.T) {
	casos := map[string]string{
		"critical.json": `{"id": "critical", "severity": "critical", "lens": "risk", "description": "escrito a mano"}`,
		"vacia.json":    `{"id": "vacia", "severity": "", "lens": "risk", "description": "sin severidad"}`,
		"blocker.json":  `{"id": "blocker", "severity": "blocker", "lens": "risk", "description": "otra escala"}`,
		"sinid.json":    `{"severity": "high", "lens": "risk", "description": "sin id"}`,
		"lista.json":    `["no", "es", "un", "hallazgo"]`,
	}
	for nombre, contenido := range casos {
		root := initRepo(t)
		foCrudo(t, root, nombre, contenido)
		g := Gate(root, "main", "high", "")
		foExigeGate(t, "CA-254 ("+nombre+")", g, verdict.StatusError, "")
		if !strings.Contains(g.Notes, nombre) {
			t.Fatalf("CA-254 (%s): las notas nombran el archivo: %q", nombre, g.Notes)
		}
		// status y finding list rotulan sin romper
		if _, _, err := List(root, "main", false); err != nil {
			t.Fatalf("CA-254 (%s): List informa, no se rompe: %v", nombre, err)
		}
	}
}

// CA-254: con varios archivos rotos el ERROR los nombra a todos, y gana
// sobre un high que bloquea (fail-closed: el veredicto no se degrada a FAIL).
func TestCA254_VariosRotosYUnHigh(t *testing.T) {
	root := initRepo(t)
	foAlta(t, root, "high", "risk", "", "high real")
	foCrudo(t, root, "uno.json", "{")
	foCrudo(t, root, "dos.json", `{"id": "dos", "severity": "critical", "description": "x"}`)
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-254", g, verdict.StatusError, "")
	for _, n := range []string{"uno.json", "dos.json"} {
		if !strings.Contains(g.Notes, n) {
			t.Fatalf("CA-254: las notas nombran cada archivo (%s): %q", n, g.Notes)
		}
	}
}

// CA-254 (borde): el ERROR no depende del alcance: con --spec de otra tarea
// un archivo ilegible igual es ERROR, porque no se sabe de que tarea es.
func TestCA254_IlegibleTambienConSpec(t *testing.T) {
	root := initRepo(t)
	spec := foSpec(t, root, "tarea-a.md")
	foCrudo(t, root, "roto.json", "no es json")
	g := Gate(root, "main", "high", spec)
	foExigeGate(t, "CA-254", g, verdict.StatusError, "spec")
	if !strings.Contains(g.Notes, "roto.json") {
		t.Fatalf("CA-254: las notas nombran el archivo: %q", g.Notes)
	}
}

// CA-255: con --spec cuentan los de la tarea del spec y los sin tarea; un high
// de otra tarea no bloquea y la nota dice cuantos quedaron afuera. Scope spec.
// Sin spec el mismo high bloquea con scope full.
func TestCA255_AlcancePorTarea(t *testing.T) {
	root := initRepo(t)
	specA := foSpec(t, root, "tarea-a.md")
	ajeno := foAlta(t, root, "high", "risk", "tarea-b", "high de otra tarea")

	g := Gate(root, "main", "high", specA)
	foExigeGate(t, "CA-255", g, verdict.StatusPass, "spec")
	if !strings.HasPrefix(g.Notes, "0 abiertos") {
		t.Fatalf("CA-255: con otra tarea el PASS empieza con '0 abiertos': %q", g.Notes)
	}
	if !strings.Contains(g.Notes, "tarea tarea-a + sin tarea") {
		t.Fatalf("CA-255: el alcance es 'tarea tarea-a + sin tarea': %q", g.Notes)
	}
	if !strings.Contains(g.Notes, "1 de otras tareas no cuentan") {
		t.Fatalf("CA-255: la nota dice '1 de otras tareas no cuentan': %q", g.Notes)
	}
	if strings.Contains(g.Notes, ajeno.ID) {
		t.Fatalf("CA-255: el de otra tarea no se lista como bloqueante: %q", g.Notes)
	}

	full := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-255", full, verdict.StatusFail, "full")
	if !strings.Contains(full.Notes, ajeno.ID) || !strings.Contains(full.Notes, "todos los hallazgos") {
		t.Fatalf("CA-255: sin spec el mismo high bloquea con alcance 'todos los hallazgos': %q", full.Notes)
	}

	// la ruta absoluta del spec da la misma tarea
	abs := Gate(root, "main", "high", filepath.Join(root, specA))
	if abs.Status != verdict.StatusPass || abs.Scope != "spec" {
		t.Fatalf("CA-255: la ruta absoluta del spec es la misma tarea: %+v", abs)
	}
}

// CA-255: los de la tarea del spec y los sin tarea SI cuentan.
func TestCA255_PropiosYSinTareaCuentan(t *testing.T) {
	for _, tarea := range []string{"tarea-a", ""} {
		root := initRepo(t)
		specA := foSpec(t, root, "tarea-a.md")
		f := foAlta(t, root, "high", "risk", tarea, "high que cuenta")
		foAlta(t, root, "high", "risk", "tarea-b", "high ajeno")
		g := Gate(root, "main", "high", specA)
		foExigeGate(t, "CA-255 (task="+tarea+")", g, verdict.StatusFail, "spec")
		if !strings.HasPrefix(g.Notes, "1 abiertos que bloquean") || !strings.Contains(g.Notes, f.ID) {
			t.Fatalf("CA-255 (task=%q): cuenta exactamente el propio/sin tarea: %q", tarea, g.Notes)
		}
		if !strings.Contains(g.Notes, "1 de otras tareas no cuentan") {
			t.Fatalf("CA-255 (task=%q): informa el ajeno que quedo fuera: %q", tarea, g.Notes)
		}
	}
}

// CA-255 (borde): un spec cuyo nombre no es slug (mayusculas, espacios) no
// casa con ninguna tarea: la comparacion es exacta, sin plegar mayusculas.
// Cuentan solo los sin tarea y la nota muestra el alcance.
func TestCA255_SpecConNombreNoSlug(t *testing.T) {
	for _, nombre := range []string{"Mi-Spec.md", "mi spec.md", "MI-SPEC.md"} {
		root := initRepo(t)
		spec := foSpec(t, root, nombre)
		foAlta(t, root, "high", "risk", "mi-spec", "high de la tarea con forma de slug")
		g := Gate(root, "main", "high", spec)
		foExigeGate(t, "CA-255 ("+nombre+")", g, verdict.StatusPass, "spec")
		if !strings.Contains(g.Notes, "sin tarea") || !strings.Contains(g.Notes, "1 de otras tareas no cuentan") {
			t.Fatalf("CA-255 (%s): la nota muestra el alcance real y el que quedo fuera: %q", nombre, g.Notes)
		}

		sin := foAlta(t, root, "high", "risk", "", "high sin tarea")
		g = Gate(root, "main", "high", spec)
		foExigeGate(t, "CA-255 ("+nombre+")", g, verdict.StatusFail, "spec")
		if !strings.Contains(g.Notes, sin.ID) {
			t.Fatalf("CA-255 (%s): el sin tarea cuenta: %q", nombre, g.Notes)
		}
	}
}

// ---------------------------------------------------------------------------
// resolve no pisa una resolucion invalida
// ---------------------------------------------------------------------------

// CA-258: 'hoom finding resolve' sobre un hallazgo con .res.json invalido se
// niega diciendo que sigue ABIERTO y nombrando el archivo; el .res.json queda
// byte a byte igual.
func TestCA258_ResolveNoPisaUnaResolucionInvalida(t *testing.T) {
	casos := map[string]func(id string) string{
		"evidencia en blanco": func(id string) string {
			return `{"finding_id": "` + id + `", "as": "refutado", "evidence": "   "}`
		},
		"as wontfix": func(id string) string {
			return `{"finding_id": "` + id + `", "as": "wontfix", "evidence": "x"}`
		},
		"ilegible": func(id string) string {
			return `{"finding_id": "` + id + `", "as": `
		},
		"sin finding_id": func(id string) string {
			return `{"as": "refutado", "evidence": "tiene evidencia pero no dice de quien"}`
		},
	}
	for nombre, cuerpo := range casos {
		root := initRepo(t)
		f := foAlta(t, root, "high", "risk", "", "hallazgo "+nombre)
		archivo := f.ID + ".res.json"
		ruta := foCrudo(t, root, archivo, cuerpo(f.ID))
		antes, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatal(err)
		}

		_, err = Resolve(root, f.ID, "refutado", "evidencia legitima que quiere cerrar", "")
		if err == nil {
			t.Fatalf("CA-258 (%s): resolve debe negarse sobre una resolucion invalida", nombre)
		}
		if !strings.Contains(err.Error(), "ABIERTO") || !strings.Contains(err.Error(), archivo) {
			t.Fatalf("CA-258 (%s): el error dice que sigue ABIERTO y nombra %s: %v", nombre, archivo, err)
		}
		if strings.Contains(err.Error(), "ya esta resuelto") {
			t.Fatalf("CA-258 (%s): no puede decir que ya esta resuelto, porque no lo esta: %v", nombre, err)
		}
		despues, err := os.ReadFile(ruta)
		if err != nil {
			t.Fatalf("CA-258 (%s): el .res.json no puede desaparecer: %v", nombre, err)
		}
		if string(antes) != string(despues) {
			t.Fatalf("CA-258 (%s): el .res.json cambio\n  antes: %q\n  ahora: %q", nombre, antes, despues)
		}
		if open, _, _ := List(root, "main", true); len(open) != 1 {
			t.Fatalf("CA-258 (%s): tras la negativa el hallazgo sigue abierto: %+v", nombre, open)
		}
	}
}
