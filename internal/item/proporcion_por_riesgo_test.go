// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// item (CA-796): Parse acepta las cinco claves del tope de la tarjeta y
// rechaza un tope que no es un entero >= 0; Cambiar escribe el tope con
// tope_motivo, tope_por y tope_en, y se niega sin motivo, con motivo en
// blanco, con un valor negativo o con un motivo sin tope, sin tocar el
// archivo; TarjetaDe y RiesgoDeLanzamiento leen la tarjeta declarada (CA-799,
// CA-804). Escritos desde el spec, las aclaraciones de la ronda 0 y las
// firmas publicas, sin leer la implementacion.
package item

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

const pprTopeErrPal = "tope_palabras tiene que ser un entero mayor o igual que 0"
const pprTopeErrCri = "tope_criterios tiene que ser un entero mayor o igual que 0"

func pprInt(n int) *int       { return &n }
func pprStr(s string) *string { return &s }
func pprLeer(t *testing.T, root string) []byte {
	t.Helper()
	raw, err := os.ReadFile(Path(root, "precios"))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func pprCon(t *testing.T) string {
	t.Helper()
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Precios", Slug: "precios", Pedido: "por region"}); err != nil {
		t.Fatal(err)
	}
	return root
}

// CA-796: Parse acepta tope_palabras, tope_criterios, tope_motivo, tope_por y
// tope_en; 0 es un tope (sin tope para la tarjeta), distinto de no tenerlo; el
// JSON del item los trae.
func TestCA796_ParseAceptaLasCincoClaves(t *testing.T) {
	const ca = "CA-796"
	extra := "tope_palabras: 1200\ntope_criterios: 0\ntope_motivo: \"el intake es largo\"\ntope_por: \"Ana <ana@hoom.dev>\"\ntope_en: 2026-11-24T10:00:05Z\n"
	it, err := Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", extra)))
	if err != nil {
		t.Fatalf("%s: Parse acepta las cinco claves: %v", ca, err)
	}
	en := time.Date(2026, 11, 24, 10, 0, 5, 0, time.UTC)
	if it.TopePalabras == nil || *it.TopePalabras != 1200 || it.TopeCriterios == nil || *it.TopeCriterios != 0 ||
		it.TopeMotivo != "el intake es largo" || it.TopePor != "Ana <ana@hoom.dev>" || it.TopeEn == nil || !it.TopeEn.Equal(en) {
		t.Fatalf("%s: Parse lee las cinco claves: %+v", ca, it)
	}
	raw, err := json.Marshal(it)
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{`"tope_palabras":1200`, `"tope_criterios":0`, `"tope_motivo":"el intake es largo"`, `"tope_en":"2026-11-24T10:00:05Z"`} {
		if !strings.Contains(string(raw), w) {
			t.Fatalf("%s: el JSON del item trae %s: %s", ca, w, raw)
		}
	}
	// sin las claves: nil, y el JSON no las nombra
	it, err = Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", "")))
	if err != nil || it.TopePalabras != nil || it.TopeCriterios != nil || it.TopeMotivo != "" || it.TopePor != "" || it.TopeEn != nil {
		t.Fatalf("%s: sin las claves la tarjeta no tiene tope: %+v %v", ca, it, err)
	}
	raw, _ = json.Marshal(it)
	if strings.Contains(string(raw), "tope_") {
		t.Fatalf("%s: sin tope el JSON no trae tope_: %s", ca, raw)
	}
}

// CA-796: un tope_palabras o tope_criterios que no es un entero >= 0 es el
// error exacto.
func TestCA796_ParseRechazaTopeQueNoEsEntero(t *testing.T) {
	const ca = "CA-796"
	for _, v := range []string{"-1", "1.5", "\"x\"", "ocho", "[1]", "{ a: 1 }", "-0.5", "true"} {
		for clave, want := range map[string]string{"tope_palabras": pprTopeErrPal, "tope_criterios": pprTopeErrCri} {
			_, err := Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", clave+": "+v+"\n")))
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("%s: %s: %s es %q: %v", ca, clave, v, want, err)
			}
		}
	}
	// sigue estricto con una clave parecida
	if _, err := Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", "tope_palabra: 3\n"))); err == nil {
		t.Fatalf("%s: tope_palabra (sin s) es una clave desconocida", ca)
	}
}

