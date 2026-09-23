// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-334, CA-335) sobre `hoom agent`: el Studio lanza el sobre en segundo
// plano, asi que necesita nombrarlo de antemano (EnvelopeID) y saber cuando
// el primer registro ya esta en disco (Started). El registro lleva el PID de
// su dueno desde el primero. Providers falsos en el PATH: ningun CLI real.
package agentcmd

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// cbStarted cuenta las llamadas a Started y fotografia, en el momento de
// cada una, el registro del sobre que ya tiene que estar en disco.
type cbStarted struct {
	mu     sync.Mutex
	root   string
	id     string
	calls  int
	enDisc bool
	rec    envelope.Record
}

func (s *cbStarted) fn() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	for _, r := range envelope.List(s.root) {
		if r.ID == s.id {
			s.enDisc = true
			s.rec = r
		}
	}
}

func (s *cbStarted) snapshot() (int, bool, envelope.Record) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls, s.enDisc, s.rec
}

// cbSinRegistros exige que el proyecto no tenga ningun registro de sobre.
func cbSinRegistros(t *testing.T, ca, caso, root, id string) {
	t.Helper()
	if recs := envelope.List(root); len(recs) != 0 {
		t.Fatalf("%s: %s: un error antes del primer registro no deja registro: %+v", ca, caso, recs)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", envelope.DirName, id+".json")); err == nil {
		t.Fatalf("%s: %s: no debe existir el registro %s.json", ca, caso, id)
	}
}

// CA-335: con EnvelopeID, el registro del sobre (y el Result) usan ese id y
// no otro: el Studio responde 202 nombrando un sobre que existe.
func TestCA335_AgentUsaElEnvelopeIDPreasignado(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "printf 'package app // implementado\\n' > app.go\nexit 0\n")
	const id = "20260923T120000_cb0a01"

	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa", EnvelopeID: id}, io.Discard)
	if err != nil {
		t.Fatalf("CA-335: el sobre no debio fallar en su armado: %v", err)
	}
	if res.EnvelopeID != id {
		t.Fatalf("CA-335: el Result nombra el sobre preasignado %q, no %q", id, res.EnvelopeID)
	}
	recs := envelope.List(root)
	if len(recs) != 1 || recs[0].ID != id {
		t.Fatalf("CA-335: un sobre, un registro, con el id preasignado %q: %+v", id, recs)
	}
	if _, err := os.Stat(filepath.Join(root, ".hoom", envelope.DirName, id+".json")); err != nil {
		t.Fatalf("CA-335: el registro vive en .hoom/envelopes/%s.json: %v", id, err)
	}
	if !recs[0].Done() || recs[0].Status != envelope.StatusDeliverable {
		t.Fatalf("CA-335: el registro preasignado es el que cierra el sobre: %+v", recs[0])
	}
}

// CA-335: Started se llama UNA vez, y cuando se llama el primer registro ya
// esta en disco (con el PID del proceso dueno). Vale tanto para un sobre que
// recorre sus cinco pasos como para uno que corta en el paso 1.
func TestCA335_AgentStartedUnaVezConElRegistroEnDisco(t *testing.T) {
	// (a) camino completo: cinco pasos, muchas escrituras del registro, una
	// sola llamada
	root := repo(t)
	fakeProvider(t, "claude", "printf 'package app // implementado\\n' > app.go\nexit 0\n")
	s := &cbStarted{root: root, id: "20260923T120000_cb0a02"}
	res, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa", EnvelopeID: s.id, Started: s.fn}, io.Discard)
	if err != nil {
		t.Fatalf("CA-335: %v", err)
	}
	calls, enDisco, rec := s.snapshot()
	if calls != 1 {
		t.Fatalf("CA-335: Started se llama una sola vez, no %d (res %+v)", calls, res)
	}
	if !enDisco {
		t.Fatalf("CA-335: cuando Started se llama, el registro %s ya esta en disco", s.id)
	}
	if rec.PID != os.Getpid() {
		t.Fatalf("CA-335: el primer registro ya trae el PID del dueno (%d): %+v", os.Getpid(), rec)
	}
	if rec.Role != "writer" || rec.Provider != "claude" {
		t.Fatalf("CA-335: el registro que Started encuentra identifica el sobre: %+v", rec)
	}

	// (b) corte en el paso 1 (spec sin aprobar): hay registro, asi que
	// Started se llama igual, una vez
	root2 := repo(t)
	spec := specDemo(t, root2)
	s2 := &cbStarted{root: root2, id: "20260923T120000_cb0a03"}
	res2, err := Run(root2, "main", Options{Role: "writer", Spec: spec, Prompt: "implementa", EnvelopeID: s2.id, Started: s2.fn}, io.Discard)
	if err != nil {
		t.Fatalf("CA-335: %v", err)
	}
	if res2.Stage != "spec" || res2.ExitCode != 1 {
		t.Fatalf("CA-335: fixture: el spec sin aprobar corta en el paso 1: %+v", res2)
	}
	calls, enDisco, _ = s2.snapshot()
	if calls != 1 || !enDisco {
		t.Fatalf("CA-335: un sobre que corta en el paso 1 ya escribio su registro: Started una vez (%d) con el registro en disco (%v)", calls, enDisco)
	}
}

