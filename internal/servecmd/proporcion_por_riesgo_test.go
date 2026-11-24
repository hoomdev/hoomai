// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre la
// aprobacion del Studio (CA-795): POST /api/specs/{name}/approve, sin cuerpo
// y con {"card": ...}, responde con error y no escribe registro con un spec
// que pasa su tope; despues de subir el tope de la tarjeta, aprueba. Escritos
// desde el spec, las aclaraciones de la ronda 0 y las firmas publicas, sin
// leer la implementacion. El codigo HTTP del error es «el de hoy»: no se fija.
package servecmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
)

// pprSpecN es tbSpecTexto (con Riesgos) con k criterios y n palabras exactas.
func pprSpecN(t *testing.T, n, k int) string {
	t.Helper()
	var crit strings.Builder
	for i := 1; i <= k; i++ {
		fmt.Fprintf(&crit, "- CA-%d: criterio %d.\n", i, i)
	}
	s := tbSpecTexto(crit.String(), true)
	base := len(strings.Fields(s))
	if n < base {
		t.Fatalf("fixture: el spec ya tiene %d palabras", base)
	}
	s = strings.Replace(s, "## Contratos\nx", "## Contratos\nx"+strings.Repeat(" palabra", n-base), 1)
	if got := len(strings.Fields(s)); got != n {
		t.Fatalf("fixture: %d palabras, no %d", got, n)
	}
	return s
}

func pprI(n int) *int       { return &n }
func pprS(s string) *string { return &s }

// CA-795: sin cuerpo, un spec del proyecto que pasa su tope: error JSON con
// la linea de spec_lint y ningun registro; con el tope de la tarjeta subido,
// 200 y el registro.
func TestCA795_StudioSinCuerpoSeNiegaYDespuesAprueba(t *testing.T) {
	const ca = "CA-795"
	root := tbProyecto(t)
	tbItem(t, root, "grande", "")
	tbEscribir(t, root, ".hoom/specs/grande.md", pprSpecN(t, 801, 2))
	s := newServer(t, root)
	code, ct, body := lnAprobar(t, s, "grande", s.Token(), "")
	if code == http.StatusOK {
		t.Fatalf("%s: un spec que pasa su tope no se aprueba: %s", ca, body)
	}
	var e map[string]string
	want := `el spec tiene 801 palabras y el tope con riesgo normal es 800: reduce el alcance, o que una persona suba el tope de la tarjeta: hoom item set grande --tope-palabras 801 --motivo "<por que>"`
	if !strings.Contains(ct, "application/json") || json.Unmarshal(body, &e) != nil || !strings.Contains(e["error"], want) {
		t.Fatalf("%s: el error JSON trae la linea de spec_lint:\n%s\nfue %d %q %s", ca, want, code, ct, body)
	}
	if ap := lnAprobaciones(t, root); len(ap) != 0 {
		t.Fatalf("%s: la negativa no escribe registro: %v", ca, ap)
	}
	if _, _, _, err := item.Cambiar(root, "grande", item.Cambios{TopePalabras: pprI(801), TopeMotivo: pprS("el intake es largo")}); err != nil {
		t.Fatal(err)
	}
	code, _, body = lnAprobar(t, s, "grande", s.Token(), "")
	if code != http.StatusOK || len(lnAprobaciones(t, root)) != 1 {
		t.Fatalf("%s: con el tope subido aprueba: %d %s %v", ca, code, body, lnAprobaciones(t, root))
	}
}

// CA-795: con {"card": ...}, la tarjeta cuyo spec pasa su tope no se aprueba
// (responde con error) ni en su espacio de trabajo ni en el proyecto; con el
// tope de la tarjeta subido vuelve a Tu aprobacion y aprueba.
func TestCA795_StudioConCardSeNiegaYDespuesAprueba(t *testing.T) {
	const ca = "CA-795"
	root := tbProyecto(t)
	tbItem(t, root, "carta", "")
	wt := lnTarea(t, ca, root, "carta")
	tbEscribir(t, wt, ".hoom/specs/carta.md", pprSpecN(t, 300, 9))
	s := newServer(t, root)
	code, ct, body := lnAprobar(t, s, "carta", s.Token(), `{"card": "carta"}`)
	if code == http.StatusOK || !strings.Contains(ct, "application/json") {
		t.Fatalf("%s: con card, un spec que pasa su tope responde con error: %d %s", ca, code, body)
	}
	if ap := lnAprobaciones(t, wt); len(ap) != 0 {
		t.Fatalf("%s: ningun registro en el espacio de trabajo: %v", ca, ap)
	}
	if ap := lnAprobaciones(t, root); len(ap) != 0 {
		t.Fatalf("%s: ningun registro en el proyecto: %v", ca, ap)
	}
	if _, _, _, err := item.Cambiar(root, "carta", item.Cambios{TopeCriterios: pprI(9), TopeMotivo: pprS("nueve")}); err != nil {
		t.Fatal(err)
	}
	lnColumna(t, ca, root, "carta", boardcmd.ColTuAprobacion)
	code, _, body = lnAprobar(t, s, "carta", s.Token(), `{"card": "carta"}`)
	if code != http.StatusOK || len(lnAprobaciones(t, wt)) != 1 {
		t.Fatalf("%s: con el tope subido aprueba en su arbol: %d %s", ca, code, body)
	}
}
