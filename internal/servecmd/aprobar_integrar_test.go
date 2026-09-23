// Tests adversariales del spec .hoom/specs/acciones-desde-la-tarjeta.md
// (CA-342, CA-343): aprobar e integrar desde la tarjeta son los endpoints de
// hoy. approve gana un cuerpo opcional y estricto {"card"} que resuelve el
// spec en el espacio de trabajo de la tarjeta y firma con la identidad git de
// ese arbol (nunca un aprobador que venga en el cuerpo); done no cambia: sus
// condiciones son las de hoom task done.
package servecmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/approval"
	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/taskcmd"
)

// lnCartaTuAprobacion: espacio de trabajo con un spec que pasa lint y nadie
// aprobo (C1: tu-aprobacion). El espacio de trabajo tiene una identidad git
// propia, distinta de la del proyecto: la firma tiene que salir de su arbol.
func lnCartaTuAprobacion(t *testing.T, ca, root, slug string) string {
	t.Helper()
	tbItem(t, root, slug, "")
	wt := lnTarea(t, ca, root, slug)
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", tbSpecTexto("- CA-904: todavia nadie lo cita.", true))
	gitRun(t, root, "config", "extensions.worktreeConfig", "true")
	gitRun(t, wt, "config", "--worktree", "user.name", "Persona del Worktree")
	gitRun(t, wt, "config", "--worktree", "user.email", "wt@hoom.dev")
	if gitx.Identity(wt) == gitx.Identity(root) {
		t.Fatalf("%s: fixture: el espacio de trabajo tiene su propia identidad: %q", ca, gitx.Identity(wt))
	}
	lnColumna(t, ca, root, slug, boardcmd.ColTuAprobacion)
	return wt
}

func lnAprobar(t *testing.T, s *Server, name, token, body string) (int, string, []byte) {
	t.Helper()
	var raw []byte
	if body != "" {
		raw = []byte(body)
	}
	rec := doPOST(t, s, "/api/specs/"+name+"/approve", token, raw, "application/json")
	return rec.Code, rec.Header().Get("Content-Type"), rec.Body.Bytes()
}

type lnAprobado struct {
	Approval *approval.Record `json:"approval"`
	Already  *bool            `json:"already"`
}

func lnAprobaciones(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".hoom", "approvals"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		out = append(out, e.Name())
	}
	return out
}

// CA-342: con {"card": slug}, la aprobacion queda en el arbol de evidencia
// de la tarjeta (su espacio de trabajo) con approved_by = gitx.Identity de
// ese arbol, la respuesta es la de hoy y la tarjeta sale de Tu aprobacion.
// Aprobar otra vez la misma tarjeta, que ya no esta ahi, es 409.
func TestCA342_AprobarDesdeLaTarjetaFirmaEnSuArbol(t *testing.T) {
	root := tbProyecto(t)
	wt := lnCartaTuAprobacion(t, "CA-342", root, "aprobame")
	s := newServer(t, root)

	code, _, body := lnAprobar(t, s, "aprobame", s.Token(), `{"card": "aprobame"}`)
	if code != http.StatusOK {
		t.Fatalf("CA-342: aprobar con card responde 200, fue %d: %s", code, body)
	}
	var resp lnAprobado
	if err := json.Unmarshal(body, &resp); err != nil || resp.Approval == nil || resp.Already == nil {
		t.Fatalf("CA-342: la respuesta es la de hoy, {approval, already}: %v %s", err, body)
	}
	firma := gitx.Identity(wt)
	if resp.Approval.ApprovedBy != firma || *resp.Already {
		t.Fatalf("CA-342: approved_by es la identidad git del espacio de trabajo (%q), y es nueva: %+v already=%v",
			firma, resp.Approval, *resp.Already)
	}
	if resp.Approval.Spec != ".hoom/specs/aprobame.md" {
		t.Fatalf("CA-342: es approval.Approve(<arbol>, <spec>): el spec relativo a ese arbol: %q", resp.Approval.Spec)
	}

	// en el arbol de la tarjeta, y solo ahi
	if ap := lnAprobaciones(t, wt); len(ap) != 1 || !strings.HasPrefix(ap[0], "aprobame_") {
		t.Fatalf("CA-342: la aprobacion queda en .hoom/approvals/ del espacio de trabajo: %v", ap)
	}
	if ap := lnAprobaciones(t, root); len(ap) != 0 {
		t.Fatalf("CA-342: el arbol del proyecto no gana aprobaciones: %v", ap)
	}
	spec := filepath.Join(wt, ".hoom", "specs", "aprobame.md")
	if state, rec, err := approval.Status(wt, spec); err != nil || state != approval.StatusApproved || rec == nil || rec.ApprovedBy != firma {
		t.Fatalf("CA-342: hoom spec status en ese arbol dice aprobado por %q: %q %+v %v", firma, state, rec, err)
	}

	// la tarjeta sale de Tu aprobacion (su criterio no tiene test: test-writer)
	c := lnColumna(t, "CA-342", root, "aprobame", boardcmd.ColTestWriter)

	// y una segunda firma con la misma tarjeta ya no encuentra la accion
	otra, ct, cuerpo := lnAprobar(t, s, "aprobame", s.Token(), `{"card": "aprobame"}`)
	var e map[string]string
	if otra != http.StatusConflict || !strings.Contains(ct, "application/json") || json.Unmarshal(cuerpo, &e) != nil || e["error"] != lnYaNoEsta(c) {
		t.Fatalf("CA-342: aprobar una tarjeta que ya no esta en Tu aprobacion es 409 con %q: %d %s", lnYaNoEsta(c), otra, cuerpo)
	}
	if ap := lnAprobaciones(t, wt); len(ap) != 1 {
		t.Fatalf("CA-342: el 409 no escribe otra aprobacion: %v", ap)
	}
}

