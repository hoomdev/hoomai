// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre la
// review (reviewcmd), en cualquier sistema: el nivel en su registro y su
// salida (CA-674), la politica antes de la primera pasada (CA-675, CA-676),
// y git-ajeno y config-de-agentes alrededor de cada pasada (CA-685, CA-689).
// Los CLIs falsos los arma fakeProvider (cliFalso: drenan su stdin); este
// archivo no escribe shebangs.
package reviewcmd

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/providers"
)

const cdwSlug = "precios"

// cdwHome deja un $HOME del test y la config global de git fuera.
func cdwHome(t *testing.T) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(home, ".gitconfig-del-test"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, k := range []string{"HOOM_ROLE", "HOOM_RUN", "HOOM_PROVIDER", "HOOM_TASK", "CODEX_HOME"} {
		t.Setenv(k, "")
		os.Unsetenv(k)
	}
	return home
}

// cdwRepo es cbRepo con extra al final del hoom.yaml (la seccion encierro,
// por ejemplo).
func cdwRepo(t *testing.T, extra string) string {
	t.Helper()
	cdwHome(t)
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n"+extra)
	write(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "inicial")
	return root
}

// cdwTarea es la tarea precios con un cambio commiteado y un writer
// observado en codex: revisa claude (cruzada).
func cdwTarea(t *testing.T, extra string) (root, wt string, opt Options) {
	t.Helper()
	root = cdwRepo(t, extra)
	wt = cbTarea(t, root, cdwSlug)
	cbMeta(t, root, wt, "20261113T090000_cdw001", "codex", "writer")
	return root, wt, Options{Task: cdwSlug, Lens: "risk", Provider: "claude"}
}

func cdwRevisar(t *testing.T, root string, opt Options) (Result, string, error) {
	t.Helper()
	var out bytes.Buffer
	res, err := Run(root, "main", opt, &out)
	return res, out.String(), err
}

func cdwProv(t *testing.T, n string) providers.Provider {
	t.Helper()
	p, err := providers.Lookup(n)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// cdwRegistroCrudo es el registro id de .hoom/reviews/ de cualquiera de los
// dirs, crudo.
func cdwRegistroCrudo(t *testing.T, id string, dirs ...string) map[string]any {
	t.Helper()
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(d, ".hoom", RecordsDir, id+".json"))
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("el registro %s es JSON: %v", id, err)
		}
		return m
	}
	t.Fatalf("el registro %s esta en .hoom/%s/ de %v", id, RecordsDir, dirs)
	return nil
}

func cdwSinRegistros(t *testing.T, ca, caso string, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if got := rcRegistros(t, d); len(got) != 0 {
			t.Fatalf("%s: %s: sin registro en .hoom/%s/ de %s: %v", ca, caso, RecordsDir, d, got)
		}
	}
}

func cdwExiste(p string) bool { _, err := os.Lstat(p); return err == nil }

// cdwExigeInfo compara la clave cruda encierro con want (el mecanismo con
// ninguno no se fija, aclaracion 7).
func cdwExigeInfo(t *testing.T, ca, que string, got any, want encierro.Info) {
	t.Helper()
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("%s: %s: lleva la clave encierro: %v", ca, que, got)
	}
	if m["nivel"] != want.Nivel {
		t.Fatalf("%s: %s: nivel %v; quiero %s", ca, que, m["nivel"], want.Nivel)
	}
	falta, ok := m["falta"].([]any)
	if !ok || len(falta) != len(want.Falta) {
		t.Fatalf("%s: %s: falta %v (nunca null); quiero %q", ca, que, m["falta"], want.Falta)
	}
	for i, f := range falta {
		if f != want.Falta[i] {
			t.Fatalf("%s: %s: falta %v; quiero %q", ca, que, falta, want.Falta)
		}
	}
	if mot, hay := m["motivo"]; (want.Motivo == "") == hay || (hay && mot != want.Motivo) {
		t.Fatalf("%s: %s: motivo %v (%v); quiero %q", ca, que, mot, hay, want.Motivo)
	}
	if want.Nivel != encierro.NivelNinguno && m["mecanismo"] != want.Mecanismo {
		t.Fatalf("%s: %s: mecanismo %v; quiero %s", ca, que, m["mecanismo"], want.Mecanismo)
	}
}

