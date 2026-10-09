// Tests de regresion de la review de 4 lentes de la cabina C2 sobre
// .hoom/specs/tablero-de-solo-lectura.md:
//
//   - 20261009T191843_bb986a (high, risk; CA-318 y CA-315): la base que
//     recibe gitx sale de base_branch de hoom.yaml. Una base que empieza con
//     "-" (por ejemplo --output=<ruta>) nunca llega a git como una opcion:
//     ninguna LECTURA de gitx (el diff del detalle, la foto del arbol con sus
//     estadisticas, y Touched) crea ni trunca un archivo. La respuesta puede
//     ser un error o un "no disponible"; el disco queda byte a byte igual.
//   - 20261009T191847_43bde9 (medium, risk; CA-315 y CA-318, enmienda 1): el
//     diff del detalle no ejecuta programas externos: ni un diff externo ni
//     un textconv de la configuracion local de git. El driver no corre
//     ninguna vez y el parche trae el contenido real de los archivos.
//
// Todas las rutas hostiles, las marcas y los drivers viven en t.TempDir().
package gitx

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const c2Previo = "CONTENIDO PREVIO\n"

// c2Foto es el contenido de cada archivo bajo dir (sin .git), por ruta
// relativa. Un directorio es "dir".
func c2Foto(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			out[rel+"/"] = "dir"
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(raw)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// c2Cambios dice, ordenado, que cambio entre dos fotos: "creo X", "trunco o
// reescribio X" o "borro X". Vacio si el disco quedo igual.
func c2Cambios(antes, despues map[string]string) []string {
	var out []string
	for k, v := range antes {
		if w, ok := despues[k]; !ok {
			out = append(out, "borro "+k)
		} else if w != v {
			out = append(out, "trunco o reescribio "+k)
		}
	}
	for k := range despues {
		if _, ok := antes[k]; !ok {
			out = append(out, "creo "+k)
		}
	}
	sort.Strings(out)
	return out
}

// c2Fuera arma el directorio de las victimas, fuera del repo y dentro de
// t.TempDir(): "previo" y "previo...HEAD" ya existen con contenido (si git
// los abre para escribir, se truncan), y "nuevo" no existe (si git lo abre,
// aparece). Devuelve el directorio y las dos bases hostiles.
func c2Fuera(t *testing.T) (string, []string) {
	t.Helper()
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	write(t, fuera, "previo", c2Previo)
	write(t, fuera, "previo...HEAD", c2Previo)
	return fuera, []string{
		"--output=" + filepath.Join(fuera, "nuevo"),
		"--output=" + filepath.Join(fuera, "previo"),
	}
}

// c2GitObedeceOutput confirma que el fixture no es ciego: el git de esta
// maquina, con --output=<ruta> como opcion de git diff, SI crea el archivo.
// Si no lo creara, los tests de abajo pasarian con cualquier codigo.
func c2GitObedeceOutput(t *testing.T, dir string) {
	t.Helper()
	testigo := filepath.Join(t.TempDir(), "testigo")
	cmd := exec.Command("git", "diff", "--no-color", "--no-ext-diff", "--output="+testigo, "--")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: git diff --output=<ruta> corre en esta maquina: %v\n%s", err, out)
	}
	if _, err := os.Stat(testigo); err != nil {
		t.Fatalf("fixture: el git de esta maquina crea el archivo de --output (sin eso el test no ve nada): %v", err)
	}
}

// c2SinEfectos corre leer con cada base hostil y exige que ni el directorio
// de las victimas ni el arbol del repo cambien.
func c2SinEfectos(t *testing.T, que string, leer func(dir, base string)) {
	t.Helper()
	dir := dfRepo(t)
	c2GitObedeceOutput(t, dir)
	fuera, hostiles := c2Fuera(t)
	antesFuera, antesRepo := c2Foto(t, fuera), c2Foto(t, dir)
	for _, base := range hostiles {
		leer(dir, base)
		if c := c2Cambios(antesFuera, c2Foto(t, fuera)); len(c) != 0 {
			t.Errorf("bb986a CA-318: %s es una lectura: con la base %q no crea ni trunca ningun archivo; fuera del repo %s",
				que, "--output=<tmp>/"+filepath.Base(strings.TrimPrefix(base, "--output=")), strings.Join(c, ", "))
		}
		if c := c2Cambios(antesRepo, c2Foto(t, dir)); len(c) != 0 {
			t.Errorf("bb986a CA-318: %s es una lectura: con una base hostil el arbol del repo queda igual; %s", que, strings.Join(c, ", "))
		}
		// cada base se juzga sola: se repone lo que la anterior haya roto
		_ = os.Remove(filepath.Join(fuera, "nuevo"))
		_ = os.Remove(filepath.Join(fuera, "nuevo...HEAD"))
		write(t, fuera, "previo", c2Previo)
		write(t, fuera, "previo...HEAD", c2Previo)
	}
}

