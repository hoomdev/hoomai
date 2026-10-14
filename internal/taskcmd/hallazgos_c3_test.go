// Tests de regresion de cuatro hallazgos de la review de C3
// (.hoom/specs/acciones-desde-la-tarjeta.md) sobre Descartar: taskcmd.Discard
// y `hoom task discard <slug> --yes` (CA-346). Escritos desde el contrato y
// los hallazgos, sin leer la implementacion:
//
//   - 20261014T180010_3113eb (high): los nombres de archivo son literales. Un
//     archivo llamado como un pathspec de git (:(glob)**, :!app.go, :(top)x,
//     *.go) es ese archivo y nada mas: descartarlo no toca ningun otro archivo
//     rastreado (ni en el indice ni en el disco) ni nada bajo .hoom/.
//   - 20261014T180752_b1b1bc (high): Descartar nunca borra un archivo que
//     HEAD tiene. Si git no puede leer lo que HEAD tiene de una ruta (su
//     objeto falta del repo), Descartar falla y no toca nada; un submodulo
//     modificado (su commit nunca esta en el repo de arriba) nunca se borra.
//   - 20261014T180436_f237b9 (high): "con un run activo en el arbol se niega"
//     vale tambien para un run lanzado desde ADENTRO del espacio de trabajo,
//     cuyo registro vive en <worktree>/.hoom/runs/ y no en la raiz.
//   - 20261014T180752_646e6f (medium): si git no puede sacar del indice un
//     archivo nuevo (el indice del worktree esta bloqueado), Descartar
//     devuelve error y el archivo sigue en el disco: su contenido no se
//     pierde.
//
// Repos git reales bajo t.TempDir. Reusa los ayudantes de discard_test.go
// (cbGit, cbEscribir, cbLeer, cbMismos, cbSlug).
package taskcmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// Evidencia sin guardar que h3Tarea deja bajo .hoom/ del worktree.
const (
	h3Veredicto     = ".hoom/verdicts/20261014T180010_h3v001.json"
	h3VeredictoBody = "{\"verdict\":\"green\"}\n"
	h3SpecBody      = "# Spec: precios\n\nCA nuevo sin commitear\n"
	h3Otra          = ".hoom/specs/otra.md"
	h3OtraBody      = "# Spec: otra\n"
)

// h3Rel es la ruta de rel (relativa al worktree) relativa a la raiz, como la
// devuelven Discardable y DiscardResult.
func h3Rel(rel string) string { return ".hoom/worktrees/" + cbSlug + "/" + rel }

// h3Foto es el estado completo del worktree, una linea por hecho: status, el
// indice exacto (modo, objeto, etapa y ruta de cada entrada) y el contenido
// de cada archivo. Dos fotos iguales = ni el indice ni el disco cambiaron.
//
// En un worktree .git es un ARCHIVO: se saltea con nil y no con SkipDir (en
// un archivo, SkipDir saltea el resto de su directorio, o sea casi toda la
// raiz del worktree).
func h3Foto(t *testing.T, wt string) string {
	t.Helper()
	var b strings.Builder
	for _, sec := range []struct {
		nombre string
		args   []string
	}{
		{"status", []string{"status", "--porcelain", "--untracked-files=all"}},
		{"indice", []string{"ls-files", "--stage"}},
	} {
		for _, l := range strings.Split(cbGit(t, wt, sec.args...), "\n") {
			if l != "" {
				fmt.Fprintf(&b, "%s: %s\n", sec.nombre, l)
			}
		}
	}
	err := filepath.WalkDir(wt, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(wt, p)
		fmt.Fprintf(&b, "disco: %s=%q\n", rel, raw)
		return nil
	})
	if err != nil {
		t.Fatalf("foto de %s: %v", wt, err)
	}
	return b.String()
}

// h3Cambios lista lo que distingue dos fotos: "- " lo que estaba y ya no,
// "+ " lo que aparecio.
func h3Cambios(antes, despues string) string {
	cuenta := map[string]int{}
	for _, l := range strings.Split(antes, "\n") {
		cuenta[l]++
	}
	for _, l := range strings.Split(despues, "\n") {
		cuenta[l]--
	}
	var menos, mas []string
	for l, n := range cuenta {
		for ; n > 0; n-- {
			menos = append(menos, "- "+l)
		}
		for ; n < 0; n++ {
			mas = append(mas, "+ "+l)
		}
	}
	sort.Strings(menos)
	sort.Strings(mas)
	return strings.Join(append(menos, mas...), "\n")
}

