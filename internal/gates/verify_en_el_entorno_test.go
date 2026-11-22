//go:build !windows

// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre
// gates.Run, con la plantilla de .hoom/local.yaml ya leida
// (Options.Local) y un prefijo falso: un `ddev` que el test pone primero en
// el PATH, que anota su directorio y sus argumentos y corre `sh -c` con el
// ultimo. Nada de docker ni de ddev de verdad.
//
//   - CA-760: exit 127 es error con NotaNoEncontrado y la ultima linea no
//     en blanco de la salida; con prefijo y sin el; un ddev que no esta en
//     el host da 127 igual que un php que no esta;
//   - CA-761: exit 126 es error con NotaNoEjecutable; cualquier otro exit
//     distinto de 0 es fail con su codigo (una senal sin timeout incluida);
//     el timeout sigue siendo error con -1;
//   - CA-764: un gate con la plantilla corre `sh -c <Linea>` en el mismo
//     directorio; los de en_el_host corren como hoy; Command es el comando
//     del gate; Entorno dice donde corrio cada uno (aclaracion 2);
//   - CA-767: ExigirEncierro: la nota de sin encierro, y con completo o
//     parcial el gate con prefijo no corre (aclaraciones 8, 9 y 10).
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc del
// paquete, sin leer la implementacion.
package gates

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/entorno"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- fixtures

