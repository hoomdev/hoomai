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
//
// Y de la review de esas correcciones (tarea fixes-cabina-c2), sobre
// .hoom/specs/historia-doctor-y-cinta.md:
//
//   - 20261009T204930_9fa7ac (medium, risk) y 20261009T205337_394505 (medium,
//     reliability), PRE-EXISTENTES; CA-360 y CA-363: la historia lee el
//     parche de los commits de la tarea para saber que tokens CA agrego cada
//     uno. Esa lectura no ejecuta un textconv de la configuracion local de
//     git, y los tokens salen del contenido real de los archivos de test, no
//     de lo que imprima un driver.
//   - 20261009T205855_d4d677 (low, resilience), PRE-EXISTENTE; CA-363 (y
//     CA-359, CA-360): si git falla mientras entrega el parche de los commits
//     de la tarea, la historia no sale como un exito con atribuciones a
//     medias: notes dice "sin historial de git: <error>". Y una linea muy
//     larga del parche no corta la lectura antes de los 8 MiB.
//
// Y de la review delta de esas correcciones, sobre la misma lectura del
// parche (el contrato: "si git falla, las entradas de git faltan y notes dice
// sin historial de git: <error>"; las entradas de git son TODAS las de source
// git, como lee TestCA359_SinHistorialDeGit):
//
//   - 20261010T042645_cc4f5b (medium, resilience): cuando git falla a mitad
//     del parche no queda NINGUNA entrada de source git (tampoco item ni
//     spec).
//   - 20261010T042641_dac517 (medium, resilience): si git falla DESPUES de
//     haber emitido mas de 8 MiB, es un fallo de git y no un corte.
//   - 20261010T042035_ad88b6 (medium, risk): el <error> de la nota sale del
//     stderr de git y esta acotado aunque git escriba megas ahi.
package boardcmd

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"os"
	"os/exec"
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

// ---------------------------------------------------------------------------
// 20261009T204930_9fa7ac y 20261009T205337_394505: la historia y el textconv
// ---------------------------------------------------------------------------

// c2DriverDice es lo que imprime el driver de diff de la historia en vez del
// contenido del archivo: cita CA-3, que ningun archivo de test cita de verdad.
const c2DriverDice = "// CA-3 segun el driver de diff"

// c2Historia es el fixture de la historia con un driver de diff: los shas de
// sus tres commits de la tarea y la marca que deja el driver si corre.
type c2Historia struct {
	root, marca         string
	atributos, uno, dos string
}

// c2HistoriaConDriver arma la tarjeta "precios" con su espacio de trabajo y
// tres commits de la tarea: (1) el spec (criterios CA-1, CA-2 y CA-3) y un
// .gitattributes que le pone el driver c2drv a *_test.go; (2) agrega
// precios_test.go, que cita CA-1; (3) lo cambia para citar tambien CA-2. La
// config LOCAL del repo define diff.c2drv.textconv con un script (fuera del
// proyecto) que deja una linea en marca cada vez que corre e imprime
// c2DriverDice. Confirma que el git log -p de siempre SI corre el driver en
// este repo, y borra la marca.
func c2HistoriaConDriver(t *testing.T) c2Historia {
	t.Helper()
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	f := c2Historia{root: root}
	bdEscribir(t, wt, bdSpec, hiSpecV1)
	bdEscribir(t, wt, ".gitattributes", "*_test.go diff=c2drv\n")
	f.atributos = hiCommit(t, wt, "spec y atributos de diff", h(1), "")
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n")
	f.uno = hiCommit(t, wt, "prueba del uno", h(2), "")
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n// CA-2: la vista muestra el precio\n")
	f.dos = hiCommit(t, wt, "prueba del dos", h(3), "")

	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f.marca = filepath.Join(fuera, "el-driver-corrio")
	driver := bdEscribir(t, fuera, "driver.sh", "#!/bin/sh\necho corrio >> '"+f.marca+"'\necho '"+c2DriverDice+"'\n")
	if err := os.Chmod(driver, 0o755); err != nil {
		t.Fatal(err)
	}
	bdGit(t, root, "config", "diff.c2drv.textconv", driver)
	if out := bdGit(t, wt, "log", "-p", "--no-ext-diff", "--format=%H", "main..HEAD", "--", "precios_test.go"); !strings.Contains(out, c2DriverDice) {
		t.Fatalf("fixture: en esta maquina git log -p corre el textconv de la config local y muestra su salida (sin eso el test no ve nada):\n%s", out)
	}
	if err := os.Remove(f.marca); err != nil {
		t.Fatalf("fixture: el driver deja su marca cuando corre: %v", err)
	}
	return f
}

