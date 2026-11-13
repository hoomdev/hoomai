// Tests de regresion de la ronda 6 de autonomia-entre-firmas sobre el gate
// del writer y el material de git de su tarea (aclaracion 25):
//
//   - hallazgos 20261113T040622_8e2d38, 88ed5f y d908ff (CA-638, CA-644): un
//     run de writer que cambia el directorio de hooks efectivo del worktree
//     de la tarea (el `hooks/` del directorio comun de git o el de
//     `core.hooksPath`), el `config` comun, el `config.worktree` del worktree
//     o el `info/attributes` comun corta su sobre como tocar un test
//     protegido: `scope`, `no-entregable`, exit 1, sin verify ni veredicto, y
//     un hallazgo high abierto del gate de scope (AutorGateDeScope) cuya
//     descripcion nombra la ruta tocada. Lo que diga agents.writer.write no
//     cuenta. Un run que lo cambia y lo deja como estaba no corta por esto.
//
// El material vive en el directorio comun de git del repo de prueba que arma
// aeaProyecto (no en el del proyecto hoom): cada ruta se pide a git desde el
// worktree de la tarea. El nombre de la regla y el texto de la nota no se
// fijan (aclaracion 25), asi que no se miran; la ruta se busca como
// subcadena de la descripcion del hallazgo. CLIs falsos (fakeProvider).
package agentcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
)

// ar6Ruta es `git rev-parse <flag> [rel]` desde el worktree, absoluta.
func ar6Ruta(t *testing.T, wt string, args ...string) string {
	t.Helper()
	out, err := ar1GitOut(t, wt, append([]string{"rev-parse"}, args...)...)
	if err != nil {
		t.Fatalf("fixture: git rev-parse %v en el worktree: %v", args, err)
	}
	p := strings.TrimSpace(out)
	if !filepath.IsAbs(p) {
		p = filepath.Join(wt, p)
	}
	return p
}

// ar6Escribir deja body en la ruta absoluta p con el modo dado.
func ar6Escribir(t *testing.T, p, body string, modo os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), modo); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, modo); err != nil {
		t.Fatal(err)
	}
}

// ar6DelGate son los hallazgos abiertos del gate de scope en los arboles.
func ar6DelGate(t *testing.T, dirs ...string) []finding.Item {
	t.Helper()
	var out []finding.Item
	for _, d := range dirs {
		items, _, err := finding.List(d, "main", true)
		if err != nil {
			continue
		}
		for _, it := range items {
			if it.Author == AutorGateDeScope && it.Status == finding.StatusOpen {
				out = append(out, it)
			}
		}
	}
	return out
}

// ar6Cortado exige el corte de la aclaracion 25 para un writer que toco el
// material de git en ruta (subcadena de la descripcion del hallazgo).
func ar6Cortado(t *testing.T, ca, caso, root, wt string, res Result, out string, antes int, ruta string) {
	t.Helper()
	if res.Stage != "scope" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 1 || !res.Scope.Cuts() {
		t.Fatalf("%s: %s: el sobre del writer cierra en scope, no-entregable, exit 1 (aclaracion 25): %+v\n%s", ca, caso, res, out)
	}
	if res.VerdictID != "" || res.Check != nil || aeaVeredictos(root, wt) != antes {
		t.Fatalf("%s: %s: sin verify ni veredicto nuevo: %+v (veredictos %d -> %d)", ca, caso, res, antes, aeaVeredictos(root, wt))
	}
	if r := aeaRegistro(t, root, res.EnvelopeID); r.Stage != "scope" || r.Status != envelope.StatusNotDeliverable {
		t.Fatalf("%s: %s: el registro dice lo mismo: %+v", ca, caso, r)
	}
	hs := ar6DelGate(t, wt, root)
	for _, h := range hs {
		if h.Severity == "high" && strings.Contains(h.Description, ruta) {
			return
		}
	}
	t.Fatalf("%s: %s: un hallazgo high abierto de %q cuya descripcion nombra %s: %+v", ca, caso, AutorGateDeScope, ruta, hs)
}

