// Tests adversariales de la ENMIENDA 5 del spec
// .hoom/specs/review-aislada-y-modelo-elegido.md (CA-414, CA-416 y el caso
// limite de 0 lentes): el orden del paso previo de `hoom review`. "No lee
// nada del arbol de trabajo antes de saber que esta limpio: primero la guarda
// de arbol sucio (git status, que no abre archivos), despues hoom.yaml,
// despues la base (merge-base y un git diff que git pueda armar) y recien
// entonces la medida con la que se deciden las lentes. Una base rota falla
// cerrado aunque el cambio pida 0 lentes: nunca termina SIN REVISAR."
//
//   - CA-414: una base inexistente, un clon shallow sin el merge-base, un
//     arbol de la base que falta en .git/objects, o un git status / git diff
//     que fallan son el error de git tambien sin --lens, con un cambio de 0
//     lentes (solo documentacion) y con uno de codigo: nunca SIN REVISAR,
//     sin pasadas ni registro; Evidence devuelve el error. Por Run y por la
//     CLI.
//   - CA-416: un hoom.yaml sin commitear (editado con una clave o un tipo que
//     no parsean, cambiado por un FIFO o por un symlink a /dev/zero, o sin
//     rastrear en un HEAD que no lo tiene) es un arbol sucio: la negativa lo
//     nombra antes de leerlo, enseguida y sin memoria en proporcion, y su
//     contenido no sale en ningun error, salida ni archivo. Por Run (sin
//     tarea y con tarea) y por la CLI sin --task, que encuentra ./hoom.yaml
//     sola.
//   - Orden: arbol sucio y base rota juntos = la negativa de arbol sucio,
//     que nombra la ruta (no el error de git); 0 lentes con el arbol sucio =
//     la negativa, no SIN REVISAR.
//
// El FIFO suelto (caso limite de CA-416) esta re-expresado en
// TestCA401_FIFOSinRastrearNoEsParteDeLaEvidencia
// (review_aislada_evidencia_test.go). Los fixtures comunes estan en
// review_aislada_helpers_test.go.
package reviewcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// ---------------------------------------------------------------- fixtures

// raTipoDeCambio es lo que la rama commitea: SOLO documentacion (con la base
// sana y el arbol limpio pide 0 lentes: SIN REVISAR) o codigo (una lente).
// arch es el archivo que la rama cambia, sub uno de un subdirectorio que la
// base trae y la rama cambia, y otro lo que main commitea despues de abrir la
// rama.
type raTipoDeCambio struct {
	nombre          string
	codigo          bool
	arch, sub, otro string
}

var raTiposDeCambio = []raTipoDeCambio{
	{"solo-documentacion", false, "README.md", "docs/guia.md", "CAMBIOS.md"},
	{"codigo", true, "app.go", "pkg/guia.go", "main.go"},
}

// raSoloDocs es el cambio de 0 lentes.
var raSoloDocs = raTiposDeCambio[0]

// raCuerpo es el contenido de la version v de un archivo del tipo de cambio.
func raCuerpo(tc raTipoDeCambio, v string) string {
	if tc.codigo {
		return "package app\n\n// " + v + " (enmienda 5)\nfunc Nuevo() {}\n"
	}
	return "# demo\n\n" + v + " (enmienda 5)\n"
}

// raCambioDe commitea en la rama el cambio del tipo tc (solo su archivo).
func raCambioDe(t *testing.T, root string, tc raTipoDeCambio) {
	t.Helper()
	raRama(t, root)
	write(t, root, tc.arch, raCuerpo(tc, "cambio de la rama"))
	git(t, root, "add", "--", tc.arch)
	git(t, root, "commit", "-q", "-m", "cambio de la rama ("+tc.nombre+")", "--", tc.arch)
}

// raDocs commitea en la rama un cambio SOLO de documentacion (README.md): con
// la base sana y el arbol limpio pide 0 lentes y la review es SIN REVISAR.
func raDocs(t *testing.T, root string) { t.Helper(); raCambioDe(t, root, raSoloDocs) }

// raConBaseInexistente es raRepo con base_branch: rama-que-no-existe
// commiteado en main (asi la CLI, que toma la base de hoom.yaml, revisa
// contra una base que no existe).
func raConBaseInexistente(t *testing.T) string {
	t.Helper()
	root := raRepo(t, "")
	raw, err := os.ReadFile(filepath.Join(root, "hoom.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	write(t, root, "hoom.yaml", strings.Replace(string(raw), "base_branch: main\n", "base_branch: rama-que-no-existe\n", 1))
	git(t, root, "commit", "-q", "-am", "base_branch inexistente")
	return root
}

// raClonShallowDe es raClonShallow con el tipo de cambio tc: la rama
// commitea tc.arch dos veces y main avanza con tc.otro. El clon (--depth 1)
// no llega al merge-base; mida lo que mida sin el, el cambio es del tipo tc.
// Devuelve tambien el origen, que tiene el merge-base: el mismo cambio con la
// base sana.
func raClonShallowDe(t *testing.T, tc raTipoDeCambio) (origen, clon string) {
	t.Helper()
	origen = raRepo(t, "")
	git(t, origen, "checkout", "-q", "-b", "feature")
	write(t, origen, tc.arch, raCuerpo(tc, "rama 1"))
	git(t, origen, "add", "-A")
	git(t, origen, "commit", "-q", "-m", "rama 1")
	write(t, origen, tc.arch, raCuerpo(tc, "rama 2"))
	git(t, origen, "commit", "-q", "-am", "rama 2")
	git(t, origen, "checkout", "-q", "main")
	write(t, origen, tc.otro, raCuerpo(tc, "main avanza"))
	git(t, origen, "add", "-A")
	git(t, origen, "commit", "-q", "-m", "main avanza")
	git(t, origen, "checkout", "-q", "feature")

	padre, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	clon = filepath.Join(padre, "clon")
	git(t, padre, "clone", "-q", "--depth", "1", "--no-single-branch", "--branch", "feature", "file://"+origen, clon)
	git(t, clon, "branch", "-q", "main", "origin/main")
	git(t, clon, "config", "user.email", "test@hoom.dev")
	git(t, clon, "config", "user.name", "hoom test")
	cmd := exec.Command("git", "merge-base", "main", "HEAD")
	cmd.Dir = clon
	if out, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("CA-414: fixture: el clon shallow no llega al merge-base, y git lo encontro: %s", out)
	}
	return origen, clon
}

// raConSubdirEnLaBase arma la base (main) con tc.sub y la rama que lo
// cambia: un cambio del tipo tc, commiteado, con la base sana.
func raConSubdirEnLaBase(t *testing.T, tc raTipoDeCambio) string {
	t.Helper()
	root := raRepo(t, "")
	write(t, root, tc.sub, raCuerpo(tc, "version de la base"))
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "la base trae "+tc.sub)
	raRama(t, root)
	write(t, root, tc.sub, raCuerpo(tc, "version de la rama"))
	git(t, root, "commit", "-q", "-am", "la rama cambia "+tc.sub)
	return root
}

