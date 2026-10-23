// Tests de la delta 1 de la review de historia-en-el-tablero (ronda 3 del
// test-writer) sobre el verbo `hoom item backfill`: su salida de texto nunca
// trae crudos los valores no confiables (la severidad de un hallazgo, el
// nombre de un archivo de hallazgo ilegible) y conserva el fail-closed
// (763d5f, feeb49); el titulo nunca sale de un spec fuera del espacio de
// trabajo (e8b795); y un directorio en el lugar del item se lista con su
// motivo aunque el spec de esa tarea no se pueda leer, sin abortar el resto
// (b3c417). El fallo a mitad del lote (enmienda 1, 142d20) esta en
// historia_hallazgos3_unix_test.go. Escritos desde el spec
// .hoom/specs/historia-en-el-tablero.md (enmienda 1) y los textos de los
// hallazgos, sin leer la implementacion. Repos git reales en t.TempDir().
// Todo identificador de este archivo lleva el prefijo h3.
package itemcmd

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
)

const (
	// h3SevHostil es una severidad con un hipervinculo OSC 8 (ESC ... BEL),
	// un retorno de carro y un salto de linea que falsifica una linea.
	h3SevHostil = "\x1b]8;;http://x\x07hihg\r\nFALSO  inyectado  creado"
	// h3Falso es lo que una linea falsificada diria al principio.
	h3Falso = "FALSO"
	// h3Secreto es el titulo del spec que vive FUERA del espacio de trabajo.
	h3Secreto = "SECRETO-DE-AFUERA"
)

// h3NombresHostiles son nombres de archivos de hallazgo ilegibles con ESC
// (una secuencia de color) y con un salto de linea.
var h3NombresHostiles = []string{
	"20261023T000000_a\x1b[31mROJO\x1b[0m.json",
	"20261023T000000_b\nFALSO  inyectado  creado.json",
}