// Hallazgo 20261009T204930_9fa7ac (PRE-EXISTENTE). historia-doctor-y-cinta
// CA-360 (los commits de la tarea, <base>..HEAD en el espacio de trabajo) y
// el contrato de TimelineFor ("los unicos comandos que corre son lecturas de
// git"): con un driver diff.<x>.textconv en la config local y un
// .gitattributes de la rama que se lo aplica al archivo de test que la tarea
// cambio, pedir la historia de la tarjeta no corre el comando del driver
// ninguna vez (el driver deja una marca fuera del proyecto si corre). La
// historia sale igual: sus tres commits de la tarea, sin error.
func TestHallazgo_9fa7ac_LaHistoriaNoEjecutaElTextconvDeLaConfigLocal(t *testing.T) {
	f := c2HistoriaConDriver(t)
	tl, err := TimelineFor(f.root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("9fa7ac CA-360: TimelineFor con un textconv configurado: %v", err)
	}
	if raw, err := os.ReadFile(f.marca); err == nil {
		t.Fatalf("9fa7ac CA-360: pedir la historia de la tarjeta no ejecuta el textconv de la config local: corrio %d veces",
			strings.Count(string(raw), "corrio"))
	}
	if got := hiCommitsDe(tl); len(got) != 3 || got[0] != f.atributos || got[1] != f.uno || got[2] != f.dos {
		t.Fatalf("9fa7ac CA-360: la historia trae los tres commits de la tarea, en orden: %v\n%s", hiCortos(got), hiListado(tl))
	}
}

// Hallazgo 20261009T205337_394505 (PRE-EXISTENTE). historia-doctor-y-cinta
// CA-363 (un commit de la tarea enciende los criterios cuyo token aparece por
// primera vez en una linea agregada de un archivo de test) sobre los commits
// de CA-360: con un textconv de la config local sobre el archivo de test, los
// tokens que la historia le atribuye a cada commit son los del contenido REAL
// del archivo, los mismos en cualquier maquina. El commit que agrega
// precios_test.go enciende CA-1 y el que lo cambia enciende CA-2; ninguno
// enciende CA-3, que es lo que imprimiria el driver.
func TestHallazgo_394505_LosTokensDeLaHistoriaSalenDelContenidoRealNoDelTextconv(t *testing.T) {
	f := c2HistoriaConDriver(t)
	tl, err := TimelineFor(f.root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("394505 CA-363: TimelineFor con un textconv configurado: %v", err)
	}
	var ids []string
	for _, s := range tl.Meter {
		ids = append(ids, s.ID)
	}
	if !strings.HasPrefix(strings.Join(ids, ","), "spec,CA-1,CA-2,CA-3,") {
		t.Fatalf("394505 CA-363: fixture: el esqueleto del medidor trae spec y los criterios CA-1, CA-2 y CA-3: %v", ids)
	}
	const ca = "394505 CA-363"
	hiEfectos(t, ca, "el commit que agrega precios_test.go (cita CA-1; el driver diria CA-3)",
		hiUna(t, ca, tl, KindCommit, f.uno, ""), map[string]string{"CA-1": SegHecho})
	hiEfectos(t, ca, "el commit que le agrega la linea de CA-2 a precios_test.go (con el driver no habria diferencia)",
		hiUna(t, ca, tl, KindCommit, f.dos, ""), map[string]string{"CA-2": SegHecho})
	hiEfectos(t, ca, "el commit del spec y de .gitattributes (ninguno es un archivo de test)",
		hiUna(t, ca, tl, KindCommit, f.atributos, ""), map[string]string{})
	for _, e := range tl.Entries {
		for _, m := range e.Meter {
			if m.ID == "CA-3" && m.State == SegHecho {
				t.Fatalf("394505 CA-363: ningun archivo de test cita CA-3 (solo lo imprime el driver): ninguna entrada lo enciende, lo encendio %s %s",
					e.Kind, hiCorto(e.Ref))
			}
		}
	}
	if hiTieneNota(tl, NoteTestsCortados) {
		t.Fatalf("394505 CA-363: un parche chico no se corta: %q", tl.Notes)
	}
}