// raBorrarArbolDeLaBase borra de .git/objects el arbol de main del
// subdirectorio de tc.sub (un objeto suelto que HEAD no comparte). git
// merge-base, git status y el hoom.yaml de la base siguen andando; git no
// puede armar el diff de la base contra HEAD ("un arbol que git no puede
// leer").
func raBorrarArbolDeLaBase(t *testing.T, real, root string, tc raTipoDeCambio) {
	t.Helper()
	sub := filepath.ToSlash(filepath.Dir(tc.sub))
	oid := raOid(t, real, root, "main:"+sub)
	if err := os.Remove(filepath.Join(root, ".git", "objects", oid[:2], oid[2:])); err != nil {
		t.Fatalf("CA-414: fixture: el arbol %s/ de la base es un objeto suelto: %v", sub, err)
	}
	for _, c := range [][]string{
		{"merge-base", "main", "HEAD"},
		{"status", "--porcelain"},
		{"show", "main:hoom.yaml"},
		{"diff", "main", "HEAD"},
	} {
		cmd := exec.Command(real, c...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		switch c[0] {
		case "diff":
			if err == nil {
				t.Fatalf("CA-414: fixture: sin el arbol de la base, git diff falla: %q", out)
			}
		case "status":
			if err != nil {
				t.Fatalf("CA-414: fixture: git status sigue andando: %v %q", err, out)
			}
			for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
				if len(l) > 3 && !strings.HasPrefix(l[3:], ".hoom/") {
					t.Fatalf("CA-414: fixture: el arbol esta limpio fuera de .hoom/: %q", out)
				}
			}
		default:
			if err != nil {
				t.Fatalf("CA-414: fixture: git %s sigue andando: %v %q", c[0], err, out)
			}
		}
	}
}

// raRunBaseConReloj es raRunConReloj contra la base que se le pase.
func raRunBaseConReloj(t *testing.T, ca string, d time.Duration, root, base string, opt Options) (Result, error, string) {
	t.Helper()
	type resultado struct {
		res Result
		err error
		out string
	}
	ch := make(chan resultado, 1)
	go func() {
		var out bytes.Buffer
		res, err := Run(root, base, opt, &out)
		ch <- resultado{res, err, out.String()}
	}()
	select {
	case r := <-ch:
		return r.res, r.err, r.out
	case <-time.After(d):
		t.Fatalf("%s: la review no volvio en %s", ca, d)
	}
	return Result{}, nil, ""
}

// raMsg junta lo que la review le dijo al usuario: su salida y su error.
func raMsg(out string, err error) string {
	if err != nil {
		return out + "\n" + err.Error()
	}
	return out
}

func raDiceSinRevisar(s string) bool { return strings.Contains(strings.ToUpper(s), "SIN REVISAR") }

// raSinTextoEnArbol exige que ningun archivo regular bajo dir (fuera de los
// .git y de excepto, el archivo que trae el texto a proposito) traiga texto:
// ni registros, ni sobres, ni runs, ni nada que la review haya escrito.
func raSinTextoEnArbol(t *testing.T, ca, dir, texto, excepto string) {
	t.Helper()
	if texto == "" {
		return
	}
	_ = filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.Name() == ".git" {
			if fi.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if fi.IsDir() || !fi.Mode().IsRegular() || p == excepto {
			return nil
		}
		if raw, _ := os.ReadFile(p); bytes.Contains(raw, []byte(texto)) {
			t.Fatalf("%s: %q quedo en %s", ca, texto, p)
		}
		return nil
	})
}

// ---------------------------------------------------------------- CA-414

// raBaseRota arma un cambio del tipo tc commiteado con la base sana (sano,
// contra baseSana) y devuelve romper, que deja la base rota y dice donde y
// contra que base revisar.
type raBaseRota struct {
	nombre string
	armar  func(t *testing.T, bin, real string, tc raTipoDeCambio) (sano, baseSana string, romper func(t *testing.T) (root, base string))
}

var raBasesRotas = []raBaseRota{
	{"base-inexistente", func(t *testing.T, bin, real string, tc raTipoDeCambio) (string, string, func(t *testing.T) (string, string)) {
		sano := raRepo(t, "")
		raCambioDe(t, sano, tc)
		return sano, "main", func(t *testing.T) (string, string) {
			root := raConBaseInexistente(t)
			raCambioDe(t, root, tc)
			raLimpio(t, "CA-414", root)
			return root, "rama-que-no-existe"
		}
	}},
	{"clon-shallow-sin-merge-base", func(t *testing.T, bin, real string, tc raTipoDeCambio) (string, string, func(t *testing.T) (string, string)) {
		origen, clon := raClonShallowDe(t, tc)
		return origen, "main", func(t *testing.T) (string, string) {
			raLimpio(t, "CA-414", clon)
			return clon, "main"
		}
	}},
	{"falta-el-arbol-de-la-base", func(t *testing.T, bin, real string, tc raTipoDeCambio) (string, string, func(t *testing.T) (string, string)) {
		root := raConSubdirEnLaBase(t, tc)
		return root, "main", func(t *testing.T) (string, string) {
			raBorrarArbolDeLaBase(t, real, root, tc)
			return root, "main"
		}
	}},
	{"status-falla", func(t *testing.T, bin, real string, tc raTipoDeCambio) (string, string, func(t *testing.T) (string, string)) {
		root := raRepo(t, "")
		raCambioDe(t, root, tc)
		return root, "main", func(t *testing.T) (string, string) {
			raGitRoto(t, bin, real, "status")
			return root, "main"
		}
	}},
	{"diff-falla", func(t *testing.T, bin, real string, tc raTipoDeCambio) (string, string, func(t *testing.T) (string, string)) {
		root := raRepo(t, "")
		raCambioDe(t, root, tc)
		return root, "main", func(t *testing.T) (string, string) {
			raGitRoto(t, bin, real, "diff", "diff-index", "diff-files", "diff-tree")
			return root, "main"
		}
	}},
}

