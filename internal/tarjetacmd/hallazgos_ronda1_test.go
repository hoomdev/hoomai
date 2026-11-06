// Tests adversariales de la ronda 1 de la review de verbos-de-la-tarjeta en
// los verbos de la tarjeta, como consumidor externo (tarjetacmd.Run y
// Deducir con proyectos git reales y CLIs falsos, los fixtures de
// tarjetacmd_test.go). Cubren los hallazgos 1243ff (la carpeta alcanzada por
// un enlace simbolico, CA-499), 726459 (la linea → se copia y se pega en un
// shell, CA-503), 896eff (el piso del provider con el presupuesto de
// hoom.yaml, CA-504), faff7c (lo que se mostro es lo que se lanza, CA-503 y
// CA-504), fbf9e5 (--si saltea la pregunta, no el texto, CA-508 y CA-509),
// 4cb6b0 (los flags de lanzamiento de hoom siguiente, CA-507) y 6e70b3 (la
// review no lleva --timeout, CA-502, enmienda 2).
package tarjetacmd_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/tarjetacmd"
)

// ===========================================================================
// hallazgo 1243ff (CA-499): una carpeta alcanzada por un enlace simbolico
// ===========================================================================

// r1Enlace crea en dir un enlace simbolico nombre -> destino.
func r1Enlace(t *testing.T, destino, dir, nombre string) string {
	t.Helper()
	p := filepath.Join(dir, nombre)
	if err := os.Symlink(destino, p); err != nil {
		t.Fatalf("fixture: enlace %s -> %s: %v", p, destino, err)
	}
	return p
}

// hallazgo 1243ff, CA-499: desde una carpeta a la que se llega por un enlace
// simbolico que esta fuera de .hoom/worktrees y que lleva a
// <proyecto>/.hoom/worktrees/<slug> (o a una subcarpeta, o a un arbol ciego
// anidado), la tarjeta es <slug> (carpeta) y el proyecto es el arbol
// principal, igual que desde la ruta real. Antes el camino lexico no tenia
// .hoom/worktrees/ y el espacio de trabajo se tomaba como proyecto.
func TestHallazgo_1243ff_CA499_EnlaceSimbolicoAlEspacioDeTrabajo(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtItem(t, root, "precios", "")
	vdtItem(t, root, "otra", "")
	wt := vdtTarea(t, root, "precios")
	vdtEscribir(t, root, tarjetacmd.ArchivoFijada, "otra\n")
	hondo := filepath.Join(wt, "internal", "pkg")
	ciego := filepath.Join(wt, ".hoom", "isolated", "20260923T080000_abc", "internal")
	for _, d := range []string{hondo, ciego} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	fuera := t.TempDir()
	atajo := r1Enlace(t, wt, fuera, "atajo")
	casos := []struct{ nombre, dir, real string }{
		{"enlace al espacio de trabajo", atajo, wt},
		{"enlace a una subcarpeta del espacio", r1Enlace(t, hondo, fuera, "hondo"), hondo},
		{"enlace a un arbol ciego anidado", r1Enlace(t, ciego, fuera, "ciego"), ciego},
		{"enlace a un enlace", r1Enlace(t, atajo, fuera, "doble"), wt},
		{"subcarpeta de un enlace", filepath.Join(atajo, "internal", "pkg"), hondo},
		{"enlace dentro del proyecto, fuera de .hoom/worktrees", r1Enlace(t, wt, root, "atajo-interno"), wt},
	}
	for _, c := range casos {
		real, err := vdtDeducir(t, "hallazgo 1243ff", c.real, "")
		if err != nil || real.Slug != "precios" || real.Origen != tarjetacmd.OrigenCarpeta || real.Root != root {
			t.Fatalf("hallazgo 1243ff (%s): fixture: desde la ruta real %s la tarjeta es precios (carpeta) en %s: %+v %v", c.nombre, c.real, root, real, err)
		}
		d, err := vdtDeducir(t, "hallazgo 1243ff", c.dir, "")
		if err != nil || d != real {
			t.Fatalf("hallazgo 1243ff, CA-499 (%s): desde %s se deduce lo mismo que desde la ruta real (%+v), fue %+v %v", c.nombre, c.dir, real, d, err)
		}
		// con el operando, el proyecto sigue siendo el arbol principal
		d, err = vdtDeducir(t, "hallazgo 1243ff", c.dir, "otra")
		if err != nil || d.Slug != "otra" || d.Origen != tarjetacmd.OrigenArgumento || d.Root != root {
			t.Fatalf("hallazgo 1243ff, CA-499 (%s): con operando, la tarjeta otra en el proyecto %s: %+v %v", c.nombre, root, d, err)
		}
		s := vdtCorrer(t, "hallazgo 1243ff", vdtOpts{dir: c.dir}, "guardar")
		if got := vdtPrimeraLinea(s.out); got != "tarjeta: precios (carpeta)" {
			t.Fatalf("hallazgo 1243ff, CA-499 (%s): hoom guardar desde %s imprime primero %q, fue %q:\n%s", c.nombre, c.dir, "tarjeta: precios (carpeta)", got, s)
		}
		// hoom usar sin operando muestra la tarjeta fijada del proyecto, no la
		// de un espacio de trabajo tomado como proyecto
		s = vdtCorrer(t, "hallazgo 1243ff", vdtOpts{dir: c.dir}, "usar")
		vdtExit(t, "hallazgo 1243ff ("+c.nombre+")", s, 0)
		if !strings.Contains(s.out, "tarjeta: otra") || strings.Contains(s.out, "sin tarjeta fijada") {
			t.Fatalf("hallazgo 1243ff, CA-499 (%s): hoom usar desde %s muestra la fijada del proyecto (otra):\n%s", c.nombre, c.dir, s)
		}
	}

	// hoom usar <slug> desde el enlace fija la tarjeta en el proyecto
	s := vdtCorrer(t, "hallazgo 1243ff", vdtOpts{dir: atajo}, "usar", "precios")
	vdtExit(t, "hallazgo 1243ff", s, 0)
	if got, ok := vdtFijada(t, root); !ok || got != "precios\n" {
		t.Fatalf("hallazgo 1243ff, CA-499: hoom usar desde el enlace fija en el proyecto: %q %v\n%s", got, ok, s)
	}
	if _, ok := vdtFijada(t, wt); ok {
		t.Fatalf("hallazgo 1243ff, CA-499: el espacio de trabajo no es un proyecto: no gana .hoom/cache/tarjeta")
	}
}

// ===========================================================================
// hallazgo 726459 (CA-503): la linea → se copia y se pega en un shell
// ===========================================================================

const r1Base = `→ hoom agent --role writer --task precios --spec .hoom/specs/precios.md --provider claude `

// r1Recorte es el pedido como va en la linea →: recortado a 60 caracteres
// con … (los casos de estos tests no tienen saltos de linea ni blancos raros).
func r1Recorte(p string) string {
	r := []rune(p)
	if len(r) <= 60 {
		return p
	}
	return string(r[:60]) + "…"
}

