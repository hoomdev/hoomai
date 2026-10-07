// Hallazgo 20261006T204527_30abfc: el mantenimiento automatico de git corre
// contra los fixtures que copian un repo. Despues de cada commit (y de merge,
// rebase, fetch, am) git lanza por su cuenta `git maintenance run --auto`
// —desde git 2.47 suelto en segundo plano, con --detach; antes de git 2.29,
// `git gc --auto`—, que crea y borra .git/objects/maintenance.lock. El que
// copia el repo recien commiteado lista el lock y, cuando lo va a mirar, ya
// no esta: en el CI, una de cada ocho corridas.
//
// Aca vive lo que los repos de prueba de este paquete llevan para que eso no
// pase (el mantenimiento apagado desde que nacen), lo que la copia les exige,
// y los tests de las dos cosas. Es todo fixture: no hay un criterio de un
// spec detras.
package agentcmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitSinMantenimiento es lo que el .git/config de un repo de prueba lleva
// para que git no lance nada por su cuenta. maintenance.auto=false apaga el
// lanzamiento (git 2.29 en adelante); gc.auto=0 es lo mismo para un git
// anterior, que lanza `git gc --auto`, y deja sin nada que hacer al gc que
// alguien lance a mano con --auto. Los worktrees comparten el .git/config de
// su repo, asi que lo heredan; un `git clone` NO: al clon hay que apagarselo
// (gitApagarMantenimiento).
const gitSinMantenimiento = "[maintenance]\n\tauto = false\n[gc]\n\tauto = 0\n"

// gitApagarMantenimiento anexa gitSinMantenimiento al .git/config de root.
// Es un archivo, no un proceso: no cuesta un git mas por repo. git(t, dir,
// "init", ...) lo llama solo.
func gitApagarMantenimiento(t *testing.T, root string) {
	t.Helper()
	cfg := filepath.Join(root, ".git", "config")
	// sin O_CREATE: si git init no dejo su config, esto no es un repo
	f, err := os.OpenFile(cfg, os.O_WRONLY|os.O_APPEND, 0)
	if err == nil {
		_, err = f.WriteString(gitSinMantenimiento)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		t.Fatalf("fixture: apagar el mantenimiento automatico de git en %s: %v", root, err)
	}
}

