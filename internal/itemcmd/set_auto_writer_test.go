// Tests propios del writer (autonomia-entre-firmas): hoom item set con
// --auto y --presupuesto-usd, por Parse y Execute.
package itemcmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/item"
)

func swSet(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	req, err := Parse(append([]string{"set"}, args...))
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	err = Execute(root, "main", "high", req, &out, io.Discard)
	return out.String(), err
}

func TestWriterSetAutoYPresupuesto(t *testing.T) {
	t.Setenv("HOOM_ROLE", "")
	root := icRepo(t)
	icEscribir(t, root, ".hoom/items/precios.yaml", icItemYAML("Precios", "2026-09-22T15:04:05Z", ""))

	if _, err := swSet(t, root, "precios", "--auto", "hasta-humano"); err == nil ||
		err.Error() != "--auto necesita --presupuesto-usd: el piloto automatico no corre sin tope" {
		t.Fatalf("auto sin presupuesto: %v", err)
	}
	if _, err := swSet(t, root, "precios", "--auto", "hasta-humano", "--presupuesto-usd", "3.5", "--riesgo", "alto"); err != nil {
		t.Fatal(err)
	}
	it, err := item.Load(root, "precios")
	if err != nil || it.Auto != item.AutoHastaHumano || it.PresupuestoUSD == nil || *it.PresupuestoUSD != 3.5 || it.Riesgo != "alto" {
		t.Fatalf("%+v %v", it, err)
	}
	if _, err := swSet(t, root, "precios", "--auto", "no"); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(item.Path(root, "precios"))
	if strings.Contains(string(raw), "auto:") || !strings.Contains(string(raw), "presupuesto_usd: 3.5") {
		t.Fatalf("--auto no quita la clave y deja el resto:\n%s", raw)
	}
	out, err := swSet(t, root, "precios", "--presupuesto-usd", "2", "--json")
	if err != nil || !strings.Contains(out, `"presupuesto_usd": 2`) || strings.Contains(out, `"auto"`) {
		t.Fatalf("%s %v", out, err)
	}

	t.Setenv("HOOM_ROLE", "writer")
	if _, err := swSet(t, root, "precios", "--riesgo", "bajo", "--auto", "no"); err == nil ||
		err.Error() != "el piloto automatico y el presupuesto de una tarjeta los cambia una persona, no un rol" {
		t.Fatalf("con rol: %v", err)
	}
}

func TestWriterSetUso(t *testing.T) {
	for args, want := range map[string]string{
		"precios":                       "falta --riesgo, --auto o --presupuesto-usd: es lo que set cambia",
		"precios --presupuesto-usd 0":   "--presupuesto-usd tiene que ser un numero mayor que 0",
		"precios --presupuesto-usd -1":  "--presupuesto-usd tiene que ser un numero mayor que 0",
		"precios --presupuesto-usd abc": "--presupuesto-usd tiene que ser un numero mayor que 0",
		"precios --auto si":             "--auto tiene que ser hasta-humano o no",
	} {
		_, err := Parse(append([]string{"set"}, strings.Fields(args)...))
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: quiero uso con %q, fue %v", args, want, err)
		}
	}
}
