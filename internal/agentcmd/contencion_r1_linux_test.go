//go:build linux

// Ronda 1 del test-writer de contencion-del-writer: regresiones de la review
// de codex sobre el sobre (agentcmd) en Linux con encierro: la administracion
// del principal (fab30f), Windows sin job simulado con ReemplazarAdopcion
// (c5790e y siguientes; aclaracion 22) y los gates del sobre del test-writer
// con su vista (a8ef36; aclaracion 23, que corrige la 10). Sin bwrap fallan,
// no se saltan (aclaracion 1).
package agentcmd

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// ---------------------------------------------------------------- punto 1

// CA-678 (id fab30f): el CLI encerrado del writer, en su tarea (un worktree
// enlazado), no cambia, no reemplaza ni borra ninguna entrada de
// encierro.AdministracionPrincipal que exista en la raiz del directorio comun
// (el index y el HEAD del principal incluidos), no mueve el HEAD del
// principal ni escribe su index con git, no crea una administracion nueva
// bajo <comun>/worktrees/ ni escribe la de otra tarea; su commit en
// hoom/demo funciona.
func TestR1_fab30f_CA678_ElWriterEncerradoNoMueveAlPrincipal(t *testing.T) {
	const ca = "CA-678"
	root, wt, salida := cdwlProyecto(t, ca, "")
	if err := taskcmd.Start(root, "otra", "main"); err != nil {
		t.Fatalf("fixture: la otra tarea: %v", err)
	}
	comun := cdwComun(t, wt)
	sha := cdwGitOut(t, root, "rev-parse", "HEAD")
	cdwEscribir(t, filepath.Join(comun, "ORIG_HEAD"), sha+"\n")
	cdwEscribir(t, filepath.Join(comun, "FETCH_HEAD"), sha+"\t\tbranch 'main' of x\n")
	cdwEscribir(t, filepath.Join(comun, "SQUASH_MSG"), "squash\n")
	piezas := qcComillas(filepath.Join(salida, "piezas"))
	antes := map[string]string{}
	script := ""
	for i, e := range encierro.AdministracionPrincipal {
		p := filepath.Join(comun, filepath.FromSlash(e))
		st, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			antes[e] = "dir"
			script += "( printf x > " + qcComillas(filepath.Join(p, "r1-colado")) + " ) 2>/dev/null && echo " + qcComillas("ESCRIBIO "+e+" (adentro)") + " >> " + piezas + "\n"
			continue
		}
		antes[e] = cdwLeerOVacio(p)
		tmp := "\"$TMPDIR/r1-reemplazo-" + strconv.Itoa(i) + "\""
		script += "( printf 'colado\\n' >> " + qcComillas(p) + " ) 2>/dev/null && echo " + qcComillas("ESCRIBIO "+e+" (agrega)") + " >> " + piezas + "\n" +
			"printf 'otro\\n' > " + tmp + " && mv -f " + tmp + " " + qcComillas(p) + " 2>/dev/null && echo " + qcComillas("ESCRIBIO "+e+" (reemplaza)") + " >> " + piezas + "\n" +
			"rm -f " + qcComillas(p) + " 2>/dev/null; [ -e " + qcComillas(p) + " ] || echo " + qcComillas("ESCRIBIO "+e+" (borra)") + " >> " + piezas + "\n"
	}
	for _, e := range []string{"HEAD", "index", "ORIG_HEAD", "FETCH_HEAD", "SQUASH_MSG"} {
		if _, ok := antes[e]; !ok {
			t.Fatalf("%s: fixture: %s existe y esta en AdministracionPrincipal (%q)", ca, e, encierro.AdministracionPrincipal)
		}
	}
	otraHead := cdwLeerOVacio(filepath.Join(comun, "worktrees", "otra", "HEAD"))
	script += "git -C " + qcComillas(root) + " symbolic-ref HEAD refs/heads/r1-otra 2>/dev/null && echo 'ESCRIBIO HEAD (symbolic-ref)' >> " + piezas + "\n" +
		"git -C " + qcComillas(root) + " update-index --chmod=+x app.go 2>/dev/null && echo 'ESCRIBIO index (update-index)' >> " + piezas + "\n" +
		"mkdir " + qcComillas(filepath.Join(comun, "worktrees", "r1-nueva")) + " 2>/dev/null && echo 'ESCRIBIO worktrees/r1-nueva' >> " + piezas + "\n" +
		"( printf x >> " + qcComillas(filepath.Join(comun, "worktrees", "otra", "HEAD")) + " ) 2>/dev/null && echo 'ESCRIBIO worktrees/otra/HEAD' >> " + piezas + "\n" +
		ar1Codigo + "git add app.go && git commit -q -m 'r1 del writer encerrado' && echo commit-ok >> " + piezas + "\nexit 0\n"
	head := cdwGitOut(t, wt, "rev-parse", "HEAD")
	// el index del principal se compara por lo que tiene (git puede refrescar
	// sus datos de stat desde afuera del encierro): ningun modo ni contenido
	// cambia.
	indexAntes := cdwGitOut(t, root, "ls-files", "-s")
	res, out := cdwCorrer(t, root, "writer", aeaTarea, script, Options{})
	got := cdwlLeer(t, filepath.Join(salida, "piezas"))
	if strings.Contains(got, "ESCRIBIO") {
		t.Errorf("%s: el writer encerrado no cambia la administracion del principal:\n%s", ca, got)
	}
	for e, a := range antes {
		p := filepath.Join(comun, filepath.FromSlash(e))
		if a == "dir" {
			if cdwExiste(filepath.Join(p, "r1-colado")) {
				t.Errorf("%s: %s/ no se escribe", ca, e)
			}
			continue
		}
		if e == "index" {
			if !cdwExiste(p) || cdwGitOut(t, root, "ls-files", "-s") != indexAntes {
				t.Errorf("%s: el index del principal sigue como estaba", ca)
			}
			continue
		}
		if !cdwExiste(p) || cdwLeerOVacio(p) != a {
			t.Errorf("%s: %s del principal sigue como estaba: %q", ca, e, cdwLeerOVacio(p))
		}
	}
	if cdwExiste(filepath.Join(comun, "worktrees", "r1-nueva")) {
		t.Errorf("%s: no se crea una administracion nueva bajo worktrees/", ca)
	}
	if cdwLeerOVacio(filepath.Join(comun, "worktrees", "otra", "HEAD")) != otraHead {
		t.Errorf("%s: la administracion de la otra tarea no se escribe", ca)
	}
	if !strings.Contains(got, "commit-ok") || cdwGitOut(t, wt, "rev-parse", "HEAD") == head {
		t.Fatalf("%s: el commit del writer encerrado en hoom/%s funciona:\n%s\n%+v\n%s", ca, aeaTarea, got, res, out)
	}
}

