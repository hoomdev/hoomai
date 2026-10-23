// Tests de regresion de la segunda ronda de la tarea endurecimiento-1 en
// envelope. Escritos desde el texto de cada hallazgo, las specs y el
// contrato que promete la correccion, SIN LEER LA IMPLEMENTACION. Reusan los
// helpers e1* de la primera ronda (hallazgos_endurecimiento1_test.go); los
// de este archivo llevan el prefijo e1b.
//
//   - 20261022T202056_9e7983 y 20261022T203213_a2b180 (risk, resilience):
//     ListErr omitia en silencio el registro de un sobre que no podia abrir:
//     con .hoom/envelopes listable y un <id>.json ilegible, devolvia exito y
//     el doctor certificaba que no habia sobres huerfanos sin haber leido el
//     unico. Ahora ListErr devuelve error (y los registros legibles igual);
//     List sigue salteandolo sin error (CA-202).
//   - 20261022T202056_94636a, 20261022T203200_be5b7a y
//     20261022T203201_30eb68 (risk, resilience): cada Cerrar fallido creaba
//     una goroutine propia, sin limite ni cancelacion, que reintentaba para
//     siempre mientras el registro existiera (aunque fuera un directorio); y
//     un error transitorio al mirar el registro abandonaba el cierre para
//     siempre. Ahora: N cierres fallidos crean a lo sumo una goroutine de
//     reintento; un registro reemplazado por un directorio termina los
//     reintentos sin escribir nada; un error transitorio no abandona el
//     cierre. Primer reintento a 1 s, despues 2 s, 4 s (CA-333, CA-202).
package envelope

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------- 9e7983 / a2b180

// CA-202 (hallazgos 9e7983 y a2b180): con .hoom/envelopes listable y el
// registro <id>.json de un sobre ilegible (000) al lado de otro legible,
// ListErr devuelve error (no pudo leer toda la evidencia) y, aun asi, trae
// el registro legible. List (best-effort) sigue sin error: trae el legible
// y saltea el ilegible. Guarda: con los dos legibles, ListErr trae los dos
// sin error.
func TestHallazgo_9e7983_a2b180_CA202_ListErrDiceQueUnRegistroNoSePudoLeer(t *testing.T) {
	const ca = "CA-202 (hallazgos 9e7983 y a2b180, ListErr con un registro ilegible)"
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("sin permisos de unix (o como root) un chmod no quita la lectura")
	}
	const ilegible = "20261022T100700_e1b0c3"
	root := t.TempDir()
	e1Escribir(t, root, e1Rec(e1IDPropio, "propio"))
	e1Escribir(t, root, e1Rec(ilegible, "ilegible"))
	if rs, err := ListErr(root); err != nil || len(rs) != 2 {
		t.Fatalf("%s: guarda: con los dos registros legibles, ListErr trae los dos sin error: %v %v", ca, e1IDs(rs), err)
	}

	reg := filepath.Join(root, ".hoom", DirName, ilegible+".json")
	if err := os.Chmod(reg, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(reg, 0o644) })
	if _, err := os.ReadFile(reg); err == nil {
		t.Skip("este sistema deja leer un archivo sin permiso de lectura")
	}
	if _, err := os.ReadDir(filepath.Dir(reg)); err != nil {
		t.Fatalf("fixture: .hoom/envelopes sigue listable: %v", err)
	}

	rs, err := ListErr(root)
	if err == nil {
		t.Errorf("%s: con el registro %s ilegible (000), ListErr devuelve error: no pudo leer toda la evidencia; dio %v sin error",
			ca, ilegible, e1IDs(rs))
	}
	if len(rs) != 1 || rs[0].ID != e1IDPropio {
		t.Errorf("%s: ListErr trae igual el registro legible (%s) y solo ese: %v", ca, e1IDPropio, e1IDs(rs))
	}
	if ls := List(root); len(ls) != 1 || ls[0].ID != e1IDPropio {
		t.Errorf("%s: List (best-effort) saltea el ilegible y trae el legible: %v", ca, e1IDs(ls))
	}
}

// ---------------------------------------------------------------- 94636a / be5b7a / 30eb68

// e1bPaquete es la cantidad de goroutines vivas que corren codigo de este
// paquete y no son de un test: las de reintento de Cerrar.
func e1bPaquete() int {
	buf := make([]byte, 1<<20)
	for {
		n := runtime.Stack(buf, true)
		if n < len(buf) {
			buf = buf[:n]
			break
		}
		buf = make([]byte, 2*len(buf))
	}
	c := 0
	for _, g := range bytes.Split(buf, []byte("\n\n")) {
		s := string(g)
		if strings.Contains(s, "hoomai/internal/envelope.") && !strings.Contains(s, "_test.go") {
			c++
		}
	}
	return c
}

