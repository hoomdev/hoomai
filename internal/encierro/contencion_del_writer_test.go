// Tests adversariales del spec .hoom/specs/contencion-del-writer.md sobre lo
// puro del paquete encierro, en cualquier sistema: la tabla del nivel
// (CA-677), los textos del contrato (CA-674, CA-676), Alcanza (CA-676),
// el entorno (CA-680, CA-682), la cache (CA-682) y Validar (CA-675).
// Escrito desde el spec y el go doc del paquete, sin mirar la implementacion.
package encierro_test

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"testing/quick"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/manifest"
	"github.com/hoomdev/hoomai/internal/providers"
)

var cdwTodasLasFaltas = []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC}

func cdwMismas(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

// ---------------------------------------------------------------- CA-677

// CA-677: la tabla del contrato, fila por fila, con los motivos que el spec
// fija literales y las constantes de macOS y Windows (aclaracion 6). Con
// ninguno no se fija el mecanismo (aclaracion 7).
func TestCA677_LaTablaDelNivel(t *testing.T) {
	linux := func(s encierro.Sonda) encierro.Caso {
		return encierro.Caso{Sistema: "linux", Sonda: s, Tarea: true, Provider: "claude", DeclaraEstado: true}
	}
	todo := encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 6}
	for _, c := range []struct {
		nombre    string
		caso      encierro.Caso
		nivel     string
		mecanismo string // solo con completo o parcial
		falta     []string
		motivo    string
	}{
		{"linux con todo (ABI 6)", linux(todo), encierro.NivelCompleto, encierro.MecanismoBwrap, nil, ""},
		{"linux con todo (ABI 11)", linux(encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 11}), encierro.NivelCompleto, encierro.MecanismoBwrap, nil, ""},
		{"linux con ABI 5", linux(encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 5}), encierro.NivelParcial, encierro.MecanismoBwrap, []string{encierro.FaltaIPC}, "landlock ABI 5"},
		{"linux sin Landlock", linux(encierro.Sonda{Bwrap: true, Espacio: true}), encierro.NivelParcial, encierro.MecanismoBwrap, []string{encierro.FaltaIPC}, "landlock ABI 0"},
		{"linux sin bwrap", linux(encierro.Sonda{LandlockABI: 11}), encierro.NivelNinguno, "", cdwTodasLasFaltas, "bwrap no esta"},
		{"linux con bwrap sin espacio de usuario", linux(encierro.Sonda{Bwrap: true, Salida: "bwrap: setting up uid map: Permission denied", LandlockABI: 11}),
			encierro.NivelNinguno, "", cdwTodasLasFaltas, "bwrap: setting up uid map: Permission denied"},
		{"windows con el job", encierro.Caso{Sistema: "windows", Tarea: true, Provider: "codex", DeclaraEstado: true, Job: true, Sonda: todo},
			encierro.NivelParcial, encierro.MecanismoJob, []string{encierro.FaltaEscrituras, encierro.FaltaIPC}, encierro.MotivoSinEscrituras},
		{"windows sin el job", encierro.Caso{Sistema: "windows", Tarea: true, Provider: "codex", DeclaraEstado: true, Sonda: todo},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, encierro.MotivoSinJob},
		{"macOS", encierro.Caso{Sistema: "darwin", Tarea: true, Provider: "claude", DeclaraEstado: true, Job: true, Sonda: todo},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, encierro.MotivoSinContencion},
		{"otro sistema", encierro.Caso{Sistema: "freebsd", Tarea: true, Sonda: todo, Job: true},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, encierro.MotivoSinContencion},
		{"sin tarea en linux con todo", encierro.Caso{Sistema: "linux", Sonda: todo, Provider: "claude", DeclaraEstado: true},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, "sin tarea"},
		{"sin tarea en windows con el job", encierro.Caso{Sistema: "windows", Job: true},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, "sin tarea"},
		{"un provider sin estado en linux con todo", encierro.Caso{Sistema: "linux", Sonda: todo, Tarea: true, Provider: "gemini"},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, "gemini no declara su estado"},
		{"sin provider (un gate) en linux con todo", encierro.Caso{Sistema: "linux", Sonda: todo, Tarea: true},
			encierro.NivelCompleto, encierro.MecanismoBwrap, nil, ""},
		{"sin provider, DeclaraEstado se ignora", encierro.Caso{Sistema: "linux", Sonda: todo, Tarea: true, DeclaraEstado: false, Provider: ""},
			encierro.NivelCompleto, encierro.MecanismoBwrap, nil, ""},
		// prioridad de motivos (aclaracion 9): sin tarea > provider sin estado > sistema
		{"sin tarea y provider sin estado", encierro.Caso{Sistema: "linux", Sonda: todo, Provider: "opencode"},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, "sin tarea"},
		{"provider sin estado en linux sin bwrap", encierro.Caso{Sistema: "linux", Tarea: true, Provider: "opencode"},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, "opencode no declara su estado"},
		{"provider sin estado en macOS", encierro.Caso{Sistema: "darwin", Tarea: true, Provider: "opencode"},
			encierro.NivelNinguno, "", cdwTodasLasFaltas, "opencode no declara su estado"},
	} {
		got := encierro.NivelDe(c.caso)
		if got.Nivel != c.nivel || !cdwMismas(got.Falta, c.falta) || got.Motivo != c.motivo {
			t.Errorf("CA-677: %s: quiero nivel %q, falta %q, motivo %q; fue %+v", c.nombre, c.nivel, c.falta, c.motivo, got)
		}
		if c.nivel != encierro.NivelNinguno && got.Mecanismo != c.mecanismo {
			t.Errorf("CA-677: %s: mecanismo %q; fue %q", c.nombre, c.mecanismo, got.Mecanismo)
		}
	}
}

