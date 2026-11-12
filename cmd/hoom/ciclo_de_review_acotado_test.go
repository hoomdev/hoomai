// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md contra
// el BINARIO: `hoom finding raise` (CA-625) y el texto de `hoom finding
// list` (`bloquea: <motivo>` / `pendiente`, los avisos de subidas
// invalidas; CA-628 y CA-626). Usa el entorno a mano de quien_cierra_test.go
// (qcCorrer): ninguna HOOM_* heredada se cuela. Compila el binario: se omite
// con -short. Escritos desde el spec y las firmas publicas, sin leer la
// implementacion.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func crRaisePath(dir, id string) string {
	return filepath.Join(dir, ".hoom", "findings", id+".raise.json")
}

// crHallazgoSev registra un hallazgo de severidad sev con el binario.
func crHallazgoSev(t *testing.T, bin, dir, sev, file string) string {
	t.Helper()
	res := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "add", "--sev", sev, "--lens", "risk",
		"--file", file, "--author", "reviewer@codex", "hallazgo "+sev+" de prueba")
	if res.exit != 0 {
		t.Fatalf("fixture: hoom finding add salio %d\n%s", res.exit, res.todo())
	}
	m := qcIDRe.FindStringSubmatch(res.stdout)
	if m == nil {
		t.Fatalf("fixture: no encontre el id en la salida de finding add:\n%s", res.stdout)
	}
	return m[1]
}

func crSinRaise(t *testing.T, ca, caso, dir, id string) {
	t.Helper()
	if raw, err := os.ReadFile(crRaisePath(dir, id)); err == nil {
		t.Fatalf("%s: %s: una negativa no escribe nada, y quedo %s.raise.json:\n%s", ca, caso, id, raw)
	}
}

// CA-625: raise escribe el .raise.json con finding_id, reason, author y
// raised_at, e imprime `hallazgo <id> subido: ahora bloquea`; otra vez sale 0
// con `ya estaba subido` sin reescribir.
func TestCA625_RaiseEscribeLaSubida(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	id := crHallazgoSev(t, bin, dir, "low", "app.go")
	res := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "raise", id, "--reason", "  toca el cobro  ")
	if res.exit != 0 || !strings.Contains(res.stdout, "hallazgo "+id+" subido: ahora bloquea") {
		t.Fatalf("CA-625: raise sale 0 con `hallazgo %s subido: ahora bloquea`: %d\n%s", id, res.exit, res.todo())
	}
	raw, err := os.ReadFile(crRaisePath(dir, id))
	if err != nil {
		t.Fatalf("CA-625: raise escribe %s.raise.json: %v", id, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("CA-625: el .raise.json es JSON: %v\n%s", err, raw)
	}
	if m["finding_id"] != id || m["reason"] != "toca el cobro" {
		t.Fatalf("CA-625: finding_id %s y reason recortada: %v", id, m)
	}
	if a, _ := m["author"].(string); !strings.Contains(a, "Persona Git") {
		t.Fatalf("CA-625: author es la identidad git: %v", m["author"])
	}
	if s, _ := m["raised_at"].(string); s == "" {
		t.Fatalf("CA-625: raised_at: %v", m)
	}
	info, _ := os.Stat(crRaisePath(dir, id))
	res = qcCorrer(t, bin, dir, qcSinCorrida, "finding", "raise", id, "--reason", "otra razon")
	if res.exit != 0 || !strings.Contains(res.stdout, "hallazgo "+id+" ya estaba subido") {
		t.Fatalf("CA-625: subir otra vez sale 0 con `ya estaba subido`: %d\n%s", res.exit, res.todo())
	}
	raw2, _ := os.ReadFile(crRaisePath(dir, id))
	info2, _ := os.Stat(crRaisePath(dir, id))
	if string(raw2) != string(raw) || !info2.ModTime().Equal(info.ModTime()) {
		t.Fatalf("CA-625: ya subido no reescribe el archivo:\nantes %s\ndespues %s", raw, raw2)
	}
}

