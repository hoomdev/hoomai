// Tests adversariales del spec .hoom/specs/evidencia-de-la-review.md
// (CA-481, CA-482) sobre las rutas de la evidencia en gitx: "la evidencia,
// la medida del rango (gitx.CambiosEntre), la guarda de arbol sucio y
// CambioDespues usan una sola definicion: todo el repositorio menos el
// estado de hoom". El estado de hoom es una lista cerrada:
// .hoom/approvals/*.json, .hoom/findings/*.json, .hoom/reviews/*.json,
// .hoom/verdicts/*.json, .hoom/items/*.yaml, .hoom/specs/*.md, .hoom/intake/
// entero, .hoom/ratchet.json, .hoom/.gitignore y los locales (.hoom/cache/,
// worktrees/, runs/, isolated/, envelopes/). *.json, *.yaml y *.md son
// archivos DIRECTOS del directorio: un subdirectorio, u otra extension en ese
// mismo lugar, no es estado y entra; todo lo demas bajo .hoom/ tambien.
//
// La tabla edrRutas es el oraculo: cada ruta dice si es estado o no, y la
// propiedad es que las tres lecturas de gitx (CambiosEntre, CandidatePatch y
// CambioSinCommitear) la clasifican igual, ruta por ruta.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// edrRuta es una fila del oraculo: la ruta, si es estado de hoom, y un
// hermano sin rastrear de la misma forma (con su clasificacion: el hermano
// de .hoom/ratchet.json no es .hoom/ratchet.json).
type edrRuta struct {
	ruta        string
	estado      bool
	nuevo       string // "" = sin hermano
	nuevoEstado bool
}

// edrRutas: lo que el spec nombra (los ejemplos de CA-481 y de la tabla) y
// las formas de la regla "archivo directo con la extension" (subdirectorios,
// otra extension, nombres que solo EMPIEZAN como un directorio de estado,
// directorios de estado anidados en otro lado, nombres no ASCII).
var edrRutas = []edrRuta{
	// entran (CA-481)
	{".hoom/semgrep/gate.sh", false, ".hoom/semgrep/otro.sh", false},
	{".hoom/semgrep/paquetes.lock", false, ".hoom/semgrep/otro.lock", false},
	{".hoom/semgrep/reglas ñandú.yaml", false, ".hoom/semgrep/más reglas.yaml", false},
	{".hoom/notas.txt", false, ".hoom/notas-locales.txt", false},
	{".hoom/otra/x.sh", false, ".hoom/otra/nuevo-x.sh", false},
	{".hoom/agents-viejo/nota.md", false, ".hoom/agents-viejo/otra.md", false},
	{".hoom/agents/04-writer.md", false, ".hoom/agents/99-nuevo.md", false},
	{".hoom/findings/x.sh", false, ".hoom/findings/nuevo-x.sh", false},
	{".hoom/findings/sub/y.json", false, ".hoom/findings/sub/nuevo-y.json", false},
	{".hoom/findings/.gitignore", false, "", false},
	{".hoom/specs/run.sh", false, ".hoom/specs/nuevo-run.sh", false},
	{".hoom/specs/sub/z.md", false, ".hoom/specs/sub/nuevo-z.md", false},
	{".hoom/approvals/leeme.txt", false, ".hoom/approvals/otro.txt", false},
	{".hoom/reviews/sub/r.json", false, ".hoom/reviews/sub/nuevo-r.json", false},
	{".hoom/verdicts/v.json.bak", false, ".hoom/verdicts/w.json.bak", false},
	{".hoom/items/i.yml", false, ".hoom/items/j.yml", false},
	{".hoom/items/sub/i.yaml", false, ".hoom/items/sub/j.yaml", false},
	{".hoom/ratchet.json.bak", false, "", false},
	{".hoom/findings-viejo/f.json", false, ".hoom/findings-viejo/g.json", false},
	{".hoom/intake2/doc.txt", false, ".hoom/intake2/otro.txt", false},
	{".hoom/cache2/c.txt", false, ".hoom/cache2/d.txt", false},
	{".hoom/otra/cache/c.txt", false, ".hoom/otra/cache/d.txt", false},
	{".hoom/otra/findings/f.json", false, ".hoom/otra/findings/g.json", false},
	{".hoom/sub/.gitignore", false, "", false},
	{"findings/f.json", false, "findings/g.json", false},
	{".hoom-fuera.go", false, "", false},
	{"app.go", false, "nuevo.go", false},
	// estado de hoom (la tabla del spec)
	{".hoom/approvals/a.json", true, ".hoom/approvals/b.json", true},
	{".hoom/findings/20261027T000000_aaaaaa.json", true, ".hoom/findings/20261027T000001_bbbbbb.json", true},
	{".hoom/findings/20261027T000000_aaaaaa.res.json", true, ".hoom/findings/20261027T000001_bbbbbb.res.json", true},
	{".hoom/findings/ñandú.json", true, ".hoom/findings/pingüino.json", true},
	{".hoom/reviews/20261027T000000_cccccc.json", true, ".hoom/reviews/20261027T000001_dddddd.json", true},
	{".hoom/verdicts/2026-10-27T00-00-00Z_eeeeee.json", true, ".hoom/verdicts/2026-10-27T00-00-01Z_ffffff.json", true},
	{".hoom/items/precios.yaml", true, ".hoom/items/otro.yaml", true},
	{".hoom/specs/x.md", true, ".hoom/specs/borrador.md", true},
	{".hoom/intake/contrato.pdf", true, ".hoom/intake/subido.pdf", true},
	{".hoom/intake/sub/nota.md", true, ".hoom/intake/nuevo/doc.docx", true},
	{".hoom/intake/x.sh", true, ".hoom/intake/y.sh", true},
	{".hoom/ratchet.json", true, "", false},
	{".hoom/.gitignore", true, "", false},
	{".hoom/cache/c.txt", true, ".hoom/cache/d.txt", true},
	{".hoom/worktrees/w.txt", true, ".hoom/worktrees/otra/x.go", true},
	{".hoom/runs/r.meta.json", true, ".hoom/runs/s.log", true},
	{".hoom/isolated/i.txt", true, ".hoom/isolated/j.txt", true},
	{".hoom/envelopes/e.json", true, ".hoom/envelopes/f.json", true},
}

