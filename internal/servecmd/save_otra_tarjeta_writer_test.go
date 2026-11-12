package servecmd

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/boardcmd"
)

// Finding 65fbb7: `hoom serve` started in task precios's worktree. Off its
// branch (another one, or a detached HEAD), Guardar on another visible card
// (envios) is 409 with the refusal naming precios, and no tree gets a
// commit; on hoom/precios it answers 200 as before.
func TestGuardarOtraTarjetaDesdeElServeDelWorktree(t *testing.T) {
	const ca = "CA-345 (hallazgo 65fbb7)"
	for _, modo := range []string{"otra", "separado", "en su rama"} {
		dir := tbProyecto(t)
		tbItem(t, dir, "precios", "")
		gitRun(t, dir, "add", "-A")
		gitRun(t, dir, "commit", "-q", "-m", "item")
		wt := filepath.Join(dir, ".hoom", "worktrees", "precios")
		gitRun(t, dir, "worktree", "add", "-q", "-b", "hoom/precios", wt, "main")
		tbItem(t, wt, "envios", "")
		dice := ""
		switch modo {
		case "otra":
			gitRun(t, wt, "checkout", "-q", "-b", "otra")
			dice = "en la rama otra"
		case "separado":
			gitRun(t, wt, "checkout", "-q", "--detach")
			dice = "con HEAD separado"
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
				if c.Slug == "envios" {
					unsynced = c.Unsynced
				}
			}
		}
		if len(unsynced) == 0 {
			t.Fatalf("%s: fixture: el tablero muestra envios sin guardar", ca)
		}
		headRaiz, headWt := atGit(t, dir, "rev-parse", "HEAD"), atGit(t, wt, "rev-parse", "HEAD")
		rec = atPedir(t, ca, s, http.MethodPost, "/api/board/envios/save", s.Token(), atCuerpoPaths(unsynced))
		if modo == "en su rama" {
			if rec.Code != http.StatusOK || atGit(t, wt, "rev-parse", "HEAD") == headWt {
				t.Fatalf("%s: en hoom/precios guarda como antes: %d %s", ca, rec.Code, rec.Body.String())
			}
			continue
		}
		if msg := tbErrorJSON(t, ca, rec, http.StatusConflict); !strings.Contains(msg, dice) || !strings.Contains(msg, `"precios"`) {
			t.Fatalf("%s: %s: el 409 nombra la tarea del worktree y %q: %q", ca, modo, dice, msg)
		}
		if atGit(t, dir, "rev-parse", "HEAD") != headRaiz || atGit(t, wt, "rev-parse", "HEAD") != headWt {
			t.Fatalf("%s: %s: ningun arbol gana un commit", ca, modo)
		}
	}
}
