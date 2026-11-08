// Tests adversariales de la enmienda 2 del spec .hoom/specs/costo-por-spec.md
// (CA-549) sobre `hoom costo`: de que tarea es un run. De su `task`; si no
// tiene, de su `finding_task`; si no tiene ninguno, de la tarea del registro
// de review que lo nombra en `runs` (un registro que nombra un run de otra
// tarea no lo suma: lo avisa). Una review `--spec` sin `--task` que no
// termina no deja registro: sus runs se muestran como `reviewer (sin ronda)`
// de la tarea de su finding_task. Escritos DESDE EL SPEC y el contrato
// publico del paquete (go doc), sin leer la implementacion.
//
// Los fixtures son los de costo_por_spec_test.go (ccRoot, ccMeta, ccRun,
// ccRegistro, ...): sidecars escritos a mano en .hoom/runs con
// Meta.FindingTask, y registros con reviewcmd.WriteRecord.
package costcmd

import (
	"errors"
	"fmt"
	"math/bits"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/reviewcmd"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// cfMeta es un run sin task cuya tarea de hallazgos es ft.
func cfMeta(ft, role, prov string, minuto int) runcmd.Meta {
	m := ccMeta("", role, prov, ccT0.Add(ccMin(minuto)), ccMin(5))
	m.FindingTask = ft
	return m
}

// cfSinNada dice si err es el *SinNada del slug.
func cfSinNada(err error, slug string) bool {
	var sn *SinNada
	return errors.As(err, &sn) && err.Error() == fmt.Sprintf(ErrSinNada, slug)
}

// cfSinRonda devuelve la fila reviewer (sin ronda) de la tarea, si la hay.
func cfSinRonda(t *testing.T, ta Tarea) (Ronda, bool) {
	t.Helper()
	var filas []Ronda
	for _, rr := range ta.RondasReview {
		if rr.SinRonda {
			filas = append(filas, rr)
		}
	}
	if len(filas) > 1 {
		t.Fatalf("CA-549: a lo sumo una fila sin ronda por tarea: %+v", ta.RondasReview)
	}
	if len(filas) == 1 {
		return filas[0], true
	}
	return Ronda{}, false
}

// ---------------------------------------------------------------- CA-549

// CA-549 (caso limite "Ronda NO ENTREGABLE"): una review --spec sin --task
// que no termino dejo runs del reviewer sin task, con finding_task = alfa, y
// ningun registro. `hoom costo alfa` los cuenta como de alfa: el total, el
// rol reviewer y la fila reviewer (sin ronda), con su gasto; el run que
// fallo cuenta igual, porque el costo se gasto.
func TestCA549_RunSinTaskConFindingTaskEsDeLaTarea(t *testing.T) {
	root := ccRoot(t)
	ok := cfMeta("alfa", "reviewer", "codex", 0)
	ok.Usage = ccCodex(1200, 800, 50, 20)
	ok.Ventanas = []runcmd.VentanaRun{ccV("5h", 40, 43, ccR5)}
	ccRun(t, root, ok)
	fallo := cfMeta("alfa", "reviewer", "codex", 10)
	fallo.Status, fallo.ExitCode = runcmd.StatusError, 3
	fallo.Usage = ccCodex(1000, 0, 10, 5)
	ccRun(t, root, fallo)

	r, err := Build(root, "alfa")
	if err != nil {
		t.Fatalf("CA-549: una tarea cuyo unico rastro son runs con finding_task tiene costo: %v", err)
	}
	ta := ccTarea(t, "CA-549", r, "alfa")
	g, hay := ta.PorProvider["codex"]
	if !hay || g.Runs != 2 {
		t.Fatalf("CA-549: los 2 runs sin task con finding_task alfa son de alfa: %+v", ta.PorProvider)
	}
	ccTokens(t, "CA-549", "total codex", g.Tokens, ccI(1400), ccI(800), ccI(60), ccI(25))
	if rol := ccPorRol(t, "CA-549", ta, "reviewer", "codex"); rol.Runs != 2 {
		t.Fatalf("CA-549: los 2 runs van en el rol reviewer: %+v", rol)
	}
	if ta.Rondas != 0 {
		t.Fatalf("CA-549: sin registro no hay rondas: %d", ta.Rondas)
	}
	if len(ta.RondasReview) != 1 {
		t.Fatalf("CA-549: una sola fila, la de reviewer (sin ronda): %+v", ta.RondasReview)
	}
	sin := ta.RondasReview[0]
	if !sin.SinRonda || sin.ID != nil || sin.Cobertura != "" || sin.Gasto == nil || sin.Gasto.Runs != 2 {
		t.Fatalf("CA-549: la fila es reviewer (sin ronda), id null, con los 2 runs: %+v", sin)
	}
	ccTokens(t, "CA-549", "sin ronda", sin.Gasto.Tokens, ccI(1400), ccI(800), ccI(60), ccI(25))
	if texto := ccTexto(r, true); !strings.Contains(texto, "reviewer (sin ronda)") {
		t.Fatalf("CA-549: el desglose muestra reviewer (sin ronda):\n%s", texto)
	}
	m := ccJSON(t, r)
	tareas, _ := m["tareas"].([]any)
	if len(tareas) != 1 {
		t.Fatalf("CA-549: el JSON trae la tarea alfa: %v", m)
	}
	e := tareas[0].(map[string]any)
	rr, _ := e["rondas_review"].([]any)
	if e["slug"] != "alfa" || len(rr) != 1 {
		t.Fatalf("CA-549: el JSON trae alfa con su fila sin ronda: %v", e)
	}
	if f := rr[0].(map[string]any); f["sin_ronda"] != true || f["id"] != nil || f["gasto"] == nil {
		t.Fatalf("CA-549: la fila sin ronda del JSON: id null, sin_ronda true y su gasto: %v", f)
	}
}

// CA-549: un slug cuyo unico rastro es un run con finding_task ya no es
// SinNada, sea cual sea el rol del run. Un slug sin nada lo sigue siendo
// aunque haya runs con finding_task de otras tareas.
func TestCA549_SlugSoloConFindingTaskNoEsSinNada(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, cfMeta("solo-reviewer", "reviewer", "codex", 0))
	ccRun(t, root, cfMeta("solo-writer", "writer", "claude", 10))
	ccRun(t, root, cfMeta("solo-sin-rol", "", "claude", 20))
	for _, slug := range []string{"solo-reviewer", "solo-writer", "solo-sin-rol"} {
		r, err := Build(root, slug)
		if err != nil {
			t.Fatalf("CA-549: %s tiene un run con su finding_task: no es SinNada: %v", slug, err)
		}
		ta := ccTarea(t, "CA-549", r, slug)
		total := 0
		for _, g := range ta.PorProvider {
			total += g.Runs
		}
		if total != 1 {
			t.Fatalf("CA-549: %s cuenta su unico run: %+v", slug, ta.PorProvider)
		}
	}
	if _, err := Build(root, "nada"); !cfSinNada(err, "nada") {
		t.Fatalf("CA-549: un slug sin runs ni reviews sigue siendo SinNada: %v", err)
	}
}

