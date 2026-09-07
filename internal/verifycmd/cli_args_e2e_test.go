// Tests adversariales del spec .hoom/specs/verify-args-estrictos.md
// (CA-217..CA-220, CA-224, CA-225, CA-227) contra el BINARIO. El mapeo
// UsageError -> stderr + os.Exit(2) vive en main: a nivel paquete no se
// observa cual stream se usa ni con que codigo muere el proceso, y ese es
// justamente el cableado que el spec fija.
package verifycmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/live"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func moduloRaiz(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		padre := filepath.Dir(dir)
		if padre == dir {
			t.Fatal("no encontre go.mod hacia arriba: hace falta el modulo para construir ./cmd/hoom")
		}
		dir = padre
	}
}

func construirHoom(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "hoom")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/hoom")
	cmd.Dir = moduloRaiz(t)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go build ./cmd/hoom fallo: %v\n%s", err, out)
	}
	return bin
}

type salidaCLI struct {
	exit           int
	stdout, stderr string
}

func correrHoom(t *testing.T, bin, dir string, args ...string) salidaCLI {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	if cmd.ProcessState == nil {
		t.Fatalf("no pude ejecutar hoom %v: %v", args, err)
	}
	return salidaCLI{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

func proyectoCLI(t *testing.T, cmdGate string) string {
	t.Helper()
	dir := t.TempDir()
	body := "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: " +
		strconv.Quote(cmdGate) + "\n"
	if err := os.WriteFile(filepath.Join(dir, manifest.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"config", "user.email", "test@hoom.dev"},
		{"config", "user.name", "hoom test"},
		{"add", "-A"},
		{"commit", "-m", "inicial"},
	} {
		c := exec.Command("git", args...)
		c.Dir = dir
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func cuentaVeredictos(t *testing.T, dir string) int {
	t.Helper()
	entradas, err := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts"))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return len(entradas)
}

// CA-217: `hoom verify show <id>` sale 2 y NO DEJA RASTRO — ningun archivo
// nuevo en .hoom/verdicts/ y verify-live.jsonl sin crear ni truncar: si
// existia de una corrida previa, queda byte por byte igual.
func TestCA217_CLIShowNoDejaRastro(t *testing.T) {
	bin := construirHoom(t)
	dir := proyectoCLI(t, "true")

	// Primero una corrida legitima: deja veredicto y narracion en disco.
	if res := correrHoom(t, bin, dir, "verify"); res.exit != 0 {
		t.Fatalf("CA-217: la corrida legitima debe salir 0, salio %d\nstderr: %s", res.exit, res.stderr)
	}
	rutaLive := filepath.Join(dir, ".hoom", "cache", live.FileName)
	antesLive, err := os.ReadFile(rutaLive)
	if err != nil {
		t.Fatalf("CA-217: la corrida legitima debia narrar: %v", err)
	}
	antes := huellaDir(t, dir)

	res := correrHoom(t, bin, dir, "verify", "show", "abc123")
	if res.exit != 2 {
		t.Fatalf("CA-217: esperaba exit 2, salio %d\nstdout: %s\nstderr: %s", res.exit, res.stdout, res.stderr)
	}
	exigeMismoArbol(t, "CA-217", antes, huellaDir(t, dir))
	despuesLive, err := os.ReadFile(rutaLive)
	if err != nil {
		t.Fatalf("CA-217: la narracion previa no puede desaparecer: %v", err)
	}
	if !bytes.Equal(antesLive, despuesLive) {
		t.Fatalf("CA-217: verify-live.jsonl fue truncado o reescrito\n  antes: %q\n  ahora: %q", antesLive, despuesLive)
	}

	// En un proyecto virgen ni siquiera se crea el archivo de narracion.
	virgen := proyectoCLI(t, "true")
	if res := correrHoom(t, bin, virgen, "verify", "show", "abc123"); res.exit != 2 {
		t.Fatalf("CA-217: esperaba exit 2 en proyecto virgen, salio %d\nstderr: %s", res.exit, res.stderr)
	}
	if _, err := os.Stat(filepath.Join(virgen, ".hoom", "cache", live.FileName)); !os.IsNotExist(err) {
		t.Fatalf("CA-217: el rechazo no puede crear verify-live.jsonl (err=%v)", err)
	}
	if n := cuentaVeredictos(t, virgen); n != 0 {
		t.Fatalf("CA-217: el rechazo no puede dejar veredictos, dejo %d", n)
	}

	// Sin hoom.yaml gana el error de ARGUMENTO, no el de manifiesto: cmdVerify
	// contesta antes de leer el proyecto.
	pelado := t.TempDir()
	sin := correrHoom(t, bin, pelado, "verify", "show", "x")
	if sin.exit != 2 {
		t.Fatalf("CA-217: sin hoom.yaml el error de argumento igual sale 2, salio %d\nstderr: %s", sin.exit, sin.stderr)
	}
	if !strings.Contains(sin.stderr, "argumento posicional no reconocido") {
		t.Fatalf("CA-217: debe ganar el error de argumento:\n%s", sin.stderr)
	}
	if strings.Contains(sin.stderr, manifest.FileName) {
		t.Fatalf("CA-217: no puede ganar el error de manifiesto:\n%s", sin.stderr)
	}
}

// CA-218: el rechazo va por STDERR con el token ofensor y el bloque UsageText
// completo. stdout queda limpio: un agente que parsea stdout no puede comerse
// prosa de error.
func TestCA218_CLIElRechazoVaPorStderr(t *testing.T) {
	bin := construirHoom(t)
	dir := proyectoCLI(t, "true")

	for _, c := range []struct {
		args   []string
		primer string
	}{
		{[]string{"verify", "show", "abc123"}, "show"},
		{[]string{"verify", "show", "--full"}, "show"},
		{[]string{"verify", "--full", "show"}, "show"},
		{[]string{"verify", "--", "show"}, "show"},
		{[]string{"verify", "."}, "."},
	} {
		res := correrHoom(t, bin, dir, c.args...)
		if res.exit != 2 {
			t.Fatalf("CA-218 %v: esperaba exit 2, salio %d\nstderr: %s", c.args, res.exit, res.stderr)
		}
		if strings.TrimSpace(res.stdout) != "" {
			t.Fatalf("CA-218 %v: el rechazo no puede ensuciar stdout:\n%s", c.args, res.stdout)
		}
		if !strings.Contains(res.stderr, strconv.Quote(c.primer)) {
			t.Fatalf("CA-218 %v: stderr debe traer el token %q:\n%s", c.args, c.primer, res.stderr)
		}
		if !strings.Contains(res.stderr, "'hoom verify' no tiene subcomandos ni argumentos posicionales.") {
			t.Fatalf("CA-218 %v: falta la linea que declara que verify no tiene subcomandos:\n%s", c.args, res.stderr)
		}
		for _, l := range lineasUtiles(UsageText) {
			if !strings.Contains(res.stderr, l) {
				t.Fatalf("CA-218 %v: a stderr le falta la linea del uso %q:\n%s", c.args, l, res.stderr)
			}
		}
	}
}

// CA-219: un flag no definido sale 2 con el bloque de hoom; "Usage of verify:"
// no aparece jamas en ninguna de las dos salidas.
func TestCA219_CLIFlagNoDefinido(t *testing.T) {
	bin := construirHoom(t)
	dir := proyectoCLI(t, "true")
	for _, args := range [][]string{
		{"verify", "--bogus"}, {"verify", "-bogus"}, {"verify", "--bogus=1"},
		{"verify", "--Full"}, {"verify", "--full", "--bogus"},
	} {
		res := correrHoom(t, bin, dir, args...)
		if res.exit != 2 {
			t.Fatalf("CA-219 %v: esperaba exit 2, salio %d\nstderr: %s", args, res.exit, res.stderr)
		}
		todo := res.stdout + res.stderr
		for _, ajeno := range []string{"Usage of verify:", "Usage of ", "flag provided but not defined"} {
			if strings.Contains(todo, ajeno) {
				t.Fatalf("CA-219 %v: aparecio el texto del paquete flag %q:\n%s", args, ajeno, todo)
			}
		}
		if !strings.Contains(res.stderr, "flag desconocido") {
			t.Fatalf("CA-219 %v: falta la razon de hoom:\n%s", args, res.stderr)
		}
		for _, l := range lineasUtiles(UsageText) {
			if !strings.Contains(res.stderr, l) {
				t.Fatalf("CA-219 %v: a stderr le falta la linea del uso %q:\n%s", args, l, res.stderr)
			}
		}
	}
}

// CA-220: `hoom verify --gate` al final de la linea sale 2 con el mensaje de
// hoom, no con el del paquete flag.
func TestCA220_CLIFlagSinValor(t *testing.T) {
	bin := construirHoom(t)
	dir := proyectoCLI(t, "true")
	for _, c := range []struct {
		args []string
		flag string
	}{
		{[]string{"verify", "--gate"}, "--gate"},
		{[]string{"verify", "--full", "--gate"}, "--gate"},
		{[]string{"verify", "--spec"}, "--spec"},
	} {
		res := correrHoom(t, bin, dir, c.args...)
		if res.exit != 2 {
			t.Fatalf("CA-220 %v: esperaba exit 2, salio %d\nstderr: %s", c.args, res.exit, res.stderr)
		}
		if !strings.Contains(res.stderr, c.flag) || !strings.Contains(res.stderr, "necesita un valor") {
			t.Fatalf("CA-220 %v: esperaba el mensaje de hoom:\n%s", c.args, res.stderr)
		}
		if strings.Contains(res.stderr, "flag needs an argument") {
			t.Fatalf("CA-220 %v: filtro el mensaje del paquete flag:\n%s", c.args, res.stderr)
		}
	}
}

// CA-224: --help y -h imprimen el uso exacto por STDOUT, salen 0 y no dejan
// artefactos. Y no necesitan proyecto: ParseArgs corre antes de manifest.Load.
func TestCA224_CLIHelpPorStdoutSinArtefactos(t *testing.T) {
	bin := construirHoom(t)
	dir := proyectoCLI(t, "true")
	antes := huellaDir(t, dir)

	for _, args := range [][]string{{"verify", "--help"}, {"verify", "-h"}} {
		res := correrHoom(t, bin, dir, args...)
		if res.exit != 0 {
			t.Fatalf("CA-224 %v: pedir ayuda no es un error, salio %d\nstderr: %s", args, res.exit, res.stderr)
		}
		for _, l := range lineasUtiles(UsageText) {
			if !strings.Contains(res.stdout, l) {
				t.Fatalf("CA-224 %v: a stdout le falta la linea del uso %q:\n%s", args, l, res.stdout)
			}
		}
		if strings.TrimSpace(res.stderr) != "" {
			t.Fatalf("CA-224 %v: la ayuda va por stdout; stderr debe quedar limpio:\n%s", args, res.stderr)
		}
		exigeMismoArbol(t, "CA-224", antes, huellaDir(t, dir))
	}

	pelado := t.TempDir()
	res := correrHoom(t, bin, pelado, "verify", "--help")
	if res.exit != 0 {
		t.Fatalf("CA-224: --help sin hoom.yaml debe salir 0, salio %d\nstderr: %s", res.exit, res.stderr)
	}
	if !strings.Contains(res.stdout, "no tiene subcomandos") {
		t.Fatalf("CA-224: el uso exacto por stdout tambien sin proyecto:\n%s", res.stdout)
	}
	if n := len(huellaDir(t, pelado)); n != 0 {
		t.Fatalf("CA-224: cero artefactos, aparecieron %d entradas", n)
	}
}

// CA-225: regresion de lo valido. Ninguna forma valida sale 2, y cada una
// escribe su veredicto. El exit se corresponde con el color: verde 0, rojo 1.
func TestCA225_CLILoValidoSigueValido(t *testing.T) {
	bin := construirHoom(t)
	dir := proyectoCLI(t, "true")

	for _, args := range [][]string{
		{"verify"},
		{"verify", "--"}, // terminador solo: corrida normal
		{"verify", "--full"},
		{"verify", "-full"},
		{"verify", "--gate", "test"},
		{"verify", "--gate=test"},
		{"verify", "--gate", "test,test"},
		{"verify", "--gate", "test,"},
	} {
		n := cuentaVeredictos(t, dir)
		res := correrHoom(t, bin, dir, args...)
		if res.exit == 2 {
			t.Fatalf("CA-225: %v es valido y salio 2\nstderr: %s", args, res.stderr)
		}
		if res.exit != 0 {
			t.Fatalf("CA-225: %v sobre gates que pasan debe salir 0, salio %d\nstderr: %s", args, res.exit, res.stderr)
		}
		if got := cuentaVeredictos(t, dir); got != n+1 {
			t.Fatalf("CA-225: %v debe escribir un veredicto (%d -> %d)", args, n, got)
		}
	}

	// --json emite el veredicto por stdout, parseable, con su schema.
	res := correrHoom(t, bin, dir, "verify", "--json")
	if res.exit != 0 {
		t.Fatalf("CA-225: --json sobre verde sale 0, salio %d\nstderr: %s", res.exit, res.stderr)
	}
	var v verdict.Verdict
	if err := json.Unmarshal([]byte(res.stdout), &v); err != nil {
		t.Fatalf("CA-225: --json debe emitir JSON parseable por stdout: %v\n%s", err, res.stdout)
	}
	if v.Schema != verdict.SchemaID {
		t.Fatalf("CA-225: el JSON debe traer el schema %q, trajo %q", verdict.SchemaID, v.Schema)
	}

	// --spec con ruta inexistente: sin cambios, los gates de spec dan ROJO
	// honesto (exit 1), nunca un error de uso.
	rojo := correrHoom(t, bin, dir, "verify", "--spec", "no-existe.md")
	if rojo.exit != 1 {
		t.Fatalf("CA-225: un spec inexistente da rojo (exit 1), salio %d\nstderr: %s", rojo.exit, rojo.stderr)
	}
}

// CA-227: la disciplina de exit codes, con los dos caminos en el MISMO test.
// Argumento no entendido = 2 y cero artefactos. Veredicto rojo con argumentos
// validos = 1 y veredicto escrito. Y el 2 no pisa al 1 nunca.
func TestCA227_CLIDosCodigosDeSalida(t *testing.T) {
	bin := construirHoom(t)
	dir := proyectoCLI(t, "false") // gate requerido que falla siempre

	// Camino A: no entendi el pedido -> 2, cero artefactos.
	antes := huellaDir(t, dir)
	malo := correrHoom(t, bin, dir, "verify", "show", "abc123")
	if malo.exit != 2 {
		t.Fatalf("CA-227: argumento no entendido = 2, salio %d\nstderr: %s", malo.exit, malo.stderr)
	}
	exigeMismoArbol(t, "CA-227", antes, huellaDir(t, dir))
	if n := cuentaVeredictos(t, dir); n != 0 {
		t.Fatalf("CA-227: el camino del 2 no escribe evidencia, escribio %d", n)
	}

	// Camino B: entendi el pedido y salio rojo -> 1, con veredicto escrito.
	rojo := correrHoom(t, bin, dir, "verify", "--full")
	if rojo.exit != 1 {
		t.Fatalf("CA-227: veredicto rojo con argumentos validos = 1, salio %d\nstderr: %s", rojo.exit, rojo.stderr)
	}
	if n := cuentaVeredictos(t, dir); n != 1 {
		t.Fatalf("CA-227: el camino del 1 escribe su veredicto, hay %d", n)
	}

	// El 2 no pisa al 1: otro pedido mal escrito no borra ni cambia la
	// evidencia del rojo.
	conRojo := huellaDir(t, dir)
	otro := correrHoom(t, bin, dir, "verify", ".")
	if otro.exit != 2 {
		t.Fatalf("CA-227: el pedido mal escrito sigue siendo 2, salio %d\nstderr: %s", otro.exit, otro.stderr)
	}
	exigeMismoArbol(t, "CA-227", conRojo, huellaDir(t, dir))
	if n := cuentaVeredictos(t, dir); n != 1 {
		t.Fatalf("CA-227: el 2 no puede tocar el veredicto rojo, quedaron %d", n)
	}
}