// CA-342: name distinto del slug (400), la tarjeta fuera de Tu aprobacion
// (409 con el motivo), una clave approved_by (400) y sin token (401): en
// ninguno se escribe una aprobacion, ni en el proyecto ni en el espacio de
// trabajo.
func TestCA342_AprobarRechazosSinFirma(t *testing.T) {
	root := tbProyecto(t)
	wt := lnCartaTuAprobacion(t, "CA-342", root, "aprobame")
	// un spec del proyecto con el nombre equivocado de la ruta
	tbEscribir(t, root, ".hoom/specs/otra.md", tbSpecTexto("- CA-908: otra cosa.", true))
	// una tarjeta en Arquitecto (su spec no pasa lint), en el arbol del proyecto
	tbItem(t, root, "borrador", "")
	tbEscribir(t, root, ".hoom/specs/borrador.md", tbSpecTexto("- CA-905: incompleto.", false))
	borrador := lnColumna(t, "CA-342", root, "borrador", boardcmd.ColArquitecto)
	s := newServer(t, root)

	sinFirma := func(que string) {
		t.Helper()
		for _, d := range []string{root, wt} {
			if ap := lnAprobaciones(t, d); len(ap) != 0 {
				t.Fatalf("CA-342: %s escribio una aprobacion en %s: %v", que, d, ap)
			}
		}
	}
	casos := []struct {
		que, name, token, body string
		code                   int
		msg                    string
	}{
		{"sin token", "aprobame", "", `{"card": "aprobame"}`, http.StatusUnauthorized, ""},
		{"con un token equivocado", "aprobame", "token-falso", `{"card": "aprobame"}`, http.StatusUnauthorized, ""},
		{"name distinto del slug", "otra", "*", `{"card": "aprobame"}`, http.StatusBadRequest, ""},
		{"una clave approved_by", "aprobame", "*", `{"card": "aprobame", "approved_by": "Otra Persona <otra@hoom.dev>"}`, http.StatusBadRequest, ""},
		{"una clave desconocida", "aprobame", "*", `{"card": "aprobame", "firmante": "hoom"}`, http.StatusBadRequest, ""},
		{"card de otro tipo", "aprobame", "*", `{"card": 7}`, http.StatusBadRequest, ""},
		{"la tarjeta en Arquitecto", "borrador", "*", `{"card": "borrador"}`, http.StatusConflict, lnYaNoEsta(borrador)},
	}
	for _, k := range casos {
		token := k.token
		if token == "*" {
			token = s.Token()
		}
		rec := doPOST(t, s, "/api/specs/"+k.name+"/approve", token, []byte(k.body), "application/json")
		msg := tbErrorJSON(t, "CA-342 ("+k.que+")", rec, k.code)
		if k.msg != "" && msg != k.msg {
			t.Fatalf("CA-342: %s responde %d con\n  %q\nfue\n  %q", k.que, k.code, k.msg, msg)
		}
		sinFirma(k.que)
	}
	// y la tarjeta sigue esperando la firma
	lnColumna(t, "CA-342", root, "aprobame", boardcmd.ColTuAprobacion)
}

