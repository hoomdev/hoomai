// Tests de regresion de los hallazgos de la review cruzada de la cabina
// (spec C1 .hoom/specs/items-y-columna-derivada.md y C3
// .hoom/specs/acciones-desde-la-tarjeta.md), escritos desde el contrato:
//
//   - 20260924T201550_4c8572 (high) y 20260924T194924_540408 (low): Start,
//     Done, Discardable y RunDiscard (y los verbos hoom task start|done|
//     discard) rechazan todo slug que no sea un slug de item
//     (^[a-z0-9][a-z0-9-]*$, hasta 64: la regla de item.ValidSlug) con un
//     error que dice "slug invalido", y no tocan nada: ni un repo victima
//     alcanzado con ../, ni ramas, ni worktrees.
//   - 20260924T195800_5e42b8 (medium): dos task done concurrentes sobre la
//     misma tarea lista: uno solo cierra, y el que falla nunca deshace la
//     marca (hecho_en, commit_final) del que cerro.
//
// Repos git reales, todos bajo t.TempDir (las victimas tambien).
package taskcmd

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const h1SlugInvalido = "slug invalido"

// h1Slug65 y h1Slug64: uno de mas y el limite exacto de item.MaxSlug.
var (
	h1Slug65 = strings.Repeat("a", 65)
	h1Slug64 = strings.Repeat("b", 64)
)

// h1Formas son los slugs con forma invalida que no escapan de
// .hoom/worktrees/ (los que escapan van con sus victimas).
func h1Formas() []string {
	return []string{"a/b", ".", "..", "", "Mayus", "-guion", h1Slug65}
}

// h1Entorno es un proyecto dentro de base (base/proyecto) con una tarea
// legitima ya creada (asi .hoom/worktrees/ existe) y cuatro repos victima,
// cada uno con un archivo rastreado modificado y uno no rastreado:
//
//	../../victim      -> <root>/victim           (dentro de la raiz)
//	../victim         -> <root>/.hoom/victim     (dentro de la raiz)
//	../../../victim2  -> <base>/victim2          (fuera de la raiz)
//	<base>/victim-abs (ruta absoluta)            (fuera de la raiz)
type h1Entorno struct {
	base, root, legit string
	orden             []string          // los slugs hostiles, en orden fijo
	victimas          map[string]string // slug hostil -> directorio de la victima
}

func h1Proyecto(t *testing.T) h1Entorno {
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
	cbEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	cbEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	cbEscribir(t, root, "app.go", "package app\n")
	cbGit(t, root, "add", "-A")
	cbGit(t, root, "commit", "-q", "-m", "inicial")
	if err := Start(root, "legit", "main"); err != nil {
		t.Fatalf("fixture: task start legit: %v", err)
	}
	e := h1Entorno{base: base, root: root, legit: filepath.Join(root, ".hoom", "worktrees", "legit"),
		victimas: map[string]string{}}
	abs := filepath.Join(base, "victim-abs")
	e.orden = []string{"../../victim", "../victim", "../../../victim2", abs}
	dirs := []string{filepath.Join(root, "victim"), filepath.Join(root, ".hoom", "victim"), filepath.Join(base, "victim2"), abs}
	for i, slug := range e.orden {
		h1Victima(t, dirs[i])
		e.victimas[slug] = dirs[i]
	}
	return e
}

// h1Victima crea un repo git con un archivo rastreado MODIFICADO y uno NO
// RASTREADO: justo lo que un descarte restauraria y borraria.
func h1Victima(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cbGit(t, dir, "init", "-q", "-b", "main")
	cbGit(t, dir, "config", "user.email", "victima@hoom.dev")
	cbGit(t, dir, "config", "user.name", "victima")
	cbEscribir(t, dir, "tracked.go", "package victima\n")
	cbGit(t, dir, "add", "-A")
	cbGit(t, dir, "commit", "-q", "-m", "de la victima")
	cbEscribir(t, dir, "tracked.go", "package victima\n\n// trabajo sin guardar de otra persona\n")
	cbEscribir(t, dir, "untracked.go", "package victima\n\n// archivo nuevo de otra persona\n")
}