// gitExigirMantenimientoApagado lee cfg (el .git/config de un repo) y
// devuelve un error si no deja apagado el mantenimiento automatico de git:
// maintenance.auto en false Y gc.auto en 0, y de cada clave manda la ultima
// aparicion, como en git. Lee el archivo y nada mas: ningun proceso.
//
// Es a proposito mas estricta que git: exige que el fixture lo haya dejado
// escrito en claro en el repo. Un valor con comillas o con un comentario al
// lado, una continuacion de linea o un include no los interpreta: se niega.
// Lo que nunca hace es dar por apagado lo que git lee prendido.
func gitExigirMantenimientoApagado(cfg string) error {
	const pide = "el repo que se copia tiene que traer apagado el mantenimiento automatico de git " +
		"(maintenance.auto = false y gc.auto = 0 en su .git/config): git(t, dir, \"init\", ...) lo deja asi, " +
		"y a un clon se lo apaga gitApagarMantenimiento"
	raw, err := os.ReadFile(cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", pide, err)
	}
	valor := map[string]string{} // seccion.clave -> el ultimo valor, tal cual
	seccion := ""
	for i, l := range strings.Split(string(raw), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || l[0] == '#' || l[0] == ';' {
			continue
		}
		if strings.HasSuffix(l, `\`) {
			return fmt.Errorf("%s: %s:%d: una linea que sigue en la siguiente (\\) no se interpreta", pide, cfg, i+1)
		}
		if l[0] == '[' {
			fin := strings.IndexByte(l, ']')
			if fin < 0 {
				return fmt.Errorf("%s: %s:%d: seccion sin cerrar", pide, cfg, i+1)
			}
			seccion = strings.ToLower(l[1:fin])
			if seccion == "include" || strings.HasPrefix(seccion, "includeif") {
				return fmt.Errorf("%s: %s:%d: un include no se sigue", pide, cfg, i+1)
			}
			if l = strings.TrimSpace(l[fin+1:]); l == "" {
				continue
			}
		}
		clave, v, hayValor := strings.Cut(l, "=")
		if !hayValor {
			v = "true" // una clave sola es true para git
		}
		valor[seccion+"."+strings.ToLower(strings.TrimSpace(clave))] = strings.TrimSpace(v)
	}
	mostrar := func(k string) string {
		if v, ok := valor[k]; ok {
			return fmt.Sprintf("%s = %q", k, v)
		}
		return k + " sin definir"
	}
	m, hayM := valor["maintenance.auto"]
	g, hayG := valor["gc.auto"]
	apagado := false
	for _, no := range []string{"false", "no", "off", "0"} {
		apagado = apagado || (hayM && strings.EqualFold(m, no))
	}
	if !apagado || !hayG || g != "0" {
		return fmt.Errorf("%s: %s trae %s y %s", pide, cfg, mostrar("maintenance.auto"), mostrar("gc.auto"))
	}
	return nil
}

// ------------------------------------------------------------------ helpers

// gitEntornoSolo es el entorno del proceso sin nada que le cambie a git su
// configuracion (ni GIT_*, ni el config global, ni el del sistema), con una
// identidad para commitear y con extra: lo que git hace por defecto.
func gitEntornoSolo(t *testing.T, extra ...string) []string {
	t.Helper()
	casa := t.TempDir()
	var env []string
	for _, kv := range os.Environ() {
		k, _, _ := strings.Cut(kv, "=")
		if strings.HasPrefix(k, "GIT_") || k == "HOME" || k == "XDG_CONFIG_HOME" {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "HOME="+casa, "XDG_CONFIG_HOME="+filepath.Join(casa, "xdg"),
		"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_NAME=hoom test", "GIT_AUTHOR_EMAIL=test@hoom.dev",
		"GIT_COMMITTER_NAME=hoom test", "GIT_COMMITTER_EMAIL=test@hoom.dev")
	return append(env, extra...)
}

// gitSalida corre git en dir con env (nil: el del proceso) y devuelve su
// salida y su error, sin exigir nada. No pasa por git(): un `init` de aca
// NO apaga el mantenimiento.
func gitSalida(dir string, env []string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// gitCommitConTraza hace un commit vacio en dir con GIT_TRACE=1 y devuelve,
// de la traza, las lineas en las que git lanza mantenimiento por su cuenta
// (`git maintenance run --auto`, o `git gc --auto` en un git viejo).
func gitCommitConTraza(t *testing.T, dir string, env []string) []string {
	t.Helper()
	if env == nil {
		env = os.Environ()
	}
	traza, err := gitSalida(dir, append(append([]string{}, env...), "GIT_TRACE=1"),
		"commit", "-q", "--allow-empty", "-m", "otro")
	if err != nil {
		t.Fatalf("fixture: git commit con GIT_TRACE=1 en %s: %v\n%s", dir, err, traza)
	}
	if !strings.Contains(traza, "trace:") {
		t.Fatalf("fixture: GIT_TRACE=1 no dejo traza del commit en %s: sin traza no se ve lo que git lanza\n%s", dir, traza)
	}
	var lanza []string
	for _, l := range strings.Split(traza, "\n") {
		if strings.Contains(l, "maintenance") || strings.Contains(l, "gc --auto") {
			lanza = append(lanza, strings.TrimSpace(l))
		}
	}
	return lanza
}

// gitRepoCrudo es un repo recien creado por `git init` a secas, sin pasar por
// git(): con el mantenimiento automatico como git lo trae, prendido.
func gitRepoCrudo(t *testing.T, dir string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := gitSalida(dir, gitEntornoSolo(t), "init", "-q", "-b", "main"); err != nil {
		t.Fatalf("fixture: git init en %s: %v\n%s", dir, err, out)
	}
	return dir
}

// gitAnexar agrega texto al final de p.
func gitAnexar(t *testing.T, p, texto string) {
	t.Helper()
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(texto); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// -------------------------------------------------------------------- tests

// Hallazgo 30abfc: un repo armado por cada constructor del paquete que hace
// `git init` (los demas —repo, repoCiego, h3Repo, qcRepoConHallazgo, las
// plantillas— parten de uno de estos) no lanza mantenimiento despues de un
// commit (GIT_TRACE=1 lo mostraria), y su .git/config pasa lo que la copia
// exige. Un worktree del repo, tampoco: comparte su .git/config. El control
// es un repo de `git init` a secas, con git sin mas configuracion que la
// suya: ahi el commit SI lo lanza (si no, este test no distinguiria nada), y
// despues de gitApagarMantenimiento deja de lanzarlo.
func TestHallazgo30abfc_LosReposDePruebaNoLanzanMantenimientoDeGit(t *testing.T) {
	control := gitRepoCrudo(t, t.TempDir())
	solo := gitEntornoSolo(t)
	if lanza := gitCommitConTraza(t, control, solo); len(lanza) == 0 {
		t.Fatalf("30abfc: control: en un repo de `git init` a secas el commit lanza el mantenimiento automatico " +
			"(git maintenance run --auto, o git gc --auto): este git no lanzo nada, y asi este test no puede ver si los fixtures lo apagan")
	}
	gitApagarMantenimiento(t, control)
	if lanza := gitCommitConTraza(t, control, solo); len(lanza) != 0 {
		t.Fatalf("30abfc: despues de gitApagarMantenimiento el commit no lanza nada; lanzo:\n%s", strings.Join(lanza, "\n"))
	}

	constructores := []struct {
		nombre string
		armar  func(t *testing.T) string
		// limpia: el constructor limpia el entorno del proceso
		// (qcLimpiarEntorno), y por eso no corre en paralelo
		limpia bool
	}{
		{"repoGate", func(t *testing.T) string { return repoGate(t, "true") }, false},
		{"piRepo", piRepo, false},
		{"raRepoConReview", raRepoConReview, false},
		{"edRepo", func(t *testing.T) string { return edRepo(t, true, true) }, true},
		{"qcr1RepoBloqueante", func(t *testing.T) string { root, _ := qcr1RepoBloqueante(t); return root }, true},
		{"qcr2RepoCiegoBloqueante", func(t *testing.T) string { root, _ := qcr2RepoCiegoBloqueante(t); return root }, true},
	}
	for _, c := range constructores {
		t.Run(c.nombre, func(t *testing.T) {
			if !c.limpia {
				t.Parallel()
			}
			root := c.armar(t)
			if err := gitExigirMantenimientoApagado(filepath.Join(root, ".git", "config")); err != nil {
				t.Fatalf("30abfc: %s: %v", c.nombre, err)
			}
			if lanza := gitCommitConTraza(t, root, nil); len(lanza) != 0 {
				t.Fatalf("30abfc: %s: un commit en el repo no lanza mantenimiento; lanzo:\n%s", c.nombre, strings.Join(lanza, "\n"))
			}
			if c.nombre != "repoGate" {
				return
			}
			// un worktree (asi arma hoom las tareas) comparte el config del repo
			wt := filepath.Join(t.TempDir(), "wt")
			git(t, root, "worktree", "add", "-q", "-b", "hoom/tarea", wt)
			if lanza := gitCommitConTraza(t, wt, nil); len(lanza) != 0 {
				t.Fatalf("30abfc: %s: un commit en un worktree del repo no lanza mantenimiento; lanzo:\n%s", c.nombre, strings.Join(lanza, "\n"))
			}
		})
	}
}

// gitConfigDeMantenimiento es una forma de escribir (o no) el apagado en un
// .git/config. apagadoParaGit es lo que git tiene que leer; laCopiaLoAcepta,
// lo que gitExigirMantenimientoApagado tiene que decir.
type gitConfigDeMantenimiento struct {
	caso            string
	anexo           string
	apagadoParaGit  bool
	laCopiaLoAcepta bool
}

func gitConfigsDeMantenimiento() []gitConfigDeMantenimiento {
	return []gitConfigDeMantenimiento{
		{"lo que anexa el fixture", gitSinMantenimiento, true, true},
		{"anexado dos veces", gitSinMantenimiento + gitSinMantenimiento, true, true},
		{"sin espacios y en una linea con su seccion", "[maintenance] auto=false\n[gc] auto=0\n", true, true},
		{"mayusculas y no/off", "[MAINTENANCE]\n\tAUTO = No\n[Gc]\n\tAuto = 0\n", true, true},
		{"maintenance.auto en 0", "[maintenance]\n\tauto = 0\n[gc]\n\tauto = 0\n", true, true},
		{"fin de linea CRLF", "[maintenance]\r\n\tauto = false\r\n[gc]\r\n\tauto = 0\r\n", true, true},
		{"otras claves alrededor", "[maintenance]\n\tstrategy = none\n\tauto = false\n[gc]\n\tautoDetach = true\n\tauto = 0\n\tpruneExpire = now\n", true, true},
		// lo que git lee apagado y la copia, mas estricta, no interpreta
		{"con un comentario al lado", "[maintenance]\n\tauto = false # apagado\n[gc]\n\tauto = 0 ; apagado\n", true, false},
		{"con comillas", "[maintenance]\n\tauto = \"false\"\n[gc]\n\tauto = \"0\"\n", true, false},
		{"valor vacio (false para git)", "[maintenance]\n\tauto =\n[gc]\n\tauto = 0\n", true, false},
		// lo que git lee prendido: la copia se niega siempre
		{"nada", "", false, false},
		{"solo maintenance.auto", "[maintenance]\n\tauto = false\n", false, false},
		{"solo gc.auto", "[gc]\n\tauto = 0\n", false, false},
		{"despues alguien lo prende", gitSinMantenimiento + "[maintenance]\n\tauto = true\n", false, false},
		{"despues alguien sube gc.auto", gitSinMantenimiento + "[gc]\n\tauto = 6700\n", false, false},
		{"prendido en mayusculas, despues", gitSinMantenimiento + "[Maintenance]\n\tAUTO = TRUE\n", false, false},
		{"la clave sola (true para git)", "[maintenance]\n\tauto\n[gc]\n\tauto = 0\n", false, false},
		{"comentado", "#[maintenance]\n#\tauto = false\n;[gc]\n;\tauto = 0\n", false, false},
		{"las claves comentadas", "[maintenance]\n\t# auto = false\n[gc]\n\t; auto = 0\n", false, false},
		{"en una subseccion", "[maintenance \"x\"]\n\tauto = false\n[gc \"x\"]\n\tauto = 0\n", false, false},
		{"en otra seccion", "[core]\n\tauto = false\n[gcx]\n\tauto = 0\n", false, false},
		{"otra clave que empieza igual", "[maintenance]\n\tautoDetach = false\n[gc]\n\tautoPackLimit = 0\n", false, false},
		{"la linea de arriba sigue en esta", "[maintenance]\n\tstrategy = none\\\n\tauto = false\n[gc]\n\tauto = 0\n", false, false},
		{"gc.auto en 1", "[maintenance]\n\tauto = false\n[gc]\n\tauto = 1\n", false, false},
	}
}

// gitLeeApagado dice si git, con el .git/config de root y sin mas
// configuracion que esa, lee maintenance.auto en false y gc.auto en 0.
func gitLeeApagado(root string, env []string) bool {
	m, errM := gitSalida(root, env, "config", "--bool", "--get", "maintenance.auto")
	g, errG := gitSalida(root, env, "config", "--int", "--get", "gc.auto")
	return errM == nil && errG == nil && strings.TrimSpace(m) == "false" && strings.TrimSpace(g) == "0"
}

// Hallazgo 30abfc: lo que el fixture anexa al .git/config es lo que git lee
// (git config --get: maintenance.auto false, gc.auto 0), y lo que la copia
// exige coincide con git en lo que importa: en cada forma de escribirlo, la
// copia acepta lo que tiene que aceptar, se niega con todo lo que git lee
// prendido, y NUNCA da por apagado lo que git lee prendido.
func TestHallazgo30abfc_LaCopiaExigeLoQueGitLee(t *testing.T) {
	root := gitRepoCrudo(t, t.TempDir())
	cfg := filepath.Join(root, ".git", "config")
	base, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	solo := gitEntornoSolo(t)
	for _, c := range gitConfigsDeMantenimiento() {
		if err := os.WriteFile(cfg, append(append([]byte{}, base...), c.anexo...), 0o644); err != nil {
			t.Fatal(err)
		}
		segunGit := gitLeeApagado(root, solo)
		if segunGit != c.apagadoParaGit {
			t.Fatalf("30abfc: %s: fixture: git lee el mantenimiento apagado=%v, y el caso dice %v:\n%s", c.caso, segunGit, c.apagadoParaGit, c.anexo)
		}
		err := gitExigirMantenimientoApagado(cfg)
		if err == nil && !segunGit {
			t.Fatalf("30abfc: %s: la copia da por apagado un mantenimiento que git lee prendido:\n%s", c.caso, c.anexo)
		}
		if (err == nil) != c.laCopiaLoAcepta {
			t.Fatalf("30abfc: %s: la copia lo acepta=%v, y tenia que ser %v (%v):\n%s", c.caso, err == nil, c.laCopiaLoAcepta, err, c.anexo)
		}
		if err != nil && (!strings.Contains(err.Error(), "maintenance.auto") || !strings.Contains(err.Error(), "gc.auto")) {
			t.Fatalf("30abfc: %s: la negativa dice que hay que apagar (maintenance.auto, gc.auto): %v", c.caso, err)
		}
	}

	// sin config que leer no hay nada apagado, y un include no se sigue
	if err := gitExigirMantenimientoApagado(filepath.Join(root, ".git", "no-existe")); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("30abfc: sin .git/config la copia se niega con el error de leerlo: %v", err)
	}
	aparte := filepath.Join(t.TempDir(), "aparte")
	if err := os.WriteFile(aparte, []byte(gitSinMantenimiento), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfg, append(append([]byte{}, base...), "[include]\n\tpath = "+aparte+"\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := gitExigirMantenimientoApagado(cfg); err == nil {
		t.Fatalf("30abfc: la copia no sigue un include: exige el apagado escrito en el .git/config del repo")
	}
}

// Hallazgo 30abfc: edCopiarArbol se niega SIEMPRE a copiar un repo con el
// mantenimiento automatico prendido —el de la raiz de la plantilla o uno de
// mas adentro—, antes de copiar su .git, y nombra lo que falta. Con el
// mantenimiento apagado copia, la copia lo trae apagado (se puede volver a
// copiar) y un commit en la copia no lanza nada. Un arbol sin repo adentro
// se copia sin pedirle nada.
func TestHallazgo30abfc_LaCopiaSeNiegaConElMantenimientoPrendido(t *testing.T) {
	negada := func(t *testing.T, caso, plantilla, repoPrendido string) {
		t.Helper()
		destino := t.TempDir()
		err := edCopiarArbolEn(plantilla, destino)
		if err == nil {
			t.Fatalf("30abfc: %s: la copia de un repo con el mantenimiento automatico prendido se niega; copio", caso)
		}
		for _, pide := range []string{"maintenance.auto", "gc.auto", filepath.Join(repoPrendido, ".git", "config")} {
			if !strings.Contains(err.Error(), pide) {
				t.Fatalf("30abfc: %s: la negativa nombra %q: %v", caso, pide, err)
			}
		}
		rel, rerr := filepath.Rel(plantilla, repoPrendido)
		if rerr != nil {
			t.Fatal(rerr)
		}
		if _, serr := os.Lstat(filepath.Join(destino, rel, ".git")); !errors.Is(serr, os.ErrNotExist) {
			t.Fatalf("30abfc: %s: la copia se niega antes de copiar el .git del repo: %v", caso, serr)
		}
	}

	// el repo de la raiz de la plantilla: recien creado, y con cada forma de
	// config que git lee prendida
	prendido := gitRepoCrudo(t, t.TempDir())
	write(t, prendido, "app.go", "package app\n")
	negada(t, "git init a secas", prendido, prendido)
	cfg := filepath.Join(prendido, ".git", "config")
	base, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range gitConfigsDeMantenimiento() {
		if c.apagadoParaGit {
			continue
		}
		if err := os.WriteFile(cfg, append(append([]byte{}, base...), c.anexo...), 0o644); err != nil {
			t.Fatal(err)
		}
		negada(t, c.caso, prendido, prendido)
	}
	if err := os.WriteFile(cfg, base, 0o644); err != nil {
		t.Fatal(err)
	}

	// un repo de mas adentro, con el de la raiz bien armado
	anidada := repo(t)
	adentro := gitRepoCrudo(t, filepath.Join(anidada, "sub", "otro"))
	negada(t, "un repo anidado con el mantenimiento prendido", anidada, adentro)
	gitAnexar(t, filepath.Join(adentro, ".git", "config"), gitSinMantenimiento)
	if err := edCopiarArbolEn(anidada, t.TempDir()); err != nil {
		t.Fatalf("30abfc: con los dos repos apagados la copia sale: %v", err)
	}

	// apagado: copia, y la copia se puede volver a copiar y commitear
	gitAnexar(t, filepath.Join(prendido, ".git", "config"), gitSinMantenimiento)
	copia := edCopiarArbol(t, prendido)
	if raw, err := os.ReadFile(filepath.Join(copia, "app.go")); err != nil || string(raw) != "package app\n" {
		t.Fatalf("30abfc: la copia trae el arbol de la plantilla: %q %v", raw, err)
	}
	if err := gitExigirMantenimientoApagado(filepath.Join(copia, ".git", "config")); err != nil {
		t.Fatalf("30abfc: la copia trae el mantenimiento apagado como su plantilla: %v", err)
	}
	copiaDeLaCopia := edCopiarArbol(t, copia)
	if lanza := gitCommitConTraza(t, copiaDeLaCopia, gitEntornoSolo(t)); len(lanza) != 0 {
		t.Fatalf("30abfc: un commit en la copia no lanza mantenimiento; lanzo:\n%s", strings.Join(lanza, "\n"))
	}

	// un arbol sin repo (un pedazo de .hoom) no tiene nada que apagar
	suelto := t.TempDir()
	write(t, suelto, "findings/a.json", "{}\n")
	if err := edCopiarArbolEn(suelto, t.TempDir()); err != nil {
		t.Fatalf("30abfc: un arbol sin repo adentro se copia sin pedirle nada: %v", err)
	}
}