// CA-335: un error ANTES del primer registro (rol desconocido, pedido vacio
// o de relleno, provider sin system_prompt, un autor de specs con --spec,
// una tarea que no existe) llega sin que Started se llame y sin registro.
func TestCA335_AgentErrorAntesDelRegistroNoLlamaStarted(t *testing.T) {
	fakeProvider(t, "claude", "exit 0\n")
	fakeProvider(t, "gemini", "exit 0\n")
	casos := []struct {
		nombre string
		opt    Options
	}{
		{"rol desconocido", Options{Role: "no-existe", Prompt: "implementa"}},
		{"pedido vacio", Options{Role: "writer", Prompt: ""}},
		{"pedido en blanco", Options{Role: "writer", Prompt: "   \n\t"}},
		{"pedido de relleno", Options{Role: "writer", Prompt: "..."}},
		{"pedido placeholder", Options{Role: "writer", Prompt: "<pedido>"}},
		{"provider sin system_prompt", Options{Role: "writer", Provider: "gemini", Prompt: "implementa"}},
		{"autor de specs con --spec", Options{Role: "arquitecto", Spec: ".hoom/specs/demo.md", Prompt: "escribi el spec"}},
		{"tarea inexistente", Options{Role: "writer", Task: "no-existe", Prompt: "implementa"}},
	}
	for i, c := range casos {
		root := repo(t)
		s := &cbStarted{root: root, id: "20260923T120000_cb0b" + string(rune('0'+i)) + "0"}
		c.opt.EnvelopeID, c.opt.Started = s.id, s.fn
		_, err := Run(root, "main", c.opt, io.Discard)
		if err == nil {
			t.Fatalf("CA-335: %s: el sobre se niega con un error", c.nombre)
		}
		if calls, _, _ := s.snapshot(); calls != 0 {
			t.Fatalf("CA-335: %s: Started no se llama si no hay registro (llamadas: %d)", c.nombre, calls)
		}
		cbSinRegistros(t, "CA-335", c.nombre, root, s.id)
		if n := runsCount(t, root); n != 0 {
			t.Fatalf("CA-335: %s: tampoco hay run: %d archivos en .hoom/runs", c.nombre, n)
		}
	}
}

// CA-334: `hoom agent` escribe en su registro el PID de su proceso, desde el
// primer registro hasta el que lo cierra.
func TestCA334_ElSobreDelAgenteLlevaSuPID(t *testing.T) {
	root := repo(t)
	fakeProvider(t, "claude", "printf 'package app // implementado\\n' > app.go\nexit 0\n")
	s := &cbStarted{root: root, id: "20260923T120000_cb0c01"}
	if _, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa", EnvelopeID: s.id, Started: s.fn}, io.Discard); err != nil {
		t.Fatalf("CA-334: %v", err)
	}
	if _, _, primero := s.snapshot(); primero.PID != os.Getpid() {
		t.Fatalf("CA-334: el primer registro trae pid = %d: %+v", os.Getpid(), primero)
	}
	recs := envelope.List(root)
	if len(recs) != 1 {
		t.Fatalf("CA-334: un sobre, un registro: %+v", recs)
	}
	if recs[0].PID != os.Getpid() {
		t.Fatalf("CA-334: el registro que cierra el sobre sigue trayendo pid = %d: %+v", os.Getpid(), recs[0])
	}

	// sin EnvelopeID ni Started (la CLI de siempre) el PID viaja igual
	root2 := repo(t)
	if _, err := Run(root2, "main", Options{Role: "writer", Prompt: "implementa"}, io.Discard); err != nil {
		t.Fatalf("CA-334: %v", err)
	}
	recs = envelope.List(root2)
	if len(recs) != 1 || recs[0].PID != os.Getpid() {
		t.Fatalf("CA-334: hoom agent escribe pid = %d tambien sin Studio: %+v", os.Getpid(), recs)
	}
}
