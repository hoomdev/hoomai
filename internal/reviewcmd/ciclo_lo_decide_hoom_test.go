// Tests adversariales del spec .hoom/specs/ciclo-lo-decide-hoom.md
// («Que es una ronda», CA-749): reviewcmd.Rondas cuenta los registros de
// review de la tarea con cobertura delta y fecha POSTERIOR a su registro
// completa mas nuevo; 0 sin completa; una completa nueva lo vuelve a 0.
// Escrito desde el spec y el go doc de reviewcmd.Rondas.
package reviewcmd

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"testing/quick"
	"time"
)

var cldT0 = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)

var cldSeq int

// cldReg es un registro de review de tarea, con cobertura cob, min minutos
// despues de cldT0.
func cldReg(tarea, cob string, min int) Record {
	cldSeq++
	return Record{ID: fmt.Sprintf("20261121T100000_%06x", cldSeq), CreatedAt: cldT0.Add(time.Duration(min) * time.Minute),
		Task: tarea, Cobertura: cob, Findings: []string{}}
}

// cldReferencia es la definicion del spec, escrita aparte: las deltas de la
// tarea posteriores a su completa mas nueva.
func cldReferencia(rs []Record, tarea string) int {
	var ult time.Time
	hay := false
	for _, r := range rs {
		if r.Task == tarea && r.Cobertura == CoberturaCompleta && (!hay || r.CreatedAt.After(ult)) {
			ult, hay = r.CreatedAt, true
		}
	}
	if !hay {
		return 0
	}
	n := 0
	for _, r := range rs {
		if r.Task == tarea && r.Cobertura == CoberturaDelta && r.CreatedAt.After(ult) {
			n++
		}
	}
	return n
}

func cldRondas(t *testing.T, ca, que string, rs []Record, tarea string, quiero int) {
	t.Helper()
	if got := Rondas(rs, tarea); got != quiero {
		t.Fatalf("%s: [%s] Rondas(%q) es %d, fue %d", ca, que, tarea, quiero, got)
	}
}

// CA-749: las deltas despues de la completa mas nueva son las rondas. Una
// delta sin hallazgos (despues de integrar la base, por ejemplo) cuenta igual:
// la definicion es por registros.
func TestCA749_CuentaLasDeltasDespuesDeLaCompleta(t *testing.T) {
	sinCorrecciones := cldReg("precios", CoberturaDelta, 30)
	sinCorrecciones.Notes = []string{"despues de integrar la base: nada que corregir"}
	rs := []Record{cldReg("precios", CoberturaCompleta, 0), cldReg("precios", CoberturaDelta, 10), cldReg("precios", CoberturaDelta, 20), sinCorrecciones}
	cldRondas(t, "CA-749", "completa y tres deltas", rs, "precios", 3)
	cldRondas(t, "CA-749", "solo la completa", rs[:1], "precios", 0)
	cldRondas(t, "CA-749", "completa y una delta", rs[:2], "precios", 1)
}

// CA-749: sin registro completa de la tarea es 0, aunque haya deltas; sin
// registros tambien.
func TestCA749_SinCompletaEsCero(t *testing.T) {
	cldRondas(t, "CA-749", "nil", nil, "precios", 0)
	cldRondas(t, "CA-749", "vacio", []Record{}, "precios", 0)
	cldRondas(t, "CA-749", "solo deltas", []Record{cldReg("precios", CoberturaDelta, 10), cldReg("precios", CoberturaDelta, 20)}, "precios", 0)
	cldRondas(t, "CA-749", "la completa es de otra tarea", []Record{cldReg("otra", CoberturaCompleta, 0),
		cldReg("precios", CoberturaDelta, 10)}, "precios", 0)
	cldRondas(t, "CA-749", "parciales y deltas", []Record{cldReg("precios", CoberturaParcial, 0),
		cldReg("precios", CoberturaDelta, 10)}, "precios", 0)
}

// CA-749: una review completa nueva vuelve a 0; las deltas que la siguen
// vuelven a contar desde ahi.
func TestCA749_UnaCompletaNuevaVuelveACero(t *testing.T) {
	rs := []Record{cldReg("precios", CoberturaCompleta, 0), cldReg("precios", CoberturaDelta, 10),
		cldReg("precios", CoberturaDelta, 20), cldReg("precios", CoberturaCompleta, 30)}
	cldRondas(t, "CA-749", "completa nueva", rs, "precios", 0)
	rs = append(rs, cldReg("precios", CoberturaDelta, 40))
	cldRondas(t, "CA-749", "una delta despues de la completa nueva", rs, "precios", 1)
}

// CA-749: las deltas anteriores a la completa mas nueva no cuentan, y la
// fecha que manda es CreatedAt, no el orden de la lista ni el id.
func TestCA749_LasDeltasAnterioresNoCuentan(t *testing.T) {
	vieja := cldReg("precios", CoberturaDelta, -10)
	vieja.ID = "20261121T235959_ffffff" // un id que ordena despues de todos
	rs := []Record{cldReg("precios", CoberturaDelta, 15), cldReg("precios", CoberturaCompleta, 5), vieja,
		cldReg("precios", CoberturaCompleta, 0), cldReg("precios", CoberturaDelta, 3)}
	// la completa mas nueva es la de min 5: cuenta solo la delta de min 15
	cldRondas(t, "CA-749", "desordenados", rs, "precios", 1)
}