// cdwRegistroDelSobre es el registro crudo del sobre de la review.
func cdwRegistroDelSobre(t *testing.T, root string) map[string]any {
	t.Helper()
	for _, r := range envelope.List(root) {
		if r.Role != "reviewer" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(root, ".hoom", envelope.DirName, r.ID+".json"))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	t.Fatalf("la review deja el registro de su sobre en .hoom/%s/", envelope.DirName)
	return nil
}

// ---------------------------------------------------------------- CA-674

// CA-674: con la seccion, el registro de la review (Record.Encierro y la
// clave cruda), el registro de su sobre y el sidecar de cada run llevan el
// nivel; debajo de completo, la salida trae el aviso con el rol reviewer.
func TestCA674_LaReviewLlevaElNivel(t *testing.T) {
	const ca = "CA-674"
	for _, s := range []*encierro.Sonda{nil, {Bwrap: true, Espacio: true, LandlockABI: 5}} {
		func() {
			if s != nil {
				restaurar := encierro.ReemplazarSonda(*s)
				defer restaurar()
			}
			root, wt, opt := cdwTarea(t, "encierro:\n  exigir: no\n")
			want := encierro.Nivel(true, cdwProv(t, "claude"))
			fakeProvider(t, "claude", "exit 0\n")
			res, out, err := cdwRevisar(t, root, opt)
			if err != nil || res.RecordID == "" {
				t.Fatalf("%s: la review corre: %v %+v\n%s", ca, err, res, out)
			}
			r := hbRecordDe(t, res.RecordID, wt, root)
			if r.Encierro == nil || r.Encierro.Nivel != want.Nivel {
				t.Fatalf("%s: Record.Encierro es el nivel %s: %+v", ca, want.Nivel, r.Encierro)
			}
			cdwExigeInfo(t, ca, "el registro de la review", cdwRegistroCrudo(t, res.RecordID, wt, root)["encierro"], want)
			cdwExigeInfo(t, ca, "el registro del sobre de la review", cdwRegistroDelSobre(t, root)["encierro"], want)
			for _, p := range res.Passes {
				if p.RunID == "" {
					continue
				}
				cdwExigeInfo(t, ca, "el sidecar de la pasada "+p.Lens, cdwMetaCruda(t, p.RunID, root, wt)["encierro"], want)
			}
			aviso := encierro.Aviso(want, "reviewer")
			if want.Nivel != encierro.NivelCompleto && !strings.Contains(out, aviso) {
				t.Fatalf("%s: debajo de completo la salida de la review trae %q:\n%s", ca, aviso, out)
			}
			if want.Nivel == encierro.NivelCompleto && strings.Contains(out, "aviso: encierro") {
				t.Fatalf("%s: con completo no hay aviso:\n%s", ca, out)
			}
		}()
	}
}

func hbRecordDe(t *testing.T, id string, dirs ...string) Record {
	t.Helper()
	for _, d := range dirs {
		recs, _ := Records(d)
		for _, r := range recs {
			if r.ID == id {
				return r
			}
		}
	}
	t.Fatalf("Records de %v trae el registro %s", dirs, id)
	return Record{}
}

func cdwMetaCruda(t *testing.T, id string, dirs ...string) map[string]any {
	t.Helper()
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(d, ".hoom", "runs", id+".meta.json"))
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	}
	t.Fatalf("el sidecar %s esta en .hoom/runs/ de %v", id, dirs)
	return nil
}

// CA-674: sin la seccion, ni el registro de la review ni el de su sobre
// llevan la clave.
func TestCA674_SinLaSeccionLaReviewNoLlevaLaClave(t *testing.T) {
	const ca = "CA-674"
	root, wt, opt := cdwTarea(t, "")
	fakeProvider(t, "claude", "exit 0\n")
	res, out, err := cdwRevisar(t, root, opt)
	if err != nil || res.RecordID == "" {
		t.Fatalf("%s: la review corre: %v %+v\n%s", ca, err, res, out)
	}
	if _, hay := cdwRegistroCrudo(t, res.RecordID, wt, root)["encierro"]; hay {
		t.Fatalf("%s: sin la seccion el registro de la review no lleva encierro", ca)
	}
	if _, hay := cdwRegistroDelSobre(t, root)["encierro"]; hay {
		t.Fatalf("%s: sin la seccion el registro del sobre de la review no lleva encierro", ca)
	}
	if hbRecordDe(t, res.RecordID, wt, root).Encierro != nil {
		t.Fatalf("%s: Record.Encierro es nil sin la seccion", ca)
	}
}

