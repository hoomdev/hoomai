//go:build !windows

// Regresiones de la delta 2 de la review de verify-en-el-entorno (1617fc),
// escritas por el writer en la ronda 3, sobre completarGitignore, que solo
// valen donde el sistema cuenta los enlaces duros:
//
//   - 31f78c: un .hoom/.gitignore con otro enlace duro (a un archivo fuera
//     del proyecto, o hecho al que hoom acaba de crear) no se escribe: es un
//     *EnlacesDuros, sin agregar nada, y los dos nombres quedan como estaban;
//   - c1ec5c: el .hoom/.gitignore que hoom crea y no puede escribir no queda
//     vacio ni a medias, salvo que tenga lineas ajenas, que quedan.
package hoomfs

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
)

// vrhEnlazar hace en otro un enlace duro a gi, o saltea el test si el
// sistema de archivos no los tiene.
func vrhEnlazar(t *testing.T, gi, otro string) {
	t.Helper()
	if err := os.Link(gi, otro); err != nil {
		t.Skipf("este sistema de archivos no hace enlaces duros: %v", err)
	}
}

// 31f78c: .hoom/.gitignore es un enlace duro a un archivo fuera del
// proyecto. EnsureIgnoredFiles(local.yaml) y EnsureIgnored(worktrees)
// devuelven un *EnlacesDuros, sin agregar nada, y los bytes de los dos
// nombres no cambian. Tambien cuando el enlace se hace al .hoom/.gitignore
// que EnsureIgnoredFiles acaba de crear, antes de escribirle su linea.
func TestHallazgo31f78c_UnGitignoreConOtroEnlaceDuroNoSeEscribe(t *testing.T) {
	for _, c := range []struct {
		nombre string
		llamar func(root string) ([]string, error)
	}{
		{"EnsureIgnoredFiles(local.yaml)", func(root string) ([]string, error) { return EnsureIgnoredFiles(root, "local.yaml") }},
		{"EnsureIgnored(worktrees)", func(root string) ([]string, error) { return EnsureIgnored(root, "worktrees") }},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			base := t.TempDir()
			root := filepath.Join(base, "proyecto")
			gi := vrhGitignore(t, root, "", 0)
			otro := filepath.Join(base, "fuera-del-proyecto", "notas.txt")
			if err := os.MkdirAll(filepath.Dir(otro), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(otro, []byte("cache/\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			vrhEnlazar(t, otro, gi)
			added, err := c.llamar(root)
			vrhNoSeEscribio(t, c.nombre, added, err, map[string]string{gi: "cache/\n", otro: "cache/\n"})
		})
	}
	t.Run("al que acaba de crear", func(t *testing.T) {
		base := t.TempDir()
		root := filepath.Join(base, "proyecto")
		gi := vrhGitignore(t, root, "", 0)
		otro := filepath.Join(base, "fuera-del-proyecto.txt")
		var corrio bool
		vrhArmar(t, &antesDeEscribirCreado, func(string) {
			corrio = true
			vrhEnlazar(t, gi, otro)
		})
		added, err := EnsureIgnoredFiles(root, "local.yaml")
		if !corrio {
			t.Fatal("fixture: el gancho entre la creacion y la escritura no corrio")
		}
		vrhNoSeEscribio(t, "EnsureIgnoredFiles(local.yaml) al que acaba de crear", added, err, map[string]string{gi: "", otro: ""})
	})
}

// vrhNoSeEscribio comprueba que la llamada devolvio un *EnlacesDuros sin
// agregar nada y que cada archivo de quedan sigue con sus bytes.
func vrhNoSeEscribio(t *testing.T, caso string, added []string, err error, quedan map[string]string) {
	t.Helper()
	var ed *EnlacesDuros
	if !errors.As(err, &ed) || ed.Enlaces != 2 {
		t.Errorf("31f78c: %s: devuelve un *EnlacesDuros con 2 enlaces: %v", caso, err)
	}
	if len(added) != 0 {
		t.Errorf("31f78c: %s: no agrega nada: %q", caso, added)
	}
	for path, want := range quedan {
		raw, rerr := os.ReadFile(path)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if string(raw) != want {
			t.Errorf("31f78c: %s: %s queda %q: %q", caso, path, want, raw)
		}
	}
}

const (
	// vrhEnvHijo y vrhEnvRoot llevan al hijo del test de c1ec5c su caso y
	// la raiz del proyecto.
	vrhEnvHijo = "HOOM_VRH_C1EC5C_HIJO"
	vrhEnvRoot = "HOOM_VRH_C1EC5C_ROOT"
	// vrhInforme marca la linea del informe del hijo.
	vrhInforme = "C1EC5C-INFORME"
)

// c1ec5c: sin .hoom/.gitignore, EnsureIgnoredFiles(local.yaml) lo crea y su
// escritura falla: un tope al tamano de archivo que el proceso puede escribir
// (RLIMIT_FSIZE) la corta antes del primer byte (tope 0) o a medias (tope 5,
// "local"). En los dos es un error, sin agregar nada, y no queda ningun
// .hoom/.gitignore. Si otra mano le anexo al recien creado una linea suya
// antes de la escritura (tope 15: entra la suya y 4 bytes de local.yaml), el
// archivo tiene bytes ajenos y queda como esta. Go ignora SIGXFSZ, asi que
// la escritura falla con "file too large".
//
// RLIMIT_FSIZE es de todo el proceso: cada caso corre en un hijo (este mismo
// binario de test, re-ejecutado con -test.run y vrhEnvHijo), como el de
// envelope (hallazgos_endurecimiento1d_unix_test.go), y deja su informe en
// la salida; el padre lo juzga. Asi el tope nunca alcanza al proceso de go
// test (su testlog, por ejemplo).
func TestHallazgoc1ec5c_UnGitignoreCreadoQueNoSePudoEscribirNoQuedaAMedias(t *testing.T) {
	if caso := os.Getenv(vrhEnvHijo); caso != "" {
		vrhHijoC1ec5c(t, caso, os.Getenv(vrhEnvRoot))
		return
	}
	for _, c := range []struct {
		nombre, caso string
		queda        *string // nil = no queda el archivo
	}{
		{"nada escrito", "0", nil},
		{"escrito a medias", "5", nil},
		{"con una linea ajena", "ajena", vrhPtr("secret.env\nloca")},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := t.TempDir()
			gi := vrhGitignore(t, root, "", 0)
			cmd := exec.Command(os.Args[0], "-test.run=^TestHallazgoc1ec5c_UnGitignoreCreadoQueNoSePudoEscribirNoQuedaAMedias$", "-test.count=1", "-test.v")
			cmd.Env = append(os.Environ(), vrhEnvHijo+"="+c.caso, vrhEnvRoot+"="+root)
			salida, err := cmd.CombinedOutput()
			if strings.Contains(string(salida), "--- SKIP") {
				t.Skipf("c1ec5c: %s: el hijo salteo el caso:\n%s", c.nombre, salida)
			}
			if err != nil {
				t.Fatalf("c1ec5c: %s: el hijo fallo: %v\n%s", c.nombre, err, salida)
			}
			agregadas, fallo := vrhInformeDelHijo(t, c.nombre, salida)
			if !fallo || agregadas != 0 {
				t.Errorf("c1ec5c: %s: la escritura cortada es un error, sin agregar nada: agregadas=%d fallo=%t\n%s", c.nombre, agregadas, fallo, salida)
			}
			raw, rerr := os.ReadFile(gi)
			switch {
			case c.queda == nil && !errors.Is(rerr, os.ErrNotExist):
				t.Errorf("c1ec5c: %s: no queda un .hoom/.gitignore vacio ni a medias: %q %v", c.nombre, raw, rerr)
			case c.queda != nil && string(raw) != *c.queda:
				t.Errorf("c1ec5c: %s: con bytes ajenos el archivo queda %q: %q %v", c.nombre, *c.queda, raw, rerr)
			}
		})
	}
}

