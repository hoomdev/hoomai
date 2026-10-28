//go:build !windows

// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md, ronda
// 3, en el tablero y en `hoom review --delta` (los hallazgos de la review de
// la correccion, escritos desde el spec):
//
//   - be631b, fec6f2, 85d5b8 (CA-487): "el merge sintetico ... no corre
//     ningun programa de la configuracion: ni drivers de merge ni filtros".
//     No solo el .gitattributes de la rama: tampoco un `* merge=drv` en
//     .git/info/attributes, un merge.default=drv en la config del repo, ni
//     un GIT_CONFIG_PARAMETERS heredado que pida merge.default=drv. En las
//     tres, merge.drv.driver del repo es un programa que deja una marca; la
//     marca nunca aparece y el repositorio queda con los mismos objetos, refs
//     e indice.
//   - d00a72 (CA-485): con un git 2.40 (el piso) el tablero separa la base:
//     solo integrarla no devuelve la tarjeta a Review.
//
// Los fixtures (edrGrande, edrCompartido, edrDriver, edrFotoRastro,
// edrMismoRastro, edrRunDelta, edrGitFalso, ...) estan en
// evidencia_de_la_review_test.go y evidencia_de_la_review_unix_test.go.
package boardcmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

// edrConDriverPosible arma el caso de CA-487 sin .gitattributes en la rama:
// la tarea cambia compartido.txt antes de la review R1 (de 4 lentes, con
// hoom review), despues commitea X; main cambia otra linea de
// compartido.txt (Y); la rama integra main sin conflicto (un merge real,
// sin driver configurado) y commitea Z, con su verde. El merge sintetico de
// D con I tiene que mezclar compartido.txt (cambio de los dos lados): si
// algo eligiera un driver, correria. Devuelve la tarjeta, R1, D e I.
func edrConDriverPosible(t *testing.T, ca string) (tj *rdTarjeta, r1 reviewcmd.Result, d, i string) {
	t.Helper()
	r1Codex(t)
	tj = edrGrande(t)
	edrMainAvanza(t, tj, "main trae compartido.txt", map[string]string{"compartido.txt": edrCompartido(0, "")})
	edrIntegrar(t, tj)
	bdEscribir(t, tj.wt, "compartido.txt", edrCompartido(2, "linea 02 cambiada por la tarea"))
	bdCommitear(t, tj.wt, "la tarea cambia compartido.txt")
	tj.verde(t)
	r1, out := tj.review(t, ca, false)
	r1Completa(t, ca, "450 lineas", r1, out, reviewcmd.Lentes)
	d = r1.Hasta
	r1Esperar()
	bdEscribir(t, tj.wt, "x.go", "package app\n\n// MARCA-X\nfunc X() {}\n")
	bdCommitear(t, tj.wt, "X")
	i = edrMainAvanza(t, tj, "Y", map[string]string{"compartido.txt": edrCompartido(18, "linea 18 cambiada por main"), "y.go": "package app\n\nfunc Y() {}\n"})
	edrIntegrar(t, tj)
	bdEscribir(t, tj.wt, "z.go", "package app\n\n// MARCA-Z\nfunc Z() {}\n")
	bdCommitear(t, tj.wt, "Z")
	tj.verde(t)
	if mb := bdGit(t, tj.wt, "merge-base", "main", "HEAD"); mb != i {
		t.Fatalf("%s: fixture: la base integrada es la punta de main: %s", ca, mb)
	}
	return tj, r1, d, i
}

// edrHoom corre el binario de hoom en dir con extra en su entorno (y sin
// HOOM_TASK), con reloj.
func edrHoom(t *testing.T, bin, dir string, extra []string, args ...string) bxSalida {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOOM_TASK=") && !strings.HasPrefix(kv, "GIT_CONFIG_PARAMETERS=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	cmd.Env = append(cmd.Env, extra...)
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	hecho := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		t.Fatalf("no pude ejecutar hoom %v: %v", args, err)
	}
	go func() { hecho <- cmd.Wait() }()
	select {
	case <-hecho:
	case <-time.After(120 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("hoom %v no volvio en 120 s", args)
	}
	return bxSalida{exit: cmd.ProcessState.ExitCode(), stdout: out.String(), stderr: errb.String()}
}

