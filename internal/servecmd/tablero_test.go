// Tests adversariales del spec .hoom/specs/tablero-de-solo-lectura.md
// (CA-316..CA-318): las dos lecturas nuevas del Studio. GET /api/board es
// hoom board --json byte a byte, GET /api/board/{slug} es el detalle de una
// tarjeta, y ninguna de las dos escribe nada ni acepta otro metodo. Los
// fixtures son los de C1 (items-y-columna-derivada.md), y cada uno cita el
// criterio de C1 que fija su columna.
package servecmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/hoomfs"
	"github.com/hoomdev/hoomai/internal/verdict"
)

const tbBlockOn = "high"

func tbEscribir(t *testing.T, dir, rel, body string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// tbProyecto: repo git en main con hoom.yaml (gate barato y findings con
// block_on high), el .gitignore canonico de .hoom y un commit inicial.
func tbProyecto(t *testing.T) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "init", "-b", "main")
	gitRun(t, dir, "config", "user.email", "test@hoom.dev")
	gitRun(t, dir, "config", "user.name", "hoom test")
	tbEscribir(t, dir, "hoom.yaml", "schema: hoom/v1\nproject: demo\nbase_branch: main\ngates:\n  test:\n    required: true\n    cmd: \"true\"\n"+
		"findings:\n  block_on: "+tbBlockOn+"\n")
	tbEscribir(t, dir, ".hoom/.gitignore", hoomfs.GitignoreBody())
	tbEscribir(t, dir, "app.go", "package app\n")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "inicial")
	return dir
}

func tbItem(t *testing.T, dir, slug, extra string) {
	t.Helper()
	tbEscribir(t, dir, ".hoom/items/"+slug+".yaml", "titulo: Tarjeta "+slug+"\ntipo: feature\nprioridad: media\n"+
		"creado_por: \"hoom test <test@hoom.dev>\"\ncreado_en: 2026-09-22T15:04:05Z\n"+extra)
}

// tbSpecTexto arma un spec con las 7 secciones (sin "riesgos" si se pide).
func tbSpecTexto(criterios string, conRiesgos bool) string {
	s := "# Spec\n\n## Objetivo\nx\n\n## No-goals\nx\n\n## Contratos\nx\n\n## Casos limite\nx\n\n" +
		"## Criterios de aceptacion\n\n" + criterios + "\n\n## Decisiones\nx\n"
	if conRiesgos {
		s += "\n## Riesgos\nx\n"
	}
	return s
}

func tbAprobar(t *testing.T, dir, slug string) {
	t.Helper()
	if _, _, err := approval.Approve(dir, filepath.Join(dir, ".hoom", "specs", slug+".md")); err != nil {
		t.Fatalf("no pude aprobar %s: %v", slug, err)
	}
}

func tbGatesVerdes() []verdict.GateResult {
	return []verdict.GateResult{
		{Name: "spec_lint", Required: true, Status: verdict.StatusPass},
		{Name: "spec_trace", Required: true, Status: verdict.StatusPass},
		{Name: "spec_approved", Required: true, Status: verdict.StatusPass},
		{Name: "findings_open", Required: true, Status: verdict.StatusPass},
		{Name: "test", Required: true, Status: verdict.StatusPass},
	}
}

// tbVeredicto escribe un veredicto verde de la tarjeta con la huella ACTUAL.
func tbVeredicto(t *testing.T, dir, slug string) *verdict.Verdict {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: time.Date(2026, 9, 22, 15, 30, 0, 0, time.UTC),
		Spec: ".hoom/specs/" + slug + ".md", Git: gitx.Snapshot(dir, "main"), Gates: tbGatesVerdes()}
	v.Finalize()
	if _, err := verdict.Write(dir, v); err != nil {
		t.Fatal(err)
	}
	return v
}

