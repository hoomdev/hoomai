// Ronda 1 del test-writer de contencion-del-writer: regresiones de la review
// de codex sobre el sobre (agentcmd), en cualquier sistema:
//
//   - el Escribe que declara el provider se valida al empezar el sobre
//     (425a75, 03a36d; aclaracion 26);
//   - git-ajeno alrededor de los gates del verify del sobre (318ca0;
//     aclaracion 20) y GitAjenoDelVeredicto;
//   - las fotos con tope de git-ajeno y de la config de los agentes (02b2f7,
//     d9ebb2, c9b147, 775300; aclaracion 27), con topes chicos y archivos
//     dispersos (Truncate), nunca de GB reales.
//
// Reusa los fixtures de contencion_del_writer_test.go (cdwProyecto,
// cdwCorrer, cdwObjeto...). Sin CLI real de IA.
package agentcmd

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- fixtures

// r1EscribeNombre es un provider falso con ConEstado cuyo Escribe pone el
// test (r1PonerEscribe). Su binario se llama como el, y solo esta en el PATH
// del test que lo pone.
const r1EscribeNombre = "r1-escribe"

type r1ProvEscribe struct{}

var (
	r1EscribeMu     sync.Mutex
	r1EscribeActual []string
	r1EscribeOnce   sync.Once
	r1EscribeErr    error
	_               providers.ConEstado = r1ProvEscribe{}
)

func (r1ProvEscribe) Name() string                         { return r1EscribeNombre }
func (r1ProvEscribe) Bin() string                          { return r1EscribeNombre }
func (r1ProvEscribe) Capabilities() providers.Capabilities { return cdwCapacidades() }
func (p r1ProvEscribe) Command(req providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: p.Bin(), Args: []string{req.Prompt}}, nil
}
func (r1ProvEscribe) Normalize(string) []providers.Event { return nil }
func (r1ProvEscribe) Estado() providers.Estado {
	r1EscribeMu.Lock()
	defer r1EscribeMu.Unlock()
	return providers.Estado{Escribe: append([]string(nil), r1EscribeActual...)}
}

// r1PonerEscribe registra el provider falso (una vez por binario) y deja su
// Escribe en escribe hasta que termina el test.
func r1PonerEscribe(t *testing.T, escribe []string) {
	t.Helper()
	r1EscribeOnce.Do(func() { r1EscribeErr = providers.Default.Register(r1ProvEscribe{}) })
	if r1EscribeErr != nil {
		t.Fatalf("fixture: registrar %s: %v", r1EscribeNombre, r1EscribeErr)
	}
	r1EscribeMu.Lock()
	r1EscribeActual = append([]string(nil), escribe...)
	r1EscribeMu.Unlock()
	t.Cleanup(func() {
		r1EscribeMu.Lock()
		r1EscribeActual = nil
		r1EscribeMu.Unlock()
	})
}

func r1Sha(data string) string {
	sum := sha1.Sum([]byte(fmt.Sprintf("blob %d\x00%s", len(data), data)))
	return hex.EncodeToString(sum[:])
}

// r1Veredicto es el veredicto id de cualquiera de los dirs, y la lista que
// lo trae (para LatestComplete).
func r1Veredicto(t *testing.T, id string, dirs ...string) (*verdict.Verdict, []*verdict.Verdict) {
	t.Helper()
	for _, d := range dirs {
		all, err := verdict.LoadAll(d)
		if err != nil {
			continue
		}
		for _, v := range all {
			if v.ID == id {
				return v, all
			}
		}
	}
	t.Fatalf("el veredicto %s esta en disco en %v", id, dirs)
	return nil, nil
}

func r1GateDe(v *verdict.Verdict, nombre string) *verdict.GateResult {
	for i := range v.Gates {
		if v.Gates[i].Name == nombre {
			return &v.Gates[i]
		}
	}
	return nil
}

