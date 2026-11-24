// Tests adversariales del spec .hoom/specs/proporcion-por-riesgo.md sobre el
// sobre (`hoom agent`): la nota de riesgo al final del SystemPrompt del
// test-writer y del arquitecto (CA-799, que re-expresa CA-182), los valores
// por riesgo de la tarjeta de --task (CA-804), el nivel traducido con el
// provider que toma el sobre (CA-805) y la linea valores: una sola vez antes
// de los pasos (CA-806). Escritos desde el spec, las aclaraciones de la ronda
// 0 y las firmas publicas, sin leer la implementacion.
package agentcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/spec"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const pprItemBase = "titulo: Demo\ntipo: feature\nprioridad: media\ncreado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n"

// pprFalso pone un claude falso que anota su argv (NUL) en dir y, si
// entrega no es "", escribe ese archivo de test.
func pprFalso(t *testing.T, entrega string) string {
	t.Helper()
	dir := t.TempDir()
	script := "printf '%s\\0' \"$@\" > '" + dir + "/argv.'$$\n"
	if entrega != "" {
		script += "printf 'package app\\n// CA-1\\n' > " + entrega + "\n"
	}
	fakeProvider(t, "claude", script+"exit 0\n")
	return dir
}

// pprConNota devuelve el unico argumento que trae la nota de riesgo.
func pprConNota(t *testing.T, ca string, args []string) string {
	t.Helper()
	var hits []string
	for _, a := range args {
		if strings.Contains(a, "Riesgo declarado de la tarjeta:") {
			hits = append(hits, a)
		}
	}
	if len(hits) != 1 {
		t.Fatalf("%s: un solo argumento (el SystemPrompt) trae la nota; fueron %d: %q", ca, len(hits), args)
	}
	return hits[0]
}

// pprTarea crea la tarea slug con su item (itemExtra; "-" = sin item;
// "roto" = ilegible) en el proyecto root.
func pprTarea(t *testing.T, root, slug, itemExtra string) {
	t.Helper()
	switch itemExtra {
	case "-":
	case "roto":
		write(t, root, ".hoom/items/"+slug+".yaml", "titulo: [\nno es yaml\n")
	default:
		write(t, root, ".hoom/items/"+slug+".yaml", pprItemBase+itemExtra)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "--allow-empty", "-m", "item "+slug)
	if err := taskcmd.Start(root, slug, "main"); err != nil {
		t.Fatalf("fixture: hoom task start %s: %v", slug, err)
	}
}

// CA-799: NotaDeRiesgo con sus textos exactos; los demas roles no tienen nota.
func TestCA799_NotaDeRiesgo(t *testing.T) {
	const ca = "CA-799"
	for _, c := range []struct {
		rol, r string
		tope   spec.Tope
		want   string
	}{
		{"test-writer", "alto", spec.Tope{Palabras: 2000, Criterios: 15}, "Riesgo declarado de la tarjeta: alto."},
		{"test-writer", "bajo", spec.Tope{}, "Riesgo declarado de la tarjeta: bajo."},
		{"arquitecto", "normal", spec.Tope{Palabras: 800, Criterios: 8, CitasPorCriterio: 10}, "Riesgo declarado de la tarjeta: normal. Tope del spec: 800 palabras y 8 criterios."},
		{"arquitecto", "alto", spec.Tope{Palabras: 0, Criterios: 15}, "Riesgo declarado de la tarjeta: alto. Tope del spec: palabras sin tope y 15 criterios."},
		{"arquitecto", "bajo", spec.Tope{Palabras: 400, Criterios: 0}, "Riesgo declarado de la tarjeta: bajo. Tope del spec: 400 palabras y criterios sin tope."},
		{"arquitecto", "normal", spec.Tope{}, "Riesgo declarado de la tarjeta: normal. Tope del spec: palabras sin tope y criterios sin tope."},
	} {
		if got := NotaDeRiesgo(c.rol, c.r, c.tope); got != c.want {
			t.Fatalf("%s: NotaDeRiesgo(%s, %s, %+v)\n%q\nfue %q", ca, c.rol, c.r, c.tope, c.want, got)
		}
	}
	for _, rol := range []string{"writer", "scout", "reviewer", "designer", "characterizer", "analista", "refutador", ""} {
		if got := NotaDeRiesgo(rol, "alto", spec.Tope{Palabras: 2000, Criterios: 15}); got != "" {
			t.Fatalf("%s: %q no recibe nota: %q", ca, rol, got)
		}
	}
}