// CA-549: "de su task" gana. Un run con task beta y finding_task alfa es de
// beta, no de alfa; alfa solo tiene el suyo. Si ese es el unico run que
// nombra a alfa, alfa no tiene costo.
func TestCA549_ElTaskGanaAlFindingTask(t *testing.T) {
	root := ccRoot(t)
	deBeta := ccMeta("beta", "reviewer", "codex", ccT0, ccMin(5))
	deBeta.FindingTask = "alfa"
	deBeta.Usage = ccCodex(7000, 0, 70, 7)
	ccRun(t, root, deBeta)
	if _, err := Build(root, "alfa"); !cfSinNada(err, "alfa") {
		t.Fatalf("CA-549: un run con task beta no es de alfa aunque su finding_task diga alfa: %v", err)
	}

	deAlfa := cfMeta("alfa", "reviewer", "codex", 10)
	deAlfa.Usage = ccCodex(300, 100, 3, 1)
	ccRun(t, root, deAlfa)
	for _, slug := range []string{"", "alfa"} {
		a := ccTarea(t, "CA-549", ccBuild(t, root, slug), "alfa")
		if g := a.PorProvider["codex"]; g.Runs != 1 {
			t.Fatalf("CA-549 (slug %q): alfa tiene solo su run (el de task beta es de beta): %+v", slug, a.PorProvider)
		}
		ccTokens(t, "CA-549", "alfa", a.PorProvider["codex"].Tokens, ccI(200), ccI(100), ccI(3), ccI(1))
	}
	for _, slug := range []string{"", "beta"} {
		b := ccTarea(t, "CA-549", ccBuild(t, root, slug), "beta")
		if g := b.PorProvider["codex"]; g.Runs != 1 {
			t.Fatalf("CA-549 (slug %q): el run de task beta es de beta: %+v", slug, b.PorProvider)
		}
		ccTokens(t, "CA-549", "beta", b.PorProvider["codex"].Tokens, ccI(7000), ccI(0), ccI(70), ccI(7))
		if sin, ok := cfSinRonda(t, b); !ok || sin.Gasto == nil || sin.Gasto.Runs != 1 {
			t.Fatalf("CA-549 (slug %q): el reviewer de beta va en la fila sin ronda de beta: %+v", slug, b.RondasReview)
		}
	}
}