// CA-677: los motivos con parametro, literales.
func TestCA677_MotivosConParametro(t *testing.T) {
	for _, abi := range []int{0, 1, 5} {
		if got, want := encierro.MotivoLandlock(abi), "landlock ABI "+strconv.Itoa(abi); got != want {
			t.Errorf("CA-677: MotivoLandlock(%d) = %q; quiero %q", abi, got, want)
		}
	}
	for _, p := range []string{"gemini", "opencode", "falso con espacios"} {
		if got, want := encierro.MotivoSinEstado(p), p+" no declara su estado"; got != want {
			t.Errorf("CA-677: MotivoSinEstado(%q) = %q; quiero %q", p, got, want)
		}
	}
	if encierro.MotivoSinBwrap != "bwrap no esta" || encierro.MotivoSinTarea != "sin tarea" {
		t.Errorf("CA-677: los motivos que fija el contrato: %q %q", encierro.MotivoSinBwrap, encierro.MotivoSinTarea)
	}
	if encierro.NivelCompleto != "completo" || encierro.NivelParcial != "parcial" || encierro.NivelNinguno != "ninguno" {
		t.Errorf("CA-677: los niveles del contrato")
	}
	if encierro.MecanismoBwrap != "bwrap" || encierro.MecanismoJob != "job" {
		t.Errorf("CA-677: los mecanismos del contrato")
	}
	if !reflect.DeepEqual(cdwTodasLasFaltas, []string{"procesos", "escrituras", "ipc"}) {
		t.Errorf("CA-677: lo que puede faltar, en su orden: %q", cdwTodasLasFaltas)
	}
}

// cdwCaso genera un Caso cualquiera, con sistemas raros y sondas absurdas
// (Espacio sin Bwrap, ABI negativo, salida con saltos de linea).
type cdwCaso struct{ c encierro.Caso }

func (cdwCaso) Generate(r *rand.Rand, _ int) reflect.Value {
	sistemas := []string{"linux", "windows", "darwin", "freebsd", "", "plan9", "LINUX"}
	provs := []string{"", "claude", "codex", "gemini", "x y"}
	salidas := []string{"", "denegado", "bwrap: No permitido\n\tlinea 2"}
	return reflect.ValueOf(cdwCaso{encierro.Caso{
		Sistema: sistemas[r.Intn(len(sistemas))],
		Sonda: encierro.Sonda{Bwrap: r.Intn(2) == 0, Espacio: r.Intn(2) == 0,
			Salida: salidas[r.Intn(len(salidas))], LandlockABI: r.Intn(14) - 2},
		Tarea:         r.Intn(4) != 0,
		Provider:      provs[r.Intn(len(provs))],
		DeclaraEstado: r.Intn(2) == 0,
		Job:           r.Intn(2) == 0,
	}})
}

func cdwSubsecuencia(f []string) bool {
	i := 0
	for _, x := range f {
		for i < len(cdwTodasLasFaltas) && cdwTodasLasFaltas[i] != x {
			i++
		}
		if i == len(cdwTodasLasFaltas) {
			return false
		}
		i++
	}
	return true
}

