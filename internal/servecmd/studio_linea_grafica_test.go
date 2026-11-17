// Tests adversariales del spec .hoom/specs/studio-tu-siguiente-paso.md
// (CA-731..CA-733): la linea grafica hoomdev del Studio. Los tokens y las
// familias se leen del <style> de index.html (estaticos, como los de hoy);
// las fuentes, de lo embebido en el binario y de lo que sirve GET /fuentes/;
// las licencias, de internal/servecmd/licencias/ (fuera de ui/, por CA-10).
package servecmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Las 14 variables de la linea grafica, con su valor (tabla "Los tokens").
var stTokensHD = map[string]string{
	"--hd-carbon":      "#0d1117",
	"--hd-carbon-900":  "#090c10",
	"--hd-grafito":     "#21262d",
	"--hd-grafito-700": "#2d333b",
	"--hd-borde":       "#30363d",
	"--hd-blanco":      "#ffffff",
	"--hd-gris-claro":  "#c9d1d9",
	"--hd-gris":        "#8b949e",
	"--hd-verde":       "#3ed18b",
	"--hd-verde-gate":  "#0ca30c",
	"--hd-amarillo":    "#fab219",
	"--hd-naranja":     "#ec835a",
	"--hd-rojo":        "#d03b3b",
	"--hd-azul":        "#3987e5",
}

// Las variables de hoy que toman el valor hoomdev, y las que no cambian (su
// valor es el de index.html antes de este spec).
var (
	stTokensNuevos = map[string]string{
		"--surface": "#0d1117",
		"--panel":   "#21262d",
		"--panel-2": "#2d333b",
		"--border":  "#30363d",
		"--text-2":  "#c9d1d9",
		"--text-3":  "#8b949e",
	}
	stTokensIguales = map[string]string{
		"--text":     "#ffffff",
		"--good":     "#0ca30c",
		"--warning":  "#fab219",
		"--serious":  "#ec835a",
		"--critical": "#d03b3b",
		"--accent":   "#3987e5",
		"--human":    "#a07cf0",
	}
)

const (
	stFamiliaNucleo = `"Space Grotesk", system-ui, -apple-system, "Segoe UI", sans-serif`
	stFamiliaMono   = `"JetBrains Mono", ui-monospace, "Cascadia Code", "Source Code Pro", Menlo, Consolas, monospace`
)

// Las 5 fuentes embebidas, con su sha256 (tabla "Las fuentes"), su familia y
// su peso.
var stFuentes = []struct{ archivo, sha, familia, peso string }{
	{"SpaceGrotesk-400.ttf", "f40060430b03d78a3c848fd7a03afe43a69b929a390189df5f6dcac8b0566106", "Space Grotesk", "400"},
	{"SpaceGrotesk-500.ttf", "86a4ead05e0ccc556313f2953d80118a4e457c653dd953decef57ea651fa7d83", "Space Grotesk", "500"},
	{"SpaceGrotesk-700.ttf", "6a8d89aa96030d44419744bf6be8d86a1493989390a39c4ce2baa54ac7100233", "Space Grotesk", "700"},
	{"JetBrainsMono-400.ttf", "0c300074685498dea5579a65920e6441b31b76778c82d5f2e3eac2d4b7c4a8ab", "JetBrains Mono", "400"},
	{"JetBrainsMono-500.ttf", "eb2184a7a02804a58a9e909d8e63353b14ad1f8f2df9969045053a4f0cc8fc5d", "JetBrains Mono", "500"},
}

var (
	stReComentarioCSS = regexp.MustCompile(`(?s)/\*.*?\*/`)
	stReDeclaracion   = regexp.MustCompile(`(--[\w-]+|[a-z-]+)\s*:\s*([^;{}]*)`)
	stReVar           = regexp.MustCompile(`var\(\s*(--[\w-]+)\s*(?:,[^)]*)?\)`)
)

// stEstilo es el CSS de los <style> de index.html, sin comentarios.
func stEstilo(t *testing.T, html string) string {
	t.Helper()
	var b strings.Builder
	for _, m := range regexp.MustCompile(`(?s)<style[^>]*>(.*?)</style>`).FindAllStringSubmatch(html, -1) {
		b.WriteString(m[1])
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		t.Fatal("CA-731: index.html tiene su <style>")
	}
	return stReComentarioCSS.ReplaceAllString(b.String(), "")
}

// stDeclaraciones de un bloque CSS: propiedad (o variable) -> valor, la
// ultima gana.
func stDeclaraciones(bloque string) map[string]string {
	out := map[string]string{}
	for _, m := range stReDeclaracion.FindAllStringSubmatch(bloque, -1) {
		out[m[1]] = strings.TrimSpace(m[2])
	}
	return out
}

// stRaiz son las variables de los bloques :root del CSS.
func stRaiz(css string) map[string]string {
	out := map[string]string{}
	for _, m := range regexp.MustCompile(`:root\s*\{([^}]*)\}`).FindAllStringSubmatch(css, -1) {
		for k, v := range stDeclaraciones(m[1]) {
			if strings.HasPrefix(k, "--") {
				out[k] = v
			}
		}
	}
	return out
}

