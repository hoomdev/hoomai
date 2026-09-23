// Tests adversariales del spec .hoom/specs/historia-doctor-y-cinta.md
// (CA-365 y la parte del Studio de CA-376): las dos lecturas nuevas del
// Studio. GET /api/board/{slug}/timeline es boardcmd.TimelineFor sobre el
// mismo arbol, y GET /api/ratchet es statuscmd.Ratchet (la clave ratchet de
// hoom status --json). Las dos son lecturas: sin token, otro metodo es 405 y
// dejan el repo y .hoom/ byte a byte iguales, ignorados incluidos.
package servecmd

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/providers"
	"github.com/hoomdev/hoomai/internal/runcmd"
	"github.com/hoomdev/hoomai/internal/statuscmd"
)

// hsSlugsInvalidos son slugs con forma invalida (los de CA-317).
var hsSlugsInvalidos = []string{"Precios", "con%20espacio", "-guion", "a_b", strings.Repeat("a", 65)}

// hsProyecto arma un proyecto con historia de verdad:
//   - con-historia: el item guardado en la raiz, un espacio de trabajo con el
//     spec aprobado, tests, codigo y un veredicto verde, cada cosa en su
//     commit, y telemetria local (un sobre entregable y su run con costo y
//     una delegacion a un subagente emparejada por tool_id);
//   - recien-creada: un item sin commit, sin spec y sin espacio de trabajo;
//   - malo: un item invalido (no es tarjeta: CA-266, CA-288).
func hsProyecto(t *testing.T) string {
	t.Helper()
	root := tbProyecto(t)
	const slug = "con-historia"
	tbItem(t, root, slug, "presupuesto_usd: 5\n")
	lnCommit(t, root, "alta de la tarjeta con-historia")
	wt := lnTarea(t, "CA-365", root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-911: citado por un test.", true))
	tbAprobar(t, wt, slug)
	lnCommit(t, wt, "spec aprobado")
	tbEscribir(t, wt, lnTestDe(slug), "package app\n\n// CA-911\n")
	tbEscribir(t, wt, "con_historia.go", "package app\n\nfunc ConHistoria() int { return 1 }\n")
	lnCommit(t, wt, "tests y codigo")
	lnVeredicto(t, wt, slug, 0)
	lnCommit(t, wt, "veredicto")

	// telemetria local: el sobre del writer y su run, con costo y una delegacion
	inicio := time.Date(2026, 9, 23, 11, 0, 0, 0, time.UTC)
	fin := inicio.Add(20 * time.Minute)
	costo := 0.42
	runID, envID := "20260923T110001_hsrun1", "20260923T110000_hsenv1"
	runAjeno(t, root, runID,
		runcmd.Meta{ID: runID, Provider: "claude", Role: "writer", Task: slug, Dir: wt,
			CreatedAt: inicio.Add(time.Second), Status: runcmd.StatusDone, ExitCode: 0, EndedAt: fin,
			Usage: &providers.Usage{CostUSD: &costo, Turns: 3}},
		[]providers.Event{
			{TS: inicio.Add(2 * time.Second), Kind: "start", Detail: "run"},
			{TS: inicio.Add(3 * time.Second), Kind: "agent", Agent: "scout", ToolID: "toolu_hs01", Detail: "explora el paquete"},
			{TS: inicio.Add(90 * time.Second), Kind: "agent_end", Agent: "scout", ToolID: "toolu_hs01", Detail: "scout termino"},
			{TS: fin, Kind: "result", Detail: "listo"},
		})
	envelope.Write(root, envelope.Record{ID: envID, Role: "writer", Provider: "claude", Task: slug, Dir: wt,
		RunID: runID, Stage: "ok", Step: 5, Steps: 5, Status: envelope.StatusDeliverable, ExitCode: 0,
		StartedAt: inicio, EndedAt: fin})

	tbItem(t, root, "recien-creada", "")
	tbEscribir(t, root, ".hoom/items/malo.yaml", "titulo: Malo\ntipo: feature\nprioridad: media\n"+
		"creado_por: x\ncreado_en: 2026-09-22T15:04:05Z\ncolumna: hecho\n")
	return root
}

// hsPedir hace un pedido sin token al Handler (un panico es una falla del
// criterio, no la caida del paquete).
func hsPedir(t *testing.T, ca string, s *Server, metodo, path string) *httptest.ResponseRecorder {
	t.Helper()
	return atPedir(t, ca, s, metodo, path, "", "")
}

// hsIgualJSON compara dos JSON por su contenido, no por su formato.
func hsIgualJSON(t *testing.T, ca, que string, api []byte, esperado any) {
	t.Helper()
	cli, err := json.Marshal(esperado)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	if err := json.Unmarshal(api, &a); err != nil {
		t.Fatalf("%s: %s es JSON: %v\n%s", ca, que, err, api)
	}
	if err := json.Unmarshal(cli, &b); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) {
		sa, _ := json.Marshal(a)
		sb, _ := json.Marshal(b)
		t.Fatalf("%s: %s es el mismo JSON que la funcion de Go:\napi: %s\ngo:  %s", ca, que, sa, sb)
	}
}

