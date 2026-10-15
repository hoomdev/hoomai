// Test de regresion del hallazgo 20261014T203135_57eff9 (medium) de la mitad 2
// de la review de C3 (.hoom/specs/acciones-desde-la-tarjeta.md), en lo que
// le tocaba a runcmd. Escrito desde el contrato (go doc Manager.Start,
// ErrBusy) y el hallazgo, sin leer la implementacion: "un solo run activo por
// arbol" vale entre procesos. Dos Manager distintos (como dos procesos: dos
// Studios, o el Studio y una terminal) sobre el mismo arbol: con un run vivo
// registrado por el primero, Start del segundo sobre ese arbol devuelve
// ErrBusy con el id del primero y no registra nada.
//
// Reusa installFake, initRepo y allDetails de runcmd_test.go.
package runcmd

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// m2Runs lista los archivos de root/.hoom/runs.
func m2Runs(t *testing.T, root string) []string {
	t.Helper()
	entradas, err := os.ReadDir(filepath.Join(root, ".hoom", "runs"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	var nombres []string
	for _, e := range entradas {
		nombres = append(nombres, e.Name())
	}
	sort.Strings(nombres)
	return nombres
}

// m2Cancelar deja para el final del test cancelar el run id de m, despues de
// que su subproceso dijo marca (para que la cancelacion lo alcance).
func m2Cancelar(t *testing.T, m *Manager, id, marca string) {
	t.Helper()
	t.Cleanup(func() {
		hasta := time.Now().Add(5 * time.Second)
		for time.Now().Before(hasta) {
			if _, evs, err := m.Events(id, 0); err != nil || strings.Contains(allDetails(evs), marca) {
				break
			}
			time.Sleep(20 * time.Millisecond)
		}
		m.Cancel(id)
		m.Wait(id)
	})
}

// Hallazgo 20261014T203135_57eff9 (medium): con un run vivo registrado por un
// Manager sobre el arbol, Start de OTRO Manager (otro proceso) sobre el mismo
// arbol devuelve ErrBusy con el id del primero y no registra nada: ni un
// sidecar ni una narracion nuevos, ni un run en su memoria. El run del
// primero sigue en curso.
func TestHallazgo_57eff9_OtroManagerSobreElMismoArbolRecibeErrBusy(t *testing.T) {
	installFake(t, "echo arranco\nsleep 30\n")
	root := initRepo(t)
	m1 := NewManager(root)
	r1, err := m1.Start(StartOptions{Provider: "claude", Prompt: "trabajo largo"})
	if err != nil {
		t.Fatal(err)
	}
	m2Cancelar(t, m1, r1.ID, "arranco")
	antes := m2Runs(t, root)

	m2 := NewManager(root)
	r2, err := m2.Start(StartOptions{Provider: "claude", Prompt: "otro trabajo"})
	if err == nil {
		m2Cancelar(t, m2, r2.ID, "arranco")
		t.Fatalf("hallazgo 57eff9: con el run %s vivo en el arbol, Start de otro Manager se niega con ErrBusy; registro %s", r1.ID, r2.ID)
	}
	var ocupado ErrBusy
	var ocupadoPtr *ErrBusy
	switch {
	case errors.As(err, &ocupado):
	case errors.As(err, &ocupadoPtr) && ocupadoPtr != nil:
		ocupado = *ocupadoPtr
	default:
		t.Fatalf("hallazgo 57eff9: Start de otro Manager sobre un arbol ocupado devuelve runcmd.ErrBusy: %T %v", err, err)
	}
	if ocupado.RunID != r1.ID {
		t.Fatalf("hallazgo 57eff9: el ErrBusy nombra el run del primero (%s): %q", r1.ID, ocupado.RunID)
	}
	if despues := m2Runs(t, root); strings.Join(despues, "\n") != strings.Join(antes, "\n") {
		t.Fatalf("hallazgo 57eff9: negado, Start no registra nada en .hoom/runs:\nantes   %v\ndespues %v", antes, despues)
	}
	if runs := m2.List(); len(runs) != 0 {
		t.Fatalf("hallazgo 57eff9: negado, el segundo Manager no tiene ningun run: %+v", runs)
	}
	if meta, ok := ReadMeta(root, r1.ID); !ok || meta.Status != StatusRunning {
		t.Fatalf("hallazgo 57eff9: el run del primero sigue en curso: %+v (existe: %v)", meta, ok)
	}
}