// h3Tarea arma la tarea precios (como `hoom task start`) con HEAD = la base
// (app.go, lib.go, x, pkg/p.go, hoom.yaml y .hoom/) mas enHEAD. preparar, si
// no es nil, corre en la raiz antes del commit inicial. Deja en el worktree
// evidencia sin guardar bajo .hoom/ de las tres formas: un veredicto nuevo
// sin rastrear, el spec rastreado y modificado y un spec nuevo en el indice.
// Fuera de .hoom/ el worktree queda limpio.
func h3Tarea(t *testing.T, enHEAD map[string]string, preparar func(root string)) (root, wt string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "proyecto")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cbGit(t, root, "init", "-q", "-b", "main")
	cbGit(t, root, "config", "user.email", "test@hoom.dev")
	cbGit(t, root, "config", "user.name", "hoom test")
	cbEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	cbEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	cbEscribir(t, root, ".hoom/specs/"+cbSlug+".md", "# Spec: precios\n")
	cbEscribir(t, root, "app.go", "package app\n")
	cbEscribir(t, root, "lib.go", "package app\n\nfunc Lib() {}\n")
	cbEscribir(t, root, "x", "x de la raiz\n")
	cbEscribir(t, root, "pkg/p.go", "package pkg\n")
	for rel, body := range enHEAD {
		cbEscribir(t, root, rel, body)
	}
	if preparar != nil {
		preparar(root)
	}
	cbGit(t, root, "add", "-A")
	cbGit(t, root, "commit", "-q", "-m", "inicial")
	if err := Start(root, cbSlug, "main"); err != nil {
		t.Fatalf("CA-346: fixture: task start: %v", err)
	}
	wt = filepath.Join(root, ".hoom", "worktrees", cbSlug)

	cbEscribir(t, wt, h3Veredicto, h3VeredictoBody)
	cbEscribir(t, wt, ".hoom/specs/"+cbSlug+".md", h3SpecBody)
	cbEscribir(t, wt, h3Otra, h3OtraBody)
	cbGit(t, wt, "add", "--", h3Otra)
	return root, wt
}

// h3Evidencia exige que la evidencia que dejo h3Tarea siga en el disco igual.
func h3Evidencia(t *testing.T, hallazgo, wt string) {
	t.Helper()
	for rel, want := range map[string]string{
		h3Veredicto:                     h3VeredictoBody,
		".hoom/specs/" + cbSlug + ".md": h3SpecBody,
		h3Otra:                          h3OtraBody,
	} {
		if got, ok := cbLeer(t, wt, rel); !ok || got != want {
			t.Fatalf("hallazgo %s (CA-346): Descartar nunca toca nada bajo .hoom/: %s quedo %q (existe: %v), tenia que seguir %q", hallazgo, rel, got, ok, want)
		}
	}
}

// h3Descartable exige, como precondicion, que lo descartable sea want.
func h3Descartable(t *testing.T, hallazgo, root string, want []string) {
	t.Helper()
	got, err := Discardable(root, cbSlug)
	if err != nil {
		t.Fatalf("hallazgo %s (CA-346): Discardable: %v", hallazgo, err)
	}
	if !cbMismos(got, want) {
		t.Fatalf("hallazgo %s (CA-346): lo descartable son las rutas sin guardar fuera de .hoom/, tal cual se llaman:\n got  %q\n want %q", hallazgo, got, want)
	}
}

// h3Descartar es `hoom task discard precios --yes --json`: el resultado
// (solo si no hubo error), la salida y el error.
func h3Descartar(t *testing.T, root string) (DiscardResult, string, error) {
	t.Helper()
	var out bytes.Buffer
	err := RunDiscard(root, cbSlug, true, true, &out)
	var res DiscardResult
	if err == nil {
		if e := json.Unmarshal(bytes.TrimSpace(out.Bytes()), &res); e != nil {
			t.Fatalf("CA-346: hoom task discard --yes --json emite un objeto JSON: %v\n%s", e, out.String())
		}
	}
	return res, out.String(), err
}

