// Tests adversariales del spec .hoom/specs/items-y-columna-derivada.md
// (CA-263, CA-264, CA-265, CA-266, CA-289, CA-290): el item como ARCHIVO que
// escribe una persona. Estricto al leer (una clave desconocida, como
// `columna:`, lo invalida), atomico al escribir, y el cierre lo registra
// `hoom task done` con MarkDone.
package item

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/gitx"
)

func itGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func itEscribir(t *testing.T, root, rel, body string) string {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// itRepo: un proyecto git real con identidad configurada y un commit.
func itRepo(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	itGit(t, root, "init", "-b", "main")
	itGit(t, root, "config", "user.email", "test@hoom.dev")
	itGit(t, root, "config", "user.name", "hoom test")
	itEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	itGit(t, root, "add", "-A")
	itGit(t, root, "commit", "-m", "inicial")
	return root
}

// itYAML devuelve el item crudo en un mapa generico (yaml.v3 deja las fechas
// como string cuando el destino es interface{}).
func itYAML(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("no pude leer %s: %v", path, err)
	}
	var m map[string]any
	if err := yaml.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s no es YAML: %v\n%s", path, err, raw)
	}
	return m
}

var itRFC3339Segundos = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)

// itFechaUTC exige una fecha RFC3339 en UTC con precision de segundos.
func itFechaUTC(t *testing.T, ca, campo string, v any) time.Time {
	t.Helper()
	s, ok := v.(string)
	if !ok {
		if tt, ok2 := v.(time.Time); ok2 {
			s = tt.Format(time.RFC3339Nano)
		} else {
			t.Fatalf("%s: %s debe ser una fecha, fue %T %v", ca, campo, v, v)
		}
	}
	if !itRFC3339Segundos.MatchString(s) {
		t.Fatalf("%s: %s va en UTC RFC3339 con segundos (ej 2026-09-22T15:04:05Z), fue %q", ca, campo, s)
	}
	tt, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("%s: %s no parsea como RFC3339: %v", ca, campo, err)
	}
	return tt
}

// itItemValido es un item escrito a mano con los obligatorios.
func itItemValido(titulo, creado string, extra string) string {
	return "titulo: " + titulo + "\n" +
		"tipo: feature\n" +
		"prioridad: media\n" +
		"creado_por: \"hoom test <test@hoom.dev>\"\n" +
		"creado_en: " + creado + "\n" + extra
}

// CA-263: Slugify cumple los ejemplos del contrato.
func TestCA263_Slugify(t *testing.T) {
	casos := map[string]string{
		"Precios por región (v2)!":     "precios-por-region-v2",
		"Ñandú Rápido":                 "nandu-rapido",
		"CAFÉ con LECHE":               "cafe-con-leche",
		"  --Hola   Mundo--  ":         "hola-mundo",
		"a__b..c":                      "a-b-c",
		"123 Go":                       "123-go",
		"ya-es-un-slug":                "ya-es-un-slug",
		"¡¡¡":                          "",
		"":                             "",
		"   ":                          "",
		strings.Repeat("a", 70):        strings.Repeat("a", 64),
		strings.Repeat("a", 63) + " b": strings.Repeat("a", 63), // corte en el '-': sin '-' final
	}
	for in, want := range casos {
		got := Slugify(in)
		if got != want {
			t.Fatalf("CA-263: Slugify(%q) = %q, esperaba %q", in, got, want)
		}
		if got != "" && !ValidSlug(got) {
			t.Fatalf("CA-263: lo que sale de Slugify(%q) es un slug valido: %q", in, got)
		}
		if len(got) > MaxSlug {
			t.Fatalf("CA-263: Slugify recorta a %d caracteres: %q", MaxSlug, got)
		}
	}
}