// e1bGoroutinas son las goroutines del proceso y las de reintento del
// paquete.
func e1bGoroutinas() (todas, paquete int) {
	return runtime.NumGoroutine(), e1bPaquete()
}

// e1bID es el id del i-esimo sobre de estos tests.
func e1bID(i int) string { return fmt.Sprintf("20261022T1100%02d_e1b%03x", i%60, i) }

// e1bSinEscritura deja los registros y .hoom/envelopes sin escritura y
// devuelve la funcion que los devuelve a como estaban. Saltea donde los
// permisos no frenan la escritura.
func e1bSinEscritura(t *testing.T, root string, ids []string) func() {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("sin permisos de unix (o como root) un chmod no quita la escritura")
	}
	dir := filepath.Join(root, ".hoom", DirName)
	for _, id := range ids {
		if err := os.Chmod(filepath.Join(dir, id+".json"), 0o444); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	devolver := func() {
		_ = os.Chmod(dir, 0o755)
		for _, id := range ids {
			_ = os.Chmod(filepath.Join(dir, id+".json"), 0o644)
		}
	}
	t.Cleanup(devolver)
	if f, err := os.CreateTemp(dir, "sonda"); err == nil {
		f.Close()
		os.Remove(f.Name())
		t.Skip("este sistema deja escribir en un directorio sin permiso de escritura")
	}
	return devolver
}

// e1bAbiertos escribe n sobres abiertos y devuelve sus registros.
func e1bAbiertos(t *testing.T, root string, n int) []Record {
	t.Helper()
	var recs []Record
	for i := 0; i < n; i++ {
		rec := e1Rec(e1bID(i), "propio")
		e1Escribir(t, root, rec)
		recs = append(recs, rec)
	}
	return recs
}

// e1bIDsDe son los ids de los registros.
func e1bIDsDe(recs []Record) []string {
	var out []string
	for _, r := range recs {
		out = append(out, r.ID)
	}
	return out
}

// e1bCerrarFalla cierra cada registro y exige que Cerrar devuelva el error
// (sin escritura el cierre no quedo).
func e1bCerrarFalla(t *testing.T, ca, root string, recs []Record) {
	t.Helper()
	for _, rec := range recs {
		if err := Cerrar(root, e1Cerrado(rec)); err == nil {
			t.Fatalf("%s: fixture: con .hoom/envelopes sin escritura Cerrar(%s) devuelve el error, no nil", ca, rec.ID)
		}
	}
}

