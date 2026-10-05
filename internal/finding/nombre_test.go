// Tests de contrato de finding.EsNombre para el hallazgo e44cca de la
// segunda ronda de review de .hoom/specs/evidencia-en-disco.md (CA-442: un
// archivo creado bajo .hoom/findings/ es legitimo solo con la forma que
// escribe hoom, <AAAAMMDDTHHMMSS>_<6 hex>.json o .res.json): una sola
// definicion de los nombres. Los nombres que Add y Resolve escriben cumplen
// EsNombre; un nombre con la forma pero con una fecha que no existe no es un
// nombre que hoom escribe. Escritos desde el contrato, sin leer la
// implementacion.
package finding

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fnArchivos devuelve los nombres de .hoom/findings/ de root.
func fnArchivos(t *testing.T, root string) map[string]bool {
	t.Helper()
	es, err := os.ReadDir(filepath.Join(root, ".hoom", "findings"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, e := range es {
		out[e.Name()] = true
	}
	return out
}

// CA-442 (hallazgo e44cca): para varios hallazgos de Add (y de Register, con
// tarea), su id + ".json" cumple EsNombre y es el archivo que quedo en
// .hoom/findings/; y el archivo de la resolucion que escribe Resolve
// (refutado y corregido) tambien cumple EsNombre.
func TestCA442_LosNombresQueEscribenAddYResolveCumplenEsNombre(t *testing.T) {
	qcFueraDeCorrida(t)
	root := initRepo(t)
	var ids []string
	for i := 0; i < 6; i++ {
		f, err := Add(root, "main", []string{"low", "medium", "high"}[i%3], "risk", "app.go", fmt.Sprintf("hallazgo %d", i), "reviewer@codex")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, f.ID)
	}
	r, err := Register(root, "main", Draft{Severity: "high", Lens: "risk", File: "app.go", Description: "con tarea", Author: "gate", Task: "evidencia-en-disco"})
	if err != nil {
		t.Fatal(err)
	}
	ids = append(ids, r.ID)
	hay := fnArchivos(t, root)
	for _, id := range ids {
		n := id + ".json"
		if !hay[n] {
			t.Fatalf("CA-442: fixture: el hallazgo %s quedo en .hoom/findings/%s: %v", id, n, hay)
		}
		if !EsNombre(n) {
			t.Fatalf("CA-442: el hallazgo que escribe Add se llama %q y EsNombre lo rechaza", n)
		}
	}

	for i, as := range []string{StatusRefuted, StatusCorrected} {
		antes := fnArchivos(t, root)
		if _, err := Resolve(root, ids[i], as, "TestRetry lo cubre", "Henry Orellana"); err != nil {
			t.Fatal(err)
		}
		var nuevos []string
		for n := range fnArchivos(t, root) {
			if !antes[n] {
				nuevos = append(nuevos, n)
			}
		}
		if len(nuevos) != 1 || !strings.HasSuffix(nuevos[0], ".res.json") {
			t.Fatalf("CA-442: fixture: Resolve escribe UN archivo .res.json: %v", nuevos)
		}
		if !EsNombre(nuevos[0]) {
			t.Fatalf("CA-442: la resolucion (%s) que escribe Resolve se llama %q y EsNombre lo rechaza", as, nuevos[0])
		}
	}
}

// CA-442 (hallazgo e44cca): un nombre con la forma de hallazgo o de
// resolucion pero con una fecha o una hora que no existen no es un nombre
// que hoom escribe: mes 13 o 00, 30 de febrero, 29 de febrero de un anio que
// no es bisiesto, dia 00 o 32, hora 24, minuto o segundo 60. Y los negativos
// de siempre siguen siendo negativos (hex en mayusculas, otro largo, .JSON,
// sufijos de mas, la forma de un veredicto, rutas, vacio). Control: las
// mismas formas con fechas que existen si lo son.
func TestCA442_UnNombreDeHallazgoConUnaFechaQueNoExisteNoEsNombre(t *testing.T) {
	imposibles := []string{
		"20261301T000000_abcdef",
		"20260230T000000_abcdef",
		"20260101T240000_abcdef",
		"20260101T006000_abcdef",
		"20260101T000060_abcdef",
		"20260001T000000_abcdef",
		"20260100T000000_abcdef",
		"20260132T000000_abcdef",
		"20260431T000000_abcdef",
		"20270229T000000_abcdef",
		"21000229T000000_abcdef",
		"20260101T999999_abcdef",
	}
	var malos []string
	for _, id := range imposibles {
		malos = append(malos, id+".json", id+".res.json")
	}
	malos = append(malos,
		".gitignore", ".oculto", ".json", ".res.json", "x.json", "x.res.json", "notas.txt", "f-nuevo.json",
		"20260101T000002_ABCDEF.json",
		"20260101T000000_ABCDEF.res.json",
		"20260101T000000_abcde.json",
		"20260101T000000_abcde.res.json",
		"20260101T000000_abcdef0.json",
		"20260101T000000_abcdeg.json",
		"2026010T000000_abcdef.json",
		"20260101-000000_abcdef.json",
		"20260101T000000abcdef.json",
		"20260101T000000_abcdef.json.bak",
		"20260101T000001_abcdef.JSON",
		"20260101T000000_abcdef.RES.json",
		"20260101T000000_abcdef.res.json.tmp",
		"20260101T000000_abcdef.res.res.json",
		"20260101T000000_abcdef",
		"20260101T000000_abcdef.json ",
		" 20260101T000000_abcdef.json",
		"20260101T000000_abcdef.json\n",
		"sub/20260101T000000_abcdef.json",
		".hoom/findings/20260101T000000_abcdef.json",
		"２０２６0101T000000_abcdef.json",
		"2026-01-01T00-00-00Z_0123abcd.json",
		"2026-01-01T00-00-00Z_0123abcd.res.json",
		"",
	)
	for _, n := range malos {
		if EsNombre(n) {
			t.Fatalf("CA-442: %q no es un nombre que hoom escribe bajo .hoom/findings/: EsNombre lo acepta", n)
		}
	}
	for _, id := range []string{"20260101T000000_abcdef", "20280229T235959_c0ffee", "20000229T000000_000000", "20261231T235959_ffffff", "20261001T184836_e44cca"} {
		for _, n := range []string{id + ".json", id + ".res.json"} {
			if !EsNombre(n) {
				t.Fatalf("CA-442: %q es un nombre que hoom escribe (control): EsNombre lo rechaza", n)
			}
		}
	}
}

// CA-442 (hallazgo e44cca, propiedad): para fechas al azar armadas campo por
// campo (anio 1970-2399, mes 0-13, dia 0-32, hora 0-25, minuto y segundo
// 0-61) y 6 hex al azar, EsNombre acepta <id>.json y <id>.res.json si y solo
// si la fecha existe (time.Date no la normaliza). La semilla queda en la
// falla.
func TestCA442_PropiedadEsNombreDeHallazgoSiYSoloSiLaFechaExiste(t *testing.T) {
	semilla := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(semilla))
	for i := 0; i < 3000; i++ {
		y, mo, d := 1970+rng.Intn(430), rng.Intn(14), rng.Intn(33)
		h, mi, s := rng.Intn(26), rng.Intn(62), rng.Intn(62)
		id := fmt.Sprintf("%04d%02d%02dT%02d%02d%02d_%06x", y, mo, d, h, mi, s, rng.Intn(1<<24))
		tt := time.Date(y, time.Month(mo), d, h, mi, s, 0, time.UTC)
		existe := tt.Year() == y && int(tt.Month()) == mo && tt.Day() == d && tt.Hour() == h && tt.Minute() == mi && tt.Second() == s
		for _, n := range []string{id + ".json", id + ".res.json"} {
			if got := EsNombre(n); got != existe {
				t.Fatalf("CA-442: semilla %d: EsNombre(%q) = %v; la fecha existe = %v", semilla, n, got, existe)
			}
		}
	}
}