// h3Nombres son nombres de archivo que, leidos como pathspec de git en vez
// de como rutas literales, nombran OTRA cosa: todo el arbol (:(glob)**), todo
// menos app.go (:!app.go), el x de la raiz (:(top)x) y todos los .go (*.go).
var h3Nombres = []string{":(glob)**", ":!app.go", ":(top)x", "*.go"}

// Hallazgo 20261014T180010_3113eb (high), CA-346: los nombres de archivo son
// literales. Un archivo nuevo sin rastrear con nombre de pathspec es lo unico
// descartable fuera de .hoom/; `hoom task discard --yes` borra ESE archivo y
// nada mas: ningun otro archivo rastreado queda marcado como borrado ni
// cambia en el indice ni en el disco, y la evidencia bajo .hoom/ (veredicto
// sin commitear, spec modificado, spec nuevo en el indice) sigue igual.
func TestHallazgo_3113eb_CA346_NombreLiteralNuevoSinRastrear(t *testing.T) {
	for _, nombre := range h3Nombres {
		t.Run(nombre, func(t *testing.T) {
			root, wt := h3Tarea(t, nil, nil)
			esperado := h3Foto(t, wt) // sin el archivo: como tiene que quedar
			cbEscribir(t, wt, nombre, "trabajo nuevo\n")
			h3Descartable(t, "3113eb", root, []string{h3Rel(nombre)})

			res, out, err := h3Descartar(t, root)
			if err != nil {
				t.Fatalf("hallazgo 3113eb (CA-346): descartar el archivo nuevo %q falla: %v\n%s", nombre, err, out)
			}
			if len(res.Restored) != 0 || !cbMismos(res.Removed, []string{h3Rel(nombre)}) {
				t.Fatalf("hallazgo 3113eb (CA-346): se borra solo el archivo literal %q (HEAD no lo tiene): restored %q removed %q", nombre, res.Restored, res.Removed)
			}
			if _, ok := cbLeer(t, wt, nombre); ok {
				t.Fatalf("hallazgo 3113eb (CA-346): el archivo literal %q no esta en HEAD y se borra", nombre)
			}
			h3Evidencia(t, "3113eb", wt)
			if despues := h3Foto(t, wt); despues != esperado {
				t.Fatalf("hallazgo 3113eb (CA-346): descartar %q toca solo ese archivo literal: ningun otro archivo rastreado cambia en el indice ni en el disco y .hoom/ sigue igual (- antes, + despues):\n%s", nombre, h3Cambios(esperado, despues))
			}
		})
	}
}

// Hallazgo 20261014T180010_3113eb (high), CA-346: igual, con el archivo nuevo
// agregado al indice (git add literal). Descartar lo saca del indice y del
// disco a EL y a nada mas.
func TestHallazgo_3113eb_CA346_NombreLiteralNuevoEnElIndice(t *testing.T) {
	for _, nombre := range h3Nombres {
		t.Run(nombre, func(t *testing.T) {
			root, wt := h3Tarea(t, nil, nil)
			esperado := h3Foto(t, wt)
			cbEscribir(t, wt, nombre, "trabajo nuevo en el indice\n")
			cbGit(t, wt, "--literal-pathspecs", "add", "--", nombre)
			if idx := cbGit(t, wt, "ls-files", "--stage"); !strings.Contains(idx, "\t"+nombre+"\n") && !strings.HasSuffix(idx, "\t"+nombre) {
				t.Fatalf("CA-346: fixture: %q tiene que quedar en el indice:\n%s", nombre, idx)
			}
			h3Descartable(t, "3113eb", root, []string{h3Rel(nombre)})

			res, out, err := h3Descartar(t, root)
			if err != nil {
				t.Fatalf("hallazgo 3113eb (CA-346): descartar el archivo nuevo en el indice %q falla: %v\n%s", nombre, err, out)
			}
			if len(res.Restored) != 0 || !cbMismos(res.Removed, []string{h3Rel(nombre)}) {
				t.Fatalf("hallazgo 3113eb (CA-346): se borra solo el archivo literal %q (HEAD no lo tiene): restored %q removed %q", nombre, res.Restored, res.Removed)
			}
			h3Evidencia(t, "3113eb", wt)
			if despues := h3Foto(t, wt); despues != esperado {
				t.Fatalf("hallazgo 3113eb (CA-346): descartar %q lo saca del indice y del disco a el solo: ningun otro archivo rastreado queda marcado como borrado ni cambia, y .hoom/ sigue igual (- antes, + despues):\n%s", nombre, h3Cambios(esperado, despues))
			}
		})
	}
}