// CA-549: sin slug, la lista muestra la tarea que solo tiene runs sin task
// con su finding_task, en su lugar por actividad. Un run sin task ni
// finding_task no es de ninguna tarea.
func TestCA549_ListaSinSlugMuestraLaTareaDeFindingTask(t *testing.T) {
	root := ccRoot(t)
	ccRun(t, root, ccMeta("vieja", "writer", "claude", ccT0, ccMin(5)))
	ccRun(t, root, cfMeta("nueva-ft", "reviewer", "codex", 60))
	ccRun(t, root, cfMeta("", "reviewer", "codex", 120))
	r := ccBuild(t, root, "")
	var slugs []string
	for _, ta := range r.Tareas {
		slugs = append(slugs, ta.Slug)
	}
	if strings.Join(slugs, ",") != "nueva-ft,vieja" {
		t.Fatalf("CA-549: la lista trae nueva-ft (solo runs con finding_task) y vieja, la mas reciente primero: %q", slugs)
	}
	if texto := ccTexto(r, false); !strings.Contains(texto, "nueva-ft") {
		t.Fatalf("CA-549: la linea de nueva-ft esta en la lista:\n%s", texto)
	}
	if m := ccJSON(t, r); len(m["tareas"].([]any)) != 2 {
		t.Fatalf("CA-549: el JSON trae las 2 tareas: %v", m["tareas"])
	}
}

// CA-549 (hostil): los runs con finding_task de otra tarea no se cuelan, ni
// los que solo se le parecen (prefijo, mayusculas, espacios): finding_task
// tiene que SER el slug. La fila sin ronda de alfa tiene sus 2 reviewer (el
// de task alfa y el de finding_task alfa), nada mas.
func TestCA549_FindingTaskDeOtraTareaNoSeCuela(t *testing.T) {
	root := ccRoot(t)
	conTask := ccMeta("alfa", "reviewer", "codex", ccT0, ccMin(5))
	conTask.Usage = ccCodex(100, 0, 1, 0)
	ccRun(t, root, conTask)
	conFT := cfMeta("alfa", "reviewer", "codex", 10)
	conFT.Usage = ccCodex(200, 0, 2, 0)
	ccRun(t, root, conFT)
	for i, ajena := range []string{"beta", "alfa-beta", "alf", "Alfa", " alfa", "alfa "} {
		m := cfMeta(ajena, "reviewer", "codex", 20+10*i)
		m.Usage = ccCodex(100000, 0, 1000, 0)
		ccRun(t, root, m)
	}
	ta := ccTarea(t, "CA-549", ccBuild(t, root, "alfa"), "alfa")
	if g := ta.PorProvider["codex"]; g.Runs != 2 {
		t.Fatalf("CA-549: alfa tiene sus 2 runs, ninguno ajeno: %+v", ta.PorProvider)
	}
	ccTokens(t, "CA-549", "alfa", ta.PorProvider["codex"].Tokens, ccI(300), ccI(0), ccI(3), nil)
	sin, ok := cfSinRonda(t, ta)
	if !ok || sin.Gasto == nil || sin.Gasto.Runs != 2 {
		t.Fatalf("CA-549: la fila sin ronda de alfa tiene sus 2 reviewer: %+v", ta.RondasReview)
	}
	ccTokens(t, "CA-549", "sin ronda de alfa", sin.Gasto.Tokens, ccI(300), ccI(0), ccI(3), nil)
	b := ccTarea(t, "CA-549", ccBuild(t, root, "beta"), "beta")
	if g := b.PorProvider["codex"]; g.Runs != 1 {
		t.Fatalf("CA-549: el run con finding_task beta es de beta: %+v", b.PorProvider)
	}
}

