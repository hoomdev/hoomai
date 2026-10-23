// Tests de regresion de la cuarta ronda de la tarea endurecimiento-1 en
// runcmd (los hallazgos de la delta 2), sobre
// .hoom/specs/evidencia-en-disco.md (CA-442). Escritos desde el texto del
// hallazgo y el contrato de la correccion, SIN LEER LA IMPLEMENTACION. Reusan
// e1cMeta de la tercera ronda; los helpers de este archivo llevan el prefijo
// e1d.
//
//   - 20261023T133203_3cb01b (CA-442): MetasErr volvia a ordenar por fecha
//     con un ordenamiento inestable despues de recibir los nombres en orden:
//     con fechas mezcladas, los empates salian permutados (el test de la
//     ronda 3 usaba 10 elementos con la misma fecha, que no lo expone).
//     Dentro de cada grupo de empate, el orden es el de nombre (creciente o
//     decreciente: "del mas nuevo al mas viejo" no dice como desempatar; el
//     mismo en todos los grupos).
package runcmd

import (
	"fmt"
	"math/rand/v2"
	"sort"
	"strings"
	"testing"
	"time"
)

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

// e1dNombreID es un nombre al azar con la forma de un id de hoom.
func e1dNombreID(r *rand.Rand) string {
	return fmt.Sprintf("2026%02d%02dT%02d%02d%02d_%06x", 1+r.IntN(12), 1+r.IntN(28), r.IntN(24), r.IntN(60), r.IntN(60), r.IntN(1<<24))
}

// CA-442 (hallazgo 3cb01b): 64 sidecars con nombres al azar y CreatedAt
// MEZCLADOS (8 instantes, al azar: grupos de empate intercalados con otras
// fechas) salen de MetasErr del mas nuevo al mas viejo y, dentro de cada
// grupo de empate, en orden de nombre (su id; creciente o decreciente, el
// mismo en todos los grupos). Con mas de 12 elementos un ordenamiento no
// estable permuta los empates aunque reciba los nombres en orden; los datos
// se eligen de forma que lo haga (guarda).
func TestHallazgo_3cb01b_CA442_MetasErrConFechasMezcladasDesempataPorNombre(t *testing.T) {
	const ca = "CA-442 (hallazgo 3cb01b, runcmd.MetasErr con fechas mezcladas)"
	xs := e1dMezcla(t, ca, 64, 8, true, e1dNombreID)
	root := t.TempDir()
	base := time.Date(2026, 10, 23, 12, 0, 0, 0, time.UTC)
	for _, x := range xs {
		e1cMeta(t, root, x.nombre, base.Add(time.Duration(x.fecha)*time.Hour), 0)
	}

	ms, err := MetasErr(root)
	if err != nil || len(ms) != len(xs) {
		t.Fatalf("%s: fixture: MetasErr trae los %d sidecars: %d, err %v", ca, len(xs), len(ms), err)
	}
	var got []string
	for _, m := range ms {
		got = append(got, m.ID)
	}
	creciente, decreciente := e1dEsperado(xs, true, false), e1dEsperado(xs, true, true)
	g := strings.Join(got, ",")
	if g != strings.Join(creciente, ",") && g != strings.Join(decreciente, ",") {
		t.Errorf("%s: con fechas mezcladas, MetasErr ordena del mas nuevo al mas viejo y desempata por nombre: %d de %d posiciones difieren del orden con empates en nombre creciente (%d del decreciente); grupos de empate fuera de orden de nombre:\n%s\n got %v\nquiere %v (o con los empates al reves)",
			ca, e1dPermutados(got, creciente), len(xs), e1dPermutados(got, decreciente), strings.Join(e1dGruposFuera(got, xs, false), "\n"), got, creciente)
	}
}