// edrRepo arma main con cada ruta de la tabla (sin .hoom/.gitignore
// canonico: lo que es estado no depende de que este ignorado) y una rama
// "tarea" que las cambia todas en un commit. Devuelve el repo y el merge-base.
func edrRepo(t *testing.T) (dir, mb string) {
	t.Helper()
	idAislarGit(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dfGit(t, dir, "init", "-b", "main")
	dfGit(t, dir, "config", "user.email", "test@hoom.dev")
	dfGit(t, dir, "config", "user.name", "hoom test")
	dfGit(t, dir, "config", "core.quotePath", "false")
	for _, r := range edrRutas {
		write(t, dir, r.ruta, "base "+r.ruta+"\n")
	}
	dfGit(t, dir, "add", "-A", "-f")
	dfGit(t, dir, "commit", "-q", "-m", "base con cada ruta")
	mb = dfGit(t, dir, "rev-parse", "HEAD")
	dfGit(t, dir, "checkout", "-q", "-b", "tarea")
	for _, r := range edrRutas {
		write(t, dir, r.ruta, "base "+r.ruta+"\nMARCA-RAMA "+r.ruta+"\n")
	}
	dfGit(t, dir, "add", "-A", "-f")
	dfGit(t, dir, "commit", "-q", "-m", "la rama cambia cada ruta")
	if s := dfGit(t, dir, "status", "--porcelain", "-uall"); s != "" {
		t.Fatalf("fixture: el arbol queda limpio despues del commit: %q", s)
	}
	return dir, mb
}

// edrEnParche dice si el parche trae la seccion de ruta: su cabecera
// 'diff --git a/<ruta> b/<ruta>' entera, con el nombre crudo o citado por
// git (entre comillas, con octales).
func edrEnParche(parche, ruta string) bool {
	var oct strings.Builder
	for i := 0; i < len(ruta); i++ {
		if c := ruta[i]; c >= 0x80 {
			fmt.Fprintf(&oct, "\\%03o", c)
		} else {
			oct.WriteByte(c)
		}
	}
	for _, cab := range []string{
		"diff --git a/" + ruta + " b/" + ruta + "\n",
		"diff --git \"a/" + oct.String() + "\" \"b/" + oct.String() + "\"\n",
	} {
		if strings.HasPrefix(parche, cab) || strings.Contains(parche, "\n"+cab) {
			return true
		}
	}
	return false
}

// edrNombra dice si la ruta que nombro CambioSinCommitear es ruta, o un
// directorio que la contiene con la barra final con la que git nombra un
// directorio nuevo sin rastrear.
func edrNombra(nombrada, ruta string) bool {
	if nombrada == ruta {
		return true
	}
	return strings.HasSuffix(nombrada, "/") && strings.HasPrefix(ruta, nombrada)
}

// CA-481 (propiedad): para cada ruta del oraculo, CambiosEntre (la medida del
// rango) y CandidatePatch (el parche de la evidencia) la traen si y solo si
// NO es estado de hoom: .hoom/semgrep/, .hoom/notas.txt, un directorio nuevo,
// .hoom/agents-viejo/, .hoom/findings/x.sh y .hoom/findings/sub/y.json
// entran; un hallazgo y su .res.json, un registro de review, un veredicto,
// un item, una aprobacion, un spec, .hoom/intake/, .hoom/ratchet.json,
// .hoom/.gitignore y los locales no.
func TestCA481_CambiosEntreYElParcheTraenTodoMenosElEstadoDeHoom(t *testing.T) {
	dir, mb := edrRepo(t)
	head := dfGit(t, dir, "rev-parse", "HEAD")

	files, ins, del, err := CambiosEntre(dir, mb, head)
	if err != nil {
		t.Fatalf("CA-481: CambiosEntre: %v", err)
	}
	var quiero []string
	for _, r := range edrRutas {
		if !r.estado {
			quiero = append(quiero, r.ruta)
		}
	}
	got := append([]string(nil), files...)
	sort.Strings(got)
	sort.Strings(quiero)
	if strings.Join(got, "\n") != strings.Join(quiero, "\n") {
		var faltan, sobran []string
		en := map[string]bool{}
		for _, f := range got {
			en[f] = true
		}
		for _, q := range quiero {
			if !en[q] {
				faltan = append(faltan, q)
			}
			delete(en, q)
		}
		for f := range en {
			sobran = append(sobran, f)
		}
		sort.Strings(sobran)
		t.Fatalf("CA-481: la medida del rango (CambiosEntre) es todo menos el estado de hoom:\nfaltan %q\nsobran (estado de hoom) %q", faltan, sobran)
	}
	if ins != len(quiero) || del != 0 {
		t.Fatalf("CA-481: la medida cuenta una linea agregada por ruta que entra: +%d/-%d, quiero +%d/-0", ins, del, len(quiero))
	}

	var buf bytes.Buffer
	over, err := CandidatePatch(dir, "main", &buf, 16<<20)
	if err != nil || over {
		t.Fatalf("CA-481: CandidatePatch arma el parche de la rama: over %v, %v", over, err)
	}
	parche := buf.String()
	for _, r := range edrRutas {
		if en := edrEnParche(parche, r.ruta); en == r.estado {
			if r.estado {
				t.Fatalf("CA-481: %s es estado de hoom: el parche de la evidencia no lo trae:\n%s", r.ruta, parche)
			}
			t.Fatalf("CA-481: %s no es estado de hoom: el parche de la evidencia lo trae:\n%s", r.ruta, parche)
		}
		if marca := "\n+MARCA-RAMA " + r.ruta + "\n"; strings.Contains(parche, marca) == r.estado {
			t.Fatalf("CA-481: %s (estado %v): la linea cambiada %q va en el parche si y solo si la ruta entra:\n%s", r.ruta, r.estado, marca, parche)
		}
	}
}

// CA-482 (propiedad, la misma tabla): CambioSinCommitear mira las mismas
// rutas. Con UNA ruta sucia por vez —una edicion sin commitear de una ruta
// rastreada, o un hermano sin rastrear de la misma forma—, nombra esa ruta si
// no es estado de hoom, y dice "" (arbol limpio) si lo es. Y el parche de la
// evidencia se niega con gitx.ArbolSucio por lo mismo, antes de mirar la base.
func TestCA482_CambioSinCommitearMiraLasMismasRutas(t *testing.T) {
	dir, _ := edrRepo(t)
	comprobar := func(caso, ruta string, estado bool) {
		t.Helper()
		got, err := CambioSinCommitear(dir)
		if err != nil {
			t.Fatalf("CA-482: %s %s: CambioSinCommitear: %v", caso, ruta, err)
		}
		if estado && got != "" {
			t.Fatalf("CA-482: %s %s es estado de hoom: no ensucia el arbol; CambioSinCommitear nombro %q", caso, ruta, got)
		}
		if !estado && !edrNombra(got, ruta) {
			t.Fatalf("CA-482: %s %s no es estado de hoom: ensucia el arbol y CambioSinCommitear la nombra; nombro %q", caso, ruta, got)
		}
		var buf bytes.Buffer
		_, perr := CandidatePatch(dir, "main", &buf, 16<<20)
		var sucio ArbolSucio
		if esSucio := errors.As(perr, &sucio); esSucio == estado {
			t.Fatalf("CA-482: %s %s (estado %v): el parche de la evidencia se niega por arbol sucio si y solo si la ruta no es estado: %v", caso, ruta, estado, perr)
		}
	}
	for _, r := range edrRutas {
		// una edicion sin commitear de la ruta rastreada
		write(t, dir, r.ruta, "EDICION SIN COMMITEAR\n")
		comprobar("edicion de", r.ruta, r.estado)
		dfGit(t, dir, "checkout", "-q", "--", r.ruta)
		// su borrado sin commitear
		if err := os.Remove(filepath.Join(dir, r.ruta)); err != nil {
			t.Fatal(err)
		}
		comprobar("borrado de", r.ruta, r.estado)
		dfGit(t, dir, "checkout", "-q", "--", r.ruta)
		// un hermano sin rastrear de la misma forma
		if r.nuevo != "" {
			write(t, dir, r.nuevo, "SIN RASTREAR\n")
			comprobar("sin rastrear", r.nuevo, r.nuevoEstado)
			if err := os.Remove(filepath.Join(dir, r.nuevo)); err != nil {
				t.Fatal(err)
			}
			// un directorio que queda vacio no es nada para git, pero se borra
			_ = os.Remove(filepath.Dir(filepath.Join(dir, r.nuevo)))
		}
		if s := dfGit(t, dir, "status", "--porcelain", "-uall"); s != "" {
			t.Fatalf("fixture: el arbol vuelve a quedar limpio despues de %s: %q", r.ruta, s)
		}
	}
}

// CA-482 (caso hostil): con NADA rastreado bajo .hoom/, git nombra el
// directorio entero (`?? .hoom/`). Si adentro solo hay estado de hoom (un
// veredicto, un hallazgo, un item, un spec, un documento subido a intake y
// los locales), el arbol esta limpio; con un .hoom/semgrep/gate.sh mas, esta
// sucio y la ruta nombrada lo contiene.
func TestCA482_HoomSinRastrearEnteroSoloConEstadoNoEnsucia(t *testing.T) {
	idAislarGit(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dfGit(t, dir, "init", "-b", "main")
	dfGit(t, dir, "config", "user.email", "test@hoom.dev")
	dfGit(t, dir, "config", "user.name", "hoom test")
	write(t, dir, "app.go", "package app\n")
	dfGit(t, dir, "add", "-A")
	dfGit(t, dir, "commit", "-q", "-m", "inicial")
	for _, p := range []string{".hoom/verdicts/2026-10-27T00-00-00Z_eeeeee.json", ".hoom/findings/20261027T000000_aaaaaa.json",
		".hoom/items/precios.yaml", ".hoom/specs/x.md", ".hoom/intake/sub/contrato.pdf", ".hoom/ratchet.json",
		".hoom/runs/r.log", ".hoom/cache/c.txt", ".hoom/.gitignore"} {
		write(t, dir, p, "estado\n")
	}
	if s := dfGit(t, dir, "status", "--porcelain"); !strings.Contains(s, "?? .hoom/") {
		t.Fatalf("fixture: git nombra .hoom/ entero como sin rastrear: %q", s)
	}
	if got, err := CambioSinCommitear(dir); err != nil || got != "" {
		t.Fatalf("CA-482: un .hoom/ sin rastrear con solo estado de hoom no ensucia el arbol: %q, %v", got, err)
	}
	write(t, dir, ".hoom/semgrep/gate.sh", "#!/bin/sh\nexit 0\n")
	got, err := CambioSinCommitear(dir)
	if err != nil || !edrNombra(got, ".hoom/semgrep/gate.sh") {
		t.Fatalf("CA-482: con .hoom/semgrep/gate.sh sin rastrear el arbol esta sucio y la ruta nombrada lo contiene: %q, %v", got, err)
	}
}

// CA-482 (caso hostil): un renombre en el indice de una ruta que entra a una
// de estado (.hoom/notas.txt -> .hoom/findings/n.json) saca .hoom/notas.txt
// de la evidencia sin commitear: ensucia el arbol y la negativa nombra
// .hoom/notas.txt. El renombre al reves (estado -> ruta que entra) tambien,
// nombrando la ruta nueva.
func TestCA482_RenombreEntreEstadoYEvidenciaEnsucia(t *testing.T) {
	dir, _ := edrRepo(t)
	dfGit(t, dir, "mv", ".hoom/notas.txt", ".hoom/findings/n.json")
	got, err := CambioSinCommitear(dir)
	if err != nil || got != ".hoom/notas.txt" {
		t.Fatalf("CA-482: .hoom/notas.txt movido (sin commitear) a .hoom/findings/n.json ensucia el arbol con .hoom/notas.txt: %q, %v", got, err)
	}
	dfGit(t, dir, "reset", "-q", "--hard")
	if err := os.MkdirAll(filepath.Join(dir, ".hoom", "verdicts", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	dfGit(t, dir, "mv", ".hoom/verdicts/2026-10-27T00-00-00Z_eeeeee.json", ".hoom/verdicts/sub/v.json")
	got, err = CambioSinCommitear(dir)
	if err != nil || got != ".hoom/verdicts/sub/v.json" {
		t.Fatalf("CA-482: un veredicto movido (sin commitear) a .hoom/verdicts/sub/v.json ensucia el arbol con la ruta nueva: %q, %v", got, err)
	}
}