// CA-549: si la review --spec sin --task termino, su registro nombra el run
// del reviewer: el run (finding_task alfa) va en esa ronda de alfa y no en
// la fila sin ronda.
// Puede pasar hoy por la tercera regla (el registro de alfa que lo nombra);
// fija que finding_task no lo saque de su ronda.
func TestCA549_RegistroDeLaTareaNombraElRunConFindingTask(t *testing.T) {
	root := ccRoot(t)
	rv := cfMeta("alfa", "reviewer", "codex", 0)
	rv.Usage = ccCodex(1000, 500, 10, 1)
	ccRun(t, root, rv)
	reg := ccRegistro(t, root, "alfa", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(6)), []string{"risk"},
		ccLensRuns([]string{"risk"}, rv), nil)
	ta := ccTarea(t, "CA-549", ccBuild(t, root, "alfa"), "alfa")
	if len(ta.RondasReview) != 1 || ta.RondasReview[0].ID == nil || *ta.RondasReview[0].ID != reg.ID {
		t.Fatalf("CA-549: una sola fila, la ronda del registro (sin fila sin ronda): %+v", ta.RondasReview)
	}
	if g := ta.RondasReview[0].Gasto; g == nil || g.Runs != 1 {
		t.Fatalf("CA-549: la ronda cuenta el run que nombra: %+v", ta.RondasReview[0])
	}
	if g := ta.PorProvider["codex"]; g.Runs != 1 {
		t.Fatalf("CA-549: el run se cuenta una vez: %+v", ta.PorProvider)
	}
}

// CA-549: finding_task va antes que el registro que nombra al run. Un
// registro de beta que nombra un run sin task con finding_task alfa no lo
// suma a beta: el run es de alfa, y el reporte lo avisa.
func TestCA549_RegistroDeOtraTareaNoSeLlevaElRunConFindingTask(t *testing.T) {
	root := ccRoot(t)
	rv := cfMeta("alfa", "reviewer", "codex", 0)
	rv.Usage = ccCodex(1000, 0, 10, 0)
	ccRun(t, root, rv)
	ccRegistro(t, root, "beta", reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(6)), []string{"risk"},
		ccLensRuns([]string{"risk"}, rv), nil)

	for _, slug := range []string{"", "alfa"} {
		a := ccTarea(t, "CA-549", ccBuild(t, root, slug), "alfa")
		if g := a.PorProvider["codex"]; g.Runs != 1 {
			t.Fatalf("CA-549 (slug %q): el run con finding_task alfa es de alfa aunque lo nombre un registro de beta: %+v", slug, a.PorProvider)
		}
	}
	for _, slug := range []string{"", "beta"} {
		r := ccBuild(t, root, slug)
		b := ccTarea(t, "CA-549", r, "beta")
		if g, hay := b.PorProvider["codex"]; hay && g.Runs != 0 {
			t.Fatalf("CA-549 (slug %q): el registro de beta no suma el run de alfa: %+v", slug, b.PorProvider)
		}
		for _, rr := range b.RondasReview {
			if rr.Gasto != nil && rr.Gasto.Runs != 0 {
				t.Fatalf("CA-549 (slug %q): la ronda de beta no suma el run de alfa: %+v", slug, rr)
			}
		}
		if len(r.Avisos) == 0 {
			t.Fatalf("CA-549 (slug %q): un registro que nombra un run de otra tarea se avisa: %+v", slug, r)
		}
	}
}

// ---------------------------------------------------------------- propiedad
//
// TestCA549_PropiedadCadaRunEnUnaSolaTarea esta armada en cuatro partes:
//
//   - el generador (cfGenerador): escribe en disco un escenario al azar de
//     runs y registros, con la semilla fija;
//   - el oraculo (cfCaso.duenio y cfModelo): la tarea de cada run segun la
//     regla del spec, y lo que cada tarea tiene que mostrar;
//   - el diagnostico (cfCaso.describe): el escenario entero, para el
//     mensaje de una falla;
//   - las comprobaciones (cfVerificacion): cada una fija un invariante del
//     reporte contra el oraculo.

