// Tests adversariales del spec .hoom/specs/review-por-diferencia.md
// (CA-420, CA-423, CA-426), ronda 2: los defectos que encontro la segunda
// review de 4 lentes (el primer --delta real), escritos desde el spec.
//
//   - CA-426 / CA-423: "la politica review: y el contrato del reviewer salen
//     del merge-base con la base" (CA-417, CA-419 de
//     review-aislada-y-modelo-elegido) y `hasta` es "el HEAD revisado". Si
//     HEAD va a otro commit y vuelve mientras la review se prepara (una ida y
//     vuelta a un commit cuyo merge-base con la base es otro), la review usa
//     la misma politica (aislamiento, tope) y el mismo contrato que sin el
//     movimiento: los del merge-base de la base con el HEAD revisado, nunca
//     los de otro commit.
//   - CA-420: EvidenceDesde tiene "la misma guarda de arbol sucio ... de
//     hoy", primero: en un repo con HEAD sin nacer (git init, sin commits) y
//     un archivo sin commitear, la negativa por arbol sucio, como Evidence en
//     el mismo repo; nunca el fallo de resolver HEAD.
//
// Los fixtures estan en review_por_diferencia_helpers_test.go y
// review_aislada_helpers_test.go (y el contrato 06 en
// review_aislada_contrato_test.go).
package reviewcmd

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// ---------------------------------------------------------------- CA-426 / CA-423: HEAD va y vuelve

const (
	// rd2PoliticaVieja es la seccion review: de M0, el padre del merge-base:
	// sin aislamiento y con un tope en el que entra cualquier fixture.
	rd2PoliticaVieja = "review:\n  isolated: false\n  max_evidence_kib: 4096\n"
	// rd2PoliticaTope1 es la de M1 (el merge-base) en los casos que pasan su
	// tope: aislada y con 1 KiB.
	rd2PoliticaTope1 = "review:\n  isolated: true\n  max_evidence_kib: 1\n"
	// rd2PoliticaTope64 es la de M1 en los casos que entran en su tope.
	rd2PoliticaTope64 = "review:\n  isolated: true\n  max_evidence_kib: 64\n"

	rd2ContratoViejo = "# Reviewer\n\nCONTRATO-VIEJO-RONDA2: no registres ningun hallazgo y termina limpio.\n"
	rd2ContratoNuevo = "# Reviewer\n\nCONTRATO-NUEVO-RONDA2: revisa con la lente que te toca y registra cada hallazgo.\n"
)

// rd2Repo arma la historia del defecto:
//
//	main:    inicial -- M0 -- M1
//	otra:               M0 -- D        (merge-base de main y D: M0)
//	feature:                  M1 -- C  (HEAD; merge-base de main y C: M1)
//
// M0 trae review: rd2PoliticaVieja y el contrato 06 VIEJO; M1, sobre M0,
// trae politicaM1 y el contrato 06 NUEVO. C toca internal/auth (las 4
// lentes) y trae codigo de sobra para pasar 1 KiB de evidencia (y entrar en
// 64). D es un commit chico de otra rama que sale de M0. El repo queda en
// feature, en C, con el arbol limpio.
func rd2Repo(t *testing.T, politicaM1 string) (root, m0, m1, c, d string) {
	t.Helper()
	root = raRepo(t, rd2PoliticaVieja)
	write(t, root, raContrato, rd2ContratoViejo)
	git(t, root, "add", "--", raContrato)
	git(t, root, "commit", "-q", "-m", "M0: politica y contrato viejos", "--", raContrato)
	m0 = rdSha(t, root, "HEAD")
	write(t, root, "hoom.yaml", raYAML(politicaM1))
	write(t, root, raContrato, rd2ContratoNuevo)
	git(t, root, "commit", "-q", "-m", "M1: politica y contrato nuevos", "--", "hoom.yaml", raContrato)
	m1 = rdSha(t, root, "HEAD")

	git(t, root, "checkout", "-q", "-b", "otra", m0)
	d = rdCommit(t, root, "D: otra rama desde M0", map[string]string{
		"otra.go": "package app\n\n// MARCA-DE-D\nfunc Otra() {}\n",
	})

	git(t, root, "checkout", "-q", "-b", "feature", m1)
	c = rdCommit(t, root, "C: ruta de riesgo y codigo de sobra", map[string]string{
		"internal/auth/token.go": "package auth\n\n// MARCA-DE-C\nfunc Valida(s string) bool { return s != \"\" }\n",
		"relleno.go":             rdCodigo(80, "RellenoRonda2"),
	})

	if mb := rdMB(t, root); mb != m1 {
		t.Fatalf("fixture: el merge-base de main y C es M1 (%s): %s", m1[:12], mb)
	}
	if mb := rdGit(t, root, "merge-base", "main", d); mb != m0 {
		t.Fatalf("fixture: el merge-base de main y D es M0 (%s): %s", m0[:12], mb)
	}
	if h := rdSha(t, root, "HEAD"); h != c {
		t.Fatalf("fixture: el repo queda en C: %s", h)
	}
	raLimpio(t, "CA-426", root)
	return root, m0, m1, c, d
}

