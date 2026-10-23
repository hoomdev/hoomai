// Tests de regresion de la tarea endurecimiento-1 en envelope. Escritos desde
// el texto de cada hallazgo, las specs y el contrato que promete la
// correccion, SIN LEER LA IMPLEMENTACION. Los helpers de este archivo llevan
// el prefijo e1.
//
//   - 20261019T153725_3826b3 (risk): List leia .hoom/envelopes/<id>.json
//     siguiendo enlaces simbolicos: un enlace al sobre del mismo id de otro
//     proyecto pasaba la validacion del id y el tablero y la historia
//     publicaban su tarea, provider, nota y gasto. Ni un registro enlazado ni
//     un .hoom/envelopes (o un .hoom) enlazado entran a List; ListErr dice
//     cuando no pudo listar (CA-202, CA-361).
//   - 20261014T204357_de4843 (resilience): el cierre del sobre se escribia
//     con Write, best-effort y sin reintento: si esa escritura fallaba el
//     registro quedaba "en curso" con el PID de un proceso vivo (hoom serve)
//     y la tarjeta quedaba bloqueada. Cerrar devuelve el error y sigue
//     reintentando en segundo plano (1 s, 2 s, 4 s...) hasta que el cierre
//     quede en disco; un registro que ya no existe no se recrea (CA-333,
//     CA-202).
package envelope

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const (
	e1IDPropio = "20261022T100000_e1a001" // un sobre de este proyecto
	e1IDAjeno  = "20261022T100500_e1b002" // un sobre que solo existe en el otro proyecto
)

// e1Rec es un registro abierto de hoom con id, del provider prov.
func e1Rec(id, prov string) Record {
	return Record{ID: id, Role: "writer", Provider: prov, Task: "precios", Dir: "/x", Stage: "run", Step: 3, Steps: 5,
		Status: StatusRunning, ExitCode: -1, StartedAt: time.Date(2026, 10, 22, 10, 0, 0, 0, time.UTC), PID: os.Getpid(),
		Note: "nota de " + prov}
}

// e1Escribir escribe rec con Write y exige que quede.
func e1Escribir(t *testing.T, root string, rec Record) {
	t.Helper()
	if err := Write(root, rec); err != nil {
		t.Fatalf("fixture: Write(%s): %v", rec.ID, err)
	}
}

// e1Enlace crea enlace -> destino; saltea donde no hay enlaces simbolicos.
func e1Enlace(t *testing.T, destino, enlace string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	if err := os.MkdirAll(filepath.Dir(enlace), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, enlace); err != nil {
		t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
	}
}

// e1IDs son los ids (y providers) de los registros.
func e1IDs(rs []Record) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.ID+"("+r.Provider+")")
	}
	return out
}

// e1SinAjeno exige que ni List ni ListErr traigan el sobre del otro proyecto.
func e1SinAjeno(t *testing.T, ca, root, como string) {
	t.Helper()
	for _, r := range List(root) {
		if r.ID == e1IDAjeno || r.Provider == "ajeno" {
			t.Errorf("%s: con %s, List no trae sobres de otro proyecto; trajo %v", ca, como, e1IDs(List(root)))
			break
		}
	}
	rs, _ := ListErr(root)
	for _, r := range rs {
		if r.ID == e1IDAjeno || r.Provider == "ajeno" {
			t.Errorf("%s: con %s, ListErr no trae sobres de otro proyecto; trajo %v", ca, como, e1IDs(rs))
			break
		}
	}
}

// CA-202, CA-361 (hallazgo 3826b3): con .hoom/envelopes real, un
// .hoom/envelopes/<id>.json que es un enlace simbolico a un registro valido
// del MISMO id en otro proyecto (por ruta absoluta o relativa) no aparece en
// List ni en ListErr; el sobre propio, al lado, sigue apareciendo.
func TestHallazgo_3826b3_CA202_RegistroEnlazadoNoSeLista(t *testing.T) {
	const ca = "CA-202 (hallazgo 3826b3, registro enlazado)"
	for _, cs := range []struct{ nombre, destino string }{
		{"enlace absoluto", ""},
		{"enlace relativo", filepath.Join("..", "..", "..", "otro", ".hoom", DirName, e1IDAjeno+".json")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root, otro := filepath.Join(base, "proyecto"), filepath.Join(base, "otro")
			e1Escribir(t, otro, e1Rec(e1IDAjeno, "ajeno"))
			e1Escribir(t, root, e1Rec(e1IDPropio, "propio"))
			destino := cs.destino
			if destino == "" {
				destino = filepath.Join(otro, ".hoom", DirName, e1IDAjeno+".json")
			}
			enlace := filepath.Join(root, ".hoom", DirName, e1IDAjeno+".json")
			e1Enlace(t, destino, enlace)
			if fi, err := os.Stat(enlace); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("fixture: el enlace lleva al registro regular del otro proyecto: %v %v", fi, err)
			}

			e1SinAjeno(t, ca, root, "el registro como enlace al otro proyecto")
			if rs := List(root); len(rs) != 1 || rs[0].ID != e1IDPropio {
				t.Errorf("%s: el sobre propio sigue en List y el enlazado no: %v", ca, e1IDs(rs))
			}
			if rs, err := ListErr(root); len(rs) != 1 || rs[0].ID != e1IDPropio {
				t.Errorf("%s: el sobre propio sigue en ListErr y el enlazado no: %v (err=%v)", ca, e1IDs(rs), err)
			}
		})
	}
}