// cfCaso es un escenario al azar: runs (todos de codex, el run i con
// entrada 1<<i y sin cache: su "nuevos" es un bit propio) y registros que
// nombran runs (cada run lo nombra a lo sumo un registro).
type cfCaso struct {
	metas    []runcmd.Meta
	regTask  []string    // tarea de cada registro
	regRuns  [][]int     // runs que nombra cada registro
	nombrado map[int]int // run -> registro que lo nombra
	regIDs   []string    // id de cada registro
}

// duenio es la regla del spec: task; si no, finding_task; si no, la tarea
// del registro que lo nombra; si no, ninguna.
func (c *cfCaso) duenio(i int) string {
	m := c.metas[i]
	if m.Task != "" {
		return m.Task
	}
	if m.FindingTask != "" {
		return m.FindingTask
	}
	if reg, ok := c.nombrado[i]; ok {
		return c.regTask[reg]
	}
	return ""
}

// describe es el escenario en texto: cada run con su task, su finding_task,
// su rol, el registro que lo nombra y la tarea que le da la regla.
func (c *cfCaso) describe() string {
	var b strings.Builder
	for i, m := range c.metas {
		fmt.Fprintf(&b, "\n  run %d: task %q finding_task %q rol %q", i, m.Task, m.FindingTask, m.Role)
		if reg, ok := c.nombrado[i]; ok {
			fmt.Fprintf(&b, " nombrado por el registro %d (tarea %s)", reg, c.regTask[reg])
		}
		fmt.Fprintf(&b, " -> %q", c.duenio(i))
	}
	return b.String()
}

func cfBits(p *int) uint {
	if p == nil {
		return 0
	}
	return uint(*p)
}

func cfLista(mask uint) []int {
	var out []int
	for i := 0; i < bits.UintSize; i++ {
		if mask&(1<<uint(i)) != 0 {
			out = append(out, i)
		}
	}
	return out
}

// ------------------------------------------------------------- el generador

// cfGenerador arma los escenarios de la propiedad. Todo lo que sale al azar
// sale de r, en un orden fijo: con la misma semilla, los mismos escenarios.
type cfGenerador struct {
	r     *rand.Rand
	slugs []string // las tareas de los runs
	roles []string // reviewer pesa doble: es el rol de la fila sin ronda
}

func cfNuevoGenerador(semilla int64) *cfGenerador {
	return &cfGenerador{r: rand.New(rand.NewSource(semilla)),
		slugs: []string{"tarea-a", "tarea-b", "tarea-c"},
		roles: []string{"reviewer", "reviewer", "writer", ""}}
}

// elegir devuelve una de las tareas, o "" con peso conVacio.
func (g *cfGenerador) elegir(conVacio int) string {
	k := g.r.Intn(len(g.slugs) + conVacio)
	if k < conVacio {
		return ""
	}
	return g.slugs[k-conVacio]
}

// escenario escribe en root un escenario nuevo: sus runs y despues sus
// registros.
func (g *cfGenerador) escenario(t *testing.T, root string) *cfCaso {
	t.Helper()
	c := &cfCaso{nombrado: map[int]int{}}
	n := 3 + g.r.Intn(10)
	g.runs(t, root, c, n)
	g.registros(t, root, c, n)
	return c
}

// runs escribe n runs de codex, uno cada 10 minutos, sin superponerse. La
// mitad no tiene task; todo run sin task tiene finding_task con peso 3/5, y
// uno de cada cuatro con task tambien (ahi task gana). El run i tiene un bit
// propio en "nuevos".
func (g *cfGenerador) runs(t *testing.T, root string, c *cfCaso, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		m := ccMeta(g.elegir(3), g.roles[g.r.Intn(len(g.roles))], "codex", ccT0.Add(ccMin(10*i)), ccMin(5))
		if m.Task == "" || g.r.Intn(4) == 0 { // a veces un task con finding_task (task gana)
			m.FindingTask = g.elegir(2)
		}
		m.Usage = ccCodex(1<<uint(i), 0, 1, 0)
		c.metas = append(c.metas, ccRun(t, root, m))
	}
}

