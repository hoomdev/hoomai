// Tests de contrato de verdict.EsNombre para el hallazgo e44cca de la
// segunda ronda de review de .hoom/specs/evidencia-en-disco.md (CA-442: un
// veredicto creado es legitimo solo con la forma que escribe hoom,
// <AAAA-MM-DDTHH-MM-SSZ>_<8 hex>.json): una sola definicion de los nombres.
// Todo nombre que verdict.Write escribe cumple EsNombre, en cualquier
// instante; y un nombre con la forma pero con una fecha que no existe no es
// un nombre que hoom escribe. Escritos desde el contrato, sin leer la
// implementacion.
package verdict_test

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/verdict"
)

// vnEscribir escribe un veredicto con CreatedAt = cuando y devuelve la ruta
// absoluta que verdict.Write dice.
func vnEscribir(t *testing.T, dir string, cuando time.Time) string {
	t.Helper()
	v := &verdict.Verdict{Schema: verdict.SchemaID, CreatedAt: cuando, Project: "demo", Verdict: "red",
		Notes: []string{"instante " + cuando.String()}}
	p, err := verdict.Write(dir, v)
	if err != nil {
		t.Fatalf("CA-442: verdict.Write en %s: %v", cuando, err)
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(dir, p)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("CA-442: fixture: verdict.Write dejo el veredicto en %s: %v", p, err)
	}
	return p
}

// CA-442 (hallazgo e44cca): para cada instante (UTC, zonas que no son UTC y
// que cruzan el dia, el mes o el anio al pasarlas a UTC, el 29 de febrero de
// un bisiesto, fines de mes y de anio, 23:59:59 con nanosegundos, la hora
// local y la de ahora), el nombre del archivo que verdict.Write escribe
// cumple verdict.EsNombre, y es <id>.json bajo .hoom/verdicts.
func TestCA442_LosNombresQueEscribeVerdictWriteCumplenEsNombre(t *testing.T) {
	dir := t.TempDir()
	menos5 := time.FixedZone("UTC-5", -5*3600)
	mas14 := time.FixedZone("UTC+14", 14*3600)
	menos0330 := time.FixedZone("UTC-03:30", -(3*3600 + 30*60))
	mas0545 := time.FixedZone("UTC+05:45", 5*3600+45*60)
	instantes := []time.Time{
		time.Date(2026, 10, 1, 18, 48, 36, 560504000, time.UTC),
		time.Date(2026, 10, 1, 18, 48, 36, 0, menos5),
		time.Date(2026, 12, 31, 22, 30, 0, 0, menos5), // en UTC ya es 2027
		time.Date(2027, 1, 1, 9, 0, 0, 0, mas14),      // en UTC todavia es 2026
		time.Date(2028, 2, 29, 12, 0, 0, 0, time.UTC), // bisiesto
		time.Date(2028, 2, 29, 23, 59, 59, 999999999, menos0330),
		time.Date(2028, 3, 1, 0, 0, 0, 0, mas0545),      // en UTC es 29 de febrero
		time.Date(2000, 2, 29, 23, 59, 59, 0, time.UTC), // bisiesto de siglo
		time.Date(2026, 12, 31, 23, 59, 59, 999999999, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 1, 31, 23, 59, 59, 0, time.UTC),
		time.Date(2026, 2, 28, 23, 59, 59, 0, time.UTC),
		time.Date(2026, 4, 30, 23, 59, 59, 0, time.UTC),
		time.Date(2026, 6, 30, 23, 59, 59, 0, mas14),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 3, 29, 1, 30, 0, 0, time.Local),
		time.Now(),
		time.Now().UTC(),
	}
	vistos := map[string]bool{}
	for _, cuando := range instantes {
		p := vnEscribir(t, dir, cuando)
		nombre := filepath.Base(p)
		if !verdict.EsNombre(nombre) {
			t.Fatalf("CA-442: el veredicto que verdict.Write escribe para %s se llama %q y verdict.EsNombre lo rechaza", cuando, nombre)
		}
		if filepath.Base(filepath.Dir(p)) != "verdicts" || filepath.Base(filepath.Dir(filepath.Dir(p))) != ".hoom" {
			t.Fatalf("CA-442: el veredicto va a .hoom/verdicts: %s", p)
		}
		if vistos[nombre] {
			t.Fatalf("CA-442: fixture: dos veredictos, dos nombres: %s", nombre)
		}
		vistos[nombre] = true
	}
}

// CA-442 (hallazgo e44cca, propiedad): para instantes al azar entre 1970 y
// 2400, en zonas al azar de -12:00 a +14:00 (con cuartos de hora), el nombre
// que verdict.Write escribe cumple verdict.EsNombre. La semilla queda en la
// falla.
func TestCA442_PropiedadTodoNombreQueEscribeVerdictWriteCumpleEsNombre(t *testing.T) {
	dir := t.TempDir()
	semilla := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(semilla))
	desde := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	hasta := time.Date(2400, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	for i := 0; i < 150; i++ {
		seg := desde + rng.Int63n(hasta-desde)
		zona := time.FixedZone(fmt.Sprintf("z%d", i), (rng.Intn(26*4+1)-12*4)*15*60)
		cuando := time.Unix(seg, rng.Int63n(1e9)).In(zona)
		nombre := filepath.Base(vnEscribir(t, dir, cuando))
		if !verdict.EsNombre(nombre) {
			t.Fatalf("CA-442: semilla %d: el veredicto que verdict.Write escribe para %s se llama %q y verdict.EsNombre lo rechaza",
				semilla, cuando, nombre)
		}
	}
}