// CA-414 (re-expresado por la enmienda 5): "si falla git merge-base (base
// inexistente o clon shallow), git status o git diff (un arbol que git no
// puede leer), hoom review devuelve el error de git sin lanzar ninguna pasada
// ni escribir registro, tambien sin --lens y antes de decidir las lentes
// (nunca termina SIN REVISAR); Evidence devuelve el mismo error", y el caso
// limite "0 lentes (solo documentacion, con el arbol limpio y la base sana):
// SIN REVISAR ...; con la base rota, el error de CA-414". Sin --lens, con un
// cambio SOLO de documentacion (0 lentes) y con uno de codigo (una lente):
// control, con la base sana y el arbol limpio la documentacion es SIN
// REVISAR y el codigo se revisa; con la base rota, Run devuelve un error (no
// la negativa por arbol sucio), no termina sin-revisar ni dice SIN REVISAR,
// no lanza ninguna pasada ni escribe registro, y Evidence tambien devuelve un
// error.
func TestCA414_BaseRotaSinLensEsErrorNuncaSinRevisar(t *testing.T) {
	real := raGitReal(t)
	for _, tc := range raTiposDeCambio {
		for _, c := range raBasesRotas {
			t.Run(c.nombre+"/"+tc.nombre, func(t *testing.T) {
				bin := raPATH(t)
				cx := raInstalar(t, bin, "codex", "")
				sano, baseSana, romper := c.armar(t, bin, real, tc)

				raLimpio(t, "CA-414", sano)
				res, rerr, out := raRunBaseConReloj(t, "CA-414", 60*time.Second, sano, baseSana, Options{Provider: "codex"})
				if tc.codigo && (rerr != nil || res.Status != "revisado" || len(res.Passes) != 1 || cx.veces() != 1) {
					t.Fatalf("CA-414: %s: control: el cambio de codigo, con la base sana y el arbol limpio, se revisa (una lente): %+v %v\n%s",
						c.nombre, res, rerr, out)
				}
				if !tc.codigo && (rerr != nil || res.Status != "sin-revisar" || res.ExitCode != 0 || len(res.Lenses) != 0 || cx.veces() != 0) {
					t.Fatalf("CA-414: %s: control: el cambio solo de documentacion, con la base sana y el arbol limpio, es SIN REVISAR (0 lentes): %+v %v\n%s",
						c.nombre, res, rerr, out)
				}

				pasadas := cx.veces()
				root, base := romper(t)
				recs, _ := Records(root)
				registros := len(recs)
				ev, err, _ := raEvidenceConReloj(t, "CA-414", 60*time.Second, root, base, "", raTopeGrande)
				if err == nil {
					t.Errorf("CA-414: %s: Evidence devuelve el error de git; devolvio una evidencia de %d bytes (Over %v)", c.nombre, ev.Bytes, ev.Over)
				}

				res, rerr, out = raRunBaseConReloj(t, "CA-414", 60*time.Second, root, base, Options{Provider: "codex"})
				msg := raMsg(out, rerr)
				if res.Status == "sin-revisar" || raDiceSinRevisar(msg) {
					t.Fatalf("CA-414: %s: con la base rota la review nunca termina SIN REVISAR, sin --lens: %+v %v\n%s",
						c.nombre, res, rerr, out)
				}
				if rerr == nil {
					t.Fatalf("CA-414: %s: con la base rota hoom review devuelve el error de git (antes de decidir las lentes): %+v\n%s", c.nombre, res, out)
				}
				if _, sucio := raRutaSucia(msg); sucio {
					t.Fatalf("CA-414: %s: el arbol esta limpio: el error es el de git, no la negativa por arbol sucio:\n%s", c.nombre, msg)
				}
				if cx.veces() != pasadas || len(res.Passes) != 0 || res.Status == "revisado" {
					t.Fatalf("CA-414: %s: la review no lanza ninguna pasada (codex %d, antes %d): %+v\n%s", c.nombre, cx.veces(), pasadas, res, msg)
				}
				if recs, _ := Records(root); len(recs) != registros {
					t.Fatalf("CA-414: %s: sin registro de review nuevo: %+v", c.nombre, recs)
				}
			})
		}
	}
}

// ---------------------------------------------------------------- CA-416: hoom.yaml sin commitear

// raHoomYamlSucio deja el hoom.yaml RASTREADO de dir sin commitear, de una
// forma que leerlo (o parsearlo) delataria. oculto es lo que no puede
// aparecer en ningun lado ("" = nada: no tiene contenido que filtrar).
type raHoomYamlSucio struct {
	nombre string
	oculto string
	// hijo: leerlo no termina (un FIFO sin escritor, /dev/zero): la review
	// corre en un proceso hijo con reloj y vigilante de memoria, que se mata
	// si se cuelga (una goroutine colgada en este proceso podria seguir
	// despues del test, con el PATH de la maquina)
	hijo    bool
	plantar func(t *testing.T, dir string)
}

var raHoomYamlSucios = []raHoomYamlSucio{
	{"editado-con-una-clave-desconocida", "SECRETO_ZX81", false, func(t *testing.T, dir string) {
		write(t, dir, "hoom.yaml", raYAML("review:\n  SECRETO_ZX81: x\n"))
	}},
	// yaml cita un valor largo cortado ("quizas_..."): lo oculto es su prefijo
	{"editado-con-un-tipo-que-no-parsea", "quizas", false, func(t *testing.T, dir string) {
		write(t, dir, "hoom.yaml", raYAML("review:\n  isolated: quizas_ZX82\n"))
	}},
	{"cambiado-por-un-fifo", "", true, func(t *testing.T, dir string) {
		p := filepath.Join(dir, "hoom.yaml")
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		raConFIFO(t, p)
	}},
	{"cambiado-por-un-symlink-a-dev-zero", "", true, func(t *testing.T, dir string) {
		if fi, err := os.Stat("/dev/zero"); err != nil || fi.Mode()&os.ModeDevice == 0 {
			t.Skip("sin /dev/zero")
		}
		p := filepath.Join(dir, "hoom.yaml")
		if err := os.Remove(p); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("/dev/zero", p); err != nil {
			t.Fatal(err)
		}
	}},
}

// raSoloHoomYamlSucio (fixture): lo unico sin commitear fuera de .hoom/ en dir
// es hoom.yaml, asi que "la primera ruta" es hoom.yaml sin ambiguedad.
func raSoloHoomYamlSucio(t *testing.T, dir string) {
	t.Helper()
	s := raSuciedad(t, dir)
	if len(s) != 1 || s[0][3:] != "hoom.yaml" {
		t.Fatalf("CA-416: fixture: lo unico sin commitear fuera de .hoom/ es hoom.yaml: %q", s)
	}
}

