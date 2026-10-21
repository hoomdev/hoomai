// Tests adversariales del hallazgo 20261020T201503_28c350 (high, risk) de la
// review de los fixes de C1 (.hoom/specs/items-y-columna-derivada.md, CA-280:
// la carpeta de la tarea es el worktree .hoom/worktrees/<slug>). Escritos
// desde el contrato "puerta completa", nunca desde la implementacion:
//
// runcmd.TaskDir(root, task) es la UNICA puerta que decide la carpeta de una
// tarea. Con task vacio devuelve root. Con task no vacio devuelve error, sin
// tocar ni lanzar nada, si
//
//	(a) el slug no es valido segun item.ValidSlug (../x, a/b, .., Mayus...);
//	(b) .hoom, .hoom/worktrees o .hoom/worktrees/<slug> es un symlink;
//	(c) .hoom/worktrees/<slug> no es un worktree registrado de ESTE repo:
//	    una carpeta comun, un clon aparte o un worktree de otro repo.
//
// Si pasa, devuelve la ruta de la carpeta. Un run con una Task que la puerta
// rechaza no lanza el provider. Guarda: un worktree de verdad, creado con
// taskcmd.Start, sigue funcionando.
//
// Paquete externo (runcmd_test) para poder crear el worktree real con
// taskcmd.Start sin ciclos de importacion. Repos git reales bajo t.TempDir.
package runcmd_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const (
	t28Legit         = "legit"
	t28NombreVictima = "victima-28c350"
)

func t28Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v (en %s): %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func t28Escribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func t28Real(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatalf("EvalSymlinks(%s): %v", p, err)
	}
	return r
}

func t28Symlink(t *testing.T, destino, enlace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(enlace), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, enlace); err != nil {
		t.Fatal(err)
	}
}

// t28Repo crea un repo git en dir con hoom.yaml, .hoom/.gitignore y app.go
// commiteados (lo minimo para `hoom task start`).
func t28Repo(t *testing.T, dir string, conHoom bool) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t28Git(t, dir, "init", "-q", "-b", "main")
	t28Git(t, dir, "config", "user.email", "test@hoom.dev")
	t28Git(t, dir, "config", "user.name", "hoom test")
	t28Escribir(t, dir, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	if conHoom {
		t28Escribir(t, dir, ".hoom/.gitignore", hoomfs.GitignoreBody())
	}
	t28Escribir(t, dir, "app.go", "package app\n")
	t28Git(t, dir, "add", "-A")
	t28Git(t, dir, "commit", "-q", "-m", "inicial")
}

// t28Victima crea un repo con un archivo rastreado MODIFICADO y uno NO
// RASTREADO: trabajo sin guardar de otra persona.
func t28Victima(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	t28Git(t, dir, "init", "-q", "-b", "main")
	t28Git(t, dir, "config", "user.email", "victima@hoom.dev")
	t28Git(t, dir, "config", "user.name", "victima")
	t28Escribir(t, dir, "tracked.go", "package victima\n")
	t28Git(t, dir, "add", "-A")
	t28Git(t, dir, "commit", "-q", "-m", "de la victima")
	t28Escribir(t, dir, "tracked.go", "package victima\n\n// trabajo sin guardar de otra persona\n")
	t28Escribir(t, dir, "untracked.go", "package victima\n\n// archivo nuevo de otra persona\n")
}

// t28Foto es el estado de un arbol: HEAD, status y el contenido de cada
// archivo (fuera de .git; los symlinks se anotan, no se siguen). Dos fotos
// iguales = no se toco nada.
func t28Foto(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
		b.WriteString(t28Git(t, dir, "rev-parse", "HEAD") + "\n")
		b.WriteString(t28Git(t, dir, "status", "--porcelain", "--untracked-files=all") + "\n--\n")
	}
	var lineas []string
	filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil // el .git de un worktree es un archivo: SkipDir saltearia sus hermanos
		}
		rel, _ := filepath.Rel(dir, p)
		if d.Type()&os.ModeSymlink != 0 {
			dest, _ := os.Readlink(p)
			lineas = append(lineas, fmt.Sprintf("%s -> %s", rel, dest))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		raw, _ := os.ReadFile(p)
		lineas = append(lineas, fmt.Sprintf("%s=%q", rel, raw))
		return nil
	})
	sort.Strings(lineas)
	b.WriteString(strings.Join(lineas, "\n"))
	return b.String()
}

// t28Entorno es un proyecto base/proyecto con la tarea legitima creada con
// taskcmd.Start y una victima en base/victima-28c350 (al lado del proyecto:
// ../../../victima-28c350 desde .hoom/worktrees/).
type t28Entorno struct {
	base, root, legit, victima string
}