// CA-342: sin cuerpo el endpoint hace lo de hoy: aprueba el spec del arbol
// del proyecto con la identidad del proyecto, aunque exista una tarjeta del
// mismo nombre con su propio espacio de trabajo.
func TestCA342_SinCuerpoApruebaComoHoy(t *testing.T) {
	root := tbProyecto(t)
	wt := lnCartaTuAprobacion(t, "CA-342", root, "doble")
	tbEscribir(t, root, ".hoom/specs/doble.md", tbSpecTexto("- CA-909: la version del proyecto.", true))
	s := newServer(t, root)

	code, _, body := lnAprobar(t, s, "doble", s.Token(), "")
	if code != http.StatusOK {
		t.Fatalf("CA-342: sin cuerpo, aprobar responde 200 como hoy: %d %s", code, body)
	}
	var resp lnAprobado
	if err := json.Unmarshal(body, &resp); err != nil || resp.Approval == nil || resp.Already == nil {
		t.Fatalf("CA-342: sin cuerpo, la respuesta de hoy: %v %s", err, body)
	}
	if resp.Approval.ApprovedBy != gitx.Identity(root) {
		t.Fatalf("CA-342: sin cuerpo firma la identidad del proyecto, como hoy: %q", resp.Approval.ApprovedBy)
	}
	if ap := lnAprobaciones(t, root); len(ap) != 1 || !strings.HasPrefix(ap[0], "doble_") {
		t.Fatalf("CA-342: sin cuerpo la aprobacion queda en el proyecto: %v", ap)
	}
	if ap := lnAprobaciones(t, wt); len(ap) != 0 {
		t.Fatalf("CA-342: sin cuerpo no se toca el espacio de trabajo de la tarjeta: %v", ap)
	}
}

// CA-343: una tarjeta en Tu aceptacion integrada con POST
// /api/tasks/{slug}/done pasa a Hecho con hecho_en y commit_final (la punta
// de su rama) en el item. Fixture de C1: lista (CA-278).
func TestCA343_IntegrarPasaAHecho(t *testing.T) {
	dir, _ := tbTablero(t)
	s := newServer(t, dir)
	lnColumna(t, "CA-343", dir, "lista", boardcmd.ColTuAceptacion)
	out, err := exec.Command("git", "-C", dir, "rev-parse", "hoom/lista").Output()
	if err != nil {
		t.Fatalf("CA-343: fixture: la rama hoom/lista: %v", err)
	}
	punta := strings.TrimSpace(string(out))

	rec := doPOST(t, s, "/api/tasks/lista/done", s.Token(), nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("CA-343: integrar desde Tu aceptacion responde 200: %d %s", rec.Code, rec.Body.String())
	}
	var ok map[string]bool
	if err := json.Unmarshal(rec.Body.Bytes(), &ok); err != nil || !ok["ok"] {
		t.Fatalf("CA-343: la respuesta de done no cambia ({\"ok\": true}): %s", rec.Body.String())
	}
	it, err := item.Load(dir, "lista")
	if err != nil {
		t.Fatalf("CA-343: el item sigue siendo valido: %v", err)
	}
	if it.HechoEn == nil || it.CommitFinal != punta {
		t.Fatalf("CA-343: el item gana hecho_en y commit_final = %s: hecho_en=%v commit_final=%q", punta, it.HechoEn, it.CommitFinal)
	}
	c := lnCarta(t, dir, "lista")
	if c.Column != boardcmd.ColHecho {
		t.Fatalf("CA-343: la tarjeta pasa a Hecho, esta en %s (%s)", c.Column, c.Plain)
	}
	if _, tiene := lnAccion(c, boardcmd.ActIntegrar); tiene {
		t.Fatalf("CA-343: en Hecho no se integra: %+v", c.Actions)
	}
}