// CA-677 (propiedad): para todo Caso, el nivel es uno de los tres; completo
// es exactamente falta vacia y sin motivo; ninguno es exactamente las tres
// faltas; parcial falta algo pero no todo y lleva motivo; lo que falta va
// siempre en el orden del contrato; sin tarea siempre es ninguno con su
// motivo; y NivelDe es puro (dos llamadas, mismo resultado).
func TestCA677_PropiedadNivelDe(t *testing.T) {
	f := func(g cdwCaso) bool {
		i := encierro.NivelDe(g.c)
		otra := encierro.NivelDe(g.c)
		if !reflect.DeepEqual(i, otra) {
			t.Logf("no es puro: %+v vs %+v", i, otra)
			return false
		}
		switch i.Nivel {
		case encierro.NivelCompleto, encierro.NivelParcial, encierro.NivelNinguno:
		default:
			t.Logf("nivel fuera de los tres: %+v para %+v", i, g.c)
			return false
		}
		completo := len(i.Falta) == 0 && i.Motivo == ""
		if (i.Nivel == encierro.NivelCompleto) != completo {
			t.Logf("completo <=> falta vacia y sin motivo: %+v para %+v", i, g.c)
			return false
		}
		ninguno := reflect.DeepEqual(i.Falta, cdwTodasLasFaltas)
		if (i.Nivel == encierro.NivelNinguno) != ninguno {
			t.Logf("ninguno <=> las tres faltas: %+v para %+v", i, g.c)
			return false
		}
		if i.Nivel == encierro.NivelParcial && (len(i.Falta) == 0 || i.Motivo == "" || i.Mecanismo == "") {
			t.Logf("parcial falta algo, tiene motivo y mecanismo: %+v", i)
			return false
		}
		if i.Nivel == encierro.NivelCompleto && i.Mecanismo != encierro.MecanismoBwrap {
			t.Logf("completo solo con bwrap: %+v", i)
			return false
		}
		if i.Nivel == encierro.NivelNinguno && i.Motivo == "" {
			t.Logf("ninguno siempre dice su motivo: %+v", i)
			return false
		}
		if !cdwSubsecuencia(i.Falta) {
			t.Logf("falta fuera del orden del contrato: %q", i.Falta)
			return false
		}
		if !g.c.Tarea && (i.Nivel != encierro.NivelNinguno || i.Motivo != encierro.MotivoSinTarea) {
			t.Logf("sin tarea: ninguno, sin tarea: %+v", i)
			return false
		}
		if g.c.Tarea && g.c.Provider != "" && !g.c.DeclaraEstado &&
			(i.Nivel != encierro.NivelNinguno || i.Motivo != encierro.MotivoSinEstado(g.c.Provider)) {
			t.Logf("provider sin estado: ninguno con su motivo: %+v para %+v", i, g.c)
			return false
		}
		if i.Nivel != encierro.NivelNinguno && g.c.Sistema != "linux" && g.c.Sistema != "windows" {
			t.Logf("solo linux y windows contienen: %+v para %+v", i, g.c)
			return false
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 3000}); err != nil {
		t.Fatalf("CA-677: %v", err)
	}
}

// CA-677: Nivel es NivelDe para este sistema, y ReemplazarSonda la cambia y
// la devuelve (parcial y ninguno en una maquina con completo).
func TestCA677_NivelEsNivelDeConLaSondaReemplazable(t *testing.T) {
	claude, err := providers.Lookup("claude")
	if err != nil {
		t.Fatal(err)
	}
	original := encierro.Sondear()
	for _, s := range []encierro.Sonda{
		{},
		{Bwrap: true, Salida: "AppArmor dijo que no"},
		{Bwrap: true, Espacio: true, LandlockABI: 5},
		{Bwrap: true, Espacio: true, LandlockABI: 6},
	} {
		restaurar := encierro.ReemplazarSonda(s)
		if got := encierro.Sondear(); !reflect.DeepEqual(got, s) {
			restaurar()
			t.Fatalf("CA-677: con ReemplazarSonda, Sondear responde la sonda puesta: %+v vs %+v", got, s)
		}
		for _, c := range []struct {
			tarea bool
			prov  providers.Provider
			caso  encierro.Caso
		}{
			{true, claude, encierro.Caso{Sistema: runtime.GOOS, Sonda: s, Tarea: true, Provider: "claude", DeclaraEstado: true, Job: true}},
			{false, claude, encierro.Caso{Sistema: runtime.GOOS, Sonda: s, Tarea: false, Provider: "claude", DeclaraEstado: true, Job: true}},
			{true, nil, encierro.Caso{Sistema: runtime.GOOS, Sonda: s, Tarea: true, Job: true}},
		} {
			if got, want := encierro.Nivel(c.tarea, c.prov), encierro.NivelDe(c.caso); !reflect.DeepEqual(got, want) {
				restaurar()
				t.Fatalf("CA-677: Nivel(%v, %v) es NivelDe de este sistema con la sonda %+v: %+v vs %+v", c.tarea, c.prov, s, got, want)
			}
		}
		restaurar()
	}
	if got := encierro.Sondear(); !reflect.DeepEqual(got, original) {
		t.Fatalf("CA-677: restaurar devuelve la sonda de antes: %+v vs %+v", got, original)
	}
	// anidados: restaurar en orden inverso deja cada una
	r1 := encierro.ReemplazarSonda(encierro.Sonda{Bwrap: true})
	r2 := encierro.ReemplazarSonda(encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 3})
	r2()
	if got := encierro.Sondear(); !reflect.DeepEqual(got, encierro.Sonda{Bwrap: true}) {
		r1()
		t.Fatalf("CA-677: restaurar la segunda devuelve la primera: %+v", got)
	}
	r1()
	if got := encierro.Sondear(); !reflect.DeepEqual(got, original) {
		t.Fatalf("CA-677: restaurar la primera devuelve la original: %+v", got)
	}
}

// CA-690 y CA-677: un provider que no implementa ConEstado da ninguno con su
// motivo, aunque el sistema pueda encerrar.
func TestCA690_UnProviderSinEstadoEsNinguno(t *testing.T) {
	restaurar := encierro.ReemplazarSonda(encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 11})
	defer restaurar()
	i := encierro.Nivel(true, cdwSinEstado{})
	if i.Nivel != encierro.NivelNinguno || i.Motivo != "sin-estado-cdw no declara su estado" || !reflect.DeepEqual(i.Falta, cdwTodasLasFaltas) {
		t.Fatalf("CA-690: un provider falso sin ConEstado da ninguno con `sin-estado-cdw no declara su estado`: %+v", i)
	}
}

