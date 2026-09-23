// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-349): el item gana la clave sesiones, la unica que se agrega. Parse la
// acepta con provider y abierta_en en cada entrada, rechaza una entrada sin
// ellos y sigue rechazando las claves desconocidas. Un item sin sesiones se
// lee y se escribe byte a byte como antes, y AddSession agrega al final sin
// tocar las demas claves.
package item

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// acItemC1 es el item tal como lo escribia hoom antes de esta spec: las
// mismas claves, en el mismo orden, sin sesiones.
type acItemC1 struct {
	Titulo         string     `yaml:"titulo"`
	Tipo           string     `yaml:"tipo"`
	Prioridad      string     `yaml:"prioridad"`
	Pedido         string     `yaml:"pedido,omitempty"`
	CreadoPor      string     `yaml:"creado_por"`
	CreadoEn       time.Time  `yaml:"creado_en"`
	PresupuestoUSD *float64   `yaml:"presupuesto_usd,omitempty"`
	HechoEn        *time.Time `yaml:"hecho_en,omitempty"`
	CommitFinal    string     `yaml:"commit_final,omitempty"`
}

// acBytesC1 son los bytes que hoom escribia para it antes de esta spec.
func acBytesC1(t *testing.T, it Item) []byte {
	t.Helper()
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(acItemC1{Titulo: it.Titulo, Tipo: it.Tipo, Prioridad: it.Prioridad, Pedido: it.Pedido,
		CreadoPor: it.CreadoPor, CreadoEn: it.CreadoEn, PresupuestoUSD: it.PresupuestoUSD, HechoEn: it.HechoEn,
		CommitFinal: it.CommitFinal}); err != nil {
		t.Fatal(err)
	}
	if err := enc.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func acLeer(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("CA-349: no pude leer %s: %v", path, err)
	}
	return raw
}

const acSesionesOK = "sesiones:\n" +
	"  - provider: claude\n" +
	"    abierta_por: \"Henry Orellana <henry@hoom.dev>\"\n" +
	"    abierta_en: 2026-09-23T18:00:00Z\n" +
	"  - provider: codex\n" +
	"    abierta_en: 2026-09-23T19:30:00Z\n"

// CA-349: Parse acepta sesiones con provider y abierta_en en cada entrada
// (abierta_por es opcional), en su orden, junto a las demas claves.
func TestCA349_ParseAceptaSesiones(t *testing.T) {
	raw := itItemValido("Precios", "2026-09-22T15:04:05Z", "pedido: el pedido\npresupuesto_usd: 5\n"+acSesionesOK)
	it, err := Parse("precios", []byte(raw))
	if err != nil {
		t.Fatalf("CA-349: un item con sesiones es valido: %v\n%s", err, raw)
	}
	if len(it.Sesiones) != 2 {
		t.Fatalf("CA-349: Parse lee las dos sesiones: %+v", it.Sesiones)
	}
	s0, s1 := it.Sesiones[0], it.Sesiones[1]
	if s0.Provider != "claude" || s0.AbiertaPor != "Henry Orellana <henry@hoom.dev>" ||
		!s0.AbiertaEn.Equal(time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)) {
		t.Fatalf("CA-349: la primera sesion es claude, de Henry, a las 18:00Z: %+v", s0)
	}
	if s1.Provider != "codex" || s1.AbiertaPor != "" || !s1.AbiertaEn.Equal(time.Date(2026, 9, 23, 19, 30, 0, 0, time.UTC)) {
		t.Fatalf("CA-349: la segunda sesion es codex sin abierta_por (opcional), a las 19:30Z: %+v", s1)
	}
	if it.Titulo != "Precios" || it.Pedido != "el pedido" || it.PresupuestoUSD == nil || *it.PresupuestoUSD != 5 {
		t.Fatalf("CA-349: las demas claves se leen como siempre: %+v", it)
	}
	// sesiones puede ir en cualquier lugar del archivo, y con hecho_en
	raw = "sesiones:\n  - provider: gemini\n    abierta_en: 2026-09-23T18:00:00Z\n" +
		itItemValido("Precios", "2026-09-22T15:04:05Z", "hecho_en: 2026-09-25T10:00:00Z\ncommit_final: abcd\n")
	if it, err = Parse("precios", []byte(raw)); err != nil || len(it.Sesiones) != 1 || it.Sesiones[0].Provider != "gemini" {
		t.Fatalf("CA-349: sesiones al principio del archivo tambien vale: %+v %v", it.Sesiones, err)
	}
	// y List no lo manda a warnings
	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z", acSesionesOK))
	items, warns, err := List(root)
	if err != nil || len(items) != 1 || len(warns) != 0 || len(items[0].Sesiones) != 2 {
		t.Fatalf("CA-349: List lee el item con sesiones sin avisos: items=%+v warns=%q err=%v", items, warns, err)
	}
}