// CA-674 y CA-677: una review sin --task revisa el principal sin encierro,
// con el motivo `sin tarea`.
func TestCA674_LaReviewSinTareaEsSinTarea(t *testing.T) {
	const ca = "CA-674"
	root := cdwRepo(t, "encierro:\n  exigir: no\n")
	git(t, root, "checkout", "-q", "-b", "feature")
	write(t, root, "app.go", "package app\n\nfunc Nuevo() {}\n")
	git(t, root, "commit", "-q", "-am", "cambio de codigo")
	fakeProvider(t, "codex", "exit 0\n")
	res, out, err := cdwRevisar(t, root, Options{Lens: "risk", Provider: "codex"})
	if err != nil || res.RecordID == "" {
		t.Fatalf("%s: la review sin --task corre: %v %+v\n%s", ca, err, res, out)
	}
	want := encierro.Info{Nivel: encierro.NivelNinguno, Falta: []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC},
		Motivo: encierro.MotivoSinTarea}
	cdwExigeInfo(t, ca, "la review sin --task", cdwRegistroCrudo(t, res.RecordID, root)["encierro"], want)
	if !strings.Contains(out, encierro.Aviso(want, "reviewer")) {
		t.Fatalf("%s: la salida trae el aviso con el rol reviewer:\n%s", ca, out)
	}
}

// ---------------------------------------------------------------- CA-676

// CA-676: con un nivel menor que exigir, la review se niega antes de su
// primera pasada con la nota, exit 1 y sin registro; el reviewer no corre.
func TestCA676_LaReviewSeNiegaConUnNivelMenorQueExigir(t *testing.T) {
	const ca = "CA-676"
	restaurar := encierro.ReemplazarSonda(encierro.Sonda{})
	defer restaurar()
	for _, exigir := range []string{"parcial", "completo"} {
		root, wt, opt := cdwTarea(t, "encierro:\n  exigir: "+exigir+"\n")
		info := encierro.Nivel(true, cdwProv(t, "claude"))
		if encierro.Alcanza(info.Nivel, exigir) {
			t.Fatalf("%s: fixture: sin bwrap el nivel no alcanza %s", ca, exigir)
		}
		marca := filepath.Join(t.TempDir(), "invocado")
		fakeProvider(t, "claude", "echo x >> '"+marca+"'\nexit 0\n")
		antes := len(h3Metas(t, root, wt))
		res, out, err := cdwRevisar(t, root, opt)
		nota := encierro.NotaExigir(info, exigir, "reviewer")
		if res.ExitCode != 1 || !(strings.Contains(out, nota) || (err != nil && strings.Contains(err.Error(), nota))) {
			t.Fatalf("%s: exigir %s: la review se niega con exit 1 y %q: %v %+v\n%s", ca, exigir, nota, err, res, out)
		}
		if cdwExiste(marca) || len(res.Passes) != 0 && res.Passes[0].RunID != "" || len(h3Metas(t, root, wt)) != antes {
			t.Fatalf("%s: exigir %s: ninguna pasada corre: %+v", ca, exigir, res)
		}
		cdwSinRegistros(t, ca, "exigir "+exigir, root, wt)
	}
}

