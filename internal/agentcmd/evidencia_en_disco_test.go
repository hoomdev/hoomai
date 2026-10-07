// Tests adversariales del spec .hoom/specs/evidencia-en-disco.md (CA-440..
// CA-443): la evidencia se mide en el disco, no en lo que git lista.
//
//   - CA-440: Take fotografia en Snapshot.Huellas el sha256 de cada archivo
//     bajo .hoom/{verdicts,findings,approvals}, tambien los que git ignora.
//   - CA-441: en la corrida de cualquier rol (el refutador incluido) una
//     evidencia que existia y cambio o desaparecio es manipulacion aunque
//     git no la vea; reescribirla igual no es cambio.
//   - CA-442: un archivo creado bajo .hoom/findings/ o .hoom/verdicts/ sin la
//     forma que escribe hoom es manipulacion; bajo .hoom/approvals/, toda
//     creacion lo es.
//   - CA-443: en una corrida ciega las mismas reglas valen para el arbol real.
//
// Las fotos son las de verdad (Take sobre un repo real) salvo donde el test
// dice que arma Huellas a mano: ahi se fija que la regla compara las fotos.
// Los sobres corren con un claude falso y con reloj; los que corren la CLI
// compilan hoom (se omiten con -short).
package agentcmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gittest"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// Frases del contrato.
const (
	edAppendOnly  = "la evidencia es append-only"
	edCambio      = "cambio durante el run"
	edDesaparecio = "desaparecio durante el run"
)

// edForma es el detalle de una creacion sin la forma de hoom bajo .hoom/<dir>.
func edForma(dir string) string {
	return "bajo .hoom/" + dir + " solo se crean archivos con la forma que escribe hoom"
}

// edReglas esconde de git los tres directorios de evidencia.
const edReglas = ".hoom/findings/\n.hoom/verdicts/\n.hoom/approvals/"

// Las formas que escribe hoom (las del spec), para validar fixtures.
var (
	edFormaHallazgo  = regexp.MustCompile(`^\.hoom/findings/[0-9]{8}T[0-9]{6}_[0-9a-f]{6}(\.res)?\.json$`)
	edFormaVeredicto = regexp.MustCompile(`^\.hoom/verdicts/[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}-[0-9]{2}-[0-9]{2}Z_[0-9a-f]{8}\.json$`)
)