func r1EntreSimples(s string) string { return `'` + strings.ReplaceAll(s, `'`, `'\''`) + `'` }

// r1Shells son los shells con los que se evalua una linea: /bin/sh (POSIX),
// dash, bash, zsh y ksh, los que esten. Ronda 2 (hallazgos 522ed0 y
// 1ccc66): ninguna linea usa $'...', asi que /bin/sh entra siempre.
func r1Shells() []string {
	var out []string
	cands := []string{"/bin/sh", "/bin/dash", "/usr/bin/dash", "/bin/bash", "/usr/bin/bash", "/usr/local/bin/bash",
		"/bin/zsh", "/usr/bin/zsh", "/bin/ksh", "/usr/bin/ksh"}
	vistos := map[string]bool{}
	for _, c := range cands {
		real, err := filepath.EvalSymlinks(c)
		if err != nil || vistos[real] {
			continue
		}
		if fi, err := os.Stat(real); err == nil && !fi.IsDir() && fi.Mode()&0o111 != 0 {
			vistos[real] = true
			out = append(out, c)
		}
	}
	return out
}

// r1Evaluar evalua palabras con sh en una carpeta vacia (printf '%s\0' de
// cada palabra) y devuelve las palabras que resultan. Evaluar no ejecuta
// nada: la carpeta sigue vacia.
func r1Evaluar(t *testing.T, ca, sh, palabras string) []string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command(sh, "-c", `printf '%s\000' `+palabras)
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir, "LC_ALL=C"}
	var errb bytes.Buffer
	cmd.Stderr = &errb
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("%s: %s no puede evaluar la linea pegada (%v): %s\n%q", ca, sh, err, errb.String(), palabras)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		var n []string
		for _, e := range ents {
			n = append(n, e.Name())
		}
		t.Fatalf("%s: pegar la linea en %s ejecuto algo (dejo %v):\n%q", ca, sh, n, palabras)
	}
	ws := strings.Split(string(out), "\x00")
	if len(ws) > 0 && ws[len(ws)-1] == "" {
		ws = ws[:len(ws)-1]
	}
	return ws
}

// r1SinControlCrudo exige que la linea no lleve un caracter de control, un
// byte que no es UTF-8 ni una runa que no se imprime tal cual.
func r1SinControlCrudo(t *testing.T, ca, linea string) {
	t.Helper()
	for i, r := range linea {
		if r == utf8.RuneError {
			if _, n := utf8.DecodeRuneInString(linea[i:]); n == 1 {
				t.Fatalf("%s: la linea lleva un byte que no es UTF-8 (posicion %d): %q", ca, i, linea)
			}
		}
		if r != ' ' && !unicode.IsPrint(r) {
			t.Fatalf("%s: la linea lleva crudo el caracter %U (posicion %d), llega asi a la terminal: %q", ca, r, i, linea)
		}
	}
}

// r1PegarEnShells evalua la linea (sin prefijo) en cada shell y exige que
// den las palabras quiero.
func r1PegarEnShells(t *testing.T, ca, resto string, quiero []string) {
	t.Helper()
	sh := r1Shells()
	if len(sh) == 0 {
		t.Fatalf("%s: no hay ningun shell para pegar la linea", ca)
	}
	for _, s := range sh {
		got := r1Evaluar(t, ca+" ("+s+")", s, resto)
		if len(got) != len(quiero) {
			t.Fatalf("%s (%s): la linea pegada da %d palabras, quiero %d:\n got   %q\n quiero %q", ca, s, len(got), len(quiero), got, quiero)
		}
		for i := range quiero {
			if got[i] != quiero[i] {
				t.Fatalf("%s (%s): la palabra %d de la linea pegada es %q, quiero %q byte a byte:\n%q", ca, s, i, got[i], quiero[i], resto)
			}
		}
	}
}