// Hallazgo 20261009T191843_bb986a. CA-315 y CA-318: el diff del detalle
// (BranchDiff) con una base que empieza con "--output=" no deja que git la
// lea como opcion: no crea <ruta>...HEAD ni trunca el que ya estaba. Y una
// base asi no es una revision: el diff nunca sale "disponible" (o hay error,
// o available es false; CA-315: si git falla, available false con la nota).
func TestHallazgo_bb986a_BranchDiffNoEscribeConUnaBaseQueEsOpcion(t *testing.T) {
	c2SinEfectos(t, "BranchDiff", func(dir, base string) {
		d, err := BranchDiff(dir, base, 256<<10)
		if err == nil && d.Available {
			t.Errorf("bb986a CA-315: una base que empieza con \"-\" no es una revision: BranchDiff da error o available false, dio available true (patch de %d bytes, nota %q)",
				len(d.Patch), d.Note)
		}
	})
}

// Hallazgo 20261009T191843_bb986a. CA-318: la foto del arbol (Snapshot, lo
// que corre en cada GET /api/board y /api/tasks) arma la lista de archivos
// cambiados y las estadisticas del diff con la base. Con una base que empieza
// con "--output=" no crea ni trunca ni <ruta> (la base sola) ni <ruta>...HEAD
// (la base con los tres puntos). El repo tiene cambios commiteados y sin
// commitear, para que los dos pedidos tengan algo que escribir.
func TestHallazgo_bb986a_SnapshotNoEscribeConUnaBaseQueEsOpcion(t *testing.T) {
	c2SinEfectos(t, "Snapshot", func(dir, base string) { _ = Snapshot(dir, base) })
}

// Hallazgo 20261009T191843_bb986a. CA-318: Touched (lo que mira el sobre,
// con la misma base de hoom.yaml) tampoco crea ni trunca nada con una base
// que empieza con "--output=".
func TestHallazgo_bb986a_TouchedNoEscribeConUnaBaseQueEsOpcion(t *testing.T) {
	c2SinEfectos(t, "Touched", func(dir, base string) { _ = Touched(dir, base) })
}

// c2SalidaDelDriver es lo que imprime el driver de diff de prueba: si aparece
// en un parche, el driver corrio.
const c2SalidaDelDriver = "SALIDA-DEL-DRIVER-DE-DIFF"

