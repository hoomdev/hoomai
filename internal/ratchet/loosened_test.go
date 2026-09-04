// Tests adversariales del spec .hoom/specs/hoom-agent-sobre-determinista.md:
// el trinquete se compara por SIGNIFICADO, no por bytes — verify --full
// reescribe el archivo cada vez que el proyecto mejora.
package ratchet

import "testing"

func f(v float64) *float64 { return &v }

func base() *File {
	return &File{Schema: Schema, Metrics: map[string]*Metric{
		"cobertura": {Direction: "up", Value: f(80)},
		"deuda":     {Direction: "down", Value: f(10)},
	}}
}

// CA-138: Loosened nombra lo que empeoro o desaparecio; apretar, congelar y
// agregar metricas no son aflojar.
func TestCA138_LoosenedSoloNombraLoQueAfloja(t *testing.T) {
	casos := []struct {
		nombre string
		after  *File
		quiere []string
	}{
		{"peor en direccion up", &File{Schema: Schema, Metrics: map[string]*Metric{
			"cobertura": {Direction: "up", Value: f(79)},
			"deuda":     {Direction: "down", Value: f(10)},
		}}, []string{"cobertura"}},
		{"peor en direccion down", &File{Schema: Schema, Metrics: map[string]*Metric{
			"cobertura": {Direction: "up", Value: f(80)},
			"deuda":     {Direction: "down", Value: f(11)},
		}}, []string{"deuda"}},
		{"metrica eliminada", &File{Schema: Schema, Metrics: map[string]*Metric{
			"deuda": {Direction: "down", Value: f(10)},
		}}, []string{"cobertura"}},
		{"metrica descongelada", &File{Schema: Schema, Metrics: map[string]*Metric{
			"cobertura": {Direction: "up"},
			"deuda":     {Direction: "down", Value: f(10)},
		}}, []string{"cobertura"}},
		{"direccion invertida", &File{Schema: Schema, Metrics: map[string]*Metric{
			"cobertura": {Direction: "down", Value: f(80)},
			"deuda":     {Direction: "down", Value: f(10)},
		}}, []string{"cobertura"}},
		{"apretado", &File{Schema: Schema, Metrics: map[string]*Metric{
			"cobertura": {Direction: "up", Value: f(91)},
			"deuda":     {Direction: "down", Value: f(4)},
		}}, nil},
		{"metrica nueva", &File{Schema: Schema, Metrics: map[string]*Metric{
			"cobertura": {Direction: "up", Value: f(80)},
			"deuda":     {Direction: "down", Value: f(10)},
			"mutacion":  {Direction: "up", Value: f(60)},
		}}, nil},
		{"archivo entero perdido", &File{Schema: Schema}, []string{"cobertura", "deuda"}},
	}
	for _, c := range casos {
		got := Loosened(base(), c.after)
		if len(got) != len(c.quiere) {
			t.Fatalf("CA-138 [%s]: Loosened = %v, se esperaba %v", c.nombre, got, c.quiere)
		}
		for i := range got {
			if got[i] != c.quiere[i] {
				t.Fatalf("CA-138 [%s]: Loosened = %v, se esperaba %v", c.nombre, got, c.quiere)
			}
		}
	}
	// congelar por primera vez no afloja nada
	sinCongelar := &File{Schema: Schema, Metrics: map[string]*Metric{"cobertura": {Direction: "up"}}}
	if got := Loosened(sinCongelar, base()); len(got) != 0 {
		t.Fatalf("CA-138: congelar por primera vez no es aflojar: %v", got)
	}
	if got := Loosened(nil, base()); len(got) != 0 {
		t.Fatalf("CA-138: sin linea base previa no hay nada que aflojar: %v", got)
	}
	// la tolerancia manda: dentro del margen declarado no hay regresion
	tol := &File{Schema: Schema, Metrics: map[string]*Metric{
		"cobertura": {Direction: "up", Value: f(80), Tolerance: 1},
	}}
	after := &File{Schema: Schema, Metrics: map[string]*Metric{
		"cobertura": {Direction: "up", Value: f(79.5)},
	}}
	if got := Loosened(tol, after); len(got) != 0 {
		t.Fatalf("CA-138: dentro de la tolerancia no hay aflojada: %v", got)
	}
}
