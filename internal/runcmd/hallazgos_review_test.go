// Tests de regresion de la review cruzada (Codex, 2026-09-06) sobre Spec D + Spec E.
package runcmd

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hrSidecar planta en .hoom/runs lo que OTRO proceso deja mientras corre: el
// sidecar como JSON crudo (asi puede llevar campos que Meta todavia no tiene,
// como pid, o un id distinto del nombre del archivo) y una narracion cuyo
// ultimo evento NO es terminal. Ambos archivos quedan con el mtime pedido.
func hrSidecar(t *testing.T, root, archivo string, sidecar map[string]any, mtime time.Time) {
	t.Helper()
	runs := runsDir(root)
	if err := os.MkdirAll(runs, 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.MarshalIndent(sidecar, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	metaFile := filepath.Join(runs, archivo+".meta.json")
	if err := os.WriteFile(metaFile, append(raw, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	linea, _ := json.Marshal(Event{TS: mtime, Kind: "start", Detail: "run " + archivo + ": claude en el proyecto"})
	logFile := filepath.Join(runs, archivo+".jsonl")
	if err := os.WriteFile(logFile, append(linea, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{metaFile, logFile} {
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}
}

// hrMetaCrudo describe un run en curso tal como lo escribe Start, mas el pid
// del proceso dueno (campo que Meta hoy no tiene).
func hrMetaCrudo(id, dir string, pid int, creado time.Time) map[string]any {
	return map[string]any{
		"id": id, "provider": "claude", "dir": dir, "created_at": creado,
		"status": StatusRunning, "exit_code": -1, "pid": pid,
	}
}

// hrPidMuerto devuelve el pid de un proceso que ya termino y fue recogido.
func hrPidMuerto(t *testing.T) int {
	t.Helper()
	muerto := exec.Command("true")
	if err := muerto.Run(); err != nil {
		t.Fatal(err)
	}
	return muerto.ProcessState.Pid()
}

// hrArbol lista todo lo que hay bajo root FUERA de .hoom/runs, como texto
// comparable.
func hrArbol(t *testing.T, root string) string {
	t.Helper()
	var rutas []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == runsDir(root) {
			return filepath.SkipDir
		}
		rel, _ := filepath.Rel(root, p)
		rutas = append(rutas, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return strings.Join(rutas, "\n")
}

// Hallazgos 20260906T235815_609005, 20260907T000657_77002d y
// 20260907T001832_b4538a: NewManager -> markOrphans cierra como huerfano
// cualquier run cuyo log no termine en end/error y settleMeta pasa su sidecar
// de running a error, sin comprobar si el proceso dueno sigue vivo. Como
// agentcmd.Run crea el manager ANTES de Busy, un run real de otro proceso queda
// falsamente cerrado y el sobre arranca un segundo actor (y un trasplante)
// sobre un arbol que sigue siendo editado.
//
// Suposicion sobre el arreglo: el sidecar lleva el pid del proceso dueno en el
// campo JSON "pid" (Meta hoy no lo tiene) y NewManager solo cierra un run cuyo
// pid ya no existe. El run muerto ademas es viejo (mtime de horas), asi que un
// arreglo por antiguedad tambien lo distingue del vivo, que es reciente.
func TestHallazgo_609005_NewManagerNoCierraElRunDeUnProcesoVivo(t *testing.T) {
	root := t.TempDir()

	// el dueno vivo es OTRO proceso, no este test
	vivo := exec.Command("sleep", "60")
	if err := vivo.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vivo.Process.Kill(); vivo.Wait() })
	pidMuerto := hrPidMuerto(t)

	dirVivo := filepath.Join(root, "arbol-vivo")
	dirMuerto := filepath.Join(root, "arbol-muerto")
	for _, d := range []string{dirVivo, dirMuerto} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	const idVivo, idMuerto = "20260906T230000_a11e00", "20260906T200000_0dead0"
	ahora := time.Now().UTC()
	hrSidecar(t, root, idVivo, hrMetaCrudo(idVivo, dirVivo, vivo.Process.Pid, ahora), ahora)
	hace3h := ahora.Add(-3 * time.Hour)
	hrSidecar(t, root, idMuerto, hrMetaCrudo(idMuerto, dirMuerto, pidMuerto, hace3h), hace3h)

	m := NewManager(root)

	if meta := leerMeta(t, root, idVivo); meta.Status != StatusRunning {
		t.Fatalf("609005: el run de un proceso vivo (pid %d) no es huerfano; NewManager dejo su sidecar en %q", vivo.Process.Pid, meta.Status)
	}
	evs, err := ReadEvents(root, idVivo)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(evs); n == 0 || evs[n-1].Kind == "error" {
		t.Fatalf("609005: tampoco se le agrega el evento de huerfano a su narracion: %+v", evs)
	}
	if id, busy := m.Busy(dirVivo); !busy || id != idVivo {
		t.Fatalf("609005: Busy debe seguir viendo ocupado el arbol del proceso vivo (dijo %q, %v)", id, busy)
	}

	// el que SI murio se cierra como hasta ahora
	if meta := leerMeta(t, root, idMuerto); meta.Status != StatusError {
		t.Fatalf("609005: el run de un proceso muerto (pid %d) sigue cerrandose como huerfano: %+v", pidMuerto, meta)
	}
	if id, busy := m.Busy(dirMuerto); busy {
		t.Fatalf("609005: y su arbol queda libre (Busy dijo %q)", id)
	}
}

// Hallazgo 20260907T001832_31542d: settleMeta lee <id>.meta.json por el nombre
// seguro pero escribe con writeMeta(meta) usando meta.ID del CONTENIDO, y
// writeMeta arma la ruta con ese id: un sidecar plantado bajo .hoom/runs (ruta
// excluida del scope) con "id":"../../evil" hace que hoom escriba fuera de
// runsDir al abrir el siguiente Manager. El run se planta muerto y viejo para
// que el cierre de huerfanos SI lo intente (ver TestHallazgo_609005).
func TestHallazgo_31542d_SettleMetaNoEscribeFueraDeRuns(t *testing.T) {
	root := filepath.Join(t.TempDir(), "proyecto")
	const archivo = "20260906T210000_7ra1c0"
	hace3h := time.Now().UTC().Add(-3 * time.Hour)
	hrSidecar(t, root, archivo, hrMetaCrudo("../../evil", root, hrPidMuerto(t), hace3h), hace3h)
	antes := hrArbol(t, root)

	NewManager(root)

	if fuga := filepath.Join(root, "evil.meta.json"); hrExiste(fuga) {
		t.Fatalf("31542d: NewManager escribio %s: el id del contenido del sidecar decidio la ruta", fuga)
	}
	if despues := hrArbol(t, root); despues != antes {
		t.Fatalf("31542d: fuera de .hoom/runs no puede aparecer nada al abrir un Manager\n antes:\n%s\n despues:\n%s", antes, despues)
	}
	entries, err := os.ReadDir(runsDir(root))
	if err != nil {
		t.Fatal(err)
	}
	var nombres []string
	for _, e := range entries {
		nombres = append(nombres, e.Name())
	}
	if strings.Join(nombres, ",") != archivo+".jsonl,"+archivo+".meta.json" {
		t.Fatalf("31542d: un sidecar con id traicionero tampoco genera archivos nuevos dentro de .hoom/runs: %v", nombres)
	}
}

func hrExiste(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