// CA-349: una entrada sin provider o sin abierta_en invalida el item, y las
// claves desconocidas siguen siendo invalidas.
func TestCA349_ParseRechazaSesionesIncompletas(t *testing.T) {
	base := itItemValido("Precios", "2026-09-22T15:04:05Z", "")
	faltan := map[string]string{
		"entrada sin provider":       "sesiones:\n  - abierta_por: \"Henry <h@hoom.dev>\"\n    abierta_en: 2026-09-23T18:00:00Z\n",
		"entrada con provider vacio": "sesiones:\n  - provider: \"\"\n    abierta_en: 2026-09-23T18:00:00Z\n",
		"entrada sin abierta_en":     "sesiones:\n  - provider: claude\n    abierta_por: \"Henry <h@hoom.dev>\"\n",
		"segunda entrada sin abierta_en": "sesiones:\n  - provider: claude\n    abierta_en: 2026-09-23T18:00:00Z\n" +
			"  - provider: codex\n",
	}
	for caso, extra := range faltan {
		_, err := Parse("precios", []byte(base+extra))
		if err == nil {
			t.Fatalf("CA-349: (%s) el item debe ser invalido:\n%s", caso, base+extra)
		}
		if strings.Contains(err.Error(), "clave desconocida") {
			t.Fatalf("CA-349: (%s) sesiones es una clave conocida: lo invalida la entrada incompleta, no la clave: %v", caso, err)
		}
	}
	malos := map[string]string{
		"abierta_en que no es fecha": "sesiones:\n  - provider: claude\n    abierta_en: ayer\n",
		"sesiones escalar":           "sesiones: claude\n",
		"sesiones mapa":              "sesiones:\n  provider: claude\n  abierta_en: 2026-09-23T18:00:00Z\n",
	}
	for caso, extra := range malos {
		if _, err := Parse("precios", []byte(base+extra)); err == nil {
			t.Fatalf("CA-349: (%s) el item debe ser invalido:\n%s", caso, base+extra)
		}
	}
	// las claves desconocidas siguen invalidando, con o sin sesiones
	for _, extra := range []string{"columna: hecho\n", "estado: writer\n", acSesionesOK + "columna: hecho\n"} {
		_, err := Parse("precios", []byte(base+extra))
		if err == nil || !strings.Contains(err.Error(), "clave desconocida") {
			t.Fatalf("CA-349: una clave desconocida sigue invalidando el item: %v\n%s", err, base+extra)
		}
	}
	// y el mensaje de la clave desconocida ya lista sesiones entre las validas
	_, err := Parse("precios", []byte(base+"columna: hecho\n"))
	if err == nil || !strings.Contains(err.Error(), "sesiones") {
		t.Fatalf("CA-349: la clave desconocida lista las validas, sesiones incluida: %v", err)
	}
}

