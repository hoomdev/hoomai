// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// item (CA-788): SetRiesgo y Cambiar escriben riesgo_por (el user.name
// <user.email> de git) y riesgo_en (UTC, RFC 3339) cuando el riesgo cambia de
// verdad, tambien con normal, que deja el archivo sin `riesgo`; con el riesgo
// que ya tenia no escriben nada (aclaracion 21); ningun otro verbo las
// escribe; Parse las acepta, con o sin `riesgo`. Escritos desde el spec, las
// aclaraciones de la ronda 0 y las firmas publicas, sin leer la
// implementacion.
package item

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

const retyQuien = "hoom test <test@hoom.dev>"

var retyRFC3339UTC = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)

// retyLinea es el valor de la linea `<clave>: ` del archivo (sin comillas),
// y si esta.
func retyLinea(raw []byte, clave string) (string, bool) {
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, clave+":") {
			v := strings.TrimSpace(strings.TrimPrefix(l, clave+":"))
			return strings.Trim(v, `"'`), true
		}
	}
	return "", false
}

// retyCuando exige riesgo_en en UTC, RFC 3339, entre desde y hasta.
func retyCuando(t *testing.T, ca string, raw []byte, desde, hasta time.Time) time.Time {
	t.Helper()
	v, ok := retyLinea(raw, "riesgo_en")
	if !ok {
		t.Fatalf("%s: el archivo lleva riesgo_en:\n%s", ca, raw)
	}
	if !retyRFC3339UTC.MatchString(v) {
		t.Fatalf("%s: riesgo_en va en UTC, RFC 3339: %q", ca, v)
	}
	en, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		t.Fatalf("%s: riesgo_en no parsea: %v", ca, err)
	}
	if en.Before(desde.Add(-time.Second)) || en.After(hasta.Add(time.Second)) {
		t.Fatalf("%s: riesgo_en es ahora (%v..%v): %v", ca, desde, hasta, en)
	}
	return en
}

func retyQuienEs(t *testing.T, ca string, raw []byte, quien string) {
	t.Helper()
	if v, ok := retyLinea(raw, "riesgo_por"); !ok || v != quien {
		t.Fatalf("%s: riesgo_por es %q: %q %v\n%s", ca, quien, v, ok, raw)
	}
}

