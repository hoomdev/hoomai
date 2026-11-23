// Tests adversariales del spec .hoom/specs/riesgo-efectivo-y-tamano.md sobre
// `hoom status` (CA-786): el ultimo veredicto trae su riesgo y su tamano
// (LastVerdict.Riesgo y Tamano), el texto los imprime debajo de la linea del
// veredicto (cada una solo si el veredicto la tiene) y --json trae `riesgo`
// y `tamano` del ultimo veredicto (aclaracion 23). Escritos desde el spec,
// las aclaraciones de la ronda 0 y las firmas publicas, sin leer la
// implementacion.
package statuscmd

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/riesgo"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const retyMotAuth = "el cambio toca internal/auth/x.go: riesgo alto"

// retyVeredicto escribe en dir un veredicto verde, el mas nuevo, con ev y
// tam (nil = sin la clave).
func retyVeredicto(t *testing.T, dir string, ev *riesgo.Evaluacion, tam *riesgo.Tamano) {
	t.Helper()
	v := &verdict.Verdict{Schema: verdict.SchemaID, CreatedAt: time.Now().UTC().Add(time.Hour), Project: "demo",
		Git:    gitx.Info{IsRepo: true, ChangeFingerprint: "huella", Insertions: 9300, Deletions: 81, ChangedFiles: []string{"internal/auth/x.go"}},
		Gates:  []verdict.GateResult{{Name: "test", Required: true, Status: verdict.StatusPass, Scope: "full"}},
		Riesgo: ev, Tamano: tam}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
}

func retyTexto(t *testing.T, s *Snapshot) []string {
	t.Helper()
	var b bytes.Buffer
	Render(&b, s, false)
	return strings.Split(b.String(), "\n")
}

// retyIndice es la primera linea que, sin blancos en los bordes, empieza
// con p (o es exactamente p si exacta); -1 si ninguna.
func retyIndice(lineas []string, p string, exacta bool) int {
	for i, l := range lineas {
		s := strings.TrimSpace(l)
		if (exacta && s == p) || (!exacta && strings.HasPrefix(s, p)) {
			return i
		}
	}
	return -1
}

func retyUltimo(t *testing.T, ca string, raw []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("%s: JSON: %v\n%s", ca, err, raw)
	}
	lv, ok := m["last_verdict"].(map[string]any)
	if !ok {
		t.Fatalf("%s: el JSON trae last_verdict: %s", ca, raw)
	}
	return lv
}

// CA-786: el ultimo veredicto con riesgo y tamano: LastVerdict, el texto
// (riesgo y tamano debajo de la linea del veredicto, en ese orden, y sin el
// viejo `cambio grande`) y el JSON.
func TestCA786_StatusConRiesgoYTamano(t *testing.T) {
	const ca = "CA-786"
	dir := initProject(t)
	ev := &riesgo.Evaluacion{Declarado: "normal", Efectivo: "alto", Motivo: retyMotAuth}
	tam := &riesgo.Tamano{Producto: 90, Pruebas: 1500, Hoom: 32}
	retyVeredicto(t, dir, ev, tam)
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Last == nil || s.Last.Riesgo == nil || *s.Last.Riesgo != *ev || s.Last.Tamano == nil || *s.Last.Tamano != *tam {
		t.Fatalf("%s: LastVerdict trae el riesgo y el tamano del veredicto: %+v", ca, s.Last)
	}
	lineas := retyTexto(t, s)
	texto := strings.Join(lineas, "\n")
	v := retyIndice(lineas, "veredicto:", false)
	r := retyIndice(lineas, "riesgo: alto (el cambio toca internal/auth/x.go: riesgo alto)", true)
	tm := retyIndice(lineas, "tamano: producto 90 · pruebas 1500 · hoom 32", true)
	if v < 0 || r < 0 || tm < 0 || !(v < r && r < tm) {
		t.Fatalf("%s: riesgo y tamano van debajo de la linea del veredicto (%d, %d, %d):\n%s", ca, v, r, tm, texto)
	}
	if strings.Contains(texto, "cambio grande") {
		t.Fatalf("%s: status no dice `cambio grande`:\n%s", ca, texto)
	}
	raw, err := JSONBytes(s)
	if err != nil {
		t.Fatal(err)
	}
	lv := retyUltimo(t, ca, raw)
	if !reflect.DeepEqual(lv["riesgo"], map[string]any{"declarado": "normal", "efectivo": "alto", "motivo": retyMotAuth}) {
		t.Fatalf("%s: last_verdict.riesgo: %v", ca, lv["riesgo"])
	}
	if !reflect.DeepEqual(lv["tamano"], map[string]any{"producto": float64(90), "pruebas": float64(1500), "hoom": float64(32)}) {
		t.Fatalf("%s: last_verdict.tamano: %v", ca, lv["tamano"])
	}
}

