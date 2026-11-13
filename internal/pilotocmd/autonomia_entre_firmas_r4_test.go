// Tests de regresion de la ronda 4 de autonomia-entre-firmas, como
// consumidor externo (proyectos git reales, CLIs de IA falsos en un PATH
// minimo, nunca uno real):
//
//   - hallazgo 20261112T203544_64fc3f (CA-644, aclaracion 23): el guardado
//     del piloto corre los hooks de git, y un hook que rechaza hace fallar el
//     guardado.
//
// Tambien apr4Hook, el helper con el que los tests de 8c4407, 272465 y
// 54fa4e instalan sus hooks.
package pilotocmd_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// apr4Hook instala el hook nombre (pre-commit, commit-msg...) en el repo de
// la tarea, donde git lo busca desde su worktree.
func apr4Hook(t *testing.T, wt, nombre, script string) {
	t.Helper()
	ruta := apGit(t, wt, "rev-parse", "--git-path", "hooks/"+nombre)
	if !filepath.IsAbs(ruta) {
		ruta = filepath.Join(wt, ruta)
	}
	if err := os.MkdirAll(filepath.Dir(ruta), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ruta, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

// Hallazgo 20261112T203544_64fc3f (CA-644, aclaracion 23): con un hook del
// repo de la tarea que rechaza el commit (sale 1 si lo que se va a commitear
// contiene SECRETO, con su motivo en stderr) y cambios sin guardar que lo
// contienen, `hoom piloto` no crea el commit de guardado: sale con exit 1 y
// `piloto automatico: se detiene: no pudo guardar el trabajo de la tarjeta:
// <error>`, el cambio sigue sin commitear en el worktree y el hook corrio.
// Dos variantes: el que rechaza es el pre-commit, o el commit-msg.
func TestHallazgo64fc3f_CA644_UnHookQueRechazaHaceFallarElGuardado(t *testing.T) {
	for _, nombre := range []string{"pre-commit", "commit-msg"} {
		ca := "64fc3f/CA-644 (" + nombre + ")"
		f := apPATH(t)
		f.cli(t, "claude")
		root := apProyecto(t, "")
		const slug = "con-secreto"
		wt := apTuAprobacion(t, root, slug, apConAuto, true)
		apEscribir(t, wt, "app.go", "package app\n\n// SECRETO: la clave de la api\n")
		head, rama := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug)

		corrio := filepath.Join(t.TempDir(), "corrio")
		apr4Hook(t, wt, nombre, "#!/bin/sh\n: > '"+corrio+"'\n"+
			"if git diff --cached | grep -q SECRETO; then\n"+
			"  echo 'detector de secretos: lo que vas a commitear tiene un SECRETO' >&2\n  exit 1\nfi\nexit 0\n")

		s := apRun(t, ca, root, slug)
		ls := apSoloLineasDelPiloto(t, ca, s)
		pre := apDetiene + "no pudo guardar el trabajo de la tarjeta: "
		if s.code != 1 || !strings.HasPrefix(apUltima(ls), pre) || strings.TrimSpace(strings.TrimPrefix(apUltima(ls), pre)) == "" {
			t.Fatalf("%s: con el hook que rechaza se detiene con `%s<error>` y exit 1:\n%s", ca, pre, s)
		}
		for _, l := range ls {
			if apGuarda.MatchString(l) {
				t.Fatalf("%s: no dice que guardo: %q", ca, l)
			}
		}
		if !apr1Existe(corrio) {
			t.Fatalf("%s: el guardado del piloto corre el hook %s (aclaracion 23):\n%s", ca, nombre, s)
		}
		if h, r := apHead(t, wt), apGit(t, wt, "rev-parse", "hoom/"+slug); h != head || r != rama {
			t.Fatalf("%s: no crea el commit de guardado: HEAD %s -> %s, hoom/%s %s -> %s", ca, head, h, slug, rama, r)
		}
		if st := apGit(t, wt, "status", "--porcelain"); !strings.Contains(st, "app.go") {
			t.Fatalf("%s: el cambio sigue sin commitear en el worktree: %q", ca, st)
		}
		if raw, _ := os.ReadFile(filepath.Join(wt, "app.go")); !strings.Contains(string(raw), "SECRETO") {
			t.Fatalf("%s: el cambio sigue en el worktree: %q", ca, raw)
		}
		f.ninguna(t, ca, "un piloto que no pudo guardar")
	}
}
