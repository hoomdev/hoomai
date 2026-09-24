// Tests de regresion de los hallazgos de la review cruzada de la cabina
// (spec C1 .hoom/specs/items-y-columna-derivada.md), contra el BINARIO y
// contra Parse, escritos desde el contrato:
//
//   - 20260924T195800_b7cb4a (medium): un item con presupuesto_usd .inf,
//     -.inf o .nan es invalido; `hoom item list --json` y `hoom board --json`
//     salen 0 con el aviso y los demas items (hoy fallan enteros con "json:
//     unsupported value: +Inf").
//   - 20260924T195800_a91dba: los archivos ocultos de .hoom/items/ (los
//     temporales de la escritura atomica) no generan avisos.
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

// Hallazgo 20260924T195800_a91dba. CA-266: los ocultos de .hoom/items/ (los
// temporales de una escritura atomica, un lock) no son items ni avisos:
// `item list` sin nada en stderr, `item list --json` y `board --json` con
// warnings vacios.
func TestCA266_H1Hallazgoa91dba_E2EOcultosSinAviso(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))
	icEscribir(t, root, ".hoom/items/.algo.yaml.tmp", "titulo: [a medio escribir")
	icEscribir(t, root, ".hoom/items/.precios.lock", "")

	r := ixCorrer(t, bin, root, "item", "list")
	if r.exit != 0 || strings.TrimSpace(r.stderr) != "" {
		t.Errorf("CA-266: item list sin avisos por los ocultos: exit %d\nstderr: %s", r.exit, r.stderr)
	}
	r = ixCorrer(t, bin, root, "item", "list", "--json")
	var lista struct {
		Warnings []string         `json:"warnings"`
		Items    []map[string]any `json:"items"`
	}
	if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &lista) != nil || len(lista.Items) != 1 || lista.Warnings == nil || len(lista.Warnings) != 0 {
		t.Errorf("CA-266: item list --json con warnings [] y el item: exit %d\n%s", r.exit, r.stdout)
	}
	r = ixCorrer(t, bin, root, "board", "--json")
	var tablero h1Tablero
	if r.exit != 0 || json.Unmarshal([]byte(r.stdout), &tablero) != nil || len(tablero.Warnings) != 0 {
		t.Errorf("CA-288: board --json sin avisos por los ocultos: exit %d warnings %q", r.exit, tablero.Warnings)
	}
}