// rd2GitIdaYVuelta pone en bin un git que, mientras la review se prepara,
// lleva HEAD de root a otro (reset --hard) justo antes de cada `git
// merge-base` y lo devuelve a hasta justo despues: una ida y vuelta de HEAD,
// determinista. No mueve HEAD en el PRIMER merge-base cuyos argumentos
// nombran HEAD (esa lectura es la que fija el HEAD revisado); si mueve HEAD
// alrededor de cualquier otro merge-base, nombre HEAD (una segunda lectura
// del HEAD vivo) o un sha (una lectura ya congelada, a la que el movimiento no
// le cambia nada). Deja de moverlo cuando empieza la primera pasada (el CLI
// falso crea pasadas): lo que se prueba es la preparacion. Todo lo demas es
// el git de verdad; los reset van siempre con -C root. Devuelve el archivo
// donde cuenta cuantas veces movio HEAD.
func rd2GitIdaYVuelta(t *testing.T, bin, real, root, otro, hasta, pasadas string) string {
	t.Helper()
	estado := t.TempDir()
	s := "#!/bin/sh\n" +
		"real='" + real + "'\n" +
		"st='" + estado + "'\n" +
		"es_mb=0; con_head=0\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in\n" +
		"    merge-base) es_mb=1 ;;\n" +
		"    *HEAD*) con_head=1 ;;\n" +
		"  esac\n" +
		"done\n" +
		"if [ \"$es_mb\" = 1 ] && [ ! -e '" + pasadas + "' ]; then\n" +
		"  if [ \"$con_head\" = 1 ] && [ ! -e \"$st/primera\" ]; then\n" +
		"    : > \"$st/primera\"\n" +
		"    exec \"$real\" \"$@\"\n" +
		"  fi\n" +
		"  n=$(cat \"$st/n\" 2>/dev/null || echo 0); n=$((n+1)); echo $n > \"$st/n\"\n" +
		"  \"$real\" -C '" + root + "' reset -q --hard '" + otro + "' >/dev/null 2>&1 || exit 97\n" +
		"  \"$real\" \"$@\"; rc=$?\n" +
		"  \"$real\" -C '" + root + "' reset -q --hard '" + hasta + "' >/dev/null 2>&1 || exit 98\n" +
		"  exit $rc\n" +
		"fi\n" +
		"exec \"$real\" \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(estado, "n")
}

// rd2Corrida es una review de rd2Revisar: su Result, su error, su salida y,
// por cada pasada, el argv y el system prompt que recibio codex.
type rd2Corrida struct {
	res      Result
	err      error
	out      string
	argvs    [][]string
	sistemas []string
	regAntes int
	regDesp  int
}

