// Tests adversariales del spec .hoom/specs/historia-en-el-tablero.md en el
// tablero (CA-446..CA-453): Candidatas, la lista de tareas con evidencia que
// `hoom item backfill` mira (la misma del evidencia-sin-item del doctor), con
// lo que haria con cada una; y la regla nueva de derivacion: un item con
// historico que se sostiene cae en Hecho con la marca historica, y uno que ya
// no se sostiene deriva como cualquier otro y el doctor da
// historica-sin-sosten. Repos git reales en t.TempDir() con veredictos,
// hallazgos y specs escritos a mano; Derive con Evidence literales. Sin
// ningun CLI de IA. Todo identificador de este archivo lleva el prefijo ht.
package boardcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	htBlockOn = "findings:\n  block_on: high\n"
	// htMotivoVeredicto es el motivo del contrato para una tarea con spec que
	// no califica (CA-449).
	htMotivoVeredicto = "sin veredicto verde aprobado en la base"
	// htCommitFantasma no existe en ningun repo de estos tests.
	htCommitFantasma = "0123456789abcdef0123456789abcdef01234567"
)

// htAhora es el now de todas las lecturas.
var htAhora = time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)

// htSpecTexto es un spec valido cuyo primer encabezado es encabezado.
func htSpecTexto(encabezado string) string {
	return strings.Replace(bdSpecTexto("- CA-1: algo. [verifica: true]", true), "# Spec: precios", encabezado, 1)
}

// htSpec escribe .hoom/specs/<slug>.md en dir.
func htSpec(t *testing.T, dir, slug, encabezado string) {
	t.Helper()
	bdEscribir(t, dir, ".hoom/specs/"+slug+".md", htSpecTexto(encabezado))
}

// htVeredicto escribe en dir un veredicto del spec de slug con el estado git
// de gitDir (su HEAD es el git.commit); mut, si no es nil, lo ajusta antes de
// cerrarlo (por ejemplo, un git.commit que no esta en la base).
func htVeredicto(t *testing.T, dir, gitDir, slug string, at time.Time, partial bool, gates []verdict.GateResult, mut func(*verdict.Verdict)) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: at, Partial: partial, Spec: ".hoom/specs/" + slug + ".md",
		Git: gitx.Snapshot(gitDir, "main"), Gates: gates}
	if mut != nil {
		mut(v)
	}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	if v.ID == "" {
		t.Fatalf("precondicion: verdict.Write le da id al veredicto de %s", slug)
	}
	return v
}

// htForma exige (precondicion) que el veredicto sea verde o rojo y completo
// o parcial como el fixture lo pide.
func htForma(t *testing.T, v *verdict.Verdict, verde, parcial bool) {
	t.Helper()
	if (v.Verdict == "green") != verde || v.IsPartial() != parcial {
		t.Fatalf("precondicion: el veredicto %s de %s es verde=%v parcial=%v; fue %s parcial=%v",
			v.ID, v.Spec, verde, parcial, v.Verdict, v.IsPartial())
	}
}

func htConCommit(sha string) func(*verdict.Verdict) {
	return func(v *verdict.Verdict) { v.Git.Commit = sha }
}

// htGatesSin son los gates verdes de la tarjeta sin el gate nombre.
func htGatesSin(nombre string) []verdict.GateResult {
	var out []verdict.GateResult
	for _, g := range bdGatesVerdes() {
		if g.Name != nombre {
			out = append(out, g)
		}
	}
	return out
}

// htGatesCon son los gates verdes con el gate nombre en status.
func htGatesCon(nombre, status string) []verdict.GateResult {
	out := bdGatesVerdes()
	for i := range out {
		if out[i].Name == nombre {
			out[i].Status = status
		}
	}
	return out
}

// htCommitSuelto crea en root un commit que NO es ancestro de main (el
// trabajo de una rama sin integrar) y lo deja en la rama rama.
func htCommitSuelto(t *testing.T, root, rama string) string {
	t.Helper()
	sha := bdGit(t, root, "commit-tree", "HEAD^{tree}", "-p", "HEAD", "-m", "trabajo sin integrar de "+rama)
	bdGit(t, root, "update-ref", "refs/heads/"+rama, sha)
	return sha
}

// htHallazgo registra un hallazgo de la tarea task con severidad sev.
func htHallazgo(t *testing.T, root, task, sev string) string {
	t.Helper()
	f, err := finding.Register(root, "main", finding.Draft{Severity: sev, Lens: "risk",
		Description: "hallazgo " + sev + " de " + task, Author: "reviewer", Task: task})
	if err != nil {
		t.Fatalf("precondicion: registrar un hallazgo %s de %s: %v", sev, task, err)
	}
	return f.ID
}

// htCerrar cierra el hallazgo id como as (corregido | refutado).
func htCerrar(t *testing.T, root, id, as string) {
	t.Helper()
	if _, err := finding.Resolve(root, id, as, "TestHT lo cierra con evidencia", "hoom test <test@hoom.dev>"); err != nil {
		t.Fatalf("precondicion: cerrar el hallazgo %s como %s: %v", id, as, err)
	}
}

