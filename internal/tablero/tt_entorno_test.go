// El entorno (CA-516, CA-518, CA-522, CA-524, CA-534, CA-536): ParseArgs,
// ConColor, ConUTF8, ComandoEditor, ArgumentosEditor, la pantalla sin color
// y en ASCII en todas las vistas, y los require de go.mod.
package tablero_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/tablero"
)

// ttEnv es un getenv de un mapa.
func ttEnv(kv map[string]string) func(string) string {
	return func(k string) string { return kv[k] }
}

// CA-516: ParseArgs: sin args nada; un flag o un operando es un
// *cliargs.UsageError (exit 2) con el bloque de uso; -h y --help son
// cliargs.ErrHelp.
func TestCA516_ParseArgs(t *testing.T) {
	for _, args := range [][]string{nil, {}} {
		if err := tablero.ParseArgs(args); err != nil {
			t.Fatalf("CA-516: ParseArgs(%q) = %v, sin args no hay error", args, err)
		}
	}
	for _, args := range [][]string{{"-h"}, {"--help"}} {
		if err := tablero.ParseArgs(args); !errors.Is(err, cliargs.ErrHelp) {
			t.Fatalf("CA-516: ParseArgs(%q) es cliargs.ErrHelp, dio %v", args, err)
		}
	}
	for _, args := range [][]string{{"--json"}, {"-x"}, {"infra-api"}, {"--model", "opus"}, {"-"}, {"--", "x"}, {"-json=true"}} {
		err := tablero.ParseArgs(args)
		var ue *cliargs.UsageError
		if !errors.As(err, &ue) {
			t.Fatalf("CA-516: ParseArgs(%q) es un *cliargs.UsageError, dio %v", args, err)
		}
		if ue.ExitCode() != 2 || ue.Usage != tablero.UsageText {
			t.Fatalf("CA-516: ParseArgs(%q): exit 2 con el bloque de uso de hoom tablero: %#v", args, ue)
		}
	}
}

// CA-534: ConColor es false solo con NO_COLOR con un valor no vacio.
func TestCA534_ConColor(t *testing.T) {
	casos := []struct {
		env    map[string]string
		quiere bool
	}{
		{map[string]string{}, true},
		{map[string]string{"NO_COLOR": ""}, true},
		{map[string]string{"NO_COLOR": "1"}, false},
		{map[string]string{"NO_COLOR": "0"}, false},
		{map[string]string{"NO_COLOR": "false"}, false},
		{map[string]string{"TERM": "dumb"}, true},
	}
	for _, c := range casos {
		if got := tablero.ConColor(ttEnv(c.env)); got != c.quiere {
			t.Fatalf("CA-534: ConColor(%v) = %v, se esperaba %v", c.env, got, c.quiere)
		}
	}
}

// CA-534: ConUTF8: el primero no vacio de LC_ALL, LC_CTYPE y LANG nombra
// UTF-8 ("UTF-8" o "utf8", sin importar mayusculas), y HOOM_ASCII no es "1".
func TestCA534_ConUTF8(t *testing.T) {
	casos := []struct {
		env    map[string]string
		quiere bool
	}{
		{map[string]string{}, false},
		{map[string]string{"LANG": "es_AR.UTF-8"}, true},
		{map[string]string{"LANG": "en_US.utf8"}, true},
		{map[string]string{"LANG": "es_ES.Utf-8"}, true},
		{map[string]string{"LANG": "es_ES.UTF8"}, true},
		{map[string]string{"LC_CTYPE": "UTF-8"}, true},
		{map[string]string{"LANG": "C"}, false},
		{map[string]string{"LANG": "POSIX"}, false},
		{map[string]string{"LANG": "es_ES.ISO-8859-1"}, false},
		{map[string]string{"LC_ALL": "C", "LANG": "es_AR.UTF-8"}, false},
		{map[string]string{"LC_ALL": "", "LC_CTYPE": "en_US.UTF-8", "LANG": "C"}, true},
		{map[string]string{"LC_CTYPE": "C", "LANG": "es_AR.UTF-8"}, false},
		{map[string]string{"LC_ALL": "es_AR.UTF-8", "LC_CTYPE": "C"}, true},
		{map[string]string{"LANG": "es_AR.UTF-8", "HOOM_ASCII": "1"}, false},
		{map[string]string{"LANG": "es_AR.UTF-8", "HOOM_ASCII": "0"}, true},
		{map[string]string{"LANG": "es_AR.UTF-8", "HOOM_ASCII": ""}, true},
		{map[string]string{"LANG": "es_AR.UTF-8", "HOOM_ASCII": "si"}, true},
	}
	for _, c := range casos {
		if got := tablero.ConUTF8(ttEnv(c.env)); got != c.quiere {
			t.Fatalf("CA-534: ConUTF8(%v) = %v, se esperaba %v", c.env, got, c.quiere)
		}
	}
}

