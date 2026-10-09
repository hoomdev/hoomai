// Tests de regresion de la review de 4 lentes de la cabina C2 sobre
// .hoom/specs/tablero-de-solo-lectura.md, en boardcmd:
//
//   - 20261009T193105_08ae7d (low, readability; CA-314): las filas de work
//     del detalle y el gasto de la tarjeta (card.spend) salen de la misma
//     telemetria y su suma es SIEMPRE identica. Un costo invalido (negativo:
//     lo unico invalido que entra por el JSON de un sidecar o de un sobre) se
//     trata igual en los dos lados: la fila queda sin costo (con sus tokens) y
//     el gasto no lo suma. Hoy la fila muestra el costo crudo y la suma de
//     work da otro numero que spend.
//   - 20261009T191843_bb986a (high, risk; CA-318): las lecturas del tablero
//     que reciben la base (Build, CardFor, DetailFor con diff y TimelineFor)
//     no crean ni truncan ningun archivo cuando la base empieza con "-". La
//     historia lee <base>..HEAD (dos puntos): es un lugar mas que los cuatro
//     de gitx.
//   - 20261009T191847_43bde9 (medium, risk; CA-315 y CA-318, enmienda 1): el
//     diff del detalle no ejecuta un textconv de la configuracion local de
//     git.
//   - 20261009T192554_12f52d (medium, reliability; CA-306 y CA-308, enmienda
//     1): el nombre de un gate se muestra como el proyecto lo escribio,
//     tambien en plain y red.plain. Es la unica excepcion al vocabulario del
//     modo normal; el resto de la frase lo sigue cumpliendo.
package boardcmd

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------------------
// 20261009T193105_08ae7d: la suma de work es card.spend
// ---------------------------------------------------------------------------

// Donde vive el usage de una pieza de la telemetria de la tarjeta.
const (
	c2RunSuelto       = "run suelto"               // un run que ningun sobre referencia: su sidecar
	c2SobreSinSidecar = "sobre sin sidecar"        // un sobre cuyo run no dejo sidecar: su propio usage
	c2SobreConSidecar = "sobre con sidecar"        // un sobre con run: manda el sidecar, no el usage del sobre
	c2Senuelo         = 0.11                       // el costo propio del sobre con sidecar: nunca cuenta
	c2FormaID         = "20261009T100000_%s%s%02d" // etiqueta (3) + letra (1) + indice (2)
)

// c2Pieza es algo que corrio para la tarjeta, con el usage que reporto.
type c2Pieza struct {
	donde   string
	costo   *float64 // nil = no reporto costo
	in, out int
}

func c2Valido(c *float64) bool { return c != nil && *c >= 0 }

// c2ID es el id de la FILA de work que le toca a la pieza i: el del run
// suelto, o el del sobre.
func c2ID(etiqueta string, i int, p c2Pieza) string {
	letra := "s"
	if p.donde == c2RunSuelto {
		letra = "r"
	}
	return fmt.Sprintf(c2FormaID, etiqueta, letra, i)
}

// c2Plantar crea la tarjeta slug y escribe su telemetria en disco: la pieza
// i empezo 10*(i+1) minutos antes de now y duro 5 (todas cerradas). etiqueta
// son 3 caracteres que hacen unicos los ids dentro del repo.
func c2Plantar(t *testing.T, root, slug, etiqueta string, now time.Time, piezas []c2Pieza) {
	t.Helper()
	bdItemArchivo(t, root, slug, "Tarjeta "+slug, "")
	for i, p := range piezas {
		ini := now.Add(-time.Duration(10*(i+1)) * time.Minute)
		fin := ini.Add(5 * time.Minute)
		uso := &providers.Usage{CostUSD: p.costo, InputTokens: p.in, OutputTokens: p.out}
		switch p.donde {
		case c2RunSuelto:
			bdRunDisco(t, root, runcmd.Meta{ID: c2ID(etiqueta, i, p), Provider: "claude", Role: "writer", Task: slug, Dir: root,
				CreatedAt: ini, EndedAt: fin, Status: runcmd.StatusDone, Usage: uso})
		case c2SobreSinSidecar:
			bdSobreDisco(t, root, envelope.Record{ID: c2ID(etiqueta, i, p), Role: "writer", Provider: "claude", Task: slug, Dir: root,
				RunID: fmt.Sprintf(c2FormaID, etiqueta, "x", i), Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
				Usage: uso, StartedAt: ini, UpdatedAt: fin, EndedAt: fin})
		case c2SobreConSidecar:
			run := fmt.Sprintf(c2FormaID, etiqueta, "c", i)
			bdRunDisco(t, root, runcmd.Meta{ID: run, Provider: "claude", Role: "writer", Task: slug, Dir: root,
				CreatedAt: ini, EndedAt: fin, Status: runcmd.StatusDone, Usage: uso})
			bdSobreDisco(t, root, envelope.Record{ID: c2ID(etiqueta, i, p), Role: "writer", Provider: "claude", Task: slug, Dir: root,
				RunID: run, Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable,
				Usage:     &providers.Usage{CostUSD: bdF(c2Senuelo), InputTokens: 1, OutputTokens: 1},
				StartedAt: ini, UpdatedAt: fin, EndedAt: fin})
		default:
			t.Fatalf("c2Plantar: donde %q", p.donde)
		}
	}
}

