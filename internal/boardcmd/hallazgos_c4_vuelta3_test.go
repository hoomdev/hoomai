// Tests de regresion de la tercera vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en boardcmd: la historia de la
// tarjeta (seccion 1, CA-361/CA-362). Escritos desde el spec y la
// descripcion del hallazgo, sin leer la implementacion; repos git reales en
// t.TempDir() y telemetria escrita a mano, sin ningun CLI de IA. Los helpers
// compartidos (hi*, bd*, c4*) estan en historia_test.go,
// historia_telemetria_test.go, gather_test.go y hallazgos_c4_test.go; los de
// este archivo llevan el prefijo c4v3.
//
//   - 20261016T213325_058fb1 (risk), caso que la vuelta 2 no cubria: el
//     .jsonl del run de la tarjeta es un archivo REGULAR, pero el directorio
//     que lo contiene llega por un enlace simbolico: .hoom/runs entero (o
//     .hoom entero) es un enlace al del proyecto vecino. TimelineFor no puede
//     publicar las delegaciones de ese otro arbol como subagentes de la
//     tarjeta: la historia trae los subagentes de los runs de la tarjeta, que
//     viven en .hoom/runs/<id>.jsonl de la raiz. La raiz misma (o un
//     directorio por encima de ella) nombrada por un enlace, como el TMPDIR
//     de macOS bajo /var, no es eso: sus subagentes siguen saliendo.
package boardcmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

const (
	c4v3Run   = "20260921T094000_vecino" // el run de la tarjeta cuyo .jsonl vive en el vecino
	c4v3Sobre = "20260921T093900_sobre8" // el sobre de la tarjeta que lo referencia (variante "sobre")
)

// c4v3Marcas no pueden aparecer en ninguna entrada: son del log del vecino.
var c4v3Marcas = []string{"espia-ajeno", "otro-espia", "toolu_AJ", "secreto"}

// c4v3EnlaceDir crea el enlace simbolico enlace -> destino (destino tal cual:
// absoluto o relativo al directorio del enlace) y exige que resuelva a un
// directorio. En Windows os.Symlink pide privilegios: el caso es de los
// sistemas con enlaces simbolicos.
func c4v3EnlaceDir(t *testing.T, destino, enlace string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	if err := os.MkdirAll(filepath.Dir(enlace), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(destino, enlace); err != nil {
		t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
	}
	fi, err := os.Lstat(enlace)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("fixture: %s es un enlace simbolico: %v %v", enlace, fi, err)
	}
	if fi, err := os.Stat(enlace); err != nil || !fi.IsDir() {
		t.Fatalf("fixture: el enlace %s resuelve a un directorio (%s): %v %v", enlace, destino, fi, err)
	}
}

// c4v3Base es un directorio temporal por su ruta real.
func c4v3Base(t *testing.T) string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return base
}

// c4v3Tarjeta arma en root la tarjeta precios: repo git, item commiteado.
func c4v3Tarjeta(t *testing.T, root string) {
	t.Helper()
	c4RepoEn(t, root)
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0.Add(-24*time.Hour), "")
}

// c4v3LogAjeno escribe en dir/.hoom/runs/<c4v3Run>.jsonl, como archivo
// regular, la narracion del vecino: delegaciones de verdad. Con via
// "sidecar", al lado va el sidecar del run, de la tarea precios.
func c4v3LogAjeno(t *testing.T, dir, root, via string) {
	t.Helper()
	hiEventos(t, dir, c4v3Run,
		hiEv(40, "start", "", "", "init"),
		hiEv(41, "agent", "espia-ajeno", "toolu_AJ", "Agent: secreto del otro proyecto"),
		hiEv(42, "agent_end", "espia-ajeno", "toolu_AJ", "espia-ajeno termino"),
		hiEv(43, "agent", "otro-espia", "toolu_AJ2", "Agent: mas secretos"))
	if via == "sidecar" {
		bdRunDisco(t, dir, runcmd.Meta{ID: c4v3Run, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
			CreatedAt: hiMin(40), EndedAt: hiMin(50), Status: runcmd.StatusDone})
	}
}