// h1FotoTodo junta la foto de cada victima, del worktree legitimo, de las
// ramas y de los worktrees del proyecto: dos fotos iguales = nada se toco.
func (e h1Entorno) h1FotoTodo(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	seccion := func(nombre, foto string) {
		for _, l := range strings.Split(foto, "\n") {
			fmt.Fprintf(&b, "[%s] %s\n", nombre, l)
		}
	}
	for _, s := range e.orden {
		seccion("victima "+s, cbFoto(t, e.victimas[s]))
	}
	seccion("legit", cbFoto(t, e.legit))
	seccion("ramas", cbGit(t, e.root, "branch", "--list", "--format=%(refname)"))
	seccion("worktrees", cbGit(t, e.root, "worktree", "list", "--porcelain"))
	entradas, _ := os.ReadDir(filepath.Join(e.root, ".hoom", "worktrees"))
	for _, en := range entradas {
		fmt.Fprintf(&b, "[wt-dir] %s\n", en.Name())
	}
	return b.String()
}

// h1Cambios resume que cambio entre dos fotos: las lineas que se fueron (-)
// y las que aparecieron (+).
func h1Cambios(antes, despues string) string {
	a := map[string]bool{}
	for _, l := range strings.Split(antes, "\n") {
		a[l] = true
	}
	d := map[string]bool{}
	for _, l := range strings.Split(despues, "\n") {
		d[l] = true
	}
	var out []string
	for _, l := range strings.Split(antes, "\n") {
		if !d[l] {
			out = append(out, "- "+l)
		}
	}
	for _, l := range strings.Split(despues, "\n") {
		if !a[l] {
			out = append(out, "+ "+l)
		}
	}
	return strings.Join(out, "\n")
}

func h1ExigeInvalido(t *testing.T, ca, que, slug string, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: %s con el slug %q debe fallar con %q, fue nil", ca, que, slug, h1SlugInvalido)
	}
	if !strings.Contains(err.Error(), h1SlugInvalido) {
		t.Fatalf("%s: %s con el slug %q debe fallar diciendo %q, fue: %v", ca, que, slug, h1SlugInvalido, err)
	}
}

// Hallazgo 20260924T201550_4c8572 (high). CA-346: `hoom task discard`
// descarta el espacio de trabajo DE LA TAREA; un slug que escapa de
// .hoom/worktrees/ (../../victim, ../victim, ../../../victim2, una ruta
// absoluta) no es una tarea: Discardable y RunDiscard (con y sin --yes, con
// --json) fallan con "slug invalido" y la victima no pierde ni el archivo
// rastreado modificado ni el no rastreado.
func TestCA346_H1Hallazgo201550_DiscardNoTocaUnRepoFueraDeLaTarea(t *testing.T) {
	e := h1Proyecto(t)
	for _, slug := range e.orden {
		antes := e.h1FotoTodo(t)
		_, err := Discardable(e.root, slug)
		h1ExigeInvalido(t, "CA-346", "Discardable", slug, err)
		var out bytes.Buffer
		err = RunDiscard(e.root, slug, false, false, &out)
		h1ExigeInvalido(t, "CA-346", "RunDiscard sin --yes", slug, err)
		if strings.Contains(out.String(), "tracked.go") {
			t.Fatalf("CA-346: sin --yes tampoco se listan archivos de la victima %q:\n%s", slug, out.String())
		}
		for _, asJSON := range []bool{false, true} {
			out.Reset()
			err = RunDiscard(e.root, slug, true, asJSON, &out)
			if despues := e.h1FotoTodo(t); despues != antes {
				t.Fatalf("CA-346: RunDiscard(--yes, json=%v) con el slug %q toco la victima u otra cosa:\n%s\nsalida:\n%s",
					asJSON, slug, h1Cambios(antes, despues), out.String())
			}
			h1ExigeInvalido(t, "CA-346", fmt.Sprintf("RunDiscard --yes (json=%v)", asJSON), slug, err)
		}
		if despues := e.h1FotoTodo(t); despues != antes {
			t.Fatalf("CA-346: con el slug %q no se toca nada:\n%s", slug, h1Cambios(antes, despues))
		}
	}
}

// Hallazgo 20260924T201550_4c8572 (high). CA-346: la primitiva Discard (la
// que usa el Studio) tampoco descarta fuera de la tarea, ni con expect nil ni
// con un expect igual a lo que listaria de la victima. (Extiende el contrato
// del hallazgo a la funcion que de verdad restaura y borra.)
func TestCA346_H1Hallazgo201550_DiscardPrimitivoNoSaleDeLaTarea(t *testing.T) {
	e := h1Proyecto(t)
	for _, slug := range e.orden {
		antes := e.h1FotoTodo(t)
		expect := []string{
			filepath.ToSlash(filepath.Join(".hoom", "worktrees", slug, "tracked.go")),
			filepath.ToSlash(filepath.Join(".hoom", "worktrees", slug, "untracked.go")),
		}
		for _, ex := range [][]string{nil, expect} {
			_, err := Discard(e.root, slug, ex)
			if despues := e.h1FotoTodo(t); despues != antes {
				t.Fatalf("CA-346: Discard con el slug %q (expect %v) toco la victima u otra cosa:\n%s", slug, ex, h1Cambios(antes, despues))
			}
			h1ExigeInvalido(t, "CA-346", "Discard", slug, err)
		}
	}
}