// ---------------------------------------------------------------------------
// 20261009T205855_d4d677: el parche de la historia, cuando git falla o una
// linea es muy larga
// ---------------------------------------------------------------------------

// c2GitCrudo corre git en dir y devuelve stdout, stderr y el codigo de
// salida, sin juzgarlos.
func c2GitCrudo(t *testing.T, dir string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	code := 0
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("fixture: no se pudo correr git %v: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

// c2NotasSinGit son las notas de la historia que empiezan con "sin historial
// de git: ".
func c2NotasSinGit(tl Timeline) []string {
	var out []string
	for _, n := range tl.Notes {
		if strings.HasPrefix(n, NoteSinGitPrefijo) {
			out = append(out, n)
		}
	}
	return out
}

// c2EntradasDeGit lista las entradas de source git de la historia ("kind
// ref artifact"), para decir cuales quedaron.
func c2EntradasDeGit(tl Timeline) []string {
	var out []string
	for _, e := range tl.Entries {
		if e.Source == SourceGit {
			out = append(out, strings.TrimSpace(e.Kind+" "+hiCorto(e.Ref)+" "+e.Artifact))
		}
	}
	return out
}

// c2Fatal es la linea "fatal: ..." del stderr de git ("" si no hay).
func c2Fatal(stderr string) string {
	fatal := ""
	for _, linea := range strings.Split(stderr, "\n") {
		if strings.HasPrefix(linea, "fatal:") {
			fatal = strings.TrimSpace(linea)
		}
	}
	return fatal
}

// c2HistoriaConParcheRoto arma la tarjeta "precios" con su espacio de
// trabajo y tres commits de la tarea: el mas viejo agrega el spec y un
// archivo de test que cita CA-1, el del medio agrega un archivo con un driver
// de diff cuyo xfuncname es invalido en la config local, y el mas nuevo
// agrega otro archivo de test que cita CA-2. Confirma que en esta maquina git
// log -p de ese rango muere con "fatal:" despues de haber emitido parte del
// parche, y devuelve el proyecto y esas palabras de git.
func c2HistoriaConParcheRoto(t *testing.T) (root, fatal string) {
	t.Helper()
	root = bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, hiSpecV1)
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n")
	hiCommit(t, wt, "spec y prueba del uno", h(1), "")
	bdEscribir(t, wt, ".gitattributes", "z_malo.txt diff=malo\n")
	bdEscribir(t, wt, "z_malo.txt", "uno\ndos\n")
	hiCommit(t, wt, "archivo con driver de diff", h(2), "")
	bdEscribir(t, wt, "vista_test.go", "package app\n\n// CA-2: la vista muestra el precio\n")
	hiCommit(t, wt, "prueba del dos", h(3), "")
	bdGit(t, root, "config", "diff.malo.xfuncname", "([")

	out, stderr, code := c2GitCrudo(t, wt, "log", "-p", "--no-ext-diff", "--no-textconv", "main..HEAD")
	if code == 0 || !strings.Contains(stderr, "fatal:") || len(out) == 0 {
		t.Fatalf("fixture: en esta maquina git log -p main..HEAD sale no-cero con fatal: despues de haber emitido algo: exit=%d, %d bytes, stderr=%q",
			code, len(out), stderr)
	}
	return root, c2Fatal(stderr)
}

// Hallazgo 20261009T205855_d4d677 (PRE-EXISTENTE). historia-doctor-y-cinta
// CA-363 sobre los commits de la tarea de CA-360, y la regla del contrato "si
// git falla, las entradas de git faltan y notes dice sin historial de git:
// <error>" (CA-359). La tarea tiene tres commits: el mas viejo agrega un
// archivo de test que cita CA-1, el del medio agrega un archivo con un driver
// de diff cuyo xfuncname es invalido en la config local, y el mas nuevo
// agrega otro archivo de test que cita CA-2. git log -p de ese rango muere
// con "fatal:" al llegar al archivo del medio, despues de haber emitido
// parte del parche (el fixture lo confirma). La historia no puede salir como
// un exito con los criterios a medias: err es nil, notes trae UNA nota "sin
// historial de git: " con las palabras de git, no hay entradas de commit,
// ninguna entrada enciende un criterio y no va la nota del corte de 8 MiB.
// De las entradas de item, spec, aprobacion y veredicto este test no dice
// nada: eso lo fija TestHallazgo_cc4f5b_SiGitFallaAMitadDelParcheNoQuedaNingunaEntradaDeGit.
func TestHallazgo_d4d677_SiGitFallaAMitadDelParcheLaHistoriaLoDice(t *testing.T) {
	root, fatal := c2HistoriaConParcheRoto(t)

	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("d4d677 CA-359: si git falla la historia no es un error (lo dice notes): %v", err)
	}
	notas := c2NotasSinGit(tl)
	if len(notas) != 1 {
		t.Errorf("d4d677 CA-363: git murio a mitad del parche de los commits de la tarea: notes trae UNA nota %q<error>, trajo %d: %q",
			NoteSinGitPrefijo, len(notas), tl.Notes)
	} else if !strings.Contains(notas[0], fatal) {
		t.Errorf("d4d677 CA-363: la nota trae las palabras de git %q, fue %q", fatal, notas[0])
	}
	if commits := hiCommitsDe(tl); len(commits) != 0 {
		t.Errorf("d4d677 CA-360: si git falla las entradas de los commits de la tarea faltan, hubo %d: %v", len(commits), hiCortos(commits))
	}
	for _, e := range tl.Entries {
		for _, m := range e.Meter {
			if strings.HasPrefix(m.ID, "CA-") && m.State == SegHecho {
				t.Errorf("d4d677 CA-363: con el parche a medias no se sabe que commit agrego cada token: ninguna entrada enciende un criterio, y %s %s encendio %s",
					e.Kind, hiCorto(e.Ref), m.ID)
			}
		}
	}
	if hiTieneNota(tl, NoteTestsCortados) {
		t.Errorf("d4d677 CA-363: un git que falla no es un parche cortado en 8 MiB: no va la nota %q: %q", NoteTestsCortados, tl.Notes)
	}
}