// registros escribe, despues de los runs, de 0 a 3 registros. Cada uno es
// de una tarea (a veces tarea-d, que solo tiene registros) y nombra runs que
// ningun otro registro nombra; puede no nombrar ninguno (un registro viejo,
// sin runs).
func (g *cfGenerador) registros(t *testing.T, root string, c *cfCaso, n int) {
	t.Helper()
	libres := g.r.Perm(n)
	for k := 0; k < g.r.Intn(4); k++ { // el tope se sortea en cada vuelta
		task := append([]string{"tarea-d"}, g.slugs...)[g.r.Intn(len(g.slugs)+1)]
		var nombra []int
		for len(libres) > 0 && g.r.Intn(3) != 0 {
			nombra = append(nombra, libres[0])
			libres = libres[1:]
		}
		sort.Ints(nombra)
		var lr []reviewcmd.LensRun
		for _, i := range nombra {
			lr = append(lr, reviewcmd.LensRun{Lens: "risk", Run: c.metas[i].ID})
			c.nombrado[i] = len(c.regTask)
		}
		reg := ccRegistro(t, root, task, reviewcmd.CoberturaCompleta, ccT0.Add(ccMin(10*n+k)), []string{"risk"}, lr, nil)
		c.regTask = append(c.regTask, task)
		c.regRuns = append(c.regRuns, nombra)
		c.regIDs = append(c.regIDs, reg.ID)
	}
}

// ---------------------------------------------------------------- el oraculo

// cfEsperado es lo que el reporte tiene que mostrar de un escenario, por
// tarea, en bits de run (el bit i es el run i).
type cfEsperado struct {
	// tareas son las que tienen algun run (por la regla) o algun registro.
	tareas map[string]bool
	// total son los runs de cada tarea.
	total map[string]uint
	// porRol son los runs de cada tarea por rol ("" = sin rol).
	porRol map[string]map[string]uint
	// sinRondaSegura son los reviewer de la tarea que ningun registro nombra:
	// tienen que estar en la fila sin ronda.
	sinRondaSegura map[string]uint
	// sinRondaPosible son los reviewer de la tarea que solo nombra un
	// registro de otra tarea: el spec no decide si van en la fila sin ronda,
	// asi que pueden estar o no.
	sinRondaPosible map[string]uint
	// cruzado: algun registro nombra un run de otra tarea (hay que avisarlo).
	cruzado bool
}

// cfModelo aplica la regla del spec (cfCaso.duenio) a cada run del
// escenario. Los runs sin tarea no van en ninguna.
func cfModelo(c *cfCaso) cfEsperado {
	e := cfEsperado{tareas: map[string]bool{}, total: map[string]uint{}, porRol: map[string]map[string]uint{},
		sinRondaSegura: map[string]uint{}, sinRondaPosible: map[string]uint{}}
	for _, task := range c.regTask {
		e.tareas[task] = true
	}
	for i, m := range c.metas {
		d := c.duenio(i)
		if reg, ok := c.nombrado[i]; ok && d != c.regTask[reg] {
			e.cruzado = true
		}
		if d == "" {
			continue
		}
		e.tareas[d] = true
		bit := uint(1) << uint(i)
		e.total[d] |= bit
		if e.porRol[d] == nil {
			e.porRol[d] = map[string]uint{}
		}
		e.porRol[d][m.Role] |= bit
		if m.Role == "reviewer" {
			reg, ok := c.nombrado[i]
			switch {
			case !ok:
				e.sinRondaSegura[d] |= bit
			case c.regTask[reg] != d:
				e.sinRondaPosible[d] |= bit
			}
		}
	}
	return e
}

// rondaEsperada son los runs que la ronda del registro id tiene que contar:
// los que el registro nombra y son de su tarea (un run de otra tarea no lo
// suma). deOtra es la tarea del registro si no es slug.
func (c *cfCaso) rondaEsperada(id, slug string) (quiero uint, deOtra string) {
	for k, rid := range c.regIDs {
		if rid != id {
			continue
		}
		if c.regTask[k] != slug {
			return 0, c.regTask[k]
		}
		for _, i := range c.regRuns[k] {
			if c.duenio(i) == slug {
				quiero |= 1 << uint(i)
			}
		}
	}
	return quiero, ""
}

// ------------------------------------------------------- las comprobaciones