// CA-625: las negativas salen 1 con el texto del contrato y sin escribir.
func TestCA625_RaiseSeNiega(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	id := crHallazgoSev(t, bin, dir, "low", "app.go")
	cerrado := crHallazgoSev(t, bin, dir, "low", "app.go")
	if r := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "resolve", cerrado, "--as", "refutado",
		"--evidence", "el test TestX lo tumba"); r.exit != 0 {
		t.Fatalf("fixture: resolve salio %d\n%s", r.exit, r.todo())
	}
	for _, c := range []struct {
		caso string
		env  map[string]string
		args []string
		id   string
		want string
	}{
		{"dentro de una corrida (refutador)", qcEnCorrida("refutador", qcRunID, "codex"), []string{id, "--reason", "x"}, id,
			"un refutador no sube hallazgos: lo hace una persona"},
		{"dentro de una corrida (writer)", qcEnCorrida("writer", qcRunID, "claude"), []string{id, "--reason", "x"}, id,
			"un writer no sube hallazgos: lo hace una persona"},
		{"sin --reason", qcSinCorrida, []string{id}, id, "falta --reason: nadie sube un hallazgo sin decir por que"},
		{"razon vacia", qcSinCorrida, []string{id, "--reason", "   "}, id, "falta --reason: nadie sube un hallazgo sin decir por que"},
		{"id que no existe", qcSinCorrida, []string{"20260101T000000_ffffff", "--reason", "x"}, "20260101T000000_ffffff",
			"hallazgo no encontrado: 20260101T000000_ffffff"},
		{"cerrado", qcSinCorrida, []string{cerrado, "--reason", "x"}, cerrado,
			"el hallazgo " + cerrado + " ya esta resuelto como \"refutado\": un hallazgo cerrado no se sube"},
	} {
		res := qcCorrer(t, bin, dir, c.env, append([]string{"finding", "raise"}, c.args...)...)
		if res.exit != 1 || !strings.Contains(res.stderr, c.want) {
			t.Fatalf("CA-625: %s sale 1 con %q: %d\n%s", c.caso, c.want, res.exit, res.todo())
		}
		crSinRaise(t, "CA-625", c.caso, dir, c.id)
	}
}

// CA-625: el subcomando desconocido nombra (add|resolve|raise|list).
func TestCA625_SubcomandoDesconocidoNombraRaise(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	res := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "subir")
	if res.exit == 0 || !strings.Contains(res.todo(), "(add|resolve|raise|list)") {
		t.Fatalf("CA-625: el error nombra (add|resolve|raise|list): %d\n%s", res.exit, res.todo())
	}
}

func crLineaDe(out, id string) string {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, id) {
			return l
		}
	}
	return ""
}

