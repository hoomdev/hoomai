// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-378) sobre `hoom item add --auto hasta-humano`: el flag escribe la
// clave auto del item y exige --presupuesto-usd (el piloto automatico no
// corre sin tope). Parse es puro: el rechazo es un error de uso antes de leer
// el proyecto, y no deja ningun archivo.
package itemcmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/item"
)

const auSinTope = "--auto necesita --presupuesto-usd: el piloto automatico no corre sin tope"

// auExec parsea y ejecuta 'hoom item <args>' en root.
func auExec(t *testing.T, root string, args ...string) (string, error) {
	t.Helper()
	req, err := Parse(args)
	if err != nil {
		t.Fatalf("CA-378: Parse(%q) es un pedido valido: %v", args, err)
	}
	var out, errb bytes.Buffer
	err = Execute(root, "main", "high", req, &out, &errb)
	return out.String(), err
}

// CA-378: Parse acepta --auto hasta-humano con --presupuesto-usd, con los
// flags antes o despues del titulo; sin --auto no hay auto.
func TestCA378_ParseAddConAuto(t *testing.T) {
	for _, args := range [][]string{
		{"add", "t", "--auto", "hasta-humano", "--presupuesto-usd", "5"},
		{"add", "--presupuesto-usd", "5", "--auto", "hasta-humano", "t"},
		{"add", "t", "--auto=hasta-humano", "--presupuesto-usd=5", "--json"},
	} {
		req, err := Parse(args)
		if err != nil {
			t.Fatalf("CA-378: %q es valido: %v", args, err)
		}
		d := req.Draft
		if req.Sub != SubAdd || d.Titulo != "t" || d.Auto != item.AutoHastaHumano || d.PresupuestoUSD == nil || *d.PresupuestoUSD != 5 {
			t.Fatalf("CA-378: %q trae auto hasta-humano y presupuesto 5: %+v %+v", args, req, d)
		}
	}
	req, err := Parse([]string{"add", "t", "--presupuesto-usd", "5"})
	if err != nil || req.Draft.Auto != "" {
		t.Fatalf("CA-378: sin --auto el borrador no lleva auto: %+v %v", req.Draft, err)
	}
}

// CA-378: --auto sin --presupuesto-usd es un error de uso con el mensaje del
// contrato; otro valor, o --auto sin valor, tambien es un error de uso.
func TestCA378_AutoSinPresupuestoEsErrorDeUso(t *testing.T) {
	for _, args := range [][]string{
		{"add", "t", "--auto", "hasta-humano"},
		{"add", "--auto", "hasta-humano", "t", "--json"},
		{"add", "t", "--auto", "hasta-humano", "--tipo", "bug", "--pedido", "x"},
	} {
		_, err := Parse(args)
		ue := icUso(t, "CA-378", args, err)
		if !strings.Contains(ue.Error(), auSinTope) {
			t.Fatalf("CA-378: %q se rechaza diciendo %q:\n%s", args, auSinTope, ue.Error())
		}
	}
	for _, args := range [][]string{
		{"add", "t", "--auto", "siempre", "--presupuesto-usd", "5"},
		{"add", "t", "--auto", "", "--presupuesto-usd", "5"},
		{"add", "t", "--presupuesto-usd", "5", "--auto"},
	} {
		_, err := Parse(args)
		icUso(t, "CA-378", args, err)
	}
}

// CA-378: Execute escribe auto: hasta-humano en el item, con su presupuesto;
// --json lo emite. Sin --auto el archivo no lleva la clave.
func TestCA378_ExecuteEscribeAuto(t *testing.T) {
	root := icRepo(t)
	out, err := auExec(t, root, "add", "t", "--auto", "hasta-humano", "--presupuesto-usd", "5", "--json")
	if err != nil {
		t.Fatalf("CA-378: item add --auto: %v", err)
	}
	var js map[string]any
	if err := json.Unmarshal([]byte(out), &js); err != nil || js["auto"] != "hasta-humano" || js["presupuesto_usd"] != float64(5) {
		t.Fatalf("CA-378: --json emite el item con auto y presupuesto: %v\n%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", "items", "t.yaml"))
	if err != nil {
		t.Fatalf("CA-378: se escribio .hoom/items/t.yaml: %v", err)
	}
	if !strings.Contains(string(raw), "\nauto: hasta-humano\n") {
		t.Fatalf("CA-378: el archivo lleva auto: hasta-humano:\n%s", raw)
	}
	it, err := item.Load(root, "t")
	if err != nil || it.Auto != item.AutoHastaHumano || it.PresupuestoUSD == nil || *it.PresupuestoUSD != 5 {
		t.Fatalf("CA-378: el item se lee con auto y presupuesto: %+v %v", it, err)
	}

	if _, err := auExec(t, root, "add", "u", "--presupuesto-usd", "5"); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(filepath.Join(root, ".hoom", "items", "u.yaml"))
	if strings.Contains(string(raw), "auto") {
		t.Fatalf("CA-378: sin --auto el item no lleva la clave:\n%s", raw)
	}
}

// CA-378: con el binario, --auto con presupuesto sale con 0 y escribe el
// item; sin presupuesto es exit 2 con el mensaje del contrato y no escribe
// nada.
func TestCA378_E2EItemAddAuto(t *testing.T) {
	bin := ixBinario(t)
	root := icRepo(t)
	r := ixCorrer(t, bin, root, "item", "add", "Cinta", "--auto", "hasta-humano", "--presupuesto-usd", "5")
	if r.exit != 0 {
		t.Fatalf("CA-378: 'hoom item add --auto hasta-humano --presupuesto-usd 5' sale con 0, fue %d\n%s%s", r.exit, r.stdout, r.stderr)
	}
	it, err := item.Load(root, "cinta")
	if err != nil || it.Auto != item.AutoHastaHumano {
		t.Fatalf("CA-378: el binario escribe auto en el item: %+v %v", it, err)
	}
	antes := ixFotoItems(t, root)
	r = ixCorrer(t, bin, root, "item", "add", "Otra", "--auto", "hasta-humano")
	if r.exit != 2 || !strings.Contains(r.stdout+r.stderr, auSinTope) {
		t.Fatalf("CA-378: --auto sin --presupuesto-usd es exit 2 con %q, fue %d\n%s%s", auSinTope, r.exit, r.stdout, r.stderr)
	}
	if !ixMismos(antes, ixFotoItems(t, root)) {
		t.Fatalf("CA-378: el rechazo no escribe ningun item: %v", ixFotoItems(t, root))
	}
}