// htReabrir borra la resolucion del hallazgo id: vuelve a estar abierto
// (como si la resolucion se revirtiera en git).
func htReabrir(t *testing.T, root, id string) {
	t.Helper()
	p := filepath.Join(root, ".hoom", "findings", id+".res.json")
	if err := os.Remove(p); err != nil {
		t.Fatalf("precondicion: la resolucion de %s vive en %s: %v", id, p, err)
	}
}

// htItemYAML es un item escrito a mano como lo dejaria backfill.
func htItemYAML(titulo, tipo, historico string) string {
	s := "titulo: " + titulo + "\ntipo: " + tipo + "\nprioridad: media\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-10-22T10:00:00Z\n"
	if historico != "" {
		s += "historico: " + historico + "\n"
	}
	return s
}

func htCandidatas(t *testing.T, ca, root, blockOn string) []Candidata {
	t.Helper()
	cs, err := Candidatas(root, "main", blockOn, htAhora)
	if err != nil {
		t.Fatalf("%s: Candidatas sobre un proyecto valido: %v", ca, err)
	}
	return cs
}

func htCand(t *testing.T, ca string, cs []Candidata, slug string) Candidata {
	t.Helper()
	for _, c := range cs {
		if c.Slug == slug {
			return c
		}
	}
	t.Fatalf("%s: Candidatas no trae la tarea %s:\n%s", ca, slug, htFmtCands(cs))
	return Candidata{}
}

func htFmtCands(cs []Candidata) string {
	var b strings.Builder
	for _, c := range cs {
		b.WriteString("  ")
		raw, _ := json.Marshal(c)
		b.Write(raw)
		b.WriteString("\n")
	}
	return b.String()
}

// htCalifica exige que la tarea califique con ese historico, tipo y titulo.
func htCalifica(t *testing.T, ca string, cs []Candidata, slug, historico, tipo, titulo string) {
	t.Helper()
	c := htCand(t, ca, cs, slug)
	if c.ConItem || c.Historico != historico || c.Motivo != "" || c.Tipo != tipo || c.Titulo != titulo {
		t.Fatalf("%s: la tarea %s califica con historico %q, tipo %q, titulo %q y sin motivo; fue %+v",
			ca, slug, historico, tipo, titulo, c)
	}
}

// htNoCalifica exige que la tarea no califique y que su motivo diga sub.
func htNoCalifica(t *testing.T, ca string, cs []Candidata, slug, sub string) Candidata {
	t.Helper()
	c := htCand(t, ca, cs, slug)
	if c.ConItem || c.Historico != "" || c.Motivo == "" || !strings.Contains(c.Motivo, sub) {
		t.Fatalf("%s: la tarea %s no califica, sin historico y con un motivo que dice %q; fue %+v", ca, slug, sub, c)
	}
	return c
}

// htSlugsDelDoctor son las tareas con evidencia-sin-item del doctor.
func htSlugsDelDoctor(t *testing.T, ca, root, blockOn string) []string {
	t.Helper()
	r, err := Doctor(root, "main", blockOn, htAhora)
	if err != nil {
		t.Fatalf("%s: Doctor sobre el proyecto: %v", ca, err)
	}
	out := []string{}
	for _, p := range r.Problems {
		if p.ID == ProbEvidenciaSinItem {
			out = append(out, p.Slug)
		}
	}
	sort.Strings(out)
	return out
}

// htMatrizIDs son los ids que la matriz deja para afirmar sobre ellos.
type htMatrizIDs struct {
	simple, sinPrefijo, elige, fusionada, editada string // el veredicto que califica
	conAlto                                       string // el hallazgo high abierto
}

var (
	htCalifican   = []string{"correccion", "editada", "elige", "fusionada", "simple", "sin-prefijo"}
	htNoCalifican = []string{"aprobado-ausente", "commit-desconocido", "con-alto", "en-espacio", "no-integrada",
		"sin-aprobar", "sin-commit", "solo-parcial", "solo-rojo"}
	htConItem = []string{"con-item", "item-roto"}
)

