// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// `hoom item` (CA-786, CA-788): `hoom item set <slug> --riesgo r` escribe
// riesgo_por y riesgo_en, tambien con normal, y con el riesgo que ya tenia
// imprime `ya es` sin escribir; un rol no cambia el riesgo (exit 1 por el
// binario, sin tocar el archivo); un item con las dos claves se lee; y `hoom
// item show` muestra ` · riesgo <efectivo>` en la linea de tipo y prioridad,
// y su JSON trae riesgo y riesgo_motivo. Escritos desde el spec, las
// aclaraciones de la ronda 0 y las firmas publicas, sin leer la
// implementacion.
package itemcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const (
	retyQuien   = "hoom test <test@hoom.dev>"
	retyNoRol   = "el riesgo de una tarjeta lo cambia una persona, no un rol"
	retyMotAuth = "el cambio toca internal/auth/x.go: riesgo alto"
)

var retyRFC3339UTC = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?Z$`)

// retyValor es el valor de la linea `<clave>: ` del archivo, sin comillas.
func retyValor(raw []byte, clave string) (string, bool) {
	for _, l := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(l, clave+":") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(l, clave+":")), `"'`), true
		}
	}
	return "", false
}

func retyLeer(t *testing.T, root, slug string) []byte {
	t.Helper()
	raw, err := os.ReadFile(item.Path(root, slug))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// retyQuienYCuando exige riesgo_por = quien y riesgo_en en UTC, RFC 3339.
func retyQuienYCuando(t *testing.T, ca string, raw []byte, quien string) {
	t.Helper()
	if v, ok := retyValor(raw, "riesgo_por"); !ok || v != quien {
		t.Fatalf("%s: riesgo_por es %q:\n%s", ca, quien, raw)
	}
	v, ok := retyValor(raw, "riesgo_en")
	if !ok || !retyRFC3339UTC.MatchString(v) {
		t.Fatalf("%s: riesgo_en en UTC, RFC 3339:\n%s", ca, raw)
	}
	en, err := time.Parse(time.RFC3339Nano, v)
	if err != nil || time.Since(en) > time.Minute || time.Until(en) > time.Minute {
		t.Fatalf("%s: riesgo_en es ahora: %q %v", ca, v, err)
	}
}

// CA-788: Execute set escribe riesgo_por y riesgo_en con cada cambio,
// tambien a normal (sin `riesgo`); con el que ya tenia, `ya es` y nada
// escrito; el JSON de set sigue siendo {slug, antes, riesgo}.
func TestCA788_ExecuteSetEscribeQuienYCuando(t *testing.T) {
	const ca = "CA-788"
	t.Setenv("HOOM_ROLE", "")
	root := icRepo(t)
	if _, err := item.Add(root, item.Draft{Titulo: "Precios", Slug: "precios"}); err != nil {
		t.Fatal(err)
	}
	crCommitearItems(t, root)
	out, _, err := icExec(t, root, "set", "precios", "--riesgo", "alto")
	if err != nil || !strings.Contains(out, "riesgo de precios: normal -> alto - guardalo con: hoom item save precios") {
		t.Fatalf("%s: %v\n%s", ca, err, out)
	}
	raw := retyLeer(t, root, "precios")
	retyQuienYCuando(t, ca, raw, retyQuien)
	if r, _ := retyValor(raw, "riesgo"); r != "alto" {
		t.Fatalf("%s: riesgo: alto:\n%s", ca, raw)
	}

	icGit(t, root, "config", "user.name", "Ana Perez")
	icGit(t, root, "config", "user.email", "ana@hoom.dev")
	out, _, err = icExec(t, root, "set", "precios", "--riesgo", "normal")
	if err != nil || !strings.Contains(out, "riesgo de precios: alto -> normal - guardalo con: hoom item save precios") {
		t.Fatalf("%s: %v\n%s", ca, err, out)
	}
	raw = retyLeer(t, root, "precios")
	if _, ok := retyValor(raw, "riesgo"); ok {
		t.Fatalf("%s: con --riesgo normal el archivo queda sin `riesgo`:\n%s", ca, raw)
	}
	retyQuienYCuando(t, ca, raw, "Ana Perez <ana@hoom.dev>")

	// el que ya tenia: `ya es`, sin escribir riesgo_por ni riesgo_en
	st0, _ := os.Stat(item.Path(root, "precios"))
	icGit(t, root, "config", "user.name", "Otra Persona")
	time.Sleep(10 * time.Millisecond)
	out, _, err = icExec(t, root, "set", "precios", "--riesgo", "normal")
	if err != nil || !strings.Contains(out, "riesgo de precios: ya es normal") {
		t.Fatalf("%s: %v\n%s", ca, err, out)
	}
	st1, _ := os.Stat(item.Path(root, "precios"))
	if raw1 := retyLeer(t, root, "precios"); !bytes.Equal(raw, raw1) || !st0.ModTime().Equal(st1.ModTime()) {
		t.Fatalf("%s: con el riesgo que ya tenia no escribe:\n%s\n---\n%s", ca, raw, raw1)
	}

	// --json: las claves de siempre, sin riesgo_por ni riesgo_en
	out, _, err = icExec(t, root, "set", "precios", "--riesgo", "bajo", "--json")
	var js map[string]any
	if err != nil || json.Unmarshal([]byte(out), &js) != nil ||
		!reflect.DeepEqual(js, map[string]any{"slug": "precios", "antes": "normal", "riesgo": "bajo"}) {
		t.Fatalf("%s: el JSON de set es {slug, antes, riesgo}: %v %v\n%s", ca, js, err, out)
	}
	retyQuienYCuando(t, ca, retyLeer(t, root, "precios"), "Otra Persona <ana@hoom.dev>")
}

// CA-788 (hostil): un rol no cambia el riesgo, ni a normal: Set y Execute se
// niegan sin tocar el archivo, aunque ya traiga riesgo_por y riesgo_en; por
// el binario sale con exit 1. Una persona si, por el binario.
func TestCA788_UnRolNoCambiaElRiesgo(t *testing.T) {
	const ca = "CA-788"
	t.Setenv("HOOM_ROLE", "")
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z",
		"riesgo: alto\nriesgo_por: \"Ana <ana@hoom.dev>\"\nriesgo_en: 2026-11-20T10:00:05Z\n"))
	foto := ixFotoItems(t, root)
	for _, r := range []string{"bajo", "normal", "alto"} {
		for _, rol := range []string{"writer", "test-writer", "reviewer", "refutador"} {
			if _, err := Set(root, "main", "high", "precios", r, rol); err == nil || !strings.Contains(err.Error(), retyNoRol) {
				t.Fatalf("%s: Set(%s, rol %s) se niega con %q: %v", ca, r, rol, retyNoRol, err)
			}
			if !ixMismos(foto, ixFotoItems(t, root)) {
				t.Fatalf("%s: Set(%s, rol %s) no escribe", ca, r, rol)
			}
		}
		res := crCorrer(t, bin, root, "writer", "item", "set", "precios", "--riesgo", r)
		if res.exit != 1 || !strings.Contains(res.stderr, retyNoRol) {
			t.Fatalf("%s: hoom item set --riesgo %s con HOOM_ROLE sale 1 con %q: %d\n%s%s", ca, r, retyNoRol, res.exit, res.stdout, res.stderr)
		}
		if !ixMismos(foto, ixFotoItems(t, root)) {
			t.Fatalf("%s: con HOOM_ROLE el archivo no cambia", ca)
		}
	}
	// el item con las dos claves se lee
	for _, args := range [][]string{{"item", "show", "precios"}, {"item", "list"}} {
		if res := crCorrer(t, bin, root, "", args...); res.exit != 0 || !strings.Contains(res.stdout, "precios") ||
			strings.Contains(res.stdout+res.stderr, "invalido") {
			t.Fatalf("%s: hoom %v lee el item con riesgo_por y riesgo_en: %d\n%s%s", ca, args, res.exit, res.stdout, res.stderr)
		}
	}
	// una persona, por el binario
	res := crCorrer(t, bin, root, "", "item", "set", "precios", "--riesgo", "bajo")
	if res.exit != 0 || !strings.Contains(res.stdout, "riesgo de precios: alto -> bajo - guardalo con: hoom item save precios") {
		t.Fatalf("%s: una persona cambia el riesgo: %d\n%s%s", ca, res.exit, res.stdout, res.stderr)
	}
	retyQuienYCuando(t, ca, retyLeer(t, root, "precios"), retyQuien)
}