// CA-522, CA-524: ComandoEditor es $VISUAL, despues $EDITOR, despues vi
// (notepad en Windows), como lo eligen los verbos de la capa 1 (un valor de
// solo espacios no cuenta).
func TestCA524_ComandoEditor(t *testing.T) {
	casos := []struct {
		env    map[string]string
		goos   string
		quiere string
	}{
		{map[string]string{"VISUAL": "code -w", "EDITOR": "vim"}, "linux", "code -w"},
		{map[string]string{"EDITOR": "nano"}, "linux", "nano"},
		{map[string]string{"VISUAL": "", "EDITOR": "nano"}, "darwin", "nano"},
		{map[string]string{"VISUAL": "   ", "EDITOR": "hx"}, "linux", "hx"},
		{map[string]string{"VISUAL": " nvim "}, "linux", "nvim"},
		{map[string]string{}, "linux", "vi"},
		{map[string]string{}, "darwin", "vi"},
		{map[string]string{}, "windows", "notepad"},
		{map[string]string{"EDITOR": "code --wait"}, "windows", "code --wait"},
	}
	for _, c := range casos {
		if got := tablero.ComandoEditor(ttEnv(c.env), c.goos); got != c.quiere {
			t.Fatalf("CA-524: ComandoEditor(%v, %s) = %q, se esperaba %q", c.env, c.goos, got, c.quiere)
		}
	}
}

// CA-522, CA-524: ArgumentosEditor: "+<linea>" y el archivo para vi, vim,
// nvim, nano, emacs, micro y hx; "-g" y "<archivo>:<linea>" para code y
// cursor; el archivo solo con otro editor o sin linea. El editor es el
// nombre base de la primera palabra, sin ".exe".
func TestCA524_ArgumentosEditor(t *testing.T) {
	f := "/proyecto/.hoom/worktrees/x/internal/a b.go"
	casos := []struct {
		editor string
		linea  int
		quiere []string
	}{
		{"vi", 12, []string{"+12", f}},
		{"vim", 1, []string{"+1", f}},
		{"nvim", 3000, []string{"+3000", f}},
		{"nano", 7, []string{"+7", f}},
		{"emacs", 7, []string{"+7", f}},
		{"micro", 7, []string{"+7", f}},
		{"hx", 7, []string{"+7", f}},
		{"vim -u NONE", 7, []string{"+7", f}},
		{"/usr/local/bin/nvim", 3, []string{"+3", f}},
		{"nvim.exe", 3, []string{"+3", f}},
		{"code", 12, []string{"-g", f + ":12"}},
		{"code --wait", 12, []string{"-g", f + ":12"}},
		{"cursor", 5, []string{"-g", f + ":5"}},
		{"code.exe", 5, []string{"-g", f + ":5"}},
		{"/opt/bin/cursor -w", 5, []string{"-g", f + ":5"}},
		{"subl -w", 12, []string{f}},
		{"emacsclient", 12, []string{f}},
		{"notepad", 12, []string{f}},
		{"vimx", 12, []string{f}},
		{"vi", 0, []string{f}},
		{"code", 0, []string{f}},
	}
	for _, c := range casos {
		if got := tablero.ArgumentosEditor(c.editor, f, c.linea); !reflect.DeepEqual(got, c.quiere) {
			t.Fatalf("CA-524: ArgumentosEditor(%q, f, %d) = %q, se esperaba %q", c.editor, c.linea, got, c.quiere)
		}
	}
}

