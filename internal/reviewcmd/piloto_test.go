// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-382) sobre `hoom review`: una review que lanzo la cinta del Studio
// (Options.Pilot) lo dice en su registro de sobre con "piloto": true, desde
// el primer registro hasta el que la cierra; sin Pilot el registro no trae la
// clave. Providers falsos en el PATH: ningun CLI de IA real.
package reviewcmd

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// plFoto guarda, en el momento en que Started se llama, el registro del
// sobre de la review tal como esta en disco (el JSON crudo).
type plFoto struct {
	mu    sync.Mutex
	root  string
	id    string
	crudo []byte
}

func (f *plFoto) fn() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.crudo, _ = os.ReadFile(filepath.Join(f.root, ".hoom", envelope.DirName, f.id+".json"))
}

func (f *plFoto) primero() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.crudo
}

func plClaves(t *testing.T, crudo []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(crudo, &m); err != nil {
		t.Fatalf("CA-382: el registro del sobre es JSON: %v\n%s", err, crudo)
	}
	return m
}

// plReview corre una review revisada de la tarea precios (escrita por
// claude, revisada por un codex falso) con el Pilot pedido, y devuelve el
// primer registro (el que Started encontro) y el ultimo.
func plReview(t *testing.T, id string, pilot bool) (primero, final []byte) {
	t.Helper()
	root := cbRepo(t)
	wt := cbTarea(t, root, "precios")
	cbMeta(t, root, wt, "20260923T110000_plw001", "claude", "writer")
	fakeProvider(t, "codex", "exit 0\n")

	f := &plFoto{root: root, id: id}
	res, err := Run(root, "main", Options{Task: "precios", Spec: ".hoom/specs/precios.md", Provider: "codex",
		EnvelopeID: id, Started: f.fn, Pilot: pilot}, io.Discard)
	if err != nil {
		t.Fatalf("CA-382: %v", err)
	}
	if res.Status != "revisado" {
		t.Fatalf("CA-382: fixture: la review termina revisada: %+v", res)
	}
	primero = f.primero()
	if len(primero) == 0 {
		t.Fatal("CA-382: fixture: Started encontro el primer registro de la review en disco")
	}
	final, err = os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, id+".json"))
	if err != nil {
		t.Fatalf("CA-382: el registro de la review vive en .hoom/envelopes/%s.json: %v", id, err)
	}
	if recs := envelope.List(root); len(recs) != 1 || recs[0].Pilot != pilot {
		t.Fatalf("CA-382: el registro leido de la review dice Pilot = %v: %+v", pilot, recs)
	}
	return primero, final
}

// CA-382: con Pilot el registro del sobre de la review trae "piloto": true.
func TestCA382_ReviewConPilotElRegistroDicePiloto(t *testing.T) {
	primero, final := plReview(t, "20260923T130000_plrev1", true)
	for nombre, crudo := range map[string][]byte{"primero": primero, "final": final} {
		if v, ok := plClaves(t, crudo)["piloto"]; !ok || v != true {
			t.Fatalf("CA-382: el registro %s de una review de la cinta trae \"piloto\": true:\n%s", nombre, crudo)
		}
	}
	if m := plClaves(t, final); m["status"] != envelope.StatusDeliverable || m["role"] != "reviewer" {
		t.Fatalf("CA-382: fixture: la review cierra entregable como reviewer: %s", final)
	}
}

// CA-382: sin Pilot el registro de la review no trae la clave "piloto".
func TestCA382_ReviewSinPilotNoHayClave(t *testing.T) {
	primero, final := plReview(t, "20260923T130000_plrev2", false)
	for nombre, crudo := range map[string][]byte{"primero": primero, "final": final} {
		if _, ok := plClaves(t, crudo)["piloto"]; ok {
			t.Fatalf("CA-382: sin Pilot el registro %s de la review no trae la clave piloto:\n%s", nombre, crudo)
		}
	}
}