// c2SumaDeWork suma las filas de work como las suma quien mira el detalle:
// el costo de las filas que traen costo (nil si ninguna trae) y los tokens.
func c2SumaDeWork(d Detail) (costo *float64, in, out int) {
	for _, w := range d.Work {
		if w.Usage == nil {
			continue
		}
		if w.Usage.CostUSD != nil {
			if costo == nil {
				costo = bdF(0)
			}
			*costo += *w.Usage.CostUSD
		}
		in += w.Usage.InputTokens
		out += w.Usage.OutputTokens
	}
	return costo, in, out
}

// c2Revisar exige el contrato de CA-314 para la tarjeta que planto
// c2Plantar con esas piezas: una fila por pieza con su usage, el costo
// invalido tratado como "sin costo" en la fila, y la suma de las filas igual
// a card.spend. Devuelve los problemas (vacio = cumple).
func c2Revisar(d Detail, etiqueta string, piezas []c2Pieza) []string {
	var mal []string
	filas := map[string]*providers.Usage{}
	tiene := map[string]bool{}
	for _, w := range d.Work {
		filas[w.ID], tiene[w.ID] = w.Usage, true
	}
	if len(d.Work) != len(piezas) {
		mal = append(mal, fmt.Sprintf("work trae una fila por sobre y una por run sin sobre: %d piezas, %d filas", len(piezas), len(d.Work)))
	}
	var quiere *float64
	quiereIn, quiereOut := 0, 0
	for i, p := range piezas {
		id := c2ID(etiqueta, i, p)
		quiereIn, quiereOut = quiereIn+p.in, quiereOut+p.out
		if c2Valido(p.costo) {
			if quiere == nil {
				quiere = bdF(0)
			}
			*quiere += *p.costo
		}
		u := filas[id]
		if !tiene[id] || u == nil {
			mal = append(mal, fmt.Sprintf("la fila de %s (%s) trae su usage: fila=%v usage=%v", id, p.donde, tiene[id], u))
			continue
		}
		if u.InputTokens != p.in || u.OutputTokens != p.out {
			mal = append(mal, fmt.Sprintf("la fila de %s (%s) trae sus tokens %d/%d, fue %d/%d", id, p.donde, p.in, p.out, u.InputTokens, u.OutputTokens))
		}
		switch {
		case c2Valido(p.costo) && (u.CostUSD == nil || !tbCasi(*u.CostUSD, *p.costo)):
			mal = append(mal, fmt.Sprintf("la fila de %s (%s) trae su costo %s, fue %s", id, p.donde, h2FmtF(p.costo), h2FmtF(u.CostUSD)))
		case !c2Valido(p.costo) && u.CostUSD != nil:
			mal = append(mal, fmt.Sprintf("la fila de %s (%s) reporto el costo %s, que no cuenta en el gasto: la fila queda SIN costo (cost_usd null), fue %s",
				id, p.donde, h2FmtF(p.costo), h2FmtF(u.CostUSD)))
		}
	}
	s := d.Card.Spend
	costo, in, out := c2SumaDeWork(d)
	if !h2MismoF(costo, s.CostUSD) {
		mal = append(mal, fmt.Sprintf("la suma de cost_usd de las filas de work (%s) es igual a card.spend.cost_usd (%s)", h2FmtF(costo), h2FmtF(s.CostUSD)))
	}
	if in != s.InputTokens || out != s.OutputTokens {
		mal = append(mal, fmt.Sprintf("la suma de tokens de las filas de work (%d/%d) es igual a la de card.spend (%d/%d)", in, out, s.InputTokens, s.OutputTokens))
	}
	if !h2MismoF(s.CostUSD, quiere) || s.InputTokens != quiereIn || s.OutputTokens != quiereOut {
		mal = append(mal, fmt.Sprintf("card.spend suma solo los costos validos y todos los tokens: cost_usd=%s tokens=%d/%d, fue %s",
			h2FmtF(quiere), quiereIn, quiereOut, h2FmtSpend(s)))
	}
	return mal
}

