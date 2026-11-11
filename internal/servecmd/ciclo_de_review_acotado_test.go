// Tests adversariales del spec .hoom/specs/ciclo-de-review-acotado.md sobre
// el Studio: GET /api/board y GET /api/board/{slug} traen `pendientes` en la
// tarjeta (CA-629), GET /api/findings trae `bloquea` y `motivo` en cada
// hallazgo con el umbral del manifiesto (CA-628), y el detalle de la tarjeta
// de tablero.js pinta la linea de pendientes como texto y solo cuando no esta
// vacia (CA-630, estatico como los de C2 y C3). Escritos desde el spec y las
// firmas publicas, sin leer la implementacion.
package servecmd

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/item"
)

// crPendientesDe busca, en un JSON cualquiera, el objeto de la tarjeta slug
// (el que tiene "slug": slug y "pendientes") y devuelve su pendientes.
func crPendientesDe(v any, slug string) (map[string]any, bool) {
	switch x := v.(type) {
	case map[string]any:
		if x["slug"] == slug {
			if p, ok := x["pendientes"].(map[string]any); ok {
				return p, true
			}
		}
		for _, e := range x {
			if p, ok := crPendientesDe(e, slug); ok {
				return p, true
			}
		}
	case []any:
		for _, e := range x {
			if p, ok := crPendientesDe(e, slug); ok {
				return p, true
			}
		}
	}
	return nil, false
}

// crRevisarPendientes exige la forma del contrato: medium, low, ids (nunca
// null) y texto, vacios para una tarjeta sin hallazgos.
func crRevisarPendientes(t *testing.T, ca, path string, body []byte, slug string) {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("%s: %s responde JSON: %v", ca, path, err)
	}
	p, ok := crPendientesDe(v, slug)
	if !ok {
		t.Fatalf("%s: %s trae pendientes en la tarjeta %s:\n%s", ca, path, slug, body)
	}
	ids, esLista := p["ids"].([]any)
	if p["medium"] != float64(0) || p["low"] != float64(0) || !esLista || len(ids) != 0 || p["texto"] != "" {
		t.Fatalf("%s: %s: sin hallazgos, pendientes es {medium 0, low 0, ids [], texto \"\"}: %v", ca, path, p)
	}
}

// CA-629: GET /api/board y GET /api/board/{slug} traen pendientes.
func TestCA629_APIBoardTraePendientes(t *testing.T) {
	dir := foProyectoStudio(t, "", "findings:\n  block_on: high\n")
	if _, err := item.Add(dir, item.Draft{Titulo: "Lista", Slug: "lista"}); err != nil {
		t.Fatalf("fixture: item.Add: %v", err)
	}
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-m", "item")
	s := newServer(t, dir)
	for _, path := range []string{"/api/board", "/api/board/lista"} {
		rec := foGET(t, s, path)
		if rec.Code != 200 {
			t.Fatalf("CA-629: %s responde 200: %d %s", path, rec.Code, rec.Body)
		}
		crRevisarPendientes(t, "CA-629", path, rec.Body.Bytes(), "lista")
	}
}

