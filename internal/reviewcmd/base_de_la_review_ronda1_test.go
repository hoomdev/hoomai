// Tests adversariales del spec .hoom/specs/base-de-la-review.md, ronda 1:
// los defectos que encontro la primera review de 4 lentes, escritos desde el
// spec (con su enmienda 1).
//
//   - CA-431 (enmienda 1): el gate de territorio usa agents.reviewer.write
//     del hoom.yaml del MERGE-BASE con la base (la del proyecto o --base),
//     nunca del arbol revisado, con o sin --task, y tampoco del checkout del
//     proyecto (un cambio sin commitear ahi no lo mueve, ni lo que main
//     commitea despues del punto de rama). Un merge-base sin hoom.yaml deja
//     el territorio por defecto del rol.
//   - CA-432: "<ref> se resuelve con git a un commit". Si la ref de --base se
//     mueve o se borra mientras la review corre, lo que se mide y se revisa
//     (lentes, tamano, evidencia, politica, contrato, desde del registro) es
//     lo del commit que la ref nombraba al empezar; una ref borrada nunca
//     termina SIN REVISAR con exit 0 (CA-414: falla cerrado).
//   - CA-430: el aviso lleva el base_branch de la rama, que es texto del
//     candidato: no puede meter saltos de linea ni escapes de terminal en la
//     salida ni en las notas del registro.
//
// Los fixtures estan en base_de_la_review_test.go (bd*),
// review_por_diferencia_helpers_test.go (rd*) y review_aislada_helpers_test.go
// (ra*).
package reviewcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"gopkg.in/yaml.v3"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// ---------------------------------------------------------------- CA-431 (enmienda 1)

// bdAgrandadaAppGo abre al reviewer app.go (y sus hallazgos).
const bdAgrandadaAppGo = "agents:\n  reviewer:\n    write:\n      allow: [\"app.go\", \".hoom/findings/**\"]\n"

// bdEnViolacion exige que la unica pasada (el reviewer reescribio app.go)
// termine en violacion de territorio: no-entregable, exit 1, el scope roto
// nombrando app.go, sin registro de review en dir.
func bdEnViolacion(t *testing.T, caso, dir string, cx *raCLI, res Result, out string) {
	t.Helper()
	if cx.veces() != 1 || len(res.Passes) != 1 {
		t.Fatalf("CA-431: %s: fixture: la pasada corre: codex %d, %+v\n%s", caso, cx.veces(), res, out)
	}
	if res.Status != "no-entregable" || res.ExitCode != 1 || res.Passes[0].Scope.OK || !bdViolacion(res.Passes[0], "app.go") {
		t.Fatalf("CA-431: %s: el territorio sale del hoom.yaml del merge-base con la base: escribir app.go es violacion de territorio: %s, exit %d, %+v\n%s",
			caso, res.Status, res.ExitCode, res.Passes[0].Scope, out)
	}
	if res.RecordID != "" || rdRegistrosEn(dir) != 0 {
		t.Fatalf("CA-431: %s: una review no-entregable no deja registro de review: %q, %d", caso, res.RecordID, rdRegistrosEn(dir))
	}
}

// bdSinViolacionDeTerritorio exige que la unica pasada (el reviewer
// reescribio app.go) respete su territorio: scope OK, sin violaciones,
// revisado con exit 0.
func bdSinViolacionDeTerritorio(t *testing.T, caso string, cx *raCLI, res Result, out string) {
	t.Helper()
	if cx.veces() != 1 || len(res.Passes) != 1 {
		t.Fatalf("CA-431: %s: fixture: la pasada corre: codex %d, %+v\n%s", caso, cx.veces(), res, out)
	}
	if !res.Passes[0].Scope.OK || len(res.Passes[0].Scope.Violations) != 0 || res.Status != "revisado" || res.ExitCode != 0 {
		t.Fatalf("CA-431: %s: lo que declara el hoom.yaml del merge-base con la base vale: escribir app.go no es violacion: %s, exit %d, %+v\n%s",
			caso, res.Status, res.ExitCode, res.Passes[0].Scope, out)
	}
}