// htMatriz arma un proyecto con cada forma de evidencia de una tarea sin
// item, todo commiteado en main como en hoomai salvo la tarea en-espacio,
// cuyo espacio de trabajo nunca se integro.
func htMatriz(t *testing.T) (string, htMatrizIDs) {
	t.Helper()
	root := bdRepo(t, htBlockOn)
	var ids htMatrizIDs
	en := func(h int) time.Time { return bdT0.Add(time.Duration(h) * time.Hour) }

	// --- con spec, califican ---
	htSpec(t, root, "simple", "# Spec: Simple del catalogo")
	bdAprobar(t, root, "simple")
	htSpec(t, root, "sin-prefijo", "# Catalogo nuevo")
	htSpec(t, root, "elige", "# Spec: Elige el mas nuevo")
	htSpec(t, root, "editada", "# Spec: Editada despues")
	bdAprobar(t, root, "editada")
	bdCommitear(t, root, "specs")
	ids.simple = htVeredicto(t, root, root, "simple", en(1), false, bdGatesVerdes(), nil).ID
	ids.sinPrefijo = htVeredicto(t, root, root, "sin-prefijo", en(1), false, bdGatesVerdes(), nil).ID
	ids.editada = htVeredicto(t, root, root, "editada", en(1), false, bdGatesVerdes(), nil).ID
	htForma(t, htVeredicto(t, root, root, "elige", en(1), false, bdGatesVerdes(), nil), true, false) // v1: integrado, mas viejo
	bdCommitear(t, root, "primeros veredictos")
	ids.elige = htVeredicto(t, root, root, "elige", en(2), false, bdGatesVerdes(), nil).ID // v2: el que gana
	suelto := htCommitSuelto(t, root, "hoom/elige-suelta")
	htForma(t, htVeredicto(t, root, root, "elige", en(3), false, bdGatesVerdes(), htConCommit(suelto)), true, false) // sin integrar
	htForma(t, htVeredicto(t, root, root, "elige", en(4), false, htGatesCon("test", verdict.StatusFail), nil), false, false)
	htForma(t, htVeredicto(t, root, root, "elige", en(5), true, bdGatesVerdes(), nil), true, true)               // parcial
	htForma(t, htVeredicto(t, root, root, "elige", en(6), false, htGatesSin("spec_approved"), nil), true, false) // sin aprobacion
	bdCommitear(t, root, "veredictos de elige")
	// el spec aprobado y editado despues de integrarlo: califica por la foto
	bdEscribir(t, root, ".hoom/specs/editada.md", htSpecTexto("# Spec: Editada despues")+"\nUn parrafo agregado despues de integrar.\n")
	bdCommitear(t, root, "editada cambia")

	// --- con spec, no califican ---
	for _, s := range []string{"sin-aprobar", "aprobado-ausente", "no-integrada", "commit-desconocido", "sin-commit", "solo-rojo", "solo-parcial"} {
		htSpec(t, root, s, "# Spec: "+s)
	}
	bdCommitear(t, root, "specs que no califican")
	htForma(t, htVeredicto(t, root, root, "sin-aprobar", en(1), false, htGatesSin("spec_approved"), nil), true, false)
	htForma(t, htVeredicto(t, root, root, "aprobado-ausente", en(1), false, htGatesCon("spec_approved", verdict.StatusAbsent), nil), true, false)
	htForma(t, htVeredicto(t, root, root, "no-integrada", en(1), false, bdGatesVerdes(), htConCommit(htCommitSuelto(t, root, "hoom/no-integrada"))), true, false)
	htForma(t, htVeredicto(t, root, root, "commit-desconocido", en(1), false, bdGatesVerdes(), htConCommit(htCommitFantasma)), true, false)
	htForma(t, htVeredicto(t, root, root, "sin-commit", en(1), false, bdGatesVerdes(), htConCommit("")), true, false)
	htForma(t, htVeredicto(t, root, root, "solo-rojo", en(1), false, htGatesCon("test", verdict.StatusFail), nil), false, false)
	htForma(t, htVeredicto(t, root, root, "solo-parcial", en(1), false, bdGatesVerdes(), func(v *verdict.Verdict) { v.Partial = true }), true, true)

	// --- sin spec ---
	alto := htHallazgo(t, root, "correccion", "high")
	htCerrar(t, root, alto, finding.StatusCorrected)
	medio := htHallazgo(t, root, "correccion", "medium")
	htCerrar(t, root, medio, finding.StatusRefuted)
	htHallazgo(t, root, "correccion", "low") // seguimiento abierto que no bloquea
	viejo := htHallazgo(t, root, "con-alto", "high")
	htCerrar(t, root, viejo, finding.StatusRefuted)
	ids.conAlto = htHallazgo(t, root, "con-alto", "high")

	// --- con item ---
	bdEscribir(t, root, ".hoom/items/con-item.yaml", htItemYAML("Con item", "feature", ""))
	htSpec(t, root, "con-item", "# Spec: Con item")
	bdEscribir(t, root, ".hoom/items/item-roto.yaml", htItemYAML("Item roto", "feature", "")+"columna: hecho\n")
	htSpec(t, root, "item-roto", "# Spec: Item roto")
	bdCommitear(t, root, "evidencia sin spec y con item")
	htVeredicto(t, root, root, "con-item", en(1), false, bdGatesVerdes(), nil)
	htVeredicto(t, root, root, "item-roto", en(1), false, bdGatesVerdes(), nil)
	bdCommitear(t, root, "veredictos con item")

	// --- con espacio de trabajo: una integrada con merge, otra nunca ---
	wt := bdWorktree(t, root, "fusionada")
	htSpec(t, wt, "fusionada", "# Spec: Fusionada desde su rama")
	bdAprobar(t, wt, "fusionada")
	bdEscribir(t, wt, "fusionada.go", "package app\n")
	bdCommitear(t, wt, "fusionada: spec y codigo")
	ids.fusionada = htVeredicto(t, wt, wt, "fusionada", en(7), false, bdGatesVerdes(), nil).ID
	bdCommitear(t, wt, "fusionada: veredicto")
	bdGit(t, root, "merge", "--no-ff", "-q", "-m", "Merge hoom/fusionada", "hoom/fusionada")
	bdGit(t, root, "worktree", "remove", wt)

	wt = bdWorktree(t, root, "en-espacio")
	htSpec(t, wt, "en-espacio", "# Spec: En su espacio")
	bdAprobar(t, wt, "en-espacio")
	bdEscribir(t, wt, "en_espacio.go", "package app\n")
	bdCommitear(t, wt, "en-espacio: spec y codigo")
	htVeredicto(t, wt, wt, "en-espacio", en(8), false, bdGatesVerdes(), nil)
	bdCommitear(t, wt, "en-espacio: veredicto")
	return root, ids
}