// tbTablero arma una tarjeta por columna con los fixtures de C1, y un item
// invalido. Devuelve el proyecto y la columna esperada de cada slug.
func tbTablero(t *testing.T) (string, map[string]string) {
	t.Helper()
	dir := tbProyecto(t)
	quiere := map[string]string{}

	// CA-269: sin .hoom/specs/S.md la columna es backlog
	tbItem(t, dir, "sin-spec", "")
	quiere["sin-spec"] = boardcmd.ColBacklog

	// CA-270: un spec sin la seccion "riesgos" es arquitecto
	tbItem(t, dir, "incompleto", "")
	tbEscribir(t, dir, ".hoom/specs/incompleto.md", tbSpecTexto("- CA-20: algo.", false))
	quiere["incompleto"] = boardcmd.ColArquitecto

	// CA-271: lint OK sin aprobacion es tu-aprobacion
	tbItem(t, dir, "sin-firma", "")
	tbEscribir(t, dir, ".hoom/specs/sin-firma.md", tbSpecTexto("- CA-30: algo.", true))
	quiere["sin-firma"] = boardcmd.ColTuAprobacion

	// CA-272: aprobado con un CA sin test es test-writer
	tbItem(t, dir, "sin-tests", "")
	tbEscribir(t, dir, ".hoom/specs/sin-tests.md", tbSpecTexto("- CA-40: nadie lo cita.", true))
	quiere["sin-tests"] = boardcmd.ColTestWriter

	// CA-273: todo trazado y sin veredicto de la tarjeta es writer
	tbItem(t, dir, "sin-veredicto", "")
	tbEscribir(t, dir, ".hoom/specs/sin-veredicto.md", tbSpecTexto("- CA-50: citado por un test.", true))
	quiere["sin-veredicto"] = boardcmd.ColWriter

	// CA-274..CA-277: verde de la tarjeta sin la tarea: review ("cerrar exige
	// la tarea", CA-277)
	tbItem(t, dir, "en-review", "")
	tbEscribir(t, dir, ".hoom/specs/en-review.md", tbSpecTexto("- CA-60: citado por un test.", true))
	quiere["en-review"] = boardcmd.ColReview

	tbEscribir(t, dir, "trazas_test.txt", "CA-50 y CA-60\n")

	// CA-279: un item con hecho_en es hecho aunque no tenga spec
	tbItem(t, dir, "cerrada", "hecho_en: 2026-09-23T10:00:00Z\ncommit_final: 3f2a1b4c5d6e7f8091a2b3c4d5e6f708192a3b4c\n")
	quiere["cerrada"] = boardcmd.ColHecho

	// CA-266, CA-288: un item invalido no es tarjeta, es un aviso
	tbEscribir(t, dir, ".hoom/items/malo.yaml", "titulo: Malo\ntipo: feature\nprioridad: media\n"+
		"creado_por: x\ncreado_en: 2026-09-22T15:04:05Z\ncolumna: hecho\n")

	// CA-278: con el worktree limpio, verde y todo cumplido: tu-aceptacion
	tbItem(t, dir, "lista", "")
	wt := filepath.Join(dir, ".hoom", "worktrees", "lista")
	gitRun(t, dir, "worktree", "add", "-q", "-b", "hoom/lista", wt, "main")
	tbEscribir(t, wt, ".hoom/specs/lista.md", tbSpecTexto("- CA-70: citado por un test.", true))
	tbEscribir(t, wt, "lista_test.go", "package app\n\n// CA-70\n")
	tbEscribir(t, wt, "lista.go", "package app\n\nfunc Lista() int { return 1 }\n")
	tbAprobar(t, wt, "lista")
	tbVeredicto(t, wt, "lista")
	gitRun(t, wt, "add", "-A")
	gitRun(t, wt, "commit", "-q", "-m", "lista para cerrar")
	quiere["lista"] = boardcmd.ColTuAceptacion

	// las aprobaciones y, al final, el veredicto (con la huella de todo lo anterior)
	for _, s := range []string{"sin-tests", "sin-veredicto", "en-review"} {
		tbAprobar(t, dir, s)
	}
	tbVeredicto(t, dir, "en-review")
	return dir, quiere
}