// Hallazgo 20261009T193105_08ae7d. CA-314: el caso del hallazgo. Dos runs de
// la tarjeta con costo -5 y 0.8: card.spend.cost_usd es 0.8 y la suma de las
// filas de work tambien (no -4.2). El run de -5 tiene su fila, con sus tokens
// y sin costo, igual que un run que no reporto costo.
func TestHallazgo_08ae7d_LaSumaDeWorkEsSpendConUnCostoNegativo(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	piezas := []c2Pieza{
		{c2RunSuelto, bdF(0.8), 20, 2},
		{c2RunSuelto, bdF(-5), 10, 1},
	}
	c2Plantar(t, root, "mezcla", "mez", now, piezas)
	d, err := DetailFor(root, "main", "high", "mezcla", now, false)
	if err != nil {
		t.Fatal(err)
	}
	costo, _, _ := c2SumaDeWork(d)
	if s := d.Card.Spend; s.CostUSD == nil || !tbCasi(*s.CostUSD, 0.8) || costo == nil || !tbCasi(*costo, 0.8) {
		t.Errorf("08ae7d CA-314: con dos runs de costo -5 y 0.8, card.spend.cost_usd y la suma de cost_usd de las filas de work son 0.8 las dos; fueron spend=%s y suma de work=%s",
			h2FmtF(s.CostUSD), h2FmtF(costo))
	}
	for _, m := range c2Revisar(d, "mez", piezas) {
		t.Errorf("08ae7d CA-314: %s", m)
	}
}

