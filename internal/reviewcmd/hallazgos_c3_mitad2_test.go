// Tests de regresion de hallazgos de la mitad 2 de la review de C3
// (.hoom/specs/acciones-desde-la-tarjeta.md) sobre `hoom review`, escritos
// desde el contrato y los hallazgos, sin leer la implementacion:
//
//   - 20261014T203056_9e2b00 (high) y 20261014T203828_5d64ca (medium),
//     CA-351: un item de la tarea que EXISTE pero no se puede leer (una clave
//     desconocida, YAML roto, sin permiso de lectura) no equivale a "sin
//     sesiones declaradas": la review falla con error antes de la primera
//     pasada (fail closed), el provider no corre y no queda registro de
//     review ni registro de sobre. Solo un item que no existe equivale a [].
//   - 20261014T203819_ccf0ab (low), CA-334: "lo que se decide antes de la
//     primera pasada ... no escribe registro". Un provider que no toma lo
//     pedido (codex con un tope de turnos o un presupuesto en USD) se decide
//     antes de la primera pasada: error, sin registro de sobre, sin pasadas y
//     sin llamar a Started.
//
// Reusa los ayudantes de los tests de CA-334/CA-335 y CA-351
// (sobre_review_test.go, writer_declarado_test.go): cbRepo, cbTarea, cbMeta,
// cbItem, cbMetasEn, cbStarted, fakeProvider.
package reviewcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// m2Corrio dice si el provider falso que deja la marca corrio.
func m2Corrio(marca string) bool {
	_, err := os.Stat(marca)
	return err == nil
}

// m2SinRegistros exige que no haya registro de review ni de sobre, ni en el
// proyecto ni en el arbol revisado.
func m2SinRegistros(t *testing.T, hallazgo, root, wt string) {
	t.Helper()
	for _, dir := range []string{root, wt} {
		if recs := envelope.List(dir); len(recs) != 0 {
			t.Fatalf("hallazgo %s: una review decidida antes de la primera pasada no deja registro de sobre (en %s): %+v", hallazgo, dir, recs)
		}
		entradas, err := os.ReadDir(filepath.Join(dir, ".hoom", RecordsDir))
		if err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		if len(entradas) != 0 {
			t.Fatalf("hallazgo %s: una review decidida antes de la primera pasada no deja registro de review (en %s): %d archivos", hallazgo, dir, len(entradas))
		}
	}
}

// m2Revision arma la tarea precios con un writer observado (claude) y un
// codex falso que deja una marca si corre.
func m2Revision(t *testing.T) (root, wt, marca string) {
	t.Helper()
	root = cbRepo(t)
	wt = cbTarea(t, root, "precios")
	cbMeta(t, root, wt, "20261014T203056_m2w001", "claude", "writer")
	marca = filepath.Join(t.TempDir(), "codex-corrio")
	fakeProvider(t, "codex", "touch '"+marca+"'\nexit 0\n")
	return root, wt, marca
}

// m2ItemConSesionDeCodex es el cuerpo de un item VALIDO que declara una
// sesion de codex: con el reviewer codex, la review seria no-cruzada.
const m2ItemConSesionDeCodex = "titulo: Precios por region\ntipo: feature\nprioridad: media\n" +
	"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-20T10:00:00Z\n" +
	"sesiones:\n" +
	"  - provider: codex\n" +
	"    abierta_por: \"hoom test <test@hoom.dev>\"\n" +
	"    abierta_en: 2026-09-23T18:00:00Z\n"

// Hallazgos 20261014T203056_9e2b00 (high) y 20261014T203828_5d64ca (medium),
// CA-351: el item de la tarea existe y declara una sesion de codex, pero no
// se puede leer (una clave desconocida, YAML roto o sin permiso de lectura).
// Revisando con codex, si el item ilegible contara como "sin sesiones", la
// review correria como cruzada con el mismo provider que escribio. Tiene que
// fallar con error ANTES de la primera pasada: el provider no corre, ningun
// run se registra, Started no se llama y no queda registro de review ni de
// sobre.
func TestHallazgo_9e2b00_5d64ca_CA351_ItemIlegibleFrenaLaReview(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		cuerpo  string
		sinLeer bool
	}{
		{"clave-desconocida", m2ItemConSesionDeCodex + "clave_rara: si\n", false},
		{"yaml-roto", m2ItemConSesionDeCodex + "titulo: [sin cerrar\n", false},
		{"sin-permiso-de-lectura", m2ItemConSesionDeCodex, true},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			if c.sinLeer && os.Geteuid() == 0 {
				t.Skip("como root el permiso de lectura no frena a nadie")
			}
			root, wt, marca := m2Revision(t)
			item := filepath.Join(root, ".hoom", "items", "precios.yaml")
			write(t, root, filepath.Join(".hoom", "items", "precios.yaml"), c.cuerpo)
			if c.sinLeer {
				if err := os.Chmod(item, 0); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.Chmod(item, 0o644) })
			}
			metas := cbMetasEn(root)
			s := &cbStarted{root: root, id: "20261014T203056_m2rev1"}

			var out strings.Builder
			res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "codex",
				EnvelopeID: s.id, Started: s.fn}, &out)
			if m2Corrio(marca) {
				t.Fatalf("hallazgos 9e2b00/5d64ca (CA-351): con el item de la tarea ilegible (%s) el provider no corre: %+v %v\n%s", c.nombre, res, err, out.String())
			}
			if err == nil {
				t.Fatalf("hallazgos 9e2b00/5d64ca (CA-351): con el item de la tarea ilegible (%s) la review falla con error antes de la primera pasada; devolvio %+v\n%s", c.nombre, res, out.String())
			}
			if len(res.Passes) != 0 || cbMetasEn(root) != metas {
				t.Fatalf("hallazgos 9e2b00/5d64ca (CA-351): con el item ilegible no hay pasadas ni runs: %+v (runs %d -> %d)", res.Passes, metas, cbMetasEn(root))
			}
			if calls, _, _, _ := s.snapshot(); calls != 0 {
				t.Fatalf("hallazgos 9e2b00/5d64ca (CA-351): con el item ilegible Started no se llama (llamadas: %d)", calls)
			}
			m2SinRegistros(t, "9e2b00/5d64ca (CA-351)", root, wt)
		})
	}
}