// CA-263: la forma del slug es la de `hoom task start`, hasta 64.
func TestCA263_ValidSlug(t *testing.T) {
	for _, s := range []string{"a", "0", "precios-por-region", "a-b-c-1", strings.Repeat("x", 64)} {
		if !ValidSlug(s) {
			t.Fatalf("CA-263: %q es un slug valido", s)
		}
	}
	for _, s := range []string{"", "-a", "A", "a_b", "a b", "Foo Bar", "a/b", "..", "ñandu", strings.Repeat("x", 65)} {
		if ValidSlug(s) {
			t.Fatalf("CA-263: %q no es un slug valido", s)
		}
	}
	if got := Path("/p", "precios"); got != filepath.Join("/p", ".hoom", "items", "precios.yaml") {
		t.Fatalf("CA-263: Path es .hoom/items/<slug>.yaml bajo root, fue %q", got)
	}
}

// CA-263: Add con los defaults: el archivo tiene titulo, tipo feature,
// prioridad media, creado_por = la identidad de 'hoom spec approve' y
// creado_en en UTC, sin presupuesto_usd, hecho_en ni commit_final.
func TestCA263_AddConDefaults(t *testing.T) {
	root := itRepo(t)
	antes := time.Now().UTC().Truncate(time.Second)
	it, err := Add(root, Draft{Titulo: "Precios por región (v2)!"})
	despues := time.Now().UTC()
	if err != nil {
		t.Fatalf("CA-263: Add: %v", err)
	}
	if it.Slug != "precios-por-region-v2" {
		t.Fatalf("CA-263: el slug sale de Slugify(titulo), fue %q", it.Slug)
	}
	path := filepath.Join(root, ".hoom", "items", "precios-por-region-v2.yaml")
	m := itYAML(t, path)
	if m["titulo"] != "Precios por región (v2)!" || m["tipo"] != "feature" || m["prioridad"] != "media" {
		t.Fatalf("CA-263: titulo y defaults en el archivo: %v", m)
	}
	for _, k := range []string{"presupuesto_usd", "hecho_en", "commit_final", "slug", "columna", "estado"} {
		if _, ok := m[k]; ok {
			t.Fatalf("CA-263: el item recien creado no lleva %q: %v", k, m)
		}
	}
	creado := itFechaUTC(t, "CA-263", "creado_en", m["creado_en"])
	if creado.Before(antes) || creado.After(despues) {
		t.Fatalf("CA-263: creado_en es el momento del alta (%v..%v), fue %v", antes, despues, creado)
	}

	// la identidad es la misma que registra 'hoom spec approve' en este repo
	spec := itEscribir(t, root, ".hoom/specs/otra.md", "# x\n")
	rec, _, err := approval.Approve(root, spec)
	if err != nil {
		t.Fatal(err)
	}
	if m["creado_por"] != rec.ApprovedBy || m["creado_por"] != gitx.Identity(root) {
		t.Fatalf("CA-263: creado_por %q debe ser la identidad de spec approve %q y de gitx.Identity %q",
			m["creado_por"], rec.ApprovedBy, gitx.Identity(root))
	}
	if it.CreadoPor != rec.ApprovedBy || it.Titulo != "Precios por región (v2)!" {
		t.Fatalf("CA-263: el Item devuelto refleja el archivo: %+v", it)
	}

	// --slug fija el slug
	otro, err := Add(root, Draft{Titulo: "Precios por región (v2)!", Slug: "otro"})
	if err != nil || otro.Slug != "otro" {
		t.Fatalf("CA-263: con Slug explicito el archivo es otro.yaml: %+v %v", otro, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "items", "otro.yaml")); err != nil {
		t.Fatalf("CA-263: falta .hoom/items/otro.yaml: %v", err)
	}
}