// raHijoHoomYaml: con esta variable el test corre como proceso hijo y corre
// la review (Run) con un vigilante que lo mata si asigna mas de 64 MiB. Asi
// una review que lee un hoom.yaml symlink a /dev/zero no se come la memoria
// de la maquina, y una que abre un hoom.yaml FIFO no deja una goroutine
// colgada en el padre, que le pone reloj y la mata.
const raHijoHoomYaml = "HOOM_TW_CA416_HOOMYAML_ROOT"

func raHijoRun(root, task string) {
	var antes runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&antes)
	go func() {
		var ms runtime.MemStats
		for {
			runtime.ReadMemStats(&ms)
			if ms.TotalAlloc-antes.TotalAlloc > 64<<20 {
				fmt.Println("RA-CA416-LEYO-DE-MAS")
				os.Exit(42)
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	inicio := time.Now()
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Provider: "codex", Task: task}, &out)
	var despues runtime.MemStats
	runtime.ReadMemStats(&despues)
	r := map[string]any{"status": res.Status, "exit_code": res.ExitCode, "passes": len(res.Passes), "out": out.String(),
		"ms": time.Since(inicio).Milliseconds(), "alloc": despues.TotalAlloc - antes.TotalAlloc}
	if err != nil {
		r["err"] = err.Error()
	}
	raw, _ := json.Marshal(r)
	fmt.Println("RA-CA416 " + string(raw))
}

// raRunEnHijo corre la review de root (con la tarea task, "" = sin tarea) en
// un proceso hijo con reloj de 15 s (la review misma tiene 10) y el vigilante
// de 64 MiB, y devuelve lo que informo. caso nombra el hoom.yaml.
func raRunEnHijo(t *testing.T, test, caso, root, task string) (status string, exit, passes int, msg string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+test+"$", "-test.count=1")
	cmd.Env = append(os.Environ(), raHijoHoomYaml+"="+root, raHijoHoomYaml+"_TASK="+task)
	raw, _ := cmd.CombinedOutput()
	salida := string(raw)
	if len(salida) > 4000 {
		salida = salida[len(salida)-4000:]
	}
	if ctx.Err() != nil {
		t.Fatalf("CA-416: %s: la review no volvio en 15 s: esta leyendo hoom.yaml\n%s", caso, salida)
	}
	if strings.Contains(salida, "RA-CA416-LEYO-DE-MAS") {
		t.Fatalf("CA-416: %s: la review asigno mas de 64 MiB: leyo hoom.yaml\n%s", caso, salida)
	}
	m := regexp.MustCompile(`(?m)^RA-CA416 (.*)$`).FindStringSubmatch(salida)
	if m == nil {
		t.Fatalf("CA-416: el proceso hijo no informo el resultado:\n%s", salida)
	}
	var r struct {
		Status   string `json:"status"`
		ExitCode int    `json:"exit_code"`
		Passes   int    `json:"passes"`
		Out      string `json:"out"`
		Err      string `json:"err"`
		Ms       int64  `json:"ms"`
		Alloc    uint64 `json:"alloc"`
	}
	if err := json.Unmarshal([]byte(m[1]), &r); err != nil {
		t.Fatalf("CA-416: resultado ilegible del hijo: %v %s", err, m[1])
	}
	if r.Ms > 10000 {
		t.Fatalf("CA-416: %s: la negativa es enseguida (10 s): tardo %d ms", caso, r.Ms)
	}
	if r.Alloc > 64<<20 {
		t.Fatalf("CA-416: %s: la review asigno %d MiB", caso, r.Alloc>>20)
	}
	msg = r.Out
	if r.Err != "" {
		msg += "\n" + r.Err
	}
	if r.Err == "" && r.ExitCode == 0 {
		// sin error y con exit 0: no se nego; el llamador lo dice
		msg += "\n(sin error, exit 0)"
	}
	exit = r.ExitCode
	if r.Err != "" && exit == 0 {
		exit = -1 // un error cuenta como no salir con 0
	}
	return r.Status, exit, r.Passes, msg
}

