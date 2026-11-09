// Running gana timeout_s y last_event_at (spec tablero-en-la-terminal,
// CA-519): hoom item show --json los emite. El proyecto (con-sobre, suelto,
// sin-run) y lo que se exige del JSON son los de runningtest, los mismos de
// hoom board --json y /api/board (hallazgo 20261108T230948_50d437).
package itemcmd_test

import (
	"bytes"
	"testing"

	"github.com/hoomdev/hoomai/internal/itemcmd"
	"github.com/hoomdev/hoomai/internal/runningtest"
)

// CA-519: hoom item show <slug> --json emite timeout_s y last_event_at en
// el running de la tarjeta (null sin eventos).
func TestCA519_ItemShowJSONTraeLasClaves(t *testing.T) {
	root, now := runningtest.Proyecto(t)
	for _, slug := range []string{"con-sobre", "sin-run"} {
		req, err := itemcmd.Parse([]string{"show", slug, "--json"})
		if err != nil {
			t.Fatalf("CA-519: Parse: %v", err)
		}
		var out, errb bytes.Buffer
		if err := itemcmd.Execute(root, "main", "high", req, &out, &errb); err != nil {
			t.Fatalf("CA-519: hoom item show %s --json: %v\n%s", slug, err, errb.String())
		}
		runningtest.RevisarJSON(t, "CA-519", "hoom item show "+slug+" --json", out.Bytes(), now, slug)
	}
}