// CA-264: lo que el draft trae queda en el archivo; presupuesto_usd es un
// numero.
func TestCA264_AddGuardaLosCampos(t *testing.T) {
	root := itRepo(t)
	cinco := 5.5
	it, err := Add(root, Draft{Titulo: "Arreglar login", Tipo: "bug", Prioridad: "alta",
		Pedido: "que el login no se cuelgue\ncon dos lineas", PresupuestoUSD: &cinco})
	if err != nil {
		t.Fatalf("CA-264: Add: %v", err)
	}
	m := itYAML(t, Path(root, it.Slug))
	if m["tipo"] != "bug" || m["prioridad"] != "alta" || m["pedido"] != "que el login no se cuelgue\ncon dos lineas" {
		t.Fatalf("CA-264: tipo, prioridad y pedido quedan en el archivo: %v", m)
	}
	switch v := m["presupuesto_usd"].(type) {
	case float64:
		if v != 5.5 {
			t.Fatalf("CA-264: presupuesto_usd 5.5, fue %v", v)
		}
	default:
		t.Fatalf("CA-264: presupuesto_usd es un numero, fue %T %v", v, v)
	}
	back, err := Load(root, it.Slug)
	if err != nil || back.PresupuestoUSD == nil || *back.PresupuestoUSD != 5.5 || back.Pedido != it.Pedido {
		t.Fatalf("CA-264: Load devuelve lo que Add escribio: %+v %v", back, err)
	}
}

// CA-264: Validate aplica defaults y rechaza lo que esta fuera de
// vocabulario, sin tocar disco.
func TestCA264_ValidateVocabulario(t *testing.T) {
	d, err := Validate(Draft{Titulo: "Precios por región"})
	if err != nil || d.Tipo != DefaultTipo || d.Prioridad != DefaultPrioridad || d.Slug != "precios-por-region" {
		t.Fatalf("CA-264: Validate aplica los defaults y resuelve el slug: %+v %v", d, err)
	}
	if DefaultTipo != "feature" || DefaultPrioridad != "media" {
		t.Fatalf("CA-264: defaults del contrato: tipo feature, prioridad media (%q, %q)", DefaultTipo, DefaultPrioridad)
	}
	for _, tipo := range Tipos {
		if _, err := Validate(Draft{Titulo: "x", Tipo: tipo}); err != nil {
			t.Fatalf("CA-264: el tipo %q es del vocabulario: %v", tipo, err)
		}
	}
	for _, p := range Prioridades {
		if _, err := Validate(Draft{Titulo: "x", Prioridad: p}); err != nil {
			t.Fatalf("CA-264: la prioridad %q es del vocabulario: %v", p, err)
		}
	}
	cero, menos := 0.0, -1.0
	malos := []Draft{
		{Titulo: "x", Tipo: "epica"},
		{Titulo: "x", Prioridad: "urgente"},
		{Titulo: "x", PresupuestoUSD: &cero},
		{Titulo: "x", PresupuestoUSD: &menos},
		{Titulo: ""},
		{Titulo: "   "},
		{Titulo: "¡¡¡"},
		{Titulo: "x", Slug: "Foo Bar"},
		{Titulo: "x", Slug: "-x"},
		{Titulo: "x", Slug: strings.Repeat("a", 65)},
	}
	for _, d := range malos {
		if _, err := Validate(d); err == nil {
			t.Fatalf("CA-264: %+v debe rechazarse", d)
		}
	}
	// un titulo sin slug se salva con un slug explicito
	if d, err := Validate(Draft{Titulo: "¡¡¡", Slug: "exclamaciones"}); err != nil || d.Slug != "exclamaciones" {
		t.Fatalf("CA-264: con --slug un titulo sin slug es valido: %+v %v", d, err)
	}
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "x", Tipo: "epica"}); err == nil {
		t.Fatal("CA-264: Add rechaza lo que Validate rechaza")
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", "items")); !os.IsNotExist(err) {
		t.Fatalf("CA-264: un rechazo no crea nada bajo .hoom/items/ (err=%v)", err)
	}
}

