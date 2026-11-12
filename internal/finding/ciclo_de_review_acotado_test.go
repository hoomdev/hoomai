// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// internal/finding: la unica definicion de que bloquea (Bloquea, CA-623,
// CA-624), la subida a mano (Raise, CA-625, CA-626), el gate con el motivo
// (CA-627) y la lista juzgada (CA-628). Escritos desde el spec y las firmas
// publicas, sin leer la implementacion. Lo que necesita registros de review
// (reviewcmd importa finding) esta en ciclo_de_review_acotado_ext_test.go.
package finding

import (
	"encoding/json"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

func crF(id, sev, file, task string) Finding {
	return Finding{ID: id, Severity: sev, File: file, Task: task, Lens: "risk", Description: "d"}
}

func crBloquea(t *testing.T, ca string, f Finding, c Contexto, bloquea bool, motivo string) {
	t.Helper()
	b, m := Bloquea(f, c)
	if b != bloquea || m != motivo {
		t.Fatalf("%s: Bloquea(%+v, %+v) = (%v, %q), quiero (%v, %q)", ca, f, c, b, m, bloquea, motivo)
	}
}

// CA-623: el caso 1 (la severidad llega al umbral), el caso 3 (medium en
// ruta de riesgo) y la severidad desconocida bloquean; un medium fuera de
// ruta de riesgo, sin file, y un low no.
func TestCA623_BloqueaSeveridadYRuta(t *testing.T) {
	for _, umbral := range []string{"high", ""} { // "" juzga con high
		c := Contexto{BlockOn: umbral}
		crBloquea(t, "CA-623", crF("a", "high", "app.go", ""), c, true, "severidad high")
		crBloquea(t, "CA-623", crF("a", "medium", "internal/auth/login.go", ""), c, true, "medium en ruta de riesgo: internal/auth/login.go")
		crBloquea(t, "CA-623", crF("a", "medium", "app/Pagos/Factura.php", "t"), c, true, "medium en ruta de riesgo: app/Pagos/Factura.php")
		crBloquea(t, "CA-623", crF("a", "medium", "app.go", ""), c, false, "")
		crBloquea(t, "CA-623", crF("a", "medium", "", ""), c, false, "") // caso limite: medium sin file
		crBloquea(t, "CA-623", crF("a", "low", "app.go", ""), c, false, "")
		crBloquea(t, "CA-623", crF("a", "low", "internal/auth/login.go", ""), c, false, "") // el caso 3 es solo de medium
		crBloquea(t, "CA-623", crF("a", "critical", "app.go", ""), c, true, `severidad desconocida "critical"`)
		crBloquea(t, "CA-623", crF("a", "", "app.go", ""), c, true, `severidad desconocida ""`)
		crBloquea(t, "CA-623", crF("a", "urgent", "app.go", ""), c, true, `severidad desconocida "urgent"`)
		// "High" o " high " estan en el vocabulario: hoom compara las
		// severidades sin distinguir mayusculas ni espacios, como hoy; bloquea
		// por el caso 1 (sin fijar la forma de <sev> en el texto)
		for _, sev := range []string{"High", " high ", "HIGH"} {
			b, m := Bloquea(crF("a", sev, "app.go", ""), c)
			if !b || !strings.HasPrefix(m, "severidad ") || strings.Contains(m, "desconocida") || !strings.EqualFold(strings.TrimSpace(strings.TrimPrefix(m, "severidad ")), "high") {
				t.Fatalf("CA-623: %q es high: bloquea con `severidad high`: (%v, %q)", sev, b, m)
			}
		}
		// "Medium" fuera de ruta de riesgo es un medium: no bloquea con high
		crBloquea(t, "CA-623", crF("a", "Medium", "app.go", ""), c, false, "")
	}
}

// CA-623 (caso limite: block_on medium): los medium ya bloquean por el caso 1
// y los casos 3 y 4 no agregan nada; con low, todo bloquea por severidad.
func TestCA623_BlockOnMediumYLow(t *testing.T) {
	c := Contexto{BlockOn: "medium", Riesgos: map[string]string{"t": "alto"},
		Completas: []RevisionCompleta{{ID: "r1", Task: "t", Findings: []string{"a"}}},
		Subidas:   map[string]Subida{"a": {FindingID: "a", Reason: "x", Author: "Ana"}}}
	crBloquea(t, "CA-623", crF("a", "medium", "internal/auth/x.go", "t"), c, true, "severidad medium")
	crBloquea(t, "CA-623", crF("a", "high", "x.go", "t"), c, true, "severidad high")
	crBloquea(t, "CA-623", crF("b", "low", "x.go", "t"), c, false, "")
	c.BlockOn = "low"
	crBloquea(t, "CA-623", crF("b", "low", "x.go", "t"), c, true, "severidad low")
	crBloquea(t, "CA-623", crF("a", "low", "x.go", "t"), c, true, "severidad low") // el caso 1 va antes que la subida
}

// CA-624: con riesgo alto, un medium que figura en una completa de su tarea
// bloquea; con normal o bajo, no; una completa de otra tarea no cuenta.
func TestCA624_MediumDeLaCompletaConRiesgoAlto(t *testing.T) {
	compl := []RevisionCompleta{
		{ID: "r-otra", Task: "otra", Findings: []string{"m1"}},
		{ID: "r1", Task: "t", Findings: []string{"x", "m1"}},
		{ID: "r2", Task: "t", Findings: []string{"m1"}},
	}
	c := Contexto{BlockOn: "high", Riesgos: map[string]string{"t": "alto", "otra": "alto"}, Completas: compl}
	crBloquea(t, "CA-624", crF("m1", "medium", "app.go", "t"), c, true, "medium de la review completa r1: riesgo alto")
	// uno que no figura en ninguna completa (figura solo en un delta: Completas
	// nunca trae deltas) no bloquea
	crBloquea(t, "CA-624", crF("m2", "medium", "app.go", "t"), c, false, "")
	// un low de la completa no bloquea por el caso 4
	c.Completas = append(c.Completas, RevisionCompleta{ID: "r3", Task: "t", Findings: []string{"l1"}})
	crBloquea(t, "CA-624", crF("l1", "low", "app.go", "t"), c, false, "")
	for _, r := range []string{"normal", "bajo", ""} {
		c.Riesgos = map[string]string{"t": r}
		crBloquea(t, "CA-624", crF("m1", "medium", "app.go", "t"), c, false, "")
	}
	// sin item (la tarea no esta en Riesgos) se juzga normal
	c.Riesgos = map[string]string{}
	crBloquea(t, "CA-624", crF("m1", "medium", "app.go", "t"), c, false, "")
	// la completa de OTRA tarea no hace bloquear el medium de t
	c.Riesgos = map[string]string{"t": "alto"}
	c.Completas = []RevisionCompleta{{ID: "r-otra", Task: "otra", Findings: []string{"m1"}}}
	crBloquea(t, "CA-624", crF("m1", "medium", "app.go", "t"), c, false, "")
	// un hallazgo sin tarea se juzga normal aunque una completa sin tarea lo nombre
	c.Riesgos = map[string]string{"": "alto"}
	c.Completas = []RevisionCompleta{{ID: "r0", Task: "", Findings: []string{"m1"}}}
	crBloquea(t, "CA-624", crF("m1", "medium", "app.go", ""), c, false, "")
}

// CA-626 (caso 2): una subida valida hace bloquear un low y un medium, con
// el autor de la subida; la severidad va antes (caso 1) y la subida antes
// que la ruta de riesgo (caso 3) y que el riesgo alto (caso 4).
func TestCA626_BloqueaPorSubida(t *testing.T) {
	sub := map[string]Subida{"a": {FindingID: "a", Reason: "lo vi en produccion", Author: "Ana <ana@hoom.dev>"}}
	c := Contexto{BlockOn: "high", Subidas: sub}
	crBloquea(t, "CA-626", crF("a", "low", "app.go", ""), c, true, "subido a mano por Ana <ana@hoom.dev>")
	crBloquea(t, "CA-626", crF("a", "medium", "internal/auth/x.go", ""), c, true, "subido a mano por Ana <ana@hoom.dev>")
	crBloquea(t, "CA-626", crF("a", "high", "app.go", ""), c, true, "severidad high")
	crBloquea(t, "CA-626", crF("b", "low", "app.go", ""), c, false, "")
	c.Riesgos = map[string]string{"t": "alto"}
	c.Completas = []RevisionCompleta{{ID: "r1", Task: "t", Findings: []string{"a"}}}
	crBloquea(t, "CA-626", crF("a", "medium", "app.go", "t"), c, true, "subido a mano por Ana <ana@hoom.dev>")
	// una severidad desconocida bloquea por eso, con o sin subida
	crBloquea(t, "CA-626", crF("a", "urgent", "app.go", ""), c, true, `severidad desconocida "urgent"`)
}

type crGen struct {
	F Finding
	C Contexto
}

func (crGen) Generate(r *rand.Rand, _ int) reflect.Value {
	sevs := []string{"low", "medium", "high", "critical", ""}
	files := []string{"", "app.go", "internal/auth/x.go", "docs/DESIGN.md", "ñ/árbol.go"}
	tasks := []string{"", "t", "u"}
	riesgos := []string{"", "bajo", "normal", "alto"}
	umbrales := []string{"", "low", "medium", "high"}
	ids := []string{"a", "b", "c"}
	f := Finding{ID: ids[r.Intn(3)], Severity: sevs[r.Intn(len(sevs))], File: files[r.Intn(len(files))], Task: tasks[r.Intn(3)]}
	c := Contexto{BlockOn: umbrales[r.Intn(4)], Riesgos: map[string]string{}, Subidas: map[string]Subida{}}
	for _, tk := range tasks {
		if r.Intn(2) == 0 {
			c.Riesgos[tk] = riesgos[r.Intn(4)]
		}
	}
	for _, id := range ids {
		if r.Intn(3) == 0 {
			c.Subidas[id] = Subida{FindingID: id, Reason: "r", Author: "P " + id}
		}
	}
	for i := r.Intn(3); i > 0; i-- {
		var fs []string
		for _, id := range ids {
			if r.Intn(2) == 0 {
				fs = append(fs, id)
			}
		}
		c.Completas = append(c.Completas, RevisionCompleta{ID: "r" + string(rune('0'+i)), Task: tasks[r.Intn(3)], Findings: fs})
	}
	return reflect.ValueOf(crGen{F: f, C: c})
}

// CA-623/CA-624/CA-626 (propiedad): bloquea si y solo si hay motivo; un high
// siempre bloquea; agregar una subida nunca hace que deje de bloquear;
// subir el umbral (bajar block_on) nunca hace que deje de bloquear; un low
// sin subida y con umbral high nunca bloquea; el resultado es determinista.
func TestCA623_PropiedadBloquea(t *testing.T) {
	prop := func(g crGen) bool {
		b, m := Bloquea(g.F, g.C)
		if b != (m != "") {
			return false
		}
		if g.F.Severity == "high" && (!b || m != "severidad high") {
			return false
		}
		b2, m2 := Bloquea(g.F, g.C)
		if b2 != b || m2 != m {
			return false
		}
		con := g.C
		con.Subidas = map[string]Subida{}
		for k, v := range g.C.Subidas {
			con.Subidas[k] = v
		}
		con.Subidas[g.F.ID] = Subida{FindingID: g.F.ID, Reason: "r", Author: "Otro"}
		if b && !func() bool { x, _ := Bloquea(g.F, con); return x }() {
			return false
		}
		if bb, _ := Bloquea(g.F, con); !bb {
			return false // con una subida valida, todo bloquea
		}
		baja := g.C
		baja.BlockOn = "low"
		if b && !func() bool { x, _ := Bloquea(g.F, baja); return x }() {
			return false
		}
		if g.F.Severity == "low" && (g.C.BlockOn == "high" || g.C.BlockOn == "" || g.C.BlockOn == "medium") {
			if _, sub := g.C.Subidas[g.F.ID]; !sub && b {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 3000}); err != nil {
		t.Fatalf("CA-623: %v", err)
	}
}

// CA-628: Juzgar llena Bloquea y Motivo de los abiertos y deja false y "" en
// los cerrados.
func TestCA628_JuzgarCerradosNoBloquean(t *testing.T) {
	items := []Item{
		{Finding: crF("a", "high", "app.go", ""), Status: StatusOpen},
		{Finding: crF("b", "high", "app.go", ""), Status: StatusRefuted},
		{Finding: crF("c", "medium", "internal/auth/x.go", ""), Status: StatusCorrected},
		{Finding: crF("d", "medium", "internal/auth/x.go", ""), Status: StatusOpen},
		{Finding: crF("e", "low", "app.go", ""), Status: StatusOpen},
	}
	got := Juzgar(items, Contexto{BlockOn: "high"})
	want := []struct {
		b bool
		m string
	}{{true, "severidad high"}, {false, ""}, {false, ""}, {true, "medium en ruta de riesgo: internal/auth/x.go"}, {false, ""}}
	if len(got) != len(want) {
		t.Fatalf("CA-628: %+v", got)
	}
	for i, w := range want {
		if got[i].Bloquea != w.b || got[i].Motivo != w.m {
			t.Fatalf("CA-628: %s: (%v, %q), quiero (%v, %q)", got[i].ID, got[i].Bloquea, got[i].Motivo, w.b, w.m)
		}
	}
}

// ---------------------------------------------------------------- subidas en disco

func crFuera(t *testing.T) {
	t.Helper()
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER"} {
		t.Setenv(k, "")
		_ = os.Unsetenv(k)
	}
}

func crAlta(t *testing.T, root, sev, file, task string) Finding {
	t.Helper()
	f, err := Register(root, "main", Draft{Severity: sev, Lens: "risk", File: file, Description: "hallazgo " + sev, Author: "reviewer@test", Task: task})
	if err != nil {
		t.Fatalf("fixture: Register: %v", err)
	}
	return f
}

func crRaisePath(root, id string) string {
	return filepath.Join(root, ".hoom", "findings", id+".raise.json")
}

func crFoto(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	es, _ := os.ReadDir(foDir(root))
	for _, e := range es {
		raw, _ := os.ReadFile(filepath.Join(foDir(root), e.Name()))
		out[e.Name()] = string(raw)
	}
	return out
}

// CA-625: Raise escribe .hoom/findings/<id>.raise.json con finding_id,
// reason, author y raised_at; dos veces no reescribe.
func TestCA625_RaiseEscribeLaSubida(t *testing.T) {
	crFuera(t)
	root := initRepo(t)
	f := crAlta(t, root, "low", "app.go", "t")
	antes := time.Now().UTC().Add(-time.Second)
	s, nueva, err := Raise(root, f.ID, "  lo vi en produccion  ")
	if err != nil || !nueva {
		t.Fatalf("CA-625: Raise: %+v %v %v", s, nueva, err)
	}
	if s.FindingID != f.ID || s.Reason != "lo vi en produccion" || !strings.Contains(s.Author, "rev@hoom.dev") ||
		s.RaisedAt.Before(antes) || s.RaisedAt.Location() != time.UTC {
		t.Fatalf("CA-625: la subida: %+v", s)
	}
	raw, err := os.ReadFile(crRaisePath(root, f.ID))
	if err != nil {
		t.Fatalf("CA-625: escribe %s: %v", crRaisePath(root, f.ID), err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-625: la subida es JSON: %v\n%s", err, raw)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, []string{"author", "finding_id", "raised_at", "reason"}) || m["finding_id"] != f.ID {
		t.Fatalf("CA-625: claves finding_id, reason, author, raised_at: %v", m)
	}
	st0, _ := os.Stat(crRaisePath(root, f.ID))
	time.Sleep(10 * time.Millisecond)
	s2, nueva, err := Raise(root, f.ID, "otra razon")
	if err != nil || nueva || s2.Reason != "lo vi en produccion" {
		t.Fatalf("CA-625: ya subido: devuelve la subida que habia, sin error y nueva false: %+v %v %v", s2, nueva, err)
	}
	raw2, _ := os.ReadFile(crRaisePath(root, f.ID))
	st1, _ := os.Stat(crRaisePath(root, f.ID))
	if string(raw2) != string(raw) || !st0.ModTime().Equal(st1.ModTime()) {
		t.Fatal("CA-625: un hallazgo ya subido no reescribe el archivo")
	}
	// la subida no es un hallazgo: List sigue viendo uno
	items, _, err := List(root, "main", false)
	if err != nil || len(items) != 1 || items[0].ID != f.ID {
		t.Fatalf("CA-625: un .raise.json nunca es un hallazgo: %+v %v", items, err)
	}
}

// CA-625: se niega, sin escribir, dentro de una corrida, sin razon, con un
// id que no existe o con un hallazgo cerrado, con los textos del contrato y
// en ese orden.
func TestCA625_RaiseSeNiega(t *testing.T) {
	crFuera(t)
	root := initRepo(t)
	f := crAlta(t, root, "medium", "app.go", "t")
	cerrado := crAlta(t, root, "high", "app.go", "t")
	if _, err := Resolve(root, cerrado.ID, "refutado", "el test TestX lo tumba", ""); err != nil {
		t.Fatal(err)
	}
	foto := crFoto(t, root)
	casos := []struct {
		id, reason string
		c          Corrida
		want       string
	}{
		{f.ID, "razon", Corrida{Role: "writer", Run: "r1"}, "un writer no sube hallazgos: lo hace una persona"},
		{f.ID, "razon", Corrida{Role: RolQueRefuta}, "un refutador no sube hallazgos: lo hace una persona"},
		{"no-existe", "", Corrida{Role: "reviewer"}, "un reviewer no sube hallazgos: lo hace una persona"}, // el rol va primero
		{f.ID, "", Corrida{}, "falta --reason: nadie sube un hallazgo sin decir por que"},
		{f.ID, " \t\n", Corrida{}, "falta --reason: nadie sube un hallazgo sin decir por que"},
		{"no-existe", "", Corrida{}, "falta --reason: nadie sube un hallazgo sin decir por que"}, // la razon antes que el id
		{"20260101T000000_abcdef", "razon", Corrida{}, "hallazgo no encontrado: 20260101T000000_abcdef"},
		{cerrado.ID, "razon", Corrida{}, `el hallazgo ` + cerrado.ID + ` ya esta resuelto como "refutado": un hallazgo cerrado no se sube`},
	}
	for _, c := range casos {
		_, nueva, err := RaiseEnCorrida(root, c.id, c.reason, c.c)
		if err == nil || nueva || err.Error() != c.want {
			t.Fatalf("CA-625: RaiseEnCorrida(%s, %q, %+v) se niega con %q: %v (nueva %v)", c.id, c.reason, c.c, c.want, err, nueva)
		}
		if !reflect.DeepEqual(foto, crFoto(t, root)) {
			t.Fatalf("CA-625: la negativa (%q) no escribe nada", c.want)
		}
	}
	// la negativa de una corrida es NoSube
	_, _, err := RaiseEnCorrida(root, f.ID, "r", Corrida{Role: "writer"})
	var ns NoSube
	if !errors.As(err, &ns) {
		t.Fatalf("CA-625: la negativa dentro de una corrida es NoSube: %T %v", err, err)
	}
}

// CA-626: las subidas invalidas no suben nada, dejan su aviso en List y no
// rompen la lectura; el gate no da ERROR por ellas.
func TestCA626_SubidasInvalidasAvisanYNoRompen(t *testing.T) {
	crFuera(t)
	root := initRepo(t)
	a := crAlta(t, root, "low", "app.go", "")
	b := crAlta(t, root, "low", "app.go", "")
	c := crAlta(t, root, "low", "app.go", "")
	huerfano := "20260101T000000_abcdef"
	foCrudo(t, root, a.ID+".raise.json", "{roto")
	foCrudo(t, root, b.ID+".raise.json", `{"finding_id": "`+c.ID+`", "reason": "otro id", "author": "Ana", "raised_at": "2026-11-10T10:00:00Z"}`)
	foCrudo(t, root, c.ID+".raise.json", `{"finding_id": "`+c.ID+`", "reason": "   ", "author": "Ana", "raised_at": "2026-11-10T10:00:00Z"}`)
	foCrudo(t, root, huerfano+".raise.json", `{"finding_id": "`+huerfano+`", "reason": "no existe", "author": "Ana", "raised_at": "2026-11-10T10:00:00Z"}`)

	items, warns, err := List(root, "main", false)
	if err != nil || len(items) != 3 {
		t.Fatalf("CA-626: List no se rompe y las subidas no son hallazgos: %d items, %v", len(items), err)
	}
	aviso := strings.Join(warns, "\n")
	for _, w := range []string{
		"subida ilegible ",
		"subida invalida ",
		"; el hallazgo " + b.ID + " no sube",
		"; el hallazgo " + c.ID + " no sube",
		"subida huerfana ",
		": no existe el hallazgo " + huerfano,
	} {
		if !strings.Contains(aviso, w) {
			t.Fatalf("CA-626: List avisa %q:\n%s", w, aviso)
		}
	}
	for _, n := range []string{a.ID + ".raise.json", b.ID + ".raise.json", c.ID + ".raise.json", huerfano + ".raise.json"} {
		if !strings.Contains(aviso, n) {
			t.Fatalf("CA-626: el aviso nombra el archivo %s:\n%s", n, aviso)
		}
	}
	// nada sube: con umbral high, los tres low son pendientes
	judged, _, err := ListConUmbral(root, "main", "high", true)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range judged {
		if it.Bloquea {
			t.Fatalf("CA-626: una subida invalida no sube %s: %+v", it.ID, it)
		}
	}
	// y el gate no da ERROR: PASS, con los avisos a la vista
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-626", g, verdict.StatusPass, "full")
	if !strings.Contains(g.OutputTail, "subida huerfana") || !strings.Contains(g.OutputTail, "subida invalida") || !strings.Contains(g.OutputTail, "subida ilegible") {
		t.Fatalf("CA-626: los avisos de las subidas van al detalle del gate:\n%s", g.OutputTail)
	}
	if strings.Contains(g.Notes, "subido a mano") {
		t.Fatalf("CA-626: nada sube: %q", g.Notes)
	}
}

// CA-626: un low con una subida valida bloquea en el gate y en la lista
// juzgada, con `subido a mano por <autor>`.
func TestCA626_SubidaValidaBloqueaEnElGate(t *testing.T) {
	crFuera(t)
	root := initRepo(t)
	f := crAlta(t, root, "low", "app.go", "t")
	g := Gate(root, "main", "high", "")
	foExigeGate(t, "CA-626 (fixture)", g, verdict.StatusPass, "full")
	s, _, err := Raise(root, f.ID, "rompe la factura")
	if err != nil {
		t.Fatal(err)
	}
	g = Gate(root, "main", "high", "")
	foExigeGate(t, "CA-626", g, verdict.StatusFail, "full")
	if !strings.Contains(g.Notes, f.ID) || !strings.Contains(g.OutputTail, "subido a mano por "+s.Author) {
		t.Fatalf("CA-626/CA-627: el gate falla por el low subido y nombra el motivo:\nnotes: %s\ntail: %s", g.Notes, g.OutputTail)
	}
	items, _, err := ListConUmbral(root, "main", "high", true)
	if err != nil || len(items) != 1 || !items[0].Bloquea || items[0].Motivo != "subido a mano por "+s.Author {
		t.Fatalf("CA-626: %+v %v", items, err)
	}
	// un hallazgo subido deja de bloquear cuando se cierra
	if _, err := Resolve(root, f.ID, "corregido", "commit abc arregla la factura", ""); err != nil {
		t.Fatal(err)
	}
	foExigeGate(t, "CA-626", Gate(root, "main", "high", ""), verdict.StatusPass, "full")
}

// CA-627: el detalle del gate nombra el motivo de lo que bloquea por ruta de
// riesgo; el caso 1 se ve como hoy. Con --spec sigue contando los sin tarea
// (caso limite: un medium en ruta de riesgo sin tarea bloquea en el gate).
func TestCA627_GateNombraElMotivoDeLaRuta(t *testing.T) {
	crFuera(t)
	root := initRepo(t)
	spec := foSpec(t, root, "t.md")
	m := crAlta(t, root, "medium", "internal/auth/login.go", "")
	h := crAlta(t, root, "high", "app.go", "t")
	crAlta(t, root, "medium", "app.go", "t")
	g := Gate(root, "main", "high", spec)
	foExigeGate(t, "CA-627", g, verdict.StatusFail, "spec")
	if !strings.HasPrefix(g.Notes, "2 abiertos que bloquean") || !strings.Contains(g.Notes, m.ID) || !strings.Contains(g.Notes, h.ID) {
		t.Fatalf("CA-627: bloquean el medium en ruta de riesgo sin tarea y el high: %q", g.Notes)
	}
	if !strings.Contains(g.OutputTail, "medium en ruta de riesgo: internal/auth/login.go") {
		t.Fatalf("CA-627: el detalle nombra el motivo del caso 3:\n%s", g.OutputTail)
	}
	if strings.Contains(g.OutputTail, "severidad high") {
		t.Fatalf("CA-627: el caso 1 se ve como hoy, sin motivo nuevo:\n%s", g.OutputTail)
	}
	// un medium en ruta de riesgo de OTRA tarea no entra con --spec
	root2 := initRepo(t)
	spec2 := foSpec(t, root2, "t.md")
	crAlta(t, root2, "medium", "internal/auth/login.go", "otra")
	foExigeGate(t, "CA-627", Gate(root2, "main", "high", spec2), verdict.StatusPass, "spec")
}

// CA-628: ListConUmbral juzga cada abierto; JSONBytesConUmbral trae bloquea
// y motivo en cada hallazgo (false y "" en los cerrados) y los avisos de las
// subidas en warnings; JSONBytes juzga con high.
func TestCA628_ListaJuzgadaYSuJSON(t *testing.T) {
	crFuera(t)
	root := initRepo(t)
	alto := crAlta(t, root, "high", "app.go", "t")
	ruta := crAlta(t, root, "medium", "internal/auth/x.go", "t")
	pend := crAlta(t, root, "medium", "app.go", "t")
	low := crAlta(t, root, "low", "app.go", "t")
	cerrado := crAlta(t, root, "high", "app.go", "t")
	if _, err := Resolve(root, cerrado.ID, "refutado", "el test TestX lo tumba", ""); err != nil {
		t.Fatal(err)
	}
	foCrudo(t, root, "20260101T000000_abcdef.raise.json", `{"finding_id": "20260101T000000_abcdef", "reason": "x", "author": "A", "raised_at": "2026-11-10T10:00:00Z"}`)

	items, warns, err := ListConUmbral(root, "main", "high", false)
	if err != nil || len(items) != 5 {
		t.Fatalf("CA-628: %d items %v", len(items), err)
	}
	quiero := map[string][2]any{
		alto.ID:    {true, "severidad high"},
		ruta.ID:    {true, "medium en ruta de riesgo: internal/auth/x.go"},
		pend.ID:    {false, ""},
		low.ID:     {false, ""},
		cerrado.ID: {false, ""},
	}
	for _, it := range items {
		w := quiero[it.ID]
		if it.Bloquea != w[0] || it.Motivo != w[1] {
			t.Fatalf("CA-628: %s (%s %s): (%v, %q), quiero %v", it.ID, it.Severity, it.Status, it.Bloquea, it.Motivo, w)
		}
	}
	if !strings.Contains(strings.Join(warns, "\n"), "subida huerfana") {
		t.Fatalf("CA-628: los avisos de subidas salen en la lista: %v", warns)
	}
	// List sin juzgar deja false y ""
	plain, _, _ := List(root, "main", false)
	for _, it := range plain {
		if it.Bloquea || it.Motivo != "" {
			t.Fatalf("CA-628: List no juzga: %+v", it)
		}
	}
	for _, raw := range [][]byte{crMust(JSONBytesConUmbral(root, "main", "high", false)), crMust(JSONBytes(root, "main", false))} {
		var crudo struct {
			Warnings []string         `json:"warnings"`
			Findings []map[string]any `json:"findings"`
		}
		if err := json.Unmarshal(raw, &crudo); err != nil {
			t.Fatal(err)
		}
		if crudo.Warnings == nil || !strings.Contains(strings.Join(crudo.Warnings, "\n"), "subida huerfana") {
			t.Fatalf("CA-628: warnings trae los avisos de subidas: %s", raw)
		}
		if len(crudo.Findings) != 5 {
			t.Fatalf("CA-628: %s", raw)
		}
		for _, f := range crudo.Findings {
			b, okb := f["bloquea"].(bool)
			m, okm := f["motivo"].(string)
			if !okb || !okm {
				t.Fatalf("CA-628: cada hallazgo trae bloquea (booleano) y motivo: %v", f)
			}
			w := quiero[f["id"].(string)]
			if b != w[0] || m != w[1] {
				t.Fatalf("CA-628: en el JSON %v: (%v, %q), quiero %v", f["id"], b, m, w)
			}
		}
	}
}

// CA-628: con block_on medium el medium pendiente pasa a bloquear por su
// severidad en la lista juzgada.
func TestCA628_ListaConOtroUmbral(t *testing.T) {
	crFuera(t)
	root := initRepo(t)
	m := crAlta(t, root, "medium", "app.go", "t")
	items, _, err := ListConUmbral(root, "main", "medium", true)
	if err != nil || len(items) != 1 || items[0].ID != m.ID || !items[0].Bloquea || items[0].Motivo != "severidad medium" {
		t.Fatalf("CA-628: %+v %v", items, err)
	}
}

func crMust(b []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return b
}
