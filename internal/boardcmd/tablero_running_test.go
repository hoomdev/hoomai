// Running gana timeout_s y last_event_at (spec tablero-en-la-terminal,
// CA-519): boardcmd.Build, DetailFor y JSONBytes los traen, desde el disco:
// el timeout_s del sidecar del run (el del sobre, o el del run suelto) y la
// hora del ultimo evento completo de su narracion (null sin eventos). El
// proyecto (con-sobre, suelto, sin-run) y lo que se exige del JSON son los de
// runningtest, los mismos de hoom item show --json y /api/board (hallazgo
// 20261108T230948_50d437).
package boardcmd_test

import (
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/boardcmd"
	"github.com/hoomdev/hoomai/internal/runningtest"
)

func tltCard(t *testing.T, b boardcmd.Board, slug string) boardcmd.Card {
	t.Helper()
	for _, c := range b.Columns {
		for _, k := range c.Cards {
			if k.Slug == slug {
				return k
			}
		}
	}
	t.Fatalf("el tablero trae la tarjeta %s", slug)
	return boardcmd.Card{}
}

// CA-519: Build trae en Running el timeout_s del run y la hora del ultimo
// evento completo de su narracion (la linea a medio escribir no cuenta); sin
// run, 0 y nil.
func TestCA519_BuildTraeTimeoutYUltimoEvento(t *testing.T) {
	root, now := runningtest.Proyecto(t)
	b, err := boardcmd.Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-519: Build: %v", err)
	}
	casos := []struct {
		slug    string
		timeout int
		ultimo  *time.Time
	}{
		{"con-sobre", 2700, ptr(now.Add(-2 * time.Second))},
		{"suelto", 1800, ptr(now.Add(-7 * time.Second))},
		{"sin-run", 0, nil},
	}
	for _, c := range casos {
		k := tltCard(t, b, c.slug)
		if k.Running == nil {
			t.Fatalf("CA-519: %s esta en curso (su dueno es este proceso)", c.slug)
		}
		if k.Running.TimeoutS != c.timeout {
			t.Fatalf("CA-519: el timeout_s de %s es %d, es %d", c.slug, c.timeout, k.Running.TimeoutS)
		}
		switch {
		case c.ultimo == nil && k.Running.LastEventAt != nil:
			t.Fatalf("CA-519: %s sin narracion: last_event_at nil, es %v", c.slug, k.Running.LastEventAt)
		case c.ultimo != nil && (k.Running.LastEventAt == nil || !k.Running.LastEventAt.Equal(*c.ultimo)):
			t.Fatalf("CA-519: el last_event_at de %s es %v, es %v", c.slug, *c.ultimo, k.Running.LastEventAt)
		}
	}
	d, err := boardcmd.DetailFor(root, "main", "high", "con-sobre", now, false)
	if err != nil {
		t.Fatalf("CA-519: DetailFor: %v", err)
	}
	if r := d.Card.Running; r == nil || r.TimeoutS != 2700 || r.LastEventAt == nil || !r.LastEventAt.Equal(now.Add(-2*time.Second)) {
		t.Fatalf("CA-519: la tarjeta del detalle trae lo mismo: %+v", r)
	}
}

func ptr(t time.Time) *time.Time { return &t }

// CA-519: hoom board --json (JSONBytes) emite timeout_s y last_event_at en
// cada running, con null sin eventos.
func TestCA519_JSONBytesTraeLasClaves(t *testing.T) {
	root, now := runningtest.Proyecto(t)
	b, err := boardcmd.Build(root, "main", "high", now)
	if err != nil {
		t.Fatalf("CA-519: Build: %v", err)
	}
	raw, err := boardcmd.JSONBytes(b)
	if err != nil {
		t.Fatalf("CA-519: JSONBytes: %v", err)
	}
	runningtest.RevisarJSON(t, "CA-519", "hoom board --json", raw, now, "con-sobre", "suelto", "sin-run")
}
