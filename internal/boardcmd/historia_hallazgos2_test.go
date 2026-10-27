// Tests de los hallazgos 752455 y 1499b7 de historia-en-el-tablero (ronda 2
// del test-writer) sobre Candidatas, la lista de lo que `hoom item backfill`
// haria con cada tarea: un directorio en el lugar del item no es "ya tiene
// item" ni "crearia", y un primer encabezado de spec con un caracter de
// control o de direccion no da un titulo. Escritos desde el spec
// .hoom/specs/historia-en-el-tablero.md y los textos de los hallazgos, sin
// leer la implementacion. Repos git reales en t.TempDir(); veredictos,
// hallazgos, specs e items escritos a mano. Sin ningun CLI de IA. Todo
// identificador de este archivo lleva el prefijo h2.
package boardcmd

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hoomdev/hoomai/internal/finding"
	"github.com/hoomdev/hoomai/internal/gitx"
	"github.com/hoomdev/hoomai/internal/item"
	"github.com/hoomdev/hoomai/internal/verdict"
)

// h2TituloAcentos es un titulo normal que tiene que seguir calificando:
// acentos (precompuestos y con un acento combinante), enie, CJK, hebreo (texto
// de derecha a izquierda SIN caracteres de direccion), emojis, una secuencia
// con ZWJ (U+200D), un selector de variacion (U+FE0F) y una bandera.
const h2TituloAcentos = "Café, ñandú y acción de José: 日本語 שלום 🚀 👩‍💻 ✔️ 🇦🇷"