// hallazgo 726459, CA-503: el pedido de la linea → va entre comillas dobles
// cuando se imprime y no tiene $, `, \, " ni !; si no, entre comillas simples
// (una ' adentro como '\”); con un caracter de control, un byte que no es
// UTF-8 o una runa que no se imprime, como "$(printf '...')" (ronda 2,
// hallazgos 522ed0 y 1ccc66; antes $'...'). Pegada en un shell, la linea da
// el pedido (recortado) byte a byte y no ejecuta nada.
func TestHallazgo_726459_CA503_ElPedidoDeLaFlechaSePegaEnUnShell(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 726459", root, "precios", "")
	const dobles, simples, dolar, cualquiera = "dobles", "simples", "dolar", "cualquiera"
	casos := []struct{ pedido, forma string }{
		{"Implementa el spec", dobles},
		{"it's fine, el 100% de * ? [x] ~ & ; | < > # {a,b}", dobles},
		{"año ñandú ç €", dobles},
		{"Implementa $(touch pwn) ya", simples},
		{"usa `touch pwn` aca", simples},
		{`la ruta C:\temp\nueva y \\ doble`, simples},
		{`di "hola" y listo`, simples},
		{"hola mundo!", simples},
		{"it's $HOME", simples},
		{"a'b'c$x", simples},
		{"$((1+1)) ${HOME} $HOME", simples},
		{"rojo \x1b[31m ya", dolar},
		{"campana \x07 y borrar \x7f", dolar},
		{"byte \xff invalido", dolar},
		{"oculto \u202e al reves", dolar},
		{"cero\u200bancho", dolar},
		{"escape \x1b con ' y \\ y $(touch pwn)", dolar},
		{"$$$" + strings.Repeat("a", 70), simples},
		{`"` + strings.Repeat("ñ", 65), simples},
		{strings.Repeat("b", 59) + "'$" + strings.Repeat("c", 10), cualquiera},
	}
	for _, c := range casos {
		s := vdtCorrer(t, "hallazgo 726459", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", c.pedido)
		ca := "hallazgo 726459, CA-503 (pedido " + strconv.QuoteToASCII(c.pedido) + ")"
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		if !strings.HasPrefix(linea, r1Base) {
			t.Fatalf("%s: la linea → empieza con %q: %q", ca, r1Base, linea)
		}
		ultimo := strings.TrimPrefix(linea, r1Base)
		cut := r1Recorte(c.pedido)
		// primero, la prueba fuerte: pegada en un shell da el pedido byte a
		// byte y no ejecuta nada
		quiero := []string{"--role", "writer", "--task", "precios", "--spec", ".hoom/specs/precios.md", "--provider", "claude", cut}
		r1PegarEnShells(t, ca, strings.TrimPrefix(linea, "→ hoom agent "), quiero)
		switch c.forma {
		case dobles:
			if ultimo != `"`+cut+`"` {
				t.Fatalf("%s: un pedido que se imprime y no tiene $ ` \\ \" ! va entre comillas dobles:\n quiero %q\n fue    %q", ca, `"`+cut+`"`, ultimo)
			}
		case simples:
			if ultimo != r1EntreSimples(cut) {
				t.Fatalf("%s: va entre comillas simples (una ' como '\\''):\n quiero %q\n fue    %q", ca, r1EntreSimples(cut), ultimo)
			}
		case dolar: // ronda 2: la forma "$(printf '...')", ya no $'...'
			if !strings.HasPrefix(ultimo, `"$(printf '`) || !strings.HasSuffix(ultimo, `')"`) || strings.Contains(linea, "$'") {
				t.Fatalf("%s: con un caracter de control, un byte que no es UTF-8 o una runa que no se imprime, va como \"$(printf '...')\", sin $'...': %q", ca, ultimo)
			}
		}
	}
	if p := f.pedidos(t, "claude"); len(p) != len(casos) {
		t.Fatalf("hallazgo 726459: fixture: cada pedido se lanzo (%d de %d)", len(p), len(casos))
	}
}

// r1Despues devuelve la palabra que sigue a flag en ws.
func r1Despues(ws []string, flag string) (string, bool) {
	for i, w := range ws {
		if w == flag && i+1 < len(ws) {
			return ws[i+1], true
		}
	}
	return "", false
}

// hallazgo 726459, CA-503: los valores de --model y --effort van tal cual
// cuando cada byte esta en [A-Za-z0-9._:/@%+,-]; si no, entre comillas
// simples (o "$(printf '...')"), nunca entre dobles. Vale para los de
// hoom.yaml y los del comando; pegada, la linea da cada valor byte a byte y
// no ejecuta nada.
func TestHallazgo_726459_CA503_ModeloYEsfuerzoSePeganEnUnShell(t *testing.T) {
	casos := []struct {
		nombre, yaml   string
		flags          []string
		model, effort  string
		formaM, formaE string // como se escriben en la linea; "" = "$(printf '...')" (ronda 2; antes $'...')
	}{
		{"de hoom.yaml, seguros", "agents:\n  writer:\n    model: \"us.anthropic/claude-opus@4:5+x,y%z_w-1\"\n    effort: high\n", nil,
			"us.anthropic/claude-opus@4:5+x,y%z_w-1", "high", "us.anthropic/claude-opus@4:5+x,y%z_w-1", "high"},
		{"de hoom.yaml, con espacio y $(...)", "agents:\n  writer:\n    model: \"mi modelo $(touch pwn)\"\n    effort: \"alto;touch pwn\"\n", nil,
			"mi modelo $(touch pwn)", "alto;touch pwn", `'mi modelo $(touch pwn)'`, `'alto;touch pwn'`},
		{"del comando, con comilla simple y backtick", "", []string{"--model", "it's", "--effort", "x`touch pwn`"},
			"it's", "x`touch pwn`", `'it'\''s'`, "'x`touch pwn`'"},
		{"del comando, con escape", "", []string{"--model", "m\x1b[2Jx", "--effort", "e f"},
			"m\x1b[2Jx", "e f", "", `'e f'`},
		{"del comando, con = y ~ y comilla doble", "", []string{"--model", "a=b", "--effort", `~"x"`},
			"a=b", `~"x"`, `'a=b'`, `'~"x"'`},
	}
	for _, c := range casos {
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, c.yaml)
		vdtWriter(t, "hallazgo 726459", root, "precios", "")
		args := append([]string{"writer", "precios"}, c.flags...)
		args = append(args, "-m", "Implementa")
		s := vdtCorrer(t, "hallazgo 726459", vdtOpts{dir: root}, "pedir", args...)
		ca := "hallazgo 726459, CA-503 (" + c.nombre + ")"
		linea := vdtFlechaDe(t, ca, s.out)
		r1SinControlCrudo(t, ca, linea)
		resto := strings.TrimPrefix(linea, "→ hoom agent ")
		for _, sh := range r1Shells() {
			ws := r1Evaluar(t, ca+" ("+sh+")", sh, resto)
			if v, _ := r1Despues(ws, "--model"); v != c.model {
				t.Fatalf("%s (%s): pegada, --model da %q, quiero %q byte a byte:\n%q", ca, sh, v, c.model, resto)
			}
			if v, _ := r1Despues(ws, "--effort"); v != c.effort {
				t.Fatalf("%s (%s): pegada, --effort da %q, quiero %q byte a byte:\n%q", ca, sh, v, c.effort, resto)
			}
			if ws[len(ws)-1] != "Implementa" {
				t.Fatalf("%s (%s): la ultima palabra es el pedido: %q", ca, sh, ws)
			}
		}
		for _, k := range []struct{ flag, forma string }{{"--model", c.formaM}, {"--effort", c.formaE}} {
			if k.forma == "" {
				i := strings.Index(linea, " "+k.flag+` "$(printf '`)
				if i < 0 || !strings.Contains(linea[i:], `')" `) || strings.Contains(linea, "$'") {
					t.Fatalf("%s: %s con un caracter de control va como \"$(printf '...')\", sin $'...': %q", ca, k.flag, linea)
				}
				continue
			}
			if !strings.Contains(linea, " "+k.flag+" "+k.forma+" ") {
				t.Fatalf("%s: la linea lleva %q:\n%q", ca, k.flag+" "+k.forma, linea)
			}
		}
		args0 := vdtUnaLlamada(t, ca, "pedir writer", f, "claude")
		if v, _ := vdtSigue(args0, "--model"); v != c.model {
			t.Fatalf("%s: fixture: el provider recibe el modelo %q: %q", ca, c.model, args0)
		}
	}
}

// r1Interrumpida es vdtInterrumpida con un id de sesion a eleccion.
func r1Interrumpida(t *testing.T, ca, root, slug, sesion string) string {
	t.Helper()
	wt := vdtWriter(t, ca, root, slug, "")
	muerto := vdtPIDMuerto(t)
	ahora := time.Now().UTC()
	runID, envID := "20260923T080000_run0ld", "20260923T075959_env0ld"
	vdtSidecar(t, root, runcmd.Meta{ID: runID, Provider: "claude", Role: "writer", Task: slug, Dir: wt,
		CreatedAt: ahora.Add(-2 * time.Hour), Status: runcmd.StatusRunning, ExitCode: -1, PID: muerto, ProviderSessionID: sesion})
	vdtSobre(t, root, envelope.Record{ID: envID, Role: "writer", Provider: "claude", Task: slug, Dir: wt,
		Spec: filepath.Join(wt, ".hoom", "specs", slug+".md"), Approval: "aprobado", RunID: runID,
		Stage: "run", Step: 3, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1,
		StartedAt: ahora.Add(-2 * time.Hour), UpdatedAt: ahora.Add(-time.Hour), PID: muerto})
	if c := vdtCarta(t, root, slug); c.Interrupted == nil || c.Interrupted.EnvelopeID != envID {
		t.Fatalf("%s: fixture: la tarjeta %s esta interrumpida por %s: %+v", ca, slug, envID, c.Interrupted)
	}
	return wt
}