// CA-628: GET /api/findings trae bloquea y motivo en cada hallazgo, con el
// umbral del manifiesto; falso y vacio en los cerrados; warnings nunca null.
func TestCA628_APIFindingsTraeBloqueaYMotivo(t *testing.T) {
	dir := foProyectoStudio(t, "", "findings:\n  block_on: medium\n")
	alto, err := finding.Add(dir, "main", "high", "risk", "a.go", "hallazgo alto", "reviewer@test")
	if err != nil {
		t.Fatal(err)
	}
	medio, _ := finding.Add(dir, "main", "medium", "risk", "a.go", "hallazgo medio", "reviewer@test")
	bajo, _ := finding.Add(dir, "main", "low", "risk", "a.go", "hallazgo bajo", "reviewer@test")
	cerrado, _ := finding.Add(dir, "main", "high", "risk", "a.go", "hallazgo cerrado", "reviewer@test")
	if _, err := finding.Resolve(dir, cerrado.ID, "corregido", "el commit abc lo arregla", "Ana"); err != nil {
		t.Fatal(err)
	}
	s := newServer(t, dir)
	rec := foGET(t, s, "/api/findings")
	var v struct {
		Warnings []string         `json:"warnings"`
		Findings []map[string]any `json:"findings"`
	}
	if rec.Code != 200 || json.Unmarshal(rec.Body.Bytes(), &v) != nil || v.Warnings == nil {
		t.Fatalf("CA-628: /api/findings responde la vista con warnings: %d %s", rec.Code, rec.Body)
	}
	quiero := map[string][2]any{
		alto.ID:    {true, "severidad high"},
		medio.ID:   {true, "severidad medium"},
		bajo.ID:    {false, ""},
		cerrado.ID: {false, ""},
	}
	for _, f := range v.Findings {
		id, _ := f["id"].(string)
		q, ok := quiero[id]
		if !ok {
			continue
		}
		delete(quiero, id)
		if f["bloquea"] != q[0] || f["motivo"] != q[1] {
			t.Fatalf("CA-628: %s trae bloquea %v y motivo %q: %v %v", id, q[0], q[1], f["bloquea"], f["motivo"])
		}
	}
	if len(quiero) != 0 {
		t.Fatalf("CA-628: /api/findings lista todos los hallazgos: faltan %v", quiero)
	}
	cli, err := finding.JSONBytesConUmbral(dir, "main", "medium", false)
	if err != nil || strings.TrimSpace(string(cli)) != strings.TrimSpace(rec.Body.String()) {
		t.Fatalf("CA-628: /api/findings es lo que emite `hoom finding list --json` (JSONBytesConUmbral con el umbral del manifiesto):\napi: %s\ncli: %s", rec.Body, cli)
	}
}

// CA-630: el detalle de la tarjeta del Studio pinta la linea de pendientes:
// tablero.js lee pendientes y su texto en una funcion, lo pinta como texto
// (esc() o textContent, nunca HTML crudo) y solo cuando no esta vacio.
func TestCA630_StudioPintaLosPendientesComoTexto(t *testing.T) {
	_, js := tbUI(t)
	if len(huFueraDeComentarios(js, "pendientes")) == 0 {
		t.Fatal("CA-630: tablero.js usa pendientes de la tarjeta, en su codigo y no solo en un comentario")
	}
	comoTexto := regexp.MustCompile(`esc\([^()]*texto|textContent\s*=[^;\n]*texto|createTextNode\([^()]*texto|innerText\s*=[^;\n]*texto`)
	condicional := regexp.MustCompile(`if\s*\([^)]*texto|texto\s*\?|texto\s*&&|texto\s*!==?\s*["'` + "`" + `]{2}|texto\.length`)
	crudo := regexp.MustCompile(`innerHTML\s*[+]?=[^;\n]*pendientes[^;\n]*texto`)
	encontrada := false
	for nombre, cuerpo := range huCuerpos(js) {
		if !huPalabra(cuerpo, "pendientes") || !huPalabra(cuerpo, "texto") {
			continue
		}
		encontrada = true
		if !comoTexto.MatchString(cuerpo) {
			t.Fatalf("CA-630: %s pinta pendientes.texto como texto (esc() o textContent)", nombre)
		}
		if !condicional.MatchString(cuerpo) {
			t.Fatalf("CA-630: %s pinta la linea solo cuando texto no esta vacio", nombre)
		}
		if m := crudo.FindString(cuerpo); m != "" && !strings.Contains(m, "esc(") {
			t.Fatalf("CA-630: %s mete pendientes.texto en innerHTML sin esc(): %s", nombre, m)
		}
	}
	if !encontrada {
		t.Fatal("CA-630: una funcion con nombre de tablero.js lee pendientes y su texto")
	}
}
