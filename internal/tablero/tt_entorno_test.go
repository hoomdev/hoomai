// El entorno (CA-516, CA-522, CA-524, CA-534, CA-536; en la v2 CA-575 y
// CA-576): ParseArgs, ConColor, ConUTF8, ConMouse, ComandoEditor,
// ArgumentosEditor, la pantalla sin color y en ASCII en todos los lugares de
// la v2 (paneles, busqueda, modales, la ayuda, la pantalla chica), y los
// require de go.mod.
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
	"time"
	"unicode/utf8"

	"github.com/hoomdev/hoomai/internal/cliargs"
	"github.com/hoomdev/hoomai/internal/runcmd"
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

// CA-575: ConMouse es false solo con HOOM_MOUSE "0"; cualquier otro valor,
// o ninguno, es si.
func TestCA575_ConMouse(t *testing.T) {
	casos := []struct {
		env    map[string]string
		quiere bool
	}{
		{map[string]string{}, true},
		{map[string]string{"HOOM_MOUSE": "0"}, false},
		{map[string]string{"HOOM_MOUSE": "1"}, true},
		{map[string]string{"HOOM_MOUSE": ""}, true},
		{map[string]string{"HOOM_MOUSE": "no"}, true},
		{map[string]string{"HOOM_MOUSE": " 0"}, true},
		{map[string]string{"HOOM_MOUSE": "00"}, true},
	}
	for _, c := range casos {
		if got := tablero.ConMouse(ttEnv(c.env)); got != c.quiere {
			t.Fatalf("CA-575: ConMouse(%v) = %v, se esperaba %v", c.env, got, c.quiere)
		}
	}
}

// ---------------------------------------------------------------- sin color y en ASCII (CA-534, CA-576)

// ttPantallas recorre los lugares de la v2 con cfg y devuelve cada pantalla
// con su nombre: los paneles con el foco en cada uno, [0] de una tarjeta, de
// lo que corre, de un hallazgo y del proyecto, la busqueda, el modo experto,
// un aviso, un error de lectura, el modal de acciones, el de pedir (con una
// lista abierta), el de firma, el de guardar, la ayuda y la pantalla chica.
func ttPantallas(t *testing.T, cfg tablero.Config) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	foto := func(nombre string, m *tablero.Modelo) { out[nombre] = m.Pantalla() }
	m := ttModeloDorado(cfg)
	foto("paneles", m)
	for _, k := range []tablero.Tecla{"1", "3", "4", "0", "2"} {
		m.Tecla(k, ttAhora)
		foto("foco "+string(k), m)
	}
	m.Tecla(tablero.TeclaEnter, ttAhora)
	foto("[0] de una tarjeta", m)
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("3", ttAhora)
	m.LeerDetalle(tablero.Detalle{Slug: "infra-api", Sobre: tablero.Sobre{EnvelopeID: "env-api", Eventos: []runcmd.Event{ttEvento(-3, "text", "leyendo")}}})
	foto("[0] de lo que corre", m)
	m.Tecla("4", ttAhora)
	m.LeerArchivo(tablero.Archivo{Ruta: "internal/api/router.go", Linea: 41, Desde: 40, Lineas: []string{"a", "b", "c"}})
	foto("[0] de un hallazgo", m)
	m.Tecla("1", ttAhora)
	m.Tecla(tablero.TeclaEnter, ttAhora)
	foto("[0] del proyecto", m)
	m.Tecla("2", ttAhora)
	m.Tecla("x", ttAhora)
	foto("experto", m)
	m.Tecla("x", ttAhora)
	m.Tecla("/", ttAhora)
	ttEscribir(m, "inf")
	foto("busqueda", m)
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("w", ttAhora) // nuc-tenancy no tiene writer: aviso
	foto("aviso", m)
	m.Leer(tablero.Lectura{Ahora: ttAhora, Err: errors.New("disco roto")})
	foto("error de lectura", m)
	m.Tecla(tablero.TeclaEspacio, ttAhora)
	foto("acciones", m)
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("A", ttAhora)
	foto("firma", m)
	m.Tecla(tablero.TeclaDer, ttAhora)
	foto("firma con la accion elegida", m)
	m.Tecla(tablero.TeclaEsc, ttAhora)
	m.Tecla("?", ttAhora)
	foto("ayuda", m)

	p := ttModelo(cfg, ttTablero(ttTarjetaCon(ttAccionWriter())))
	p.Tecla("w", ttAhora)
	foto("pedir", p)
	p.Tecla(tablero.TeclaTab, ttAhora)
	p.Tecla(tablero.TeclaTab, ttAhora)
	p.Tecla(tablero.TeclaEnter, ttAhora) // la lista de Modelo
	foto("pedir con la lista abierta", p)

	g := ttModelo(cfg, ttTablero(ttTarjetaCon(ttAccionGuardar(true, "", "internal/a.go", "b.go"))))
	g.Tecla("s", ttAhora)
	foto("guardar", g)

	c := cfg
	c.Ancho, c.Alto = 50, 10
	foto("chica", ttModelo(c, ttTableroPrincipal()))
	u := cfg
	u.Ancho, u.Alto = 70, 24
	foto("una columna", ttModeloDorado(u))
	return out
}