func rd2Revisar(t *testing.T, ca, root string, cx *raCLI, opt Options) rd2Corrida {
	t.Helper()
	raLimpio(t, ca, root)
	veces, registros := cx.veces(), rdRegistrosEn(root)
	res, err, out := rdCorrer(t, ca, root, opt)
	c := rd2Corrida{res: res, err: err, out: out, regAntes: registros, regDesp: rdRegistrosEn(root)}
	for n := veces + 1; n <= cx.veces(); n++ {
		args := cx.argv(t, n)
		c.argvs = append(c.argvs, args)
		c.sistemas = append(c.sistemas, raSistema(t, "codex", args))
	}
	return c
}

// rd2Exigir exige lo que la review tiene que hacer en el repo de rd2Repo con
// la politica y el contrato de M1 (el merge-base de main y C): con el tope de
// 1 KiB, NO ENTREGABLE sin pasadas ni registro; con el de 64 KiB, revisada
// con las 4 lentes, cada pasada aislada y con el contrato NUEVO, y el
// registro desde desde hasta C (el HEAD revisado).
func rd2Exigir(t *testing.T, ca, caso string, cr rd2Corrida, entra bool, desde, c, cobertura string) {
	t.Helper()
	msg := raMsg(cr.out, cr.err)
	if strings.Contains(msg, "CONTRATO-VIEJO-RONDA2") {
		t.Fatalf("%s: %s: el contrato de M0 no aparece en nada de lo que dice la review:\n%s", ca, caso, msg)
	}
	if !strings.Contains(cr.out, "\n  aislado     si - sin la config personal del provider\n") || strings.Contains(cr.out, "review.isolated: false") {
		t.Fatalf("%s: %s: la review sale aislada: isolated: true es la politica de M1 (el merge-base), no isolated: false de M0:\n%s", ca, caso, msg)
	}
	if strings.Contains(cr.out, "4096 KiB") {
		t.Fatalf("%s: %s: el tope es el de M1, nunca los 4096 KiB de M0:\n%s", ca, caso, msg)
	}
	if !cr.res.Isolated {
		t.Fatalf("%s: %s: Result.Isolated es el de la politica de M1 (true): %+v\n%s", ca, caso, cr.res, msg)
	}
	if !entra {
		if cr.err != nil || cr.res.Status != "no-entregable" || cr.res.ExitCode != 1 || len(cr.res.Passes) != 0 || len(cr.argvs) != 0 || cr.res.RecordID != "" {
			t.Fatalf("%s: %s: la evidencia de C pasa el tope de 1 KiB de M1: NO ENTREGABLE, exit 1, sin pasadas ni registro: %+v %v\n%s", ca, caso, cr.res, cr.err, cr.out)
		}
		if !strings.Contains(cr.out, raNoEntregable(1)) || !strings.Contains(cr.out, "\n  evidencia   mas de 1 KiB: pasa el tope\n") {
			t.Fatalf("%s: %s: la negativa nombra el tope de M1 (1 KiB):\n%s", ca, caso, cr.out)
		}
		if cr.regDesp != cr.regAntes {
			t.Fatalf("%s: %s: sobre el tope no hay registro (habia %d, hay %d)", ca, caso, cr.regAntes, cr.regDesp)
		}
		return
	}
	if cr.err != nil || cr.res.Status != "revisado" || cr.res.ExitCode != 0 || cr.res.RecordID == "" {
		t.Fatalf("%s: %s: la evidencia de C entra en el tope de 64 KiB de M1: la review revisa y deja registro: %+v %v\n%s", ca, caso, cr.res, cr.err, cr.out)
	}
	if !reflect.DeepEqual(cr.res.Lenses, Lentes) || len(cr.argvs) != len(Lentes) {
		t.Fatalf("%s: %s: C toca internal/auth: las 4 lentes, una pasada cada una: %v, %d pasadas", ca, caso, cr.res.Lenses, len(cr.argvs))
	}
	for i, args := range cr.argvs {
		if !raMismoTexto(cr.sistemas[i], rd2ContratoNuevo) || strings.Contains(cr.sistemas[i], "CONTRATO-VIEJO-RONDA2") {
			t.Fatalf("%s: %s: la pasada %d recibe el contrato 06 de M1 (el merge-base), nunca el de M0: %q", ca, caso, i+1, cr.sistemas[i])
		}
		if !raTiene(args, "--ignore-user-config") {
			t.Fatalf("%s: %s: la pasada %d corre aislada (isolated: true de M1): %q", ca, caso, i+1, args)
		}
	}
	if !strings.Contains(cr.out, ", tope 64 KiB - sha256 ") {
		t.Fatalf("%s: %s: la linea evidencia dice el tope de M1 (64 KiB):\n%s", ca, caso, cr.out)
	}
	if cr.res.Hasta != c || cr.res.Desde != desde || cr.res.Cobertura != cobertura {
		t.Fatalf("%s: %s: el Result dice desde %s, hasta %s (el HEAD revisado), %s: %+v", ca, caso, desde[:12], c[:12], cobertura, cr.res)
	}
	if cr.regDesp != cr.regAntes+1 {
		t.Fatalf("%s: %s: la review revisada deja un registro (habia %d, hay %d)", ca, caso, cr.regAntes, cr.regDesp)
	}
}