// CA-265: un slug que ya existe envuelve ErrExists, nombra el archivo, y el
// item existente queda byte a byte igual. Un spec con ese slug no es error.
func TestCA265_AddExistenteNoPisa(t *testing.T) {
	root := itRepo(t)
	original := []byte(itItemValido("Precios", "2026-09-22T15:04:05Z", "pedido: el original\n"))
	path := itEscribir(t, root, ".hoom/items/precios.yaml", string(original))
	_, err := Add(root, Draft{Titulo: "Precios"})
	if !errors.Is(err, ErrExists) {
		t.Fatalf("CA-265: un slug existente envuelve ErrExists, fue %v", err)
	}
	if !strings.Contains(err.Error(), filepath.Join(".hoom", "items", "precios.yaml")) && !strings.Contains(err.Error(), ".hoom/items/precios.yaml") {
		t.Fatalf("CA-265: el error nombra el archivo: %v", err)
	}
	if !strings.Contains(err.Error(), "--slug") {
		t.Fatalf("CA-265: el error sugiere --slug: %v", err)
	}
	if raw, _ := os.ReadFile(path); !bytes.Equal(raw, original) {
		t.Fatalf("CA-265: el item existente queda byte a byte igual:\n%s", raw)
	}

	// adoptar un spec que ya existe es un caso normal
	itEscribir(t, root, ".hoom/specs/catalogo.md", "# Spec: catalogo\n")
	it, err := Add(root, Draft{Titulo: "Catalogo"})
	if err != nil || it.Slug != "catalogo" {
		t.Fatalf("CA-265: con .hoom/specs/catalogo.md existente el item se crea: %+v %v", it, err)
	}
}

// CA-266: Parse es estricto: una clave desconocida (columna:), un campo
// obligatorio que falta o un valor fuera de vocabulario invalidan el item.
func TestCA266_ParseEstricto(t *testing.T) {
	ok := itItemValido("Precios", "2026-09-22T15:04:05Z",
		"pedido: |\n  que muestre precios\npresupuesto_usd: 5\nhecho_en: 2026-09-25T10:00:00Z\ncommit_final: 3f2a0000000000000000000000000000000000aa\n")
	it, err := Parse("precios", []byte(ok))
	if err != nil {
		t.Fatalf("CA-266: el item del contrato es valido: %v", err)
	}
	if it.Slug != "precios" || it.Titulo != "Precios" || it.PresupuestoUSD == nil || *it.PresupuestoUSD != 5 ||
		it.HechoEn == nil || it.CommitFinal == "" || !strings.Contains(it.Pedido, "que muestre precios") {
		t.Fatalf("CA-266: Parse lee todos los campos y el slug sale del nombre: %+v", it)
	}
	base := itItemValido("Precios", "2026-09-22T15:04:05Z", "")
	malos := map[string]string{
		"clave desconocida":         base + "columna: writer\n",
		"clave estado":              base + "estado: hecho\n",
		"yaml roto":                 "titulo: [sin cerrar\n",
		"tipo fuera de vocabulario": strings.Replace(base, "tipo: feature", "tipo: epica", 1),
		"prioridad invalida":        strings.Replace(base, "prioridad: media", "prioridad: urgente", 1),
		"presupuesto cero":          base + "presupuesto_usd: 0\n",
		"presupuesto negativo":      base + "presupuesto_usd: -1\n",
		"presupuesto texto":         base + "presupuesto_usd: \"cinco\"\n",
		"sin titulo":                strings.Replace(base, "titulo: Precios\n", "", 1),
		"titulo vacio":              strings.Replace(base, "titulo: Precios", "titulo: \"\"", 1),
		"sin tipo":                  strings.Replace(base, "tipo: feature\n", "", 1),
		"sin prioridad":             strings.Replace(base, "prioridad: media\n", "", 1),
		"sin creado_por":            strings.Replace(base, "creado_por: \"hoom test <test@hoom.dev>\"\n", "", 1),
		"sin creado_en":             strings.Replace(base, "creado_en: 2026-09-22T15:04:05Z\n", "", 1),
		"no es un mapa":             "- titulo: x\n",
	}
	for caso, raw := range malos {
		if _, err := Parse("precios", []byte(raw)); err == nil {
			t.Fatalf("CA-266 (%s): el item debe ser invalido:\n%s", caso, raw)
		}
	}
	_, err = Parse("precios", []byte(base+"columna: writer\n"))
	if err == nil || !strings.Contains(err.Error(), "clave desconocida") || !strings.Contains(err.Error(), "columna") ||
		!strings.Contains(err.Error(), "evidencia") {
		t.Fatalf("CA-266: la clave columna se rechaza diciendo que la columna sale de la evidencia: %v", err)
	}
}