// hsLista exige que la clave sea una lista JSON (nunca null).
func hsLista(t *testing.T, ca, que string, claves map[string]json.RawMessage, k string) []json.RawMessage {
	t.Helper()
	raw, ok := claves[k]
	if !ok {
		t.Fatalf("%s: a %s le falta la clave %q", ca, que, k)
	}
	var l []json.RawMessage
	if strings.TrimSpace(string(raw)) == "null" || json.Unmarshal(raw, &l) != nil || l == nil {
		t.Fatalf("%s: %s trae %q como lista, nunca null: %s", ca, que, k, raw)
	}
	return l
}

// hsRutaSoloGET exige que Routes() registre el patron GET y ningun otro
// metodo sobre esa ruta (la regla de oro se afirma sobre Routes, CA-352).
func hsRutaSoloGET(t *testing.T, ca, patron string) {
	t.Helper()
	_, ruta, _ := strings.Cut(patron, " ")
	hay := false
	for _, r := range Routes() {
		if r == patron {
			hay = true
			continue
		}
		if _, otra, _ := strings.Cut(r, " "); otra == ruta {
			t.Fatalf("%s: %s es una lectura: Routes() no registra %q", ca, ruta, r)
		}
	}
	if !hay {
		t.Fatalf("%s: Routes() lista %q: %q", ca, patron, Routes())
	}
}

// ---------------------------------------------------------------------------

