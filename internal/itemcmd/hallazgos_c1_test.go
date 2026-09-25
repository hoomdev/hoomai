// Tests de regresion de los hallazgos de la review cruzada de la cabina
// (spec C1 .hoom/specs/items-y-columna-derivada.md), contra el BINARIO y
// contra Parse, escritos desde el contrato:
//
//   - 20260924T195800_b7cb4a (medium): un item con presupuesto_usd .inf,
//     -.inf o .nan es invalido; `hoom item list --json` y `hoom board --json`
//     salen 0 con el aviso y los demas items (hoy fallan enteros con "json:
//     unsupported value: +Inf").
//   - 20260924T195800_a91dba: los temporales de la escritura atomica de
//     .hoom/items/ (.<slug>.yaml.tmp-NNN) no generan avisos; cualquier otro
//     oculto si (corregido por el hallazgo 20260925T044719_d23526, CA-266).
package itemcmd

import (
	"encoding/json"
	"strings"
	"testing"
)

// h1Tablero es lo que se lee de `hoom board --json`.
type h1Tablero struct {
	Columns []struct {
		ID    string `json:"id"`
		Cards []struct {
			Slug string `json:"slug"`
		} `json:"cards"`
	} `json:"columns"`
	Warnings []string `json:"warnings"`
}

func (b h1Tablero) h1Slugs() []string {
	var out []string
	for _, c := range b.Columns {
		for _, card := range c.Cards {
			out = append(out, card.Slug)
		}
	}
	return out
}

func h1Nombra(warns []string, archivo string) bool {
	for _, w := range warns {
		if strings.Contains(w, archivo) {
			return true
		}
	}
	return false
}

// Hallazgo 20260924T195800_b7cb4a (medium). CA-266 (item list: los
// invalidos se omiten con un aviso y el comando sale 0; --json nunca falla
// por ellos) y CA-288 (board --json trae los avisos de items invalidos, exit
// 0): un presupuesto .inf, -.inf o .nan no tumba la lista ni el tablero.
func TestCA266_H1Hallazgo195800b7_E2EListYBoardConPresupuestoNoFinito(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/bueno.yaml", icItemYAML("Bueno", "2026-09-22T15:04:05Z", "presupuesto_usd: 5\n"))
	malos := map[string]string{"infinito": ".inf", "menos-infinito": "-.inf", "no-numero": ".nan"}
	for slug, v := range malos {
		icEscribir(t, root, ".hoom/items/"+slug+".yaml", icItemYAML("Malo "+slug, "2026-09-22T16:00:00Z", "presupuesto_usd: "+v+"\n"))
	}

	r := ixCorrer(t, bin, root, "item", "list", "--json")
	var lista struct {
		Warnings []string         `json:"warnings"`
		Items    []map[string]any `json:"items"`
	}
	switch {
	case r.exit != 0:
		t.Errorf("CA-266: item list --json sale 0 aunque haya items invalidos, salio %d\nstdout: %s\nstderr: %s", r.exit, r.stdout, r.stderr)
	case json.Unmarshal([]byte(r.stdout), &lista) != nil:
		t.Errorf("CA-266: item list --json emite JSON:\n%s", r.stdout)
	default:
		if len(lista.Items) != 1 || lista.Items[0]["slug"] != "bueno" {
			t.Errorf("CA-266: item list --json trae solo el item valido: %v", lista.Items)
		}
		for slug, v := range malos {
			if !h1Nombra(lista.Warnings, slug+".yaml") {
				t.Errorf("CA-266: item list --json avisa por %s.yaml (presupuesto_usd: %s): %q", slug, v, lista.Warnings)
			}
		}
	}

	r = ixCorrer(t, bin, root, "item", "list")
	if r.exit != 0 || !strings.Contains(r.stdout, "bueno") || !strings.Contains(r.stderr, "infinito.yaml") {
		t.Errorf("CA-266: item list (texto) sale 0, lista el valido y avisa por stderr: %d\nstdout: %s\nstderr: %s", r.exit, r.stdout, r.stderr)
	}

	r = ixCorrer(t, bin, root, "board", "--json")
	if r.exit != 0 {
		t.Fatalf("CA-288: board --json sale 0 aunque haya items invalidos, salio %d\nstdout: %s\nstderr: %s", r.exit, r.stdout, r.stderr)
	}
	var tablero h1Tablero
	if err := json.Unmarshal([]byte(r.stdout), &tablero); err != nil {
		t.Fatalf("CA-288: board --json emite JSON: %v\n%s", err, r.stdout)
	}
	if len(tablero.Columns) != 8 || strings.Join(tablero.h1Slugs(), ",") != "bueno" {
		t.Errorf("CA-288: el tablero trae las 8 columnas y solo la tarjeta valida: %d columnas, tarjetas %v", len(tablero.Columns), tablero.h1Slugs())
	}
	for slug := range malos {
		if !h1Nombra(tablero.Warnings, slug+".yaml") {
			t.Errorf("CA-288: board --json avisa por %s.yaml: %q", slug, tablero.Warnings)
		}
	}
	if r := ixCorrer(t, bin, root, "board"); r.exit != 0 {
		t.Errorf("CA-288: board (texto) sale 0: %d\n%s", r.exit, r.stderr)
	}
}