// Hallazgos 20261113T040622_8e2d38, 88ed5f y d908ff (CA-638, CA-644,
// aclaracion 25): un writer que cambia codigo y ademas cambia el material de
// git de su tarea corta su sobre en scope, no-entregable, exit 1, sin
// veredicto, con un hallazgo high abierto del gate de scope que nombra la
// ruta. Casos: crea hooks/pre-commit o hooks/reference-transaction en el
// directorio de hooks efectivo; le cambia el permiso de ejecucion a un hook
// existente (en los dos sentidos); borra un hook existente; cambia el
// destino de un hook symlink (los dos destinos con el mismo contenido);
// agrega una clave al config comun; crea el config.worktree del worktree;
// escribe info/attributes; y, con core.hooksPath dentro del worktree,
// cambia un hook de ahi aunque agents.writer.write lo permita.
func TestHallazgos8e2d38_88ed5f_d908ff_CA638_CA644_ElWriterNoCambiaElMaterialDeGit(t *testing.T) {
	const permiteTodo = "agents:\n  writer:\n    write:\n      allow: [\"**\"]\n"
	for _, c := range []struct {
		nombre, extra, script, ruta string
		antes                       func(t *testing.T, wt string)
	}{
		{nombre: "crea hooks/pre-commit", ruta: "hooks/pre-commit",
			script: "h=$(git rev-parse --git-path hooks)\nmkdir -p \"$h\"\nprintf '#!/bin/sh\\nexit 0\\n' > \"$h/pre-commit\"\nchmod +x \"$h/pre-commit\"\n"},
		{nombre: "crea hooks/reference-transaction", ruta: "hooks/reference-transaction",
			script: "h=$(git rev-parse --git-path hooks)\nmkdir -p \"$h\"\nprintf '#!/bin/sh\\ncat > /dev/null\\nexit 0\\n' > \"$h/reference-transaction\"\nchmod +x \"$h/reference-transaction\"\n"},
		{nombre: "le da permiso de ejecucion a un hook existente", ruta: "hooks/pre-push",
			antes: func(t *testing.T, wt string) {
				ar6Escribir(t, ar6Ruta(t, wt, "--git-path", "hooks/pre-push"), "#!/bin/sh\nexit 0\n", 0o644)
			},
			script: "chmod +x \"$(git rev-parse --git-path hooks/pre-push)\"\n"},
		{nombre: "le saca el permiso de ejecucion a un hook existente", ruta: "hooks/pre-push",
			antes: func(t *testing.T, wt string) {
				ar6Escribir(t, ar6Ruta(t, wt, "--git-path", "hooks/pre-push"), "#!/bin/sh\nexit 0\n", 0o755)
			},
			script: "chmod -x \"$(git rev-parse --git-path hooks/pre-push)\"\n"},
		{nombre: "borra un hook existente", ruta: "hooks/pre-push",
			antes: func(t *testing.T, wt string) {
				ar6Escribir(t, ar6Ruta(t, wt, "--git-path", "hooks/pre-push"), "#!/bin/sh\nexit 0\n", 0o755)
			},
			script: "rm \"$(git rev-parse --git-path hooks/pre-push)\"\n"},
		{nombre: "cambia el destino de un hook symlink", ruta: "hooks/post-merge",
			antes: func(t *testing.T, wt string) {
				d := t.TempDir() // los dos destinos, fuera del repo
				ar6Escribir(t, filepath.Join(d, "uno"), "#!/bin/sh\nexit 0\n", 0o755)
				ar6Escribir(t, filepath.Join(d, "otro"), "#!/bin/sh\nexit 0\n", 0o755)
				h := ar6Ruta(t, wt, "--git-path", "hooks/post-merge")
				if err := os.MkdirAll(filepath.Dir(h), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(d, "uno"), h); err != nil {
					t.Fatal(err)
				}
			},
			script: "h=$(git rev-parse --git-path hooks/post-merge)\nd=$(dirname \"$(readlink \"$h\")\")\nln -sfn \"$d/otro\" \"$h\"\n"},
		{nombre: "agrega una clave al config comun", ruta: "config",
			script: "git config hoom.colado si\n"},
		{nombre: "crea el config.worktree del worktree", ruta: "config.worktree",
			script: "printf '[core]\\n\\tfoo = bar\\n' > \"$(git rev-parse --git-dir)/config.worktree\"\n"},
		{nombre: "escribe info/attributes", ruta: "info/attributes",
			script: "a=$(git rev-parse --git-path info/attributes)\nmkdir -p \"$(dirname \"$a\")\"\nprintf '*.go diff\\n' > \"$a\"\n"},
		{nombre: "con core.hooksPath dentro del worktree y write.allow **, cambia un hook de ahi", ruta: "githooks/pre-commit",
			extra: permiteTodo,
			antes: func(t *testing.T, wt string) {
				ar6Escribir(t, filepath.Join(wt, "githooks", "pre-commit"), "#!/bin/sh\nexit 0\n", 0o755)
				git(t, wt, "add", "githooks")
				git(t, wt, "commit", "-q", "-m", "hooks del repo")
				git(t, wt, "config", "core.hooksPath", filepath.Join(wt, "githooks"))
				if h := ar6Ruta(t, wt, "--git-path", "hooks"); h != filepath.Join(wt, "githooks") {
					t.Fatalf("fixture: el directorio de hooks efectivo es %s/githooks: %s", wt, h)
				}
			},
			script: "printf '#!/bin/sh\\necho colado\\nexit 0\\n' > githooks/pre-commit\n"},
	} {
		ca := "8e2d38/88ed5f/d908ff/CA-638/CA-644"
		root, wt := aeaProyecto(t, "true", c.extra)
		if c.antes != nil {
			c.antes(t, wt)
		}
		antes := aeaVeredictos(root, wt)
		res, out := aeaCorrer(t, root, "writer", ar1Codigo+c.script+"exit 0\n", Options{})
		if raw, _ := os.ReadFile(filepath.Join(wt, "app.go")); !strings.Contains(string(raw), "implementado") {
			t.Fatalf("%s: %s: fixture: el writer cambio codigo: %q", ca, c.nombre, raw)
		}
		ar6Cortado(t, ca, c.nombre, root, wt, res, out, antes, c.ruta)
	}
}