type cdwSinEstado struct{}

func (cdwSinEstado) Name() string { return "sin-estado-cdw" }
func (cdwSinEstado) Bin() string  { return "hoom-sin-estado-cdw" }
func (cdwSinEstado) Capabilities() providers.Capabilities {
	return providers.Capabilities{SystemPrompt: true}
}
func (p cdwSinEstado) Command(req providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: p.Bin(), Args: []string{req.Prompt}}, nil
}
func (cdwSinEstado) Normalize(string) []providers.Event { return nil }

// ---------------------------------------------------------------- CA-674

// CA-674: el JSON de Info: con completo, "falta" es la lista vacia (nunca
// null) y "motivo" no va; con parcial y ninguno, motivo va.
func TestCA674_ElJSONDeInfo(t *testing.T) {
	completo := encierro.NivelDe(encierro.Caso{Sistema: "linux", Tarea: true, Sonda: encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 6}})
	raw, err := json.Marshal(completo)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	falta, ok := m["falta"].([]any)
	if m["nivel"] != "completo" || m["mecanismo"] != "bwrap" || !ok || len(falta) != 0 {
		t.Fatalf("CA-674: con completo, {\"nivel\":\"completo\",\"mecanismo\":\"bwrap\",\"falta\":[]}: %s", raw)
	}
	if _, hay := m["motivo"]; hay || !strings.Contains(string(raw), `"falta":[]`) {
		t.Fatalf("CA-674: con completo, falta es [] (no null) y motivo no va: %s", raw)
	}
	parcial := encierro.NivelDe(encierro.Caso{Sistema: "linux", Tarea: true, Sonda: encierro.Sonda{Bwrap: true, Espacio: true, LandlockABI: 4}})
	raw, _ = json.Marshal(parcial)
	m = nil
	_ = json.Unmarshal(raw, &m)
	if m["nivel"] != "parcial" || m["motivo"] != "landlock ABI 4" || !reflect.DeepEqual(m["falta"], []any{"ipc"}) {
		t.Fatalf("CA-674: con parcial, falta [ipc] y motivo: %s", raw)
	}
	ninguno := encierro.NivelDe(encierro.Caso{Sistema: "darwin", Tarea: true})
	raw, _ = json.Marshal(ninguno)
	m = nil
	_ = json.Unmarshal(raw, &m)
	if m["nivel"] != "ninguno" || m["motivo"] != encierro.MotivoSinContencion ||
		!reflect.DeepEqual(m["falta"], []any{"procesos", "escrituras", "ipc"}) {
		t.Fatalf("CA-674: con ninguno, las tres faltas y el motivo: %s", raw)
	}
	if _, hay := m["nivel"]; !hay {
		t.Fatalf("CA-674: la clave nivel va siempre: %s", raw)
	}
}

// CA-674: el aviso de un nivel menor que completo y la linea del log, con
// el rol y la lista unida con ", ".
func TestCA674_AvisoYLinea(t *testing.T) {
	parcial := encierro.Info{Nivel: encierro.NivelParcial, Mecanismo: encierro.MecanismoBwrap, Falta: []string{encierro.FaltaIPC}, Motivo: "landlock ABI 5"}
	job := encierro.Info{Nivel: encierro.NivelParcial, Mecanismo: encierro.MecanismoJob, Falta: []string{encierro.FaltaEscrituras, encierro.FaltaIPC}, Motivo: encierro.MotivoSinEscrituras}
	ninguno := encierro.Info{Nivel: encierro.NivelNinguno, Falta: cdwTodasLasFaltas, Motivo: "bwrap no esta"}
	completo := encierro.Info{Nivel: encierro.NivelCompleto, Mecanismo: encierro.MecanismoBwrap, Falta: []string{}}
	for _, c := range []struct {
		i         encierro.Info
		rol, want string
	}{
		{parcial, "writer", "aviso: encierro parcial (falta: ipc): lo que el writer deje fuera de su arbol no se ve"},
		{job, "reviewer", "aviso: encierro parcial (falta: escrituras, ipc): lo que el reviewer deje fuera de su arbol no se ve"},
		{ninguno, "test-writer", "aviso: encierro ninguno (falta: procesos, escrituras, ipc): lo que el test-writer deje fuera de su arbol no se ve"},
		{completo, "writer", ""},
	} {
		if got := encierro.Aviso(c.i, c.rol); got != c.want {
			t.Errorf("CA-674: Aviso(%+v, %q) = %q; quiero %q", c.i, c.rol, got, c.want)
		}
	}
	for _, c := range []struct {
		i    encierro.Info
		want string
	}{
		{completo, "encierro completo con bwrap"},
		{parcial, "encierro parcial con bwrap"},
		{job, "encierro parcial con job"},
		{ninguno, "sin encierro: bwrap no esta"},
		{encierro.Info{Nivel: encierro.NivelNinguno, Falta: cdwTodasLasFaltas, Motivo: "sin tarea"}, "sin encierro: sin tarea"},
	} {
		if got := encierro.Linea(c.i); got != c.want {
			t.Errorf("CA-674: Linea(%+v) = %q; quiero %q", c.i, got, c.want)
		}
	}
}