// CA-431 (enmienda 1): "una rama que lo agranda en su hoom.yaml no evita que
// un reviewer que escribe fuera del territorio por defecto termine en
// violacion de territorio, ... tambien sin --task con --base". Sin tarea,
// el arbol revisado es el checkout (rama feature) y es el que commitea
// agents.reviewer.write abierto (allow ["**"], o allow que nombra app.go);
// main no lo abre. Con --base main (Options.Base) y tambien sin --base (la
// base es la que Run recibe; la enmienda 1: "todo lo que sale de ella
// (politica, contrato, territorio) sale del merge-base, no de la rama"), un
// reviewer que reescribe app.go termina en violacion de territorio, sin
// registro.
func TestCA431_SinTareaLaRamaNoAgrandaElTerritorio(t *testing.T) {
	for _, b := range []struct{ nombre, base string }{{"con-base-main", "main"}, {"sin-base", ""}} {
		for _, c := range []struct{ nombre, agrandada string }{{"allow-todo", bdAgrandada}, {"allow-app-go", bdAgrandadaAppGo}} {
			caso := b.nombre + "_" + c.nombre
			t.Run(caso, func(t *testing.T) {
				bin := raPATH(t)
				root := raRepo(t, "")
				rdCommit(t, root, "codigo", map[string]string{"app.go": "package app\n\nfunc EnLaRama() {}\n"})
				rdCommit(t, root, "la rama agranda el territorio del reviewer", map[string]string{"hoom.yaml": raYAML(c.agrandada)})
				cx := bdReviewerQueEscribe(t, bin, root)

				res, out := bdRevisarEn(t, "CA-431", root, "main", root, Options{Provider: "codex", Lens: "risk", Base: b.base})
				bdEnViolacion(t, caso, root, cx, res, out)
			})
		}
	}
}

// CA-431 (enmienda 1): "lo que el proyecto declara en la base si vale", sin
// tarea y en la otra direccion. main commitea agents.reviewer.write.allow
// ["**"]; la rama (el arbol revisado) lo achica en su hoom.yaml commiteado.
// Con --base main y sin --base, el territorio es el del merge-base (abierto):
// un reviewer que reescribe app.go no viola su territorio. El hoom.yaml del
// arbol revisado no manda en ninguna direccion.
func TestCA431_SinTareaLoQueDeclaraLaBaseVale(t *testing.T) {
	for _, b := range []struct{ nombre, base string }{{"con-base-main", "main"}, {"sin-base", ""}} {
		t.Run(b.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, bdAgrandada)
			rdCommit(t, root, "codigo", map[string]string{"app.go": "package app\n\nfunc EnLaRama() {}\n"})
			rdCommit(t, root, "la rama achica el territorio del reviewer", map[string]string{"hoom.yaml": raYAML("")})
			cx := bdReviewerQueEscribe(t, bin, root)

			res, out := bdRevisarEn(t, "CA-431", root, "main", root, Options{Provider: "codex", Lens: "risk", Base: b.base})
			bdSinViolacionDeTerritorio(t, b.nombre, cx, res, out)
		})
	}
}

// CA-431 (enmienda 1): "del hoom.yaml del merge-base con la base (la del
// proyecto o --base)". El territorio sigue a la base que se elige: la rama
// commitea allow ["**"] en T1 (la ref hito apunta a T1) y despues codigo.
// Con --base hito el merge-base es T1, que abre el territorio: reescribir
// app.go vale. Con --base main el merge-base es main, que no lo abre:
// violacion de territorio, aunque el arbol revisado sea el mismo.
func TestCA431_ElTerritorioSigueALaBaseElegida(t *testing.T) {
	for _, c := range []struct {
		nombre, base string
		vale         bool
	}{{"base-del-commit-que-lo-abre", "hito", true}, {"base-main", "main", false}} {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			rdCommit(t, root, "codigo", map[string]string{"app.go": "package app\n\nfunc EnLaRama() {}\n"})
			t1 := rdCommit(t, root, "el commit que abre el territorio", map[string]string{"hoom.yaml": raYAML(bdAgrandada)})
			git(t, root, "branch", "hito", t1)
			rdCommit(t, root, "mas codigo", map[string]string{"b.go": "package app\n\nfunc B() int { return 2 }\n"})
			if mb := bdMB(t, root, "hito"); mb != t1 {
				t.Fatalf("CA-431: fixture: el merge-base de hito con HEAD es T1 (%s): %s", t1[:12], mb)
			}
			cx := bdReviewerQueEscribe(t, bin, root)

			res, out := bdRevisarEn(t, "CA-431", root, "main", root, Options{Provider: "codex", Lens: "risk", Base: c.base})
			if c.vale {
				bdSinViolacionDeTerritorio(t, c.nombre, cx, res, out)
			} else {
				bdEnViolacion(t, c.nombre, root, cx, res, out)
			}
		})
	}
}

