package pilotocmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/hoomfs"
)

// Pruebas propias del writer (autonomia-entre-firmas): el candado, el aviso
// de la firma y los codigos de salida que no necesitan un CLI.

func TestWriterCandadoYMensaje(t *testing.T) {
	root := t.TempDir()
	if got, want := Candado(root, "t"), filepath.Join(root, ".hoom", "cache", "locks", "piloto-t.lock"); got != want {
		t.Fatalf("Candado = %q, quiero %q", got, want)
	}
	if got := MsgYaCorre("t"); got != "el piloto de t ya corre en otro proceso" {
		t.Fatalf("MsgYaCorre = %q", got)
	}
}

func TestWriterRunConElCandadoTomado(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "hoom.yaml"), []byte("schema: hoom/v1\nproject: p\nprofile: go\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	soltar, err := hoomfs.Lock(Candado(root, "t"), 0)
	if err != nil {
		t.Fatal(err)
	}
	defer soltar()
	var out, errb bytes.Buffer
	if code := Run([]string{"t"}, Entorno{Dir: root, Out: &out, Err: &errb}); code != 1 {
		t.Fatalf("exit %d, quiero 1 (stderr %q)", code, errb.String())
	}
	if got := strings.TrimSpace(errb.String()); got != "hoom: el piloto de t ya corre en otro proceso" {
		t.Fatalf("stderr %q", got)
	}
}

func TestWriterRunUso(t *testing.T) {
	var out, errb bytes.Buffer
	for _, args := range [][]string{nil, {"--nada", "t"}, {"a", "b"}, {"No-Slug"}} {
		out.Reset()
		errb.Reset()
		if code := Run(args, Entorno{Dir: t.TempDir(), Out: &out, Err: &errb}); code != 2 {
			t.Fatalf("%v: exit %d, quiero 2", args, code)
		}
	}
	if code := Run([]string{"-h"}, Entorno{Out: &out, Err: &errb}); code != 0 || !strings.Contains(out.String(), "hoom piloto <slug>") {
		t.Fatalf("-h: exit %d, %q", code, out.String())
	}
}

func TestWriterAvisoDeFirma(t *testing.T) {
	root := t.TempDir()
	items := filepath.Join(root, ".hoom", "items")
	if err := os.MkdirAll(items, 0o755); err != nil {
		t.Fatal(err)
	}
	escribir := func(slug, cuerpo string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(items, slug+".yaml"), []byte(cuerpo), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	const base = "tipo: feature\nprioridad: media\ncreado_por: Prueba <p@example.com>\ncreado_en: 2026-11-01T10:00:00Z\n"
	escribir("con", "titulo: con\n"+base+"presupuesto_usd: 5\nauto: hasta-humano\n")
	escribir("sin", "titulo: sin\n"+base)
	if got := AvisoDeFirma(root, ".hoom/specs/con.md"); got != "la tarjeta tiene piloto automatico: sigue con: hoom piloto con" {
		t.Fatalf("con piloto: %q", got)
	}
	for _, spec := range []string{".hoom/specs/sin.md", ".hoom/specs/nadie.md", "docs/con.md"} {
		if got := AvisoDeFirma(root, spec); got != "" {
			t.Fatalf("%s: %q, quiero vacio", spec, got)
		}
	}
}