// ---------------------------------------------------------------- CA-676

// CA-676: la nota del sobre que se niega por exigir, literal.
func TestCA676_NotaExigir(t *testing.T) {
	parcial := encierro.Info{Nivel: encierro.NivelParcial, Mecanismo: encierro.MecanismoBwrap, Falta: []string{encierro.FaltaIPC}, Motivo: "landlock ABI 5"}
	ninguno := encierro.Info{Nivel: encierro.NivelNinguno, Falta: cdwTodasLasFaltas, Motivo: "sin tarea"}
	for _, c := range []struct {
		i              encierro.Info
		exigir, rol, w string
	}{
		{parcial, manifest.ExigirCompleto, "writer", "el encierro de este sistema es parcial (falta: ipc) y hoom.yaml exige completo: el writer no corre"},
		{ninguno, manifest.ExigirParcial, "reviewer", "el encierro de este sistema es ninguno (falta: procesos, escrituras, ipc) y hoom.yaml exige parcial: el reviewer no corre"},
		{ninguno, manifest.ExigirCompleto, "refutador", "el encierro de este sistema es ninguno (falta: procesos, escrituras, ipc) y hoom.yaml exige completo: el refutador no corre"},
	} {
		if got := encierro.NotaExigir(c.i, c.exigir, c.rol); got != c.w {
			t.Errorf("CA-676: NotaExigir = %q; quiero %q", got, c.w)
		}
	}
	if manifest.ExigirCompleto != "completo" || manifest.ExigirParcial != "parcial" || manifest.ExigirNo != "no" {
		t.Errorf("CA-675: los valores de exigir: %q %q %q", manifest.ExigirCompleto, manifest.ExigirParcial, manifest.ExigirNo)
	}
}

// CA-676: Alcanza, la tabla entera.
func TestCA676_AlcanzaTabla(t *testing.T) {
	niveles := []string{encierro.NivelNinguno, encierro.NivelParcial, encierro.NivelCompleto}
	exigencias := []string{manifest.ExigirNo, "", manifest.ExigirParcial, manifest.ExigirCompleto}
	want := map[[2]string]bool{
		{"ninguno", "no"}: true, {"parcial", "no"}: true, {"completo", "no"}: true,
		{"ninguno", ""}: true, {"parcial", ""}: true, {"completo", ""}: true,
		{"ninguno", "parcial"}: false, {"parcial", "parcial"}: true, {"completo", "parcial"}: true,
		{"ninguno", "completo"}: false, {"parcial", "completo"}: false, {"completo", "completo"}: true,
	}
	for _, n := range niveles {
		for _, e := range exigencias {
			if got := encierro.Alcanza(n, e); got != want[[2]string{n, e}] {
				t.Errorf("CA-676: Alcanza(%q, %q) = %v; quiero %v", n, e, got, want[[2]string{n, e}])
			}
		}
	}
}

// CA-676 (propiedad): Alcanza es monotona en el nivel (mas nivel nunca
// alcanza menos) y antitona en exigir (exigir mas nunca alcanza mas).
func TestCA676_PropiedadAlcanzaMonotona(t *testing.T) {
	niveles := []string{encierro.NivelNinguno, encierro.NivelParcial, encierro.NivelCompleto}
	exigencias := []string{manifest.ExigirNo, manifest.ExigirParcial, manifest.ExigirCompleto}
	f := func(a, b, e, e2 uint8) bool {
		i, j := int(a)%3, int(b)%3
		x, y := int(e)%3, int(e2)%3
		if i > j {
			i, j = j, i
		}
		if x > y {
			x, y = y, x
		}
		if encierro.Alcanza(niveles[i], exigencias[x]) && !encierro.Alcanza(niveles[j], exigencias[x]) {
			t.Logf("no es monotona: %s alcanza %s y %s no", niveles[i], exigencias[x], niveles[j])
			return false
		}
		if encierro.Alcanza(niveles[i], exigencias[y]) && !encierro.Alcanza(niveles[i], exigencias[x]) {
			t.Logf("exigir menos alcanza menos: %s con %s y %s", niveles[i], exigencias[y], exigencias[x])
			return false
		}
		return encierro.Alcanza(niveles[i], manifest.ExigirNo)
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 500}); err != nil {
		t.Fatalf("CA-676: %v", err)
	}
}

// ---------------------------------------------------------------- CA-682