// CA-788: SetRiesgo escribe riesgo_por y riesgo_en al cambiar, tambien a
// normal (que deja el archivo sin `riesgo`), y no toca el archivo con el
// riesgo que ya tenia.
func TestCA788_SetRiesgoEscribeQuienYCuando(t *testing.T) {
	const ca = "CA-788"
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Precios", Slug: "precios"}); err != nil {
		t.Fatal(err)
	}
	raw0, _ := os.ReadFile(Path(root, "precios"))
	for _, clave := range []string{"riesgo_por", "riesgo_en", "riesgo"} {
		if _, ok := retyLinea(raw0, clave); ok {
			t.Fatalf("%s: Add sin riesgo no escribe %s:\n%s", ca, clave, raw0)
		}
	}
	desde := time.Now().UTC()
	ant, cambio, err := SetRiesgo(root, "precios", "alto")
	hasta := time.Now().UTC()
	if err != nil || !cambio || ant != "normal" {
		t.Fatalf("%s: de normal a alto: (%q, %v, %v)", ca, ant, cambio, err)
	}
	raw, _ := os.ReadFile(Path(root, "precios"))
	retyQuienEs(t, ca, raw, retyQuien)
	en1 := retyCuando(t, ca, raw, desde, hasta)
	if r, _ := retyLinea(raw, "riesgo"); r != "alto" {
		t.Fatalf("%s: el archivo lleva riesgo: alto:\n%s", ca, raw)
	}
	it, err := Load(root, "precios")
	if err != nil || it.Riesgo != "alto" || it.RiesgoPor != retyQuien || it.RiesgoEn == nil || !it.RiesgoEn.Equal(en1) {
		t.Fatalf("%s: Load trae riesgo_por y riesgo_en: %+v %v", ca, it, err)
	}

	// a normal: sin `riesgo`, con riesgo_por y riesgo_en de este cambio
	time.Sleep(1100 * time.Millisecond)
	desde = time.Now().UTC()
	ant, cambio, err = SetRiesgo(root, "precios", "normal")
	hasta = time.Now().UTC()
	if err != nil || !cambio || ant != "alto" {
		t.Fatalf("%s: de alto a normal: (%q, %v, %v)", ca, ant, cambio, err)
	}
	raw, _ = os.ReadFile(Path(root, "precios"))
	if _, ok := retyLinea(raw, "riesgo"); ok {
		t.Fatalf("%s: con normal el archivo queda sin `riesgo`:\n%s", ca, raw)
	}
	retyQuienEs(t, ca, raw, retyQuien)
	if en2 := retyCuando(t, ca, raw, desde, hasta); !en2.After(en1) {
		t.Fatalf("%s: riesgo_en es el de este cambio: %v despues de %v", ca, en2, en1)
	}
	if it, err := Load(root, "precios"); err != nil || it.Riesgo != "normal" || it.RiesgoPor != retyQuien || it.RiesgoEn == nil {
		t.Fatalf("%s: %+v %v", ca, it, err)
	}

	// el que ya tenia: no escribe (ni riesgo_por ni riesgo_en)
	st1, _ := os.Stat(Path(root, "precios"))
	time.Sleep(10 * time.Millisecond)
	ant, cambio, err = SetRiesgo(root, "precios", "normal")
	if err != nil || cambio || ant != "normal" {
		t.Fatalf("%s: con el que ya tenia: (%q, %v, %v)", ca, ant, cambio, err)
	}
	raw2, _ := os.ReadFile(Path(root, "precios"))
	st2, _ := os.Stat(Path(root, "precios"))
	if !bytes.Equal(raw, raw2) || !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatalf("%s: con el riesgo que ya tenia no escribe:\n%s\n---\n%s", ca, raw, raw2)
	}

	// otra persona: su identidad de git
	itGit(t, root, "config", "user.name", "Ana Perez")
	itGit(t, root, "config", "user.email", "ana@hoom.dev")
	if _, cambio, err := SetRiesgo(root, "precios", "bajo"); err != nil || !cambio {
		t.Fatalf("%s: a bajo: %v %v", ca, cambio, err)
	}
	raw, _ = os.ReadFile(Path(root, "precios"))
	retyQuienEs(t, ca, raw, "Ana Perez <ana@hoom.dev>")
	if r, _ := retyLinea(raw, "riesgo"); r != "bajo" {
		t.Fatalf("%s: %s", ca, raw)
	}
	// el resto del item sigue igual: creado_por es de quien lo creo
	if it, err := Load(root, "precios"); err != nil || it.CreadoPor != retyQuien || it.Titulo != "Precios" {
		t.Fatalf("%s: las demas claves no cambian: %+v %v", ca, it, err)
	}
}

// CA-788: un riesgo invalido o vacio no escribe riesgo_por ni riesgo_en.
func TestCA788_SetRiesgoInvalidoNoEscribeQuien(t *testing.T) {
	const ca = "CA-788"
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Precios", Slug: "precios"}); err != nil {
		t.Fatal(err)
	}
	raw0, _ := os.ReadFile(Path(root, "precios"))
	for _, r := range []string{"", "critico", "ALTO", " alto"} {
		if _, cambio, err := SetRiesgo(root, "precios", r); err == nil || cambio {
			t.Fatalf("%s: SetRiesgo(%q) es un error: %v %v", ca, r, cambio, err)
		}
		if raw, _ := os.ReadFile(Path(root, "precios")); !bytes.Equal(raw0, raw) {
			t.Fatalf("%s: SetRiesgo(%q) no escribe:\n%s", ca, r, raw)
		}
	}
}