// Hallazgo 20261009T205855_d4d677 (PRE-EXISTENTE). historia-doctor-y-cinta
// CA-363 ("el parche de los commits de la tarea se lee hasta 8 MiB") sobre
// los commits de CA-360: una linea muy larga no corta la lectura. La tarea
// tiene un commit viejo que agrega un archivo de test que cita CA-1 y uno mas
// nuevo que agrega un archivo que no es de test con una sola linea de 5 MiB.
// El parche entero queda bajo los 8 MiB y git sale 0 (el fixture lo
// confirma), asi que la historia se lee completa: los dos commits estan, el
// viejo enciende CA-1, y no hay nota de corte ni de "sin historial de git".
func TestHallazgo_d4d677_UnaLineaLargaNoCortaLaLecturaDelParche(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, hiSpecV1)
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n")
	viejo := hiCommit(t, wt, "spec y prueba del uno", h(1), "")
	bdEscribir(t, wt, "relleno.txt", strings.Repeat("x", 5<<20)+"\n")
	nuevo := hiCommit(t, wt, "una linea de 5 MiB", h(2), "")

	out, stderr, code := c2GitCrudo(t, wt, "log", "-p", "--no-ext-diff", "--no-textconv", "main..HEAD")
	if code != 0 || len(out) <= 5<<20 || len(out) >= TimelinePatchMax {
		t.Fatalf("fixture: git log -p main..HEAD sale 0 con un parche de mas de 5 MiB y menos de %d bytes: exit=%d, %d bytes, stderr=%q",
			TimelinePatchMax, code, len(out), stderr)
	}

	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("d4d677 CA-363: TimelineFor con una linea de 5 MiB en el parche: %v", err)
	}
	if got := hiCommitsDe(tl); len(got) != 2 || got[0] != viejo || got[1] != nuevo {
		t.Fatalf("d4d677 CA-360: la historia trae los dos commits de la tarea, en orden: %v\n%s", hiCortos(got), hiListado(tl))
	}
	const ca = "d4d677 CA-363"
	hiEfectos(t, ca, "el commit viejo, que agrega precios_test.go con CA-1 (el parche del commit mas nuevo trae una linea de 5 MiB y la lectura sigue)",
		hiUna(t, ca, tl, KindCommit, viejo, ""), map[string]string{"CA-1": SegHecho})
	hiEfectos(t, ca, "el commit de la linea de 5 MiB (no es un archivo de test)",
		hiUna(t, ca, tl, KindCommit, nuevo, ""), map[string]string{})
	if hiTieneNota(tl, NoteTestsCortados) {
		t.Fatalf("d4d677 CA-363: un parche de menos de 8 MiB no se corta: no va la nota %q: %q", NoteTestsCortados, tl.Notes)
	}
	if notas := c2NotasSinGit(tl); len(notas) != 0 {
		t.Fatalf("d4d677 CA-359: git no fallo: no va ninguna nota %q<error>: %q", NoteSinGitPrefijo, notas)
	}
}