// CA-786 y aclaracion 16: un veredicto sin las claves (anterior al spec) no
// imprime las lineas ni trae las claves; uno con riesgo y sin tamano (fuera
// de un repo) imprime solo la de riesgo.
func TestCA786_StatusSinLasClaves(t *testing.T) {
	const ca = "CA-786"
	dir := initProject(t)
	retyVeredicto(t, dir, nil, nil)
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Last == nil || s.Last.Riesgo != nil || s.Last.Tamano != nil {
		t.Fatalf("%s: sin las claves en el veredicto, LastVerdict no las trae: %+v", ca, s.Last)
	}
	lineas := retyTexto(t, s)
	if retyIndice(lineas, "riesgo:", false) >= 0 || retyIndice(lineas, "tamano:", false) >= 0 {
		t.Fatalf("%s: sin las claves no hay lineas riesgo ni tamano:\n%s", ca, strings.Join(lineas, "\n"))
	}
	raw, _ := JSONBytes(s)
	lv := retyUltimo(t, ca, raw)
	for _, k := range []string{"riesgo", "tamano"} {
		if _, ok := lv[k]; ok {
			t.Fatalf("%s: sin la clave en el veredicto, last_verdict no trae %s: %s", ca, k, raw)
		}
	}

	dir = initProject(t)
	retyVeredicto(t, dir, &riesgo.Evaluacion{Declarado: "normal", Efectivo: "normal", Motivo: "sin tarjeta: riesgo normal"}, nil)
	s, err = Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	lineas = retyTexto(t, s)
	if retyIndice(lineas, "riesgo: normal (sin tarjeta: riesgo normal)", true) < 0 || retyIndice(lineas, "tamano:", false) >= 0 {
		t.Fatalf("%s: con riesgo y sin tamano, solo la linea de riesgo:\n%s", ca, strings.Join(lineas, "\n"))
	}
	raw, _ = JSONBytes(s)
	lv = retyUltimo(t, ca, raw)
	if _, ok := lv["tamano"]; ok || lv["riesgo"] == nil {
		t.Fatalf("%s: last_verdict trae riesgo y no tamano: %s", ca, raw)
	}
}

// CA-786: con un verify de verdad en el principal (sin tarea), status
// muestra `riesgo: normal (sin tarjeta: riesgo normal)` y el tamano.
func TestCA786_StatusDespuesDeUnVerify(t *testing.T) {
	const ca = "CA-786"
	dir := initProject(t)
	verify(t, dir)
	s, err := Build(dir, "main")
	if err != nil {
		t.Fatal(err)
	}
	if s.Last == nil || s.Last.Riesgo == nil ||
		*s.Last.Riesgo != (riesgo.Evaluacion{Declarado: "normal", Efectivo: "normal", Motivo: riesgo.MotivoSinTarjeta}) {
		t.Fatalf("%s: el verify del principal no tiene tarea: %+v", ca, s.Last)
	}
	if s.Last.Tamano == nil || s.Last.Tamano.Producto != 0 || s.Last.Tamano.Pruebas != 0 {
		t.Fatalf("%s: sin cambios de producto ni pruebas: %+v", ca, s.Last.Tamano)
	}
	lineas := retyTexto(t, s)
	if retyIndice(lineas, "riesgo: normal (sin tarjeta: riesgo normal)", true) < 0 || retyIndice(lineas, s.Last.Tamano.Linea(), true) < 0 {
		t.Fatalf("%s: status muestra riesgo y tamano del verify:\n%s", ca, strings.Join(lineas, "\n"))
	}
}
