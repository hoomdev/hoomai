//go:build !windows

// Tests adversariales del spec .hoom/specs/gates-por-ruta.md contra el
// BINARIO:
//
//   - CA-809: con un aplica_en invalido (los tres errores), `hoom verify`
//     sale distinto de 0 con "hoom.yaml invalido: " y el texto exacto, sin
//     correr ningun gate y sin escribir veredicto;
//   - CA-813: `hoom verify --help` nombra --cierre, con el uso EXACTO de la
//     aclaracion 1;
//   - CA-814: el verify de trabajo de una tarea deja no_aplica; `hoom task
//     done` se niega con el texto exacto y sin cerrar; `hoom check` sale 0 y
//     VERDE (aclaracion 21); tras `hoom verify --cierre` commiteado, task done
//     cierra.
//
// Usa qcHoom, qcCorrer, qcGit y qcSinCorrida (quien_cierra_test.go). Compila
// el binario: se omite con -short. Escritos desde el spec, las aclaraciones
// de la ronda 0 y el go doc, sin leer la implementacion.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
	hmanifest "github.com/hoomdev/hoomai/internal/manifest"
	htaskcmd "github.com/hoomdev/hoomai/internal/taskcmd"
)

// gprUso es el uso de `hoom verify` de la aclaracion 1, exacto.
const gprUso = `Uso: hoom verify [--full] [--cierre] [--gate a,b] [--spec <ruta>] [--json]

  --full          Ignora el scoping por diff (corrida completa, ej. nocturna)
  --cierre        Corre todos los gates sin mirar aplica_en, con su
                  diff_cmd: el verify que exigen 'hoom task done' e integrar
  --gate a,b      Ejecuta solo esos gates (el resto queda 'skipped'); el
                  veredicto queda PARCIAL: diagnostico, nunca referencia
                  de 'hoom check' ni de 'hoom task done'
  --spec <ruta>   Asocia el veredicto a un spec y suma los gates spec_lint,
                  spec_trace y spec_approved
  --json          Emite el veredicto como JSON en stdout (para agentes)

'hoom verify' no tiene subcomandos ni argumentos posicionales.
Accion: para leer veredictos ya emitidos usa 'hoom report'; para el estado
del proyecto, 'hoom status'.`

func gprEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func gprCommit(t *testing.T, dir, msg string) {
	t.Helper()
	qcGit(t, dir, "add", "-A")
	qcGit(t, dir, "commit", "-q", "-m", msg)
}

// gprRepo es un proyecto git en main con hoom.yaml (base_branch main y
// gatesYAML), el .gitignore canonico de .hoom, app/Models/User.php y
// resources/js/app.js, commiteado.
func gprRepo(t *testing.T, gatesYAML string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	qcGit(t, dir, "init", "-q", "-b", "main")
	qcGit(t, dir, "config", "user.email", "test@hoom.dev")
	qcGit(t, dir, "config", "user.name", "hoom test")
	gprEscribir(t, dir, hmanifest.FileName, "schema: hoom/v1\nproject: demo\nbase_branch: main\n"+gatesYAML)
	gprEscribir(t, dir, ".hoom/.gitignore", hoomfs.GitignoreBody())
	gprEscribir(t, dir, "app/Models/User.php", "<?php\n")
	gprEscribir(t, dir, "resources/js/app.js", "console.log(1)\n")
	gprCommit(t, dir, "inicial")
	return dir
}