func edSHA(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// edEnDisco recorre el disco de root y devuelve, para cada archivo regular
// bajo .hoom/{verdicts,findings,approvals}, su ruta relativa (con /) y el
// sha256 de su contenido: lo que Huellas tiene que decir.
func edEnDisco(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, d := range []string{"verdicts", "findings", "approvals"} {
		base := filepath.Join(root, ".hoom", d)
		fi, err := os.Stat(base)
		if err != nil || !fi.IsDir() {
			continue
		}
		err = filepath.WalkDir(base, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !de.Type().IsRegular() {
				return nil
			}
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			out[filepath.ToSlash(rel)] = edSHA(raw)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// edMinusculas normaliza los hex de un mapa de huellas (el contrato dice
// "el sha256", no en que caja).
func edMinusculas(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = strings.ToLower(v)
	}
	return out
}

func edClaves(m map[string]string) []string {
	var ks []string
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// edManipulaciones devuelve las violaciones de manipulacion en ruta.
func edManipulaciones(vs []Violation, ruta string) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Path == ruta && v.Rule == RuleTampering {
			out = append(out, v)
		}
	}
	return out
}

// edExigir exige UNA violacion de manipulacion en ruta (el piso no la
// duplica), con cada parte en su detalle y el gate marcado. La devuelve.
func edExigir(t *testing.T, ca, caso string, res ScopeResult, ruta string, partes ...string) Violation {
	t.Helper()
	ms := edManipulaciones(res.Violations, ruta)
	if len(ms) != 1 {
		t.Fatalf("%s: %s: %s es UNA violacion de manipulacion (hay %d): %s", ca, caso, ruta, len(ms), edLista(res.Violations))
	}
	for _, p := range partes {
		if !strings.Contains(ms[0].Detail, p) {
			t.Fatalf("%s: %s: el detalle de la manipulacion en %s dice %q: %q", ca, caso, ruta, p, ms[0].Detail)
		}
	}
	if !res.Tampering || res.OK {
		t.Fatalf("%s: %s: el gate queda marcado como manipulacion: %+v", ca, caso, res)
	}
	return ms[0]
}

// edSinManipulacion exige que ruta no tenga violacion de manipulacion.
func edSinManipulacion(t *testing.T, ca, caso string, res ScopeResult, ruta string) {
	t.Helper()
	if ms := edManipulaciones(res.Violations, ruta); len(ms) != 0 {
		t.Fatalf("%s: %s: %s es legitimo, no manipulacion: %+v", ca, caso, ruta, ms)
	}
}

func edLista(vs []Violation) string {
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

// edHallazgoDelGate exige que la violacion traiga el hallazgo del gate, y
// que ese hallazgo este en root, high y abierto.
func edHallazgoDelGate(t *testing.T, ca, caso, root string, v Violation) string {
	t.Helper()
	if v.FindingID == "" {
		t.Fatalf("%s: %s: la violacion en %s registra su hallazgo high (finding_id vacio)", ca, caso, v.Path)
	}
	items, _, err := finding.List(root, "main", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.ID != v.FindingID {
			continue
		}
		if it.Severity != "high" || it.Status != finding.StatusOpen {
			t.Fatalf("%s: %s: el hallazgo del gate %s es high y queda abierto: %s/%s", ca, caso, it.ID, it.Severity, it.Status)
		}
		return it.ID
	}
	t.Fatalf("%s: %s: el hallazgo del gate %s quedo en .hoom/findings/", ca, caso, v.FindingID)
	return ""
}

// edGitLista dice si git status nombra rel (sin los ignorados).
func edGitLista(t *testing.T, root, rel string) bool {
	t.Helper()
	cmd := exec.Command("git", "status", "--porcelain", "--untracked-files=all", "--", rel)
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	return strings.TrimSpace(string(out)) != ""
}

// edCommit commitea todo lo que git ve.
func edCommit(t *testing.T, root, msg string) {
	t.Helper()
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "--allow-empty", "-m", msg)
}

// edLeer lee rel de root.
func edLeer(t *testing.T, root, rel string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatal(err)
	}
	return raw
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

// edHallazgo registra un hallazgo y devuelve su ruta relativa.
func edHallazgo(t *testing.T, root, sev, desc string) (string, string) {
	t.Helper()
	f, err := finding.Add(root, "main", sev, "risk", "app.go", desc, "reviewer@codex")
	if err != nil {
		t.Fatal(err)
	}
	rel := ".hoom/findings/" + f.ID + ".json"
	if !edFormaHallazgo.MatchString(rel) {
		t.Fatalf("fixture: finding.Add escribe la forma de hoom: %s", rel)
	}
	return f.ID, rel
}

// edVeredicto escribe un veredicto con verdict.Write y devuelve su ruta.
func edVeredicto(t *testing.T, root, nota string) string {
	t.Helper()
	v := &verdict.Verdict{Schema: verdict.SchemaID, CreatedAt: time.Now().UTC(), Project: "demo",
		Verdict: "red", Notes: []string{nota}}
	p, err := verdict.Write(root, v)
	if err != nil {
		t.Fatal(err)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	rel, err := filepath.Rel(root, p)
	if err != nil {
		t.Fatal(err)
	}
	rel = filepath.ToSlash(rel)
	if !edFormaVeredicto.MatchString(rel) {
		t.Fatalf("fixture: verdict.Write escribe la forma de hoom: %s", rel)
	}
	return rel
}

// edAprobacion aprueba el spec demo y devuelve la ruta de su registro.
func edAprobacion(t *testing.T, root string) string {
	t.Helper()
	spec := specDemo(t, root)
	antes := map[string]bool{}
	for _, n := range filesIn(t, filepath.Join(root, ".hoom", "approvals")) {
		antes[n] = true
	}
	if _, _, err := approval.Approve(root, filepath.Join(root, spec)); err != nil {
		t.Fatal(err)
	}
	for _, n := range filesIn(t, filepath.Join(root, ".hoom", "approvals")) {
		if !antes[n] {
			return ".hoom/approvals/" + n
		}
	}
	t.Fatal("fixture: approval.Approve escribio su registro en .hoom/approvals/")
	return ""
}

// edEscondite es una forma de que git no vea una evidencia que ya existia.
type edEscondite struct {
	caso string
	// antes corre ANTES de crear la evidencia: la regla que la esconde.
	antes func(t *testing.T, root string)
	// sellar corre DESPUES de crearla (skip-worktree: commitearla y
	// pedirle a git que no mire su copia).
	sellar func(t *testing.T, root string, rutas []string)
}

func edEscondites() []edEscondite {
	return []edEscondite{
		{caso: ".gitignore de la raiz", antes: func(t *testing.T, root string) {
			write(t, root, ".gitignore", edReglas+"\n")
			edCommit(t, root, "la raiz ignora la evidencia")
		}},
		{caso: ".hoom/findings/.gitignore", antes: func(t *testing.T, root string) {
			write(t, root, ".hoom/findings/.gitignore", "*.json\n")
			edCommit(t, root, "findings ignora sus json")
		}},
		{caso: ".git/info/exclude", antes: func(t *testing.T, root string) {
			qcr1Anexar(t, qcr1Exclude(t, root), edReglas)
		}},
		{caso: "commiteada con skip-worktree", sellar: func(t *testing.T, root string, rutas []string) {
			git(t, root, append([]string{"add", "-f", "--"}, rutas...)...)
			git(t, root, "commit", "-q", "-m", "evidencia commiteada")
			git(t, root, append([]string{"update-index", "--skip-worktree", "--"}, rutas...)...)
		}},
	}
}

// edOculta dice si git no ve rel con el escondite e (despues del cambio).
func edOculta(t *testing.T, root, rel string, e edEscondite) bool {
	t.Helper()
	if e.sellar != nil {
		return !edGitLista(t, root, rel)
	}
	return qcr1Ignorado(t, root, rel)
}

// edRepo arma un proyecto: con findings.block_on: high si bloquea, y con lo
// que un rol ciego necesita (spec, test, .hoom/.gitignore) si ciego.
func edRepo(t *testing.T, bloquea, ciego bool) string {
	t.Helper()
	qcLimpiarEntorno(t)
	root := t.TempDir()
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	y := "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"
	if bloquea {
		y += "findings:\n  block_on: high\n"
	}
	write(t, root, "hoom.yaml", y)
	write(t, root, "app.go", "package app\n")
	if ciego {
		write(t, root, ".hoom/.gitignore", "cache/\nworktrees/\nruns/\nisolated/\n")
		write(t, root, "app_test.go", "package app\n\n// CA-1\n")
		specDemo(t, root)
	}
	edCommit(t, root, "inicial")
	return root
}

// ---------------------------------------------------------------- plantillas
//
// El gate registra un hallazgo high por cada manipulacion, y registrarlo
// mira git: cada violacion cuesta del orden de una foto. Por eso los tests
// de la tabla (variantes x roles x rutas) corren cada caso en paralelo y en
// su propia copia de una plantilla: el repo armado UNA vez por variante (git
// init, commits, reglas, la evidencia previa) y copiado para cada rol, que
// despues saca sus fotos, hace su corrida y corre su gate sobre su copia,
// igual que si hubiera armado el repo el mismo. Los subtests en paralelo no
// tocan el entorno (es del proceso): lo limpia una vez el test de arriba con
// qcLimpiarEntorno, y su limpieza corre recien cuando terminan todos.

// edCopiarArbol copia la plantilla (directorios, archivos regulares con su
// modo y su fecha, y symlinks tal cual; .git incluido) a un directorio
// temporal nuevo del test y lo devuelve. Cualquier otra entrada en la
// plantilla es un error del fixture.
//
// Y un repo git con el mantenimiento automatico prendido tambien (hallazgo
// 30abfc): despues de cada commit git crea y borra por su cuenta
// .git/objects/maintenance.lock, y la copia, que lista y despues mira, se
// cae cuando el lock ya no esta. La copia no tolera archivos que
// desaparecen: exige que el .git/config de cada repo que copia lo tenga
// apagado (gittest.ExigirMantenimientoApagado), y sin eso se niega siempre.
func edCopiarArbol(t *testing.T, plantilla string) string {
	t.Helper()
	destino := t.TempDir()
	if err := edCopiarArbolEn(plantilla, destino); err != nil {
		t.Fatalf("fixture: copiar la plantilla %s: %v", plantilla, err)
	}
	return destino
}

// edCopiarArbolEn es la copia de edCopiarArbol sobre destino (un directorio
// que ya existe y esta vacio), con su error.
func edCopiarArbolEn(plantilla, destino string) error {
	return filepath.WalkDir(plantilla, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(plantilla, p)
		if err != nil || rel == "." {
			return err
		}
		q := filepath.Join(destino, rel)
		fi, err := de.Info()
		if err != nil {
			return err
		}
		switch {
		case de.IsDir():
			if de.Name() == ".git" {
				if err := gittest.ExigirMantenimientoApagado(filepath.Join(p, "config")); err != nil {
					return err
				}
			}
			return os.Mkdir(q, fi.Mode().Perm())
		case de.Type()&fs.ModeSymlink != 0:
			l, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(l, q)
		case de.Type().IsRegular():
			raw, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err := os.WriteFile(q, raw, fi.Mode().Perm()); err != nil {
				return err
			}
			return os.Chtimes(q, fi.ModTime(), fi.ModTime())
		}
		return fmt.Errorf("plantilla: %s no es un directorio, un archivo regular ni un symlink (%s)", rel, fi.Mode())
	})
}

// edIgnorados dice, para cada ruta, si git la ignora en root: lo mismo que
// qcr1Ignorado (git check-ignore --no-index) ruta por ruta, en un solo
// proceso.
func edIgnorados(t *testing.T, root string, rels []string) map[string]bool {
	t.Helper()
	var in bytes.Buffer
	for _, r := range rels {
		in.WriteString(r)
		in.WriteByte(0)
	}
	cmd := exec.Command("git", "check-ignore", "-z", "--stdin", "--no-index")
	cmd.Dir = root
	cmd.Stdin = &in
	out, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); err != nil && (!ok || ee.ExitCode() != 1) {
		t.Fatalf("git check-ignore: %v", err)
	}
	ign := map[string]bool{}
	for _, r := range strings.Split(string(out), "\x00") {
		if r != "" {
			ign[r] = true
		}
	}
	return ign
}

// edCorrer corre el sobre con reloj.
func edCorrer(t *testing.T, ca, caso, root string, opt Options) Result {
	t.Helper()
	type corrida struct {
		res Result
		err error
	}
	ch := make(chan corrida, 1)
	go func() {
		res, err := Run(root, "main", opt, io.Discard)
		ch <- corrida{res, err}
	}()
	select {
	case c := <-ch:
		if c.err != nil {
			t.Fatalf("%s: %s: el sobre no se rompe: %v", ca, caso, c.err)
		}
		return c.res
	case <-time.After(qcr1Reloj):
		t.Fatalf("%s: %s: el sobre no volvio en %s", ca, caso, qcr1Reloj)
	}
	return Result{}
}

// edPreparado deja contenido en un archivo fuera del arbol, para que el
// provider falso lo copie con cat (sin depender de sed).
func edPreparado(t *testing.T, contenido []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "preparado")
	if err := os.WriteFile(p, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// edCopiar es el trozo de script que pisa destino con el archivo preparado.
func edCopiar(preparado, destino string) string {
	return "cat " + qcComillas(preparado) + " > " + qcComillas(destino) + "\n"
}

// edImplementar es el trabajo legitimo del writer.
const edImplementar = "printf 'package app // implementado\\n' > app.go\n"

// edSiguienteRoja corre un writer legitimo despues y exige que termine rojo
// en verify porque findings_open cuenta el hallazgo high del gate gid.
func edSiguienteRoja(t *testing.T, ca, caso, root, gid string) {
	t.Helper()
	fakeProvider(t, "claude", "printf 'package app // la corrida siguiente\\n' > app.go\nexit 0\n")
	res := edCorrer(t, ca, caso+", siguiente", root, Options{Role: "writer", Prompt: "implementa"})
	if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
		t.Fatalf("%s: %s: la corrida siguiente no queda verde: status=%s verdict=%s exit=%d scope=%+v",
			ca, caso, res.Status, res.Verdict, res.ExitCode, res.Scope)
	}
	if res.Stage != "verify" || res.Verdict != "red" || res.VerdictID == "" {
		t.Fatalf("%s: %s: la siguiente corta en verify con veredicto rojo: %+v", ca, caso, res)
	}
	if g := qcr2FindingsOpen(t, root, res.VerdictID); g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.Notes, gid) {
		t.Fatalf("%s: %s: %s falla por el hallazgo high del gate %s: %+v", ca, caso, finding.GateName, gid, g)
	}
}

// ================================================================ CA-440: la foto

