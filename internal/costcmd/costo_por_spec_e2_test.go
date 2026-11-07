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

// cfDuenio es la regla del spec: task; si no, finding_task; si no, la tarea
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

// CA-549 (propiedad, semilla fija): para combinaciones al azar de task,
// finding_task, rol y registro que nombra el run, cada run se cuenta en
// exactamente la tarea que dice la regla del spec (task; si no,
// finding_task; si no, el registro que lo nombra) y nunca en dos: ni en dos
// tareas, ni en dos roles, ni en dos filas de rondas. Los runs sin ninguna
// tarea no se cuentan. Se verifica con la lista y con el slug de cada tarea.
func TestCA549_PropiedadCadaRunEnUnaSolaTarea(t *testing.T) {
	r := rand.New(rand.NewSource(549))
	slugs := []string{"tarea-a", "tarea-b", "tarea-c"}
	elegir := func(conVacio int) string { // conVacio: peso del ""
		k := r.Intn(len(slugs) + conVacio)
		if k < conVacio {
			return ""
		}
		return slugs[k-conVacio]
	}
	roles := []string{"reviewer", "reviewer", "writer", ""}
	for caso := 0; caso < 40; caso++ {
		root := ccRoot(t)
		c := &cfCaso{nombrado: map[int]int{}}
		n := 3 + r.Intn(10)
		for i := 0; i < n; i++ {
			m := ccMeta(elegir(3), roles[r.Intn(len(roles))], "codex", ccT0.Add(ccMin(10*i)), ccMin(5))
			if m.Task == "" || r.Intn(4) == 0 { // a veces un task con finding_task (task gana)
				m.FindingTask = elegir(2)
			}
			m.Usage = ccCodex(1<<uint(i), 0, 1, 0)
			c.metas = append(c.metas, ccRun(t, root, m))
		}
		// registros: cada uno de una tarea (a veces una que solo tiene
		// registros) nombra runs que ningun otro registro nombra
		libres := r.Perm(n)
		for k := 0; k < r.Intn(4); k++ {
			task := append([]string{"tarea-d"}, slugs...)[r.Intn(len(slugs)+1)]
			var nombra []int
			for len(libres) > 0 && r.Intn(3) != 0 {
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

		// lo esperado, por tarea: el total, por rol, y la fila sin ronda
		// (la segura y la que el spec no decide: un reviewer de T que solo
		// nombra un registro de otra tarea)
		total := map[string]uint{}
		porRol := map[string]map[string]uint{}
		sinRondaSegura, sinRondaPosible := map[string]uint{}, map[string]uint{}
		tareas := map[string]bool{}
		for _, task := range c.regTask {
			tareas[task] = true
		}
		cruzado := false
		for i, m := range c.metas {
			d := c.duenio(i)
			if reg, ok := c.nombrado[i]; ok && d != c.regTask[reg] {
				cruzado = true
			}
			if d == "" {
				continue
			}
			tareas[d] = true
			bit := uint(1) << uint(i)
			total[d] |= bit
			if porRol[d] == nil {
				porRol[d] = map[string]uint{}
			}
			porRol[d][m.Role] |= bit
			if m.Role == "reviewer" {
				reg, ok := c.nombrado[i]
				switch {
				case !ok:
					sinRondaSegura[d] |= bit
				case c.regTask[reg] != d:
					sinRondaPosible[d] |= bit
				}
			}
		}
		describe := func() string {
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

		lista := ccBuild(t, root, "")
		vistos := map[string]bool{}
		var enRondas uint
		for _, ta := range lista.Tareas {
			if vistos[ta.Slug] {
				t.Fatalf("CA-549: caso %d: la tarea %q aparece dos veces%s", caso, ta.Slug, describe())
			}
			vistos[ta.Slug] = true
			if !tareas[ta.Slug] {
				t.Fatalf("CA-549: caso %d: la tarea %q no tiene runs ni registros%s", caso, ta.Slug, describe())
			}
		}
		for slug := range tareas {
			if !vistos[slug] {
				t.Fatalf("CA-549: caso %d: falta la tarea %q en la lista%s", caso, slug, describe())
			}
		}
		for slug := range tareas {
			sola := ccBuild(t, root, slug)
			for _, rep := range []struct {
				que string
				r   Reporte
			}{{"lista", lista}, {"slug " + slug, sola}} {
				ta := ccTarea(t, "CA-549", rep.r, slug)
				que := fmt.Sprintf("caso %d, %s, tarea %s", caso, rep.que, slug)
				g := ta.PorProvider["codex"]
				if got := cfBits(g.Tokens.Nuevos); got != total[slug] || g.Runs != len(cfLista(total[slug])) {
					t.Fatalf("CA-549: %s: cuenta los runs %v (%d), conto %v (%d)%s", que,
						cfLista(total[slug]), len(cfLista(total[slug])), cfLista(got), g.Runs, describe())
				}
				var roles uint
				for _, gr := range ta.PorRol {
					if gr.Provider != "codex" {
						continue
					}
					got := cfBits(gr.Tokens.Nuevos)
					if got != porRol[slug][gr.Rol] || roles&got != 0 {
						t.Fatalf("CA-549: %s: el rol %q cuenta %v, conto %v%s", que, gr.Rol,
							cfLista(porRol[slug][gr.Rol]), cfLista(got), describe())
					}
					roles |= got
				}
				if roles != total[slug] {
					t.Fatalf("CA-549: %s: los roles cubren %v, cubrieron %v%s", que, cfLista(total[slug]), cfLista(roles), describe())
				}
				var filas uint
				for _, rr := range ta.RondasReview {
					var got uint
					if rr.Gasto != nil {
						got = cfBits(rr.Gasto.Tokens.Nuevos)
					}
					if filas&got != 0 {
						t.Fatalf("CA-549: %s: un run en dos filas de rondas: %v%s", que, cfLista(filas&got), describe())
					}
					filas |= got
					if rr.SinRonda {
						if seg, pos := sinRondaSegura[slug], sinRondaPosible[slug]; got&seg != seg || got&^(seg|pos) != 0 {
							t.Fatalf("CA-549: %s: la fila reviewer (sin ronda) tiene %v (y quiza %v), tiene %v%s", que,
								cfLista(seg), cfLista(pos), cfLista(got), describe())
						}
						continue
					}
					if rr.ID == nil {
						t.Fatalf("CA-549: %s: una fila sin id que no es la sin ronda: %+v", que, rr)
					}
					var quiero uint
					for k, id := range c.regIDs {
						if id != *rr.ID {
							continue
						}
						if c.regTask[k] != slug {
							t.Fatalf("CA-549: %s: la ronda %s es de %s, no de esta tarea%s", que, id, c.regTask[k], describe())
						}
						for _, i := range c.regRuns[k] {
							if c.duenio(i) == slug {
								quiero |= 1 << uint(i)
							}
						}
					}
					if got != quiero {
						t.Fatalf("CA-549: %s: la ronda %s cuenta los runs %v que nombra y son de la tarea, conto %v%s", que,
							*rr.ID, cfLista(quiero), cfLista(got), describe())
					}
				}
				if sinRondaSegura[slug] != 0 {
					if _, ok := cfSinRonda(t, ta); !ok {
						t.Fatalf("CA-549: %s: hay reviewer sin registro (%v): falta la fila sin ronda%s", que,
							cfLista(sinRondaSegura[slug]), describe())
					}
				}
				if rep.que == "lista" {
					if enRondas&filas != 0 {
						t.Fatalf("CA-549: %s: un run en las rondas de dos tareas: %v%s", que, cfLista(enRondas&filas), describe())
					}
					enRondas |= filas
				}
			}
		}
		if cruzado && len(lista.Avisos) == 0 {
			t.Fatalf("CA-549: caso %d: un registro nombra un run de otra tarea y el reporte no lo avisa%s", caso, describe())
		}
		for _, s := range []string{"nunca", "tarea-z"} {
			if _, err := Build(root, s); !cfSinNada(err, s) {
				t.Fatalf("CA-549: caso %d: %s no tiene runs ni registros: SinNada, fue %v", caso, s, err)
			}
		}
	}
}
