// Tests adversariales del spec .hoom/specs/evidencia-en-disco.md dentro de
// 'hoom review' (CA-443, y CA-441/CA-442 en la corrida del reviewer): la
// evidencia se mide en el disco, no en lo que git lista. Un reviewer que
// crea .hoom/findings/.gitignore (esconda algo o no todavia), o que edita o
// borra un hallazgo que git ignora, termina NO ENTREGABLE por territorio, sin
// registro de review, con el hallazgo high del gate. Registrar un hallazgo
// nuevo, o reescribir uno igual, sigue siendo revisado.
//
// Providers falsos en el PATH; las reviews corren con reloj.
package reviewcmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/agentcmd"
)

// Frases del contrato.
const (
	edAppendOnly  = "la evidencia es append-only"
	edCambio      = "cambio durante el run"
	edDesaparecio = "desaparecio durante el run"
	edFormaFind   = "bajo .hoom/findings solo se crean archivos con la forma que escribe hoom"
)

// edExclude devuelve la ruta absoluta del .git/info/exclude de root.
func edExclude(t *testing.T, root string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--git-path", "info/exclude")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse --git-path info/exclude: %v", err)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return p
}

// edAnexar agrega una linea al final de un archivo (lo crea si falta).
func edAnexar(t *testing.T, path, linea string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("\n" + linea + "\n"); err != nil {
		t.Fatal(err)
	}
}