// CA-440: Take fotografia en Huellas, para CADA archivo bajo
// .hoom/{verdicts,findings,approvals} en el disco, su ruta relativa (con /)
// y el sha256 de su contenido: los que git lista, los commiteados y los que
// git ignora por el .gitignore de la raiz, por un .hoom/findings/.gitignore
// o por .git/info/exclude; dotfiles, subdirectorios, vacios y binarios
// incluidos. Y nada mas: lo de al lado (.hoom/specs, .hoom/runs, un
// .hoom/findings-borrador.md, un .hoom/findings anidado en otro lado) no es
// evidencia.
func TestCA440_TakeFotografiaCadaEvidenciaDelDiscoConSuSHA256(t *testing.T) {
	qcLimpiarEntorno(t)
	root := repo(t)
	// reglas: la raiz esconde los veredictos, findings sus resoluciones,
	// info/exclude las aprobaciones
	write(t, root, ".gitignore", ".hoom/verdicts/\n")
	write(t, root, ".hoom/findings/.gitignore", "*.res.json\n")
	_, comiteado := edHallazgo(t, root, "high", "commiteado")
	edCommit(t, root, "reglas y un hallazgo commiteado")
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/approvals/")

	idVisible, visible := edHallazgo(t, root, "medium", "sin commitear, visible")
	resolucion := qcResRel(idVisible)
	write(t, root, resolucion, qcResolucionAMano(idVisible, finding.StatusRefuted))
	veredicto := edVeredicto(t, root, "escondido por la raiz")
	aprobacion := edAprobacion(t, root)
	otros := map[string]string{
		".hoom/findings/.oculto":                                      "un dotfile\n",
		".hoom/findings/sub/notas.txt":                                "en un subdirectorio\n",
		".hoom/findings/vacio.json":                                   "",
		".hoom/findings/binario.bin":                                  "\x00\x01\xff\xfe\x00",
		".hoom/findings/ñandú-ünïcode.json":                           "{\"ñ\":\"ü\"}\n",
		".hoom/verdicts/sub/profundo/x.json":                          "{}\n",
		".hoom/approvals/.gitignore":                                  "*\n",
		".hoom/specs/x.md":                                            "no es evidencia\n",
		".hoom/runs/20260101T000000_aa.jsonl":                         "no es evidencia\n",
		".hoom/reviews/20260922T150405_ab12cd.json":                   "{}\n",
		".hoom/items/precios.yaml":                                    "titulo: x\n",
		".hoom/findings-borrador.md":                                  "al lado, no adentro\n",
		".hoom/verdicts-viejos/x.json":                                "{}\n",
		"internal/.hoom/findings/20260101T000000_abcdef.json":         "{}\n",
		".hoom/isolated/q/.hoom/findings/20260101T000000_abcdef.json": "{}\n",
	}
	for rel, body := range otros {
		write(t, root, rel, body)
	}

	for _, rel := range []string{resolucion, veredicto, aprobacion, ".hoom/verdicts/sub/profundo/x.json"} {
		if !qcr1Ignorado(t, root, rel) {
			t.Fatalf("CA-440: fixture: git ignora %s", rel)
		}
	}
	if !qcr1Ignorado(t, root, ".hoom/approvals/.gitignore") {
		t.Fatalf("CA-440: fixture: git ignora .hoom/approvals/.gitignore")
	}

	quiere := edEnDisco(t, root)
	for _, rel := range []string{comiteado, visible, resolucion, veredicto, aprobacion, ".hoom/findings/.gitignore",
		".hoom/findings/.oculto", ".hoom/findings/sub/notas.txt", ".hoom/findings/vacio.json",
		".hoom/findings/binario.bin", ".hoom/findings/ñandú-ünïcode.json", ".hoom/verdicts/sub/profundo/x.json",
		".hoom/approvals/.gitignore"} {
		if _, ok := quiere[rel]; !ok {
			t.Fatalf("CA-440: fixture: %s esta en el disco: %v", rel, edClaves(quiere))
		}
	}
	if quiere[".hoom/findings/vacio.json"] != "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Fatalf("CA-440: fixture: el sha256 de un archivo vacio")
	}

	s := Take(root, "main")
	if s.Huellas == nil {
		t.Fatalf("CA-440: Take llena Huellas siempre: es nil")
	}
	got := edMinusculas(s.Huellas)
	if !reflect.DeepEqual(got, quiere) {
		var falta, sobra, distinto []string
		for k, v := range quiere {
			g, ok := got[k]
			if !ok {
				falta = append(falta, k)
			} else if g != v {
				distinto = append(distinto, k)
			}
		}
		for k := range got {
			if _, ok := quiere[k]; !ok {
				sobra = append(sobra, k)
			}
		}
		sort.Strings(falta)
		sort.Strings(sobra)
		sort.Strings(distinto)
		t.Fatalf("CA-440: Huellas es el sha256 de cada archivo de evidencia en el disco, y nada mas:\nfaltan: %v\nsobran: %v\nsha distinto: %v",
			falta, sobra, distinto)
	}
}

// CA-440: sin evidencia en el disco, Huellas es un mapa vacio, no nil: un
// repo sin .hoom/, uno con los directorios de evidencia vacios (y otras cosas
// en .hoom/), y uno donde solo hay evidencia en un arbol anidado.
func TestCA440_SinEvidenciaHuellasEsUnMapaVacio(t *testing.T) {
	for _, c := range []struct {
		caso  string
		armar func(t *testing.T, root string)
	}{
		{"sin .hoom", func(t *testing.T, root string) {}},
		{"directorios de evidencia vacios", func(t *testing.T, root string) {
			for _, d := range []string{"verdicts", "findings", "approvals", "findings/sub"} {
				if err := os.MkdirAll(filepath.Join(root, ".hoom", d), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			specDemo(t, root)
			write(t, root, ".hoom/runs/20260101T000000_aa.jsonl", "{}\n")
		}},
		{"evidencia solo en un arbol anidado", func(t *testing.T, root string) {
			write(t, root, "sub/.hoom/findings/20260101T000000_abcdef.json", "{}\n")
		}},
	} {
		t.Run(c.caso, func(t *testing.T) {
			qcLimpiarEntorno(t)
			root := repo(t)
			c.armar(t, root)
			s := Take(root, "main")
			if s.Huellas == nil || len(s.Huellas) != 0 {
				t.Fatalf("CA-440: %s: sin evidencia Huellas es un mapa vacio (no nil): %#v", c.caso, s.Huellas)
			}
		})
	}
}

// CA-440: "Take lo llena siempre", diga lo que diga git: en un directorio
// que no es un repo, la evidencia del disco igual queda fotografiada.
func TestCA440_FueraDeGitTakeIgualFotografiaElDisco(t *testing.T) {
	qcLimpiarEntorno(t)
	root := t.TempDir()
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n")
	write(t, root, ".hoom/findings/20260101T000000_abcdef.json", "{\"severity\":\"high\"}\n")
	write(t, root, ".hoom/verdicts/2026-01-01T00-00-00Z_0123abcd.json", "{}\n")
	s := Take(root, "main")
	if quiere := edEnDisco(t, root); !reflect.DeepEqual(edMinusculas(s.Huellas), quiere) {
		t.Fatalf("CA-440: fuera de git Huellas sigue siendo el disco: %#v, se esperaba %#v", s.Huellas, quiere)
	}
}

// CA-440 (propiedad): la huella es funcion del contenido y de nada mas. Para
// todo par de contenidos a y b de un hallazgo que git ignora: la huella es el
// sha256 de a; reescribir a con los mismos bytes no la mueve; escribir b la
// mueve si y solo si b != a.
func TestCA440_PropiedadLaHuellaEsElSHA256DelContenido(t *testing.T) {
	qcLimpiarEntorno(t)
	root := repo(t)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
	const ruta = ".hoom/findings/20260101T000000_abcdef.json"
	prop := func(a, b []byte, igual bool) bool {
		if igual {
			b = append([]byte(nil), a...)
		}
		write(t, root, ruta, string(a))
		s1 := Take(root, "main")
		write(t, root, ruta, string(a))
		later := time.Now().Add(time.Hour)
		if err := os.Chtimes(filepath.Join(root, ruta), later, later); err != nil {
			t.Fatal(err)
		}
		s2 := Take(root, "main")
		write(t, root, ruta, string(b))
		s3 := Take(root, "main")
		h1, h2, h3 := strings.ToLower(s1.Huellas[ruta]), strings.ToLower(s2.Huellas[ruta]), strings.ToLower(s3.Huellas[ruta])
		ok := h1 == edSHA(a) && h2 == h1 && h3 == edSHA(b) && (bytes.Equal(a, b) == (h3 == h1))
		if !ok {
			t.Logf("CA-440: a=%q b=%q: huellas %q %q %q", a, b, h1, h2, h3)
		}
		return ok
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 15}); err != nil {
		t.Fatalf("CA-440: la huella es el sha256 del contenido: %v", err)
	}
}

// ================================================================ CA-441: lo que existia

// edEvidencia es la evidencia que existe antes de la corrida y lo que la
// corrida le hace.
type edEvidencia struct {
	ruta  string
	frase string // edCambio | edDesaparecio
}

// edArmarEvidencia crea, con el escondite e, un hallazgo high, uno medium,
// la resolucion de un tercero, dos veredictos y una aprobacion. Devuelve
// root, las rutas, y el id del hallazgo high.
func edArmarEvidencia(t *testing.T, e edEscondite) (string, map[string]string, string) {
	t.Helper()
	qcLimpiarEntorno(t)
	return edArmarEvidenciaSinEntorno(t, e)
}

// edArmarEvidenciaSinEntorno es edArmarEvidencia sin limpiar el entorno, que
// es del proceso: para un subtest en paralelo cuyo test de arriba ya lo
// limpio.
func edArmarEvidenciaSinEntorno(t *testing.T, e edEscondite) (string, map[string]string, string) {
	t.Helper()
	root := repo(t)
	if e.antes != nil {
		e.antes(t, root)
	}
	alto, f1 := edHallazgo(t, root, "high", "el retry no respeta el backoff")
	_, f2 := edHallazgo(t, root, "medium", "otro")
	f3id, _ := edHallazgo(t, root, "high", "un tercero")
	r3 := qcResRel(f3id)
	write(t, root, r3, qcResolucionAMano(f3id, finding.StatusRefuted))
	v1 := edVeredicto(t, root, "uno")
	v2 := edVeredicto(t, root, "dos")
	if v1 == v2 {
		t.Fatalf("fixture: dos veredictos, dos archivos: %s", v1)
	}
	a1 := edAprobacion(t, root)
	rutas := map[string]string{"f1": f1, "f2": f2, "r3": r3, "v1": v1, "v2": v2, "a1": a1}
	if e.sellar != nil {
		var rs []string
		for _, r := range rutas {
			rs = append(rs, r)
		}
		sort.Strings(rs)
		e.sellar(t, root, rs)
	}
	return root, rutas, alto
}

// CA-441: para cada forma de esconder la evidencia de git (.gitignore de la
// raiz, .hoom/findings/.gitignore, .git/info/exclude, o commiteada con
// skip-worktree) y para CADA rol de la tabla, el refutador incluido: un
// hallazgo high bajado a low, una resolucion reescrita, un veredicto y una
// aprobacion editados son manipulacion "cambio durante el run"; un hallazgo y
// un veredicto borrados, "desaparecio durante el run". Una sola violacion
// por ruta, con su hallazgo high del gate. (Cada rol corre en paralelo, en
// su copia de la evidencia armada para el escondite.)
func TestCA441_EvidenciaQueCambiaODesapareceEsManipulacionAunqueGitNoLaVea(t *testing.T) {
	qcLimpiarEntorno(t)
	roles := agents.Roles()
	if len(roles) < 6 {
		t.Fatalf("CA-441: fixture: la tabla trae los roles de hoom (refutador incluido), trajo %d", len(roles))
	}
	for _, e := range edEscondites() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla, r, _ := edArmarEvidenciaSinEntorno(t, e)
			for _, rol := range roles {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := Take(root, "main")

					write(t, root, r["f1"], string(edBajarSeveridad(t, edLeer(t, root, r["f1"]))))
					if err := os.Remove(filepath.Join(root, r["f2"])); err != nil {
						t.Fatal(err)
					}
					f3 := strings.TrimSuffix(strings.TrimPrefix(r["r3"], ".hoom/findings/"), ".res.json")
					write(t, root, r["r3"], qcResolucionAMano(f3, finding.StatusCorrected))
					write(t, root, r["v1"], strings.Replace(string(edLeer(t, root, r["v1"])), "red", "green", 1))
					if err := os.Remove(filepath.Join(root, r["v2"])); err != nil {
						t.Fatal(err)
					}
					write(t, root, r["a1"], string(edLeer(t, root, r["a1"]))+" ")
					after := Take(root, "main")

					casos := []edEvidencia{
						{r["f1"], edCambio}, {r["f2"], edDesaparecio}, {r["r3"], edCambio},
						{r["v1"], edCambio}, {r["v2"], edDesaparecio}, {r["a1"], edCambio},
					}
					oculta := map[string]bool{}
					for _, c := range casos {
						oculta[c.ruta] = edOculta(t, root, c.ruta, e)
					}
					if !oculta[r["f1"]] || !oculta[r["r3"]] {
						t.Fatalf("CA-441: fixture: con %s git no ve los hallazgos: %v", e.caso, oculta)
					}

					res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
					for _, c := range casos {
						var v Violation
						if oculta[c.ruta] {
							v = edExigir(t, "CA-441", caso, res, c.ruta, edAppendOnly, c.ruta, c.frase)
						} else {
							// git la ve: el piso de hoy ya la marca y no se duplica
							v = edExigir(t, "CA-441", caso, res, c.ruta)
						}
						if v.FindingID == "" {
							t.Fatalf("CA-441: %s: la manipulacion en %s registra su hallazgo high del gate", caso, c.ruta)
						}
						if rol.Slug == "writer" {
							edHallazgoDelGate(t, "CA-441", caso, root, v)
						}
					}
				})
			}
		})
	}
}