// gprVeredictos cuenta los veredictos en disco.
func gprVeredictos(dir string) int {
	es, _ := os.ReadDir(filepath.Join(dir, ".hoom", "verdicts"))
	n := 0
	for _, e := range es {
		if strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- CA-809

// CA-809: los tres errores de aplica_en por el binario, con y sin --json y
// con --cierre: exit distinto de 0, el texto exacto con su prefijo, ningun
// gate corre (test dejaria gpr-corrio) y no queda veredicto.
func TestCA809_VerifyConAplicaEnInvalidoNoCorreNiEscribe(t *testing.T) {
	bin := qcHoom(t)
	for _, c := range []struct {
		nombre, linea, want string
	}{
		{"lista vacia", "    aplica_en: []\n", fmt.Sprintf(hmanifest.ErrAplicaEnLista, "mutation")},
		{"patron invalido", "    aplica_en: [\"a//b\"]\n", fmt.Sprintf(hmanifest.ErrAplicaEnPatron, "mutation", "a//b")},
		{"clave desconocida", "    aplicaen: [app/**]\n", fmt.Sprintf(hmanifest.ErrGateClaveDesconocida, "mutation", "aplicaen")},
	} {
		for _, args := range [][]string{{"verify"}, {"verify", "--json"}, {"verify", "--cierre"}} {
			dir := gprRepo(t, "gates:\n  test:\n    required: true\n    cmd: \"touch gpr-corrio\"\n"+
				"  mutation:\n    required: true\n    cmd: \"touch gpr-corrio\"\n"+c.linea)
			r := qcCorrer(t, bin, dir, qcSinCorrida, args...)
			if r.exit == 0 || !strings.Contains(r.todo(), "hoom.yaml invalido: "+c.want) {
				t.Fatalf("CA-809: %s %q: sale distinto de 0 con %q: exit %d\n%s", c.nombre, args, "hoom.yaml invalido: "+c.want, r.exit, r.todo())
			}
			if n := gprVeredictos(dir); n != 0 {
				t.Fatalf("CA-809: %s %q: no queda veredicto: %d", c.nombre, args, n)
			}
			if _, err := os.Stat(filepath.Join(dir, "gpr-corrio")); err == nil {
				t.Fatalf("CA-809: %s %q: ningun gate corre", c.nombre, args)
			}
		}
	}
}

// ---------------------------------------------------------------- CA-813

// CA-813 y aclaracion 1: `hoom verify --help` sale 0 e imprime en stdout el
// uso exacto, que nombra --cierre.
func TestCA813_VerifyHelpNombraCierre(t *testing.T) {
	bin := qcHoom(t)
	r := qcCorrer(t, bin, t.TempDir(), qcSinCorrida, "verify", "--help")
	if r.exit != 0 {
		t.Fatalf("CA-813: --help sale 0: exit %d\n%s", r.exit, r.todo())
	}
	for _, l := range strings.Split(gprUso, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		if !strings.Contains(r.stdout, l) {
			t.Fatalf("CA-813: a `hoom verify --help` le falta la linea %q:\n%s", l, r.stdout)
		}
	}
	if !strings.Contains(r.stdout, "--cierre") {
		t.Fatalf("CA-813: `hoom verify --help` nombra --cierre:\n%s", r.stdout)
	}
}

// ---------------------------------------------------------------- CA-814

// CA-814 (CAL-9.2) por el binario: item add y task start; el verify de
// trabajo con solo resources/js/app.js cambiado y commiteado; task done se
// niega con el texto de sin-cierre y la tarea sigue; check sale 0 y VERDE;
// verify --cierre, commit y task done cierra.
func TestCA814_TaskDoneYCheckPorElBinario(t *testing.T) {
	bin := qcHoom(t)
	yaml := "gates:\n  test:\n    required: true\n    cmd: \"true\"\n"
	for _, n := range []string{"static", "mutation"} {
		yaml += "  " + n + ":\n    required: true\n    cmd: \"true\"\n    diff_cmd: \"true\"\n    aplica_en: [\"app/**\"]\n"
	}
	root := gprRepo(t, yaml)
	if r := qcCorrer(t, bin, root, qcSinCorrida, "item", "add", "Precios", "--pedido", "que el catalogo muestre precios"); r.exit != 0 {
		t.Fatalf("CA-814: fixture: hoom item add: exit %d\n%s", r.exit, r.todo())
	}
	gprCommit(t, root, "item precios")
	if r := qcCorrer(t, bin, root, qcSinCorrida, "task", "start", "precios"); r.exit != 0 {
		t.Fatalf("CA-814: fixture: hoom task start: exit %d\n%s", r.exit, r.todo())
	}
	wt := filepath.Join(root, ".hoom", "worktrees", "precios")
	gprEscribir(t, wt, "resources/js/app.js", "console.log(2)\n")
	gprCommit(t, wt, "solo el js")
	if r := qcCorrer(t, bin, wt, qcSinCorrida, "verify"); r.exit != 0 {
		t.Fatalf("CA-814: el verify de trabajo es verde: exit %d\n%s", r.exit, r.todo())
	}
	gprCommit(t, wt, "veredicto de trabajo")

	want := fmt.Sprintf(htaskcmd.FormatoSinCierre, "precios", "static, mutation")
	r := qcCorrer(t, bin, root, qcSinCorrida, "task", "done", "precios")
	if r.exit == 0 || !strings.Contains(r.todo(), want) {
		t.Fatalf("CA-814: task done se niega con\n  %q\nexit %d\n%s", want, r.exit, r.todo())
	}
	if _, err := os.Stat(wt); err != nil {
		t.Fatalf("CA-814: task done no cierra: el worktree sigue: %v", err)
	}
	if r := qcCorrer(t, bin, wt, qcSinCorrida, "check"); r.exit != 0 || !strings.Contains(r.todo(), "VERDE") {
		t.Fatalf("CA-814: hoom check acepta no_aplica: exit 0 y VERDE: exit %d\n%s", r.exit, r.todo())
	}

	if r := qcCorrer(t, bin, wt, qcSinCorrida, "verify", "--cierre"); r.exit != 0 {
		t.Fatalf("CA-814: verify --cierre es verde: exit %d\n%s", r.exit, r.todo())
	}
	gprCommit(t, wt, "veredicto de cierre")
	if r := qcCorrer(t, bin, root, qcSinCorrida, "task", "done", "precios"); r.exit != 0 {
		t.Fatalf("CA-814: tras el verde de --cierre, task done cierra: exit %d\n%s", r.exit, r.todo())
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("CA-814: task done quito el worktree (err=%v)", err)
	}
}