// cfVerificacion compara los reportes de un escenario con su oraculo.
type cfVerificacion struct {
	t    *testing.T
	caso int
	c    *cfCaso
	e    cfEsperado
}

// lista fija que la lista sin slug trae cada tarea con runs o registros una
// sola vez, y ninguna otra.
func (v cfVerificacion) lista(lista Reporte) {
	v.t.Helper()
	vistos := map[string]bool{}
	for _, ta := range lista.Tareas {
		if vistos[ta.Slug] {
			v.t.Fatalf("CA-549: caso %d: la tarea %q aparece dos veces%s", v.caso, ta.Slug, v.c.describe())
		}
		vistos[ta.Slug] = true
		if !v.e.tareas[ta.Slug] {
			v.t.Fatalf("CA-549: caso %d: la tarea %q no tiene runs ni registros%s", v.caso, ta.Slug, v.c.describe())
		}
	}
	for slug := range v.e.tareas {
		if !vistos[slug] {
			v.t.Fatalf("CA-549: caso %d: falta la tarea %q en la lista%s", v.caso, slug, v.c.describe())
		}
	}
}

// tarea fija lo de la tarea slug en el reporte rep (la lista o el de su
// slug): que esta una vez, su total, sus roles y sus rondas. Devuelve los
// runs que cuentan sus filas de rondas.
func (v cfVerificacion) tarea(donde string, rep Reporte, slug string) (filas uint) {
	v.t.Helper()
	ta := ccTarea(v.t, "CA-549", rep, slug)
	que := fmt.Sprintf("caso %d, %s, tarea %s", v.caso, donde, slug)
	v.total(que, ta, slug)
	v.roles(que, ta, slug)
	return v.rondas(que, ta, slug)
}

// total fija que el total de codex de la tarea cuenta exactamente sus runs
// segun la regla: ni uno de otra tarea, ni uno sin tarea, ninguno dos veces.
func (v cfVerificacion) total(que string, ta Tarea, slug string) {
	v.t.Helper()
	quiero := v.e.total[slug]
	g := ta.PorProvider["codex"]
	if got := cfBits(g.Tokens.Nuevos); got != quiero || g.Runs != len(cfLista(quiero)) {
		v.t.Fatalf("CA-549: %s: cuenta los runs %v (%d), conto %v (%d)%s", que,
			cfLista(quiero), len(cfLista(quiero)), cfLista(got), g.Runs, v.c.describe())
	}
}

// roles fija que cada fila por rol de codex cuenta exactamente los runs de
// la tarea con ese rol, que ningun run esta en dos roles y que los roles
// juntos cubren el total.
func (v cfVerificacion) roles(que string, ta Tarea, slug string) {
	v.t.Helper()
	var roles uint
	for _, gr := range ta.PorRol {
		if gr.Provider != "codex" {
			continue
		}
		got := cfBits(gr.Tokens.Nuevos)
		if got != v.e.porRol[slug][gr.Rol] || roles&got != 0 {
			v.t.Fatalf("CA-549: %s: el rol %q cuenta %v, conto %v%s", que, gr.Rol,
				cfLista(v.e.porRol[slug][gr.Rol]), cfLista(got), v.c.describe())
		}
		roles |= got
	}
	if roles != v.e.total[slug] {
		v.t.Fatalf("CA-549: %s: los roles cubren %v, cubrieron %v%s", que, cfLista(v.e.total[slug]), cfLista(roles), v.c.describe())
	}
}