// CA-628 (y CA-626): `hoom finding list` muestra `bloquea: <motivo>` o
// `pendiente` en cada abierto; un low subido bloquea con `subido a mano por
// <autor>`; el JSON trae bloquea y motivo (falso y "" en los cerrados).
func TestCA628_FindingListBloqueaOPendiente(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	alto := crHallazgoSev(t, bin, dir, "high", "app.go")
	bajo := crHallazgoSev(t, bin, dir, "low", "app.go")
	subido := crHallazgoSev(t, bin, dir, "low", "app.go")
	ruta := crHallazgoSev(t, bin, dir, "medium", "internal/auth/login.go")
	cerrado := crHallazgoSev(t, bin, dir, "high", "app.go")
	if r := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "raise", subido, "--reason", "toca el cobro"); r.exit != 0 {
		t.Fatalf("fixture: raise salio %d\n%s", r.exit, r.todo())
	}
	if r := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "resolve", cerrado, "--as", "refutado",
		"--evidence", "el test TestX lo tumba"); r.exit != 0 {
		t.Fatalf("fixture: resolve salio %d\n%s", r.exit, r.todo())
	}
	res := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "list")
	if res.exit != 0 {
		t.Fatalf("CA-628: finding list sale 0: %d\n%s", res.exit, res.todo())
	}
	for id, want := range map[string]string{
		alto:   "bloquea: severidad high",
		bajo:   "pendiente",
		subido: "bloquea: subido a mano por ",
		ruta:   "bloquea: medium en ruta de riesgo: internal/auth/login.go",
	} {
		if l := crLineaDe(res.stdout, id); !strings.Contains(l, want) {
			t.Fatalf("CA-628: la linea de %s dice %q: %q\n%s", id, want, l, res.stdout)
		}
	}
	if l := crLineaDe(res.stdout, bajo); strings.Contains(l, "bloquea:") {
		t.Fatalf("CA-628: un low sin subir no dice bloquea: %q", l)
	}
	if l := crLineaDe(res.stdout, cerrado); strings.Contains(l, "bloquea:") || strings.Contains(l, "pendiente") {
		t.Fatalf("CA-628: un cerrado no dice bloquea ni pendiente: %q", l)
	}

	res = qcCorrer(t, bin, dir, qcSinCorrida, "finding", "list", "--json")
	var v struct {
		Warnings []string         `json:"warnings"`
		Findings []map[string]any `json:"findings"`
	}
	if res.exit != 0 || json.Unmarshal([]byte(res.stdout), &v) != nil {
		t.Fatalf("CA-628: finding list --json: %d\n%s", res.exit, res.todo())
	}
	for _, f := range v.Findings {
		id := f["id"]
		b, _ := f["bloquea"].(bool)
		mo, hay := f["motivo"].(string)
		if !hay {
			t.Fatalf("CA-628: cada hallazgo trae motivo: %v", f)
		}
		switch id {
		case alto, subido, ruta:
			if !b || mo == "" {
				t.Fatalf("CA-628: %v bloquea con motivo: %v", id, f)
			}
		case bajo, cerrado:
			if b || mo != "" {
				t.Fatalf("CA-628: %v no bloquea y motivo vacio: %v", id, f)
			}
		}
	}
}

// CA-628 y CA-626: los avisos de subidas invalidas salen en la lista y en sus
// warnings, y no rompen la lectura (sale 0 y lista los hallazgos).
func TestCA628_AvisosDeSubidasInvalidas(t *testing.T) {
	bin := qcHoom(t)
	dir := qcRepo(t)
	a := crHallazgoSev(t, bin, dir, "low", "app.go")
	b := crHallazgoSev(t, bin, dir, "low", "app.go")
	fdir := filepath.Join(dir, ".hoom", "findings")
	escribir := func(nombre, body string) {
		if err := os.WriteFile(filepath.Join(fdir, nombre), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escribir(a+".raise.json", `{"finding_id":"`+b+`","reason":"x","author":"Ana","raised_at":"2026-11-01T00:00:00Z"}`)
	escribir("20260101T000000_eeeeee.raise.json", `{"finding_id":"20260101T000000_eeeeee","reason":"x","author":"Ana","raised_at":"2026-11-01T00:00:00Z"}`)
	escribir(b+".raise.json", `{no es json`)

	res := qcCorrer(t, bin, dir, qcSinCorrida, "finding", "list")
	if res.exit != 0 {
		t.Fatalf("CA-626: una subida invalida no rompe la lista: %d\n%s", res.exit, res.todo())
	}
	for _, w := range []string{"subida invalida", "subida huerfana", "subida ilegible"} {
		if !strings.Contains(res.todo(), w) {
			t.Fatalf("CA-628: la lista avisa %q:\n%s", w, res.todo())
		}
	}
	for _, id := range []string{a, b} {
		if l := crLineaDe(res.stdout, id); !strings.Contains(l, "pendiente") || strings.Contains(l, "bloquea:") {
			t.Fatalf("CA-626: una subida invalida no sube %s: %q\n%s", id, l, res.stdout)
		}
	}
	res = qcCorrer(t, bin, dir, qcSinCorrida, "finding", "list", "--json")
	var v struct {
		Warnings []string `json:"warnings"`
	}
	if res.exit != 0 || json.Unmarshal([]byte(res.stdout), &v) != nil {
		t.Fatalf("CA-628: finding list --json: %d\n%s", res.exit, res.todo())
	}
	todas := strings.Join(v.Warnings, "\n")
	for _, w := range []string{"subida invalida", "subida huerfana", "subida ilegible"} {
		if !strings.Contains(todas, w) {
			t.Fatalf("CA-628: warnings trae %q: %v", w, v.Warnings)
		}
	}
}
