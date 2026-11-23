// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// gitx (CA-775): los archivos del cambio y sus lineas se miden desde el
// merge-base contra el arbol de trabajo; un commit nuevo de la base no suma,
// un borrado suma y cuenta para el riesgo, un sin seguimiento suma sus
// lineas (una ultima sin salto cuenta), un binario suma 0, un renombre son
// dos archivos (--no-renames); y el rango de una review (Comparacion.Lineas)
// mide solo lo de la rama. Escritos desde el spec, las aclaraciones de la
// ronda 0 y las firmas publicas, sin leer la implementacion.
package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/riesgo"
)

func retyGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func retyEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// retyLineas es un texto de n lineas, cada una con salto.
func retyLineas(n int, marca string) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(marca)
		b.WriteString(" ")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteString("\n")
	}
	return b.String()
}

func retyRepo(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	retyGit(t, dir, "init", "-q", "-b", "main")
	gittest.ApagarMantenimiento(t, dir)
	retyGit(t, dir, "config", "user.email", "test@hoom.dev")
	retyGit(t, dir, "config", "user.name", "hoom test")
	return dir
}

func retyCommit(t *testing.T, dir, msg string) string {
	t.Helper()
	retyGit(t, dir, "add", "-A")
	retyGit(t, dir, "commit", "-q", "--allow-empty", "-m", msg)
	return retyGit(t, dir, "rev-parse", "HEAD")
}

// retyEscenario es main con archivos, una rama tarea que cambia, borra,
// renombra y agrega, una base que avanza despues, y un arbol de trabajo con
// cambios sin commitear, uno preparado, sin seguimiento (texto sin salto
// final, binario, vacio, en un directorio nuevo) e ignorados, si conArbol.
func retyEscenario(t *testing.T, conArbol bool) string {
	t.Helper()
	dir := retyRepo(t)
	var a strings.Builder
	for i := 1; i <= 10; i++ {
		a.WriteString("linea " + strings.Repeat("a", i) + "\n")
	}
	retyEscribir(t, dir, "a.go", a.String())
	retyEscribir(t, dir, "internal/auth/viejo.go", retyLineas(5, "auth"))
	retyEscribir(t, dir, "mover.go", retyLineas(3, "mover"))
	retyEscribir(t, dir, "logo.png", "\x89PNG\x00\x01\x02")
	retyEscribir(t, dir, "keep.txt", "uno\n")
	retyEscribir(t, dir, ".gitignore", "ignorado.log\n/build/\n")
	retyCommit(t, dir, "inicial")
	retyGit(t, dir, "checkout", "-q", "-b", "tarea")
	retyEscribir(t, dir, "a.go", strings.Replace(a.String(), "linea aaaaa\n", "cambiada\n", 1))
	retyEscribir(t, dir, "nuevo.go", retyLineas(3, "nuevo"))
	retyEscribir(t, dir, ".hoom/verdicts/20261120T100000_abcdef.json", retyLineas(4, "{"))
	if err := os.Remove(filepath.Join(dir, "internal", "auth", "viejo.go")); err != nil {
		t.Fatal(err)
	}
	retyGit(t, dir, "mv", "mover.go", "movido.go")
	retyEscribir(t, dir, "logo.png", "\x89PNG\x00\x01\x03")
	retyCommit(t, dir, "la tarea")
	// La base avanza despues: nada de esto es del cambio.
	retyGit(t, dir, "checkout", "-q", "main")
	retyEscribir(t, dir, "base_nueva.go", retyLineas(50, "base"))
	retyEscribir(t, dir, "keep.txt", "uno\ndos\n")
	retyCommit(t, dir, "la base avanza")
	retyGit(t, dir, "checkout", "-q", "tarea")
	if !conArbol {
		return dir
	}
	// El arbol de trabajo.
	retyEscribir(t, dir, "nuevo.go", retyLineas(5, "nuevo"))
	retyEscribir(t, dir, "staged.go", "s1\ns2\n")
	retyGit(t, dir, "add", "staged.go")
	retyEscribir(t, dir, "sin_seguir.go", "x\ny\nz")
	retyEscribir(t, dir, "datos.bin", "ab\x00cd\nef\n")
	retyEscribir(t, dir, "vacio.go", "")
	retyEscribir(t, dir, "dir/nuevo/a_test.go", "t1\nt2\nt3\nt4\n")
	retyEscribir(t, dir, "ignorado.log", retyLineas(9, "log"))
	retyEscribir(t, dir, "build/out.go", retyLineas(9, "build"))
	return dir
}