// Hallazgos be631b, fec6f2 y 85d5b8 (CA-487): tres formas de elegir un
// driver de merge que no son el .gitattributes de la rama, cada una por
// separado: `* merge=drv` en .git/info/attributes, merge.default=drv en la
// config del repo, y GIT_CONFIG_PARAMETERS="'merge.default'='drv'" heredado
// por los hoom que corren (en un subproceso: el binario de hoom, con esa
// variable en su entorno). merge.drv.driver del repo deja una marca, y una
// guarda prueba que con esa forma un merge real de D con I la deja. Despues
// del tablero que evalua la tarjeta (el merge sintetico de D con I para la
// cola de R1), del --delta que separa (base_integrada = I) y de otro
// tablero: la marca no aparecio, y el repo tiene los mismos objetos, refs e
// indice, con el arbol cambiado solo en lo que escribe una review.
func TestHallazgo_be631b_fec6f2_85d5b8_CA487_NingunaConfigEligeUnDriver(t *testing.T) {
	const parametros = "GIT_CONFIG_PARAMETERS='merge.default'='drv'"
	bin := bxBinario(t) // antes del PATH minimo de r1Codex: compila con go
	casos := []struct {
		nombre string
		// elegir deja la forma en el repo de dir (el proyecto o el clon de la
		// guarda) y devuelve lo que va en el entorno de los git y los hoom.
		elegir func(t *testing.T, dir string) []string
	}{
		{"info-attributes", func(t *testing.T, dir string) []string {
			p := bdGit(t, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
			bdEscribir(t, p, filepath.Join("info", "attributes"), "* merge=drv\n")
			return nil
		}},
		{"merge-default-en-la-config", func(t *testing.T, dir string) []string {
			bdGit(t, dir, "config", "merge.default", "drv")
			return nil
		}},
		{"git-config-parameters-heredado", func(t *testing.T, dir string) []string {
			return []string{parametros}
		}},
	}
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			ca := "be631b/fec6f2/85d5b8 CA-487 (" + c.nombre + ")"
			tj, r1, d, i := edrConDriverPosible(t, ca)
			driver, marca := edrDriver(t)

			// guarda: con esta forma, un merge REAL de D con I (en un clon)
			// corre el driver
			clon := filepath.Join(t.TempDir(), "clon")
			bdGit(t, tj.root, "clone", "-q", "--no-hardlinks", tj.root, clon)
			gittest.ApagarMantenimiento(t, clon)
			bdGit(t, clon, "config", "user.email", "test@hoom.dev")
			bdGit(t, clon, "config", "user.name", "hoom test")
			bdGit(t, clon, "config", "merge.drv.driver", driver)
			entorno := c.elegir(t, clon)
			bdGit(t, clon, "checkout", "-q", "--detach", d)
			merge := exec.Command("git", "merge", "-q", "--no-edit", i)
			merge.Dir, merge.Env = clon, append(os.Environ(), entorno...)
			if out, err := merge.CombinedOutput(); err != nil {
				t.Fatalf("%s: fixture: el merge real de D con I en el clon: %v\n%s", ca, err, out)
			}
			if _, err := os.Stat(marca); err != nil {
				t.Fatalf("%s: fixture: con esta forma, un merge real de D con I corre el driver (deja %s): %v", ca, marca, err)
			}
			if err := os.Remove(marca); err != nil {
				t.Fatal(err)
			}

			// el proyecto: el driver y la forma
			bdGit(t, tj.root, "config", "merge.drv.driver", driver)
			bdGit(t, tj.root, "config", "merge.drv.name", "driver de prueba")
			entorno = c.elegir(t, tj.root)

			antes := edrFotoRastro(t, tj)
			if entorno == nil {
				if _, err := Build(tj.root, "main", "high", time.Now().UTC()); err != nil {
					t.Fatalf("%s: el tablero se arma: %v", ca, err)
				}
				edrPideElDelta(t, ca, "X y Z despues de la review", tj.card(t), r1.RecordID, d)
				r1Esperar()
				res, err := edrRunDelta(t, tj)
				if err != nil || res.Status != "revisado" || res.Cobertura != reviewcmd.CoberturaDelta {
					t.Fatalf("%s: el --delta revisa: %+v %v", ca, res, err)
				}
				if m := edrJSONMapa(t, res); m["base_integrada"] != i {
					t.Fatalf("%s: el --delta separo la base integrada (base_integrada %s): %v", ca, i[:12], m["base_integrada"])
				}
				if _, err := Build(tj.root, "main", "high", time.Now().UTC()); err != nil {
					t.Fatalf("%s: el tablero se arma: %v", ca, err)
				}
			} else {
				edrPideElDelta(t, ca, "X y Z despues de la review (hoom board)", edrCardDelBinario(t, ca, edrHoom(t, bin, tj.root, entorno, "board", "--json")), r1.RecordID, d)
				r1Esperar()
				r := edrHoom(t, bin, tj.root, entorno, "review", "--provider", "codex", "--task", bdSlug, "--spec", bdSpec, "--delta", "--json")
				var m map[string]any
				if err := json.Unmarshal([]byte(r.stdout), &m); err != nil || r.exit != 0 {
					t.Fatalf("%s: hoom review --delta --json sale con 0 y JSON (exit %d, %v):\n%s\n%s", ca, r.exit, err, r.stdout, r.stderr)
				}
				if m["status"] != "revisado" || m["cobertura"] != reviewcmd.CoberturaDelta || m["base_integrada"] != i {
					t.Fatalf("%s: el --delta revisa y separa la base integrada (base_integrada %s): %v", ca, i[:12], m)
				}
				edrCardDelBinario(t, ca, edrHoom(t, bin, tj.root, entorno, "board", "--json"))
			}
			despues := edrFotoRastro(t, tj)

			if raw, err := os.ReadFile(marca); err == nil {
				t.Fatalf("%s: el merge sintetico no corre el driver que elige %s: corrio para %q", ca, c.nombre, raw)
			}
			edrMismoRastro(t, ca, antes, despues)
		})
	}
}

