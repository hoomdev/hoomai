// Tests adversariales del hallazgo 20261020T201503_28c350 (high, risk) de la
// review de los fixes de C1 (.hoom/specs/items-y-columna-derivada.md: CA-280,
// la evidencia y el trabajo de la tarjeta estan en el worktree
// .hoom/worktrees/<slug>; CA-286, lo sin sincronizar del worktree), sobre
// `hoom item save` (CA-344 de .hoom/specs/acciones-desde-la-tarjeta.md, el
// "Guardar" del Studio). Escritos desde el contrato "puerta completa", nunca
// desde la implementacion:
//
// itemcmd.Save pasa por runcmd.TaskDir, la unica puerta que decide la
// carpeta de una tarea. Con .hoom/worktrees/<slug> como symlink a OTRO repo
// o a la raiz, o como una carpeta comun que no es un worktree registrado de
// este repo, Save falla: no commitea nada en ningun arbol (ni en el otro
// repo, ni en la raiz), no corre los hooks del otro repo y el trabajo sin
// commitear queda como estaba. Guarda: con el worktree real de
// taskcmd.Start, Save sigue guardando.
//
// Repos git reales bajo t.TempDir (la victima tambien).
package itemcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

const t28NombreVictima = "victima-28c350"

// t28Entorno: base/proyecto (con la tarea legit de taskcmd.Start) y
// base/victima-28c350, un repo con trabajo ajeno sin commitear y un hook
// pre-commit que deja una marca si alguien commitea ahi.
type t28Entorno struct {
	base, root, legit, victima, marcaHook string
}

func (e t28Entorno) wt(slug string) string { return filepath.Join(e.root, ".hoom", "worktrees", slug) }