// hallazgo 726459, CA-503 y CA-511: el valor de --resume va tal cual si es
// seguro y, si no, entre comillas simples; hoom siguiente imprime la misma
// linea con el pedido citado igual que hoom pedir.
func TestHallazgo_726459_CA503_ResumeYSiguienteSePeganEnUnShell(t *testing.T) {
	for _, k := range []struct{ sesion, forma string }{
		{"ses-1.2_3:4/5@6%7+8,9", "ses-1.2_3:4/5@6%7+8,9"},
		{"ses 1$(touch pwn)", `'ses 1$(touch pwn)'`},
	} {
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, "")
		r1Interrumpida(t, "hallazgo 726459", root, "precios", k.sesion)
		s := vdtCorrer(t, "hallazgo 726459", vdtOpts{dir: root}, "reanudar", "precios", "-m", "Termina `touch pwn` lo que quedo")
		ca := "hallazgo 726459, CA-503 (--resume " + k.sesion + ")"
		linea := vdtFlechaDe(t, ca, s.out)
		resto := strings.TrimPrefix(linea, "→ hoom agent ")
		for _, sh := range r1Shells() {
			ws := r1Evaluar(t, ca+" ("+sh+")", sh, resto)
			if v, _ := r1Despues(ws, "--resume"); v != k.sesion {
				t.Fatalf("%s (%s): pegada, --resume da %q, quiero %q: %q", ca, sh, v, k.sesion, resto)
			}
			if ws[len(ws)-1] != "Termina `touch pwn` lo que quedo" {
				t.Fatalf("%s (%s): pegada, la ultima palabra es el pedido: %q", ca, sh, ws)
			}
		}
		if !strings.Contains(linea, " --resume "+k.forma+" ") {
			t.Fatalf("%s: la linea lleva --resume %s: %q", ca, k.forma, linea)
		}
		if !strings.HasSuffix(linea, " 'Termina `touch pwn` lo que quedo'") {
			t.Fatalf("%s: el pedido con ` va entre comillas simples: %q", ca, linea)
		}
		args := vdtUnaLlamada(t, ca, "reanudar", f, "claude")
		if v, _ := vdtSigue(args, "--resume"); v != k.sesion {
			t.Fatalf("%s: fixture: el provider retoma la sesion %q: %q", ca, k.sesion, args)
		}
	}

	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 726459", root, "precios", "")
	s := vdtCorrer(t, "hallazgo 726459", vdtOpts{dir: root}, "siguiente", "precios", "-m", "Implementa $(touch pwn) ya")
	linea := vdtFlechaDe(t, "hallazgo 726459 (siguiente)", s.out)
	if linea != r1Base+`'Implementa $(touch pwn) ya'` {
		t.Fatalf("hallazgo 726459, CA-507: hoom siguiente imprime la → de hoom pedir, con el pedido entre comillas simples: %q", linea)
	}
	for _, sh := range r1Shells() {
		ws := r1Evaluar(t, "hallazgo 726459 (siguiente, "+sh+")", sh, strings.TrimPrefix(linea, "→ hoom agent "))
		if ws[len(ws)-1] != "Implementa $(touch pwn) ya" {
			t.Fatalf("hallazgo 726459 (siguiente, %s): pegada da el pedido: %q", sh, ws)
		}
	}
}

// hallazgo 726459, CA-503: la → del reviewer (hoom review ...) tambien se
// pega sin ejecutar nada, con un modelo de review: que no es seguro.
func TestHallazgo_726459_CA503_FlechaDelReviewerSePegaEnUnShell(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "review:\n  model: \"mi modelo $(touch pwn)\"\n  effort: \"a`touch pwn`\"\n")
	vdtReview(t, "hallazgo 726459", root, "precios")
	s := vdtCorrer(t, "hallazgo 726459", vdtOpts{dir: root}, "pedir", "reviewer", "precios")
	linea := vdtFlechaDe(t, "hallazgo 726459 (reviewer)", s.out)
	r1SinControlCrudo(t, "hallazgo 726459 (reviewer)", linea)
	const pre = "→ hoom review --task precios --spec .hoom/specs/precios.md --provider claude"
	if !strings.HasPrefix(linea, pre) {
		t.Fatalf("hallazgo 726459, CA-503: la → del reviewer empieza con %q: %q", pre, linea)
	}
	resto := strings.TrimPrefix(linea, "→ hoom review ")
	for _, sh := range r1Shells() {
		ws := r1Evaluar(t, "hallazgo 726459 (reviewer, "+sh+")", sh, resto)
		if v, ok := r1Despues(ws, "--model"); ok && v != "mi modelo $(touch pwn)" {
			t.Fatalf("hallazgo 726459 (reviewer, %s): pegada, --model da %q: %q", sh, v, resto)
		}
		if v, ok := r1Despues(ws, "--effort"); ok && v != "a`touch pwn`" {
			t.Fatalf("hallazgo 726459 (reviewer, %s): pegada, --effort da %q: %q", sh, v, resto)
		}
	}
}

// ===========================================================================
// hallazgo 896eff (CA-504): el piso del provider con el presupuesto de hoom.yaml
// ===========================================================================

const r1Piso = "claude necesita al menos 0.5 USD por run"

// hallazgo 896eff, CA-504: un presupuesto que sale de hoom.yaml
// (agents.writer.budget_usd: 0.1) por debajo del piso de claude se rechaza
// como uno pedido: exit 1 con el motivo, sin lanzar nada (ni sobre ni CLI),
// desde hoom pedir y hoom siguiente, con -m o con el editor.
func TestHallazgo_896eff_CA504_PresupuestoDeHoomYamlBajoElPiso(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    budget_usd: 0.1\n")
	vdtWriter(t, "hallazgo 896eff", root, "precios", "")
	a := vdtAccion(t, "hallazgo 896eff", vdtCarta(t, root, "precios"), boardcmd.ActPedirWriter)
	if a.BudgetUSD == nil || *a.BudgetUSD != 0.1 {
		t.Fatalf("hallazgo 896eff: fixture: pedir-writer propone el presupuesto de hoom.yaml (0.1): %+v", a.BudgetUSD)
	}
	ed, _ := vdtEditor(t, "editor", "Implementa el spec", 0)
	casos := []struct {
		nombre string
		o      vdtOpts
		verbo  string
		args   []string
	}{
		{"pedir -m", vdtOpts{dir: root}, "pedir", []string{"writer", "precios", "-m", "Implementa el spec"}},
		{"siguiente -m", vdtOpts{dir: root}, "siguiente", []string{"precios", "-m", "Implementa el spec"}},
		{"pedir sin terminal", vdtOpts{dir: root}, "pedir", []string{"writer", "precios"}},
		{"pedir con el editor", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed}}, "pedir", []string{"writer", "precios"}},
	}
	for _, c := range casos {
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "hallazgo 896eff", c.o, c.verbo, c.args...)
		vdtExit(t, "hallazgo 896eff, CA-504 ("+c.nombre+")", s, 1)
		if !strings.Contains(s.err, r1Piso) {
			t.Fatalf("hallazgo 896eff, CA-504 (%s): el presupuesto de hoom.yaml bajo el piso dice %q:\n%s", c.nombre, r1Piso, s)
		}
		vdtSinEfectos(t, "hallazgo 896eff", c.nombre, antes, vdtFoto(t, root))
	}
	f.ninguna(t, "hallazgo 896eff", "un presupuesto de hoom.yaml bajo el piso")
	if n := len(vdtSobres(t, root)); n != 0 {
		t.Fatalf("hallazgo 896eff: no se lanzo nada: %d sobres", n)
	}

	// el mismo presupuesto pedido en el comando da el mismo motivo
	root2 := vdtProyecto(t, "")
	vdtWriter(t, "hallazgo 896eff", root2, "precios", "")
	s := vdtCorrer(t, "hallazgo 896eff", vdtOpts{dir: root2}, "pedir", "writer", "precios", "--budget-usd", "0.1", "-m", "Implementa el spec")
	vdtExit(t, "hallazgo 896eff (--budget-usd 0.1)", s, 1)
	if !strings.Contains(s.err, r1Piso) {
		t.Fatalf("hallazgo 896eff: fixture: --budget-usd 0.1 pedido dice %q:\n%s", r1Piso, s)
	}
	f.ninguna(t, "hallazgo 896eff", "un presupuesto pedido bajo el piso")
}