// ---------------------------------------------------------------- punto 2

// r1InfoSinJob es el nivel real de un lanzamiento en Windows sin job.
var r1InfoSinJob = encierro.Info{Nivel: encierro.NivelNinguno,
	Falta: []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC}, Motivo: encierro.MotivoSinJob}

// CA-676 y CA-677 (ids c5790e, 5380e1, 15f87d, c1983c, 185514; aclaracion
// 22): con ReemplazarAdopcion simulando Windows sin job y exigir parcial (que
// el nivel de este Linux alcanza al empezar), hoom mata el arbol del CLI
// enseguida y el sobre del writer cierra como una negativa de exigir con el
// nivel real: en run, exit 1, con NotaExigir de ninguno (MotivoSinJob), la
// clave encierro del registro con ese nivel, y el CLI falso no sigue vivo.
// Con exigir no, el mismo caso corre y el sidecar de su run dice ninguno.
func TestR1_c5790e_CA676_CA677_SinJobElSobreSeNiegaConElNivelReal(t *testing.T) {
	const ca = "CA-676"
	cdwlEncierro(t, ca)
	if got, mata := encierro.TrasAdoptar("windows", false, "parcial"); !mata || got.Nivel != encierro.NivelNinguno || got.Motivo != encierro.MotivoSinJob {
		t.Fatalf("%s: fixture: TrasAdoptar dice que sin job y con exigir parcial se mata: %+v %v", ca, got, mata)
	}
	restaurar := encierro.ReemplazarAdopcion("windows", func(int) bool { return false })
	defer restaurar()

	root, _ := cdwProyecto(t, cdwExigir("parcial"))
	tok := cdwlToken(t)
	desde := time.Now()
	res, out := cdwCorrer(t, root, "writer", aeaTarea, "exec sh -c 'while :; do sleep 1; done' "+tok+"\n", Options{Timeout: 2 * time.Minute})
	if d := time.Since(desde); d > 45*time.Second {
		t.Fatalf("%s: sin job el arbol muere enseguida, no al vencer el timeout: %s", ca, d)
	}
	want := encierro.NotaExigir(r1InfoSinJob, "parcial", "writer")
	if res.Stage != "run" || res.ExitCode != 1 || res.Note != want {
		t.Fatalf("%s: el sobre cierra en run, exit 1, con %q: %+v\n%s", ca, want, res, out)
	}
	got, _ := cdwClave(t, root, res.EnvelopeID)
	cdwExigeInfo(t, ca, "el registro del sobre sin job", got, r1InfoSinJob)
	cdwlNadieVivo(t, ca, "sin job con exigir parcial", tok)

	// exigir no: corre, con el nivel real en el sidecar
	root, wt := cdwProyecto(t, cdwSeccion)
	res, out = cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
	if res.RunID == "" || res.Stage == "run" {
		t.Fatalf("%s: sin job y con exigir no, el sobre corre: %+v\n%s", ca, res, out)
	}
	meta, _ := cdwMetaCruda(t, res.RunID, root, wt)["encierro"].(map[string]any)
	cdwExigeInfo(t, "CA-677", "el sidecar del run sin job", meta, r1InfoSinJob)
}

