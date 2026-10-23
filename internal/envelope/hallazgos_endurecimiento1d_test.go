// Tests de regresion de la cuarta ronda de la tarea endurecimiento-1 en
// envelope (los hallazgos de la delta 2). Escritos desde el texto de cada
// hallazgo y el contrato de la correccion, SIN LEER LA IMPLEMENTACION. Reusan
// los helpers e1*, e1b* y e1c* de las rondas anteriores; los de este archivo
// llevan el prefijo e1d.
//
//   - 20261023T133139_92fa84 y 20261023T132848_8bc636 (CA-333): cada ruta
//     que pasaba por Cerrar dejaba un *sync.Mutex en un mapa global del
//     proceso que nadie borraba, aunque la escritura tuviera exito: la
//     memoria del Studio crecia con el total historico de sobres. Despues de
//     N cierres exitosos de registros distintos, el estado del proceso no
//     crece con N.
//   - 20261023T133203_3cb01b (CA-442): ListErr volvia a ordenar por fecha con
//     un ordenamiento inestable despues de recibir los nombres en orden: con
//     fechas mezcladas, los empates salian permutados. Dentro de cada grupo
//     de empate, el orden es el de nombre.
//
// Los casos de 4647ba, 22806c y da912f se arman con un FIFO y estan en
// hallazgos_endurecimiento1d_unix_test.go.
package envelope

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

const (
	// e1dN cierres exitosos de registros distintos, de a e1dHilos a la vez
	// (cada hilo en su proyecto).
	e1dN     = 10000
	e1dHilos = 8
)

// e1dID es el id del i-esimo sobre de la prueba de memoria: unico, con la
// forma de un id.
func e1dID(i int) string { return fmt.Sprintf("20261023T%06d_%06x", i%240000, i) }

// e1dVivo es la memoria viva del heap despues de un GC completo.
func e1dVivo() uint64 {
	runtime.GC()
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// e1dCerrarTodos abre (Write) y cierra (Cerrar) los sobres desde..hasta-1,
// repartidos en e1dHilos proyectos bajo base; cada Cerrar tiene que dar nil.
func e1dCerrarTodos(t *testing.T, ca, base string, desde, hasta int) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make(chan error, e1dHilos)
	for h := 0; h < e1dHilos; h++ {
		wg.Add(1)
		go func(h int) {
			defer wg.Done()
			root := filepath.Join(base, fmt.Sprintf("p%d", h))
			for i := desde + h; i < hasta; i += e1dHilos {
				rec := e1Rec(e1dID(i), "propio")
				if err := Write(root, rec); err != nil {
					errs <- fmt.Errorf("Write(%s): %w", rec.ID, err)
					return
				}
				if err := Cerrar(root, e1Cerrado(rec)); err != nil {
					errs <- fmt.Errorf("Cerrar(%s): %w", rec.ID, err)
					return
				}
			}
		}(h)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("%s: fixture: con todo escribible, abrir y cerrar da nil: %v", ca, err)
	}
}

// e1dRetencion es lo que crece la memoria viva al retener una entrada por
// cada sobre desde..hasta-1: un mapa de su ruta a un *sync.Mutex, la forma
// que describe 8bc636. Es la vara del medidor.
func e1dRetencion(base string, desde, hasta int) int64 {
	antes := e1dVivo()
	m := map[string]*sync.Mutex{}
	for i := desde; i < hasta; i++ {
		root := filepath.Join(base, fmt.Sprintf("p%d", i%e1dHilos))
		m[filepath.Join(root, ".hoom", DirName, e1dID(i)+".json")] = &sync.Mutex{}
	}
	despues := e1dVivo()
	runtime.KeepAlive(m)
	return int64(despues) - int64(antes)
}