// CA-682: la cache de cada quien, en el checkout principal, y nadie
// comparte la de otro.
func TestCA682_LaCacheDeCadaQuien(t *testing.T) {
	if encierro.DirCache != ".hoom/cache/encierro" {
		t.Fatalf("CA-682: DirCache: %q", encierro.DirCache)
	}
	if encierro.QuienVerify != "verify" || encierro.QuienGuardado != "guardado" {
		t.Fatalf("CA-682: los quien que no son un rol: %q %q", encierro.QuienVerify, encierro.QuienGuardado)
	}
	principal := filepath.Join(string(filepath.Separator), "proyectos", "demo con espacios")
	if got, want := encierro.Cache(principal, "mi-tarea", "writer"), filepath.Join(principal, ".hoom", "cache", "encierro", "mi-tarea", "writer"); got != want {
		t.Fatalf("CA-682: Cache = %q; quiero %q", got, want)
	}
	quienes := []string{"writer", "test-writer", "reviewer", "characterizer", "refutador", "orquestador", encierro.QuienVerify, encierro.QuienGuardado}
	vistos := map[string]string{}
	for _, q := range quienes {
		c := encierro.Cache(principal, "mi-tarea", q)
		for otro, oc := range vistos {
			if c == oc || strings.HasPrefix(c, oc+string(filepath.Separator)) || strings.HasPrefix(oc, c+string(filepath.Separator)) {
				t.Fatalf("CA-682: la cache de %s (%s) y la de %s (%s) se pisan", q, c, otro, oc)
			}
		}
		vistos[q] = c
		if !strings.HasPrefix(c, filepath.Join(principal, encierro.DirCache, "mi-tarea")+string(filepath.Separator)) {
			t.Fatalf("CA-682: la cache de %s vive en .hoom/cache/encierro/<slug>/: %s", q, c)
		}
	}
	if encierro.Cache(principal, "otra", "writer") == encierro.Cache(principal, "mi-tarea", "writer") {
		t.Fatal("CA-682: dos tareas no comparten la cache del mismo rol")
	}
}

func cdwVar(env []string, k string) (string, int) {
	v, n := "", 0
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i >= 0 && kv[:i] == k {
			v = kv[i+1:]
			n++
		}
	}
	return v, n
}

// CA-680 y CA-682: Entorno, con un caso fijo.
func TestCA680_CA682_ElEntorno(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "no-existe-todavia", "writer")
	v := encierro.Vista{Dir: "/arbol", Cache: cache, VarsCache: []string{"GOCACHE", "GOMODCACHE", "GOTMPDIR"}}
	base := []string{
		"PATH=/usr/bin:/bin", "HOME=/home/x", "SSH_AUTH_SOCK=/run/user/1000/ssh", "GPG_AGENT_INFO=x",
		"DBUS_SESSION_BUS_ADDRESS=unix:path=/run/user/1000/bus", "DISPLAY=:0", "WAYLAND_DISPLAY=wayland-0",
		"XDG_RUNTIME_DIR=/run/user/1000", "TMPDIR=/tmp/de-la-persona", "GOCACHE=/home/x/.cache/go-build",
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=user.name", "GIT_CONFIG_VALUE_0=hoom", "HOOM_ROLE=writer",
	}
	antes := append([]string(nil), base...)
	env := encierro.Entorno(base, v)
	if !reflect.DeepEqual(base, antes) {
		t.Fatalf("CA-680: Entorno no toca su base: %q", base)
	}
	for _, k := range encierro.VariablesQuitadas {
		if _, n := cdwVar(env, k); n != 0 {
			t.Errorf("CA-680: %s no va en el entorno de lo encerrado: %q", k, env)
		}
	}
	want := []string{"SSH_AUTH_SOCK", "GPG_AGENT_INFO", "DBUS_SESSION_BUS_ADDRESS", "DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR"}
	if !reflect.DeepEqual(encierro.VariablesQuitadas, want) {
		t.Errorf("CA-680: VariablesQuitadas es la lista del contrato: %q", encierro.VariablesQuitadas)
	}
	for _, k := range []string{"TMPDIR", "XDG_CACHE_HOME", "GOCACHE", "GOMODCACHE", "GOTMPDIR"} {
		val, n := cdwVar(env, k)
		if n != 1 || val != filepath.Join(cache, k) {
			t.Errorf("CA-682: %s es filepath.Join(cache, %s) una sola vez: %q (%d)", k, k, val, n)
		}
	}
	for _, k := range []string{"PATH", "HOME", "HOOM_ROLE"} {
		if got, _ := cdwVar(env, k); got != func() string { g, _ := cdwVar(base, k); return g }() {
			t.Errorf("CA-680: el resto del entorno sigue como hoy: %s=%q", k, got)
		}
	}
	if n, _ := cdwVar(env, "GIT_CONFIG_COUNT"); n != "4" {
		t.Fatalf("CA-680: GIT_CONFIG_COUNT suma las tres de ConfigGit a la que ya habia: %q", n)
	}
	if k, _ := cdwVar(env, "GIT_CONFIG_KEY_0"); k != "user.name" {
		t.Fatalf("CA-680: la entrada que ya traia la base queda: %q", k)
	}
	for i, kv := range encierro.ConfigGit {
		k, _ := cdwVar(env, fmt.Sprintf("GIT_CONFIG_KEY_%d", i+1))
		val, _ := cdwVar(env, fmt.Sprintf("GIT_CONFIG_VALUE_%d", i+1))
		if k != kv[0] || val != kv[1] {
			t.Errorf("CA-680: GIT_CONFIG_*_%d es %s=%s: %s=%s", i+1, kv[0], kv[1], k, val)
		}
	}
	wantGit := [][2]string{{"gc.auto", "0"}, {"maintenance.auto", "false"}, {"core.fsmonitor", "false"}}
	if !reflect.DeepEqual(encierro.ConfigGit, wantGit) {
		t.Errorf("CA-680: ConfigGit es la del contrato: %q", encierro.ConfigGit)
	}
	if _, err := os.Stat(filepath.Dir(cache)); !os.IsNotExist(err) {
		t.Fatalf("CA-682: Entorno es puro: no crea la cache (%v)", err)
	}
}

