// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md
// (CA-481, CA-482, CA-483 en la review): la evidencia es todo menos el
// estado de hoom, el arbol sucio se mide sobre esas mismas rutas, y una rama
// que solo cambia .hoom/semgrep/ tiene evidencia y lentes. Los fixtures
// estan en evidencia_de_la_review_helpers_test.go.
package reviewcmd

import (
	"bytes"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// ---------------------------------------------------------------- CA-481

// edrEntran son las rutas de CA-481 que la evidencia trae; edrNoEntran, las
// del estado de hoom que no trae (el spec va entero, aparte).
var (
	edrEntran = []string{
		".hoom/semgrep/gate.sh",
		".hoom/semgrep/paquetes.lock",
		".hoom/notas.txt",
		".hoom/otra/x.sh",
		".hoom/agents-viejo/nota.md",
		".hoom/findings/x.sh",
		".hoom/findings/sub/y.json",
	}
	edrNoEntran = []string{
		".hoom/findings/20261027T120000_a1b2c3.json",
		".hoom/findings/20261027T120000_a1b2c3.res.json",
		".hoom/reviews/20261027T120000_d4e5f6.json",
		".hoom/verdicts/2026-10-27T12-00-00Z_0a0b0c.json",
		".hoom/items/precios.yaml",
		".hoom/approvals/x.json",
		edrSpec,
		".hoom/intake/contrato.pdf",
		".hoom/intake/cliente/requisitos.md",
		".hoom/ratchet.json",
		".hoom/.gitignore",
	}
)

// edrContenido es el contenido de ruta en el commit commit: una linea con
// su marca por commit, para ver en el diff que cambio y que no.
func edrContenido(ruta, commit string) string {
	if ruta == ".hoom/.gitignore" {
		return hoomfs.GitignoreBody() + "# MARCA-" + commit + " " + ruta + "\n"
	}
	return "base de " + ruta + "\nMARCA-" + commit + " " + ruta + "\n"
}

// CA-481: con cambios commiteados en .hoom/semgrep/gate.sh,
// .hoom/semgrep/paquetes.lock, .hoom/notas.txt, un directorio nuevo
// .hoom/otra/x.sh, .hoom/agents-viejo/nota.md, .hoom/findings/x.sh y
// .hoom/findings/sub/y.json, Evidence y EvidenceDesde los traen; y no traen
// los cambios commiteados en el estado de hoom (un hallazgo .json y su
// .res.json, un registro de review, un veredicto, un item, una aprobacion, el
// spec, .hoom/intake/, .hoom/ratchet.json, .hoom/.gitignore). El spec sigue
// yendo entero. Dos commits A y B tocan todo: Evidence (del merge-base)
// trae las rutas que entran como archivos nuevos con lo de B; EvidenceDesde
// A trae solo lo que B cambio de ellas; EvidenceDesde el merge-base es
// Evidence.
func TestCA481_EvidenceYEvidenceDesdeTraenTodoMenosElEstadoDeHoom(t *testing.T) {
	root := raRepo(t, "")
	mb := rdSha(t, root, "main")
	todos := append(append([]string{}, edrEntran...), edrNoEntran...)
	commitear := func(commit string) string {
		archivos := map[string]string{"app.go": "package app\n\n// MARCA-" + commit + " app.go\n"}
		for _, r := range todos {
			archivos[r] = edrContenido(r, commit)
		}
		return rdCommit(t, root, "la rama toca todo ("+commit+")", archivos)
	}
	a := commitear("A")
	commitear("B")
	edrLimpio(t, "CA-481", root)

	ev, err := Evidence(root, "main", edrSpec, raTopeGrande)
	if err != nil || ev.Over {
		t.Fatalf("CA-481: Evidence arma la evidencia: %v (Over %v)", err, ev.Over)
	}
	desdeA, err := EvidenceDesde(root, a, edrSpec, raTopeGrande)
	if err != nil || desdeA.Over {
		t.Fatalf("CA-481: EvidenceDesde(A) arma la evidencia: %v (Over %v)", err, desdeA.Over)
	}
	for _, c := range []struct {
		nombre string
		ev     Evidencia
		quito  bool // la evidencia trae lo que quito (la linea de A)
	}{{"Evidence", ev, false}, {"EvidenceDesde(A)", desdeA, true}} {
		diff := string(c.ev.Diff)
		for _, r := range edrEntran {
			sec := raSeccion(diff, "diff --git a/"+r+" b/"+r)
			if sec == "" || !strings.Contains(sec, "\n+MARCA-B "+r+"\n") {
				t.Fatalf("CA-481: %s trae el cambio commiteado en %s (con su linea de B):\n%s", c.nombre, r, diff)
			}
			if c.quito != strings.Contains(sec, "\n-MARCA-A "+r+"\n") {
				t.Fatalf("CA-481: %s: la seccion de %s trae la linea que quito B si y solo si arranca en A:\n%s", c.nombre, r, sec)
			}
		}
		for _, r := range edrNoEntran {
			if strings.Contains(diff, "a/"+r+" ") || strings.Contains(diff, "b/"+r+"\n") || strings.Contains(diff, "MARCA-B "+r+"\n") ||
				strings.Contains(diff, "MARCA-A "+r+"\n") {
				t.Fatalf("CA-481: %s no trae el cambio commiteado en %s (estado de hoom):\n%s", c.nombre, r, diff)
			}
		}
		if !strings.Contains(diff, "+// MARCA-B app.go") {
			t.Fatalf("CA-481: %s trae el codigo:\n%s", c.nombre, diff)
		}
		if string(c.ev.Spec) != edrContenido(edrSpec, "B") {
			t.Fatalf("CA-481: %s: el spec sigue yendo entero, el de HEAD: %q", c.nombre, c.ev.Spec)
		}
		if c.ev.Bytes != len(c.ev.Diff)+len(c.ev.Spec) || c.ev.SHA256 != raSHA(c.ev.Diff, c.ev.Spec) {
			t.Fatalf("CA-481: %s: Bytes y SHA256 cierran: %d %q", c.nombre, c.ev.Bytes, c.ev.SHA256)
		}
	}
	desdeMB, err := EvidenceDesde(root, mb, edrSpec, raTopeGrande)
	if err != nil || !bytes.Equal(desdeMB.Diff, ev.Diff) || desdeMB.SHA256 != ev.SHA256 {
		t.Fatalf("CA-481: EvidenceDesde con el merge-base es Evidence, byte a byte: %v", err)
	}
}

// ---------------------------------------------------------------- CA-482

// edrCasoSucio es una forma de ensuciar el arbol fuera del estado de hoom.
type edrCasoSucio struct {
	nombre, ruta, oculto string
	ensuciar             func(t *testing.T, dir string)
}

var edrCasosSucios = []edrCasoSucio{
	{"gate-de-semgrep-modificado", ".hoom/semgrep/gate.sh", "GATE-SIN-COMMITEAR", func(t *testing.T, dir string) {
		write(t, dir, ".hoom/semgrep/gate.sh", "set -e\nsemgrep --config .hoom/semgrep/reglas.yaml\nexit 0 # GATE-SIN-COMMITEAR\n")
	}},
	{"notas-sin-rastrear", ".hoom/notas.txt", "NOTAS-SIN-COMMITEAR", func(t *testing.T, dir string) {
		write(t, dir, ".hoom/notas.txt", "NOTAS-SIN-COMMITEAR\n")
	}},
	{"contrato-del-writer-editado", ".hoom/agents/04-writer.md", "CONTRATO-SIN-COMMITEAR", func(t *testing.T, dir string) {
		write(t, dir, ".hoom/agents/04-writer.md", "# Writer\n\nCONTRATO-SIN-COMMITEAR\n")
	}},
}

// edrBaseConHoom commitea en main (el checkout de root) un gate de semgrep y
// el contrato del writer: lo que los casos sucios editan.
func edrBaseConHoom(t *testing.T, root string) {
	t.Helper()
	write(t, root, ".hoom/semgrep/gate.sh", "set -e\nsemgrep --config .hoom/semgrep/reglas.yaml\n")
	write(t, root, ".hoom/agents/04-writer.md", "# Writer\n\ncontrato commiteado\n")
	write(t, root, ".hoom/ratchet.json", "{}\n")
	write(t, root, edrSpec, "# Spec x\n\nversion commiteada\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "la base trae un gate, un contrato, el trinquete y un spec")
}

// CA-482: con una modificacion sin commitear de .hoom/semgrep/gate.sh, un
// .hoom/notas.txt sin rastrear o una edicion sin commitear de
// .hoom/agents/04-writer.md, Evidence y `hoom review` (con y sin --task)
// devuelven el error de arbol sucio con esa ruta, sin pasadas ni registro, y
// nada de lo sin commitear llega al provider ni a .hoom/.
func TestCA482_ArbolSucioEnHoomFueraDelEstadoFrenaLaReview(t *testing.T) {
	for _, c := range edrCasosSucios {
		for _, conTarea := range []bool{false, true} {
			nombre := c.nombre + "/sin-task"
			if conTarea {
				nombre = c.nombre + "/con-task"
			}
			t.Run(nombre, func(t *testing.T) {
				bin := raPATH(t)
				root := raRepo(t, "")
				edrBaseConHoom(t, root)
				dir, opt := root, Options{Provider: "codex", Lens: "risk"}
				if conTarea {
					dir = bdTarea(t, root, bdSlug, "main")
					opt.Task = bdSlug
					rdCommit(t, dir, "codigo de la tarea", map[string]string{"app.go": "package app\n\nfunc EnLaTarea() {}\n"})
				} else {
					raCambio(t, root)
				}
				edrLimpio(t, "CA-482", dir)
				c.ensuciar(t, dir)
				cx := raInstalar(t, bin, "codex", "")

				ev, err, _ := raEvidenceConReloj(t, "CA-482", 20*time.Second, dir, "main", "", raTopeGrande)
				if err == nil {
					t.Fatalf("CA-482: Evidence devuelve %q; armo %d bytes de evidencia", raErrSucio(c.ruta), ev.Bytes)
				}
				if ruta, ok := raRutaSucia(err.Error()); !ok || ruta != c.ruta {
					t.Fatalf("CA-482: Evidence devuelve el error de arbol sucio con %s: %v", c.ruta, err)
				}
				if len(ev.Diff) != 0 || ev.SHA256 != "" {
					t.Fatalf("CA-482: sin armar la evidencia: %d bytes", len(ev.Diff))
				}

				res, rerr, out := raRunConReloj(t, "CA-482", 30*time.Second, root, opt)
				msg := rdNegada(t, "CA-482", nombre, dir, cx, 0, 0, res, rerr, out, raErrSucio(c.ruta))
				if strings.Contains(msg, c.oculto) {
					t.Fatalf("CA-482: nada sin commitear aparece en la salida ni en el error (%q):\n%s", c.oculto, msg)
				}
				raSinTextoEnArbol(t, "CA-482", root, c.oculto, filepath.Join(dir, filepath.FromSlash(c.ruta)))
			})
		}
	}
}

// CA-482 (control): con cambios sin commitear SOLO en entradas del estado de
// hoom —un veredicto, un hallazgo, un registro de review, un item, una
// aprobacion, un documento recien subido a .hoom/intake/, un borrador de spec
// y la edicion del spec commiteado, el trinquete, .hoom/.gitignore y los
// locales—, la review corre (con y sin --task), y nada de eso va en la
// evidencia.
func TestCA482_CambiosSoloEnElEstadoDeHoomNoFrenanLaReview(t *testing.T) {
	for _, conTarea := range []bool{false, true} {
		nombre := "sin-task"
		if conTarea {
			nombre = "con-task"
		}
		t.Run(nombre, func(t *testing.T) {
			bin := raPATH(t)
			root := raRepo(t, "")
			edrBaseConHoom(t, root)
			dir, opt := root, Options{Provider: "codex", Lens: "risk"}
			if conTarea {
				dir = bdTarea(t, root, bdSlug, "main")
				opt.Task = bdSlug
				rdCommit(t, dir, "codigo de la tarea", map[string]string{"app.go": "package app\n\nfunc Nuevo() {}\n"})
			} else {
				raCambio(t, root)
			}
			// el ritual: un veredicto, un hallazgo, un registro de review
			rcVeredicto(t, dir)
			if _, err := finding.Add(dir, "main", "low", "risk", "app.go", "HALLAZGO-SIN-COMMITEAR", "reviewer@claude"); err != nil {
				t.Fatal(err)
			}
			if _, err := WriteRecord(dir, Record{Task: "otra", Spec: ".hoom/specs/otra.md", Lenses: []string{"risk"},
				Provider: "claude", Cross: CrossUnknown, Findings: []string{}, WritersDeclared: []string{}, Usage: []LensUsage{}}); err != nil {
				t.Fatal(err)
			}
			// lo que sube el Studio y lo que escribe hoom
			write(t, dir, ".hoom/items/otro.yaml", "titulo: ITEM-SIN-COMMITEAR\n")
			write(t, dir, ".hoom/approvals/borrador.json", `{"spec":"BORRADOR-APROBADO-SIN-COMMITEAR"}`+"\n")
			write(t, dir, ".hoom/intake/cliente/requisitos.pdf", "%PDF-1.4 INTAKE-RECIEN-SUBIDO\x00\x01")
			write(t, dir, ".hoom/specs/borrador.md", "# Borrador\n\nBORRADOR-DE-SPEC\n")
			write(t, dir, edrSpec, "# Spec x\n\nEDICION-DEL-SPEC-SIN-COMMITEAR\n")
			write(t, dir, ".hoom/ratchet.json", `{"TRINQUETE-SIN-COMMITEAR":1}`+"\n")
			write(t, dir, ".hoom/.gitignore", hoomfs.GitignoreBody()+"# GITIGNORE-COMPLETADO-POR-HOOM\n")
			write(t, dir, ".hoom/runs/x.log", "RUN-LOCAL\n")
			write(t, dir, ".hoom/cache/y.txt", "CACHE-LOCAL\n")
			edrLimpio(t, "CA-482", dir)
			ocultos := []string{"HALLAZGO-SIN-COMMITEAR", "ITEM-SIN-COMMITEAR", "BORRADOR-APROBADO-SIN-COMMITEAR", "INTAKE-RECIEN-SUBIDO",
				"BORRADOR-DE-SPEC", "EDICION-DEL-SPEC-SIN-COMMITEAR", "TRINQUETE-SIN-COMMITEAR", "GITIGNORE-COMPLETADO-POR-HOOM", "RUN-LOCAL", "CACHE-LOCAL"}

			ev, err, _ := raEvidenceConReloj(t, "CA-482", 30*time.Second, dir, "main", "", raTopeGrande)
			if err != nil || ev.Over || !strings.Contains(string(ev.Diff), "func ") {
				t.Fatalf("CA-482: con cambios solo en el estado de hoom la evidencia se arma: %v (Over %v)", err, ev.Over)
			}
			cx := raInstalar(t, bin, "codex", "")
			res, rerr, out := raRunConReloj(t, "CA-482", 60*time.Second, root, opt)
			if rerr != nil || res.Status != "revisado" || cx.veces() != 1 || res.RecordID == "" {
				t.Fatalf("CA-482: con cambios solo en el estado de hoom la review corre: codex %d, %+v %v\n%s", cx.veces(), res, rerr, out)
			}
			if res.EvidenceSHA256 != ev.SHA256 {
				t.Fatalf("CA-482: la review recibe esa misma evidencia: %s vs %s", res.EvidenceSHA256, ev.SHA256)
			}
			p := cx.pedido(t, 1)
			for _, o := range ocultos {
				if strings.Contains(string(ev.Diff), o) || strings.Contains(p, o) {
					t.Fatalf("CA-482: %q (estado de hoom sin commitear) no va en la evidencia ni llega al provider", o)
				}
			}
		})
	}
}

// ---------------------------------------------------------------- CA-483

// CA-483 (caso limite: "una rama que solo cambia .hoom/semgrep/: la review
// tiene evidencia y lentes, y no termina SIN REVISAR"). La rama commitea solo
// .hoom/semgrep/gate.sh y .hoom/semgrep/paquetes.lock: la review sin rango
// revisa (las lentes de la regla), y cada pasada recibe el diff de los dos.
func TestCA483_UnaRamaQueSoloCambiaSemgrepSeRevisa(t *testing.T) {
	bin := raPATH(t)
	root := raRepo(t, "")
	rdCommit(t, root, "solo semgrep", map[string]string{
		".hoom/semgrep/gate.sh":       "set -e\n# GATE-DE-LA-RAMA\nsemgrep --config .hoom/semgrep/reglas.yaml\n",
		".hoom/semgrep/paquetes.lock": "semgrep==1.2.3 # LOCK-DE-LA-RAMA\n",
	})
	cx := raInstalar(t, bin, "codex", "")
	res, out := edrRevisar(t, "CA-483", root, Options{Provider: "codex"})
	if res.Status != "revisado" || len(res.Lenses) == 0 || len(res.Passes) != len(res.Lenses) || cx.veces() != len(res.Lenses) || raDiceSinRevisar(out) {
		t.Fatalf("CA-483: una rama que solo cambia .hoom/semgrep/ se revisa con lentes, no SIN REVISAR: %+v\n%s", res, out)
	}
	diff, _ := edrDiffDelPedido(t, "CA-483", cx.pedido(t, 1))
	for _, quiero := range []string{"diff --git a/.hoom/semgrep/gate.sh b/.hoom/semgrep/gate.sh", "+# GATE-DE-LA-RAMA",
		"diff --git a/.hoom/semgrep/paquetes.lock b/.hoom/semgrep/paquetes.lock", "LOCK-DE-LA-RAMA"} {
		if !strings.Contains(diff, quiero) {
			t.Fatalf("CA-483: la evidencia de la pasada trae %q:\n%s", quiero, diff)
		}
	}
}

// CA-483: despues de una review, un commit que solo toca
// .hoom/semgrep/gate.sh es algo que revisar: --delta lo revisa con lentes
// (no SIN REVISAR), y su evidencia es el diff de gate.sh. Y un commit que solo
// toca el estado de hoom (un veredicto, hallazgos, un registro de review)
// no lo es: --delta despues de el termina SIN REVISAR, sin cambios.
func TestCA483_DeltaDeUnCommitSoloDeSemgrep(t *testing.T) {
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")
	root, r1, d := edrConReview(t, "CA-483", cx)
	gate := rdCommit(t, root, "solo el gate", map[string]string{".hoom/semgrep/gate.sh": "set -e\n# GATE-NUEVO\nexit 0\n"})
	antes := cx.veces()
	res, out := edrRevisar(t, "CA-483", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
	if res.Status != "revisado" || res.Cobertura != CoberturaDelta || res.DesdeReview != r1.RecordID || res.Desde != d || res.Hasta != gate ||
		len(res.Lenses) == 0 || cx.veces() != antes+len(res.Lenses) {
		t.Fatalf("CA-483: --delta revisa con lentes el commit que solo toca .hoom/semgrep/gate.sh: %+v\n%s", res, out)
	}
	diff, _ := edrDiffDelPedido(t, "CA-483", cx.pedido(t, antes+1))
	if got := edrCabeceras(diff); !reflect.DeepEqual(got, []string{".hoom/semgrep/gate.sh"}) || !strings.Contains(diff, "+# GATE-NUEVO") {
		t.Fatalf("CA-483: la evidencia del delta es el diff de gate.sh (secciones %q):\n%s", got, diff)
	}

	// un commit que solo toca el estado de hoom
	rdEsperarSegundo()
	rcVeredicto(t, root)
	if _, err := finding.Add(root, "main", "low", "risk", "app.go", "un hallazgo", "reviewer@codex"); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A", "--", ".hoom/verdicts", ".hoom/findings", ".hoom/reviews")
	git(t, root, "commit", "-q", "-m", "solo estado de hoom: veredicto, hallazgo y registros")
	antes = cx.veces()
	registros := rdRegistrosEn(root)
	res, err, out := rdCorrer(t, "CA-483", root, Options{Provider: "codex", Spec: edrSpec, Delta: true})
	rdSinRevisar(t, "CA-483", "commit solo de estado", root, cx, antes, registros, res, err, out, rdSinCambios(gate[:12]))
}
