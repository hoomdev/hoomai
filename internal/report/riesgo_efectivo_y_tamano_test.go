// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// el texto de `hoom verify` (report.Render, CA-781): las lineas `riesgo:` y
// `tamano:` bajo `cambio:`, el aviso con `hoom item set <slug> --riesgo
// <efectivo>` cuando el efectivo supera al declarado (solo con tarea), el de
// 400 lineas de producto solo con efectivo normal, y el viejo `cambio grande
// (>400 lineas)` que ya no sale. Escritos desde el spec, las aclaraciones de
// la ronda 0 y las firmas publicas, sin leer la implementacion.
package report

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const retyViejo = "cambio grande (>400 lineas)"

// retyVeredicto es un veredicto verde de un repo con un cambio de +9300/-81
// lineas en git.*, y riesgo, tamano y tarea dados (nil = sin la clave).
func retyVeredicto(ev *riesgo.Evaluacion, tam *riesgo.Tamano, tarea string) *verdict.Verdict {
	v := synth(time.Date(2026, 11, 20, 10, 0, 0, 0, time.UTC), "20261120T100000_abcdef",
		verdict.GateResult{Name: "test", Required: true, Status: verdict.StatusPass, Scope: "full"})
	v.Git = gitx.Info{IsRepo: true, Commit: strings.Repeat("a", 40), Branch: "hoom/precios", Base: "main",
		ChangedFiles: []string{"internal/auth/x.go", "internal/precio_test.go"}, Insertions: 9300, Deletions: 81}
	v.Riesgo = ev
	v.Tamano = tam
	v.Tarea = tarea
	return v
}

func retyRender(v *verdict.Verdict) []string {
	var b bytes.Buffer
	Render(&b, v, ".hoom/verdicts/20261120T100000_abcdef.json")
	return strings.Split(b.String(), "\n")
}

// retyLinea es el indice de la primera linea que, sin los blancos de los
// bordes, es exactamente l; -1 si ninguna.
func retyLinea(lineas []string, l string) int {
	for i, x := range lineas {
		if strings.TrimSpace(x) == l {
			return i
		}
	}
	return -1
}

func retyCambio(lineas []string) int {
	for i, x := range lineas {
		if strings.HasPrefix(strings.TrimSpace(x), "cambio:") {
			return i
		}
	}
	return -1
}

func retySinViejo(t *testing.T, ca string, lineas []string) {
	t.Helper()
	if s := strings.Join(lineas, "\n"); strings.Contains(s, retyViejo) {
		t.Fatalf("%s: el aviso viejo %q ya no sale:\n%s", ca, retyViejo, s)
	}
}

// CA-781: las lineas riesgo y tamano, y el aviso de mayor que el declarado
// con el slug de la tarea; con efectivo alto no sale el de 400 lineas.
func TestCA781_LineasYAvisoMayorQueDeclarado(t *testing.T) {
	const ca = "CA-781"
	ev := &riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: "el cambio toca internal/auth/x.go: riesgo alto"}
	lineas := retyRender(retyVeredicto(ev, &riesgo.Tamano{Producto: 900, Pruebas: 1500, Hoom: 30}, "precios"))
	texto := strings.Join(lineas, "\n")
	r := retyLinea(lineas, "riesgo: alto (el cambio toca internal/auth/x.go: riesgo alto)")
	tm := retyLinea(lineas, "tamano: producto 900 · pruebas 1500 · hoom 30")
	av := retyLinea(lineas, "! el riesgo efectivo (alto) es mayor que el declarado (normal): hoom item set precios --riesgo alto lo sube")
	if r < 0 || tm < 0 || av < 0 {
		t.Fatalf("%s: el texto trae riesgo (%d), tamano (%d) y el aviso (%d):\n%s", ca, r, tm, av, texto)
	}
	if !(r < tm && tm < av) {
		t.Fatalf("%s: van riesgo, tamano y el aviso, en ese orden:\n%s", ca, texto)
	}
	if c := retyCambio(lineas); c >= 0 && c > r {
		t.Fatalf("%s: las lineas van debajo de `cambio:`:\n%s", ca, texto)
	}
	if retyLinea(lineas, riesgo.AvisoProductoGrande) >= 0 {
		t.Fatalf("%s: con efectivo alto no sale el aviso de 400 lineas de producto:\n%s", ca, texto)
	}
	retySinViejo(t, ca, lineas)
}

