// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-378): el item gana la clave opcional auto, con un solo valor,
// hasta-humano, que activa la cinta del Studio. Parse la acepta, rechaza
// cualquier otro valor con el mensaje del contrato y sigue rechazando las
// claves desconocidas. Un item sin auto se lee y se escribe byte a byte como
// antes, y las escrituras de hoom (Add, MarkDone, AddSession) conservan auto.
package item

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

const auLinea = "auto: hasta-humano\n"

// auSinLinea quita la linea de auto de un archivo de item.
func auSinLinea(raw []byte) []byte {
	return bytes.Replace(raw, []byte(auLinea), nil, 1)
}

// CA-378: el vocabulario de auto es uno solo.
func TestCA378_VocabularioDeAuto(t *testing.T) {
	if AutoHastaHumano != "hasta-humano" || len(Autos) != 1 || Autos[0] != AutoHastaHumano {
		t.Fatalf("CA-378: auto tiene un solo valor, hasta-humano: %q %v", AutoHastaHumano, Autos)
	}
}

// CA-378: Parse acepta auto: hasta-humano, con o sin presupuesto, en
// cualquier lugar del archivo, y List no lo manda a warnings.
func TestCA378_ParseAceptaAuto(t *testing.T) {
	raw := itItemValido("Precios", "2026-09-22T15:04:05Z", "presupuesto_usd: 5\n"+auLinea)
	it, err := Parse("precios", []byte(raw))
	if err != nil {
		t.Fatalf("CA-378: un item con auto: hasta-humano es valido: %v\n%s", err, raw)
	}
	if it.Auto != AutoHastaHumano || it.PresupuestoUSD == nil || *it.PresupuestoUSD != 5 || it.Titulo != "Precios" {
		t.Fatalf("CA-378: Parse lee auto y las demas claves como siempre: %+v", it)
	}

	// con auto y sin presupuesto se lee bien (la cinta se detiene en el caso 10)
	raw = itItemValido("Precios", "2026-09-22T15:04:05Z", auLinea)
	if it, err = Parse("precios", []byte(raw)); err != nil || it.Auto != AutoHastaHumano || it.PresupuestoUSD != nil {
		t.Fatalf("CA-378: auto sin presupuesto se lee bien: %+v %v", it, err)
	}

	// al principio del archivo, con sesiones, hecho_en y commit_final
	raw = auLinea + itItemValido("Precios", "2026-09-22T15:04:05Z",
		"hecho_en: 2026-09-25T10:00:00Z\ncommit_final: abcd\n"+acSesionesOK)
	if it, err = Parse("precios", []byte(raw)); err != nil || it.Auto != AutoHastaHumano || len(it.Sesiones) != 2 || it.HechoEn == nil {
		t.Fatalf("CA-378: auto con las demas claves opcionales: %+v %v", it, err)
	}

	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", "presupuesto_usd: 5\n"+auLinea))
	items, warns, err := List(root)
	if err != nil || len(items) != 1 || len(warns) != 0 || items[0].Auto != AutoHastaHumano {
		t.Fatalf("CA-378: List lee el item con auto sin avisos: items=%+v warns=%q err=%v", items, warns, err)
	}

	// el JSON del item lleva auto (la tarjeta lo muestra como piloto automatico)
	js, _ := json.Marshal(items[0])
	if !strings.Contains(string(js), `"auto":"hasta-humano"`) {
		t.Fatalf("CA-378: el JSON del item trae auto: %s", js)
	}
}

// CA-378: otro valor es invalido con el mensaje del contrato, y las claves
// desconocidas siguen invalidando.
func TestCA378_ParseRechazaOtroValorDeAuto(t *testing.T) {
	base := itItemValido("Precios", "2026-09-22T15:04:05Z", "presupuesto_usd: 5\n")
	for _, v := range []string{"siempre", "Hasta-Humano", "hasta_humano", "hasta-humano-y-mas", "writer"} {
		_, err := Parse("precios", []byte(base+"auto: "+v+"\n"))
		want := `auto "` + v + `" fuera del vocabulario (hasta-humano)`
		if err == nil || err.Error() != want {
			t.Fatalf("CA-378: auto %q es invalido con %q, fue %v", v, want, err)
		}
	}
	// un valor que no es texto tampoco vale
	for _, extra := range []string{"auto: [hasta-humano]\n", "auto:\n  modo: hasta-humano\n"} {
		if _, err := Parse("precios", []byte(base+extra)); err == nil {
			t.Fatalf("CA-378: auto que no es un valor del vocabulario es invalido:\n%s", base+extra)
		}
	}
	// las claves desconocidas siguen invalidando, con o sin auto
	for _, extra := range []string{auLinea + "columna: writer\n", "piloto: hasta-humano\n", "auto_run: true\n"} {
		_, err := Parse("precios", []byte(base+extra))
		if err == nil || !strings.Contains(err.Error(), "clave desconocida") {
			t.Fatalf("CA-378: una clave desconocida sigue invalidando el item: %v\n%s", err, base+extra)
		}
	}
	// y List lo manda a warnings sin fallar
	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/malo.yaml", base+"auto: siempre\n")
	itEscribir(t, root, ".hoom/items/bueno.yaml", base+auLinea)
	items, warns, err := List(root)
	if err != nil || len(items) != 1 || items[0].Slug != "bueno" || len(warns) != 1 ||
		!strings.Contains(warns[0], ".hoom/items/malo.yaml") || !strings.Contains(warns[0], `auto "siempre" fuera del vocabulario (hasta-humano)`) {
		t.Fatalf("CA-378: el item con auto invalido va a warnings con su motivo: items=%+v warns=%q err=%v", items, warns, err)
	}
}

