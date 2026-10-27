// Tests adversariales del spec .hoom/specs/historia-en-el-tablero.md sobre el
// campo nuevo del item (CA-447, CA-448): `historico` dice que evidencia
// sostiene una tarjeta anterior a la cabina, con dos formas y solo dos:
// "veredicto <id>" o "hallazgos". Lo escribe solo `hoom item backfill` (via
// Draft.Historico), viaja en el YAML y en el JSON del item, y nunca trae
// hecho_en ni commit_final. Repos git reales en t.TempDir(). Todo
// identificador de este archivo lleva el prefijo ht.
package item

import (
	"encoding/json"
	"math/rand"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

// htIDVeredicto es un id con la forma de los de hoom (timestamp_hash).
const htIDVeredicto = "2026-09-23T06-10-06Z_ab41dc52"

// CA-447, CA-448: las dos formas del historico y su lectura.
func TestCA447_HistoricoVeredictoYLeerHistorico(t *testing.T) {
	if got := HistoricoVeredicto(htIDVeredicto); got != "veredicto "+htIDVeredicto {
		t.Fatalf("CA-447: HistoricoVeredicto(%q) es \"veredicto <id>\", fue %q", htIDVeredicto, got)
	}
	v, h, ok := LeerHistorico("veredicto " + htIDVeredicto)
	if !ok || h || v != htIDVeredicto {
		t.Fatalf("CA-447: LeerHistorico(\"veredicto <id>\") da el id, sin hallazgos y ok: (%q, %v, %v)", v, h, ok)
	}
	if HistoricoHallazgos != "hallazgos" {
		t.Fatalf("CA-448: el historico de una tarea sin spec es \"hallazgos\", fue %q", HistoricoHallazgos)
	}
	v, h, ok = LeerHistorico(HistoricoHallazgos)
	if !ok || !h || v != "" {
		t.Fatalf("CA-448: LeerHistorico(\"hallazgos\") es hallazgos y ok, sin veredicto: (%q, %v, %v)", v, h, ok)
	}
}

// CA-447, CA-448: lo que no es ninguna de las dos formas no se lee como
// historico (ni un veredicto sin id ni algo parecido a hallazgos).
func TestCA447_LeerHistoricoRechazaLoQueNoEsUnaDeLasDosFormas(t *testing.T) {
	for _, s := range []string{
		"", " ", "veredicto", "veredicto ", "hallazgo", "HALLAZGOS", "Hallazgos", "hallazgos extra",
		"otra cosa", "hecho", "commit 3f2a1b4c", "Veredicto " + htIDVeredicto,
	} {
		v, h, ok := LeerHistorico(s)
		if ok {
			t.Fatalf("CA-447: LeerHistorico(%q) no es ninguna de las dos formas y no debe ser ok: (%q, %v, %v)", s, v, h, ok)
		}
	}
}

// CA-447 (propiedad): para cualquier id con la forma de un veredicto,
// LeerHistorico(HistoricoVeredicto(id)) devuelve ese id y nunca hallazgos.
func TestCA447_PropiedadIdaYVueltaDelVeredicto(t *testing.T) {
	const alfabeto = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	gen := func(r *rand.Rand) string {
		n := 1 + r.Intn(40)
		b := make([]byte, n)
		for i := range b {
			b[i] = alfabeto[r.Intn(len(alfabeto))]
		}
		return string(b)
	}
	prop := func(semilla int64) bool {
		id := gen(rand.New(rand.NewSource(semilla)))
		h := HistoricoVeredicto(id)
		v, hall, ok := LeerHistorico(h)
		if !ok || hall || v != id || h != "veredicto "+id {
			t.Logf("CA-447: id %q -> historico %q -> (%q, %v, %v)", id, h, v, hall, ok)
			return false
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("CA-447: ida y vuelta del historico de un veredicto: %v", err)
	}
}

// CA-447, CA-448: un item escrito a mano con historico se lee con el campo, y
// el JSON del item lo trae; uno sin historico no trae la clave.
func TestCA447_ParseLeeElHistorico(t *testing.T) {
	for _, h := range []string{"veredicto " + htIDVeredicto, "hallazgos"} {
		raw := itItemValido("Historica", "2026-09-22T15:04:05Z", "historico: "+h+"\n")
		it, err := Parse("historica", []byte(raw))
		if err != nil {
			t.Fatalf("CA-447: un item con historico %q es valido: %v\n%s", h, err, raw)
		}
		if it.Historico != h || it.HechoEn != nil || it.CommitFinal != "" {
			t.Fatalf("CA-447: el item lee historico %q sin hecho_en ni commit_final: %+v", h, it)
		}
		js, _ := json.Marshal(it)
		var m map[string]any
		_ = json.Unmarshal(js, &m)
		if m["historico"] != h {
			t.Fatalf("CA-451: el JSON del item trae historico %q: %s", h, js)
		}
	}
	it, err := Parse("comun", []byte(itItemValido("Comun", "2026-09-22T15:04:05Z", "")))
	if err != nil || it.Historico != "" {
		t.Fatalf("CA-447: un item sin historico se lee sin historico: %+v %v", it, err)
	}
	if js, _ := json.Marshal(it); strings.Contains(string(js), `"historico"`) {
		t.Fatalf("CA-447: el JSON de un item sin historico no trae la clave: %s", js)
	}
}

// CA-447, CA-448: Add con Draft.Historico (lo que usa backfill) escribe el
// campo, nunca hecho_en ni commit_final, y lo que escribe se lee igual.
// MarkDone y AddSession lo conservan como cualquier otro campo. Add sin
// historico no escribe la clave.
func TestCA447_AddEscribeElHistoricoSinHechoEn(t *testing.T) {
	root := itRepo(t)
	casos := []struct{ titulo, slug, tipo, historico string }{
		{"Alfa del catalogo", "alfa", "feature", HistoricoVeredicto(htIDVeredicto)},
		{"estabilizar-ci", "estabilizar-ci", "bug", HistoricoHallazgos},
	}
	for _, c := range casos {
		it, err := Add(root, Draft{Titulo: c.titulo, Slug: c.slug, Tipo: c.tipo, Prioridad: "media", Historico: c.historico})
		if err != nil {
			t.Fatalf("CA-447: Add con historico %q: %v", c.historico, err)
		}
		if it.Historico != c.historico {
			t.Fatalf("CA-447: el Item devuelto lleva el historico %q: %+v", c.historico, it)
		}
		m := itYAML(t, Path(root, c.slug))
		if m["historico"] != c.historico || m["tipo"] != c.tipo || m["prioridad"] != "media" || m["titulo"] != c.titulo {
			t.Fatalf("CA-447: el archivo trae historico %q, tipo, prioridad y titulo: %v", c.historico, m)
		}
		for _, k := range []string{"hecho_en", "commit_final"} {
			if _, ok := m[k]; ok {
				t.Fatalf("CA-447: un item historico nunca trae %s: %v", k, m)
			}
		}
		leido, err := Load(root, c.slug)
		if err != nil || leido.Historico != c.historico || leido.HechoEn != nil || leido.CommitFinal != "" {
			t.Fatalf("CA-447: el item escrito con historico se lee igual: %+v %v", leido, err)
		}
	}

	// las otras escrituras de hoom lo conservan
	if wrote, err := AddSession(root, "alfa", "claude", time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)); err != nil || !wrote {
		t.Fatalf("CA-447: AddSession sobre un item historico: %v %v", wrote, err)
	}
	if leido, err := Load(root, "alfa"); err != nil || leido.Historico != HistoricoVeredicto(htIDVeredicto) {
		t.Fatalf("CA-447: AddSession conserva el historico: %+v %v", leido, err)
	}

	// sin historico no hay clave
	if _, err := Add(root, Draft{Titulo: "Comun"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(Path(root, "comun"))
	if strings.Contains(string(raw), "historico") {
		t.Fatalf("CA-447: un item sin historico no escribe la clave:\n%s", raw)
	}

	// Validate no pierde el historico del borrador
	d, err := Validate(Draft{Titulo: "Beta", Historico: HistoricoHallazgos, Tipo: "bug"})
	if err != nil || d.Historico != HistoricoHallazgos || !reflect.DeepEqual(d.Slug, "beta") {
		t.Fatalf("CA-448: Validate conserva el historico del borrador: %+v %v", d, err)
	}
}