// CA-441 (control, caso limite del spec): reescribir la evidencia con el
// mismo contenido byte a byte no es cambio — tampoco moverle la fecha, ni
// borrarla y volver a crearla igual. Para cada escondite y cada rol: sin
// violaciones.
func TestCA441_ReescribirLaEvidenciaConElMismoContenidoNoEsViolacion(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondites() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			root, r, _ := edArmarEvidenciaSinEntorno(t, e)
			before := Take(root, "main")
			later := time.Now().Add(2 * time.Hour)
			for _, k := range edClaves(r) {
				rel := r[k]
				raw := edLeer(t, root, rel)
				if k == "f2" {
					if err := os.Remove(filepath.Join(root, rel)); err != nil {
						t.Fatal(err)
					}
				}
				write(t, root, rel, string(raw))
				if err := os.Chtimes(filepath.Join(root, rel), later, later); err != nil {
					t.Fatal(err)
				}
			}
			after := Take(root, "main")
			for _, rol := range agents.Roles() {
				res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
				if len(res.Violations) != 0 || !res.OK || res.Tampering {
					t.Fatalf("CA-441: %s, rol %s: el mismo contenido no es un cambio: %s", e.caso, rol.Slug, edLista(res.Violations))
				}
			}
		})
	}
}

// CA-441 (casos limite): borrar y volver a crear con OTRO contenido es un
// cambio; renombrar un hallazgo es que el viejo desaparecio (el nuevo, con
// la forma de hoom, es una creacion legitima); y una evidencia sin la forma
// de hoom que ya estaba (un .hoom/findings/.gitignore previo) puede quedar
// intacta, pero no cambiar.
func TestCA441_CasosLimiteDeLoQueExistia(t *testing.T) {
	wr, err := agents.Lookup("writer")
	if err != nil {
		t.Fatal(err)
	}
	qcLimpiarEntorno(t)
	root := repo(t)
	write(t, root, ".hoom/findings/.gitignore", "*.json\n")
	edCommit(t, root, "findings ignora sus json")
	_, f1 := edHallazgo(t, root, "high", "uno")
	_, f2 := edHallazgo(t, root, "high", "dos")
	if !qcr1Ignorado(t, root, f1) {
		t.Fatalf("CA-441: fixture: git ignora %s", f1)
	}

	// el .gitignore previo, intacto: no es obra de la corrida
	before := Take(root, "main")
	res := Gate(root, "main", "", wr, before, Take(root, "main"), PolicyFor(nil, wr), nil)
	if len(res.Violations) != 0 || !res.OK {
		t.Fatalf("CA-441: una evidencia previa e intacta, aunque no tenga la forma de hoom, no es obra de la corrida: %s", edLista(res.Violations))
	}

	// borrar y crear con otro contenido; renombrar; cambiar el .gitignore previo
	before = Take(root, "main")
	raw := edLeer(t, root, f1)
	if err := os.Remove(filepath.Join(root, f1)); err != nil {
		t.Fatal(err)
	}
	write(t, root, f1, string(edBajarSeveridad(t, raw)))
	nuevo := ".hoom/findings/20991231T235959_abcdef.json"
	if err := os.Rename(filepath.Join(root, f2), filepath.Join(root, nuevo)); err != nil {
		t.Fatal(err)
	}
	write(t, root, ".hoom/findings/.gitignore", "*.json\n*.res.json\n")
	res = Gate(root, "main", "", wr, before, Take(root, "main"), PolicyFor(nil, wr), nil)
	edExigir(t, "CA-441", "borrado y recreado distinto", res, f1, edAppendOnly, edCambio)
	edExigir(t, "CA-441", "renombrado", res, f2, edAppendOnly, edDesaparecio)
	edSinManipulacion(t, "CA-441", "el nombre nuevo tiene la forma de hoom", res, nuevo)
	// el .gitignore previo esta commiteado y git ve su cambio: el piso de hoy
	// ya lo marca, y no se duplica
	edExigir(t, "CA-441", "el .gitignore previo que cambia", res, ".hoom/findings/.gitignore")
}

