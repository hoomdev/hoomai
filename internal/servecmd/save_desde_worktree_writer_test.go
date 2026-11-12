package servecmd

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/itemcmd"
)

// Findings d26397 and f4d55b (CA-345, Henry's decision): with `hoom serve`
// started in the task's worktree and the item in the main checkout, Guardar
// with the unsynced GET /api/board shows answers 200 and makes one commit,
// in the worktree, with only that; the main checkout's item stays for `hoom
// item save`. Another set is 409 with no commit.
func TestGuardarDesdeElServeDelWorktree(t *testing.T) {
	const ca = "CA-345 (hallazgos d26397, f4d55b)"
	dir := tbProyecto(t)
	const slug = "precios"
	tbItem(t, dir, slug, "")
	gitRun(t, dir, "add", "-A")
	gitRun(t, dir, "commit", "-q", "-m", "item")
	wt := filepath.Join(dir, ".hoom", "worktrees", slug)
	gitRun(t, dir, "worktree", "add", "-q", "-b", "hoom/"+slug, wt, "main")
	tbEscribir(t, wt, ".hoom/specs/"+slug+".md", "# spec\n")
	if _, err := itemcmd.Set(wt, "main", tbBlockOn, slug, "alto", ""); err != nil {
		t.Fatal(err)
	}

	s := newServer(t, wt)
	rec := tbGET(t, s, "/api/board")
	var b boardcmd.Board
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &b) != nil {
		t.Fatalf("%s: GET /api/board: %d %s", ca, rec.Code, rec.Body.String())
	}
	var unsynced []string
	for _, col := range b.Columns {
		for _, c := range col.Cards {
			if c.Slug == slug {
				unsynced = c.Unsynced
			}
		}
	}
	spec := ".hoom/specs/" + slug + ".md"
	if !atMismas(unsynced, []string{spec}) {
		t.Fatalf("%s: fixture: el tablero del worktree muestra %q", ca, unsynced)
	}
	headRaiz, headWt := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")

	rec = atPedir(t, ca, s, http.MethodPost, "/api/board/"+slug+"/save", s.Token(),
		atCuerpoPaths([]string{".hoom/items/" + slug + ".yaml", ".hoom/worktrees/" + slug + "/" + spec}))
	if msg := tbErrorJSON(t, ca, rec, http.StatusConflict); msg != itemcmd.ErrChanged {
		t.Fatalf("%s: otras rutas: %q", ca, msg)
	}
	if atGit(t, dir, "rev-parse", "HEAD") != headRaiz || atGit(t, wt, "rev-parse", "HEAD") != headWt {
		t.Fatalf("%s: con otras rutas ningun arbol gana un commit", ca)
	}

	rec = atPedir(t, ca, s, http.MethodPost, "/api/board/"+slug+"/save", s.Token(), atCuerpoPaths(unsynced))
	if rec.Code != http.StatusOK {
		t.Fatalf("%s: con las rutas de unsynced es 200: %d %s", ca, rec.Code, rec.Body.String())
	}
	var res itemcmd.SaveResult
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || len(res.Commits) != 1 {
		t.Fatalf("%s: un solo commit: %s", ca, rec.Body.String())
	}
	if atGit(t, wt, "rev-parse", "HEAD") != res.Commits[0].SHA || atGit(t, wt, "rev-parse", "--abbrev-ref", "HEAD") != "hoom/"+slug {
		t.Fatalf("%s: el commit queda en hoom/%s", ca, slug)
	}
	if files := atGit(t, wt, "show", "--name-only", "--format=", "HEAD"); files != spec {
		t.Fatalf("%s: el commit lleva solo el spec: %q", ca, files)
	}
	if atGit(t, dir, "rev-parse", "HEAD") != headRaiz {
		t.Fatalf("%s: el principal no gana un commit", ca)
	}
	if st := atGit(t, dir, "status", "--porcelain", "--", ".hoom/items/"+slug+".yaml"); st == "" {
		t.Fatalf("%s: el item del principal sigue sin guardar", ca)
	}
	if it, err := item.Load(dir, slug); err != nil || it.Riesgo != "alto" {
		t.Fatalf("%s: el item del principal sigue alto en el disco: %+v %v", ca, it, err)
	}

	// `hoom item save` from the worktree (expect nil) commits it (42ffc0)
	if _, err := itemcmd.Save(wt, "main", tbBlockOn, slug, nil); err != nil {
		t.Fatal(err)
	}
	if body := atGit(t, dir, "show", "HEAD:.hoom/items/"+slug+".yaml"); !strings.Contains(body, "riesgo: alto") {
		t.Fatalf("%s: hoom item save commitea el item del principal:\n%s", ca, body)
	}
	if st := atGit(t, dir, "status", "--porcelain", "--", ".hoom/items"); st != "" {
		t.Fatalf("%s: el item del principal queda limpio: %q", ca, st)
	}
}