// ---------------------------------------------------------------------------
// 20261010T042645_cc4f5b, 20261010T042641_dac517 y 20261010T042035_ad88b6: si
// git falla al entregar el parche, faltan TODAS las entradas de git
// ---------------------------------------------------------------------------

// Hallazgo 20261010T042645_cc4f5b. historia-doctor-y-cinta CA-359 ("si git
// falla, las entradas de git faltan y notes dice sin historial de git:
// <error>") sobre el parche de los commits de la tarea (CA-360, CA-363): con
// el mismo fixture del fallo a mitad del parche (el xfuncname invalido en el
// commit del medio), la historia no queda poblada a medias. NINGUNA entrada
// tiene source git: ni los commits, ni el item, ni el spec. err es nil y la
// nota "sin historial de git: " esta una vez. De las entradas de telemetria
// este test no dice nada.
func TestHallazgo_cc4f5b_SiGitFallaAMitadDelParcheNoQuedaNingunaEntradaDeGit(t *testing.T) {
	root, _ := c2HistoriaConParcheRoto(t)
	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("cc4f5b CA-359: si git falla la historia no es un error (lo dice notes): %v", err)
	}
	if notas := c2NotasSinGit(tl); len(notas) != 1 {
		t.Errorf("cc4f5b CA-359: git murio a mitad del parche: notes trae UNA nota %q<error>, trajo %d: %q", NoteSinGitPrefijo, len(notas), tl.Notes)
	}
	if quedan := c2EntradasDeGit(tl); len(quedan) != 0 {
		t.Errorf("cc4f5b CA-359: si git falla las entradas de git faltan, TODAS (item, spec, aprobacion, veredicto, hallazgo, resolucion, review y commit): quedaron %d: %q",
			len(quedan), quedan)
	}
}