// Hallazgo 20261014T180010_3113eb (high), CA-346: igual, con el archivo
// commiteado en HEAD y modificado, mas un cambio sin commitear en otro
// archivo rastreado (app.go, que tambien es descartable) y la evidencia bajo
// .hoom/. Descartar vuelve a HEAD exactamente esos dos archivos: el literal
// queda como esta en HEAD y .hoom/ (spec modificado, spec nuevo en el indice,
// veredicto) sigue igual.
func TestHallazgo_3113eb_CA346_NombreLiteralEnHEADModificado(t *testing.T) {
	const enHEAD, trabajo = "como esta en HEAD\n", "trabajo sin guardar\n"
	for _, nombre := range h3Nombres {
		t.Run(nombre, func(t *testing.T) {
			root, wt := h3Tarea(t, map[string]string{nombre: enHEAD}, nil)
			esperado := h3Foto(t, wt) // fuera de .hoom/ todo como en HEAD
			cbEscribir(t, wt, nombre, trabajo)
			cbEscribir(t, wt, "app.go", "package app\n\nfunc Roto() {}\n")
			h3Descartable(t, "3113eb", root, []string{h3Rel(nombre), h3Rel("app.go")})

			res, out, err := h3Descartar(t, root)
			if err != nil {
				t.Fatalf("hallazgo 3113eb (CA-346): descartar %q (en HEAD, modificado) y app.go falla: %v\n%s", nombre, err, out)
			}
			if !cbMismos(res.Restored, []string{h3Rel(nombre), h3Rel("app.go")}) || len(res.Removed) != 0 {
				t.Fatalf("hallazgo 3113eb (CA-346): se restauran %q y app.go, que HEAD tiene, y no se borra nada: restored %q removed %q", nombre, res.Restored, res.Removed)
			}
			if got, ok := cbLeer(t, wt, nombre); !ok || got != enHEAD {
				t.Fatalf("hallazgo 3113eb (CA-346): el archivo literal %q vuelve a como esta en HEAD: %q (existe: %v)", nombre, got, ok)
			}
			h3Evidencia(t, "3113eb", wt)
			if despues := h3Foto(t, wt); despues != esperado {
				t.Fatalf("hallazgo 3113eb (CA-346): descartar %q y app.go vuelve a HEAD esos dos y nada mas: .hoom/ sigue igual en el indice y en el disco (- antes, + despues):\n%s", nombre, h3Cambios(esperado, despues))
			}
		})
	}
}

// h3Secreto es un contenido que ningun otro archivo del fixture tiene: su
// blob es solo de secreto.go.
const h3Secreto = "package app\n\n// b1b1bc: contenido unico de HEAD\n"

// h3SinObjeto borra del almacen de objetos del proyecto el blob que HEAD
// tiene para rel: HEAD sigue nombrando la ruta, pero git ya no puede leer lo
// que tiene.
func h3SinObjeto(t *testing.T, root, wt, rel string) {
	t.Helper()
	sha := cbGit(t, wt, "rev-parse", "HEAD:"+rel)
	obj := filepath.Join(root, ".git", "objects", sha[:2], sha[2:])
	if err := os.Remove(obj); err != nil {
		t.Fatalf("CA-346: fixture: el blob de %s tiene que estar suelto en %s: %v", rel, obj, err)
	}
	if ls := cbGit(t, wt, "ls-tree", "HEAD", "--", rel); !strings.Contains(ls, sha) {
		t.Fatalf("CA-346: fixture: HEAD sigue teniendo %s: %q", rel, ls)
	}
	cmd := exec.Command("git", "cat-file", "-e", "HEAD:"+rel)
	cmd.Dir = wt
	if cmd.Run() == nil {
		t.Fatalf("CA-346: fixture: el objeto de HEAD:%s tiene que faltar", rel)
	}
}