// veeQ es s como una palabra de sh entre comillas simples.
func veeQ(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// veeFalso pone primero en el PATH un prefijo falso llamado nombre: anota
// en log (fuera del proyecto) su directorio y sus argumentos, separados por
// NUL, y corre `sh -c` con su ultimo argumento, con VEE_DENTRO=1 para que
// el gate sepa que corrio "dentro". Devuelve la carpeta del log.
func veeFalso(t *testing.T, nombre string) string {
	t.Helper()
	bin, log := t.TempDir(), t.TempDir()
	script := "#!/bin/sh\n" +
		"log=" + veeQ(log) + "\n" +
		"{ printf '%s\\000' \"$(pwd -P)\"; for a do printf '%s\\000' \"$a\"; done; } > \"$log/.tmp.$$\"\n" +
		"mv \"$log/.tmp.$$\" \"$log/llamada.$$\"\n" +
		"for ultimo do :; done\n" +
		"VEE_DENTRO=1\nexport VEE_DENTRO\n" +
		"exec sh -c \"$ultimo\"\n"
	if err := os.WriteFile(filepath.Join(bin, nombre), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

// veeLlamada es una invocacion del prefijo falso.
type veeLlamada struct {
	dir  string
	args []string
}

func veeLlamadas(t *testing.T, log string) []veeLlamada {
	t.Helper()
	ents, err := os.ReadDir(log)
	if err != nil {
		t.Fatal(err)
	}
	var out []veeLlamada
	for _, e := range ents {
		if !strings.HasPrefix(e.Name(), "llamada.") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(log, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		partes := strings.Split(strings.TrimSuffix(string(b), "\x00"), "\x00")
		out = append(out, veeLlamada{dir: partes[0], args: partes[1:]})
	}
	sort.Slice(out, func(i, j int) bool { return strings.Join(out[i].args, " ") < strings.Join(out[j].args, " ") })
	return out
}

// veeDirReal es un directorio temporal sin symlinks en su ruta.
func veeDirReal(t *testing.T) string {
	t.Helper()
	d, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// veeManifest arma un manifest en dir con un gate requerido por cada
// entrada de cmds.
func veeManifest(dir string, cmds map[string]string) *manifest.Manifest {
	m := &manifest.Manifest{Dir: dir, Gates: map[string]manifest.Gate{}}
	for nombre, c := range cmds {
		c, req := c, true
		m.Gates[nombre] = manifest.Gate{Cmd: &c, Required: &req}
	}
	return m
}

func veeRun(m *manifest.Manifest, opt Options) []verdict.GateResult {
	if opt.Timeout == 0 {
		opt.Timeout = time.Minute
	}
	if opt.TailSize == 0 {
		opt.TailSize = 4096
	}
	return Run(m, gitx.Info{}, opt)
}

func veeGate(t *testing.T, res []verdict.GateResult, nombre string) verdict.GateResult {
	t.Helper()
	for _, r := range res {
		if r.Name == nombre {
			return r
		}
	}
	t.Fatalf("fixture: Run no devolvio el gate %q: %+v", nombre, res)
	return verdict.GateResult{}
}

// veeUnGate corre un solo gate "test" con cmd en un directorio nuevo.
func veeUnGate(t *testing.T, cmd string, opt Options) verdict.GateResult {
	t.Helper()
	return veeGate(t, veeRun(veeManifest(veeDirReal(t), map[string]string{"test": cmd}), opt), "test")
}

// veePlantilla es la de ddev del perfil laravel.
const veePlantilla = "ddev exec -d /var/www/html/{rel} sh -c {cmd}"

func veeAjuste(ejecutar, rel string, enElHost ...string) *entorno.Ajuste {
	return &entorno.Ajuste{Existe: true, Ejecutar: ejecutar, Rel: rel, EnElHost: enElHost, SHA256: strings.Repeat("ab", 32)}
}

// veeUltimaLinea es la ultima linea no en blanco de s, sin blancos al
// borde ("" si no hay).
func veeUltimaLinea(s string) string {
	lineas := strings.Split(s, "\n")
	for i := len(lineas) - 1; i >= 0; i-- {
		if l := strings.TrimSpace(lineas[i]); l != "" {
			return l
		}
	}
	return ""
}

// veeNota es la nota que fija el spec para un 127 o un 126 con salida s.
func veeNota(constante, s string) string {
	if l := veeUltimaLinea(s); l != "" {
		return constante + ": " + l
	}
	return constante
}

// ---------------------------------------------------------------- CA-760

// CA-760: los textos de las notas son los del spec.
func TestCA760_LosTextosDeLasNotas(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{NotaNoEncontrado, "no pudo correr: comando no encontrado (exit 127)"},
		{NotaNoEjecutable, "no pudo correr: el comando no se puede ejecutar (exit 126)"},
		{NotaSinEncierroConPrefijo, "sin encierro: corre con el prefijo de .hoom/local.yaml"},
		{fmt.Sprintf(NotaExigirConPrefijo, "completo"), "el prefijo de .hoom/local.yaml no corre encerrado y encierro.exigir es completo"},
	} {
		if c.got != c.want {
			t.Fatalf("CA-760: el texto es %q; es %q", c.want, c.got)
		}
	}
}

// CA-760: exit 127 es error con exit_code 127 y la nota NotaNoEncontrado,
// ": " y la ultima linea no en blanco de la salida, sin blancos al borde; la
// constante sola cuando no hay tal linea. Nunca fail.
func TestCA760_Exit127EsErrorConLaUltimaLineaNoEnBlanco(t *testing.T) {
	for _, c := range []struct{ nombre, cmd, nota string }{
		{"exit 127 sin salida", "exit 127", NotaNoEncontrado},
		{"solo blancos", "printf '\\n  \\n\\t\\n\\n'; exit 127", NotaNoEncontrado},
		{"ultima linea con blancos al borde", "printf 'primera\\n   segunda  \\t\\n\\n   \\n'; exit 127", NotaNoEncontrado + ": segunda"},
		{"ultima linea sin salto final", "printf 'a\\nb'; exit 127", NotaNoEncontrado + ": b"},
		{"en stderr", "echo 'por stdout'; echo '  por stderr  ' >&2; exit 127", NotaNoEncontrado + ": por stderr"},
		{"con CRLF", "printf 'x\\r\\n'; exit 127", NotaNoEncontrado + ": x"},
	} {
		r := veeUnGate(t, c.cmd, Options{})
		if r.Status != verdict.StatusError || r.ExitCode != 127 || r.Notes != c.nota {
			t.Fatalf("CA-760: %s: error, 127, nota %q: %+v", c.nombre, c.nota, r)
		}
	}
}

// CA-760 y caso limite "el comando no esta": un comando que sh no encuentra,
// uno que env no encuentra y una ruta que no existe son error con 127, y la
// nota nombra el comando (la ultima linea de la salida, donde sh y env lo
// nombran).
func TestCA760_ComandoQueNoEstaEsErrorQueLoNombra(t *testing.T) {
	for _, cmd := range []string{
		"vee-no-existe-760 --version",
		"echo antes; vee-no-existe-760",
		"env vee-no-existe-760",
		"./vee-no-existe-760/bin/php artisan test",
	} {
		r := veeUnGate(t, cmd, Options{})
		if r.Status != verdict.StatusError || r.ExitCode != 127 {
			t.Fatalf("CA-760: %q: error con 127, nunca fail: %+v", cmd, r)
		}
		if r.Notes != veeNota(NotaNoEncontrado, r.OutputTail) || !strings.Contains(r.Notes, "vee-no-existe-760") {
			t.Fatalf("CA-760: %q: la nota es %q y nombra el comando: %q (salida %q)", cmd, veeNota(NotaNoEncontrado, r.OutputTail), r.Notes, r.OutputTail)
		}
		if r.Command != cmd {
			t.Fatalf("CA-760: %q: Command es el comando: %q", cmd, r.Command)
		}
	}
}

// CA-760 con la plantilla: un 127 de adentro del contenedor (el comando que
// no esta alla) es el 127 del gate; un ddev que no esta en el host da 127
// igual que un php que no esta.
func TestCA760_Exit127ConPrefijoDeAdentroYDelHost(t *testing.T) {
	log := veeFalso(t, "ddev")
	local := veeAjuste(veePlantilla, ".")
	r := veeUnGate(t, "echo 'php: no esta'; exit 127", Options{Local: local})
	if r.Status != verdict.StatusError || r.ExitCode != 127 || r.Notes != NotaNoEncontrado+": php: no esta" || r.Entorno != verdict.EntornoPrefijo {
		t.Fatalf("CA-760: el 127 de adentro del contenedor es el del gate: %+v", r)
	}
	r = veeUnGate(t, "vee-no-existe-760 test", Options{Local: local})
	if r.Status != verdict.StatusError || r.ExitCode != 127 || !strings.Contains(r.Notes, "vee-no-existe-760") || r.Notes != veeNota(NotaNoEncontrado, r.OutputTail) {
		t.Fatalf("CA-760: un comando que no esta adentro del contenedor: %+v", r)
	}
	if n := len(veeLlamadas(t, log)); n != 2 {
		t.Fatalf("CA-760: los dos gates corrieron con el prefijo: %d llamadas", n)
	}

	sinDdev := veeAjuste("vee-ddev-que-no-esta exec -d /var/www/html/{rel} sh -c {cmd}", ".")
	r = veeUnGate(t, "true", Options{Local: sinDdev})
	if r.Status != verdict.StatusError || r.ExitCode != 127 || r.Command != "true" || r.Entorno != verdict.EntornoPrefijo {
		t.Fatalf("CA-760: un ddev que no esta en el host da 127, error: %+v", r)
	}
	if r.Notes != veeNota(NotaNoEncontrado, r.OutputTail) || !strings.Contains(r.Notes, "vee-ddev-que-no-esta") {
		t.Fatalf("CA-760: la nota nombra el ddev que falta: %q (salida %q)", r.Notes, r.OutputTail)
	}
}

// ---------------------------------------------------------------- CA-761

// CA-761: exit 126 (esta pero no se puede ejecutar) es error con
// NotaNoEjecutable, como 127; un archivo sin permiso de ejecucion y un
// directorio dan 126.
func TestCA761_Exit126EsErrorConNotaNoEjecutable(t *testing.T) {
	r := veeUnGate(t, "exit 126", Options{})
	if r.Status != verdict.StatusError || r.ExitCode != 126 || r.Notes != NotaNoEjecutable {
		t.Fatalf("CA-761: exit 126 sin salida: %+v", r)
	}
	r = veeUnGate(t, "printf 'algo\\n  no ejecutable  \\n\\n'; exit 126", Options{})
	if r.Status != verdict.StatusError || r.ExitCode != 126 || r.Notes != NotaNoEjecutable+": no ejecutable" {
		t.Fatalf("CA-761: exit 126 con salida: %+v", r)
	}
	d := veeDirReal(t)
	if err := os.WriteFile(filepath.Join(d, "vee-script.sh"), []byte("#!/bin/sh\necho no deberia correr\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(d, "vee-carpeta"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []string{"./vee-script.sh", "./vee-carpeta"} {
		r := veeGate(t, veeRun(veeManifest(d, map[string]string{"test": cmd}), Options{}), "test")
		if r.Status != verdict.StatusError || r.ExitCode != 126 || r.Notes != veeNota(NotaNoEjecutable, r.OutputTail) || r.Notes == NotaNoEjecutable {
			t.Fatalf("CA-761: %q no se puede ejecutar: error, 126, la nota con la linea de sh: %+v", cmd, r)
		}
	}
}

// CA-761: cualquier exit distinto de 0, 126 y 127 es fail con su codigo,
// sin nota de "no pudo correr". Todos, del 1 al 255.
func TestCA761_TodoOtroExitEsFailConSuCodigo(t *testing.T) {
	cmds := map[string]string{}
	for n := 1; n <= 255; n++ {
		if n == 126 || n == 127 {
			continue
		}
		cmds[fmt.Sprintf("g%03d", n)] = "echo salida; exit " + strconv.Itoa(n)
	}
	res := veeRun(veeManifest(veeDirReal(t), cmds), Options{})
	for nombre := range cmds {
		n, _ := strconv.Atoi(nombre[1:])
		r := veeGate(t, res, nombre)
		if r.Status != verdict.StatusFail || r.ExitCode != n || strings.Contains(r.Notes, "no pudo correr") {
			t.Fatalf("CA-761: exit %d es fail con %d: %+v", n, n, r)
		}
	}
}

// CA-761: un gate muerto por una senal, sin timeout, es fail (no error), y
// exit 0 sigue siendo pass.
func TestCA761_MuertoPorUnaSenalSinTimeoutEsFail(t *testing.T) {
	for _, cmd := range []string{"kill -9 $$", "kill -TERM $$", "kill -USR1 $$"} {
		r := veeUnGate(t, cmd, Options{})
		if r.Status != verdict.StatusFail || r.ExitCode == 0 || r.ExitCode == 126 || r.ExitCode == 127 {
			t.Fatalf("CA-761: %q (senal sin timeout) es fail con su codigo: %+v", cmd, r)
		}
		if strings.Contains(r.Notes, "no pudo correr") || strings.Contains(r.Notes, "timeout") {
			t.Fatalf("CA-761: %q no es un 'no pudo correr' ni un timeout: %+v", cmd, r)
		}
	}
	if r := veeUnGate(t, "echo ok", Options{}); r.Status != verdict.StatusPass || r.ExitCode != 0 || r.Notes != "" {
		t.Fatalf("CA-761: exit 0 es pass: %+v", r)
	}
}

// CA-761 con la plantilla: un contenedor apagado (el prefijo sale con 1) o
// un prefijo que sale con 3 es fail con su codigo, no error; el timeout con
// prefijo es error con -1, como sin el.
func TestCA761_ConPrefijoFailConSuCodigoYTimeout(t *testing.T) {
	for _, c := range []struct {
		ejecutar string
		code     int
	}{
		{`echo "el proyecto ddev no esta corriendo" >&2; exit 1; : {cmd}`, 1},
		{"exit 2; : {cmd}", 2},
		{"sh -c 'exit 3' {cmd}", 3},
	} {
		r := veeUnGate(t, "true", Options{Local: veeAjuste(c.ejecutar, ".")})
		if r.Status != verdict.StatusFail || r.ExitCode != c.code || r.Entorno != verdict.EntornoPrefijo || r.Command != "true" {
			t.Fatalf("CA-761: %q es fail con %d: %+v", c.ejecutar, c.code, r)
		}
	}

	veeFalso(t, "ddev")
	for _, local := range []*entorno.Ajuste{nil, veeAjuste(veePlantilla, ".")} {
		inicio := time.Now()
		r := veeUnGate(t, "sleep 30", Options{Local: local, Timeout: 500 * time.Millisecond})
		if r.Status != verdict.StatusError || r.ExitCode != -1 || !strings.Contains(r.Notes, "timeout") {
			t.Fatalf("CA-761: el timeout (prefijo %v) es error con -1 y su nota: %+v", local != nil, r)
		}
		if d := time.Since(inicio); d > 20*time.Second {
			t.Fatalf("CA-761: el timeout con prefijo corta igual que sin el: tardo %v", d)
		}
	}
}

// ---------------------------------------------------------------- CA-764

// CA-764: con la plantilla, el gate corre `sh -c <Linea>` en m.Dir: el
// prefijo recibe {rel} y {cmd} como un argumento cada uno, el gate corre
// adentro, Command es el comando del gate y Entorno prefijo; el de
// en_el_host corre como hoy, en el host, con Entorno host.
func TestCA764_ConPrefijoYEnElHost(t *testing.T) {
	log := veeFalso(t, "ddev")
	d := veeDirReal(t)
	cmd := `echo "dentro=${VEE_DENTRO:-no}"; printf '%s\n' "it's $(pwd -P)"`
	m := veeManifest(d, map[string]string{"test": cmd, "build": cmd})
	res := veeRun(m, Options{Local: veeAjuste(veePlantilla, ".hoom/worktrees/demo", "build")})

	test, build := veeGate(t, res, "test"), veeGate(t, res, "build")
	if test.Status != verdict.StatusPass || test.Command != cmd || test.Entorno != verdict.EntornoPrefijo ||
		!strings.Contains(test.OutputTail, "dentro=1") || !strings.Contains(test.OutputTail, "it's "+d) {
		t.Fatalf("CA-764: test corre con el prefijo, en %s: %+v", d, test)
	}
	if build.Status != verdict.StatusPass || build.Command != cmd || build.Entorno != verdict.EntornoHost ||
		!strings.Contains(build.OutputTail, "dentro=no") || !strings.Contains(build.OutputTail, "it's "+d) {
		t.Fatalf("CA-764: build (en_el_host) corre como hoy: %+v", build)
	}
	ll := veeLlamadas(t, log)
	want := []string{"exec", "-d", "/var/www/html/.hoom/worktrees/demo", "sh", "-c", cmd}
	if len(ll) != 1 || ll[0].dir != d || strings.Join(ll[0].args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("CA-764: una sola llamada al prefijo, en %s, con %q: %+v", d, want, ll)
	}
}

// CA-764: {dir} es la ruta absoluta del arbol verificado, y cada marca llega
// como un solo argumento aunque lleve blancos y comillas.
func TestCA764_DirYComillasLleganComoUnArgumento(t *testing.T) {
	log := veeFalso(t, "ddev")
	base := veeDirReal(t)
	d := filepath.Join(base, "it's a dir")
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := `printf '%s|' "a b" 'c'\''d' "$HOME" '{cmd}'`
	m := veeManifest(d, map[string]string{"test": cmd})
	r := veeGate(t, veeRun(m, Options{Local: veeAjuste("ddev {dir} {rel} {cmd}", "x y/it's")}), "test")
	if r.Status != verdict.StatusPass || r.Command != cmd || !strings.Contains(r.OutputTail, "a b|c'd|"+os.Getenv("HOME")+"|{cmd}|") {
		t.Fatalf("CA-764: el comando llega intacto al sh de adentro: %+v", r)
	}
	ll := veeLlamadas(t, log)
	if len(ll) != 1 || len(ll[0].args) != 3 || ll[0].args[0] != d || ll[0].args[1] != "x y/it's" || ll[0].args[2] != cmd || ll[0].dir != d {
		t.Fatalf("CA-764: {dir}, {rel} y {cmd} un argumento cada uno: %+v", ll)
	}
}

// CA-764 (aclaracion 1): con diff, Command es el diff_cmd del gate con
// {files} ya resuelto, y es exactamente lo que recibe el prefijo como {cmd}.
func TestCA764_CommandEsElComandoResueltoQueRecibeElPrefijo(t *testing.T) {
	log := veeFalso(t, "ddev")
	d := veeDirReal(t)
	full, diff, req := "echo completo", "echo diff {files}", true
	m := &manifest.Manifest{Dir: d, Gates: map[string]manifest.Gate{"lint": {Cmd: &full, DiffCmd: &diff, Required: &req}}}
	git := gitx.Info{IsRepo: true, Base: "main", ChangedFiles: []string{"a.go", "b.go"}}
	r := veeGate(t, Run(m, git, Options{Timeout: time.Minute, TailSize: 4096, Local: veeAjuste(veePlantilla, ".")}), "lint")
	if r.Status != verdict.StatusPass || r.Scope != "diff" || r.Entorno != verdict.EntornoPrefijo ||
		strings.Contains(r.Command, "{files}") || !strings.Contains(r.Command, "a.go") || !strings.Contains(r.Command, "b.go") {
		t.Fatalf("CA-764: el gate con diff corre su diff_cmd resuelto: %+v", r)
	}
	ll := veeLlamadas(t, log)
	if len(ll) != 1 || len(ll[0].args) == 0 || ll[0].args[len(ll[0].args)-1] != r.Command {
		t.Fatalf("CA-764: {cmd} es el Command del veredicto (%q): %+v", r.Command, ll)
	}
	if strings.Contains(r.Command, "ddev") || strings.Contains(r.Command, "/var/www/html") {
		t.Fatalf("CA-764: Command no lleva la plantilla: %q", r.Command)
	}
}

// CA-764 y aclaracion 3: sin plantilla (Local nil, el Ajuste cero, o uno
// que existe sin ejecutar), los gates corren como hoy: en el host, el
// prefijo no se llama y ningun GateResult lleva Entorno.
func TestCA764_SinPlantillaComoHoy(t *testing.T) {
	log := veeFalso(t, "ddev")
	cmd := `echo "dentro=${VEE_DENTRO:-no}"`
	for _, local := range []*entorno.Ajuste{nil, {}, {Existe: true, EnElHost: []string{"build"}, SHA256: strings.Repeat("cd", 32), Rel: "."}} {
		res := veeRun(veeManifest(veeDirReal(t), map[string]string{"test": cmd, "build": cmd, "vacio": ""}), Options{Local: local})
		for _, r := range res {
			if r.Entorno != "" {
				t.Fatalf("CA-764: sin plantilla ningun gate lleva Entorno (%+v): %+v", local, r)
			}
			if r.Name != "vacio" && (r.Status != verdict.StatusPass || r.Command != cmd || !strings.Contains(r.OutputTail, "dentro=no")) {
				t.Fatalf("CA-764: sin plantilla el gate corre como hoy (%+v): %+v", local, r)
			}
		}
	}
	if ll := veeLlamadas(t, log); len(ll) != 0 {
		t.Fatalf("CA-764: sin plantilla el prefijo no se llama: %+v", ll)
	}
}

// CA-766 por gates.Run (aclaracion 2): un gate ausente o skipped lleva el
// lugar donde habria corrido.
func TestCA766_AusentesYSkippedLlevanDondeHabrianCorrido(t *testing.T) {
	log := veeFalso(t, "ddev")
	m := veeManifest(veeDirReal(t), map[string]string{
		"test": "true", "lint": "", "build": "", "fmt": "touch vee-no-corre", "semgrep": "touch vee-no-corre",
	})
	res := veeRun(m, Options{
		Local: veeAjuste(veePlantilla, ".", "build", "semgrep"),
		Only:  map[string]bool{"test": true, "lint": true, "build": true},
	})
	for nombre, w := range map[string]struct{ status, entorno string }{
		"test":    {verdict.StatusPass, verdict.EntornoPrefijo},
		"lint":    {verdict.StatusAbsent, verdict.EntornoPrefijo},
		"build":   {verdict.StatusAbsent, verdict.EntornoHost},
		"fmt":     {verdict.StatusSkipped, verdict.EntornoPrefijo},
		"semgrep": {verdict.StatusSkipped, verdict.EntornoHost},
	} {
		if r := veeGate(t, res, nombre); r.Status != w.status || r.Entorno != w.entorno {
			t.Fatalf("CA-766: %s es %s con entorno %s: %+v", nombre, w.status, w.entorno, r)
		}
	}
	if _, err := os.Stat(filepath.Join(m.Dir, "vee-no-corre")); err == nil {
		t.Fatal("CA-766: un gate skipped no corre")
	}
	if n := len(veeLlamadas(t, log)); n != 1 {
		t.Fatalf("CA-766: solo test corrio con el prefijo: %d llamadas", n)
	}
}

// ---------------------------------------------------------------- CA-767

// CA-767: con ExigirEncierro no, el gate con prefijo corre sin encierro,
// con la nota exacta; el de en_el_host, sin nota. Sin ExigirEncierro, sin
// nota. Con ExigirEncierro pero sin plantilla, nadie lleva la nota.
func TestCA767_ExigirNoLlevaLaNotaDeSinEncierro(t *testing.T) {
	veeFalso(t, "ddev")
	cmd := "touch vee-corrio-$VEE_DENTRO"
	m := veeManifest(veeDirReal(t), map[string]string{"test": cmd, "build": "true"})
	res := veeRun(m, Options{Local: veeAjuste(veePlantilla, ".", "build"), ExigirEncierro: manifest.ExigirNo})
	test, build := veeGate(t, res, "test"), veeGate(t, res, "build")
	if test.Status != verdict.StatusPass || test.Notes != NotaSinEncierroConPrefijo || test.Entorno != verdict.EntornoPrefijo {
		t.Fatalf("CA-767: con exigir no, el gate con prefijo corre con la nota %q: %+v", NotaSinEncierroConPrefijo, test)
	}
	if _, err := os.Stat(filepath.Join(m.Dir, "vee-corrio-1")); err != nil {
		t.Fatalf("CA-767: con exigir no el gate corrio adentro: %v", err)
	}
	if build.Status != verdict.StatusPass || build.Notes != "" || build.Entorno != verdict.EntornoHost {
		t.Fatalf("CA-767: el de en_el_host no lleva la nota: %+v", build)
	}

	if r := veeUnGate(t, "true", Options{Local: veeAjuste(veePlantilla, ".")}); r.Notes != "" {
		t.Fatalf("CA-767: sin la verify de un sobre con encierro, no hay nota: %+v", r)
	}
	for _, exigir := range []string{manifest.ExigirNo, manifest.ExigirParcial, manifest.ExigirCompleto} {
		for _, local := range []*entorno.Ajuste{nil, {Existe: true, Rel: "."}} {
			r := veeUnGate(t, "true", Options{Local: local, ExigirEncierro: exigir})
			if r.Status != verdict.StatusPass || r.Notes != "" || r.Entorno != "" {
				t.Fatalf("CA-767: exigir %s sin plantilla no toca al gate: %+v", exigir, r)
			}
		}
	}
}

// CA-767 (aclaracion 9): con exigir completo o parcial, el gate con prefijo
// no corre (ni el prefijo se llama): error, -1, su Command y su Entorno
// como si hubiera corrido, y la nota con el valor de exigir. El de
// en_el_host corre como hoy.
func TestCA767_ExigirCompletoOParcialNoCorreElGateConPrefijo(t *testing.T) {
	for _, exigir := range []string{manifest.ExigirCompleto, manifest.ExigirParcial} {
		log := veeFalso(t, "ddev")
		m := veeManifest(veeDirReal(t), map[string]string{"test": "touch vee-test-corrio", "lint": "touch vee-lint-corrio", "build": "touch vee-build-corrio"})
		res := veeRun(m, Options{Local: veeAjuste(veePlantilla, ".", "build"), ExigirEncierro: exigir})
		nota := fmt.Sprintf(NotaExigirConPrefijo, exigir)
		for _, nombre := range []string{"test", "lint"} {
			r := veeGate(t, res, nombre)
			if r.Status != verdict.StatusError || r.ExitCode != -1 || r.Notes != nota || r.Command != "touch vee-"+nombre+"-corrio" || r.Entorno != verdict.EntornoPrefijo {
				t.Fatalf("CA-767: exigir %s: %s no corre, error -1 con %q: %+v", exigir, nombre, nota, r)
			}
			if _, err := os.Stat(filepath.Join(m.Dir, "vee-"+nombre+"-corrio")); err == nil {
				t.Fatalf("CA-767: exigir %s: %s no corre", exigir, nombre)
			}
		}
		if ll := veeLlamadas(t, log); len(ll) != 0 {
			t.Fatalf("CA-767: exigir %s: el prefijo no se llama: %+v", exigir, ll)
		}
		build := veeGate(t, res, "build")
		if build.Status != verdict.StatusPass || build.Notes != "" || build.Entorno != verdict.EntornoHost {
			t.Fatalf("CA-767: exigir %s: el de en_el_host corre como hoy: %+v", exigir, build)
		}
		if _, err := os.Stat(filepath.Join(m.Dir, "vee-build-corrio")); err != nil {
			t.Fatalf("CA-767: exigir %s: build corrio: %v", exigir, err)
		}
	}
	if manifest.ExigirNo != "no" || manifest.ExigirParcial != "parcial" || manifest.ExigirCompleto != "completo" {
		t.Fatalf("CA-767: los valores de exigir: %q %q %q", manifest.ExigirNo, manifest.ExigirParcial, manifest.ExigirCompleto)
	}
}

// CA-767 (aclaracion 8): un gate con prefijo que tambien tiene nota propia
// (127, 126, timeout) lleva "<su nota>; " + NotaSinEncierroConPrefijo.
func TestCA767_DosNotasSeJuntanConPuntoYComa(t *testing.T) {
	veeFalso(t, "ddev")
	opt := Options{Local: veeAjuste(veePlantilla, "."), ExigirEncierro: manifest.ExigirNo}
	r := veeUnGate(t, "echo ' la ultima '; exit 127", opt)
	if r.Status != verdict.StatusError || r.ExitCode != 127 || r.Notes != NotaNoEncontrado+": la ultima; "+NotaSinEncierroConPrefijo {
		t.Fatalf("CA-767: 127 con exigir no: %+v", r)
	}
	r = veeUnGate(t, "exit 126", opt)
	if r.Status != verdict.StatusError || r.ExitCode != 126 || r.Notes != NotaNoEjecutable+"; "+NotaSinEncierroConPrefijo {
		t.Fatalf("CA-767: 126 con exigir no: %+v", r)
	}
	opt.Timeout = 500 * time.Millisecond
	r = veeUnGate(t, "sleep 30", opt)
	propia := strings.TrimSuffix(r.Notes, "; "+NotaSinEncierroConPrefijo)
	if r.Status != verdict.StatusError || r.ExitCode != -1 || propia == r.Notes || !strings.Contains(propia, "timeout") || strings.Contains(propia, NotaSinEncierroConPrefijo) {
		t.Fatalf("CA-767: timeout con exigir no: \"<nota del timeout>; %s\": %+v", NotaSinEncierroConPrefijo, r)
	}
	if r := veeUnGate(t, "exit 3", opt); r.Status != verdict.StatusFail || r.ExitCode != 3 || r.Notes != NotaSinEncierroConPrefijo {
		t.Fatalf("CA-767: un fail sin nota propia lleva solo la de sin encierro: %+v", r)
	}
}
