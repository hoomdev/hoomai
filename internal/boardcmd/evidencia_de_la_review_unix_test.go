//go:build !windows

// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md
// (CA-486, CA-487) en el tablero, con scripts sh al frente del PATH o en la
// config del repo:
//
//   - CA-486: con un git que se anuncia 2.39.0, un merge limpio de la base
//     despues de la review no se separa: para el tablero el codigo cambio.
//   - CA-487: el merge sintetico no deja rastro ni corre programas: despues
//     de un --delta que separo y de un tablero que evaluo esa tarjeta, el
//     repositorio tiene los mismos objetos, refs e indice, y el arbol solo
//     cambia en lo que toda review escribe bajo .hoom/; el driver de merge
//     que la rama elige por .gitattributes nunca corre. Y una falla de git en
//     el merge sintetico no cae al diff crudo: la review no cuenta.
package boardcmd

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/reviewcmd"
)

// edrGitReal es la ruta absoluta del git de verdad, buscada antes de poner
// un git falso al frente del PATH.
func edrGitReal(t *testing.T) string {
	t.Helper()
	p, err := exec.LookPath("git")
	if err != nil {
		t.Skip("sin git en el PATH")
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// edrGitFalso pone al frente del PATH un git que corre casos sobre su
// subcomando (despues de las opciones globales) y para todo lo demas es el
// git de verdad.
func edrGitFalso(t *testing.T, real, casos string) {
	t.Helper()
	bin := t.TempDir()
	s := "#!/bin/sh\nsaltar=0\nfor a in \"$@\"; do\n" +
		"\tif [ \"$saltar\" = 1 ]; then saltar=0; continue; fi\n" +
		"\tcase \"$a\" in\n" +
		"\t-c|-C|--git-dir|--work-tree|--namespace|--config-env) saltar=1 ;;\n" +
		casos +
		"\t-*) ;;\n\t*) break ;;\n\tesac\ndone\n" +
		"exec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

// ---------------------------------------------------------------- CA-486

// CA-486: "un git que se anuncia anterior a 2.40 da lo mismo": despues de la
// review de la tarjeta, main avanza solo con documentacion y la rama la
// integra sin conflicto; con un git 2.39.0 ese merge no se separa y para el
// tablero el codigo cambio: la tarjeta vuelve a Review con el motivo y el
// siguiente paso del delta.
func TestCA486_ConGitAnteriorA240ElMergeDeLaBaseDevuelveLaTarjetaAReview(t *testing.T) {
	real := edrGitReal(t)
	tj := rdGrande(t)
	r1 := tj.completa(t, "20261027T160000_c0mp1a")
	rdCuenta(t, "CA-486 (fixture)", "completa", tj.card(t), r1.ID)
	edrMainAvanza(t, tj, "main avanza con docs", map[string]string{"README.md": "# demo\n\nprosa de main\n", "docs/guia.md": "# guia\n"})
	edrIntegrar(t, tj)
	tj.verde(t)
	// sin derivar la tarjeta antes del git falso: el tablero guarda las colas
	// por arbol, hasta y HEAD (CA-427). Con un git que separa, este mismo caso
	// sigue con su review: TestCA485_SoloIntegrarLaBaseNoDevuelveLaTarjetaAReview.
	edrGitFalso(t, real, "\t--version|version) echo 'git version 2.39.0'; exit 0 ;;\n")
	if out, err := exec.Command("git", "version").Output(); err != nil || strings.TrimSpace(string(out)) != "git version 2.39.0" {
		t.Fatalf("CA-486: fixture: el git del PATH se anuncia 2.39.0: %q %v", out, err)
	}
	edrPideElDelta(t, "CA-486", "git 2.39.0", tj.card(t), r1.ID, r1.Hasta)
}

// ---------------------------------------------------------------- CA-487

// edrGrande es rdGrande con el mantenimiento automatico de git apagado
// desde el init (gittest.ApagarMantenimiento): ningun git lanzado por un
// commit toca .git/objects mientras el test lo fotografia.
func edrGrande(t *testing.T) *rdTarjeta {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bdGit(t, root, "init", "-b", "main")
	gittest.ApagarMantenimiento(t, root)
	bdGit(t, root, "config", "user.email", "test@hoom.dev")
	bdGit(t, root, "config", "user.name", "hoom test")
	bdEscribir(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\ngates:\n  test:\n    required: true\n    cmd: \"true\"\nfindings:\n  block_on: high\n")
	bdEscribir(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	bdEscribir(t, root, "app.go", "package app\n")
	bdGit(t, root, "add", "-A")
	bdGit(t, root, "commit", "-q", "-m", "inicial")
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	wt := bdWorktree(t, root, bdSlug)
	bdEscribir(t, wt, bdSpec, bdSpecTexto("- CA-1: algo. [verifica: true]", true))
	bdEscribir(t, wt, "precios.go", rdCodigo(450, "Precio"))
	bdAprobar(t, wt, bdSlug)
	bdCommitear(t, wt, "spec y codigo")
	tj := &rdTarjeta{root: root, wt: wt, mb: bdGit(t, wt, "merge-base", "main", "HEAD"), at: time.Minute}
	tj.verde(t)
	bdCol(t, "CA-487 (fixture)", tj.card(t), ColReview)
	return tj
}

// edrCompartido es un archivo de 20 lineas con la linea n cambiada por v
// (n = 0: ninguna).
func edrCompartido(n int, v string) string {
	var b strings.Builder
	for i := 1; i <= 20; i++ {
		if i == n {
			b.WriteString(v + "\n")
			continue
		}
		fmt.Fprintf(&b, "linea %02d del archivo compartido\n", i)
	}
	return b.String()
}

// edrRastro es lo que el merge sintetico no puede cambiar: los objetos
// (count-objects y cada archivo de .git/objects con su tamano), las refs (y
// los HEAD y pseudo-refs: todo .git fuera de objects/ y de los indices), las
// entradas de cada indice, y el arbol de trabajo (bdFoto). Los bytes de cada
// indice van aparte.
type edrRastro struct {
	objetos       string
	refs          string
	git           map[string]string
	entradas      map[string]string
	bytesIndice   map[string]string
	arbol         map[string]string
	indices, gits []string
}

func edrFotoRastro(t *testing.T, tj *rdTarjeta) edrRastro {
	t.Helper()
	gitDir := filepath.Join(tj.root, ".git")
	r := edrRastro{git: map[string]string{}, entradas: map[string]string{}, bytesIndice: map[string]string{}}
	r.indices = []string{filepath.Join(gitDir, "index"), filepath.Join(gitDir, "worktrees", bdSlug, "index")}
	var objs []string
	err := filepath.WalkDir(gitDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(gitDir, p)
		info, err := d.Info()
		if err != nil {
			return err
		}
		if strings.HasPrefix(rel, "objects"+string(filepath.Separator)) {
			objs = append(objs, fmt.Sprintf("%s %d", rel, info.Size()))
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		suma := fmt.Sprintf("%x", sha256.Sum256(raw))
		for _, idx := range r.indices {
			if p == idx {
				r.bytesIndice[rel] = suma
				return nil
			}
		}
		r.git[rel] = suma
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(objs)
	r.objetos = bdGit(t, tj.root, "count-objects", "-v") + "\n" + strings.Join(objs, "\n")
	r.refs = bdGit(t, tj.root, "for-each-ref", "--format=%(refname) %(objectname)") + "\n" +
		"HEAD del proyecto " + bdGit(t, tj.root, "rev-parse", "HEAD") + " " + bdGit(t, tj.root, "symbolic-ref", "-q", "HEAD") + "\n" +
		"HEAD de la tarea " + bdGit(t, tj.wt, "rev-parse", "HEAD") + " " + bdGit(t, tj.wt, "symbolic-ref", "-q", "HEAD")
	r.entradas["proyecto"] = bdGit(t, tj.root, "ls-files", "--stage", "-z")
	r.entradas["tarea"] = bdGit(t, tj.wt, "ls-files", "--stage", "-z")
	r.arbol = bdFoto(t, tj.root)
	return r
}

// edrLoQueEscribeUnaReview: lo unico que el arbol de trabajo puede ganar
// entre las dos fotos, en el proyecto o en el worktree de la tarea: el
// registro de la review, su sobre y su narracion.
var edrLoQueEscribeUnaReview = []string{".hoom/reviews/", ".hoom/envelopes/", ".hoom/runs/"}

func edrPermitido(rel string) bool {
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, ".hoom/worktrees/"+bdSlug+"/")
	for _, p := range edrLoQueEscribeUnaReview {
		if strings.HasPrefix(rel, p) {
			return true
		}
	}
	return false
}

func edrMismoRastro(t *testing.T, ca string, antes, despues edrRastro) {
	t.Helper()
	if antes.objetos != despues.objetos {
		t.Fatalf("%s: el repositorio tiene los mismos objetos que antes:\nantes:\n%s\ndespues:\n%s", ca, antes.objetos, despues.objetos)
	}
	if antes.refs != despues.refs {
		t.Fatalf("%s: el repositorio tiene las mismas refs y HEAD que antes:\nantes:\n%s\ndespues:\n%s", ca, antes.refs, despues.refs)
	}
	for rel, v := range antes.git {
		if w, ok := despues.git[rel]; !ok || w != v {
			t.Fatalf("%s: .git/%s (refs, HEAD, pseudo-refs, config) no cambia: antes %q, despues %q", ca, rel, v, w)
		}
	}
	for rel := range despues.git {
		if _, ok := antes.git[rel]; !ok {
			t.Fatalf("%s: en .git no aparece nada fuera de objects/ (refs, ORIG_HEAD, MERGE_HEAD, AUTO_MERGE): aparecio .git/%s", ca, rel)
		}
	}
	for k, v := range antes.entradas {
		if despues.entradas[k] != v {
			t.Fatalf("%s: el indice de %s tiene las mismas entradas (git ls-files --stage):\nantes %q\ndespues %q", ca, k, v, despues.entradas[k])
		}
	}
	for rel, v := range antes.bytesIndice {
		if despues.bytesIndice[rel] != v {
			// las entradas son las mismas (arriba); los bytes no se esconden
			t.Logf("%s: POSIBLE HALLAZGO: los bytes de .git/%s cambiaron con las mismas entradas (%s -> %s)", ca, rel, v[:12], despues.bytesIndice[rel])
		}
	}
	for rel, v := range antes.arbol {
		if w, ok := despues.arbol[rel]; !ok {
			t.Fatalf("%s: el arbol de trabajo no pierde nada: desaparecio %s", ca, rel)
		} else if w != v && !edrPermitido(rel) {
			t.Fatalf("%s: el arbol de trabajo solo cambia en lo que toda review escribe bajo .hoom/ (%q): cambio %s", ca, edrLoQueEscribeUnaReview, rel)
		}
	}
	for rel := range despues.arbol {
		if _, ok := antes.arbol[rel]; !ok && !edrPermitido(rel) {
			t.Fatalf("%s: el arbol de trabajo solo cambia en lo que toda review escribe bajo .hoom/ (%q): aparecio %s", ca, edrLoQueEscribeUnaReview, rel)
		}
	}
}

// edrDriver deja fuera del arbol un driver de merge que anota cada vez que
// corre, y devuelve la linea de comando para merge.<drv>.driver y la marca.
func edrDriver(t *testing.T) (cmd, marca string) {
	t.Helper()
	dir := t.TempDir()
	marca = filepath.Join(dir, "corrio-el-driver")
	script := filepath.Join(dir, "driver.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho \"$4\" >> '"+marca+"'\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return "'" + script + "' %O %A %B %P", marca
}

// CA-487: "el merge sintetico no deja rastro ni corre programas". La rama
// declara `* merge=drv` en su .gitattributes y cambia compartido.txt (eso lo
// cubre la review R1, de 4 lentes, con hoom review); despues commitea X, main
// cambia otra linea de compartido.txt (Y), la rama integra main sin conflicto
// (un merge real, sin el driver configurado todavia) y commitea Z. Recien
// entonces la config del repo define merge.drv.driver como un programa que
// deja una marca (en un merge real de D con I corre: guarda del fixture).
// Despues de un tablero que evaluo la tarjeta (el merge sintetico de D con
// I), de un --delta que separo (base_integrada = I) y de otro tablero, el
// repositorio tiene los mismos objetos, refs e indice que antes, el arbol
// solo cambia en lo que toda review escribe bajo .hoom/, y la marca nunca
// aparecio.
func TestCA487_ElMergeSinteticoNoDejaRastroNiCorreProgramas(t *testing.T) {
	r1Codex(t)
	tj := edrGrande(t)
	edrMainAvanza(t, tj, "main trae compartido.txt", map[string]string{"compartido.txt": edrCompartido(0, "")})
	edrIntegrar(t, tj)
	bdEscribir(t, tj.wt, ".gitattributes", "* merge=drv\n")
	bdEscribir(t, tj.wt, "compartido.txt", edrCompartido(2, "linea 02 cambiada por la tarea"))
	bdCommitear(t, tj.wt, "la tarea elige su driver de merge y cambia compartido.txt")
	tj.verde(t)
	r1, out := tj.review(t, "CA-487", false)
	r1Completa(t, "CA-487", "450 lineas", r1, out, reviewcmd.Lentes)
	d := r1.Hasta

	r1Esperar()
	bdEscribir(t, tj.wt, "x.go", "package app\n\n// MARCA-X\nfunc X() {}\n")
	bdCommitear(t, tj.wt, "X")
	i := edrMainAvanza(t, tj, "Y", map[string]string{"compartido.txt": edrCompartido(18, "linea 18 cambiada por main"), "y.go": "package app\n\nfunc Y() {}\n"})
	edrIntegrar(t, tj)
	bdEscribir(t, tj.wt, "z.go", "package app\n\n// MARCA-Z\nfunc Z() {}\n")
	bdCommitear(t, tj.wt, "Z")
	tj.verde(t)
	if mb := bdGit(t, tj.wt, "merge-base", "main", "HEAD"); mb != i {
		t.Fatalf("CA-487: fixture: la base integrada es la punta de main: %s", mb)
	}

	driver, marca := edrDriver(t)
	// guarda: en un merge REAL de D con I, en un clon con la misma config,
	// el driver que la rama eligio corre
	clon := filepath.Join(t.TempDir(), "clon")
	bdGit(t, tj.root, "clone", "-q", "--no-hardlinks", tj.root, clon)
	gittest.ApagarMantenimiento(t, clon)
	bdGit(t, clon, "config", "user.email", "test@hoom.dev")
	bdGit(t, clon, "config", "user.name", "hoom test")
	bdGit(t, clon, "config", "merge.drv.driver", driver)
	bdGit(t, clon, "checkout", "-q", "--detach", d)
	bdGit(t, clon, "merge", "-q", "--no-edit", i)
	if _, err := os.Stat(marca); err != nil {
		t.Fatalf("CA-487: fixture: en un merge real de D con I el driver de la rama corre (deja %s): %v", marca, err)
	}
	if err := os.Remove(marca); err != nil {
		t.Fatal(err)
	}
	bdGit(t, tj.root, "config", "merge.drv.driver", driver)
	bdGit(t, tj.root, "config", "merge.drv.name", "driver de prueba")

	antes := edrFotoRastro(t, tj)
	if _, err := Build(tj.root, "main", "high", time.Now().UTC()); err != nil {
		t.Fatalf("CA-487: el tablero se arma: %v", err)
	}
	edrPideElDelta(t, "CA-487", "X y Z despues de la review", tj.card(t), r1.RecordID, d)

	r1Esperar()
	res, err := edrRunDelta(t, tj)
	if err != nil || res.Status != "revisado" || res.Cobertura != reviewcmd.CoberturaDelta {
		t.Fatalf("CA-487: el --delta revisa: %+v %v", res, err)
	}
	if m := edrJSONMapa(t, res); m["base_integrada"] != i {
		t.Fatalf("CA-487: el --delta separo la base integrada (base_integrada %s): %v", i[:12], m["base_integrada"])
	}
	if _, err := Build(tj.root, "main", "high", time.Now().UTC()); err != nil {
		t.Fatalf("CA-487: el tablero se arma: %v", err)
	}
	tj.card(t)
	despues := edrFotoRastro(t, tj)

	if raw, err := os.ReadFile(marca); err == nil {
		t.Fatalf("CA-487: el merge sintetico no corre el driver de merge que la rama elige por .gitattributes: corrio para %q", raw)
	}
	edrMismoRastro(t, "CA-487", antes, despues)
}

// edrRunDelta corre `hoom review --task precios --spec ... --delta` con
// reloj, sin commitear lo que deja.
func edrRunDelta(t *testing.T, tj *rdTarjeta) (reviewcmd.Result, error) {
	t.Helper()
	type resultado struct {
		res reviewcmd.Result
		err error
	}
	ch := make(chan resultado, 1)
	go func() {
		var out strings.Builder
		res, err := reviewcmd.Run(tj.root, "main", reviewcmd.Options{Provider: "codex", Task: bdSlug, Spec: bdSpec, Delta: true}, &out)
		if err != nil {
			err = fmt.Errorf("%v\n%s", err, out.String())
		}
		ch <- resultado{res, err}
	}()
	select {
	case r := <-ch:
		return r.res, r.err
	case <-time.After(90 * time.Second):
		t.Fatal("hoom review --delta no volvio en 90 s")
	}
	return reviewcmd.Result{}, nil
}

// CA-487 (falla cerrada en el tablero: "cualquier otra falla de git es un
// error ... y nunca se cae al diff crudo"). Despues de la review de la
// tarjeta, main avanza solo con documentacion y la rama la integra sin
// conflicto. Con un git que falla en merge-tree (fatal, exit 128: no es un
// conflicto), el tablero no puede saber que cambio despues de la review: esa
// review no cuenta (con el diff crudo seria "solo documentacion" y contaria).
func TestCA487_UnaFallaDeGitEnElMergeSinteticoNoCuentaLaReview(t *testing.T) {
	real := edrGitReal(t)
	tj := rdGrande(t)
	r1 := tj.completa(t, "20261027T160000_c0mp1a")
	edrMainAvanza(t, tj, "main avanza con docs", map[string]string{"README.md": "# demo\n\nprosa de main\n"})
	edrIntegrar(t, tj)
	tj.verde(t)
	// sin derivar la tarjeta antes del git falso: el tablero guarda las colas
	// por arbol, hasta y HEAD (CA-427). Con un git sano, este mismo caso sigue
	// con su review: TestCA485_SoloIntegrarLaBaseNoDevuelveLaTarjetaAReview.
	marca := filepath.Join(t.TempDir(), "merge-tree-fallo")
	edrGitFalso(t, real, "\tmerge-tree) echo x >> '"+marca+"'; echo 'fatal: FALLA-SIMULADA-EN-MERGE-TREE' >&2; exit 128 ;;\n")
	c := tj.card(t)
	bdCol(t, "CA-487 (merge-tree falla)", c, ColReview)
	if c.Evidence.ReviewID == r1.ID {
		t.Fatalf("CA-487: con el merge sintetico fallando la review no cuenta (falla cerrada, nunca el diff crudo): %+v", c.Evidence)
	}
	if _, err := os.Stat(marca); err != nil {
		t.Fatalf("CA-487: la falla es la de git en el merge sintetico (git merge-tree no corrio): %v", err)
	}
}

// edrJSONMapa es v como lo escribe --json, leido como mapa (sin depender de
// los campos que el tipo tenga hoy).
func edrJSONMapa(t *testing.T, v any) map[string]any {
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
