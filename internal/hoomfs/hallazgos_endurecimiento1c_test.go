// Tests de regresion de la tercera ronda de la tarea endurecimiento-1 en
// hoomfs, sobre .hoom/specs/evidencia-en-disco.md (CA-442) y
// .hoom/specs/historia-doctor-y-cinta.md (CA-369). Escritos desde el texto de
// cada hallazgo y el contrato de la correccion, SIN LEER LA IMPLEMENTACION.
// Los helpers de este archivo llevan el prefijo e1c.
//
// API nueva: hoomfs.RecorrerDirectorio(root, partes, quiere, max, cada)
// reemplaza a hoomfs.LeerDirectorio (que desaparece). Sobre 51d6c22 este
// archivo NO COMPILA: es lo esperado.
//
//   - 20261023T122536_c85840 (high), 20261023T123049_8c7270 y
//     20261023T123540_b7bb41: LeerDirectorio acumulaba en []Entrada el
//     contenido de todos los archivos antes de devolver; con topes de 256 MiB
//     por archivo, N registros validos retenian N x 256 MiB. RecorrerDirectorio
//     entrega las entradas DE A UNA: la funcion cada recibe cada entrada antes
//     de que se lea el archivo siguiente.
//   - 20261023T123057_b36303 (low): LeerDirectorio devolvia el orden del
//     directorio (File.ReadDir), no el de nombre que daba os.ReadDir.
//     RecorrerDirectorio entrega en orden de nombre, determinista.
package hoomfs

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// e1cRecorrido es lo que cada recibio, en orden.
type e1cRecorrido struct {
	nombres    []string
	contenidos map[string]string
	errs       map[string]error
}

// e1cRecorrer corre RecorrerDirectorio sobre root/partes y anota lo que
// entrega. antes, si no es nil, corre dentro de cada al recibir cada entrada
// (despues de anotarla).
func e1cRecorrer(t *testing.T, root string, partes []string, quiere func(string) bool, max int64,
	antes func(Entrada)) (e1cRecorrido, error) {
	t.Helper()
	r := e1cRecorrido{contenidos: map[string]string{}, errs: map[string]error{}}
	err := RecorrerDirectorio(root, partes, quiere, max, func(e Entrada) {
		r.nombres = append(r.nombres, e.Nombre)
		r.contenidos[e.Nombre] = string(e.Contenido)
		r.errs[e.Nombre] = e.Err
		if antes != nil {
			antes(e)
		}
	})
	return r, err
}

func e1cTodos(string) bool { return true }

func e1cJSON(n string) bool { return strings.HasSuffix(n, ".json") }

func e1cEscribir(t *testing.T, p, contenido string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(contenido), 0o644); err != nil {
		t.Fatal(err)
	}
}

// CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41): RecorrerDirectorio
// entrega cada archivo a cada ANTES de leer el siguiente. Con cinco archivos
// (e1c-0.json .. e1c-4.json, en orden de nombre), cada vez que cada recibe
// e1c-<i>.json reescribe en disco e1c-<i+1>.json: la entrada siguiente trae
// ese contenido nuevo, no el que tenia el archivo al empezar. Un lector que
// lee todo y despues llama a cada (lo que hacia LeerDirectorio: retener el
// contenido de todos a la vez) entrega el contenido viejo.
func TestHallazgo_c85840_8c7270_b7bb41_CA442_RecorrerDirectorioEntregaDeAUna(t *testing.T) {
	const ca = "CA-442, CA-369 (hallazgos c85840, 8c7270 y b7bb41, de a una)"
	root := t.TempDir()
	partes := []string{".hoom", "findings"}
	dir := filepath.Join(root, ".hoom", "findings")
	const n = 5
	nombre := func(i int) string { return fmt.Sprintf("e1c-%d.json", i) }
	for i := 0; i < n; i++ {
		e1cEscribir(t, filepath.Join(dir, nombre(i)), fmt.Sprintf(`{"antes":%d}`, i))
	}

	r, err := e1cRecorrer(t, root, partes, e1cJSON, 1<<20, func(e Entrada) {
		var i int
		if _, err := fmt.Sscanf(e.Nombre, "e1c-%d.json", &i); err != nil || i+1 >= n {
			return
		}
		e1cEscribir(t, filepath.Join(dir, nombre(i+1)), fmt.Sprintf(`{"despues-de":%d,"relleno":"%s"}`, i, strings.Repeat("x", i)))
	})
	if err != nil {
		t.Fatalf("%s: RecorrerDirectorio sobre un directorio legible no da error: %v", ca, err)
	}
	if len(r.nombres) != n {
		t.Fatalf("%s: cada recibe los %d archivos, una vez cada uno: %v", ca, n, r.nombres)
	}
	for i := 0; i < n; i++ {
		if err := r.errs[nombre(i)]; err != nil {
			t.Fatalf("%s: %s es un archivo regular legible: Err %v", ca, nombre(i), err)
		}
	}
	if got, want := r.contenidos[nombre(0)], `{"antes":0}`; got != want {
		t.Errorf("%s: el primero trae su contenido: %q, quiere %q", ca, got, want)
	}
	for i := 1; i < n; i++ {
		want := fmt.Sprintf(`{"despues-de":%d,"relleno":"%s"}`, i-1, strings.Repeat("x", i-1))
		if got := r.contenidos[nombre(i)]; got != want {
			t.Errorf("%s: %s se lee DESPUES de que cada recibio %s (entradas de a una, sin retener el contenido de todos a la vez): trajo %q, quiere %q",
				ca, nombre(i), nombre(i-1), got, want)
		}
	}
}