// CA-441 (propiedad, fotos con Huellas armadas a mano): la regla compara las
// dos fotos. Para toda evidencia previa y toda corrida que la deja igual, la
// cambia, la borra o crea otra (con o sin la forma de hoom, o una
// aprobacion), y la muestre git o no (Touched): lo que cambio o desaparecio
// es UNA manipulacion; lo que se creo sin forma, UNA manipulacion con el
// detalle de la forma; una aprobacion creada, UNA manipulacion; lo intacto y
// lo creado con forma, ninguna. Para el refutador y el writer.
//
// 25 semillas al azar por rol (lo que antes sacaba quick.Check con MaxCount
// 25: la propiedad solo usaba la semilla), cada una en paralelo y con su
// propia copia del repo donde el gate registra sus hallazgos. La semilla
// queda en el nombre del subtest y en cada falla.
func TestCA441_PropiedadElPisoComparaLasHuellasDeLasFotos(t *testing.T) {
	qcLimpiarEntorno(t)
	plantilla := repo(t) // donde el gate registra sus hallazgos (una copia por semilla)
	const semillasPorRol = 25
	azar := rand.New(rand.NewSource(time.Now().UnixNano()))
	previas := []string{
		".hoom/findings/20260101T000000_aaaaaa.json", ".hoom/findings/20260101T000001_bbbbbb.json",
		".hoom/findings/20260101T000000_aaaaaa.res.json", ".hoom/verdicts/2026-01-01T00-00-00Z_0123abcd.json",
		".hoom/verdicts/2026-01-02T00-00-00Z_89abcdef.json", ".hoom/approvals/demo_0123abcd.json",
		".hoom/findings/.gitignore",
	}
	conForma := []string{
		".hoom/findings/20260202T020202_c0ffee.json", ".hoom/findings/20260202T020203_0a1b2c.json",
		".hoom/verdicts/2026-02-02T02-02-02Z_deadbeef.json", ".hoom/verdicts/2026-02-03T02-02-02Z_00ff00ff.json",
	}
	resConForma := ".hoom/findings/20260202T020202_c0ffee.res.json"
	sinForma := []struct{ ruta, dir string }{
		{".hoom/findings/.gitignore2", "findings"}, {".hoom/findings/notas.txt", "findings"},
		{".hoom/findings/x.json", "findings"}, {".hoom/findings/sub/20260202T020202_c0ffee.json", "findings"},
		{".hoom/findings/20260202T020202_C0FFEE.json", "findings"}, {".hoom/verdicts/x.json", "verdicts"},
		{".hoom/verdicts/2026-02-02T02-02-02Z_deadbee.json", "verdicts"}, {".hoom/verdicts/.oculto", "verdicts"},
	}
	aprobaciones := []string{".hoom/approvals/otra_89abcdef.json", ".hoom/approvals/.gitignore"}

	for _, slug := range []string{finding.RolQueRefuta, "writer"} {
		rol, err := agents.Lookup(slug)
		if err != nil {
			t.Fatal(err)
		}
		// prop dice por que la semilla rompe la propiedad ("" si la cumple).
		prop := func(root string, semilla int64) string {
			rng := rand.New(rand.NewSource(semilla))
			before, after := snap(map[string]string{}), snap(map[string]string{})
			before.Huellas, after.Huellas = map[string]string{}, map[string]string{}
			type esperado struct {
				frases []string // nil = sin manipulacion
				regla  bool     // manipulacion sin detalle exigido
			}
			quiere := map[string]esperado{}
			for _, p := range previas {
				if rng.Intn(3) == 0 {
					continue
				}
				h := edSHA([]byte(p))
				before.Huellas[p], before.Evidence[p] = h, true
				visible := rng.Intn(2) == 0
				if visible {
					before.Touched[p] = "t0"
				}
				switch rng.Intn(4) {
				case 0, 1: // intacta
					after.Huellas[p], after.Evidence[p] = h, true
					if visible {
						after.Touched[p] = "t0"
					}
					quiere[p] = esperado{}
				case 2: // cambio
					after.Huellas[p], after.Evidence[p] = edSHA([]byte(p+"!")), true
					if visible {
						after.Touched[p] = "t1"
						quiere[p] = esperado{regla: true}
					} else {
						quiere[p] = esperado{frases: []string{edAppendOnly, p, edCambio}}
					}
				case 3: // desaparecio
					if visible {
						after.Touched[p] = "-"
						quiere[p] = esperado{regla: true}
					} else {
						quiere[p] = esperado{frases: []string{edAppendOnly, p, edDesaparecio}}
					}
				}
			}
			crear := func(p string) bool {
				h := edSHA([]byte("nuevo " + p))
				after.Huellas[p], after.Evidence[p] = h, true
				visible := rng.Intn(2) == 0
				if visible {
					after.Touched[p] = "t1"
				}
				return visible
			}
			for _, p := range conForma {
				if rng.Intn(2) == 0 {
					crear(p)
					quiere[p] = esperado{}
				}
			}
			if slug == finding.RolQueRefuta && rng.Intn(2) == 0 {
				crear(resConForma)
				quiere[resConForma] = esperado{}
			}
			for _, c := range sinForma {
				if rng.Intn(3) == 0 {
					crear(c.ruta)
					quiere[c.ruta] = esperado{frases: []string{edForma(c.dir), c.ruta}}
				}
			}
			for _, p := range aprobaciones {
				if rng.Intn(3) == 0 {
					crear(p)
					quiere[p] = esperado{regla: true}
				}
			}

			res := Gate(root, "main", "", rol, before, after, todo(), nil)
			hay := false
			for p, q := range quiere {
				ms := edManipulaciones(res.Violations, p)
				if q.frases == nil && !q.regla {
					if len(ms) != 0 {
						return fmt.Sprintf("%s no es manipulacion: %s", p, edLista(res.Violations))
					}
					continue
				}
				hay = true
				if len(ms) != 1 {
					return fmt.Sprintf("%s es UNA manipulacion (hay %d): %s", p, len(ms), edLista(res.Violations))
				}
				for _, f := range q.frases {
					if !strings.Contains(ms[0].Detail, f) {
						return fmt.Sprintf("el detalle en %s dice %q: %q", p, f, ms[0].Detail)
					}
				}
			}
			if hay != res.Tampering || hay == res.OK {
				return fmt.Sprintf("el gate queda marcado si y solo si hubo manipulacion: %+v", res)
			}
			return ""
		}
		for i := 0; i < semillasPorRol; i++ {
			semilla := azar.Int63()
			t.Run(fmt.Sprintf("%s, semilla %d", slug, semilla), func(t *testing.T) {
				t.Parallel()
				if porque := prop(edCopiarArbol(t, plantilla), semilla); porque != "" {
					t.Fatalf("CA-441: %s, semilla %d: el piso en el disco compara las huellas de las dos fotos: %s", slug, semilla, porque)
				}
			})
		}
	}
}

// CA-441 (guarda, decision del spec): una foto sin Huellas (nil, armada a
// mano) no aplica las reglas del disco: crear bajo .hoom/findings/ o
// .hoom/verdicts/ un archivo sin la forma de hoom sigue siendo lo de hoy
// (trabajo legitimo), por CheckScope y por el gate.
func TestCA441_FotosArmadasSinHuellasSiguenComoHoy(t *testing.T) {
	b, a := snap(nil), snap(map[string]string{
		".hoom/findings/.gitignore": "h1",
		".hoom/findings/notas.txt":  "h1",
		".hoom/verdicts/x.json":     "h1",
	})
	if b.Huellas != nil || a.Huellas != nil {
		t.Fatalf("CA-441: fixture: snap() arma fotos sin Huellas")
	}
	if res := CheckScope(b, a, todo()); len(res.Violations) != 0 || !res.OK {
		t.Fatalf("CA-441: sin Huellas no hay regla del disco (CheckScope): %s", edLista(res.Violations))
	}
	qcLimpiarEntorno(t)
	root := repo(t)
	ref, err := agents.Lookup(finding.RolQueRefuta)
	if err != nil {
		t.Fatal(err)
	}
	if res := Gate(root, "main", "", ref, b, a, todo(), nil); len(res.Violations) != 0 || !res.OK {
		t.Fatalf("CA-441: sin Huellas no hay regla del disco (Gate): %s", edLista(res.Violations))
	}
}

// ---------------------------------------------------------------- CA-441: el sobre

// CA-441 de punta a punta: 'hoom agent --role writer' en un proyecto con
// findings.block_on: high y un hallazgo high que git no ve (por la raiz, por
// .hoom/findings/.gitignore, por info/exclude, commiteado con skip-worktree,
// o porque el writer agrega la regla a info/exclude durante la corrida). El
// writer implementa y baja el hallazgo a low (o lo borra). El sobre NO
// termina verde: UNA manipulacion ("cambio/desaparecio durante el run"
// cuando git no lo vio nunca), su hallazgo high registrado, corte en scope
// sin veredicto; y un writer legitimo despues termina rojo en findings_open
// por ese hallazgo del gate.
func TestCA441_ElWriterQueBajaOBorraUnHallazgoQueGitNoVeNoTerminaVerde(t *testing.T) {
	type caso struct {
		nombre  string
		e       edEscondite
		durante string // regla que el writer agrega durante la corrida
		borrar  bool
	}
	var casos []caso
	for _, e := range edEscondites() {
		casos = append(casos, caso{nombre: "baja la severidad, " + e.caso, e: e})
	}
	casos = append(casos,
		caso{nombre: "baja la severidad, regla en info/exclude durante la corrida", durante: qcr1ExcluirEnScript(".hoom/findings/")},
		caso{nombre: "lo borra, .git/info/exclude", e: edEscondites()[2], borrar: true},
	)
	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			root := edRepo(t, true, false)
			if c.e.antes != nil {
				c.e.antes(t, root)
			}
			id, rel := edHallazgo(t, root, "high", "el retry no respeta el backoff")
			if c.e.sellar != nil {
				c.e.sellar(t, root, []string{rel})
			}
			var accion string
			if c.borrar {
				accion = "rm -f " + qcComillas(rel) + "\n"
			} else {
				accion = edCopiar(edPreparado(t, edBajarSeveridad(t, edLeer(t, root, rel))), rel)
			}
			fakeProvider(t, "claude", edImplementar+c.durante+accion+"exit 0\n")

			res := edCorrer(t, "CA-441", c.nombre, root, Options{Role: "writer", Prompt: "implementa"})
			if c.borrar {
				if existeEn(t, filepath.Join(root, rel)) {
					t.Fatalf("CA-441: fixture: el writer falso borro %s", rel)
				}
			} else if bytes.Contains(edLeer(t, root, rel), []byte(`"high"`)) {
				t.Fatalf("CA-441: fixture: el writer falso bajo la severidad de %s", rel)
			}
			if c.e.caso != "" && !edOculta(t, root, rel, c.e) || c.durante != "" && !qcr1Ignorado(t, root, rel) {
				t.Fatalf("CA-441: fixture: git no ve %s", rel)
			}
			if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
				t.Errorf("CA-441: %s: una evidencia editada que git no ve no deja verde al sobre: status=%s verdict=%s exit=%d stage=%s scope=%s",
					c.nombre, res.Status, res.Verdict, res.ExitCode, res.Stage, edLista(res.Scope.Violations))
			}
			frases := []string{edAppendOnly, rel, edCambio}
			if c.borrar {
				frases[2] = edDesaparecio
			}
			if c.durante != "" {
				// git lo vio antes de la corrida: el piso de hoy ya lo marca
				// con su detalle, y la regla nueva no lo duplica
				frases = nil
			}
			v := edExigir(t, "CA-441", c.nombre, res.Scope, rel, frases...)
			gid := edHallazgoDelGate(t, "CA-441", c.nombre, root, v)
			if gid == id {
				t.Fatalf("CA-441: fixture: el hallazgo del gate es otro que el editado")
			}
			if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status != "no-entregable" {
				t.Fatalf("CA-441: %s: la manipulacion corta el sobre en scope, no-entregable, exit 1, sin veredicto: %+v", c.nombre, res)
			}
			edSiguienteRoja(t, "CA-441", c.nombre, root, gid)
		})
	}
}