// hallazgo 896eff: el piso no rechaza de mas: budget_usd igual al piso
// lanza con ese tope, y con codex (que no acepta tope) budget_usd 0.1 no se
// pasa y lanza.
func TestHallazgo_896eff_ElPisoNoRechazaDeMas(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    budget_usd: 0.5\n")
	vdtWriter(t, "hallazgo 896eff", root, "precios", "")
	s := vdtCorrer(t, "hallazgo 896eff", vdtOpts{dir: root}, "pedir", "writer", "precios", "-m", "Implementa el spec")
	if strings.Contains(s.err, r1Piso) {
		t.Fatalf("hallazgo 896eff: 0.5 no esta por debajo del piso de claude:\n%s", s)
	}
	args := vdtUnaLlamada(t, "hallazgo 896eff", "pedir writer con 0.5", f, "claude")
	if v, _ := vdtSigue(args, "--max-budget-usd"); v != "0.5" {
		t.Fatalf("hallazgo 896eff: el tope de hoom.yaml llega al provider: %q", args)
	}

	f2 := vdtPATH(t)
	f2.cli(t, "codex", "")
	root2 := vdtProyecto(t, "agents:\n  writer:\n    provider: codex\n    budget_usd: 0.1\n")
	vdtWriter(t, "hallazgo 896eff", root2, "precios", "")
	s = vdtCorrer(t, "hallazgo 896eff", vdtOpts{dir: root2}, "pedir", "writer", "precios", "-m", "Implementa el spec")
	if strings.Contains(s.err, "necesita al menos") {
		t.Fatalf("hallazgo 896eff: codex no acepta tope: budget_usd 0.1 no se pasa y no tiene piso:\n%s", s)
	}
	vdtUnaLlamada(t, "hallazgo 896eff", "pedir writer con codex", f2, "codex")
}

// ===========================================================================
// hallazgo faff7c (CA-503, CA-504): lo que se mostro es lo que se lanza
// ===========================================================================

func r1Cambio(lista string) string {
	return "la tarjeta cambio mientras escribias el pedido (" + lista + "): no se lanzo nada, vuelve a pedirlo"
}