// CA-675: con la politica rota contra el arbol, la review se niega antes de
// su primera pasada con el error de Validar, exit 1 y sin registro.
func TestCA675_LaReviewSeNiegaConLaPoliticaRota(t *testing.T) {
	const ca = "CA-675"
	root, wt, opt := cdwTarea(t, "")
	escribible := filepath.Join(root, ".hoom", "items")
	write(t, root, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\nencierro:\n  escribibles:\n    - "+escribible+"\n")
	git(t, root, "add", "hoom.yaml")
	git(t, root, "commit", "-q", "-m", "politica rota")
	marca := filepath.Join(t.TempDir(), "invocado")
	fakeProvider(t, "claude", "echo x >> '"+marca+"'\nexit 0\n")
	res, out, err := cdwRevisar(t, root, opt)
	want := "encierro: escribibles: " + escribible + " contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
	if res.ExitCode != 1 || !(strings.Contains(out, want) || (err != nil && strings.Contains(err.Error(), want))) || cdwExiste(marca) {
		t.Fatalf("%s: la review se niega con exit 1 y %q, sin pasada: %v %+v\n%s", ca, want, err, res, out)
	}
	cdwSinRegistros(t, ca, "politica rota", root, wt)
}

// ---------------------------------------------------------------- CA-685

func cdwSha(data string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(data), data)))
	return hex.EncodeToString(sum[:])
}

// cdwComun es el directorio comun de git del arbol, absoluto.
func cdwComun(t *testing.T, wt string) string {
	t.Helper()
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = wt
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	p := strings.TrimSpace(string(out))
	if !filepath.IsAbs(p) {
		p = filepath.Join(wt, p)
	}
	return p
}

// cdwExigePasada exige que err sea el error de la pasada n con rutas, por
// valor o por puntero, con su texto del contrato.
func cdwExigePasada(t *testing.T, ca string, err error, n int, rutas []string, texto string, gitAjeno bool) {
	t.Helper()
	var pasada int
	var got []string
	switch {
	case gitAjeno:
		var v PasadaConGitAjeno
		var p *PasadaConGitAjeno
		if errors.As(err, &v) {
			pasada, got = v.Pasada, v.Rutas
		} else if errors.As(err, &p) && p != nil {
			pasada, got = p.Pasada, p.Rutas
		} else {
			t.Fatalf("%s: Run devuelve PasadaConGitAjeno: %T %v", ca, err, err)
		}
	default:
		var v PasadaConConfigDeAgentes
		var p *PasadaConConfigDeAgentes
		if errors.As(err, &v) {
			pasada, got = v.Pasada, v.Rutas
		} else if errors.As(err, &p) && p != nil {
			pasada, got = p.Pasada, p.Rutas
		} else {
			t.Fatalf("%s: Run devuelve PasadaConConfigDeAgentes: %T %v", ca, err, err)
		}
	}
	if pasada != n || !reflect.DeepEqual(got, rutas) {
		t.Fatalf("%s: la pasada %d con %q; fue la %d con %q", ca, n, rutas, pasada, got)
	}
	if !strings.Contains(err.Error(), texto) {
		t.Fatalf("%s: el error dice %q; fue %q", ca, texto, err.Error())
	}
}

// CA-685: la primera pasada que crea una ref de reemplazo hace fallar la
// review con PasadaConGitAjeno, su texto del contrato y sin registro; vale
// igual sin encierro.
func TestCA685_UnaPasadaQueCreaUnaRefDeReemplazo(t *testing.T) {
	const ca = "CA-685"
	root, wt, opt := cdwTarea(t, "")
	a, b := cdwSha("cdw a"), cdwSha("cdw b")
	fakeProvider(t, "claude", "printf 'cdw a' | git hash-object -w --stdin >/dev/null\nprintf 'cdw b' | git hash-object -w --stdin >/dev/null\n"+
		"git replace -f "+a+" "+b+"\nexit 0\n")
	res, out, err := cdwRevisar(t, root, opt)
	ruta := filepath.Join(cdwComun(t, wt), "refs", "replace", a)
	cdwExigePasada(t, ca, err, 1, []string{ruta}, "la pasada 1 del reviewer cambio git fuera de la rama de su tarea: "+ruta+": no registro la review", true)
	if res.RecordID != "" {
		t.Fatalf("%s: sin registro: %+v\n%s", ca, res, out)
	}
	cdwSinRegistros(t, ca, "git-ajeno", root, wt)
	if (PasadaConGitAjeno{Pasada: 3, Rutas: []string{"/r/a", "/r/b"}}).Error() != "la pasada 3 del reviewer cambio git fuera de la rama de su tarea: /r/a, /r/b: no registro la review" {
		t.Fatalf("%s: el texto con varias rutas las une con \", \"", ca)
	}
}

