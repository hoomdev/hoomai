package servecmd

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/itemcmd"
)

// Finding 13bba6: `hoom serve` started in the task's worktree, left on
// another branch or with a detached HEAD: Guardar with the unsynced the
// board shows is 409 with the refusal of the task commands, and no tree
// gets a commit.
func TestGuardarDesdeElServeDelWorktreeFueraDeSuRama(t *testing.T) {
	const ca = "CA-345 (hallazgo 13bba6)"
	for _, separado := range []bool{false, true} {
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
		dice := "en la rama otra"
		if separado {
			gitRun(t, wt, "checkout", "-q", "--detach")
			dice = "con HEAD separado"
		} else {
			gitRun(t, wt, "checkout", "-q", "-b", "otra")
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
		if len(unsynced) == 0 {
			t.Fatalf("%s: fixture: el tablero muestra algo sin guardar", ca)
		}
		headRaiz, headWt := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")
		rec = atPedir(t, ca, s, http.MethodPost, "/api/board/"+slug+"/save", s.Token(), atCuerpoPaths(unsynced))
		if msg := tbErrorJSON(t, ca, rec, http.StatusConflict); !strings.Contains(msg, dice) {
			t.Fatalf("%s: separado=%v: el 409 dice %q: %q", ca, separado, dice, msg)
		}
		if atGit(t, dir, "rev-parse", "HEAD") != headRaiz || atGit(t, wt, "rev-parse", "HEAD") != headWt {
			t.Fatalf("%s: separado=%v: ningun arbol gana un commit", ca, separado)
		}
	}
}
