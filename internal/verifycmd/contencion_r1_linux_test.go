//go:build linux

// Ronda 1 del test-writer de contencion-del-writer: regresiones de la review
// de codex sobre verifycmd en Linux con encierro: los gates del sobre del
// test-writer corren con su vista (a8ef36; aclaracion 23, que corrige la
// 10), el gate de Windows sin job queda en error con el texto de SinJob
// (c5790e; aclaracion 22, simulado con ReemplazarAdopcion) y git-ajeno con
// el gate encerrado (318ca0). Sin bwrap fallan (aclaracion 1).
package verifycmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/encierro"
	"github.com/hoomdev/hoomai/internal/verdict"
)

func r1Leer(p string) string {
	raw, _ := os.ReadFile(p)
	return string(raw)
}

func r1Existe(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// r1HayAdentro dice si hay un archivo nombre en algun lugar bajo dir.
func r1HayAdentro(dir, nombre string) bool {
	hay := false
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && info.Name() == nombre {
			hay = true
		}
		return nil
	})
	return hay
}

// ---------------------------------------------------------------- punto 7

// CA-683 y CA-695 (id a8ef36; aclaracion 23): con Sobre{Rol: "test-writer",
// Ciego} directo contra Run, el gate encerrado escribe en Ciego y en la
// cache verify (su TMPDIR), y lo que intenta escribir en el arbol del
// manifest (app.go, que existe, y un archivo nuevo) no deja nada en el disco.
// Con Ciego "" (el arbol ciego ya no esta), solo la cache. Con Rol writer el
// arbol si es escribible, como hoy.
func TestR1_a8ef36_CA683_CA695_ElGateDelTestWriterEscribeEnSuCiegoYNoEnElArbol(t *testing.T) {
	const ca = "CA-683"
	cdwlEncierro(t, ca)
	for _, c := range []struct {
		caso, rol string
		ciego     bool
	}{
		{"test-writer con su arbol ciego", "test-writer", true},
		{"test-writer sin arbol ciego", "test-writer", false},
		{"writer", "writer", false},
	} {
		var ciego, app, nuevo, enCiego string
		root, _, m, _ := r1ConGate(t, "encierro:\n  exigir: no\n", func(t *testing.T, wt, comun string) (string, string) {
			ciego = filepath.Join(wt, ".hoom", "isolated", "r1-ciego")
			if c.ciego {
				if err := os.MkdirAll(ciego, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			app = filepath.Join(wt, "app.go")
			nuevo = filepath.Join(wt, "r1_del_gate.go")
			enCiego = filepath.Join(ciego, "r1_en_ciego_test.go")
			return "printf r1-del-gate >> " + app + " 2>/dev/null; printf x > " + nuevo + " 2>/dev/null; " +
				"printf x > " + enCiego + " 2>/dev/null; printf x > $TMPDIR/r1-gate-tmp; true", ""
		})
		sobre := &Sobre{Root: root, Task: cdwTarea, Rol: c.rol}
		if c.ciego {
			sobre.Ciego = ciego
		}
		appAntes := r1Leer(app)
		v, _, err := Run(m, Options{Full: true, Sobre: sobre})
		if err != nil {
			t.Fatalf("%s: %s: verify con Sobre corre: %v", ca, c.caso, err)
		}
		if g := r1GateDe(v, "test"); g == nil || g.Status != verdict.StatusPass {
			t.Fatalf("%s: %s: fixture: el gate corrio y paso: %+v", ca, c.caso, g)
		}
		cache := encierro.Cache(root, cdwTarea, encierro.QuienVerify)
		if !r1HayAdentro(cache, "r1-gate-tmp") {
			t.Errorf("CA-682: %s: la cache verify sigue escribible para el gate (TMPDIR adentro de %s)", c.caso, cache)
		}
		if c.rol == "writer" {
			if !strings.Contains(r1Leer(app), "r1-del-gate") || !r1Existe(nuevo) {
				t.Fatalf("%s: %s: control: el gate del writer escribe el arbol, como hoy: app.go %q, nuevo %v", ca, c.caso, r1Leer(app), r1Existe(nuevo))
			}
			continue
		}
		if r1Leer(app) != appAntes || r1Existe(nuevo) {
			t.Fatalf("%s: %s: el gate del test-writer no escribe la implementacion del arbol: app.go %q, nuevo %v", ca, c.caso, r1Leer(app), r1Existe(nuevo))
		}
		if c.ciego && !r1Existe(enCiego) {
			t.Fatalf("CA-695: %s: el gate del test-writer escribe en su arbol ciego %s", c.caso, ciego)
		}
		if !c.ciego && r1Existe(enCiego) {
			t.Fatalf("%s: %s: sin arbol ciego, el gate no crea uno", ca, c.caso)
		}
	}
}

// ---------------------------------------------------------------- punto 2

// CA-676 y CA-677 (id c5790e; aclaracion 22): con ReemplazarAdopcion
// simulando Windows sin job y exigir parcial (que este Linux alcanza), el
// gate encerrado muere enseguida y queda en error con el texto de SinJob del
// nivel real; el veredicto es rojo. Con exigir no, el mismo gate corre.
func TestR1_c5790e_CA676_CA677_SinJobElGateQuedaEnError(t *testing.T) {
	const ca = "CA-676"
	cdwlEncierro(t, ca)
	restaurar := encierro.ReemplazarAdopcion("windows", func(int) bool { return false })
	defer restaurar()
	info := encierro.Info{Nivel: encierro.NivelNinguno,
		Falta: []string{encierro.FaltaProcesos, encierro.FaltaEscrituras, encierro.FaltaIPC}, Motivo: encierro.MotivoSinJob}
	want := (&encierro.SinJob{Info: info, Exigir: "parcial"}).Error()
	if !strings.Contains(want, "hoom.yaml exige parcial") || !strings.HasSuffix(want, encierro.MotivoSinJob) {
		t.Fatalf("%s: fixture: el texto de SinJob: %q", ca, want)
	}
	gate := "sleep 40; true"
	root, _, m := cdwProyecto(t, gate, "encierro:\n  exigir: parcial\n")
	desde := time.Now()
	v, _, err := Run(m, Options{Full: true, Sobre: cdwSobre(root)})
	if err != nil {
		t.Fatalf("%s: verify con Sobre corre: %v", ca, err)
	}
	if d := time.Since(desde); d > 30*time.Second {
		t.Fatalf("%s: sin job el arbol del gate muere enseguida: %s", ca, d)
	}
	g := r1GateDe(v, "test")
	if g == nil || g.Status != verdict.StatusError || !strings.Contains(g.OutputTail+"\n"+g.Notes, want) {
		t.Fatalf("%s: el gate queda en error con %q: %+v", ca, want, g)
	}
	if v.Verdict != "red" {
		t.Fatalf("%s: el veredicto con un gate requerido en error es rojo: %s", ca, v.Verdict)
	}
	root, _, m = cdwProyecto(t, "true", "encierro:\n  exigir: no\n")
	v, _, err = Run(m, Options{Full: true, Sobre: cdwSobre(root)})
	if err != nil {
		t.Fatal(err)
	}
	if g := r1GateDe(v, "test"); g == nil || g.Status != verdict.StatusPass {
		t.Fatalf("CA-677: con exigir no, sin job el gate corre: %+v", g)
	}
}

// ---------------------------------------------------------------- punto 4

// CA-685 y CA-687 (id 318ca0): con la seccion y el gate encerrado (vista del
// writer), lo que el gate logra cambiar de git-ajeno nunca pasa sin verse:
// si la ref de reemplazo o el objeto cambiaron en el disco, git_ajeno esta en
// fail con esa ruta y el veredicto es rojo.
func TestR1_318ca0_CA685_CA687_ConElGateEncerradoGitAjenoSeVe(t *testing.T) {
	const ca = "CA-685"
	cdwlEncierro(t, ca)
	for _, c := range r1CasosGitAjeno {
		var antes string
		root, _, m, ruta := r1ConGate(t, "encierro:\n  exigir: no\n", func(t *testing.T, wt, comun string) (string, string) {
			gate, ruta := c.arma(t, wt, comun)
			antes = r1Leer(ruta)
			return gate, ruta
		})
		v, _, err := Run(m, Options{Full: true, Sobre: cdwSobre(root)})
		if err != nil {
			t.Fatalf("%s: %s: verify con Sobre corre: %v", ca, c.nombre, err)
		}
		if r1Leer(ruta) == antes {
			t.Logf("%s: %s: el gate encerrado no logro cambiar %s", ca, c.nombre, ruta)
			continue
		}
		g := r1GateDe(v, GateGitAjeno)
		if g == nil || g.Status != verdict.StatusFail || !r1Tiene(g.OutputTail, ruta) || v.Verdict != "red" {
			t.Fatalf("%s: %s: el gate encerrado cambio %s y git_ajeno lo nombra, con el veredicto rojo: %+v %s", ca, c.nombre, ruta, g, v.Verdict)
		}
	}
}