// CA-416 (enmienda 5): "un hoom.yaml sin commitear" es un arbol sucio, y
// "hoom review y Evidence devuelven el error del contrato con la primera ruta
// antes de leer hoom.yaml o medir el arbol ... y ningun contenido sin
// commitear aparece en la salida" (caso limite: "Un hoom.yaml editado y sin
// commitear (o reemplazado por un FIFO o un symlink a /dev/zero): la negativa
// de arbol sucio lo nombra antes de leerlo; su contenido no sale en ningun
// error"). Con un cambio de codigo commiteado y el hoom.yaml RASTREADO
// cambiado sin commitear (una clave desconocida con un secreto, un tipo que
// no parsea, un FIFO, un symlink a /dev/zero), en el proyecto (sin tarea) y
// en el worktree de una tarea (Options.Task), sin --lens: `hoom review`
// vuelve enseguida (reloj de 10 s; el FIFO y /dev/zero corren en un proceso
// hijo que se mata si se cuelga, con un vigilante de 64 MiB) con la negativa
// por arbol sucio que nombra
// hoom.yaml, no con un error de parseo; no lanza ninguna pasada, no escribe
// registro, no sale con 0, y el secreto no aparece en la salida, en el error
// ni en ningun archivo del proyecto. Evidence, igual.
func TestCA416_HoomYamlSinCommitearEsArbolSucioAntesDeLeerlo(t *testing.T) {
	const test = "TestCA416_HoomYamlSinCommitearEsArbolSucioAntesDeLeerlo"
	if root := os.Getenv(raHijoHoomYaml); root != "" {
		raHijoRun(root, os.Getenv(raHijoHoomYaml+"_TASK"))
		return
	}
	for _, conTarea := range []bool{false, true} {
		for _, c := range raHoomYamlSucios {
			nombre := "sin-tarea/" + c.nombre
			if conTarea {
				nombre = "con-tarea/" + c.nombre
			}
			t.Run(nombre, func(t *testing.T) {
				bin := raPATH(t)
				cx := raInstalar(t, bin, "codex", "")
				root := raRepo(t, "")
				dir, task := root, ""
				if conTarea {
					dir, task = cbTarea(t, root, "precios"), "precios"
				} else {
					raCambio(t, root)
				}
				raLimpio(t, "CA-416", dir)
				c.plantar(t, dir)
				raSoloHoomYamlSucio(t, dir)

				var status, msg string
				var exit, passes int
				if c.hijo {
					status, exit, passes, msg = raRunEnHijo(t, test, c.nombre, root, task)
				} else {
					res, rerr, out := raRunConReloj(t, "CA-416", 10*time.Second, root, Options{Provider: "codex", Task: task})
					status, passes, msg = res.Status, len(res.Passes), raMsg(out, rerr)
					exit = res.ExitCode
					if rerr != nil && exit == 0 {
						exit = -1
					}
				}
				if cx.veces() != 0 || passes != 0 || status == "revisado" || status == "sin-revisar" {
					t.Fatalf("CA-416: %s: con hoom.yaml sin commitear no se lanza ninguna pasada (codex %d, status %q):\n%s", c.nombre, cx.veces(), status, msg)
				}
				if exit == 0 {
					t.Fatalf("CA-416: %s: la negativa por arbol sucio no sale con 0:\n%s", c.nombre, msg)
				}
				if ruta, ok := raRutaSucia(msg); !ok || ruta != "hoom.yaml" {
					t.Fatalf("CA-416: %s: la review dice %q, nombrando hoom.yaml antes de leerlo:\n%s", c.nombre, raErrSucio("hoom.yaml"), msg)
				}
				if strings.Contains(msg, "clave desconocida") || strings.Contains(msg, "unmarshal") {
					t.Fatalf("CA-416: %s: la negativa es la de arbol sucio, no un error de parseo del hoom.yaml sin commitear:\n%s", c.nombre, msg)
				}
				if c.oculto != "" && strings.Contains(msg, c.oculto) {
					t.Fatalf("CA-416: %s: el contenido del hoom.yaml sin commitear (%q) no sale en ningun error ni salida:\n%s", c.nombre, c.oculto, msg)
				}
				for _, d := range []string{root, dir} {
					if recs, _ := Records(d); len(recs) != 0 {
						t.Fatalf("CA-416: %s: sin pasadas no hay registro: %+v", c.nombre, recs)
					}
				}
				raSinTextoEnArbol(t, "CA-416: "+c.nombre, root, c.oculto, filepath.Join(dir, "hoom.yaml"))

				if c.hijo {
					return // Evidence con un FIFO o un symlink sin commitear: raCasosSucios
				}
				ev, err, _ := raEvidenceConReloj(t, "CA-416", 10*time.Second, dir, "main", "", raTopeGrande)
				if err == nil {
					t.Fatalf("CA-416: %s: Evidence devuelve %q; armo una evidencia de %d bytes", c.nombre, raErrSucio("hoom.yaml"), ev.Bytes)
				}
				if ruta, ok := raRutaSucia(err.Error()); !ok || ruta != "hoom.yaml" {
					t.Fatalf("CA-416: %s: Evidence devuelve %q: %v", c.nombre, raErrSucio("hoom.yaml"), err)
				}
				if c.oculto != "" && strings.Contains(err.Error(), c.oculto) {
					t.Fatalf("CA-416: %s: el error de Evidence no trae nada sin commitear: %v", c.nombre, err)
				}
			})
		}
	}
}

// raRepoSinHoomYaml es un proyecto cuyo HEAD no tiene hoom.yaml: main con
// app.go (y la telemetria escondida en .hoom/.gitignore) y la rama feature
// con un cambio de codigo commiteado.
func raRepoSinHoomYaml(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	git(t, root, "init", "-q", "-b", "main")
	git(t, root, "config", "user.email", "test@hoom.dev")
	git(t, root, "config", "user.name", "hoom test")
	write(t, root, ".hoom/.gitignore", hoomfs.GitignoreBody())
	write(t, root, "app.go", "package app\n")
	git(t, root, "add", "-A")
	git(t, root, "commit", "-q", "-m", "inicial sin hoom.yaml")
	raCambio(t, root)
	return root
}

// CA-416 (enmienda 5): "uno sin rastrear no ignorado ... y un hoom.yaml sin
// commitear", con el orden "primero la guarda de arbol sucio, despues
// hoom.yaml". Un hoom.yaml SIN RASTREAR en un proyecto cuyo HEAD no lo tiene
// (con una clave desconocida que carga un secreto) es un arbol sucio: `hoom
// review` se niega nombrando hoom.yaml antes de leerlo, sin pasadas ni
// registro, y el secreto no sale en ningun lado; Evidence, igual.
func TestCA416_HoomYamlSinRastrearEnUnHEADSinHoomYaml(t *testing.T) {
	const oculto = "SECRETO_ZX85"
	bin := raPATH(t)
	cx := raInstalar(t, bin, "codex", "")
	root := raRepoSinHoomYaml(t)
	write(t, root, "hoom.yaml", raYAML("review:\n  "+oculto+": x\n"))
	raSoloHoomYamlSucio(t, root)

	res, rerr, out := raRunConReloj(t, "CA-416", 10*time.Second, root, Options{Provider: "codex"})
	msg := raMsg(out, rerr)
	if cx.veces() != 0 || len(res.Passes) != 0 || res.Status == "revisado" || res.Status == "sin-revisar" || (rerr == nil && res.ExitCode == 0) {
		t.Fatalf("CA-416: con un hoom.yaml sin rastrear la review se niega sin pasadas (codex %d): %+v\n%s", cx.veces(), res, msg)
	}
	if ruta, ok := raRutaSucia(msg); !ok || ruta != "hoom.yaml" {
		t.Fatalf("CA-416: la review dice %q:\n%s", raErrSucio("hoom.yaml"), msg)
	}
	if strings.Contains(msg, oculto) {
		t.Fatalf("CA-416: el contenido del hoom.yaml sin rastrear no sale en la salida ni en el error:\n%s", msg)
	}
	if recs, _ := Records(root); len(recs) != 0 {
		t.Fatalf("CA-416: sin pasadas no hay registro: %+v", recs)
	}
	raSinTextoEnArbol(t, "CA-416", root, oculto, filepath.Join(root, "hoom.yaml"))

	ev, err, _ := raEvidenceConReloj(t, "CA-416", 10*time.Second, root, "main", "", raTopeGrande)
	if err == nil {
		t.Fatalf("CA-416: Evidence devuelve %q; armo %d bytes", raErrSucio("hoom.yaml"), ev.Bytes)
	}
	if ruta, ok := raRutaSucia(err.Error()); !ok || ruta != "hoom.yaml" || strings.Contains(err.Error(), oculto) {
		t.Fatalf("CA-416: Evidence devuelve %q sin el contenido: %v", raErrSucio("hoom.yaml"), err)
	}
}

// ---------------------------------------------------------------- Orden