// CA-333 (hallazgos 92fa84 y 8bc636): despues de 10000 cierres EXITOSOS de
// registros distintos (cada sobre se abre con Write y se cierra con Cerrar,
// de a 8 a la vez), la memoria viva del proceso no crece con N: crece menos
// de un cuarto de lo que ocupa retener una entrada por registro cerrado (un
// mapa de su ruta a un *sync.Mutex, medido igual, ~2 MB). Antes se mide con
// 400 cierres de calentamiento. Con el error, cada cierre deja su entrada y
// la memoria crece lo mismo que ese mapa. Guarda: el medidor ve esa
// retencion (por lo menos 512 KiB).
func TestHallazgo_92fa84_8bc636_CA333_LosCierresExitososNoDejanEstadoQueCrece(t *testing.T) {
	const ca = "CA-333 (hallazgos 92fa84 y 8bc636, estado por cierre exitoso)"
	const calentar = 400
	base := t.TempDir()
	e1dCerrarTodos(t, ca, base, 0, calentar)
	antes := e1dVivo()
	t0 := time.Now()
	e1dCerrarTodos(t, ca, base, calentar, calentar+e1dN)
	dur := time.Since(t0)
	despues := e1dVivo()
	crecio := int64(despues) - int64(antes)

	retencion := e1dRetencion(base, calentar, calentar+e1dN)
	t.Logf("%s: %d cierres exitosos en %s: la memoria viva paso de %d a %d bytes (%+d, %.1f por cierre); retener una entrada por registro ocupa %d bytes (%.1f por registro)",
		ca, e1dN, dur.Round(time.Millisecond), antes, despues, crecio, float64(crecio)/e1dN, retencion, float64(retencion)/e1dN)
	if retencion < 512<<10 {
		t.Fatalf("%s: fixture: el medidor no ve la retencion de una entrada por registro: %d entradas solo subieron la memoria viva %d bytes", ca, e1dN, retencion)
	}
	if tope := retencion / 4; crecio > tope {
		t.Errorf("%s: despues de %d cierres exitosos de registros distintos, el estado del proceso no crece con N: la memoria viva crecio %d bytes (%.1f por cierre), mas que el tope de %d (un cuarto de retener una entrada por registro cerrado, %d)",
			ca, e1dN, crecio, float64(crecio)/e1dN, tope, retencion)
	}
}

// ---------------------------------------------------------------- 3cb01b

// e1dElem es un elemento de la prueba de orden: su nombre y el indice de su
// fecha.
type e1dElem struct {
	nombre string
	fecha  int
}

// e1dMezcla da n nombres al azar (unicos, con la forma que da forma) en orden
// de nombre, y a cada uno una de f fechas (indices 0..f-1) tambien al azar:
// grupos de empate intercalados con otras fechas. Busca, de forma
// determinista, la primera semilla con la que un ordenamiento no estable de
// la biblioteca (sort.Slice) por fecha, partiendo del orden de nombre,
// permuta algun empate: asi el caso distingue estable de inestable.
func e1dMezcla(t *testing.T, ca string, n, f int, masNuevoPrimero bool, forma func(r *rand.Rand) string) []e1dElem {
	t.Helper()
	for semilla := uint64(1); semilla <= 64; semilla++ {
		r := rand.New(rand.NewPCG(semilla, 0x3cb01b))
		vistos := map[string]bool{}
		var xs []e1dElem
		for len(xs) < n {
			nom := forma(r)
			if vistos[nom] {
				continue
			}
			vistos[nom] = true
			xs = append(xs, e1dElem{nom, r.IntN(f)})
		}
		sort.Slice(xs, func(i, j int) bool { return xs[i].nombre < xs[j].nombre })
		inestable := append([]e1dElem{}, xs...)
		sort.Slice(inestable, func(i, j int) bool { return e1dAntes(inestable[i], inestable[j], masNuevoPrimero) })
		if strings.Join(e1dNombres(inestable), ",") != strings.Join(e1dEsperado(xs, masNuevoPrimero, false), ",") {
			return xs
		}
	}
	t.Fatalf("%s: fixture: en 64 semillas, un ordenamiento no estable no permuto ningun empate: el caso no distingue", ca)
	return nil
}

// e1dAntes dice si a va antes que b por fecha.
func e1dAntes(a, b e1dElem, masNuevoPrimero bool) bool {
	if masNuevoPrimero {
		return a.fecha > b.fecha
	}
	return a.fecha < b.fecha
}

// e1dNombres son los nombres de xs.
func e1dNombres(xs []e1dElem) []string {
	out := []string{}
	for _, x := range xs {
		out = append(out, x.nombre)
	}
	return out
}