// CA-799: el SystemPrompt del test-writer termina, despues de una linea en
// blanco y del parrafo del arbol ciego, en la nota exacta y nada mas; sin
// --task es normal. Re-expresa CA-182: siguen siendo seis pasos y el contrato
// en disco no cambia.
func TestCA799_TestWriterTerminaEnLaNota(t *testing.T) {
	const ca = "CA-799"
	argv := pprFalso(t, "nuevo_test.go")
	root := repoCiego(t)
	contrato := filepath.Join(root, ".hoom", "agents", "05-test-writer.md")
	antes, errAntes := os.ReadFile(contrato)
	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "test-writer", Prompt: "tests"}, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "[1/6] spec") || !strings.Contains(buf.String(), "[6/6] check") {
		t.Fatalf("%s: siguen siendo seis pasos:\n%s", ca, buf.String())
	}
	sp := pprConNota(t, ca, vdtUnaInvocacion(t, ca, "test-writer", argv))
	if !strings.HasSuffix(sp, "\n\nRiesgo declarado de la tarjeta: normal.") {
		t.Fatalf("%s: termina en la linea en blanco y la nota, sin salto final:\n%q", ca, sp[max(0, len(sp)-200):])
	}
	ic, in := strings.Index(sp, "Este arbol es CIEGO"), strings.Index(sp, "Riesgo declarado de la tarjeta:")
	if ic < 0 || ic > in || !strings.Contains(sp, "Regla de oro") {
		t.Fatalf("%s: la nota va despues del contrato y del parrafo ciego (%d, %d)", ca, ic, in)
	}
	despues, errDespues := os.ReadFile(contrato)
	if (errAntes == nil) != (errDespues == nil) || !bytes.Equal(antes, despues) {
		t.Fatalf("%s: el contrato en disco no cambia", ca)
	}
}

// CA-799: con --task, el riesgo declarado de su tarjeta; con la tarjeta
// ilegible, alto; sin item, normal.
func TestCA799_TestWriterConElRiesgoDeSuTarjeta(t *testing.T) {
	const ca = "CA-799"
	for _, c := range []struct {
		itemExtra, want string
	}{
		{"riesgo: alto\n", "alto"},
		{"riesgo: bajo\n", "bajo"},
		{"", "normal"},
		{"roto", "alto"},
		{"-", "normal"},
	} {
		argv := pprFalso(t, "nuevo_test.go")
		root := repoCiego(t)
		pprTarea(t, root, "demo", c.itemExtra)
		var buf bytes.Buffer
		if _, err := Run(root, "main", Options{Role: "test-writer", Task: "demo", Prompt: "tests"}, &buf); err != nil {
			t.Fatalf("%s: item %q: %v\n%s", ca, c.itemExtra, err, buf.String())
		}
		sp := pprConNota(t, ca, vdtUnaInvocacion(t, ca, "test-writer con --task", argv))
		if !strings.HasSuffix(sp, "\n\nRiesgo declarado de la tarjeta: "+c.want+".") {
			t.Fatalf("%s: item %q: la nota dice %s: %q", ca, c.itemExtra, c.want, sp[max(0, len(sp)-120):])
		}
	}
}

