// Tests de la fixture misma (hallazgos 20261006T204527_30abfc y
// 20261007T142722_de562b): si lo que se anexa no fuera lo que git lee, si
// ExigirMantenimientoApagado diera por apagado lo que git lee prendido, o si
// EntornoSolo dejara pasar la configuracion del que corre los tests, los
// tests de agentcmd y de reviewcmd que se apoyan en este paquete pasarian sin
// medir nada. Es todo fixture: no hay un criterio de un spec detras.
package gittest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// ------------------------------------------------------------------ helpers

// sinSoltar es EntornoSolo con maintenance.autoDetach=false: el mantenimiento
// que un commit lanza termina antes de que el commit vuelva. Suelto (git 2.47
// en adelante, con --detach) seguiria creando y borrando
// .git/objects/maintenance.lock mientras el test termina y borra su
// directorio temporal: lo mismo que 30abfc, contra estos tests. Un git
// anterior no conoce la clave, y ya lo corre asi. extra va al final.
func sinSoltar(t *testing.T, extra ...string) []string {
	t.Helper()
	return EntornoSolo(t, append([]string{"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=maintenance.autoDetach", "GIT_CONFIG_VALUE_0=false"}, extra...)...)
}

// leeApagado dice si git, con el .git/config de root y sin mas configuracion
// que esa, lee maintenance.auto en false y gc.auto en 0.
func leeApagado(root string, env []string) bool {
	m, errM := salida(root, env, "config", "--bool", "--get", "maintenance.auto")
	g, errG := salida(root, env, "config", "--int", "--get", "gc.auto")
	return errM == nil && errG == nil && strings.TrimSpace(m) == "false" && strings.TrimSpace(g) == "0"
}

// tbQueFalla es el testing.TB que recibe una fixture de la que se espera un
// Fatalf: lo guarda y corta la goroutine, como hace el de verdad.
type tbQueFalla struct {
	testing.TB
	fatal string
}

func (f *tbQueFalla) Helper() {}

func (f *tbQueFalla) Fatalf(format string, args ...any) {
	f.fatal = fmt.Sprintf(format, args...)
	runtime.Goexit()
}

// fatalDe corre fixture con un tbQueFalla y devuelve con que hizo Fatalf
// ("": no lo hizo).
func fatalDe(t *testing.T, fixture func(tb testing.TB)) string {
	t.Helper()
	falso := &tbQueFalla{TB: t}
	listo := make(chan struct{})
	go func() {
		defer close(listo)
		fixture(falso)
	}()
	<-listo
	return falso.fatal
}

// -------------------------------------------------------------------- tests

// Hallazgo 30abfc: en un repo de `git init` a secas, con git sin mas
// configuracion que la suya, el commit lanza el mantenimiento automatico y
// ExigirMantenimientoApagado se niega; despues de ApagarMantenimiento el
// .git/config es el que dejo git con SinMantenimiento al final, el commit no
// lanza nada y ExigirMantenimientoApagado lo acepta. Un worktree comparte el
// .git/config de su repo y tampoco lanza nada; un clon NO lo hereda: lanza
// hasta que se lo apagan a el.
func TestHallazgo30abfc_ApagarMantenimientoApagaLoQueGitLanza(t *testing.T) {
	root := RepoCrudo(t, t.TempDir())
	cfg := filepath.Join(root, ".git", "config")
	base, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	solo := sinSoltar(t)
	if lanza := CommitConTraza(t, root, solo); len(lanza) == 0 {
		t.Fatalf("30abfc: control: en un repo de `git init` a secas el commit lanza el mantenimiento automatico " +
			"(git maintenance run --auto, o git gc --auto): este git no lanzo nada, y asi este test no puede ver si ApagarMantenimiento lo apaga")
	}
	if err := ExigirMantenimientoApagado(cfg); err == nil {
		t.Fatalf("30abfc: ExigirMantenimientoApagado se niega con un repo de `git init` a secas, y lo acepto")
	}

	ApagarMantenimiento(t, root)
	if raw, err := os.ReadFile(cfg); err != nil || string(raw) != string(base)+SinMantenimiento {
		t.Fatalf("30abfc: ApagarMantenimiento anexa SinMantenimiento a lo que dejo git init, y nada mas: %q %v", raw, err)
	}
	if !leeApagado(root, solo) {
		t.Fatalf("30abfc: despues de ApagarMantenimiento git lee maintenance.auto en false y gc.auto en 0")
	}
	if lanza := CommitConTraza(t, root, solo); len(lanza) != 0 {
		t.Fatalf("30abfc: despues de ApagarMantenimiento el commit no lanza nada; lanzo:\n%s", strings.Join(lanza, "\n"))
	}
	if err := ExigirMantenimientoApagado(cfg); err != nil {
		t.Fatalf("30abfc: ExigirMantenimientoApagado acepta lo que deja ApagarMantenimiento: %v", err)
	}

	// un worktree comparte el .git/config de su repo
	wt := filepath.Join(t.TempDir(), "wt")
	if out, err := salida(root, solo, "worktree", "add", "-q", "-b", "hoom/tarea", wt); err != nil {
		t.Fatalf("fixture: git worktree add %s: %v\n%s", wt, err, out)
	}
	if lanza := CommitConTraza(t, wt, solo); len(lanza) != 0 {
		t.Fatalf("30abfc: un commit en un worktree de un repo apagado no lanza nada; lanzo:\n%s", strings.Join(lanza, "\n"))
	}

	// un clon no hereda el .git/config de su origen
	clon := filepath.Join(t.TempDir(), "clon")
	if out, err := salida(root, solo, "clone", "-q", root, clon); err != nil {
		t.Fatalf("fixture: git clone %s: %v\n%s", root, err, out)
	}
	if lanza := CommitConTraza(t, clon, solo); len(lanza) == 0 {
		t.Fatalf("30abfc: control: un clon no hereda el apagado de su origen, y el commit en el clon no lanzo nada: " +
			"asi no se ve por que a un clon hay que apagarselo aparte")
	}
	ApagarMantenimiento(t, clon)
	if lanza := CommitConTraza(t, clon, solo); len(lanza) != 0 {
		t.Fatalf("30abfc: despues de ApagarMantenimiento el commit en el clon no lanza nada; lanzo:\n%s", strings.Join(lanza, "\n"))
	}
}

