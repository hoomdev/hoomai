// Tests adversariales del spec .hoom/specs/ciego-sin-config-personal.md
// (CA-588, CA-590, CA-591, CA-592) sobre `hoom agent`: el rol ciego corre
// siempre con Isolated (sin la configuracion personal del CLI), y un provider
// que no puede aislarse no corre un rol ciego: el sobre cierra en aislar
// antes de cualquier otro chequeo del arbol ciego.
//
// Re-expresa CA-400 solo para el rol ciego: ver
// TestCA400_ElSobreSoloAislaAlRolCiegoYNuncaPideEsfuerzo en
// review_aislada_test.go.
package agentcmd

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// cscCinco son los cinco argumentos del aislamiento de claude (CA-589).
var cscCinco = []string{"--strict-mcp-config", "--setting-sources", "project", "--settings", `{"autoMemoryEnabled":false}`}

// cscSinAislarNombre es el provider falso: system prompt y casi todo lo de
// claude, pero SIN Isolation. Su binario nunca esta en el PATH salvo en el
// test que lo pone, asi que no cambia el provider por defecto de nadie.
const (
	cscSinAislarNombre = "sin-aislar-ca592"
	cscSinAislarBin    = "hoom-sin-aislar-ca592"
)

type cscSinAislar struct{}

func (cscSinAislar) Name() string { return cscSinAislarNombre }
func (cscSinAislar) Bin() string  { return cscSinAislarBin }
func (cscSinAislar) Capabilities() providers.Capabilities {
	return providers.Capabilities{Structured: true, SessionID: true, Model: true, SystemPrompt: true,
		Tools: true, ReadOnly: true, NoExec: true, Unattended: true, MaxTurns: true, Budget: true, Effort: true}
}
func (p cscSinAislar) Command(req providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: p.Bin(), Args: []string{req.Prompt}}, nil
}
func (cscSinAislar) Normalize(string) []providers.Event { return nil }

var cscRegistro sync.Once

// cscRegistrar deja el provider falso en providers.Default una sola vez por
// binario de test (no hay Unregister).
func cscRegistrar(t *testing.T) providers.Provider {
	t.Helper()
	var err error
	cscRegistro.Do(func() { err = providers.Default.Register(cscSinAislar{}) })
	if err != nil {
		t.Fatalf("CA-592: fixture: registrar el provider sin Isolation: %v", err)
	}
	p, err := providers.Lookup(cscSinAislarNombre)
	if err != nil {
		t.Fatalf("CA-592: fixture: el provider sin Isolation esta registrado: %v", err)
	}
	return p
}

// cscError es el error de dos lineas del contrato, literal.
func cscError(name, lista string) string {
	return name + " no puede correr sin tu configuracion personal (MCP, memoria, hooks): un rol ciego no corre ahi.\n" +
		"  Accion: usa un provider que pueda aislarse: " + lista
}

// cscListaRegistrada son los providers registrados que declaran Isolation, en
// el orden del registro.
func cscListaRegistrada() string {
	var ns []string
	for _, p := range providers.All() {
		if p.Capabilities().Isolation {
			ns = append(ns, p.Name())
		}
	}
	return strings.Join(ns, ", ")
}

