// Tests adversariales del spec .hoom/specs/arquitecto-bajo-el-sobre.md
// (CA-240): hoom status distingue "no habia nada que certificar" de "se
// certifico y salio rojo". Un sobre sin entrega es un estado terminal propio,
// pintado SIN ENTREGA en amarillo con su nota, y nunca NO ENTREGABLE.
package statuscmd

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/live"
)

const abNota = "el rol arquitecto escribe y el run no dejo ningun archivo: no hay arbol nuevo que certificar"

func abSobreSinEntrega(t *testing.T, dir string, now time.Time) {
	t.Helper()
	envelope.Write(dir, envelope.Record{
		ID: "20260918T120000_ab12cd", Role: "arquitecto", Provider: "claude",
		Stage: "scope", Step: 3, Steps: 5, Status: envelope.StatusNoDelivery, ExitCode: 1,
		Note: abNota, RunID: "20260918T120000_run001",
		StartedAt: now.Add(-2 * time.Minute), EndedAt: now.Add(-time.Minute),
	})
}

// CA-240: status lo pinta SIN ENTREGA con la nota y no NO ENTREGABLE; su JSON
// lleva "sin-entrega"; y es terminal: no cuenta como en curso.
func TestCA240_StatusPintaSinEntrega(t *testing.T) {
	dir := initProject(t)
	verify(t, dir) // check verde: nada mas en la pantalla dice NO ENTREGABLE
	now := time.Now().UTC()
	abSobreSinEntrega(t, dir, now)

	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Envelopes) != 1 {
		t.Fatalf("CA-240: el ultimo sobre cerrado se ve: %+v", s.Envelopes)
	}
	e := s.Envelopes[0]
	if e.Status != envelope.StatusNoDelivery || !e.Done() {
		t.Fatalf("CA-240: sin-entrega es un estado terminal: %+v", e)
	}

	var buf bytes.Buffer
	Render(&buf, s, false)
	out := buf.String()
	if !strings.Contains(out, "SIN ENTREGA") {
		t.Fatalf("CA-240: status debe pintar SIN ENTREGA:\n%s", out)
	}
	if !strings.Contains(out, abNota) {
		t.Fatalf("CA-240: status debe mostrar la nota del sobre:\n%s", out)
	}
	if strings.Contains(out, "NO ENTREGABLE") {
		t.Fatalf("CA-240: un sobre sin entrega no es NO ENTREGABLE:\n%s", out)
	}
	if strings.Contains(out, "1 en curso") {
		t.Fatalf("CA-240: un sobre sin entrega termino, no esta en curso:\n%s", out)
	}
	if !strings.Contains(out, "rol arquitecto") {
		t.Fatalf("CA-240: el sobre sigue identificado por su rol:\n%s", out)
	}

	// JSON: el mismo dato
	raw, err := JSONBytes(s)
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	sobres, ok := back["envelopes"].([]any)
	if !ok || len(sobres) != 1 {
		t.Fatalf("CA-240: el JSON trae el sobre: %s", raw)
	}
	sobre := sobres[0].(map[string]any)
	if sobre["status"] != "sin-entrega" {
		t.Fatalf("CA-240: el JSON lleva \"status\": \"sin-entrega\": %v", sobre)
	}
	if sobre["note"] != abNota || sobre["stage"] != "scope" || sobre["exit_code"] != float64(1) {
		t.Fatalf("CA-240: el JSON conserva nota, paso y exit: %v", sobre)
	}
}

// CA-240: el color es amarillo, no el rojo del NO ENTREGABLE.
func TestCA240_SinEntregaEnAmarillo(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	abSobreSinEntrega(t, dir, time.Now().UTC())
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	Render(&buf, s, true)
	out := buf.String()
	i := strings.Index(out, "SIN ENTREGA")
	if i < 0 {
		t.Fatalf("CA-240: con color tambien dice SIN ENTREGA: %q", out)
	}
	previo := out[:i]
	j := strings.LastIndex(previo, "\x1b[")
	if j < 0 {
		t.Fatalf("CA-240: SIN ENTREGA debe ir coloreado: %q", out)
	}
	codigo := previo[j:]
	if !strings.Contains(codigo, "33") || strings.Contains(codigo, "31") {
		t.Fatalf("CA-240: SIN ENTREGA va en amarillo (33), no en rojo: %q", codigo)
	}
}

// CA-240: terminal de verdad: aunque pase el umbral de silencio, un sobre
// sin entrega no se etiqueta posible huerfano.
func TestCA240_SinEntregaNoEsHuerfano(t *testing.T) {
	dir := initProject(t)
	verify(t, dir)
	now := time.Now().UTC()
	abSobreSinEntrega(t, dir, now)
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	s.Now = now.Add(live.OrphanAfter + time.Hour)
	s.Envelopes = envelopes(dir, s.Now)
	for _, e := range s.Envelopes {
		if e.ID == "20260918T120000_ab12cd" && e.PossibleOrphan {
			t.Fatalf("CA-240: un sobre cerrado no puede quedar huerfano: %+v", e)
		}
	}
	var buf bytes.Buffer
	Render(&buf, s, false)
	if strings.Contains(buf.String(), "posible huerfano") {
		t.Fatalf("CA-240: un sobre sin entrega termino:\n%s", buf.String())
	}
}