// CA-416 / CA-414 (orden, enmienda 5): "primero la guarda de arbol sucio
// (git status, que no abre archivos), despues hoom.yaml, despues la base" y
// "0 lentes ... con el arbol sucio va primero la negativa de CA-416; con la
// base rota, el error de CA-414". Con el arbol sucio (un no rastreado con
// contenido, fuera de .hoom/) Y la base rota (inexistente, clon shallow sin
// merge-base, un arbol de la base que falta, un git diff que falla), `hoom
// review` se niega por arbol sucio nombrando la ruta, no con el error de git.
// Y con un cambio de 0 lentes (solo documentacion) y el arbol sucio (un no
// rastreado, la documentacion editada sin commitear, hoom.yaml editado), la
// negativa, no SIN REVISAR. Con y sin --lens; nunca una pasada ni registro, y
// nada sin commitear en la salida.
func TestCA416_ArbolSucioVaAntesQueLaBaseRotaYQueCeroLentes(t *testing.T) {
	real := raGitReal(t)
	const oculto = "SIN-COMMITEAR-ORDEN-ZX83"
	nuevo := func(t *testing.T, dir string) {
		write(t, dir, "nuevo.go", "package app\n\n// "+oculto+"\n")
	}
	casos := []struct {
		nombre string
		armar  func(t *testing.T, bin string) (root, base string, rutas []string)
	}{
		{"base-inexistente", func(t *testing.T, bin string) (string, string, []string) {
			root := raConBaseInexistente(t)
			raDocs(t, root)
			nuevo(t, root)
			return root, "rama-que-no-existe", []string{"nuevo.go"}
		}},
		{"clon-shallow-sin-merge-base", func(t *testing.T, bin string) (string, string, []string) {
			_, clon := raClonShallowDe(t, raSoloDocs)
			nuevo(t, clon)
			return clon, "main", []string{"nuevo.go"}
		}},
		{"falta-el-arbol-de-la-base", func(t *testing.T, bin string) (string, string, []string) {
			root := raConSubdirEnLaBase(t, raSoloDocs)
			raBorrarArbolDeLaBase(t, real, root, raSoloDocs)
			nuevo(t, root)
			return root, "main", []string{"nuevo.go"}
		}},
		{"diff-falla", func(t *testing.T, bin string) (string, string, []string) {
			root := raRepo(t, "")
			raDocs(t, root)
			nuevo(t, root)
			raGitRoto(t, bin, real, "diff", "diff-index", "diff-files", "diff-tree")
			return root, "main", []string{"nuevo.go"}
		}},
		{"cero-lentes-con-un-no-rastreado", func(t *testing.T, bin string) (string, string, []string) {
			root := raRepo(t, "")
			raDocs(t, root)
			nuevo(t, root)
			return root, "main", []string{"nuevo.go"}
		}},
		{"cero-lentes-con-la-documentacion-editada-sin-commitear", func(t *testing.T, bin string) (string, string, []string) {
			root := raRepo(t, "")
			raDocs(t, root)
			write(t, root, "README.md", "# demo\n\n"+oculto+"\n")
			return root, "main", []string{"README.md"}
		}},
		{"cero-lentes-con-hoom-yaml-editado", func(t *testing.T, bin string) (string, string, []string) {
			root := raRepo(t, "")
			raDocs(t, root)
			write(t, root, "hoom.yaml", raYAML("review:\n  "+strings.ReplaceAll(oculto, "-", "_")+": x\n"))
			return root, "main", []string{"hoom.yaml"}
		}},
	}
	for _, c := range casos {
		for _, lens := range []string{"", "risk"} {
			nombre := c.nombre + "/sin-lens"
			if lens != "" {
				nombre = c.nombre + "/lens-" + lens
			}
			t.Run(nombre, func(t *testing.T) {
				bin := raPATH(t)
				cx := raInstalar(t, bin, "codex", "")
				root, base, rutas := c.armar(t, bin)

				res, rerr, out := raRunBaseConReloj(t, "CA-416", 60*time.Second, root, base, Options{Provider: "codex", Lens: lens})
				msg := raMsg(out, rerr)
				if ruta, ok := raRutaSucia(msg); !ok || !raRutaAceptada(ruta, rutas) {
					t.Fatalf("CA-416: %s: con el arbol sucio va primero la negativa %q (una de %q), antes que la base y que las lentes: %+v %v\n%s",
						c.nombre, raErrSucio("<ruta>"), rutas, res, rerr, out)
				}
				if res.Status == "sin-revisar" || raDiceSinRevisar(msg) {
					t.Fatalf("CA-416: %s: con el arbol sucio la review no termina SIN REVISAR:\n%s", c.nombre, msg)
				}
				if cx.veces() != 0 || len(res.Passes) != 0 || res.Status == "revisado" || (rerr == nil && res.ExitCode == 0) {
					t.Fatalf("CA-416: %s: la negativa no lanza ninguna pasada ni sale con 0 (codex %d): %+v\n%s", c.nombre, cx.veces(), res, msg)
				}
				if recs, _ := Records(root); len(recs) != 0 {
					t.Fatalf("CA-416: %s: sin pasadas no hay registro: %+v", c.nombre, recs)
				}
				for _, o := range []string{oculto, strings.ReplaceAll(oculto, "-", "_")} {
					if strings.Contains(msg, o) {
						t.Fatalf("CA-416: %s: nada sin commitear aparece en la salida ni en el error (%q):\n%s", c.nombre, o, msg)
					}
				}
			})
		}
	}
}

// ---------------------------------------------------------------- la CLI

// raCorrida es lo que dejo una corrida del binario de hoom.
type raCorrida struct {
	code         int
	out, errOut  string
	colgado      bool   // no volvio a tiempo: se lo mato
	glotona      bool   // paso los 512 MiB residentes: se lo mato
	maxRSS       uint64 // memoria residente maxima, en bytes
	stdoutStderr string
}

// raRSS es la memoria residente del proceso pid, en bytes (0 si no se sabe).
func raRSS(pid int) uint64 {
	if runtime.GOOS == "linux" {
		raw, err := os.ReadFile(fmt.Sprintf("/proc/%d/statm", pid))
		if err != nil {
			return 0
		}
		f := strings.Fields(string(raw))
		if len(f) < 2 {
			return 0
		}
		n, _ := strconv.ParseUint(f[1], 10, 64)
		return n * uint64(os.Getpagesize())
	}
	out, err := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return 0
	}
	kb, _ := strconv.ParseUint(strings.TrimSpace(string(out)), 10, 64)
	return kb * 1024
}

