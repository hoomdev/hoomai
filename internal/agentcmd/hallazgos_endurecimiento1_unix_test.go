//go:build !windows

// Tests de regresion de la tarea endurecimiento-1 en agentcmd, sobre
// .hoom/specs/evidencia-en-disco.md (CA-440..CA-445) y el gate de scope de
// .hoom/specs/hoom-agent-sobre-determinista.md. Escritos desde el spec, el
// texto del hallazgo y el contrato de la correccion, SIN LEER LA
// IMPLEMENTACION. Viven en un archivo de unix porque mkfifo es de unix (el
// FIFO es el de internal/carreratest). Los helpers de este archivo llevan el
// prefijo e1.
//
//   - 20261002T144018_5c5342 (risk): la foto (Take) listaba con git
//     (ls-files --others --exclude-standard) ANTES de fotografiar la
//     evidencia en el disco, y sin tope de tiempo. Un rol que deja un FIFO en
//     .hoom/findings/.gitignore hace que git lo abra como archivo de
//     exclusiones y se quede bloqueado: la foto de despues nunca llega al piso
//     en disco ni registra la manipulacion. gitx.TopeListado acota cada git
//     del listado; con el FIFO, Take vuelve en tiempo acotado, dice en
//     Snapshot.Listado por que git no pudo listar, la foto del disco ya marca
//     el FIFO como no regular, y el gate corta (CA-442: un FIFO "nunca se
//     sigue ni se lee"; CA-141: la manipulacion corta).
package agentcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/agents"
	"github.com/hoomdev/hoomai/internal/carreratest"
	"github.com/hoomdev/hoomai/internal/gitx"
)

// e1Reloj es cuanto puede tardar la foto con el FIFO: con TopeListado en
// 2 s, cada git del listado vuelve en 2 s; mas que esto es que se colgo.
const e1Reloj = 30 * time.Second

// e1Take es Take con reloj: si no vuelve, el test falla en vez de colgar la
// suite (el FIFO se suelta al final del test y la foto colgada termina).
func e1Take(t *testing.T, ca, caso, root string, reloj time.Duration) Snapshot {
	t.Helper()
	ch := make(chan Snapshot, 1)
	t0 := time.Now()
	go func() { ch <- Take(root, "main") }()
	select {
	case s := <-ch:
		t.Logf("%s: %s: Take volvio en %s", ca, caso, time.Since(t0).Round(time.Millisecond))
		return s
	case <-time.After(reloj):
		t.Fatalf("%s: %s: Take no volvio en %s: un git del listado se quedo bloqueado abriendo el FIFO y la foto nunca llego al disco",
			ca, caso, reloj)
	}
	return Snapshot{}
}

// e1Gate es Gate con reloj.
func e1Gate(t *testing.T, ca, root string, rol agents.Role, before, after Snapshot) ScopeResult {
	t.Helper()
	ch := make(chan ScopeResult, 1)
	go func() { ch <- Gate(root, "main", "", rol, before, after, PolicyFor(nil, rol), nil) }()
	select {
	case r := <-ch:
		return r
	case <-time.After(2 * e1Reloj):
		t.Fatalf("%s: el gate del rol %s no volvio en %s con el FIFO en .hoom/findings/.gitignore", ca, rol.Slug, 2*e1Reloj)
	}
	return ScopeResult{}
}

// CA-442, CA-141 (hallazgo 5c5342): un rol deja un FIFO (que nadie escribe)
// en .hoom/findings/.gitignore. Con gitx.TopeListado en 2 s:
//   - Take vuelve en tiempo acotado (no se cuelga en el git que lista);
//   - la foto dice por que git no pudo listar: Snapshot.Listado no vacio;
//   - la foto del disco ya marca .hoom/findings/.gitignore como no regular
//     (HuellaNoRegular), sin abrirlo;
//   - el gate de scope con esa foto de despues corta (Cuts).
//
// Guarda: la foto de antes (sin el FIFO) no tiene Listado.
func TestHallazgo_5c5342_CA442_FIFOEnElGitignoreDeFindingsNoCuelgaLaFoto(t *testing.T) {
	const ca = "CA-442 (hallazgo 5c5342)"
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

	const rel = ".hoom/findings/.gitignore"
	carreratest.Fifo(t, filepath.Join(root, rel))
	after := e1Take(t, ca, "foto con el FIFO", root, e1Reloj)

	if after.Listado == "" {
		t.Errorf("%s: con el FIFO en %s git no pudo listar, y la foto lo dice: Snapshot.Listado no vacio", ca, rel)
	}
	if h, ok := after.Huellas[rel]; !ok || !strings.HasPrefix(h, HuellaNoRegular) {
		t.Errorf("%s: la foto del disco marca %s como no regular (%q...), sin abrirlo; tiene %q (esta: %v)", ca, rel, HuellaNoRegular, h, ok)
	}

	rol, err := agents.Lookup("writer")
	if err != nil {
		t.Fatal(err)
	}
	res := e1Gate(t, ca, root, rol, before, after)
	if !res.Cuts() {
		t.Errorf("%s: con la foto de despues sin listado y con el FIFO, el gate de scope corta: %+v", ca, res)
	}
}
