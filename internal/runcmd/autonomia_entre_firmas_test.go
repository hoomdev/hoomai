// Tests adversariales del spec .hoom/specs/autonomia-entre-firmas.md
// (CA-651) sobre el corte por tiempo: un run que muere por su timeout deja
// `"corte": "tiempo"` en su sidecar (Meta.Corte) y en su estado (Run.Corte),
// con el cierre de siempre; un run que termina de cualquier otra forma no
// lleva la clave. CLIs falsos al frente del PATH, tiempos de segundos.
package runcmd

import (
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// aerCorteCrudo es el valor crudo de "corte" en el sidecar de id, y si esta.
func aerCorteCrudo(t *testing.T, root, id string) (string, bool) {
	t.Helper()
	v, ok := vdtMetaCruda(t, root, id)["corte"]
	return strings.TrimSpace(string(v)), ok
}

// CA-651: el valor es el mismo en el sidecar y en el registro del sobre.
func TestCA651_CorteTiempoEsTiempo(t *testing.T) {
	if CorteTiempo != "tiempo" || envelope.CorteTiempo != CorteTiempo {
		t.Fatalf("CA-651: corte: \"tiempo\" en el sidecar y en el sobre: %q %q", CorteTiempo, envelope.CorteTiempo)
	}
}

// CA-651: el run que vence su Timeout cierra como siempre (error) y su
// sidecar y su estado dicen corte tiempo; tambien cuando el tiempo limite es
// el del Manager.
func TestCA651_ElRunQueMuerePorSuTimeoutDejaCorteTiempo(t *testing.T) {
	for _, c := range []struct {
		nombre  string
		manager time.Duration
		opcion  time.Duration
	}{
		{"timeout del run", 0, 2 * time.Second},
		{"timeout del Manager", 2 * time.Second, 0},
	} {
		installFake(t, "printf 'trabajando\\n'\nexec sleep 60\n")
		root := vdtRaiz(t)
		m := NewManager(root)
		if c.manager > 0 {
			m.Timeout = c.manager
		}
		fin, evs, _ := vdtCorrer(t, "CA-651", m, StartOptions{Provider: "claude", Prompt: "trabaja", Role: "writer", Timeout: c.opcion}, 30*time.Second)
		if fin.Status != StatusError || vdtDetalleTimeout(evs) == "" {
			t.Fatalf("CA-651: %s: el cierre no cambia (error con su detalle de timeout): %+v\n%s", c.nombre, fin, allDetails(evs))
		}
		if fin.Corte != CorteTiempo {
			t.Fatalf("CA-651: %s: Run.Corte es %q: %+v", c.nombre, CorteTiempo, fin)
		}
		meta, ok := ReadMeta(root, fin.ID)
		if !ok || meta.Corte != CorteTiempo || meta.Status != StatusError {
			t.Fatalf("CA-651: %s: el sidecar trae corte tiempo y el status de siempre: %+v", c.nombre, meta)
		}
		if v, ok := aerCorteCrudo(t, root, fin.ID); !ok || v != `"tiempo"` {
			t.Fatalf("CA-651: %s: el sidecar en disco tiene \"corte\": \"tiempo\", tiene %q (%v)", c.nombre, v, ok)
		}
		if g, err := m.Get(fin.ID); err != nil || g.Corte != CorteTiempo {
			t.Fatalf("CA-651: %s: Get dice el corte: %+v %v", c.nombre, g, err)
		}
		if ms := Metas(root); len(ms) != 1 || ms[0].Corte != CorteTiempo {
			t.Fatalf("CA-651: %s: Metas lee el corte: %+v", c.nombre, ms)
		}
	}
}

// CA-651: un run que termina de otra forma (bien, con un exit distinto de 0,
// cancelado, o narrando la palabra timeout) no lleva la clave en el sidecar.
func TestCA651_UnRunQueTerminaDeOtraFormaNoLlevaCorte(t *testing.T) {
	for _, c := range []struct {
		nombre, script, status string
	}{
		{"termina bien", "printf 'listo\\n'\nexit 0\n", StatusDone},
		{"falla", "printf 'algo salio mal\\n' >&2\nexit 3\n", StatusError},
		{"narra un timeout ajeno", "printf 'timeout tras 2s: corte tiempo\\n'\nprintf '{\"corte\":\"tiempo\"}\\n'\nexit 1\n", StatusError},
	} {
		installFake(t, c.script)
		root := vdtRaiz(t)
		m := NewManager(root)
		fin, evs, _ := vdtCorrer(t, "CA-651", m, StartOptions{Provider: "claude", Prompt: "trabaja", Role: "writer", Timeout: 20 * time.Second}, 30*time.Second)
		if fin.Status != c.status {
			t.Fatalf("CA-651: fixture: %s cierra %s: %+v\n%s", c.nombre, c.status, fin, allDetails(evs))
		}
		if fin.Corte != "" {
			t.Fatalf("CA-651: %s: sin corte por tiempo Run.Corte es \"\": %+v", c.nombre, fin)
		}
		if v, ok := aerCorteCrudo(t, root, fin.ID); ok {
			t.Fatalf("CA-651: %s: el sidecar no lleva la clave corte, lleva %s", c.nombre, v)
		}
	}

	// cancelado: tampoco es un corte por tiempo
	installFake(t, "exec sleep 60\n")
	root := vdtRaiz(t)
	m := NewManager(root)
	info, err := m.Start(StartOptions{Provider: "claude", Prompt: "trabaja", Role: "writer", Timeout: 20 * time.Second})
	if err != nil {
		t.Fatalf("CA-651: fixture: el run arranca: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := m.Cancel(info.ID); err != nil {
		t.Fatalf("CA-651: fixture: Cancel: %v", err)
	}
	listo := make(chan Run, 1)
	go func() { listo <- m.Wait(info.ID) }()
	var fin Run
	select {
	case fin = <-listo:
	case <-time.After(20 * time.Second):
		t.Fatal("CA-651: fixture: el run cancelado no cerro")
	}
	if fin.Status != StatusCanceled || fin.Corte != "" {
		t.Fatalf("CA-651: un run cancelado no lleva corte: %+v", fin)
	}
	if v, ok := aerCorteCrudo(t, root, fin.ID); ok {
		t.Fatalf("CA-651: cancelado: el sidecar no lleva la clave corte, lleva %s", v)
	}
}
