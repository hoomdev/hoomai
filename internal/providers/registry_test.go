// Tests adversariales del spec .hoom/specs/providers-v2-interfaz-y-claude.md
// (CA-108): el Registry rechaza nombres vacios y duplicados, preserva el
// orden de insercion en All/Detect, Lookup desconocido lista los soportados,
// y Default trae claude, opencode, codex, gemini con installed segun el PATH.
package providers

import (
	"reflect"
	"strings"
	"testing"
)

// stubProvider: Provider minimo, sin capacidades, para ejercitar el Registry
// sin depender de los adapters reales.
type stubProvider struct{ name string }

func (s stubProvider) Name() string               { return s.name }
func (s stubProvider) Bin() string                { return s.name }
func (s stubProvider) Capabilities() Capabilities { return Capabilities{} }
func (s stubProvider) Command(req Request) (Invocation, error) {
	return Invocation{Bin: s.name, Args: []string{req.Prompt}}, nil
}
func (s stubProvider) Normalize(line string) []Event {
	if strings.TrimSpace(line) == "" {
		return nil
	}
	return []Event{{Kind: "text", Detail: line}}
}

// CA-108: Register rechaza nombre vacio y duplicado; All y Detect conservan el
// orden de insercion; Lookup de un nombre desconocido falla listando los
// soportados; Lookup conocido resuelve.
func TestCA108_RegistroOrdenYLookup(t *testing.T) {
	r := NewRegistry()

	if err := r.Register(stubProvider{name: ""}); err == nil {
		t.Fatal("CA-108: Register debe rechazar un nombre vacio")
	}
	for _, n := range []string{"uno", "dos", "tres"} {
		if err := r.Register(stubProvider{name: n}); err != nil {
			t.Fatalf("CA-108: Register de %q no debe fallar: %v", n, err)
		}
	}
	if err := r.Register(stubProvider{name: "dos"}); err == nil {
		t.Fatal("CA-108: Register debe rechazar un nombre duplicado")
	}

	want := []string{"uno", "dos", "tres"}
	all := r.All()
	if len(all) != len(want) {
		t.Fatalf("CA-108: All debe traer %d providers, trae %d", len(want), len(all))
	}
	for i, n := range want {
		if all[i].Name() != n {
			t.Fatalf("CA-108: All debe respetar el orden de insercion: posicion %d es %q, esperaba %q", i, all[i].Name(), n)
		}
	}
	det := r.Detect()
	if len(det) != len(want) {
		t.Fatalf("CA-108: Detect debe traer %d entradas, trae %d", len(want), len(det))
	}
	for i, n := range want {
		if det[i].Name != n {
			t.Fatalf("CA-108: Detect debe respetar el orden de insercion: posicion %d es %q, esperaba %q", i, det[i].Name, n)
		}
	}

	_, err := r.Lookup("cuatro")
	if err == nil {
		t.Fatal("CA-108: Lookup de un nombre desconocido debe fallar")
	}
	for _, n := range want {
		if !strings.Contains(err.Error(), n) {
			t.Fatalf("CA-108: el error de Lookup desconocido debe listar los soportados, falta %q: %v", n, err)
		}
	}
	p, err := r.Lookup("dos")
	if err != nil || p.Name() != "dos" {
		t.Fatalf("CA-108: Lookup de un nombre conocido debe resolver: p=%v err=%v", p, err)
	}
}

// CA-108: Default trae claude, opencode, codex y gemini en ese orden; Detect
// reporta installed segun el PATH real (aca: solo claude presente).
func TestCA108_DefaultOrdenYDeteccion(t *testing.T) {
	t.Setenv("PATH", fakeBin(t, "claude")) // SOLO claude existe en este PATH

	var names []string
	for _, p := range All() {
		names = append(names, p.Name())
	}
	want := []string{"claude", "opencode", "codex", "gemini"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("CA-108: Default debe traer %v en ese orden, trae %v", want, names)
	}

	infos := Detect()
	if len(infos) != len(want) {
		t.Fatalf("CA-108: Detect debe traer %d providers, trae %d", len(want), len(infos))
	}
	got := map[string]bool{}
	for i, in := range infos {
		if in.Name != want[i] {
			t.Fatalf("CA-108: Detect debe respetar el orden de Default: posicion %d es %q", i, in.Name)
		}
		got[in.Name] = in.Installed
	}
	if !got["claude"] {
		t.Fatal("CA-108: claude esta en PATH y debe reportarse installed")
	}
	if got["opencode"] || got["codex"] || got["gemini"] {
		t.Fatalf("CA-108: providers ausentes no pueden reportarse installed: %v", got)
	}
}