// CA-266: List ordena por creado_en y despues por slug, omite los invalidos
// con un aviso que nombra el archivo, y nunca falla por ellos.
func TestCA266_ListOrdenYAvisos(t *testing.T) {
	root := itRepo(t)
	itEscribir(t, root, ".hoom/items/zeta.yaml", itItemValido("Zeta", "2026-09-22T09:00:00Z", ""))
	itEscribir(t, root, ".hoom/items/beta.yaml", itItemValido("Beta", "2026-09-22T10:00:00Z", ""))
	itEscribir(t, root, ".hoom/items/alfa.yaml", itItemValido("Alfa", "2026-09-22T10:00:00Z", ""))
	itEscribir(t, root, ".hoom/items/gamma.yaml", itItemValido("Gamma", "2026-09-23T08:00:00Z", ""))
	// invalidos: cada uno con su aviso
	itEscribir(t, root, ".hoom/items/roto.yaml", "titulo: [sin cerrar\n")
	itEscribir(t, root, ".hoom/items/con-columna.yaml", itItemValido("Col", "2026-09-22T10:00:00Z", "columna: hecho\n"))
	itEscribir(t, root, ".hoom/items/epica.yaml", strings.Replace(itItemValido("Ep", "2026-09-22T10:00:00Z", ""), "tipo: feature", "tipo: epica", 1))
	itEscribir(t, root, ".hoom/items/Foo Bar.yaml", itItemValido("Foo", "2026-09-22T10:00:00Z", ""))
	itEscribir(t, root, ".hoom/items/x.yml", itItemValido("X", "2026-09-22T10:00:00Z", ""))

	items, warns, err := List(root)
	if err != nil {
		t.Fatalf("CA-266: los items invalidos nunca son fatales: %v", err)
	}
	var slugs []string
	for _, it := range items {
		slugs = append(slugs, it.Slug)
	}
	if strings.Join(slugs, ",") != "zeta,alfa,beta,gamma" {
		t.Fatalf("CA-266: orden por creado_en y despues slug: zeta,alfa,beta,gamma; fue %v", slugs)
	}
	for _, archivo := range []string{"roto.yaml", "con-columna.yaml", "epica.yaml", "Foo Bar.yaml", "x.yml"} {
		encontrado := false
		for _, w := range warns {
			if strings.Contains(w, archivo) {
				encontrado = true
			}
		}
		if !encontrado {
			t.Fatalf("CA-266: falta el aviso que nombra %s: %q", archivo, warns)
		}
	}
	for _, w := range warns {
		if strings.Contains(w, "con-columna.yaml") && !strings.Contains(w, "clave desconocida") {
			t.Fatalf("CA-266: el aviso de la clave columna dice 'clave desconocida': %q", w)
		}
	}

	// un proyecto sin items: lista vacia, sin avisos, sin error
	vacio := itRepo(t)
	items, warns, err = List(vacio)
	if err != nil || len(items) != 0 || len(warns) != 0 {
		t.Fatalf("CA-266: sin .hoom/items/ no hay nada que listar: %v %v %v", items, warns, err)
	}
}