// CA-536 (guarda): los require de go.mod son exactamente gopkg.in/yaml.v3,
// golang.org/x/term y golang.org/x/sys.
func TestCA536_RequireDeGoMod(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		t.Fatalf("CA-536: no pude leer go.mod: %v", err)
	}
	var mods []string
	enBloque := false
	for _, l := range strings.Split(string(raw), "\n") {
		l = strings.TrimSpace(l)
		if i := strings.Index(l, "//"); i >= 0 {
			l = strings.TrimSpace(l[:i])
		}
		switch {
		case l == "require (":
			enBloque = true
		case enBloque && l == ")":
			enBloque = false
		case enBloque && l != "":
			mods = append(mods, strings.Fields(l)[0])
		case strings.HasPrefix(l, "require "):
			mods = append(mods, strings.Fields(l)[1])
		}
	}
	sort.Strings(mods)
	quiere := []string{"golang.org/x/sys", "golang.org/x/term", "gopkg.in/yaml.v3"}
	if !reflect.DeepEqual(mods, quiere) {
		t.Fatalf("CA-536: los require de go.mod son exactamente %q, son %q", quiere, mods)
	}
}

// ---------------------------------------------------------------- sin color y en ASCII

var ttSGR = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// ttPantallas recorre las vistas del contrato con cfg y devuelve cada
// pantalla con su nombre: la lista, la busqueda, el modo experto, las cinco
// pestanas, el menu, el formulario, la confirmacion, la de guardar, la ayuda,
// un aviso en la barra y la pantalla chica.
func ttPantallas(t *testing.T, cfg tablero.Config) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	foto := func(nombre string, m *tablero.Modelo) { out[nombre] = m.Pantalla() }
	m := ttModelo(cfg, ttTableroPrincipal())
	foto("lista", m)
	m.Tecla("x", ttAhora)
	foto("experto", m)
	m.Tecla("x", ttAhora)
	m.Tecla("/", ttAhora)
	ttEscribir(m, "inf")
	foto("busqueda", m)
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("w", ttAhora) // la primera tarjeta no tiene writer: aviso en la barra
	foto("aviso", m)
	m.Leer(tablero.Lectura{Ahora: ttAhora, Err: errors.New("disco roto")})
	foto("error de lectura", m)

	m = ttAbrirInfraAPI(t, cfg)
	for i, p := range tablero.Pestanas {
		m.Tecla(tablero.Tecla(string(rune('1'+i))), ttAhora)
		m.LeerDetalle(ttDetalleInfraAPI(ttInfraAPI(true)))
		foto("pestana "+string(p), m)
	}
	m.Tecla("1", ttAhora)
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	foto("menu", m)
	m.Tecla("x", ttAhora)
	foto("menu experto", m)
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("w", ttAhora)
	foto("formulario", m)
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("A", ttAhora)
	foto("confirmacion", m)
	m.Tecla(tablero.TeclaDer, ttAhora)
	foto("confirmacion firmar", m)
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("?", ttAhora)
	foto("ayuda", m)

	g := ttModelo(cfg, ttTablero(ttTarjetaCon(ttAccionGuardar(true, "", "internal/a.go", "b.go"))))
	g.Tecla("s", ttAhora)
	foto("guardar", g)

	c := cfg
	c.Ancho, c.Alto = 50, 10
	foto("chica", ttModelo(c, ttTableroPrincipal()))
	return out
}

// CA-534: sin color ninguna pantalla lleva una secuencia de escape (y cada
// una cumple la forma del contrato).
func TestCA534_SinColorNingunaSecuencia(t *testing.T) {
	for _, utf := range []bool{false, true} {
		cfg := ttConfig(100, 30, utf)
		for nombre, p := range ttPantallas(t, cfg) {
			if len(p) == 0 {
				t.Fatalf("CA-534: la pantalla %q (utf8=%v) esta vacia", nombre, utf)
			}
			for i, l := range p {
				if strings.Contains(l, "\x1b") {
					t.Fatalf("CA-534: sin color, la linea %d de %q (utf8=%v) no lleva secuencias: %q", i+1, nombre, utf, l)
				}
				if strings.TrimRight(l, " ") != l {
					t.Fatalf("CA-534: la linea %d de %q termina en espacios: %q", i+1, nombre, l)
				}
			}
		}
	}
}