// CA-202, CA-361 (hallazgo 3826b3): si .hoom/envelopes entero (por ruta
// absoluta o relativa) o .hoom entero es un enlace simbolico al del otro
// proyecto, ninguno de sus sobres aparece en List ni en ListErr, y ListErr
// dice que no pudo listar los sobres de este proyecto (error). Guarda: con
// el directorio real, el sobre propio aparece y ListErr no da error.
func TestHallazgo_3826b3_CA202_DirectorioEnlazadoNoSeLista(t *testing.T) {
	const ca = "CA-202 (hallazgo 3826b3, directorio enlazado)"
	for _, cs := range []struct{ nombre, que, destino string }{
		{".hoom/envelopes enlace absoluto", "envelopes", ""},
		{".hoom/envelopes enlace relativo", "envelopes", filepath.Join("..", "..", "otro", ".hoom", DirName)},
		{".hoom enlace absoluto", "hoom", ""},
		{".hoom enlace relativo", "hoom", filepath.Join("..", "otro", ".hoom")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root, otro := filepath.Join(base, "proyecto"), filepath.Join(base, "otro")
			e1Escribir(t, otro, e1Rec(e1IDAjeno, "ajeno"))
			e1Escribir(t, root, e1Rec(e1IDPropio, "propio"))
			if rs, err := ListErr(root); err != nil || len(rs) != 1 || rs[0].ID != e1IDPropio {
				t.Fatalf("%s: guarda: con el directorio real, ListErr trae el sobre propio sin error: %v %v", ca, e1IDs(rs), err)
			}

			enlace, destino := filepath.Join(root, ".hoom", DirName), filepath.Join(otro, ".hoom", DirName)
			if cs.que == "hoom" {
				enlace, destino = filepath.Join(root, ".hoom"), filepath.Join(otro, ".hoom")
			}
			if err := os.RemoveAll(enlace); err != nil {
				t.Fatal(err)
			}
			if cs.destino != "" {
				destino = cs.destino
			}
			e1Enlace(t, destino, enlace)
			if fi, err := os.Stat(filepath.Join(root, ".hoom", DirName, e1IDAjeno+".json")); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("fixture: a traves del enlace, el registro del otro proyecto es un archivo regular: %v %v", fi, err)
			}

			e1SinAjeno(t, ca, root, cs.nombre)
			if _, err := ListErr(root); err == nil {
				t.Errorf("%s: con %s, ListErr dice que no pudo listar los sobres de este proyecto (error), no una lista", ca, cs.nombre)
			}
		})
	}
}

// CA-202 (hallazgo 3826b3, API nueva ListErr): sin .hoom/envelopes, ListErr
// no da nada y ningun error (como List: un proyecto que nunca corrio un sobre
// no es un problema); con .hoom/envelopes que existe y no se puede listar
// (000), da un error en vez de una lista vacia que calla lo que no vio.
func TestHallazgo_3826b3_CA202_ListErrDistingueNadaDeIlegible(t *testing.T) {
	const ca = "CA-202 (hallazgo 3826b3, ListErr)"
	root := t.TempDir()
	if rs, err := ListErr(root); err != nil || len(rs) != 0 {
		t.Fatalf("%s: sin .hoom/envelopes, ListErr no da nada y ningun error: %v %v", ca, e1IDs(rs), err)
	}
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("sin permisos de unix (o como root) un chmod no quita la lectura")
	}
	e1Escribir(t, root, e1Rec(e1IDPropio, "propio"))
	dir := filepath.Join(root, ".hoom", DirName)
	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		t.Skip("este sistema deja listar un directorio sin permiso de lectura")
	}
	if rs, err := ListErr(root); err == nil {
		t.Errorf("%s: con .hoom/envelopes ilegible, ListErr da un error y no una lista (%v) que calla lo que no pudo ver", ca, e1IDs(rs))
	}
}

// ---------------------------------------------------------------- de4843

// e1SinEscritura deja el registro y .hoom/envelopes sin escritura (ni en el
// lugar ni con un archivo nuevo al lado) y devuelve la funcion que los
// devuelve a como estaban. Saltea donde los permisos no frenan la escritura.
func e1SinEscritura(t *testing.T, root, id string) func() {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("sin permisos de unix (o como root) un chmod no quita la escritura")
	}
	dir := filepath.Join(root, ".hoom", DirName)
	reg := filepath.Join(dir, id+".json")
	if err := os.Chmod(reg, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	devolver := func() {
		_ = os.Chmod(dir, 0o755)
		_ = os.Chmod(reg, 0o644)
	}
	t.Cleanup(devolver)
	if f, err := os.CreateTemp(dir, "sonda"); err == nil {
		f.Close()
		os.Remove(f.Name())
		t.Skip("este sistema deja escribir en un directorio sin permiso de escritura")
	}
	return devolver
}