// c2RepoConDriver arma main con notas.txt y una rama "tarea" que commitea un
// .gitattributes que le pone el driver c2drv a *.txt, cambia notas.txt y
// agrega nuevo.txt. La config LOCAL del repo define diff.c2drv.<clave> con
// un script (fuera del repo) que deja una linea en marca cada vez que corre e
// imprime c2SalidaDelDriver en vez del contenido. Devuelve el repo y la
// marca.
func c2RepoConDriver(t *testing.T, clave string) (dir, marca string) {
	t.Helper()
	idAislarGit(t) // sin la config global ni del sistema del que corre el test
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fuera, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	marca = filepath.Join(fuera, "el-driver-corrio")
	driver := filepath.Join(fuera, "driver.sh")
	if err := os.WriteFile(driver, []byte("#!/bin/sh\necho corrio >> '"+marca+"'\necho "+c2SalidaDelDriver+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	dfGit(t, dir, "init", "-b", "main")
	dfGit(t, dir, "config", "user.email", "test@hoom.dev")
	dfGit(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "notas.txt", "linea vieja\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "inicial")
	dfGit(t, dir, "checkout", "-q", "-b", "tarea")
	write(t, dir, ".gitattributes", "*.txt diff=c2drv\n")
	write(t, dir, "notas.txt", "linea vieja\nlinea real agregada\n")
	write(t, dir, "nuevo.txt", "contenido real del archivo nuevo\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "cambio de la tarea")
	dfGit(t, dir, "config", "diff.c2drv."+clave, driver)
	return dir, marca
}

// c2Corridas es cuantas veces corrio el driver (0 si no hay marca).
func c2Corridas(t *testing.T, marca string) int {
	t.Helper()
	raw, err := os.ReadFile(marca)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(raw), "corrio")
}

// Hallazgo 20261009T191847_43bde9. CA-315 y CA-318 (enmienda 1): con un
// driver diff.<x>.textconv en la config local del repo y un .gitattributes
// que se lo aplica a los archivos que la rama cambio, BranchDiff no corre el
// comando del driver ninguna vez (el driver deja una marca si corre), y el
// parche es el contenido real: exactamente git diff --no-textconv, con las
// lineas de verdad y sin la salida del driver. Lo mismo con un diff externo
// (diff.<x>.command), que ya no corria: es la guarda de la otra mitad de "no
// ejecuta programas externos". El fixture no es ciego: el git diff de
// siempre, sobre ese mismo repo, SI corre el driver.
func TestHallazgo_43bde9_BranchDiffNoEjecutaUnDriverDeLaConfigLocal(t *testing.T) {
	for _, c := range []struct {
		nombre, clave string
		deSiempre     []string // el git diff que si corre el driver
	}{
		{"textconv", "textconv", []string{"diff", "--no-color", "--no-ext-diff", "main...HEAD", "--"}},
		{"diff-externo", "command", []string{"diff", "--no-color", "main...HEAD", "--"}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			dir, marca := c2RepoConDriver(t, c.clave)
			if out := dfGit(t, dir, c.deSiempre...); c2Corridas(t, marca) == 0 || !strings.Contains(out, c2SalidaDelDriver) {
				t.Fatalf("fixture: en esta maquina git %s corre el driver diff.c2drv.%s y muestra su salida (sin eso el test no ve nada): corridas=%d\n%s",
					strings.Join(c.deSiempre, " "), c.clave, c2Corridas(t, marca), out)
			}
			if err := os.Remove(marca); err != nil {
				t.Fatal(err)
			}

			d, err := BranchDiff(dir, "main", 256<<10)

			if n := c2Corridas(t, marca); n != 0 {
				t.Errorf("43bde9 CA-315: el diff del detalle no ejecuta el driver diff.c2drv.%s de la config local: corrio %d veces", c.clave, n)
			}
			if err != nil || !d.Available || d.Note != "" || d.Truncated {
				t.Fatalf("43bde9 CA-315: con un driver de diff configurado el diff sigue disponible, entero y sin nota: err=%v available=%v truncated=%v note=%q",
					err, d.Available, d.Truncated, d.Note)
			}
			if strings.Contains(d.Patch, c2SalidaDelDriver) {
				t.Errorf("43bde9 CA-315: el parche no trae la salida del driver (%s):\n%s", c2SalidaDelDriver, d.Patch)
			}
			for _, real := range []string{"+linea real agregada", "+contenido real del archivo nuevo", "diff --git a/notas.txt b/notas.txt", "diff --git a/nuevo.txt b/nuevo.txt"} {
				if !strings.Contains(d.Patch, real) {
					t.Errorf("43bde9 CA-315: el parche trae el contenido real del archivo (%q):\n%s", real, d.Patch)
				}
			}
			cmd := exec.Command("git", "diff", "--no-color", "--no-ext-diff", "--no-textconv", "main...HEAD", "--")
			cmd.Dir = dir
			crudo, cerr := cmd.Output()
			if cerr != nil {
				t.Fatalf("fixture: git diff --no-ext-diff --no-textconv main...HEAD: %v", cerr)
			}
			if d.Patch != string(crudo) {
				t.Errorf("43bde9 CA-315: el parche es exactamente git diff --no-ext-diff --no-textconv main...HEAD:\nquiere:\n%s\nfue:\n%s", crudo, d.Patch)
			}
			porRuta := map[string]DiffFile{}
			for _, f := range d.Files {
				porRuta[f.Path] = f
			}
			if len(d.Files) != 3 || porRuta["notas.txt"].Insertions != 1 || porRuta["notas.txt"].Deletions != 0 ||
				porRuta["nuevo.txt"].Insertions != 1 || porRuta[".gitattributes"].Insertions != 1 || d.Insertions != 3 || d.Deletions != 0 {
				t.Errorf("43bde9 CA-315: el numstat cuenta las lineas reales (notas.txt 1/0, nuevo.txt 1/0, .gitattributes 1/0): %+v", d.Files)
			}
		})
	}
}
