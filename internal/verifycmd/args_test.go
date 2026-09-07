// Tests adversariales del spec .hoom/specs/verify-args-estrictos.md
// (CA-217..CA-222, CA-225, CA-227): `hoom verify` no ejecuta nada que no le
// pidieron. Dos etapas: ParseArgs juzga la sintaxis SIN tocar el disco, y Run
// juzga los nombres de gate contra el manifiesto ANTES de narrar y antes de
// correr un solo gate. Escritos desde el spec, sin leer la implementacion.
package verifycmd

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// usoEsperado es el bloque de uso EXACTO tal como lo fija el spec. No se
// importa de la implementacion a proposito: si UsageText se desvia del spec,
// este archivo tiene que fallar.
const usoEsperado = `Uso: hoom verify [--full] [--gate a,b] [--spec <ruta>] [--json]

  --full          Ignora el scoping por diff (corrida completa, ej. nocturna)
  --gate a,b      Ejecuta solo esos gates (el resto queda 'skipped'); el
                  veredicto queda PARCIAL: diagnostico, nunca referencia
                  de 'hoom check' ni de 'hoom task done'
  --spec <ruta>   Asocia el veredicto a un spec y suma los gates spec_lint,
                  spec_trace y spec_approved
  --json          Emite el veredicto como JSON en stdout (para agentes)

'hoom verify' no tiene subcomandos ni argumentos posicionales.
Accion: para leer veredictos ya emitidos usa 'hoom report'; para el estado
del proyecto, 'hoom status'.`

// sinteticos: los gates que gobiernan --spec y --full, jamas seleccionables
// por --gate.
var sinteticos = []string{"spec_lint", "spec_trace", "spec_approved", "ratchet"}

func lineasUtiles(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		out = append(out, strings.TrimRight(l, " \t"))
	}
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

// huellaDir devuelve ruta -> sha256 de cada archivo del arbol (sin .git). Es
// la forma de exigir "cero efectos en disco" sin enumerar a mano que archivos
// podria escribir una corrida.
func huellaDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			out[rel+string(filepath.Separator)] = "<dir>"
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func exigeMismoArbol(t *testing.T, ca string, antes, despues map[string]string) {
	t.Helper()
	for k, v := range despues {
		prev, ok := antes[k]
		if !ok {
			t.Fatalf("%s: aparecio %q; el pedido rechazado no puede dejar rastro", ca, k)
		}
		if prev != v {
			t.Fatalf("%s: cambio el contenido de %q; el pedido rechazado no puede tocar nada", ca, k)
		}
	}
	for k := range antes {
		if _, ok := despues[k]; !ok {
			t.Fatalf("%s: desaparecio %q", ca, k)
		}
	}
}