// edPreparado deja contenido fuera del arbol para que el reviewer falso lo
// copie con cat.
func edPreparado(t *testing.T, contenido []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "preparado")
	if err := os.WriteFile(p, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// edBajarSeveridad devuelve el hallazgo con su severidad high bajada a low.
func edBajarSeveridad(t *testing.T, raw []byte) []byte {
	t.Helper()
	out := regexp.MustCompile(`"severity"(\s*):(\s*)"high"`).ReplaceAll(raw, []byte(`"severity"${1}:${2}"low"`))
	if bytes.Equal(out, raw) {
		t.Fatalf("fixture: el hallazgo era high: %s", raw)
	}
	return out
}

func edLista(vs []agentcmd.Violation) string {
	var b strings.Builder
	b.WriteString("[")
	for i, v := range vs {
		if i > 0 {
			b.WriteString(" | ")
		}
		b.WriteString(v.Path + " (" + v.Rule + "): " + v.Detail)
	}
	b.WriteString("]")
	return b.String()
}

// edRevisar corre una pasada de risk con reloj.
func edRevisar(t *testing.T, caso, root string) (Result, string) {
	t.Helper()
	res, err, out := raRunConReloj(t, "CA-443", qcr1Reloj, root, Options{Lens: "risk", Provider: "codex"})
	if err != nil {
		t.Fatalf("CA-443: %s: la review no se rompe: %v\n%s", caso, err, out)
	}
	if len(res.Passes) != 1 {
		t.Fatalf("CA-443: %s: fixture: una pasada de risk: %+v\n%s", caso, res, out)
	}
	return res, out
}

// edNoEntregable exige la review NO ENTREGABLE por territorio: UNA
// manipulacion en ruta con cada parte en su detalle, el territorio roto, su
// hallazgo high del gate registrado y abierto, exit 1 y sin registro.
func edNoEntregable(t *testing.T, caso, root string, res Result, out, ruta string, partes ...string) {
	t.Helper()
	p := res.Passes[0]
	var ms []agentcmd.Violation
	for _, v := range p.Scope.Violations {
		if v.Path == ruta && v.Rule == agentcmd.RuleTampering {
			ms = append(ms, v)
		}
	}
	if len(ms) != 1 {
		t.Fatalf("CA-443: %s: %s es UNA violacion de manipulacion en la pasada del reviewer (hay %d): %s\nstatus=%s exit=%d\n%s",
			caso, ruta, len(ms), edLista(p.Scope.Violations), res.Status, res.ExitCode, out)
	}
	for _, parte := range partes {
		if !strings.Contains(ms[0].Detail, parte) {
			t.Fatalf("CA-443: %s: el detalle en %s dice %q: %q", caso, ruta, parte, ms[0].Detail)
		}
	}
	if p.Scope.OK || !p.Scope.Tampering {
		t.Fatalf("CA-443: %s: el territorio del reviewer queda roto por manipulacion: %+v", caso, p.Scope)
	}
	if _, porque := qcr1HallazgoDelGate(root, p.Scope, ruta); porque != "" {
		t.Fatalf("CA-443: %s: %s", caso, porque)
	}
	if res.Status != "no-entregable" || res.ExitCode != 1 {
		t.Fatalf("CA-443: %s: la review termina NO ENTREGABLE por territorio, exit 1: %s, exit %d\n%s", caso, res.Status, res.ExitCode, out)
	}
	if res.RecordID != "" || rdRegistrosEn(root) != 0 {
		t.Fatalf("CA-443: %s: una review no entregable no deja registro: %q, %d", caso, res.RecordID, rdRegistrosEn(root))
	}
}

// CA-443 (y CA-442 en la corrida del reviewer): el reviewer que crea
// .hoom/findings/.gitignore — con *.json, con nada que esconda todavia, o
// escondido el mismo por info/exclude — termina NO ENTREGABLE por
// territorio, sin registro, con "bajo .hoom/findings solo se crean archivos
// con la forma que escribe hoom".
func TestCA443_ElReviewerQueCreaUnGitignoreEnFindingsEsNoEntregable(t *testing.T) {
	const ruta = ".hoom/findings/.gitignore"
	for _, c := range []struct {
		caso, contenido string
		previo          bool // .hoom/findings/ ya en info/exclude
	}{
		{"esconde los json", `*.json\n`, false},
		{"todavia no esconde nada", `# nada\n`, false},
		{"escondido el mismo por info/exclude", `*.json\n`, true},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, _ := qcRepoConHallazgo(t)
			if c.previo {
				edAnexar(t, edExclude(t, root), ".hoom/findings/")
			}
			fakeProvider(t, "codex", "mkdir -p .hoom/findings\nprintf '"+c.contenido+"' > "+ruta+"\nexit 0\n")

			res, out := edRevisar(t, c.caso, root)
			if _, err := os.Stat(filepath.Join(root, ruta)); err != nil {
				t.Fatalf("CA-443: fixture: el reviewer falso creo %s: %v", ruta, err)
			}
			if c.previo && !qcr1Ignorado(root, ruta) {
				t.Fatalf("CA-443: fixture: git ignora %s", ruta)
			}
			edNoEntregable(t, c.caso, root, res, out, ruta, edFormaFind, ruta)
		})
	}
}