// CA-749: una delta con la misma fecha que la completa no es posterior.
func TestCA749_LaMismaFechaNoEsPosterior(t *testing.T) {
	rs := []Record{cldReg("precios", CoberturaCompleta, 0), cldReg("precios", CoberturaDelta, 0)}
	cldRondas(t, "CA-749", "misma fecha", rs, "precios", 0)
	rs = append(rs, Record{ID: "20261121T100000_aaaaaa", CreatedAt: cldT0.Add(time.Nanosecond), Task: "precios", Cobertura: CoberturaDelta})
	cldRondas(t, "CA-749", "un nanosegundo despues", rs, "precios", 1)
}

// CA-749: los registros de otras tareas, los parciales y los sin cobertura
// nunca cuentan; una completa de otra tarea no vuelve a 0 a esta.
func TestCA749_OtrasTareasParcialesYSinCoberturaNoCuentan(t *testing.T) {
	rs := []Record{
		cldReg("precios", CoberturaCompleta, 0),
		cldReg("otra", CoberturaDelta, 10),
		cldReg("precios", CoberturaParcial, 12),
		cldReg("precios", "", 14),
		cldReg("precios", "Delta", 15),
		cldReg("otra", CoberturaCompleta, 16),
		cldReg("precios", CoberturaDelta, 20),
		cldReg("precios-2", CoberturaDelta, 22),
		cldReg("", CoberturaDelta, 24),
	}
	cldRondas(t, "CA-749", "mezcla", rs, "precios", 1)
	cldRondas(t, "CA-749", "la otra tarea", rs, "otra", 0)
	cldRondas(t, "CA-749", "una tarea sin registros", rs, "nadie", 0)
}

// CA-749 (propiedad): para cualquier lista de registros, Rondas es la
// definicion del spec y no depende del orden de la lista.
func TestCA749_PropiedadDefinicionYOrden(t *testing.T) {
	tareas := []string{"precios", "otra"}
	cobs := []string{CoberturaCompleta, CoberturaDelta, CoberturaParcial, ""}
	prop := func(semilla int64, cuantos uint8) bool {
		r := rand.New(rand.NewSource(semilla))
		n := int(cuantos % 16)
		var rs []Record
		for i := 0; i < n; i++ {
			rs = append(rs, cldReg(tareas[r.Intn(len(tareas))], cobs[r.Intn(len(cobs))], r.Intn(12)))
		}
		for _, tarea := range tareas {
			quiero := cldReferencia(rs, tarea)
			if Rondas(rs, tarea) != quiero {
				return false
			}
			mezcla := append([]Record(nil), rs...)
			r.Shuffle(len(mezcla), func(i, j int) { mezcla[i], mezcla[j] = mezcla[j], mezcla[i] })
			if Rondas(mezcla, tarea) != quiero {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, &quick.Config{MaxCount: 300}); err != nil {
		t.Fatalf("CA-749: %v", err)
	}
}

// CA-749: Rondas no modifica los registros que recibe.
func TestCA749_NoTocaLosRegistros(t *testing.T) {
	rs := []Record{cldReg("precios", CoberturaDelta, 20), cldReg("precios", CoberturaCompleta, 0), cldReg("precios", CoberturaDelta, 10)}
	copia := append([]Record(nil), rs...)
	Rondas(rs, "precios")
	if !reflect.DeepEqual(rs, copia) {
		t.Fatalf("CA-749: Rondas deja los registros como estaban: %+v", rs)
	}
}

// CA-749: sobre los registros de disco (WriteRecord y Records, lo que leen
// el piloto y el tablero) cuenta lo mismo.
func TestCA749_ConLosRegistrosDeDisco(t *testing.T) {
	dir := t.TempDir()
	escribir := func(tarea, cob string, min int) {
		t.Helper()
		if _, err := WriteRecord(dir, Record{CreatedAt: cldT0.Add(time.Duration(min) * time.Minute), Task: tarea,
			Spec: ".hoom/specs/" + tarea + ".md", Fingerprint: "h", VerdictID: "v", Verdict: "green", Lenses: Lentes,
			Provider: "codex", Writer: "claude", Cross: CrossYes, Findings: []string{}, Usage: []LensUsage{},
			Cobertura: cob, Base: "main", Desde: strings.Repeat("a", 40), Hasta: strings.Repeat("b", 40)}); err != nil {
			t.Fatalf("fixture: WriteRecord: %v", err)
		}
	}
	escribir("precios", CoberturaCompleta, 0)
	escribir("precios", CoberturaDelta, 10)
	escribir("otra", CoberturaDelta, 15)
	escribir("precios", CoberturaParcial, 17)
	escribir("precios", CoberturaDelta, 20)
	rs, avisos := Records(dir)
	if len(avisos) != 0 || len(rs) != 5 {
		t.Fatalf("fixture: Records lee los cinco registros: %d %q", len(rs), avisos)
	}
	cldRondas(t, "CA-749", "de disco", rs, "precios", 2)
	escribir("precios", CoberturaCompleta, 30)
	rs, _ = Records(dir)
	cldRondas(t, "CA-749", "de disco, completa nueva", rs, "precios", 0)
}