// Hallazgo 20261009T193105_08ae7d. CA-314: el costo invalido se trata igual
// en la fila que en la suma, este donde este el usage que cuenta: en el
// sidecar de un run suelto, en el usage de un sobre cuyo run no dejo sidecar,
// o en el sidecar del run de un sobre (ahi el usage propio del sobre no
// cuenta ni para la fila ni para el gasto). Y si ningun costo es valido, el
// gasto es null y ninguna fila trae costo.
func TestHallazgo_08ae7d_ElCostoInvalidoQuedaSinCostoEnLaFilaYEnElGasto(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	casos := []struct {
		slug, etiqueta string
		piezas         []c2Pieza
	}{
		{"en-run-suelto", "aaa", []c2Pieza{{c2RunSuelto, bdF(-100), 20, 2}, {c2RunSuelto, bdF(0.9), 1000, 100}}},
		{"en-sobre-sin-sidecar", "bbb", []c2Pieza{{c2SobreSinSidecar, bdF(-0.01), 20, 2}, {c2RunSuelto, bdF(0.9), 1000, 100}}},
		{"en-sobre-con-sidecar", "ccc", []c2Pieza{{c2SobreConSidecar, bdF(-3), 20, 2}, {c2SobreConSidecar, bdF(0.9), 1000, 100}}},
		{"solo-invalidos", "ddd", []c2Pieza{{c2RunSuelto, bdF(-5), 5, 5}, {c2SobreSinSidecar, bdF(-1), 7, 7}}},
		{"invalido-y-sin-costo", "eee", []c2Pieza{{c2RunSuelto, bdF(-5), 5, 5}, {c2RunSuelto, nil, 50000, 2900}, {c2SobreSinSidecar, bdF(0), 3, 3}}},
	}
	for _, c := range casos {
		c2Plantar(t, root, c.slug, c.etiqueta, now, c.piezas)
	}
	for _, c := range casos {
		d, err := DetailFor(root, "main", "high", c.slug, now, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range c2Revisar(d, c.etiqueta, c.piezas) {
			t.Errorf("08ae7d CA-314 [%s]: %s", c.slug, m)
		}
	}
	// sin ningun costo valido: gasto null y ninguna fila con costo
	d, err := DetailFor(root, "main", "high", "solo-invalidos", now, false)
	if err != nil {
		t.Fatal(err)
	}
	if costo, _, _ := c2SumaDeWork(d); costo != nil || d.Card.Spend.CostUSD != nil {
		t.Errorf("08ae7d CA-314 [solo-invalidos]: costo null si ninguna fila trae costo: card.spend.cost_usd=%s, suma de work=%s",
			h2FmtF(d.Card.Spend.CostUSD), h2FmtF(costo))
	}
}

// Hallazgo 20261009T193105_08ae7d, como propiedad. CA-314: para cualquier
// mezcla de runs sueltos y sobres (con y sin sidecar) que reportan un costo
// valido, negativo o ninguno, la suma de cost_usd y de tokens de las filas de
// work es igual a card.spend, y ninguna fila trae un costo que el gasto no
// cuenta.
func TestHallazgo_08ae7d_PropiedadLaSumaDeWorkEsSpend(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	donde := []string{c2RunSuelto, c2SobreSinSidecar, c2SobreConSidecar}
	n := 0
	prop := func(codigos []uint8, centavos []int16) bool {
		largo := len(codigos)
		if len(centavos) < largo {
			largo = len(centavos)
		}
		if largo > 5 {
			largo = 5
		}
		var piezas []c2Pieza
		for i := 0; i < largo; i++ {
			p := c2Pieza{donde: donde[int(codigos[i])%len(donde)], in: 10 + i, out: 1 + i}
			if (codigos[i]/3)%4 != 0 { // 1 de cada 4 no reporta costo
				p.costo = bdF(float64(centavos[i]) / 100) // negativo o no
			}
			piezas = append(piezas, p)
		}
		n++
		etiqueta := fmt.Sprintf("p%02d", n)
		slug := "prop-" + etiqueta
		c2Plantar(t, root, slug, etiqueta, now, piezas)
		d, err := DetailFor(root, "main", "high", slug, now, false)
		if err != nil {
			t.Logf("DetailFor(%s): %v", slug, err)
			return false
		}
		mal := c2Revisar(d, etiqueta, piezas)
		for _, m := range mal {
			t.Logf("%s", m)
		}
		for _, w := range d.Work {
			if w.Usage != nil && w.Usage.CostUSD != nil && (*w.Usage.CostUSD < 0 || math.IsNaN(*w.Usage.CostUSD)) {
				t.Logf("la fila %s trae el costo %s, que el gasto no cuenta", w.ID, h2FmtF(w.Usage.CostUSD))
				return false
			}
		}
		return len(mal) == 0
	}
	cfg := &quick.Config{MaxCount: 20, Rand: rand.New(rand.NewSource(20261009))}
	if err := quick.Check(prop, cfg); err != nil {
		t.Fatalf("08ae7d CA-314: la suma de las filas de work es siempre card.spend: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 20261009T191843_bb986a: las lecturas del tablero con una base hostil
// ---------------------------------------------------------------------------

// c2Cambios dice, ordenado, que cambio entre dos fotos de bdFoto. Vacio si
// el disco quedo igual.
func c2Cambios(antes, despues map[string]string) []string {
	var out []string
	for k, v := range antes {
		if w, ok := despues[k]; !ok {
			out = append(out, "borro "+k)
		} else if w != v {
			out = append(out, "trunco o reescribio "+k)
		}
	}
	for k := range despues {
		if _, ok := antes[k]; !ok {
			out = append(out, "creo "+k)
		}
	}
	sort.Strings(out)
	return out
}

// c2ConBaseHostil arma un proyecto con la tarjeta "tarea", su espacio de
// trabajo con un commit propio y cambios sin commitear (en el espacio de
// trabajo y en el proyecto), y un directorio de victimas fuera del proyecto:
// <tmp>/x no existe, y <tmp>/x...HEAD y <tmp>/x..HEAD ya existen con
// contenido. hostil es la base "--output=<tmp>/x". revisar corre una lectura
// y exige que no cree ni trunque nada afuera, y que el proyecto quede byte a
// byte igual.
func c2ConBaseHostil(t *testing.T) (root, hostil string, now time.Time, revisar func(nombre string, leer func())) {
	t.Helper()
	root = bdRepo(t, "")
	now = time.Now().UTC()
	bdItemArchivo(t, root, c2SlugHostil, "Tarea", "")
	wt := bdWorktree(t, root, c2SlugHostil)
	bdEscribir(t, wt, "tarea.go", "package app\n\nfunc Tarea() int { return 1 }\n")
	bdCommitear(t, wt, "trabajo de la tarea")
	bdEscribir(t, wt, "sucio.go", "package app\n\nvar Sucio = 1\n")
	bdEscribir(t, root, "suelto.go", "package app\n\nvar Suelto = 1\n")

	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	const previo = "CONTENIDO PREVIO\n"
	reponer := func() {
		ents, err := os.ReadDir(fuera)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range ents {
			if err := os.RemoveAll(filepath.Join(fuera, e.Name())); err != nil {
				t.Fatal(err)
			}
		}
		bdEscribir(t, fuera, "x...HEAD", previo)
		bdEscribir(t, fuera, "x..HEAD", previo)
	}
	reponer()
	hostil = "--output=" + filepath.Join(fuera, "x")

	revisar = func(nombre string, leer func()) {
		t.Helper()
		antesFuera, antesRepo := bdFoto(t, fuera), bdFoto(t, root)
		leer()
		if c := c2Cambios(antesFuera, bdFoto(t, fuera)); len(c) != 0 {
			t.Errorf("bb986a CA-318: %s es una lectura: con la base \"--output=<tmp>/x\" no crea ni trunca ningun archivo; fuera del repo %s",
				nombre, strings.Join(c, ", "))
		}
		if c := c2Cambios(antesRepo, bdFoto(t, root)); len(c) != 0 {
			t.Errorf("bb986a CA-318: %s deja el repo y .hoom/ byte a byte iguales, ignorados incluidos; %s", nombre, strings.Join(c, ", "))
		}
		reponer()
	}
	return root, hostil, now, revisar
}

const c2SlugHostil = "tarea"

// Hallazgo 20261009T191843_bb986a. CA-318: Build, CardFor y DetailFor (con y
// sin diff) son lecturas. Llamadas directo con una base que empieza con
// "--output=" (lo que les llegaria de un base_branch hostil), sobre una
// tarjeta con espacio de trabajo y commits propios, no crean <tmp>/x ni
// truncan <tmp>/x...HEAD, y dejan el proyecto byte a byte igual. Pueden
// fallar o decir "no disponible"; lo que no pueden es escribir.
func TestHallazgo_bb986a_LasLecturasDelTableroNoEscribenConUnaBaseQueEsOpcion(t *testing.T) {
	root, hostil, now, revisar := c2ConBaseHostil(t)
	revisar("Build", func() { _, _ = Build(root, hostil, "high", now) })
	revisar("CardFor", func() { _, _ = CardFor(root, hostil, "high", c2SlugHostil, now) })
	revisar("DetailFor", func() { _, _ = DetailFor(root, hostil, "high", c2SlugHostil, now, false) })
	revisar("DetailFor con diff", func() { _, _ = DetailFor(root, hostil, "high", c2SlugHostil, now, true) })
}

// Hallazgo 20261009T191843_bb986a, el quinto lugar. CA-318 (y la historia de
// C4: CA-359, solo lectura, y CA-360, que lee los commits de <base>..HEAD):
// TimelineFor, llamada directo con una base que empieza con "--output=", no
// crea <tmp>/x ni trunca <tmp>/x..HEAD (dos puntos) ni <tmp>/x...HEAD. Es la
// misma inyeccion que en los cuatro lugares de gitx, en un pedido de git que
// el refutador no listo: lo encontro el test de punta a punta
// (GET /api/board/{slug}/timeline dejaba x..HEAD).
func TestHallazgo_bb986a_LaHistoriaNoEscribeConUnaBaseQueEsOpcion(t *testing.T) {
	root, hostil, now, revisar := c2ConBaseHostil(t)
	revisar("TimelineFor", func() { _, _ = TimelineFor(root, hostil, "high", c2SlugHostil, now) })
}

// ---------------------------------------------------------------------------
// 20261009T191847_43bde9: el diff del detalle no ejecuta un textconv
// ---------------------------------------------------------------------------

// Hallazgo 20261009T191847_43bde9. CA-315 y CA-318 (enmienda 1): el detalle
// con diff de una tarjeta cuyo espacio de trabajo cambio un archivo con un
// driver diff.<x>.textconv de la config local (puesto por un .gitattributes
// de la rama) no corre el comando del driver ninguna vez: el driver deja una
// marca fuera del proyecto si corre. El diff sale disponible y con el
// contenido real del archivo, no con lo que imprime el driver. El detalle
// sin diff tampoco lo corre.
func TestHallazgo_43bde9_ElDiffDelDetalleNoEjecutaElTextconvDeLaConfigLocal(t *testing.T) {
	root := bdRepo(t, "")
	now := time.Now().UTC()
	const slug = "con-driver"
	bdItemArchivo(t, root, slug, "Con driver", "")
	wt := bdWorktree(t, root, slug)
	bdEscribir(t, wt, ".gitattributes", "*.txt diff=c2drv\n")
	bdEscribir(t, wt, "notas.txt", "linea real de la tarea\n")
	bdCommitear(t, wt, "notas con driver de diff")

	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marca := filepath.Join(fuera, "el-driver-corrio")
	const salida = "SALIDA-DEL-DRIVER-DE-DIFF"
	driver := bdEscribir(t, fuera, "driver.sh", "#!/bin/sh\necho corrio >> '"+marca+"'\necho "+salida+"\n")
	if err := os.Chmod(driver, 0o755); err != nil {
		t.Fatal(err)
	}
	bdGit(t, root, "config", "diff.c2drv.textconv", driver)
	// el fixture no es ciego: el git diff de siempre, en el espacio de trabajo, SI corre el driver
	if out := bdGit(t, wt, "diff", "--no-color", "--no-ext-diff", "main...HEAD", "--"); !strings.Contains(out, salida) {
		t.Fatalf("fixture: en esta maquina git diff corre el textconv de la config local y muestra su salida:\n%s", out)
	}
	if err := os.Remove(marca); err != nil {
		t.Fatalf("fixture: el driver deja su marca cuando corre: %v", err)
	}

	for _, conDiff := range []bool{false, true} {
		d, err := DetailFor(root, "main", "high", slug, now, conDiff)
		if err != nil {
			t.Fatal(err)
		}
		if raw, err := os.ReadFile(marca); err == nil {
			t.Fatalf("43bde9 CA-315: pedir el detalle (diff=%v) no ejecuta el textconv de la config local: corrio %d veces",
				conDiff, strings.Count(string(raw), "corrio"))
		}
		if !conDiff {
			continue
		}
		if d.Diff == nil || !d.Diff.Available || d.Diff.Note != "" {
			t.Fatalf("43bde9 CA-315: con un textconv configurado el diff del detalle sigue disponible y sin nota: %+v", d.Diff)
		}
		if strings.Contains(d.Diff.Patch, salida) || !strings.Contains(d.Diff.Patch, "+linea real de la tarea") {
			t.Fatalf("43bde9 CA-315: el parche trae el contenido real del archivo (+linea real de la tarea) y no la salida del driver:\n%s", d.Diff.Patch)
		}
	}
}

// ---------------------------------------------------------------------------
// 20261009T192554_12f52d: el nombre del gate se muestra como esta escrito
// ---------------------------------------------------------------------------

// c2Rojo es la tarjeta de bdEv con un veredicto rojo donde fallan esos gates
// requeridos (y build pasa).
func c2Rojo(fallan ...string) Card {
	gates := []verdict.GateResult{{Name: "build", Required: true, Status: verdict.StatusPass}}
	for _, g := range fallan {
		gates = append(gates, verdict.GateResult{Name: g, Required: true, Status: verdict.StatusFail})
	}
	ev := bdEv()
	ev.Verdict = bdVeredicto("v-rojo", bdT0.Add(40*time.Minute), "huella-1", 50, 10, gates)
	return Derive(ev)
}

// Hallazgo 20261009T192554_12f52d (enmienda 1: manda el nombre del gate).
// CA-306 y CA-308: un gate requerido llamado git-secrets que falla da, en
// plain y en red.plain, "la verificacion dio rojo: fallo el gate git-secrets":
// el nombre se muestra como el proyecto lo escribio en su hoom.yaml, aunque
// traiga una palabra que el modo normal no dice. Callarlo esconderia que
// fallo. Este test pasa desde antes de la enmienda: esta para que nadie
// "arregle" la excepcion filtrando o cambiando el nombre.
func TestHallazgo_12f52d_ElNombreDelGateSeMuestraComoElProyectoLoEscribio(t *testing.T) {
	for _, nombre := range []string{"git-secrets", "commit-lint", "git", "diff", "merge-check", "HEAD", "check-branch", "huella"} {
		c := c2Rojo(nombre)
		quiere := "la verificacion dio rojo: fallo el gate " + nombre
		if c.Column != ColWriter {
			t.Fatalf("12f52d CA-306: fixture: un veredicto rojo deja la tarjeta en writer, quedo en %s", c.Column)
		}
		if c.Plain != quiere {
			t.Errorf("12f52d CA-306: con el gate requerido %q en fail, plain es %q, fue %q", nombre, quiere, c.Plain)
		}
		if c.Red == nil || c.Red.Plain != quiere {
			t.Errorf("12f52d CA-308: con el gate requerido %q en fail, red.plain es %q, fue %+v", nombre, quiere, c.Red)
		}
	}
	// varios: cada nombre tal cual, en el orden del veredicto
	c := c2Rojo("git-secrets", "test")
	const varios = "la verificacion dio rojo: fallaron los gates git-secrets, test"
	if c.Plain != varios || c.Red == nil || c.Red.Plain != varios {
		t.Errorf("12f52d CA-306: con git-secrets y test en fail, plain y red.plain son %q; fueron %q y %+v", varios, c.Plain, c.Red)
	}
}

// Hallazgo 20261009T192554_12f52d (enmienda 1). CA-306 y CA-308: la excepcion
// es SOLO el nombre del gate. Con un gate de nombre comun, plain y red.plain
// siguen sin traer ninguna palabra prohibida, ni .hoom/ ni un comando; y con
// git-secrets, la frase sin el nombre del gate tambien cumple el vocabulario
// del modo normal.
func TestHallazgo_12f52d_LaExcepcionEsSoloElNombreDelGate(t *testing.T) {
	for _, nombre := range []string{"test", "build-web", "lint", "seguridad", "e2e"} {
		c := c2Rojo(nombre)
		if c.Red == nil {
			t.Fatalf("12f52d CA-308: fixture: el veredicto rojo es el rojo de la tarjeta (gate %s)", nombre)
		}
		if quiere := "la verificacion dio rojo: fallo el gate " + nombre; c.Plain != quiere || c.Red.Plain != quiere {
			t.Errorf("12f52d CA-306: con el gate %q en fail, plain y red.plain son %q; fueron %q y %q", nombre, quiere, c.Plain, c.Red.Plain)
		}
		tbNormal(t, "12f52d CA-306", "plain con el gate "+nombre, c.Plain)
		tbNormal(t, "12f52d CA-308", "red.plain con el gate "+nombre, c.Red.Plain)
	}
	c := c2Rojo("git-secrets")
	if c.Red == nil {
		t.Fatal("12f52d CA-308: fixture: el veredicto rojo es el rojo de la tarjeta")
	}
	for que, frase := range map[string]string{"plain": c.Plain, "red.plain": c.Red.Plain} {
		if !strings.Contains(frase, "git-secrets") {
			t.Fatalf("12f52d CA-306: %s nombra el gate git-secrets: %q", que, frase)
		}
		tbNormal(t, "12f52d CA-306", que+" sin el nombre del gate", strings.Replace(frase, "git-secrets", "<gate>", 1))
	}
}