// CA-431 (enmienda 1, Contratos): "Un merge-base sin hoom.yaml ... deja el
// territorio por defecto del rol". El primer commit del repo (C0, la ref
// sinyaml) no tiene hoom.yaml; main lo agrega despues; la rama feature abre
// el territorio del reviewer en su hoom.yaml. Con --base sinyaml el
// merge-base es C0, sin hoom.yaml: el territorio es el por defecto y
// reescribir app.go es violacion de territorio.
func TestCA431_MergeBaseSinHoomYAMLDejaElTerritorioPorDefecto(t *testing.T) {
	bin := raPATH(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "sin hoom.yaml")
	c0 := rdSha(t, root, "HEAD")
	git(t, root, "branch", "sinyaml", c0)
	write(t, root, "hoom.yaml", raYAML(""))
	write(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "el proyecto trae su hoom.yaml")
	rdCommit(t, root, "codigo", map[string]string{"app.go": "package app\n\nfunc EnLaRama() {}\n"})
	rdCommit(t, root, "la rama agranda el territorio del reviewer", map[string]string{"hoom.yaml": raYAML(bdAgrandada)})
	if mb := bdMB(t, root, "sinyaml"); mb != c0 {
		t.Fatalf("CA-431: fixture: el merge-base de sinyaml con HEAD es C0: %s", mb)
	}
	if out, err := bdGitSalida(root, "cat-file", "-e", c0+":hoom.yaml"); err == nil {
		t.Fatalf("CA-431: fixture: C0 no tiene hoom.yaml: %s", out)
	}
	cx := bdReviewerQueEscribe(t, bin, root)

	res, out := bdRevisarEn(t, "CA-431", root, "main", root, Options{Provider: "codex", Lens: "risk", Base: "sinyaml"})
	bdEnViolacion(t, "--base de un merge-base sin hoom.yaml", root, cx, res, out)
}

// CA-431 (enmienda 1, Decisiones: "Sale del merge-base ... y no del
// hoom.yaml del checkout del proyecto: ... un cambio sin commitear en el
// checkout no la mueve"). Con --task, el territorio del reviewer no sale del
// checkout del proyecto:
//   - sin-commitear-no-abre: main no abre el territorio; el checkout del
//     proyecto (en main) lo abre en su hoom.yaml SIN commitear. Reescribir
//     app.go es violacion de territorio.
//   - sin-commitear-no-cierra: main commiteo allow ["**"] antes del punto de
//     rama (esta en el merge-base); el checkout lo achica SIN commitear.
//     Reescribir app.go vale.
//   - commiteado-despues-del-punto-de-rama-no-abre: main commitea allow
//     ["**"] DESPUES de crear la tarea: esta en el checkout del proyecto,
//     commiteado, pero no en el merge-base. Reescribir app.go es violacion
//     de territorio.
//
// El worktree de la tarea no toca hoom.yaml en ningun caso.
func TestCA431_ConTareaElTerritorioNoSaleDelCheckoutDelProyecto(t *testing.T) {
	for _, c := range []struct {
		nombre, enMain string
		vale           bool
		checkout       func(t *testing.T, root string)
	}{
		{"sin-commitear-no-abre", "", false, func(t *testing.T, root string) {
			write(t, root, "hoom.yaml", raYAML(bdAgrandada))
		}},
		{"sin-commitear-no-cierra", bdAgrandada, true, func(t *testing.T, root string) {
			write(t, root, "hoom.yaml", raYAML(""))
		}},
		{"commiteado-despues-del-punto-de-rama-no-abre", "", false, func(t *testing.T, root string) {
			write(t, root, "hoom.yaml", raYAML(bdAgrandada))
			git(t, root, "add", "--", "hoom.yaml")
			git(t, root, "commit", "-q", "-m", "main abre el territorio del reviewer despues del punto de rama", "--", "hoom.yaml")
		}},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, c.enMain)
			wt := bdTarea(t, root, bdSlug, "main")
			rdCommit(t, wt, "codigo", map[string]string{"app.go": "package app\n\nfunc EnLaTarea() {}\n"})
			mb := bdMB(t, wt, "main")
			c.checkout(t, root)
			if rama := rdGit(t, root, "rev-parse", "--abbrev-ref", "HEAD"); rama != "main" {
				t.Fatalf("CA-431: fixture: el checkout del proyecto esta en main: %s", rama)
			}
			if bdMB(t, wt, "main") != mb {
				t.Fatalf("CA-431: fixture: el merge-base de la tarea con main es el punto de rama (%s)", mb[:12])
			}
			cx := bdReviewerQueEscribe(t, bin, wt)

			res, out := bdRevisarEn(t, "CA-431", root, "main", wt, Options{Task: bdSlug, Provider: "codex", Lens: "risk"})
			if c.vale {
				bdSinViolacionDeTerritorio(t, c.nombre, cx, res, out)
			} else {
				bdEnViolacion(t, c.nombre, wt, cx, res, out)
			}
		})
	}
}

