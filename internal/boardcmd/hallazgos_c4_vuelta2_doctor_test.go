// Tests adversariales de la segunda vuelta de la review de C4 sobre el doctor
// (.hoom/specs/historia-doctor-y-cinta.md, pieza 3: CA-367, CA-369). Escritos
// desde el spec y la descripcion del hallazgo, sin leer la implementacion.
// Los helpers de este archivo llevan el prefijo c4v2.
//
//   - 20261016T213329_fffe28 (risk): el rol de un sobre se valida contra los
//     roles de hoom recortando los espacios de alrededor, pero la accion de
//     sobre-huerfano interpola el valor crudo. Un sobre con role "\nwriter"
//     pasa la validacion y, al pegar la accion, parte 'hoom agent --role' de
//     un segundo comando 'writer --task ...'. La accion del contrato nombra
//     el rol de hoom: '--role writer', exacto.
//
// "Pegar la accion" es literal: la accion (desde su 'hoom ') se corre con
// /bin/sh -c (sh POSIX) con un PATH que es SOLO un directorio de falsos: hoom,
// rm y touch, y los nombres que una accion partida ejecutaria (los roles y
// los providers). Cada falso anota su nombre y sus argumentos; touch ademas
// crea sus archivos, y los roles y providers falsos crean PWNED. Nada real
// corre, y cualquier otro comando sale como "not found" en la salida.
package boardcmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// ---------------------------------------------------------------------------
// Arnes: un PATH solo de falsos, y pegar una accion en sh -c.
// ---------------------------------------------------------------------------

// c4v2Ajenos son los comandos que una accion partida ejecutaria: un rol o un
// provider que quedo solo en su linea. Sus falsos crean PWNED.
var c4v2Ajenos = []string{"writer", "reviewer", "test-writer", "arquitecto", "scout",
	"claude", "codex", "opencode", "gemini", "aider"}

type c4v2Falso struct{ bin, log string }

func c4v2Falsos(t *testing.T) c4v2Falso {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	anota := func(n string) string {
		return "#!/bin/sh\n{ printf '%s\\n' '== " + n + "'; for a in \"$@\"; do printf '%s\\n' \"$a\"; done; } >> \"$C4V2_LOG\"\n"
	}
	scripts := map[string]string{
		"hoom":  anota("hoom") + "exit 0\n",
		"rm":    anota("rm") + "exit 0\n",
		"touch": anota("touch") + "for a in \"$@\"; do : > \"$a\"; done\nexit 0\n",
	}
	for _, n := range c4v2Ajenos {
		scripts[n] = anota(n) + ": > PWNED\nexit 0\n"
	}
	for n, s := range scripts {
		if err := os.WriteFile(filepath.Join(bin, n), []byte(s), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return c4v2Falso{bin: bin, log: filepath.Join(dir, "llamadas.log")}
}

// pegar corre linea con /bin/sh -c en dir, con PATH = solo los falsos, y
// devuelve la salida (stdout y stderr).
func (f c4v2Falso) pegar(t *testing.T, dir, linea string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", linea)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "PATH=") && !strings.HasPrefix(kv, "C4V2_LOG=") && !strings.HasPrefix(kv, "HOOM_TASK=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, "PATH="+f.bin, "C4V2_LOG="+f.log)
	out, _ := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("pegar %q en sh -c no termino en 30 s", linea)
	}
	return string(out)
}

// llamadas devuelve cada invocacion de los falsos: [nombre, args...].
func (f c4v2Falso) llamadas(t *testing.T) [][]string {
	t.Helper()
	raw, err := os.ReadFile(f.log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var out [][]string
	for _, l := range strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n") {
		if strings.HasPrefix(l, "== ") {
			out = append(out, []string{strings.TrimPrefix(l, "== ")})
			continue
		}
		if len(out) > 0 {
			out[len(out)-1] = append(out[len(out)-1], l)
		}
	}
	return out
}

// c4v2Forma es una accion valida: su texto exacto y lo que recibe hoom.
type c4v2Forma struct {
	texto string
	args  []string
}

