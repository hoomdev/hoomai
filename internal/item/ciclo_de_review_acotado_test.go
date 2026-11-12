// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// el item (CA-615, CA-616, CA-617): la clave opcional riesgo. Parse la lee
// estricta, normal es la ausencia de la clave (ningun verbo escribe
// `riesgo: normal`) y SetRiesgo cambia solo esa clave. Escritos desde el spec
// y las firmas publicas, sin leer la implementacion.
package item

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const crTexto = `riesgo %q fuera del vocabulario (bajo, normal, alto)`

// CA-615: Parse acepta bajo, normal y alto; sin la clave da normal.
func TestCA615_ParseLeeElRiesgo(t *testing.T) {
	for _, r := range []string{"bajo", "normal", "alto"} {
		raw := itItemValido("Precios", "2026-09-22T15:04:05Z", "riesgo: "+r+"\n")
		it, err := Parse("precios", []byte(raw))
		if err != nil {
			t.Fatalf("CA-615: riesgo: %s es valido: %v", r, err)
		}
		if it.Riesgo != r {
			t.Fatalf("CA-615: Parse lee riesgo %q: %+v", r, it)
		}
	}
	it, err := Parse("precios", []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", "")))
	if err != nil || it.Riesgo != "normal" {
		t.Fatalf("CA-615: sin la clave el riesgo es normal: %q %v", it.Riesgo, err)
	}
	// al principio del archivo y con las demas claves opcionales
	raw := "riesgo: alto\n" + itItemValido("Precios", "2026-09-22T15:04:05Z", "pedido: algo\nauto: hasta-humano\npresupuesto_usd: 3\n")
	if it, err = Parse("precios", []byte(raw)); err != nil || it.Riesgo != "alto" || it.Auto != AutoHastaHumano {
		t.Fatalf("CA-615: riesgo con las demas claves: %+v %v", it, err)
	}
}

// CA-615: cualquier otro valor es el error con el texto exacto.
func TestCA615_ParseRechazaOtroRiesgo(t *testing.T) {
	for _, r := range []string{"x", "Alto", "medio", "[alto]", "1", "true", "altó", "bajo normal"} {
		raw := itItemValido("Precios", "2026-09-22T15:04:05Z", "riesgo: "+r+"\n")
		_, err := Parse("precios", []byte(raw))
		if err == nil {
			t.Fatalf("CA-615: riesgo: %s no es del vocabulario:\n%s", r, raw)
		}
		if r == "x" && !strings.Contains(err.Error(), fmt.Sprintf(crTexto, "x")) {
			t.Fatalf("CA-615: el error dice %q: %v", fmt.Sprintf(crTexto, "x"), err)
		}
	}
	// el texto exacto del CA, con "x"
	_, err := Parse("precios", []byte(itItemValido("P", "2026-09-22T15:04:05Z", "riesgo: x\n")))
	if err == nil || !strings.Contains(err.Error(), `riesgo "x" fuera del vocabulario (bajo, normal, alto)`) {
		t.Fatalf("CA-615: %v", err)
	}
	// y una clave desconocida sigue invalidando
	if _, err := Parse("precios", []byte(itItemValido("P", "2026-09-22T15:04:05Z", "riesgos: alto\n"))); err == nil {
		t.Fatal("CA-615: una clave desconocida (riesgos) sigue invalidando el item")
	}
}

// CA-615: List manda a warnings el item con un riesgo invalido, y lee bien
// los validos.
func TestCA615_ListAvisaElRiesgoInvalido(t *testing.T) {
	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/bueno.yaml", itItemValido("Bueno", "2026-09-22T15:04:05Z", "riesgo: alto\n"))
	itEscribir(t, root, ".hoom/items/malo.yaml", itItemValido("Malo", "2026-09-22T15:04:06Z", "riesgo: critico\n"))
	items, warns, err := List(root)
	if err != nil || len(items) != 1 || items[0].Slug != "bueno" || items[0].Riesgo != "alto" {
		t.Fatalf("CA-615: List lee el bueno: %+v %v", items, err)
	}
	if len(warns) != 1 || !strings.Contains(warns[0], "malo") {
		t.Fatalf("CA-615: el riesgo invalido es un aviso: %q", warns)
	}
}

// CA-616: Add con riesgo alto o bajo escribe la clave; con normal o vacio no.
func TestCA616_AddEscribeElRiesgoSoloSiNoEsNormal(t *testing.T) {
	root := itRepo(t)
	for _, c := range []struct{ slug, r, linea string }{
		{"con-alto", "alto", "riesgo: alto\n"},
		{"con-bajo", "bajo", "riesgo: bajo\n"},
		{"con-normal", "normal", ""},
		{"sin", "", ""},
	} {
		it, err := Add(root, Draft{Titulo: "T " + c.slug, Slug: c.slug, Riesgo: c.r})
		if err != nil {
			t.Fatalf("CA-616: Add con riesgo %q: %v", c.r, err)
		}
		raw, _ := os.ReadFile(Path(root, it.Slug))
		if c.linea != "" {
			if bytes.Count(raw, []byte(c.linea)) != 1 {
				t.Fatalf("CA-616: el item lleva %q:\n%s", c.linea, raw)
			}
		} else if strings.Contains(string(raw), "riesgo") {
			t.Fatalf("CA-616: con riesgo %q el archivo no lleva la clave:\n%s", c.r, raw)
		}
		leido, err := Load(root, it.Slug)
		want := c.r
		if want == "" {
			want = "normal"
		}
		if err != nil || leido.Riesgo != want {
			t.Fatalf("CA-616: el item se lee con riesgo %q: %+v %v", want, leido, err)
		}
	}
}

// CA-616: un riesgo fuera del vocabulario no crea el item, con el texto de
// Parse; Validate lo recorta.
func TestCA616_AddRechazaOtroRiesgoSinCrear(t *testing.T) {
	root := itRepo(t)
	if _, err := Validate(Draft{Titulo: "Otra", Riesgo: "critico"}); err == nil || err.Error() != fmt.Sprintf(crTexto, "critico") {
		t.Fatalf("CA-616: Validate da el texto de Parse: %v", err)
	}
	if _, err := Add(root, Draft{Titulo: "Otra", Riesgo: "critico"}); err == nil {
		t.Fatal("CA-616: Add rechaza el riesgo fuera del vocabulario")
	}
	if _, err := os.Stat(Path(root, "otra")); !os.IsNotExist(err) {
		t.Fatalf("CA-616: un riesgo invalido no deja archivo: %v", err)
	}
	d, err := Validate(Draft{Titulo: "Recorte", Riesgo: "  alto "})
	if err != nil || d.Riesgo != "alto" {
		t.Fatalf("CA-616: Validate recorta el riesgo: %+v %v", d, err)
	}
}

// CA-617: SetRiesgo cambia solo esa clave: las demas conservan sus valores.
func TestCA617_SetRiesgoCambiaSoloLaClave(t *testing.T) {
	root := itRepo(t)
	cinco := 5.0
	it, err := Add(root, Draft{Titulo: "Cinta", Pedido: "que avance", PresupuestoUSD: &cinco, Auto: AutoHastaHumano, Tipo: "bug", Prioridad: "alta"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddSession(root, it.Slug, "claude", time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	antes, err := Load(root, it.Slug)
	if err != nil {
		t.Fatal(err)
	}
	ant, cambio, err := SetRiesgo(root, it.Slug, "alto")
	if err != nil || !cambio || ant != "normal" {
		t.Fatalf("CA-617: de normal a alto: (%q, %v, %v)", ant, cambio, err)
	}
	despues, err := Load(root, it.Slug)
	if err != nil || despues.Riesgo != "alto" {
		t.Fatalf("CA-617: el item queda alto: %+v %v", despues, err)
	}
	despues.Riesgo = antes.Riesgo
	if !crIgual(antes, despues) {
		t.Fatalf("CA-617: las demas claves conservan sus valores:\nantes   %+v\ndespues %+v", antes, despues)
	}
	raw, _ := os.ReadFile(Path(root, it.Slug))
	if bytes.Count(raw, []byte("riesgo: alto\n")) != 1 {
		t.Fatalf("CA-617: el archivo lleva riesgo: alto:\n%s", raw)
	}

	// el mismo riesgo: no escribe nada
	st1, _ := os.Stat(Path(root, it.Slug))
	raw1, _ := os.ReadFile(Path(root, it.Slug))
	time.Sleep(10 * time.Millisecond)
	ant, cambio, err = SetRiesgo(root, it.Slug, "alto")
	if err != nil || cambio || ant != "alto" {
		t.Fatalf("CA-617: con el riesgo que ya tenia: (%q, %v, %v)", ant, cambio, err)
	}
	st2, _ := os.Stat(Path(root, it.Slug))
	raw2, _ := os.ReadFile(Path(root, it.Slug))
	if !bytes.Equal(raw1, raw2) || !st1.ModTime().Equal(st2.ModTime()) {
		t.Fatal("CA-617: con el riesgo que ya tenia no escribe")
	}

	// a normal: el archivo queda sin la clave
	ant, cambio, err = SetRiesgo(root, it.Slug, "normal")
	if err != nil || !cambio || ant != "alto" {
		t.Fatalf("CA-617: de alto a normal: (%q, %v, %v)", ant, cambio, err)
	}
	raw, _ = os.ReadFile(Path(root, it.Slug))
	if strings.Contains(string(raw), "riesgo") {
		t.Fatalf("CA-617: con normal el archivo queda sin la clave:\n%s", raw)
	}
	// y vuelve a normal byte a byte como lo escribe hoom sin la clave
	if final, err := Load(root, it.Slug); err != nil || final.Riesgo != "normal" || final.Auto != AutoHastaHumano || len(final.Sesiones) != 1 {
		t.Fatalf("CA-617: %+v %v", final, err)
	}
}

// CA-617: SetRiesgo sobre un item escrito a mano conserva las demas claves.
func TestCA617_SetRiesgoSobreItemAMano(t *testing.T) {
	root := itRepo(t)
	p := itEscribir(t, root, ".hoom/items/mano.yaml", itItemValido("A mano", "2026-09-22T15:04:05Z", "pedido: algo\nriesgo: bajo\n"))
	ant, cambio, err := SetRiesgo(root, "mano", "alto")
	if err != nil || !cambio || ant != "bajo" {
		t.Fatalf("CA-617: (%q %v %v)", ant, cambio, err)
	}
	it, err := Load(root, "mano")
	if err != nil || it.Riesgo != "alto" || it.Pedido != "algo" || it.Titulo != "A mano" || it.Tipo != "feature" {
		t.Fatalf("CA-617: %+v %v", it, err)
	}
	raw, _ := os.ReadFile(p)
	if strings.Contains(string(raw), "bajo") {
		t.Fatalf("CA-617: el riesgo viejo no queda:\n%s", raw)
	}
}

// CA-617 (hostil): un riesgo fuera del vocabulario o vacio, un item que no
// existe o invalido: error y nada escrito.
func TestCA617_SetRiesgoErroresNoEscriben(t *testing.T) {
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Uno", Slug: "uno"}); err != nil {
		t.Fatal(err)
	}
	raw0, _ := os.ReadFile(Path(root, "uno"))
	for _, r := range []string{"", "critico", "ALTO", " alto"} {
		if _, cambio, err := SetRiesgo(root, "uno", r); err == nil || cambio {
			t.Fatalf("CA-617: SetRiesgo(%q) es un error: %v %v", r, cambio, err)
		}
		if raw, _ := os.ReadFile(Path(root, "uno")); !bytes.Equal(raw, raw0) {
			t.Fatalf("CA-617: SetRiesgo(%q) no escribe", r)
		}
	}
	if _, _, err := SetRiesgo(root, "uno", "critico"); err == nil || err.Error() != fmt.Sprintf(crTexto, "critico") {
		t.Fatalf("CA-617: el error es el de riesgo.Validar: %v", err)
	}
	_, _, err := SetRiesgo(root, "nadie", "alto")
	if err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("CA-617: un item que no existe es un error que envuelve os.ErrNotExist: %v", err)
	}
	if _, err := os.Stat(Path(root, "nadie")); !os.IsNotExist(err) {
		t.Fatal("CA-617: no crea el item que no existe")
	}
	p := itEscribir(t, root, ".hoom/items/roto.yaml", "titulo: [\n")
	rawRoto, _ := os.ReadFile(p)
	if _, _, err := SetRiesgo(root, "roto", "alto"); err == nil {
		t.Fatal("CA-617: un item invalido es un error")
	}
	if raw, _ := os.ReadFile(p); !bytes.Equal(raw, rawRoto) {
		t.Fatal("CA-617: un item invalido no se reescribe")
	}
}

// CA-617 (concurrencia): SetRiesgo y AddSession a la vez sobre el mismo item
// no se pisan: los dos efectos quedan (el mismo candado que MarkDone).
func TestCA617_SetRiesgoYAddSessionConcurrentes(t *testing.T) {
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Doble", Slug: "doble"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		var wg sync.WaitGroup
		r := "alto"
		if i%2 == 1 {
			r = "bajo"
		}
		wg.Add(2)
		go func() { defer wg.Done(); _, _, _ = SetRiesgo(root, "doble", r) }()
		go func() { defer wg.Done(); _, _ = AddSession(root, "doble", "claude", time.Now()) }()
		wg.Wait()
		it, err := Load(root, "doble")
		if err != nil || it.Riesgo != r || len(it.Sesiones) != i+1 {
			t.Fatalf("CA-617: vuelta %d: riesgo %q y %d sesiones: %+v %v", i, r, i+1, it, err)
		}
	}
}

// CA-616/CA-617: ningun verbo escribe `riesgo: normal`: MarkDone y AddSession
// sobre un item sin la clave no la agregan, y sobre uno con alto la conservan.
func TestCA616_LasEscriturasNoEscribenNormal(t *testing.T) {
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Sin", Slug: "sin"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Add(root, Draft{Titulo: "Con", Slug: "con", Riesgo: "alto"}); err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"sin", "con"} {
		if _, err := AddSession(root, s, "codex", time.Now()); err != nil {
			t.Fatal(err)
		}
		if _, err := MarkDone(root, s, "abcd", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	raw, _ := os.ReadFile(Path(root, "sin"))
	if strings.Contains(string(raw), "riesgo") {
		t.Fatalf("CA-616: un item sin riesgo no gana la clave:\n%s", raw)
	}
	raw, _ = os.ReadFile(Path(root, "con"))
	if bytes.Count(raw, []byte("riesgo: alto\n")) != 1 {
		t.Fatalf("CA-616: las escrituras conservan riesgo: alto:\n%s", raw)
	}
}

// crIgual compara dos items por valor (los punteros, por lo que apuntan).
func crIgual(a, b Item) bool {
	f := func(p *float64) string {
		if p == nil {
			return "nil"
		}
		return fmt.Sprint(*p)
	}
	g := func(p *time.Time) string {
		if p == nil {
			return "nil"
		}
		return p.UTC().Format(time.RFC3339Nano)
	}
	if f(a.PresupuestoUSD) != f(b.PresupuestoUSD) || g(a.HechoEn) != g(b.HechoEn) {
		return false
	}
	a.PresupuestoUSD, b.PresupuestoUSD, a.HechoEn, b.HechoEn = nil, nil, nil, nil
	return fmt.Sprintf("%+v", a) == fmt.Sprintf("%+v", b)
}