// ---------------------------------------------------------------- CA-432: la ref se resuelve una vez

// bdGitQueMueveLaRef pone en bin un git que cuenta las invocaciones que
// NOMBRAN la base: un argumento que es la ref como palabra (ref,
// ref^{commit}, ref..HEAD, HEAD...ref, ref:ruta, refs/heads/ref) o que
// contiene el sha completo al que apuntaba al empezar. Justo DESPUES de la
// vez-esima (el git de verdad ya corrio y respondio), mueve refs/heads/<ref>
// a otro commit (otro != "") o la borra (otro == ""), una sola vez y siempre
// con -C root, como otro proceso que la mueve a mitad de la review. Todo lo
// demas es el git de verdad. Devuelve el directorio de estado: n cuenta las
// invocaciones que nombraron la base; hecho existe si la ref se movio.
func bdGitQueMueveLaRef(t *testing.T, bin, real, root, ref, sha, otro string, vez int) string {
	t.Helper()
	estado := t.TempDir()
	accion := "update-ref refs/heads/" + ref + " " + otro
	if otro == "" {
		accion = "update-ref -d refs/heads/" + ref
	}
	const letra = "A-Za-z0-9_-"
	s := "#!/bin/sh\n" +
		"real='" + real + "'\n" +
		"nombra=0\n" +
		"for a in \"$@\"; do\n" +
		"  case \"$a\" in\n" +
		"    *" + sha + "*) nombra=1 ;;\n" +
		"    " + ref + "|" + ref + "[!" + letra + "]*|refs/heads/" + ref + "|refs/heads/" + ref + "[!" + letra + "]*) nombra=1 ;;\n" +
		"    *[!" + letra + "/]" + ref + "|*[!" + letra + "/]" + ref + "[!" + letra + "]*) nombra=1 ;;\n" +
		"  esac\n" +
		"done\n" +
		"if [ \"$nombra\" = 0 ]; then exec \"$real\" \"$@\"; fi\n" +
		"\"$real\" \"$@\"; rc=$?\n" +
		"n=$(cat '" + estado + "/n' 2>/dev/null || echo 0); n=$((n+1)); echo $n > '" + estado + "/n'\n" +
		"if [ \"$n\" = " + strconv.Itoa(vez) + " ]; then\n" +
		"  \"$real\" -C '" + root + "' " + accion + " >/dev/null 2>&1 || exit 97\n" +
		"  echo 1 > '" + estado + "/hecho'\n" +
		"fi\n" +
		"exit $rc\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	return estado
}