// h2EsHostil dice si r no puede estar en un titulo que sale de un spec: los
// controles C0 (salvo \t y \n), DEL, los controles C1 (U+0080..U+009F, entre
// ellos el CSI de 8 bits U+009B) y los caracteres de embebido, anulacion y
// aislamiento de direccion (U+202A..U+202E, U+2066..U+2069; los de "Trojan
// Source").
func h2EsHostil(r rune) bool {
	switch {
	case r < 0x20:
		return r != '\t' && r != '\n'
	case r >= 0x7f && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e:
		return true
	case r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}

// h2Hostiles son TODAS las runas para las que h2EsHostil es verdad.
func h2Hostiles() []rune {
	var out []rune
	for r := rune(0); r <= 0x2069; r++ {
		if h2EsHostil(r) {
			out = append(out, r)
		}
	}
	return out
}

// h2PrimeraHostil devuelve la primera runa hostil de s.
func h2PrimeraHostil(s string) (rune, bool) {
	for _, r := range s {
		if h2EsHostil(r) {
			return r, true
		}
	}
	return 0, false
}

// h2DiceTitulo dice si el motivo habla del titulo o del encabezado del spec.
func h2DiceTitulo(motivo string) bool {
	m := strings.ToLower(motivo)
	return strings.Contains(m, "encabezado") || strings.Contains(m, "titulo") || strings.Contains(m, "título")
}

// h2SlugHostil es la tarea cuyo encabezado lleva r.
func h2SlugHostil(r rune) string { return fmt.Sprintf("hostil-%04x", r) }

// h2Veredicto escribe en root un veredicto completo y verde (con
// spec_approved en pass) del spec de slug, con el estado git snap. Es
// htVeredicto sin una foto de git por veredicto (la prueba de las runas
// escribe 75).
func h2Veredicto(t *testing.T, root string, snap gitx.Info, slug string) string {
	t.Helper()
	v := &verdict.Verdict{Project: "demo", CreatedAt: bdT0, Spec: ".hoom/specs/" + slug + ".md", Git: snap, Gates: bdGatesVerdes()}
	v.Finalize()
	if _, err := verdict.Write(root, v); err != nil {
		t.Fatal(err)
	}
	htForma(t, v, true, false)
	if v.ID == "" || v.Git.Commit == "" {
		t.Fatalf("precondicion: el veredicto de %s tiene id y git.commit: %+v", slug, v)
	}
	return v.ID
}

// h2SinItem son las tareas sin item de Candidatas, en orden.
func h2SinItem(cs []Candidata) []string {
	out := []string{}
	for _, c := range cs {
		if !c.ConItem {
			out = append(out, c.Slug)
		}
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// 1499b7: un primer encabezado con un caracter de control o de direccion.
// ---------------------------------------------------------------------------

// CA-446, CA-447, hallazgo 1499b7: el titulo del item sale del primer
// encabezado del spec, que llega por git sin que nadie lo tipee. Si ese
// encabezado trae un caracter de control o de direccion, la tarea NO
// califica (aunque su veredicto verde aprobado e integrado este) y su motivo
// habla del encabezado o del titulo, sin repetir el caracter crudo (el motivo
// se imprime en la terminal). Propiedad sobre TODAS las runas hostiles (C0
// salvo \t y \n, DEL, C1, U+202A..U+202E, U+2066..U+2069), cada una en el
// medio del encabezado de su propia tarea. El resto sigue igual (protecciones
// contra un arreglo demasiado amplio): un titulo con acentos, emojis, ZWJ,
// CJK y hebreo, un spec con fin de linea CRLF (el \r del final de linea no es
// parte del encabezado) y una tarea de correccion sin spec siguen calificando
// con su titulo exacto. Ninguna tarea que califica trae un titulo hostil, y
// la lista sigue siendo la del evidencia-sin-item del doctor.
func TestCA447_Hallazgo1499b7_EncabezadoConControlNoCalifica(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	hostiles := h2Hostiles()
	if len(hostiles) != 30+1+32+5+4 {
		t.Fatalf("precondicion: 72 runas hostiles, fueron %d", len(hostiles))
	}
	for _, r := range hostiles {
		htSpec(t, root, h2SlugHostil(r), "# Spec: Precios"+string(r)+"rojo")
	}
	htSpec(t, root, "precios", "# Spec: Precios por region")
	htSpec(t, root, "acentos", "# Spec: "+h2TituloAcentos)
	bdEscribir(t, root, ".hoom/specs/crlf.md", strings.ReplaceAll(htSpecTexto("# Spec: Con CRLF"), "\n", "\r\n"))
	bdCommitear(t, root, "specs")

	ids := map[string]string{}
	conSpec := []string{"precios", "acentos", "crlf"}
	for _, r := range hostiles {
		conSpec = append(conSpec, h2SlugHostil(r))
	}
	snap := gitx.Snapshot(root, "main") // una sola foto de git: todos los verdes son del mismo commit de main
	for _, s := range conSpec {
		ids[s] = h2Veredicto(t, root, snap, s)
	}
	htCerrar(t, root, htHallazgo(t, root, "correccion", "high"), finding.StatusCorrected)
	bdCommitear(t, root, "veredictos y hallazgos")

	antes := bdFoto(t, root)
	cs := htCandidatas(t, "CA-447", root, "high")
	bdMismaFoto(t, "CA-447", antes, bdFoto(t, root))

	t.Run("CA-447 protecciones: acentos, emojis, CRLF y sin spec siguen calificando", func(t *testing.T) {
		htCalifica(t, "CA-447", cs, "precios", "veredicto "+ids["precios"], "feature", "Precios por region")
		htCalifica(t, "CA-447", cs, "acentos", "veredicto "+ids["acentos"], "feature", h2TituloAcentos)
		htCalifica(t, "CA-447", cs, "crlf", "veredicto "+ids["crlf"], "feature", "Con CRLF")
		htCalifica(t, "CA-448", cs, "correccion", item.HistoricoHallazgos, "bug", "correccion")
	})

	t.Run("CA-447 un encabezado con un caracter de control o de direccion no califica", func(t *testing.T) {
		var mal []string
		for _, r := range hostiles {
			c := htCand(t, "CA-447 (1499b7)", cs, h2SlugHostil(r))
			_, motivoHostil := h2PrimeraHostil(c.Motivo)
			if c.ConItem || c.Historico != "" || c.Motivo == "" || !h2DiceTitulo(c.Motivo) ||
				strings.Contains(c.Motivo, htMotivoVeredicto) || motivoHostil {
				mal = append(mal, fmt.Sprintf("  U+%04X: %+q", r, fmt.Sprintf("%+v", c)))
			}
		}
		if len(mal) > 0 {
			t.Fatalf("CA-447 (1499b7): con un caracter de control o de direccion en el primer encabezado del spec la tarea no "+
				"califica (sin historico) y su motivo habla del encabezado o del titulo, sin el caracter crudo; %d de %d no:\n%s",
				len(mal), len(hostiles), strings.Join(mal, "\n"))
		}
	})

	t.Run("CA-447 ninguna que califica trae un titulo hostil", func(t *testing.T) {
		for _, c := range cs {
			if r, ok := h2PrimeraHostil(c.Titulo); ok && c.Historico != "" {
				t.Fatalf("CA-447 (1499b7): una tarea que califica nunca trae un titulo con U+%04X: %+q", r, fmt.Sprintf("%+v", c))
			}
		}
	})

	t.Run("CA-446 la misma lista que evidencia-sin-item", func(t *testing.T) {
		if doc, got := htSlugsDelDoctor(t, "CA-446", root, "high"), h2SinItem(cs); !reflect.DeepEqual(doc, got) {
			t.Fatalf("CA-446: Candidatas y el evidencia-sin-item del doctor miran la misma lista:\ndoctor:     %v\ncandidatas: %v", doc, got)
		}
	})
}

// ---------------------------------------------------------------------------
// 752455: un directorio en el lugar del item.
// ---------------------------------------------------------------------------

// CA-446, CA-453, hallazgo 752455: si .hoom/items/<slug>.yaml es un
// DIRECTORIO (con un archivo adentro, como llegaria por git, o vacio), la
// tarea no tiene item: no sale como ConItem ("ya tiene item"), y tampoco
// califica (backfill no puede crear el item ahi): sin historico y con un
// motivo que dice que en el lugar del item hay algo que no es un item (no el
// de "sin veredicto verde", que si esta). Los controles: la tarea de al lado
// califica, y una con un item de verdad sale como ConItem. Candidatas sigue
// siendo read-only y su lista sin item es la del evidencia-sin-item del
// doctor.
func TestCA453_Hallazgo752455_DirectorioEnLugarDelItemNoCalifica(t *testing.T) {
	root := bdRepo(t, htBlockOn)
	slugs := []string{"precios", "vacia", "control", "con-item"}
	for _, s := range slugs {
		htSpec(t, root, s, "# Spec: "+s)
		bdAprobar(t, root, s)
	}
	bdCommitear(t, root, "specs")
	ids := map[string]string{}
	for _, s := range slugs {
		v := htVeredicto(t, root, root, s, bdT0, false, bdGatesVerdes(), nil)
		htForma(t, v, true, false)
		ids[s] = v.ID
	}
	bdEscribir(t, root, ".hoom/items/precios.yaml/nota.txt", "no soy un item\n")
	bdEscribir(t, root, ".hoom/items/con-item.yaml", htItemYAML("Con item", "feature", ""))
	bdCommitear(t, root, "veredictos, un item y un directorio en el lugar de otro")
	if err := os.MkdirAll(filepath.Join(root, ".hoom", "items", "vacia.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}

	antes := bdFoto(t, root)
	cs := htCandidatas(t, "CA-453", root, "high")
	bdMismaFoto(t, "CA-453", antes, bdFoto(t, root))

	for _, s := range []string{"precios", "vacia"} {
		t.Run("CA-453 "+s+": un directorio no es un item", func(t *testing.T) {
			c := htCand(t, "CA-453 (752455)", cs, s)
			if c.ConItem {
				t.Fatalf("CA-453 (752455): con un directorio en .hoom/items/%s.yaml la tarea no \"ya tiene item\": %+v", s, c)
			}
			if c.Historico != "" || c.Motivo == "" {
				t.Fatalf("CA-446 (752455): con un directorio en .hoom/items/%s.yaml backfill no puede crear el item: no califica "+
					"(sin historico) y lo dice en su motivo; fue %+v", s, c)
			}
			if !strings.Contains(strings.ToLower(c.Motivo), "item") || strings.Contains(c.Motivo, htMotivoVeredicto) {
				t.Fatalf("CA-446 (752455): el motivo de %s dice que en el lugar del item hay algo que no es un item (su veredicto "+
					"verde si esta): %q", s, c.Motivo)
			}
		})
	}
	t.Run("controles", func(t *testing.T) {
		htCalifica(t, "CA-447 (control)", cs, "control", "veredicto "+ids["control"], "feature", "control")
		if c := htCand(t, "CA-453 (control)", cs, "con-item"); !c.ConItem || c.Historico != "" {
			t.Fatalf("CA-453 (control): la tarea con un item de verdad sale como ConItem: %+v", c)
		}
	})
	t.Run("CA-446 la misma lista que evidencia-sin-item", func(t *testing.T) {
		if doc, got := htSlugsDelDoctor(t, "CA-446", root, "high"), h2SinItem(cs); !reflect.DeepEqual(doc, got) {
			t.Fatalf("CA-446: Candidatas y el evidencia-sin-item del doctor miran la misma lista:\ndoctor:     %v\ncandidatas: %v", doc, got)
		}
	})
}