// raHoomConReloj corre el binario de hoom en root (sin HOOM_TASK, con el
// PATH del test) con reloj y un vigilante de memoria: si no vuelve en d, o
// si pasa los 512 MiB residentes, lo mata y lo dice.
func raHoomConReloj(t *testing.T, hoom, root string, d time.Duration, args ...string) raCorrida {
	t.Helper()
	cmd := exec.Command(hoom, args...)
	cmd.Dir = root
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HOOM_TASK=") {
			cmd.Env = append(cmd.Env, kv)
		}
	}
	var o, e bytes.Buffer
	cmd.Stdout, cmd.Stderr = &o, &e
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		t.Fatalf("no pude correr hoom %v: %v", args, err)
	}
	hecho := make(chan struct{})
	go func() { _ = cmd.Wait(); close(hecho) }()
	var c raCorrida
	reloj := time.NewTimer(d)
	defer reloj.Stop()
	tic := time.NewTicker(25 * time.Millisecond)
	defer tic.Stop()
espera:
	for {
		select {
		case <-hecho:
			break espera
		case <-reloj.C:
			c.colgado = true
			_ = cmd.Process.Kill()
			<-hecho
			break espera
		case <-tic.C:
			if raRSS(cmd.Process.Pid) > 512<<20 {
				c.glotona = true
				_ = cmd.Process.Kill()
				<-hecho
				break espera
			}
		}
	}
	c.code = cmd.ProcessState.ExitCode()
	if ru, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage); ok {
		c.maxRSS = uint64(ru.Maxrss)
		if runtime.GOOS == "linux" {
			c.maxRSS *= 1024
		}
	}
	c.out, c.errOut = o.String(), e.String()
	c.stdoutStderr = c.out + "\n" + c.errOut
	return c
}

// raCLIVolvio exige que la corrida haya vuelto sola, a tiempo y sin
// comerse la memoria (tope de 128 MiB residentes, una fraccion de lo que
// cuesta leer /dev/zero).
func raCLIVolvio(t *testing.T, ca, caso string, c raCorrida) {
	t.Helper()
	if c.colgado {
		t.Fatalf("%s: %s: hoom review no volvio a tiempo (lo mato el reloj): esta leyendo algo que no termina\n%s", ca, caso, c.stdoutStderr)
	}
	if c.glotona {
		t.Fatalf("%s: %s: hoom review paso los 512 MiB residentes (lo mato el vigilante): esta leyendo algo que no termina", ca, caso)
	}
	if c.maxRSS > 128<<20 {
		t.Fatalf("%s: %s: hoom review llego a %d MiB residentes: leyo en proporcion a algo que no tenia que leer", ca, caso, c.maxRSS>>20)
	}
}

// CA-416 (enmienda 5), de punta a punta y por la CLI SIN --task, que
// encuentra ./hoom.yaml por su cuenta: con el hoom.yaml RASTREADO cambiado
// sin commitear (una clave desconocida con un secreto, un tipo que no
// parsea, un FIFO, un symlink a /dev/zero) o SIN RASTREAR en un HEAD que no
// lo tiene, `hoom review --provider codex` vuelve enseguida (reloj de 10 s,
// vigilante de memoria) con la negativa por arbol sucio que nombra hoom.yaml
// (no un error de parseo), sale con un codigo distinto de 0, no lanza ninguna
// pasada ni escribe registro, y el secreto no sale por stdout, stderr ni en
// ningun archivo. Control: con el hoom.yaml commiteado la CLI revisa.
func TestCA416_E2EHoomYamlSinCommitearPorLaCLI(t *testing.T) {
	hoom := hbHoomReal(t)

	t.Run("control-hoom-yaml-commiteado", func(t *testing.T) {
		bin := raPATH(t)
		cx := raInstalar(t, bin, "codex", "")
		root := raRepo(t, "")
		raCambio(t, root)
		raLimpio(t, "CA-416", root)
		c := raHoomConReloj(t, hoom, root, 60*time.Second, "review", "--provider", "codex")
		raCLIVolvio(t, "CA-416", "control", c)
		if c.code != 0 || cx.veces() != 1 {
			t.Fatalf("CA-416: control: con el hoom.yaml commiteado la CLI revisa (exit %d, codex %d):\n%s", c.code, cx.veces(), c.stdoutStderr)
		}
	})

	casos := append([]raHoomYamlSucio{}, raHoomYamlSucios...)
	casos = append(casos, raHoomYamlSucio{"sin-rastrear-en-un-head-sin-hoom-yaml", "SECRETO_ZX85", false, nil})
	for _, caso := range casos {
		t.Run(caso.nombre, func(t *testing.T) {
			bin := raPATH(t)
			cx := raInstalar(t, bin, "codex", "")
			var root string
			if caso.plantar == nil {
				root = raRepoSinHoomYaml(t)
				write(t, root, "hoom.yaml", raYAML("review:\n  "+caso.oculto+": x\n"))
			} else {
				root = raRepo(t, "")
				raCambio(t, root)
				raLimpio(t, "CA-416", root)
				caso.plantar(t, root)
			}
			raSoloHoomYamlSucio(t, root)

			c := raHoomConReloj(t, hoom, root, 10*time.Second, "review", "--provider", "codex")
			raCLIVolvio(t, "CA-416", caso.nombre, c)
			if c.code == 0 || cx.veces() != 0 {
				t.Fatalf("CA-416: %s: la CLI se niega sin lanzar ninguna pasada (exit %d, codex %d):\n%s", caso.nombre, c.code, cx.veces(), c.stdoutStderr)
			}
			if ruta, ok := raRutaSucia(c.stdoutStderr); !ok || ruta != "hoom.yaml" {
				t.Fatalf("CA-416: %s: la CLI dice %q antes de leer hoom.yaml:\n%s", caso.nombre, raErrSucio("hoom.yaml"), c.stdoutStderr)
			}
			if strings.Contains(c.stdoutStderr, "clave desconocida") || strings.Contains(c.stdoutStderr, "unmarshal") {
				t.Fatalf("CA-416: %s: la negativa es la de arbol sucio, no un error de parseo:\n%s", caso.nombre, c.stdoutStderr)
			}
			if caso.oculto != "" && strings.Contains(c.stdoutStderr, caso.oculto) {
				t.Fatalf("CA-416: %s: el contenido del hoom.yaml sin commitear (%q) no sale por stdout ni stderr:\n%s", caso.nombre, caso.oculto, c.stdoutStderr)
			}
			if recs, _ := Records(root); len(recs) != 0 {
				t.Fatalf("CA-416: %s: sin pasadas no hay registro: %+v", caso.nombre, recs)
			}
			raSinTextoEnArbol(t, "CA-416: "+caso.nombre, root, caso.oculto, filepath.Join(root, "hoom.yaml"))
		})
	}
}