// rondas fija las filas de rondas de la tarea: ningun run en dos filas; la
// fila reviewer (sin ronda) tiene los reviewer que ningun registro nombra
// (y a lo sumo los que solo nombra un registro de otra tarea); toda otra
// fila es un registro de esta tarea y cuenta los runs que nombra y son de
// ella; y si hay reviewer sin registro, la fila sin ronda existe (una sola).
// Devuelve los runs que cuentan sus filas.
func (v cfVerificacion) rondas(que string, ta Tarea, slug string) (filas uint) {
	v.t.Helper()
	for _, rr := range ta.RondasReview {
		var got uint
		if rr.Gasto != nil {
			got = cfBits(rr.Gasto.Tokens.Nuevos)
		}
		if filas&got != 0 {
			v.t.Fatalf("CA-549: %s: un run en dos filas de rondas: %v%s", que, cfLista(filas&got), v.c.describe())
		}
		filas |= got
		if rr.SinRonda {
			if seg, pos := v.e.sinRondaSegura[slug], v.e.sinRondaPosible[slug]; got&seg != seg || got&^(seg|pos) != 0 {
				v.t.Fatalf("CA-549: %s: la fila reviewer (sin ronda) tiene %v (y quiza %v), tiene %v%s", que,
					cfLista(seg), cfLista(pos), cfLista(got), v.c.describe())
			}
			continue
		}
		if rr.ID == nil {
			v.t.Fatalf("CA-549: %s: una fila sin id que no es la sin ronda: %+v", que, rr)
		}
		quiero, deOtra := v.c.rondaEsperada(*rr.ID, slug)
		if deOtra != "" {
			v.t.Fatalf("CA-549: %s: la ronda %s es de %s, no de esta tarea%s", que, *rr.ID, deOtra, v.c.describe())
		}
		if got != quiero {
			v.t.Fatalf("CA-549: %s: la ronda %s cuenta los runs %v que nombra y son de la tarea, conto %v%s", que,
				*rr.ID, cfLista(quiero), cfLista(got), v.c.describe())
		}
	}
	if v.e.sinRondaSegura[slug] != 0 {
		if _, ok := cfSinRonda(v.t, ta); !ok {
			v.t.Fatalf("CA-549: %s: hay reviewer sin registro (%v): falta la fila sin ronda%s", que,
				cfLista(v.e.sinRondaSegura[slug]), v.c.describe())
		}
	}
	return filas
}

// rondasDeUnaSolaTarea fija que, en la lista, ningun run esta en las filas
// de rondas de dos tareas. enRondas acumula los de las tareas ya vistas.
func (v cfVerificacion) rondasDeUnaSolaTarea(que string, enRondas *uint, filas uint) {
	v.t.Helper()
	if *enRondas&filas != 0 {
		v.t.Fatalf("CA-549: %s: un run en las rondas de dos tareas: %v%s", que, cfLista(*enRondas&filas), v.c.describe())
	}
	*enRondas |= filas
}

// avisos fija que un registro que nombra un run de otra tarea se avisa.
func (v cfVerificacion) avisos(lista Reporte) {
	v.t.Helper()
	if v.e.cruzado && len(lista.Avisos) == 0 {
		v.t.Fatalf("CA-549: caso %d: un registro nombra un run de otra tarea y el reporte no lo avisa%s", v.caso, v.c.describe())
	}
}

// sinNada fija que un slug sin runs ni registros sigue siendo SinNada.
func (v cfVerificacion) sinNada(root string) {
	v.t.Helper()
	for _, s := range []string{"nunca", "tarea-z"} {
		if _, err := Build(root, s); !cfSinNada(err, s) {
			v.t.Fatalf("CA-549: caso %d: %s no tiene runs ni registros: SinNada, fue %v", v.caso, s, err)
		}
	}
}

// CA-549 (propiedad, semilla fija): para combinaciones al azar de task,
// finding_task, rol y registro que nombra el run, cada run se cuenta en
// exactamente la tarea que dice la regla del spec (task; si no,
// finding_task; si no, el registro que lo nombra) y nunca en dos: ni en dos
// tareas, ni en dos roles, ni en dos filas de rondas. Los runs sin ninguna
// tarea no se cuentan. Se verifica con la lista y con el slug de cada tarea.
func TestCA549_PropiedadCadaRunEnUnaSolaTarea(t *testing.T) {
	g := cfNuevoGenerador(549)
	for caso := 0; caso < 40; caso++ {
		root := ccRoot(t)
		c := g.escenario(t, root)
		v := cfVerificacion{t: t, caso: caso, c: c, e: cfModelo(c)}

		lista := ccBuild(t, root, "")
		v.lista(lista)
		var enRondas uint
		for slug := range v.e.tareas {
			sola := ccBuild(t, root, slug)
			filas := v.tarea("lista", lista, slug)
			v.rondasDeUnaSolaTarea(fmt.Sprintf("caso %d, %s, tarea %s", caso, "lista", slug), &enRondas, filas)
			v.tarea("slug "+slug, sola, slug)
		}
		v.avisos(lista)
		v.sinNada(root)
	}
}
