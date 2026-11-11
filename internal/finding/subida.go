package finding

import "time"

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
	return Subida{}, false, nil
}