// CA-343: done sobre una tarjeta en Writer es 409 con el mensaje de hoom
// task done, y su item y su espacio de trabajo quedan igual.
func TestCA343_IntegrarEnWriterNoCambiaNada(t *testing.T) {
	root := tbProyecto(t)
	wt := lnCartaWriter(t, "CA-343", root, "a-medias", "")
	itemPath := filepath.Join(root, ".hoom", "items", "a-medias.yaml")
	itemAntes, err := os.ReadFile(itemPath)
	if err != nil {
		t.Fatal(err)
	}
	fotoAntes := tbFoto(t, wt)
	want := taskcmd.Ready(root, "a-medias", lnBase) // lo que task done dice antes de tocar nada
	if want == nil {
		t.Fatal("CA-343: fixture: hoom task done rechaza una tarjeta en Writer")
	}
	s := newServer(t, root)

	msg := tbErrorJSON(t, "CA-343", doPOST(t, s, "/api/tasks/a-medias/done", s.Token(), nil, ""), http.StatusConflict)
	if msg != want.Error() {
		t.Fatalf("CA-343: el 409 trae el mensaje de hoom task done, verbatim:\napi: %q\ncli: %q", msg, want.Error())
	}
	if raw, err := os.ReadFile(itemPath); err != nil || !bytes.Equal(raw, itemAntes) {
		t.Fatalf("CA-343: el item queda igual: %v\n%s", err, raw)
	}
	fotoDespues := tbFoto(t, wt)
	for k, v := range fotoAntes {
		if w, ok := fotoDespues[k]; !ok || w != v {
			t.Fatalf("CA-343: el espacio de trabajo queda igual: %s cambio o desaparecio", k)
		}
	}
	for k := range fotoDespues {
		if _, ok := fotoAntes[k]; !ok {
			t.Fatalf("CA-343: el espacio de trabajo queda igual: aparecio %s", k)
		}
	}
	c := lnColumna(t, "CA-343", root, "a-medias", boardcmd.ColWriter)
	if _, tiene := lnAccion(c, boardcmd.ActIntegrar); tiene {
		t.Fatalf("CA-343: integrar aparece solo en Tu aceptacion, no en Writer: %+v", c.Actions)
	}
}

// CA-343: integrar aparece solo en Tu aceptacion, y ahi es la accion
// principal (la primera), habilitada. Sobre el tablero de C1 entero.
func TestCA343_IntegrarSoloEnTuAceptacion(t *testing.T) {
	dir, _ := tbTablero(t)
	b, err := boardcmd.Build(dir, lnBase, tbBlockOn, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	enAceptacion := 0
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			a, tiene := lnAccion(c, boardcmd.ActIntegrar)
			if tiene != (col.ID == boardcmd.ColTuAceptacion) {
				t.Fatalf("CA-343: integrar aparece solo en Tu aceptacion: %s en %s tiene=%v", c.Slug, col.ID, tiene)
			}
			if col.ID != boardcmd.ColTuAceptacion {
				continue
			}
			enAceptacion++
			if len(c.Actions) == 0 || c.Actions[0].ID != boardcmd.ActIntegrar {
				t.Fatalf("CA-343: en Tu aceptacion integrar es la accion principal: %+v", c.Actions)
			}
			if !a.Enabled || a.Why != "" || a.Label != "Integrar" || a.Expert {
				t.Fatalf("CA-343: integrar de %s esta habilitada, con su etiqueta, fuera del modo experto: %+v", c.Slug, a)
			}
		}
	}
	if enAceptacion == 0 {
		t.Fatal("CA-343: fixture: el tablero de C1 tiene una tarjeta en Tu aceptacion (lista)")
	}
}