func tbGET(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

func tbErrorJSON(t *testing.T, ca string, rec *httptest.ResponseRecorder, code int) string {
	t.Helper()
	if rec.Code != code {
		t.Fatalf("%s: se esperaba %d, fue %d: %s", ca, code, rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("%s: el error es JSON (Content-Type %q)", ca, rec.Header().Get("Content-Type"))
	}
	var e map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &e); err != nil || e["error"] == "" {
		t.Fatalf("%s: el cuerpo es {\"error\": ...}: %q", ca, rec.Body.String())
	}
	return e["error"]
}

// CA-316: GET /api/board emite los mismos bytes que
// boardcmd.JSONBytes(boardcmd.Build(...)) sobre el mismo arbol (lo que
// imprime hoom board --json), con cada tarjeta en su columna segun C1
// (CA-269 backlog, CA-270 arquitecto, CA-271 tu-aprobacion, CA-272
// test-writer, CA-273 writer, CA-274..CA-277 review, CA-278 tu-aceptacion,
// CA-279 hecho), human solo en las dos humanas y el item invalido en warnings
// (CA-266, CA-288).
func TestCA316_ApiBoardEsHoomBoardJSON(t *testing.T) {
	dir, quiere := tbTablero(t)
	s := newServer(t, dir)
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-316: GET /api/board responde 200, respondio %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("CA-316: /api/board es JSON: %q", rec.Header().Get("Content-Type"))
	}
	b, err := boardcmd.Build(dir, "main", tbBlockOn, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	cli, err := boardcmd.JSONBytes(b)
	if err != nil {
		t.Fatal(err)
	}
	if got := bytes.TrimSpace(rec.Body.Bytes()); !bytes.Equal(got, bytes.TrimSpace(cli)) {
		t.Fatalf("CA-316: /api/board son los bytes de hoom board --json:\napi: %s\ncli: %s", got, cli)
	}

	var tab struct {
		Columns []struct {
			ID    string `json:"id"`
			Human bool   `json:"human"`
			Cards []struct {
				Slug          string `json:"slug"`
				Column        string `json:"column"`
				NeedsDecision bool   `json:"needs_decision"`
				Plain         string `json:"plain"`
			} `json:"cards"`
		} `json:"columns"`
		Warnings []string `json:"warnings"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &tab); err != nil {
		t.Fatal(err)
	}
	if len(tab.Columns) != 8 {
		t.Fatalf("CA-316: las ocho columnas: %d", len(tab.Columns))
	}
	vistas := map[string]string{}
	for _, col := range tab.Columns {
		humana := col.ID == boardcmd.ColTuAprobacion || col.ID == boardcmd.ColTuAceptacion
		if col.Human != humana {
			t.Fatalf("CA-316: human es true solo en tu-aprobacion y tu-aceptacion: %s human=%v", col.ID, col.Human)
		}
		for _, c := range col.Cards {
			if c.Column != col.ID {
				t.Fatalf("CA-316: la tarjeta %s esta bajo %s y dice %s", c.Slug, col.ID, c.Column)
			}
			vistas[c.Slug] = col.ID
			if c.Plain == "" || c.NeedsDecision != humana {
				t.Fatalf("CA-316: la tarjeta %s trae plain y needs_decision (%v): %+v", c.Slug, humana, c)
			}
		}
	}
	for slug, col := range quiere {
		if vistas[slug] != col {
			t.Errorf("CA-316: la tarjeta %s debe estar en %s, esta en %q", slug, col, vistas[slug])
		}
	}
	if len(vistas) != len(quiere) {
		t.Fatalf("CA-316: una tarjeta por item valido (%d), hubo %d: %v", len(quiere), len(vistas), vistas)
	}
	hayAviso := false
	for _, w := range tab.Warnings {
		if strings.Contains(w, "malo.yaml") {
			hayAviso = true
		}
	}
	if !hayAviso {
		t.Fatalf("CA-316: el item invalido aparece en warnings: %q", tab.Warnings)
	}
}

// CA-316: sin items, /api/board trae las ocho columnas vacias, igual que
// hoom board --json (CA-288).
func TestCA316_ApiBoardSinItems(t *testing.T) {
	dir := tbProyecto(t)
	s := newServer(t, dir)
	rec := tbGET(t, s, "/api/board")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-316: GET /api/board sin items responde 200: %d %s", rec.Code, rec.Body.String())
	}
	b, err := boardcmd.Build(dir, "main", tbBlockOn, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	cli, _ := boardcmd.JSONBytes(b)
	if !bytes.Equal(bytes.TrimSpace(rec.Body.Bytes()), bytes.TrimSpace(cli)) {
		t.Fatalf("CA-316: sin items, los mismos bytes que el CLI:\napi: %s\ncli: %s", rec.Body.String(), cli)
	}
	if strings.Contains(rec.Body.String(), "null") {
		t.Fatalf("CA-316: sin items ninguna lista es null: %s", rec.Body.String())
	}
}

// CA-317: GET /api/board/{slug} responde 200 con el detalle, cuya card es la
// de hoom item show <slug> --json (boardcmd.CardFor); ?diff=1 agrega diff.
// Fixtures de C1: en-review por CA-277 (verde sin la tarea), lista por
// CA-278 (tu-aceptacion, con worktree) y sin-spec por CA-269 (backlog).
func TestCA317_ApiBoardSlugEsElDetalle(t *testing.T) {
	dir, _ := tbTablero(t)
	s := newServer(t, dir)
	for _, slug := range []string{"en-review", "lista", "sin-spec"} {
		rec := tbGET(t, s, "/api/board/"+slug)
		if rec.Code != http.StatusOK {
			t.Fatalf("CA-317: GET /api/board/%s responde 200: %d %s", slug, rec.Code, rec.Body.String())
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			t.Fatalf("CA-317: el detalle es JSON: %v\n%s", err, rec.Body.String())
		}
		for _, k := range []string{"card", "spec", "criteria", "verdict", "fingerprint", "findings", "reviews", "work", "paths"} {
			if _, ok := m[k]; !ok {
				t.Fatalf("CA-317: al detalle de %s le falta %q: %s", slug, k, rec.Body.String())
			}
		}
		if _, ok := m["diff"]; ok {
			t.Fatalf("CA-317: sin ?diff=1 no hay diff: %s", rec.Body.String())
		}
		card, err := boardcmd.CardFor(dir, "main", tbBlockOn, slug, time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		cli, _ := json.Marshal(card)
		var a, b any
		_ = json.Unmarshal(m["card"], &a)
		_ = json.Unmarshal(cli, &b)
		sa, _ := json.Marshal(a)
		sb, _ := json.Marshal(b)
		if string(sa) != string(sb) {
			t.Fatalf("CA-317: la card del detalle de %s es la de item show:\napi: %s\ncli: %s", slug, sa, sb)
		}
	}

	// ?diff=1: con worktree, disponible; en el arbol, la nota
	var d struct {
		Diff *gitx.Diff `json:"diff"`
	}
	rec := tbGET(t, s, "/api/board/lista?diff=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-317: GET /api/board/lista?diff=1: %d %s", rec.Code, rec.Body.String())
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || d.Diff == nil || !d.Diff.Available || d.Diff.Base != "main" {
		t.Fatalf("CA-317: ?diff=1 con worktree trae el diff disponible: %v %s", err, rec.Body.String())
	}
	hayCodigo := false
	for _, f := range d.Diff.Files {
		if f.Path == "lista.go" {
			hayCodigo = true
		}
	}
	if !hayCodigo {
		t.Fatalf("CA-317: el diff de la tarea trae lista.go: %+v", d.Diff.Files)
	}
	d.Diff = nil
	rec = tbGET(t, s, "/api/board/en-review?diff=1")
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil || d.Diff == nil || d.Diff.Available ||
		d.Diff.Note != "sin espacio de trabajo de la tarea: no hay diff base...HEAD que mostrar" {
		t.Fatalf("CA-317: ?diff=1 sin worktree trae available false y la nota: %v %s", err, rec.Body.String())
	}
}

// CA-317: un slug con forma invalida es 400 JSON; un item inexistente o
// invalido es 404 JSON con el mensaje de CardFor. El item invalido es el
// fixture de C1 que va a warnings (CA-266, CA-288).
func TestCA317_ApiBoardSlugErrores(t *testing.T) {
	dir, _ := tbTablero(t)
	s := newServer(t, dir)
	for _, mal := range []string{"Precios", "con%20espacio", "-guion", "a_b", strings.Repeat("a", 65)} {
		tbErrorJSON(t, "CA-317 (slug "+mal+")", tbGET(t, s, "/api/board/"+mal), http.StatusBadRequest)
	}
	for _, slug := range []string{"no-existe", "malo"} {
		msg := tbErrorJSON(t, "CA-317 ("+slug+")", tbGET(t, s, "/api/board/"+slug), http.StatusNotFound)
		_, err := boardcmd.CardFor(dir, "main", tbBlockOn, slug, time.Now().UTC())
		if err == nil || msg != err.Error() {
			t.Fatalf("CA-317: el 404 de %s trae el mensaje de CardFor:\napi: %q\ncli: %v", slug, msg, err)
		}
	}
	// y con ?diff=1 el error es el mismo
	tbErrorJSON(t, "CA-317", tbGET(t, s, "/api/board/no-existe?diff=1"), http.StatusNotFound)
	tbErrorJSON(t, "CA-317", tbGET(t, s, "/api/board/Precios?diff=1"), http.StatusBadRequest)
}

// tbFoto fotografia TODO el arbol salvo .git/: ignorados incluidos
// (worktrees, runs, envelopes, cache). Contenido, modo y mtime.
func tbFoto(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			out[rel+"/"] = "dir"
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if d.Type()&fs.ModeSymlink != 0 {
			dest, _ := os.Readlink(p)
			out[rel] = "link " + dest
			return nil
		}
		if d.Name() == ".git" { // el archivo .git de un worktree apunta al repo: es de git
			return nil
		}
		raw, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = fmt.Sprintf("%v %x %d", info.Mode(), sha256.Sum256(raw), info.ModTime().UnixNano())
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func tbMismaFoto(t *testing.T, ca string, antes, despues map[string]string) {
	t.Helper()
	for k, v := range antes {
		if w, ok := despues[k]; !ok {
			t.Fatalf("%s: leer el tablero borro %s", ca, k)
		} else if w != v {
			t.Fatalf("%s: leer el tablero modifico %s", ca, k)
		}
	}
	for k := range despues {
		if _, ok := antes[k]; !ok {
			t.Fatalf("%s: leer el tablero creo %s", ca, k)
		}
	}
}

// CA-318: POST, PUT, PATCH y DELETE sobre /api/board y /api/board/{slug}
// son 405, con token o sin el; los GET no piden token; y pedir el tablero y
// el detalle (con diff) deja el repo y .hoom/ byte a byte iguales, ignorados
// incluidos. Sobre el tablero de C1 entero (CA-269..CA-279), con el worktree
// de lista (CA-278) para que el diff corra git de verdad.
func TestCA318_SoloLecturaSinEfectos(t *testing.T) {
	dir, _ := tbTablero(t)
	s := newServer(t, dir)
	antes := tbFoto(t, dir)

	for _, path := range []string{"/api/board", "/api/board/en-review", "/api/board/lista"} {
		for _, metodo := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
			for _, token := range []string{"", s.Token()} {
				req := httptest.NewRequest(metodo, path, strings.NewReader(`{"slug":"en-review","column":"hecho"}`))
				req.Header.Set("Content-Type", "application/json")
				if token != "" {
					req.Header.Set(TokenHeader, token)
				}
				rec := httptest.NewRecorder()
				s.Handler().ServeHTTP(rec, req)
				if rec.Code != http.StatusMethodNotAllowed {
					t.Fatalf("CA-318: %s %s (token=%v) es 405, fue %d: %s", metodo, path, token != "", rec.Code, rec.Body.String())
				}
			}
		}
	}

	// los GET, sin token
	for _, path := range []string{"/api/board", "/api/board/en-review", "/api/board/lista?diff=1",
		"/api/board/en-review?diff=1", "/api/board/sin-spec", "/api/board/no-existe", "/api/board/Mal"} {
		rec := tbGET(t, s, path)
		if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusMethodNotAllowed {
			t.Fatalf("CA-318: GET %s no pide token: %d", path, rec.Code)
		}
		if strings.HasPrefix(path, "/api/board/en-review") || path == "/api/board" || strings.HasPrefix(path, "/api/board/lista") {
			if rec.Code != http.StatusOK {
				t.Fatalf("CA-318: GET %s sin token responde 200: %d %s", path, rec.Code, rec.Body.String())
			}
		}
	}
	tbMismaFoto(t, "CA-318", antes, tbFoto(t, dir))
}