// cdwBase es un entorno de partida cualquiera para la propiedad de Entorno.
type cdwBase struct {
	env []string
	k   int // las entradas GIT_CONFIG que trae
}

func (cdwBase) Generate(r *rand.Rand, _ int) reflect.Value {
	var env []string
	nombres := []string{"PATH", "HOME", "LANG", "HOOM_TASK", "GOFLAGS", "TERM", "X_Y", "GIT_DIR"}
	for _, n := range nombres {
		if r.Intn(2) == 0 {
			env = append(env, n+"=v"+strconv.Itoa(r.Intn(100)))
		}
	}
	for _, q := range encierro.VariablesQuitadas {
		for j := r.Intn(3); j > 0; j-- { // alguna, repetida
			env = append(env, q+"=algo"+strconv.Itoa(j))
		}
	}
	for _, s := range []string{"TMPDIR", "XDG_CACHE_HOME", "GOCACHE", "GOTMPDIR"} {
		for j := r.Intn(3); j > 0; j-- {
			env = append(env, s+"=/de/la/persona/"+strconv.Itoa(j))
		}
	}
	k := r.Intn(4)
	if k > 0 || r.Intn(2) == 0 {
		env = append(env, "GIT_CONFIG_COUNT="+strconv.Itoa(k))
		for i := 0; i < k; i++ {
			env = append(env, fmt.Sprintf("GIT_CONFIG_KEY_%d=clave.n%d", i, i), fmt.Sprintf("GIT_CONFIG_VALUE_%d=valor %d", i, i))
		}
	}
	r.Shuffle(len(env), func(i, j int) { env[i], env[j] = env[j], env[i] })
	return reflect.ValueOf(cdwBase{env: env, k: k})
}