// r1EditorQueCambia es un editor que deja pedido en el archivo y, mientras
// "edita", reescribe cada archivo de cambios con su contenido nuevo (como
// otra persona que toca hoom.yaml en ese momento).
func r1EditorQueCambia(t *testing.T, pedido string, cambios map[string]string) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\nfor ultimo; do :; done\n")
	b.WriteString("cat > \"$ultimo\" <<'R1_PEDIDO'\n" + pedido + "\nR1_PEDIDO\n")
	i := 0
	for ruta, cont := range cambios {
		if !strings.HasSuffix(cont, "\n") {
			cont += "\n"
		}
		fin := "R1_FIN_" + itoa(i)
		b.WriteString("cat > '" + ruta + "' <<'" + fin + "'\n" + cont + fin + "\n")
		i++
	}
	b.WriteString("exit 0\n")
	p := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(p, []byte(b.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func r1Leer(t *testing.T, p string) string {
	t.Helper()
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func r1Reemplazar(t *testing.T, s string, pares ...string) string {
	t.Helper()
	for i := 0; i+1 < len(pares); i += 2 {
		if !strings.Contains(s, pares[i]) {
			t.Fatalf("fixture: %q no esta en:\n%s", pares[i], s)
		}
		s = strings.Replace(s, pares[i], pares[i+1], 1)
	}
	return s
}

// r1NadaLanzado: ningun CLI corrio, ningun sobre ni run.
func r1NadaLanzado(t *testing.T, ca, que string, f *vdtFalsos, root string) {
	t.Helper()
	f.ninguna(t, ca, que)
	if n := len(vdtSobres(t, root)); n != 0 {
		t.Fatalf("%s: %s no lanza nada: hay %d sobres", ca, que, n)
	}
	if n := len(runcmd.Metas(root)); n != 0 {
		t.Fatalf("%s: %s no lanza nada: hay %d runs", ca, que, n)
	}
}

// hallazgo faff7c, CA-504: si, mientras la persona escribe el pedido en el
// editor, cambia lo que la accion propone (el hoom.yaml del proyecto), hoom
// pedir no lanza otra cosa que lo que mostro: exit 1 con el motivo que nombra
// lo que cambio, en el orden provider, modelo, esfuerzo, presupuesto, tiempo
// limite; nada se lanza.
func TestHallazgo_faff7c_CA504_PedirNoLanzaOtraCosaQueLoQueMostro(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	f.cli(t, "codex", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    model: m-a\n    effort: high\n    budget_usd: 2\n    timeout: 45m\n")
	vdtWriter(t, "hallazgo faff7c", root, "precios", "")
	yamlP := filepath.Join(root, "hoom.yaml")
	orig := r1Leer(t, yamlP)
	casos := []struct {
		nombre string
		pares  []string
		lista  string
	}{
		{"tiempo limite 45m -> 1h", []string{"timeout: 45m", "timeout: 1h"}, "tiempo limite"},
		{"modelo", []string{"model: m-a", "model: m-b"}, "modelo"},
		{"esfuerzo", []string{"effort: high", "effort: low"}, "esfuerzo"},
		{"presupuesto", []string{"budget_usd: 2", "budget_usd: 3"}, "presupuesto"},
		{"modelo y tiempo limite, al reves", []string{"timeout: 45m", "timeout: 2h", "model: m-a", "model: m-c"}, "modelo, tiempo limite"},
		{"todo", []string{"writer:\n", "writer:\n    provider: codex\n", "model: m-a", "model: m-z", "effort: high", "effort: max",
			"budget_usd: 2", "budget_usd: 4", "timeout: 45m", "timeout: 50m"}, "provider, modelo, esfuerzo, presupuesto, tiempo limite"},
	}
	for _, c := range casos {
		ed := r1EditorQueCambia(t, "Implementa el spec con cuidado", map[string]string{yamlP: r1Reemplazar(t, orig, c.pares...)})
		s := vdtCorrer(t, "hallazgo faff7c", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed}}, "pedir", "writer", "precios")
		if r1Leer(t, yamlP) == orig {
			t.Fatalf("hallazgo faff7c (%s): fixture: el editor cambio hoom.yaml:\n%s", c.nombre, s)
		}
		vdtExit(t, "hallazgo faff7c, CA-504 ("+c.nombre+")", s, 1)
		if !strings.Contains(s.err, r1Cambio(c.lista)) {
			t.Fatalf("hallazgo faff7c, CA-504 (%s): el motivo dice %q:\n%s", c.nombre, r1Cambio(c.lista), s)
		}
		r1NadaLanzado(t, "hallazgo faff7c ("+c.nombre+")", "una tarjeta que cambio mientras se escribia el pedido", f, root)
		vdtEscribir(t, root, "hoom.yaml", orig)
	}

	// el provider de hoom.yaml (sin presupuesto declarado, para que nada mas cambie)
	f2 := vdtPATH(t)
	f2.cli(t, "claude", "")
	f2.cli(t, "codex", "")
	root2 := vdtProyecto(t, "agents:\n  writer:\n    provider: claude\n")
	vdtWriter(t, "hallazgo faff7c", root2, "precios", "")
	y2 := filepath.Join(root2, "hoom.yaml")
	ed := r1EditorQueCambia(t, "Implementa el spec", map[string]string{y2: r1Reemplazar(t, r1Leer(t, y2), "provider: claude", "provider: codex")})
	s := vdtCorrer(t, "hallazgo faff7c", vdtOpts{dir: root2, terminal: true, env: map[string]string{"EDITOR": ed}}, "pedir", "writer", "precios")
	vdtExit(t, "hallazgo faff7c, CA-504 (provider)", s, 1)
	if !strings.Contains(s.err, r1Cambio("provider")) {
		t.Fatalf("hallazgo faff7c, CA-504 (provider): el motivo dice %q:\n%s", r1Cambio("provider"), s)
	}
	r1NadaLanzado(t, "hallazgo faff7c (provider)", "un provider que cambio mientras se escribia el pedido", f2, root2)
}

// hallazgo faff7c: un cambio que no toca lo que la accion propone (los
// valores de otro rol) no es un cambio: se lanza lo que se mostro.
func TestHallazgo_faff7c_UnCambioDeOtroRolNoRechaza(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    timeout: 45m\n")
	vdtWriter(t, "hallazgo faff7c", root, "precios", "")
	yamlP := filepath.Join(root, "hoom.yaml")
	nuevo := r1Leer(t, yamlP) + "  arquitecto:\n    timeout: 2h\n    model: otro\n"
	ed := r1EditorQueCambia(t, "Implementa el spec con cuidado", map[string]string{yamlP: nuevo})
	s := vdtCorrer(t, "hallazgo faff7c", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed}}, "pedir", "writer", "precios")
	if strings.Contains(s.err, "la tarjeta cambio mientras escribias el pedido") {
		t.Fatalf("hallazgo faff7c: cambiar los valores de otro rol no cambia lo que se mostro:\n%s", s)
	}
	if p := f.pedidos(t, "claude"); len(p) != 1 || !strings.Contains(p[0], "Implementa el spec con cuidado") {
		t.Fatalf("hallazgo faff7c: se lanza lo que se mostro, con el pedido del editor: %q\n%s", p, s)
	}
	if args := vdtUnaLlamada(t, "hallazgo faff7c", "pedir writer", f, "claude"); strings.Contains(strings.Join(args, " "), "otro") {
		t.Fatalf("hallazgo faff7c: los valores del arquitecto no llegan al writer: %q", args)
	}
	if m := runcmd.Metas(root); len(m) != 1 || m[0].TimeoutS != 2700 {
		t.Fatalf("hallazgo faff7c: el writer lleva el tiempo limite que se mostro (45m): %+v", m)
	}
}