// CA-349: un item sin sesiones se lee y se escribe byte a byte como antes:
// Add y MarkDone escriben lo mismo que escribia hoom antes de esta spec, sin
// ninguna clave sesiones, y el JSON del item tampoco la trae.
func TestCA349_SinSesionesByteAByteComoAntes(t *testing.T) {
	root := itRepo(t)
	cinco := 5.0
	it, err := Add(root, Draft{Titulo: "Precios por region", Pedido: "que el catalogo muestre precios", PresupuestoUSD: &cinco})
	if err != nil {
		t.Fatal(err)
	}
	path := Path(root, it.Slug)
	raw := acLeer(t, path)
	if want := acBytesC1(t, it); !bytes.Equal(raw, want) {
		t.Fatalf("CA-349: Add escribe el item como antes:\nquiere:\n%s\nfue:\n%s", want, raw)
	}
	if strings.Contains(string(raw), "sesiones") {
		t.Fatalf("CA-349: un item sin sesiones no escribe la clave:\n%s", raw)
	}
	leido, err := Load(root, it.Slug)
	if err != nil {
		t.Fatalf("CA-349: el item sin sesiones se lee: %v", err)
	}
	if len(leido.Sesiones) != 0 {
		t.Fatalf("CA-349: sin sesiones en el archivo no hay sesiones: %+v", leido.Sesiones)
	}
	js, _ := json.Marshal(leido)
	if strings.Contains(string(js), "sesiones") {
		t.Fatalf("CA-349: el JSON de un item sin sesiones no trae la clave: %s", js)
	}

	commit := "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c"
	if wrote, err := MarkDone(root, it.Slug, commit, time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)); err != nil || !wrote {
		t.Fatalf("CA-349: MarkDone sobre un item sin sesiones: %v %v", wrote, err)
	}
	raw = acLeer(t, path)
	hecho, err := Load(root, it.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if want := acBytesC1(t, hecho); !bytes.Equal(raw, want) {
		t.Fatalf("CA-349: MarkDone escribe el item como antes:\nquiere:\n%s\nfue:\n%s", want, raw)
	}

	// un item escrito a mano sin sesiones, releido y reescrito por MarkDone,
	// tampoco gana la clave
	p := itEscribir(t, root, ".hoom/items/mano.yaml", itItemValido("A mano", "2026-09-22T15:04:05Z", "pedido: algo\n"))
	if _, err := MarkDone(root, "mano", "abcd", time.Now()); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(acLeer(t, p)), "sesiones") {
		t.Fatalf("CA-349: reescribir un item sin sesiones no agrega la clave:\n%s", acLeer(t, p))
	}
}