// CA-788: Cambiar escribe riesgo_por y riesgo_en solo cuando cambia el
// riesgo; un cambio de otra clave, o con el mismo riesgo, los deja como
// estaban.
func TestCA788_CambiarEscribeQuienSoloSiCambiaElRiesgo(t *testing.T) {
	const ca = "CA-788"
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Precios", Slug: "precios"}); err != nil {
		t.Fatal(err)
	}
	cinco, siete := 5.0, 7.0
	// solo el presupuesto: sin riesgo_por ni riesgo_en
	if _, _, cambio, err := Cambiar(root, "precios", Cambios{PresupuestoUSD: &cinco}); err != nil || !cambio {
		t.Fatalf("%s: Cambiar presupuesto: %v %v", ca, cambio, err)
	}
	raw, _ := os.ReadFile(Path(root, "precios"))
	for _, clave := range []string{"riesgo_por", "riesgo_en"} {
		if _, ok := retyLinea(raw, clave); ok {
			t.Fatalf("%s: un cambio sin riesgo no escribe %s:\n%s", ca, clave, raw)
		}
	}
	// el riesgo que ya tenia (normal) con otro presupuesto: tampoco
	normal := "normal"
	if _, _, cambio, err := Cambiar(root, "precios", Cambios{Riesgo: &normal, PresupuestoUSD: &siete}); err != nil || !cambio {
		t.Fatalf("%s: Cambiar normal y presupuesto: %v %v", ca, cambio, err)
	}
	raw, _ = os.ReadFile(Path(root, "precios"))
	if _, ok := retyLinea(raw, "riesgo_por"); ok {
		t.Fatalf("%s: con el mismo riesgo no escribe riesgo_por:\n%s", ca, raw)
	}
	// el riesgo cambia: los escribe
	alto := "alto"
	desde := time.Now().UTC()
	antes, despues, cambio, err := Cambiar(root, "precios", Cambios{Riesgo: &alto})
	hasta := time.Now().UTC()
	if err != nil || !cambio || antes.Riesgo != "normal" || despues.Riesgo != "alto" || despues.RiesgoPor != retyQuien || despues.RiesgoEn == nil {
		t.Fatalf("%s: Cambiar a alto: %+v %+v %v %v", ca, antes, despues, cambio, err)
	}
	raw, _ = os.ReadFile(Path(root, "precios"))
	retyQuienEs(t, ca, raw, retyQuien)
	en := retyCuando(t, ca, raw, desde, hasta)
	if !despues.RiesgoEn.Equal(en) {
		t.Fatalf("%s: despues trae el riesgo_en del archivo: %v %v", ca, despues.RiesgoEn, en)
	}
	// luego otro presupuesto: riesgo_por y riesgo_en quedan como estaban
	itGit(t, root, "config", "user.name", "Ana Perez")
	time.Sleep(1100 * time.Millisecond)
	if _, _, _, err := Cambiar(root, "precios", Cambios{PresupuestoUSD: &cinco}); err != nil {
		t.Fatal(err)
	}
	raw2, _ := os.ReadFile(Path(root, "precios"))
	retyQuienEs(t, ca, raw2, retyQuien)
	v1, _ := retyLinea(raw, "riesgo_en")
	v2, _ := retyLinea(raw2, "riesgo_en")
	if v1 != v2 {
		t.Fatalf("%s: un cambio sin riesgo deja riesgo_en: %q %q", ca, v1, v2)
	}
}