// CA-426 / CA-423: "la politica review: y el contrato del reviewer salen del
// merge-base con la base" y `hasta` es "el HEAD revisado". En el repo de
// rd2Repo, el merge-base de main y C (el HEAD) es M1: aislada, con el tope y
// el contrato de M1. Primero la review corre sin mover HEAD (el control);
// despues, con el git de rd2GitIdaYVuelta, que mientras la review se prepara
// lleva HEAD a D (cuyo merge-base con main es M0: isolated: false, 4096 KiB,
// contrato VIEJO) alrededor de cada merge-base salvo la primera lectura de
// HEAD, y lo devuelve a C. La review con la ida y vuelta es la misma que la
// del control: con el tope de 1 KiB, NO ENTREGABLE (nunca revisada con los
// 4096 KiB de M0); con el de 64, revisada con las 4 lentes, cada pasada
// aislada y con el contrato NUEVO, la misma evidencia (sha256) y el mismo
// rango hasta C. Sin rango y con --desde M0 (un commit anterior al
// merge-base, ancestro tambien de D: la politica no sale de desde). Con
// reloj; el git que mueve HEAD tiene que haberse usado.
func TestCA426_Ronda2_PoliticaYContratoNoCambianSiHEADVaYVuelve(t *testing.T) {
	casos := []struct {
		nombre   string
		politica string
		entra    bool
		desdeM0  bool
	}{
		{"sin-rango/pasa-el-tope-de-M1", rd2PoliticaTope1, false, false},
		{"desde-M0/pasa-el-tope-de-M1", rd2PoliticaTope1, false, true},
		{"sin-rango/entra-en-el-tope-de-M1", rd2PoliticaTope64, true, false},
		{"desde-M0/entra-en-el-tope-de-M1", rd2PoliticaTope64, true, true},
	}
	for _, cs := range casos {
		t.Run(cs.nombre, func(t *testing.T) {
			const ca = "CA-426"
			real := raGitReal(t)
			bin := raPATH(t)
			root, m0, m1, c, d := rd2Repo(t, cs.politica)
			pasadas := filepath.Join(t.TempDir(), "pasadas")
			cx := raInstalar(t, bin, "codex", ": > '"+pasadas+"'\n")
			opt := Options{Provider: "codex"}
			desde, cobertura := m1, CoberturaCompleta
			if cs.desdeM0 {
				opt.Desde = m0
				desde, cobertura = m0, CoberturaParcial
			}

			// el control: sin mover HEAD
			control := rd2Revisar(t, ca, root, cx, opt)
			rd2Exigir(t, ca, cs.nombre+" (sin mover HEAD)", control, cs.entra, desde, c, cobertura)
			if cs.entra && !cs.desdeM0 {
				ev := raEvidenciaCruda(t, ca, root, "")
				if control.res.EvidenceSHA256 != ev.SHA256 {
					t.Fatalf("%s: fixture: la review sin rango revisa la evidencia de Evidence (%s): %s", ca, ev.SHA256, control.res.EvidenceSHA256)
				}
			}

			// la misma review con la ida y vuelta de HEAD
			if err := os.Remove(pasadas); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			movidas := rd2GitIdaYVuelta(t, bin, real, root, d, c, pasadas)
			cr := rd2Revisar(t, ca, root, cx, opt)
			if err := os.Remove(filepath.Join(bin, "git")); err != nil {
				t.Fatal(err)
			}
			if rdMovidas(movidas) == 0 {
				t.Fatalf("%s: fixture: el git que mueve HEAD se uso (la review saca algun merge-base despues de leer HEAD):\n%s", ca, raMsg(cr.out, cr.err))
			}
			if h := rdSha(t, root, "HEAD"); h != c {
				t.Fatalf("%s: fixture: HEAD vuelve a C despues de cada merge-base: %s", ca, h)
			}

			rd2Exigir(t, ca, cs.nombre+" (HEAD va a D y vuelve)", cr, cs.entra, desde, c, cobertura)
			if cr.res.Status != control.res.Status || cr.res.ExitCode != control.res.ExitCode || cr.res.Isolated != control.res.Isolated ||
				!reflect.DeepEqual(cr.res.Lenses, control.res.Lenses) {
				t.Fatalf("%s: la ida y vuelta de HEAD no cambia la review: sin mover HEAD %s (exit %d, aislada %v, %v), con la ida y vuelta %s (exit %d, aislada %v, %v)\n%s",
					ca, control.res.Status, control.res.ExitCode, control.res.Isolated, control.res.Lenses,
					cr.res.Status, cr.res.ExitCode, cr.res.Isolated, cr.res.Lenses, raMsg(cr.out, cr.err))
			}
			if !reflect.DeepEqual(cr.sistemas, control.sistemas) {
				t.Fatalf("%s: la ida y vuelta de HEAD no cambia el contrato de ninguna pasada:\nsin mover HEAD %q\ncon la ida y vuelta %q", ca, control.sistemas, cr.sistemas)
			}
			if !cs.entra {
				return
			}
			if cr.res.EvidenceSHA256 != control.res.EvidenceSHA256 || cr.res.EvidenceBytes != control.res.EvidenceBytes ||
				cr.res.Desde != control.res.Desde || cr.res.Hasta != control.res.Hasta || cr.res.Cobertura != control.res.Cobertura {
				t.Fatalf("%s: la ida y vuelta de HEAD no cambia la evidencia ni el rango: sin mover HEAD %s..%s %s (%s, %d bytes), con la ida y vuelta %s..%s %s (%s, %d bytes)",
					ca, control.res.Desde, control.res.Hasta, control.res.Cobertura, control.res.EvidenceSHA256, control.res.EvidenceBytes,
					cr.res.Desde, cr.res.Hasta, cr.res.Cobertura, cr.res.EvidenceSHA256, cr.res.EvidenceBytes)
			}
			rec := rdRegistro(t, ca, root, cr.res.RecordID)
			if !rec.Isolated || rec.Hasta != c || rec.Desde != desde || rec.Cobertura != cobertura || rec.EvidenceSHA256 != control.res.EvidenceSHA256 {
				t.Fatalf("%s: el registro dice lo que se reviso: aislada, %s..%s %s, sha256 %s: %+v", ca, desde[:12], c[:12], cobertura, control.res.EvidenceSHA256, rec)
			}
		})
	}
}