// e1Cerrado es rec cerrado: entregable, en ok, con su fin.
func e1Cerrado(rec Record) Record {
	rec.Status, rec.Stage, rec.Step, rec.ExitCode = StatusDeliverable, "ok", 5, 0
	rec.EndedAt = time.Now().UTC()
	return rec
}

// e1EnDisco es el registro id tal como esta en disco (List), o false.
func e1EnDisco(root, id string) (Record, bool) {
	for _, r := range List(root) {
		if r.ID == id {
			return r, true
		}
	}
	return Record{}, false
}

// CA-333, CA-202 (hallazgo de4843): el cierre del sobre se escribe con
// Cerrar. Con el registro abierto en disco y .hoom/envelopes sin escritura,
// Cerrar devuelve el error (el cierre no quedo) y sigue reintentando en
// segundo plano: apenas se puede escribir otra vez, en pocos segundos (el
// primer reintento es a 1 s, el segundo 2 s despues) el registro en disco
// es el cerrado, con su estado y su fin. Sin esto el registro quedaba "en
// curso" con el PID de un proceso vivo y la tarjeta, bloqueada. Guarda: con
// todo escribible, Cerrar da nil y el cierre ya esta en disco.
func TestHallazgo_de4843_CA333_CerrarReintentaHastaQueElCierreQuedaEnDisco(t *testing.T) {
	const ca = "CA-333 (hallazgo de4843)"
	t.Run("guarda: escribible", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := e1Rec(e1IDPropio, "propio")
		e1Escribir(t, root, rec)
		if err := Cerrar(root, e1Cerrado(rec)); err != nil {
			t.Fatalf("%s: con todo escribible, Cerrar da nil: %v", ca, err)
		}
		if r, ok := e1EnDisco(root, rec.ID); !ok || r.Status != StatusDeliverable || r.EndedAt.IsZero() {
			t.Fatalf("%s: con todo escribible, el cierre ya esta en disco cuando Cerrar vuelve: %+v %v", ca, r, ok)
		}
	})
	t.Run("sin escritura y despues con escritura", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		rec := e1Rec(e1IDPropio, "propio")
		e1Escribir(t, root, rec)
		devolver := e1SinEscritura(t, root, rec.ID)
		cerrado := e1Cerrado(rec)
		if err := Cerrar(root, cerrado); err == nil {
			t.Fatalf("%s: con .hoom/envelopes sin escritura el cierre no quedo, y Cerrar devuelve el error, no nil", ca)
		}
		if r, ok := e1EnDisco(root, rec.ID); !ok || r.Status != StatusRunning {
			t.Fatalf("fixture: sin escritura, el registro en disco sigue abierto: %+v %v", r, ok)
		}
		devolver()
		limite := time.Now().Add(6 * time.Second)
		for {
			r, ok := e1EnDisco(root, rec.ID)
			if ok && r.Status == StatusDeliverable && !r.EndedAt.IsZero() && r.Stage == "ok" {
				return
			}
			if time.Now().After(limite) {
				t.Fatalf("%s: Cerrar que fallo sigue reintentando en segundo plano: 6 s despues de que se puede escribir otra vez, el registro en disco sigue %q en %q (quiere %q con ended_at)",
					ca, r.Status, r.Stage, StatusDeliverable)
			}
			time.Sleep(50 * time.Millisecond)
		}
	})
}

// CA-333, CA-202 (hallazgo de4843): el reintento en segundo plano cierra un
// registro que existe; uno que ya no existe en disco no se recrea. Cerrar
// falla (sin escritura), se devuelve la escritura, y antes del primer
// reintento se borra el registro (o el proyecto entero): 3.5 s despues (dos
// reintentos) ni el registro ni el proyecto volvieron a aparecer.
func TestHallazgo_de4843_CA333_ElReintentoNoRecreaLoQueSeBorro(t *testing.T) {
	const ca = "CA-333 (hallazgo de4843, no recrea)"
	for _, cs := range []struct {
		nombre   string
		proyecto bool
	}{
		{"se borra el registro", false},
		{"se borra el proyecto", true},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(t.TempDir(), "proyecto")
			rec := e1Rec(e1IDPropio, "propio")
			e1Escribir(t, root, rec)
			devolver := e1SinEscritura(t, root, rec.ID)
			if err := Cerrar(root, e1Cerrado(rec)); err == nil {
				t.Fatalf("%s: con .hoom/envelopes sin escritura Cerrar devuelve el error, no nil", ca)
			}
			devolver()
			borrado := filepath.Join(root, ".hoom", DirName, rec.ID+".json")
			if cs.proyecto {
				borrado = root
			}
			if err := os.RemoveAll(borrado); err != nil {
				t.Fatal(err)
			}
			time.Sleep(3500 * time.Millisecond)
			if _, err := os.Lstat(borrado); err == nil {
				t.Errorf("%s: el reintento de un cierre no recrea lo que ya no existe en disco: %s volvio a aparecer", ca, strings.TrimPrefix(borrado, filepath.Dir(root)+"/"))
			}
		})
	}
}
