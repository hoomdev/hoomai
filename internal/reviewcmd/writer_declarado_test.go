// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-351): el writer declarado. `hoom review` lee las `sesiones` del item de
// la tarea en el arbol raiz (.hoom/items/<task>.yaml): son writers
// DECLARADOS, que pueden volver una review no-cruzada pero nunca ascenderla a
// cruzada. El observado sigue saliendo del sidecar del run. Providers falsos
// en el PATH: ningun CLI de IA real.
package reviewcmd

import (
	"encoding/json"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func cbRegistroDeReview(t *testing.T, dir, id string) map[string]any {
	t.Helper()
	if id == "" {
		t.Fatal("CA-351: una review revisada trae record_id")
	}
	return rcLeer(t, filepath.Join(dir, ".hoom", RecordsDir, id+".json"))
}

func cbLista(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := []string{}
	for _, x := range raw {
		s, _ := x.(string)
		out = append(out, s)
	}
	return out
}

// CA-351: con el reviewer igual a un writer DECLARADO la review es
// no-cruzada y se niega sin --same-provider, aunque el writer observado sea
// otro. Con --same-provider corre, marcada no-cruzada.
func TestCA351_ReviewerDeclaradoEsNoCruzada(t *testing.T) {
	root := cbRepo(t)
	wt := cbTarea(t, root, "precios")
	cbMeta(t, root, wt, "20260923T110000_cbw101", "claude", "writer") // observado: claude
	cbItem(t, root, "precios", "codex")                               // declarado: codex
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")

	var out strings.Builder
	res, err := Run(root, "main", Options{Task: "precios", Lens: "risk", Provider: "codex"}, &out)
	if err != nil {
		t.Fatalf("CA-351: %v\n%s", err, out.String())
	}
	if res.Writer != "claude" {
		t.Fatalf("CA-351: el declarado nunca reemplaza al observado (claude): %+v", res)
	}
	if !reflect.DeepEqual(res.WritersDeclared, []string{"codex"}) {
		t.Fatalf("CA-351: writers_declared sale de las sesiones del item: %v", res.WritersDeclared)
	}
	if res.Cross != CrossNo || res.ExitCode != 1 || res.Status != "no-entregable" || len(res.Passes) != 0 {
		t.Fatalf("CA-351: revisar con un writer declarado es no-cruzada y se niega sin --same-provider: %+v\n%s", res, out.String())
	}
	if !strings.Contains(out.String(), "--same-provider") {
		t.Fatalf("CA-351: la negativa trae el escape declarado, como hoy: %s", out.String())
	}

	res, err = Run(root, "main", Options{Task: "precios", Lens: "risk", Provider: "codex", SameProvider: true}, io.Discard)
	if err != nil {
		t.Fatalf("CA-351: %v", err)
	}
	if res.Cross != CrossNo || res.Status != "revisado" || len(res.Passes) != 1 {
		t.Fatalf("CA-351: con --same-provider corre y queda marcada no-cruzada: %+v", res)
	}
	m := cbRegistroDeReview(t, wt, res.RecordID)
	if m["cross"] != CrossNo || !reflect.DeepEqual(cbLista(m["writers_declared"]), []string{"codex"}) {
		t.Fatalf("CA-351: el registro de review guarda cross y writers_declared: %v", m)
	}
}

// CA-351: sin writer observado, con declarados distintos del reviewer, la
// review es cruzada-declarada (nunca cruzada: nadie lo vio escribir); con el
// reviewer entre los declarados, no-cruzada.
func TestCA351_SinObservadoEsCruzadaDeclarada(t *testing.T) {
	root := cbRepo(t)
	wt := cbTarea(t, root, "precios")
	cbItem(t, root, "precios", "codex")
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")

	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Lens: "risk", Provider: "claude"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-351: %v", err)
	}
	if res.Writer != "" {
		t.Fatalf("CA-351: sin sidecar no hay writer observado: %+v", res)
	}
	if res.Cross != CrossDeclared {
		t.Fatalf("CA-351: sin observado y con un declarado distinto del reviewer es %q, nunca %q: %+v", CrossDeclared, CrossYes, res)
	}
	if res.Status != "revisado" || res.ExitCode != 0 || len(res.Passes) != 1 {
		t.Fatalf("CA-351: una review cruzada-declarada corre: %+v", res)
	}
	m := cbRegistroDeReview(t, wt, res.RecordID)
	if m["cross"] != CrossDeclared {
		t.Fatalf("CA-351: el registro de review guarda cross = %q: %v", CrossDeclared, m["cross"])
	}
	if !reflect.DeepEqual(cbLista(m["writers_declared"]), []string{"codex"}) {
		t.Fatalf("CA-351: el registro de review guarda writers_declared [codex]: %v", m["writers_declared"])
	}
	raw, _ := json.Marshal(res)
	if !strings.Contains(string(raw), `"writers_declared":["codex"]`) || !strings.Contains(string(raw), `"cross":"`+CrossDeclared+`"`) {
		t.Fatalf("CA-351: el JSON del Result trae writers_declared y cross: %s", raw)
	}

	// el reviewer es el declarado: no-cruzada, y se niega
	res, err = Run(root, "main", Options{Task: "precios", Lens: "risk", Provider: "codex"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-351: %v", err)
	}
	if res.Cross != CrossNo || res.ExitCode != 1 || len(res.Passes) != 0 {
		t.Fatalf("CA-351: el reviewer declarado como writer es no-cruzada aunque no haya observado: %+v", res)
	}
}

// CA-351: writers_declared son los providers DISTINTOS de las sesiones,
// ordenados; sin sesiones (o sin item) es [] y nunca null, en el Result y en
// el registro de review. Con observado y sin declarados, la cruzada es la de
// siempre.
func TestCA351_WritersDeclaredDistintosOrdenadosYVacios(t *testing.T) {
	root := cbRepo(t)
	wt := cbTarea(t, root, "precios")
	cbItem(t, root, "precios", "opencode", "codex", "opencode")
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	res, err := Run(root, "main", Options{Task: "precios", Lens: "risk", Provider: "claude"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-351: %v", err)
	}
	if !reflect.DeepEqual(res.WritersDeclared, []string{"codex", "opencode"}) {
		t.Fatalf("CA-351: writers_declared son los providers distintos de las sesiones, ordenados: %v", res.WritersDeclared)
	}
	if res.Cross != CrossDeclared || res.Status != "revisado" {
		t.Fatalf("CA-351: claude no es ninguno de los declarados: cruzada-declarada: %+v", res)
	}
	m := cbRegistroDeReview(t, wt, res.RecordID)
	if !reflect.DeepEqual(cbLista(m["writers_declared"]), []string{"codex", "opencode"}) {
		t.Fatalf("CA-351: el registro de review guarda los mismos writers_declared: %v", m["writers_declared"])
	}

	// item sin sesiones: [] (no null), y con observado la review es cruzada
	root2 := cbRepo(t)
	wt2 := cbTarea(t, root2, "precios")
	cbItem(t, root2, "precios")
	cbMeta(t, root2, wt2, "20260923T110000_cbw102", "claude", "writer")
	res, err = Run(root2, "main", Options{Task: "precios", Lens: "risk", Provider: "codex"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-351: %v", err)
	}
	if res.WritersDeclared == nil || len(res.WritersDeclared) != 0 {
		t.Fatalf("CA-351: sin sesiones writers_declared es [] y no null: %#v", res.WritersDeclared)
	}
	if res.Cross != CrossYes || res.Status != "revisado" {
		t.Fatalf("CA-351: con observado y sin declarados la cruzada es la de siempre: %+v", res)
	}
	raw, _ := json.Marshal(res)
	if !strings.Contains(string(raw), `"writers_declared":[]`) {
		t.Fatalf("CA-351: el JSON del Result trae writers_declared: []: %s", raw)
	}
	m = cbRegistroDeReview(t, wt2, res.RecordID)
	if l, ok := m["writers_declared"].([]any); !ok || len(l) != 0 {
		t.Fatalf("CA-351: el registro de review trae writers_declared: [] y no null: %v", m["writers_declared"])
	}

	// sin item: tambien []
	root3 := cbRepo(t)
	wt3 := cbTarea(t, root3, "precios")
	cbMeta(t, root3, wt3, "20260923T110000_cbw103", "claude", "writer")
	res, err = Run(root3, "main", Options{Task: "precios", Lens: "risk", Provider: "codex"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-351: %v", err)
	}
	if res.WritersDeclared == nil || len(res.WritersDeclared) != 0 || res.Cross != CrossYes {
		t.Fatalf("CA-351: sin item no hay declarados ([]), y la review es cruzada: %+v", res)
	}
}

// CA-351: el item que cuenta es el del arbol RAIZ: un item con sesiones que
// solo existe en el arbol revisado no declara a nadie.
func TestCA351_ElItemQueCuentaEsElDelArbolRaiz(t *testing.T) {
	root := cbRepo(t)
	wt := cbTarea(t, root, "precios")
	cbItem(t, wt, "precios", "codex") // en el worktree, no en la raiz
	cbItem(t, root, "precios")        // en la raiz, sin sesiones
	cbMeta(t, root, wt, "20260923T110000_cbw104", "claude", "writer")
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "codex", "exit 0\n")
	res, err := Run(root, "main", Options{Task: "precios", Lens: "risk", Provider: "codex"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-351: %v", err)
	}
	if len(res.WritersDeclared) != 0 || res.Cross != CrossYes {
		t.Fatalf("CA-351: las sesiones se leen del item del arbol raiz (sin sesiones): %+v", res)
	}
}

// CA-351: sin --provider la review elige el primer provider instalado que no
// sea ni el writer observado ni uno declarado.
func TestCA351_EleccionAutomaticaEvitaObservadoYDeclarados(t *testing.T) {
	bin := cbPathAislado(t)
	cbFake(t, bin, "claude", "exit 0\n")
	cbFake(t, bin, "codex", "exit 0\n")

	// sin observado y claude declarado: el primero del registro (claude) no
	// sirve, se elige codex
	root := cbRepo(t)
	cbTarea(t, root, "precios")
	cbItem(t, root, "precios", "claude")
	res, err := Run(root, "main", Options{Task: "precios", Lens: "risk"}, io.Discard)
	if err != nil {
		t.Fatalf("CA-351: %v", err)
	}
	if res.Provider != "codex" || res.Cross != CrossDeclared || res.Status != "revisado" {
		t.Fatalf("CA-351: sin --provider se elige uno que no sea un declarado (codex), cruzada-declarada: %+v", res)
	}

	// codex observado y claude declarado: no queda ninguno que sirva, y la
	// review no corre ninguna pasada
	root2 := cbRepo(t)
	wt2 := cbTarea(t, root2, "precios")
	cbItem(t, root2, "precios", "claude")
	cbMeta(t, root2, wt2, "20260923T110000_cbw105", "codex", "writer")
	res, _ = Run(root2, "main", Options{Task: "precios", Lens: "risk"}, io.Discard)
	if len(res.Passes) != 0 || res.Status == "revisado" {
		t.Fatalf("CA-351: si todo provider instalado es observado o declarado, no se revisa: %+v", res)
	}
	if got := rcRegistros(t, wt2); len(got) != 0 {
		t.Fatalf("CA-351: y no queda registro de review: %v", got)
	}
}