// e1dEsperado es el orden por fecha con los empates en orden de nombre
// (creciente, o decreciente si alReves).
func e1dEsperado(xs []e1dElem, masNuevoPrimero, alReves bool) []string {
	ys := append([]e1dElem{}, xs...)
	sort.SliceStable(ys, func(i, j int) bool {
		if ys[i].fecha != ys[j].fecha {
			return e1dAntes(ys[i], ys[j], masNuevoPrimero)
		}
		if alReves {
			return ys[i].nombre > ys[j].nombre
		}
		return ys[i].nombre < ys[j].nombre
	})
	return e1dNombres(ys)
}

// e1dPermutados cuenta las posiciones en que got difiere de quiero.
func e1dPermutados(got, quiero []string) int {
	n := 0
	for i := range quiero {
		if i >= len(got) || got[i] != quiero[i] {
			n++
		}
	}
	return n
}

// e1dGruposFuera describe los grupos de empate de got (por fecha) cuyos
// nombres no estan en orden de nombre: creciente, o decreciente si alReves.
func e1dGruposFuera(got []string, xs []e1dElem, alReves bool) []string {
	fecha := map[string]int{}
	for _, x := range xs {
		fecha[x.nombre] = x.fecha
	}
	grupos := map[int][]string{}
	var orden []int
	for _, g := range got {
		f := fecha[g]
		if _, ok := grupos[f]; !ok {
			orden = append(orden, f)
		}
		grupos[f] = append(grupos[f], g)
	}
	var fuera []string
	for _, f := range orden {
		ns := grupos[f]
		ok := sort.SliceIsSorted(ns, func(i, j int) bool {
			if alReves {
				return ns[i] > ns[j]
			}
			return ns[i] < ns[j]
		})
		if !ok {
			fuera = append(fuera, fmt.Sprintf("fecha %d: %v", f, ns))
		}
	}
	return fuera
}

// CA-442 (hallazgo 3cb01b): 64 registros de sobre con nombres al azar y
// fechas MEZCLADAS (8 instantes, al azar: grupos de empate intercalados con
// otras fechas) salen de ListErr del mas nuevo al mas viejo y, dentro de cada
// grupo de empate, en orden de nombre (su id; creciente o decreciente, el
// mismo en todos los grupos). Con mas de 12 elementos un ordenamiento no
// estable permuta los empates aunque reciba los nombres en orden; los datos
// se eligen de forma que lo haga (guarda).
func TestHallazgo_3cb01b_CA442_ListErrConFechasMezcladasDesempataPorNombre(t *testing.T) {
	const ca = "CA-442 (hallazgo 3cb01b, envelope.ListErr con fechas mezcladas)"
	xs := e1dMezcla(t, ca, 64, 8, true, func(r *rand.Rand) string {
		return fmt.Sprintf("2026%02d%02dT%02d%02d%02d_%06x", 1+r.IntN(12), 1+r.IntN(28), r.IntN(24), r.IntN(60), r.IntN(60), r.IntN(1<<24))
	})
	root := t.TempDir()
	dir := filepath.Join(root, ".hoom", DirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, x := range xs {
		rec := e1Rec(x.nombre, "propio")
		rec.StartedAt = base.Add(time.Duration(x.fecha) * time.Hour)
		rec.UpdatedAt = rec.StartedAt
		raw, err := json.MarshalIndent(rec, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, x.nombre+".json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rs, err := ListErr(root)
	if err != nil || len(rs) != len(xs) {
		t.Fatalf("%s: fixture: ListErr trae los %d registros: %d, err %v", ca, len(xs), len(rs), err)
	}
	var got []string
	for _, r := range rs {
		got = append(got, r.ID)
	}
	creciente, decreciente := e1dEsperado(xs, true, false), e1dEsperado(xs, true, true)
	g := strings.Join(got, ",")
	if g != strings.Join(creciente, ",") && g != strings.Join(decreciente, ",") {
		t.Errorf("%s: con fechas mezcladas, ListErr ordena del mas nuevo al mas viejo y desempata por nombre: %d de %d posiciones difieren del orden con empates en nombre creciente (%d del decreciente); grupos de empate fuera de orden de nombre:\n%s\n got %v\nquiere %v (o con los empates al reves)",
			ca, e1dPermutados(got, creciente), len(xs), e1dPermutados(got, decreciente), strings.Join(e1dGruposFuera(got, xs, false), "\n"), got, creciente)
	}
}