// e1bEsperar espera hasta limite a que ok de true.
func e1bEsperar(limite time.Duration, ok func() bool) bool {
	fin := time.Now().Add(limite)
	for {
		if ok() {
			return true
		}
		if time.Now().After(fin) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// CA-333, CA-202 (hallazgo be5b7a): un error TRANSITORIO al mirar el
// registro durante un reintento no abandona el cierre. Cerrar falla (sin
// escritura) y deja el reintento en segundo plano; enseguida
// .hoom/envelopes queda en 000 (mirar el registro da EACCES, no "no
// existe") durante el primer reintento (a 1 s); despues se devuelven todos
// los permisos. Los reintentos siguen (2 s y 4 s despues) y el cierre llega
// a disco: el registro queda entregable, en ok y con su fin.
func TestHallazgo_be5b7a_CA333_UnErrorTransitorioNoAbandonaElCierre(t *testing.T) {
	const ca = "CA-333 (hallazgo be5b7a, error transitorio al mirar el registro)"
	root := t.TempDir()
	recs := e1bAbiertos(t, root, 1)
	rec := recs[0]
	devolver := e1bSinEscritura(t, root, []string{rec.ID})
	dir := filepath.Join(root, ".hoom", DirName)
	reg := filepath.Join(dir, rec.ID+".json")

	t0 := time.Now()
	e1bCerrarFalla(t, ca, root, recs)
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(reg); err == nil || os.IsNotExist(err) {
		t.Skipf("este sistema no da un error de permiso al mirar un registro en un directorio 000: %v", err)
	}
	// el primer reintento (1 s) mira el registro con el directorio en 000
	time.Sleep(2200*time.Millisecond - time.Since(t0))
	devolver()

	ok := e1bEsperar(12*time.Second, func() bool {
		r, ok := e1EnDisco(root, rec.ID)
		return ok && r.Status == StatusDeliverable && r.Stage == "ok" && !r.EndedAt.IsZero()
	})
	if !ok {
		r, _ := e1EnDisco(root, rec.ID)
		t.Errorf("%s: un error transitorio (EACCES) al mirar el registro no abandona el cierre: %s despues del Cerrar fallido (y 12 s despues de devolver los permisos) el registro sigue %q en %q (quiere %q con ended_at)",
			ca, time.Since(t0).Round(time.Second), r.Status, r.Stage, StatusDeliverable)
	}
}

// CA-333, CA-202 (hallazgo 30eb68): N cierres fallidos de N registros
// distintos no crean N goroutines de reintento: con 10 sobres abiertos y
// .hoom/envelopes sin escritura, despues de los 10 Cerrar que fallan hay a
// lo sumo UNA goroutine mas de reintento del paquete (y el proceso no gano
// mas de 3 goroutines). Y esa una alcanza: al devolver la escritura, los
// diez cierres llegan a disco, y las goroutines de reintento terminan.
func TestHallazgo_30eb68_CA333_NCierresFallidosNoCreanNGoroutines(t *testing.T) {
	const ca = "CA-333 (hallazgo 30eb68, N cierres fallidos)"
	const n = 10
	root := t.TempDir()
	recs := e1bAbiertos(t, root, n)
	devolver := e1bSinEscritura(t, root, e1bIDsDe(recs))

	todas0, paq0 := e1bGoroutinas()
	e1bCerrarFalla(t, ca, root, recs)
	time.Sleep(300 * time.Millisecond)
	todas1, paq1 := e1bGoroutinas()
	t.Logf("%s: goroutines antes %d (del paquete %d), despues de %d cierres fallidos %d (del paquete %d)",
		ca, todas0, paq0, n, todas1, paq1)
	if paq1-paq0 > 1 {
		t.Errorf("%s: %d cierres fallidos crean a lo sumo una goroutine de reintento; crearon %d", ca, n, paq1-paq0)
	}
	if todas1-todas0 > 3 {
		t.Errorf("%s: %d cierres fallidos no crean una goroutine cada uno: el proceso paso de %d a %d goroutines", ca, n, todas0, todas1)
	}

	devolver()
	cerrados := func() int {
		c := 0
		for _, rec := range recs {
			if r, ok := e1EnDisco(root, rec.ID); ok && r.Status == StatusDeliverable && !r.EndedAt.IsZero() {
				c++
			}
		}
		return c
	}
	if !e1bEsperar(15*time.Second, func() bool { return cerrados() == n }) {
		t.Errorf("%s: al devolver la escritura, los %d cierres que fallaron llegan a disco; llegaron %d", ca, n, cerrados())
	}
	if !e1bEsperar(5*time.Second, func() bool { _, p := e1bGoroutinas(); return p <= paq0 }) {
		_, p := e1bGoroutinas()
		t.Errorf("%s: con los cierres en disco, las goroutines de reintento terminan: del paquete habia %d antes y quedan %d", ca, paq0, p)
	}
}

// CA-333, CA-202 (hallazgo 94636a): si despues de un Cerrar fallido el
// registro en disco se reemplaza por un DIRECTORIO (lo puede hacer el rol),
// los reintentos terminan en vez de seguir para siempre: con 5 sobres asi,
// en pocos segundos las goroutines de reintento del paquete vuelven a las
// de antes (y el proceso, a lo sumo 2 mas que antes), y no se escribe nada:
// cada directorio sigue vacio y en .hoom/envelopes no aparece ningun
// archivo.
func TestHallazgo_94636a_CA333_RegistroReemplazadoPorUnDirectorioTerminaLosReintentos(t *testing.T) {
	const ca = "CA-333 (hallazgo 94636a, registro reemplazado por un directorio)"
	const n = 5
	root := t.TempDir()
	recs := e1bAbiertos(t, root, n)
	devolver := e1bSinEscritura(t, root, e1bIDsDe(recs))
	dir := filepath.Join(root, ".hoom", DirName)

	todas0, paq0 := e1bGoroutinas()
	e1bCerrarFalla(t, ca, root, recs)
	devolver()
	for _, rec := range recs {
		reg := filepath.Join(dir, rec.ID+".json")
		if err := os.Remove(reg); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(reg, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	termino := e1bEsperar(10*time.Second, func() bool {
		todas, paq := e1bGoroutinas()
		return paq <= paq0 && todas <= todas0+2
	})
	todas, paq := e1bGoroutinas()
	t.Logf("%s: goroutines antes %d (del paquete %d), ahora %d (del paquete %d)", ca, todas0, paq0, todas, paq)
	if !termino {
		t.Errorf("%s: con los %d registros reemplazados por directorios, los reintentos terminan: 10 s despues las goroutines del paquete pasaron de %d a %d y las del proceso de %d a %d",
			ca, n, paq0, paq, todas0, todas)
	}

	entradas, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entradas {
		if !e.IsDir() {
			t.Errorf("%s: el reintento no escribe nada: en .hoom/envelopes aparecio %s", ca, e.Name())
			continue
		}
		dentro, err := os.ReadDir(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range dentro {
			t.Errorf("%s: el reintento no escribe nada: dentro del directorio %s aparecio %s", ca, e.Name(), d.Name())
		}
	}
	if len(entradas) != n {
		t.Errorf("%s: en .hoom/envelopes quedan solo los %d directorios: hay %d entradas", ca, n, len(entradas))
	}
}