// bdPedidoDeHito exige que el pedido de la pasada n sea el de la base que
// hito nombraba al empezar (T1): la primera linea con el sha256 y los KiB de
// la evidencia de T1 a HEAD, la tercera 'Base: hito. Tamano: ...' con el
// tamano de T1 a HEAD, la evidencia entera entre sus marcadores y el
// contrato 06 de T1.
func bdPedidoDeHito(t *testing.T, caso string, cx *raCLI, n int, ev Evidencia, g gitx.Info) {
	t.Helper()
	ped := cx.pedido(t, n)
	l := raLineas(ped)
	sha := "(sha256 " + ev.SHA256 + ", " + strconv.Itoa(raKiB(ev.Bytes)) + " KiB)"
	if len(l) < 3 || !strings.Contains(l[0], sha) {
		t.Fatalf("CA-432: %s: la primera linea del pedido %d dice la evidencia de hito a HEAD %s:\n%.600s", caso, n, sha, ped)
	}
	tam := fmt.Sprintf("Base: hito. Tamano: %d archivos, +%d/-%d lineas.", len(g.ChangedFiles), g.Insertions, g.Deletions)
	if l[2] != tam {
		t.Fatalf("CA-432: %s: la tercera linea del pedido %d mide de hito a HEAD:\nquiero %q\nfue    %q", caso, n, tam, l[2])
	}
	if !strings.Contains(ped, raBloque(t, ev, "", false)) {
		t.Fatalf("CA-432: %s: el pedido %d trae la evidencia de hito a HEAD, entera, entre sus marcadores:\n%.2000s", caso, n, ped)
	}
	if sys := raSistema(t, "codex", cx.argv(t, n)); !raMismoTexto(sys, bdContratoHito) {
		t.Fatalf("CA-432: %s: la pasada %d recibe el contrato 06 del merge-base con hito:\nquiero %q\nfue    %q", caso, n, bdContratoHito, sys)
	}
}

// bdLaMismaReviewQueEnHito exige la review que --base hito da sin que nada
// se mueva: revisado, desde T1, hasta T3, completa, base hito, la lente
// dominante (lo que va de T1 a HEAD es codigo chico), la primera linea de la
// salida con el tamano de T1 a HEAD, la linea evidencia con su sha256, el
// pedido de hito, la politica y el contrato de T1, y el registro con lo
// mismo.
func bdLaMismaReviewQueEnHito(t *testing.T, caso, root string, cx *raCLI, res Result, out string, ev Evidencia, g gitx.Info, t1, t3 string) {
	t.Helper()
	if res.Status != "revisado" || res.ExitCode != 0 || res.RecordID == "" {
		t.Fatalf("CA-432: %s: la review revisa la base que hito nombraba al empezar: %+v\n%s", caso, res, out)
	}
	if res.Desde != t1 || res.Hasta != t3 || res.Cobertura != CoberturaCompleta || res.Base != "hito" {
		t.Fatalf("CA-432: %s: desde es el merge-base con el commit que hito nombraba (%s), hasta HEAD, completa, base hito: desde %s, hasta %s, %s, base %q\n%s",
			caso, t1[:12], res.Desde, res.Hasta, res.Cobertura, res.Base, out)
	}
	if !reflect.DeepEqual(res.Lenses, []string{LenteDominante}) || cx.veces() != 1 {
		t.Fatalf("CA-432: %s: las lentes se miden de hito a HEAD (codigo chico: %s), no del commit al que se movio la ref: %v, codex %d\n%s",
			caso, LenteDominante, res.Lenses, cx.veces(), out)
	}
	if res.EvidenceSHA256 != ev.SHA256 || res.EvidenceBytes != ev.Bytes {
		t.Fatalf("CA-432: %s: la evidencia es la de hito a HEAD (sha256 %s, %d bytes): %s, %d", caso, ev.SHA256[:12], ev.Bytes, res.EvidenceSHA256, res.EvidenceBytes)
	}
	primera := fmt.Sprintf("hoom review: %d archivos cambiados, +%d/-%d lineas ", len(g.ChangedFiles), g.Insertions, g.Deletions)
	if l := raLineas(out); !strings.HasPrefix(l[0], primera) {
		t.Fatalf("CA-432: %s: la primera linea de la salida mide de hito a HEAD:\nquiero %q...\nfue    %q", caso, primera, l[0])
	}
	evid := fmt.Sprintf("  evidencia   %d KiB (diff %d + spec 0), tope 320 KiB - sha256 %s", raKiB(ev.Bytes), raKiB(len(ev.Diff)), ev.SHA256[:12])
	if raCuenta(out, evid) != 1 {
		t.Fatalf("CA-432: %s: la linea evidencia es la de hito a HEAD %q:\n%s", caso, evid, out)
	}
	bdRevisadaConHito(t, "CA-432", caso, root, cx, 0, res, out)
	bdPedidoDeHito(t, caso, cx, 1, ev, g)
	rec := rdRegistro(t, "CA-432", root, res.RecordID)
	if rec.Desde != t1 || rec.Hasta != t3 || rec.Cobertura != CoberturaCompleta || rec.EvidenceSHA256 != ev.SHA256 ||
		rec.EvidenceBytes != ev.Bytes || !reflect.DeepEqual(rec.Lenses, []string{LenteDominante}) {
		t.Fatalf("CA-432: %s: el registro dice desde %s (el commit que hito nombraba al empezar), hasta HEAD, completa, la evidencia y la lente de hito a HEAD: %+v",
			caso, t1[:12], rec)
	}
}

