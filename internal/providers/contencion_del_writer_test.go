// Tests adversariales del spec .hoom/specs/contencion-del-writer.md (CA-690):
// providers.ConEstado es opcional; claude y codex la implementan, con rutas
// en el formato del contrato. Las listas no se fijan (aclaracion 14).
package providers_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/providers"
)

// cdwSinEstado es un provider falso que NO implementa ConEstado: sigue
// compilando como Provider (CA-690).
type cdwSinEstado struct{}

func (cdwSinEstado) Name() string { return "sin-estado-cdw" }
func (cdwSinEstado) Bin() string  { return "hoom-sin-estado-cdw" }
func (cdwSinEstado) Capabilities() providers.Capabilities {
	return providers.Capabilities{SystemPrompt: true}
}
func (p cdwSinEstado) Command(req providers.Request) (providers.Invocation, error) {
	return providers.Invocation{Bin: p.Bin(), Args: []string{req.Prompt}}, nil
}
func (cdwSinEstado) Normalize(string) []providers.Event { return nil }

var _ providers.Provider = cdwSinEstado{}

func cdwFuera(p string) bool { return filepath.IsAbs(p) || strings.HasPrefix(p, "~/") }

func cdwDentro(p string) bool {
	return p != "" && !filepath.IsAbs(p) && !strings.HasPrefix(p, "~") && !strings.Contains(p, `\`) &&
		p != ".." && !strings.HasPrefix(p, "../") && !strings.Contains(p, "/../")
}

// CA-690: claude y codex implementan ConEstado; sus rutas de afuera son
// absolutas o empiezan con ~/, las del arbol son relativas con "/", y cada
// clave de Corre es una ruta de claves separadas por ".".
func TestCA690_ClaudeYCodexDeclaranSuEstado(t *testing.T) {
	for _, n := range []string{"claude", "codex"} {
		p, err := providers.Lookup(n)
		if err != nil {
			t.Fatal(err)
		}
		ce, ok := p.(providers.ConEstado)
		if !ok {
			t.Fatalf("CA-690: %s implementa providers.ConEstado", n)
		}
		e := ce.Estado()
		for _, w := range e.Escribe {
			if !cdwFuera(w) {
				t.Errorf("CA-690: %s: Escribe %q es absoluta o empieza con ~/", n, w)
			}
		}
		for _, a := range e.EnArbol {
			if !cdwDentro(a) {
				t.Errorf("CA-690: %s: EnArbol %q es relativa al arbol, con /", n, a)
			}
		}
		for _, c := range e.Corre {
			if !cdwFuera(c.Archivo) {
				t.Errorf("CA-690: %s: Corre.Archivo %q es absoluto o empieza con ~/", n, c.Archivo)
			}
			for _, k := range c.Claves {
				if k == "" || strings.HasPrefix(k, ".") || strings.HasSuffix(k, ".") || strings.Contains(k, "..") {
					t.Errorf("CA-690: %s: la clave %q de %s es una ruta de claves unidas con .", n, k, c.Archivo)
				}
			}
		}
	}
	var p providers.Provider = cdwSinEstado{}
	if _, ok := p.(providers.ConEstado); ok {
		t.Fatal("CA-690: fixture: el falso no implementa ConEstado")
	}
}
