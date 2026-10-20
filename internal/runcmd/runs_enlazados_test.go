// Helpers compartidos por los tests de 058fb1 (CA-361, CA-362) sobre
// .hoom/specs/historia-doctor-y-cinta.md: la narracion de un run es
// .hoom/runs/<id>.jsonl de la raiz, y lo que llega por un enlace simbolico
// desde otro arbol no se lee. Los usan hallazgos_c4_vuelta2_test.go (el
// .jsonl es un enlace) y hallazgos_c4_vuelta3_test.go (.hoom/runs o .hoom
// son un enlace). Un cambio al contrato de ReadEvents, ReadEventsFrom o
// ReadDelegations se escribe aca una sola vez (hallazgo bc0087).
package runcmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// narracionDeOtroArbol es la narracion de un run de otro arbol: un start y
// tres delegaciones de verdad (dos agent y un agent_end). Leida entera son 4
// eventos y 3 delegaciones.
func narracionDeOtroArbol(t0 time.Time) []Event {
	return []Event{
		{TS: t0, Kind: "start", Detail: "init"},
		{TS: t0.Add(time.Minute), Kind: "agent", Agent: "espia-ajeno", ToolID: "toolu_AJ", Detail: "Agent: secreto del otro proyecto"},
		{TS: t0.Add(2 * time.Minute), Kind: "agent_end", Agent: "espia-ajeno", ToolID: "toolu_AJ", Detail: "espia-ajeno termino"},
		{TS: t0.Add(3 * time.Minute), Kind: "agent", Agent: "otro-espia", ToolID: "toolu_AJ2", Detail: "Agent: mas secretos"},
	}
}

// exigirQueSeLeaEntero exige que el id se lea entero por los tres caminos:
// ReadEvents (todos los eventos), ReadEventsFrom desde 0 (todos, y el offset
// al final del archivo) y ReadDelegations (solo agent y agent_end). Corta el
// test al primer camino que falla.
func exigirQueSeLeaEntero(t *testing.T, ca, root, id string, eventos, delegaciones int) {
	t.Helper()
	evs, err := ReadEvents(root, id)
	if err != nil || len(evs) != eventos {
		t.Fatalf("%s: ReadEvents(%q, %q) lee los %d eventos: err=%v, %d eventos %+v", ca, root, id, eventos, err, len(evs), evs)
	}
	fi, err := os.Stat(filepath.Join(root, ".hoom", "runs", id+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	evs, next, err := ReadEventsFrom(root, id, 0)
	if err != nil || len(evs) != eventos || next != fi.Size() {
		t.Fatalf("%s: ReadEventsFrom(%q, %q, 0) lee los %d eventos y deja el offset en %d: err=%v, %d eventos, offset %d",
			ca, root, id, eventos, fi.Size(), err, len(evs), next)
	}
	ds, err := ReadDelegations(root, id)
	if err != nil || len(ds) != delegaciones {
		t.Fatalf("%s: ReadDelegations(%q, %q) lee las %d delegaciones: err=%v, %d %+v", ca, root, id, delegaciones, err, len(ds), ds)
	}
	for _, d := range ds {
		if d.Kind != "agent" && d.Kind != "agent_end" {
			t.Fatalf("%s: ReadDelegations(%q) trae solo agent y agent_end: %+v", ca, id, d)
		}
	}
}

// exigirQueNoSeLea exige que el id no se lea por ningun camino: ReadEvents y
// ReadDelegations devuelven un error y nada, y ReadEventsFrom (desde 0 y
// desde mas adelante) un error, nada, y el offset donde estaba. como dice
// por que (el enlace que hay en el camino). Reporta cada camino que falla y
// sigue.
func exigirQueNoSeLea(t *testing.T, ca, root, id, como string) {
	t.Helper()
	evs, err := ReadEvents(root, id)
	if err == nil || len(evs) != 0 {
		t.Errorf("%s: ReadEvents(%q), con %s, es un error y no lee nada; fue err=%v y %d eventos: %+v",
			ca, id, como, err, len(evs), evs)
	}
	for _, desde := range []int64{0, 10} {
		evs, next, err := ReadEventsFrom(root, id, desde)
		if err == nil || len(evs) != 0 || next != desde {
			t.Errorf("%s: ReadEventsFrom(%q, %d), con %s, es un error, no lee nada y no avanza el offset; fue err=%v, %d eventos, offset %d",
				ca, id, desde, como, err, len(evs), next)
		}
	}
	ds, err := ReadDelegations(root, id)
	if err == nil || len(ds) != 0 {
		t.Errorf("%s: ReadDelegations(%q), con %s, es un error y no trae delegaciones; fue err=%v y %d: %+v",
			ca, id, como, err, len(ds), ds)
	}
}