// Hallazgo 30abfc: ApagarMantenimiento no inventa un .git/config donde git no
// dejo uno (un directorio que no es un repo, o el dir equivocado): es un
// error del fixture, que nombra el directorio, y no deja nada escrito.
func TestHallazgo30abfc_ApagarMantenimientoFallaDondeNoHayRepo(t *testing.T) {
	for _, conDotGit := range []bool{false, true} {
		dir := t.TempDir()
		if conDotGit {
			if err := os.Mkdir(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		fatal := fatalDe(t, func(tb testing.TB) { ApagarMantenimiento(tb, dir) })
		if !strings.Contains(fatal, "fixture:") || !strings.Contains(fatal, dir) {
			t.Fatalf("30abfc: sin .git/config (con .git: %v) ApagarMantenimiento falla como fixture y nombra %s: %q", conDotGit, dir, fatal)
		}
		if _, err := os.Lstat(filepath.Join(dir, ".git", "config")); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("30abfc: sin .git/config (con .git: %v) ApagarMantenimiento no crea uno: %v", conDotGit, err)
		}
	}
}

// Hallazgo 30abfc: lo que el fixture anexa al .git/config es lo que git lee
// (git config --get: maintenance.auto false, gc.auto 0), y lo que
// ExigirMantenimientoApagado exige coincide con git en lo que importa: en
// cada forma de escribirlo acepta lo que tiene que aceptar, se niega con todo
// lo que git lee prendido, y NUNCA da por apagado lo que git lee prendido.
func TestHallazgo30abfc_ExigirMantenimientoApagadoExigeLoQueGitLee(t *testing.T) {
	root := RepoCrudo(t, t.TempDir())
	cfg := filepath.Join(root, ".git", "config")
	base, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	solo := EntornoSolo(t)
	for _, c := range ConfigsDeMantenimiento() {
		if err := os.WriteFile(cfg, append(append([]byte{}, base...), c.Anexo...), 0o644); err != nil {
			t.Fatal(err)
		}
		segunGit := leeApagado(root, solo)
		if segunGit != c.ApagadoParaGit {
			t.Fatalf("30abfc: %s: fixture: git lee el mantenimiento apagado=%v, y el caso dice %v:\n%s", c.Caso, segunGit, c.ApagadoParaGit, c.Anexo)
		}
		err := ExigirMantenimientoApagado(cfg)
		if err == nil && !segunGit {
			t.Fatalf("30abfc: %s: ExigirMantenimientoApagado da por apagado un mantenimiento que git lee prendido:\n%s", c.Caso, c.Anexo)
		}
		if (err == nil) != c.Aceptado {
			t.Fatalf("30abfc: %s: ExigirMantenimientoApagado lo acepta=%v, y tenia que ser %v (%v):\n%s", c.Caso, err == nil, c.Aceptado, err, c.Anexo)
		}
		if err != nil && (!strings.Contains(err.Error(), "maintenance.auto") || !strings.Contains(err.Error(), "gc.auto")) {
			t.Fatalf("30abfc: %s: la negativa dice que hay que apagar (maintenance.auto, gc.auto): %v", c.Caso, err)
		}
	}

	// sin config que leer no hay nada apagado, y un include no se sigue
	if err := ExigirMantenimientoApagado(filepath.Join(root, ".git", "no-existe")); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("30abfc: sin .git/config ExigirMantenimientoApagado se niega con el error de leerlo: %v", err)
	}
	aparte := filepath.Join(t.TempDir(), "aparte")
	if err := os.WriteFile(aparte, []byte(SinMantenimiento), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, append(append([]byte{}, base...), "[include]\n\tpath = "+aparte+"\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ExigirMantenimientoApagado(cfg); err == nil {
		t.Fatalf("30abfc: ExigirMantenimientoApagado no sigue un include: exige el apagado escrito en el .git/config del repo")
	}
}

// Hallazgo 30abfc: EntornoSolo es git sin mas configuracion que la suya. Con
// el que corre los tests trayendo el mantenimiento apagado y otra identidad
// —en su config global y en su entorno—, git con EntornoSolo no lee nada de
// eso: el commit en un repo de `git init` a secas lanza el mantenimiento
// igual (si no, el control de los tests que lo usan fallaria en esa maquina,
// o no distinguiria nada), y lo firma la identidad del fixture. Lo de extra
// va al final, y gana.
func TestHallazgo30abfc_EntornoSoloNoLeeLaConfiguracionDelQueCorreLosTests(t *testing.T) {
	casa := t.TempDir()
	delAmbiente := SinMantenimiento + "[user]\n\tname = del ambiente\n\temail = ambiente@hoom.dev\n"
	global := filepath.Join(casa, ".gitconfig")
	xdg := filepath.Join(casa, "xdg", "git", "config")
	if err := os.MkdirAll(filepath.Dir(xdg), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{global, xdg} {
		if err := os.WriteFile(p, []byte(delAmbiente), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", casa)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(casa, "xdg"))
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_COUNT", "2")
	t.Setenv("GIT_CONFIG_KEY_0", "maintenance.auto")
	t.Setenv("GIT_CONFIG_VALUE_0", "false")
	t.Setenv("GIT_CONFIG_KEY_1", "gc.auto")
	t.Setenv("GIT_CONFIG_VALUE_1", "0")
	for _, k := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(k, "del ambiente")
	}
	for _, k := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(k, "ambiente@hoom.dev")
	}

	root := RepoCrudo(t, t.TempDir())
	if !leeApagado(root, nil) {
		t.Fatalf("30abfc: control: con el entorno del proceso git lee el mantenimiento apagado del ambiente; " +
			"no lo leyo, y asi este test no puede ver si EntornoSolo lo deja afuera")
	}
	// sin extra: lo que el ambiente trae en GIT_CONFIG_COUNT no lo tapa nadie
	limpio := EntornoSolo(t)
	for _, clave := range []string{"maintenance.auto", "gc.auto", "user.name", "user.email"} {
		if out, err := salida(root, limpio, "config", "--get", clave); err == nil || strings.TrimSpace(out) != "" {
			t.Fatalf("30abfc: con EntornoSolo git no lee %s de ningun lado; leyo %q (%v)", clave, out, err)
		}
	}
	solo := sinSoltar(t)
	if lanza := CommitConTraza(t, root, solo); len(lanza) == 0 {
		t.Fatalf("30abfc: con EntornoSolo el commit en un repo de `git init` a secas lanza el mantenimiento automatico, " +
			"lo traiga apagado o no el que corre los tests; no lanzo nada")
	}
	firma := func() string {
		t.Helper()
		out, err := salida(root, solo, "log", "-1", "--format=%an <%ae> / %cn <%ce>")
		if err != nil {
			t.Fatalf("fixture: git log en %s: %v\n%s", root, err, out)
		}
		return strings.TrimSpace(out)
	}
	if f := firma(); f != "hoom test <test@hoom.dev> / hoom test <test@hoom.dev>" {
		t.Fatalf("30abfc: con EntornoSolo el commit lo firma la identidad del fixture, no la del ambiente: %q", f)
	}

	conExtra := sinSoltar(t, "GIT_AUTHOR_NAME=la de extra")
	if ultimo := conExtra[len(conExtra)-1]; ultimo != "GIT_AUTHOR_NAME=la de extra" {
		t.Fatalf("30abfc: lo de extra va al final del entorno: termina en %q", ultimo)
	}
	CommitConTraza(t, root, conExtra)
	if f := firma(); f != "la de extra <test@hoom.dev> / hoom test <test@hoom.dev>" {
		t.Fatalf("30abfc: lo de extra le gana a lo que EntornoSolo pone: %q", f)
	}
}