// CA-442 (hallazgo b36303): RecorrerDirectorio entrega en orden de nombre
// (orden de bytes), sin depender del orden en que el sistema de archivos
// lista el directorio. Con 300 archivos de nombres al azar creados en
// desorden (y unos .txt que quiere rechaza), cada recibe exactamente los
// .json, en orden estrictamente creciente. Corrido dos veces, el mismo orden.
func TestHallazgo_b36303_CA442_RecorrerDirectorioEntregaEnOrdenDeNombre(t *testing.T) {
	const ca = "CA-442 (hallazgo b36303, orden de nombre)"
	root := t.TempDir()
	dir := filepath.Join(root, ".hoom", "verdicts")
	rnd := rand.New(rand.NewSource(20261023))
	vistos := map[string]bool{}
	var quiero []string
	for len(quiero) < 300 {
		// nombres de largo y forma distintos; sin dos que solo difieran en
		// mayusculas (APFS y NTFS no distinguen)
		var b strings.Builder
		for j := 0; j < 1+rnd.Intn(12); j++ {
			b.WriteByte("0123456789abcdefghijklmnopqrstuvwxyz-_"[rnd.Intn(38)])
		}
		n := b.String() + ".json"
		if vistos[strings.ToLower(n)] {
			continue
		}
		vistos[strings.ToLower(n)] = true
		quiero = append(quiero, n)
	}
	todos := append([]string{}, quiero...)
	for i := 0; i < 20; i++ {
		todos = append(todos, fmt.Sprintf("Z-otro-%02d.txt", i))
	}
	rnd.Shuffle(len(todos), func(i, j int) { todos[i], todos[j] = todos[j], todos[i] })
	for _, n := range todos {
		e1cEscribir(t, filepath.Join(dir, n), "{}")
	}
	sort.Strings(quiero)

	f, err := os.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	crudo, err := f.Readdirnames(-1)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s: el sistema de archivos lista el directorio en orden de nombre: %v", ca, sort.StringsAreSorted(crudo))

	for vuelta := 1; vuelta <= 2; vuelta++ {
		r, err := e1cRecorrer(t, root, []string{".hoom", "verdicts"}, e1cJSON, 1<<20, nil)
		if err != nil {
			t.Fatalf("%s: RecorrerDirectorio: %v", ca, err)
		}
		if len(r.nombres) != len(quiero) {
			t.Fatalf("%s: vuelta %d: cada recibe los %d .json y ninguno que quiere rechaza: recibio %d", ca, vuelta, len(quiero), len(r.nombres))
		}
		for i, n := range r.nombres {
			if n != quiero[i] {
				t.Fatalf("%s: vuelta %d: las entradas llegan en orden de nombre, no en el del directorio: la %d es %q, quiere %q (primeras recibidas: %v)",
					ca, vuelta, i, n, quiero[i], r.nombres[:min(8, len(r.nombres))])
			}
		}
	}
}

