// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md
// (CA-481..CA-487, enmienda 1) sobre `hoom review`:
//
//   - la evidencia es todo el repositorio menos el estado de hoom (una lista
//     cerrada: .hoom/approvals/*.json, .hoom/findings/*.json,
//     .hoom/reviews/*.json, .hoom/verdicts/*.json, .hoom/items/*.yaml,
//     .hoom/specs/*.md, .hoom/intake/ entero, .hoom/ratchet.json,
//     .hoom/.gitignore y los locales), y el arbol sucio se mide sobre esas
//     mismas rutas;
//   - la review con rango compara contra la punta anterior CON LA BASE
//     INTEGRADA (un merge sintetico): lo que trajo un merge de la base no es
//     parte del rango, y si ese merge no se separa (conflicto, git anterior a
//     2.40) la review es la completa de hoy, con un aviso.
//
// Este archivo tiene los fixtures; el git aislado del binario (lo que
// TestMain le saca y le fija al entorno) es gittest.AislarGit, una sola
// politica para gitx, reviewcmd y boardcmd (hallazgos 20261029T172636_8f2253
// y 20261029T172811_c4957f). Los tests estan en
// evidencia_de_la_review_{evidencia,delta,hallazgos*}_test.go y, los que
// necesitan un git falso al frente del PATH, en
// evidencia_de_la_review_unix_test.go. Reusa los de
// review_aislada_helpers_test.go y review_por_diferencia_helpers_test.go
// (CLIs falsos en un PATH minimo, la base main y la rama feature, rdCommit,
// rdSha, ...).
package reviewcmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// edrSpec es el spec de la tarea x de los fixtures de delta (rdDeltaRepo).
const edrSpec = ".hoom/specs/x.md"

// edrGitConEntorno corre git en dir con extra agregado al entorno de este
// proceso (que TestMain ya aislo) y devuelve su stdout sin espacios
// alrededor: git puede avisar por stderr (que info/grafts esta en desuso,
// por ejemplo). Es para las guardas que leen lo plantado con una variable
// que hoom va a heredar (GIT_REPLACE_REF_BASE, GIT_GRAFT_FILE).
func edrGitConEntorno(t *testing.T, dir string, extra []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, append(os.Environ(), extra...)
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("fixture: git %v en %s con %q: %v\n%s", args, dir, extra, err, stderr)
	}
	return strings.TrimSpace(string(out))
}

// edrEsEstado es el oraculo de la tabla del spec: si ruta es estado de hoom.
// *.json, *.yaml y *.md son archivos DIRECTOS de su directorio; .hoom/intake/
// y los locales van enteros.
func edrEsEstado(ruta string) bool {
	r, ok := strings.CutPrefix(ruta, ".hoom/")
	if !ok {
		return false
	}
	if r == "ratchet.json" || r == ".gitignore" {
		return true
	}
	dir, resto, ok := strings.Cut(r, "/")
	if !ok {
		return false
	}
	switch dir {
	case "intake", "cache", "worktrees", "runs", "isolated", "envelopes":
		return true
	}
	if strings.Contains(resto, "/") {
		return false
	}
	switch dir {
	case "approvals", "findings", "reviews", "verdicts":
		return strings.HasSuffix(resto, ".json")
	case "items":
		return strings.HasSuffix(resto, ".yaml")
	case "specs":
		return strings.HasSuffix(resto, ".md")
	}
	return false
}

// edrSucio lista lo que git ve sin commitear en dir fuera del estado de hoom
// (sin lo ignorado), una entrada 'XY ruta' por archivo.
func edrSucio(t *testing.T, dir string) []string {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain=v1", "-z", "-uall", "--no-renames")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("fixture: git status en %s: %v", dir, err)
	}
	var lista []string
	for _, e := range strings.Split(string(out), "\x00") {
		if len(e) < 4 || edrEsEstado(e[3:]) {
			continue
		}
		lista = append(lista, e)
	}
	return lista
}