// CA-441: 'hoom agent --role refutador' tampoco edita evidencia: el
// refutador que baja a low un hallazgo high que git ignora es manipulacion,
// con su hallazgo high, y el sobre corta en scope.
func TestCA441_ElRefutadorQueBajaUnHallazgoIgnoradoEsManipulacion(t *testing.T) {
	root := edRepo(t, true, false)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
	_, rel := edHallazgo(t, root, "high", "el retry no respeta el backoff")
	fakeProvider(t, "claude", edCopiar(edPreparado(t, edBajarSeveridad(t, edLeer(t, root, rel))), rel)+"exit 0\n")

	res := edCorrer(t, "CA-441", "refutador", root, Options{Role: finding.RolQueRefuta, Prompt: "refuta los hallazgos abiertos"})
	if !qcr1Ignorado(t, root, rel) || bytes.Contains(edLeer(t, root, rel), []byte(`"high"`)) {
		t.Fatalf("CA-441: fixture: el refutador falso bajo %s, que git ignora", rel)
	}
	v := edExigir(t, "CA-441", "refutador", res.Scope, rel, edAppendOnly, rel, edCambio)
	edHallazgoDelGate(t, "CA-441", "refutador", root, v)
	if res.Stage != "scope" || res.ExitCode != 1 || res.Status == "entregable" {
		t.Fatalf("CA-441: el sobre del refutador corta en scope: %+v", res)
	}
}

// CA-441 (control de punta a punta): el writer que reescribe con los mismos
// bytes el hallazgo high ignorado no manipula nada: sin violaciones, el sobre
// llega a verify, que queda rojo por ese hallazgo high abierto (no por la
// corrida).
func TestCA441_ElWriterQueReescribeIgualNoManipula(t *testing.T) {
	root := edRepo(t, true, false)
	qcr1Anexar(t, qcr1Exclude(t, root), ".hoom/findings/")
	id, rel := edHallazgo(t, root, "high", "el retry no respeta el backoff")
	fakeProvider(t, "claude", edImplementar+edCopiar(edPreparado(t, edLeer(t, root, rel)), rel)+"touch "+qcComillas(rel)+"\nexit 0\n")

	res := edCorrer(t, "CA-441", "mismo contenido", root, Options{Role: "writer", Prompt: "implementa"})
	if len(res.Scope.Violations) != 0 || !res.Scope.OK || res.Scope.Tampering {
		t.Fatalf("CA-441: reescribir igual no es violacion: %s", edLista(res.Scope.Violations))
	}
	if res.Stage != "verify" || res.Verdict != "red" || res.VerdictID == "" {
		t.Fatalf("CA-441: el sobre llega a verify, rojo por el hallazgo high abierto: %+v", res)
	}
	if g := qcr2FindingsOpen(t, root, res.VerdictID); g == nil || g.Status != verdict.StatusFail || !strings.Contains(g.Notes, id) {
		t.Fatalf("CA-441: %s falla por el hallazgo original %s: %+v", finding.GateName, id, g)
	}
}

// ================================================================ CA-442: lo que se crea

// edSinFormaComun son creaciones sin la forma de hoom que no terminan en
// .res.json (esas tienen ademas la regla de quien-cierra-un-hallazgo). Las
// variantes de caja (hex en mayusculas, .JSON) llevan un id propio: en un
// disco que no distingue mayusculas (APFS por defecto) no pueden caer sobre
// un nombre valido creado en la misma corrida.
var edSinFormaComun = []struct{ ruta, dir string }{
	{".hoom/findings/.gitignore", "findings"},
	{".hoom/findings/.oculto", "findings"},
	{".hoom/findings/.json", "findings"},
	{".hoom/findings/notas.txt", "findings"},
	{".hoom/findings/x.json", "findings"},
	{".hoom/findings/f-nuevo.json", "findings"},
	{".hoom/findings/notas-ñandú.json", "findings"},
	{".hoom/findings/sub/20260101T000000_abcdef.json", "findings"},
	{".hoom/findings/20260101T000002_ABCDEF.json", "findings"},
	{".hoom/findings/20260101T000000_abcde.json", "findings"},
	{".hoom/findings/20260101T000000_abcdef0.json", "findings"},
	{".hoom/findings/20260101T000000_abcdeg.json", "findings"},
	{".hoom/findings/2026010T000000_abcdef.json", "findings"},
	{".hoom/findings/20260101-000000_abcdef.json", "findings"},
	{".hoom/findings/20260101T000000abcdef.json", "findings"},
	{".hoom/findings/20260101T000000_abcdef.json.bak", "findings"},
	{".hoom/findings/20260101T000001_abcdef.JSON", "findings"},
	{".hoom/findings/20260101T000000_abcdef.res.json.tmp", "findings"},
	{".hoom/findings/2026-01-01T00-00-00Z_0123abcd.json", "findings"}, // un id de veredicto
	{".hoom/verdicts/.gitignore", "verdicts"},
	{".hoom/verdicts/x.json", "verdicts"},
	{".hoom/verdicts/nuevo.json", "verdicts"},
	{".hoom/verdicts/notas.txt", "verdicts"},
	{".hoom/verdicts/sub/2026-01-01T00-00-00Z_0123abcd.json", "verdicts"},
	{".hoom/verdicts/2026-01-01T00-00-01Z_0123ABCD.json", "verdicts"},
	{".hoom/verdicts/2026-01-01T00-00-00Z_0123abc.json", "verdicts"},
	{".hoom/verdicts/2026-01-01T00-00-00Z_0123abcd0.json", "verdicts"},
	{".hoom/verdicts/2026-01-01T00-00-00_0123abcd.json", "verdicts"},
	{".hoom/verdicts/2026-01-01T00-00-00Z_0123abcd.res.json", "verdicts"},
	{".hoom/verdicts/20260101T000000_abcdef.json", "verdicts"}, // un id de hallazgo
	{".hoom/verdicts/2026-09-18T12-00-00Z_propio.json", "verdicts"},
}

// edEsconditeNuevo es una forma de que git no vea lo que la corrida crea.
type edEsconditeNuevo struct {
	caso   string
	armar  func(t *testing.T, root string)
	oculto bool
}

func edEscondenLoNuevo() []edEsconditeNuevo {
	return []edEsconditeNuevo{
		{"git lo ve", func(t *testing.T, root string) {}, false},
		{".git/info/exclude", func(t *testing.T, root string) { qcr1Anexar(t, qcr1Exclude(t, root), edReglas) }, true},
		{".gitignore de la raiz", func(t *testing.T, root string) {
			write(t, root, ".gitignore", edReglas+"\n")
			edCommit(t, root, "la raiz ignora la evidencia")
		}, true},
	}
}