// c4v2Agent: 'hoom agent' de rol para precios, con --spec salvo los roles
// que escriben specs, reanudando (provider != "") o volviendo a lanzar.
func c4v2Agent(rol, provider string) c4v2Forma {
	conSpec := rol != "arquitecto"
	texto := "hoom agent --role " + rol + " --task precios"
	args := []string{"agent", "--role", rol, "--task", "precios"}
	if conSpec {
		texto += " --spec .hoom/specs/precios.md"
		args = append(args, "--spec", ".hoom/specs/precios.md")
	}
	if provider != "" {
		texto += " --provider " + provider + " --resume sesion-abc123"
		args = append(args, "--provider", provider, "--resume", "sesion-abc123")
	}
	return c4v2Forma{texto + ` "<pedido>"`, append(args, "<pedido>")}
}

var c4v2Review = c4v2Forma{"hoom review --task precios --spec .hoom/specs/precios.md",
	[]string{"review", "--task", "precios", "--spec", ".hoom/specs/precios.md"}}

// c4v2AccionSegura exige, para una accion de sobre-huerfano del doctor:
//
//   - si es un comando de hoom (empieza con "hoom "): que su texto sea
//     exactamente una de las formas validas, y que pegarla en sh -c no
//     imprima nada (ningun comando "not found"), llame UNA vez y solo a
//     hoom, con los argumentos de una forma valida, y no cree PWNED;
//   - si no (una instruccion en palabras): que pegarla entera, o desde
//     cualquier "hoom " que nombre, no cree PWNED ni ejecute ningun comando
//     que no sea hoom, y que si llama a hoom sea con una forma valida.
func c4v2AccionSegura(t *testing.T, ca, nombre, accion string, formas []c4v2Forma) {
	t.Helper()
	var textos []string
	var want [][]string
	for _, f := range formas {
		textos = append(textos, f.texto)
		want = append(want, append([]string{"hoom"}, f.args...))
	}
	valida := func(ll []string) bool {
		if len(ll) == 0 || ll[0] != "hoom" {
			return false
		}
		for _, f := range formas {
			if reflect.DeepEqual(ll[1:], f.args) {
				return true
			}
		}
		return false
	}

	if strings.HasPrefix(accion, "hoom ") {
		exacta := false
		for _, f := range formas {
			exacta = exacta || accion == f.texto
		}
		if !exacta {
			t.Errorf("%s: [%s] la accion nombra el rol y el provider de hoom, exactos; fue %q, y las validas son %q", ca, nombre, accion, textos)
		}
		falsos := c4v2Falsos(t)
		dir := t.TempDir()
		out := falsos.pegar(t, dir, accion)
		ll := falsos.llamadas(t)
		if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
			t.Errorf("%s: [%s] pegar la accion en sh -c ejecuto un comando ajeno (aparecio PWNED); accion %q, llamadas %q", ca, nombre, accion, ll)
		}
		if out != "" {
			t.Errorf("%s: [%s] pegar la accion en sh -c corre un solo comando de hoom y no imprime nada; accion %q, salida %q, llamadas %q", ca, nombre, accion, out, ll)
		}
		if len(ll) != 1 || !valida(ll[0]) {
			t.Errorf("%s: [%s] pegar la accion llama una sola vez a hoom con los argumentos de una forma valida (--role y --provider exactos); accion %q, llamadas %q, validas %q",
				ca, nombre, accion, ll, want)
		}
		return
	}

	candidatas := []string{accion}
	for i := 0; ; {
		j := strings.Index(accion[i:], "hoom ")
		if j < 0 {
			break
		}
		candidatas = append(candidatas, accion[i+j:])
		i += j + len("hoom ")
	}
	for _, l := range candidatas {
		falsos := c4v2Falsos(t)
		dir := t.TempDir()
		falsos.pegar(t, dir, l)
		ll := falsos.llamadas(t)
		if _, err := os.Stat(filepath.Join(dir, "PWNED")); err == nil {
			t.Errorf("%s: [%s] pegar en sh -c la instruccion %q ejecuto un comando ajeno (aparecio PWNED); llamadas %q", ca, nombre, l, ll)
		}
		for _, c := range ll {
			if !valida(c) {
				t.Errorf("%s: [%s] pegar en sh -c la instruccion %q llama a %q, que no es una forma valida %q", ca, nombre, l, c, want)
			}
		}
	}
}