// CA-534, CA-576: sin color ninguna pantalla lleva una secuencia de escape
// (y cada una cumple la forma del contrato: Alto lineas de a lo sumo Ancho
// celdas, sin espacios al final).
func TestCA534_CA576_SinColorNingunaSecuencia(t *testing.T) {
	for _, utf := range []bool{false, true} {
		cfg := ttConfig(100, 30, utf)
		for nombre, p := range ttPantallas(t, cfg) {
			if len(p) == 0 {
				t.Fatalf("CA-576: la pantalla %q (utf8=%v) esta vacia", nombre, utf)
			}
			for i, l := range p {
				if strings.Contains(l, "\x1b") {
					t.Fatalf("CA-576: sin color, la linea %d de %q (utf8=%v) no lleva secuencias: %q", i+1, nombre, utf, l)
				}
				if strings.TrimRight(l, " ") != l {
					t.Fatalf("CA-576: la linea %d de %q termina en espacios: %q", i+1, nombre, l)
				}
			}
		}
	}
}

// ttSoloConColor es lo que el titulo de [4] suma con color, con los signos
// de linea que le siguen en el borde: " · <k> high" y su relleno.
var ttSoloConColor = regexp.MustCompile(` · [0-9]+ high[-─=═]+`)

// CA-534, CA-576: con color la pantalla lleva secuencias SGR (el foco, los
// titulos de seccion, la seleccion), y sin ellas es la misma pantalla que
// sin color; las unicas secuencias son SGR, cada pieza con estilo cerrada
// con "\x1b[0m".
func TestCA534_CA576_ConColorEsLaMismaPantallaConSGR(t *testing.T) {
	plano := ttPantallas(t, ttConfig(100, 30, true))
	cfg := ttConfig(100, 30, true)
	cfg.Color = true
	color := ttPantallas(t, cfg)
	conSGR := 0
	for nombre, p := range color {
		q := plano[nombre]
		if len(p) != len(q) {
			t.Fatalf("CA-576: la pantalla %q con color tiene %d lineas, sin color %d", nombre, len(p), len(q))
		}
		for i := range p {
			if ttSGR.MatchString(p[i]) {
				conSGR++
				if !strings.HasSuffix(strings.TrimRight(p[i], " "), "\x1b[0m") && strings.Count(p[i], "\x1b[0m") == 0 {
					t.Fatalf("CA-576: con color cada pieza con estilo se cierra con \\x1b[0m: %q", p[i])
				}
			}
			sin := ttSinSGR(p[i])
			// el titulo de [4] suma " · <k> high" solo con color (spec, "[4]
			// Hallazgos"; TestCA559_CA524_FilasDeHallazgos): fuera de eso, la
			// misma linea
			if !strings.Contains(q[i], " high") {
				sin = ttSoloConColor.ReplaceAllStringFunc(sin, func(x string) string {
					r := []rune(x)
					return strings.Repeat(string(r[len(r)-1]), ttCeldas(x))
				})
			}
			if strings.TrimRight(sin, " ") != q[i] {
				t.Fatalf("CA-576: la linea %d de %q con color, sin sus SGR, es la de sin color:\n con %q\n sin %q", i+1, nombre, p[i], q[i])
			}
			if strings.Contains(ttSinSGR(p[i]), "\x1b") {
				t.Fatalf("CA-576: con color las unicas secuencias son SGR: %q", p[i])
			}
		}
	}
	if conSGR == 0 {
		t.Fatalf("CA-576: con color alguna linea lleva una secuencia SGR")
	}
	for _, nombre := range []string{"paneles", "pedir", "firma", "ayuda"} {
		n := 0
		for _, l := range color[nombre] {
			if strings.Contains(l, "\x1b[1;32m") {
				n++
			}
		}
		if n == 0 {
			t.Fatalf("CA-551, CA-565: con color, en %q el borde del foco (o del modal) va en verde y negrita (1;32)", nombre)
		}
	}
}