// CA-442: para cada forma de esconder (o no) lo creado y para CADA rol, el
// refutador incluido: un archivo creado bajo .hoom/findings/ o
// .hoom/verdicts/ sin la forma que escribe hoom (un .gitignore, un dotfile,
// otro nombre, un subdirectorio, un id con hex en mayusculas o de otro
// largo, la forma del otro directorio) es UNA manipulacion con "bajo
// .hoom/<dir> solo se crean archivos con la forma que escribe hoom" y la
// ruta. En la misma corrida, lo creado con la forma de hoom (finding.Add,
// verdict.Write, o los mismos nombres escritos a mano) no es manipulacion.
// (Cada rol corre en paralelo, en su copia del repo armado para la variante.)
func TestCA442_UnArchivoCreadoSinLaFormaDeHoomEsManipulacion(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			e.armar(t, plantilla)
			for _, rol := range agents.Roles() {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					caso := e.caso + ", rol " + rol.Slug
					root := edCopiarArbol(t, plantilla)
					before := Take(root, "main")

					for _, c := range edSinFormaComun {
						write(t, root, c.ruta, "{\"lo\":\"creo el rol\"}\n")
					}
					_, hallazgo := edHallazgo(t, root, "medium", "un hallazgo nuevo")
					veredicto := edVeredicto(t, root, "un veredicto nuevo")
					aMano := []string{".hoom/findings/20260101T000000_abcdef.json", ".hoom/verdicts/2026-01-01T00-00-00Z_0123abcd.json"}
					write(t, root, aMano[0], `{"id":"20260101T000000_abcdef","created_at":"2026-01-01T00:00:00Z","severity":"low",`+
						`"lens":"risk","file":"app.go","description":"a mano","author":"rol@claude"}`+"\n")
					write(t, root, aMano[1], "{\"verdict\":\"green\"}\n")
					after := Take(root, "main")
					if e.oculto {
						var rutas []string
						for _, c := range edSinFormaComun {
							rutas = append(rutas, c.ruta)
						}
						ignoradas := edIgnorados(t, root, rutas)
						for _, c := range edSinFormaComun {
							if !ignoradas[c.ruta] {
								t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, c.ruta)
							}
						}
					}

					res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
					for _, c := range edSinFormaComun {
						v := edExigir(t, "CA-442", caso, res, c.ruta, edForma(c.dir), c.ruta)
						if v.FindingID == "" {
							t.Fatalf("CA-442: %s: la manipulacion en %s registra su hallazgo high del gate", caso, c.ruta)
						}
					}
					for _, ok := range append([]string{hallazgo, veredicto}, aMano...) {
						edSinManipulacion(t, "CA-442", caso, res, ok)
					}
				})
			}
		})
	}
}

// CA-442 (resoluciones): en la corrida del refutador una resolucion
// <id>.res.json con la forma de hoom es legitima, escondida o no; una con un
// id que hoom no escribe (hex en mayusculas, otro largo, un id de veredicto)
// es manipulacion de forma.
func TestCA442_ElRefutadorSoloCreaResolucionesConLaFormaDeHoom(t *testing.T) {
	ref, err := agents.Lookup(finding.RolQueRefuta)
	if err != nil {
		t.Fatal(err)
	}
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			root := repo(t)
			e.armar(t, root)
			id, _ := edHallazgo(t, root, "high", "a refutar")
			before := Take(root, "main")
			write(t, root, qcResRel(id), qcResolucionAMano(id, finding.StatusRefuted))
			malas := []string{
				".hoom/findings/20260101T000000_ABCDEF.res.json",
				".hoom/findings/20260101T000000_abcde.res.json",
				".hoom/findings/2026-01-01T00-00-00Z_0123abcd.res.json",
				".hoom/findings/" + id + ".res.res.json",
			}
			for _, m := range malas {
				write(t, root, m, qcResolucionAMano(id, finding.StatusRefuted))
			}
			res := Gate(root, "main", "", ref, before, Take(root, "main"), PolicyFor(nil, ref), nil)
			edSinManipulacion(t, "CA-442", e.caso+", resolucion con forma", res, qcResRel(id))
			for _, m := range malas {
				edExigir(t, "CA-442", e.caso, res, m, edForma("findings"), m)
			}
		})
	}
}

// CA-442: bajo .hoom/approvals/ toda creacion es manipulacion aunque git la
// ignore — la de approval.Approve (la forma real), un JSON cualquiera y un
// .gitignore — para cada rol, el refutador incluido. (Cada rol corre en
// paralelo, en su copia del repo armado para la variante.)
func TestCA442_CrearBajoApprovalsEsManipulacionAunqueGitLaIgnore(t *testing.T) {
	qcLimpiarEntorno(t)
	for _, e := range edEscondenLoNuevo() {
		t.Run(e.caso, func(t *testing.T) {
			t.Parallel()
			plantilla := repo(t)
			e.armar(t, plantilla)
			specDemo(t, plantilla) // el spec, antes: la corrida solo aprueba
			for _, rol := range agents.Roles() {
				t.Run("rol "+rol.Slug, func(t *testing.T) {
					t.Parallel()
					root := edCopiarArbol(t, plantilla)
					before := Take(root, "main")
					creadas := []string{edAprobacion(t, root), ".hoom/approvals/x.json", ".hoom/approvals/.gitignore"}
					write(t, root, creadas[1], "{}\n")
					write(t, root, creadas[2], "# nada\n")
					after := Take(root, "main")
					if e.oculto {
						for _, c := range creadas {
							if !qcr1Ignorado(t, root, c) {
								t.Fatalf("CA-442: fixture: con %s git ignora %s", e.caso, c)
							}
						}
					}
					res := Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil)
					for _, c := range creadas {
						v := edExigir(t, "CA-442", e.caso+", rol "+rol.Slug, res, c)
						if v.FindingID == "" {
							t.Fatalf("CA-442: %s, rol %s: la manipulacion en %s registra su hallazgo high", e.caso, rol.Slug, c)
						}
					}
				})
			}
		})
	}
}

// ---------------------------------------------------------------- CA-442: el sobre

// CA-442 de punta a punta: 'hoom agent --role writer' que implementa y
// ademas crea un .hoom/findings/.gitignore (que esconde los json, que no
// esconde nada, o escondido el mismo por info/exclude), un archivo sin forma
// bajo .hoom/verdicts/, o una aprobacion que git ignora: manipulacion, su
// hallazgo high queda en el disco (aunque el .gitignore se lo esconda a git),
// y el sobre corta en scope sin veredicto.
func TestCA442_ElWriterQueCreaArchivosSinFormaCortaEnScope(t *testing.T) {
	for _, c := range []struct {
		caso, ruta, script string
		previo             string // regla en info/exclude antes de la corrida
		frases             []string
	}{
		{".hoom/findings/.gitignore que esconde los json", ".hoom/findings/.gitignore",
			"mkdir -p .hoom/findings\nprintf '*.json\\n' > .hoom/findings/.gitignore\n", "",
			[]string{edForma("findings"), ".hoom/findings/.gitignore"}},
		{".hoom/findings/.gitignore que todavia no esconde nada", ".hoom/findings/.gitignore",
			"mkdir -p .hoom/findings\nprintf '# nada\\n' > .hoom/findings/.gitignore\n", "",
			[]string{edForma("findings"), ".hoom/findings/.gitignore"}},
		{".hoom/findings/.gitignore escondido por info/exclude", ".hoom/findings/.gitignore",
			"mkdir -p .hoom/findings\nprintf '*.json\\n' > .hoom/findings/.gitignore\n", ".hoom/findings/",
			[]string{edForma("findings"), ".hoom/findings/.gitignore"}},
		{".hoom/verdicts/notas.txt escondido", ".hoom/verdicts/notas.txt",
			"mkdir -p .hoom/verdicts\nprintf 'verde\\n' > .hoom/verdicts/notas.txt\n", ".hoom/verdicts/",
			[]string{edForma("verdicts"), ".hoom/verdicts/notas.txt"}},
		{"una aprobacion escondida", ".hoom/approvals/demo_0123abcd.json",
			"mkdir -p .hoom/approvals\nprintf '{}\\n' > .hoom/approvals/demo_0123abcd.json\n", ".hoom/approvals/", nil},
	} {
		t.Run(c.caso, func(t *testing.T) {
			root := edRepo(t, false, false)
			if c.previo != "" {
				qcr1Anexar(t, qcr1Exclude(t, root), c.previo)
			}
			fakeProvider(t, "claude", edImplementar+c.script+"exit 0\n")
			res := edCorrer(t, "CA-442", c.caso, root, Options{Role: "writer", Prompt: "implementa"})
			if !existeEn(t, filepath.Join(root, c.ruta)) {
				t.Fatalf("CA-442: fixture: el writer falso creo %s", c.ruta)
			}
			if c.previo != "" && !qcr1Ignorado(t, root, c.ruta) {
				t.Fatalf("CA-442: fixture: git ignora %s", c.ruta)
			}
			v := edExigir(t, "CA-442", c.caso, res.Scope, c.ruta, c.frases...)
			edHallazgoDelGate(t, "CA-442", c.caso, root, v)
			if res.Stage != "scope" || res.ExitCode != 1 || res.VerdictID != "" || res.Status != "no-entregable" {
				t.Fatalf("CA-442: %s: la manipulacion corta el sobre en scope, no-entregable, exit 1, sin veredicto: %+v", c.caso, res)
			}
		})
	}
}