// Hallazgo 20261014T180752_b1b1bc, CA-346: Descartar nunca borra un archivo
// que HEAD tiene. Si git no puede leer lo que HEAD tiene de secreto.go (falta
// su objeto del repo), no sabe volverlo a HEAD: Descartar falla con error y
// no toca nada, ni el indice ni el disco; el trabajo sin guardar sigue ahi.
func TestHallazgo_b1b1bc_CA346_ObjetoFaltanteNoBorraLoQueHEADTiene(t *testing.T) {
	root, wt := h3Tarea(t, map[string]string{"secreto.go": h3Secreto}, nil)
	h3SinObjeto(t, root, wt, "secreto.go")
	const trabajo = "package app\n\n// trabajo sin guardar que no se puede perder\n"
	cbEscribir(t, wt, "secreto.go", trabajo)
	h3Descartable(t, "b1b1bc", root, []string{h3Rel("secreto.go")})
	antes := h3Foto(t, wt)

	res, out, err := h3Descartar(t, root)
	if got, ok := cbLeer(t, wt, "secreto.go"); !ok || got != trabajo {
		t.Fatalf("hallazgo b1b1bc (CA-346): Descartar nunca borra un archivo que HEAD tiene: secreto.go quedo %q (existe: %v), tenia que seguir con el trabajo sin guardar (error: %v, removed %q)\n%s", got, ok, err, res.Removed, out)
	}
	if err == nil {
		t.Fatalf("hallazgo b1b1bc (CA-346): si git no puede leer lo que HEAD tiene de secreto.go, Descartar falla con error; devolvio exito: restored %q removed %q\n%s", res.Restored, res.Removed, out)
	}
	if despues := h3Foto(t, wt); despues != antes {
		t.Fatalf("hallazgo b1b1bc (CA-346): si Descartar falla no toca nada, ni el indice ni el disco (- antes, + despues):\n%s", h3Cambios(antes, despues))
	}
}

// Hallazgo 20261014T180752_b1b1bc, CA-346 (guarda): en un repo sano la misma
// tarea se descarta: secreto.go vuelve a como esta en HEAD.
func TestHallazgo_b1b1bc_CA346_GuardaRepoSanoRestaura(t *testing.T) {
	root, wt := h3Tarea(t, map[string]string{"secreto.go": h3Secreto}, nil)
	cbEscribir(t, wt, "secreto.go", "package app\n\n// trabajo sin guardar\n")
	res, out, err := h3Descartar(t, root)
	if err != nil {
		t.Fatalf("hallazgo b1b1bc (CA-346): guarda: en un repo sano Descartar funciona: %v\n%s", err, out)
	}
	if !cbMismos(res.Restored, []string{h3Rel("secreto.go")}) || len(res.Removed) != 0 {
		t.Fatalf("hallazgo b1b1bc (CA-346): guarda: secreto.go, que HEAD tiene, se restaura y no se borra: restored %q removed %q", res.Restored, res.Removed)
	}
	if got, ok := cbLeer(t, wt, "secreto.go"); !ok || got != h3Secreto {
		t.Fatalf("hallazgo b1b1bc (CA-346): guarda: secreto.go vuelve a como esta en HEAD: %q (existe: %v)", got, ok)
	}
	h3Evidencia(t, "b1b1bc", wt)
}