// CA-289: MarkDone escribe hecho_en (UTC, segundos) y commit_final, y
// conserva los demas campos.
func TestCA289_MarkDoneRegistraElCierre(t *testing.T) {
	root := itRepo(t)
	path := itEscribir(t, root, ".hoom/items/precios.yaml", itItemValido("Precios", "2026-09-22T15:04:05Z",
		"pedido: el pedido original\npresupuesto_usd: 5\n"))
	antes := itYAML(t, path)
	commit := "3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c"
	at := time.Date(2026, 9, 25, 7, 0, 0, 123456789, time.FixedZone("-03", -3*3600))
	wrote, err := MarkDone(root, "precios", commit, at)
	if err != nil || !wrote {
		t.Fatalf("CA-289: MarkDone sobre un item valido escribe: wrote=%v err=%v", wrote, err)
	}
	despues := itYAML(t, path)
	hecho := itFechaUTC(t, "CA-289", "hecho_en", despues["hecho_en"])
	if !hecho.Equal(time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("CA-289: hecho_en es el instante en UTC truncado a segundos (2026-09-25T10:00:00Z), fue %v", hecho)
	}
	if despues["commit_final"] != commit {
		t.Fatalf("CA-289: commit_final es el sha completo recibido: %v", despues["commit_final"])
	}
	for k, v := range antes {
		if despues[k] != v {
			t.Fatalf("CA-289: MarkDone conserva %s: %v -> %v", k, v, despues[k])
		}
	}
	if len(despues) != len(antes)+2 {
		t.Fatalf("CA-289: MarkDone solo agrega hecho_en y commit_final: %v", despues)
	}
	it, err := Load(root, "precios")
	if err != nil || it.HechoEn == nil || it.CommitFinal != commit {
		t.Fatalf("CA-289: el item marcado sigue siendo valido: %+v %v", it, err)
	}
}

// CA-290: sin item no hay nada que marcar; un item ya hecho no se reescribe;
// uno invalido es error y queda byte a byte igual.
func TestCA290_MarkDoneCasosLimite(t *testing.T) {
	root := itRepo(t)
	wrote, err := MarkDone(root, "no-existe", "abc", time.Now())
	if err != nil || wrote {
		t.Fatalf("CA-290: sin item MarkDone es (false, nil): %v %v", wrote, err)
	}
	if _, err := os.Stat(Path(root, "no-existe")); !os.IsNotExist(err) {
		t.Fatalf("CA-290: sin item MarkDone no crea uno (err=%v)", err)
	}

	hecho := itItemValido("Hecho", "2026-09-22T15:04:05Z", "hecho_en: 2026-09-24T10:00:00Z\ncommit_final: aaaa\n")
	path := itEscribir(t, root, ".hoom/items/hecho.yaml", hecho)
	wrote, err = MarkDone(root, "hecho", "bbbb", time.Now())
	if err != nil || wrote {
		t.Fatalf("CA-290: un item ya hecho es (false, nil): %v %v", wrote, err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != hecho {
		t.Fatalf("CA-290: un item ya hecho no se reescribe:\n%s", raw)
	}

	for nombre, cuerpo := range map[string]string{
		"invalido": itItemValido("Malo", "2026-09-22T15:04:05Z", "columna: hecho\n"),
		"ilegible": "titulo: [roto\n",
	} {
		p := itEscribir(t, root, ".hoom/items/"+nombre+".yaml", cuerpo)
		wrote, err = MarkDone(root, nombre, "cccc", time.Now())
		if err == nil || wrote {
			t.Fatalf("CA-290: un item %s es error: %v %v", nombre, wrote, err)
		}
		if raw, _ := os.ReadFile(p); string(raw) != cuerpo {
			t.Fatalf("CA-290: el item %s queda byte a byte igual:\n%s", nombre, raw)
		}
	}
}

// CA-263: las escrituras son atomicas: no quedan temporales en .hoom/items/.
func TestCA263_EscriturasSinRestos(t *testing.T) {
	root := itRepo(t)
	if _, err := Add(root, Draft{Titulo: "Uno"}); err != nil {
		t.Fatal(err)
	}
	if _, err := MarkDone(root, "uno", "abcd", time.Now()); err != nil {
		t.Fatal(err)
	}
	entradas, err := os.ReadDir(filepath.Join(root, ".hoom", "items"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entradas) != 1 || entradas[0].Name() != "uno.yaml" {
		var nombres []string
		for _, e := range entradas {
			nombres = append(nombres, e.Name())
		}
		t.Fatalf("CA-263: en .hoom/items/ queda solo uno.yaml: %v", nombres)
	}
}