// CA-442 (controles de punta a punta, casos limite del spec): el writer que
// corre el 'hoom finding add' real, o el 'hoom verify' real, dentro de su
// corrida crea un <id>.json o un veredicto con la forma de hoom: legitimo,
// escondido de git o no. El sobre entrega.
func TestCA442_HoomFindingAddYHoomVerifyDentroDeLaCorridaSiguenLegitimos(t *testing.T) {
	bin := qcHoom(t)
	for _, c := range []struct {
		caso, comando string
	}{
		{"finding add", qcComillas(bin) + " finding add --sev medium --lens risk --file app.go --author writer@claude 'de paso' >/dev/null 2>&1 || exit 7\n"},
		{"verify", qcComillas(bin) + " verify >/dev/null 2>&1\n"},
	} {
		for _, previo := range []string{"", edReglas} {
			nombre := c.caso
			if previo != "" {
				nombre += ", escondido por info/exclude"
			}
			t.Run(nombre, func(t *testing.T) {
				root := edRepo(t, false, false)
				if previo != "" {
					qcr1Anexar(t, qcr1Exclude(t, root), previo)
				}
				fakeProvider(t, "claude", edImplementar+c.comando+"exit 0\n")
				res := edCorrer(t, "CA-442", nombre, root, Options{Role: "writer", Prompt: "implementa"})

				var creados []string
				dir := "findings"
				if c.caso == "verify" {
					dir = "verdicts"
				}
				for _, n := range filesIn(t, filepath.Join(root, ".hoom", dir)) {
					creados = append(creados, ".hoom/"+dir+"/"+n)
				}
				minimo := 1
				if c.caso == "verify" {
					minimo = 2 // el del rol y el del sobre
				}
				if len(creados) < minimo {
					t.Fatalf("CA-442: %s: fixture: la CLI real dejo su archivo en .hoom/%s/: %v (sobre %+v)", nombre, dir, creados, res)
				}
				for _, p := range creados {
					if !edFormaHallazgo.MatchString(p) && !edFormaVeredicto.MatchString(p) {
						t.Fatalf("CA-442: %s: fixture: la CLI real escribe la forma de hoom: %s", nombre, p)
					}
					edSinManipulacion(t, "CA-442", nombre, res.Scope, p)
				}
				if len(res.Scope.Violations) != 0 || !res.Scope.OK || res.Scope.Tampering {
					t.Fatalf("CA-442: %s: lo que crea la CLI de hoom es legitimo: %s", nombre, edLista(res.Scope.Violations))
				}
				if res.Status != "entregable" || res.ExitCode != 0 || res.Verdict != "green" {
					t.Fatalf("CA-442: %s: el writer que implementa y corre hoom entrega: %+v", nombre, res)
				}
			})
		}
	}
}

// ================================================================ CA-443: la corrida ciega

// edRealDesdeLaCuarentena es la ruta, desde la cuarentena, a rel en el arbol
// real.
func edRealDesdeLaCuarentena(rel string) string { return qcr2Real + rel }

// CA-443: 'hoom agent --role test-writer' corre ciego en .hoom/isolated y,
// ademas de escribir su test, toca la evidencia del arbol REAL con una ruta
// relativa: baja a low un hallazgo high que git ignora, lo borra, crea un
// .hoom/findings/.gitignore o una aprobacion que git ignora, o edita un
// hallazgo commiteado que git si ve. En cada caso: manipulacion en esa ruta
// (UNA; puede venir ademas la fuga), con el detalle del contrato cuando git
// no la ve, y el sobre NO termina verde. Y la corrida siguiente de un writer
// legitimo queda roja en findings_open por el hallazgo del gate.
func TestCA443_ElRolCiegoQueTocaLaEvidenciaDelArbolRealEsManipulacion(t *testing.T) {
	type caso struct {
		nombre    string
		previo    string // regla en info/exclude antes de la corrida
		commitear bool   // el hallazgo high queda commiteado (git lo ve)
		ruta      func(id string) string
		script    func(t *testing.T, root, rel string) string
		frases    func(rel string) []string
		siguiente bool
	}
	hallazgo := func(id string) string { return ".hoom/findings/" + id + ".json" }
	bajar := func(t *testing.T, root, rel string) string {
		return edCopiar(edPreparado(t, edBajarSeveridad(t, edLeer(t, root, rel))), edRealDesdeLaCuarentena(rel))
	}
	for _, c := range []caso{
		{nombre: "baja un hallazgo que git ignora", previo: ".hoom/findings/", ruta: hallazgo, script: bajar,
			frases: func(rel string) []string { return []string{edAppendOnly, rel, edCambio} }, siguiente: true},
		{nombre: "borra un hallazgo que git ignora", previo: ".hoom/findings/", ruta: hallazgo,
			script: func(t *testing.T, root, rel string) string {
				return "rm -f " + qcComillas(edRealDesdeLaCuarentena(rel)) + "\n"
			},
			frases: func(rel string) []string { return []string{edAppendOnly, rel, edDesaparecio} }},
		{nombre: "crea .hoom/findings/.gitignore escondido", previo: ".hoom/findings/",
			ruta: func(string) string { return ".hoom/findings/.gitignore" },
			script: func(t *testing.T, root, rel string) string {
				return "printf '*.json\\n' > " + qcComillas(edRealDesdeLaCuarentena(rel)) + "\n"
			},
			frases: func(rel string) []string { return []string{edForma("findings"), rel} }},
		{nombre: "crea una aprobacion escondida", previo: ".hoom/approvals/",
			ruta: func(string) string { return ".hoom/approvals/demo_0123abcd.json" },
			script: func(t *testing.T, root, rel string) string {
				return "mkdir -p " + qcComillas(edRealDesdeLaCuarentena(".hoom/approvals")) + "\n" +
					"printf '{}\\n' > " + qcComillas(edRealDesdeLaCuarentena(rel)) + "\n"
			},
			frases: func(string) []string { return nil }},
		{nombre: "edita un hallazgo commiteado que git ve", commitear: true, ruta: hallazgo, script: bajar,
			frases: func(string) []string { return nil }},
	} {
		t.Run(c.nombre, func(t *testing.T) {
			root := edRepo(t, true, true)
			if c.previo != "" {
				qcr1Anexar(t, qcr1Exclude(t, root), c.previo)
			}
			id, _ := edHallazgo(t, root, "high", "el retry no respeta el backoff")
			if c.commitear {
				edCommit(t, root, "hallazgo alto commiteado")
			}
			rel := c.ruta(id)
			donde := filepath.Join(t.TempDir(), "pwd.txt")
			fakeProvider(t, "claude", qcr2Donde(donde)+qcr2TestLegitimo+c.script(t, root, rel)+"exit 0\n")

			res := edCorrer(t, "CA-443", c.nombre, root, Options{Role: "test-writer", Prompt: "escribi los tests"})
			qcr2EnLaCuarentena(t, c.nombre, donde)
			if res.Isolation == nil {
				t.Fatalf("CA-443: %s: fixture: el test-writer corrio ciego: %+v", c.nombre, res)
			}
			if c.previo != "" && !qcr1Ignorado(t, root, rel) {
				t.Fatalf("CA-443: %s: fixture: git ignora %s en el arbol real", c.nombre, rel)
			}
			if res.Status == "entregable" || res.Verdict == "green" || res.ExitCode == 0 {
				t.Errorf("CA-443: %s: tocar la evidencia del arbol real no deja verde al sobre ciego: status=%s verdict=%s exit=%d stage=%s scope=%s",
					c.nombre, res.Status, res.Verdict, res.ExitCode, res.Stage, edLista(res.Scope.Violations))
			}
			v := edExigir(t, "CA-443", c.nombre, res.Scope, rel, c.frases(rel)...)
			gid := edHallazgoDelGate(t, "CA-443", c.nombre, root, v)
			if c.siguiente {
				edSiguienteRoja(t, "CA-443", c.nombre, root, gid)
			}
		})
	}
}

// CA-443 (control): la corrida ciega que solo escribe su test no se vuelve
// manipulacion porque el arbol real tenga evidencia que git ignora y un
// .hoom/findings/.gitignore previo: intactos, no son obra de la corrida. El
// sobre entrega verde y el test viaja.
func TestCA443_ElRolCiegoQueSoloEscribeSuTestSigueEntregando(t *testing.T) {
	root := edRepo(t, false, true)
	write(t, root, ".hoom/findings/.gitignore", "*.json\n")
	edCommit(t, root, "findings ignora sus json")
	_, rel := edHallazgo(t, root, "low", "previo e ignorado")
	if !qcr1Ignorado(t, root, rel) {
		t.Fatalf("CA-443: fixture: git ignora %s", rel)
	}
	fakeProvider(t, "claude", qcr2TestLegitimo+"exit 0\n")

	res := edCorrer(t, "CA-443", "control", root, Options{Role: "test-writer", Prompt: "escribi los tests"})
	if res.Isolation == nil {
		t.Fatalf("CA-443: fixture: el test-writer corrio ciego: %+v", res)
	}
	if len(res.Scope.Violations) != 0 || !res.Scope.OK || res.Scope.Tampering {
		t.Fatalf("CA-443: la evidencia previa e intacta del arbol real no es obra de la corrida: %s", edLista(res.Scope.Violations))
	}
	if res.Status != "entregable" || res.Verdict != "green" || res.ExitCode != 0 {
		t.Fatalf("CA-443: sin violaciones el sobre ciego entrega verde: %+v", res)
	}
	if !existeEn(t, filepath.Join(root, "nuevo_test.go")) {
		t.Fatalf("CA-443: el test legitimo viaja al arbol real: %+v", res.Isolation)
	}
}
