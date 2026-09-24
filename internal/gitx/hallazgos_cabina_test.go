// Tests de regresion de la review de la cabina C2 sobre
// .hoom/specs/tablero-de-solo-lectura.md (CA-315): BranchDiff lee el parche
// de git por streaming, y la memoria que usa queda acotada por maxBytes, no
// por el tamano del parche. Lo que no cambia: git diff base...HEAD, solo lo
// commiteado, numstat completo, parche cortado en fin de linea en <= maxBytes
// con Truncated true, y maxBytes <= 0 es sin tope.
package gitx

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

// hcDiffCrudo es la salida exacta (sin recortar) de
// `git diff --no-color --no-ext-diff <base>...HEAD --` en dir.
func hcDiffCrudo(t *testing.T, dir, base string) string {
	t.Helper()
	cmd := exec.Command("git", "diff", "--no-color", "--no-ext-diff", base+"...HEAD", "--")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git diff %s...HEAD: %v", base, err)
	}
	return string(out)
}

// Hallazgo 20260924T051338_395420. CA-315: sobre una rama que agrega un
// archivo de ~16 MiB de lineas, BranchDiff con maxBytes = 256 KiB no se lleva
// el parche entero a memoria: lo que aloja durante la llamada (TotalAlloc)
// queda muy por debajo del parche. Y el resultado sigue siendo el de CA-315:
// disponible y sin nota, truncado, de a lo sumo maxBytes, cortado en el
// ultimo fin de linea que entra y prefijo exacto de git diff base...HEAD, con
// el numstat entero. Un diff chico sin tope (maxBytes <= 0) es identico a la
// salida de git.
func TestHallazgo_395420_BranchDiffNoLeeElParcheEnteroAMemoria(t *testing.T) {
	dir := dfRepo(t)
	const lineasGrandes = 262144 // 64 bytes cada una: 16 MiB
	var b strings.Builder
	b.Grow(lineasGrandes * 64)
	for i := 0; i < lineasGrandes; i++ {
		fmt.Fprintf(&b, "linea %08d %s\n", i, strings.Repeat("g", 48))
	}
	write(t, dir, "grande.txt", b.String())
	b.Reset()
	dfGit(t, dir, "add", "grande.txt")
	dfGit(t, dir, "commit", "-q", "-m", "archivo gigante")

	const max = 256 << 10
	const techo = 4 << 20

	runtime.GC()
	var antes, despues runtime.MemStats
	runtime.ReadMemStats(&antes)
	d, err := BranchDiff(dir, "main", max)
	runtime.ReadMemStats(&despues)
	alojado := despues.TotalAlloc - antes.TotalAlloc

	if err != nil {
		t.Fatalf("395420 CA-315: BranchDiff no devuelve error (si git falla, lo dice la nota): %v", err)
	}
	crudo := hcDiffCrudo(t, dir, "main")
	if len(crudo) < 16<<20 {
		t.Fatalf("fixture: el parche de git es de al menos 16 MiB: %d bytes", len(crudo))
	}

	if !d.Available || d.Note != "" {
		t.Fatalf("395420 CA-315: el diff grande esta disponible y sin nota: available=%v note=%q", d.Available, d.Note)
	}
	if !d.Truncated {
		t.Fatalf("395420 CA-315: un parche de %d bytes con maxBytes %d se corta y lo dice (truncated)", len(crudo), max)
	}
	if len(d.Patch) > max || !strings.HasSuffix(d.Patch, "\n") {
		t.Fatalf("395420 CA-315: el parche cortado no pasa maxBytes y termina en fin de linea: len=%d", len(d.Patch))
	}
	if !strings.HasPrefix(crudo, d.Patch) {
		t.Fatalf("395420 CA-315: el parche cortado es prefijo exacto de git diff main...HEAD (len %d)", len(d.Patch))
	}
	if want := crudo[:strings.LastIndexByte(crudo[:max], '\n')+1]; d.Patch != want {
		t.Fatalf("395420 CA-315: el corte es el ultimo fin de linea antes de maxBytes: len %d, quiere %d", len(d.Patch), len(want))
	}
	porRuta := map[string]DiffFile{}
	for _, f := range d.Files {
		porRuta[f.Path] = f
	}
	if f, ok := porRuta["grande.txt"]; !ok || f.Insertions != lineasGrandes || f.Deletions != 0 {
		t.Fatalf("395420 CA-315: el numstat va entero aunque el parche se corte: grande.txt %+v", d.Files)
	}
	if len(d.Files) != 3 || d.Insertions != lineasGrandes+4 || d.Deletions != 1 {
		t.Fatalf("395420 CA-315: numstat completo (grande.txt, nuevo.go, app.go; %d y 1): %d archivos, %d/%d",
			lineasGrandes+4, len(d.Files), d.Insertions, d.Deletions)
	}

	// lo chico sin tope: identico a la salida de git, byte a byte
	chico := dfRepo(t)
	crudoChico := hcDiffCrudo(t, chico, "main")
	for _, sinTope := range []int{0, -1} {
		dc, err := BranchDiff(chico, "main", sinTope)
		if err != nil || !dc.Available || dc.Truncated || dc.Patch != crudoChico {
			t.Fatalf("395420 CA-315: con maxBytes %d (sin tope) el parche es identico a git diff main...HEAD: err=%v available=%v truncated=%v\nquiere %q\nfue    %q",
				sinTope, err, dc.Available, dc.Truncated, crudoChico, dc.Patch)
		}
	}

	if alojado >= techo {
		t.Fatalf("395420 CA-315: BranchDiff lee el parche de git por streaming: con maxBytes %d KiB y un parche de %d MiB aloja %.1f MiB (techo %d MiB): la memoria la acota maxBytes, no el tamano del parche",
			max>>10, len(crudo)>>20, float64(alojado)/(1<<20), techo>>20)
	}
}