// Hallazgo 20261014T180752_b1b1bc, CA-346: un submodulo con commits nuevos
// es una ruta que HEAD tiene (su entrada 160000), aunque el commit al que
// apunta nunca esta en el repo de arriba. Descartar nunca lo borra: haga lo
// que haga (volverlo a HEAD o negarse), la entrada del submodulo sigue en el
// indice tal cual, su directorio y sus archivos siguen en el disco y no se
// informa como removed.
func TestHallazgo_b1b1bc_CA346_SubmoduloModificadoNoSeBorra(t *testing.T) {
	up, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	cbGit(t, up, "init", "-q", "-b", "main")
	cbGit(t, up, "config", "user.email", "test@hoom.dev")
	cbGit(t, up, "config", "user.name", "hoom test")
	cbEscribir(t, up, "lib.txt", "v1\n")
	cbGit(t, up, "add", "-A")
	cbGit(t, up, "commit", "-q", "-m", "c1")

	root, wt := h3Tarea(t, nil, func(root string) {
		cbGit(t, root, "-c", "protocol.file.allow=always", "submodule", "add", "-q", up, "sub")
	})
	cbGit(t, wt, "-c", "protocol.file.allow=always", "submodule", "update", "--init", "-q")
	sub := filepath.Join(wt, "sub")
	cbEscribir(t, sub, "lib.txt", "v2\n")
	cbGit(t, sub, "-c", "user.email=test@hoom.dev", "-c", "user.name=hoom test", "commit", "-q", "-am", "c2")

	gitlink := cbGit(t, wt, "ls-files", "--stage", "--", "sub")
	if !strings.HasPrefix(gitlink, "160000 ") {
		t.Fatalf("CA-346: fixture: sub es un submodulo en el indice: %q", gitlink)
	}
	cmd := exec.Command("git", "cat-file", "-e", "HEAD:sub")
	cmd.Dir = wt
	if cmd.Run() == nil {
		t.Fatal("CA-346: fixture: el commit del submodulo no esta en el repo de arriba")
	}
	h3Descartable(t, "b1b1bc", root, []string{h3Rel("sub")})

	res, out, err := h3Descartar(t, root)
	if got := cbGit(t, wt, "ls-files", "--stage", "--", "sub"); got != gitlink {
		t.Fatalf("hallazgo b1b1bc (CA-346): Descartar nunca borra lo que HEAD tiene: la entrada del submodulo sub tiene que seguir en el indice tal cual (%q), quedo %q (error: %v)\n%s", gitlink, got, err, out)
	}
	if _, ok := cbLeer(t, sub, "lib.txt"); !ok {
		t.Fatalf("hallazgo b1b1bc (CA-346): Descartar nunca borra lo que HEAD tiene: el submodulo sub sigue en el disco (error: %v)\n%s", err, out)
	}
	for _, p := range res.Removed {
		if p == h3Rel("sub") {
			t.Fatalf("hallazgo b1b1bc (CA-346): el submodulo sub esta en HEAD y no se informa como removed: %q", res.Removed)
		}
	}
	h3Evidencia(t, "b1b1bc", wt)
}

// h3Vivo arranca OTRO proceso vivo (no este test) que hace de dueno del run.
func h3Vivo(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cmd.Process.Kill()
		cmd.Wait()
	})
	return cmd.Process.Pid
}