// CA-799: el SystemPrompt del arquitecto termina en la nota con su tope: el
// de spec.topes del riesgo, el de la tarjeta, sin tope con 0, y alto con la
// tarjeta ilegible (aclaracion 13).
func TestCA799_ArquitectoConSuTope(t *testing.T) {
	const ca = "CA-799"
	for _, c := range []struct {
		yaml, itemExtra, want string
	}{
		{"", "", "Riesgo declarado de la tarjeta: normal. Tope del spec: 800 palabras y 8 criterios."},
		{"", "-", "Riesgo declarado de la tarjeta: normal. Tope del spec: 800 palabras y 8 criterios."},
		{"", "riesgo: bajo\n", "Riesgo declarado de la tarjeta: bajo. Tope del spec: 400 palabras y 4 criterios."},
		{"", "tope_palabras: 1500\ntope_motivo: largo\n", "Riesgo declarado de la tarjeta: normal. Tope del spec: 1500 palabras y 8 criterios."},
		{"spec:\n  topes:\n    alto: { palabras: 0 }\n", "riesgo: alto\n", "Riesgo declarado de la tarjeta: alto. Tope del spec: palabras sin tope y 15 criterios."},
		{"spec:\n  topes:\n    alto: { criterios: 20 }\n", "roto", "Riesgo declarado de la tarjeta: alto. Tope del spec: 2000 palabras y 20 criterios."},
		{"", "riesgo: alto\ntope_criterios: 0\ntope_motivo: sin tope\n", "Riesgo declarado de la tarjeta: alto. Tope del spec: 2000 palabras y criterios sin tope."},
	} {
		argv := pprFalso(t, "")
		root := vdtRepo(t, c.yaml)
		pprTarea(t, root, "demo", c.itemExtra)
		var buf bytes.Buffer
		if _, err := Run(root, "main", Options{Role: "arquitecto", Task: "demo", Prompt: "escribi el spec"}, &buf); err != nil {
			t.Fatalf("%s: %q/%q: %v\n%s", ca, c.yaml, c.itemExtra, err, buf.String())
		}
		sp := pprConNota(t, ca, vdtUnaInvocacion(t, ca, "arquitecto", argv))
		if !strings.HasSuffix(sp, "\n\n"+c.want) {
			t.Fatalf("%s: %q/%q: termina en\n%q\nfue %q", ca, c.yaml, c.itemExtra, c.want, sp[max(0, len(sp)-160):])
		}
	}
}

// CA-799: el writer no recibe nota, tampoco con una tarjeta alto.
func TestCA799_WriterSinNota(t *testing.T) {
	const ca = "CA-799"
	argv := pprFalso(t, "")
	root := vdtRepo(t, "")
	pprTarea(t, root, "demo", "riesgo: alto\n")
	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "writer", Task: "demo", Prompt: "implementa"}, &buf); err != nil {
		t.Fatal(err)
	}
	for _, a := range vdtUnaInvocacion(t, ca, "writer", argv) {
		if strings.Contains(a, "Riesgo declarado de la tarjeta") {
			t.Fatalf("%s: el writer no recibe nota: %q", ca, a)
		}
	}
}

// CA-804: el writer en una tarjeta alto con agents.writer.alto.model lanza
// con ese modelo y origen `hoom.yaml, riesgo alto`; normal, con
// agents.writer.model; el comando gana; la tarjeta ilegible usa alto; sin
// --task, normal.
func TestCA804_AgentConElRiesgoDeLaTarjeta(t *testing.T) {
	const ca = "CA-804"
	yaml := "agents:\n  writer:\n    model: m-yaml\n    alto: { model: m-alto, effort: e-alto }\n"
	for _, c := range []struct {
		itemExtra, model, cmd, want, origen string
	}{
		{"riesgo: alto\n", "", "m-alto", "m-alto", "hoom.yaml, riesgo alto"},
		{"", "", "m-yaml", "m-yaml", "hoom.yaml"},
		{"riesgo: alto\n", "m-cmd", "m-cmd", "m-cmd", "comando"},
		{"roto", "", "m-alto", "m-alto", "hoom.yaml, riesgo alto"},
		{"-", "", "m-yaml", "m-yaml", "hoom.yaml"},
	} {
		argv := pprFalso(t, "")
		root := vdtRepo(t, yaml)
		pprTarea(t, root, "demo", c.itemExtra)
		var buf bytes.Buffer
		if _, err := Run(root, "main", Options{Role: "writer", Task: "demo", Model: c.model, Prompt: "implementa"}, &buf); err != nil {
			t.Fatalf("%s: %q: %v\n%s", ca, c.itemExtra, err, buf.String())
		}
		args := vdtUnaInvocacion(t, ca, "writer", argv)
		if v, _ := vdtSigue(args, "--model"); v != c.cmd {
			t.Fatalf("%s: item %q: --model %s: %q", ca, c.itemExtra, c.cmd, args)
		}
		if !strings.Contains(buf.String(), " · modelo "+c.want+" ("+c.origen+") · ") {
			t.Fatalf("%s: item %q: la linea valores: dice modelo %s (%s):\n%s", ca, c.itemExtra, c.want, c.origen, buf.String())
		}
	}
	// sin --task: normal
	argv := pprFalso(t, "")
	root := vdtRepo(t, yaml)
	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf); err != nil {
		t.Fatal(err)
	}
	if v, _ := vdtSigue(vdtUnaInvocacion(t, ca, "sin task", argv), "--model"); v != "m-yaml" {
		t.Fatalf("%s: sin --task el riesgo es normal: %q", ca, v)
	}
}