// La matriz se arma UNA vez (es lo caro: git real) y cada subtest afirma un
// criterio sobre la misma lectura. Candidatas es read-only, asi que leerla
// varias veces sobre el mismo proyecto da lo mismo.
func TestCA446_CandidatasDeLaMatriz(t *testing.T) {
	root, ids := htMatriz(t)
	// evidencia sin tarea: no es de nadie y no es candidata
	htVeredicto(t, root, root, "x", bdT0, false, bdGatesVerdes(), func(v *verdict.Verdict) { v.Spec = "" })
	drHallazgoDisco(t, root, "", "hallazgo sin tarea")

	antes := bdFoto(t, root)
	headAntes := bdGit(t, root, "rev-parse", "HEAD")
	cs := htCandidatas(t, "CA-446", root, "high")

	// CA-446: Candidatas es read-only, va en orden de slug sin repetidos ni
	// tareas vacias, y sus tareas sin item son EXACTAMENTE las del
	// evidencia-sin-item del doctor: ni una mas, ni una menos (lo que
	// backfill no ve, el doctor lo seguiria reportando).
	t.Run("CA-446 la misma lista que evidencia-sin-item", func(t *testing.T) {
		bdMismaFoto(t, "CA-446", antes, bdFoto(t, root))
		if h := bdGit(t, root, "rev-parse", "HEAD"); h != headAntes {
			t.Fatalf("CA-446: Candidatas no commitea: HEAD %s -> %s", headAntes, h)
		}
		want := append(append([]string{}, htCalifican...), htNoCalifican...)
		sort.Strings(want)
		doc := htSlugsDelDoctor(t, "CA-446", root, "high")
		if !reflect.DeepEqual(doc, want) {
			t.Fatalf("precondicion: el doctor da evidencia-sin-item de %v, dio %v", want, doc)
		}
		sinItem := []string{}
		for i, c := range cs {
			if c.Slug == "" {
				t.Fatalf("CA-446: ninguna candidata sin tarea:\n%s", htFmtCands(cs))
			}
			if i > 0 && cs[i-1].Slug >= c.Slug {
				t.Fatalf("CA-446: las candidatas van en orden de slug y sin repetir:\n%s", htFmtCands(cs))
			}
			if !c.ConItem {
				sinItem = append(sinItem, c.Slug)
				if (c.Historico == "") == (c.Motivo == "") {
					t.Fatalf("CA-446: cada tarea sin item trae historico (califica) o motivo (no), nunca los dos ni ninguno: %+v", c)
				}
			}
		}
		if !reflect.DeepEqual(sinItem, doc) {
			t.Fatalf("CA-446: Candidatas y el evidencia-sin-item del doctor miran la misma lista:\ndoctor:     %v\ncandidatas: %v", doc, sinItem)
		}
	})

	// CA-447: una tarea con spec califica por el veredicto completo, verde,
	// con spec_approved en pass e integrado en la base MAS NUEVO (no por uno
	// mas nuevo sin integrar, rojo, parcial o sin aprobacion); el titulo es
	// el primer encabezado del spec sin "Spec: ", y el tipo feature. Califica
	// tambien la integrada desde su rama con merge, y la del spec editado
	// despues de integrarla (por la foto de cuando se integro).
	t.Run("CA-447 con spec califica por el veredicto mas nuevo", func(t *testing.T) {
		htCalifica(t, "CA-447", cs, "simple", "veredicto "+ids.simple, "feature", "Simple del catalogo")
		htCalifica(t, "CA-447", cs, "sin-prefijo", "veredicto "+ids.sinPrefijo, "feature", "Catalogo nuevo")
		htCalifica(t, "CA-447", cs, "elige", "veredicto "+ids.elige, "feature", "Elige el mas nuevo")
		htCalifica(t, "CA-447", cs, "fusionada", "veredicto "+ids.fusionada, "feature", "Fusionada desde su rama")
		htCalifica(t, "CA-447", cs, "editada", "veredicto "+ids.editada, "feature", "Editada despues")
		for _, c := range cs {
			if c.Historico == "" {
				continue
			}
			if v, h, ok := item.LeerHistorico(c.Historico); !ok || (v == "" && !h) {
				t.Fatalf("CA-447: el historico de %s es una de las dos formas: %q", c.Slug, c.Historico)
			}
		}
	})

	// CA-448: una tarea sin spec califica por sus hallazgos cuando ninguno de
	// severidad block_on o mayor esta abierto (los cerrados como corregido o
	// refutado no cuentan; un seguimiento low abierto tampoco): historico
	// hallazgos, tipo bug, titulo el slug.
	t.Run("CA-448 sin spec califica por hallazgos", func(t *testing.T) {
		htCalifica(t, "CA-448", cs, "correccion", item.HistoricoHallazgos, "bug", "correccion")
	})

	// CA-449: una tarea con spec sin un veredicto que cumpla TODO no califica
	// y su motivo lo dice: sin spec_approved (ausente o sin el gate), con el
	// commit fuera de la base (rama sin integrar, commit que no existe, sin
	// commit), solo rojo, solo parcial, y la que solo tiene su verde en un
	// espacio de trabajo nunca integrado.
	t.Run("CA-449 sin veredicto verde aprobado en la base", func(t *testing.T) {
		for _, s := range []string{"sin-aprobar", "aprobado-ausente", "no-integrada", "commit-desconocido", "sin-commit",
			"solo-rojo", "solo-parcial", "en-espacio"} {
			htNoCalifica(t, "CA-449", cs, s, htMotivoVeredicto)
		}
	})

	// CA-450: una tarea sin spec con un hallazgo high abierto (block_on high)
	// no califica y su motivo nombra el hallazgo.
	t.Run("CA-450 hallazgo high abierto", func(t *testing.T) {
		htNoCalifica(t, "CA-450", cs, "con-alto", ids.conAlto)
	})

	// CA-453: las tareas que ya tienen .hoom/items/<slug>.yaml salen como
	// ConItem (tambien si el item es invalido: cuenta como item, como en el
	// doctor), y ninguna otra.
	t.Run("CA-453 las que tienen item salen aparte", func(t *testing.T) {
		got := []string{}
		for _, c := range cs {
			if c.ConItem {
				got = append(got, c.Slug)
			}
		}
		sort.Strings(got)
		if !reflect.DeepEqual(got, htConItem) {
			t.Fatalf("CA-453: ConItem son exactamente las tareas con item %v, fueron %v:\n%s", htConItem, got, htFmtCands(cs))
		}
	})
}