// edrLimpio (fixture): el arbol de dir no tiene nada sin commitear fuera del
// estado de hoom, asi que la review no tiene por que negarse por arbol sucio.
func edrLimpio(t *testing.T, ca, dir string) {
	t.Helper()
	if s := edrSucio(t, dir); len(s) != 0 {
		t.Fatalf("%s: fixture: el arbol no tiene cambios sin commitear fuera del estado de hoom: %q", ca, s)
	}
}

// edrRevisar corre la review (con reloj) con el arbol limpio y exige que no
// se rompa.
func edrRevisar(t *testing.T, ca, root string, opt Options) (Result, string) {
	t.Helper()
	edrLimpio(t, ca, root)
	res, err, out := rdCorrer(t, ca, root, opt)
	if err != nil {
		t.Fatalf("%s: la review no se rompe: %v\n%s", ca, err, out)
	}
	return res, out
}

// edrJSON es v como lo escribe --json: su JSON, leido como mapa (sin
// depender de los campos que el tipo tenga hoy).
func edrJSON(t *testing.T, v any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// edrSinClave exige que el JSON no traiga clave (o la traiga vacia).
func edrSinClave(t *testing.T, ca, donde string, m map[string]any, clave string) {
	t.Helper()
	if v, ok := m[clave]; ok && v != "" {
		t.Fatalf("%s: %s no trae %q: %v", ca, donde, clave, v)
	}
}

// edrMainAvanza commitea en main los archivos (ruta -> contenido) y vuelve a
// la rama feature. Devuelve el sha de main.
func edrMainAvanza(t *testing.T, root, msg string, archivos map[string]string) string {
	t.Helper()
	git(t, root, "checkout", "-q", "main")
	var rutas []string
	for ruta, cuerpo := range archivos {
		write(t, root, ruta, cuerpo)
		rutas = append(rutas, ruta)
	}
	git(t, root, append([]string{"add", "--"}, rutas...)...)
	git(t, root, append([]string{"commit", "-q", "-m", msg, "--"}, rutas...)...)
	sha := rdSha(t, root, "HEAD")
	git(t, root, "checkout", "-q", "feature")
	return sha
}

// edrIntegrar integra main en la rama con un merge real, sin conflicto, y
// con extra (ruta -> contenido) escrito en el mismo commit de merge (un "evil
// merge"; nil = el merge limpio). Devuelve el sha del merge.
func edrIntegrar(t *testing.T, root string, extra map[string]string) string {
	t.Helper()
	git(t, root, "merge", "-q", "--no-ff", "--no-commit", "main")
	for ruta, cuerpo := range extra {
		write(t, root, ruta, cuerpo)
		git(t, root, "add", "--", ruta)
	}
	git(t, root, "commit", "-q", "--no-edit")
	return rdSha(t, root, "HEAD")
}

// edrIntegrarConConflicto integra main en la rama con un merge real que
// tiene conflicto, y lo resuelve con resolucion (ruta -> contenido).
// Devuelve el sha del merge.
func edrIntegrarConConflicto(t *testing.T, root string, resolucion map[string]string) string {
	t.Helper()
	cmd := exec.Command("git", "merge", "-q", "--no-ff", "--no-commit", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("fixture: el merge de main tiene conflicto, y salio sin error:\n%s", out)
	}
	for ruta, cuerpo := range resolucion {
		write(t, root, ruta, cuerpo)
		git(t, root, "add", "--", ruta)
	}
	git(t, root, "commit", "-q", "--no-edit")
	return rdSha(t, root, "HEAD")
}

// edrConReview arma rdDeltaRepo (la rama feature con el spec x y el commit
// A), corre la review completa de la tarea x con el codex cx y deja pasar un
// segundo. Devuelve el repo, el registro de la review y su hasta (D).
func edrConReview(t *testing.T, ca string, cx *raCLI) (root string, r1 Result, d string) {
	t.Helper()
	return edrConReviewTras(t, ca, cx, nil)
}

// edrConReviewTras es edrConReview con un commit mas de la rama (archivos,
// ruta -> contenido) antes de la review: lo que la review ya cubre.
func edrConReviewTras(t *testing.T, ca string, cx *raCLI, archivos map[string]string) (root string, r1 Result, d string) {
	t.Helper()
	root = rdDeltaRepo(t)
	if archivos != nil {
		rdCommit(t, root, "la rama antes de la review", archivos)
	}
	antes := cx.veces()
	r1, out := edrRevisar(t, ca, root, Options{Provider: "codex", Spec: edrSpec})
	if r1.Status != "revisado" || r1.RecordID == "" || r1.Cobertura != CoberturaCompleta || cx.veces() <= antes {
		t.Fatalf("%s: fixture: la primera review (sin rango) revisa, es completa y deja registro: %+v\n%s", ca, r1, out)
	}
	d = rdSha(t, root, "HEAD")
	if r1.Hasta != d {
		t.Fatalf("%s: fixture: la primera review llega a HEAD (%s): %+v", ca, d[:12], r1)
	}
	rdEsperarSegundo()
	return root, r1, d
}

// edrLineaBase es la linea de la salida, despues de la linea rango, cuando
// se desconto la base.
func edrLineaBase(i, d string) string {
	return "  base        integrada hasta " + i[:12] + " despues de " + d[:12] + ": lo que trajo no es parte del rango"
}

// edrLineaPedido es la linea del pedido, despues de la linea Rango:, cuando
// se desconto la base.
func edrLineaPedido(i, d string) string {
	return "La rama integro la base hasta " + i[:12] + " despues de " + d[:12] + ": el diff es contra " + d[:12] +
		" con esa base integrada, asi que lo que trajo la base no esta en la evidencia y no es INTRODUCIDO."
}

// edrAviso es el aviso cuando el merge de la base no se separa (sin el
// prefijo '  aviso: ' de la salida).
func edrAviso(i, d, motivo string) string {
	return "la rama integro la base (" + i[:12] + ") despues de " + d[:12] + " y ese merge no se separa (" + motivo + "): la review es completa"
}

func edrMotivoConflicto(ruta string) string { return "conflicto en " + ruta }

func edrMotivoGitViejo(version string) string {
	return "git " + version + " no arma el merge sintetico, hace falta 2.40"
}

// edrDiffDelPedido es la evidencia que recibio una pasada: lo que va entre
// '=== diff <sha256> ===' y '=== fin de la evidencia <sha256> ===', y ese
// sha256.
func edrDiffDelPedido(t *testing.T, ca, p string) (diff, sha string) {
	t.Helper()
	m := regexp.MustCompile(`(?m)^=== diff ([0-9a-f]{64}) ===$`).FindStringSubmatchIndex(p)
	if m == nil {
		t.Fatalf("%s: el pedido trae la evidencia entre sus marcadores:\n%s", ca, p)
	}
	sha = p[m[2]:m[3]]
	resto := p[m[1]+1:]
	fin := "=== fin de la evidencia " + sha + " ==="
	j := strings.Index(resto, fin)
	if j < 0 {
		t.Fatalf("%s: el pedido cierra la evidencia con %q:\n%s", ca, fin, p)
	}
	return resto[:j], sha
}

// edrCabeceras son las rutas de las secciones 'diff --git a/<r> b/<r>' de un
// parche, en orden.
func edrCabeceras(diff string) []string {
	var rutas []string
	for _, l := range strings.Split(diff, "\n") {
		if r, ok := strings.CutPrefix(l, "diff --git a/"); ok {
			if i := strings.Index(r, " b/"); i >= 0 {
				rutas = append(rutas, r[:i])
			}
		}
	}
	return rutas
}

// edrAgregadas son las lineas agregadas ('+', sin la cabecera '+++') de la
// seccion de ruta en el parche.
func edrAgregadas(diff, ruta string) []string {
	sec := raSeccion(diff, "diff --git a/"+ruta+" b/"+ruta)
	var out []string
	for _, l := range strings.Split(sec, "\n") {
		if strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++") {
			out = append(out, l)
		}
	}
	return out
}

// edrLineasDe cuenta las lineas de un contenido que termina en salto de
// linea.
func edrLineasDe(s string) int { return strings.Count(s, "\n") }

// edrCodigo es un archivo Go chico con su marca.
func edrCodigo(marca string) string {
	return "package app\n\n// " + marca + "\nfunc F" + strings.ReplaceAll(marca, "-", "") + "() {}\n"
}