func t28Proyecto(t *testing.T) t28Entorno {
	t.Helper()
	base := t28Real(t, t.TempDir())
	root := filepath.Join(base, "proyecto")
	t28Repo(t, root, true)
	if err := taskcmd.Start(root, t28Legit, "main"); err != nil {
		t.Fatalf("fixture: task start %s: %v", t28Legit, err)
	}
	e := t28Entorno{base: base, root: root,
		legit:   filepath.Join(root, ".hoom", "worktrees", t28Legit),
		victima: filepath.Join(base, t28NombreVictima)}
	t28Victima(t, e.victima)
	return e
}

func (e t28Entorno) wt(slug string) string { return filepath.Join(e.root, ".hoom", "worktrees", slug) }

// t28Rechaza exige que la puerta devuelva error para slug.
func t28Rechaza(t *testing.T, regla, root, slug string) {
	t.Helper()
	got, err := runcmd.TaskDir(root, slug)
	if err == nil {
		t.Errorf("CA-280: hallazgo 28c350 (%s): TaskDir(%q) debe devolver error, devolvio %q sin error", regla, slug, got)
	}
}

// t28Acepta exige que la puerta devuelva la carpeta wt (comparando rutas
// reales: la raiz puede venir por un symlink).
func t28Acepta(t *testing.T, caso, root, slug, wt string) {
	t.Helper()
	got, err := runcmd.TaskDir(root, slug)
	if err != nil {
		t.Fatalf("CA-280: guarda 28c350 (%s): TaskDir(%q) de un worktree real debe funcionar: %v", caso, slug, err)
	}
	if t28Real(t, got) != t28Real(t, wt) {
		t.Fatalf("CA-280: guarda 28c350 (%s): TaskDir(%q) = %q, la carpeta de la tarea es %q", caso, slug, got, wt)
	}
}

// Hallazgo 28c350, regla (a). CA-280: un slug que no es de item no es una
// tarea, aunque la ruta que arma EXISTA: ../../../victima (otro repo al lado
// del proyecto), ../worktrees/legit (el worktree real por otro camino), ..
// (.hoom), . (.hoom/worktrees), a/b, Mayus, -guion y 65 caracteres (como
// worktrees reales hechos a mano con git worktree add), legit/ (barra al
// final), un slug en blanco y una ruta absoluta.
func TestHallazgo28c350_CA280_TaskDirRechazaSlugInvalido(t *testing.T) {
	e := t28Proyecto(t)
	slug65 := strings.Repeat("c", 65)
	for i, raro := range []string{"a/b", "Mayus", "-guion", slug65} {
		t28Git(t, e.root, "worktree", "add", "-q", "-b", fmt.Sprintf("hoom/raro-%d", i), e.wt(raro), "main")
	}
	antes := t28Foto(t, e.victima)
	slugs := []string{
		"../../../" + t28NombreVictima,
		"../worktrees/" + t28Legit,
		"..", ".",
		"a/b", "Mayus", "-guion", slug65,
		t28Legit + "/",
		"   ", "\t",
		e.victima,
	}
	for _, slug := range slugs {
		if item.ValidSlug(slug) {
			t.Fatalf("fixture: %q deberia ser un slug invalido para item.ValidSlug", slug)
		}
		t28Rechaza(t, "a: slug invalido", e.root, slug)
	}
	if despues := t28Foto(t, e.victima); despues != antes {
		t.Fatalf("CA-280: hallazgo 28c350: TaskDir no toca nada, y la victima cambio")
	}
	t28Acepta(t, "junto a slugs invalidos", e.root, t28Legit, e.legit)
}

// Hallazgo 28c350, regla (b). CA-280: .hoom/worktrees/<slug> como symlink
// no es la carpeta de la tarea, apunte adonde apunte: a otro repo (ruta
// absoluta y relativa), a la raiz del proyecto (../..), al worktree real de
// otra tarea de ESTE repo (relativo y absoluto) o a un subdirectorio de la
// raiz.
func TestHallazgo28c350_CA280_TaskDirRechazaElSlugSymlink(t *testing.T) {
	e := t28Proyecto(t)
	t28Escribir(t, e.root, "pkg/x.go", "package pkg\n")
	casos := []struct{ slug, destino string }{
		{"a-otro-repo", e.victima},
		{"a-otro-repo-rel", "../../../" + t28NombreVictima},
		{"a-la-raiz", "../.."},
		{"a-la-raiz-abs", e.root},
		{"a-otra-tarea", t28Legit},
		{"a-otra-tarea-abs", e.legit},
		{"a-un-subdir", "../../pkg"},
	}
	for _, c := range casos {
		t28Symlink(t, c.destino, e.wt(c.slug))
	}
	antes := t28Foto(t, e.victima) + t28Foto(t, e.legit)
	for _, c := range casos {
		t28Rechaza(t, "b: "+c.slug+" -> "+c.destino, e.root, c.slug)
	}
	if despues := t28Foto(t, e.victima) + t28Foto(t, e.legit); despues != antes {
		t.Fatalf("CA-280: hallazgo 28c350: TaskDir no toca nada, y la victima o el worktree real cambiaron")
	}
	t28Acepta(t, "junto a symlinks", e.root, t28Legit, e.legit)
}