// CA-576: en ASCII ninguna pantalla lleva los signos UTF-8 del contrato
// (bordes simples y del foco, la seleccion, Hecho, la tira, los criterios,
// las flechas de los selectores, el cursor, el corte, las flechas de la
// linea de teclas y de la ayuda) ni las palabras con acento de la TUI.
func TestCA576_EnASCIISinSignosUTF8(t *testing.T) {
	const signos = "●○✋✓✗⚑▸▾─│┌┐└┘═║╔╗╚╝‹›▌…↑↓←→ÓóÍíÁáÉéÚú¿"
	for nombre, p := range ttPantallas(t, ttConfig(100, 30, false)) {
		for i, l := range p {
			if j := strings.IndexAny(l, signos); j >= 0 {
				r, _ := utf8.DecodeRuneInString(l[j:])
				t.Fatalf("CA-576: en ASCII la linea %d de %q no lleva %q: %q", i+1, nombre, string(r), l)
			}
		}
	}
}

// CA-576: con UTF-8 el foco usa la linea doble (╔ ═ ╗ ║ ╚ ╝), los otros
// paneles la simple, la seleccion ▸ y Hecho cerrada ▸; en ASCII, # = para
// el foco, + - | para los otros y > para la seleccion y Hecho.
func TestCA576_SignosDeLosDosModos(t *testing.T) {
	m := ttModeloDorado(ttConfig(120, 32, true))
	cs := ttCajas(m)
	if c := cs["[2]"]; !strings.HasPrefix(c.Arriba, "╔[2] Tarjetas═") || !strings.HasSuffix(c.Arriba, "╗") ||
		!strings.HasPrefix(c.Abajo, "╚") || !strings.HasSuffix(c.Abajo, "╝") {
		t.Fatalf("CA-576: con UTF-8 el foco va en linea doble: %q / %q", c.Arriba, c.Abajo)
	}
	if c := cs["[1]"]; !strings.HasPrefix(c.Arriba, "┌[1] Proyecto─") || !strings.HasSuffix(c.Arriba, "┐") || !strings.HasPrefix(c.Abajo, "└") {
		t.Fatalf("CA-576: con UTF-8 los otros paneles van en linea simple: %q", c.Arriba)
	}
	if !ttTiene(m, "║ ▸ nuc-tenancy") || !ttTiene(m, " HECHO 3 ▸") || !ttTiene(m, "NECESITA TU DECISIÓN") {
		t.Fatalf("CA-576: con UTF-8, ▸ en la seleccion y en Hecho cerrada, y los titulos con acento:\n%s", ttPantalla(m))
	}
	a := ttModeloDorado(ttConfig(120, 32, false))
	cs = ttCajas(a)
	if c := cs["[2]"]; !strings.HasPrefix(c.Arriba, "#[2] Tarjetas=") || !strings.HasSuffix(c.Arriba, "#") || !strings.HasPrefix(c.Abajo, "#=") {
		t.Fatalf("CA-576: en ASCII el borde del foco usa # y =: %q / %q", c.Arriba, c.Abajo)
	}
	if c := cs["[1]"]; !strings.HasPrefix(c.Arriba, "+[1] Proyecto-") || !strings.HasSuffix(c.Arriba, "+") || !strings.HasPrefix(c.Dentro[0], " ") {
		t.Fatalf("CA-576: en ASCII los otros paneles usan + - |: %q", c.Arriba)
	}
	if !ttTiene(a, "# > nuc-tenancy") || !ttTiene(a, " HECHO 3 >") || !ttTiene(a, "NECESITA TU DECISION") {
		t.Fatalf("CA-576: en ASCII, > en la seleccion y en Hecho cerrada, y los titulos sin acento:\n%s", ttPantalla(a))
	}
	// guarda: el reloj no mueve nada de esto
	a.Tic(ttAhora.Add(time.Second))
	if !ttTiene(a, "# > nuc-tenancy") {
		t.Fatalf("CA-576: tras un Tic la seleccion sigue:\n%s", ttPantalla(a))
	}
}