// ---------------------------------------------------------------- punto 7

// CA-683 y CA-695 (id a8ef36; aclaracion 23, que corrige la 10): los gates del
// verify del sobre del test-writer corren con la vista del test-writer: uno
// que intenta escribir la implementacion del arbol de la tarea (app.go, que
// existe fuera del arbol ciego) o crear un archivo en el no deja nada en el
// disco, y su TMPDIR sigue escribible dentro de la cache verify. Control: el
// mismo gate del sobre del writer escribe el arbol, como hoy.
func TestR1_a8ef36_CA683_CA695_LosGatesDelTestWriterNoEscribenLaImplementacion(t *testing.T) {
	const ca = "CA-683"
	cdwlEncierro(t, ca)
	for _, rol := range []string{"test-writer", "writer"} {
		root, wt := cdwProyecto(t, cdwSeccion)
		app := filepath.Join(wt, "app.go")
		nuevo := filepath.Join(wt, "r1_del_gate.go")
		r1CambiarGate(t, root, wt, "printf r1-del-gate >> "+app+" 2>/dev/null; printf x > "+nuevo+" 2>/dev/null; printf x > $TMPDIR/r1-gate-"+rol+"; true")
		res, out := cdwCorrer(t, root, rol, aeaTarea, cdwlTrabajo(rol)+"exit 0\n", Options{})
		if res.VerdictID == "" {
			t.Fatalf("%s: %s: fixture: el verify del sobre corrio: %+v\n%s", ca, rol, res, out)
		}
		cache := encierro.Cache(root, aeaTarea, encierro.QuienVerify)
		if !cdwHayAdentro(cache, "r1-gate-"+rol) {
			t.Errorf("CA-682: %s: la cache verify sigue escribible para el gate (TMPDIR adentro de %s)", rol, cache)
		}
		if rol == "test-writer" {
			if strings.Contains(cdwLeerOVacio(app), "r1-del-gate") || cdwExiste(nuevo) {
				t.Fatalf("%s: el gate del sobre del test-writer no escribe la implementacion del arbol de la tarea: app.go %q, nuevo %v\n%+v\n%s",
					ca, cdwLeerOVacio(app), cdwExiste(nuevo), res, out)
			}
			continue
		}
		if !strings.Contains(cdwLeerOVacio(app), "r1-del-gate") || !cdwExiste(nuevo) {
			t.Fatalf("%s: control: el gate del sobre del writer escribe el arbol, como hoy: app.go %q, nuevo %v", ca, cdwLeerOVacio(app), cdwExiste(nuevo))
		}
	}
}