// CA-685: con las cuatro lentes, la que rompe es la segunda pasada, y el
// error la nombra.
func TestCA685_LaSegundaPasadaQueCambiaGitAjeno(t *testing.T) {
	const ca = "CA-685"
	root, wt, opt := cdwTarea(t, "")
	write(t, wt, "internal/auth/token.go", "package auth\n\n// Valida revisa un token\nfunc Valida(s string) bool { return s != \"\" }\n")
	raCommit(t, wt, "una ruta de riesgo: cuatro lentes")
	opt.Lens = ""
	cuenta := filepath.Join(t.TempDir(), "cuenta")
	a, b := cdwSha("cdw a2"), cdwSha("cdw b2")
	fakeProvider(t, "claude", "n=$(cat '"+cuenta+"' 2>/dev/null || echo 0)\nn=$((n+1))\necho $n > '"+cuenta+"'\n"+
		"if [ \"$n\" = 2 ]; then printf 'cdw a2' | git hash-object -w --stdin >/dev/null; printf 'cdw b2' | git hash-object -w --stdin >/dev/null; git replace -f "+a+" "+b+"; fi\nexit 0\n")
	_, out, err := cdwRevisar(t, root, opt)
	ruta := filepath.Join(cdwComun(t, wt), "refs", "replace", a)
	if strings.TrimSpace(cdwLeer(cuenta)) == "1" {
		t.Fatalf("%s: fixture: la review con una ruta de riesgo corre mas de una pasada:\n%s", ca, out)
	}
	cdwExigePasada(t, ca, err, 2, []string{ruta}, "la pasada 2 del reviewer cambio git fuera de la rama de su tarea: "+ruta+": no registro la review", true)
	cdwSinRegistros(t, ca, "la segunda pasada", root, wt)
}

func cdwLeer(p string) string { raw, _ := os.ReadFile(p); return string(raw) }

// ---------------------------------------------------------------- CA-689

// CA-689: la pasada que cambia la config de los agentes (hooks de
// ~/.claude/settings.json) hace fallar la review con su error, exit 1 y sin
// registro; que cambie otra clave no.
func TestCA689_UnaPasadaQueCambiaLaConfigDeLosAgentes(t *testing.T) {
	const ca = "CA-689"
	root, wt, opt := cdwTarea(t, "")
	settings := filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	write(t, filepath.Dir(settings), "settings.json", `{"hooks":{},"numStartups":1}`)
	fakeProvider(t, "claude", "printf '{\"hooks\":{\"Stop\":[]},\"numStartups\":2}' > '"+settings+"'\nexit 0\n")
	res, out, err := cdwRevisar(t, root, opt)
	cdwExigePasada(t, ca, err, 1, []string{settings}, "la pasada 1 del reviewer cambio la config de los agentes: "+settings+": no registro la review", false)
	if res.RecordID != "" {
		t.Fatalf("%s: sin registro: %+v\n%s", ca, res, out)
	}
	cdwSinRegistros(t, ca, "config-de-agentes", root, wt)

	// otra clave del mismo archivo no es config-de-agentes
	root, wt, opt = cdwTarea(t, "")
	settings = filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")
	write(t, filepath.Dir(settings), "settings.json", `{"hooks":{},"numStartups":1}`)
	fakeProvider(t, "claude", "printf '{\"numStartups\":9,\"hooks\":{}}' > '"+settings+"'\nexit 0\n")
	res, out, err = cdwRevisar(t, root, opt)
	if err != nil || res.RecordID == "" {
		t.Fatalf("%s: que claude cambie sus contadores no hace fallar la review: %v %+v\n%s", ca, err, res, out)
	}
	_ = wt
	if (PasadaConConfigDeAgentes{Pasada: 2, Rutas: []string{".mcp.json", ".claude/x"}}).Error() != "la pasada 2 del reviewer cambio la config de los agentes: .mcp.json, .claude/x: no registro la review" {
		t.Fatalf("%s: el texto con varias rutas las une con \", \"", ca)
	}
}