// Hallazgo d00a72 (CA-485): con un git que se anuncia 2.40.0 y que, como
// ese git, no conoce la opcion global --attr-source (llego en 2.41), el
// tablero separa la base: despues de la review de la tarjeta, main avanza
// con codigo y la rama solo lo integra sin conflicto (con su verde): la
// tarjeta sigue con su review. (Sin derivar la tarjeta antes del git falso:
// el tablero guarda las colas, CA-427.)
func TestHallazgod00a72_CA485_ConGit240ElTableroSeparaLaBase(t *testing.T) {
	real := edrGitReal(t)
	tj := rdGrande(t)
	r1 := tj.completa(t, "20261028T160000_c0mp1a")
	edrMainAvanza(t, tj, "main avanza con codigo", map[string]string{"internal/auth/main.go": rdCodigo(450, "Main")})
	edrIntegrar(t, tj)
	tj.verde(t)
	edrGitFalso(t, real, "\t--version|version) echo 'git version 2.40.0'; exit 0 ;;\n"+
		"\t--attr-source|--attr-source=*) echo \"unknown option: $a\" >&2; exit 129 ;;\n")
	if out, err := exec.Command("git", "version").Output(); err != nil || strings.TrimSpace(string(out)) != "git version 2.40.0" {
		t.Fatalf("d00a72: fixture: el git del PATH se anuncia 2.40.0: %q %v", out, err)
	}
	if err := exec.Command("git", "--attr-source=HEAD", "version").Run(); err == nil {
		t.Fatal("d00a72: fixture: el git 2.40 del PATH no conoce --attr-source")
	}
	rdCuenta(t, "d00a72/CA-485", "git 2.40.0: solo integrar la base", tj.card(t), r1.ID)
}