// CA-781 y aclaracion 11: sin tarea no sale el aviso de mayor que el
// declarado, ni con efectivo alto; con efectivo igual al declarado tampoco.
func TestCA781_ElAvisoDeMayorSoloConTarea(t *testing.T) {
	const ca = "CA-781"
	for _, k := range []struct {
		ev    riesgo.Evaluacion
		tarea string
	}{
		{riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: "el cambio toca internal/auth/x.go: riesgo alto"}, ""},
		{riesgo.Evaluacion{Declarado: "alto", Efectivo: "alto", Motivo: "declarado alto"}, "precios"},
		{riesgo.Evaluacion{Declarado: "normal", Efectivo: "normal", Motivo: "declarado normal"}, "precios"},
		{riesgo.Evaluacion{Declarado: "bajo", Efectivo: "bajo", Motivo: "declarado bajo"}, "precios"},
	} {
		ev := k.ev
		lineas := retyRender(retyVeredicto(&ev, &riesgo.Tamano{Producto: 10}, k.tarea))
		texto := strings.Join(lineas, "\n")
		if strings.Contains(texto, "es mayor que el declarado") {
			t.Fatalf("%s: %+v con tarea %q no lleva el aviso de mayor que el declarado:\n%s", ca, ev, k.tarea, texto)
		}
		if retyLinea(lineas, ev.Linea()) < 0 {
			t.Fatalf("%s: la linea %q sale siempre:\n%s", ca, ev.Linea(), texto)
		}
	}
	ev := riesgo.Evaluacion{Declarado: "bajo", Efectivo: "normal", Motivo: "el cambio toca main.go, fuera de las rutas de bajo riesgo: riesgo normal"}
	lineas := retyRender(retyVeredicto(&ev, &riesgo.Tamano{Producto: 10}, "precios"))
	if retyLinea(lineas, "! el riesgo efectivo (normal) es mayor que el declarado (bajo): hoom item set precios --riesgo normal lo sube") < 0 {
		t.Fatalf("%s: bajo que sube a normal lleva el aviso:\n%s", ca, strings.Join(lineas, "\n"))
	}
}

// CA-781 (actualizado por CAL-2): normal > 400 ya no sube a 4 lentes, asi que
// el aviso de 400 lineas no sale para ningun riesgo.
func TestCA781_ElAvisoDeProductoGrande(t *testing.T) {
	const ca = "CA-781"
	for _, k := range []struct {
		efectivo string
		producto int
		sale     bool
	}{
		{"normal", 401, false},
		{"normal", 400, false},
		{"normal", 90, false},
		{"bajo", 401, false},
		{"bajo", 5000, false},
		{"alto", 401, false},
	} {
		ev := &riesgo.Evaluacion{Declarado: k.efectivo, Efectivo: k.efectivo, Motivo: "declarado " + k.efectivo}
		lineas := retyRender(retyVeredicto(ev, &riesgo.Tamano{Producto: k.producto, Pruebas: 3000}, "precios"))
		texto := strings.Join(lineas, "\n")
		if got := retyLinea(lineas, "! mas de 400 lineas de producto: la review exige las 4 lentes") >= 0; got != k.sale {
			t.Fatalf("%s: efectivo %s y producto %d: el aviso de 400 lineas sale=%v, fue %v:\n%s", ca, k.efectivo, k.producto, k.sale, got, texto)
		}
		if retyLinea(lineas, (&riesgo.Tamano{Producto: k.producto, Pruebas: 3000}).Linea()) < 0 {
			t.Fatalf("%s: la linea tamano sale:\n%s", ca, texto)
		}
		retySinViejo(t, ca, lineas)
	}
}

// CA-781 y aclaracion 16: un veredicto anterior al spec (sin riesgo ni
// tamano) no imprime esas lineas ni el aviso viejo, aunque git diga +9300;
// sin tamano (fuera de un repo) sale solo la de riesgo.
func TestCA781_VeredictoSinLasClaves(t *testing.T) {
	const ca = "CA-781"
	lineas := retyRender(retyVeredicto(nil, nil, ""))
	texto := strings.Join(lineas, "\n")
	for _, l := range lineas {
		s := strings.TrimSpace(l)
		if strings.HasPrefix(s, "riesgo:") || strings.HasPrefix(s, "tamano:") || strings.HasPrefix(s, "! mas de 400") {
			t.Fatalf("%s: un veredicto sin riesgo ni tamano no imprime %q:\n%s", ca, s, texto)
		}
	}
	retySinViejo(t, ca, lineas)
	ev := &riesgo.Evaluacion{Declarado: "normal", Efectivo: "normal", Motivo: "sin tarjeta: riesgo normal"}
	lineas = retyRender(retyVeredicto(ev, nil, ""))
	if retyLinea(lineas, "riesgo: normal (sin tarjeta: riesgo normal)") < 0 {
		t.Fatalf("%s: sin tamano sale la linea de riesgo:\n%s", ca, strings.Join(lineas, "\n"))
	}
	for _, l := range lineas {
		if strings.HasPrefix(strings.TrimSpace(l), "tamano:") {
			t.Fatalf("%s: sin tamano no sale la linea tamano:\n%s", ca, strings.Join(lineas, "\n"))
		}
	}
	retySinViejo(t, ca, lineas)
}