// stResolver reemplaza cada var(--x) por el valor de --x en :root.
func stResolver(vars map[string]string, v string) string {
	for i := 0; i < 8 && stReVar.MatchString(v); i++ {
		v = stReVar.ReplaceAllStringFunc(v, func(m string) string {
			return vars[stReVar.FindStringSubmatch(m)[1]]
		})
	}
	return v
}

// stNormalFamilia junta espacios y usa comillas dobles.
func stNormalFamilia(v string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(v, "'", `"`)), " ")
}

// CA-731: index.html define en :root las 14 variables --hd-* con los valores
// de la tabla, las seis variables de hoy con sus valores nuevos, y las siete
// que no cambian con su valor de hoy.
func TestCA731_TokensDeLaLineaGrafica(t *testing.T) {
	html, _ := tbUI(t)
	vars := stRaiz(stEstilo(t, html))
	revisar := func(que string, tabla map[string]string) {
		for k, quiere := range tabla {
			v, ok := vars[k]
			if !ok {
				t.Fatalf("CA-731: el :root de index.html define %s (%s)", k, que)
			}
			if got := strings.ToLower(strings.TrimSpace(stResolver(vars, v))); got != quiere {
				t.Fatalf("CA-731: %s vale %s (%s), vale %q", k, quiere, que, v)
			}
		}
	}
	revisar("token hoomdev", stTokensHD)
	revisar("valor hoomdev de una variable de hoy", stTokensNuevos)
	revisar("variable que no cambia", stTokensIguales)
}

// CA-731: el body usa la familia del nucleo ("Space Grotesk" con sus
// respaldos) y --mono es "JetBrains Mono" con los suyos (caso limite "sin
// fuentes": la pagina cae en las familias de respaldo).
func TestCA731_FamiliasDelNucleoYMono(t *testing.T) {
	html, _ := tbUI(t)
	css := stEstilo(t, html)
	vars := stRaiz(css)

	mono, ok := vars["--mono"]
	if !ok {
		t.Fatal("CA-731: el :root de index.html define --mono")
	}
	mono = stNormalFamilia(stResolver(vars, mono))
	if !strings.HasPrefix(mono, `"JetBrains Mono"`) {
		t.Fatalf(`CA-731: --mono empieza con "JetBrains Mono": %s`, mono)
	}
	if mono != stFamiliaMono {
		t.Fatalf("CA-731: --mono es %s, es %s", stFamiliaMono, mono)
	}

	familia := ""
	for _, m := range regexp.MustCompile(`(?:^|[}\s;,])body\s*\{([^}]*)\}`).FindAllStringSubmatch(css, -1) {
		for _, d := range stReDeclaracion.FindAllStringSubmatch(m[1], -1) {
			if d[1] == "font" || d[1] == "font-family" {
				familia = strings.TrimSpace(d[2])
			}
		}
	}
	if familia == "" {
		t.Fatal("CA-731: el body declara su familia (font o font-family)")
	}
	if got := stNormalFamilia(stResolver(vars, familia)); !strings.Contains(got, stFamiliaNucleo) {
		t.Fatalf("CA-731: el body usa el nucleo %s, usa %s", stFamiliaNucleo, got)
	}
}

// CA-732: los 5 .ttf de internal/servecmd/ui/fuentes/ tienen exactamente los
// sha256 de la tabla, y GET /fuentes/<archivo> responde 200 con esos bytes.
func TestCA732_FuentesEmbebidasYServidas(t *testing.T) {
	dir := newProject(t)
	for _, f := range stFuentes {
		raw, err := fs.ReadFile(uiFS, "ui/fuentes/"+f.archivo)
		if err != nil {
			t.Fatalf("CA-732: ui/fuentes/%s esta embebido en el binario: %v", f.archivo, err)
		}
		suma := sha256.Sum256(raw)
		if got := hex.EncodeToString(suma[:]); got != f.sha {
			t.Fatalf("CA-732: el sha256 de %s es %s, es %s", f.archivo, f.sha, got)
		}
		rec := serveGET(t, dir, "/fuentes/"+f.archivo)
		if rec.Code != http.StatusOK {
			t.Fatalf("CA-732: GET /fuentes/%s responde 200, respondio %d", f.archivo, rec.Code)
		}
		if !bytes.Equal(rec.Body.Bytes(), raw) {
			t.Fatalf("CA-732: GET /fuentes/%s sirve el archivo embebido", f.archivo)
		}
	}
}

