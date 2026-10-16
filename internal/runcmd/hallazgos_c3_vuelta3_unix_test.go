//go:build unix

// Test de regresion del hallazgo 1da9c4 de la review delta 1 de las
// correcciones de C3 (.hoom/specs/acciones-desde-la-tarjeta.md), del lado de
// runcmd. Escrito desde el contrato (go doc Manager.Input, LockArbol, Metas)
// y el hallazgo, sin leer la implementacion.
//
// CA-346 (enmienda 1): "con un run activo en el arbol se niega, tambien si
// [...] se esta registrando". El registro del run (su sidecar en
// .hoom/runs/) es lo unico con lo que OTRO proceso (Descartar, Guardar, otro
// Studio, CA-340) sabe que el arbol esta ocupado; continuar un run terminado
// (Manager.Input) es registrarlo de nuevo (go doc LockArbol: "Registering a
// run on it (Start, Input)"). Igual que Start (hallazgo da369a), si el
// registro no se puede escribir, Input devuelve error, el run queda como
// estaba (terminal, mismo exit code, sigue continuable) y el provider no se
// vuelve a invocar: un provider trabajando sin registro es un run que nadie
// mas ve.
//
// Ningun CA del spec dice con mas precision "un run se registra en disco
// para que otro proceso lo vea"; CA-346 es el que lo exige (y CA-340 del
// lado del Studio).
//
// Reusa installFake e initRepo (runcmd_test.go) y waitRun (options_test.go).
package runcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// v3Invocaciones cuenta las invocaciones del provider falso: una linea por
// invocacion en marca.
func v3Invocaciones(t *testing.T, marca string) int {
	t.Helper()
	raw, err := os.ReadFile(marca)
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	return strings.Count(string(raw), "\n")
}

// v3Provider es el provider falso: deja una linea en marca por invocacion,
// informa su sesion y termina enseguida: la primera invocacion con exit 3 y
// las siguientes con 0 (asi una invocacion de mas cambia el exit code del
// run).
func v3Provider(marca string) string {
	return "echo invocacion >> '" + marca + "'\n" +
		"n=$(wc -l < '" + marca + "' | tr -d ' ')\n" +
		`printf '{"type":"system","subtype":"init","session_id":"s-1da9c4"}\n'` + "\n" +
		"echo \"invocacion $n\"\n" +
		"if [ \"$n\" -eq 1 ]; then exit 3; fi\n" +
		"exit 0\n"
}

// v3SinRegistro deja el registro del run (meta) imposible de escribir, de
// una de dos formas, y devuelve como restaurarlo (idempotente; tambien se
// restaura al final del test).
var v3SinRegistro = []struct {
	nombre   string
	preparar func(t *testing.T, runs, meta string) func()
}{
	// .hoom/runs sin permiso de escritura (no se puede crear ni renombrar
	// nada ahi) y el sidecar de solo lectura (tampoco se puede reescribir en
	// su lugar).
	{"runs-sin-permiso-de-escritura", func(t *testing.T, runs, meta string) func() {
		if os.Geteuid() == 0 {
			t.Skip("como root el permiso de escritura no frena a nadie")
		}
		if err := os.Chmod(meta, 0o444); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(runs, 0o555); err != nil {
			t.Fatal(err)
		}
		var una sync.Once
		restaurar := func() {
			una.Do(func() {
				os.Chmod(runs, 0o755)
				os.Chmod(meta, 0o644)
			})
		}
		t.Cleanup(restaurar)
		return restaurar
	}},
	// macOS: el sidecar inmutable (chflags uchg): no se puede reescribir ni
	// reemplazar con un rename, aunque el directorio si se pueda escribir.
	{"registro-inmutable", func(t *testing.T, runs, meta string) func() {
		if runtime.GOOS != "darwin" {
			t.Skip("chflags uchg es de macOS")
		}
		if out, err := exec.Command("chflags", "uchg", meta).CombinedOutput(); err != nil {
			t.Skipf("chflags uchg no se pudo poner: %v %s", err, out)
		}
		var una sync.Once
		restaurar := func() { una.Do(func() { exec.Command("chflags", "nouchg", meta).Run() }) }
		t.Cleanup(restaurar)
		return restaurar
	}},
}