// CA-443 (y CA-441 en la corrida del reviewer): el reviewer que baja a low
// el hallazgo high abierto que git ignora (por info/exclude, por un
// .hoom/findings/.gitignore previo, o por el .gitignore de la raiz), o que lo
// borra, termina NO ENTREGABLE por territorio, sin registro, con "la
// evidencia es append-only" y "cambio/desaparecio durante el run".
func TestCA443_ElReviewerQueEditaOBorraUnHallazgoIgnoradoEsNoEntregable(t *testing.T) {
	for _, c := range []struct {
		caso    string
		ocultar func(t *testing.T, root string)
		borrar  bool
	}{
		{"baja la severidad, info/exclude", func(t *testing.T, root string) {
			edAnexar(t, edExclude(t, root), ".hoom/findings/")
		}, false},
		{"baja la severidad, .hoom/findings/.gitignore previo", func(t *testing.T, root string) {
			write(t, root, ".hoom/findings/.gitignore", "*.json\n")
		}, false},
		{"baja la severidad, .gitignore de la raiz", func(t *testing.T, root string) {
			write(t, root, ".gitignore", ".hoom/findings/\n")
			git(t, root, "add", ".gitignore")
			git(t, root, "commit", "-q", "-m", "la raiz ignora los hallazgos")
		}, false},
		{"lo borra, info/exclude", func(t *testing.T, root string) {
			edAnexar(t, edExclude(t, root), ".hoom/findings/")
		}, true},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root, id := qcRepoConHallazgo(t)
			rel := ".hoom/findings/" + id + ".json"
			c.ocultar(t, root)
			if !qcr1Ignorado(root, rel) {
				t.Fatalf("CA-443: fixture: git ignora %s", rel)
			}
			raw, err := os.ReadFile(filepath.Join(root, rel))
			if err != nil {
				t.Fatal(err)
			}
			accion := "cat " + qcComillas(edPreparado(t, edBajarSeveridad(t, raw))) + " > " + qcComillas(rel) + "\n"
			frase := edCambio
			if c.borrar {
				accion, frase = "rm -f "+qcComillas(rel)+"\n", edDesaparecio
			}
			fakeProvider(t, "codex", accion+"exit 0\n")

			res, out := edRevisar(t, c.caso, root)
			despues, err := os.ReadFile(filepath.Join(root, rel))
			if c.borrar != (err != nil) || !c.borrar && bytes.Contains(despues, []byte(`"high"`)) {
				t.Fatalf("CA-443: fixture: el reviewer falso edito o borro %s: %v", rel, err)
			}
			edNoEntregable(t, c.caso, root, res, out, rel, edAppendOnly, rel, frase)
		})
	}
}

// CA-443 (controles): mirar el disco no convierte en violacion el trabajo
// del reviewer. Con los hallazgos escondidos de git por info/exclude:
// registrar un hallazgo nuevo con la forma de hoom, o reescribir con los
// mismos bytes el hallazgo previo, termina revisado, sin violaciones.
func TestCA443_ElReviewerQueRegistraOReescribeIgualSigueRevisado(t *testing.T) {
	t.Run("registra un hallazgo nuevo", func(t *testing.T) {
		root, _ := qcRepoConHallazgo(t)
		edAnexar(t, edExclude(t, root), ".hoom/findings/")
		nuevo := "20261001T120000_c0ffee"
		fakeProvider(t, "codex", hallazgoFalso(root, nuevo, "reviewer@codex")+"exit 0\n")

		res, out := edRevisar(t, "registra", root)
		if !qcr1Ignorado(root, ".hoom/findings/"+nuevo+".json") {
			t.Fatalf("CA-443: fixture: git ignora el hallazgo nuevo")
		}
		p := res.Passes[0]
		if len(p.Scope.Violations) != 0 || !p.Scope.OK || p.Scope.Tampering {
			t.Fatalf("CA-443: un hallazgo nuevo con la forma de hoom no es violacion, ignorado o no: %s\n%s", edLista(p.Scope.Violations), out)
		}
		if res.Status != "revisado" || res.ExitCode != 0 {
			t.Fatalf("CA-443: la review termina revisada: %s, exit %d\n%s", res.Status, res.ExitCode, out)
		}
	})

	t.Run("reescribe igual el hallazgo ignorado", func(t *testing.T) {
		root, id := qcRepoConHallazgo(t)
		edAnexar(t, edExclude(t, root), ".hoom/findings/")
		rel := ".hoom/findings/" + id + ".json"
		raw, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		fakeProvider(t, "codex", "cat "+qcComillas(edPreparado(t, raw))+" > "+qcComillas(rel)+"\ntouch "+qcComillas(rel)+"\nexit 0\n")

		res, out := edRevisar(t, "reescribe igual", root)
		p := res.Passes[0]
		if len(p.Scope.Violations) != 0 || !p.Scope.OK || p.Scope.Tampering {
			t.Fatalf("CA-443: reescribir con el mismo contenido no es cambio: %s\n%s", edLista(p.Scope.Violations), out)
		}
		if res.Status != "revisado" || res.ExitCode != 0 {
			t.Fatalf("CA-443: la review termina revisada: %s, exit %d\n%s", res.Status, res.ExitCode, out)
		}
	})
}