// Hallazgos 20261014T203056_9e2b00 y 20261014T203828_5d64ca, CA-351
// (guarda): sin item (no existe) la review sigue: corre una pasada con codex
// y termina revisada.
func TestHallazgo_9e2b00_5d64ca_CA351_GuardaSinItemLaReviewSigue(t *testing.T) {
	root, _, marca := m2Revision(t)
	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "codex"}, &strings.Builder{})
	if err != nil {
		t.Fatalf("hallazgos 9e2b00/5d64ca (CA-351): guarda: sin item la review sigue: %v", err)
	}
	if res.Status != "revisado" || len(res.Passes) != 1 || !m2Corrio(marca) {
		t.Fatalf("hallazgos 9e2b00/5d64ca (CA-351): guarda: sin item la review corre su pasada y termina revisada: %+v (corrio: %v)", res, m2Corrio(marca))
	}
}

// Hallazgo 20261014T203819_ccf0ab (low), CA-334: un provider que no toma lo
// pedido (codex con un tope de turnos o con un presupuesto en USD) se decide
// antes de la primera pasada: la review falla con error, el provider no
// corre, ningun run se registra, Started no se llama y no queda registro de
// sobre en .hoom/envelopes/ (ni de review).
func TestHallazgo_ccf0ab_CA334_ProviderQueNoTomaLoPedidoNoDejaRegistro(t *testing.T) {
	for _, c := range []struct {
		nombre string
		pedir  func(*Options)
	}{
		{"tope-de-turnos", func(o *Options) { o.MaxTurns = 3 }},
		{"presupuesto-usd", func(o *Options) { o.BudgetUSD = 1.5 }},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root, wt, marca := m2Revision(t)
			metas := cbMetasEn(root)
			s := &cbStarted{root: root, id: "20261014T203819_m2rev2"}
			opt := Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "codex",
				EnvelopeID: s.id, Started: s.fn}
			c.pedir(&opt)

			var out strings.Builder
			res, err := Run(root, "main", opt, &out)
			if err == nil {
				t.Fatalf("hallazgo ccf0ab (CA-334): codex no toma %s: la review se decide con error antes de la primera pasada; devolvio %+v\n%s", c.nombre, res, out.String())
			}
			if m2Corrio(marca) || len(res.Passes) != 0 || cbMetasEn(root) != metas {
				t.Fatalf("hallazgo ccf0ab (CA-334): con %s no hay pasadas: el provider no corre y no se registra ningun run: %+v (corrio: %v, runs %d -> %d)", c.nombre, res.Passes, m2Corrio(marca), metas, cbMetasEn(root))
			}
			if calls, _, _, _ := s.snapshot(); calls != 0 {
				t.Fatalf("hallazgo ccf0ab (CA-334): con %s Started no se llama (llamadas: %d)", c.nombre, calls)
			}
			m2SinRegistros(t, "ccf0ab (CA-334)", root, wt)
		})
	}
}

// Hallazgo 20261014T203819_ccf0ab, CA-334 (guarda): la misma review con
// codex sin tope de turnos ni presupuesto corre y deja su registro de sobre.
func TestHallazgo_ccf0ab_CA334_GuardaSinPedidoQueNoTomaCorre(t *testing.T) {
	root, _, marca := m2Revision(t)
	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "codex"}, &strings.Builder{})
	if err != nil {
		t.Fatalf("hallazgo ccf0ab (CA-334): guarda: %v", err)
	}
	if res.Status != "revisado" || len(res.Passes) != 1 || !m2Corrio(marca) {
		t.Fatalf("hallazgo ccf0ab (CA-334): guarda: sin tope ni presupuesto la review corre: %+v (corrio: %v)", res, m2Corrio(marca))
	}
	if recs := envelope.List(root); len(recs) != 1 {
		t.Fatalf("hallazgo ccf0ab (CA-334): guarda: la review que corre deja su registro de sobre: %+v", recs)
	}
}