// bdBarrido es hasta que vez que la review nombra la base se barre el
// momento en que la ref se mueve o se borra.
const bdBarrido = 8

// CA-432: "`<ref>` se resuelve con git a un commit" (y CA-414: un error de
// git falla cerrado). La review con --base hito (T1) resuelve la ref a un
// commit: si otro proceso mueve la ref hito a la punta de main, o la borra, a
// mitad de la review, lo que se mide y se revisa sigue siendo lo del commit
// que hito nombraba al empezar. Con la ref en main, la review mediria el
// cambio entero contra main (la ruta de riesgo de T1: las 4 lentes), con la
// politica aislada y el contrato de main; sin la ref, un diff contra hito no
// tiene nada que comparar.
//
// El momento se barre: la ref se mueve (o se borra) justo despues de la
// vez-esima invocacion de git que nombra la base, por su nombre o por su sha
// (1..bdBarrido; eso cubre, entre otras, la primera `git merge-base hito
// ...`). La primera vez tiene que existir (resolver la ref es una
// invocacion de git que la nombra); si la review nombra la base menos veces
// que vez, nada se mueve y la review es la de siempre.
//   - la-ref-se-mueve: la misma review que sin moverla (desde T1, la lente
//     dominante, el tamano y la evidencia de T1 a HEAD en la salida, el
//     pedido y el registro, la politica y el contrato de T1).
//   - la-ref-se-borra: la misma review, o una falla cerrada: nunca SIN
//     REVISAR, nunca exit 0 sin revisar, sin registro, y ninguna pasada con
//     otra base.
//
// Con reloj (60 s por review).
func TestCA432_LaBaseEsElCommitQueLaRefNombrabaAlEmpezar(t *testing.T) {
	for _, c := range []struct {
		nombre string
		borra  bool
	}{{"la-ref-se-mueve", false}, {"la-ref-se-borra", true}} {
		for vez := 1; vez <= bdBarrido; vez++ {
			caso := fmt.Sprintf("%s-despues-de-la-vez-%d", c.nombre, vez)
			t.Run(caso, func(t *testing.T) {
				real := raGitReal(t)
				bin := raPATH(t)
				root, mb, t1, _, t3 := bdHito(t)
				cx := raInstalar(t, bin, "codex", "")
				ev := rdOraculo(t, "CA-432", root, t1, "")
				g := gitx.Snapshot(root, "rd-oraculo-"+t1[:12])
				if len(g.ChangedFiles) != 2 || g.Deletions != 0 || !strings.Contains(string(ev.Diff), "MARCA-C-CA432") ||
					strings.Contains(string(ev.Diff), "internal/auth/token.go") {
					t.Fatalf("CA-432: fixture: de hito a HEAD van b.go y c.go, sin la ruta de riesgo: %+v", g)
				}
				if deMain := raEvidenciaCruda(t, "CA-432", root, ""); deMain.SHA256 == ev.SHA256 || !strings.Contains(string(deMain.Diff), "internal/auth/token.go") {
					t.Fatalf("CA-432: fixture: la evidencia contra main (a donde se mueve la ref) es otra y trae la ruta de riesgo")
				}
				raLimpio(t, "CA-432", root)
				otro := mb
				if c.borra {
					otro = ""
				}

				estado := bdGitQueMueveLaRef(t, bin, real, root, "hito", t1, otro, vez)
				res, err, out := rdCorrer(t, "CA-432", root, Options{Provider: "codex", Base: "hito"})
				if rerr := os.Remove(filepath.Join(bin, "git")); rerr != nil {
					t.Fatal(rerr)
				}
				nombro := rdMovidas(filepath.Join(estado, "n"))
				_, herr := os.Stat(filepath.Join(estado, "hecho"))
				movida := herr == nil
				t.Logf("la review nombro la base en %d invocaciones de git; la ref se %s: %v", nombro, map[bool]string{false: "movio", true: "borro"}[c.borra], movida)
				if vez == 1 && !movida {
					t.Fatalf("CA-432: fixture: la review resuelve --base hito con git (una invocacion que nombra la base), y el git que mueve la ref no la vio:\n%s",
						raMsg(out, err))
				}
				switch {
				case !movida:
					if h := rdSha(t, root, "refs/heads/hito"); h != t1 {
						t.Fatalf("CA-432: fixture: sin moverla, la ref hito sigue en T1: %s", h)
					}
				case c.borra:
					if o, gerr := bdGitSalida(root, "rev-parse", "--verify", "-q", "refs/heads/hito"); gerr == nil {
						t.Fatalf("CA-432: fixture: la ref hito quedo borrada: %s", o)
					}
				default:
					if h := rdSha(t, root, "refs/heads/hito"); h != mb {
						t.Fatalf("CA-432: fixture: la ref hito quedo en la punta de main (%s): %s", mb[:12], h)
					}
				}

				if !c.borra || !movida || res.Status == "revisado" {
					if err != nil {
						t.Fatalf("CA-432: %s: la review no se rompe: %v\n%s", caso, err, out)
					}
					bdLaMismaReviewQueEnHito(t, caso, root, cx, res, out, ev, g, t1, t3)
					return
				}
				// la ref se borro y la review no reviso: falla cerrada
				msg := raMsg(out, err)
				if res.Status == "sin-revisar" || raDiceSinRevisar(msg) {
					t.Fatalf("CA-432/CA-414: %s: una ref borrada a mitad de la review no termina SIN REVISAR (no hay cero cambios: hay un error de git): %+v\n%s",
						caso, res, msg)
				}
				if err == nil && res.ExitCode == 0 {
					t.Fatalf("CA-432/CA-414: %s: la falla cerrada no sale con 0: %+v\n%s", caso, res, msg)
				}
				if res.RecordID != "" || rdRegistrosEn(root) != 0 {
					t.Fatalf("CA-432: %s: sin revisar no hay registro: %q, %d", caso, res.RecordID, rdRegistrosEn(root))
				}
				for n := 1; n <= cx.veces(); n++ {
					bdPedidoDeHito(t, caso+" (falla cerrada)", cx, n, ev, g)
				}
			})
		}
	}
}

