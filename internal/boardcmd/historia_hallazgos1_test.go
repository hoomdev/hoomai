// Tests de los hallazgos de la review de historia-en-el-tablero (registro
// 20261023T132035_0607fa) sobre el tablero: Candidatas (lo que `hoom item
// backfill` haria con cada tarea) y la regla de derivacion de las tarjetas
// historicas (Gather, Build, Doctor). Escritos desde el spec
// .hoom/specs/historia-en-el-tablero.md y los textos de los hallazgos, sin
// leer la implementacion. Repos git reales en t.TempDir(); veredictos,
// hallazgos, specs e items escritos a mano. Sin ningun CLI de IA. Todo
// identificador de este archivo lleva el prefijo h1.
package boardcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
)

// h1Ilegible es un archivo de hallazgo que no se puede leer (JSON roto): su
// tarea y su severidad son desconocidas.
const h1Ilegible = "20261023T000000_dead00"

// h1Build deriva el tablero y el doctor del proyecto.
func h1Build(t *testing.T, ca, root string) (Board, DoctorReport) {
	t.Helper()
	b, err := Build(root, "main", "high", htAhora)
	if err != nil {
		t.Fatalf("%s: Build: %v", ca, err)
	}
	r, err := Doctor(root, "main", "high", htAhora)
	if err != nil {
		t.Fatalf("%s: Doctor: %v", ca, err)
	}
	return b, r
}

// h1SeSostiene exige que la historica slug se sostenga: Gather lo lee, la
// tarjeta esta en Hecho con la marca y ningun doctor da historica-sin-sosten.
func h1SeSostiene(t *testing.T, ca, root, slug string) {
	t.Helper()
	b, r := h1Build(t, ca, root)
	c := bdCard(t, b, slug)
	ev := Gather(root, "main", "high", c.Item, htAhora)
	if !ev.HistoricaSostenida || ev.HistoricaMotivo != "" {
		t.Fatalf("%s: la evidencia de %s (%s) la sostiene: sostenida=%v motivo=%q", ca, slug, c.Item.Historico,
			ev.HistoricaSostenida, ev.HistoricaMotivo)
	}
	if c.Column != ColHecho || !c.Historica {
		t.Fatalf("%s: la historica %s que se sostiene esta en Hecho con la marca; fue %s historica=%v", ca, slug, c.Column, c.Historica)
	}
	for _, p := range append(append([]Problem{}, c.Doctor...), r.Problems...) {
		if p.ID == ProbHistoricaSinSosten && p.Slug == slug {
			t.Fatalf("%s: la historica %s que se sostiene no da historica-sin-sosten: %+v", ca, slug, p)
		}
	}
}

// h1NoSeSostiene exige que la historica slug NO se sostenga (CA-452): Gather
// lo lee con su motivo, la tarjeta no esta en Hecho ni lleva la marca, y el
// doctor de la tarjeta y el del proyecto dan UN historica-sin-sosten con su
// accion; si ref no es "", el problema nombra esa evidencia.
func h1NoSeSostiene(t *testing.T, ca, root, slug, ref string) {
	t.Helper()
	b, r := h1Build(t, ca, root)
	c := bdCard(t, b, slug)
	ev := Gather(root, "main", "high", c.Item, htAhora)
	if ev.HistoricaSostenida || ev.HistoricaMotivo == "" {
		t.Fatalf("%s: la evidencia de %s (%s) ya no la sostiene, y Gather dice por que: sostenida=%v motivo=%q",
			ca, slug, c.Item.Historico, ev.HistoricaSostenida, ev.HistoricaMotivo)
	}
	if c.Column == ColHecho || c.Historica {
		t.Fatalf("%s: la historica %s que no se sostiene deja Hecho y la marca; fue %s historica=%v", ca, slug, c.Column, c.Historica)
	}
	var enTarjeta *Problem
	for i := range c.Doctor {
		if c.Doctor[i].ID == ProbHistoricaSinSosten {
			enTarjeta = &c.Doctor[i]
		}
	}
	if enTarjeta == nil || enTarjeta.Action == "" || enTarjeta.What == "" || enTarjeta.Slug != slug {
		t.Fatalf("%s: la tarjeta %s trae historica-sin-sosten con su accion: %s", ca, slug, drFmt(c.Doctor))
	}
	if ref != "" {
		htMenciona(t, ca, *enTarjeta, ref)
	}
	n := 0
	for _, p := range r.Problems {
		if p.ID == ProbHistoricaSinSosten && p.Slug == slug {
			n++
			if p.Action == "" {
				t.Fatalf("%s: historica-sin-sosten de %s trae su accion: %+v", ca, slug, p)
			}
		}
	}
	if n != 1 {
		t.Fatalf("%s: el doctor del proyecto da UN historica-sin-sosten de %s, dio %d:\n%s", ca, slug, n, drFmt(r.Problems))
	}
}