// c4v3SobreDeLaTarjeta escribe en root el sobre de la tarjeta que nombra
// c4v3Run (variante "sobre").
func c4v3SobreDeLaTarjeta(t *testing.T, root string) {
	t.Helper()
	bdSobreDisco(t, root, envelope.Record{ID: c4v3Sobre, Role: "writer", Provider: "claude", Task: bdSlug, Dir: root,
		RunID: c4v3Run, Stage: "ok", Step: 7, Steps: 7, Status: envelope.StatusDeliverable,
		StartedAt: hiMin(39), UpdatedAt: hiMin(50), EndedAt: hiMin(50)})
}

// c4v3CopiarDir copia los archivos regulares de src (un nivel) a dst.
func c4v3CopiarDir(t *testing.T, src, dst string) {
	t.Helper()
	des, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, d := range des {
		if !d.Type().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(src, d.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, d.Name()), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// c4v3VeAlEspia exige (guarda del fixture) que la delegacion a espia-ajeno
// sea una entrada subagente de la tarjeta: el run es de la tarjeta y su
// .jsonl, regular y en un directorio real, se lee.
func c4v3VeAlEspia(t *testing.T, root, via, que string) {
	t.Helper()
	tl := hiTimeline(t, "CA-362", root)
	for _, e := range hiDeKind(tl, KindSubagente) {
		if strings.HasPrefix(e.Who, "espia-ajeno") {
			return
		}
	}
	t.Fatalf("fixture (%s): con %s, la delegacion a espia-ajeno es una entrada subagente de la tarjeta:\n%s", via, que, hiListado(tl))
}

// c4v3SinAjenos exige que la historia no publique nada del log del vecino:
// ninguna entrada subagente ni subagente-fin, y ninguna entrada que nombre lo
// que hay en el. Que TimelineFor devuelva un error en vez de la historia
// tambien cumple (no publica nada); se deja constancia en el log.
func c4v3SinAjenos(t *testing.T, root, via, como string) {
	t.Helper()
	tl, err := TimelineFor(root, "main", "high", bdSlug, hiAhora)
	if err != nil {
		t.Logf("CA-362 (hallazgo 058fb1, %s): con %s, TimelineFor no da la historia: %v (no publica nada del vecino)", via, como, err)
		return
	}
	raw, err := json.Marshal(tl.Entries)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range c4v3Marcas {
		if strings.Contains(string(raw), m) {
			t.Errorf("CA-361, CA-362 (hallazgo 058fb1, %s): con %s, la historia no publica el log del vecino; una entrada nombra %q:\n%s",
				via, como, m, hiListado(tl))
			break
		}
	}
	sub, fin := hiDeKind(tl, KindSubagente), hiDeKind(tl, KindSubagenteFin)
	if len(sub) != 0 || len(fin) != 0 {
		t.Errorf("CA-362 (hallazgo 058fb1, %s): con %s, ningun .jsonl de la tarjeta se lee desde la raiz, asi que no hay subagentes; hubo %d subagente y %d subagente-fin: %+v %+v\n%s",
			via, como, len(sub), len(fin), sub, fin, hiListado(tl))
	}
}

// CA-361, CA-362 (hallazgo 058fb1): si .hoom/runs entero es un enlace
// simbolico al .hoom/runs del proyecto vecino (por ruta absoluta o
// relativa), el .jsonl del run de la tarjeta (nombrado por el sidecar de la
// tarjeta, que esta al lado, o por un sobre de la tarjeta) es un archivo
// REGULAR del vecino, con delegaciones de verdad: TimelineFor no lo publica,
// ni como subagente ni como subagente-fin, y ninguna entrada nombra lo que
// hay en el.
//
// Guarda en el mismo fixture: con .hoom/runs como directorio real y una
// copia de los mismos archivos, la delegacion a espia-ajeno si es un
// subagente de la tarjeta (el run es de la tarjeta y su nombre se lee; el
// enlace es lo unico que cambia).
func TestHallazgo_058fb1_CA361_CA362_RunsEnlazadoNoTraeSubagentesDelVecino(t *testing.T) {
	for _, via := range []string{"sidecar", "sobre"} {
		for _, cs := range []struct{ nombre, destino string }{
			{"enlace absoluto al .hoom/runs del vecino", ""},
			{"enlace relativo al .hoom/runs del vecino", filepath.Join("..", "..", "otro", ".hoom", "runs")},
		} {
			t.Run(via+"/"+cs.nombre, func(t *testing.T) {
				base := c4v3Base(t)
				root := filepath.Join(base, "proyecto")
				otro := filepath.Join(base, "otro")
				runs := filepath.Join(root, ".hoom", "runs")
				vecinoRuns := filepath.Join(otro, ".hoom", "runs")
				c4v3Tarjeta(t, root)
				c4v3LogAjeno(t, otro, root, via)
				if via == "sobre" {
					c4v3SobreDeLaTarjeta(t, root)
				}

				// guarda: .hoom/runs real con una copia regular de lo mismo
				c4v3CopiarDir(t, vecinoRuns, runs)
				c4v3VeAlEspia(t, root, via, ".hoom/runs como directorio real")
				if err := os.RemoveAll(runs); err != nil {
					t.Fatal(err)
				}

				// el ataque: .hoom/runs entero es un enlace al del vecino
				destino := cs.destino
				if destino == "" {
					destino = vecinoRuns
				}
				c4v3EnlaceDir(t, destino, runs)
				if fi, err := os.Stat(filepath.Join(runs, c4v3Run+".jsonl")); err != nil || !fi.Mode().IsRegular() {
					t.Fatalf("fixture: a traves del enlace, %s.jsonl es un archivo regular del vecino: %v %v", c4v3Run, fi, err)
				}

				c4v3SinAjenos(t, root, via, ".hoom/runs como enlace a "+destino)
			})
		}
	}
}

// CA-361, CA-362 (hallazgo 058fb1): lo mismo un nivel mas arriba. Si .hoom
// entero es un enlace simbolico al .hoom del proyecto vecino (por ruta
// absoluta o relativa), .hoom/runs y el .jsonl del run de la tarjeta son un
// directorio y un archivo regulares del vecino: TimelineFor no publica sus
// delegaciones. El fixture: la tarjeta se arma con su .hoom real (item
// commiteado, sobre o sidecar, y el log con delegaciones), la guarda exige
// que en ese estado la delegacion a espia-ajeno salga, y despues ese .hoom
// se muda al vecino y en su lugar queda el enlace. Como todo .hoom llega por
// el enlace, que TimelineFor falle tambien cumple.
func TestHallazgo_058fb1_CA361_CA362_HoomEnlazadoNoTraeSubagentesDelVecino(t *testing.T) {
	for _, via := range []string{"sidecar", "sobre"} {
		for _, cs := range []struct{ nombre, destino string }{
			{"enlace absoluto al .hoom del vecino", ""},
			{"enlace relativo al .hoom del vecino", filepath.Join("..", "otro", ".hoom")},
		} {
			t.Run(via+"/"+cs.nombre, func(t *testing.T) {
				base := c4v3Base(t)
				root := filepath.Join(base, "proyecto")
				hoom := filepath.Join(root, ".hoom")
				vecinoHoom := filepath.Join(base, "otro", ".hoom")
				c4v3Tarjeta(t, root)
				c4v3LogAjeno(t, root, root, via)
				if via == "sobre" {
					c4v3SobreDeLaTarjeta(t, root)
				}

				// guarda: con .hoom real, la delegacion sale
				c4v3VeAlEspia(t, root, via, ".hoom como directorio real")

				// el ataque: ese .hoom vive en el vecino y la raiz lo ve por un enlace
				if err := os.MkdirAll(filepath.Dir(vecinoHoom), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(hoom, vecinoHoom); err != nil {
					t.Fatal(err)
				}
				destino := cs.destino
				if destino == "" {
					destino = vecinoHoom
				}
				c4v3EnlaceDir(t, destino, hoom)
				if fi, err := os.Lstat(filepath.Join(hoom, "runs", c4v3Run+".jsonl")); err != nil || !fi.Mode().IsRegular() {
					t.Fatalf("fixture: a traves del enlace, %s.jsonl es un archivo regular del vecino: %v %v", c4v3Run, fi, err)
				}

				c4v3SinAjenos(t, root, via, ".hoom como enlace a "+destino)
			})
		}
	}
}

// CA-361, CA-362 (hallazgo 058fb1, guarda): la raiz nombrada a traves de un
// enlace simbolico no es un enlace dentro de la raiz. Con .hoom y .hoom/runs
// reales, el .jsonl regular del run de la tarjeta da su subagente y su
// subagente-fin con la raiz: tal como la da t.TempDir() (en macOS el TMPDIR
// vive bajo /var, que es un enlace a /private/var), por su ruta real, por un
// enlace a la raiz misma y por un enlace a un directorio por encima de ella.
func TestHallazgo_058fb1_CA361_CA362_Guarda_RaizPorEnlaceTraeSusSubagentes(t *testing.T) {
	crudo := t.TempDir() // sin resolver: en macOS, /var/folders/...
	real, err := filepath.EvalSymlinks(crudo)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(crudo, "proyecto")
	c4v3Tarjeta(t, root)
	bdRunDisco(t, root, runcmd.Meta{ID: hiR1, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
		CreatedAt: hiMin(10), EndedAt: hiMin(30), Status: runcmd.StatusDone})
	hiEventos(t, root, hiR1,
		hiEv(10, "start", "", "", "init"),
		hiEv(12, "agent", "general-purpose", "toolu_OK", "Agent: buscar precios"),
		hiEv(14, "agent_end", "general-purpose", "toolu_OK", "general-purpose termino"),
		hiEv(29, "end", "", "", "listo"))

	aRaiz := filepath.Join(real, "enlace-al-proyecto")
	c4v3EnlaceDir(t, filepath.Join(real, "proyecto"), aRaiz)
	aArriba := filepath.Join(real, "enlace-arriba")
	c4v3EnlaceDir(t, real, aArriba)

	for _, r := range []struct{ nombre, root string }{
		{"raiz como la da t.TempDir", root},
		{"raiz por su ruta real", filepath.Join(real, "proyecto")},
		{"raiz por un enlace a ella", aRaiz},
		{"un enlace por encima de la raiz", filepath.Join(aArriba, "proyecto")},
	} {
		tl := hiTimeline(t, "CA-362", r.root)
		sub, fin := hiDeKind(tl, KindSubagente), hiDeKind(tl, KindSubagenteFin)
		if len(sub) != 1 || sub[0].Who != "general-purpose (claude)" || len(fin) != 1 || fin[0].Who != "general-purpose (claude)" {
			t.Errorf("CA-362 (hallazgo 058fb1, guarda, %s: %s): el run de la tarjeta da 1 subagente y 1 subagente-fin de general-purpose; hubo %d y %d:\n%s",
				r.nombre, r.root, len(sub), len(fin), hiListado(tl))
			continue
		}
		if a := ".hoom/runs/" + hiR1 + ".jsonl"; sub[0].Artifact != a || fin[0].Artifact != a {
			t.Errorf("CA-362 (hallazgo 058fb1, guarda, %s): las delegaciones salen de %s: %q, %q", r.nombre, a, sub[0].Artifact, fin[0].Artifact)
		}
	}
}