// Hallazgos 20260924T201550_4c8572 y 20260924T194924_540408. CA-346 y
// CA-263: las formas invalidas (a/b, ., .., vacio, Mayus, -guion, 65
// caracteres) son "slug invalido" en Discardable y RunDiscard AUNQUE exista
// un worktree con ese nombre hecho a mano (git worktree add): la forma se
// mira antes que el disco.
func TestCA346_H1Hallazgo194924_DiscardRechazaFormasInvalidas(t *testing.T) {
	e := h1Proyecto(t)
	for _, raro := range []string{"Mayus", h1Slug65} {
		wt := filepath.Join(e.root, ".hoom", "worktrees", raro)
		cbGit(t, e.root, "worktree", "add", "-q", "-b", "hoom/"+raro, wt, "main")
		cbEscribir(t, wt, "app.go", "package app\n\n// trabajo en un worktree con nombre raro\n")
		cbEscribir(t, wt, "nuevo.go", "package app\n")
	}
	antes := e.h1FotoTodo(t)
	for _, slug := range append(h1Formas(), filepath.Join(e.base, "victim-abs")) {
		_, err := Discardable(e.root, slug)
		h1ExigeInvalido(t, "CA-346", "Discardable", slug, err)
		var out bytes.Buffer
		err = RunDiscard(e.root, slug, false, false, &out)
		h1ExigeInvalido(t, "CA-346", "RunDiscard sin --yes", slug, err)
		out.Reset()
		err = RunDiscard(e.root, slug, true, true, &out)
		if despues := e.h1FotoTodo(t); despues != antes {
			t.Fatalf("CA-346: RunDiscard --yes con el slug %q toco algo:\n%s", slug, h1Cambios(antes, despues))
		}
		h1ExigeInvalido(t, "CA-346", "RunDiscard --yes --json", slug, err)
	}
}

// Hallazgos 20260924T201550_4c8572 y 20260924T194924_540408. CA-289 y
// CA-277: task done (con y sin --force) rechaza con "slug invalido" todo
// slug que no es de item, y no toca nada: ni victimas, ni el worktree
// legitimo, ni ramas.
func TestCA289_H1Hallazgo201550_DoneRechazaSlugInvalido(t *testing.T) {
	e := h1Proyecto(t)
	slugs := append(h1Formas(), e.orden...)
	antes := e.h1FotoTodo(t)
	for _, slug := range slugs {
		for _, force := range []bool{false, true} {
			err := Done(e.root, slug, "main", force)
			if despues := e.h1FotoTodo(t); despues != antes {
				t.Fatalf("CA-289: Done(force=%v) con el slug %q toco algo:\n%s", force, slug, h1Cambios(antes, despues))
			}
			h1ExigeInvalido(t, "CA-289", fmt.Sprintf("Done(force=%v)", force), slug, err)
		}
	}
}

// Hallazgos 20260924T201550_4c8572 y 20260924T194924_540408. CA-263: el
// slug de la tarea es el del item, hasta 64 caracteres. task start con 65
// (y con cualquier forma invalida, o que escapa) falla con "slug invalido"
// sin crear rama ni worktree. Guarda: 64 caracteres sigue funcionando.
func TestCA263_H1Hallazgo194924_StartRechazaSlugInvalido(t *testing.T) {
	e := h1Proyecto(t)
	slugs := append(h1Formas(), e.orden...)
	antes := e.h1FotoTodo(t)
	for _, slug := range slugs {
		err := Start(e.root, slug, "main")
		if despues := e.h1FotoTodo(t); despues != antes {
			t.Fatalf("CA-263: Start con el slug %q (%d caracteres) creo rama o worktree:\n%s", slug, len(slug), h1Cambios(antes, despues))
		}
		h1ExigeInvalido(t, "CA-263", "Start", slug, err)
	}
	if out := cbGit(t, e.root, "branch", "--list", "hoom/"+h1Slug65); out != "" {
		t.Fatalf("CA-263: no existe la rama de 65 caracteres: %q", out)
	}
}