// CA-365: GET /api/board/{slug}/timeline responde 200 con el JSON de
// boardcmd.TimelineFor(root, base, blockOn, slug, now) sobre el mismo arbol:
// una tarjeta con historia de git y telemetria, y una recien creada sin
// commits. Sus listas nunca son null.
func TestCA365_TimelineEsLaDeTimelineFor(t *testing.T) {
	root := hsProyecto(t)
	s := newServer(t, root)
	for _, slug := range []string{"con-historia", "recien-creada"} {
		path := "/api/board/" + slug + "/timeline"
		rec := hsPedir(t, "CA-365", s, http.MethodGet, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("CA-365: GET %s responde 200, respondio %d: %s", path, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
			t.Fatalf("CA-365: GET %s es JSON (Content-Type %q)", path, rec.Header().Get("Content-Type"))
		}
		var claves map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &claves); err != nil {
			t.Fatalf("CA-365: la historia es un objeto JSON: %v\n%s", err, rec.Body.String())
		}
		for _, k := range []string{"slug", "card", "meter", "entries", "telemetry", "notes"} {
			if _, ok := claves[k]; !ok {
				t.Fatalf("CA-365: a la historia de %s le falta %q: %s", slug, k, rec.Body.String())
			}
		}
		for _, k := range []string{"meter", "entries", "notes"} {
			hsLista(t, "CA-365", "la historia de "+slug, claves, k)
		}
		var cabeza struct {
			Slug string `json:"slug"`
			Card struct {
				Slug string `json:"slug"`
			} `json:"card"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &cabeza); err != nil || cabeza.Slug != slug || cabeza.Card.Slug != slug {
			t.Fatalf("CA-365: la historia es la de %s (slug y card.slug): %v %+v", slug, err, cabeza)
		}

		tl, err := boardcmd.TimelineFor(root, lnBase, tbBlockOn, slug, time.Now().UTC())
		if err != nil {
			t.Fatalf("CA-365: TimelineFor(%s) sobre el mismo arbol: %v", slug, err)
		}
		hsIgualJSON(t, "CA-365", "GET "+path, rec.Body.Bytes(), tl)
	}
}

// CA-365: un slug con forma invalida es 400 JSON; un item que no existe y
// uno invalido son 404 JSON con el mensaje de CardFor (el mismo de
// /api/board/{slug}).
func TestCA365_TimelineSlugEItem(t *testing.T) {
	root := hsProyecto(t)
	s := newServer(t, root)
	for _, mal := range hsSlugsInvalidos {
		tbErrorJSON(t, "CA-365 (slug "+mal+")", hsPedir(t, "CA-365", s, http.MethodGet, "/api/board/"+mal+"/timeline"),
			http.StatusBadRequest)
	}
	for _, slug := range []string{"no-existe", "malo"} {
		msg := tbErrorJSON(t, "CA-365 ("+slug+")", hsPedir(t, "CA-365", s, http.MethodGet, "/api/board/"+slug+"/timeline"),
			http.StatusNotFound)
		_, err := boardcmd.CardFor(root, lnBase, tbBlockOn, slug, time.Now().UTC())
		if err == nil || msg != err.Error() {
			t.Fatalf("CA-365: el 404 de %s trae el mensaje de CardFor:\napi: %q\ncli: %v", slug, msg, err)
		}
	}
}

// CA-365: la historia es una lectura. No pide token (los GET sin header
// responden), POST, PUT, PATCH y DELETE son 405 con token o sin el, la ruta
// se registra solo como GET, y pedirla (tambien sus errores) deja el repo y
// .hoom/ byte a byte iguales, ignorados incluidos (espacios de trabajo, runs,
// sobres, cache): git solo se lee.
func TestCA365_TimelineSoloLecturaSinEfectos(t *testing.T) {
	hsRutaSoloGET(t, "CA-365", "GET /api/board/{slug}/timeline")
	root := hsProyecto(t)
	s := newServer(t, root)
	antes := tbFoto(t, root)

	for i := 0; i < 2; i++ { // dos veces: la segunda no encuentra nada que "arreglar"
		for _, slug := range []string{"con-historia", "recien-creada"} {
			rec := hsPedir(t, "CA-365", s, http.MethodGet, "/api/board/"+slug+"/timeline")
			if rec.Code != http.StatusOK {
				t.Fatalf("CA-365: GET /api/board/%s/timeline sin token responde 200, respondio %d: %s", slug, rec.Code, rec.Body.String())
			}
		}
		for _, path := range []string{"/api/board/no-existe/timeline", "/api/board/malo/timeline", "/api/board/Mal/timeline"} {
			if rec := hsPedir(t, "CA-365", s, http.MethodGet, path); rec.Code == http.StatusUnauthorized || rec.Code == http.StatusMethodNotAllowed {
				t.Fatalf("CA-365: GET %s no pide token ni es 405: %d", path, rec.Code)
			}
		}
	}

	for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, token := range []string{"", s.Token()} {
			rec := atPedir(t, "CA-365", s, metodo, "/api/board/con-historia/timeline", token, `{"column": "hecho"}`)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("CA-365: %s /api/board/con-historia/timeline (token=%v) es 405, fue %d: %s",
					metodo, token != "", rec.Code, rec.Body.String())
			}
		}
	}
	tbMismaFoto(t, "CA-365", antes, tbFoto(t, root))
}

// --- el trinquete ---

// hsRatchetJSON es una linea base con tres metricas: cobertura (sube, con
// sus movimientos: congelada, apretada, aflojada con motivo y apretada de
// nuevo), complejidad (baja, congelada y apretada) y mutantes (declarada sin
// congelar, sin movimientos). El history del archivo va del mas viejo al
// mas nuevo, con las metricas intercaladas.
const hsRatchetJSON = `{
  "schema": "hoom.ratchet/v1",
  "metrics": {
    "cobertura": {"cmd": "echo 82", "direction": "up", "tolerance": 0.5, "value": 82, "updated_at": "2026-09-20T10:00:00Z"},
    "complejidad": {"cmd": "echo 10", "direction": "down", "value": 10, "updated_at": "2026-09-21T10:00:00Z"},
    "mutantes": {"cmd": "echo 0", "direction": "up"}
  },
  "history": [
    {"ts": "2026-09-01T10:00:00Z", "metric": "cobertura", "to": 70, "kind": "frozen"},
    {"ts": "2026-09-05T10:00:00Z", "metric": "cobertura", "from": 70, "to": 78, "kind": "tightened"},
    {"ts": "2026-09-08T10:00:00Z", "metric": "complejidad", "to": 12, "kind": "frozen"},
    {"ts": "2026-09-10T10:00:00Z", "metric": "cobertura", "from": 78, "to": 75, "kind": "loosened", "reason": "se borro un modulo con tests"},
    {"ts": "2026-09-20T10:00:00Z", "metric": "cobertura", "from": 75, "to": 82, "kind": "tightened"},
    {"ts": "2026-09-21T10:00:00Z", "metric": "complejidad", "from": 12, "to": 10, "kind": "tightened"}
  ]
}
`

type hsMovimiento struct {
	TS     time.Time `json:"ts"`
	Metric string    `json:"metric"`
	From   *float64  `json:"from"`
	To     float64   `json:"to"`
	Kind   string    `json:"kind"`
	Reason string    `json:"reason"`
}

func hsF(v float64) *float64 { return &v }

func hsDia(d int) time.Time { return time.Date(2026, 9, d, 10, 0, 0, 0, time.UTC) }

// hsMovimientosEsperados son los movimientos de cada metrica de
// hsRatchetJSON, del mas viejo al mas nuevo.
var hsMovimientosEsperados = map[string][]hsMovimiento{
	"cobertura": {
		{TS: hsDia(1), Metric: "cobertura", To: 70, Kind: "frozen"},
		{TS: hsDia(5), Metric: "cobertura", From: hsF(70), To: 78, Kind: "tightened"},
		{TS: hsDia(10), Metric: "cobertura", From: hsF(78), To: 75, Kind: "loosened", Reason: "se borro un modulo con tests"},
		{TS: hsDia(20), Metric: "cobertura", From: hsF(75), To: 82, Kind: "tightened"},
	},
	"complejidad": {
		{TS: hsDia(8), Metric: "complejidad", To: 12, Kind: "frozen"},
		{TS: hsDia(21), Metric: "complejidad", From: hsF(12), To: 10, Kind: "tightened"},
	},
	"mutantes": {},
}

// hsRatchet pide /api/ratchet sin token y exige 200 JSON.
func hsRatchet(t *testing.T, s *Server) *httptest.ResponseRecorder {
	t.Helper()
	rec := hsPedir(t, "CA-376", s, http.MethodGet, "/api/ratchet")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-376: GET /api/ratchet sin token responde 200, respondio %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("CA-376: /api/ratchet es JSON (Content-Type %q)", rec.Header().Get("Content-Type"))
	}
	return rec
}

// CA-376: con linea base, GET /api/ratchet es statuscmd.Ratchet(root) (lo
// mismo que la clave ratchet de hoom status --json): declared, cada metrica
// con su direccion y su base, y en history sus movimientos del mas viejo al
// mas nuevo (los aflojados incluidos, con su motivo), lista nunca null aun
// sin movimientos.
func TestCA376_ApiRatchetConLineaBase(t *testing.T) {
	root := tbProyecto(t)
	tbEscribir(t, root, ".hoom/ratchet.json", hsRatchetJSON)
	s := newServer(t, root)
	rec := hsRatchet(t, s)
	hsIgualJSON(t, "CA-376", "GET /api/ratchet", rec.Body.Bytes(), statuscmd.Ratchet(root))

	var vista struct {
		Declared *bool  `json:"declared"`
		Error    string `json:"error"`
		Metrics  []struct {
			Name      string          `json:"name"`
			Value     *float64        `json:"value"`
			Direction string          `json:"direction"`
			History   json.RawMessage `json:"history"`
		} `json:"metrics"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &vista); err != nil {
		t.Fatalf("CA-376: /api/ratchet es un RatchetView: %v\n%s", err, rec.Body.String())
	}
	if vista.Declared == nil || !*vista.Declared || vista.Error != "" {
		t.Fatalf("CA-376: con linea base legible, declared true y sin error: %s", rec.Body.String())
	}
	vistas := map[string]bool{}
	for _, m := range vista.Metrics {
		quiere, ok := hsMovimientosEsperados[m.Name]
		if !ok {
			t.Fatalf("CA-376: metrica inesperada %q: %s", m.Name, rec.Body.String())
		}
		vistas[m.Name] = true
		switch m.Name {
		case "cobertura":
			if m.Direction != "up" || m.Value == nil || *m.Value != 82 {
				t.Fatalf("CA-376: cobertura sube y su base vigente es 82: %+v", m)
			}
		case "complejidad":
			if m.Direction != "down" || m.Value == nil || *m.Value != 10 {
				t.Fatalf("CA-376: complejidad baja y su base vigente es 10: %+v", m)
			}
		case "mutantes":
			if m.Value != nil {
				t.Fatalf("CA-376: mutantes esta declarada sin congelar (sin value): %+v", m)
			}
		}
		if strings.TrimSpace(string(m.History)) == "null" || len(m.History) == 0 {
			t.Fatalf("CA-376: history de %s es una lista, nunca null: %s", m.Name, rec.Body.String())
		}
		var got []hsMovimiento
		if err := json.Unmarshal(m.History, &got); err != nil {
			t.Fatalf("CA-376: history de %s es una lista de movimientos: %v (%s)", m.Name, err, m.History)
		}
		if len(got) != len(quiere) {
			t.Fatalf("CA-376: history de %s trae sus %d movimientos del archivo: %s", m.Name, len(quiere), m.History)
		}
		for i := range quiere {
			g, q := got[i], quiere[i]
			mismoFrom := (g.From == nil) == (q.From == nil) && (g.From == nil || *g.From == *q.From)
			if !g.TS.Equal(q.TS) || g.Metric != q.Metric || !mismoFrom || g.To != q.To || g.Kind != q.Kind || g.Reason != q.Reason {
				t.Fatalf("CA-376: el movimiento %d de %s (del mas viejo al mas nuevo) es %+v, fue %+v", i, m.Name, q, g)
			}
		}
	}
	if len(vistas) != len(hsMovimientosEsperados) {
		t.Fatalf("CA-376: /api/ratchet trae las tres metricas declaradas: %s", rec.Body.String())
	}
}

// CA-376: sin .hoom/ratchet.json, declared false y metrics [] (una lista
// vacia, no null); con un archivo ilegible, declared true y error. En los
// dos casos, lo mismo que statuscmd.Ratchet(root).
func TestCA376_ApiRatchetSinLineaBaseEIlegible(t *testing.T) {
	sin := tbProyecto(t)
	rec := hsRatchet(t, newServer(t, sin))
	hsIgualJSON(t, "CA-376", "GET /api/ratchet sin linea base", rec.Body.Bytes(), statuscmd.Ratchet(sin))
	var claves map[string]json.RawMessage
	if err := json.Unmarshal(rec.Body.Bytes(), &claves); err != nil {
		t.Fatalf("CA-376: /api/ratchet es un objeto JSON: %v", err)
	}
	if strings.TrimSpace(string(claves["declared"])) != "false" {
		t.Fatalf("CA-376: sin linea base, declared false: %s", rec.Body.String())
	}
	if m := hsLista(t, "CA-376", "/api/ratchet sin linea base", claves, "metrics"); len(m) != 0 {
		t.Fatalf("CA-376: sin linea base, metrics []: %s", rec.Body.String())
	}

	ilegible := tbProyecto(t)
	tbEscribir(t, ilegible, ".hoom/ratchet.json", "{esto no es json\n")
	rec = hsRatchet(t, newServer(t, ilegible))
	hsIgualJSON(t, "CA-376", "GET /api/ratchet con el archivo ilegible", rec.Body.Bytes(), statuscmd.Ratchet(ilegible))
	var vista struct {
		Declared bool   `json:"declared"`
		Error    string `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &vista); err != nil || !vista.Declared || strings.TrimSpace(vista.Error) == "" {
		t.Fatalf("CA-376: con el archivo ilegible, declared true y error: %v %s", err, rec.Body.String())
	}
}

// CA-376: /api/ratchet es una lectura: la ruta se registra solo como GET, no
// pide token, POST, PUT, PATCH y DELETE son 405 con token o sin el, y no
// modifica nada (ni el archivo, ni .hoom/, ni ningun ignorado).
func TestCA376_ApiRatchetSoloLectura(t *testing.T) {
	hsRutaSoloGET(t, "CA-376", "GET /api/ratchet")
	root := tbProyecto(t)
	tbEscribir(t, root, ".hoom/ratchet.json", hsRatchetJSON)
	s := newServer(t, root)
	antes := tbFoto(t, root)
	hsRatchet(t, s)
	hsRatchet(t, s)
	for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		for _, token := range []string{"", s.Token()} {
			rec := atPedir(t, "CA-376", s, metodo, "/api/ratchet", token, `{"metric": "cobertura", "to": 0}`)
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("CA-376: %s /api/ratchet (token=%v) es 405, fue %d: %s", metodo, token != "", rec.Code, rec.Body.String())
			}
		}
	}
	tbMismaFoto(t, "CA-376", antes, tbFoto(t, root))
}