// CA-796: Cambiar con TopePalabras y motivo escribe tope_palabras, tope_motivo
// recortado, tope_por (git) y tope_en (UTC, al segundo); conserva el resto y
// no escribe riesgo_por.
func TestCA796_CambiarEscribeElTopeConQuienYCuando(t *testing.T) {
	const ca = "CA-796"
	root := pprCon(t)
	desde := time.Now().UTC().Truncate(time.Second)
	antes, despues, cambio, err := Cambiar(root, "precios", Cambios{TopePalabras: pprInt(1200), TopeMotivo: pprStr("  el intake es largo  ")})
	hasta := time.Now().UTC()
	if err != nil || !cambio {
		t.Fatalf("%s: Cambiar escribe el tope: %v %v", ca, cambio, err)
	}
	if antes.TopePalabras != nil || despues.TopePalabras == nil || *despues.TopePalabras != 1200 || despues.TopeCriterios != nil {
		t.Fatalf("%s: antes sin tope, despues 1200: %+v %+v", ca, antes, despues)
	}
	if despues.TopeMotivo != "el intake es largo" || despues.TopePor != retyQuien || despues.TopeEn == nil {
		t.Fatalf("%s: motivo recortado, quien y cuando: %+v", ca, despues)
	}
	raw := pprLeer(t, root)
	for clave, want := range map[string]string{"tope_palabras": "1200", "tope_motivo": "el intake es largo", "tope_por": retyQuien} {
		if v, ok := retyLinea(raw, clave); !ok || v != want {
			t.Fatalf("%s: el archivo lleva %s: %s:\n%s", ca, clave, want, raw)
		}
	}
	v, ok := retyLinea(raw, "tope_en")
	if !ok || !retyRFC3339UTC.MatchString(v) {
		t.Fatalf("%s: tope_en en UTC, RFC 3339: %q\n%s", ca, v, raw)
	}
	en, _ := time.Parse(time.RFC3339Nano, v)
	if en.Before(desde) || en.After(hasta.Add(time.Second)) || en.Nanosecond() != 0 {
		t.Fatalf("%s: tope_en es ahora, al segundo: %v (%v..%v)", ca, en, desde, hasta)
	}
	for _, clave := range []string{"tope_criterios", "riesgo_por", "riesgo_en", "riesgo"} {
		if _, ok := retyLinea(raw, clave); ok {
			t.Fatalf("%s: un tope de palabras no escribe %s:\n%s", ca, clave, raw)
		}
	}
	if despues.Titulo != "Precios" || despues.Pedido != "por region" || !despues.CreadoEn.Equal(antes.CreadoEn) {
		t.Fatalf("%s: el resto se conserva: %+v", ca, despues)
	}
	// los dos topes juntos, y 0 (sin tope para la tarjeta) se escribe
	_, despues, cambio, err = Cambiar(root, "precios", Cambios{TopePalabras: pprInt(0), TopeCriterios: pprInt(12), TopeMotivo: pprStr("otro")})
	if err != nil || !cambio || despues.TopePalabras == nil || *despues.TopePalabras != 0 || despues.TopeCriterios == nil || *despues.TopeCriterios != 12 {
		t.Fatalf("%s: los dos topes juntos, el 0 incluido: %+v %v %v", ca, despues, cambio, err)
	}
	raw = pprLeer(t, root)
	if v, ok := retyLinea(raw, "tope_palabras"); !ok || v != "0" {
		t.Fatalf("%s: tope_palabras: 0 queda escrito (sin tope, distinto de no tenerlo):\n%s", ca, raw)
	}
	if v, ok := retyLinea(raw, "tope_criterios"); !ok || v != "12" {
		t.Fatalf("%s: tope_criterios: 12:\n%s", ca, raw)
	}
	again, err := Load(root, "precios")
	if err != nil || again.TopePalabras == nil || *again.TopePalabras != 0 {
		t.Fatalf("%s: lo escrito se vuelve a leer: %+v %v", ca, again, err)
	}
}

