// Tests de regresion de la segunda vuelta de la review de la cabina C4 sobre
// .hoom/specs/historia-doctor-y-cinta.md, en boardcmd: la historia de la
// tarjeta (seccion 1, CA-361/CA-362). Escritos desde el spec y la
// descripcion del hallazgo, sin leer la implementacion; repos git reales en
// t.TempDir() y telemetria escrita a mano, sin ningun CLI de IA. Los helpers
// compartidos (hi*, bd*, c4*) estan en historia_test.go, gather_test.go y
// hallazgos_c4_test.go; los de este archivo llevan el prefijo c4v2.
//
//   - 20261016T213325_058fb1 (risk): un .hoom/runs/<id>.jsonl de nombre
//     valido que es un enlace simbolico a un log de otro arbol hace que
//     TimelineFor lea y publique las delegaciones de ese arbol. La historia
//     trae los subagentes de los runs de la tarjeta, que viven en
//     .hoom/runs/<id>.jsonl: lo que el enlace trae de afuera no es una
//     entrada.
package boardcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hoomdev/hoomai/internal/envelope"
	"github.com/hoomdev/hoomai/internal/runcmd"
)

// c4v2Enlace crea el enlace simbolico enlace -> destino (destino tal cual:
// absoluto o relativo al directorio del enlace) y exige que resuelva a un
// archivo. En Windows os.Symlink pide privilegios: el caso es de los
// sistemas con enlaces simbolicos.
func c4v2Enlace(t *testing.T, destino, enlace string) {
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
	if fi, err := os.Stat(enlace); err != nil || !fi.Mode().IsRegular() {
		t.Fatalf("fixture: el enlace %s resuelve a un archivo regular (%s): %v %v", enlace, destino, fi, err)
	}
}

const (
	c4v2Run    = "20260921T094000_enlace" // el run de la tarjeta cuyo .jsonl se cambia por un enlace
	c4v2Sobre  = "20260921T093900_sobre7" // el sobre de la tarjeta que lo referencia (variante "sobre")
	c4v2Puente = "20260921T095000_puente" // un eslabon de la cadena de enlaces
)

// c4v2Marcas no pueden aparecer en ninguna entrada: son del log ajeno.
var c4v2Marcas = []string{"espia-ajeno", "otro-espia", "toolu_AJ", "secreto"}

// c4v2Proyecto arma la tarjeta precios en base/proyecto, con un run legitimo
// (hiR1, .jsonl regular, una delegacion a general-purpose que cierra), el
// log ajeno del proyecto vecino base/otro/.hoom/runs/ajeno.jsonl (con
// delegaciones de verdad), una copia del log ajeno en el mismo proyecto fuera
// de .hoom/runs (.hoom/notas/ajeno.jsonl), y el run c4v2Run de la tarjeta:
// via "sidecar" (su .meta.json con task precios) o via "sobre" (un sobre de
// la tarjeta con run_id c4v2Run). El .jsonl de c4v2Run todavia no existe.
func c4v2Proyecto(t *testing.T, via string) (base, root, vecino string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "proyecto")
	c4RepoEn(t, root)
	bdItemArchivo(t, root, bdSlug, "Precios", "")
	hiCommit(t, root, "alta del item", hiTelD0.Add(-24*time.Hour), "")

	bdRunDisco(t, root, runcmd.Meta{ID: hiR1, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
		CreatedAt: hiMin(10), EndedAt: hiMin(30), Status: runcmd.StatusDone})
	hiEventos(t, root, hiR1,
		hiEv(10, "start", "", "", "init"),
		hiEv(12, "agent", "general-purpose", "toolu_OK", "Agent: buscar precios"),
		hiEv(14, "agent_end", "general-purpose", "toolu_OK", "general-purpose termino"),
		hiEv(29, "end", "", "", "listo"))

	vecino = hiEventos(t, filepath.Join(base, "otro"), "ajeno",
		hiEv(40, "start", "", "", "init"),
		hiEv(41, "agent", "espia-ajeno", "toolu_AJ", "Agent: secreto del otro proyecto"),
		hiEv(42, "agent_end", "espia-ajeno", "toolu_AJ", "espia-ajeno termino"),
		hiEv(43, "agent", "otro-espia", "toolu_AJ2", "Agent: mas secretos"))
	raw, err := os.ReadFile(vecino)
	if err != nil {
		t.Fatal(err)
	}
	bdEscribir(t, root, filepath.Join(".hoom", "notas", "ajeno.jsonl"), string(raw))

	switch via {
	case "sidecar":
		bdRunDisco(t, root, runcmd.Meta{ID: c4v2Run, Provider: "claude", Role: "writer", Task: bdSlug, Dir: root,
			CreatedAt: hiMin(40), EndedAt: hiMin(50), Status: runcmd.StatusDone})
	case "sobre":
		bdSobreDisco(t, root, envelope.Record{ID: c4v2Sobre, Role: "writer", Provider: "claude", Task: bdSlug, Dir: root,
			RunID: c4v2Run, Stage: "ok", Step: 7, Steps: 7, Status: envelope.StatusDeliverable,
			StartedAt: hiMin(39), UpdatedAt: hiMin(50), EndedAt: hiMin(50)})
	default:
		t.Fatalf("via desconocida %q", via)
	}
	return base, root, vecino
}

