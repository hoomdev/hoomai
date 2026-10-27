// Tests de regresion de la tarea endurecimiento-2 en envelope. Escritos desde
// el texto de cada hallazgo y el contrato de la correccion, SIN LEER LA
// IMPLEMENTACION. Reusan los helpers e1* (endurecimiento-1) y hdPlantar
// (hallazgos_c4_doctor_test.go); los de este archivo llevan el prefijo e2.
//
//   - 20261024T141937_26eef9 (high, CA-442, d0d331): ListErr descartaba sin
//     error un archivo con nombre de registro de hoom (<id>.json) que no era
//     un registro (JSON roto o truncado, sin id), y el doctor certificaba
//     "sin problemas" sin haberlo leido. Ahora es un error, y lo que se pudo
//     leer se sigue devolviendo. Un registro ENTERO cuyo id no es el de su
//     nombre se sigue salteando sin error (f34bc3), y un nombre que no es id
//     de hoom se ignora.
//   - 20261024T141937_28f494 (high, CA-333): el reintento de un cierre
//     pendiente miraba solo la entrada final y escribia por ruta: si .hoom o
//     .hoom/envelopes pasaba a ser un enlace a otro proyecto con un <id>.json
//     regular, el reintento reemplazaba el registro de ese otro proyecto.
//     Ahora el reintento no escribe fuera del proyecto.
//
// Los casos de 332b11 (el gancho antesDeEscribir) y de c6c300 (un FIFO) estan
// en hallazgos_endurecimiento2_gancho_test.go y
// hallazgos_endurecimiento2_unix_test.go.
package envelope

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const (
	e2Legible = "20261024T110000_a1b2c3" // un sobre de hoom, legible
	e2Roto    = "20261024T120000_deadbe" // el nombre del registro que no es un registro
	// e2NoID no es un id de sobre de hoom (tiene un espacio). Ojo: la regla
	// de ids de hoom acepta nombres cortos como "notas" o "x"; por eso un
	// "notas.json" SI tiene nombre de registro de hoom.
	e2NoID = "notas sueltas"
)

// e2Archivo escribe .hoom/envelopes/<nombre> con contenido tal cual.
func e2Archivo(t *testing.T, root, nombre string, contenido []byte) {
	t.Helper()
	p := filepath.Join(root, ".hoom", DirName, nombre)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, contenido, 0o644); err != nil {
		t.Fatal(err)
	}
}