// CA-796: las negativas no tocan el archivo y van antes de leer el item: un
// valor negativo, sin motivo, con motivo en blanco y motivo sin tope.
func TestCA796_CambiarSeNiegaSinTocarElArchivo(t *testing.T) {
	const ca = "CA-796"
	root := pprCon(t)
	antes := pprLeer(t, root)
	for _, c := range []struct {
		nombre string
		c      Cambios
		check  func(error) bool
	}{
		{"palabras -1", Cambios{TopePalabras: pprInt(-1), TopeMotivo: pprStr("x")}, func(e error) bool { return e != nil && strings.Contains(e.Error(), pprTopeErrPal) }},
		{"criterios -3", Cambios{TopeCriterios: pprInt(-3), TopeMotivo: pprStr("x")}, func(e error) bool { return e != nil && strings.Contains(e.Error(), pprTopeErrCri) }},
		{"sin motivo", Cambios{TopePalabras: pprInt(900)}, func(e error) bool { return errors.Is(e, ErrTopeSinMotivo) }},
		{"motivo vacio", Cambios{TopeCriterios: pprInt(9), TopeMotivo: pprStr("")}, func(e error) bool { return errors.Is(e, ErrTopeSinMotivo) }},
		{"motivo en blanco", Cambios{TopePalabras: pprInt(900), TopeMotivo: pprStr(" \t\n ")}, func(e error) bool { return errors.Is(e, ErrTopeSinMotivo) }},
		{"motivo sin tope", Cambios{TopeMotivo: pprStr("porque si")}, func(e error) bool { return errors.Is(e, ErrMotivoSinTope) }},
		{"motivo sin tope con riesgo", Cambios{Riesgo: pprStr("alto"), TopeMotivo: pprStr("porque si")}, func(e error) bool { return errors.Is(e, ErrMotivoSinTope) }},
	} {
		_, _, cambio, err := Cambiar(root, "precios", c.c)
		if !c.check(err) || cambio {
			t.Fatalf("%s: %s se niega: %v %v", ca, c.nombre, cambio, err)
		}
		if !bytes.Equal(pprLeer(t, root), antes) {
			t.Fatalf("%s: %s no toca el archivo", ca, c.nombre)
		}
		// antes de leer el item: con un slug sin item es la misma negativa
		if _, _, _, err := Cambiar(root, "no-existe", c.c); !c.check(err) || errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s: %s se niega antes de buscar el item: %v", ca, c.nombre, err)
		}
	}
	if ErrTopeSinMotivo.Error() != `item set: cambiar el tope del spec pide --motivo "<por que>"` {
		t.Fatalf("%s: ErrTopeSinMotivo: %q", ca, ErrTopeSinMotivo)
	}
	if ErrMotivoSinTope.Error() != "item set: --motivo va con --tope-palabras o --tope-criterios" {
		t.Fatalf("%s: ErrMotivoSinTope: %q", ca, ErrMotivoSinTope)
	}
}