func r1Lineas(s string) []string {
	var out []string
	for _, l := range strings.Split(strings.TrimSpace(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

func r1Tiene(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// r1Disperso crea p como un archivo disperso de n bytes (Truncate: no ocupa
// disco).
func r1Disperso(t *testing.T, p string, n int64) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(n); err != nil {
		f.Close()
		t.Fatalf("fixture: un archivo disperso de %d bytes: %v", n, err)
	}
	f.Close()
}

// r1Agregar agrega data al final de p (con permiso de escritura).
func r1Agregar(t *testing.T, p, data string) {
	t.Helper()
	if err := os.Chmod(p, 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
	f.Close()
}

// ---------------------------------------------------------------- punto 3

// CA-690 y CA-678 (ids 425a75 y 03a36d; aclaracion 26): con la seccion, un
// provider falso con ConEstado cuyo Escribe es `/`, el $HOME (como "~/" o
// absoluto), el checkout principal o algo dentro del directorio comun niega
// el sobre en run, exit 1, sin lanzar (ni run ni CLI), con el error de
// encierro.ValidarEscribe; un Escribe valido corre como hoy.
func TestR1_425a75_CA690_CA678_ElEscribeDelProviderSeValidaAlEmpezarElSobre(t *testing.T) {
	const ca = "CA-690"
	for _, c := range []struct {
		nombre  string
		escribe func(root, home, comun string) []string
		texto   func(root, home, comun string) string
	}{
		{"todo el disco", func(r, h, c string) []string { return []string{"/"} },
			func(r, h, c string) string {
				return "encierro: r1-escribe escribe /: es todo el disco o todo el directorio personal: no puede ser escribible"
			}},
		{"el $HOME como ~/", func(r, h, c string) []string { return []string{"~/"} },
			func(r, h, c string) string {
				return "encierro: r1-escribe escribe ~/: es todo el disco o todo el directorio personal: no puede ser escribible"
			}},
		{"el $HOME absoluto", func(r, h, c string) []string { return []string{h} },
			func(r, h, c string) string {
				return "encierro: r1-escribe escribe " + h + ": es todo el disco o todo el directorio personal: no puede ser escribible"
			}},
		{"el checkout principal", func(r, h, c string) []string { return []string{r} },
			func(r, h, c string) string {
				return "encierro: r1-escribe escribe " + r + ": contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
			}},
		{"dentro del directorio comun", func(r, h, c string) []string { return []string{filepath.Join(c, "objects")} },
			func(r, h, c string) string {
				return "encierro: r1-escribe escribe " + filepath.Join(c, "objects") + ": contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
			}},
	} {
		root, wt := cdwProyecto(t, cdwSeccion)
		home, comun := os.Getenv("HOME"), cdwComun(t, wt)
		escribe := c.escribe(root, home, comun)
		r1PonerEscribe(t, escribe)
		want := c.texto(root, home, comun)
		if err := encierro.ValidarEscribe(r1EscribeNombre, escribe, root, comun); err == nil || err.Error() != want {
			t.Fatalf("%s: %s: encierro.ValidarEscribe da el texto del contrato: %v; quiero %q", ca, c.nombre, err, want)
		}
		marca := filepath.Join(t.TempDir(), "corrio")
		antes := runsCount(t, root) + runsCount(t, wt)
		res, out := cdwCorrer(t, root, "writer", aeaTarea, "( : > '"+marca+"' ) 2>/dev/null\n"+ar1Codigo+"exit 0\n", Options{Provider: r1EscribeNombre})
		if res.Stage != "run" || res.ExitCode != 1 || res.RunID != "" || res.Note != want || cdwExiste(marca) {
			t.Fatalf("%s: %s: el sobre se niega en run, exit 1, sin lanzar, con %q: %+v\n%s", ca, c.nombre, want, res, out)
		}
		if n := runsCount(t, root) + runsCount(t, wt); n != antes {
			t.Fatalf("%s: %s: ningun run: %d -> %d", ca, c.nombre, antes, n)
		}
		if r := aeaRegistro(t, root, res.EnvelopeID); r.Stage != "run" || r.Note != want {
			t.Fatalf("%s: %s: el registro dice lo mismo: %+v", ca, c.nombre, r)
		}
	}
	// control: un Escribe valido corre como hoy
	root, _ := cdwProyecto(t, cdwSeccion)
	r1PonerEscribe(t, []string{t.TempDir(), "~/.r1-escribe/sesiones"})
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{Provider: r1EscribeNombre})
	if res.RunID == "" || res.Stage == "run" || strings.Contains(res.Note, "no puede ser escribible") {
		t.Fatalf("%s: control: un Escribe valido corre como hoy: %+v\n%s", ca, res, out)
	}
}

// ---------------------------------------------------------------- punto 4

// CA-685 y CA-687 (id 318ca0; aclaracion 20): un gate del verify del sobre
// del writer que crea una ref bajo refs/replace/ o reescribe un objeto suelto
// que existia deja el gate git_ajeno requerido, en fail, de scope full, con
// la ruta en su salida; el veredicto rojo (LatestComplete no lo da como
// verde); el sobre cerrado en verify, no-entregable, exit 1, con un hallazgo
// high de git-ajeno que DeGitAjeno reconoce (el que lee el piloto para
// TocoGitAjeno); y GitAjenoDelVeredicto con esa ruta. Sin la seccion, y con
// la seccion y el gate en gates_sin_encierro.
func TestR1_318ca0_CA685_CA687_UnGateQueCambiaGitAjenoDejaRojoElSobreDelWriter(t *testing.T) {
	const ca = "CA-685"
	for _, extra := range []string{"", "encierro:\n  exigir: no\n  gates_sin_encierro:\n    - test\n"} {
		for _, c := range []struct {
			nombre string
			// arma devuelve el comando del gate (entre comillas dobles en el
			// YAML) y la ruta que tiene que nombrar; el objeto que reescribe
			// existe antes del sobre
			arma func(t *testing.T, comun string) (gate, ruta string)
		}{
			{"crea una ref de reemplazo", func(t *testing.T, comun string) (string, string) {
				const a, b = "r1-gate-a", "r1-gate-b"
				return "x=$(printf " + a + " | git hash-object -w --stdin); y=$(printf " + b + " | git hash-object -w --stdin); git replace -f $x $y; true",
					filepath.Join(comun, "refs", "replace", r1Sha(a))
			}},
			{"reescribe un objeto suelto que existia", func(t *testing.T, comun string) (string, string) {
				_, p := cdwObjeto(t, comun, fmt.Sprintf("r1 un objeto que el gate reescribe %d", rand.Int()))
				return "chmod u+w " + p + "; printf basura >> " + p + "; true", p
			}},
		} {
			caso := c.nombre
			if extra != "" {
				caso += " (gates_sin_encierro)"
			}
			cdwHome(t)
			// el proyecto se arma con un gate de relleno: el de verdad
			// necesita el directorio comun, que existe recien despues
			root, wt := aeaProyecto(t, "true", extra)
			comun := cdwComun(t, wt)
			gate, ruta := c.arma(t, comun)
			r1CambiarGate(t, root, wt, gate)
			res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
			if res.Stage != "verify" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 1 || res.VerdictID == "" || res.Verdict == "green" {
				t.Fatalf("%s: %s: el sobre del writer cierra en verify, no-entregable, exit 1, con veredicto rojo: %+v\n%s", ca, caso, res, out)
			}
			v, all := r1Veredicto(t, res.VerdictID, wt, root)
			g := r1GateDe(v, "git_ajeno")
			if g == nil || !g.Required || g.Status != verdict.StatusFail || g.Scope != "full" || !r1Tiene(r1Lineas(g.OutputTail), ruta) {
				t.Fatalf("%s: %s: el veredicto trae git_ajeno requerido, en fail, scope full, con %s en su salida: %+v (gates %+v)", ca, caso, ruta, g, v.Gates)
			}
			if v.Verdict != "red" {
				t.Fatalf("%s: %s: el veredicto queda rojo: %s", ca, caso, v.Verdict)
			}
			if lc := verdict.LatestComplete(all); lc != nil && lc.Verdict == "green" {
				t.Fatalf("%s: %s: verdict.LatestComplete no da un verde: %s", ca, caso, lc.ID)
			}
			if got := GitAjenoDelVeredicto(v); !r1Tiene(got, ruta) {
				t.Fatalf("%s: %s: GitAjenoDelVeredicto nombra %s: %q", ca, caso, ruta, got)
			}
			pre := "git-ajeno: el rol writer escribio " + ruta + " - "
			hay := false
			for _, h := range ar6DelGate(t, wt, root) {
				if h.Severity == "high" && strings.HasPrefix(h.Description, pre) && DeGitAjeno(h.Finding, "writer") && !DeMaterialDeGit(h.Finding, "writer") {
					hay = true
				}
			}
			if !hay {
				t.Fatalf("%s: %s: el sobre del writer registra el hallazgo high %q...: %+v", ca, caso, pre, ar6DelGate(t, wt, root))
			}
		}
	}
}

// r1CambiarGate cambia el comando del gate test (el "true" de aeaProyecto)
// en el hoom.yaml del principal y del arbol, y lo commitea en los dos.
func r1CambiarGate(t *testing.T, root, wt, gate string) {
	t.Helper()
	for _, d := range []string{root, wt} {
		p := filepath.Join(d, "hoom.yaml")
		raw, err := os.ReadFile(p)
		if err != nil || !strings.Contains(string(raw), "cmd: \"true\"") {
			t.Fatalf("fixture: el hoom.yaml de %s tiene el gate de relleno: %v\n%s", d, err, raw)
		}
		write(t, d, "hoom.yaml", strings.Replace(string(raw), "cmd: \"true\"", "cmd: \""+gate+"\"", 1))
		git(t, d, "add", "hoom.yaml")
		git(t, d, "commit", "-q", "-m", "el gate de la ronda 1")
	}
}

// CA-685 (id 318ca0): un gate que solo crea objetos nuevos (y mueve otra
// rama) no es git-ajeno: no hay gate git_ajeno, ni hallazgo, y
// GitAjenoDelVeredicto es nil.
func TestR1_318ca0_CA685_UnGateQueSoloCreaObjetosNoEsGitAjeno(t *testing.T) {
	const ca = "CA-685"
	cdwHome(t)
	root, wt := aeaProyecto(t, "printf r1-nuevo | git hash-object -w --stdin >/dev/null; git branch -f r1-otra HEAD; true", "")
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
	if res.VerdictID == "" {
		t.Fatalf("%s: fixture: el verify del sobre corrio: %+v\n%s", ca, res, out)
	}
	v, _ := r1Veredicto(t, res.VerdictID, wt, root)
	if g := r1GateDe(v, "git_ajeno"); g != nil || GitAjenoDelVeredicto(v) != nil {
		t.Fatalf("%s: crear objetos nuevos no es git-ajeno: %+v %q", ca, g, GitAjenoDelVeredicto(v))
	}
	for _, h := range ar6DelGate(t, wt, root) {
		if DeGitAjeno(h.Finding, "writer") {
			t.Fatalf("%s: sin hallazgo de git-ajeno: %+v", ca, h)
		}
	}
}

// CA-685 (id 318ca0; aclaracion 20): GitAjenoDelVeredicto lista las rutas del
// gate git_ajeno en fail, una por linea de su salida; nil si el gate no esta
// o no esta en fail.
func TestR1_318ca0_CA685_GitAjenoDelVeredicto(t *testing.T) {
	const ca = "CA-685"
	con := func(gs ...verdict.GateResult) *verdict.Verdict {
		return &verdict.Verdict{Gates: append([]verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass}}, gs...)}
	}
	a, b := "/repo/.git/refs/replace/aaaa", "/repo/.git/objects/ab/cdef"
	if got := GitAjenoDelVeredicto(con(verdict.GateResult{Name: "git_ajeno", Required: true, Status: verdict.StatusFail, Scope: "full", OutputTail: a + "\n" + b + "\n"})); len(got) != 2 || got[0] != a || got[1] != b {
		t.Errorf("%s: una ruta por linea de la salida: %q", ca, got)
	}
	if got := GitAjenoDelVeredicto(con(verdict.GateResult{Name: "git_ajeno", Required: true, Status: verdict.StatusPass, OutputTail: a})); got != nil {
		t.Errorf("%s: el gate que no esta en fail no cuenta: %q", ca, got)
	}
	if got := GitAjenoDelVeredicto(con(verdict.GateResult{Name: "material_de_git", Required: true, Status: verdict.StatusFail, OutputTail: a})); got != nil {
		t.Errorf("%s: otro gate no es git_ajeno: %q", ca, got)
	}
	if got := GitAjenoDelVeredicto(con()); got != nil {
		t.Errorf("%s: sin el gate, nil: %q", ca, got)
	}
}

// ---------------------------------------------------------------- punto 6

// r1Plazo corre f y falla si tarda mas que plazo (sin colgar el test: lo
// espera hasta 4 veces el plazo y despues sigue).
func r1Plazo(t *testing.T, ca, que string, plazo time.Duration, f func()) {
	t.Helper()
	listo := make(chan struct{})
	desde := time.Now()
	go func() { f(); close(listo) }()
	select {
	case <-listo:
	case <-time.After(4 * plazo):
		t.Fatalf("%s: %s: no vuelve ni a %s (plazo %s): cuelga", ca, que, 4*plazo, plazo)
	}
	if d := time.Since(desde); d > plazo {
		t.Fatalf("%s: %s: vuelve dentro de %s; tardo %s", ca, que, plazo, d)
	}
}

// CA-685 (id 02b2f7; aclaracion 27): un objeto suelto grande y nuevo
// (disperso, 8 GiB) no cuelga la comparacion con un tope chico: vuelve en un
// plazo acotado y no lo nombra (crear no es git-ajeno). La foto de un arbol
// con un objeto asi que ya existia tambien vuelve en plazo.
func TestR1_02b2f7_CA685_UnObjetoGrandeNuevoNoCuelgaLaComparacion(t *testing.T) {
	const ca = "CA-685"
	_, wt := cdwProyecto(t, "")
	comun := cdwComun(t, wt)
	antes := FotoGitAjenoConTope(wt, TopeFotoGitAjeno, 1<<20)
	nuevo := filepath.Join(comun, "objects", "ab", strings.Repeat("c", 38))
	r1Disperso(t, nuevo, 8<<30)
	var cambios []string
	r1Plazo(t, ca, "CambiosGitAjenoConTope con un objeto disperso de 8 GiB nuevo", 6*time.Second, func() {
		cambios = CambiosGitAjenoConTope(antes, 500*time.Millisecond, 1<<20)
	})
	if r1Tiene(cambios, nuevo) {
		t.Fatalf("%s: un objeto nuevo no es git-ajeno aunque sea grande: %q", ca, cambios)
	}
	// ahora ya existe: la foto lo guarda sin leerlo entero
	r1Plazo(t, ca, "FotoGitAjenoConTope con un objeto disperso de 8 GiB que existe", 6*time.Second, func() {
		_ = FotoGitAjenoConTope(wt, 500*time.Millisecond, 1<<20)
	})
}

// CA-685 (id d9ebb2; aclaracion 27): reescribir un objeto que existia y que
// pasa maxLeido cuenta como cambiado, por su ruta: cuando la foto no lo leyo
// entero (maxLeido chico en la foto) y cuando la comparacion no lo lee
// entero (maxLeido chico al comparar), tambien con uno disperso de 8 GiB. Uno
// grande que nadie toca no cuenta.
func TestR1_d9ebb2_CA685_ReescribirUnObjetoMasAllaDeMaxLeidoCuentaComoCambiado(t *testing.T) {
	const ca = "CA-685"
	_, wt := cdwProyecto(t, "")
	comun := cdwComun(t, wt)
	grande := func(n int) string {
		r := rand.New(rand.NewSource(int64(n)))
		b := make([]byte, 4096)
		for i := range b {
			b[i] = byte('a' + r.Intn(26))
		}
		return string(b)
	}
	_, tocado := cdwObjeto(t, comun, grande(1))
	_, quieto := cdwObjeto(t, comun, grande(2))
	if st, err := os.Stat(tocado); err != nil || st.Size() <= 64 {
		t.Fatalf("fixture: el objeto pasa los 64 bytes de maxLeido: %v %v", st, err)
	}
	// la foto no los lee enteros
	antes := FotoGitAjenoConTope(wt, TopeFotoGitAjeno, 64)
	time.Sleep(20 * time.Millisecond)
	r1Agregar(t, tocado, "basura")
	got := CambiosGitAjenoConTope(antes, TopeFotoGitAjeno, 64)
	if !r1Tiene(got, tocado) || r1Tiene(got, quieto) {
		t.Fatalf("%s: con la foto sin leerlo entero, reescribirlo cuenta como cambiado por su ruta (%s), y el que nadie toca no (%s): %q", ca, tocado, quieto, got)
	}
	// la comparacion no lo lee entero
	_, otro := cdwObjeto(t, comun, grande(3))
	antes = FotoGitAjenoConTope(wt, TopeFotoGitAjeno, MaxLeidoGitAjeno)
	time.Sleep(20 * time.Millisecond)
	r1Agregar(t, otro, "basura")
	if got := CambiosGitAjenoConTope(antes, TopeFotoGitAjeno, 64); !r1Tiene(got, otro) || r1Tiene(got, quieto) {
		t.Fatalf("%s: con la comparacion sin leerlo entero, reescribirlo cuenta como cambiado por su ruta: %q", ca, got)
	}
	// uno disperso de 8 GiB que existia y crece un byte
	disperso := filepath.Join(comun, "objects", "cd", strings.Repeat("e", 38))
	r1Disperso(t, disperso, 8<<30)
	var foto GitAjeno
	r1Plazo(t, ca, "la foto con un objeto disperso de 8 GiB", 6*time.Second, func() { foto = FotoGitAjenoConTope(wt, time.Second, 1<<20) })
	time.Sleep(20 * time.Millisecond)
	r1Agregar(t, disperso, "x")
	r1Plazo(t, ca, "la comparacion con un objeto disperso de 8 GiB reescrito", 6*time.Second, func() { got = CambiosGitAjenoConTope(foto, time.Second, 1<<20) })
	if !r1Tiene(got, disperso) {
		t.Fatalf("%s: el objeto grande que existia y cambio cuenta como cambiado por su ruta: %q", ca, got)
	}
}

// CA-685 (id c9b147; aclaracion 27): una foto de git-ajeno que pasa su tiempo
// da lugar a FotoSinTerminar en cualquier comparacion contra ella, aunque nada
// cambie; una comparacion que pasa su tiempo suma FotoSinTerminar.
func TestR1_c9b147_CA685_UnaFotoQuePasaSuTiempoEsFotoSinTerminar(t *testing.T) {
	const ca = "CA-685"
	_, wt := cdwProyecto(t, "")
	comun := cdwComun(t, wt)
	var objetos []string
	for i := 0; i < 300; i++ {
		_, p := cdwObjeto(t, comun, fmt.Sprintf("r1 objeto %d", i))
		objetos = append(objetos, p)
	}
	sinTerminar := FotoGitAjenoConTope(wt, time.Nanosecond, MaxLeidoGitAjeno)
	if got := CambiosGitAjeno(sinTerminar); !r1Tiene(got, FotoSinTerminar) {
		t.Fatalf("%s: contra una foto que paso su tiempo, la comparacion da %s aunque nada cambie: %q", ca, FotoSinTerminar, got)
	}
	completa := FotoGitAjeno(wt)
	if got := CambiosGitAjeno(completa); got != nil {
		t.Fatalf("%s: control: contra una foto completa y sin cambios, nil: %q", ca, got)
	}
	// la comparacion tiene que releer los 300 (les cambia el mtime) y no le
	// alcanza el tiempo
	futuro := time.Now().Add(time.Hour)
	for _, p := range objetos {
		if err := os.Chtimes(p, futuro, futuro); err != nil {
			t.Fatal(err)
		}
	}
	if got := CambiosGitAjenoConTope(completa, time.Nanosecond, MaxLeidoGitAjeno); !r1Tiene(got, FotoSinTerminar) {
		t.Fatalf("%s: una comparacion que pasa su tiempo suma %s: %q", ca, FotoSinTerminar, got)
	}
}

// CA-689 (id 775300; aclaracion 27): la foto de la config de los agentes con
// tope: un .claude/ con muchas entradas y un tope minimo es FotoSinTerminar
// sola, y contra ella (en los dos sentidos, y contra otra igual) la
// comparacion da [FotoSinTerminar]; un ~/.claude.json mas grande que
// maxLeido (disperso, 8 GiB) no se decodifica, vuelve rapido, su Huella es la
// de un archivo grande, y crecerle un byte lo cambia.
func TestR1_775300_CA689_LaFotoDeLaConfigConTope(t *testing.T) {
	const ca = "CA-689"
	_, wt := cdwProyecto(t, "")
	for i := 0; i < 3000; i++ {
		cdwEscribir(t, filepath.Join(wt, ".claude", "commands", fmt.Sprintf("c%04d.md", i)), "x")
	}
	completa := FotoConfigAgentes(wt)
	if _, ok := completa[FotoSinTerminar]; ok || len(completa) < 3000 {
		t.Fatalf("%s: fixture: con su tope la foto termina: %d entradas", ca, len(completa))
	}
	sin := FotoConfigAgentesConTope(wt, time.Nanosecond, MaxLeidoConfigAgentes)
	if _, ok := sin[FotoSinTerminar]; !ok || len(sin) != 1 {
		t.Fatalf("%s: la foto que pasa su tiempo es FotoSinTerminar sola: %d entradas", ca, len(sin))
	}
	otra := FotoConfigAgentesConTope(wt, time.Nanosecond, MaxLeidoConfigAgentes)
	for _, c := range []struct {
		nombre      string
		antes, desp ConfigAgentes
	}{{"completa contra sin terminar", completa, sin}, {"sin terminar contra completa", sin, completa}, {"sin terminar contra sin terminar", sin, otra}} {
		if got := CambiosConfigAgentes(c.antes, c.desp); len(got) != 1 || got[0] != FotoSinTerminar {
			t.Errorf("%s: %s: CambiosConfigAgentes = %q; quiero [%s]", ca, c.nombre, got, FotoSinTerminar)
		}
	}

	claudeJSON := filepath.Join(os.Getenv("HOME"), ".claude.json")
	r1Disperso(t, claudeJSON, 8<<30)
	var foto ConfigAgentes
	r1Plazo(t, ca, "FotoConfigAgentesConTope con un ~/.claude.json disperso de 8 GiB", 6*time.Second, func() {
		foto = FotoConfigAgentesConTope(wt, TopeFotoConfigAgentes, 1<<20)
	})
	if h, ok := foto[claudeJSON]; !ok || !strings.HasPrefix(h, HuellaGrande) {
		t.Fatalf("%s: un ~/.claude.json mas grande que maxLeido no se decodifica: su Huella es la de un archivo grande (%q): %q %v", ca, HuellaGrande, h, ok)
	}
	time.Sleep(20 * time.Millisecond)
	r1Agregar(t, claudeJSON, "x")
	var despues ConfigAgentes
	r1Plazo(t, ca, "la foto otra vez", 6*time.Second, func() { despues = FotoConfigAgentesConTope(wt, TopeFotoConfigAgentes, 1<<20) })
	if got := CambiosConfigAgentes(foto, despues); !r1Tiene(got, claudeJSON) {
		t.Fatalf("%s: crecerle un byte al ~/.claude.json grande lo cambia: %q", ca, got)
	}
}
