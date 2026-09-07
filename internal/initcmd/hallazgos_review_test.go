// Tests de regresion de la review cruzada (Codex, 2026-09-06) sobre Spec D + Spec E.
package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/envelope"
)

// hrTieneRegla dice si un .gitignore contiene la regla como linea propia.
func hrTieneRegla(gitignore, regla string) bool {
	for _, l := range strings.Split(gitignore, "\n") {
		if strings.TrimSpace(l) == regla {
			return true
		}
	}
	return false
}

// hrTrasUnSobre planta un .hoom/.gitignore con el contenido dado en un
// proyecto nuevo, escribe un sobre y devuelve como quedo el archivo.
func hrTrasUnSobre(t *testing.T, gitignore string) string {
	t.Helper()
	root := t.TempDir()
	gi := filepath.Join(root, ".hoom", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(gi), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gi, []byte(gitignore), 0o644); err != nil {
		t.Fatal(err)
	}
	envelope.Write(root, envelope.Record{ID: "20260906T230000_b26478", Role: "writer", Provider: "claude",
		Stage: "spec", Step: 1, Steps: 5, Status: envelope.StatusRunning, ExitCode: -1})
	raw, err := os.ReadFile(gi)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// Hallazgo 20260906T235816_b26478: .hoom/.gitignore de este repo agrega
// isolated/ pero omite envelopes/, asi que el primer envelope.Write lo
// "repara" mutando un archivo rastreado: si el sobre corta antes de verify
// (spec sin aprobar, por ejemplo) deja el candidato con drift y rompe el hoom
// check previo, contra CA-203 (registrar telemetria no cambia el check). El
// .gitignore canonico —el que genera hoom init y el que este repo tiene
// commiteado— tiene que esconder envelopes/ e isolated/ de antemano, y
// envelope.Write tiene que dejarlo byte a byte igual.
func TestHallazgo_b26478_ElGitignoreCanonicoYaEscondeLosSobres(t *testing.T) {
	// (1) el que genera hoom init
	proyecto := t.TempDir()
	if err := Run(proyecto, "demo", "go"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(proyecto, ".hoom", ".gitignore"))
	if err != nil {
		t.Fatalf("b26478: hoom init debe dejar .hoom/.gitignore: %v", err)
	}
	generado := string(raw)
	for _, regla := range []string{"envelopes/", "isolated/"} {
		if !hrTieneRegla(generado, regla) {
			t.Fatalf("b26478: el .hoom/.gitignore de hoom init debe traer %q: %q", regla, generado)
		}
	}
	if got := hrTrasUnSobre(t, generado); got != generado {
		t.Fatalf("b26478: con el .gitignore de hoom init, envelope.Write no debe tocarlo:\n antes=%q\n despues=%q", generado, got)
	}

	// (2) el de este repo, tal como esta commiteado (../.. desde el paquete,
	// como hace copiaDeEsteRepo en agentcmd)
	propio, err := os.ReadFile(filepath.Join("..", "..", ".hoom", ".gitignore"))
	if os.IsNotExist(err) {
		t.Skip("b26478: este checkout no tiene .hoom/.gitignore (arbol parcial)")
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := hrTrasUnSobre(t, string(propio)); got != string(propio) {
		t.Fatalf("b26478: el .hoom/.gitignore commiteado en este repo no esconde envelopes/, asi que el primer envelope.Write muta un archivo rastreado:\n antes=%q\n despues=%q", propio, got)
	}
}