// CA-732: index.html declara cada fuente con @font-face, src:
// url("fuentes/<archivo>") y font-display: swap, con su familia y su peso.
func TestCA732_FontFaceDeCadaFuente(t *testing.T) {
	html, _ := tbUI(t)
	css := stEstilo(t, html)
	bloques := regexp.MustCompile(`@font-face\s*\{([^}]*)\}`).FindAllStringSubmatch(css, -1)
	if len(bloques) < len(stFuentes) {
		t.Fatalf("CA-732: index.html declara las %d fuentes con @font-face, declara %d", len(stFuentes), len(bloques))
	}
	pesos := map[string][]string{"400": {"400", "normal"}, "500": {"500"}, "700": {"700", "bold"}}
	for _, f := range stFuentes {
		url := `url("fuentes/` + f.archivo + `")`
		var bloque string
		for _, b := range bloques {
			if strings.Contains(b[1], url) {
				bloque = b[1]
			}
		}
		if bloque == "" {
			t.Fatalf("CA-732: un @font-face de index.html tiene src: %s", url)
		}
		d := stDeclaraciones(bloque)
		if !strings.Contains(d["src"], url) {
			t.Fatalf("CA-732: el @font-face de %s lo declara en src: %q", f.archivo, d["src"])
		}
		if strings.TrimSpace(d["font-display"]) != "swap" {
			t.Fatalf("CA-732: el @font-face de %s tiene font-display: swap: %q", f.archivo, d["font-display"])
		}
		if fam := stNormalFamilia(d["font-family"]); fam != `"`+f.familia+`"` {
			t.Fatalf("CA-732: el @font-face de %s es de la familia %q: %q", f.archivo, f.familia, d["font-family"])
		}
		ok := false
		for _, p := range pesos[f.peso] {
			if strings.TrimSpace(d["font-weight"]) == p {
				ok = true
			}
		}
		if !ok {
			t.Fatalf("CA-732: el @font-face de %s tiene font-weight %s: %q", f.archivo, f.peso, d["font-weight"])
		}
	}
}

// CA-732: la tarea mueve los .ttf de intake a ui/fuentes/: en
// .hoom/intake/studio-tu-siguiente-paso/fuentes/ ya no hay ningun .ttf.
func TestCA732_IntakeSinLasFuentes(t *testing.T) {
	intake := filepath.Join("..", "..", ".hoom", "intake", "studio-tu-siguiente-paso", "fuentes")
	err := filepath.WalkDir(intake, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		if !d.IsDir() && strings.EqualFold(filepath.Ext(p), ".ttf") {
			t.Fatalf("CA-732: los .ttf se mueven con git mv a internal/servecmd/ui/fuentes/: queda %s", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// CA-733: internal/servecmd/licencias/OFL-SpaceGrotesk.txt y
// OFL-JetBrainsMono.txt llevan, arriba, la linea de copyright de su familia
// tal como esta en el contrato, y despues el texto de la SIL Open Font
// License, Version 1.1 con sus cinco condiciones.
func TestCA733_LicenciasOFL(t *testing.T) {
	for _, l := range []struct{ archivo, copyright string }{
		{"OFL-SpaceGrotesk.txt", "Copyright 2020 The Space Grotesk Project Authors (https://github.com/floriankarsten/space-grotesk)"},
		{"OFL-JetBrainsMono.txt", "Copyright 2020 The JetBrains Mono Project Authors (https://github.com/JetBrains/JetBrainsMono)"},
	} {
		raw, err := os.ReadFile(filepath.Join("licencias", l.archivo))
		if err != nil {
			t.Fatalf("CA-733: existe internal/servecmd/licencias/%s: %v", l.archivo, err)
		}
		texto := strings.Join(strings.Fields(string(raw)), " ")
		c := strings.Index(texto, l.copyright)
		if c < 0 {
			t.Fatalf("CA-733: %s tiene la linea %q", l.archivo, l.copyright)
		}
		h := strings.Index(texto, "SIL OPEN FONT LICENSE Version 1.1")
		if h < 0 {
			t.Fatalf("CA-733: %s tiene el texto de la SIL OPEN FONT LICENSE Version 1.1", l.archivo)
		}
		if c > h {
			t.Fatalf("CA-733: en %s la linea de copyright va arriba de la licencia", l.archivo)
		}
		p := strings.Index(texto[h:], "PERMISSION & CONDITIONS")
		if p < 0 {
			t.Fatalf("CA-733: %s tiene la seccion PERMISSION & CONDITIONS de la OFL 1.1", l.archivo)
		}
		p += h
		for i, cond := range []struct{ marca, frase string }{
			{"1)", "may be sold by itself"},
			{"2)", "provided that each copy contains the above copyright notice and this license"},
			{"3)", "Reserved Font Name"},
			{"4)", "shall not be used to promote, endorse or advertise"},
			{"5)", "must be distributed entirely under this license"},
		} {
			k := strings.Index(texto[p:], cond.marca)
			if k < 0 {
				t.Fatalf("CA-733: %s tiene la condicion %d de la OFL 1.1", l.archivo, i+1)
			}
			q := strings.Index(texto[p+k:], cond.frase)
			if q < 0 {
				t.Fatalf("CA-733: la condicion %d de %s dice %q", i+1, l.archivo, cond.frase)
			}
			p += k + q + len(cond.frase)
		}
	}
}