// CA-349: AddSession agrega una entrada AL FINAL de sesiones (en el orden de
// las llamadas, no por fecha), con abierta_por = gitx.Identity(root), y no
// toca ninguna otra clave. MarkDone despues conserva las sesiones.
func TestCA349_AddSessionAgregaAlFinal(t *testing.T) {
	root := itRepo(t)
	cinco := 5.0
	it, err := Add(root, Draft{Titulo: "Precios por region", Pedido: "que muestre precios", PresupuestoUSD: &cinco})
	if err != nil {
		t.Fatal(err)
	}
	path := Path(root, it.Slug)
	antesRaw := acLeer(t, path)
	antes := itYAML(t, path)

	t1 := time.Date(2026, 9, 23, 18, 0, 0, 0, time.UTC)
	wrote, err := AddSession(root, it.Slug, "claude", t1)
	if err != nil || !wrote {
		t.Fatalf("CA-349: AddSession sobre un item valido escribe: wrote=%v err=%v", wrote, err)
	}
	despuesRaw := acLeer(t, path)
	if !bytes.HasPrefix(despuesRaw, antesRaw) {
		t.Fatalf("CA-349: AddSession no toca las demas claves: el archivo de antes sigue igual y sesiones va al final\nantes:\n%s\ndespues:\n%s", antesRaw, despuesRaw)
	}
	despues := itYAML(t, path)
	for k, v := range antes {
		if !reflect.DeepEqual(despues[k], v) {
			t.Fatalf("CA-349: AddSession conserva %s: %v -> %v", k, v, despues[k])
		}
	}
	if len(despues) != len(antes)+1 {
		t.Fatalf("CA-349: AddSession solo agrega sesiones: %v", despues)
	}
	if _, ok := despues["sesiones"].([]any); !ok {
		t.Fatalf("CA-349: sesiones es una lista en el YAML: %v", despues["sesiones"])
	}
	got, err := Load(root, it.Slug)
	if err != nil {
		t.Fatalf("CA-349: el item con su sesion sigue siendo valido: %v", err)
	}
	quien := gitx.Identity(root)
	if quien == "" || quien == "desconocido" {
		t.Fatalf("CA-349: el repo de prueba tiene identidad git: %q", quien)
	}
	if len(got.Sesiones) != 1 || got.Sesiones[0].Provider != "claude" || got.Sesiones[0].AbiertaPor != quien ||
		!got.Sesiones[0].AbiertaEn.Equal(t1) {
		t.Fatalf("CA-349: la sesion trae provider, abierta_por = %q y abierta_en: %+v", quien, got.Sesiones)
	}

	// la segunda, con una fecha ANTERIOR, va igual al final; y una tercera
	// del mismo provider no se funde con la primera
	t2 := t1.Add(-time.Hour)
	if wrote, err := AddSession(root, it.Slug, "codex", t2); err != nil || !wrote {
		t.Fatalf("CA-349: la segunda sesion: %v %v", wrote, err)
	}
	enOtraZona := time.Date(2026, 9, 23, 17, 30, 0, 0, time.FixedZone("-03", -3*3600))
	if wrote, err := AddSession(root, it.Slug, "claude", enOtraZona); err != nil || !wrote {
		t.Fatalf("CA-349: la tercera sesion: %v %v", wrote, err)
	}
	got, err = Load(root, it.Slug)
	if err != nil {
		t.Fatal(err)
	}
	var provs []string
	for _, s := range got.Sesiones {
		provs = append(provs, s.Provider)
		if s.AbiertaPor != quien {
			t.Fatalf("CA-349: cada sesion lleva abierta_por = %q: %+v", quien, s)
		}
	}
	if strings.Join(provs, ",") != "claude,codex,claude" {
		t.Fatalf("CA-349: AddSession agrega al final, en el orden de las llamadas: %v", provs)
	}
	if !got.Sesiones[1].AbiertaEn.Equal(t2) || !got.Sesiones[2].AbiertaEn.Equal(enOtraZona) {
		t.Fatalf("CA-349: abierta_en es el instante recibido: %+v", got.Sesiones)
	}
	if got.Titulo != it.Titulo || got.Pedido != it.Pedido || got.PresupuestoUSD == nil || *got.PresupuestoUSD != 5 ||
		!got.CreadoEn.Equal(it.CreadoEn) || got.CreadoPor != it.CreadoPor {
		t.Fatalf("CA-349: las demas claves quedan como estaban: %+v", got)
	}

	// MarkDone conserva las sesiones (y agrega solo lo suyo)
	if wrote, err := MarkDone(root, it.Slug, "abcd", time.Now()); err != nil || !wrote {
		t.Fatalf("CA-349: MarkDone sobre un item con sesiones: %v %v", wrote, err)
	}
	hecho, err := Load(root, it.Slug)
	if err != nil || hecho.HechoEn == nil || !reflect.DeepEqual(hecho.Sesiones, got.Sesiones) {
		t.Fatalf("CA-349: MarkDone conserva las sesiones: %+v %v", hecho.Sesiones, err)
	}

	// sin restos de la escritura en .hoom/items/
	entradas, err := os.ReadDir(filepath.Join(root, ".hoom", "items"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 1 {
		var nombres []string
		for _, e := range entradas {
			nombres = append(nombres, e.Name())
		}
		t.Fatalf("CA-349: AddSession escribe atomico, sin temporales: %v", nombres)
	}
}

// CA-349: sin item AddSession es (false, nil) y no crea nada; un item
// invalido o ilegible es error y queda byte a byte igual.
func TestCA349_AddSessionSinItemEInvalido(t *testing.T) {
	root := itRepo(t)
	wrote, err := AddSession(root, "no-existe", "claude", time.Now())
	if err != nil || wrote {
		t.Fatalf("CA-349: sin item AddSession es (false, nil): %v %v", wrote, err)
	}
	if _, err := os.Stat(Path(root, "no-existe")); !os.IsNotExist(err) {
		t.Fatalf("CA-349: sin item AddSession no crea uno (err=%v)", err)
	}
	for nombre, cuerpo := range map[string]string{
		"invalido": itItemValido("Malo", "2026-09-22T15:04:05Z", "columna: hecho\n"),
		"ilegible": "titulo: [roto\n",
		"sesion-incompleta": itItemValido("Incompleta", "2026-09-22T15:04:05Z",
			"sesiones:\n  - provider: claude\n"),
	} {
		p := itEscribir(t, root, ".hoom/items/"+nombre+".yaml", cuerpo)
		wrote, err := AddSession(root, nombre, "codex", time.Now())
		if err == nil || wrote {
			t.Fatalf("CA-349: AddSession sobre un item %s es error: %v %v", nombre, wrote, err)
		}
		if raw := acLeer(t, p); string(raw) != cuerpo {
			t.Fatalf("CA-349: el item %s queda byte a byte igual:\n%s", nombre, raw)
		}
	}
}