// h1Severidad reescribe a mano la severidad del hallazgo id en el JSON de
// dir, como llegaria por git sin pasar por Register (que la valida).
func h1Severidad(t *testing.T, dir, id, sev string) {
	t.Helper()
	p := filepath.Join(dir, ".hoom", "findings", id+".json")
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("precondicion: el hallazgo %s vive en %s: %v", id, p, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil || m["severity"] == nil {
		t.Fatalf("precondicion: el JSON del hallazgo trae \"severity\": %v %s", err, raw)
	}
	m["severity"] = sev
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(out, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// h1Borrar borra los archivos del hallazgo id (el hallazgo y su resolucion)
// en dir, como si se revirtieran en git.
func h1Borrar(t *testing.T, dir, id string) {
	t.Helper()
	p := filepath.Join(dir, ".hoom", "findings", id+".json")
	if err := os.Remove(p); err != nil {
		t.Fatalf("precondicion: el hallazgo %s vive en %s: %v", id, p, err)
	}
	if err := os.Remove(filepath.Join(dir, ".hoom", "findings", id+".res.json")); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

// ---------------------------------------------------------------------------
// 639517 / 1bd803: un veredicto de OTRA tarea no sostiene la historica.
// ---------------------------------------------------------------------------

// CA-451, CA-452, hallazgos 639517 y 1bd803: una historica "veredicto <id>"
// se sostiene solo si ese veredicto esta atado a SU spec
// (.hoom/specs/<slug>.md). Un item que apunta (por copia o por una mala
// fusion) al verde aprobado e integrado de otra tarea no se sostiene: no queda
// en Hecho y el doctor da historica-sin-sosten nombrando el veredicto. Se
// mira con el veredicto ajeno en root (item sin espacio de trabajo), en el
// espacio de trabajo de la tarea, y en root con la tarea en su espacio (la
// ruta de respaldo al arbol raiz). El control: el mismo veredicto sostiene a
// la tarjeta de su propia tarea.
func TestCA452_Hallazgo639517_VeredictoDeOtraTareaNoSostiene(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	for _, s := range []string{"otra", "mia-root", "mia-wt", "mia-respaldo"} {
		htSpec(t, root, s, "# Spec: "+s)
		bdAprobar(t, root, s)
	}
	bdCommitear(t, root, "specs")
	vOtra := htVeredicto(t, root, root, "otra", bdT0, false, bdGatesVerdes(), nil)
	htForma(t, vOtra, true, false)
	bdEscribir(t, root, ".hoom/items/otra.yaml", htItemYAML("Otra", "feature", item.HistoricoVeredicto(vOtra.ID)))
	bdEscribir(t, root, ".hoom/items/mia-root.yaml", htItemYAML("Mia root", "feature", item.HistoricoVeredicto(vOtra.ID)))
	bdEscribir(t, root, ".hoom/items/mia-respaldo.yaml", htItemYAML("Mia respaldo", "feature", item.HistoricoVeredicto(vOtra.ID)))
	bdCommitear(t, root, "veredicto de otra e items")

	// mia-wt: en su espacio de trabajo hay un verde de OTRA tarea, aprobado y
	// con el commit de main (integrado)
	wt := bdWorktree(t, root, "mia-wt")
	vEnWt := htVeredicto(t, wt, wt, "otra", bdT0.Add(time.Hour), false, bdGatesVerdes(), nil)
	htForma(t, vEnWt, true, false)
	if vEnWt.ID == vOtra.ID {
		t.Fatalf("precondicion: los dos veredictos de otra tienen ids distintos: %s", vEnWt.ID)
	}
	bdEscribir(t, root, ".hoom/items/mia-wt.yaml", htItemYAML("Mia wt", "feature", item.HistoricoVeredicto(vEnWt.ID)))
	// mia-respaldo: tiene su espacio de trabajo, pero el veredicto que nombra
	// solo esta en root
	bdWorktree(t, root, "mia-respaldo")

	// el control: el veredicto de otra sostiene a otra
	h1SeSostiene(t, "CA-451 (control)", root, "otra")

	t.Run("veredicto ajeno en root", func(t *testing.T) {
		h1NoSeSostiene(t, "CA-452 (639517)", root, "mia-root", vOtra.ID)
	})
	t.Run("veredicto ajeno en el espacio de trabajo de la tarea", func(t *testing.T) {
		h1NoSeSostiene(t, "CA-452 (1bd803)", root, "mia-wt", vEnWt.ID)
	})
	t.Run("veredicto ajeno en root con la tarea en su espacio", func(t *testing.T) {
		h1NoSeSostiene(t, "CA-452 (1bd803)", root, "mia-respaldo", vOtra.ID)
	})
}

// ---------------------------------------------------------------------------
// 6d0cce / 6fdcc5 / a28967: "historico: hallazgos" mira root Y el espacio de
// trabajo de la tarea, y exige al menos un hallazgo legible.
// ---------------------------------------------------------------------------

// CA-448, CA-452, hallazgos a28967 y 6fdcc5: una historica "hallazgos" se
// sostiene mientras exista al menos un hallazgo de su tarea (la condicion con
// la que califico). Si se borran TODOS los hallazgos de la tarea, su
// evidencia desaparecio, igual que un veredicto borrado: deja Hecho y el
// doctor da historica-sin-sosten. El control: la de al lado, con sus
// hallazgos cerrados, sigue en Hecho.
func TestCA452_Hallazgoa28967_SinNingunHallazgoNoSeSostiene(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	var borrar []string
	for _, sev := range []string{"high", "medium"} {
		id := htHallazgo(t, root, "borrada", sev)
		htCerrar(t, root, id, finding.StatusCorrected)
		borrar = append(borrar, id)
	}
	htCerrar(t, root, htHallazgo(t, root, "sana", "high"), finding.StatusRefuted)
	bdEscribir(t, root, ".hoom/items/borrada.yaml", htItemYAML("borrada", "bug", item.HistoricoHallazgos))
	bdEscribir(t, root, ".hoom/items/sana.yaml", htItemYAML("sana", "bug", item.HistoricoHallazgos))
	bdCommitear(t, root, "hallazgos e items historicos")
	h1SeSostiene(t, "CA-451 (precondicion)", root, "borrada")

	for _, id := range borrar {
		h1Borrar(t, root, id)
	}
	bdCommitear(t, root, "se revierten los hallazgos de borrada")
	h1NoSeSostiene(t, "CA-452 (a28967)", root, "borrada", "")
	h1SeSostiene(t, "CA-451 (control)", root, "sana")
}

// h1Correccion arma una tarea de correccion "corr" con su item historico
// "hallazgos" y su espacio de trabajo. Devuelve root y el espacio.
func h1Correccion(t *testing.T) (string, string) {
	t.Helper()
	root := bdRepo(t, htBlockOn)
	bdEscribir(t, root, ".hoom/items/corr.yaml", htItemYAML("corr", "bug", item.HistoricoHallazgos))
	bdCommitear(t, root, "item historico de corr")
	return root, bdWorktree(t, root, "corr")
}

// CA-448, CA-452, hallazgos 6d0cce y 6fdcc5: la historica "hallazgos" mira
// los hallazgos de su tarea en root Y en su espacio de trabajo. Con el item
// en su espacio (donde sus hallazgos estan cerrados), un high abierto de la
// tarea en root la saca de Hecho; y al reves, con los cerrados en root y el
// high abierto en el espacio, tampoco se sostiene. Los dos controles: todos
// cerrados en los dos arboles se sostiene, y cerrados solo en root con el
// item en su espacio (sin ningun hallazgo alli) tambien.
func TestCA452_Hallazgo6d0cce_HallazgosDeRootYDelEspacio(t *testing.T) {
	t.Run("high abierto en root con el item en su espacio", func(t *testing.T) {
		root, wt := h1Correccion(t)
		htCerrar(t, wt, htHallazgo(t, wt, "corr", "high"), finding.StatusCorrected)
		bdCommitear(t, wt, "corr: hallazgo cerrado en su espacio")
		h1SeSostiene(t, "CA-451 (precondicion)", root, "corr")
		alto := htHallazgo(t, root, "corr", "high")
		h1NoSeSostiene(t, "CA-452 (6d0cce)", root, "corr", alto)
	})
	t.Run("al reves: cerrados en root y high abierto en el espacio", func(t *testing.T) {
		root, wt := h1Correccion(t)
		htCerrar(t, root, htHallazgo(t, root, "corr", "high"), finding.StatusCorrected)
		h1SeSostiene(t, "CA-451 (precondicion)", root, "corr")
		alto := htHallazgo(t, wt, "corr", "high")
		h1NoSeSostiene(t, "CA-452 (6d0cce)", root, "corr", alto)
	})
	t.Run("control: todos cerrados en los dos arboles", func(t *testing.T) {
		root, wt := h1Correccion(t)
		htCerrar(t, root, htHallazgo(t, root, "corr", "high"), finding.StatusRefuted)
		htCerrar(t, wt, htHallazgo(t, wt, "corr", "medium"), finding.StatusCorrected)
		htHallazgo(t, wt, "corr", "low") // un seguimiento por debajo de block_on
		h1SeSostiene(t, "CA-451", root, "corr")
	})
}

// ---------------------------------------------------------------------------
// 1d6b30 / 6fdcc5: un hallazgo ilegible.
// ---------------------------------------------------------------------------

// CA-450, CA-452, hallazgos 1d6b30 y 6fdcc5: con un hallazgo ilegible en el
// arbol (su tarea y su severidad son desconocidas, y el gate findings_open
// falla cerrado ante el), la historica "hallazgos" no puede demostrar que sus
// bloqueantes estan cerrados: aunque su unico hallazgo legible este cerrado,
// no se sostiene, y el doctor nombra el archivo ilegible. Candidatas tampoco
// la hace calificar, y su motivo nombra el archivo.
func TestCA452_Hallazgo1d6b30_HallazgoIlegibleNoSostiene(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	htCerrar(t, root, htHallazgo(t, root, "corr", "high"), finding.StatusCorrected)
	htCerrar(t, root, htHallazgo(t, root, "nueva", "high"), finding.StatusCorrected)
	bdEscribir(t, root, ".hoom/items/corr.yaml", htItemYAML("corr", "bug", item.HistoricoHallazgos))
	bdCommitear(t, root, "hallazgos cerrados")
	h1SeSostiene(t, "CA-451 (precondicion)", root, "corr")
	htCalifica(t, "CA-448 (precondicion)", htCandidatas(t, "CA-448", root, "high"), "nueva", item.HistoricoHallazgos, "bug", "nueva")

	bdEscribir(t, root, ".hoom/findings/"+h1Ilegible+".json", "{esto no es un hallazgo")
	bdCommitear(t, root, "un hallazgo ilegible")
	if ev := Gather(root, "main", "high", bdCard(t, h1Tablero(t, root), "corr").Item, htAhora); len(ev.UnreadableFindings) == 0 {
		t.Fatalf("precondicion: el tablero registra el hallazgo ilegible: %+v", ev.UnreadableFindings)
	}

	t.Run("CA-452 la historica existente no se sostiene", func(t *testing.T) {
		h1NoSeSostiene(t, "CA-452 (1d6b30)", root, "corr", h1Ilegible)
	})
	t.Run("CA-450 Candidatas no la hace calificar", func(t *testing.T) {
		htNoCalifica(t, "CA-450 (1d6b30)", htCandidatas(t, "CA-450", root, "high"), "nueva", h1Ilegible)
	})
}

func h1Tablero(t *testing.T, root string) Board {
	t.Helper()
	b, err := Build(root, "main", "high", htAhora)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ---------------------------------------------------------------------------
// eda92d: severidad desconocida.
// ---------------------------------------------------------------------------

// CA-450, CA-452, hallazgo eda92d: un hallazgo ABIERTO de la tarea con una
// severidad que hoom no conoce ("hihg") cuenta como bloqueante, como el gate
// findings_open, que falla cerrado ante el: Candidatas no la hace calificar y
// su motivo nombra el hallazgo; una historica existente no se sostiene y el
// doctor lo nombra. Cerrado, el mismo hallazgo no bloquea.
func TestCA452_Hallazgoeda92d_SeveridadDesconocidaBloquea(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	for _, s := range []string{"corr", "nueva", "cerrada"} {
		htCerrar(t, root, htHallazgo(t, root, s, "high"), finding.StatusCorrected)
	}
	bdEscribir(t, root, ".hoom/items/corr.yaml", htItemYAML("corr", "bug", item.HistoricoHallazgos))
	bdCommitear(t, root, "hallazgos cerrados")
	h1SeSostiene(t, "CA-451 (precondicion)", root, "corr")

	enCorr := htHallazgo(t, root, "corr", "high")
	h1Severidad(t, root, enCorr, "hihg")
	enNueva := htHallazgo(t, root, "nueva", "high")
	h1Severidad(t, root, enNueva, "hihg")
	cerrado := htHallazgo(t, root, "cerrada", "high")
	h1Severidad(t, root, cerrado, "hihg")
	htCerrar(t, root, cerrado, finding.StatusRefuted)
	bdCommitear(t, root, "hallazgos con severidad desconocida")
	abiertos, _, err := finding.List(root, "main", true)
	if err != nil {
		t.Fatal(err)
	}
	vistos := 0
	for _, f := range abiertos {
		if (f.ID == enCorr || f.ID == enNueva) && f.Severity == "hihg" {
			vistos++
		}
	}
	if vistos != 2 {
		t.Fatalf("precondicion: los dos hallazgos \"hihg\" se leen abiertos: %+v", abiertos)
	}

	t.Run("CA-452 la historica existente no se sostiene", func(t *testing.T) {
		h1NoSeSostiene(t, "CA-452 (eda92d)", root, "corr", enCorr)
	})
	t.Run("CA-450 Candidatas no la hace calificar", func(t *testing.T) {
		cs := htCandidatas(t, "CA-450", root, "high")
		htCalifica(t, "CA-448 (control: cerrado no bloquea)", cs, "cerrada", item.HistoricoHallazgos, "bug", "cerrada")
		htNoCalifica(t, "CA-450 (eda92d)", cs, "nueva", enNueva)
	})
}

// ---------------------------------------------------------------------------
// 939eec: el spec solo en el espacio de trabajo de la tarea.
// ---------------------------------------------------------------------------

// CA-447, CA-448, hallazgo 939eec: una tarea activa cuyo spec existe solo en
// su espacio de trabajo (.hoom/worktrees/<slug>/.hoom/specs/<slug>.md, el
// estado normal antes de integrar) es una tarea CON spec: no califica por sus
// hallazgos cerrados; sin un veredicto verde aprobado en la base, su motivo
// es "sin veredicto verde aprobado en la base". Con los hallazgos cerrados en
// el espacio y en root.
func TestCA447_Hallazgo939eec_SpecSoloEnElEspacioEsConSpec(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	for _, s := range []string{"activa-espacio", "activa-root"} {
		wt := bdWorktree(t, root, s)
		htSpec(t, wt, s, "# Spec: "+s)
		bdAprobar(t, wt, s)
		bdEscribir(t, wt, s+".go", "package app\n")
		donde := root
		if s == "activa-espacio" {
			donde = wt
		}
		htCerrar(t, donde, htHallazgo(t, donde, s, "high"), finding.StatusCorrected)
		bdCommitear(t, wt, s+": spec, codigo y hallazgos")
		if _, err := os.Stat(filepath.Join(root, ".hoom", "specs", s+".md")); !os.IsNotExist(err) {
			t.Fatalf("precondicion: el spec de %s no esta en root: %v", s, err)
		}
	}
	bdCommitear(t, root, "hallazgos de root")
	cs := htCandidatas(t, "CA-447", root, "high")
	for _, s := range []string{"activa-espacio", "activa-root"} {
		t.Run(s, func(t *testing.T) {
			htNoCalifica(t, "CA-447 (939eec)", cs, s, htMotivoVeredicto)
		})
	}
}

// ---------------------------------------------------------------------------
// 552a4e: un veredicto que solo esta en el espacio de trabajo de OTRA tarea.
// ---------------------------------------------------------------------------

// CA-447, CA-452, hallazgo 552a4e: backfill elige el veredicto en los arboles
// donde la derivacion lo va a volver a buscar (root y el espacio de trabajo
// de la tarea). Un verde aprobado e integrado del spec de "propia" que solo
// vive en el espacio de trabajo de OTRA tarea ("ajena") no es elegido: si es
// la unica evidencia, "propia" no califica. El control: el verde integrado de
// "enrama" en su propio espacio si califica.
func TestCA447_Hallazgo552a4e_VeredictoEnEspacioAjenoNoSeElige(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	for _, s := range []string{"propia", "enrama"} {
		htSpec(t, root, s, "# Spec: "+s)
		bdAprobar(t, root, s)
	}
	bdCommitear(t, root, "specs")
	ajena := bdWorktree(t, root, "ajena")
	v := htVeredicto(t, ajena, ajena, "propia", bdT0, false, bdGatesVerdes(), nil)
	htForma(t, v, true, false)
	enrama := bdWorktree(t, root, "enrama")
	vEnrama := htVeredicto(t, enrama, enrama, "enrama", bdT0, false, bdGatesVerdes(), nil)
	htForma(t, vEnrama, true, false)

	cs := htCandidatas(t, "CA-447", root, "high")
	htCalifica(t, "CA-447 (control)", cs, "enrama", item.HistoricoVeredicto(vEnrama.ID), "feature", "enrama")
	if c := htCand(t, "CA-447", cs, "propia"); strings.Contains(c.Historico, v.ID) {
		t.Fatalf("CA-447 (552a4e): backfill no elige el veredicto %s, que solo esta en el espacio de otra tarea: %+v", v.ID, c)
	}
	htNoCalifica(t, "CA-447 (552a4e)", cs, "propia", htMotivoVeredicto)
}

// ---------------------------------------------------------------------------
// 9ca0a5: no es un cambio de comportamiento; fija lo que hoy ocurre.
// ---------------------------------------------------------------------------

// CA-447, CA-449, hallazgo 9ca0a5 (para la refutacion; PASA hoy): una tarea
// con un veredicto atado a su spec sigue la ruta "con spec" aunque el spec se
// haya borrado despues. Si su veredicto verde no esta integrado en la base,
// no califica ("sin veredicto verde aprobado en la base"), aunque sus
// hallazgos esten cerrados: nunca cae a "historico: hallazgos".
func TestCA449_Hallazgo9ca0a5_SpecBorradoConVeredictoSigueConSpec(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	htSpec(t, root, "borrada", "# Spec: Borrada")
	bdAprobar(t, root, "borrada")
	bdCommitear(t, root, "spec de borrada")
	suelto := htCommitSuelto(t, root, "hoom/borrada")
	htForma(t, htVeredicto(t, root, root, "borrada", bdT0, false, bdGatesVerdes(), htConCommit(suelto)), true, false)
	htCerrar(t, root, htHallazgo(t, root, "borrada", "high"), finding.StatusCorrected)
	htCerrar(t, root, htHallazgo(t, root, "borrada", "medium"), finding.StatusRefuted)
	bdCommitear(t, root, "veredicto sin integrar y hallazgos cerrados")
	if err := os.Remove(filepath.Join(root, ".hoom", "specs", "borrada.md")); err != nil {
		t.Fatal(err)
	}
	bdCommitear(t, root, "se borra el spec")
	c := htNoCalifica(t, "CA-449 (9ca0a5)", htCandidatas(t, "CA-449", root, "high"), "borrada", htMotivoVeredicto)
	if c.Historico == item.HistoricoHallazgos {
		t.Fatalf("CA-449 (9ca0a5): con un veredicto atado a su spec nunca califica por hallazgos: %+v", c)
	}
}
