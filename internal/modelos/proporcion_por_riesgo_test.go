// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre
// modelos (CA-805): la tabla de niveles es la del spec y es el unico lugar
// donde un nivel se vuelve un modelo; un nombre que no es nivel (Fuerte
// incluido) pasa tal cual; a gemini y opencode no les llega modelo.
// Escritos desde el spec, las aclaraciones de la ronda 0 y las firmas
// publicas, sin leer la implementacion.
package modelos_test

import (
	"reflect"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/modelos"
)

// CA-805: la tabla completa de niveles, en el orden del spec, como copia.
func TestCA805_TablaDeNiveles(t *testing.T) {
	const ca = "CA-805"
	want := []modelos.FilaDeNiveles{
		{Provider: "claude", Fuerte: "opus", Medio: "sonnet", Barato: "haiku"},
		{Provider: "codex", Fuerte: "gpt-6-astra", Medio: "gpt-6.1-sol", Barato: "gpt-6-luna"},
	}
	got := modelos.TablaDeNiveles()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: la tabla de niveles es la del spec: %+v", ca, got)
	}
	got[0].Medio = "otro"
	if again := modelos.TablaDeNiveles(); !reflect.DeepEqual(again, want) {
		t.Fatalf("%s: TablaDeNiveles devuelve una copia: %+v", ca, again)
	}
	if modelos.Fuerte != "fuerte" || modelos.Medio != "medio" || modelos.Barato != "barato" ||
		!reflect.DeepEqual(modelos.Niveles, []string{"fuerte", "medio", "barato"}) {
		t.Fatalf("%s: los niveles en minusculas, en su orden: %v", ca, modelos.Niveles)
	}
	for _, f := range want {
		for nivel, m := range map[string]string{"fuerte": f.Fuerte, "medio": f.Medio, "barato": f.Barato} {
			tr := modelos.Traducir(f.Provider, nivel)
			if tr.Modelo != m || tr.Nivel != nivel || !tr.Recibe {
				t.Fatalf("%s: %s a %s es %s: %+v", ca, nivel, f.Provider, m, tr)
			}
		}
	}
}

// CA-805: medio llega a claude como sonnet y a codex como gpt-6.1-sol; a
// gemini y opencode no les llega ningun modelo (con el nivel dicho); un
// provider desconocido o vacio tampoco.
func TestCA805_TraducirMedioYProvidersSinModelo(t *testing.T) {
	const ca = "CA-805"
	if tr := modelos.Traducir("claude", "medio"); tr != (modelos.Traduccion{Modelo: "sonnet", Nivel: "medio", Recibe: true}) {
		t.Fatalf("%s: medio en claude: %+v", ca, tr)
	}
	if tr := modelos.Traducir("codex", "medio"); tr != (modelos.Traduccion{Modelo: "gpt-6.1-sol", Nivel: "medio", Recibe: true}) {
		t.Fatalf("%s: medio en codex: %+v", ca, tr)
	}
	for _, p := range []string{"gemini", "opencode", "otro", ""} {
		for _, n := range modelos.Niveles {
			tr := modelos.Traducir(p, n)
			if tr.Modelo != "" || tr.Nivel != n || tr.Recibe {
				t.Fatalf("%s: a %q no le llega modelo para %s: %+v", ca, p, n, tr)
			}
		}
	}
	if modelos.RecibeModelo("gemini") || modelos.RecibeModelo("opencode") || !modelos.RecibeModelo("claude") || !modelos.RecibeModelo("codex") {
		t.Fatalf("%s: RecibeModelo", ca)
	}
}

// CA-805: un nombre que no es nivel pasa tal cual y sin nivel: Fuerte, MEDIO,
// con espacios, un modelo real; tambien en un provider sin modelo, como hoy;
// vacio es vacio.
func TestCA805_NombreQueNoEsNivelPasa(t *testing.T) {
	const ca = "CA-805"
	for _, m := range []string{"Fuerte", "MEDIO", "Barato", " medio", "medio ", "opus", "gpt-7-sol", "fuerte-2", "medio\n"} {
		if modelos.EsNivel(m) {
			t.Fatalf("%s: %q no es un nivel", ca, m)
		}
		for _, p := range []string{"claude", "codex"} {
			if tr := modelos.Traducir(p, m); tr.Modelo != m || tr.Nivel != "" || !tr.Recibe {
				t.Fatalf("%s: %q pasa tal cual a %s: %+v", ca, m, p, tr)
			}
		}
		if tr := modelos.Traducir("gemini", m); tr.Modelo != m || tr.Nivel != "" || tr.Recibe {
			t.Fatalf("%s: un nombre en gemini va como hoy (Recibe false): %+v", ca, tr)
		}
	}
	for _, n := range []string{"fuerte", "medio", "barato"} {
		if !modelos.EsNivel(n) {
			t.Fatalf("%s: %q es un nivel", ca, n)
		}
	}
	if tr := modelos.Traducir("claude", ""); tr.Modelo != "" || tr.Nivel != "" {
		t.Fatalf("%s: un modelo vacio es vacio: %+v", ca, tr)
	}
	// propiedad: lo que no es nivel no cambia
	prop := func(m string) bool {
		if modelos.EsNivel(m) {
			return true
		}
		a, b := modelos.Traducir("claude", m), modelos.Traducir("codex", m)
		return a.Modelo == m && b.Modelo == m && a.Nivel == "" && b.Nivel == ""
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Fatalf("%s: un nombre pasa tal cual: %v", ca, err)
	}
}