// e2RegistroJSON es el JSON de un registro de sobre de hoom con ese id.
func e2RegistroJSON(t *testing.T, id string) []byte {
	t.Helper()
	raw, err := json.MarshalIndent(e1Rec(id, "propio"), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// CA-442 (hallazgo 26eef9): con un registro legible y, al lado,
// .hoom/envelopes/<id>.json con nombre de registro de hoom que NO es un
// registro (no es JSON, esta truncado, esta vacio, es un JSON que no es un
// objeto, no tiene id o tiene el id vacio), ListErr devuelve error (no pudo
// leer todos los sobres: el doctor no puede certificar que no hay huerfanos)
// y, aun asi, devuelve el registro legible. List (best-effort) sigue sin
// error y trae el legible.
func TestHallazgo_26eef9_CA442_UnArchivoDeHoomQueNoEsUnRegistroEsUnErrorDeListErr(t *testing.T) {
	const ca = "CA-442 (hallazgo 26eef9, archivo de hoom que no es un registro)"
	entero := e2RegistroJSON(t, e2Roto)
	var sinID map[string]any
	if err := json.Unmarshal(entero, &sinID); err != nil {
		t.Fatal(err)
	}
	delete(sinID, "id")
	rawSinID, err := json.Marshal(sinID)
	if err != nil {
		t.Fatal(err)
	}
	for _, cs := range []struct {
		nombre    string
		contenido []byte
	}{
		{"no es JSON", []byte("{esto no es json")},
		{"JSON truncado", entero[:len(entero)/2]},
		{"vacio", []byte{}},
		{"JSON que no es un objeto", []byte(`["no", "es", "un", "registro"]`)},
		{"sin id", rawSinID},
		{"id vacio", e2RegistroJSON(t, "")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			root := t.TempDir()
			e1Escribir(t, root, e1Rec(e2Legible, "propio"))
			if rs, err := ListErr(root); err != nil || len(rs) != 1 {
				t.Fatalf("%s: guarda: con un solo registro legible, ListErr lo trae sin error: %v %v", ca, e1IDs(rs), err)
			}
			e2Archivo(t, root, e2Roto+".json", cs.contenido)

			rs, err := ListErr(root)
			if err == nil {
				t.Errorf("%s: .hoom/envelopes/%s.json (%s) tiene nombre de registro de hoom y no es un registro: ListErr devuelve error, no %v sin error",
					ca, e2Roto, cs.nombre, e1IDs(rs))
			}
			if len(rs) != 1 || rs[0].ID != e2Legible {
				t.Errorf("%s: con %s al lado, ListErr devuelve igual el registro legible (%s) y solo ese: %v", ca, cs.nombre, e2Legible, e1IDs(rs))
			}
			if ls := List(root); len(ls) != 1 || ls[0].ID != e2Legible {
				t.Errorf("%s: con %s al lado, List (best-effort) trae el legible y solo ese: %v", ca, cs.nombre, e1IDs(ls))
			}
		})
	}
}

// CA-442, CA-369 (hallazgo 26eef9, guardas de f34bc3 y del contrato de
// siempre): no son un error de ListErr, y no aparecen: un registro ENTERO
// cuyo id no es el de su nombre (con forma de id o sin ella), un archivo
// cuyo nombre no es id de hoom (aunque no sea JSON), uno de otra extension y
// el temporal que deja una escritura a medias. El registro legible sigue
// saliendo.
func TestHallazgo_26eef9_CA442_LoQueNoEsDeHoomSeSalteaSinError(t *testing.T) {
	const ca = "CA-442 (hallazgo 26eef9, lo que no es de hoom)"
	// guarda: e2NoID no es un id de sobre de hoom: un registro entero con
	// ese id y ese nombre no sale de ListErr
	guarda := t.TempDir()
	hdPlantar(t, guarda, e2NoID+".json", e1Rec(e2NoID, "propio"))
	if rs, err := ListErr(guarda); err != nil || len(rs) != 0 {
		t.Fatalf("%s: fixture: %q no es un id de sobre de hoom: un registro entero con ese id no sale de ListErr: %v %v", ca, e2NoID, e1IDs(rs), err)
	}
	for _, cs := range []struct {
		nombre, archivo string
		contenido       []byte
	}{
		{"registro entero de otro id", e2Roto + ".json", e2RegistroJSON(t, "20261024T120002_otro01")},
		{"registro entero con id sin forma de id", e2Roto + ".json", e2RegistroJSON(t, "../../VICTIMA")},
		{"nombre que no es id de hoom (con un espacio), no JSON", e2NoID + ".json", []byte("{no es json")},
		{"otra extension", e2Roto + ".json.bak", []byte("{no es json")},
		{"temporal de una escritura", "." + e2Roto + ".json.tmp-123", []byte("{no es")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			root := t.TempDir()
			e1Escribir(t, root, e1Rec(e2Legible, "propio"))
			e2Archivo(t, root, cs.archivo, cs.contenido)
			rs, err := ListErr(root)
			if err != nil {
				t.Errorf("%s: .hoom/envelopes/%s (%s) no es de hoom: ListErr no da error por el: %v", ca, cs.archivo, cs.nombre, err)
			}
			if len(rs) != 1 || rs[0].ID != e2Legible {
				t.Errorf("%s: con %s al lado, ListErr trae el registro legible y solo ese: %v", ca, cs.nombre, e1IDs(rs))
			}
		})
	}
}

// ---------------------------------------------------------------- 28f494