// CA-534: con color la pantalla lleva secuencias SGR (titulos en negrita, la
// linea de la interrumpida en rojo), y sin ellas es la misma pantalla que
// sin color.
func TestCA534_ConColorEsLaMismaPantallaConSGR(t *testing.T) {
	plano := ttPantallas(t, ttConfig(100, 30, true))
	cfg := ttConfig(100, 30, true)
	cfg.Color = true
	color := ttPantallas(t, cfg)
	conSGR := 0
	for nombre, p := range color {
		q := plano[nombre]
		if len(p) != len(q) {
			t.Fatalf("CA-534: la pantalla %q con color tiene %d lineas, sin color %d", nombre, len(p), len(q))
		}
		for i := range p {
			if ttSGR.MatchString(p[i]) {
				conSGR++
			}
			if sin := ttSGR.ReplaceAllString(p[i], ""); strings.TrimRight(sin, " ") != q[i] {
				t.Fatalf("CA-534: la linea %d de %q con color, sin sus SGR, es la de sin color:\n con %q\n sin %q", i+1, nombre, p[i], q[i])
			}
			if strings.Contains(ttSGR.ReplaceAllString(p[i], ""), "\x1b") {
				t.Fatalf("CA-534: con color las unicas secuencias son SGR: %q", p[i])
			}
		}
	}
	if conSGR == 0 {
		t.Fatalf("CA-534: con color alguna linea lleva una secuencia SGR")
	}
	lista := 0
	for _, l := range color["lista"] {
		if ttSGR.MatchString(l) {
			lista++
		}
	}
	if lista == 0 {
		t.Fatalf("CA-534: con color la lista lleva secuencias SGR (titulos en negrita, la interrumpida en rojo)")
	}
}

// CA-534: en ASCII ninguna pantalla lleva los signos UTF-8 del contrato ni
// las palabras con acento de la TUI (la barra 2 y la ayuda quedan afuera: sus
// flechas no estan en la tabla de signos).
func TestCA534_EnASCIISinSignosUTF8(t *testing.T) {
	const signos = "●○✋✓✗⚑▸▾─┌┐└┘│‹›▌→…ÓóÍíÁáÉéÚú"
	for nombre, p := range ttPantallas(t, ttConfig(100, 30, false)) {
		if nombre == "ayuda" {
			continue
		}
		for i, l := range p {
			if i == len(p)-1 && nombre != "chica" {
				continue // la barra 2
			}
			if j := strings.IndexAny(l, signos); j >= 0 {
				r, _ := utf8.DecodeRuneInString(l[j:])
				t.Fatalf("CA-534: en ASCII la linea %d de %q no lleva %q: %q", i+1, nombre, string(r), l)
			}
		}
	}
}

// CA-518, caso limite: titulos y plain con emojis y caracteres de ancho
// doble pueden desalinear, pero la pantalla sigue siendo Alto lineas de a lo
// sumo Ancho runas.
func TestCA518_EmojisYAnchoDoble(t *testing.T) {
	c := ttCard("cohete-🚀-漢字", "Título 🚀 con 漢字 y ñandú", boardcmd.ColWriter, "plain con 🚀🚀🚀 y 漢字漢字漢字 "+strings.Repeat("ｗ", 80),
		ttSeg("s", "s", boardcmd.SegHecho, "d"))
	for _, utf := range []bool{false, true} {
		m := ttModelo(ttConfig(80, 24, utf), ttTablero(c))
		ttForma(t, "CA-518", m, 80, 24, false)
		m.Tecla(tablero.TeclaEnter, ttAhora)
		ttForma(t, "CA-518", m, 80, 24, false)
		if l := ttLinea(m, 1); !strings.HasPrefix(l, " cohete-🚀-漢字 · Título 🚀") {
			t.Fatalf("CA-518: el texto de la tarjeta se imprime como es: %q", l)
		}
	}
}