// CA-414 / CA-416 (enmienda 5), de punta a punta y por la CLI (que toma la
// base de hoom.yaml), sin --lens: control, con la base sana un cambio SOLO de
// documentacion da `hoom review --json` con exit 0 y status sin-revisar; con
// la base rota (base_branch que no existe, clon shallow sin merge-base, un
// arbol de la base que falta) y un cambio de documentacion o de codigo, sale
// con un codigo distinto de 0, no dice SIN REVISAR (ni en texto ni en
// --json), no lanza ninguna pasada ni escribe registro. Y con la base rota Y
// el arbol sucio, la negativa por arbol sucio que nombra la ruta, no el error
// de git.
func TestCA414_E2EBaseRotaSinLensPorLaCLI(t *testing.T) {
	hoom := hbHoomReal(t)
	real := raGitReal(t)

	t.Run("control-base-sana", func(t *testing.T) {
		bin := raPATH(t)
		cx := raInstalar(t, bin, "codex", "")
		root := raRepo(t, "")
		raDocs(t, root)
		raLimpio(t, "CA-414", root)
		c := raHoomConReloj(t, hoom, root, 60*time.Second, "review", "--provider", "codex", "--json")
		raCLIVolvio(t, "CA-414", "control", c)
		var m map[string]any
		if err := json.Unmarshal([]byte(c.out), &m); err != nil || c.code != 0 || m["status"] != "sin-revisar" || cx.veces() != 0 {
			t.Fatalf("CA-414: control: solo documentacion con la base sana es sin-revisar y exit 0 (exit %d, codex %d, %v):\n%s", c.code, cx.veces(), err, c.stdoutStderr)
		}
	})

	rotas := []struct {
		nombre string
		armar  func(t *testing.T, tc raTipoDeCambio) string
	}{
		{"base-inexistente", func(t *testing.T, tc raTipoDeCambio) string {
			root := raConBaseInexistente(t)
			raCambioDe(t, root, tc)
			return root
		}},
		{"clon-shallow-sin-merge-base", func(t *testing.T, tc raTipoDeCambio) string {
			_, clon := raClonShallowDe(t, tc)
			return clon
		}},
		{"falta-el-arbol-de-la-base", func(t *testing.T, tc raTipoDeCambio) string {
			root := raConSubdirEnLaBase(t, tc)
			raBorrarArbolDeLaBase(t, real, root, tc)
			return root
		}},
	}
	for _, tc := range raTiposDeCambio {
		for _, r := range rotas {
			raE2EBaseRota(t, hoom, r.nombre+"/"+tc.nombre, func(t *testing.T) string { return r.armar(t, tc) })
		}
	}
}

// raE2EBaseRota son dos casos de TestCA414_E2EBaseRotaSinLensPorLaCLI: la
// base rota que arma armar, por la CLI sin --lens, con el arbol limpio y con
// el arbol sucio.
func raE2EBaseRota(t *testing.T, hoom, nombre string, armar func(t *testing.T) string) {
	t.Run(nombre, func(t *testing.T) {
		bin := raPATH(t)
		cx := raInstalar(t, bin, "codex", "")
		root := armar(t)
		raLimpio(t, "CA-414", root)
		for _, args := range [][]string{{"review", "--provider", "codex"}, {"review", "--provider", "codex", "--json"}} {
			c := raHoomConReloj(t, hoom, root, 60*time.Second, args...)
			raCLIVolvio(t, "CA-414", nombre, c)
			var m map[string]any
			if json.Unmarshal([]byte(c.out), &m) == nil && m["status"] == "sin-revisar" {
				t.Fatalf("CA-414: %s: %v: con la base rota el resultado nunca es sin-revisar:\n%s", nombre, args, c.stdoutStderr)
			}
			if raDiceSinRevisar(c.stdoutStderr) {
				t.Fatalf("CA-414: %s: %v: con la base rota la CLI nunca dice SIN REVISAR:\n%s", nombre, args, c.stdoutStderr)
			}
			if c.code == 0 {
				t.Fatalf("CA-414: %s: %v: con la base rota la CLI sale con un codigo distinto de 0:\n%s", nombre, args, c.stdoutStderr)
			}
			if _, sucio := raRutaSucia(c.stdoutStderr); sucio {
				t.Fatalf("CA-414: %s: %v: el arbol esta limpio: el error es el de git, no la negativa por arbol sucio:\n%s", nombre, args, c.stdoutStderr)
			}
			if cx.veces() != 0 {
				t.Fatalf("CA-414: %s: %v: sin pasadas (codex %d)", nombre, args, cx.veces())
			}
			if recs, _ := Records(root); len(recs) != 0 {
				t.Fatalf("CA-414: %s: %v: sin registro de review: %+v", nombre, args, recs)
			}
		}
	})

	// orden: la misma base rota con el arbol sucio ademas: va primero la
	// negativa (un fixture nuevo, asi este caso se juzga solo)
	t.Run(nombre+"/con-el-arbol-sucio", func(t *testing.T) {
		bin := raPATH(t)
		cx := raInstalar(t, bin, "codex", "")
		root := armar(t)
		const oculto = "SIN-COMMITEAR-ORDEN-CLI-ZX86"
		write(t, root, "nuevo.go", "package app\n\n// "+oculto+"\n")
		c := raHoomConReloj(t, hoom, root, 60*time.Second, "review", "--provider", "codex")
		raCLIVolvio(t, "CA-416", nombre, c)
		if ruta, ok := raRutaSucia(c.stdoutStderr); !ok || ruta != "nuevo.go" {
			t.Fatalf("CA-416: %s: con el arbol sucio y la base rota va primero %q:\n%s", nombre, raErrSucio("nuevo.go"), c.stdoutStderr)
		}
		if c.code == 0 || cx.veces() != 0 || raDiceSinRevisar(c.stdoutStderr) || strings.Contains(c.stdoutStderr, oculto) {
			t.Fatalf("CA-416: %s: la negativa sale con != 0, sin pasadas, sin SIN REVISAR y sin lo sin commitear (exit %d, codex %d):\n%s",
				nombre, c.code, cx.veces(), c.stdoutStderr)
		}
		if recs, _ := Records(root); len(recs) != 0 {
			t.Fatalf("CA-416: %s: sin registro de review: %+v", nombre, recs)
		}
	})
}