// CA-775: los archivos del cambio y sus lineas, exactos.
func TestCA775_LineasDelCambioDesdeElMergeBase(t *testing.T) {
	const ca = "CA-775"
	dir := retyEscenario(t, true)
	got, err := LineasDelCambio(dir, "main")
	if err != nil {
		t.Fatalf("%s: LineasDelCambio: %v", ca, err)
	}
	want := map[string]int{
		"a.go":                   2, // una linea cambiada: 1 insercion + 1 borrado
		"nuevo.go":               5, // lo commiteado y lo de despues, contra el arbol
		"internal/auth/viejo.go": 5, // borrado: sus lineas borradas
		"mover.go":               3, // --no-renames: el renombre son dos archivos
		"movido.go":              3,
		"logo.png":               0, // binario
		".hoom/verdicts/20261120T100000_abcdef.json": 4, // .hoom/ no se excluye
		"staged.go":           2, // preparado sin commitear
		"sin_seguir.go":       3, // sin seguimiento; la ultima sin salto cuenta
		"datos.bin":           0, // sin seguimiento con NUL: binario
		"vacio.go":            0,
		"dir/nuevo/a_test.go": 4, // en un directorio nuevo: el archivo, no el directorio
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: LineasDelCambio\n  es %v\nfue %v", ca, want, got)
	}
	for _, fuera := range []string{"base_nueva.go", "keep.txt", "ignorado.log", "build/out.go", "build/", "dir/", "dir/nuevo/"} {
		if _, ok := got[fuera]; ok {
			t.Fatalf("%s: %s no es del cambio (la base, un ignorado o un directorio): %v", ca, fuera, got)
		}
	}
	// Con las clases: el borrado suma a producto y sube el riesgo.
	c := riesgo.Clasificador{Rutas: riesgo.RutasBase(), Pruebas: []string{".hoom/**", "**/*_test.go"}}
	if tam := c.Medir(got); tam != (riesgo.Tamano{Producto: 23, Pruebas: 4, Hoom: 4}) {
		t.Fatalf("%s: el tamano del cambio es producto 23, pruebas 4, hoom 4: %+v", ca, tam)
	}
	var files []string
	for f := range got {
		files = append(files, f)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	e := c.Evaluar(riesgo.Tarjeta{Tarea: "tarea", Riesgo: "bajo"}, files)
	if e != (riesgo.Evaluacion{Declarado: "bajo", Efectivo: "alto", Motivo: "el cambio toca internal/auth/viejo.go: riesgo alto"}) {
		t.Fatalf("%s: un archivo borrado cuenta para el riesgo: %+v", ca, e)
	}
}

// CA-775: con la base integrada (un merge de main en la rama), lo que trajo
// la base sigue sin contar.
func TestCA775_LaBaseIntegradaNoSuma(t *testing.T) {
	const ca = "CA-775"
	dir := retyEscenario(t, false)
	retyGit(t, dir, "merge", "-q", "--no-edit", "main")
	got, err := LineasDelCambio(dir, "main")
	if err != nil {
		t.Fatalf("%s: LineasDelCambio: %v", ca, err)
	}
	if _, ok := got["base_nueva.go"]; ok {
		t.Fatalf("%s: un commit nuevo de la base no suma, aunque la rama lo integre: %v", ca, got)
	}
	if _, ok := got["keep.txt"]; ok {
		t.Fatalf("%s: keep.txt lo cambio la base: %v", ca, got)
	}
	if got["nuevo.go"] != 3 || got["a.go"] != 2 || got["internal/auth/viejo.go"] != 5 {
		t.Fatalf("%s: lo de la rama sigue: %v", ca, got)
	}
}

// CA-775: sin cambios es un mapa vacio; fuera de un repo, o con una base
// que no existe, es un error.
func TestCA775_SinCambiosYErrores(t *testing.T) {
	const ca = "CA-775"
	dir := retyRepo(t)
	retyEscribir(t, dir, "a.go", "a\n")
	retyCommit(t, dir, "inicial")
	got, err := LineasDelCambio(dir, "main")
	if err != nil || len(got) != 0 {
		t.Fatalf("%s: sin cambios no hay archivos: %v %v", ca, got, err)
	}
	if _, err := LineasDelCambio(dir, "no-existe"); err == nil {
		t.Fatalf("%s: una base que no existe es un error", ca)
	}
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(fuera))
	if _, err := LineasDelCambio(fuera, "main"); err == nil {
		t.Fatalf("%s: fuera de un repo es un error", ca)
	}
}

// CA-775 y CA-783: Comparacion.Lineas mide solo lo que la rama cambio desde
// desde, archivo por archivo.
func TestCA775_LasLineasDeUnRango(t *testing.T) {
	const ca = "CA-775"
	dir := retyRepo(t)
	retyEscribir(t, dir, "a.go", retyLineas(3, "a"))
	retyCommit(t, dir, "inicial")
	retyGit(t, dir, "checkout", "-q", "-b", "tarea")
	retyEscribir(t, dir, "grande.go", retyLineas(450, "grande"))
	desde := retyCommit(t, dir, "lo ya revisado")
	retyEscribir(t, dir, "rango.go", retyLineas(4, "rango"))
	retyEscribir(t, dir, "rango_test.go", retyLineas(6, "prueba"))
	retyEscribir(t, dir, "logo.png", "\x89PNG\x00")
	hasta := retyCommit(t, dir, "la correccion")
	mb := retyGit(t, dir, "merge-base", "main", hasta)
	c, err := SinLaBase(dir, desde, hasta, mb)
	if err != nil {
		t.Fatalf("%s: SinLaBase: %v", ca, err)
	}
	defer c.Cerrar()
	got, err := c.Lineas(dir, hasta)
	if err != nil {
		t.Fatalf("%s: Comparacion.Lineas: %v", ca, err)
	}
	if want := map[string]int{"rango.go": 4, "rango_test.go": 6, "logo.png": 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("%s: las lineas del rango son %v, fueron %v", ca, want, got)
	}
}
