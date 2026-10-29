//go:build !windows

// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md
// (CA-486, CA-487) que necesitan un git falso al frente del PATH (un script
// sh que hace exec del git de verdad, con su ruta absoluta resuelta antes):
//
//   - CA-486: "un git que se anuncia anterior a 2.40 da lo mismo [la review
//     completa] con el motivo `git <version> no arma el merge sintetico, hace
//     falta 2.40`". El git falso responde `git version 2.39.0`.
//   - CA-487: "una falla de git en el merge sintetico que no es un conflicto
//     da un error, sin pasadas ni registro". El git falso falla en
//     `git merge-tree` (con la que se arma el merge sintetico) con un fatal,
//     exit 128: no es el exit 1 de un conflicto.
//
// Los fixtures estan en evidencia_de_la_review_helpers_test.go y
// evidencia_de_la_review_delta_test.go.
package reviewcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// edrSaltarOpciones es el comienzo del script de un git falso: recorre los
// argumentos salteando las opciones globales de git (y el valor de las que
// llevan uno aparte) hasta el subcomando, y corre casos sobre el.
func edrSaltarOpciones(casos string) string {
	return "saltar=0\nfor a in \"$@\"; do\n" +
		"\tif [ \"$saltar\" = 1 ]; then saltar=0; continue; fi\n" +
		"\tcase \"$a\" in\n" +
		"\t-c|-C|--git-dir|--work-tree|--namespace|--config-env) saltar=1 ;;\n" +
		casos +
		"\t-*) ;;\n" +
		"\t*) break ;;\n" +
		"\tesac\ndone\n"
}

// edrGitViejo pone en bin un git que se anuncia 2.39.0 (a `git version` y a
// `git --version`) y para todo lo demas es el git de verdad (real, ruta
// absoluta).
func edrGitViejo(t *testing.T, bin, real string) {
	t.Helper()
	s := "#!/bin/sh\n" + edrSaltarOpciones("\t--version|version) echo 'git version 2.39.0'; exit 0 ;;\n") +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("git", "version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "git version 2.39.0" {
		t.Fatalf("fixture: el git del PATH se anuncia 2.39.0: %q %v", out, err)
	}
}

// edrGitSinMergeTree pone en bin un git que falla en `git merge-tree` (deja
// una marca, escribe un fatal y sale con 128) y para todo lo demas es el git
// de verdad. Devuelve la marca.
func edrGitSinMergeTree(t *testing.T, bin, real string) (marca string) {
	t.Helper()
	marca = filepath.Join(t.TempDir(), "merge-tree-fallo")
	s := "#!/bin/sh\n" + edrSaltarOpciones("\tmerge-tree) echo x >> '"+marca+"'; echo 'fatal: FALLA-SIMULADA-EN-MERGE-TREE' >&2; exit 128 ;;\n") +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return marca
}

// CA-486: con un git que se anuncia 2.39.0, una rama que integro la base
// sin conflicto despues de la review no se separa: `hoom review --delta` (y
// --desde D) hace la review completa, con el aviso `la rama integro la base
// (<I12>) despues de <D12> y ese merge no se separa (git 2.39.0 no arma el
// merge sintetico, hace falta 2.40): la review es completa`. Tambien cuando
// la rama solo integro la base: no termina SIN REVISAR.
func TestCA486_GitAnteriorA240NoSeparaYEsLaReviewCompleta(t *testing.T) {
	casos := []struct {
		nombre string
		armar  func(t *testing.T, cx *raCLI) (root, d, i, h string)
		opt    Options
	}{
		{"x-merge-limpio-z", func(t *testing.T, cx *raCLI) (string, string, string, string) {
			root, _, d, i, h := edrConBaseIntegrada(t, "CA-486", cx)
			return root, d, i, h
		}, Options{Provider: "codex", Spec: edrSpec, Delta: true}},
		{"x-merge-limpio-z-con-desde", func(t *testing.T, cx *raCLI) (string, string, string, string) {
			root, _, d, i, h := edrConBaseIntegrada(t, "CA-486", cx)
			return root, d, i, h
		}, Options{Provider: "codex", Spec: edrSpec, Desde: "D"}},
		{"solo-un-merge-limpio", func(t *testing.T, cx *raCLI) (string, string, string, string) {
			root, _, d := edrConReview(t, "CA-486", cx)
			i := edrMainAvanza(t, root, "Y", map[string]string{"y.go": edrCodigo("MARCA-Y")})
			h := edrIntegrar(t, root, nil)
			return root, d, i, h
		}, Options{Provider: "codex", Spec: edrSpec, Delta: true}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			real := raGitReal(t)
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, d, i, h := c.armar(t, cx)
			opt := c.opt
			if opt.Desde != "" {
				opt.Desde = d
			}
			edrGitViejo(t, bin, real)
			antes := cx.veces()
			res, out := edrRevisar(t, "CA-486", root, opt)
			edrCompleta(t, "CA-486", c.nombre, root, cx, antes, res, out, d, i, h, edrMotivoGitViejo("2.39.0"))
		})
	}
}

