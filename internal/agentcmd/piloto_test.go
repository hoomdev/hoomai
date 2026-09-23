// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-382) sobre `hoom agent`: un sobre que lanzo la cinta del Studio
// (Options.Pilot) lo dice en su registro con "piloto": true, desde el primer
// registro hasta el que lo cierra; sin Pilot el registro no trae la clave.
// Providers falsos en el PATH: ningun CLI real.
package agentcmd

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
// sobre tal como esta en disco (el JSON crudo).
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

// plClaves lee un registro de sobre como mapa: sirve para saber si una clave
// esta o no esta.
func plClaves(t *testing.T, ca string, crudo []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(crudo, &m); err != nil {
		t.Fatalf("%s: el registro del sobre es JSON: %v\n%s", ca, err, crudo)
	}
	return m
}

func plRegistro(t *testing.T, root, id string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, id+".json"))
	if err != nil {
		t.Fatalf("CA-382: el registro vive en .hoom/envelopes/%s.json: %v", id, err)
	}
	return raw
}

// CA-382: con Pilot el registro del sobre trae "piloto": true, en el primer
// registro (el que Started encuentra) y en el que cierra el sobre.
func TestCA382_AgentConPilotElRegistroDicePiloto(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "printf 'package app // implementado\\n' > app.go\nexit 0\n")
	f := &plFoto{root: root, id: "20260923T130000_pl0a01"}

	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa",
		EnvelopeID: f.id, Started: f.fn, Pilot: true}, io.Discard)
	if err != nil {
		t.Fatalf("CA-382: %v", err)
	}
	if res.EnvelopeID != f.id {
		t.Fatalf("CA-382: fixture: el sobre usa el id preasignado: %+v", res)
	}

	primero := f.primero()
	if len(primero) == 0 {
		t.Fatal("CA-382: fixture: Started encontro el primer registro en disco")
	}
	if v, ok := plClaves(t, "CA-382", primero)["piloto"]; !ok || v != true {
		t.Fatalf("CA-382: el primer registro de un sobre de la cinta ya dice \"piloto\": true:\n%s", primero)
	}

	final := plRegistro(t, root, f.id)
	m := plClaves(t, "CA-382", final)
	if v, ok := m["piloto"]; !ok || v != true {
		t.Fatalf("CA-382: el registro que cierra el sobre de la cinta trae \"piloto\": true:\n%s", final)
	}
	if m["status"] != envelope.StatusDeliverable {
		t.Fatalf("CA-382: fixture: el sobre cierra entregable: %s", final)
	}
	recs := envelope.List(root)
	if len(recs) != 1 || !recs[0].Pilot {
		t.Fatalf("CA-382: el registro leido dice Pilot: %+v", recs)
	}
}

// CA-382: un sobre de la cinta que corta (spec sin aprobar, paso 1) tambien
// deja "piloto": true: la historia lo muestra en cualquier estado.
func TestCA382_AgentConPilotQueCortaTambienLoDice(t *testing.T) {
	root := repo(t)
	spec := specDemo(t, root)
	fakeProvider(t, "claude", "exit 0\n")
	const id = "20260923T130000_pl0a02"

	res, err := Run(root, "main", Options{Role: "writer", Spec: spec, Prompt: "implementa",
		EnvelopeID: id, Pilot: true}, io.Discard)
	if err != nil {
		t.Fatalf("CA-382: %v", err)
	}
	if res.Stage != "spec" || res.ExitCode != 1 {
		t.Fatalf("CA-382: fixture: el spec sin aprobar corta en el paso 1: %+v", res)
	}
	raw := plRegistro(t, root, id)
	if v, ok := plClaves(t, "CA-382", raw)["piloto"]; !ok || v != true {
		t.Fatalf("CA-382: el registro no-entregable de un sobre de la cinta trae \"piloto\": true:\n%s", raw)
	}
}

// CA-382: sin Pilot (la terminal, o una persona desde el Studio) el registro
// no trae la clave "piloto", ni en el primer registro ni en el ultimo.
func TestCA382_AgentSinPilotNoHayClave(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "printf 'package app // implementado\\n' > app.go\nexit 0\n")
	f := &plFoto{root: root, id: "20260923T130000_pl0a03"}

	if _, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa",
		EnvelopeID: f.id, Started: f.fn}, io.Discard); err != nil {
		t.Fatalf("CA-382: %v", err)
	}
	for nombre, crudo := range map[string][]byte{"primero": f.primero(), "final": plRegistro(t, root, f.id)} {
		if len(crudo) == 0 {
			t.Fatalf("CA-382: fixture: el registro %s existe", nombre)
		}
		if _, ok := plClaves(t, "CA-382", crudo)["piloto"]; ok {
			t.Fatalf("CA-382: sin Pilot el registro %s no trae la clave piloto:\n%s", nombre, crudo)
		}
	}
	if recs := envelope.List(root); len(recs) != 1 || recs[0].Pilot {
		t.Fatalf("CA-382: sin Pilot el registro leido no es de la cinta: %+v", recs)
	}
}
