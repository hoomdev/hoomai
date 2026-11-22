// Tests adversariales del spec .hoom/specs/verify-en-el-entorno.md sobre
// `hoom init` (initcmd.Run):
//
//   - CA-770: un .hoom/.gitignore nuevo es hoomfs.GitignoreBody (con
//     local.yaml); a uno que ya existe se le agrega la linea local.yaml si le
//     falta, y nada mas cambia; si la tiene, queda byte a byte;
//   - CA-769: al final imprime la Sugerencia de cada entorno del perfil que
//     se cumple en el directorio (aclaracion 12: ddev y sail, en ese orden),
//     y nunca escribe .hoom/local.yaml.
//
// Escritos desde el spec, las aclaraciones de la ronda 0 y el go doc, sin
// leer la implementacion. No se fija a que stream va la sugerencia: se lee
// stdout y stderr juntos.
package initcmd

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/profiles"
)

// veeCorrer corre Run capturando stdout y stderr juntos.
func veeCorrer(t *testing.T, dir, perfil string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	viejoOut, viejoErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = w, w
	leido := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		leido <- string(b)
	}()
	runErr := Run(dir, "demo", perfil)
	os.Stdout, os.Stderr = viejoOut, viejoErr
	w.Close()
	out := <-leido
	r.Close()
	return out, runErr
}

// veeSinPHP deja un PATH con las herramientas del sistema que init pueda
// usar (git, sh...) y sin php.
func veeSinPHP(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	for _, n := range []string{"git", "sh", "bash", "env", "cat", "ls", "mkdir", "rm", "cp", "mv", "uname", "id", "tr", "sed", "grep", "head", "tail", "dirname", "basename", "printf", "true", "false", "test"} {
		if p, err := exec.LookPath(n); err == nil {
			_ = os.Symlink(p, filepath.Join(bin, n))
		}
	}
	t.Setenv("PATH", bin)
	return bin
}

func veeEscribir(t *testing.T, dir, rel, body string, modo os.FileMode) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), modo); err != nil {
		t.Fatal(err)
	}
}

func veeLeer(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// CA-770: hoom init en un proyecto nuevo deja .hoom/.gitignore igual a
// hoomfs.GitignoreBody, con la linea local.yaml.
func TestCA770_InitNuevoDejaElGitignoreCanonico(t *testing.T) {
	const ca = "CA-770"
	dir := t.TempDir()
	if _, err := veeCorrer(t, dir, "go"); err != nil {
		t.Fatalf("%s: hoom init: %v", ca, err)
	}
	got := veeLeer(t, filepath.Join(dir, ".hoom", ".gitignore"))
	if got != hoomfs.GitignoreBody() || !hrTieneRegla(got, "local.yaml") {
		t.Fatalf("%s: el .gitignore nuevo es GitignoreBody con local.yaml: %q", ca, got)
	}
}

// CA-770: a un .hoom/.gitignore que ya existe, hoom init le agrega la linea
// local.yaml si le falta y nada mas cambia; si la tiene, queda igual.
func TestCA770_InitConGitignoreExistente(t *testing.T) {
	const ca = "CA-770"
	for _, c := range []struct{ antes, despues string }{
		{"cache/\nmi-cosa.log\n", "cache/\nmi-cosa.log\nlocal.yaml\n"},
		{"worktrees/\n# de esta maquina\n", "worktrees/\n# de esta maquina\nlocal.yaml\n"},
		{"cache/\n  local.yaml\nmi-cosa\n", "cache/\n  local.yaml\nmi-cosa\n"},
		{hoomfs.GitignoreBody(), hoomfs.GitignoreBody()},
	} {
		dir := t.TempDir()
		veeEscribir(t, dir, ".hoom/.gitignore", c.antes, 0o644)
		if _, err := veeCorrer(t, dir, "go"); err != nil {
			t.Fatalf("%s: hoom init con .gitignore %q: %v", ca, c.antes, err)
		}
		if got := veeLeer(t, filepath.Join(dir, ".hoom", ".gitignore")); got != c.despues {
			t.Fatalf("%s: con %q el .gitignore queda %q; fue %q", ca, c.antes, c.despues, got)
		}
	}
}

// CA-769 y aclaracion 12: hoom init de un proyecto laravel sin php en el
// PATH imprime al final la sugerencia de cada entorno que se cumple, en el
// orden del perfil, una vez cada una, y no escribe .hoom/local.yaml; sin
// los archivos, o con php en el PATH, no sugiere nada.
func TestCA769_InitImprimeLasSugerenciasAlFinal(t *testing.T) {
	const ca = "CA-769"
	if runtime.GOOS == "windows" {
		t.Skip("el php falso es un archivo con permiso de ejecucion")
	}
	ents, err := profiles.Entornos("laravel")
	if err != nil || len(ents) != 2 || ents[0].Nombre != "ddev" || ents[1].Nombre != "sail" {
		t.Fatalf("%s: fixture: laravel declara ddev y sail: %+v %v", ca, ents, err)
	}
	ddev, sail := ents[0].Sugerencia(), ents[1].Sugerencia()
	bin := veeSinPHP(t)
	for _, c := range []struct {
		nombre   string
		archivos []string
		want     []string
	}{
		{"ddev", []string{".ddev/config.yaml"}, []string{ddev}},
		{"sail", []string{"vendor/bin/sail"}, []string{sail}},
		{"los dos", []string{".ddev/config.yaml", "vendor/bin/sail"}, []string{ddev, sail}},
		{"ninguno", []string{"composer.json"}, nil},
	} {
		dir := t.TempDir()
		veeEscribir(t, dir, "composer.json", "{}\n", 0o644)
		for _, a := range c.archivos {
			veeEscribir(t, dir, a, "x\n", 0o755)
		}
		out, err := veeCorrer(t, dir, "laravel")
		if err != nil {
			t.Fatalf("%s: %s: hoom init: %v\n%s", ca, c.nombre, err, out)
		}
		if n := strings.Count(out, "sugerencia: "); n != len(c.want) {
			t.Fatalf("%s: %s: %d sugerencias, una por entorno que se cumple: %d\n%s", ca, c.nombre, len(c.want), n, out)
		}
		fin := 0
		for _, s := range c.want {
			i := strings.Index(out, s)
			if i < fin {
				t.Fatalf("%s: %s: la sugerencia, en el orden del perfil:\n%s\n--- salida ---\n%s", ca, c.nombre, s, out)
			}
			fin = i + len(s)
		}
		if len(c.want) > 0 && strings.TrimSpace(out[fin:]) != "" {
			t.Fatalf("%s: %s: las sugerencias van al final: sobra %q", ca, c.nombre, out[fin:])
		}
		if _, err := os.Lstat(filepath.Join(dir, ".hoom", "local.yaml")); err == nil {
			t.Fatalf("%s: %s: hoom init nunca escribe .hoom/local.yaml", ca, c.nombre)
		}
	}

	veeEscribir(t, bin, "php", "#!/bin/sh\nexit 0\n", 0o755)
	dir := t.TempDir()
	veeEscribir(t, dir, "composer.json", "{}\n", 0o644)
	veeEscribir(t, dir, ".ddev/config.yaml", "x\n", 0o644)
	out, err := veeCorrer(t, dir, "laravel")
	if err != nil || strings.Contains(out, "sugerencia: ") {
		t.Fatalf("%s: con php en el PATH no hay sugerencia: %v\n%s", ca, err, out)
	}
}