// c2HistoriaGrande arma la tarjeta "precios" con tres commits de la tarea: el
// mas viejo agrega el spec y un archivo de test que cita CA-1, el del medio
// agrega z_malo.txt con el atributo diff=malo, y el mas nuevo agrega un
// archivo de 9 MiB en lineas cortas. git log -p emite primero el commit mas
// nuevo, asi que pasa los 8 MiB antes de llegar a z_malo.txt. Con roto, la
// config local define diff.malo.xfuncname invalido y git muere ahi, DESPUES
// de haber emitido mas de 8 MiB; sin roto, git sale 0. El mismo git log -p
// confirma las dos cosas. Devuelve el proyecto, los tres commits (del mas
// viejo al mas nuevo) y las palabras de git si fallo.
func c2HistoriaGrande(t *testing.T, roto bool) (root string, commits []string, fatal string) {
	t.Helper()
	root = bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, hiSpecV1)
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n")
	commits = append(commits, hiCommit(t, wt, "spec y prueba del uno", h(1), ""))
	bdEscribir(t, wt, ".gitattributes", "z_malo.txt diff=malo\n")
	bdEscribir(t, wt, "z_malo.txt", "uno\ndos\n")
	commits = append(commits, hiCommit(t, wt, "archivo con driver de diff", h(2), ""))
	linea := "relleno de prueba sin ningun criterio citado\n"
	bdEscribir(t, wt, "relleno.txt", strings.Repeat(linea, (TimelinePatchMax+(1<<20))/len(linea)))
	commits = append(commits, hiCommit(t, wt, "relleno grande", h(3), ""))
	if roto {
		bdGit(t, root, "config", "diff.malo.xfuncname", "([")
	}

	out, stderr, code := c2GitCrudo(t, wt, "log", "-p", "--no-ext-diff", "--no-textconv", "main..HEAD")
	if len(out) <= TimelinePatchMax {
		t.Fatalf("fixture: git log -p main..HEAD emite mas de %d bytes (8 MiB), emitio %d", TimelinePatchMax, len(out))
	}
	if roto && (code == 0 || !strings.Contains(stderr, "fatal:")) {
		t.Fatalf("fixture: con el xfuncname invalido git log -p sale no-cero con fatal: DESPUES de emitir %d bytes: exit=%d stderr=%q", len(out), code, stderr)
	}
	if !roto && code != 0 {
		t.Fatalf("fixture: sin el driver roto git log -p sale 0: exit=%d stderr=%q", code, stderr)
	}
	return root, commits, c2Fatal(stderr)
}

// Hallazgo 20261010T042641_dac517. historia-doctor-y-cinta CA-359 y CA-363:
// pasar los 8 MiB del parche no convierte un fallo de git en un corte. La
// tarea agrega un archivo de 9 MiB en su commit mas nuevo y, mas atras, un
// archivo con un xfuncname invalido: git log -p emite mas de 8 MiB y DESPUES
// muere con "fatal:" (el fixture confirma las dos cosas con el mismo git log
// -p). git fallo, asi que: err es nil, notes trae UNA nota "sin historial de
// git: " con las palabras de git, NO trae la nota del corte de 8 MiB, y no
// queda ninguna entrada de source git.
func TestHallazgo_dac517_SiGitFallaDespuesDeLos8MiBNoEsUnCorte(t *testing.T) {
	root, _, fatal := c2HistoriaGrande(t, true)
	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("dac517 CA-359: si git falla la historia no es un error (lo dice notes): %v", err)
	}
	notas := c2NotasSinGit(tl)
	if len(notas) != 1 {
		t.Errorf("dac517 CA-359: git murio despues de emitir mas de 8 MiB de parche: notes trae UNA nota %q<error>, trajo %d: %q",
			NoteSinGitPrefijo, len(notas), tl.Notes)
	} else if !strings.Contains(notas[0], fatal) {
		t.Errorf("dac517 CA-359: la nota trae las palabras de git %q, fue %q", fatal, notas[0])
	}
	if hiTieneNota(tl, NoteTestsCortados) {
		t.Errorf("dac517 CA-363: un git que falla despues de los 8 MiB no es un parche cortado: no va la nota %q: %q", NoteTestsCortados, tl.Notes)
	}
	if quedan := c2EntradasDeGit(tl); len(quedan) != 0 {
		t.Errorf("dac517 CA-359: si git falla las entradas de git faltan, todas: quedaron %d: %q", len(quedan), quedan)
	}
}

// Guarda del hallazgo 20261010T042641_dac517. CA-363 y CA-360: el mismo repo
// SIN el driver roto. El parche pasa los 8 MiB y git sale 0: eso si es un
// corte. notes trae la nota del corte, estan los tres commits de la tarea y
// no hay nota "sin historial de git". (TestCA363_ParcheDeMasDe8MiB ya fija la
// nota del corte; esta guarda es la gemela del test de arriba y agrega que
// los commits siguen y que cortar a proposito no se confunde con un fallo.)
func TestHallazgo_dac517_GuardaUnParcheSanoDeMasDe8MiBSigueSiendoUnCorte(t *testing.T) {
	root, commits, _ := c2HistoriaGrande(t, false)
	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("dac517 CA-363: TimelineFor con un parche sano de mas de 8 MiB: %v", err)
	}
	if !hiTieneNota(tl, NoteTestsCortados) {
		t.Fatalf("dac517 CA-363: con un parche de mas de 8 MiB y git en 0, notes dice %q: %q", NoteTestsCortados, tl.Notes)
	}
	if got := hiCommitsDe(tl); len(got) != 3 || got[0] != commits[0] || got[1] != commits[1] || got[2] != commits[2] {
		t.Fatalf("dac517 CA-360: cortar el parche no quita los commits de la tarea: deben ser %v, fueron %v\n%s", hiCortos(commits), hiCortos(got), hiListado(tl))
	}
	if notas := c2NotasSinGit(tl); len(notas) != 0 {
		t.Fatalf("dac517 CA-359: git no fallo (se corto la lectura a proposito): no va ninguna nota %q<error>: %q", NoteSinGitPrefijo, notas)
	}
}