// c4v2Nombra dice si alguna entrada nombra algo del log ajeno.
func c4v2Nombra(tl Timeline) (TimelineEntry, string, bool) {
	for _, e := range tl.Entries {
		todo := strings.Join([]string{e.Who, e.Plain, e.Summary, e.Artifact, e.Ref}, " | ")
		for _, m := range c4v2Marcas {
			if strings.Contains(todo, m) {
				return e, m, true
			}
		}
	}
	return TimelineEntry{}, "", false
}

// CA-361, CA-362 (hallazgo 058fb1): la historia trae los subagentes de los
// runs de la tarjeta, que viven en .hoom/runs/<id>.jsonl. Si el .jsonl de un
// run de la tarjeta (con nombre valido, de un run con sidecar de la tarjeta
// o referenciado por un sobre de la tarjeta) es un enlace simbolico a un
// .jsonl de otro directorio (el proyecto vecino por ruta absoluta o
// relativa, el mismo proyecto fuera de .hoom/runs, o una cadena que pasa por
// otro enlace de .hoom/runs), TimelineFor se lee igual pero ninguna entrada
// sale de ese archivo: ni subagente ni subagente-fin, y ninguna nombra lo que
// hay en el. Los subagentes del run legitimo siguen saliendo.
//
// Guarda en el mismo fixture: con el mismo contenido en un .jsonl REGULAR en
// ese lugar, sus delegaciones si salen (el run es de la tarjeta y su nombre
// se lee; el enlace es lo unico que cambia).
func TestHallazgo_058fb1_CA362_EnlaceEnRunsNoTraeSubagentesAjenos(t *testing.T) {
	destinos := []struct{ nombre, destino string }{
		{"enlace absoluto al proyecto vecino", ""}, // se completa con la ruta del vecino
		{"enlace relativo al proyecto vecino", filepath.Join("..", "..", "..", "otro", ".hoom", "runs", "ajeno.jsonl")},
		{"enlace al mismo proyecto, fuera de .hoom/runs", filepath.Join("..", "notas", "ajeno.jsonl")},
		{"cadena de enlaces que pasa por otro de .hoom/runs", c4v2Puente + ".jsonl"},
	}
	for _, via := range []string{"sidecar", "sobre"} {
		for _, d := range destinos {
			t.Run(via+"/"+d.nombre, func(t *testing.T) {
				_, root, vecino := c4v2Proyecto(t, via)
				jsonl := filepath.Join(root, ".hoom", "runs", c4v2Run+".jsonl")
				destino := d.destino
				if destino == "" {
					destino = vecino
				}

				// guarda: el mismo contenido, regular, se lee
				raw, err := os.ReadFile(vecino)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(jsonl, raw, 0o644); err != nil {
					t.Fatal(err)
				}
				tl := hiTimeline(t, "CA-362", root)
				visto := false
				for _, e := range hiDeKind(tl, KindSubagente) {
					visto = visto || strings.HasPrefix(e.Who, "espia-ajeno")
				}
				if !visto {
					t.Fatalf("fixture (%s): con un .jsonl regular en %s, la delegacion a espia-ajeno es una entrada subagente de la tarjeta:\n%s",
						via, filepath.Join(".hoom", "runs", c4v2Run+".jsonl"), hiListado(tl))
				}

				// el ataque: el mismo nombre, como enlace simbolico al log ajeno
				if err := os.Remove(jsonl); err != nil {
					t.Fatal(err)
				}
				if d.destino == c4v2Puente+".jsonl" {
					c4v2Enlace(t, vecino, filepath.Join(root, ".hoom", "runs", c4v2Puente+".jsonl"))
				}
				c4v2Enlace(t, destino, jsonl)

				tl = hiTimeline(t, "CA-362", root)
				if e, m, ok := c4v2Nombra(tl); ok {
					t.Fatalf("CA-362 (hallazgo 058fb1, %s): un .jsonl de .hoom/runs que es un enlace a %s no lleva la historia fuera de .hoom/runs: la entrada %s nombra %q del log ajeno: %+v\n%s",
						via, destino, e.Kind, m, e, hiListado(tl))
				}
				sub, fin := hiDeKind(tl, KindSubagente), hiDeKind(tl, KindSubagenteFin)
				if len(sub) != 1 || sub[0].Who != "general-purpose (claude)" || len(fin) != 1 || fin[0].Who != "general-purpose (claude)" {
					t.Fatalf("CA-362 (hallazgo 058fb1, %s): los unicos subagentes son los del run legitimo (1 subagente y 1 subagente-fin de general-purpose); hubo %d y %d:\n%s",
						via, len(sub), len(fin), hiListado(tl))
				}
				if a := ".hoom/runs/" + hiR1 + ".jsonl"; sub[0].Artifact != a || fin[0].Artifact != a {
					t.Fatalf("CA-362 (hallazgo 058fb1, %s): las delegaciones salen de %s: %q, %q", via, a, sub[0].Artifact, fin[0].Artifact)
				}
			})
		}
	}
}