// GUARDA (verde en la base). Hallazgos 20260924T201550_4c8572 y
// 20260924T194924_540408. CA-263 y CA-346: el limite exacto (64 caracteres)
// es un slug valido: Start crea rama y worktree, Discardable y RunDiscard lo
// descartan, y Done --force lo cierra.
func TestCA263_H1Hallazgo194924_Guarda64SigueFuncionando(t *testing.T) {
	e := h1Proyecto(t)
	if !item.ValidSlug(h1Slug64) || item.ValidSlug(h1Slug65) {
		t.Fatalf("CA-263 (guarda): item.ValidSlug acepta 64 y rechaza 65")
	}
	if err := Start(e.root, h1Slug64, "main"); err != nil {
		t.Fatalf("CA-263 (guarda): un slug valido de 64 caracteres crea la tarea: %v", err)
	}
	if out := cbGit(t, e.root, "branch", "--list", "hoom/"+h1Slug64); out == "" {
		t.Fatal("CA-263 (guarda): con 64 caracteres existe la rama hoom/<slug>")
	}
	if _, err := os.Stat(filepath.Join(e.root, ".hoom", "worktrees", h1Slug64)); err != nil {
		t.Fatalf("CA-263 (guarda): con 64 caracteres existe el worktree: %v", err)
	}
	wt := filepath.Join(e.root, ".hoom", "worktrees", h1Slug64)
	cbEscribir(t, wt, "sucio.go", "package app\n")
	got, err := Discardable(e.root, h1Slug64)
	if err != nil || !cbMismos(got, []string{".hoom/worktrees/" + h1Slug64 + "/sucio.go"}) {
		t.Fatalf("CA-346 (guarda): Discardable con 64 caracteres funciona: %v %v", got, err)
	}
	var out bytes.Buffer
	if err := RunDiscard(e.root, h1Slug64, true, false, &out); err != nil {
		t.Fatalf("CA-346 (guarda): RunDiscard --yes con 64 caracteres descarta: %v\n%s", err, out.String())
	}
	if _, ok := cbLeer(t, wt, "sucio.go"); ok {
		t.Fatal("CA-346 (guarda): el archivo sin guardar de la tarea de 64 se descarto")
	}
	if err := Done(e.root, h1Slug64, "main", true); err != nil {
		t.Fatalf("CA-289 (guarda): Done --force con 64 caracteres cierra: %v", err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("CA-289 (guarda): Done --force quito el worktree de 64 (err=%v)", err)
	}
}

// Hallazgo 20260924T201550_4c8572 (high), contra el BINARIO. CA-346 y
// CA-289: `hoom task discard <slug> --yes`, `hoom task done <slug>` y
// `hoom task start <slug>` rechazan el slug invalido (exit != 0, "slug
// invalido") y la victima queda igual. Guarda: start con 64 caracteres.
func TestCA346_H1Hallazgo201550_E2EVerbosRechazanSlugInvalido(t *testing.T) {
	bin := tdBinario(t)
	e := h1Proyecto(t)
	antes := e.h1FotoTodo(t)
	casos := [][]string{
		{"task", "discard", "../../victim", "--yes"},
		{"task", "discard", "../victim", "--yes", "--json"},
		{"task", "discard", "../../../victim2", "--yes"},
		{"task", "discard", filepath.Join(e.base, "victim-abs"), "--yes"},
		{"task", "discard", "../../victim"},
		{"task", "done", "../victim"},
		{"task", "done", "../../victim", "--force"},
		{"task", "done", "Mayus"},
		{"task", "start", h1Slug65},
		{"task", "start", "../../victim"},
	}
	for _, args := range casos {
		r := tdCorrer(t, bin, e.root, args...)
		if despues := e.h1FotoTodo(t); despues != antes {
			t.Fatalf("CA-346: hoom %v toco la victima, una rama o un worktree:\n%s\nstdout: %s\nstderr: %s",
				args, h1Cambios(antes, despues), r.stdout, r.stderr)
		}
		if r.exit == 0 {
			t.Fatalf("CA-346: hoom %v debe fallar, salio 0\nstdout: %s\nstderr: %s", args, r.stdout, r.stderr)
		}
		if !strings.Contains(r.stdout+r.stderr, h1SlugInvalido) {
			t.Fatalf("CA-346: hoom %v dice %q:\nstdout: %s\nstderr: %s", args, h1SlugInvalido, r.stdout, r.stderr)
		}
	}
}

// GUARDA (verde en la base). CA-263: con el binario, task start con un slug
// valido de 64 caracteres sigue creando la tarea.
func TestCA263_H1Hallazgo194924_E2EGuardaStart64(t *testing.T) {
	bin := tdBinario(t)
	e := h1Proyecto(t)
	tdOK(t, "CA-263 (guarda)", tdCorrer(t, bin, e.root, "task", "start", h1Slug64), "task", "start", h1Slug64)
	if out := cbGit(t, e.root, "branch", "--list", "hoom/"+h1Slug64); out == "" {
		t.Fatal("CA-263 (guarda): hoom task start con 64 caracteres crea la rama")
	}
}

// h1TareaLista arma, sin binario, una tarea lista para cerrar (limpia, con
// un veredicto completo verde de la huella actual, todo commiteado) y el
// item de la tarjeta en el arbol raiz: lo que CA-343/CA-289 cierran.
func h1TareaLista(t *testing.T, slug string) (root, wt, itemPath, sha string) {
	t.Helper()
	root, wt = rdTarea(t, slug)
	rdEscribir(t, wt, "feature.go", "package app\n\nfunc Feature() int { return 1 }\n")
	rdCommit(t, wt, "feature")
	rdVeredicto(t, wt, time.Now().UTC(), verdict.StatusPass, false, "")
	rdCommit(t, wt, "veredicto verde")
	if err := Ready(root, slug, "main"); err != nil {
		t.Fatalf("fixture: la tarea %s debe estar lista: %v", slug, err)
	}
	rdEscribir(t, root, ".hoom/items/"+slug+".yaml", tdItemYAML("Concurrente", ""))
	itemPath = filepath.Join(root, ".hoom", "items", slug+".yaml")
	sha = rdGit(t, root, "rev-parse", "hoom/"+slug)
	return root, wt, itemPath, sha
}

// Hallazgo 20260924T195800_5e42b8 (medium). CA-343 y CA-289 (y CA-279: con
// hecho_en la tarjeta es Hecho): dos task done concurrentes sobre la misma
// tarea lista. Exactamente uno devuelve nil, y el item termina con hecho_en
// y commit_final = el sha de hoom/<slug>: el que fallo nunca deshace la marca
// del que cerro. Adversarial: se repite con desfases chicos entre los dos.
func TestCA343_H1Hallazgo5e42b8_DoneConcurrenteNoDeshaceLaMarca(t *testing.T) {
	desfases := []time.Duration{0, 0, 0, 200 * time.Microsecond, 500 * time.Microsecond,
		time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond, 5 * time.Millisecond,
		8 * time.Millisecond, 12 * time.Millisecond, 20 * time.Millisecond}
	var fallas []string
	for i, d := range desfases {
		slug := fmt.Sprintf("concurrente-%d", i)
		root, wt, itemPath, sha := h1TareaLista(t, slug)

		var wg sync.WaitGroup
		largada := make(chan struct{})
		errs := make([]error, 2)
		for k := 0; k < 2; k++ {
			wg.Add(1)
			go func(k int) {
				defer wg.Done()
				<-largada
				if k == 1 && d > 0 {
					time.Sleep(d)
				}
				errs[k] = Done(root, slug, "main", false)
			}(k)
		}
		close(largada)
		wg.Wait()

		nils := 0
		for _, err := range errs {
			if err == nil {
				nils++
			}
		}
		m := map[string]any{}
		raw, err := os.ReadFile(itemPath)
		if err == nil {
			err = yaml.Unmarshal(raw, &m)
		}
		_, wtErr := os.Stat(wt)
		var mal []string
		if nils != 1 {
			mal = append(mal, fmt.Sprintf("%d Done devolvieron nil", nils))
		}
		if err != nil {
			mal = append(mal, fmt.Sprintf("el item no se lee: %v", err))
		} else if m["hecho_en"] == nil || m["commit_final"] != sha {
			mal = append(mal, fmt.Sprintf("el item perdio la marca (hecho_en=%v commit_final=%v, sha %s)", m["hecho_en"], m["commit_final"], sha))
		}
		if nils > 0 && !os.IsNotExist(wtErr) {
			mal = append(mal, fmt.Sprintf("uno cerro pero el worktree sigue (%v)", wtErr))
		}
		if len(mal) > 0 {
			fallas = append(fallas, fmt.Sprintf("desfase %v: %s (errs %v)", d, strings.Join(mal, "; "), errs))
		}
	}
	if len(fallas) > 0 {
		t.Fatalf("CA-343: dos task done concurrentes: uno cierra y la marca queda (%d de %d rondas fallaron):\n  %s",
			len(fallas), len(desfases), strings.Join(fallas, "\n  "))
	}
}