// sembrarNarracion deja un verify-live.jsonl con bytes centinela: si el pedido
// rechazado abre el escritor de eventos vivos, el archivo se trunca y el
// centinela desaparece. Es el bug de origen (pisar la narracion de una corrida
// legitima en curso) hecho aserto.
func sembrarNarracion(t *testing.T, dir string) []byte {
	t.Helper()
	cache := filepath.Join(dir, ".hoom", "cache")
	if err := os.MkdirAll(cache, 0o755); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"schema":"hoom/live/v1","kind":"verify_start","nota":"corrida legitima en curso"}` + "\n")
	if err := os.WriteFile(filepath.Join(cache, live.FileName), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return raw
}

func narracion(t *testing.T, dir string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, ".hoom", "cache", live.FileName))
	if err != nil {
		t.Fatalf("no pude leer la narracion: %v", err)
	}
	return raw
}

func proyectoConGates(t *testing.T, gatesYAML string) *manifest.Manifest {
	t.Helper()
	dir := t.TempDir()
	body := "schema: hoom/v1\nproject: demo\ngates:\n" + gatesYAML
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := manifest.Load(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func enDirectorio(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(prev) })
}

func conjunto(xs []string) []string {
	visto := map[string]bool{}
	var out []string
	for _, x := range xs {
		if !visto[x] {
			visto[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}

func gatesEjecutados(v *verdict.Verdict) []string {
	var out []string
	for _, g := range v.Gates {
		if g.Status != verdict.StatusSkipped {
			out = append(out, g.Name)
		}
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// Etapa sintactica: ParseArgs
// ---------------------------------------------------------------------------

// CA-217: `verify show <id>` y toda su familia salen 2. El mensaje nombra el
// PRIMER token no reconocido, tanto si va antes como si va despues de un flag
// valido.
func TestCA217_ParseArgsRechazaPosicionales(t *testing.T) {
	casos := []struct {
		args   []string
		primer string
	}{
		{[]string{"show", "abc123"}, "show"},         // el pedido que origino el spec
		{[]string{"."}, "."},                         // calco de `hoom init .`
		{[]string{"show", "--full"}, "show"},         // el token primero
		{[]string{"--full", "show"}, "show"},         // el token despues: mismo mensaje
		{[]string{"--", "show"}, "show"},             // '--' no habilita operandos
		{[]string{"--full", "ayer"}, "ayer"},         // el tercer caso del spec
		{[]string{"--gate", "test", "show"}, "show"}, // el valor de --gate no es posicional
		{[]string{"show", "--help"}, "show"},         // pedir ayuda no rescata un pedido roto
		{[]string{"report"}, "report"},               // el nombre de OTRO verbo tampoco vale
		{[]string{""}, ""},                           // argumento vacio: sigue siendo argumento
		{[]string{"ñandú"}, "ñandú"},                 // unicode
		{[]string{"🚀", "show"}, "🚀"},                 // fuera del BMP
	}
	for _, c := range casos {
		_, err := ParseArgs(c.args)
		ue := comoUso(t, "CA-217", err)
		if ue.Verb != "verify" {
			t.Fatalf("CA-217 %v: el error nombra su verbo, Verb=%q", c.args, ue.Verb)
		}
		quiero := "hoom verify: argumento posicional no reconocido: " + strconv.Quote(c.primer)
		if got := primeraLinea(ue.Error()); got != quiero {
			t.Fatalf("CA-217 %v: primera linea\n  quiero: %s\n  tengo:  %s", c.args, quiero, got)
		}
	}
}

// CA-217: ParseArgs es PURO. Con un proyecto entero abajo (manifiesto,
// narracion de una corrida en curso, veredictos viejos) y el cwd adentro, ni
// el pedido valido ni el rechazado mueven un solo byte.
func TestCA217_ParseArgsNoTocaElDisco(t *testing.T) {
	m := loadProject(t)
	sembrarNarracion(t, m.Dir)
	if err := os.MkdirAll(filepath.Join(m.Dir, ".hoom", "verdicts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(m.Dir, ".hoom", "verdicts", "viejo.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	antes := huellaDir(t, m.Dir)

	enDirectorio(t, m.Dir)
	for _, args := range [][]string{
		{"show", "abc123"}, {"."}, {"--", "show"}, {"--bogus"}, {"--gate"},
		{"--gate", ","}, {"--spec", ""}, {}, {"--full"}, {"--gate", "test"},
		{"--spec", "no-existe.md"}, {"--json"}, {"--help"},
	} {
		_, _ = ParseArgs(args)
	}
	exigeMismoArbol(t, "CA-217", antes, huellaDir(t, m.Dir))
}

// CA-217 (propiedad): no hay token afortunado. Para cualquier posicional, el
// rechazo es total, con exit 2 y el token citado.
func TestCA217_PropiedadNingunPosicionalPasa(t *testing.T) {
	f := func(tok tokenHostil) bool {
		_, err := ParseArgs([]string{string(tok)})
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) || ue.ExitCode() != 2 {
			t.Logf("CA-217: %q no fue rechazado con exit 2: %v", string(tok), err)
			return false
		}
		if !strings.Contains(ue.Error(), strconv.Quote(string(tok))) {
			t.Logf("CA-217: el mensaje no cita %q: %s", string(tok), ue.Error())
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("CA-217: %v", err)
	}
}

// CA-218: el rechazo trae el bloque verifycmd.UsageText COMPLETO, incluida la
// linea que declara que verify no tiene subcomandos. Y UsageText es, linea por
// linea, el texto que fija el spec: es la fuente unica, no una parafrasis.
func TestCA218_ElRechazoTraeElUsageTextCompleto(t *testing.T) {
	quiero, tengo := lineasUtiles(usoEsperado), lineasUtiles(UsageText)
	if len(quiero) != len(tengo) {
		t.Fatalf("CA-218: UsageText tiene %d lineas utiles y el spec fija %d:\n--- spec ---\n%s\n--- UsageText ---\n%s",
			len(tengo), len(quiero), usoEsperado, UsageText)
	}
	for i := range quiero {
		if quiero[i] != tengo[i] {
			t.Fatalf("CA-218: UsageText difiere del spec en la linea %d\n  quiero: %q\n  tengo:  %q", i+1, quiero[i], tengo[i])
		}
	}
	for _, r := range UsageText {
		if r > 127 {
			t.Fatalf("CA-218: el uso vive en Go sin acentos; aparecio %q en UsageText", r)
		}
	}

	_, err := ParseArgs([]string{"show", "abc123"})
	ue := comoUso(t, "CA-218", err)
	if ue.Usage != UsageText {
		t.Fatalf("CA-218: el UsageError debe llevar UsageText sin tocar:\n--- tengo ---\n%s", ue.Usage)
	}
	msg := ue.Error()
	if !strings.Contains(msg, strconv.Quote("show")) {
		t.Fatalf("CA-218: el rechazo debe traer el token ofensor:\n%s", msg)
	}
	if !strings.Contains(msg, "'hoom verify' no tiene subcomandos ni argumentos posicionales.") {
		t.Fatalf("CA-218: falta la linea que declara que verify no tiene subcomandos:\n%s", msg)
	}
	if !strings.Contains(msg, "hoom report") || !strings.Contains(msg, "hoom status") {
		t.Fatalf("CA-218: la accion debe mandar a report y a status:\n%s", msg)
	}
	exigeBloque(t, "CA-218", msg, UsageText)
}

// CA-219: flag no definido, en cualquier dialecto, sale 2 con el bloque de
// hoom; la cadena "Usage of verify:" del paquete flag no aparece jamas.
func TestCA219_ParseArgsFlagNoDefinido(t *testing.T) {
	_, err := ParseArgs([]string{"--bogus"})
	ue := comoUso(t, "CA-219", err)
	if got, quiero := primeraLinea(ue.Error()), "hoom verify: flag desconocido: --bogus"; got != quiero {
		t.Fatalf("CA-219: primera linea\n  quiero: %s\n  tengo:  %s", quiero, got)
	}

	for _, args := range [][]string{
		{"-bogus"}, {"--bogus"}, {"--bogus=1"}, {"--Full"}, {"--GATE", "test"},
		{"--full", "--bogus"}, {"--gates", "test"}, {"--espec", "x.md"},
	} {
		_, err := ParseArgs(args)
		ue := comoUso(t, "CA-219", err)
		msg := ue.Error()
		if !strings.HasPrefix(msg, "hoom verify: flag desconocido: ") {
			t.Fatalf("CA-219 %v: esperaba la razon 'flag desconocido', fue %q", args, primeraLinea(msg))
		}
		for _, ajeno := range []string{"Usage of verify:", "Usage of ", "flag provided but not defined"} {
			if strings.Contains(msg, ajeno) {
				t.Fatalf("CA-219 %v: aparecio el texto del paquete flag %q:\n%s", args, ajeno, msg)
			}
		}
		exigeBloque(t, "CA-219", msg, UsageText)
	}
}

// CA-220: un flag definido sin valor al final de la linea sale 2 con el
// mensaje de hoom, no con el del paquete flag.
func TestCA220_ParseArgsFlagDefinidoSinValor(t *testing.T) {
	_, err := ParseArgs([]string{"--gate"})
	ue := comoUso(t, "CA-220", err)
	if got, quiero := primeraLinea(ue.Error()), "hoom verify: --gate necesita un valor"; got != quiero {
		t.Fatalf("CA-220: primera linea\n  quiero: %s\n  tengo:  %s", quiero, got)
	}

	for _, c := range []struct {
		args []string
		flag string
	}{
		{[]string{"--gate"}, "--gate"},
		{[]string{"-gate"}, "--gate"},
		{[]string{"--full", "--gate"}, "--gate"},
		{[]string{"--spec"}, "--spec"},
		{[]string{"--full", "--json", "--spec"}, "--spec"},
	} {
		_, err := ParseArgs(c.args)
		ue := comoUso(t, "CA-220", err)
		linea := primeraLinea(ue.Error())
		if !strings.Contains(linea, c.flag) || !strings.Contains(linea, "necesita un valor") {
			t.Fatalf("CA-220 %v: esperaba %q sin valor, fue %q", c.args, c.flag, linea)
		}
		if strings.Contains(ue.Error(), "flag needs an argument") {
			t.Fatalf("CA-220 %v: filtro el mensaje del paquete flag:\n%s", c.args, ue.Error())
		}
		exigeBloque(t, "CA-220", ue.Error(), UsageText)
	}
}

// CA-221: escribir un flag y no darle valor —o darle uno que no selecciona
// nada— es un typo, no el default. En particular `--gate ","` no vuelve nunca
// a producir una corrida completa de referencia: no produce corrida alguna.
func TestCA221_ValorVacioOSeleccionVacia(t *testing.T) {
	m := loadProject(t)
	sembrarNarracion(t, m.Dir)
	antes := huellaDir(t, m.Dir)
	enDirectorio(t, m.Dir)

	// La seleccion que resuelve a CERO nombres tiene su linea exacta.
	for _, args := range [][]string{
		{"--gate", ","}, {"--gate", " , "}, {"--gate", ",,,"}, {"--gate=,"},
		{"--gate", "  "}, {"--gate", "\t"},
	} {
		_, err := ParseArgs(args)
		ue := comoUso(t, "CA-221", err)
		if got, quiero := primeraLinea(ue.Error()), "hoom verify: --gate vacio: no selecciona ningun gate"; got != quiero {
			t.Fatalf("CA-221 %v: primera linea\n  quiero: %s\n  tengo:  %s", args, quiero, got)
		}
		exigeBloque(t, "CA-221", ue.Error(), UsageText)
	}

	// Un flag escrito con valor vacio: rechazo nombrando el flag.
	for _, c := range []struct {
		args []string
		flag string
	}{
		{[]string{"--gate", ""}, "--gate"},
		{[]string{"--gate="}, "--gate"},
		{[]string{"--spec", ""}, "--spec"},
		{[]string{"--spec="}, "--spec"},
		{[]string{"--full", "--spec", "", "--json"}, "--spec"},
	} {
		_, err := ParseArgs(c.args)
		ue := comoUso(t, "CA-221", err)
		if !strings.HasPrefix(primeraLinea(ue.Error()), "hoom verify: "+c.flag+" ") {
			t.Fatalf("CA-221 %v: el rechazo debe nombrar %q, fue %q", c.args, c.flag, primeraLinea(ue.Error()))
		}
		exigeBloque(t, "CA-221", ue.Error(), UsageText)
	}

	// Ninguno de esos rechazos toco el disco: no hubo corrida de referencia.
	exigeMismoArbol(t, "CA-221", antes, huellaDir(t, m.Dir))

	// Control: con al menos un nombre valido, comas sobrantes y duplicados se
	// aceptan (seleccion inequivoca) y llegan normalizados.
	for _, c := range []struct {
		valor  string
		quiero []string
	}{
		{"test", []string{"test"}},
		{"test,", []string{"test"}},
		{",test", []string{"test"}},
		{"test,test", []string{"test"}},
		{" test , build ", []string{"build", "test"}},
		{"test,,build,", []string{"build", "test"}},
	} {
		opt, err := ParseArgs([]string{"--gate", c.valor})
		if err != nil {
			t.Fatalf("CA-221: --gate %q es inequivoco y fue rechazado: %v", c.valor, err)
		}
		for _, g := range opt.Gates {
			if g == "" || g != strings.TrimSpace(g) {
				t.Fatalf("CA-221: --gate %q dejo un nombre sin normalizar: %q", c.valor, g)
			}
		}
		if got := conjunto(opt.Gates); !reflect.DeepEqual(got, c.quiero) {
			t.Fatalf("CA-221: --gate %q selecciona %v, esperaba %v", c.valor, got, c.quiero)
		}
	}
}

// ---------------------------------------------------------------------------
// Etapa semantica: Run contra el manifiesto
// ---------------------------------------------------------------------------

// CA-222: un nombre de gate que el proyecto no declara se rechaza NOMBRANDOLO
// y listando los gates de este proyecto, antes de abrir el escritor de eventos
// vivos y sin escribir veredicto.
func TestCA222_GateDesconocidoMuereAntesDeNarrar(t *testing.T) {
	m := loadProject(t) // declara: build, test
	centinela := sembrarNarracion(t, m.Dir)
	antes := huellaDir(t, m.Dir)

	v, path, err := Run(m, Options{Gates: []string{"noexiste"}})
	ue := comoUso(t, "CA-222", err)
	if v != nil || path != "" {
		t.Fatalf("CA-222: un pedido rechazado no produce veredicto: v=%+v path=%q", v, path)
	}

	linea := primeraLinea(ue.Error())
	prefijo := `hoom verify: gate desconocido "noexiste"; este proyecto declara: `
	if !strings.HasPrefix(linea, prefijo) {
		t.Fatalf("CA-222: primera linea\n  quiero prefijo: %s\n  tengo:          %s", prefijo, linea)
	}
	var listados []string
	for _, n := range strings.Split(strings.TrimPrefix(linea, prefijo), ",") {
		if n = strings.TrimSpace(n); n != "" {
			listados = append(listados, n)
		}
	}
	var declarados []string
	for n := range m.Gates {
		declarados = append(declarados, n)
	}
	if !reflect.DeepEqual(conjunto(listados), conjunto(declarados)) {
		t.Fatalf("CA-222: el mensaje debe listar los gates del proyecto %v, listo %v", conjunto(declarados), conjunto(listados))
	}
	for _, s := range sinteticos {
		for _, l := range listados {
			if l == s {
				t.Fatalf("CA-222: %q es sintetico y no puede aparecer como seleccionable: %q", s, linea)
			}
		}
	}
	exigeBloque(t, "CA-222", ue.Error(), UsageText)

	// La narracion en curso quedo byte por byte igual: no se abrio el escritor.
	if got := narracion(t, m.Dir); string(got) != string(centinela) {
		t.Fatalf("CA-222: la narracion fue tocada\n  antes: %q\n  ahora: %q", centinela, got)
	}
	exigeMismoArbol(t, "CA-222", antes, huellaDir(t, m.Dir))
}

// CA-222: los gates sinteticos no son seleccionables por --gate NUNCA, ni
// siquiera en la corrida donde existirian (--full para ratchet, --spec para
// los de spec). Cambio de conducta consciente: hoy no seleccionan nada y dejan
// todo en 'skipped'.
func TestCA222_GatesSinteticosNoSonSeleccionables(t *testing.T) {
	m := loadProject(t)
	spec := writeSpecFile(t, m.Dir)
	antes := huellaDir(t, m.Dir)

	casos := []Options{
		{Gates: []string{"spec_lint"}},
		{Gates: []string{"ratchet"}},
		{Gates: []string{"spec_trace"}},
		{Gates: []string{"spec_approved"}},
		{Gates: []string{"ratchet"}, Full: true},
		{Gates: []string{"spec_lint"}, Spec: spec},
		{Gates: []string{"test", "spec_lint"}}, // un nombre malo envenena la seleccion
		{Gates: []string{"test", "noexiste"}},
		{Gates: []string{""}},     // nombre vacio: tampoco existe
		{Gates: []string{"TEST"}}, // los nombres son exactos, no case-insensitive
	}
	for _, opt := range casos {
		v, path, err := Run(m, opt)
		ue := comoUso(t, "CA-222", err)
		if v != nil || path != "" {
			t.Fatalf("CA-222 %+v: no puede haber veredicto: v=%+v path=%q", opt, v, path)
		}
		if !strings.HasPrefix(primeraLinea(ue.Error()), "hoom verify: gate desconocido ") {
			t.Fatalf("CA-222 %+v: esperaba 'gate desconocido', fue %q", opt, primeraLinea(ue.Error()))
		}
	}
	exigeMismoArbol(t, "CA-222", antes, huellaDir(t, m.Dir))

	// Control: el gate real del proyecto sigue seleccionandose.
	if _, _, err := Run(m, Options{Gates: []string{"test"}}); err != nil {
		t.Fatalf("CA-222: 'test' esta declarado y debe correr: %v", err)
	}
}

// CA-225: lo valido sigue valido. Cada forma escrita produce exactamente las
// mismas Options, y el veredicto resultante es el mismo que el de llamar a Run
// con las Options a mano.
func TestCA225_LoValidoSigueValido(t *testing.T) {
	casos := []struct {
		args   []string
		quiero Options
	}{
		{[]string{}, Options{}},
		{[]string{"--"}, Options{}}, // terminador solo: NArg 0, corrida normal
		{[]string{"--full"}, Options{Full: true}},
		{[]string{"-full"}, Options{Full: true}}, // dialecto de Go: un guion
		{[]string{"--full=true"}, Options{Full: true}},
		{[]string{"--gate", "test"}, Options{Gates: []string{"test"}}},
		{[]string{"--gate=test"}, Options{Gates: []string{"test"}}},
		{[]string{"-gate", "test"}, Options{Gates: []string{"test"}}},
		{[]string{"--spec", "x.md"}, Options{Spec: "x.md"}},
		{[]string{"--spec=x.md"}, Options{Spec: "x.md"}},
		{[]string{"--spec", "no-existe.md"}, Options{Spec: "no-existe.md"}}, // la ruta no se valida al parsear
		{[]string{"--full", "--gate", "test", "--spec", "x.md"},
			Options{Full: true, Gates: []string{"test"}, Spec: "x.md"}},
	}
	for _, c := range casos {
		opt, err := ParseArgs(c.args)
		if err != nil {
			t.Fatalf("CA-225: %v es valido y fue rechazado: %v", c.args, err)
		}
		if opt.Full != c.quiero.Full || opt.Spec != c.quiero.Spec {
			t.Fatalf("CA-225: %v dio Full=%v Spec=%q, esperaba Full=%v Spec=%q",
				c.args, opt.Full, opt.Spec, c.quiero.Full, c.quiero.Spec)
		}
		if !reflect.DeepEqual(conjunto(opt.Gates), conjunto(c.quiero.Gates)) {
			t.Fatalf("CA-225: %v selecciona %v, esperaba %v", c.args, conjunto(opt.Gates), conjunto(c.quiero.Gates))
		}
	}

	// --json parsea sin quejarse y no ensucia el resto de las Options.
	opt, err := ParseArgs([]string{"--json", "--full", "--gate", "test"})
	if err != nil {
		t.Fatalf("CA-225: --json es valido: %v", err)
	}
	if !opt.Full || !reflect.DeepEqual(conjunto(opt.Gates), []string{"test"}) {
		t.Fatalf("CA-225: --json no puede alterar el resto: %+v", opt)
	}

	// Y el veredicto es el mismo que el de las Options armadas a mano.
	m := loadProject(t)
	parseadas, err := ParseArgs([]string{"--gate", "test"})
	if err != nil {
		t.Fatal(err)
	}
	viaArgs, path, err := Run(m, parseadas)
	if err != nil || path == "" {
		t.Fatalf("CA-225: la corrida valida escribe su veredicto: err=%v path=%q", err, path)
	}
	viaMano, _, err := Run(m, Options{Gates: []string{"test"}})
	if err != nil {
		t.Fatal(err)
	}
	if viaArgs.Verdict != viaMano.Verdict || viaArgs.Partial != viaMano.Partial {
		t.Fatalf("CA-225: mismo pedido, distinto veredicto: %+v vs %+v", viaArgs.Summary, viaMano.Summary)
	}
	if !reflect.DeepEqual(gatesEjecutados(viaArgs), gatesEjecutados(viaMano)) {
		t.Fatalf("CA-225: mismo pedido, distintos gates: %v vs %v", gatesEjecutados(viaArgs), gatesEjecutados(viaMano))
	}

	// Un duplicado no corre el gate dos veces: la seleccion es inequivoca.
	dup, err := ParseArgs([]string{"--gate", "test,test"})
	if err != nil {
		t.Fatal(err)
	}
	v, _, err := Run(m, dup)
	if err != nil {
		t.Fatal(err)
	}
	vistos := map[string]int{}
	for _, g := range v.Gates {
		vistos[g.Name]++
	}
	if vistos["test"] != 1 {
		t.Fatalf("CA-225: --gate test,test debe dejar UN solo resultado de 'test', dejo %d: %+v", vistos["test"], v.Gates)
	}
}

// seleccionRuidosa: un multiconjunto de gates reales, renderizado con comas de
// mas, segmentos vacios y espacios alrededor.
type seleccionRuidosa struct {
	nombres []string
	texto   string
}

func (seleccionRuidosa) Generate(rnd *rand.Rand, size int) reflect.Value {
	base := []string{"test", "build"}
	pads := []string{"", " ", "  ", "\t"}
	k := 1 + rnd.Intn(4)
	var nombres, partes []string
	for i := 0; i < k; i++ {
		n := base[rnd.Intn(len(base))]
		nombres = append(nombres, n)
		partes = append(partes, pads[rnd.Intn(len(pads))]+n+pads[rnd.Intn(len(pads))])
		if rnd.Intn(2) == 0 {
			partes = append(partes, pads[rnd.Intn(len(pads))]) // segmento vacio
		}
	}
	texto := strings.Join(partes, ",")
	if rnd.Intn(2) == 0 {
		texto += ","
	}
	return reflect.ValueOf(seleccionRuidosa{nombres: nombres, texto: texto})
}

// CA-225 (propiedad): mientras quede al menos un nombre, el ruido no cambia la
// seleccion. Es la otra cara de CA-221: cero nombres se rechaza, nombres con
// ruido se aceptan y resuelven a lo mismo.
func TestCA225_PropiedadElRuidoNoCambiaLaSeleccion(t *testing.T) {
	f := func(s seleccionRuidosa) bool {
		opt, err := ParseArgs([]string{"--gate", s.texto})
		if err != nil {
			t.Logf("CA-225: --gate %q tiene nombres validos y fue rechazado: %v", s.texto, err)
			return false
		}
		for _, g := range opt.Gates {
			if g == "" || g != strings.TrimSpace(g) {
				t.Logf("CA-225: --gate %q dejo %q sin normalizar", s.texto, g)
				return false
			}
		}
		if !reflect.DeepEqual(conjunto(opt.Gates), conjunto(s.nombres)) {
			t.Logf("CA-225: --gate %q selecciona %v, esperaba %v", s.texto, conjunto(opt.Gates), conjunto(s.nombres))
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 200}); err != nil {
		t.Fatalf("CA-225: %v", err)
	}
}

// CA-227: los dos caminos, en el mismo test y sobre el mismo proyecto. Pedido
// no entendido: error de uso con exit 2 y CERO artefactos. Pedido entendido
// que sale mal: veredicto ROJO escrito en disco (que main traduce a exit 1).
// El 2 no pisa al 1 ni al reves.
func TestCA227_DosCaminosDosCodigos(t *testing.T) {
	m := proyectoConGates(t, "  test:\n    required: true\n    cmd: \"false\"\n")
	antes := huellaDir(t, m.Dir)

	// Camino A: no entendi el pedido.
	v, path, err := Run(m, Options{Gates: []string{"noexiste"}})
	ue := comoUso(t, "CA-227", err)
	if ue.ExitCode() != 2 {
		t.Fatalf("CA-227: argumento no entendido = 2, fue %d", ue.ExitCode())
	}
	if v != nil || path != "" {
		t.Fatalf("CA-227: el camino del 2 no escribe evidencia: v=%+v path=%q", v, path)
	}
	exigeMismoArbol(t, "CA-227", antes, huellaDir(t, m.Dir))

	// Camino B: entendi el pedido y salio rojo.
	var ueB *cliargs.UsageError
	v, path, err = Run(m, Options{Full: true})
	if err != nil {
		t.Fatalf("CA-227: un rojo legitimo no es un error de uso: %v", err)
	}
	if errors.As(err, &ueB) {
		t.Fatalf("CA-227: un rojo legitimo nunca es UsageError")
	}
	if v == nil || v.Verdict != "red" {
		t.Fatalf("CA-227: un gate requerido que falla da ROJO: %+v", v)
	}
	if path == "" {
		t.Fatalf("CA-227: el camino del 1 SI escribe el veredicto")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("CA-227: el veredicto debe existir en disco: %v", err)
	}

	// Y el 2 no pisa al 1: repetir el pedido mal escrito no borra la evidencia.
	conRojo := huellaDir(t, m.Dir)
	if _, _, err := Run(m, Options{Gates: []string{"noexiste"}}); !errors.As(err, &ueB) {
		t.Fatalf("CA-227: seguia siendo un error de uso: %v", err)
	}
	exigeMismoArbol(t, "CA-227", conRojo, huellaDir(t, m.Dir))
}