// hallazgo faff7c, CA-504, CA-507 y CA-511: hoom siguiente, hoom reanudar y
// hoom pedir al arquitecto en Backlog tambien pasan lo que mostraron: un
// cambio mientras se escribe el pedido es exit 1 con el motivo, sin lanzar
// nada (en Backlog, tampoco la tarea); en reanudar, una sesion que cambio se
// nombra como sesion.
func TestHallazgo_faff7c_CA504_SiguienteReanudarYBacklog(t *testing.T) {
	// hoom siguiente sobre una tarjeta en Writer
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "agents:\n  writer:\n    timeout: 45m\n")
	vdtWriter(t, "hallazgo faff7c", root, "precios", "")
	yamlP := filepath.Join(root, "hoom.yaml")
	ed := r1EditorQueCambia(t, "Implementa el spec", map[string]string{yamlP: r1Reemplazar(t, r1Leer(t, yamlP), "timeout: 45m", "timeout: 1h")})
	s := vdtCorrer(t, "hallazgo faff7c", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed}}, "siguiente", "precios")
	vdtExit(t, "hallazgo faff7c, CA-507 (siguiente)", s, 1)
	if !strings.Contains(s.err, r1Cambio("tiempo limite")) {
		t.Fatalf("hallazgo faff7c, CA-507: hoom siguiente dice %q:\n%s", r1Cambio("tiempo limite"), s)
	}
	r1NadaLanzado(t, "hallazgo faff7c (siguiente)", "hoom siguiente con la tarjeta cambiada", f, root)

	// hoom reanudar: el tiempo limite y la sesion
	for _, k := range []struct {
		nombre, lista string
		cambio        func(t *testing.T, root string) map[string]string
	}{
		{"tiempo limite", "tiempo limite", func(t *testing.T, root string) map[string]string {
			p := filepath.Join(root, "hoom.yaml")
			return map[string]string{p: r1Reemplazar(t, r1Leer(t, p), "timeout: 45m", "timeout: 1h")}
		}},
		{"sesion", "sesion", func(t *testing.T, root string) map[string]string {
			p := filepath.Join(root, ".hoom", "runs", "20260923T080000_run0ld.meta.json")
			return map[string]string{p: r1Reemplazar(t, r1Leer(t, p), vdtSesion, "sesion-nueva-456")}
		}},
	} {
		f := vdtPATH(t)
		f.cli(t, "claude", "")
		root := vdtProyecto(t, "agents:\n  writer:\n    timeout: 45m\n")
		vdtInterrumpida(t, "hallazgo faff7c", root, "precios", true)
		ed := r1EditorQueCambia(t, "Segui desde el paso 3", k.cambio(t, root))
		s := vdtCorrer(t, "hallazgo faff7c", vdtOpts{dir: root, terminal: true, env: map[string]string{"EDITOR": ed}}, "reanudar", "precios")
		vdtExit(t, "hallazgo faff7c, CA-511 (reanudar, "+k.nombre+")", s, 1)
		if !strings.Contains(s.err, r1Cambio(k.lista)) {
			t.Fatalf("hallazgo faff7c, CA-511 (reanudar, %s): el motivo dice %q:\n%s", k.nombre, r1Cambio(k.lista), s)
		}
		f.ninguna(t, "hallazgo faff7c (reanudar, "+k.nombre+")", "reanudar con la tarjeta cambiada")
		if n := len(vdtSobres(t, root)); n != 1 {
			t.Fatalf("hallazgo faff7c (reanudar, %s): no se lanzo nada: solo el sobre interrumpido, hay %d", k.nombre, n)
		}
		if n := len(runcmd.Metas(root)); n != 1 {
			t.Fatalf("hallazgo faff7c (reanudar, %s): no se lanzo nada: solo el run interrumpido, hay %d", k.nombre, n)
		}
	}

	// hoom pedir arquitecto en Backlog: tampoco se crea la tarea
	f3 := vdtPATH(t)
	f3.cli(t, "claude", "")
	root3 := vdtProyecto(t, "agents:\n  arquitecto:\n    timeout: 45m\n")
	vdtBacklog(t, "hallazgo faff7c", root3, "precios")
	y3 := filepath.Join(root3, "hoom.yaml")
	ed = r1EditorQueCambia(t, "Escribi el spec de precios", map[string]string{y3: r1Reemplazar(t, r1Leer(t, y3), "timeout: 45m", "timeout: 1h")})
	s = vdtCorrer(t, "hallazgo faff7c", vdtOpts{dir: root3, terminal: true, env: map[string]string{"EDITOR": ed}}, "pedir", "arquitecto", "precios")
	vdtExit(t, "hallazgo faff7c, CA-504 (Backlog)", s, 1)
	if !strings.Contains(s.err, r1Cambio("tiempo limite")) {
		t.Fatalf("hallazgo faff7c, CA-504 (Backlog): el motivo dice %q:\n%s", r1Cambio("tiempo limite"), s)
	}
	r1NadaLanzado(t, "hallazgo faff7c (Backlog)", "pedir al arquitecto con la tarjeta cambiada", f3, root3)
	if _, err := os.Stat(filepath.Join(root3, ".hoom", "worktrees", "precios")); !os.IsNotExist(err) {
		t.Fatalf("hallazgo faff7c, CA-504: no se lanzo nada: tampoco la tarea de Backlog: %v", err)
	}
}

// ===========================================================================
// hallazgo fbf9e5 (CA-508, CA-509): --si saltea la pregunta, no el texto
// ===========================================================================

// hallazgo fbf9e5, CA-508: con terminal, hoom aprobar --si imprime el mismo
// texto de la firma que sin --si y no pregunta (ni lee la respuesta); sin
// terminal y con --si, sin texto.
func TestHallazgo_fbf9e5_CA508_AprobarSiConTerminalMuestraElTexto(t *testing.T) {
	root := vdtProyecto(t, "")
	wt := vdtTuAprobacion(t, "hallazgo fbf9e5", root, "precios", "- CA-1: uno.\n- CA-2: dos.")
	firma := vdtAccion(t, "hallazgo fbf9e5", vdtCarta(t, root, "precios"), boardcmd.ActAprobar).Signer
	texto := vdtTextoAprobar("Tarjeta precios", 2, firma)
	s := vdtCorrer(t, "hallazgo fbf9e5", vdtOpts{dir: root, terminal: true, in: "n\n"}, "aprobar", "precios", "--si")
	vdtExit(t, "hallazgo fbf9e5, CA-508", s, 0)
	vdtDiceOut(t, "hallazgo fbf9e5, CA-508", s, texto)
	if strings.Contains(s.out+s.err, "¿Seguir?") {
		t.Fatalf("hallazgo fbf9e5, CA-508: --si saltea la pregunta:\n%s", s)
	}
	if i, j := strings.Index(s.out, texto), strings.Index(s.out, "aprobado: "); j < 0 || i > j {
		t.Fatalf("hallazgo fbf9e5, CA-508: el texto va antes de aprobado: (%d %d):\n%s", i, j, s)
	}
	if len(vdtAprobaciones(t, wt)) != 1 {
		t.Fatalf("hallazgo fbf9e5, CA-508: con --si aprueba (la respuesta n no se lee):\n%s", s)
	}

	wt2 := vdtTuAprobacion(t, "hallazgo fbf9e5", root, "otra", "- CA-1: uno.")
	s = vdtCorrer(t, "hallazgo fbf9e5", vdtOpts{dir: root}, "aprobar", "otra", "--si")
	vdtExit(t, "hallazgo fbf9e5, CA-508 (sin terminal)", s, 0)
	if strings.Contains(s.out+s.err, "Vas a aprobar") || strings.Contains(s.out+s.err, "¿Seguir?") {
		t.Fatalf("hallazgo fbf9e5, CA-508: sin terminal y con --si no hay texto ni pregunta:\n%s", s)
	}
	if st, _, _ := approval.Status(wt2, filepath.Join(wt2, ".hoom", "specs", "otra.md")); st != approval.StatusApproved {
		t.Fatalf("hallazgo fbf9e5, CA-508: sin terminal y con --si aprueba: %v\n%s", st, s)
	}
}

// hallazgo fbf9e5, CA-509: lo mismo para hoom integrar --si.
func TestHallazgo_fbf9e5_CA509_IntegrarSiConTerminalMuestraElTexto(t *testing.T) {
	root := vdtProyecto(t, "")
	vdtTuAceptacion(t, "hallazgo fbf9e5", root, "precios")
	texto := vdtIntegrarTexto("precios")
	s := vdtCorrer(t, "hallazgo fbf9e5", vdtOpts{dir: root, terminal: true, in: "n\n"}, "integrar", "precios", "--si")
	vdtExit(t, "hallazgo fbf9e5, CA-509", s, 0)
	vdtDiceOut(t, "hallazgo fbf9e5, CA-509", s, texto)
	if strings.Contains(s.out+s.err, "¿Seguir?") {
		t.Fatalf("hallazgo fbf9e5, CA-509: --si saltea la pregunta:\n%s", s)
	}
	if i, j := strings.Index(s.out, texto), strings.Index(s.out, "listo: la tarjeta pasa a Hecho"); j < 0 || i > j {
		t.Fatalf("hallazgo fbf9e5, CA-509: el texto va antes de listo: (%d %d):\n%s", i, j, s)
	}
	vdtColumna(t, "hallazgo fbf9e5, CA-509", root, "precios", boardcmd.ColHecho)

	vdtTuAceptacion(t, "hallazgo fbf9e5", root, "otra")
	s = vdtCorrer(t, "hallazgo fbf9e5", vdtOpts{dir: root}, "integrar", "otra", "--si")
	vdtExit(t, "hallazgo fbf9e5, CA-509 (sin terminal)", s, 0)
	if strings.Contains(s.out+s.err, "Se cierra el trabajo") || strings.Contains(s.out+s.err, "¿Seguir?") {
		t.Fatalf("hallazgo fbf9e5, CA-509: sin terminal y con --si no hay texto ni pregunta:\n%s", s)
	}
	if it, err := item.Load(root, "otra"); err != nil || it.HechoEn == nil {
		t.Fatalf("hallazgo fbf9e5, CA-509: sin terminal y con --si integra: %+v %v", it, err)
	}
}

