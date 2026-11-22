// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre los
// entornos de los perfiles (CA-769): laravel declara ddev y sail, en ese
// orden y con sus plantillas; Sugerencias da los que se cumplen en el
// directorio (el archivo existe y el comando no esta en el PATH), en el
// orden del perfil (aclaracion 12), sin escribir nada; Sugerencia es el
// texto exacto de dos lineas.
//
// Como consumidor externo: solo la API publica. Escritos desde el spec, las
// aclaraciones de la ronda 0 y el go doc, sin leer la implementacion.
package profiles_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/hoomdev/hoomai/internal/profiles"
)

var (
	veeDdev = profiles.Entorno{Nombre: "ddev", SiExiste: ".ddev/config.yaml", Sin: "php",
		Ejecutar: "ddev exec -d /var/www/html/{rel} sh -c {cmd}"}
	veeSail = profiles.Entorno{Nombre: "sail", SiExiste: "vendor/bin/sail", Sin: "php",
		Ejecutar: "docker compose exec -T -w /var/www/html/{rel} laravel.test sh -c {cmd}"}
)

// veeSinPHP deja el PATH en una carpeta vacia (sin php) y la devuelve.
func veeSinPHP(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	return bin
}

func veeArchivo(t *testing.T, dir, rel string, modo os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("x\n"), modo); err != nil {
		t.Fatal(err)
	}
}

// veeArbol es la lista de lo que hay bajo dir, con su contenido.
func veeArbol(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			out[rel] = "dir"
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// CA-769: laravel declara ddev y sail, en ese orden, con las plantillas del
// spec; un perfil que no existe es el error de Resolve.
func TestCA769_EntornosDeLaravel(t *testing.T) {
	const ca = "CA-769"
	got, err := profiles.Entornos("laravel")
	if err != nil {
		t.Fatalf("%s: Entornos(laravel): %v", ca, err)
	}
	if !reflect.DeepEqual(got, []profiles.Entorno{veeDdev, veeSail}) {
		t.Fatalf("%s: laravel declara ddev y sail, en ese orden: %+v", ca, got)
	}
	if _, err := profiles.Entornos("vee-no-existe"); err == nil {
		t.Fatalf("%s: un perfil que no existe es un error", ca)
	}
	_, _, rerr := profiles.Resolve("vee-no-existe")
	if _, err := profiles.Entornos("vee-no-existe"); rerr == nil || err == nil || err.Error() != rerr.Error() {
		t.Fatalf("%s: el error de un perfil desconocido es el de Resolve: %v vs %v", ca, err, rerr)
	}
	for _, n := range profiles.Names() {
		if _, err := profiles.Entornos(n); err != nil {
			t.Fatalf("%s: Entornos(%s) de un perfil que existe: %v", ca, n, err)
		}
	}
}

// CA-769: Sugerencia es el texto exacto, dos lineas sin salto final, con la
// plantilla tal cual entre comillas dobles.
func TestCA769_SugerenciaEsElTextoExacto(t *testing.T) {
	const ca = "CA-769"
	for _, c := range []struct {
		e    profiles.Entorno
		want string
	}{
		{veeDdev, "sugerencia: este proyecto parece usar ddev y el host no tiene php; para correr los gates ahi, crea .hoom/local.yaml con:\n" +
			"  ejecutar: \"ddev exec -d /var/www/html/{rel} sh -c {cmd}\""},
		{veeSail, "sugerencia: este proyecto parece usar sail y el host no tiene php; para correr los gates ahi, crea .hoom/local.yaml con:\n" +
			"  ejecutar: \"docker compose exec -T -w /var/www/html/{rel} laravel.test sh -c {cmd}\""},
		{profiles.Entorno{Nombre: "lando", Sin: "node", Ejecutar: `lando ssh -c {cmd} "x"`},
			"sugerencia: este proyecto parece usar lando y el host no tiene node; para correr los gates ahi, crea .hoom/local.yaml con:\n" +
				"  ejecutar: \"lando ssh -c {cmd} \"x\"\""},
	} {
		if got := c.e.Sugerencia(); got != c.want {
			t.Fatalf("%s: la sugerencia de %s es\n%q\nfue\n%q", ca, c.e.Nombre, c.want, got)
		}
	}
}

// CA-769 y aclaracion 12: Sugerencias da, en el orden del perfil, los
// entornos cuyo archivo existe en el directorio y cuyo comando no esta en
// el PATH; nil si ninguno o si el perfil no existe. Nunca escribe nada.
func TestCA769_SugerenciasSegunElDirectorioYElPATH(t *testing.T) {
	const ca = "CA-769"
	if runtime.GOOS == "windows" {
		t.Skip("el php falso es un archivo con permiso de ejecucion")
	}
	bin := veeSinPHP(t)
	for _, c := range []struct {
		nombre   string
		archivos []string
		want     []profiles.Entorno
	}{
		{"ddev", []string{".ddev/config.yaml"}, []profiles.Entorno{veeDdev}},
		{"sail", []string{"vendor/bin/sail"}, []profiles.Entorno{veeSail}},
		{"los dos, en el orden del perfil", []string{"vendor/bin/sail", ".ddev/config.yaml"}, []profiles.Entorno{veeDdev, veeSail}},
		{"ninguno", []string{"composer.json", ".ddev/otro.yaml", "vendor/bin/otro"}, nil},
	} {
		dir := t.TempDir()
		for _, a := range c.archivos {
			veeArchivo(t, dir, a, 0o644)
		}
		antes := veeArbol(t, dir)
		got := profiles.Sugerencias("laravel", dir)
		if !reflect.DeepEqual(got, c.want) {
			t.Fatalf("%s: %s: Sugerencias es %+v: %+v", ca, c.nombre, c.want, got)
		}
		if c.want == nil && got != nil {
			t.Fatalf("%s: %s: sin ninguna, nil: %#v", ca, c.nombre, got)
		}
		if despues := veeArbol(t, dir); !reflect.DeepEqual(antes, despues) {
			t.Fatalf("%s: %s: Sugerencias no escribe nada: %v -> %v", ca, c.nombre, antes, despues)
		}
	}

	dir := t.TempDir()
	veeArchivo(t, dir, ".ddev/config.yaml", 0o644)
	veeArchivo(t, dir, "vendor/bin/sail", 0o755)
	if got := profiles.Sugerencias("vee-no-existe", dir); got != nil {
		t.Fatalf("%s: un perfil que no existe no sugiere: %+v", ca, got)
	}

	// un php que no se puede ejecutar no cuenta como php en el PATH
	veeArchivo(t, bin, "php", 0o644)
	if got := profiles.Sugerencias("laravel", dir); !reflect.DeepEqual(got, []profiles.Entorno{veeDdev, veeSail}) {
		t.Fatalf("%s: un php sin permiso de ejecucion no esta en el PATH: %+v", ca, got)
	}
	// con php en el PATH, ninguno se cumple
	if err := os.Chmod(filepath.Join(bin, "php"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := profiles.Sugerencias("laravel", dir); got != nil {
		t.Fatalf("%s: con php en el PATH no hay sugerencias: %+v", ca, got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".hoom", "local.yaml")); err == nil {
		t.Fatalf("%s: nadie escribe .hoom/local.yaml sino la persona", ca)
	}
}