func cscRol(t *testing.T, slug string) agents.Role {
	t.Helper()
	r, err := agents.Lookup(slug)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func cscProv(t *testing.T, name string) providers.Provider {
	t.Helper()
	p, err := providers.Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func cscBloques(args []string) int {
	n := 0
	for i := 0; i+len(cscCinco) <= len(args); i++ {
		if reflect.DeepEqual(args[i:i+len(cscCinco)], cscCinco) {
			n++
		}
	}
	return n
}

func cscCount(args []string, a string) int {
	n := 0
	for _, x := range args {
		if x == a {
			n++
		}
	}
	return n
}

// cscAisladoClaude: los cinco seguidos, una vez, despues de -p.
func cscAisladoClaude(t *testing.T, que string, args []string) {
	t.Helper()
	if cscBloques(args) != 1 {
		t.Fatalf("CA-590: %s lanza claude con %v seguidos una vez: %v", que, cscCinco, args)
	}
	p := -1
	for i, a := range args {
		if a == "-p" {
			p = i
			break
		}
	}
	for i := range args {
		if i+len(cscCinco) <= len(args) && reflect.DeepEqual(args[i:i+len(cscCinco)], cscCinco) && (p < 0 || i < p) {
			t.Fatalf("CA-590: %s: los cinco van despues de -p: %v", que, args)
		}
	}
}

// cscNingunAislamiento: ni los cinco de claude ni el de codex.
func cscNingunAislamiento(t *testing.T, que string, args []string) {
	t.Helper()
	for _, a := range args {
		if a == "--ignore-user-config" || a == "--settings" || a == `{"autoMemoryEnabled":false}` ||
			a == "--strict-mcp-config" || a == "--setting-sources" {
			t.Fatalf("CA-590: %s no lleva aislamiento (%q): %v", que, a, args)
		}
	}
}

// ---------------------------------------------------------------- CA-588

// CA-588: startOptions da Isolated: true para el test-writer, con claude y
// con codex, e Isolated: false para los otros nueve roles, que se nombran uno
// por uno; el esfuerzo sigue sin pedirse (el resto de CA-400).
func TestCA588_StartOptionsAislaSoloAlTestWriter(t *testing.T) {
	noCiegos := []string{"orquestador", "arquitecto", "designer", "scout", "writer", "reviewer",
		"characterizer", "analista", "refutador"}
	if len(agents.Roles()) != len(noCiegos)+1 {
		t.Fatalf("CA-588: fixture: hoy son 10 roles: %d", len(agents.Roles()))
	}
	for _, name := range []string{"claude", "codex"} {
		prov := cscProv(t, name)
		so, _ := startOptions(prov, cscRol(t, "test-writer"), "C", Options{Prompt: "escribi los tests", Model: "m"})
		if !so.Isolated {
			t.Fatalf("CA-588: el test-writer en %s corre con Isolated: %+v", name, so)
		}
		if so.Effort != "" {
			t.Fatalf("CA-588/CA-400: el test-writer en %s sigue sin esfuerzo pedido: %+v", name, so)
		}
		for _, slug := range noCiegos {
			so, _ := startOptions(prov, cscRol(t, slug), "C", Options{Prompt: "trabaja", Model: "m"})
			if so.Isolated || so.Effort != "" {
				t.Fatalf("CA-588/CA-400: el rol %s en %s no lleva Isolated ni Effort: %+v", slug, name, so)
			}
		}
	}
}

// CA-588: Isolated sale de la tabla de roles (Role.Isolated), no del slug:
// para todo rol, so.Isolated == role.Isolated.
func TestCA588_IsolatedSaleDeLaTablaDeRoles(t *testing.T) {
	ciegos := 0
	for _, name := range []string{"claude", "codex"} {
		prov := cscProv(t, name)
		for _, role := range agents.Roles() {
			so, _ := startOptions(prov, role, "C", Options{Prompt: "trabaja"})
			if so.Isolated != role.Isolated {
				t.Fatalf("CA-588: el rol %s en %s: Isolated=%v, la tabla dice %v", role.Slug, name, so.Isolated, role.Isolated)
			}
			if role.Isolated {
				ciegos++
			}
		}
	}
	if ciegos != 2 {
		t.Fatalf("CA-588: fixture: hoy solo el test-writer es ciego (%d)", ciegos)
	}
}

// CA-588 (caso limite --resume): un test-writer que retoma una sesion por id
// sigue aislado; y el resto de sus opciones no cambia.
func TestCA588_ElTestWriterQueRetomaSigueAislado(t *testing.T) {
	tw := cscRol(t, "test-writer")
	for _, name := range []string{"claude", "codex"} {
		prov := cscProv(t, name)
		plano, _ := startOptions(prov, tw, "C", Options{Prompt: "seguimos"})
		so, _ := startOptions(prov, tw, "C", Options{Prompt: "seguimos", ResumeID: "sess-ca588"})
		if !so.Isolated || so.ResumeID != "sess-ca588" {
			t.Fatalf("CA-588: el test-writer en %s retoma aislado: %+v", name, so)
		}
		so.ResumeID = ""
		if !reflect.DeepEqual(plano, so) {
			t.Fatalf("CA-588: retomar no cambia nada mas:\n%+v\n%+v", plano, so)
		}
	}
}

// CA-588 (caso limite: un pedido mas al mismo run): el run del test-writer
// recuerda sus StartOptions, asi que el Input que lo continua tambien lanza
// claude con los cinco argumentos de CA-589.
func TestCA588_UnPedidoMasAlRunCiegoSigueAislado(t *testing.T) {
	dir := t.TempDir()
	fakeProvider(t, "claude", "d='"+dir+"'\n"+
		"n=$(cat \"$d/n\" 2>/dev/null || echo 0); n=$((n+1)); echo $n > \"$d/n\"\n"+
		"printf '%s\\n' \"$@\" > \"$d/argv.$n\"\n"+
		"echo '{\"type\":\"system\",\"subtype\":\"init\",\"session_id\":\"s-ca588\"}'\n"+
		"exit 0\n")
	so, _ := startOptions(cscProv(t, "claude"), cscRol(t, "test-writer"), "C", Options{Prompt: "escribi los tests"})
	so.Provider, so.Dir = "claude", t.TempDir()
	m := runcmd.NewManager(t.TempDir())
	run, err := m.Start(so)
	if err != nil {
		t.Fatalf("CA-588: el run del test-writer arranca: %v", err)
	}
	m.Wait(run.ID)
	if _, err := m.Input(run.ID, "segui con el CA-2"); err != nil {
		t.Fatalf("CA-588: Input al run del test-writer: %v", err)
	}
	m.Wait(run.ID)
	for n, que := range map[string]string{"1": "el primer lanzamiento", "2": "el pedido de mas"} {
		args := raArgvDe(t, filepath.Join(dir, "argv."+n))
		if cscBloques(args) != 1 {
			t.Fatalf("CA-588: %s del test-writer lleva los cinco de CA-589: %v", que, args)
		}
	}
}

// ---------------------------------------------------------------- CA-590

// CA-590: con los CLIs falsos, `hoom agent --role test-writer` lanza claude
// con los cinco argumentos y codex con --ignore-user-config.
func TestCA590_HoomAgentTestWriterLanzaAislado(t *testing.T) {
	for _, name := range []string{"claude", "codex"} {
		t.Run(name, func(t *testing.T) {
			root := repoCiego(t)
			dump := filepath.Join(t.TempDir(), "argv.txt")
			fakeProvider(t, name, "printf '%s\\n' \"$@\" > '"+dump+"'\nexit 0\n")
			var out bytes.Buffer
			res, err := Run(root, "main", Options{Role: "test-writer", Provider: name, Prompt: "escribi los tests"}, &out)
			if err != nil || res.RunID == "" {
				t.Fatalf("CA-590: hoom agent --role test-writer con %s corre: %v %+v\n%s", name, err, res, out.String())
			}
			args := raArgvDe(t, dump)
			if name == "claude" {
				cscAisladoClaude(t, "hoom agent --role test-writer", args)
			} else {
				if cscCount(args, "--ignore-user-config") != 1 {
					t.Fatalf("CA-590: hoom agent --role test-writer lanza codex con --ignore-user-config una vez: %v", args)
				}
				for _, a := range cscCinco {
					if cscCount(args, a) != 0 {
						t.Fatalf("CA-590: codex no lleva los argumentos de claude (%q): %v", a, args)
					}
				}
			}
		})
	}
}

// CA-590: `hoom agent --role writer` no lanza ni claude ni codex con esos
// argumentos.
func TestCA590_HoomAgentWriterNoAisla(t *testing.T) {
	for _, name := range []string{"claude", "codex"} {
		t.Run(name, func(t *testing.T) {
			root := repo(t)
			dump := filepath.Join(t.TempDir(), "argv.txt")
			fakeProvider(t, name, "printf '%s\\n' \"$@\" > '"+dump+"'\nprintf 'package app // hecho\\n' > app.go\nexit 0\n")
			if _, err := Run(root, "main", Options{Role: "writer", Provider: name, Prompt: "implementa"}, io.Discard); err != nil {
				t.Fatalf("CA-590: hoom agent --role writer con %s corre: %v", name, err)
			}
			cscNingunAislamiento(t, "hoom agent --role writer con "+name, raArgvDe(t, dump))
		})
	}
}

// CA-590 (caso limite --resume): `hoom agent --role test-writer --resume`
// retoma la sesion Y sigue aislado, en claude y en codex.
func TestCA590_HoomAgentTestWriterConResumeSigueAislado(t *testing.T) {
	for _, name := range []string{"claude", "codex"} {
		t.Run(name, func(t *testing.T) {
			root := repoCiego(t)
			dump := filepath.Join(t.TempDir(), "argv.txt")
			fakeProvider(t, name, "printf '%s\\n' \"$@\" > '"+dump+"'\nexit 0\n")
			res, err := Run(root, "main", Options{Role: "test-writer", Provider: name, Prompt: "seguimos", ResumeID: "sess-ca590"}, io.Discard)
			if err != nil || res.RunID == "" {
				t.Fatalf("CA-590: el test-writer retoma: %v %+v", err, res)
			}
			args := raArgvDe(t, dump)
			if cscCount(args, "sess-ca590") != 1 {
				t.Fatalf("CA-590: fixture: el argv retoma la sesion: %v", args)
			}
			if name == "claude" {
				cscAisladoClaude(t, "el test-writer que retoma", args)
			} else if cscCount(args, "--ignore-user-config") != 1 {
				t.Fatalf("CA-590: el test-writer que retoma en codex sigue con --ignore-user-config: %v", args)
			}
		})
	}
}

// ---------------------------------------------------------------- CA-591

// CA-591: nil para todo rol sin Isolated con cualquier provider (los
// registrados, el falso sin Isolation, uno mudo y uno sin nada) y para el
// test-writer con claude y con codex.
func TestCA591_IsolationForNilParaRolesNoCiegosYProvidersAislables(t *testing.T) {
	falso := cscRegistrar(t)
	ps := append(append([]providers.Provider{}, providers.All()...), providerMudo{},
		cscVacio{})
	vioFalso := false
	for _, p := range ps {
		if p.Name() == falso.Name() {
			vioFalso = true
		}
		for _, r := range agents.Roles() {
			if r.Isolated {
				continue
			}
			if err := IsolationFor(p, r); err != nil {
				t.Fatalf("CA-591: %s no es ciego: IsolationFor(%s) es nil: %v", r.Slug, p.Name(), err)
			}
		}
	}
	if !vioFalso {
		t.Fatal("CA-591: fixture: el provider falso esta en providers.All()")
	}
	tw := cscRol(t, "test-writer")
	for _, name := range []string{"claude", "codex"} {
		if err := IsolationFor(cscProv(t, name), tw); err != nil {
			t.Fatalf("CA-591: %s puede aislarse: IsolationFor(test-writer) es nil: %v", name, err)
		}
	}
}

// cscVacio no declara ninguna capacidad.
type cscVacio struct{}

func (cscVacio) Name() string                         { return "vacio-ca591" }
func (cscVacio) Bin() string                          { return "hoom-vacio-ca591" }
func (cscVacio) Capabilities() providers.Capabilities { return providers.Capabilities{} }
func (cscVacio) Command(providers.Request) (providers.Invocation, error) {
	return providers.Invocation{}, nil
}
func (cscVacio) Normalize(string) []providers.Event { return nil }

// CA-591: con un provider que no declara Isolation, el test-writer recibe el
// error de dos lineas, con el nombre del provider y la lista claude, codex
// sacada de los providers registrados que declaran Isolation.
func TestCA591_IsolationForNiegaAlTestWriterSinIsolation(t *testing.T) {
	cscRegistrar(t)
	if got := cscListaRegistrada(); got != "claude, codex" {
		t.Fatalf("CA-591: fixture: los registrados con Isolation son claude, codex (el falso no entra): %q", got)
	}
	tw := cscRol(t, "test-writer")
	for _, p := range []providers.Provider{cscSinAislar{}, providerMudo{}, cscVacio{},
		cscProv(t, "gemini"), cscProv(t, "opencode")} {
		err := IsolationFor(p, tw)
		if err == nil {
			t.Fatalf("CA-591: %s no puede aislarse: el test-writer no corre ahi", p.Name())
		}
		want := cscError(p.Name(), "claude, codex")
		if err.Error() != want {
			t.Fatalf("CA-591: el error del contrato, literal:\nquiere %q\nfue    %q", want, err.Error())
		}
		lineas := strings.Split(err.Error(), "\n")
		if len(lineas) != 2 || strings.HasSuffix(lineas[1], ".") {
			t.Fatalf("CA-591: dos lineas y la segunda sin punto final: %q", err.Error())
		}
	}
}

// ---------------------------------------------------------------- CA-592

// cscCorteEnAislar comprueba el cierre de CA-592 en root.
func cscCorteEnAislar(t *testing.T, root string, res Result, out string, veredictosAntes int, dump string) {
	t.Helper()
	want := cscError(cscSinAislarNombre, "claude, codex")
	if res.Stage != "aislar" || res.ExitCode != 1 {
		t.Fatalf("CA-592: el sobre cierra en aislar con exit 1: %+v\n%s", res, out)
	}
	if res.RunID != "" || res.VerdictID != "" || res.Verdict != "" || res.Check != nil {
		t.Fatalf("CA-592: sin run ni veredicto: %+v", res)
	}
	if res.Provider != cscSinAislarNombre {
		t.Fatalf("CA-592: el Result nombra el provider: %+v", res)
	}
	for _, l := range strings.Split(want, "\n") {
		if !strings.Contains(out, strings.TrimSpace(l)) {
			t.Fatalf("CA-592: la salida del sobre trae la nota de CA-591 (%q):\n%s", l, out)
		}
	}
	recs := sobres(t, root)
	if len(recs) != 1 {
		t.Fatalf("CA-592: un sobre, un registro: %+v", recs)
	}
	rec := recs[0]
	if rec.ID != res.EnvelopeID || rec.Stage != "aislar" || rec.ExitCode != 1 || rec.RunID != "" || rec.VerdictID != "" {
		t.Fatalf("CA-592: el registro cierra en aislar con exit 1, sin run ni veredicto: %+v", rec)
	}
	if strings.TrimSpace(rec.Note) != want {
		t.Fatalf("CA-592: la nota del registro es el error de CA-591:\nquiere %q\nfue    %q", want, rec.Note)
	}
	if rec.EndedAt.IsZero() {
		t.Fatalf("CA-592: el registro queda cerrado: %+v", rec)
	}
	// CA-592 (hallazgo b5bf6b): la nota de CA-591 viaja tambien en el Result,
	// literal, y es la misma que guarda el registro del sobre.
	cscNotaComoElRegistro(t, res, rec.Note)
	if res.Note != want {
		t.Fatalf("CA-592: el Result trae la nota de CA-591, literal:\nquiere %q\nfue    %q", want, res.Note)
	}
	if existeEn(t, filepath.Join(root, ".hoom", "isolated")) && len(filesIn(t, filepath.Join(root, ".hoom", "isolated"))) != 0 {
		t.Fatalf("CA-592: nada en .hoom/isolated/: %v", filesIn(t, filepath.Join(root, ".hoom", "isolated")))
	}
	if n := runsCount(t, root); n != 0 {
		t.Fatalf("CA-592: ningun run en .hoom/runs/: %d", n)
	}
	if n := len(filesIn(t, filepath.Join(root, ".hoom", "verdicts"))); n != veredictosAntes {
		t.Fatalf("CA-592: sin veredicto nuevo: %d -> %d", veredictosAntes, n)
	}
	if existeEn(t, dump) {
		t.Fatal("CA-592: el CLI del provider sin Isolation nunca se lanza")
	}
}

// cscNotaComoElRegistro: CA-592 — res.Note es exactamente la nota del registro
// del sobre (no vacia), y el JSON del Result (lo que lee `hoom agent --json`)
// la trae en la clave "note", con el mismo texto.
func cscNotaComoElRegistro(t *testing.T, res Result, recNote string) {
	t.Helper()
	if strings.TrimSpace(recNote) == "" {
		t.Fatalf("CA-592: fixture: el registro del sobre guarda una nota: %q", recNote)
	}
	if res.Note != recNote {
		t.Fatalf("CA-592: el Result trae la misma nota que su registro:\nregistro %q\nresult   %q", recNote, res.Note)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("CA-592: el Result se serializa: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-592: el JSON del Result es un objeto: %v\n%s", err, raw)
	}
	got, ok := m["note"].(string)
	if !ok || got != recNote {
		t.Fatalf("CA-592: el JSON del Result trae \"note\" con la nota del registro:\nquiere %q\nfue    %v (%s)", recNote, m["note"], raw)
	}
}

// cscPonerBin pone en el PATH (solo en este test) el binario del provider
// falso, que deja rastro si alguien lo lanza.
func cscPonerBin(t *testing.T) string {
	t.Helper()
	dump := filepath.Join(t.TempDir(), "lanzado.txt")
	fakeProvider(t, cscSinAislarBin, "printf '%s\\n' \"$@\" > '"+dump+"'\nexit 0\n")
	return dump
}

// CA-592: con un provider registrado que tiene system prompt y no declara
// Isolation, `hoom agent --role test-writer` (con un spec aprobado y sin
// spec) cierra en aislar con exit 1 y la nota de CA-591, sin cuarentena, sin
// run y sin veredicto.
func TestCA592_HoomAgentTestWriterSinIsolationCierraEnAislar(t *testing.T) {
	cscRegistrar(t)
	for _, conSpec := range []bool{true, false} {
		root := repoCiego(t)
		opt := Options{Role: "test-writer", Provider: cscSinAislarNombre, Prompt: "escribi los tests"}
		if conSpec {
			opt.Spec = specAprobado(t, root)
		}
		dump := cscPonerBin(t)
		antes := len(filesIn(t, filepath.Join(root, ".hoom", "verdicts")))
		var out bytes.Buffer
		res, err := Run(root, "main", opt, &out)
		if err != nil {
			t.Fatalf("CA-592: el sobre reporta, no explota (spec=%v): %v\n%s", conSpec, err, out.String())
		}
		cscCorteEnAislar(t, root, res, out.String(), antes, dump)
	}
}

// CA-592 (hostil: antes de cualquier otro chequeo del arbol ciego): sin git,
// o con un run activo en el arbol de trabajo, la negativa es la de
// IsolationFor y no la de esos chequeos.
func TestCA592_IsolationForVaAntesQueLosOtrosChequeosDeAislar(t *testing.T) {
	cscRegistrar(t)

	// sin git (lo que CA-179 corta en aislar)
	sinGit := t.TempDir()
	write(t, sinGit, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n"+
		"  test:\n    required: true\n    cmd: \"true\"\n")
	dump := cscPonerBin(t)
	var out bytes.Buffer
	res, err := Run(sinGit, "main", Options{Role: "test-writer", Provider: cscSinAislarNombre, Prompt: "tests"}, &out)
	if err != nil {
		t.Fatalf("CA-592: sin git el sobre reporta: %v", err)
	}
	cscCorteEnAislar(t, sinGit, res, out.String(), 0, dump)

	// con un run activo en el arbol de trabajo (lo que CA-181 corta en aislar)
	root := repoCiego(t)
	write(t, root, ".hoom/runs/20260905T000000_aaaaaa.meta.json",
		`{"id":"20260905T000000_aaaaaa","provider":"claude","dir":`+
			jsonStr(root)+`,"status":"running","exit_code":-1}`)
	out.Reset()
	res, err = Run(root, "main", Options{Role: "test-writer", Provider: cscSinAislarNombre, Prompt: "tests"}, &out)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "aislar" || res.ExitCode != 1 {
		t.Fatalf("CA-592: corta en aislar: %+v\n%s", res, out.String())
	}
	recs := sobres(t, root)
	if len(recs) != 1 || strings.TrimSpace(recs[0].Note) != cscError(cscSinAislarNombre, "claude, codex") {
		t.Fatalf("CA-592: la nota es la de IsolationFor, no la del run activo: %+v", recs)
	}
	cscNotaComoElRegistro(t, res, recs[0].Note)
	if want := cscError(cscSinAislarNombre, "claude, codex"); res.Note != want {
		t.Fatalf("CA-592: el Result trae la nota de IsolationFor, no la del run activo:\nquiere %q\nfue    %q", want, res.Note)
	}
	if existeEn(t, filepath.Join(root, ".hoom", "isolated")) && len(filesIn(t, filepath.Join(root, ".hoom", "isolated"))) != 0 {
		t.Fatal("CA-592: nada en .hoom/isolated/")
	}
	if existeEn(t, dump) {
		t.Fatal("CA-592: el CLI nunca se lanza")
	}
}

// CA-592 (hostil: la regla es "la misma nota que guarda su registro", no un
// caso aparte de IsolationFor): con claude, que SI se aisla, el sobre del
// test-writer corta en aislar por OTROS chequeos (sin git, sin commits, con un
// run activo), y en cada uno res.Note es la nota del registro del sobre, y el
// JSON del Result la trae en "note". Ninguna es la de IsolationFor.
func TestCA592_OtraNegativaDeAislarTambienViajaEnElResult(t *testing.T) {
	hoomYAML := "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n" +
		"  test:\n    required: true\n    cmd: \"true\"\n"
	casos := map[string]func(t *testing.T) string{
		"sin git": func(t *testing.T) string {
			d := t.TempDir()
			write(t, d, "hoom.yaml", hoomYAML)
			return d
		},
		"sin commits": func(t *testing.T) string {
			d := t.TempDir()
			write(t, d, "hoom.yaml", hoomYAML)
			git(t, d, "init", "-b", "main")
			return d
		},
		"run activo": func(t *testing.T) string {
			d := repoCiego(t)
			write(t, d, ".hoom/runs/20260905T000000_aaaaaa.meta.json",
				`{"id":"20260905T000000_aaaaaa","provider":"claude","dir":`+
					jsonStr(d)+`,"status":"running","exit_code":-1}`)
			return d
		},
	}
	for nombre, armar := range casos {
		t.Run(nombre, func(t *testing.T) {
			dump := filepath.Join(t.TempDir(), "lanzado.txt")
			fakeProvider(t, "claude", "printf '%s\\n' \"$@\" > '"+dump+"'\nexit 0\n")
			root := armar(t)
			var out bytes.Buffer
			res, err := Run(root, "main", Options{Role: "test-writer", Provider: "claude", Prompt: "tests"}, &out)
			if err != nil {
				t.Fatalf("CA-592: el sobre reporta, no explota (%s): %v\n%s", nombre, err, out.String())
			}
			if res.Stage != "aislar" || res.ExitCode != 1 || res.RunID != "" {
				t.Fatalf("CA-592: fixture: %s corta en aislar con exit 1, sin run: %+v\n%s", nombre, res, out.String())
			}
			recs := sobres(t, root)
			if len(recs) != 1 {
				t.Fatalf("CA-592: un sobre, un registro (%s): %+v", nombre, recs)
			}
			if strings.Contains(recs[0].Note, "no puede correr sin tu configuracion personal") {
				t.Fatalf("CA-592: claude se aisla: la negativa (%s) no es la de IsolationFor: %q", nombre, recs[0].Note)
			}
			cscNotaComoElRegistro(t, res, recs[0].Note)
			if existeEn(t, dump) {
				t.Fatalf("CA-592: el CLI nunca se lanza (%s)", nombre)
			}
		})
	}
}

// CA-592 (hostil: el paso aislar empieza despues del spec): con un spec sin
// aprobar, el sobre sigue cortando en el paso 1, como siempre.
func TestCA592_ElSpecSinAprobarCortaAntes(t *testing.T) {
	cscRegistrar(t)
	root := repoCiego(t)
	res, err := Run(root, "main", Options{Role: "test-writer", Provider: cscSinAislarNombre,
		Spec: ".hoom/specs/demo.md", Prompt: "tests"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stage != "spec" || res.ExitCode != 1 {
		t.Fatalf("CA-592: un spec sin aprobar corta en spec, antes de aislar: %+v", res)
	}
}

// CA-591/CA-592 (hostil: la negativa es solo del rol ciego): con el mismo
// provider sin Isolation, el writer pasa de largo el aislamiento y su CLI se
// lanza.
func TestCA592_UnRolNoCiegoCorreConElProviderSinIsolation(t *testing.T) {
	cscRegistrar(t)
	root := repo(t)
	dump := cscPonerBin(t)
	var out bytes.Buffer
	res, err := Run(root, "main", Options{Role: "writer", Provider: cscSinAislarNombre, Prompt: "implementa"}, &out)
	if err != nil {
		t.Fatalf("CA-592: el writer con un provider sin Isolation corre: %v\n%s", err, out.String())
	}
	if res.Stage == "aislar" || strings.Contains(out.String(), "no puede correr sin tu configuracion personal") {
		t.Fatalf("CA-592: el writer no es ciego, no se le pide aislamiento: %+v\n%s", res, out.String())
	}
	if _, err := os.Stat(dump); err != nil {
		t.Fatalf("CA-592: el CLI del writer se lanza: %v\n%s", err, out.String())
	}
}
