// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (CA-394): la seccion review: de hoom.yaml. Estricta como findings: una
// clave desconocida o un tope que no es positivo hacen fallar manifest.Load
// con el texto del contrato; sin la seccion, el reviewer corre aislado y el
// tope de la evidencia es 320 KiB. Modelo y esfuerzo son vocabulario del
// provider y no se validan.
package manifest

import (
	"math/rand"
	"strconv"
	"strings"
	"testing"
	"testing/quick"
)

const raValidas = "(validas: provider, model, effort, same_provider, isolated, max_evidence_kib)"

func raCargar(t *testing.T, reviewYAML string) (*Manifest, error) {
	t.Helper()
	return Load(foManifiesto(t, reviewYAML), nil)
}

// CA-394: las seis claves parsean tal cual; modelo y esfuerzo no se validan
// (cualquier texto del provider pasa, con unicode incluido).
func TestCA394_ReviewParseaLasSeisClaves(t *testing.T) {
	m, err := raCargar(t, "review:\n"+
		"  provider: codex\n"+
		"  model: gpt-5.6-sol\n"+
		"  effort: turbo-9000-ñ\n"+
		"  same_provider: true\n"+
		"  isolated: false\n"+
		"  max_evidence_kib: 100\n")
	if err != nil {
		t.Fatalf("CA-394: una seccion review: con las seis claves validas no es un hoom.yaml invalido: %v", err)
	}
	if m.Review == nil {
		t.Fatal("CA-394: la seccion review: queda en Manifest.Review")
	}
	r := m.Review
	if r.Provider != "codex" || r.Model != "gpt-5.6-sol" || r.Effort != "turbo-9000-ñ" {
		t.Fatalf("CA-394: provider, model y effort se leen tal cual (sin validar vocabulario): %+v", r)
	}
	if r.SameProvider == nil || !*r.SameProvider {
		t.Fatalf("CA-394: same_provider: true se lee: %+v", r.SameProvider)
	}
	if r.Isolated == nil || *r.Isolated {
		t.Fatalf("CA-394: isolated: false se lee: %+v", r.Isolated)
	}
	if r.MaxEvidenceKiB == nil || *r.MaxEvidenceKiB != 100 {
		t.Fatalf("CA-394: max_evidence_kib: 100 se lee: %+v", r.MaxEvidenceKiB)
	}
	if m.ReviewIsolated() {
		t.Fatal("CA-394: isolated: false apaga el aislamiento del reviewer")
	}
	if got := m.ReviewMaxEvidenceKiB(); got != 100 {
		t.Fatalf("CA-394: el tope es el de hoom.yaml (100), fue %d", got)
	}

	// isolated: true explicito y un tope de 1 KiB (el minimo positivo)
	m, err = raCargar(t, "review: {isolated: true, max_evidence_kib: 1}\n")
	if err != nil {
		t.Fatalf("CA-394: isolated: true y max_evidence_kib: 1 son validos: %v", err)
	}
	if !m.ReviewIsolated() || m.ReviewMaxEvidenceKiB() != 1 {
		t.Fatalf("CA-394: isolated true y tope 1: isolated=%v tope=%d", m.ReviewIsolated(), m.ReviewMaxEvidenceKiB())
	}
}