// Hallazgos 20261113T040622_8e2d38, 88ed5f y d908ff (CA-638, aclaracion 25),
// control: un writer que cambia codigo y, en el mismo run, crea y borra un
// hook, cambia el config comun y lo restaura, y crea y borra el
// config.worktree e info/attributes, deja el material como estaba: su
// sobre no corta por esto, verify corre y no queda ningun hallazgo abierto
// del gate de scope.
func TestHallazgos8e2d38_88ed5f_d908ff_CA638_CambiarYRestaurarNoCorta(t *testing.T) {
	const ca = "8e2d38/88ed5f/d908ff/CA-638 (control)"
	root, wt := aeaProyecto(t, "true", "")
	if _, err := os.Stat(ar6Ruta(t, wt, "--git-path", "info/attributes")); err == nil {
		t.Fatalf("%s: fixture: el repo de prueba no tiene info/attributes", ca)
	}
	script := ar1Codigo +
		"h=$(git rev-parse --git-path hooks)\nmkdir -p \"$h\"\nprintf '#!/bin/sh\\nexit 0\\n' > \"$h/pre-commit\"\nchmod +x \"$h/pre-commit\"\nrm \"$h/pre-commit\"\n" +
		"c=$(git rev-parse --git-common-dir)/config\ncp -p \"$c\" \"$c.hoom-copia\"\ngit config hoom.temporal si\nmv \"$c.hoom-copia\" \"$c\"\n" +
		"w=$(git rev-parse --git-dir)/config.worktree\nprintf '[core]\\n' > \"$w\"\nrm \"$w\"\n" +
		"a=$(git rev-parse --git-path info/attributes)\nmkdir -p \"$(dirname \"$a\")\"\nprintf '*.go diff\\n' > \"$a\"\nrm \"$a\"\n" +
		"exit 0\n"
	res, out := aeaCorrer(t, root, "writer", script, Options{})
	if v, err := ar1GitOut(t, wt, "config", "--get", "hoom.temporal"); err == nil {
		t.Fatalf("%s: fixture: el writer restauro el config comun: hoom.temporal=%q", ca, v)
	}
	if res.Scope.Cuts() || res.Stage == "scope" || res.VerdictID == "" {
		t.Fatalf("%s: cambiar el material de git y dejarlo como estaba no corta: el sobre no corta y verify corre: %+v\n%s", ca, res, out)
	}
	if hs := ar6DelGate(t, wt, root); len(hs) != 0 {
		t.Fatalf("%s: ningun hallazgo abierto del gate de scope: %+v", ca, hs)
	}
}