func t28Proyecto(t *testing.T) t28Entorno {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(base, "proyecto")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cbGit(t, root, "init", "-q", "-b", "main")
	cbGit(t, root, "config", "user.email", "test@hoom.dev")
	cbGit(t, root, "config", "user.name", "hoom test")
	icEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	icEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	icEscribir(t, root, "app.go", "package app\n")
	cbGit(t, root, "add", "-A")
	cbGit(t, root, "commit", "-q", "-m", "inicial")
	if err := taskcmd.Start(root, "legit", "main"); err != nil {
		t.Fatalf("fixture: task start legit: %v", err)
	}
	e := t28Entorno{base: base, root: root, legit: filepath.Join(root, ".hoom", "worktrees", "legit"),
		victima: filepath.Join(base, t28NombreVictima), marcaHook: filepath.Join(base, "hook-de-la-victima-corrio")}

	v := e.victima
	if err := os.MkdirAll(v, 0o755); err != nil {
		t.Fatal(err)
	}
	cbGit(t, v, "init", "-q", "-b", "main")
	cbGit(t, v, "config", "user.email", "victima@hoom.dev")
	cbGit(t, v, "config", "user.name", "victima")
	icEscribir(t, v, "tracked.go", "package victima\n")
	cbGit(t, v, "add", "-A")
	cbGit(t, v, "commit", "-q", "-m", "de la victima")
	icEscribir(t, v, "tracked.go", "package victima\n\n// trabajo sin guardar de otra persona\n")
	icEscribir(t, v, "untracked.go", "package victima\n\n// archivo nuevo de otra persona\n")
	hook := filepath.Join(v, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\ntouch '"+e.marcaHook+"'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	// trabajo sin commitear de la persona en la raiz
	icEscribir(t, root, "app.go", "package app\n\n// trabajo sin guardar en la raiz\n")
	icEscribir(t, root, "raiz-nuevo.go", "package app\n")
	return e
}

// t28Item deja el item de slug sin commitear en la raiz: la tarjeta existe.
func t28Item(t *testing.T, root, slug string) {
	t.Helper()
	icEscribir(t, root, ".hoom/items/"+slug+".yaml", icItemYAML("Tarjeta "+slug, "2026-10-20T10:00:00Z", ""))
}

// t28Estado: HEAD y status de un arbol (status con todo lo no rastreado).
func t28Estado(t *testing.T, dir string) string {
	t.Helper()
	return cbHead(t, dir) + "\n" + cbGit(t, dir, "status", "--porcelain", "--untracked-files=all")
}

// t28Vigilar fotografia la raiz, la victima y la tarea real de e, y
// devuelve la funcion que exige que nada de eso haya cambiado y que el hook
// de la victima no haya corrido.
func t28Vigilar(t *testing.T, e t28Entorno, slug string) func(como string) {
	t.Helper()
	dirs := map[string]string{"raiz": e.root, "victima": e.victima, "legit": e.legit}
	antes := map[string]string{}
	for nombre, dir := range dirs {
		antes[nombre] = t28Estado(t, dir)
	}
	return func(como string) {
		t.Helper()
		for nombre, dir := range dirs {
			if ahora := t28Estado(t, dir); ahora != antes[nombre] {
				t.Errorf("CA-280: hallazgo 28c350: %s de %q cambio el arbol %s:\nantes:\n%s\ndespues:\n%s", como, slug, nombre, antes[nombre], ahora)
			}
		}
		if _, err := os.Stat(e.marcaHook); err == nil {
			t.Errorf("CA-280: hallazgo 28c350: %s de %q corrio el hook pre-commit de la victima", como, slug)
		}
	}
}

// t28SaveRechazado exige que guardar la tarjeta slug falle, por la
// primitiva y por el verbo `hoom item save` (cada uno en un entorno nuevo
// de armar), sin commitear nada en la raiz, en la victima ni en la tarea
// real, y sin correr el hook de la victima.
func t28SaveRechazado(t *testing.T, armar func(t *testing.T) t28Entorno, slug string) {
	t.Helper()
	e := armar(t)
	t28Item(t, e.root, slug)
	revisar := t28Vigilar(t, e, slug)
	res, err := Save(e.root, "main", "high", slug, nil)
	if err == nil {
		t.Errorf("CA-280: hallazgo 28c350: Save de %q debe fallar (la carpeta no es un worktree de la tarea), guardo: %+v", slug, res)
	}
	revisar("Save")

	e = armar(t)
	t28Item(t, e.root, slug)
	revisar = t28Vigilar(t, e, slug)
	req, perr := Parse([]string{"save", slug})
	if perr != nil {
		t.Fatalf("fixture: Parse(save %s): %v", slug, perr)
	}
	var out, errb strings.Builder
	err = Execute(e.root, "main", "high", req, &out, &errb)
	if err == nil {
		t.Errorf("CA-280: hallazgo 28c350: `hoom item save %s` debe fallar, salio bien:\n%s", slug, out.String())
	}
	for _, ajeno := range []string{"tracked.go", "untracked.go", "raiz-nuevo.go"} {
		if strings.Contains(out.String()+errb.String(), ajeno) {
			t.Errorf("CA-280: hallazgo 28c350: `hoom item save %s` no nombra archivos ajenos (%s):\n%s%s", slug, ajeno, out.String(), errb.String())
			break
		}
	}
	revisar("`hoom item save`")
}

// t28ConSymlink arma el proyecto con .hoom/worktrees/<slug> -> destino
// (destino "" = la victima por ruta absoluta).
func t28ConSymlink(slug, destino string) func(t *testing.T) t28Entorno {
	return func(t *testing.T) t28Entorno {
		e := t28Proyecto(t)
		d := destino
		if d == "" {
			d = e.victima
		}
		if err := os.Symlink(d, e.wt(slug)); err != nil {
			t.Fatal(err)
		}
		return e
	}
}

// Hallazgo 28c350 (high). CA-280 y CA-286: con .hoom/worktrees/<slug> como
// symlink a OTRO repo (absoluto y relativo), guardar la tarjeta no commitea
// en ese repo ni corre sus hooks, y su trabajo sin commitear queda igual.
func TestHallazgo28c350_CA280_SaveNoCommiteaEnOtroRepo(t *testing.T) {
	t.Run("a-otro-repo", func(t *testing.T) {
		t28SaveRechazado(t, t28ConSymlink("a-otro-repo", ""), "a-otro-repo")
	})
	t.Run("a-otro-repo-rel", func(t *testing.T) {
		t28SaveRechazado(t, t28ConSymlink("a-otro-repo-rel", "../../../"+t28NombreVictima), "a-otro-repo-rel")
	})
}

// Hallazgo 28c350 (high). CA-280 y CA-286: con .hoom/worktrees/<slug> como
// symlink a la RAIZ o al worktree real de OTRA tarea, o como una carpeta
// comun (git ahi adentro ve la raiz), guardar la tarjeta no commitea el
// trabajo de la persona en la raiz ni el de la otra tarea.
func TestHallazgo28c350_CA280_SaveNoCommiteaLaRaizPorUnaCarpetaFalsa(t *testing.T) {
	t.Run("a-la-raiz", func(t *testing.T) {
		t28SaveRechazado(t, t28ConSymlink("a-la-raiz", "../.."), "a-la-raiz")
	})
	t.Run("comun", func(t *testing.T) {
		t28SaveRechazado(t, func(t *testing.T) t28Entorno {
			e := t28Proyecto(t)
			icEscribir(t, e.wt("comun"), "x.go", "package x\n")
			return e
		}, "comun")
	})
	t.Run("a-otra-tarea", func(t *testing.T) {
		t28SaveRechazado(t, func(t *testing.T) t28Entorno {
			e := t28ConSymlink("a-otra-tarea", "legit")(t)
			icEscribir(t, e.legit, "app.go", "package app\n\n// trabajo sin guardar de legit\n")
			return e
		}, "a-otra-tarea")
	})
}

// Guarda del hallazgo 28c350. CA-280: con el worktree real de
// taskcmd.Start (y un symlink hostil al lado), Save guarda la tarjeta: un
// commit en el worktree con su cambio y el item en la raiz; la victima y el
// resto del trabajo de la raiz quedan igual.
func TestHallazgo28c350_CA280_SaveDelWorktreeRealSigueAndando(t *testing.T) {
	e := t28Proyecto(t)
	if err := os.Symlink(e.victima, e.wt("a-otro-repo")); err != nil {
		t.Fatal(err)
	}
	t28Item(t, e.root, "legit")
	icEscribir(t, e.legit, "app.go", "package app\n\n// trabajo de la tarea\n")
	headWt, victima := cbHead(t, e.legit), t28Estado(t, e.victima)

	res, err := Save(e.root, "main", "high", "legit", nil)
	if err != nil {
		t.Fatalf("CA-280: guarda 28c350: Save del worktree real funciona: %v", err)
	}
	if cbHead(t, e.legit) == headWt || len(res.Commits) == 0 {
		t.Fatalf("CA-280: guarda 28c350: Save commitea en el worktree real: %+v", res)
	}
	if got := cbDelCommit(t, e.legit); !cbMismos(got, []string{"app.go"}) {
		t.Fatalf("CA-280: guarda 28c350: el commit del worktree lleva su cambio: %v", got)
	}
	if st := cbGit(t, e.legit, "status", "--porcelain", "--untracked-files=all"); st != "" {
		t.Fatalf("CA-280: guarda 28c350: el worktree real queda guardado:\n%s", st)
	}
	if t28Estado(t, e.victima) != victima {
		t.Fatalf("CA-280: guarda 28c350: guardar la tarea real no toca la victima")
	}
	if _, err := os.Stat(e.marcaHook); err == nil {
		t.Fatalf("CA-280: guarda 28c350: guardar la tarea real no corre hooks de la victima")
	}
	st := cbGit(t, e.root, "status", "--porcelain", "--untracked-files=all")
	if !strings.Contains(st, "app.go") || !strings.Contains(st, "raiz-nuevo.go") {
		t.Fatalf("CA-280: guarda 28c350: el trabajo ajeno de la raiz sigue sin commitear:\n%s", st)
	}
}