// CA-442, CA-369 (hallazgos c85840 y b36303): RecorrerDirectorio reemplaza a
// LeerDirectorio y conserva su contrato de lectura: un directorio que no
// existe no es error ni entradas; un subdirectorio no se entrega; quiere
// filtra por nombre; un archivo de mas de max bytes llega con su Err y sin
// contenido (nunca cortado); un symlink llega con su Err (no se sigue) y un
// archivo sin permiso de lectura tambien; un directorio que no se puede
// listar es un error.
func TestHallazgo_c85840_b36303_CA442_RecorrerDirectorioConservaElContratoDeLaLectura(t *testing.T) {
	const ca = "CA-442 (hallazgos c85840 y b36303, contrato de RecorrerDirectorio)"
	root := t.TempDir()

	llamadas := 0
	err := RecorrerDirectorio(root, []string{".hoom", "reviews"}, e1cTodos, 1<<20, func(Entrada) { llamadas++ })
	if err != nil || llamadas != 0 {
		t.Errorf("%s: un directorio que no existe no es error ni entradas: err %v, %d entradas", ca, err, llamadas)
	}

	dir := filepath.Join(root, ".hoom", "reviews")
	e1cEscribir(t, filepath.Join(dir, "a-chico.json"), `{"ok":true}`)
	e1cEscribir(t, filepath.Join(dir, "b-grande.json"), strings.Repeat("g", 2048))
	e1cEscribir(t, filepath.Join(dir, "c-otro.txt"), "no lo quiere")
	e1cEscribir(t, filepath.Join(dir, "d-sub.json", "adentro.json"), "{}")
	e1cEscribir(t, filepath.Join(root, "afuera.json"), `{"de":"afuera"}`)
	enlace := runtime.GOOS != "windows"
	if enlace {
		if err := os.Symlink(filepath.Join(root, "afuera.json"), filepath.Join(dir, "e-enlace.json")); err != nil {
			enlace = false
		}
	}

	r, err := e1cRecorrer(t, root, []string{".hoom", "reviews"}, e1cJSON, 1024, nil)
	if err != nil {
		t.Fatalf("%s: RecorrerDirectorio sobre un directorio legible no da error: %v", ca, err)
	}
	want := []string{"a-chico.json", "b-grande.json"}
	if enlace {
		want = append(want, "e-enlace.json")
	}
	if strings.Join(r.nombres, ",") != strings.Join(want, ",") {
		t.Errorf("%s: entrega los archivos que quiere acepta, sin el subdirectorio d-sub.json, en orden de nombre: %v, quiere %v", ca, r.nombres, want)
	}
	if r.errs["a-chico.json"] != nil || r.contenidos["a-chico.json"] != `{"ok":true}` {
		t.Errorf("%s: un archivo regular chico llega entero y sin Err: %q %v", ca, r.contenidos["a-chico.json"], r.errs["a-chico.json"])
	}
	if r.errs["b-grande.json"] == nil || r.contenidos["b-grande.json"] != "" {
		t.Errorf("%s: un archivo de mas de max bytes llega con su Err y sin contenido (nunca cortado): %d bytes, Err %v",
			ca, len(r.contenidos["b-grande.json"]), r.errs["b-grande.json"])
	}
	if enlace && (r.errs["e-enlace.json"] == nil || strings.Contains(r.contenidos["e-enlace.json"], "afuera")) {
		t.Errorf("%s: un symlink no se sigue: llega con su Err y sin el contenido de afuera: %q, Err %v",
			ca, r.contenidos["e-enlace.json"], r.errs["e-enlace.json"])
	}

	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		return
	}
	sinLeer := filepath.Join(dir, "a-chico.json")
	if err := os.Chmod(sinLeer, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sinLeer, 0o644) })
	r, err = e1cRecorrer(t, root, []string{".hoom", "reviews"}, e1cJSON, 1024, nil)
	if err != nil {
		t.Fatalf("%s: un archivo ilegible no hace fallar el recorrido (llega con su Err): %v", ca, err)
	}
	if r.errs["a-chico.json"] == nil {
		t.Errorf("%s: un archivo sin permiso de lectura llega con su Err: %q", ca, r.contenidos["a-chico.json"])
	}
	_ = os.Chmod(sinLeer, 0o644)

	if err := os.Chmod(dir, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	if _, err := os.ReadDir(dir); err == nil {
		return
	}
	llamadas = 0
	err = RecorrerDirectorio(root, []string{".hoom", "reviews"}, e1cTodos, 1024, func(Entrada) { llamadas++ })
	if err == nil {
		t.Errorf("%s: un directorio que existe y no se puede listar es un error, no un recorrido vacio (%d entradas)", ca, llamadas)
	}
}