// CA-450: el umbral es el block_on del proyecto (sin block_on, high): con
// medium bloquea un medium abierto; con low, cualquiera; un seguimiento por
// debajo del umbral no impide calificar, y el motivo nombra el hallazgo.
func TestCA450_CandidatasUmbralEsElBlockOn(t *testing.T) {
	root := bdRepo(t, "")
	ids := map[string]string{}
	ids["medio"] = htHallazgo(t, root, "medio", "medium")
	htCerrar(t, root, htHallazgo(t, root, "medio", "high"), finding.StatusCorrected)
	ids["alto"] = htHallazgo(t, root, "alto", "high")
	ids["bajo"] = htHallazgo(t, root, "bajo", "low")
	htCerrar(t, root, htHallazgo(t, root, "cerrado", "medium"), finding.StatusRefuted)
	bdCommitear(t, root, "hallazgos")

	type esp struct{ medio, alto, bajo, cerrado bool } // califica
	for _, c := range []struct {
		blockOn string
		want    esp
	}{
		{"", esp{medio: true, alto: false, bajo: true, cerrado: true}},
		{"high", esp{medio: true, alto: false, bajo: true, cerrado: true}},
		{"medium", esp{medio: false, alto: false, bajo: true, cerrado: true}},
		{"low", esp{medio: false, alto: false, bajo: false, cerrado: true}},
	} {
		ca := "CA-450 (block_on " + c.blockOn + ")"
		cs := htCandidatas(t, ca, root, c.blockOn)
		for slug, califica := range map[string]bool{"medio": c.want.medio, "alto": c.want.alto, "bajo": c.want.bajo, "cerrado": c.want.cerrado} {
			if califica {
				htCalifica(t, ca, cs, slug, item.HistoricoHallazgos, "bug", slug)
			} else {
				htNoCalifica(t, ca, cs, slug, ids[slug])
			}
		}
	}
}

// CA-447 (hostil): una tarea escrita a mano en un hallazgo que no es un slug
// valido (con metacaracteres, con barras, con mayusculas) nunca califica: el
// item que crearia no seria .hoom/items/<slug>.yaml.
func TestCA447_CandidatasTareasHostilesNoCalifican(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	hostiles := []string{"../fuera", "a/b", "x;touch PWNED_HT;#", "$(touch PWNED_HT)", "Con Mayus", "-guion"}
	for i, h := range hostiles {
		hdHallazgoConTarea(t, root, "hostil "+string(rune('a'+i)), h)
	}
	htHallazgo(t, root, "valida", "low")
	cs := htCandidatas(t, "CA-447", root, "high")
	for _, c := range cs {
		if c.Historico != "" && !item.ValidSlug(c.Slug) {
			t.Fatalf("CA-447: una tarea que no es un slug valido nunca califica: %+v", c)
		}
	}
	htCalifica(t, "CA-447", cs, "valida", item.HistoricoHallazgos, "bug", "valida")
	if got := hdBuscar(t, root, "PWNED_HT"); len(got) > 0 {
		t.Fatalf("CA-447: leer las candidatas no ejecuta nada: %v", got)
	}
}