// CA-333 (hallazgo 28f494): el reintento de un cierre pendiente no escribe
// fuera del proyecto. Cerrar falla (sin escritura) y queda pendiente; antes
// del primer reintento (a 1 s), .hoom/envelopes (o .hoom entero) pasa a ser
// un enlace simbolico, absoluto o relativo, al del otro proyecto, que tiene
// un <id>.json regular del mismo id; y vuelve la escritura. Durante 4.5 s
// (los reintentos de 1 s y 3 s) el registro del otro proyecto queda intacto,
// byte a byte, y en su .hoom/envelopes no aparece nada nuevo.
func TestHallazgo_28f494_CA333_ElReintentoNoEscribeFueraDelProyecto(t *testing.T) {
	const ca = "CA-333 (hallazgo 28f494, reintento contra un enlace a otro proyecto)"
	for _, cs := range []struct{ nombre, que, relativo string }{
		{".hoom/envelopes, enlace absoluto", "envelopes", ""},
		{".hoom/envelopes, enlace relativo", "envelopes", filepath.Join("..", "..", "otro", ".hoom", DirName)},
		{".hoom, enlace absoluto", "hoom", ""},
		{".hoom, enlace relativo", "hoom", filepath.Join("..", "otro", ".hoom")},
	} {
		t.Run(cs.nombre, func(t *testing.T) {
			t.Parallel()
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			root, otro := filepath.Join(base, "proyecto"), filepath.Join(base, "otro")
			rec := e1Rec(e1IDPropio, "propio")
			e1Escribir(t, root, rec)
			ajeno := e1Rec(e1IDPropio, "ajeno")
			ajeno.Note = "registro del otro proyecto: el reintento no lo toca"
			e1Escribir(t, otro, ajeno)
			regAjeno := filepath.Join(otro, ".hoom", DirName, rec.ID+".json")
			antes, err := os.ReadFile(regAjeno)
			if err != nil {
				t.Fatal(err)
			}

			devolver := e1SinEscritura(t, root, rec.ID)
			if err := Cerrar(root, e1Cerrado(rec)); err == nil {
				t.Fatalf("%s: fixture: con .hoom/envelopes sin escritura Cerrar devuelve el error, no nil", ca)
			}
			t0 := time.Now()
			devolver()
			enlace, destino := filepath.Join(root, ".hoom", DirName), filepath.Join(otro, ".hoom", DirName)
			if cs.que == "hoom" {
				enlace, destino = filepath.Join(root, ".hoom"), filepath.Join(otro, ".hoom")
			}
			if err := os.Rename(enlace, enlace+".original"); err != nil {
				t.Fatal(err)
			}
			if cs.relativo != "" {
				destino = cs.relativo
			}
			e1Enlace(t, destino, enlace)
			if fi, err := os.Stat(filepath.Join(root, ".hoom", DirName, rec.ID+".json")); err != nil || !fi.Mode().IsRegular() {
				t.Fatalf("%s: fixture: a traves del enlace, %s.json es el registro regular del otro proyecto: %v %v", ca, rec.ID, fi, err)
			}
			if d := time.Since(t0); d > 800*time.Millisecond {
				t.Fatalf("%s: fixture: el enlace tiene que estar antes del primer reintento (1 s); tardo %s", ca, d)
			}

			for time.Since(t0) < 4500*time.Millisecond {
				if ahora, err := os.ReadFile(regAjeno); err != nil || !bytes.Equal(ahora, antes) {
					r, _ := e1EnDisco(otro, rec.ID)
					t.Fatalf("%s: %s despues de volver la escritura, el registro del otro proyecto cambio (el reintento del cierre pendiente escribio a traves del enlace, fuera del proyecto): ahora %q de %q (err %v)",
						ca, time.Since(t0).Round(10*time.Millisecond), r.Status, r.Provider, err)
				}
				time.Sleep(20 * time.Millisecond)
			}
			entradas, err := os.ReadDir(filepath.Join(otro, ".hoom", DirName))
			if err != nil {
				t.Fatal(err)
			}
			for _, e := range entradas {
				if e.Name() != rec.ID+".json" {
					t.Errorf("%s: en .hoom/envelopes del otro proyecto no aparece nada nuevo: aparecio %s", ca, e.Name())
				}
			}
		})
	}
}