// vrhInformeDelHijo lee de la salida del hijo de c1ec5c cuantas lineas dijo
// EnsureIgnoredFiles que agrego y si fallo.
func vrhInformeDelHijo(t *testing.T, caso string, salida []byte) (agregadas int, fallo bool) {
	t.Helper()
	for _, l := range strings.Split(string(salida), "\n") {
		if rest, ok := strings.CutPrefix(l, vrhInforme+" "); ok {
			if _, err := fmt.Sscanf(rest, "agregadas=%d fallo=%t", &agregadas, &fallo); err != nil {
				t.Fatalf("c1ec5c: %s: informe ilegible %q: %v", caso, l, err)
			}
			return agregadas, fallo
		}
	}
	t.Fatalf("c1ec5c: %s: el hijo no dejo informe:\n%s", caso, salida)
	return 0, false
}

// vrhHijoC1ec5c es el caso de c1ec5c en el proceso hijo: baja RLIMIT_FSIZE,
// llama EnsureIgnoredFiles(root, "local.yaml"), devuelve el tope y deja el
// informe en la salida.
func vrhHijoC1ec5c(t *testing.T, caso, root string) {
	tope := uint64(15)
	if caso == "ajena" {
		gi := filepath.Join(root, ".hoom", ".gitignore")
		vrhArmar(t, &antesDeEscribirCreado, func(string) {
			f, err := os.OpenFile(gi, os.O_WRONLY|os.O_APPEND, 0)
			if err != nil {
				t.Error(err)
				return
			}
			defer f.Close()
			if _, err := f.WriteString("secret.env\n"); err != nil {
				t.Error(err)
			}
		})
	} else {
		n, err := strconv.ParseUint(caso, 10, 64)
		if err != nil {
			t.Fatalf("caso desconocido %q", caso)
		}
		tope = n
	}
	var antes syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &antes); err != nil {
		t.Skipf("este sistema no deja leer RLIMIT_FSIZE: %v", err)
	}
	nuevo := antes
	nuevo.Cur = tope
	if err := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &nuevo); err != nil {
		t.Skipf("este sistema no deja bajar RLIMIT_FSIZE: %v", err)
	}
	added, err := EnsureIgnoredFiles(root, "local.yaml")
	if serr := syscall.Setrlimit(syscall.RLIMIT_FSIZE, &antes); serr != nil {
		t.Fatalf("no se pudo devolver RLIMIT_FSIZE: %v", serr)
	}
	t.Logf("EnsureIgnoredFiles: %q %v", added, err)
	fmt.Printf("%s agregadas=%d fallo=%t\n", vrhInforme, len(added), err != nil)
}
