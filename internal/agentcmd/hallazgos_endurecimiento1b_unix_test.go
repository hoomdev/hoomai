//go:build !windows

// Tests de regresion de la segunda ronda de la tarea endurecimiento-1 en
// agentcmd que necesitan un FIFO (el de internal/carreratest, que nadie
// escribe y que se suelta al final del test). Escritos desde el spec, el
// texto del hallazgo y el contrato de la correccion, SIN LEER LA
// IMPLEMENTACION. Reusan e1Take y e1Reloj de la primera ronda
// (hallazgos_endurecimiento1_unix_test.go); los helpers de este archivo
// llevan el prefijo e1b.
//
//   - 20261022T203201_f472ee (resilience): el gate calculaba la huella con
//     tope (HuellaConTope) de nuevo por cada violacion, para el hallazgo que
//     registra. Con un git del listado bloqueado (un FIFO en
//     .hoom/findings/.gitignore), cada hallazgo consumia el TopeListado
//     entero: un rol que deja muchas rutas en infraccion convertia el limite
//     en N x TopeListado y dejaba el gate y su sobre bloqueados por horas.
//     El gate vuelve en un tiempo que no crece con la cantidad de
//     violaciones, y corta (CA-442, CA-141). La correccion agrega
//     finding.Draft.Huella (*string); este test no la usa.
package agentcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/carreratest"
	"github.com/hoomdev/hoomai/internal/gitx"
)

const (
	// e1bViolaciones es cuantos archivos fuera de lugar deja el rol.
	e1bViolaciones = 20
	// e1bTopeGate es lo mas que puede tardar el gate con TopeListado en
	// 2 s: mucho menos que e1bViolaciones x 2 s (40 s).
	e1bTopeGate = 15 * time.Second
	// e1bEsperaGate es cuanto se espera al gate viejo para que termine
	// limpio antes de soltar el FIFO.
	e1bEsperaGate = 2 * time.Minute
)

// CA-442, CA-141 (hallazgo f472ee): con gitx.TopeListado en 2 s, el rol
// writer deja un FIFO (que nadie escribe) en .hoom/findings/.gitignore y
// ademas 20 archivos fuera de lugar bajo .hoom/findings (sin la forma de un
// hallazgo: cada uno es una violacion por si mismo, la liste git o no).
// Con la foto de despues, el gate de scope:
//   - vuelve en mucho menos que 20 x 2 s (menos de 15 s): calcular la huella
//     de los hallazgos de las violaciones no consume el tope una vez por
//     violacion;
//   - corta (Cuts);
//   - registra las violaciones (al menos 20: el fixture si las produce).
func TestHallazgo_f472ee_CA442_ElGateNoMultiplicaElTopePorCadaViolacion(t *testing.T) {
	const ca = "CA-442 (hallazgo f472ee)"
	qcLimpiarEntorno(t)
	viejo := gitx.TopeListado
	gitx.TopeListado = 2 * time.Second
	t.Cleanup(func() { gitx.TopeListado = viejo })

	root := repo(t)
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "findings"), 0o755); err != nil {
		t.Fatal(err)
	}
	before := e1Take(t, ca, "foto de antes", root, e1Reloj)
	if before.Listado != "" {
		t.Fatalf("%s: guarda: sin el FIFO, git lista y la foto no dice que no pudo: %q", ca, before.Listado)
	}

	fifo := filepath.Join(root, ".hoom", "findings", ".gitignore")
	carreratest.Fifo(t, fifo)
	for i := 0; i < e1bViolaciones; i++ {
		write(t, root, fmt.Sprintf(".hoom/findings/fuera-de-lugar-%02d.txt", i), "no es un hallazgo de hoom\n")
	}
	after := e1Take(t, ca, "foto de despues", root, e1Reloj)

	rol, err := agents.Lookup("writer")
	if err != nil {
		t.Fatal(err)
	}
	ch := make(chan ScopeResult, 1)
	t0 := time.Now()
	go func() { ch <- Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil) }()
	var res ScopeResult
	tarde := false
	select {
	case res = <-ch:
	case <-time.After(e1bTopeGate):
		tarde = true
		t.Errorf("%s: con %d violaciones y TopeListado en 2 s, el gate vuelve en menos de %s (no %d x 2 s); no volvio en %s",
			ca, e1bViolaciones, e1bTopeGate, e1bViolaciones, e1bTopeGate)
		select {
		case res = <-ch:
		case <-time.After(e1bEsperaGate):
			t.Fatalf("%s: el gate no volvio ni en %s", ca, e1bTopeGate+e1bEsperaGate)
		}
	}
	tarda := time.Since(t0)
	t.Logf("%s: el gate volvio en %s con %d violaciones", ca, tarda.Round(time.Millisecond), len(res.Violations))
	if tarda >= e1bTopeGate && !tarde {
		t.Errorf("%s: el gate tardo %s con %d violaciones: con TopeListado en 2 s tiene que volver en menos de %s",
			ca, tarda.Round(time.Millisecond), len(res.Violations), e1bTopeGate)
	}
	if !res.Cuts() {
		t.Errorf("%s: con el FIFO y los archivos fuera de lugar, el gate de scope corta: %+v", ca, res)
	}
	if len(res.Violations) < e1bViolaciones {
		t.Errorf("%s: fixture: los %d archivos fuera de lugar son violaciones; hay %d: %+v", ca, e1bViolaciones, len(res.Violations), res.Violations)
	}
}