// ---------------------------------------------------------------- CA-430: el aviso no lleva controles

const bdPreAviso = "el hoom.yaml de la rama dice base_branch "

// bdSinControles dice si s no tiene ningun caracter de control (salto de
// linea, retorno de carro, ESC, tabulador...).
func bdSinControles(s string) bool {
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// bdAvisoEscapado exige que s sea el aviso del contrato sobre el base_branch
// de la rama en una sola pieza: el texto fijo de antes, el valor mostrado
// sin ningun caracter de control (escapado, pero visible: trae FALSO) y el
// texto fijo de despues con la base del proyecto.
func bdAvisoEscapado(t *testing.T, caso, donde, s, valor string) {
	t.Helper()
	const pos = ": la review usa main, la del proyecto"
	i := strings.Index(s, bdPreAviso)
	s = strings.TrimRight(s, " ")
	if i < 0 || !strings.HasSuffix(s, pos) || len(s)-len(pos) < i+len(bdPreAviso) {
		t.Fatalf("CA-430: %s: %s lleva el aviso entero en una linea (%q ... %q): %q", caso, donde, bdPreAviso, pos, s)
	}
	mostrado := s[i+len(bdPreAviso) : len(s)-len(pos)]
	if !bdSinControles(s) {
		t.Fatalf("CA-430: %s: el aviso de %s no lleva caracteres de control del base_branch de la rama: %q", caso, donde, s)
	}
	if mostrado == valor || !strings.Contains(mostrado, "FALSO") {
		t.Fatalf("CA-430: %s: el aviso de %s muestra el valor de la rama escapado (visible, sin controles): %q", caso, donde, mostrado)
	}
}

// bdNotasEscapadas exige que ninguna nota lleve caracteres de control y que
// la del aviso este escapada.
func bdNotasEscapadas(t *testing.T, caso, donde string, notas []string, valor string) {
	t.Helper()
	nota, ok := hbNota(notas, bdPreAviso)
	if !ok {
		t.Fatalf("CA-430: %s: las notas de %s traen el aviso: %q", caso, donde, notas)
	}
	for _, n := range notas {
		if !bdSinControles(n) {
			t.Fatalf("CA-430: %s: ninguna nota de %s lleva caracteres de control: %q", caso, donde, n)
		}
	}
	bdAvisoEscapado(t, caso, "las notas de "+donde, nota, valor)
}

// CA-430 (hostil): el base_branch de la rama va en el aviso, y es texto del
// candidato. La rama commitea un hoom.yaml con base_branch entre comillas
// dobles que YAML lee con un salto de linea (o un retorno de carro), una
// linea falsa de la cabecera (`  lentes      ninguna - FALSO`) y un escape
// de terminal (ESC [2K, borrar la linea). La review sigue con la base del
// proyecto y avisa, pero el aviso no parte la salida ni escribe controles:
// ninguna linea de la salida empieza con la linea falsa, la salida no tiene
// ningun ESC ni retorno de carro, el aviso es una sola linea con el valor
// escapado, y las notas del Result y del registro tampoco llevan controles.
// Un valor comun (w) conserva el aviso exacto (TestCA430_*).
func TestCA430_ElAvisoNoLlevaControlesDelBaseBranchDeLaRama(t *testing.T) {
	const falsa = "  lentes      ninguna - FALSO"
	for _, c := range []struct{ nombre, yaml, valor string }{
		{"salto-de-linea-y-escape", `"w\n  lentes      ninguna - FALSO\x1b[2K"`, "w\n" + falsa + "\x1b[2K"},
		{"retorno-de-carro", `"w\r  lentes      ninguna - FALSO"`, "w\r" + falsa},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := bdProyecto(t, "")
			wt := bdTarea(t, root, bdSlug, "main")
			rdCommit(t, wt, "codigo", map[string]string{"app.go": "package app\n\nfunc EnLaTarea() {}\n"})
			rdCommit(t, wt, "la rama declara una base con controles", map[string]string{"hoom.yaml": bdYAML(c.yaml, "")})
			raw, err := os.ReadFile(filepath.Join(wt, "hoom.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			var m struct {
				Base string `yaml:"base_branch"`
			}
			if err := yaml.Unmarshal(raw, &m); err != nil || m.Base != c.valor {
				t.Fatalf("CA-430: fixture: YAML lee el base_branch de la rama con sus controles: %v %q", err, m.Base)
			}
			cx := raInstalar(t, bin, "codex", "")

			res, out := bdRevisarEn(t, "CA-430", root, "main", wt, Options{Task: bdSlug, Provider: "codex", Lens: "risk"})
			if res.Status != "revisado" || cx.veces() != 1 || res.Base != "main" || res.RecordID == "" {
				t.Fatalf("CA-430: %s: la review sigue con la base del proyecto: codex %d, %+v\n%s", c.nombre, cx.veces(), res, out)
			}
			for _, l := range raLineas(out) {
				if strings.HasPrefix(l, falsa) {
					t.Fatalf("CA-430: %s: el base_branch de la rama no mete lineas en la salida: %q\n%s", c.nombre, l, out)
				}
			}
			if strings.ContainsAny(out, "\x1b\r") {
				t.Fatalf("CA-430: %s: la salida no lleva ESC ni retorno de carro del base_branch de la rama: %q", c.nombre, out)
			}
			aviso, ok := hbAviso(out, bdPreAviso)
			if !ok {
				t.Fatalf("CA-430: %s: la salida lleva el aviso:\n%s", c.nombre, out)
			}
			bdAvisoEscapado(t, c.nombre, "la salida", aviso, c.valor)
			bdNotasEscapadas(t, c.nombre, "el Result", res.Notes, c.valor)
			bdNotasEscapadas(t, c.nombre, "el registro", rdRegistro(t, "CA-430", wt, res.RecordID).Notes, c.valor)
		})
	}
}