// CA-680 y CA-682 (propiedad): para toda base, Entorno nunca deja una de
// VariablesQuitadas; toda variable de cache (TMPDIR, XDG_CACHE_HOME y las
// del perfil) va una sola vez y estrictamente dentro de la cache; las
// entradas GIT_CONFIG de la base quedan y ConfigGit va despues; el resto de
// las variables de la base queda con su valor.
func TestCA680_CA682_PropiedadEntorno(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "c", "writer")
	f := func(b cdwBase, conPerfil bool) bool {
		v := encierro.Vista{Dir: "/a", Cache: cache}
		if conPerfil {
			v.VarsCache = []string{"GOCACHE", "GOMODCACHE", "GOTMPDIR"}
		}
		env := encierro.Entorno(b.env, v)
		for _, q := range encierro.VariablesQuitadas {
			if _, n := cdwVar(env, q); n != 0 {
				t.Logf("quedo %s: %q", q, env)
				return false
			}
		}
		vars := append([]string{"TMPDIR", "XDG_CACHE_HOME"}, v.VarsCache...)
		for _, k := range vars {
			val, n := cdwVar(env, k)
			if n != 1 || val != filepath.Join(cache, k) || !strings.HasPrefix(val, cache+string(filepath.Separator)) {
				t.Logf("%s=%q (%d veces) fuera de la cache %s", k, val, n, cache)
				return false
			}
		}
		cuenta, _ := cdwVar(env, "GIT_CONFIG_COUNT")
		if cuenta != strconv.Itoa(b.k+len(encierro.ConfigGit)) {
			t.Logf("GIT_CONFIG_COUNT=%q con %d de la base", cuenta, b.k)
			return false
		}
		for i := 0; i < b.k; i++ {
			k, _ := cdwVar(env, fmt.Sprintf("GIT_CONFIG_KEY_%d", i))
			val, _ := cdwVar(env, fmt.Sprintf("GIT_CONFIG_VALUE_%d", i))
			if k != fmt.Sprintf("clave.n%d", i) || val != fmt.Sprintf("valor %d", i) {
				t.Logf("la entrada %d de la base no quedo: %s=%s", i, k, val)
				return false
			}
		}
		for j, kv := range encierro.ConfigGit {
			k, _ := cdwVar(env, fmt.Sprintf("GIT_CONFIG_KEY_%d", b.k+j))
			val, _ := cdwVar(env, fmt.Sprintf("GIT_CONFIG_VALUE_%d", b.k+j))
			if k != kv[0] || val != kv[1] {
				t.Logf("ConfigGit %d no va despues de la base: %s=%s", j, k, val)
				return false
			}
		}
		puestas := map[string]bool{"GIT_CONFIG_COUNT": true}
		for _, k := range vars {
			puestas[k] = true
		}
		for _, q := range encierro.VariablesQuitadas {
			puestas[q] = true
		}
		for _, kv := range b.env {
			k := kv[:strings.IndexByte(kv, '=')]
			if puestas[k] || strings.HasPrefix(k, "GIT_CONFIG_") {
				continue
			}
			want, _ := cdwVar(b.env, k)
			if got, _ := cdwVar(env, k); got != want {
				t.Logf("%s cambio: %q -> %q", k, want, got)
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, &quick.Config{MaxCount: 800}); err != nil {
		t.Fatalf("CA-680/CA-682: %v", err)
	}
	if _, err := os.Stat(cache); !os.IsNotExist(err) {
		t.Fatalf("CA-682: Entorno es puro: no crea nada (%v)", err)
	}
}

// ---------------------------------------------------------------- CA-675

// CA-675: Validar, cada regla con su texto, la ruta como la escribe
// hoom.yaml, en orden (las escribibles primero, en su orden, despues los
// gates), y sin correr git (PATH vacio).
func TestCA675_Validar(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", "")
	principal := filepath.Join(home, "proyecto")
	comunAparte := filepath.Join(t.TempDir(), "repos", "demo.git")
	gates := []string{"test", "lint", "e2e"}
	errEscribible := func(r string) string {
		return "encierro: escribibles: " + r + " contiene o esta dentro del checkout principal o del directorio comun de git: no puede ser escribible"
	}
	errGate := func(g string) string {
		return "encierro: gates_sin_encierro: el gate " + g + " no esta en los gates de este arbol"
	}
	for _, c := range []struct {
		nombre string
		pol    manifest.EncierroPolicy
		comun  string
		want   string
	}{
		{"sin nada", manifest.EncierroPolicy{Exigir: manifest.ExigirNo}, filepath.Join(principal, ".git"), ""},
		{"una escribible ajena y un gate del arbol", manifest.EncierroPolicy{Escribibles: []string{"~/.semgrep", "/opt/cache"}, GatesSinEncierro: []string{"e2e"}}, filepath.Join(principal, ".git"), ""},
		{"el checkout principal", manifest.EncierroPolicy{Escribibles: []string{principal}}, filepath.Join(principal, ".git"), errEscribible(principal)},
		{"dentro del checkout principal", manifest.EncierroPolicy{Escribibles: []string{filepath.Join(principal, ".hoom", "items")}}, filepath.Join(principal, ".git"), errEscribible(filepath.Join(principal, ".hoom", "items"))},
		{"lo contiene", manifest.EncierroPolicy{Escribibles: []string{home}}, filepath.Join(principal, ".git"), errEscribible(home)},
		{"con ~/ como lo escribe hoom.yaml", manifest.EncierroPolicy{Escribibles: []string{"~/proyecto"}}, filepath.Join(principal, ".git"), errEscribible("~/proyecto")},
		{"con ~/ dentro", manifest.EncierroPolicy{Escribibles: []string{"~/proyecto/.hoom/worktrees"}}, filepath.Join(principal, ".git"), errEscribible("~/proyecto/.hoom/worktrees")},
		{"vuelta por .. hasta adentro", manifest.EncierroPolicy{Escribibles: []string{filepath.Join(home, "otro") + "/../proyecto/sub"}}, filepath.Join(principal, ".git"), errEscribible(filepath.Join(home, "otro") + "/../proyecto/sub")},
		{"un vecino con el mismo prefijo no es adentro", manifest.EncierroPolicy{Escribibles: []string{principal + "-otro", principal + "x/y"}}, filepath.Join(principal, ".git"), ""},
		{"el directorio comun aparte", manifest.EncierroPolicy{Escribibles: []string{filepath.Dir(comunAparte)}}, comunAparte, errEscribible(filepath.Dir(comunAparte))},
		{"dentro del directorio comun aparte", manifest.EncierroPolicy{Escribibles: []string{filepath.Join(comunAparte, "objects")}}, comunAparte, errEscribible(filepath.Join(comunAparte, "objects"))},
		{"la primera que rompe, en su orden", manifest.EncierroPolicy{Escribibles: []string{"/opt/ok", filepath.Join(principal, "b"), principal}}, filepath.Join(principal, ".git"), errEscribible(filepath.Join(principal, "b"))},
		{"un gate que no esta", manifest.EncierroPolicy{GatesSinEncierro: []string{"e2e", "docker"}}, filepath.Join(principal, ".git"), errGate("docker")},
		{"la escribible antes que el gate", manifest.EncierroPolicy{Escribibles: []string{principal}, GatesSinEncierro: []string{"docker"}}, filepath.Join(principal, ".git"), errEscribible(principal)},
		{"el gate por su nombre exacto", manifest.EncierroPolicy{GatesSinEncierro: []string{"Test"}}, filepath.Join(principal, ".git"), errGate("Test")},
	} {
		err := encierro.Validar(c.pol, principal, c.comun, gates)
		switch {
		case c.want == "" && err != nil:
			t.Errorf("CA-675: %s: nil; fue %v", c.nombre, err)
		case c.want != "" && (err == nil || err.Error() != c.want):
			t.Errorf("CA-675: %s:\n quiero %q\n fue    %v", c.nombre, c.want, err)
		}
	}
}
