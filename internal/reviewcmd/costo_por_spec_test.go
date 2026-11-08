// Tests adversariales del spec .hoom/specs/costo-por-spec.md (CA-542): el
// registro de una review guarda `runs`, el run de cada lente, para que
// `hoom costo` pueda separar una ronda de otra. Escritos DESDE EL SPEC y las
// firmas publicas (Record.Runs, LensRun), sin leer la implementacion.
//
// Los CLIs de IA falsos salen de raInstalar (cliFalso): este archivo no arma
// ningun script a mano. CODEX_HOME apunta a un directorio vacio propio: la
// review nunca lee la sesion de un Codex real de esta maquina.
package reviewcmd

import (
	"reflect"
	"testing"

	"github.com/hoomdev/hoomai/internal/runcmd"
)

// cpsRunsDelRegistro exige que el registro diga, en orden, la lente y el run
// de cada pasada, y que cada run sea un sidecar del reviewer de esta review.
func cpsRunsDelRegistro(t *testing.T, root string, res Result, m map[string]any) {
	t.Helper()
	recs := raRegistros(t, root)
	if len(recs) != 1 || recs[0].ID != res.RecordID {
		t.Fatalf("CA-542: fixture: un registro, el de la review: %+v", recs)
	}
	rec := recs[0]
	if len(rec.Runs) != len(res.Passes) || len(rec.Runs) != len(rec.Lenses) {
		t.Fatalf("CA-542: runs tiene una entrada por lente (%d): %+v", len(res.Passes), rec.Runs)
	}
	vistos := map[string]bool{}
	for i, lr := range rec.Runs {
		p := res.Passes[i]
		if lr.Lens != p.Lens || lr.Lens != rec.Lenses[i] {
			t.Fatalf("CA-542: la entrada %d de runs es la lente %q, fue %+v", i, p.Lens, lr)
		}
		if lr.Run == "" || lr.Run != p.RunID {
			t.Fatalf("CA-542: la entrada %d de runs nombra el run de su pasada (%q): %+v", i, p.RunID, lr)
		}
		if vistos[lr.Run] {
			t.Fatalf("CA-542: cada lente tiene su propio run: %+v", rec.Runs)
		}
		vistos[lr.Run] = true
		meta, ok := runcmd.ReadMeta(root, lr.Run)
		if !ok || meta.ID != lr.Run || meta.Role != "reviewer" || meta.Provider != res.Provider {
			t.Fatalf("CA-542: el run %s de la lente %s es un sidecar del reviewer (%s): %+v %v", lr.Run, lr.Lens, res.Provider, meta, ok)
		}
	}
	crudo, ok := m["runs"].([]any)
	if !ok || len(crudo) != len(rec.Runs) {
		t.Fatalf("CA-542: el JSON del registro trae runs como lista: %v", m["runs"])
	}
	for i, e := range crudo {
		obj, _ := e.(map[string]any)
		quiero := map[string]any{"lens": rec.Runs[i].Lens, "run": rec.Runs[i].Run}
		if !reflect.DeepEqual(obj, quiero) {
			t.Fatalf("CA-542: cada entrada de runs es {lens, run}: %v, fue %v", quiero, obj)
		}
	}
}

// CA-542: una review de 4 lentes deja en su registro las 4 entradas
// {lens, run}, en el orden en que corrieron (risk primero).
func TestCA542_ElRegistroGuardaElRunDeCadaLente(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	raInstalar(t, bin, "codex", raUsoCodex)
	res, out := raRevisar(t, "CA-542", root, Options{Provider: "codex"})
	if res.Status != "revisado" || len(res.Passes) != 4 || res.RecordID == "" {
		t.Fatalf("CA-542: fixture: las 4 lentes corren y la review deja registro: %+v\n%s", res, out)
	}
	cpsRunsDelRegistro(t, root, res, raRegistroCrudo(t, root, res.RecordID))
	recs := raRegistros(t, root)
	if recs[0].Runs[0].Lens != "risk" {
		t.Fatalf("CA-542: risk corre primero y es la primera entrada: %+v", recs[0].Runs)
	}
}

// CA-542: con una lente, runs tiene una entrada; las pasadas que no
// informaron consumo tambien nombran su run (el costo se gasto igual).
func TestCA542_UnaLenteSinConsumoTambienNombraSuRun(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	raInstalar(t, bin, "codex", "")
	res, out := raRevisar(t, "CA-542", root, Options{Provider: "codex", Lens: "risk"})
	if res.Status != "revisado" || len(res.Passes) != 1 || res.RecordID == "" {
		t.Fatalf("CA-542: fixture: una lente y registro: %+v\n%s", res, out)
	}
	cpsRunsDelRegistro(t, root, res, raRegistroCrudo(t, root, res.RecordID))
}

// CA-542: solo las pasadas pares informan consumo; runs igual nombra las 4
// (usage tiene 2 entradas, runs 4: no son la misma lista).
func TestCA542_RunsNoDependeDelConsumo(t *testing.T) {
	t.Setenv("CODEX_HOME", t.TempDir())
	bin := raPATH(t)
	root := raRepo(t, "")
	raCuatro(t, root)
	raInstalar(t, bin, "codex", raUsoPares)
	res, out := raRevisar(t, "CA-542", root, Options{Provider: "codex"})
	if res.Status != "revisado" || len(res.Passes) != 4 {
		t.Fatalf("CA-542: fixture: %+v\n%s", res, out)
	}
	m := raRegistroCrudo(t, root, res.RecordID)
	cpsRunsDelRegistro(t, root, res, m)
	if u, _ := m["usage"].([]any); len(u) != 2 {
		t.Fatalf("CA-542: fixture: usage sigue con una entrada por pasada con datos (2): %v", m["usage"])
	}
}

// CA-542 y CA-546: un registro viejo, sin runs, se sigue leyendo sin avisos
// y con Runs vacio (es la ronda que `hoom costo` muestra como reviewer sin
// ronda).
func TestCA542_RegistroViejoSinRunsSeSigueLeyendo(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, ".hoom/reviews/20261001T100000_v1ej0b.json", `{
  "id": "20261001T100000_v1ej0b",
  "created_at": "2026-10-01T10:00:00Z",
  "task": "precios",
  "spec": ".hoom/specs/precios.md",
  "fingerprint": "abc",
  "verdict_id": "v1",
  "verdict": "green",
  "lenses": ["risk"],
  "provider": "codex",
  "writer": "claude",
  "cross": "cruzada",
  "writers_declared": [],
  "findings": [],
  "usage": []
}
`)
	recs, avisos := Records(dir)
	if len(avisos) != 0 || len(recs) != 1 || len(recs[0].Runs) != 0 {
		t.Fatalf("CA-542: un registro sin runs se lee sin avisos y sin runs: %v %+v", avisos, recs)
	}
}