// CA-362 (hallazgo 058fb1, guarda): la raiz del proyecto nombrada a traves
// de un enlace simbolico (como /tmp o /var en macOS) no es un enlace dentro
// de .hoom/runs: con la raiz por el enlace, las delegaciones del .jsonl
// regular de un run de la tarjeta salen igual que con la ruta real.
func TestHallazgo_058fb1_CA362_Guarda_RaizPorEnlaceTraeSusSubagentes(t *testing.T) {
	base, root, _ := c4v2Proyecto(t, "sidecar")
	enlace := filepath.Join(base, "enlace-al-proyecto")
	if runtime.GOOS == "windows" {
		t.Skip("los enlaces simbolicos en Windows piden privilegios: el caso se prueba en Linux y macOS")
	}
	if err := os.Symlink(root, enlace); err != nil {
		t.Skipf("este sistema no deja crear enlaces simbolicos: %v", err)
	}
	for _, r := range []string{root, enlace} {
		tl := hiTimeline(t, "CA-362", r)
		sub, fin := hiDeKind(tl, KindSubagente), hiDeKind(tl, KindSubagenteFin)
		if len(sub) != 1 || sub[0].Who != "general-purpose (claude)" || len(fin) != 1 || fin[0].Who != "general-purpose (claude)" {
			t.Fatalf("CA-362 (hallazgo 058fb1, guarda, raiz %s): el run legitimo da 1 subagente y 1 subagente-fin de general-purpose; hubo %d y %d:\n%s",
				r, len(sub), len(fin), hiListado(tl))
		}
		if a := ".hoom/runs/" + hiR1 + ".jsonl"; sub[0].Artifact != a {
			t.Fatalf("CA-362 (hallazgo 058fb1, guarda, raiz %s): la delegacion sale de %s: %q", r, a, sub[0].Artifact)
		}
	}
}