// htHistoricas arma el proyecto de las tarjetas historicas de CA-451/452:
// simple (spec con su verde integrado), correccion, reabierta y seguimiento
// (sin spec, hallazgos cerrados) con su item historico, y normal: la misma
// evidencia que simple con un item sin historico.
func htHistoricas(t *testing.T) (root string, verdeSimple string, cerradoReabierta string) {
	t.Helper()
	root = bdRepo(t, htBlockOn)
	for _, s := range []string{"simple", "normal"} {
		htSpec(t, root, s, "# Spec: "+s)
		bdAprobar(t, root, s)
	}
	bdCommitear(t, root, "specs")
	verdeSimple = htVeredicto(t, root, root, "simple", bdT0, false, bdGatesVerdes(), nil).ID
	htVeredicto(t, root, root, "normal", bdT0, false, bdGatesVerdes(), nil)
	for _, s := range []string{"correccion", "reabierta", "seguimiento"} {
		id := htHallazgo(t, root, s, "high")
		htCerrar(t, root, id, finding.StatusCorrected)
		if s == "reabierta" {
			cerradoReabierta = id
		}
	}
	bdEscribir(t, root, ".hoom/items/simple.yaml", htItemYAML("Simple", "feature", item.HistoricoVeredicto(verdeSimple)))
	bdEscribir(t, root, ".hoom/items/normal.yaml", htItemYAML("Normal", "feature", ""))
	for _, s := range []string{"correccion", "reabierta", "seguimiento"} {
		bdEscribir(t, root, ".hoom/items/"+s+".yaml", htItemYAML(s, "bug", item.HistoricoHallazgos))
	}
	bdCommitear(t, root, "evidencia e items historicos")
	return root, verdeSimple, cerradoReabierta
}

// htJSONCard es la tarjeta de slug como la emite JSONBytes.
func htJSONCard(t *testing.T, ca string, b Board, slug string) map[string]any {
	t.Helper()
	raw, err := JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	var js struct {
		Columns []struct {
			ID    string           `json:"id"`
			Cards []map[string]any `json:"cards"`
		} `json:"columns"`
	}
	if err := json.Unmarshal(raw, &js); err != nil {
		t.Fatalf("%s: JSONBytes es JSON: %v", ca, err)
	}
	for _, col := range js.Columns {
		for _, c := range col.Cards {
			if c["slug"] == slug {
				return c
			}
		}
	}
	t.Fatalf("%s: el JSON del tablero no trae la tarjeta %s:\n%s", ca, slug, raw)
	return nil
}

// CA-451: Build pone en Hecho, con Historica, el item historico que se
// sostiene (por su veredicto o por sus hallazgos cerrados), y su JSON trae
// "historica": true; el texto del tablero y el de la tarjeta lo marcan. Un
// item sin historico con la misma evidencia no cae en Hecho ni se marca.
func TestCA451_BuildPoneEnHechoLaHistoricaQueSeSostiene(t *testing.T) {
	root, verde, _ := htHistoricas(t)
	b, err := Build(root, "main", "high", htAhora)
	if err != nil {
		t.Fatalf("CA-451: Build: %v", err)
	}
	if len(b.Warnings) > 0 {
		t.Fatalf("CA-451: un item con historico es valido para el tablero: %q", b.Warnings)
	}
	for _, s := range []string{"simple", "correccion", "reabierta", "seguimiento"} {
		c := bdCard(t, b, s)
		if c.Column != ColHecho || !c.Historica {
			t.Fatalf("CA-451: el item historico %s que se sostiene esta en Hecho con historica; fue %s historica=%v (missing=%q)",
				s, c.Column, c.Historica, c.Missing)
		}
		for _, p := range c.Doctor {
			if p.ID == ProbHistoricaSinSosten {
				t.Fatalf("CA-451: la historica %s que se sostiene no da historica-sin-sosten: %+v", s, p)
			}
		}
		if js := htJSONCard(t, "CA-451", b, s); js["historica"] != true {
			t.Fatalf("CA-451: --json trae \"historica\": true en %s: %v", s, js["historica"])
		}
	}
	normal := bdCard(t, b, "normal")
	if normal.Column == ColHecho || normal.Historica {
		t.Fatalf("CA-451: un item sin historico no cae en Hecho por su evidencia vieja: %s historica=%v", normal.Column, normal.Historica)
	}
	if js := htJSONCard(t, "CA-451", b, "normal"); js["historica"] == true {
		t.Fatalf("CA-451: --json de una tarjeta no historica no trae historica true: %v", js)
	}

	ev := Gather(root, "main", "high", bdCard(t, b, "simple").Item, htAhora)
	if !ev.HistoricaSostenida || ev.HistoricaMotivo != "" {
		t.Fatalf("CA-451: Gather lee que el veredicto %s sostiene a simple: sostenida=%v motivo=%q", verde, ev.HistoricaSostenida, ev.HistoricaMotivo)
	}

	// la marca visible en el texto
	var buf bytes.Buffer
	Render(&buf, b)
	if !strings.Contains(buf.String(), "historica") {
		t.Fatalf("CA-451: el texto de 'hoom board' marca la tarjeta historica:\n%s", buf.String())
	}
	buf.Reset()
	RenderCard(&buf, bdCard(t, b, "simple"))
	if !strings.Contains(buf.String(), "historica") {
		t.Fatalf("CA-451: el texto de la tarjeta historica la marca:\n%s", buf.String())
	}
	buf.Reset()
	RenderCard(&buf, normal)
	if strings.Contains(buf.String(), "historica") {
		t.Fatalf("CA-451: una tarjeta que no es historica no lleva la marca:\n%s", buf.String())
	}
	buf.Reset()
	Render(&buf, bdTablero(normal))
	if strings.Contains(buf.String(), "historica") {
		t.Fatalf("CA-451: un tablero sin historicas no lleva la marca:\n%s", buf.String())
	}
}

