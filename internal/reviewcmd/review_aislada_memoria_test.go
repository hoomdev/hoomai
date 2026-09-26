// Tests adversariales del spec .hoom/specs/review-aislada-y-modelo-elegido.md
// (enmienda 1) sobre la evidencia de `hoom review`, a partir de la segunda
// review cruzada de la implementacion:
//
//   - CA-413: el tope se aplica al LEER toda salida de git: la lista de
//     nombres de los no rastreados no se lee entera antes de cortar, y el diff
//     rastreado no se guarda dos veces ("Nunca retiene mas de maxBytes +
//     64 KiB").
//   - CA-401: un binario lleva la linea que git escribe para un binario, sin
//     contenido, aunque la config local del repo le ponga un driver de diff
//     (textconv o diff externo) por .gitattributes; el driver nunca corre.
//   - CA-414: un git merge-base que falla es el error de Evidence aunque el
//     spec solo ya pase el tope: el error de git no queda tapado por Over.
//   - CA-412: un spec FIFO no es un archivo regular: error sin abrirlo ni
//     quedarse bloqueado; y un spec que cambia a symlink de afuera entre los
//     git que corre Evidence nunca se lee de afuera.
//
// Reusa los fixtures de review_aislada_test.go (raRepo, raEvidenceConReloj,
// raOver, raGitReal, raClonShallow, ...). En este paquete ningun test corre en
// paralelo: el TotalAlloc que miden los tests de memoria es el de Evidence.
package reviewcmd

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// ---------------------------------------------------------------- CA-413

// ramListaNoRastreados es el tamano en bytes de la lista de nombres de los no
// rastreados de root, tal como la escribe git: lo que Evidence NO puede leer
// entero antes de aplicar el tope.
func ramListaNoRastreados(t *testing.T, root string) int {
	t.Helper()
	cmd := exec.Command("git", "ls-files", "--others", "--exclude-standard", "-z")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("CA-413: fixture: git ls-files --others: %v", err)
	}
	return len(out)
}