// CA-442 (hallazgo e44cca): un nombre con la forma de veredicto pero con una
// fecha o una hora que no existen no es un nombre que hoom escribe (ningun
// instante lo da): mes 13 o 00, dia 30 de febrero, 29 de febrero de un anio
// que no es bisiesto, dia 00 o 32, hora 24, minuto o segundo 60. Y los
// negativos de siempre siguen siendo negativos: hex en mayusculas, otro
// largo, .JSON, sufijos de mas, la forma de un hallazgo, rutas, vacio.
// Control: las mismas formas con fechas que existen si lo son.
func TestCA442_UnNombreDeVeredictoConUnaFechaQueNoExisteNoEsNombre(t *testing.T) {
	for _, n := range []string{
		"2026-13-01T00-00-00Z_0123abcd.json",
		"2026-02-30T00-00-00Z_0123abcd.json",
		"2026-00-10T00-00-00Z_0123abcd.json",
		"2026-01-00T00-00-00Z_0123abcd.json",
		"2026-01-32T00-00-00Z_0123abcd.json",
		"2026-04-31T00-00-00Z_0123abcd.json",
		"2027-02-29T00-00-00Z_0123abcd.json",
		"2100-02-29T00-00-00Z_0123abcd.json",
		"2026-01-01T24-00-00Z_0123abcd.json",
		"2026-01-01T00-60-00Z_0123abcd.json",
		"2026-01-01T00-00-60Z_0123abcd.json",
		"2026-01-01T99-99-99Z_0123abcd.json",
		// los de siempre
		"2026-01-01T00-00-01Z_0123ABCD.json",
		"2026-01-01T00-00-00Z_0123abc.json",
		"2026-01-01T00-00-00Z_0123abcd0.json",
		"2026-01-01T00-00-00Z_0123abcg.json",
		"2026-01-01T00-00-00_0123abcd.json",
		"2026-01-01T00-00-00Z0123abcd.json",
		"2026-01-01 00-00-00Z_0123abcd.json",
		"2026-1-01T00-00-00Z_0123abcd.json",
		"2026-01-01T00-00-00Z_0123abcd.JSON",
		"2026-01-01T00-00-00Z_0123abcd.json.bak",
		"2026-01-01T00-00-00Z_0123abcd.res.json",
		"2026-01-01T00-00-00Z_0123abcd",
		"2026-01-01T00-00-00Z_0123abcd.json ",
		" 2026-01-01T00-00-00Z_0123abcd.json",
		"2026-01-01T00-00-00Z_0123abcd.json\n",
		"sub/2026-01-01T00-00-00Z_0123abcd.json",
		".hoom/verdicts/2026-01-01T00-00-00Z_0123abcd.json",
		"２０２６-01-01T00-00-00Z_0123abcd.json",
		"20260101T000000_abcdef.json",
		"2026-09-18T12-00-00Z_propio.json",
		"x.json",
		".gitignore",
		".json",
		"",
	} {
		if verdict.EsNombre(n) {
			t.Fatalf("CA-442: %q no es un nombre que verdict.Write escribe: verdict.EsNombre lo acepta", n)
		}
	}
	for _, n := range []string{
		"2026-01-01T00-00-00Z_0123abcd.json",
		"2028-02-29T23-59-59Z_deadbeef.json",
		"2000-02-29T00-00-00Z_00000000.json",
		"2026-12-31T23-59-59Z_ffffffff.json",
		"2026-04-30T12-30-45Z_a1b2c3d4.json",
	} {
		if !verdict.EsNombre(n) {
			t.Fatalf("CA-442: %q es un nombre que verdict.Write escribe (control): verdict.EsNombre lo rechaza", n)
		}
	}
}

// CA-442 (hallazgo e44cca, propiedad): para fechas al azar armadas campo por
// campo (anio 1970-2399, mes 0-13, dia 0-32, hora 0-25, minuto y segundo
// 0-61) y 8 hex al azar, verdict.EsNombre acepta el nombre si y solo si la
// fecha existe (time.Date no la normaliza). La semilla queda en la falla.
func TestCA442_PropiedadEsNombreDeVeredictoSiYSoloSiLaFechaExiste(t *testing.T) {
	semilla := time.Now().UnixNano()
	rng := rand.New(rand.NewSource(semilla))
	for i := 0; i < 3000; i++ {
		y, mo, d := 1970+rng.Intn(430), rng.Intn(14), rng.Intn(33)
		h, mi, s := rng.Intn(26), rng.Intn(62), rng.Intn(62)
		n := fmt.Sprintf("%04d-%02d-%02dT%02d-%02d-%02dZ_%08x.json", y, mo, d, h, mi, s, rng.Uint32())
		tt := time.Date(y, time.Month(mo), d, h, mi, s, 0, time.UTC)
		existe := tt.Year() == y && int(tt.Month()) == mo && tt.Day() == d && tt.Hour() == h && tt.Minute() == mi && tt.Second() == s
		if got := verdict.EsNombre(n); got != existe {
			t.Fatalf("CA-442: semilla %d: verdict.EsNombre(%q) = %v; la fecha existe = %v", semilla, n, got, existe)
		}
	}
}