// retyTarjetaJSON es card.evidence de `hoom item show --json`.
func retyTarjetaJSON(t *testing.T, ca, out string) map[string]any {
	t.Helper()
	var js struct {
		Card map[string]any `json:"card"`
	}
	if err := json.Unmarshal([]byte(out), &js); err != nil {
		t.Fatalf("%s: item show --json es JSON: %v\n%s", ca, err, out)
	}
	e, ok := js.Card["evidence"].(map[string]any)
	if !ok {
		t.Fatalf("%s: la tarjeta trae evidence: %s", ca, out)
	}
	return e
}

// CA-786 y aclaracion 17: `hoom item show` muestra ` · riesgo <efectivo>` en
// la linea de tipo y prioridad cuando no es normal, y su JSON trae riesgo y
// riesgo_motivo (sin tamano): con el declarado sin veredicto, y con el
// efectivo de un veredicto que subio una tarjeta que declara normal.
func TestCA786_ItemShowConElRiesgoDeLaTarjeta(t *testing.T) {
	const ca = "CA-786"
	t.Setenv("HOOM_ROLE", "")
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/alto.yaml", icItemYAML("Alto", "2026-09-22T15:04:05Z", "riesgo: alto\n"))
	icEscribir(t, root, ".hoom/items/llano.yaml", icItemYAML("Llano", "2026-09-22T15:04:05Z", ""))
	icEscribir(t, root, ".hoom/items/subido.yaml", icItemYAML("Subido", "2026-09-22T15:04:05Z", ""))
	icEscribir(t, root, ".hoom/specs/subido.md", "# Spec subido\n")
	v := &verdict.Verdict{Schema: verdict.SchemaID, CreatedAt: time.Now().UTC().Add(-time.Minute), Project: "demo",
		Spec:   ".hoom/specs/subido.md",
		Git:    gitx.Info{IsRepo: true, ChangeFingerprint: "huella", Insertions: 50, Deletions: 10, ChangedFiles: []string{"internal/auth/x.go"}},
		Gates:  []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass, Scope: "full"}},
		Riesgo: &riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: retyMotAuth},
		Tamano: &riesgo.Tamano{Producto: 60, Pruebas: 0, Hoom: 0}}
	v.Finalize()
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatal(err)
	}
	for _, k := range []struct {
		slug, quiero, motivo string
	}{
		{"alto", "alto", "declarado alto"},
		{"llano", "", "declarado normal"},
		{"subido", "alto", retyMotAuth},
	} {
		res := crCorrer(t, bin, root, "", "item", "show", k.slug)
		if res.exit != 0 {
			t.Fatalf("%s: hoom item show %s: %d\n%s%s", ca, k.slug, res.exit, res.stdout, res.stderr)
		}
		if strings.Contains(res.stdout, "· riesgo normal") {
			t.Fatalf("%s: nunca `· riesgo normal`:\n%s", ca, res.stdout)
		}
		if k.quiero == "" {
			if strings.Contains(res.stdout, "· riesgo") {
				t.Fatalf("%s: %s es normal: sin el borde:\n%s", ca, k.slug, res.stdout)
			}
		} else {
			linea := ""
			for _, l := range strings.Split(res.stdout, "\n") {
				if strings.Contains(l, "feature") && strings.Contains(l, "media") {
					linea = l
					break
				}
			}
			if !strings.Contains(linea, " · riesgo "+k.quiero) {
				t.Fatalf("%s: %s: ` · riesgo %s` en la linea de tipo y prioridad: %q\n%s", ca, k.slug, k.quiero, linea, res.stdout)
			}
		}
		res = crCorrer(t, bin, root, "", "item", "show", k.slug, "--json")
		e := retyTarjetaJSON(t, ca, res.stdout)
		quiero := k.quiero
		if quiero == "" {
			quiero = "normal"
		}
		if e["riesgo"] != quiero || e["riesgo_motivo"] != k.motivo {
			t.Fatalf("%s: %s: evidence.riesgo %q y riesgo_motivo %q: %v", ca, k.slug, quiero, k.motivo, e)
		}
		if _, ok := e["tamano"]; ok {
			t.Fatalf("%s: el JSON de la tarjeta no trae tamano: %v", ca, e)
		}
	}
	// el veredicto no cambia el item: sigue sin `riesgo`
	if raw, _ := os.ReadFile(filepath.Join(root, ".hoom", "items", "subido.yaml")); strings.Contains(string(raw), "riesgo") {
		t.Fatalf("%s: el efectivo no se escribe en el item:\n%s", ca, raw)
	}
}