// h3EsCrudo: dentro de una linea, un control C0 (salvo \t), DEL, un control
// C1, o un caracter de embebido, anulacion o aislamiento de direccion.
func h3EsCrudo(r rune) bool {
	switch {
	case r < 0x20:
		return r != '\t'
	case r >= 0x7f && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// h3TextoLimpio exige que la salida de texto de backfill tenga el resumen y
// una linea por tarea (n), que ninguna linea traiga caracteres crudos y que
// ninguna sea una linea falsificada.
func h3TextoLimpio(t *testing.T, ca, out string, n int) []string {
	t.Helper()
	lineas := htLineas(out)
	for _, l := range lineas {
		for _, r := range l {
			if h3EsCrudo(r) {
				t.Fatalf("%s: la salida de 'hoom item backfill' no trae caracteres de control crudos (U+%04X) en %+q\n%+q", ca, r, l, out)
			}
		}
		if strings.HasPrefix(strings.TrimSpace(l), h3Falso) {
			t.Fatalf("%s: la salida de 'hoom item backfill' no trae lineas falsificadas: %+q\n%+q", ca, l, out)
		}
	}
	if len(lineas) != 1+n {
		t.Fatalf("%s: el resumen y una linea por tarea (%d), fueron %d lineas:\n%+q", ca, 1+n, len(lineas), out)
	}
	return lineas
}

// ---------------------------------------------------------------------------
// 763d5f / feeb49: valores no confiables en la salida de texto.
// ---------------------------------------------------------------------------

// CA-450, hallazgos 763d5f y feeb49: un hallazgo abierto de la tarea con una
// severidad que trae ESC/OSC, \r y \n, o archivos de hallazgo ilegibles
// cuyo nombre trae ESC o \n. 'hoom item backfill' (con --dry-run y sin) no
// imprime esos caracteres crudos ni lineas falsificadas, y el fail-closed se
// conserva: la tarea no califica, no se crea, y su linea dice "no crea" (con
// la severidad hostil, nombrando el hallazgo). El control de 763d5f: "sana",
// con sus hallazgos cerrados, se crea.
func TestCA450_Hallazgo763d5f_BackfillTextoSinCrudos(t *testing.T) {
	t.Run("763d5f severidad hostil", func(t *testing.T) {
		root := htRepo(t, h1BlockOn)
		htCerrar(t, root, htHallazgo(t, root, "corr", "high"), finding.StatusCorrected)
		htCerrar(t, root, htHallazgo(t, root, "sana", "high"), finding.StatusCorrected)
		hostil := htHallazgo(t, root, "corr", "high")
		h1Severidad(t, root, hostil, h3SevHostil)
		htCommitear(t, root, "hallazgos")
		for _, args := range [][]string{{"backfill", "--dry-run"}, {"backfill"}} {
			ca := "CA-450 (763d5f, " + strings.Join(args, " ") + ")"
			out, _, err := htExec(t, root, args...)
			if err != nil {
				t.Fatalf("%s: backfill sale bien aunque una tarea no califique: %v", ca, err)
			}
			lineas := h3TextoLimpio(t, ca, out, 2)
			if l := htLineaDe(t, ca, lineas, "corr"); !strings.Contains(l, "no crea") || !strings.Contains(l, hostil) {
				t.Fatalf("%s: la linea de corr dice \"no crea\" y nombra el hallazgo %s: %+q", ca, hostil, l)
			}
		}
		if _, err := os.Stat(item.Path(root, "corr")); !os.IsNotExist(err) {
			t.Fatalf("CA-450 (763d5f): el fail-closed se conserva: corr no se crea: %v", err)
		}
		if _, err := os.Stat(item.Path(root, "sana")); err != nil {
			t.Fatalf("CA-448 (control): sana se crea: %v", err)
		}
	})
	t.Run("feeb49 nombres de hallazgos ilegibles hostiles", func(t *testing.T) {
		root := htRepo(t, h1BlockOn)
		htCerrar(t, root, htHallazgo(t, root, "corr", "high"), finding.StatusCorrected)
		htCommitear(t, root, "hallazgo cerrado")
		for _, n := range h3NombresHostiles {
			icEscribir(t, root, filepath.Join(".hoom", "findings", n), "{esto no es un hallazgo")
		}
		for _, args := range [][]string{{"backfill", "--dry-run"}, {"backfill"}} {
			ca := "CA-450 (feeb49, " + strings.Join(args, " ") + ")"
			out, _, err := htExec(t, root, args...)
			if err != nil {
				t.Fatalf("%s: backfill sale bien aunque una tarea no califique: %v", ca, err)
			}
			lineas := h3TextoLimpio(t, ca, out, 1)
			if l := htLineaDe(t, ca, lineas, "corr"); !strings.Contains(l, "no crea") {
				t.Fatalf("%s: el fail-closed se conserva: la linea de corr dice \"no crea\": %+q", ca, l)
			}
		}
		if _, err := os.Stat(item.Path(root, "corr")); !os.IsNotExist(err) {
			t.Fatalf("CA-450 (feeb49): el fail-closed se conserva: corr no se crea: %v", err)
		}
	})
}

// ---------------------------------------------------------------------------
// e8b795: el spec del espacio de trabajo por un symlink.
// ---------------------------------------------------------------------------

// h3SinSecreto exige que ni la salida ni el error traigan el titulo de afuera.
func h3SinSecreto(t *testing.T, ca, que, salida string, err error) {
	t.Helper()
	if err != nil && strings.Contains(err.Error(), h3Secreto) {
		t.Fatalf("%s: el error de %s no trae el titulo de afuera: %v", ca, que, err)
	}
	if strings.Contains(salida, h3Secreto) {
		t.Fatalf("%s: %s no usa el titulo de un spec fuera del espacio de trabajo (por un symlink en .hoom/specs):\n%s", ca, que, salida)
	}
}

// CA-447, hallazgo e8b795: en el espacio de trabajo de la tarea "fuga",
// .hoom/specs es un symlink a un directorio FUERA del espacio, con un
// fuga.md cuyo encabezado es conocido; la tarea tiene un verde aprobado e
// integrado atado a .hoom/specs/fuga.md. Backfill no usa ese titulo en
// ninguna salida (resultado, texto, --json, con --dry-run y sin) ni en
// ningun item. Es aceptable que termine con error, o que trate la tarea con
// el slug como titulo; no que aparezca el titulo de afuera.
func TestCA447_Hallazgoe8b795_BackfillSpecPorSymlinkNoDaTitulo(t *testing.T) {
	root := htRepo(t, h1BlockOn)
	wt := htWorktree(t, root, "fuga")
	fuera := t.TempDir()
	htSpec(t, fuera, "fuga", "# Spec: "+h3Secreto)
	link := filepath.Join(wt, ".hoom", "specs")
	if err := os.RemoveAll(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(fuera, ".hoom", "specs"), link); err != nil {
		t.Fatal(err)
	}
	htVeredicto(t, wt, "fuga", htT0, htGatesVerdes(), true)
	ca := "CA-447 (e8b795)"

	res, err := Backfill(root, "main", "high", true, time.Now().UTC())
	raw, _ := json.Marshal(res)
	h3SinSecreto(t, ca, "backfill --dry-run", string(raw), err)
	for _, args := range [][]string{{"backfill", "--dry-run"}, {"backfill", "--dry-run", "--json"}, {"backfill", "--json"}, {"backfill"}} {
		out, _, err := htExec(t, root, args...)
		h3SinSecreto(t, ca, "'hoom item "+strings.Join(args, " ")+"'", out, err)
	}
	err = filepath.WalkDir(filepath.Join(root, ".hoom", "items"), func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		if strings.Contains(string(b), h3Secreto) {
			t.Fatalf("%s: ningun item lleva el titulo de afuera: %s\n%s", ca, p, b)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if _, err := os.Stat(item.Path(root, "fuga")); err == nil {
		if got := h1Titulo(t, root, "fuga"); got != "fuga" {
			t.Fatalf("%s: si crea el item de fuga, su titulo es el slug: %q", ca, got)
		}
	}
}

// ---------------------------------------------------------------------------
// b3c417: un directorio en el lugar del item y un spec ilegible.
// ---------------------------------------------------------------------------

// CA-446, CA-453, hallazgo b3c417: la tarea "doble" tiene un DIRECTORIO en
// .hoom/items/doble.yaml y su spec sin permiso de lectura. La ruta del item
// ya hace imposible crearlo, asi que no hace falta leer el spec: backfill
// (con --dry-run y sin) sale bien, lista "doble" como que no califica con el
// motivo del item, y crea las otras tareas que califican ("control", con
// spec, y "corr", sin spec).
func TestCA453_Hallazgob3c417_DirectorioYSpecIlegible(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("como root (o en windows) los permisos 000 no impiden leer")
	}
	root := htRepo(t, h1BlockOn)
	for _, s := range []string{"doble", "control"} {
		htSpec(t, root, s, "# Spec: "+s)
		htAprobar(t, root, s)
	}
	htCommitear(t, root, "specs")
	for _, s := range []string{"doble", "control"} {
		htVeredicto(t, root, s, htT0, htGatesVerdes(), true)
	}
	htCerrar(t, root, htHallazgo(t, root, "corr", "high"), finding.StatusCorrected)
	htCommitear(t, root, "veredictos y hallazgo")
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "items", "doble.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	spec := filepath.Join(root, ".hoom", "specs", "doble.md")
	if err := os.Chmod(spec, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(spec, 0o644) })

	for _, dry := range []bool{true, false} {
		ca := "CA-446 (b3c417, --dry-run)"
		if !dry {
			ca = "CA-453 (b3c417)"
		}
		res, err := Backfill(root, "main", "high", dry, time.Now().UTC())
		if err != nil {
			t.Fatalf("%s: con un directorio en el lugar del item y el spec ilegible, backfill sale bien y lista la tarea: %v", ca, err)
		}
		h2NoCalificaPorDirectorio(t, ca, res, "doble")
		for _, s := range []string{"control", "corr"} {
			if !h1Tiene(res.Creados, s) {
				t.Fatalf("%s: la tarea %s, que califica, se crea (o se crearia): %s", ca, s, h2Fmt(res))
			}
		}
	}
	for _, s := range []string{"control", "corr"} {
		if _, err := os.Stat(item.Path(root, s)); err != nil {
			t.Fatalf("CA-447 (b3c417): backfill crea el item de %s: %v", s, err)
		}
	}
	if fi, err := os.Stat(filepath.Join(root, ".hoom", "items", "doble.yaml")); err != nil || !fi.IsDir() {
		t.Fatalf("CA-453 (b3c417): el directorio en el lugar del item de doble no se toca: %v", err)
	}
}