// CA-788: ningun otro verbo escribe riesgo_por ni riesgo_en: Add (tambien
// con riesgo), AddSession y MarkDone; y los que ya estan se conservan.
func TestCA788_LosOtrosVerbosNoLasEscriben(t *testing.T) {
	const ca = "CA-788"
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Alto", Slug: "alto", Riesgo: "alto"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, Draft{Titulo: "Puesto", Slug: "puesto"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := SetRiesgo(root, "puesto", "bajo"); err != nil {
		t.Fatal(err)
	}
	rawPuesto, _ := os.ReadFile(Path(root, "puesto"))
	quien, _ := retyLinea(rawPuesto, "riesgo_por")
	cuando, _ := retyLinea(rawPuesto, "riesgo_en")
	itGit(t, root, "config", "user.name", "Otra Persona")
	for _, s := range []string{"alto", "puesto"} {
		if _, err := AddSession(root, s, "codex", time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := MarkDone(root, s, "abcd", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(Path(root, "alto"))
	for _, clave := range []string{"riesgo_por", "riesgo_en"} {
		if _, ok := retyLinea(raw, clave); ok {
			t.Fatalf("%s: Add, AddSession y MarkDone no escriben %s:\n%s", ca, clave, raw)
		}
	}
	raw, _ = os.ReadFile(Path(root, "puesto"))
	if q, _ := retyLinea(raw, "riesgo_por"); q != quien {
		t.Fatalf("%s: riesgo_por se conserva: %q -> %q", ca, quien, q)
	}
	if c, _ := retyLinea(raw, "riesgo_en"); c != cuando {
		t.Fatalf("%s: riesgo_en se conserva: %q -> %q", ca, cuando, c)
	}
}

// CA-788: Parse acepta riesgo_por y riesgo_en, con o sin `riesgo`; sigue
// estricto con una clave desconocida, y riesgo_en tiene que ser una fecha.
func TestCA788_ParseAceptaLasClaves(t *testing.T) {
	const ca = "CA-788"
	en := time.Date(2026, 11, 20, 10, 0, 5, 0, time.UTC)
	for _, extra := range []string{
		"riesgo: alto\nriesgo_por: \"Ana <ana@hoom.dev>\"\nriesgo_en: 2026-11-20T10:00:05Z\n",
		"riesgo_por: \"Ana <ana@hoom.dev>\"\nriesgo_en: 2026-11-20T10:00:05Z\n",
	} {
		it, err := Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", extra)))
		if err != nil {
			t.Fatalf("%s: Parse acepta las claves:\n%s\n%v", ca, extra, err)
		}
		if it.RiesgoPor != "Ana <ana@hoom.dev>" || it.RiesgoEn == nil || !it.RiesgoEn.Equal(en) {
			t.Fatalf("%s: Parse lee riesgo_por y riesgo_en: %+v", ca, it)
		}
		want := "normal"
		if strings.HasPrefix(extra, "riesgo: alto") {
			want = "alto"
		}
		if it.Riesgo != want {
			t.Fatalf("%s: el riesgo es %q: %q", ca, want, it.Riesgo)
		}
		raw, err := json.Marshal(it)
		if err != nil || !strings.Contains(string(raw), `"riesgo_por":"Ana \u003cana@hoom.dev\u003e"`) && !strings.Contains(string(raw), `"riesgo_por":"Ana <ana@hoom.dev>"`) {
			t.Fatalf("%s: el JSON del item trae riesgo_por: %s %v", ca, raw, err)
		}
		if !strings.Contains(string(raw), `"riesgo_en":"2026-11-20T10:00:05Z"`) {
			t.Fatalf("%s: el JSON del item trae riesgo_en: %s", ca, raw)
		}
	}
	for _, extra := range []string{
		"riesgo_quien: Ana\n",
		"riesgo_en: ayer\n",
	} {
		if _, err := Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", extra))); err == nil {
			t.Fatalf("%s: Parse sigue estricto: %q es un error", ca, extra)
		}
	}
	// sin las claves, el JSON no las trae
	it, err := Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", "")))
	if err != nil {
		t.Fatal(err)
	}
	if raw, _ := json.Marshal(it); strings.Contains(string(raw), "riesgo_por") || strings.Contains(string(raw), "riesgo_en") {
		t.Fatalf("%s: sin las claves el JSON no las trae: %s", ca, raw)
	}
}
