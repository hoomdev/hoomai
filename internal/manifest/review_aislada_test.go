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

// ---------------------------------------------------------------- enmienda 3

// raTecho es el texto del contrato para un tope por encima de 16384 KiB
// (enmienda 3 de CA-394).
const raTecho = "review: max_evidence_kib no puede pasar de 16384"

// raCargarSinPanico carga el hoom.yaml y convierte un panico en un error
// visible del test: un tope enorme no puede tumbar al proceso (hoom serve).
func raCargarSinPanico(t *testing.T, reviewYAML string) (m *Manifest, err error, panico any) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			panico = r
		}
	}()
	m, err = raCargar(t, reviewYAML)
	return m, err, nil
}

// CA-394 (enmienda 3): max_evidence_kib tiene techo de 16384 KiB (16 MiB).
// 16385, y hasta el maximo de int64 (que multiplicado por 1024 desborda),
// fallan con el texto del contrato sin desbordar ni entrar en panico; 16384
// se acepta tal cual; 0 sigue fallando con su texto de siempre.
func TestCA394_TopeMayorA16384Falla(t *testing.T) {
	for _, v := range []string{
		"16385", "16386", "32768", "1048576",
		"2097152",             // 2^21 KiB = 2^31 bytes: desborda un int32
		"9007199254740993",    // 2^53 + 1: no entra exacto en un float64
		"9223372036854775807", // maximo de int64: por 1024 desborda un int64
	} {
		_, err, panico := raCargarSinPanico(t, "review:\n  max_evidence_kib: "+v+"\n")
		if panico != nil {
			t.Fatalf("CA-394: max_evidence_kib: %s no entra en panico: %v", v, panico)
		}
		if err == nil || !strings.Contains(err.Error(), raTecho) {
			t.Fatalf("CA-394: max_evidence_kib: %s falla con %q: %v", v, raTecho, err)
		}
		if strings.Contains(err.Error(), "debe ser mayor que 0") {
			t.Fatalf("CA-394: max_evidence_kib: %s no es un tope <= 0 (no desbordo a negativo): %v", v, err)
		}
	}

	// en la forma de flujo y junto a otras claves, igual
	_, err, panico := raCargarSinPanico(t, "review: {model: gpt-5.6-sol, effort: xhigh, max_evidence_kib: 9223372036854775807}\n")
	if panico != nil || err == nil || !strings.Contains(err.Error(), raTecho) {
		t.Fatalf("CA-394: el maximo de int64 junto a otras claves falla con %q sin panico: %v %v", raTecho, err, panico)
	}

	// 16384 es el techo exacto: se acepta y se respeta
	m, err, panico := raCargarSinPanico(t, "review:\n  max_evidence_kib: 16384\n")
	if panico != nil || err != nil {
		t.Fatalf("CA-394: max_evidence_kib: 16384 es valido: %v %v", err, panico)
	}
	if got := m.ReviewMaxEvidenceKiB(); got != 16384 {
		t.Fatalf("CA-394: el tope 16384 se respeta tal cual, fue %d", got)
	}

	// 0 sigue con el texto de siempre, no con el del techo
	_, err, panico = raCargarSinPanico(t, "review:\n  max_evidence_kib: 0\n")
	if panico != nil || err == nil || !strings.Contains(err.Error(), "review: max_evidence_kib debe ser mayor que 0") || strings.Contains(err.Error(), raTecho) {
		t.Fatalf("CA-394: max_evidence_kib: 0 falla con \"review: max_evidence_kib debe ser mayor que 0\": %v %v", err, panico)
	}
	// el minimo de int64 es <= 0: su texto, sin panico
	_, err, panico = raCargarSinPanico(t, "review:\n  max_evidence_kib: -9223372036854775808\n")
	if panico != nil || err == nil || !strings.Contains(err.Error(), "review: max_evidence_kib debe ser mayor que 0") {
		t.Fatalf("CA-394: el minimo de int64 falla con \"debe ser mayor que 0\" sin panico: %v %v", err, panico)
	}
}

// CA-394 (enmienda 3, caso limite): un numero que ni entra en un int64 no es
// un hoom.yaml valido; lo unico que se exige es un error, sin panico (el
// spec no decide el texto para lo que no es un entero de 64 bits).
func TestCA394_TopeFueraDeInt64FallaSinPanico(t *testing.T) {
	for _, v := range []string{"9223372036854775808", "18446744073709551616", "1e30", "99999999999999999999999999999999"} {
		_, err, panico := raCargarSinPanico(t, "review:\n  max_evidence_kib: "+v+"\n")
		if panico != nil {
			t.Fatalf("CA-394: max_evidence_kib: %s no entra en panico: %v", v, panico)
		}
		if err == nil {
			t.Fatalf("CA-394: max_evidence_kib: %s no es un tope valido y hoom.yaml debe fallar", v)
		}
	}
}

// CA-394 (enmienda 3, propiedad): todo tope en 1..16384 se respeta tal cual
// y todo tope en 16385..maximo de int64 se rechaza con el texto del techo,
// sin panico.
func TestCA394_TechoPropiedad(t *testing.T) {
	cfg := &quick.Config{MaxCount: 60, Rand: rand.New(rand.NewSource(16384))}
	dentro := func(n uint16) bool {
		v := int(n)%16384 + 1
		m, err, panico := raCargarSinPanico(t, "review:\n  max_evidence_kib: "+strconv.Itoa(v)+"\n")
		return panico == nil && err == nil && m.ReviewMaxEvidenceKiB() == v
	}
	if err := quick.Check(dentro, cfg); err != nil {
		t.Fatalf("CA-394: un tope de 1 a 16384 se respeta: %v", err)
	}
	fuera := func(n uint64, corto bool) bool {
		d := n & (1<<63 - 1) // 0..maximo de int64
		if corto {
			d %= 1 << 20 // tambien muy cerca del techo
		}
		v := uint64(16385)
		if d > uint64(1<<63-1)-v {
			v = 1<<63 - 1
		} else {
			v += d
		}
		_, err, panico := raCargarSinPanico(t, "review:\n  max_evidence_kib: "+strconv.FormatUint(v, 10)+"\n")
		return panico == nil && err != nil && strings.Contains(err.Error(), raTecho)
	}
	if err := quick.Check(fuera, cfg); err != nil {
		t.Fatalf("CA-394: un tope de 16385 al maximo de int64 se rechaza con %q: %v", raTecho, err)
	}
}
