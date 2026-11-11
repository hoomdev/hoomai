package finding

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hoomdev/hoomai/internal/gitx"
)

// Subida is a person's raise of an open finding: the immutable record
// .hoom/findings/<id>.raise.json that makes it block whatever its severity
// (Bloquea's case 2). There is no way down: a raised finding stops blocking
// when it is closed, with the rules as always.
//
// Everything that reads .hoom/findings/ (List and its relatives, Gate,
// ContextoDe, the board) reads a .raise.json as a raise, never as a finding.
// A raise is valid when its file can be read, its finding_id is the id in
// its name, its reason is not blank and its finding exists. An invalid one
// raises nothing and leaves a warning, the way an invalid resolution does,
// and it never turns the reading into an error (the gate does not go ERROR
// over it):
//
//   - `subida ilegible <archivo>`;
//   - `subida invalida <archivo>: <motivo>; el hallazgo <id> no sube`;
//   - `subida huerfana <archivo>: no existe el hallazgo <id>`.
type Subida struct {
	FindingID string    `json:"finding_id"`
	Reason    string    `json:"reason"`
	Author    string    `json:"author"` // the git identity of whoever raised it
	RaisedAt  time.Time `json:"raised_at"`
}

// NoSube is the refusal of a raise inside a role's run: no role raises a
// finding, the refutador included. Its text is
// `un <rol> no sube hallazgos: lo hace una persona`.
type NoSube string

func (e NoSube) Error() string { return string(e) }

// Raise is RaiseEnCorrida outside any run (an empty Corrida).
func Raise(root, id, reason string) (s Subida, nueva bool, err error) {
	return RaiseEnCorrida(root, id, reason, Corrida{})
}

// RaiseEnCorrida is `hoom finding raise <id> --reason "<por que>"`: it writes
// .hoom/findings/<id>.raise.json with finding_id id, the reason (trimmed),
// author the git identity of root (as resolve) and raised_at now (UTC). The
// CLI then prints `hallazgo <id> subido: ahora bloquea`.
//
// It refuses, writing nothing, in this order (exit 1 in the CLI):
//
//   - c.Role not empty: NoSube(`un <rol> no sube hallazgos: lo hace una persona`);
//   - a blank reason: `falta --reason: nadie sube un hallazgo sin decir por que`;
//   - no finding id: `hallazgo no encontrado: <id>`;
//   - a closed finding:
//     `el hallazgo <id> ya esta resuelto como "<as>": un hallazgo cerrado no se sube`.
//
// A finding that already has a valid raise is no error: it returns that
// raise with nueva false and does not rewrite its file (the CLI prints
// `hallazgo <id> ya estaba subido` and exits 0). nueva is true only when
// this call wrote the file.
func RaiseEnCorrida(root, id, reason string, c Corrida) (s Subida, nueva bool, err error) {
	if rol := strings.TrimSpace(c.Role); rol != "" {
		return Subida{}, false, NoSube(fmt.Sprintf("un %s no sube hallazgos: lo hace una persona", rol))
	}
	if strings.TrimSpace(reason) == "" {
		return Subida{}, false, fmt.Errorf("falta --reason: nadie sube un hallazgo sin decir por que")
	}
	// the id becomes a path: only a name hoom gives a finding reaches the disk
	if !EsNombre(id+".json") || strings.HasSuffix(id, ".res") {
		return Subida{}, false, fmt.Errorf("hallazgo no encontrado: %s", id)
	}
	if st, err := os.Stat(findingPath(root, id)); err != nil || !st.Mode().IsRegular() {
		return Subida{}, false, fmt.Errorf("hallazgo no encontrado: %s", id)
	}
	if raw, err := os.ReadFile(resPath(root, id)); err == nil {
		if prev, motivo := parseResolution(raw); motivo == "" {
			return Subida{}, false, fmt.Errorf("el hallazgo %s ya esta resuelto como %q: un hallazgo cerrado no se sube", id, prev.As)
		}
	}
	for intento := 0; ; intento++ {
		if raw, err := os.ReadFile(raisePath(root, id)); err == nil {
			prev, motivo := parseSubida(raw, id)
			if motivo != "" {
				// the binary never rewrites append-only evidence, as resolve
				return Subida{}, false, fmt.Errorf("la subida existente de %s es invalida (%s): el hallazgo no sube.\nAccion: repara o borra a mano %s (el cambio queda en el diff de Git) y vuelve a subirlo",
					id, motivo, raisePath(root, id))
			}
			return prev, false, nil
		} else if !os.IsNotExist(err) {
			return Subida{}, false, err
		}
		s = Subida{FindingID: id, Reason: strings.TrimSpace(reason), Author: gitx.Identity(root),
			RaisedAt: time.Now().UTC()}
		raw, err := json.MarshalIndent(s, "", "  ")
		if err != nil {
			return Subida{}, false, err
		}
		// never over another: two raises at once leave one file, and the
		// loser reads the winner's
		f, err := os.OpenFile(raisePath(root, id), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if os.IsExist(err) && intento == 0 {
			continue
		}
		if err != nil {
			return Subida{}, false, err
		}
		_, werr := f.Write(append(raw, '\n'))
		if cerr := f.Close(); werr == nil {
			werr = cerr
		}
		if werr != nil {
			os.Remove(raisePath(root, id))
			return Subida{}, false, werr
		}
		return s, true, nil
	}
}

// parseSubida reads a raise named <id>.raise.json and says why it raises
// nothing ("" = it raises, when its finding exists): "ilegible", or why it
// is invalid.
func parseSubida(raw []byte, id string) (Subida, string) {
	var s Subida
	if json.Unmarshal(raw, &s) != nil {
		return s, "ilegible"
	}
	if s.FindingID != id {
		return s, fmt.Sprintf("finding_id %q no es el de su nombre", s.FindingID)
	}
	if strings.TrimSpace(s.Reason) == "" {
		return s, "sin razon"
	}
	return s, ""
}