// ===========================================================================
// hallazgo 4cb6b0 (CA-507): los flags de lanzamiento de hoom siguiente
// ===========================================================================

// hallazgo 4cb6b0, CA-507: cuando hoom siguiente no lanza un rol (Tu
// aprobacion, Tu aceptacion, Hecho), cada flag de lanzamiento, escrito como
// --flag valor, --flag=valor o -flag valor, es exit 2 nombrando el flag como
// se escribio, sin efectos.
func TestHallazgo_4cb6b0_CA507_CadaFlagDeLanzamientoSinRol(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "")
	vdtTuAprobacion(t, "hallazgo 4cb6b0", root, "por-aprobar", "- CA-1: uno.")
	vdtTuAceptacion(t, "hallazgo 4cb6b0", root, "por-integrar")
	vdtHecho(t, "hallazgo 4cb6b0", root, "hecha")
	tarjetas := []struct{ slug, sigue string }{
		{"por-aprobar", "; ahora te toca hoom aprobar"},
		{"por-integrar", "; ahora te toca hoom integrar"},
		{"hecha", "; "},
	}
	flags := []struct{ nombre, valor string }{
		{"m", "x"}, {"provider", "claude"}, {"model", "opus"}, {"effort", "high"},
		{"budget-usd", "2"}, {"timeout", "45m"}, {"json", ""},
	}
	for _, tj := range tarjetas {
		for _, fl := range flags {
			canon := "--" + fl.nombre
			if fl.nombre == "m" {
				canon = "-m"
			}
			type forma struct {
				args    []string
				escrito string
			}
			var formas []forma
			if fl.valor == "" {
				formas = []forma{{[]string{canon}, canon}, {[]string{canon + "=true"}, canon}, {[]string{"-" + fl.nombre}, "-" + fl.nombre}}
			} else {
				formas = []forma{{[]string{canon, fl.valor}, canon}, {[]string{canon + "=" + fl.valor}, canon}}
				if canon != "-"+fl.nombre {
					formas = append(formas, forma{[]string{"-" + fl.nombre, fl.valor}, "-" + fl.nombre})
				}
			}
			for _, fo := range formas {
				args := append([]string{tj.slug}, fo.args...)
				antes := vdtFoto(t, root)
				s := vdtCorrer(t, "hallazgo 4cb6b0", vdtOpts{dir: root}, "siguiente", args...)
				que := "hoom siguiente " + strings.Join(args, " ")
				vdtExit(t, "hallazgo 4cb6b0, CA-507 ("+que+")", s, 2)
				want := fo.escrito + " solo sirve cuando hoom siguiente lanza un rol" + tj.sigue
				if !strings.Contains(s.out+s.err, want) {
					t.Fatalf("hallazgo 4cb6b0, CA-507: %s dice %q:\n%s", que, want, s)
				}
				vdtSinEfectos(t, "hallazgo 4cb6b0", que, antes, vdtFoto(t, root))
			}
		}
	}
	f.ninguna(t, "hallazgo 4cb6b0", "un flag de lanzamiento sin rol")
}

// ===========================================================================
// hallazgo 6e70b3 (CA-502, enmienda 2): la review no lleva --timeout
// ===========================================================================

const r1SinTimeout = "la revision toma su tiempo limite de review.timeout: no lleva --timeout"

// hallazgo 6e70b3, CA-502: hoom pedir reviewer --timeout (en cualquier
// forma) es exit 2 con el texto del contrato, sin lanzar nada; lo mismo
// hoom siguiente --timeout cuando lanzaria al reviewer. Sin --timeout el
// reviewer se lanza como antes.
func TestHallazgo_6e70b3_CA502_LaReviewNoLlevaTimeout(t *testing.T) {
	f := vdtPATH(t)
	f.cli(t, "claude", "")
	root := vdtProyecto(t, "review:\n  timeout: 60m\n")
	vdtReview(t, "hallazgo 6e70b3", root, "precios")
	casos := [][]string{
		{"pedir", "reviewer", "precios", "--timeout", "45m"},
		{"pedir", "reviewer", "precios", "--timeout=45m"},
		{"pedir", "reviewer", "precios", "-timeout", "2h"},
		{"pedir", "reviewer", "precios", "-timeout=1h30m"},
		{"pedir", "reviewer", "--timeout", "60m", "precios"},
		{"siguiente", "precios", "--timeout", "45m"},
		{"siguiente", "precios", "--timeout=45m"},
		{"siguiente", "precios", "-timeout", "45m"},
	}
	for _, c := range casos {
		antes := vdtFoto(t, root)
		s := vdtCorrer(t, "hallazgo 6e70b3", vdtOpts{dir: root}, c[0], c[1:]...)
		que := "hoom " + strings.Join(c, " ")
		vdtExit(t, "hallazgo 6e70b3, CA-502 ("+que+")", s, 2)
		if !strings.Contains(s.err, r1SinTimeout) {
			t.Fatalf("hallazgo 6e70b3, CA-502: %s dice %q:\n%s", que, r1SinTimeout, s)
		}
		vdtSinEfectos(t, "hallazgo 6e70b3", que, antes, vdtFoto(t, root))
	}
	f.ninguna(t, "hallazgo 6e70b3", "la review con --timeout")

	s := vdtCorrer(t, "hallazgo 6e70b3", vdtOpts{dir: root}, "pedir", "reviewer", "precios")
	if strings.Contains(s.err, r1SinTimeout) || len(f.llamadas(t, "claude")) == 0 {
		t.Fatalf("hallazgo 6e70b3, CA-502: sin --timeout el reviewer se lanza como antes:\n%s", s)
	}
	if v := vdtValoresDe(t, "hallazgo 6e70b3", s.out); !strings.HasSuffix(v, " · tiempo limite 1h (hoom.yaml)") {
		t.Fatalf("hallazgo 6e70b3, CA-502: el tiempo limite de la review sale de review.timeout: %q", v)
	}
}
