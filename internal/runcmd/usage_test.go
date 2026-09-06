// Tests del writer para el spec .hoom/specs/datos-en-vez-de-prosa.md
// (CA-199, CA-200): el run acumula lo que gasto, y el acumulado sobrevive al
// proceso en el sidecar. La regla de sumar se apoya en un hecho medido: en un
// --resume real, Claude reporto 1 turno y 0.1120905 USD —lo de ESA
// invocacion—, no el acumulado de la sesion.
package runcmd

import (
	"testing"
)

// resultado imita una invocacion de Claude que abre sesion y cierra diciendo
// lo que gasto.
const resultado = `printf '{"type":"system","subtype":"init","session_id":"s-1"}\n` +
	`{"type":"result","subtype":"success","result":"ok","num_turns":1,"duration_ms":1562,` +
	`"total_cost_usd":0.111583,"usage":{"input_tokens":2,"cache_creation_input_tokens":10641,` +
	`"cache_read_input_tokens":10126,"output_tokens":4}}\n'` + "\nexit 0\n"

// CA-199: dos invocaciones (Start + Input) suman costo, turnos y tokens.
func TestCA199_ElRunAcumulaElGasto(t *testing.T) {
	installFake(t, resultado)
	root := t.TempDir()
	m := NewManager(root)

	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	primera := waitRun(t, m, info.ID)
	if primera.Usage == nil || primera.Usage.CostUSD == nil {
		t.Fatalf("CA-199: la primera invocacion ya deja gasto: %+v", primera.Usage)
	}
	if *primera.Usage.CostUSD != 0.111583 || primera.Usage.Turns != 1 {
		t.Fatalf("CA-199: gasto de una invocacion: %+v", primera.Usage)
	}

	if _, err := m.Input(info.ID, "y ahora esto"); err != nil {
		t.Fatal(err)
	}
	segunda := waitRun(t, m, info.ID)
	if segunda.Usage == nil || segunda.Usage.CostUSD == nil {
		t.Fatalf("CA-199: el acumulado no puede perderse: %+v", segunda.Usage)
	}
	if *segunda.Usage.CostUSD != 0.111583*2 {
		t.Fatalf("CA-199: el costo de dos invocaciones se SUMA: %v", *segunda.Usage.CostUSD)
	}
	if segunda.Usage.Turns != 2 || segunda.Usage.InputTokens != 10643*2 || segunda.Usage.OutputTokens != 8 {
		t.Fatalf("CA-199: turnos y tokens tambien se suman: %+v", segunda.Usage)
	}
}

// CA-199: un provider que no reporta costo deja el total AUSENTE, no en cero.
func TestCA199_SinCostoElTotalQuedaAusente(t *testing.T) {
	installFakeNamed(t, "codex", `printf '{"type":"thread.started","thread_id":"t-1"}\n`+
		`{"type":"turn.completed","usage":{"input_tokens":17408,"cached_input_tokens":11264,"output_tokens":5}}\n'`+
		"\nexit 0\n")
	m := NewManager(t.TempDir())
	info, err := m.Start(StartOptions{Provider: "codex", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	fin := waitRun(t, m, info.ID)
	if fin.Usage == nil || fin.Usage.InputTokens != 17408 || fin.Usage.Turns != 1 {
		t.Fatalf("CA-199: los tokens si se cuentan: %+v", fin.Usage)
	}
	if fin.Usage.CostUSD != nil {
		t.Fatalf("CA-199: sin costo reportado el total queda ausente, no en 0: %v", *fin.Usage.CostUSD)
	}
}

// CA-200: el acumulado es durable — vive en el sidecar, que es lo unico que
// sobrevive al proceso — y el JSON omite lo que no se midio.
func TestCA200_ElSidecarGuardaElGasto(t *testing.T) {
	installFake(t, resultado)
	root := t.TempDir()
	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m, info.ID)

	meta := leerMeta(t, root, info.ID)
	if meta.Usage == nil || meta.Usage.CostUSD == nil || *meta.Usage.CostUSD != 0.111583 {
		t.Fatalf("CA-200: el sidecar guarda el gasto del run: %+v", meta.Usage)
	}
	if meta.Usage.Turns != 1 || meta.Usage.CachedTokens != 10126 {
		t.Fatalf("CA-200: con turnos y tokens: %+v", meta.Usage)
	}

	// Metas() —la fuente que status y el Studio leen— lo devuelve igual
	metas := Metas(root)
	if len(metas) != 1 || metas[0].Usage == nil || metas[0].Usage.CostUSD == nil {
		t.Fatalf("CA-200: Metas devuelve el gasto: %+v", metas)
	}

	// un run sin ninguna medicion no escribe un usage vacio en el sidecar
	installFake(t, `printf '{"type":"system","subtype":"init","session_id":"s-2"}\n'`+"\nexit 0\n")
	root2 := t.TempDir()
	m2 := NewManager(root2)
	otro, err := m2.Start(StartOptions{Provider: "claude", Prompt: "hola"})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, m2, otro.ID)
	if u := leerMeta(t, root2, otro.ID).Usage; u != nil {
		t.Fatalf("CA-200: sin medicion el sidecar no inventa un usage: %+v", u)
	}
}