// Hallazgo 28c350, regla (b). CA-280: si .hoom/worktrees es un symlink, nada
// de lo que hay adentro es la carpeta de una tarea, aunque el destino sea un
// worktree REGISTRADO de este mismo repo (creado fuera con git worktree add).
func TestHallazgo28c350_CA280_TaskDirRechazaWorktreesSymlink(t *testing.T) {
	base := t28Real(t, t.TempDir())
	root := filepath.Join(base, "proyecto")
	t28Repo(t, root, true)
	externo := filepath.Join(base, "externo")
	if err := os.MkdirAll(externo, 0o755); err != nil {
		t.Fatal(err)
	}
	t28Git(t, root, "worktree", "add", "-q", "-b", "hoom/foo", filepath.Join(externo, "foo"), "main")
	t28Symlink(t, externo, filepath.Join(root, ".hoom", "worktrees"))
	if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "foo", "app.go")); err != nil {
		t.Fatalf("fixture: el worktree se ve a traves del symlink: %v", err)
	}
	t28Rechaza(t, "b: .hoom/worktrees symlink a un worktree registrado", root, "foo")
}

// Hallazgo 28c350, regla (b). CA-280: si .hoom es un symlink, tampoco: el
// destino es un worktree registrado de este repo, pero fuera del proyecto.
func TestHallazgo28c350_CA280_TaskDirRechazaHoomSymlink(t *testing.T) {
	base := t28Real(t, t.TempDir())
	root := filepath.Join(base, "proyecto")
	t28Repo(t, root, false)
	otro := filepath.Join(base, "otro-hoom")
	if err := os.MkdirAll(filepath.Join(otro, "worktrees"), 0o755); err != nil {
		t.Fatal(err)
	}
	t28Git(t, root, "worktree", "add", "-q", "-b", "hoom/foo", filepath.Join(otro, "worktrees", "foo"), "main")
	t28Symlink(t, otro, filepath.Join(root, ".hoom"))
	if _, err := os.Stat(filepath.Join(root, ".hoom", "worktrees", "foo", "app.go")); err != nil {
		t.Fatalf("fixture: el worktree se ve a traves del symlink: %v", err)
	}
	t28Rechaza(t, "b: .hoom symlink a un worktree registrado", root, "foo")
}

// Hallazgo 28c350, regla (c). CA-280: .hoom/worktrees/<slug> tiene que ser
// un worktree registrado de ESTE repo. No lo son: una carpeta comun, un clon
// aparte del proyecto, un repo nuevo, un worktree de OTRO repo, ni una
// carpeta comun que quedo en el lugar de un worktree registrado que se borro
// a mano (el registro de git sigue ahi, la carpeta ya no es un worktree).
func TestHallazgo28c350_CA280_TaskDirRechazaLoQueNoEsWorktreeDeEsteRepo(t *testing.T) {
	e := t28Proyecto(t)

	t28Escribir(t, e.wt("comun"), "x.go", "package x\n")

	t28Git(t, e.base, "clone", "-q", e.root, e.wt("clon"))

	t28Git(t, e.base, "init", "-q", "-b", "main", e.wt("repo-nuevo"))

	otro := filepath.Join(e.base, "otro")
	t28Repo(t, otro, false)
	t28Git(t, otro, "worktree", "add", "-q", "-b", "hoom/ajeno", e.wt("ajeno"), "main")

	t28Git(t, e.root, "worktree", "add", "-q", "-b", "hoom/viejo", e.wt("viejo"), "main")
	if err := os.RemoveAll(e.wt("viejo")); err != nil {
		t.Fatal(err)
	}
	t28Escribir(t, e.wt("viejo"), "app.go", "package app\n")
	if !strings.Contains(t28Git(t, e.root, "worktree", "list", "--porcelain"), e.wt("viejo")) {
		t.Fatalf("fixture: el registro del worktree viejo sigue en git")
	}

	for _, slug := range []string{"comun", "clon", "repo-nuevo", "ajeno", "viejo"} {
		if !item.ValidSlug(slug) {
			t.Fatalf("fixture: %q es un slug valido", slug)
		}
		t28Rechaza(t, "c: "+slug+" no es un worktree registrado de este repo", e.root, slug)
	}
	t28Acepta(t, "junto a carpetas que no son worktrees", e.root, t28Legit, e.legit)
}