// Hallazgo 1da9c4, CA-346: continuar un run terminado (Manager.Input) cuando
// su registro no se puede escribir devuelve error, deja el run como estaba
// (terminal en Get, el mismo exit code, el sidecar en disco igual, el arbol
// libre) y no vuelve a invocar al provider. Restaurado el permiso, el run se
// puede continuar: la negativa no lo dejo trabado.
func TestHallazgo_1da9c4_CA346_InputSinRegistroNoContinua(t *testing.T) {
	for _, c := range v3SinRegistro {
		t.Run(c.nombre, func(t *testing.T) {
			marca := filepath.Join(t.TempDir(), "invocaciones")
			installFake(t, v3Provider(marca))
			root := initRepo(t)
			m := NewManager(root)

			r, err := m.Start(StartOptions{Provider: "claude", Prompt: "primer turno"})
			if err != nil {
				t.Fatalf("CA-346: fixture: Start: %v", err)
			}
			antes := m.Wait(r.ID)
			if antes.Status == StatusRunning || antes.ExitCode != 3 {
				t.Fatalf("CA-346: fixture: el primer turno termina con exit 3: %+v", antes)
			}
			if n := v3Invocaciones(t, marca); n != 1 {
				t.Fatalf("CA-346: fixture: el provider corrio una vez: %d", n)
			}
			runs := filepath.Join(root, ".hoom", "runs")
			meta := filepath.Join(runs, r.ID+".meta.json")
			metaAntes, err := os.ReadFile(meta)
			if err != nil {
				t.Fatalf("CA-346: fixture: el registro del run %s esta en disco: %v", r.ID, err)
			}
			restaurar := c.preparar(t, runs, meta)

			cont, err := m.Input(r.ID, "segundo turno")
			if err == nil {
				waitRun(t, m, r.ID)
			}
			// un provider que arranco igual ya dejo su marca
			for hasta := time.Now().Add(700 * time.Millisecond); time.Now().Before(hasta) && v3Invocaciones(t, marca) == 1; {
				time.Sleep(20 * time.Millisecond)
			}
			n := v3Invocaciones(t, marca)
			if err == nil {
				t.Fatalf("hallazgo 1da9c4 (CA-346): si el registro del run no se puede escribir (%s), Input devuelve error; devolvio %+v (el provider se invoco %d veces en total)", c.nombre, cont, n)
			}
			if n != 1 {
				t.Fatalf("hallazgo 1da9c4 (CA-346): sin registro (%s) el provider no se vuelve a invocar: se invoco %d veces (error: %v)", c.nombre, n, err)
			}
			got, gerr := m.Get(r.ID)
			if gerr != nil {
				t.Fatalf("hallazgo 1da9c4 (CA-346): el run sigue estando despues de la negativa: %v", gerr)
			}
			if got.Status != antes.Status || got.ExitCode != antes.ExitCode {
				t.Fatalf("hallazgo 1da9c4 (CA-346): la negativa deja el run como estaba: era %s/%d, quedo %s/%d", antes.Status, antes.ExitCode, got.Status, got.ExitCode)
			}
			if id, busy := m.Busy(root); busy {
				t.Fatalf("hallazgo 1da9c4 (CA-346): sin registro (%s) el arbol no queda ocupado: Busy ve %s", c.nombre, id)
			}
			for _, mt := range Metas(root) {
				if mt.Status == StatusRunning {
					t.Fatalf("hallazgo 1da9c4 (CA-346): sin registro (%s) no queda ningun sidecar running: %+v", c.nombre, mt)
				}
			}
			if raw, err := os.ReadFile(meta); err != nil || string(raw) != string(metaAntes) {
				t.Fatalf("hallazgo 1da9c4 (CA-346): el registro del run en disco queda igual (error de lectura: %v):\nantes:\n%s\ndespues:\n%s", err, metaAntes, raw)
			}

			// Restaurado el registro, el run se puede continuar.
			restaurar()
			if _, err := m.Input(r.ID, "segundo turno"); err != nil {
				t.Fatalf("hallazgo 1da9c4 (CA-346): la negativa no deja el run trabado: con el registro restaurado, Input continua: %v", err)
			}
			fin := waitRun(t, m, r.ID)
			if n := v3Invocaciones(t, marca); n != 2 || fin.ExitCode != 0 {
				t.Fatalf("hallazgo 1da9c4 (CA-346): con el registro restaurado el provider se invoca una vez mas y el run termina con su exit (0): %d invocaciones, %+v", n, fin)
			}
		})
	}
}

// Hallazgo 1da9c4, CA-346 (guarda): con el registro escribible, el mismo
// run terminado se continua: Input no da error y el provider se invoca una
// segunda vez.
func TestHallazgo_1da9c4_CA346_GuardaInputConRegistroContinua(t *testing.T) {
	marca := filepath.Join(t.TempDir(), "invocaciones")
	installFake(t, v3Provider(marca))
	root := initRepo(t)
	m := NewManager(root)
	r, err := m.Start(StartOptions{Provider: "claude", Prompt: "primer turno"})
	if err != nil {
		t.Fatal(err)
	}
	if antes := m.Wait(r.ID); antes.Status == StatusRunning || antes.ExitCode != 3 {
		t.Fatalf("CA-346: fixture: el primer turno termina con exit 3: %+v", antes)
	}
	if _, err := m.Input(r.ID, "segundo turno"); err != nil {
		t.Fatalf("hallazgo 1da9c4 (CA-346): guarda: con el registro escribible Input continua el run: %v", err)
	}
	fin := waitRun(t, m, r.ID)
	if n := v3Invocaciones(t, marca); n != 2 || fin.ExitCode != 0 {
		t.Fatalf("hallazgo 1da9c4 (CA-346): guarda: el provider se invoca una segunda vez y el run termina con su exit (0): %d invocaciones, %+v", n, fin)
	}
}