// c4v2AccionesTexto saca de la salida de 'hoom board doctor' las lineas
// 'Accion:' de los problemas con ese id (la cabecera de cada problema es
// "  <id>  <slug>").
func c4v2AccionesTexto(stdout, id string) []string {
	var out []string
	actual := ""
	for _, l := range strings.Split(stdout, "\n") {
		switch {
		case strings.HasPrefix(l, "    Accion: "):
			if actual == id {
				out = append(out, strings.TrimPrefix(l, "    Accion: "))
			}
		case strings.HasPrefix(l, "  ") && !strings.HasPrefix(l, "   ") && strings.TrimSpace(l) != "":
			actual = strings.Fields(l)[0]
		}
	}
	return out
}

// c4v2Huerfanos son las acciones de los sobre-huerfano de la tarjeta.
func c4v2Huerfanos(ps []Problem) []string {
	var out []string
	for _, p := range ps {
		if p.ID == ProbSobreHuerfano && p.Slug == bdSlug {
			out = append(out, p.Action)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// fffe28: el rol (y el provider) con espacios alrededor.
// ---------------------------------------------------------------------------

// c4v2Rodeos son espacios que strings.TrimSpace recorta alrededor de un
// valor: los de sh (espacio, tab, salto de linea) y los que sh no separa
// (\r, \v, \f, NBSP, NEL).
var c4v2Rodeos = []struct{ antes, despues string }{
	{"\n", ""},
	{"", "\n"},
	{" ", " "},
	{"\t", ""},
	{"", "\t"},
	{"\r\n", "\r\n"},
	{"\n\n", "\n\n"},
	{"\v", ""},
	{"\f", ""},
	{"\u00a0", ""},
	{"", "\u0085"},
}

// c4v2Arquitecto: la tarjeta en Backlog (sin spec y sin cambios), como en
// TestCA367_SobreHuerfano: el rol que la retoma escribe specs.
func c4v2Arquitecto() Evidence {
	ev := drSinSpec(drEv())
	ev.TreeChanges = nil
	return ev
}

// c4v2Revisor: la review exigida de un writer de claude, cortada con un
// reviewer de codex en la lente 2 de 4, como en TestCA367_SobreHuerfano.
func c4v2Revisor(rol string) Evidence {
	ev := acGrande(drEv())
	ev.Runs = []RunState{acGasto("run-w", "writer", "claude", nil, 5*time.Minute)}
	ev = drInterrumpido(ev, rol, "codex")
	ev.Envelopes[0].Record.Step, ev.Envelopes[0].Record.Steps = 2, 4
	return ev
}

type c4v2Caso struct {
	nombre string
	ev     Evidence
	formas []c4v2Forma
}

// c4v2CasosHostiles son sobres interrumpidos de la tarjeta con el rol o el
// provider rodeado de espacios (o con un comando detras de un salto de
// linea), con las formas de accion validas para cada uno.
func c4v2CasosHostiles() []c4v2Caso {
	var casos []c4v2Caso
	writer := []c4v2Forma{c4v2Agent("writer", "claude"), c4v2Agent("writer", "")}
	for _, r := range c4v2Rodeos {
		rol := r.antes + "writer" + r.despues
		casos = append(casos, c4v2Caso{fmt.Sprintf("writer con rol %q", rol), drInterrumpido(drWriterQuieto(), rol, "claude"), writer})
	}
	for _, rol := range []string{"writer\n; touch PWNED", "\nwriter\ntouch PWNED", "writer\ntouch PWNED\n"} {
		casos = append(casos, c4v2Caso{fmt.Sprintf("writer con rol %q", rol), drInterrumpido(drWriterQuieto(), rol, "claude"), writer})
	}
	arq := []c4v2Forma{c4v2Agent("arquitecto", "claude"), c4v2Agent("arquitecto", "")}
	for _, rol := range []string{"\narquitecto", " arquitecto\t", "arquitecto\n"} {
		casos = append(casos, c4v2Caso{fmt.Sprintf("arquitecto con rol %q", rol), drInterrumpido(c4v2Arquitecto(), rol, "claude"), arq})
	}
	for _, rol := range []string{"\nreviewer", " reviewer", "reviewer\n", "\u00a0reviewer"} {
		casos = append(casos, c4v2Caso{fmt.Sprintf("reviewer con rol %q", rol), c4v2Revisor(rol), []c4v2Forma{c4v2Review}})
	}
	for _, p := range []struct{ hostil, canonico string }{
		{"claude\n; touch PWNED", "claude"},
		{"\nclaude", "claude"},
		{"claude\n", "claude"},
		{" claude ", "claude"},
		{"\tclaude", "claude"},
		{"claude\r\n", "claude"},
		{"\u00a0claude", "claude"},
		{"\ncodex", "codex"},
		{" codex\n", "codex"},
	} {
		casos = append(casos, c4v2Caso{fmt.Sprintf("writer con provider %q", p.hostil), drInterrumpido(drWriterQuieto(), "writer", p.hostil),
			[]c4v2Forma{c4v2Agent("writer", p.canonico), c4v2Agent("writer", "")}})
	}
	return casos
}

// CA-367 (hallazgo fffe28): un sobre interrumpido de una tarjeta cuyo rol es
// un rol de hoom rodeado de espacios o saltos de linea ("\nwriter",
// "writer\n", " writer ", "\twriter", con \r, \v, \f, NBSP o NEL), o que
// lleva un comando detras de un salto de linea, no produce una accion de
// sobre-huerfano que pegada en sh -c parta el comando: si el doctor propone
// 'hoom agent' (o 'hoom review'), la accion es exactamente la del contrato
// con el rol de hoom ('--role writer'), y pegarla llama una sola vez a hoom
// con esos argumentos, sin segundo comando y sin PWNED. Lo mismo con el
// provider de la accion de reanudar ('--provider claude', exacto).
func TestCA367_fffe28_RolYProviderConEspaciosNoPartenLaAccion(t *testing.T) {
	for _, cs := range c4v2CasosHostiles() {
		c := Derive(cs.ev)
		if c.Doctor == nil {
			t.Errorf("CA-367: [%s] doctor es una lista, nunca nil", cs.nombre)
			continue
		}
		for _, a := range c4v2Huerfanos(c.Doctor) {
			c4v2AccionSegura(t, "CA-367 (hallazgo fffe28)", cs.nombre, a, cs.formas)
		}
	}
}

// CA-367 (hallazgo fffe28, guarda): con el rol y el provider limpios, el
// sobre interrumpido sigue dando su accion exacta del contrato (reanudar
// para writer y arquitecto, la review para el reviewer), y pegarla con el
// mismo arnes llama una vez a hoom con esos argumentos y nada mas.
func TestCA367_fffe28_Guarda_RolLimpioDaSuAccionExacta(t *testing.T) {
	for _, cs := range []c4v2Caso{
		{"writer de claude", drInterrumpido(drWriterQuieto(), "writer", "claude"), []c4v2Forma{c4v2Agent("writer", "claude")}},
		{"writer de codex", drInterrumpido(drWriterQuieto(), "writer", "codex"), []c4v2Forma{c4v2Agent("writer", "codex")}},
		{"arquitecto de claude", drInterrumpido(c4v2Arquitecto(), "arquitecto", "claude"), []c4v2Forma{c4v2Agent("arquitecto", "claude")}},
		{"reviewer de codex", c4v2Revisor("reviewer"), []c4v2Forma{c4v2Review}},
	} {
		_, ps := drDoctor(t, "CA-367", cs.nombre, cs.ev)
		as := c4v2Huerfanos(ps)
		if len(as) != 1 || as[0] != cs.formas[0].texto {
			t.Fatalf("CA-367: [%s] el sobre interrumpido da UN sobre-huerfano con la accion %q:\n%s", cs.nombre, cs.formas[0].texto, drFmt(ps))
		}
		c4v2AccionSegura(t, "CA-367 (hallazgo fffe28, guarda)", cs.nombre, as[0], cs.formas)
	}
}

// c4v2SobreEnDisco arma un proyecto con la tarjeta precios (con su espacio
// de trabajo, sin cambios) y un registro de sobre suyo abierto, de un PID
// muerto, con el rol dado; ningun otro sobre lo releva.
func c4v2SobreEnDisco(t *testing.T, rol string) (root string, now time.Time) {
	t.Helper()
	root = bdRepo(t, "findings:\n  block_on: high\n")
	now = time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	bdWorktree(t, root, bdSlug)
	drSobre(t, root, "20260923T080000_rolesp", bdSlug, rol, "run", envelope.StatusRunning, bdPIDMuerto(t),
		now.Add(-60*time.Minute), now.Add(-55*time.Minute))
	return root, now
}

// CA-369 (hallazgo fffe28): el mismo sobre, plantado en disco: Doctor junta
// los problemas de las tarjetas, y ni la accion del reporte, ni la de 'hoom
// board doctor --json', ni la linea 'Accion:' del texto parten el comando
// cuando el rol del registro es un rol de hoom rodeado de espacios o saltos
// de linea: si proponen 'hoom agent', es la del contrato con '--role writer'
// exacto, y pegarla llama una vez a hoom con esos argumentos.
func TestCA369_fffe28_SobreEnDiscoConRolRodeado(t *testing.T) {
	bin := bxBinario(t)
	formas := []c4v2Forma{c4v2Agent("writer", "claude"), c4v2Agent("writer", "")}
	for _, rol := range []string{"\nwriter", "writer\n", " writer ", "\twriter", "\u00a0writer"} {
		nombre := fmt.Sprintf("rol %q en disco", rol)
		root, now := c4v2SobreEnDisco(t, rol)
		r, err := Doctor(root, "main", "high", now)
		if err != nil {
			t.Fatalf("CA-369: [%s] Doctor: %v", nombre, err)
		}
		acciones := c4v2Huerfanos(r.Problems)
		js := bxCorrer(t, bin, root, "board", "doctor", "--json")
		bxOK(t, "CA-370", js, "board", "doctor", "--json")
		acciones = append(acciones, c4v2Huerfanos(drDoctorJSON(t, "CA-370", js.stdout))...)
		txt := bxCorrer(t, bin, root, "board", "doctor")
		bxOK(t, "CA-370", txt, "board", "doctor")
		acciones = append(acciones, c4v2AccionesTexto(txt.stdout, ProbSobreHuerfano)...)
		for _, a := range acciones {
			c4v2AccionSegura(t, "CA-369 (hallazgo fffe28)", nombre, a, formas)
		}
	}
}

// CA-369 (hallazgo fffe28, guarda): con el rol limpio en disco, la tarjeta
// trae su sobre-huerfano con la accion de volver a lanzar del contrato (sin
// run no hay sesion que reanudar), igual en el reporte, en --json y en el
// texto, y pegarla llama una vez a hoom con esos argumentos.
func TestCA369_fffe28_Guarda_SobreEnDiscoConRolLimpio(t *testing.T) {
	bin := bxBinario(t)
	root, now := c4v2SobreEnDisco(t, "writer")
	forma := c4v2Agent("writer", "")
	r, err := Doctor(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-369: Doctor: %v", err)
	}
	if as := c4v2Huerfanos(r.Problems); len(as) != 1 || as[0] != forma.texto {
		t.Fatalf("CA-369: la tarjeta trae UN sobre-huerfano con la accion %q:\n%s", forma.texto, drFmt(r.Problems))
	}
	js := bxCorrer(t, bin, root, "board", "doctor", "--json")
	bxOK(t, "CA-370", js, "board", "doctor", "--json")
	if as := c4v2Huerfanos(drDoctorJSON(t, "CA-370", js.stdout)); len(as) != 1 || as[0] != forma.texto {
		t.Fatalf("CA-370: --json trae la misma accion %q: %q", forma.texto, as)
	}
	txt := bxCorrer(t, bin, root, "board", "doctor")
	bxOK(t, "CA-370", txt, "board", "doctor")
	if as := c4v2AccionesTexto(txt.stdout, ProbSobreHuerfano); len(as) != 1 || as[0] != forma.texto {
		t.Fatalf("CA-370: el texto trae, bajo sobre-huerfano, la linea 'Accion: %s': %q\n%s", forma.texto, as, txt.stdout)
	}
	c4v2AccionSegura(t, "CA-369 (hallazgo fffe28, guarda)", "rol limpio en disco", forma.texto, []c4v2Forma{forma})
}