// c2TopeDeLaNota: lo que puede medir, como mucho, la nota "sin historial de
// git: <error>" cuando git escribe megas en stderr.
const c2TopeDeLaNota = 128 << 10

// Hallazgo 20261010T042035_ad88b6. historia-doctor-y-cinta CA-359: el <error>
// de "sin historial de git: <error>" sale del stderr de git, y la rama
// controla cuanto escribe git ahi. El commit mas nuevo de la tarea trae un
// .gitattributes con 20000 lineas con un nombre de atributo invalido (git
// avisa "is not a valid attribute name" por cada una) y un archivo con un
// xfuncname invalido, para que ademas falle. El fixture mide el stderr real
// de git log -p de ese rango y exige que pase de 1 MiB. La historia no es un
// error, la nota esta, y mide menos de 128 KiB: lo que git escriba en stderr
// no se guarda ni se devuelve entero.
func TestHallazgo_ad88b6_ElErrorDeLaNotaEstaAcotadoAunqueGitEscribaMegasEnStderr(t *testing.T) {
	root := bdRepo(t, "")
	d0 := time.Date(2026, 9, 22, 6, 0, 0, 0, time.UTC)
	h := func(n int) time.Time { return d0.Add(time.Duration(n) * time.Hour) }
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "item", h(0), "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, hiSpecV1)
	bdEscribir(t, wt, "precios_test.go", "package app\n\n// CA-1: el precio sale por region\n")
	hiCommit(t, wt, "spec y prueba del uno", h(1), "")
	var atributos strings.Builder
	atributos.WriteString("z_malo.txt diff=malo\n")
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&atributos, "*.x%d in!valido%d\n", i, i)
	}
	bdEscribir(t, wt, ".gitattributes", atributos.String())
	bdEscribir(t, wt, "z_malo.txt", "uno\ndos\n")
	hiCommit(t, wt, "atributos invalidos y archivo con driver de diff", h(2), "")
	bdGit(t, root, "config", "diff.malo.xfuncname", "([")

	_, stderr, code := c2GitCrudo(t, wt, "log", "-p", "--no-ext-diff", "--no-textconv", "main..HEAD")
	if code == 0 || !strings.Contains(stderr, "fatal:") || len(stderr) <= 1<<20 {
		t.Fatalf("fixture: en esta maquina git log -p main..HEAD falla con fatal: y escribe mas de 1 MiB en stderr: exit=%d, stderr de %d bytes", code, len(stderr))
	}

	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Fatalf("ad88b6 CA-359: si git falla la historia no es un error (lo dice notes): %v", err)
	}
	notas := c2NotasSinGit(tl)
	if len(notas) != 1 {
		t.Fatalf("ad88b6 CA-359: git fallo: notes trae UNA nota %q<error>, trajo %d", NoteSinGitPrefijo, len(notas))
	}
	if strings.TrimSpace(strings.TrimPrefix(notas[0], NoteSinGitPrefijo)) == "" {
		t.Errorf("ad88b6 CA-359: la nota dice %q seguido del error de git, y vino sin error", NoteSinGitPrefijo)
	}
	if len(notas[0]) >= c2TopeDeLaNota {
		t.Errorf("ad88b6 CA-359: git escribio %d bytes en stderr: la nota %q<error> esta acotada (menos de %d bytes), mide %d",
			len(stderr), NoteSinGitPrefijo, c2TopeDeLaNota, len(notas[0]))
	}
}