// h3RunEnCurso deja en <base>/.hoom/runs/ lo que deja un run en curso sobre
// dir: el sidecar (con el pid de su dueno vivo) y una narracion cuyo ultimo
// evento no es terminal. task vacio = un run lanzado desde el propio arbol,
// que para el es la raiz.
func h3RunEnCurso(t *testing.T, base, dir, id, task string, pid int) {
	t.Helper()
	meta := map[string]any{
		"id": id, "provider": "claude", "role": "writer", "dir": dir,
		"created_at": time.Now().UTC(), "status": "running", "exit_code": -1, "pid": pid,
	}
	if task != "" {
		meta["task"] = task
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	cbEscribir(t, base, ".hoom/runs/"+id+".meta.json", string(raw)+"\n")
	ev, err := json.Marshal(map[string]any{"ts": time.Now().UTC(), "kind": "start", "detail": "run " + id + ": claude en " + dir})
	if err != nil {
		t.Fatal(err)
	}
	cbEscribir(t, base, ".hoom/runs/"+id+".jsonl", string(ev)+"\n")
}

// h3SeNiega exige que Discard y `hoom task discard --yes` se nieguen
// nombrando el run id y sin tocar nada del worktree.
func h3SeNiega(t *testing.T, donde, root, wt, id string) {
	t.Helper()
	antes := h3Foto(t, wt)
	res, err := Discard(root, cbSlug, nil)
	if err == nil || !strings.Contains(err.Error(), id) {
		t.Fatalf("hallazgo f237b9 (CA-346): con un run activo en el arbol (registro en %s) Discard se niega nombrando el run %s: error %v, restored %q removed %q", donde, id, err, res.Restored, res.Removed)
	}
	if despues := h3Foto(t, wt); despues != antes {
		t.Fatalf("hallazgo f237b9 (CA-346): con un run activo (registro en %s) Discard no toca nada (- antes, + despues):\n%s", donde, h3Cambios(antes, despues))
	}
	_, out, err := h3Descartar(t, root)
	if err == nil {
		t.Fatalf("hallazgo f237b9 (CA-346): con un run activo (registro en %s) hoom task discard --yes tambien se niega:\n%s", donde, out)
	}
	if despues := h3Foto(t, wt); despues != antes {
		t.Fatalf("hallazgo f237b9 (CA-346): con un run activo (registro en %s) hoom task discard --yes no toca nada (- antes, + despues):\n%s", donde, h3Cambios(antes, despues))
	}
}

// h3TareaConCambios es h3Tarea con cosas descartables fuera de .hoom/: un
// archivo rastreado modificado y uno nuevo.
func h3TareaConCambios(t *testing.T) (root, wt string) {
	t.Helper()
	root, wt = h3Tarea(t, nil, nil)
	cbEscribir(t, wt, "app.go", "package app\n\nfunc Roto() {}\n")
	cbEscribir(t, wt, "nuevo.go", "package app\n")
	h3Descartable(t, "f237b9", root, []string{h3Rel("app.go"), h3Rel("nuevo.go")})
	return root, wt
}

// Hallazgo 20261014T180436_f237b9, CA-346: "con un run activo en el arbol se
// niega" vale para un run lanzado desde ADENTRO del espacio de trabajo de la
// tarea: su registro vive en <worktree>/.hoom/runs/, no en la raiz. Con o sin
// task en el sidecar, Discard y la CLI con --yes se niegan y no tocan nada.
func TestHallazgo_f237b9_CA346_RunActivoDesdeElWorktreeSeNiega(t *testing.T) {
	for _, task := range []string{"", cbSlug} {
		nombre := "sin-task"
		if task != "" {
			nombre = "con-task"
		}
		t.Run(nombre, func(t *testing.T) {
			root, wt := h3TareaConCambios(t)
			const id = "20261014T180436_f237b9"
			h3RunEnCurso(t, wt, wt, id, task, h3Vivo(t))
			h3SeNiega(t, "<worktree>/.hoom/runs", root, wt, id)
		})
	}
}

// Hallazgo 20261014T180436_f237b9, CA-346 (guarda): el mismo run en curso
// con su registro en la raiz (.hoom/runs/ del proyecto) ya se negaba.
func TestHallazgo_f237b9_CA346_GuardaRunActivoEnLaRaiz(t *testing.T) {
	root, wt := h3TareaConCambios(t)
	const id = "20261014T180436_f237b0"
	h3RunEnCurso(t, root, wt, id, cbSlug, h3Vivo(t))
	h3SeNiega(t, "la raiz", root, wt, id)
}

// Hallazgo 20261014T180752_646e6f, CA-346: si git no puede sacar del indice
// un archivo nuevo (el indice del worktree esta bloqueado por un index.lock
// en su gitdir), Descartar devuelve error y NO borra el archivo del disco:
// el contenido no se pierde y, sacado el bloqueo, sigue en el indice.
func TestHallazgo_646e6f_CA346_IndiceBloqueadoNoBorraElArchivoNuevo(t *testing.T) {
	root, wt := h3Tarea(t, nil, nil)
	const contenido = "package app\n\n// nuevo en el indice: no se puede perder\n"
	cbEscribir(t, wt, "indexado.go", contenido)
	cbGit(t, wt, "add", "--", "indexado.go")
	h3Descartable(t, "646e6f", root, []string{h3Rel("indexado.go")})

	lock := filepath.Join(cbGit(t, wt, "rev-parse", "--absolute-git-dir"), "index.lock")
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Remove(lock) })
	antes := h3Foto(t, wt)

	res, out, err := h3Descartar(t, root)
	if got, ok := cbLeer(t, wt, "indexado.go"); !ok || got != contenido {
		t.Fatalf("hallazgo 646e6f (CA-346): si git no puede sacarlo del indice, Descartar NO borra indexado.go del disco: quedo %q (existe: %v) (error: %v, removed %q)\n%s", got, ok, err, res.Removed, out)
	}
	if err == nil {
		t.Fatalf("hallazgo 646e6f (CA-346): si git no puede sacar indexado.go del indice, Descartar devuelve error; devolvio exito: restored %q removed %q\n%s", res.Restored, res.Removed, out)
	}
	if despues := h3Foto(t, wt); despues != antes {
		t.Fatalf("hallazgo 646e6f (CA-346): si Descartar falla no toca nada (- antes, + despues):\n%s", h3Cambios(antes, despues))
	}
	if err := os.Remove(lock); err != nil {
		t.Fatalf("hallazgo 646e6f (CA-346): el index.lock es del test y Descartar no lo toca: %v", err)
	}
	if idx := cbGit(t, wt, "ls-files", "--stage", "--", "indexado.go"); idx == "" {
		t.Fatal("hallazgo 646e6f (CA-346): sacado el bloqueo, indexado.go sigue en el indice")
	}
}