// htMenciona exige que el problema nombre la evidencia que se cayo.
func htMenciona(t *testing.T, ca string, p Problem, id string) {
	t.Helper()
	if !strings.Contains(p.What+" "+p.Ref+" "+p.Action, id) {
		t.Fatalf("%s: historica-sin-sosten de %s nombra %s: %+v", ca, p.Slug, id, p)
	}
}

// CA-452: si el veredicto de una historica desaparece, o se abre un hallazgo
// bloqueante de su tarea (uno nuevo, o uno cerrado cuya resolucion se
// revierte), la tarjeta deja Hecho, deriva como cualquier otra (la misma
// columna que sin historico), y el doctor (el del proyecto y el de la
// tarjeta) da historica-sin-sosten con su accion. Un seguimiento por debajo
// de block_on no la saca de Hecho.
func TestCA452_LaHistoricaQueNoSeSostieneDejaHecho(t *testing.T) {
	root, verde, cerrado := htHistoricas(t)
	if err := os.Remove(filepath.Join(root, ".hoom", "verdicts", verde+".json")); err != nil {
		t.Fatalf("precondicion: el veredicto %s vive en .hoom/verdicts: %v", verde, err)
	}
	nuevo := htHallazgo(t, root, "correccion", "high")
	htReabrir(t, root, cerrado)
	htHallazgo(t, root, "seguimiento", "low")

	b, err := Build(root, "main", "high", htAhora)
	if err != nil {
		t.Fatalf("CA-452: Build: %v", err)
	}
	if len(b.Warnings) > 0 {
		t.Fatalf("CA-452: un item con historico es valido para el tablero: %q", b.Warnings)
	}
	r, err := Doctor(root, "main", "high", htAhora)
	if err != nil {
		t.Fatalf("CA-452: Doctor: %v", err)
	}
	cayo := map[string]string{"simple": verde, "correccion": nuevo, "reabierta": cerrado}
	for s, id := range cayo {
		c := bdCard(t, b, s)
		if c.Column == ColHecho || c.Historica {
			t.Fatalf("CA-452: la historica %s que ya no se sostiene deja Hecho y la marca; fue %s historica=%v", s, c.Column, c.Historica)
		}
		ev := Gather(root, "main", "high", c.Item, htAhora)
		if ev.HistoricaSostenida || ev.HistoricaMotivo == "" {
			t.Fatalf("CA-452: Gather lee que %s ya no se sostiene y por que: sostenida=%v motivo=%q", s, ev.HistoricaSostenida, ev.HistoricaMotivo)
		}
		comun := ev
		comun.Item.Historico, comun.HistoricaSostenida, comun.HistoricaMotivo = "", false, ""
		if want := Derive(comun).Column; c.Column != want {
			t.Fatalf("CA-452: %s deriva como cualquier otra: columna %s, fue %s", s, want, c.Column)
		}
		var enTarjeta *Problem
		for i := range c.Doctor {
			if c.Doctor[i].ID == ProbHistoricaSinSosten {
				enTarjeta = &c.Doctor[i]
			}
		}
		if enTarjeta == nil || enTarjeta.Action == "" || enTarjeta.What == "" || enTarjeta.Slug != s {
			t.Fatalf("CA-452: la tarjeta %s trae historica-sin-sosten con su accion: %s", s, drFmt(c.Doctor))
		}
		htMenciona(t, "CA-452", *enTarjeta, id)
		n := 0
		for _, p := range r.Problems {
			if p.ID == ProbHistoricaSinSosten && p.Slug == s {
				n++
				if p.Action == "" {
					t.Fatalf("CA-452: historica-sin-sosten de %s trae su accion: %+v", s, p)
				}
			}
		}
		if n != 1 {
			t.Fatalf("CA-452: el doctor da UN historica-sin-sosten de %s, dio %d:\n%s", s, n, drFmt(r.Problems))
		}
	}
	seg := bdCard(t, b, "seguimiento")
	if seg.Column != ColHecho || !seg.Historica {
		t.Fatalf("CA-452: un seguimiento low abierto no saca de Hecho a la historica: %s historica=%v", seg.Column, seg.Historica)
	}
	for _, p := range r.Problems {
		if p.ID == ProbHistoricaSinSosten && cayo[p.Slug] == "" {
			t.Fatalf("CA-452: solo las que no se sostienen dan historica-sin-sosten: %+v", p)
		}
	}
}

