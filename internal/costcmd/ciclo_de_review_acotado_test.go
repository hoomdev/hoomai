// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// `hoom costo` (CA-634): el reloj de la firma a la aceptacion. firmada_en es
// el approved_at mas viejo de .hoom/approvals/ para el spec de la tarea (una
// enmienda no reinicia el reloj), reloj_ms la diferencia con aceptada_en, y
// null cuando falta alguno; la linea de texto agrega `reloj <duracion>`.
// Re-expresa CA-547 solo en que cada tarea suma esas dos claves. Escritos
// desde el spec y las firmas publicas, sin leer la implementacion.
package costcmd

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

var crSeqAprob int

// crAprobacion escribe un registro de .hoom/approvals/ como lo deja `hoom
// spec approve`: spec, sha256, approved_by y approved_at.
func crAprobacion(t *testing.T, root, spec string, cuando time.Time) {
	t.Helper()
	crSeqAprob++
	sha := fmt.Sprintf("%064x", crSeqAprob)
	slug := strings.TrimSuffix(filepath.Base(spec), ".md")
	raw, _ := json.MarshalIndent(map[string]string{
		"spec": spec, "sha256": sha, "approved_by": "Ana <ana@hoom.dev>",
		"approved_at": cuando.UTC().Format(time.RFC3339Nano),
	}, "", "  ")
	dir := filepath.Join(root, ".hoom", "approvals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, slug+"_"+sha[56:]+".json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

// la aceptacion de alfa en ccAlfa: ccT0 + 180 min = 2026-11-07 13:00 UTC.
var crAceptadaAlfa = ccT0.Add(ccMin(180))

// CA-634: firmada_en es la aprobacion mas vieja del spec de la tarea y
// reloj_ms la diferencia hasta aceptada_en; una enmienda (otra aprobacion
// mas nueva) no reinicia el reloj; la aprobacion de otro spec no cuenta.
func TestCA634_FirmadaEnYRelojMS(t *testing.T) {
	fx := ccAlfa(t)
	firma := crAceptadaAlfa.Add(-(19*time.Hour + 4*time.Minute + 30*time.Second))
	crAprobacion(t, fx.root, ".hoom/specs/alfa.md", firma.Add(5*time.Hour)) // enmienda: mas nueva
	crAprobacion(t, fx.root, ".hoom/specs/alfa.md", firma)
	crAprobacion(t, fx.root, ".hoom/specs/alfa-v2.md", firma.Add(-48*time.Hour)) // otro spec
	crAprobacion(t, fx.root, ".hoom/specs/otra/alfa.md", firma.Add(-72*time.Hour))

	for _, slug := range []string{"", "alfa"} {
		ta := ccTarea(t, "CA-634", ccBuild(t, fx.root, slug), "alfa")
		if ta.FirmadaEn == nil || !ta.FirmadaEn.Equal(firma) {
			t.Fatalf("CA-634: firmada_en es la aprobacion mas vieja de alfa (%v): %v", firma, ta.FirmadaEn)
		}
		want := crAceptadaAlfa.Sub(firma).Milliseconds()
		if ta.RelojMS == nil || *ta.RelojMS != want {
			t.Fatalf("CA-634: reloj_ms es %d: %v", want, ta.RelojMS)
		}
		m := ccJSON(t, ccBuild(t, fx.root, slug))
		var js map[string]any
		for _, e := range m["tareas"].([]any) {
			if e.(map[string]any)["slug"] == "alfa" {
				js = e.(map[string]any)
			}
		}
		if js["reloj_ms"] != float64(want) {
			t.Fatalf("CA-634: el JSON trae reloj_ms %d: %v", want, js["reloj_ms"])
		}
		f, err := time.Parse(time.RFC3339Nano, fmt.Sprint(js["firmada_en"]))
		if err != nil || !f.Equal(firma) {
			t.Fatalf("CA-634: el JSON trae firmada_en %v: %v", firma, js["firmada_en"])
		}
	}
	// la linea de texto: reloj 19h4m (minutos truncados), despues de las rondas
	texto := ccTexto(ccBuild(t, fx.root, ""), false)
	alfa := ccLineasCon(texto, "alfa")[0]
	i, j := strings.Index(alfa, "rondas 2"), strings.Index(alfa, "reloj 19h4m")
	if i < 0 || j < 0 || j < i {
		t.Fatalf("CA-634: la linea de alfa agrega `reloj 19h4m` despues de las rondas: %q", alfa)
	}
}

// CA-634: sin aprobacion, firmada_en y reloj_ms son null y la linea no dice
// reloj; sin aceptacion, firmada_en esta y reloj_ms es null.
func TestCA634_NullSinFirmaOSinAceptacion(t *testing.T) {
	fx := ccAlfa(t)
	ccRun(t, fx.root, ccMeta("beta", "writer", "codex", ccT0.Add(ccMin(500)), ccMin(5)))
	ccItem(t, fx.root, "beta")
	crAprobacion(t, fx.root, ".hoom/specs/beta.md", ccT0)

	r := ccBuild(t, fx.root, "")
	alfa := ccTarea(t, "CA-634", r, "alfa")
	if alfa.FirmadaEn != nil || alfa.RelojMS != nil {
		t.Fatalf("CA-634: sin aprobacion, null y null: %v %v", alfa.FirmadaEn, alfa.RelojMS)
	}
	beta := ccTarea(t, "CA-634", r, "beta")
	if beta.FirmadaEn == nil || !beta.FirmadaEn.Equal(ccT0) || beta.RelojMS != nil {
		t.Fatalf("CA-634: sin aceptacion, firmada_en y reloj_ms null: %v %v", beta.FirmadaEn, beta.RelojMS)
	}
	m := ccJSON(t, r)
	for _, e := range m["tareas"].([]any) {
		ta := e.(map[string]any)
		for _, k := range []string{"firmada_en", "reloj_ms"} {
			if _, hay := ta[k]; !hay {
				t.Fatalf("CA-634: cada tarea trae %s (null si falta): %v", k, ta)
			}
		}
		if ta["reloj_ms"] != nil {
			t.Fatalf("CA-634: reloj_ms null: %v", ta)
		}
		if ta["slug"] == "alfa" && ta["firmada_en"] != nil {
			t.Fatalf("CA-634: firmada_en null sin aprobacion: %v", ta)
		}
	}
	texto := ccTexto(r, false)
	for _, l := range append(ccLineasCon(texto, "alfa"), ccLineasCon(texto, "beta")...) {
		if strings.Contains(l, "reloj") {
			t.Fatalf("CA-634: sin dato la linea no agrega nada: %q", l)
		}
	}
}

// CA-634: el formato de la duracion: horas totales y minutos truncados, o
// solo minutos con menos de una hora.
func TestCA634_FormatoDelReloj(t *testing.T) {
	for _, c := range []struct {
		d    time.Duration
		want string
	}{
		{45*time.Minute + 59*time.Second, "reloj 45m"},
		{50 * time.Hour, "reloj 50h0m"},
		{19*time.Hour + 4*time.Minute + 59*time.Second, "reloj 19h4m"},
		{59 * time.Second, "reloj 0m"},
		{time.Hour, "reloj 1h0m"},
		{3*24*time.Hour + 7*time.Minute, "reloj 72h7m"},
	} {
		fx := ccAlfa(t)
		crAprobacion(t, fx.root, ".hoom/specs/alfa.md", crAceptadaAlfa.Add(-c.d))
		r := ccBuild(t, fx.root, "")
		alfa := ccLineasCon(ccTexto(r, false), "alfa")[0]
		if !crTieneToken(alfa, c.want) {
			t.Fatalf("CA-634: %v se escribe %q: %q", c.d, c.want, alfa)
		}
		if ta := ccTarea(t, "CA-634", r, "alfa"); ta.RelojMS == nil || *ta.RelojMS != c.d.Milliseconds() {
			t.Fatalf("CA-634: reloj_ms %d: %v", c.d.Milliseconds(), ta.RelojMS)
		}
	}
}

// crTieneToken: s contiene want seguido de fin o de algo que no es digito
// ni letra (asi "reloj 45m" no casa con "reloj 45m3").
func crTieneToken(s, want string) bool {
	for i := strings.Index(s, want); i >= 0; {
		fin := i + len(want)
		if fin == len(s) || !strings.ContainsAny(s[fin:fin+1], "0123456789abcdefghijklmnopqrstuvwxyz") {
			return true
		}
		k := strings.Index(s[fin:], want)
		if k < 0 {
			return false
		}
		i = fin + k
	}
	return false
}

type crDur int64

func (crDur) Generate(r *rand.Rand, _ int) reflect.Value {
	return reflect.ValueOf(crDur(r.Int63n(int64(200 * time.Hour))))
}

// CA-634 (propiedad): para toda duracion d entre la firma y la aceptacion,
// la linea dice `reloj <h>h<m>m` (o `<m>m` bajo una hora) con minutos
// truncados, y reloj_ms es d en milisegundos.
func TestCA634_PropiedadFormato(t *testing.T) {
	fx := ccAlfa(t)
	prop := func(x crDur) bool {
		d := time.Duration(x)
		_ = os.RemoveAll(filepath.Join(fx.root, ".hoom", "approvals"))
		crAprobacion(t, fx.root, ".hoom/specs/alfa.md", crAceptadaAlfa.Add(-d))
		r, err := Build(fx.root, "")
		if err != nil {
			return false
		}
		ta := ccTarea(t, "CA-634", r, "alfa")
		if ta.RelojMS == nil || *ta.RelojMS != d.Milliseconds() {
			return false
		}
		h, m := int64(d/time.Hour), int64((d%time.Hour)/time.Minute)
		want := fmt.Sprintf("reloj %dm", m)
		if h > 0 {
			want = fmt.Sprintf("reloj %dh%dm", h, m)
		}
		return crTieneToken(ccLineasCon(ccTexto(r, false), "alfa")[0], want)
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 60}); err != nil {
		t.Fatalf("CA-634: %v", err)
	}
}