// CA-378: un item sin auto se lee y se escribe byte a byte como antes: Add y
// MarkDone escriben lo mismo que antes de esta spec, sin la clave auto, y el
// JSON del item tampoco la trae.
func TestCA378_SinAutoByteAByteComoAntes(t *testing.T) {
	root := itRepo(t)
	cinco := 5.0
	it, err := Add(root, Draft{Titulo: "Precios por region", Pedido: "que el catalogo muestre precios", PresupuestoUSD: &cinco})
	if err != nil {
		t.Fatal(err)
	}
	path := Path(root, it.Slug)
	raw, _ := os.ReadFile(path)
	if want := acBytesC1(t, it); !bytes.Equal(raw, want) {
		t.Fatalf("CA-378: Add sin auto escribe el item como antes:\nquiere:\n%s\nfue:\n%s", want, raw)
	}
	if strings.Contains(string(raw), "auto") {
		t.Fatalf("CA-378: un item sin auto no escribe la clave:\n%s", raw)
	}
	leido, err := Load(root, it.Slug)
	if err != nil || leido.Auto != "" {
		t.Fatalf("CA-378: el item sin auto se lee sin auto: %+v %v", leido, err)
	}
	js, _ := json.Marshal(leido)
	if strings.Contains(string(js), `"auto"`) {
		t.Fatalf("CA-378: el JSON de un item sin auto no trae la clave: %s", js)
	}
	if wrote, err := MarkDone(root, it.Slug, "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c", time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)); err != nil || !wrote {
		t.Fatalf("CA-378: MarkDone sobre un item sin auto: %v %v", wrote, err)
	}
	raw, _ = os.ReadFile(path)
	hecho, err := Load(root, it.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if want := acBytesC1(t, hecho); !bytes.Equal(raw, want) {
		t.Fatalf("CA-378: MarkDone sin auto escribe el item como antes:\nquiere:\n%s\nfue:\n%s", want, raw)
	}

	// escrito a mano sin auto, reescrito por AddSession: no gana la clave
	p := itEscribir(t, root, ".hoom/items/mano.yaml", itItemValido("A mano", "2026-09-22T15:04:05Z", "pedido: algo\n"))
	if _, err := AddSession(root, "mano", "claude", time.Now()); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(p); strings.Contains(string(raw), "auto") {
		t.Fatalf("CA-378: reescribir un item sin auto no agrega la clave:\n%s", raw)
	}
}

// CA-378: Add escribe auto cuando el borrador lo trae (lo demas, igual que
// antes), y MarkDone y AddSession lo conservan. Otro valor no se escribe.
func TestCA378_AddEscribeAutoYLasEscriturasLoConservan(t *testing.T) {
	root := itRepo(t)
	cinco := 5.0
	it, err := Add(root, Draft{Titulo: "Cinta", Pedido: "que avance sola", PresupuestoUSD: &cinco, Auto: AutoHastaHumano})
	if err != nil {
		t.Fatalf("CA-378: Add con auto hasta-humano: %v", err)
	}
	if it.Auto != AutoHastaHumano {
		t.Fatalf("CA-378: el Item devuelto lleva auto: %+v", it)
	}
	path := Path(root, it.Slug)
	raw, _ := os.ReadFile(path)
	if bytes.Count(raw, []byte(auLinea)) != 1 {
		t.Fatalf("CA-378: el archivo tiene la linea %q:\n%s", auLinea, raw)
	}
	if want := acBytesC1(t, it); !bytes.Equal(auSinLinea(raw), want) {
		t.Fatalf("CA-378: fuera de auto, el item se escribe como antes:\nquiere:\n%s\nfue (sin auto):\n%s", want, auSinLinea(raw))
	}
	leido, err := Load(root, it.Slug)
	if err != nil || leido.Auto != AutoHastaHumano {
		t.Fatalf("CA-378: el item escrito con auto se lee con auto: %+v %v", leido, err)
	}

	if wrote, err := AddSession(root, it.Slug, "claude", time.Date(2026, 9, 24, 9, 0, 0, 0, time.UTC)); err != nil || !wrote {
		t.Fatalf("CA-378: AddSession sobre un item con auto: %v %v", wrote, err)
	}
	if leido, err = Load(root, it.Slug); err != nil || leido.Auto != AutoHastaHumano || len(leido.Sesiones) != 1 {
		t.Fatalf("CA-378: AddSession conserva auto: %+v %v", leido, err)
	}
	if wrote, err := MarkDone(root, it.Slug, "abcd", time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)); err != nil || !wrote {
		t.Fatalf("CA-378: MarkDone sobre un item con auto: %v %v", wrote, err)
	}
	if leido, err = Load(root, it.Slug); err != nil || leido.Auto != AutoHastaHumano || leido.HechoEn == nil || len(leido.Sesiones) != 1 {
		t.Fatalf("CA-378: MarkDone conserva auto: %+v %v", leido, err)
	}

	// un borrador con otro valor de auto no escribe un item que Parse rechazaria
	if _, err := Validate(Draft{Titulo: "Otra", PresupuestoUSD: &cinco, Auto: "siempre"}); err == nil {
		t.Fatal("CA-378: Validate rechaza auto fuera del vocabulario")
	}
	if _, err := Add(root, Draft{Titulo: "Otra", PresupuestoUSD: &cinco, Auto: "siempre"}); err == nil {
		t.Fatal("CA-378: Add rechaza auto fuera del vocabulario")
	}
	if _, err := os.Stat(Path(root, "otra")); !os.IsNotExist(err) {
		t.Fatalf("CA-378: un auto invalido no deja archivo: %v", err)
	}
}