// Guarda del hallazgo 28c350. CA-280: sin tarea la puerta devuelve root tal
// cual; un worktree real de taskcmd.Start da su carpeta, tambien en los
// bordes validos del slug (64 caracteres, empieza con numero) y aunque la
// RAIZ del proyecto se alcance por un symlink (el contrato prohibe symlinks
// en .hoom, .hoom/worktrees y .hoom/worktrees/<slug>, no en la ruta de la
// raiz: /var en macOS lo es, y un proyecto enlazado tambien).
func TestHallazgo28c350_CA280_TaskDirDelWorktreeRealSigueAndando(t *testing.T) {
	e := t28Proyecto(t)
	if got, err := runcmd.TaskDir(e.root, ""); err != nil || got != e.root {
		t.Fatalf("CA-280: guarda 28c350: sin tarea TaskDir devuelve root (%q), devolvio %q, %v", e.root, got, err)
	}
	t28Acepta(t, "worktree de task start", e.root, t28Legit, e.legit)

	for _, slug := range []string{strings.Repeat("b", 64), "9-lote"} {
		if err := taskcmd.Start(e.root, slug, "main"); err != nil {
			t.Fatalf("fixture: task start %q: %v", slug, err)
		}
		t28Acepta(t, "borde valido del slug", e.root, slug, e.wt(slug))
	}

	enlace := filepath.Join(e.base, "enlace-al-proyecto")
	t28Symlink(t, e.root, enlace)
	t28Acepta(t, "raiz por symlink, tarea creada por la ruta real", enlace, t28Legit, e.legit)
	if got, err := runcmd.TaskDir(enlace, ""); err != nil || got != enlace {
		t.Fatalf("CA-280: guarda 28c350: sin tarea TaskDir devuelve root tal cual (%q), devolvio %q, %v", enlace, got, err)
	}
	if err := taskcmd.Start(enlace, "por-el-enlace", "main"); err != nil {
		t.Fatalf("fixture: task start por el enlace: %v", err)
	}
	t28Acepta(t, "raiz por symlink, tarea creada por el enlace", enlace, "por-el-enlace", filepath.Join(enlace, ".hoom", "worktrees", "por-el-enlace"))
}

// t28Provider pone un `claude` falso que anota su cwd fisico en marca.
func t28Provider(t *testing.T) string {
	t.Helper()
	bin := t.TempDir()
	marca := filepath.Join(t.TempDir(), "corrio-en.txt")
	script := "#!/bin/sh\npwd -P >> '" + marca + "'\nexit 0\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return marca
}

// Hallazgo 28c350. CA-280: un run con una Task que la puerta rechaza no
// arranca y el provider no corre: ni con ../../../victima, ni con un symlink
// a otro repo o a la raiz, ni con una carpeta comun. La victima queda igual.
// Guarda: con la tarea real el provider corre en su worktree.
func TestHallazgo28c350_CA280_RunConTareaInvalidaNoLanzaNada(t *testing.T) {
	marca := t28Provider(t)
	e := t28Proyecto(t)
	t28Symlink(t, e.victima, e.wt("a-otro-repo"))
	t28Symlink(t, "../..", e.wt("a-la-raiz"))
	t28Symlink(t, t28Legit, e.wt("a-otra-tarea"))
	t28Escribir(t, e.wt("comun"), "x.go", "package x\n")
	antes := t28Foto(t, e.victima)

	m := runcmd.NewManager(e.root)
	for _, slug := range []string{"../../../" + t28NombreVictima, "..", "a-otro-repo", "a-la-raiz", "a-otra-tarea", "comun"} {
		run, err := m.Start(runcmd.StartOptions{Provider: "claude", Prompt: "hola", Task: slug})
		if err == nil {
			m.Wait(run.ID)
			t.Errorf("CA-280: hallazgo 28c350: un run con Task %q no debe arrancar (arranco %s)", slug, run.ID)
		}
		if raw, errLeer := os.ReadFile(marca); errLeer == nil {
			t.Errorf("CA-280: hallazgo 28c350: con Task %q el provider no debe correr, corrio en:\n%s", slug, raw)
			os.Remove(marca)
		}
	}
	if despues := t28Foto(t, e.victima); despues != antes {
		t.Fatalf("CA-280: hallazgo 28c350: un run rechazado no toca la victima")
	}

	run, err := m.Start(runcmd.StartOptions{Provider: "claude", Prompt: "hola", Task: t28Legit})
	if err != nil {
		t.Fatalf("CA-280: guarda 28c350: el run de la tarea real arranca: %v", err)
	}
	m.Wait(run.ID)
	raw, err := os.ReadFile(marca)
	if err != nil {
		t.Fatalf("CA-280: guarda 28c350: el provider de la tarea real corrio: %v", err)
	}
	if got := strings.TrimSpace(string(raw)); got != t28Real(t, e.legit) {
		t.Fatalf("CA-280: guarda 28c350: el run de la tarea real corre en su worktree (%s), corrio en %s", t28Real(t, e.legit), got)
	}
}
