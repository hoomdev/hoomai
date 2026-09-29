// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-420..CA-426) sobre `hoom review`: el registro guarda de que commit a
// que commit reviso (desde, hasta, cobertura, desde_review); --desde <c> y
// --delta revisan un rango (la evidencia es el git diff --find-renames de
// desde a HEAD con las mismas rutas, tope y marcadores); las lentes de un
// rango son las mas estrictas entre la regla sobre el rango y sobre el
// cambio entero; el pedido y la salida con rango cambian solo lo que el
// contrato dice; la politica review: y el contrato del reviewer siguen
// saliendo del merge-base con la base.
//
// Este archivo tiene los fixtures; los tests estan en
// review_por_diferencia_{evidencia,rango,registro,pedido,cli}_test.go.
// Reusa los de review_aislada_helpers_test.go (CLIs falsos en un PATH
// minimo, repos con la base main y la rama feature).
//
// Oraculo de la evidencia de un rango: si desde es ancestro de HEAD, el
// merge-base de una rama que apunta a desde con HEAD es desde mismo. Asi
// Evidence(dir, <rama en desde>, ...), ya verificado por los tests de
// review-aislada-y-modelo-elegido, da exactamente la evidencia que el
// contrato pide para `--desde desde` sin copiar la implementacion.
package reviewcmd

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

// rdGit corre git en dir y devuelve su salida sin espacios alrededor.
func rdGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture: git %v en %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// rdSha es el sha completo del commit rev en dir.
func rdSha(t *testing.T, dir, rev string) string {
	t.Helper()
	s := rdGit(t, dir, "rev-parse", "--verify", rev+"^{commit}")
	if !rdHex40(s) {
		t.Fatalf("fixture: %s no es un sha de 40 hex: %q", rev, s)
	}
	return s
}

// rdMB es el merge-base de main y HEAD en dir.
func rdMB(t *testing.T, dir string) string {
	t.Helper()
	return rdGit(t, dir, "merge-base", "main", "HEAD")
}

var rdReHex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

func rdHex40(s string) bool { return rdReHex40.MatchString(s) }

// rdCommit escribe los archivos (ruta -> contenido) en la rama (raRama) y
// commitea SOLO esas rutas. Devuelve el sha del commit.
func rdCommit(t *testing.T, dir, msg string, archivos map[string]string) string {
	t.Helper()
	raRama(t, dir)
	var rutas []string
	for ruta, cuerpo := range archivos {
		write(t, dir, ruta, cuerpo)
		rutas = append(rutas, ruta)
	}
	git(t, dir, append([]string{"add", "--"}, rutas...)...)
	git(t, dir, append([]string{"commit", "-q", "-m", msg, "--"}, rutas...)...)
	return rdSha(t, dir, "HEAD")
}

// rdCodigo es un archivo Go de n lineas, con la marca en cada una: n lineas
// de codigo que la regla cuenta.
func rdCodigo(n int, marca string) string {
	var b strings.Builder
	b.WriteString("package app\n\n")
	for i := 2; i < n; i++ {
		fmt.Fprintf(&b, "var %s%04d = %d\n", marca, i, i)
	}
	return b.String()
}

// rdRamaDos arma raRepo(reviewYAML) y la rama feature con dos commits de
// codigo chico (una lente cada uno, sin rutas de riesgo): A trae a.go y
// a_util.go (con MARCA-DE-A), B trae b.go y b_util.go (con MARCA-DE-B). Si
// spec no es "", el spec va commiteado en A. Devuelve el repo, el merge-base
// (main), A y B.
func rdRamaDos(t *testing.T, reviewYAML, spec string) (root, mb, a, b string) {
	t.Helper()
	root = raRepo(t, reviewYAML)
	mb = rdSha(t, root, "main")
	archA := map[string]string{
		"a.go":      "package app\n\n// MARCA-DE-A\nfunc A() int { return 1 }\n",
		"a_util.go": "package app\n\n// MARCA-DE-A util\nfunc AUtil() int { return 2 }\n",
	}
	if spec != "" {
		archA[spec] = "# Spec x\n\n- CA-1: algo.\n"
	}
	a = rdCommit(t, root, "commit A", archA)
	b = rdCommit(t, root, "commit B", map[string]string{
		"b.go":      "package app\n\n// MARCA-DE-B\nfunc B() int { return 3 }\n",
		"b_util.go": "package app\n\n// MARCA-DE-B util\nfunc BUtil() int { return 4 }\n\nfunc BMas() int { return 5 }\n",
	})
	if rdMB(t, root) != mb {
		t.Fatalf("fixture: el merge-base de la rama es main (%s)", mb)
	}
	return root, mb, a, b
}