// CA-796: el mismo tope con el mismo motivo no escribe nada; un motivo
// distinto con el mismo valor escribe, con tope_por y tope_en nuevos
// (aclaracion 8).
func TestCA796_MismoTopeMismoMotivoNoEscribe(t *testing.T) {
	const ca = "CA-796"
	root := pprCon(t)
	if _, _, _, err := Cambiar(root, "precios", Cambios{TopePalabras: pprInt(1000), TopeMotivo: pprStr("largo")}); err != nil {
		t.Fatal(err)
	}
	uno := pprLeer(t, root)
	_, _, cambio, err := Cambiar(root, "precios", Cambios{TopePalabras: pprInt(1000), TopeMotivo: pprStr("largo")})
	if err != nil || cambio || !bytes.Equal(pprLeer(t, root), uno) {
		t.Fatalf("%s: el mismo tope y motivo no escribe: %v %v", ca, cambio, err)
	}
	itGit(t, root, "config", "user.name", "Ana Perez")
	time.Sleep(1100 * time.Millisecond)
	_, despues, cambio, err := Cambiar(root, "precios", Cambios{TopePalabras: pprInt(1000), TopeMotivo: pprStr("mas largo")})
	if err != nil || !cambio || despues.TopeMotivo != "mas largo" || despues.TopePor != "Ana Perez <test@hoom.dev>" {
		t.Fatalf("%s: otro motivo con el mismo valor escribe: %+v %v %v", ca, despues, cambio, err)
	}
	dos := pprLeer(t, root)
	v1, _ := retyLinea(uno, "tope_en")
	v2, _ := retyLinea(dos, "tope_en")
	if v1 == v2 {
		t.Fatalf("%s: tope_en nuevo: %q %q", ca, v1, v2)
	}
	// un cambio que no toca el tope deja tope_por y tope_en
	cinco := 5.0
	if _, _, _, err := Cambiar(root, "precios", Cambios{PresupuestoUSD: &cinco}); err != nil {
		t.Fatal(err)
	}
	tres := pprLeer(t, root)
	if a, _ := retyLinea(tres, "tope_en"); a != v2 {
		t.Fatalf("%s: un cambio sin tope no reescribe tope_en: %q %q", ca, a, v2)
	}
	if a, _ := retyLinea(tres, "tope_por"); a != "Ana Perez <test@hoom.dev>" {
		t.Fatalf("%s: un cambio sin tope deja tope_por: %q", ca, a)
	}
}

// CA-796: TarjetaDe lee el item de la tarjeta: hay sin error; sin item, hay
// false; un item ilegible es err con hay true. RiesgoDeLanzamiento es el
// riesgo declarado, alto con el item ilegible, normal sin item o sin slug.
func TestCA796_TarjetaDeYRiesgoDeLanzamiento(t *testing.T) {
	const ca = "CA-796"
	root := pprCon(t)
	if _, _, _, err := Cambiar(root, "precios", Cambios{Riesgo: pprStr("bajo"), TopeCriterios: pprInt(6), TopeMotivo: pprStr("x")}); err != nil {
		t.Fatal(err)
	}
	it, hay, err := TarjetaDe(root, "precios")
	if err != nil || !hay || it.Riesgo != "bajo" || it.TopeCriterios == nil || *it.TopeCriterios != 6 {
		t.Fatalf("%s: TarjetaDe lee la tarjeta: %+v %v %v", ca, it, hay, err)
	}
	if _, hay, err := TarjetaDe(root, "otra"); hay || err != nil {
		t.Fatalf("%s: sin item, hay false sin error: %v %v", ca, hay, err)
	}
	if _, hay, err := TarjetaDe(root, "No Es Slug"); hay || err != nil {
		t.Fatalf("%s: un slug invalido no es tarjeta: %v %v", ca, hay, err)
	}
	if r := RiesgoDeLanzamiento(root, "precios"); r != "bajo" {
		t.Fatalf("%s: el riesgo de lanzamiento es el declarado: %q", ca, r)
	}
	for _, s := range []string{"", "otra"} {
		if r := RiesgoDeLanzamiento(root, s); r != "normal" {
			t.Fatalf("%s: sin tarjeta (%q) el riesgo es normal: %q", ca, s, r)
		}
	}
	itEscribir(t, root, ".hoom/items/rota.yaml", "titulo: [\nno es yaml\n")
	if _, hay, err := TarjetaDe(root, "rota"); !hay || err == nil {
		t.Fatalf("%s: un item ilegible es err con hay true: %v %v", ca, hay, err)
	}
	if r := RiesgoDeLanzamiento(root, "rota"); r != "alto" {
		t.Fatalf("%s: con el item ilegible el riesgo de lanzamiento es alto: %q", ca, r)
	}
	// un item con una clave de tope invalida tambien es ilegible
	itEscribir(t, root, ".hoom/items/mala.yaml", itItemValido("Mala", "2026-09-22T15:04:05Z", "tope_palabras: -1\n"))
	if _, hay, err := TarjetaDe(root, "mala"); !hay || err == nil || !strings.Contains(err.Error(), pprTopeErrPal) {
		t.Fatalf("%s: un tope invalido hace ilegible la tarjeta: %v %v", ca, hay, err)
	}
}