// CA-487: "una falla de git en el merge sintetico que no es un conflicto da
// un error, sin pasadas ni registro", con --delta y con --desde D: nunca la
// review del diff crudo, ni la completa del aviso, ni SIN REVISAR.
func TestCA487_UnaFallaDeGitEnElMergeSinteticoEsUnError(t *testing.T) {
	for _, delta := range []bool{true, false} {
		caso := "--desde"
		if delta {
			caso = "--delta"
		}
		t.Run(caso, func(t *testing.T) {
			real := raGitReal(t)
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			root, _, d, _, _ := edrConBaseIntegrada(t, "CA-487", cx)
			opt := Options{Provider: "codex", Spec: edrSpec, Delta: delta}
			if !delta {
				opt.Desde = d
			}
			marca := edrGitSinMergeTree(t, bin, real)
			edrLimpio(t, "CA-487", root)
			antes, registros := cx.veces(), rdRegistrosEn(root)
			res, err, out := rdCorrer(t, "CA-487", root, opt)
			msg := rdNegada(t, "CA-487", caso, root, cx, antes, registros, res, err, out, "")
			if raDiceSinRevisar(msg) || strings.Contains(msg, "no se separa") {
				t.Fatalf("CA-487: %s: una falla de git no es SIN REVISAR ni la review completa del aviso:\n%s", caso, msg)
			}
			if _, err := os.Stat(marca); err != nil {
				t.Fatalf("CA-487: %s: el error es el de git en el merge sintetico (git merge-tree no corrio): %v\n%s", caso, err, msg)
			}
		})
	}
}

// CA-482 / CA-487 (el orden de la review con rango, extendido: "guarda de
// arbol sucio -> hoom.yaml -> base -> --desde/--delta -> separacion ->
// medida -> lentes"). Con el merge sintetico fallando (git merge-tree con un
// fatal): un .hoom/notas.txt sin rastrear da la negativa de arbol sucio, y un
// --desde que no es ancestro de HEAD da su error; en los dos casos sin
// llegar a armar el merge sintetico.
func TestCA487_LaSeparacionVaDespuesDeLaGuardaYDeDesde(t *testing.T) {
	real := raGitReal(t)
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")
	root, _, _, _, _ := edrConBaseIntegrada(t, "CA-487", cx)
	otra := edrMainAvanza(t, root, "main sigue", map[string]string{"main2.go": edrCodigo("MARCA-MAIN-2")})
	marca := edrGitSinMergeTree(t, bin, real)
	antes, registros := cx.veces(), rdRegistrosEn(root)

	write(t, root, ".hoom/notas.txt", "NOTAS-SIN-COMMITEAR\n")
	res, err, out := rdCorrer(t, "CA-482", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
	rdNegada(t, "CA-482", "arbol sucio antes de la separacion", root, cx, antes, registros, res, err, out, raErrSucio(".hoom/notas.txt"))
	if _, err := os.Stat(marca); err == nil {
		t.Fatalf("CA-482/CA-487: la guarda de arbol sucio va antes de la separacion: el merge sintetico no se armo")
	}
	if err := os.Remove(filepath.Join(root, ".hoom", "notas.txt")); err != nil {
		t.Fatal(err)
	}

	res, err, out = rdCorrer(t, "CA-487", root, Options{Provider: "codex", Spec: edrSpec, Desde: otra})
	rdNegada(t, "CA-487", "--desde que no es ancestro", root, cx, antes, registros, res, err, out, rdErrNoAncestro(otra))
	if _, err := os.Stat(marca); err == nil {
		t.Fatalf("CA-487: --desde va antes de la separacion: el merge sintetico no se armo")
	}
}