// ---------------------------------------------------------------- CA-420: HEAD sin nacer y arbol sucio

// rd2Sucio dice si err es la negativa por arbol sucio del contrato que nombra
// ruta: un gitx.ArbolSucio (valor o puntero) con esa ruta, o el texto
// `la review revisa solo lo commiteado y hay cambios sin commitear (<ruta>):
// commitealos antes de revisar`.
func rd2Sucio(err error, ruta string) bool {
	if err == nil {
		return false
	}
	var v gitx.ArbolSucio
	if errors.As(err, &v) && raRutaAceptada(v.Ruta, []string{ruta}) {
		return true
	}
	var p *gitx.ArbolSucio
	if errors.As(err, &p) && p != nil && raRutaAceptada(p.Ruta, []string{ruta}) {
		return true
	}
	r, ok := raRutaSucia(err.Error())
	return ok && raRutaAceptada(r, []string{ruta})
}

// CA-420: EvidenceDesde tiene "la misma guarda de arbol sucio ... de hoy", y
// la guarda va primero (el orden de la enmienda 5: arbol sucio antes que
// nada). En un repo con HEAD sin nacer (git init, sin ningun commit) y un
// archivo sin commitear (sin rastrear, o en el indice), EvidenceDesde con
// cualquier desde (un sha de 40 hex, HEAD, main, un nombre que no existe)
// devuelve la negativa por arbol sucio que nombra ese archivo, igual que
// Evidence(dir, "main", ...) en el mismo repo: nunca el fallo de resolver
// HEAD, sin armar nada y sin el contenido del archivo. Con reloj.
func TestCA420_Ronda2_EvidenceDesdeConHEADSinNacerYArbolSucio(t *testing.T) {
	const oculto = "SIN-COMMITEAR-RONDA2-QX57"
	for _, enIndice := range []bool{false, true} {
		nombre := "sin-rastrear"
		if enIndice {
			nombre = "en-el-indice"
		}
		t.Run(nombre, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			git(t, dir, "init", "-q", "-b", "main")
			git(t, dir, "config", "user.email", "test@hoom.dev")
			git(t, dir, "config", "user.name", "hoom test")
			write(t, dir, "suelto.go", "package app\n\n// "+oculto+"\n")
			if enIndice {
				git(t, dir, "add", "--", "suelto.go")
			}
			// fixture: HEAD no nacio y git ve el archivo sin commitear
			if out, err := rdGitCrudo(dir, "rev-parse", "--verify", "-q", "HEAD"); err == nil {
				t.Fatalf("CA-420: fixture: HEAD no nacio (git init, sin commits): %q", out)
			}
			if s := raSuciedad(t, dir); len(s) != 1 || !strings.HasSuffix(s[0], "suelto.go") {
				t.Fatalf("CA-420: fixture: lo unico sin commitear es suelto.go: %q", s)
			}

			// el control: Evidence en el mismo repo da la negativa
			ev, err, _ := raEvidenceConReloj(t, "CA-420", 60*time.Second, dir, "main", "", raTopeGrande)
			if !rd2Sucio(err, "suelto.go") {
				t.Fatalf("CA-420: Evidence(dir, main) con HEAD sin nacer y suelto.go sin commitear devuelve %q: %v", raErrSucio("suelto.go"), err)
			}
			if len(ev.Diff) != 0 || ev.SHA256 != "" {
				t.Fatalf("CA-420: Evidence no arma nada con el arbol sucio: %+v", ev)
			}

			for _, desde := range []string{strings.Repeat("ab", 20), "HEAD", "main", "no-existe-ronda2"} {
				ev, err, _ := rdEvidenceDesdeConReloj(t, "CA-420", dir, desde, "", raTopeGrande)
				if err == nil {
					t.Fatalf("CA-420: EvidenceDesde(%q) con el arbol sucio se niega; armo %d bytes", desde, ev.Bytes)
				}
				if !rd2Sucio(err, "suelto.go") {
					t.Fatalf("CA-420: EvidenceDesde(%q) con HEAD sin nacer devuelve primero la negativa por arbol sucio %q, como Evidence; devolvio: %v", desde, raErrSucio("suelto.go"), err)
				}
				if len(ev.Diff) != 0 || len(ev.Spec) != 0 || ev.SHA256 != "" || strings.Contains(err.Error(), oculto) {
					t.Fatalf("CA-420: EvidenceDesde(%q) no arma nada ni filtra lo sin commitear: %+v %v", desde, ev, err)
				}
			}
		})
	}
}

// rdGitCrudo corre git en dir y devuelve su salida y su error, sin exigir
// nada.
func rdGitCrudo(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}