// CA-413: "Se lee con tope: hoom deja de leer la salida de git ... en cuanto
// pasan maxBytes. Nunca retiene mas de maxBytes + 64 KiB." La salida de git
// incluye la LISTA de nombres de los no rastreados. Con 20.000 no rastreados
// de nombre largo (la lista pesa ~6 MB) y un tope de 1 KiB, Evidence vuelve
// con Over y asigna mucho menos que esa lista.
//
// Umbral: 2 MiB de TotalAlloc durante Evidence. Leer la lista entera antes de
// cortar cuesta al menos su tamano (~6 MB, el triple del umbral, y mas con el
// crecimiento del buffer). Armar la evidencia hasta cortar cuesta el tope +
// 64 KiB, los buffers de lectura y un punado de procesos git: decenas o
// cientos de KiB, lejos de 2 MiB. El primer par de parches de archivo nuevo
// (cada uno lleva el nombre de 300 bytes tres veces) ya pasa el tope.
func TestCA413_ListaDeNoRastreadosSeLeeConTope(t *testing.T) {
	if testing.Short() {
		t.Skip("CA-413: planta 20.000 archivos no rastreados; corre sin -short")
	}
	root := raRepo(t, "")
	const dirs, porDir = 20, 1000
	for d := 0; d < dirs; d++ {
		dir := fmt.Sprintf("no-rastreado-%02d-", d) + strings.Repeat("d", 80)
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
		for i := 0; i < porDir; i++ {
			nombre := fmt.Sprintf("archivo-%02d-%04d-", d, i) + strings.Repeat("n", 180) + ".txt"
			if err := os.WriteFile(filepath.Join(root, dir, nombre), []byte("contenido del no rastreado\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	lista := ramListaNoRastreados(t, root)
	if lista < 4<<20 {
		t.Fatalf("CA-413: fixture: la lista de nombres de los no rastreados pesa mas de 4 MiB: %d bytes", lista)
	}

	const tope = 1024
	ev, err, asignado := raEvidenceConReloj(t, "CA-413", 120*time.Second, root, "main", "", tope)
	if err != nil {
		t.Fatalf("CA-413: Evidence: %v", err)
	}
	raOver(t, "CA-413", ev, tope)
	if asignado >= 2<<20 {
		t.Fatalf("CA-413: con un tope de 1 KiB Evidence asigno %.1f MiB con una lista de no rastreados de %.1f MiB: "+
			"leyo la lista entera antes de aplicar el tope (umbral 2 MiB)", float64(asignado)/(1<<20), float64(lista)/(1<<20))
	}
}

// CA-413: el diff rastreado se lee UNA vez, directo a la evidencia. Con un
// diff rastreado de ~3,9 MiB y un tope de 5 MiB (no hay Over), Evidence
// devuelve el parche entero, lo que retiene no pasa maxBytes + 64 KiB, y
// asigna menos de 2,75 veces el parche.
//
// Umbral: 2,75x el parche, el punto medio entre leerlo una vez y dos. El
// parche queda justo debajo de 4 MiB a proposito: asi un buffer que crece
// duplicando para en 4 MiB y no en 8. Medido con go1.27 para ese tamano,
// guardarlo UNA vez cuesta 1,31x (reservar el tope + 64 KiB), 2,06x
// (bytes.Buffer o cmd.Output) o 2,43x (io.ReadAll); guardarlo DOS veces (toda
// la salida de git a un buffer y despues copiada a otro) cuesta 3,07x con
// bytes.Buffer y 3,43x con io.ReadAll (con un parche apenas sobre 4 MiB, la
// review lo midio en ~5x). Dos copias vivas del parche (~7,8 MiB) pasan
// ademas maxBytes + 64 KiB (5,06 MiB).
func TestCA413_DiffRastreadoNoSeGuardaDosVeces(t *testing.T) {
	if testing.Short() {
		t.Skip("CA-413: arma un diff rastreado de ~4 MiB; corre sin -short")
	}
	root := raRepo(t, "")
	write(t, root, "grande.txt", "linea base del archivo grande\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "archivo grande en la base")

	linea := func(i int) string {
		return fmt.Sprintf("linea %06d del diff rastreado grande: relleno sin commitear, 64 B\n", i)
	}
	ancho := len(linea(0))
	n := (31 << 17) / (ancho + 1) // parche ~3,875 MiB: cada linea va con su '+'
	var b strings.Builder
	b.Grow(n * ancho)
	for i := 0; i < n; i++ {
		b.WriteString(linea(i))
	}
	write(t, root, "grande.txt", b.String())

	const tope = 5 << 20
	ev, err, asignado := raEvidenceConReloj(t, "CA-413", 60*time.Second, root, "main", "", tope)
	if err != nil {
		t.Fatalf("CA-413: Evidence: %v", err)
	}
	if ev.Over {
		t.Fatalf("CA-413: un diff de ~3,9 MiB con tope de 5 MiB no pasa el tope: Bytes %d", ev.Bytes)
	}
	p := len(ev.Diff)
	if p < 7<<19 || p > 4<<20-64<<10 {
		t.Fatalf("CA-413: fixture: el parche queda entre 3,5 MiB y 4 MiB - 64 KiB: %d bytes", p)
	}
	if !bytes.Contains(ev.Diff, []byte("+"+linea(n-1))) || !bytes.Contains(ev.Diff, []byte("-linea base del archivo grande\n")) {
		t.Fatalf("CA-413: bajo el tope la evidencia trae el parche entero (primera y ultima linea)")
	}
	if ev.Bytes != p || ev.SHA256 != raSHA(ev.Diff, nil) {
		t.Fatalf("CA-401: Bytes = len(Diff) y SHA256 del diff: %d vs %d, %q", ev.Bytes, p, ev.SHA256)
	}
	if retenido := cap(ev.Diff) + cap(ev.Spec); retenido > tope+64<<10 {
		t.Fatalf("CA-413: la evidencia devuelta retiene %d bytes, mas que maxBytes + 64 KiB (%d)", retenido, tope+64<<10)
	}
	if asignado*4 >= 11*uint64(p) {
		t.Fatalf("CA-413: Evidence asigno %.1f MiB para un parche de %.1f MiB (%.2fx): guarda el diff rastreado mas de una vez (umbral 2,75x)",
			float64(asignado)/(1<<20), float64(p)/(1<<20), float64(asignado)/float64(p))
	}
	ref := raEvidencia(t, "CA-413", root, "")
	if !bytes.Equal(ref.Diff, ev.Diff) || ref.SHA256 != ev.SHA256 {
		t.Fatalf("CA-413: bajo el tope, el tope no cambia la evidencia: %d vs %d bytes", len(ev.Diff), len(ref.Diff))
	}
}

// ---------------------------------------------------------------- CA-401

// CA-401 (caso limite): "Un binario lleva la linea que git escribe para un
// binario, sin contenido." La config LOCAL del repo define un driver de diff
// (textconv, o un diff externo) y .gitattributes se lo pone a *.bin. La
// evidencia de un binario rastreado que cambia (x.bin) y de uno no rastreado
// (y.bin) sigue trayendo la linea 'Binary files ... differ' de git, sin la
// salida del driver ni el contenido; y el driver no corre nunca (deja una
// marca fuera del arbol si corre). Con textconv, ademas, x.bin no puede
// desaparecer de la evidencia: los dos lados convertidos dan el mismo texto.
func TestCA401_BinarioConDriverDeDiffLlevaLaLineaDeGit(t *testing.T) {
	for _, c := range []struct{ nombre, clave string }{
		{"textconv", "diff.fake.textconv"},
		{"diff-externo", "diff.fake.command"},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := raRepo(t, "")
			write(t, root, ".gitattributes", "*.bin diff=fake\n")
			write(t, root, "x.bin", "\x00\x01MARCA-BINARIA-BASE\x00")
			git(t, root, "add", "-A")
			git(t, root, "commit", "-q", "-m", "binario con driver de diff")

			fuera, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			marca := filepath.Join(fuera, "el-driver-corrio")
			driver := filepath.Join(fuera, "driver.sh")
			if err := os.WriteFile(driver, []byte("#!/bin/sh\necho corrio >> '"+marca+"'\necho SALIDA-DEL-DRIVER-DE-DIFF\n"), 0o755); err != nil {
				t.Fatal(err)
			}
			git(t, root, "config", c.clave, driver)

			write(t, root, "x.bin", "\x00\x02MARCA-BINARIA-NUEVA\x00\xff")
			write(t, root, "y.bin", "\x00\x03MARCA-BINARIA-NO-RASTREADA\x00\xff")

			ev, err, _ := raEvidenceConReloj(t, "CA-401", 60*time.Second, root, "main", "", raTopeGrande)
			if err != nil || ev.Over {
				t.Fatalf("CA-401: Evidence: %v (Over %v)", err, ev.Over)
			}
			diff := string(ev.Diff)
			if _, err := os.Stat(marca); err == nil {
				corridas, _ := os.ReadFile(marca)
				t.Errorf("CA-401: el driver de diff %s de la config local corrio %d veces armando la evidencia",
					c.clave, strings.Count(string(corridas), "corrio"))
			}
			for _, nunca := range []string{"SALIDA-DEL-DRIVER-DE-DIFF", "MARCA-BINARIA", "GIT binary patch"} {
				if strings.Contains(diff, nunca) {
					t.Errorf("CA-401: la evidencia de un binario no trae %q (ni la salida del driver ni el contenido):\n%s", nunca, diff)
				}
			}
			x := raSeccion(diff, "diff --git a/x.bin b/x.bin")
			if !strings.Contains(x, "\nBinary files a/x.bin and b/x.bin differ\n") {
				t.Errorf("CA-401: el binario rastreado que cambia lleva la linea de git 'Binary files a/x.bin and b/x.bin differ':\n%s", diff)
			}
			y := raSeccion(diff, "diff --git a/y.bin b/y.bin")
			if !strings.Contains(y, "\nnew file mode ") || !strings.Contains(y, "\nBinary files /dev/null and b/y.bin differ\n") {
				t.Errorf("CA-401: el binario no rastreado va como archivo nuevo con la linea de git 'Binary files /dev/null and b/y.bin differ':\n%s", diff)
			}
		})
	}
}

// ---------------------------------------------------------------- CA-414

// CA-414 (orden): "si falla git merge-base (un clon shallow, una base que no
// existe) ... Evidence devuelve el error de git". Sin excepcion por tamano: un
// spec que solo ya pasa el tope no tapa el error con Over. Evidence (tope
// 1 KiB) devuelve el error y no Over, y `hoom review` (tope por defecto,
// 320 KiB, con un spec de ~420 KiB) no lanza ninguna pasada, no escribe
// registro y no le dice al usuario que la evidencia pasa el tope.
func TestCA414_ErrorDeMergeBaseAntesQueUnSpecSobreElTope(t *testing.T) {
	const spec = ".hoom/specs/grande.md"
	grande := strings.Repeat("criterio de un spec mas grande que el tope, que no tapa el error de git\n", 6000)
	if len(grande) <= 320<<10 {
		t.Fatalf("CA-414: fixture: el spec pasa el tope por defecto: %d", len(grande))
	}
	casos := []struct {
		nombre string
		armar  func(t *testing.T) (root, base string)
	}{
		{"base-inexistente", func(t *testing.T) (string, string) {
			root := raRepo(t, "")
			raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			write(t, root, "hoom.yaml", strings.Replace(string(raw), "base_branch: main\n", "base_branch: rama-que-no-existe\n", 1))
			git(t, root, "commit", "-q", "-am", "base_branch inexistente")
			raCambio(t, root)
			return root, "rama-que-no-existe"
		}},
		{"clon-shallow-sin-merge-base", func(t *testing.T) (string, string) {
			clon := raClonShallow(t)
			write(t, clon, "rama.go", "package app\n\nvar Rama = 3 // sin commitear\n")
			return clon, "main"
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root, base := c.armar(t)
			write(t, root, spec, grande)

			ev, err, _ := raEvidenceConReloj(t, "CA-414", 60*time.Second, root, base, spec, 1024)
			if err == nil || ev.Over {
				t.Errorf("CA-414: %s: con un spec sobre el tope Evidence devuelve igual el error de git, no Over: err %v, Over %v, Bytes %d",
					c.nombre, err, ev.Over, ev.Bytes)
			}

			cx := raInstalar(t, bin, "codex", "")
			var out bytes.Buffer
			res, rerr := Run(root, base, Options{Provider: "codex", Lens: "risk", Spec: spec}, &out)
			if cx.veces() != 0 || len(res.Passes) != 0 || res.Status == "revisado" {
				t.Fatalf("CA-414: %s: la review no lanza ninguna pasada (codex %d): %+v %v\n%s", c.nombre, cx.veces(), res, rerr, out.String())
			}
			if recs, _ := Records(root); len(recs) != 0 {
				t.Fatalf("CA-414: %s: sin registro de review: %+v", c.nombre, recs)
			}
			msg := out.String()
			if rerr != nil {
				msg += "\n" + rerr.Error()
			}
			if strings.Contains(msg, "pasa el tope") {
				t.Fatalf("CA-414: %s: el error de git no queda tapado por el tope del spec: la review dice que la evidencia pasa el tope\n%s", c.nombre, msg)
			}
		})
	}
}

// ---------------------------------------------------------------- CA-412

// CA-412 (caso limite): un spec que es un FIFO existe pero no es un archivo
// regular: Evidence devuelve 'el spec <ruta> no es un archivo del arbol' sin
// leerlo, y vuelve enseguida. Abrir un FIFO para leer bloquea hasta que
// aparece quien escriba: una implementacion que lo abre antes de mirar que es
// (por ejemplo, abrir y despues fstat del descriptor, para cerrar la carrera
// entre mirar y leer) se queda colgada. Al final el test abre el FIFO para
// escribir, asi un Evidence colgado se destraba y no queda leyendo.
func TestCA412_SpecFIFONoSeAbreNiBloquea(t *testing.T) {
	mkfifo, err := exec.LookPath("mkfifo")
	if err != nil {
		t.Skip("sin mkfifo")
	}
	root := raRepo(t, "")
	raCambio(t, root)
	const spec = ".hoom/specs/x.md"
	p := filepath.Join(root, spec)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(mkfifo, p).CombinedOutput(); err != nil {
		t.Fatalf("CA-412: fixture: mkfifo: %v %s", err, out)
	}
	t.Cleanup(func() {
		if f, err := os.OpenFile(p, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
			f.Close()
		}
	})

	ev, err, _ := raEvidenceConReloj(t, "CA-412", 30*time.Second, root, "main", spec, raTopeGrande)
	if err == nil || !strings.Contains(err.Error(), raNoEsDelArbol(spec)) {
		t.Fatalf("CA-412: un spec FIFO no es un archivo regular: Evidence devuelve %q: %v (spec %q)", raNoEsDelArbol(spec), err, ev.Spec)
	}
	if len(ev.Spec) != 0 {
		t.Fatalf("CA-412: el spec FIFO no se lee: %q", ev.Spec)
	}
}

// CA-412 (la carrera, en lo que se puede reproducir sin azar): el spec tiene
// que ser un archivo regular del arbol cuando se LEE, no solo cuando se mira.
// Un git falso al frente del PATH cambia el spec por un symlink a un archivo
// de afuera justo en la invocacion k de git (k = 1..N, N = las que hace
// Evidence) y despues hace de git de verdad. Para cada k, Evidence o devuelve
// 'el spec <ruta> no es un archivo del arbol', o trae el spec original: nunca
// el texto de afuera. Es determinista (las invocaciones de git son en serie)
// y atrapa una implementacion que mira el spec, corre git y lo lee despues.
// NO alcanza la ventana entre mirar y leer sin ningun git en el medio: esa
// carrera no se reproduce sin azar y aca no hay un test de carrera.
func TestCA412_SpecQueCambiaMientrasCorreGitNoSeLeeDeAfuera(t *testing.T) {
	real := raGitReal(t)
	bin := raPATH(t)
	root := raRepo(t, "")
	raCambio(t, root)
	write(t, root, "nuevo.go", "package app\n\n// no rastreado\n")
	const spec = ".hoom/specs/x.md"
	const original = "# Spec x\n\nel spec que se miro y se tiene que leer\n"
	p := filepath.Join(root, spec)
	fuera := raFuera(t, "fuera.md", "# SECRETO-FUERA-DEL-ARBOL\n")

	ctl := t.TempDir()
	s := "#!/bin/sh\nd='" + ctl + "'\n" +
		"n=$(cat \"$d/n\" 2>/dev/null || echo 0); n=$((n+1)); echo $n > \"$d/n\"\n" +
		"if [ \"$n\" = \"$(cat \"$d/k\" 2>/dev/null)\" ]; then\n" +
		"  rm -f '" + p + "' && ln -s '" + fuera + "' '" + p + "' && echo $n >> \"$d/cambios\"\n" +
		"fi\n" +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	armar := func(k int) (Evidencia, error) {
		t.Helper()
		if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
		write(t, root, spec, original)
		for nombre, v := range map[string]string{"n": "0", "k": strconv.Itoa(k)} {
			if err := os.WriteFile(filepath.Join(ctl, nombre), []byte(v+"\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		ev, err, _ := raEvidenceConReloj(t, "CA-412", 60*time.Second, root, "main", spec, raTopeGrande)
		return ev, err
	}
	invocaciones := func() int {
		raw, _ := os.ReadFile(filepath.Join(ctl, "n"))
		n, _ := strconv.Atoi(strings.TrimSpace(string(raw)))
		return n
	}

	ev, err := armar(0)
	if err != nil || string(ev.Spec) != original {
		t.Fatalf("CA-412: sin cambios, el spec del arbol se lee: %v %q", err, ev.Spec)
	}
	n := invocaciones()
	if n == 0 {
		t.Fatalf("CA-412: fixture: Evidence corre git por el PATH (como en CA-414) y el git falso lo ve")
	}
	for k := 1; k <= n; k++ {
		ev, err := armar(k)
		if bytes.Contains(ev.Spec, []byte("SECRETO-FUERA-DEL-ARBOL")) {
			t.Fatalf("CA-412: con el spec cambiado a un symlink de afuera en la invocacion %d de %d de git, Evidence leyo el archivo de afuera: %q",
				k, n, ev.Spec)
		}
		if err != nil && !strings.Contains(err.Error(), raNoEsDelArbol(spec)) {
			t.Fatalf("CA-412: con el spec cambiado en la invocacion %d de git, el error es %q: %v", k, raNoEsDelArbol(spec), err)
		}
		if err == nil && string(ev.Spec) != original {
			t.Fatalf("CA-412: con el spec cambiado en la invocacion %d de git, sin error Evidence trae el spec que miro: %q", k, ev.Spec)
		}
	}
	if raw, _ := os.ReadFile(filepath.Join(ctl, "cambios")); len(bytes.Fields(raw)) == 0 {
		t.Fatalf("CA-412: fixture: el git falso cambio el spec por el symlink al menos una vez (N = %d)", n)
	}
}