// CA-451, CA-452 (propiedad sobre Derive, pura): en TODAS las Evidence de los
// fixtures de C1 a C4, con cada historico y cada estado de su evidencia:
//   - sin historico, HistoricaSostenida no cambia nada;
//   - con hecho_en, manda hecho_en: Hecho y sin la marca historica;
//   - con historico que se sostiene: Hecho, Historica, "historica": true en
//     el JSON y sin historica-sin-sosten;
//   - con historico que no se sostiene: la columna de la misma tarjeta sin
//     historico, sin la marca, y su doctor es el de esa tarjeta MAS un
//     historica-sin-sosten con accion.
func TestCA451_PropiedadDerivaLaHistorica(t *testing.T) {
	historicos := []string{item.HistoricoVeredicto("2026-09-22T15-30-00Z_aaaa1111"), item.HistoricoHallazgos}
	n := 0
	for _, f := range drTodas() {
		base := f.ev
		base.Item.Historico, base.HistoricaSostenida, base.HistoricaMotivo = "", false, ""
		cb := Derive(base)

		sinHist := base
		sinHist.HistoricaSostenida, sinHist.HistoricaMotivo = true, "no deberia importar"
		if c := Derive(sinHist); !reflect.DeepEqual(c, cb) || c.Historica {
			t.Fatalf("CA-451: [%s] sin historico, HistoricaSostenida no cambia la tarjeta", f.nombre)
		}

		for _, h := range historicos {
			for _, sost := range []bool{true, false} {
				n++
				ev := base
				ev.Item.Historico, ev.HistoricaSostenida = h, sost
				if !sost {
					ev.HistoricaMotivo = "la evidencia del historico ya no esta"
				}
				c := Derive(ev)
				nombre := f.nombre + " / " + h
				switch {
				case ev.Item.HechoEn != nil:
					if c.Column != ColHecho || c.Historica {
						t.Fatalf("CA-451: [%s] hecho_en se mira primero: Hecho sin la marca historica; fue %s historica=%v", nombre, c.Column, c.Historica)
					}
				case sost:
					if c.Column != ColHecho || !c.Historica {
						t.Fatalf("CA-451: [%s] el historico que se sostiene da Hecho con historica; fue %s historica=%v", nombre, c.Column, c.Historica)
					}
					for _, p := range c.Doctor {
						if p.ID == ProbHistoricaSinSosten {
							t.Fatalf("CA-451: [%s] una historica en Hecho no da historica-sin-sosten", nombre)
						}
					}
					raw, _ := json.Marshal(c)
					var m map[string]any
					_ = json.Unmarshal(raw, &m)
					if m["historica"] != true {
						t.Fatalf("CA-451: [%s] el JSON de la tarjeta trae \"historica\": true: %v", nombre, m["historica"])
					}
				default:
					if c.Column != cb.Column || c.Historica {
						t.Fatalf("CA-452: [%s] la historica que no se sostiene deriva como cualquier otra (%s), sin la marca; fue %s historica=%v",
							nombre, cb.Column, c.Column, c.Historica)
					}
					resto := []string{}
					cuantos := 0
					for _, p := range c.Doctor {
						if p.ID == ProbHistoricaSinSosten {
							cuantos++
							if p.Action == "" || p.Slug != c.Slug {
								t.Fatalf("CA-452: [%s] historica-sin-sosten trae la tarjeta y su accion: %+v", nombre, p)
							}
							continue
						}
						resto = append(resto, p.ID)
					}
					if cuantos != 1 || !reflect.DeepEqual(resto, drIDs(cb.Doctor)) {
						t.Fatalf("CA-452: [%s] el doctor es el de la tarjeta comun %v mas UN historica-sin-sosten; fue %v",
							nombre, drIDs(cb.Doctor), drIDs(c.Doctor))
					}
				}
				if c.Historica && c.Column != ColHecho {
					t.Fatalf("CA-451: [%s] historica solo en Hecho; fue %s", nombre, c.Column)
				}
			}
		}
	}
	if n < 100 {
		t.Fatalf("CA-451: la propiedad tiene que mirar muchos casos (al menos 100), miro %d", n)
	}
}