// GUARDA (verde en la base). Hallazgo 20260924T195800_b7cb4a (medium).
// CA-264: `--presupuesto-usd` tiene que ser un numero > 0; inf, Infinity y
// NaN (que strconv acepta) son *cliargs.UsageError, exit 2: por la CLI no se
// puede crear el item que tumba la lista.
func TestCA264_H1Hallazgo195800b7_ParseRechazaPresupuestoNoFinito(t *testing.T) {
	for _, v := range []string{"inf", "+Inf", "Infinity", "NaN", "nan", "-inf", "1e999"} {
		args := []string{"add", "Precios", "--presupuesto-usd", v}
		_, err := Parse(args)
		if err == nil {
			t.Errorf("CA-264: --presupuesto-usd %s es *cliargs.UsageError, Parse lo acepto", v)
			continue
		}
		icUso(t, "CA-264", args, err)
	}
	// guarda: un numero finito positivo sigue valiendo
	if _, err := Parse([]string{"add", "Precios", "--presupuesto-usd", "2.5"}); err != nil {
		t.Fatalf("CA-264 (guarda): --presupuesto-usd 2.5 es valido: %v", err)
	}
}

// Hallazgo 20260924T195800_a91dba, CORREGIDO por el hallazgo
// 20260925T044719_d23526 (reliability). CA-266: "un nombre de archivo que no
// es slug se omite con un aviso que nombra el archivo, y el comando sale 0".
// La version anterior de este test exigia que TODO oculto de .hoom/items/
// fuera silencioso (stderr vacio, warnings []), lo que contradice CA-266.
// Contrato corregido: solo los temporales que hoom mismo escribe
// (".<slug>.yaml.tmp-NNN") se callan; los demas ocultos (.algo.yaml.tmp sin
// guion, un .lock, .DS_Store, .oculto.yaml) no son items y avisan
// nombrandose, en `item list` (stderr), `item list --json` y `board --json`
// (CA-288), todos con exit 0. El nombre del test se conserva por
// trazabilidad con a91dba.
func TestCA266_H1Hallazgoa91dba_E2EOcultosSinAviso(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	temporales := []string{".precios.yaml.tmp-1234567", ".facturas.yaml.tmp-7654321"}
	icEscribir(t, root, ".hoom/items/.precios.yaml.tmp-1234567", "titulo: [a medio escribir")
	icEscribir(t, root, ".hoom/items/.facturas.yaml.tmp-7654321", icItemYAML("Facturas", "2026-09-22T16:00:00Z", ""))
	conAviso := []string{".algo.yaml.tmp", ".precios.lock", ".facturas.lock", ".DS_Store", ".oculto.yaml"}
	icEscribir(t, root, ".hoom/items/.algo.yaml.tmp", "titulo: [a medio escribir")
	icEscribir(t, root, ".hoom/items/.precios.lock", "")
	icEscribir(t, root, ".hoom/items/.facturas.lock", "12345\n")
	icEscribir(t, root, ".hoom/items/.DS_Store", "\x00\x01basura")
	icEscribir(t, root, ".hoom/items/.oculto.yaml", icItemYAML("Oculto", "2026-09-22T17:00:00Z", ""))

	avisos := func(ca, donde string, warns []string, texto string) {
		t.Helper()
		for _, n := range conAviso {
			if !h1Nombra(warns, n) && !strings.Contains(texto, n) {
				t.Errorf("%s: hallazgo d23526: %s avisa por %s (no es <slug>.yaml ni un temporal de hoom): %q%s", ca, donde, n, warns, texto)
			}
		}
		for _, n := range temporales {
			if h1Nombra(warns, n) || strings.Contains(texto, n) {
				t.Errorf("%s: hallazgo d23526: %s calla el temporal de hoom %s: %q%s", ca, donde, n, warns, texto)
			}
		}
	}

	r := ixCorrer(t, bin, root, "item", "list")
	if r.exit != 0 || !strings.Contains(r.stdout, "precios") {
		t.Errorf("CA-266: item list sale 0 y lista precios: exit %d\nstdout: %s\nstderr: %s", r.exit, r.stdout, r.stderr)
	}
	avisos("CA-266", "item list (stderr)", nil, r.stderr)

	r = ixCorrer(t, bin, root, "item", "list", "--json")
	var lista struct {
		Warnings []string         `json:"warnings"`
		Items    []map[string]any `json:"items"`
	}
	if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &lista) != nil {
		t.Fatalf("CA-266: item list --json sale 0 con JSON: exit %d\nstdout: %s\nstderr: %s", r.exit, r.stdout, r.stderr)
	}
	if len(lista.Items) != 1 || lista.Items[0]["slug"] != "precios" {
		t.Errorf("CA-266: ni un temporal ni un oculto son items: %v", lista.Items)
	}
	avisos("CA-266", "item list --json", lista.Warnings, "")

	r = ixCorrer(t, bin, root, "board", "--json")
	var tablero h1Tablero
	if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &tablero) != nil {
		t.Fatalf("CA-288: board --json sale 0 con JSON: exit %d\nstdout: %s\nstderr: %s", r.exit, r.stdout, r.stderr)
	}
	if strings.Join(tablero.h1Slugs(), ",") != "precios" {
		t.Errorf("CA-288: el tablero trae solo la tarjeta precios: %v", tablero.h1Slugs())
	}
	avisos("CA-288", "board --json", tablero.Warnings, "")
	if r := ixCorrer(t, bin, root, "board"); r.exit != 0 {
		t.Errorf("CA-288: board (texto) sale 0: %d\n%s", r.exit, r.stderr)
	}
}