// CA-394: sin la seccion (o vacia, o con otras claves) isolated es true y el
// tope 320 KiB; un *Manifest nil da lo mismo.
func TestCA394_SinSeccionAisladoYTope320(t *testing.T) {
	if DefaultMaxEvidenceKiB != 320 {
		t.Fatalf("CA-394: el tope por defecto es 320 KiB, la constante dice %d", DefaultMaxEvidenceKiB)
	}
	casos := map[string]string{
		"sin seccion":          "",
		"review: {}":           "review: {}\n",
		"review vacio (null)":  "review:\n",
		"solo modelo":          "review:\n  model: gpt-5.6-sol\n",
		"solo esfuerzo":        "review:\n  effort: high\n",
		"isolated sin valor":   "review:\n  isolated:\n",
		"tope sin valor":       "review:\n  max_evidence_kib:\n",
		"same_provider y prov": "review:\n  provider: claude\n  same_provider: false\n",
	}
	for nombre, yml := range casos {
		m, err := raCargar(t, yml)
		if err != nil {
			t.Fatalf("CA-394 (%s): no es un hoom.yaml invalido: %v", nombre, err)
		}
		if !m.ReviewIsolated() {
			t.Fatalf("CA-394 (%s): isolated ausente = true", nombre)
		}
		if got := m.ReviewMaxEvidenceKiB(); got != 320 {
			t.Fatalf("CA-394 (%s): tope ausente = 320 KiB, fue %d", nombre, got)
		}
	}
	var nada *Manifest
	if !nada.ReviewIsolated() || nada.ReviewMaxEvidenceKiB() != DefaultMaxEvidenceKiB {
		t.Fatalf("CA-394: un Manifest nil da los valores por defecto: isolated=%v tope=%d",
			nada.ReviewIsolated(), nada.ReviewMaxEvidenceKiB())
	}
	if !(&Manifest{}).ReviewIsolated() || (&Manifest{}).ReviewMaxEvidenceKiB() != 320 {
		t.Fatal("CA-394: un Manifest sin Review da los valores por defecto")
	}
}

// CA-394: una clave desconocida falla con el texto del contrato, que nombra
// la clave y las seis validas. Las claves distinguen mayusculas.
func TestCA394_ClaveDesconocidaFalla(t *testing.T) {
	for _, clave := range []string{"modelo", "Model", "esfuerzo", "isolate", "max_evidence", "maxEvidenceKiB", "reviewer", "same-provider"} {
		_, err := raCargar(t, "review:\n  "+clave+": x\n")
		if err == nil {
			t.Fatalf("CA-394: review.%s es una clave desconocida y hoom.yaml debe fallar", clave)
		}
		want := `review: clave desconocida "` + clave + `" ` + raValidas
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("CA-394: el error dice %q, fue %q", want, err.Error())
		}
	}
	// junto a claves validas, la desconocida igual falla
	_, err := raCargar(t, "review:\n  model: gpt-5.6-sol\n  efort: high\n")
	if err == nil || !strings.Contains(err.Error(), `review: clave desconocida "efort" `+raValidas) {
		t.Fatalf("CA-394: un typo junto a claves validas falla nombrandolo: %v", err)
	}
	// una seccion que no es un mapa tampoco es un hoom.yaml valido
	if _, err := raCargar(t, "review: codex\n"); err == nil {
		t.Fatal("CA-394: review: debe ser un mapa")
	}
}

// CA-394: max_evidence_kib 0 o negativo falla con el texto del contrato.
func TestCA394_TopeCeroONegativoFalla(t *testing.T) {
	for _, v := range []string{"0", "-1", "-320"} {
		_, err := raCargar(t, "review:\n  max_evidence_kib: "+v+"\n")
		if err == nil || !strings.Contains(err.Error(), "review: max_evidence_kib debe ser mayor que 0") {
			t.Fatalf("CA-394: max_evidence_kib: %s falla con \"review: max_evidence_kib debe ser mayor que 0\": %v", v, err)
		}
	}
}

// CA-394 (propiedad): todo tope positivo se respeta tal cual y todo tope <= 0
// se rechaza; el resto de la seccion no cambia la respuesta.
func TestCA394_TopePropiedad(t *testing.T) {
	cfg := &quick.Config{MaxCount: 40, Rand: rand.New(rand.NewSource(394))}
	positivo := func(n uint16) bool {
		v := int(n%4096) + 1
		m, err := raCargar(t, "review:\n  effort: high\n  max_evidence_kib: "+strconv.Itoa(v)+"\n")
		return err == nil && m.ReviewMaxEvidenceKiB() == v && m.ReviewIsolated()
	}
	if err := quick.Check(positivo, cfg); err != nil {
		t.Fatalf("CA-394: un tope positivo se respeta: %v", err)
	}
	noPositivo := func(n uint16) bool {
		v := -int(n % 4096)
		_, err := raCargar(t, "review:\n  max_evidence_kib: "+strconv.Itoa(v)+"\n")
		return err != nil && strings.Contains(err.Error(), "review: max_evidence_kib debe ser mayor que 0")
	}
	if err := quick.Check(noPositivo, cfg); err != nil {
		t.Fatalf("CA-394: un tope <= 0 se rechaza: %v", err)
	}
}