// rdOraculo es la evidencia de desde a HEAD segun el contrato, armada con el
// Evidence ya verificado contra una rama que apunta a desde (su merge-base
// con HEAD es desde).
func rdOraculo(t *testing.T, ca, root, desde, spec string) Evidencia {
	t.Helper()
	rama := "rd-oraculo-" + desde[:12]
	git(t, root, "branch", "-f", rama, desde)
	if mb := rdGit(t, root, "merge-base", rama, "HEAD"); mb != desde {
		t.Fatalf("%s: fixture: %s es ancestro de HEAD y el merge-base de su rama con HEAD es el mismo: %s", ca, desde, mb)
	}
	raLimpio(t, ca, root)
	ev, err := Evidence(root, rama, spec, raTopeGrande)
	if err != nil {
		t.Fatalf("%s: fixture: Evidence contra la rama en %s: %v", ca, desde[:12], err)
	}
	if ev.Over {
		t.Fatalf("%s: fixture: la evidencia de referencia no pasa el tope grande", ca)
	}
	return ev
}

// rdTamano es el tamano de desde..HEAD como lo cuenta git (archivos, +, -),
// fuera de .hoom/. Los fixtures que lo usan no tocan .hoom/agents/ en el
// rango ni tienen renombres ni binarios.
func rdTamano(t *testing.T, dir, desde string) (n, ins, del int) {
	t.Helper()
	out := rdGit(t, dir, "diff", "--numstat", "--no-renames", desde, "HEAD", "--", ".", ":(exclude).hoom")
	for _, l := range strings.Split(out, "\n") {
		if strings.TrimSpace(l) == "" {
			continue
		}
		f := strings.Split(l, "\t")
		if len(f) < 3 {
			t.Fatalf("fixture: numstat ilegible: %q", l)
		}
		i, err1 := strconv.Atoi(f[0])
		d, err2 := strconv.Atoi(f[1])
		if err1 != nil || err2 != nil {
			t.Fatalf("fixture: el rango no tiene binarios: %q", l)
		}
		n++
		ins += i
		del += d
	}
	return n, ins, del
}

// Los textos del contrato de los errores de --desde y --delta.
func rdErrNoCommit(c string) string { return "--desde " + c + ": no es un commit de este repositorio" }

func rdErrNoAncestro(c string) string {
	return "--desde " + c + ": no es un ancestro de HEAD (la review revisa de " + c + " a HEAD)"
}

func rdErrDelta(tarea string) string {
	return "--delta: la tarea " + tarea + " no tiene una review completa o delta que llegue a este HEAD: corre hoom review sin --delta"
}

func rdSinCambios(d12 string) string {
	return "SIN REVISAR - no hay cambios desde " + d12 + ": no hay nada que revisar"
}

func rdSoloDocs(d12 string) string {
	return "SIN REVISAR - lo que cambio desde " + d12 + " es solo documentacion: no se invoca review"
}

// rdCorrer corre la review con reloj (60 s): no puede colgarse.
func rdCorrer(t *testing.T, ca, root string, opt Options) (Result, error, string) {
	t.Helper()
	return raRunConReloj(t, ca, 60*time.Second, root, opt)
}

// rdRevisar corre la review con el arbol limpio y exige que no se rompa.
func rdRevisar(t *testing.T, ca, root string, opt Options) (Result, string) {
	t.Helper()
	raLimpio(t, ca, root)
	res, err, out := rdCorrer(t, ca, root, opt)
	if err != nil {
		t.Fatalf("%s: la review no se rompe: %v\n%s", ca, err, out)
	}
	return res, out
}

// rdRegistrosEn cuenta los registros de review de dir (con o sin avisos).
func rdRegistrosEn(dir string) int {
	recs, _ := Records(dir)
	return len(recs)
}

// rdNegada exige que la review no haya corrido: ni pasadas (el CLI falso no
// se invoco mas que antes), ni registro nuevo, ni revisado ni sin-revisar, ni
// exit 0; y que lo que dijo (salida + error) traiga texto.
func rdNegada(t *testing.T, ca, caso, dir string, cli *raCLI, vecesAntes, registrosAntes int, res Result, err error, out, texto string) string {
	t.Helper()
	msg := raMsg(out, err)
	if !strings.Contains(msg, texto) {
		t.Fatalf("%s: %s: la review dice %q:\n%s", ca, caso, texto, msg)
	}
	if cli != nil && cli.veces() != vecesAntes {
		t.Fatalf("%s: %s: no se lanza ninguna pasada (el provider corrio %d veces, antes %d):\n%s", ca, caso, cli.veces(), vecesAntes, msg)
	}
	if len(res.Passes) != 0 || res.Status == "revisado" || res.Status == "sin-revisar" || res.RecordID != "" {
		t.Fatalf("%s: %s: sin pasadas, sin registro, ni revisado ni sin-revisar: %+v\n%s", ca, caso, res, msg)
	}
	if err == nil && res.ExitCode == 0 {
		t.Fatalf("%s: %s: el error no sale con 0: %+v\n%s", ca, caso, res, msg)
	}
	if n := rdRegistrosEn(dir); n != registrosAntes {
		t.Fatalf("%s: %s: sin registro nuevo (habia %d, hay %d)", ca, caso, registrosAntes, n)
	}
	return msg
}