// CA-805 y CA-806: un nivel llega al provider traducido (medio a claude es
// sonnet, tambien con el provider que hoom agent toma por estar instalado),
// y la linea valores: sale una vez, despues de la cabecera y antes de [1/5],
// con `nivel medio`.
func TestCA806_AgentImprimeValoresUnaVezConElNivel(t *testing.T) {
	const ca = "CA-806"
	argv := pprFalso(t, "")
	root := vdtRepo(t, "agents:\n  writer:\n    model: medio\n")
	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "writer", Prompt: "implementa"}, &buf); err != nil {
		t.Fatal(err)
	}
	if v, _ := vdtSigue(vdtUnaInvocacion(t, ca, "writer", argv), "--model"); v != "sonnet" {
		t.Fatalf("CA-805: medio llega a claude como sonnet: %q", v)
	}
	lineas := strings.Split(buf.String(), "\n")
	iv, ip, ih, n := -1, -1, -1, 0
	for i, l := range lineas {
		switch {
		case strings.HasPrefix(strings.TrimSpace(l), "valores: "):
			n++
			iv = i
		case ip < 0 && strings.Contains(l, "[1/5]"):
			ip = i
		case ih < 0 && strings.HasPrefix(l, "hoom agent: rol"):
			ih = i
		}
	}
	if n != 1 || iv < 0 || ip < 0 || iv > ip || (ih >= 0 && ih > iv) {
		t.Fatalf("%s: una linea valores: (%d), despues de la cabecera (%d) y antes de [1/5] (%d): %d\n%s", ca, n, ih, ip, iv, buf.String())
	}
	if !strings.Contains(lineas[iv], " · modelo sonnet (nivel medio, hoom.yaml) · ") {
		t.Fatalf("%s: modelo sonnet (nivel medio, hoom.yaml): %q", ca, lineas[iv])
	}
	for _, nivel := range []string{"modelo medio ", "modelo fuerte ", "modelo barato "} {
		if strings.Contains(buf.String(), nivel) {
			t.Fatalf("%s: ningun nivel llega sin traducir a la pantalla:\n%s", ca, buf.String())
		}
	}
	if !strings.Contains(buf.String(), "[5/5]") || strings.Contains(buf.String(), "[6/5]") {
		t.Fatalf("%s: valores: no es un paso: siguen cinco:\n%s", ca, buf.String())
	}
}

// CA-806: con ValoresImpresos (el verbo ya la imprimio) el sobre no la
// imprime; --model fuerte del comando se traduce con su nivel.
func TestCA806_ConValoresImpresosNoLaRepite(t *testing.T) {
	const ca = "CA-806"
	argv := pprFalso(t, "")
	root := vdtRepo(t, "")
	var buf bytes.Buffer
	if _, err := Run(root, "main", Options{Role: "writer", Model: "fuerte", Prompt: "implementa", ValoresImpresos: true}, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "valores: ") {
		t.Fatalf("%s: con ValoresImpresos no se repite:\n%s", ca, buf.String())
	}
	if v, _ := vdtSigue(vdtUnaInvocacion(t, ca, "writer", argv), "--model"); v != "opus" {
		t.Fatalf("CA-805: --model fuerte llega a claude como opus: %q", v)
	}
	// sin ValoresImpresos, el mismo pedido la imprime con el origen comando
	buf.Reset()
	if _, err := Run(root, "main", Options{Role: "writer", Model: "fuerte", Prompt: "implementa"}, &buf); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buf.String(), "valores: ") != 1 || !strings.Contains(buf.String(), " · modelo opus (nivel fuerte, comando) · ") {
		t.Fatalf("%s: la linea con el nivel del comando:\n%s", ca, buf.String())
	}
}
