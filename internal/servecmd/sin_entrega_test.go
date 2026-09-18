// Tests adversariales del spec .hoom/specs/arquitecto-bajo-el-sobre.md
// (CA-240): el Studio ve el sobre sin entrega como lo ve hoom status: con su
// estado propio y su badge, no como un NO ENTREGABLE mas.
package servecmd

import (
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// CA-240: /api/envelopes sirve el estado tal cual y la UI embebida tiene el
// badge SIN ENTREGA para el estado sin-entrega.
func TestCA240_ElStudioTieneElBadgeSinEntrega(t *testing.T) {
	dir := newGitProject(t)
	srv := newServer(t, dir)
	now := time.Now().UTC()
	envelope.Write(dir, envelope.Record{
		ID: "20260918T120000_ab12cd", Role: "arquitecto", Provider: "claude",
		Stage: "scope", Step: 3, Steps: 5, Status: envelope.StatusNoDelivery, ExitCode: 1,
		Note:      "el rol arquitecto escribe y el run no dejo ningun archivo: no hay arbol nuevo que certificar",
		StartedAt: now.Add(-time.Minute), EndedAt: now,
	})

	var sobres []map[string]any
	get(t, srv, "/api/envelopes", &sobres)
	if len(sobres) != 1 || sobres[0]["status"] != "sin-entrega" {
		t.Fatalf("CA-240: /api/envelopes sirve el estado sin-entrega: %v", sobres)
	}
	if sobres[0]["note"] == "" || sobres[0]["note"] == nil || sobres[0]["exit_code"] != float64(1) {
		t.Fatalf("CA-240: el Studio recibe la nota y el exit: %v", sobres[0])
	}

	raw, err := fs.ReadFile(uiFS, "ui/index.html")
	if err != nil {
		t.Fatal(err)
	}
	html := string(raw)
	for _, quiero := range []string{"sin-entrega", "SIN ENTREGA"} {
		if !strings.Contains(html, quiero) {
			t.Fatalf("CA-240: la UI debe pintar el badge del estado sin-entrega (%q)", quiero)
		}
	}
	// el badge de siempre sigue en su lugar
	if !strings.Contains(html, "NO ENTREGABLE") {
		t.Fatal("CA-240: el badge NO ENTREGABLE no puede desaparecer")
	}
}