// rdSinRevisar exige una review con rango que termina SIN REVISAR con el
// texto dado: exit 0, sin error, sin pasadas ni registro.
func rdSinRevisar(t *testing.T, ca, caso, dir string, cli *raCLI, vecesAntes, registrosAntes int, res Result, err error, out, texto string) {
	t.Helper()
	if err != nil {
		t.Fatalf("%s: %s: un rango sin nada que revisar no es un error: %v\n%s", ca, caso, err, out)
	}
	if res.Status != "sin-revisar" || res.ExitCode != 0 || len(res.Passes) != 0 || len(res.Lenses) != 0 || res.RecordID != "" {
		t.Fatalf("%s: %s: SIN REVISAR con exit 0, 0 lentes, sin pasadas ni registro: %+v\n%s", ca, caso, res, out)
	}
	if !strings.Contains(out, texto) {
		t.Fatalf("%s: %s: la salida dice %q:\n%s", ca, caso, texto, out)
	}
	if cli != nil && cli.veces() != vecesAntes {
		t.Fatalf("%s: %s: no se lanza ninguna pasada (%d, antes %d)", ca, caso, cli.veces(), vecesAntes)
	}
	if n := rdRegistrosEn(dir); n != registrosAntes {
		t.Fatalf("%s: %s: sin registro nuevo (habia %d, hay %d)", ca, caso, registrosAntes, n)
	}
}

// rdRegistroAMano escribe un registro de review en dir tal cual (el nombre
// del archivo es su id), como lo dejaria una version vieja de hoom o una
// persona.
func rdRegistroAMano(t *testing.T, dir string, r Record) {
	t.Helper()
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	write(t, dir, filepath.Join(".hoom", RecordsDir, r.ID+".json"), string(raw)+"\n")
}

// rdRegistroViejoAMano escribe un registro con el formato de antes de esta
// spec: sin las claves desde, hasta, cobertura ni desde_review.
func rdRegistroViejoAMano(t *testing.T, dir, id, task string, lentes []string) {
	t.Helper()
	l, _ := json.Marshal(lentes)
	cuerpo := `{
  "id": "` + id + `",
  "created_at": "2026-09-20T10:00:00Z",
  "task": "` + task + `",
  "spec": ".hoom/specs/` + task + `.md",
  "fingerprint": "abc",
  "verdict_id": "v1",
  "verdict": "green",
  "lenses": ` + string(l) + `,
  "provider": "codex",
  "writer": "claude",
  "cross": "cruzada",
  "writers_declared": [],
  "findings": [],
  "model": "",
  "effort": "",
  "isolated": true,
  "evidence_bytes": 10,
  "evidence_sha256": "x",
  "usage": []
}
`
	write(t, dir, filepath.Join(".hoom", RecordsDir, id+".json"), cuerpo)
}

// rdRegistro es el registro que la review dejo (por su id), leido con
// Records.
func rdRegistro(t *testing.T, ca, dir, id string) Record {
	t.Helper()
	recs, _ := Records(dir)
	for _, r := range recs {
		if r.ID == id {
			return r
		}
	}
	t.Fatalf("%s: el registro %s esta en %s: %+v", ca, id, dir, recs)
	return Record{}
}

// rdEsperarSegundo deja pasar un segundo: los ids de los registros tienen
// resolucion de segundos, y "el mas nuevo" tiene que ser inequivoco.
func rdEsperarSegundo() { time.Sleep(1100 * time.Millisecond) }

// rdGitEspia pone en bin un git que anota cada invocacion (sus argumentos
// separados por NUL, una por linea) en log y despues es el git de verdad.
func rdGitEspia(t *testing.T, bin, real string) string {
	t.Helper()
	log := filepath.Join(t.TempDir(), "git.log")
	s := "#!/bin/sh\nprintf '%s\\000' \"$@\" >> '" + log + "'\nprintf '\\n' >> '" + log + "'\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return log
}

// rdGitRecibio dice si alguna invocacion de git anotada en log recibio un
// argumento que contiene v.
func rdGitRecibio(t *testing.T, log, v string) bool {
	t.Helper()
	raw, err := os.ReadFile(log)
	if err != nil {
		return false
	}
	for _, linea := range strings.Split(string(raw), "\n") {
		for _, a := range strings.Split(linea, "\x00") {
			if a != "" && strings.Contains(a, v) {
				return true
			}
		}
	}
	return false
}
